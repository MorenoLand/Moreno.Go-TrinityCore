package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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
func (s *Server) onPetCommandAttack(ctx context.Context, mapID, instanceID uint32, petGUID uint64, targetGUID uint64) {
	if s == nil || petGUID == 0 || targetGUID == 0 {
		return
	}
	// Eluna CREATURE_EVENT_ON_PRE_COMBAT (10): ElunaCreatureAI::AttackStart
	// (ElunaCreatureAI.h:143-148) fires (event, creature, target) before the
	// attack starts; a boolean true vetoes ScriptedAI::AttackStart — the
	// engage below. C++ fires it whenever a script drives AI()->AttackStart
	// on an Eluna-driven creature (ScriptedCreature.cpp:650,
	// SmartScript.cpp, CreatureMethods.h:1246, FleeingMovementGenerator.
	// cpp:266); in Go the only script-driven attack-start is the pet attack
	// command (the PetAI::AttackStart twin), so the hook fires here, ahead of
	// the GM gate the way UnitAI::AttackStart's me->Attack precedes the
	// Unit::Attack GM leg. Fires unlocked: handlers re-enter the motion
	// registry.
	motion := s.findCreatureMotion(mapID, instanceID, petGUID)
	if motion == nil {
		return
	}
	if s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnPreCombat, s.luaUnitObjectForGUID(mapID, instanceID, targetGUID)) {
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
	// Pet::ToggleAutocast (Pet.cpp): the spell must be autocastable
	// (SpellInfo::IsAutocastable, SpellInfo.cpp:1101-1108 — not passive and
	// without SPELL_ATTR1_UNAUTOCASTABLE_BY_PET) and known by the pet;
	// otherwise the toggle is ignored.
	if s.Data != nil {
		if spell, found, err := s.Data.Spell(spellID); err != nil || !found ||
			spell.Attributes&spellAttributePassive != 0 ||
			spell.AttributesEx&spellAttr1UnautocastableByPet != 0 {
			return
		}
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motion := s.motionMapLocked(mapID, instanceID)[petGUID]
	if motion == nil {
		return
	}
	known := false
	for _, sp := range motion.Spells {
		if sp == spellID {
			known = true
			break
		}
	}
	if !known {
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

// luaUnitObjectForGUID resolves a GUID to the Lua object Eluna pushes for a
// hook argument: an online player session becomes the player object, a live
// creature motion becomes the creature object, anything else is nil (Eluna
// pushes nullptr). It mirrors the target resolution in
// fireCreatureSpellHitTarget (boss_ai.go).
func (s *Server) luaUnitObjectForGUID(mapID, instanceID uint32, guid uint64) any {
	if s == nil || guid == 0 {
		return nil
	}
	if ts := s.findSessionByGUID(guid); ts != nil && ts.player != nil {
		return ts.luaPlayer()
	}
	if tm := s.findCreatureMotion(mapID, instanceID, guid); tm != nil {
		return s.luaMotionCreature(tm)
	}
	return nil
}

// triggerPetDefensive triggers the owner's controlled creatures into attack
// mode when the owner either attacks (assist) or takes damage (defend).
// Mirrors TrinityCore PetAI::OwnerAttackedBy / PetAI::OwnerAttacked
// (PetAI.cpp:240-310) and the base CreatureAI::OnOwnerCombatInteraction
// (CreatureAI.cpp:126-133): both fire for every non-passive controlled
// creature with no live victim, regardless of react state or command —
// defensive-only/follow-only gating is not in the C++.
// (PetAI::_AttackStart: stay-commanded pets attack without chasing.)
//
// hookEvent is the Eluna creature hook fired for each pet that engages,
// with the other party as the extra argument, or 0 for no hook:
// CREATURE_EVENT_ON_OWNER_ATTACKED (12, Eluna::OwnerAttacked,
// CreatureHooks.cpp:323-329, fired from Unit::Attack and Spell::_cast) when
// the owner is the attacker, CREATURE_EVENT_ON_OWNER_ATTACKED_AT (13,
// Eluna::OwnerAttackedBy, CreatureHooks.cpp:314-321, fired from
// Unit::DealDamage, Unit.cpp:728-733) when the owner is the victim. A true
// return vetoes that pet's engage, mirroring the C++
// `if (!sEluna->OwnerAttacked(...)) ScriptedAI::OwnerAttacked(...)` gate
// over the default engage. The hook fires outside the motion lock (handlers
// re-enter it, see the enter-combat hook); the engage re-validates under the
// lock, so a pet that died, despawned, or gained a victim in between is
// skipped. Only engaging pets fire the hook: C++ fires it ahead of the
// default's guards, but Go's controlled creatures are PetAI-modeled (the
// hook has no PetAI call chain), so the hook gates the PetAI engage.
func (s *Server) triggerPetDefensive(ctx context.Context, mapID, instanceID uint32, ownerGUID, targetGUID uint64, hookEvent int) {
	if s == nil || ownerGUID == 0 || targetGUID == 0 || ownerGUID == targetGUID {
		return
	}
	var cands []uint64
	s.motionMu.Lock()
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
		cands = append(cands, m.GUID)
	}
	s.motionMu.Unlock()
	vetoed := make(map[uint64]bool, len(cands))
	if hookEvent != 0 {
		for _, guid := range cands {
			m := s.findCreatureMotion(mapID, instanceID, guid)
			if m == nil {
				vetoed[guid] = true
				continue
			}
			if s.fireCreatureLuaEvent(ctx, m, hookEvent, s.luaUnitObjectForGUID(mapID, instanceID, targetGUID)) {
				vetoed[guid] = true
			}
		}
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, guid := range cands {
		if vetoed[guid] {
			continue
		}
		m := s.motionMapLocked(mapID, instanceID)[guid]
		if m == nil || m.OwnerGUID != ownerGUID || m.Health == 0 {
			continue
		}
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
//
// Eluna CREATURE_EVENT_ON_OWNER_ATTACKED (12, Eluna::OwnerAttacked,
// CreatureHooks.cpp:323-329) fires for each engaging pet ahead of the
// engage, with the spell target as the extra argument: the virtual
// OwnerAttacked call in Spell::_cast routes through ElunaCreatureAI for
// binding-carrying pets. A true return vetoes that pet's engage, mirroring
// the C++ `if (!sEluna->OwnerAttacked(...)) ScriptedAI::OwnerAttacked(...)`
// gate. The hook fires outside the motion lock (handlers re-enter it); the
// engage re-validates under the lock.
func (s *Server) triggerPetOwnerAttacked(ctx context.Context, mapID, instanceID uint32, ownerGUID, targetGUID uint64) {
	if s == nil || ownerGUID == 0 || targetGUID == 0 || ownerGUID == targetGUID {
		return
	}
	var cands []uint64
	s.motionMu.Lock()
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
		cands = append(cands, m.GUID)
	}
	s.motionMu.Unlock()
	vetoed := make(map[uint64]bool, len(cands))
	for _, guid := range cands {
		m := s.findCreatureMotion(mapID, instanceID, guid)
		if m == nil {
			vetoed[guid] = true
			continue
		}
		if s.fireCreatureLuaEvent(ctx, m, scripting.CreatureEventOnOwnerAttacked, s.luaUnitObjectForGUID(mapID, instanceID, targetGUID)) {
			vetoed[guid] = true
		}
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, guid := range cands {
		if vetoed[guid] {
			continue
		}
		m := s.motionMapLocked(mapID, instanceID)[guid]
		if m == nil || m.OwnerGUID != ownerGUID || m.Health == 0 {
			continue
		}
		if m.PetReact == PetReactPassive {
			continue
		}
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

	// PetAI::UpdateAI breakable-CC arm (PetAI.cpp:74-80): when the victim
	// carries a breakable-by-damage crowd-control aura the pet must not
	// break it — the tick's melee swing and autocast are suppressed, but
	// the pet stays in combat and its chase motion (independent of
	// UpdateAI) continues, so pursuit movement below is unaffected.
	ccHoldAttacks := s.petVictimHasBreakableCC(motion, targetGUID, isTargetPlayer, targetSess)

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

		// Melee attack (suppressed by the breakable-CC arm above).
		attackTime := time.Duration(motion.AttackTime) * time.Millisecond
		if attackTime <= 0 {
			attackTime = 2 * time.Second
		}
		if !ccHoldAttacks {
			if motion.LastAttack.IsZero() || now.Sub(motion.LastAttack) >= attackTime {
				s.executePetMeleeAttack(ctx, motion, owner.Sess, targetGUID, isTargetPlayer, targetSess, targetHealth, targetArmor, targetLevel, now)
				motion.LastAttack = now
			}

			// Autocast spells (suppressed by the breakable-CC arm above).
			if len(motion.AutocastSpells) > 0 && (motion.LastSpell.IsZero() || now.Sub(motion.LastSpell) >= 3*time.Second) {
				s.executePetAutocast(ctx, motion, targetGUID, now)
			}
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
		armor := float64(targetArmor)
		if !isTargetPlayer {
			if vm := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); vm != nil {
				key := creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}
				basePct, totalPct := s.petArmorPctBonus(key, vm.OwnerGUID != 0)
				armor = (armor*basePct + s.petBaseArmorBonus(key, vm.OwnerGUID != 0)) * totalPct
			}
		}
		if armor < 0 {
			armor = 0
		}
		if armor > 0 {
			// The pet's 123/269 ignore-resist arms run on the auto-swing
			// path (Unit.cpp:1632-1650, null spellInfo).
			armor = s.creatureIgnoreTargetResistArmor(creatureAuraKeyForMotion(motion), armor)
			damage = calcArmorReducedDamage(armor, uint8(motion.Level), targetLevel, damage)
		}
	}

	victimDodgeBP := int32(-1)
	// Unit::GetUnitParryChance / Unit::GetUnitBlockChance read the victim's
	// PLAYER_PARRY_PERCENTAGE / PLAYER_BLOCK_PERCENTAGE (Unit.cpp:2712/2788);
	// -1 keeps the legacy 5%-base fallback.
	victimParryBP := int32(-1)
	victimBlockBP := int32(-1)
	if isTargetPlayer && targetSess != nil && targetSess.player != nil {
		victimDodgeBP = int32(math.Round(float64(targetSess.player.DodgePercentage) * 100))
		victimParryBP = int32(math.Round(float64(targetSess.player.ParryPercentage) * 100))
		victimBlockBP = int32(math.Round(float64(targetSess.player.BlockPercentage) * 100))
	}
	// Unit::RollMeleeOutcomeAgainst (Unit.cpp:2213-2232) +
	// Unit::GetUnitParryChance / Unit::GetUnitBlockChance victim arms
	// (Unit.cpp:2708/2777). Player victims parry (a weapon — modeled as level
	// >= 10, matching the wild-creature path) and block (CanBlock + Block >
	// 0); their own SPELL_AURA_MOD_PARRY_PERCENT (47) /
	// SPELL_AURA_MOD_BLOCK_PERCENT (51) totals add to the 5% base. Nobody can
	// parry or block a pet attacking from behind, and players can't dodge
	// from behind either (Unit.cpp:2213-2217) — SPELL_AURA_IGNORE_HIT_DIRECTION
	// (288) exempts the victim. A victim mid cast-bar cast cannot avoid
	// (Unit.cpp:2219-2224; the UNIT_STATE_CONTROLLED half has no Go
	// unit-state model, stays unbridged).
	canParry := false
	canBlock := false
	canDodge := true
	var victimParryBonusBP, victimBlockBonusBP, victimDodgeBonusBP int32
	if isTargetPlayer && targetSess != nil && targetSess.player != nil {
		canParry = targetSess.player.Level >= 10 || targetSess.player.Level == 0
		canBlock = targetSess.player.CanBlock && targetSess.player.Block > 0
		victimParryBonusBP = targetSess.totalAuraModifier(spellAuraModParryPercent) * 100
		victimBlockBonusBP = targetSess.totalAuraModifier(spellAuraModBlockPercent) * 100
		if attackerInFront := hasInArc(targetSess.player.Orientation, targetSess.player.X, targetSess.player.Y, motion.X, motion.Y, math.Pi); !attackerInFront && !targetSess.hasAuraType(spellAuraIgnoreHitDirection) {
			canParry = false
			canBlock = false
			canDodge = false
		}
		if targetSess.genericCastInProgress() {
			canParry = false
			canBlock = false
			canDodge = false
		}
	}
	// Creature victims parry (5% base + 47) and block (5% base + 51) when the
	// attacker is in the front arc — 288 exempts — unless the template
	// carries CREATURE_FLAG_EXTRA_NO_PARRY (0x4) / NO_BLOCK (0x10). Totems
	// never avoid.
	if !isTargetPlayer {
		if vm := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); vm != nil && !s.isTotemGUID(targetGUID) {
			victimKey := creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}
			// Unit::GetUnitDodgeChance (Unit.cpp:2676-2681): the victim's
			// SPELL_AURA_MOD_DODGE_PERCENT (49) total adds to the 5% base
			// (dodge is not arc-gated for creatures).
			victimDodgeBonusBP = s.creatureAuraTypeSum(victimKey, spellAuraModDodgePercent) * 100
			inFront := hasInArc(vm.Orientation, vm.X, vm.Y, motion.X, motion.Y, math.Pi) ||
				s.creatureAuraTypeSum(victimKey, spellAuraIgnoreHitDirection) != 0
			if inFront && vm.FlagsExtra&0x00000004 == 0 {
				canParry = true
				victimParryBonusBP = s.creatureAuraTypeSum(victimKey, spellAuraModParryPercent) * 100
			}
			if inFront && vm.FlagsExtra&0x00000010 == 0 {
				canBlock = true
				victimBlockBonusBP = s.creatureAuraTypeSum(victimKey, spellAuraModBlockPercent) * 100
			}
		}
	}
	// Unit::GetUnitDodgeChance (Unit.cpp:2676-2681): the whole creature arm
	// is gated on !victim->IsTotem() — totems never dodge.
	if !isTargetPlayer && s.isTotemGUID(targetGUID) {
		canDodge = false
	}
	// The glancing victim gate excludes pets (Unit.cpp:2276-2279); pets
	// glance like players on the attacker side (Unit.cpp:2276).
	isPetVictim := false
	if !isTargetPlayer {
		if vm := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); vm != nil {
			isPetVictim = vm.OwnerGUID != 0
		}
	}
	// Unit::RollMeleeOutcomeAgainst's sitting-victim arm (Unit.cpp:2246-2247):
	// a non-standing player victim always takes a crit when the attacker
	// has nonzero crit chance.
	victimSitting := isTargetPlayer && targetSess != nil && targetSess.player != nil && targetSess.player.StandState != 0
	outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, true, isTargetPlayer, isPetVictim, false, victimSitting, false, canBlock, canParry, canDodge, 0, 0, 0, 0, victimDodgeBP, 0, 0, victimParryBonusBP, victimBlockBonusBP, victimParryBP, victimBlockBP, victimDodgeBonusBP)
	if !isTargetPlayer && s.isCreatureEvadingInInstance(motion.Map, motion.InstanceID, targetGUID) {
		// Unit::RollMeleeOutcomeAgainst (Unit.cpp:2191-2192): a creature
		// victim in evade mode evades all melee swings.
		outcome = protocol.MeleeHitEvade
		hitInfo = protocol.HitInfoMiss
		targetState = protocol.VictimStateEvades
	}
	// Pet attackers carry no Go aura model, so the 248 dodge-reduction arm
	// (Unit.cpp:2694-2695) and the 240/4 dodge/parry-reduction arm
	// (Unit::GetUnitDodgeChance's and GetUnitParryChance's creature arms,
	// Unit.cpp:2700-2703/2756-2759) stay unbridged on this path.
	// The roll's player/pet-attacker flag is true here: pets glance like
	// players (Unit.cpp:2276) and, being player-controlled, never crush
	// (Unit.cpp:2311).
	blocked := uint32(0)
	switch outcome {
	case protocol.MeleeHitMiss, protocol.MeleeHitDodge, protocol.MeleeHitParry, protocol.MeleeHitEvade, protocol.MeleeHitImmune:
		damage = 0
	case protocol.MeleeHitBlock:
		// Unit::CalculateMeleeDamage MELEE_HIT_BLOCK arm (Unit.cpp:1340-1376).
		var blockValue uint32
		var critChance int32
		if isTargetPlayer && targetSess != nil {
			blockValue, critChance = targetSess.shieldBlockParams()
		} else {
			blockValue = uint32(targetLevel) / 2
			critChance = s.creatureAuraTypeSum(creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}, spellAuraModBlockCritChance)
		}
		var fullBlock bool
		blocked, fullBlock = applyShieldBlockValue(blockValue, critChance, damage)
		damage -= blocked
		if fullBlock {
			targetState = protocol.VictimStateBlocks
		}
	case protocol.MeleeHitGlancing:
		// Unit::CalculateMeleeDamage glancing leg (Unit.cpp:1377-1397) —
		// pets glance like players (Unit.cpp:2276).
		damage = glancingReducedDamage(damage, uint8(motion.Level), targetLevel)
	case protocol.MeleeHitCrit:
		damage *= 2
	}

	overkill := uint32(0)
	if damage >= targetHealth {
		overkill = damage - targetHealth
		damage = targetHealth
	}

	// Unit::DealMeleeDamage (Unit.cpp:1480-1510): when the victim parries,
	// its next attack is hastened — the 20%/60% remaining bands collapse to
	// 20% of swing time or shrink by 40% (calcParryHastedRemaining). The
	// player and wild-creature paths bridge this (combat.go:834,
	// creaturemotion.go:2398); the pet path had no arm. NO_PARRY_HASTEN
	// (CREATURE_FLAG_EXTRA 0x8, combat.go:16) exempts creature victims.
	if targetState == protocol.VictimStateParry {
		if isTargetPlayer && targetSess != nil && targetSess.player != nil {
			vMainSpeed := time.Duration(targetSess.player.AttackTime) * time.Millisecond
			if vMainSpeed <= 0 {
				vMainSpeed = 2 * time.Second
			}
			elapsed := now.Sub(targetSess.lastSwing)
			if elapsed < vMainSpeed {
				rem := vMainSpeed - elapsed
				hasted := calcParryHastedRemaining(rem, vMainSpeed)
				targetSess.lastSwing = now.Add(-(vMainSpeed - hasted))
			}
		} else if !isTargetPlayer {
			s.motionMu.Lock()
			if vm := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, targetGUID); vm != nil {
				if vm.FlagsExtra&creatureFlagExtraNoParryHasten == 0 {
					cSpeed := time.Duration(vm.AttackTime) * time.Millisecond
					if cSpeed <= 0 {
						cSpeed = 2 * time.Second
					}
					elapsed := now.Sub(vm.LastAttack)
					if elapsed < cSpeed {
						rem := cSpeed - elapsed
						hasted := calcParryHastedRemaining(rem, cSpeed)
						vm.LastAttack = now.Add(-(cSpeed - hasted))
					}
				}
			}
			s.motionMu.Unlock()
		}
	}

	asuPkt := protocol.BuildAttackerStateUpdate(motion.GUID, targetGUID, damage, overkill, hitInfo, targetState, blocked)
	if isTargetPlayer && targetSess != nil {
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, true)
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, targetSess)
		// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
		// after the attacker-state update (sent pre-DealDamage in C++).
		// shieldGateDamage keeps the pre-god value for the damage-shield
		// gate below (Unit.cpp:1555).
		shieldGateDamage := damage
		damage = targetSess.negateGodModeDamage(damage)
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras
		// (81/153) redirect part of the damage to the aura's caster, ahead
		// of the 300 share leg.
		if owner != nil && damage > 0 {
			damage = owner.server.splitDamageToCaster(ctx, targetGUID, true, creatureAuraKey{}, motion.GUID, damage, 1)
		}
		// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
		if owner != nil && damage > 0 {
			owner.splitShareDamagePct(ctx, targetGUID, true, creatureAuraKey{}, motion.GUID, damage, 1)
		}
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED fires
		// for any damage to a player victim, pet attackers included.
		targetSess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
		// creatures are signaled OwnerAttackedBy on any non-DoT damage.
		s.triggerPetDefensive(ctx, targetSess.player.Map, targetSess.player.InstanceID, targetGUID, motion.GUID, scripting.CreatureEventOnOwnerAttackedAt)
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
		// Unit::AttackerStateUpdate's ProcSkillsAndAuras tail (Unit.cpp:2169):
		// after DealMeleeDamage the pet's own auras evaluate the done-side
		// melee event — ProcAttacker = PROC_FLAG_DONE_MELEE_AUTO_ATTACK |
		// PROC_FLAG_DONE_MAINHAND_ATTACK (Unit.cpp:1194; pets swing
		// BASE_ATTACK only), PROC_SPELL_TYPE_NONE, PROC_SPELL_PHASE_NONE,
		// outcome-derived hit mask (pet melee has no block roll). Runs on
		// both arms regardless of outcome or lethality, matching C++ (the
		// call is unconditional after DealMeleeDamage); the trigger spell
		// targets the victim.
		s.fireCreatureAuraProcLoop(ctx, motion, targetGUID, procEventInfo{
			typeMask:       procFlagDoneMeleeAutoAttack | procFlagDoneMainhandAttack,
			schoolMask:     spellSchoolMaskNormal,
			spellTypeMask:  procSpellTypeNone,
			spellPhaseMask: procSpellPhaseNone,
			hitMask:        meleeOutcomeProcHitMask(outcome, hitInfo, targetState, 0),
			actorGUID:      motion.GUID,
			damage:         damage,
		})
		// Unit::DealMeleeDamage (Unit.cpp:1555-1600): the player victim's
		// SPELL_AURA_DAMAGE_SHIELD (15) effects strike the pet attacker
		// after a damaging melee hit, gated on the pre-god damage (see
		// shieldGateDamage above).
		if shieldGateDamage > 0 {
			s.applyDamageShieldRetaliation(ctx,
				damageShieldSide{sess: targetSess, isPlayer: true},
				damageShieldSide{key: creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID}},
				shieldGateDamage)
		}
	} else {
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras
		// (81/153) redirect part of the damage to the aura's caster, ahead
		// of the 300 share leg.
		if owner != nil && damage > 0 {
			damage = owner.server.splitDamageToCaster(ctx, targetGUID, false, creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}, motion.GUID, damage, 1)
		}
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
				// Unit::setDeathState(JUST_DIED) -> RemoveAllAurasOnDeath
				// (Unit.cpp): the death aura strip runs even on this minimal
				// path — ahead of the KilledUnit/JustDied hooks, matching
				// the C++ order (strip at Unit.cpp:11320, hooks at
				// 11324-11387). The 86 channel-death-item leg
				// (SpellAuraEffects.cpp:4690) rides ahead of the strip, and
				// death-persistent auras survive it, mirroring
				// onCreatureKilled's strip.
				deathKey := creatureAuraKeyForMotion(victim)
				s.auraMu.Lock()
				var deathAuras []*activeAura
				for _, aura := range s.activeCreatureAuras[deathKey] {
					deathAuras = append(deathAuras, aura)
				}
				s.auraMu.Unlock()
				s.grantChannelDeathItems(ctx, deathAuras, victim.Level, true, victim.TapPlayerGUID, victim.TapGroupID)
				s.clearCreatureAurasOnDeath(deathKey)
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
		// Unit::DealMeleeDamage's victim leg (Unit.cpp:1194-1195): a pet's
		// BASE_ATTACK swing fires the creature victim's own taken-side proc
		// auras with PROC_FLAG_TAKEN_MELEE_AUTO_ATTACK, the trigger spell
		// targeting the pet attacker — the creature-side twin of the wild
		// path's procVictimAuraTriggers leg (creaturemotion.go) and the
		// player twin here (procs.go:2113). Runs after the damage legs
		// regardless of outcome (miss/dodge/parry/evade/immune carry the
		// outcome-derived hit mask, like the wild path's twin); on lethal
		// blows the victim's auras were stripped in the death chain, so
		// the event evaluates an empty set (net no-op). The pet's own
		// done-side melee leg (Unit.cpp:2169) fires after the taken-side
		// leg below.
		if victimMotion := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); victimMotion != nil {
			s.fireCreatureAuraProcLoop(ctx, victimMotion, motion.GUID, procEventInfo{
				typeMask:       procFlagTakenMeleeAutoAttack,
				schoolMask:     spellSchoolMaskNormal,
				spellTypeMask:  procSpellTypeNone,
				spellPhaseMask: procSpellPhaseNone,
				hitMask:        meleeOutcomeProcHitMask(outcome, hitInfo, targetState, 0),
				actorGUID:      motion.GUID,
				damage:         damage,
			})
		}
		// Unit::AttackerStateUpdate's ProcSkillsAndAuras tail (Unit.cpp:2169):
		// the pet's own done-side melee leg — the creature-victim twin of
		// the player-branch leg above. Fires regardless of outcome or
		// lethality; the pet's auras are unaffected by the victim's death.
		s.fireCreatureAuraProcLoop(ctx, motion, targetGUID, procEventInfo{
			typeMask:       procFlagDoneMeleeAutoAttack | procFlagDoneMainhandAttack,
			schoolMask:     spellSchoolMaskNormal,
			spellTypeMask:  procSpellTypeNone,
			spellPhaseMask: procSpellPhaseNone,
			hitMask:        meleeOutcomeProcHitMask(outcome, hitInfo, targetState, 0),
			actorGUID:      motion.GUID,
			damage:         damage,
		})
		// Unit::DealMeleeDamage (Unit.cpp:1555-1600): the creature victim's
		// SPELL_AURA_DAMAGE_SHIELD (15) effects strike the pet attacker
		// after a damaging melee hit. (The pet path models no absorb arm,
		// so the gate is the post-armor damage.)
		if damage > 0 {
			s.applyDamageShieldRetaliation(ctx,
				damageShieldSide{key: creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}},
				damageShieldSide{key: creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: motion.GUID}},
				damage)
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

// executePetAutocast mirrors PetAI::UpdateAI's autocast block
// (PetAI.cpp:96-188): every autocast slot is evaluated, spells held by the
// global cooldown (SpellHistory::HasGlobalCooldown) or still cooling down
// (SpellHistory::IsReady) are skipped, and ONE eligible candidate is picked
// at random (urand over targetSpellStore) and cast non-triggered. Go
// previously fired only AutocastSpells[0] every tick with no cooldown or
// GCD gate, so multi-spell pets never rotated and on-cooldown spells were
// re-cast freely.
// Positive spells (SpellInfo::IsPositive, SpellInfo.cpp:1205-1208 — the
// SPELL_ATTR0_CU_NEGATIVE flag, not the per-effect fold) take the
// CanBeUsedInCombat in-combat gate (PetAI.cpp:133-137): skipped while the
// pet is neither in combat nor commanded to attack. Their enemy-first
// targeting (pet attacker, then owner attacker, PetAI.cpp:142-156) has no
// Go twin — there is no attacker index — so the pet's current victim
// stands in as the enemy analog; the _allySet scan (PetAI.cpp:160-176)
// likewise has no twin, so the owner is the single ally candidate (C++
// scans pet+owner+group by GUID order; owner-only is the documented
// approximation). SPELL_EFFECT_JUMP_DEST spells (DK Ghoul's Leap) target
// enemies only (PetAI.cpp:158-160) and take the victim leg.
// Offensive spells take the victim leg (PetAI.cpp:183-190): the caller
// guarantees a live victim in combat, and CanBeUsedInCombat gates. The
// Spell::CanAutoCast aura-stack precheck rides petCanAutoCastTarget.
func (s *Server) executePetAutocast(ctx context.Context, motion *creatureMotion, targetGUID uint64, now time.Time) {
	if s == nil || motion == nil || len(motion.AutocastSpells) == 0 || s.Data == nil {
		return
	}
	owner := s.findSessionByGUID(motion.OwnerGUID)
	if owner == nil {
		return
	}
	type petAutocastCandidate struct {
		spell  wotlk.Spell
		target uint64
	}
	var candidates []petAutocastCandidate
	for _, spellID := range motion.AutocastSpells {
		if spellID == 0 {
			continue
		}
		spell, found, err := s.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		// PetAI.cpp:124-131 — HasGlobalCooldown / IsReady.
		if s.petGlobalCooldownActive(motion, spell, now) {
			continue
		}
		if !s.petSpellCooldownReady(motion, spell, now) {
			continue
		}
		usableInCombat := spell.Attributes&spellAttr0CantUsedInCombat == 0
		if spell.AttributesCu&spellAttr0CuNegativeMask == 0 {
			// Positive spell (PetAI.cpp:132-181).
			if usableInCombat && !motion.InCombat && motion.PetCommand != PetCommandAttack {
				continue
			}
			jumpToEnemy := false
			for _, eff := range spell.Effects {
				if eff.Effect == spellEffectJumpDest {
					jumpToEnemy = true
					break
				}
			}
			if jumpToEnemy {
				if targetGUID != 0 && usableInCombat && s.petCanAutoCastTarget(ctx, owner, motion, spell, targetGUID) {
					candidates = append(candidates, petAutocastCandidate{spell: spell, target: targetGUID})
				}
				continue
			}
			if ownerGUID := motion.OwnerGUID; ownerGUID != 0 && s.petCanAutoCastTarget(ctx, owner, motion, spell, ownerGUID) {
				candidates = append(candidates, petAutocastCandidate{spell: spell, target: ownerGUID})
			}
		} else if targetGUID != 0 && motion.InCombat && usableInCombat &&
			s.petCanAutoCastTarget(ctx, owner, motion, spell, targetGUID) {
			candidates = append(candidates, petAutocastCandidate{spell: spell, target: targetGUID})
		}
	}
	if len(candidates) == 0 {
		return
	}
	// PetAI.cpp:191-196 — urand(0, targetSpellStore.size() - 1).
	pick := candidates[rand.Intn(len(candidates))]
	owner.executePetSpell(ctx, motion, pick.spell, 0, protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: pick.target})
}

// petVictimHasBreakableCC bridges the PetAI::UpdateAI breakable-CC arm
// (PetAI.cpp:74-80): when the victim carries a breakable-by-damage
// crowd-control aura, the pet interrupts its non-melee casts and skips the
// rest of the tick — it must not exit combat and must not break the CC.
// Go pets carry no in-flight cast state (pet spells resolve instantly, so
// InterruptNonMeleeSpells is vacuous) and no channeled-spell state (the
// CURRENT_CHANNELED_SPELL outer gate and the channeled self-exclusion are
// vacuous), so the arm reduces to the tick-skip the caller applies. Covers
// player and creature victims via the shared breakableCCAuraPresent scan.
func (s *Server) petVictimHasBreakableCC(motion *creatureMotion, targetGUID uint64, isTargetPlayer bool, targetSess *session) bool {
	if s == nil || motion == nil || targetGUID == 0 {
		return false
	}
	if isTargetPlayer {
		if targetSess == nil {
			return false
		}
		return breakableCCAuraPresent(targetSess.activeAuras)
	}
	s.motionMu.Lock()
	victim := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, targetGUID)
	s.motionMu.Unlock()
	if victim == nil {
		return false
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	return breakableCCAuraPresent(s.activeCreatureAuras[creatureAuraKeyForMotion(victim)])
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
	if !s.server.checkMotionSpellCheckLocation(ctx, motion, spell) {
		return true
	}
	if !s.server.motionSpellFocusFound(ctx, motion, spell) {
		return true
	}
	if !s.server.checkCreatureSpellRange(motion, spell, targetGUID) {
		return true
	}
	// Spell::CheckCast CheckCasterAuras arm (Spell.cpp:5509): after
	// CheckPower on the creature path (boss_ai.go). Go's triggered models
	// TRIGGERED_IGNORE_CASTER_AURAS, so triggered pet casts skip the gate.
	if !triggered {
		if !s.server.checkCreatureCasterAuras(motion, spell) {
			return true
		}
	}
	// SPELL_EFFECT_DISPEL pre-cast arm (Spell.cpp:5540-5562, the
	// nothing-to-dispel / sanctuary gate): the creature-caster twin
	// (creatureDispelPreCast, dispel.go) applies to pet casters too — C++
	// gates on Unit::ToUnit, not on TYPEID_PLAYER — and the
	// !IsTriggered() gate is inside the twin. Placed before
	// takePetSpellPower like the creature path (a failing CheckCast gate
	// never drains power in C++).
	if !s.server.creatureDispelPreCast(motion, spell, targetGUID, triggered) {
		return true
	}
	// Spell::prepare stealth-break arm (Spell.cpp:3180-3191, gated by
	// TRIGGERED_IGNORE_AURA_INTERRUPT_FLAGS 0x100): a non-triggered pet
	// cast of a stealth-breaking spell (SpellInfo::IsBreakingStealth,
	// SpellInfo.cpp:1244-1247 == !SPELL_ATTR1_NOT_BREAK_STEALTH) drops
	// the pet's AURA_INTERRUPT_FLAG_CAST auras at cast start, ahead of
	// SendSpellStart in C++; any unit-target effect additionally drops
	// AURA_INTERRUPT_FLAG_SPELL_ATTACK auras (the player path's
	// spellEffectTargetsUnit approximation, spells.go:3076). Go's
	// triggered models TRIGGERED_FULL_MASK (0x0007FFFF), which carries
	// both TRIGGERED_CAST_DIRECTLY (0x80) and
	// TRIGGERED_IGNORE_AURA_INTERRUPT_FLAGS, so triggered casts skip the
	// arm — including the trigger-spell recursion below, whose parent
	// cast already broke stealth. C++ relative order: after the
	// CheckCast gates above, ahead of the _cast arms (the
	// lost-explicit-target arm below), matching the creature path.
	if !triggered && spell.AttributesEx&spellAttr1NotBreakStealth == 0 {
		s.server.removeCreatureAurasWithInterruptFlags(motion, auraInterruptFlagCast)
		for _, eff := range spell.Effects {
			if spellEffectTargetsUnit(eff.Effect) {
				s.server.removeCreatureAurasWithInterruptFlags(motion, auraInterruptFlagSpellAttack)
				break
			}
		}
	}
	// Spell::_cast lost-explicit-target arm (Spell.cpp:3287-3291, reached
	// via UpdatePointers at 3280): an explicit object target that no
	// longer resolves cancels the cast — no GO packet, no power drain, no
	// cooldown. The creature-path twin (creatureCastTargetResolves,
	// boss_ai.go) covers pet motions too. Self casts and target-less casts
	// are exempt (GetObjectTargetGUID()==0 skips the arm), matching the
	// creature path's guard. Runs for triggered casts too — the arm has
	// no triggered gate. C++ tests the loss before CheckCast; the pet
	// path collapses prepare+_cast, so it lands here after the CheckCast
	// gates and before the _cast arms, like the creature path.
	if targetGUID != 0 && targetGUID != motion.GUID && !s.server.creatureCastTargetResolves(motion, targetGUID) {
		return true
	}
	// Spell::_cast diminishing-returns recheck (Spell.cpp:3378-3399,
	// Unit::HasStrongerAuraWithDR, Unit.cpp:4744-4762): after CheckCast,
	// before SelectSpellTargets/TakePower — a crowd-control spell whose
	// DR-adjusted duration would land shorter than an already-active aura
	// of the same DR group on the target bounces
	// (SPELL_FAILED_AURA_BOUNCED). The session twin
	// (spellDiminishingBounced, spells.go) bridges player targets only —
	// Go's DR state is session-scoped with no per-creature
	// Unit::m_Diminishings bridge, so creature targets skip the recheck
	// (documented gap, same as the player path). The twin's non-triggered
	// DR group is C++-exact here: C++ keys on m_triggeredByAuraSpell,
	// which the pet path never sets (effect-triggered casts have it null,
	// pet AI casts are not aura-triggered).
	if s.spellDiminishingBounced(spell, targetGUID) {
		return true
	}
	// Spell::_cast creature SetInFront leg (Spell.cpp:3402-3406,
	// Unit::SetInFront, Unit.cpp:13663-13667): a creature caster with a
	// resolving non-empty combat target that isn't possessed faces it
	// before SelectSpellTargets. Keys on the pet's combat target
	// (GetTarget), not the spell target, like the creature path. The
	// UNIT_STATE_CANNOT_TURN gate has no Go model (standing no-bridge);
	// the angle mirrors Unit::GetAbsoluteAngle (atan2 normalized to
	// [0, 2pi), same as the distract bridge).
	if motion.TargetGUID != 0 && motion.UnitFlags&unitFlagPossessed == 0 {
		if tx, ty, ok := s.server.creatureUnitTargetPos(motion, motion.TargetGUID); ok {
			angle := float32(math.Atan2(float64(ty-motion.Y), float64(tx-motion.X)))
			if angle < 0 {
				angle += 2 * math.Pi
			}
			motion.Orientation = angle
		}
	}
	// Spell::SelectExplicitTargets magic-redirect arm (Spell.cpp:724-757)
	// via Spell::SelectSpellTargets (Spell.cpp:761, run in _cast at
	// 3410): no caster-type gate — it runs for pet casts too, after the
	// DR recheck and SetInFront and ahead of TakePower (3449). A live
	// Grounding Totem magnet on the victim pulls the cast onto the totem
	// and consumes the magnet, like the creature path
	// (boss_ai.go). The redirect precedes the dest-family arms inside
	// SelectSpellTargets, so the destination re-resolves on the
	// redirected target; the LOS gates above keep the pre-redirect
	// resolution (C++ evaluates them at CheckCast time, before the
	// redirect). Runs for triggered casts too (no C++ triggered
	// exemption on SelectExplicitTargets).
	if redirectGUID, ok := s.server.creatureGroundingTotemRedirect(ctx, motion, spell, targetGUID); ok {
		targetGUID = redirectGUID
		target.UnitGUID = redirectGUID
		dx, dy, dz, hasDest = s.server.resolveCreatureSpellDestination(motion, spell, targetGUID)
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
	// Spell::prepare aura-scaling arm (Spell.cpp:3040-3056) + the AddUnitTarget
	// min-level arm (Spell.cpp:2115-2150) + the SelectTargets removal leg
	// (Spell.cpp:807-830) for a pet caster. The fill arm gates on
	// IsControlledByPlayer(), which holds on this path: the pet path serves
	// player-owned pets only (driven by the owner session), mirroring the
	// minion leg (Unit.cpp:6056). The cast row is the pristine DBC row, so
	// the basepoint-equality gate is exact via spellAuraScaleMask(spell,
	// spell, true). Triggered pet casts model TRIGGERED_FULL_MASK
	// (0x0007FFFF, SpellDefines.h:153), which carries
	// TRIGGERED_IGNORE_AURA_SCALING (0x10), so the arm is skipped for
	// triggered casts. Targets failing the min-level check drop out of the
	// hit list ahead of the GO packet (SelectSpellTargets at Spell.cpp:3410
	// precedes SendSpellGo at 4338); when none remain the cast fails with
	// SPELL_FAILED_LOWLEVEL (807-830) — silent here, the AI path has no
	// client to report to. The downrank map feeds the aura application below
	// (PreprocessSpellHit scaleAura, Spell.cpp:2780-2795).
	var petAuraScaleDownranks map[uint64]wotlk.Spell
	if !triggered && s.server != nil && s.server.Data != nil {
		if mask := spellAuraScaleMask(spell, spell, true); mask != 0 {
			var failed bool
			hitTargets, petAuraScaleDownranks, failed = s.applyPetAuraScaling(ctx, spell, motion.GUID, mask, hitTargets)
			if failed {
				return true
			}
		}
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
	// Spell::AddUnitTarget reflect arm (Spell.cpp:2152, 2174-2188) for a
	// pet caster: a victim whose reflect roll succeeds becomes a REFLECT
	// miss and the spell returns to the pet (PreprocessTarget /
	// DoEffectOnLaunchTarget, Spell.cpp:2350, 7740-7743 — the pet is the
	// return trip's _spellHitTarget). Runs ahead of SendSpellGo like C++
	// (AddUnitTarget precedes 4338). Each reflected target produces its
	// own return trip on the pet. The ProcReflectDelayed arm
	// (Spell.cpp:2181) fires the reflector's taken-side procs at missile
	// arrival — creature reflectors have no Go event-list model, so only
	// player reflectors arm it (same as the player-path doReflect).
	reflectedSet := map[uint64]bool{}
	type petReflectTrip struct {
		victimGUID uint64
		status     uint8
	}
	var reflectTrips []petReflectTrip
	for _, guid := range hitTargets {
		if guid == 0 || reflectedSet[guid] {
			continue
		}
		if ok, status := s.petSpellReflectVerdict(ctx, motion, spell, guid); ok {
			reflectedSet[guid] = true
			reflectTrips = append(reflectTrips, petReflectTrip{victimGUID: guid, status: status})
		}
	}
	var missStatus []protocol.SpellMissStatus
	packetHits := make([]uint64, 0, len(hitTargets)+1)
	petHitListed := false
	for _, guid := range hitTargets {
		if reflectedSet[guid] {
			continue
		}
		packetHits = append(packetHits, guid)
	}
	for _, trip := range reflectTrips {
		missStatus = append(missStatus, protocol.SpellMissStatus{TargetGUID: trip.victimGUID, Reason: protocol.SpellMissReflect, ReflectStatus: trip.status})
		if trip.status == protocol.SpellMissNone && !petHitListed {
			packetHits = append(packetHits, motion.GUID)
			petHitListed = true
		}
		if s.server.findSessionByGUID(trip.victimGUID) != nil {
			reflectorGUID := trip.victimGUID
			delayMs := s.spellTargetTimeDelayMs(ctx, spell, trip.victimGUID)
			time.AfterFunc(time.Duration(delayMs)*time.Millisecond, func() {
				if s.server == nil {
					return
				}
				if rsess := s.server.findSessionByGUID(reflectorGUID); rsess != nil {
					rsess.procSpellReflectTakenAuraTriggers(context.Background(), motion.GUID, spell, triggered)
				}
			})
		}
	}
	// resolveReflectTarget maps a reflected victim to the pet for the
	// effect legs below — the return trip's _spellHitTarget is the caster
	// (Spell.cpp:2350, 7740-7743), so damage, heal, dispel, aura and
	// trigger effects all land on the pet instead of the reflector.
	resolveReflectTarget := func(guid uint64) uint64 {
		if reflectedSet[guid] {
			return motion.GUID
		}
		return guid
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
	goPacket := protocol.BuildSpellGoWithPower(motion.GUID, motion.GUID, castCount, spell.ID, castFlags, stamp, packetHits, missStatus, spellGoPacketTarget(spell, target), power)
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPacket, true); err != nil {
		return false
	}
	nearbyPacket := protocol.BuildSpellGo(motion.GUID, motion.GUID, castCount, spell.ID, castFlags&^protocol.SpellCastFlagPowerLeftSelf, stamp, packetHits, missStatus, spellGoPacketTarget(spell, target))
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearbyPacket, s)
	damage, hasDamage := creatureSpellDamage(s.server, spell, motion.Level, true)
	if hasDamage {
		schoolMask := uint8(spell.SchoolMask)
		if schoolMask == 0 {
			schoolMask = 1
		}
		s.executePetSpellDamage(ctx, motion, resolveReflectTarget(targetGUID), spell.ID, damage, schoolMask, triggered)
		// SelectImplicitAreaTargets (Spell.cpp:1227): each area-selected
		// enemy takes its own hit — the per-target miss roll, resistance,
		// absorb and kill legs run per target inside
		// executePetSpellDamage, matching DoTargetSpellHit per target.
		for _, areaHit := range areaHits {
			s.executePetSpellDamage(ctx, motion, resolveReflectTarget(areaHit.GUID), spell.ID, damage, schoolMask, triggered)
		}
	}
	for i, effect := range spell.Effects {
		if effect.Effect == spellEffectHeal || effect.Effect == spellEffectHealMechanical || effect.Effect == spellEffectHealPct {
			// Spell::EffectHeal / EffectHealMechanical / EffectHealPct
			// (SpellEffects.cpp:1393/1495/1513): damage < 0 heals nothing —
			// the uint32 wrap must never turn a negative rolled value into
			// a huge heal. The old raw 105 here was
			// SPELL_EFFECT_SUMMON_OBJECT_SLOT2 (SharedDefines.h:916), not
			// SPELL_EFFECT_HEAL_MECHANICAL (75) — summon-object spells were
			// being healed.
			healValue := effect.CalcValueForLevel(spell, uint32(motion.Level))
			if healValue > 0 {
				heal := uint32(healValue)
				if effect.Effect == spellEffectHealPct {
					// Spell::EffectHealPct (SpellEffects.cpp:1477-1492):
					// the effect value is a percentage of the TARGET's max
					// health (Unit::CountPctFromMaxHealth), not a flat
					// amount — like the player path at spells.go:10110.
					healTarget, ok := s.getCombatTarget(ctx, resolveReflectTarget(targetGUID))
					if !ok || healTarget.MaxHealth == 0 {
						continue
					}
					heal = uint32(uint64(healTarget.MaxHealth) * uint64(healValue) / 100)
				}
				s.executePetSpellHeal(ctx, motion, resolveReflectTarget(targetGUID), spell.ID, heal)
			}
		}
		if effect.Effect == spellEffectTriggerSpell && effect.TriggerSpell != 0 && effect.TriggerSpell != spell.ID {
			if triggered, found, err := s.server.Data.Spell(effect.TriggerSpell); err == nil && found {
				triggerTarget := target
				if reflectedSet[target.UnitGUID] {
					triggerTarget.UnitGUID = motion.GUID
				}
				s.executePetSpellWithOptions(ctx, motion, triggered, 0, triggerTarget, true)
			}
		}
		if effect.Effect == spellEffectDispel {
			s.executePetSpellDispel(ctx, motion, resolveReflectTarget(targetGUID), spell, effect, reflectedSet[targetGUID])
		}
		if effect.Effect == spellEffectDispelMechanic {
			// Spell::EffectDispelMechanic (SpellEffects.cpp:4733-4756):
			// the pet path's dispel arm handled only effect 38 — 108
			// fell to the unhandled arm and did nothing, while C++
			// dispatches it per unit target (SpellEffects.cpp:177) and the
			// player path bridges it (spells.go case 108). Reflection
			// maps the victim to the pet like the 38 arm above
			// (Spell.cpp:2350, 7740-7743).
			s.handleEffectDispelMechanic(ctx, resolveReflectTarget(targetGUID), spell, effect)
		}
		if effect.Effect == spellEffectApplyAura || effect.Effect == spellEffectApplyAreaAuraParty || effect.Effect == spellEffectApplyAreaAuraEnemy {
			// Spell::PreprocessSpellHit scaleAura leg (Spell.cpp:2780-2795):
			// a target whose aura scales takes the aura from the
			// rank-appropriate row — duration entry, basepoints and effect
			// row all come from it, like hitInfo.AuraBasePoints.
			auraSpell, auraEffect := petAuraRankRow(spell, i, petAuraScaleDownranks, targetGUID)
			durationMs := petAuraDuration(s.server, auraSpell, motion)
			amount := uint32(auraEffect.CalcValueForLevel(auraSpell, uint32(motion.Level)))
			if s.applyPetAura(ctx, motion, auraSpell, auraEffect, resolveReflectTarget(targetGUID), durationMs, amount) {
			}
			// SPELL_EFFECT_APPLY_AREA_AURA_ENEMY (129, SharedDefines.h:940):
			// an enemy area aura applies to every unit the area selection
			// picked — the wire target above plus the extra area hits. The
			// old raw 27 was SPELL_EFFECT_PERSISTENT_AREA_AURA, which C++
			// routes through DynamicObject (no pet-path bridge), never
			// per-target application.
			if effect.Effect == spellEffectApplyAreaAuraEnemy && (isAreaEnemyTargetType(effect.ImplicitTargetA) || isAreaEnemyTargetType(effect.ImplicitTargetB)) {
				for _, areaHit := range areaHits {
					aSpell, aEffect := petAuraRankRow(spell, i, petAuraScaleDownranks, areaHit.GUID)
					aDurationMs := petAuraDuration(s.server, aSpell, motion)
					aAmount := uint32(aEffect.CalcValueForLevel(aSpell, uint32(motion.Level)))
					s.applyPetAura(ctx, motion, aSpell, aEffect, resolveReflectTarget(areaHit.GUID), aDurationMs, aAmount)
				}
			}
		}
	}
	if !triggered {
		now := time.Now()
		s.recordPetSpellCooldown(motion, spell, now)
		// Spell::cast (Spell.cpp:3196): a non-triggered cast triggers
		// the global cooldown unless TRIGGERED_IGNORE_GCD — Go's
		// triggered models TRIGGERED_FULL_MASK, which carries it.
		s.server.triggerPetGlobalCooldown(motion, spell, now)
	}
	// Spell::_cast spell_linked_spell tail (Spell.cpp:3503-3512): type-0 rows
	// fire at cast completion for pet casters too — the twin is shared with
	// the creature path (pets are creature motions, and the linked cast is
	// triggered, so the pet/creature path split is inert there). No
	// triggered exemption in C++, so this runs for triggered casts as well.
	s.server.fireCreatureSpellLinkedTriggers(ctx, motion, spell.ID, targetGUID)
	// Spell::_cast modOwner tail (Spell.cpp:3514-3522): the cheat-cooldown
	// reset on the owner's session. No triggered exemption in C++.
	s.resetPetSpellCooldownCheat(motion, spell.ID)
	// Spell::_cast "Handle procs on cast" tail (Spell.cpp:3525-3545): the
	// PHASE_CAST proc leg fires for pet casters too — C++ runs
	// Unit::ProcSkillsAndAuras on m_originalCaster (the pet motion here),
	// with no triggered exemption. Pet self-buffs land in
	// activeCreatureAuras under creatureAuraKeyForMotion(motion):
	// getCombatTarget resolves the pet through findCreatureMotionLocked,
	// carrying the motion's Map/InstanceID with the pet GUID, and the pet
	// cast-aura key (pets.go) is the same {Map, InstanceID, petGUID} —
	// so the shared creature proc loop scans the right index. Placed
	// after the modOwner arm, matching the C++ tail order
	// (spell_linked -> modOwner -> procs).
	s.server.fireCreatureCastPhaseAuraProcs(ctx, motion, spell.ID, triggered)
	return true
}

func casterLevel(motion *creatureMotion) uint32 {
	if motion == nil || motion.Level == 0 {
		return 1
	}
	return motion.Level
}

// applyPetAuraScaling mirrors the AddUnitTarget ScaleAura arm
// (Spell.cpp:2115-2150) and the SelectTargets removal leg (Spell.cpp:807-830)
// for a pet caster. casterGUID is the pet's own GUID — the C++ self-cast
// exemption keys on m_caster, not on the pet's owner. For every hit target
// other than the caster whose effect mask is exactly the aura-scale mask,
// the target passes the min-level check when targetLevel+10 reaches the
// first rank's SpellLevel; failing targets drop from the hit list, and when
// every target is dropped the cast fails with SPELL_FAILED_LOWLEVEL
// (Spell.cpp:827-830), reported via failed. Targets whose level cannot be
// resolved are kept unscaled. Passing lower-level targets are recorded in
// the returned map with their downranked spell row — the PreprocessSpellHit
// scaleAura leg (Spell.cpp:2780-2795) that applies the aura with the
// rank-appropriate SpellInfo and basepoints.
func (s *session) applyPetAuraScaling(ctx context.Context, spell wotlk.Spell, casterGUID uint64, mask uint8, hitTargets []uint64) ([]uint64, map[uint64]wotlk.Spell, bool) {
	var targetMask uint8
	for i := range spell.Effects {
		if spell.Effects[i].Effect != 0 {
			targetMask |= 1 << uint(i)
		}
	}
	var firstLevel uint32
	if s.server != nil && s.server.Data != nil {
		if fr, found, _ := s.server.Data.Spell(s.server.spellFirstRank(spell.ID)); found {
			firstLevel = fr.SpellLevel
		}
	}
	downranks := make(map[uint64]wotlk.Spell)
	kept := make([]uint64, 0, len(hitTargets))
	for _, guid := range hitTargets {
		if guid == 0 || guid == casterGUID || targetMask != mask {
			kept = append(kept, guid)
			continue
		}
		level, ok := s.auraScaleTargetLevel(ctx, guid)
		if !ok || uint32(level)+10 < firstLevel {
			continue
		}
		if s.server != nil {
			if ds, ok := s.server.spellAuraRankForLevel(spell.ID, level); ok && ds.ID != spell.ID {
				downranks[guid] = ds
			}
		}
		kept = append(kept, guid)
	}
	if len(hitTargets) > 0 && len(kept) == 0 {
		return nil, nil, true
	}
	return kept, downranks, false
}

// petAuraRankRow mirrors the Spell::PreprocessSpellHit scaleAura leg
// (Spell.cpp:2780-2795) on the pet path: a target whose aura scales takes
// the aura from the rank-appropriate row. The spellAuraRankForLevel twin
// already enforced the effect-type match across all three effects, falling
// back to the cast row on mismatch, so the downrank row is safe to index
// directly.
func petAuraRankRow(spell wotlk.Spell, effectIdx int, downranks map[uint64]wotlk.Spell, targetGUID uint64) (wotlk.Spell, wotlk.SpellEffect) {
	if ds, ok := downranks[targetGUID]; ok {
		return ds, ds.Effects[effectIdx]
	}
	return spell, spell.Effects[effectIdx]
}

// petAuraDuration resolves the DBC duration for one pet-path aura
// application from the (possibly downranked) spell row, mirroring the
// Aura::CalcMaxDuration duration-entry leg (SpellAuras.cpp:871-886).
func petAuraDuration(s *Server, spell wotlk.Spell, motion *creatureMotion) uint32 {
	durationMs := uint32(0)
	if s != nil && s.Data != nil && spell.DurationIndex > 0 {
		if duration, found, err := s.Data.SpellDuration(spell.DurationIndex, casterLevel(motion)); err == nil && found && duration > 0 {
			durationMs = uint32(duration)
		}
	}
	return durationMs
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
		aura := &activeAura{SpellID: spell.ID, DispelType: spell.DispelType, Mechanic: spell.Mechanic, AuraType: effect.Aura, EffectMask: effectMask, RecalculateMask: recalculateMask, CasterGUID: caster.GUID, TargetGUID: targetGUID, SchoolMask: spell.SchoolMask, MiscValue: effect.MiscValue, Amount: uint32(amount), Amounts: amounts, BaseAmounts: baseAmounts, DurationMs: durationMs, PeriodMs: periodMs, RemainingMs: durationMs, Slot: slot, Positive: positive, CasterLevel: uint8(casterLevel(caster)), StoredCasterLevel: uint8(casterLevel(caster)), AuraInterruptFlags: spell.AuraInterruptFlags, TriggerSpell: effect.TriggerSpell, StackAmount: spell.StackAmount, HideDuration: spell.AttributesEx5&spellAttr5HideDuration != 0, StackCount: 1, OwnerPetAura: source.SpellID != 0, OwnerPetAuraSourceSpell: source.SpellID, OwnerPetAuraSourceEffect: source.EffectIndex, OwnerPetAuraSourceDamage: sourceDamage, OwnerPetAuraRemoveOnChange: removeOnChange}
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
		// No-stack purge on apply: AURA_REMOVE_BY_DEFAULT, not BY_EXPIRE.
		for _, purgeID := range purgeIDs {
			targetSess.removeAura(purgeID.spellID)
		}
		for _, e := range scPurge {
			s.expireSingleCastEntry(e)
		}
		wireMax, wireDuration := auraWireDurations(spell, durationMs, durationMs)
		packet := protocol.BuildAuraUpdateWithStackEffect(targetGUID, caster.GUID, slot, spell.ID, false, positive, wireMax, wireDuration, aura.StoredCasterLevel, 1, aura.EffectMask)
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
	aura := &activeAura{SpellID: spell.ID, DispelType: spell.DispelType, Mechanic: spell.Mechanic, AuraType: effect.Aura, EffectMask: effectMask, RecalculateMask: recalculateMask, CasterGUID: caster.GUID, TargetGUID: targetGUID, TargetKey: targetKey, SchoolMask: spell.SchoolMask, MiscValue: effect.MiscValue, Amount: uint32(amount), Amounts: amounts, BaseAmounts: baseAmounts, DurationMs: durationMs, PeriodMs: periodMs, RemainingMs: durationMs, Slot: slot, Positive: positive, CasterLevel: uint8(casterLevel(caster)), StoredCasterLevel: uint8(casterLevel(caster)), AuraInterruptFlags: spell.AuraInterruptFlags, TriggerSpell: effect.TriggerSpell, StackAmount: spell.StackAmount, HideDuration: spell.AttributesEx5&spellAttr5HideDuration != 0, StackCount: 1, OwnerPetAura: source.SpellID != 0, OwnerPetAuraSourceSpell: source.SpellID, OwnerPetAuraSourceEffect: source.EffectIndex, OwnerPetAuraSourceDamage: sourceDamage, OwnerPetAuraRemoveOnChange: removeOnChange}
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
	// No-stack purge on apply: AURA_REMOVE_BY_DEFAULT, not BY_EXPIRE.
	for _, p := range purge {
		s.server.removeCreatureAura(targetKey, p.spellID)
	}
	for _, e := range scPurge {
		s.expireSingleCastEntry(e)
	}
	wireMax, wireDuration := auraWireDurations(spell, durationMs, durationMs)
	packet := protocol.BuildAuraUpdateWithStackEffect(targetGUID, caster.GUID, slot, spell.ID, false, positive, wireMax, wireDuration, aura.StoredCasterLevel, 1, aura.EffectMask)
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

// petSpellReflectVerdict mirrors the AddUnitTarget reflect arm
// (Spell.cpp:2152, 2174-2188; WorldObject::SpellHitResult,
// Object.cpp:2617-2648) for a pet caster. The victim rolls the reflect arm:
// player victims via checkSpellReflection, creature victims via
// creatureCheckSpellReflection — C++ offers reflect to any Unit victim,
// not just players. The offer gate is spellReflectOffered with the owner
// session standing in for the pet's friendly relations (a pet shares its
// owner's faction template, so IsFriendlyTo matches). The this == victim
// early-out (Object.cpp:2633) sits ahead of the canReflect arm, so a
// pet self-cast never reflects. On success the second return is the
// return-trip ReflectResult (Spell.cpp:2178): the pet's own
// SpellHitResult with canReflect=false, which can only report IMMUNE —
// the immune arms precede the this == victim short-circuit, and the
// magic hit/resist roll sits behind it and never runs. A non-NONE
// status nulls the return trip (Spell.cpp:2350, 7740-7743): the GO
// packet still reports REFLECT with the status, but the pet takes
// nothing.
func (s *session) petSpellReflectVerdict(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, targetGUID uint64) (bool, uint8) {
	if s == nil || s.server == nil || motion == nil || targetGUID == 0 || targetGUID == motion.GUID {
		return false, protocol.SpellMissNone
	}
	if !spellCanBeReflected(spell) {
		return false, protocol.SpellMissNone
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	offered := false
	if targetSess != nil {
		offered = spellReflectOffered(spell, s, targetGUID, targetSess)
	} else {
		var targetFaction uint32
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			targetFaction = tgt.Faction
		}
		offered = s.creatureReflectOffered(spell, targetFaction)
	}
	if !offered {
		return false, protocol.SpellMissNone
	}
	reflected := false
	if targetSess != nil {
		reflected = targetSess.checkSpellReflection(spell)
	} else {
		var key creatureAuraKey
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			key = creatureAuraKeyForTarget(tgt)
		}
		reflected = s.server.creatureCheckSpellReflection(key, spell)
	}
	if !reflected {
		return false, protocol.SpellMissNone
	}
	if s.creatureTargetImmuneToSpell(ctx, motion.GUID, spell, s, motion.Faction) {
		return true, protocol.SpellMissImmune
	}
	if spellHasOnlyDamageEffects(spell) && creatureImmuneToDamageSpell(s.server, creatureAuraKeyForMotion(motion), spell, spell.SchoolMask) {
		return true, protocol.SpellMissImmune
	}
	return true, protocol.SpellMissNone
}

// firePetSpellDoneSideProcs evaluates the done-side proc leg of
// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586) on the pet
// caster's own auras: Unit::ProcSkillsAndAuras(caster, unitTarget, ...) with
// the m_procAttacker prefill (Spell.cpp:1999-2056 via
// spellDamageProcTypeMask), PROC_SPELL_TYPE_DAMAGE, PROC_SPELL_PHASE_HIT.
// It runs on every damage-dealing hit — misses and fully absorbed hits
// return before the damage legs, matching the taken-side placement. The
// canEffectTrigger gate (Spell.cpp:2440-2442) reduces to the attr3
// CANT_TRIGGER_PROC check: the CanExecuteTriggersOnHit arm folds into the
// pet path's non-zero-effect model and the creature path carries no CanProc
// reader. The reflect return trip (target == caster) is excluded — its
// bridge already fires the combined done+taken event on the pet's auras.
func (s *session) firePetSpellDoneSideProcs(ctx context.Context, caster *creatureMotion, targetGUID uint64, spellID uint32, schoolMask uint8, triggered bool) {
	if s == nil || s.server == nil || s.server.Data == nil || caster == nil || targetGUID == caster.GUID {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found || spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	spellCopy := spell
	s.server.fireCreatureAuraProcLoop(ctx, caster, targetGUID, procEventInfo{
		typeMask:       spellDamageProcTypeMask(spellCopy),
		schoolMask:     uint32(schoolMask),
		spellTypeMask:  procSpellTypeDamage,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        procHitNormal,
		triggered:      triggered,
		eventSpell:     &spellCopy,
		actorGUID:      caster.GUID,
	})
}

// countDoTsByCaster mirrors Unit::GetDoTsByCaster (Unit.cpp:4808-4832): the
// number of SPELL_AURA_PERIODIC_DAMAGE (3) /
// SPELL_AURA_PERIODIC_DAMAGE_PERCENT (89) aura effects applied to the victim
// whose caster is ownerGUID. C++ counts per AuraEffect, so each masked
// periodic effect of an aura counts separately here too.
func (s *Server) countDoTsByCaster(target combatTarget, ownerGUID uint64) uint32 {
	if s == nil || s.Data == nil || target.GUID == 0 || ownerGUID == 0 {
		return 0
	}
	isDotEffect := func(aura *activeAura, effect wotlk.SpellEffect, index int) bool {
		if aura == nil || aura.Stopped || aura.CasterGUID != ownerGUID {
			return false
		}
		if aura.EffectMask&(1<<uint(index)) == 0 {
			return false
		}
		return effect.Aura == spellAuraPeriodicDamage || effect.Aura == spellAuraPeriodicDamagePercent
	}
	countEffects := func(auras map[uint32]*activeAura) uint32 {
		var n uint32
		for _, aura := range auras {
			if aura == nil {
				continue
			}
			spell, found, err := s.Data.Spell(aura.SpellID)
			if err != nil || !found {
				continue
			}
			for index, effect := range spell.Effects {
				if isDotEffect(aura, effect, index) {
					n++
				}
			}
		}
		return n
	}
	if victim := s.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
		victim.castMu.Lock()
		n := countEffects(victim.activeAuras)
		victim.castMu.Unlock()
		return n
	}
	s.auraMu.Lock()
	auras := s.activeCreatureAuras[creatureAuraKeyForTarget(target)]
	copies := make(map[uint32]*activeAura, len(auras))
	for id, aura := range auras {
		copies[id] = aura
	}
	s.auraMu.Unlock()
	return countEffects(copies)
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
	// Unit::SpellDamagePctDone (Unit.cpp:7021-7023): the Shadow Bite arm — a
	// pet's Warlock-family spell with SpellFamilyFlags[1] & 0x00400000 gains
	// 15% per DoT the victim carries from the pet's owner (GetDoTsByCaster
	// counts PERIODIC_DAMAGE / PERIODIC_DAMAGE_PERCENT effects by caster
	// GUID). C++ gates on IsPet(); Go pet motions carry OwnerGUID.
	if caster.OwnerGUID != 0 && s.server != nil && s.server.Data != nil {
		if pspell, found, perr := s.server.Data.Spell(spellID); perr == nil && found &&
			pspell.SpellFamilyName == spellFamilyWarlock && pspell.SpellFamilyFlags[1]&0x00400000 != 0 {
			if dots := s.server.countDoTsByCaster(target, caster.OwnerGUID); dots > 0 {
				damage = uint32(uint64(damage) * (100 + 15*uint64(dots)) / 100)
			}
		}
	}
	hitInfo := uint32(0)
	resisted := uint32(0)
	absorbed := uint32(0)
	if target.GUID != caster.GUID {
		// WorldObject::MagicSpellHitResult (Object.cpp:2555): the caster's
		// m_modSpellHitChance arm runs for any unit caster — the pet's live
		// SPELL_AURA_MOD_SPELL_HIT_CHANCE (55) sum (AuraEffect::
		// HandleModSpellHitChance's creature arm) folds in here in percent
		// units. Player-cast pet buffs land in the creature aura store, so
		// the fold sees them; victim-side 186/160 arms stay parked (no
		// misc-masked creature aura sum), same as the player path.
		bonusHit := float64(0)
		if s.server != nil {
			bonusHit = float64(s.server.creatureAuraSpellHitSum(creatureAuraKeyForMotion(caster)))
		}
		miss := magicSpellHitResult(uint8(maxUint32(caster.Level, 1)), target.Level, isPlayerVictim, bonusHit)
		if miss != protocol.SpellMissNone {
			damage = 0
			hitInfo = 0x01
		} else if petSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
			// WorldObject::MagicSpellHitResult (Object.cpp:2585-2592): the
			// SPELL_AURA_MOD_DEBUFF_RESISTANCE (178) arm runs for pet-cast
			// spells too (pet debuff spells carry aura effects).
			if s.spellDebuffResisted(ctx, petSpell, targetGUID, s.server.findSessionByGUID(target.GUID)) {
				damage = 0
				hitInfo = 0x01
			} else if s.spellDeflectedByVictim(ctx, targetGUID, s.server.findSessionByGUID(target.GUID), caster.X, caster.Y) {
				// WorldObject::MagicSpellHitResult (Object.cpp:2597-2604):
				// the SPELL_AURA_DEFLECT_SPELLS (287) arm runs for
				// pet-cast spells too (any unit caster).
				damage = 0
				hitInfo = 0x01
			}
		}
	}
	if damage > 0 && schoolMask > 1 {
		// AuraEffect::HandleAuraModResistance (SpellAuraEffects.cpp:3255):
		// the live 22 flat amounts fold into a pet victim's resistances.
		resistances := s.server.petFoldedResistances(creatureAuraKeyForTarget(target), target.Resistances)
		resisted, damage = calcMagicSpellResistance(damage, schoolMask, resistances, uint8(maxUint32(caster.Level, 1)), target.Level, !isPlayerVictim, false, 0)
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
		// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras
		// (81/153) redirect part of the damage to the aura's caster, ahead
		// of the 300 share leg.
		if damage > 0 {
			damage = s.server.splitDamageToCaster(ctx, target.GUID, true, creatureAuraKey{}, caster.GUID, damage, uint32(schoolMask))
		}
		// Unit::DealDamage (Unit.cpp:766-788): SPELL_AURA_SHARE_DAMAGE_PCT.
		if damage > 0 {
			s.splitShareDamagePct(ctx, target.GUID, true, creatureAuraKey{}, caster.GUID, damage, uint32(schoolMask))
		}
		// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED fires
		// for any damage to a player victim, pet-caster spells included.
		victim.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
		// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
		// creatures are signaled OwnerAttackedBy on any non-DoT damage.
		s.server.triggerPetDefensive(ctx, victim.player.Map, victim.player.InstanceID, target.GUID, caster.GUID, scripting.CreatureEventOnOwnerAttackedAt)
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
			victim.procSpellDirectDamageAuras(spellID, damage)
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
		// The pet caster's done-side leg (DoDamageAndTriggers, Spell.cpp:2581)
		// runs after the taken side on both the lethal and non-lethal arms.
		s.firePetSpellDoneSideProcs(ctx, caster, target.GUID, spellID, schoolMask, triggered)
		return
	}
	// Unit::CalcAbsorbResist (Unit.cpp:1977-2060): split-damage auras (81/153)
	// redirect part of the damage to the aura's caster, ahead of the 300
	// share leg.
	if damage > 0 {
		damage = s.server.splitDamageToCaster(ctx, target.GUID, false, creatureAuraKey{Map: target.Map, InstanceID: target.InstanceID, GUID: target.GUID}, caster.GUID, damage, uint32(schoolMask))
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
	// The pet caster's done-side leg (DoDamageAndTriggers, Spell.cpp:2581)
	// runs after the taken side on both the lethal and non-lethal arms —
	// C++ fires ProcSkillsAndAuras after DealSpellDamage even when the blow
	// kills. The self-hit is excluded (the return-trip bridge below already
	// fires the combined done+taken event on the pet's auras).
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586):
	// Unit::ProcSkillsAndAuras (Unit.cpp:5355-5366) fires the victim-side
	// half with the taken pre-fill plus PROC_FLAG_TAKEN_DAMAGE ORed by
	// the damage arm (Spell.cpp:2545) — the creature victim's own auras
	// evaluate the hit as a taken-proc event (PROC_SPELL_TYPE_DAMAGE,
	// PROC_SPELL_PHASE_HIT; Unit.cpp:10385-10418). Mirrors the
	// player-victim branch's taken leg above: runs after the damage legs
	// on both the lethal and non-lethal arms, the hit mask always NORMAL
	// (misses return early above), the attr3 arm checked inline (the
	// creature loop has no attr3 gate), and the trigger spell targeting
	// the pet attacker. The self-hit is excluded — the return-trip
	// bridge below already fires the combined done+taken event there.
	if target.GUID != caster.GUID {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found && spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
			typeMask := spellDamageTakenProcTypeMask(spell) | procFlagTakenDamage
			if typeMask != 0 {
				spellCopy := spell
				s.server.fireCreatureAuraProcLoop(ctx, targetMotion, caster.GUID, procEventInfo{
					typeMask:       typeMask,
					schoolMask:     uint32(schoolMask),
					spellTypeMask:  procSpellTypeDamage,
					spellPhaseMask: procSpellPhaseHit,
					hitMask:        procHitNormal,
					triggered:      triggered,
					eventSpell:     &spellCopy,
					actorGUID:      caster.GUID,
					damage:         damage,
				})
			}
		}
	}
	s.firePetSpellDoneSideProcs(ctx, caster, target.GUID, spellID, schoolMask, triggered)
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581-2586) on the
	// reflect return trip: the pet is both caster and _spellHitTarget
	// (Spell.cpp:2350), so its own auras evaluate the hit with the
	// done-side prefill, the taken-side prefill plus PROC_FLAG_TAKEN_DAMAGE
	// (Spell.cpp:2545) — the same parked self-proc leg as the inline
	// path. Only the self-hit (target == caster) runs it here: the pet
	// path has no done-side model for ordinary hits (standing gap), and
	// the return trip is the one hit whose victim is the pet itself.
	// Non-lethal only — C++ strips the caster's auras in Unit::Kill before
	// DoDamageAndTriggers runs, so a lethal return trip evaluates an empty
	// set (net no-op).
	if targetMotion.Health != 0 && target.GUID == caster.GUID {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found && spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
			typeMask := spellDamageProcTypeMask(spell) | spellDamageTakenProcTypeMask(spell) | procFlagTakenDamage
			if typeMask != 0 {
				spellCopy := spell
				s.server.fireCreatureAuraProcLoop(ctx, caster, caster.GUID, procEventInfo{
					typeMask:       typeMask,
					schoolMask:     uint32(schoolMask),
					spellTypeMask:  procSpellTypeDamage,
					spellPhaseMask: procSpellPhaseHit,
					hitMask:        procHitNormal,
					triggered:      triggered,
					eventSpell:     &spellCopy,
					actorGUID:      caster.GUID,
					damage:         damage,
				})
			}
		}
	}
}
