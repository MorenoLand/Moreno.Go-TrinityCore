package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type playerPos struct {
	Map        uint32
	InstanceID uint32
	X          float32
	Y          float32
	Z          float32
	GUID       uint64
	Race       uint8
	Class      uint8
	Level      uint8
	IsGM       bool
	// SeesHidden mirrors the SERVERSIDE_VISIBILITY_GM detect arm
	// (Object.cpp:1622): false for a GM after `.gm visible on`, even though
	// IsGM stays true — hidden creatures stay out of the motion set.
	SeesHidden      bool
	IsDead          bool
	FactionTemplate uint32
	Reputations     map[uint32]playerReputation
	Sess            *session
}

type creatureMotionSpawnKey struct {
	MapID      uint32
	InstanceID uint32
	GUID       uint32
}

// creatureMotion tracks live server-side creature movement state, the role
// TrinityCore's MotionMaster fills: home position (for random wander around
// spawn), current position, waypoint path/point and the next move deadline.
type creatureMotion struct {
	GUID          uint64
	Entry         uint32
	Map           uint32
	InstanceID    uint32
	TransportGUID uint64
	HomeX         float32
	HomeY         float32
	HomeZ         float32
	X             float32
	Y             float32
	Z             float32
	Orientation   float32
	Speed         float32 // yd/s walk speed used for wander
	RunSpeed      float32 // yd/s run speed used for pursuit
	MoveType      uint32  // 1 random, 2 waypoint; 0 = IDLE_MOTION_TYPE (MovementDefines.h:28-30)
	Wander        float64
	// WanderSteps is the remaining _wanderSteps of RandomMovementGenerator
	// (RandomMovementGenerator.cpp:DoInitialize): the creature walks
	// urand(2,10) splines, then pauses urand(4,10) seconds after the last one.
	WanderSteps int

	Faction         uint32
	Level           uint32
	UnitFlags       uint32
	DynamicFlags    uint32
	FlagsExtra      uint32
	TypeFlags       uint32 // creature_template.type_flags (CreatureData.h:180); the BOSS_MOB bit (0x4, SharedDefines.h:2731) feeds Creature::isWorldBoss (Creature.cpp:2353) gates
	CanFly          bool
	ReactState      uint8
	ReactStateKnown bool
	// CreatureType mirrors creature_template.type (8 = critter,
	// CritterAI::Permissible, PassiveAI.cpp:95-100).
	CreatureType uint32
	// AIName mirrors creature_template.AIName (TurretAI, CombatAI.cpp:231-265).
	AIName string
	// FleeingUntil is the CritterAI flee timer (TimedFleeingMovementGenerator,
	// CONFIG_CREATURE_FAMILY_FLEE_DELAY 7000ms, Unit.cpp:11712): while in the
	// future the critter is fleeing and the step tick skips combat/wander;
	// on expiry it evades home (CritterAI::OnMovementGeneratorFinalized,
	// PassiveAI.cpp:81-86). Zero when not fleeing.
	FleeingUntil time.Time
	AttackTime   uint32
	CombatReach  float32

	Armor       uint32
	Resistances [7]uint32
	MinDamage   float32
	MaxDamage   float32

	Health          uint32
	MaxHealth       uint32
	Mana            uint32
	MaxMana         uint32
	PowerType       uint32
	Powers          [7]uint32
	MaxPowers       [7]uint32
	Stats           [5]uint32
	PetID           uint32
	PetType         uint8
	PetNextLevelXP  uint32
	UnitFlags2      uint32
	FocusRegenTimer time.Duration
	HappinessTimer  time.Duration
	Happiness       uint32
	Experience      uint32

	TargetGUID uint64
	InCombat   bool
	LastAttack time.Time
	// LastDamaged mirrors Creature::m_lastDamagedTime as stamped by the
	// evade arm of Unit::DealDamage (Unit.cpp:900-907): direct (melee,
	// ranged, spell-direct) hits with damage > 0 reset it to now +
	// MAX_AGGRO_RESET_TIME (10s, Unit.h:40); DoT ticks never stamp it.
	// Creature::CanCreatureAttack (Creature.cpp:2560-2603) skips the
	// home-distance check while it is fresh (or while the creature is
	// taunted) — except for world bosses (isWorldBoss, Creature.cpp:2353).
	LastDamaged time.Time
	// LastSanctuaryTime mirrors Unit::m_lastSanctuaryTime (Unit.h:1459),
	// stamped by Spell::EffectSanctuary (SpellEffects.cpp:3778): in-flight
	// (delayed) non-positive missiles launched at or before this time
	// fizzle on this creature (DoTargetSpellHit, Spell.cpp:2403). Game ms
	// from gameTimeMS; zero means no sanctuary recorded.
	LastSanctuaryTime uint32
	LastSpell         time.Time
	Spells            []uint32
	NextSpellIdx      int
	// SpellEventTimes is the per-spell event schedule for CombatAI/CasterAI
	// template-spell casts (CombatAI.cpp _events EventMap): each AICOND_COMBAT
	// template spell's next fire time. Armed at engage
	// (CombatAI::JustEngagedWith / CasterAI::JustEngagedWith), fired and
	// re-armed by the combat tick (CombatAI::UpdateAI / CasterAI::UpdateAI),
	// reset on evade (CombatAI::Reset).
	SpellEventTimes map[uint32]time.Time
	// CastingUntil bridges the UNIT_STATE_CASTING window set by
	// Unit::CastSpell for non-triggered casts: CombatAI::UpdateAI and
	// CasterAI::UpdateAI (CombatAI.cpp:97, 159) skip both the spell event
	// and the melee swing while it is armed. Stamped at cast start to
	// now + the DBC cast time; cleared by the CasterAI breakable-CC
	// interrupt arm and on evade (CreatureAI::_EnterEvadeMode ->
	// Unit::CombatStop(true) -> InterruptNonMeleeSpells(false), Unit.cpp
	// 5809-5812). Triggered casts never set it (Unit::CastSpell with
	// triggered=true skips UNIT_STATE_CASTING); the AICOND_DIE death cast
	// stamps it only on an already-dead motion, which the tick's
	// Health==0 gate never reaches. No SMSG_SPELL_START is modeled — Go
	// creature casts stay packet-only SPELL_GO; this field is the
	// scheduling gate only.
	CastingUntil              time.Time
	SpellCooldowns            map[uint32]time.Time
	SpellCategoryCooldowns    map[uint32]time.Time
	SpellCooldownCategories   map[uint32]uint32
	SpellCooldownCategoryEnds map[uint32]time.Time

	ThreatMgr           *ThreatManager
	BossAI              BossAI
	ScriptName          string
	Name                string
	EncounterMapID      uint32
	EncounterInstanceID uint32

	OwnerGUID      uint64
	CharmerGUID    uint64
	Charmed        bool
	CharmUnitFlags uint32
	CharmFaction   uint32
	CharmOwnerGUID uint64
	PetCommand     uint8 // 0: stay, 1: follow, 2: attack

	// Looted mirrors Loot::isLooted (Loot.h:236 — gold == 0 && unlootedCount
	// == 0): set when the corpse's loot window fully empties, cleared on
	// respawn. Consulted by the skinning CheckCast gate (spells.go).
	Looted bool
	// TapPlayerGUID/TapGroupID mirror Creature::SetLootRecipient
	// (Creature.cpp:1328-1355): the first player-attributed damager
	// (charmer/owner-resolved, so pet hits credit the owner) claims the
	// loot rights at first hit (Unit::DealDamage, Unit.cpp:872-876), not
	// at kill time; the UNIT_DYNFLAG_TAPPED flag rides with the tap and
	// the group half feeds the group loot-permission check.
	TapPlayerGUID uint64
	TapGroupID    uint64
	// PlayerDamageReq mirrors Creature::m_PlayerDamageReq
	// (Creature.h:322-325): MaxHealth/2 at spawn, lowered by every
	// player-attributed hit (Creature::LowerPlayerDamageReq,
	// Creature.cpp:1577-1582, capped at the pre-hit health); kill
	// rewards require it at 0 (IsDamageEnoughForLootingAndReward).
	PlayerDamageReq uint32
	PetReact        uint8 // 0: passive, 1: defensive, 2: aggressive
	AutocastSpells  []uint32

	PathID  uint32
	Points  []waypointPoint
	NextIdx int

	Moving   bool
	Evading  bool
	MoveEnds time.Time
	// ChaseTX/ChaseTY record the victim's position when the combat pursuit
	// spline launched — ChaseMovementGenerator's _lastTargetPosition
	// (ChaseMovementGenerator.h): the chase re-paths whenever the target
	// moves, so a spline whose victim left the launch point is relaunched.
	// Zero when no pursuit spline is armed.
	ChaseTX   float32
	ChaseTY   float32
	WaitUntil time.Time
	Refreshed time.Time
	// DistractedUntil is the DistractMovementGenerator hold
	// (Spell::EffectDistract, SpellEffects.cpp:2547): the creature turned to
	// face the distraction and pauses wandering until this time. Zero when
	// not distracted.
	DistractedUntil         time.Time
	FormationLeadGUID       uint64    // leader world GUID once LeaderStartedMoving pulls this member into formation (CreatureGroups.cpp:280)
	FormationDist           float32   // formation slot distance (creature_formations.dist)
	FormationAngle          float32   // formation slot angle (FollowAngle + pi; LeaderStartedMoving inverts the DB angle)
	FormationPoint1         uint32    // leader waypoint id that mirrors the slot angle (creature_formations.point_1)
	FormationPoint2         uint32    // second leader waypoint id that mirrors the slot angle (creature_formations.point_2)
	FormationActive         bool      // member moves on the leader's spline rhythm; suppresses its own wander/waypoint legs
	FormationPredicted      bool      // _hasPredictedDestination: member launched against a moving leader
	FormationLeaderMoveEnds time.Time // leader MoveEnds of the spline the member last launched against (movespline GetId analog)
	FormationNextCheck      time.Time // next FORMATION_MOVEMENT_INTERVAL (1200ms) re-check
	FormationLeaderX        float32   // leader X at last re-check (DoUpdate's _lastLeaderPosition)
	FormationLeaderY        float32   // leader Y at last re-check
	MoveHeading             float32   // travel heading of the current leg, recorded at launch (FormationMovementGenerator's relativeAngle)
	MoveVelocity            float32   // speed of the current leg, recorded at launch (movespline Velocity analog for catchup)
}

type waypointPoint struct {
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	MoveType    uint32
	Delay       uint32
}

// waypointMotionType mirrors WAYPOINT_MOTION_TYPE (MovementDefines.h:30),
// the type id Eluna::MovementInform reports for waypoint arrivals.
const waypointMotionType = 2

// motionTTL bounds how long idle state survives between nearby sweeps so a
// creature nobody observes stops consuming memory.
const motionTTL = 10 * time.Minute

const (
	creatureBaseWalkSpeed   = 2.5
	creatureBaseRunSpeed    = 7.0
	creatureBaseFlightSpeed = 7.0
)

// creatureTypeFlagBossMob is CREATURE_TYPE_FLAG_BOSS_MOB
// (SharedDefines.h:2731): the type_flags bit behind Creature::isWorldBoss
// (Creature.cpp:2353-2357).
const creatureTypeFlagBossMob = 0x4

// creatureSplineVelocity mirrors the velocity selection of
// MoveSplineInit::Launch (MoveSplineInit.cpp:106-124): without an explicit
// SetVelocity the spline runs at SelectSpeedType(unit movement flags), and
// MOVEMENTFLAG_FLYING selects MOVE_FLIGHT before the WALKING leg, so a
// flying creature's waypoint/evade/pursuit/wander spline runs at
// GetSpeed(MOVE_FLIGHT) = 1.0 * 7.0 (Creature::Initialize sets the flight
// rate to 1.0, Creature.cpp:532). Go tracks the flying analog on
// motion.CanFly, so flyers use the flight velocity regardless of walk.
// Documented no-bridge arms of the same Launch block: the SWIMMING leg of
// SelectSpeedType (creatureMotion carries no swim flag), the 0.66
// searched-assistance multiplier (no assistance model), and the client
// speed cap min(velocity, catmullrom||flying ? 50 : max(28, run*4)) —
// vacuous for template speeds (velocity stays near the base rates).
// Waypoint LAND/TAKEOFF set only AnimationTier (cosmetic), unmodeled.
func creatureSplineVelocity(motion *creatureMotion, walk bool) float32 {
	if motion != nil && motion.CanFly {
		return creatureBaseFlightSpeed
	}
	if walk {
		return motion.Speed
	}
	return motion.RunSpeed
}

// splineDurationMs mirrors MoveSpline's duration computation
// (MoveSpline.cpp:104-114): CommonInitializer starts the clock at
// minimal_duration (1ms) and each segment adds trunc(segLength*1000/velocity).
// Go launches single-segment linear splines, so the duration is
// 1 + trunc(dist*1000/speed); C++ applies no per-generator floor (a
// zero-length spline becomes a 1ms spline, MoveSpline.cpp:145-149).
func splineDurationMs(dist float64, speed float32) uint32 {
	return 1 + uint32(dist*1000/float64(speed))
}

const (
	creatureReactPassive uint8 = iota
	creatureReactDefensive
	creatureReactAggressive
)

// Item 6 coverage-bullet audit (passive, neutral, hostile, critter, GM,
// dead/ghost, invisible, summoned) vs Creature::InitializeReactState
// (Creature.cpp:1255-1266) and the react-state dispatch in
// CreatureAI::MoveInLineOfSight (CreatureAI.cpp:118-123):
//   - passive: bridged — the mapping below mirrors C++ exactly (totem,
//     trigger, critter, spirit-service; the civilian/REACT_DEFENSIVE arm is
//     commented out in C++ too, so creatureReactDefensive is defined but
//     never assigned); the aggro scan skips passive motions.
//   - neutral (REACT_NEUTRAL): no model — C++ init never assigns it either
//     (only scripted SetReactState produces neutrals); faction-neutral
//     creatures are still covered by the isAttackableFaction gate, so only
//     scripted hostile-faction neutrals diverge (they aggro on sight in Go
//     instead of engaging on damage only).
//   - hostile: the default path, fully covered by the aggro scan.
//   - critter: maps to passive via creatureType 8, matching C++
//     IsCritter()->REACT_PASSIVE; CritterAI::JustEngagedWith's flee
//     (PassiveAI.cpp:76-79, UNIT_STATE_FLEEING) is unmodeled — Go has no
//     fleeing state, so critters stand instead of fleeing.
//   - GM: bridged on all player-initiated paths — the aggro scan skips GM
//     players, player spells reject GM/invisible targets at completion
//     (explicitTargetGMBlocked), and melee swings / pet attack commands /
//     directed pet casts reject them via gmAttackTargetBlocked (the
//     IsValidAttackTarget GM/invisibility legs, Object.cpp:2945-2947, and
//     the Unit::Attack GM leg, Unit.cpp:5664-5668).
//   - dead/ghost: bridged — the aggro scan skips dead players, melee
//     rejects dead targets, and the spell dead-target gate is in the
//     completion path.
//   - invisible: unmodeled beyond the GM-invisibility flag — Go has no
//     invisibility-level/detect-level comparison (the CanSeeOrDetect legs);
//     stealth detection is the only modeled sense.
//   - summoned: guardian AI selection (CritterAI::Permissible,
//     PassiveAI.cpp:95-100) is unmodeled; creature-vs-creature acquisition
//     has no bridge — the aggro scan only iterates player targets, so C++
//     MoveInLineOfSight engagements against non-player units (guards vs
//     invaders, creatures vs summoned guardians) never start in Go.
func creatureReactState(creatureType, npcFlags, flagsExtra uint32, aiName string) uint8 {
	if creatureType == 8 || creatureType == 11 || flagsExtra&(0x00000002|0x00000080) != 0 || npcFlags&0x0000C000 != 0 || aiName == "PassiveAI" || aiName == "NullCreatureAI" || aiName == "TriggerAI" {
		return creatureReactPassive
	}
	return creatureReactAggressive
}

func isCreaturePassive(motion *creatureMotion) bool {
	return motion != nil && motion.ReactStateKnown && motion.ReactState == creatureReactPassive
}

func creatureWalkVelocity(multiplier float64) float32 {
	if multiplier <= 0 {
		return creatureBaseWalkSpeed
	}
	return float32(multiplier) * creatureBaseWalkSpeed
}

func creatureRunVelocity(multiplier float64) float32 {
	if multiplier <= 0 {
		return creatureBaseRunSpeed
	}
	return float32(multiplier) * creatureBaseRunSpeed
}

func (s *Server) motionMapLocked(mapID, instanceID uint32) map[uint64]*creatureMotion {
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	if s.instanceCreatureMotion == nil {
		s.instanceCreatureMotion = make(map[instanceAdmissionKey]map[uint64]*creatureMotion)
	}
	if s.instanceCreatureMotion[key] == nil {
		s.instanceCreatureMotion[key] = make(map[uint64]*creatureMotion)
	}
	return s.instanceCreatureMotion[key]
}

func (s *Server) findCreatureMotionLocked(mapID, instanceID uint32, guid uint64) *creatureMotion {
	motions := s.motionMapLocked(mapID, instanceID)
	if motion := motions[guid]; motion != nil && motion.Map == mapID && motion.InstanceID == instanceID {
		return motion
	}
	low, entry := uint32(guid&0x00FFFFFF), uint32(guid>>24&0x00FFFFFF)
	if motion := motions[creatureWorldGUID(low, entry)]; motion != nil && motion.Map == mapID && motion.InstanceID == instanceID {
		return motion
	}
	return nil
}

func (s *Server) findCreatureMotion(mapID, instanceID uint32, guid uint64) *creatureMotion {
	if s == nil {
		return nil
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	return s.findCreatureMotionLocked(mapID, instanceID, guid)
}

func (s *session) findCreatureMotion(guid uint64) *creatureMotion {
	if s == nil || s.server == nil || s.player == nil {
		return nil
	}
	return s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, guid)
}

func (s *Server) storeCreatureMotion(mapID, instanceID uint32, guid uint64, motion *creatureMotion) {
	if s == nil || motion == nil {
		return
	}
	s.motionMu.Lock()
	motion.Map, motion.InstanceID = mapID, instanceID
	s.motionMapLocked(mapID, instanceID)[guid] = motion
	s.motionMu.Unlock()
}

// motionFor/motionForLocked take creature_template speed RATES (speed_walk,
// speed_run — the C++ SetSpeedRate multipliers, Creature.cpp:529-530) and
// convert them to absolute yd/s via creatureWalkVelocity/creatureRunVelocity,
// mirroring GetSpeed = rate * baseMoveSpeed (Unit.cpp:86-96). A previous
// revision mixed the two conventions: the transport-passenger caller passed
// the raw rate (1.0) into Speed, so wander splines ran at 1.0 yd/s instead of
// 2.5, and RunSpeed stayed a flat 7.0 instead of 7*speed_run (8.0 default).
func (s *Server) motionFor(ctx context.Context, guid, entry, mapID, instanceID uint32, x, y, z, orientation float32, moveType uint32, wander float64, walkSpeedRate, runSpeedRate float32, currentHealth uint32) *creatureMotion {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	return s.motionForLocked(ctx, guid, entry, mapID, instanceID, x, y, z, orientation, moveType, wander, walkSpeedRate, runSpeedRate, currentHealth)
}

func (s *Server) motionForLocked(ctx context.Context, guid, entry, mapID, instanceID uint32, x, y, z, orientation float32, moveType uint32, wander float64, walkSpeedRate, runSpeedRate float32, currentHealth uint32) *creatureMotion {
	motions := s.motionMapLocked(mapID, instanceID)
	key := creatureWorldGUID(guid, entry)
	motion := motions[key]
	created := false
	if motion == nil || motion.Entry != entry || motion.Map != mapID || motion.InstanceID != instanceID {
		st := s.loadCreatureStats(ctx, entry)
		health := st.Health
		if currentHealth > 0 {
			health = currentHealth
		}
		if st.MaxHealth > 0 && health > st.MaxHealth {
			health = st.MaxHealth
		}
		motion = &creatureMotion{
			GUID:        key,
			Entry:       entry,
			Map:         mapID,
			InstanceID:  instanceID,
			HomeX:       x,
			HomeY:       y,
			HomeZ:       z,
			Orientation: orientation,
			X:           x,
			Y:           y,
			Z:           z,
			Speed:       creatureWalkVelocity(float64(walkSpeedRate)),
			RunSpeed:    creatureRunVelocity(float64(runSpeedRate)),
			MoveType:    moveType,
			Wander:      wander,
			WanderSteps: 2 + rand.Intn(9), // urand(2,10), RandomMovementGenerator.cpp:DoInitialize
			Health:      health,
			// Creature::ResetPlayerDamageReq (Creature.h:324): GetHealth()/2.
			PlayerDamageReq: health / 2,
			MaxHealth:       st.MaxHealth,
			Armor:           st.Armor,
			Resistances:     st.Resistances,
			MinDamage:       st.MinDamage,
			MaxDamage:       st.MaxDamage,
			Level:           st.Level,
			AttackTime:      st.AttackTime,
			CombatReach:     st.CombatReach,
			UnitFlags:       st.UnitFlags,
			FlagsExtra:      st.FlagsExtra,
			TypeFlags:       st.TypeFlags,
			CanFly:          st.CanFly,
			ReactState:      st.ReactState,
			ReactStateKnown: st.ReactStateKnown,
			CreatureType:    st.CreatureType,
			AIName:          st.AIName,
		}
		if st.UnitClass == 2 || st.UnitClass == 8 {
			motion.MaxPowers[0] = st.Mana
			motion.Powers[0] = st.Mana
			motion.MaxMana = st.Mana
			motion.Mana = st.Mana
		}
		if motion.ThreatMgr == nil {
			motion.ThreatMgr = NewThreatManager(motion)
		}
		if motion.BossAI == nil {
			motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
		}
		if moveType == 2 {
			motion.PathID = s.loadCreaturePathID(ctx, guid, entry)
			motion.Points = s.loadWaypoints(ctx, motion.PathID)
		}
		motions[key] = motion
		created = true
	}
	if created {
		// Creature_SelectLevel analog: a freshly materialized creature gets
		// its AutoBalance attributes computed before it acts.
		s.autoBalanceModifyCreatureAttributes(ctx, motion, true)
	}
	motion.Refreshed = time.Now()
	return motion
}

func (s *Server) relocateTransportCreatureMotions(ctx context.Context, passengers []creatureSpawn) {
	if s == nil || len(passengers) == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	now := time.Now()
	for _, passenger := range passengers {
		if passenger.GUID == 0 || passenger.Entry == 0 || passenger.TransportGUID == 0 {
			continue
		}
		key := creatureWorldGUID(passenger.GUID, passenger.Entry)
		isNew := s.motionMapLocked(passenger.Map, 0)[key] == nil
		motion := s.motionForLocked(ctx, passenger.GUID, passenger.Entry, passenger.Map, 0, passenger.X, passenger.Y, passenger.Z, passenger.Orientation, 0, 0, passenger.WalkSpeed, passenger.RunSpeed, passenger.Health)
		motion.TransportGUID = passenger.TransportGUID
		motion.Map, motion.InstanceID = passenger.Map, 0
		motion.HomeX, motion.HomeY, motion.HomeZ = passenger.X, passenger.Y, passenger.Z
		motion.X, motion.Y, motion.Z, motion.Orientation = passenger.X, passenger.Y, passenger.Z, passenger.Orientation
		motion.Moving, motion.MoveType, motion.Wander = false, 0, 0
		motion.PathID, motion.Points, motion.NextIdx = 0, nil, 0
		motion.MoveEnds, motion.Refreshed = time.Time{}, now
		if isNew {
			motion.Health = creatureSpawnHealth(passenger.RegenerateHealth, passenger.Health, motion.MaxHealth)
		}
	}
}

func (s *Server) unloadTransportCreatureMotions(mapID uint32, transportGUID uint64, passengers []creatureSpawn) {
	if s == nil || len(passengers) == 0 || transportGUID == 0 {
		return
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, passenger := range passengers {
		key := creatureWorldGUID(passenger.GUID, passenger.Entry)
		if motion := s.motionMapLocked(mapID, 0)[key]; motion != nil && motion.Map == mapID && motion.TransportGUID == transportGUID {
			delete(s.motionMapLocked(mapID, 0), key)
		}
	}
}

func (s *Server) triggerCreatureAggro(ctx context.Context, creatureGUID, playerGUID uint64) {
	playerSess := s.findSessionByGUID(playerGUID)
	mapID, instanceID := uint32(0), uint32(0)
	if playerSess != nil && playerSess.player != nil {
		mapID, instanceID = playerSess.player.Map, playerSess.player.InstanceID
	}
	s.motionMu.Lock()
	enteredCombat := false
	guid := uint32(creatureGUID & 0x00FFFFFF)
	entry := uint32((creatureGUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(guid, entry)
	motion := s.findCreatureMotionLocked(mapID, instanceID, creatureGUID)
	if motion == nil && s.WorldStore != nil && s.WorldStore.DB != nil {
		var x, y, z float64
		var mapID, faction, curHealth int64
		var name, scriptName string
		if err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT c.map, c.position_x, c.position_y, c.position_z,
			COALESCE(t.faction, 0), COALESCE(t.name, ''), COALESCE(t.ScriptName, ''), COALESCE(c.curhealth, 0)
			FROM creature AS c
			JOIN creature_template AS t ON t.entry = c.id
			WHERE c.guid = ?`, guid).Scan(&mapID, &x, &y, &z, &faction, &name, &scriptName, &curHealth); err == nil {
			st := s.loadCreatureStats(ctx, entry)
			health := st.Health
			if curHealth > 0 {
				health = uint32(curHealth)
			}
			if st.MaxHealth > 0 && health > st.MaxHealth {
				health = st.MaxHealth
			}
			motion = &creatureMotion{
				GUID:       creatureGUID,
				Entry:      entry,
				Map:        uint32(mapID),
				InstanceID: instanceID,
				HomeX:      float32(x),
				HomeY:      float32(y),
				HomeZ:      float32(z),
				X:          float32(x),
				Y:          float32(y),
				Z:          float32(z),
				Speed:      creatureBaseWalkSpeed,
				RunSpeed:   creatureBaseRunSpeed,
				Faction:    uint32(faction),
				Level:      st.Level,
				Health:     health,
				// Creature::ResetPlayerDamageReq (Creature.h:324): GetHealth()/2.
				PlayerDamageReq: health / 2,
				MaxHealth:       st.MaxHealth,
				Armor:           st.Armor,
				MinDamage:       st.MinDamage,
				MaxDamage:       st.MaxDamage,
				AttackTime:      st.AttackTime,
				CombatReach:     st.CombatReach,
				UnitFlags:       st.UnitFlags,
				FlagsExtra:      st.FlagsExtra,
				TypeFlags:       st.TypeFlags,
				CanFly:          st.CanFly,
				ReactState:      st.ReactState,
				ReactStateKnown: st.ReactStateKnown,
				Name:            name,
				ScriptName:      scriptName,
			}
			motions := s.motionMapLocked(motion.Map, instanceID)
			motions[creatureGUID] = motion
			motions[stdKey] = motion
		}
	}
	if motion != nil && motion.Health > 0 {
		if isCreaturePassive(motion) {
			s.motionMu.Unlock()
			return
		}
		if motion.Evading {
			s.motionMu.Unlock()
			return
		}
		if motion.ThreatMgr == nil {
			motion.ThreatMgr = NewThreatManager(motion)
		}
		if motion.BossAI == nil {
			motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
		}
		// Unit::EngageWithTarget (Unit.cpp:8429-8438) seeds 0.0f threat with
		// ignoreModifiers/ignoreRedirects; the add still registers the entry
		// and runs the victim leg (ThreatManager::AddThreat new-target arm).
		motion.ThreatMgr.AddThreat(playerGUID, 0.0, true)
		if !motion.InCombat {
			enteredCombat = true
			s.broadcastAIReactionInInstance(motion.Map, motion.InstanceID, creatureGUID, 2)
			startPkt := buildAttackStart(creatureGUID, playerGUID)
			if playerSess != nil {
				_ = playerSess.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
				s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, playerSess)
			} else {
				s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, nil)
			}
			if motion.BossAI != nil {
				if playerSess != nil {
					s.beginInstanceEncounter(motion, playerSess.player)
				}
				motion.BossAI.OnAggro(ctx, s, motion, playerGUID)
			}
		}
		motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
		motion.InCombat = true
		motion.Moving = false
		// CombatAI::JustEngagedWith / CasterAI::JustEngagedWith pre-arm
		// (CombatAI.cpp:76-88, 139-162): AICOND_COMBAT template spells get
		// their first event scheduled here, inside the motion lock.
		if enteredCombat {
			if motion.AIName == "CasterAI" {
				// motionMu is held here: no creature-motion lookup for
				// target positions (see resolveAISpellTargetGUID).
				s.scheduleCasterAISpellEvents(ctx, motion, motion.TargetGUID, time.Now(), nil, false)
			} else {
				s.scheduleAISpellEvents(motion, time.Now())
			}
		}
	}
	// The Lua enter-combat hook must run outside the motion lock: hook
	// handlers call back into creature methods that take it themselves.
	// Reference: Eluna CREATURE_EVENT_ON_ENTER_COMBAT (event 1), fired when
	// the creature freshly enters combat, mirroring the OnAggro above.
	fireMotion := motion
	firePlayerGUID := playerGUID
	s.motionMu.Unlock()
	// CreatureGroup::MemberEngagingTarget (CreatureGroups.cpp:226-255) via
	// Creature::AtEngage (Creature.cpp:3431-3453): formation assists fire on
	// fresh engages only (enteredCombat) — in C++ they ride AtEngage, not
	// every damage event.
	if enteredCombat && fireMotion != nil {
		s.memberEngagingTarget(ctx, fireMotion, firePlayerGUID)
	}
	if enteredCombat && fireMotion != nil {
		var target any
		if playerSess != nil {
			target = playerSess.luaPlayer()
		} else if found := s.findPlayer(firePlayerGUID); found != nil {
			target = found
		}
		s.fireCreatureLuaEvent(ctx, fireMotion, scripting.CreatureEventOnEnterCombat, target)
	}
	if enteredCombat && fireMotion != nil {
		// CombatAI::JustEngagedWith / CasterAI::JustEngagedWith
		// (CombatAI.cpp:76-88, 139-162): AICOND_AGGRO template spells are
		// cast once at the engage victim, non-triggered; the AICOND_COMBAT
		// event pre-arm ran inside the motion lock above.
		s.castAggroConditionSpells(ctx, fireMotion, firePlayerGUID)
	}
}

// castAggroConditionSpells bridges the AICOND_AGGRO arm of
// CombatAI::JustEngagedWith (CombatAI.cpp:76-88, inherited by CasterAI):
// template spells classified AICOND_AGGRO by UnitAI::FillAISpellInfo
// (UnitAI.cpp:189-196) — not castable-while-dead, and passive
// (SPELL_ATTR0_PASSIVE) or infinite-duration (GetDuration() == -1) — are
// cast once at the engage victim, non-triggered. Packet-only via
// castCreatureSpell, matching the AICOND_DIE death-cast bridge in kill.go.
// The InitializeAI DBC gate (CombatAI.cpp:58, via loadCreatureSpells) means
// the list holds only DBC-valid spells here; a DBC row that disappears
// after load is still skipped defensively.
func (s *Server) castAggroConditionSpells(ctx context.Context, m *creatureMotion, victimGUID uint64) {
	if s == nil || m == nil || victimGUID == 0 {
		return
	}
	if len(m.Spells) == 0 && s.WorldStore != nil && s.WorldStore.DB != nil {
		m.Spells = s.loadCreatureSpells(ctx, m.Entry)
	}
	if len(m.Spells) == 0 || s.Data == nil {
		return
	}
	for _, spellID := range m.Spells {
		spellInfo, found, err := s.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		if spellInfo.Attributes&spellAttr0CastableWhileDead != 0 {
			continue
		}
		aggro := spellInfo.Attributes&spellAttr0Passive != 0
		if !aggro && spellInfo.DurationIndex != 0 {
			if dur, ok, derr := s.Data.SpellDuration(spellInfo.DurationIndex, 1); derr == nil && ok && dur < 0 {
				aggro = true
			}
		}
		if aggro {
			s.castCreatureSpell(ctx, m, spellID, victimGUID)
		}
	}
}

// AI-spell condition classes from UnitAI::FillAISpellInfo (UnitAI.cpp:188-206),
// mirroring enum AICondition (CreatureAIImpl.h:43-48): AICOND_AGGRO,
// AICOND_COMBAT, AICOND_DIE.
const (
	aiSpellCondAggro = iota
	aiSpellCondCombat
	aiSpellCondDie
)

// AI-spell target classes from UnitAI::FillAISpellInfo + UnitAI::DoCast
// (UnitAI.cpp:113-163, 210-226), mirroring enum AITarget
// (CreatureAIImpl.h:33-41). The numeric order matters: FillAISpellInfo's
// UPDATE_TARGET only ever raises the target, so the Go consts keep the C++
// order SELF < VICTIM < ENEMY < ALLY < BUFF < DEBUFF.
const (
	aiSpellTargetSelf = iota
	aiSpellTargetVictim
	aiSpellTargetEnemy
	aiSpellTargetAlly
	aiSpellTargetBuff
	aiSpellTargetDebuff
)

// aiSpellCondition classifies a creature-template spell per
// UnitAI::FillAISpellInfo (UnitAI.cpp:188-206): castable-while-dead ->
// AICOND_DIE, passive or infinite-duration -> AICOND_AGGRO, else
// AICOND_COMBAT. The DBC-validity gate of CombatAI::InitializeAI
// (CombatAI.cpp:58) is the caller's responsibility: only DBC-found spells
// are classified here.
func (s *Server) aiSpellCondition(spell wotlk.Spell) int {
	if spell.Attributes&spellAttr0CastableWhileDead != 0 {
		return aiSpellCondDie
	}
	if spell.Attributes&spellAttr0Passive != 0 {
		return aiSpellCondAggro
	}
	if spell.DurationIndex != 0 && s != nil && s.Data != nil {
		if dur, ok, derr := s.Data.SpellDuration(spell.DurationIndex, 1); derr == nil && ok && dur < 0 {
			return aiSpellCondAggro
		}
	}
	return aiSpellCondCombat
}

// aiSpellCooldownMs mirrors the AISpellInfo cooldown used by
// CombatAI::JustEngagedWith / CombatAI::UpdateAI (CombatAI.cpp:76-106):
// AISpellInfoType defaults cooldown to AI_DEFAULT_COOLDOWN (5000ms,
// CreatureAIImpl.h:55) and FillAISpellInfo raises it to the spell's
// RecoveryTime when higher (UnitAI.cpp:208-209).
func aiSpellCooldownMs(spell wotlk.Spell) uint32 {
	if spell.RecoveryTime > 5000 {
		return spell.RecoveryTime
	}
	return 5000
}

// aiSpellRealCooldownMs mirrors AISpellInfo::realCooldown (UnitAI.cpp:227:
// RecoveryTime + StartRecoveryTime), used by CasterAI::JustEngagedWith /
// CasterAI::UpdateAI (CombatAI.cpp:139-166).
func aiSpellRealCooldownMs(spell wotlk.Spell) uint32 {
	return spell.RecoveryTime + spell.StartRecoveryTime
}

// aiSpellCastTimeMs mirrors Unit::GetCurrentSpellCastTime for the CasterAI
// re-arm (CombatAI.cpp:157-158, 163): the DBC SpellCastTimes row for the
// spell's CastingTimeIndex; 0 when the DBC row is absent.
func (s *Server) aiSpellCastTimeMs(spell wotlk.Spell) uint32 {
	if s == nil || s.Data == nil || spell.CastingTimeIndex == 0 {
		return 0
	}
	if value, ok, err := s.Data.SpellCastTime(spell.CastingTimeIndex); err == nil && ok && value > 0 {
		return uint32(value)
	}
	return 0
}

// aiSpellTargetClass mirrors the target-classification leg of
// UnitAI::FillAISpellInfo (UnitAI.cpp:210-226): only when the spell has a
// non-zero max range, implicit-target hits upgrade the default AITARGET_SELF:
// TARGET_UNIT_TARGET_ENEMY (6) or TARGET_DEST_TARGET_ENEMY (53) -> VICTIM,
// TARGET_UNIT_DEST_AREA_ENEMY (16) -> ENEMY; a SPELL_EFFECT_APPLY_AURA (6)
// effect on TARGET_UNIT_TARGET_ENEMY -> DEBUFF, and a positive aura ->
// BUFF (SharedDefines.h:1447, 1452, 1489).
func (s *Server) aiSpellTargetClass(spell wotlk.Spell) int {
	target := aiSpellTargetSelf
	if s == nil || s.Data == nil {
		return target
	}
	maxRange := float32(0)
	if spellRange, rangeFound, rangeErr := s.Data.SpellRange(spell.RangeIndex); rangeErr == nil && rangeFound {
		maxRange = spellRange.MaxHostile
	}
	if maxRange == 0 {
		return target
	}
	for i := range spell.Effects {
		eff := spell.Effects[i]
		targetType := eff.ImplicitTargetA
		switch targetType {
		case 6, 53: // TARGET_UNIT_TARGET_ENEMY, TARGET_DEST_TARGET_ENEMY
			if target < aiSpellTargetVictim {
				target = aiSpellTargetVictim
			}
		case 16: // TARGET_UNIT_DEST_AREA_ENEMY
			if target < aiSpellTargetEnemy {
				target = aiSpellTargetEnemy
			}
		}
		if eff.Effect == 6 { // SPELL_EFFECT_APPLY_AURA
			if targetType == 6 {
				if target < aiSpellTargetDebuff {
					target = aiSpellTargetDebuff
				}
			} else if spellIsPositive(spell) {
				if target < aiSpellTargetBuff {
					target = aiSpellTargetBuff
				}
			}
		}
	}
	return target
}

// combatReachOrDefault mirrors the engine's default combat reach (1.5,
// the C++ UNIT_FIELD_COMBATREACH default for units without model data).
func (m *creatureMotion) combatReachOrDefault() float32 {
	if m == nil || m.CombatReach <= 0 {
		return 1.5
	}
	return m.CombatReach
}

// spellRangeFlagMelee / spellRangeFlagRanged mirror SpellRangeFlags
// (Spell.h:112-113).
const (
	spellRangeFlagMelee  = 1
	spellRangeFlagRanged = 2
)

// aiDoCastVictimInRange mirrors Spell::CheckRange (Spell.cpp:6540-6575,
// strict=true via Spell::prepare:3100) with GetMinMaxRange(strict=true)
// for the UnitAI::DoCast victim arm: the exact 3D center distance must sit
// inside [minRange, maxRange]. A MELEE-flagged range collapses to the
// caster's melee range (Unit::GetMeleeRange, Unit.cpp:614-618); otherwise
// maxRange = MaxHostile + both combat reaches, and minRange = MinHostile
// (+ melee range for SPELL_RANGE_RANGED) plus both reaches only when
// MinHostile > 0 and the range is not RANGED-flagged — a 0-min spell casts
// at melee range in C++. Deltas: the both-moving +8/3 arm has no Go model
// (motions carry no walk/run flag distinction), and SPELLMOD_RANGE / the
// REQ_AMMO ranged-weapon arm are player-side; a missing DBC range row stays
// permissive per the unknown-data convention.
func (s *Server) aiDoCastVictimInRange(motion *creatureMotion, target *playerPos, spell wotlk.Spell, cReach, victimReach float32) bool {
	if s == nil || s.Data == nil || motion == nil || target == nil {
		return true
	}
	rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex)
	if err != nil || !ok {
		return true
	}
	dist := float32(distance3D(motion.X, motion.Y, motion.Z, target.X, target.Y, target.Z))
	var minRange, maxRange float32
	if rangeEntry.Flags&spellRangeFlagMelee != 0 {
		maxRange = float32(calcMeleeRange(cReach, victimReach))
	} else {
		minRange = rangeEntry.MinHostile
		maxRange = rangeEntry.MaxHostile
		if rangeEntry.Flags&spellRangeFlagRanged != 0 {
			minRange += float32(calcMeleeRange(cReach, victimReach))
		} else if minRange > 0 {
			minRange += cReach + victimReach
		}
		maxRange += cReach + victimReach
	}
	if dist > maxRange {
		return false
	}
	if minRange > 0 && dist < minRange {
		return false
	}
	return true
}

// aiThreatUnitPos resolves a threat entry's live position and combat reach:
// players via the tick's player list or their session, other units (pets)
// via the creature motion map. allowMotionLookup=false is for callers
// holding motionMu (the engage path), where the motion map cannot be
// consulted — the C++ IsInMap/InSamePhase gate (Unit.cpp:585) then
// excludes them, documented. isPlayer mirrors DefaultTargetSelector's
// _playerOnly arm (UnitAI.cpp:270: TYPEID_PLAYER).
func (s *Server) aiThreatUnitPos(m *creatureMotion, guid uint64, players []playerPos, allowMotionLookup bool) (x, y, z, reach float32, isPlayer, ok bool) {
	if s == nil || m == nil || guid == 0 {
		return 0, 0, 0, 0, false, false
	}
	for i := range players {
		if players[i].GUID == guid && players[i].Sess != nil && players[i].Sess.player != nil {
			reach = players[i].Sess.player.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			return players[i].X, players[i].Y, players[i].Z, reach, true, true
		}
	}
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
		reach = sess.player.CombatReach
		if reach <= 0 {
			reach = 1.5
		}
		return sess.player.X, sess.player.Y, sess.player.Z, reach, true, true
	}
	if allowMotionLookup {
		if cm := s.findCreatureMotion(m.Map, m.InstanceID, guid); cm != nil {
			reach = cm.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			return cm.X, cm.Y, cm.Z, reach, false, true
		}
	}
	return 0, 0, 0, 0, false, false
}

// aiSpellThreatCandidates mirrors the SelectTarget threat-list scan behind
// UnitAI::DoCast's ENEMY/DEBUFF arms (UnitAI.cpp:126-147): every threat
// entry within GetMaxRange(false) of the caster, player-only via
// SPELL_ATTR3_ONLY_TARGET_PLAYERS, the victim included (withTank=true).
// Range is DefaultTargetSelector's (UnitAI.cpp:273): 3D distance strictly
// below maxRange plus both combat reaches (Unit::IsWithinCombatRange,
// Unit.cpp:583-597); maxRange<=0 disables the filter (dist 0 is ignored).
func (s *Server) aiSpellThreatCandidates(m *creatureMotion, maxRange float32, playerOnly bool, players []playerPos, allowMotionLookup bool) []uint64 {
	if m == nil || m.ThreatMgr == nil {
		return nil
	}
	reach := m.combatReachOrDefault()
	var candidates []uint64
	for _, e := range m.ThreatMgr.SortedEntries() {
		if e.VictimGUID == 0 {
			continue
		}
		x, y, z, vreach, isPlayer, ok := s.aiThreatUnitPos(m, e.VictimGUID, players, allowMotionLookup)
		if !ok {
			continue
		}
		if playerOnly && !isPlayer {
			continue
		}
		if maxRange > 0 && distance3D(m.X, m.Y, m.Z, x, y, z) >= float64(maxRange+reach+vreach) {
			continue
		}
		candidates = append(candidates, e.VictimGUID)
	}
	return candidates
}

// aiDoCastMaxRange mirrors the range term of UnitAI::DoCast's ENEMY/DEBUFF
// arms (UnitAI.cpp:129, 137): spellInfo->GetMaxRange(false).
func (s *Server) aiDoCastMaxRange(spell wotlk.Spell) float32 {
	if s != nil && s.Data != nil {
		if rangeEntry, ok, err := s.Data.SpellRange(spell.RangeIndex); err == nil && ok {
			return rangeEntry.MaxHostile
		}
	}
	return 0
}

// resolveAISpellTargetGUID mirrors UnitAI::DoCast target selection
// (UnitAI.cpp:113-163): SELF/ALLY/BUFF -> me; VICTIM -> the current
// victim; DEBUFF -> the victim when it passes DefaultTargetSelector
// (in range, playerOnly; the -spellId aura arm has no Go creature-aura
// model so it always passes), else a random in-range threat entry;
// ENEMY -> a random in-range threat entry, victim included
// (withTank=true), player-only when SPELL_ATTR3_ONLY_TARGET_PLAYERS.
// A null pick returns 0: C++ returns SPELL_FAILED_BAD_TARGETS without
// casting, and callers must skip the cast (the event still re-arms).
func (s *Server) resolveAISpellTargetGUID(m *creatureMotion, spell wotlk.Spell, victimGUID uint64, players []playerPos, allowMotionLookup bool) uint64 {
	if m == nil {
		return victimGUID
	}
	switch s.aiSpellTargetClass(spell) {
	case aiSpellTargetVictim:
		if victimGUID == 0 {
			return m.GUID
		}
		return victimGUID
	case aiSpellTargetDebuff:
		maxRange := s.aiDoCastMaxRange(spell)
		playerOnly := spell.AttributesEx3&spellAttr3OnlyTargetPlayers != 0
		if victimGUID != 0 {
			if x, y, z, vreach, isPlayer, ok := s.aiThreatUnitPos(m, victimGUID, players, allowMotionLookup); ok &&
				(!playerOnly || isPlayer) &&
				(maxRange <= 0 || distance3D(m.X, m.Y, m.Z, x, y, z) < float64(maxRange+m.combatReachOrDefault()+vreach)) {
				return victimGUID
			}
		}
		if candidates := s.aiSpellThreatCandidates(m, maxRange, playerOnly, players, allowMotionLookup); len(candidates) > 0 {
			return candidates[rand.Intn(len(candidates))]
		}
		return 0
	case aiSpellTargetEnemy:
		maxRange := s.aiDoCastMaxRange(spell)
		playerOnly := spell.AttributesEx3&spellAttr3OnlyTargetPlayers != 0
		if candidates := s.aiSpellThreatCandidates(m, maxRange, playerOnly, players, allowMotionLookup); len(candidates) > 0 {
			return candidates[rand.Intn(len(candidates))]
		}
		return 0
	default:
		return m.GUID
	}
}

// aiVictimHasBreakableCC bridges the CasterAI::UpdateAI guard
// (CombatAI.cpp:155):
// me->EnsureVictim()->HasBreakableByDamageCrowdControlAura(me)
// (Unit.cpp:673-683). True when the victim carries a CONFUSE/FEAR/STUN/
// ROOT/TRANSFORM aura (HasBreakableByDamageAuraType, Unit.cpp:663-671)
// whose AuraInterruptFlags include AURA_INTERRUPT_FLAG_TAKE_DAMAGE and
// whose caster is this creature. C++ checks aura EFFECTS by type; Go's
// activeAura carries only the first effect's AuraType, so a multi-effect
// aura whose CC effect is not first is missed — noted, not bridged. The
// channeled-CC self-exclusion (Unit.cpp:676) has no Go model (creatures
// carry no channeled-spell state) and is noted, not bridged.
func (s *Server) aiVictimHasBreakableCC(m *creatureMotion, sess *session) bool {
	if m == nil || sess == nil {
		return false
	}
	for _, aura := range sess.activeAuras {
		if aura == nil || aura.CasterGUID != m.GUID {
			continue
		}
		switch aura.AuraType {
		case spellAuraModConfuse, spellAuraModFear, spellAuraModStun, spellAuraModRoot, spellAuraTransform:
		default:
			continue
		}
		if getSpellAuraInterruptFlags(aura.SpellID, aura.AuraInterruptFlags)&auraInterruptFlagTakeDamage != 0 {
			return true
		}
	}
	return false
}

// scheduleAISpellEvents bridges the pre-arm of CombatAI::JustEngagedWith
// (CombatAI.cpp:76-88): every AICOND_COMBAT template spell gets its first
// event at now + cooldown + rand32() % cooldown. The jitter leg is skipped
// when the cooldown is 0 (rand32() % 0 is undefined in C++; RecoveryTime is
// 0 only for instant-cooldown DBC rows).
func (s *Server) scheduleAISpellEvents(m *creatureMotion, now time.Time) {
	if s == nil || m == nil || s.Data == nil {
		return
	}
	if m.SpellEventTimes == nil {
		m.SpellEventTimes = make(map[uint32]time.Time)
	}
	for _, spellID := range m.Spells {
		spellInfo, found, err := s.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		if s.aiSpellCondition(spellInfo) != aiSpellCondCombat {
			continue
		}
		cooldown := aiSpellCooldownMs(spellInfo)
		delay := cooldown
		if cooldown > 0 {
			delay += uint32(rand.Intn(int(cooldown)))
		}
		m.SpellEventTimes[spellID] = now.Add(time.Duration(delay) * time.Millisecond)
	}
}

// scheduleCasterAISpellEvents bridges CasterAI::JustEngagedWith
// (CombatAI.cpp:139-162): every AICOND_COMBAT template spell schedules at
// realCooldown (RecoveryTime + StartRecoveryTime, UnitAI.cpp:227); the
// randomly picked spell (rand32() % _spells.size() over the DBC-valid list,
// i.e. the CombatAI::InitializeAI gate) is cast immediately via the DoCast
// analog and its schedule additionally absorbs the current cast time. When
// the pick lands on an AGGRO/DIE spell nothing extra fires — the AGGRO arm
// is cast separately by castAggroConditionSpells.
func (s *Server) scheduleCasterAISpellEvents(ctx context.Context, m *creatureMotion, victimGUID uint64, now time.Time, players []playerPos, allowMotionLookup bool) {
	if s == nil || m == nil || victimGUID == 0 || len(m.Spells) == 0 || s.Data == nil {
		return
	}
	if m.SpellEventTimes == nil {
		m.SpellEventTimes = make(map[uint32]time.Time)
	}
	var valid []uint32
	var infos []wotlk.Spell
	var combat []bool
	for _, spellID := range m.Spells {
		spellInfo, found, err := s.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		valid = append(valid, spellID)
		infos = append(infos, spellInfo)
		combat = append(combat, s.aiSpellCondition(spellInfo) == aiSpellCondCombat)
	}
	if len(valid) == 0 {
		return
	}
	pick := rand.Intn(len(valid))
	for i, spellID := range valid {
		if !combat[i] {
			continue
		}
		delay := aiSpellRealCooldownMs(infos[i])
		if i == pick {
			// UnitAI::DoCast (UnitAI.cpp:113-163): a null target pick
			// (ENEMY/DEBUFF with no in-range candidate) casts nothing
			// (SPELL_FAILED_BAD_TARGETS) but still consumes the event.
			if tgt := s.resolveAISpellTargetGUID(m, infos[i], victimGUID, players, allowMotionLookup); tgt != 0 {
				s.castCreatureSpell(ctx, m, spellID, tgt)
			}
			delay += s.aiSpellCastTimeMs(infos[i])
		}
		m.SpellEventTimes[spellID] = now.Add(time.Duration(delay) * time.Millisecond)
	}
}

// dueAISpell bridges the event-execution arm of CombatAI::UpdateAI /
// CasterAI::UpdateAI (CombatAI.cpp:90-106, 153-166): it returns the first
// due AICOND_COMBAT template spell in _spells order (EventMap pops the
// earliest scheduled event; _spells order is the m_spells[8] array order,
// CreatureData.h:144), with its DoCast target. A creature whose schedule
// was never armed (an engage path that predates the scheduler) arms lazily
// here via the CombatAI/CasterAI engage pre-arm, matching the C++ invariant
// that _events is always armed by JustEngagedWith.
func (s *Server) dueAISpell(ctx context.Context, m *creatureMotion, victimGUID uint64, now time.Time, players []playerPos, allowMotionLookup bool) (uint32, wotlk.Spell, uint64, bool) {
	if s == nil || m == nil || s.Data == nil || len(m.Spells) == 0 {
		return 0, wotlk.Spell{}, 0, false
	}
	armed := false
	if m.SpellEventTimes != nil {
		for _, spellID := range m.Spells {
			if t, ok := m.SpellEventTimes[spellID]; ok && !t.IsZero() {
				armed = true
				break
			}
		}
	}
	if !armed {
		if m.AIName == "CasterAI" {
			s.scheduleCasterAISpellEvents(ctx, m, victimGUID, now, players, allowMotionLookup)
		} else {
			s.scheduleAISpellEvents(m, now)
		}
	}
	for _, spellID := range m.Spells {
		spellInfo, found, err := s.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		if s.aiSpellCondition(spellInfo) != aiSpellCondCombat {
			continue
		}
		if fireAt, ok := m.SpellEventTimes[spellID]; ok && !fireAt.After(now) {
			return spellID, spellInfo, s.resolveAISpellTargetGUID(m, spellInfo, victimGUID, players, allowMotionLookup), true
		}
	}
	return 0, wotlk.Spell{}, 0, false
}

// rearmAISpell schedules a fired spell's next event: CombatAI::UpdateAI
// (CombatAI.cpp:96-99) re-arms with cooldown + rand32() % cooldown
// (cooldown = max(5000, RecoveryTime), CreatureAIImpl.h:55, UnitAI.cpp:208-209);
// CasterAI::UpdateAI (CombatAI.cpp:163-166) re-arms with
// (casttime ? casttime : 500ms) + realCooldown. The UNIT_STATE_CASTING
// early-out is bridged by motion.CastingUntil, gated in the combat tick
// above (CombatAI.cpp:97, 159) — the schedule keeps ticking during the
// cast exactly as C++'s EventMap does past the early-out return.
func (s *Server) rearmAISpell(m *creatureMotion, spell wotlk.Spell, spellID uint32, now time.Time) {
	if m == nil {
		return
	}
	if m.SpellEventTimes == nil {
		m.SpellEventTimes = make(map[uint32]time.Time)
	}
	var delay uint32
	if m.AIName == "CasterAI" {
		castTime := s.aiSpellCastTimeMs(spell)
		if castTime == 0 {
			castTime = 500
		}
		delay = castTime + aiSpellRealCooldownMs(spell)
	} else {
		cooldown := aiSpellCooldownMs(spell)
		delay = cooldown
		if cooldown > 0 {
			delay += uint32(rand.Intn(int(cooldown)))
		}
	}
	m.SpellEventTimes[spellID] = now.Add(time.Duration(delay) * time.Millisecond)
}

func (s *Server) charmCreature(ctx context.Context, key creatureAuraKey, charmerGUID uint64, charmerRace uint8) ([]uint32, uint8, uint8, bool) {
	if s == nil || key.GUID == 0 || charmerGUID == 0 {
		return nil, 0, 0, false
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if motion == nil {
		s.motionMu.Unlock()
		return nil, 0, 0, false
	}
	if len(motion.Spells) == 0 {
		motion.Spells = s.loadCreatureSpells(ctx, motion.Entry)
	}
	if !motion.Charmed {
		motion.CharmUnitFlags = motion.UnitFlags
		motion.CharmFaction = motion.Faction
		motion.CharmOwnerGUID = motion.OwnerGUID
	}
	motion.CharmerGUID = charmerGUID
	motion.Charmed = true
	motion.UnitFlags |= unitFlagPlayerControlled
	motion.Faction = s.raceFaction(charmerRace)
	motion.OwnerGUID = charmerGUID
	motion.PetCommand = PetCommandFollow
	motion.PetReact = PetReactDefensive
	motion.InCombat = false
	motion.TargetGUID = 0
	motion.Moving = false
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	spells := append([]uint32(nil), motion.Spells...)
	reactState, commandState := motion.ReactState, motion.PetCommand
	mapID, rawGUID := motion.Map, motion.GUID
	flags, faction := motion.UnitFlags, motion.Faction
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(mapID, motion.InstanceID, rawGUID, map[int]uint32{unitFieldFlags: flags, unitFieldFaction: faction})
	return spells, reactState, commandState, true
}

func (s *Server) uncharmCreature(key creatureAuraKey, charmerGUID uint64) {
	if s == nil || key.GUID == 0 {
		return
	}
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
	if motion == nil || !motion.Charmed || (charmerGUID != 0 && motion.CharmerGUID != charmerGUID) {
		s.motionMu.Unlock()
		return
	}
	lastCharmer := motion.CharmerGUID
	passive := isCreaturePassive(motion)
	motion.Charmed = false
	motion.CharmerGUID = 0
	motion.UnitFlags = motion.CharmUnitFlags
	motion.Faction = motion.CharmFaction
	motion.OwnerGUID = motion.CharmOwnerGUID
	motion.PetCommand = PetCommandStay
	motion.PetReact = PetReactPassive
	motion.CharmUnitFlags = 0
	motion.CharmFaction = 0
	motion.CharmOwnerGUID = 0
	// Unit::RemoveCharmedBy ends charm with CombatStop() (Unit.cpp:11940+):
	// the unit leaves combat before the restored AI's OnCharmed runs.
	// Threat is kept — C++ CombatStop does not clear the threat table.
	motion.TargetGUID = 0
	motion.InCombat = false
	motion.Moving = false
	mapID, instanceID, rawGUID := motion.Map, motion.InstanceID, motion.GUID
	flags, faction := motion.UnitFlags, motion.Faction
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, rawGUID, map[int]uint32{unitFieldFlags: flags, unitFieldFaction: faction})
	// CreatureAI::OnCharmed (CreatureAI.cpp:54-70): the restored AI engages
	// the last charmer unless passive (the EngageWithTarget 0.0f threat seed,
	// Unit.cpp:8429-8438). ObjectAccessor::GetUnit only resolves live units
	// in the creature's map, so a gone charmer is not engaged — checked
	// without the motion lock (findSessionByGUID takes sessionsMu, and no
	// caller holds motionMu across it). The lastCharmer==0 arm needs no
	// bridge: the C++ gate `me->LastCharmerGUID` is false there, so C++ does
	// nothing too.
	engaged := false
	if lastCharmer != 0 && !passive && s.charmerResolvesInInstance(mapID, instanceID, lastCharmer) {
		s.motionMu.Lock()
		m := s.findCreatureMotionLocked(mapID, instanceID, rawGUID)
		if m != nil && !m.Charmed {
			if m.ThreatMgr == nil {
				m.ThreatMgr = NewThreatManager(m)
			}
			m.ThreatMgr.AddThreat(lastCharmer, 0, true)
			m.TargetGUID = lastCharmer
			m.InCombat = true
			engaged = true
		}
		s.motionMu.Unlock()
		if engaged {
			startPkt := buildAttackStart(rawGUID, lastCharmer)
			s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, nil)
		}
	}
	if lastCharmer != 0 && !engaged {
		// The no-engage tail: C++ clears LastCharmerGUID and, still out of
		// combat, evades home (EnterEvadeMode(EVADE_REASON_NO_HOSTILES)) —
		// the evade clears the threat table CombatStop kept, restores health,
		// and resets the tap. triggerCreatureEvade locks motionMu itself and
		// fires Lua hooks, so the motion is re-looked-up and re-checked
		// unlocked, matching the threat.go:849 pattern; a motion that
		// re-engaged or despawned in between keeps its current state.
		s.motionMu.Lock()
		m := s.findCreatureMotionLocked(mapID, instanceID, rawGUID)
		evade := m != nil && !m.Charmed && !m.InCombat
		s.motionMu.Unlock()
		if evade {
			s.triggerCreatureEvade(context.Background(), m, time.Now())
		}
	}
}

// charmerResolvesInInstance mirrors ObjectAccessor::GetUnit for the OnCharmed
// engage leg (CreatureAI.cpp:59): the charmer must be a live unit in the
// creature's map/instance, player or creature. Call without the motion lock
// held — findSessionByGUID takes sessionsMu.
func (s *Server) charmerResolvesInInstance(mapID, instanceID uint32, guid uint64) bool {
	if s == nil || guid == 0 {
		return false
	}
	if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil &&
		sess.player.Map == mapID && sess.player.InstanceID == instanceID {
		return true
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	return s.findCreatureMotionLocked(mapID, instanceID, guid) != nil
}

// triggerCreatureEvade resets a creature's combat state, clears threat & auras,
// restores health to max, and routes it back to its spawn position with Evading = true.
// Reference: TrinityCore Creature::EnterEvadeMode (Creature.cpp).
//
// CreatureAI::_EnterEvadeMode (CreatureAI.cpp:294-316) leg audit, in C++ relative
// order: RemoveAurasOnEvade (Unit.cpp:4343) is bridged via clearCreatureAuras with
// two deltas — C++ keeps SPELL_AURA_CONTROL_VEHICLE/CLONE_CASTER auras (Go has no
// aura-type model for either, so the clear is unconditional) and skips the removal
// entirely for charmed/player-owned creatures (vacuous here — OwnerGUID != 0
// motions return through updatePetMotion ahead of the combat tick, so they never
// reach this function). CombatStop(true) is bridged (threat own-table and
// victim halves, c3701e6/dc4885d; attack stop). LoadCreaturesAddon has no bridge —
// Go models only creature_addon/creature_template_addon's path_id (loadCreaturePathID);
// addon flags/emotes/auras/mount are unmodeled. SetLootRecipient(nullptr) is
// bridged via the tap reset below (TapPlayerGUID/TapGroupID=0, TAPPED flag
// clear); ResetPlayerDamageReq is bridged (PlayerDamageReq=MaxHealth/2);
// SetLastDamagedTime(0) is bridged (LastDamaged zeroed, CreatureAI.cpp:311).
// SetCannotReachTarget(false)
// is vacuous (the flag can never be set, see the no-path note below).
// DoNotReacquireSpellFocusTarget is vacuous (creature casts acquire no spell focus).
// SetTarget(Empty) is bridged (TargetGUID=0). GetSpellHistory()->ResetAllCooldowns()
// is vacuous — SpellCooldowns/SpellCategoryCooldowns live only on pet motions
// (pet_cooldowns.go, pets.go:2022, pet_save.go), and pet motions never route here,
// while world creature casts (castCreatureSpell, the combat tick below) never read
// or write either map, so there is nothing to reset. EngagementOver()->AtDisengage()
// has no AI-disengage model; the Go analog is InCombat=false plus the Eluna
// On_Reset/On_LeaveCombat hooks already fired above, in Eluna::EnterEvadeMode
// (CreatureHooks.cpp:202) order. Health is restored at evade start where C++
// restores it on home arrival (HomeMovementGenerator::DoFinalize::SetSpawnHealth);
// that delta is documented with the JustReachedHome note below.
//
// Call-site EvadeReason audit (CreatureAI.h:93-98): CreatureAI::_EnterEvadeMode
// takes EvadeReason /*why*/ (CreatureAI.cpp:294) — the reason never branches the
// evade body, so coverage is a call-site-reachability question. NO_PATH is closed
// (f18f6c7, no bridge). NO_HOSTILES ("the creature's threat list is empty"): the
// Go analog is the combat-tick site at creaturemotion.go:847 — target
// invalid/gone with an empty threat table routes here. Sub-site deltas:
// CreatureAI.cpp:261 (UpdateVictim, REACT_PASSIVE && !InCombat) has a lighter Go
// analog at creaturemotion.go:805-812 (inline ClearThreat + InCombat=false, no
// home-walk, no health restore, no Eluna hooks); CreatureAI.cpp:67 (OnCharmed —
// engage the last charmer, evade if still not in combat) is bridged in
// uncharmCreature (resolvable charmers engaged; the restore evades home
// when it does not engage); PassiveAI.cpp:51 (engaged && !InCombat) is
// vacuous as a separate site — in Go engagement is InCombat. BOUNDARY ("the creature h
// evade boundary"): Creature::Update (Creature.cpp:830-838) calls
// AI()->CheckInRoom() every 2.5s while engaged → EnterEvadeMode at
// CreatureAI.cpp:425. Go has no boundary model (no SetBoundary/IsInBounds
// analog; the instance GetBossBoundary arm is unmodeled, matching the per-boss
// Lua notes); the visibility-range leash at creaturemotion.go:1895 is the
// open-world analog only (dungeons skip it per CanCreatureAttack :2585).
// SEQUENCE_BREAK ("boss prerequisites not defeated"):
// BossAI::_JustEngagedWith (ScriptedCreature.cpp:530-535, CheckRequiredBosses
// fail) plus the hadronox / blood_prince_council / lady_deathwhisper /
// sindragosa / valithria_dreamwalker boss scripts — no bridge: Go has no
// CheckRequiredBosses / instance boss-state model, consistent with the per-boss
// "BossAI bookkeeping has no bridges" notes. OTHER: this function is the single
// funnel for all unreasoned calls — covered; the CritterAI flee-done site
// (PassiveAI.cpp:85) can never fire — Go has no UNIT_STATE_FLEEING model
// (critter fleeing is unmodeled; item 6's critter bullet stays open).
// triggerCritterFlee bridges CritterAI::JustEngagedWith (PassiveAI.cpp:76-79):
// a critter (creature_template.type 8, non-guardian) attacked by a player
// flees — SetControlled(true, UNIT_STATE_FLEEING) -> SetFeared(true) ->
// MoveFleeing(caster, CONFIG_CREATURE_FAMILY_FLEE_DELAY = 7000ms, Unit.cpp:11712).
// Go has no fleeing movement generator: the bridge launches a single
// straight-line run spline away from the attacker and arms FleeingUntil;
// the step tick skips combat/wander while it runs and evades home on expiry
// (CritterAI::OnMovementGeneratorFinalized, PassiveAI.cpp:81-86). The 24yd
// leg approximates the generator's first repath at melee range
// (frand(0.4,1.3) * (MIN_QUIET_DISTANCE 28 - casterDistance),
// FleeingMovementGenerator.cpp:204-208); the continuous repathing, the
// UNIT_FLAG_FLEEING cosmetic, and spell-damage engages (Go never engages
// creatures on damage) are documented deltas. Re-attacks while fleeing are
// no-ops, matching the HasUnitState(FLEEING) return in JustEngagedWith.
func (s *Server) triggerCritterFlee(ctx context.Context, target combatTarget, attackerX, attackerY float32) {
	if s == nil || target.GUID == 0 {
		return
	}
	now := time.Now()
	s.motionMu.Lock()
	motion := s.findCreatureMotionLocked(target.Map, target.InstanceID, target.GUID)
	if motion == nil || motion.CreatureType != 8 || motion.OwnerGUID != 0 || motion.Health == 0 || now.Before(motion.FleeingUntil) {
		s.motionMu.Unlock()
		return
	}
	dx := float64(motion.X - attackerX)
	dy := float64(motion.Y - attackerY)
	dist := math.Hypot(dx, dy)
	if dist < 0.2 {
		dx, dy, dist = 1, 0, 1
	}
	const fleeDist = 24.0
	destX := motion.X + float32(dx/dist*fleeDist)
	destY := motion.Y + float32(dy/dist*fleeDist)
	speed := creatureSplineVelocity(motion, false)
	if speed <= 0 {
		speed = creatureBaseRunSpeed
	}
	duration := splineDurationMs(fleeDist, speed)
	mapID, instanceID, rawGUID := motion.Map, motion.InstanceID, motion.GUID
	fromX, fromY, fromZ := motion.X, motion.Y, motion.Z
	motion.X, motion.Y = destX, destY
	motion.Moving = true
	motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
	motion.FleeingUntil = now.Add(7 * time.Second)
	s.motionMu.Unlock()
	s.broadcastMonsterMoveInInstance(mapID, instanceID, rawGUID, fromX, fromY, fromZ, destX, destY, fromZ, duration, false, 0, false)
}

func (s *Server) triggerCreatureEvade(ctx context.Context, motion *creatureMotion, now time.Time) {
	if motion == nil {
		return
	}
	if motion.TransportGUID != 0 {
		motion.Moving = false
		return
	}
	wasInCombat := motion.InCombat
	// Eluna::EnterEvadeMode (CreatureHooks.cpp:202) calls On_Reset first —
	// CREATURE_EVENT_ON_RESET, event 23, a void hook (START_HOOK at
	// CreatureHooks.cpp:277), return discarded — then fires
	// CREATURE_EVENT_ON_LEAVE_COMBAT (event 2) ahead of
	// ScriptedAI::EnterEvadeMode(). A Lua handler returning boolean true
	// vetoes the whole base evade — threat clear, move home, and reset are
	// all skipped, so the creature keeps its combat state. No motion lock
	// is held on this path, so the hooks run inline.
	if wasInCombat {
		s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnReset)
		if s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnLeaveCombat) {
			return
		}
	}
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	stopPkt := buildAttackStop(motion.GUID, motion.TargetGUID, false)
	s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
	s.broadcastThreatClearInInstance(motion.Map, motion.InstanceID, motion.GUID)
	// ThreatManager::RemoveMeFromThreatLists (ThreatManager.cpp:690-697):
	// evade drops the creature from everyone else's threat table too, so no
	// stale entry keeps another creature hunting an evaded/reset target.
	// The own-table half (ClearAllThreat, ThreatManager.cpp:483-492) is the
	// ClearThreat + SMSG_THREAT_CLEAR broadcast above.
	s.removeThreatVictimFromAllLists(motion.Map, motion.InstanceID, motion.GUID)
	if motion.BossAI != nil {
		s.clearInstanceEncounter(motion)
		motion.BossAI.OnEvade(ctx, s, motion)
	}
	motion.InCombat = false
	motion.TargetGUID = 0
	// CombatAI::Reset (CombatAI.cpp:64-67): evade clears the AI event
	// schedule; the next engage re-arms it via the JustEngagedWith pre-arm.
	motion.SpellEventTimes = nil
	// CreatureAI::_EnterEvadeMode -> Unit::CombatStop(true) interrupts a
	// non-melee cast in flight (Unit.cpp:5809-5812), so the cast-state
	// gate clears with the event schedule.
	motion.CastingUntil = time.Time{}
	motion.Health = motion.MaxHealth
	// CreatureAI::_EnterEvadeMode (CreatureAI.cpp:311): the leash's
	// last-damaged timer resets with the evade.
	motion.LastDamaged = time.Time{}
	// CreatureAI::EnterEvadeMode (CreatureAI.cpp:307-311): the tap and the
	// damage requirement reset with the evade (SetLootRecipient(nullptr)
	// clears UNIT_DYNFLAG_TAPPED, so it rides the health broadcast).
	motion.TapPlayerGUID, motion.TapGroupID = 0, 0
	motion.PlayerDamageReq = motion.MaxHealth / 2
	motion.DynamicFlags &^= unitDynFlagTapped
	if s != nil {
		s.clearCreatureAuras(creatureAuraKeyForMotion(motion))
		s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{
			unitFieldHealth:       motion.MaxHealth,
			unitFieldDynamicFlags: motion.DynamicFlags,
		})
	}
	homeDist := float32(math.Hypot(float64(motion.HomeX-motion.X), float64(motion.HomeY-motion.Y)))
	if homeDist > 0.5 {
		speed := creatureSplineVelocity(motion, false)
		if speed <= 0 {
			speed = creatureBaseRunSpeed
		}
		duration := splineDurationMs(float64(homeDist), speed)
		s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, motion.HomeX, motion.HomeY, motion.HomeZ, duration, false, 0, false)
		motion.X, motion.Y, motion.Z = motion.HomeX, motion.HomeY, motion.HomeZ
		motion.Moving = true
		motion.Evading = true
		motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
		motion.WaitUntil = motion.MoveEnds
	} else {
		motion.X, motion.Y, motion.Z = motion.HomeX, motion.HomeY, motion.HomeZ
		motion.Moving = false
		motion.Evading = false
		// HomeMovementGenerator<Creature>::DoFinalize (HomeMovementGenerator.cpp:151)
		// fires AI()->JustReachedHome() when the home move completes, including
		// the instant case where the creature is already at home (the spline
		// finalizes immediately, so movementInform fires). Eluna::
		// JustReachedHome (CreatureHooks.cpp:220) is START_HOOK_WITH_RETVAL
		// (CREATURE_EVENT_ON_REACH_HOME, 24; args (event, creature)); the veto
		// gates only ScriptedAI::JustReachedHome, which no ScriptedAI subclass
		// overrides — BossAI::_JustReachedHome is me->setActive(false) and
		// Go's native boss AIs carry no ReachedHome model — so the return is
		// discarded as a provable no-op, matching the JustDied/SpellHit
		// rulings. Health was already restored to max at evade start, matching
		// DoFinalize's SetSpawnHealth ahead of the hook.
		s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnReachHome)
	}
}

func (s *Server) loadCreaturePathID(ctx context.Context, guid, entry uint32) uint32 {
	var pathID int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(ca.path_id, 0), NULLIF(cta.path_id, 0), ?) FROM creature AS c LEFT JOIN creature_addon AS ca ON ca.guid = c.guid LEFT JOIN creature_template_addon AS cta ON cta.entry = c.id WHERE c.guid = ?", guid, guid).Scan(&pathID); err != nil {
		return guid
	}
	if pathID == 0 {
		return guid
	}
	return uint32(pathID)
}

func (s *Server) loadWaypoints(ctx context.Context, pathID uint32) []waypointPoint {
	query := "SELECT position_x, position_y, position_z, orientation, move_type, delay FROM waypoint_data WHERE id = ? ORDER BY point"
	rows, err := s.WorldStore.DB.QueryContext(ctx, query, pathID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var points []waypointPoint
	for rows.Next() {
		var p waypointPoint
		var x, y, z, orientation float64
		var moveType, delay int64
		if err := rows.Scan(&x, &y, &z, &orientation, &moveType, &delay); err != nil {
			continue
		}
		p.X, p.Y, p.Z, p.Orientation, p.MoveType, p.Delay = float32(x), float32(y), float32(z), float32(orientation), uint32(moveType), uint32(delay)
		points = append(points, p)
	}
	return points
}

// updateActiveCreatures drives wander/patrol/combat motion for creatures near
// online players, mirroring RandomMovementGenerator, WaypointMovementGenerator,
// and TargetedMovementGenerator behaviour.
func (s *Server) updateActiveCreatures(ctx context.Context) {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var players []playerPos
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil {
			isGM := (sess.player.ExtraFlags&playerExtraGMOn != 0) || (sess.player.PlayerFlags&playerFlagGM != 0)
			// Phase-anywhere rides on GM_ON; serverside-hidden visibility
			// rides on the GM visibility detect (see creatures.go) — a GM
			// after `.gm visible on` keeps phase-anywhere but loses hidden.
			seesHidden := sess.gmVisibilityDetect != 0
			isDead := (sess.player.Health == 0 && sess.player.MaxHealth > 0) || sess.player.PlayerFlags&playerFlagGhost != 0
			players = append(players, playerPos{
				Map:             sess.player.Map,
				InstanceID:      sess.player.InstanceID,
				X:               sess.player.X,
				Y:               sess.player.Y,
				Z:               sess.player.Z,
				GUID:            sess.playerGUID,
				Race:            sess.player.Race,
				Class:           sess.player.Class,
				Level:           sess.player.Level,
				IsGM:            isGM,
				SeesHidden:      seesHidden,
				IsDead:          isDead,
				FactionTemplate: s.raceFaction(sess.player.Race),
				Reputations:     playerReputationMap(sess.player.Reputations),
				Sess:            sess,
			})
		}
	}
	s.sessionsMu.RUnlock()
	if len(players) == 0 {
		return
	}
	now := time.Now()
	s.pruneCreatureMotion(now)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 100.0
	}
	query := `SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, c.orientation, c.MovementType, c.wander_distance,
		COALESCE(NULLIF(t.speed_walk, 0), 1.0), COALESCE(NULLIF(t.speed_run, 0), 1.14286),
		COALESCE(t.faction, 0), COALESCE(t.maxlevel, 1), COALESCE(t.unit_flags, 0), COALESCE(t.dynamicflags, 0), COALESCE(t.flags_extra, 0), COALESCE(NULLIF(t.BaseAttackTime, 0), 2000),
		c.curhealth
		FROM creature AS c
		JOIN creature_template AS t ON t.entry = c.id
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (? OR c.phaseMask = 0 OR (c.phaseMask & 1) <> 0)
		AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(t.npcflag, 0) & 0xC000) = 0))`
	seenCreatures := make(map[creatureMotionSpawnKey]struct{})
	for _, p := range players {
		rows, err := s.WorldStore.DB.QueryContext(ctx, query, p.Map, float64(p.X)-distance, float64(p.X)+distance, float64(p.Y)-distance, float64(p.Y)+distance, p.IsGM, p.SeesHidden, p.IsDead)
		if err != nil {
			continue
		}
		for rows.Next() {
			var guid, entry, moveType, faction, level, unitFlags, dynamicFlags, flagsExtra, attackTime, curHealth int64
			var x, y, z, orientation, wander, walkSpeed, runSpeed float64
			if err := rows.Scan(&guid, &entry, &x, &y, &z, &orientation, &moveType, &wander, &walkSpeed, &runSpeed, &faction, &level, &unitFlags, &dynamicFlags, &flagsExtra, &attackTime, &curHealth); err != nil {
				continue
			}
			spawnKey := creatureMotionSpawnKey{MapID: p.Map, InstanceID: p.InstanceID, GUID: uint32(guid)}
			if _, dup := seenCreatures[spawnKey]; dup {
				continue
			}
			seenCreatures[spawnKey] = struct{}{}
			if curHealth <= 0 {
				continue
			}
			motion := s.motionFor(ctx, uint32(guid), uint32(entry), p.Map, p.InstanceID, float32(x), float32(y), float32(z), float32(orientation), uint32(moveType), wander, float32(walkSpeed), float32(runSpeed), uint32(curHealth))
			motion.Faction = uint32(faction)
			motion.Level = uint32(level)
			motion.UnitFlags = uint32(unitFlags)
			motion.DynamicFlags = uint32(dynamicFlags)
			motion.FlagsExtra = uint32(flagsExtra)
			if !motion.ReactStateKnown {
				if reactState, known, aiName := s.loadCreatureReaction(ctx, uint32(entry)); known {
					motion.ReactState = reactState
					motion.ReactStateKnown = true
					motion.AIName = aiName
				}
			}
			motion.AttackTime = uint32(attackTime)
			// Re-affirm absolute velocities on every sweep: motions created by
			// older builds may still hold the raw template rate in Speed
			// (see the motionFor rate/absolute fix) or a flat RunSpeed.
			motion.Speed = creatureWalkVelocity(walkSpeed)
			motion.RunSpeed = creatureRunVelocity(runSpeed)
			if motion.MaxHealth == 0 {
				health := uint32(curHealth)
				if health == 0 {
					health = 42
				}
				motion.Health, motion.MaxHealth = health, health
			}
			if motion.MaxHealth > 0 && motion.Health > motion.MaxHealth {
				motion.Health = motion.MaxHealth
			}
			if motion.Health == 0 {
				continue
			}
			// OnAllCreatureUpdate analog: re-evaluate AutoBalance attributes;
			// the recalc early-out keeps this cheap when nothing changed.
			s.autoBalanceModifyCreatureAttributes(ctx, motion, false)
			s.stepCreatureMotion(ctx, motion, players, now)
		}
		rows.Close()
	}
}

// stepCreatureMotion advances one creature: handles combat pursuit/attacks,
// finishes in-flight moves, honors waypoint delays, or wanders randomly.
// triggerCreatureAlert mirrors CreatureAI::TriggerAlert (CreatureAI.cpp),
// which CreatureUnitRelocationWorker (GridNotifiers.cpp:139) fires when a
// stealthed player sits inside the alert band but outside normal detect
// range. Gates: the target is a player (the scan only iterates players);
// the creature is an NPC not currently engaged (!InCombat — the scan only
// runs there); not confused, stunned, fleeing, or distracted — distracted
// rides DistractedUntil, confuse/stun/fear ride creatureHasControlLossAura
// (the same aura-gate the Distract effect bridge uses), fleeing has no Go
// model; civilian and REACT_PASSIVE ride the scan's isCreaturePassive guard
// (civilian maps to passive in creatureReactState); hostility rides
// isAttackableFaction (the _IsTargetAcceptable friendly/targetable arms ride
// the scan guard's GM/dead/ghost-visibility skips; the vehicle arm is
// vacuous). The UNIT_STATE_SIGHTLESS pre-check of the relocation worker has
// no Go model. Effect: the pre-aggro AI_REACTION_ALERT sound (AiReaction 0,
// SharedDefines.h:3253) plus MoveDistract(5s, facing the player). The
// distracted gate also rate-limits: re-alerts during the 5s hold are
// no-ops, matching C++ re-fires on every relocation tick while distracted.
func (s *Server) triggerCreatureAlert(ctx context.Context, motion *creatureMotion, p playerPos, dist, aggroDist float32, now time.Time) {
	if s == nil || motion == nil || p.Sess == nil || p.Sess.player == nil {
		return
	}
	inBand, alertRange := creatureStealthAlertBand(motion, p.Sess, dist)
	// WorldObject::CanDetectStealthOf (Object.cpp:1782-1784): no alert when
	// the alert range reaches the creature's attack distance.
	if !inBand || alertRange >= aggroDist {
		return
	}
	if !s.isAttackableFaction(motion.Faction, p) {
		return
	}
	if now.Before(motion.DistractedUntil) {
		return
	}
	if s.creatureHasControlLossAura(creatureAuraKeyForMotion(motion)) {
		return
	}
	// Unit::GetAbsoluteAngle(who): atan2 normalized to [0, 2pi).
	angle := float32(math.Atan2(float64(p.Y-motion.Y), float64(p.X-motion.X)))
	if angle < 0 {
		angle += 2 * math.Pi
	}
	motion.Orientation = angle
	motion.DistractedUntil = now.Add(5 * time.Second)
	motion.Moving = false
	s.broadcastAIReactionInInstance(motion.Map, motion.InstanceID, motion.GUID, 0)
	// DistractMovementGenerator::Initialize launches an in-place MoveTo+SetFacing
	// spline; Go reuses the facing arm of the monster move packet for the
	// client-side turn, matching the Distract effect bridge.
	s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, motion.X, motion.Y, motion.Z, 1000, false, angle, true)
	_ = ctx
}

func (s *Server) stepCreatureMotion(ctx context.Context, motion *creatureMotion, players []playerPos, now time.Time) {
	if motion == nil {
		return
	}
	if motion.TransportGUID != 0 {
		motion.Moving = false
		return
	}
	if motion.MaxHealth == 0 {
		health := uint32(math.Max(float64(motion.Level)*30, 42))
		motion.Health, motion.MaxHealth = health, health
	}
	if motion.Health == 0 {
		motion.InCombat = false
		motion.TargetGUID = 0
		motion.Moving = false
		return
	}
	if motion.Charmed && motion.OwnerGUID == 0 {
		return
	}
	if isCreaturePassive(motion) && motion.InCombat {
		if motion.ThreatMgr != nil {
			motion.ThreatMgr.ClearThreat()
		}
		motion.InCombat = false
		motion.TargetGUID = 0
		motion.Moving = false
	}

	// Pet AI handling if this creature is a player pet
	if motion.OwnerGUID != 0 {
		s.updatePetMotion(ctx, motion, players, now)
		return
	}
	// ThreatManager::Update (ThreatManager.cpp:199-209): the 1s AI-tick
	// re-runs victim selection (UpdateVictim -> ReselectVictim, 516-585).
	// The tick is the only C++ path that re-gates the whole list, so it
	// catches current-victim threat decay, taunt-aura expiry, and fixate
	// changes the eager AddThreat gate never revisits.
	if motion.InCombat && motion.ThreatMgr != nil {
		inMeleeOf := func(victimGUID uint64) bool {
			for i := range players {
				if players[i].GUID != victimGUID || players[i].Sess == nil || players[i].Sess.player == nil {
					continue
				}
				dist := distance3D(motion.X, motion.Y, motion.Z, players[i].X, players[i].Y, players[i].Z)
				return inMeleeThreatRange(motion.CombatReach, players[i].Sess.player.CombatReach, dist)
			}
			return false
		}
		if switched, newVictim, dirty := motion.ThreatMgr.Update(100, inMeleeOf); switched && newVictim != 0 {
			motion.TargetGUID = newVictim
			s.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, motion.ThreatMgr.SortedEntries())
		} else if dirty {
			// The list moved without a victim switch: SendThreatListToClients
			// sends SMSG_THREAT_UPDATE (ThreatManager.cpp:524, 770-787).
			s.broadcastThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.ThreatMgr.SortedEntries())
		}
	}
	if motion.Moving {
		if now.Before(motion.MoveEnds) {
			return
		}
		motion.Moving = false
	}

	// CritterAI flee (PassiveAI.cpp:76-93): while the flee timer runs the
	// critter keeps fleeing (combat/wander skipped); on expiry the timed
	// fleeing generator finalizes and CritterAI::OnMovementGeneratorFinalized
	// evades — the light analog here walks home (the critter was never
	// engaged in Go's model, so the full evade funnel's threat/aura/combat
	// resets are vacuous; the home walk below mirrors triggerCreatureEvade's
	// tail exactly). Killed mid-flee is handled by the Health==0 gate above.
	if !motion.FleeingUntil.IsZero() {
		if now.Before(motion.FleeingUntil) {
			return
		}
		motion.FleeingUntil = time.Time{}
		homeDist := float32(math.Hypot(float64(motion.HomeX-motion.X), float64(motion.HomeY-motion.Y)))
		if homeDist > 0.5 {
			speed := creatureSplineVelocity(motion, false)
			if speed <= 0 {
				speed = creatureBaseRunSpeed
			}
			duration := splineDurationMs(float64(homeDist), speed)
			s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, motion.HomeX, motion.HomeY, motion.HomeZ, duration, false, 0, false)
			motion.X, motion.Y, motion.Z = motion.HomeX, motion.HomeY, motion.HomeZ
			motion.Moving = true
			motion.Evading = true
			motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
			motion.WaitUntil = motion.MoveEnds
			return
		}
	}

	// 1. If currently in combat with a target:
	if motion.InCombat && motion.TargetGUID != 0 {
		var target *playerPos
		for i := range players {
			if players[i].GUID == motion.TargetGUID && players[i].Map == motion.Map {
				target = &players[i]
				break
			}
		}
		// If target left map, logged out, dead, or turned on GM mode: drop threat
		if target == nil || target.IsDead || target.IsGM || target.Sess == nil || target.Sess.player == nil || target.Sess.isDeadOrGhost() || (motion.FlagsExtra&0x00000400 != 0 && !target.IsDead) {
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.RemoveThreat(motion.TargetGUID)
			}
			if motion.ThreatMgr != nil && !motion.ThreatMgr.IsEmpty() {
				nextVictim := motion.ThreatMgr.GetCurrentVictim()
				motion.TargetGUID = nextVictim
				entries := motion.ThreatMgr.SortedEntries()
				s.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, nextVictim, entries)
				return
			}
			s.triggerCreatureEvade(ctx, motion, now)
			return
		}
		dist := float32(math.Hypot(float64(target.X-motion.X), float64(target.Y-motion.Y)))
		// Creature::Update no-path evade arm (Creature.cpp:924-930, CREATURE_NOPATH_EVADE_TIME
		// = 5 * IN_MILLISECONDS, CreatureData.h:139) — document-only, no Go bridge.
		// In C++ the ChaseMovementGenerator sets the m_cannotReachTarget flag when the
		// target is in an inaccessible place (Unit::isInAccessiblePlaceFor: target in
		// water requires the creature to swim, otherwise it must walk or fly —
		// ChaseMovementGenerator.cpp:164) or when pathfinding fails outright
		// (NOPATH, :200); SetCannotReachTarget resets the 5s timer on every change
		// (Creature.cpp:3001-3010, clear sites at :207/CreatureAI.cpp:312/
		// ScriptedCreature.cpp:259/Creature.cpp:2127), and the timer arm skips raid
		// maps before calling AI()->EnterEvadeMode(EVADE_REASON_NO_PATH). Go has no
		// PathGenerator — pursuit runs straight-line to the target's XYZ below, so
		// the path-fail site can never fire; and creatureMotion carries CanFly only
		// (no CanSwim/CanWalk flags) while player positions carry no liquid status,
		// so the accessible-place predicate cannot be evaluated either. The flag can
		// never be set, so the timer can never start.
		// TurretAI (CombatAI.cpp:253-258): AttackStart(who) calls
		// me->Attack(who, false) — no chase (meleePossible=false) — and the
		// turret only ever casts spell[0] inside its range band. It
		// legitimately engages as far out as the CanStartAttack arm above,
		// so the open-world leash is extended by its spell max range.
		// Creature::CanCreatureAttack (Creature.cpp:2560-2608): the leash
		// measures the VICTIM's distance from HOME (IsInDist, :2607 — 2D
		// for flight, :2604-2605), not the creature-to-victim chase gap;
		// the radius is min(map visibility range, 2*SIZE_OF_GRID_CELL) +
		// both combat reaches (:2593-2601) — 100yd on continents, 133.33yd
		// in BGs/arenas — and dungeon maps skip the leash entirely (:2585).
		// The recently-damaged (MAX_AGGRO_RESET_TIME = 10s, Unit.h:40; DoT
		// ticks never stamp LastDamaged, Unit.cpp:903) and any-live-taunt
		// (HasAuraType(SPELL_AURA_MOD_TAUNT), :2589) skips apply
		// creature-wide, not per victim. World bosses (Creature.cpp:2353:
		// type_flags & CREATURE_TYPE_FLAG_BOSS_MOB, SharedDefines.h:2731)
		// leash regardless of recent damage. The charmer/player gate
		// (!GetCharmerOrOwnerGUID().IsPlayer(), :2583) is vacuous:
		// OwnerGUID != 0 motions return through updatePetMotion ahead of
		// the combat tick and never reach this site. The config-overridable
		// visibility distances are unmodeled; the C++ defaults are used.
		inDungeon := false
		leashBase := float32(100.0)
		if s != nil && s.Data != nil {
			if mapEntry, found, err := s.Data.Map(motion.Map); err == nil && found {
				inDungeon = mapEntry.IsDungeon()
				if mapEntry.IsBattleground() || mapEntry.IsBattleArena() {
					leashBase = float32(133.3333)
				}
			}
		}
		leashDist := leashBase
		if motion.AIName == "TurretAI" {
			leashDist += turretSpellMaxRange(s, motion)
		}
		leashVictimReach := float32(1.5)
		if target.Sess != nil && target.Sess.player != nil && target.Sess.player.CombatReach > 0 {
			leashVictimReach = target.Sess.player.CombatReach
		}
		leashCReach := motion.CombatReach
		if leashCReach <= 0 {
			leashCReach = 1.5
		}
		leashDist += leashCReach + leashVictimReach
		var homeDist float32
		if motion.CanFly {
			homeDist = float32(math.Hypot(float64(target.X-motion.HomeX), float64(target.Y-motion.HomeY)))
		} else {
			homeDist = float32(distance3D(target.X, target.Y, target.Z, motion.HomeX, motion.HomeY, motion.HomeZ))
		}
		if !inDungeon && homeDist > leashDist {
			taunted := motion.ThreatMgr != nil && motion.ThreatMgr.HasAnyTaunt()
			recentlyDamaged := !motion.LastDamaged.IsZero() && now.Sub(motion.LastDamaged) < 10*time.Second
			bossMob := motion.TypeFlags&creatureTypeFlagBossMob != 0
			if bossMob || (!recentlyDamaged && !taunted) {
				// Evade / drop combat if player ran too far: reset health, stop attack, and run back home
				if motion.ThreatMgr != nil {
					motion.ThreatMgr.RemoveThreat(motion.TargetGUID)
				}
				if motion.ThreatMgr != nil && !motion.ThreatMgr.IsEmpty() {
					nextVictim := motion.ThreatMgr.GetCurrentVictim()
					motion.TargetGUID = nextVictim
					entries := motion.ThreatMgr.SortedEntries()
					s.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, nextVictim, entries)
					return
				}
				s.triggerCreatureEvade(ctx, motion, now)
				return
			}
		}

		if motion.BossAI != nil {
			motion.BossAI.OnUpdate(ctx, s, motion, 200*time.Millisecond, players, now)
		}

		if len(motion.Spells) == 0 && s != nil && s.WorldStore != nil && s.WorldStore.DB != nil {
			motion.Spells = s.loadCreatureSpells(ctx, motion.Entry)
		}
		victimReach := float32(1.5)
		if target.Sess != nil && target.Sess.player != nil && target.Sess.player.CombatReach > 0 {
			victimReach = target.Sess.player.CombatReach
		}
		cReach := motion.CombatReach
		if cReach <= 0 {
			cReach = 1.5
		}
		contactDist := float32(calcMeleeRange(cReach, victimReach))
		// CombatAI::UpdateAI / CasterAI::UpdateAI (CombatAI.cpp:90-106,
		// 153-166): the first due AICOND_COMBAT spell event fires and
		// re-arms; a fired spell preempts the melee swing below on this
		// tick (spell/melee exclusivity, bridged 08:14). Range gating uses
		// the due spell's own range band; self/buff-targeted spells cast
		// packet-only regardless of range (C++ CastSpell at me).
		// CasterAI::UpdateAI (CombatAI.cpp:155-159): the victim's
		// breakable-by-damage crowd-control check runs BEFORE the
		// cast-state gate — an AI-held breakable CC aura on the victim
		// interrupts the creature's non-melee casts
		// (InterruptNonMeleeSpells(false)) and suppresses the whole
		// spell/melee tick, so the creature never breaks its own crowd
		// control.
		if motion.AIName == "CasterAI" && target.Sess != nil && s != nil && s.aiVictimHasBreakableCC(motion, target.Sess) {
			motion.CastingUntil = time.Time{}
			return
		}
		// CombatAI::UpdateAI / CasterAI::UpdateAI (CombatAI.cpp:97, 159):
		// UNIT_STATE_CASTING early-out — no spell event fires and no
		// melee swing starts while a non-triggered cast is in flight.
		if now.Before(motion.CastingUntil) {
			return
		}
		spellID, dueSpell, dueTargetGUID, dueOK := s.dueAISpell(ctx, motion, target.GUID, now, players, true)
		if dueOK && dueTargetGUID == 0 {
			// UnitAI::DoCast (UnitAI.cpp:113-163): a null SelectTarget pick
			// (ENEMY/DEBUFF with no in-range candidate) returns
			// SPELL_FAILED_BAD_TARGETS — nothing casts, but
			// CombatAI::UpdateAI still consumes and re-arms the event.
			s.rearmAISpell(motion, dueSpell, spellID, now)
			return
		}
		// The DoCast victim arm's range gate is Spell::CheckRange, not the
		// tick's 2D dist with the melee fudge (see aiDoCastVictimInRange).
		victimInRange := true
		if dueOK && dueTargetGUID == target.GUID {
			victimInRange = s.aiDoCastVictimInRange(motion, target, dueSpell, cReach, victimReach)
		}
		if dueOK && (dueTargetGUID != target.GUID || victimInRange) {
			if dueTargetGUID != target.GUID {
				// DoCast at a non-victim target (UnitAI.cpp:113-163): self
				// buffs, ally heals, or random-enemy picks — packet-only
				// (the damage/log legs below are victim-player specific and
				// have no C++ DoCast arm here).
				if target.Sess != nil {
					target.Sess.debug("creature spell attack", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "target_guid", dueTargetGUID)
				}
				s.castCreatureSpell(ctx, motion, spellID, dueTargetGUID)
				s.rearmAISpell(motion, dueSpell, spellID, now)
				return
			}
			if target.Sess != nil {
				target.Sess.debug("creature spell attack", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "target_guid", target.GUID)
			}
			castID := uint8(1)
			castTimeStamp := uint32(now.UnixMilli())
			hitTargets := []uint64{target.GUID}
			spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: target.GUID}
			if s != nil && s.Data != nil {
				if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
					spellTarget = spellGoPacketTarget(spellInfo, spellTarget)
				}
			}
			goPkt := protocol.BuildSpellGo(motion.GUID, motion.GUID, castID, spellID, spellCastFlagGo, castTimeStamp, hitTargets, nil, spellTarget)
			if target.Sess != nil {
				_ = target.Sess.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
			}
			if s != nil {
				s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, target.Sess)
			}

			schoolMask := uint8(1)
			damage, hasDBCSpell := uint32(0), false
			if s != nil && s.Data != nil {
				if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
					if spellInfo.SchoolMask != 0 {
						schoolMask = uint8(spellInfo.SchoolMask)
					}
					damage, hasDBCSpell = creatureSpellDamage(s, spellInfo, motion.Level, false)
				}
			}
			if !hasDBCSpell {
				lvl := float64(motion.Level)
				if lvl < 1 {
					lvl = 1
				}
				baseDmg := lvl * 1.5
				damage = uint32(baseDmg + rand.Float64()*(baseDmg*0.5))
				if damage < 1 {
					damage = 1
				}
			}
			// AutoBalance_UnitScript::ModifySpellDamageTaken analog.
			damage = s.autoBalanceModifyDealDamage(motion, target.Sess, damage)
			overkill := uint32(0)
			if target.Sess != nil && target.Sess.player != nil {
				// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD victims take no
				// damage — the kill/health legs are skipped explicitly (not via a
				// damage==0 gate) because absorbed-to-zero damage still runs them
				// in C++. The damage log below keeps the original amount, matching
				// C++ sending SMSG_SPELLNONMELEEDAMAGELOG before DealDamage
				// (Spell.cpp:2542).
				godNegated := target.Sess.godCheatActive()
				// Unit::DealDamage (Unit.cpp:766-788):
				// SPELL_AURA_SHARE_DAMAGE_PCT copies CalculatePct(damage,
				// amount) to the aura's caster.
				if !godNegated && damage > 0 {
					target.Sess.splitShareDamagePct(ctx, target.GUID, true, creatureAuraKey{}, motion.GUID, damage, uint32(schoolMask))
				}
				// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED
				// fires for any damage to a player victim, creature-caster
				// spells included.
				target.Sess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
				victimHealth := target.Sess.player.Health
				// Duel defeat (Unit.cpp:825-853, 957-973): a charmed creature's
				// spell lands as a duel defeat when its charmer is the duel
				// opponent (GetControllingPlayer, Unit.cpp:5996); a wild
				// creature's exactly-health-1 hit still completes the duel as
				// won, while its lethal hit kills and interrupts.
				if !godNegated && duelDefeatOnDamage(target.Sess, target.Sess.controllingPlayerGUID(motion.GUID), damage, victimHealth) {
					// Duel defeat consumed the hit — loser at 1 HP, duel complete.
				} else if !godNegated && damage >= victimHealth {
					overkill = damage - victimHealth
					target.Sess.player.Health = 0
					target.IsDead = true
					target.Sess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
					// GetCharmerOrOwnerPlayerOrPlayerItself (Unit.cpp:11279):
					// a player-owned pet's kill credits its owner; a wild
					// creature resolves to no player.
					var creatureKiller *session
					if motion.OwnerGUID != 0 {
						creatureKiller = s.findSessionByGUID(motion.OwnerGUID)
					}
					target.Sess.killPlayer(ctx, creatureKiller, false, true)
					// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill
					// player-victim branch — attacker is a wild creature (no
					// owner player, so no pet arm):
					// attacker->AI()->KilledUnit(victim) (Unit.cpp:11359-11361).
					s.fireCreatureTargetDied(ctx, motion, target.Sess.luaPlayer())
					if motion.BossAI != nil {
						motion.BossAI.OnKillPlayer(ctx, s, motion, target.GUID)
					}
				} else if !godNegated {
					target.Sess.player.Health -= damage
					// Unit::DealDamage (Unit.cpp:915-924): rage from damage received.
					target.Sess.grantRageFromDamageTaken(ctx, damage)
					// Unit::DealDamage (Unit.cpp:906-913): random durability
					// loss on HIT TAKEN — the victim is a player.
					target.Sess.rollDurabilityLossOnHit(ctx, damage)
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
						target.Sess.delayCurrentCast()
						target.Sess.delayCurrentChannel()
					}
					target.Sess.procDamageAuras(true)
				}
				logPkt := buildSpellNonMeleeDamageLog(target.GUID, motion.GUID, spellID, damage, overkill, schoolMask)
				_ = target.Sess.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, true)
				if s != nil {
					s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, target.Sess)
				}
				target.Sess.lastCombatTime = now
				if target.Sess.player.UnitFlags&unitFlagInCombat == 0 {
					target.Sess.player.UnitFlags |= unitFlagInCombat
				}
				target.Sess.sendPlayerUpdate()
				// Unit::DealDamage (Unit.cpp:728-733): the victim's controlled
				// creatures are signaled OwnerAttackedBy on any non-DoT damage.
				if s != nil {
					s.triggerPetDefensive(target.Map, target.InstanceID, target.GUID, motion.GUID)
				}
			}
			motion.LastSpell = now
			// Unit::CastSpell: a non-triggered cast holds UNIT_STATE_CASTING
			// for its DBC cast time; the tick's early-out above bridges
			// CombatAI.cpp:97 / CasterAI.cpp:159.
			if castMs := s.aiSpellCastTimeMs(dueSpell); castMs > 0 {
				motion.CastingUntil = now.Add(time.Duration(castMs) * time.Millisecond)
			}
			s.rearmAISpell(motion, dueSpell, spellID, now)
			// CombatAI::UpdateAI (CombatAI.cpp:90-106): a fired spell event
			// and the melee swing are mutually exclusive per tick — the
			// event arm runs instead of DoMeleeAttackIfReady, so a spell
			// tick never also swings, at any range.
			return
		}

		if dist > contactDist {
			// Pursue player: move towards target at run speed.
			// TurretAI never pursues (CombatAI.cpp:253-258: AttackStart ->
			// me->Attack(who, false), meleePossible=false); it stands and
			// casts spell[0] when the victim sits in the range band, so an
			// out-of-band victim just ends the tick here.
			if motion.AIName != "TurretAI" {
				// ChaseMovementGenerator::Update (ChaseMovementGenerator.cpp:98-106):
				// UNIT_STATE_NOT_MOVE (root | stun | died | distracted, Unit.h:256)
				// pauses the chase. Died rides the tick's Health gate and
				// distracted rides DistractedUntil above; fear and confuse
				// replace the chase with the fleeing and confused generators,
				// which have no Go model (documented no-bridge), so they are
				// not paused here. The melee swing below still fires while
				// stunned or rooted (DoMeleeAttackIfReady gates UNIT_STATE_CASTING
				// only, UnitAI.cpp:61-63), so only the pursuit pauses.
				if !s.creatureHasNotMoveAura(creatureAuraKeyForMotion(motion)) {
					// ChaseMovementGenerator re-paths whenever the target moves
					// (_lastTargetPosition, ChaseMovementGenerator.cpp:157-159):
					// relaunch the spline when the victim left the launch
					// point so a kiting victim can't outrun a stale spline.
					targetMoved := motion.ChaseTX != target.X || motion.ChaseTY != target.Y
					if !motion.Moving || now.After(motion.MoveEnds) || targetMoved {
						// ChaseMovementGenerator paths to the victim's center
						// then shortens to maxTarget = CONTACT_DISTANCE +
						// hitboxSum (CONTACT_DISTANCE 0.5, ObjectDefines.h:23;
						// ShortenPathUntilDist, ChaseMovementGenerator.cpp:211):
						// the creature stops at melee-ring contact, not on the
						// victim's center.
						shorten := float32(0.5) + cReach + victimReach
						destX, destY := target.X, target.Y
						if dist > shorten {
							destX = target.X + (motion.X-target.X)/dist*shorten
							destY = target.Y + (motion.Y-target.Y)/dist*shorten
						}
						// init.SetFacing(target) (ChaseMovementGenerator.cpp:217):
						// face the victim for the whole spline.
						faceAngle := float32(math.Atan2(float64(target.Y-motion.Y), float64(target.X-motion.X)))
						if faceAngle < 0 {
							faceAngle += 2 * math.Pi
						}
						motion.Orientation = faceAngle
						duration := splineDurationMs(float64(dist), creatureSplineVelocity(motion, false))
						s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, target.Z, duration, false, faceAngle, true)
						motion.X, motion.Y, motion.Z = destX, destY, target.Z
						motion.ChaseTX, motion.ChaseTY = target.X, target.Y
						motion.Moving = true
						motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
					}
				}
			}
			return
		}
		// In melee range: attack player.
		// TurretAI::UpdateAI (CombatAI.cpp:259-265) has no melee arm — it
		// only casts spell[0] via DoSpellAttackIfReady inside the range
		// band (already gated above) — so a turret never swings.
		// CasterAI::UpdateAI (CombatAI.cpp:167-186) likewise never calls
		// DoMeleeAttackIfReady — a CasterAI creature casts only, and never
		// melee-swings, even in melee range.
		if motion.AIName == "TurretAI" || motion.AIName == "CasterAI" {
			return
		}
		motion.Moving = false
		attackTime := time.Duration(motion.AttackTime) * time.Millisecond
		if attackTime <= 0 {
			attackTime = 2 * time.Second
		}
		if motion.LastAttack.IsZero() || now.Sub(motion.LastAttack) >= attackTime {
			var damage uint32
			if motion.MinDamage > 0 && motion.MaxDamage >= motion.MinDamage {
				variance := float64(motion.MaxDamage - motion.MinDamage)
				baseDmg := float64(motion.MinDamage)
				if variance > 0 {
					baseDmg += rand.Float64() * variance
				}
				damage = uint32(baseDmg)
			} else {
				lvl := float64(motion.Level)
				if lvl < 1 {
					lvl = 1
				}
				attSpeed := float64(motion.AttackTime) / 1000.0
				if attSpeed <= 0 {
					attSpeed = 2.0
				}
				minDmg := lvl * 0.75 * attSpeed
				maxDmg := lvl * 1.25 * attSpeed
				if minDmg < 1 {
					minDmg = 1
				}
				if maxDmg < minDmg {
					maxDmg = minDmg + 1
				}
				damage = uint32(minDmg + rand.Float64()*(maxDmg-minDmg))
			}
			// AutoBalance_UnitScript::ModifyMeleeDamage analog.
			damage = s.autoBalanceModifyDealDamage(motion, target.Sess, damage)
			if target.Sess.player.Armor > 0 {
				damage = calcArmorReducedDamage(float64(target.Sess.player.Armor), uint8(motion.Level), damage)
			}
			if damage < 1 {
				damage = 1
			}

			isPlayerVictim := target.Sess != nil && target.Sess.player != nil
			targetLevel := uint8(1)
			if isPlayerVictim && target.Sess.player.Level > 0 {
				targetLevel = target.Sess.player.Level
			}
			canBlock := isPlayerVictim && target.Sess.player.CanBlock && target.Sess.player.Block > 0
			canParry := isPlayerVictim && (target.Sess.player.Level >= 10 || target.Sess.player.Level == 0)
			canDodge := true
			if isPlayerVictim {
				// Player defender cannot block, parry, or dodge if creature is attacking from behind
				// (Unit::RollMeleeOutcomeAgainst, Unit.cpp:2213-2217). SPELL_AURA_IGNORE_HIT_DIRECTION
				// (288) exempts the victim from the behind-arc kill.
				attackerInFront := hasInArc(target.Sess.player.Orientation, target.Sess.player.X, target.Sess.player.Y, motion.X, motion.Y, math.Pi)
				if !attackerInFront && !target.Sess.hasAuraType(spellAuraIgnoreHitDirection) {
					canBlock = false
					canParry = false
					canDodge = false
				}
				// A victim mid cast-bar cast cannot avoid (Unit.cpp:2219-2224:
				// victim->IsNonMeleeSpellCast(false); UNIT_STATE_CONTROLLED has
				// no Go unit-state model, stays unbridged).
				if target.Sess.genericCastInProgress() {
					canDodge = false
					canParry = false
					canBlock = false
				}
			}
			victimDodgeBP := int32(-1)
			if isPlayerVictim && target.Sess != nil && target.Sess.player != nil {
				victimDodgeBP = int32(math.Round(float64(target.Sess.player.DodgePercentage) * 100))
			}
			outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, false, isPlayerVictim, false, canBlock, canParry, canDodge, 0, 0, 0, 0, victimDodgeBP, s.creatureDodgeReductionBP(creatureAuraKeyForMotion(motion)))
			if isPlayerVictim && target.Sess != nil {
				if target.Sess.isImmuneToDamage(1) {
					outcome = protocol.MeleeHitImmune
					hitInfo = protocol.HitInfoMiss
					targetState = protocol.VictimStateIsImmune
				}
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
			case protocol.MeleeHitCrushing:
				damage = uint32(float64(damage) * 1.5)
			}

			// Handle player parry haste when player parries creature attack
			if targetState == protocol.VictimStateParry && isPlayerVictim {
				pMainSpeed := time.Duration(target.Sess.player.AttackTime) * time.Millisecond
				if pMainSpeed <= 0 {
					pMainSpeed = 2 * time.Second
				}
				elapsed := now.Sub(target.Sess.lastSwing)
				if elapsed < pMainSpeed {
					rem := pMainSpeed - elapsed
					hasted := calcParryHastedRemaining(rem, pMainSpeed)
					target.Sess.lastSwing = now.Add(-(pMainSpeed - hasted))
				}
			}

			overkill := uint32(0)
			// absorbedDmg carries the pre-absorb portion for the rage legs
			// (Unit.cpp:815-924): the received leg converts damage + absorbed.
			absorbedDmg := uint32(0)
			if damage > 0 && isPlayerVictim {
				// Unit::CalcAbsorbResist (Unit.cpp:1839-1857): the creature
				// attacker's MOD_TARGET_ABSORB_SCHOOL (194) pct bypasses absorbs.
				bypass := absorbIgnoreBypass(damage, s.creatureAbsorbIgnorePct(creatureAuraKeyForMotion(motion), 1))
				absorbed, rem := target.Sess.applyAbsorptionShields(damage-bypass, 1)
				absorbedDmg = absorbed
				damage = rem + bypass
				if rem == 0 && absorbed > 0 {
					hitInfo |= protocol.HitInfoFullAbsorb
				} else if absorbed > 0 {
					hitInfo |= protocol.HitInfoPartialAbsorb
				}
			}
			// Unit::DealDamage (Unit.cpp:735-737): CHEAT_GOD negates the damage
			// after absorption (C++ absorbs before DealDamage) — the application
			// block below is then skipped: no health loss, no death, no pushback,
			// no procs. lastCombatTime / in-combat still update, as entering
			// combat precedes DealDamage in C++.
			damage = target.Sess.negateGodModeDamage(damage)
			// Unit::DealDamage (Unit.cpp:815-819): rage from fully absorbed
			// damage — ahead of the application block the god arm skips (the
			// god arm itself returns before this leg in C++).
			if damage == 0 && absorbedDmg > 0 && !target.Sess.godCheatActive() {
				target.Sess.grantRageFromDamageTaken(ctx, absorbedDmg)
				// Unit::DealDamage (Unit.cpp:742-747): absorbed damage still
				// strips TAKE_DAMAGE-interrupt auras — the removal runs before
				// the !damage early-return.
				target.Sess.procDamageAuras(false)
				// Unit::DealDamage (Unit.cpp:749-761): fully absorbed non-DoT
				// damage aborts ABORT_ON_DMG casts (creature attacker !=
				// player victim).
				target.Sess.interruptAbsorbedCast()
			}
			if damage > 0 {
				// Unit::DealDamage (Unit.cpp:766-788):
				// SPELL_AURA_SHARE_DAMAGE_PCT.
				target.Sess.splitShareDamagePct(ctx, target.GUID, true, creatureAuraKey{}, motion.GUID, damage, 1)
				// Unit::DealDamage (Unit.cpp:869-870): HIGHEST_HIT_RECEIVED
				// fires for any damage to a player victim, wild-creature
				// attackers included.
				target.Sess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
				victimHealth := target.Sess.player.Health
				// Duel defeat (Unit.cpp:825-853, 957-973): a charmed creature's
				// blow lands as a duel defeat when its charmer is the duel
				// opponent (GetControllingPlayer, Unit.cpp:5996); a wild
				// creature's exactly-health-1 hit still completes the duel as
				// won, while its lethal hit kills and interrupts.
				if duelDefeatOnDamage(target.Sess, target.Sess.controllingPlayerGUID(motion.GUID), damage, victimHealth) {
					// Duel defeat consumed the hit — loser at 1 HP, duel complete.
				} else if damage >= victimHealth {
					overkill = damage - victimHealth
					target.Sess.player.Health = 0
					target.IsDead = true
					target.Sess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
					// GetCharmerOrOwnerPlayerOrPlayerItself (Unit.cpp:11279):
					// a player-owned pet's kill credits its owner; a wild
					// creature resolves to no player.
					var creatureKiller *session
					if motion.OwnerGUID != 0 {
						creatureKiller = s.findSessionByGUID(motion.OwnerGUID)
					}
					target.Sess.killPlayer(ctx, creatureKiller, false, true)
					if motion.BossAI != nil {
						motion.BossAI.OnKillPlayer(ctx, s, motion, target.GUID)
					}
				} else {
					target.Sess.player.Health -= damage
					// Unit::DealDamage (Unit.cpp:915-924): rage from damage
					// received — damage + absorbed for the conversion.
					target.Sess.grantRageFromDamageTaken(ctx, damage+absorbedDmg)
					// Unit::DealDamage (Unit.cpp:906-913): random durability
					// loss on HIT TAKEN — the victim is a player.
					target.Sess.rollDurabilityLossOnHit(ctx, damage)
					// Reference Unit::DealDamage -> Spell::Delayed / DelayedChannel
					target.Sess.delayCurrentCast()
					target.Sess.delayCurrentChannel()
					target.Sess.procDamageAuras(true, damage)
				}
				target.Sess.lastCombatTime = now
				if target.Sess.player != nil && target.Sess.player.UnitFlags&unitFlagInCombat == 0 {
					target.Sess.player.UnitFlags |= unitFlagInCombat
				}
			}
			asuPkt := protocol.BuildAttackerStateUpdate(motion.GUID, target.GUID, damage, overkill, hitInfo, targetState, blocked)
			target.Sess.debug("creature melee attack", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "target_guid", target.GUID)
			_ = target.Sess.write(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, true)
			if s != nil {
				s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, target.Sess)
			}
			target.Sess.sendPlayerUpdate()
			if s != nil {
				s.triggerPetDefensive(target.Map, target.InstanceID, target.GUID, motion.GUID)
			}
			motion.LastAttack = now
		}
		return
	}

	// 2. If creature is currently in Evade mode returning home:
	// maintain attack/spell immunity, ignore aggro, and finish return when reached
	if motion.Evading {
		if now.Before(motion.MoveEnds) {
			return
		}
		motion.Evading = false
		motion.Moving = false
		motion.X, motion.Y, motion.Z = motion.HomeX, motion.HomeY, motion.HomeZ
		motion.WaitUntil = motion.MoveEnds
		// Home-move completion: HomeMovementGenerator<Creature>::DoFinalize
		// (HomeMovementGenerator.cpp:151) calls AI()->JustReachedHome() when
		// the spline finalizes after evade — Eluna::JustReachedHome
		// (CreatureHooks.cpp:220), CREATURE_EVENT_ON_REACH_HOME (24), args
		// (event, creature). Same provable no-op veto ruling as the instant
		// arm above: the veto gates ScriptedAI::JustReachedHome, which only
		// BossAI overrides (me->setActive(false), unmodeled in Go), so the
		// return is discarded.
		s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnReachHome)
		return
	}

	// 3. Check for nearby hostile aggro
	for _, p := range players {
		if p.Map != motion.Map || p.InstanceID != motion.InstanceID || p.IsGM || p.IsDead || (motion.FlagsExtra&0x00000400 != 0) || isCreaturePassive(motion) || creatureCombatDisabled(motion.UnitFlags, motion.FlagsExtra) {
			continue
		}
		dist := float32(distance3D(p.X, p.Y, p.Z, motion.X, motion.Y, motion.Z))
		// Creature::GetAttackDistance (Creature.cpp:2022-2058): 20 yards at equal
		// level, minus combat reach, +/-1 yard per creature-minus-player level
		// difference, clamped to [5, 45]. (Creature::GetAggroRange, 3130-3168,
		// is the pet-only variant — world-creature acquisition uses
		// GetAttackDistance.) Documented deltas: the SPELL_AURA_MOD_DETECT_RANGE /
		// MOD_DETECTED_RANGE terms are unmodeled (no aura-modifier query on
		// creatures or players); the RATE_CREATURE_AGGRO multiplier and the
		// expansion-max-level clamp are unmodeled (no rate model); the
		// + m_CombatDistance term of CanStartAttack (Creature.cpp:1987-1989)
		// is unmodeled (creature motions carry no combat-distance field).
		levelDiff := int32(motion.Level) - int32(p.Level)
		aggroDist := float32(20.0) - motion.CombatReach + float32(levelDiff)
		if aggroDist < 5.0 {
			aggroDist = 5.0
		} else if aggroDist > 45.0 {
			aggroDist = 45.0
		}
		// TurretAI (CombatAI.cpp:239-242): m_SightDistance = m_CombatDistance
		// = spell[0]'s max range, and Creature::CanStartAttack acquires at
		// GetAttackDistance + m_CombatDistance (Creature.cpp:1986), so a
		// turret sights far beyond the standard aggro radius.
		if motion.AIName == "TurretAI" {
			if len(motion.Spells) == 0 && s != nil && s.WorldStore != nil && s.WorldStore.DB != nil {
				motion.Spells = s.loadCreatureSpells(ctx, motion.Entry)
			}
			aggroDist += turretSpellMaxRange(s, motion)
		}
		if !canCreatureDetectStealthOfPlayer(motion, p.Sess, dist) {
			// CreatureAI::TriggerAlert (CreatureAI.cpp) via
			// CreatureUnitRelocationWorker (GridNotifiers.cpp:139): a stealthed
			// player inside the alert band but outside normal detect range
			// makes the creature play the pre-aggro alert sound and turn to
			// face them for 5 seconds instead of engaging.
			s.triggerCreatureAlert(ctx, motion, p, dist, aggroDist, now)
			continue
		}
		if !isCreaturePassive(motion) && s.isAttackableFaction(motion.Faction, p) && canCreatureStartAttack(motion, p, dist, aggroDist) && !noGrayAggroBlocked(uint32(p.Level), motion.Level, s.Config.NoGrayAggroAbove, s.Config.NoGrayAggroBelow) && s.hasLineOfSight(motion.Map, motion.X, motion.Y, motion.Z, p.X, p.Y, p.Z) {
			s.debug("creature aggro", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "player_guid", p.GUID, "player_zone", p.Sess.player.Zone)
			// Eluna CREATURE_EVENT_ON_MOVE_IN_LOS (27): a boolean true vetoes
			// the default aggro engage, mirroring ElunaCreatureAI::
			// MoveInLineOfSight's `if (!sEluna->MoveInLineOfSight(me, who))
			// ScriptedAI::MoveInLineOfSight(who)`.
			if s.fireCreatureMoveInLOS(ctx, motion, p.Sess) {
				continue
			}
			// Aggro acquisition audit vs CreatureAI::MoveInLineOfSight
			// (CreatureAI.cpp:118-123) + Creature::CanStartAttack
			// (Creature.cpp:1960-2004): the IsEngaged return lands via block
			// order (this scan only runs when !InCombat); the REACT_AGGRESSIVE
			// gate lands via the !isCreaturePassive guard (C++ InitializeReactState,
			// Creature.cpp:1255-1266, also only distinguishes passive vs
			// aggressive — the civilian arm is commented out there too);
			// stealth detection rides canCreatureDetectStealthOfPlayer
			// (stealth only — the CanSeeOrDetect invisibility legs are
			// unmodeled); LOS rides hasLineOfSight; the civilian arm of
			// CanStartAttack rides the passive mapping; CallAssistance is
			// absent here as in C++ (sight-aggro only engages via
			// Unit::EngageWithTarget, Unit.cpp:8429-8438 — assistance fires on
			// AttackStart, not on acquisition). Documented no-bridge arms: the
			// home-distance gate of CanCreatureAttack (Creature.cpp:2582-2599 —
			// non-dungeon victim must sit within visibility-range/2-cell of
			// home unless recently damaged or taunted; Go puts no
			// home-proximity limit on acquisition); the gray-aggro config
			// (CheckNoGrayAggroConfig, Creature.cpp:2000-2013) is bridged in
			// the guard above via noGrayAggroBlocked — vacuous under the
			// default NoGrayAggro.Above/Below = 0 config (World.cpp:1299);
			// the immune-to-NPC/PC target pairing of CanStartAttack
			// (Creature.cpp:1964-1970 — creatureCombatDisabled only models the
			// creature-side IMMUNE_TO_PC bit, not the target-side pairing);
			// PetAI's aggressive-pet acquisition (PetAI.cpp:352,
			// SelectNearestHostileUnitInAggroRange — Go pets never
			// auto-acquire). Bridged: Unit::EngageWithTarget seeds 0.0f threat
			// (Unit.cpp:8429-8438) — Go seeds 0.0 at the add below, and
			// AddThreat registers the new-victim entry and runs the victim
			// leg on a zero add (ThreatManager::AddThreat new-target arm).
			motion.InCombat = true
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(motion)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			if isCreaturePassive(motion) {
				continue
			}
			// Unit::EngageWithTarget seeds 0.0f threat (Unit.cpp:8429-8438) — the
			// add still registers the entry and runs the victim leg
			// (ThreatManager::AddThreat new-target arm, ThreatManager.cpp:308-410).
			motion.ThreatMgr.AddThreat(p.GUID, 0.0, true)
			motion.TargetGUID = p.GUID
			motion.Moving = false
			s.broadcastAIReactionInInstance(motion.Map, motion.InstanceID, motion.GUID, 2)
			p.Sess.lastCombatTime = now
			if p.Sess.player != nil && p.Sess.player.UnitFlags&unitFlagInCombat == 0 {
				p.Sess.player.UnitFlags |= unitFlagInCombat
				p.Sess.sendPlayerUpdate()
			}
			startPkt := buildAttackStart(motion.GUID, p.GUID)
			_ = p.Sess.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
			s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, p.Sess)
			if motion.BossAI != nil {
				if p.Sess != nil {
					s.beginInstanceEncounter(motion, p.Sess.player)
				}
				motion.BossAI.OnAggro(ctx, s, motion, p.GUID)
			}
			// CombatAI::JustEngagedWith AICOND_AGGRO arm (CombatAI.cpp:76-88):
			// aggro-condition template spells are cast once at the engage
			// victim, non-triggered; the AICOND_COMBAT event pre-arm
			// (CombatAI::JustEngagedWith / CasterAI::JustEngagedWith,
			// CombatAI.cpp:76-88, 139-162) is scheduled here too.
			s.castAggroConditionSpells(ctx, motion, p.GUID)
			if motion.AIName == "CasterAI" {
				s.scheduleCasterAISpellEvents(ctx, motion, p.GUID, now, players, true)
			} else {
				s.scheduleAISpellEvents(motion, now)
			}
			// Creature::AtEngage arm (Creature.cpp:3431-3453): formation
			// members assist per CreatureGroup::MemberEngagingTarget
			// (CreatureGroups.cpp:226-255). Fresh engage by construction —
			// the acquisition scan only runs when !InCombat.
			s.memberEngagingTarget(ctx, motion, p.GUID)
			return
		}
	}

	// 3. Normal wandering or waypoint patrolling
	// Distract hold (Spell::EffectDistract, SpellEffects.cpp:2547): the
	// creature stands facing the distraction until the timer expires; the
	// C++ DistractMovementGenerator replaces the idle-slot generator for the
	// duration, so wandering/waypoint movement pauses here. Aggro acquisition
	// above still runs — the C++ generator does not suppress it.
	if now.Before(motion.DistractedUntil) {
		return
	}
	if motion.Moving {
		if now.Before(motion.MoveEnds) {
			return
		}
		motion.Moving = false
		motion.WaitUntil = motion.MoveEnds
		if motion.MoveType == 2 && len(motion.Points) > 0 {
			// WaypointMovementGenerator::OnArrived (WaypointMovementGenerator.cpp):
			// the C++ arrival hook calls AI->MovementInform(WAYPOINT_MOTION_TYPE,
			// _currentNode); Eluna::MovementInform (CreatureHooks.cpp:182-190) turns
			// it into CREATURE_EVENT_ON_REACH_WP (6) with (type=2, id=node index).
			// Go's CreatureEventOnReachWP was defined but never fired, so any Lua
			// patrol/escort script registered on event 6 was dead. NextIdx was
			// advanced at launch, so the arrived node is (NextIdx-1) mod len.
			// The veto return gates ScriptedAI::MovementInform — a provable no-op
			// here (only PetAI overrides MovementInform, for POINT_MOTION_TYPE pet
			// returns, and pets never run waypoint generators), so it is discarded
			// like the ON_REACH_HOME arm in the evade section.
			arrived := (motion.NextIdx - 1 + len(motion.Points)) % len(motion.Points)
			s.fireCreatureLuaEvent(ctx, motion, scripting.CreatureEventOnReachWP, uint32(waypointMotionType), uint32(arrived))
			// WaypointMovementGenerator::StartMove: "if (waypoint.orientation &&
			// waypoint.delay) init.SetFacing(waypoint.orientation)" — the node's
			// facing applies at arrival, only when both are set (0 orientation is
			// the DB default meaning "no facing"). The client-side turn rode the
			// Final_Angle arm of the SMSG_MONSTER_MOVE packet at launch; this sets
			// the logical orientation to match, the way X/Y/Z already jump.
			if point := motion.Points[arrived]; point.Orientation != 0 && point.Delay > 0 {
				motion.Orientation = point.Orientation
			}
		}
	}
	if now.Before(motion.WaitUntil) {
		return
	}
	if motion.MoveType == 2 && len(motion.Points) == 0 {
		return
	}
	// WaypointMovementGenerator::DoUpdate (WaypointMovementGenerator.cpp:126-132)
	// and RandomMovementGenerator::DoUpdate (RandomMovementGenerator.cpp:204-210):
	// UNIT_STATE_NOT_MOVE (or LOST_CONTROL) / a preventing cast interrupts the
	// generator and no new leg is launched until the state clears. Go's atomic
	// splines make the mid-leg StopMoving unbridgeable; the launch gate is the
	// observable arm — a rooted or casting patrol no longer teleports along its
	// path while stunned. UNIT_STATE_LOST_CONTROL (fear/confuse generators)
	// stays unmodeled (no Go fear/confuse generator), matching the chase arm's
	// documented delta.
	if s.creatureHasNotMoveAura(creatureAuraKeyForMotion(motion)) || now.Before(motion.CastingUntil) {
		return
	}
	// FormationMovementGenerator::DoUpdate (FormationMovementGenerator.cpp):
	// a member pulled into formation moves on the leader's spline rhythm
	// instead of its own wander/waypoint legs. Combat takes precedence (the
	// C++ chase generator is pushed over the formation generator), and while
	// the member's own leg is in flight the tick returns at the Moving gate
	// above — C++ would relaunch mid-spline on a new leader spline, which
	// the atomic-spline model cannot express (documented delta; the 1200ms
	// interval re-check picks it up on the next tick).
	if motion.FormationActive && !motion.InCombat {
		if s.stepFormationMember(motion, now) {
			return
		}
	}
	var destX, destY, destZ float32
	var speed float32
	var wait time.Duration
	var facing float32
	var hasFacing bool
	walk := true
	if motion.MoveType == 2 {
		point := motion.Points[motion.NextIdx]
		destX, destY, destZ = point.X, point.Y, point.Z
		walk = point.MoveType == 0 // WAYPOINT_MOVE_TYPE_WALK (WaypointDefines.h:26); default RUN
		speed = creatureSplineVelocity(motion, walk)
		if point.Delay > 0 {
			wait = time.Duration(point.Delay) * time.Second
		}
		// WaypointMovementGenerator::StartMove (WaypointMovementGenerator.cpp):
		// "if (waypoint.orientation && waypoint.delay) init.SetFacing(waypoint.orientation)"
		// — the destination facing rides the spline's Final_Angle arm only when
		// BOTH are set; 0 orientation is the DB default meaning "no facing".
		// The logical orientation itself is set at arrival (see the arrival
		// block above), matching the C++ spline-end timing.
		if point.Orientation != 0 && point.Delay > 0 {
			facing, hasFacing = point.Orientation, true
		}
		motion.NextIdx = (motion.NextIdx + 1) % len(motion.Points)
		// WaypointMovementGenerator::DoUpdate (WaypointMovementGenerator.cpp:169-175):
		// while the spline is in motion, every update sets the home position
		// to the creature's current position ("set home position at place").
		// The transport guard (MOVEMENTFLAG_ONTRANSPORT with a trans GUID)
		// has no Go model — Go creatures never move on transports — so the
		// arm is unconditional. Go's model jumps to the leg destination at
		// launch, so the "current position" for the rest of the leg is the
		// destination: a creature that aggros mid-patrol and then evades
		// returns to where it was patrolling, not to its original spawn.
		// (RandomMovementGenerator carries no such arm. The orientation in
		// C++ SetHomePosition(x,y,z,o) has no Go HomeO model and stays
		// unmodeled, as does the non-repeating-path end-of-path arm,
		// WaypointMovementGenerator.cpp:297-309 — Go paths repeat by
		// default.)
		motion.HomeX, motion.HomeY, motion.HomeZ = destX, destY, destZ
	} else if motion.MoveType == 1 {
		// RandomMovementGenerator<Creature>::SetRandomLocation
		// (RandomMovementGenerator.cpp): frand(0, _wanderDistance) around the
		// reference point, walking; MovePositionToFirstCollision, the LOS
		// re-check, and the PathGenerator leg (30.0 path-length limit) have no
		// bridge — Go has no PathGenerator model, consistent with the NO_PATH
		// note at the evade arm. The walk/run legs of
		// CreatureMovementData::Random (CanRun/AlwaysRun, CreatureData.h:105)
		// have no bridge either (Go models no creature_template_addon fields
		// beyond path_id). SignalFormationMovement (creature groups) is
		// unmodeled; the UNIT_STATE_NOT_MOVE/casting interruption guards of
		// SetRandomLocation (RandomMovementGenerator.cpp:204-210) ride the
		// shared launch gate above (the LOST_CONTROL leg has no Go
		// fear/confuse model).
		angle := rand.Float64() * 2 * math.Pi
		dist := rand.Float64() * motion.Wander
		destX = float32(float64(motion.HomeX) + dist*math.Cos(angle))
		destY = float32(float64(motion.HomeY) + dist*math.Sin(angle))
		destZ = motion.HomeZ
		// RandomMovementGenerator defaults to walk (RandomMovementGenerator.cpp:156-167);
		// CanRun/AlwaysRun template legs stay unmodeled (no creature_template_addon bridge).
		speed = creatureSplineVelocity(motion, true)
	} else {
		// MoveType 0 is IDLE_MOTION_TYPE (MovementDefines.h:28): Creature::Initialize
		// (Creature.cpp:543-545) demotes RANDOM to IDLE when wander_distance is 0,
		// and IDLE never wanders.
		return
	}
	moveDist := math.Hypot(float64(destX-motion.X), float64(destY-motion.Y))
	if moveDist < 0.5 {
		return
	}
	if motion.MoveType == 1 {
		// RandomMovementGenerator.cpp:SetRandomLocation: each launched spline
		// consumes one step; once the steps run out the creature rests
		// urand(4,10)s (rounded, retail) and the step counter resets to
		// urand(2,10).
		motion.WanderSteps--
		if motion.WanderSteps <= 0 {
			wait = time.Duration(4+rand.Intn(7)) * time.Second
			motion.WanderSteps = 2 + rand.Intn(9)
		}
	}
	duration := splineDurationMs(moveDist, speed)
	// The leg's travel heading and speed are recorded for the formation slot
	// predictor (FormationMovementGenerator's relativeAngle and the 1.65s
	// catchup): the atomic-spline model jumps to the destination at launch,
	// so the heading would otherwise be lost.
	motion.MoveHeading = float32(math.Atan2(float64(destY-motion.Y), float64(destX-motion.X)))
	motion.MoveVelocity = speed
	s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, walk, facing, hasFacing)
	motion.X, motion.Y, motion.Z = destX, destY, destZ
	motion.Moving = true
	motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
	motion.WaitUntil = motion.MoveEnds.Add(wait)
	// Creature::SignalFormationMovement (Creature.cpp:362-369) via
	// CreatureGroup::LeaderStartedMoving (CreatureGroups.cpp:280-295): a
	// leader launching an idle-path leg pulls FLAG_IDLE_IN_FORMATION members
	// into formation movement. PointMovementGenerator's two signal sites
	// have no Go analog (no MovePoint launch); wander/waypoint cover this
	// leg launcher.
	s.formationLeaderStartedMoving(ctx, motion, now)
}

// noGrayAggroBlocked ports Creature::CheckNoGrayAggroConfig
// (Creature.cpp:2000-2013): a creature never starts an attack on a player
// whose level renders it gray (mob_level <= GetGrayLevel(player_level),
// the XP_GRAY fallthrough of Formulas::XP::GetColorCode) when the
// NoGrayAggro.Above/Below custom switches say so — vacuous when both are 0
// (the default). The OnColorCodeCalculation script hook has no Go model.
func noGrayAggroBlocked(playerLevel, creatureLevel uint32, above, below uint32) bool {
	if creatureLevel > grayLevel(playerLevel) {
		return false
	}
	if above == 0 && below == 0 {
		return false
	}
	return playerLevel <= below || (playerLevel >= above && above > 0)
}

// canCreatureStartAttack mirrors the range arms of Creature::CanStartAttack
// (Creature.cpp:1960-2004): the non-flyer Z check (CREATURE_Z_ATTACK_RANGE,
// Creature.h:57 — the + m_CombatDistance term is unmodeled) and the
// GetAttackDistance distance check (C++ also adds m_CombatDistance there).
// The civilian, immunity, and _IsTargetAcceptable arms live in the aggro
// scan's guard and its audit note above; the gray-aggro arm
// (CheckNoGrayAggroConfig) is wired in the scan guard before the LOS check,
// matching the C++ position ahead of IsWithinLOSInMap.
// turretSpellMaxRange mirrors the m_CombatDistance arm of the TurretAI
// constructor (CombatAI.cpp:239-242): spell[0]'s max range, expressed in the
// same contactDist-relative terms as the combat tick's spell band
// (spellMaxDist = contactDist + MaxHostile).
func turretSpellMaxRange(s *Server, motion *creatureMotion) float32 {
	if s == nil || s.Data == nil || motion == nil || len(motion.Spells) == 0 {
		return 0
	}
	cReach := motion.CombatReach
	if cReach <= 0 {
		cReach = 1.5
	}
	maxDist := float32(calcMeleeRange(cReach, 1.5))
	if spellInfo, found, err := s.Data.Spell(motion.Spells[0]); err == nil && found {
		if spellRange, rangeFound, rangeErr := s.Data.SpellRange(spellInfo.RangeIndex); rangeErr == nil && rangeFound {
			maxDist += spellRange.MaxHostile
		}
	}
	return maxDist
}

func canCreatureStartAttack(motion *creatureMotion, target playerPos, distance, attackDistance float32) bool {
	return motion != nil && (motion.CanFly || math.Abs(float64(target.Z-motion.Z)) <= 3.0) && distance <= attackDistance
}

// npcEffectCanScale mirrors the canEffectScale switches in
// SpellEffectInfo::CalcValue (SpellInfo.cpp:466-504): the effect type or the
// applied aura must be one whose value scales with the NPC caster's level.
func npcEffectCanScale(effect wotlk.SpellEffect) bool {
	switch effect.Effect {
	case 2, 3, 8, 9, 10, 58, 62, 77, 121, 141, 142, 148:
		// SPELL_EFFECT_SCHOOL_DAMAGE, DUMMY, POWER_DRAIN, HEALTH_LEECH,
		// HEAL, WEAPON_DAMAGE, POWER_BURN, SCRIPT_EFFECT,
		// NORMALIZED_WEAPON_DMG, FORCE_CAST_WITH_VALUE,
		// TRIGGER_SPELL_WITH_VALUE, TRIGGER_MISSILE_SPELL_WITH_VALUE
		return true
	}
	switch effect.Aura {
	case 3, 4, 8, 15, 43, 53, 64, 69, 227:
		// SPELL_AURA_PERIODIC_DAMAGE, DUMMY, PERIODIC_HEAL, DAMAGE_SHIELD,
		// PROC_TRIGGER_DAMAGE, PERIODIC_LEECH, PERIODIC_MANA_LEECH,
		// SCHOOL_ABSORB, PERIODIC_TRIGGER_SPELL_WITH_VALUE
		return true
	}
	return false
}

// npcEffectValueScale mirrors the level-scaling term of
// SpellEffectInfo::CalcValue (SpellInfo.cpp:459-511): for a cast by a unit
// not controlled by a player, when the spell's SpellLevel is nonzero and
// differs from the caster's level, the effect carries no per-level points,
// the spell has SPELL_ATTR0_LEVEL_DAMAGE_CALCULATION, and the effect is
// scale-capable, the value is multiplied by
// GtNPCManaCostScaler[casterLevel-1] / GtNPCManaCostScaler[spellLevel-1].
// Returns 1.0 when any gate fails or a DBC lookup misses (C++ requires both
// lookups to succeed before scaling).
func (s *Server) npcEffectValueScale(spell wotlk.Spell, effect wotlk.SpellEffect, casterLevel uint32, controlledByPlayer bool) float64 {
	if controlledByPlayer || casterLevel == 0 || spell.SpellLevel == 0 || spell.SpellLevel == casterLevel || effect.RealPointsPerLevel != 0 {
		return 1
	}
	if spell.Attributes&spellAttr0LevelDamageCalculation == 0 {
		return 1
	}
	if !npcEffectCanScale(effect) {
		return 1
	}
	if s == nil || s.Data == nil {
		return 1
	}
	spellScaler, okSpell, _ := s.Data.GtNPCManaCostScaler(spell.SpellLevel)
	casterScaler, okCaster, _ := s.Data.GtNPCManaCostScaler(casterLevel)
	if !okSpell || !okCaster || spellScaler == 0 {
		return 1
	}
	return float64(casterScaler) / float64(spellScaler)
}

func creatureSpellDamage(s *Server, spell wotlk.Spell, casterLevel uint32, controlledByPlayer bool) (uint32, bool) {
	var damage uint32
	found := false
	for _, effect := range spell.Effects {
		switch effect.Effect {
		case 2, 17, 31, 58, 87:
			found = true
			value := effect.BasePoints + 1
			if value > 0 {
				scale := float64(1)
				if s != nil {
					scale = s.npcEffectValueScale(spell, effect, casterLevel, controlledByPlayer)
				}
				damage += uint32(float64(value) * scale)
			}
		}
	}
	return damage, found
}

func creatureCombatDisabled(unitFlags, flagsExtra uint32) bool {
	// UNIT_FLAG_NON_ATTACKABLE (0x00000002), UNIT_FLAG_NOT_SELECTABLE (0x02000000), UNIT_FLAG_IMMUNE_TO_PC (0x00000100)
	// CREATURE_FLAG_EXTRA_TRIGGER (0x00000080) or CREATURE_FLAG_EXTRA_NO_COMBAT (0x00002000)
	return unitFlags&(0x00000002|0x02000000|0x00000100) != 0 || flagsExtra&(0x00000080|0x00002000) != 0
}

// spellTargetUnitBlocked mirrors creatureCombatDisabled for spell target
// selection with the two ATTR6 bypasses (SharedDefines.h:637/658). C++ gates
// exactly two of the bundled terms inside
// Unit::IsValidAttackTarget/IsValidAssistTarget (Object.cpp:2972/3127/2991/3134),
// which is where implicit selection reaches them (Spell.cpp:8341/8345; the
// SpellInfo.cpp:1723 CheckTarget line is commented out, so CheckTarget
// itself never rejects these flags):
//   - UNIT_FLAG_NOT_SELECTABLE (0x02000000) is skipped when the spell carries
//     SPELL_ATTR6_CAN_TARGET_UNTARGETABLE (0x01000000).
//   - UNIT_FLAG_IMMUNE_TO_PC (0x100) is skipped for positive spells carrying
//     SPELL_ATTR6_ASSIST_IGNORE_IMMUNE_FLAG (0x8); negative spells always
//     reject. (The IsImmuneToNPC half has no Go rejection to bypass, and the
//     PvC type_flags assist gate at Object.cpp:3179 has no Go infra), and
//
// the remaining terms (NON_ATTACKABLE, TRIGGER, NO_COMBAT) have no ATTR6
// bypass in C++.
// assist selects the IsValidAssistTarget shape: the NON_ATTACKABLE /
// CREATURE_FLAG_EXTRA_TRIGGER / CREATURE_FLAG_EXTRA_NO_COMBAT bundle only
// rejects negative spells there (Object.cpp:3131), while IsValidAttackTarget
// always rejects it (Object.cpp:2980).
func spellTargetUnitBlocked(spell wotlk.Spell, unitFlags, flagsExtra uint32, assist bool) bool {
	if !assist || isHarmfulSpell(spell) {
		if unitFlags&0x00000002 != 0 || flagsExtra&(0x00000080|0x00002000) != 0 {
			return true
		}
	}
	if unitFlags&0x02000000 != 0 && spell.AttributesEx6&spellAttr6CanTargetUntargetable == 0 {
		return true
	}
	if unitFlags&0x00000100 != 0 && (isHarmfulSpell(spell) || spell.AttributesEx6&spellAttr6AssistIgnoreImmuneFlag == 0) {
		return true
	}
	return false
}

func playerReputationMap(values []playerReputation) map[uint32]playerReputation {
	if len(values) == 0 {
		return nil
	}
	reputations := make(map[uint32]playerReputation, len(values))
	for _, value := range values {
		reputations[value.FactionID] = value
	}
	return reputations
}

func (s *session) contestedPvPActive(now time.Time) bool {
	if s == nil || s.player == nil || s.player.PlayerFlags&playerFlagContestedPVP == 0 {
		return false
	}
	if !s.contestedPVPEnd.IsZero() && !now.Before(s.contestedPVPEnd) && s.attackTarget == 0 && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.PlayerFlags &^= playerFlagContestedPVP
		s.contestedPVPEnd = time.Time{}
		s.sendPlayerUpdate()
		return false
	}
	return true
}

func (s *Server) updateContestedPvP(now time.Time) {
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		sess.contestedPvPActive(now)
	}
}

func (s *Server) updatePvPFlags(now time.Time) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && !sess.pvpEnd.IsZero() && !now.Before(sess.pvpEnd) {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		if sess.pvpHostile || sess.player.PlayerFlags&playerFlagInPVP != 0 {
			continue
		}
		sess.pvpEnd = time.Time{}
		sess.player.PlayerFlags &^= playerFlagPVPTimer
		sess.player.PVPFlags &^= 0x01
		sess.sendPlayerUpdate()
	}
}

func (s *Server) isHostileFaction(creatureFaction uint32, player playerPos) bool {
	if s.Data != nil && player.FactionTemplate != 0 {
		creatureTemplate, creatureFound, creatureErr := s.Data.FactionTemplate(creatureFaction)
		playerTemplate, playerFound, playerErr := s.Data.FactionTemplate(player.FactionTemplate)
		if creatureErr == nil && playerErr == nil && creatureFound && playerFound {
			if player.Sess != nil && player.Sess.contestedPvPActive(time.Now()) && creatureTemplate.Flags&0x00001000 != 0 {
				return true
			}
			if reputation, found, err := s.Data.Reputation(creatureTemplate.Faction, player.Race, player.Class); err == nil && found && reputation.ReputationList >= 0 {
				standing := int64(reputation.BaseStanding)
				if saved, ok := player.Reputations[creatureTemplate.Faction]; ok {
					standing = int64(totalReputationStanding(saved))
				}
				return reputationRank(standing) <= 1
			}
			for _, enemy := range creatureTemplate.Enemies {
				if enemy != 0 && enemy == playerTemplate.Faction {
					return true
				}
			}
			if creatureTemplate.EnemyGroup&playerTemplate.FactionGroup != 0 {
				return true
			}
			for _, friend := range creatureTemplate.Friends {
				if friend != 0 && friend == playerTemplate.Faction {
					return false
				}
			}
			for _, friend := range playerTemplate.Friends {
				if friend != 0 && friend == creatureTemplate.Faction {
					return false
				}
			}
			if creatureTemplate.FriendGroup&playerTemplate.FactionGroup != 0 || creatureTemplate.FactionGroup&playerTemplate.FriendGroup != 0 || playerTemplate.FriendGroup&creatureTemplate.FactionGroup != 0 || playerTemplate.FactionGroup&creatureTemplate.FriendGroup != 0 {
				return false
			}
			return creatureTemplate.Flags&0x00002000 != 0
		}
	}
	return isHostileFactionFallback(creatureFaction, player.Race)
}

// isAttackableFaction extends isHostileFaction with the at-war term of the
// PvC attack path. A player-owned unit treats a reputation faction's units as
// hostile exactly when the player is at war with that faction
// (WorldObject::GetReactionTo, Object.cpp:2776-2785 — "if faction has
// reputation, hostile state depends only from AtWar state"), and
// WorldObject::IsValidAttackTarget refuses the PvC/CvP attack outright when
// the player is not at war with the creature's faction (Object.cpp:3036-
// 3053), which is what Creature::CanCreatureAttack evaluates for aggro
// (Creature.cpp:2560-2565). The reverse direction — the creature's own
// reaction to the player — only clamps the rank down to neutral when at war
// (Object.cpp:2817-2823), which never flips a rank<=HOSTILE verdict, so the
// questgiver/taxi checks that use the creature's reaction keep the plain
// isHostileFaction (Player.cpp:17159, TaxiHandler.cpp:46).
func (s *Server) isAttackableFaction(creatureFaction uint32, player playerPos) bool {
	if s.isHostileFaction(creatureFaction, player) {
		return true
	}
	if s.Data == nil || player.FactionTemplate == 0 {
		return false
	}
	creatureTemplate, found, err := s.Data.FactionTemplate(creatureFaction)
	if err != nil || !found {
		return false
	}
	reputation, found, err := s.Data.Reputation(creatureTemplate.Faction, player.Race, player.Class)
	if err != nil || !found || reputation.ReputationList < 0 {
		return false
	}
	saved, ok := player.Reputations[creatureTemplate.Faction]
	return ok && saved.Flags&factionFlagAtWar != 0
}

// isFriendlyFaction mirrors the friendly half of the isHostileFaction
// lookup: a reputation rank of friendly (4) or better when the player has a
// standing row for the creature's faction and is not at war with it, else the
// faction-template friend lists and friend/faction-group cross terms. Neutral
// factions are neither friendly nor hostile. At war the player's own reaction
// is hostile regardless of standing (Object.cpp:2776-2785), so the reputation
// branch reports not friendly.
func (s *Server) isFriendlyFaction(creatureFaction uint32, player playerPos) bool {
	if s.Data != nil && player.FactionTemplate != 0 {
		creatureTemplate, creatureFound, creatureErr := s.Data.FactionTemplate(creatureFaction)
		playerTemplate, playerFound, playerErr := s.Data.FactionTemplate(player.FactionTemplate)
		if creatureErr == nil && playerErr == nil && creatureFound && playerFound {
			if reputation, found, err := s.Data.Reputation(creatureTemplate.Faction, player.Race, player.Class); err == nil && found && reputation.ReputationList >= 0 {
				standing := int64(reputation.BaseStanding)
				if saved, ok := player.Reputations[creatureTemplate.Faction]; ok {
					standing = int64(totalReputationStanding(saved))
					// At-war term of the player-side reaction
					// (WorldObject::GetReactionTo, Object.cpp:2776-2785): at war
					// with the faction the player is hostile to its units
					// regardless of standing, so never friendly.
					if saved.Flags&factionFlagAtWar != 0 {
						return false
					}
				}
				return reputationRank(standing) >= 4
			}
			for _, friend := range creatureTemplate.Friends {
				if friend != 0 && friend == playerTemplate.Faction {
					return true
				}
			}
			for _, friend := range playerTemplate.Friends {
				if friend != 0 && friend == creatureTemplate.Faction {
					return true
				}
			}
			if creatureTemplate.FriendGroup&playerTemplate.FactionGroup != 0 || creatureTemplate.FactionGroup&playerTemplate.FriendGroup != 0 || playerTemplate.FriendGroup&creatureTemplate.FactionGroup != 0 || playerTemplate.FactionGroup&creatureTemplate.FriendGroup != 0 {
				return true
			}
		}
	}
	return isFriendlyFactionFallback(creatureFaction, player.Race)
}

func isFriendlyFactionFallback(creatureFaction uint32, playerRace uint8) bool {
	switch creatureFaction {
	case 14, 16, 17, 38, 48, 91, 100, 101, 102, 103, 104, 105, 106, 117, 168, 188, 189, 214, 254:
		return false
	}
	isAlliance := isAllianceRace(playerRace)
	switch creatureFaction {
	case 1, 3, 4, 11, 12, 55, 57, 72, 115:
		return isAlliance
	case 2, 5, 6, 8, 10, 29, 67, 68, 76, 116:
		return !isAlliance
	}
	return false
}

func isHostileFactionFallback(creatureFaction uint32, playerRace uint8) bool {
	if creatureFaction == 0 || creatureFaction == 35 || creatureFaction == 7 || creatureFaction == 8 || creatureFaction == 114 || creatureFaction == 120 || creatureFaction == 534 {
		return false
	}
	switch creatureFaction {
	case 14, 16, 17, 38, 48, 91, 100, 101, 102, 103, 104, 105, 106, 117, 168, 188, 189, 214, 254:
		return true
	}
	isAlliance := isAllianceRace(playerRace)
	switch creatureFaction {
	case 1, 3, 4, 11, 12, 55, 57, 72, 115:
		return !isAlliance
	case 2, 5, 6, 8, 10, 29, 67, 68, 76, 116:
		return isAlliance
	}
	return false
}

func isAllianceRace(race uint8) bool {
	switch race {
	case 1, 3, 4, 7, 11:
		return true
	default:
		return false
	}
}

func (s *Server) pruneCreatureMotion(now time.Time) {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for scope, motions := range s.instanceCreatureMotion {
		for guid, motion := range motions {
			if now.Sub(motion.Refreshed) > motionTTL {
				delete(motions, guid)
			}
		}
		if len(motions) == 0 {
			delete(s.instanceCreatureMotion, scope)
		}
	}
}

// stopCreatureMotion immediately halts creature movement and broadcasts MonsterMoveStop.
func (s *Server) stopCreatureMotion(mapID uint32, guid uint64, x, y, z float32) {
	s.motionMu.Lock()
	if motion := s.findCreatureMotionLocked(mapID, 0, guid); motion != nil {
		motion.Moving = false
		motion.Evading = false
		motion.InCombat = false
		motion.TargetGUID = 0
		if motion.X != 0 || motion.Y != 0 {
			x = motion.X
			y = motion.Y
			z = motion.Z
		}
	}
	s.motionMu.Unlock()
	s.broadcastMonsterMoveStop(mapID, guid, x, y, z)
}

func (s *Server) stopCreatureMotionInInstance(mapID, instanceID uint32, guid uint64, x, y, z float32) {
	s.motionMu.Lock()
	if motion := s.findCreatureMotionLocked(mapID, instanceID, guid); motion != nil {
		motion.Moving = false
		motion.Evading = false
		motion.InCombat = false
		motion.TargetGUID = 0
		if motion.X != 0 || motion.Y != 0 {
			x, y, z = motion.X, motion.Y, motion.Z
		}
	}
	s.motionMu.Unlock()
	s.broadcastMonsterMoveStopInInstance(mapID, instanceID, guid, x, y, z)
}

func (s *Server) broadcastMonsterMoveStop(mapID uint32, guid uint64, x, y, z float32) {
	packet := protocol.NewBuffer(32)
	packet.WritePackedGUID(guid)
	packet.WriteU8(0) // Transport flag
	packet.WriteF32(x)
	packet.WriteF32(y)
	packet.WriteF32(z)
	packet.WriteU32(0) // Spline ID
	packet.WriteU8(1)  // MonsterMoveStop = 1
	distance := s.Config.VisibilityDistanceContinents
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		if math.Hypot(float64(x-sess.player.X), float64(y-sess.player.Y)) <= distance {
			_ = sess.write(uint16(protocol.OpcodeSMSG_MONSTER_MOVE), packet.Bytes(), true)
		}
	}
}

func (s *Server) broadcastMonsterMoveStopInInstance(mapID, instanceID uint32, guid uint64, x, y, z float32) {
	packet := protocol.NewBuffer(32)
	packet.WritePackedGUID(guid)
	packet.WriteU8(0)
	packet.WriteF32(x)
	packet.WriteF32(y)
	packet.WriteF32(z)
	packet.WriteU32(0)
	packet.WriteU8(1)
	distance := s.Config.VisibilityDistanceContinents
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		if math.Hypot(float64(x-sess.player.X), float64(y-sess.player.Y)) <= distance {
			_ = sess.write(uint16(protocol.OpcodeSMSG_MONSTER_MOVE), packet.Bytes(), true)
		}
	}
}

func (s *Server) broadcastAIReaction(mapID uint32, guid uint64, reactionType uint32) {
	buf := protocol.NewBuffer(12)
	buf.WriteU64(guid)
	buf.WriteU32(reactionType)
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		_ = sess.write(uint16(protocol.OpcodeSMSG_AI_REACTION), buf.Bytes(), true)
	}
}

func (s *Server) broadcastAIReactionInInstance(mapID, instanceID uint32, guid uint64, reactionType uint32) {
	buf := protocol.NewBuffer(12)
	buf.WriteU64(guid)
	buf.WriteU32(reactionType)
	s.broadcastToInstance(mapID, instanceID, uint16(protocol.OpcodeSMSG_AI_REACTION), buf.Bytes(), nil)
}

// loadCreatureSpells queries spells configured for this creature entry from creature_template_spell.
// Reference: ObjectMgr::LoadCreatureTemplateSpells (ObjectMgr.cpp:660).
// It bridges the CombatAI::InitializeAI _spells fill (CombatAI.cpp:55-62):
// index order from the ORDER BY `Index` (the MAX_CREATURE_SPELLS m_spells
// array order, Creature.cpp:546-547), plus the sSpellMgr->GetSpellInfo gate —
// only spells with DBC data join the list.
func (s *Server) loadCreatureSpells(ctx context.Context, entry uint32) []uint32 {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || entry == 0 {
		return nil
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT Spell FROM creature_template_spell WHERE CreatureID = ? ORDER BY `Index`", entry)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var spells []uint32
	for rows.Next() {
		var sp int64
		if rows.Scan(&sp) == nil && sp > 0 && s.creatureSpellDBCValid(uint32(sp)) {
			spells = append(spells, uint32(sp))
		}
	}
	return spells
}

// creatureSpellDBCValid bridges the sSpellMgr->GetSpellInfo(spell) gate in
// CombatAI::InitializeAI (CombatAI.cpp:58): spells without DBC data never
// enter _spells, so they never consume rotation slots or engage/death casts.
func (s *Server) creatureSpellDBCValid(spell uint32) bool {
	if s == nil || s.Data == nil {
		return true
	}
	_, found, err := s.Data.Spell(spell)
	return err == nil && found
}
