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
// A gameobject-base cast takes the rebuild leg instead of the
// re-lookup: the GO motion is synthetic and never stored
// (smartStateForGO builds a fresh one per fire), so findCreatureMotion
// can never resolve it and the fire would drop. A GO with no SmartAI
// rows rebuilds through syntheticGOMotion instead (the goober Use leg
// casts from row-less GOs). No token or health gate on that leg: C++
// GO casts are independent SpellEvents on the GO's own m_Events
// (GameObject::Update, GameObject.cpp:437; Spell::prepare,
// Spell.cpp:3072-3075), and prepare's in-progress check is
// ToUnit-gated, so a second GO cast never cancels the first; a
// despawned GO drops via the nil rebuild (its m_Events die with it
// in C++). Transports ride the same leg (smartStateForGO routes the
// 0x1FC0 GUID mark to smartStateForTransport).
func (s *Server) scheduleDeferredTwinCast(m *creatureMotion, spellID uint32, targetGUID uint64, basePointOverrides map[int]int32, explicitDest *[3]float32, castMs uint32, token uint64, radiusMod float32, maxTargets int32) {
	if s == nil || m == nil || castMs == 0 {
		return
	}
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	isGameObject := m.BaseIsGameObject
	var bpCopy map[int]int32
	if basePointOverrides != nil {
		bpCopy = make(map[int]int32, len(basePointOverrides))
		for i, bp := range basePointOverrides {
			bpCopy[i] = bp
		}
	}
	var destCopy *[3]float32
	if explicitDest != nil {
		d := *explicitDest
		destCopy = &d
	}
	time.AfterFunc(time.Duration(castMs)*time.Millisecond, func() {
		motion := s.findCreatureMotion(mapID, instanceID, guid)
		if motion == nil {
			if !isGameObject {
				return
			}
			motion, _ = s.smartStateForGO(context.Background(), mapID, instanceID, guid)
			if motion == nil {
				// A GO with no SmartAI rows has no script state to
				// rebuild from, but the GO itself is still a live
				// caster — rebuild the bare synthetic motion (the
				// goober Use leg casts from row-less GOs).
				motion = s.syntheticGOMotion(mapID, instanceID, guid)
			}
			if motion == nil {
				return
			}
		} else {
			if motion.Health == 0 || motion.PendingCastToken != token {
				return
			}
			motion.PendingCastSpellID = 0
		}
		s.castCreatureSpellInternalWithBasePoints(context.Background(), motion, spellID, targetGUID, false, bpCopy, destCopy, true, radiusMod, maxTargets)
	})
}
