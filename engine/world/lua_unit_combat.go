package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// luaUnitCombat groups the shared Eluna Unit attack/damage/heal funnels
// (UnitMethods.h Attack, DealDamage, DealHeal) behind one target model so
// the player, session-creature, and handler-creature surfaces share the
// legs instead of triplicating them. A luaUnitRef resolves a GUID to
// whichever side exists: a live player session first, else a creature
// motion in the caller's map/instance. Creature victims in another
// instance do not resolve (Go's motion registry is per-instance);
// player victims resolve globally, like GetVictim's findPlayer leg.

// luaUnitRef is one side of a scripted Unit interaction.
type luaUnitRef struct {
	srv        *Server
	guid       uint64
	isPlayer   bool
	sess       *session
	motion     *creatureMotion
	mapID      uint32
	instanceID uint32
}

// luaUnitPlayerRef builds the attacker ref for the player surface.
func luaUnitPlayerRef(s *session) luaUnitRef {
	if s == nil || s.player == nil {
		return luaUnitRef{}
	}
	return luaUnitRef{srv: s.server, guid: s.playerGUID, isPlayer: true, sess: s, mapID: s.player.Map, instanceID: s.player.InstanceID}
}

// luaUnitCreatureRef builds the attacker ref for a creature motion.
func luaUnitCreatureRef(srv *Server, motion *creatureMotion) luaUnitRef {
	if srv == nil || motion == nil {
		return luaUnitRef{}
	}
	return luaUnitRef{srv: srv, guid: motion.GUID, motion: motion, mapID: motion.Map, instanceID: motion.InstanceID}
}

// luaUnitRef resolves a target GUID to a live unit, or reports it gone
// (despawned creature, offline player). Callers turn an unresolvable
// target argument (GUID 0) into an arg error before reaching here.
func (srv *Server) luaUnitRef(guid uint64, mapID, instanceID uint32) (luaUnitRef, bool) {
	if srv == nil || guid == 0 {
		return luaUnitRef{}, false
	}
	if sess := srv.findSessionByGUID(guid); sess != nil && sess.player != nil {
		return luaUnitRef{srv: srv, guid: guid, isPlayer: true, sess: sess, mapID: sess.player.Map, instanceID: sess.player.InstanceID}, true
	}
	if motion := srv.findCreatureMotion(mapID, instanceID, guid); motion != nil {
		return luaUnitRef{srv: srv, guid: guid, motion: motion, mapID: motion.Map, instanceID: motion.InstanceID}, true
	}
	return luaUnitRef{}, false
}

// luaUnitSnapshot is the lock-released combat state a funnel leg reads.
// Creature fields are read under motionMu; player fields are read live
// like the rest of the session paths.
type luaUnitSnapshot struct {
	health      uint32
	maxHealth   uint32
	level       uint32
	armor       float64
	resistances [7]uint32
	alive       bool
	evading     bool
	inFlight    bool
	isGM        bool
	mounted     bool
	targetGUID  uint64
	ownerGUID   uint64
	charmerGUID uint64
	inCombat    bool
	x, y, z     float32
	phase       uint32
}

func (ref luaUnitRef) snapshot() luaUnitSnapshot {
	var snap luaUnitSnapshot
	if ref.isPlayer {
		if ref.sess == nil || ref.sess.player == nil {
			return snap
		}
		player := ref.sess.player
		snap.health, snap.maxHealth = player.Health, player.MaxHealth
		snap.level = uint32(player.Level)
		snap.armor = float64(player.Armor)
		snap.resistances = player.Resistances
		snap.alive = player.Health > 0 && !ref.sess.isDeadOrGhost()
		snap.inFlight = ref.sess.inFlight
		snap.isGM = ref.sess.security > 0
		snap.mounted = ref.sess.isPlayerMounted()
		snap.targetGUID = ref.sess.attackTarget
		snap.inCombat = ref.sess.attackTarget != 0 || player.UnitFlags&unitFlagInCombat != 0
		snap.x, snap.y, snap.z = player.X, player.Y, player.Z
		snap.phase = ref.sess.currentPlayerPhaseMask()
		return snap
	}
	if ref.motion == nil || ref.srv == nil {
		return snap
	}
	ref.srv.motionMu.Lock()
	motion := ref.motion
	snap.health, snap.maxHealth, snap.level = motion.Health, motion.MaxHealth, motion.Level
	snap.armor = float64(motion.Armor)
	snap.resistances = motion.Resistances
	snap.alive = motion.Health > 0
	snap.evading = motion.Evading
	snap.targetGUID = motion.TargetGUID
	snap.ownerGUID = motion.OwnerGUID
	snap.charmerGUID = motion.CharmerGUID
	snap.inCombat = motion.InCombat
	snap.x, snap.y, snap.z = motion.X, motion.Y, motion.Z
	snap.phase = motion.PhaseMask
	ref.srv.motionMu.Unlock()
	return snap
}

func (ref luaUnitRef) attackerSession() *session {
	if ref.isPlayer {
		return ref.sess
	}
	return nil
}

// luaUnitAttack is Unit::Attack (Unit.cpp:5650-5760): the gates, the
// same-victim no-op, and the new-victim engage tail. The attacker-list
// swap (_addAttacker/_removeAttacker) has no Go model — no per-unit
// attacker set exists; threat lists cover engagement — and the
// offhand-attack-timer delay has no Go swing-timer model. Go models
// UNIT_STATE_MELEE_ATTACKING as merely having a target (conditions.go),
// with no melee/ranged distinction, so a same-victim call changes
// nothing and returns false; C++ would return true on a melee-state
// toggle (ranged target re-attacked with meleeAttack, or vice versa).
// A new victim with meleeAttack=false engages (threat, assistance,
// emote clear, pet notify) but sends no SMSG_ATTACK_START, matching
// C++; a target switch sends no SMSG_ATTACK_STOP either (C++ only
// interrupts the melee spell), unlike startAttackOn's switch-stop arm.
func luaUnitAttack(ctx context.Context, attacker luaUnitRef, victimGUID uint64, meleeAttack bool) bool {
	srv := attacker.srv
	if srv == nil || attacker.guid == 0 || victimGUID == 0 || victimGUID == attacker.guid {
		return false
	}
	victim, ok := srv.luaUnitRef(victimGUID, attacker.mapID, attacker.instanceID)
	if !ok {
		return false
	}
	attackerSnap := attacker.snapshot()
	victimSnap := victim.snapshot()
	if !attackerSnap.alive || !victimSnap.alive {
		return false
	}
	if attacker.isPlayer && attackerSnap.mounted {
		return false
	}
	if !attacker.isPlayer && attackerSnap.evading {
		return false
	}
	if victim.isPlayer && victimSnap.isGM {
		return false
	}
	// Creature::IsEvadingAttacks is IsInEvadeMode() || CanNotReachTarget();
	// Go models only the evade half (motion.Evading), the reach half has
	// no model.
	if !victim.isPlayer && victimSnap.evading {
		return false
	}
	// SPELL_AURA_MOD_UNATTACKABLE (93) is stripped from the attacker.
	if attacker.isPlayer {
		if attacker.sess != nil && attacker.sess.hasAuraType(spellAuraModUnattackable) {
			attacker.sess.removeAurasByType(spellAuraModUnattackable)
		}
	} else if attacker.motion != nil {
		srv.removeCreatureAurasByType(creatureAuraKeyForMotion(attacker.motion), spellAuraModUnattackable)
	}
	if attackerSnap.targetGUID == victimGUID {
		return false
	}
	if attacker.isPlayer {
		sess := attacker.sess
		if sess == nil || sess.player == nil {
			return false
		}
		sess.attackTarget = victimGUID
		sess.lastCombatTime = time.Now()
		if sess.player.UnitFlags&unitFlagInCombat == 0 {
			markPlayerInCombat(ctx, sess, victimGUID)
			sess.sendPlayerUpdate()
		}
		if meleeAttack {
			startPkt := buildAttackStart(attacker.guid, victimGUID)
			_ = sess.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
			srv.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z, sess)
		}
		sess.notifyPetsOwnerAttacked(ctx, victimGUID)
		return true
	}
	motion := attacker.motion
	if motion == nil {
		return false
	}
	srv.motionMu.Lock()
	wasInCombat := motion.InCombat
	wild := motion.OwnerGUID == 0 && motion.CharmerGUID == 0
	if wild {
		if motion.ThreatMgr == nil {
			motion.ThreatMgr = NewThreatManager(motion)
		}
		// EngageWithTarget (Unit.cpp:8429-8438) seeds 0.0f threat;
		// Attack's explicit victim wins over the threat list's
		// current victim (the engage path's GetCurrentVictim arm is
		// aggro-driven, this one is script-driven).
		srv.creatureAddThreat(motion, victimGUID, 0.0, true, srv.findSessionByGUID(victimGUID), srv.findCreatureMotionLocked(motion.Map, motion.InstanceID, victimGUID))
		// Unit::Attack (Unit.cpp:5734) engages wild creatures via
		// EngageWithTarget -> AddThreat, so a fresh Attack dismounts and
		// re-homes (Creature::AtEngage, Creature.cpp:3435-3436, 3446-3447).
		// Controlled creatures never reach EngageWithTarget there (the
		// IsControlledByPlayer gate), so the legs ride the wild arm only.
		if !wasInCombat {
			atEngageDismount(motion)
			atEngageSetHomePosition(motion)
		}
		motion.EmoteState = 0
	}
	motion.TargetGUID = victimGUID
	motion.ScriptTargetGUID = victimGUID
	motion.InCombat = true
	mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
	srv.motionMu.Unlock()
	if wild {
		if !wasInCombat {
			srv.broadcastAIReactionInInstance(mapID, instanceID, attacker.guid, 2)
		}
		// CallAssistance only models player enemies (its enemyOK arm);
		// a creature victim leaves assistance a no-op, documented.
		srv.callCreatureAssistance(ctx, motion)
	}
	if meleeAttack {
		startPkt := buildAttackStart(attacker.guid, victimGUID)
		srv.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, mapID, instanceID, x, y, z, phase)
	}
	return true
}

// luaUnitDealDamage is Unit::DealDamage through the Eluna legs
// (UnitMethods.h:2724-2857, TRINITY branch): flat damage for school 7,
// armor + absorb + resist + DealDamageMods + attack-state update for a
// set school without a spell, and the spell-info gate + spell-nonmelee
// log + DealSpellDamage for a set school with a spell. The damage
// itself rides the dealDirectDamage funnels (DamageTaken hooks,
// split/share, tap, rage, threat, kill/health legs, broadcasts).
//
// DELTAS: BuildAttackerStateUpdate carries no absorb/resist/blocked
// fields beyond the block flag, so the school leg's packet shows only
// the post-mitigation damage (the C++ packet carries absorb/resist);
// the swing-type byte (1 flat, 0 school) has no builder slot either.
// The spell leg deals 0, matching C++ Trinity: SpellNonMeleeDamage's
// constructor sets damage(0) and Eluna never assigns the Lua damage
// into it, so DealSpellDamage(0) changes no health (the log still
// fires with 0). The 0-damage funnel early-returns, so C++'s
// Unit::DealDamage(0) AI DamageTaken/DamageDealt + OnDamage hooks do
// not fire in Go on this leg. The spell leg applies no absorb/resist
// (C++ runs no CalcAbsorbResist on it).
// durabilityloss is accepted but unmodeled: the funnels always apply
// death durability loss on a lethal player hit and always roll hit
// durability, so false cannot suppress either. DealDamageMods has no
// standalone twin; its dead/in-flight/evading zeroing is applied
// inline, and the funnel's DamageTaken hook/proc legs cover the rest.
func luaUnitDealDamage(ctx context.Context, attacker luaUnitRef, victimGUID uint64, damage uint32, durabilityLoss bool, school uint32, spellID uint32) {
	_ = durabilityLoss
	srv := attacker.srv
	if srv == nil || attacker.guid == 0 || victimGUID == 0 {
		return
	}
	victim, ok := srv.luaUnitRef(victimGUID, attacker.mapID, attacker.instanceID)
	if !ok {
		return
	}
	attackerSnap := attacker.snapshot()
	victimSnap := victim.snapshot()
	var schoolMask uint32
	var funnelSpell uint32
	var sendASU bool
	var sendSpellLog bool
	var absorb, resist uint32
	dmg := damage
	if school == 7 {
		schoolMask = 1
		sendASU = true
	} else if spellID == 0 {
		schoolMask = 1 << school
		// IsDamageReducedByArmor (Unit.cpp:1618): only physical damage
		// is armor-reduced, and Eluna passes a NULL spell info, so the
		// IGNORE_ARMOR attribute never applies on this path.
		if schoolMask&1 != 0 {
			dmg = calcArmorReducedDamage(victimSnap.armor, uint8(attackerSnap.level), uint8(victimSnap.level), dmg)
		}
		// CalcAbsorbResist (Unit.cpp:1828) early-returns on a dead
		// victim, consuming nothing; the DealDamageMods gate below
		// still zeroes the damage and folds it into absorb.
		if victimSnap.alive {
			if victim.isPlayer {
				if victim.sess != nil {
					var remaining uint32
					absorb, remaining = victim.sess.applyAbsorptionShields(dmg, uint8(schoolMask))
					dmg = remaining
				}
			} else if victim.motion != nil {
				var remaining uint32
				absorb, remaining = srv.applyCreatureAbsorptionShields(creatureAuraKeyForMotion(victim.motion), dmg, uint8(schoolMask))
				dmg = remaining
			}
			// Partial magic resist (CalcSpellResistedDamage twin used
			// by the spell-damage paths); physical damage is never
			// resisted (mask 1 returns the damage untouched).
			var remaining uint32
			resist, remaining = calcMagicSpellResistance(dmg, uint8(schoolMask), victimSnap.resistances, uint8(attackerSnap.level), uint8(victimSnap.level), !victim.isPlayer, false, 0, spellResistInput{})
			dmg = remaining
		}
		// DealDamageMods (Unit.cpp:686): a dead, in-flight, or
		// evading-attacks victim takes nothing; the full amount folds
		// into absorb.
		if !victimSnap.alive || victimSnap.inFlight || victimSnap.evading {
			absorb += dmg
			dmg = 0
		}
		sendASU = true
	} else {
		spell, found, err := spellInfoForLuaUnit(srv, spellID)
		if err != nil || !found {
			return
		}
		// SpellNonMeleeDamage's constructor sets damage(0) and Eluna
		// never assigns the Lua damage into it, so C++ Trinity deals
		// 0 on this leg (the arg-school armor leg runs in C++ too,
		// but its result is discarded with the damage); the log
		// below still fires with 0, matching C++ order.
		dmg = 0
		schoolMask = spell.SchoolMask
		funnelSpell = spellID
		// DealSpellDamage (Unit.cpp:1136) early-returns on a dead,
		// in-flight, or evading-attacks victim; the log below still
		// fires first with the zeroed damage, matching C++ order.
		if !victimSnap.alive || victimSnap.inFlight || victimSnap.evading {
			dmg = 0
		}
		sendSpellLog = true
	}
	overkill := uint32(0)
	if dmg >= victimSnap.health && victimSnap.health > 0 {
		overkill = dmg - victimSnap.health
	}
	attackerSess := attacker.attackerSession()
	if sendSpellLog {
		// C++ logs before DealSpellDamage, with the pre-funnel damage.
		logPkt := buildSpellNonMeleeDamageLog(victimGUID, attacker.guid, spellID, dmg, overkill, uint8(schoolMask), absorb, resist, 0, 0)
		luaUnitBroadcastDamageLog(srv, attacker, victim, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt)
	}
	if victim.isPlayer {
		if victim.sess != nil {
			srv.dealDirectDamageToPlayer(ctx, attacker.guid, attackerSess, victim.sess, funnelSpell, schoolMask, dmg)
		}
	} else if victim.motion != nil {
		srv.dealDirectDamageToCreature(ctx, attacker.guid, attackerSess, victim.motion, creatureAuraKeyForMotion(victim.motion), funnelSpell, schoolMask, true, dmg)
	}
	if sendASU {
		asuPkt := protocol.BuildAttackerStateUpdate(attacker.guid, victimGUID, dmg, overkill, protocol.HitInfoAffectsVictim, protocol.VictimStateHit, 0)
		luaUnitBroadcastDamageLog(srv, attacker, victim, uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt)
	}
}

// spellInfoForLuaUnit resolves a spell id through the DBC store: the
// GetSpellInfo gate for DealDamage's spell leg and DealHeal. A missing
// store reads as unknown (silent return, the C++ behavior).
func spellInfoForLuaUnit(srv *Server, spellID uint32) (wotlk.Spell, bool, error) {
	if srv == nil || srv.Data == nil {
		return wotlk.Spell{}, false, nil
	}
	return srv.Data.Spell(spellID)
}

// luaUnitBroadcastDamageLog fans a damage packet out from the attacker:
// a player attacker writes to itself and broadcasts to its visible set
// (the SendAttackStateUpdate/SendMessageToSet pattern in combat.go);
// a creature attacker broadcasts from its phase position.
func luaUnitBroadcastDamageLog(srv *Server, attacker, victim luaUnitRef, opcode uint16, pkt []byte) {
	if srv == nil {
		return
	}
	if attacker.isPlayer {
		if attacker.sess != nil && attacker.sess.player != nil {
			_ = attacker.sess.write(opcode, pkt, true)
			srv.broadcastToVisibleSet(opcode, pkt, attacker.sess.player.Map, attacker.sess.player.InstanceID, attacker.sess.player.X, attacker.sess.player.Y, attacker.sess.player.Z, attacker.sess)
		}
		return
	}
	snap := attacker.snapshot()
	srv.broadcastToVisibleSetFromPhase(opcode, pkt, attacker.mapID, attacker.instanceID, snap.x, snap.y, snap.z, snap.phase)
}

// luaUnitDealHeal is Unit::HealBySpell (Unit.cpp:6561-6570) through the
// Eluna DealHeal binding (UnitMethods.h:2863-2892): unknown spells
// return silently, CalcHealAbsorb consumes the target's heal-absorb
// shields, the DealHeal AI hooks fire on the pre-clamp amount, the gain
// caps at max health, and the heal log carries the critical flag.
// ModifyHealReceived has no Go script-hook twin (the AutoBalance scale
// for creature healers, the pet-twin precedent, is the closest leg);
// healing threat is NOT forwarded (HealBySpell itself forwards none —
// the Spell.cpp:2514 forward belongs to the spell-effect path, not this
// binding). Dead targets are skipped, the Go heal-path convention;
// C++ ModifyHealth-on-corpse is unmodeled.
func luaUnitDealHeal(ctx context.Context, healer luaUnitRef, targetGUID uint64, spellID, amount uint32, critical bool) {
	srv := healer.srv
	if srv == nil || healer.guid == 0 || targetGUID == 0 {
		return
	}
	spell, found, err := spellInfoForLuaUnit(srv, spellID)
	if err != nil || !found {
		return
	}
	target, ok := srv.luaUnitRef(targetGUID, healer.mapID, healer.instanceID)
	if !ok {
		return
	}
	targetSnap := target.snapshot()
	if !targetSnap.alive {
		return
	}
	var absorbed uint32
	if target.isPlayer {
		if target.sess != nil {
			absorbed = target.sess.applyHealAbsorb(spell.SchoolMask, amount)
		}
	} else if target.motion != nil {
		absorbed = srv.creatureApplyHealAbsorb(creatureAuraKeyForMotion(target.motion), spell.SchoolMask, amount)
	}
	heal := amount - absorbed
	if !healer.isPlayer && healer.motion != nil {
		heal = srv.autoBalanceModifyDealDamageOnMap(healer.motion, target.mapID, heal)
	}
	var victimMotion, healerMotion *creatureMotion
	if !target.isPlayer {
		victimMotion = target.motion
	}
	if !healer.isPlayer {
		healerMotion = healer.motion
	}
	fireDealHealAIHooks(ctx, srv, victimMotion, healerMotion, healer.guid, targetGUID, &heal)
	overheal := uint32(0)
	effectiveHeal := heal
	if uint64(targetSnap.health)+uint64(heal) > uint64(targetSnap.maxHealth) {
		overheal = uint32(uint64(targetSnap.health) + uint64(heal) - uint64(targetSnap.maxHealth))
		effectiveHeal = heal - overheal
		if targetSnap.health >= targetSnap.maxHealth {
			effectiveHeal = 0
		}
	}
	newHealth := targetSnap.health + effectiveHeal
	if newHealth > targetSnap.maxHealth {
		newHealth = targetSnap.maxHealth
	}
	if target.isPlayer {
		if target.sess != nil && target.sess.player != nil {
			target.sess.player.Health = newHealth
			target.sess.sendPlayerUpdate()
		}
	} else if target.motion != nil {
		srv.motionMu.Lock()
		if live := srv.findCreatureMotionLocked(target.mapID, target.instanceID, targetGUID); live != nil && live.Health > 0 {
			live.Health = newHealth
		}
		srv.motionMu.Unlock()
		srv.broadcastCreatureValuesUpdateInInstance(target.mapID, target.instanceID, targetGUID, map[int]uint32{unitFieldHealth: newHealth})
	}
	if healer.isPlayer && healer.sess != nil {
		if effectiveHeal > 0 {
			healer.sess.updateAchievementCriteria(criteriaTypeHealingDone, 0, effectiveHeal)
		}
		healer.sess.setAchievementCriteria(criteriaTypeHighestHealCasted, 0, heal)
		if srv != nil {
			srv.updateArenaHealingScore(healer.sess, effectiveHeal)
			srv.addBGScore(healer.sess, func(score *bgPlayerScore) { score.HealingDone += effectiveHeal })
		}
	}
	if target.isPlayer && target.sess != nil {
		target.sess.updateAchievementCriteria(criteriaTypeTotalHealingReceived, 0, effectiveHeal)
		target.sess.setAchievementCriteria(criteriaTypeHighestHealingRecv, 0, heal)
	}
	logPkt := buildSpellHealLog(targetGUID, healer.guid, spellID, heal, overheal, absorbed, critical)
	if healer.isPlayer {
		if healer.sess != nil {
			_ = healer.sess.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), logPkt, true)
			srv.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), logPkt, healer.sess.player.Map, healer.sess.player.InstanceID, healer.sess.player.X, healer.sess.player.Y, healer.sess.player.Z, healer.sess)
		}
		return
	}
	if target.isPlayer && target.sess != nil {
		_ = target.sess.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), logPkt, true)
		snap := healer.snapshot()
		srv.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), logPkt, healer.mapID, healer.instanceID, snap.x, snap.y, snap.z, target.sess)
		return
	}
	snap := healer.snapshot()
	srv.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), logPkt, healer.mapID, healer.instanceID, snap.x, snap.y, snap.z, snap.phase)
}
