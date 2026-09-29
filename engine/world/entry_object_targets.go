package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	implicitTargetGONearbyEntry   uint32 = 40 // TARGET_GAMEOBJECT_NEARBY_ENTRY (SharedDefines.h:1476)
	implicitTargetDestNearbyEntry uint32 = 46 // TARGET_DEST_NEARBY_ENTRY (SharedDefines.h:1482)
	typeIDGameObject              = 5  // TYPEID_GAMEOBJECT (ObjectGuid.h:39)
)

// isGONearbyEntrySpell reports spells carrying TARGET_GAMEOBJECT_NEARBY_ENTRY
// (40) on any effect: {GOBJ, CASTER, NEARBY, ENTRY} (SpellImplicitTargetInfo
// table, SpellInfo.cpp:260; SelectImplicitNearbyTargets, Spell.cpp:1036).
func isGONearbyEntrySpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetGONearbyEntry || eff.ImplicitTargetB == implicitTargetGONearbyEntry {
			return true
		}
	}
	return false
}

// isDestNearbyEntrySpell reports spells carrying TARGET_DEST_NEARBY_ENTRY
// (46) on any effect: {DEST, CASTER, NEARBY, ENTRY} (SpellImplicitTargetInfo
// table, SpellInfo.cpp:266; SelectImplicitNearbyTargets, Spell.cpp:1036).
func isDestNearbyEntrySpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetDestNearbyEntry || eff.ImplicitTargetB == implicitTargetDestNearbyEntry {
			return true
		}
	}
	return false
}

// spellHasDestNearbyEntryTarget is the resolveImplicitSpellDestination gate
// for the 46 half of this unit.
func spellHasDestNearbyEntryTarget(spell wotlk.Spell) bool {
	return isDestNearbyEntrySpell(spell)
}

// entryNearbyObjectRange mirrors the TARGET_CHECK_ENTRY arm of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1052): range =
// GetMaxRange(IsPositive()).
func (s *session) entryNearbyObjectRange(spell wotlk.Spell) float64 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok {
		return 0
	}
	maxRange := float64(rangeEntry.MaxHostile)
	if !isHarmfulSpell(spell) {
		maxRange = float64(rangeEntry.MaxFriendly)
	}
	return maxRange
}

// entryImplicitEffectMask collects the effect bits carrying the given
// implicit target on either slot, for the implicit-target condition loader
// (CONDITION_SOURCE_TYPE_SPELL_IMPLICIT_TARGET, SourceGroup & effectMask).
func entryImplicitEffectMask(spell wotlk.Spell, target uint32) uint32 {
	var effectMask uint32
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == target || eff.ImplicitTargetB == target {
			effectMask |= 1 << uint(i)
		}
	}
	return effectMask
}

// implicitTargetEntryObjectMask mirrors GetSearcherTypeMask (Spell.cpp:1809)
// narrowed by Condition::GetSearcherTypeMaskForConditionList
// (ConditionMgr.cpp:843) for CONDITION_OBJECT_ENTRY_GUID rows: the base mask
// is gameobject-only for TARGET_OBJECT_TYPE_GOBJ (40) and all types for
// TARGET_OBJECT_TYPE_DEST (46, the default case keeps GRID_MAP_TYPE_MASK_ALL).
// Corpses have no Go model and never match. Non-entry condition rows are
// skipped (same gap as the 38 run).
func implicitTargetEntryObjectMask(rows []conditionRow, basePlayers, baseCreatures, baseGameObjects bool) (players, creatures, gameobjects bool) {
	players, creatures, gameobjects = basePlayers, baseCreatures, baseGameObjects
	narrowed := false
	for _, row := range rows {
		if row.ConditionType != conditionObjectEntryGUID {
			continue
		}
		if !narrowed {
			players, creatures, gameobjects = false, false, false
			narrowed = true
		}
		switch row.Value1 {
		case typeIDUnit:
			creatures = true
		case typeIDPlayer:
			players = true
		case typeIDGameObject:
			gameobjects = true
		}
	}
	return players, creatures, gameobjects
}

// implicitEntryGOConditionsMet evaluates the implicit-target condition rows
// for a gameobject candidate with ConditionMgr's ElseGroup grouping, mirroring
// Condition::Meets for CONDITION_OBJECT_ENTRY_GUID (ConditionMgr.cpp:358):
// the TypeID must be TYPEID_GAMEOBJECT, a zero entry is a wildcard, and
// Value3 optionally pins the spawn GUID. Only CONDITION_OBJECT_ENTRY_GUID
// rows are evaluated per candidate; other condition types have no
// candidate-side Go evaluation and are skipped.
func implicitEntryGOConditionsMet(rows []conditionRow, entry, spawnLow uint32) bool {
	if len(rows) == 0 {
		return true
	}
	groups := make(map[int64][]conditionRow)
	for _, row := range rows {
		if row.ConditionType != conditionObjectEntryGUID {
			continue
		}
		groups[row.ElseGroup] = append(groups[row.ElseGroup], row)
	}
	if len(groups) == 0 {
		return true
	}
	for _, group := range groups {
		met := true
		for _, row := range group {
			ok := row.Value1 == typeIDGameObject &&
				(row.Value2 == 0 || uint32(row.Value2) == entry) &&
				(row.Value3 == 0 || uint32(row.Value3) == spawnLow)
			if row.Negative {
				ok = !ok
			}
			if !ok {
				met = false
				break
			}
		}
		if met {
			return true
		}
	}
	return false
}

// nearbyGameObject is a scan candidate for the gameobject container,
// covering runtime dynamic spawns and static world rows.
type nearbyGameObject struct {
	guid     uint64
	entry    uint32
	spawnLow uint32
	x, y, z  float32
}

// scanNearbyGameObjects visits runtime and static gameobjects in the caster's
// instance within the x/y box of radius, skipping hidden ones — the Go
// equivalent of the gameobject container scan in SearchTargets
// (Spell.cpp:1844) for TARGET_OBJECT_TYPE_GOBJ (GetSearcherTypeMask,
// Spell.cpp:1809).
func (s *session) scanNearbyGameObjects(ctx context.Context, radius float32, visit func(nearbyGameObject)) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	mapID, instanceID := s.player.Map, s.player.InstanceID
	seen := make(map[uint64]struct{})
	for _, dyn := range s.server.gameObjectStatesInInstance(mapID, instanceID) {
		if dyn.Hidden {
			continue
		}
		seen[dyn.GUID] = struct{}{}
		visit(nearbyGameObject{guid: dyn.GUID, entry: dyn.Entry, spawnLow: dyn.LowGUID, x: dyn.X, y: dyn.Y, z: dyn.Z})
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	goArgs := make([]any, 0, 4)
	eventClause := gameEventSpawnClause("geg.eventEntry", s.server.activeEventList(ctx), &goArgs)
	query := `SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z
		FROM gameobject AS g
		LEFT JOIN game_event_gameobject AS geg ON geg.guid = g.guid
		WHERE g.map = ?
		AND g.position_x BETWEEN ? AND ?
		AND g.position_y BETWEEN ? AND ?
		AND (g.spawnMask = 0 OR (g.spawnMask & 1) <> 0)
		AND ` + eventClause
	args := append([]any{mapID, float64(s.player.X - radius), float64(s.player.X + radius), float64(s.player.Y - radius), float64(s.player.Y + radius)}, goArgs...)
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var low, entry int64
		var x, y, z float64
		if err := rows.Scan(&low, &entry, &x, &y, &z); err != nil {
			continue
		}
		guid := gameObjectGUID(uint32(low), uint32(entry))
		if _, dup := seen[guid]; dup {
			continue
		}
		if s.server.isGameObjectHiddenInInstance(mapID, instanceID, guid) {
			continue
		}
		visit(nearbyGameObject{guid: guid, entry: uint32(entry), spawnLow: uint32(low), x: float32(x), y: float32(y), z: float32(z)})
	}
}

// spellEntryNearbyGOTarget ports the GOBJ half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) for
// TARGET_GAMEOBJECT_NEARBY_ENTRY (40): the single nearest gameobject within
// GetMaxRange(IsPositive()) passing the spell's implicit-target conditions.
// SpellInfo::CheckTarget is always valid for gameobjects (SpellInfo.cpp:1707:
// "other types of objects - always valid"), so only range and the entry
// conditions gate the search. The emergency fallback (Spell.cpp:1065): with
// no conditions and RequiresSpellFocus set, the focus object is the target.
// No match returns false and the caller fails the cast with
// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993; Spell.cpp:1111).
func (s *session) spellEntryNearbyGOTarget(ctx context.Context, spell wotlk.Spell, spellID uint32) (uint64, bool) {
	if s == nil || s.player == nil || s.server == nil {
		return 0, false
	}
	maxRange := s.entryNearbyObjectRange(spell)
	if maxRange <= 0 {
		return 0, false
	}
	condRows := s.loadImplicitTargetConditions(ctx, spellID, entryImplicitEffectMask(spell, implicitTargetGONearbyEntry))
	if len(condRows) == 0 && spell.RequiresSpellFocus != 0 {
		// Spell.cpp:1071: focus object or cast failure; the handleCastSpell
		// focus gate already guarantees the object exists here.
		if guid, _, _, _, ok := s.spellFocusObject(ctx, spell); ok {
			return guid, true
		}
		return 0, false
	}
	_, _, allowGO := implicitTargetEntryObjectMask(condRows, false, false, true)
	if !allowGO {
		return 0, false
	}
	bestGUID := uint64(0)
	bestDist := maxRange
	s.scanNearbyGameObjects(ctx, float32(maxRange), func(g nearbyGameObject) {
		if !implicitEntryGOConditionsMet(condRows, g.entry, g.spawnLow) {
			return
		}
		dist := distance3D(g.x, g.y, g.z, s.player.X, s.player.Y, s.player.Z)
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8388): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist = dist
			bestGUID = g.guid
		}
	})
	if bestGUID == 0 {
		return 0, false
	}
	return bestGUID, true
}

// spellEntryNearbyDestPosition ports the DEST half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) for
// TARGET_DEST_NEARBY_ENTRY (46): the destination becomes the position of the
// nearest world object within GetMaxRange(IsPositive()) passing
// SpellInfo::CheckTarget and the spell's implicit-target conditions. The
// container mask starts from all types and is narrowed by
// CONDITION_OBJECT_ENTRY_GUID rows; corpses have no Go model and never
// match. With no conditions and RequiresSpellFocus set, the focus object's
// position is used (Spell.cpp:1085). No match returns false and the caller
// fails the cast with SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:1111).
func (s *session) spellEntryNearbyDestPosition(ctx context.Context, spell wotlk.Spell, spellID uint32) (float32, float32, float32, bool) {
	if s == nil || s.player == nil || s.server == nil {
		return 0, 0, 0, false
	}
	maxRange := s.entryNearbyObjectRange(spell)
	if maxRange <= 0 {
		return 0, 0, 0, false
	}
	condRows := s.loadImplicitTargetConditions(ctx, spellID, entryImplicitEffectMask(spell, implicitTargetDestNearbyEntry))
	if len(condRows) == 0 && spell.RequiresSpellFocus != 0 {
		// Spell.cpp:1089: focus-object position or cast failure.
		if _, x, y, z, ok := s.spellFocusObject(ctx, spell); ok {
			return x, y, z, true
		}
		return 0, 0, 0, false
	}
	allowPlayers, allowCreatures, allowGO := implicitTargetEntryObjectMask(condRows, true, true, true)
	allowDead := spellAllowsDeadTarget(spell)
	bestX, bestY, bestZ := float32(0), float32(0), float32(0)
	bestDist := maxRange
	found := false
	consider := func(x, y, z float32) {
		dist := distance3D(x, y, z, s.player.X, s.player.Y, s.player.Z)
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8388): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist, bestX, bestY, bestZ = dist, x, y, z
			found = true
		}
	}
	if allowPlayers || allowCreatures {
		// Unit candidates carry the 38 check terms: alive (unless the spell
		// allows dead targets, SpellInfo.cpp:1715), not combat-disabled; the
		// faction switch has no TARGET_CHECK_ENTRY case (Spell.cpp:8316).
		s.friendlyScanCandidates(ctx, s.player.X, s.player.Y, float32(maxRange), func(c friendlyCandidate) {
			if c.mapID != s.player.Map || c.instanceID != s.player.InstanceID {
				return
			}
			if c.isPlayer && !allowPlayers {
				return
			}
			if !c.isPlayer && !allowCreatures {
				return
			}
			if c.health == 0 && !allowDead {
				return
			}
			if spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra) {
				return
			}
			spawnLow := uint32(0)
			if !c.isPlayer {
				spawnLow = uint32(c.guid & 0xFFFFFF)
			}
			if !implicitEntryConditionsMet(condRows, c.isPlayer, c.entry, spawnLow) {
				return
			}
			consider(c.x, c.y, c.z)
		})
	}
	if allowGO {
		s.scanNearbyGameObjects(ctx, float32(maxRange), func(g nearbyGameObject) {
			if !implicitEntryGOConditionsMet(condRows, g.entry, g.spawnLow) {
				return
			}
			consider(g.x, g.y, g.z)
		})
	}
	if !found {
		return 0, 0, 0, false
	}
	return bestX, bestY, bestZ, true
}
