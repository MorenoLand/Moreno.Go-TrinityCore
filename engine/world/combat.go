package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	creatureFlagExtraNoParryHasten = 0x00000008
	// CREATURE_FLAG_EXTRA_NO_TAUNT (CreatureData.h:45): creature is immune
	// to taunt auras and the ATTACK_ME effect — Creature::ApplySpellImmune
	// rows at Creature.cpp:634-638.
	creatureFlagExtraNoTaunt = 0x00000100
	// CREATURE_FLAG_EXTRA_IMMUNITY_KNOCKBACK (CreatureData.h:67):
	// Creature.cpp:1184-1187 applies IMMUNITY_EFFECT KNOCK_BACK +
	// KNOCK_BACK_DEST.
	creatureFlagExtraImmunityKnockback = 0x40000000
	attackDisplayDelay                 = 200 * time.Millisecond
	// MAX_SHEATH_STATE (UnitDefines.h:98): sheath states run 0 (unarmed) to
	// 2 (ranged); HandleSetSheathedOpcode (CombatHandler.cpp) rejects anything
	// at or past this value.
	maxSheathState = 3
	// Sheath states (UnitDefines.h:93-97) for Player::SetSheath's virtual
	// item slot switch (Player.cpp:9655-9682).
	sheathStateUnarmed = 0
	sheathStateMelee   = 1
	sheathStateRanged  = 2
)

// haveOffhandWeapon checks if the player has an offhand weapon equipped.
func (s *session) haveOffhandWeapon() bool {
	if s == nil || s.player == nil {
		return false
	}
	return s.player.OffhandAttackTime > 0 || s.player.MaxOffhandDamage > 0
}

// calcParryHastedRemaining computes new remaining attack swing time when a defender parries,
// matching TrinityCore Unit.cpp:1480-1510.
// If remaining time is <= 20% of weapon speed, it is unchanged.
// If remaining time is between 20% and 60%, it is set to 20%.
// If remaining time is > 60%, it is reduced by 40% (2 * 20%).
func calcParryHastedRemaining(remaining, attackTime time.Duration) time.Duration {
	if attackTime <= 0 || remaining <= 0 {
		return remaining
	}
	percent20 := attackTime / 5
	percent60 := 3 * percent20
	if remaining > percent20 && remaining <= percent60 {
		return percent20
	}
	if remaining > percent60 {
		hasted := remaining - 2*percent20
		if hasted < percent20 {
			return percent20
		}
		return hasted
	}
	return remaining
}

type combatTarget struct {
	GUID        uint64
	Map         uint32
	InstanceID  uint32
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	Health      uint32
	MaxHealth   uint32
	Armor       uint32
	Resistances [7]uint32
	MinDamage   float32
	MaxDamage   float32
	Level       uint8
	UnitFlags   uint32
	FlagsExtra  uint32
	Faction     uint32
	CombatReach float32
}

// calcMeleeRange computes maximum melee attack distance between attacker and target,
// matching TrinityCore Unit::GetMeleeRange (Unit.cpp:614-618):
// max(attacker.CombatReach + target.CombatReach + 4.0/3.0, NOMINAL_MELEE_RANGE)
// (NOMINAL_MELEE_RANGE = 5.0f; the 1.5 defaults mirror the engine's default
// combat reach). Boundary matches: C++ IsWithinMeleeRangeAt tests
// distsq <= maxdist*maxdist (Unit.cpp:611), Go tests dist <= calcMeleeRange.
// Audit notes (Unit::IsWithinCombatRange 583-597 / IsWithinMeleeRangeAt 599-612
// / GetMeleeRange 614-618 / resetAttackTimer 571-575 / Player::Update melee
// block Player.cpp:1116-1180):
//   - Unit::IsWithinCombatRange (dist2compare + both combat reaches, 3D,
//     strict <) is the AI spell-range predicate (UnitAI::DoSpellAttackIfReady,
//     UnitAI.cpp:88-100). The DoCast victim arm instead runs Spell::CheckRange
//     (exact 3D center distance vs GetMinMaxRange), bridged by
//     aiDoCastVictimInRange (creaturemotion.go) — the old gate's
//     calcMeleeRange +4/3 and 5.0 floor overshoot is closed.
//   - IsWithinMeleeRangeAt's Position overload (predicted-position test) has
//     no Go analog; Go always measures from live positions.
//   - C++ applies no range slop on the swing gate; Go adds +2.0 (updatePlayerCombat,
//     handleAttackSwing) as lag compensation — a deliberate delta.
//   - resetAttackTimer multiplies by m_modAttackSpeedPct (SPELL_AURA_MOD_MELEE_HASTE
//     / MOD_RANGED_HASTE via Unit::applyAttackTimePercentMod, Unit.cpp:10851-10864);
//     Go's getHastedMeleeSpeed/getHastedRangedSpeed model rating haste only, so
//     aura-driven attack-speed changes never alter swing cadence.
//   - The m_attackTimer init (Unit.cpp:316-318, all zero) means the first swing
//     fires on the next update tick; Go's handleAttackSwing likewise never
//     swings — it only sets the victim (Unit::Attack never swings, and this
//     tree has no ResetAttackTimer call in it), so the first swing also
//     lands on the next 100ms tick via updatePlayerCombat. A same-victim
//     T re-press is a no-op, exactly like C++ returning false.
//   - C++ delays the offhand timer only for non-players
//     (Unit.cpp:5743, GetTypeId() != TYPEID_PLAYER), so Go applies no
//     offhand delay on attack start either.
func calcMeleeRange(attackerReach, victimReach float32) float64 {
	if attackerReach <= 0 {
		attackerReach = 1.5
	}
	if victimReach <= 0 {
		victimReach = 1.5
	}
	rangeVal := float64(attackerReach + victimReach + 4.0/3.0)
	if rangeVal < 5.0 {
		return 5.0
	}
	return rangeVal
}

// inMeleeThreatRange is the positional melee test for the threat-switch gate,
// matching the per-candidate Unit::IsWithinMeleeRange check C++
// ThreatManager::ReselectVictim applies (Unit::IsWithinMeleeRangeAt 599-612 /
// GetMeleeRange 614-618): both combat reaches + 4/3, floored at
// NOMINAL_MELEE_RANGE.
func inMeleeThreatRange(attackerReach, victimReach float32, dist float64) bool {
	return dist <= calcMeleeRange(attackerReach, victimReach)
}

func (s *session) getCombatTarget(ctx context.Context, guid uint64) (combatTarget, bool) {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	// Check if target is an online player (e.g. duel opponent or PvP target)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(guid); playerSess != nil && playerSess.player != nil {
			reach := playerSess.player.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			return combatTarget{
				GUID:        guid,
				Map:         playerSess.player.Map,
				InstanceID:  playerSess.player.InstanceID,
				X:           playerSess.player.X,
				Y:           playerSess.player.Y,
				Z:           playerSess.player.Z,
				Orientation: playerSess.player.Orientation,
				Health:      playerSess.player.Health,
				MaxHealth:   playerSess.player.MaxHealth,
				Armor:       playerSess.player.Armor,
				Resistances: playerSess.player.Resistances,
				MinDamage:   playerSess.player.MinDamage,
				MaxDamage:   playerSess.player.MaxDamage,
				Level:       playerSess.player.Level,
				UnitFlags:   playerSess.player.UnitFlags,
				FlagsExtra:  0,
				Faction:     0,
				CombatReach: reach,
			}, true
		}
	}
	s.server.motionMu.Lock()
	if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, guid); motion != nil {
		if motion != nil {
			reach := motion.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			target := combatTarget{
				GUID:        guid,
				Map:         motion.Map,
				InstanceID:  motion.InstanceID,
				X:           motion.X,
				Y:           motion.Y,
				Z:           motion.Z,
				Orientation: motion.Orientation,
				Health:      motion.Health,
				MaxHealth:   motion.MaxHealth,
				Armor:       motion.Armor,
				Resistances: motion.Resistances,
				MinDamage:   motion.MinDamage,
				MaxDamage:   motion.MaxDamage,
				Level:       uint8(motion.Level),
				UnitFlags:   motion.UnitFlags,
				FlagsExtra:  motion.FlagsExtra,
				Faction:     motion.Faction,
				CombatReach: reach,
			}
			s.server.motionMu.Unlock()
			return target, true
		}
	}
	s.server.motionMu.Unlock()

	target, err := s.loadCombatTarget(ctx, guid)
	if err != nil {
		return combatTarget{}, false
	}
	return target, true
}

// gmAttackTargetBlocked mirrors the player-target legs of
// WorldObject::IsValidAttackTarget (Object.cpp:2945-2947): a target in GM mode
// (PLAYER_EXTRA_GM_ON, the "can't attack GMs" leg) or GM invisibility (the
// "can't attack invisible" visibility leg) cannot be attacked. The melee swing
// path reaches it via HandleAttackSwingOpcode's IsValidAttackTarget gate
// (CombatHandler.cpp), the pet attack command via PetAI::AttackStart into
// Unit::Attack's GM leg (Unit.cpp:5664-5668), and directed pet spell casts via
// SpellInfo::CheckTarget (SpellInfo.cpp:1736-1743). Creature targets never
// match: findSessionByGUID returns nil for them.
func (s *Server) gmAttackTargetBlocked(guid uint64) bool {
	if s == nil || guid == 0 {
		return false
	}
	targetSess := s.findSessionByGUID(guid)
	if targetSess == nil || targetSess.player == nil {
		return false
	}
	flags := targetSess.player.ExtraFlags
	return flags&playerExtraGMInvisible != 0 || flags&playerExtraGMOn != 0
}

// ghostAttackTargetBlocked mirrors the "can't attack dead" leg of
// WorldObject::IsValidAttackTarget (Object.cpp:2935-2937):
// ((!bySpell || !bySpell->IsAllowingDeadTarget()) && unitTarget &&
// !unitTarget->IsAlive()) rejects the target. Ghosts carry Health=1 in Go
// (buildPlayerRepop, death.go:473) but IsAlive() is false in C++
// (deathState CORPSE), so the Health==0 dead gates miss them. Creature
// targets never match: dead creatures report Health==0 through
// getCombatTarget, and no ghost flag exists on creature motions.
func (s *session) ghostAttackTargetBlocked(guid uint64) bool {
	if s == nil || s.server == nil || guid == 0 {
		return false
	}
	if vicSess := s.server.findSessionByGUID(guid); vicSess != nil && vicSess.player != nil {
		return vicSess.isDeadOrGhost()
	}
	return false
}

// startAttackOn mirrors the tail of C++ Unit::Attack (Unit.cpp:5650-5760) as
// bridged in handleAttackSwing: it records the victim and emits
// SMSG_ATTACK_START to the caster and nearby players. It is for
// server-driven attack starts (e.g. Spell::EffectCharge's HIT_TARGET arm,
// SpellEffects.cpp:4520-4525) where the target has already passed spell
// target validation, so the CMSG gates are not re-run.
func (s *session) startAttackOn(ctx context.Context, victim uint64) {
	if s == nil || s.player == nil || victim == 0 {
		return
	}
	if s.attackTarget != 0 && s.attackTarget == victim {
		return
	}
	if s.attackTarget != 0 {
		if err := s.sendAttackStop(s.attackTarget, false); err != nil {
			return
		}
	}
	s.attackTarget = victim
	s.lastCombatTime = time.Now()
	if s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}
	startPayload := buildAttackStart(s.playerGUID, victim)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPayload, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPayload, s)
	}
	// Unit::Attack's controlled-pet notify (Unit.cpp:5752-5760) fires for
	// spell-driven attack starts too (EffectCharge calls Attack directly,
	// SpellEffects.cpp:4520-4525).
	s.notifyPetsOwnerAttacked(ctx, victim)
}

func (s *session) handleAttackSwing(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.isDeadOrGhost() {
		s.attackTarget = 0
		return true
	}
	if s.player.UnitFlags&(unitFlagConfused|unitFlagFleeing) != 0 || s.hasAuraType(spellAuraCharm) {
		s.attackTarget = 0
		return true
	}
	reader := protocol.NewReader(payload)
	// CombatPackets.cpp:21-24: CMSG_ATTACK_SWING carries the victim as a
	// packed ObjectGuid (mask byte + non-zero bytes), not a full u64.
	victim, err := reader.ReadPackedGUID()
	if err != nil {
		s.debug("attack rejected", "account", s.accountName, "error", err)
		return true
	}
	target, ok := s.getCombatTarget(ctx, victim)
	if !ok {
		// CombatHandler.cpp:31-36: a CMSG_ATTACKSWING naming a unit the
		// server cannot resolve answers SMSG_ATTACK_STOP with an empty
		// victim guid (WorldSession::SendAttackStop(nullptr)); the packet
		// goes to the session only, not the surrounding set.
		_ = s.sendAttackStopSelf(0)
		return true
	}
	// WorldObject::IsValidAttackTarget (Object.cpp:2936-2938): attacking
	// yourself is never valid; the handler answers with SMSG_ATTACK_STOP
	// (HandleAttackSwingOpcode, CombatHandler.cpp:40-43).
	if victim == s.playerGUID {
		return s.sendAttackStopSelf(victim) == nil
	}
	// WorldObject::IsValidAttackTarget GM/invisibility legs (Object.cpp:2945-2947)
	// via HandleAttackSwingOpcode (CombatHandler.cpp): a GM-mode or GM-invisible
	// player target rejects CMSG_ATTACK_SWING with SMSG_ATTACK_STOP. C++ checks
	// IsValidAttackTarget before Unit::Attack, so this gate sits ahead of the
	// dead-target reject and of Unit::Attack's own GM leg (Unit.cpp:5664-5668).
	if s.server != nil && s.server.gmAttackTargetBlocked(victim) {
		return s.sendAttackStopSelf(victim) == nil
	}
	if target.Health == 0 {
		// CombatHandler.cpp:40-43: a dead target fails IsValidAttackTarget and
		// answers SMSG_ATTACK_STOP via SendAttackStop(enemy). The
		// SMSG_ATTACKSWING_DEADTARGET sender (Player::SendAttackSwingDeadTarget,
		// Player.cpp:20585) has no callers in this revision, so C++ never
		// emits that packet on this path.
		return s.sendAttackStopSelf(victim) == nil
	}
	// WorldObject::IsValidAttackTarget's "can't attack dead" leg
	// (Object.cpp:2935-2937): ghosts model IsAlive()==false with Health=1,
	// so the dead-target gate above misses them. C++ answers an
	// IsValidAttackTarget failure with SendAttackStop (HandleAttackSwingOpcode,
	// CombatHandler.cpp:40-43), not the dead-target packet (whose sender,
	// Player::SendAttackSwingDeadTarget, has no callers in this revision).
	if s.ghostAttackTargetBlocked(victim) {
		return s.sendAttackStopSelf(victim) == nil
	}
	if creatureCombatDisabled(target.UnitFlags, target.FlagsExtra) {
		return s.sendAttackStopSelf(victim) == nil
	}
	// WorldObject::IsValidAttackTarget's remaining flag arms for the melee
	// (bySpell=null) path (Object.cpp:2982): the bundle is
	// NON_ATTACKABLE | TAXI_FLIGHT | NOT_ATTACKABLE_1 | NON_ATTACKABLE_2.
	// creatureCombatDisabled covers NON_ATTACKABLE (0x2); the two static
	// terms NOT_ATTACKABLE_1 (0x80) and NON_ATTACKABLE_2 (0x10000, aura 93's
	// flag, written at runtime on player victims at spells.go) reject here.
	// TAXI_FLIGHT (0x100000) and the UNIT_STATE_UNATTACKABLE state arm
	// (Object.cpp:2939-2941) ride the live TaxiPath flag: MoveTaxiFlight is
	// TYPEID_PLAYER-only, so a session with a live taxi path is an
	// in-flight victim C++ rejects before Unit::Attack.
	if target.UnitFlags&(unitFlagNotAttackable1|unitFlagNonAttackable2) != 0 {
		return s.sendAttackStopSelf(victim) == nil
	}
	if s.server != nil {
		if victimSess := s.server.findSessionByGUID(victim); victimSess != nil && victimSess.player != nil && victimSess.player.TaxiPath != "" {
			return s.sendAttackStopSelf(victim) == nil
		}
	}
	if target.Map != s.player.Map {
		return s.sendAttackStopSelf(victim) == nil
	}
	// WorldObject::IsValidAttackTarget (Object.cpp:3026-3027): friendly
	// targets reject with SMSG_ATTACK_STOP, checked in both directions
	// (IsFriendlyTo(target) || target->IsFriendlyTo(this)).
	if s.attackTargetFriendlyToPlayer(victim, target) {
		return s.sendAttackStopSelf(victim) == nil
	}
	// CombatHandler.cpp:47-56: a passenger may start a melee swing only when
	// their vehicle seat carries VEHICLE_SEAT_FLAG_CAN_ATTACK; otherwise the
	// swing is rejected with SMSG_ATTACK_STOP before Unit::Attack is reached.
	if s.player.VehicleGUID != 0 && s.server != nil {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			if _, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID); seatInfo != nil && !seatInfo.HasFlag(wotlk.VehicleSeatFlagCanAttack) {
				return s.sendAttackStopSelf(victim) == nil
			}
		}
	}
	// Unit::Attack (Unit.cpp:5660-5662): a mounted player cannot attack — the
	// call returns false silently (no packet), so this sits after the handler's
	// AttackStop gates and ahead of the attack tail.
	if s.isPlayerMounted() {
		s.attackTarget = 0
		return true
	}
	// Unit::Attack (Unit.cpp:5664-5676): a creature victim that is evading
	// rejects silently — no packet, no attack-state change. (The GM-victim
	// leg is already answered above via IsValidAttackTarget.)
	if s.server != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, victim) {
		return true
	}
	// Unit::Attack (Unit.cpp:5680-5683): remove SPELL_AURA_MOD_UNATTACKABLE at
	// attack (non-interruptible stun auras that still forbid attacking are
	// stripped here); this runs before the same-target no-op below.
	if s.hasAuraType(spellAuraModUnattackable) {
		s.removeAurasByType(spellAuraModUnattackable)
	}
	// C++ Unit::Attack (Unit.cpp:5650-5760): re-pressing T on the current
	// victim while melee-attacking is a no-op (returns false, no packet);
	// it never swings and never touches the attack timer. Swings come
	// only from the update loop when the timer expires (m_attackTimer
	// starts at 0, so the first swing lands on the next tick). Go
	// previously swung immediately on every CMSG_ATTACKSWING, letting
	// clients spam T to bypass weapon speed entirely.
	if s.attackTarget != 0 && s.attackTarget == victim {
		return true
	}
	if s.attackTarget != 0 {
		// Unit::Attack (Unit.cpp:5701-5708): switching target interrupts
		// CURRENT_MELEE_SPELL — the queued next-swing spell never lands on
		// the new target — but sends no SMSG_ATTACK_STOP for the old
		// target; the switch is announced by the SMSG_ATTACK_START below.
		s.cancelNextSwingSpell()
	}
	s.attackTarget = victim
	s.lastCombatTime = time.Now()
	if s.player != nil && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}
	s.debug("attack started", "account", s.accountName, "guid", victim)
	startPayload := buildAttackStart(s.playerGUID, victim)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPayload, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPayload, s)
	}
	s.notifyPetsOwnerAttacked(ctx, victim)
	return true
}

// ownerAttackTargetValid mirrors the owner->IsValidAttackTarget(TargetUnit)
// gate on the pet attack command (PetHandler.cpp HandlePetActionHelper
// COMMAND_ATTACK:203-207): the order drops silently unless the owner — the
// session player for every player-controlled pet — passes
// WorldObject::IsValidAttackTarget (bySpell=null, Object.cpp:2930+) on the
// named target. It carries the same static arms as the CMSG_ATTACKSWING
// twin (GM/invisibility, dead/ghost, NON_ATTACKABLE bundle, taxi/
// UNIT_STATE_UNATTACKABLE, cross-map) plus the arms the swing path lacks:
// the self-target leg (Object.cpp:2938, this==target) and the
// "can't attack friendly targets" verdict (Object.cpp:3031, via the
// explicit-enemy faction gate with the session player as attacker).
// No-model deltas: CanSeeOrDetect visibility, the IsImmuneToNPC/PC legs,
// and TREAT_AS_RAID_UNIT. For charmed wild creatures C++ skips the gate
// (GetOwner is null); Go applies the owner verdict, a documented
// over-strictness on that path.
func (s *session) ownerAttackTargetValid(ctx context.Context, targetGUID uint64) bool {
	if s == nil || s.player == nil || targetGUID == 0 || targetGUID == s.playerGUID {
		return false
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok {
		// PetHandler.cpp:210-213: an unresolvable unit target drops the
		// order before any engage or AI reaction (ObjectAccessor::GetUnit
		// nil arm).
		return false
	}
	if s.server != nil && s.server.gmAttackTargetBlocked(targetGUID) {
		return false
	}
	if target.Health == 0 || s.ghostAttackTargetBlocked(targetGUID) {
		return false
	}
	if creatureCombatDisabled(target.UnitFlags, target.FlagsExtra) {
		return false
	}
	if target.UnitFlags&(unitFlagNotAttackable1|unitFlagNonAttackable2) != 0 {
		return false
	}
	if s.server != nil {
		if victimSess := s.server.findSessionByGUID(targetGUID); victimSess != nil && victimSess.player != nil && victimSess.player.TaxiPath != "" {
			return false
		}
	}
	if target.Map != s.player.Map {
		return false
	}
	return !s.explicitTargetFactionBlocked(targetFlagUnitEnemy, targetGUID, target)
}

// godCheatActive reports whether this session's player carries the CHEAT_GOD
// flag (.cheat god, commands.go).
func (s *session) godCheatActive() bool {
	return s != nil && s.player != nil && s.player.ActiveCheats&cheatGod != 0
}

// negateGodModeDamage mirrors Unit::DealDamage's CHEAT_GOD arm (Unit.cpp:735-737):
// a player victim with the god cheat takes no damage — DealDamage returns 0
// before the aura-interrupt, share-damage, rage, duel, achievement, and
// kill/health legs. Every player-victim damage application site zeroes through
// here (melee, ranged, direct spell, periodic, pet, creature), and
// environmentalDamage (movement.go) does too, because C++
// Player::EnvironmentalDamage routes through Unit::DealDamage (Player.cpp:784),
// which also covers the HandleFall zeroing (Player.cpp:25390). It is applied
// after the victim-side hooks (procs, combat logs) because in C++ those fire
// ahead of DealDamage as well (DealMeleeDamage procs, SMSG_SPELLNONMELEEDAMAGELOG
// / periodic aura log precede the DealDamage call).
func (s *session) negateGodModeDamage(damage uint32) uint32 {
	if s.godCheatActive() {
		return 0
	}
	return damage
}

// hitTriggerAura is one victim-side SPELL_AURA_ADD_CASTER_HIT_TRIGGER (111)
// aura effect eligible for the melee redirect: the magnet is the aura's
// caster GUID and chance is the effect amount (roll_chance_i percent).
type hitTriggerAura struct {
	spellID    uint32
	chance     int32
	magnet     uint64
	aura       *activeAura
	owner      *session // player victim; nil for creature victims
	creaKey    creatureAuraKey
	isCreature bool
}

// victimHitTriggerAuras collects the victim's 111 aura effects, player or
// creature victim, mirroring victim->GetAuraEffectsByType(
// SPELL_AURA_ADD_CASTER_HIT_TRIGGER) (Unit.cpp:6351). Per-effect aura kinds
// come from the DBC like targetAuraEffectsByType (spells.go).
func (s *session) victimHitTriggerAuras(victimGUID uint64) []hitTriggerAura {
	var out []hitTriggerAura
	collect := func(spellID uint32, aura *activeAura, owner *session, key creatureAuraKey, isCreature bool) {
		if aura == nil || aura.Stopped || aura.CasterGUID == 0 {
			return
		}
		for i := 0; i < 3; i++ {
			if aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			t := aura.AuraType
			if sp, found, _ := s.server.Data.Spell(spellID); found && i < len(sp.Effects) {
				t = sp.Effects[i].Aura
			}
			if t != spellAuraAddCasterHitTrigger {
				continue
			}
			out = append(out, hitTriggerAura{spellID: spellID, chance: aura.Amounts[i], magnet: aura.CasterGUID, aura: aura, owner: owner, creaKey: key, isCreature: isCreature})
		}
	}
	if vs := s.server.findSessionByGUID(victimGUID); vs != nil && vs.player != nil {
		vs.castMu.Lock()
		for id, aura := range vs.activeAuras {
			collect(id, aura, vs, creatureAuraKey{}, false)
		}
		vs.castMu.Unlock()
	} else if s.player != nil {
		key := creatureAuraKeyForPlayer(*s.player, victimGUID)
		s.server.auraMu.Lock()
		for id, aura := range s.server.activeCreatureAuras[key] {
			collect(id, aura, nil, key, true)
		}
		s.server.auraMu.Unlock()
	}
	return out
}

// dropHitTriggerCharge mirrors the DropCharge(AURA_REMOVE_BY_EXPIRE) on a
// successful redirect (Unit.cpp:6359): one charge is consumed and the aura
// is removed at zero charges. The removals lock internally, so the charge
// decrement runs under the map lock first and removal after release.
func (s *session) dropHitTriggerCharge(ht hitTriggerAura) {
	if ht.isCreature {
		s.server.auraMu.Lock()
		remove := false
		if m := s.server.activeCreatureAuras[ht.creaKey]; m != nil {
			if a, ok := m[ht.spellID]; ok && a == ht.aura && !a.Stopped {
				if a.RemainingCharges > 0 {
					a.RemainingCharges--
				}
				remove = a.RemainingCharges == 0
			}
		}
		s.server.auraMu.Unlock()
		if remove {
			s.server.removeCreatureAura(ht.creaKey, ht.spellID)
		}
		return
	}
	vs := ht.owner
	if vs == nil {
		return
	}
	vs.castMu.Lock()
	remove := false
	if a, ok := vs.activeAuras[ht.spellID]; ok && a == ht.aura && !a.Stopped {
		if a.RemainingCharges > 0 {
			a.RemainingCharges--
		}
		remove = a.RemainingCharges == 0
	}
	vs.castMu.Unlock()
	if remove {
		vs.removeAura(ht.spellID)
	}
}

// meleeHitRedirectGUID mirrors the auto-attack arm of Unit::AttackerStateUpdate
// (Unit.cpp:2130-2156) via Unit::GetMeleeHitRedirectTarget
// (Unit.cpp:6349-6363): while the victim carries a
// SPELL_AURA_ADD_CASTER_HIT_TRIGGER (111) aura — Warrior Intervene (3411) /
// hunter-pet Intervene (53476), 100% chance, 1 charge — the next melee swing
// is redirected onto the aura's caster (the magnet) when the magnet is a
// valid attack target, consuming one charge. C++ additionally requires the
// magnet within LOS of the attacker and runs CheckExplicitTarget/CheckTarget
// with a null spellInfo (vacuous); Go has no LOS model, so the validity gate
// degrades to alive + hostile like the grounding-totem bridge. The ranged
// auto-attack path never redirects (Unit.cpp:2146-2147 "ignore ranged case");
// the SPELL_DAMAGE_CLASS_RANGED explicit-target arm of
// Spell::SelectExplicitTargets (Spell.cpp:743-748) is bridged in
// finishSpellCast (spells.go), ahead of every effect-target use of the
// explicit unit GUID.
func (s *session) meleeHitRedirectGUID(ctx context.Context, victimGUID uint64) (uint64, bool) {
	if s == nil || s.server == nil || s.player == nil || victimGUID == 0 {
		return 0, false
	}
	for _, ht := range s.victimHitTriggerAuras(victimGUID) {
		if ht.magnet == 0 || ht.magnet == victimGUID || ht.magnet == s.playerGUID {
			continue
		}
		magnet, ok := s.getCombatTarget(ctx, ht.magnet)
		if !ok || magnet.Health == 0 {
			continue
		}
		if ms := s.server.findSessionByGUID(ht.magnet); ms != nil {
			if s.isFriendlyToTarget(ht.magnet, ms) {
				continue
			}
		} else if !isHostileFactionFallback(magnet.Faction, s.player.Race) {
			continue
		}
		// roll_chance_i(amount): Random.h:59-62, chance > irand(0, 99).
		if ht.chance <= 0 || rand.IntN(100) >= int(ht.chance) {
			continue
		}
		s.dropHitTriggerCharge(ht)
		return ht.magnet, true
	}
	return 0, false
}

// swingBlockValue folds the victim-side inputs of the MELEE_HIT_BLOCK arm of
// Unit::CalculateMeleeDamage (Unit.cpp:1340-1376): the victim's shield block
// value — Player::GetShieldBlockValue (Player.cpp:5426-5430) for player
// victims, the Creature::GetShieldBlockValue (Creature.cpp:2715) level/2
// stand-in for creature victims (combatTarget carries no Strength, so the
// STR/20 leg stays unbridged — the same stand-in spellTakenBlockAmount uses)
// — plus the SPELL_AURA_MOD_BLOCK_CRIT_CHANCE (253) total feeding
// Unit::isBlockCritical (Unit.cpp:2432-2437).
func (s *session) swingBlockValue(target combatTarget, isPlayerVictim bool) (uint32, int32) {
	var blockValue uint32
	var critChance int32
	if isPlayerVictim {
		if s.server != nil {
			if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
				blockValue, critChance = vicSess.shieldBlockParams()
			}
		}
	} else if s.server != nil {
		blockValue = uint32(target.Level) / 2
		critChance = creatureAuraModifierSum(s.server, creatureAuraKeyForTarget(target), spellAuraModBlockCritChance)
	}
	return blockValue, critChance
}

// shieldBlockParams bundles the player-victim inputs of the MELEE_HIT_BLOCK
// arm: getShieldBlockValue (Player::GetShieldBlockValue, Player.cpp:5426-5430)
// and the 253 aura total for Unit::isBlockCritical (Unit.cpp:2432-2437).
func (s *session) shieldBlockParams() (uint32, int32) {
	blockValue := s.getShieldBlockValue()
	var critChance int32
	for _, amount := range s.auraTypeModifiersFiltered(spellAuraModBlockCritChance, func(int32) bool { return true }) {
		critChance += amount
	}
	return blockValue, critChance
}

// applyShieldBlockValue runs the MELEE_HIT_BLOCK consumption arm
// (Unit.cpp:1340-1376): the block value doubles on a block-crit roll
// (Unit::isBlockCritical, Unit.cpp:2432-2437); a full block zeroes the damage
// and reports true so the caller sets VICTIMSTATE_BLOCKS, a partial block
// just subtracts. The spell block arm (Unit.cpp:1060-1077) shares the same
// semantics, so the ranged white path reuses this too.
func applyShieldBlockValue(blockValue uint32, critChance int32, damage uint32) (uint32, bool) {
	if critChance > 0 && rand.IntN(100) < int(critChance) {
		blockValue += blockValue
	}
	if blockValue >= damage {
		return damage, true
	}
	return blockValue, false
}

// glancingReducedDamage mirrors the MELEE_HIT_GLANCING arm of
// Unit::CalculateMeleeDamage (Unit.cpp:1377-1397): the reduction is
// irand(leveldif*10-10, leveldif*10) percent with leveldif capped at 3 —
// ~25% average vs +3-level targets, not a flat 25%.
func glancingReducedDamage(damage uint32, attackerLevel, victimLevel uint8) uint32 {
	leveldif := int32(victimLevel) - int32(attackerLevel)
	if leveldif > 3 {
		leveldif = 3
	}
	reduction := leveldif*10 - 10 + int32(rand.IntN(11))
	reduced := uint32((1.0 - float64(reduction)/100.0) * float64(damage))
	if reduced < 1 {
		reduced = 1
	}
	return reduced
}

func (s *session) executeMeleeSwing(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType) {
	if s.player == nil || s.isDeadOrGhost() || target.Health == 0 {
		return
	}
	// Unit::AttackerStateUpdate (Unit.cpp:2141): AtTargetAttacked(victim,
	// true) fires at the top of every swing — ahead of the next-swing-spell
	// slot, the redirect, and the outcome roll — so the victim engages even
	// on a miss. On a critter that engagement fires
	// CritterAI::JustEngagedWith -> flee (PassiveAI.cpp:76-79), bridged in
	// triggerCritterFlee. (The old T-press site was wrong: C++ Unit::Attack
	// (5650-5760) never engages the victim for player attackers — the
	// critter used to bolt before the first swing could land.)
	if s.server != nil && s.player != nil {
		s.server.triggerCritterFlee(ctx, target, s.player.X, s.player.Y)
	}
	// Unit::AttackerStateUpdate (Unit.cpp:2141-2142): AtTargetAttacked then
	// RemoveAurasWithInterruptFlags(AURA_INTERRUPT_FLAG_MELEE_ATTACK) fires
	// at the top of every swing, ahead of the next-swing-spell slot.
	s.removeAurasWithInterruptFlags(auraInterruptFlagMeleeAttack)
	// Unit::AttackerStateUpdate (Unit.cpp:2151-2152): "melee attack spell cast
	// at main hand attack only - no normal melee dmg dealt" — a queued
	// on-next-swing spell (CURRENT_MELEE_SPELL slot) fires in place of the
	// normal main-hand swing damage. Off-hand swings never consume it.
	if attType == protocol.BaseAttack {
		if q := s.takeNextSwingSpell(); q != nil {
			s.finishNextSwingCast(ctx, q)
			return
		}
	}
	// Unit::AttackerStateUpdate (Unit.cpp:2155-2156): "attack can be
	// redirected to another target" — the ADD_CASTER_HIT_TRIGGER (111)
	// magnet leg runs before the damage calc, after the on-next-swing
	// spell slot is consumed (a queued spell fires in place of the swing
	// with no redirect).
	if redirectGUID, ok := s.meleeHitRedirectGUID(ctx, target.GUID); ok {
		if redirected, ok := s.getCombatTarget(ctx, redirectGUID); ok && redirected.Health > 0 {
			target = redirected
		}
	}
	now := time.Now()
	var minDmg, maxDmg float32
	var attTime uint32

	if attType == protocol.OffAttack {
		s.lastOffhandSwing = now
		minDmg = s.player.MinOffhandDamage
		maxDmg = s.player.MaxOffhandDamage
		attTime = s.player.OffhandAttackTime
	} else {
		s.lastSwing = now
		minDmg = s.player.MinDamage
		maxDmg = s.player.MaxDamage
		attTime = s.player.AttackTime
	}

	damage := uint32(20 + int(s.player.Level)*5)
	// absorbedDmg carries the pre-absorb portion for the rage-from-damage
	// legs (Unit.cpp:789-924): the dealt leg uses post-absorb damage while the
	// received leg adds the absorbed amount back.
	absorbedDmg := uint32(0)
	if maxDmg > minDmg && minDmg > 0 {
		attSpeed := float64(attTime) / 1000.0
		if attSpeed <= 0 {
			attSpeed = 2.0
		}
		// Unit::CalculateMinMaxDamage (StatSystem.cpp:549-552): the AP leg
		// is GetTotalAttackPowerValue(attType)/14 * attackPowerMod, where
		// attackPowerMod = max(GetAPMultiplier, 0.25) (Unit.cpp:11034). The
		// live aura folds (99/166/268) ride totalAttackPowerValue — the
		// plain AttackPower field misses every aura-driven AP change.
		if attSpeed < 0.25 {
			attSpeed = 0.25
		}
		apBonus := (float64(s.totalAttackPowerValue(false)) * attSpeed) / 14.0
		variance := float64(maxDmg - minDmg)
		baseDmg := float64(minDmg)
		if variance > 0 {
			baseDmg += rand.Float64() * variance
		}
		damage = uint32(baseDmg + apBonus)
		// Unit::CalculateMinMaxDamage (StatSystem.cpp:553-556) with the
		// aura legs Unit::UpdateDamageDoneMods / UpdateDamagePctDoneMods
		// (Unit.cpp:9334-9416): ((weapon + AP/14) * basePct + flat13) *
		// pct79 (* pct122 offhand, and the UNIT_MOD_DAMAGE_OFFHAND 0.5
		// from Unit.cpp:353 rides the same TOTAL_PCT). Go models no damage
		// BASE_PCT (always 1.0 in C++ too — nothing writes it), so it drops
		// out. The flat and pct folds are per attack type with the
		// CheckAttackFitToAuraRequirement weapon-fit gate.
		// Unit::CalculateMeleeDamage (Unit.cpp:1236-1237) then runs
		// MeleeDamageBonusDone / MeleeDamageBonusTaken on the (offhand-halved)
		// weapon base: Done's APbonus/creature flat lands after the halving
		// and before the versus product (Unit.cpp:8061-8068), then Taken's
		// flat/pct arms (Unit.cpp:8154-8271).
		pct := s.meleeDamageDonePctMod(ctx, attType)
		if attType == protocol.OffAttack {
			// Off-hand weapon attacks suffer a 50% damage penalty by default (TrinityCore Unit.cpp:353)
			pct *= 0.5
		}
		done := (float64(damage)+float64(s.meleeDamageDoneFlat(ctx, attType)))*pct +
			float64(s.swingDoneFlatBonus(ctx, false, false, target.GUID, target))
		damage = uint32(done * s.meleeDamageDoneVersusMod(ctx, target.GUID))
		damage, _ = s.meleeWeaponDamageTaken(ctx, damage, spellSchoolMaskNormal, target.GUID, target, false, true)
	}

	// Apply target armor damage reduction
	armor := float64(target.Armor)
	if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil {
		armor = s.server.liveCreatureArmor(creatureAuraKeyForTarget(target), tm, float64(target.Armor))
	}
	if armor < 0 {
		armor = 0
	}
	if armor > 0 {
		// The 123/269 ignore-resist arms run ahead of the ArP leg on the
		// auto-swing path (Unit.cpp:1632-1650, null spellInfo).
		armor = s.ignoreTargetResistArmorSwing(armor)
		damage = calcArmorReducedDamage(armor, s.player.Level, target.Level, damage, s.getArmorPenPct(ctx, attType))
	}
	if damage < 1 {
		damage = 1
	}

	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	// Unit::RollMeleeOutcomeAgainst (Unit.cpp:2276-2279): pets can never be
	// glancing victims — Go pet motions carry OwnerGUID.
	isPetVictim := false
	if !isPlayerVictim && s.server != nil {
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil {
			isPetVictim = tm.OwnerGUID != 0
		}
	}
	outcome := protocol.MeleeHitNormal
	hitInfo := protocol.HitInfoAffectsVictim
	targetState := protocol.VictimStateHit

	isDualWielding := s.haveOffhandWeapon()
	canBlock := false
	canParry := target.Level >= 10 || isPlayerVictim
	canDodge := true
	var critReductionBP int32
	// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the victim-side
	// crit-taken arms. victimFlatCritTakenBP carries the pre-resilience
	// SPELL_AURA_MOD_ATTACKER_MELEE/RANGED_CRIT_CHANCE (187/188) total plus the
	// caster-matched SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) total;
	// victimSpellWeaponCritBP carries the post-resilience
	// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197) total.
	var victimFlatCritTakenBP, victimSpellWeaponCritBP int32
	victimDodgeBP := int32(-1)
	// The attacker's melee crit bonus (GetUnitCriticalChanceDone's player arm,
	// Unit.cpp:2823-2834) folds into the resilience base below: C++ applies
	// ApplyResilience to critDone + the 187/308 flats (Unit.cpp:2871).
	critBonusBP := int32(math.Round(s.getMeleeCritPct() * 100))
	// Unit::GetUnitParryChance / Unit::GetUnitBlockChance read the victim's
	// PLAYER_PARRY_PERCENTAGE / PLAYER_BLOCK_PERCENTAGE (Unit.cpp:2712/2788);
	// -1 keeps the legacy 5%-base fallback.
	victimParryBP := int32(-1)
	victimBlockBP := int32(-1)
	var vicSess *session
	if isPlayerVictim && s.server != nil {
		vicSess = s.server.findSessionByGUID(target.GUID)
		if vicSess != nil && vicSess.player != nil {
			canBlock = vicSess.player.CanBlock && vicSess.player.Block > 0
			// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2861-2875): the
			// victim's SPELL_AURA_MOD_ATTACKER_MELEE_CRIT_CHANCE (187) total
			// and the caster-matched SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER
			// (308) total join the resilience base; resilience reduces
			// critDone + those flats, and
			// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197)
			// applies after it.
			victimFlatCritTakenBP = int32(math.Round(float64(vicSess.playerAuraModifier(spellAuraModAttackerMeleeCritChance))*100)) +
				int32(math.Round(vicSess.victimCritChanceForCaster(s.playerGUID)*100))
			resilBaseBP := int32(500) + critBonusBP + victimFlatCritTakenBP
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenMelee, &resilBaseBP)
			critReductionBP = int32(500) + critBonusBP + victimFlatCritTakenBP - resilBaseBP
			victimSpellWeaponCritBP = int32(math.Round(float64(vicSess.playerAuraModifier(spellAuraModAttackerSpellAndWeaponCritChance)) * 100))
			// Gt-based dodge (Unit::GetUnitDodgeChance reads the victim's
			// PLAYER_DODGE_PERCENTAGE, Unit.cpp:2667).
			victimDodgeBP = int32(math.Round(float64(vicSess.player.DodgePercentage) * 100))
			victimParryBP = int32(math.Round(float64(vicSess.player.ParryPercentage) * 100))
			victimBlockBP = int32(math.Round(float64(vicSess.player.BlockPercentage) * 100))
		}
	}

	// Unit::GetUnitParryChance / Unit::GetUnitBlockChance (Unit.cpp:2708/2777):
	// the victim's own SPELL_AURA_MOD_PARRY_PERCENT (47) /
	// SPELL_AURA_MOD_BLOCK_PERCENT (51) totals add to the 5% base. A player
	// victim's fold is read here; the creature-victim fold lands below, once
	// the front-arc resolution is known.
	var victimParryBonusBP, victimBlockBonusBP, victimDodgeBonusBP int32
	if isPlayerVictim && vicSess != nil {
		victimParryBonusBP = vicSess.totalAuraModifier(spellAuraModParryPercent) * 100
		victimBlockBonusBP = vicSess.totalAuraModifier(spellAuraModBlockPercent) * 100
	}

	// Positional defense checks (TrinityCore Unit::RollMeleeOutcomeAgainst):
	// A defender can only parry or block attacks from within their front 180° arc (M_PI).
	// Player victims cannot dodge attacks from behind. (NPCs can dodge from behind).
	// SPELL_AURA_IGNORE_HIT_DIRECTION (288) exempts the victim from the
	// behind-arc kill (Unit.cpp:2213: canParryOrBlock carries the HasAuraType
	// arm; player-victim dodge from behind rides canParryOrBlock).
	attackerInFront := hasInArc(target.Orientation, target.X, target.Y, s.player.X, s.player.Y, math.Pi)
	ignoreHitDirection := false
	if isPlayerVictim && vicSess != nil {
		ignoreHitDirection = vicSess.hasAuraType(spellAuraIgnoreHitDirection)
	} else if !isPlayerVictim && s.server != nil && s.player != nil {
		key := creatureAuraKeyForPlayer(*s.player, target.GUID)
		s.server.auraMu.Lock()
		for _, aura := range s.server.activeCreatureAuras[key] {
			if aura != nil && !aura.Stopped && aura.AuraType == spellAuraIgnoreHitDirection {
				ignoreHitDirection = true
				break
			}
		}
		s.server.auraMu.Unlock()
	}
	if !attackerInFront && !ignoreHitDirection {
		canBlock = false
		canParry = false
		if isPlayerVictim {
			canDodge = false
		}
	}

	// Creature victims parry (5% base + 47) and block (5% base + 51) when the
	// attacker is in the front arc (Unit::RollMeleeOutcomeAgainst,
	// Unit.cpp:2213-2232) — unless the template carries
	// CREATURE_FLAG_EXTRA_NO_PARRY (0x4) / NO_BLOCK (0x10). Totems never
	// avoid. The behind-arc kill above already handled the arc leg for parry.
	if !isPlayerVictim && s.server != nil {
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil && !s.server.isTotemGUID(target.GUID) {
			victimKey := creatureAuraKeyForTarget(target)
			if tm.FlagsExtra&0x00000004 != 0 {
				canParry = false
			} else {
				victimParryBonusBP = s.server.creatureAuraTypeSum(victimKey, spellAuraModParryPercent) * 100
			}
			// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the
			// victim's SPELL_AURA_MOD_ATTACKER_MELEE_CRIT_CHANCE (187) total
			// and caster-matched SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308)
			// total are pre-resilience; C++ ApplyResilience no-ops for a
			// creature victim without a player owner, so no resilience here;
			// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197)
			// applies after resilience.
			victimFlatCritTakenBP = creatureAuraModifierSum(s.server, victimKey, spellAuraModAttackerMeleeCritChance)*100 +
				int32(math.Round(creatureVictimCritChanceForCaster(s.server, victimKey, s.playerGUID)*100))
			victimSpellWeaponCritBP = creatureAuraModifierSum(s.server, victimKey, spellAuraModAttackerSpellAndWeaponCritChance) * 100
			// Unit::GetUnitDodgeChance (Unit.cpp:2676-2681): the victim's
			// SPELL_AURA_MOD_DODGE_PERCENT (49) total adds to the 5% base for
			// creature victims (totems gated above, like the parry arm).
			victimDodgeBonusBP = s.server.creatureAuraTypeSum(victimKey, spellAuraModDodgePercent) * 100
			if (attackerInFront || ignoreHitDirection) && tm.FlagsExtra&0x00000010 == 0 {
				canBlock = true
				victimBlockBonusBP = s.server.creatureAuraTypeSum(victimKey, spellAuraModBlockPercent) * 100
			}
		}
	}
	// Unit::GetUnitDodgeChance (Unit.cpp:2676-2681): the whole creature arm
	// is gated on !victim->IsTotem() — totems never dodge.
	if !isPlayerVictim && s.server != nil && s.server.isTotemGUID(target.GUID) {
		canDodge = false
	}

	// A victim mid cast or CC'd (confused/stunned/fleeing) cannot
	// dodge/parry/block (Unit.cpp:2238-2242:
	// victim->IsNonMeleeSpellCast(false) || victim->HasUnitState(UNIT_STATE_CONTROLLED);
	// nonMeleeCastInProgress is the Go analog of the cast half — cast-bar
	// and channeled (autorepeat has no Go model, stays unbridged) — and the
	// CONTROLLED half bridges via the 5/7/12 aura twin (victimIsControlled).
	// Creature victims bridge the cast half via motion.CastingUntil.
	if isPlayerVictim && vicSess != nil && (vicSess.nonMeleeCastInProgress() || victimIsControlled(s.server, true, vicSess, target)) {
		canDodge = false
		canParry = false
		canBlock = false
	}
	if !isPlayerVictim && s.server != nil {
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil && (time.Now().Before(tm.CastingUntil) || victimIsControlled(s.server, false, nil, target)) {
			canDodge = false
			canParry = false
			canBlock = false
		}
	}

	if s.player.Level > 0 {
		hitBonusBP := int32(math.Round(s.getMeleeHitPct() * 100))
		expertiseBP := int32(math.Round(s.getExpertiseDodgeParryReductionPct(ctx, attType) * 100))
		// SPELL_AURA_MOD_HIT_CHANCE (54) on the attacker and
		// SPELL_AURA_MOD_ATTACKER_MELEE_HIT_CHANCE (184) on the victim fold
		// into the miss roll at percent scale (Unit::MeleeSpellMissChance,
		// Unit.cpp:12425).
		hitBonusBP += s.totalAuraModifier(spellAuraModHitChance) * 100
		hitBonusBP += victimAttackerHitChanceBP(s, target.GUID, isPlayerVictim, spellAuraModAttackerMeleeHitChance)
		// The attacker's SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248) dodge
		// reduction (Unit.cpp:2694-2695) and the ungated
		// SPELL_AURA_MOD_ENEMY_DODGE (251) aura sum (Unit.cpp:2697-2698) —
		// the same arms meleeSpellHitResult carries for melee spells.
		dodgeReductionBP := (s.totalAuraModifierByMiscValue(spellAuraModCombatResultChance, victimStateDodge) + s.totalAuraModifier(spellAuraModEnemyDodge)) * 100
		// Unit::RollMeleeOutcomeAgainst's sitting-victim arm (Unit.cpp:2246-2247):
		// a non-standing player victim always takes a crit when the attacker
		// has nonzero crit chance. player.StandState is the Go analog of
		// UNIT_FIELD_BYTES_1 stand state (0 = stand).
		victimSitting := isPlayerVictim && vicSess != nil && vicSess.player != nil && vicSess.player.StandState != 0
		outcome, hitInfo, targetState = rollMeleeOutcome(s.player.Level, target.Level, true, isPlayerVictim, isPetVictim, false, true, victimSitting, isDualWielding, canBlock, canParry, canDodge, critReductionBP, hitBonusBP, critBonusBP, expertiseBP, victimDodgeBP, dodgeReductionBP, expertiseBP, victimParryBonusBP, victimBlockBonusBP, victimParryBP, victimBlockBP, victimDodgeBonusBP, victimFlatCritTakenBP, victimSpellWeaponCritBP, 0)
	}
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			if vicSess.isImmuneToDamage(1) {
				outcome = protocol.MeleeHitImmune
				hitInfo = protocol.HitInfoMiss
				targetState = protocol.VictimStateIsImmune
			}
		}
	} else if !isPlayerVictim && s.server != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, target.GUID) {
		outcome = protocol.MeleeHitEvade
		hitInfo = protocol.HitInfoMiss
		targetState = protocol.VictimStateEvades
	}
	if attType == protocol.OffAttack {
		hitInfo |= protocol.HitInfoOffHand
	}
	blocked := uint32(0)

	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitBlock:
		// Unit::CalculateMeleeDamage MELEE_HIT_BLOCK arm (Unit.cpp:1340-1376):
		// the victim's shield block value, doubled on a block crit, consumed
		// against the swing; a full block zeroes damage and reports
		// VICTIMSTATE_BLOCKS.
		blockValue, critChance := s.swingBlockValue(target, isPlayerVictim)
		var fullBlock bool
		blocked, fullBlock = applyShieldBlockValue(blockValue, critChance, damage)
		damage -= blocked
		if fullBlock {
			targetState = protocol.VictimStateBlocks
		}
	case protocol.MeleeHitCrit:
		damage *= 2
		// Unit::CalculateMeleeDamage crit leg (Unit.cpp:1302-1319): the
		// 203/204 + 163 + 169 arms AddPct after the doubling, ahead of the
		// resilience reduction below.
		damage = s.meleeCritDamageBonus(ctx, damage, target.GUID, target, false)
	case protocol.MeleeHitGlancing:
		// Unit::CalculateMeleeDamage glancing leg (Unit.cpp:1377-1397).
		damage = glancingReducedDamage(damage, s.player.Level, target.Level)
	case protocol.MeleeHitCrushing:
		damage = uint32(float64(damage) * 1.5)
	}

	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			vicSess.applyResilienceToDamage(true, &damage, outcome == protocol.MeleeHitCrit, CombatRatingCritTakenMelee)
			if damage > 0 {
				// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): the attacker's
				// MOD_TARGET_ABSORB_SCHOOL (194) pct of damage bypasses absorbs.
				bypass := absorbIgnoreBypass(damage, s.absorbIgnorePct(1))
				absorbed, remaining := vicSess.applyAbsorptionShields(damage-bypass, 1)
				absorbedDmg = absorbed
				damage = remaining + bypass
				if remaining == 0 && absorbed > 0 {
					hitInfo |= protocol.HitInfoFullAbsorb
				} else if absorbed > 0 {
					hitInfo |= protocol.HitInfoPartialAbsorb
				}
			}
		}
	} else if !isPlayerVictim && s.server != nil && damage > 0 {
		// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): the attacker's
		// MOD_TARGET_ABSORB_SCHOOL (194) pct of damage bypasses absorbs.
		bypass := absorbIgnoreBypass(damage, s.absorbIgnorePct(1))
		absorbed, remaining := s.server.applyCreatureAbsorptionShields(creatureAuraKeyForTarget(target), damage-bypass, 1)
		damage = remaining + bypass
		if remaining == 0 && absorbed > 0 {
			hitInfo |= protocol.HitInfoFullAbsorb
		} else if absorbed > 0 {
			hitInfo |= protocol.HitInfoPartialAbsorb
		}
	}

	// Unit::DealDamage (Unit.cpp:789-813): rage from damage dealt — direct
	// weapon damage only, attacker POWER_RAGE, attacker != victim. Ranged
	// attacks never grant it (Unit.cpp:806-807, RANGED_ATTACK breaks with no
	// RewardRage). weaponSpeedHitFactor = attackTime/1000 * 3.5 (main) or 1.75
	// (offhand), doubled on crit. Miss/dodge/parry outcomes grant nothing
	// (C++ never reaches DealDamage for them); a fully absorbed hit still
	// grants the factor leg, matching C++ (the leg is not damage-gated).
	switch outcome {
	case protocol.MeleeHitNormal, protocol.MeleeHitCrit, protocol.MeleeHitGlancing, protocol.MeleeHitCrushing, protocol.MeleeHitBlock:
		if playerPowerType(s.player) == powerRage {
			factor := uint32(float64(attTime) / 1000.0 * 3.5)
			if attType == protocol.OffAttack {
				factor = uint32(float64(attTime) / 1000.0 * 1.75)
			}
			if outcome == protocol.MeleeHitCrit {
				factor *= 2
			}
			if pts := rewardRagePoints(uint32(s.player.Level), damage, factor, true, s.rageFromDamageDealtPct(), false); pts > 0 {
				s.adjustSpellPower(ctx, s.playerGUID, powerRage, int64(pts))
			}
		}
	}

	// Handle parry haste: if defender parried, haste defender's next attack!
	// Matching TrinityCore Unit.cpp:1480-1510: the 20%/60% bands come from
	// the pristine swing (GetAttackTime divides the scaled field by
	// m_modAttackSpeedPct, which ApplyAttackTimePercentMod scales
	// identically), while the remaining time is the live aura-scaled timer
	// (GetAttackTime*m). So bands stay pristine; only the remaining uses
	// the victim's hasted swing.
	if targetState == protocol.VictimStateParry && s.server != nil {
		if playerVictim := s.server.findSessionByGUID(target.GUID); playerVictim != nil && playerVictim.player != nil {
			vMainSpeed := time.Duration(playerVictim.player.AttackTime) * time.Millisecond
			if vMainSpeed <= 0 {
				vMainSpeed = 2 * time.Second
			}
			vSwing := playerVictim.getHastedMeleeSpeed(vMainSpeed, false)
			elapsed := now.Sub(playerVictim.lastSwing)
			if elapsed < vSwing {
				rem := vSwing - elapsed
				hasted := calcParryHastedRemaining(rem, vMainSpeed)
				playerVictim.lastSwing = now.Add(-(vSwing - hasted))
			}
		} else {
			s.server.motionMu.Lock()
			if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID); motion != nil {
				if motion.FlagsExtra&creatureFlagExtraNoParryHasten == 0 {
					cSpeed := time.Duration(motion.AttackTime) * time.Millisecond
					if cSpeed <= 0 {
						cSpeed = 2 * time.Second
					}
					cSwing := time.Duration(float64(cSpeed) * s.server.creatureAttackTimeFactor(creatureAuraKeyForMotion(motion)))
					elapsed := now.Sub(motion.LastAttack)
					if elapsed < cSwing {
						rem := cSwing - elapsed
						hasted := calcParryHastedRemaining(rem, cSpeed)
						motion.LastAttack = now.Add(-(cSwing - hasted))
					}
				}
			}
			s.server.motionMu.Unlock()
		}
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	asuPayload := protocol.BuildAttackerStateUpdate(s.playerGUID, target.GUID, damage, overkill, hitInfo, targetState, blocked)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPayload, true)
	if s.server != nil {
		// Unit::SendAttackStateUpdate (Unit.cpp): SendMessageToSet on the
		// attacker — the attacker's visibility set.
		s.server.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPayload, s.player.Map, s.player.InstanceID, s.player.X, s.player.Y, s.player.Z, s)
	}

	// Attacker enters combat on swing
	if s.player != nil && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
	}
	s.lastCombatTime = time.Now()

	// Trigger weapon enchantment and trinket procs on hit (TrinityCore Unit::ProcDamageAndSpellFor)
	s.procWeaponEnchantments(ctx, target, attType, outcome)
	s.procItemAndTrinketEffects(ctx, target, attType, outcome)
	s.procAuraTriggers(ctx, target, attType, outcome, hitInfo, targetState, blocked, damage)

	// Unit::ProcSkillsAndReactives (Unit.cpp:10280-10351): dodge/parry/block
	// avoidance arms the victim's reactive aura states (Revenge/Counterattack
	// usability), and a warrior's dodged swing banks the Overpower combo
	// point + 4000ms window.
	if vicSess != nil {
		vicSess.procReactiveVictimArms(outcome)
	}
	s.procReactiveAttackerArms(target.GUID, outcome)

	if s.server != nil {
		s.server.triggerPetDefensive(ctx, s.player.Map, s.player.InstanceID, s.playerGUID, target.GUID, scripting.CreatureEventOnOwnerAttacked)
	}

	// If target is an online player (e.g. duel opponent or PvP)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil && playerSess.player != nil {
			// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
			// before the lethal / non-lethal legs (duel defeat included).
			// shieldGateDamage keeps the pre-god value: C++ DealMeleeDamage
			// gates the damage-shield block on damageInfo->Damages (post-
			// absorb, pre-god — the god early-return never zeroes Damages),
			// so a god-mode victim's shields still strike (Unit.cpp:1555).
			shieldGateDamage := damage
			damage = playerSess.negateGodModeDamage(damage)
			// Unit::DealDamage (Unit.cpp:815-819): rage from fully absorbed
			// damage — the god arm above returns before this leg in C++.
			if damage == 0 && absorbedDmg > 0 && !playerSess.godCheatActive() {
				playerSess.grantRageFromDamageTaken(ctx, absorbedDmg)
				// Unit::DealDamage (Unit.cpp:742-747): absorbed damage still
				// strips TAKE_DAMAGE-interrupt auras — the removal runs before
				// the !damage early-return.
				playerSess.procDamageAuras(false)
				// Unit::DealDamage (Unit.cpp:749-761): fully absorbed non-DoT
				// damage aborts ABORT_ON_DMG casts (attacker != victim holds —
				// melee vs another player).
				playerSess.interruptAbsorbedCast()
			}
			if playerSess.player.UnitFlags&unitFlagInCombat == 0 {
				playerSess.player.UnitFlags |= unitFlagInCombat
			}
			playerSess.lastCombatTime = time.Now()
			// Victim-side aura procs on the melee event (TrinityCore
			// Unit.cpp:1194-1198 — ProcVictim = PROC_FLAG_TAKEN_MELEE_AUTO_ATTACK),
			// before the victim-side damage application, mirroring C++ proc
			// ordering; the trigger spell targets the attacker.
			playerSess.procVictimAuraTriggers(ctx, s.playerGUID, outcome, hitInfo, targetState, blocked, damage)
			if damage > 0 {
				// Unit::CalcAbsorbResist (Unit.cpp:1977-2060):
				// SPELL_AURA_SPLIT_DAMAGE (81/153) redirects part of the
				// damage to the aura's caster. Runs inside DealDamage, ahead
				// of the health reduction below.
				damage = s.server.splitDamageToCaster(ctx, target.GUID, true, creatureAuraKey{}, s.playerGUID, damage, 1)
				// Unit::DealDamage (Unit.cpp:766-788):
				// SPELL_AURA_SHARE_DAMAGE_PCT copies CalculatePct(damage,
				// amount) to the aura's caster. Runs inside DealDamage, ahead
				// of the health reduction below.
				s.splitShareDamagePct(ctx, target.GUID, true, creatureAuraKey{}, s.playerGUID, damage, 1)
				// Unit::DealDamage (Unit.cpp:855-877): the arena damage score
				// and the killer achievement arms (DAMAGE_DONE capped at the
				// victim's pre-damage health — no overkill credit, and
				// HIGHEST_HIT_DEALT) fire on any player-attacker damage, plus
				// the victim-side HIGHEST_HIT_RECEIVED arm.
				s.server.updateArenaDamageScore(s, damage)
				victimHealth := playerSess.player.Health
				s.updateAchievementCriteria(criteriaTypeDamageDone, 0, min(damage, victimHealth))
				s.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)
				playerSess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
				// Duel defeat (Unit.cpp:825-853, 957-973): any damage >=
				// health-1 on a duelist ends the duel — the clamped hit
				// leaves the loser at 1 HP.
				if duelDefeatOnDamage(playerSess, s.playerGUID, damage, victimHealth) {
					playerSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, victimHealth-1)
				} else if victimHealth > 0 && damage+1 >= victimHealth {
					playerSess.player.Health = 0
					playerSess.sendPlayerUpdate()
					playerSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, victimHealth)
					s.server.creditHonorableKill(s, playerSess)
					// Unit::Kill (Unit.cpp:11341-11343): the attacker is a player.
					playerSess.killPlayer(ctx, s, true, true)
					// Eluna PLAYER_EVENT_ON_KILL_PLAYER (6): Unit::Kill tail
					// (Unit.cpp:11466-11480), attacker and victim players.
					s.server.firePVPKillHook(ctx, s, playerSess)
					// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill
					// pet arm — attacker is the player (player = attacker
					// itself), so only the attacker's live pet gets
					// KilledUnit(victim) (Unit.cpp:11324-11335).
					if pet := s.livePetMotion(); pet != nil {
						s.server.fireCreatureTargetDied(ctx, pet, playerSess.luaPlayer())
					}
					_ = s.sendAttackStop(target.GUID, true)
					s.attackTarget = 0
				} else {
					playerSess.player.Health -= damage
					// Unit::DealDamage (Unit.cpp:915-924): rage from damage
					// received — damage + absorbed for the conversion.
					playerSess.grantRageFromDamageTaken(ctx, damage+absorbedDmg)
					// Unit::DealDamage (Unit.cpp:906-913, 925-931): random
					// durability loss — HIT TAKEN on the player victim and
					// HIT DONE on the player attacker, rolled independently.
					playerSess.rollDurabilityLossOnHit(ctx, damage)
					s.rollDurabilityLossOnHit(ctx, damage)
					playerSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, damage)
					playerSess.delayCurrentCast()
					playerSess.delayCurrentChannel()
					playerSess.procDamageAuras(true, damage)
					playerSess.sendPlayerUpdate()
				}
			}
			// Unit::DealMeleeDamage (Unit.cpp:1555-1600): the victim's
			// SPELL_AURA_DAMAGE_SHIELD (15) effects strike the attacker
			// after a damaging melee hit. Gated on the pre-god post-absorb
			// damage (see shieldGateDamage above): C++ checks
			// Damages[0]+Damages[1] after DealDamage ran CalcAbsorbResist,
			// so a fully absorbed hit triggers nothing, but god-mode
			// negation does not stop the shields.
			if shieldGateDamage > 0 {
				s.server.applyDamageShieldRetaliation(ctx,
					damageShieldSide{sess: playerSess, isPlayer: true},
					damageShieldSide{sess: s, isPlayer: true},
					shieldGateDamage)
			}
			return
		}
	}

	if damage == 0 {
		if s.server != nil && (isPlayerVictim || !s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, target.GUID)) {
			s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
		}
		return
	}

	// Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9): fires before damage apply;
	// handlers may rewrite damage via the second return (Unit::DealDamage,
	// Unit.cpp:697-702). The damage==0 miss/dodge arm above already
	// returned, matching C++ skipping DealDamage when damage, absorb and
	// resist are all zero (Unit.cpp:1513); Go has no creature melee absorb
	// model, so the remaining zero-damage cases are exactly the skipped ones.
	if s.server != nil {
		if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, target.GUID); motion != nil {
			damage = s.server.fireCreatureDamageTaken(ctx, motion, s.luaPlayer(), damage)
		}
	}

	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	if damage > 0 {
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060):
		// SPELL_AURA_SPLIT_DAMAGE (81/153) redirects part of the damage to
		// the aura's caster, ahead of the 300 share leg.
		damage = s.server.splitDamageToCaster(ctx, target.GUID, false, creatureAuraKeyForTarget(target), s.playerGUID, damage, 1)
		s.splitShareDamagePct(ctx, target.GUID, false, creatureAuraKeyForTarget(target), s.playerGUID, damage, 1)
	}

	// Unit::DealDamage (Unit.cpp:699-704): the AI hooks fire at the top of
	// DealDamage — ahead of the lethal leg — on every path, including killing
	// blows. Fired under motionMu: the hook arms (vancleefAI) mutate motion
	// state directly. The lethal leg decides on the post-hook damage against
	// the locked motion health (the pre-lock snapshot is stale once hooks
	// run); when the motion is gone the snapshot decides, as before.
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
	if motion != nil {
		if motion.BossAI == nil {
			motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
		}
		fireDealDamageAIHooks(ctx, s.server, motion, nil, s.playerGUID, motion.GUID, &damage)
	}
	victimHealth := target.Health
	if motion != nil {
		victimHealth = motion.Health
	}
	// Unit::DealDamage vehicle-mounted duel leg (Unit.cpp:837-853): a blow
	// that would leave a ridden vehicle at health-1 ends the rider's mounted
	// duel instead — the hit is consumed, skipping the health legs below.
	if motion != nil && duelDefeatOnVehicleDamage(s, motion, s.playerGUID, damage, victimHealth) {
		s.server.motionMu.Unlock()
		return
	}
	if damage >= victimHealth {
		// Target dies
		corpseFlags := unitDynFlagLootable
		if motion != nil {
			// Unit::DealDamage tap block (Unit.cpp:872-876): the killing
			// blow taps and lowers the damage requirement when untapped.
			s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, motion.Health, true)
			s.server.clearInstanceEncounter(motion)
			motion.Health = 0
			motion.DynamicFlags |= unitDynFlagLootable
			// The corpse keeps TAPPED when the tap survived to death
			// (C++ clears it only on a failed damage requirement,
			// Unit.cpp:11172-11175), so broadcast the real flags.
			corpseFlags = motion.DynamicFlags
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		// Deferred Eluna summon hooks queued by boss OnDamageTaken (e.g.
		// VanCleef's 50% summon arm): the fire must run after the unlock
		// since Lua handler methods lock motionMu on demand. Queued hooks
		// fire on the lethal path too — C++ runs DamageTaken ahead of the
		// lethal leg, so a killing blow still triggers them.
		if motion != nil {
			s.server.drainBossSummonHooks(ctx, motion, motion.BossAI)
		}

		s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: corpseFlags,
		})
		s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
		_ = s.sendAttackStop(target.GUID, true)
		s.attackTarget = 0
		s.onCreatureKilled(ctx, target, nil)
		// Eluna PLAYER_EVENT_ON_KILL_CREATURE (7): Unit::Kill tail
		// (Unit.cpp:11466-11480), attacker is the player.
		if s.server != nil {
			if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
				s.server.fireCreatureKillHook(ctx, s, motion)
			}
		}
		s.debug("target slain by auto-attack", "account", s.accountName, "guid", target.GUID)
	} else {
		newHealth := victimHealth - damage
		// Unit::DealDamage (Unit.cpp:925-931): random durability loss on
		// HIT DONE — the attacker is the player.
		s.rollDurabilityLossOnHit(ctx, damage)
		rageChanged := false
		var rageNext, rageMapID, rageInstanceID uint32
		var rageGUID uint64
		newlyTapped := false
		tappedFlags := uint32(0)
		if motion != nil {
			// Unit::DealDamage (Unit.cpp:701-704): the AI hooks fired above
			// on the mutable damage ahead of the tap/health/threat/rage
			// legs; a hook-adjusted damage flows into all of them. Threshold
			// arms (VanCleef's HealthBelowPct quotes) therefore evaluate the
			// pre-hit health, as in C++.
			newHealth = motion.Health - damage
			// Unit::DealDamage tap block (Unit.cpp:872-876): first
			// player-attributed hit claims the loot rights, every hit
			// lowers the damage requirement.
			if s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, motion.Health, true) {
				newlyTapped = true
				tappedFlags = motion.DynamicFlags
			}
			motion.Health = newHealth
			// Unit::DealDamage (Unit.cpp:900-907): the evade leash's
			// last-damaged stamp — direct damage with damage > 0 resets it;
			// DoT ticks never stamp (the tick funnel skips this), and
			// player-owned victims (minions) are excluded from the arm.
			// The damage-shield-aura exclusion is vacuous: Go models no
			// shield-retaliation damage funnel.
			if damage > 0 && motion.OwnerGUID == 0 {
				motion.LastDamaged = now
			}
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received
			// (creature victims; creatures have no absorbed-damage model here).
			if next, changed := s.server.addCreatureRageLocked(motion, motion.Level, damage); changed {
				rageChanged, rageNext = true, next
				rageMapID, rageInstanceID, rageGUID = motion.Map, motion.InstanceID, motion.GUID
			}
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(motion)
			}
			dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
			inMelee := inMeleeThreatRange(motion.CombatReach, s.player.CombatReach, dist)
			threat := float32(damage) * s.getThreatMultiplier(1)
			// Unit::DealDamage (Unit.cpp:906) calls AddThreat with default
			// args (ignoreRedirects=false): the caster's redirect registry
			// applies to damage threat. The whole arm is skipped for units
			// that cannot have a threat list — ThreatManager::AddThreat's
			// !CanHaveThreatList() early leg (ThreatManager.cpp:328-339)
			// runs before redirect consumption in C++, so the redirect
			// split and the victim switch are gated here too; combat state
			// alone is kept (the aggro path below sets InCombat).
			if motion.ThreatMgr.OwnerCanHaveThreatList() {
				threat, rSwitched, rVictim := s.splitThreatRedirects(motion, threat)
				switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
				if rSwitched {
					switched, newVictim = true, rVictim
				}
				if switched && newVictim != motion.TargetGUID {
					motion.TargetGUID = newVictim
					entries := motion.ThreatMgr.SortedEntries()
					s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
				}
			}
		}
		s.server.motionMu.Unlock()

		// Deferred Eluna summon hooks queued by boss OnDamageTaken (e.g.
		// VanCleef's 50% summon arm): the fire must run after the unlock
		// since Lua handler methods lock motionMu on demand.
		if motion != nil {
			s.server.drainBossSummonHooks(ctx, motion, motion.BossAI)
		}

		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth: newHealth,
		})
		if rageChanged {
			s.server.broadcastCreatureValuesUpdateInInstance(rageMapID, rageInstanceID, rageGUID, map[int]uint32{
				unitFieldPower1 + powerRage: rageNext,
			})
		}
		if newlyTapped {
			// New tap: the client grays the name via UNIT_DYNFLAG_TAPPED.
			s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
				unitFieldDynamicFlags: tappedFlags,
			})
		}
		s.server.procCreatureDirectDamageAuras(creatureAuraKeyForTarget(target), 0, damage, target.MaxHealth)
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
		// Unit::DealMeleeDamage (Unit.cpp:1555-1600): the creature victim's
		// SPELL_AURA_DAMAGE_SHIELD (15) effects strike the attacker after a
		// damaging melee hit (post-absorb damage; no god arm on this path).
		if damage > 0 {
			s.server.applyDamageShieldRetaliation(ctx,
				damageShieldSide{key: creatureAuraKeyForTarget(target)},
				damageShieldSide{sess: s, isPlayer: true},
				damage)
		}
	}
}

// takeAmmo mirrors Spell::TakeAmmo (Spell.cpp:4875-4909): it consumes one
// count of ammo for a ranged-attack spell and reports (consumed, outOfAmmo).
// The attack-type gate (m_attackType != RANGED_ATTACK, Spell.cpp:4880) is the
// C++-exact predicate DmgClass == SPELL_DAMAGE_CLASS_RANGED (Go
// spell.DefenseType, SpellInfo.cpp:856) plus IsRangedWeaponSpell — so a
// channeled hunter-family spell with DmgClass NONE (e.g. Tame Beast) consumes
// nothing, matching C++. The player-only arm is vacuous (session casters are
// players). The weapon legs use Player::GetWeaponForAttack(RANGED_ATTACK) =
// EQUIPMENT_SLOT_RANGED (17), following the checkSpellRangedWeaponCast slot
// query: no weapon, a broken weapon (Item::IsBroken, Item.h:105), or a wand
// all return early with no consumption (Spell.cpp:4884-4887). Thrown weapons
// (InventoryType INVTYPE_THROWN, matched here by subclass) lose one count
// when stackable, or one durability point when Stackable == 1
// (ItemTemplate::GetMaxStackSize == 1, ItemTemplate.h:686-689; C++
// DurabilityPointLossForEquipSlot). Otherwise PLAYER_AMMO_ID's item loses one
// count (outOfAmmo when the last count goes or the item is gone, mirroring
// the CheckItems empty-ammo PLAYER_AMMO_ID = 0 leg at Spell.cpp:7174).
func (s *session) takeAmmo(ctx context.Context, spell wotlk.Spell) (consumed, outOfAmmo bool) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false, false
	}
	if spell.DefenseType != spellDamageClassRanged || !spellIsRangedWeaponSpell(spell) {
		return false, false
	}
	cdb := s.server.CharactersStore.DB
	var instanceGUID, itemEntry int64
	if err := cdb.QueryRowContext(ctx,
		`SELECT ii.guid, ii.itemEntry FROM character_inventory ci
		JOIN item_instance ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot = ? LIMIT 1`,
		s.playerGUID, int64(equipSlotRanged)).Scan(&instanceGUID, &itemEntry); err != nil || itemEntry <= 0 {
		return false, false
	}
	weapon, ok := s.server.getItemStoreTemplateInfo(ctx, uint32(itemEntry))
	if !ok {
		return false, false
	}
	var durability int64
	if err := cdb.QueryRowContext(ctx,
		`SELECT COALESCE(durability, 0) FROM item_instance WHERE guid = ? LIMIT 1`,
		instanceGUID).Scan(&durability); err == nil && weapon.MaxDurability > 0 && durability == 0 {
		return false, false
	}
	if weapon.SubClass == itemSubclassWeaponWand {
		return false, false
	}
	if weapon.SubClass == itemSubclassWeaponThrown {
		var count int64
		if err := cdb.QueryRowContext(ctx,
			`SELECT COALESCE(count, 0) FROM item_instance WHERE guid = ? LIMIT 1`,
			instanceGUID).Scan(&count); err != nil || count <= 0 {
			return false, true
		}
		if weapon.Stackable == 1 {
			// Player::DurabilityPointsLoss (Player.cpp:4960): clamped at
			// zero; a no-durability item (max 0, current 0) is a silent
			// no-op — the C++ broken-item arm above already handled
			// MaxDurability > 0 && current == 0.
			if durability == 0 {
				return false, false
			}
			_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET durability = ? WHERE guid = ?", durability-1, instanceGUID)
			_ = s.sendInventoryItems(ctx)
			s.sendPlayerUpdate()
			return true, false
		}
		if count <= 1 {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", instanceGUID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", instanceGUID)
			s.adjustQuestItemCount(ctx, uint32(itemEntry), 1, false)
			return true, true
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - 1 WHERE guid = ?", instanceGUID)
		s.adjustQuestItemCount(ctx, uint32(itemEntry), 1, false)
		return true, false
	}
	ammoEntry := s.player.AmmoID
	if ammoEntry == 0 {
		return false, true
	}
	var itemGUID, count int64
	err := cdb.QueryRowContext(ctx, "SELECT ii.guid, ii.count FROM character_inventory ci JOIN item_instance ii ON ci.item = ii.guid WHERE ci.guid = ? AND ii.itemEntry = ? AND ii.count > 0 LIMIT 1", s.playerGUID, ammoEntry).Scan(&itemGUID, &count)
	if err != nil {
		// The ammo row is gone (mirrors the CheckItems empty-ammo
		// PLAYER_AMMO_ID = 0 leg, Spell.cpp:7173); clear the id AND the
		// derived DPS like the last-count arm below — otherwise the stale
		// AmmoDPS bonus survives with no ammo equipped.
		s.player.AmmoID = 0
		s.player.AmmoDPS = 0
		_ = s.calculatePlayerStats(ctx, s.player)
		s.sendPlayerUpdate()
		return false, true
	}
	if count <= 1 {
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", itemGUID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
		s.adjustQuestItemCount(ctx, ammoEntry, 1, false)
		s.player.AmmoID = 0
		// Player::RemoveAmmo (Player.cpp:12118-12126) clears the derived
		// m_ammoDPS alongside PLAYER_AMMO_ID — same as the empty-row arm
		// above; otherwise the stale AmmoDPS bonus survives the last shot.
		s.player.AmmoDPS = 0
		_ = s.calculatePlayerStats(ctx, s.player)
		s.sendPlayerUpdate()
		return true, true
	}
	_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET count = count - 1 WHERE guid = ?", itemGUID)
	s.adjustQuestItemCount(ctx, ammoEntry, 1, false)
	return true, false
}

func (s *session) executeRangedAttack(ctx context.Context, target combatTarget, spellID uint32) {
	if s.player == nil || s.isDeadOrGhost() || target.Health == 0 {
		return
	}
	// Unit::AttackerStateUpdate (Unit.cpp:2141): AtTargetAttacked(victim,
	// true) fires ahead of the ranged early-out (Unit.cpp:2144) too — a
	// shot engages the victim just like a melee swing. On a critter that
	// engagement fires CritterAI::JustEngagedWith -> flee
	// (PassiveAI.cpp:76-79), bridged in triggerCritterFlee.
	if s.server != nil && s.player != nil {
		s.server.triggerCritterFlee(ctx, target, s.player.X, s.player.Y)
	}
	// Unit::AttackerStateUpdate (Unit.cpp:2141-2144): the MELEE_ATTACK strip
	// fires ahead of the ranged early-out, so shots break these auras too.
	s.removeAurasWithInterruptFlags(auraInterruptFlagMeleeAttack)
	now := time.Now()
	s.lastRangedSwing = now

	// Per-swing TakeAmmo for Auto Shot (Spell 75) (TC Spell::TakeAmmo,
	// Spell.cpp:4875): consume one ammo (or one thrown weapon) per swing;
	// when the last count goes the auto-repeat stops with
	// SPELL_FAILED_NO_AMMO, matching the C++ next-swing CheckCast failure.
	// A wand/broken/missing ranged weapon or a non-ranged attack type
	// consumes nothing and the repeat keeps firing, per TakeAmmo's
	// early-return arms.
	if spellID == 75 {
		if spell, found, serr := s.server.Data.Spell(spellID); found && serr == nil {
			if _, outOfAmmo := s.takeAmmo(ctx, spell); outOfAmmo {
				s.autoRepeatSpell = 0
				s.autoRepeatTarget = 0
				buf := protocol.NewBuffer(9)
				buf.WritePackedGUID(s.playerGUID)
				_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(1, spellID, 75), true) // SPELL_FAILED_NO_AMMO = 75
			}
		}
	}

	// Visual: broadcast SMSG_SPELL_GO (TC Spell::SendSpellGo)
	castID := uint8(1)
	castTimeStamp := uint32(now.UnixMilli())
	hitTargets := []uint64{target.GUID}
	spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: target.GUID}
	if s.server != nil && s.server.Data != nil {
		if spellInfo, found, err := s.server.Data.Spell(spellID); err == nil && found {
			spellTarget = spellGoPacketTarget(spellInfo, spellTarget)
		}
	}
	goPkt := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagGo, castTimeStamp, hitTargets, nil, spellTarget)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
		s.server.triggerPetDefensive(ctx, s.player.Map, s.player.InstanceID, s.playerGUID, target.GUID, scripting.CreatureEventOnOwnerAttacked)
	}

	// Outcome: ranged attacks can be dodged or blocked, but cannot be parried (TC rollMeleeOutcome)
	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	// Glancing victims exclude pets (Unit.cpp:2276-2279).
	isPetVictim := false
	if !isPlayerVictim && s.server != nil {
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil {
			isPetVictim = tm.OwnerGUID != 0
		}
	}
	canBlock := false
	canDodge := true
	var critReductionBP int32
	// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the victim-side
	// crit-taken arms — victimFlatCritTakenBP is the pre-resilience
	// SPELL_AURA_MOD_ATTACKER_RANGED_CRIT_CHANCE (188) total plus the
	// caster-matched SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) total;
	// victimSpellWeaponCritBP is the post-resilience
	// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197) total.
	var victimFlatCritTakenBP, victimSpellWeaponCritBP int32
	// The attacker's ranged crit bonus (GetUnitCriticalChanceDone's player
	// arm, Unit.cpp:2830-2832) folds into the resilience base: C++ applies
	// ApplyResilience to critDone + the 188/308 flats (Unit.cpp:2871).
	critBonusBP := int32(math.Round(s.getRangedCritPct() * 100))
	victimDodgeBP := int32(-1)
	// Unit::GetUnitParryChance / Unit::GetUnitBlockChance read the victim's
	// PLAYER_PARRY_PERCENTAGE / PLAYER_BLOCK_PERCENTAGE (Unit.cpp:2712/2788);
	// -1 keeps the legacy 5%-base fallback.
	victimParryBP := int32(-1)
	victimBlockBP := int32(-1)
	// Unit::GetUnitBlockChance (Unit.cpp:2777): the victim's own
	// SPELL_AURA_MOD_BLOCK_PERCENT (51) total adds to the 5% base.
	// Unit::GetUnitDodgeChance (Unit.cpp:2676-2681): the victim's
	// SPELL_AURA_MOD_DODGE_PERCENT (49) total adds to the 5% base.
	var victimBlockBonusBP, victimDodgeBonusBP int32
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && vicSess.player != nil {
			canBlock = vicSess.player.CanBlock && vicSess.player.Block > 0
			// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2861-2875): the
			// victim's SPELL_AURA_MOD_ATTACKER_RANGED_CRIT_CHANCE (188)
			// total and the caster-matched
			// SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) total join the
			// resilience base (critDone + flats, Unit.cpp:2871);
			// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197)
			// applies after resilience.
			victimFlatCritTakenBP = int32(math.Round(float64(vicSess.playerAuraModifier(spellAuraModAttackerRangedCritChance))*100)) +
				int32(math.Round(vicSess.victimCritChanceForCaster(s.playerGUID)*100))
			resilBaseBP := int32(500) + critBonusBP + victimFlatCritTakenBP
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenRanged, &resilBaseBP)
			critReductionBP = int32(500) + critBonusBP + victimFlatCritTakenBP - resilBaseBP
			victimSpellWeaponCritBP = int32(math.Round(float64(vicSess.playerAuraModifier(spellAuraModAttackerSpellAndWeaponCritChance)) * 100))
			victimDodgeBP = int32(math.Round(float64(vicSess.player.DodgePercentage) * 100))
			victimParryBP = int32(math.Round(float64(vicSess.player.ParryPercentage) * 100))
			victimBlockBP = int32(math.Round(float64(vicSess.player.BlockPercentage) * 100))
			victimBlockBonusBP = vicSess.totalAuraModifier(spellAuraModBlockPercent) * 100
		}
	}
	// Creature victims (Unit.cpp:2676-2681): 49 total adds to the 5% dodge
	// base. Ranged attacks never parry and creatures never block ranged
	// (canBlock stays false), so only the dodge fold is wired here.
	if !isPlayerVictim && s.server != nil {
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil && !s.server.isTotemGUID(target.GUID) {
			victimDodgeBonusBP = s.server.creatureAuraTypeSum(creatureAuraKeyForTarget(target), spellAuraModDodgePercent) * 100
			// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the
			// victim's SPELL_AURA_MOD_ATTACKER_RANGED_CRIT_CHANCE (188)
			// total and caster-matched
			// SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) total are
			// pre-resilience; C++ ApplyResilience no-ops for a creature
			// victim without a player owner, so no resilience here;
			// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197)
			// applies after resilience.
			victimKey := creatureAuraKeyForTarget(target)
			victimFlatCritTakenBP = creatureAuraModifierSum(s.server, victimKey, spellAuraModAttackerRangedCritChance)*100 +
				int32(math.Round(creatureVictimCritChanceForCaster(s.server, victimKey, s.playerGUID)*100))
			victimSpellWeaponCritBP = creatureAuraModifierSum(s.server, victimKey, spellAuraModAttackerSpellAndWeaponCritChance) * 100
		}
		// Unit::GetUnitDodgeChance (Unit.cpp:2676-2681): the whole creature
		// arm is gated on !victim->IsTotem() — totems never dodge.
		if s.server.isTotemGUID(target.GUID) {
			canDodge = false
		}
	}
	// Ranged attacks ride Unit::RollMeleeOutcomeAgainst's positional arms
	// (Unit.cpp:2213-2217): no parry (canParry stays false), and
	// SPELL_AURA_IGNORE_HIT_DIRECTION (288) exempts the victim from the
	// behind-arc kill — the melee path's ignoreHitDirection twin.
	ignoreHitDirection := false
	if s.server != nil && s.player != nil {
		if isPlayerVictim {
			if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
				ignoreHitDirection = vicSess.hasAuraType(spellAuraIgnoreHitDirection)
			}
		} else {
			key := creatureAuraKeyForPlayer(*s.player, target.GUID)
			s.server.auraMu.Lock()
			for _, aura := range s.server.activeCreatureAuras[key] {
				if aura != nil && !aura.Stopped && aura.AuraType == spellAuraIgnoreHitDirection {
					ignoreHitDirection = true
					break
				}
			}
			s.server.auraMu.Unlock()
		}
	}
	attackerInFront := hasInArc(target.Orientation, target.X, target.Y, s.player.X, s.player.Y, math.Pi)
	if !attackerInFront && !ignoreHitDirection {
		canBlock = false
		if isPlayerVictim {
			canDodge = false
		}
	}
	// Unit::RollMeleeOutcomeAgainst's casting/cc'd victim arm
	// (Unit.cpp:2238-2242) applies to ranged attacks too: a victim that is
	// casting or controlled (confused/stunned/fleeing) cannot dodge/block.
	// Player victims bridge the cast half via nonMeleeCastInProgress and the
	// CONTROLLED half via the 5/7/12 aura twin; creature victims bridge the
	// cast half via motion.CastingUntil.
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && (vicSess.nonMeleeCastInProgress() || victimIsControlled(s.server, true, vicSess, target)) {
			canBlock = false
			canDodge = false
		}
	} else if s.server != nil {
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil && (time.Now().Before(tm.CastingUntil) || victimIsControlled(s.server, false, nil, target)) {
			canBlock = false
			canDodge = false
		}
	}
	hitBonusBP := int32(math.Round(s.getRangedHitPct() * 100))
	// Player::GetExpertiseDodgeOrParryReduction (Player.cpp:5540) returns 0
	// for RANGED_ATTACK — expertise never reduces dodge/parry on ranged
	// attacks (Unit::RollRangedOutcomeAgainst has no expertise arm).
	expertiseBP := int32(math.Round(s.getExpertiseDodgeParryReductionPct(ctx, protocol.RangedAttack) * 100))
	// SPELL_AURA_MOD_HIT_CHANCE (54) on the attacker and
	// SPELL_AURA_MOD_ATTACKER_RANGED_HIT_CHANCE (185) on the victim fold
	// into the miss roll at percent scale (Unit::MeleeSpellMissChance,
	// Unit.cpp:12425).
	hitBonusBP += s.totalAuraModifier(spellAuraModHitChance) * 100
	hitBonusBP += victimAttackerHitChanceBP(s, target.GUID, isPlayerVictim, spellAuraModAttackerRangedHitChance)
	// Ranged attacks dodge through the same RollMeleeOutcomeAgainst dodge
	// arm (Unit.cpp:2255-2262, no attack-type gate), so the attacker's
	// SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248) dodge reduction
	// (Unit.cpp:2694-2695) and the ungated SPELL_AURA_MOD_ENEMY_DODGE (251)
	// sum (Unit.cpp:2697-2698) apply here like the melee site above.
	dodgeReductionBP := (s.totalAuraModifierByMiscValue(spellAuraModCombatResultChance, victimStateDodge) + s.totalAuraModifier(spellAuraModEnemyDodge)) * 100
	// Unit::RollMeleeOutcomeAgainst's sitting-victim arm (Unit.cpp:2246-2247):
	// a non-standing player victim always takes a crit when the attacker
	// has nonzero crit chance.
	victimSitting := false
	if isPlayerVictim && s.server != nil {
		if vs := s.server.findSessionByGUID(target.GUID); vs != nil && vs.player != nil && vs.player.StandState != 0 {
			victimSitting = true
		}
	}
	outcome, _, _ := rollMeleeOutcome(s.player.Level, target.Level, true, isPlayerVictim, isPetVictim, false, false, victimSitting, false, canBlock, false, canDodge, critReductionBP, hitBonusBP, critBonusBP, expertiseBP, victimDodgeBP, dodgeReductionBP, expertiseBP, 0, victimBlockBonusBP, victimParryBP, victimBlockBP, victimDodgeBonusBP, victimFlatCritTakenBP, victimSpellWeaponCritBP, 0)
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			if vicSess.isImmuneToDamage(1) {
				outcome = protocol.MeleeHitImmune
			}
		}
	} else if !isPlayerVictim && s.server != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, target.GUID) {
		outcome = protocol.MeleeHitEvade
	}

	minDmg := s.player.MinRangedDamage
	maxDmg := s.player.MaxRangedDamage
	if maxDmg < minDmg || minDmg <= 0 {
		minDmg = 1.0
		maxDmg = 2.0
	}
	// Unit::CalculateMeleeDamage with RANGED_ATTACK (Unit.cpp:1236-1237):
	// MeleeDamageBonusDone's APbonus/creature flat lands after the weapon
	// base and before the versus product, then MeleeDamageBonusTaken's
	// flat/pct arms (87 + Sanctified Wrath ride here on the white path).
	// Player::CalculateMinMaxDamage (StatSystem.cpp:552, 590-591): the AP
	// leg GetTotalAttackPowerValue(RANGED_ATTACK)/14 * attackPowerMod rides
	// baked into UNIT_FIELD_MIN/MAXRANGEDDAMAGE, re-baked by
	// Player::UpdateAttackPowerAndDamage(true) on every AP change. Go bakes
	// the term at stat-calc time from the base RangedAttackPower field
	// (player_state.go) and never re-bakes on aura changes (124/166/167/
	// 212), so the live-vs-baked delta re-syncs it here; the ammo-DPS leg
	// cancels out of the delta.
	rangedSpeedMod := float64(s.player.RangedAttackTime) / 1000.0
	if rangedSpeedMod <= 0 {
		rangedSpeedMod = 2.0
	}
	if rangedSpeedMod < 0.25 {
		rangedSpeedMod = 0.25
	}
	rangedAPDelta := (float64(s.totalAttackPowerValue(true)) - float64(s.player.RangedAttackPower)) / 14.0 * rangedSpeedMod
	done := (float64(minDmg)+float64(rand.Float32()*(maxDmg-minDmg))+rangedAPDelta+float64(s.meleeDamageDoneFlat(ctx, protocol.RangedAttack)))*
		s.meleeDamageDonePctMod(ctx, protocol.RangedAttack) +
		float64(s.swingDoneFlatBonus(ctx, true, false, target.GUID, target))
	damage := uint32(done * s.meleeDamageDoneVersusMod(ctx, target.GUID))
	damage, _ = s.meleeWeaponDamageTaken(ctx, damage, spellSchoolMaskNormal, target.GUID, target, true, true)
	// Unit::CalculateMeleeDamage with RANGED_ATTACK (Unit.cpp:1216-1262):
	// the armor reduction runs in the pre-roll damage loop — Done, Taken,
	// armor, then the outcome roll and the crit doubling (Unit.cpp:1302-1319).
	// The melee white path already follows this order; the ranged path had
	// armor after the crit switch (armor was eating the crit bonus).
	schoolMask := uint8(1) // Physical default
	if s.server != nil && s.server.Data != nil {
		if spellInfo, found, err := s.server.Data.Spell(spellID); err == nil && found && spellInfo.SchoolMask != 0 {
			schoolMask = uint8(spellInfo.SchoolMask)
		}
	}
	// Apply target armor damage reduction for physical ranged attacks
	armor := float64(target.Armor)
	if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); tm != nil {
		armor = s.server.liveCreatureArmor(creatureAuraKeyForTarget(target), tm, float64(target.Armor))
	}
	if armor < 0 {
		armor = 0
	}
	if schoolMask == 1 && armor > 0 && damage > 0 {
		// The 123/269 ignore-resist arms run ahead of the ArP leg on the
		// ranged auto-attack path (Unit.cpp:1632-1650, null spellInfo).
		armor = s.ignoreTargetResistArmorSwing(armor)
		damage = calcArmorReducedDamage(armor, s.player.Level, target.Level, damage, s.getArmorPenPct(ctx, protocol.RangedAttack))
	}
	if damage < 1 {
		damage = 1
	}

	blocked := uint32(0)
	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitBlock:
		// The ranged white path mirrors Unit::CalculateMeleeDamage with
		// RANGED_ATTACK — the same block arm as the melee site above
		// (Unit.cpp:1340-1376; the spell block arm at Unit.cpp:1060-1077
		// shares the semantics). Only player victims can block ranged
		// (canBlock stays false for creatures), so this always resolves
		// through the victim's shield block value.
		blockValue, critChance := s.swingBlockValue(target, isPlayerVictim)
		blocked, _ = applyShieldBlockValue(blockValue, critChance, damage)
		damage -= blocked
	case protocol.MeleeHitCrit:
		damage *= 2
		// Unit::CalculateMeleeDamage crit leg (Unit.cpp:1302-1319): the
		// 203/204 + 163 + 169 arms AddPct after the doubling, ahead of the
		// resilience reduction below.
		damage = s.meleeCritDamageBonus(ctx, damage, target.GUID, target, true)
	case protocol.MeleeHitGlancing:
		// Unit::CalculateMeleeDamage glancing leg (Unit.cpp:1377-1397) — the
		// outcome roll has no attack-type gate on glancing, so a ranged
		// swing can glance too.
		damage = glancingReducedDamage(damage, s.player.Level, target.Level)
	}

	// Unit::ProcSkillsAndReactives victim arm (Unit.cpp:10283-10324): a
	// dodged/blocked ranged shot arms AURA_STATE_DEFENSE on the victim.
	// Ranged attacks never parry (rollMeleeOutcome comment above), so the
	// parry arm cannot fire here; the attacker arm is melee-only in C++
	// (Overpower is a melee swing response).
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			vicSess.applyResilienceToDamage(true, &damage, outcome == protocol.MeleeHitCrit, CombatRatingCritTakenRanged)
			vicSess.procReactiveVictimArms(outcome)
		}
	}

	absorbed := uint32(0)
	if isPlayerVictim && s.server != nil && damage > 0 {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): the attacker's
			// MOD_TARGET_ABSORB_SCHOOL (194) / MOD_TARGET_ABILITY_ABSORB_SCHOOL
			// (245, affecting this spell) pct of damage bypasses absorbs.
			bypass := absorbIgnoreBypass(damage, s.absorbIgnorePctForSpell(uint32(schoolMask), spellID))
			absorbed, damage = vicSess.applyAbsorptionShields(damage-bypass, schoolMask)
			damage += bypass
		}
	} else if !isPlayerVictim && s.server != nil && damage > 0 {
		// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): the attacker's
		// MOD_TARGET_ABSORB_SCHOOL (194) / MOD_TARGET_ABILITY_ABSORB_SCHOOL
		// (245, affecting this spell) pct of damage bypasses absorbs.
		bypass := absorbIgnoreBypass(damage, s.absorbIgnorePctForSpell(uint32(schoolMask), spellID))
		absorbed, damage = s.server.applyCreatureAbsorptionShields(creatureAuraKeyForTarget(target), damage-bypass, schoolMask)
		damage += bypass
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	logPkt := buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, s)
	}

	if s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}
	s.lastCombatTime = now

	// Ranged auto-attack aura procs on the done side (TrinityCore
	// Spell::TargetInfo::DoDamageAndTriggers, Spell.cpp:2427-2540 — the
	// auto-shot/wand arm of Spell::prepareDataForTriggerSystem,
	// Spell.cpp:2018-2034); the trigger spell targets the victim.
	s.procRangedAutoAttackAuraTriggers(ctx, target.GUID, spellID, outcome, absorbed, blocked, damage)

	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && vicSess.player != nil {
			// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
			// before the lethal / non-lethal legs (arena score included).
			damage = vicSess.negateGodModeDamage(damage)
			// Unit::DealDamage (Unit.cpp:815-819): rage from fully absorbed
			// damage — the god arm above returns before this leg in C++.
			if damage == 0 && absorbed > 0 && !vicSess.godCheatActive() {
				vicSess.grantRageFromDamageTaken(ctx, absorbed)
				// Unit::DealDamage (Unit.cpp:742-747): absorbed damage still
				// strips TAKE_DAMAGE-interrupt auras — the removal runs before
				// the !damage early-return.
				vicSess.procDamageAuras(false)
				// Unit::DealDamage (Unit.cpp:749-761): fully absorbed non-DoT
				// damage aborts ABORT_ON_DMG casts (attacker != victim holds —
				// ranged vs another player).
				vicSess.interruptAbsorbedCast()
			}
			if vicSess.player.UnitFlags&unitFlagInCombat == 0 {
				vicSess.player.UnitFlags |= unitFlagInCombat
			}
			vicSess.lastCombatTime = now
			// Victim-side aura procs on the ranged auto-attack event
			// (TrinityCore Spell.cpp:2020/2032 — ProcVictim =
			// PROC_FLAG_TAKEN_RANGED_AUTO_ATTACK), before the victim-side
			// damage application, mirroring the melee ordering; the trigger
			// spell targets the attacker.
			vicSess.procRangedVictimAuraTriggers(ctx, s.playerGUID, spellID, outcome, absorbed, blocked, damage)
			if damage > 0 {
				// Unit::CalcAbsorbResist (Unit.cpp:1977-2060):
				// SPELL_AURA_SPLIT_DAMAGE (81/153) redirects part of the
				// damage to the aura's caster, ahead of the 300 share leg.
				damage = s.server.splitDamageToCaster(ctx, target.GUID, true, creatureAuraKey{}, s.playerGUID, damage, uint32(schoolMask))
				// Unit::DealDamage (Unit.cpp:766-788):
				// SPELL_AURA_SHARE_DAMAGE_PCT copies CalculatePct(damage,
				// amount) to the aura's caster. Runs inside DealDamage, ahead
				// of the health reduction below.
				s.splitShareDamagePct(ctx, target.GUID, true, creatureAuraKey{}, s.playerGUID, damage, uint32(schoolMask))
				// Unit::DealDamage (Unit.cpp:855-877): the arena damage score
				// and the killer/victim achievement arms fire on player
				// ranged damage exactly like the melee leg; DAMAGE_DONE and
				// TOTAL_DAMAGE_RECEIVED are capped at pre-damage health.
				s.server.updateArenaDamageScore(s, damage)
				victimHealth := vicSess.player.Health
				s.updateAchievementCriteria(criteriaTypeDamageDone, 0, min(damage, victimHealth))
				s.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)
				vicSess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
				// Duel defeat (Unit.cpp:825-853, 957-973): any damage >=
				// health-1 on a duelist ends the duel — the clamped hit
				// leaves the loser at 1 HP.
				if duelDefeatOnDamage(vicSess, s.playerGUID, damage, victimHealth) {
					vicSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, victimHealth-1)
				} else if victimHealth > 0 && damage+1 >= victimHealth {
					if arena := s.server.findArenaState(s.player.Map, 0); arena != nil {
						arena.mu.Lock()
						if sc, ok := arena.Scores[s.playerGUID]; ok {
							sc.KillingBlows++
						}
						arena.mu.Unlock()
					}
					vicSess.player.Health = 0
					vicSess.sendPlayerUpdate()
					vicSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, victimHealth)
					if s.server != nil {
						s.server.creditHonorableKill(s, vicSess)
					}
					// Unit::Kill (Unit.cpp:11341-11343): the attacker is a player.
					vicSess.killPlayer(ctx, s, true, true)
					// Eluna PLAYER_EVENT_ON_KILL_PLAYER (6): Unit::Kill tail
					// (Unit.cpp:11466-11480), attacker and victim players.
					s.server.firePVPKillHook(ctx, s, vicSess)
					// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill
					// pet arm — attacker is the player, so only the
					// attacker's live pet gets KilledUnit(victim)
					// (Unit.cpp:11324-11335).
					if pet := s.livePetMotion(); pet != nil {
						s.server.fireCreatureTargetDied(ctx, pet, vicSess.luaPlayer())
					}
					s.server.handleWGPlayerDeath(vicSess, s)
					s.autoRepeatSpell = 0
					s.autoRepeatTarget = 0
					buf := protocol.NewBuffer(9)
					buf.WritePackedGUID(s.playerGUID)
					_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
				} else {
					vicSess.player.Health -= damage
					// Unit::DealDamage (Unit.cpp:915-924): rage from damage
					// received — damage + absorbed for the conversion.
					vicSess.grantRageFromDamageTaken(ctx, damage+absorbed)
					// Unit::DealDamage (Unit.cpp:906-913, 925-931): random
					// durability loss — HIT TAKEN on the player victim and
					// HIT DONE on the player attacker, rolled independently.
					vicSess.rollDurabilityLossOnHit(ctx, damage)
					s.rollDurabilityLossOnHit(ctx, damage)
					vicSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, damage)
					vicSess.delayCurrentCast()
					vicSess.delayCurrentChannel()
					vicSess.procSpellDirectDamageAuras(spellID, damage)
					vicSess.sendPlayerUpdate()
				}
			}
		}
		return
	}

	// Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9): fires before damage apply;
	// handlers may rewrite damage via the second return (Unit::DealDamage,
	// Unit.cpp:697-702). Fired after the damage log, matching C++ sending
	// the log before DealDamage (Spell.cpp:2542). Skipped when damage and
	// absorb are both zero, matching C++ skipping DealDamage when damage,
	// absorb and resist are all zero (Unit.cpp:1513); the ranged path has
	// no resist model, so the remaining zero-damage cases are the skipped
	// ones. Player victims return above; the motion lookup nil-guards
	// anything else.
	if s.server != nil && (damage > 0 || absorbed > 0) {
		if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, target.GUID); motion != nil {
			damage = s.server.fireCreatureDamageTaken(ctx, motion, s.luaPlayer(), damage)
		}
	}

	// Unit::CalcAbsorbResist (Unit.cpp:1977-2060):
	// SPELL_AURA_SPLIT_DAMAGE (81/153) redirects part of the damage to the
	// aura's caster, ahead of the 300 share leg.
	if damage > 0 {
		damage = s.server.splitDamageToCaster(ctx, target.GUID, false, creatureAuraKeyForTarget(target), s.playerGUID, damage, uint32(schoolMask))
	}
	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	if damage > 0 {
		s.splitShareDamagePct(ctx, target.GUID, false, creatureAuraKeyForTarget(target), s.playerGUID, damage, uint32(schoolMask))
	}

	// Unit::DealDamage (Unit.cpp:699-704): the AI hooks fire at the top of
	// DealDamage — ahead of the lethal leg — on every path, including killing
	// blows. Fired under motionMu: the hook arms (vancleefAI) mutate motion
	// state directly. The lethal leg decides on the post-hook damage against
	// the locked motion health (the pre-lock snapshot is stale once hooks
	// run); when the motion is gone the snapshot decides, as before.
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
	if motion != nil {
		if motion.BossAI == nil {
			motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
		}
		fireDealDamageAIHooks(ctx, s.server, motion, nil, s.playerGUID, motion.GUID, &damage)
	}
	victimHealth := target.Health
	if motion != nil {
		victimHealth = motion.Health
	}
	// Unit::DealDamage vehicle-mounted duel leg (Unit.cpp:837-853): a blow
	// that would leave a ridden vehicle at health-1 ends the rider's mounted
	// duel instead — the hit is consumed, skipping the health legs below.
	if motion != nil && duelDefeatOnVehicleDamage(s, motion, s.playerGUID, damage, victimHealth) {
		s.server.motionMu.Unlock()
		return
	}
	if damage >= victimHealth {
		// Target dies
		corpseFlags := unitDynFlagLootable
		if motion != nil {
			s.server.clearInstanceEncounter(motion)
			// Unit::DealDamage tap block (Unit.cpp:872-876): the killing
			// blow taps and lowers the damage requirement when untapped.
			s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, motion.Health, true)
			motion.Health = 0
			motion.DynamicFlags |= unitDynFlagLootable
			// The corpse keeps TAPPED when the tap survived to death
			// (C++ clears it only on a failed damage requirement,
			// Unit.cpp:11172-11175), so broadcast the real flags.
			corpseFlags = motion.DynamicFlags
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		// Deferred Eluna summon hooks queued by boss OnDamageTaken (e.g.
		// VanCleef's 50% summon arm): the fire must run after the unlock
		// since Lua handler methods lock motionMu on demand. Queued hooks
		// fire on the lethal path too — C++ runs DamageTaken ahead of the
		// lethal leg, so a killing blow still triggers them.
		if motion != nil {
			s.server.drainBossSummonHooks(ctx, motion, motion.BossAI)
		}

		s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: corpseFlags,
		})
		s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		s.onCreatureKilled(ctx, target, nil)
		// Eluna PLAYER_EVENT_ON_KILL_CREATURE (7): Unit::Kill tail
		// (Unit.cpp:11466-11480), attacker is the player.
		if s.server != nil {
			if motion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); motion != nil {
				s.server.fireCreatureKillHook(ctx, s, motion)
			}
		}
		s.debug("target slain", "account", s.accountName, "guid", target.GUID)
	} else {
		if !isPlayerVictim && s.server != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, target.GUID) {
			return
		}
		newHealth := target.Health - damage
		// Unit::DealDamage (Unit.cpp:925-931): random durability loss on
		// HIT DONE — the attacker is the player.
		s.rollDurabilityLossOnHit(ctx, damage)
		rageChanged := false
		var rageNext, rageMapID, rageInstanceID uint32
		var rageGUID uint64
		newlyTapped := false
		tappedFlags := uint32(0)
		if motion != nil {
			// Unit::DealDamage (Unit.cpp:701-704): the AI hooks fired above
			// on the mutable damage ahead of the tap/health/threat/rage
			// legs; a hook-adjusted damage flows into all of them. Threshold
			// arms (VanCleef's HealthBelowPct quotes) therefore evaluate the
			// pre-hit health, as in C++.
			newHealth = motion.Health - damage
			// Unit::DealDamage tap block (Unit.cpp:872-876).
			if s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, motion.Health, true) {
				newlyTapped = true
				tappedFlags = motion.DynamicFlags
			}
			motion.Health = newHealth
			// Unit::DealDamage (Unit.cpp:900-907): the evade leash's
			// last-damaged stamp — direct damage with damage > 0 resets it;
			// DoT ticks never stamp (the tick funnel skips this), and
			// player-owned victims (minions) are excluded from the arm.
			if damage > 0 && motion.OwnerGUID == 0 {
				motion.LastDamaged = now
			}
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received
			// (creature victims; no absorbed model on this path).
			if next, changed := s.server.addCreatureRageLocked(motion, motion.Level, damage); changed {
				rageChanged, rageNext = true, next
				rageMapID, rageInstanceID, rageGUID = motion.Map, motion.InstanceID, motion.GUID
			}
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(motion)
			}
			threat := float32(damage) * s.getThreatMultiplier(uint32(schoolMask))
			// Unit::DealDamage (Unit.cpp:906) calls AddThreat with default
			// args (ignoreRedirects=false): the caster's redirect registry
			// applies to damage threat. The whole arm is skipped for units
			// that cannot have a threat list — ThreatManager::AddThreat's
			// !CanHaveThreatList() early leg (ThreatManager.cpp:328-339)
			// runs before redirect consumption in C++; combat state alone
			// is kept (the aggro path below sets InCombat).
			if motion.ThreatMgr.OwnerCanHaveThreatList() {
				threat, rSwitched, rVictim := s.splitThreatRedirects(motion, threat)
				dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
				inMelee := inMeleeThreatRange(motion.CombatReach, s.player.CombatReach, dist)
				switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
				if rSwitched {
					switched, newVictim = true, rVictim
				}
				if switched && newVictim != motion.TargetGUID {
					motion.TargetGUID = newVictim
					entries := motion.ThreatMgr.SortedEntries()
					s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
				}
			}
		}
		s.server.motionMu.Unlock()

		// Deferred Eluna summon hooks queued by boss OnDamageTaken (e.g.
		// VanCleef's 50% summon arm): the fire must run after the unlock
		// since Lua handler methods lock motionMu on demand.
		if motion != nil {
			s.server.drainBossSummonHooks(ctx, motion, motion.BossAI)
		}

		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth: newHealth,
		})
		if rageChanged {
			s.server.broadcastCreatureValuesUpdateInInstance(rageMapID, rageInstanceID, rageGUID, map[int]uint32{
				unitFieldPower1 + powerRage: rageNext,
			})
		}
		if newlyTapped {
			// New tap: the client grays the name via UNIT_DYNFLAG_TAPPED.
			s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
				unitFieldDynamicFlags: tappedFlags,
			})
		}
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
	}
}

func (s *session) handleAttackStop() bool {
	if !s.playerLoaded {
		return true
	}
	victim := s.attackTarget
	s.attackTarget = 0
	if victim == 0 {
		// Unit::AttackStop (Unit.cpp:5763-5766): a CMSG_ATTACKSTOP arriving while
		// not attacking returns false before SendMeleeAttackStop and before the
		// CURRENT_MELEE_SPELL interrupt — no packet, no swing-slot cancel. Go
		// previously emitted SMSG_ATTACK_STOP with an empty victim every time.
		return true
	}
	// Unit::AttackStop (Unit.cpp:5778) interrupts CURRENT_MELEE_SPELL — the
	// queued on-next-swing spell (Heroic Strike et al.) never lands after a
	// stop; Go's nextSwing model is that slot (server.go:494).
	s.cancelNextSwingSpell()
	if err := s.sendAttackStop(victim, false); err != nil {
		s.debug("attack stop failed", "account", s.accountName, "error", err)
		return false
	}
	s.debug("attack stopped", "account", s.accountName, "guid", victim)
	return true
}

func (s *session) stopPvPCombatForSanctuary() {
	if s == nil || s.player == nil || s.server == nil || s.duelPartner != 0 || s.attackTarget == 0 {
		return
	}
	opponent := s.server.findSessionByGUID(s.attackTarget)
	if opponent == nil || opponent == s || !opponent.worldReady.Load() || opponent.player == nil {
		return
	}
	victim := s.attackTarget
	s.attackTarget = 0
	_ = s.sendAttackStop(victim, false)
	s.player.UnitFlags &^= unitFlagInCombat
	s.sendPlayerUpdate()
	if opponent.attackTarget == s.playerGUID {
		opponent.attackTarget = 0
		_ = opponent.sendAttackStop(s.playerGUID, false)
		if opponent.player != nil {
			opponent.player.UnitFlags &^= unitFlagInCombat
			opponent.sendPlayerUpdate()
		}
	}
}

func (s *session) handleSetSheathed(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	state, err := reader.ReadU32()
	if err != nil {
		return true
	}
	// HandleSetSheathedOpcode (CombatHandler.cpp:73-78): sheath states at or
	// past MAX_SHEATH_STATE are logged and dropped without touching state.
	if state >= maxSheathState {
		s.debug("unknown sheath state rejected", "account", s.accountName, "state", state)
		return true
	}
	// Player::SetSheath (Player.cpp:9655-9682): the sheath state sets the
	// three UNIT_VIRTUAL_ITEM_SLOT_ID values (weapon entries shown on the
	// model) before the sheath byte itself; UNARMED clears all three.
	var slots [3]uint32
	switch state {
	case sheathStateMelee:
		_, slots[0] = s.equippedWeaponInstance(ctx, 15) // EQUIPMENT_SLOT_MAINHAND
		_, slots[1] = s.equippedWeaponInstance(ctx, 16) // EQUIPMENT_SLOT_OFFHAND
	case sheathStateRanged:
		_, slots[2] = s.equippedWeaponInstance(ctx, 17) // EQUIPMENT_SLOT_RANGED
	}
	s.player.SheathState = uint8(state)
	s.player.VirtualItemSlots = slots
	s.sendPlayerUpdate()
	s.debug("sheath state changed", "account", s.accountName, "state", state)
	return true
}

func (s *session) sendAttackStop(victim uint64, nowDead bool) error {
	payload := buildAttackStop(s.playerGUID, victim, nowDead)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), payload, true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), payload, s)
	}
	return nil
}

// sendAttackStopSelf is WorldSession::SendAttackStop (CombatHandler.cpp:82-85):
// a session-only SMSG_ATTACK_STOP. The swing handler's IsValidAttackTarget
// rejects answer the client alone, never the surrounding set; the
// broadcast twin above stays for genuine stops (death, AttackStop) per the
// tree's broadcast convention.
func (s *session) sendAttackStopSelf(victim uint64) error {
	return s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), buildAttackStop(s.playerGUID, victim, false), true)
}

// attackTargetFriendlyToPlayer mirrors the friendly-target arm of
// WorldObject::IsValidAttackTarget (Object.cpp:3026-3027): a target the
// player is friendly to — in either direction — rejects CMSG_ATTACKSWING.
func (s *session) attackTargetFriendlyToPlayer(victim uint64, target combatTarget) bool {
	if s == nil || s.player == nil || s.server == nil {
		return false
	}
	if victimSess := s.server.findSessionByGUID(victim); victimSess != nil && victimSess.player != nil {
		return s.isFriendlyToTarget(victim, victimSess)
	}
	if target.Faction == 0 {
		return false
	}
	playerTpl, found := s.factionTemplateEntry(s.effectiveFactionTemplate(s.player.Race))
	if !found {
		return false
	}
	creatureTpl, found := s.factionTemplateEntry(target.Faction)
	if !found {
		return false
	}
	return factionTemplateFriendlyTo(playerTpl, creatureTpl) || factionTemplateFriendlyTo(creatureTpl, playerTpl)
}

// notifyPetsOwnerAttacked ports the controlled-pet notify in Unit::Attack
// (Unit.cpp:5752-5760): starting a melee attack tells every controlled pet
// via PetAI::OwnerAttacked (PetAI.cpp:295-314) — a live, non-passive pet
// whose current victim is not alive assists the owner (AttackStart). Go's
// AttackStart twin is onPetCommandAttack.
func (s *session) notifyPetsOwnerAttacked(ctx context.Context, victim uint64) {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 || victim == 0 {
		return
	}
	motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	if motion == nil || motion.Health == 0 || motion.PetReact == PetReactPassive {
		return
	}
	if motion.TargetGUID != 0 {
		victimAlive := false
		if targetSess := s.server.findSessionByGUID(motion.TargetGUID); targetSess != nil && targetSess.player != nil {
			victimAlive = targetSess.player.Health > 0
		} else if targetMotion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, motion.TargetGUID); targetMotion != nil {
			victimAlive = targetMotion.Health > 0
		}
		if victimAlive {
			return
		}
	}
	s.server.onPetCommandAttack(ctx, s.player.Map, s.player.InstanceID, s.player.PetGUID, victim)
}

func buildAttackStart(attacker, victim uint64) []byte {
	// CombatPackets.cpp:26-32: AttackStart writes both guids packed.
	packet := protocol.NewBuffer(16)
	packet.WritePackedGUID(attacker)
	packet.WritePackedGUID(victim)
	return packet.Bytes()
}

func buildAttackStop(attacker, victim uint64, nowDead bool) []byte {
	packet := protocol.NewBuffer(24)
	packet.WritePackedGUID(attacker)
	packet.WritePackedGUID(victim)
	if nowDead {
		packet.WriteU32(1)
	} else {
		packet.WriteU32(0)
	}
	return packet.Bytes()
}

// calcArmorReducedDamage computes physical damage reduction based on victim armor,
// attacker level (the reduction curve), victim level (the armor-penetration
// cap), and optional attacker armor penetration percentage (ArP).
// Matching TrinityCore Unit::CalcArmorReducedDamage (Unit.cpp:1600-1700).
func calcArmorReducedDamage(armor float64, attackerLevel uint8, victimLevel uint8, damage uint32, armorPenPct ...float64) uint32 {
	if armor <= 0 || damage == 0 {
		return damage
	}
	levelModifier := float64(attackerLevel)
	if levelModifier < 1 {
		levelModifier = 1
	}
	if levelModifier > 59.0 {
		levelModifier += 4.5 * (levelModifier - 59.0)
	}

	effectiveArmor := armor
	if len(armorPenPct) > 0 && armorPenPct[0] > 0 {
		arp := armorPenPct[0]
		if arp > 100.0 {
			arp = 100.0
		}
		// Unit.cpp:1666-1675: the penetration cap keys on the VICTIM's
		// level, not the attacker's (levelModifier above feeds only the
		// reduction curve): maxArmorPen = 400 + 85*victimLevel below 60,
		// plus 4.5*85*(victimLevel-59) above; capped at
		// min((armor + maxArmorPen)/3, armor), then armor -=
		// CalculatePct(capped, arpPct).
		vLvl := float64(victimLevel)
		if vLvl < 1 {
			vLvl = 1
		}
		var maxArmorPen float64
		if vLvl < 60 {
			maxArmorPen = 400 + 85*vLvl
		} else {
			maxArmorPen = 400 + 85*vLvl + 4.5*85*(vLvl-59)
		}
		capped := math.Min((armor+maxArmorPen)/3.0, armor)
		effectiveArmor -= capped * (arp / 100.0)
		if effectiveArmor < 0 {
			effectiveArmor = 0
		}
	}

	damageReduction := 0.1 * effectiveArmor / (8.5*levelModifier + 40.0)
	damageReduction /= (1.0 + damageReduction)
	if damageReduction < 0 {
		damageReduction = 0
	} else if damageReduction > 0.75 {
		damageReduction = 0.75
	}
	// Unit.cpp:1700: uint32(std::ceil(std::max(damage * (1.0f -
	// damageReduction), 0.0f))) — ceil, not round; with damage >= 1 and
	// damageReduction <= 0.75 the result is >= 1 by construction.
	return uint32(math.Ceil(math.Max(float64(damage)*(1.0-damageReduction), 0.0)))
}

type creatureStats struct {
	Level           uint32
	Health          uint32
	MaxHealth       uint32
	Mana            uint32
	UnitClass       uint32
	Armor           uint32
	Resistances     [7]uint32
	MinDamage       float32
	MaxDamage       float32
	AttackTime      uint32
	BoundingRadius  float32
	CombatReach     float32
	UnitFlags       uint32
	FlagsExtra      uint32
	TypeFlags       uint32
	NPCFlags        uint32 // creature_template.npcflag; live value behind Lua CreatureMethods::GetNPCFlags/SetNPCFlags (UNIT_NPC_FLAGS)
	CanFly          bool
	ReactState      uint8
	ReactStateKnown bool
	// CreatureType mirrors creature_template.type (SharedDefines.h creature
	// types; 8 = critter, CritterAI::Permissible, PassiveAI.cpp:95-100).
	CreatureType uint32
	// AIName mirrors creature_template.AIName, used by the TurretAI arms
	// (CombatAI.cpp:231-265) and the critter/passive react mapping.
	AIName string
	// MechanicImmuneMask / SpellSchoolImmuneMask mirror the
	// creature_template columns feeding Creature::LoadTemplateImmunities
	// (Creature.cpp:2279-2313): bit (i-1) of MechanicImmuneMask is an
	// IMMUNITY_MECHANIC row for mechanic i; bit i of SpellSchoolImmuneMask
	// is an IMMUNITY_SCHOOL row for school i.
	MechanicImmuneMask    uint32
	SpellSchoolImmuneMask uint32
}

func (s *Server) loadCreatureStats(ctx context.Context, entry uint32) creatureStats {
	if s == nil {
		return creatureStats{
			Level:          1,
			Health:         100,
			MaxHealth:      100,
			Armor:          10,
			MinDamage:      1.0,
			MaxDamage:      2.0,
			AttackTime:     2000,
			BoundingRadius: 0.306349,
			CombatReach:    1.5,
			ReactState:     creatureReactAggressive,
		}
	}
	s.statsMu.RLock()
	if s.creatureStatsCache != nil {
		if st, ok := s.creatureStatsCache[entry]; ok {
			s.statsMu.RUnlock()
			return st
		}
	}
	s.statsMu.RUnlock()

	stats := creatureStats{
		Level:          1,
		Health:         100,
		MaxHealth:      100,
		Armor:          10,
		MinDamage:      1.0,
		MaxDamage:      2.0,
		AttackTime:     2000,
		BoundingRadius: 0.306349,
		CombatReach:    1.5,
		ReactState:     creatureReactAggressive,
	}
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return stats
	}

	var maxlevel, unitClass, exp, baseAttackTime, unitFlags, flagsExtra, typeFlags, npcFlags, flight int64
	var creatureType int64
	var mechanicImmuneMask, spellSchoolImmuneMask int64
	var healthMod, manaMod, armorMod, damageMod, baseVariance float64

	row := s.WorldStore.DB.QueryRowContext(ctx, `SELECT 
		COALESCE(maxlevel, 1), 
		COALESCE(unit_class, 1), 
		COALESCE(exp, 0), 
		COALESCE(BaseAttackTime, 2000), 
		COALESCE(HealthModifier, 1.0), 
		COALESCE(ManaModifier, 1.0),
		COALESCE(ArmorModifier, 1.0), 
		COALESCE(DamageModifier, 1.0),
		COALESCE(ct.unit_flags, 0),
		COALESCE(ct.flags_extra, 0),
		COALESCE(ct.type_flags, 0),
		COALESCE(ct.npcflag, 0),
		COALESCE(ctm.Flight, 0),
		COALESCE(ct.type, 0),
		COALESCE(ct.MechanicImmuneMask, 0),
		COALESCE(ct.SpellSchoolImmuneMask, 0),
		COALESCE(ct.BaseVariance, 1.0)
		FROM creature_template ct LEFT JOIN creature_template_movement ctm ON ctm.CreatureId = ct.entry WHERE ct.entry = ?`, entry)
	if err := row.Scan(&maxlevel, &unitClass, &exp, &baseAttackTime, &healthMod, &manaMod, &armorMod, &damageMod, &unitFlags, &flagsExtra, &typeFlags, &npcFlags, &flight, &creatureType, &mechanicImmuneMask, &spellSchoolImmuneMask, &baseVariance); err != nil {
		return stats
	}
	if reactState, known, aiName := s.loadCreatureReaction(ctx, entry); known {
		stats.ReactState = reactState
		stats.ReactStateKnown = true
		stats.AIName = aiName
	}
	stats.CreatureType = uint32(creatureType)

	if maxlevel < 1 {
		maxlevel = 1
	}
	if unitClass <= 0 {
		unitClass = 1
	}
	if baseAttackTime <= 0 {
		baseAttackTime = 2000
	}
	if healthMod <= 0 {
		healthMod = 1.0
	}
	if armorMod <= 0 {
		armorMod = 1.0
	}
	if damageMod <= 0 {
		damageMod = 1.0
	}

	stats.Level = uint32(maxlevel)
	stats.UnitClass = uint32(unitClass)
	stats.AttackTime = uint32(baseAttackTime)
	stats.UnitFlags = uint32(unitFlags)
	stats.FlagsExtra = uint32(flagsExtra)
	stats.TypeFlags = uint32(typeFlags)
	stats.NPCFlags = uint32(npcFlags)
	stats.CanFly = flight != 0
	stats.MechanicImmuneMask = uint32(mechanicImmuneMask)
	stats.SpellSchoolImmuneMask = uint32(spellSchoolImmuneMask)

	// Fallback values based on level
	fallbackHealth := uint32(maxlevel * 30)
	if fallbackHealth < 42 {
		fallbackHealth = 42
	}
	stats.Health = fallbackHealth
	stats.MaxHealth = fallbackHealth
	stats.Armor = uint32(maxlevel * 10)
	attSpeed := float32(baseAttackTime) / 1000.0
	stats.MinDamage = float32(maxlevel) * 0.75 * attSpeed
	stats.MaxDamage = float32(maxlevel) * 1.25 * attSpeed

	// Query creature_classlevelstats
	var basehp0, basehp1, basehp2, basearmor, basemana, attackPower int64
	var dmgBase, dmgExp1, dmgExp2 float64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT
		basehp0, basehp1, basehp2, basearmor, basemana, damage_base, damage_exp1, damage_exp2, attackpower
		FROM creature_classlevelstats WHERE level = ? AND class = ?`, maxlevel, unitClass).
		Scan(&basehp0, &basehp1, &basehp2, &basearmor, &basemana, &dmgBase, &dmgExp1, &dmgExp2, &attackPower)
	if err == nil {
		if basemana > 0 && manaMod > 0 {
			stats.Mana = uint32(math.Ceil(float64(basemana) * manaMod))
		}
		var selectedHP int64
		var selectedDmg float64
		switch {
		case exp >= 2:
			selectedHP = basehp2
			selectedDmg = dmgExp2
		case exp == 1:
			selectedHP = basehp1
			selectedDmg = dmgExp1
		default:
			selectedHP = basehp0
			selectedDmg = dmgBase
		}
		if selectedHP > 0 {
			hp := uint32(math.Ceil(float64(selectedHP) * healthMod))
			if hp > 0 {
				stats.Health = hp
				stats.MaxHealth = hp
			}
		}
		if basearmor > 0 {
			stats.Armor = uint32(math.Ceil(float64(basearmor) * armorMod))
		}
		if selectedDmg > 0 {
			// Creature::CalculateMinMaxDamage (StatSystem.cpp:1161-1173):
			// baseValue = GetFlatModifierValue(UNIT_MOD_DAMAGE_MAINHAND,
			// BASE_VALUE) + (GetTotalAttackPowerValue(BASE_ATTACK) / 14) *
			// BaseVariance * GetAPMultiplier(BASE_ATTACK, false). At load
			// the flat BASE_VALUE is 0 (Creature::UpdateLevelDependantStats,
			// Creature.cpp:1535-1548, writes the weapon fields directly and
			// parks the classlevelstats AttackPower on UNIT_MOD_ATTACK_POWER
			// instead); the AP multiplier for creatures is GetAttackTime /
			// 1000 with no 0.25 floor (the floor is Player-only,
			// StatSystem.cpp:549). dmgMultiplier = ModDamage (the rank fold
			// at ObjectMgr.cpp:1200 is 1.0 under default rates); basePct /
			// totalValue / totalPct are 1 / 0 / 1 without auras — the live
			// aura arms ride the swing path, and a live AP-aura change would
			// re-run UpdateDamagePhysical in C++ (unbridged here; the
			// load-time value is baked).
			ap := float64(attackPower)
			if ap < 0 {
				ap = 0
			}
			apBonus := ap / 14.0 * baseVariance * (float64(baseAttackTime) / 1000.0)
			minDmg := (selectedDmg + apBonus) * damageMod
			maxDmg := (selectedDmg*1.5 + apBonus) * damageMod
			if minDmg > 0 {
				stats.MinDamage = float32(minDmg)
				stats.MaxDamage = float32(maxDmg)
			}
		}
	}
	if stats.MinDamage < 1.0 {
		stats.MinDamage = 1.0
	}
	if stats.MaxDamage < stats.MinDamage {
		stats.MaxDamage = stats.MinDamage + 1.0
	}

	var modelBoundingRadius, modelCombatReach sql.NullFloat64
	_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT cmi.BoundingRadius, cmi.CombatReach FROM creature_template ct JOIN creature_model_info cmi ON ct.modelid1 = cmi.DisplayID WHERE ct.entry = ?", entry).Scan(&modelBoundingRadius, &modelCombatReach)
	if modelBoundingRadius.Valid && modelBoundingRadius.Float64 > 0 {
		stats.BoundingRadius = float32(modelBoundingRadius.Float64)
	}
	if modelCombatReach.Valid && modelCombatReach.Float64 > 0 {
		stats.CombatReach = float32(modelCombatReach.Float64)
	}

	s.statsMu.Lock()
	if s.creatureStatsCache == nil {
		s.creatureStatsCache = make(map[uint32]creatureStats)
	}
	s.creatureStatsCache[entry] = stats
	s.statsMu.Unlock()

	return stats
}

func (s *Server) loadCreatureReaction(ctx context.Context, entry uint32) (uint8, bool, string) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return creatureReactAggressive, false, ""
	}
	var creatureType, npcFlags, flagsExtra int64
	var aiName string
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0), COALESCE(npcflag, 0), COALESCE(flags_extra, 0), COALESCE(AIName, '') FROM creature_template WHERE entry = ?", entry).Scan(&creatureType, &npcFlags, &flagsExtra, &aiName); err != nil {
		return creatureReactAggressive, false, ""
	}
	return creatureReactState(uint32(creatureType), uint32(npcFlags), uint32(flagsExtra), aiName), true, aiName
}

func (s *session) loadCombatTarget(ctx context.Context, guid uint64) (combatTarget, error) {
	var target combatTarget
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return target, fmt.Errorf("world store DB not initialized")
	}
	var low, entry, mapID int64
	var curHealth sql.NullInt64
	lowGUID := uint32(guid & 0x00FFFFFF)
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, c.curhealth FROM creature AS c WHERE c.guid = ?", lowGUID).Scan(&low, &entry, &mapID, &target.X, &target.Y, &target.Z, &curHealth); err != nil {
		return target, err
	}
	target.GUID = creatureWorldGUID(uint32(low), uint32(entry))
	target.Map = uint32(mapID)

	var ori sql.NullFloat64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT c.orientation FROM creature AS c WHERE c.guid = ?", lowGUID).Scan(&ori)
		if ori.Valid {
			target.Orientation = float32(ori.Float64)
		}
	}

	st := s.server.loadCreatureStats(ctx, uint32(entry))
	target.UnitFlags = st.UnitFlags
	target.FlagsExtra = st.FlagsExtra
	_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(faction, 0) FROM creature_template WHERE entry = ?", uint32(entry)).Scan(&target.Faction)
	target.Armor = st.Armor
	target.Resistances = st.Resistances
	target.MinDamage = st.MinDamage
	target.MaxDamage = st.MaxDamage
	target.Level = uint8(st.Level)
	target.MaxHealth = st.MaxHealth
	target.CombatReach = st.CombatReach
	if curHealth.Valid {
		if curHealth.Int64 > 0 {
			target.Health = uint32(curHealth.Int64)
		} else {
			target.Health = st.Health
		}
	} else {
		target.Health = st.Health
	}
	if target.MaxHealth > 0 && target.Health > target.MaxHealth {
		target.Health = target.MaxHealth
	}

	s.server.motionMu.Lock()
	if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID); motion != nil {
		target.X, target.Y, target.Z, target.Orientation = motion.X, motion.Y, motion.Z, motion.Orientation
		target.UnitFlags, target.FlagsExtra = motion.UnitFlags, motion.FlagsExtra
		target.Health = motion.Health
		if motion.Armor > 0 {
			target.Armor = motion.Armor
		}
		target.Resistances = motion.Resistances
		if motion.MinDamage > 0 {
			target.MinDamage = motion.MinDamage
			target.MaxDamage = motion.MaxDamage
		}
		if motion.CombatReach > 0 {
			target.CombatReach = motion.CombatReach
		} else {
			motion.CombatReach = target.CombatReach
		}
	} else {
		motion := &creatureMotion{
			GUID:       target.GUID,
			LootMode:   lootModeDefault,
			Entry:      uint32(entry),
			Map:        target.Map,
			InstanceID: s.player.InstanceID,
			HomeX:      target.X,
			HomeY:      target.Y,
			HomeZ:      target.Z,
			X:          target.X,
			Y:          target.Y,
			Z:          target.Z,
			Speed:      2.5,
			RunSpeed:   7.0,
			UnitFlags:  target.UnitFlags,
			FlagsExtra: target.FlagsExtra,
			TypeFlags:  st.TypeFlags,
			Health:     target.Health,
			// Creature::ResetPlayerDamageReq (Creature.h:324): GetHealth()/2.
			PlayerDamageReq: target.Health / 2,
			MaxHealth:       target.MaxHealth,
			Armor:           target.Armor,
			Resistances:     target.Resistances,
			MinDamage:       target.MinDamage,
			MaxDamage:       target.MaxDamage,
			Level:           uint32(target.Level),
			AttackTime:      st.AttackTime,
			CombatReach:     target.CombatReach,
			Refreshed:       time.Now(),
		}
		motions := s.server.motionMapLocked(target.Map, s.player.InstanceID)
		motions[target.GUID] = motion
		if guid != target.GUID {
			motions[guid] = motion
		}
	}
	s.server.motionMu.Unlock()
	return target, nil
}

func (s *session) stopAttacksForFaction(ctx context.Context, factionID uint32) {
	if s == nil || s.player == nil || factionID == 0 || s.server == nil {
		return
	}
	if s.attackTarget != 0 {
		if target, ok := s.getCombatTarget(ctx, s.attackTarget); ok && target.Faction == factionID {
			_ = s.handleAttackStop()
		}
	}
	stops := make([]uint64, 0)
	s.server.motionMu.Lock()
	for _, motion := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if motion == nil || motion.TargetGUID != s.playerGUID || motion.Faction != factionID {
			continue
		}
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.Moving = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.RemoveThreat(s.playerGUID)
		}
		stops = append(stops, motion.GUID)
	}
	s.server.motionMu.Unlock()
	for _, guid := range stops {
		packet := buildAttackStop(guid, s.playerGUID, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), packet, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), packet, s)
	}
}

func distance3D(x1, y1, z1, x2, y2, z2 float32) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)
	dz := float64(z1 - z2)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// distance2D mirrors WorldObject::_IsWithinDist with is3D=false
// (Object.cpp:1166 — IsInDist2d): the trade range checks
// (TradeHandler.cpp:271/695, TRADE_DISTANCE) compare X/Y only.
func distance2D(x1, y1, x2, y2 float32) float64 {
	dx := float64(x1 - x2)
	dy := float64(y1 - y2)
	return math.Sqrt(dx*dx + dy*dy)
}

// creatureDodgeReductionBP returns the creature attacker's total
// SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248) amount with
// MiscValue == VICTIMSTATE_DODGE (Unit.cpp:2694-2695), scaled to basis points
// for rollMeleeOutcome's dodge leg (the C++ arm is percent-scale and both
// melee roll sites scale the result by 100, Unit.cpp:2206/2589).
func (s *Server) creatureDodgeReductionBP(key creatureAuraKey) int32 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	var total int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		if s.Data != nil {
			if spell, found, err := s.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != spellAuraModCombatResultChance || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					if effect.MiscValue != victimStateDodge {
						continue
					}
					if amount := aura.Amounts[index]; amount != 0 {
						total += amount
					}
				}
				continue
			}
		}
		if aura.AuraType == spellAuraModCombatResultChance && aura.MiscValue == victimStateDodge {
			total += int32(aura.Amount)
		}
	}
	s.auraMu.Unlock()
	return total * 100
}

// creatureEnemyDodgeBP returns the creature attacker's total ungated
// SPELL_AURA_MOD_ENEMY_DODGE (251) amount (Unit.cpp:2697-2698), scaled to
// basis points for rollMeleeOutcome's dodge leg, alongside
// creatureDodgeReductionBP (the C++ arm is percent-scale and the melee roll
// site scales the result by 100, Unit.cpp:2206).
func (s *Server) creatureEnemyDodgeBP(key creatureAuraKey) int32 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	var total int32
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		if s.Data != nil {
			if spell, found, err := s.Data.Spell(aura.SpellID); err == nil && found {
				for index, effect := range spell.Effects {
					if effect.Aura != spellAuraModEnemyDodge || aura.EffectMask&(1<<uint(index)) == 0 {
						continue
					}
					if amount := aura.Amounts[index]; amount != 0 {
						total += amount
					}
				}
				continue
			}
		}
		if aura.AuraType == spellAuraModEnemyDodge {
			total += int32(aura.Amount)
		}
	}
	s.auraMu.Unlock()
	return total * 100
}

// creatureExpertiseReductionBP returns the creature attacker's total
// SPELL_AURA_MOD_EXPERTISE (240) amount scaled to basis points for
// rollMeleeOutcome's dodge and parry legs. C++ reduces BOTH dodge
// (Unit::GetUnitDodgeChance, Unit.cpp:2700-2703) and parry
// (Unit::GetUnitParryChance, Unit.cpp:2756-2759) by
// GetTotalAuraModifier(240)/4.0f percent for non-player attackers, so the
// value rides expertiseBP ([3]), which the roll subtracts from dodge and
// which parryExpertiseBP ([6]) defaults to; the [6] slot at the creature
// call site carries the same value and is now redundant but kept for
// positional stability.
func (s *Server) creatureExpertiseReductionBP(key creatureAuraKey) int32 {
	if s == nil || key.GUID == 0 {
		return 0
	}
	return s.creatureAuraTypeSum(key, spellAuraModExpertise) * 25
}

// rollMeleeOutcome implements TrinityCore's single-roll melee attack table:
// MISS > DODGE > PARRY > GLANCING > BLOCK > CRIT > CRUSHING > HIT
// Reference: Unit::RollMeleeOutcomeAgainst (Unit.cpp:2189-2320).
// Optional modifiers:
// victimAttackerHitChanceBP sums the victim's
// SPELL_AURA_MOD_ATTACKER_MELEE_HIT_CHANCE (184) /
// SPELL_AURA_MOD_ATTACKER_RANGED_HIT_CHANCE (185) debuffs in basis points —
// the Unit::MeleeSpellMissChance victim arm (Unit.cpp:12425) folds them into
// the attacker's miss chance at percent scale.
func victimAttackerHitChanceBP(s *session, targetGUID uint64, isPlayerVictim bool, auraType uint32) int32 {
	if s == nil || s.server == nil {
		return 0
	}
	var total int32
	if isPlayerVictim {
		if vs := s.server.findSessionByGUID(targetGUID); vs != nil {
			total = vs.totalAuraModifier(auraType)
		}
	} else if s.player != nil {
		total = s.server.creatureAuraTypeSum(creatureAuraKeyForPlayer(*s.player, targetGUID), auraType)
	}
	return total * 100
}

// [0] critReductionBP (from defender resilience)
// [1] hitBonusBP (from attacker hit rating)
// [2] critBonusBP (from attacker crit rating & agility)
// [3] expertiseBP (reduces defender dodge and parry). Player attackers
// pass GetExpertiseDodgeParryReductionPct (Player::GetExpertiseDodgeOrParryReduction,
// Player.cpp:5540; 0 for ranged); creature attackers pass
// creatureExpertiseReductionBP (C++ reduces both dodge and parry by
// GetTotalAuraModifier(240)/4.0f percent for non-player attackers,
// Unit.cpp:2700-2703 and 2756-2759).
// [4] victimDodgeBP (see below)
// [5] attackerDodgeReductionBP: the attacker's
// SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248, MiscValue == VICTIMSTATE_DODGE)
// total in basis points — C++ GetUnitDodgeChance (Unit.cpp:2694-2695) adds the
// attacker's 248 total to the victim's dodge chance (Go's caller folds it in
// as attackerDodgeReductionBP for the shared leg).
// percent-scale aura amount to the percent-scale chance, and both melee roll
// sites scale the result by 100 (Unit.cpp:2206, 2589), so the Go basis-point
// table adds amount*100.
// [6] parryExpertiseBP (optional): parry-only expertise reduction in basis
// points — C++ Unit::GetUnitParryChance's creature arm
// (GetTotalAuraModifier(SPELL_AURA_MOD_EXPERTISE)/4.0f, Unit.cpp:2756-2759).
// The dodge twin (Unit.cpp:2700-2703) now rides [3]; absent, it defaults
// to [3], matching the player arm where GetExpertiseDodgeOrParryReduction
// covers both.
// [7] victimParryBonusBP (optional): the creature victim's
// SPELL_AURA_MOD_PARRY_PERCENT (47) total in basis points — C++
// GetUnitParryChance adds the victim's 47 total to the 5% base
// (Unit.cpp). Absent means none.
// [8] victimBlockBonusBP (optional): the creature victim's
// SPELL_AURA_MOD_BLOCK_PERCENT (51) total in basis points — C++
// GetUnitBlockChance adds the victim's 51 total to the 5% base
// (Unit.cpp). Absent means none.
// [11] victimDodgeBonusBP (optional): the creature victim's
// SPELL_AURA_MOD_DODGE_PERCENT (49) total in basis points — C++
// GetUnitDodgeChance adds the victim's 49 total to the 5% base
// (Unit.cpp:2676-2681). Absent means none. (Slots [9]/[10] are the player
// victim's derived parry/block percentages.)
// [12] victimFlatCritTakenBP (optional): the victim's pre-resilience crit-taken
// flat in basis points — SPELL_AURA_MOD_ATTACKER_MELEE/RANGED_CRIT_CHANCE
// (187/188) total plus the caster-matched SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER
// (308) total (Unit::GetUnitCriticalChanceTaken, Unit.cpp:2861-2867). C++ adds
// these before ApplyResilience, so callers fold them into the resilience base
// when computing [0]. Signed; absent means none.
// [13] victimSpellWeaponCritBP (optional): the victim's
// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197) total in basis
// points — C++ applies it after resilience (Unit.cpp:2875). Signed; absent
// means none.
// [14] attackerCritAdjustBP (optional): signed adjustment to the 5% crit base
// in basis points — non-player attackers add their
// SPELL_AURA_MOD_WEAPON_CRIT_PERCENT (52) + SPELL_AURA_MOD_CRIT_PCT (290)
// totals (Unit::GetUnitCriticalChanceDone, Unit.cpp:2839-2842), and
// CREATURE_FLAG_EXTRA_NO_CRIT (0x20000) zeroes the whole base, so callers pass
// -500 plus the aura totals. Absent means none (player/pet callers).
func rollMeleeOutcome(attackerLevel, victimLevel uint8, isPlayerOrPetAttacker, isPlayerVictim, isPetVictim, attackerNoCrush, attackerCanCrush, victimSitting bool, isDualWielding bool, canBlock, canParry, canDodge bool, modifiers ...int32) (protocol.MeleeHitOutcome, uint32, uint8) {
	if attackerLevel == 0 {
		attackerLevel = 1
	}
	if victimLevel == 0 {
		victimLevel = 1
	}

	var critReductionBP, hitBonusBP, critBonusBP, expertiseBP int32
	if len(modifiers) > 0 {
		critReductionBP = modifiers[0]
	}
	if len(modifiers) > 1 {
		hitBonusBP = modifiers[1]
	}
	if len(modifiers) > 2 {
		critBonusBP = modifiers[2]
	}
	if len(modifiers) > 3 {
		expertiseBP = modifiers[3]
	}
	// Optional 5th modifier: the victim's Gt-based dodge percentage in basis
	// points (PLAYER_DODGE_PERCENTAGE). Present only when the victim is a
	// player whose derived stats are known; -1 keeps the legacy fallback.
	victimDodgeBP := int32(-1)
	if len(modifiers) > 4 {
		victimDodgeBP = modifiers[4]
	}
	// Optional 6th modifier: the attacker's MOD_COMBAT_RESULT_CHANCE dodge
	// reduction in basis points (Unit.cpp:2694-2695); absent means none.
	attackerDodgeReductionBP := int32(0)
	if len(modifiers) > 5 {
		attackerDodgeReductionBP = modifiers[5]
	}
	// Optional 7th modifier: the parry-only expertise reduction in basis
	// points. For non-player attackers C++ reduces parry by
	// GetTotalAuraModifier(SPELL_AURA_MOD_EXPERTISE, 240)/4.0f percent
	// (Unit::GetUnitParryChance, Unit.cpp:2756-2759); since
	// GetUnitDodgeChance carries the same creature arm (Unit.cpp:2700-2703),
	// creature call sites now pass the value in [3] as well and this slot
	// carries the identical value for positional stability. The player arm
	// folds expertise into expertiseBP for both dodge and parry, so the
	// default is expertiseBP.
	parryExpertiseBP := expertiseBP
	if len(modifiers) > 6 {
		parryExpertiseBP = modifiers[6]
	}
	// Optional 8th/9th modifiers: the creature victim's SPELL_AURA_MOD_PARRY_PERCENT
	// (47) / SPELL_AURA_MOD_BLOCK_PERCENT (51) totals in basis points —
	// C++ GetUnitParryChance / GetUnitBlockChance add the victim's 47/51
	// total to the 5% base (Unit.cpp). Absent means none.
	victimParryBonusBP := int32(0)
	if len(modifiers) > 7 {
		victimParryBonusBP = modifiers[7]
	}
	victimBlockBonusBP := int32(0)
	if len(modifiers) > 8 {
		victimBlockBonusBP = modifiers[8]
	}
	// Optional 12th modifier: the creature victim's SPELL_AURA_MOD_DODGE_PERCENT
	// (49) total in basis points — C++ GetUnitDodgeChance adds the victim's 49
	// total to the 5% base (Unit.cpp:2676-2681). Absent means none.
	victimDodgeBonusBP := int32(0)
	if len(modifiers) > 11 {
		victimDodgeBonusBP = modifiers[11]
	}
	// Optional 10th/11th modifiers: the player victim's derived
	// PLAYER_PARRY_PERCENTAGE / PLAYER_BLOCK_PERCENTAGE in basis points.
	// These already fold base 5%, the defense-skill term, aura 47/51 and the
	// rating (Player::UpdateParryPercentage / UpdateBlockPercentage), so the
	// legs below must not add victimParryBonusBP / victimBlockBonusBP again
	// for player victims. -1 keeps the legacy 5%-base fallback.
	victimParryBP := int32(-1)
	if len(modifiers) > 9 {
		victimParryBP = modifiers[9]
	}
	victimBlockBP := int32(-1)
	if len(modifiers) > 10 {
		victimBlockBP = modifiers[10]
	}
	// Optional 13th/14th/15th modifiers: the victim-side crit-taken arms from
	// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880) — the pre-resilience
	// 187/188+308 flat [12], the post-resilience 197 total [13] — and the
	// attacker's crit-base adjustment [14] (creature 52/290 auras, NO_CRIT
	// zeroes the base). All signed basis points; absent means none.
	victimFlatCritTakenBP := int32(0)
	if len(modifiers) > 12 {
		victimFlatCritTakenBP = modifiers[12]
	}
	victimSpellWeaponCritBP := int32(0)
	if len(modifiers) > 13 {
		victimSpellWeaponCritBP = modifiers[13]
	}
	attackerCritAdjustBP := int32(0)
	if len(modifiers) > 14 {
		attackerCritAdjustBP = modifiers[14]
	}

	leveldif := int32(victimLevel) - int32(attackerLevel)
	// Weapon/defense skill difference at 5 skill per level, the C++ 1:1
	// model for every skillDiff arm below (GetWeaponSkillValue /
	// GetMaxSkillValueForLevel have no Go skill-bonus model, so skills are
	// 5/level by construction — same convention as the crushing-blow fix).
	skillDiff := leveldif * 5

	// 1. Miss chance: TrinityCore Unit::MeleeSpellMissChance (Unit.cpp:12408).
	var missChance int32
	if isPlayerVictim {
		// Unit::GetUnitMissChance (Unit.cpp:2762): 5% base. The
		// GetMissPercentageFromDefense above-cap term rides the Go
		// defense-skill model only in the sheet percentages, so the roll
		// keeps the flat 5% — same convention as the crushing-blow fix.
		// Skill arm (Unit.cpp:12433-12436): diff = victimMaxSkill -
		// attackerWeaponSkill; diff>0 adds diff*0.04%, else diff*0.02%
		// (percent) = 4/2 basis points per skill point.
		missChance = 500
		if skillDiff > 0 {
			missChance += skillDiff * 4
		} else {
			missChance += skillDiff * 2
		}
	} else {
		// PvE against creatures (Unit.cpp:12438-12444): diff>10 adds
		// 1% + (diff-10)*0.4%, else diff*0.1% (percent).
		missChance = 500
		if skillDiff > 10 {
			missChance += 100 + (skillDiff-10)*40
		} else {
			missChance += skillDiff * 10
		}
		// Low level mob scaling matching TC: if victimLevel < 10, missChance *= victimLevel / 10
		if victimLevel < 10 {
			missChance = int32(float64(missChance) * (float64(victimLevel) / 10.0))
		}
	}
	// Dual-wielding auto-attacks have +19% chance to miss (+1900 / 10000)
	// Reference: TrinityCore Unit::MeleeSpellMissChance (Unit.cpp:12425).
	if isDualWielding {
		missChance += 1900
	}
	if hitBonusBP > 0 {
		missChance -= hitBonusBP
	}
	if missChance < 0 {
		missChance = 0
	}

	// 2. Dodge chance: TrinityCore Unit::GetUnitDodgeChance (Unit.cpp:2657).
	// For player victims the chance is the victim's Gt-based, diminished
	// PLAYER_DODGE_PERCENTAGE (Player::GetDodgeFromAgility, Player.cpp:5449)
	// plus 0.04% per point of defense-skill difference against the attacker
	// (5 skill per level = 20 basis points per level).
	dodgeChance := int32(0)
	if canDodge && (isPlayerVictim || victimLevel >= 10) {
		if isPlayerVictim && victimDodgeBP >= 0 {
			dodgeChance = victimDodgeBP
			dodgeChance += leveldif * 20
		} else {
			// Creature victims (Unit.cpp:2676-2681): 5% + aura 49 +
			// skillDiff*0.1% (diff<=10) or 1% + (diff-10)*0.1% (diff>10) —
			// both arms reduce to 50*leveldif basis points.
			dodgeChance = 500 + victimDodgeBonusBP + leveldif*50
		}
		if expertiseBP > 0 {
			dodgeChance -= expertiseBP
		}
		// Reduce the victim's dodge chance by the attacker's
		// SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248) auras with
		// MiscValue == VICTIMSTATE_DODGE (Unit.cpp:2694-2695).
		dodgeChance += attackerDodgeReductionBP
		if dodgeChance < 0 {
			dodgeChance = 0
		}
	}

	// 3. Parry chance: TrinityCore Unit::GetUnitParryChance (Unit.cpp:2708).
	parryChance := int32(0)
	if canParry && (isPlayerVictim || victimLevel >= 10) {
		if isPlayerVictim && victimParryBP >= 0 {
			// PLAYER_PARRY_PERCENTAGE already folds base 5%, the
			// defense-skill term, aura 47 and parry rating
			// (Player::UpdateParryPercentage); the roll-time skill arm is
			// 0.04*skillDiff percent (Unit.cpp:2714).
			parryChance = victimParryBP + skillDiff*4
		} else {
			// Creature victims (Unit.cpp:2722-2728): 5% + aura 47 +
			// skillDiff*0.1% (diff<=10) or 1% + (diff-10)*1.6% (diff>10).
			parryChance = 500 + victimParryBonusBP
			if skillDiff <= 10 {
				parryChance += skillDiff * 10
			} else {
				parryChance += 100 + (skillDiff-10)*160
			}
		}
		if parryExpertiseBP > 0 {
			parryChance -= parryExpertiseBP
		}
		if parryChance < 0 {
			parryChance = 0
		}
	}

	// 4. Glancing blow: players/pets against higher level mobs; pets can
	// never be the victim (Unit.cpp:2276-2279:
	// victim->GetTypeId() != TYPEID_PLAYER && !victim->IsPet()).
	glancingChance := int32(0)
	if isPlayerOrPetAttacker && !isPlayerVictim && !isPetVictim && victimLevel > attackerLevel {
		glancingChance = 600 + (int32(victimLevel)-int32(attackerLevel))*600
		if glancingChance > 4000 {
			glancingChance = 4000
		}
	}

	// 5. Block chance: TrinityCore Unit::GetUnitBlockChance (Unit.cpp:2777).
	blockChance := int32(0)
	if canBlock {
		if isPlayerVictim && victimBlockBP >= 0 {
			// PLAYER_BLOCK_PERCENTAGE already folds base 5%, the
			// defense-skill term, aura 51 and block rating
			// (Player::UpdateBlockPercentage, StatSystem.cpp:601); the
			// roll-time skill arm is 0.04*skillDiff percent (Unit.cpp:2796).
			blockChance = victimBlockBP + skillDiff*4
		} else {
			// Creature victims (Unit.cpp:2804-2807): 5% + aura 51 +
			// skillDiff*0.1% (diff<=10) or 1% + (diff-10)*0.1% (diff>10) —
			// both arms reduce to 50*leveldif basis points.
			blockChance = 500 + victimBlockBonusBP + skillDiff*10
		}
		if blockChance < 0 {
			blockChance = 0
		}
	}

	// 6. Crit chance: TrinityCore Unit::GetUnitCriticalChanceTaken
	// (Unit.cpp:2858). The victim-type skill arms:
	// player: -skillDiff*0.04% (Unit.cpp:2889);
	// creature: -skillDiff*0.12%, minus 3% when skillDiff>=15 (Unit.cpp:2890-2894).
	critChance := int32(500)
	if isPlayerVictim {
		critChance -= skillDiff * 4
	} else {
		critChance -= skillDiff * 12
		if skillDiff >= 15 {
			critChance -= 300
		}
	}
	if critBonusBP > 0 {
		critChance += critBonusBP
	}
	// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the victim-side
	// 187/188 + 308 flat is added before resilience, the 197 total after it,
	// then the defense-skill arm above; the roll clamps at zero like C++.
	if victimFlatCritTakenBP != 0 {
		critChance += victimFlatCritTakenBP
	}
	if critReductionBP > 0 {
		critChance -= critReductionBP
	}
	if victimSpellWeaponCritBP != 0 {
		critChance += victimSpellWeaponCritBP
	}
	if attackerCritAdjustBP != 0 {
		critChance += attackerCritAdjustBP
	}
	if critChance < 0 {
		critChance = 0
	}

	// 7. Crushing blow (Unit.cpp:2310-2331): the gate is
	// GetLevelForTarget(victim) >= victim->GetLevelForTarget(this)+4 with
	// !IsControlledByPlayer() and no victim-type gate at all — players are
	// NOT controlled by a player (m_ControlledByPlayer is only set on
	// minions/charms, Unit.cpp:6056/6219), so a player 4+ levels above the
	// victim crushes too (genuine pre-4.0.1 behavior), and mob-vs-pet
	// victims crush as well. attackerCanCrush carries the !IsControlledByPlayer
	// half plus the melee-table-only leg (ranged attacks never enter
	// RollMeleeOutcomeAgainst — DealMeleeDamage returns early for
	// RANGED_ATTACK, Unit.cpp:1206-1214 — so the ranged-white path passes
	// false); pets are controlled and can never crush. tmp =
	// attackerMaxSkillValueForLevel - min(victimDefenseSkill,
	// victimMaxSkillValueForLevel) clamped to >= 20, then tmp*200 - 1500. Go
	// models no weapon/defense skill bonuses, so both skills are 5/level and
	// the min arm is vacuous by construction. CREATURE_FLAG_EXTRA_NO_CRUSH
	// (0x20, CreatureData.h:42) bars the attacker from crushing
	// (Unit.cpp:2312).
	crushingChance := int32(0)
	if attackerCanCrush && !attackerNoCrush && attackerLevel >= victimLevel+4 {
		skillDiff := int32(attackerLevel-victimLevel) * 5
		if skillDiff < 20 {
			skillDiff = 20
		}
		crushingChance = skillDiff*200 - 1500
	}

	roll := rand.IntN(10000)
	sum := int32(0)

	// 1. MISS
	sum += missChance
	if roll < int(sum) {
		return protocol.MeleeHitMiss, protocol.HitInfoMiss, protocol.VictimStateIntact
	}

	// 1a. Sitting player victims always take a crit when the attacker has
	// nonzero crit chance (Unit.cpp:2246-2247: "always crit against a sitting
	// target (except 0 crit chance)") — checked after the miss roll, before
	// the dodge/parry/glancing/block arms.
	if isPlayerVictim && victimSitting && critChance > 0 {
		return protocol.MeleeHitCrit, protocol.HitInfoAffectsVictim | protocol.HitInfoCriticalHit, protocol.VictimStateHit
	}

	// 2. DODGE
	sum += dodgeChance
	if roll < int(sum) {
		return protocol.MeleeHitDodge, protocol.HitInfoNormalSwing, protocol.VictimStateDodge
	}

	// 3. PARRY
	if parryChance > 0 {
		sum += parryChance
		if roll < int(sum) {
			return protocol.MeleeHitParry, protocol.HitInfoNormalSwing, protocol.VictimStateParry
		}
	}

	// 4. GLANCING
	if glancingChance > 0 {
		sum += glancingChance
		if roll < int(sum) {
			return protocol.MeleeHitGlancing, protocol.HitInfoAffectsVictim | protocol.HitInfoGlancing, protocol.VictimStateHit
		}
	}

	// 5. BLOCK
	if blockChance > 0 {
		sum += blockChance
		if roll < int(sum) {
			return protocol.MeleeHitBlock, protocol.HitInfoAffectsVictim | protocol.HitInfoBlock, protocol.VictimStateHit
		}
	}

	// 6. CRIT
	if critChance > 0 {
		sum += critChance
		if roll < int(sum) {
			return protocol.MeleeHitCrit, protocol.HitInfoAffectsVictim | protocol.HitInfoCriticalHit, protocol.VictimStateHit
		}
	}

	// 7. CRUSHING
	if crushingChance > 0 {
		sum += crushingChance
		if roll < int(sum) {
			return protocol.MeleeHitCrushing, protocol.HitInfoAffectsVictim | protocol.HitInfoCrushing, protocol.VictimStateHit
		}
	}

	return protocol.MeleeHitNormal, protocol.HitInfoAffectsVictim, protocol.VictimStateHit
}

func buildAttackerStateUpdate(attacker, victim uint64, damage, overkill uint32) []byte {
	return protocol.BuildAttackerStateUpdate(attacker, victim, damage, overkill, protocol.HitInfoAffectsVictim, protocol.VictimStateHit, 0)
}

// handleDuelAccepted processes CMSG_DUEL_ACCEPTED (0x16C).
// Reference: WorldSession::HandleDuelAcceptedOpcode (DuelHandler.cpp:25-51).
func (s *session) handleDuelAccepted(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// DuelHandler.cpp:27-28: no duel, the acceptor is the initiator, or the
	// duel already left DUEL_STATE_CHALLENGED -> silent return. The
	// IN_PROGRESS leg (DuelTeam != 0, set when the 3s countdown elapses) has
	// no duelCountdown equivalent — without it a duplicate accept re-armed
	// the countdown mid-duel.
	if s.duelPartner == 0 || s.duelInitiator || s.duelCountdown || s.player.DuelTeam != 0 {
		return true
	}
	var partner *session
	if s.server != nil {
		partner = s.server.findSessionByGUID(s.duelPartner)
	}
	// DuelHandler.cpp:31-32: recvPacket >> guid — a raw 8-byte read, not
	// packed; the arbiter GUID on the wire must match the opponent's
	// PLAYER_DUEL_ARBITER (both sides share the flag object, so the
	// local copy is the equivalent check).
	r := protocol.NewReader(payload)
	arbiterGUID, err := r.ReadU64()
	if err != nil || arbiterGUID != s.player.DuelArbiter {
		return true
	}
	// DuelHandler.cpp:44-45: StartTime = now + 3, state -> DUEL_STATE_COUNTDOWN.
	s.duelCountdown = true
	if partner != nil {
		partner.duelCountdown = true
	}
	buf := protocol.NewBuffer(4)
	buf.WriteU32(3000) // 3000ms duel countdown
	_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_COUNTDOWN), buf.Bytes(), true)
	if partner != nil {
		_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_COUNTDOWN), buf.Bytes(), true)
	}

	// Initialize arbiter coordinates if not yet set
	if s.duelArbiterX == 0 && s.duelArbiterY == 0 && partner != nil && partner.player != nil {
		midX := s.player.X + (partner.player.X-s.player.X)/2
		midY := s.player.Y + (partner.player.Y-s.player.Y)/2
		midZ := s.player.Z
		s.duelArbiterX, s.duelArbiterY, s.duelArbiterZ = midX, midY, midZ
		partner.duelArbiterX, partner.duelArbiterY, partner.duelArbiterZ = midX, midY, midZ
	}

	// Player::UpdateDuelFlag (Player.cpp:20785-20797): COUNTDOWN elapsed ->
	// OnPlayerDuelStart hook, PLAYER_DUEL_TEAM 1/2, state IN_PROGRESS. (The
	// hook has no Go dispatch bridge; see the duel_reset audit.)
	partnerGUID := s.duelPartner
	time.AfterFunc(3*time.Second, func() {
		if s.player == nil || !s.duelCountdown || s.duelPartner != partnerGUID {
			return
		}
		// Player::UpdateDuelFlag (Player.cpp:20785-20797): the OnPlayerDuelStart
		// hook fires when the countdown elapses, before PLAYER_DUEL_TEAM 1/2
		// are assigned. Eluna arg order is (event, player1, player2) = (this, opponent).
		if partner != nil && partner.player != nil && partner.duelPartner == s.playerGUID {
			s.triggerPlayerEvent(context.Background(), scripting.PlayerEventDuelStart, s.luaPlayer(), partner.luaPlayer())
		}
		s.duelCountdown = false
		s.player.DuelTeam = 1
		s.sendPlayerUpdate()
		if partner != nil && partner.player != nil && partner.duelPartner == s.playerGUID {
			partner.duelCountdown = false
			partner.player.DuelTeam = 2
			partner.sendPlayerUpdate()
		}
	})
	return true
}

// handleDuelCancelled processes CMSG_DUEL_CANCELLED (0x16D).
// Reference: WorldSession::HandleDuelCancelledOpcode (DuelHandler.cpp:53-74).
func (s *session) handleDuelCancelled(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// DuelHandler.cpp:55-57: C++ reads a raw 8-byte guid from the wire even
	// though it is unused — a short packet throws and the duel is left
	// untouched, so the read comes before the state checks.
	r := protocol.NewReader(payload)
	if _, err := r.ReadU64(); err != nil {
		return true
	}
	// DuelHandler.cpp:59-60: no duel requested or already completed -> silent
	// return (no SMSG_DUEL_COMPLETE).
	if s.duelPartner == 0 {
		return true
	}
	// Player surrendered in an active duel using /forfeit (TC: HandleDuelCancelledOpcode:62-69)
	if s.player.DuelTeam != 0 {
		// CombatStopWithPets(true) on both duelists (DuelHandler.cpp:65-66):
		// the minion arm via stopControlledPetCombat; the includingCast half
		// is the players' own non-melee cast interrupt (Unit::CombatStop(true)
		// -> InterruptNonMeleeSpells(false)); AttackStop is covered by
		// clearDuelState inside endDuel. The charmed-creature arm stays
		// unmodeled (no charmed-creature bridge).
		s.interruptCurrentCast()
		s.interruptCurrentChannel()
		s.stopControlledPetCombat()
		if s.server != nil {
			if partner := s.server.findSessionByGUID(s.duelPartner); partner != nil {
				partner.interruptCurrentCast()
				partner.interruptCurrentChannel()
				partner.stopControlledPetCombat()
			}
		}
		// DuelHandler.cpp:68: the surrendering player casts 7267 (beg) on
		// themselves BEFORE DuelComplete(DUEL_WON). The damage-win path
		// casts it the same way at its own call site (Unit.cpp:972, in
		// duelDefeatOnDamage); DuelComplete itself only casts 52852
		// (victory cheer) on the winner (Player.cpp:7398-7400).
		s.castVisualSpell(7267)
		s.endDuel(true, s.duelPartner, false)
		return true
	}
	s.endDuel(false, 0, false)
	return true
}

// stopControlledPetCombat stops the session's pet combat: the pet half of
// CombatStopWithPets(true) on the duel-surrender path (DuelHandler.cpp:65-66).
// Mirrors the handlePetStopAttack core (PetHandler.cpp:261-280).
func (s *session) stopControlledPetCombat() {
	if s == nil || s.player == nil || s.player.PetGUID == 0 || s.server == nil {
		return
	}
	motion := s.controlledPetMotion(s.player.PetGUID)
	if motion == nil {
		return
	}
	s.server.motionMu.Lock()
	victim := motion.TargetGUID
	motion.TargetGUID = 0
	motion.InCombat = false
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	s.server.motionMu.Unlock()
	stopPkt := buildAttackStop(s.player.PetGUID, victim, false)
	_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, s)
}

// DuelCompleteType mirrors TrinityCore's DuelCompleteType enum.
type DuelCompleteType uint8

const (
	DuelInterrupted DuelCompleteType = 0
	DuelWon         DuelCompleteType = 1
	DuelFled        DuelCompleteType = 2
)

// buildDuelWinner builds SMSG_DUEL_WINNER (0x16B).
// Reference: Player::DuelComplete (Player.cpp:7353-7358).
func buildDuelWinner(fled bool, winnerName, loserName string) []byte {
	packet := protocol.NewBuffer(1 + len(winnerName) + 1 + len(loserName) + 1)
	if fled {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteCString(winnerName)
	packet.WriteCString(loserName)
	return packet.Bytes()
}

// castVisualSpell broadcasts SMSG_SPELL_GO for instant non-combat visual spells (e.g. kneel 7267, victory cheer 52852).
func (s *session) castVisualSpell(spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	castPkt := protocol.NewBuffer(16)
	castPkt.WritePackedGUID(s.playerGUID)
	castPkt.WritePackedGUID(s.playerGUID)
	castPkt.WriteU8(1)
	castPkt.WriteU32(spellID)
	castPkt.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), castPkt.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), castPkt.Bytes(), s)
	}
}

// checkDuelBounds checks distance to duel arbiter (flag).
// > 50yd sends SMSG_DUEL_OUTOFBOUNDS and starts 10s timer.
// <= 40yd sends SMSG_DUEL_INBOUNDS and resets timer.
// > 10s out of bounds ends duel as fled.
// Reference: Player::CheckDuelOutOfBounds (Player.cpp:7290-7321).
func (s *session) checkDuelBounds() {
	if s == nil || s.player == nil || s.player.DuelTeam == 0 || s.duelPartner == 0 || s.player.DuelArbiter == 0 {
		return
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, s.duelArbiterX, s.duelArbiterY, s.duelArbiterZ)
	now := time.Now()
	if s.duelOutOfBounds.IsZero() {
		if dist > 50.0 {
			s.duelOutOfBounds = now.Add(10 * time.Second)
			_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_OUTOFBOUNDS), []byte{}, true)
			partner := s.duelPartner
			time.AfterFunc(10*time.Second, func() {
				if s.player != nil && s.player.DuelTeam != 0 && s.duelPartner == partner && !s.duelOutOfBounds.IsZero() && time.Now().After(s.duelOutOfBounds) {
					s.checkDuelBounds()
				}
			})
		}
	} else {
		if dist <= 40.0 {
			s.duelOutOfBounds = time.Time{}
			_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_INBOUNDS), []byte{}, true)
		} else if now.After(s.duelOutOfBounds) {
			s.endDuel(false, s.duelPartner, true)
		}
	}
}

// endDuel cleans up duel flags, clears arbiter/team, and emits SMSG_DUEL_COMPLETE and SMSG_DUEL_WINNER.
// Reference: Player::DuelComplete (Player.cpp:7328-7442).
// Documented no-bridge arms of DuelComplete: the OnPlayerDuelStart/OnPlayerDuelEnd
// script hooks (no PlayerScript duel dispatch bridge — duel_reset audit); the
// negative-aura strip (auras cast during the duel, apply time >= StartTime — no
// aura apply-time tracking model); the DK quest 12733 credit and
// CONFIG_HONOR_AFTER_DUEL arms; the duel-flag gameobject removal (Go's arbiter is a synthetic GUID, no
// gameobject). CombatStopWithPets' charmed-creature arm is unmodeled — the pet
// arm is bridged via stopControlledPetCombat on the surrender path.
func (s *session) endDuel(won bool, winnerGUID uint64, fled bool) {
	partnerGUID := s.duelPartner
	var partner *session
	if partnerGUID != 0 && s.server != nil {
		partner = s.server.findSessionByGUID(partnerGUID)
	}

	duelEnded := won || fled
	completeResult := uint8(0)
	if duelEnded {
		completeResult = 1
	}

	buf := protocol.NewBuffer(1)
	buf.WriteU8(completeResult)
	_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	if partner != nil {
		_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	}

	// Player::DuelComplete (Player.cpp:7360): the interrupted arm fires
	// OnPlayerDuelEnd after SMSG_DUEL_COMPLETE, before any state cleanup.
	// C++ fires (opponent, this, type); the won/fled arm fires below after
	// the SMSG_DUEL_WINNER broadcast, matching the packet order there.
	if !duelEnded {
		s.triggerPlayerEvent(context.Background(), scripting.PlayerEventDuelEnd, luaPlayerOrNil(partner), s.luaPlayer(), uint8(DuelInterrupted))
	}

	// Player::DuelComplete (Player.cpp:7432-7435): each duelist loses
	// banked combo points when the combo target is controlled by the
	// opponent — the opponent themselves or their pet.
	clearDuelComboPoints := func(sess, opponent *session) {
		if sess == nil || sess.player == nil || opponent == nil || opponent.player == nil || sess.comboTargetGUID == 0 {
			return
		}
		if sess.comboTargetGUID == opponent.playerGUID || sess.comboTargetGUID == opponent.player.PetGUID {
			sess.clearSessionComboPoints()
		}
	}
	clearDuelComboPoints(s, partner)
	clearDuelComboPoints(partner, s)

	if duelEnded && winnerGUID != 0 {
		var winnerSess, loserSess *session
		if s.playerGUID == winnerGUID {
			winnerSess = s
			loserSess = partner
		} else {
			winnerSess = partner
			loserSess = s
		}

		winnerName := ""
		loserName := ""
		if winnerSess != nil && winnerSess.player != nil {
			winnerName = winnerSess.player.Name
		}
		if loserSess != nil && loserSess.player != nil {
			loserName = loserSess.player.Name
		}

		winnerPkt := buildDuelWinner(fled, winnerName, loserName)
		_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_WINNER), winnerPkt, true)
		if partner != nil {
			_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_WINNER), winnerPkt, true)
		}
		if s.server != nil && s.player != nil {
			s.server.sessionsMu.RLock()
			for target := range s.server.sessions {
				if target == s || target == partner || !target.authed || !target.worldReady.Load() || target.player == nil {
					continue
				}
				if target.player.Map == s.player.Map {
					_ = target.write(uint16(protocol.OpcodeSMSG_DUEL_WINNER), winnerPkt, true)
				}
			}
			s.server.sessionsMu.RUnlock()
		}

		// Spells: Winner casts 52852 (Victory cheer)
		// Player::DuelComplete (Player.cpp:7360): OnPlayerDuelEnd fires after
		// the SMSG_DUEL_WINNER broadcast, before the victory spell and the
		// achievements/honor below. C++ fires (opponent, this, type) from the
		// loser's DuelComplete — (winner, loser, type) — matching the Hooks.h
		// (event, winner, loser, type) contract.
		duelEndType := DuelWon
		if fled {
			duelEndType = DuelFled
		}
		s.triggerPlayerEvent(context.Background(), scripting.PlayerEventDuelEnd, luaPlayerOrNil(winnerSess), luaPlayerOrNil(loserSess), uint8(duelEndType))
		if winnerSess != nil {
			winnerSess.castVisualSpell(52852)
			winnerSess.updateAchievementCriteria(criteriaTypeWinDuel, 0, 1)
			// Honor points after duel (Player.cpp:7388-7390): the DUEL_WON
			// arm (not DUEL_FLED) grants the winner HonorPointsAfterDuel
			// honor via RewardHonor(nullptr, 1, amount). The /forfeit path
			// above completes as DUEL_WON (DuelHandler.cpp:71), so the
			// winner takes the honor there too.
			if !fled && s.server != nil {
				if amount := s.server.Config.HonorPointsAfterDuel; amount > 0 {
					winnerSess.rewardHonorPoints(context.Background(), amount)
				}
			}
		}
		// 7267 (beg) is NOT cast here: DuelHandler.cpp:68 casts it on the
		// surrendering player, and Unit.cpp:972 casts it on the damage-win
		// loser — both at their own call sites before DuelComplete(DUEL_WON).
		if loserSess != nil {
			loserSess.updateAchievementCriteria(criteriaTypeLoseDuel, 0, 1)
		}
	}

	// Stop combat on both
	s.clearDuelState(partner)
}

// interruptDuel mirrors Player::DuelComplete(DUEL_INTERRUPTED)
// (Player.cpp:7328-7358): SMSG_DUEL_COMPLETE carries the not-completed flag
// and no SMSG_DUEL_WINNER / win-lose achievement legs run — only the state
// cleanup. Reached from Unit::Kill when a duelist dies to anyone other than
// the duel opponent (Unit.cpp:11359-11365).
func (s *session) interruptDuel() {
	partnerGUID := s.duelPartner
	var partner *session
	if partnerGUID != 0 && s.server != nil {
		partner = s.server.findSessionByGUID(partnerGUID)
	}
	// Unit::Kill (Unit.cpp:11359-11365): before the interrupt completes, both
	// duelists' combat stops WITH pets — CombatStopWithPets(true) interrupts
	// non-melee casts and drops pet attacks on both sides. This arm is not
	// gated by Spirit of Redemption in C++: it runs at the original death even
	// when the victim's cast interrupt above was deferred, so both calls stay
	// ungated here (the interrupt helpers are no-ops when nothing is active).
	s.interruptCurrentCast()
	s.interruptCurrentChannel()
	// CombatStopWithPets(true) also interrupts CURRENT_AUTOREPEAT_SPELL
	// (Unit.cpp:3219-3220, 5809-5811).
	s.cancelAutoRepeatOnInterrupt()
	s.stopOwnPetAttacks()
	if partner != nil {
		partner.interruptCurrentCast()
		partner.interruptCurrentChannel()
		partner.cancelAutoRepeatOnInterrupt()
		partner.stopOwnPetAttacks()
	}
	buf := protocol.NewBuffer(1)
	buf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	if partner != nil {
		_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	}
	// Unit::Kill (Unit.cpp:11363-11368): the duelist killed by anyone other
	// than the duel opponent completes as DUEL_INTERRUPTED — DuelComplete
	// fires the hook after SMSG_DUEL_COMPLETE, before state cleanup.
	s.triggerPlayerEvent(context.Background(), scripting.PlayerEventDuelEnd, luaPlayerOrNil(partner), s.luaPlayer(), uint8(DuelInterrupted))
	s.clearDuelState(partner)
}

// duelDefeatOnDamage mirrors the duel legs of Unit::DealDamage
// (Unit.cpp:825-853 + 957-973): any damage >= health-1 on a dueling player
// ends the duel. attackerPlayerGUID is the blow's GetControllingPlayer()
// (Unit.cpp:5996) — the attacker's own GUID for players, the owner's GUID for
// pets and charmed creatures, 0 for wild creatures. Environmental damage never
// reaches here: with no attacker C++ returns before the health legs
// (Unit.cpp:828-830).
// A blow from the duel opponent (or its controlled creature) is capped to
// health-1 and completes the duel as won; a lethal blow from anyone else is
// NOT consumed — the caller runs its normal kill path, which interrupts the
// duel (Unit::Kill, Unit.cpp:11363-11368). Exactly-health-1 damage from a
// non-opponent still completes the duel as won, since C++ sets duel_hasEnded
// regardless of the attacker. Damage of 0 never reaches the duel leg (the
// !damage early-return precedes it). Returns true when the duel leg consumed
// the hit and the caller must skip its normal health/kill legs.
func duelDefeatOnDamage(victim *session, attackerPlayerGUID uint64, damage, health uint32) bool {
	if victim == nil || victim.player == nil || victim.duelPartner == 0 || victim.player.DuelTeam == 0 {
		return false
	}
	if damage == 0 || health == 0 || damage+1 < health {
		return false
	}
	if attackerPlayerGUID != victim.duelPartner && damage >= health {
		return false
	}
	victim.player.Health = 1
	victim.sendPlayerUpdate()
	if victim.server != nil {
		opp := victim.server.findSessionByGUID(victim.duelPartner)
		if opp == nil {
			opp = victim
		}
		// Unit.cpp:969-970: the duel-win path combat-stops both duelists
		// (CombatStopWithPets(true) — non-melee interrupt, attack stop, pet
		// stop) before the beg cast and DuelComplete, exactly like the
		// /forfeit path in HandleDuelCancelledOpcode.
		opp.interruptCurrentCast()
		opp.interruptCurrentChannel()
		opp.stopControlledPetCombat()
		victim.interruptCurrentCast()
		victim.interruptCurrentChannel()
		victim.stopControlledPetCombat()
		// Unit.cpp:972: the damage-win path casts 7267 (beg) on the loser
		// before DuelComplete(DUEL_WON), same as the /forfeit arm
		// (DuelHandler.cpp:68); the cast lives at the call sites, never
		// inside DuelComplete.
		victim.castVisualSpell(7267)
		opp.endDuel(true, victim.duelPartner, false)
	}
	return true
}

// controllingPlayerGUID mirrors Unit::GetControllingPlayer (Unit.cpp:5996) for
// duel/ownership gates: a player GUID resolves to itself, a pet or charmed
// creature's GUID to its player owner, anything else to 0.
func (s *session) controllingPlayerGUID(guid uint64) uint64 {
	if s == nil || s.server == nil {
		return 0
	}
	if ps := s.server.findSessionByGUID(guid); ps != nil {
		return guid
	}
	if m := s.findCreatureMotion(guid); m != nil && m.OwnerGUID != 0 {
		if os := s.server.findSessionByGUID(m.OwnerGUID); os != nil {
			return m.OwnerGUID
		}
	}
	return 0
}

// duelDefeatOnVehicleDamage mirrors the vehicle-mounted duel leg of
// Unit::DealDamage (Unit.cpp:837-853 + 957-973): a blow that would leave a
// ridden vehicle at health-1 ends the rider's mounted duel. victim->IsVehicle()
// is the vehicle kit on the victim motion; victim->GetCharmer() is the
// CAN_CONTROL seat's passenger (VehicleKit.CharmerGUID); the duel->IsMounted
// latch is session.duelMounted (SpellEffects.cpp:3866-3868, spell 62875).
// The opponent (or its controlled creature, via GetControllingPlayer) clamps
// the hit to health-1; a non-opponent's exactly-health-1 hit still completes
// the duel as won, while its lethal hit falls through to the kill path (which
// interrupts the duel via Unit::Kill, Unit.cpp:11359-11365). The vehicle is
// left at 1 HP, both duelists combat-stop, the rider casts the beg visual
// (7267), and DuelComplete(DUEL_WON) runs. Returns true when the leg consumed
// the hit. The caller must hold motionMu and skip its normal health legs.
// Damage of 0 never reaches the leg (the !damage early-return precedes it).
func duelDefeatOnVehicleDamage(s *session, motion *creatureMotion, attackerPlayerGUID uint64, damage, health uint32) bool {
	if s == nil || s.server == nil || motion == nil {
		return false
	}
	kit := s.server.getVehicleKit(motion.Map, motion.InstanceID, motion.GUID)
	if kit == nil {
		return false
	}
	rider := s.server.findSessionByGUID(kit.CharmerGUID())
	if rider == nil || rider.player == nil || rider.duelPartner == 0 || !rider.duelMounted {
		return false
	}
	if damage == 0 || health == 0 || damage+1 < health {
		return false
	}
	if attackerPlayerGUID != rider.duelPartner && damage >= health {
		return false
	}
	motion.Health = 1
	s.server.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{unitFieldHealth: 1})
	opp := s.server.findSessionByGUID(rider.duelPartner)
	if opp == nil {
		opp = rider
	}
	// Unit.cpp:966-972: the duel-win path combat-stops both duelists
	// (CombatStopWithPets(true)) before the beg cast and DuelComplete,
	// exactly like the plain arm above.
	opp.interruptCurrentCast()
	opp.interruptCurrentChannel()
	opp.stopControlledPetCombat()
	rider.interruptCurrentCast()
	rider.interruptCurrentChannel()
	rider.stopControlledPetCombat()
	rider.castVisualSpell(7267)
	opp.endDuel(true, rider.duelPartner, false)
	return true
}

// clearDuelState performs the shared end-of-duel cleanup: attack stops and
// field resets on both duelists.
func (s *session) clearDuelState(partner *session) {
	_ = s.sendAttackStop(s.attackTarget, false)
	s.attackTarget = 0
	if partner != nil {
		_ = partner.sendAttackStop(partner.attackTarget, false)
		partner.attackTarget = 0
	}

	// Clean up fields on s
	if s.player != nil {
		s.player.DuelArbiter = 0
		s.player.DuelTeam = 0
		s.sendPlayerUpdate()
	}
	s.duelPartner = 0
	s.duelInitiator = false
	s.duelCountdown = false
	s.duelMounted = false
	s.duelOutOfBounds = time.Time{}

	// Clean up fields on partner
	if partner != nil {
		if partner.player != nil {
			partner.player.DuelArbiter = 0
			partner.player.DuelTeam = 0
			partner.sendPlayerUpdate()
		}
		partner.duelPartner = 0
		partner.duelInitiator = false
		partner.duelCountdown = false
		partner.duelMounted = false
		partner.duelOutOfBounds = time.Time{}
	}
}

// isInCombat mirrors Unit::IsInCombat.
func (s *session) isInCombat() bool {
	if s == nil || s.player == nil {
		return false
	}
	return s.attackTarget != 0 || (s.player.UnitFlags&unitFlagInCombat != 0)
}
