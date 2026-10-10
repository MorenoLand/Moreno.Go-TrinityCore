package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetNearbyEnemy         uint32 = 2  // TARGET_UNIT_NEARBY_ENEMY (SharedDefines.h:1443)
	implicitTargetNearbyParty         uint32 = 3  // TARGET_UNIT_NEARBY_PARTY (SharedDefines.h:1443)
	implicitTargetNearbyAlly          uint32 = 4  // TARGET_UNIT_NEARBY_ALLY
	implicitTargetLastTargetAreaParty uint32 = 37 // TARGET_UNIT_LASTTARGET_AREA_PARTY
	implicitTargetNearbyRaid          uint32 = 58 // TARGET_UNIT_NEARBY_RAID
	implicitTargetConeAlly            uint32 = 59 // TARGET_UNIT_CONE_ALLY
	implicitTargetConeEntry           uint32 = 60 // TARGET_UNIT_CONE_ENTRY
	implicitTargetTargetAreaRaidClass uint32 = 61 // TARGET_UNIT_TARGET_AREA_RAID_CLASS
)

const (
	// friendlyCheckAlly/Party/Raid live in friendly_area_targets.go (0/1/2).
	friendlyCheckRaidClass uint8 = 3
	friendlyCheckEntry     uint8 = 4
)

// isFriendlyNearbyTargetType reports the SpellImplicitTargetInfo values
// (SpellInfo.cpp:218-330) that Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) resolves with TARGET_CHECK_PARTY/ALLY/RAID.
func isFriendlyNearbyTargetType(target uint32) bool {
	switch target {
	case implicitTargetNearbyParty, implicitTargetNearbyAlly, implicitTargetNearbyRaid:
		return true
	default:
		return false
	}
}

// isFriendlyConeTargetType reports the values Spell::SelectImplicitConeTargets
// (Spell.cpp:1176) resolves with TARGET_CHECK_ALLY/ENTRY.
func isFriendlyConeTargetType(target uint32) bool {
	switch target {
	case implicitTargetConeAlly, implicitTargetConeEntry:
		return true
	default:
		return false
	}
}

func isFriendlyNearbySpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isFriendlyNearbyTargetType(eff.ImplicitTargetA) || isFriendlyNearbyTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// isHostileNearbySpell reports the spells Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) resolves with TARGET_CHECK_ENEMY: any effect carrying
// TARGET_UNIT_NEARBY_ENEMY (2).
func isHostileNearbySpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetNearbyEnemy || eff.ImplicitTargetB == implicitTargetNearbyEnemy {
			return true
		}
	}
	return false
}

// spellHostileNearbyTarget ports Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) + Spell::SearchNearbyTarget (Spell.cpp:1869) for
// TARGET_UNIT_NEARBY_ENEMY (2): the single nearest hostile unit within
// GetMaxRange(false) — the spell's hostile max range — passing
// SpellInfo::CheckTarget and TARGET_CHECK_ENEMY (Spell.cpp:8323-8328:
// totem rejection + IsValidAttackTarget, proxied by isAttackableFaction
// for creatures and the alliance split for players, like the area-enemy
// path). The nearby check is a 3D distance with strict less-than,
// shrinking to the nearest match (WorldObjectSpellNearbyTargetCheck,
// Spell.cpp:8389-8399). The caster fails its own IsValidAttackTarget, so
// it is never selected. No match returns false and the caller fails the
// cast with SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993;
// Spell.cpp:1111). Documented deltas: the ImplicitTargetConditions arm
// (base WorldObjectSpellTargetCheck tail, Spell.cpp:8380-8387) is not
// wired — Go's condition evaluator is entry-oriented (entry_targets.go);
// corpse-type candidates have no Go model.
func (s *session) spellHostileNearbyTarget(ctx context.Context, spell wotlk.Spell) (uint64, bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, false
	}
	rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok || rangeEntry.MaxHostile <= 0 {
		return 0, false
	}
	maxRange := float64(rangeEntry.MaxHostile)
	player := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	s.friendlyScanCandidates(ctx, s.player.X, s.player.Y, float32(maxRange), spell, func(c friendlyCandidate) {
		if c.guid == s.playerGUID || c.mapID != player.Map || c.instanceID != player.InstanceID ||
			c.health == 0 || spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, false) ||
			s.server.isTotemGUID(c.guid) {
			return
		}
		if c.isPlayer {
			if c.alliance {
				return
			}
		} else if !s.server.isAttackableFaction(c.faction, player) {
			return
		}
		dist := distance3D(c.x, c.y, c.z, s.player.X, s.player.Y, s.player.Z)
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8394): strict
		// less-than, shrinking to the nearest match.
		if dist < bestDist {
			bestDist = dist
			bestGUID = c.guid
			bestX, bestY, bestZ = c.x, c.y, c.z
		}
	})
	if bestGUID == 0 {
		return 0, false
	}
	// Spell::CheckEffectTarget (Spell.cpp:1130: AddUnitTarget with
	// checkIfValid=true) drops a nearby unit target without line of sight
	// to the caster; an empty result fails the cast with
	// SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:794).
	if !s.implicitUnitTargetLOSPasses(spell, bestGUID, bestX, bestY, bestZ) {
		return 0, false
	}
	return bestGUID, true
}

func isFriendlyConeSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isFriendlyConeTargetType(eff.ImplicitTargetA) || isFriendlyConeTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

func isFriendlyLastTargetAreaSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetLastTargetAreaParty || eff.ImplicitTargetB == implicitTargetLastTargetAreaParty {
			return true
		}
	}
	return false
}

func isFriendlyTargetAreaRaidClassSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.ImplicitTargetA == implicitTargetTargetAreaRaidClass || eff.ImplicitTargetB == implicitTargetTargetAreaRaidClass {
			return true
		}
	}
	return false
}

// friendlyCandidate is one plausible unit for friendly implicit selection.
type friendlyCandidate struct {
	guid       uint64
	mapID      uint32
	instanceID uint32
	x, y, z    float32
	unitFlags  uint32
	flagsExtra uint32
	health     uint32
	faction    uint32 // creatures: template faction
	alliance   bool   // players: same alliance as the caster
	class      uint8  // players only; 0 = unknown (creatures)
	isPlayer   bool
	ownerGUID  uint64 // creatures: charmer/owner resolution (Unit.cpp:12126)
	entry      uint32 // creatures: template entry (0 for players)
}

// friendlyScanCandidates visits every plausible unit once: motion creatures,
// online players, then static creature rows inside the center/radius box.
func (s *session) friendlyScanCandidates(ctx context.Context, cx, cy, radius float32, spell wotlk.Spell, visit func(friendlyCandidate)) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	mapID, instanceID := s.player.Map, s.player.InstanceID
	// Spell::GetSearcherTypeMask (Spell.cpp:1836-1842):
	// SPELL_ATTR3_ONLY_TARGET_PLAYERS / SPELL_ATTR3_ONLY_TARGET_GHOSTS drop
	// the CREATURE container, so the motion and DB sweeps below contribute
	// nothing for such spells.
	playersOnly := spellSearchPlayersOnly(spell)
	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(mapID, instanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	motionGUIDs := make(map[uint64]struct{}, len(motions))
	for _, motion := range motions {
		if playersOnly {
			continue
		}
		motionGUIDs[motion.GUID] = struct{}{}
		owner := motion.OwnerGUID
		if owner == 0 {
			owner = motion.CharmOwnerGUID
		}
		visit(friendlyCandidate{guid: motion.GUID, mapID: motion.Map, instanceID: motion.InstanceID,
			x: motion.X, y: motion.Y, z: motion.Z, unitFlags: motion.UnitFlags, flagsExtra: motion.FlagsExtra,
			health: motion.Health, faction: motion.Faction, ownerGUID: owner, entry: motion.Entry})
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil {
			continue
		}
		p := targetSession.player
		visit(friendlyCandidate{guid: targetSession.playerGUID, mapID: p.Map, instanceID: p.InstanceID,
			x: p.X, y: p.Y, z: p.Z, unitFlags: p.UnitFlags, health: p.Health,
			alliance: targetSession.playerAlliance() == s.playerAlliance(), class: p.Class, isPlayer: true})
	}
	s.server.sessionsMu.RUnlock()
	if playersOnly {
		return
	}
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, mapID, float64(cx-radius), float64(cx+radius), float64(cy-radius), float64(cy+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, rowMap, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &rowMap, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; !hasMotion {
						visit(friendlyCandidate{guid: guid, mapID: uint32(rowMap), instanceID: instanceID,
							x: float32(x), y: float32(y), z: float32(z), unitFlags: uint32(unitFlags),
							flagsExtra: uint32(flagsExtra), health: uint32(health), faction: uint32(faction), entry: uint32(entry)})
					}
				}
			}
		}
	}
}

// friendlyAssistOK mirrors the ally terms of WorldObject::IsValidAssistTarget
// (Object.cpp:3087) used by TARGET_CHECK_ALLY/PARTY/RAID/RAID_CLASS: alive,
// not combat-disabled, non-hostile. GM exclusion, visibility and vehicle terms
// have no Go infra (same noted gap as the area path), and totem units are not
// tracked in Go (TARGET_CHECK_* all reject totems in C++).
func (s *session) friendlyAssistOK(spell wotlk.Spell, c friendlyCandidate, caster playerPos, allyOf func(faction uint32) bool) bool {
	if c.mapID != caster.Map || c.instanceID != caster.InstanceID || c.health == 0 || spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, true) {
		return false
	}
	if c.isPlayer {
		return c.alliance
	}
	return allyOf(c.faction)
}

// friendlyRefererScope carries the party/raid membership testers and class of
// the referer unit for LAST/TARGET-reference selection, mirroring
// Unit::IsInPartyWith (Unit.cpp:12126) and Unit::IsInRaidWith (Unit.cpp:12155).
type friendlyRefererScope struct {
	inParty  func(c friendlyCandidate) bool
	inRaid   func(c friendlyCandidate) bool
	class    uint8
	isPlayer bool
}

func groupMembershipSnapshot(server *Server, groupID uint64, guid uint64) (subGroup uint8, party, raid map[uint64]struct{}, inGroup bool) {
	if server == nil || groupID == 0 {
		return 0, nil, nil, false
	}
	server.groupsMu.RLock()
	g := server.groups[groupID]
	if g == nil {
		server.groupsMu.RUnlock()
		return 0, nil, nil, false
	}
	members := append([]groupMember(nil), g.Members...)
	server.groupsMu.RUnlock()
	for _, m := range members {
		if m.GUID == guid {
			subGroup = m.SubGroup
			inGroup = true
			break
		}
	}
	if !inGroup {
		return 0, nil, nil, false
	}
	party = make(map[uint64]struct{}, len(members))
	raid = make(map[uint64]struct{}, len(members))
	for _, m := range members {
		raid[m.GUID] = struct{}{}
		if m.SubGroup == subGroup {
			party[m.GUID] = struct{}{}
		}
	}
	return subGroup, party, raid, true
}

// playerGroupScope builds party/raid testers from a player's group snapshot:
// self, same-subgroup members (party), all group members (raid), plus
// creatures owned by them (charmer/owner resolution, Unit.cpp:12126).
func playerGroupScope(refererGUID uint64, partyMembers, raidMembers map[uint64]struct{}, inGroup bool, class uint8) friendlyRefererScope {
	scope := friendlyRefererScope{class: class, isPlayer: true}
	memberOf := func(set map[uint64]struct{}, c friendlyCandidate) bool {
		if c.guid == refererGUID {
			return true
		}
		if !inGroup {
			return false
		}
		if c.isPlayer {
			_, ok := set[c.guid]
			return ok
		}
		if c.ownerGUID == 0 {
			return false
		}
		_, ok := set[c.ownerGUID]
		return ok
	}
	scope.inParty = func(c friendlyCandidate) bool { return memberOf(partyMembers, c) }
	scope.inRaid = func(c friendlyCandidate) bool { return memberOf(raidMembers, c) }
	return scope
}

// friendlyRefererScope resolves the party/raid scope for a referer unit. Player
// referers use their own group snapshot (61's raid test is relative to the
// target, not the caster); creature referers resolve through their owner, and
// ownerless creatures fall back to the C++ unit-unit branch (same faction,
// Unit.cpp:12126).
func (s *session) friendlyRefererScope(refererGUID uint64) friendlyRefererScope {
	if s == nil || s.server == nil || s.player == nil {
		return friendlyRefererScope{}
	}
	if refererGUID == s.playerGUID {
		_, party, raid, inGroup := groupMembershipSnapshot(s.server, s.groupID, s.playerGUID)
		return playerGroupScope(refererGUID, party, raid, inGroup, s.player.Class)
	}
	if refererSession := s.server.findSessionByGUID(refererGUID); refererSession != nil && refererSession.player != nil {
		_, party, raid, inGroup := groupMembershipSnapshot(s.server, refererSession.groupID, refererGUID)
		return playerGroupScope(refererGUID, party, raid, inGroup, refererSession.player.Class)
	}
	if motion := s.findCreatureMotion(refererGUID); motion != nil {
		owner := motion.OwnerGUID
		if owner == 0 {
			owner = motion.CharmOwnerGUID
		}
		if owner != 0 {
			if ownerSession := s.server.findSessionByGUID(owner); ownerSession != nil && ownerSession.player != nil {
				_, party, raid, inGroup := groupMembershipSnapshot(s.server, ownerSession.groupID, owner)
				return playerGroupScope(refererGUID, party, raid, inGroup, 0)
			}
		}
		refFaction := motion.Faction
		scope := friendlyRefererScope{}
		sameFaction := func(c friendlyCandidate) bool {
			return c.guid == refererGUID || (!c.isPlayer && c.faction == refFaction)
		}
		scope.inParty = sameFaction
		scope.inRaid = sameFaction
		return scope
	}
	return friendlyRefererScope{}
}

// friendlyReferer resolves the center unit for LAST/TARGET-reference area
// selection (Spell.cpp:1227): its position and party/raid scope.
type friendlyReferer struct {
	x, y, z float32
	scope   friendlyRefererScope
	found   bool
}

func (s *session) friendlyReferer(ctx context.Context, guid uint64) friendlyReferer {
	if guid == 0 {
		return friendlyReferer{}
	}
	tgt, ok := s.getCombatTarget(ctx, guid)
	if !ok {
		return friendlyReferer{}
	}
	return friendlyReferer{x: tgt.X, y: tgt.Y, z: tgt.Z, scope: s.friendlyRefererScope(guid), found: true}
}

// spellFriendlyNearbyTarget ports Spell::SelectImplicitNearbyTargets
// (Spell.cpp:1036) + SearchNearbyTarget (Spell.cpp:1869) for 3/4/58: the
// single nearest unit within GetMaxRange(true) (Spell.dbc range, friendly
// side) passing the check. The caster itself is a candidate (WorldObject::
// IsValidAssistTarget, Object.cpp:3087: "can assist to self", and
// Unit::IsInPartyWith/IsInRaidWith return true for self), so it wins at
// distance 0 whenever it passes. No match -> SPELL_FAILED_BAD_IMPLICIT_TARGETS
// (SharedDefines.h:993) and the cast fails (Spell.cpp:1111).
func (s *session) spellFriendlyNearbyTarget(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) (uint64, bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0, false
	}
	rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok || rangeEntry.MaxFriendly <= 0 {
		return 0, false
	}
	maxRange := float64(rangeEntry.MaxFriendly)
	checks := make(map[uint8]struct{})
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch targetType {
			case implicitTargetNearbyParty:
				checks[friendlyCheckParty] = struct{}{}
			case implicitTargetNearbyAlly:
				checks[friendlyCheckAlly] = struct{}{}
			case implicitTargetNearbyRaid:
				checks[friendlyCheckRaid] = struct{}{}
			}
		}
	}
	if len(checks) == 0 {
		return 0, false
	}
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	allyOf := func(faction uint32) bool { return !s.server.isHostileFaction(faction, caster) }
	scope := s.friendlyRefererScope(s.playerGUID)
	bestGUID := uint64(0)
	bestDist := maxRange
	var bestX, bestY, bestZ float32
	s.friendlyScanCandidates(ctx, s.player.X, s.player.Y, float32(maxRange), spell, func(c friendlyCandidate) {
		if !s.friendlyAssistOK(spell, c, caster, allyOf) {
			return
		}
		_, wantAlly := checks[friendlyCheckAlly]
		_, wantParty := checks[friendlyCheckParty]
		_, wantRaid := checks[friendlyCheckRaid]
		if !((wantAlly) || (wantParty && scope.inParty(c)) || (wantRaid && scope.inRaid(c))) {
			return
		}
		dist := distance3D(c.x, c.y, c.z, s.player.X, s.player.Y, s.player.Z)
		// WorldObjectSpellNearbyTargetCheck (Spell.cpp:8394): strict less-than,
		// shrinking to the nearest match.
		if dist < bestDist {
			bestDist = dist
			bestGUID = c.guid
			bestX, bestY, bestZ = c.x, c.y, c.z
		}
	})
	if bestGUID == 0 {
		return 0, false
	}
	// Spell::CheckEffectTarget (Spell.cpp:1130: AddUnitTarget with
	// checkIfValid=true) drops a nearby unit target without line of sight
	// to the caster; an empty result fails the cast with
	// SPELL_FAILED_BAD_IMPLICIT_TARGETS (Spell.cpp:794).
	if !s.implicitUnitTargetLOSPasses(spell, bestGUID, bestX, bestY, bestZ) {
		return 0, false
	}
	return bestGUID, true
}

// spellFriendlyConeTargets ports Spell::SelectImplicitConeTargets
// (Spell.cpp:1176) for 59/60: coneAngle = M_PI/2 (WorldObjectSpellConeTargetCheck,
// Spell.cpp:8432), the C++ cylinder range test (2D + |dz|, Spell.cpp:8409),
// and the caster always in its own cone (Position::HasInArc, Position.cpp:120:
// "always have self in arc"). 59 uses TARGET_CHECK_ALLY; 60 uses
// TARGET_CHECK_ENTRY, which in C++ filters through ImplicitTargetConditions —
// Go has no condition infra, so 60 selects any valid unit in the cone.
func (s *session) spellFriendlyConeTargets(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	radius := float32(0)
	checks := make(map[uint8]struct{})
	// Spell.cpp:1194-1196: zero-radius fallback to the spell's max range.
	var maxRange float32
	if rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex); err == nil && ok {
		maxRange = float32(spellDBCMaxRange(spell, rangeEntry))
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch targetType {
			case implicitTargetConeAlly:
				checks[friendlyCheckAlly] = struct{}{}
			case implicitTargetConeEntry:
				checks[friendlyCheckEntry] = struct{}{}
			default:
				continue
			}
		}
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok && value > 0 {
			if value > radius {
				radius = value
			}
		} else if maxRange > radius {
			// Spell.cpp:1194-1196: zero-radius fallback.
			radius = maxRange
		}
	}
	if radius <= 0 || len(checks) == 0 {
		return nil
	}
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	allyOf := func(faction uint32) bool { return !s.server.isHostileFaction(faction, caster) }
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	s.friendlyScanCandidates(ctx, s.player.X, s.player.Y, radius, spell, func(c friendlyCandidate) {
		if c.guid == 0 || c.mapID != caster.Map || c.instanceID != caster.InstanceID || c.health == 0 || spellTargetUnitBlocked(spell, c.unitFlags, c.flagsExtra, true) {
			return
		}
		dx, dy := float64(c.x-s.player.X), float64(c.y-s.player.Y)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(c.z-s.player.Z)) > float64(radius) {
			return
		}
		if c.guid != s.playerGUID && !hasInArc(s.player.Orientation, s.player.X, s.player.Y, c.x, c.y, math.Pi/2) {
			return
		}
		_, wantAlly := checks[friendlyCheckAlly]
		_, wantEntry := checks[friendlyCheckEntry]
		pass := wantEntry
		if !pass && wantAlly {
			// TARGET_CHECK_ALLY rejects totems (Spell.cpp:8333); TARGET_CHECK_ENTRY
			// (target 60) carries no such exclusion, so the gate sits on the ally path only.
			pass = s.friendlyAssistOK(spell, c, caster, allyOf) && !s.server.isTotemGUID(c.guid)
		}
		if !pass {
			return
		}
		if _, ok := seen[c.guid]; ok {
			return
		}
		seen[c.guid] = struct{}{}
		targets = append(targets, c.guid)
	})
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(targets)) > maxTargets {
		// Spell.cpp:1196 — cap to MaxAffectedTargets plus
		// SPELL_AURA_MOD_MAX_AFFECTED_TARGETS aura modifiers, then
		// Trinity::Containers::RandomResize.
		maxTargets += uint32(s.totalAuraModifierByAffectMask(spellAuraModMaxAffectedTargets, spell))
		if uint32(len(targets)) > maxTargets {
			rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
			targets = targets[:maxTargets]
		}
	}
	return targets
}

// spellFriendlyRefCenteredAreaTargets ports Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) for the non-caster reference types: 37
// TARGET_UNIT_LASTTARGET_AREA_PARTY (TARGET_REFERENCE_TYPE_LAST, centered on
// the last target added for the effect — Go's single shared hit list has no
// per-effect history, so the incoming seed's last entry, i.e. the explicit
// unit target, is the referer) and 61 TARGET_UNIT_TARGET_AREA_RAID_CLASS
// (TARGET_REFERENCE_TYPE_TARGET, centered on the explicit unit target;
// Prayer of Mending). 61 additionally requires the candidate's class to
// match the referer's (fallthrough to TARGET_CHECK_RAID, Spell.cpp:8349);
// creature classes are unknown in Go (0), noted below.
func (s *session) spellFriendlyRefCenteredAreaTargets(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData, seed []uint64) []uint64 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	type centeredSelection struct {
		check       uint8
		refererGUID uint64
	}
	selections := make([]centeredSelection, 0, 2)
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch targetType {
			case implicitTargetLastTargetAreaParty:
				refererGUID := uint64(0)
				if len(seed) > 0 {
					refererGUID = seed[len(seed)-1]
				}
				selections = append(selections, centeredSelection{check: friendlyCheckParty, refererGUID: refererGUID})
			case implicitTargetTargetAreaRaidClass:
				if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
					selections = append(selections, centeredSelection{check: friendlyCheckRaidClass, refererGUID: target.UnitGUID})
				}
			}
		}
	}
	if len(selections) == 0 {
		return nil
	}
	radius := float32(0)
	// Spell.cpp:1249-1251: zero-radius fallback to the spell's max range.
	var maxRange float32
	if rangeEntry, ok, err := s.server.Data.SpellRange(spell.RangeIndex); err == nil && ok {
		maxRange = float32(spellDBCMaxRange(spell, rangeEntry))
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok && value > 0 {
			if value > radius {
				radius = value
			}
		} else if maxRange > radius {
			// Spell.cpp:1249-1251: zero-radius fallback.
			radius = maxRange
		}
	}
	if radius <= 0 {
		return nil
	}
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	allyOf := func(faction uint32) bool { return !s.server.isHostileFaction(faction, caster) }
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	for _, sel := range selections {
		referer := s.friendlyReferer(ctx, sel.refererGUID)
		if !referer.found {
			continue
		}
		s.friendlyScanCandidates(ctx, referer.x, referer.y, radius, spell, func(c friendlyCandidate) {
			// TARGET_CHECK_PARTY and TARGET_CHECK_RAID_CLASS both reject totems
			// (Spell.cpp:8342/8354).
			if c.guid == 0 || c.mapID != caster.Map || c.instanceID != caster.InstanceID || s.server.isTotemGUID(c.guid) {
				return
			}
			if !s.friendlyAssistOK(spell, c, caster, allyOf) {
				return
			}
			switch sel.check {
			case friendlyCheckParty:
				if !referer.scope.inParty(c) {
					return
				}
			case friendlyCheckRaidClass:
				// Spell.cpp:8349: refUnit->GetClass() != unitTarget->GetClass()
				// rejects; creature classes are unknown in Go (0), so a
				// creature only matches a classless referer.
				if c.class != referer.scope.class {
					return
				}
				if !referer.scope.inRaid(c) {
					return
				}
			}
			dx, dy := float64(c.x-referer.x), float64(c.y-referer.y)
			if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(c.z-referer.z)) > float64(radius) {
				return
			}
			if _, ok := seen[c.guid]; ok {
				return
			}
			seen[c.guid] = struct{}{}
			targets = append(targets, c.guid)
		})
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 {
		maxTargets += uint32(s.totalAuraModifierByAffectMask(spellAuraModMaxAffectedTargets, spell))
		if uint32(len(targets)) > maxTargets {
			rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
			targets = targets[:maxTargets]
		}
	}
	return targets
}
