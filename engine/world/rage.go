package world

import "context"

const (
	powerRage = 1 // POWER_RAGE (SharedDefines.h power enum)

	spellAuraShareDamagePct         = 300   // SPELL_AURA_SHARE_DAMAGE_PCT (SpellAuraDefines.h:380)
	spellAuraModRageFromDamageDealt = 213   // SPELL_AURA_MOD_RAGE_FROM_DAMAGE_DEALT (SpellAuraDefines.h:293)
	spellBerserkerRage              = 18499 // Berserker Rage (doubles rage from damage taken, Unit.cpp:13471)
)

// rewardRagePoints mirrors Unit::RewardRage (Unit.cpp:13449-13478): the
// rage-conversion curve by level and the attacker/victim multipliers.
// Returns rage in POWER_RAGE units (1/10 point). RATE_POWER_RAGE_INCOME
// defaults to 1.0 in C++ and Go has no rate config (runes.go precedent), so
// it is folded as 1.0. The victim (taken) leg has no weaponSpeedHitFactor;
// the attacker (dealt) leg adds it before the 1/2 split and scales with
// SPELL_AURA_MOD_RAGE_FROM_DAMAGE_DEALT.
func rewardRagePoints(level, damage, weaponSpeedHitFactor uint32, attacker bool, rageFromDamageDealtPct int32, berserkerRage bool) uint32 {
	if level == 0 {
		level = 1
	}
	lvl := float64(level)
	rageconversion := 0.0091107836*lvl*lvl + 3.225598133*lvl + 4.2652911
	if level > 70 {
		rageconversion += 13.27 * float64(level-70)
	}
	var addRage float64
	if attacker {
		addRage = (float64(damage)/rageconversion*7.5 + float64(weaponSpeedHitFactor)) / 2
		addRage += addRage * float64(rageFromDamageDealtPct) / 100
	} else {
		addRage = float64(damage) / rageconversion * 2.5
		if berserkerRage {
			addRage *= 2
		}
	}
	return uint32(addRage * 10)
}

// rageFromDamageDealtPct sums the attacker's SPELL_AURA_MOD_RAGE_FROM_DAMAGE_DEALT
// (213) modifiers (Unit::RewardRage, Unit.cpp:13464, via GetTotalAuraModifier).
func (s *session) rageFromDamageDealtPct() int32 {
	if s == nil || s.player == nil {
		return 0
	}
	var total int32
	s.castMu.Lock()
	defer s.castMu.Unlock()
	for _, aura := range s.activeAuras {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraModRageFromDamageDealt {
			continue
		}
		total += int32(aura.Amount)
	}
	return total
}

// hasBerserkerRage reports the victim-side Berserker Rage (18499) arm of
// Unit::RewardRage (Unit.cpp:13470-13472), which doubles rage from damage taken.
func (s *session) hasBerserkerRage() bool {
	if s == nil || s.player == nil {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	aura, ok := s.activeAuras[spellBerserkerRage]
	return ok && aura != nil && !aura.Stopped
}

// grantRageFromDamageTaken mirrors the victim-side RewardRage leg of
// Unit::DealDamage (Unit.cpp:915-924): a surviving rage-user victim gains
// rage from damage + absorbed (no weaponSpeedHitFactor). Callers pass the
// absorbed-inclusive amount; absorbed-to-zero hits pass the absorbed amount
// (Unit.cpp:815-819).
func (s *session) grantRageFromDamageTaken(ctx context.Context, dmg uint32) {
	if s == nil || s.player == nil || s.player.Health == 0 || dmg == 0 {
		return
	}
	if playerPowerType(s.player) != powerRage {
		return
	}
	pts := rewardRagePoints(uint32(s.player.Level), dmg, 0, false, 0, s.hasBerserkerRage())
	if pts == 0 {
		return
	}
	s.adjustSpellPower(ctx, s.playerGUID, powerRage, int64(pts))
}

// grantCreatureRageFromDamageTaken is the creature-victim side of the
// DealDamage rage-received leg (Unit.cpp:915-924): rage-user creatures gain
// rage when they survive a hit. Go creature motions carry Powers[1] with a
// 100 max (autobalance_creature.go), so the bridge is the clamped
// ModifyPower analog plus the values-update broadcast.
func (sv *Server) grantCreatureRageFromDamageTaken(key creatureAuraKey, level uint32, dmg uint32) {
	if sv == nil || dmg == 0 {
		return
	}
	sv.motionMu.Lock()
	motion := sv.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	next, changed := sv.addCreatureRageLocked(motion, level, dmg)
	var mapID, instanceID uint32
	var guid uint64
	if changed {
		mapID, instanceID, guid = motion.Map, motion.InstanceID, motion.GUID
	}
	sv.motionMu.Unlock()
	if changed {
		sv.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldPower1 + powerRage: next})
	}
}

// addCreatureRageLocked applies the rage-received RewardRage to a motion while
// the caller holds motionMu; it returns the new rage value and whether it
// changed (the caller owns the broadcast).
func (sv *Server) addCreatureRageLocked(motion *creatureMotion, level uint32, dmg uint32) (uint32, bool) {
	if motion == nil || dmg == 0 || motion.PowerType != powerRage || motion.Health == 0 {
		return 0, false
	}
	pts := rewardRagePoints(level, dmg, 0, false, 0, false)
	if pts == 0 {
		return 0, false
	}
	maximum := motion.MaxPowers[powerRage]
	if maximum == 0 {
		return 0, false
	}
	next := motion.Powers[powerRage] + pts
	if next < motion.Powers[powerRage] || next > maximum {
		next = maximum
	}
	if next == motion.Powers[powerRage] {
		return next, false
	}
	motion.Powers[powerRage] = next
	return next, true
}

// shareDamageEntry is one live SPELL_AURA_SHARE_DAMAGE_PCT effect on a damage
// victim: the aura's caster receives the copied damage.
type shareDamageEntry struct {
	casterGUID uint64
	amount     uint32
	schoolMask uint32
}

// collectShareDamageAuras snapshots the victim's live aura-300 effects whose
// misc school mask covers the damage school (Unit::DealDamage, Unit.cpp:766-772).
// The C++ arm copies the aura list before iterating so mid-iteration removals
// are safe; the snapshot plays that role. Entries whose aura is no longer
// applied on the victim (TargetGUID mismatch) are skipped, mirroring the
// IsAppliedOnTarget check.
func (s *session) collectShareDamageAuras(victimGUID uint64, victimIsPlayer bool, key creatureAuraKey, schoolMask uint32) []shareDamageEntry {
	if s == nil || s.server == nil || schoolMask == 0 {
		return nil
	}
	var out []shareDamageEntry
	accept := func(aura *activeAura) {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraShareDamagePct {
			return
		}
		if aura.TargetGUID != 0 && aura.TargetGUID != victimGUID {
			return
		}
		if uint32(aura.MiscValue)&schoolMask == 0 {
			return
		}
		if aura.CasterGUID == 0 {
			return
		}
		out = append(out, shareDamageEntry{casterGUID: aura.CasterGUID, amount: aura.Amount, schoolMask: aura.SchoolMask})
	}
	if victimIsPlayer {
		ts := s.server.findSessionByGUID(victimGUID)
		if ts == nil || ts.player == nil {
			return nil
		}
		ts.castMu.Lock()
		for _, aura := range ts.activeAuras {
			accept(aura)
		}
		ts.castMu.Unlock()
		return out
	}
	s.server.auraMu.Lock()
	for _, aura := range s.server.activeCreatureAuras[key] {
		accept(aura)
	}
	s.server.auraMu.Unlock()
	return out
}

// splitShareDamagePct mirrors Unit::DealDamage's SPELL_AURA_SHARE_DAMAGE_PCT arm
// (Unit.cpp:766-788): for each aura-300 on the victim covering the damage school,
// CalculatePct(damage, amount) is dealt to the aura's caster as NODAMAGE — no aura
// interrupts, no procs, no rage-dealt, no pushback, no threat from the split itself
// (threat still accrues on the shared hit like any DealDamage), and no recursive
// split: the C++ split leg is gated on damagetype != NODAMAGE.
func (s *session) splitShareDamagePct(ctx context.Context, victimGUID uint64, victimIsPlayer bool, key creatureAuraKey, attackerGUID uint64, damage uint32, schoolMask uint32) {
	if damage == 0 || s == nil || s.server == nil {
		return
	}
	for _, e := range s.collectShareDamageAuras(victimGUID, victimIsPlayer, key, schoolMask) {
		share := uint32(uint64(damage) * uint64(e.amount) / 100)
		if share == 0 {
			continue
		}
		if ts := s.server.findSessionByGUID(e.casterGUID); ts != nil && ts.player != nil {
			s.applySharedDamageToPlayer(ctx, e.casterGUID, attackerGUID, share)
			continue
		}
		ckey := creatureAuraKey{Map: key.Map, InstanceID: key.InstanceID, GUID: e.casterGUID}
		if victimIsPlayer && s.player != nil {
			ckey = creatureAuraKeyForPlayer(*s.player, e.casterGUID)
		}
		s.applySharedDamageToCreature(ctx, ckey, attackerGUID, share)
	}
}

// applySharedDamageToPlayer applies one NODAMAGE shared hit to a player
// (Unit::DealDamage with damagetype NODAMAGE, Unit.cpp:700-973): god-cheat,
// duel-defeat, achievement, kill, and the rage-received legs run; aura
// interrupts, procs, pushback, and the rage-dealt leg do not.
func (s *session) applySharedDamageToPlayer(ctx context.Context, victimGUID, attackerGUID uint64, share uint32) {
	if s == nil || s.server == nil || share == 0 {
		return
	}
	ts := s.server.findSessionByGUID(victimGUID)
	if ts == nil || ts.player == nil {
		return
	}
	share = ts.negateGodModeDamage(share)
	if share == 0 {
		return
	}
	health := ts.player.Health
	// Duel defeat (Unit.cpp:825-853, 957-973): the recursive NODAMAGE split is
	// a full DealDamage with the original attacker — the duel leg runs with
	// the attacker's GetControllingPlayer (Unit.cpp:5996). A lethal blow from
	// anyone but the duel opponent is not consumed: the kill path below
	// interrupts the duel (Unit::Kill, Unit.cpp:11363-11368).
	if duelDefeatOnDamage(ts, ts.controllingPlayerGUID(attackerGUID), share, health) {
		ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, health-1)
		return
	}
	if health > 0 && share+1 >= health {
		ts.player.Health = 0
		ts.sendPlayerUpdate()
		ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, health)
		var killer *session
		if as := s.server.findSessionByGUID(attackerGUID); as != nil {
			killer = as
		}
		ts.killPlayer(ctx, killer, true)
		return
	}
	ts.player.Health -= share
	ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, share)
	ts.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, share)
	if as := s.server.findSessionByGUID(attackerGUID); as != nil {
		as.updateAchievementCriteria(criteriaTypeDamageDone, 0, min(share, health))
		as.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, share)
	}
	// Rage from damage received runs on NODAMAGE hits (Unit.cpp:915-924 is not
	// damagetype-gated).
	ts.grantRageFromDamageTaken(ctx, share)
	// Unit::DealDamage (Unit.cpp:906-913, 925-931): the recursive NODAMAGE
	// split is a full DealDamage — HIT TAKEN on the player victim, HIT DONE
	// on the attacker when it is a player (findSessionByGUID only resolves
	// player sessions).
	ts.rollDurabilityLossOnHit(ctx, share)
	if as := s.server.findSessionByGUID(attackerGUID); as != nil {
		as.rollDurabilityLossOnHit(ctx, share)
	}
	ts.sendPlayerUpdate()
}

// applySharedDamageToCreature applies one NODAMAGE shared hit to a creature
// (e.g. boss Soul Link 38007 shared onto the inner demon): tap, threat, and
// the kill/health legs run; aura interrupts, procs, and the split do not.
// SetLastDamagedTime has no Go bridge (kill.go), matching the main paths.
func (s *session) applySharedDamageToCreature(ctx context.Context, key creatureAuraKey, attackerGUID uint64, share uint32) {
	if s == nil || s.server == nil || share == 0 {
		return
	}
	var groupID uint64
	if as := s.server.findSessionByGUID(attackerGUID); as != nil {
		groupID = as.groupID
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if motion == nil || motion.Health == 0 {
		s.server.motionMu.Unlock()
		return
	}
	// Unit::DealDamage tap block (Unit.cpp:872-876) is not damagetype-gated.
	s.server.recordCreatureTap(motion, attackerGUID, groupID, share, motion.Health)
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion.GUID)
	}
	motion.ThreatMgr.AddThreat(attackerGUID, float32(share), false)
	killed := share >= motion.Health
	if killed {
		motion.Health = 0
		motion.DynamicFlags |= unitDynFlagLootable
		motion.InCombat = false
		motion.TargetGUID = 0
		motion.Moving = false
		motion.ThreatMgr.ClearThreat()
	} else {
		motion.Health -= share
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	x, y, z := motion.X, motion.Y, motion.Z
	newHealth := motion.Health
	s.server.motionMu.Unlock()
	// Unit::DealDamage (Unit.cpp:925-931): the recursive NODAMAGE split is a
	// full DealDamage — HIT DONE rolls for the attacker when it is a player
	// (findSessionByGUID only resolves player sessions).
	if as := s.server.findSessionByGUID(attackerGUID); as != nil {
		as.rollDurabilityLossOnHit(ctx, share)
	}
	if killed {
		s.server.stopCreatureMotionInInstance(mapID, instanceID, guid, x, y, z)
		s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: unitDynFlagLootable,
		})
		s.server.broadcastThreatClearInInstance(mapID, instanceID, guid)
		s.onCreatureKilled(ctx, combatTarget{GUID: guid, Map: mapID, InstanceID: instanceID}, nil)
		return
	}
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{
		unitFieldHealth: newHealth,
	})
}
