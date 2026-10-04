package world

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// This file wires the ".npcbot" command family
// (src/server/game/AI/NpcBots/botcommands.cpp:97-125). Only the "add" arm
// is converted so far; it gives Recruit/AddBotFree their first real call
// site. The remaining arms (remove/spawn/move/delete/lookup/revive/
// reloadconfig/command/info/hide/unhide/show/recall/kill/suicide/distance/
// order) land in later units.
//
// The "add" arm is selection-driven in C++ (owner->GetSelectedUnit() must be
// a live uncontrolled npcbot creature). Go keeps no live-creature model, so
// the selection GUID is bridged at the entry level: HighGuid must be a
// creature type, the entry's creature_template.flags_extra must carry the
// NPCBOT mask (== Creature::IsNPCBot), and the entry's persisted owner must
// be 0 (== !GetBotAI()->GetBotOwnerGuid(); Go has no live BotAI state).

// npcbotCreatureFlagMask mirrors the npcbot gate in npcbotEntryBlocked
// (commands_npc.go:62): CREATURE_FLAG_EXTRA_NPCBOT|_NPCBOT_PET
// (CreatureData.h:63-64).
const npcbotCreatureFlagMask = 0x04000000 | 0x08000000

// handleCmdNpcBot dispatches the "npcbot" root (botcommands.cpp:124-126).
func (s *session) handleCmdNpcBot(ctx context.Context, args []string) {
	const syntax = "Syntax: .npcbot add"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here (commands_npc.go convention).
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("add", sub):
		s.handleNpcBotAddCommand(ctx)
	default:
		s.sendSysMessage(syntax)
	}
}

// handleNpcBotAddCommand mirrors HandleNpcBotAddCommand (botcommands.cpp:1356):
// hires the selected uncontrolled npcbot through BotMgr::AddBot(bot, false),
// the takeMoney=false arm == NPCBotManager.AddBotFree (the C++ command never
// passes takeMoney=true; the paid path belongs to the gossip-hire flow).
func (s *session) handleNpcBotAddCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCBotAdd) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	// C++: Unit* cre = owner->GetSelectedUnit(); no cre or not TYPEID_UNIT →
	// ".npcbot add" + "Allows to hire selected uncontrolled bot".
	sel := s.selection
	if sel == 0 {
		s.npcbotAddUsage()
		return
	}
	switch uint16(sel >> 48) {
	case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
	default:
		s.npcbotAddUsage()
		return
	}
	entry := uint32((sel >> 24) & 0x00FFFFFF)
	// C++: !bot->IsNPCBot() || bot->GetBotAI()->GetBotOwnerGuid() →
	// "You must select uncontrolled npcbot".
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok || flagsExtra&npcbotCreatureFlagMask == 0 {
		s.sendSysMessage("You must select uncontrolled npcbot")
		return
	}
	if data, ok := mgr.Get(entry); !ok || data.Owner != 0 {
		s.sendSysMessage("You must select uncontrolled npcbot")
		return
	}
	result, err := mgr.AddBotFree(ctx, uint32(s.playerGUID), entry)
	if err != nil {
		s.debug("npcbot add failed", "account", s.accountName, "entry", entry, "error", err)
	}
	// C++ checks only AddBot(...) == BOT_ADD_SUCCESS; every other result
	// (including the silent ALREADY_HAVE arm) answers the same failure line.
	if err != nil || result != BotAddSuccess {
		s.sendSysMessage("NpcBot is NOT added for some reason!")
		return
	}
	s.sendSysMessage(fmt.Sprintf("%s is now your npcbot", s.npcbotTemplateName(ctx, entry)))
}

func (s *session) npcbotAddUsage() {
	s.sendSysMessage(".npcbot add")
	s.sendSysMessage("Allows to hire selected uncontrolled bot")
}

// npcbotTemplateName reads the creature_template display name
// (== Creature::GetName() on the hired bot).
func (s *session) npcbotTemplateName(ctx context.Context, entry uint32) string {
	if db := s.npcWorldDB(); db != nil {
		var name sql.NullString
		if err := db.QueryRowContext(ctx, "SELECT name FROM creature_template WHERE entry = ?", entry).Scan(&name); err == nil && name.Valid && name.String != "" {
			return name.String
		}
	}
	return fmt.Sprintf("Npcbot %d", entry)
}
