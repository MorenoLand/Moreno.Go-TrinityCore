package world

import (
	"context"
	"time"
)

// cancelPendingCreatureCast drops a motion's pending cast-time cast: the
// token moves so an armed twin fire callback no longer matches, the
// tick-driven inline pending clears, and the twin pending spell ID
// clears. Callers are every site that ends a cast early (evade,
// breakable-CC interrupt, Lua CombatStop with includingCast, the
// EffectInterruptCast arm) and every site where a new non-triggered cast
// irrevocably starts (the twin deferral point, the twin GO send, the
// inline schedule point) — C++ Unit::CastSpell replaces the current
// generic spell, and a fizzled CheckCast never reaches those points, so
// a failed cast never cancels a live one.
func (s *Server) cancelPendingCreatureCast(m *creatureMotion) {
	if m == nil {
		return
	}
	m.PendingCastToken++
	m.PendingCastSpellID = 0
	m.PendingInlineSpellID = 0
	m.PendingInlineTargetGUID = 0
	m.PendingInlineFireAt = time.Time{}
}

// scheduleDeferredTwinCast arms the fire timer for a twin cast-time cast
// (Spell::prepare, Spell.cpp:3174-3206: START at t0, _cast at
// t0+casttime). The callback re-looks-up the motion and drops the cast
// when the motion is gone or dead, or the token moved (a newer cast, an
// interrupt, an evade, or a combat stop). The fire call re-runs the whole
// twin body (C++ _cast re-runs CheckCast), with deferredFire suppressing
// the deferral decision, the START snapshot, and the cast-time
// CastingUntil stamp. The time.AfterFunc shape mirrors the aura-expiry
// and reflect-proc timers already in this package.
func (s *Server) scheduleDeferredTwinCast(m *creatureMotion, spellID uint32, targetGUID uint64, basePoints []int32, castMs uint32, token uint64) {
	if s == nil || m == nil || castMs == 0 {
		return
	}
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	bpCopy := append([]int32(nil), basePoints...)
	time.AfterFunc(time.Duration(castMs)*time.Millisecond, func() {
		motion := s.findCreatureMotion(mapID, instanceID, guid)
		if motion == nil || motion.Health == 0 || motion.PendingCastToken != token {
			return
		}
		motion.PendingCastSpellID = 0
		s.castCreatureSpellInternalWithBasePoints(context.Background(), motion, spellID, targetGUID, false, bpCopy, true)
	})
}
