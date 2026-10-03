package world

import (
	"context"
	"fmt"
	"strings"
)

// quest command port: quest_commandscript (cs_quest.cpp), the "quest" root
// with 4 arms (add, complete, remove, reward). THIRTIETH of 39 Commands
// groups (cs_script_loader.cpp decl 48 / call 93; call order re-verified this
// run: npc(92) -> quest(93)). Trinity checks permission only on the invoker
// leaf node (ChatCommand.cpp:487), so each arm gates exactly its own C++
// permission (RBAC.h:470-474); the root permission 602 covers the bare
// ".quest".
//
// All four arms are native or partial-native on the Go quest bridges
// (quest_template, character_queststatus, addQuestToPlayer, completeQuest,
// loadQuestRewardView/commitQuestReward):
//
//   - `add` is native: template gate, quest-disabled gate, the
//     item-start-quest guard, the already-active guard, then
//     AddQuestAndCheckCompletion via addQuestToPlayer.
//   - `remove` is native: template gate, status gate, quest-log slot clear,
//     character_queststatus + character_queststatus_rewarded deletes.
//   - `complete` is partial: the gates and the CompleteQuest transition are
//     native; the required-item grant, kill-credit, reputation and money
//     objective legs have no Go bridge, so a GM-completed quest flips to
//     complete without auto-granting its objectives.
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
// the quest arms (cs_quest.cpp:95): a plain entry id or a
// |Hquest:quest_id:quest_level|h[name]|h|r shift-click link.
func parseQuestLink(arg string) (uint32, bool) {
	if i := strings.Index(arg, "Hquest:"); i >= 0 {
		rest := arg[i+len("Hquest:"):]
		num := ""
		for _, r := range rest {
			if r < '0' || r > '9' {
				break
			}
			num += string(r)
		}
		if num == "" {
			return 0, false
		}
		return uint32(cAtoi(num)), true
	}
	v := cAtoi(arg)
	if v <= 0 {
		return 0, false
	}
	return uint32(v), true
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

// questTarget mirrors getSelectedPlayerOrSelf for the add/complete arms.
func (s *session) questTargetPlayerOrSelf() *session {
	target := s
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			target = ts
		} else {
			s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
			return nil
		}
	}
	if target.player == nil {
		s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
		return nil
	}
	return target
}

// questTargetPlayer mirrors getSelectedPlayer for the remove/reward arms: no
// self fallback, an unresolvable selection reports LANG_NO_CHAR_SELECTED.
func (s *session) questTargetPlayer() *session {
	if s.selection != 0 && s.server != nil {
		if ts := s.server.playerSessionForGUID(s.selection); ts != nil && ts.player != nil {
			return ts
		}
	}
	s.sendSysMessage("No character selected.") // LANG_NO_CHAR_SELECTED 116
	return nil
}

// handleCmdQuest dispatches the "quest" root (cs_quest.cpp:47-58).
func (s *session) handleCmdQuest(ctx context.Context, args []string) {
	const syntax = "Syntax: .quest add|complete|remove|reward <entry>"
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandQuest) {
			return
		}
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch {
	case strings.HasPrefix("add", sub):
		s.handleQuestAdd(ctx, rest)
	case strings.HasPrefix("complete", sub):
		s.handleQuestComplete(ctx, rest)
	case strings.HasPrefix("remove", sub):
		s.handleQuestRemove(ctx, rest)
	case strings.HasPrefix("reward", sub):
		s.handleQuestReward(ctx, rest)
	default:
		s.sendSysMessage(syntax)
	}
}

// handleQuestAdd mirrors HandleQuestAdd (cs_quest.cpp:78).
func (s *session) handleQuestAdd(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandQuestAdd) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest add <entry>")
		return
	}
	target := s.questTargetPlayerOrSelf()
	if target == nil {
		return
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		return
	}
	if !s.questTemplateGate(ctx, entry) || s.questDisabled(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	// Item-start quests must come from the item (cs_quest.cpp:112).
	if wdb := s.server.WorldStore.DB; wdb != nil {
		var itemEntry uint32
		if err := wdb.QueryRowContext(ctx, "SELECT entry FROM item_template WHERE StartQuest = ? LIMIT 1", entry).Scan(&itemEntry); err == nil {
			s.sendSysMessage(fmt.Sprintf("Quest %d starts from item %d.", entry, itemEntry)) // LANG_COMMAND_QUEST_STARTFROMITEM 472
			return
		}
	}
	if st, err := target.characterQuestStatus(ctx, entry); err == nil && st != int64(questStatusNoneValue) {
		return // already active: C++ returns false silently
	}
	target.addQuestToPlayer(ctx, entry) // AddQuestAndCheckCompletion
}

// handleQuestRemove mirrors HandleQuestRemove (cs_quest.cpp:130).
func (s *session) handleQuestRemove(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandQuestRemove) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest remove <entry>")
		return
	}
	target := s.questTargetPlayer()
	if target == nil {
		return
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		return
	}
	if !s.questTemplateGate(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	st, err := target.characterQuestStatus(ctx, entry)
	if err != nil || st == int64(questStatusNoneValue) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_queststatus WHERE guid = ? AND quest = ?", target.playerGUID, entry)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", target.playerGUID, entry)
	}
	if target.player != nil {
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if target.player.QuestLog[slot].QuestID == entry {
				target.player.QuestLog[slot] = questLogEntry{}
				target.sendPlayerQuestLogUpdate(slot)
			}
		}
	}
	// The TakeQuestSourceItem and PvP-quest legs have no Go bridge; the
	// quest-log and DB deletes are the full observable state.
	s.sendSysMessage("Quest removed.") // LANG_COMMAND_QUEST_REMOVED 473
}

// handleQuestComplete mirrors HandleQuestComplete (cs_quest.cpp:181): the
// gates and the CompleteQuest transition are native; the required-item grant,
// kill-credit, reputation and money objective legs have no Go bridge.
func (s *session) handleQuestComplete(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandQuestComplete) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest complete <entry>")
		return
	}
	target := s.questTargetPlayerOrSelf()
	if target == nil {
		return
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		return
	}
	if !s.questTemplateGate(ctx, entry) || s.questDisabled(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	st, err := target.characterQuestStatus(ctx, entry)
	if err != nil || st == int64(questStatusNoneValue) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	target.completeQuest(ctx, entry) // Player::CompleteQuest
}

// handleQuestReward mirrors HandleQuestReward (cs_quest.cpp:268): the quest
// must be complete, then the RewardQuest path runs with no choice item.
func (s *session) handleQuestReward(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandQuestReward) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .quest reward <entry>")
		return
	}
	target := s.questTargetPlayer()
	if target == nil {
		return
	}
	entry, ok := parseQuestLink(args[0])
	if !ok {
		return
	}
	if !s.questTemplateGate(ctx, entry) || s.questDisabled(ctx, entry) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	st, err := target.characterQuestStatus(ctx, entry)
	if err != nil || st != int64(questStatusComplete) {
		s.sendSysMessage(fmt.Sprintf("Quest %d not found.", entry)) // LANG_COMMAND_QUEST_NOTFOUND 471
		return
	}
	view, err := target.loadQuestRewardView(ctx, entry)
	if err != nil {
		return
	}
	_, _, _ = target.commitQuestReward(ctx, view, 0) // RewardQuest(quest, 0, player)
}
