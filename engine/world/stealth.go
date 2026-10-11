package world

import (
	"math"
)

// isStealthed checks if the session's player currently has an active stealth aura.
// Mirrors TrinityCore HasAuraType(SPELL_AURA_MOD_STEALTH) (SpellAuraDefines.h:96).
func (s *session) isStealthed() bool {
	if s.player == nil {
		return false
	}
	return s.hasAuraType(16) // SPELL_AURA_MOD_STEALTH
}

// getStealthValue returns the total stealth rating of the player.
// Mirrors TrinityCore m_stealth (SPELL_AURA_MOD_STEALTH + SPELL_AURA_MOD_STEALTH_LEVEL).
func (s *session) getStealthValue() int32 {
	if s.player == nil {
		return 0
	}
	return s.getTotalAuraModifier(16) + s.getTotalAuraModifier(154)
}

// getStealthDetectValue returns the total stealth detection bonus rating of the player.
// Mirrors TrinityCore m_stealthDetect (SPELL_AURA_MOD_STEALTH_DETECT).
func (s *session) getStealthDetectValue() int32 {
	if s.player == nil {
		return 0
	}
	return s.getTotalAuraModifier(17)
}

// stealthMiscValues returns the distinct stealth-type misc values of the
// session's live SPELL_AURA_MOD_STEALTH (16) effects, defaulting to
// STEALTH_GENERAL (0) when no misc is modeled. It serves the
// SPELL_AURA_DETECT_STEALTH (228) misc-matched auto-detect arm of
// WorldObject::CanDetectStealthOf (Object.cpp:1753), which checks the
// observer's 228 auras per stealth type the target is stealthed in.
func (s *session) stealthMiscValues() []int32 {
	seen := map[int32]bool{}
	var out []int32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		matched := false
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != 16 || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					if !seen[effect.MiscValue] {
						seen[effect.MiscValue] = true
						out = append(out, effect.MiscValue)
					}
					matched = true
				}
			}
		}
		if !matched && aura.AuraType == 16 && !seen[aura.MiscValue] {
			seen[aura.MiscValue] = true
			out = append(out, aura.MiscValue)
		}
	}
	if len(out) == 0 {
		return []int32{0}
	}
	return out
}

// canDetectStealthOf determines if the observer player can detect the target stealthed player.
// Mirrors TrinityCore WorldObject::CanDetectStealthOf (Object.cpp:1719-1790).
func (s *session) canDetectStealthOf(target *session) bool {
	if target == nil || target.player == nil {
		return true
	}
	if !target.isStealthed() {
		return true
	}
	if s.player == nil {
		return false
	}
	// GM or God mode sees all stealthed units
	if (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0 {
		return true
	}
	// Target is in the same party/group: party members can always see each other
	if s.groupID != 0 && s.groupID == target.groupID {
		return true
	}
	// Detect Stealth aura (SPELL_AURA_TRACK_STEALTHED = 151)
	if s.hasAuraType(151) {
		return true
	}
	// Detect Stealth aura (SPELL_AURA_DETECT_STEALTH = 228): an observer
	// carrying a 228 effect whose misc matches one of the target's stealth
	// types sees through it outright (Object.cpp:1753).
	for _, mv := range target.stealthMiscValues() {
		if s.hasAuraTypeWithMiscValue(spellAuraDetectStealth, mv) {
			return true
		}
	}

	dist := float32(distance3D(s.player.X, s.player.Y, s.player.Z, target.player.X, target.player.Y, target.player.Z))
	combatReach := float32(1.5)
	if s.player.CombatReach > 0 {
		combatReach = s.player.CombatReach
	}
	// Contact distance always detects stealth (both front and back)
	if dist < combatReach {
		return true
	}

	// Back check: units cannot detect stealth behind them
	if !hasInArc(s.player.Orientation, s.player.X, s.player.Y, target.player.X, target.player.Y, math.Pi) {
		return false
	}

	obsLevel := s.player.Level
	if obsLevel == 0 {
		obsLevel = 1
	}

	detectionValue := int32(30) + int32(obsLevel-1)*5
	detectionValue += s.getStealthDetectValue()
	detectionValue -= target.getStealthValue()

	visibilityRange := float32(detectionValue)*0.3 + combatReach
	if visibilityRange > 30.0 { // MAX_PLAYER_STEALTH_DETECT_RANGE
		visibilityRange = 30.0
	}
	if visibilityRange <= 0 {
		return false
	}

	return dist <= visibilityRange
}

func (s *session) canDetectInvisibilityOfPlayer(target *session) bool {
	if target == nil || target.player == nil {
		return true
	}
	if (s.player != nil && (s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0)) || s.security > 0 {
		return true
	}
	detect := s.loadedAuras()
	for _, targetAura := range target.loadedAuras() {
		if targetAura == nil || targetAura.AuraType != 18 {
			continue
		}
		found := false
		for _, detectAura := range detect {
			if detectAura != nil && detectAura.AuraType == 19 && detectAura.MiscValue == targetAura.MiscValue && detectAura.Amount >= targetAura.Amount {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// canCreatureDetectStealthOfPlayer determines if a creature can detect a stealthed player.
// Mirrors TrinityCore WorldObject::CanDetectStealthOf for Creature observers.
func canCreatureDetectStealthOfPlayer(motion *creatureMotion, targetSess *session, dist float32) bool {
	if targetSess == nil || targetSess.player == nil {
		return true
	}
	if !targetSess.isStealthed() {
		return true
	}

	// GuardAI::CanSeeAlways (GuardAI.cpp:45-51): a guard always sees a
	// player-controlled unit it is engaged by. Guards ride
	// CREATURE_FLAG_EXTRA_GUARD (0x80); engagement is the current combat
	// victim (motion.TargetGUID). Dormant while guards map to passive in
	// creatureReactState (they never acquire a TargetGUID via the scan).
	if motion != nil && motion.FlagsExtra&creatureFlagExtraGuard != 0 && motion.TargetGUID != 0 && targetSess.playerGUID == motion.TargetGUID {
		return true
	}

	combatReach := float32(1.5)
	if motion != nil && motion.CombatReach > 0 {
		combatReach = motion.CombatReach
	}
	if dist < combatReach {
		return true
	}

	if motion != nil && !hasInArc(motion.Orientation, motion.X, motion.Y, targetSess.player.X, targetSess.player.Y, math.Pi) {
		return false
	}

	// SPELL_AURA_DETECT_STEALTH (228; Object.cpp:1753): a creature carrying a
	// 228 effect whose misc matches the target's stealth type auto-detects it.
	if targetSess != nil && targetSess.server != nil && motion != nil {
		for _, mv := range targetSess.stealthMiscValues() {
			if targetSess.server.creatureAuraTypeSumByMisc(creatureAuraKeyForMotion(motion), spellAuraDetectStealth, mv) > 0 {
				return true
			}
		}
	}

	cLevel := uint32(1)
	if motion != nil && motion.Level > 0 {
		cLevel = motion.Level
	}

	detectionValue := int32(30) + int32(cLevel-1)*5
	detectionValue -= targetSess.getStealthValue()

	visibilityRange := float32(detectionValue)*0.3 + combatReach
	if visibilityRange <= 0 {
		return false
	}

	return dist <= visibilityRange
}

// creatureStealthAlertBand mirrors the checkAlert=true arm of
// WorldObject::CanDetectStealthOf (Object.cpp:1719-1790): when a stealthed
// player sits outside the normal detect range, the creature still "hears"
// them 8% further out plus 1.5 yards (Object.cpp:1779-1781) — the band that
// fires CreatureAI::TriggerAlert (CreatureAI.cpp) from
// CreatureUnitRelocationWorker (GridNotifiers.cpp:139). The behind-arc gate
// (Object.cpp:1739-1740) applies to the alert check as well. Returns
// (inBand, alertRange): inBand is true when the player is stealthed, not
// normally detectable, but within the alert band. The "no alert when the
// alert range reaches aggro distance" arm (Object.cpp:1782-1784) is applied
// by the caller, which owns the creature's attack distance; the DETECT_STEALTH
// aura always-detect arm (Object.cpp:1754-1755) and the stealth-detect value
// modifiers ride the same documented deltas as canCreatureDetectStealthOfPlayer.
func creatureStealthAlertBand(motion *creatureMotion, targetSess *session, dist float32) (bool, float32) {
	if targetSess == nil || targetSess.player == nil || !targetSess.isStealthed() {
		return false, 0
	}

	combatReach := float32(1.5)
	if motion != nil && motion.CombatReach > 0 {
		combatReach = motion.CombatReach
	}
	// Inside combat reach the player is always detected outright — no alert.
	if dist < combatReach {
		return false, 0
	}

	if motion != nil && !hasInArc(motion.Orientation, motion.X, motion.Y, targetSess.player.X, targetSess.player.Y, math.Pi) {
		return false, 0
	}

	cLevel := uint32(1)
	if motion != nil && motion.Level > 0 {
		cLevel = motion.Level
	}

	detectionValue := int32(30) + int32(cLevel-1)*5
	detectionValue -= targetSess.getStealthValue()

	visibilityRange := float32(detectionValue)*0.3 + combatReach
	if visibilityRange <= 0 {
		return false, 0
	}

	alertRange := visibilityRange*1.08 + 1.5
	if dist <= visibilityRange || dist > alertRange {
		return false, 0
	}
	return true, alertRange
}
