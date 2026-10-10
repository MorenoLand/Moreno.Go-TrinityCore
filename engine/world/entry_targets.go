package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const implicitTargetNearbyEntry uint32 = 38 // TARGET_UNIT_NEARBY_ENTRY (SharedDefines.h:1474)

const (
	conditionObjectEntryGUID = 31 // CONDITION_OBJECT_ENTRY_GUID (ConditionMgr.h:69)
	typeIDUnit               = 3  // TYPEID_UNIT
	typeIDPlayer              = 4  // TYPEID_PLAYER
)

// isEntryNearbySpell reports spells carrying TARGET_UNIT_NEARBY_ENTRY (38)
// on any effect: {UNIT, CASTER, NEARBY, ENTRY} (SpellImplicitTargetInfo
// table, SpellInfo.cpp:218-330; SelectImplicitNearbyTargets, Spell.cpp:1036).
func isEntryNearbySpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetNearbyEntry || eff.ImplicitTargetB == implicitTargetNearbyEntry {
			return true
		}
	}
	return false
}

// implicitTargetEntryMask mirrors Condition::GetSearcherTypeMaskForConditionList
// (ConditionMgr.cpp:679) for CONDITION_OBJECT_ENTRY_GUID rows: each row
// narrows the scan to its TypeID. With no entry rows every unit kind is
// scanned (GetSearcherTypeMask, Spell.cpp:1809).
func implicitTargetEntryMask(rows []conditionRow) (players, creatures bool) {
	players, creatures = true, true
	narrowed := false
	for _, row := range rows {
		if row.ConditionType != conditionObjectEntryGUID {
			continue
		}
		if !narrowed {
			players, creatures = false, false
			narrowed = true
		}
		switch row.Value1 {
		case typeIDUnit:
			creatures = true
		case typeIDPlayer:
			players = true
		}
	}
	return players, creatures
}

// implicitEntryGUIDMet mirrors Condition::Meets for CONDITION_OBJECT_ENTRY_GUID
// (ConditionMgr.cpp:358): the TypeID must match, a zero entry is a wildcard,
// and Value3 optionally pins the spawn GUID.
func implicitEntryGUIDMet(row conditionRow, isPlayer bool, entry, spawnLow uint32) bool {
	if isPlayer {
		if row.Value1 != typeIDPlayer {
			return false
		}
		return row.Value2 == 0
	}
	if row.Value1 != typeIDUnit {
		return false
	}
	if row.Value2 != 0 && uint32(row.Value2) != entry {
		return false
	}
	if row.Value3 != 0 && uint32(row.Value3) != spawnLow {
		return false
	}
	return true
}

// implicitEntryConditionsMet evaluates the implicit-target condition rows
// with ConditionMgr's ElseGroup grouping (rows sharing an ElseGroup AND
// together, distinct ElseGroups OR). Only CONDITION_OBJECT_ENTRY_GUID rows
// are evaluated per candidate; other condition types have no candidate-side
// Go evaluation and are skipped.
func implicitEntryConditionsMet(rows []conditionRow, isPlayer bool, entry, spawnLow uint32) bool {
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
			ok := implicitEntryGUIDMet(row, isPlayer, entry, spawnLow)
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

// spellEntryNearbyTarget ports Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) + Spell::SearchNearbyTarget (Spell.cpp:1869) for
// TARGET_UNIT_NEARBY_ENTRY (38): the single nearest unit within
// GetMaxRange(IsPositive()) passing SpellInfo::CheckTarget and the spell's
// implicit-target conditions. The faction switch in
// WorldObjectSpellTargetCheck (Spell.cpp:8316) has no TARGET_CHECK_ENTRY
// case, so only the CheckTarget terms apply: alive (unless the spell allows
// dead targets, SpellInfo.cpp:1715), not combat-disabled. No match returns
// false and the caller fails the cast with SPELL_FAILED_BAD_IMPLICIT_TARGETS
// (SharedDefines.h:993; Spell.cpp:1111).
func (s *session) spellEntryNearbyTarget(ctx context.Context, spell wotlk.Spell, spellID uint32) (uint64, bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, false
	}
	rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok {
		return 0, false
	}
	maxRange := float64(rangeEntry.MaxHostile)
	if !isHarmfulSpell(spell) {
		maxRange = float64(rangeEntry.MaxFriendly)
	}
	if maxRange <= 0 {
		return 0, false
	}
	var effectMask uint32
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetNearbyEntry || eff.ImplicitTargetB == implicitTargetNearbyEntry {
			effectMask |= 1 << uint(i)
		}
	}
	condRows := s.loadImplicitTargetConditions(ctx, spellID, effectMask)
	allowPlayers, allowCreatures := implicitTargetEntryMask(condRows)
	allowDead := spellAllowsDeadTarget(spell)
	bestGUID := uint64(0)
	bestDist := maxRange
	s.friendlyScanCandidates(ctx, s.player.X, s.player.Y, float32(maxRange), spell, func(c friendlyCandidate) {
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
		if spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, false) {
			return
		}
		spawnLow := uint32(0)
		if !c.isPlayer {
			spawnLow = uint32(c.guid & 0xFFFFFF)
		}
		if !implicitEntryConditionsMet(condRows, c.isPlayer, c.entry, spawnLow) {
			return
		}
		dist := distance3D(c.x, c.y, c.z, s.player.X, s.player.Y, s.player.Z)
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8388): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist = dist
			bestGUID = c.guid
		}
	})
	if bestGUID == 0 {
		return 0, false
	}
	return bestGUID, true
}
