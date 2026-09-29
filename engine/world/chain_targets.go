package world

import (
	"context"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	spellDamageClassNone   uint32 = 0 // SPELL_DAMAGE_CLASS_NONE (SharedDefines.h:1578)
	spellDamageClassMelee  uint32 = 2 // SPELL_DAMAGE_CLASS_MELEE (SharedDefines.h:1581)
	spellDamageClassRanged uint32 = 3 // SPELL_DAMAGE_CLASS_RANGED (SharedDefines.h:1582)

	spellAttr4AreaTargetChain uint32 = 0x00040000 // SPELL_ATTR4_AREA_TARGET_CHAIN (SharedDefines.h:578)

	spellImplicitTargetChainHealAlly uint32 = 45 // TARGET_UNIT_TARGET_CHAINHEAL_ALLY (SharedDefines.h:1481)
)

// chainSpellJumps returns the number of additional chain jumps for a spell and
// whether it selects allies (Chain Heal). Mirrors the maxTargets > 1 gate in
// Spell::SelectImplicitChainTargets (Spell.cpp:1593): jumps = ChainTarget - 1.
// Go shares one hit-target list across effects, so the largest ChainTarget wins.
func chainSpellJumps(spell wotlk.Spell) (jumps uint32, isChainHeal bool) {
	for _, eff := range spell.Effects {
		if eff.ChainTargets > 1 && eff.ChainTargets-1 > jumps {
			jumps = eff.ChainTargets - 1
		}
		if eff.ImplicitTargetA == spellImplicitTargetChainHealAlly || eff.ImplicitTargetB == spellImplicitTargetChainHealAlly {
			isChainHeal = true
		}
	}
	return jumps, isChainHeal
}

// chainScaledAmount ports the per-target compounding in Spell::DoEffectOnLaunchTarget
// (Spell.cpp:7771-7777): the k-th chain jump (0 = primary target) multiplies the
// effect's base damage/healing by EffectChainAmplitude^k, where the per-jump
// multiplier is SpellEffectInfo::CalcDamageMultiplier (SpellInfo.cpp:537).
// Go has no SPELLMOD_DAMAGE_MULTIPLIER spellmod infra, so the mod step is skipped.
func chainScaledAmount(base uint32, eff wotlk.SpellEffect, jumpIndex int) uint32 {
	if jumpIndex <= 0 {
		return base
	}
	return uint32(float64(base) * math.Pow(float64(eff.ChainAmplitude), float64(jumpIndex)))
}

type chainCandidate struct {
	guid    uint64
	x, y, z float32
	deficit uint32 // maxHealth - health, used by the chain-heal selection
}

// spellSearchChainTargets ports Spell::SearchChainTargets (Spell.cpp:1891).
// Starting from the primary target, it finds up to jumps additional units the
// spell jumps to, in jump order. Chain Heal picks the highest HP deficit within
// jump radius; other chains pick the closest unit. Every jump requires line of
// sight (LINEOFSIGHT_ALL_CHECKS). Non-bouncing chains only jump to targets in
// front of the caster (HasInArc(M_PI)).
func (s *session) spellSearchChainTargets(ctx context.Context, spell wotlk.Spell, primaryTargetGUID uint64, jumps uint32, isChainHeal bool) []uint64 {
	if s == nil || s.player == nil || s.server == nil || jumps == 0 {
		return nil
	}
	primary, ok := s.getCombatTarget(ctx, primaryTargetGUID)
	if !ok {
		return nil
	}
	// Spell.cpp:1897-1913. SpellInfo::DmgClass is spellEntry->DefenseType in
	// this TrinityCore version (SpellInfo.cpp:856).
	var jumpRadius float32
	switch spell.DefenseType {
	case spellDamageClassRanged:
		jumpRadius = 7.5
	case spellDamageClassMelee:
		jumpRadius = 5.0
	default: // NONE / MAGIC
		jumpRadius = 10.0
		if isChainHeal {
			jumpRadius = 12.5
		}
	}
	// Spell.cpp:1916-1918
	isBouncingFar := spell.AttributesEx4&spellAttr4AreaTargetChain != 0 ||
		spell.DefenseType == spellDamageClassNone || spell.DefenseType == spellDamageClassMagic
	searchRadius := jumpRadius
	if isBouncingFar {
		searchRadius *= float32(jumps)
	}
	candidates := s.chainCandidates(ctx, spell, primary, searchRadius, isChainHeal, primaryTargetGUID)
	if !isBouncingFar {
		// Spell.cpp:1931-1940: non-bouncing chains only jump in front of the caster
		kept := candidates[:0]
		for _, c := range candidates {
			if hasInArc(s.player.Orientation, s.player.X, s.player.Y, c.x, c.y, math.Pi) {
				kept = append(kept, c)
			}
		}
		candidates = kept
	}
	result := make([]uint64, 0, jumps)
	lx, ly, lz := primary.X, primary.Y, primary.Z
	for remaining := jumps; remaining > 0 && len(candidates) > 0; remaining-- {
		best := -1
		if isChainHeal {
			// Spell.cpp:1950-1963: unit with highest HP deficit in jump radius
			var maxDeficit uint32
			for i, c := range candidates {
				if c.deficit > maxDeficit && distance3D(c.x, c.y, c.z, lx, ly, lz) <= float64(jumpRadius) &&
					s.server.hasLineOfSight(primary.Map, lx, ly, lz, c.x, c.y, c.z) {
					best, maxDeficit = i, c.deficit
				}
			}
		} else {
			// Spell.cpp:1966-1977: closest unit (within jump radius for bouncing chains)
			for i, c := range candidates {
				d := distance3D(c.x, c.y, c.z, lx, ly, lz)
				if best == -1 {
					if (!isBouncingFar || d <= float64(jumpRadius)) &&
						s.server.hasLineOfSight(primary.Map, lx, ly, lz, c.x, c.y, c.z) {
						best = i
					}
				} else if d < distance3D(candidates[best].x, candidates[best].y, candidates[best].z, lx, ly, lz) &&
					s.server.hasLineOfSight(primary.Map, lx, ly, lz, c.x, c.y, c.z) {
					best = i
				}
			}
		}
		if best == -1 {
			break
		}
		result = append(result, candidates[best].guid)
		lx, ly, lz = candidates[best].x, candidates[best].y, candidates[best].z
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	return result
}

// chainCandidates collects units around the primary target within radius that
// pass the chain spell's target check (WorldObjectSpellAreaTargetCheck,
// Spell.cpp:8409, via WorldObjectSpellTargetCheck::operator(), Spell.cpp:8316):
// TARGET_CHECK_ENEMY ~= Unit::IsValidAttackTarget, TARGET_CHECK_ALLY ~=
// Unit::IsValidAssistTarget. Range uses the C++ cylinder test
// (IsWithinDist2d + |dz| <= range).
func (s *session) chainCandidates(ctx context.Context, spell wotlk.Spell, primary combatTarget, radius float32, isChainHeal bool, excludeGUID uint64) []chainCandidate {
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	candidates := make([]chainCandidate, 0, 16)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z float32, unitFlags, flagsExtra, health, maxHealth uint32, allyToCaster bool) {
		if guid == 0 || guid == excludeGUID {
			return
		}
		if mapID != primary.Map || instanceID != primary.InstanceID || health == 0 {
			return
		}
		if spellTargetUnitBlocked(spell, unitFlags, flagsExtra) {
			return
		}
		dx, dy := float64(x-primary.X), float64(y-primary.Y)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(z-primary.Z)) > float64(radius) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		if isChainHeal != allyToCaster {
			return
		}
		seen[guid] = struct{}{}
		var deficit uint32
		if maxHealth > health {
			deficit = maxHealth - health
		}
		candidates = append(candidates, chainCandidate{guid: guid, x: x, y: y, z: z, deficit: deficit})
	}
	allyOf := func(faction uint32) bool { return !s.server.isHostileFaction(faction, caster) }
	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(primary.Map, primary.InstanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	motionGUIDs := make(map[uint64]struct{}, len(motions))
	for _, motion := range motions {
		motionGUIDs[motion.GUID] = struct{}{}
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.UnitFlags, motion.FlagsExtra, motion.Health, motion.MaxHealth, allyOf(motion.Faction))
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		accept(targetSession.playerGUID, targetSession.player.Map, targetSession.player.InstanceID, targetSession.player.X, targetSession.player.Y, targetSession.player.Z, targetSession.player.UnitFlags, 0, targetSession.player.Health, targetSession.player.MaxHealth, targetSession.playerAlliance() == s.playerAlliance())
	}
	s.server.sessionsMu.RUnlock()
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, primary.Map, float64(primary.X-radius), float64(primary.X+radius), float64(primary.Y-radius), float64(primary.Y+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &mapID, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; !hasMotion {
						st := s.server.loadCreatureStats(ctx, uint32(entry))
						accept(guid, uint32(mapID), primary.InstanceID, float32(x), float32(y), float32(z), uint32(unitFlags), uint32(flagsExtra), uint32(health), st.MaxHealth, allyOf(uint32(faction)))
					}
				}
			}
		}
	}
	return candidates
}
