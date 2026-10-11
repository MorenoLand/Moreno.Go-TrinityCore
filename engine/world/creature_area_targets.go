package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// creatureAIAreaEnemyPlayers mirrors Spell::SelectImplicitAreaTargets
// (Spell.cpp:1227) for the inline AI victim-spell path. The inline path is a
// shortcut that never runs SelectSpellTargets, so a creature-cast AoE applied
// its damage only to the wire victim; in C++ the creature's cast runs the
// full selection and every enemy unit in the area becomes a hit target.
// Returns the extra player hits (the wire victim is excluded) as positions
// with sessions attached, in an unspecified order.
func (s *Server) creatureAIAreaEnemyPlayers(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, victim *playerPos) []*playerPos {
	if s == nil || motion == nil || victim == nil || !isAreaEnemySpell(spell) {
		return nil
	}
	radius := float32(0)
	cone := false
	destinationCenter := false
	// WorldObjectSpellConeTargetCheck::operator() (Spell.cpp:8432): the cone
	// direction is C++-custom-attr driven — CU_CONE_BACK (set in
	// SpellMgr.cpp:2840 when SpellVisual[0] == 3879) keeps targets in the back
	// cone; otherwise the front cone at the hardcoded M_PI/2 (same read as
	// the player-path area sweep).
	backCone := len(spell.SpellVisual) > 0 && spell.SpellVisual[0] == 3879
	// Spell.cpp:1249-1251: when an effect has no radius entry (CalcRadius ==
	// 0), the area radius falls back to the spell's max range (hostile/
	// friendly per IsPositiveEffect — spellDBCMaxRange mirrors the same
	// split per isHarmfulSpell).
	var maxRange float32
	if s.Data != nil {
		if rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex); err == nil && ok {
			maxRange = float32(spellDBCMaxRange(spell, rangeEntry))
		}
	}
	for _, eff := range spell.Effects {
		areaTargetA := isAreaEnemyTargetType(eff.ImplicitTargetA) || (eff.Effect == 27 && eff.ImplicitTargetA == 18)
		areaTargetB := isAreaEnemyTargetType(eff.ImplicitTargetB) || (eff.Effect == 27 && eff.ImplicitTargetB == 18)
		if eff.Effect == 0 || (!areaTargetA && !areaTargetB) {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == 24 || targetType == 54 || targetType == 104 {
				cone = true
			}
			if targetType == 16 || targetType == 18 || targetType == 28 {
				destinationCenter = true
			}
		}
		// SpellEffectInfo::CalcRadius(m_caster): per-level radius from the
		// DBC entry; the inline path has no client-side radius override.
		if s.Data != nil {
			if value, ok, err := s.Data.SpellRadius(eff.RadiusIndex, uint32(motion.Level)); err == nil && ok && value > 0 {
				if value > radius {
					radius = value
				}
			} else if maxRange > radius {
				radius = maxRange
			}
		}
	}
	if radius <= 0 {
		return nil
	}
	// SelectImplicitAreaTargets (Spell.cpp:1227-1270): CASTER/SRC reference
	// types center on the caster; DEST reference types center on the
	// destination. The inline path never resolves a dest packet field, so a
	// DEST-ref area falls back to the wire target's position — the same
	// fallback the dest-family twin applies for target-dest on the creature
	// path.
	centerX, centerY, centerZ := motion.X, motion.Y, motion.Z
	if destinationCenter {
		centerX, centerY, centerZ = victim.X, victim.Y, victim.Z
	}
	seen := map[uint64]struct{}{victim.GUID: {}}
	var hits []*playerPos
	s.sessionsMu.RLock()
	for targetSession := range s.sessions {
		if targetSession == nil || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil || targetSession.player.Health == 0 {
			continue
		}
		if targetSession.player.Map != motion.Map || targetSession.player.InstanceID != motion.InstanceID {
			continue
		}
		if _, dup := seen[targetSession.playerGUID]; dup {
			continue
		}
		// The player-path area sweep applies no unit-flag block to player
		// candidates (the C++ totem/unattackable arms can't match a player
		// session); the faction gate below carries the enemy check.
		candidate := playerPos{Map: targetSession.player.Map, InstanceID: targetSession.player.InstanceID, X: targetSession.player.X, Y: targetSession.player.Y, Z: targetSession.player.Z, GUID: targetSession.playerGUID, Race: targetSession.player.Race, Class: targetSession.player.Class, Level: targetSession.player.Level, FactionTemplate: s.raceFaction(targetSession.player.Race), Reputations: playerReputationMap(targetSession.player.Reputations), Sess: targetSession}
		// WorldObjectSpellTargetCheck::operator() (Spell.cpp:8316):
		// TARGET_CHECK_ENEMY — the same call shape as the creature's own
		// aggro pick (creaturemotion.go canCreatureStartAttack).
		if !s.isAttackableFaction(motion.Faction, candidate) {
			continue
		}
		// WorldObjectSpellAreaTargetCheck::operator() (Spell.cpp:8409): the
		// area is a cylinder — 2D distance within the radius AND |dz| within
		// the radius — not a 3D sphere.
		dx, dy := float64(candidate.X-centerX), float64(candidate.Y-centerY)
		if dx*dx+dy*dy > float64(radius*radius) || math.Abs(float64(candidate.Z-centerZ)) > float64(radius) {
			continue
		}
		if cone {
			if backCone {
				// WorldObject::isInBack (Object.cpp:1417): !HasInArc(2*pi - arc, target).
				if hasInArc(motion.Orientation, motion.X, motion.Y, candidate.X, candidate.Y, 2*math.Pi-math.Pi/2) {
					continue
				}
			} else if !hasInArc(motion.Orientation, motion.X, motion.Y, candidate.X, candidate.Y, math.Pi/2) {
				continue
			}
		}
		seen[targetSession.playerGUID] = struct{}{}
		cp := candidate
		hits = append(hits, &cp)
	}
	s.sessionsMu.RUnlock()
	// Spell.cpp:1293: MaxAffectedTargets caps the area list, then
	// Trinity::Containers::RandomResize keeps that many chosen uniformly at
	// random (shuffle + truncate draws the same uniform subset). The motion
	// has no aura model, so no SPELL_AURA_MOD_MAX_AFFECTED_TARGETS term
	// applies.
	if maxTargets := spell.MaxTargets; maxTargets > 0 && uint32(len(hits)) > maxTargets {
		rand.Shuffle(len(hits), func(a, b int) { hits[a], hits[b] = hits[b], hits[a] })
		hits = hits[:maxTargets]
	}
	return hits
}

// applyInlineCreatureSpellHit runs the per-hit legs of the inline AI
// victim-spell path for one player hit: the AutoBalance damage scaling,
// Unit::DealDamage (Unit.cpp:735-973), the SMSG_SPELLNONMELEEDAMAGELOG
// (Spell.cpp:2542), the victim-side proc half (Spell.cpp:2581-2586), the
// stand-up arm (Spell.cpp:2609-2617), the caster-side SpellHitTarget Eluna
// hook (Spell.cpp:2639-2650), the per-hit trigger consumer
// (Spell.cpp:2652/2913-2957) and the spell_linked type-0 tail
// (Spell.cpp:3503-3512). In C++ these run inside DoTargetSpellHit for every
// target SelectImplicitAreaTargets (Spell.cpp:1227) selected — not just the
// wire victim — which is why this helper exists separately from the cast.
func (s *Server) applyInlineCreatureSpellHit(ctx context.Context, motion *creatureMotion, p *playerPos, spellID, damage uint32, schoolMask uint8, now time.Time) {
	// AutoBalance_UnitScript::ModifySpellDamageTaken analog.
	damage = s.autoBalanceModifyDealDamage(motion, p.Sess, damage)
	overkill := uint32(0)
	if p.Sess != nil && p.Sess.player != nil {
		// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD victims take no
		// damage — the kill/health legs are skipped explicitly (not via a
		// damage==0 gate) because absorbed-to-zero damage still runs them
		// in C++. The damage log below keeps the original amount, matching
		// C++ sending SMSG_SPELLNONMELEEDAMAGELOG before DealDamage
		// (Spell.cpp:2542).
		godNegated := p.Sess.godCheatActive()
		// Unit::DealDamage (Unit.cpp:766-788):
		// SPELL_AURA_SHARE_DAMAGE_PCT copies CalculatePct(damage,
		// amount) to the aura's caster.
		if !godNegated && damage > 0 {
			p.Sess.splitShareDamagePct(ctx, p.GUID, true, creatureAuraKey{}, motion.GUID, damage, uint32(schoolMask))
		}
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED
		// fires for any damage to a player victim, creature-caster
		// spells included.
		p.Sess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		victimHealth := p.Sess.player.Health
		// Duel defeat (Unit.cpp:825-853, 957-973): a charmed creature's
		// spell lands as a duel defeat when its charmer is the duel
		// opponent (GetControllingPlayer, Unit.cpp:5996); a wild
		// creature's exactly-health-1 hit still completes the duel as
		// won, while its lethal hit kills and interrupts.
		if !godNegated && duelDefeatOnDamage(p.Sess, p.Sess.controllingPlayerGUID(motion.GUID), damage, victimHealth) {
			// Duel defeat consumed the hit — loser at 1 HP, duel complete.
		} else if !godNegated && damage >= victimHealth {
			overkill = damage - victimHealth
			p.Sess.player.Health = 0
			p.IsDead = true
			p.Sess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
			// GetCharmerOrOwnerPlayerOrPlayerItself (Unit.cpp:11279):
			// a player-owned pet's kill credits its owner; a wild
			// creature resolves to no player.
			var creatureKiller *session
			if motion.OwnerGUID != 0 {
				creatureKiller = s.findSessionByGUID(motion.OwnerGUID)
			}
			p.Sess.killPlayer(ctx, creatureKiller, false, true)
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill
			// player-victim branch — attacker is a wild creature (no
			// owner player, so no pet arm):
			// attacker->AI()->KilledUnit(victim) (Unit.cpp:11359-11361).
			s.fireCreatureTargetDied(ctx, motion, p.Sess.luaPlayer())
			if motion.BossAI != nil {
				motion.BossAI.OnKillPlayer(ctx, s, motion, p.GUID)
			}
			// Unit::Kill (Unit.cpp:11283-11290): the PROC_FLAG_KILL leg
			// fires for the killing-blow unit regardless of damage
			// source — the melee path bridges it at its own kill site,
			// and this spell-hit site was missing it; the KILLED/DEATH
			// legs already ran inside killPlayer (death.go).
			s.fireCreatureMeleeAuraProcs(ctx, motion, p.GUID, procEventInfo{
				typeMask:       procFlagKill,
				spellTypeMask:  procSpellTypeMaskAll,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        procHitNone,
				actorGUID:      motion.GUID,
			})
		} else if !godNegated {
			p.Sess.player.Health -= damage
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
			p.Sess.grantRageFromDamageTaken(ctx, damage)
			// Unit::DealDamage (Unit.cpp:906-913): random durability
			// loss on HIT TAKEN — the victim is a player.
			p.Sess.rollDurabilityLossOnHit(ctx, damage)
			// Unit::DealDamage (Unit.cpp:936-937): a damage spell carrying
			// SPELL_ATTR7_NO_PUSHBACK_ON_DAMAGE or
			// SPELL_ATTR3_TREAT_AS_PERIODIC never delays the victim's
			// cast or channel — the !spellProto arm pushes back when the
			// DBC row is unavailable, and the pushback leg requires
			// non-zero damage. The victim != attacker arm is vacuous
			// here (creature caster, player victim).
			pushesBack := true
			if s != nil && s.Data != nil {
				if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
					pushesBack = spellInfo.AttributesEx3&spellAttr3TreatAsPeriodic == 0 &&
						spellInfo.AttributesEx7&spellAttr7NoPushbackOnDamage == 0
				}
			}
			if damage > 0 && pushesBack {
				// Reference Unit::DealDamage -> Spell::Delayed / DelayedChannel
				p.Sess.delayCurrentCast()
				p.Sess.delayCurrentChannel()
			}
			p.Sess.procDamageAuras(true)
		}
		logPkt := buildSpellNonMeleeDamageLog(p.GUID, motion.GUID, spellID, damage, overkill, schoolMask)
		_ = p.Sess.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
		if s != nil {
			s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, p.Sess)
		}
		p.Sess.lastCombatTime = now
		if p.Sess.player.UnitFlags&unitFlagInCombat == 0 {
			p.Sess.player.UnitFlags |= unitFlagInCombat
		}
		p.Sess.sendPlayerUpdate()
		// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
		// creatures are signaled OwnerAttackedBy on any non-DoT damage.
		if s != nil {
			s.triggerPetDefensive(p.Map, p.InstanceID, p.GUID, motion.GUID)
		}
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586):
	// Unit::ProcSkillsAndAuras (Unit.cpp:5355-5366) fires the victim-
	// side half with the taken pre-fill plus PROC_FLAG_TAKEN_DAMAGE
	// ORed by the damage arm (Spell.cpp:2545) — the player victim's
	// own auras evaluate the hit as a taken-proc event
	// (PROC_SPELL_TYPE_DAMAGE, PROC_SPELL_PHASE_HIT; Unit.cpp:
	// 10385-10418). The canEffectTrigger gate (Spell.cpp:2440-2442):
	// SPELL_ATTR3_CANT_TRIGGER_PROC never reaches the loop (checked
	// inside procSpellHitTakenAuraTriggers), the m_procDeep recursion
	// guard is the victim session's procDeep (Unit::CanProc,
	// Unit.h:1609), and the CanExecuteTriggersOnHit(EffectMask) arm
	// folds into the non-zero-effect model — this path applies every
	// non-zero effect to every hit p. Runs after the damage
	// legs and before the stand-up arm, matching C++ order (2581
	// precedes 2609), and fires even under the god cheat — C++'s
	// CHEAT_GOD skip lives inside DealDamage, not the proc call.
	// The creature actor has no auras, so only the taken leg runs.
	if p.Sess != nil && p.Sess.player != nil && p.Sess.procDeep == 0 {
		p.Sess.procSpellHitTakenAuraTriggers(ctx, motion.GUID, spellID, true, false, false, false, false, 0, damage, false, false)
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617): a
	// non-friendly non-positive (or dispel-effect) spell hit stands
	// the target up. The EVADE-miss arm is vacuous (no miss model on
	// this path) and the friendly gate rides the path — the tick only
	// casts hostile AI spells at its attack victim. The AtTargetAttacked
	// half is covered by the combat-state writes in the damage block
	// above (lastCombatTime / UNIT_FLAG_IN_COMBAT); its threat, loot-
	// recipient and PvP legs need a player threat manager, a player
	// caster, or a player owner — none of which a wild creature caster
	// has — documented no-bridge.
	//
	// Spell::TargetInfo::DoDamageAndTriggers AI legs (Spell.cpp:2639-
	// 2650): the caster-side SpellHitTarget Eluna hook — the same arm
	// bridged in castCreatureSpellInternal (boss_ai.go); this inline
	// AI victim-spell path is a second hit site for it. Both arms run
	// after the damage legs above, matching C++ order (damage is dealt
	// before 2609/2639), and both gate on a DBC-known spell the way
	// the castCreatureSpellInternal bridge does.
	if s != nil && s.Data != nil {
		if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
			if p.Sess != nil && p.Sess.player != nil && p.Sess.player.StandState != 0 &&
				(!spellIsPositive(spellInfo) || spellHasEffect(spellInfo, spellEffectDispel)) {
				p.Sess.player.StandState = 0
				p.Sess.sendPlayerUpdate()
			}
			s.fireCreatureSpellHitTarget(ctx, motion, p.GUID, spellID)
			// Spell::DoTriggersOnSpellHit (Spell.cpp:2652/2913-2957):
			// the per-hit trigger consumer — the ADD_TARGET_TRIGGER
			// snapshot plus the spell_linked type-1 rows — runs
			// inside DoTargetSpellHit per hit target, ahead of the
			// _cast tail, so it fires here on the inline path too
			// (castCreatureSpellInternal runs the same consumer for
			// its own casts).
			s.fireCreatureHitTriggers(ctx, motion, spellInfo, spellID, []uint64{p.GUID})
			// Spell::_cast spell_linked tail (Spell.cpp:3503-3512):
			// type-0 rows fire at cast completion; the inline path
			// has no _cast, so the tail runs here after the per-hit
			// consumer, matching C++ relative order.
			s.fireCreatureSpellLinkedTriggers(ctx, motion, spellID, p.GUID)
		}
	}
}
