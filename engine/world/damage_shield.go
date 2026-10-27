package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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
			if bonus := casterSess.spellDamageDoneFlat(ctx, dmgSpell, attacker.guid(), target, sh.effIndex, false, 1); bonus != 0 {
				if v := int32(damage) + bonus; v > 0 {
					damage = uint32(v)
				} else {
					damage = 0
				}
			}
		}
		damage = spellDamageBonusTaken(damage, dmgSpell, sh.schoolMask, attackerSess, s.findSessionByGUID(sh.casterGUID), 1.0)
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
	// Unit.cpp:1595: victim->SendMessageToSet — the shield holder's
	// visibility set; the holder writes its own copy and a creature holder
	// has no session leg.
	if victim.isPlayer && victim.sess != nil && victim.sess.player != nil {
		_ = victim.sess.write(uint16(protocol.OpcodeSMSG_SPELLDAMAGESHIELD), pkt.Bytes(), true)
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLDAMAGESHIELD), pkt.Bytes(), victim.sess.player.Map, victim.sess.player.InstanceID, victim.sess.player.X, victim.sess.player.Y, victim.sess.player.Z, victim.sess)
	} else if !victim.isPlayer {
		if m := s.findCreatureMotion(victim.key.Map, victim.key.InstanceID, victim.key.GUID); m != nil {
			s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLDAMAGESHIELD), pkt.Bytes(), m.Map, m.InstanceID, m.X, m.Y, m.Z, nil)
		}
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
	s.dealDirectDamageToPlayer(ctx, holderGUID, holderSess, ts, sh.spellID, sh.schoolMask, damage)
}

// dealDirectDamageToPlayer applies one recursive SPELL_DIRECT_DAMAGE strike
// to a player (Unit::DealDamage, Unit.cpp:696-973): god, the split-damage
// (81/153) and share-damage (300) legs, achievements, duel, kill,
// rage-received, durability, pushback, the proc passes and pet-defensive all
// run. Shared by damage-shield retaliation and split-damage redirection;
// attackerGUID/attackerSess identify the original attacker (a nil session for
// creatures).
func (s *Server) dealDirectDamageToPlayer(ctx context.Context, attackerGUID uint64, attackerSess *session, ts *session, spellID uint32, schoolMask uint32, damage uint32) {
	if s == nil || ts == nil || ts.player == nil || damage == 0 {
		return
	}
	damage = ts.negateGodModeDamage(damage)
	// Unit::DealDamage (Unit.cpp:738-750): the TAKE_DAMAGE strip precedes the
	// !damage early-return and the split-damage legs (:766-788).
	ts.stripRecursiveTakeDamageAuras(spellID)
	// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras on the
	// recursive victim redirect part of the damage to their caster, ahead of
	// the 300 share leg below (Unit::DealDamage, Unit.cpp:766-788).
	if damage > 0 {
		damage = s.splitDamageToCaster(ctx, ts.playerGUID, true, creatureAuraKey{}, attackerGUID, damage, schoolMask)
	}
	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	if damage > 0 {
		ts.splitShareDamagePct(ctx, ts.playerGUID, true, creatureAuraKey{}, attackerGUID, damage, schoolMask)
	}
	ts.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
	ts.lastCombatTime = time.Now()
	if ts.player.UnitFlags&unitFlagInCombat == 0 {
		ts.player.UnitFlags |= unitFlagInCombat
	}
	if attackerSess != nil && attackerSess.player != nil {
		if attackerSess.player.UnitFlags&unitFlagInCombat == 0 {
			attackerSess.player.UnitFlags |= unitFlagInCombat
		}
		attackerSess.lastCombatTime = time.Now()
		attackerSess.updateAchievementCriteria(criteriaTypeDamageDone, 0, damage)
		attackerSess.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)
	}
	health := ts.player.Health
	// Duel defeat (Unit.cpp:825-853, 957-973) with the original attacker as
	// the damage source (GetControllingPlayer, Unit.cpp:5996).
	attackerPlayerGUID := attackerGUID
	if ts != nil {
		attackerPlayerGUID = ts.controllingPlayerGUID(attackerGUID)
	}
	if duelDefeatOnDamage(ts, attackerPlayerGUID, damage, health) {
		ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, health-1)
		return
	}
	if damage > 0 && health > 0 && damage+1 >= health {
		ts.player.Health = 0
		ts.sendPlayerUpdate()
		ts.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, health)
		ts.killPlayer(ctx, attackerSess, true, true)
		// Eluna kill hooks: Unit::Kill tail (Unit.cpp:11466-11480) — the
		// shield holder is the attacker (Unit::DealDamage(victim, this, ...),
		// Unit.cpp:1605).
		if attackerSess != nil {
			// Holder is a player: PLAYER_EVENT_ON_KILL_PLAYER (6).
			s.firePVPKillHook(ctx, attackerSess, ts)
		} else if motion := s.findCreatureMotion(ts.player.Map, ts.player.InstanceID, attackerGUID); motion != nil {
			// Holder is a creature: PLAYER_EVENT_ON_KILLED_BY_CREATURE (8).
			s.fireKilledByCreatureHook(ctx, motion, ts)
		}
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
	// HIT TAKEN on the victim, HIT DONE on the original attacker.
	ts.rollDurabilityLossOnHit(ctx, damage)
	if attackerSess != nil {
		attackerSess.rollDurabilityLossOnHit(ctx, damage)
	}
	if ts.spellDamagePushesBack(spellID, ts.playerGUID) {
		ts.delayCurrentCast()
		ts.delayCurrentChannel()
	}
	// Unit::DealDamage (Unit.cpp:897-898): the DIRECT_DAMAGE strip runs after
	// the health leg; the breakable-CC drain rides the taken proc pass.
	ts.stripRecursiveDirectDamageAuras(spellID, damage)
	// ProcDamageAndSpellFor (Unit.cpp:10179+): the taken pass on the victim
	// and the done pass on the original attacker, carrying the aura's spell.
	ts.procSpellHitTakenAuraTriggers(ctx, attackerGUID, spellID, true, false, false, false, false, 0, damage, false, false, 0, false, false)
	if attackerSess != nil {
		attackerSess.procSpellHitAuraTriggers(ctx, ts.playerGUID, spellID, true, false, false, false, false, 0, damage, false, false, false)
	}
	ts.sendPlayerUpdate()
	// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled creatures
	// are signaled OwnerAttackedBy against the original attacker.
	s.triggerPetDefensive(ctx, ts.player.Map, ts.player.InstanceID, ts.playerGUID, attackerGUID, scripting.CreatureEventOnOwnerAttackedAt)
}

// applyDamageShieldToCreature deals one shield strike to a creature attacker
// (Unit::DealDamage, Unit.cpp:696-973): the Eluna DamageTaken hook, the 300
// split, tap, rage, threat and the kill/health legs run. The evade leash's
// LastDamaged stamp EXCLUDES damage-shield-aura damage (Unit.cpp:902), and no
// threat arm fires when the shield holder is a player (the DealDamage victim
// — a player — owns no threat list); when the holder is a creature its
// manager gains threat on the original attacker per Unit.cpp:906.
func (s *Server) applyDamageShieldToCreature(ctx context.Context, victim damageShieldSide, motion *creatureMotion, key creatureAuraKey, sh damageShieldAura, damage uint32) {
	if s == nil || motion == nil || damage == 0 {
		return
	}
	holderGUID := victim.guid()
	var holderSess *session
	if victim.isPlayer {
		holderSess = victim.sess
	}
	s.dealDirectDamageToCreature(ctx, holderGUID, holderSess, motion, key, sh.spellID, sh.schoolMask, false, damage)
}

// dealDirectDamageToCreature applies one recursive SPELL_DIRECT_DAMAGE strike
// to a creature (Unit::DealDamage, Unit.cpp:696-973): the Eluna DamageTaken
// hook, the split-damage (81/153) and share-damage (300) legs, tap, rage,
// threat and the kill/health legs run. Shared by damage-shield retaliation
// and split-damage redirection; attackerGUID/attackerSess identify the
// original attacker (a nil session for creatures). stampLastDamaged carries
// the Unit.cpp:900-907 evade-leash arm — false for shield strikes (the
// damage-shield-aura exclusion), true for split strikes.
func (s *Server) dealDirectDamageToCreature(ctx context.Context, attackerGUID uint64, attackerSess *session, motion *creatureMotion, key creatureAuraKey, spellID uint32, schoolMask uint32, stampLastDamaged bool, damage uint32) {
	if s == nil || motion == nil || damage == 0 {
		return
	}
	// Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9): fires before damage apply
	// (Unit::DealDamage, Unit.cpp:697-702); handlers may rewrite damage.
	if attackerSess != nil {
		damage = s.fireCreatureDamageTaken(ctx, motion, attackerSess.luaPlayer(), damage)
	}
	if damage == 0 {
		return
	}
	// Unit::DealDamage (Unit.cpp:738-750): the TAKE_DAMAGE strip precedes the
	// split-damage legs (:766-788); the Eluna DamageTaken hook above fires
	// ahead of it (:697-702).
	s.stripCreatureRecursiveTakeDamageAuras(motion, spellID)
	// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras on the
	// recursive victim redirect part of the damage to their caster, ahead of
	// the 300 share leg below (Unit::DealDamage, Unit.cpp:766-788).
	damage = s.splitDamageToCaster(ctx, key.GUID, false, key, attackerGUID, damage, schoolMask)
	if damage == 0 {
		return
	}
	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	// Creature attackers have no session for the split funnel (documented
	// delta); player attackers split through their own session.
	if attackerSess != nil {
		attackerSess.splitShareDamagePct(ctx, key.GUID, false, key, attackerGUID, damage, schoolMask)
	}
	s.motionMu.Lock()
	m := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if m == nil || m.Health == 0 {
		s.motionMu.Unlock()
		return
	}
	if m.BossAI == nil {
		m.BossAI = getBossAIForCreature(m, m.ScriptName)
	}
	// Unit::DealDamage (Unit.cpp:699-704): the AI hooks fire at the top of
	// DealDamage — the damage-shield/split recursive strikes route through
	// it (Unit.cpp:1608) — ahead of the tap/lethal legs. The attacker leg
	// resolves the original attacker's creature motion when it is a
	// creature; player attackers carry no AI, as on the direct paths.
	var attackerMotion *creatureMotion
	if am := s.findCreatureMotionLocked(key.Map, key.InstanceID, attackerGUID); am != nil && am.Health > 0 {
		attackerMotion = am
	}
	fireDealDamageAIHooks(ctx, s, m, attackerMotion, attackerGUID, m.GUID, &damage)
	// Unit::DealDamage tap block (Unit.cpp:872-876): not damagetype-gated.
	// The tap attribution resolves the C++ SetLootRecipient /
	// LowerPlayerDamageReq gates (Creature.cpp:1342-1347, Unit.cpp:873-876):
	// a wild-creature attacker taps nothing and leaves the damage
	// requirement standing; a pet/player-controlled attacker taps for its
	// controlling player.
	tapPlayer, tapGroup, counts := s.tapAttackerAttribution(attackerGUID, attackerSess, attackerMotion)
	s.recordCreatureTap(m, tapPlayer, tapGroup, damage, m.Health, counts)
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
		if attackerSess != nil {
			_ = attackerSess.sendAttackStop(key.GUID, true)
			attackerSess.attackTarget = 0
		}
		var ct combatTarget
		ct.GUID, ct.Map, ct.InstanceID, ct.X, ct.Y, ct.Z = m.GUID, m.Map, m.InstanceID, m.X, m.Y, m.Z
		ct.Health, ct.MaxHealth, ct.Level = 0, m.MaxHealth, uint8(m.Level)
		if attackerSess != nil {
			attackerSess.onCreatureKilled(ctx, ct, nil)
			// Eluna PLAYER_EVENT_ON_KILL_CREATURE (7): Unit::Kill tail
			// (Unit.cpp:11466-11480) — the shield holder (a player here)
			// is the attacker.
			if motion := s.findCreatureMotion(ct.Map, ct.InstanceID, ct.GUID); motion != nil {
				s.fireCreatureKillHook(ctx, attackerSess, motion)
			}
		}
		// Unit::Kill death chain for a creature attacker (Unit.cpp:11320
		// setDeathState JUST_DIED, :11385-11391 KilledUnit/JustDied):
		// the session arm above runs the funnel instead, so these legs
		// are gated on attackerSess == nil like the stamp below, and
		// the respawn legs mirror the boss_ai no-owner path.
		if attackerSess == nil {
			// Unit::Kill's KILL/KILLED/DEATH proc legs (Unit.cpp:11257-11279)
			// fire before the strip, while the victim's auras are still
			// present — the boss_ai no-owner precedent: KILL on the
			// attacker's auras (no bearer when the attacker motion is
			// gone), KILLED on the victim's, DEATH on the victim's;
			// the KILL/KILLED legs gate on the victim not being a
			// critter, DEATH is unconditional. The pet/totem-owner
			// arm (Unit.cpp:11257-11263) stays vacuous here, as on
			// the insta path.
			if m.CreatureType != creatureTypeCritter {
				if attackerMotion != nil {
					s.fireCreatureMeleeAuraProcs(ctx, attackerMotion, m.GUID, procEventInfo{
						typeMask:       procFlagKill,
						spellTypeMask:  procSpellTypeMaskAll,
						spellPhaseMask: procSpellPhaseNone,
						hitMask:        procHitNone,
						actorGUID:      attackerMotion.GUID,
					})
				}
				s.fireCreatureMeleeAuraProcs(ctx, m, attackerGUID, procEventInfo{
					typeMask:       procFlagKilled,
					spellTypeMask:  procSpellTypeMaskAll,
					spellPhaseMask: procSpellPhaseNone,
					hitMask:        procHitNone,
					actorGUID:      m.GUID,
				})
			}
			s.fireCreatureMeleeAuraProcs(ctx, m, m.GUID, procEventInfo{
				typeMask:       procFlagDeath,
				spellTypeMask:  procSpellTypeMaskAll,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        procHitNone,
				actorGUID:      m.GUID,
			})
			s.auraMu.Lock()
			var deathAuras []*activeAura
			for _, aura := range s.activeCreatureAuras[key] {
				deathAuras = append(deathAuras, aura)
			}
			s.auraMu.Unlock()
			s.grantChannelDeathItems(ctx, deathAuras, m.Level, true, m.TapPlayerGUID, m.TapGroupID)
			s.clearCreatureAurasOnDeath(key)
			s.removeThreatVictimFromAllLists(key.Map, key.InstanceID, key.GUID)
			// Unit::setDeathState (Unit.cpp:8901-8911) clears — the funnel leg,
			// fired here because the attackerSess == nil arm skips onCreatureKilled.
			s.clearCreatureDeathState(m)
			if attackerMotion != nil {
				if attackerMotion.OwnerGUID != 0 {
					s.fireCreatureTargetDied(ctx, attackerMotion, s.luaMotionCreature(m))
				}
				s.fireCreatureTargetDied(ctx, attackerMotion, s.luaMotionCreature(m))
			}
			s.fireCreatureDiedByCreature(ctx, m, attackerMotion)
			if m.BossAI != nil {
				m.BossAI.OnDied(ctx, s, m)
			}
			// CombatAI::JustDied (CombatAI.cpp:68-73): the AICOND_DIE death
			// cast at the attacker killer — the funnel leg, fired here
			// because the attackerSess == nil arm skips onCreatureKilled.
			// A gone attacker motion leaves killerGUID 0, matching the
			// event-4 twin's nil killer on this path.
			killerGUID := uint64(0)
			if attackerMotion != nil {
				killerGUID = attackerMotion.GUID
			}
			s.fireCreatureDeathCast(ctx, m, killerGUID)
			// GuardAI::JustDied (GuardAI.cpp:72-76): the zone-under-attack
			// leg, resolved through the attacker motion's charmer/owner
			// player — a gone attacker motion broadcasts nothing.
			s.fireGuardDeathBroadcast(m, attackerMotion)
			// Unit::Kill loot leg (Unit.cpp:11169-11267): kill-time loot state
			// for this no-funnel chain - see recordCreatureKillLoot.
			s.recordCreatureKillLoot(m, attackerMotion)
			// Unit::Kill reward leg (Unit.cpp:11199-11267) — the funnel leg,
			// fired here because the attackerSess == nil arm skips onCreatureKilled.
			s.grantCreatureKillReward(ctx, m, attackerMotion)
			// Unit::Kill dungeon instance-binding block (Unit.cpp:11404-11425) — the funnel leg,
			// fired here because this path skips onCreatureKilled.
			s.bindInstanceOnNoFunnelKill(ctx, m, attackerMotion)
			respawnHealth := m.MaxHealth
			if respawnHealth == 0 {
				respawnHealth = 1
			}
			if key.InstanceID == 0 {
				s.scheduleCreatureRespawn(ctx, uint32(key.GUID&0x00FFFFFF), respawnHealth, time.Now())
			} else {
				s.scheduleInstanceCreatureRespawn(ctx, ct, respawnHealth, time.Now())
			}
		}
		// TempSummon corpse/dead legs: a creature attacker/holder runs
		// no session funnel, so stamp here; the session arm above
		// already stamps via the onCreatureKilled tail.
		if attackerSess == nil {
			s.checkDeadTempSummon(ctx, m, time.Now())
		}
		return
	}
	newHealth := m.Health - damage
	m.Health = newHealth
	// Unit::DealDamage (Unit.cpp:900-907): the evade leash's last-damaged
	// stamp — direct-damage strikes reset it; DoTs and damage-shield-aura
	// strikes never do.
	if stampLastDamaged && damage > 0 && m.OwnerGUID == 0 {
		m.LastDamaged = time.Now()
	}
	// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
	_, _ = s.addCreatureRageLocked(m, m.Level, damage)
	if m.ThreatMgr == nil {
		m.ThreatMgr = NewThreatManager(m)
	}
	// Unit::DealDamage (Unit.cpp:906): the DealDamage victim (the original
	// attacker) gains threat on the damage source, via
	// ThreatManager::CalculateModifiedThreat (ThreatManager.cpp:606-659) —
	// the same damageThreatAmount fold the direct-spell path uses. The
	// step-1 gate (ThreatManager.cpp:311-317) suppresses both threat and
	// engagement for gated spells.
	if m.ThreatMgr.OwnerCanHaveThreatList() {
		threat := float32(damage)
		step1Gate := false
		if attackerSess != nil {
			threat, step1Gate = attackerSess.damageThreatAmount(ctx, spellID, schoolMask, float32(damage), wasInCombat)
		}
		if !step1Gate {
			m.InCombat = true
			switched, newVictim := m.ThreatMgr.AddThreat(attackerGUID, threat, true)
			if switched && newVictim != m.TargetGUID {
				m.TargetGUID = newVictim
				entries := m.ThreatMgr.SortedEntries()
				s.broadcastHighestThreatUpdateInInstance(key.Map, key.InstanceID, key.GUID, newVictim, entries)
			}
		}
	}
	s.motionMu.Unlock()
	// Unit::DealDamage (Unit.cpp:897-898): the DIRECT_DAMAGE strip runs after
	// the health leg; the breakable-CC drain rides the taken proc pass.
	s.stripCreatureRecursiveDirectDamageAuras(key, spellID, damage, m.MaxHealth)
	// Unit::DealDamage (Unit.cpp:925-931): random durability loss on HIT
	// DONE — the original attacker is the damage source.
	if attackerSess != nil {
		attackerSess.rollDurabilityLossOnHit(ctx, damage)
	}
	s.broadcastCreatureValuesUpdateInInstance(key.Map, key.InstanceID, key.GUID, map[int]uint32{
		unitFieldHealth: newHealth,
	})
}
