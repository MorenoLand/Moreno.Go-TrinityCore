package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Unit::DealMeleeDamage damage-shield retaliation (Unit.cpp:1555-1600): after
// a melee hit deals damage, each SPELL_AURA_DAMAGE_SHIELD (15) effect on the
// victim strikes the attacker with its aura amount. C++ rolls a binary
// SpellHitResult per shield (miss -> SendSpellMiss), skips immuned attackers,
// scales the amount by the aura caster's SpellDamageBonusDone and the
// attacker's SpellDamageBonusTaken, runs DealDamageMods, broadcasts
// SMSG_SPELLDAMAGESHIELD, then deals the damage as SPELL_DIRECT_DAMAGE with
// the shield's SpellInfo — notably with NO CalcAbsorbResist ("No
// Unit::CalcAbsorbResist here - opcode doesn't send that data").

// damageShieldSide is one combatant of a shield retaliation: the shield
// holder (victim of the melee hit) or the original attacker.
type damageShieldSide struct {
	sess     *session
	key      creatureAuraKey
	isPlayer bool
}

func (d damageShieldSide) guid() uint64 {
	if d.isPlayer && d.sess != nil {
		return d.sess.playerGUID
	}
	return d.key.GUID
}

// damageShieldAura is one resolved SPELL_AURA_DAMAGE_SHIELD effect.
type damageShieldAura struct {
	spellID    uint32
	casterGUID uint64
	amount     int32
	effIndex   int
	schoolMask uint32
}

// collectDamageShieldAuras snapshots the shield holder's aura-15 effects
// (Unit.cpp:1556: GetAuraEffectsByType(SPELL_AURA_DAMAGE_SHIELD), copied
// before iteration so mid-loop removals can't invalidate the walk).
func (s *Server) collectDamageShieldAuras(victimSess *session, key creatureAuraKey, victimIsPlayer bool) []damageShieldAura {
	var out []damageShieldAura
	appendAura := func(aura *activeAura) {
		if aura == nil || aura.Stopped {
			return
		}
		schoolMask := aura.SchoolMask
		if s != nil && s.Data != nil {
			if spell, found, err := s.Data.Spell(aura.SpellID); err == nil && found && spell.SchoolMask != 0 {
				schoolMask = spell.SchoolMask
			}
		}
		for i := 0; i < 3; i++ {
			if aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			amount := aura.Amounts[i]
			if amount == 0 {
				amount = int32(aura.Amount)
			}
			if amount <= 0 {
				continue
			}
			out = append(out, damageShieldAura{
				spellID:    aura.SpellID,
				casterGUID: aura.CasterGUID,
				amount:     amount,
				effIndex:   i,
				schoolMask: schoolMask,
			})
		}
	}
	if victimIsPlayer {
		if victimSess == nil || victimSess.player == nil {
			return nil
		}
		victimSess.castMu.Lock()
		for _, aura := range victimSess.activeAuras {
			if aura != nil && !aura.Stopped && aura.AuraType == spellAuraDamageShield {
				appendAura(aura)
			}
		}
		victimSess.castMu.Unlock()
		return out
	}
	if s == nil || key.GUID == 0 {
		return nil
	}
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura != nil && !aura.Stopped && aura.AuraType == spellAuraDamageShield {
			appendAura(aura)
		}
	}
	s.auraMu.Unlock()
	return out
}

// applyDamageShieldRetaliation fires the holder's damage shields after a
// melee hit dealt damageDealt > 0 (Unit.cpp:1555: Damages[0]+Damages[1] > 0,
// post-absorb — a fully absorbed hit triggers nothing).
func (s *Server) applyDamageShieldRetaliation(ctx context.Context, victim, attacker damageShieldSide, damageDealt uint32) {
	if s == nil || damageDealt == 0 {
		return
	}
	var victimSess *session
	var victimKey creatureAuraKey
	if victim.isPlayer {
		victimSess = victim.sess
	} else {
		victimKey = victim.key
	}
	shields := s.collectDamageShieldAuras(victimSess, victimKey, victim.isPlayer)
	for _, sh := range shields {
		s.applyOneDamageShield(ctx, victim, attacker, sh)
	}
}

// attackerShieldTarget resolves the retaliation target's level, health and
// session/motion for one shield strike.
func (s *Server) attackerShieldTarget(attacker damageShieldSide) (level uint8, health uint32, attackerSess *session, motion *creatureMotion, ok bool) {
	if attacker.isPlayer {
		attackerSess = s.findSessionByGUID(attacker.guid())
		if attackerSess == nil || attackerSess.player == nil || attackerSess.player.Health == 0 {
			return 0, 0, nil, nil, false
		}
		return attackerSess.player.Level, attackerSess.player.Health, attackerSess, nil, true
	}
	motion = s.findCreatureMotion(attacker.key.Map, attacker.key.InstanceID, attacker.key.GUID)
	if motion == nil || motion.Health == 0 {
		return 0, 0, nil, nil, false
	}
	return uint8(motion.Level), motion.Health, nil, motion, true
}

// applyOneDamageShield resolves and deals a single shield strike
// (Unit.cpp:1564-1600).
func (s *Server) applyOneDamageShield(ctx context.Context, victim, attacker damageShieldSide, sh damageShieldAura) {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	level, health, attackerSess, motion, ok := s.attackerShieldTarget(attacker)
	if !ok {
		// Unit::DealDamageMods (Unit.cpp:686-694): a dead, in-flight or
		// evading attacker zeroes the damage. The evade arm has no Go
		// player-side model; the creature side is covered by the motion
		// lookup's evade handling at the call sites' damage gate.
		return
	}
	// Unit.cpp:1566-1571: the shield can be resisted — binary SpellHitResult
	// from the holder. Creature holders have no Go spell-hit model, so the
	// roll runs only for player holders (documented delta).
	if victim.isPlayer && victim.sess != nil {
		if !victim.sess.rollSpellHit(level, attacker.isPlayer) {
			victim.sess.sendSpellMiss(attacker.guid(), sh.spellID, protocol.SpellMissMiss)
			return
		}
	}
	// Unit.cpp:1573-1577: immunity check. Matches the direct-spell path's
	// creature arm (creatureImmuneToDamageSpell, Unit.cpp:7818-7860).
	if attacker.isPlayer {
		if attackerSess.isImmuneToDamage(sh.schoolMask) {
			return
		}
	} else if s != nil && s.Data != nil {
		if spell, found, err := s.Data.Spell(sh.spellID); err == nil && found {
			if creatureImmuneToDamageSpell(s, attacker.key, spell, sh.schoolMask) {
				return
			}
		}
	}
	// Unit.cpp:1579-1583: amount scaled by the AURA CASTER's
	// SpellDamageBonusDone (direct) and the attacker's SpellDamageBonusTaken.
	// spellDamageDoneFlat already folds the attacker's aura-14
	// MOD_DAMAGE_TAKEN into the advertised benefit (victimDamageTakenFlat),
	// matching Unit.cpp:6676-6677 where the victim's aura-14 rides the done
	// side. An offline/unknown caster deals the flat amount (C++: no caster
	// -> no bonus legs).
	damage := uint32(sh.amount)
	var dmgSpell wotlk.Spell
	spellKnown := false
	if s != nil && s.Data != nil {
		if spell, found, err := s.Data.Spell(sh.spellID); err == nil && found {
			dmgSpell, spellKnown = spell, true
		}
	}
	if spellKnown {
		if casterSess := s.findSessionByGUID(sh.casterGUID); casterSess != nil && casterSess.player != nil {
			var target combatTarget
			if at, ok := casterSess.getCombatTarget(ctx, attacker.guid()); ok {
				target = at
			}
			if bonus := casterSess.spellDamageDoneFlat(ctx, dmgSpell, attacker.guid(), target, sh.effIndex, false); bonus != 0 {
				if v := int32(damage) + bonus; v > 0 {
					damage = uint32(v)
				} else {
					damage = 0
				}
			}
		}
		damage = spellDamageBonusTaken(damage, dmgSpell, sh.schoolMask, attackerSess, s.findSessionByGUID(sh.casterGUID))
	}
	// Unit.cpp:1585-1595: SMSG_SPELLDAMAGESHIELD (holder, attacker, spell,
	// damage, overkill, school) to the holder's set — before DealDamage.
	overkill := uint32(0)
	if damage > health {
		overkill = damage - health
	}
	pkt := protocol.NewBuffer(32)
	pkt.WriteU64(victim.guid())
	pkt.WriteU64(attacker.guid())
	pkt.WriteU32(sh.spellID)
	pkt.WriteU32(damage)
	pkt.WriteU32(overkill)
	pkt.WriteU32(sh.schoolMask)
	if victim.isPlayer && victim.sess != nil {
		_ = victim.sess.write(uint16(protocol.OpcodeSMSG_SPELLDAMAGESHIELD), pkt.Bytes(), true)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLDAMAGESHIELD), pkt.Bytes(), victim.sess)
	} else if !victim.isPlayer {
		s.broadcastToInstance(victim.key.Map, victim.key.InstanceID, uint16(protocol.OpcodeSMSG_SPELLDAMAGESHIELD), pkt.Bytes(), nil)
	}
	// Unit.cpp:1597: no CalcAbsorbResist on shield damage — it bypasses
	// absorbs entirely, so neither applyAbsorptionShields arm runs here.
	if damage == 0 {
		return
	}
	if attacker.isPlayer {
		s.applyDamageShieldToPlayer(ctx, victim, attackerSess, sh, damage)
	} else {
		s.applyDamageShieldToCreature(ctx, victim, motion, attacker.key, sh, damage)
	}
}

// applyDamageShieldToPlayer deals one shield strike to a player attacker as
// SPELL_DIRECT_DAMAGE (Unit::DealDamage, Unit.cpp:696-973): god, the 300
// share-damage split, achievements, duel, kill, rage-received, durability,
// pushback and the proc passes all run; absorbs were already skipped by the
// caller.
func (s *Server) applyDamageShieldToPlayer(ctx context.Context, victim damageShieldSide, ts *session, sh damageShieldAura, damage uint32) {
	if s == nil || ts == nil || ts.player == nil || damage == 0 {
		return
	}
	holderGUID := victim.guid()
	var holderSess *session
	if victim.isPlayer {
		holderSess = victim.sess
	}
	damage = ts.negateGodModeDamage(damage)
	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT on the
	// shield damage itself — the C++ split leg is gated on damagetype !=
	// NODAMAGE, and the shield strike is SPELL_DIRECT_DAMAGE.
	if damage > 0 {
		ts.splitShareDamagePct(ctx, ts.playerGUID, true, creatureAuraKey{}, holderGUID, damage, sh.schoolMask)
	}
	ts.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
	ts.lastCombatTime = time.Now()
	if ts.player.UnitFlags&unitFlagInCombat == 0 {
		ts.player.UnitFlags |= unitFlagInCombat
	}
	if holderSess != nil && holderSess.player != nil {
		if holderSess.player.UnitFlags&unitFlagInCombat == 0 {
			holderSess.player.UnitFlags |= unitFlagInCombat
		}
		holderSess.lastCombatTime = time.Now()
		holderSess.updateAchievementCriteria(criteriaTypeDamageDone, 0, damage)
		holderSess.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)
	}
	health := ts.player.Health
	// Duel defeat (Unit.cpp:825-853, 957-973) with the shield holder as the
	// damage source (GetControllingPlayer, Unit.cpp:5996).
	holderPlayerGUID := holderGUID
	if ts != nil {
		holderPlayerGUID = ts.controllingPlayerGUID(holderGUID)
	}
	if duelDefeatOnDamage(ts, holderPlayerGUID, damage, health) {
		ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, health-1)
		return
	}
	if damage > 0 && health > 0 && damage+1 >= health {
		ts.player.Health = 0
		ts.sendPlayerUpdate()
		ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, health)
		ts.killPlayer(ctx, holderSess, true, true)
		return
	}
	if damage == 0 {
		return
	}
	ts.player.Health -= damage
	// Unit::DealDamage (Unit.cpp:891-894): TOTAL_DAMAGE_RECEIVED fires with
	// the dealt damage on a non-lethal hit.
	ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, damage)
	// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
	ts.grantRageFromDamageTaken(ctx, damage)
	// Unit::DealDamage (Unit.cpp:906-913, 925-931): random durability loss —
	// HIT TAKEN on the attacker, HIT DONE on the shield holder.
	ts.rollDurabilityLossOnHit(ctx, damage)
	if holderSess != nil {
		holderSess.rollDurabilityLossOnHit(ctx, damage)
	}
	if ts.spellDamagePushesBack(sh.spellID, ts.playerGUID) {
		ts.delayCurrentCast()
		ts.delayCurrentChannel()
	}
	// ProcDamageAndSpellFor (Unit.cpp:10179+): the taken pass on the attacker
	// and the done pass on the shield holder, carrying the shield's spell.
	ts.procSpellHitTakenAuraTriggers(ctx, holderGUID, sh.spellID, true, false, false, false, false, 0, damage, false, false)
	if holderSess != nil {
		holderSess.procSpellHitAuraTriggers(ctx, ts.playerGUID, sh.spellID, true, false, false, false, false, 0, damage, false)
	}
	ts.sendPlayerUpdate()
	// Unit::DealDamage (Unit.cpp:728-733): the attacker's controlled
	// creatures are signaled OwnerAttackedBy — the attacker is the DealDamage
	// victim here, so its pets defend against the shield holder.
	s.triggerPetDefensive(ts.player.Map, ts.player.InstanceID, ts.playerGUID, holderGUID)
}

// applyDamageShieldToCreature deals one shield strike to a creature attacker
// (Unit::DealDamage, Unit.cpp:696-973): the Eluna DamageTaken hook, the 300
// split, tap, rage, threat and the kill/health legs run. Two C++ deviations
// are load-bearing here: the evade leash's LastDamaged stamp EXCLUDES
// damage-shield-aura damage (Unit.cpp:902), and no threat arm fires when the
// shield holder is a player (the DealDamage victim — a player — owns no
// threat list); when the holder is a creature its manager gains threat on
// the original attacker per Unit.cpp:906.
func (s *Server) applyDamageShieldToCreature(ctx context.Context, victim damageShieldSide, motion *creatureMotion, key creatureAuraKey, sh damageShieldAura, damage uint32) {
	if s == nil || motion == nil || damage == 0 {
		return
	}
	holderGUID := victim.guid()
	var holderSess *session
	if victim.isPlayer {
		holderSess = victim.sess
	}
	// Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9): fires before damage apply
	// (Unit::DealDamage, Unit.cpp:697-702); handlers may rewrite damage.
	if holderSess != nil {
		damage = s.fireCreatureDamageTaken(ctx, motion, holderSess.luaPlayer(), damage)
	}
	if damage == 0 {
		return
	}
	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	// Creature holders have no session for the split funnel (documented
	// delta); player holders split through their own session.
	if holderSess != nil {
		holderSess.splitShareDamagePct(ctx, key.GUID, false, key, holderGUID, damage, sh.schoolMask)
	}
	var groupID uint64
	if holderSess != nil {
		groupID = holderSess.groupID
	}
	s.motionMu.Lock()
	m := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if m == nil || m.Health == 0 {
		s.motionMu.Unlock()
		return
	}
	// Unit::DealDamage tap block (Unit.cpp:872-876): not damagetype-gated.
	s.recordCreatureTap(m, holderGUID, groupID, damage, m.Health)
	wasInCombat := m.InCombat
	if damage >= m.Health {
		m.Health = 0
		m.DynamicFlags |= unitDynFlagLootable
		corpseFlags := m.DynamicFlags
		m.InCombat = false
		m.TargetGUID = 0
		m.Moving = false
		if m.ThreatMgr != nil {
			m.ThreatMgr.ClearThreat()
		}
		s.motionMu.Unlock()
		s.stopCreatureMotionInInstance(key.Map, key.InstanceID, key.GUID, m.X, m.Y, m.Z)
		s.broadcastCreatureValuesUpdateInInstance(key.Map, key.InstanceID, key.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: corpseFlags,
		})
		s.broadcastThreatClearInInstance(key.Map, key.InstanceID, key.GUID)
		if holderSess != nil {
			_ = holderSess.sendAttackStop(key.GUID, true)
			holderSess.attackTarget = 0
		}
		var ct combatTarget
		ct.GUID, ct.Map, ct.InstanceID, ct.X, ct.Y, ct.Z = m.GUID, m.Map, m.InstanceID, m.X, m.Y, m.Z
		ct.Health, ct.MaxHealth, ct.Level = 0, m.MaxHealth, uint8(m.Level)
		if holderSess != nil {
			holderSess.onCreatureKilled(ctx, ct, nil)
		}
		return
	}
	newHealth := m.Health - damage
	m.Health = newHealth
	// Unit::DealDamage (Unit.cpp:900-907): the damage-shield-aura exclusion —
	// shield retaliation never stamps the evade leash. (The direct-spell path
	// stamps unconditionally; this funnel is the anticipated non-vacuous
	// case of that comment.)
	// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
	_, _ = s.addCreatureRageLocked(m, m.Level, damage)
	if m.ThreatMgr == nil {
		m.ThreatMgr = NewThreatManager(m)
	}
	// Unit::DealDamage (Unit.cpp:906): the DealDamage victim (the original
	// attacker) gains threat on the damage source (the shield holder), via
	// ThreatManager::CalculateModifiedThreat (ThreatManager.cpp:606-659) —
	// the same damageThreatAmount fold the direct-spell path uses. The
	// step-1 gate (ThreatManager.cpp:311-317) suppresses both threat and
	// engagement for gated spells.
	if m.ThreatMgr.OwnerCanHaveThreatList() {
		threat := float32(damage)
		step1Gate := false
		if holderSess != nil {
			threat, step1Gate = holderSess.damageThreatAmount(ctx, sh.spellID, sh.schoolMask, float32(damage), wasInCombat)
		}
		if !step1Gate {
			m.InCombat = true
			// The shield only fires on melee hits, so the attacker is in
			// melee range by construction.
			switched, newVictim := m.ThreatMgr.AddThreat(holderGUID, threat, true)
			if switched && newVictim != m.TargetGUID {
				m.TargetGUID = newVictim
				entries := m.ThreatMgr.SortedEntries()
				s.broadcastHighestThreatUpdateInInstance(key.Map, key.InstanceID, key.GUID, newVictim, entries)
			}
		}
	}
	s.motionMu.Unlock()
	// Unit::DealDamage (Unit.cpp:925-931): random durability loss on HIT
	// DONE — the shield holder is the damage source.
	if holderSess != nil {
		holderSess.rollDurabilityLossOnHit(ctx, damage)
	}
	s.broadcastCreatureValuesUpdateInInstance(key.Map, key.InstanceID, key.GUID, map[int]uint32{
		unitFieldHealth: newHealth,
	})
}
