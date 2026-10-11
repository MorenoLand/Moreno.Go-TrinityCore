package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	protocol "github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	petPowerTypeHealth            uint32 = 0xFFFFFFFE
	petSpellFailedCasterAuraState uint8  = 22
	petSpellFailedNoPower         uint8  = 85
	petSpellFailedUnknown         uint8  = 187
	// petFollowDist is PET_FOLLOW_DIST (PetDefines.h:85): the pet's follow
	// destination sits this far plus both combat reaches from the owner
	// (WorldObject::GetNearPoint, Object.cpp:3307).
	petFollowDist float32 = 1.0
	// petFollowRangeTolerance is FOLLOW_RANGE_TOLERANCE
	// (FollowMovementGenerator.h:31): the follow spline launches only once
	// the pet is past follow distance + this tolerance.
	petFollowRangeTolerance float32 = 1.0
)

func ResolvePetSpellPowerCost(spell wotlk.Spell, maxPower, maxHealth uint32) uint32 {
	if spell.PowerType == petPowerTypeHealth {
		maxPower = maxHealth
	}
	cost := uint64(spell.ManaCost) + uint64(maxPower)*uint64(spell.ManaCostPct)/100
	if cost > uint64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(cost)
}

func PetCanPaySpell(spell wotlk.Spell, current, maxPower, maxHealth uint32) bool {
	cost := ResolvePetSpellPowerCost(spell, maxPower, maxHealth)
	if spell.PowerType == petPowerTypeHealth {
		return current > cost
	}
	return current >= cost
}

func petSpellPowerState(motion *creatureMotion, powerType uint32) (uint32, uint32, int, bool, bool) {
	if motion == nil {
		return 0, 0, 0, false, false
	}
	if powerType == petPowerTypeHealth {
		return motion.Health, motion.MaxHealth, unitFieldHealth, true, true
	}
	if powerType >= uint32(len(motion.Powers)) {
		return 0, 0, 0, false, false
	}
	return motion.Powers[powerType], motion.MaxPowers[powerType], unitFieldPower1 + int(powerType), true, false
}

func (s *session) checkPetSpellPower(motion *creatureMotion, spell wotlk.Spell, castCount uint8) bool {
	if s == nil || s.server == nil || motion == nil {
		return false
	}
	s.server.motionMu.Lock()
	current, maximum, _, valid, healthPower := petSpellPowerState(motion, spell.PowerType)
	s.server.motionMu.Unlock()
	if !valid {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spell.ID, petSpellFailedUnknown), true)
		return false
	}
	if !PetCanPaySpell(spell, current, maximum, motion.MaxHealth) {
		failure := petSpellFailedNoPower
		if healthPower {
			failure = petSpellFailedCasterAuraState
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spell.ID, failure), true)
		return false
	}
	return true
}

func (s *session) takePetSpellPower(motion *creatureMotion, spell wotlk.Spell, castCount uint8, ignoreCost bool) (uint32, bool, bool) {
	if s == nil || s.server == nil || motion == nil {
		return 0, false, false
	}
	s.server.motionMu.Lock()
	current, maximum, field, valid, healthPower := petSpellPowerState(motion, spell.PowerType)
	if !valid {
		s.server.motionMu.Unlock()
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spell.ID, petSpellFailedUnknown), true)
		return 0, false, false
	}
	cost := uint32(0)
	if !ignoreCost {
		cost = ResolvePetSpellPowerCost(spell, maximum, motion.MaxHealth)
	}
	if !ignoreCost && !PetCanPaySpell(spell, current, maximum, motion.MaxHealth) {
		s.server.motionMu.Unlock()
		failure := petSpellFailedNoPower
		if healthPower {
			failure = petSpellFailedCasterAuraState
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spell.ID, failure), true)
		return 0, false, false
	}
	if cost > 0 {
		current -= cost
		if healthPower {
			motion.Health = current
		} else {
			motion.Powers[spell.PowerType] = current
			if spell.PowerType == 0 {
				motion.Mana = current
				// Spell::TakePower (Spell.cpp:4868-4870): spending mana
				// starts the five-second timer that suppresses
				// Creature::Regenerate's mana arm (Unit.cpp:11074).
				motion.LastManaUse = time.Now()
			}
		}
	}
	s.server.motionMu.Unlock()
	if cost > 0 {
		s.server.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{field: current})
	}
	return current, !healthPower, true
}

// Pet Action and Reaction constants mirroring TrinityCore PetDefines.h
const (
	PetCommandStay    uint8 = 0
	PetCommandFollow  uint8 = 1
	PetCommandAttack  uint8 = 2
	PetCommandAbandon uint8 = 3

	PetReactPassive    uint8 = 0
	PetReactDefensive  uint8 = 1
	PetReactAggressive uint8 = 2
)

// onPetCommandAttack handles ordering the pet to attack a specific target.
// Mirrors TrinityCore PetAI::AttackStart (PetAI.cpp:115).
func (s *Server) onPetCommandAttack(mapID, instanceID uint32, petGUID uint64, targetGUID uint64) {
	if s == nil || petGUID == 0 || targetGUID == 0 {
		return
	}
	// Unit::Attack GM leg (Unit.cpp:5664-5668) via PetAI::AttackStart: a pet
	// cannot be ordered onto a GM-mode or GM-invisible player; the command is
	// dropped without engaging, like the failed Attack in C++.
	if s.gmAttackTargetBlocked(targetGUID) {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil && motion.Health > 0 {
		motion.TargetGUID = targetGUID
		motion.InCombat = true
		motion.PetCommand = PetCommandAttack
		motion.Moving = true
	}
}

// onPetCommandFollow handles recalling the pet back to follow the owner.
// Mirrors TrinityCore PetAI::DoRecall (PetAI.cpp:215).
func (s *Server) onPetCommandFollow(mapID, instanceID uint32, petGUID uint64) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.PetCommand = PetCommandFollow
		motion.Moving = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
	}
}

// onPetCommandStay handles ordering the pet to stay at its current position.
func (s *Server) onPetCommandStay(mapID, instanceID uint32, petGUID uint64) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.PetCommand = PetCommandStay
		motion.Moving = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
	}
}

// onPetSetReaction updates the pet's reaction state (passive, defensive, aggressive).
func (s *Server) onPetSetReaction(mapID, instanceID uint32, petGUID uint64, reactState uint8) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		motion.PetReact = reactState
		if reactState == PetReactPassive {
			// In passive mode, disengage unless ordered to attack
			if motion.PetCommand != PetCommandAttack {
				motion.TargetGUID = 0
				motion.InCombat = false
				motion.Moving = false
			}
		}
	}
}

// onPetToggleAutocast enables or disables auto-cast for a pet spell.
func (s *Server) onPetToggleAutocast(mapID, instanceID uint32, petGUID uint64, spellID uint32, enable bool) {
	if s == nil || petGUID == 0 || spellID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motion := s.motionMapLocked(mapID, instanceID)[petGUID]
	if motion == nil {
		return
	}

	foundIdx := -1
	for i, sp := range motion.AutocastSpells {
		if sp == spellID {
			foundIdx = i
			break
		}
	}
	if enable && foundIdx == -1 {
		motion.AutocastSpells = append(motion.AutocastSpells, spellID)
	} else if !enable && foundIdx != -1 {
		motion.AutocastSpells = append(motion.AutocastSpells[:foundIdx], motion.AutocastSpells[foundIdx+1:]...)
	}
}

// triggerPetDefensive triggers the owner's controlled creatures into attack
// mode when the owner either attacks (assist) or takes damage (defend).
// Mirrors TrinityCore PetAI::OwnerAttackedBy / PetAI::OwnerAttacked
// (PetAI.cpp:240-310) and the base CreatureAI::OnOwnerCombatInteraction
// (CreatureAI.cpp:126-133): both fire for every non-passive controlled
// creature with no live victim, regardless of react state or command —
// defensive-only/follow-only gating is not in the C++.
// (PetAI::_AttackStart: stay-commanded pets attack without chasing.)
func (s *Server) triggerPetDefensive(mapID, instanceID uint32, ownerGUID, targetGUID uint64) {
	if s == nil || ownerGUID == 0 || targetGUID == 0 || ownerGUID == targetGUID {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, m := range s.motionMapLocked(mapID, instanceID) {
		if m.OwnerGUID != ownerGUID || m.Health == 0 {
			continue
		}
		// PetAI::OwnerAttackedBy / PetAI::OwnerAttacked: passive pets don't
		// do anything; a pet with a live victim does not disengage
		// (PetAI::AttackStart's victim-alive guard).
		if m.PetReact == PetReactPassive {
			continue
		}
		if m.TargetGUID != 0 && m.InCombat {
			continue
		}
		m.TargetGUID = targetGUID
		m.InCombat = true
		m.Moving = m.PetCommand != PetCommandStay
	}
}

// triggerPetOwnerAttacked mirrors PetAI::OwnerAttacked (PetAI.cpp:292-312),
// called from Spell::_cast (Spell.cpp:3293-3304): when a player casts a
// harmful spell (DmgClass != SPELL_DAMAGE_CLASS_NONE, which is
// spellEntry->DefenseType in this TrinityCore version, SpellInfo.cpp:856),
// every controlled creature's AI is told the owner attacked the explicit unit
// target. Passive pets ignore the call, and a pet with a live victim does
// not disengage (PetAI::AttackStart's victim-alive guard); other pets start
// attacking the target. Go pets are creature motions with OwnerGUID set, so
// the motion-map scan replaces the m_Controlled loop; the DmgClass != NONE
// gate keeps non-damaging spells such as Hunter's Mark from triggering the
// pet (Spell.cpp:3301-3302).
func (s *Server) triggerPetOwnerAttacked(mapID, instanceID uint32, ownerGUID, targetGUID uint64) {
	if s == nil || ownerGUID == 0 || targetGUID == 0 || ownerGUID == targetGUID {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, m := range s.motionMapLocked(mapID, instanceID) {
		if m.OwnerGUID != ownerGUID || m.Health == 0 {
			continue
		}
		// PetAI::OwnerAttacked: passive pets don't do anything.
		if m.PetReact == PetReactPassive {
			continue
		}
		// PetAI::AttackStart: prevent the pet from disengaging from its
		// current target.
		if m.TargetGUID != 0 && m.InCombat {
			continue
		}
		m.TargetGUID = targetGUID
		m.InCombat = true
		m.Moving = true
	}
}

// updatePetMotion is the per-tick AI driver for active player pets.
func (s *Server) updatePetMotion(ctx context.Context, motion *creatureMotion, players []playerPos, now time.Time) {
	if s == nil || motion == nil || motion.OwnerGUID == 0 {
		return
	}

	var owner *playerPos
	for i := range players {
		if players[i].GUID == motion.OwnerGUID && players[i].Map == motion.Map && players[i].InstanceID == motion.InstanceID {
			owner = &players[i]
			break
		}
	}
	if owner == nil || owner.IsDead || owner.Map != motion.Map {
		motion.InCombat = false
		motion.TargetGUID = 0
		motion.Moving = false
		return
	}

	// 1. Pet Stay Command
	if motion.PetCommand == PetCommandStay {
		if motion.InCombat && motion.TargetGUID != 0 {
			s.petCombatPursuitAndAttack(ctx, motion, owner, now)
		}
		return
	}

	// 2. Pet in Combat (either via CommandAttack or Defensive/Aggressive aggro)
	if motion.InCombat && motion.TargetGUID != 0 {
		s.petCombatPursuitAndAttack(ctx, motion, owner, now)
		return
	}

	// 3. Pet Aggressive Reaction: scan for nearby hostiles within 15 yards
	if motion.PetReact == PetReactAggressive && motion.PetCommand != PetCommandStay {
		hostileGUID := s.findNearbyPetHostile(motion, 15.0, players)
		if hostileGUID != 0 {
			motion.TargetGUID = hostileGUID
			motion.InCombat = true
			motion.Moving = true
			s.petCombatPursuitAndAttack(ctx, motion, owner, now)
			return
		}
	}

	// 4. Follow Owner
	if motion.PetCommand == PetCommandFollow {
		dist := float32(math.Hypot(float64(owner.X-motion.X), float64(owner.Y-motion.Y)))
		// Follow stop/resume distances use both combat reaches
		// (GetNearPoint / PositionOkay, Object.cpp:3307 /
		// FollowMovementGenerator.cpp:43-50).
		ownerReach := float32(1.5)
		if owner.Sess != nil && owner.Sess.player != nil && owner.Sess.player.CombatReach > 0 {
			ownerReach = owner.Sess.player.CombatReach
		}
		petReach := motion.CombatReach
		if petReach <= 0 {
			petReach = 1.5
		}
		if dist > 45.0 {
			// Teleport directly to owner if too far
			motion.X = owner.X + 1.5
			motion.Y = owner.Y + 1.5
			motion.Z = owner.Z
			if owner.Sess != nil && owner.Sess.player != nil {
				motion.Orientation = owner.Sess.player.Orientation
			}
			motion.Moving = false
			s.broadcastMonsterMoveStopInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z)
		} else if dist > petFollowDist+petFollowRangeTolerance+ownerReach+petReach {
			// Run to catch up with owner.
			// FollowMovementGenerator (FollowMovementGenerator.cpp:57-140):
			// the pet holds ChaseAngle(PI/2, PI/4) relative to the owner
			// (PET_FOLLOW_ANGLE, PetDefines.h:85; GetFollowAngle, Unit.h:1655):
			// its current relative angle is kept while inside [PI/4, 3PI/4]
			// and clamped to the nearer bound otherwise
			// (FollowMovementGenerator.cpp:122-135). The destination is the
			// near point at PET_FOLLOW_DIST plus both combat reaches
			// (WorldObject::GetNearPoint, Object.cpp:3307). The walk-when-the-
			// owner-walks arm (init.SetWalk(target->IsWalking()),
			// FollowMovementGenerator.cpp:155) has no bridge — Go models no
			// player walk state. init.SetFacing(target->GetOrientation())
			// (FollowMovementGenerator.cpp:156) faces the pet at the owner's
			// orientation for the whole spline. The stop distance is the
			// combat reaches + PET_FOLLOW_DIST (PositionOkay,
			// FollowMovementGenerator.cpp:43-50) with the launch hysteresis
			// adding FOLLOW_RANGE_TOLERANCE. The 45-yard teleport above has no
			// C++ arm — a Go pragmatism (the generator would keep running
			// indefinitely), documented here rather than bridged.
			ownerOrient := float32(0)
			if owner.Sess != nil && owner.Sess.player != nil {
				ownerOrient = owner.Sess.player.Orientation
			}
			relAngle := math.Atan2(float64(motion.Y-owner.Y), float64(motion.X-owner.X)) - float64(ownerOrient)
			for relAngle < 0 {
				relAngle += 2 * math.Pi
			}
			for relAngle >= 2*math.Pi {
				relAngle -= 2 * math.Pi
			}
			const followAngleCenter = math.Pi / 2
			const followAngleTol = math.Pi / 4
			upperBound, lowerBound := followAngleCenter+followAngleTol, followAngleCenter-followAngleTol
			chosenAngle := relAngle
			if relAngle < lowerBound || relAngle > upperBound {
				diffUpper := relAngle - upperBound
				for diffUpper < 0 {
					diffUpper += 2 * math.Pi
				}
				diffLower := lowerBound - relAngle
				for diffLower < 0 {
					diffLower += 2 * math.Pi
				}
				if diffUpper < diffLower {
					chosenAngle = upperBound
				} else {
					chosenAngle = lowerBound
				}
			}
			absAngle := float64(ownerOrient) + chosenAngle
			nearDist := float64(petFollowDist + ownerReach + petReach)
			destX := owner.X + float32(nearDist*math.Cos(absAngle))
			destY := owner.Y + float32(nearDist*math.Sin(absAngle))
			destZ := owner.Z
			motion.Orientation = ownerOrient
			speed := creatureSplineVelocity(motion, false)
			if speed <= 0 {
				speed = 7.0
			}
			duration := splineDurationMs(float64(dist), speed)
			s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, false, ownerOrient, true)
			motion.X = destX
			motion.Y = destY
			motion.Z = destZ
			motion.Moving = true
			motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
		} else {
			motion.Moving = false
		}
	}
}

// findNearbyPetHostile scans for nearby hostile creatures within range.
func (s *Server) findNearbyPetHostile(pet *creatureMotion, maxDist float32, players []playerPos) uint64 {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for guid, m := range s.motionMapLocked(pet.Map, pet.InstanceID) {
		if guid == pet.GUID || m == nil || m.OwnerGUID != 0 || m.Health == 0 || m.InstanceID != pet.InstanceID {
			continue
		}
		dist := float32(math.Hypot(float64(m.X-pet.X), float64(m.Y-pet.Y)))
		if dist <= maxDist && pet.Faction != m.Faction {
			return guid
		}
	}
	return 0
}

// petCombatPursuitAndAttack handles pathing towards and attacking the pet's target.
func (s *Server) petCombatPursuitAndAttack(ctx context.Context, motion *creatureMotion, owner *playerPos, now time.Time) {
	// Look up target
	targetGUID := motion.TargetGUID
	var targetX, targetY, targetZ float32
	var targetHealth, targetArmor uint32
	var targetLevel uint8
	targetFound := false
	isTargetPlayer := false
	var targetSess *session

	// Check if target is player
	if pSess := s.findSessionByGUID(targetGUID); pSess != nil && pSess.player != nil {
		targetFound = true
		isTargetPlayer = true
		targetSess = pSess
		targetX, targetY, targetZ = pSess.player.X, pSess.player.Y, pSess.player.Z
		targetHealth = pSess.player.Health
		targetArmor = pSess.player.Armor
		targetLevel = pSess.player.Level
	} else {
		// Check if target is creature
		s.motionMu.Lock()
		cMotion := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, targetGUID)
		if cMotion != nil {
			targetFound = true
			targetX, targetY, targetZ = cMotion.X, cMotion.Y, cMotion.Z
			targetHealth = cMotion.Health
			targetArmor = cMotion.Armor
			targetLevel = uint8(cMotion.Level)
		}
		s.motionMu.Unlock()
	}

	// If target is missing or dead, disengage
	// The ghost leg mirrors WorldObject::IsValidAttackTarget's "can't attack
	// dead" (Object.cpp:2935-2937): ghosts carry Health=1 in Go (death.go:473)
	// but IsAlive() is false in C++ (deathState CORPSE).
	if !targetFound || targetHealth == 0 || (isTargetPlayer && targetSess != nil && targetSess.isDeadOrGhost()) {
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.Moving = false
		if motion.PetCommand == PetCommandAttack {
			motion.PetCommand = PetCommandFollow
		}
		stopPkt := buildAttackStop(motion.GUID, targetGUID, false)
		s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
		return
	}

	// PetAI::NeedToStop owner-distance arm (PetAI.cpp:575-578): a pet stops
	// attacking when it strays beyond the owner's visibility range minus 10
	// yards. C++ GetVisibilityRange on continents is 100 (ObjectDefines.h:35),
	// matching Go's VisibilityDistanceContinents default; the per-map-type
	// C++ ranges (instances/BGs) have no Go config analog. The charmed-victim
	// arm (PetAI.cpp:572-573) is unmodeled — charmed creatures are
	// player-driven in Go with no victim-vs-charmer tracking.
	if owner != nil {
		visRange := float32(100)
		if s != nil && s.Config.VisibilityDistanceContinents > 0 {
			visRange = float32(s.Config.VisibilityDistanceContinents)
		}
		if ownerDist := float32(distance3D(owner.X, owner.Y, owner.Z, motion.X, motion.Y, motion.Z)); ownerDist >= visRange-10.0 {
			// PetAI::StopAttack (PetAI.cpp:583-601): clear the victim and
			// combat state, drop a command-attack order, and let the next
			// tick's follow arm return the pet (HandleReturnMovement analog).
			motion.TargetGUID = 0
			motion.InCombat = false
			motion.Moving = false
			if motion.PetCommand == PetCommandAttack {
				motion.PetCommand = PetCommandFollow
			}
			stopPkt := buildAttackStop(motion.GUID, targetGUID, false)
			s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
			return
		}
	}

	dist := float32(math.Hypot(float64(targetX-motion.X), float64(targetY-motion.Y)))
	meleeRange := float32(3.5)

	if dist > meleeRange {
		if motion.PetCommand == PetCommandStay {
			// PetAI::UpdateAI COMMAND_STAY arm (PetAI.cpp:84-89): a
			// stay-commanded pet only swings when the victim is within
			// melee range — out of range it holds position with no swing,
			// no chase, and no enemy autocast (C++ enemy-targeted autocast
			// also gates on CanAttack, PetAI.cpp:144-150). An explicit
			// attack order replaces Stay with PetCommandAttack in Go, so
			// the C++ stay+command-attack chase arm is subsumed by the
			// chase leg below. The victim is kept: C++ keeps it until
			// NeedToStop or death clears it.
			motion.Moving = false
			return
		}
		// Run towards target
		dx := targetX - motion.X
		dy := targetY - motion.Y
		angle := float32(math.Atan2(float64(dy), float64(dx)))
		motion.Orientation = angle
		destX := targetX - 1.5*float32(math.Cos(float64(angle)))
		destY := targetY - 1.5*float32(math.Sin(float64(angle)))
		destZ := targetZ
		speed := motion.RunSpeed
		if speed <= 0 {
			speed = 7.0
		}
		duration := uint32(float64(dist) / float64(speed) * 1000.0)
		if duration < 200 {
			duration = 200
		}
		motion.X = destX
		motion.Y = destY
		motion.Z = destZ
		motion.Moving = true
		s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, false, 0, false)
	} else {
		motion.Moving = false

		// Melee attack
		attackTime := time.Duration(motion.AttackTime) * time.Millisecond
		if attackTime <= 0 {
			attackTime = 2 * time.Second
		}
		if motion.LastAttack.IsZero() || now.Sub(motion.LastAttack) >= attackTime {
			s.executePetMeleeAttack(ctx, motion, owner.Sess, targetGUID, isTargetPlayer, targetSess, targetHealth, targetArmor, targetLevel, now)
			motion.LastAttack = now
		}

		// Autocast spells
		if len(motion.AutocastSpells) > 0 && (motion.LastSpell.IsZero() || now.Sub(motion.LastSpell) >= 3*time.Second) {
			s.executePetAutocast(ctx, motion, targetGUID, now)
		}
	}
}

// executePetMeleeAttack conducts the pet's physical swing on the target.
// owner is the pet owner's session (from the pet tick's owner lookup); it
// runs the full death chain on a creature kill.
func (s *Server) executePetMeleeAttack(ctx context.Context, motion *creatureMotion, owner *session, targetGUID uint64, isTargetPlayer bool, targetSess *session, targetHealth, targetArmor uint32, targetLevel uint8, now time.Time) {
	damage := uint32(float64(motion.MinDamage) + rand.Float64()*float64(motion.MaxDamage-motion.MinDamage))
	if damage < 1 {
		damage = 1
	}

	if targetArmor > 0 {
		damage = calcArmorReducedDamage(float64(targetArmor), uint8(motion.Level), damage)
	}

	victimDodgeBP := int32(-1)
	if isTargetPlayer && targetSess != nil && targetSess.player != nil {
		victimDodgeBP = int32(math.Round(float64(targetSess.player.DodgePercentage) * 100))
	}
	outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, false, isTargetPlayer, false, false, false, true, 0, 0, 0, 0, victimDodgeBP)
	// Pet attackers carry no Go aura model, so the 248 dodge-reduction arm
	// (Unit.cpp:2694-2695) stays unbridged on this path.
	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitCrit:
		damage *= 2
	}

	overkill := uint32(0)
	if damage >= targetHealth {
		overkill = damage - targetHealth
		damage = targetHealth
	}

	asuPkt := protocol.BuildAttackerStateUpdate(motion.GUID, targetGUID, damage, overkill, hitInfo, targetState, 0)
	if isTargetPlayer && targetSess != nil {
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, true)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, targetSess)
		// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
		// after the attacker-state update (sent pre-DealDamage in C++).
		damage = targetSess.negateGodModeDamage(damage)
		// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
		if owner != nil && damage > 0 {
			owner.splitShareDamagePct(ctx, targetGUID, true, creatureAuraKey{}, motion.GUID, damage, 1)
		}
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED fires
		// for any damage to a player victim, pet attackers included.
		targetSess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
		// creatures are signaled OwnerAttackedBy on any non-DoT damage.
		s.triggerPetDefensive(targetSess.player.Map, targetSess.player.InstanceID, targetGUID, motion.GUID)
		// Duel defeat (Unit.cpp:825-853, 957-973): a pet is controlled by its owner
		// (GetControllingPlayer, Unit.cpp:5996) — the blow lands as a duel
		// defeat when the owner is the victim's duel opponent; a lethal blow
		// from anyone else's pet kills and interrupts instead.
		var petOwnerGUID uint64
		if owner != nil && owner.player != nil {
			petOwnerGUID = owner.playerGUID
		}
		if duelDefeatOnDamage(targetSess, petOwnerGUID, damage, targetHealth) {
			// Duel defeat consumed the hit — loser at 1 HP, duel complete.
		} else if damage >= targetHealth {
			targetSess.player.Health = 0
			targetSess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
			// GetCharmerOrOwnerPlayerOrPlayerItself (Unit.cpp:11164) resolves a
			// player-owned pet to its owner: PvP death.
			targetSess.killPlayer(ctx, owner, true, true)
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): the attacker is the
			// pet — C++ Unit::Kill fires the pet arm (the owner's pet is the
			// attacker itself) AND the player-victim branch arm, i.e. two
			// dispatches with the pet (Unit.cpp pet arm + 11359-11361).
			s.fireCreatureTargetDied(ctx, motion, targetSess.luaPlayer())
			s.fireCreatureTargetDied(ctx, motion, targetSess.luaPlayer())
		} else {
			targetSess.player.Health -= damage
			// Unit::DealDamage (Unit.cpp:896-897): pet melee lands as
			// DIRECT_DAMAGE (DealMeleeDamage, Unit.cpp:1520), so a non-lethal
			// hit strips DIRECT_DAMAGE-interrupt auras alongside the
			// TAKE_DAMAGE strip — the absorbed arm above (742-747) only ran
			// the TAKE_DAMAGE half.
			targetSess.procDamageAuras(true, damage)
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
			targetSess.grantRageFromDamageTaken(ctx, damage)
			// Unit::DealDamage (Unit.cpp:906-913): random durability loss on
			// HIT TAKEN — the victim is a player.
			targetSess.rollDurabilityLossOnHit(ctx, damage)
			// Unit::DealDamage (Unit.cpp:934-952): pushback — pet melee
			// carries no spellProto, so the victim's cast/channel is always
			// delayed on damage > 0, like the C++ null-spellProto path.
			if damage > 0 {
				targetSess.delayCurrentCast()
				targetSess.delayCurrentChannel()
			}
			targetSess.sendPlayerUpdate()
		}
	} else {
		// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
		if owner != nil && damage > 0 {
			owner.splitShareDamagePct(ctx, targetGUID, false, creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}, motion.GUID, damage, 1)
		}
		s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, nil)
		var killedTarget combatTarget
		var killed bool
		var killX, killY, killZ float32
		s.motionMu.Lock()
		cMotion := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, targetGUID)
		if cMotion != nil {
			// Unit::DealDamage tap block (Unit.cpp:872-876): pet damage
			// taps for the owner (Creature::SetLootRecipient's
			// GetCharmerOrOwnerPlayerOrPlayerItself).
			var tapPlayer, tapGroup uint64
			if owner != nil && owner.player != nil {
				tapPlayer, tapGroup = owner.playerGUID, owner.groupID
			}
			newlyTapped := s.recordCreatureTap(cMotion, tapPlayer, tapGroup, damage, cMotion.Health)
			if damage >= cMotion.Health {
				// Snapshot the pre-kill target for the death chain, like the
				// player swing path's getCombatTarget-before-damage.
				killedTarget = combatTarget{
					GUID:       cMotion.GUID,
					Map:        cMotion.Map,
					InstanceID: cMotion.InstanceID,
					X:          cMotion.X,
					Y:          cMotion.Y,
					Z:          cMotion.Z,
					Health:     cMotion.Health,
					MaxHealth:  cMotion.MaxHealth,
					Level:      uint8(cMotion.Level),
				}
				killX, killY, killZ = cMotion.X, cMotion.Y, cMotion.Z
				cMotion.Health = 0
				cMotion.DynamicFlags |= unitDynFlagLootable
				cMotion.InCombat = false
				cMotion.TargetGUID = 0
				cMotion.Moving = false
				if cMotion.ThreatMgr != nil {
					cMotion.ThreatMgr.ClearThreat()
				}
				// The corpse keeps TAPPED when the tap survived to death
				// (Unit.cpp:11172-11175), so broadcast the real flags.
				s.broadcastCreatureValuesUpdateInInstance(cMotion.Map, cMotion.InstanceID, targetGUID, map[int]uint32{unitFieldHealth: 0, unitFieldDynamicFlags: cMotion.DynamicFlags})
				killed = true
			} else {
				cMotion.Health -= damage
				healthUpdate := map[int]uint32{unitFieldHealth: cMotion.Health}
				// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
				if next, changed := s.addCreatureRageLocked(cMotion, cMotion.Level, damage); changed {
					healthUpdate[unitFieldPower1+powerRage] = next
				}
				if newlyTapped {
					healthUpdate[unitFieldDynamicFlags] = cMotion.DynamicFlags
				}
				s.broadcastCreatureValuesUpdateInInstance(cMotion.Map, cMotion.InstanceID, targetGUID, healthUpdate)
			}
		}
		s.motionMu.Unlock()
		if killed {
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): pet attacker → pet arm
			// + branch arm double dispatch (Unit.cpp pet arm + 11385-11387),
			// fired after the lootable flag, ahead of event 4 — all inside
			// the full death chain below (XP/loot/respawn/event 4), matching
			// the pet-spell kill path and C++ Unit::Kill.
			s.stopCreatureMotionInInstance(motion.Map, motion.InstanceID, targetGUID, killX, killY, killZ)
			s.broadcastThreatClearInInstance(motion.Map, motion.InstanceID, targetGUID)
			if owner != nil && owner.player != nil {
				owner.onCreatureKilled(ctx, killedTarget, motion)
			} else if victim := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); victim != nil {
				// Owner session gone mid-tick: the full chain needs the
				// owner's session, so keep the pre-existing minimal
				// registration (health zero + lootable flag broadcast above)
				// and the event-3 double dispatch.
				// ThreatManager::RemoveMeFromThreatLists (ThreatManager.cpp:690-697):
				// this path skips onCreatureKilled, so the death-side victim
				// drop must run here — otherwise the dead creature stays a
				// victim in every other table on the map/instance.
				s.removeThreatVictimFromAllLists(motion.Map, motion.InstanceID, targetGUID)
				s.fireCreatureTargetDied(ctx, motion, s.luaMotionCreature(victim))
				s.fireCreatureTargetDied(ctx, motion, s.luaMotionCreature(victim))
				// BossAI::_JustDied (ScriptedCreature.cpp:512-518): this
				// path skips onCreatureKilled, so the native death hook must
				// fire here — vancleefAI.OnDied despawns the 50%-arm
				// blackguards even when the pet owner is gone mid-tick.
				if victim.BossAI != nil {
					victim.BossAI.OnDied(ctx, s, victim)
				}
			}
		}
	}
}

// petCanAutoCastTarget mirrors the aura-stack precheck of Spell::CanAutoCast
// (Spell.cpp:6461-6505): before a pet fires an autocast spell, C++ verifies
// the target does not already carry the same spell or a stack-rule-exclusive
// peer of any of the spell's aura effects — same spell ID, an EXCLUSIVE
// group peer, an EXCLUSIVE_FROM_SAME_CASTER peer from this caster, or an
// EXCLUSIVE_HIGHEST peer whose amount meets or beats this spell's base
// points (abs(BasePoints) <= abs(peer amount)). EXCLUSIVE_SAME_EFFECT
// carries no false arm in C++ ("further checks not necessary for autocast
// logic"). Go's executePetAutocast fired the spell unconditionally, so pets
// re-cast buffs onto already-buffed allies (and re-applied exclusive
// debuffs) every tick. The CheckPetCast / SelectSpellTargets containment
// tail of CanAutoCast (Spell.cpp:6506-6523) has no Go counterpart on the pet
// path — Go gates power through takePetSpellPower and targets the combat
// target directly — so it stays unbridged, as does PetAI's enemy-then-ally
// target selection (PetAI.cpp:144-190).
func (s *Server) petCanAutoCastTarget(ctx context.Context, owner *session, caster *creatureMotion, spell wotlk.Spell, targetGUID uint64) bool {
	if s == nil || s.Data == nil || caster == nil || targetGUID == 0 {
		return true
	}
	hasAuraEffect := false
	for _, eff := range spell.Effects {
		if spellEffectIsAuraEffect(eff) {
			hasAuraEffect = true
			break
		}
	}
	if !hasAuraEffect {
		return true
	}
	type auraPeer struct {
		spellID    uint32
		casterGUID uint64
		amounts    [3]int32
		mask       uint8
	}
	var peers []auraPeer
	if sess := s.findSessionByGUID(targetGUID); sess != nil && sess.player != nil {
		sess.castMu.Lock()
		for _, a := range sess.activeAuras {
			if a == nil || a.Stopped {
				continue
			}
			peers = append(peers, auraPeer{a.SpellID, a.CasterGUID, a.Amounts, a.EffectMask})
		}
		sess.castMu.Unlock()
	} else if owner != nil {
		if target, ok := owner.getCombatTarget(ctx, targetGUID); ok {
			s.auraMu.Lock()
			for _, a := range s.activeCreatureAuras[creatureAuraKeyForTarget(target)] {
				if a == nil || a.Stopped {
					continue
				}
				peers = append(peers, auraPeer{a.SpellID, a.CasterGUID, a.Amounts, a.EffectMask})
			}
			s.auraMu.Unlock()
		}
	}
	ownFirst := s.spellFirstRank(spell.ID)
	for _, eff := range spell.Effects {
		if !spellEffectIsAuraEffect(eff) {
			continue
		}
		ownBP := absAuraAmount(eff.BasePoints)
		for _, p := range peers {
			exSpell, found, err := s.Data.Spell(p.spellID)
			if err != nil || !found {
				continue
			}
			for index, exEff := range exSpell.Effects {
				if index >= 8 || exEff.Aura != eff.Aura {
					continue
				}
				if p.mask&(1<<uint(index)) == 0 {
					continue
				}
				if p.spellID == spell.ID {
					return false
				}
				switch s.spellGroupStackRule(ownFirst, s.spellFirstRank(p.spellID)) {
				case spellGroupStackRuleExclusive:
					return false
				case spellGroupStackRuleExclusiveSameCaster:
					if caster.GUID == p.casterGUID {
						return false
					}
				case spellGroupStackRuleExclusiveHighest:
					if ownBP <= absAuraAmount(p.amounts[index]) {
						return false
					}
				}
			}
		}
	}
	return true
}

// executePetAutocast casts the highest priority available pet autocast spell.
func (s *Server) executePetAutocast(ctx context.Context, motion *creatureMotion, targetGUID uint64, now time.Time) {
	if s == nil || motion == nil || len(motion.AutocastSpells) == 0 || s.Data == nil {
		return
	}
	spellID := motion.AutocastSpells[0]
	owner := s.findSessionByGUID(motion.OwnerGUID)
	spell, found, err := s.Data.Spell(spellID)
	if owner == nil || err != nil || !found {
		return
	}
	// Spell::CanAutoCast aura-stack precheck (Spell.cpp:6461-6505): C++
	// skips the autocast when the target already carries the spell or a
	// stack-exclusive peer of one of its aura effects.
	if !s.petCanAutoCastTarget(ctx, owner, motion, spell, targetGUID) {
		return
	}
	owner.executePetSpell(ctx, motion, spell, 0, protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: targetGUID})
}

func (s *session) executePetSpell(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, castCount uint8, target protocol.SpellTargetData) bool {
	return s.executePetSpellWithOptions(ctx, motion, spell, castCount, target, false)
}

// petAreaVictimPos resolves the pet spell's wire target to a position for
// the area-selection twin's DEST-ref center fallback. getCombatTarget
// resolves both player victims (pet autocast at its attack target) and
// creature-motion victims; the twin's `seen` map uses the wire GUID to
// exclude the explicit target from the area hits.
func (s *session) petAreaVictimPos(ctx context.Context, motion *creatureMotion, targetGUID uint64) *playerPos {
	if s == nil || s.server == nil || motion == nil || targetGUID == 0 {
		return nil
	}
	if target, ok := s.getCombatTarget(ctx, targetGUID); ok {
		return &playerPos{Map: target.Map, InstanceID: target.InstanceID, X: target.X, Y: target.Y, Z: target.Z, GUID: targetGUID, Sess: s.server.findSessionByGUID(targetGUID)}
	}
	return nil
}

// checkPetSpellCheckTarget is the pet-caster twin of the explicit-target
// SpellInfo::CheckTarget block in Spell::CheckCast (Spell.cpp:5372 —
// CheckTarget(m_caster, target, implicit=false) for a creature caster).
// C++ relative order: between the conditions block (5337-5348) and the
// behind/facing arms (5379-5384); runs for triggered casts too (no C++
// triggered exemption). Returns false when the cast must fizzle. Only
// arms with a Go motion model are bridged; the rest are documented
// below, matching the player path (spells.go) where it bridged them.
// The client-initiated gauntlet (pets.go checkPetSpellTarget) already
// sends fail codes for the CANT_TARGET_SELF and GM/invisibility arms;
// this is the shared backstop for AI and triggered pet casts, which
// fizzle silently like the other CheckCast twins here.
func (s *session) checkPetSpellCheckTarget(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, targetGUID uint64, explicitUnitTarget bool) bool {
	if s == nil || motion == nil {
		return true
	}
	// C++ only enters the block for m_targets.GetUnitTarget() != null.
	if !explicitUnitTarget {
		return true
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1637-1638):
	// SPELL_ATTR1_CANT_TARGET_SELF with caster == target.
	if spell.AttributesEx&spellAttr1CantTargetSelf != 0 && targetGUID == motion.GUID {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1641-1642): the CanSeeOrDetect
	// visibility gate is unbridged — Go has no stealth/invisibility-level
	// model on either side (documented no-bridge, like the player path).
	// SpellInfo::CheckTarget (SpellInfo.cpp:1650-1651):
	// SPELL_ATTR1_CANT_TARGET_IN_COMBAT with an in-combat target. The
	// UNIT_FLAG_PET_IN_COMBAT half has no Go model (unitTargetInCombat).
	if spell.AttributesEx&spellAttr1CantTargetInCombat != 0 && s.unitTargetInCombat(targetGUID) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1654-1662):
	// SPELL_ATTR3_ONLY_TARGET_GHOSTS — no Go model (target-side ghost aura
	// state is unmodeled); documented no-bridge.
	// SpellInfo::CheckTarget (SpellInfo.cpp:1667-1692): the CANT_TARGET_TAPPED,
	// PICKPOCKET and DISARM arms sit inside C++'s
	// `caster->GetTypeId() == TYPEID_PLAYER` block — vacuous for a pet
	// caster, so they are correctly absent here.
	// SpellInfo::CheckTarget (SpellInfo.cpp:1697-1710): corpse targets —
	// no Go corpse-target model on the pet path; documented no-bridge.
	// SpellInfo::CheckTarget (SpellInfo.cpp:1712-1713):
	// SPELL_ATTR3_ONLY_TARGET_PLAYERS with a non-player target.
	if spell.AttributesEx3&spellAttr3OnlyTargetPlayers != 0 {
		if _, isPlayer := s.targetCreatureTypeMask(ctx, targetGUID); !isPlayer {
			return false
		}
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1715-1716): dead targets fail
	// unless the spell allows them (IsAllowingDeadTarget, 1177).
	// Unresolvable targets fail open, like the LOS arms below.
	if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
		if tgt.Health == 0 && !spellAllowsDeadTarget(spell) {
			return false
		}
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1718-1720): the implicit-only
	// crowd-control gate is vacuous — C++ passes implicit=false for
	// creature casters. (SpellInfo.cpp:1723 UNIT_FLAG_NOT_SELECTABLE is
	// commented out in C++; vacant.)
	// SpellInfo::CheckTarget (SpellInfo.cpp:1728): target creature-type
	// gate, ahead of the aura-state/aura-spell gates in C++ order.
	if failReason := s.checkTargetCreatureType(ctx, spell, targetGUID); failReason != 0 {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1736-1743): GM-mode /
	// GM-invisibility. The pet caster's GetAffectingPlayer is the owner
	// session, so the gate fires for harmful and helpful casts alike,
	// like C++; self-targets are exempt (unitTarget != caster).
	if targetGUID != motion.GUID && s.server.gmAttackTargetBlocked(targetGUID) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1745-1747): in-flight targets.
	if s.explicitTargetFlyingBlocked(spell, targetGUID) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1757-1766): target aura-state
	// requirements. C++ skips both arms for vehicle casters and when the
	// target is the caster's charmer/owner — the pet's owner (or charmer)
	// is exempt here. Documented delta: targetHasAuraState filters
	// per-caster attributed auras on the owner's GUID, while pet-applied
	// auras record the pet's motion GUID as caster.
	charmerExempt := targetGUID == motion.OwnerGUID || (motion.CharmOwnerGUID != 0 && targetGUID == motion.CharmOwnerGUID)
	if !charmerExempt {
		if spell.TargetAuraState != 0 && !s.targetHasAuraState(ctx, targetGUID, spell.TargetAuraState, spell) {
			return false
		}
		if spell.ExcludeTargetAuraState != 0 && s.targetHasAuraState(ctx, targetGUID, spell.ExcludeTargetAuraState, spell) {
			return false
		}
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1769-1773): target aura-spell
	// requirements (no charmer exemption in C++).
	if spell.TargetAuraSpell != 0 && !s.targetHasAura(ctx, targetGUID, spell.TargetAuraSpell) {
		return false
	}
	if spell.ExcludeTargetAuraSpell != 0 && s.targetHasAura(ctx, targetGUID, spell.ExcludeTargetAuraSpell) {
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1776-1778):
	// SPELL_AURA_PREVENT_RESURRECTION — unbridged on the player path too;
	// documented no-bridge parity.
	return true
}

func (s *session) executePetSpellWithOptions(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, castCount uint8, target protocol.SpellTargetData, triggered bool) bool {
	if s == nil || s.server == nil || motion == nil {
		return false
	}
	targetGUID := target.UnitGUID
	explicitUnitTarget := targetGUID != 0
	if targetGUID == 0 {
		targetGUID = motion.GUID
	}
	// Spell::CheckCast gates (Spell.cpp:5247-5496) for a pet caster: the
	// pet is a creatureMotion, so the creature-caster twins apply unchanged
	// (boss_ai.go). C++ relative order: the shapeshift leg (5247), the
	// SPELL_AURA_BLOCK_SPELL_FAMILY leg (5277-5278) and the caster-state
	// block (5298-5314) precede the conditions block (5337-5348), which
	// precedes the explicit-target CheckTarget block (5353-5377), the
	// behind/facing arms (5379-5384), the unit-target
	// line-of-sight arm (5391-5401), the dest line-of-sight arm (5403-5409),
	// the spell-focus arm (5477-5481), and CheckRange (5496) last. All of
	// them precede the power drain (TakePower, Spell.cpp:3449 runs after
	// CheckCast), so a failing gate fizzles the cast before
	// takePetSpellPower. The !triggered gate mirrors the creature path
	// (TRIGGERED_IGNORE_SHAPESHIFT / TRIGGERED_IGNORE_CASTER_AURASTATE);
	// conditions, behind/facing, LOS, focus and range run for triggered
	// casts too, like C++ (no triggered exemption).
	if !triggered {
		if !s.server.checkCreatureSpellShapeshift(motion, spell) {
			return true
		}
		if !s.server.checkCreatureSpellFamilyBlocked(motion, spell) {
			return true
		}
		if !s.server.checkCreatureSpellCasterState(motion, spell) {
			return true
		}
	}
	if !s.server.checkCreatureSpellCastConditions(ctx, motion, spell.ID, targetGUID) {
		return true
	}
	// SpellInfo::CheckExplicitTarget null-target arm (SpellInfo.cpp:1784-1790):
	// a spell whose explicit target mask needs a target fizzles when none
	// was provided. The client-initiated gauntlet (pets.go) already covers
	// this via Spell::CheckPetCast (Spell.cpp:6230-6235,
	// SPELL_FAILED_BAD_IMPLICIT_TARGETS); this is the shared backstop for
	// AI and triggered pet casts. C++ relative order: CheckExplicitTarget
	// (5365) precedes CheckTarget (5372).
	if !explicitUnitTarget && spellNeedsExplicitUnitTarget(spell) {
		return true
	}
	// SpellInfo::CheckExplicitTarget faction arms (SpellInfo.cpp:1793-1811):
	// the enemy/ally/party/raid/minipet/passenger verdict on the explicit
	// unit target. C++ relative order: CheckExplicitTarget (5365) runs
	// ahead of CheckTarget (5372). The pet's affecting player is the owner
	// session. Only truly explicit targets reach the faction block — C++
	// returns before it when no target was sent (1784-1790, bridged above).
	if explicitUnitTarget && !s.server.checkCreatureExplicitTargetFaction(motion, s, spell, targetGUID) {
		return true
	}
	// SpellInfo::CheckTarget on the explicit unit target (Spell.cpp:5372),
	// between the conditions block and the behind/facing arms in C++
	// relative order. Runs for triggered casts too (no C++ triggered
	// exemption on this block).
	if !s.checkPetSpellCheckTarget(ctx, motion, spell, targetGUID, explicitUnitTarget) {
		return true
	}
	if !s.server.checkCreatureSpellBehindFacing(motion, spell, targetGUID) {
		return true
	}
	// Spell::SelectSpellTargets (Spell.cpp:3410) runs ahead of CheckCast in
	// C++'s Spell::prepare, so the dest-family resolution lands before the
	// LOS gates here; the same values feed the GO packet write below.
	dx, dy, dz, hasDest := s.server.resolveCreatureSpellDestination(motion, spell, targetGUID)
	if targetGUID != 0 && targetGUID != motion.GUID &&
		spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
		spell.AttributesEx5&spellAttr5SkipCheckcastLosCheck == 0 {
		if x, y, z, _, _, found := s.server.aiThreatUnitPos(motion, targetGUID, nil, true); found {
			if !s.server.hasLineOfSight(motion.Map, motion.X, motion.Y, motion.Z, x, y, z) {
				return true
			}
		}
	}
	if hasDest &&
		spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
		spell.AttributesEx5&spellAttr5SkipCheckcastLosCheck == 0 {
		if !s.server.hasLineOfSight(motion.Map, motion.X, motion.Y, motion.Z, dx, dy, dz) {
			return true
		}
	}
	// Spell::CheckCast pet-presence arm (Spell.cpp:5410-5432): any effect
	// with TargetA == TARGET_UNIT_PET (5) needs the caster's guardian pet
	// (Unit::GetGuardianPet). The npcbot arm is vacuous (no npcbot model);
	// pet motions carry no guardian pet, so the first such effect fizzles
	// the cast — SPELL_FAILED_NO_PET (or SPELL_FAILED_DONT_REPORT when
	// triggered by an aura; both fizzle silently here since the AI path
	// has no client to report to). C++ relative order: after the dest-LOS
	// arm (5400), ahead of the arena arm (5439). Runs for triggered casts
	// too (no C++ triggered exemption).
	if spellFirstEffectTargetsUnitPet(spell) {
		return true
	}
	// Spell::CheckCast arena arm (Spell.cpp:5439-5447): the arm carries no
	// caster-type gate — ATTR4_NOT_USABLE_IN_ARENA, or a recovery time over
	// 10 minutes without ATTR4_USABLE_IN_ARENA, fails on battle-arena maps
	// for pet casters too. (The 01:24 "vacuous" claim was wrong: the arm has
	// no player gate.) The battleground-only arm (5434-5437) and the mounted
	// arm (5464-5474) are TYPEID_PLAYER-gated, vacuous on the pet path.
	// GetRecoveryTime = max(RecoveryTime, CategoryRecoveryTime)
	// (SpellInfo.cpp:3149-3152); the map-entry-must-exist shape mirrors the
	// creature path and the C++ sMapStore lookup. Runs for triggered casts
	// too (no C++ triggered exemption).
	recoveryTime := spell.RecoveryTime
	if spell.CategoryRecoveryTime > recoveryTime {
		recoveryTime = spell.CategoryRecoveryTime
	}
	if spell.AttributesEx4&spellAttr4NotUsableInArena != 0 ||
		(recoveryTime > 10*60*1000 && spell.AttributesEx4&spellAttr4UsableInArena == 0) {
		if entry, found, err := s.server.Data.Map(motion.Map); err == nil && found && entry.IsBattleArena() {
			return true
		}
	}
	if !s.server.motionSpellFocusFound(ctx, motion, spell) {
		return true
	}
	if !s.server.checkCreatureSpellRange(motion, spell, targetGUID) {
		return true
	}
	remainingPower, hasRemainingPower, ok := s.takePetSpellPower(motion, spell, castCount, triggered)
	if !ok {
		return true
	}
	hitTargets := []uint64{targetGUID}
	// Spell::SelectImplicitAreaTargets (Spell.cpp:1227): pet casts run the
	// full target selection in C++ — an area AoE applies to every enemy unit
	// in the area, not just the wire target. The motion-anchored twin
	// (creatureAIAreaEnemyPlayers) finds the extra enemy players; the wire
	// victim is excluded from its results, and their GUIDs extend the GO
	// packet hit list below.
	var areaHits []*playerPos
	if isAreaEnemySpell(spell) {
		if victimPos := s.petAreaVictimPos(ctx, motion, targetGUID); victimPos != nil {
			areaHits = s.server.creatureAIAreaEnemyPlayers(ctx, motion, spell, victimPos)
		}
	}
	for _, areaHit := range areaHits {
		hitTargets = append(hitTargets, areaHit.GUID)
	}
	// Spell::SelectSpellTargets dest-family arms (Spell.cpp:3410) for a pet
	// caster: the same server-side destination resolution the wild-creature
	// path bridges (resolveCreatureSpellDestination) — C++ runs the full
	// SelectSpellTargets for pet casts too, seeding m_targets at the caster
	// (CheckDst, Spell.cpp:6523) and writing it into SendSpellGo
	// (Spell.cpp:4338). The implicit arms overwrite any client-supplied
	// destination, so the resolved position replaces it.
	// Spell::SelectSpellTargets dest-family arms (Spell.cpp:3410) for a pet
	// caster: the same server-side destination resolution the wild-creature
	// path bridges (resolveCreatureSpellDestination), computed above ahead
	// of the CheckCast gates — C++ runs the full SelectSpellTargets for
	// pet casts too, seeding m_targets at the caster (CheckDst,
	// Spell.cpp:6523) and writing it into SendSpellGo (Spell.cpp:4338).
	// The implicit arms overwrite any client-supplied destination, so the
	// resolved position replaces it.
	if hasDest {
		target.Flags |= protocol.SpellTargetFlagDestLocation
		target.Destination = protocol.SpellTargetLocation{X: dx, Y: dy, Z: dz}
	}
	stamp := gameTimeMS()
	castFlags := uint32(spellCastFlagGo)
	if triggered && castCount == 0 {
		castFlags |= spellCastFlagPending
	}
	if spell.StartRecoveryTime == 0 {
		castFlags |= protocol.SpellCastFlagNoGCD
	}
	var power *uint32
	if hasRemainingPower {
		castFlags |= protocol.SpellCastFlagPowerLeftSelf
		power = &remainingPower
	}
	goPacket := protocol.BuildSpellGoWithPower(motion.GUID, motion.GUID, castCount, spell.ID, castFlags, stamp, hitTargets, nil, spellGoPacketTarget(spell, target), power)
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPacket, true); err != nil {
		return false
	}
	nearbyPacket := protocol.BuildSpellGo(motion.GUID, motion.GUID, castCount, spell.ID, castFlags&^protocol.SpellCastFlagPowerLeftSelf, stamp, hitTargets, nil, spellGoPacketTarget(spell, target))
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearbyPacket, s)
	damage, hasDamage := creatureSpellDamage(s.server, spell, motion.Level, true)
	handledEffect := false
	if hasDamage {
		schoolMask := uint8(spell.SchoolMask)
		if schoolMask == 0 {
			schoolMask = 1
		}
		s.executePetSpellDamage(ctx, motion, targetGUID, spell.ID, damage, schoolMask, triggered)
		// SelectImplicitAreaTargets (Spell.cpp:1227): each area-selected
		// enemy takes its own hit — the per-target miss roll, resistance,
		// absorb and kill legs run per target inside
		// executePetSpellDamage, matching DoTargetSpellHit per target.
		for _, areaHit := range areaHits {
			s.executePetSpellDamage(ctx, motion, areaHit.GUID, spell.ID, damage, schoolMask, triggered)
		}
		handledEffect = true
	}
	for _, effect := range spell.Effects {
		if effect.Effect == 10 || effect.Effect == 105 || effect.Effect == 136 {
			heal := uint32(effect.BasePoints + 1)
			if heal > 0 {
				s.executePetSpellHeal(ctx, motion, targetGUID, spell.ID, heal)
				handledEffect = true
			}
		}
		if effect.Effect == spellEffectTriggerSpell && effect.TriggerSpell != 0 && effect.TriggerSpell != spell.ID {
			if triggered, found, err := s.server.Data.Spell(effect.TriggerSpell); err == nil && found {
				s.executePetSpellWithOptions(ctx, motion, triggered, 0, target, true)
				handledEffect = true
			}
		}
		if effect.Effect == spellEffectDispel {
			s.executePetSpellDispel(ctx, motion, targetGUID, spell, effect)
			handledEffect = true
		}
		if effect.Effect == 6 || effect.Effect == 27 || effect.Effect == 35 {
			durationMs := uint32(0)
			if spell.DurationIndex > 0 {
				if duration, found, err := s.server.Data.SpellDuration(spell.DurationIndex, casterLevel(motion)); err == nil && found && duration > 0 {
					durationMs = uint32(duration)
				}
			}
			amount := uint32(effect.BasePoints + 1)
			if s.applyPetAura(ctx, motion, spell, effect, targetGUID, durationMs, amount) {
				handledEffect = true
			}
			// SPELL_EFFECT_APPLY_AREA_AURA_ENEMY (effect 27 with an enemy
			// area target) applies to every unit the area selection picked —
			// the wire target above plus the extra area hits.
			if effect.Effect == 27 && (isAreaEnemyTargetType(effect.ImplicitTargetA) || isAreaEnemyTargetType(effect.ImplicitTargetB)) {
				for _, areaHit := range areaHits {
					if s.applyPetAura(ctx, motion, spell, effect, areaHit.GUID, durationMs, amount) {
						handledEffect = true
					}
				}
			}
		}
	}
	if !handledEffect {
		if !triggered {
			s.recordPetSpellCooldown(motion, spell, time.Now())
		}
		return true
	}
	if !triggered {
		s.recordPetSpellCooldown(motion, spell, time.Now())
	}
	return true
}

func casterLevel(motion *creatureMotion) uint32 {
	if motion == nil || motion.Level == 0 {
		return 1
	}
	return motion.Level
}

func (s *session) applyPetAura(ctx context.Context, caster *creatureMotion, spell wotlk.Spell, effect wotlk.SpellEffect, targetGUID uint64, durationMs, amount uint32) bool {
	return s.applyPetAuraWithSource(ctx, caster, spell, effect, targetGUID, durationMs, int32(amount), ownerPetAuraKey{}, 0, false)
}

func (s *session) applyPetAuraWithSource(ctx context.Context, caster *creatureMotion, spell wotlk.Spell, effect wotlk.SpellEffect, targetGUID uint64, durationMs uint32, amount int32, source ownerPetAuraKey, sourceDamage int32, removeOnChange bool) bool {
	if s == nil || s.server == nil || caster == nil || targetGUID == 0 {
		return false
	}
	effectMask, recalculateMask := PetAuraEffectMask(spell), uint8(0)
	var amounts, baseAmounts [3]int32
	for index, candidate := range spell.Effects {
		if effectMask&(1<<uint(index)) == 0 {
			continue
		}
		bit := uint8(1 << uint(index))
		amounts[index], baseAmounts[index] = candidate.CalcValueForLevel(spell, uint32(caster.Level)), candidate.BasePoints
		if candidate == effect {
			amounts[index] = amount
		}
		if auraEffectCanBeRecalculated(candidate.Aura) {
			recalculateMask |= bit
		}
	}
	if effectMask == 0 {
		return false
	}
	positive := !isHarmfulAura(effect.Aura)
	periodMs := effect.AuraPeriod
	if periodMs == 0 && (effect.Aura == 3 || effect.Aura == 8 || effect.Aura == 23 || effect.Aura == 24 || effect.Aura == 89) {
		periodMs = 3000
	}
	// AuraEffect::CalculatePeriodic (SpellAuraEffects.cpp:596-599): a pet
	// caster's SPELLMOD_ACTIVATION_TIME mods fold through the spellmod owner
	// (the pet's owner — this session). The haste legs read the pet's own
	// UNIT_MOD_CAST_SPEED, which has no Go model.
	if periodMs > 0 {
		periodMs = uint32(max(s.applySpellMod(spell, spellModActivationTime, int32(periodMs)), 1))
	}
	if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil && targetSess.player != nil {
		targetSess.castMu.Lock()
		if targetSess.activeAuras == nil {
			targetSess.activeAuras = make(map[uint32]*activeAura)
		}
		if targetSess.auras == nil {
			targetSess.auras = make(map[uint32]struct{})
		}
		if targetSess.auraSlots == nil {
			targetSess.auraSlots = make(map[uint32]uint8)
		}
		// Unit::IsHighestExclusiveAura (Unit.cpp:13991): a fresh aura whose
		// effect is strictly lower than an existing EXCLUSIVE_HIGHEST peer
		// is never applied (SpellAuras.cpp:696, addUnit=false; Unit.cpp:3648
		// removes it before the no-stack purge). Evaluated before the
		// same-spell replacement below so a suppressed pet cast keeps the
		// existing aura.
		highest := s.server.exclusiveHighestVerdict(spell, effect, amount, targetGUID, targetSess.activeAuras)
		if highest.suppressed {
			targetSess.castMu.Unlock()
			return false
		}
		if previous := targetSess.activeAuras[spell.ID]; previous != nil {
			previous.Stopped = true
			if previous.Timer != nil {
				previous.Timer.Stop()
			}
			if previous.TickTimer != nil {
				previous.TickTimer.Stop()
			}
			s.server.unregisterSingleCastAura(previous)
		}
		slot, found := targetSess.auraSlots[spell.ID]
		if !found {
			slot = uint8(len(targetSess.auraSlots))
			targetSess.auraSlots[spell.ID] = slot
		}
		aura := &activeAura{SpellID: spell.ID, DispelType: spell.DispelType, Mechanic: spell.Mechanic, AuraType: effect.Aura, EffectMask: effectMask, RecalculateMask: recalculateMask, CasterGUID: caster.GUID, TargetGUID: targetGUID, SchoolMask: spell.SchoolMask, MiscValue: effect.MiscValue, Amount: uint32(amount), Amounts: amounts, BaseAmounts: baseAmounts, DurationMs: durationMs, PeriodMs: periodMs, RemainingMs: durationMs, Slot: slot, Positive: positive, CasterLevel: uint8(casterLevel(caster)), AuraInterruptFlags: spell.AuraInterruptFlags, TriggerSpell: effect.TriggerSpell, StackAmount: spell.StackAmount, HideDuration: spell.AttributesEx5&spellAttr5HideDuration != 0, StackCount: 1, OwnerPetAura: source.SpellID != 0, OwnerPetAuraSourceSpell: source.SpellID, OwnerPetAuraSourceEffect: source.EffectIndex, OwnerPetAuraSourceDamage: sourceDamage, OwnerPetAuraRemoveOnChange: removeOnChange}
		targetSess.activeAuras[spell.ID] = aura
		targetSess.auras[spell.ID] = struct{}{}
		// Unit::_AddAura single-target dance (Unit.cpp:3397-3420).
		var scPurge []singleCastEntry
		if spellIsSingleTarget(spell) {
			scPurge = s.server.registerSingleCastAura(spell, aura)
		}
		// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640): pet casts run
		// the same no-stack purge as player casts — the rank-chain term
		// (Aura::CanStackWith, SpellAuras.cpp:1994-2004), the spell-group
		// exclusive terms (SpellAuras.cpp:1924-1932), the spell-specific
		// exclusivity gates (SpellAuras.cpp:1914-1921), the EXCLUSIVE_HIGHEST
		// comparisons (Unit.cpp:13991), and the _AddAura single-target
		// dance (Unit.cpp:3397-3420) — keyed by the casting creature's
		// GUID, not the owner's.
		purgeIDs := s.server.rankChainNoStackPurge(spell, caster.GUID, aura.ItemGUID, targetSess.activeAuras)
		purgeIDs = append(purgeIDs, s.server.spellGroupNoStackPurge(spell, caster.GUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, s.server.spellSpecificNoStackPurge(spell, caster.GUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, s.server.singleTargetNoStackPurge(spell, caster.GUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, highest.purge...)
		targetSess.castMu.Unlock()
		for _, purgeID := range purgeIDs {
			targetSess.expirePlayerAura(purgeID.spellID)
		}
		for _, e := range scPurge {
			s.expireSingleCastEntry(e)
		}
		wireMax, wireDuration := auraWireDurations(spell, durationMs, durationMs)
		packet := protocol.BuildAuraUpdateWithStackEffect(targetGUID, caster.GUID, slot, spell.ID, false, positive, wireMax, wireDuration, uint8(casterLevel(caster)), 1, aura.EffectMask)
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), packet, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), packet, targetSess)
		if periodMs > 0 {
			targetSess.schedulePlayerPeriodicTickInitial(aura, periodMs, periodicTickInitialDelay(spell, periodMs))
		}
		if durationMs > 0 && durationMs < 18000000 {
			aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() { targetSess.expirePlayerAura(spell.ID) })
		}
		return true
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return false
	}
	targetKey := creatureAuraKeyForTarget(target)
	s.server.auraMu.Lock()
	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[creatureAuraKey]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[targetKey] == nil {
		s.server.activeCreatureAuras[targetKey] = make(map[uint32]*activeAura)
	}
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
	}
	if s.server.creatureAuras[targetKey] == nil {
		s.server.creatureAuras[targetKey] = make(map[uint32]struct{})
	}
	// Unit::IsHighestExclusiveAura (Unit.cpp:13991): a fresh aura whose
	// effect is strictly lower than an existing EXCLUSIVE_HIGHEST peer
	// is never applied (SpellAuras.cpp:696, addUnit=false; Unit.cpp:3648
	// removes it before the no-stack purge).
	highest := s.server.exclusiveHighestVerdict(spell, effect, amount, targetGUID, s.server.activeCreatureAuras[targetKey])
	if highest.suppressed {
		s.server.auraMu.Unlock()
		return false
	}
	// Same-spell replacement (Unit::_TryStackingOrRefreshingExistingAura,
	// Unit.cpp:3326): the old aura instance is removed before the new one is
	// inserted — expireCreatureAura matches by spell ID, so a stale duration
	// timer would delete the replacement early, and a stale tick timer would
	// keep firing the orphaned struct. Mirrors the player-target branch above.
	if previous := s.server.activeCreatureAuras[targetKey][spell.ID]; previous != nil {
		previous.Stopped = true
		if previous.Timer != nil {
			previous.Timer.Stop()
		}
		if previous.TickTimer != nil {
			previous.TickTimer.Stop()
		}
		s.server.unregisterSingleCastAura(previous)
	}
	slot := uint8(len(s.server.activeCreatureAuras[targetKey]))
	aura := &activeAura{SpellID: spell.ID, DispelType: spell.DispelType, Mechanic: spell.Mechanic, AuraType: effect.Aura, EffectMask: effectMask, RecalculateMask: recalculateMask, CasterGUID: caster.GUID, TargetGUID: targetGUID, TargetKey: targetKey, SchoolMask: spell.SchoolMask, MiscValue: effect.MiscValue, Amount: uint32(amount), Amounts: amounts, BaseAmounts: baseAmounts, DurationMs: durationMs, PeriodMs: periodMs, RemainingMs: durationMs, Slot: slot, Positive: positive, CasterLevel: uint8(casterLevel(caster)), AuraInterruptFlags: spell.AuraInterruptFlags, TriggerSpell: effect.TriggerSpell, StackAmount: spell.StackAmount, HideDuration: spell.AttributesEx5&spellAttr5HideDuration != 0, StackCount: 1, OwnerPetAura: source.SpellID != 0, OwnerPetAuraSourceSpell: source.SpellID, OwnerPetAuraSourceEffect: source.EffectIndex, OwnerPetAuraSourceDamage: sourceDamage, OwnerPetAuraRemoveOnChange: removeOnChange}
	s.server.activeCreatureAuras[targetKey][spell.ID] = aura
	s.server.creatureAuras[targetKey][spell.ID] = struct{}{}
	// Unit::_AddAura single-target dance (Unit.cpp:3397-3420).
	var scPurge []singleCastEntry
	if spellIsSingleTarget(spell) {
		scPurge = s.server.registerSingleCastAura(spell, aura)
	}
	// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640): pet casts run
	// the same no-stack purge as player casts — the rank-chain term
	// (Aura::CanStackWith, SpellAuras.cpp:1994-2004), the spell-group
	// exclusive terms (SpellAuras.cpp:1924-1932), the spell-specific
	// exclusivity gates (SpellAuras.cpp:1914-1921), the EXCLUSIVE_HIGHEST
	// comparisons (Unit.cpp:13991), and the _AddAura single-target
	// dance (Unit.cpp:3397-3420) — keyed by the casting creature's
	// GUID, not the owner's.
	purge := s.server.rankChainNoStackPurge(spell, caster.GUID, aura.ItemGUID, s.server.activeCreatureAuras[targetKey])
	purge = append(purge, s.server.spellGroupNoStackPurge(spell, caster.GUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, s.server.spellSpecificNoStackPurge(spell, caster.GUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, s.server.singleTargetNoStackPurge(spell, caster.GUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, highest.purge...)
	s.server.auraMu.Unlock()
	for _, p := range purge {
		s.expireCreatureAura(targetKey, p.spellID, p.slot)
	}
	for _, e := range scPurge {
		s.expireSingleCastEntry(e)
	}
	wireMax, wireDuration := auraWireDurations(spell, durationMs, durationMs)
	packet := protocol.BuildAuraUpdateWithStackEffect(targetGUID, caster.GUID, slot, spell.ID, false, positive, wireMax, wireDuration, uint8(casterLevel(caster)), 1, aura.EffectMask)
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), packet, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), packet, s)
	if periodMs > 0 {
		s.scheduleCreaturePeriodicTickInitial(aura, periodMs, periodicTickInitialDelay(spell, periodMs))
	}
	if durationMs > 0 && durationMs < 18000000 {
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() { s.expireCreatureAura(targetKey, spell.ID, slot) })
	}
	return true
}

func (s *session) executePetSpellHeal(ctx context.Context, caster *creatureMotion, targetGUID uint64, spellID, heal uint32) {
	if s == nil || s.server == nil || caster == nil || heal == 0 {
		return
	}
	target, ok := s.getCombatTarget(ctx, uint64(targetGUID))
	if !ok || target.Health == 0 {
		return
	}
	// Unit::SpellHealingBonusTaken (Unit.cpp:7714-7759) runs on every
	// EffectHeal ahead of the crit roll (SpellEffects.cpp:1466):
	// MOD_HEALING_PCT, the Nourish 1.2x leg and MOD_HEALING_RECEIVED
	// (caster-matched against the pet's GUID) — previously the raw amount
	// healed. Direct heals are HEAL type, so the MOD_HOT_PCT leg is off
	// (dotType=false). Pets carry no Go spellpower model, so the
	// SpellHealingBonusDone leg is identity here.
	targetSess := s.server.findSessionByGUID(target.GUID)
	if targetSess != nil && targetSess.player != nil {
		heal = s.healingTakenBonus(targetSess, caster.GUID, spellID, heal, false)
	} else if s.server.Data != nil {
		if hs, found, err := s.server.Data.Spell(spellID); err == nil && found {
			heal = creatureHealingTakenBonus(s.server, creatureAuraKeyForTarget(target), caster.GUID, hs, true, heal, false)
		}
	}
	overheal := uint32(0)
	if uint64(target.Health)+uint64(heal) > uint64(target.MaxHealth) {
		overheal = uint32(uint64(target.Health) + uint64(heal) - uint64(target.MaxHealth))
	}
	packet := buildSpellHealLog(target.GUID, caster.GUID, spellID, heal, overheal, 0, false)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, s)
	if targetSess != nil && targetSess.player != nil {
		newHealth := targetSess.player.Health + heal
		if newHealth > targetSess.player.MaxHealth {
			newHealth = targetSess.player.MaxHealth
		}
		targetSess.player.Health = newHealth
		targetSess.sendPlayerUpdate()
		return
	}
	s.server.motionMu.Lock()
	targetMotion := s.server.findCreatureMotionLocked(target.Map, target.InstanceID, target.GUID)
	if targetMotion != nil {
		newHealth := targetMotion.Health + heal
		if newHealth > targetMotion.MaxHealth {
			newHealth = targetMotion.MaxHealth
		}
		targetMotion.Health = newHealth
	}
	s.server.motionMu.Unlock()
	if targetMotion != nil {
		s.server.broadcastCreatureValuesUpdateInInstance(targetMotion.Map, targetMotion.InstanceID, targetMotion.GUID, map[int]uint32{unitFieldHealth: targetMotion.Health})
	}
}

func (s *session) executePetSpellDamage(ctx context.Context, caster *creatureMotion, targetGUID uint64, spellID, damage uint32, schoolMask uint8, triggered bool) {
	if s == nil || s.server == nil || caster == nil || damage == 0 {
		return
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	// Ghost leg mirrors WorldObject::IsValidAttackTarget's "can't attack dead"
	// (Object.cpp:2935-2937): ghosts carry Health=1 in Go.
	if !ok || target.Health == 0 || s.ghostAttackTargetBlocked(targetGUID) {
		return
	}
	isPlayerVictim := s.server.findSessionByGUID(target.GUID) != nil
	hitInfo := uint32(0)
	resisted := uint32(0)
	absorbed := uint32(0)
	if target.GUID != caster.GUID {
		miss := magicSpellHitResult(uint8(maxUint32(caster.Level, 1)), target.Level, isPlayerVictim)
		if miss != protocol.SpellMissNone {
			damage = 0
			hitInfo = 0x01
		}
	}
	if damage > 0 && schoolMask > 1 {
		resisted, damage = calcMagicSpellResistance(damage, schoolMask, target.Resistances, uint8(maxUint32(caster.Level, 1)), target.Level, !isPlayerVictim, false)
	}
	if isPlayerVictim {
		if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
			victim.applyResilienceToDamage(true, &damage, false, CombatRatingCritTakenSpell)
			if damage > 0 {
				// Pet attackers carry no Go aura model, so the
				// MOD_TARGET_ABSORB_SCHOOL (194) absorb-ignore arm
				// (Unit.cpp:1839-1857) stays unbridged here.
				absorbed, damage = victim.applyAbsorptionShields(damage, schoolMask)
			}
		}
	}
	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	logPacket := buildSpellNonMeleeDamageLog(target.GUID, caster.GUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPacket, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPacket, s)
	if damage == 0 {
		// Unit::DealDamage absorbed legs (Unit.cpp:742-761, 813-819): a fully
		// absorbed pet-spell hit still strips TAKE_DAMAGE-interrupt auras,
		// grants absorbed rage, and aborts ABORT_ON_DMG casts on a player
		// victim — the god arm above returns before all of these in C++.
		if isPlayerVictim && absorbed > 0 && s.server != nil {
			if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil && !victim.godCheatActive() {
				victim.grantRageFromDamageTaken(ctx, absorbed)
				victim.procDamageAuras(false)
				victim.interruptAbsorbedCast()
			}
		}
		return
	}
	if isPlayerVictim {
		victim := s.server.findSessionByGUID(target.GUID)
		if victim == nil || victim.player == nil {
			return
		}
		// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
		// after the spell damage log (sent pre-DealDamage in C++); the taken
		// proc event below still carries the pre-negation amount — C++'s god
		// skip lives inside DealDamage, after the DamageInfo is built
		// (Spell.cpp:2539-2549).
		procEventDamage := damage
		damage = victim.negateGodModeDamage(damage)
		// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
		if damage > 0 {
			s.splitShareDamagePct(ctx, target.GUID, true, creatureAuraKey{}, caster.GUID, damage, uint32(schoolMask))
		}
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED fires
		// for any damage to a player victim, pet-caster spells included.
		victim.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
		// creatures are signaled OwnerAttackedBy on any non-DoT damage.
		s.server.triggerPetDefensive(victim.player.Map, victim.player.InstanceID, target.GUID, caster.GUID)
		// Duel defeat (Unit.cpp:825-853, 957-973): a pet is controlled by its
		// owner (s is the owner session here; GetControllingPlayer,
		// Unit.cpp:5996) — the blow lands as a duel defeat when the owner is
		// the victim's duel opponent; a lethal blow from anyone else's pet
		// kills and interrupts instead.
		if duelDefeatOnDamage(victim, s.playerGUID, damage, victim.player.Health) {
			// Duel defeat consumed the hit — loser at 1 HP, duel complete.
		} else if damage >= victim.player.Health {
			victim.player.Health = 0
			victim.sendPlayerUpdate()
			// Unit::Kill (Unit.cpp:11455-11458): attacker is a creature (the
			// pet) → KILLED_BY_CREATURE with the pet's entry.
			victim.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((caster.GUID>>24)&0xFFFFFF), 1)
			// The pet spell's killer resolves to the pet's owner player.
			victim.killPlayer(ctx, s, true, true)
			// Unit::Kill (Unit.cpp:11283-11290): the PROC_FLAG_KILL leg fires
			// for the killing-blow unit regardless of source — the owner-
			// player leg runs inside killPlayer (death.go); the pet creature
			// itself is a second killer and its own aura procs fire here,
			// before the KilledUnit AI dispatches below (C++ order: the proc
			// calls at 11285-11290 precede the AI hooks at 11359-11361).
			s.server.fireCreatureMeleeAuraProcs(ctx, caster, target.GUID, procEventInfo{
				typeMask:       procFlagKill,
				spellTypeMask:  procSpellTypeMaskAll,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        procHitNone,
				actorGUID:      caster.GUID,
			})
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): the attacker is the
			// pet — C++ Unit::Kill fires the pet arm (the owner's pet is the
			// attacker itself) AND the player-victim branch arm, i.e. two
			// dispatches with the pet (Unit.cpp pet arm + 11359-11361).
			s.server.fireCreatureTargetDied(ctx, caster, victim.luaPlayer())
			s.server.fireCreatureTargetDied(ctx, caster, victim.luaPlayer())
		} else {
			victim.player.Health -= damage
			// Unit::DealDamage (Unit.cpp:896-897): pet spells land as
			// SPELL_DIRECT_DAMAGE (DealSpellDamage, Unit.cpp:1157), so a
			// non-lethal hit strips DIRECT_DAMAGE-interrupt auras alongside
			// the TAKE_DAMAGE strip — the absorbed arm above (742-747) only
			// ran the TAKE_DAMAGE half.
			victim.procDamageAuras(true, damage)
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
			victim.grantRageFromDamageTaken(ctx, damage)
			// Unit::DealDamage (Unit.cpp:906-913): random durability loss on
			// HIT TAKEN — the victim is a player.
			victim.rollDurabilityLossOnHit(ctx, damage)
			// Unit::DealDamage (Unit.cpp:934-952): pushback — the pet spell's
			// attributes gate the delay like any direct-damage spell.
			if damage > 0 && s.spellDamagePushesBack(spellID, victim.playerGUID) {
				victim.delayCurrentCast()
				victim.delayCurrentChannel()
			}
			victim.sendPlayerUpdate()
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
		// folds into the non-zero-effect model — the pet path applies
		// every non-zero effect to every hit target. Runs after the
		// damage legs on both the lethal and non-lethal arms — C++ fires
		// the call after DealSpellDamage regardless of the victim's death.
		// Hit detail: misses (hitInfo 0x01) return before this branch, so
		// the mask is always NORMAL, optionally with the partial-absorb
		// bit; pet spell hits carry no crit model (standing delta).
		if victim.procDeep == 0 {
			victim.procSpellHitTakenAuraTriggers(ctx, caster.GUID, spellID, true, false, false, false, false, absorbed, procEventDamage, triggered, false)
		}
		return
	}
	// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
	if damage > 0 {
		s.splitShareDamagePct(ctx, target.GUID, false, creatureAuraKey{Map: target.Map, InstanceID: target.InstanceID, GUID: target.GUID}, caster.GUID, damage, uint32(schoolMask))
	}
	s.server.motionMu.Lock()
	targetMotion := s.server.findCreatureMotionLocked(target.Map, target.InstanceID, target.GUID)
	newlyTapped := false
	rageChanged := false
	var rageNext uint32
	if targetMotion != nil {
		// Unit::DealDamage tap block (Unit.cpp:872-876): pet spell
		// damage taps for the owner (GetCharmerOrOwnerPlayerOrPlayerItself).
		tapPlayer := s.playerGUID
		if caster != nil && caster.OwnerGUID != 0 {
			tapPlayer = caster.OwnerGUID
		}
		newlyTapped = s.server.recordCreatureTap(targetMotion, tapPlayer, s.groupID, damage, targetMotion.Health)
		if damage >= targetMotion.Health {
			targetMotion.Health = 0
			targetMotion.DynamicFlags |= unitDynFlagLootable
			targetMotion.InCombat = false
			targetMotion.TargetGUID = 0
			targetMotion.Moving = false
			if targetMotion.ThreatMgr != nil {
				targetMotion.ThreatMgr.ClearThreat()
			}
		} else {
			targetMotion.Health -= damage
			if targetMotion.ThreatMgr == nil {
				targetMotion.ThreatMgr = NewThreatManager(targetMotion)
			}
			targetMotion.ThreatMgr.AddThreat(caster.OwnerGUID, float32(damage), false)
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
			if next, changed := s.server.addCreatureRageLocked(targetMotion, targetMotion.Level, damage); changed {
				rageNext, rageChanged = next, true
			}
		}
	}
	s.server.motionMu.Unlock()
	if targetMotion == nil {
		return
	}
	if targetMotion.Health == 0 {
		s.server.stopCreatureMotionInInstance(targetMotion.Map, targetMotion.InstanceID, targetMotion.GUID, targetMotion.X, targetMotion.Y, targetMotion.Z)
		// The corpse keeps TAPPED when the tap survived to death
		// (Unit.cpp:11172-11175), so broadcast the real flags.
		s.server.broadcastCreatureValuesUpdateInInstance(targetMotion.Map, targetMotion.InstanceID, targetMotion.GUID, map[int]uint32{unitFieldHealth: 0, unitFieldDynamicFlags: targetMotion.DynamicFlags})
		s.onCreatureKilled(ctx, target, caster)
	} else {
		healthUpdate := map[int]uint32{unitFieldHealth: targetMotion.Health}
		if newlyTapped {
			healthUpdate[unitFieldDynamicFlags] = targetMotion.DynamicFlags
		}
		if rageChanged {
			healthUpdate[unitFieldPower1+powerRage] = rageNext
		}
		s.server.broadcastCreatureValuesUpdateInInstance(targetMotion.Map, targetMotion.InstanceID, targetMotion.GUID, healthUpdate)
		s.server.triggerCreatureAggro(ctx, targetMotion.GUID, caster.OwnerGUID)
	}
}
