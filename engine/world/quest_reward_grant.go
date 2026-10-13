package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	questRewardChoiceLimit  = 6
	inventoryRewardBag      = 0
	inventoryRewardFirst    = 23
	inventoryRewardLast     = 38
	questInventoryFull      = 4
	questFlagsDaily         = 0x00001000
	questFlagsWeekly        = 0x00008000
	questFlagsPVP           = 0x00002000 // QUEST_FLAGS_FLAGS_PVP (QuestDef.h:145)
	questFlagsNoMoneyFromXP = 0x00000100 // QUEST_FLAGS_NO_MONEY_FROM_XP (QuestDef.h:140)
	questBindQuestItem      = 4          // BIND_QUEST_ITEM (ItemTemplate.h:102)
	questSpecialRepeatable  = 0x001
	questSpecialDungeonFind = 0x008
	questSpecialMonthly     = 0x010
	questSpecialAllowed     = 0x03F
)

var errQuestInventoryFull = errors.New("quest reward inventory is full")
var errQuestAlreadyRewarded = errors.New("quest reward already claimed in current reset window")

type inventoryRewardRecord struct {
	Bag      int64
	Slot     uint8
	ItemGUID uint32
	Entry    uint32
	Count    uint32
}

type questItemGrant struct {
	ItemGUID       uint32
	Entry          uint32
	Count          uint32
	Bag            uint8
	Slot           uint32
	InventoryCount uint32
	Stacked        bool
	// Template duration seeded at creation (Item::Create, Item.cpp:291) so
	// the reward-claim handler can mirror Player::AddItemDurations.
	Duration uint32
}

type questRewardPersistenceState struct {
	DailyFlag     bool
	DailyStatus   bool
	DungeonFinder bool
	Weekly        bool
	Monthly       bool
	Seasonal      bool
	Rewarded      bool
	SortID        int32
	EventID       uint32
}

func (s *session) handleQuestgiverChooseReward(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	giverGUID, err := reader.ReadU64()
	if err != nil {
		return true
	}
	questID, err := reader.ReadU32()
	if err != nil {
		return true
	}
	reward, err := reader.ReadU32()
	if err != nil {
		return true
	}
	if reward >= questRewardChoiceLimit {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reward", reward, "reason", "invalid choice")
		return true
	}
	if _, ok := s.questGiverEnder(ctx, giverGUID, questID); !ok {
		return true
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		s.debug("quest reward load failed", "account", s.accountName, "quest", questID, "error", err)
		return true
	}
	if s.isQuestRewarded(ctx, questID) {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reason", "already rewarded")
		return true
	}
	status, err := s.characterQuestStatus(ctx, questID)
	if err != nil {
		return false
	}
	// QuestHandler.cpp:293-300 — this packet never transitions quest state;
	// the client confirms the turn-in through the request-reward packet first.
	// A cheat client skipping that step is rejected here exactly like the C++
	// gate (QUEST_STATUS_COMPLETE or Quest::IsAutoComplete, QuestDef.cpp:315).
	if status != questStatusComplete && !s.questIsAutoComplete(ctx, questID, view.Detail.Flags) {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reason", "quest incomplete", "status", status)
		return true
	}
	if len(view.Detail.ChoiceItems) > 0 && (reward >= uint32(len(view.Detail.ChoiceItems)) || view.Detail.ChoiceItems[reward].ID == 0) {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reward", reward, "reason", "choice unavailable")
		return true
	}
	if complete, itemErr := s.hasQuestRequiredItems(ctx, view.RequiredItems); itemErr != nil {
		return false
	} else if !complete {
		return true
	}
	// Player::CanRewardQuest (Player.cpp:15094-15095): a quest that requires
	// money cannot be turned in without it (silent on this arm).
	if moneyRew := s.questRewOrReqMoney(view.Detail); moneyRew < 0 && s.player.Money < uint32(-int64(moneyRew)) {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reason", "insufficient money")
		return true
	}
	grants, destroyedGUIDs, err := s.commitQuestReward(ctx, view, reward)
	if errors.Is(err, errQuestInventoryFull) {
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_FAILED), buildQuestFailed(view.Detail.ID, questInventoryFull), true)
		return true
	}
	if errors.Is(err, errQuestAlreadyRewarded) {
		s.debug("quest reward rejected", "account", s.accountName, "quest", questID, "reason", "already rewarded in current reset window")
		return true
	}
	if err != nil {
		s.debug("quest reward commit failed", "account", s.accountName, "quest", questID, "error", err)
		return false
	}
	return s.applyQuestRewardGrants(ctx, view, questID, giverGUID, reward, grants, destroyedGUIDs, true)
}

// rewardTrackingQuest mirrors the TRACKING arm of Player::CompleteQuest
// (Player.cpp:15304-15307): RewardQuest(qInfo, 0, this, false). The quest is
// rewarded automatically with the choice-0 grant set — ObjectMgr only honors
// RewardChoiceItemId[0] for TRACKING quests (ObjectMgr.cpp:4727) — and
// announce=false, so no SMSG_QUESTGIVER_QUEST_COMPLETE goes out. The quest
// giver is the player itself: MailSender(Object*) (Mail.cpp:50-53) writes
// MAIL_NORMAL with the player GUID counter.
func (s *session) rewardTrackingQuest(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil {
		return
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		s.debug("quest tracking reward failed", "account", s.accountName, "quest", questID, "error", err)
		return
	}
	grants, destroyedGUIDs, err := s.commitQuestReward(ctx, view, 0)
	if err != nil {
		s.debug("quest tracking reward failed", "account", s.accountName, "quest", questID, "error", err)
		return
	}
	s.applyQuestRewardGrants(ctx, view, questID, s.playerGUID, 0, grants, destroyedGUIDs, false)
}

// applyQuestRewardGrants runs the post-commit grant arms of
// Player::RewardQuest (Player.cpp:15380-15515), shared by the turn-in packet
// and the .quest reward GM command (cs_quest.cpp:313:
// player->RewardQuest(quest, 0, player)). announce mirrors the RewardQuest
// announce parameter: false skips the SMSG_QUESTGIVER_QUEST_COMPLETE packet
// (Player::SendQuestReward, Player.cpp:15470) and the gossip close —
// Player::RewardQuest itself never touches the gossip frame.
func (s *session) applyQuestRewardGrants(ctx context.Context, view questRewardView, questID uint32, giverGUID uint64, reward uint32, grants []questItemGrant, destroyedGUIDs []uint64, announce bool) bool {
	// C++ Player::RewardQuest (Player.cpp:15439-15448): the quest reward mail
	// goes out as part of the reward flow, once the reward is committed.
	s.sendQuestRewardMail(ctx, questID, giverGUID)
	// Eluna CREATURE_EVENT_ON_QUEST_REWARD (event 34), fired from the
	// reward-completion switch arm after the grant (QuestHandler.cpp:330-335);
	// the reward-choice index rides as the opt argument.
	s.fireCreatureQuestHook(ctx, giverGUID, scripting.CreatureEventOnQuestReward, s.luaQuest(ctx, questID), reward)
	s.fireGameObjectQuestHook(ctx, giverGUID, scripting.GameObjectEventOnQuestReward, s.luaQuest(ctx, questID), reward)
	for slot := range s.player.QuestLog {
		if s.player.QuestLog[slot].QuestID == questID {
			s.player.QuestLog[slot] = questLogEntry{}
			s.sendPlayerQuestLogUpdate(slot)
			break
		}
	}
	for _, fullGUID := range destroyedGUIDs {
		s.sendDestroyObject(fullGUID, false)
		s.despawnItem(fullGUID)
	}
	for _, grant := range grants {
		if err := s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildItemPushResult(s.playerGUID, grant.Bag, grant.Slot, grant.Entry, grant.Count, grant.InventoryCount, grant.Stacked), true); err != nil {
			return false
		}
		// Player::AddItemDurations (Player.cpp:12251): freshly granted
		// duration items push SMSG_ITEM_TIME_UPDATE at store time.
		if !grant.Stacked && grant.Duration > 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_TIME_UPDATE), protocol.BuildItemTimeUpdate(uint64(grant.ItemGUID)|(uint64(0x4000)<<48), grant.Duration), true)
		}
	}
	// Player::RewardQuest (Player.cpp:15406-15411): the money arm runs the
	// signed RewOrReqMoney through ModifyMoney — required money subtracts
	// (clamped at zero on overdraw), positive rewards stop at
	// MAX_MONEY_AMOUNT, and at max level the quest pays the better of the
	// normal reward and the max-level bonus (Quest::GetRewOrReqMoney,
	// QuestDef.cpp:287-298).
	moneyRew := s.questRewOrReqMoney(view.Detail)
	newMoney, _ := applyMoneyDelta(s.player.Money, moneyRew)
	s.setPlayerMoney(ctx, newMoney)
	rewardXP := s.calculateQuestRewardXP(ctx, view.Detail.ID, view.Detail.RewardXPDifficulty)
	// Player::RewardQuest (Player.cpp:15403-15404):
	// SPELL_AURA_MOD_XP_QUEST_PCT (291) scales the quest XP after the rate.
	if rewardXP > 0 {
		rewardXP = uint32(float64(rewardXP) * float64(ResolveAuraPercentMultiplier(s.gatedAuraModifierAmounts(spellAuraModXPQuestPct, s.auraModifierCandidatesFiltered(spellAuraModXPQuestPct, nil)))))
	}
	if rewardXP > 0 {
		s.grantXP(ctx, rewardXP)
	}
	// Player::RewardQuest (Player.cpp:15409-15413): quest honor is granted at
	// the PLAYER level via RewardHonor(nullptr, 0, honor) (Player.cpp:6865).
	// The arena gate (victim-null arm returns false in an arena) and the
	// player-inactive aura gate apply; the victim-only aura multiplier arm is
	// moot with a null victim, and the world honor rate multiplies last
	// (Player.cpp:6993). The C++ only calls when the gain is non-zero.
	if honor := s.calculateQuestHonorGain(ctx, view.Detail.ID, uint32(s.player.Level)); honor > 0 && !s.hasAura(honorAuraInactiveID) && !s.inArenaNow() {
		s.updateHonorFields()
		s.grantHonor(ctx, int32(float64(honor)*s.honorRate()), 0, 0)
	}
	// Player::RewardQuest (Player.cpp:15414-15420): a CharTitleId reward earns
	// the title via SetTitle (Player.cpp:24814) — the CharTitles.dbc MaskID
	// bit in the known-titles field plus SMSG_TITLE_EARNED.
	if titleID := view.Detail.RewardTitleID; titleID > 0 && s.server.Data != nil {
		if title, found, terr := s.server.Data.CharTitle(titleID); terr == nil && found {
			if idx := title.MaskID / 32; int(idx) < len(s.player.KnownTitles) {
				if s.player.KnownTitles[idx]&(uint32(1)<<(title.MaskID%32)) == 0 {
					s.player.KnownTitles[idx] |= uint32(1) << (title.MaskID % 32)
					s.persistTitles(ctx, s)
					packet := protocol.NewBuffer(8)
					packet.WriteU32(title.MaskID)
					packet.WriteU32(1) // 1 - earned, 0 - lost
					_ = s.write(uint16(protocol.OpcodeSMSG_TITLE_EARNED), packet.Bytes(), true)
				}
			}
		}
	}
	// Player::RewardQuest (Player.cpp:15422-15425): bonus talents accumulate
	// in the quest-reward talent count (the DK Ebon-Hold arm at 18812 feeds
	// the same counter); Go's playerState.QuestBonus is that counter, and it
	// is recomputed from character_queststatus_rewarded at login.
	if talents := view.Detail.RewardTalents; talents > 0 {
		s.player.QuestBonus += talents
		s.sendPlayerUpdate()
	}
	// Player::RewardQuest (Player.cpp:15427-15428): RewArenaPoints goes
	// through ModifyArenaPoints (Player.cpp:7085): clamp at zero, persist.
	if arena := view.Detail.RewardArenaPoints; arena != 0 {
		s.modifyQuestArenaPoints(ctx, arena)
	}
	// Player::RewardQuest (Player.cpp:15500): reputation rewards run the
	// full Player::RewardReputation pipeline — the template values are
	// QuestFactionReward.dbc field indices, not flat reputation.
	s.rewardQuestReputation(ctx, view, questID)
	// Player::RewardQuest (Player.cpp:15479-15497): the quest's reward spell
	// is cast after the quest is marked complete (some spells gate on the
	// completed state via spell_area data). RewSpellCast wins over RewSpell.
	// The questgiver-creature cast arm is visual-only — the cast is
	// triggered — so Go casts as the player, which teaches
	// SPELL_EFFECT_LEARN_SPELL payloads through the normal cast pipeline.
	var rewSpellCast int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(RewSpellCast, 0) FROM quest_template WHERE ID = ?", questID).Scan(&rewSpellCast)
	}
	rewardSpellID := uint32(0)
	if rewSpellCast > 0 {
		rewardSpellID = uint32(rewSpellCast)
	} else if view.Detail.RewardSpell > 0 {
		rewardSpellID = uint32(view.Detail.RewardSpell)
	}
	if rewardSpellID > 0 {
		s.castSpellDirect(ctx, rewardSpellID, s.playerGUID)
	}
	// Player::SendQuestReward (Player.cpp:17013-17027): the packet zeroes XP at
	// max level; the honor field is 10 * CalculateHonorGain at the quest's
	// level (Player::GetQuestLevel: quest level when positive, else the
	// player's level).
	packetXP := rewardXP
	if s.server != nil && uint32(s.player.Level) >= s.server.Config.MaxPlayerLevel {
		packetXP = 0
	}
	var qLevel int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT QuestLevel FROM quest_template WHERE ID = ?", questID).Scan(&qLevel)
	}
	if qLevel <= 0 {
		qLevel = int64(s.player.Level)
	}
	packetHonor := 10 * s.calculateQuestHonorGain(ctx, questID, uint32(qLevel))
	if announce {
		if err := s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_COMPLETE), buildQuestRewardComplete(view.Detail.ID, packetXP, uint32(moneyRew), packetHonor, view.Detail.RewardTalents, uint32(view.Detail.RewardArenaPoints)), true); err != nil {
			return false
		}
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
	if giverGUID != 0 {
		entry, _ := questGiverMailSource(ctx, s, giverGUID)
		status := uint8(questDialogNone)
		if entry != 0 {
			if qStatus, err := s.questDialogStatus(ctx, entry); err == nil {
				status = qStatus
			}
		}
		packet := protocol.NewBuffer(9)
		packet.WriteU64(giverGUID)
		packet.WriteU8(status)
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS), packet.Bytes(), true)
	}
	// Player::RewardQuest (Player.cpp:15515): the turn-in re-broadcasts the
	// multiple status so nearby questgivers' "?" states refresh in-session
	// instead of waiting for a re-login, zone change, or manual query.
	s.sendQuestgiverStatusMultiple(ctx)
	// Player::RewardQuest (Player.cpp:15507-15510): turning in a PvP-flagged
	// quest recomputes the hostile state — the completed quest no longer
	// forces it — and pushes the new flags.
	if view.Detail.Flags&questFlagsPVP != 0 {
		if s.applyZoneState(ctx, s.player, s.player.Zone, s.areaID) {
			s.sendPlayerUpdate()
		}
	}
	if announce {
		s.gossip = nil
		s.gossipClosed = true
	}
	s.debug("quest rewarded", "account", s.accountName, "quest", questID, "reward", reward, "items", len(grants), "money", view.Detail.RewardMoney)
	return true
}

// questGiverMailSource resolves the quest giver's creature/gameobject entry
// and its mail message type, mirroring the MailSender(Object*) constructor
// (Mail.cpp:38-57): creatures send as MAIL_CREATURE (3) with the creature
// entry, gameobjects as MAIL_GAMEOBJECT (4) with the gameobject entry,
// stationery MAIL_STATIONERY_DEFAULT in both cases.
func questGiverMailSource(ctx context.Context, s *session, giverGUID uint64) (entry uint32, messageType uint32) {
	messageType = 3 // MAIL_CREATURE (Mail.h:39)
	if creature := s.luaCreature(ctx, giverGUID); creature != nil {
		entry = objectUint32OrZero(creature, "Entry")
	} else if goObj := s.luaGameObject(ctx, giverGUID); goObj != nil {
		entry = objectUint32OrZero(goObj, "Entry")
		messageType = 4 // MAIL_GAMEOBJECT (Mail.h:40)
	}
	if entry == 0 {
		entry = uint32((giverGUID >> 24) & 0x00FFFFFF)
	}
	if entry == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rawGUID := giverGUID & 0x00000000FFFFFFFF
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", rawGUID).Scan(&entry)
	}
	return entry, messageType
}

// sendQuestRewardMail mirrors the quest reward mail arm of Player::RewardQuest
// (Player.cpp:15439-15448): when quest_template_addon.RewardMailTemplateID is
// set, the player receives a template mail once the reward commits. The sender
// is quest_mail_sender.RewardMailSenderEntry (MailSender(uint32) ->
// MAIL_CREATURE) or, when unset, the quest giver. ObjectMgr.cpp:5148-5168
// zeroes the template id when the template is missing from MailTemplate.dbc
// or is shared by more than one quest, so no mail goes out in either case.
// MailDraft::SendMailTo (Mail.cpp:187-215) anchors deliver_time on now +
// RewMailDelaySecs; quest template mails never carry COD and the sender is a
// creature entry (never a GM player), so the 30-day expire arm applies. The
// subject/body are stored empty — the client renders them from MailTemplate.dbc
// by mailTemplateId — and checked is MAIL_CHECK_MASK_HAS_BODY (0x10).
func (s *session) sendQuestRewardMail(ctx context.Context, questID uint32, giverGUID uint64) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	cdb := s.server.CharactersStore.DB
	var templateID, delaySecs uint32
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(RewardMailTemplateID, 0), COALESCE(RewardMailDelay, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&templateID, &delaySecs); err != nil || templateID == 0 {
		return
	}
	var useCount int
	if err := wdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM quest_template_addon WHERE RewardMailTemplateID = ?", templateID).Scan(&useCount); err != nil || useCount != 1 {
		return
	}
	if _, ok, err := s.server.Data.MailTemplate(templateID); err != nil || !ok {
		return
	}
	var senderEntry uint32
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(RewardMailSenderEntry, 0) FROM quest_mail_sender WHERE QuestId = ?", questID).Scan(&senderEntry)
	messageType := uint32(3) // MAIL_CREATURE (Mail.h:39)
	if senderEntry == 0 {
		senderEntry, messageType = questGiverMailSource(ctx, s, giverGUID)
	}
	// Player::CompleteQuest's TRACKING arm calls RewardQuest(qInfo, 0, this,
	// false): the quest giver is the player itself, so MailSender(Object*)
	// (Mail.cpp:50-53) writes MAIL_NORMAL with the player GUID counter,
	// not a creature sender.
	if giverGUID != 0 && s != nil && giverGUID == s.playerGUID {
		senderEntry, messageType = uint32(s.playerGUID), 0 // MAIL_NORMAL (Mail.h:36)
	}
	now := time.Now().Unix()
	deliverTime := now + int64(delaySecs)
	expireTime := deliverTime + mailSendExpireDelay(false, 0)
	var money uint32
	// C++ MailDraft::prepareItems (Mail.cpp:102-104): the mail sent after
	// turning in the quest "The Good News and The Bad News" contains 100g.
	if templateID == 123 {
		money = 1000000
	}
	// MailDraft::prepareItems (Mail.cpp:106-121): roll the template's
	// mail_loot_template items for the receiver.
	itemGUIDs := s.server.rollMailTemplateItems(ctx, templateID, s.playerGUID)
	hasItems := 0
	if len(itemGUIDs) > 0 {
		hasItems = 1
	}
	nextMailID := s.server.generateMailID()
	_, _ = cdb.ExecContext(ctx, `INSERT INTO mail (id, messageType, stationery, mailTemplateId, sender, receiver, subject, body, has_items, expire_time, deliver_time, money, cod, checked)
		VALUES (?, ?, 41, ?, ?, ?, '', '', ?, ?, ?, ?, 0, 16)`,
		nextMailID, messageType, templateID, senderEntry, s.playerGUID, hasItems, expireTime, deliverTime, money)
	for _, ig := range itemGUIDs {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO mail_items (mail_id, item_guid, receiver) VALUES (?, ?, ?)", nextMailID, ig, s.playerGUID)
	}
	s.sendMailNotify(s.playerGUID)
}

func (s *session) commitQuestReward(ctx context.Context, view questRewardView, choice uint32) ([]questItemGrant, []uint64, error) {
	s.server.inventoryMu.Lock()
	defer s.server.inventoryMu.Unlock()
	questState, err := s.loadQuestRewardPersistenceState(ctx, view.Detail.ID)
	if err != nil {
		return nil, nil, err
	}

	fixed := append([]questRewardItem(nil), view.Detail.RewardItems...)
	if len(view.Detail.ChoiceItems) > 0 && choice < uint32(len(view.Detail.ChoiceItems)) {
		fixed = append(fixed, view.Detail.ChoiceItems[choice])
	}
	equippedBags := s.getEquippedBags(ctx, s.playerGUID)
	stackables := make(map[uint32]int64)
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		for _, it := range fixed {
			if it.ID > 0 {
				var st int64
				_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT stackable FROM item_template WHERE entry = ?", it.ID).Scan(&st)
				if st < 1 {
					st = 1
				}
				stackables[it.ID] = st
			}
		}
	}

	tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	rollback := func(cause error) ([]questItemGrant, []uint64, error) {
		_ = tx.Rollback()
		return nil, nil, cause
	}
	if err := rejectClaimedQuestReward(ctx, tx, s.playerGUID, view.Detail.ID, questState); err != nil {
		return rollback(err)
	}
	inventory, err := loadInventoryRewardRecords(ctx, tx, s.playerGUID)
	if err != nil {
		return rollback(err)
	}
	destroyedGUIDs, err := s.destroyQuestTurnInItems(ctx, tx, inventory, view, view.Detail.ID)
	if err != nil {
		return rollback(err)
	}
	grants, err := s.storeQuestRewardItems(ctx, tx, inventory, fixed, equippedBags, stackables)
	if err != nil {
		return rollback(err)
	}
	// The money arm commits atomically with the reward; the absolute write
	// mirrors SetMoney so required-money subtraction behaves identically on
	// signed and unsigned money columns (the old relative write wrapped the
	// negative amount through uint32, inflating SQLite money).
	moneyRew := s.questRewOrReqMoney(view.Detail)
	if moneyRew != 0 {
		newMoney, _ := applyMoneyDelta(s.player.Money, moneyRew)
		if _, err := tx.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ? AND account = ?", newMoney, s.playerGUID, s.accountID); err != nil {
			return rollback(err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_queststatus WHERE guid = ? AND quest = ?", s.playerGUID, view.Detail.ID); err != nil {
		return rollback(err)
	}
	insertIgnore := "INSERT OR IGNORE"
	if s.server.CharactersStore.Backend != database.BackendSQLite {
		insertIgnore = "INSERT IGNORE"
	}
	if questState.DailyStatus {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_daily (guid, quest, time) VALUES (?, ?, ?)", s.playerGUID, view.Detail.ID, time.Now().Unix()); err != nil {
			return rollback(err)
		}
	} else if questState.Weekly {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_weekly (guid, quest) VALUES (?, ?)", s.playerGUID, view.Detail.ID); err != nil {
			return rollback(err)
		}
	} else if questState.Monthly {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_monthly (guid, quest) VALUES (?, ?)", s.playerGUID, view.Detail.ID); err != nil {
			return rollback(err)
		}
	} else if questState.Seasonal {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_seasonal (guid, quest, event) VALUES (?, ?, ?)", s.playerGUID, view.Detail.ID, questState.EventID); err != nil {
			return rollback(err)
		}
	}
	if questState.Rewarded {
		if _, err := tx.ExecContext(ctx, insertIgnore+" INTO character_queststatus_rewarded (guid, quest, active) VALUES (?, ?, 1)", s.playerGUID, view.Detail.ID); err != nil {
			return rollback(err)
		}
	}
	questID := view.Detail.ID
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	for _, item := range view.RequiredItems {
		if item.ID != 0 && item.Quantity != 0 {
			s.adjustQuestItemCount(ctx, item.ID, item.Quantity, false)
		}
	}
	for _, item := range fixed {
		if item.ID != 0 && item.Quantity != 0 {
			s.adjustQuestItemCount(ctx, item.ID, item.Quantity, true)
		}
	}
	s.applyQuestRewardPersistenceState(questID, questState)
	s.updateAchievementCriteria(criteriaTypeCompleteQuest, questID, 1)
	s.updateAchievementCriteria(criteriaTypeQuestCount, 0, 1)
	if moneyRew > 0 {
		s.updateAchievementCriteria(criteriaTypeMoneyFromQuest, 0, uint32(moneyRew))
	}
	if questState.SortID > 0 {
		s.updateAchievementCriteria(criteriaTypeCompleteQuestsInZone, uint32(questState.SortID), 1)
	}
	if questState.DailyFlag {
		s.updateAchievementCriteria(criteriaTypeCompleteDailyQuest, 0, 1)
		s.updateAchievementCriteria(criteriaTypeCompleteDailyQuestDaily, 0, 1)
	}
	return grants, destroyedGUIDs, nil
}

func rejectClaimedQuestReward(ctx context.Context, tx *sql.Tx, guid uint64, questID uint32, state questRewardPersistenceState) error {
	queries := []struct {
		enabled bool
		query   string
		args    []any
	}{
		{state.Rewarded, "SELECT 1 FROM character_queststatus_rewarded WHERE guid = ? AND quest = ? AND active = 1 LIMIT 1", []any{guid, questID}},
		{state.DailyStatus, "SELECT 1 FROM character_queststatus_daily WHERE guid = ? AND quest = ? LIMIT 1", []any{guid, questID}},
		{state.Weekly, "SELECT 1 FROM character_queststatus_weekly WHERE guid = ? AND quest = ? LIMIT 1", []any{guid, questID}},
		{state.Monthly, "SELECT 1 FROM character_queststatus_monthly WHERE guid = ? AND quest = ? LIMIT 1", []any{guid, questID}},
		{state.Seasonal, "SELECT 1 FROM character_queststatus_seasonal WHERE guid = ? AND quest = ? AND event = ? LIMIT 1", []any{guid, questID, state.EventID}},
	}
	for _, check := range queries {
		if !check.enabled {
			continue
		}
		var found int
		err := tx.QueryRowContext(ctx, check.query, check.args...).Scan(&found)
		if err == nil {
			return errQuestAlreadyRewarded
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

func (s *session) loadQuestRewardPersistenceState(ctx context.Context, questID uint32) (questRewardPersistenceState, error) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return questRewardPersistenceState{}, errors.New("world database unavailable for quest reward state")
	}
	var flags uint32
	var sortID int32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Flags, QuestSortID FROM quest_template WHERE ID = ?", questID).Scan(&flags, &sortID); err != nil {
		return questRewardPersistenceState{}, err
	}
	var specialFlags uint32
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	if err != nil && err != sql.ErrNoRows {
		return questRewardPersistenceState{}, err
	}
	specialFlags &= questSpecialAllowed
	if flags&(questFlagsDaily|questFlagsWeekly) != 0 {
		specialFlags |= questSpecialRepeatable
	}
	state := questRewardPersistenceState{
		DailyFlag:     flags&questFlagsDaily != 0,
		DailyStatus:   flags&questFlagsDaily != 0 || specialFlags&questSpecialDungeonFind != 0,
		DungeonFinder: specialFlags&questSpecialDungeonFind != 0,
		Weekly:        flags&questFlagsWeekly != 0,
		Monthly:       specialFlags&questSpecialMonthly != 0,
		SortID:        sortID,
	}
	state.Seasonal = isSeasonalQuestSort(sortID) && specialFlags&questSpecialRepeatable == 0
	state.Rewarded = !state.DungeonFinder && !state.DailyFlag && (specialFlags&questSpecialRepeatable == 0 || state.Weekly || state.Monthly || state.Seasonal)
	if state.DailyStatus || state.Weekly {
		state.Monthly = false
		state.Seasonal = false
	} else if state.Monthly {
		state.Seasonal = false
	}
	if state.Seasonal {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT eventEntry FROM game_event_seasonal_questrelation WHERE questId = ?", questID)
		if err != nil {
			return questRewardPersistenceState{}, err
		}
		for rows.Next() {
			var eventID int64
			if err := rows.Scan(&eventID); err != nil {
				_ = rows.Close()
				return questRewardPersistenceState{}, err
			}
			if eventID >= 0 {
				state.EventID = uint32(uint16(eventID))
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return questRewardPersistenceState{}, err
		}
		if err := rows.Close(); err != nil {
			return questRewardPersistenceState{}, err
		}
	}
	return state, nil
}

func isSeasonalQuestSort(sortID int32) bool {
	switch sortID {
	case -22, -284, -366, -369, -370, -374, -376:
		return true
	default:
		return false
	}
}

func (s *session) applyQuestRewardPersistenceState(questID uint32, state questRewardPersistenceState) {
	if s.player == nil {
		return
	}
	if state.DailyStatus {
		if dailyTime := time.Now().Unix(); dailyTime > s.player.LastDailyQuestTime {
			s.player.LastDailyQuestTime = dailyTime
		}
		if state.DungeonFinder {
			if s.player.DungeonFinderQuests == nil {
				s.player.DungeonFinderQuests = make(map[uint32]struct{})
			}
			s.player.DungeonFinderQuests[questID] = struct{}{}
		} else {
			for index := range s.player.DailyQuests {
				if s.player.DailyQuests[index] == 0 {
					s.player.DailyQuests[index] = questID
					break
				}
			}
		}
		return
	}
	if state.Weekly {
		if s.player.WeeklyQuests == nil {
			s.player.WeeklyQuests = make(map[uint32]struct{})
		}
		s.player.WeeklyQuests[questID] = struct{}{}
	} else if state.Monthly {
		if s.player.MonthlyQuests == nil {
			s.player.MonthlyQuests = make(map[uint32]struct{})
		}
		s.player.MonthlyQuests[questID] = struct{}{}
	} else if state.Seasonal {
		if s.player.SeasonalQuests == nil {
			s.player.SeasonalQuests = make(map[uint32]map[uint32]struct{})
		}
		if s.player.SeasonalQuests[state.EventID] == nil {
			s.player.SeasonalQuests[state.EventID] = make(map[uint32]struct{})
		}
		s.player.SeasonalQuests[state.EventID][questID] = struct{}{}
	}
}

func loadInventoryRewardRecords(ctx context.Context, tx *sql.Tx, guid uint64) ([]inventoryRewardRecord, error) {
	rows, err := tx.QueryContext(ctx, "SELECT ci.bag, ci.slot, ci.item, ii.itemEntry, ii.count FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? ORDER BY ci.bag, ci.slot", guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]inventoryRewardRecord, 0)
	for rows.Next() {
		var bag, slot, itemGUID, entry, count int64
		if err := rows.Scan(&bag, &slot, &itemGUID, &entry, &count); err != nil {
			return nil, err
		}
		if bag < 0 || slot < 0 || slot > 255 || itemGUID < 0 || itemGUID > int64(^uint32(0)) || entry < 0 || entry > int64(^uint32(0)) || count <= 0 {
			continue
		}
		if count > int64(^uint32(0)) {
			count = int64(^uint32(0))
		}
		result = append(result, inventoryRewardRecord{Bag: bag, Slot: uint8(slot), ItemGUID: uint32(itemGUID), Entry: uint32(entry), Count: uint32(count)})
	}
	return result, rows.Err()
}

type rewardSlotLocation struct {
	BagKey    int64
	ClientBag uint8
	Slot      uint8
}

func (s *session) storeQuestRewardItems(ctx context.Context, tx *sql.Tx, inventory []inventoryRewardRecord, items []questRewardItem, equippedBags []equippedBagInfo, stackables map[uint32]int64) ([]questItemGrant, error) {
	grants := make([]questItemGrant, 0, len(items))
	for _, item := range items {
		if item.ID == 0 || item.Quantity == 0 {
			continue
		}
		stackable := stackables[item.ID]
		if stackable < 1 {
			stackable = 1
		}
		remaining := uint64(item.Quantity)
		// Item::Create (Item.cpp:291) seeds ITEM_FIELD_DURATION from the
		// template at once on every freshly created quest-reward stack.
		rewardDuration := s.server.itemTemplateDuration(ctx, item.ID)
		for index := range inventory {
			record := &inventory[index]
			if record.Entry != item.ID || record.Count >= uint32(stackable) || remaining == 0 {
				continue
			}
			space := uint64(uint32(stackable) - record.Count)
			add := remaining
			if add > space {
				add = space
			}
			record.Count += uint32(add)
			remaining -= add
			if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", record.Count, record.ItemGUID); err != nil {
				return nil, err
			}
			clientBag := resolveRewardClientBag(record.Bag, equippedBags)
			grants = append(grants, questItemGrant{
				Entry:          item.ID,
				Count:          uint32(add),
				Bag:            clientBag,
				Slot:           uint32(record.Slot),
				InventoryCount: record.Count,
				Stacked:        true,
			})
		}
		for remaining != 0 {
			loc, ok := findNextRewardSlot(inventory, equippedBags)
			if !ok {
				return nil, errQuestInventoryFull
			}
			add := remaining
			if add > uint64(stackable) {
				add = uint64(stackable)
			}
			nextGUID := s.server.generateItemGUID()
			if nextGUID == 0 {
				return nil, errors.New("item guid space exhausted")
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, giftCreatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, ?, ?, 0, 0, ?, ?, '', 0, '', 0, 0, 0, NULL)", nextGUID, item.ID, s.playerGUID, add, rewardDuration); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)", s.playerGUID, loc.BagKey, loc.Slot, nextGUID); err != nil {
				return nil, err
			}
			inventory = append(inventory, inventoryRewardRecord{Bag: loc.BagKey, Slot: loc.Slot, ItemGUID: nextGUID, Entry: item.ID, Count: uint32(add)})
			remaining -= add
			grants = append(grants, questItemGrant{
				ItemGUID:       nextGUID,
				Entry:          item.ID,
				Count:          uint32(add),
				Bag:            loc.ClientBag,
				Slot:           uint32(loc.Slot),
				InventoryCount: uint32(add),
				Stacked:        false,
				Duration:       uint32(rewardDuration),
			})
		}
	}
	return grants, nil
}

func findNextRewardSlot(inventory []inventoryRewardRecord, equippedBags []equippedBagInfo) (rewardSlotLocation, bool) {
	occupiedBackpack := make(map[uint8]struct{})
	for _, record := range inventory {
		if record.Bag == 0 && record.Slot >= inventoryRewardFirst && record.Slot <= inventoryRewardLast && record.Count != 0 {
			occupiedBackpack[record.Slot] = struct{}{}
		}
	}
	for slot := inventoryRewardFirst; slot <= inventoryRewardLast; slot++ {
		if _, ok := occupiedBackpack[uint8(slot)]; !ok {
			return rewardSlotLocation{BagKey: 0, ClientBag: 0, Slot: uint8(slot)}, true
		}
	}

	for _, bag := range equippedBags {
		if bag.slots == 0 {
			continue
		}
		occupiedBag := make(map[uint8]struct{})
		for _, record := range inventory {
			if record.Bag == bag.guid && record.Count != 0 {
				occupiedBag[record.Slot] = struct{}{}
			}
		}
		for slot := uint8(0); slot < uint8(bag.slots); slot++ {
			if _, ok := occupiedBag[slot]; !ok {
				return rewardSlotLocation{BagKey: bag.guid, ClientBag: bag.slot, Slot: slot}, true
			}
		}
	}

	return rewardSlotLocation{}, false
}

func resolveRewardClientBag(bagKey int64, equippedBags []equippedBagInfo) uint8 {
	if bagKey == 0 {
		return 0
	}
	for _, bag := range equippedBags {
		if bag.guid == bagKey {
			return bag.slot
		}
	}
	return 0
}

func buildItemPushResult(playerGUID uint64, bag uint8, slot, entry, count, inventoryCount uint32, stacked bool) []byte {
	packet := protocol.NewBuffer(48)
	packet.WriteU64(playerGUID)
	packet.WriteU32(1)
	packet.WriteU32(0)
	packet.WriteU32(0)
	packet.WriteU8(bag)
	if stacked {
		packet.WriteU32(^uint32(0))
	} else {
		packet.WriteU32(slot)
	}
	packet.WriteU32(entry)
	packet.WriteU32(0)
	packet.WriteI32(0)
	packet.WriteU32(count)
	packet.WriteU32(inventoryCount)
	return packet.Bytes()
}

// calculateQuestHonorGain mirrors Quest::CalculateHonorGain
// (QuestDef.cpp:348-367): level clamps at GT_MAX_LEVEL (100); the
// TeamContributionPoints.dbc Data row scales the kill-honor multiplier by
// 0.1 and the flat addition is summed on. A missing DBC row or zero
// addition/multiplier yields zero, exactly like the C++ early-outs.
func (s *session) calculateQuestHonorGain(ctx context.Context, questID, level uint32) uint32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var addition int64
	var multiplier float64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(RewardHonor, 0), COALESCE(RewardKillHonor, 0) FROM quest_template WHERE ID = ?", questID).Scan(&addition, &multiplier)
	}
	if addition <= 0 && multiplier <= 0 {
		return 0
	}
	if level > 100 {
		level = 100
	}
	data, ok, err := s.server.Data.TeamContributionPoints(level)
	if err != nil || !ok {
		return 0
	}
	return uint32(data*float32(multiplier)*0.1) + uint32(addition)
}

// inArenaNow mirrors the InArena() gate of Player::RewardHonor
// (Player.cpp:6865): Go keeps no live-instance model, so an arena queue
// entry past wait-queue (BGStatusInProgress) is the established proxy —
// the same proxy the arena-team leave arm uses.
func (s *session) inArenaNow() bool {
	if s == nil {
		return false
	}
	for i := range s.bgQueues {
		e := &s.bgQueues[i]
		if e.Active && e.IsArena && e.Status == BGStatusInProgress {
			return true
		}
	}
	return false
}

// modifyQuestArenaPoints mirrors Player::ModifyArenaPoints (Player.cpp:7085):
// the total clamps at zero and persists to characters.arenaPoints. The
// player-account gate (AccountMgr::IsPlayerAccount, security SEC_PLAYER)
// matches the C++ early return.
func (s *session) modifyQuestArenaPoints(ctx context.Context, value int32) {
	if s.player == nil || s.security != 0 {
		return
	}
	newValue := int64(s.player.ArenaPoints) + int64(value)
	if newValue < 0 {
		newValue = 0
	}
	s.player.ArenaPoints = uint32(newValue)
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET arenaPoints = ? WHERE guid = ?", newValue, s.playerGUID)
	}
	if s.player.ArenaPoints != 0 {
		s.addKnownCurrency(s.player, itemArenaPointsID)
	}
	s.sendPlayerUpdate()
}

func buildQuestRewardComplete(questID, xp, money, honor, talents, arena uint32) []byte {
	packet := protocol.NewBuffer(24)
	packet.WriteU32(questID)
	packet.WriteU32(xp)
	packet.WriteU32(money)
	packet.WriteU32(honor)
	packet.WriteU32(talents)
	packet.WriteU32(arena)
	return packet.Bytes()
}

func buildQuestFailed(questID, reason uint32) []byte {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(questID)
	packet.WriteU32(reason)
	return packet.Bytes()
}

func (s *session) calculateQuestRewardXP(ctx context.Context, questID, xpDiff uint32) uint32 {
	if xpDiff >= 9 {
		return 0
	}
	var qLevel int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT QuestLevel FROM quest_template WHERE entry = ?", questID).Scan(&qLevel)
	}
	if qLevel <= 0 {
		if s.player != nil && s.player.Level > 0 {
			qLevel = int64(s.player.Level)
		} else {
			qLevel = 1
		}
	}
	if qLevel < 1 {
		qLevel = 1
	} else if qLevel > 100 {
		qLevel = 100
	}
	entry, ok := questXPDBC[uint32(qLevel)]
	if !ok {
		return 0
	}
	xp := entry[xpDiff]
	if s.server != nil {
		xp = uint32(float64(xp) * s.server.Config.XPRateQuest)
	}
	return xp
}

var questXPDBC = map[uint32][9]uint32{
	1:  {0, 10, 20, 40, 60, 80, 100, 120, 160},
	2:  {0, 15, 45, 85, 130, 170, 215, 255, 340},
	3:  {0, 25, 65, 125, 190, 250, 315, 380, 500},
	4:  {0, 35, 90, 180, 270, 355, 445, 540, 710},
	5:  {0, 45, 110, 225, 335, 450, 560, 670, 900},
	6:  {0, 55, 135, 270, 405, 540, 680, 810, 1080},
	7:  {0, 65, 160, 315, 475, 630, 790, 950, 1260},
	8:  {0, 70, 175, 350, 530, 700, 880, 1050, 1400},
	9:  {0, 80, 195, 390, 590, 780, 980, 1150, 1560},
	10: {0, 85, 210, 420, 630, 840, 1050, 1250, 1680},
	11: {0, 90, 220, 440, 660, 880, 1100, 1300, 1760},
	12: {0, 90, 225, 455, 680, 910, 1150, 1350, 1820},
	13: {0, 90, 230, 455, 680, 910, 1150, 1350, 1820},
	14: {0, 100, 245, 490, 740, 980, 1250, 1450, 1960},
	15: {0, 105, 270, 540, 800, 1050, 1350, 1600, 2100},
	16: {0, 115, 290, 580, 880, 1150, 1450, 1750, 2300},
	17: {0, 125, 315, 630, 950, 1250, 1600, 1900, 2500},
	18: {0, 135, 340, 680, 1000, 1350, 1700, 2050, 2700},
	19: {0, 145, 365, 730, 1100, 1450, 1800, 2200, 2900},
	20: {0, 155, 390, 780, 1150, 1550, 1950, 2350, 3100},
	21: {0, 165, 415, 830, 1250, 1650, 2050, 2500, 3300},
	22: {0, 175, 435, 870, 1300, 1750, 2200, 2600, 3500},
	23: {0, 185, 460, 920, 1400, 1850, 2300, 2750, 3700},
	24: {0, 195, 485, 970, 1450, 1950, 2400, 2900, 3900},
	25: {0, 200, 510, 1000, 1500, 2000, 2550, 3050, 4000},
	26: {0, 210, 530, 1050, 1600, 2100, 2650, 3150, 4200},
	27: {0, 220, 550, 1100, 1650, 2200, 2750, 3300, 4400},
	28: {0, 230, 570, 1150, 1700, 2300, 2850, 3400, 4600},
	29: {0, 235, 590, 1200, 1750, 2350, 2950, 3550, 4700},
	30: {0, 245, 610, 1200, 1850, 2450, 3050, 3650, 4900},
	31: {0, 250, 630, 1250, 1900, 2540, 3150, 3800, 5080},
	32: {0, 270, 670, 1350, 2000, 2710, 3350, 4050, 5420},
	33: {0, 290, 720, 1450, 2150, 2900, 3600, 4350, 5800},
	34: {0, 310, 770, 1550, 2300, 3100, 3850, 4650, 6200},
	35: {0, 330, 820, 1650, 2450, 3300, 4100, 4950, 6600},
	36: {0, 350, 870, 1750, 2600, 3500, 4350, 5250, 7000},
	37: {0, 370, 920, 1850, 2750, 3710, 4600, 5550, 7420},
	38: {0, 390, 980, 1950, 2900, 3920, 4900, 5850, 7840},
	39: {0, 410, 1030, 2050, 3100, 4140, 5150, 6200, 8280},
	40: {0, 435, 1090, 2150, 3250, 4370, 5450, 6550, 8740},
	41: {0, 455, 1140, 2250, 3400, 4590, 5700, 6850, 9180},
	42: {0, 480, 1200, 2400, 3600, 4820, 6000, 7200, 9640},
	43: {0, 505, 1260, 2500, 3750, 5050, 6300, 7550, 10100},
	44: {0, 525, 1320, 2600, 3950, 5290, 6600, 7900, 10580},
	45: {0, 550, 1380, 2750, 4150, 5540, 6900, 8300, 11080},
	46: {0, 575, 1440, 2850, 4300, 5790, 7200, 8650, 11580},
	47: {0, 600, 1510, 3000, 4500, 6040, 7550, 9050, 12080},
	48: {0, 625, 1570, 3100, 4700, 6290, 7850, 9400, 12580},
	49: {0, 655, 1630, 3250, 4900, 6550, 8150, 9800, 13100},
	50: {0, 680, 1700, 3400, 5100, 6810, 8500, 10200, 13620},
	51: {0, 705, 1760, 3500, 5300, 7070, 8800, 10600, 14140},
	52: {0, 730, 1830, 3650, 5500, 7340, 9150, 11000, 14680},
	53: {0, 760, 1900, 3800, 5700, 7610, 9500, 11400, 15220},
	54: {0, 785, 1970, 3900, 5900, 7890, 9850, 11800, 15780},
	55: {0, 815, 2040, 4050, 6100, 8170, 10200, 12250, 16340},
	56: {0, 845, 2110, 4200, 6300, 8450, 10550, 12650, 16900},
	57: {0, 870, 2180, 4350, 6500, 8730, 10900, 13050, 17460},
	58: {0, 900, 2250, 4500, 6750, 9020, 11250, 13500, 18040},
	59: {0, 930, 2320, 4650, 6950, 9310, 11600, 13950, 18620},
	60: {0, 955, 2380, 4750, 7150, 9550, 11900, 14300, 19100},
	61: {0, 970, 2400, 4850, 7300, 9800, 12200, 14700, 19600},
	62: {0, 1000, 2550, 5000, 7600, 10050, 12600, 15050, 20100},
	63: {0, 1050, 2600, 5250, 7800, 10400, 12950, 15550, 20800},
	64: {0, 1080, 2650, 5400, 8050, 10750, 13350, 16000, 21500},
	65: {0, 1100, 2700, 5500, 8250, 11000, 13750, 16500, 22000},
	66: {0, 1130, 2850, 5650, 8550, 11300, 14150, 17000, 22600},
	67: {0, 1160, 2900, 5800, 8750, 11650, 14600, 17450, 23300},
	68: {0, 1200, 3050, 6000, 9000, 12000, 14950, 17950, 24000},
	69: {0, 1230, 3100, 6150, 9250, 12300, 15400, 18450, 24600},
	70: {0, 1250, 3150, 6250, 9500, 12650, 15800, 19000, 25300},
	71: {0, 2000, 5050, 10050, 15100, 20100, 25150, 30150, 40200},
	72: {0, 2050, 5100, 10150, 15250, 20300, 25400, 30450, 40600},
	73: {0, 2050, 5150, 10250, 15400, 20500, 25650, 30750, 41000},
	74: {0, 2100, 5200, 10400, 15550, 20750, 25950, 31150, 41500},
	75: {0, 2100, 5250, 10500, 15700, 20950, 26200, 31450, 41900},
	76: {0, 2100, 5300, 10600, 15850, 21150, 26450, 31750, 42300},
	77: {0, 2150, 5350, 10700, 16050, 21400, 26750, 32100, 42800},
	78: {0, 2150, 5400, 10800, 16200, 21600, 27000, 32400, 43200},
	79: {0, 2200, 5450, 10900, 16350, 21800, 27250, 32700, 43600},
	80: {0, 2200, 5500, 11050, 16550, 22050, 27550, 33100, 44100},
}

// questRewOrReqMoney mirrors Quest::GetRewOrReqMoney (QuestDef.cpp:287-298).
// The template RewardMoney rides the view as uint32, wrapping the signed DB
// value; int32() recovers it. Negative money is required money (returned
// as-is); positive rewards scale by RATE_MONEY_QUEST, and at max level the
// quest pays the better of the normal reward and the max-level bonus
// (GetRewMoneyMaxLevel), which is zero under QUEST_FLAGS_NO_MONEY_FROM_XP.
// Go exposes no money-rate config; the C++ defaults are 1.0, so the rate
// arms are identity here.
func (s *session) questRewOrReqMoney(detail questDetailData) int32 {
	raw := int32(detail.RewardMoney)
	if raw < 0 {
		return raw
	}
	if s.player == nil || s.server == nil || uint32(s.player.Level) < s.server.Config.MaxPlayerLevel {
		return raw
	}
	bonus := int32(0)
	if detail.Flags&questFlagsNoMoneyFromXP == 0 {
		bonus = int32(detail.RewardBonusMoney)
	}
	if bonus > raw {
		return bonus
	}
	return raw
}

// applyMoneyDelta mirrors Player::ModifyMoney (Player.cpp:22821-22845) as a
// pure function: a zero amount is a no-op, negative amounts clamp at zero on
// overdraw (no wrap), and positive amounts past MAX_MONEY_AMOUNT are
// rejected (ok=false, value unchanged — the quest path ignores the failure
// exactly like RewardQuest ignores ModifyMoney's return).
func applyMoneyDelta(current uint32, amount int32) (uint32, bool) {
	if amount == 0 {
		return current, true
	}
	if amount < 0 {
		if need := uint32(-int64(amount)); current > need {
			return current - need, true
		}
		return 0, true
	}
	if current < maxMoneyAmount-uint32(amount) {
		return current + uint32(amount), true
	}
	return current, false
}

// setPlayerMoney persists an absolute money value, mirroring the
// SetMoney/MoneyChanged leg of Player::ModifyMoney (Player.cpp:22847).
func (s *session) setPlayerMoney(ctx context.Context, value uint32) {
	if s.player == nil {
		return
	}
	s.player.Money = value
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", value, s.playerGUID)
	}
	s.sendPlayerUpdate()
	// Player::SetMoney (Player.cpp:22847-22851): every money change runs
	// the quest MoneyChanged scan.
	s.questMoneyChanged(ctx)
}

// questMoneyChanged mirrors Player::MoneyChanged (Player.cpp:16860-16882),
// the quest leg of every money change: the quest log is scanned for
// required-money quests (RewOrReqMoney < 0). An incomplete quest whose
// requirement is now met completes (gated by CanCompleteQuest); a complete
// quest whose requirement broke flips back via IncompleteQuest.
func (s *session) questMoneyChanged(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	wdb := s.server.WorldStore.DB
	money := int64(s.player.Money)
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		questID := s.player.QuestLog[slot].QuestID
		if questID == 0 {
			continue
		}
		var reqMoney int64
		if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(RewOrReqMoney, 0) FROM quest_template WHERE ID = ?", questID).Scan(&reqMoney); err != nil || reqMoney >= 0 {
			continue
		}
		required := -reqMoney
		status, err := s.characterQuestStatus(ctx, questID)
		if err != nil {
			continue
		}
		switch status {
		case questStatusIncomplete:
			if money >= required && s.canCompleteQuest(ctx, questID) {
				s.completeQuest(ctx, questID)
			}
		case questStatusComplete:
			if money < required {
				s.incompleteQuest(ctx, questID)
			}
		}
	}
}

// destroyQuestTurnInItems mirrors the item-destruction arms at the head of
// Player::RewardQuest (Player.cpp:15338-15359): required objective items and
// source (ItemDrop) items leave the player's bags. A quest-bound item
// (bonding BIND_QUEST_ITEM) on a non-repeatable quest is wiped entirely when
// no other quest still needs it — checked with the turn-in variant of
// HasQuestForItem for required items (:15341) and the plain variant for
// source items (:15352); otherwise only the required count is destroyed.
// Items without an item_template row are skipped, like the C++ null-template
// arm.
func (s *session) destroyQuestTurnInItems(ctx context.Context, tx *sql.Tx, inventory []inventoryRewardRecord, view questRewardView, questID uint32) ([]uint64, error) {
	type destroySpec struct {
		entry      uint32
		count      uint32
		destroyAll bool
		strict     bool
	}
	specs := make([]destroySpec, 0, len(view.RequiredItems)+len(view.SourceItems))
	repeatable := s.questIsRepeatable(ctx, questID)
	plan := func(items []questRewardItem, turnIn bool, strict bool) {
		for _, item := range items {
			if item.ID == 0 {
				continue
			}
			spec := destroySpec{entry: item.ID, count: item.Quantity, strict: strict}
			var bonding int64 = -1
			if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
				_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT bonding FROM item_template WHERE entry = ?", item.ID).Scan(&bonding)
			}
			if bonding < 0 {
				continue
			}
			if item.Quantity > 0 && bonding == questBindQuestItem && !repeatable && !s.hasQuestForItemEx(ctx, item.ID, questID, turnIn) {
				spec.destroyAll = true
			}
			specs = append(specs, spec)
		}
	}
	plan(view.RequiredItems, true, true)
	plan(view.SourceItems, false, false)
	var destroyed []uint64
	for _, spec := range specs {
		remaining := uint64(spec.count)
		if spec.destroyAll {
			remaining = ^uint64(0)
		}
		for index := range inventory {
			record := &inventory[index]
			if record.Entry != spec.entry || record.Count == 0 || remaining == 0 {
				continue
			}
			remove := uint64(record.Count)
			if remove > remaining {
				remove = remaining
			}
			record.Count -= uint32(remove)
			remaining -= remove
			if record.Count == 0 {
				fullGUID := uint64(record.ItemGUID) | (uint64(0x4000) << 48)
				destroyed = append(destroyed, fullGUID)
				if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", record.ItemGUID); err != nil {
					return nil, err
				}
				if _, err := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", record.ItemGUID); err != nil {
					return nil, err
				}
			} else if _, err := tx.ExecContext(ctx, "UPDATE item_instance SET count = ? WHERE guid = ?", record.Count, record.ItemGUID); err != nil {
				return nil, err
			}
		}
		if spec.strict && !spec.destroyAll && remaining != 0 {
			return nil, fmt.Errorf("required quest item %d is unavailable", spec.entry)
		}
	}
	return destroyed, nil
}

// rewardQuestReputation mirrors Player::RewardReputation(Quest const*)
// (Player.cpp:6784-6825): the quest_template RewardFactionValue columns are
// QuestFactionReward.dbc field indices, not flat reputation — row 1 for
// positive values, row 2 for negative (QuestDef.cpp:456) — while
// RewardFactionOverride carries a flat value /100 that skips the aura bonus.
// Gains scale by quest source (daily/weekly/monthly/repeatable/quest) and
// the three faction team hacks gate cross-team rewards
// (ReputationMgr::IsReputationAllowedForTeam, ReputationMgr.cpp:69-82).
func (s *session) rewardQuestReputation(ctx context.Context, view questRewardView, questID uint32) {
	if s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	team := playerTeam(s.player.Race)
	for i := 0; i < questRewardFactions; i++ {
		facID := view.Detail.RewardFactionID[i]
		if facID == 0 || !isReputationAllowedForQuestTeam(team, facID) {
			continue
		}
		rep := int32(0)
		noQuestBonus := false
		if override := view.Detail.RewardFactionOverride[i]; override != 0 {
			rep = override / 100
			noQuestBonus = true
		} else {
			value := view.Detail.RewardFactionValue[i]
			row := uint32(1)
			field := uint32(value)
			if value < 0 {
				row = 2
				field = uint32(-int64(value))
			}
			dbcRep, found, err := s.server.Data.QuestFactionReward(row, field)
			if err != nil || !found {
				continue
			}
			rep = dbcRep
		}
		if rep == 0 {
			continue
		}
		rep = s.calculateQuestReputationGain(facID, rep, noQuestBonus)
		if rep == 0 || !s.spellFactionEntryExists(facID) {
			continue
		}
		s.addReputationAmount(ctx, facID, rep)
	}
}

// isReputationAllowedForQuestTeam mirrors the team hack in
// ReputationMgr::IsReputationAllowedForTeam (ReputationMgr.cpp:69-82).
func isReputationAllowedForQuestTeam(team uint32, factionID uint32) bool {
	if team == teamHorde && (factionID == 1037 || factionID == 946) {
		return false
	}
	if team == teamAlliance && factionID == 947 {
		return false
	}
	return true
}

// calculateQuestReputationGain mirrors Player::CalculateReputationGain
// (Player.cpp:6651-6726) for quest sources: the 156 aura modifier folds into
// percent (skipped under noQuestBonus), gains use 100+mod and losses
// 100-mod, and a zero or negative percent grants nothing. The low-level
// quest rate has no Go config (C++ default 1.0, so the gray-level arm is a
// no-op here); RepRewardRate and the recruit-a-friend bonus have no Go
// model. Go's reputationGainPctMod also folds the faction-specific 190
// modifier, which C++ gates to kills — the quest path keeps Go's existing
// shared-sink behavior there (noted in reputation.go).
func (s *session) calculateQuestReputationGain(factionID uint32, rep int32, noQuestBonus bool) int32 {
	// All quest sources (daily/weekly/monthly/repeatable/plain) share
	// RATE_REPUTATION_LOWLEVEL_QUEST in the C++; with no Go config for it
	// the C++ default 1.0 applies, so the source switch and the gray-level
	// reduction arm are no-ops here.
	percent := float32(100)
	if !noQuestBonus {
		if mod := s.reputationGainPctMod(factionID); mod != 0 {
			if rep > 0 {
				percent += float32(mod)
			} else {
				percent -= float32(mod)
			}
		}
	}
	if percent <= 0 {
		return 0
	}
	return int32(float32(rep) * percent / 100)
}
