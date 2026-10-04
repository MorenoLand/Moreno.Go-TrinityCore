package world

import "math"

const (
	// SpellCustomAttrReqTargetFacingCaster indicates caster must be in front arc of target (e.g. Gouge).
	// Mirrors TrinityCore SPELL_ATTR0_CU_REQ_TARGET_FACING_CASTER (SpellInfo.h:468).
	SpellCustomAttrReqTargetFacingCaster uint32 = 0x00010000

	// SpellCustomAttrReqCasterBehindTarget indicates caster must be behind target (e.g. Backstab, Ambush, Shred, Ravage, Garrote).
	// Mirrors TrinityCore SPELL_ATTR0_CU_REQ_CASTER_BEHIND_TARGET (SpellInfo.h:469).
	SpellCustomAttrReqCasterBehindTarget uint32 = 0x00020000

	// SpellCustomAttrAllowInflightTarget exempts a spell from the flying-target
	// rejection in SpellInfo::CheckTarget (SpellInfo.cpp:1745-1747).
	// Mirrors TrinityCore SPELL_ATTR0_CU_ALLOW_INFLIGHT_TARGET (SpellInfo.h:196),
	// read from AttributesCu via Server.getSpellCustomAttr.
	SpellCustomAttrAllowInflightTarget uint32 = 0x00040000

	// SpellCustomAttrShareDamage marks Meteor-like spells whose school damage is
	// divided by the number of hit targets (Spell::EffectSchoolDMG,
	// SpellEffects.cpp:334-348). Mirrors TrinityCore
	// SPELL_ATTR0_CU_SHARE_DAMAGE (SpellInfo.h:181), read from AttributesCu via
	// Server.getSpellCustomAttr.
	SpellCustomAttrShareDamage uint32 = 0x00000008

	// SpellFacingFlagInfront requires target to be within caster's 120° frontal cone (2*pi/3).
	// Mirrors TrinityCore SPELL_FACING_FLAG_INFRONT (DBCStructure.h:1409, Spell.dbc field 19).
	SpellFacingFlagInfront uint32 = 0x0001
)

// normalizeOrientation normalizes an orientation angle to [0, 2*pi).
// Mirrors TrinityCore Position::NormalizeOrientation (Position.h:44).
func normalizeOrientation(o float32) float32 {
	const twoPi = float32(2 * math.Pi)
	if o < 0 {
		rem := float32(math.Mod(float64(o), float64(twoPi)))
		if rem < 0 {
			rem += twoPi
		}
		return rem
	}
	if o >= twoPi {
		return float32(math.Mod(float64(o), float64(twoPi)))
	}
	return o
}

// hasInArc checks if the target position is within the observer's frontal arc.
// Mirrors TrinityCore Position::HasInArc (Position.cpp:120).
// An arc of math.Pi corresponds to a 180-degree frontal hemisphere (M_PI).
// An arc of 2*math.Pi/3 corresponds to a 120-degree frontal cone.
func hasInArc(observerOri, observerX, observerY, targetX, targetY float32, arc float64) bool {
	dx := float64(targetX - observerX)
	dy := float64(targetY - observerY)
	angleToTarget := math.Atan2(dy, dx)
	diff := angleToTarget - float64(observerOri)
	for diff > math.Pi {
		diff -= 2 * math.Pi
	}
	for diff < -math.Pi {
		diff += 2 * math.Pi
	}
	halfArc := arc / 2.0
	return diff >= -halfArc && diff <= halfArc
}

// isInFrontOf checks if candidate is within the observer's 180° frontal arc.
// Mirrors TrinityCore Position::IsInFront (Position.h:103).
func isInFrontOf(observerOri, observerX, observerY, candidateX, candidateY float32) bool {
	return hasInArc(observerOri, observerX, observerY, candidateX, candidateY, math.Pi)
}

// isBehindTarget checks if candidate is behind the target (outside target's 180° frontal arc).
// Mirrors TrinityCore Spell::CheckCast positional behind requirement.
func isBehindTarget(targetOri, targetX, targetY, candidateX, candidateY float32) bool {
	return !hasInArc(targetOri, targetX, targetY, candidateX, candidateY, math.Pi)
}

// isFacingTarget checks if caster is facing the target within the given frontal arc (default 120° / 2*pi/3).
// Mirrors TrinityCore Spell::CheckCast SPELL_FACING_FLAG_INFRONT requirement.
func isFacingTarget(casterOri, casterX, casterY, targetX, targetY float32, arc ...float64) bool {
	arcVal := 2.0 * math.Pi / 3.0
	if len(arc) > 0 && arc[0] > 0 {
		arcVal = arc[0]
	}
	return hasInArc(casterOri, casterX, casterY, targetX, targetY, arcVal)
}

// defaultSpellCustomAttrs holds base custom attributes for core abilities to guarantee correct
// positional requirements even in test environments where the world database is uninitialized.
var defaultSpellCustomAttrs = map[uint32]uint32{
	// Backstab (SPELL_ATTR0_CU_REQ_CASTER_BEHIND_TARGET = 0x20000)
	53: SpellCustomAttrReqCasterBehindTarget, 2589: SpellCustomAttrReqCasterBehindTarget,
	2590: SpellCustomAttrReqCasterBehindTarget, 2591: SpellCustomAttrReqCasterBehindTarget,
	8721: SpellCustomAttrReqCasterBehindTarget, 11279: SpellCustomAttrReqCasterBehindTarget,
	11280: SpellCustomAttrReqCasterBehindTarget, 11281: SpellCustomAttrReqCasterBehindTarget,
	25300: SpellCustomAttrReqCasterBehindTarget, 26863: SpellCustomAttrReqCasterBehindTarget,
	48656: SpellCustomAttrReqCasterBehindTarget, 48657: SpellCustomAttrReqCasterBehindTarget,

	// Garrote
	703: SpellCustomAttrReqCasterBehindTarget, 8631: SpellCustomAttrReqCasterBehindTarget,
	8632: SpellCustomAttrReqCasterBehindTarget, 8633: SpellCustomAttrReqCasterBehindTarget,
	11289: SpellCustomAttrReqCasterBehindTarget, 11290: SpellCustomAttrReqCasterBehindTarget,
	26839: SpellCustomAttrReqCasterBehindTarget, 26884: SpellCustomAttrReqCasterBehindTarget,
	48675: SpellCustomAttrReqCasterBehindTarget, 48676: SpellCustomAttrReqCasterBehindTarget,

	// Ambush
	8676: SpellCustomAttrReqCasterBehindTarget, 8724: SpellCustomAttrReqCasterBehindTarget,
	8725: SpellCustomAttrReqCasterBehindTarget, 11267: SpellCustomAttrReqCasterBehindTarget,
	11268: SpellCustomAttrReqCasterBehindTarget, 11269: SpellCustomAttrReqCasterBehindTarget,
	27441: SpellCustomAttrReqCasterBehindTarget, 48689: SpellCustomAttrReqCasterBehindTarget,
	48690: SpellCustomAttrReqCasterBehindTarget, 48691: SpellCustomAttrReqCasterBehindTarget,

	// Shred
	5221: SpellCustomAttrReqCasterBehindTarget, 6785: SpellCustomAttrReqCasterBehindTarget,
	6787: SpellCustomAttrReqCasterBehindTarget, 9829: SpellCustomAttrReqCasterBehindTarget,
	9830: SpellCustomAttrReqCasterBehindTarget, 27001: SpellCustomAttrReqCasterBehindTarget,
	27002: SpellCustomAttrReqCasterBehindTarget, 48571: SpellCustomAttrReqCasterBehindTarget,
	48572: SpellCustomAttrReqCasterBehindTarget,

	// Ravage
	6800: SpellCustomAttrReqCasterBehindTarget, 8992: SpellCustomAttrReqCasterBehindTarget,
	9866: SpellCustomAttrReqCasterBehindTarget, 9867: SpellCustomAttrReqCasterBehindTarget,
	27005: SpellCustomAttrReqCasterBehindTarget, 48578: SpellCustomAttrReqCasterBehindTarget,
	48579: SpellCustomAttrReqCasterBehindTarget,

	// Gouge (SPELL_ATTR0_CU_REQ_TARGET_FACING_CASTER = 0x10000)
	1776: SpellCustomAttrReqTargetFacingCaster, 1777: SpellCustomAttrReqTargetFacingCaster,
	8629: SpellCustomAttrReqTargetFacingCaster, 11285: SpellCustomAttrReqTargetFacingCaster,
	11286: SpellCustomAttrReqTargetFacingCaster, 38764: SpellCustomAttrReqTargetFacingCaster,
}

// getSpellCustomAttr returns the custom attributes for a given spell ID.
// Mirrors TrinityCore SpellInfo::HasCustomAttribute and spell_custom_attr table.
func (s *Server) getSpellCustomAttr(spellID uint32) uint32 {
	if s == nil {
		return defaultSpellCustomAttrs[spellID]
	}
	s.spellCustomAttrMu.RLock()
	if s.spellCustomAttrLoaded {
		attr := s.spellCustomAttr[spellID]
		s.spellCustomAttrMu.RUnlock()
		return attr
	}
	s.spellCustomAttrMu.RUnlock()

	s.spellCustomAttrMu.Lock()
	defer s.spellCustomAttrMu.Unlock()
	if s.spellCustomAttrLoaded {
		return s.spellCustomAttr[spellID]
	}
	s.spellCustomAttr = make(map[uint32]uint32)
	for k, v := range defaultSpellCustomAttrs {
		s.spellCustomAttr[k] = v
	}
	s.spellCustomAttrLoaded = true
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		rows, err := s.WorldStore.DB.Query(`SELECT entry, attributes FROM spell_custom_attr`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var entry, attr uint32
				if err := rows.Scan(&entry, &attr); err == nil && entry > 0 {
					s.spellCustomAttr[entry] = attr
				}
			}
		}
	}
	return s.spellCustomAttr[spellID]
}
