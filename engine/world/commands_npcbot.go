package world

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// This file wires the ".npcbot" command family
// (src/server/game/AI/NpcBots/botcommands.cpp:97-125). The "add", "remove",
// "spawn", "move", "delete", "lookup" and "revive" arms are converted; the
// first five give Recruit/AddBotFree/Add their first real call sites. The
// remaining arms (reloadconfig/command/info/hide/unhide/show/recall/kill/
// suicide/distance/order) land in later units.
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

	// Bot template entry range and class/race bounds for the lookup arm.
	npcBotEntryBegin       = uint32(70001) // BOT_ENTRY_BEGIN (botcommon.h:13)
	npcBotEntryEnd         = uint32(71000) // BOT_ENTRY_END (botcommon.h:14)
	npcBotEntryMirrorImage = uint32(70552) // BOT_ENTRY_MIRROR_IMAGE_BM (botcommon.h:17)
	npcBotClassEnd         = uint8(18)     // BOT_CLASS_END (botcommon.h:718)
	npcBotRaceMax          = uint8(12)     // MAX_RACES (SharedDefines.h:110)
)

// Player races (SharedDefines.h:84-110), needed by the lookup arm's label
// switch. RACE_UNDEAD_PLAYER is labeled "Forsaken" == the C++ switch.
const (
	npcBotRaceNone     = uint8(0)  // RACE_NONE
	npcBotRaceHuman    = uint8(1)  // RACE_HUMAN
	npcBotRaceOrc      = uint8(2)  // RACE_ORC
	npcBotRaceDwarf    = uint8(3)  // RACE_DWARF
	npcBotRaceNightElf = uint8(4)  // RACE_NIGHTELF
	npcBotRaceUndead   = uint8(5)  // RACE_UNDEAD_PLAYER
	npcBotRaceTauren   = uint8(6)  // RACE_TAUREN
	npcBotRaceGnome    = uint8(7)  // RACE_GNOME
	npcBotRaceTroll    = uint8(8)  // RACE_TROLL
	npcBotRaceBloodElf = uint8(10) // RACE_BLOODELF
	npcBotRaceDraenei  = uint8(11) // RACE_DRAENEI
)

// handleCmdNpcBot dispatches the "npcbot" root (botcommands.cpp:124-126).
func (s *session) handleCmdNpcBot(ctx context.Context, args []string) {
	const syntax = "Syntax: .npcbot add|remove|spawn|move|delete|lookup|revive"
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
	case strings.HasPrefix("move", sub):
		s.handleNpcBotMoveCommand(ctx, rest)
	case strings.HasPrefix("delete", sub):
		s.handleNpcBotDeleteCommand(ctx)
	case strings.HasPrefix("lookup", sub):
		s.handleNpcBotLookupCommand(ctx, rest)
	case strings.HasPrefix("revive", sub):
		s.handleNpcBotReviveCommand(ctx)
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

// handleNpcBotMoveCommand mirrors HandleNpcBotMoveCommand
// (botcommands.cpp:908): relocates the npcbot's spawn point to the handler's
// position, persisting the move with the same UPDATE creature statement
// (position_x/y/z, orientation, map WHERE guid). The live legs (FindBot,
// CreatureData spawn-point relocate, TeleportBot for free in-world bots) have
// no Go bridge — Go keeps no live BotAI or creature-object model, so the DB
// row IS the spawn point.
func (s *session) handleNpcBotMoveCommand(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCBotMove) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	raw := strings.TrimSpace(strings.Join(args, " "))
	// C++: handler->getSelectedCreature(); no args and no creature → the
	// three usage lines. Go's selection holds the full GUID; 0xF130 is the
	// creature HighGuid == GetSelectedCreature's Creature* return.
	var selEntry uint32
	if uint16(s.selection>>48) == 0xF130 {
		selEntry = uint32((s.selection >> 24) & 0x00FFFFFF)
	}
	if raw == "" && selEntry == 0 {
		s.sendSysMessage(".npcbot move")
		s.sendSysMessage("Moves npcbot to your location")
		s.sendSysMessage("Syntax: .npcbot move [#ID]")
		return
	}
	// C++: extractKeyFromLink(args, "Hcreature_entry"); wrong link type or
	// empty with no creature → silent false. A present key selects the args
	// ID even when it is "0" (C++ then reports "does not exist").
	var id uint32
	hasKey := false
	if raw != "" {
		if key := extractNpcBotCreatureKey(raw); key != "" {
			id, hasKey = uint32(cAtoi(key)), true
		}
		if !hasKey && selEntry == 0 {
			return
		}
		if !hasKey {
			id = selEntry
		}
	} else {
		id = selEntry
	}
	flagsExtra, ok := s.npcTemplateGate(ctx, id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf("creature id %d does not exist!", id))
		return
	}
	if flagsExtra&npcBotSpawnFlagNPCBot == 0 {
		s.sendSysMessage(fmt.Sprintf("creature id %d is not a npcbot!", id))
		return
	}
	// C++: BotDataMgr::SelectNpcBotData(id) → "NpcBot %u is not spawned!".
	if _, ok := mgr.Get(id); !ok {
		s.sendSysMessage(fmt.Sprintf("NpcBot %d is not spawned!", id))
		return
	}
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	// C++: BotDataMgr::FindBot(id) → ASSERT + CreatureData lowguid. Go uses
	// the world DB row the same way the spawn arm inserted it; a missing
	// row takes the same "not spawned" exit C++ would never reach.
	var guid uint32
	if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE id = ? LIMIT 1", id).Scan(&guid); err != nil {
		s.sendSysMessage(fmt.Sprintf("NpcBot %d is not spawned!", id))
		return
	}
	// C++: WorldDatabase.PExecute("UPDATE creature SET position_x = %.3f,
	// ... orientation = %.3f, map = %u WHERE guid = %u", ...).
	p := s.player
	if _, err := db.ExecContext(ctx,
		"UPDATE creature SET position_x = ?, position_y = ?, position_z = ?, orientation = ?, map = ? WHERE guid = ?",
		p.X, p.Y, p.Z, p.Orientation, p.Map, guid); err != nil {
		s.debug("npcbot move failed", "account", s.accountName, "entry", id, "error", err)
		return
	}
	s.sendSysMessage(fmt.Sprintf("NpcBot %d (guid %d) was moved", id, guid))
}

// handleNpcBotDeleteCommand mirrors HandleNpcBotDeleteCommand
// (botcommands.cpp:860): deletes the selected npcbot's spawn from the world
// and the DB. The bridgeable effects are RemoveBot(BOT_REMOVE_DISMISS) →
// owner=0 (BotDataMgr::UpdateNpcBotData, botmgr.cpp:820), UnEquipAll → the
// equips column zeroing (NpcBotUpdateEquips), Creature::DeleteFromDB(spawnId)
// → DELETE FROM creature, and UpdateNpcBotData(NPCBOT_UPDATE_ERASE) →
// NpcBotUpdateErase. The live legs (CombatStop, botAI Reset/canUpdate=false,
// AddObjectToRemoveList, the receiver choice for the unequipped gear) have no
// Go bridge — Go keeps no live BotAI, inventory, or remove-list model.
func (s *session) handleNpcBotDeleteCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCBotDelete) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	// C++: chr->GetSelectedUnit(); null → the two usage lines.
	if s.selection == 0 {
		s.sendSysMessage(".npcbot delete")
		s.sendSysMessage("Deletes selected npcbot spawn from world and DB")
		return
	}
	// C++: ToCreature() null or !IsNPCBot() → "No npcbot selected".
	switch uint16(s.selection >> 48) {
	case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
	default:
		s.sendSysMessage("No npcbot selected")
		return
	}
	entry := uint32((s.selection >> 24) & 0x00FFFFFF)
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok || flagsExtra&npcbotCreatureFlagMask == 0 {
		s.sendSysMessage("No npcbot selected")
		return
	}
	data, ok := mgr.Get(entry)
	if !ok {
		s.sendSysMessage("No npcbot selected")
		return
	}
	// C++: !bot->GetBotAI()->UnEquipAll(receiver) → the unequip-failure
	// line. The only bridgeable failure is the DB write; Go has no
	// per-item unequip model, so every item always unequips.
	if err := mgr.Update(ctx, entry, NpcBotUpdateEquips, [BotInventorySize]uint32{}); err != nil {
		s.debug("npcbot delete unequip failed", "account", s.accountName, "entry", entry, "error", err)
		s.sendSysMessage(fmt.Sprintf("%s is unable to unequip some gear. Please remove equips before deleting bot!", s.npcbotTemplateName(ctx, entry)))
		return
	}
	// C++: botowner->GetBotMgr()->RemoveBot(bot->GetGUID(),
	// BOT_REMOVE_DISMISS) — the persisted leg is the owner=0 update
	// (botmgr.cpp:820), the same arm the remove command uses.
	if data.Owner != 0 {
		if err := mgr.Update(ctx, entry, NpcBotUpdateOwner, uint32(0)); err != nil {
			s.debug("npcbot delete dismiss failed", "account", s.accountName, "entry", entry, "error", err)
		}
	}
	// C++: Creature::DeleteFromDB(bot->GetSpawnId()).
	db := s.npcWorldDB()
	if db != nil {
		var guid uint32
		if err := db.QueryRowContext(ctx, "SELECT guid FROM creature WHERE id = ? LIMIT 1", entry).Scan(&guid); err == nil {
			if _, err := db.ExecContext(ctx, "DELETE FROM creature WHERE guid = ?", guid); err != nil {
				s.debug("npcbot delete spawn failed", "account", s.accountName, "entry", entry, "error", err)
			}
		}
	}
	// C++: BotDataMgr::UpdateNpcBotData(bot->GetEntry(), NPCBOT_UPDATE_ERASE).
	if err := mgr.Update(ctx, entry, NpcBotUpdateErase, nil); err != nil {
		s.debug("npcbot delete erase failed", "account", s.accountName, "entry", entry, "error", err)
		return
	}
	s.sendSysMessage("Npcbot successfully deleted")
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

// npcbotPlayerName reads the player name == Unit::GetName() on the selected
// master (the revive arm's "%s has no npcbots!" line).
func (s *session) npcbotPlayerName(ctx context.Context, guid uint32) string {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var name sql.NullString
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid).Scan(&name); err == nil && name.Valid && name.String != "" {
			return name.String
		}
	}
	return fmt.Sprintf("Player %d", guid)
}

// handleNpcBotLookupCommand mirrors HandleNpcBotLookupCommand
// (botcommands.cpp:727): lists npcbot template entries of a bot class,
// ascending by entry, with the race label per entry. BotDataMgr::
// SelectNpcBotExtras (== NPCBotManager.Extras) supplies the class and race;
// creature_template supplies the names. Creature-locale names have no Go
// bridge (no session db-locale model; Go carries enUS text only — the chat.go
// convention), so the template name is always used; empty names are skipped
// == C++.
func (s *session) handleNpcBotLookupCommand(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCBotLookup) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	if len(args) == 0 {
		s.sendSysMessage(".npcbot lookup #class")
		s.sendSysMessage("Looks up npcbots by #class, and returns all matches with their creature ID's")
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_WARRIOR = %d", npcBotClassWarrior))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_PALADIN = %d", npcBotClassPaladin))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_HUNTER = %d", npcBotClassHunter))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_ROGUE = %d", npcBotClassRogue))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_PRIEST = %d", npcBotClassPriest))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_DEATH_KNIGHT = %d", BotClassDeathKnight))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_SHAMAN = %d", npcBotClassShaman))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_MAGE = %d", npcBotClassMage))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_WARLOCK = %d", npcBotClassWarlock))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_DRUID = %d", npcBotClassDruid))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_BLADEMASTER = %d", BotClassBlademaster))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_SPHYNX = %d", BotClassObsidianDestroyer))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_ARCHMAGE = %d", BotClassArchmage))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_DREADLORD = %d", BotClassDreadlord))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_SPELLBREAKER = %d", BotClassSpellbreaker))
		s.sendSysMessage(fmt.Sprintf("BOT_CLASS_DARK_RANGER = %d", BotClassDarkRanger))
		return
	}
	// C++: strtok(args, " ") takes the first token; (uint8)atoi classifies it.
	botclass := uint8(cAtoi(args[0]))
	if botclass == 0 || botclass >= npcBotClassEnd {
		s.sendSysMessage(fmt.Sprintf("Unknown bot class %d", botclass))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Looking for bots of class %d...", botclass))
	db := s.npcWorldDB()
	if db == nil {
		return
	}
	type npcBotLookupHit struct {
		id   uint32
		name string
		race uint8
	}
	var hits []npcBotLookupHit
	// C++ iterates sObjectMgr->GetCreatureTemplates() (entry-ordered) with
	// the [BOT_ENTRY_BEGIN, BOT_ENTRY_END] gate; ORDER BY entry reproduces it.
	rows, err := db.QueryContext(ctx, "SELECT entry, COALESCE(name, '') FROM creature_template WHERE entry BETWEEN ? AND ? ORDER BY entry", npcBotEntryBegin, npcBotEntryEnd)
	if err != nil {
		s.debug("npcbot lookup query failed", "account", s.accountName, "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		if id == npcBotEntryMirrorImage {
			continue
		}
		// Blademaster disabled: the C++ gate sits inside the entry loop, so
		// a class-12 lookup always comes back empty.
		if botclass == BotClassBlademaster {
			continue
		}
		extras, ok := mgr.Extras(id)
		if !ok || extras.Class != botclass {
			continue
		}
		if name == "" {
			continue
		}
		hits = append(hits, npcBotLookupHit{id: id, name: name, race: extras.Race})
	}
	if len(hits) == 0 {
		// C++ sends LANG_COMMAND_NOCREATUREFOUND (447, Language.h:499); the
		// enUS default is hardcoded like other untranslated trinity_string
		// references (chat.go convention — unseeded in world.sql).
		s.sendSysMessage("No creature template found.")
		return
	}
	// C++: botlist.sort(&script_bot_commands::sortbots) — ascending id.
	sort.Slice(hits, func(i, j int) bool { return hits[i].id < hits[j].id })
	for _, hit := range hits {
		s.sendSysMessage(fmt.Sprintf("%d - |cffffffff|Hcreature_entry:%d|h[%s]|h|r %s", hit.id, hit.id, hit.name, npcBotLookupRaceName(hit.race)))
	}
}

// npcBotLookupRaceName mirrors the race switch in HandleNpcBotLookupCommand
// (botcommands.cpp:786-814): races >= MAX_RACES are clamped to RACE_NONE
// before the switch == C++.
func npcBotLookupRaceName(race uint8) string {
	if race >= npcBotRaceMax {
		race = npcBotRaceNone
	}
	switch race {
	case npcBotRaceHuman:
		return "Human"
	case npcBotRaceOrc:
		return "Orc"
	case npcBotRaceDwarf:
		return "Dwarf"
	case npcBotRaceNightElf:
		return "Night Elf"
	case npcBotRaceUndead:
		return "Forsaken"
	case npcBotRaceTauren:
		return "Tauren"
	case npcBotRaceGnome:
		return "Gnome"
	case npcBotRaceTroll:
		return "Troll"
	case npcBotRaceBloodElf:
		return "Blood Elf"
	case npcBotRaceDraenei:
		return "Draenei"
	case npcBotRaceNone:
		return "No Race"
	default:
		return "Unknown"
	}
}

// handleNpcBotReviveCommand mirrors HandleNpcBotReviveCommand
// (botcommands.cpp:1309): revives the selected npcbot, or all npcbots of the
// selected player. BotMgr::_reviveBot (botmgr.cpp:533) performs zero DB
// writes — resurrection visual, teleport to owner, display/faction/health/
// flags reset, follow state — all live-creature legs with no Go bridge (Go
// keeps no live creature state for bots), and the IsAlive gate has no
// bridgeable model either. The arm answers the C++ guard and success lines;
// the revive itself is a state-change no-op.
func (s *session) handleNpcBotReviveCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCBotRevive) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	sel := s.selection
	if sel == 0 {
		s.sendSysMessage(".npcbot revive")
		s.sendSysMessage("Revives selected npcbot. If player is selected, revives all selected player's npcbots")
		return
	}
	// C++: u->ToPlayer(); !HaveBot() → "%s has no npcbots!"; else
	// ReviveAllBots() (no bridge) → "Npcbots revived".
	if uint16(sel>>48) == 0x0000 {
		guid := uint32(sel)
		if mgr.CountByOwner(guid) == 0 {
			s.sendSysMessage(fmt.Sprintf("%s has no npcbots!", s.npcbotPlayerName(ctx, guid)))
			return
		}
		s.sendSysMessage("Npcbots revived")
		return
	}
	// C++: u->ToCreature() + GetBotAI() — entry-level bridged as the template
	// npcbot mask + persisted bot data, like the delete arm.
	switch uint16(sel >> 48) {
	case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
	default:
		s.sendSysMessage("You must select player or npcbot")
		return
	}
	entry := uint32((sel >> 24) & 0x00FFFFFF)
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok || flagsExtra&npcbotCreatureFlagMask == 0 {
		s.sendSysMessage("You must select player or npcbot")
		return
	}
	if _, ok := mgr.Get(entry); !ok {
		s.sendSysMessage("You must select player or npcbot")
		return
	}
	// C++: bot->IsAlive() → "%s is not dead" has no Go model (no live
	// creature state for bots); ReviveBot's legs are all live-only. The
	// guards above match, so the success line is answered.
	s.sendSysMessage(fmt.Sprintf("%s revived", s.npcbotTemplateName(ctx, entry)))
}
