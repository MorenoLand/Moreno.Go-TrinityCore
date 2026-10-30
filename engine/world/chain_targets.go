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

	// SpellTargetCheckTypes (SpellInfo.h:116-123) — the per-implicit-target
	// selection predicate Spell::SearchChainTargets hands to
	// WorldObjectSpellTargetCheck (Spell.cpp:1891 -> 8316).
	targetCheckDefault   uint32 = 0 // TARGET_CHECK_DEFAULT
	targetCheckEntry     uint32 = 1 // TARGET_CHECK_ENTRY
	targetCheckEnemy     uint32 = 2 // TARGET_CHECK_ENEMY
	targetCheckAlly      uint32 = 3 // TARGET_CHECK_ALLY
	targetCheckParty     uint32 = 4 // TARGET_CHECK_PARTY
	targetCheckRaid      uint32 = 5 // TARGET_CHECK_RAID
	targetCheckRaidClass uint32 = 6 // TARGET_CHECK_RAID_CLASS
	targetCheckPassenger uint32 = 7 // TARGET_CHECK_PASSENGER
)

// chainSelectionEligibleTarget mirrors the SelectImplicitChainTargets call
// sites in Spell.cpp: chain selection only runs out of
// SelectImplicitNearbyTargets (1173) and SelectImplicitTargetObjectTargets
// (1575). An effect's ChainTarget > 1 is inert unless its implicit target
// has select category NEARBY (2/3/4/38/58; 40/46 are GO/dest nearby entries
// that chain no units) or is a UNIT object with a TARGET reference
// (6/21/25/35/45/57/90/95). Cone (24/54/59/60/104/108), area, channel,
// caster-dest, and caster-object effects never chain-select — Multi-Shot /
// Swipe-style multi-hit comes from the cone selection plus
// MaxAffectedTargets instead (Spell.cpp:1208-1215).
func chainSelectionEligibleTarget(target uint32) bool {
	switch target {
	case 2, 3, 4, 38, 58, // TARGET_SELECT_CATEGORY_NEARBY unit targets
		6, 21, 25, 35, 45, 57, 90, 95: // TARGET_OBJECT_TYPE_UNIT + TARGET_REFERENCE_TYPE_TARGET
		return true
	}
	return false
}

// targetObjectTarget reports the TARGET-reference UNIT object targets
// (SpellInfo.cpp:218-330): the DEFAULT-category subset of the chain set
// above. SelectImplicitTargetObjectTargets (Spell.cpp:1558-1580) runs for
// these regardless of ChainTarget, adding the explicit unit target via
// AddUnitTarget ahead of the chain search — so a ChainTarget-1 effect
// still adds the wire primary even though SelectImplicitChainTargets
// (Spell.cpp:1582) adds no jumps for it. Nearby targets (2/3/4/38/58)
// are excluded: their target is the nearby pick, not the wire target.
// Caster-object targets (1/5/27/92/94/96-103) are excluded: their target
// is the caster. GO-typed TARGET effects (23/26) add the explicit object
// target through the same arm, but a unit wire target with a GO-typed
// effect is a malformed combination with no incidence, so it stays out
// of the unit-primary scope. The area-category TARGET effect (61) is
// excluded: it selects through the area arm, not the object arm.
func targetObjectTarget(target uint32) bool {
	switch target {
	case 6, 21, 25, 35, 45, 57, 90, 95: // TARGET_OBJECT_TYPE_UNIT + TARGET_REFERENCE_TYPE_TARGET
		return true
	}
	return false
}

// nearbyUnitTarget reports the UNIT nearby implicit targets (2/3/4/38/58),
// the per-effect nearby legs of Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) whose pick C++ adds via AddUnitTarget regardless of
// ChainTarget — the chain arm (Spell.cpp:1173 -> 1582) only jumps when
// maxTargets > 1, so a ChainTarget-1 (or 0) nearby effect's pick is a
// plain primary, not a chain anchor.
func nearbyUnitTarget(target uint32) bool {
	switch target {
	case 2, 3, 4, 38, 58: // TARGET_UNIT_NEARBY_ENEMY/PARTY/ALLY/ENTRY/RAID
		return true
	}
	return false
}

// creatureChainFamilyPick maps a winning NEARBY chain target to the pick
// of its own family from a creature path's nearby block: the blocks pick
// one target per family (entry, hostile, friendly) but anchor the chain
// on a precedence pick (entry > hostile > friendly), so a winning
// friendly chain effect anchored on the entry pick when the spell also
// carries an entry leg. C++ selects per effect (Spell.cpp:761-900), so
// each nearby effect chains from its own pick. The families share one
// helper each on the creature paths (one friendly helper for 3/4/58),
// so the pick is family-scoped, not winning-effect-scoped like the
// session spellChainNearbyPick; a winning effect that is not its
// family's first selecting effect still rides the family's pick.
// Returns false when the family's pick is 0 (leg did not run or the
// spell carries no such leg) and the caller keeps its prior anchor.
func creatureChainFamilyPick(winTarget uint32, unitPick, hostilePick, friendlyPick uint64) (uint64, bool) {
	switch winTarget {
	case 38:
		if unitPick != 0 {
			return unitPick, true
		}
	case 2:
		if hostilePick != 0 {
			return hostilePick, true
		}
	case 3, 4, 58:
		if friendlyPick != 0 {
			return friendlyPick, true
		}
	}
	return 0, false
}

// creatureChainNearbyPick ports the nearby-pick half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) + SearchNearbyTarget
// (Spell.cpp:1869) for the WINNING chain effect of a creature caster, the
// motion twin of (*session).spellChainNearbyPick. C++ selects per effect
// (Spell.cpp:758-900), so the winning effect chains from its own pick:
// its check type (ENEMY/ALLY/PARTY/RAID/ENTRY) and its own
// ImplicitTargetConditions rows (effectMask), not the family helpers'
// union checks / first-effect conditions. The predicate arms mirror the
// three creature nearby helpers exactly (same range sides, totem/GM
// gates, party/raid scope via motionChainRefererScope, LOS gate); a
// miss returns nearbyPickGenuineMiss when the search ran with valid
// range data and a known check type and no candidate passed, and
// nearbyPickUnknown for range/data/unknown misses (including the LOS
// tail-drop: C++ found the target and AddUnitTarget drops it silently,
// so no :1111 fail) — the session spellChainNearbyPick split, mirrored
// here. Unknown-miss callers keep their prior anchor behavior.
func (s *Server) creatureChainNearbyPick(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32, effectMask uint32, winTarget uint32) (uint64, nearbyPickStatus) {
	if s == nil || m == nil || s.Data == nil {
		return 0, nearbyPickUnknown
	}
	var maxRange float64
	switch winTarget {
	case implicitTargetNearbyEnemy:
		rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex)
		if err != nil || !ok || rangeEntry.MaxHostile <= 0 {
			return 0, nearbyPickUnknown
		}
		maxRange = spellDBCMaxRange(false, rangeEntry)
		if modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID); modOwner != nil {
			maxRange = modOwner.applySpellModFloat(spell, spellModRange, maxRange)
		}
	case implicitTargetNearbyParty, implicitTargetNearbyAlly, implicitTargetNearbyRaid:
		rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex)
		if err != nil || !ok || rangeEntry.MaxFriendly <= 0 {
			return 0, nearbyPickUnknown
		}
		maxRange = spellDBCMaxRange(true, rangeEntry)
		if modOwner := s.spellModOwnerSession(m.Map, m.InstanceID, m.GUID); modOwner != nil {
			maxRange = modOwner.applySpellModFloat(spell, spellModRange, maxRange)
		}
	case implicitTargetNearbyEntry:
		maxRange = s.creatureEntryNearbyObjectRange(m, spell)
	default:
		return 0, nearbyPickUnknown
	}
	if maxRange <= 0 {
		return 0, nearbyPickUnknown
	}
	condRows := s.loadImplicitTargetConditionRows(ctx, spellID, effectMask)
	allowPlayers, allowCreatures := true, true
	if winTarget == implicitTargetNearbyEntry {
		allowPlayers, allowCreatures = implicitTargetEntryMask(condRows)
	}
	if spellSearchPlayersOnly(spell) {
		allowCreatures = false
	}
	allowDead := spellAllowsDeadTarget(spell)
	scope := s.motionChainRefererScope(m, m.GUID)
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	consider := func(guid uint64, x, y, z, reach float32) {
		dist := float64(distance3D(x, y, z, m.X, m.Y, m.Z)) - float64(reach)
		if dist < 0 {
			dist = 0
		}
		if dist < bestDist {
			bestDist, bestGUID, bestX, bestY, bestZ = dist, guid, x, y, z
		}
	}
	// accept applies the winning effect's check-type gates to one
	// candidate; isPlayer selects the session vs motion/DB arm's
	// faction call, ownerGUID feeds the party/raid scope. Player
	// candidates carry the full playerPos: the faction helpers read
	// Race/Class/Reputations, not just the template.
	accept := func(guid uint64, x, y, z, reach float32, unitFlags, flagsExtra, health uint32, isPlayer, isGM bool, entry, spawnLow, faction uint32, ownerGUID uint64, pp playerPos) {
		if guid == 0 {
			return
		}
		switch winTarget {
		case implicitTargetNearbyEnemy:
			if guid == m.GUID || health == 0 || isGM || s.isTotemGUID(guid) ||
				spellTargetUnitBlocked(spell, unitFlags, flagsExtra, false) {
				return
			}
			if isPlayer {
				if !s.isAttackableFaction(m.Faction, pp) {
					return
				}
			} else if !s.factionTemplatesHostile(m.Faction, faction) {
				return
			}
		case implicitTargetNearbyParty, implicitTargetNearbyAlly, implicitTargetNearbyRaid:
			if health == 0 || isGM || s.isTotemGUID(guid) ||
				spellTargetUnitBlocked(spell, unitFlags, flagsExtra, true) {
				return
			}
			if isPlayer {
				// The friendly helper's gate is non-attackable, not
				// friendly-standing: neutrals pass here as there.
				if s.isAttackableFaction(m.Faction, pp) {
					return
				}
			} else if s.factionTemplatesHostile(m.Faction, faction) {
				return
			}
			cand := friendlyCandidate{guid: guid, isPlayer: isPlayer, ownerGUID: ownerGUID, faction: faction}
			switch winTarget {
			case implicitTargetNearbyParty:
				if !scope.inParty(cand) {
					return
				}
			case implicitTargetNearbyRaid:
				if !scope.inRaid(cand) {
					return
				}
			}
		case implicitTargetNearbyEntry:
			if isPlayer && !allowPlayers {
				return
			}
			if !isPlayer && !allowCreatures {
				return
			}
			if health == 0 && !allowDead {
				return
			}
			if spellTargetUnitBlocked(spell, unitFlags, flagsExtra, false) {
				return
			}
		}
		if !implicitEntryConditionsMet(condRows, isPlayer, entry, spawnLow) {
			return
		}
		consider(guid, x, y, z, reach)
	}
	if allowPlayers {
		s.sessionsMu.RLock()
		for targetSession := range s.sessions {
			if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
				continue
			}
			p := targetSession.player
			if p.Map != m.Map || p.InstanceID != m.InstanceID {
				continue
			}
			candidate := playerPos{Map: p.Map, InstanceID: p.InstanceID, X: p.X, Y: p.Y, Z: p.Z, GUID: targetSession.playerGUID, Race: p.Race, Class: p.Class, Level: p.Level, FactionTemplate: targetSession.effectiveFactionTemplate(p.Race), Reputations: playerReputationMap(p.Reputations), Sess: targetSession}
			accept(targetSession.playerGUID, p.X, p.Y, p.Z, p.CombatReach, p.UnitFlags, 0, p.Health, true, sessionIsGameMaster(targetSession), 0, 0, candidate.FactionTemplate, 0, candidate)
		}
		s.sessionsMu.RUnlock()
	}
	if allowCreatures {
		s.motionMu.Lock()
		motionMap := s.motionMapLocked(m.Map, m.InstanceID)
		motions := make([]*creatureMotion, 0, len(motionMap))
		motionGUIDs := make(map[uint64]struct{}, len(motionMap))
		for _, cand := range motionMap {
			if cand != nil {
				motions = append(motions, cand)
				motionGUIDs[cand.GUID] = struct{}{}
			}
		}
		s.motionMu.Unlock()
		for _, cand := range motions {
			owner := cand.OwnerGUID
			if owner == 0 {
				owner = cand.CharmOwnerGUID
			}
			accept(cand.GUID, cand.X, cand.Y, cand.Z, cand.CombatReach, cand.UnitFlags, cand.FlagsExtra, cand.Health, false, false, cand.Entry, uint32(cand.GUID&0xFFFFFF), cand.Faction, owner, playerPos{})
		}
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, m.Map, float64(m.X)-maxRange, float64(m.X)+maxRange, float64(m.Y)-maxRange, float64(m.Y)+maxRange)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var low, entry, rowMap, faction, unitFlags, flagsExtra, health int64
					var x, y, z float64
					if rows.Scan(&low, &entry, &rowMap, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) != nil {
						continue
					}
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; hasMotion {
						continue
					}
					if uint32(rowMap) != m.Map {
						continue
					}
					accept(guid, float32(x), float32(y), float32(z), 0, uint32(unitFlags), uint32(flagsExtra), uint32(health), false, false, uint32(entry), uint32(low), uint32(faction), 0, playerPos{})
				}
			}
		}
	}
	if bestGUID == 0 {
		return 0, nearbyPickGenuineMiss
	}
	if !s.implicitUnitTargetLOSPassesForMotion(spell, m, bestGUID, bestX, bestY, bestZ) {
		return 0, nearbyPickUnknown
	}
	return bestGUID, nearbyPickFound
}

// spellHasTargetObjectEffect reports whether any live effect of the spell
// carries a TARGET-reference UNIT object implicit target, the scope of
// the plain-primary rejoin on the triggered path.
func spellHasTargetObjectEffect(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 && eff.Aura == 0 {
			continue
		}
		if targetObjectTarget(eff.ImplicitTargetA) || targetObjectTarget(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// chainCheckType maps a chain-eligible implicit target to its
// SpellImplicitTargetInfo::GetCheckType (the SelectionCheckType column of
// the SpellInfo.cpp:218-330 table) — the predicate
// WorldObjectSpellTargetCheck::operator() applies per candidate
// (Spell.cpp:8316-8384).
func chainCheckType(target uint32) uint32 {
	switch target {
	case 2, 6: // TARGET_UNIT_NEARBY_ENEMY, TARGET_UNIT_TARGET_ENEMY
		return targetCheckEnemy
	case 4, 21, 45: // TARGET_UNIT_NEARBY_ALLY, TARGET_UNIT_TARGET_ALLY, TARGET_UNIT_TARGET_CHAINHEAL_ALLY
		return targetCheckAlly
	case 3, 35: // TARGET_UNIT_NEARBY_PARTY, TARGET_UNIT_TARGET_PARTY
		return targetCheckParty
	case 57, 58: // TARGET_UNIT_TARGET_RAID, TARGET_UNIT_NEARBY_RAID
		return targetCheckRaid
	case 38: // TARGET_UNIT_NEARBY_ENTRY
		return targetCheckEntry
	case 95: // TARGET_UNIT_TARGET_PASSENGER
		return targetCheckPassenger
	default: // 25 TARGET_UNIT_TARGET_ANY, 90 TARGET_UNIT_TARGET_MINIPET
		return targetCheckDefault
	}
}

// chainSpellJumps returns the number of additional chain jumps for a spell,
// whether it selects allies (Chain Heal), the winning effect's target check
// type, and the winning effect's mask (for the ImplicitTargetConditions
// load). Mirrors the maxTargets > 1 gate in
// Spell::SelectImplicitChainTargets (Spell.cpp:1582-1624): the player's
// SPELLMOD_JUMP_TARGETS modifiers fold into ChainTarget BEFORE the >1 gate
// (C++ applies ApplySpellMod to maxTargets at 1585-1588, then checks
// maxTargets > 1 at 1593), so jumps = moddedChainTarget - 1. Creature casters
// have no spell-mod owner, so the fold is a pass-through when s carries no
// mods. Go shares one hit-target list across effects, so the largest modded
// ChainTarget among chain-eligible effects wins, and its check type routes
// the candidate predicate (C++ runs the search per effect with
// targetType.GetCheckType(), Spell.cpp:1602). Effects whose ChainTarget > 1
// rides a non-eligible implicit target (cone, area, caster-object,
// caster-dest) contribute nothing, matching the C++ call sites above.
func chainSpellJumps(s *session, spell wotlk.Spell) (jumps uint32, isChainHeal bool, checkType uint32, effectMask uint32) {
	var winTarget uint32
	for i, eff := range spell.Effects {
		if chainSelectionEligibleTarget(eff.ImplicitTargetA) || chainSelectionEligibleTarget(eff.ImplicitTargetB) {
			// Spell.cpp:1585-1593: the mod folds into maxTargets before the
			// >1 gate, so it runs even when the raw ChainTarget is 0 or 1.
			maxTargets := int32(eff.ChainTargets)
			if s != nil {
				maxTargets = s.applySpellMod(spell, spellModJumpTargets, maxTargets)
			}
			if moddedJumps := maxTargets - 1; moddedJumps > int32(jumps) && moddedJumps > 0 {
				jumps = uint32(moddedJumps)
				effectMask = 1 << uint(i)
				winTarget = eff.ImplicitTargetA
				if !chainSelectionEligibleTarget(winTarget) {
					winTarget = eff.ImplicitTargetB
				}
				isChainHeal = winTarget == spellImplicitTargetChainHealAlly
			}
		}
	}
	return jumps, isChainHeal, chainCheckType(winTarget), effectMask
}

// winningSecondChainLeg returns the winning effect's second chain leg
// when the effect carries two chain-eligible legs and the second leg's
// search never ran: one TARGET-object leg and one NEARBY leg, or two
// TARGET-object legs with differing check types (Spell.cpp:758-779 selects
// TargetA and TargetB separately, each ending in its own chain arm at
// :1173/:1575). The winning blocks pick the first eligible leg (A else B),
// so such a winning effect chained only its first leg. Two TARGET-object
// legs share one anchor (the wire target) but run separate chain searches
// with their own check types and chain-heal flags — DEFAULT never marks the
// effect processed (Spell.cpp:866-900), so both object arms run. Nearby+
// nearby pairs stay out: a NEARBY first slot marks the effect processed, so
// the second NEARBY slot never runs (same :866-900 arm) and A-only chaining
// is correct there, not a gap. Returns false when the legs are identical
// (one search already covers both) or the second leg is not chain-eligible.
func winningSecondChainLeg(spell wotlk.Spell, effectMask uint32) (uint32, bool) {
	for i, eff := range spell.Effects {
		if effectMask != 1<<uint(i) {
			continue
		}
		first := eff.ImplicitTargetA
		if !chainSelectionEligibleTarget(first) {
			first = eff.ImplicitTargetB
		}
		if !chainSelectionEligibleTarget(first) {
			return 0, false
		}
		second := eff.ImplicitTargetB
		if first == eff.ImplicitTargetB {
			second = eff.ImplicitTargetA
		}
		if !chainSelectionEligibleTarget(second) {
			return 0, false
		}
		if (targetObjectTarget(first) && nearbyUnitTarget(second)) || (nearbyUnitTarget(first) && targetObjectTarget(second)) {
			return second, true
		}
		if targetObjectTarget(first) && targetObjectTarget(second) && first != second {
			return second, true
		}
		return 0, false
	}
	return 0, false
}

// spellChainNearbyPick ports the nearby-pick half of
// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036) + SearchNearbyTarget
// (Spell.cpp:1869) for the WINNING chain effect only. C++ selects per effect
// (Spell.cpp:758-900), so a NEARBY chain effect in an area, friendly-list,
// or GO spell anchors its jumps on its own pick even though no nearby branch
// ran (the area/list branch won Go's else-if selection chain). The spell-wide
// pick helpers cannot serve here: spellFriendlyNearbyTarget unions
// ALLY/PARTY/RAID checks across effects, and spellHostileNearbyTarget /
// spellEntryNearbyTarget carry the FIRST selecting effect's conditions, while
// the chain pick must carry the winning effect's check type and conditions
// (Spell.cpp:1066, the selecting effect's ImplicitTargetConditions). The
// predicate arms mirror the three helpers exactly (same range sides,
// totem/GM gates, LOS gate); a miss returns nearbyPickGenuineMiss when the search ran with valid
// range data and a known check type and no candidate passed, and
// nearbyPickUnknown for range/data/unknown misses (including the LOS
// tail-drop: C++ found the target and AddUnitTarget drops it silently,
// so no :1111 fail); unknown-miss callers keep their prior anchor behavior.
// nearbyPickStatus distinguishes a scoped nearby pick's miss kinds for
// the :1111 fail-cast arm: only a genuine miss fails the cast.
type nearbyPickStatus uint8

const (
	nearbyPickFound nearbyPickStatus = iota
	nearbyPickGenuineMiss
	nearbyPickUnknown
)

func (s *session) spellChainNearbyPick(ctx context.Context, spell wotlk.Spell, spellID uint32, effectMask uint32, winTarget uint32) (uint64, nearbyPickStatus) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, nearbyPickUnknown
	}
	rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok {
		return 0, nearbyPickUnknown
	}
	var maxRange float64
	switch winTarget {
	case implicitTargetNearbyEnemy:
		if rangeEntry.MaxHostile <= 0 {
			return 0, nearbyPickUnknown
		}
		maxRange = float64(rangeEntry.MaxHostile)
	case implicitTargetNearbyParty, implicitTargetNearbyAlly, implicitTargetNearbyRaid:
		if rangeEntry.MaxFriendly <= 0 {
			return 0, nearbyPickUnknown
		}
		maxRange = float64(rangeEntry.MaxFriendly)
	case implicitTargetNearbyEntry:
		maxRange = float64(rangeEntry.MaxHostile)
		if spellIsPositive(spell) {
			maxRange = float64(rangeEntry.MaxFriendly)
		}
	default:
		return 0, nearbyPickUnknown
	}
	// SelectImplicitNearbyTargets (Spell.cpp:1047-1057) ranges the search
	// with GetMaxRange, which folds SPELLMOD_RANGE (SpellInfo.cpp:3070-3072).
	maxRange = s.applySpellModFloat(spell, spellModRange, maxRange)
	if maxRange <= 0 {
		return 0, nearbyPickUnknown
	}
	// WorldObject::IsValidAttackTarget (Object.cpp:2977): a UBER-flagged
	// player attacker cannot attack, so an ENEMY pick selects nothing; the
	// rejection sits inside the search predicate (Spell.cpp:8341), so this
	// is a genuine miss, not a data miss.
	if winTarget == implicitTargetNearbyEnemy && sessionIsUber(s) {
		return 0, nearbyPickGenuineMiss
	}
	condRows := s.loadImplicitTargetConditions(ctx, spellID, effectMask)
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	allyOf := func(faction uint32) bool { return !s.server.isHostileFaction(faction, caster) }
	scope := s.friendlyRefererScope(s.playerGUID)
	allowPlayers, allowCreatures := implicitTargetEntryMask(condRows)
	allowDead := spellAllowsDeadTarget(spell)
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	s.friendlyScanCandidates(ctx, s.player.X, s.player.Y, float32(maxRange), spell, func(c friendlyCandidate) {
		switch winTarget {
		case implicitTargetNearbyEnemy:
			if c.guid == s.playerGUID || c.mapID != caster.Map || c.instanceID != caster.InstanceID ||
				c.health == 0 || spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, false) ||
				s.server.isTotemGUID(c.guid) {
				return
			}
			if c.isPlayer {
				if c.isGM || c.alliance {
					return
				}
			} else if !s.server.isAttackableFaction(c.faction, caster) {
				return
			}
		case implicitTargetNearbyParty, implicitTargetNearbyAlly, implicitTargetNearbyRaid:
			// TARGET_CHECK_ALLY/PARTY/RAID all reject totems (Spell.cpp:8345/
			// 8354/8371); the gate sits before the nearest-shrink step.
			if s.server.isTotemGUID(c.guid) || !s.friendlyAssistOK(spell, c, caster, allyOf) {
				return
			}
			switch winTarget {
			case implicitTargetNearbyParty:
				if !scope.inParty(c) {
					return
				}
			case implicitTargetNearbyRaid:
				if !scope.inRaid(c) {
					return
				}
			}
		case implicitTargetNearbyEntry:
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
		}
		// Base WorldObjectSpellTargetCheck tail (Spell.cpp:8380-8387):
		// a candidate failing the effect's conditions never becomes the
		// nearest pick and never shrinks the search range.
		spawnLow := uint32(0)
		if !c.isPlayer {
			spawnLow = uint32(c.guid & 0xFFFFFF)
		}
		if !implicitEntryConditionsMet(condRows, c.isPlayer, c.entry, spawnLow) {
			return
		}
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8394): strict less-than,
		// shrinking to the nearest match. GetDistance (Object.cpp:1175)
		// subtracts the candidate's combat reach and floors at 0.
		dist := float64(distance3D(c.x, c.y, c.z, s.player.X, s.player.Y, s.player.Z)) - float64(c.reach)
		if dist < 0 {
			dist = 0
		}
		if dist < bestDist {
			bestDist = dist
			bestGUID = c.guid
			bestX, bestY, bestZ = c.x, c.y, c.z
		}
	})
	if bestGUID == 0 {
		return 0, nearbyPickGenuineMiss
	}
	// Spell::CheckEffectTarget (Spell.cpp:1130: AddUnitTarget with
	// checkIfValid=true) drops a nearby unit target without line of sight
	// to the caster.
	if !s.implicitUnitTargetLOSPasses(spell, bestGUID, bestX, bestY, bestZ) {
		return 0, nearbyPickUnknown
	}
	return bestGUID, nearbyPickFound
}

// chainScaledAmount applies the per-jump falloff of a chain spell:
// base * multiplier^jumpIndex, mirroring the m_damageMultipliers
// accumulation in Spell::DoEffectOnLaunchTarget (Spell.cpp:7771-7774).
// multiplier is the SPELLMOD_DAMAGE_MULTIPLIER-adjusted EffectChainAmplitude
// from SpellEffectInfo::CalcDamageMultiplier (SpellInfo.cpp:537-544),
// computed once per effect by the caller.
func chainScaledAmount(base uint32, multiplier float64, jumpIndex int) uint32 {
	if jumpIndex <= 0 {
		return base
	}
	return uint32(float64(base) * math.Pow(multiplier, float64(jumpIndex)))
}

type chainCandidate struct {
	guid    uint64
	x, y, z float32
	deficit uint32  // maxHealth - health, used by the chain-heal selection
	reach   float32 // combat reach: IsWithinDist adds both ends' reaches (Object.cpp:1147)
}

// gameObjectAnchorPosition resolves a gameobject GUID to a chain-anchor
// position: Spell::SelectImplicitTargetObjectTargets (Spell.cpp:1575)
// chains from the explicit object target whatever its type, so a
// gameobject wire target anchors SearchChainTargets (Spell.cpp:1891) at
// the gameobject's position while the jumps themselves stay units
// (SelectImplicitChainTargets adds only ToUnit results, Spell.cpp:1622).
// A live dynamic state (summoned gameobjects, materialized static spawns)
// wins over the static DB row, which carries no instance; a gameobject
// with neither stays unresolvable and chains nothing. Gameobjects carry
// no combat reach (GameObject::Create never sets one), so callers use a
// synthetic anchor reach of 0 — C++ IsWithinDist (Object.cpp:1171) then
// measures from the gameobject's center plus the jump candidate's reach.
func (s *Server) gameObjectAnchorPosition(ctx context.Context, guid uint64, mapID, instanceID uint32) (x, y, z float32, ok bool) {
	if s == nil || uint16(guid>>48) != 0xF110 {
		return 0, 0, 0, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if st := s.gameObjectState(mapID, instanceID, guid); st != nil {
		return st.X, st.Y, st.Z, true
	}
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		var goMap int64
		var gx, gy, gz float64
		if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT map, position_x, position_y, position_z FROM gameobject WHERE guid = ?", uint32(guid&0xFFFFFF)).Scan(&goMap, &gx, &gy, &gz); err == nil && uint32(goMap) == mapID {
			return float32(gx), float32(gy), float32(gz), true
		}
	}
	return 0, 0, 0, false
}

// spellSearchChainTargets ports Spell::SearchChainTargets (Spell.cpp:1891).
// Starting from the primary target, it finds up to jumps additional units the
// spell jumps to, in jump order. Chain Heal picks the highest HP deficit within
// jump radius; other chains pick the closest unit. Every jump requires line of
// sight (LINEOFSIGHT_ALL_CHECKS). Non-bouncing chains only jump to targets in
// front of the caster (HasInArc(M_PI)).
func (s *session) spellSearchChainTargets(ctx context.Context, spell wotlk.Spell, spellID uint32, primaryTargetGUID uint64, jumps uint32, isChainHeal bool, checkType uint32, effectMask uint32) []uint64 {
	if s == nil || s.player == nil || s.server == nil || jumps == 0 {
		return nil
	}
	primary, ok := s.getCombatTarget(ctx, primaryTargetGUID)
	if !ok {
		// A gameobject primary anchors the search at its position
		// (Spell.cpp:1575 -> 1891); unit-only resolution dropped every
		// jump for such spells. The synthetic anchor carries no combat
		// reach and no health — the search reads only its position,
		// map/instance, and reach.
		if gx, gy, gz, goOK := s.server.gameObjectAnchorPosition(ctx, primaryTargetGUID, s.player.Map, s.player.InstanceID); goOK {
			primary = combatTarget{GUID: primaryTargetGUID, Map: s.player.Map, InstanceID: s.player.InstanceID, X: gx, Y: gy, Z: gz}
			ok = true
		}
	}
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
	condRows := s.loadImplicitTargetConditions(ctx, spellID, effectMask)
	scope := s.friendlyRefererScope(s.playerGUID)
	candidates := s.chainCandidates(ctx, spell, primary, searchRadius, primaryTargetGUID, checkType, condRows, scope)
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
	nodeReach := primary.CombatReach
	for remaining := jumps; remaining > 0 && len(candidates) > 0; remaining-- {
		best := -1
		// Object.cpp:1147-1164: IsWithinDist folds BOTH units' combat reaches
		// into the radius and compares with strict less-than.
		if isChainHeal {
			// Spell.cpp:1950-1963: unit with highest HP deficit in jump radius
			var maxDeficit uint32
			for i, c := range candidates {
				if c.deficit > maxDeficit && distance3D(c.x, c.y, c.z, lx, ly, lz) < float64(jumpRadius)+float64(nodeReach)+float64(c.reach) &&
					s.server.hasLineOfSightSpellCheck(primary.Map, lx, ly, lz, c.x, c.y, c.z) &&
					// Spell::CheckEffectTarget default arm (Spell.cpp:7483-7492):
					// chain selection passes losPosition == nullptr, so every
					// jump target also needs line of sight from the caster.
					s.implicitUnitTargetLOSPasses(spell, c.guid, c.x, c.y, c.z) {
					best, maxDeficit = i, c.deficit
				}
			}
		} else {
			// Spell.cpp:1966-1977: closest unit (within jump radius for bouncing chains)
			for i, c := range candidates {
				d := distance3D(c.x, c.y, c.z, lx, ly, lz)
				// Spell::CheckEffectTarget default arm (Spell.cpp:7483-7492):
				// chain selection passes losPosition == nullptr, so every
				// jump target also needs line of sight from the caster.
				if best == -1 {
					if (!isBouncingFar || d < float64(jumpRadius)+float64(nodeReach)+float64(c.reach)) &&
						s.server.hasLineOfSightSpellCheck(primary.Map, lx, ly, lz, c.x, c.y, c.z) &&
						s.implicitUnitTargetLOSPasses(spell, c.guid, c.x, c.y, c.z) {
						best = i
					}
				} else if d < distance3D(candidates[best].x, candidates[best].y, candidates[best].z, lx, ly, lz) &&
					s.server.hasLineOfSightSpellCheck(primary.Map, lx, ly, lz, c.x, c.y, c.z) &&
					s.implicitUnitTargetLOSPasses(spell, c.guid, c.x, c.y, c.z) {
					best = i
				}
			}
		}
		if best == -1 {
			break
		}
		result = append(result, candidates[best].guid)
		lx, ly, lz = candidates[best].x, candidates[best].y, candidates[best].z
		nodeReach = candidates[best].reach
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	return result
}

// chainCandidates collects units around the primary target within radius that
// pass the chain spell's target check (WorldObjectSpellAreaTargetCheck,
// Spell.cpp:8409, via WorldObjectSpellTargetCheck::operator(), Spell.cpp:8316).
// The check type routes the per-candidate predicate: TARGET_CHECK_ENEMY ~=
// Unit::IsValidAttackTarget, TARGET_CHECK_ALLY ~= Unit::IsValidAssistTarget,
// PARTY/RAID add the caster-referer group test (Unit::IsInPartyWith /
// IsInRaidWith, Unit.cpp:12126/12155), ENTRY runs only the effect's entry
// conditions, DEFAULT/PASSENGER apply no faction filter. Range uses the C++
// cylinder test (IsWithinDist2d + |dz| <= range).
func (s *session) chainCandidates(ctx context.Context, spell wotlk.Spell, primary combatTarget, radius float32, excludeGUID uint64, checkType uint32, condRows []conditionRow, scope friendlyRefererScope) []chainCandidate {
	// WorldObject::IsValidAttackTarget (Object.cpp:2977): a UBER-flagged
	// player attacker cannot attack, so an ENEMY chain selects no jump
	// targets. Assist-family chains route through IsValidAssistTarget,
	// which carries no UBER arm, and are untouched.
	if checkType == targetCheckEnemy && sessionIsUber(s) {
		return nil
	}
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	candidates := make([]chainCandidate, 0, 16)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z float32, unitFlags, flagsExtra, health, maxHealth uint32, allyToCaster bool, isPlayer bool, isGM bool, entry, spawnLow, faction uint32, ownerGUID uint64, reach float32) {
		if guid == 0 || guid == excludeGUID {
			return
		}
		// SpellInfo::CheckTarget (SpellInfo.cpp:1637) opens
		// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8318), so
		// SPELL_ATTR1_CANT_TARGET_SELF rejects the caster from implicit
		// chain selection too.
		if spell.AttributesEx&spellAttr1CantTargetSelf != 0 && guid == s.playerGUID {
			return
		}
		if mapID != primary.Map || instanceID != primary.InstanceID || health == 0 {
			return
		}
		if spellTargetUnitBlocked(spell, unitFlags, flagsExtra, checkType == targetCheckAlly || checkType == targetCheckParty || checkType == targetCheckRaid) {
			return
		}
		// Spell.cpp:8333/8344/8354/8365: only the ENEMY/ALLY/PARTY/RAID
		// check types reject totems; DEFAULT/ENTRY/PASSENGER do not.
		switch checkType {
		case targetCheckEnemy, targetCheckAlly, targetCheckParty, targetCheckRaid:
			if s.server.isTotemGUID(guid) {
				return
			}
		}
		// WorldObject::IsValidAttackTarget (Object.cpp:2946) and
		// IsValidAssistTarget (Object.cpp:3103) both refuse GM players,
		// so the ENEMY/ALLY/PARTY/RAID check types (Spell.cpp:8336-8374)
		// never select one; DEFAULT/ENTRY/PASSENGER route through no
		// IsValid* call and keep them. The assist self arm precedes the
		// GM arm (Object.cpp:3095-3103), so the caster themself stays a
		// candidate on ALLY/PARTY/RAID; the attack self arm only fires
		// with no spell (Object.cpp:2937-2940), so a GM caster is out
		// even on ENEMY.
		if isPlayer && isGM {
			switch checkType {
			case targetCheckEnemy:
				return
			case targetCheckAlly, targetCheckParty, targetCheckRaid:
				if guid != s.playerGUID {
					return
				}
			}
		}
		cand := friendlyCandidate{guid: guid, isPlayer: isPlayer, ownerGUID: ownerGUID, entry: entry, faction: faction}
		switch checkType {
		case targetCheckEnemy:
			// TARGET_CHECK_ENEMY: Unit::IsValidAttackTarget (Spell.cpp:8336).
			if allyToCaster {
				return
			}
		case targetCheckAlly:
			// TARGET_CHECK_ALLY: Unit::IsValidAssistTarget (Spell.cpp:8347).
			if !allyToCaster {
				return
			}
		case targetCheckParty:
			// TARGET_CHECK_PARTY: assist target in the caster's party
			// (Spell.cpp:8351-8359; Unit::IsInPartyWith, Unit.cpp:12126).
			if !allyToCaster || !scope.inParty(cand) {
				return
			}
		case targetCheckRaid:
			// TARGET_CHECK_RAID: assist target in the caster's raid
			// (Spell.cpp:8361-8374; Unit::IsInRaidWith, Unit.cpp:12155).
			if !allyToCaster || !scope.inRaid(cand) {
				return
			}
		case targetCheckEntry:
			// TARGET_CHECK_ENTRY has no faction arm (Spell.cpp:8316: no
			// case) — CheckTarget plus the effect's entry conditions only.
		default:
			// TARGET_CHECK_DEFAULT / TARGET_CHECK_PASSENGER: no faction
			// filter (Spell.cpp:8316: default case).
		}
		// Spell.cpp:8381-8384: the effect's ImplicitTargetConditions gate
		// every check type.
		if !implicitEntryConditionsMet(condRows, isPlayer, entry, spawnLow) {
			return
		}
		dx, dy := float64(x-primary.X), float64(y-primary.Y)
		reachRadius := float64(radius) + float64(reach)
		if dx*dx+dy*dy >= reachRadius*reachRadius || math.Abs(float64(z-primary.Z)) > float64(radius) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		seen[guid] = struct{}{}
		var deficit uint32
		if maxHealth > health {
			deficit = maxHealth - health
		}
		candidates = append(candidates, chainCandidate{guid: guid, x: x, y: y, z: z, deficit: deficit, reach: reach})
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
	// SearchChainTargets -> SearchAreaTargets -> Spell::GetSearcherTypeMask
	// (Spell.cpp:1891/1881/1809): SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS drop the CREATURE container.
	playersOnly := spellSearchPlayersOnly(spell)
	for _, motion := range motions {
		if playersOnly {
			continue
		}
		motionGUIDs[motion.GUID] = struct{}{}
		motionOwner := motion.OwnerGUID
		if motionOwner == 0 {
			motionOwner = motion.CharmOwnerGUID
		}
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.UnitFlags, motion.FlagsExtra, motion.Health, motion.MaxHealth, allyOf(motion.Faction), false, false, motion.Entry, 0, motion.Faction, motionOwner, motion.CombatReach)
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		accept(targetSession.playerGUID, targetSession.player.Map, targetSession.player.InstanceID, targetSession.player.X, targetSession.player.Y, targetSession.player.Z, targetSession.player.UnitFlags, 0, targetSession.player.Health, targetSession.player.MaxHealth, targetSession.playerAlliance() == s.playerAlliance(), true, sessionIsGameMaster(targetSession), 0, 0, 0, 0, targetSession.player.CombatReach)
	}
	s.server.sessionsMu.RUnlock()
	// The CREATURE container is dropped by the GetSearcherTypeMask attribute arm above.
	if !playersOnly && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
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
						accept(guid, uint32(mapID), primary.InstanceID, float32(x), float32(y), float32(z), uint32(unitFlags), uint32(flagsExtra), uint32(health), st.MaxHealth, allyOf(uint32(faction)), false, false, uint32(entry), uint32(low), uint32(faction), 0, st.CombatReach)
					}
				}
			}
		}
	}
	return candidates
}

// creatureFactionHostileTo ports the creature-caster direction of the
// WorldObjectSpellTargetCheck faction arms
// (WorldObjectSpellTargetCheck::operator(), Spell.cpp:8316-8384): the ENEMY
// arm is !IsValidAttackTarget and the ALLY arm is IsValidAssistTarget, both
// of which bottom out in the faction-template reaction
// (FactionTemplateEntry::IsHostileTo, DBCStructure.h:710-722). When DBC
// faction-template data is unavailable the fallback treats a different
// template as hostile and the caster's own template as not — the C++
// zeroed-entry read of IsHostileTo degrades the same way (no enemies, no
// enemy group).
func (s *Server) creatureFactionHostileTo(casterFaction, targetFaction uint32) bool {
	if s != nil && s.Data != nil && casterFaction != 0 && targetFaction != 0 {
		if casterTpl, ok, err := s.Data.FactionTemplate(casterFaction); err == nil && ok {
			if targetTpl, ok, err := s.Data.FactionTemplate(targetFaction); err == nil && ok {
				return factionTemplateHostileTo(casterTpl, targetTpl)
			}
		}
	}
	return casterFaction != targetFaction
}

// spellSearchChainTargetsForMotion is the Server-level twin of
// (*session).spellSearchChainTargets for a creature caster: the search roots
// at the motion's position, orientation and faction instead of the player
// session. Spell::SelectImplicitChainTargets (Spell.cpp:1582-1624) carries
// no caster-type gate — SelectSpellTargets (Spell.cpp:3410) runs it for
// creature casts in _cast (Spell.cpp:3410) — so creature-cast chain spells
// (Black Stalker 31717 via creature:CastSpell) jump in C++ but never did in
// Go, where the chain machinery was reachable only from the two
// player-session paths.
func (s *Server) spellSearchChainTargetsForMotion(ctx context.Context, m *creatureMotion, spell wotlk.Spell, spellID uint32, primaryTargetGUID uint64, jumps uint32, isChainHeal bool, checkType uint32, effectMask uint32) []uint64 {
	if s == nil || m == nil || jumps == 0 {
		return nil
	}
	primary, ok := s.combatTargetByGUID(ctx, m.Map, m.InstanceID, primaryTargetGUID)
	if !ok {
		// Gameobject primary, same as the session twin: the search
		// anchors at the gameobject's position with zero reach.
		if gx, gy, gz, goOK := s.gameObjectAnchorPosition(ctx, primaryTargetGUID, m.Map, m.InstanceID); goOK {
			primary = combatTarget{GUID: primaryTargetGUID, Map: m.Map, InstanceID: m.InstanceID, X: gx, Y: gy, Z: gz}
			ok = true
		}
	}
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
	condRows := s.loadImplicitTargetConditionRows(ctx, spellID, effectMask)
	scope := s.motionChainRefererScope(m, m.GUID)
	candidates := s.chainCandidatesForMotion(ctx, m, spell, primary, searchRadius, primaryTargetGUID, checkType, condRows, scope)
	if !isBouncingFar {
		// Spell.cpp:1931-1940: non-bouncing chains only jump in front of the caster
		kept := candidates[:0]
		for _, c := range candidates {
			if hasInArc(m.Orientation, m.X, m.Y, c.x, c.y, math.Pi) {
				kept = append(kept, c)
			}
		}
		candidates = kept
	}
	result := make([]uint64, 0, jumps)
	lx, ly, lz := primary.X, primary.Y, primary.Z
	nodeReach := primary.CombatReach
	for remaining := jumps; remaining > 0 && len(candidates) > 0; remaining-- {
		best := -1
		// Object.cpp:1147-1164: IsWithinDist folds BOTH units' combat reaches
		// into the radius and compares with strict less-than.
		if isChainHeal {
			// Spell.cpp:1950-1963: unit with highest HP deficit in jump radius
			var maxDeficit uint32
			for i, c := range candidates {
				if c.deficit > maxDeficit && distance3D(c.x, c.y, c.z, lx, ly, lz) < float64(jumpRadius)+float64(nodeReach)+float64(c.reach) &&
					s.hasLineOfSightSpellCheck(primary.Map, lx, ly, lz, c.x, c.y, c.z) &&
					// Spell::CheckEffectTarget default arm (Spell.cpp:7483-7492):
					// chain selection passes losPosition == nullptr, so every
					// jump target also needs line of sight from the caster.
					s.implicitUnitTargetLOSPassesForMotion(spell, m, c.guid, c.x, c.y, c.z) {
					best, maxDeficit = i, c.deficit
				}
			}
		} else {
			// Spell.cpp:1966-1977: closest unit (within jump radius for bouncing chains)
			for i, c := range candidates {
				d := distance3D(c.x, c.y, c.z, lx, ly, lz)
				// Spell::CheckEffectTarget default arm (Spell.cpp:7483-7492):
				// chain selection passes losPosition == nullptr, so every
				// jump target also needs line of sight from the caster.
				if best == -1 {
					if (!isBouncingFar || d < float64(jumpRadius)+float64(nodeReach)+float64(c.reach)) &&
						s.hasLineOfSightSpellCheck(primary.Map, lx, ly, lz, c.x, c.y, c.z) &&
						s.implicitUnitTargetLOSPassesForMotion(spell, m, c.guid, c.x, c.y, c.z) {
						best = i
					}
				} else if d < distance3D(candidates[best].x, candidates[best].y, candidates[best].z, lx, ly, lz) &&
					s.hasLineOfSightSpellCheck(primary.Map, lx, ly, lz, c.x, c.y, c.z) &&
					s.implicitUnitTargetLOSPassesForMotion(spell, m, c.guid, c.x, c.y, c.z) {
					best = i
				}
			}
		}
		if best == -1 {
			break
		}
		result = append(result, candidates[best].guid)
		lx, ly, lz = candidates[best].x, candidates[best].y, candidates[best].z
		nodeReach = candidates[best].reach
		candidates = append(candidates[:best], candidates[best+1:]...)
	}
	return result
}

// chainCandidatesForMotion is the Server-level twin of
// (*session).chainCandidates for a creature caster. The per-candidate
// faction predicates use the creature-rooted gates: ENEMY is the caster's
// attackable check (isAttackableFaction, the same call shape as the
// creature's own aggro pick), ALLY is the friendly check
// (isFriendlyFaction); creature candidates resolve through the
// faction-template reaction (creatureFactionHostileTo). PARTY/RAID ride the
// motion-rooted referer scope.
func (s *Server) chainCandidatesForMotion(ctx context.Context, m *creatureMotion, spell wotlk.Spell, primary combatTarget, radius float32, excludeGUID uint64, checkType uint32, condRows []conditionRow, scope friendlyRefererScope) []chainCandidate {
	candidates := make([]chainCandidate, 0, 16)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z float32, unitFlags, flagsExtra, health, maxHealth uint32, hostileToCaster, allyToCaster bool, isPlayer bool, isGM bool, entry, spawnLow, faction uint32, ownerGUID uint64, reach float32) {
		if guid == 0 || guid == excludeGUID {
			return
		}
		// SpellInfo::CheckTarget (SpellInfo.cpp:1637) opens
		// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8318), so
		// SPELL_ATTR1_CANT_TARGET_SELF rejects the caster from implicit
		// chain selection too.
		if spell.AttributesEx&spellAttr1CantTargetSelf != 0 && guid == m.GUID {
			return
		}
		if mapID != primary.Map || instanceID != primary.InstanceID || health == 0 {
			return
		}
		if spellTargetUnitBlocked(spell, unitFlags, flagsExtra, checkType == targetCheckAlly || checkType == targetCheckParty || checkType == targetCheckRaid) {
			return
		}
		// Spell.cpp:8333/8344/8354/8365: only the ENEMY/ALLY/PARTY/RAID
		// check types reject totems; DEFAULT/ENTRY/PASSENGER do not.
		switch checkType {
		case targetCheckEnemy, targetCheckAlly, targetCheckParty, targetCheckRaid:
			if s.isTotemGUID(guid) {
				return
			}
		}
		// Same GM arm as the session twin (Object.cpp:2946 attack,
		// Object.cpp:3103 assist): the creature caster is never a player
		// GUID, so the caster-self exemption on ALLY/PARTY/RAID can
		// never fire here, but the shape is kept uniform.
		if isPlayer && isGM {
			switch checkType {
			case targetCheckEnemy:
				return
			case targetCheckAlly, targetCheckParty, targetCheckRaid:
				if guid != m.GUID {
					return
				}
			}
		}
		cand := friendlyCandidate{guid: guid, isPlayer: isPlayer, ownerGUID: ownerGUID, entry: entry, faction: faction}
		switch checkType {
		case targetCheckEnemy:
			// TARGET_CHECK_ENEMY: Unit::IsValidAttackTarget (Spell.cpp:8336).
			if !hostileToCaster {
				return
			}
		case targetCheckAlly:
			// TARGET_CHECK_ALLY: Unit::IsValidAssistTarget (Spell.cpp:8347).
			if !allyToCaster {
				return
			}
		case targetCheckParty:
			// TARGET_CHECK_PARTY: assist target in the caster's party
			// (Spell.cpp:8351-8359; Unit::IsInPartyWith, Unit.cpp:12126).
			if !allyToCaster || !scope.inParty(cand) {
				return
			}
		case targetCheckRaid:
			// TARGET_CHECK_RAID: assist target in the caster's raid
			// (Spell.cpp:8361-8374; Unit::IsInRaidWith, Unit.cpp:12155).
			if !allyToCaster || !scope.inRaid(cand) {
				return
			}
		case targetCheckEntry:
			// TARGET_CHECK_ENTRY has no faction arm (Spell.cpp:8316: no
			// case) — CheckTarget plus the effect's entry conditions only.
		default:
			// TARGET_CHECK_DEFAULT / TARGET_CHECK_PASSENGER: no faction
			// filter (Spell.cpp:8316: default case).
		}
		// Spell.cpp:8381-8384: the effect's ImplicitTargetConditions gate
		// every check type.
		if !implicitEntryConditionsMet(condRows, isPlayer, entry, spawnLow) {
			return
		}
		dx, dy := float64(x-primary.X), float64(y-primary.Y)
		reachRadius := float64(radius) + float64(reach)
		if dx*dx+dy*dy >= reachRadius*reachRadius || math.Abs(float64(z-primary.Z)) > float64(radius) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		seen[guid] = struct{}{}
		var deficit uint32
		if maxHealth > health {
			deficit = maxHealth - health
		}
		candidates = append(candidates, chainCandidate{guid: guid, x: x, y: y, z: z, deficit: deficit, reach: reach})
	}
	s.motionMu.Lock()
	motionMap := s.motionMapLocked(primary.Map, primary.InstanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.motionMu.Unlock()
	motionGUIDs := make(map[uint64]struct{}, len(motions))
	// SearchChainTargets -> SearchAreaTargets -> Spell::GetSearcherTypeMask
	// (Spell.cpp:1891/1881/1809): SPELL_ATTR3_ONLY_TARGET_PLAYERS /
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS drop the CREATURE container.
	playersOnly := spellSearchPlayersOnly(spell)
	for _, motion := range motions {
		if playersOnly {
			continue
		}
		motionGUIDs[motion.GUID] = struct{}{}
		motionOwner := motion.OwnerGUID
		if motionOwner == 0 {
			motionOwner = motion.CharmOwnerGUID
		}
		hostile := s.creatureFactionHostileTo(m.Faction, motion.Faction)
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.UnitFlags, motion.FlagsExtra, motion.Health, motion.MaxHealth, hostile, !hostile, false, false, motion.Entry, 0, motion.Faction, motionOwner, motion.CombatReach)
	}
	s.sessionsMu.RLock()
	for targetSession := range s.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		candidate := playerPos{Map: targetSession.player.Map, InstanceID: targetSession.player.InstanceID, X: targetSession.player.X, Y: targetSession.player.Y, Z: targetSession.player.Z, GUID: targetSession.playerGUID, Race: targetSession.player.Race, Class: targetSession.player.Class, Level: targetSession.player.Level, FactionTemplate: targetSession.effectiveFactionTemplate(targetSession.player.Race), Reputations: playerReputationMap(targetSession.player.Reputations), Sess: targetSession}
		accept(targetSession.playerGUID, targetSession.player.Map, targetSession.player.InstanceID, targetSession.player.X, targetSession.player.Y, targetSession.player.Z, targetSession.player.UnitFlags, 0, targetSession.player.Health, targetSession.player.MaxHealth, s.isAttackableFaction(m.Faction, candidate), s.isFriendlyFaction(m.Faction, candidate), true, sessionIsGameMaster(targetSession), 0, 0, 0, 0, targetSession.player.CombatReach)
	}
	s.sessionsMu.RUnlock()
	// The CREATURE container is dropped by the GetSearcherTypeMask attribute arm above.
	if !playersOnly && s.WorldStore != nil && s.WorldStore.DB != nil {
		rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, primary.Map, float64(primary.X-radius), float64(primary.X+radius), float64(primary.Y-radius), float64(primary.Y+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &mapID, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; !hasMotion {
						st := s.loadCreatureStats(ctx, uint32(entry))
						hostile := s.creatureFactionHostileTo(m.Faction, uint32(faction))
						accept(guid, uint32(mapID), primary.InstanceID, float32(x), float32(y), float32(z), uint32(unitFlags), uint32(flagsExtra), uint32(health), st.MaxHealth, hostile, !hostile, false, false, uint32(entry), uint32(low), uint32(faction), 0, st.CombatReach)
					}
				}
			}
		}
	}
	return candidates
}

// motionChainRefererScope is the Server-level twin of the creature branch of
// (*session).friendlyRefererScope: the party/raid scope for a creature
// referer. Owned/charmed creatures resolve through their owner's group
// snapshot; ownerless creatures fall back to the C++ unit-unit branch (same
// faction, Unit.cpp:12126). refererGUID is the looked-up referer (it can
// differ from m.GUID under the low/entry fallback form), matching the
// session method's comparisons.
func (s *Server) motionChainRefererScope(m *creatureMotion, refererGUID uint64) friendlyRefererScope {
	if s == nil || m == nil {
		return friendlyRefererScope{}
	}
	owner := m.OwnerGUID
	if owner == 0 {
		owner = m.CharmOwnerGUID
	}
	if owner != 0 {
		if ownerSession := s.findSessionByGUID(owner); ownerSession != nil && ownerSession.player != nil {
			_, party, raid, inGroup := groupMembershipSnapshot(s, ownerSession.groupID, owner)
			return playerGroupScope(refererGUID, party, raid, inGroup, 0)
		}
	}
	refFaction := m.Faction
	scope := friendlyRefererScope{}
	sameFaction := func(c friendlyCandidate) bool {
		return c.guid == refererGUID || (!c.isPlayer && c.faction == refFaction)
	}
	scope.inParty = sameFaction
	scope.inRaid = sameFaction
	return scope
}

// implicitUnitTargetLOSPassesForMotion is the Server-level twin of
// (*session).implicitUnitTargetLOSPasses for a creature caster: the default
// arm of Spell::CheckEffectTarget (Spell.cpp:7395-7450) with the line of
// sight measured from the motion, and the motion itself exempt like the
// session caster.
func (s *Server) implicitUnitTargetLOSPassesForMotion(spell wotlk.Spell, m *creatureMotion, targetGUID uint64, tx, ty, tz float32) bool {
	if s == nil || m == nil {
		return true
	}
	if spell.AttributesEx1&spellAttr2CanTargetNotInLOS != 0 {
		return true
	}
	if targetGUID == m.GUID {
		return true
	}
	return s.hasLineOfSightSpellCheck(m.Map, m.X, m.Y, m.Z, tx, ty, tz)
}
