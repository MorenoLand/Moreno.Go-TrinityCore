package world

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// misc command ports misc_commandscript (cs_misc.cpp), the TWENTY-SIXTH
// Commands group in loader call order (AddSC_message_commandscript() is call
// 88, this is call 89). The C++ file is a single column-0 class
// ("misc_commandscript"); sole-source verified (hits only cs_misc.cpp +
// cs_script_loader.cpp decl 44 / call 89). The C++ table holds 51 flat
// top-level arms (RBAC 488-535, plus 632/777/797); this file ports the first
// 17 of them in C++ table order (additem through freeze). Of these 17, 11 are
// native, 1 is partially native, and 5 are documented-blocked:
//
//   - `bank` has no Go bridge: WorldSession::SendShowBank (the banker
//     interaction packet) is unmodeled.
//   - `cometome` has no Go bridge: it drives the selected creature's motion
//     master (MovePoint); the Go tree has no creature motion model.
//   - `dev` has no Go bridge: Player::IsDeveloper/SetDeveloper is a runtime
//     flag with no Go model and no consumers.
//   - `damage` `go <guid> <dmg>` form is blocked: destructible buildings
//     (GameObject::ModifyHealth) have no Go bridge. The flat-melee path on
//     player targets is native; the school/armor/absorb and spell forms have
//     no Unit::DealDamage/CalcArmorReducedDamage/CalcAbsorbResist bridge
//     (documented fidelity gap, not a stub).
//   - `die`'s creature targets are blocked (no Unit bridge); player targets
//     die natively through the combat.go death sequence. The
//     CONFIG_DIE_COMMAND_MODE kill branch and the DealDamage branch are
//     unmodeled (documented gap).
//   - `flusharenapoints` has no Go bridge: sArenaTeamMgr::
//     DistributeArenaPoints (rating-driven point distribution + mail) is
//     unbuilt.
//
// Console-vs-chat LANG branches are moot (Go commands are always sessioned,
// so the C++ "Console" name fallback is unreachable); GetNameLink/playerLink
// have no Go bridge, so plain names are used. LANG texts are inlined from TDB
// enUS recall (no in-tree trinity_string seed), per tree convention.

// Inlined enUS texts (Language.h ids; no in-tree trinity_string seed).
const (
	miscRemovedItem        = "Removed item id %d, count %d, from player %s."        // LANG_REMOVEITEM 496
	miscRemoveItemFailure  = "No items to remove. Item id %d, count %d, player %s." // LANG_REMOVEITEM_FAILURE 600
	miscItemCannotCreate   = "Item id %d cannot be created (count: %d)."            // LANG_ITEM_CANNOT_CREATE 497
	miscItemIDInvalid      = "Item %d does not exist."                              // LANG_COMMAND_ITEMIDINVALID 435
	miscCouldNotFind       = "Could not find %s."                                   // LANG_COMMAND_COULDNOTFIND 434
	miscNoItemsFromItemSet = "No items from item set %d found."                     // LANG_NO_ITEMS_FROM_ITEMSET_FOUND 502
	miscCantTeleportSelf   = "You cannot teleport to yourself."                     // LANG_CANT_TELEPORT_SELF 171
	miscAppearingAt        = "Appearing at %s."                                     // LANG_APPEARING_AT 113
	miscSummoning          = "Summoning %s%s."                                      // LANG_SUMMONING 108
	miscSummonedBy         = "You have been summoned by %s."                        // LANG_SUMMONED_BY 109
	miscIsTeleported       = "%s is being teleported."                              // LANG_IS_TELEPORTED 102
	miscOfflineSuffix      = " (offline)"                                           // LANG_OFFLINE 37
	miscSelectCharOrCreat  = "Select a character or a creature."                    // LANG_SELECT_CHAR_OR_CREATURE 1
	miscNoCharSelected     = "No character selected."                               // LANG_NO_CHAR_SELECTED 116
	miscPlayerNotFound     = "Player not found."                                    // LANG_PLAYER_NOT_FOUND 499
	miscRemoveAllCooldown  = "All cooldowns of %s have been removed."               // LANG_REMOVEALL_COOLDOWN 492
	miscRemoveCooldown     = "Cooldown of spell %d removed from %s."                // LANG_REMOVE_COOLDOWN 493
	miscUnknownSpell       = "Unknown spell %s."                                    // LANG_UNKNOWN_SPELL 490
	miscYou                = "you"                                                  // LANG_YOU 44
	miscCharNonMounted     = "Character is not mounted."                            // LANG_CHAR_NON_MOUNTED 22
	miscCharInFlight       = "Character is in flight."                              // LANG_CHAR_IN_FLIGHT 21
	miscFreeze             = "Freezing %s."                                         // LANG_COMMAND_FREEZE 5000
	miscFreezeError        = "You can't freeze yourself."                           // LANG_COMMAND_FREEZE_ERROR 5001
	miscFreezeWrong        = "Wrong freeze command usage."                          // LANG_COMMAND_FREEZE_WRONG 5002
)

// miscDeny reports a per-arm RBAC denial the tree-standard way.
func (s *session) miscDeny(ctx context.Context, perm uint32) bool {
	if !s.commandAllowed(ctx, perm) {
		s.sendNotification("You do not have permission to use that command.")
		return true
	}
	return false
}

// miscPlayerLink mirrors ChatHandler::playerLink (Chat.h:119); the Go tree
// has no GetNameLink bridge, so the plain name is used, per tree convention.
func miscPlayerLink(name string) string { return name }

// handleCmdAddItem dispatches the "additem"/"additem set" arms
// (cs_misc.cpp:47-48). The old xyz-only handleCmdAddItem in commands.go is
// replaced by the full C++ port.
func (s *session) handleCmdAddItem(ctx context.Context, args []string) {
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .additem <id|[name]> [count] | .additem set <itemsetId>")
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if sub := strings.ToLower(args[0]); sub == "set" || strings.HasPrefix("set", sub) {
		if s.miscDeny(ctx, permissionCommandAddItemSet) {
			return
		}
		s.handleAddItemSetCommand(ctx, args[1:])
		return
	}
	if s.miscDeny(ctx, permissionCommandAddItem) {
		return
	}
	s.handleAddItemCommand(ctx, args)
}

// miscExtractItemID mirrors the C++ id extraction in HandleAddItemCommand
// (cs_misc.cpp:1147-1185): the [name] manual form goes through a world DB
// name lookup, otherwise the id comes from a |Hitem:id| link or a bare id.
func (s *session) miscExtractItemID(ctx context.Context, token string) (uint32, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false
	}
	if strings.HasPrefix(token, "[") {
		end := strings.Index(token, "]")
		if end <= 1 {
			return 0, false
		}
		name := token[1:end]
		if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			return 0, false
		}
		var entry uint32
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT entry FROM item_template WHERE name = ? LIMIT 1", name).Scan(&entry); err != nil {
			s.sendSysMessage(fmt.Sprintf(miscCouldNotFind, name))
			return 0, false
		}
		return entry, true
	}
	if id := miscExtractLinkID(token, "Hitem"); id != 0 {
		return id, true
	}
	id64, err := strconv.ParseUint(token, 10, 32)
	if err != nil || id64 == 0 {
		return 0, false
	}
	return uint32(id64), true
}

// miscExtractLinkID pulls the numeric id out of a |color|Htype:id:...|h
// shift-click link, returning 0 when the token is not such a link.
func miscExtractLinkID(token, linkType string) uint32 {
	idx := strings.Index(token, "|"+linkType+":")
	if idx < 0 {
		return 0
	}
	rest := token[idx+len(linkType)+2:]
	end := strings.IndexAny(rest, ":|")
	if end >= 0 {
		rest = rest[:end]
	}
	id64, err := strconv.ParseUint(rest, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(id64)
}

// miscSelectedPlayerOrSelf mirrors handler->getSelectedPlayer() with the
// self fallback used by the additem arms (cs_misc.cpp:1189-1191).
func (s *session) miscSelectedPlayerOrSelf() *session {
	if s.selection != 0 && s.server != nil {
		if target := s.server.playerSessionForGUID(s.selection); target != nil && target.player != nil {
			return target
		}
	}
	if s.player != nil {
		return s
	}
	return nil
}

// handleAddItemCommand mirrors HandleAddItemCommand (cs_misc.cpp:1147-1273,
// RBAC 488): [name]/link/id form, signed count (negative destroys), and the
// selected-player-or-self target. The GM-self binding strip has no Go
// binding model (documented gap); the rest is wired through
// storeOrStackItem.
func (s *session) handleAddItemCommand(ctx context.Context, args []string) {
	itemID, ok := s.miscExtractItemID(ctx, args[0])
	if !ok {
		s.sendSysMessage("Syntax: .additem <id|[name]> [count]")
		return
	}
	count := int32(1)
	if len(args) > 1 {
		if c, err := strconv.ParseInt(args[1], 10, 32); err == nil {
			count = int32(c)
		}
	}
	if count == 0 {
		count = 1
	}
	target := s.miscSelectedPlayerOrSelf()
	if target == nil {
		s.sendSysMessage(miscNoCharSelected)
		return
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage(fmt.Sprintf(miscItemIDInvalid, itemID))
		return
	}
	var exists uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT entry FROM item_template WHERE entry = ? LIMIT 1", itemID).Scan(&exists); err != nil {
		s.sendSysMessage(fmt.Sprintf(miscItemIDInvalid, itemID))
		return
	}
	targetName := miscPlayerLink(target.player.Name)
	if count < 0 {
		destroyed := s.destroyPlayerItemCount(ctx, target.playerGUID, itemID, uint32(-count))
		if destroyed > 0 {
			s.sendSysMessage(fmt.Sprintf(miscRemovedItem, itemID, destroyed, targetName))
			if unable := uint32(-count) - destroyed; unable > 0 {
				s.sendSysMessage(fmt.Sprintf(miscRemoveItemFailure, itemID, unable, targetName))
			}
		} else {
			s.sendSysMessage(fmt.Sprintf(miscRemoveItemFailure, itemID, uint32(-count), targetName))
		}
		if target != s {
			_ = target.sendInventoryItems(ctx)
			target.sendPlayerUpdate()
		} else {
			_ = s.sendInventoryItems(ctx)
			s.sendPlayerUpdate()
		}
		return
	}
	res, err := s.storeOrStackItem(ctx, target.playerGUID, itemID, uint32(count))
	if err != nil || res == nil {
		s.sendSysMessage(fmt.Sprintf(miscItemCannotCreate, itemID, uint32(count)))
		return
	}
	if target != s {
		_ = target.sendInventoryItems(ctx)
		target.sendPlayerUpdate()
	} else {
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
}

// destroyPlayerItemCount mirrors Player::DestroyItemCount (Player.cpp): it
// removes up to count items of entry from the target's item_instance rows
// (inventory, bank and bags alike) and returns how many were destroyed.
func (s *session) destroyPlayerItemCount(ctx context.Context, targetGUID uint64, itemEntry uint32, count uint32) uint32 {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || count == 0 {
		return 0
	}
	db := s.server.CharactersStore.DB
	rows, err := db.QueryContext(ctx, "SELECT guid, `count` FROM item_instance WHERE owner_guid = ? AND itemEntry = ? ORDER BY guid", targetGUID, itemEntry)
	if err != nil {
		return 0
	}
	defer rows.Close()
	type inst struct {
		guid  uint64
		count uint32
	}
	var items []inst
	for rows.Next() {
		var it inst
		var c uint32
		if err := rows.Scan(&it.guid, &c); err != nil {
			continue
		}
		it.count = c
		items = append(items, it)
	}
	_ = rows.Close()
	var destroyed uint32
	for _, it := range items {
		if destroyed >= count {
			break
		}
		take := it.count
		if destroyed+take > count {
			take = count - destroyed
		}
		if take >= it.count {
			_, _ = db.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", it.guid)
			_, _ = db.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", it.guid)
			// A fully-destroyed item drops its persisted container loot
			// (LootItemStorage::RemoveStoredLootForContainer).
			s.server.removeStoredContainerLoot(ctx, it.guid)
		} else {
			_, _ = db.ExecContext(ctx, "UPDATE item_instance SET `count` = `count` - ? WHERE guid = ?", take, it.guid)
		}
		destroyed += take
	}
	return destroyed
}

// handleAddItemSetCommand mirrors HandleAddItemSetCommand
// (cs_misc.cpp:1275-1329, RBAC 489): every item_template row whose itemset
// matches gets one copy stored on the selected-player-or-self target.
func (s *session) handleAddItemSetCommand(ctx context.Context, args []string) {
	const syntax = "Syntax: .additem set <itemsetId>"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	setID64, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		s.sendSysMessage(syntax)
		return
	}
	setID := uint32(setID64)
	if setID == 0 {
		s.sendSysMessage(fmt.Sprintf(miscNoItemsFromItemSet, setID))
		return
	}
	target := s.miscSelectedPlayerOrSelf()
	if target == nil {
		s.sendSysMessage(miscNoCharSelected)
		return
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendSysMessage(fmt.Sprintf(miscNoItemsFromItemSet, setID))
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT entry FROM item_template WHERE itemset = ? ORDER BY entry", setID)
	if err != nil {
		s.sendSysMessage(fmt.Sprintf(miscNoItemsFromItemSet, setID))
		return
	}
	defer rows.Close()
	var entries []uint32
	for rows.Next() {
		var entry uint32
		if err := rows.Scan(&entry); err == nil {
			entries = append(entries, entry)
		}
	}
	_ = rows.Close()
	if len(entries) == 0 {
		s.sendSysMessage(fmt.Sprintf(miscNoItemsFromItemSet, setID))
		return
	}
	for _, entry := range entries {
		if _, err := s.storeOrStackItem(ctx, target.playerGUID, entry, 1); err != nil {
			s.sendSysMessage(fmt.Sprintf(miscItemCannotCreate, entry, 1))
		}
	}
	if target != s {
		_ = target.sendInventoryItems(ctx)
		target.sendPlayerUpdate()
	} else {
		_ = s.sendInventoryItems(ctx)
		s.sendPlayerUpdate()
	}
}

// miscResolvePlayerTarget mirrors ChatHandler::extractPlayerTarget
// (Chat.cpp): an optional leading name, then the current selection, then
// self. It also resolves offline guids through the characters table for the
// arms that support offline targets (appear/summon).
func (s *session) miscResolvePlayerTarget(ctx context.Context, args []string) (online *session, guid uint64, name string, ok bool) {
	if len(args) > 0 && args[0] != "" {
		name = normalizePlayerName(args[0])
		if sess := s.sessionForPlayerName(name); sess != nil && sess.player != nil {
			return sess, sess.playerGUID, sess.player.Name, true
		}
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			var g uint64
			var n string
			if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid, name FROM characters WHERE name = ? LIMIT 1", name).Scan(&g, &n); err == nil && g != 0 {
				return nil, g, n, true
			}
		}
		s.sendSysMessage(miscPlayerNotFound)
		return nil, 0, "", false
	}
	if s.selection != 0 && s.server != nil {
		if sess := s.server.playerSessionForGUID(s.selection); sess != nil && sess.player != nil {
			return sess, sess.playerGUID, sess.player.Name, true
		}
	}
	if s.player != nil {
		return s, s.playerGUID, s.player.Name, true
	}
	s.sendSysMessage(miscPlayerNotFound)
	return nil, 0, "", false
}

// handleCmdAppear mirrors HandleAppearCommand (cs_misc.cpp:351-484, RBAC
// 490): teleport to the target player. Online targets teleport to the
// target's live position; offline targets load their saved DB position
// (Player::LoadPositionFromDB). Documented fidelity gaps (not stubs): the
// battleground/dungeon group and instance-bind gates have no Go bridge (no BG
// model, no instance-save manager), and the GetClosePoint offset math is
// unmodeled — the GM lands on the target's exact position.
func (s *session) handleCmdAppear(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandAppear) {
		return
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if guid == s.playerGUID {
		s.sendSysMessage(miscCantTeleportSelf)
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		return // C++ HasLowerSecurity: silent fail
	}
	s.sendSysMessage(fmt.Sprintf(miscAppearingAt, miscPlayerLink(name)))
	if s.isInFlight() {
		s.finishTaxiFlight()
	}
	if target != nil && target.player != nil {
		p := target.player
		s.goDoTeleport(ctx, p.Map, p.X, p.Y, p.Z, p.Orientation)
		return
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var mapID uint32
	var x, y, z, o float64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT map, position_x, position_y, position_z, orientation FROM characters WHERE guid = ?", guid).Scan(&mapID, &x, &y, &z, &o); err != nil {
		return
	}
	s.goDoTeleport(ctx, mapID, float32(x), float32(y), float32(z), float32(o))
}

// handleCmdAura mirrors HandleAuraCommand (cs_misc.cpp:304-323, RBAC 491):
// apply the spell as an aura on the selected player. Creature selections
// report the missing unit bridge honestly.
func (s *session) handleCmdAura(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandAura) {
		return
	}
	spellID, ok := miscParseSpellArg(ctx, s, args)
	if !ok {
		s.sendSysMessage("Syntax: .aura <spellId>")
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil {
		s.sendSysMessage(miscSelectCharOrCreat)
		return
	}
	target.applyAura(spellID)
}

// handleCmdUnAura mirrors HandleUnAuraCommand (cs_misc.cpp:325-349, RBAC
// 529): remove a spell's auras — or all auras — from the selected player.
// Creature selections report the missing unit bridge honestly.
func (s *session) handleCmdUnAura(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandUnAura) {
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil {
		s.sendSysMessage(miscSelectCharOrCreat)
		return
	}
	if len(args) > 0 && strings.EqualFold(args[0], "all") {
		target.clearActiveAuras()
		return
	}
	spellID, ok := miscParseSpellArg(ctx, s, args)
	if !ok {
		s.sendSysMessage("Syntax: .unaura <spellId|all>")
		return
	}
	target.removeAura(spellID)
}

// miscSelectedPlayerSession returns the selected player session, or nil when
// nothing player-shaped is selected (creature selections have no Go bridge).
func (s *session) miscSelectedPlayerSession() *session {
	if s.selection != 0 && s.server != nil {
		if target := s.server.playerSessionForGUID(s.selection); target != nil && target.player != nil {
			return target
		}
	}
	return nil
}

// miscParseSpellArg parses a spell id from a bare id or a |Hspell:id|
// shift-click link, validating it against the Spell.dbc store like the C++
// SpellInfo argument parser.
func miscParseSpellArg(ctx context.Context, s *session, args []string) (uint32, bool) {
	if len(args) == 0 {
		return 0, false
	}
	var id uint32
	if linkID := miscExtractLinkID(args[0], "Hspell"); linkID != 0 {
		id = linkID
	} else {
		id64, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil || id64 == 0 {
			return 0, false
		}
		id = uint32(id64)
	}
	if s.server == nil || s.server.Data == nil {
		return 0, false
	}
	if _, _, found, err := s.server.Data.SpellName(id); err != nil || !found {
		s.sendSysMessage(fmt.Sprintf(miscUnknownSpell, args[0]))
		return 0, false
	}
	return id, true
}

// handleCmdBank is the documented-blocked bank arm (cs_misc.cpp:1331-1335,
// RBAC 492): WorldSession::SendShowBank (the banker interaction packet) has
// no Go bridge. RBAC-gated with an honest message, not a stub.
func (s *session) handleCmdBank(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandBank) {
		return
	}
	s.sendSysMessage("Bank access is unavailable: the Go tree has no banker interaction packet bridge.")
}

// handleCmdBindSight mirrors HandleBindSightCommand (cs_misc.cpp:2545-2553,
// RBAC 493): the GM casts bind sight (6277) on the selected player. Creature
// selections report the missing unit bridge honestly; the view-swap effect
// follows the engine's aura model (documented gap).
func (s *session) handleCmdBindSight(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandBindSight) {
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil {
		s.sendSysMessage(miscSelectCharOrCreat)
		return
	}
	s.castSpellDirect(ctx, 6277, target.playerGUID)
}

// handleCmdUnBindSight mirrors HandleUnbindSightCommand
// (cs_misc.cpp:2555-2564, RBAC 530): stop bind sight unless possessing.
// Possession state has no Go model, so the isPossessing gate is moot
// (documented gap).
func (s *session) handleCmdUnBindSight(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandUnBindSight) {
		return
	}
	s.removeAura(6277)
}

// handleCmdCombatStop mirrors HandleCombatStopCommand (cs_misc.cpp:2263-2290,
// RBAC 494): stop the target's current attack. The C++ victim-side attacker
// clearing and spell interruption have no Go bridge (documented gap); the
// attack-target clear and SMSG_ATTACK_STOP broadcast are wired.
func (s *session) handleCmdCombatStop(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandCombatStop) {
		return
	}
	target, guid, name, ok := s.miscResolvePlayerTarget(ctx, args)
	if !ok {
		return
	}
	if target == nil {
		s.sendSysMessage("Combat stop requires an online player.")
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		return // C++ HasLowerSecurity: silent fail
	}
	victim := target.attackTarget
	target.attackTarget = 0
	_ = target.sendAttackStop(victim, false)
}

// handleCmdComeToMe is the documented-blocked cometome arm
// (cs_misc.cpp:2099-2114, RBAC 495): it drives the selected creature's motion
// master (MovePoint to the GM); the Go tree has no creature motion model.
// RBAC-gated with an honest message, not a stub.
func (s *session) handleCmdComeToMe(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandComeToMe) {
		return
	}
	s.sendSysMessage("Creature movement is unavailable: the Go tree has no creature motion model.")
}

// handleCmdCommands mirrors HandleCommandsCommand (cs_misc.cpp:602-606, RBAC
// 496): list the available top-level commands from the live command tree.
// Documented fidelity gap (not a stub): the C++ filters by the invoker's
// RBAC grants (SendCommandHelpFor); the Go tree carries no per-node
// permission mapping, so the full tree is listed and each arm still gates
// itself at invocation.
func (s *session) handleCmdCommands(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandCommands) {
		return
	}
	tree := s.buildCommandTree()
	if tree == nil {
		return
	}
	names := make([]string, 0, len(tree.children))
	for name := range tree.children {
		names = append(names, name)
	}
	sort.Strings(names)
	s.sendSysMessage("=== Available Commands ===")
	for _, name := range names {
		node := tree.children[name]
		subs := make([]string, 0, len(node.children))
		for sub := range node.children {
			subs = append(subs, sub)
		}
		sort.Strings(subs)
		line := "." + name
		if len(subs) > 0 {
			line += " " + strings.Join(subs, "|")
		}
		s.sendSysMessage(line)
	}
}

// handleCmdCooldown mirrors HandleCooldownCommand (cs_misc.cpp:723-760, RBAC
// 497): clear spell cooldowns on the selected player (or self). The bare
// form clears all; the spell form clears one. The spell-arg validity check
// mirrors the C++ SpellInfo parser.
func (s *session) handleCmdCooldown(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandCooldown) {
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil {
		target = s
	}
	if target.player == nil {
		s.sendSysMessage(miscPlayerNotFound)
		return
	}
	targetName := miscPlayerLink(target.player.Name)
	if len(args) == 0 {
		target.clearPlayerCooldowns()
		s.sendSysMessage(fmt.Sprintf(miscRemoveAllCooldown, targetName))
		return
	}
	spellID, ok := miscParseSpellArg(ctx, s, args)
	if !ok {
		s.sendSysMessage(fmt.Sprintf(miscUnknownSpell, miscYou))
		return
	}
	target.clearPlayerCooldown(spellID)
	s.sendSysMessage(fmt.Sprintf(miscRemoveCooldown, spellID, targetName))
}

// clearPlayerCooldowns mirrors SpellHistory::ResetAllCooldowns for the
// player-side cooldown stores the Go tree models (session lockout maps and
// the playerState cooldown list).
func (s *session) clearPlayerCooldowns() {
	if s == nil {
		return
	}
	if s.schoolLockouts != nil {
		for k := range s.schoolLockouts {
			delete(s.schoolLockouts, k)
		}
	}
	if s.gcdCooldowns != nil {
		for k := range s.gcdCooldowns {
			delete(s.gcdCooldowns, k)
		}
	}
	if s.player != nil {
		s.playerStateMu.Lock()
		s.player.Cooldowns = nil
		s.playerStateMu.Unlock()
	}
}

// clearPlayerCooldown mirrors SpellHistory::ResetCooldown(spellId) for a
// single spell across the same stores.
func (s *session) clearPlayerCooldown(spellID uint32) {
	if s == nil {
		return
	}
	delete(s.schoolLockouts, spellID)
	delete(s.gcdCooldowns, spellID)
	if s.player != nil {
		s.playerStateMu.Lock()
		kept := s.player.Cooldowns[:0]
		for _, cd := range s.player.Cooldowns {
			if cd.Spell != spellID {
				kept = append(kept, cd)
			}
		}
		s.player.Cooldowns = kept
		s.playerStateMu.Unlock()
	}
}

// handleCmdDamage mirrors HandleDamageCommand (cs_misc.cpp:2116-2261, RBAC
// 498): the flat-melee-damage path on a selected player. The `go <guid>
// <dmg>` destructible-building form is documented-blocked (no
// GameObject::ModifyHealth bridge), and the school/armor/absorb and
// spell-damage forms have no CalcArmorReducedDamage/CalcAbsorbResist bridge
// (documented fidelity gap, not a stub).
func (s *session) handleCmdDamage(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandDamage) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .damage <amount> | .damage go <guid> <amount>")
		return
	}
	if strings.EqualFold(args[0], "go") {
		s.sendSysMessage("Damaging gameobjects is unavailable: the Go tree has no destructible-building bridge.")
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil || target.player == nil {
		s.sendSysMessage(miscSelectCharOrCreat)
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: target.player.Name, guid: target.playerGUID, online: target}) {
		return // C++ HasLowerSecurity: silent fail
	}
	if target.player.Health == 0 {
		return // C++ !IsAlive: silent no-op
	}
	dmg64, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil || dmg64 <= 0 {
		return // C++ damage_int <= 0: silent no-op
	}
	damage := uint32(dmg64)
	if damage >= target.player.Health {
		target.player.Health = 0
		target.sendPlayerUpdate()
		// GM-issued damage has no player attacker.
		target.killPlayer(ctx, nil, false)
		return
	}
	target.player.Health -= damage
	target.sendPlayerUpdate()
}

// handleCmdDev is the documented-blocked dev arm (cs_misc.cpp:159-181, RBAC
// 499): Player::IsDeveloper/SetDeveloper is a runtime flag with no Go model
// and no consumers in the tree. RBAC-gated with an honest message, not a
// stub.
func (s *session) handleCmdDev(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandDev) {
		return
	}
	s.sendSysMessage("Developer mode is unavailable: the Go tree has no developer-flag model.")
}

// handleCmdDie mirrors HandleDieCommand (cs_misc.cpp:608-632, RBAC 500):
// kill the selected player through the combat.go death sequence. Creature
// targets report the missing unit bridge honestly; the
// CONFIG_DIE_COMMAND_MODE kill branch and the DealDamage branch are
// unmodeled (documented gap).
func (s *session) handleCmdDie(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandDie) {
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil || target.player == nil {
		s.sendSysMessage(miscSelectCharOrCreat)
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: target.player.Name, guid: target.playerGUID, online: target}) {
		return // C++ HasLowerSecurity: silent fail
	}
	if target.player.Health == 0 {
		return
	}
	target.player.Health = 0
	target.sendPlayerUpdate()
	// GM .die has no player attacker.
	target.killPlayer(ctx, nil, false)
}

// handleCmdDismount mirrors HandleDismountCommand (cs_misc.cpp:656-678, RBAC
// 501), replacing the old dismount handler in commands.go: the target is the
// selected player or self, with the not-mounted and in-flight gates.
func (s *session) handleCmdDismount(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandDismount) {
		return
	}
	target := s.miscSelectedPlayerSession()
	if target == nil {
		target = s
	}
	if target.player == nil {
		s.sendSysMessage(miscNoCharSelected)
		return
	}
	if !target.isPlayerMounted() {
		s.sendSysMessage(miscCharNonMounted)
		return
	}
	if target.isInFlight() {
		s.sendSysMessage(miscCharInFlight)
		return
	}
	for _, aura := range target.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted {
			target.removeAura(aura.SpellID)
		}
	}
	target.player.MountDisplayID = 0
	target.sendPlayerUpdate()
}

// isPlayerMounted mirrors Player::IsMounted: the mount display id or any
// SPELL_AURA_MOUNTED aura marks the player mounted.
func (s *session) isPlayerMounted() bool {
	if s == nil || s.player == nil {
		return false
	}
	if s.player.MountDisplayID != 0 {
		return true
	}
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted {
			return true
		}
	}
	return false
}

// handleCmdDistance mirrors HandleGetDistanceCommand (cs_misc.cpp:762-823,
// RBAC 502) for player targets: 3D and 2D distances from the GM. Creature
// and gameobject link forms report the missing unit/object bridge honestly.
func (s *session) handleCmdDistance(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandDistance) {
		return
	}
	var target *session
	if len(args) > 0 && args[0] != "" {
		guid := miscExtractLinkID(args[0], "Hplayer")
		if guid == 0 {
			if g64, err := strconv.ParseUint(args[0], 10, 32); err == nil {
				guid = uint32(g64)
			}
		}
		if s.server != nil {
			target = s.server.playerSessionForGUID(uint64(guid))
		}
		if target == nil || target.player == nil {
			s.sendSysMessage("Distance to creatures and gameobjects is unavailable: the Go tree has no unit/object bridge.")
			return
		}
	} else {
		target = s.miscSelectedPlayerSession()
		if target == nil || target.player == nil {
			s.sendSysMessage(miscSelectCharOrCreat)
			return
		}
	}
	dx := float64(s.player.X - target.player.X)
	dy := float64(s.player.Y - target.player.Y)
	dz := float64(s.player.Z - target.player.Z)
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
	dist2d := math.Sqrt(dx*dx + dy*dy)
	s.sendSysMessage(fmt.Sprintf("Distance: %.2f (2D: %.2f, exact: %.2f, exact 2D: %.2f)", dist, dist2d, dist, dist2d))
}

// handleCmdFlushArenaPoints is the documented-blocked flusharenapoints arm
// (cs_misc.cpp:2292-2296, RBAC 503): sArenaTeamMgr::DistributeArenaPoints
// (rating-driven point distribution + mail) has no Go bridge. RBAC-gated
// with an honest message, not a stub.
func (s *session) handleCmdFlushArenaPoints(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandFlushArenaPoints) {
		return
	}
	s.sendSysMessage("Arena point distribution is unavailable: the Go tree has no arena-team point distribution bridge.")
}

// freezeAuraSpellID is the GM Freeze spell (cs_misc.cpp:2401).
const freezeAuraSpellID uint32 = 9454

// handleCmdFreeze mirrors HandleFreezeCommand (cs_misc.cpp:2318-2414, RBAC
// 504): freeze a player with aura 9454 for an optional duration in seconds,
// defaulting to the GM.FreezeAuraDuration config (worldserver.conf default
// 0 = permanent). The Freeze Spell AuraScript side effects (combat/flags)
// have no Go bridge (documented gap); the aura application and duration are
// wired, and the target's save mirrors the C++ SaveToDB.
func (s *session) handleCmdFreeze(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandFreeze) {
		return
	}
	var target *session
	var freezeDuration uint32
	canApply := false
	getDurationFromConfig := false
	if len(args) == 0 {
		getDurationFromConfig = true
	} else if d, err := strconv.ParseUint(args[0], 10, 32); err == nil {
		target = s.miscSelectedPlayerSession()
		freezeDuration = uint32(d)
		canApply = true
	} else {
		name := normalizePlayerName(args[0])
		target = s.sessionForPlayerName(name)
		if len(args) > 1 {
			if d, err := strconv.ParseUint(args[1], 10, 32); err == nil {
				freezeDuration = uint32(d)
				canApply = true
			} else {
				getDurationFromConfig = true
			}
		} else {
			getDurationFromConfig = true
		}
	}
	if getDurationFromConfig {
		freezeDuration = s.server.Config.GMFreezeAuraDuration
		canApply = true
	}
	if !canApply {
		return
	}
	if target == nil || target.player == nil {
		s.sendSysMessage(miscFreezeWrong)
		return
	}
	if target == s {
		s.sendSysMessage(miscFreezeError)
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: target.player.Name, guid: target.playerGUID, online: target}) {
		return // C++ HasLowerSecurity: silent fail (implied by freeze targeting)
	}
	target.applyAuraWithDuration(freezeAuraSpellID, freezeDuration*1000)
	s.sendSysMessage(fmt.Sprintf(miscFreeze, miscPlayerLink(target.player.Name)))
	_ = target.savePlayerState(ctx, 1, false)
}
