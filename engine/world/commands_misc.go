package world

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// misc command ports misc_commandscript (cs_misc.cpp), the TWENTY-SIXTH
// Commands group in loader call order (AddSC_message_commandscript() is call
// 88, this is call 89). The C++ file is a single column-0 class
// ("misc_commandscript"); sole-source verified (hits only cs_misc.cpp +
// cs_script_loader.cpp decl 44 / call 89). The C++ table holds 51 flat
// top-level arms (RBAC 488-535, plus 632/777/797); this file ports the first
// 17 of them in C++ table order (additem through freeze). Of these 17, 14 are
// native, 1 is partially native, and 2 are documented-blocked:
//
//   - `bank` is wired: SendShowBank with the player's own GUID, the exact
//     bank.go/gossip.go SMSG_SHOW_BANK packet (no interactability gate,
//     unlike the banker-activate opcode path).
//   - `cometome` is wired: the selected creature rides the shared script
//     spline (the Lua MoveTo MovePoint precedent) to the GM's position.
//   - `dev` has no Go bridge: Player::IsDeveloper/SetDeveloper is a runtime
//     flag with no Go model and no consumers.
//   - `damage` `go <guid> <dmg>` form is blocked: destructible buildings
//     (GameObject::ModifyHealth) have no Go bridge. The flat-melee path on
//     player targets is native; the school/armor/absorb and spell forms have
//     no Unit::DealDamage/CalcArmorReducedDamage/CalcAbsorbResist bridge
//     (documented fidelity gap, not a stub).
//   - `die` is wired for creature targets through the rage.go kill block
//     (Health 0 + lootable flag + onCreatureKilled + event-7 hook); the
//     CONFIG_DIE_COMMAND_MODE Kill-vs-DealDamage branch choice is unmodeled
//     (documented gap).
//   - `flusharenapoints` is bridged via Server.FlushWeeklyArenaPoints
//     (arena_rating.go); the C++ SendWorldText announcements are not sent,
//     the invoker gets a summary instead.
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
	miscCannotSummonToInst = "Cannot summon to instance."                           // LANG_CANNOT_SUMMON_TO_INST 103
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

// miscSelectedPlayerStrict mirrors ChatHandler::getSelectedPlayer
// (Chat.cpp:301-310) exactly: no selection falls back to self, but a set
// selection resolves only to a connected player — a creature selection
// yields nil, not self. The freeze/unfreeze arms depend on that nil leg
// (cs_misc.cpp:2387-2391/2429-2433).
func (s *session) miscSelectedPlayerStrict() *session {
	if s.selection == 0 {
		return s
	}
	return s.miscSelectedPlayerSession()
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
			// Player::DestroyItemCount (Player.cpp:12727) funnels whole-stack
			// removals through DestroyItem, so OnItemRemove fires; partial
			// decrements never reach DestroyItem and fire nothing. The
			// GM-command caller may target another player, and the hook's
			// player arg must be the item's owner, so cross-player
			// destroys stay unfired rather than pushing the wrong player.
			if targetGUID == s.playerGUID {
				s.fireItemRemoveHook(ctx, it.guid)
			}
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

// extractPlayerNameFromLink mirrors ChatHandler::extractPlayerNameFromLink
// (Chat.cpp:643): a plain first token is used verbatim, a
// |color|Hplayer:name|h[name]|h|r shift-click link yields the name key, and
// a wrong link type yields "" (LANG_WRONG_LINK_TYPE in C++, then
// LANG_PLAYER_NOT_FOUND from extractPlayerTarget).
func extractPlayerNameFromLink(raw string) string {
	raw = strings.TrimLeft(raw, " \t\b")
	if raw == "" {
		return ""
	}
	if raw[0] != '|' {
		return strings.SplitN(raw, " ", 2)[0]
	}
	segments := strings.Split(raw, "|")
	if len(segments) < 3 || segments[1] == "" {
		return ""
	}
	link := segments[2]
	if !strings.HasPrefix(link, "Hplayer:") {
		return ""
	}
	return strings.SplitN(strings.TrimPrefix(link, "Hplayer:"), ":", 2)[0]
}

// miscResolvePlayerTarget mirrors ChatHandler::extractPlayerTarget
// (Chat.cpp): an optional leading name, then the current selection, then
// self. It also resolves offline guids through the characters table for the
// arms that support offline targets (appear/summon).
func (s *session) miscResolvePlayerTarget(ctx context.Context, args []string) (online *session, guid uint64, name string, ok bool) {
	if len(args) > 0 && args[0] != "" {
		name = normalizePlayerName(extractPlayerNameFromLink(args[0]))
		if name == "" {
			s.sendSysMessage(miscPlayerNotFound)
			return nil, 0, "", false
		}
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
// target's close point (Object.cpp:3307) facing the target, and take the
// target's phase mask; offline targets load their saved DB position
// (Player::LoadPositionFromDB) keeping the invoker's orientation.
// Documented fidelity gaps (not stubs): the battleground/dungeon group and
// instance-bind gates have no Go bridge (no BG model, no instance-save
// manager), and the GetNearPoint LoS sweep and ground clamp have none
// either, so the first close point stands.
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
	if target != nil && target.player != nil {
		p := target.player
		// C++ evaluates the facing before TeleportTo runs, so the
		// absolute angle (Position.h) is from the GM's pre-teleport
		// position toward the target.
		facing := normalizeScriptAngle(float32(math.Atan2(float64(p.Y-s.player.Y), float64(p.X-s.player.X))))
		targetPhase := target.currentPlayerPhaseMask()
		cx, cy := miscClosePoint(p.X, p.Y, p.Orientation, miscPlayerCombatReach(p), miscPlayerCombatReach(s.player), 1)
		// goDoTeleport carries the C++ flight-stop-or-recall branch.
		if s.goDoTeleport(ctx, p.Map, cx, cy, p.Z, facing) {
			miscSetPhaseMask(s, targetPhase)
		}
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
	// C++ keeps the invoker's orientation on the offline leg; the saved
	// orientation is read only because LoadPositionFromDB fills it.
	_ = o
	s.goDoTeleport(ctx, mapID, float32(x), float32(y), float32(z), s.player.Orientation)
}

// handleCmdAura mirrors HandleAuraCommand (cs_misc.cpp:304-323, RBAC 491):
// apply the spell as an aura on the selected player, or on the invoker when
// nothing is selected (getSelectedUnit falls back to self). Creature
// selections have no Go unit bridge and keep the select message.
func (s *session) handleCmdAura(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandAura) {
		return
	}
	spellID, ok := miscParseSpellArg(ctx, s, args)
	if !ok {
		s.sendSysMessage("Syntax: .aura <spellId>")
		return
	}
	target := s.miscSelectedUnitPlayer()
	if target == nil {
		s.sendSysMessage(miscSelectCharOrCreat)
		return
	}
	target.applyAura(spellID)
}

// handleCmdUnAura mirrors HandleUnAuraCommand (cs_misc.cpp:325-349, RBAC
// 529): remove a spell's auras — or all auras — from the selected player,
// or from the invoker when nothing is selected (getSelectedUnit falls back
// to self). Creature selections have no Go unit bridge and keep the select
// message.
func (s *session) handleCmdUnAura(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandUnAura) {
		return
	}
	target := s.miscSelectedUnitPlayer()
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

// miscSelectedUnitPlayer mirrors ChatHandler::getSelectedUnit (Chat.cpp:312):
// the selected player, or the invoker when nothing is selected. A creature
// selection has no Go player twin and reports nil (documented gap).
func (s *session) miscSelectedUnitPlayer() *session {
	if target := s.miscSelectedPlayerSession(); target != nil {
		return target
	}
	if s.selection != 0 {
		return nil
	}
	return s
}

// miscPlayerCombatReach mirrors inspectCombatReach (who.go): the stored
// reach, defaulting to 1.5 when unset.
func miscPlayerCombatReach(p *playerState) float32 {
	return float32(inspectCombatReach(p))
}

// miscClosePoint mirrors WorldObject::GetClosePoint (Object.cpp:3307) with
// a nil searcher: GetNearPoint2D adds the anchor's own combat reach to the
// size+distance argument, along the anchor's orientation, at the anchor's Z.
// The LoS sweep and UpdateAllowedPositionZ ground clamp inside GetNearPoint
// have no Go bridge, so the first point stands.
func miscClosePoint(anchorX, anchorY, anchorOrientation, anchorReach, size, distance float32) (float32, float32) {
	dist := float64(anchorReach) + float64(size) + float64(distance)
	return anchorX + float32(dist*math.Cos(float64(anchorOrientation))), anchorY + float32(dist*math.Sin(float64(anchorOrientation)))
}

// miscSetPhaseMask mirrors Player::SetPhaseMask(mask, true) through the
// derived phase model: the .modify phase override slot is the stored-mask
// twin (commands_modify3.go). Like C++, a mask already in effect is left
// untouched, so a GM's anywhere mask survives appearing to a same-phase
// target; the visibility fan-out has no bridge, as with .modify phase.
func miscSetPhaseMask(target *session, mask uint32) {
	if target == nil || target.player == nil || target.currentPlayerPhaseMask() == mask {
		return
	}
	target.player.PhaseMaskOverride = mask
	target.player.PhaseMaskOverrideSet = true
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

// handleCmdBank mirrors HandleBankCommand (cs_misc.cpp:1331-1335,
// RBAC 492): WorldSession::SendShowBank (BankHandler.cpp:183-188) with the
// player's own GUID — latch m_currentBankerGUID, send SMSG_SHOW_BANK
// carrying only that GUID. No interactability gate, unlike the
// banker-activate opcode path (bank.go).
func (s *session) handleCmdBank(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandBank) {
		return
	}
	s.currentBankerGUID = s.playerGUID
	bank := protocol.NewBuffer(8)
	bank.WriteU64(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SHOW_BANK), bank.Bytes(), true)
}

// handleCmdBindSight mirrors HandleBindSightCommand (cs_misc.cpp:2545-2553,
// RBAC 493): the GM casts bind sight (6277) on the selected player, or on
// themself when nothing is selected (getSelectedUnit falls back to self).
// Creature selections have no Go unit bridge and keep the select message;
// the view-swap effect follows the engine's aura model (documented gap).
func (s *session) handleCmdBindSight(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandBindSight) {
		return
	}
	target := s.miscSelectedUnitPlayer()
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
		// C++ looks the name up online only (cs_misc.cpp:2267-2275):
		// an offline target reports PLAYER_NOT_FOUND.
		s.sendSysMessage(miscPlayerNotFound)
		return
	}
	if s.characterTargetLowerSecurity(ctx, characterTarget{name: name, guid: guid, online: target}) {
		return // C++ HasLowerSecurity: silent fail
	}
	victim := target.attackTarget
	target.attackTarget = 0
	_ = target.sendAttackStop(victim, false)
	// Unit::CombatStop (Unit.cpp:5809-5826) ends PvE/PvP combat, not just
	// the attack swing: clear the in-combat flag as well.
	markPlayerOutOfCombat(ctx, target)
}

// handleCmdComeToMe mirrors HandleComeToMeCommand (cs_misc.cpp:2099-2114,
// RBAC 495): the selected creature rides the shared script spline (the Lua
// MoveTo MovePoint precedent, launchCreatureScriptSpline) to the GM's
// position. Straight-line spline: Go has no PathGenerator, the standing
// MoveTo delta; a dead creature never launches, as in C++.
func (s *session) handleCmdComeToMe(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandComeToMe) {
		return
	}
	motion := s.castCommandSelectedCreature()
	if motion == nil {
		s.sendSysMessage("Select a creature first.")
		return
	}
	p := s.player
	s.server.launchCreatureScriptSpline(motion, p.X, p.Y, p.Z, false, movementGeneratorPoint)
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
	// C++ names the owner link for the all-clear, but "you" for the
	// single-spell and unknown-spell messages when the owner is the invoker
	// (cs_misc.cpp:723-760); the no-selection fallback already made the
	// invoker the target, so target == s is exactly that case.
	reportName := targetName
	if target == s {
		reportName = miscYou
	}
	if len(args) == 0 {
		target.clearPlayerCooldowns()
		s.sendSysMessage(fmt.Sprintf(miscRemoveAllCooldown, targetName))
		return
	}
	spellID, ok := miscParseSpellArg(ctx, s, args)
	if !ok {
		s.sendSysMessage(fmt.Sprintf(miscUnknownSpell, reportName))
		return
	}
	target.clearPlayerCooldown(spellID)
	s.sendSysMessage(fmt.Sprintf(miscRemoveCooldown, spellID, reportName))
}

// clearPlayerCooldowns mirrors SpellHistory::ResetAllCooldowns for the
// playerState cooldown list only: ResetAllCooldowns clears
// _spellCooldowns and _categoryCooldowns and touches neither the school
// lockouts nor the global cooldowns. SpellHistory::SendClearCooldowns
// (SpellHistory.cpp:606-618) sends SMSG_CLEAR_COOLDOWN per spell still
// holding a cooldown before _spellCooldowns is erased — mirrored here by
// sending for every distinct spell id in the player cooldown list (the
// _spellCooldowns analog; category/lockout maps are not part of the C++
// packet list).
func (s *session) clearPlayerCooldowns() {
	if s == nil {
		return
	}
	var spellIDs []uint32
	if s.player != nil {
		s.playerStateMu.Lock()
		seen := make(map[uint32]struct{}, len(s.player.Cooldowns))
		for _, cd := range s.player.Cooldowns {
			if _, ok := seen[cd.Spell]; !ok {
				seen[cd.Spell] = struct{}{}
				spellIDs = append(spellIDs, cd.Spell)
			}
		}
		s.playerStateMu.Unlock()
	}
	for _, spellID := range spellIDs {
		_ = s.write(uint16(protocol.OpcodeSMSG_CLEAR_COOLDOWN), buildClearCooldown(s.playerGUID, spellID), true)
	}
	if s.player != nil {
		s.playerStateMu.Lock()
		s.player.Cooldowns = nil
		s.playerStateMu.Unlock()
	}
}

// playerCooldownRemainingMs mirrors SpellHistory::GetRemainingCooldown
// (SpellHistory.cpp:494-515) in milliseconds: the spell entry's End
// first — an expired entry reads 0 and never falls through to the
// category arm — else the last same-category entry's CategoryEnd. The
// C++ _categoryCooldowns map holds one pointer per category, the entry
// last AddCooldown'd for it, so the last matching Go entry decides,
// matching the interrupt.go lockout arm. Ends are Unix seconds.
func (s *session) playerCooldownRemainingMs(spellID uint32, category uint32) uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	now := time.Now().Unix()
	for _, cd := range s.player.Cooldowns {
		if cd.Spell == spellID {
			if cd.End > now {
				return clampCooldownMs((cd.End - now) * 1000)
			}
			return 0
		}
	}
	if category == 0 {
		return 0
	}
	var lastCatEnd int64
	found := false
	for _, cd := range s.player.Cooldowns {
		if cd.Category == category {
			lastCatEnd = cd.CategoryEnd
			found = true
		}
	}
	if found && lastCatEnd > now {
		return clampCooldownMs((lastCatEnd - now) * 1000)
	}
	return 0
}

func clampCooldownMs(ms int64) uint32 {
	if ms > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(ms)
}

// clearPlayerCooldown mirrors SpellHistory::ResetCooldown(spellId, true)
// (SpellHistory.cpp:432-454): the erase is silent when the spell holds no
// cooldown, otherwise SMSG_CLEAR_COOLDOWN fires for that spell before the
// erase. The cs_misc `.cooldown <spell>` path (cs_misc.cpp:756) passes
// update=true; the silent default arm is unneeded on this path.
func (s *session) clearPlayerCooldown(spellID uint32) {
	s.resetPlayerCooldown(spellID, true)
}

// resetPlayerCooldown mirrors SpellHistory::ResetCooldown(spellId, update)
// (SpellHistory.cpp:432-454) plus EraseCooldown (SpellHistory.h:140-144):
// erasing the entry also erases _categoryCooldowns[CategoryId], so
// remaining entries of the category lose their category arm — mirrored
// here by zeroing their CategoryEnd. With update=false the erase is
// silent; with update=true SMSG_CLEAR_COOLDOWN fires for the spell, but
// only when it held a cooldown.
func (s *session) resetPlayerCooldown(spellID uint32, update bool) {
	if s == nil {
		return
	}
	hadCooldown := false
	var erasedCategory uint32
	if s.player != nil {
		s.playerStateMu.Lock()
		kept := s.player.Cooldowns[:0]
		for _, cd := range s.player.Cooldowns {
			if cd.Spell != spellID {
				kept = append(kept, cd)
			} else {
				hadCooldown = true
				erasedCategory = cd.Category
			}
		}
		s.player.Cooldowns = kept
		if hadCooldown && erasedCategory != 0 {
			for i := range s.player.Cooldowns {
				if s.player.Cooldowns[i].Category == erasedCategory {
					s.player.Cooldowns[i].CategoryEnd = 0
				}
			}
		}
		s.playerStateMu.Unlock()
	}
	if hadCooldown && update {
		_ = s.write(uint16(protocol.OpcodeSMSG_CLEAR_COOLDOWN), buildClearCooldown(s.playerGUID, spellID), true)
	}
}

// resetPlayerCooldownsByCategory mirrors SpellHistory::ResetCooldowns
// (SpellHistory.h:98-115) over the PlayerMethods.h:3526 predicate —
// every entry whose spell's DBC category equals the given category is
// erased, and with update=true SMSG_CLEAR_COOLDOWN fires once per erased
// spell after the erases, but only when something matched. The predicate
// reads the DBC category when the store is available and falls back to
// the entry's stored Category without one; EraseCooldown's category-map
// erase is mirrored by zeroing CategoryEnd on remaining entries carrying
// an erased entry's stored category.
func (s *session) resetPlayerCooldownsByCategory(category uint32, update bool) {
	if s == nil || s.player == nil {
		return
	}
	s.playerStateMu.Lock()
	snapshot := append([]spellCooldown(nil), s.player.Cooldowns...)
	s.playerStateMu.Unlock()
	matched := make(map[uint32]struct{})
	for _, cd := range snapshot {
		cat := cd.Category
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(cd.Spell); err == nil && found {
				cat = spell.Category
			}
		}
		if cat == category {
			matched[cd.Spell] = struct{}{}
		}
	}
	if len(matched) == 0 {
		return
	}
	erasedCategories := make(map[uint32]struct{})
	s.playerStateMu.Lock()
	kept := s.player.Cooldowns[:0]
	for _, cd := range s.player.Cooldowns {
		if _, drop := matched[cd.Spell]; !drop {
			kept = append(kept, cd)
		} else if cd.Category != 0 {
			erasedCategories[cd.Category] = struct{}{}
		}
	}
	s.player.Cooldowns = kept
	for i := range s.player.Cooldowns {
		if _, erased := erasedCategories[s.player.Cooldowns[i].Category]; erased {
			s.player.Cooldowns[i].CategoryEnd = 0
		}
	}
	s.playerStateMu.Unlock()
	if update {
		for spellID := range matched {
			_ = s.write(uint16(protocol.OpcodeSMSG_CLEAR_COOLDOWN), buildClearCooldown(s.playerGUID, spellID), true)
		}
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
		// C++ damages the selected creature too; the flat path here is
		// player-only (no Unit::DealDamage bridge), so a creature
		// selection gets the honest block instead of the no-target text.
		if s.castCommandSelectedCreature() != nil {
			s.sendSysMessage("Damaging creatures is unavailable: the Go tree has no Unit::DealDamage bridge.")
			return
		}
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
		// GM-issued damage has no player attacker in the kill chain, but
		// C++ deals it with the GM as attacker (cs_misc.cpp:2207), so the
		// Unit::Kill tail still fires Eluna PLAYER_EVENT_ON_KILL_PLAYER (6).
		target.killPlayer(ctx, nil, false, false)
		s.server.firePVPKillHook(ctx, s, target)
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
// kill the selected player through the combat.go death sequence, or the
// selected creature through the rage.go kill block below. The
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
		if motion := s.castCommandSelectedCreature(); motion != nil {
			s.dieSelectedCreature(ctx, motion)
			return
		}
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
	// GM .die has no player attacker in the kill chain, but C++ deals it
	// with the GM as attacker (cs_misc.cpp:628), so the Unit::Kill tail
	// still fires Eluna PLAYER_EVENT_ON_KILL_PLAYER (6).
	target.killPlayer(ctx, nil, false, false)
	s.server.firePVPKillHook(ctx, s, target)
}

// dieSelectedCreature kills the selected creature through the rage.go kill
// block (HandleDieCommand's DealDamage leg, cs_misc.cpp:608-632): Health 0,
// lootable flag, combat/target/motion clear, threat clear, values + threat
// broadcasts, then the onCreatureKilled funnel (respawn scheduling included)
// and the Eluna PLAYER_EVENT_ON_KILL_CREATURE (7) hook with the GM as the
// kill-attributed session, matching C++'s GM-attacker DealDamage. An
// already-dead creature is a silent no-op, the C++ IsAlive gate. No tap is
// recorded, so kill rewards ride the funnel's no-tap path (documented gap).
func (s *session) dieSelectedCreature(ctx context.Context, motion *creatureMotion) {
	if s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	if motion.Health == 0 {
		s.server.motionMu.Unlock()
		return
	}
	motion.Health = 0
	motion.DynamicFlags |= unitDynFlagLootable
	motion.InCombat = false
	motion.TargetGUID = 0
	motion.Moving = false
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	x, y, z, phase := motion.X, motion.Y, motion.Z, motion.PhaseMask
	maxHealth, level := motion.MaxHealth, motion.Level
	s.server.motionMu.Unlock()
	s.server.stopCreatureMotionInInstance(mapID, instanceID, guid, x, y, z)
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{
		unitFieldHealth:       0,
		unitFieldDynamicFlags: unitDynFlagLootable,
	})
	s.server.broadcastThreatClearInInstance(mapID, instanceID, x, y, z, guid, phase)
	s.onCreatureKilled(ctx, combatTarget{GUID: guid, Map: mapID, InstanceID: instanceID, X: x, Y: y, Z: z, Health: 0, MaxHealth: maxHealth, Level: uint8(level)}, nil)
	if victim := s.server.findCreatureMotion(mapID, instanceID, guid); victim != nil {
		s.server.fireCreatureKillHook(ctx, s, victim)
	}
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
		// C++ extractLowGuidFromLink (Chat.cpp:598-624) takes only |H...:|
		// links: a bare number fails silently (cs_misc.cpp:768-770).
		tag, data := gpsLinkTagData(args[0])
		switch tag {
		case "player":
			name := normalizePlayerName(data)
			if name == "" {
				return
			}
			target = s.sessionForPlayerName(name)
			if target == nil || target.player == nil {
				// The name resolves through the character cache even
				// offline (Chat.cpp:611-614); only an unknown name
				// fails silently, an offline player reports not-found.
				if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
					var g uint64
					if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ? LIMIT 1", name).Scan(&g); err == nil && g != 0 {
						s.sendSysMessage(miscPlayerNotFound)
						return
					}
				}
				return
			}
		case "creature", "gameobject":
			s.sendSysMessage("Distance to creatures and gameobjects is unavailable: the Go tree has no unit/object bridge.")
			return
		default:
			return
		}
	} else {
		// C++ getSelectedUnit() falls back to self (Chat.cpp:312-320).
		target = s
	}
	dx := float64(s.player.X - target.player.X)
	dy := float64(s.player.Y - target.player.Y)
	dz := float64(s.player.Z - target.player.Z)
	exact := math.Sqrt(dx*dx + dy*dy + dz*dz)
	exact2d := math.Sqrt(dx*dx + dy*dy)
	// WorldObject::GetDistance/GetDistance2d (Object.cpp:1169-1191) subtract
	// both combat reaches and clamp at 0; GetExactDist is the raw center
	// distance.
	reach := float64(miscPlayerCombatReach(s.player) + miscPlayerCombatReach(target.player))
	dist := math.Max(0, exact-reach)
	dist2d := math.Max(0, exact2d-reach)
	s.sendSysMessage(fmt.Sprintf("Distance: %.2f (2D: %.2f, exact: %.2f, exact 2D: %.2f)", dist, dist2d, exact, exact2d))
}

// handleCmdFlushArenaPoints mirrors HandleFlushArenaPointsCommand
// (cs_misc.cpp:2292-2296, RBAC 503): sArenaTeamMgr::DistributeArenaPoints
// runs via Server.FlushWeeklyArenaPoints (arena_rating.go). C++ takes no
// player and sends world-text announcements; Go keeps the sessioned-command
// player gate and reports the total to the invoker instead.
func (s *session) handleCmdFlushArenaPoints(ctx context.Context, args []string) {
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	if s.miscDeny(ctx, permissionCommandFlushArenaPoints) {
		return
	}
	if s.server == nil {
		return
	}
	total, err := s.server.FlushWeeklyArenaPoints(ctx)
	if err != nil {
		s.sendSysMessage("Arena point distribution failed.")
		return
	}
	s.sendSysMessage(fmt.Sprintf("Arena points distributed: %d points granted.", total))
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
		target = s.miscSelectedPlayerStrict() // C++ case 1: bare .freeze freezes the selected player (getSelectedPlayer self-fallback)
		getDurationFromConfig = true
	} else if d, err := strconv.ParseUint(args[0], 10, 32); err == nil {
		target = s.miscSelectedPlayerStrict()
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
	// C++ HandleFreezeCommand (cs_misc.cpp:2318-2415) has no HasLowerSecurity
	// gate: any GM holding the command permission may freeze.
	target.applyAuraWithDuration(freezeAuraSpellID, freezeDuration*1000)
	s.sendSysMessage(fmt.Sprintf(miscFreeze, miscPlayerLink(target.player.Name)))
	_ = target.savePlayerState(ctx, 1, false)
}
