package world

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	questRewardItems       = 4
	questChoiceItems       = 6
	questRewardFactions    = 5
	questDetailEmotes      = 4
	questAutoCompleteFlags = 0x00010000
	questFlagsAutoAccept   = 0x00080000 // QUEST_FLAGS_AUTO_ACCEPT (QuestDef.h:151)
	questFlagsPartyAccept  = 0x00000002 // QUEST_FLAGS_PARTY_ACCEPT (QuestDef.h:133)
	questFlagsSharable     = 0x00000008 // QUEST_FLAGS_SHARABLE (QuestDef.h:135)
)

// Quest::HasSpecialFlag bits used by the timed-quest model (QuestDef.h:173-175).
const (
	questSpecialFlagsTimed            = 0x400  // QUEST_SPECIAL_FLAGS_TIMED
	questSpecialFlagsCompletedAtStart = 0x1000 // QUEST_SPECIAL_FLAGS_COMPLETED_AT_START
)

// addTimedQuest mirrors Player::AddTimedQuest (Player.h:1318):
// m_timedquests.insert(questId).
func (s *session) addTimedQuest(questID uint32) {
	if s == nil {
		return
	}
	s.timedQuestsMu.Lock()
	if s.timedQuests == nil {
		s.timedQuests = make(map[uint32]struct{})
	}
	s.timedQuests[questID] = struct{}{}
	s.timedQuestsMu.Unlock()
}

// removeTimedQuest mirrors Player::RemoveTimedQuest (Player.h:1319):
// m_timedquests.erase(questId).
func (s *session) removeTimedQuest(questID uint32) {
	if s == nil {
		return
	}
	s.timedQuestsMu.Lock()
	delete(s.timedQuests, questID)
	s.timedQuestsMu.Unlock()
}

// questTimedRemainingMs reports the milliseconds left on a timed quest whose
// quest-log TIME field holds the absolute expiry epoch (Player::AddQuest,
// Player.cpp:15258).
func questTimedRemainingMs(timer, now uint32) int64 {
	return (int64(timer) - int64(now)) * 1000
}

// updateTimedQuests mirrors the timed-quest block of Player::Update
// (Player.cpp:1096-1113), run on the world tick: every timed quest whose
// remaining time fits inside the tick diff fails via Player::FailQuest.
func (s *Server) updateTimedQuests(ctx context.Context, diff time.Duration) {
	now := uint32(time.Now().Unix())
	diffMs := diff.Milliseconds()
	s.sessionsMu.RLock()
	type expiry struct {
		sess    *session
		questID uint32
	}
	var due []expiry
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		sess.timedQuestsMu.Lock()
		for questID := range sess.timedQuests {
			var timer uint32
			for slot := 0; slot < playerQuestLogSlots; slot++ {
				if sess.player.QuestLog[slot].QuestID == questID {
					timer = sess.player.QuestLog[slot].Timer
					break
				}
			}
			// C++: q_status.Timer <= p_time → FailQuest. The Go log slot
			// TIME is the absolute expiry epoch, so the remaining
			// milliseconds come from timer - now.
			if questTimedRemainingMs(timer, now) <= diffMs {
				due = append(due, expiry{sess, questID})
			}
		}
		sess.timedQuestsMu.Unlock()
	}
	s.sessionsMu.RUnlock()
	for _, e := range due {
		e.sess.failQuest(ctx, e.questID)
	}
}

// restoreTimedQuests mirrors the timed-quest arm of Player::_LoadQuestStatus
// (Player.cpp:18734-18742): an incomplete, unrewarded timed quest rejoins
// the timed set; its DB timer (absolute expiry epoch, like the quest-log
// TIME field) stays as loaded, or becomes Timer=1 when already expired so
// the next world tick fails the quest.
func (s *session) restoreTimedQuests(ctx context.Context, state *playerState) {
	if s == nil || state == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	now := uint32(time.Now().Unix())
	wdb := s.server.WorldStore.DB
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		entry := &state.QuestLog[slot]
		if entry.QuestID == 0 || s.isQuestRewarded(ctx, entry.QuestID) {
			continue
		}
		var specialFlags int64
		if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", entry.QuestID).Scan(&specialFlags); err != nil || specialFlags&questSpecialFlagsTimed == 0 {
			continue
		}
		if status, _ := s.characterQuestStatus(ctx, entry.QuestID); status != questStatusIncomplete {
			continue
		}
		s.addTimedQuest(entry.QuestID)
		if entry.Timer <= now {
			entry.Timer = 1
		}
	}
}

// failQuest mirrors Player::FailQuest (Player.cpp:15529-15577): only an
// INCOMPLETE quest can fail (or a COMPLETE timed quest with
// COMPLETED_AT_START). The log slot takes Timer=1 + QUEST_STATE_FAIL; timed
// quests leave the timed set with their timer cleared and answer
// SMSG_QUESTUPDATE_FAILEDTIMER (Player::SendQuestTimerFailed,
// Player.cpp:17043) instead of SMSG_QUESTGIVER_QUEST_FAILED.
func (s *session) failQuest(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var specialFlags int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags); err != nil {
		return
	}
	timed := specialFlags&questSpecialFlagsTimed != 0
	status, _ := s.characterQuestStatus(ctx, questID)
	if status != questStatusIncomplete {
		if status != questStatusComplete || !timed || specialFlags&questSpecialFlagsCompletedAtStart == 0 {
			return
		}
	}
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusFailed, s.playerGUID, questID)
	}
	// Player::FailQuest (Player.cpp:15543): the FAILED transition runs
	// through Player::SetQuestStatus (update=true) — SendQuestUpdate first
	// (Player.cpp:16179), then OnQuestStatusChange — before the log-slot
	// and destroy arms.
	s.sendQuestUpdate(ctx, questID)
	s.triggerPlayerEvent(ctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), questID)
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		if s.player.QuestLog[slot].QuestID == questID {
			s.player.QuestLog[slot].Timer = 1
			s.player.QuestLog[slot].State = questSlotStateFail
			s.sendPlayerQuestLogUpdate(slot)
			break
		}
	}
	if timed {
		s.removeTimedQuest(questID)
		packet := protocol.NewBuffer(4)
		packet.WriteU32(questID)
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_FAILEDTIMER), packet.Bytes(), true)
	} else {
		packet := protocol.NewBuffer(8)
		packet.WriteU32(questID)
		packet.WriteU32(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_FAILED), packet.Bytes(), true)
	}
	s.destroyQuestItems(ctx, questID)
}

// QuestFailedReason values for SMSG_QUESTGIVER_QUEST_INVALID (QuestDef.h:48).
const (
	questInvalidDontHaveReq = 0
	questInvalidAlreadyDone = 7
	questInvalidAlreadyOn   = 13
)

type questRewardItem struct {
	ID        uint32
	Quantity  uint32
	DisplayID uint32
}

type questDescEmote struct {
	Type  uint32
	Delay uint32
}

type questDetailData struct {
	ID                    uint32
	Title                 string
	Objectives            string
	Details               string
	Flags                 uint32
	SuggestedGroupNum     uint32
	RewardMoney           uint32
	RequiredMoney         uint32
	RewardXPDifficulty    uint32
	RewardBonusMoney      uint32
	RewardDisplaySpell    uint32
	RewardSpell           int32
	RewardHonor           uint32
	RewardKillHonor       float32
	RewardTitleID         uint32
	RewardTalents         uint32
	RewardArenaPoints     int32
	RewardItems           []questRewardItem
	ChoiceItems           []questRewardItem
	RewardFactionID       [questRewardFactions]uint32
	RewardFactionValue    [questRewardFactions]int32
	RewardFactionOverride [questRewardFactions]int32
	DescEmotes            [questDetailEmotes]questDescEmote
}

// sendQuestGiverQuestInvalid mirrors Player::SendCanTakeQuestResponse
// (Player.cpp:17054): SMSG_QUESTGIVER_QUEST_INVALID carries only the
// QuestFailedReason.
func (s *session) sendQuestGiverQuestInvalid(reason uint32) bool {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(reason)
	return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_QUEST_INVALID), packet.Bytes(), true) == nil
}

func (s *session) handleQuestgiverQueryQuest(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	questID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if len(payload) > 12 {
		if _, err := reader.ReadU8(); err != nil {
			return false
		}
	}
	// TrinityCore HandleQuestgiverQueryQuestOpcode (QuestHandler.cpp:239-244)
	// resolves TYPEMASK_UNIT | TYPEMASK_GAMEOBJECT | TYPEMASK_ITEM and closes
	// the menu unless the object starts the quest or is involved in it. Go
	// previously resolved creatures only, so gameobject and item givers were
	// always answered with a menu close.
	if !s.questGiverObjectHasQuest(ctx, guid, questID) {
		return s.sendGossipComplete()
	}
	// TrinityCore HandleQuestgiverQueryQuestOpcode (QuestHandler.cpp:233) gates the
	// details on CanTakeQuest(quest, true): already-on, already-done, and failed
	// requirement checks reply with SMSG_QUESTGIVER_QUEST_INVALID instead of the
	// details, and the opcode never routes into the CompleteQuest path.
	status, _ := s.characterQuestStatus(ctx, questID)
	if status != 0 && status != questStatusRewarded {
		return s.sendQuestGiverQuestInvalid(questInvalidAlreadyOn)
	}
	if s.isQuestRewarded(ctx, questID) {
		return s.sendQuestGiverQuestInvalid(questInvalidAlreadyDone)
	}
	if canTake, err := s.canTakeQuest(ctx, questID); err != nil || !canTake {
		if err != nil {
			s.debug("quest details failed", "account", s.accountName, "quest", questID, "error", err)
			return false
		}
		// Go's canTakeQuest does not report which Satisfy* check failed, so the
		// generic INVALIDREASON_DONT_HAVE_REQ goes out where C++ sends the specific
		// reason.
		return s.sendQuestGiverQuestInvalid(questInvalidDontHaveReq)
	}
	data, err := s.loadQuestDetailData(ctx, questID)
	if errors.Is(err, sql.ErrNoRows) {
		return s.sendGossipComplete()
	}
	if err != nil {
		s.debug("quest details failed", "account", s.accountName, "quest", questID, "error", err)
		return false
	}

	// In TrinityCore Player.cpp:14836 and HandleQuestgiverQueryQuestOpcode
	// (QuestHandler.cpp:257), if quest is AutoAccept, add it upon viewing
	// details — but only behind the full CanAddQuest(msg=true) gate. Go
	// auto-accepted on the SpecialFlags bit alone, so a full quest log or an
	// unstoreable source item still added the quest (with the C++ SMSG_QUESTLOG_FULL
	// / equip-error feedback missing).
	var specialFlags int64
	_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT SpecialFlags FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	// Quest::IsAutoAccept (QuestDef.cpp:310-313) is HasFlag(QUEST_FLAGS_AUTO_ACCEPT);
	// Quest::LoadQuestTemplateAddon (QuestDef.cpp:193) folds
	// QUEST_SPECIAL_FLAGS_AUTO_ACCEPT (0x4) into _flags at load, so Go (raw DB
	// read) must OR both legs. Go checked only the SpecialFlags leg, so quests
	// flagged via quest_template.Flags never auto-accepted here.
	if (data.Flags&questFlagsAutoAccept != 0 || specialFlags&4 != 0) && s.canAddQuest(ctx, questID, true) {
		s.addQuestToPlayer(ctx, questID, guid)
		// Eluna CREATURE_EVENT_ON_QUEST_ACCEPT (event 31), fired from
		// Player::AddQuestAndCheckCompletion (Player.cpp:15119 region).
		s.fireCreatureQuestHook(ctx, guid, scripting.CreatureEventOnQuestAccept, s.luaQuest(ctx, questID))
		s.fireGameObjectQuestHook(ctx, guid, scripting.GameObjectEventOnQuestAccept, s.luaQuest(ctx, questID))
		// Eluna ITEM_EVENT_ON_QUEST_ACCEPT (event 3), fired from the
		// TYPEID_ITEM/TYPEID_CONTAINER arm of Player::AddQuestAndCheckCompletion
		// (Player.cpp:15128); fires only for item giver GUIDs.
		s.fireItemQuestHook(ctx, guid, questID)
	}

	// TrinityCore HandleQuestgiverQueryQuestOpcode (QuestHandler.cpp:233): quests
	// with QUEST_FLAGS_AUTOCOMPLETE send RequestItems (collapsing to OfferReward
	// when no items are required and the quest is complete, as
	// PlayerMenu::SendQuestGiverRequestItems does in GossipDef.cpp:471) instead
	// of the details packet.
	if data.Flags&questAutoCompleteFlags != 0 {
		view, err := s.loadQuestRewardView(ctx, questID)
		if err != nil {
			s.debug("quest details failed", "account", s.accountName, "quest", questID, "error", err)
			return false
		}
		canComplete := s.canCompleteQuest(ctx, questID)
		if len(view.RequiredItems) == 0 && canComplete {
			return s.sendQuestOfferReward(view, guid, true)
		}
		return s.sendQuestRequestItems(view, guid, canComplete, true)
	}

	return s.sendQuestGiverQuestDetails(ctx, guid, data)
}

// sendQuestGiverQuestDetails mirrors Player::SendQuestGiverQuestDetails
// (PlayerMenu): the raw quest-details packet with no CanTakeQuest gate.
// The query opcode calls it after its own gates; the single-quest fast-open
// arm of Player::SendPreparedQuest (Player.cpp:14840) calls it directly.
func (s *session) sendQuestGiverQuestDetails(ctx context.Context, guid uint64, data questDetailData) bool {
	packet := buildQuestGiverDetails(data, guid, s.sharingQuestSender)
	s.debug("quest details response", "account", s.accountName, "quest", data.ID)
	return s.write(uint16(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS), packet, true) == nil
}

// rejectQuestgiverAccept mirrors the CLOSE_GOSSIP_CLEAR_SHARING_INFO
// lambda of WorldSession::HandleQuestgiverAcceptQuestOpcode
// (QuestHandler.cpp:131-135): every rejection arm closes the gossip window
// AND clears the quest-sharing info. Go's arms closed the window but left a
// stale sharing offer behind, so a later confirm-accept could bind to a
// sharer from a quest that was never taken.
func (s *session) rejectQuestgiverAccept() bool {
	s.sharingQuestID = 0
	s.sharingQuestSender = 0
	return s.sendGossipComplete()
}

// satisfyQuestLog mirrors Player::SatisfyQuestLog (Player.cpp:15645-15657):
// a free quest-log slot must exist; msg=true answers SMSG_QUESTLOG_FULL,
// matching C++'s SendDirectMessage of the empty packet.
func (s *session) satisfyQuestLog(msg bool) bool {
	if s.player != nil {
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if s.player.QuestLog[slot].QuestID == 0 {
				return true
			}
		}
	}
	if msg {
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTLOG_FULL), nil, true)
	}
	return false
}

// canAddQuest mirrors Player::CanAddQuest (Player.cpp:14946-14967): the log
// must have a free slot (SatisfyQuestLog) and the quest's source item must
// fit (CanStoreNewItem). EQUIP_ERR_CANT_CARRY_MORE_OF_THIS still allows the
// add — the player already holds the max. C++ runs this gate in
// HandleQuestgiverAcceptQuestOpcode (QuestHandler.cpp:153) before
// AddQuestAndCheckCompletion; Go ran no gate at all, so a full quest log
// wrote the character_queststatus row and granted the source item even
// though the slot claim silently dropped them.
func (s *session) canAddQuest(ctx context.Context, questID uint32, msg bool) bool {
	if !s.satisfyQuestLog(msg) {
		return false
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	var srcItem, srcCount int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(StartItem, 0), COALESCE(StartItemCount, 0) FROM quest_template WHERE ID = ?", questID).Scan(&srcItem, &srcCount); err != nil || srcItem <= 0 {
		return true
	}
	count := uint32(srcCount)
	if count == 0 {
		count = 1
	}
	switch res := s.canStoreNewItem(ctx, s.playerGUID, uint32(srcItem), count); res {
	case equipErrOk, equipErrCantCarryMoreOfThis:
		return true
	default:
		if msg {
			s.sendEquipError(res, 0)
		}
		return false
	}
}

// canInteractWithQuestGiver mirrors Player::CanInteractWithQuestGiver
// (Player.cpp:2296-2312): the WPE guard HandleQuestgiverAcceptQuestOpcode
// runs (QuestHandler.cpp:149) before the quest template is even loaded.
// Creatures need the QUESTGIVER npc flag inside interaction range, GOs the
// QUESTGIVER type on the player's map/instance in range, players need both
// parties alive, items only the accepting player. The player-giver share
// validation (CanShareQuest/IsInSameRaidWith, QuestHandler.cpp:138-151)
// belongs to handleQuestConfirmAccept, Go's share-accept path.
func (s *session) canInteractWithQuestGiver(ctx context.Context, guid uint64) bool {
	if s == nil || s.player == nil {
		return false
	}
	high := uint16(guid >> 48)
	switch {
	case high == 0x4000:
		return s.player.Health > 0
	case high == 0x0000 && guid != 0:
		giver := s.server.findSessionByGUID(guid)
		return s.player.Health > 0 && giver != nil && giver.player != nil && giver.player.Health > 0
	case high == 0xF110:
		// Player::CanInteractWithQuestGiver (Player.cpp:2301) -> TYPEID_GAMEOBJECT
		// -> GetGameObjectIfCanInteractWith -> the in-flight gate
		// (Player.cpp:2371). The player-giver and item-giver arms above have
		// no such gate in C++ (IsAlive-only).
		if s.isInFlight() {
			return false
		}
		if s.server == nil {
			return false
		}
		entry := uint32((guid >> 24) & 0x00FFFFFF)
		low := uint32(guid & 0x00FFFFFF)
		goState, err := s.server.getOrLoadGameObjectState(ctx, guid, low, entry, s.player.Map, s.player.InstanceID)
		if err != nil || goState == nil || goState.IconName == "Point" || goState.Type != GameObjectTypeQuestGiver {
			return false
		}
		if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID ||
			distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 10.0 {
			return false
		}
		return true
	default:
		creature := s.luaCreature(ctx, guid)
		if creature == nil || !s.canInteractWithCreature(ctx, creature) {
			return false
		}
		return objectUint32OrZero(creature, "NPCFlags")&npcFlagQuestgiver != 0
	}
}

func (s *session) handleQuestgiverAcceptQuest(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	questID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if len(payload) > 12 {
		if _, err := reader.ReadU32(); err != nil {
			return false
		}
	}
	if !s.questgiverStartsQuest(ctx, guid, questID) {
		s.debug("quest accept rejected", "account", s.accountName, "quest", questID, "guid", guid, "reason", "questgiver does not start quest")
		return s.rejectQuestgiverAccept()
	}
	// WorldSession::HandleQuestgiverAcceptQuestOpcode (QuestHandler.cpp:149):
	// the WPE guard runs before the quest template is loaded.
	if !s.canInteractWithQuestGiver(ctx, guid) {
		s.debug("quest accept rejected", "account", s.accountName, "quest", questID, "guid", guid, "reason", "cannot interact with quest giver")
		return s.rejectQuestgiverAccept()
	}
	// If player is already on this quest, close gossip and return success
	status, _ := s.characterQuestStatus(ctx, questID)
	if status == questStatusIncomplete {
		return s.rejectQuestgiverAccept()
	}
	// TrinityCore re-validates CanTakeQuest on accept before AddQuest.
	canTake, err := s.canTakeQuest(ctx, questID)
	if err != nil || !canTake {
		s.debug("quest accept rejected", "account", s.accountName, "quest", questID, "reason", "canTakeQuest failed", "error", err)
		return s.rejectQuestgiverAccept()
	}
	// WorldSession::HandleQuestgiverAcceptQuestOpcode (QuestHandler.cpp:153):
	// CanAddQuest gates AddQuestAndCheckCompletion — a full quest log or an
	// unstoreable source item rejects with the window closed and the sharing
	// info cleared.
	if !s.canAddQuest(ctx, questID, true) {
		s.debug("quest accept rejected", "account", s.accountName, "quest", questID, "reason", "canAddQuest failed")
		return s.rejectQuestgiverAccept()
	}
	// QuestHandler.cpp:144-151 — an accept that lands while the player still
	// holds quest-sharing info answers QUEST_PARTY_MSG_ACCEPT_QUEST to the
	// sharer and clears the sharing info, ahead of CanAddQuest like the C++.
	if s.sharingQuestSender != 0 && s.server != nil {
		if sharer := s.server.findSessionByGUID(s.sharingQuestSender); sharer != nil {
			sharer.sendPushToPartyResponse(s.playerGUID, QuestPartyMsgAcceptQuest)
			s.sharingQuestID = 0
			s.sharingQuestSender = 0
		}
	}
	s.addQuestToPlayer(ctx, questID, guid)
	// Eluna CREATURE_EVENT_ON_QUEST_ACCEPT (event 31), fired from
	// Player::AddQuestAndCheckCompletion after the auto-complete check
	// (Player.cpp:15105-15122).
	s.fireCreatureQuestHook(ctx, guid, scripting.CreatureEventOnQuestAccept, s.luaQuest(ctx, questID))
	s.fireGameObjectQuestHook(ctx, guid, scripting.GameObjectEventOnQuestAccept, s.luaQuest(ctx, questID))
	// Eluna ITEM_EVENT_ON_QUEST_ACCEPT (event 3), fired from the
	// TYPEID_ITEM/TYPEID_CONTAINER arm of Player::AddQuestAndCheckCompletion
	// (Player.cpp:15128); fires only for item giver GUIDs.
	s.fireItemQuestHook(ctx, guid, questID)
	// Player::AddQuestAndCheckCompletion, TYPEID_ITEM arm (Player.cpp:15124):
	// accepting from an item destroys the start item (modulo the
	// required-unique and source-item exceptions).
	if uint16(guid>>48) == 0x4000 {
		s.destroyQuestStartItem(ctx, questID, guid)
	}
	s.debug("quest accepted", "account", s.accountName, "quest", questID)

	// QuestHandler.cpp:155-173 — QUEST_FLAGS_PARTY_ACCEPT: every group member
	// that can take the quest gets a confirm-accept prompt.
	s.broadcastQuestPartyAccept(ctx, questID)
	// QuestHandler.cpp:176-177 — the quest's source spell is cast triggered.
	s.castQuestSourceSpell(ctx, questID)

	// Deliberate UX push (not C++: Player::AddQuest only sends SendQuestUpdate) —
	// refresh the questgiver's overhead icon immediately so the "!" clears or
	// updates without waiting for the client's next status query.
	if guid != 0 {
		var entry uint32
		if creature := s.luaCreature(ctx, guid); creature != nil {
			entry = objectUint32OrZero(creature, "Entry")
		}
		if entry == 0 {
			entry = uint32((guid >> 24) & 0x00FFFFFF)
		}
		if entry == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			rawGUID := guid & 0x00000000FFFFFFFF
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", rawGUID).Scan(&entry)
		}
		status := uint8(questDialogNone)
		if entry != 0 {
			if st, err := s.questDialogStatus(ctx, entry); err == nil {
				status = st
			}
		}
		packet := protocol.NewBuffer(9)
		packet.WriteU64(guid)
		packet.WriteU8(status)
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS), packet.Bytes(), true)
	}

	return s.sendGossipComplete()
}

// broadcastQuestPartyAccept mirrors the QUEST_FLAGS_PARTY_ACCEPT arm of
// WorldSession::HandleQuestgiverAcceptQuestOpcode (QuestHandler.cpp:155-173):
// every group member (not self) that CanTakeQuest(quest, true) gets the quest
// sharing info, a gossip close, and SMSG_QUEST_CONFIRM_ACCEPT (questID, title,
// quester GUID — Player::SendQuestConfirmAccept, Player.cpp:17062).
func (s *session) broadcastQuestPartyAccept(ctx context.Context, questID uint32) {
	data, err := s.loadQuestDetailData(ctx, questID)
	if err != nil || data.Flags&questFlagsPartyAccept == 0 {
		return
	}
	if s.groupID == 0 || s.server == nil {
		return
	}
	for _, member := range s.server.getGroupSessions(s.groupID) {
		if member == nil || member == s || member.player == nil || !member.worldReady.Load() {
			continue
		}
		canTake, err := member.canTakeQuest(ctx, questID)
		if err != nil || !canTake {
			continue
		}
		member.sharingQuestID = questID
		member.sharingQuestSender = s.playerGUID
		member.sendGossipComplete()
		packet := protocol.NewBuffer(64)
		packet.WriteU32(questID)
		packet.WriteCString(data.Title)
		packet.WriteU64(s.playerGUID)
		_ = member.write(uint16(protocol.OpcodeSMSG_QUEST_CONFIRM_ACCEPT), packet.Bytes(), true)
	}
}

// castQuestSourceSpell mirrors the source-spell arm of
// WorldSession::HandleQuestgiverAcceptQuestOpcode (QuestHandler.cpp:176-177)
// and WorldSession::HandleQuestConfirmAccept (QuestHandler.cpp:501-503): the
// quest_template_addon SrcSpell is cast triggered on the player. The packet
// bridge follows the trainer teach-spell arm; spell effects resolve
// server-side only where modeled.
func (s *session) castQuestSourceSpell(ctx context.Context, questID uint32) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var srcSpell int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SrcSpell, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&srcSpell); err != nil || srcSpell <= 0 {
		return
	}
	castTimeStamp := uint32(time.Now().UnixMilli())
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
	goPkt := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 1, uint32(srcSpell), spellCastFlagGo, castTimeStamp, []uint64{s.playerGUID}, nil, target)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
	}
}

func (s *session) addQuestToPlayer(ctx context.Context, questID uint32, questGiverGUID uint64) bool {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	now := uint32(time.Now().Unix())
	// Player::AddQuest, timed-quest arm (Player.cpp:15248-15266): a
	// QUEST_SPECIAL_FLAGS_TIMED quest joins the timed set, its in-memory
	// timer starts at TimeAllowed seconds, and the quest-log TIME field
	// carries the absolute expiry epoch (qtime). The DB timer column keeps
	// the same absolute epoch (Player::_SaveQuestStatus, Player.cpp:20184).
	var timed bool
	var timedExpiry uint32
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		wdb := s.server.WorldStore.DB
		var specialFlags, timeAllowed int64
		if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags); err == nil && specialFlags&questSpecialFlagsTimed != 0 {
			timed = true
			_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(TimeAllowed, 0) FROM quest_template WHERE ID = ?", questID).Scan(&timeAllowed)
			// Shared timed quest (Player.cpp:15253): when the giver is a
			// player, the taker inherits the giver's remaining time.
			if questGiverGUID != 0 && uint16(questGiverGUID>>48) == 0x0000 {
				if giver := s.server.findSessionByGUID(questGiverGUID); giver != nil && giver.player != nil {
					shared := int64(0)
					for slot := 0; slot < playerQuestLogSlots; slot++ {
						if giver.player.QuestLog[slot].QuestID == questID {
							if gt := int64(giver.player.QuestLog[slot].Timer); gt > int64(now) {
								shared = gt - int64(now)
							}
							timeAllowed = shared
							break
						}
					}
				}
			}
			if timeAllowed < 0 {
				timeAllowed = 0
			}
			timedExpiry = now + uint32(timeAllowed)
			s.addTimedQuest(questID)
		}
	}
	s.startTimedAchievement(timedTypeQuest, questID)
	query := `INSERT OR REPLACE INTO character_queststatus (guid, quest, status) VALUES (?, ?, ?)`
	args := []any{s.playerGUID, questID, questStatusIncomplete}
	if s.server.CharactersStore.Backend != database.BackendSQLite {
		query = `INSERT INTO character_queststatus (guid, quest, status) VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE status = VALUES(status)`
	}
	if timed {
		query = `INSERT OR REPLACE INTO character_queststatus (guid, quest, status, timer) VALUES (?, ?, ?, ?)`
		args = append(args, timedExpiry)
		if s.server.CharactersStore.Backend != database.BackendSQLite {
			query = `INSERT INTO character_queststatus (guid, quest, status, timer) VALUES (?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE status = VALUES(status), timer = VALUES(timer)`
		}
	}
	if _, err := s.server.CharactersStore.DB.ExecContext(ctx, query, args...); err != nil {
		if !missingTable(err) {
			s.debug("quest accept failed", "account", s.accountName, "quest", questID, "error", err)
			return false
		}
	}
	// Player::AddQuest (Player.cpp:15236): the quest's source item is
	// granted before the quest log slot is claimed — the C++ grant runs
	// at 15236 while the slot is set at 15276, so the grant's
	// ItemAddedQuestCheck scan (Player.cpp:16544) cannot see the new
	// quest yet. Like the C++ the result is ignored by the caller.
	s.giveQuestSourceItem(ctx, questID)

	// Player::AddQuest claims a free log slot and updates PLAYER_QUEST_LOG
	if s.player != nil {
		targetSlot := -1
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if s.player.QuestLog[slot].QuestID == questID {
				targetSlot = slot
				break
			}
			if targetSlot == -1 && s.player.QuestLog[slot].QuestID == 0 {
				targetSlot = slot
			}
		}
		if targetSlot != -1 {
			s.player.QuestLog[targetSlot] = questLogEntry{QuestID: questID, Timer: timedExpiry}
			s.sendPlayerQuestLogUpdate(targetSlot)
		}
	}
	// Player::AddQuest (Player.cpp:15237): the requirement counts are
	// re-adjusted after the grant, so a granted item that doubles as a
	// required item lands in the item objectives.
	s.initializeQuestItemCounts(ctx, questID)

	// Player::AddQuest (Player.cpp:15241-15247, 15267-15271): accepting a
	// quest makes its rep-objective factions visible, and a PvP-flagged
	// quest forces the hostile state immediately (UpdatePvPState). The
	// quest row is already written above, so the forcing-quest scan sees
	// the new quest.
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var questFlags, repFac1, repFac2 int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(Flags, 0), COALESCE(RepObjectiveFaction, 0), COALESCE(RepObjectiveFaction2, 0) FROM quest_template WHERE ID = ?`, questID).Scan(&questFlags, &repFac1, &repFac2); err == nil {
			if repFac1 > 0 {
				s.setFactionVisible(ctx, uint32(repFac1))
			}
			if repFac2 > 0 {
				s.setFactionVisible(ctx, uint32(repFac2))
			}
			if questFlags&questFlagsPVP != 0 && s.player != nil {
				if s.applyZoneState(ctx, s.player, s.player.Zone, s.areaID) {
					s.sendPlayerUpdate()
				}
			}
		}
	}

	// Player::AddQuest (Player.cpp:15284/15290): SendQuestUpdate runs at
	// the end of AddQuest, right before the status-change hook and before
	// AddQuestAndCheckCompletion's completion check (which may fire again
	// via SetQuestStatus in CompleteQuest).
	s.sendQuestUpdate(ctx, questID)
	s.triggerPlayerEvent(ctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), questID)

	// If quest has no objectives or immediate completion requirements, mark complete immediately (Player::AddQuestAndCheckCompletion)
	if s.canCompleteQuest(ctx, questID) {
		s.completeQuest(ctx, questID)
	}

	return true
}

// giveQuestSourceItem mirrors Player::GiveQuestSourceItem (Player.cpp:16048-16076).
// No grant when the quest was started by the item itself
// (item_template.StartQuest == quest id), the stored count defaults to 1,
// and the CanStoreNewItem gate decides: EQUIP_ERR_OK stores and notifies,
// CANT_CARRY_MORE_OF_THIS reports success (the player already holds the
// max), anything else sends the equip error and fails the grant.
func (s *session) giveQuestSourceItem(ctx context.Context, questID uint32) bool {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	wdb := s.server.WorldStore.DB
	var startItem, startCount int64
	if err := wdb.QueryRowContext(ctx, "SELECT StartItem, COALESCE(StartItemCount, 0) FROM quest_template WHERE ID = ?", questID).Scan(&startItem, &startCount); err != nil || startItem <= 0 {
		return true
	}
	var startQuest int64
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(StartQuest, 0) FROM item_template WHERE entry = ?", startItem).Scan(&startQuest); err == nil && uint32(startQuest) == questID {
		return true
	}
	count := uint32(startCount)
	if count == 0 {
		count = 1
	}
	switch res := s.canStoreNewItem(ctx, s.playerGUID, uint32(startItem), count); res {
	case equipErrOk:
		s.grantQuestStartItem(ctx, uint32(startItem), count)
		return true
	case equipErrCantCarryMoreOfThis:
		return true
	default:
		s.sendEquipError(res, 0)
		return false
	}
}

// takeQuestSourceItem mirrors Player::TakeQuestSourceItem (Player.cpp:16081-16116):
// on abandon the source item granted at accept is destroyed with the
// CanUnequipItems gate, unless the item is the quest's own start item.
func (s *session) takeQuestSourceItem(ctx context.Context, questID uint32, msg bool) bool {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	wdb := s.server.WorldStore.DB
	var startItem, startCount int64
	if err := wdb.QueryRowContext(ctx, "SELECT StartItem, COALESCE(StartItemCount, 0) FROM quest_template WHERE ID = ?", questID).Scan(&startItem, &startCount); err != nil || startItem <= 0 {
		return true
	}
	count := uint32(startCount)
	if count == 0 {
		count = 1
	}
	if res := s.canUnequipQuestItems(ctx, uint32(startItem), count); res != equipErrOk {
		if msg {
			s.sendEquipError(res, 0)
		}
		return false
	}
	var startQuest int64
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(StartQuest, 0) FROM item_template WHERE entry = ?", startItem).Scan(&startQuest); err == nil && uint32(startQuest) == questID {
		return true
	}
	s.destroyQuestItemEntry(ctx, uint32(startItem), count)
	return true
}

// canUnequipQuestItems mirrors Player::CanUnequipItems (Player.cpp:9860-9890)
// for the abandon path: equipped copies of the entry fail the unequip gate
// (Go has no locked/equipped quest items, so an equipment copy can never be
// auto-unequipped), backpack copies just count toward the total; like the
// C++ the last error is returned when the count is not reached.
func (s *session) canUnequipQuestItems(ctx context.Context, entry, count uint32) uint8 {
	cdb := s.server.CharactersStore.DB
	res := uint8(equipErrOk)
	var temp uint32
	rows, err := cdb.QueryContext(ctx, `SELECT ci.slot, ii.count FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ii.itemEntry = ?`, s.playerGUID, entry)
	if err != nil {
		return equipErrOk
	}
	defer rows.Close()
	for rows.Next() {
		var slot uint8
		var c uint32
		if rows.Scan(&slot, &c) != nil {
			continue
		}
		if slot <= 18 {
			res = equipErrCantUnequip
			continue
		}
		temp += c
		if temp >= count {
			return equipErrOk
		}
	}
	return res
}

// destroyQuestItemEntry destroys up to count copies of entry across the
// player's inventory instances (Player::DestroyItemCount fan-out used by
// TakeQuestSourceItem, Player.cpp:16112).
func (s *session) destroyQuestItemEntry(ctx context.Context, entry, count uint32) {
	cdb := s.server.CharactersStore.DB
	rows, err := cdb.QueryContext(ctx, `SELECT ci.item FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ii.itemEntry = ?
		ORDER BY ci.bag, ci.slot`, s.playerGUID, entry)
	if err != nil {
		return
	}
	var guids []uint64
	for rows.Next() {
		var g uint64
		if rows.Scan(&g) == nil {
			guids = append(guids, g)
		}
	}
	rows.Close()
	for _, g := range guids {
		if count == 0 {
			break
		}
		var cur uint32
		if err := cdb.QueryRowContext(ctx, "SELECT `count` FROM item_instance WHERE guid = ?", g).Scan(&cur); err != nil || cur == 0 {
			continue
		}
		take := count
		if cur < take {
			take = cur
		}
		s.destroyItemInstanceCount(ctx, g, take)
		count -= take
	}
}

// destroyQuestStartItem mirrors the TYPEID_ITEM arm of
// Player::AddQuestAndCheckCompletion (Player.cpp:15124-15149): accepting a
// quest from an item destroys the start item unless it is required for the
// quest (unique RequiredItemId) or it is the quest's own source item.
func (s *session) destroyQuestStartItem(ctx context.Context, questID uint32, itemGUID uint64) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	itemLow := itemGUID & 0x0000FFFFFFFFFFFF
	var instanceGUID uint64
	var entry uint32
	if err := cdb.QueryRowContext(ctx, "SELECT guid, itemEntry FROM item_instance WHERE guid = ? OR guid = ?", itemLow, itemGUID).Scan(&instanceGUID, &entry); err != nil || entry == 0 {
		return
	}
	var req [6]uint32
	if err := wdb.QueryRowContext(ctx, `SELECT RequiredItemId1, RequiredItemId2, RequiredItemId3, RequiredItemId4, RequiredItemId5, RequiredItemId6
		FROM quest_template WHERE ID = ?`, questID).Scan(&req[0], &req[1], &req[2], &req[3], &req[4], &req[5]); err == nil {
		for _, id := range req {
			if id != 0 && id == entry {
				var maxCount int64
				if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(MaxCount, 0) FROM item_template WHERE entry = ?", entry).Scan(&maxCount); err == nil && maxCount > 0 {
					return
				}
			}
		}
	}
	var startItem int64
	if err := wdb.QueryRowContext(ctx, "SELECT StartItem FROM quest_template WHERE ID = ?", questID).Scan(&startItem); err == nil && uint32(startItem) == entry {
		return
	}
	s.destroyItemInstance(ctx, instanceGUID)
}

func (s *session) grantQuestStartItem(ctx context.Context, itemEntry, count uint32) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	usedSlots := make(map[uint8]bool)
	rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = 0", s.playerGUID)
	if err == nil {
		for rows.Next() {
			var sl int64
			if rows.Scan(&sl) == nil {
				usedSlots[uint8(sl)] = true
			}
		}
		rows.Close()
	}
	freeSlot := uint8(0xFF)
	for sl := uint8(23); sl <= 38; sl++ {
		if !usedSlots[sl] {
			freeSlot = sl
			break
		}
	}
	if freeSlot == 0xFF {
		return
	}
	nextGUID := uint64(s.server.generateItemGUID())
	if nextGUID == 0 {
		nextGUID = uint64(time.Now().UnixNano())
	}
	// Player::AddQuest (QuestDef.cpp): a quest's source item lands via the
	// StoreNewItem path — Item::Create seeds proto->Duration (Item.cpp:291)
	// and AddItemDurations pushes SMSG_ITEM_TIME_UPDATE at once.
	startDuration := s.server.itemTemplateDuration(ctx, itemEntry)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, ?, ?, 0, ?, ?, 0, 0, '', 0, 100, 0, '')", nextGUID, itemEntry, s.playerGUID, count, startDuration)
	_, _ = cdb.ExecContext(ctx, "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, 0, ?, ?)", s.playerGUID, freeSlot, nextGUID)
	s.adjustQuestItemCount(ctx, itemEntry, count, true)
	_ = s.sendItemCreate(nextGUID, itemEntry, count, 0, freeSlot)
	if startDuration > 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_TIME_UPDATE), protocol.BuildItemTimeUpdate(nextGUID|(uint64(0x4000)<<48), uint32(startDuration)), true)
	}
	_ = s.sendInventoryItems(ctx)
	s.sendPlayerUpdate()
}

func (s *session) handleQuestgiverCancel() bool {
	s.gossip = nil
	s.gossipClosed = true
	return s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true) == nil
}

func (s *session) handleQuestLogRemoveQuest(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	slot := int(payload[0])
	if slot < 0 || slot >= playerQuestLogSlots {
		return true
	}
	questID := s.player.QuestLog[slot].QuestID
	if questID == 0 {
		return true
	}
	s.player.QuestLog[slot] = questLogEntry{}
	s.sendPlayerQuestLogUpdate(slot)
	s.updateAchievementCriteria(criteriaTypeQuestAbandoned, 0, 1)

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_queststatus WHERE guid = ? AND quest = ?", s.playerGUID, questID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_queststatus_daily WHERE guid = ? AND quest = ?", s.playerGUID, questID)
	}
	// WorldSession::HandleQuestLogRemoveQuest (QuestHandler.cpp:428): the
	// first Player::TakeQuestSourceItem call is a gate — an unequip failure
	// rejects the abandon before anything is removed.
	if !s.takeQuestSourceItem(ctx, questID, true) {
		return true
	}
	// Player::AbandonQuest (Player.cpp:15578-15598): BIND_QUEST_ITEM copies
	// of the required/source item lists are destroyed; the handler's PvP
	// re-arm is a provable no-op (C++ recomputes IsHostile while the quest
	// is still in the quest log).
	// AchievementMgr::RemoveTimedAchievement (AchievementMgr.cpp:1487-1501):
	// the abandoned quest's timed accept criteria lose their timers.
	// QuestHandler.cpp:443 — the second TakeQuestSourceItem pass (idempotent).
	s.takeQuestSourceItem(ctx, questID, true)
	s.destroyQuestItems(ctx, questID)
	// WorldSession::HandleQuestLogRemoveQuest (QuestHandler.cpp:434): a
	// timed quest leaves the timed set on abandon.
	s.removeTimedQuest(questID)
	s.removeTimedAchievements(timedTypeQuest, questID)
	// Eluna PLAYER_EVENT_ON_QUEST_ABANDON (38): Eluna::OnQuestAbandon
	// (PlayerHooks.cpp:101-107), (event, player, questId), void
	// (CallAllFunctions). CMSG_QUESTLOG_REMOVE_QUEST (QuestHandler.cpp:449)
	// fires it after AbandonQuest/RemoveActiveQuest/RemoveTimedAchievement.
	s.triggerPlayerEvent(ctx, scripting.PlayerEventQuestAbandon, s.luaPlayer(), questID)
	// WorldSession::HandleQuestLogRemoveQuest (QuestHandler.cpp:465): the
	// abandon also fires OnQuestStatusChange after AbandonQuest /
	// RemoveActiveQuest / RemoveTimedAchievement.
	s.triggerPlayerEvent(ctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), questID)
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		wdb := s.server.WorldStore.DB
		var entries []uint32
		rows, err := wdb.QueryContext(ctx, "SELECT id FROM creature_queststarter WHERE quest = ? UNION SELECT id FROM creature_questender WHERE quest = ?", questID, questID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var ent uint32
				if rows.Scan(&ent) == nil {
					entries = append(entries, ent)
				}
			}
		}
		for _, ent := range entries {
			status, err := s.questDialogStatus(ctx, ent)
			if err == nil {
				crows, err := wdb.QueryContext(ctx, "SELECT guid FROM creature WHERE id = ? AND map = ?", ent, s.player.Map)
				if err == nil {
					for crows.Next() {
						var spawnGUID uint32
						if crows.Scan(&spawnGUID) == nil {
							guid := creatureWorldGUID(spawnGUID, ent)
							packet := protocol.NewBuffer(9)
							packet.WriteU64(guid)
							packet.WriteU8(status)
							_ = s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS), packet.Bytes(), true)
						}
					}
					crows.Close()
				}
			}
		}
	}
	s.debug("quest abandoned", "account", s.accountName, "quest", questID, "slot", slot)
	return true
}

func (s *session) questgiverStartsQuest(ctx context.Context, guid uint64, questID uint32) bool {
	if guid == 0 {
		return true
	}
	high := uint16(guid >> 48)
	if high == 0xF110 {
		entry := uint32((guid >> 24) & 0x00FFFFFF)
		return questRelationExists(ctx, s.server.WorldStore.DB, "gameobject_queststarter", entry, questID)
	}
	// Item starting quest (HighGuid 0x4000 or item GUID)
	if high == 0x4000 {
		var itemEntry uint32
		itemLow := guid & 0x0000FFFFFFFFFFFF
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ? OR guid = ?", itemLow, guid).Scan(&itemEntry)
		}
		if itemEntry != 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var startQuest uint32
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT startquest FROM item_template WHERE entry = ?", itemEntry).Scan(&startQuest); err == nil && startQuest == questID {
				return true
			}
		}
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	if entry != 0 && s.creatureStartsQuest(ctx, entry, questID) {
		return true
	}
	creature := s.luaCreature(ctx, guid)
	if creature != nil {
		return s.creatureStartsQuest(ctx, objectUint32OrZero(creature, "Entry"), questID)
	}
	var spawnEntry uint32
	spawnGUID := uint32(guid & 0x00FFFFFF)
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", spawnGUID).Scan(&spawnEntry); err == nil && spawnEntry != 0 {
		return s.creatureStartsQuest(ctx, spawnEntry, questID)
	}
	// Also check item_template directly with entry
	if entry != 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var startQuest uint32
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT startquest FROM item_template WHERE entry = ?", entry).Scan(&startQuest); err == nil && startQuest == questID {
			return true
		}
	}
	return false
}

// questGiverObjectHasQuest mirrors the object gate of
// WorldSession::HandleQuestgiverQueryQuestOpcode (QuestHandler.cpp:239-244):
// the details query accepts TYPEMASK_UNIT | TYPEMASK_GAMEOBJECT | TYPEMASK_ITEM
// and answers unless the object neither starts nor is involved in the quest.
// Creature::hasInvolvedQuest / GameObject::hasInvolvedQuest read the
// *_questender relations (Creature.cpp:1856, GameObject.cpp:1202);
// Item::hasQuest is GetTemplate()->StartQuest == quest (Item.h:175).
// TYPEMASK_UNIT excludes TYPEMASK_PLAYER, so player GUIDs always close.
func (s *session) questGiverObjectHasQuest(ctx context.Context, guid uint64, questID uint32) bool {
	db := s.server.WorldStore.DB
	switch uint16(guid >> 48) {
	case 0x0000: // player GUID: TYPEMASK_UNIT excludes TYPEMASK_PLAYER
		return false
	case 0xF110: // gameobject
		entry := uint32((guid >> 24) & 0x00FFFFFF)
		return questRelationExists(ctx, db, "gameobject_queststarter", entry, questID) ||
			questRelationExists(ctx, db, "gameobject_questender", entry, questID)
	case 0x4000: // item
		var itemEntry uint32
		itemLow := guid & 0x0000FFFFFFFFFFFF
		if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return false
		}
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT itemEntry FROM item_instance WHERE guid = ? OR guid = ?", itemLow, guid).Scan(&itemEntry)
		if itemEntry == 0 {
			return false
		}
		var startQuest uint32
		if err := db.QueryRowContext(ctx, "SELECT startquest FROM item_template WHERE entry = ?", itemEntry).Scan(&startQuest); err != nil {
			return false
		}
		return startQuest == questID
	default: // unit
		creature := s.luaCreature(ctx, guid)
		var creatureEntry uint32
		if creature != nil {
			creatureEntry = objectUint32OrZero(creature, "Entry")
		} else {
			// Fallback: extract entry from packed GUID or look up spawn GUID
			creatureEntry = uint32((guid >> 24) & 0x00FFFFFF)
			if creatureEntry == 0 {
				spawnGUID := uint32(guid & 0x00FFFFFF)
				_ = db.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", spawnGUID).Scan(&creatureEntry)
			}
		}
		return creatureEntry != 0 && s.creatureHasQuest(ctx, creatureEntry, questID)
	}
}

func (s *session) sendGossipComplete() bool {
	s.gossip = nil
	s.gossipClosed = true
	return s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true) == nil
}

func (s *session) creatureHasQuest(ctx context.Context, entry, questID uint32) bool {
	return s.creatureStartsQuest(ctx, entry, questID) || s.creatureEndsQuest(ctx, entry, questID)
}

func (s *session) creatureStartsQuest(ctx context.Context, entry, questID uint32) bool {
	return questRelationExists(ctx, s.server.WorldStore.DB, "creature_queststarter", entry, questID)
}

func (s *session) creatureEndsQuest(ctx context.Context, entry, questID uint32) bool {
	return questRelationExists(ctx, s.server.WorldStore.DB, "creature_questender", entry, questID)
}

func questRelationExists(ctx context.Context, db *sql.DB, table string, entry, questID uint32) bool {
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(1) FROM "+table+" WHERE id = ? AND quest = ?", entry, questID).Scan(&count); err != nil {
		return false
	}
	return count != 0
}

func (s *session) loadQuestDetailData(ctx context.Context, questID uint32) (questDetailData, error) {
	var data questDetailData
	var title, objectives, details sql.NullString
	var flags, suggestedGroup, rewardMoney, rewardXP, rewardBonusMoney, rewardDisplaySpell, rewardHonor, rewardTitle, rewardTalents int64
	var rewardSpell, rewardArenaPoints int64
	var rewardKillHonor float64
	var rewardIDs [questRewardItems]int64
	var rewardCounts [questRewardItems]int64
	var choiceIDs [questChoiceItems]int64
	var choiceCounts [questChoiceItems]int64
	var factionIDs [questRewardFactions]int64
	var factionValues [questRewardFactions]int64
	var factionOverrides [questRewardFactions]int64
	columns := []string{"LogTitle", "LogDescription", "QuestDescription", "Flags", "SuggestedGroupNum", "RewardMoney", "RewardXPDifficulty", "RewardBonusMoney", "RewardDisplaySpell", "RewardSpell", "RewardHonor", "RewardKillHonor", "RewardTitle", "RewardTalents", "RewardArenaPoints"}
	targets := []any{&title, &objectives, &details, &flags, &suggestedGroup, &rewardMoney, &rewardXP, &rewardBonusMoney, &rewardDisplaySpell, &rewardSpell, &rewardHonor, &rewardKillHonor, &rewardTitle, &rewardTalents, &rewardArenaPoints}
	for index := 1; index <= questRewardItems; index++ {
		columns = append(columns, "RewardItem"+itoa(index), "RewardAmount"+itoa(index))
		targets = append(targets, &rewardIDs[index-1], &rewardCounts[index-1])
	}
	for index := 1; index <= questChoiceItems; index++ {
		columns = append(columns, "RewardChoiceItemID"+itoa(index), "RewardChoiceItemQuantity"+itoa(index))
		targets = append(targets, &choiceIDs[index-1], &choiceCounts[index-1])
	}
	for index := 1; index <= questRewardFactions; index++ {
		columns = append(columns, "RewardFactionID"+itoa(index), "RewardFactionValue"+itoa(index), "RewardFactionOverride"+itoa(index))
		targets = append(targets, &factionIDs[index-1], &factionValues[index-1], &factionOverrides[index-1])
	}
	query := "SELECT " + strings.Join(columns, ", ") + " FROM quest_template WHERE ID = ?"
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, query, questID).Scan(targets...); err != nil {
		// Fallback for minimal schema
		var minFlags int64
		var minTitle, minDesc sql.NullString
		if err2 := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(LogTitle, ''), Flags FROM quest_template WHERE ID = ?", questID).Scan(&minTitle, &minFlags); err2 == nil {
			data.ID = questID
			data.Title = minTitle.String
			data.Flags = uint32(minFlags)
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(LogDescription, Details, '') FROM quest_template WHERE ID = ?", questID).Scan(&minDesc)
			data.Details = minDesc.String
			return data, nil
		}
		return data, err
	}
	data.ID, data.Title, data.Objectives, data.Details = questID, title.String, objectives.String, details.String
	data.Flags, data.SuggestedGroupNum, data.RewardMoney, data.RewardXPDifficulty = uint32(flags), uint32(suggestedGroup), uint32(rewardMoney), uint32(rewardXP)
	if rewardMoney < 0 {
		// Negative RewardMoney is required money (Quest::GetRewOrReqMoney in
		// QuestDef.cpp:286); the RequestItems packet carries it as a positive
		// amount while the details/offer packets write the raw signed value.
		data.RequiredMoney = uint32(-rewardMoney)
	}
	data.RewardBonusMoney, data.RewardDisplaySpell, data.RewardSpell, data.RewardHonor = uint32(rewardBonusMoney), uint32(rewardDisplaySpell), int32(rewardSpell), uint32(rewardHonor)
	data.RewardKillHonor, data.RewardTitleID, data.RewardTalents, data.RewardArenaPoints = float32(rewardKillHonor), uint32(rewardTitle), uint32(rewardTalents), int32(rewardArenaPoints)
	for index, itemID := range rewardIDs {
		if itemID == 0 {
			continue
		}
		data.RewardItems = append(data.RewardItems, questRewardItem{ID: uint32(itemID), Quantity: uint32(rewardCounts[index]), DisplayID: s.itemDisplayID(ctx, uint32(itemID))})
	}
	for index, itemID := range choiceIDs {
		if itemID == 0 {
			continue
		}
		data.ChoiceItems = append(data.ChoiceItems, questRewardItem{ID: uint32(itemID), Quantity: uint32(choiceCounts[index]), DisplayID: s.itemDisplayID(ctx, uint32(itemID))})
	}
	for index := range data.RewardFactionID {
		data.RewardFactionID[index], data.RewardFactionValue[index], data.RewardFactionOverride[index] = uint32(factionIDs[index]), int32(factionValues[index]), int32(factionOverrides[index])
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT Emote1, Emote2, Emote3, Emote4, EmoteDelay1, EmoteDelay2, EmoteDelay3, EmoteDelay4 FROM quest_details WHERE ID = ?", questID)
	if err == nil {
		if rows.Next() {
			var types [questDetailEmotes]int64
			var delays [questDetailEmotes]int64
			dest := make([]any, 0, questDetailEmotes*2)
			for index := range types {
				dest = append(dest, &types[index])
			}
			for index := range delays {
				dest = append(dest, &delays[index])
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return data, err
			}
			for index := range data.DescEmotes {
				data.DescEmotes[index] = questDescEmote{Type: uint32(types[index]), Delay: uint32(delays[index])}
			}
		}
		rows.Close()
	} else if !missingTable(err) {
		return data, err
	}
	return data, nil
}

func (s *session) itemDisplayID(ctx context.Context, entry uint32) uint32 {
	var displayID int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT displayid FROM item_template WHERE entry = ?", entry).Scan(&displayID); err != nil {
		return 0
	}
	return uint32(displayID)
}

func buildQuestGiverDetails(data questDetailData, giverGUID, informGUID uint64) []byte {
	packet := protocol.NewBuffer(512)
	packet.WriteU64(giverGUID)
	packet.WriteU64(informGUID)
	packet.WriteU32(data.ID)
	packet.WriteCString(data.Title)
	packet.WriteCString(data.Details)
	packet.WriteCString(data.Objectives)
	packet.WriteU8(1)
	packet.WriteU32(data.Flags)
	packet.WriteU32(data.SuggestedGroupNum)
	packet.WriteU8(0)
	packet.WriteU32(uint32(len(data.ChoiceItems)))
	for _, item := range data.ChoiceItems {
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
		packet.WriteU32(item.DisplayID)
	}
	packet.WriteU32(uint32(len(data.RewardItems)))
	for _, item := range data.RewardItems {
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
		packet.WriteU32(item.DisplayID)
	}
	packet.WriteU32(data.RewardMoney)
	packet.WriteU32(data.RewardXPDifficulty)
	packet.WriteU32(data.RewardHonor)
	packet.WriteF32(data.RewardKillHonor)
	packet.WriteU32(data.RewardDisplaySpell)
	packet.WriteI32(data.RewardSpell)
	packet.WriteU32(data.RewardTitleID)
	packet.WriteU32(data.RewardTalents)
	packet.WriteI32(data.RewardArenaPoints)
	packet.WriteU32(0)
	for _, value := range data.RewardFactionID {
		packet.WriteU32(value)
	}
	for _, value := range data.RewardFactionValue {
		packet.WriteI32(value)
	}
	for _, value := range data.RewardFactionOverride {
		packet.WriteI32(value)
	}
	packet.WriteI32(int32(len(data.DescEmotes)))
	for _, emote := range data.DescEmotes {
		packet.WriteU32(emote.Type)
		packet.WriteU32(emote.Delay)
	}
	return packet.Bytes()
}

func itoa(value int) string {
	if value < 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	if digits == "" {
		return "0"
	}
	return digits
}
