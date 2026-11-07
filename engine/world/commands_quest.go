package world

import (
	"context"
	"fmt"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// quest command port: quest_commandscript (cs_quest.cpp), the "quest" root
// with 4 arms (add, complete, remove, reward). THIRTIETH of 40 Commands
// groups (cs_script_loader.cpp decl 49 / call 93; call order: npc(92) ->
// quest(93)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (RBAC.h:471-474). The root's own RBAC_PERM_COMMAND_QUEST (RBAC.h:470, 602)
// is DEAD in C++: the root uses the deprecated 6-arg ChatCommandBuilder
// overload (ChatCommand.h:262), which drops the RBACPermissions param and
// delegates to the sub-only constructor — so a bare ".quest" prints the
// syntax line with no gate ever checked (same dead-root-perm pattern as the
// mmap/modify/npc ports); permissionCommandQuest stays in permissions.go as
// documentation only.
//
// All four arms are native or partial-native on the Go quest bridges
// (quest_template, character_queststatus, addQuestToPlayer, completeQuest,
// loadQuestRewardView/commitQuestReward):
//
//   - `add` is native: template gate, quest-disabled gate, the
//     item-start-quest guard, the already-active guard, the CanAddQuest
//     gate (quest_details.go, log slot + source-item fit), then
//     AddQuestAndCheckCompletion via addQuestToPlayer.
//   - `remove` is native: template gate, status gate, quest-log slot clear,
//     character_queststatus + character_queststatus_rewarded deletes.
//   - `complete` is native: the gates, the objective-fill legs
//     (required-item top-up, kill credit, player-kill credit, reputation
//     objectives, required money), then the CompleteQuest transition.
//     Only the quest-tracker leg stays out: Go has no
//     CONFIG_QUEST_ENABLE_QUEST_TRACKER equivalent.
//   - `reward` is native: template gate, must-be-complete gate,
//     disabled gate, then the RewardQuest path via loadQuestRewardView +
//     commitQuestReward (no choice item, like the C++ choice-0 call).
//
// Console-vs-chat branches are moot (Go commands are always sessioned).
// LANG texts are inlined from the TDB enUS recall (no in-tree
// trinity_string seed).

// questStatusNoneValue is QUEST_STATUS_NONE (QuestDef.h): characterQuestStatus
// returns (0, nil) when the player has no row for the quest.
const questStatusNoneValue = 0

// parseQuestLink mirrors the extractKeyFromLink("Hquest") + atoul pattern in
// the quest arms (cs_quest.cpp:73): a plain entry id or a
// |Hquest:quest_id:quest_level|h[name]|h|r shift-click link. Only a token
// starting with '|' is link-parsed (Chat.cpp:362: text[0] != '|' returns the
// first token verbatim, so a bare "Hquest:123" reaches atoul and yields 0);
// a '|' token without an Hquest link is a wrong link type — the C++ sends
// LANG_WRONG_LINK_TYPE (511) and the handler returns false, so ok=false and
// the caller prints the arm syntax (the framework-help equivalent; the 511
// text is unseeded in-tree, not guessed). extractKeyFromLink only returns
// nullptr on empty text (impossible here — every arm gates on len(args)) or
// a totally malformed link; anything else, including "0" and "abc", reaches
// atoul and yields 0, so a zero entry flows to the template gate and its
// NOTFOUND message instead of failing silently.
func parseQuestLink(arg string) (uint32, bool) {
	if strings.HasPrefix(arg, "|") {
		i := strings.Index(arg, "Hquest:")
		if i < 0 {
			return 0, false
		}
		rest := arg[i+len("Hquest:"):]
		num := ""
		for _, r := range rest {
			if r < '0' || r > '9' {
				break
			}
			num += string(r)
		}
		if num == "" {
			return 0, true // atoul of an empty key is 0 -> template gate reports NOTFOUND
		}
		return uint32(cAtoi(num)), true
	}
	return uint32(cAtoi(arg)), true // atoul: "0"/"abc" -> 0 -> template gate
}

// questTemplateGate mirrors sObjectMgr->GetQuestTemplate: the entry must
// exist in quest_template.
func (s *session) questTemplateGate(ctx context.Context, entry uint32) bool {
	db := s.server.WorldStore.DB
	if db == nil {
		return false
	}
	var one int
	return db.QueryRowContext(ctx, "SELECT 1 FROM quest_template WHERE ID = ?", entry).Scan(&one) == nil
}

// questDisabled mirrors DisableMgr::IsDisabledFor(DISABLE_TYPE_QUEST, entry)
// via the disables table (disableTypeQuest = 1).
func (s *session) questDisabled(ctx context.Context, entry uint32) bool {
	db := s.server.WorldStore.DB
	if db == nil {
		return false
	}
	var one int
	return db.QueryRowContext(ctx, "SELECT 1 FROM disables WHERE sourceType = 1 AND entry = ?", entry).Scan(&one) == nil
}

// questTargetPlayerOrSelf mirrors getSelectedPlayerOrSelf (Chat.cpp:344-360):
// an unresolvable selection falls back to the invoker silently, never nil
// in-session (same bug class the modify port fixed).
func (s *session) questTargetPlayerOrSelf() *session {
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			target = ts
		}
	}
	if target.player == nil {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return nil
	}
	return target
}

// questTargetPlayer mirrors getSelectedPlayer (Chat.cpp:300) for the
// remove/reward arms: no selection resolves to the invoker; only an
// unresolvable selection yields null and LANG_NO_CHAR_SELECTED.
func (s *session) questTargetPlayer() *session {
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			return ts
		}
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
		return nil
	}
	if s.player == nil {
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
		return nil
	}
	return s
}

// handleCmdQuest dispatches the "quest" root (cs_quest.cpp:47-58). A bare
// ".quest" prints the syntax line with no permission gate: the root's own
// 602 is dead in C++ (ChatCommand.h:262 deprecated overload). The bool
// reports C++ handler success: the RBAC denial returns false silently so
// dispatchCommand shows the node's help listing (ChatCommand.cpp:296-309);
// paths that print a message return true.
func (s *session) handleCmdQuest(ctx context.Context, args []string) bool {
	const syntax = "Syntax: .quest add|complete|remove|reward <entry>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return true
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return true
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("add", sub):
		return s.handleQuestAdd(ctx, rest)
	case strings.HasPrefix("complete", sub):
		return s.handleQuestComplete(ctx, rest)
	case strings.HasPrefix("remove", sub):
		return s.handleQuestRemove(ctx, rest)
	case strings.HasPrefix("reward", sub):
		return s.handleQuestReward(ctx, rest)
	default:
		s.sendSysMessage(syntax)
	}
	return true
}

// handleQuestAdd mirrors HandleQuestAdd (cs_quest.cpp:61).
func (s *session) handleQuestAdd(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandQuestAdd) {
		return false
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest add <entry>")
		return true
	}
	target := s.questTargetPlayerOrSelf()
	if target == nil {
		return true
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .quest add <entry>")
		return true
	}
	if !s.questTemplateGate(ctx, entry) || s.questDisabled(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	// Item-start quests must come from the item (cs_quest.cpp:112).
	if wdb := s.server.WorldStore.DB; wdb != nil {
		var itemEntry uint32
		if err := wdb.QueryRowContext(ctx, "SELECT entry FROM item_template WHERE StartQuest = ? LIMIT 1", entry).Scan(&itemEntry); err == nil {
			s.sendSysMessage(fmt.Sprintf("Quest %d starts from item %d.", entry, itemEntry)) // LANG_COMMAND_QUEST_STARTFROMITEM 472
			return true
		}
	}
	if st, err := target.characterQuestStatus(ctx, entry); err == nil && st != int64(questStatusNoneValue) {
		return false // already active: C++ returns false silently
	}
	// CanAddQuest gate (cs_quest.cpp:105-106): a full log or an
	// un-storable source item blocks the add; the gate itself sends the
	// log-full / equip error (msg=true), so the handler stays silent here.
	if !target.canAddQuest(ctx, entry, true) {
		return true
	}
	target.addQuestToPlayer(ctx, entry, 0) // AddQuestAndCheckCompletion
	return true
}

// handleQuestRemove mirrors HandleQuestRemove (cs_quest.cpp:112).
func (s *session) handleQuestRemove(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandQuestRemove) {
		return false
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest remove <entry>")
		return true
	}
	target := s.questTargetPlayer()
	if target == nil {
		return true
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .quest remove <entry>")
		return true
	}
	if !s.questTemplateGate(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	st, err := target.characterQuestStatus(ctx, entry)
	if err != nil || st == int64(questStatusNoneValue) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	target.clearQuestLogEntries(ctx, entry)
	// The TakeQuestSourceItem, PvP-quest-flag and OnQuestStatusChange legs
	// ride along in clearQuestLogEntries (Spell::EffectQuestClear arm order).
	s.sendSysMessage("Quest removed.") // LANG_COMMAND_QUEST_REMOVED 473
	return true
}

// handleQuestComplete mirrors HandleQuestComplete (cs_quest.cpp:175): the
// gates, the objective-fill legs, then the CompleteQuest transition.
func (s *session) handleQuestComplete(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandQuestComplete) {
		return false
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest complete <entry>")
		return true
	}
	target := s.questTargetPlayerOrSelf()
	if target == nil {
		return true
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .quest complete <entry>")
		return true
	}
	if !s.questTemplateGate(ctx, entry) || s.questDisabled(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	st, err := target.characterQuestStatus(ctx, entry)
	if err != nil || st == int64(questStatusNoneValue) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	target.fillQuestCompleteObjectives(ctx, entry)
	target.completeQuest(ctx, entry) // Player::CompleteQuest
	return true
}

// fillQuestCompleteObjectives mirrors the objective-fill legs of
// HandleQuestComplete (cs_quest.cpp:198-272), in C++ order: required-item
// top-up, creature/GO kill credit, player-kill credit, reputation
// objectives, required money. The quest-tracker leg stays out: Go has no
// CONFIG_QUEST_ENABLE_QUEST_TRACKER equivalent.
func (s *session) fillQuestCompleteObjectives(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var itemIDs, itemCounts [6]int64
	var npcOrGo, npcOrGoCounts [4]int64
	var playerKills, repFac, repVal, repFac2, repVal2, reqMoney int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(RequiredItemId1, 0), COALESCE(RequiredItemId2, 0), COALESCE(RequiredItemId3, 0), COALESCE(RequiredItemId4, 0), COALESCE(RequiredItemId5, 0), COALESCE(RequiredItemId6, 0), COALESCE(RequiredItemCount1, 0), COALESCE(RequiredItemCount2, 0), COALESCE(RequiredItemCount3, 0), COALESCE(RequiredItemCount4, 0), COALESCE(RequiredItemCount5, 0), COALESCE(RequiredItemCount6, 0), COALESCE(RequiredNpcOrGo1, 0), COALESCE(RequiredNpcOrGo2, 0), COALESCE(RequiredNpcOrGo3, 0), COALESCE(RequiredNpcOrGo4, 0), COALESCE(RequiredNpcOrGoCount1, 0), COALESCE(RequiredNpcOrGoCount2, 0), COALESCE(RequiredNpcOrGoCount3, 0), COALESCE(RequiredNpcOrGoCount4, 0), COALESCE(RequiredPlayerKills, 0), COALESCE(RepObjectiveFaction, 0), COALESCE(RepObjectiveValue, 0), COALESCE(RepObjectiveFaction2, 0), COALESCE(RepObjectiveValue2, 0), COALESCE(RewOrReqMoney, 0) FROM quest_template WHERE ID = ?`, questID).Scan(&itemIDs[0], &itemIDs[1], &itemIDs[2], &itemIDs[3], &itemIDs[4], &itemIDs[5], &itemCounts[0], &itemCounts[1], &itemCounts[2], &itemCounts[3], &itemCounts[4], &itemCounts[5], &npcOrGo[0], &npcOrGo[1], &npcOrGo[2], &npcOrGo[3], &npcOrGoCounts[0], &npcOrGoCounts[1], &npcOrGoCounts[2], &npcOrGoCounts[3], &playerKills, &repFac, &repVal, &repFac2, &repVal2, &reqMoney); err != nil {
		return
	}
	// Required items (cs_quest.cpp:200-217): top the stack up to the
	// required count. ownedItemCount covers character_inventory rows,
	// bank slots included, like GetItemCount(id, true).
	itemsStored := false
	for i := 0; i < 6; i++ {
		if itemIDs[i] <= 0 || itemCounts[i] <= 0 {
			continue
		}
		cur := s.ownedItemCount(ctx, s.playerGUID, uint32(itemIDs[i]))
		if cur >= uint32(itemCounts[i]) {
			continue // C++ wraps count-cur to a huge uint32 and CanStore fails: same no-grant outcome
		}
		need := uint32(itemCounts[i]) - cur
		if s.canStoreNewItem(ctx, s.playerGUID, uint32(itemIDs[i]), need) != equipErrOk {
			continue
		}
		if _, err := s.storeOrStackItem(ctx, s.playerGUID, uint32(itemIDs[i]), need); err == nil {
			itemsStored = true
		}
	}
	if itemsStored {
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
	// Creature/GO objectives (cs_quest.cpp:220-235): one KilledMonster per
	// kill, one KillCreditGO per activation; the creature arm skips
	// entries with no creature template, like the C++ GetCreatureTemplate
	// gate. Kill-credit calls cap at the required count in
	// creditQuestObjectiveEntries, so the loop count is harmless.
	for i := 0; i < 4; i++ {
		if npcOrGo[i] == 0 || npcOrGoCounts[i] <= 0 {
			continue
		}
		if npcOrGo[i] > 0 {
			var exists int
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT 1 FROM creature_template WHERE entry = ?", npcOrGo[i]).Scan(&exists); err != nil {
				continue
			}
			for z := int64(0); z < npcOrGoCounts[i]; z++ {
				s.creditQuestKills(ctx, uint32(npcOrGo[i]), 0)
			}
		} else {
			for z := int64(0); z < npcOrGoCounts[i]; z++ {
				s.creditQuestGOActivation(ctx, uint32(-npcOrGo[i]), 0)
			}
		}
	}
	// Player kills (cs_quest.cpp:238-241): PLAYER_KILL is load-computed
	// from RequiredPlayerKills (ObjectMgr.cpp:5216), so the nonzero count
	// is the flag check.
	if playerKills > 0 {
		s.creditQuestPlayerKills(ctx, questID, uint32(playerKills))
	}
	// Reputation objectives (cs_quest.cpp:244-261): raise to the objective
	// value when below it, gated on a DBC faction row like the C++
	// sFactionStore.LookupEntry check.
	for _, leg := range [2][2]int64{{repFac, repVal}, {repFac2, repVal2}} {
		if leg[0] <= 0 || s.reputationStanding(uint32(leg[0])) >= int32(leg[1]) {
			continue
		}
		if s.server.Data != nil {
			if _, found, err := s.server.Data.Reputation(uint32(leg[0]), s.player.Race, s.player.Class); err != nil || !found {
				continue
			}
		}
		s.setReputationStanding(ctx, uint32(leg[0]), int32(leg[1]))
	}
	// Required money (cs_quest.cpp:264-267): GetRewOrReqMoney is raw
	// negative here, so ModifyMoney grants the requirement amount; the
	// tail mirrors the modify-money arm (persist, MoneyChanged scan,
	// money packet).
	if reqMoney < 0 {
		if s.modifyPlayerMoney(ctx, int32(-reqMoney)) {
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
			}
			s.questMoneyChanged(ctx)
			s.sendPlayerMoneyUpdate()
		}
	}
}

// creditQuestPlayerKills mirrors Player::KilledPlayerCreditForQuest
// (Player.cpp:16723-16746) for one quest: the log's PlayerCount rises to
// the required kill count (capped, never past it) with the
// SMSG_QUESTUPDATE_ADD_PVP_KILL packet. The CanCompleteQuest tail is left
// to the caller: HandleQuestComplete completes the quest directly below.
func (s *session) creditQuestPlayerKills(ctx context.Context, questID uint32, required uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || required == 0 {
		return
	}
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		entry := s.player.QuestLog[slot]
		if entry.QuestID != questID {
			continue
		}
		if uint32(entry.PlayerCount) >= required {
			return
		}
		entry.PlayerCount = uint16(required)
		entry.Counters[0] = entry.PlayerCount
		s.player.QuestLog[slot] = entry
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET playercount = ? WHERE guid = ? AND quest = ?", entry.PlayerCount, s.playerGUID, questID)
		packet := protocol.NewBuffer(12)
		packet.WriteU32(questID)
		packet.WriteU32(uint32(entry.PlayerCount))
		packet.WriteU32(required)
		_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_ADD_PVP_KILL), packet.Bytes(), true)
		s.sendPlayerQuestLogUpdate(slot)
		return
	}
}

// handleQuestReward mirrors HandleQuestReward (cs_quest.cpp:285): the quest
// must be complete, then the RewardQuest path runs with no choice item.
func (s *session) handleQuestReward(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandQuestReward) {
		return false
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest reward <entry>")
		return true
	}
	target := s.questTargetPlayer()
	if target == nil {
		return true
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		s.sendSysMessage("Syntax: .quest reward <entry>")
		return true
	}
	if !s.questTemplateGate(ctx, entry) || s.questDisabled(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	st, err := target.characterQuestStatus(ctx, entry)
	if err != nil || st != int64(questStatusComplete) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return true
	}
	view, err := target.loadQuestRewardView(ctx, entry)
	if err != nil {
		return true
	}
	// .quest reward runs the full Player::RewardQuest grant path (cs_quest.cpp:313),
	// shared with the turn-in packet: items, money, XP, honor, title,
	// talents, arena points, reputation, mail, and the completion packet.
	grants, destroyedGUIDs, err := target.commitQuestReward(ctx, view, 0)
	if err != nil {
		return true
	}
	target.applyQuestRewardGrants(ctx, view, entry, 0, 0, grants, destroyedGUIDs, true)
	return true
}
