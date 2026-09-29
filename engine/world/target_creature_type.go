package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// checkTargetCreatureType mirrors SpellInfo::CheckTargetCreatureType
// (SpellInfo.cpp:1868-1886) and its CheckTarget call site (SpellInfo.cpp:1728):
// the spell's TargetCreatureType mask (Spell.dbc field 17) must intersect the
// target's creature-type mask, or the cast fails. Returns 0 when the cast may
// proceed, otherwise the SPELL_FAILED_* reason C++ returns.
func (s *session) checkTargetCreatureType(ctx context.Context, spell wotlk.Spell, targetGUID uint64) uint8 {
	if targetGUID == 0 {
		return 0
	}
	mask, isPlayer := s.targetCreatureTypeMask(ctx, targetGUID)
	// Curse of Doom special case (SpellInfo.cpp:1871-1878): warlock spells of
	// category 1179 reject player targets outright, creatures always pass.
	if spell.SpellFamilyName == spellFamilyWarlock && spell.Category == 1179 {
		if isPlayer {
			return spellFailedTargetIsPlayer
		}
		return 0
	}
	if spell.TargetCreatureType == 0 || mask == 0 || mask&spell.TargetCreatureType != 0 {
		return 0
	}
	if isPlayer {
		return spellFailedTargetIsPlayer
	}
	return spellFailedBadTargets
}

// targetCreatureTypeMask mirrors Unit::GetCreatureTypeMask (Unit.cpp:9153):
// mask = (type >= 1) ? 1 << (type-1) : 0. A zero mask means the type could not
// be resolved; like C++'s !creatureType short-circuit, an unknown type does
// not fail the check.
// Gaps (not stubs): the magnet redirect (Grounding Totem, SpellInfo.cpp:1880)
// has no Go infra — the mask check always applies. NPC-bot shapeshift targets
// (Unit.cpp:9135) do not exist in Go.
func (s *session) targetCreatureTypeMask(ctx context.Context, targetGUID uint64) (uint32, bool) {
	if s.player != nil && targetGUID == s.playerGUID {
		return s.playerCreatureTypeMask(s.player), true
	}
	if s.server != nil {
		if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
			return s.playerCreatureTypeMask(targetSess.player), true
		}
	}
	entry, ok := s.targetCreatureEntry(targetGUID)
	if !ok {
		return 0, false
	}
	var creatureType int64
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, false
	}
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", entry).Scan(&creatureType); err != nil {
		return 0, false
	}
	if creatureType < 1 {
		return 0, false
	}
	return uint32(1) << (uint32(creatureType) - 1), false
}

// playerCreatureTypeMask mirrors Unit::GetCreatureType (Unit.cpp:9124-9150)
// for players: the shapeshift form's CreatureType when set (> 0), else the
// race's CreatureType (ChrRaces.dbc field 8).
func (s *session) playerCreatureTypeMask(p *playerState) uint32 {
	if p == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	if form := p.ShapeshiftForm; form != 0 {
		if shape, found, err := s.server.Data.ShapeshiftForm(uint32(form)); err == nil && found && shape.CreatureType > 0 {
			return uint32(1) << (uint32(shape.CreatureType) - 1)
		}
	}
	if race, found, err := s.server.Data.Race(uint32(p.Race)); err == nil && found && race.CreatureType >= 1 {
		return uint32(1) << (race.CreatureType - 1)
	}
	return 0
}

// targetCreatureEntry resolves a non-player unit target's template entry:
// runtime dynamic spawns from the motion map, static spawns from the entry
// embedded in the runtime creature GUID (creatureWorldGUID, creatures.go:376).
func (s *session) targetCreatureEntry(targetGUID uint64) (uint32, bool) {
	if s.server != nil && s.player != nil {
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
		s.server.motionMu.Unlock()
		if motion != nil {
			return motion.Entry, true
		}
	}
	if entry := uint32(targetGUID>>24) & 0xFFFFFF; entry != 0 {
		return entry, true
	}
	return 0, false
}
