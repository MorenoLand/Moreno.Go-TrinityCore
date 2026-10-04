package world

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
)

// This file wires the ".npcbot" command family
// (src/server/game/AI/NpcBots/botcommands.cpp:97-125). The "add", "remove" and
// "spawn" arms are converted; they give Recruit/AddBotFree/Add their first
// real call sites. The remaining arms (move/delete/lookup/revive/
// reloadconfig/command/info/hide/unhide/show/recall/kill/suicide/distance/
// order) land in later units.
//
// The "add"/"remove" arms are selection-driven in C++ (owner->GetSelectedUnit()
// must be a live uncontrolled/controlled npcbot creature). Go keeps no
// live-creature model, so the selection GUID is bridged at the entry level:
// HighGuid must be a creature type, the entry's creature_template.flags_extra
// must carry the NPCBOT mask (== Creature::IsNPCBot), and the entry's persisted
// owner must be 0 (== !GetBotAI()->GetBotOwnerGuid(); Go has no live BotAI state).

// npcbotCreatureFlagMask mirrors the npcbot gate in npcbotEntryBlocked
// (commands_npc.go:62): CREATURE_FLAG_EXTRA_NPCBOT|_NPCBOT_PET
// (CreatureData.h:63-64).
const npcbotCreatureFlagMask = 0x04000000 | 0x08000000

// Base player classes (SharedDefines.h), needed by the default roles/spec
// mirrors. The custom bot classes live in npcbots.go (BotClassBlademaster etc.).
const (
	npcBotClassWarrior    = uint8(1)
	npcBotClassPaladin    = uint8(2)
	npcBotClassHunter     = uint8(3)
	npcBotClassRogue      = uint8(4)
	npcBotClassPriest     = uint8(5)
	npcBotClassShaman     = uint8(7)
	npcBotClassMage       = uint8(8)
	npcBotClassWarlock    = uint8(9)
	npcBotClassDruid      = uint8(11)
	npcBotSpecDefault     = uint8(31)          // BOT_SPEC_DEFAULT (botcommon.h:799)
	npcBotSpawnFlagNPCBot = uint32(0x04000000) // CREATURE_FLAG_EXTRA_NPCBOT (CreatureData.h:63)
)

// handleCmdNpcBot dispatches the "npcbot" root (botcommands.cpp:124-126).
func (s *session) handleCmdNpcBot(ctx context.Context, args []string) {
	const syntax = "Syntax: .npcbot add|remove|spawn"
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
	rest := args[1:]
	switch {
	case strings.HasPrefix("add", sub):
		s.handleNpcBotAddCommand(ctx)
	case strings.HasPrefix("remove", sub):
		s.handleNpcBotRemoveCommand(ctx)
	case strings.HasPrefix("spawn", sub):
		s.handleNpcBotSpawnCommand(ctx, rest)
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

// handleNpcBotRemoveCommand mirrors HandleNpcBotRemoveCommand
// (botcommands.cpp:1254): dismisses the selected controlled npcbot
// (BotMgr::RemoveBot(guid, BOT_REMOVE_DISMISS)), or dismisses all bots owned
// by the selected player (BotMgr::RemoveAllBots(BOT_REMOVE_DISMISS)).
// The bridgeable DB effect of RemoveBot with BOT_REMOVE_DISMISS is
// BotDataMgr::UpdateNpcBotData(entry, NPCBOT_UPDATE_OWNER, 0)
// (botmgr.cpp:820); the live legs (CleanupsBeforeBotDelete group/unsummon
// removal, ResetBotAI, faction/level restore, RemoveFromWorld) have no Go
// bridge — Go keeps no live BotAI or group-membership model for bots.
func (s *session) handleNpcBotRemoveCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCBotRemove) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	sel := s.selection
	if sel == 0 {
		s.sendSysMessage(".npcbot remove")
		s.sendSysMessage("Frees selected npcbot from it's owner. Select player to remove all npcbots")
		return
	}
	// C++: Player* master = u->ToPlayer() (HighGuid::Player == 0x0000).
	if uint16(sel>>48) == 0x0000 {
		// C++: master->RemoveAllBots(BOT_REMOVE_DISMISS), then
		// !master->HaveBot() → success. The owner counter convention is
		// uint32(guid), matching the add arm's uint32(s.playerGUID).
		owner := uint32(sel)
		if mgr.CountByOwner(owner) == 0 {
			s.sendSysMessage("Npcbots are not found!")
			return
		}
		if err := mgr.UpdateOwnerAll(ctx, owner, 0); err != nil {
			s.debug("npcbot remove all failed", "account", s.accountName, "owner", owner, "error", err)
			s.sendSysMessage("Some npcbots were not removed!")
			return
		}
		if mgr.CountByOwner(owner) != 0 {
			s.sendSysMessage("Some npcbots were not removed!")
			return
		}
		s.sendSysMessage("Npcbots were successfully removed")
		return
	}
	// C++: Creature* cre = u->ToCreature(); requires IsNPCBot() and
	// !IsFreeBot() (== persisted owner set); otherwise falls through to the
	// "You must select player or controlled npcbot" line.
	var high = uint16(sel >> 48)
	if high != 0xF130 && high != 0xF140 && high != 0xF150 {
		s.sendSysMessage("You must select player or controlled npcbot")
		return
	}
	entry := uint32((sel >> 24) & 0x00FFFFFF)
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok || flagsExtra&npcbotCreatureFlagMask == 0 {
		s.sendSysMessage("You must select player or controlled npcbot")
		return
	}
	data, ok := mgr.Get(entry)
	if !ok || data.Owner == 0 {
		s.sendSysMessage("You must select player or controlled npcbot")
		return
	}
	// C++: master->GetBotMgr()->RemoveBot(cre->GetGUID(), BOT_REMOVE_DISMISS),
	// then GetBot(guid) == nullptr → "NpcBot successfully removed".
	if err := mgr.Update(ctx, entry, NpcBotUpdateOwner, uint32(0)); err != nil {
		s.debug("npcbot remove failed", "account", s.accountName, "entry", entry, "error", err)
		s.sendSysMessage("NpcBot was NOT removed for some stupid reason!")
		return
	}
	if data, ok := mgr.Get(entry); !ok || data.Owner != 0 {
		s.sendSysMessage("NpcBot was NOT removed for some stupid reason!")
		return
	}
	s.sendSysMessage("NpcBot successfully removed")
}

// handleNpcBotSpawnCommand mirrors HandleNpcBotSpawnCommand
// (botcommands.cpp:979): registers a new npcbot spawn of the given entry at
// the handler's position. Creature visibility is DB-driven (the commands_npc.go
// convention), so the creature-row insert IS the spawn; the live legs
// (Creature::Create, LoadBotCreatureFromDB, AddCreatureToGrid) have no Go
// bridge. The transport and instance gates (botcommands.cpp:1044/1058) have no
// bridge either — Go keeps no player-transport or live-map model.
func (s *session) handleNpcBotSpawnCommand(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCBotSpawn) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	raw := strings.TrimSpace(strings.Join(args, " "))
	if raw == "" {
		s.sendSysMessage(".npcbot spawn")
		s.sendSysMessage("Adds new npcbot spawn of given entry in world. You can shift-link the npc")
		s.sendSysMessage("Syntax: .npcbot spawn #entry")
		return
	}
	// C++: handler->extractKeyFromLink(args, "Hcreature_entry"); null →
	// silent false. A non-link first token is used verbatim; a wrong link
	// type yields no key (Chat.cpp:362).
	key := extractNpcBotCreatureKey(raw)
	if key == "" {
		return
	}
	id := uint32(cAtoi(key))
	flagsExtra, ok := s.npcTemplateGate(ctx, id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf("creature %d does not exist!", id))
		return
	}
	if flagsExtra&npcBotSpawnFlagNPCBot == 0 {
		s.sendSysMessage(fmt.Sprintf("creature %d is not a npcbot!", id))
		return
	}
	// C++: BotDataMgr::SelectNpcBotData(id) → "already exists in
	// `characters_npcbot` table".
	if _, ok := mgr.Get(id); ok {
		s.sendSysMessage(fmt.Sprintf("Npcbot %d already exists in `characters_npcbot` table!", id))
		s.sendSysMessage("If you want to move this bot to a new location use '.npcbot move' command")
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	// C++: WORLD_SEL_CREATURE_BY_ID ("SELECT guid FROM creature WHERE id = ?").
	var existing uint32
	if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE id = ?", id).Scan(&existing); err != nil && err != sql.ErrNoRows {
		return
	} else if err == nil {
		s.sendSysMessage(fmt.Sprintf("Npcbot %d already exists in `creature` table!", id))
		return
	}
	// C++: chr->GetTransport() → "Cannot spawn bots on transport!"; no Go
	// bridge (documented above).
	// C++: map->Instanceable() → "Cannot spawn bots in instances!"; no Go
	// bridge (documented above).
	// C++: NpcBotExtras const* _botExtras = BotDataMgr::SelectNpcBotExtras(id).
	extras, ok := mgr.Extras(id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf("No class/race data found for bot %d!", id))
		return
	}
	// C++: BotDataMgr::AddNpcBotData(id, bot_ai::DefaultRolesForClass(bclass),
	// bot_ai::DefaultSpecForClass(bclass), creature->GetCreatureTemplate()->faction).
	var faction uint32
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(faction, 0) FROM creature_template WHERE entry = ?", id).Scan(&faction); err != nil {
		return
	}
	if err := mgr.Add(ctx, id, defaultNpcBotRoles(extras.Class), defaultNpcBotSpec(extras.Class), faction); err != nil {
		s.debug("npcbot spawn add failed", "account", s.accountName, "entry", id, "error", err)
		return
	}
	// C++: creature->SaveToDB(map->GetId(), (1 << map->GetSpawnMode()),
	// chr->GetPhaseMaskForSpawn()). spawnMask 1 and phaseMask 1 match the
	// `.npc add` insert convention (commands_npc.go:217); spawntimesecs 120
	// mirrors the world DB column default for fresh rows.
	var guid uint32
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM creature").Scan(&guid); err != nil {
		return
	}
	p := s.player
	if _, err := db.ExecContext(ctx,
		"INSERT INTO creature (guid, id, map, spawnMask, phaseMask, position_x, position_y, position_z, orientation, spawntimesecs, MovementType) VALUES (?, ?, ?, 1, 1, ?, ?, ?, ?, 120, 0)",
		guid, id, p.Map, p.X, p.Y, p.Z, p.Orientation); err != nil {
		s.debug("npcbot spawn insert failed", "account", s.accountName, "entry", id, "error", err)
		return
	}
	s.sendSysMessage("NpcBot successfully spawned")
}

// extractNpcBotCreatureKey mirrors ChatHandler::extractKeyFromLink(text,
// "Hcreature_entry") (Chat.cpp:362): leading spaces skipped, a non-link first
// token returned verbatim, a |color|Hcreature_entry:key|h[name]|h|r link
// returning the key, and a wrong link type returning no key (silent C++
// false).
func extractNpcBotCreatureKey(raw string) string {
	raw = strings.TrimLeft(raw, " \t\b")
	if raw == "" {
		return ""
	}
	if raw[0] != '|' {
		if i := strings.IndexByte(raw, ' '); i >= 0 {
			return raw[:i]
		}
		return raw
	}
	segments := strings.Split(raw, "|")
	if len(segments) < 3 || segments[1] == "" {
		return ""
	}
	link := segments[2]
	if !strings.HasPrefix(link, "Hcreature_entry:") {
		return ""
	}
	return strings.SplitN(strings.TrimPrefix(link, "Hcreature_entry:"), ":", 2)[0]
}

// defaultNpcBotRoles mirrors bot_ai::DefaultRolesForClass (bot_ai.cpp:9938):
// DPS always, RANGED unless a melee class, HEAL for healing classes.
func defaultNpcBotRoles(class uint8) uint16 {
	roles := BotRoleDPS
	if !npcBotIsMeleeClass(class) {
		roles |= BotRoleRanged
	}
	if npcBotIsHealingClass(class) {
		roles |= BotRoleHeal
	}
	return roles
}

// npcBotIsMeleeClass mirrors bot_ai::IsMeleeClass (bot_ai.cpp:12359).
func npcBotIsMeleeClass(class uint8) bool {
	switch class {
	case npcBotClassWarrior, npcBotClassRogue, npcBotClassPaladin,
		BotClassDeathKnight, BotClassBlademaster, BotClassDreadlord,
		BotClassSpellbreaker:
		return true
	}
	return false
}

// npcBotIsHealingClass mirrors bot_ai::IsHealingClass (bot_ai.cpp:12384).
func npcBotIsHealingClass(class uint8) bool {
	switch class {
	case npcBotClassPriest, npcBotClassDruid, npcBotClassShaman,
		npcBotClassPaladin, BotClassObsidianDestroyer:
		return true
	}
	return false
}

// defaultNpcBotSpec mirrors bot_ai::DefaultSpecForClass (bot_ai.cpp:10290):
// a random tree (urand(1,3)) inside the class's spec block, BOT_SPEC_DEFAULT
// for the custom classes that have no spec block.
func defaultNpcBotSpec(class uint8) uint8 {
	spec := uint8(1 + rand.Intn(3))
	switch class {
	case npcBotClassWarrior, npcBotClassPaladin, npcBotClassHunter,
		npcBotClassRogue, npcBotClassPriest, BotClassDeathKnight,
		npcBotClassShaman, npcBotClassMage, npcBotClassWarlock:
		spec += (class - 1) * 3
	case npcBotClassDruid:
		spec += (class - 2) * 3
	default:
		spec = npcBotSpecDefault
	}
	return spec
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
