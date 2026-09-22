package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	protocol "github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

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
func (s *Server) onPetCommandAttack(petGUID uint64, targetGUID uint64) {
	if s == nil || petGUID == 0 || targetGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return
	}
	if motion, ok := s.creatureMotion[petGUID]; ok && motion != nil && motion.Health > 0 {
		motion.TargetGUID = targetGUID
		motion.InCombat = true
		motion.PetCommand = PetCommandAttack
		motion.Moving = true
	}
}

// onPetCommandFollow handles recalling the pet back to follow the owner.
// Mirrors TrinityCore PetAI::DoRecall (PetAI.cpp:215).
func (s *Server) onPetCommandFollow(petGUID uint64) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return
	}
	if motion, ok := s.creatureMotion[petGUID]; ok && motion != nil {
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
func (s *Server) onPetCommandStay(petGUID uint64) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return
	}
	if motion, ok := s.creatureMotion[petGUID]; ok && motion != nil {
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
func (s *Server) onPetSetReaction(petGUID uint64, reactState uint8) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return
	}
	if motion, ok := s.creatureMotion[petGUID]; ok && motion != nil {
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
func (s *Server) onPetToggleAutocast(petGUID uint64, spellID uint32, enable bool) {
	if s == nil || petGUID == 0 || spellID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return
	}
	motion, ok := s.creatureMotion[petGUID]
	if !ok || motion == nil {
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

// triggerPetDefensive triggers the owner's active pet into defensive attack mode if appropriate.
// Mirrors TrinityCore PetAI::OwnerAttackedBy / PetAI::OwnerAttacked (PetAI.cpp:240-310).
func (s *Server) triggerPetDefensive(ownerGUID uint64, targetGUID uint64) {
	if s == nil || ownerGUID == 0 || targetGUID == 0 || ownerGUID == targetGUID {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return
	}
	for _, m := range s.creatureMotion {
		if m.OwnerGUID == ownerGUID && m.Health > 0 {
			// Pet must be in Defensive mode, currently following, and not already attacking a target
			if m.PetReact == PetReactDefensive && m.PetCommand == PetCommandFollow && (m.TargetGUID == 0 || !m.InCombat) {
				m.TargetGUID = targetGUID
				m.InCombat = true
				m.Moving = true
			}
			break
		}
	}
}

// updatePetMotion is the per-tick AI driver for active player pets.
func (s *Server) updatePetMotion(ctx context.Context, motion *creatureMotion, players []playerPos, now time.Time) {
	if s == nil || motion == nil || motion.OwnerGUID == 0 {
		return
	}

	var owner *playerPos
	for i := range players {
		if players[i].GUID == motion.OwnerGUID {
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
		if dist > 45.0 {
			// Teleport directly to owner if too far
			motion.X = owner.X + 1.5
			motion.Y = owner.Y + 1.5
			motion.Z = owner.Z
			if owner.Sess != nil && owner.Sess.player != nil {
				motion.Orientation = owner.Sess.player.Orientation
			}
			motion.Moving = false
			s.broadcastMonsterMoveStop(motion.Map, motion.GUID, motion.X, motion.Y, motion.Z)
		} else if dist > 3.0 {
			// Run to catch up with owner
			dx := owner.X - motion.X
			dy := owner.Y - motion.Y
			angle := float32(math.Atan2(float64(dy), float64(dx)))
			motion.Orientation = angle
			destX := owner.X - 1.5*float32(math.Cos(float64(angle)))
			destY := owner.Y - 1.5*float32(math.Sin(float64(angle)))
			destZ := owner.Z
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
			s.broadcastMonsterMove(motion.Map, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration)
		} else {
			motion.Moving = false
		}
	}
}

// findNearbyPetHostile scans for nearby hostile creatures within range.
func (s *Server) findNearbyPetHostile(pet *creatureMotion, maxDist float32, players []playerPos) uint64 {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for guid, m := range s.creatureMotion {
		if guid == pet.GUID || m.OwnerGUID != 0 || m.Health == 0 || m.Map != pet.Map {
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
		cMotion := s.creatureMotion[targetGUID]
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
	if !targetFound || targetHealth == 0 {
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.Moving = false
		if motion.PetCommand == PetCommandAttack {
			motion.PetCommand = PetCommandFollow
		}
		stopPkt := buildAttackStop(motion.GUID, targetGUID, false)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
		return
	}

	dist := float32(math.Hypot(float64(targetX-motion.X), float64(targetY-motion.Y)))
	meleeRange := float32(3.5)

	if dist > meleeRange && motion.PetCommand != PetCommandStay {
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
		s.broadcastMonsterMove(motion.Map, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration)
	} else {
		motion.Moving = false

		// Melee attack
		attackTime := time.Duration(motion.AttackTime) * time.Millisecond
		if attackTime <= 0 {
			attackTime = 2 * time.Second
		}
		if motion.LastAttack.IsZero() || now.Sub(motion.LastAttack) >= attackTime {
			s.executePetMeleeAttack(ctx, motion, targetGUID, isTargetPlayer, targetSess, targetHealth, targetArmor, targetLevel, now)
			motion.LastAttack = now
		}

		// Autocast spells
		if len(motion.AutocastSpells) > 0 && (motion.LastSpell.IsZero() || now.Sub(motion.LastSpell) >= 3*time.Second) {
			s.executePetAutocast(ctx, motion, targetGUID, now)
		}
	}
}

// executePetMeleeAttack conducts the pet's physical swing on the target.
func (s *Server) executePetMeleeAttack(ctx context.Context, motion *creatureMotion, targetGUID uint64, isTargetPlayer bool, targetSess *session, targetHealth, targetArmor uint32, targetLevel uint8, now time.Time) {
	damage := uint32(float64(motion.MinDamage) + rand.Float64()*float64(motion.MaxDamage-motion.MinDamage))
	if damage < 1 {
		damage = 1
	}

	if targetArmor > 0 {
		damage = calcArmorReducedDamage(float64(targetArmor), uint8(motion.Level), damage)
	}

	outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, false, isTargetPlayer, false, false, false, true)
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
		if damage >= targetHealth {
			targetSess.player.Health = 0
			targetSess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
			targetSess.killPlayer(ctx)
		} else {
			targetSess.player.Health -= damage
			targetSess.sendPlayerUpdate()
		}
	} else {
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, nil)
		s.motionMu.Lock()
		cMotion := s.creatureMotion[targetGUID]
		if cMotion != nil {
			if damage >= cMotion.Health {
				cMotion.Health = 0
				cMotion.InCombat = false
				cMotion.Moving = false
				s.broadcastCreatureValuesUpdate(cMotion.Map, targetGUID, map[int]uint32{unitFieldHealth: 0, unitFieldDynamicFlags: 1})
			} else {
				cMotion.Health -= damage
				s.broadcastCreatureValuesUpdate(cMotion.Map, targetGUID, map[int]uint32{unitFieldHealth: cMotion.Health})
			}
		}
		s.motionMu.Unlock()
	}
}

// executePetAutocast casts the highest priority available pet autocast spell.
func (s *Server) executePetAutocast(ctx context.Context, motion *creatureMotion, targetGUID uint64, now time.Time) {
	if len(motion.AutocastSpells) == 0 {
		return
	}
	spellID := motion.AutocastSpells[0]
	castTimeStamp := uint32(now.UnixMilli())
	spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
	goPkt := protocol.BuildSpellGo(motion.GUID, motion.GUID, 1, spellID, spellCastFlagGo, castTimeStamp, []uint64{targetGUID}, nil, spellTarget)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, nil)

	// Direct spell damage for pet abilities (e.g. Firebolt, Claw, Bite, Torment)
	baseDmg := uint32(motion.Level*2 + 10)
	s.motionMu.Lock()
	if cMotion := s.creatureMotion[targetGUID]; cMotion != nil && cMotion.Health > 0 {
		if baseDmg >= cMotion.Health {
			cMotion.Health = 0
			cMotion.InCombat = false
			s.broadcastCreatureValuesUpdate(cMotion.Map, targetGUID, map[int]uint32{unitFieldHealth: 0, unitFieldDynamicFlags: 1})
		} else {
			cMotion.Health -= baseDmg
			s.broadcastCreatureValuesUpdate(cMotion.Map, targetGUID, map[int]uint32{unitFieldHealth: cMotion.Health})
		}
	}
	s.motionMu.Unlock()
	motion.LastSpell = now
}

func (s *session) executePetSpell(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, castCount uint8, target protocol.SpellTargetData) bool {
	if s == nil || s.server == nil || motion == nil {
		return false
	}
	targetGUID := target.UnitGUID
	if targetGUID == 0 {
		targetGUID = motion.GUID
	}
	hitTargets := []uint64{targetGUID}
	stamp := uint32(time.Now().UnixMilli())
	goPacket := protocol.BuildSpellGo(motion.GUID, motion.GUID, castCount, spell.ID, spellCastFlagGo, stamp, hitTargets, nil, target)
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPacket, true); err != nil {
		return false
	}
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPacket, s)
	damage, hasDamage := creatureSpellDamage(spell)
	handledEffect := false
	if hasDamage {
		schoolMask := uint8(spell.SchoolMask)
		if schoolMask == 0 {
			schoolMask = 1
		}
		s.executePetSpellDamage(ctx, motion, targetGUID, spell.ID, damage, schoolMask)
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
				s.executePetSpell(ctx, motion, triggered, castCount, target)
				handledEffect = true
			}
		}
	}
	if !handledEffect {
		now := time.Now()
		s.server.motionMu.Lock()
		if motion.SpellCooldowns == nil {
			motion.SpellCooldowns = make(map[uint32]time.Time)
		}
		motion.SpellCooldowns[spell.ID] = now
		s.server.motionMu.Unlock()
		motion.LastSpell = now
		return true
	}
	now := time.Now()
	s.server.motionMu.Lock()
	if motion.SpellCooldowns == nil {
		motion.SpellCooldowns = make(map[uint32]time.Time)
	}
	motion.SpellCooldowns[spell.ID] = now
	s.server.motionMu.Unlock()
	motion.LastSpell = now
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
	overheal := uint32(0)
	if uint64(target.Health)+uint64(heal) > uint64(target.MaxHealth) {
		overheal = uint32(uint64(target.Health) + uint64(heal) - uint64(target.MaxHealth))
	}
	packet := buildSpellHealLog(target.GUID, caster.GUID, spellID, heal, overheal, 0, false)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, s)
	if targetSess := s.server.findSessionByGUID(target.GUID); targetSess != nil && targetSess.player != nil {
		newHealth := targetSess.player.Health + heal
		if newHealth > targetSess.player.MaxHealth {
			newHealth = targetSess.player.MaxHealth
		}
		targetSess.player.Health = newHealth
		targetSess.sendPlayerUpdate()
		return
	}
	low := uint32(target.GUID & 0x00FFFFFF)
	entry := uint32((target.GUID >> 24) & 0x00FFFFFF)
	key := creatureWorldGUID(low, entry)
	s.server.motionMu.Lock()
	targetMotion := s.server.creatureMotion[target.GUID]
	if targetMotion == nil {
		targetMotion = s.server.creatureMotion[key]
	}
	if targetMotion != nil {
		newHealth := targetMotion.Health + heal
		if newHealth > targetMotion.MaxHealth {
			newHealth = targetMotion.MaxHealth
		}
		targetMotion.Health = newHealth
	}
	s.server.motionMu.Unlock()
	if targetMotion != nil {
		s.server.broadcastCreatureValuesUpdate(targetMotion.Map, targetMotion.GUID, map[int]uint32{unitFieldHealth: targetMotion.Health})
	}
}

func (s *session) executePetSpellDamage(ctx context.Context, caster *creatureMotion, targetGUID uint64, spellID, damage uint32, schoolMask uint8) {
	if s == nil || s.server == nil || caster == nil || damage == 0 {
		return
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
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
		resistance := target.Resistances[schoolMaskToResistanceIndex(schoolMask)]
		resisted, damage = calcMagicSpellResistance(damage, schoolMask, resistance, uint8(maxUint32(caster.Level, 1)), target.Level)
	}
	if isPlayerVictim {
		if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
			victim.applyResilienceToDamage(true, &damage, false, CombatRatingCritTakenSpell)
			if damage > 0 {
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
		return
	}
	if isPlayerVictim {
		victim := s.server.findSessionByGUID(target.GUID)
		if victim == nil || victim.player == nil {
			return
		}
		if damage >= victim.player.Health {
			victim.player.Health = 0
			victim.sendPlayerUpdate()
			victim.killPlayer(ctx)
		} else {
			victim.player.Health -= damage
			victim.sendPlayerUpdate()
		}
		return
	}
	low := uint32(target.GUID & 0x00FFFFFF)
	entry := uint32((target.GUID >> 24) & 0x00FFFFFF)
	key := creatureWorldGUID(low, entry)
	s.server.motionMu.Lock()
	targetMotion := s.server.creatureMotion[target.GUID]
	if targetMotion == nil {
		targetMotion = s.server.creatureMotion[key]
	}
	if targetMotion != nil {
		if damage >= targetMotion.Health {
			targetMotion.Health = 0
			targetMotion.InCombat = false
			targetMotion.TargetGUID = 0
			targetMotion.Moving = false
			if targetMotion.ThreatMgr != nil {
				targetMotion.ThreatMgr.ClearThreat()
			}
		} else {
			targetMotion.Health -= damage
			if targetMotion.ThreatMgr == nil {
				targetMotion.ThreatMgr = NewThreatManager(targetMotion.GUID)
			}
			targetMotion.ThreatMgr.AddThreat(caster.OwnerGUID, float32(damage), false)
		}
	}
	s.server.motionMu.Unlock()
	if targetMotion == nil {
		return
	}
	if targetMotion.Health == 0 {
		s.server.stopCreatureMotion(targetMotion.Map, targetMotion.GUID, targetMotion.X, targetMotion.Y, targetMotion.Z)
		s.server.broadcastCreatureValuesUpdate(targetMotion.Map, targetMotion.GUID, map[int]uint32{unitFieldHealth: 0, unitFieldDynamicFlags: 1})
		s.onCreatureKilled(ctx, target)
	} else {
		s.server.broadcastCreatureValuesUpdate(targetMotion.Map, targetMotion.GUID, map[int]uint32{unitFieldHealth: targetMotion.Health})
		s.server.triggerCreatureAggro(ctx, targetMotion.GUID, caster.OwnerGUID)
	}
}
