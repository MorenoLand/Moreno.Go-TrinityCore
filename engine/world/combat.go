package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	creatureFlagExtraNoParryHasten = 0x00000008
	attackDisplayDelay             = 200 * time.Millisecond
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
//     UnitAI.cpp:88-100). The Go creature spell-cast gate (creaturemotion.go)
//     uses calcMeleeRange as the sizefactor instead, so it carries the +4/3
//     and the 5.0 floor that C++ omits there — a documented range overshoot.
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
func (s *session) startAttackOn(victim uint64) {
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
	victim, err := reader.ReadU64()
	if err != nil {
		s.debug("attack rejected", "account", s.accountName, "error", err)
		return false
	}
	target, ok := s.getCombatTarget(ctx, victim)
	if !ok {
		s.debug("attack target not found", "account", s.accountName, "victim", victim)
		return true
	}
	// WorldObject::IsValidAttackTarget GM/invisibility legs (Object.cpp:2945-2947)
	// via HandleAttackSwingOpcode (CombatHandler.cpp): a GM-mode or GM-invisible
	// player target rejects CMSG_ATTACK_SWING with SMSG_ATTACK_STOP. C++ checks
	// IsValidAttackTarget before Unit::Attack, so this gate sits ahead of the
	// dead-target reject and of Unit::Attack's own GM leg (Unit.cpp:5664-5668).
	if s.server != nil && s.server.gmAttackTargetBlocked(victim) {
		s.attackTarget = 0
		return s.sendAttackStop(victim, false) == nil
	}
	if target.Health == 0 {
		s.attackTarget = 0
		return s.write(uint16(protocol.OpcodeSMSG_ATTACK_SWING_DEAD_TARGET), nil, true) == nil
	}
	// WorldObject::IsValidAttackTarget's "can't attack dead" leg
	// (Object.cpp:2935-2937): ghosts model IsAlive()==false with Health=1,
	// so the dead-target gate above misses them. C++ answers an
	// IsValidAttackTarget failure with SendAttackStop (HandleAttackSwingOpcode,
	// CombatHandler.cpp:40-43), not the dead-target packet (whose sender,
	// Player::SendAttackSwingDeadTarget, has no callers in this revision).
	if s.ghostAttackTargetBlocked(victim) {
		s.attackTarget = 0
		return s.sendAttackStop(victim, false) == nil
	}
	if creatureCombatDisabled(target.UnitFlags, target.FlagsExtra) {
		s.attackTarget = 0
		return s.sendAttackStop(victim, false) == nil
	}
	if target.Map != s.player.Map {
		s.attackTarget = 0
		return s.sendAttackStop(victim, false) == nil
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
		if err := s.sendAttackStop(s.attackTarget, false); err != nil {
			return false
		}
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
	// Unit::Attack (Unit.cpp:5650-5760) engages the victim; on a critter
	// that engagement fires CritterAI::JustEngagedWith -> flee
	// (PassiveAI.cpp:76-79), bridged in triggerCritterFlee.
	if s.server != nil && s.player != nil {
		s.server.triggerCritterFlee(ctx, target, s.player.X, s.player.Y)
	}
	return true
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

func (s *session) executeMeleeSwing(ctx context.Context, target combatTarget, attType protocol.WeaponAttackType) {
	if s.player == nil || s.isDeadOrGhost() || target.Health == 0 {
		return
	}
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
		apBonus := (float64(s.player.AttackPower) * attSpeed) / 14.0
		variance := float64(maxDmg - minDmg)
		baseDmg := float64(minDmg)
		if variance > 0 {
			baseDmg += rand.Float64() * variance
		}
		damage = uint32(baseDmg + apBonus)
	}

	// Off-hand weapon attacks suffer a 50% damage penalty by default (TrinityCore Unit.cpp:353)
	if attType == protocol.OffAttack {
		damage = uint32(math.Round(float64(damage) * 0.5))
	}

	// Apply target armor damage reduction
	if target.Armor > 0 {
		damage = calcArmorReducedDamage(float64(target.Armor), s.player.Level, damage, s.getArmorPenPct())
	}
	if damage < 1 {
		damage = 1
	}

	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	outcome := protocol.MeleeHitNormal
	hitInfo := protocol.HitInfoAffectsVictim
	targetState := protocol.VictimStateHit

	isDualWielding := s.haveOffhandWeapon()
	canBlock := false
	canParry := target.Level >= 10 || isPlayerVictim
	canDodge := true
	var critReductionBP int32
	victimDodgeBP := int32(-1)
	var vicSess *session
	if isPlayerVictim && s.server != nil {
		vicSess = s.server.findSessionByGUID(target.GUID)
		if vicSess != nil && vicSess.player != nil {
			canBlock = vicSess.player.CanBlock && vicSess.player.Block > 0
			critChanceBP := int32(500)
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenMelee, &critChanceBP)
			critReductionBP = 500 - critChanceBP
			// Gt-based dodge (Unit::GetUnitDodgeChance reads the victim's
			// PLAYER_DODGE_PERCENTAGE, Unit.cpp:2667).
			victimDodgeBP = int32(math.Round(float64(vicSess.player.DodgePercentage) * 100))
		}
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

	// A victim mid cast-bar cast cannot dodge/parry/block (Unit.cpp:2219-2224:
	// victim->IsNonMeleeSpellCast(false); genericCastInProgress is the Go
	// analog — channeled/autorepeat casts are skipped by construction there.
	// The UNIT_STATE_CONTROLLED half has no Go unit-state model, stays
	// unbridged; creature victims have no Go cast model, stays unbridged).
	if isPlayerVictim && vicSess != nil && vicSess.genericCastInProgress() {
		canDodge = false
		canParry = false
		canBlock = false
	}

	if s.player.Level > 0 {
		hitBonusBP := int32(math.Round(s.getMeleeHitPct() * 100))
		critBonusBP := int32(math.Round(s.getMeleeCritPct() * 100))
		expertiseBP := int32(math.Round(s.getExpertiseDodgeParryReductionPct() * 100))
		// The attacker's SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248) dodge
		// reduction (Unit.cpp:2694-2695) — the same arm meleeSpellHitResult
		// carries for melee spells.
		dodgeReductionBP := s.totalAuraModifierByMiscValue(spellAuraModCombatResultChance, victimStateDodge) * 100
		outcome, hitInfo, targetState = rollMeleeOutcome(s.player.Level, target.Level, true, isPlayerVictim, isDualWielding, canBlock, canParry, canDodge, critReductionBP, hitBonusBP, critBonusBP, expertiseBP, victimDodgeBP, dodgeReductionBP)
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
		blocked = damage / 4
		if blocked < 1 {
			blocked = 1
		}
		damage -= blocked
	case protocol.MeleeHitCrit:
		damage *= 2
	case protocol.MeleeHitGlancing:
		damage = uint32(float64(damage) * 0.75)
		if damage < 1 {
			damage = 1
		}
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
	// Matching TrinityCore Unit.cpp:1480-1510.
	if targetState == protocol.VictimStateParry && s.server != nil {
		if playerVictim := s.server.findSessionByGUID(target.GUID); playerVictim != nil && playerVictim.player != nil {
			vMainSpeed := time.Duration(playerVictim.player.AttackTime) * time.Millisecond
			if vMainSpeed <= 0 {
				vMainSpeed = 2 * time.Second
			}
			elapsed := now.Sub(playerVictim.lastSwing)
			if elapsed < vMainSpeed {
				rem := vMainSpeed - elapsed
				hasted := calcParryHastedRemaining(rem, vMainSpeed)
				playerVictim.lastSwing = now.Add(-(vMainSpeed - hasted))
			}
		} else {
			s.server.motionMu.Lock()
			if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID); motion != nil {
				if motion.FlagsExtra&creatureFlagExtraNoParryHasten == 0 {
					cSpeed := time.Duration(motion.AttackTime) * time.Millisecond
					if cSpeed <= 0 {
						cSpeed = 2 * time.Second
					}
					elapsed := now.Sub(motion.LastAttack)
					if elapsed < cSpeed {
						rem := cSpeed - elapsed
						hasted := calcParryHastedRemaining(rem, cSpeed)
						motion.LastAttack = now.Add(-(cSpeed - hasted))
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
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPayload, s)
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

	if s.server != nil {
		s.server.triggerPetDefensive(s.player.Map, s.player.InstanceID, s.playerGUID, target.GUID)
	}

	// If target is an online player (e.g. duel opponent or PvP)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil && playerSess.player != nil {
			// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
			// before the lethal / non-lethal legs (duel defeat included).
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
					playerSess.killPlayer(ctx, s, true)
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
		s.splitShareDamagePct(ctx, target.GUID, false, creatureAuraKeyForTarget(target), s.playerGUID, damage, 1)
	}

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		corpseFlags := unitDynFlagLootable
		if motion != nil {
			// Unit::DealDamage tap block (Unit.cpp:872-876): the killing
			// blow taps and lowers the damage requirement when untapped.
			s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, target.Health)
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

		s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: corpseFlags,
		})
		s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
		_ = s.sendAttackStop(target.GUID, true)
		s.attackTarget = 0
		s.onCreatureKilled(ctx, target, nil)
		s.debug("target slain by auto-attack", "account", s.accountName, "guid", target.GUID)
	} else {
		newHealth := target.Health - damage
		// Unit::DealDamage (Unit.cpp:925-931): random durability loss on
		// HIT DONE — the attacker is the player.
		s.rollDurabilityLossOnHit(ctx, damage)
		rageChanged := false
		var rageNext, rageMapID, rageInstanceID uint32
		var rageGUID uint64
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		newlyTapped := false
		tappedFlags := uint32(0)
		if motion != nil {
			// Unit::DealDamage tap block (Unit.cpp:872-876): first
			// player-attributed hit claims the loot rights, every hit
			// lowers the damage requirement.
			if s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, target.Health) {
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
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
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
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
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
		s.server.procCreatureDamageAuras(creatureAuraKeyForTarget(target), true, damage, target.MaxHealth)
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
	}
}

// consumeRangedAmmo mirrors Spell::TakeAmmo (Spell.cpp:4875): it destroys one
// count of the equipped ammo (PLAYER_AMMO_ID) and reports whether ammo
// remains afterwards. It no-ops (returning false) when no ammo is equipped;
// callers own the out-of-ammo fallout. The wand / broken-ranged /
// thrown-weapon legs have no Go bridge (no ranged-slot or thrown-weapon
// model); the auto-repeat-specific fallout stays with the caller.
func (s *session) consumeRangedAmmo(ctx context.Context) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	ammoEntry := s.player.AmmoID
	if ammoEntry == 0 {
		return false
	}
	var itemGUID, count int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT ii.guid, ii.count FROM character_inventory ci JOIN item_instance ii ON ci.item = ii.guid WHERE ci.guid = ? AND ii.itemEntry = ? AND ii.count > 0 LIMIT 1", s.playerGUID, ammoEntry).Scan(&itemGUID, &count)
	if err != nil {
		return false
	}
	if count <= 1 {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_inventory WHERE item = ?", itemGUID)
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", itemGUID)
		s.adjustQuestItemCount(ctx, ammoEntry, 1, false)
		s.player.AmmoID = 0
		_ = s.calculatePlayerStats(ctx, s.player)
		s.sendPlayerUpdate()
		return false
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE item_instance SET count = count - 1 WHERE guid = ?", itemGUID)
	s.adjustQuestItemCount(ctx, ammoEntry, 1, false)
	return true
}

func (s *session) executeRangedAttack(ctx context.Context, target combatTarget, spellID uint32) {
	if s.player == nil || s.isDeadOrGhost() || target.Health == 0 {
		return
	}
	now := time.Now()
	s.lastRangedSwing = now

	// Consume ammo for hunter bow/gun/crossbow (Spell 75) (TC Spell::TakeAmmo)
	if spellID == 75 && s.player.AmmoID > 0 && !s.consumeRangedAmmo(ctx) {
		// the last ammo was consumed: stop the auto-repeat; the next
		// swing's CheckCast fails SPELL_FAILED_NO_AMMO
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(1, spellID, 75), true) // SPELL_FAILED_NO_AMMO = 75
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
		s.server.triggerPetDefensive(s.player.Map, s.player.InstanceID, s.playerGUID, target.GUID)
	}

	// Outcome: ranged attacks can be dodged or blocked, but cannot be parried (TC rollMeleeOutcome)
	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	canBlock := false
	canDodge := true
	var critReductionBP int32
	victimDodgeBP := int32(-1)
	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil && vicSess.player != nil {
			canBlock = vicSess.player.CanBlock && vicSess.player.Block > 0
			critChanceBP := int32(500)
			vicSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenRanged, &critChanceBP)
			critReductionBP = 500 - critChanceBP
			victimDodgeBP = int32(math.Round(float64(vicSess.player.DodgePercentage) * 100))
		}
	}
	attackerInFront := hasInArc(target.Orientation, target.X, target.Y, s.player.X, s.player.Y, math.Pi)
	if !attackerInFront {
		canBlock = false
		if isPlayerVictim {
			canDodge = false
		}
	}
	hitBonusBP := int32(math.Round(s.getRangedHitPct() * 100))
	critBonusBP := int32(math.Round(s.getRangedCritPct() * 100))
	expertiseBP := int32(math.Round(s.getExpertiseDodgeParryReductionPct() * 100))
	outcome, _, _ := rollMeleeOutcome(s.player.Level, target.Level, true, isPlayerVictim, false, canBlock, false, canDodge, critReductionBP, hitBonusBP, critBonusBP, expertiseBP, victimDodgeBP)
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
	damage := uint32(minDmg + rand.Float32()*(maxDmg-minDmg))
	if damage < 1 {
		damage = 1
	}

	blocked := uint32(0)
	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitBlock:
		blocked = damage / 4
		if blocked < 1 {
			blocked = 1
		}
		damage -= blocked
	case protocol.MeleeHitCrit:
		damage *= 2
	}

	if isPlayerVictim && s.server != nil {
		if vicSess := s.server.findSessionByGUID(target.GUID); vicSess != nil {
			vicSess.applyResilienceToDamage(true, &damage, outcome == protocol.MeleeHitCrit, CombatRatingCritTakenRanged)
		}
	}

	schoolMask := uint8(1) // Physical default
	if s.server != nil && s.server.Data != nil {
		if spellInfo, found, err := s.server.Data.Spell(spellID); err == nil && found && spellInfo.SchoolMask != 0 {
			schoolMask = uint8(spellInfo.SchoolMask)
		}
	}
	// Apply target armor damage reduction for physical ranged attacks
	if schoolMask == 1 && target.Armor > 0 && damage > 0 {
		damage = calcArmorReducedDamage(float64(target.Armor), s.player.Level, damage, s.getArmorPenPct())
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
					vicSess.killPlayer(ctx, s, true)
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
					vicSess.procDamageAuras(true, damage)
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

	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	if damage > 0 {
		s.splitShareDamagePct(ctx, target.GUID, false, creatureAuraKeyForTarget(target), s.playerGUID, damage, uint32(schoolMask))
	}

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		corpseFlags := unitDynFlagLootable
		if motion != nil {
			s.server.clearInstanceEncounter(motion)
			// Unit::DealDamage tap block (Unit.cpp:872-876): the killing
			// blow taps and lowers the damage requirement when untapped.
			s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, target.Health)
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
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		newlyTapped := false
		tappedFlags := uint32(0)
		if motion != nil {
			// Unit::DealDamage tap block (Unit.cpp:872-876).
			if s.server.recordCreatureTap(motion, s.playerGUID, s.groupID, damage, target.Health) {
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
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
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
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
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
	if s.autoRepeatSpell != 0 {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
	}
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

func (s *session) handleSetSheathed(payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	reader := protocol.NewReader(payload)
	state, err := reader.ReadU32()
	if err != nil {
		return false
	}
	s.player.SheathState = uint8(state)
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

func buildAttackStart(attacker, victim uint64) []byte {
	packet := protocol.NewBuffer(16)
	packet.WriteU64(attacker)
	packet.WriteU64(victim)
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
// attacker level, and optional attacker armor penetration percentage (ArP).
// Matching TrinityCore Unit::CalcArmorReducedDamage (Unit.cpp:1600-1650).
func calcArmorReducedDamage(armor float64, attackerLevel uint8, damage uint32, armorPenPct ...float64) uint32 {
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
		// WotLK 3.3.5 Armor Penetration cap formula:
		// maxArmorPen = (armor + 400.0 + 85.0 * levelModifier) / 3.0
		// Reference: TrinityCore Unit::CalcArmorReducedDamage
		maxArmorPen := (armor + 400.0 + 85.0*levelModifier) / 3.0
		penetrated := math.Min(armor, maxArmorPen) * (arp / 100.0)
		effectiveArmor -= penetrated
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
	reduced := uint32(math.Round(float64(damage) * (1.0 - damageReduction)))
	if reduced < 1 {
		reduced = 1
	}
	return reduced
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
	CanFly          bool
	ReactState      uint8
	ReactStateKnown bool
	// CreatureType mirrors creature_template.type (SharedDefines.h creature
	// types; 8 = critter, CritterAI::Permissible, PassiveAI.cpp:95-100).
	CreatureType uint32
	// AIName mirrors creature_template.AIName, used by the TurretAI arms
	// (CombatAI.cpp:231-265) and the critter/passive react mapping.
	AIName string
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

	var maxlevel, unitClass, exp, baseAttackTime, unitFlags, flagsExtra, typeFlags, flight int64
	var creatureType int64
	var healthMod, manaMod, armorMod, damageMod float64

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
		COALESCE(ctm.Flight, 0),
		COALESCE(ct.type, 0)
		FROM creature_template ct LEFT JOIN creature_template_movement ctm ON ctm.CreatureId = ct.entry WHERE ct.entry = ?`, entry)
	if err := row.Scan(&maxlevel, &unitClass, &exp, &baseAttackTime, &healthMod, &manaMod, &armorMod, &damageMod, &unitFlags, &flagsExtra, &typeFlags, &flight, &creatureType); err != nil {
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
	stats.CanFly = flight != 0

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
	var basehp0, basehp1, basehp2, basearmor, basemana int64
	var dmgBase, dmgExp1, dmgExp2 float64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT 
		basehp0, basehp1, basehp2, basearmor, basemana, damage_base, damage_exp1, damage_exp2 
		FROM creature_classlevelstats WHERE level = ? AND class = ?`, maxlevel, unitClass).
		Scan(&basehp0, &basehp1, &basehp2, &basearmor, &basemana, &dmgBase, &dmgExp1, &dmgExp2)
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
			dmg := float32(selectedDmg * damageMod)
			if dmg > 0 {
				stats.MinDamage = dmg
				stats.MaxDamage = dmg * 1.5
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

// rollMeleeOutcome implements TrinityCore's single-roll melee attack table:
// MISS > DODGE > PARRY > GLANCING > BLOCK > CRIT > CRUSHING > HIT
// Reference: Unit::RollMeleeOutcomeAgainst (Unit.cpp:2189-2320).
// Optional modifiers:
// [0] critReductionBP (from defender resilience)
// [1] hitBonusBP (from attacker hit rating)
// [2] critBonusBP (from attacker crit rating & agility)
// [3] expertiseBP (reduces defender dodge and parry)
// [4] victimDodgeBP (see below)
// [5] attackerDodgeReductionBP: the attacker's
// SPELL_AURA_MOD_COMBAT_RESULT_CHANCE (248, MiscValue == VICTIMSTATE_DODGE)
// total in basis points — C++ GetUnitDodgeChance (Unit.cpp:2694-2695) adds the
// percent-scale aura amount to the percent-scale chance, and both melee roll
// sites scale the result by 100 (Unit.cpp:2206, 2589), so the Go basis-point
// table adds amount*100.
func rollMeleeOutcome(attackerLevel, victimLevel uint8, isPlayerAttacker, isPlayerVictim bool, isDualWielding bool, canBlock, canParry, canDodge bool, modifiers ...int32) (protocol.MeleeHitOutcome, uint32, uint8) {
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

	leveldif := int32(victimLevel) - int32(attackerLevel)

	// 1. Miss chance
	var missChance int32
	if isPlayerVictim {
		missChance = 500
		if leveldif > 0 {
			missChance += leveldif * 40
		} else {
			missChance += leveldif * 20
		}
	} else {
		// PvE against creatures
		if leveldif > 10 {
			missChance = 100 + (leveldif-10)*400
		} else if leveldif > 0 {
			missChance = 500 + leveldif*100
		} else {
			missChance = 500 + leveldif*100
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
			dodgeChance = 500
			if leveldif > 0 {
				dodgeChance += leveldif * 10
			}
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

	// 3. Parry chance: base 5% (500/10000) if victim can parry
	parryChance := int32(0)
	if canParry && (isPlayerVictim || victimLevel >= 10) {
		parryChance = 500
		if leveldif > 0 {
			parryChance += leveldif * 10
		}
		if expertiseBP > 0 {
			parryChance -= expertiseBP
		}
		if parryChance < 0 {
			parryChance = 0
		}
	}

	// 4. Glancing blow: players/pets against higher level mobs
	glancingChance := int32(0)
	if isPlayerAttacker && !isPlayerVictim && victimLevel > attackerLevel {
		glancingChance = 600 + (int32(victimLevel)-int32(attackerLevel))*600
		if glancingChance > 4000 {
			glancingChance = 4000
		}
	}

	// 5. Block chance: base 5% (500/10000) if victim can block
	blockChance := int32(0)
	if canBlock {
		blockChance = 500
	}

	// 6. Crit chance: base 5% (500/10000)
	critChance := int32(500)
	if leveldif > 2 {
		critChance -= (leveldif - 2) * 100
	} else if leveldif > 0 {
		critChance -= leveldif * 20
	}
	if critBonusBP > 0 {
		critChance += critBonusBP
	}
	if critReductionBP > 0 {
		critChance -= critReductionBP
	}
	if critChance < 0 {
		critChance = 0
	}

	// 7. Crushing blow: a mob 4+ levels above the victim (Unit.cpp:2314-2331):
	// tmp = attackerMaxSkillValueForLevel - min(victimDefenseSkill,
	// victimMaxSkillValueForLevel) clamped to >= 20, then tmp*200 - 1500. Go
	// models no weapon/defense skill bonuses, so both skills are 5/level and
	// the min arm is vacuous by construction.
	crushingChance := int32(0)
	if !isPlayerAttacker && isPlayerVictim && attackerLevel >= victimLevel+4 {
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
	// duel already left DUEL_STATE_CHALLENGED -> silent return.
	if s.duelPartner == 0 || s.duelInitiator || s.duelCountdown {
		return true
	}
	var partner *session
	if s.server != nil {
		partner = s.server.findSessionByGUID(s.duelPartner)
	}
	// DuelHandler.cpp:30-34: the arbiter GUID on the wire must match the
	// opponent's PLAYER_DUEL_ARBITER (both sides share the flag object, so the
	// local copy is the equivalent check).
	r := protocol.NewReader(payload)
	arbiterGUID, err := r.ReadPackedGUID()
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
	// DuelHandler.cpp:59-60: no duel requested or already completed -> silent
	// return (no SMSG_DUEL_COMPLETE).
	if s.duelPartner == 0 {
		return true
	}
	// Player surrendered in an active duel using /forfeit (TC: HandleDuelCancelledOpcode:62-69)
	if s.player.DuelTeam != 0 {
		// CombatStopWithPets(true) on both duelists (DuelHandler.cpp:65-66).
		s.stopControlledPetCombat()
		if s.server != nil {
			if partner := s.server.findSessionByGUID(s.duelPartner); partner != nil {
				partner.stopControlledPetCombat()
			}
		}
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
// aura apply-time tracking model); the combo-point cleanup (no combo-point
// model); the Death Knight quest 12733 credit and CONFIG_HONOR_AFTER_DUEL arms;
// the duel-flag gameobject removal (Go's arbiter is a synthetic GUID, no
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
		// Loser casts 7267 (Beg / surrender kneel) if won normally
		if !fled && loserSess != nil {
			loserSess.castVisualSpell(7267)
		}
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
	s.stopOwnPetAttacks()
	if partner != nil {
		partner.interruptCurrentCast()
		partner.interruptCurrentChannel()
		partner.stopOwnPetAttacks()
	}
	buf := protocol.NewBuffer(1)
	buf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	if partner != nil {
		_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_COMPLETE), buf.Bytes(), true)
	}
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
