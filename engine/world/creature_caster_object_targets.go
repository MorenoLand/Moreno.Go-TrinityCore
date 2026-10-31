package world

import (
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

const (
	implicitTargetUnitMaster     uint32 = 27  // TARGET_UNIT_MASTER (SharedDefines.h:1463)
	implicitTargetUnitSummoner   uint32 = 92  // TARGET_UNIT_SUMMONER (SharedDefines.h:1528)
	implicitTargetUnitPassenger0 uint32 = 96  // TARGET_UNIT_PASSENGER_0 (SharedDefines.h:1532)
	implicitTargetUnitPassenger7 uint32 = 103 // TARGET_UNIT_PASSENGER_7 (SharedDefines.h:1539)
)

// creatureCasterObjectUnit resolves one checkIfValid=true caster-object
// target (Spell::AddUnitTarget, Spell.cpp:2095) to a live unit on the
// caster's map/instance: the CheckTarget dead gate (SpellInfo.cpp:1712-1713,
// IsAllowingDeadTarget via spellAllowsDeadTarget) and the CheckEffectTarget
// default LOS arm (Spell.cpp:7395-7450, via
// implicitUnitTargetLOSPassesForMotion, target == caster exempt). The
// remaining CheckTarget gates (creature-type, GM, aura-state, aura-spell,
// prevent-resurrection) stay out, the same approximation the session twin
// (implicitCasterUnitTarget, spells.go) uses for its 5/94 legs. A gameobject
// GUID never resolves (ObjectAccessor::GetUnit, Spell.cpp:3404-3409
// precedent); a session on another map/instance never resolves either
// (each map holds its own object store, so a departed charmer/owner is
// null on the caster's map).
func (s *Server) creatureCasterObjectUnit(m *creatureMotion, spell wotlk.Spell, guid uint64) (uint64, bool) {
	if s == nil || m == nil || guid == 0 || uint16(guid>>48) == 0xF110 {
		return 0, false
	}
	var x, y, z float32
	alive := false
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil && sess.player.Map == m.Map && sess.player.InstanceID == m.InstanceID {
		x, y, z = sess.player.X, sess.player.Y, sess.player.Z
		alive = !sess.isDeadOrGhost()
	} else if tm := s.findCreatureMotion(m.Map, m.InstanceID, guid); tm != nil {
		x, y, z = tm.X, tm.Y, tm.Z
		alive = tm.Health > 0
	} else {
		return 0, false
	}
	if !alive && !spellAllowsDeadTarget(spell) {
		return 0, false
	}
	if !s.implicitUnitTargetLOSPassesForMotion(spell, m, guid, x, y, z) {
		return 0, false
	}
	return guid, true
}

// creatureCasterObjectTargets returns, per effect index, the unit GUIDs
// that Spell::SelectImplicitCasterObjectTargets (Spell.cpp:1500-1556) adds
// for a creature caster, shared by the boss-ai, inline and pet paths:
//   - TARGET_UNIT_CASTER (1): the caster itself, checkIfValid=false, so
//     no dead/LOS gate (the LOS arm exempts target == caster anyway).
//   - TARGET_UNIT_MASTER (27): GetCharmerOrOwner (Unit.h:1175) — the
//     charmer when one is stamped, else the owner (the
//     creatureCharmerOrOwnerGUID precedent, spells.go).
//   - TARGET_UNIT_SUMMONER (92): the summoner, but only for a TempSummon
//     (Unit::IsSummon, the motion IsTempSummon flag); a temp summon's
//     OwnerGUID is its summoner (summonTempCreature stamps it).
//   - TARGET_UNIT_VEHICLE (94): the vehicle base the caster rides
//     (motion.VehicleGUID, the Vehicle::AddPassenger seat stamp).
//   - TARGET_UNIT_PASSENGER_0..7 (96-103): the kit passenger on that
//     seat (Vehicle::GetPassenger), gated on a kit existing — the Go
//     twin of the caster IsVehicle() arm.
//
// TARGET_UNIT_PET (5) is deliberately absent: the CheckCast pet-presence
// arm (Spell.cpp:5410-5432) fizzles any TargetA == 5 creature cast on all
// three paths, and a TargetB == 5 resolves to null (GetGuardianPet has
// no Go model for creature casters), so it adds nothing on selection.
// Callers iterate the spell's effects in order for a deterministic flat
// order; the map itself carries no order.
func (s *Server) creatureCasterObjectTargets(m *creatureMotion, spell wotlk.Spell) map[int][]uint64 {
	if s == nil || m == nil {
		return nil
	}
	var kit *VehicleKit
	kitLoaded := false
	targets := map[int][]uint64{}
	add := func(i int, guid uint64) {
		if guid == 0 {
			return
		}
		for _, g := range targets[i] {
			if g == guid {
				return
			}
		}
		targets[i] = append(targets[i], guid)
	}
	for i, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, t := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch {
			case t == targetUnitCaster:
				add(i, m.GUID)
			case t == implicitTargetUnitMaster:
				master := m.CharmerGUID
				if master == 0 {
					master = m.OwnerGUID
				}
				if guid, ok := s.creatureCasterObjectUnit(m, spell, master); ok {
					add(i, guid)
				}
			case t == implicitTargetUnitSummoner:
				if m.IsTempSummon {
					if guid, ok := s.creatureCasterObjectUnit(m, spell, m.OwnerGUID); ok {
						add(i, guid)
					}
				}
			case t == implicitTargetUnitVehicle:
				if guid, ok := s.creatureCasterObjectUnit(m, spell, m.VehicleGUID); ok {
					add(i, guid)
				}
			case t >= implicitTargetUnitPassenger0 && t <= implicitTargetUnitPassenger7:
				if !kitLoaded {
					kit = s.getVehicleKit(m.Map, m.InstanceID, m.GUID)
					kitLoaded = true
				}
				if kit != nil {
					if guid, ok := s.creatureCasterObjectUnit(m, spell, kit.GetPassenger(int8(t-implicitTargetUnitPassenger0))); ok {
						add(i, guid)
					}
				}
			}
		}
	}
	return targets
}
