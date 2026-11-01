package world

import (
	"math"
	"sort"
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

// hasStalkedAuraFrom reports whether this session carries a live
// SPELL_AURA_MOD_STALKED (68) effect cast by the given seer GUID.
func (s *session) hasStalkedAuraFrom(seerGUID uint64) bool {
	for _, aura := range s.loadedAuras() {
		if aura != nil && !aura.Stopped && aura.AuraType == spellAuraModStalked && aura.CasterGUID == seerGUID {
			return true
		}
	}
	return false
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
	// SPELL_AURA_MOD_STALKED (68) cast by the observer: Unit::IsAlwaysDetectableFor
	// (Unit.cpp:8640-8647) is checked by CanDetect before both invisibility and
	// stealth arms (Object.cpp:1676-1682), so a marked target is always seen.
	if target.hasStalkedAuraFrom(s.playerGUID) {
		return true
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

	// Detect Stealth aura (SPELL_AURA_DETECT_STEALTH = 228): an observer
	// carrying a 228 effect whose misc matches one of the target's stealth
	// types sees through it outright (Object.cpp:1753). This arm sits after
	// the back-arc gate above: C++ returns false on the arc check before
	// reaching the per-type 228 arm, so detect-stealth never sees behind.
	// (SPELL_AURA_TRACK_STEALTHED = 151 has no arm here: in C++ it only sets
	// the client minimap PLAYER_FIELD_BYTE_TRACK_STEALTHED flag
	// (SpellAuraEffects.cpp:2496) and the SPELL_SPECIFIC_TRACKER class
	// (SpellInfo.cpp:2230); it grants no in-world detection.)
	for _, mv := range target.stealthMiscValues() {
		if s.hasAuraTypeWithMiscValue(spellAuraDetectStealth, mv) {
			return true
		}
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
	// MOD_STALKED arm of Unit::IsAlwaysDetectableFor, checked by CanDetect
	// before the invisibility arms (Object.cpp:1676-1682).
	if target.hasStalkedAuraFrom(s.playerGUID) {
		return true
	}
	// WorldObject::CanDetectInvisibilityOf (Object.cpp:1688-1717) works on
	// per-type summed values, not single auras: the target's invisibility
	// value for a type is the sum of its MOD_INVISIBILITY (18) effects of
	// that misc, and detection needs a MOD_INVISIBILITY_DETECT (19) sum at
	// least as large for every type the target carries. A single-aura
	// comparison misses detection split across two auras of one type.
	invisSum := map[int32]int32{}
	targetDetectHas := map[int32]bool{}
	for _, targetAura := range target.loadedAuras() {
		if targetAura == nil || targetAura.Stopped {
			continue
		}
		if targetAura.AuraType == 18 {
			invisSum[targetAura.MiscValue] += int32(targetAura.Amount)
		}
		if targetAura.AuraType == 19 {
			targetDetectHas[targetAura.MiscValue] = true
		}
	}
	if len(invisSum) == 0 {
		return true
	}
	detectSum := map[int32]int32{}
	detectHas := map[int32]bool{}
	for _, detectAura := range s.loadedAuras() {
		if detectAura == nil || detectAura.Stopped || detectAura.AuraType != 19 {
			continue
		}
		detectSum[detectAura.MiscValue] += int32(detectAura.Amount)
		detectHas[detectAura.MiscValue] = true
	}
	for misc, value := range invisSum {
		if !detectHas[misc] || detectSum[misc] < value {
			return false
		}
	}
	// Reciprocal flags arm (Object.cpp:1700-1703, the spell-66 rule): an
	// invisible seer cannot detect a unit that cannot detect the seer's own
	// invisibility types, so the seer's invisibility misc set must be a
	// subset of the target's detect misc set. Flags only, no value compare.
	seerInvisHas := map[int32]bool{}
	for _, seerAura := range s.loadedAuras() {
		if seerAura == nil || seerAura.Stopped || seerAura.AuraType != 18 {
			continue
		}
		seerInvisHas[seerAura.MiscValue] = true
	}
	for misc := range seerInvisHas {
		if !targetDetectHas[misc] {
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

	// MOD_STALKED arm of Unit::IsAlwaysDetectableFor, as in the player path:
	// a target marked by this creature is always detectable.
	if motion != nil && targetSess.hasStalkedAuraFrom(motion.GUID) {
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

// wildCreatureCanDetectTarget is the wild-creature-seer twin of
// WorldObject::CanDetect (Object.cpp:1664-1686) for the spell gate: the
// seer is the motion itself (Object.cpp:1664-1666 — no controller routing
// for a wild caster), so its stealth/invisibility detect sums come from
// the motion's live creature auras. Player targets judge through the
// session's aura folds, creature targets through the motion's
// creature-aura sums. checkAlert is false on this path (SpellInfo::
// CheckTarget calls CanSeeOrDetect with the defaults, SpellInfo.cpp:1639),
// so the alert band and the aggro-distance no-alert arm never fire here;
// the GuardAI CanSeeAlways arm is an aggro-scan leg, not a CanDetect leg,
// and stays out, which is why this does not reuse
// canCreatureDetectStealthOfPlayer. Unresolvable targets and self targets
// fail open (CanSeeOrDetect returns true for this == obj, Object.cpp:1575).
// Sight-range, ghost-team/group, vehicle-accessory and despawn legs stay
// out, as on the player path.
func (s *Server) wildCreatureCanDetectTarget(m *creatureMotion, targetGUID uint64) bool {
	if s == nil || m == nil || targetGUID == 0 || targetGUID == m.GUID {
		return true
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return s.wildCreatureCanDetectPlayer(m, ts)
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		return s.wildCreatureCanDetectCreature(m, tm)
	}
	return true
}

// wildCreatureCanDetectPlayer runs CanDetect for a wild motion seer
// against a player target, in C++ order: IsAlwaysDetectableFor (stalked
// by the seer, Unit.cpp:8640) first, then invisibility
// (Object.cpp:1688-1717), then stealth (Object.cpp:1719-1790).
func (s *Server) wildCreatureCanDetectPlayer(m *creatureMotion, ts *session) bool {
	if m == nil || ts == nil || ts.player == nil {
		return true
	}
	if ts.hasStalkedAuraFrom(m.GUID) {
		return true
	}
	if !s.wildCreatureCanDetectInvisibilityOfPlayer(m, ts) {
		return false
	}
	return s.wildCreatureCanDetectStealthOfPlayer(m, ts)
}

// wildCreatureCanDetectInvisibilityOfPlayer mirrors
// WorldObject::CanDetectInvisibilityOf (Object.cpp:1688-1717) with a
// creature seer: every invisibility type the target carries must be
// covered by the seer's MOD_INVISIBILITY_DETECT (19) sum for that type
// (presence gate plus value compare, so a zero-value invisibility still
// needs a detect flag), and the reciprocal flags arm (Object.cpp:1704-1706)
// requires every invisibility type the seer carries to sit in the
// target's detect set, or the seer cannot see the target at all.
func (s *Server) wildCreatureCanDetectInvisibilityOfPlayer(m *creatureMotion, ts *session) bool {
	if m == nil || ts == nil || ts.player == nil {
		return true
	}
	targetInvis := map[int32]int32{}
	targetDetectHas := map[int32]bool{}
	for _, aura := range ts.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		if aura.AuraType == spellAuraModInvisibility {
			targetInvis[aura.MiscValue] += int32(aura.Amount)
		}
		if aura.AuraType == spellAuraInvisibilityDetect {
			targetDetectHas[aura.MiscValue] = true
		}
	}
	seerDetect := s.totemCreatureAuraMiscSums(m, spellAuraInvisibilityDetect)
	for misc, value := range targetInvis {
		detect, ok := seerDetect[misc]
		if !ok || detect < value {
			return false
		}
	}
	seerInvis := s.totemCreatureAuraMiscSums(m, spellAuraModInvisibility)
	for misc := range seerInvis {
		if !targetDetectHas[misc] {
			return false
		}
	}
	return true
}

// wildCreatureCanDetectStealthOfPlayer mirrors
// WorldObject::CanDetectStealthOf (Object.cpp:1719-1790) with a creature
// seer: contact distance and the front-arc gate first, then per stealth
// type in ascending order the DETECT_STEALTH (228) misc-matched
// auto-detect arm and the level/detect-sum range arm. The seer's
// MOD_STEALTH_DETECT (17) sum is per type (Object.cpp:1764), and there is
// no MAX_PLAYER_STEALTH_DETECT_RANGE cap: C++ caps the range only when
// the seer is a player (Object.cpp:1776). The target's stealth value
// follows the session-path convention (global 16+154 sum).
func (s *Server) wildCreatureCanDetectStealthOfPlayer(m *creatureMotion, ts *session) bool {
	if m == nil || ts == nil || ts.player == nil || !ts.isStealthed() {
		return true
	}
	combatReach := float32(1.5)
	if m.CombatReach > 0 {
		combatReach = m.CombatReach
	}
	dist := float32(distance3D(m.X, m.Y, m.Z, ts.player.X, ts.player.Y, ts.player.Z))
	if dist < combatReach {
		return true
	}
	if !hasInArc(m.Orientation, m.X, m.Y, ts.player.X, ts.player.Y, math.Pi) {
		return false
	}
	detectStealth := s.totemCreatureAuraMiscSums(m, spellAuraDetectStealth)
	detectValue := s.totemCreatureAuraMiscSums(m, spellAuraStealthDetect)
	seerLevel := uint32(1)
	if m.Level > 0 {
		seerLevel = m.Level
	}
	targetValue := ts.getStealthValue()
	miscs := append([]int32(nil), ts.stealthMiscValues()...)
	sort.Slice(miscs, func(i, j int) bool { return miscs[i] < miscs[j] })
	for _, mv := range miscs {
		if _, ok := detectStealth[mv]; ok {
			return true
		}
		detectionValue := int32(30) + int32(seerLevel-1)*5 + detectValue[mv] - targetValue
		visibilityRange := float32(detectionValue)*0.3 + combatReach
		if visibilityRange <= 0 || dist > visibilityRange {
			return false
		}
	}
	return true
}

// wildCreatureCanDetectCreature is the creature-target half of CanDetect
// for a wild motion seer, in the same C++ order as the player half:
// stalked, invisibility, stealth. The target's aura state comes from its
// live creature auras, so a stealthed or invisible creature target now
// judges instead of passing unconditionally.
func (s *Server) wildCreatureCanDetectCreature(m, tm *creatureMotion) bool {
	if m == nil || tm == nil {
		return true
	}
	if s.creatureHasStalkedAuraFrom(tm, m.GUID) {
		return true
	}
	if !s.wildCreatureCanDetectInvisibilityOfCreature(m, tm) {
		return false
	}
	return s.wildCreatureCanDetectStealthOfCreature(m, tm)
}

// creatureHasStalkedAuraFrom mirrors the Unit::IsAlwaysDetectableFor
// stalked arm (Unit.cpp:8640-8647) for a creature target: a live
// SPELL_AURA_MOD_STALKED (68) effect cast by the seer GUID.
func (s *Server) creatureHasStalkedAuraFrom(tm *creatureMotion, seerGUID uint64) bool {
	if s == nil || tm == nil || seerGUID == 0 {
		return false
	}
	key := creatureAuraKeyForMotion(tm)
	if key.GUID == 0 {
		return false
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura != nil && !aura.Stopped && aura.AuraType == spellAuraModStalked && aura.CasterGUID == seerGUID {
			return true
		}
	}
	return false
}

// wildCreatureCanDetectInvisibilityOfCreature is the creature-target twin
// of wildCreatureCanDetectInvisibilityOfPlayer: per-type 18/19 sums on
// both motions, with the same presence-gated value compare and reciprocal
// flags arm.
func (s *Server) wildCreatureCanDetectInvisibilityOfCreature(m, tm *creatureMotion) bool {
	if m == nil || tm == nil {
		return true
	}
	targetInvis := s.totemCreatureAuraMiscSums(tm, spellAuraModInvisibility)
	targetDetect := s.totemCreatureAuraMiscSums(tm, spellAuraInvisibilityDetect)
	seerDetect := s.totemCreatureAuraMiscSums(m, spellAuraInvisibilityDetect)
	for misc, value := range targetInvis {
		detect, ok := seerDetect[misc]
		if !ok || detect < value {
			return false
		}
	}
	seerInvis := s.totemCreatureAuraMiscSums(m, spellAuraModInvisibility)
	for misc := range seerInvis {
		if _, ok := targetDetect[misc]; !ok {
			return false
		}
	}
	return true
}

// wildCreatureCanDetectStealthOfCreature is the creature-target twin of
// wildCreatureCanDetectStealthOfPlayer: the target's stealth types are the
// misc keys of its live MOD_STEALTH (16) sums, its stealth value the
// 16+154 sums, and the seer's 17 sum is per type as in C++.
func (s *Server) wildCreatureCanDetectStealthOfCreature(m, tm *creatureMotion) bool {
	if m == nil || tm == nil {
		return true
	}
	targetStealth := s.totemCreatureAuraMiscSums(tm, spellAuraModStealth)
	if len(targetStealth) == 0 {
		return true
	}
	combatReach := float32(1.5)
	if m.CombatReach > 0 {
		combatReach = m.CombatReach
	}
	dist := float32(distance3D(m.X, m.Y, m.Z, tm.X, tm.Y, tm.Z))
	if dist < combatReach {
		return true
	}
	if !hasInArc(m.Orientation, m.X, m.Y, tm.X, tm.Y, math.Pi) {
		return false
	}
	detectStealth := s.totemCreatureAuraMiscSums(m, spellAuraDetectStealth)
	detectValue := s.totemCreatureAuraMiscSums(m, spellAuraStealthDetect)
	seerLevel := uint32(1)
	if m.Level > 0 {
		seerLevel = m.Level
	}
	targetKey := creatureAuraKeyForMotion(tm)
	targetValue := s.creatureAuraTypeSum(targetKey, spellAuraModStealth) + s.creatureAuraTypeSum(targetKey, spellAuraStealthLevel)
	miscs := make([]int32, 0, len(targetStealth))
	for mv := range targetStealth {
		miscs = append(miscs, mv)
	}
	sort.Slice(miscs, func(i, j int) bool { return miscs[i] < miscs[j] })
	for _, mv := range miscs {
		if _, ok := detectStealth[mv]; ok {
			return true
		}
		detectionValue := int32(30) + int32(seerLevel-1)*5 + detectValue[mv] - targetValue
		visibilityRange := float32(detectionValue)*0.3 + combatReach
		if visibilityRange <= 0 || dist > visibilityRange {
			return false
		}
	}
	return true
}
