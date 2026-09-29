package world

import (
	"context"
	"math"
	"math/rand"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	implicitTargetCasterAreaParty uint32 = 20 // TARGET_UNIT_CASTER_AREA_PARTY (SharedDefines.h:1466)
	implicitTargetSrcAreaAlly     uint32 = 30 // TARGET_UNIT_SRC_AREA_ALLY
	implicitTargetDestAreaAlly    uint32 = 31 // TARGET_UNIT_DEST_AREA_ALLY
	implicitTargetSrcAreaParty    uint32 = 33 // TARGET_UNIT_SRC_AREA_PARTY
	implicitTargetDestAreaParty   uint32 = 34 // TARGET_UNIT_DEST_AREA_PARTY
	implicitTargetCasterAreaRaid  uint32 = 56 // TARGET_UNIT_CASTER_AREA_RAID
)

const (
	friendlyCheckAlly uint8 = iota
	friendlyCheckParty
	friendlyCheckRaid
)

// isFriendlyAreaTargetType reports the SpellImplicitTargetInfo values
// (SpellInfo.cpp:218-330) that Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) resolves with TARGET_CHECK_PARTY/ALLY/RAID.
func isFriendlyAreaTargetType(target uint32) bool {
	switch target {
	case implicitTargetCasterAreaParty,
		implicitTargetSrcAreaAlly, implicitTargetDestAreaAlly,
		implicitTargetSrcAreaParty, implicitTargetDestAreaParty,
		implicitTargetCasterAreaRaid:
		return true
	default:
		return false
	}
}

func isFriendlyAreaSpell(spell wotlk.Spell) bool {
	if isHarmfulSpell(spell) {
		return false
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isFriendlyAreaTargetType(eff.ImplicitTargetA) || isFriendlyAreaTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// friendlyAreaCheck maps an implicit target value to its TARGET_CHECK_*
// selection type (SpellImplicitTargetInfo::_data, SpellInfo.cpp:218).
func friendlyAreaCheck(targetType uint32) uint8 {
	switch targetType {
	case implicitTargetCasterAreaParty, implicitTargetSrcAreaParty, implicitTargetDestAreaParty:
		return friendlyCheckParty
	case implicitTargetCasterAreaRaid:
		return friendlyCheckRaid
	default: // 30, 31
		return friendlyCheckAlly
	}
}

// friendlyAreaDestCentered reports the TARGET_REFERENCE_TYPE_DEST values
// (31, 34): the area centers on the spell destination. SRC-reference values
// (30, 33) have no Go src-position tracking and default to the caster, the
// same heuristic Go applies to TARGET_SRC_CASTER (22).
func friendlyAreaDestCentered(targetType uint32) bool {
	return targetType == implicitTargetDestAreaAlly || targetType == implicitTargetDestAreaParty
}

// friendlyGroupSnapshot copies the caster's group membership: subgroup of the
// caster and the GUIDs sharing it (party), plus every member GUID (raid).
// Mirrors Player::IsInSameGroupWith (Player.cpp:2536: same group AND same
// subgroup) and Player::IsInSameRaidWith (Player.cpp:2543: same group).
func (s *session) friendlyGroupSnapshot() (subGroup uint8, party, raid map[uint64]struct{}, inGroup bool) {
	if s == nil {
		return 0, nil, nil, false
	}
	return groupMembershipSnapshot(s.server, s.groupID, s.playerGUID)
}

// spellFriendlyAreaTargets ports Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) for the friendly TARGET_CHECK_PARTY/ALLY/RAID values
// 20/30/31/33/34/56, resolved through SearchAreaTargets +
// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316).
//
// Ally test (TARGET_CHECK_ALLY): WorldObject::IsValidAssistTarget
// (Object.cpp:3087) — approximated as alive, not combat-disabled, and
// non-hostile: same alliance for players, !isHostileFaction for creatures.
// TARGET_CHECK_PARTY additionally requires Unit::IsInPartyWith
// (Unit.cpp:12126): self, same-subgroup group members, or creatures owned by
// them (charmer/owner resolution). TARGET_CHECK_RAID requires
// Unit::IsInRaidWith (Unit.cpp:12155): same group regardless of subgroup.
// Range uses the C++ cylinder test (2D dist + |dz| <= radius, Spell.cpp:8409)
// with no line-of-sight gate. The MaxTargets cap mirrors the enemy-area path
// (Spell.cpp:1293: MOD_MAX_AFFECTED_TARGETS aura modifier + RandomResize).
func (s *session) spellFriendlyAreaTargets(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || !isFriendlyAreaSpell(spell) || s.server.Data == nil {
		return nil
	}
	radius := float32(0)
	destinationCenter := false
	checks := make(map[uint8]struct{})
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if !isFriendlyAreaTargetType(targetType) {
				continue
			}
			if friendlyAreaDestCentered(targetType) {
				destinationCenter = true
			}
			checks[friendlyAreaCheck(targetType)] = struct{}{}
		}
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok && value > radius {
			radius = value
		}
	}
	if radius <= 0 || len(checks) == 0 {
		return nil
	}
	centerX, centerY, centerZ := s.player.X, s.player.Y, s.player.Z
	if destinationCenter && target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		centerX, centerY, centerZ = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if destinationCenter && target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		// Spell.cpp:1258 CheckDst: an explicit unit target becomes the
		// destination for DEST-reference area selection.
		if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			centerX, centerY, centerZ = destination.X, destination.Y, destination.Z
		}
	}
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	allyOf := func(faction uint32) bool { return !s.server.isHostileFaction(faction, caster) }
	_, partyMembers, raidMembers, inGroup := s.friendlyGroupSnapshot()
	inParty := func(guid uint64) bool {
		if guid == s.playerGUID {
			return true
		}
		if !inGroup {
			return false
		}
		_, ok := partyMembers[guid]
		return ok
	}
	inRaid := func(guid uint64) bool {
		if guid == s.playerGUID {
			return true
		}
		if !inGroup {
			return false
		}
		_, ok := raidMembers[guid]
		return ok
	}

	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z float32, unitFlags, flagsExtra, health uint32, ally, party, raid bool) {
		if guid == 0 || mapID != caster.Map || instanceID != caster.InstanceID || health == 0 || spellTargetUnitBlocked(spell, unitFlags, flagsExtra) {
			return
		}
		dx, dy := float64(x-centerX), float64(y-centerY)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(z-centerZ)) > float64(radius) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		_, wantAlly := checks[friendlyCheckAlly]
		_, wantParty := checks[friendlyCheckParty]
		_, wantRaid := checks[friendlyCheckRaid]
		if !((wantAlly && ally) || (wantParty && party) || (wantRaid && raid)) {
			return
		}
		seen[guid] = struct{}{}
		targets = append(targets, guid)
	}

	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(caster.Map, caster.InstanceID)
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
		// Unit.cpp:12126 charmer/owner resolution: a creature counts as party
		// or raid with the caster when its owner does.
		owner := motion.OwnerGUID
		if owner == 0 {
			owner = motion.CharmOwnerGUID
		}
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z,
			motion.UnitFlags, motion.FlagsExtra, motion.Health,
			allyOf(motion.Faction), inParty(owner), inRaid(owner))
	}

	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil || targetSession.player.Health == 0 {
			continue
		}
		p := targetSession.player
		accept(targetSession.playerGUID, p.Map, p.InstanceID, p.X, p.Y, p.Z,
			p.UnitFlags, 0, p.Health,
			targetSession.playerAlliance() == s.playerAlliance(), inParty(targetSession.playerGUID), inRaid(targetSession.playerGUID))
	}
	s.server.sessionsMu.RUnlock()

	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, caster.Map, float64(centerX-radius), float64(centerX+radius), float64(centerY-radius), float64(centerY+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &mapID, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; !hasMotion {
						// Static spawns have no owner tracking; party/raid
						// membership for them resolves through faction only.
						ally := allyOf(uint32(faction))
						accept(guid, uint32(mapID), caster.InstanceID, float32(x), float32(y), float32(z),
							uint32(unitFlags), uint32(flagsExtra), uint32(health), ally, false, false)
					}
				}
			}
		}
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 {
		// Spell.cpp:1293 — cap to MaxAffectedTargets plus
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
