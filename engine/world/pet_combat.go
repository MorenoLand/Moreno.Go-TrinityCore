package world

import (
	"context"
	"math"
	"math/rand"
	"sort"
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
	enteredCombat := false
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil && motion.Health > 0 {
		enteredCombat = !motion.InCombat
		motion.TargetGUID = targetGUID
		motion.InCombat = true
		motion.PetCommand = PetCommandAttack
		motion.Moving = true
	}
	s.motionMu.Unlock()
	// Eluna CREATURE_EVENT_ON_ENTER_COMBAT (1): C++ PetAI::AttackStart
	// reaches it via ElunaCreatureAI::JustEngagedWith for
	// binding-carrying pets (CreatureHooks.cpp:105-113). Fires only on
	// fresh engages (enteredCombat), after the unlock — Lua handlers lock
	// motionMu on demand.
	if enteredCombat {
		s.firePetEnterCombat(ctx, motion, targetGUID)
	}
}

// onPetCommandFollow handles recalling the pet back to follow the owner.
// Mirrors TrinityCore PetAI::DoRecall (PetAI.cpp:215).
func (s *Server) onPetCommandFollow(ctx context.Context, mapID, instanceID uint32, petGUID uint64) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	var leftMotion *creatureMotion
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		if motion.InCombat {
			leftMotion = motion
		}
		motion.TargetGUID = 0
		motion.InCombat = false
		motion.PetCommand = PetCommandFollow
		motion.Moving = false
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
	}
	s.motionMu.Unlock()
	// Eluna CREATURE_EVENT_ON_LEAVE_COMBAT (2) + On_Reset (23) twin:
	// the recall drops a live pet out of combat (PetAI::DoRecall leg).
	if leftMotion != nil {
		s.firePetLeaveCombat(ctx, leftMotion)
	}
}

// onPetCommandStay handles ordering the pet to stay at its current position.
// PetHandler.cpp HandlePetActionHelper COMMAND_STAY:151-162 clears the
// motion and sets the stay command state but does NOT stop the attack — the
// pet keeps its victim and keeps swinging while the victim stays in melee
// range (PetAI::UpdateAI:84-89, bridged in petCombatPursuitAndAttack's
// stay arm). Go used to clear TargetGUID/InCombat/threat here, which
// disengaged the pet outright; the victim now survives until NeedToStop or
// death clears it.
func (s *Server) onPetCommandStay(mapID, instanceID uint32, petGUID uint64) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		motion.PetCommand = PetCommandStay
		motion.Moving = false
	}
}

// onPetSetReaction updates the pet's reaction state (passive, defensive, aggressive).
func (s *Server) onPetSetReaction(ctx context.Context, mapID, instanceID uint32, petGUID uint64, reactState uint8, source *session) {
	if s == nil || petGUID == 0 {
		return
	}
	var stopVictim uint64
	var leftMotion *creatureMotion
	s.motionMu.Lock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		motion.PetReact = reactState
		if reactState == PetReactPassive {
			// PetHandler.cpp:263-264: the REACT_PASSIVE arm calls
			// pet->AttackStop() — a pet mid-command-attack disengages
			// too, where the old PetCommand != Attack carve-out kept it
			// attacking. Unit::AttackStop (Unit.cpp:5763-5789) emits
			// SMSG_ATTACK_STOP (broadcast to the set) only when an
			// attack was actually in progress.
			stopVictim = motion.TargetGUID
			if motion.InCombat {
				leftMotion = motion
			}
			motion.TargetGUID = 0
			motion.InCombat = false
			motion.Moving = false
		}
	}
	s.motionMu.Unlock()
	// Eluna CREATURE_EVENT_ON_LEAVE_COMBAT (2) + On_Reset (23) twin: the
	// passive switch drops a live pet out of combat. Fires after the
	// unlock — Lua handlers lock motionMu on demand.
	if leftMotion != nil {
		s.firePetLeaveCombat(ctx, leftMotion)
	}
	if stopVictim != 0 {
		stopPkt := buildAttackStop(petGUID, stopVictim, false)
		// Unit::AttackStop -> SendMeleeAttackStop (Unit.cpp:2402-2404) uses
		// SendMessageToSet(&data, true): the broadcast reaches the nearby
		// set including the sender — the old whole-instance broadcast was
		// too wide and skipped the owner.
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, source)
		if source != nil {
			_ = source.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
		}
	}
}

// petCommandOnSpellCast mirrors the CharmInfo clears at the top of the
// pet action-bar spell arm (PetHandler.cpp:304-310): the pet leaves its
// stay/follow/return states and enters command-attack as if the owner
// clicked 'attack'. Returns the previous command so the failure path can
// restore it (PetHandler.cpp:380-382).
func (s *Server) petCommandOnSpellCast(mapID, instanceID uint32, petGUID uint64) uint8 {
	if s == nil || petGUID == 0 {
		return PetCommandStay
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	motion := s.motionMapLocked(mapID, instanceID)[petGUID]
	if motion == nil {
		return PetCommandStay
	}
	prev := motion.PetCommand
	motion.PetCommand = PetCommandAttack
	return prev
}

// restorePetCommand restores the pet's command state after a failed
// action-bar spell cast (PetHandler.cpp:380-382: SetIsCommandAttack(false);
// the AI resets the rest after a successful AttackStart).
func (s *Server) restorePetCommand(mapID, instanceID uint32, petGUID uint64, prev uint8) {
	if s == nil || petGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		motion.PetCommand = prev
	}
}

// switchPetVictim mirrors the forced victim switch after SPELL_CAST_OK in
// the pet action-bar spell arm (PetHandler.cpp:351-365): PetAI::AttackStart
// onto the spell's unit target when it differs from the current victim.
func (s *Server) switchPetVictim(ctx context.Context, mapID, instanceID uint32, petGUID, targetGUID uint64) {
	if s == nil || petGUID == 0 || targetGUID == 0 {
		return
	}
	s.motionMu.Lock()
	var engagedMotion *creatureMotion
	if motion := s.motionMapLocked(mapID, instanceID)[petGUID]; motion != nil {
		if !motion.InCombat {
			engagedMotion = motion
		}
		motion.TargetGUID = targetGUID
		motion.InCombat = true
	}
	s.motionMu.Unlock()
	// Eluna CREATURE_EVENT_ON_ENTER_COMBAT (1): a victim switch onto an
	// idle pet is a fresh engage (ElunaCreatureAI::JustEngagedWith twin);
	// a mid-fight switch fires no new event, matching C++.
	if engagedMotion != nil {
		s.firePetEnterCombat(ctx, engagedMotion, targetGUID)
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
	type petEngage struct {
		motion     *creatureMotion
		targetGUID uint64
	}
	var entered []petEngage
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
		if !m.InCombat {
			entered = append(entered, petEngage{motion: m, targetGUID: targetGUID})
		}
		m.TargetGUID = targetGUID
		m.InCombat = true
		m.Moving = m.PetCommand != PetCommandStay
	}
	s.motionMu.Unlock()
	// Eluna CREATURE_EVENT_ON_ENTER_COMBAT (1): defensive pets engage via
	// PetAI::OwnerAttackedBy / OwnerAttacked (PetAI.cpp); binding-carrying
	// pets reach it through ElunaCreatureAI::JustEngagedWith. Fires only
	// for fresh engages, after the unlock.
	for _, e := range entered {
		s.firePetEnterCombat(ctx, e.motion, e.targetGUID)
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
	var entered []*creatureMotion
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
		if !m.InCombat {
			entered = append(entered, m)
		}
		m.TargetGUID = targetGUID
		m.InCombat = true
		m.Moving = true
	}
	s.motionMu.Unlock()
	// Eluna CREATURE_EVENT_ON_ENTER_COMBAT (1): pets engage via
	// PetAI::OwnerAttacked (PetAI.cpp:292-312); binding-carrying pets
	// reach it through ElunaCreatureAI::JustEngagedWith. Fresh engages
	// only, fired after the unlock.
	for _, m := range entered {
		s.firePetEnterCombat(ctx, m, targetGUID)
	}
}

// checkTempSummonExpiry twins the TempSummon::Update timed arms
// (TemporarySummon.cpp:66-170). TEMPSUMMON_TIMED_DESPAWN (2) decrements
// UNCONDITIONALLY — even in combat — and fires at spawn+lifetime
// (TemporarySummon.cpp:80-89). The TIMED_OR_DEAD (0) / TIMED_OR_CORPSE (1)
// / TIMED_DESPAWN_OUT_OF_COMBAT (3) arms freeze the timer while the
// summon is in combat: m_timer is reset to m_lifetime on every Update
// tick in combat (TemporarySummon.cpp:91-106, 133-168), so the expiry
// only runs once the summon leaves combat. The corpse/dead legs
// (4/5/6) never reach this function — the script-summon bridge only
// stamps the timer for the timed family, and the spell-summon path only
// produces type 2.
// Returns true when the timer fired and the guardian was unsummoned — the
// caller must stop driving the motion.
func (s *Server) checkTempSummonExpiry(ctx context.Context, motion *creatureMotion, now time.Time) bool {
	if s == nil || motion == nil || motion.TempSummonLifetime <= 0 {
		return false
	}
	switch motion.TempSummonType {
	case 0, 1, 3:
		s.motionMu.Lock()
		inCombat := motion.InCombat
		s.motionMu.Unlock()
		if inCombat {
			s.motionMu.Lock()
			motion.TempSummonExpiresAt = now.Add(motion.TempSummonLifetime)
			s.motionMu.Unlock()
			return false
		}
	}
	if now.Before(motion.TempSummonExpiresAt) {
		return false
	}
	s.expireTempSummonGuardian(ctx, motion)
	return true
}

// expireTempSummonGuardian is the TempSummon::UnSummon twin
// (TemporarySummon.cpp:250-304) for timed guardians: the client object is
// destroyed, the motion removed, and the character_pet row deleted — C++
// TempSummons are never saved, so expiry must leave no row for a
// login/logout resummon to resurrect. Robust to a missing owner session:
// the server-side teardown proceeds and the owner's PetGUID cleanup is
// skipped when the owner is offline.
func (s *Server) expireTempSummonGuardian(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	ownerGUID, petID := motion.OwnerGUID, motion.PetID
	s.motionMu.Lock()
	if s.findCreatureMotionLocked(mapID, instanceID, guid) != motion {
		s.motionMu.Unlock()
		return
	}
	delete(s.motionMapLocked(mapID, instanceID), guid)
	s.motionMu.Unlock()
	s.clearCreatureAuras(creatureAuraKey{Map: mapID, InstanceID: instanceID, GUID: guid})
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(guid)
	destroyBuf.WriteU8(0)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), nil)
	if s.CharactersStore != nil && s.CharactersStore.DB != nil && petID != 0 {
		cdb := s.CharactersStore.DB
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_pet WHERE id = ? AND owner = ?", petID, ownerGUID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ?", petID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid = ?", petID)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_spell_cooldown WHERE guid = ?", petID)
	}
	if owner := s.findSessionByGUID(ownerGUID); owner != nil {
		owner.removeOwnerPetAuraSourcesOnPetChange(ctx, guid)
		if owner.player != nil && owner.player.PetGUID == guid {
			owner.player.PetGUID = 0
			owner.player.PetNumber = 0
			owner.sendPlayerUpdate()
			buf := protocol.NewBuffer(8)
			buf.WriteU64(0)
			_ = owner.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		}
	}
	// TempSummon::UnSummon (TemporarySummon.cpp:274-284) fires
	// Eluna::SummonedCreatureDespawn on the summoner creature when a timed
	// summon expires. Player owners have no creature hook; a creature
	// owner (the script-summon bridge path) gets event 20. motionMu is
	// released above — fireCreatureSummoned requires that.
	if ownerMotion := s.findCreatureMotion(mapID, instanceID, ownerGUID); ownerMotion != nil {
		s.fireCreatureSummoned(ctx, ownerMotion, scripting.CreatureEventOnSummonedCreatureDespawn, motion)
	}
}

// updatePetMotion is the per-tick AI driver for active player pets.
func (s *Server) updatePetMotion(ctx context.Context, motion *creatureMotion, players []playerPos, now time.Time) {
	if s == nil || motion == nil || motion.OwnerGUID == 0 {
		return
	}
	// TempSummon::Update timed arms (TemporarySummon.cpp:66-170): the
	// guardian despawn sweep. A fired timer ends the motion's tick.
	if s.checkTempSummonExpiry(ctx, motion, now) {
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
		if motion.InCombat {
			motion.InCombat = false
			// Eluna CREATURE_EVENT_ON_LEAVE_COMBAT (2) + On_Reset (23):
			// an ownerless pet stops attacking (PetAI owner-absent arm).
			// Tick path: motionMu is not held here.
			s.firePetLeaveCombat(ctx, motion)
		}
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
			// Eluna CREATURE_EVENT_ON_ENTER_COMBAT (1): an aggressive
			// pet's auto-aggro engage (PetAI aggressive reaction arm).
			// Tick path: motionMu is not held here.
			s.firePetEnterCombat(ctx, motion, hostileGUID)
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
			speed := creatureSplineVelocity(motion, false)
			if speed < 0.001 {
				// MoveSplineInit::Launch -> MoveSplineInitArgs::Validate
				// (MoveSpline.cpp:210): velocity below 0.001 fails validation —
				// no spline is launched and no SMSG_MONSTER_MOVE is sent. A
				// zero-speed pet (e.g. 100%-slowed run speed) stands still
				// instead of sprinting to its owner at base run speed — the old
				// `speed = 7.0` fallback fabricated a leg C++ would reject.
				// FollowMovementGenerator::Update retries each tick, so this is
				// a per-tick stand-still, not a stuck state.
				motion.Moving = false
			} else {
				motion.Orientation = ownerOrient
				duration := splineDurationMs(float64(dist), speed)
				s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, false, ownerOrient, true)
				motion.X = destX
				motion.Y = destY
				motion.Z = destZ
				motion.Moving = true
				motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
			}
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

// consumeKillCommandStack ports spell_hun_kill_command_pet::HandleProc
// (spell_hunter.cpp:677-690): a Kill Command-empowered pet attack consumes
// one 58914 stack and one of the owner's 34027 stacks (PreventDefaultAction
// skips the default charge drop). The hunter-side 34027 aura is unmodeled
// in Go, so only the pet-side stack is tracked.
func consumeKillCommandStack(motion *creatureMotion) {
	if motion != nil && motion.KillCommandStacks > 0 {
		motion.KillCommandStacks--
	}
}

// petSelectNextTarget ports PetAI::SelectNextTarget (PetAI.cpp:253-283):
// after the pet's victim dies, the pet retargets — its own attacker first
// (so it doesn't drag a bunch of targets to the owner), then the owner's
// attacker, then the owner's victim. Passive pets do no next-target
// selection, and attackers under breakable-by-damage crowd control are
// skipped (PetAI.cpp:258-260, 266-268). The aggressive auto-select tail
// has no bridge — Go pets never auto-acquire (documented in the aggro
// audit) — and the owner's victim leg folds into the owner-attacker scan:
// Go tracks no separate owner-victim, a creature the owner is fighting
// carries the owner as its target/threat victim in this model.
func (s *Server) petSelectNextTarget(motion *creatureMotion, ownerGUID uint64) uint64 {
	if s == nil || motion == nil || motion.PetReact == PetReactPassive {
		return 0
	}
	var petAttackers, ownerAttackers []uint64
	s.motionMu.Lock()
	for guid, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil || guid == motion.GUID || m.Health == 0 || !m.InCombat {
			continue
		}
		if m.TargetGUID == motion.GUID {
			petAttackers = append(petAttackers, guid)
		} else if ownerGUID != 0 && m.TargetGUID == ownerGUID {
			ownerAttackers = append(ownerAttackers, guid)
		}
	}
	s.motionMu.Unlock()
	for _, candidates := range [][]uint64{petAttackers, ownerAttackers} {
		for _, guid := range candidates {
			if !s.petVictimHasBreakableCC(motion, guid, false, nil) {
				return guid
			}
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
	var victimReach float32 = 1.5
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
		if pSess.player.CombatReach > 0 {
			victimReach = pSess.player.CombatReach
		}
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
			if cMotion.CombatReach > 0 {
				victimReach = cMotion.CombatReach
			}
		}
		s.motionMu.Unlock()
	}

	// If target is missing or dead, disengage
	// The ghost leg mirrors WorldObject::IsValidAttackTarget's "can't attack
	// dead" (Object.cpp:2935-2937): ghosts carry Health=1 in Go (death.go:473)
	// but IsAlive() is false in C++ (deathState CORPSE).
	if !targetFound || targetHealth == 0 || (isTargetPlayer && targetSess != nil && targetSess.isDeadOrGhost()) {
		// PetAI::KilledUnit (PetAI.cpp:232-248): when the victim DIED (not
		// merely lost), the pet AttackStops and tries SelectNextTarget
		// before returning — its own attacker first, then the owner's
		// attacker, then the owner's victim — instead of disengaging.
		if targetFound && targetHealth == 0 && owner != nil {
			if next := s.petSelectNextTarget(motion, owner.GUID); next != 0 {
				stopPkt := buildAttackStop(motion.GUID, targetGUID, false)
				s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
				motion.TargetGUID = next
				motion.InCombat = true
				motion.Moving = true
				startPkt := buildAttackStart(motion.GUID, next)
				s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, nil)
				return
			}
		}
		motion.TargetGUID = 0
		leftCombat := motion.InCombat
		motion.InCombat = false
		motion.Moving = false
		if leftCombat {
			// Eluna CREATURE_EVENT_ON_LEAVE_COMBAT (2) + On_Reset (23):
			// PetAI::KilledUnit victim-lost arm with no next target.
			// Tick path: motionMu is not held here.
			s.firePetLeaveCombat(ctx, motion)
		}
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
			leftCombat := motion.InCombat
			motion.InCombat = false
			motion.Moving = false
			if leftCombat {
				// Eluna CREATURE_EVENT_ON_LEAVE_COMBAT (2) + On_Reset
				// (23): the out-of-range StopAttack drops combat.
				// Tick path: motionMu is not held here.
				s.firePetLeaveCombat(ctx, motion)
			}
			if motion.PetCommand == PetCommandAttack {
				motion.PetCommand = PetCommandFollow
			}
			stopPkt := buildAttackStop(motion.GUID, targetGUID, false)
			s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
			return
		}
	}

	dist := float32(distance3D(motion.X, motion.Y, motion.Z, targetX, targetY, targetZ))
	// UnitAI::DoMeleeAttackIfReady (UnitAI.cpp:61-83) swings only inside
	// Unit::IsWithinMeleeRange (Unit.cpp:599-618): both combat reaches +
	// 4/3, floored at NOMINAL_MELEE_RANGE (5.0), in 3D — the same contact
	// test the wild-creature tick uses (calcMeleeRange). The old hardcoded
	// 3.5-yard 2D band made pets chase closer than C++ before swinging.
	meleeRange := float32(calcMeleeRange(motion.CombatReach, victimReach))

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
		destX := targetX - 1.5*float32(math.Cos(float64(angle)))
		destY := targetY - 1.5*float32(math.Sin(float64(angle)))
		destZ := targetZ
		speed := motion.RunSpeed
		if speed < 0.001 {
			// ChaseMovementGenerator launches with no explicit velocity
			// (ChaseMovementGenerator.cpp:230-234), so the owner's speed feeds
			// MoveSplineInitArgs::Validate (MoveSpline.cpp:210) — below 0.001
			// no spline launches and no SMSG_MONSTER_MOVE is sent. A zero-speed
			// pet stands still instead of sprinting at base run speed.
			motion.Moving = false
		} else {
			motion.Orientation = angle
			duration := uint32(float64(dist) / float64(speed) * 1000.0)
			if duration < 200 {
				duration = 200
			}
			motion.X = destX
			motion.Y = destY
			motion.Z = destZ
			motion.Moving = true
			s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, false, 0, false)
		}
	} else {
		motion.Moving = false

		// Melee attack (suppressed by the breakable-CC arm above).
		// Unit::ApplyAttackTimePercentMod (Unit.cpp:10847-10864): the
		// pet's swing interval is UNIT_FIELD_BASEATTACKTIME scaled by the
		// live attack-time auras (9/138/192/193/252) — creatureAttackTimeFactor
		// is the field twin. The damage-field speed legs
		// (executePetMeleeAttack, petPhysicalDamageRange) stay on the
		// pristine base: C++ GetAttackTime divides the scaled field by
		// m_modAttackSpeedPct, which the same call scales identically.
		attackTime := time.Duration(float64(motion.AttackTime)*s.creatureAttackTimeFactor(creatureAuraKeyForMotion(motion))) * time.Millisecond
		if attackTime <= 0 {
			attackTime = 2 * time.Second
		}
		if !ccHoldAttacks {
			if motion.LastAttack.IsZero() || now.Sub(motion.LastAttack) >= attackTime {
				s.executePetMeleeAttack(ctx, motion, owner.Sess, targetGUID, isTargetPlayer, targetSess, targetHealth, targetArmor, targetLevel, now)
				motion.LastAttack = now
			}
		}
	}
	// PetAI::UpdateAI autocast (PetAI.cpp:96-196): the leg runs every tick —
	// not gated on melee range (ranged pet spells fire while chasing; the
	// per-spell range check lives in petCanAutoCastTarget) and with no fixed
	// throttle (gated on the pet GCD + per-spell cooldown inside
	// executePetAutocast, PetAI.cpp:124-131). Suppressed by the
	// breakable-CC arm above, like the swing.
	if !ccHoldAttacks && len(motion.AutocastSpells) > 0 {
		s.executePetAutocast(ctx, motion, targetGUID, now)
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

	// Guardian::UpdateAttackPowerAndDamage (StatSystem.cpp:1386-1477) +
	// Guardian::UpdateDamagePhysical (StatSystem.cpp:1505-1510): the pet's
	// total attack power adds AP/14 * attackSpeed to the BASE_ATTACK swing
	// ahead of the done-side folds — C++ folds it into the weapon damage the
	// same CalculateMeleeDamage call then runs MeleeDamageBonusDone over.
	// The happiness leg multiplies the whole swing (weapon range + AP term)
	// for hunter pets (StatSystem.cpp:1521-1534), so the AP term rides the
	// pet's live happiness mod; other pets swing unscaled.
	if total, _, _, _ := s.petGuardianAttackPower(owner, motion); total != 0 {
		speed := float64(motion.AttackTime) / 1000
		if speed <= 0 {
			speed = 2
		}
		happy := 1.0
		if motion.PetType == petTypeHunter {
			happy = float64(petHappinessDamageMod(motion.Powers[4]))
		}
		damage = uint32(float64(damage) + total/14*speed*happy)
	}

	// Unit::CalculateMeleeDamage (Unit.cpp:1236-1237): MeleeDamageBonusTaken
	// runs on the victim ahead of the armor reduction for every attacker
	// type — the pet path never bridged the victim-side flat (14 + 125),
	// percent (87 + 126) or Cheat Death arms (the player twin rides
	// meleeWeaponDamageTaken). The Sanctified Wrath 269 clawback reads the
	// attacker's auras; the pet's auras live in the creature aura store
	// (applyAuraToTarget's creature-target branch), keyed by the motion.
	petKey := creatureAuraKeyForMotion(motion)
	// Unit::MeleeDamageBonusDone (Unit.cpp:7996): the done-side flat
	// NORMAL-school SPELL_AURA_MOD_DAMAGE_DONE (13) arm and the percent
	// SPELL_AURA_MOD_DAMAGE_PERCENT_DONE (79) arm (Unit.cpp:9334-9360) run
	// before the taken leg — same order and shape as the wild-creature
	// attacker path.
	if damage > 0 {
		if flat := s.creatureAuraDamageDoneFlat(petKey); flat != 0 {
			if dmg := int32(damage) + flat; dmg <= 0 {
				damage = 0
			} else {
				damage = uint32(dmg)
			}
		}
		if pct := s.creatureAuraDamageDonePct(petKey); pct != 1.0 {
			damage = uint32(float64(damage) * pct)
		}
	}
	if damage > 0 {
		ignoreResist := creatureAuraModifiersByMiscMask(s, creatureAuraKeyForMotion(motion), spellAuraModIgnoreTargetResist, spellSchoolMaskNormal)
		damage, _ = s.applyMeleeDamageTaken(damage, spellSchoolMaskNormal, ignoreResist, targetGUID, combatTarget{
			GUID: targetGUID, Map: motion.Map, InstanceID: motion.InstanceID,
		}, false, true)
	}

	if targetArmor > 0 {
		armor := float64(targetArmor)
		if !isTargetPlayer {
			if vm := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); vm != nil {
				armor = s.liveCreatureArmor(creatureAuraKey{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID}, vm, float64(targetArmor))
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
	// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the victim-side
	// crit-taken arms — victimFlatCritTakenBP is the pre-resilience
	// SPELL_AURA_MOD_ATTACKER_MELEE_CRIT_CHANCE (187) total plus the
	// caster-matched SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER (308) total;
	// victimSpellWeaponCritBP is the post-resilience
	// SPELL_AURA_MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE (197) total.
	// The pet attacker can apply resilience (Unit::CanApplyResilience,
	// Unit.cpp:12333: owner is a player), so critReductionBP reduces the
	// 500 + flat resilience base for player victims (and for pet victims
	// through the owner's rating, per ApplyResilience's owner lookup).
	var critReductionBP, victimFlatCritTakenBP, victimSpellWeaponCritBP int32
	// Unit::GetUnitCriticalChanceDone's creature arm (Unit.cpp:2836-2844):
	// 5% base + SPELL_AURA_MOD_WEAPON_CRIT_PERCENT (52) +
	// SPELL_AURA_MOD_CRIT_PCT (290) from the pet's creature aura store,
	// gated on !CREATURE_FLAG_EXTRA_NO_CRIT (0x20000) — the flag zeroes the
	// whole base, so the adjustment is -500 plus the aura totals.
	attackerCritAdjustBP := s.creatureAuraTypeSum(petKey, spellAuraModWeaponCritPercent)*100 +
		s.creatureAuraTypeSum(petKey, spellAuraModCritPct)*100
	if motion.FlagsExtra&0x00020000 != 0 {
		attackerCritAdjustBP -= 500
	}
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
		// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2861-2875): the 187
		// total and the caster-matched 308 total join the resilience base
		// (critDone + flats, Unit.cpp:2871); 197 applies after resilience.
		victimFlatCritTakenBP = int32(math.Round(float64(targetSess.playerAuraModifier(spellAuraModAttackerMeleeCritChance))*100)) +
			int32(math.Round(targetSess.victimCritChanceForCaster(motion.GUID)*100))
		resilBaseBP := int32(500) + victimFlatCritTakenBP
		targetSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenMelee, &resilBaseBP)
		critReductionBP = int32(500) + victimFlatCritTakenBP - resilBaseBP
		victimSpellWeaponCritBP = int32(math.Round(float64(targetSess.playerAuraModifier(spellAuraModAttackerSpellAndWeaponCritChance)) * 100))
		if attackerInFront := hasInArc(targetSess.player.Orientation, targetSess.player.X, targetSess.player.Y, motion.X, motion.Y, math.Pi); !attackerInFront && !targetSess.hasAuraType(spellAuraIgnoreHitDirection) {
			canParry = false
			canBlock = false
			canDodge = false
		}
		if targetSess.genericCastInProgress() || victimIsControlled(s, true, targetSess, combatTarget{}) {
			// Unit::RollMeleeOutcomeAgainst's casting/cc'd victim arm
			// (Unit.cpp:2238-2242): a victim mid cast-bar or controlled
			// (confused/stunned/fleeing — the 5/7/12 aura twin) cannot
			// parry/block/dodge.
			canParry = false
			canBlock = false
			canDodge = false
		}
	}
	// Unit::RollMeleeOutcomeAgainst's casting/cc'd victim arm
	// (Unit.cpp:2238-2242) for creature victims: the cast half bridges via
	// motion.CastingUntil, the CONTROLLED half via the 5/7/12 aura twin
	// (victimIsControlled).
	if !isTargetPlayer {
		if vm := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); vm != nil && (time.Now().Before(vm.CastingUntil) || victimIsControlled(s, false, nil, combatTarget{Map: motion.Map, InstanceID: motion.InstanceID, GUID: targetGUID})) {
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
			// Unit::GetUnitCriticalChanceTaken (Unit.cpp:2858-2880): the
			// victim's 187 total and caster-matched 308 total are
			// pre-resilience; ApplyResilience no-ops for a creature victim
			// without a player owner — the pet-victim case (owner lookup,
			// Unit.cpp:12346) is handled below once the owner is known;
			// 197 applies after resilience.
			victimFlatCritTakenBP = creatureAuraModifierSum(s, victimKey, spellAuraModAttackerMeleeCritChance)*100 +
				int32(math.Round(creatureVictimCritChanceForCaster(s, victimKey, motion.GUID)*100))
			victimSpellWeaponCritBP = creatureAuraModifierSum(s, victimKey, spellAuraModAttackerSpellAndWeaponCritChance) * 100
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
			// Unit::ApplyResilience's owner lookup (Unit.cpp:12342-12351):
			// a player-owned pet victim takes crit-chance reduction from the
			// owner's resilience rating, against an attacker that can apply
			// it (the pet's owner is a player, Unit.cpp:12333). A non-player
			// vehicle victim takes no resilience (Unit.cpp:12342-12343).
			if isPetVictim && vm.OwnerGUID != 0 && s.creatureVictimTakesResilience(vm) {
				if ownerSess := s.findSessionByGUID(vm.OwnerGUID); ownerSess != nil {
					resilBaseBP := int32(500) + victimFlatCritTakenBP
					ownerSess.applyResilienceToMeleeCritChance(true, CombatRatingCritTakenMelee, &resilBaseBP)
					critReductionBP = int32(500) + victimFlatCritTakenBP - resilBaseBP
				}
			}
		}
	}
	// Unit::RollMeleeOutcomeAgainst's sitting-victim arm (Unit.cpp:2246-2247):
	// a non-standing player victim always takes a crit when the attacker
	// has nonzero crit chance.
	victimSitting := isTargetPlayer && targetSess != nil && targetSess.player != nil && targetSess.player.StandState != 0
	attackerExpertiseBP := s.creatureExpertiseReductionBP(petKey)
	outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, true, isTargetPlayer, isPetVictim, false, false, victimSitting, false, canBlock, canParry, canDodge, critReductionBP, 0, 0, attackerExpertiseBP, victimDodgeBP, s.creatureDodgeReductionBP(petKey)+s.creatureEnemyDodgeBP(petKey), attackerExpertiseBP, victimParryBonusBP, victimBlockBonusBP, victimParryBP, victimBlockBP, victimDodgeBonusBP, victimFlatCritTakenBP, victimSpellWeaponCritBP, attackerCritAdjustBP)
	if !isTargetPlayer && s.isCreatureEvadingInInstance(motion.Map, motion.InstanceID, targetGUID) {
		// Unit::RollMeleeOutcomeAgainst (Unit.cpp:2191-2192): a creature
		// victim in evade mode evades all melee swings.
		outcome = protocol.MeleeHitEvade
		hitInfo = protocol.HitInfoMiss
		targetState = protocol.VictimStateEvades
	}
	// Pet attackers read the dodge/parry-reduction arms from their creature
	// aura store (Unit::GetUnitDodgeChance / Unit::GetUnitParryChance,
	// Unit.cpp:2694-2703/2756-2759): the SPELL_AURA_MOD_COMBAT_RESULT_CHANCE
	// (248, VICTIMSTATE_DODGE) + SPELL_AURA_MOD_ENEMY_DODGE (251) totals ride
	// the dodge leg, and the SPELL_AURA_MOD_EXPERTISE (240)/4.0 total rides
	// both the dodge and parry legs. The victim-side crit-taken arms
	// (187/308 pre-resilience, 197 post-resilience, plus resilience itself —
	// the pet's owner is a player, Unit::CanApplyResilience) ride the roll
	// above.
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
	// its next attack is hastened — the 20%/60% remaining bands come from
	// the pristine swing (GetAttackTime divides the scaled field by
	// m_modAttackSpeedPct, which ApplyAttackTimePercentMod scales
	// identically), while the remaining time is the live aura-scaled timer
	// (GetAttackTime*m). So bands stay pristine; only the remaining uses
	// the victim's hasted swing.
	if targetState == protocol.VictimStateParry {
		if isTargetPlayer && targetSess != nil && targetSess.player != nil {
			vMainSpeed := time.Duration(targetSess.player.AttackTime) * time.Millisecond
			if vMainSpeed <= 0 {
				vMainSpeed = 2 * time.Second
			}
			vSwing := targetSess.getHastedMeleeSpeed(vMainSpeed, false)
			elapsed := now.Sub(targetSess.lastSwing)
			if elapsed < vSwing {
				rem := vSwing - elapsed
				hasted := calcParryHastedRemaining(rem, vMainSpeed)
				targetSess.lastSwing = now.Add(-(vSwing - hasted))
			}
		} else if !isTargetPlayer {
			s.motionMu.Lock()
			if vm := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, targetGUID); vm != nil {
				if vm.FlagsExtra&creatureFlagExtraNoParryHasten == 0 {
					cSpeed := time.Duration(vm.AttackTime) * time.Millisecond
					if cSpeed <= 0 {
						cSpeed = 2 * time.Second
					}
					cSwing := time.Duration(float64(cSpeed) * s.creatureAttackTimeFactor(creatureAuraKeyForMotion(vm)))
					elapsed := now.Sub(vm.LastAttack)
					if elapsed < cSwing {
						rem := cSwing - elapsed
						hasted := calcParryHastedRemaining(rem, cSpeed)
						vm.LastAttack = now.Add(-(cSwing - hasted))
					}
				}
			}
			s.motionMu.Unlock()
		}
	}

	asuPkt := protocol.BuildAttackerStateUpdate(motion.GUID, targetGUID, damage, overkill, hitInfo, targetState, blocked)
	if isTargetPlayer && targetSess != nil {
		// Unit::SendAttackStateUpdate: SendMessageToSet on the attacker — the
		// pet's visibility set; the pet has no session leg.
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, nil)
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
		// Unit::SendAttackStateUpdate: the pet attacker's visibility set, not
		// the whole instance.
		s.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, nil)
		var killedTarget combatTarget
		var killed bool
		var killX, killY, killZ float32
		s.motionMu.Lock()
		cMotion := s.findCreatureMotionLocked(motion.Map, motion.InstanceID, targetGUID)
		if cMotion != nil {
			if cMotion.BossAI == nil {
				cMotion.BossAI = getBossAIForCreature(cMotion, cMotion.ScriptName)
			}
			// Unit::DealDamage (Unit.cpp:701-704): pet damage fires the
			// victim's DamageTaken like any attacker; the pet motion
			// carries no BossAI, so the DamageDealt leg is a documented
			// no-fire (C++ PetAI::DamageDealt is a base no-op).
			fireDealDamageAIHooks(ctx, s, cMotion, motion, motion.GUID, targetGUID, &damage)
			// Unit::DealDamage tap block (Unit.cpp:872-876): pet damage
			// taps for the owner (Creature::SetLootRecipient's
			// GetCharmerOrOwnerPlayerOrPlayerItself).
			var tapPlayer, tapGroup uint64
			if owner != nil && owner.player != nil {
				tapPlayer, tapGroup = owner.playerGUID, owner.groupID
			}
			newlyTapped := s.recordCreatureTap(cMotion, tapPlayer, tapGroup, damage, cMotion.Health, true)
			// Unit::DealDamage (Unit.cpp:905-906): the victim's threat
			// manager adds threat for the attacker — the pet, not its
			// owner. The add runs even on the killing blow (C++ order);
			// the death branch below clears the table right after.
			if cMotion.ThreatMgr == nil {
				cMotion.ThreatMgr = NewThreatManager(cMotion)
			}
			cMotion.ThreatMgr.AddThreat(motion.GUID, float32(damage), true)
			consumeKillCommandStack(motion)
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
				// path skips onCreatureKilled, so the death hooks must fire
				// here — Lua 23+4 first (Eluna parity, kill.go:772), then
				// the native hook. Lua-scripted bosses keep their
				// summon cleanup on the event-4 fire (e.g. vancleef's
				// blackguards) even when the pet owner is gone mid-tick.
				// luaPlayer() is nil-safe for the gone owner; the killer
				// arg is informational on this path.
				if owner != nil {
					owner.fireCreatureDied(ctx, victim)
				}
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

// guardianBonusSpellDamage mirrors Guardian::GetBonusDamage as maintained by
// Guardian::UpdateAttackPowerAndDamage (StatSystem.cpp:1425-1457): the flat
// spell bonus a guardian pet folds into the advertised benefit in
// Unit::SpellDamageBonusDone (Unit.cpp:6681-6683) and
// Unit::SpellHealingBonusDone (Unit.cpp:7558-7561). The session is the
// guardian's owner player; C++ keeps only the finished int32, so the helper
// returns the truncated value exactly as SetBonusDamage stores it. The
// hunter-pet Wild Hunt talent mod (StatSystem.cpp:1406-1418) reads the pet's
// spellbook via petWildHuntTalent below.
// petWildHuntMod returns the hunter-pet Wild Hunt talent multiplier from
// Guardian::UpdateAttackPowerAndDamage (StatSystem.cpp:1406-1418): 1.0 plus
// CalculatePct(1.0, Effects[1].CalcValue()) when the pet's spellbook carries
// 62758 (rank 1, checked first per C++) or 62762 (rank 2) — shared by the
// SetBonusDamage spell arm and the bonusAP attack-power arm.
func (s *session) petWildHuntMod(motion *creatureMotion) float32 {
	mod := float32(1)
	if motion == nil {
		return mod
	}
	if talent := s.petWildHuntTalent(s.petNumberForGUID(motion.GUID)); talent != 0 &&
		s.server != nil && s.server.Data != nil {
		if tspell, found, terr := s.server.Data.Spell(talent); terr == nil && found && len(tspell.Effects) > 1 {
			mod += float32(tspell.Effects[1].CalcValue()) / 100
		}
	}
	return mod
}

// warlockDemonSpellPowerMax mirrors the owner-spell-power read shared by the
// warlock-demon arms of Guardian::UpdateAttackPowerAndDamage
// (StatSystem.cpp:1442-1449): max(owner fire, owner shadow damage-done bonus),
// floored at 0 — the SetBonusDamage arm takes 0.15 of it, the bonusAP arm
// 0.57 of it.
func (s *session) warlockDemonSpellPowerMax() int32 {
	if s == nil || s.player == nil {
		return 0
	}
	fire := s.player.SpellDamagePositive[2] - s.player.SpellDamageNegative[2]
	shadow := s.player.SpellDamagePositive[5] - s.player.SpellDamageNegative[5]
	maximum := fire
	if shadow > maximum {
		maximum = shadow
	}
	if maximum < 0 {
		maximum = 0
	}
	return maximum
}

// petWildHuntTalent returns the Wild Hunt talent spell the pet's spellbook
// carries — 62758 (rank 1, checked first per C++) or 62762 (rank 2) — or 0.
// StatSystem.cpp:1408-1412 reads ToPet()->m_spells; Go's pet_spell table is
// the spellbook twin (learnPetTalent inserts talent spells keyed by pet
// number, resetPetTalentsAtLogin removes them).
func (s *session) petWildHuntTalent(petNumber uint32) uint32 {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || petNumber == 0 {
		return 0
	}
	for _, id := range []uint32{62758, 62762} {
		var spell uint32
		if err := s.server.CharactersStore.DB.QueryRowContext(context.Background(), "SELECT spell FROM pet_spell WHERE guid = ? AND spell = ?", petNumber, id).Scan(&spell); err == nil && spell != 0 {
			return spell
		}
	}
	return 0
}

func (s *session) guardianBonusSpellDamage(motion *creatureMotion) int32 {
	if s == nil || s.player == nil || motion == nil {
		return 0
	}
	switch {
	case motion.PetType == petTypeHunter:
		// IsHunterPet (StatSystem.cpp:1425): owner ranged AP * 0.1287 * mod,
		// mod from petWildHuntMod (the Wild Hunt talent,
		// StatSystem.cpp:1408-1415). The stamina (StatSystem.cpp:1237-1241)
		// arm has no Go model; the bonusAP (0.22) arm now rides
		// petGuardianAttackPower on the melee path.
		return int32(s.totalAttackPowerValue(true) * 0.1287 * s.petWildHuntMod(motion))
	case motion.Entry == 26125 || motion.Entry == 30230:
		// IsPetGhoul / IsRisenAlly (TemporarySummon.h:81-82,
		// StatSystem.cpp:1428-1430): owner base AP * 0.1287.
		return int32(s.totalAttackPowerValue(false) * 0.1287)
	case motion.Entry == 29264:
		// IsSpiritWolf (TemporarySummon.h:85, StatSystem.cpp:1432-1439):
		// owner base AP * 0.31, 0.61 when the owner carries the Glyph of
		// Feral Spirit (StatSystem.cpp:1434-1435 checks
		// GetAuraEffect(63271, 0)). Go applies glyph auras into the owner's
		// live aura set at login (player_auras.go), so s.hasAura is the
		// exact predicate.
		mult := float32(0.31)
		if s.hasAura(63271) {
			mult = 0.61
		}
		return int32(s.totalAttackPowerValue(false) * mult)
	case motion.PetType == petTypeSummon && s.player.Class == 9:
		// IsPet warlock demon (StatSystem.cpp:1442-1449):
		// warlockDemonSpellPowerMax() * 0.15.
		return int32(float32(s.warlockDemonSpellPowerMax()) * 0.15)
	case motion.Entry == 510:
		// ENTRY_WATER_ELEMENTAL (StatSystem.cpp:1185, 1452-1457): owner
		// frost damage-done bonus * 0.4, floored at 0.
		frost := s.player.SpellDamagePositive[4] - s.player.SpellDamageNegative[4]
		if frost < 0 {
			frost = 0
		}
		return int32(float32(frost) * 0.4)
	}
	return 0
}

// petGuardianAttackPower mirrors Guardian::UpdateAttackPowerAndDamage's AP
// composition (StatSystem.cpp:1386-1477) for the pet's BASE_ATTACK path:
// BASE_VALUE = val + bonusAP with val = 2*GetStat(STAT_STRENGTH)-20
// (imp: GetStat(STAT_STRENGTH)-10, StatSystem.cpp:1397-1400), then the
// unit-mod fold (flatBASE * pctBASE + flatTOTAL) * pctTOTAL — BASE_PCT has no
// guardian feeder (1.0), flatTOTAL is the aura-99 fold, pctTOTAL the aura-166
// fold. owner is the pet's owner session; a nil owner (offline) keeps the val
// and aura arms — C++ computes val unconditionally and only the bonusAP arms
// need the owner. The water elemental (510) takes no bonusAP arm in C++
// (spell bonus only), so it lands in the val-only default.
func (s *Server) petGuardianAttackPower(owner *session, motion *creatureMotion) (total, base float64, flat int32, mult float64) {
	if motion == nil {
		return 0, 0, 0, 0
	}
	// GetStat(STAT_STRENGTH) is the aura-fed stat; the strength owner-bonus
	// arm is commented out in C++ (StatSystem.cpp:1228), so petGuardianStat
	// is the whole value.
	str := s.petGuardianStat(creatureAuraKeyForMotion(motion), motion, 0)
	val := 2*str - 20
	if motion.Entry == 416 { // ENTRY_IMP (StatSystem.cpp:1180)
		val = str - 10
	}
	var bonusAP float64
	if owner != nil {
		switch {
		case motion.PetType == petTypeHunter:
			// StatSystem.cpp:1405-1424: owner ranged AP * 0.22 * Wild Hunt
			// mod; Animal Handler (34453) EFFECT_1 pcts both bonusAP and
			// val (AddPct on each).
			bonusAP = float64(owner.totalAttackPowerValue(true)) * 0.22 * float64(owner.petWildHuntMod(motion))
			if amt, ok := owner.auraEffectAmount(34453, 1); ok {
				bonusAP *= 1 + float64(amt)/100
				val *= 1 + float64(amt)/100
			}
		case motion.Entry == 26125 || motion.Entry == 30230:
			// IsPetGhoul / IsRisenAlly (StatSystem.cpp:1428-1430): owner
			// base AP * 0.22.
			bonusAP = float64(owner.totalAttackPowerValue(false)) * 0.22
		case motion.Entry == 29264:
			// IsSpiritWolf (StatSystem.cpp:1432-1439): owner base AP * 0.31,
			// 0.61 with the owner's Glyph of Feral Spirit (63271).
			m := 0.31
			if owner.hasAura(63271) {
				m = 0.61
			}
			bonusAP = float64(owner.totalAttackPowerValue(false)) * m
		case motion.PetType == petTypeSummon && owner.player != nil && owner.player.Class == 9:
			// IsPet warlock demon (StatSystem.cpp:1442-1449):
			// max(owner fire, owner shadow) * 0.57.
			bonusAP = float64(owner.warlockDemonSpellPowerMax()) * 0.57
		}
	}
	base = val + bonusAP
	key := creatureAuraKeyForMotion(motion)
	flat = s.creatureAuraAttackPowerFlat(key)
	pct := s.creatureAuraAttackPowerPct(key)
	mult = pct - 1
	total = (base + float64(flat)) * pct
	return total, base, flat, mult
}

// petPhysicalDamageRange mirrors Guardian::UpdateDamagePhysical
// (StatSystem.cpp:1479-1554), which rewrites UNIT_FIELD_MINDAMAGE/MAXDAMAGE
// on the pet: mindamage = ((flat(BASE of UNIT_MOD_DAMAGE_MAINHAND) +
// AP/14*att_speed + bonusDamage + weapon_mindamage) * base_pct + total_value)
// * total_pct, then the hunter-pet happiness arm and the 61682/61683
// MOD_ATTACKSPEED reduction. For guardians flat(BASE) and base_pct have no
// feeders (only TOTAL_VALUE via UpdateDamageDoneMods and TOTAL_PCT via
// UpdateDamagePctDoneMods write the mod), so the composition is
// ((AP/14*speed + bonusDamage + weaponRange) + flat13) * pct79. bonusDamage
// is the treant 0.09 x owner nature bonus (entry 1964) and the fire
// elemental 0.4 x owner fire bonus (entry 15438), positive-only, owner a
// player. The AP term reuses petGuardianAttackPower's total (the
// GetTotalAttackPowerValue twin), so strength/99/166 auras feed the fields
// through the UpdateAttackPowerAndDamage tail like C++. owner is the pet's
// owner session; it may be nil (offline owner), in which case the owner
// bonus arms read 0 — the weapon/AP/aura legs are owner-independent.
func (s *Server) petPhysicalDamageRange(owner *session, motion *creatureMotion) (mindmg, maxdmg float64) {
	if motion == nil {
		return 0, 0
	}
	total, _, _, _ := s.petGuardianAttackPower(owner, motion)
	speed := float64(motion.AttackTime) / 1000
	if speed <= 0 {
		speed = 2
	}
	base := total/14*speed + petGuardianBonusDamage(owner, motion)
	key := creatureAuraKeyForMotion(motion)
	flat := float64(s.creatureAuraDamageDoneFlat(key))
	pct := s.creatureAuraDamageDonePct(key)
	mindmg = (base + float64(motion.MinDamage) + flat) * pct
	maxdmg = (base + float64(motion.MaxDamage) + flat) * pct
	if motion.PetType == petTypeHunter {
		happy := float64(petHappinessDamageMod(motion.Powers[4]))
		mindmg *= happy
		maxdmg *= happy
	}
	debuff := s.petWeaponDamageSpeedDebuffPct(key)
	mindmg *= debuff
	maxdmg *= debuff
	return mindmg, maxdmg
}

// petGuardianBonusDamage mirrors the bonusDamage arm of
// Guardian::UpdateDamagePhysical (StatSystem.cpp:1484-1497): force of nature
// treants (entry 1964) add 9% of the owner's nature damage-done bonus and
// greater fire elementals (entry 15438) add 40% of the owner's fire bonus,
// each only when the owner is a player and the bonus is positive. Go has no
// temp-summon motions (item-9 parked), so the arms only fire if such a
// motion ever exists — they are entry-gated exactly like C++.
func petGuardianBonusDamage(owner *session, motion *creatureMotion) float64 {
	if owner == nil || owner.player == nil || motion == nil {
		return 0
	}
	var bonus int32
	switch motion.Entry {
	case 1964: // ENTRY_TREANT
		bonus = owner.player.SpellDamagePositive[3] - owner.player.SpellDamageNegative[3]
		if bonus > 0 {
			return float64(bonus) * 0.09
		}
	case 15438: // ENTRY_FIRE_ELEMENTAL
		bonus = owner.player.SpellDamagePositive[2] - owner.player.SpellDamageNegative[2]
		if bonus > 0 {
			return float64(bonus) * 0.4
		}
	}
	return 0
}

// petWeaponDamageSpeedDebuffPct mirrors the @todo-marked legacy arm inside
// Guardian::UpdateDamagePhysical (StatSystem.cpp:1536-1550): MOD_ATTACKSPEED
// auras from spells 61682/61683 reduce the pet's weapon damage fields by
// AddPct(mindamage, -amount) each. Callers must not hold auraMu.
func (s *Server) petWeaponDamageSpeedDebuffPct(key creatureAuraKey) float64 {
	if s == nil || key.GUID == 0 || s.Data == nil {
		return 1.0
	}
	factor := 1.0
	s.auraMu.Lock()
	for _, aura := range s.activeCreatureAuras[key] {
		if aura == nil || aura.Stopped {
			continue
		}
		if aura.SpellID != 61682 && aura.SpellID != 61683 {
			continue
		}
		spell, found, err := s.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range spell.Effects {
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if effect.Aura != spellAuraModAttackSpeed {
				continue
			}
			if index < len(aura.Amounts) {
				factor *= 1 - float64(aura.Amounts[index])/100
			}
		}
	}
	s.auraMu.Unlock()
	return factor
}

// petGuardianDoneDamageBonus mirrors the advertised-benefit fold of
// Unit::SpellDamageBonusDone (Unit.cpp:6679-6683, 6725-6731): the pet's own
// SPELL_AURA_MOD_DAMAGE_DONE (13) school-gated fold
// (SpellBaseDamageBonusDone, Unit.cpp:7134-7136) plus the guardian's
// GetBonusDamage(), then DoneTotal += int32(DoneAdvertisedBenefit * coeff *
// factorMod) runs per damage effect. The ATTR3_NO_DONE_BONUS gate
// (Unit.cpp:6604-6605) and the DmgClass-NONE early-out — which fires only
// when no spell_bonus_data row exists (Unit.cpp:6705-6709) — are honored.
// The victim's SPELL_AURA_MOD_DAMAGE_TAKEN (14) arm (Unit.cpp:6682) is per
// victim and folds at the petDamageFor call site. The owner's flat
// OVERRIDE_CLASS_SCRIPTS set and the pet's own MOD_FLAT_SPELL_DAMAGE_VERSUS
// (180) arm (Unit.cpp:6617-6647) ride DoneTotal flat via
// petDoneFlatClassScripts/petDoneFlatVersus at the call sites. The remaining
// DoneTotal/DoneTotalMod legs (Impurity, AP arms, pct mods) stay unbridged
// on the pet path.
func (s *session) petGuardianDoneDamageBonus(motion *creatureMotion, spell wotlk.Spell) int32 {
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
		return 0
	}
	if _, hasRow := s.spellBonusRow(spell.ID); !hasRow && spell.DefenseType == spellDamageClassNone {
		return 0
	}
	// Unit::SpellDamageBonusDone (Unit.cpp:6679-6683): the advertised benefit
	// starts at SpellBaseDamageBonusDone — for a non-player caster that is
	// the pet's own SPELL_AURA_MOD_DAMAGE_DONE (13) school-gated fold
	// (Unit.cpp:7134-7136, GetTotalAuraModifierByMiscMask excludes misc-0)
	// — then the guardian's GetBonusDamage() rides on top. The victim's
	// SPELL_AURA_MOD_DAMAGE_TAKEN (14) arm (Unit.cpp:6682) is per victim and
	// folds at the petDamageFor call site instead.
	advertised := s.guardianBonusSpellDamage(motion)
	if s != nil && s.server != nil && motion != nil {
		for _, amt := range creatureAuraModifiersByMiscMask(s.server, creatureAuraKeyForMotion(motion), spellAuraModDamageDone, uint32(spell.SchoolMask)) {
			advertised += amt
		}
	}
	if advertised == 0 {
		return 0
	}
	var total int32
	for i, effect := range spell.Effects {
		switch effect.Effect {
		case spellEffectSchoolDamage, spellEffectWeaponDamageNoschool, spellEffectWeaponPercentDamage, spellEffectWeaponDamage, spellEffectNormalizedWeaponDmg:
			if effect.CalcValueForLevel(spell, uint32(motion.Level)) <= 0 {
				continue
			}
			total += int32(float64(advertised) * s.spellDoneCoefficientCasterLevel(spell, i, false, motion.Level))
		}
	}
	return total
}

// petDamageEffectBonus folds a flat advertised-benefit arm into DoneTotal
// per damage effect — int32(arm * coeff) over the same effect set
// petGuardianDoneDamageBonus counts (Unit.cpp:6725-6727). Used for the
// per-victim SPELL_AURA_MOD_DAMAGE_TAKEN arm, which cannot ride the
// victim-independent advertised fold above.
func (s *session) petDamageEffectBonus(spell wotlk.Spell, motion *creatureMotion, arm int32) int32 {
	var total int32
	for i, effect := range spell.Effects {
		switch effect.Effect {
		case spellEffectSchoolDamage, spellEffectWeaponDamageNoschool, spellEffectWeaponPercentDamage, spellEffectWeaponDamage, spellEffectNormalizedWeaponDmg:
			if motion != nil && effect.CalcValueForLevel(spell, uint32(motion.Level)) <= 0 {
				continue
			}
			total += int32(float64(arm) * s.spellDoneCoefficientCasterLevel(spell, i, false, motion.Level))
		}
	}
	return total
}

// petDoneFlatClassScripts mirrors the done-scripted-mod arm of
// Unit::SpellDamageBonusDone (Unit.cpp:6617-6637) for a guardian pet caster:
// the flat OVERRIDE_CLASS_SCRIPTS set (4418/4554/4555/5142/5147/5148/6008/8627)
// is taken from the OWNER (Unit.cpp:6617-6618; s is the owner's session),
// added flat to DoneTotal without a coefficient leg, ahead of the pct leg.
// The ATTR3_NO_DONE_BONUS gate sits above it (Unit.cpp:6604-6605).
func (s *session) petDoneFlatClassScripts(spell wotlk.Spell) int32 {
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
		return 0
	}
	var flat int32
	for _, ae := range s.overrideClassScriptAuras(spell) {
		switch ae.misc {
		case 4418, 4554, 4555, 5142, 5147, 5148, 6008, 8627:
			flat += ae.amount
		}
	}
	return flat
}

// petDoneFlatVersus mirrors the MOD_FLAT_SPELL_DAMAGE_VERSUS arm of
// Unit::SpellDamageBonusDone (Unit.cpp:6646-6647) for a guardian pet caster:
// the pet's own aura-180 flat by the victim's creature-type mask, added flat
// to DoneTotal (no coefficient leg), skipped under
// SPELL_ATTR6_LIMIT_PCT_DAMAGE_MODS. Per victim like the aura-14 arm; the
// ATTR3_NO_DONE_BONUS gate sits above it (Unit.cpp:6604-6605).
func (s *session) petDoneFlatVersus(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, targetGUID uint64) int32 {
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
		return 0
	}
	if spell.AttributesEx6&spellAttr6LimitPctDamageMods != 0 {
		return 0
	}
	if s == nil || s.server == nil || motion == nil {
		return 0
	}
	var flat int32
	if typMask, ok := s.targetCreatureTypeMask(ctx, targetGUID); ok && typMask != 0 {
		for _, amt := range creatureAuraModifiersByMiscMask(s.server, creatureAuraKeyForMotion(motion), auraModFlatSpellDamageVersus, typMask) {
			flat += amt
		}
	}
	return flat
}

// petDamagePctDone mirrors Unit::SpellDamagePctDone (Unit.cpp:6738-7050) for a
// guardian pet caster: the pet's own SPELL_AURA_MOD_DAMAGE_PERCENT_DONE (79)
// school arm (Unit.cpp:6771-6779, non-player twin), the versus
// (Unit.cpp:6783) and versus-aurastate (Unit.cpp:6786-6791) arms on the pet's
// auras, then the owner-scoped OVERRIDE_CLASS_SCRIPTS / family table
// (Unit.cpp:6797-7050) via spellDamagePctClassScripts — C++ reads the class
// scripts from the owner (Unit.cpp:6794-6795), and s is the owner's session.
// The Shadow Bite arm (Unit.cpp:7019-7022) folds here too, after the shared
// table: C++ puts it inside DoneTotalMod, before the mod-owner SPELLMOD tail
// (Unit.cpp:6731-6734). Spells with SPELL_ATTR3_NO_DONE_BONUS or
// SPELL_ATTR6_LIMIT_PCT_DAMAGE_MODS skip the leg (Unit.cpp:6744-6749). The rank
// spell-damage mod (Unit.cpp:6765-6766) applies to non-pet creatures only;
// the family arms inside the shared table read the owner's auras where C++
// reads the pet's, but no other pet-reachable spell carries those family
// flags, so the residual is nil in practice.
func (s *session) petDamagePctDone(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, targetGUID uint64, target combatTarget) float64 {
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 || spell.AttributesEx6&spellAttr6LimitPctDamageMods != 0 {
		return 1.0
	}
	mod := 1.0
	if s != nil && s.server != nil && motion != nil {
		key := creatureAuraKeyForMotion(motion)
		if key.GUID != 0 {
			schoolMask := uint32(spell.SchoolMask)
			for _, amt := range creatureAuraModifiersByMiscMask(s.server, key, auraModDamagePercentDone, schoolMask) {
				mod *= 1 + float64(amt)/100
			}
			if typMask, ok := s.targetCreatureTypeMask(ctx, targetGUID); ok && typMask != 0 {
				for _, amt := range creatureAuraModifiersByMiscMask(s.server, key, auraModDamageDoneVersus, typMask) {
					mod *= 1 + float64(amt)/100
				}
			}
			for _, ae := range petAuraTypeMiscAmounts(s.server, key, auraModDamageDoneVersusAurastate) {
				if s.targetHasAuraState(ctx, targetGUID, uint32(ae.misc), spell) {
					mod *= 1 + float64(ae.amount)/100
				}
			}
		}
	}
	if s == nil {
		return mod
	}
	mod = s.spellDamagePctClassScripts(ctx, spell, targetGUID, target, mod)
	// Unit::SpellDamagePctDone (Unit.cpp:7019-7022): the Shadow Bite arm — a
	// pet's Warlock-family spell with SpellFamilyFlags[1] & 0x00400000 gains
	// 15% per DoT the victim carries from the pet's owner (GetDoTsByCaster
	// counts PERIODIC_DAMAGE / PERIODIC_DAMAGE_PERCENT effects by caster
	// GUID). C++ gates on IsPet(); Go pet motions carry OwnerGUID. It folds
	// inside DoneTotalMod, before the mod-owner SPELLMOD_DAMAGE tail
	// (Unit.cpp:6731-6734) — not at the damage site.
	if motion != nil && motion.OwnerGUID != 0 && spell.SpellFamilyName == spellFamilyWarlock && spell.SpellFamilyFlags[1]&0x00400000 != 0 {
		if dots := s.server.countDoTsByCaster(target, motion.OwnerGUID); dots > 0 {
			mod *= 1 + float64(dots)*0.15
		}
	}
	return mod
}

// petAuraTypeMiscAmounts is the creature-key twin of auraTypeMiscAmounts: live
// (misc, amount) pairs for every effect of the given aura type on the pet,
// for the arms that switch on misc values (MOD_DAMAGE_DONE_VERSUS_AURASTATE).
func petAuraTypeMiscAmounts(server *Server, key creatureAuraKey, auraType uint32) []classScriptAura {
	var out []classScriptAura
	if server == nil || server.Data == nil || key.GUID == 0 {
		return nil
	}
	server.auraMu.Lock()
	auras := server.activeCreatureAuras[key]
	copies := make([]*activeAura, 0, len(auras))
	for _, aura := range auras {
		copies = append(copies, aura)
	}
	server.auraMu.Unlock()
	for _, aura := range copies {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		auraSpell, found, err := server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, eff := range auraSpell.Effects {
			if index >= len(aura.Amounts) || index >= 8 || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if !spellEffectIsAuraEffect(eff) || eff.Aura != auraType {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 {
				amount = eff.BasePoints
			}
			out = append(out, classScriptAura{spellID: aura.SpellID, misc: eff.MiscValue, amount: amount})
		}
	}
	return out
}

// petHealingPctDone mirrors Unit::SpellHealingPctDone (Unit.cpp:7634-7711) for
// a guardian pet caster: the pet's own SPELL_AURA_MOD_HEALING_DONE_PERCENT
// (136) fold plus the owner-scoped OVERRIDE_CLASS_SCRIPTS table. Spells with
// SPELL_ATTR3_NO_DONE_BONUS, SPELL_ATTR6_LIMIT_PCT_HEALING_MODS, or of the
// potion family skip the leg (Unit.cpp:7642-7651). The totem redirect
// (Unit.cpp:7637-7639) has no pet twin — a guardian is never a totem. The
// leech/funnel zeroing (Unit.cpp:7607-7619) kills only the flat DoneTotal;
// the percent leg still applies (Unit.cpp:7612), so spellHealingDoneZeroed
// does not gate this fold. C++ reads the class scripts from the owner
// (Unit.cpp:7661), and s is the owner's session.
func (s *session) petHealingPctDone(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, victimGUID uint64) float64 {
	if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 || spell.AttributesEx6&spellAttr6LimitPctHealingMods != 0 ||
		spell.SpellFamilyName == spellFamilyPotion {
		return 1.0
	}
	mod := 1.0
	// Unit.cpp:7656: GetTotalAuraMultiplier applies no misc mask — every live
	// effect of the aura type folds multiplicatively.
	if s != nil && s.server != nil && s.server.Data != nil && motion != nil {
		if key := creatureAuraKeyForMotion(motion); key.GUID != 0 {
			for _, ae := range petAuraTypeMiscAmounts(s.server, key, auraModHealingDonePercent) {
				mod *= 1 + float64(ae.amount)/100
			}
		}
	}
	if s == nil {
		return mod
	}
	addPct := func(amt int32) { mod *= 1 + float64(amt)/100 } // AddPct fold
	for _, ae := range s.overrideClassScriptAuras(spell) {
		switch ae.misc {
		case 21, 6935, 6918: // Test of Faith — victim below 50% health
			if target, ok := s.getCombatTarget(ctx, victimGUID); ok && target.MaxHealth > 0 &&
				target.Health*2 < target.MaxHealth {
				addPct(ae.amount)
			}
		case 7798: // Glyph of Regrowth — victim carries Rejuvenation/Regrowth
			if petHealTargetHasAura(s, ctx, victimGUID, spellAuraPeriodicHeal, spellFamilyDruid, 0x40, 0, 0) {
				addPct(ae.amount)
			}
		case 8477: // Nourish Heal Boost — step percent per own HoT stack on the victim
			step := ae.amount
			boost := int32(0)
			if s.server != nil && s.server.Data != nil && motion != nil {
				// Unit.cpp:7678-7694: the victim auras must be cast by the
				// healer — the pet's GUID, not the owner's.
				for _, aura := range s.victimAurasForLookup(ctx, victimGUID) {
					if aura == nil || aura.Stopped || aura.CasterGUID != motion.GUID {
						continue
					}
					aspell, found, err := s.server.Data.Spell(aura.SpellID)
					if err != nil || !found || aspell.SpellFamilyName != spellFamilyDruid {
						continue
					}
					if aspell.SpellFamilyFlags[1]&0x10 == 0 && aspell.SpellFamilyFlags[0]&0x50 == 0 {
						continue
					}
					st := int32(aura.StackCount)
					if st == 0 {
						st = 1
					}
					boost += step * st
				}
			}
			addPct(boost)
		case 7871: // Glyph of Lesser Healing Wave — victim carries Earth Shield
			if petHealTargetHasAura(s, ctx, victimGUID, spellAuraDummy, spellFamilyShaman, 0, 0x400, 0) {
				addPct(ae.amount)
			}
		}
	}
	return mod
}

// petHealTargetHasAura is the victim-side twin of session.hasAuraOfType for a
// pet heal target (player session or creature): the victim's live auras are
// scanned for an effect of the given aura type whose spell is of the given
// family with at least one of the non-zero flag words matching the spell's
// SpellFamilyFlags (flag96::HasFlag, Util.h:401-404).
func petHealTargetHasAura(s *session, ctx context.Context, victimGUID uint64, auraType, familyName, flag0, flag1, flag2 uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, aura := range s.victimAurasForLookup(ctx, victimGUID) {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || auraSpell.SpellFamilyName != familyName {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if index >= len(aura.Amounts) || !spellEffectIsAuraEffect(effect) || effect.Aura != auraType ||
				aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if flag0|flag1|flag2 == 0 {
				return true
			}
			if flag0 != 0 && auraSpell.SpellFamilyFlags[0]&flag0 != 0 {
				return true
			}
			if flag1 != 0 && auraSpell.SpellFamilyFlags[1]&flag1 != 0 {
				return true
			}
			if flag2 != 0 && auraSpell.SpellFamilyFlags[2]&flag2 != 0 {
				return true
			}
		}
	}
	return false
}

// petApBonusDoneFlat mirrors the ap_bonus/ap_dot_bonus arm of
// Unit::SpellDamageBonusDone (Unit.cpp:6684-6706) and
// Unit::SpellHealingBonusDone (Unit.cpp:7568-7581) for a guardian pet
// caster: with a spell_bonus_data row, the ap_bonus/ap_dot_bonus
// coefficient scales APbonus — the victim's
// SPELL_AURA_MELEE_ATTACK_POWER_ATTACKER_BONUS (165) auras plus the pet's
// total attack power (petGuardianAttackPower, the GetTotalAttackPowerValue
// twin) — by the stack. The Impurity ApCoeffMod arm (Unit.cpp:7523-7548,
// 6649-6679) is nil for a guardian caster — C++ gates it on the caster
// being a player — so the coefficient modifier is 1.0. Pets cast no
// ranged-weapon spells, so the BASE_ATTACK attacker-bonus aura (165) is
// the live one (Unit.cpp:6687/7571).
func (s *session) petApBonusDoneFlat(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, victimGUID uint64, victim combatTarget, isDot bool, stack uint32) int32 {
	if s == nil || s.server == nil || motion == nil {
		return 0
	}
	bonus, ok := s.spellBonusRow(spell.ID)
	if !ok {
		return 0
	}
	coeff := bonus.apDirect
	if isDot {
		coeff = bonus.apDot
	}
	if coeff <= 0 {
		return 0
	}
	if stack == 0 {
		stack = 1
	}
	ap := float64(s.swingVictimAuraSum(victimGUID, victim, spellAuraMeleeAttackPowerAttackerBonus, 0, false))
	if total, _, _, _ := s.server.petGuardianAttackPower(s, motion); total != 0 {
		ap += total
	}
	return int32(coeff * float64(stack) * ap)
}

// petGuardianDoneHealingBonus mirrors the full advertised-benefit fold of
// Unit::SpellHealingBonusDone (Unit.cpp:7490-7605) for a guardian caster:
// DoneAdvertisedBenefit = SpellBaseHealingBonusDone (the pet's own
// SPELL_AURA_MOD_HEALING_DONE auras, Unit.cpp:7552/7764-7771) + the victim's
// SPELL_AURA_MOD_HEALING auras (Unit.cpp:7554) + the guardian's GetBonusDamage()
// (Unit.cpp:7558-7561), applied as int32(benefit * coeff * factorMod)
// (Unit.cpp:7599-7605). The potion-family gate (Unit.cpp:7494-7496) and the
// DmgClass-NONE early-out (Unit.cpp:7583-7587) are honored, and leech /
// health-funnel spells zero the whole DoneTotal after the fold
// (Unit.cpp:7607-7619, via spellHealingDoneZeroed). Impurity (Unit.cpp:7523-7548)
// is correctly nil for a guardian caster — C++ gates the arm on the caster
// being a player. The ap_bonus/ap_dot_bonus arms (Unit.cpp:7568-7581) ride
// petApBonusDoneFlat below — the pet's total attack power comes from
// petGuardianAttackPower (the GetTotalAttackPowerValue twin). The percent leg (SpellHealingPctDone) rides the call
// site via petHealingPctDone. The coefficient's level-penalty factor reads the
// pet's motion level (spellHealingDoneCoefficientCasterLevel), matching C++'s
// caster-level read in Unit::CalculateSpellpowerCoefficientLevelPenalty.
func (s *session) petGuardianDoneHealingBonus(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, effIndex int, victimGUID uint64, isDot bool, stack uint32) int32 {
	if s == nil || motion == nil {
		return 0
	}
	if spellHealingDoneZeroed(spell) {
		return 0
	}
	if spell.SpellFamilyName == spellFamilyPotion {
		return 0
	}
	if _, hasRow := s.spellBonusRow(spell.ID); !hasRow && spell.DefenseType == spellDamageClassNone {
		return 0
	}
	if stack == 0 {
		stack = 1
	}
	// Unit.cpp:7505-7519: the flat OVERRIDE_CLASS_SCRIPTS arms (Increased
	// Rejuvenation Healing 4415, Increased Lesser Healing Wave 4953, LK Arena
	// Savage Totem of the Third Wind 3736) come from the owner — s is the
	// owner's session. They land in DoneTotal after the advertised x
	// coefficient fold, so they survive even when the advertised benefit is 0
	// (C++ adds them before the DoneAdvertisedBenefit check).
	flat := int32(0)
	for _, ae := range s.overrideClassScriptAuras(spell) {
		switch ae.misc {
		case 4415, 4953, 3736:
			flat += ae.amount
		}
	}
	schoolMask := uint32(spell.SchoolMask)
	advertised := s.guardianBonusSpellDamage(motion)
	advertised += s.petHealingDoneAuraModifier(motion, schoolMask)
	advertised += s.healVictimModHealing(ctx, victimGUID, schoolMask)
	// Unit.cpp:7568-7581: the ap_bonus/ap_dot_bonus arm rides DoneTotal
	// flat beside the advertised fold (no coefficient leg).
	var victim combatTarget
	if ct, ok := s.getCombatTarget(ctx, victimGUID); ok {
		victim = ct
	}
	apArm := s.petApBonusDoneFlat(ctx, motion, spell, victimGUID, victim, isDot, stack)
	if advertised == 0 && flat == 0 && apArm == 0 {
		return 0
	}
	return int32(float64(advertised)*s.spellHealingDoneCoefficientCasterLevel(spell, effIndex, isDot, motion.Level)*float64(stack)) + flat + apArm
}

// petTickHealDoneLegs mirrors the per-tick Unit::SpellHealingBonusDone (DOT)
// for a pet caster on periodic-heal ticks (SpellAuraEffects.cpp:5384): the
// flat advertised-benefit fold (petGuardianDoneHealingBonus, dot
// coefficient, tick stack), the percent leg (petHealingPctDone), then the
// mod-owner's SPELLMOD_DOT tail (Unit.cpp:7614-7616). The Go tick paths
// resolve a pet caster to no session, so these legs never ran for pet HoTs
// while the player path ran them per tick. owner is the pet's owner session
// (GetSpellModOwner); a nil owner passes the legs through, matching the
// unresolved-caster convention on the tick paths. The leech/funnel zeroing
// kills only the flat leg — the pct fold runs even then (Unit.cpp:7612),
// like the cast path.
func petTickHealDoneLegs(owner *session, ctx context.Context, motion *creatureMotion, spell wotlk.Spell, effIndex int, victimGUID uint64, heal, stack uint32) uint32 {
	if motion == nil {
		return heal
	}
	if bonus := owner.petGuardianDoneHealingBonus(ctx, motion, spell, effIndex, victimGUID, true, stack); bonus != 0 {
		if adjusted := int32(heal) + bonus; adjusted > 0 {
			heal = uint32(adjusted)
		} else {
			heal = 0
		}
	}
	if pct := owner.petHealingPctDone(ctx, motion, spell, victimGUID); pct != 1.0 {
		tmp := float64(int32(heal)) * pct
		if tmp < 0 {
			tmp = 0
		}
		heal = uint32(tmp)
	}
	if out := owner.applySpellMod(spell, spellModDot, int32(heal)); out < 0 {
		heal = 0
	} else {
		heal = uint32(out)
	}
	return heal
}

// petPeriodicApplyDoneLegs mirrors the apply-time done leg in
// AuraEffect::CalculateAmount (SpellAuraEffects.cpp:505-518): for unit auras a
// PERIODIC_DAMAGE (3) / PERIODIC_LEECH (53) effect's amount is
// caster->SpellDamageBonusDone(victim, spell, amount, DOT, effIndex, donePct)
// and a PERIODIC_HEAL (8) effect's is caster->SpellHealingBonusDone(victim,
// spell, amount, DOT, effIndex, donePct), with donePct the per-target pct leg
// (Aura::SaveCasterInfo runs per aura instance, SpellAuras.cpp:521-525).
// PERIODIC_DAMAGE_PERCENT (89), OBS_MOD_HEALTH (20) and
// PERIODIC_TRIGGER_SPELL (23) carry no done leg (SpellAuraEffects.cpp:498-507).
// The pet cast path applied raw CalcValueForLevel amounts, so pet DoTs/HoTs
// carried no done leg at all — this is the apply-time twin of the per-tick
// op-22 (SPELLMOD_DOT) tail petTickHealDoneLegs folds for pet HoT ticks
// (C++ runs done legs per tick only for DYNOBJ_AURA_TYPE,
// SpellAuraEffects.cpp:5145; unit auras bake at apply).
func (s *session) petPeriodicApplyDoneLegs(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, effIndex int, targetGUID uint64, target combatTarget, amount uint32) uint32 {
	if s == nil || motion == nil || effIndex < 0 || effIndex >= len(spell.Effects) {
		return amount
	}
	_, hasBonusRow := s.spellBonusRow(spell.ID)
	dmgClassNone := spell.DefenseType == spellDamageClassNone
	// foldFlat applies the flat DoneTotal and the mod-owner SPELLMOD_DOT
	// tail; the DmgClassNONE-no-row early-out (Unit.cpp:6707-6710,
	// 7583-7587) applies only the pct leg, never the spellmod.
	foldTail := func(value uint32) uint32 {
		if out := s.applySpellMod(spell, spellModDot, int32(value)); out < 0 {
			return 0
		} else {
			return uint32(out)
		}
	}
	foldPct := func(value uint32, pct float64) uint32 {
		if pct == 1.0 {
			return value
		}
		if tmp := float64(int32(value)) * pct; tmp > 0 {
			return uint32(tmp)
		}
		return 0
	}
	switch spell.Effects[effIndex].Aura {
	case spellAuraPeriodicDamage, spellAuraPeriodicLeech:
		// Unit::SpellDamageBonusDone (Unit.cpp:6602-6734).
		if spell.AttributesEx3&spellAttr3NoDoneBonus != 0 {
			return amount
		}
		if hasBonusRow || !dmgClassNone {
			// DoneAdvertisedBenefit: the pet's own SPELL_AURA_MOD_DAMAGE_DONE
			// (13) school fold (Unit.cpp:7134-7136, misc-0 excluded) plus the
			// guardian's GetBonusDamage (Unit.cpp:6681-6683), with the
			// per-victim SPELL_AURA_MOD_DAMAGE_TAKEN (14) arm (Unit.cpp:6682).
			advertised := s.guardianBonusSpellDamage(motion)
			if s.server != nil {
				for _, amt := range creatureAuraModifiersByMiscMask(s.server, creatureAuraKeyForMotion(motion), spellAuraModDamageDone, uint32(spell.SchoolMask)) {
					advertised += amt
				}
			}
			victimTaken := s.victimDamageTakenFlat(spell, targetGUID, target)
			if advertised != 0 || victimTaken != 0 {
				coeff := s.spellDoneCoefficientCasterLevel(spell, effIndex, true, motion.Level)
				total := int32(float64(advertised+victimTaken) * coeff)
				if adjusted := int32(amount) + total; adjusted > 0 {
					amount = uint32(adjusted)
				} else {
					amount = 0
				}
			}
			// Unit::SpellDamageBonusDone (Unit.cpp:6617-6647): the flat
			// scripted arms ride DoneTotal flat (no coefficient leg) — the
			// owner's OVERRIDE_CLASS_SCRIPTS set plus the pet's own
			// MOD_FLAT_SPELL_DAMAGE_VERSUS by victim type.
			if flat := s.petDoneFlatClassScripts(spell); flat != 0 {
				if adjusted := int32(amount) + flat; adjusted > 0 {
					amount = uint32(adjusted)
				} else {
					amount = 0
				}
			}
			if v180 := s.petDoneFlatVersus(ctx, motion, spell, targetGUID); v180 != 0 {
				if adjusted := int32(amount) + v180; adjusted > 0 {
					amount = uint32(adjusted)
				} else {
					amount = 0
				}
			}
			// Unit::SpellDamageBonusDone (Unit.cpp:6684-6706): the
			// ap_dot_bonus arm rides DoneTotal flat beside the advertised
			// fold (no coefficient leg); the bake-at-apply aura carries
			// stack 1 like the advertised fold above.
			if ap := s.petApBonusDoneFlat(ctx, motion, spell, targetGUID, target, true, 1); ap != 0 {
				if adjusted := int32(amount) + ap; adjusted > 0 {
					amount = uint32(adjusted)
				} else {
					amount = 0
				}
			}
		}
		amount = foldPct(amount, s.petDamagePctDone(ctx, motion, spell, targetGUID, target))
		if hasBonusRow || !dmgClassNone {
			amount = foldTail(amount)
		}
		return amount
	case spellAuraPeriodicHeal:
		// Unit::SpellHealingBonusDone (Unit.cpp:7487-7632): potion spells
		// take no bonus at all (Unit.cpp:7494-7496); there is no
		// ATTR3_NO_DONE_BONUS gate on the heal path.
		if spell.SpellFamilyName == spellFamilyPotion {
			return amount
		}
		if hasBonusRow || !dmgClassNone {
			// The flat fold (petGuardianDoneHealingBonus, dot coefficient,
			// stack 1) is zeroed for leech/funnel spells
			// (Unit.cpp:7605-7610) — the pct leg and the SPELLMOD_DOT tail
			// still apply (Unit.cpp:7612, 7624-7626).
			if flat := s.petGuardianDoneHealingBonus(ctx, motion, spell, effIndex, targetGUID, true, 1); flat != 0 {
				if adjusted := int32(amount) + flat; adjusted > 0 {
					amount = uint32(adjusted)
				} else {
					amount = 0
				}
			}
		}
		amount = foldPct(amount, s.petHealingPctDone(ctx, motion, spell, targetGUID))
		if hasBonusRow || !dmgClassNone {
			amount = foldTail(amount)
		}
		return amount
	default:
		return amount
	}
}

// petHealingDoneAuraModifier mirrors the pet-caster leg of
// Unit::SpellBaseHealingBonusDone (Unit.cpp:7764-7771): the guardian's own
// SPELL_AURA_MOD_HEALING_DONE (135) auras fold into the advertised benefit
// with the C++-exact misc gate — an unset misc applies to every school,
// otherwise the misc mask must intersect the spell's school mask. The
// player-only spellpower/of-stat/of-AP legs never apply to a guardian.
func (s *session) petHealingDoneAuraModifier(motion *creatureMotion, schoolMask uint32) int32 {
	var total int32
	if s == nil || s.server == nil || s.server.Data == nil || motion == nil {
		return total
	}
	key := creatureAuraKeyForMotion(motion)
	if key.GUID == 0 {
		return total
	}
	s.server.auraMu.Lock()
	auras := s.server.activeCreatureAuras[key]
	copies := make([]*activeAura, 0, len(auras))
	for _, aura := range auras {
		copies = append(copies, aura)
	}
	s.server.auraMu.Unlock()
	for _, aura := range copies {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if index >= len(aura.Amounts) || !spellEffectIsAuraEffect(effect) ||
				effect.Aura != auraModHealingDone || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			misc := uint32(effect.MiscValue)
			if misc != 0 && misc&schoolMask == 0 {
				continue
			}
			amt := aura.Amounts[index]
			if amt == 0 {
				amt = effect.BasePoints
			}
			total += amt
		}
	}
	return total
}

// healVictimModHealing mirrors the victim leg of Unit::SpellHealingBonusDone
// (Unit.cpp:7554): the victim's SPELL_AURA_MOD_HEALING (115, Amplify/Dampen
// Magic) auras raise the caster's advertised benefit per school. C++ gates on
// (misc & schoolMask) != 0 (Unit.cpp:4937-4945), so misc-0 auras never fire —
// the shared misc-mask helpers already carry that gate.
func (s *session) healVictimModHealing(ctx context.Context, victimGUID uint64, schoolMask uint32) int32 {
	if s == nil || victimGUID == 0 || schoolMask == 0 {
		return 0
	}
	if victimGUID == s.playerGUID {
		var total int32
		for _, amt := range s.auraTypeModifiersByMiscMask(auraModHealing, schoolMask) {
			total += amt
		}
		return total
	}
	if s.server == nil {
		return 0
	}
	if ps := s.server.findSessionByGUID(victimGUID); ps != nil && ps.player != nil {
		var total int32
		for _, amt := range ps.auraTypeModifiersByMiscMask(auraModHealing, schoolMask) {
			total += amt
		}
		return total
	}
	if target, ok := s.getCombatTarget(ctx, victimGUID); ok {
		var total int32
		for _, amt := range creatureAuraModifiersByMiscMask(s.server, creatureAuraKeyForTarget(target), auraModHealing, schoolMask) {
			total += amt
		}
		return total
	}
	return 0
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
			if !s.server.hasLineOfSightSpellCheck(motion.Map, motion.X, motion.Y, motion.Z, x, y, z) {
				return true
			}
		}
	}
	if hasDest &&
		spell.AttributesEx1&spellAttr2CanTargetNotInLOS == 0 &&
		spell.AttributesEx5&spellAttr5SkipCheckcastLosCheck == 0 {
		if !s.server.hasLineOfSightSpellCheck(motion.Map, motion.X, motion.Y, motion.Z, dx, dy, dz) {
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
	if !s.server.checkCreatureSpellRange(motion, spell, targetGUID, s) {
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
	// (creatureAIAreaEnemyPlayers) finds the extra enemy players and the
	// creature-motion twin (creatureAIAreaEnemyCreatures) the extra enemy
	// creatures; the wire victim is excluded from both results, and their
	// GUIDs extend the GO packet hit list below.
	var areaHits []*playerPos
	var areaCreatureHits []uint64
	if isAreaEnemySpell(spell) {
		if victimPos := s.petAreaVictimPos(ctx, motion, targetGUID); victimPos != nil {
			areaHits = s.server.creatureAIAreaEnemyPlayers(ctx, motion, spell, victimPos)
			areaCreatureHits = s.server.creatureAIAreaEnemyCreatures(ctx, motion, spell, victimPos)
		}
	}
	for _, areaHit := range areaHits {
		hitTargets = append(hitTargets, areaHit.GUID)
	}
	hitTargets = append(hitTargets, areaCreatureHits...)
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
	evadedSet := map[uint64]bool{}
	type petReflectTrip struct {
		victimGUID uint64
		status     uint8
	}
	var reflectTrips []petReflectTrip
	for _, guid := range hitTargets {
		if guid == 0 || reflectedSet[guid] {
			continue
		}
		reflected, evaded, status := s.petSpellReflectVerdict(ctx, motion, spell, guid)
		if evaded {
			evadedSet[guid] = true
			continue
		}
		if reflected {
			reflectedSet[guid] = true
			reflectTrips = append(reflectTrips, petReflectTrip{victimGUID: guid, status: status})
		}
	}
	var missStatus []protocol.SpellMissStatus
	// Spell::SendSpellGo (Spell.cpp:4494-4507): only MISS_NONE targets ride
	// HitTargets — reflected victims and the return-trip caster report
	// through MissStatus (REFLECT + ReflectStatus), never as hits.
	packetHits := make([]uint64, 0, len(hitTargets))
	for _, guid := range hitTargets {
		if reflectedSet[guid] || evadedSet[guid] {
			continue
		}
		packetHits = append(packetHits, guid)
	}
	for guid := range evadedSet {
		missStatus = append(missStatus, protocol.SpellMissStatus{TargetGUID: guid, Reason: protocol.SpellMissEvade})
	}
	for _, trip := range reflectTrips {
		missStatus = append(missStatus, protocol.SpellMissStatus{TargetGUID: trip.victimGUID, Reason: protocol.SpellMissReflect, ReflectStatus: trip.status})
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
	// Spell::PreprocessTarget (Spell.cpp:2350) and DoSpellHitOnUnit
	// (Spell.cpp:7740-7743): a non-NONE ReflectResult nulls _spellHitTarget,
	// so no unit resolves and the caster takes nothing — the reflector's
	// entry rides MissStatus in the GO packet, but no effect leg runs for
	// it. Only clean (NONE) reflects retarget the return trip to the pet.
	reflectNulled := make(map[uint64]bool, len(reflectTrips))
	for _, trip := range reflectTrips {
		if trip.status != protocol.SpellMissNone {
			reflectNulled[trip.victimGUID] = true
		}
	}
	// resolveReflectTarget maps a reflected victim to the pet for the
	// effect legs below — the return trip's _spellHitTarget is the caster
	// (Spell.cpp:2350, 7740-7743), so damage, heal, dispel, aura and
	// trigger effects all land on the pet instead of the reflector. A
	// nulled victim (non-NONE ReflectResult) resolves to 0 and every
	// effect leg below skips it.
	resolveReflectTarget := func(guid uint64) uint64 {
		if reflectNulled[guid] || evadedSet[guid] {
			return 0
		}
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
		// Unit::SpellDamageBonusDone (Unit.cpp:6681-6683, 6725-6731): the
		// guardian's GetBonusDamage() rides the advertised benefit into the
		// per-effect DoneTotal.
		if bonus := s.petGuardianDoneDamageBonus(motion, spell); bonus != 0 {
			if adjusted := int32(damage) + bonus; adjusted > 0 {
				damage = uint32(adjusted)
			} else {
				damage = 0
			}
		}
		// Unit::SpellDamageBonusDone (Unit.cpp:6617-6637): the flat
		// done-scripted-mod arm — the owner's OVERRIDE_CLASS_SCRIPTS set —
		// rides DoneTotal flat, victim-independent like the advertised fold
		// above. (The per-victim 180 arm folds inside petDamageFor.)
		if flat := s.petDoneFlatClassScripts(spell); flat != 0 {
			if adjusted := int32(damage) + flat; adjusted > 0 {
				damage = uint32(adjusted)
			} else {
				damage = 0
			}
		}
		schoolMask := uint8(spell.SchoolMask)
		if schoolMask == 0 {
			schoolMask = 1
		}
		// Unit::SpellDamagePctDone (Unit.cpp:6738-7050): the done percent
		// multiplier is per victim in C++ (the versus and class-script arms
		// read the victim), so the pet's school/versus arms plus the
		// owner-scoped class-script table fold per target here, after the
		// flat DoneTotal above (C++ order: (pdamage + DoneTotal) * DoneTotalMod).
		petDamageFor := func(victimGUID uint64) uint32 {
			d := damage
			if target, ok := s.getCombatTarget(ctx, victimGUID); ok {
				// Unit::SpellDamageBonusDone (Unit.cpp:6682): the victim's
				// SPELL_AURA_MOD_DAMAGE_TAKEN (14, Amplify/Dampen Magic) arm
				// rides the advertised benefit ahead of the pct leg — the
				// weapon-damage gate inside victimDamageTakenFlat keeps it
				// off the melee-taken side (Unit.cpp:7996).
				if v14 := s.victimDamageTakenFlat(spell, victimGUID, target); v14 != 0 {
					if adjusted := int32(d) + s.petDamageEffectBonus(spell, motion, v14); adjusted > 0 {
						d = uint32(adjusted)
					} else {
						d = 0
					}
				}
				// Unit::SpellDamageBonusDone (Unit.cpp:6646-6647): the pet's
				// own MOD_FLAT_SPELL_DAMAGE_VERSUS (180) arm by the victim's
				// creature-type mask rides DoneTotal flat per victim, ahead
				// of the pct leg like the aura-14 arm.
				if v180 := s.petDoneFlatVersus(ctx, motion, spell, victimGUID); v180 != 0 {
					if adjusted := int32(d) + v180; adjusted > 0 {
						d = uint32(adjusted)
					} else {
						d = 0
					}
				}
				// Unit::SpellDamageBonusDone (Unit.cpp:6684-6706): the
				// ap_bonus arm rides DoneTotal flat per victim, ahead of
				// the pct leg; the direct cast path carries stack 1.
				if ap := s.petApBonusDoneFlat(ctx, motion, spell, victimGUID, target, false, 1); ap != 0 {
					if adjusted := int32(d) + ap; adjusted > 0 {
						d = uint32(adjusted)
					} else {
						d = 0
					}
				}
				if pct := s.petDamagePctDone(ctx, motion, spell, victimGUID, target); pct != 1.0 {
					d = uint32(float64(d) * pct)
				}
			}
			// Unit::SpellDamageBonusDone (Unit.cpp:6731-6734): the mod-owner's
			// SPELLMOD_DAMAGE / SPELLMOD_DOT fold applies to the damage total
			// after the pct leg — the cast loop carries direct damage effects
			// only, so the op is SPELLMOD_DAMAGE (the DoT tick path is the
			// dot twin).
			if out := s.applySpellMod(spell, spellModDamage, int32(d)); out < 0 {
				d = 0
			} else {
				d = uint32(out)
			}
			return d
		}
		if rt := resolveReflectTarget(targetGUID); rt != 0 {
			reflectorGUID := uint64(0)
			if reflectedSet[targetGUID] {
				reflectorGUID = targetGUID
			}
			s.executePetSpellDamage(ctx, motion, rt, spell.ID, petDamageFor(rt), schoolMask, triggered, reflectorGUID)
		}
		// SelectImplicitAreaTargets (Spell.cpp:1227): each area-selected
		// enemy takes its own hit — the per-target miss roll, resistance,
		// absorb and kill legs run per target inside
		// executePetSpellDamage, matching DoTargetSpellHit per target.
		for _, areaHit := range areaHits {
			if rt := resolveReflectTarget(areaHit.GUID); rt != 0 {
				reflectorGUID := uint64(0)
				if reflectedSet[areaHit.GUID] {
					reflectorGUID = areaHit.GUID
				}
				s.executePetSpellDamage(ctx, motion, rt, spell.ID, petDamageFor(rt), schoolMask, triggered, reflectorGUID)
			}
		}
		// SelectImplicitAreaTargets (Spell.cpp:1227): the area-selected
		// enemy creatures take their own hits too — executePetSpellDamage
		// is GUID-generic (getCombatTarget resolves motions; the taken
		// side runs the creature twin), and the reflect/evade verdicts
		// above already ran for these GUIDs through hitTargets, so a
		// reflected creature hit lands on the pet and an evaded one
		// resolves to 0 and skips, exactly like the player area hits.
		for _, areaCreatureGUID := range areaCreatureHits {
			if rt := resolveReflectTarget(areaCreatureGUID); rt != 0 {
				reflectorGUID := uint64(0)
				if reflectedSet[areaCreatureGUID] {
					reflectorGUID = areaCreatureGUID
				}
				s.executePetSpellDamage(ctx, motion, rt, spell.ID, petDamageFor(rt), schoolMask, triggered, reflectorGUID)
			}
		}
	}
	if !hasDamage {
		// Reflect return trip for a spell with no damage effect: the
		// damage leg never runs, but DoDamageAndTriggers still runs its
		// tail per TargetInfo (Spell.cpp:2565-2575 !hasDamage branch,
		// 2609-2652) — NODAMAGE procs, the reflector stand-up, the
		// SpellHitTarget hook with the pet as _spellHitTarget, and the
		// per-hit trigger consumer. Only clean (NONE) reflects: a nulled
		// trip resolves no _spellHitTarget (Spell.cpp:2350) and runs
		// nothing. The leech leg needs no arm here — a zero-damage trip
		// has a zero leech basis.
		zeroSchoolMask := uint8(spell.SchoolMask)
		if zeroSchoolMask == 0 {
			zeroSchoolMask = 1
		}
		for _, trip := range reflectTrips {
			if reflectNulled[trip.victimGUID] {
				continue
			}
			if spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
				typeMask := spellDamageProcTypeMask(spell) | spellDamageTakenProcTypeMask(spell)
				if typeMask != 0 {
					spellCopy := spell
					s.server.fireCreatureAuraProcLoop(ctx, motion, motion.GUID, procEventInfo{
						typeMask:       typeMask,
						schoolMask:     uint32(zeroSchoolMask),
						spellTypeMask:  procSpellTypeNoDmgHeal,
						spellPhaseMask: procSpellPhaseHit,
						hitMask:        procHitNormal,
						triggered:      triggered,
						eventSpell:     &spellCopy,
						actorGUID:      motion.GUID,
						damage:         0,
					})
				}
			}
			s.server.standPetReflectReflectorUp(motion, trip.victimGUID, spell)
			s.server.fireCreatureSpellHitTarget(ctx, motion, motion.GUID, spell.ID)
			s.server.fireCreatureHitTriggers(ctx, motion, spell, spell.ID, []uint64{motion.GUID})
		}
	}
	for i, effect := range spell.Effects {
		if effect.Effect == spellEffectHeal || effect.Effect == spellEffectHealMechanical || effect.Effect == spellEffectHealPct {
			// Spell::PreprocessTarget (Spell.cpp:2350): a non-NONE
			// ReflectResult nulls _spellHitTarget, so the heal leg never
			// runs for the nulled victim (resolveReflectTarget returns 0).
			if resolveReflectTarget(targetGUID) == 0 {
				continue
			}
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
				// Unit::SpellHealingBonusDone (Unit.cpp:7558-7561, 7599-7605):
				// the guardian's GetBonusDamage() rides the heal advertised
				// benefit into the per-effect DoneTotal. Direct heal effects:
				// the direct coefficient and a stack of 1.
				if bonus := s.petGuardianDoneHealingBonus(ctx, motion, spell, i, resolveReflectTarget(targetGUID), false, 1); bonus != 0 {
					if adjusted := int32(heal) + bonus; adjusted > 0 {
						heal = uint32(adjusted)
					} else {
						heal = 0
					}
				}
				// Unit::SpellHealingPctDone (Unit.cpp:7634-7711): the done
				// percent leg is per victim in C++ and multiplies after the
				// flat DoneTotal — heal = (healamount + DoneTotal) *
				// DoneTotalMod (Unit.cpp:7612). The leech/funnel zeroing kills
				// only the flat leg, so this fold runs even when the bonus
				// above was 0.
				if pct := s.petHealingPctDone(ctx, motion, spell, resolveReflectTarget(targetGUID)); pct != 1.0 {
					tmp := float64(int32(heal)) * pct
					if tmp < 0 {
						tmp = 0
					}
					heal = uint32(tmp)
				}
				// Unit::SpellHealingBonusDone (Unit.cpp:7614-7616): the
				// SPELLMOD_DAMAGE / SPELLMOD_DOT spellmod applies to the heal
				// total via GetSpellModOwner — the owner player (s). The pet
				// cast loop carries direct heal effects only, so the op is
				// SPELLMOD_DAMAGE (the HoT tick path is the dot twin).
				if out := s.applySpellMod(spell, spellModDamage, int32(heal)); out < 0 {
					heal = 0
				} else {
					heal = uint32(out)
				}
				s.executePetSpellHeal(ctx, motion, resolveReflectTarget(targetGUID), spell.ID, heal)
			}
		}
		if effect.Effect == spellEffectTriggerSpell && effect.TriggerSpell != 0 && effect.TriggerSpell != spell.ID {
			if triggered, found, err := s.server.Data.Spell(effect.TriggerSpell); err == nil && found {
				triggerTarget := target
				if reflectNulled[target.UnitGUID] {
					// Spell.cpp:2350 — nulled reflect victim: the trigger
					// leg's _spellHitTarget resolves no unit, so the
					// trigger spell does not fire.
					continue
				}
				if reflectedSet[target.UnitGUID] {
					triggerTarget.UnitGUID = motion.GUID
				}
				s.executePetSpellWithOptions(ctx, motion, triggered, 0, triggerTarget, true)
			}
		}
		if effect.Effect == spellEffectDispel {
			if rt := resolveReflectTarget(targetGUID); rt != 0 {
				s.executePetSpellDispel(ctx, motion, rt, spell, effect, reflectedSet[targetGUID])
			}
		}
		if effect.Effect == spellEffectDispelMechanic {
			// Spell::EffectDispelMechanic (SpellEffects.cpp:4733-4756):
			// the pet path's dispel arm handled only effect 38 — 108
			// fell to the unhandled arm and did nothing, while C++
			// dispatches it per unit target (SpellEffects.cpp:177) and the
			// player path bridges it (spells.go case 108). Reflection
			// maps the victim to the pet like the 38 arm above
			// (Spell.cpp:2350, 7740-7743); a nulled victim skips it.
			if rt := resolveReflectTarget(targetGUID); rt != 0 {
				s.handleEffectDispelMechanic(ctx, rt, spell, effect)
			}
		}
		if effect.Effect == spellEffectInstaKill {
			// Spell::EffectInstaKill (SpellEffects.cpp:274-296) for a pet
			// caster: the HIT_TARGET arm, previously missing on this path
			// entirely — an effect-1 pet spell killed nothing. C++ runs it
			// per hit target (DoProcessTargetContainer, Spell.cpp:3555-
			// 3566), so the wire victim plus the area-selected victims
			// all take it, unlike the heal/dispel/aura arms above which
			// only model the wire target. The shared creature-caster twin
			// (boss_ai.go) carries the gates, the INSTAKILLLOG fan-out
			// and the kill funnels; its reflected map retargets landed
			// reflects to the pet (Spell.cpp:2350) and skips bounced
			// and evaded victims, matching the damage leg's
			// resolveReflectTarget. Runs after the damage leg above —
			// the pet path's grouped-effect approximation, like the
			// other arms in this loop.
			reflected := make(map[uint64]uint8, len(reflectTrips))
			for _, trip := range reflectTrips {
				reflected[trip.victimGUID] = trip.status
			}
			evaded := make(map[uint64]bool, len(evadedSet))
			for guid := range evadedSet {
				evaded[guid] = true
			}
			s.server.creatureEffectInstaKill(ctx, motion, spell.ID, hitTargets, reflected, evaded)
		}
		if effect.Effect == spellEffectApplyAura || effect.Effect == spellEffectApplyAreaAuraParty || effect.Effect == spellEffectApplyAreaAuraEnemy {
			// Spell::PreprocessSpellHit scaleAura leg (Spell.cpp:2780-2795):
			// a target whose aura scales takes the aura from the
			// rank-appropriate row — duration entry, basepoints and effect
			// row all come from it, like hitInfo.AuraBasePoints.
			auraSpell, auraEffect := petAuraRankRow(spell, i, petAuraScaleDownranks, targetGUID)
			durationMs := petAuraDuration(s.server, auraSpell, motion)
			victimGUID := resolveReflectTarget(targetGUID)
			if victimGUID == 0 {
				// Spell.cpp:2350 — nulled reflect victim: no aura leg.
				continue
			}
			var victim combatTarget
			if ct, ok := s.getCombatTarget(ctx, victimGUID); ok {
				victim = ct
			}
			amount := uint32(auraEffect.CalcValueForLevel(auraSpell, uint32(motion.Level)))
			// AuraEffect::CalculateAmount (SpellAuraEffects.cpp:505-518):
			// the periodic done leg bakes at apply for unit auras.
			amount = s.petPeriodicApplyDoneLegs(ctx, motion, auraSpell, i, victimGUID, victim, amount)
			if s.applyPetAura(ctx, motion, auraSpell, auraEffect, victimGUID, durationMs, amount) {
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
					aVictimGUID := resolveReflectTarget(areaHit.GUID)
					if aVictimGUID == 0 {
						continue
					}
					var aVictim combatTarget
					if ct, ok := s.getCombatTarget(ctx, aVictimGUID); ok {
						aVictim = ct
					}
					aAmount := uint32(aEffect.CalcValueForLevel(aSpell, uint32(motion.Level)))
					aAmount = s.petPeriodicApplyDoneLegs(ctx, motion, aSpell, i, aVictimGUID, aVictim, aAmount)
					s.applyPetAura(ctx, motion, aSpell, aEffect, aVictimGUID, aDurationMs, aAmount)
				}
				// SelectImplicitAreaTargets (Spell.cpp:1227): the
				// area-selected enemy creatures take the area aura
				// too — applyPetAura is GUID-generic (creature
				// targets land in the creature aura store), and the
				// downrank map above already covers these GUIDs
				// through hitTargets.
				for _, areaCreatureGUID := range areaCreatureHits {
					aSpell, aEffect := petAuraRankRow(spell, i, petAuraScaleDownranks, areaCreatureGUID)
					aDurationMs := petAuraDuration(s.server, aSpell, motion)
					aVictimGUID := resolveReflectTarget(areaCreatureGUID)
					if aVictimGUID == 0 {
						continue
					}
					var aVictim combatTarget
					if ct, ok := s.getCombatTarget(ctx, aVictimGUID); ok {
						aVictim = ct
					}
					aAmount := uint32(aEffect.CalcValueForLevel(aSpell, uint32(motion.Level)))
					aAmount = s.petPeriodicApplyDoneLegs(ctx, motion, aSpell, i, aVictimGUID, aVictim, aAmount)
					s.applyPetAura(ctx, motion, aSpell, aEffect, aVictimGUID, aDurationMs, aAmount)
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
		// Aura::_casterInfo.ApplyResilience (SpellAuras.cpp:450): captured at
		// aura creation from the live caster's Unit::CanApplyResilience, read
		// at every periodic tick (SpellAuras.h:167). Computed before castMu.
		applyResilience := s.casterCanApplyResilience(caster.GUID)
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
		aura := &activeAura{SpellID: spell.ID, DispelType: spell.DispelType, Mechanic: spell.Mechanic, AuraType: effect.Aura, EffectMask: effectMask, RecalculateMask: recalculateMask, CasterGUID: caster.GUID, TargetGUID: targetGUID, SchoolMask: spell.SchoolMask, MiscValue: effect.MiscValue, Amount: uint32(amount), Amounts: amounts, ApplyResilience: applyResilience, BaseAmounts: baseAmounts, DurationMs: durationMs, PeriodMs: periodMs, RemainingMs: durationMs, Slot: slot, Positive: positive, CasterLevel: uint8(casterLevel(caster)), StoredCasterLevel: uint8(casterLevel(caster)), AuraInterruptFlags: spell.AuraInterruptFlags, TriggerSpell: effect.TriggerSpell, StackAmount: spell.StackAmount, HideDuration: spell.AttributesEx5&spellAttr5HideDuration != 0, StackCount: 1, OwnerPetAura: source.SpellID != 0, OwnerPetAuraSourceSpell: source.SpellID, OwnerPetAuraSourceEffect: source.EffectIndex, OwnerPetAuraSourceDamage: sourceDamage, OwnerPetAuraRemoveOnChange: removeOnChange}
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
	// Aura::_casterInfo.ApplyResilience (SpellAuras.cpp:450): captured at
	// aura creation from the live caster's Unit::CanApplyResilience.
	// Computed before auraMu is taken.
	applyResilience := s.casterCanApplyResilience(caster.GUID)
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
	aura := &activeAura{SpellID: spell.ID, DispelType: spell.DispelType, Mechanic: spell.Mechanic, AuraType: effect.Aura, EffectMask: effectMask, RecalculateMask: recalculateMask, CasterGUID: caster.GUID, TargetGUID: targetGUID, TargetKey: targetKey, SchoolMask: spell.SchoolMask, MiscValue: effect.MiscValue, Amount: uint32(amount), Amounts: amounts, ApplyResilience: applyResilience, BaseAmounts: baseAmounts, DurationMs: durationMs, PeriodMs: periodMs, RemainingMs: durationMs, Slot: slot, Positive: positive, CasterLevel: uint8(casterLevel(caster)), StoredCasterLevel: uint8(casterLevel(caster)), AuraInterruptFlags: spell.AuraInterruptFlags, TriggerSpell: effect.TriggerSpell, StackAmount: spell.StackAmount, HideDuration: spell.AttributesEx5&spellAttr5HideDuration != 0, StackCount: 1, OwnerPetAura: source.SpellID != 0, OwnerPetAuraSourceSpell: source.SpellID, OwnerPetAuraSourceEffect: source.EffectIndex, OwnerPetAuraSourceDamage: sourceDamage, OwnerPetAuraRemoveOnChange: removeOnChange}
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
	// Guardian::UpdateStats on pet aura apply (StatSystem.cpp:1266-1282 via
	// Unit::UpdateUnitMod -> UpdateStats, Unit.cpp:9304-9310): the pet-cast
	// path runs the same stat-aura recompute as the player-cast path —
	// the trigger gates on the spell's 29/137 effects and on motionIsPet.
	s.server.recomputePetDerivedOnStatAuraChange(targetKey, spell.ID, true, nil)
	// Guardian::UpdateAttackPowerAndDamage on the same apply
	// (StatSystem.cpp:1274 / Unit.cpp:9336-9337): a strength, 99 or 166
	// aura landing on a pet rewrites its AP unit fields.
	s.server.recomputePetAttackPowerOnAuraChange(targetKey, spell.ID)
	// Guardian::UpdateDamagePhysical on the same apply (Unit.cpp:9335,
	// StatSystem.cpp:1479-1554): a 13/79 NORMAL-misc aura landing on a pet
	// rewrites UNIT_FIELD_MINDAMAGE/MAXDAMAGE (the strength/AP arms ride
	// the recomputePetAttackPowerOnAuraChange tail above).
	s.server.recomputePetDamageFieldsOnAuraChange(targetKey, spell.ID)
	// Guardian::UpdateArmor / Creature::UpdateArmor on the same apply
	// (StatSystem.cpp:1317-1330, 1018-1022): a direct armor aura (22/83/
	// 101/142/143, NORMAL misc) rewrites UNIT_FIELD_RESISTANCES (armor).
	s.server.recomputeCreatureArmorOnAuraChange(targetKey, spell.ID)
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

// creatureApplyHealAbsorb mirrors Unit::CalcHealAbsorb (Unit.cpp:2067-2113)
// over the creature aura store: the target's 301
// (SPELL_AURA_SCHOOL_HEAL_ABSORB) shields whose misc mask covers the heal's
// school consume the heal in slot order, each shield decremented by what it
// took; shields drained to zero are removed after the loop (the C++
// existExpired leg). The player-side twin is applyHealAbsorb.
func (s *Server) creatureApplyHealAbsorb(key creatureAuraKey, schoolMask, heal uint32) uint32 {
	if s == nil || heal == 0 || key.GUID == 0 {
		return 0
	}
	s.auraMu.Lock()
	auras := s.activeCreatureAuras[key]
	copies := make([]*activeAura, 0, len(auras))
	for _, aura := range auras {
		copies = append(copies, aura)
	}
	s.auraMu.Unlock()
	sort.Slice(copies, func(i, j int) bool {
		if copies[i] == nil || copies[j] == nil {
			return copies[j] == nil
		}
		return copies[i].Slot < copies[j].Slot
	})
	var exhausted []uint32
	var absorbed uint32
	s.auraMu.Lock()
	for _, aura := range copies {
		if aura == nil || aura.Stopped || aura.AuraType != spellAuraSchoolHealAbsorb || absorbed >= heal {
			continue
		}
		if aura.MiscValue&int32(schoolMask) == 0 {
			continue
		}
		if aura.Amount == 0 {
			exhausted = append(exhausted, aura.SpellID)
			continue
		}
		take := aura.Amount
		if take > heal-absorbed {
			take = heal - absorbed
		}
		aura.Amount -= take
		absorbed += take
		if aura.Amount == 0 {
			exhausted = append(exhausted, aura.SpellID)
		}
	}
	s.auraMu.Unlock()
	for _, id := range exhausted {
		s.removeCreatureAura(key, id)
	}
	return absorbed
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
	// (dotType=false). The SpellHealingBonusDone done legs ride the pet
	// cast site in executePetSpellWithOptions (flat advertised fold with
	// the guardian bonus arm, AP arms and OVERRIDE_CLASS_SCRIPTS arms via
	// petGuardianDoneHealingBonus; the SpellHealingPctDone leg; the
	// SPELLMOD_DAMAGE tail) — only the taken leg remains here.
	targetSess := s.server.findSessionByGUID(target.GUID)
	var absorbMask uint32
	if s.server.Data != nil {
		if hs, found, err := s.server.Data.Spell(spellID); err == nil && found {
			absorbMask = uint32(hs.SchoolMask)
		}
	}
	if targetSess != nil && targetSess.player != nil {
		heal = s.healingTakenBonus(targetSess, caster.GUID, spellID, heal, false)
	} else if s.server.Data != nil {
		if hs, found, err := s.server.Data.Spell(spellID); err == nil && found {
			heal = creatureHealingTakenBonus(s.server, creatureAuraKeyForTarget(target), caster.GUID, hs, true, heal, false)
		}
	}
	// Unit::HealBySpell (Unit.cpp:6561-6570): CalcHealAbsorb consumes the
	// target's 301 heal-absorb shields, then sScriptMgr->ModifyHealRecieved
	// — AutoBalance_UnitScript::_Modifer_DealDamage scales a creature
	// caster's direct heal the same way it scales its damage — ahead of
	// DealHeal and the heal log. The taken legs above feed both, and the
	// hook fires for player and creature targets alike (the attacker arm
	// keys on the caster, which is always a creature motion here).
	var absorbed uint32
	if targetSess != nil && targetSess.player != nil {
		absorbed = targetSess.applyHealAbsorb(absorbMask, heal)
	} else {
		absorbed = s.server.creatureApplyHealAbsorb(creatureAuraKeyForTarget(target), absorbMask, heal)
	}
	heal -= absorbed
	heal = s.server.autoBalanceModifyDealDamageOnMap(caster, target.Map, heal)
	// Unit::SpellCritChanceDone (Unit.cpp:7167-7212) creature arm on direct
	// heals (Spell::DoEffectOnLaunchTarget, Spell.cpp:7782-7790): pets may
	// crit heals. Positive spells skip the taken arms in C++
	// (Unit.cpp:7228-7240), so only the done leg rolls; the bonus rides
	// Unit::SpellCriticalHealingBonus via the nil-session base (the
	// periodic-tick sites use the same convention).
	isCrit := false
	if s.server != nil && s.server.Data != nil {
		if hs, found, err := s.server.Data.Spell(spellID); err == nil && found {
			chance := s.server.creatureSpellCritChanceDone(creatureAuraKeyForMotion(caster), caster.FlagsExtra&0x20000 != 0, uint8(hs.SchoolMask), hs, true)
			// Unit::SpellCritChanceDone (Unit.cpp:7209-7210): the owner's
			// SPELLMOD_CRITICAL_CHANCE folds into the done chance — the
			// mod arm has no done-gate in C++.
			if caster.OwnerGUID != 0 && s.player != nil && caster.OwnerGUID == s.playerGUID {
				chance = float64(s.applySpellMod(hs, spellModCriticalChance, int32(math.Round(chance*100)))) / 100.0
			}
			isCrit = chance > 0 && rand.Float64() < chance
			if isCrit {
				var nilSess *session
				heal = nilSess.spellCriticalHealingBonus(ctx, heal, targetGUID, hs.DefenseType, false)
			}
		}
	}
	// Unit::DealHeal (Unit.cpp:6299-6304): the victim AI's HealReceived and
	// the healer AI's HealDone fire ahead of the health gain on the
	// pre-clamp addhealth — the C++ hook takes addhealth by reference, so
	// it may still adjust the amount. Player victims carry no AI model in
	// Go, so the received leg fires only for creature victims.
	var victimMotion *creatureMotion
	if targetSess == nil || targetSess.player == nil {
		victimMotion = s.findCreatureMotion(target.GUID)
	}
	fireDealHealAIHooks(ctx, s.server, victimMotion, caster, caster.GUID, target.GUID, &heal)
	overheal := uint32(0)
	if uint64(target.Health)+uint64(heal) > uint64(target.MaxHealth) {
		overheal = uint32(uint64(target.Health) + uint64(heal) - uint64(target.MaxHealth))
	}
	packet := buildSpellHealLog(target.GUID, caster.GUID, spellID, heal, overheal, absorbed, isCrit)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, true)
	// Unit::SendHealSpellLog (Unit.cpp:6546-6558) fans out via
	// SendMessageToSet — the caster creature's visibility set (100.0f),
	// not the whole instance.
	if s.server != nil && caster != nil {
		s.server.broadcastToVisibleSet(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), packet, caster.Map, caster.InstanceID, caster.X, caster.Y, caster.Z, s)
	}
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
// The evaded return reports a creature victim in evade mode:
// WorldObject::SpellHitResult (Object.cpp:2637-2639) returns
// SPELL_MISS_EVADE ahead of the reflect roll, so an evading creature
// never rolls (or consumes) its reflect aura.
func (s *session) petSpellReflectVerdict(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, targetGUID uint64) (reflected bool, evaded bool, status uint8) {
	if s == nil || s.server == nil || motion == nil || targetGUID == 0 || targetGUID == motion.GUID {
		return false, false, protocol.SpellMissNone
	}
	if !spellCanBeReflected(spell) {
		return false, false, protocol.SpellMissNone
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
		return false, false, protocol.SpellMissNone
	}
	reflected = false
	if targetSess != nil {
		reflected = targetSess.checkSpellReflection(spell)
		if reflected {
			// Synchronous pet path: the C++ ProcReflectDelayed arm
			// (Spell.cpp:2181) would fire at TimeDelay, but this path
			// applies everything synchronously — the charge is spent
			// here, the arrival equivalent.
			targetSess.consumeReflectAuraCharge()
		}
	} else {
		if s.player != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, targetGUID) {
			return false, true, protocol.SpellMissNone
		}
		var key creatureAuraKey
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			key = creatureAuraKeyForTarget(tgt)
		}
		reflected = s.server.creatureCheckSpellReflection(key, spell)
		if reflected {
			s.server.consumeCreatureReflectAuraCharge(key)
		}
	}
	if !reflected {
		return false, false, protocol.SpellMissNone
	}
	if s.creatureTargetImmuneToSpell(ctx, motion.GUID, spell, s, motion.Faction) {
		return true, false, protocol.SpellMissImmune
	}
	if spellHasOnlyDamageEffects(spell) && creatureImmuneToDamageSpell(s.server, creatureAuraKeyForMotion(motion), spell, spell.SchoolMask) {
		return true, false, protocol.SpellMissImmune
	}
	return true, false, protocol.SpellMissNone
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
func (s *session) firePetSpellDoneSideProcs(ctx context.Context, caster *creatureMotion, targetGUID uint64, spellID uint32, schoolMask uint8, triggered, isCrit bool) {
	if s == nil || s.server == nil || s.server.Data == nil || caster == nil || targetGUID == caster.GUID {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found || spell.AttributesEx3&spellAttr3CantTriggerProc != 0 {
		return
	}
	spellCopy := spell
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2581): the
	// done-side proc event's hit mask carries the rolled crit
	// (targetInfo.IsCrit, Spell.cpp:7790).
	hitMask := procHitNormal
	if isCrit {
		hitMask = procHitCritical
	}
	s.server.fireCreatureAuraProcLoop(ctx, caster, targetGUID, procEventInfo{
		typeMask:       spellDamageProcTypeMask(spellCopy),
		schoolMask:     uint32(schoolMask),
		spellTypeMask:  procSpellTypeDamage,
		spellPhaseMask: procSpellPhaseHit,
		hitMask:        hitMask,
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

// applyPetHealthLeechHeal mirrors Spell::EffectHealthLeech
// (SpellEffects.cpp:1513-1553) for a pet caster: the caster heals for the
// damage the target actually lost (leechDealt — post-absorb, overkill
// excluded, captured at the damage-log site) times the effect's
// CalcValueMultiplier (SpellInfo.cpp:528-536). The SpellHealingBonusDone
// percent leg (136 auras + mod-owner SPELLMOD_DAMAGE, Unit.cpp:7505-7632)
// and the SpellHealingBonusTaken direct arms (Unit.cpp:7714-7759) fold on
// the caster's own aura store ahead of the absorb — same as the inline twin
// (creature_area_targets.go). The self-heal runs through Unit::HealBySpell — CalcHealAbsorb and the AutoBalance
// ModifyHealRecieved hook (AutoBalance.cpp:245) ahead of the health gain
// — under the C++ caster-alive gate (SpellEffects.cpp:1542). No heal log:
// the inline twin sends none. No threat: Unit::DealHeal carries no
// threat leg.
func (s *Server) applyPetHealthLeechHeal(caster *creatureMotion, spellID uint32, schoolMask uint8, leechDealt uint32) {
	if s == nil || s.Data == nil || caster == nil || leechDealt == 0 {
		return
	}
	leechSpell, found, err := s.Data.Spell(spellID)
	if err != nil || !found || !spellHasEffect(leechSpell, spellEffectHealthLeech) {
		return
	}
	leechIdx := -1
	for i := range leechSpell.Effects {
		if leechSpell.Effects[i].Effect == spellEffectHealthLeech {
			leechIdx = i
			break
		}
	}
	if leechIdx < 0 {
		return
	}
	// SpellEffectInfo::CalcValueMultiplier (SpellInfo.cpp:528-536): the
	// caster mod-owner's SPELLMOD_VALUE_MULTIPLIER folds via
	// GetSpellModOwner — the pet's owner player
	// (Object.cpp:2246-2261).
	modOwner := s.spellModOwnerSession(caster.Map, caster.InstanceID, caster.GUID)
	multiplier := calcValueMultiplier(leechSpell, leechIdx, modOwner)
	heal := effectValueMultiplied(leechDealt, multiplier)
	if heal == 0 {
		return
	}
	// Unit::SpellHealingBonusDone (Unit.cpp:7505-7632): for HEALTH_LEECH
	// spells the flat DoneTotal is zeroed (Unit.cpp:7605-7610), so only the
	// DoneTotalMod percent leg over the caster's 136 auras survives,
	// followed by the mod-owner's SPELLMOD_DAMAGE fold (Unit.cpp:7624).
	heal = uint32(float64(heal) * creatureHealingDonePctMultiplier(s, creatureAuraKeyForMotion(caster), leechSpell))
	if modOwner != nil {
		if sm := modOwner.applySpellMod(leechSpell, uint8(spellModDamage), int32(heal)); sm > 0 {
			heal = uint32(sm)
		} else {
			heal = 0
		}
	}
	// Unit::SpellHealingBonusTaken (Unit.cpp:7714-7759): the direct-heal
	// arms — MOD_HEALING_PCT min/max and the caster-matched
	// MOD_HEALING_RECEIVED — on the caster's own aura store.
	heal = creatureHealingTakenBonus(s, creatureAuraKeyForMotion(caster), caster.GUID, leechSpell, true, heal, false)
	healAbsorbed := s.creatureApplyHealAbsorb(creatureAuraKeyForMotion(caster), uint32(schoolMask), heal)
	scaledHeal := heal - healAbsorbed
	scaledHeal = s.autoBalanceModifyDealDamageOnMap(caster, caster.Map, scaledHeal)
	s.motionMu.Lock()
	if live := s.findCreatureMotionLocked(caster.Map, caster.InstanceID, caster.GUID); live != nil && live.Health > 0 {
		live.Health += scaledHeal
		if live.MaxHealth > 0 && live.Health > live.MaxHealth {
			live.Health = live.MaxHealth
		}
	}
	s.motionMu.Unlock()
}

// standPetReflectReflectorUp runs the DoDamageAndTriggers stand-up arm
// (Spell.cpp:2609-2617) for a pet-path reflect return trip: C++ stands
// TargetInfo::unit — the reflector, resolved from TargetGUID — not
// _spellHitTarget (the pet, PreprocessTarget Spell.cpp:2350). Gated on
// the C++ arms: not friendly to the pet caster, and non-positive or
// dispel-effect. A player reflector stands via its session, a creature
// reflector via standCreatureMotionUp.
func (s *Server) standPetReflectReflectorUp(caster *creatureMotion, reflectorGUID uint64, spell wotlk.Spell) {
	if s == nil || caster == nil || reflectorGUID == 0 || spell.ID == 0 {
		return
	}
	if spellIsPositive(spell) && !spellHasEffect(spell, spellEffectDispel) {
		return
	}
	if s.creatureFriendlyToTarget(caster, reflectorGUID) {
		return
	}
	if rsess := s.findSessionByGUID(reflectorGUID); rsess != nil && rsess.player != nil {
		if rsess.player.StandState != 0 {
			rsess.player.StandState = 0
			rsess.sendPlayerUpdate()
		}
		return
	}
	if tm := s.findCreatureMotion(caster.Map, caster.InstanceID, reflectorGUID); tm != nil {
		s.standCreatureMotionUp(tm)
	}
}

// executePetSpellDamage deals one pet-cast spell hit. reflectorGUID is the
// reflecting victim's GUID on a reflect return trip (the damage target is
// the pet itself), or 0 on a direct hit.
// executePetSpellZeroDamageTail runs the Spell::TargetInfo::DoDamageAndTriggers
// tail for a pet-spell hit whose entry damage is zero (Spell.cpp:2560-2575
// !hasHealing && !hasDamage branch): C++ takes that branch when m_damage is
// zero on entry — a damage-effect spell whose computed damage folded to zero
// — and still runs NODAMAGE procs, the stand-up arm, the SpellHitTarget
// hook and the per-hit trigger consumer. The old entry guard returned
// silently, so such a hit ran none of them, unlike both the !hasDamage
// return-trip branch in executePetSpellWithOptions and the fully-mitigated
// arm inside executePetSpellDamage (whose pre-mitigation m_damage > 0 keeps
// the DAMAGE proc type and the TAKEN_DAMAGE flag — both dropped here, since
// entry-zero has neither). No damage log (C++ logs only in the hasDamage
// branch), no health/threat/tap/strip legs (no DealDamage call on this
// branch), no leech (zero basis). Evaded victims never reach here: the
// call sites skip them via resolveReflectTarget, and the miss roll inside
// this function is a damage>0 arm, so no miss model applies on entry zero.
// The done-side proc leg is skipped for ordinary direct hits, matching the
// standing model (no done-side model for ordinary hits); the self-hit
// return trip fires the combined done+taken loop below, like the !hasDamage
// return-trip branch.
func (s *session) executePetSpellZeroDamageTail(ctx context.Context, caster *creatureMotion, target combatTarget, spellID uint32, schoolMask uint8, triggered bool, reflectorGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil || caster == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return
	}
	spellCopy := spell
	if spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
		if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil && victim.procDeep == 0 {
			victim.procAuraTriggerLoop(ctx, caster.GUID, procEventInfo{
				typeMask:       spellDamageTakenProcTypeMask(spellCopy),
				schoolMask:     uint32(schoolMask),
				spellTypeMask:  procSpellTypeNoDmgHeal,
				spellPhaseMask: procSpellPhaseHit,
				hitMask:        procHitNormal,
				triggered:      triggered,
				eventSpell:     &spellCopy,
				actorGUID:      caster.GUID,
				damage:         0,
			})
		} else if targetMotion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); targetMotion != nil {
			typeMask := spellDamageTakenProcTypeMask(spellCopy)
			if target.GUID == caster.GUID {
				typeMask = spellDamageProcTypeMask(spellCopy) | typeMask
			}
			if typeMask != 0 {
				s.server.fireCreatureAuraProcLoop(ctx, targetMotion, caster.GUID, procEventInfo{
					typeMask:       typeMask,
					schoolMask:     uint32(schoolMask),
					spellTypeMask:  procSpellTypeNoDmgHeal,
					spellPhaseMask: procSpellPhaseHit,
					hitMask:        procHitNormal,
					triggered:      triggered,
					eventSpell:     &spellCopy,
					actorGUID:      caster.GUID,
					damage:         0,
				})
			}
		}
	}
	if reflectorGUID != 0 {
		s.server.standPetReflectReflectorUp(caster, reflectorGUID, spellCopy)
	} else if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
		if victim.player.StandState != 0 && (!spellIsPositive(spellCopy) || spellHasEffect(spellCopy, spellEffectDispel)) {
			victim.player.StandState = 0
			victim.sendPlayerUpdate()
		}
	} else if targetMotion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); targetMotion != nil {
		if (!spellIsPositive(spellCopy) || spellHasEffect(spellCopy, spellEffectDispel)) && !s.server.creatureFriendlyToTarget(caster, target.GUID) {
			s.server.standCreatureMotionUp(targetMotion)
		}
	}
	if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
		s.server.fireCreatureSpellHitTarget(ctx, caster, target.GUID, spellID)
		s.server.fireCreatureHitTriggers(ctx, caster, spellCopy, spellID, []uint64{target.GUID})
	} else if targetMotion := s.server.findCreatureMotion(target.Map, target.InstanceID, target.GUID); targetMotion != nil {
		s.server.fireCreatureLuaEvent(ctx, targetMotion, scripting.CreatureEventOnHitBySpell, s.server.luaMotionCreature(caster), spellID)
		s.server.fireCreatureSpellHitTarget(ctx, caster, target.GUID, spellID)
		s.server.fireCreatureHitTriggers(ctx, caster, spellCopy, spellID, []uint64{target.GUID})
	}
}

func (s *session) executePetSpellDamage(ctx context.Context, caster *creatureMotion, targetGUID uint64, spellID, damage uint32, schoolMask uint8, triggered bool, reflectorGUID uint64) {
	if s == nil || s.server == nil || caster == nil {
		return
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	// Ghost leg mirrors WorldObject::IsValidAttackTarget's "can't attack dead"
	// (Object.cpp:2935-2937): ghosts carry Health=1 in Go.
	if !ok || target.Health == 0 || s.ghostAttackTargetBlocked(targetGUID) {
		return
	}
	if damage == 0 {
		s.executePetSpellZeroDamageTail(ctx, caster, target, spellID, schoolMask, triggered, reflectorGUID)
		return
	}
	isPlayerVictim := s.server.findSessionByGUID(target.GUID) != nil
	hitInfo := uint32(0)
	resisted := uint32(0)
	absorbed := uint32(0)
	// Unit::CalculateSpellDamageTaken block arm (Unit.cpp:1013-1062): the
	// melee/ranged-DmgClass block subtraction lands post-crit, pre-resilience
	// on the pet path too.
	blocked := uint32(0)
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
		// WorldObject::MagicSpellHitResult (Object.cpp:2534-2536): the
		// SPELLMOD_RESIST_MISS_CHANCE arm runs on the pet's mod owner —
		// the owner player (Object.cpp:2246-2261), whose registry is this
		// session's — folding the int32 level-diff base before the aura
		// legs.
		baseHit := int32(magicSpellHitBase(uint8(maxUint32(caster.Level, 1)), target.Level, isPlayerVictim))
		var petSpell wotlk.Spell
		petSpellKnown := false
		if s.server != nil && s.server.Data != nil {
			if ps, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
				petSpell, petSpellKnown = ps, true
				baseHit = s.applySpellMod(petSpell, spellModResistMissChance, baseHit)
			}
		}
		miss := magicSpellHitResultFromBase(float64(baseHit), bonusHit)
		if miss != protocol.SpellMissNone {
			damage = 0
			hitInfo = 0x01
		} else if petSpellKnown {
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
	// Unit::CalculateSpellDamageTaken (Unit.cpp:987-1132): the victim-side
	// taken pct legs run on every direct hit ahead of the armor arm —
	// Spell::EffectSchoolDMG (SpellEffects.cpp:785) folds
	// Unit::SpellDamageBonusTaken (Unit.cpp:7052) on the direct-damage
	// roll. C++ gates the whole arm on DOT (Unit.cpp:7053-7054 returns
	// identity for SPELL_DIRECT_DAMAGE); the Go twins run it on direct
	// damage on every sibling path (player-cast at spells.go:14159, the
	// inline path at creature_area_targets.go:182), so the pet path joins
	// the convention here. The caster arrives as nil on both twins — the
	// pet carries no session auras (Sanctified Wrath bypass) and the
	// from-caster multiplier keys on the caster GUID, which the twins
	// cannot express for a creature caster — matching the inline path's
	// nil-caster convention (creature_area_targets.go:182).
	if damage > 0 && s.server != nil && s.server.Data != nil {
		if takenSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
			if isPlayerVictim {
				if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
					damage = spellDamageBonusTaken(damage, takenSpell, uint32(schoolMask), victim, nil, 1.0)
				}
			} else {
				damage = creatureSpellDamageBonusTaken(s.server, damage, takenSpell, uint32(schoolMask), creatureAuraKeyForTarget(target), nil, 1.0)
			}
		}
	}
	var petResistInput spellResistInput
	petPenetration := uint32(0)
	var petSpell wotlk.Spell
	petSpellKnown := false
	if s.server != nil && s.server.Data != nil {
		if sp, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
			petSpell, petSpellKnown = sp, true
		}
	}
	if petSpellKnown {
		petResistInput.ignoreResistances = petSpell.AttributesEx4&spellAttr4IgnoreResistances != 0
		petResistInput.binary = petSpell.AttributesCu&spellAttr0CuBinarySpell != 0
		petResistInput.hybrid = petSpell.SchoolMask&spellSchoolMaskNormal != 0 && petSpell.SchoolMask&0x7E != 0
		// Unit.cpp:1786-1789: pets inherit the spell-mod owner's (the
		// calling session is the owner's) 123-term and spell penetration.
		petResistInput.targetResistMod = s.casterTargetResistMod(uint32(schoolMask))
		if s.player != nil && caster.OwnerGUID != 0 && caster.OwnerGUID == s.playerGUID {
			petPenetration = s.player.SpellPenetration
		}
	}
	if damage > 0 && schoolMask&spellSchoolMaskNormal != 0 {
		// Unit::CalculateSpellDamageTaken (Unit.cpp:1002-1006): the
		// physical-school armor arm runs for pet casters too. A null
		// spellInfo runs the arm in C++, so an unknown spell runs it here;
		// the 270 affect gate needs the spell row and is skipped then.
		// Hybrid (melee+magic) spells skip this arm: C++ strips the NORMAL
		// bit at load (SpellMgr.cpp:2830), so IsDamageReducedByArmor is
		// false for them, and the armor comparison moves into
		// CalcSpellResistedDamage (Unit.cpp:1764-1770) — which runs even
		// under CU_IGNORE_ARMOR.
		armorSpell := petSpell
		armorSpellKnown := petSpellKnown
		// The victim's live pet armor fold (Guardian::UpdateArmor,
		// StatSystem.cpp:1316-1330): pet victims fold their aura legs
		// here like the melee/ranged/spell damage paths do.
		armorValue := float64(target.Armor)
		if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, targetGUID); tm != nil {
			armorValue = s.server.liveCreatureArmor(creatureAuraKeyForTarget(target), tm, float64(target.Armor))
		}
		armor := s.server.creatureIgnoreTargetResistArmorForSpell(creatureAuraKeyForMotion(caster), armorValue, armorSpell, armorSpellKnown)
		// Hybrid spells skip this arm (the figure is computed post-crit at
		// the resist call); the hybrid leg runs even under CU_IGNORE_ARMOR.
		if !petResistInput.hybrid && (!armorSpellKnown || armorSpell.Attributes&spellAttr0CUIgnoreArmor == 0) {
			damage = calcArmorReducedDamage(armor, uint8(maxUint32(caster.Level, 1)), target.Level, damage)
		}
	}
	// Unit::SpellCritChanceDone (Unit.cpp:7167-7212) creature arm: pets may
	// crit with spells (the IsNPCBotOrPet exception to the "mobs can't crit"
	// rule). The crit bonus sits in the CalculateSpellDamageTaken DmgClass
	// switch (Unit.cpp:1023-1040), post-armor, pre-resilience — so the roll
	// and bonus land here, ahead of the resist leg and the resilience arm.
	// DoEffectOnLaunchTarget rolls crit per TargetInfo with no self-target
	// exclusion (Spell.cpp:7782-7790), so a reflect return trip (target ==
	// caster) rolls like any other hit.
	isCrit := false
	if damage > 0 && s.server != nil && s.server.Data != nil {
		if critSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
			chance := s.server.creatureSpellCritChanceDone(creatureAuraKeyForMotion(caster), caster.FlagsExtra&0x20000 != 0, schoolMask, critSpell, true)
			// Spell::DoEffectOnLaunchTarget (Spell.cpp:7782-7790): the taken
			// fold runs unconditionally — no done-gate before it.
			// Unit::SpellCritChanceDone (Unit.cpp:7209-7210): the
			// owner's SPELLMOD_CRITICAL_CHANCE folds in via
			// GetSpellModOwner() — the calling session is the pet's
			// owner player.
			if caster.OwnerGUID != 0 && s.player != nil && caster.OwnerGUID == s.playerGUID {
				chance = float64(s.applySpellMod(critSpell, spellModCriticalChance, int32(math.Round(chance*100)))) / 100.0
			}
			if critSpell.DefenseType == spellDamageClassMagic || critSpell.DefenseType == spellDamageClassMelee || critSpell.DefenseType == spellDamageClassRanged {
				// Unit::SpellCritChanceTaken (Unit.cpp:7228-7240):
				// MAGIC takes 179 pre-resilience and 197 post, the
				// resilience crit-chance reduction runs when the pet's
				// owner is a player (CanApplyResilience,
				// Unit.cpp:12333 — the pre-charm owner: a creature-charmed
				// pet resolves to nil and skips the reduction), and
				// MELEE/RANGED take the
				// GetUnitCriticalChanceTaken leg (Unit.cpp:2858-2901).
				// Caster-side scripted arms (Shatter 849/910/911) read
				// the caster's OVERRIDE_CLASS_SCRIPTS auras; pets carry
				// none in the creature aura store, so they stay
				// unbridged.
				var vicSess *session
				if isPlayerVictim {
					vicSess = s.server.findSessionByGUID(target.GUID)
				}
				chance = s.server.creatureSpellCritChanceTaken(chance, vicSess, creatureAuraKeyForTarget(target), isPlayerVictim, caster.GUID, caster.Level, uint32(target.Level), schoolMask, critSpell, s.server.creatureCasterCanApplyResilience(caster))
			}
			isCrit = chance > 0 && rand.Float64() < chance
			if isCrit {
				// Unit::SpellCriticalDamageBonus (Unit.cpp:7411-7449) /
				// the melee/ranged arm (Unit.cpp:1045-1062): the pet's
				// 163/169 crit-damage auras now fold from the creature
				// store (the old nil-session call carried only the base
				// half/double), plus the victim's 203/204 on the
				// melee/ranged arm.
				mask, _ := s.targetCreatureTypeMask(ctx, targetGUID)
				var vicSess *session
				if isPlayerVictim {
					vicSess = s.server.findSessionByGUID(target.GUID)
				}
				// The pet's GetSpellModOwner() is its owner — the calling
				// session — so the owner's SPELLMOD_CRIT_DAMAGE_BONUS folds
				// into both crit-bonus arms.
				if critSpell.DefenseType == spellDamageClassMagic {
					damage = s.server.creatureSpellCriticalDamageBonus(creatureAuraKeyForMotion(caster), uint32(schoolMask), damage, mask, critSpell, s)
				} else {
					damage = s.server.creatureSpellWeaponCritDamageBonus(creatureAuraKeyForMotion(caster), creatureAuraKeyForTarget(target), vicSess, isPlayerVictim, uint32(schoolMask), damage, critSpell, mask, s)
				}
				hitInfo |= 0x02
			}
		}
	}
	// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2527-2535): the
	// IsImmunedToDamage check is the FIRST arm of the damage block -
	// ahead of CalculateSpellDamageTaken (Spell.cpp:2538), so ahead of
	// the block, resilience, resist and absorb legs below. On immune the
	// damage zeroes and NO damage-log packet is sent ("no packet found
	// in sniffs"); DealSpellDamage never runs, so no hooks, strips,
	// leech or shield consumption fire. C++ gates the check on
	// m_damage > 0 - a miss never consults immunity.
	if damage > 0 && petSpellKnown {
		immuneToDmg := false
		if isPlayerVictim {
			if vs := s.server.findSessionByGUID(target.GUID); vs != nil && vs.player != nil {
				immuneToDmg = vs.isImmuneToDamageSpell(petSpell, uint32(schoolMask))
			}
		} else if s.findCreatureMotion(target.GUID) != nil {
			immuneToDmg = creatureImmuneToDamageSpell(s.server, creatureAuraKeyForTarget(target), petSpell, uint32(schoolMask))
		}
		if immuneToDmg {
			// Reflect return trip: C++ still runs the stand-up, hook
			// and trigger legs on an immune hit (DoDamageAndTriggers gates
			// them only on MissCondition != EVADE and a non-null
			// _spellHitTarget) — only the damage pipeline returns nothing.
			if reflectorGUID != 0 {
				s.server.standPetReflectReflectorUp(caster, reflectorGUID, petSpell)
				s.server.fireCreatureSpellHitTarget(ctx, caster, caster.GUID, spellID)
				s.server.fireCreatureHitTriggers(ctx, caster, petSpell, spellID, []uint64{caster.GUID})
			}
			return
		}
	}
	// Unit::CalculateSpellDamageTaken block arm (Unit.cpp:1013-1062) with
	// Unit::isSpellBlocked (Unit.cpp:2412-2430): melee/ranged-DmgClass
	// physical-school spells can be blocked from a pet caster — the block
	// chance and shield value are victim-side. Runs post-crit and
	// pre-resilience, matching the C++ arm order; the block roll and the
	// crit roll are independent Bernoulli trials in C++, so Go's order is
	// damage-exact. A creature victim mid cast-bar blocks (the player
	// path's mid-cast gate has no creature cast-bar model — same delta as
	// the player path's documented gaps).
	if damage > 0 && s.server != nil && s.server.Data != nil {
		if blockSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
			var vicSess *session
			if isPlayerVictim {
				vicSess = s.server.findSessionByGUID(target.GUID)
			}
			if b, ok := s.server.spellTakenBlockAmountFrom(target, isPlayerVictim, vicSess, blockSpell, damage, uint8(maxUint32(caster.Level, 1)), caster.X, caster.Y); ok {
				blocked = b
				damage -= b
			}
		}
	}
	// Unit::CalculateSpellDamageTaken (Unit.cpp:1079-1080): the
	// MELEE/RANGED-DmgClass arm reduces by the melee/ranged taken
	// rating, not the spell one.
	resilCR := uint8(CombatRatingCritTakenSpell)
	if resilSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found && resilSpell.DefenseType != spellDamageClassMagic {
		if spellWeaponAttackType(resilSpell) == protocol.RangedAttack {
			resilCR = CombatRatingCritTakenRanged
		} else {
			resilCR = CombatRatingCritTakenMelee
		}
	}
	// Unit::CalculateSpellDamageTaken (Unit.cpp:1079, 1105): the resilience
	// arm runs only when the caster CanApplyResilience() (Unit.cpp:12333) —
	// the pet's owner must be a player (pre-charm owner) and the pet must
	// not be a vehicle. The absorb leg below is NOT resilience-gated in C++
	// and stays outside the gate.
	casterCanResil := s.server.creatureCasterCanApplyResilience(caster)
	if isPlayerVictim {
		if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil {
			if casterCanResil {
				victim.applyResilienceToDamage(true, &damage, isCrit, resilCR)
			}
		}
	} else if targetMotion := s.findCreatureMotion(target.GUID); targetMotion != nil && s.server.creatureVictimTakesResilience(targetMotion) {
		// Unit::ApplyResilience (Unit.cpp:12342-12350): a creature victim
		// owned by a player takes the owner's damage resilience — same
		// crit + damage reduction arms as the player-victim call above,
		// still behind the caster's CanApplyResilience gate. A non-player
		// vehicle victim takes none (Unit.cpp:12342-12343).
		if ownerSess := s.server.ownerPlayerSessionOfMotion(targetMotion); ownerSess != nil && casterCanResil {
			ownerSess.applyResilienceToDamage(true, &damage, isCrit, resilCR)
		}
	}
	preResistDmg := damage
	if damage > 0 && schoolMask > 1 {
		// AuraEffect::HandleAuraModResistance (SpellAuraEffects.cpp:3255):
		// the live 22 flat amounts fold into a pet victim's resistances.
		resistances := s.server.petFoldedResistances(creatureAuraKeyForTarget(target), target.Resistances)
		// Unit::CalcSpellResistedDamage (Unit.cpp:1764-1770): the hybrid
		// armor figure is evaluated on the CalcAbsorbResist entry damage
		// (post-resilience here), like the C++ leg inside DealDamage.
		if petResistInput.hybrid {
			armorValue := float64(target.Armor)
			if tm := s.server.findCreatureMotion(target.Map, target.InstanceID, targetGUID); tm != nil {
				armorValue = s.server.liveCreatureArmor(creatureAuraKeyForTarget(target), tm, float64(target.Armor))
			}
			armor := s.server.creatureIgnoreTargetResistArmorForSpell(creatureAuraKeyForMotion(caster), armorValue, petSpell, petSpellKnown)
			petResistInput.hybridArmorReduced = calcArmorReducedDamage(armor, uint8(maxUint32(caster.Level, 1)), target.Level, damage)
		}
		resisted, damage = calcMagicSpellResistance(damage, schoolMask, resistances, uint8(maxUint32(caster.Level, 1)), target.Level, !isPlayerVictim, false, 0, petResistInput, petPenetration)
	}
	if isPlayerVictim {
		if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil && damage > 0 {
			// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): max(194 misc-masked pct, 245 ability pct affecting the cast spell) of the damage bypasses the victim's shields. C++ takes the pct on the pre-resist entry damage (:1853) and re-adds it after the absorb loops (:1975); the pool is the post-resist damage less the bypass, floored at zero (ModifyDamage clamp, Unit.cpp:194-196).
			bypass := absorbIgnoreBypass(preResistDmg, s.server.creatureAbsorbIgnorePctForSpell(creatureAuraKeyForMotion(caster), uint32(schoolMask), spellID))
			pool := damage
			if bypass > pool {
				pool = 0
			} else {
				pool -= bypass
			}
			absorbed, damage = victim.applyAbsorptionShields(pool, schoolMask)
			damage += bypass
		}
	}
	if !isPlayerVictim && damage > 0 {
		// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): shield absorbs on a
		// creature victim soak pet spell damage, matching the player
		// funnel's creature arm (spells.go:15360); the attacker's 194/245
		// bypass pct folds the pet's creature aura key.
		bypass := absorbIgnoreBypass(preResistDmg, s.server.creatureAbsorbIgnorePctForSpell(creatureAuraKeyForMotion(caster), uint32(schoolMask), spellID))
		pool := damage
		if bypass > pool {
			pool = 0
		} else {
			pool -= bypass
		}
		absorbed, damage = s.server.applyCreatureAbsorptionShields(creatureAuraKeyForTarget(target), pool, schoolMask)
		damage += bypass
	}
	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	// Spell::EffectHealthLeech leech basis (SpellEffects.cpp:1539-1540):
	// the damage the target actually lost — post-absorb, overkill
	// excluded — captured before the damage legs mutate health; the
	// caster-heal arm lands at the end of this function with the other
	// cast-completion legs, mirroring the inline twin
	// (creature_area_targets.go). The basis is the pre-DealDamage
	// damage: the C++ effect runs in the HIT_TARGET phase
	// (Spell.cpp:2909), ahead of DealDamage's CHEAT_GOD return
	// (Unit.cpp:735-737), so a god-cheat victim still feeds the heal —
	// the damage log below likewise carries the pre-negation amount.
	leechDealt := damage - overkill
	logPacket := buildSpellNonMeleeDamageLog(target.GUID, caster.GUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo, blocked)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPacket, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPacket, s)
	if damage == 0 {
		// A missed or deflected hit never reaches Unit::DealDamage in C++
		// (hitInfo 0x01 returns above). A zero-damage hit that reaches the
		// damage block — fully absorbed, or fully resisted inside
		// CalcSpellResistedDamage (Unit.cpp:1704-1770 can roll 10/10) —
		// still runs DealDamage's pre-return legs (Spell.cpp:2548 calls
		// DealSpellDamage on the pre-mitigation damage>0 arm): the AI
		// hooks and the TAKE_DAMAGE strip fire; only tap, threat and
		// engage are skipped at the !damage return (Unit.cpp:814-818).
		// The AI hooks arm (Unit.cpp:700-704) is C++-true here too and fires
		// ahead of the god return and the strips — the player arm fires it
		// like the creature arm below.
		if hitInfo&0x01 != 0 {
			return
		}
		if isPlayerVictim {
			// Unit::DealDamage (Unit.cpp:700-704): the DamageTaken/DamageDealt
			// AI hooks fire at the top of DealDamage on every non-miss hit —
			// ahead of the CHEAT_GOD return (Unit.cpp:735-737) and the strip
			// arms (Unit.cpp:738+). The victim is a player (no BossAI leg)
			// and the pet motion carries no BossAI, so both legs are
			// documented no-fires today (pet_combat.go convention); the call
			// holds the hook point like the creature arm below.
			if s.server != nil && caster != nil {
				s.server.motionMu.Lock()
				fireDealDamageAIHooks(ctx, s.server, nil, caster, caster.GUID, target.GUID, &damage)
				s.server.motionMu.Unlock()
			}
			// Unit::DealDamage (Unit.cpp:739-747): the TAKE_DAMAGE-interrupt
			// strip runs before the !damage early-return — a fully resisted
			// or absorbed hit strips too (Spell.cpp:2548 calls
			// DealSpellDamage on the pre-mitigation damage>0 arm). Like the
			// non-zero arm's procSpellDirectDamageAuras twin, the strip is
			// skipped for SPELL_ATTR4_DAMAGE_DOESNT_BREAK_AURAS spells and
			// excludes the damaging spell's own auras; the DIRECT_DAMAGE
			// strip (Unit.cpp:897-898, past the !damage return) does not
			// run here. The HITBYSPELL strip DOES run here:
			// Spell::PreprocessSpellHit (Spell.cpp:2743-2744) strips it at
			// spell-hit time on every non-miss hit — including fully
			// absorbed/resisted ones — and the pet funnel never models
			// the hit arm, so the damage-time path is its only coverage
			// (ungated, no exception, like C++). The absorbed-gated legs
			// below (Unit.cpp:749-761, 814-818: ABORT_ON_DMG abort and
			// absorbed rage, both keyed on cleanDamage->absorbed_damage)
			// stay gated; the god arm above returns before all of these
			// in C++.
			if s.server != nil {
				if victim := s.server.findSessionByGUID(target.GUID); victim != nil && victim.player != nil && !victim.godCheatActive() {
					if tickDamageBreaksAuras(s.server, spellID) {
						victim.removeAurasWithInterruptFlags(auraInterruptFlagTakeDamage, spellID)
					}
					victim.removeAurasWithInterruptFlags(auraInterruptFlagHitBySpell)
					if absorbed > 0 {
						victim.grantRageFromDamageTaken(ctx, absorbed)
						victim.interruptAbsorbedCast()
					}
				}
			}
			return
		}
		// Unit::DealDamage (Unit.cpp:700-703): the DamageTaken/DamageDealt AI
		// hooks fire at the top of DealDamage on every non-miss hit — a fully
		// resisted zero-damage hit notifies the AI too, not just absorbed
		// ones (Spell.cpp:2548 calls DealSpellDamage on the pre-mitigation
		// damage>0 arm).
		// The pre-return legs (Unit.cpp:814-818) then grant absorbed rage
		// (absorbed-gated, like C++'s cleanDamage->absorbed_damage check) and
		// return before the tap, threat and engage legs, like C++.
		var tm *creatureMotion
		if s.server != nil {
			s.server.motionMu.Lock()
			if m := s.server.findCreatureMotionLocked(target.Map, target.InstanceID, target.GUID); m != nil {
				tm = m
				if tm.BossAI == nil {
					tm.BossAI = getBossAIForCreature(tm, tm.ScriptName)
				}
				fireDealDamageAIHooks(ctx, s.server, tm, caster, caster.GUID, tm.GUID, &damage)
				if absorbed > 0 {
					if next, changed := s.server.addCreatureRageLocked(tm, tm.Level, absorbed); changed {
						s.server.broadcastCreatureValuesUpdateInInstance(tm.Map, tm.InstanceID, tm.GUID, map[int]uint32{unitFieldPower1 + powerRage: next})
					}
				}
			}
			s.server.motionMu.Unlock()
			// Unit::DealDamage (Unit.cpp:739-747): the TAKE_DAMAGE strip runs
			// before the !damage early-return for any non-NODAMAGE hit — a
			// fully resisted or absorbed pet-spell hit strips the creature
			// victim's TAKE_DAMAGE auras too, attr4-gated and excepting the
			// hitting spell's own aura. Runs after the unlock: the aura
			// remove path re-resolves the motion (findCreatureMotion) and
			// must not run under motionMu (damage_shield.go convention). The
			// DIRECT_DAMAGE strip (Unit.cpp:897-898) lives past the !damage
			// return and does not run on the zero arm. The HITBYSPELL strip
			// does: Spell::PreprocessSpellHit (Spell.cpp:2743-2744) strips it
			// at hit time on every non-miss spell hit and the pet funnel
			// never models the hit arm (ungated, no exception, like C++).
			if tm != nil {
				if tickDamageBreaksAuras(s.server, spellID) {
					s.server.removeCreatureAurasWithInterruptFlags(tm, auraInterruptFlagTakeDamage, spellID)
				}
				s.server.removeCreatureAurasWithInterruptFlags(tm, auraInterruptFlagHitBySpell)
			}
			// Reflect return trip, fully mitigated: C++ still runs the
			// DoDamageAndTriggers tail (Spell.cpp:2581-2652) — the damage
			// proc loop (m_damage > 0 pre-mitigation, so DAMAGE type with a
			// zero post-mitigation amount), the reflector stand-up, the
			// SpellHitTarget hook with the pet as _spellHitTarget, and the
			// per-hit trigger consumer. The leech leg needs no arm: a
			// zero-damage trip has a zero leech basis.
			if reflectorGUID != 0 && tm != nil && tm.Health != 0 {
				if hitSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
					if hitSpell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
						typeMask := spellDamageProcTypeMask(hitSpell) | spellDamageTakenProcTypeMask(hitSpell) | procFlagTakenDamage
						if typeMask != 0 {
							spellCopy := hitSpell
							s.server.fireCreatureAuraProcLoop(ctx, caster, caster.GUID, procEventInfo{
								typeMask:       typeMask,
								schoolMask:     uint32(schoolMask),
								spellTypeMask:  procSpellTypeDamage,
								spellPhaseMask: procSpellPhaseHit,
								hitMask:        procHitNormal,
								triggered:      triggered,
								eventSpell:     &spellCopy,
								actorGUID:      caster.GUID,
								damage:         0,
							})
						}
					}
					s.server.standPetReflectReflectorUp(caster, reflectorGUID, hitSpell)
					s.server.fireCreatureSpellHitTarget(ctx, caster, caster.GUID, spellID)
					s.server.fireCreatureHitTriggers(ctx, caster, hitSpell, spellID, []uint64{caster.GUID})
				}
			}
		}
		return
	}
	if isPlayerVictim {
		victim := s.server.findSessionByGUID(target.GUID)
		if victim == nil || victim.player == nil {
			return
		}
		// Unit::DealDamage (Unit.cpp:700-704): the DamageTaken/DamageDealt AI
		// hooks fire at the top of DealDamage on every non-miss hit, ahead of
		// the CHEAT_GOD return (Unit.cpp:735-737). Same documented-no-fire
		// legs as the zero arm (player victim, pet motion with no BossAI).
		if s.server != nil && caster != nil {
			s.server.motionMu.Lock()
			fireDealDamageAIHooks(ctx, s.server, nil, caster, caster.GUID, target.GUID, &damage)
			s.server.motionMu.Unlock()
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
		// the mask is NORMAL or CRIT; pet spell crits roll at the
		// taken-leg site above (Unit::SpellCritChanceDone, Unit.cpp:7167).
		if victim.procDeep == 0 {
			victim.procSpellHitTakenAuraTriggers(ctx, caster.GUID, spellID, true, false, false, false, isCrit, absorbed, procEventDamage, triggered, false, 0)
		}
		// The pet caster's done-side leg (DoDamageAndTriggers, Spell.cpp:2581)
		// runs after the taken side on both the lethal and non-lethal arms.
		s.firePetSpellDoneSideProcs(ctx, caster, target.GUID, spellID, schoolMask, triggered, isCrit)
		// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617): a
		// non-friendly non-positive (or dispel-effect) spell hit stands
		// the target up. The EVADE-miss arm is vacuous — misses zero the
		// damage above and return early, so every hit reaching this site
		// is a non-evade hit — and the friendly gate rides the path (pet
		// spells target hostiles), matching the inline twin
		// (creature_area_targets.go:441-447).
		//
		// Spell::TargetInfo::DoDamageAndTriggers AI legs (Spell.cpp:2639-
		// 2650): the caster-side SpellHitTarget Eluna hook fires for pet
		// casters too — the pet is a creature motion with AI hooks, a
		// third hit site alongside castCreatureSpellInternal and the
		// inline path.
		//
		// Spell::DoTriggersOnSpellHit (Spell.cpp:2652/2913-2957): the
		// per-hit trigger consumer — the ADD_TARGET_TRIGGER snapshot
		// plus the spell_linked type-1 rows — runs per hit target after
		// the damage and proc legs, ahead of the _cast tail, so it fires
		// here on the pet path too.
		if hitSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
			if victim.player.StandState != 0 && (!spellIsPositive(hitSpell) || spellHasEffect(hitSpell, spellEffectDispel)) {
				victim.player.StandState = 0
				victim.sendPlayerUpdate()
			}
			s.server.fireCreatureSpellHitTarget(ctx, caster, target.GUID, spellID)
			s.server.fireCreatureHitTriggers(ctx, caster, hitSpell, spellID, []uint64{target.GUID})
		}
		// Spell::EffectHealthLeech (SpellEffects.cpp:1513-1553) for a pet
		// caster: the caster-heal arm for this hit's leech basis, at the
		// cast-completion position like the inline twin
		// (creature_area_targets.go:484-519).
		s.server.applyPetHealthLeechHeal(caster, spellID, schoolMask, leechDealt)
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
	// ThreatManager::AddThreat step-1 gate (ThreatManager.cpp:311-317):
	// probed pre-lock for the motion==nil tail path (conservative:
	// NO_INITIAL_AGGRO gates here) and refined in the threat block
	// below with the true pre-damage engagement state.
	threatGated := s.spellDamageStep1Gate(spellID, false)
	// Reflect return trip: EffectHealthLeech runs in the effect dispatch
	// (DoProcessTargetContainer, Spell.cpp:3555-3566) ahead of
	// DoDamageAndTriggers, so the pet heals for the damage it is about
	// to take — before the health mutation below, not as a tail. That
	// order is observable here because caster == victim: a leech heal
	// can keep a would-be-lethal trip non-lethal, and the MaxHealth cap
	// applies to the pre-damage health. The basis is the direct-hit
	// leechDealt (post-absorb damage actually lost, overkill excluded);
	// the tail leech call below is skipped when this pre-heal ran.
	leechPreHealed := false
	if reflectorGUID != 0 && leechDealt > 0 {
		s.server.applyPetHealthLeechHeal(caster, spellID, schoolMask, leechDealt)
		leechPreHealed = true
	}
	s.server.motionMu.Lock()
	targetMotion := s.server.findCreatureMotionLocked(target.Map, target.InstanceID, target.GUID)
	newlyTapped := false
	rageChanged := false
	var rageNext uint32
	if targetMotion != nil {
		if targetMotion.BossAI == nil {
			targetMotion.BossAI = getBossAIForCreature(targetMotion, targetMotion.ScriptName)
		}
		// Unit::DealDamage (Unit.cpp:701-704): pet spell damage fires the
		// victim's DamageTaken like any attacker; the pet motion carries
		// no BossAI, so the DamageDealt leg is a documented no-fire (C++
		// PetAI::DamageDealt is a base no-op).
		fireDealDamageAIHooks(ctx, s.server, targetMotion, caster, caster.GUID, targetMotion.GUID, &damage)
		// Unit::DealDamage tap block (Unit.cpp:872-876): pet spell
		// damage taps for the owner (GetCharmerOrOwnerPlayerOrPlayerItself).
		tapPlayer := s.playerGUID
		if caster != nil && caster.OwnerGUID != 0 {
			tapPlayer = caster.OwnerGUID
		}
		newlyTapped = s.server.recordCreatureTap(targetMotion, tapPlayer, s.groupID, damage, targetMotion.Health, true)
		if damage >= targetMotion.Health {
			targetMotion.Health = 0
			targetMotion.DynamicFlags |= unitDynFlagLootable
			targetMotion.InCombat = false
			targetMotion.TargetGUID = 0
			targetMotion.Moving = false
			if targetMotion.ThreatMgr != nil {
				targetMotion.ThreatMgr.ClearThreat()
			}
			// ThreatManager::ClearAllThreat always sends SendClearAllThreatToClients
			// (ThreatManager.cpp:483-490) — every other death path pairs the
			// ClearThreat with the SMSG_THREAT_CLEAR broadcast; the pet-spell
			// kill path omitted it, leaving client threat UIs stale.
			s.server.broadcastThreatClearInInstance(targetMotion.Map, targetMotion.InstanceID, targetMotion.GUID)
		} else {
			targetMotion.Health -= damage
			// Unit::DealDamage's threat add runs on the victim's pre-damage
			// engagement state for the step-1 gate (ThreatManager.cpp:315-317):
			// capture it before the funnel's own engagement set.
			wasInCombat := targetMotion.InCombat
			if targetMotion.ThreatMgr == nil {
				targetMotion.ThreatMgr = NewThreatManager(targetMotion)
			}
			dist := distance3D(caster.X, caster.Y, caster.Z, targetMotion.X, targetMotion.Y, targetMotion.Z)
			inMelee := inMeleeThreatRange(targetMotion.CombatReach, caster.CombatReach, dist)
			// Unit::DealDamage (Unit.cpp:906) calls AddThreat with the
			// default args (ignoreModifiers=false, ignoreRedirects=false):
			// the amount folds the spell_threat-row pctMod, the attacker's
			// SPELLMOD_THREAT mods and school multipliers
			// (ThreatManager::CalculateModifiedThreat, ThreatManager.cpp:606),
			// and the caster's redirect registry applies on top
			// (splitThreatRedirects).
			threat, step1Gate := s.damageThreatAmount(ctx, spellID, uint32(schoolMask), float32(damage), wasInCombat)
			// ThreatManager::AddThreat step 1 (ThreatManager.cpp:311-317)
			// returns before SetInCombatWith: a gated spell neither adds
			// threat nor engages the victim. Every other arm — including
			// zero-damage hits — engages: "threat implies combat"
			// (ThreatManager.cpp:366-368).
			threatGated = step1Gate
			if !step1Gate {
				targetMotion.InCombat = true
			}
			// The threat arm is skipped for units that cannot have a threat
			// list (ThreatManager::AddThreat's !CanHaveThreatList() early leg,
			// ThreatManager.cpp:328-339): no redirect consumption, no victim
			// switch — combat state alone is kept. A zero amount still runs
			// AddThreat (the ref is created with 0 threat and the victim leg
			// runs — Unit::EngageWithTarget seeds 0.0f the same way); only the
			// redirect split is skipped (amount > 0.0f gate, ThreatManager.cpp:348).
			if !step1Gate && targetMotion.ThreatMgr.OwnerCanHaveThreatList() {
				var switched bool
				var newVictim uint64
				if threat > 0 {
					var rSwitched bool
					var rVictim uint64
					threat, rSwitched, rVictim = s.splitThreatRedirects(targetMotion, threat)
					switched, newVictim = targetMotion.ThreatMgr.AddThreat(caster.GUID, threat, inMelee)
					if rSwitched {
						switched, newVictim = true, rVictim
					}
				} else {
					switched, newVictim = targetMotion.ThreatMgr.AddThreat(caster.GUID, 0, inMelee)
				}
				if switched && newVictim != targetMotion.TargetGUID {
					targetMotion.TargetGUID = newVictim
					entries := targetMotion.ThreatMgr.SortedEntries()
					s.server.broadcastHighestThreatUpdateInInstance(targetMotion.Map, targetMotion.InstanceID, targetMotion.GUID, newVictim, entries)
				} else {
					targetMotion.TargetGUID = targetMotion.ThreatMgr.GetCurrentVictim()
				}
			}
			consumeKillCommandStack(caster)
			// Unit::DealDamage (Unit.cpp:915-924): rage from damage received;
			// rage_damage folds the absorbed amount (Unit.cpp:919).
			if next, changed := s.server.addCreatureRageLocked(targetMotion, targetMotion.Level, damage+absorbed); changed {
				rageNext, rageChanged = next, true
			}
		}
	}
	s.server.motionMu.Unlock()
	if targetMotion == nil {
		return
	}
	if targetMotion.Health == 0 {
		// Unit::DealDamage (Unit.cpp:738-748 + :882-889): the TAKE_DAMAGE
		// strip fires before the lethal check, so the lethal arm strips it
		// too — attr4-gated, excepting the hitting spell's aura, plus the
		// HITBYSPELL strip (Spell.cpp:2743-2744, every non-miss spell hit).
		// The DIRECT_DAMAGE strip (Unit.cpp:897) sits inside the non-lethal
		// else and never fires on a lethal hit — Kill's death cleanup owns
		// those auras — so the lethal arm must not run it.
		s.server.procCreatureLethalDamageAuras(creatureAuraKeyForMotion(targetMotion), spellID, damage, targetMotion.MaxHealth)
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
		// Unit::DealDamage (Unit.cpp:743-748 + 897-898): the TAKE_DAMAGE
		// strip (attr4-gated, excepting the hitting spell's aura) and the
		// DIRECT_DAMAGE strip, plus the breakable-CC drain — the same twin
		// the player-cast path runs at spells.go:15784. The zero-damage arm
		// above runs only the TAKE_DAMAGE half.
		s.server.procCreatureDirectDamageAuras(creatureAuraKeyForMotion(targetMotion), spellID, damage, targetMotion.MaxHealth)
		// The step-1 gate skips victim engagement
		// (ThreatManager.cpp:311-317) — without this guard the tail
		// aggro call would re-engage a victim C++ leaves unengaged.
		if !threatGated {
			s.server.triggerCreatureAggro(ctx, targetMotion.GUID, caster.OwnerGUID)
		}
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
	// on both the lethal and non-lethal arms, the hit mask follows the crit roll
	// (misses return early above), the attr3 arm checked inline (the
	// creature loop has no attr3 gate), and the trigger spell targeting
	// the pet attacker. The self-hit is excluded — the return-trip
	// bridge below already fires the combined done+taken event there.
	procHitMask := procHitNormal
	if isCrit {
		procHitMask = procHitCritical
	}
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
					hitMask:        procHitMask,
					triggered:      triggered,
					eventSpell:     &spellCopy,
					actorGUID:      caster.GUID,
					damage:         damage,
				})
			}
		}
	}
	s.firePetSpellDoneSideProcs(ctx, caster, target.GUID, spellID, schoolMask, triggered, isCrit)
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
		returnTripHitMask := procHitNormal
		if isCrit {
			returnTripHitMask = procHitCritical
		}
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found && spell.AttributesEx3&spellAttr3CantTriggerProc == 0 {
			typeMask := spellDamageProcTypeMask(spell) | spellDamageTakenProcTypeMask(spell) | procFlagTakenDamage
			if typeMask != 0 {
				spellCopy := spell
				s.server.fireCreatureAuraProcLoop(ctx, caster, caster.GUID, procEventInfo{
					typeMask:       typeMask,
					schoolMask:     uint32(schoolMask),
					spellTypeMask:  procSpellTypeDamage,
					spellPhaseMask: procSpellPhaseHit,
					hitMask:        returnTripHitMask,
					triggered:      triggered,
					eventSpell:     &spellCopy,
					actorGUID:      caster.GUID,
					damage:         damage,
				})
			}
		}
	}
	// Spell::TargetInfo::DoDamageAndTriggers AI legs (Spell.cpp:2629-
	// 2650) + Spell::DoTriggersOnSpellHit (Spell.cpp:2652/2913-2957) on
	// the creature-victim arm: the Eluna SpellHit hook (2629-2638, on the
	// victim creature's AI — the _spellHitTarget->ToCreature() gate, which
	// the creature-victim arm satisfies by construction) ahead of the
	// SpellHitTarget hook (2639-2650), then the per-hit trigger consumer,
	// mirroring the player-victim arm above. Misses return early above
	// (hitInfo 0x01), so every hit reaching this site is a non-miss, the
	// C++ _spellHitTarget-null arm. The stand-up leg (Spell.cpp:2609-2617)
	// runs inline below for the creature victim via standCreatureMotionUp;
	// the player-victim arm stands players above (the inline twin stands
	// players only, creature_area_targets.go:441-447).
	if hitSpell, found, perr := s.server.Data.Spell(spellID); perr == nil && found {
		// Spell::TargetInfo::DoDamageAndTriggers (Spell.cpp:2609-2617): a
		// non-friendly non-positive (or dispel-effect) spell hit stands
		// the target up — ahead of the AI legs below, matching C++ order
		// (2609 precedes 2629). On a reflect return trip the stood unit
		// is the reflector (TargetInfo::unit, resolved from TargetGUID),
		// not the pet _spellHitTarget — the helper carries the C++ gates
		// for that case. Misses return early above (damage == 0).
		if reflectorGUID != 0 {
			s.server.standPetReflectReflectorUp(caster, reflectorGUID, hitSpell)
		} else if !spellIsPositive(hitSpell) || spellHasEffect(hitSpell, spellEffectDispel) {
			if target.GUID != 0 && !s.server.creatureFriendlyToTarget(caster, target.GUID) {
				s.server.standCreatureMotionUp(targetMotion)
			}
		}
		s.server.fireCreatureLuaEvent(ctx, targetMotion, scripting.CreatureEventOnHitBySpell, s.server.luaMotionCreature(caster), spellID)
		s.server.fireCreatureSpellHitTarget(ctx, caster, target.GUID, spellID)
		s.server.fireCreatureHitTriggers(ctx, caster, hitSpell, spellID, []uint64{target.GUID})
	}
	// Spell::EffectHealthLeech (SpellEffects.cpp:1513-1553) for a pet
	// caster on the creature-victim arm: the caster-heal arm for this
	// hit's leech basis, at the cast-completion position like the
	// player-victim arm above and the inline twin. Skipped on a return
	// trip: the pre-heal above already healed for this hit.
	if !leechPreHealed {
		s.server.applyPetHealthLeechHeal(caster, spellID, schoolMask, leechDealt)
	}
}
