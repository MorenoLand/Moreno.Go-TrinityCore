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
	// SPELL_AURA_MOD_STALKED (68) cast by the observer: Unit::IsAlwaysDetectableFor
	// (Unit.cpp:8640-8647) is checked by CanDetect before both invisibility and
	// stealth arms (Object.cpp:1676-1682), so a marked target is always seen.
	// The stalked arm is the Unit level of IsAlwaysDetectableFor and is
	// checked before the Player-level duel arm below, so a marked duel
	// opponent stays always detectable.
	if target.hasStalkedAuraFrom(s.playerGUID) {
		return true
	}
	// Player group leg of IsAlwaysDetectableFor (Player.cpp:22584-22597),
	// replacing the old group-ID-equality arm: same group is not enough,
	// the seer must be group-visible for the target (same subgroup under
	// the default group-visibility config), and a duel opponent past
	// CHALLENGED loses the leg and falls through to the range arms.
	if target.playerGroupAlwaysDetectable(s) {
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
	// Player group leg of IsAlwaysDetectableFor (Player.cpp:22584-22597):
	// a group-visible player target is always detectable, so the
	// invisibility arms below never ran for group members in C++ either.
	// The stealth twin carried this leg as group-ID equality; the
	// invisibility twin had no leg at all.
	if target.playerGroupAlwaysDetectable(s) {
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

	// Per stealth type (Object.cpp:1764): the seer's MOD_STEALTH_DETECT
	// (17) sum for that type widens the range; every type the target
	// carries must pass, as in the routed twin. The target value stays
	// the global 16+154 sum by session-path convention.
	for _, mv := range targetSess.stealthMiscValues() {
		detectionValue := int32(30) + int32(cLevel-1)*5
		if targetSess.server != nil && motion != nil {
			detectionValue += targetSess.server.creatureAuraTypeSumByMisc(creatureAuraKeyForMotion(motion), spellAuraStealthDetect, mv)
		}
		detectionValue -= targetSess.getStealthValue()
		visibilityRange := float32(detectionValue)*0.3 + combatReach
		if visibilityRange <= 0 || dist > visibilityRange {
			return false
		}
	}
	return true
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

	// Per stealth type (Object.cpp:1764-1784): the seer's MOD_STEALTH_DETECT
	// (17) sum for that type widens the range before the +8%/+1.5 alert
	// addition. The band holds only when every type is inside its alert
	// range and at least one type sits outside its normal range; the
	// returned range is the widest alert range, so the caller's
	// aggro-distance no-alert arm suppresses when any type reaches it.
	alerted := false
	maxAlert := float32(0)
	for _, mv := range targetSess.stealthMiscValues() {
		detectionValue := int32(30) + int32(cLevel-1)*5
		if targetSess.server != nil && motion != nil {
			detectionValue += targetSess.server.creatureAuraTypeSumByMisc(creatureAuraKeyForMotion(motion), spellAuraStealthDetect, mv)
		}
		detectionValue -= targetSess.getStealthValue()
		visibilityRange := float32(detectionValue)*0.3 + combatReach
		if visibilityRange <= 0 {
			return false, 0
		}
		alertRange := visibilityRange*1.08 + 1.5
		if dist > alertRange {
			return false, 0
		}
		if dist > visibilityRange {
			alerted = true
			if alertRange > maxAlert {
				maxAlert = alertRange
			}
		}
	}
	if !alerted {
		return false, 0
	}
	return true, maxAlert
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

// canDetectSeer is the resolved WorldObject::CanDetect seer for a sight-scan
// or CanStartAttack-query scanner: a controlled unit judges through its
// charmer-or-owner (Object.cpp:1664-1686; creatureCharmerOrOwnerGUID, the
// charmer GUID while charmed else the owner GUID). sess is set when the
// controller is a player session, motion when it is a creature motion on
// the scanner's map/instance; a wild scanner or an unresolvable controller
// falls back to the scanner itself, so wild behavior is unchanged. This
// is scan/query routing only: the spell gate keeps its own seer plumbing
// (boss_ai.go and pet_combat.go resolve the controller before calling the
// wild twins), and routing inside the wild twins would double-route a
// controller motion through its own controller.
type canDetectSeer struct {
	guid   uint64
	sess   *session
	motion *creatureMotion
}

// canDetectSeerFor resolves the CanDetect seer for a scanner motion.
func (s *Server) canDetectSeerFor(m *creatureMotion) canDetectSeer {
	if m == nil {
		return canDetectSeer{}
	}
	guid := creatureCharmerOrOwnerGUID(m)
	if guid == 0 || guid == m.GUID {
		return canDetectSeer{guid: m.GUID, motion: m}
	}
	if s != nil {
		if cs := s.findSessionByGUID(guid); cs != nil && cs.player != nil {
			return canDetectSeer{guid: guid, sess: cs}
		}
		if cm := s.findCreatureMotion(m.Map, m.InstanceID, guid); cm != nil {
			return canDetectSeer{guid: guid, motion: cm}
		}
	}
	return canDetectSeer{guid: m.GUID, motion: m}
}

// canDetectSeerInvisibility returns a CanDetect seer's
// MOD_INVISIBILITY_DETECT (19) sums and MOD_INVISIBILITY (18) misc set:
// session aura folds for a player controller, live creature-aura sums for
// a creature seer.
func (s *Server) canDetectSeerInvisibility(seer canDetectSeer) (map[int32]int32, map[int32]bool) {
	if seer.sess != nil {
		detect := map[int32]int32{}
		invis := map[int32]bool{}
		for _, aura := range seer.sess.loadedAuras() {
			if aura == nil || aura.Stopped {
				continue
			}
			if aura.AuraType == spellAuraInvisibilityDetect {
				detect[aura.MiscValue] += int32(aura.Amount)
			}
			if aura.AuraType == spellAuraModInvisibility {
				invis[aura.MiscValue] = true
			}
		}
		return detect, invis
	}
	if seer.motion != nil {
		detect := s.totemCreatureAuraMiscSums(seer.motion, spellAuraInvisibilityDetect)
		invis := map[int32]bool{}
		for misc := range s.totemCreatureAuraMiscSums(seer.motion, spellAuraModInvisibility) {
			invis[misc] = true
		}
		return detect, invis
	}
	return map[int32]int32{}, map[int32]bool{}
}

// seerCanDetectInvisibilityOfPlayer is the CanDetect invisibility leg
// (Object.cpp:1688-1717) for a resolved seer: every invisibility type the
// target carries must be covered by the seer's MOD_INVISIBILITY_DETECT
// (19) sum for that type (presence gate plus value compare), and the
// reciprocal flags arm requires every invisibility type the seer carries
// to sit in the target's detect set. The wild twin delegates with a
// self-seer; the scan passes the resolved controller seer.
func (s *Server) seerCanDetectInvisibilityOfPlayer(seer canDetectSeer, ts *session) bool {
	if ts == nil || ts.player == nil {
		return true
	}
	// Player level of IsAlwaysDetectableFor (Player.cpp:22584-22597):
	// a player target group-visible for a player controller seer is
	// always detectable, so the invisibility arms never run for it.
	// A wild seer (sess nil) never reaches this leg in C++.
	if seer.sess != nil && ts.playerGroupAlwaysDetectable(seer.sess) {
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
	seerDetect, seerInvis := s.canDetectSeerInvisibility(seer)
	for misc, value := range targetInvis {
		detect, ok := seerDetect[misc]
		if !ok || detect < value {
			return false
		}
	}
	for misc := range seerInvis {
		if !targetDetectHas[misc] {
			return false
		}
	}
	return true
}

// seerCanDetectInvisibilityOfCreature is the creature-target twin of
// seerCanDetectInvisibilityOfPlayer: per-type 18/19 sums on both sides,
// with the same presence-gated value compare and reciprocal flags arm.
func (s *Server) seerCanDetectInvisibilityOfCreature(seer canDetectSeer, tm *creatureMotion) bool {
	if tm == nil {
		return true
	}
	targetInvis := s.totemCreatureAuraMiscSums(tm, spellAuraModInvisibility)
	targetDetect := s.totemCreatureAuraMiscSums(tm, spellAuraInvisibilityDetect)
	seerDetect, seerInvis := s.canDetectSeerInvisibility(seer)
	for misc, value := range targetInvis {
		detect, ok := seerDetect[misc]
		if !ok || detect < value {
			return false
		}
	}
	for misc := range seerInvis {
		if _, ok := targetDetect[misc]; !ok {
			return false
		}
	}
	return true
}

// seerCanDetectStealthOfPlayer is the CanDetect stealth leg
// (Object.cpp:1719-1790) for a resolved seer, checkAlert=false: contact
// distance and the front-arc gate first, then per stealth type in
// ascending order the DETECT_STEALTH (228) misc-matched auto-detect arm
// and the level/detect-sum range arm. A player seer caps the range at
// MAX_PLAYER_STEALTH_DETECT_RANGE (Object.cpp:1776); a creature seer
// takes the wild formula unchanged. The stalked arm is the caller's: it
// judges by the resolved seer GUID, not the scanner's. The wild twin
// delegates with a self-seer.
func (s *Server) seerCanDetectStealthOfPlayer(seer canDetectSeer, ts *session) bool {
	if ts == nil || ts.player == nil || !ts.isStealthed() {
		return true
	}
	if seer.sess != nil {
		ctrl := seer.sess
		if ctrl.player == nil {
			return true
		}
		// Player level of IsAlwaysDetectableFor (Player.cpp:22584-22597),
		// as in the session twins: a group-visible player target is
		// always detectable for its controller, so the range arms
		// never run for it. The stalked arm stays the caller's.
		if ts.playerGroupAlwaysDetectable(ctrl) {
			return true
		}
		combatReach := float32(1.5)
		if ctrl.player.CombatReach > 0 {
			combatReach = ctrl.player.CombatReach
		}
		dist := float32(distance3D(ctrl.player.X, ctrl.player.Y, ctrl.player.Z, ts.player.X, ts.player.Y, ts.player.Z))
		if dist < combatReach {
			return true
		}
		if !hasInArc(ctrl.player.Orientation, ctrl.player.X, ctrl.player.Y, ts.player.X, ts.player.Y, math.Pi) {
			return false
		}
		seerDetect := map[int32]int32{}
		for _, aura := range ctrl.loadedAuras() {
			if aura == nil || aura.Stopped || aura.AuraType != spellAuraStealthDetect {
				continue
			}
			seerDetect[aura.MiscValue] += int32(aura.Amount)
		}
		seerLevel := uint32(1)
		if ctrl.player.Level > 0 {
			seerLevel = uint32(ctrl.player.Level)
		}
		targetValue := ts.getStealthValue()
		miscs := append([]int32(nil), ts.stealthMiscValues()...)
		sort.Slice(miscs, func(i, j int) bool { return miscs[i] < miscs[j] })
		for _, mv := range miscs {
			if ctrl.hasAuraTypeWithMiscValue(spellAuraDetectStealth, mv) {
				return true
			}
			detectionValue := int32(30) + int32(seerLevel-1)*5 + seerDetect[mv] - targetValue
			visibilityRange := float32(detectionValue)*0.3 + combatReach
			if visibilityRange > 30.0 {
				visibilityRange = 30.0
			}
			if visibilityRange <= 0 || dist > visibilityRange {
				return false
			}
		}
		return true
	}
	if seer.motion == nil {
		return true
	}
	sm := seer.motion
	combatReach := float32(1.5)
	if sm.CombatReach > 0 {
		combatReach = sm.CombatReach
	}
	dist := float32(distance3D(sm.X, sm.Y, sm.Z, ts.player.X, ts.player.Y, ts.player.Z))
	if dist < combatReach {
		return true
	}
	if !hasInArc(sm.Orientation, sm.X, sm.Y, ts.player.X, ts.player.Y, math.Pi) {
		return false
	}
	detectStealth := s.totemCreatureAuraMiscSums(sm, spellAuraDetectStealth)
	detectValue := s.totemCreatureAuraMiscSums(sm, spellAuraStealthDetect)
	seerLevel := uint32(1)
	if sm.Level > 0 {
		seerLevel = sm.Level
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

// seerCanDetectStealthOfCreature is the creature-target twin of
// seerCanDetectStealthOfPlayer: a player seer judges through the
// session-seer twin (30-yard cap included), a creature seer through the
// wild formula with the seer motion substituted.
func (s *Server) seerCanDetectStealthOfCreature(seer canDetectSeer, tm *creatureMotion) bool {
	if tm == nil {
		return true
	}
	if seer.sess != nil {
		return s.playerSeerCanDetectStealthOfCreature(seer.sess, tm)
	}
	if seer.motion == nil {
		return true
	}
	sm := seer.motion
	targetStealth := s.totemCreatureAuraMiscSums(tm, spellAuraModStealth)
	if len(targetStealth) == 0 {
		return true
	}
	combatReach := float32(1.5)
	if sm.CombatReach > 0 {
		combatReach = sm.CombatReach
	}
	dist := float32(distance3D(sm.X, sm.Y, sm.Z, tm.X, tm.Y, tm.Z))
	if dist < combatReach {
		return true
	}
	if !hasInArc(sm.Orientation, sm.X, sm.Y, tm.X, tm.Y, math.Pi) {
		return false
	}
	detectStealth := s.totemCreatureAuraMiscSums(sm, spellAuraDetectStealth)
	detectValue := s.totemCreatureAuraMiscSums(sm, spellAuraStealthDetect)
	seerLevel := uint32(1)
	if sm.Level > 0 {
		seerLevel = sm.Level
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

// creatureSightCanDetectStealthOfPlayer is the scan/query stealth leg for
// a possibly-controlled scanner: the guard arm stays scanner-side
// (GuardAI CanSeeAlways rides the scanner, not the controller), the
// stalked arm judges by the resolved seer GUID, and a routed scanner
// judges through the controller; a wild scanner keeps
// canCreatureDetectStealthOfPlayer, which now carries the per-type
// MOD_STEALTH_DETECT (17) sums as C++ does. The passed dist is the
// scanner-to-target distance and is
// used only on the wild path; routed paths recompute from the controller.
func (s *Server) creatureSightCanDetectStealthOfPlayer(motion *creatureMotion, ts *session, dist float32) bool {
	if ts == nil || ts.player == nil || !ts.isStealthed() {
		return true
	}
	if motion != nil && motion.FlagsExtra&creatureFlagExtraGuard != 0 && motion.TargetGUID != 0 && ts.playerGUID == motion.TargetGUID {
		return true
	}
	seer := s.canDetectSeerFor(motion)
	if seer.guid != 0 && ts.hasStalkedAuraFrom(seer.guid) {
		return true
	}
	if seer.sess != nil || (seer.motion != nil && seer.motion != motion) {
		return s.seerCanDetectStealthOfPlayer(seer, ts)
	}
	return canCreatureDetectStealthOfPlayer(motion, ts, dist)
}

// creatureSightCanDetectCreature is the creature-victim scan's CanDetect
// leg for a possibly-controlled scanner: stalked by the resolved seer
// GUID, then invisibility + stealth through the controller; a wild
// scanner keeps wildCreatureCanDetectCreature byte-identical. No alert
// band: the worker's TriggerAlert arm is player-only
// (GridNotifiers.cpp:139).
func (s *Server) creatureSightCanDetectCreature(motion, victim *creatureMotion) bool {
	if motion == nil || victim == nil {
		return true
	}
	seer := s.canDetectSeerFor(motion)
	if seer.guid != 0 && s.creatureHasStalkedAuraFrom(victim, seer.guid) {
		return true
	}
	if seer.sess != nil {
		if !s.seerCanDetectInvisibilityOfCreature(seer, victim) {
			return false
		}
		return s.playerSeerCanDetectStealthOfCreature(seer.sess, victim)
	}
	if seer.motion != nil && seer.motion != motion {
		if !s.seerCanDetectInvisibilityOfCreature(seer, victim) {
			return false
		}
		return s.seerCanDetectStealthOfCreature(seer, victim)
	}
	return s.wildCreatureCanDetectCreature(motion, victim)
}

// creatureSightAlertBand is the checkAlert=true band for a
// possibly-controlled scanner: geometry, level and reach come from the
// controller (a player controller caps the range at 30 yards before the
// alert addition, Object.cpp:1776-1781; a creature controller rides the
// wild band with controller geometry), while the aggro-distance no-alert
// arm stays the caller's scanner-side check and the turn/distract effect
// stays scanner-side. A wild scanner keeps creatureStealthAlertBand,
// which now carries the per-type MOD_STEALTH_DETECT (17) sums as C++
// does, and a creature controller rides that same band.
func (s *Server) creatureSightAlertBand(motion *creatureMotion, ts *session, dist float32) (bool, float32) {
	if motion == nil || ts == nil || ts.player == nil || !ts.isStealthed() {
		return false, 0
	}
	seer := s.canDetectSeerFor(motion)
	if seer.sess != nil && seer.sess.player != nil {
		ctrl := seer.sess
		combatReach := float32(1.5)
		if ctrl.player.CombatReach > 0 {
			combatReach = ctrl.player.CombatReach
		}
		cdist := float32(distance3D(ctrl.player.X, ctrl.player.Y, ctrl.player.Z, ts.player.X, ts.player.Y, ts.player.Z))
		if cdist < combatReach {
			return false, 0
		}
		if !hasInArc(ctrl.player.Orientation, ctrl.player.X, ctrl.player.Y, ts.player.X, ts.player.Y, math.Pi) {
			return false, 0
		}
		cLevel := uint32(1)
		if ctrl.player.Level > 0 {
			cLevel = uint32(ctrl.player.Level)
		}
		// Player level of IsAlwaysDetectableFor (Player.cpp:22584-22597):
		// a group-visible target is always detectable for its
		// controller, so it never sits in the alert-only band.
		if ts.playerGroupAlwaysDetectable(ctrl) {
			return false, 0
		}
		seerDetect := map[int32]int32{}
		for _, aura := range ctrl.loadedAuras() {
			if aura == nil || aura.Stopped || aura.AuraType != spellAuraStealthDetect {
				continue
			}
			seerDetect[aura.MiscValue] += int32(aura.Amount)
		}
		alerted := false
		maxAlert := float32(0)
		for _, mv := range ts.stealthMiscValues() {
			detectionValue := int32(30) + int32(cLevel-1)*5 + seerDetect[mv] - ts.getStealthValue()
			visibilityRange := float32(detectionValue)*0.3 + combatReach
			if visibilityRange > 30.0 {
				visibilityRange = 30.0
			}
			if visibilityRange <= 0 {
				return false, 0
			}
			alertRange := visibilityRange*1.08 + 1.5
			if cdist > alertRange {
				return false, 0
			}
			if cdist > visibilityRange {
				alerted = true
				if alertRange > maxAlert {
					maxAlert = alertRange
				}
			}
		}
		if !alerted {
			return false, 0
		}
		return true, maxAlert
	}
	if seer.motion != nil && seer.motion != motion {
		cdist := float32(distance3D(seer.motion.X, seer.motion.Y, seer.motion.Z, ts.player.X, ts.player.Y, ts.player.Z))
		return creatureStealthAlertBand(seer.motion, ts, cdist)
	}
	return creatureStealthAlertBand(motion, ts, dist)
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
	if m == nil {
		return true
	}
	return s.seerCanDetectInvisibilityOfPlayer(canDetectSeer{guid: m.GUID, motion: m}, ts)
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
	if m == nil {
		return true
	}
	return s.seerCanDetectStealthOfPlayer(canDetectSeer{guid: m.GUID, motion: m}, ts)
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
	if m == nil {
		return true
	}
	return s.seerCanDetectInvisibilityOfCreature(canDetectSeer{guid: m.GUID, motion: m}, tm)
}

// wildCreatureCanDetectStealthOfCreature is the creature-target twin of
// wildCreatureCanDetectStealthOfPlayer: the target's stealth types are the
// misc keys of its live MOD_STEALTH (16) sums, its stealth value the
// 16+154 sums, and the seer's 17 sum is per type as in C++.
func (s *Server) wildCreatureCanDetectStealthOfCreature(m, tm *creatureMotion) bool {
	if m == nil {
		return true
	}
	return s.seerCanDetectStealthOfCreature(canDetectSeer{guid: m.GUID, motion: m}, tm)
}

// playerSeerCanDetectCreature is the player-seer twin of
// wildCreatureCanDetectCreature: WorldObject::CanDetect (Object.cpp:1664-1686)
// is generic over units, so a player caster judges a creature target through
// the target motion's live creature-aura sums, not unconditionally. Order is
// the C++ order: stalked by the seer (Unit.cpp:8640) first, then
// invisibility (Object.cpp:1688-1717), then stealth (Object.cpp:1719-1790,
// checkAlert=false as on the spell gate). The seer's side rides the session
// aura folds (position, level, combat reach, orientation, 17/19/228 sums);
// the target's side rides the motion's creature-aura sums, so Lua AddAura
// presence-only markers stay invisible here as on the wild twin. GM seers
// pass inside, mirroring canDetectStealthOf/canDetectInvisibilityOfPlayer.
// Unresolvable seer/target and a target carrying no stealth or invisibility
// aura state pass; the reciprocal invisibility arm still judges a seer
// carrying invisibility against a target that cannot detect it.
func (s *Server) playerSeerCanDetectCreature(seer *session, tm *creatureMotion) bool {
	if s == nil || seer == nil || seer.player == nil || tm == nil {
		return true
	}
	if (seer.player.ExtraFlags&playerExtraGMOn != 0) || (seer.player.PlayerFlags&playerFlagGM != 0) || seer.security > 0 {
		return true
	}
	if s.creatureHasStalkedAuraFrom(tm, seer.playerGUID) {
		return true
	}
	if !s.playerSeerCanDetectInvisibilityOfCreature(seer, tm) {
		return false
	}
	return s.playerSeerCanDetectStealthOfCreature(seer, tm)
}

// playerSeerCanDetectInvisibilityOfCreature mirrors
// WorldObject::CanDetectInvisibilityOf (Object.cpp:1688-1717) with a player
// seer and a creature target: every invisibility type the target carries
// must be covered by the seer's MOD_INVISIBILITY_DETECT (19) sum for that
// type (presence gate plus value compare), and the reciprocal flags arm
// (Object.cpp:1704-1706) requires every invisibility type the seer carries
// to sit in the target's detect set.
func (s *Server) playerSeerCanDetectInvisibilityOfCreature(seer *session, tm *creatureMotion) bool {
	if s == nil || seer == nil || seer.player == nil || tm == nil {
		return true
	}
	targetInvis := s.totemCreatureAuraMiscSums(tm, spellAuraModInvisibility)
	targetDetect := s.totemCreatureAuraMiscSums(tm, spellAuraInvisibilityDetect)
	seerDetect := map[int32]int32{}
	seerInvis := map[int32]bool{}
	for _, aura := range seer.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		if aura.AuraType == spellAuraInvisibilityDetect {
			seerDetect[aura.MiscValue] += int32(aura.Amount)
		}
		if aura.AuraType == spellAuraModInvisibility {
			seerInvis[aura.MiscValue] = true
		}
	}
	for misc, value := range targetInvis {
		detect, ok := seerDetect[misc]
		if !ok || detect < value {
			return false
		}
	}
	for misc := range seerInvis {
		if _, ok := targetDetect[misc]; !ok {
			return false
		}
	}
	return true
}

// playerSeerCanDetectStealthOfCreature mirrors
// WorldObject::CanDetectStealthOf (Object.cpp:1719-1790) with a player seer
// and a creature target, checkAlert=false: contact distance and the
// front-arc gate first, then per stealth type in ascending order the
// DETECT_STEALTH (228) misc-matched auto-detect arm and the level/detect-sum
// range arm. The 30-yard cap applies because the seer is a player
// (Object.cpp:1776), unlike the creature-seer twin. The target's stealth
// value follows the wild twin's convention (global 16+154 sum); the seer's
// MOD_STEALTH_DETECT (17) sum is per type (Object.cpp:1764).
func (s *Server) playerSeerCanDetectStealthOfCreature(seer *session, tm *creatureMotion) bool {
	if s == nil || seer == nil || seer.player == nil || tm == nil {
		return true
	}
	targetStealth := s.totemCreatureAuraMiscSums(tm, spellAuraModStealth)
	if len(targetStealth) == 0 {
		return true
	}
	combatReach := float32(1.5)
	if seer.player.CombatReach > 0 {
		combatReach = seer.player.CombatReach
	}
	dist := float32(distance3D(seer.player.X, seer.player.Y, seer.player.Z, tm.X, tm.Y, tm.Z))
	if dist < combatReach {
		return true
	}
	if !hasInArc(seer.player.Orientation, seer.player.X, seer.player.Y, tm.X, tm.Y, math.Pi) {
		return false
	}
	seerDetect := map[int32]int32{}
	for _, aura := range seer.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraStealthDetect {
			continue
		}
		seerDetect[aura.MiscValue] += int32(aura.Amount)
	}
	seerLevel := uint32(1)
	if seer.player.Level > 0 {
		seerLevel = uint32(seer.player.Level)
	}
	targetKey := creatureAuraKeyForMotion(tm)
	targetValue := s.creatureAuraTypeSum(targetKey, spellAuraModStealth) + s.creatureAuraTypeSum(targetKey, spellAuraStealthLevel)
	miscs := make([]int32, 0, len(targetStealth))
	for mv := range targetStealth {
		miscs = append(miscs, mv)
	}
	sort.Slice(miscs, func(i, j int) bool { return miscs[i] < miscs[j] })
	for _, mv := range miscs {
		if seer.hasAuraTypeWithMiscValue(spellAuraDetectStealth, mv) {
			return true
		}
		detectionValue := int32(30) + int32(seerLevel-1)*5 + seerDetect[mv] - targetValue
		visibilityRange := float32(detectionValue)*0.3 + combatReach
		if visibilityRange > 30.0 {
			visibilityRange = 30.0
		}
		if visibilityRange <= 0 || dist > visibilityRange {
			return false
		}
	}
	return true
}

// secGameMaster is SEC_GAMEMASTER on the AccountTypes scale (SEC_PLAYER 0,
// SEC_MODERATOR 1, SEC_GAMEMASTER 2): the GM-visibility value
// Unit::SetVisible(false) stores on a script-hidden creature (Unit.cpp:8659)
// and the compare point of the CanSeeOrDetect GM leg.
const secGameMaster uint8 = 2

// ghostBits carries one side of the SERVERSIDE_VISIBILITY_GHOST
// intersection test in WorldObject::CanSeeOrDetect (Object.cpp:1632): the
// target's visibility bits against the seer's detect bits. The ghost leg
// fires only when the intersection is empty.
type ghostBits struct {
	alive bool
	ghost bool
}

// playerGhostBits returns a player's visibility/detect bits: a ghost
// player carries GHOST only (SpellAuraEffects.cpp:1567-1568, the
// SPELL_AURA_GHOST apply arm), an alive player ALIVE only
// (SpellAuraEffects.cpp:1576-1577, the remove arm).
func playerGhostBits(isGhost bool) ghostBits {
	if isGhost {
		return ghostBits{ghost: true}
	}
	return ghostBits{alive: true}
}

// creatureGhostBits returns a creature's visibility bits and detect bits
// (Object.cpp:988-989 defaults, Creature.cpp:1142/1176): spirit healers,
// spirit guides and CREATURE_FLAG_EXTRA_GHOST_VISIBILITY (0x400) creatures
// carry GHOST on both sides; CREATURE_TYPE_FLAG_GHOST_VISIBLE creatures are
// visible to both sides (ALIVE|GHOST visibility, ALIVE detect); every other
// creature carries ALIVE on both sides.
func creatureGhostBits(m *creatureMotion) (vis, detect ghostBits) {
	if m == nil {
		return ghostBits{alive: true}, ghostBits{alive: true}
	}
	if m.NPCFlags&(npcFlagSpiritHealer|npcFlagSpiritGuide) != 0 || m.FlagsExtra&0x400 != 0 {
		return ghostBits{ghost: true}, ghostBits{ghost: true}
	}
	if m.TypeFlags&creatureTypeFlagGhostVisible != 0 {
		return ghostBits{alive: true, ghost: true}, ghostBits{alive: true}
	}
	return ghostBits{alive: true}, ghostBits{alive: true}
}

// playerTargetGMValue is a player target's SERVERSIDE_VISIBILITY_GM value:
// the account security while .gm visible off (Player::SetGMVisible(false),
// Player.cpp:2518, stores GetSecurity()), SEC_PLAYER otherwise. The marker
// is playerExtraGMInvisible; .gm on alone (SetGameMaster) changes only the
// detect side.
func playerTargetGMValue(ts *session) uint8 {
	if ts == nil || ts.player == nil || ts.player.ExtraFlags&playerExtraGMInvisible == 0 {
		return 0
	}
	return ts.security
}

// creatureTargetGMValue is a creature target's SERVERSIDE_VISIBILITY_GM
// value: SEC_GAMEMASTER while script-hidden (Unit::SetVisible(false)),
// SEC_PLAYER otherwise.
func creatureTargetGMValue(tm *creatureMotion) uint8 {
	if tm != nil && tm.ScriptHidden {
		return secGameMaster
	}
	return 0
}

// gmLegVerdict runs the GM leg of WorldObject::CanSeeOrDetect
// (Object.cpp:1622-1630) and reports whether it decides the verdict: a
// GM-mode seer (nonzero detect) sees a normally-visible target outright,
// skipping the ghost, despawn and detect legs; a hidden target is visible
// only to a seer whose detect level reaches its value.
func gmLegVerdict(seerDetectGM, targetGMValue uint8) (decided, visible bool) {
	if targetGMValue == 0 {
		if seerDetectGM != 0 {
			return true, true
		}
		return false, false
	}
	return true, seerDetectGM >= targetGMValue
}

// ghostLegVerdict runs the ghost leg of WorldObject::CanSeeOrDetect
// (Object.cpp:1632-1647): when the target's visibility bits and the seer's
// detect bits share nothing, only a player seer against a player target of
// the same team that is group-visible for the seer still sees the target;
// every other seer/target pair fails.
func ghostLegVerdict(seerDetect, targetVis ghostBits, seerIsPlayer, targetIsPlayer, sameTeamAndGroupVisible bool) (decided, visible bool) {
	if (targetVis.alive && seerDetect.alive) || (targetVis.ghost && seerDetect.ghost) {
		return false, false
	}
	if seerIsPlayer && targetIsPlayer {
		return true, sameTeamAndGroupVisible
	}
	return true, false
}

// playerGroupAlwaysDetectable mirrors the Player level of
// IsAlwaysDetectableFor (Player.cpp:22584-22597) at the CanDetect level:
// a player target is always detectable for a player seer group-visible
// for it under the default group-visibility config (IsGroupVisibleFor
// defaulting to IsInSameGroupWith: the same session, or the same group
// in the same subgroup, via sessionsInSameGroupWith). The duel arm
// suppresses the leg, not detection itself: while the target's duel
// against the seer has left CHALLENGED (countdown running, or DuelTeam
// set once in progress), the target is not always detectable for that
// seer and the normal invisibility/stealth arms judge instead; a merely
// challenged duel leaves the leg live. Callers check the Unit-level
// stalked arm before this leg, as C++ does. Creature seers never reach
// this leg (seer->ToPlayer() is null in C++), and creature targets have
// no Player level at all, so the wild and creature-target twins are
// untouched; the controlled/pet session-seer paths inherit it through
// the two session detect functions.
func (target *session) playerGroupAlwaysDetectable(seer *session) bool {
	if target == nil || target.player == nil || seer == nil || seer.player == nil {
		return false
	}
	if target.duelPartner == seer.playerGUID && (target.duelCountdown || target.player.DuelTeam != 0) {
		return false
	}
	srv := target.server
	if srv == nil {
		srv = seer.server
	}
	if srv == nil {
		return false
	}
	return srv.sessionsInSameGroupWith(seer, target)
}

// sessionsInSameGroupWith mirrors Player::IsInSameGroupWith
// (Player.cpp:2537-2541) under the default group-visibility config: the
// same session, or two sessions in one group in the same subgroup. A
// missing group row reads as not group-visible.
func (s *Server) sessionsInSameGroupWith(a, b *session) bool {
	if s == nil || a == nil || b == nil || a.player == nil || b.player == nil {
		return false
	}
	if a.playerGUID == b.playerGUID {
		return true
	}
	if a.groupID == 0 || a.groupID != b.groupID {
		return false
	}
	g := s.findGroupByID(a.groupID)
	if g == nil {
		return false
	}
	as, okA := g.memberSubGroup(a.playerGUID)
	bs, okB := g.memberSubGroup(b.playerGUID)
	return okA && okB && as == bs
}

// spellGatePlayerNeverSeesPlayer runs the CanNeverSee leg
// (Object.cpp:1659-1662: different map or disjoint phase) plus the
// IsNeverVisible player legs (Player.cpp:22560-22568: logout or loading)
// for a player seer against a player target. A different instance is a
// different Map object in C++, so the instance id rides the map leg.
// The seer's phase mask is currentPlayerPhaseMask (GM players carry
// PHASEMASK_ANYWHERE, Player.cpp:2456, SetGameMaster). The
// findSessionByGUID worldReady gate makes the loading half unreachable
// through the spell gates; it is kept so the leg reads complete
// against the C++.
func spellGatePlayerNeverSeesPlayer(seer, ts *session) bool {
	if seer == nil || seer.player == nil || ts == nil || ts.player == nil {
		return false
	}
	if ts.loggingOut || ts.playerLoading {
		return true
	}
	if seer.player.Map != ts.player.Map || seer.player.InstanceID != ts.player.InstanceID {
		return true
	}
	return seer.currentPlayerPhaseMask()&ts.currentPlayerPhaseMask() == 0
}

// creatureEffectivePhaseMask is a creature motion's live phase mask for
// the CanNeverSee phase arm: the runtime 261-phase override while one is
// active (the totem helper's read), else motion.PhaseMask, else
// PHASEMASK_NORMAL (1) for motions whose spawn row was never read.
func (s *Server) creatureEffectivePhaseMask(m *creatureMotion) uint32 {
	if m == nil {
		return 1
	}
	if s != nil {
		s.auraMu.Lock()
		mask, ok := s.creaturePhaseOverride[creatureAuraKeyForMotion(m)]
		s.auraMu.Unlock()
		if ok && mask != 0 {
			return mask
		}
	}
	if m.PhaseMask != 0 {
		return m.PhaseMask
	}
	return 1
}

// spellGatePlayerNeverSeesCreature runs CanNeverSee for a player seer
// against a creature target: map/instance (vacuous through the
// seer-scoped motion lookup, kept for callers passing a foreign
// motion) and the phase intersection.
func (s *Server) spellGatePlayerNeverSeesCreature(seer *session, tm *creatureMotion) bool {
	if seer == nil || seer.player == nil || tm == nil {
		return false
	}
	if seer.player.Map != tm.Map || seer.player.InstanceID != tm.InstanceID {
		return true
	}
	return seer.currentPlayerPhaseMask()&s.creatureEffectivePhaseMask(tm) == 0
}

// spellGateCreatureNeverSeesPlayer runs CanNeverSee for a creature seer
// (wild, controlled, or pet motion) against a player target: the
// caster motion's map/instance/phase, never the controller session's
// (CanNeverSee reads this; only CanDetect routes through the
// controller, Object.cpp:1664-1680).
func (s *Server) spellGateCreatureNeverSeesPlayer(m *creatureMotion, ts *session) bool {
	if m == nil || ts == nil || ts.player == nil {
		return false
	}
	if m.Map != ts.player.Map || m.InstanceID != ts.player.InstanceID {
		return true
	}
	return s.creatureEffectivePhaseMask(m)&ts.currentPlayerPhaseMask() == 0
}

// spellGateCreatureNeverSeesCreature runs CanNeverSee for a creature
// seer against a creature target.
func (s *Server) spellGateCreatureNeverSeesCreature(m, tm *creatureMotion) bool {
	if m == nil || tm == nil {
		return false
	}
	if m.Map != tm.Map || m.InstanceID != tm.InstanceID {
		return true
	}
	return s.creatureEffectivePhaseMask(m)&s.creatureEffectivePhaseMask(tm) == 0
}

// creatureOwnerGroupVisibleFor runs the second IsAlwaysVisibleFor leg
// (Unit.cpp:8624-8628): a unit whose owner is a player is always
// visible to a player the owner is group-visible for. GetOwner is the
// owner, not the charmer, so the motion's OwnerGUID is the read, and
// the group read is the default-config IsInSameGroupWith twin
// (sessionsInSameGroupWith). An offline owner resolves to no session
// and the leg stays out.
func (s *Server) creatureOwnerGroupVisibleFor(seer *session, tm *creatureMotion) bool {
	if s == nil || seer == nil || seer.player == nil || tm == nil || tm.OwnerGUID == 0 {
		return false
	}
	os := s.findSessionByGUID(tm.OwnerGUID)
	if os == nil || os.player == nil {
		return false
	}
	return s.sessionsInSameGroupWith(seer, os)
}

// playerSeerSpellGateVisibleToPlayer runs the spell gate's visibility
// check (SpellInfo::CheckTarget, SpellInfo.cpp:1641-1642) for a player
// seer against a player target: the CanNeverSee map/phase and
// IsNeverVisible legs, the CanAlwaysSee farsight leg, the GM leg, then
// the ghost team/group leg, then the detect sums. The despawn leg cannot
// fire for players (Object.h:573 base, no Unit override) and the
// distanceCheck legs (sight-range, corpse-visibility,
// vehicle-accessory) never run: CheckTarget calls CanSeeOrDetect with
// the defaults (Object.h:391).
func (s *session) playerSeerSpellGateVisibleToPlayer(ts *session) bool {
	if s == nil || s.player == nil || ts == nil || ts.player == nil {
		return true
	}
	if spellGatePlayerNeverSeesPlayer(s, ts) {
		return false
	}
	// CanAlwaysSee (Player.cpp:22571-22580): the seer's farsight
	// viewpoint is always visible; the being-moved leg is vacuous for
	// player targets (the mover is the seer, a possessed creature, or
	// a vehicle, never another player).
	if s.player.FarsightGUID == ts.playerGUID {
		return true
	}
	if decided, visible := gmLegVerdict(s.gmVisibilityDetect, playerTargetGMValue(ts)); decided {
		return visible
	}
	seerGhost := s.player.PlayerFlags&playerFlagGhost != 0
	targetGhost := ts.player.PlayerFlags&playerFlagGhost != 0
	sameTeamAndGroup := raceTeam(s.player.Race) == raceTeam(ts.player.Race) && s.server.sessionsInSameGroupWith(s, ts)
	if decided, visible := ghostLegVerdict(playerGhostBits(seerGhost), playerGhostBits(targetGhost), true, true, sameTeamAndGroup); decided {
		return visible
	}
	return s.canDetectStealthOf(ts) && s.canDetectInvisibilityOfPlayer(ts)
}

// playerSeerMovedUnitGUID mirrors Unit::GetUnitBeingMoved for a player
// seer (Player::CanAlwaysSee, Player.cpp:22571-22575): the possessed
// creature while possessing (SetClientControl -> SetMovedUnit,
// Player.cpp:24239-24272), else the driven vehicle while driving a
// control seat (moverGUID's vehicle arm), else 0 — the self case is
// already true at Object.cpp:1575 and never needs the leg, and 0
// keeps a self-moving seer from matching a motion GUID numerically.
func (s *session) playerSeerMovedUnitGUID() uint64 {
	if s == nil || s.player == nil {
		return 0
	}
	if g := s.possessedCreatureGUID(); g != 0 {
		return g
	}
	if s.controlsVehicle() {
		return s.player.VehicleGUID
	}
	return 0
}

// playerSeerSpellGateVisibleToCreature is the creature-target twin: GM leg
// (script-hidden creatures need a GM-mode seer), ghost leg (alive seers
// cannot target ghost-only creatures, ghost seers cannot target alive-only
// ones), then the player-seer detect core. The despawn leg stays out:
// motions carry no corpse-remove-time model (Creature::IsInvisibleDueToDespawn,
// Creature.cpp:1936-1948, needs the corpse delay expiry, which Go never
// records).
func (s *Server) playerSeerSpellGateVisibleToCreature(seer *session, tm *creatureMotion) bool {
	if s == nil || seer == nil || seer.player == nil || tm == nil {
		return true
	}
	if s.spellGatePlayerNeverSeesCreature(seer, tm) {
		return false
	}
	// CanAlwaysSee legs (Player.cpp:22571-22580): the farsight
	// viewpoint, as on the player-target twin, and the unit being
	// moved (GetUnitBeingMoved, Unit.h:1203) via
	// playerSeerMovedUnitGUID — a driven vehicle or a possessed
	// creature. A possessed target is also covered by the charmer leg
	// below; the moved-unit leg is what carries the driven-vehicle
	// case. The stale "sessions carry no vehicle GUID" note is
	// corrected: player.VehicleGUID plus moverGUID carry it.
	if seer.player.FarsightGUID == tm.GUID {
		return true
	}
	if seer.playerSeerMovedUnitGUID() == tm.GUID {
		return true
	}
	if s.creatureOwnerGroupVisibleFor(seer, tm) {
		return true
	}
	// IsAlwaysVisibleFor (Object.cpp:1581, Unit.cpp:8615-8621) precedes
	// the GM leg: a unit is always visible to its charmer-or-owner.
	if creatureCharmerOrOwnerGUID(tm) == seer.playerGUID {
		return true
	}
	if decided, visible := gmLegVerdict(seer.gmVisibilityDetect, creatureTargetGMValue(tm)); decided {
		return visible
	}
	targetVis, _ := creatureGhostBits(tm)
	seerGhost := seer.player.PlayerFlags&playerFlagGhost != 0
	if decided, visible := ghostLegVerdict(playerGhostBits(seerGhost), targetVis, true, false, false); decided {
		return visible
	}
	return s.playerSeerCanDetectCreature(seer, tm)
}

// creatureSeerSpellGateVisibleToPlayer is the wild/controlled-creature
// seer twin for player targets: the creature's own GM detect is 0
// (creatures carry no security), so GM-hidden player targets fail; the
// ghost leg uses the creature's own detect bits, not the controller's
// (only CanDetect, Object.cpp:1664-1680, routes through the controller);
// then the wild detect core.
func (s *Server) creatureSeerSpellGateVisibleToPlayer(m *creatureMotion, ts *session) bool {
	if s == nil || m == nil || ts == nil || ts.player == nil {
		return true
	}
	if s.spellGateCreatureNeverSeesPlayer(m, ts) {
		return false
	}
	// No always leg: CanAlwaysSee is base-false for creatures, and
	// the target player's charmer-or-owner leg has no Go model
	// (sessions carry no charmer GUID for mind-controlled players).
	if decided, visible := gmLegVerdict(0, playerTargetGMValue(ts)); decided {
		return visible
	}
	_, seerDetect := creatureGhostBits(m)
	targetGhost := ts.player.PlayerFlags&playerFlagGhost != 0
	if decided, visible := ghostLegVerdict(seerDetect, playerGhostBits(targetGhost), false, true, false); decided {
		return visible
	}
	return s.wildCreatureCanDetectPlayer(m, ts)
}

// creatureSeerSpellGateVisibleToCreature is the creature-target twin,
// with the despawn leg out for the same missing corpse-time model.
func (s *Server) creatureSeerSpellGateVisibleToCreature(m, tm *creatureMotion) bool {
	if s == nil || m == nil || tm == nil {
		return true
	}
	if s.spellGateCreatureNeverSeesCreature(m, tm) {
		return false
	}
	if creatureCharmerOrOwnerGUID(tm) == m.GUID {
		return true
	}
	if decided, visible := gmLegVerdict(0, creatureTargetGMValue(tm)); decided {
		return visible
	}
	_, seerDetect := creatureGhostBits(m)
	targetVis, _ := creatureGhostBits(tm)
	if decided, visible := ghostLegVerdict(seerDetect, targetVis, false, false, false); decided {
		return visible
	}
	return s.wildCreatureCanDetectCreature(m, tm)
}

// creatureSeerSpellGateVisibleTarget resolves the target like
// wildCreatureCanDetectTarget and runs the matching twin; unresolvable
// targets fail open as on the detect core.
func (s *Server) creatureSeerSpellGateVisibleTarget(m *creatureMotion, targetGUID uint64) bool {
	if s == nil || m == nil || targetGUID == 0 || targetGUID == m.GUID {
		return true
	}
	if ts := s.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		return s.creatureSeerSpellGateVisibleToPlayer(m, ts)
	}
	if tm := s.findCreatureMotion(m.Map, m.InstanceID, targetGUID); tm != nil {
		return s.creatureSeerSpellGateVisibleToCreature(m, tm)
	}
	return true
}

// controlledCreatureSeerSpellGateVisibleToPlayer is the charmed/owned
// creature-caster twin for player targets: GM and ghost legs judge through
// the caster motion's own values (CanSeeOrDetect reads this, not the
// controller), while the detect sums still judge through the controller
// session (CanDetect routing).
func (s *Server) controlledCreatureSeerSpellGateVisibleToPlayer(m *creatureMotion, seer, ts *session) bool {
	if s == nil || m == nil || seer == nil || ts == nil || ts.player == nil {
		return true
	}
	if s.spellGateCreatureNeverSeesPlayer(m, ts) {
		return false
	}
	if decided, visible := gmLegVerdict(0, playerTargetGMValue(ts)); decided {
		return visible
	}
	_, seerDetect := creatureGhostBits(m)
	targetGhost := ts.player.PlayerFlags&playerFlagGhost != 0
	if decided, visible := ghostLegVerdict(seerDetect, playerGhostBits(targetGhost), false, true, false); decided {
		return visible
	}
	return seer.canDetectStealthOf(ts) && seer.canDetectInvisibilityOfPlayer(ts)
}

// controlledCreatureSeerSpellGateVisibleToCreature is the
// creature-target twin of controlledCreatureSeerSpellGateVisibleToPlayer:
// the head legs and the GM/ghost legs read the caster motion (this),
// while the detect sums judge through the controller session's
// player-seer detect core (CanDetect routing, Object.cpp:1664-1680).
// Callers exclude self targets (this == obj, Object.cpp:1575).
func (s *Server) controlledCreatureSeerSpellGateVisibleToCreature(m *creatureMotion, seer *session, tm *creatureMotion) bool {
	if s == nil || m == nil || seer == nil || seer.player == nil || tm == nil {
		return true
	}
	if s.spellGateCreatureNeverSeesCreature(m, tm) {
		return false
	}
	if creatureCharmerOrOwnerGUID(tm) == m.GUID {
		return true
	}
	if decided, visible := gmLegVerdict(0, creatureTargetGMValue(tm)); decided {
		return visible
	}
	_, seerDetect := creatureGhostBits(m)
	targetVis, _ := creatureGhostBits(tm)
	if decided, visible := ghostLegVerdict(seerDetect, targetVis, false, false, false); decided {
		return visible
	}
	return s.playerSeerCanDetectCreature(seer, tm)
}

// petSeerSpellGateVisibleToPlayer is the pet-caster twin: the pet's own GM
// detect is 0 and its ghost detect is its own creature bits, while the
// detect sums judge through the driving session's core as before.
func (s *Server) petSeerSpellGateVisibleToPlayer(motion *creatureMotion, detectSeer, ts *session) bool {
	if s == nil || detectSeer == nil || ts == nil || ts.player == nil {
		return true
	}
	// CanNeverSee reads the pet motion, not the driving session.
	if s.spellGateCreatureNeverSeesPlayer(motion, ts) {
		return false
	}
	if decided, visible := gmLegVerdict(0, playerTargetGMValue(ts)); decided {
		return visible
	}
	_, seerDetect := creatureGhostBits(motion)
	targetGhost := ts.player.PlayerFlags&playerFlagGhost != 0
	if decided, visible := ghostLegVerdict(seerDetect, playerGhostBits(targetGhost), false, true, false); decided {
		return visible
	}
	return detectSeer.canDetectStealthOf(ts) && detectSeer.canDetectInvisibilityOfPlayer(ts)
}

// petSeerSpellGateVisibleToCreature is the pet-caster creature-target
// twin, with the same seer split and the despawn leg out.
func (s *Server) petSeerSpellGateVisibleToCreature(motion *creatureMotion, detectSeer *session, tm *creatureMotion) bool {
	if s == nil || detectSeer == nil || tm == nil {
		return true
	}
	if s.spellGateCreatureNeverSeesCreature(motion, tm) {
		return false
	}
	if motion != nil && creatureCharmerOrOwnerGUID(tm) == motion.GUID {
		return true
	}
	if decided, visible := gmLegVerdict(0, creatureTargetGMValue(tm)); decided {
		return visible
	}
	_, seerDetect := creatureGhostBits(motion)
	targetVis, _ := creatureGhostBits(tm)
	if decided, visible := ghostLegVerdict(seerDetect, targetVis, false, false, false); decided {
		return visible
	}
	return s.playerSeerCanDetectCreature(detectSeer, tm)
}

// creatureCharmerOrOwnerGUID mirrors Unit::GetCharmerOrOwnerGUID
// (Unit.h:1170): the charmer GUID while charmed, else the owner GUID.
// IsAlwaysVisibleFor (Unit.cpp:8615-8621) compares it against the seer.
func creatureCharmerOrOwnerGUID(m *creatureMotion) uint64 {
	if m == nil {
		return 0
	}
	if m.CharmerGUID != 0 {
		return m.CharmerGUID
	}
	return m.OwnerGUID
}
