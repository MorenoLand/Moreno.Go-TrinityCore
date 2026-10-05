package world

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// This file wires the ".npcbot" command family
// (src/server/game/AI/NpcBots/botcommands.cpp:97-125). The "add", "remove",
// "spawn", "move", "delete", "lookup", "revive", "reloadconfig", "command"
// (standstill/stopfully/follow), "info", "hide", "unhide", "show", "recall",
// "kill", "suicide", "distance" (follow + attack short/long/exact) and "order"
// (cast) arms are converted; the first five give Recruit/AddBotFree/Add their
// first real call sites. The remaining "set" sub-table (faction/owner/spec)
// lands in a later unit.
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
	npcBotClassWarrior     = uint8(1)
	npcBotClassPaladin     = uint8(2)
	npcBotClassHunter      = uint8(3)
	npcBotClassRogue       = uint8(4)
	npcBotClassPriest      = uint8(5)
	npcBotClassDeathKnight = uint8(6)
	npcBotClassShaman      = uint8(7)
	npcBotClassMage        = uint8(8)
	npcBotClassWarlock     = uint8(9)
	npcBotClassDruid       = uint8(11)
	npcBotSpecDefault      = uint8(31)          // BOT_SPEC_DEFAULT (botcommon.h:799)
	npcBotSpawnFlagNPCBot  = uint32(0x04000000) // CREATURE_FLAG_EXTRA_NPCBOT (CreatureData.h:63)

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
	const syntax = "Syntax: .npcbot add|remove|spawn|move|delete|lookup|revive|reloadconfig|command|info|hide|unhide|show|recall|kill|suicide|distance|order|set"
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
	case strings.HasPrefix("reloadconfig", sub):
		s.handleNpcBotReloadConfigCommand(ctx)
	case strings.HasPrefix("command", sub):
		s.handleNpcBotCommandSubCommand(ctx, rest)
	case strings.HasPrefix("info", sub):
		s.handleNpcBotInfoCommand(ctx)
	case strings.HasPrefix("hide", sub):
		s.handleNpcBotHideCommand(ctx)
	case strings.HasPrefix("unhide", sub):
		s.handleNpcBotUnhideCommand(ctx)
	case strings.HasPrefix("show", sub):
		s.handleNpcBotUnhideCommand(ctx) // C++ maps "show" to HandleNpcBotUnhideCommand
	case strings.HasPrefix("recall", sub):
		s.handleNpcBotRecallCommand(ctx)
	case strings.HasPrefix("kill", sub):
		s.handleNpcBotKillCommand(ctx)
	case strings.HasPrefix("suicide", sub):
		s.handleNpcBotKillCommand(ctx) // C++ maps "suicide" to HandleNpcBotKillCommand
	case strings.HasPrefix("distance", sub):
		s.handleNpcBotDistanceCommand(ctx, rest)
	case strings.HasPrefix("order", sub):
		s.handleNpcBotOrderCommand(ctx, rest)
	case strings.HasPrefix("set", sub):
		s.handleNpcBotSetCommand(ctx, rest)
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
// RemoveBot's temp-bot deferral arm (botmgr.cpp:803-808, IsTempBot == entry
// 70552) never issues the owner write and fails the command's GetBot check:
// single dismiss answers "NOT removed for some stupid reason!", and the
// remove-all write skips temp rows (dismissableCountByOwner mirrors the
// HaveBot gates the same way).
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
		// The gates count only dismissable (non-temp) ownership: temp bots
		// are deferred out of RemoveBot's DB-write arm (botmgr.cpp:803-808),
		// so a temp row keeps its owner and never trips the gates.
		owner := uint32(sel)
		if mgr.dismissableCountByOwner(owner) == 0 {
			s.sendSysMessage("Npcbots are not found!")
			return
		}
		if err := mgr.UpdateOwnerAll(ctx, owner, 0); err != nil {
			s.debug("npcbot remove all failed", "account", s.accountName, "owner", owner, "error", err)
			s.sendSysMessage("Some npcbots were not removed!")
			return
		}
		if mgr.dismissableCountByOwner(owner) != 0 {
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
	// C++: RemoveBot's temp-bot deferral arm (botmgr.cpp:803-808) — IsTempBot
	// (entry == BOT_ENTRY_MIRROR_IMAGE_BM = 70552, bot_ai.h:114) bots are
	// added to _removeList and kept in _bots, so the command's GetBot check
	// fails and it answers "NpcBot was NOT removed for some stupid reason!"
	// with no DB write (the NPCBOT_UPDATE_OWNER write at botmgr.cpp:820-826
	// is only reached for non-temp bots). CleanupsBeforeBotDelete's live
	// legs (group/unsummon/owner-reset) have no Go bridge.
	if entry == npcBotEntryMirrorImage {
		s.sendSysMessage("NpcBot was NOT removed for some stupid reason!")
		return
	}
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

// bot command state bits mirrored from botcommon.h:1017-1021; the Go tree keeps
// no live BotAI, so they label which C++ state each arm would set rather than
// drive anything.
const (
	botCommandStay     = 0x01 // BOT_COMMAND_STAY
	botCommandFollow   = 0x02 // BOT_COMMAND_FOLLOW
	botCommandFullStop = 0x10 // BOT_COMMAND_FULLSTOP
)

// attack range modes mirrored from botmgr.h:38-40; the Go tree keeps no live
// BotMgr, so they label which C++ mode each arm would set rather than drive
// anything.
const (
	botAttackRangeShort = 1 // BOT_ATTACK_RANGE_SHORT
	botAttackRangeLong  = 2 // BOT_ATTACK_RANGE_LONG
	botAttackRangeExact = 3 // BOT_ATTACK_RANGE_EXACT
)

// handleNpcBotReloadConfigCommand mirrors HandleNpcBotReloadConfigCommand
// (botcommands.cpp:1392, GM_COMMANDS, Console::Yes). The C++ arm re-reads the
// world and NpcBot config files (sWorld->LoadConfigSettings + BotMgr::ReloadConfig
// re-reading the 36 NpcBot.* keys from the config manager). The Go server does
// not retain the config file path, so live re-read has no bridge — the same
// documented block as handleReloadConfig (commands_reload.go:366). The arm
// gates the GM permission and the NpcBots feature like its siblings, then
// answers with the tree-standard not-supported line instead of claiming a
// reload through the C++ global GM messages.
func (s *session) handleNpcBotReloadConfigCommand(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandNPCBotReloadConfig) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	s.sendSysMessage("Reload of NpcBot config is not supported by this server (config path not retained).")
}

// handleNpcBotCommandSubCommand dispatches the ".npcbot command" sub-table
// (botcommands.cpp:72-76: standstill/stopfully/follow). The arms are
// PLAYER_COMMANDS, so no permission gate is applied — any in-game player may
// use them (the root dispatcher already requires s.player != nil).
func (s *session) handleNpcBotCommandSubCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .npcbot command standstill|stopfully|follow"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	sub := strings.ToLower(args[0])
	switch {
	case strings.HasPrefix("standstill", sub):
		s.handleNpcBotBotCommandState(ctx, botCommandStay, "STAY",
			".npcbot command standstill",
			"Forces your npcbots to stop all movement and remain stationed")
	case strings.HasPrefix("stopfully", sub):
		s.handleNpcBotBotCommandState(ctx, botCommandFullStop, "FULLSTOP",
			".npcbot command stopfully",
			"Forces your npcbots to stop all activity")
	case strings.HasPrefix("follow", sub):
		s.handleNpcBotBotCommandState(ctx, botCommandFollow, "FOLLOW",
			".npcbot command follow",
			"Allows npcbots to follow you again if stopped")
	default:
		s.sendSysMessage(syntax)
	}
}

// handleNpcBotBotCommandState mirrors HandleNpcBotCommandStandstillCommand,
// HandleNpcBotCommandStopfullyCommand and HandleNpcBotCommandFollowCommand
// (botcommands.cpp:1178-1236): the !HaveBot() gate answers the C++ usage lines
// verbatim; the selected-unit branch answers the per-bot "%s's command state
// set to 'STATE'" line when the selection decodes to one of the caller's owned
// npcbot entries (entry-level bridge of owner->GetBotMgr()->GetBot), else the
// "Bots' command state set to 'STATE'" all-bots line. BotMgr::SendBotCommandState
// and bot_ai::SetBotCommandState are live-AI-only (the Go tree keeps no BotMap),
// so the state change itself is a documented no-bridge and the success lines
// are answered as a state-change no-op.
func (s *session) handleNpcBotBotCommandState(ctx context.Context, state uint8, stateName, syntaxArm, syntaxDesc string) {
	_ = state // which BOT_COMMAND_* the arm would set; no live BotAI to receive it
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	owner := uint32(s.playerGUID)
	if mgr.CountByOwner(owner) == 0 {
		s.sendSysMessage(syntaxArm)
		s.sendSysMessage(syntaxDesc)
		return
	}
	sel := s.selection
	if sel != 0 {
		switch uint16(sel >> 48) {
		case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
			entry := uint32((sel >> 24) & 0x00FFFFFF)
			if data, ok := mgr.Get(entry); ok && data.Owner == owner {
				s.sendSysMessage(fmt.Sprintf("%s's command state set to '%s'", s.npcbotTemplateName(ctx, entry), stateName))
				return
			}
		}
	}
	s.sendSysMessage(fmt.Sprintf("Bots' command state set to '%s'", stateName))
}

// handleNpcBotDistanceCommand dispatches the ".npcbot distance" sub-table
// (botcommands.cpp:86-90: "attack" -> npcbotAttackDistanceCommandTable, ""
// -> HandleNpcBotFollowDistanceCommand). The C++ table matches empty input
// text against the "attack" arm first (empty input matches every arm name),
// which then matches "short" — so bare ".npcbot distance" answers the short
// arm's usage gate, and ".npcbot distance attack" (no range) does the same.
func (s *session) handleNpcBotDistanceCommand(ctx context.Context, args []string) {
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	if len(args) == 0 {
		s.handleNpcBotAttackDistanceCommand(ctx, nil)
		return
	}
	if strings.HasPrefix("attack", strings.ToLower(args[0])) {
		s.handleNpcBotAttackDistanceCommand(ctx, args[1:])
		return
	}
	s.handleNpcBotFollowDistanceCommand(ctx, args)
}

// handleNpcBotFollowDistanceCommand mirrors HandleNpcBotFollowDistanceCommand
// (botcommands.cpp:342): the !HaveBot() || !dist_str gate answers the C++
// usage lines verbatim; the distance clamps to [0,100] == C++. BotMgr::
// SetBotFollowDist writes the live BotMgr _followdist field (botmgr.h:130),
// which has no Go model, so the state change is a documented no-bridge and
// the success line is answered as a state-change no-op.
func (s *session) handleNpcBotFollowDistanceCommand(ctx context.Context, args []string) {
	owner := uint32(s.playerGUID)
	if s.server.Features.NPCBots.CountByOwner(owner) == 0 || len(args) == 0 {
		s.sendSysMessage(".npcbot distance #[attack] #newdist")
		s.sendSysMessage("Sets follow / attack distance for bots")
		return
	}
	newdist, _ := strconv.Atoi(args[0])
	if newdist < 0 {
		newdist = 0
	}
	if newdist > 100 {
		newdist = 100
	}
	s.sendSysMessage(fmt.Sprintf("Bots' follow distance is set to %d", newdist))
}

// handleNpcBotAttackDistanceCommand dispatches npcbotAttackDistanceCommandTable
// (botcommands.cpp:80-84: "short"/"long"/"" -> ExactCommand). Empty input
// matches "short" first in C++; any token that is not a prefix of "short" or
// "long" falls to the "" arm == HandleNpcBotAttackDistanceExactCommand.
func (s *session) handleNpcBotAttackDistanceCommand(ctx context.Context, args []string) {
	if len(args) > 0 {
		switch sub := strings.ToLower(args[0]); {
		case strings.HasPrefix("short", sub):
			s.handleNpcBotAttackDistanceShortLong(ctx, botAttackRangeShort, "short")
			return
		case strings.HasPrefix("long", sub):
			s.handleNpcBotAttackDistanceShortLong(ctx, botAttackRangeLong, "long")
			return
		}
		s.handleNpcBotAttackDistanceExactCommand(ctx, args)
		return
	}
	s.handleNpcBotAttackDistanceShortLong(ctx, botAttackRangeShort, "short")
}

// handleNpcBotAttackDistanceShortLong mirrors HandleNpcBotAttackDistanceShortCommand
// and HandleNpcBotAttackDistanceLongCommand (botcommands.cpp:361-391): the
// !HaveBot() gate answers each arm's C++ usage lines verbatim. BotMgr::
// SetBotAttackRangeMode is live-BotMgr-only (botmgr.h:134), so the mode change
// is a documented no-bridge and the success line is answered as a
// state-change no-op.
func (s *session) handleNpcBotAttackDistanceShortLong(ctx context.Context, mode uint8, modeName string) {
	_ = ctx
	_ = mode // which BOT_ATTACK_RANGE_* the arm would set; no live BotMgr to receive it
	owner := uint32(s.playerGUID)
	if s.server.Features.NPCBots.CountByOwner(owner) == 0 {
		s.sendSysMessage(fmt.Sprintf(".npcbot distance attack %s", modeName))
		s.sendSysMessage("Sets attack distance for bots")
		return
	}
	s.sendSysMessage(fmt.Sprintf("Bots' attack distance is set to '%s'", modeName))
}

// handleNpcBotAttackDistanceExactCommand mirrors
// HandleNpcBotAttackDistanceExactCommand (botcommands.cpp:393): the
// !HaveBot() || !dist_str gate answers the C++ usage lines verbatim; the
// distance clamps to [0,50] == C++. The SetBotAttackRangeMode(EXACT, range)
// leg is live-BotMgr-only — same documented no-bridge as the short/long arms.
func (s *session) handleNpcBotAttackDistanceExactCommand(ctx context.Context, args []string) {
	owner := uint32(s.playerGUID)
	if s.server.Features.NPCBots.CountByOwner(owner) == 0 || len(args) == 0 {
		s.sendSysMessage(".npcbot distance attack #newdist")
		s.sendSysMessage("Sets attack distance for bots")
		return
	}
	newdist, _ := strconv.Atoi(args[0])
	if newdist < 0 {
		newdist = 0
	}
	if newdist > 50 {
		newdist = 50
	}
	s.sendSysMessage(fmt.Sprintf("Bots' attack distance is set to %d", newdist))
}

// handleNpcBotOrderCommand dispatches the ".npcbot order" sub-table
// (botcommands.cpp:92-95: only "cast"). Empty input matches "cast" in C++
// (empty input matches every arm name), so bare ".npcbot order" answers the
// cast arm's usage gate.
func (s *session) handleNpcBotOrderCommand(ctx context.Context, args []string) {
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	if len(args) == 0 || strings.HasPrefix("cast", strings.ToLower(args[0])) {
		var rest []string
		if len(args) > 0 {
			rest = args[1:]
		}
		s.handleNpcBotOrderCastCommand(ctx, rest)
		return
	}
	s.sendSysMessage(".npcbot order cast #bot_name #spell_underscored_name #[target_token]")
	s.sendSysMessage("Orders bot to cast a spell immediately")
}

// handleNpcBotOrderCastCommand mirrors HandleNpcBotOrderCastCommand
// (botcommands.cpp:233): the !HaveBot() || !bot_name || !spell_name gate
// answers the C++ usage lines verbatim; underscores in the spell name become
// spaces == C++; the bot lookup answers "Bot %s is not found!" with the typed
// name; the target-token validation answers the C++ invalid-token lines
// verbatim. bot->GetBotAI()->GetSpellMap(), GetSpellCooldown(), the IsAlive
// gate, ObjectAccessor::GetUnit target resolution and AddOrder(BOT_ORDER_
// SPELLCAST) are all live-creature/bot_ai legs with no Go model — bot_ai is
// unconverted, so the spell and order dispatch have no bridge. DEBUG_BOT_
// ORDERS is 0 (botcommon.h:1036), meaning the C++ success path sends no
// message, so the Go arm returns silently after the bridgeable guards pass.
func (s *session) handleNpcBotOrderCastCommand(ctx context.Context, args []string) {
	mgr := s.server.Features.NPCBots
	owner := uint32(s.playerGUID)
	if mgr.CountByOwner(owner) == 0 || len(args) < 2 {
		s.sendSysMessage(".npcbot order cast #bot_name #spell_underscored_name #[target_token]")
		s.sendSysMessage("Orders bot to cast a spell immediately")
		return
	}
	botName := args[0]
	spellName := strings.ReplaceAll(args[1], "_", " ")
	var targetToken string
	if len(args) > 2 {
		targetToken = args[2]
	}

	// BotMgr::GetBotByName (botmgr.cpp:576): case-insensitive full-name match
	// against the owner's bots. No live creature names exist in Go; the
	// template name == Creature::GetName for template-spawned bots (the
	// session-locale override has no bridge — chat.go enUS convention).
	var found bool
	want := strings.ToLower(botName)
	for _, data := range mgr.Snapshot() {
		if data.Owner != owner {
			continue
		}
		if strings.ToLower(s.npcbotTemplateName(ctx, data.Entry)) == want {
			found = true
			break
		}
	}
	if !found {
		s.sendSysMessage(fmt.Sprintf("Bot %s is not found!", botName))
		return
	}

	// Target token validation == C++ verbatim (botcommands.cpp:290-303); the
	// GetUnit/map resolution below it is live-world-only.
	if targetToken != "" {
		switch strings.ToLower(targetToken) {
		case "bot", "self", "me", "master", "target", "mytarget":
		default:
			s.sendSysMessage(fmt.Sprintf("Invalid target token '%s'!", targetToken))
			s.sendSysMessage("Valid target tokens:\n    '','bot','self', 'me','master', 'target', 'mytarget'")
			return
		}
	}
	// Spell lookup (bot_ai::GetSpellMap), the IsAlive gate, cooldown check and
	// AddOrder(BOT_ORDER_SPELLCAST, botcommon.h:1034) are live-only; the spell
	// name was still underscore-normalized above == C++.
	_ = spellName
}

// handleNpcBotInfoCommand mirrors HandleNpcBotInfoCommand
// (botcommands.cpp:1089, PLAYER_COMMANDS, Console::No): lists the selected
// player's npcbot count per class. The target decode mirrors C++: no target
// answers the usage lines, a non-player target answers "No player selected",
// and the HasLowerSecurity arm answers "Invalid target" (bridged via the
// target account's security level == ChatHandler::HasLowerSecurity). The
// per-class counts come from the persisted bot data plus NPCBotManager.Extras
// (== BotDataMgr::SelectNpcBotExtras) — the same source as the lookup arm.
// The per-class alive counts and GetNpcBotsCount's live BotMap walk have no
// Go bridge (Go keeps no live-creature state for bots); the alive field is
// answered as 0 and the count is the persisted owned-bot total.
func (s *session) handleNpcBotInfoCommand(ctx context.Context) {
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	sel := s.selection
	if sel == 0 {
		s.sendSysMessage(".npcbot info")
		s.sendSysMessage("Lists NpcBots count of each class owned by selected player. You can use this on self and your party members")
		return
	}
	// C++: owner->GetSelectedPlayer() — only a player target qualifies.
	if uint16(sel>>48) != 0x0000 {
		s.sendSysMessage("No player selected")
		return
	}
	master := uint32(sel)
	var accountID uint32
	if cs := s.server.CharactersStore; cs != nil && cs.DB != nil {
		_ = cs.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", master).Scan(&accountID)
	}
	if s.security < s.accountSecurityLevel(ctx, accountID) {
		s.sendSysMessage("Invalid target")
		return
	}
	if mgr.CountByOwner(master) == 0 {
		s.sendSysMessage(fmt.Sprintf("%s has no NpcBots!", s.npcbotPlayerName(ctx, master)))
		return
	}
	s.sendSysMessage(fmt.Sprintf("Listing NpcBots for %s", s.npcbotPlayerName(ctx, master)))
	s.sendSysMessage(fmt.Sprintf("Owned NpcBots: %d", mgr.CountByOwner(master)))
	counts := make(map[uint8]uint8)
	for _, bot := range mgr.Snapshot() {
		if bot.Owner != master {
			continue
		}
		extras, ok := mgr.Extras(bot.Entry)
		if !ok {
			continue
		}
		if extras.Class < npcBotClassWarrior || extras.Class >= npcBotClassEnd {
			continue
		}
		counts[extras.Class]++
	}
	for class := npcBotClassWarrior; class < npcBotClassEnd; class++ {
		if counts[class] == 0 {
			continue
		}
		s.sendSysMessage(fmt.Sprintf("%s: %d (alive: %d)", npcbotInfoClassLabel(class), counts[class], 0))
	}
}

// npcbotInfoClassLabel mirrors the class-label switch in
// HandleNpcBotInfoCommand (botcommands.cpp:1118-1139).
func npcbotInfoClassLabel(class uint8) string {
	switch class {
	case npcBotClassWarrior:
		return "Warriors"
	case npcBotClassPaladin:
		return "Paladins"
	case npcBotClassMage:
		return "Mages"
	case npcBotClassPriest:
		return "Priests"
	case npcBotClassWarlock:
		return "Warlocks"
	case npcBotClassDruid:
		return "Druids"
	case npcBotClassDeathKnight:
		return "Death Knights"
	case npcBotClassRogue:
		return "Rogues"
	case npcBotClassShaman:
		return "Shamans"
	case npcBotClassHunter:
		return "Hunters"
	case BotClassBlademaster:
		return "Blademasters"
	case BotClassObsidianDestroyer:
		return "Destroyers"
	case BotClassArchmage:
		return "Archmagi"
	case BotClassDreadlord:
		return "Dreadlords"
	case BotClassSpellbreaker:
		return "Spell Breakers"
	case BotClassDarkRanger:
		return "Dark Rangers"
	default:
		return "Unknown Class"
	}
}

// handleNpcBotHideCommand mirrors HandleNpcBotHideCommand (botcommands.cpp:412,
// PLAYER_COMMANDS, Console::No). The !HaveBot() gate answers the C++ usage
// lines verbatim; the !IsAlive() gate bridges to isDeadOrGhost() and the
// IsPartyInCombat() gate bridges to the session combat state ==
// LANG_YOU_IN_COMBAT(23) (commands_tele.go:388). BotMgr::SetBotsHidden(true)
// and the IsPartyInCombat party remainder are live-world legs with no Go
// model (no BotMap), so the success line is answered as a state-change no-op.
func (s *session) handleNpcBotHideCommand(ctx context.Context) {
	_ = ctx
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	if s.server.Features.NPCBots.CountByOwner(uint32(s.playerGUID)) == 0 {
		s.sendSysMessage(".npcbot hide")
		s.sendSysMessage("Removes your owned npcbots from world temporarily")
		return
	}
	if s.isDeadOrGhost() {
		s.sendNotification("You are dead")
		return
	}
	if s.isInCombat() {
		s.sendNotification("You are in combat!") // LANG_YOU_IN_COMBAT (23)
		return
	}
	s.sendSysMessage("Bots hidden")
}

// handleNpcBotUnhideCommand mirrors HandleNpcBotUnhideCommand
// (botcommands.cpp:444, PLAYER_COMMANDS, Console::No); the C++ table maps both
// "unhide" and "show" to this handler, so this covers both Go arms. The
// guard chain and the SetBotsHidden(false) no-bridge delta are identical to
// the hide arm above.
func (s *session) handleNpcBotUnhideCommand(ctx context.Context) {
	_ = ctx
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	if s.server.Features.NPCBots.CountByOwner(uint32(s.playerGUID)) == 0 {
		s.sendSysMessage(".npcbot unhide | show")
		s.sendSysMessage("Returns your temporarily hidden bots back")
		return
	}
	if s.isDeadOrGhost() {
		s.sendNotification("You are dead")
		return
	}
	if s.isInCombat() {
		s.sendNotification("You are in combat!") // LANG_YOU_IN_COMBAT (23)
		return
	}
	s.sendSysMessage("Bots unhidden")
}

// npcbotSelectedOwnedBot bridges owner->GetBotMgr()->GetBot(guid) at the
// entry level for the recall/kill arms: the selection must be a creature
// HighGuid (the IsAnyTypeCreature set, mail.go:157), the entry must carry the
// NPCBOT template mask (== Creature::IsNPCBot, the delete-arm gate), and the
// entry must decode to one of the caller's owned npcbot entries
// (npcbotTemplateGate + mgr.Get == the revive-arm convention).
func (s *session) npcbotSelectedOwnedBot(ctx context.Context, mgr *NPCBotManager, owner uint32, sel uint64) (NpcBotData, bool) {
	var zero NpcBotData
	switch uint16(sel >> 48) {
	case 0xF130, 0xF140, 0xF150:
	default:
		return zero, false
	}
	entry := uint32((sel >> 24) & 0x00FFFFFF)
	flagsExtra, ok := s.npcTemplateGate(ctx, entry)
	if !ok || flagsExtra&npcbotCreatureFlagMask == 0 {
		return zero, false
	}
	data, ok := mgr.Get(entry)
	if !ok || data.Owner != owner {
		return zero, false
	}
	return data, true
}

// handleNpcBotRecallCommand mirrors HandleNpcBotRecallCommand
// (botcommands.cpp:502, PLAYER_COMMANDS, Console::No): teleports the selected
// npcbot onto the owner's position, or all owned npcbots when the owner
// selects themselves. The !guid || !HaveBot() guard answers the C++ usage
// lines verbatim; the IsPartyInCombat gate bridges to the session combat
// state == LANG_YOU_IN_COMBAT(23) (commands_tele.go:388). BotMgr::RecallAllBots
// and BotMgr::RecallBot (botmgr.cpp:1085-1098) are MovePoint teleports —
// live-world legs with no Go model (no live creature state for bots), so the
// recall is a documented state-change no-op; C++ returns true with no message
// on success, mirrored here.
func (s *session) handleNpcBotRecallCommand(ctx context.Context) {
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	owner := uint32(s.playerGUID)
	sel := s.selection
	if sel == 0 || mgr.CountByOwner(owner) == 0 {
		s.sendSysMessage(".npcbot recall")
		s.sendSysMessage("Forces npcbots to move directly on your position. Select a npcbot you want to move or select yourself to move all bots")
		return
	}
	if s.isInCombat() {
		s.sendNotification("You are in combat!") // LANG_YOU_IN_COMBAT (23)
		return
	}
	if uint16(sel>>48) == 0x0000 && uint32(sel) == owner {
		return // C++: RecallAllBots() → true, no message
	}
	if _, ok := s.npcbotSelectedOwnedBot(ctx, mgr, owner, sel); ok {
		return // C++: RecallBot(bot) → true, no message
	}
	s.sendSysMessage("You must select one of your bots or yourself")
}

// handleNpcBotKillCommand mirrors HandleNpcBotKillCommand (botcommands.cpp:473,
// PLAYER_COMMANDS, Console::No); the C++ table maps both "kill" and "suicide"
// to this handler. Note the C++ copy-paste quirk (botcommands.cpp:476): the
// kill handler's first usage line literally reads ".npcbot recall" — mirrored
// verbatim. BotMgr::KillAllBots (botmgr.cpp:1100) / BotMgr::KillBot
// (botmgr.cpp:1106) perform zero DB writes (setDeathState(JUST_DIED) +
// bot_ai::JustDied, live-only), so the kill is a documented state-change
// no-op; C++ returns true with no message on success, mirrored here.
func (s *session) handleNpcBotKillCommand(ctx context.Context) {
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	owner := uint32(s.playerGUID)
	sel := s.selection
	if sel == 0 || mgr.CountByOwner(owner) == 0 {
		s.sendSysMessage(".npcbot recall") // C++ quirk: kill's usage line says ".npcbot recall"
		s.sendSysMessage("Makes your npcbot just drop dead. If you select yourself ALL your bots will die")
		return
	}
	if uint16(sel>>48) == 0x0000 && uint32(sel) == owner {
		return // C++: KillAllBots() → true, no message
	}
	if _, ok := s.npcbotSelectedOwnedBot(ctx, mgr, owner, sel); ok {
		return // C++: KillBot(bot) → true, no message
	}
	s.sendSysMessage("You must select one of your bots or yourself")
}

// handleNpcBotSetCommand dispatches the ".npcbot set" sub-table
// (botcommands.cpp:65-70: faction/owner/spec, all GM_COMMANDS, Console::No).
// The C++ table matches empty input against "faction" first (empty input
// matches every arm name), so bare ".npcbot set" answers the faction arm's
// usage gate — the distance-arm precedent.
func (s *session) handleNpcBotSetCommand(ctx context.Context, args []string) {
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	if len(args) == 0 {
		s.handleNpcBotSetFactionCommand(ctx, nil)
		return
	}
	switch sub := strings.ToLower(args[0]); {
	case strings.HasPrefix("faction", sub):
		s.handleNpcBotSetFactionCommand(ctx, args[1:])
	case strings.HasPrefix("owner", sub):
		s.handleNpcBotSetOwnerCommand(ctx, args[1:])
	case strings.HasPrefix("spec", sub):
		s.handleNpcBotSetSpecCommand(ctx, args[1:])
	default:
		s.sendSysMessage("Syntax: .npcbot set faction|owner|spec")
	}
}

// handleNpcBotSetFactionCommand mirrors HandleNpcBotSetFactionCommand
// (botcommands.cpp:580): sets the faction of the selected uncontrolled
// npcbot, persisted in the DB. The !ubot || !*args gate answers the C++ usage
// lines verbatim; the creature + NPCBOT-template-mask + owner==0 decode
// answers ToCreature()/IsNPCBot()/IsFreeBot() == the add-arm convention. The
// 'a'/'h'/'m'/'f' first-char shortcuts (case-sensitive == C++) map to
// 1802/1801/14/35; anything else goes through the |Hfaction link extractor
// (== ChatHandler::extractKeyFromLink, extractModifyFactionKey in
// commands_modify4.go) and atoi == C++. BotDataMgr::UpdateNpcBotData(
// NPCBOT_UPDATE_FACTION) == NPCBotManager.Update(NpcBotUpdateFaction); the
// faction-template check == sFactionTemplateStore.LookupEntry via
// factionTemplateEntry; bot->GetBotAI()->ReInitFaction() is a live bot_ai leg
// (bot_ai is unconverted) with no Go model.
func (s *session) handleNpcBotSetFactionCommand(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCBotSetFaction) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	sel := s.selection
	if sel == 0 || len(args) == 0 {
		s.sendSysMessage(".npcbot set faction #faction")
		s.sendSysMessage("Sets faction for selected npcbot (saved in DB)")
		s.sendSysMessage("Use 'a', 'h', 'm' or 'f' as argument to set faction to alliance, horde, monsters (hostile to all) or friends (friendly to all)")
		return
	}
	var entry uint32
	free := false
	switch uint16(sel >> 48) {
	case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
		entry = uint32((sel >> 24) & 0x00FFFFFF)
		if flagsExtra, ok := s.npcTemplateGate(ctx, entry); ok && flagsExtra&npcbotCreatureFlagMask != 0 {
			if data, ok := mgr.Get(entry); ok && data.Owner == 0 {
				free = true // == !bot->GetBotAI()->GetBotOwnerGuid()
			}
		}
	}
	if !free {
		s.sendSysMessage("You must select uncontrolled npcbot")
		return
	}
	var factionID uint32
	switch args[0][0] {
	case 'a':
		factionID = 1802 // Alliance
	case 'h':
		factionID = 1801 // Horde
	case 'm':
		factionID = 14 // Monsters
	case 'f':
		factionID = 35 // Friendly to all
	default:
		key, _ := extractModifyFactionKey(args[0])
		if n, _ := strconv.Atoi(key); n > 0 {
			factionID = uint32(n)
		}
	}
	if _, ok := s.factionTemplateEntry(factionID); !ok {
		// LANG_WRONG_FACTION (129): the enUS text lives in the TDB seed,
		// which is not part of this checkout (world.sql seeds zero
		// trinity_string rows) and no local DB is available, so the exact
		// wording is unverifiable — the semantic content is answered instead.
		s.sendSysMessage(fmt.Sprintf("Invalid faction id %d", factionID))
		return
	}
	if err := mgr.Update(ctx, entry, NpcBotUpdateFaction, factionID); err != nil {
		s.debug("npcbot set faction failed", "account", s.accountName, "entry", entry, "error", err)
		return
	}
	s.sendSysMessage(fmt.Sprintf("%s's faction set to %d", s.npcbotTemplateName(ctx, entry), factionID))
}

// handleNpcBotSetOwnerCommand mirrors HandleNpcBotSetOwnerCommand
// (botcommands.cpp:634): binds the selected npcbot to a new player owner by
// guid or name, persisted in the DB. The !ubot || !*args gate answers the C++
// usage lines verbatim; the creature + NPCBOT-template-mask decode answers
// ToCreature()/IsNPCBot() (== the delete-arm gate), and the persisted owner
// answers GetBotAI()->GetBotOwnerGuid() for the "already has owner" gate.
// The name/guid resolution mirrors C++: a numeric token looks up the name by
// guid (miss → found=false), otherwise the guid by name (miss → guidlow=0);
// !guidlow || !found answers "Player not found". BotDataMgr::UpdateNpcBotData(
// NPCBOT_UPDATE_OWNER) == NPCBotManager.Update(NpcBotUpdateOwner); bot->
// GetBotAI()->ReinitOwner() is a live bot_ai leg with no Go model.
func (s *session) handleNpcBotSetOwnerCommand(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCBotSetOwner) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	sel := s.selection
	if sel == 0 || len(args) == 0 {
		s.sendSysMessage(".npcbot set owner #guid | #name")
		s.sendSysMessage("Binds selected npcbot to new player owner using guid or name and updates owner in DB")
		return
	}
	var entry uint32
	isBot := false
	switch uint16(sel >> 48) {
	case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
		entry = uint32((sel >> 24) & 0x00FFFFFF)
		if flagsExtra, ok := s.npcTemplateGate(ctx, entry); ok && flagsExtra&npcbotCreatureFlagMask != 0 {
			isBot = true
		}
	}
	if !isBot {
		s.sendSysMessage("You must select a npcbot")
		return
	}
	var owner uint32
	if data, ok := mgr.Get(entry); ok {
		owner = data.Owner
	}
	if owner != 0 {
		s.sendSysMessage("This npcbot already has owner")
		return
	}
	token := args[0]
	guidlow, _ := strconv.Atoi(token)
	characterName := token
	found := true
	cdb := s.server.CharactersStore
	if guidlow != 0 {
		// C++: GetCharacterNameByGuid overwrites characterName; a miss leaves
		// found=false == "Player not found".
		if cdb == nil || cdb.DB == nil {
			found = false
		} else {
			var name sql.NullString
			if err := cdb.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guidlow).Scan(&name); err != nil || !name.Valid || name.String == "" {
				found = false
			} else {
				characterName = name.String
			}
		}
	} else if cdb != nil && cdb.DB != nil {
		// C++: GetCharacterGuidByName; a miss leaves guidlow=0.
		var guid int64
		if err := cdb.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", token).Scan(&guid); err != nil || guid <= 0 {
			guidlow = 0
		} else {
			guidlow = int(guid)
		}
	}
	if guidlow == 0 || !found {
		s.sendSysMessage("Player not found")
		return
	}
	if err := mgr.Update(ctx, entry, NpcBotUpdateOwner, uint32(guidlow)); err != nil {
		s.debug("npcbot set owner failed", "account", s.accountName, "entry", entry, "error", err)
		return
	}
	s.sendSysMessage(fmt.Sprintf("%s's new owner is %s (guidlow: %d)", s.npcbotTemplateName(ctx, entry), characterName, guidlow))
}

// handleNpcBotSetSpecCommand mirrors HandleNpcBotSetSpecCommand
// (botcommands.cpp:689): changes the talent spec of the selected npcbot. The
// !ubot || !*args gate answers the C++ usage lines verbatim; the creature +
// NPCBOT-template-mask decode answers ToCreature()/IsNPCBot() (== the
// delete-arm gate). The range check == C++ verbatim, including the C++ quirk
// that the "Spec is out of range (1 to 3)!" text claims 1-3 while the
// comparison is against BOT_SPEC_BEGIN=1 (botcommon.h:801, ==
// BOT_SPEC_WARRIOR_ARMS) and BOT_SPEC_END=31 (botcommon.h:802, ==
// BOT_SPEC_DEFAULT); the (uint8)atoi cast wraps == C++ (e.g. 300 → 44, in
// range). C++ calls bot_ai::SetSpec(spec) with activate=true, whose persisted
// arm BotDataMgr::UpdateNpcBotData(NPCBOT_UPDATE_SPEC) == NPCBotManager.
// Update(NpcBotUpdateSpec); the rest of SetSpec (UnsummonAll, spell/talent
// re-init) is a live bot_ai leg with no Go model.
func (s *session) handleNpcBotSetSpecCommand(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandNPCBotSetSpec) {
		return
	}
	if s.server == nil || s.server.Features == nil || s.server.Features.NPCBots == nil {
		s.sendSysMessage("NpcBots is unavailable.")
		return
	}
	mgr := s.server.Features.NPCBots
	sel := s.selection
	if sel == 0 || len(args) == 0 {
		s.sendSysMessage(".npcbot set spec #specnumber")
		s.sendSysMessage("Changes talent spec for selected npcbot")
		return
	}
	var entry uint32
	isBot := false
	switch uint16(sel >> 48) {
	case 0xF130, 0xF140, 0xF150: // unit/pet/vehicle: the IsAnyTypeCreature set (mail.go:157)
		entry = uint32((sel >> 24) & 0x00FFFFFF)
		if flagsExtra, ok := s.npcTemplateGate(ctx, entry); ok && flagsExtra&npcbotCreatureFlagMask != 0 {
			isBot = true
		}
	}
	if !isBot {
		s.sendSysMessage("You must select a npcbot")
		return
	}
	n, _ := strconv.Atoi(args[0])
	spec := uint8(n)           // (uint8)atoi == C++; values wrap (300 → 44)
	if spec < 1 || spec > 31 { // BOT_SPEC_BEGIN..BOT_SPEC_END (botcommon.h:801-802)
		s.sendSysMessage("Spec is out of range (1 to 3)!")
		return
	}
	if err := mgr.Update(ctx, entry, NpcBotUpdateSpec, spec); err != nil {
		s.debug("npcbot set spec failed", "account", s.accountName, "entry", entry, "error", err)
		return
	}
	s.sendSysMessage(fmt.Sprintf("%s's new spec is %d", s.npcbotTemplateName(ctx, entry), spec))
}
