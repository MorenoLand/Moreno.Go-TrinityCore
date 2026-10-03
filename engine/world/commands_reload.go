package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// reload command port: reload_commandscript (cs_reload.cpp), the "reload"
// root with 89 flat arms plus the 11-arm "all" sub-table. THIRTY-SECOND of
// 39 Commands groups (cs_script_loader.cpp decl 51 / call 96; call order
// rbac(95) -> reload(96)). Trinity checks permission only on the invoker
// leaf node (ChatCommand.cpp:487), so each arm gates exactly its own C++
// permission (RBAC.h:475-580, 106 constants in permissions.go); the root
// permission 607 covers the bare ".reload".
//
// The Go worldserver has no ObjectMgr-style in-memory stores: every one of
// these tables is read on demand straight from the world DB. "Reloading" a
// table is therefore a probe read against the live DB (the Go equivalent of
// the C++ Load* call, which re-reads the same tables) plus invalidation of
// the few real Go caches (creatureStatsCache, the vehicle template maps).
// Each arm then broadcasts the same message the C++ arm sends via
// SendGlobalGMSysMessage.
//
// Special arms:
//   - `creature_template` takes entry args (C++ returns false on empty args):
//     each entry is verified in creature_template and every online session's
//     creatureStatsCache is dropped (LANG 817/818 on a missing entry).
//   - `auctions` / `gm_tickets` probe the characters DB (auctionhouse /
//     gm_ticket); gm_tickets sends no message, like the C++ arm.
//   - `rbac` probes the auth DB rbac_* tables.
//   - `vehicle_accessory` / `vehicle_template_accessory` reset the server
//     template maps and re-run the Go loaders (native).
//   - `config` is documented-blocked: the server does not retain the config
//     file path, so live worldserver.conf re-read has no bridge (the C++ arm
//     only applies a subset of settings live anyway).
//   - `reload all scripts` skips the IsScriptScheduled guard (no bridge: the
//     Go tree has no DB-script scheduler concept).

// reloadStore selects which database a reload arm probes.
type reloadStore int

const (
	reloadStoreWorld reloadStore = iota
	reloadStoreCharacters
	reloadStoreAuth
)

// reloadArm is one flat `reload <name>` arm (cs_reload.cpp table, lines
// 77-167). msg is the C++ SendGlobalGMSysMessage text ("" = silent, like
// gm_tickets); table is probed on store ("" = special arm, handled in code).
type reloadArm struct {
	name  string
	perm  uint32
	msg   string
	store reloadStore
	table string
}

// reloadArms mirrors the C++ command table order.
var reloadArms = []reloadArm{
	{"auctions", permissionCommandReloadAuctions, "Auctions reloaded.", reloadStoreCharacters, "auctionhouse"},
	{"access_requirement", permissionCommandReloadAccessRequirement, "DB table `access_requirement` reloaded.", reloadStoreWorld, "access_requirement"},
	{"achievement_criteria_data", permissionCommandReloadAchievementCriteriaData, "DB table `achievement_criteria_data` reloaded.", reloadStoreWorld, "achievement_criteria_data"},
	{"achievement_reward", permissionCommandReloadAchievementReward, "DB table `achievement_reward` reloaded.", reloadStoreWorld, "achievement_reward"},
	{"areatrigger_involvedrelation", permissionCommandReloadAreatriggerInvolvedrelation, "DB table `areatrigger_involvedrelation` (quest area triggers) reloaded.", reloadStoreWorld, "areatrigger_involvedrelation"},
	{"areatrigger_tavern", permissionCommandReloadAreatriggerTavern, "DB table `areatrigger_tavern` reloaded.", reloadStoreWorld, "areatrigger_tavern"},
	{"areatrigger_teleport", permissionCommandReloadAreatriggerTeleport, "DB table `areatrigger_teleport` reloaded.", reloadStoreWorld, "areatrigger_teleport"},
	{"autobroadcast", permissionCommandReloadAutobroadcast, "DB table `autobroadcast` reloaded.", reloadStoreWorld, "autobroadcast"},
	{"battleground_template", permissionCommandReloadBattlegroundTemplate, "DB table `battleground_template` reloaded.", reloadStoreWorld, "battleground_template"},
	{"broadcast_text", permissionCommandReloadBroadcastText, "DB table `broadcast_text` reloaded.", reloadStoreWorld, "broadcast_text"},
	{"conditions", permissionCommandReloadConditions, "Conditions reloaded.", reloadStoreWorld, "conditions"},
	{"creature_text", permissionCommandReloadCreatureText, "Creature Texts reloaded.", reloadStoreWorld, "creature_text"},
	{"creature_questender", permissionCommandReloadCreatureQuestender, "DB table `creature_questender` reloaded.", reloadStoreWorld, "creature_questender"},
	{"creature_linked_respawn", permissionCommandReloadCreatureLinkedRespawn, "DB table `creature_linked_respawn` (creature linked respawns) reloaded.", reloadStoreWorld, "creature_linked_respawn"},
	{"creature_loot_template", permissionCommandReloadCreatureLootTemplate, "DB table `creature_loot_template` reloaded.", reloadStoreWorld, "creature_loot_template"},
	{"creature_movement_override", permissionCommandReloadCreatureMovementOverride, "DB table `creature_movement_override` reloaded.", reloadStoreWorld, "creature_movement_override"},
	{"creature_onkill_reputation", permissionCommandReloadCreatureOnkillReputation, "DB table `creature_onkill_reputation` reloaded.", reloadStoreWorld, "creature_onkill_reputation"},
	{"creature_queststarter", permissionCommandReloadCreatureQueststarter, "DB table `creature_queststarter` reloaded.", reloadStoreWorld, "creature_queststarter"},
	{"creature_summon_groups", permissionCommandReloadCreatureSummonGroups, "DB table `creature_summon_groups` reloaded.", reloadStoreWorld, "creature_summon_groups"},
	{"disables", permissionCommandReloadDisables, "DB table `disables` reloaded.", reloadStoreWorld, "disables"},
	{"disenchant_loot_template", permissionCommandReloadDisenchantLootTemplate, "DB table `disenchant_loot_template` reloaded.", reloadStoreWorld, "disenchant_loot_template"},
	{"event_scripts", permissionCommandReloadEventScripts, "DB table `event_scripts` reloaded.", reloadStoreWorld, "event_scripts"},
	{"fishing_loot_template", permissionCommandReloadFishingLootTemplate, "DB table `fishing_loot_template` reloaded.", reloadStoreWorld, "fishing_loot_template"},
	{"graveyard_zone", permissionCommandReloadGraveyardZone, "DB table `game_graveyard_zone` reloaded.", reloadStoreWorld, "game_graveyard_zone"},
	{"game_tele", permissionCommandReloadGameTele, "DB table `game_tele` reloaded.", reloadStoreWorld, "game_tele"},
	{"gameobject_questender", permissionCommandReloadGameobjectQuestender, "DB table `gameobject_questender` reloaded.", reloadStoreWorld, "gameobject_questender"},
	{"gameobject_loot_template", permissionCommandReloadGameobjectQuestLootTemplate, "DB table `gameobject_loot_template` reloaded.", reloadStoreWorld, "gameobject_loot_template"},
	{"gameobject_queststarter", permissionCommandReloadGameobjectQueststarter, "DB table `gameobject_queststarter` reloaded.", reloadStoreWorld, "gameobject_queststarter"},
	{"gossip_menu", permissionCommandReloadGossipMenu, "DB table `gossip_menu` reloaded.", reloadStoreWorld, "gossip_menu"},
	{"gossip_menu_option", permissionCommandReloadGossipMenuOption, "DB table `gossip_menu_option` reloaded.", reloadStoreWorld, "gossip_menu_option"},
	{"item_enchantment_template", permissionCommandReloadItemEnchantmentTemplate, "DB table `item_enchantment_template` reloaded.", reloadStoreWorld, "item_enchantment_template"},
	{"item_loot_template", permissionCommandReloadItemLootTemplate, "DB table `item_loot_template` reloaded.", reloadStoreWorld, "item_loot_template"},
	{"item_set_names", permissionCommandReloadItemSetNames, "DB table `item_set_names` reloaded.", reloadStoreWorld, "item_set_names"},
	{"lfg_dungeon_rewards", permissionCommandReloadLfgDungeonRewards, "DB table `lfg_dungeon_rewards` reloaded.", reloadStoreWorld, "lfg_dungeon_rewards"},
	{"achievement_reward_locale", permissionCommandReloadAchievementRewardLocale, "DB table `achievement_reward_locale` reloaded.", reloadStoreWorld, "achievement_reward_locale"},
	{"creature_template_locale", permissionCommandReloadCretureTemplateLocale, "DB table `creature_template_locale` reloaded.", reloadStoreWorld, "creature_template_locale"},
	{"creature_text_locale", permissionCommandReloadCretureTextLocale, "DB table `creature_text_locale` reloaded.", reloadStoreWorld, "creature_text_locale"},
	{"gameobject_template_locale", permissionCommandReloadGameobjectTemplateLocale, "DB table `gameobject_template_locale` reloaded.", reloadStoreWorld, "gameobject_template_locale"},
	{"gossip_menu_option_locale", permissionCommandReloadGossipMenuOptionLocale, "DB table `gossip_menu_option_locale` reloaded.", reloadStoreWorld, "gossip_menu_option_locale"},
	{"item_template_locale", permissionCommandReloadItemTemplateLocale, "DB table `item_template_locale` reloaded.", reloadStoreWorld, "item_template_locale"},
	{"item_set_name_locale", permissionCommandReloadItemSetNameLocale, "DB table `item_set_name_locale` reloaded.", reloadStoreWorld, "item_set_name_locale"},
	{"npc_text_locale", permissionCommandReloadNpcTextLocale, "DB table `npc_text_locale` reloaded.", reloadStoreWorld, "npc_text_locale"},
	{"page_text_locale", permissionCommandReloadPageTextLocale, "DB table `page_text_locale` reloaded.", reloadStoreWorld, "page_text_locale"},
	{"points_of_interest_locale", permissionCommandReloadPointsOfInterestLocale, "DB table `points_of_interest_locale` reloaded.", reloadStoreWorld, "points_of_interest_locale"},
	{"quest_template_locale", permissionCommandReloadQuestTemplateLocale, "DB table `quest_template_locale` reloaded.", reloadStoreWorld, "quest_template_locale"},
	{"mail_level_reward", permissionCommandReloadMailLevelReward, "DB table `mail_level_reward` reloaded.", reloadStoreWorld, "mail_level_reward"},
	{"mail_loot_template", permissionCommandReloadMailLootTemplate, "DB table `mail_loot_template` reloaded.", reloadStoreWorld, "mail_loot_template"},
	{"milling_loot_template", permissionCommandReloadMillingLootTemplate, "DB table `milling_loot_template` reloaded.", reloadStoreWorld, "milling_loot_template"},
	{"npc_spellclick_spells", permissionCommandReloadNpcSpellclickSpells, "DB table `npc_spellclick_spells` reloaded.", reloadStoreWorld, "npc_spellclick_spells"},
	{"npc_vendor", permissionCommandReloadNpcVendor, "DB table `npc_vendor` reloaded.", reloadStoreWorld, "npc_vendor"},
	{"page_text", permissionCommandReloadPageText, "DB table `page_text` reloaded.", reloadStoreWorld, "page_text"},
	{"pickpocketing_loot_template", permissionCommandReloadPickpocketingLootTemplate, "DB table `pickpocketing_loot_template` reloaded.", reloadStoreWorld, "pickpocketing_loot_template"},
	{"points_of_interest", permissionCommandReloadPointsOfInterest, "DB table `points_of_interest` reloaded.", reloadStoreWorld, "points_of_interest"},
	{"prospecting_loot_template", permissionCommandReloadProspectingLootTemplate, "DB table `prospecting_loot_template` reloaded.", reloadStoreWorld, "prospecting_loot_template"},
	{"quest_greeting", permissionCommandReloadQuestGreeting, "DB table `quest_greeting` reloaded.", reloadStoreWorld, "quest_greeting"},
	{"quest_greeting_locale", permissionCommandReloadQuestGreetingLocale, "DB table `quest_greeting_locale` reloaded.", reloadStoreWorld, "quest_greeting_locale"},
	{"quest_poi", permissionCommandReloadQuestPoi, "DB Table `quest_poi` and `quest_poi_points` reloaded.", reloadStoreWorld, "quest_poi"},
	{"quest_template", permissionCommandReloadQuestTemplate, "DB table `quest_template` (quest definitions) reloaded.", reloadStoreWorld, "quest_template"},
	{"reference_loot_template", permissionCommandReloadReferenceLootTemplate, "DB table `reference_loot_template` reloaded.", reloadStoreWorld, "reference_loot_template"},
	{"reserved_name", permissionCommandReloadReservedName, "DB table `reserved_name` (player reserved names) reloaded.", reloadStoreWorld, "reserved_name"},
	{"reputation_reward_rate", permissionCommandReloadReputationRewardRate, "DB table `reputation_reward_rate` reloaded.", reloadStoreWorld, "reputation_reward_rate"},
	{"reputation_spillover_template", permissionCommandReloadSpilloverTemplate, "DB table `reputation_reward_rate` reloaded.", reloadStoreWorld, "reputation_spillover_template"},
	{"skill_discovery_template", permissionCommandReloadSkillDiscoveryTemplate, "DB table `skill_discovery_template` (recipes discovered at crafting) reloaded.", reloadStoreWorld, "skill_discovery_template"},
	{"skill_extra_item_template", permissionCommandReloadSkillExtraItemTemplate, "DB table `skill_extra_item_template` (extra item creation when crafting) reloaded.", reloadStoreWorld, "skill_extra_item_template"},
	{"skill_fishing_base_level", permissionCommandReloadSkillFishingBaseLevel, "DB table `skill_fishing_base_level` (fishing base level for zone/subzone) reloaded.", reloadStoreWorld, "skill_fishing_base_level"},
	{"skinning_loot_template", permissionCommandReloadSkinningLootTemplate, "DB table `skinning_loot_template` reloaded.", reloadStoreWorld, "skinning_loot_template"},
	{"smart_scripts", permissionCommandReloadSmartScripts, "Smart Scripts reloaded.", reloadStoreWorld, "smart_scripts"},
	{"spell_required", permissionCommandReloadSpellRequired, "DB table `spell_required` reloaded.", reloadStoreWorld, "spell_required"},
	{"spell_area", permissionCommandReloadSpellArea, "DB table `spell_area` (spell dependences from area/quest/auras state) reloaded.", reloadStoreWorld, "spell_area"},
	{"spell_bonus_data", permissionCommandReloadSpellBonusData, "DB table `spell_bonus_data` (spell damage/healing coefficients) reloaded.", reloadStoreWorld, "spell_bonus_data"},
	{"spell_group", permissionCommandReloadSpellGroup, "DB table `spell_group` (spell groups) reloaded.", reloadStoreWorld, "spell_group"},
	{"spell_learn_spell", permissionCommandReloadSpellLearnSpell, "DB table `spell_learn_spell` reloaded.", reloadStoreWorld, "spell_learn_spell"},
	{"spell_loot_template", permissionCommandReloadSpellLootTemplate, "DB table `spell_loot_template` reloaded.", reloadStoreWorld, "spell_loot_template"},
	{"spell_linked_spell", permissionCommandReloadSpellLinkedSpell, "DB table `spell_linked_spell` reloaded.", reloadStoreWorld, "spell_linked_spell"},
	{"spell_pet_auras", permissionCommandReloadSpellPetAuras, "DB table `spell_pet_auras` reloaded.", reloadStoreWorld, "spell_pet_auras"},
	{"spell_proc", permissionCommandReloadSpellProc, "DB table `spell_proc` (spell proc conditions and data) reloaded.", reloadStoreWorld, "spell_proc"},
	{"spell_scripts", permissionCommandReloadSpellScripts, "DB table `spell_scripts` reloaded.", reloadStoreWorld, "spell_scripts"},
	{"spell_target_position", permissionCommandReloadSpellTargetPosition, "DB table `spell_target_position` (destination coordinates for spell targets) reloaded.", reloadStoreWorld, "spell_target_position"},
	{"spell_threats", permissionCommandReloadSpellThreats, "DB table `spell_threat` (spell aggro definitions) reloaded.", reloadStoreWorld, "spell_threats"},
	{"spell_group_stack_rules", permissionCommandReloadSpellGroupStackRules, "DB table `spell_group_stack_rules` (spell stacking definitions) reloaded.", reloadStoreWorld, "spell_group_stack_rules"},
	{"trainer", permissionCommandReloadTrainer, "DB table `trainer` reloaded.\nDB table `trainer_locale` reloaded.\nDB table `trainer_spell` reloaded.\nDB table `creature_default_trainer` reloaded.", reloadStoreWorld, "trainer"},
	{"trinity_string", permissionCommandReloadTrinityString, "DB table `trinity_string` reloaded.", reloadStoreWorld, "trinity_string"},
	{"waypoint_scripts", permissionCommandReloadWaypointScripts, "DB table `waypoint_scripts` reloaded.", reloadStoreWorld, "waypoint_scripts"},
	{"waypoint_data", permissionCommandReloadWaypointData, "DB Table 'waypoint_data' reloaded.", reloadStoreWorld, "waypoint_data"},
}

// reloadArmByName indexes the flat arms for the all-groups.
var reloadArmByName = func() map[string]reloadArm {
	m := make(map[string]reloadArm, len(reloadArms))
	for _, a := range reloadArms {
		m[a.name] = a
	}
	return m
}()

// runReloadArm executes one flat arm: probe the table (the Go equivalent of
// the C++ Load* re-read), then broadcast the C++ message unless quiet.
func (s *session) runReloadArm(ctx context.Context, arm reloadArm, quiet bool) bool {
	if arm.table != "" {
		var db *sql.DB
		switch arm.store {
		case reloadStoreCharacters:
			if s.server.CharactersStore != nil {
				db = s.server.CharactersStore.DB
			}
		case reloadStoreAuth:
			if s.server.AuthStore != nil {
				db = s.server.AuthStore.DB
			}
		default:
			if s.server.WorldStore != nil {
				db = s.server.WorldStore.DB
			}
		}
		if db != nil {
			var one int
			if err := db.QueryRowContext(ctx, "SELECT 1 FROM "+arm.table+" LIMIT 1").Scan(&one); err != nil && !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
				s.sendSysMessage("DB table `" + arm.table + "` reload failed.")
				return false
			}
		}
	}
	if !quiet && arm.msg != "" {
		s.server.sendGlobalGMMessage(ctx, arm.msg)
	}
	return true
}

// runReloadArms runs a list of flat arms quietly (the C++ "all" groups call
// the inner handlers directly, bypassing their RBAC checks).
func (s *session) runReloadArms(ctx context.Context, names ...string) {
	for _, n := range names {
		if arm, ok := reloadArmByName[n]; ok {
			s.runReloadArm(ctx, arm, true)
		}
	}
}

// reloadAllGroups mirrors the C++ HandleReloadAll*Command handlers
// (cs_reload.cpp:183-211): permission for the group plus the member arms and
// any extra group message.
type reloadAllGroup struct {
	perm  uint32
	arms  []string
	extra string
}

var reloadAllGroups = map[string]reloadAllGroup{
	"achievement": {permissionCommandReloadAllAchievement,
		[]string{"achievement_criteria_data", "achievement_reward"}, ""},
	"area": {permissionCommandReloadAllArea,
		[]string{"areatrigger_teleport", "areatrigger_tavern", "graveyard_zone"}, ""},
	"gossips": {permissionCommandReloadAllGossip,
		[]string{"gossip_menu", "gossip_menu_option", "points_of_interest"}, ""},
	"item": {permissionCommandReloadAllItem,
		[]string{"page_text", "item_enchantment_template"}, ""},
	"locales": {permissionCommandReloadAllLocales,
		[]string{"achievement_reward_locale", "creature_template_locale", "creature_text_locale",
			"gameobject_template_locale", "gossip_menu_option_locale", "item_template_locale",
			"item_set_name_locale", "npc_text_locale", "page_text_locale", "points_of_interest_locale",
			"quest_template_locale", "quest_greeting_locale"}, ""},
	"loot": {permissionCommandReloadAllLoot,
		[]string{"creature_loot_template", "disenchant_loot_template", "fishing_loot_template",
			"gameobject_loot_template", "item_loot_template", "milling_loot_template",
			"pickpocketing_loot_template", "prospecting_loot_template", "mail_loot_template",
			"reference_loot_template", "skinning_loot_template", "spell_loot_template"},
		"DB tables `*_loot_template` reloaded."},
	"npc": {permissionCommandReloadAllNpc,
		[]string{"trainer", "npc_vendor", "points_of_interest", "npc_spellclick_spells"}, ""},
	"quest": {permissionCommandReloadAllQuest,
		[]string{"quest_greeting", "areatrigger_involvedrelation", "quest_poi", "quest_template",
			"creature_queststarter", "creature_questender", "gameobject_queststarter", "gameobject_questender"},
		"DB tables `*_queststarter` and `*_questender` reloaded."},
	"scripts": {permissionCommandReloadAllScripts,
		[]string{"event_scripts", "spell_scripts", "waypoint_scripts", "waypoint_data"},
		"DB tables `*_scripts` reloaded."},
	"spell": {permissionCommandReloadAllSpell,
		[]string{"skill_discovery_template", "skill_extra_item_template", "spell_required",
			"spell_area", "spell_group", "spell_learn_spell", "spell_linked_spell", "spell_proc",
			"spell_bonus_data", "spell_target_position", "spell_threats", "spell_group_stack_rules",
			"spell_pet_auras"}, ""},
}

// handleCmdReload dispatches the "reload" root (cs_reload.cpp:59-175).
func (s *session) handleCmdReload(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandReload) {
			return
		}
		s.sendSysMessage("Syntax: .reload <table>|all [group]")
		return
	}
	sub := strings.ToLower(args[0])
	if sub == "all" {
		s.handleReloadAll(ctx, args[1:])
		return
	}
	// Prefix-match flat arms like the Trinity parser.
	var matched *reloadArm
	for i := range reloadArms {
		if strings.HasPrefix(reloadArms[i].name, sub) {
			matched = &reloadArms[i]
			break
		}
	}
	switch {
	case matched != nil:
		if s.miscDeny(ctx, matched.perm) {
			return
		}
		s.runReloadArm(ctx, *matched, false)
	case sub == "creature_template":
		s.handleReloadCreatureTemplate(ctx, args[1:])
	case sub == "config":
		s.handleReloadConfig(ctx)
	case sub == "gm_tickets":
		s.handleReloadGMTickets(ctx)
	case sub == "rbac":
		s.handleReloadRBACData(ctx)
	case sub == "vehicle_accessory":
		s.handleReloadVehicleAccessory(ctx)
	case sub == "vehicle_template_accessory":
		s.handleReloadVehicleTemplateAccessory(ctx)
	default:
		s.sendSysMessage("Syntax: .reload <table>|all [group]")
	}
}

// handleReloadAll mirrors the `reload all` sub-table (cs_reload.cpp:62-73):
// `reload all` runs everything; `reload all <group>` runs one group.
func (s *session) handleReloadAll(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandReloadAll) {
			return
		}
		// C++ HandleReloadAllCommand order (cs_reload.cpp:183-211).
		for _, g := range []string{"achievement", "area", "loot", "npc", "quest", "spell", "item", "gossips", "locales"} {
			s.runReloadAllGroup(ctx, g)
		}
		s.runReloadArms(ctx, "skill_fishing_base_level", "access_requirement", "mail_level_reward",
			"reserved_name", "trinity_string", "game_tele", "creature_movement_override",
			"creature_summon_groups", "autobroadcast", "battleground_template")
		s.handleReloadVehicleAccessoryQuiet(ctx)
		s.handleReloadVehicleTemplateAccessoryQuiet(ctx)
		return
	}
	sub := strings.ToLower(args[0])
	for name, g := range reloadAllGroups {
		if strings.HasPrefix(name, sub) {
			if s.miscDeny(ctx, g.perm) {
				return
			}
			s.runReloadAllGroup(ctx, name)
			return
		}
	}
	s.sendSysMessage("Syntax: .reload all [achievement|area|gossips|item|locales|loot|npc|quest|scripts|spell]")
}

// runReloadAllGroup runs one all-group's member arms quietly plus its extra
// message (the C++ handlers call the inner commands directly, bypassing
// their individual RBAC checks).
func (s *session) runReloadAllGroup(ctx context.Context, name string) {
	g, ok := reloadAllGroups[name]
	if !ok {
		return
	}
	s.runReloadArms(ctx, g.arms...)
	if g.extra != "" {
		s.server.sendGlobalGMMessage(ctx, g.extra)
	}
}

// handleReloadCreatureTemplate mirrors HandleReloadCreatureTemplateCommand
// (cs_reload.cpp:422): entry args are verified in creature_template, then
// the server's creatureStatsCache is dropped.
func (s *session) handleReloadCreatureTemplate(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandReloadCreatureTemplate) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .reload creature_template <entry> [<entry>...]")
		return
	}
	wdb := s.server.WorldStore.DB
	for _, tok := range args {
		entry := uint32(cAtoi(tok))
		if entry == 0 {
			continue
		}
		var one int
		if wdb == nil || wdb.QueryRowContext(ctx, "SELECT 1 FROM creature_template WHERE entry = ?", entry).Scan(&one) != nil {
			s.sendSysMessage(fmt.Sprintf("Creature template (Entry: %d) not found.", entry)) // LANG_COMMAND_CREATURETEMPLATE_NOTFOUND 817
			continue
		}
	}
	s.server.statsMu.Lock()
	s.server.creatureStatsCache = nil
	s.server.statsMu.Unlock()
	s.server.sendGlobalGMMessage(ctx, "Creature template reloaded.")
}

// handleReloadConfig is documented-blocked (cs_reload.cpp:331): the server
// does not retain the config file path, so live worldserver.conf re-read
// has no bridge.
func (s *session) handleReloadConfig(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandReloadConfig) {
		return
	}
	s.sendSysMessage("Reload of world config is not supported by this server (config path not retained).")
}

// handleReloadGMTickets mirrors HandleReloadGMTicketsCommand
// (cs_reload.cpp:177): silent, like the C++ arm.
func (s *session) handleReloadGMTickets(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandReloadGmTickets) {
		return
	}
	var db *sql.DB
	if s.server.CharactersStore != nil {
		db = s.server.CharactersStore.DB
	}
	if db != nil {
		var one int
		_ = db.QueryRowContext(ctx, "SELECT 1 FROM gm_ticket LIMIT 1").Scan(&one)
	}
}

// handleReloadRBACData mirrors HandleReloadRBACCommand (cs_reload.cpp:1180):
// the auth-DB rbac tables are read on demand, so the probe is the reload.
func (s *session) handleReloadRBACData(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandReloadRbac) {
		return
	}
	if s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		for _, t := range []string{"rbac_permissions", "rbac_linked_permissions", "rbac_default_permissions", "rbac_account_permissions"} {
			var one int
			_ = s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT 1 FROM "+t+" LIMIT 1").Scan(&one)
		}
	}
	s.server.sendGlobalGMMessage(ctx, "RBAC data reloaded.")
}

// handleReloadVehicleAccessory mirrors HandleReloadVehicleAccessoryCommand
// (cs_reload.cpp:1162): native, via the Go template loader.
func (s *session) handleReloadVehicleAccessory(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandReloadVehicleAccesory) {
		return
	}
	s.handleReloadVehicleAccessoryQuiet(ctx)
	s.server.sendGlobalGMMessage(ctx, "Vehicle accessories reloaded.")
}

func (s *session) handleReloadVehicleAccessoryQuiet(ctx context.Context) {
	if s.server != nil {
		s.server.vehicleMu.Lock()
		s.server.vehicleAccessories = nil
		s.server.vehicleMu.Unlock()
		s.server.loadVehicleAccessories(ctx)
	}
}

// handleReloadVehicleTemplateAccessory mirrors
// HandleReloadVehicleTemplateAccessoryCommand (cs_reload.cpp:1170): native.
func (s *session) handleReloadVehicleTemplateAccessory(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandReloadVehicleTemplateAccessory) {
		return
	}
	s.handleReloadVehicleTemplateAccessoryQuiet(ctx)
	s.server.sendGlobalGMMessage(ctx, "Vehicle template accessories reloaded.")
}

func (s *session) handleReloadVehicleTemplateAccessoryQuiet(ctx context.Context) {
	if s.server != nil {
		s.server.vehicleMu.Lock()
		s.server.vehicleSeatAddons = nil
		s.server.vehicleMu.Unlock()
		s.server.loadVehicleSeatAddons(ctx)
	}
}
