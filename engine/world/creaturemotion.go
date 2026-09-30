package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type playerPos struct {
	Map             uint32
	InstanceID      uint32
	X               float32
	Y               float32
	Z               float32
	GUID            uint64
	Race            uint8
	Class           uint8
	Level           uint8
	IsGM            bool
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
	MoveType      uint32  // 1 random, 2 waypoint
	Wander        float64

	Faction         uint32
	Level           uint32
	UnitFlags       uint32
	DynamicFlags    uint32
	FlagsExtra      uint32
	CanFly          bool
	ReactState      uint8
	ReactStateKnown bool
	AttackTime      uint32
	CombatReach     float32

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

	TargetGUID                uint64
	InCombat                  bool
	LastAttack                time.Time
	LastSpell                 time.Time
	Spells                    []uint32
	NextSpellIdx              int
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
	PetReact       uint8 // 0: passive, 1: defensive, 2: aggressive
	AutocastSpells []uint32

	PathID  uint32
	Points  []waypointPoint
	NextIdx int

	Moving    bool
	Evading   bool
	MoveEnds  time.Time
	WaitUntil time.Time
	Refreshed time.Time
}

type waypointPoint struct {
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	MoveType    uint32
	Delay       uint32
}

// motionTTL bounds how long idle state survives between nearby sweeps so a
// creature nobody observes stops consuming memory.
const motionTTL = 10 * time.Minute

const (
	creatureBaseWalkSpeed = 2.5
	creatureBaseRunSpeed  = 7.0
)

const (
	creatureReactPassive uint8 = iota
	creatureReactDefensive
	creatureReactAggressive
)

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

func (s *Server) motionFor(ctx context.Context, guid, entry, mapID, instanceID uint32, x, y, z, orientation float32, moveType uint32, wander float64, walkSpeed float32, currentHealth uint32) *creatureMotion {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	return s.motionForLocked(ctx, guid, entry, mapID, instanceID, x, y, z, orientation, moveType, wander, walkSpeed, currentHealth)
}

func (s *Server) motionForLocked(ctx context.Context, guid, entry, mapID, instanceID uint32, x, y, z, orientation float32, moveType uint32, wander float64, walkSpeed float32, currentHealth uint32) *creatureMotion {
	motions := s.motionMapLocked(mapID, instanceID)
	key := creatureWorldGUID(guid, entry)
	motion := motions[key]
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
			GUID:            key,
			Entry:           entry,
			Map:             mapID,
			InstanceID:      instanceID,
			HomeX:           x,
			HomeY:           y,
			HomeZ:           z,
			Orientation:     orientation,
			X:               x,
			Y:               y,
			Z:               z,
			Speed:           walkSpeed,
			RunSpeed:        creatureBaseRunSpeed,
			MoveType:        moveType,
			Wander:          wander,
			Health:          health,
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
			CanFly:          st.CanFly,
			ReactState:      st.ReactState,
			ReactStateKnown: st.ReactStateKnown,
		}
		if st.UnitClass == 2 || st.UnitClass == 8 {
			motion.MaxPowers[0] = st.Mana
			motion.Powers[0] = st.Mana
			motion.MaxMana = st.Mana
			motion.Mana = st.Mana
		}
		if walkSpeed <= 0 {
			motion.Speed = creatureBaseWalkSpeed
		}
		if motion.ThreatMgr == nil {
			motion.ThreatMgr = NewThreatManager(key)
		}
		if motion.BossAI == nil {
			motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
		}
		if moveType == 2 {
			motion.PathID = s.loadCreaturePathID(ctx, guid, entry)
			motion.Points = s.loadWaypoints(ctx, motion.PathID)
		}
		motions[key] = motion
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
		motion := s.motionForLocked(ctx, passenger.GUID, passenger.Entry, passenger.Map, 0, passenger.X, passenger.Y, passenger.Z, passenger.Orientation, 0, 0, passenger.WalkSpeed, passenger.Health)
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
	defer s.motionMu.Unlock()
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
				GUID:            creatureGUID,
				Entry:           entry,
				Map:             uint32(mapID),
				InstanceID:      instanceID,
				HomeX:           float32(x),
				HomeY:           float32(y),
				HomeZ:           float32(z),
				X:               float32(x),
				Y:               float32(y),
				Z:               float32(z),
				Speed:           creatureBaseWalkSpeed,
				RunSpeed:        creatureBaseRunSpeed,
				Faction:         uint32(faction),
				Level:           st.Level,
				Health:          health,
				MaxHealth:       st.MaxHealth,
				Armor:           st.Armor,
				MinDamage:       st.MinDamage,
				MaxDamage:       st.MaxDamage,
				AttackTime:      st.AttackTime,
				CombatReach:     st.CombatReach,
				UnitFlags:       st.UnitFlags,
				FlagsExtra:      st.FlagsExtra,
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
			return
		}
		if motion.Evading {
			return
		}
		if motion.ThreatMgr == nil {
			motion.ThreatMgr = NewThreatManager(creatureGUID)
		}
		if motion.BossAI == nil {
			motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
		}
		motion.ThreatMgr.AddThreat(playerGUID, 100.0, true)
		if !motion.InCombat {
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
	}
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
	mapID, rawGUID := motion.Map, motion.GUID
	flags, faction := motion.UnitFlags, motion.Faction
	s.motionMu.Unlock()
	s.broadcastCreatureValuesUpdateInInstance(mapID, motion.InstanceID, rawGUID, map[int]uint32{unitFieldFlags: flags, unitFieldFaction: faction})
}

// triggerCreatureEvade resets a creature's combat state, clears threat & auras,
// restores health to max, and routes it back to its spawn position with Evading = true.
// Reference: TrinityCore Creature::EnterEvadeMode (Creature.cpp).
func (s *Server) triggerCreatureEvade(ctx context.Context, motion *creatureMotion, now time.Time) {
	if motion == nil {
		return
	}
	if motion.TransportGUID != 0 {
		motion.Moving = false
		return
	}
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	stopPkt := buildAttackStop(motion.GUID, motion.TargetGUID, false)
	s.broadcastToInstance(motion.Map, motion.InstanceID, uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
	s.broadcastThreatClearInInstance(motion.Map, motion.InstanceID, motion.GUID)
	if motion.BossAI != nil {
		s.clearInstanceEncounter(motion)
		motion.BossAI.OnEvade(ctx, s, motion)
	}
	motion.InCombat = false
	motion.TargetGUID = 0
	motion.Health = motion.MaxHealth
	if s != nil {
		s.clearCreatureAuras(creatureAuraKeyForMotion(motion))
		s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, map[int]uint32{
			unitFieldHealth: motion.MaxHealth,
		})
	}
	homeDist := float32(math.Hypot(float64(motion.HomeX-motion.X), float64(motion.HomeY-motion.Y)))
	if homeDist > 0.5 {
		speed := motion.RunSpeed
		if speed <= 0 {
			speed = creatureBaseRunSpeed
		}
		duration := uint32((homeDist / speed) * 1000)
		if duration < 500 {
			duration = 500
		}
		s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, motion.HomeX, motion.HomeY, motion.HomeZ, duration, false)
		motion.X, motion.Y, motion.Z = motion.HomeX, motion.HomeY, motion.HomeZ
		motion.Moving = true
		motion.Evading = true
		motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
		motion.WaitUntil = motion.MoveEnds
	} else {
		motion.X, motion.Y, motion.Z = motion.HomeX, motion.HomeY, motion.HomeZ
		motion.Moving = false
		motion.Evading = false
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
		rows, err := s.WorldStore.DB.QueryContext(ctx, query, p.Map, float64(p.X)-distance, float64(p.X)+distance, float64(p.Y)-distance, float64(p.Y)+distance, p.IsGM, p.IsGM, p.IsDead)
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
			walkVelocity := creatureWalkVelocity(walkSpeed)
			motion := s.motionFor(ctx, uint32(guid), uint32(entry), p.Map, p.InstanceID, float32(x), float32(y), float32(z), float32(orientation), uint32(moveType), wander, walkVelocity, uint32(curHealth))
			motion.Faction = uint32(faction)
			motion.Level = uint32(level)
			motion.UnitFlags = uint32(unitFlags)
			motion.DynamicFlags = uint32(dynamicFlags)
			motion.FlagsExtra = uint32(flagsExtra)
			if !motion.ReactStateKnown {
				if reactState, known := s.loadCreatureReaction(ctx, uint32(entry)); known {
					motion.ReactState = reactState
					motion.ReactStateKnown = true
				}
			}
			motion.AttackTime = uint32(attackTime)
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
			s.stepCreatureMotion(ctx, motion, players, now)
		}
		rows.Close()
	}
}

// stepCreatureMotion advances one creature: handles combat pursuit/attacks,
// finishes in-flight moves, honors waypoint delays, or wanders randomly.
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
	if motion.Moving {
		if now.Before(motion.MoveEnds) {
			return
		}
		motion.Moving = false
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
		if dist > 45.0 {
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
		spellMinDist, spellMaxDist := contactDist, contactDist
		if len(motion.Spells) > 0 && s != nil && s.Data != nil {
			if spellInfo, found, err := s.Data.Spell(motion.Spells[motion.NextSpellIdx%len(motion.Spells)]); err == nil && found {
				if spellRange, rangeFound, rangeErr := s.Data.SpellRange(spellInfo.RangeIndex); rangeErr == nil && rangeFound {
					spellMinDist += spellRange.MinHostile
					spellMaxDist += spellRange.MaxHostile
				}
			}
		}
		if len(motion.Spells) > 0 && dist >= spellMinDist && dist <= spellMaxDist && (motion.LastSpell.IsZero() || now.Sub(motion.LastSpell) >= 6*time.Second) {
			spellID := motion.Spells[motion.NextSpellIdx%len(motion.Spells)]
			motion.NextSpellIdx++
			if target.Sess != nil {
				target.Sess.debug("creature spell attack", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "target_guid", target.GUID)
			}
			castID := uint8(1)
			castTimeStamp := uint32(now.UnixMilli())
			hitTargets := []uint64{target.GUID}
			spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: target.GUID}
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
			overkill := uint32(0)
			if target.Sess != nil && target.Sess.player != nil {
				if damage >= target.Sess.player.Health {
					overkill = damage - target.Sess.player.Health
					target.Sess.player.Health = 0
					target.IsDead = true
					target.Sess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
					target.Sess.killPlayer(ctx)
					if motion.BossAI != nil {
						motion.BossAI.OnKillPlayer(ctx, s, motion, target.GUID)
					}
				} else {
					target.Sess.player.Health -= damage
					// Reference Unit::DealDamage -> Spell::Delayed / DelayedChannel
					target.Sess.delayCurrentCast()
					target.Sess.delayCurrentChannel()
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
			}
			motion.LastSpell = now
			if dist > contactDist {
				return
			}
		}

		if dist > contactDist {
			// Pursue player: move towards target at run speed
			if !motion.Moving || now.After(motion.MoveEnds) {
				duration := uint32((dist / motion.RunSpeed) * 1000)
				if duration < 300 {
					duration = 300
				}
				s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, target.X, target.Y, target.Z, duration, false)
				motion.X, motion.Y, motion.Z = target.X, target.Y, target.Z
				motion.Moving = true
				motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
			}
			return
		}
		// In melee range: attack player
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
			canBlock := isPlayerVictim && target.Sess.player.Block > 0
			canParry := isPlayerVictim && (target.Sess.player.Level >= 10 || target.Sess.player.Level == 0)
			canDodge := true
			if isPlayerVictim {
				// Player defender cannot block, parry, or dodge if creature is attacking from behind
				attackerInFront := hasInArc(target.Sess.player.Orientation, target.Sess.player.X, target.Sess.player.Y, motion.X, motion.Y, math.Pi)
				if !attackerInFront {
					canBlock = false
					canParry = false
					canDodge = false
				}
			}
			victimDodgeBP := int32(-1)
			if isPlayerVictim && target.Sess != nil && target.Sess.player != nil {
				victimDodgeBP = int32(math.Round(float64(target.Sess.player.DodgePercentage) * 100))
			}
			outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, false, isPlayerVictim, false, canBlock, canParry, canDodge, 0, 0, 0, 0, victimDodgeBP)
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
			if damage > 0 && isPlayerVictim {
				absorbed, rem := target.Sess.applyAbsorptionShields(damage, 1)
				damage = rem
				if rem == 0 && absorbed > 0 {
					hitInfo |= protocol.HitInfoFullAbsorb
				} else if absorbed > 0 {
					hitInfo |= protocol.HitInfoPartialAbsorb
				}
			}
			if damage > 0 {
				if damage >= target.Sess.player.Health {
					overkill = damage - target.Sess.player.Health
					target.Sess.player.Health = 0
					target.IsDead = true
					target.Sess.updateAchievementCriteria(criteriaTypeKilledByCreature, uint32((motion.GUID>>24)&0xFFFFFF), 1)
					target.Sess.killPlayer(ctx)
					if motion.BossAI != nil {
						motion.BossAI.OnKillPlayer(ctx, s, motion, target.GUID)
					}
				} else {
					target.Sess.player.Health -= damage
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
		return
	}

	// 3. Check for nearby hostile aggro
	for _, p := range players {
		if p.Map != motion.Map || p.InstanceID != motion.InstanceID || p.IsGM || p.IsDead || (motion.FlagsExtra&0x00000400 != 0) || isCreaturePassive(motion) || creatureCombatDisabled(motion.UnitFlags, motion.FlagsExtra) {
			continue
		}
		dist := float32(distance3D(p.X, p.Y, p.Z, motion.X, motion.Y, motion.Z))
		if !canCreatureDetectStealthOfPlayer(motion, p.Sess, dist) {
			continue
		}
		// Creature::GetAggroRange (Creature.cpp:2017-2033): 20 yards at equal
		// level, +/-1 yard per level difference, clamped to [5, 45].
		levelDiff := int32(motion.Level) - int32(p.Level)
		aggroDist := float32(20.0) - motion.CombatReach + float32(levelDiff)
		if aggroDist < 5.0 {
			aggroDist = 5.0
		} else if aggroDist > 45.0 {
			aggroDist = 45.0
		}
		if !isCreaturePassive(motion) && s.isAttackableFaction(motion.Faction, p) && canCreatureStartAttack(motion, p, dist, aggroDist) && s.hasLineOfSight(motion.Map, motion.X, motion.Y, motion.Z, p.X, p.Y, p.Z) {
			s.debug("creature aggro", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "player_guid", p.GUID, "player_zone", p.Sess.player.Zone)
			motion.InCombat = true
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(motion.GUID)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			if isCreaturePassive(motion) {
				continue
			}
			motion.ThreatMgr.AddThreat(p.GUID, 100.0, true)
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
			return
		}
	}

	// 3. Normal wandering or waypoint patrolling
	if motion.Moving {
		if now.Before(motion.MoveEnds) {
			return
		}
		motion.Moving = false
		motion.WaitUntil = motion.MoveEnds
	}
	if now.Before(motion.WaitUntil) {
		return
	}
	if motion.MoveType == 2 && len(motion.Points) == 0 {
		return
	}
	var destX, destY, destZ float32
	var speed float32
	var wait time.Duration
	walk := true
	if motion.MoveType == 2 {
		point := motion.Points[motion.NextIdx]
		destX, destY, destZ = point.X, point.Y, point.Z
		speed = motion.RunSpeed
		walk = point.MoveType == 0
		if walk {
			speed = motion.Speed
		}
		if point.Delay > 0 {
			wait = time.Duration(point.Delay) * time.Second
		}
		motion.NextIdx = (motion.NextIdx + 1) % len(motion.Points)
	} else {
		angle := rand.Float64() * 2 * math.Pi
		dist := rand.Float64() * motion.Wander
		destX = float32(float64(motion.HomeX) + dist*math.Cos(angle))
		destY = float32(float64(motion.HomeY) + dist*math.Sin(angle))
		destZ = motion.HomeZ
		speed = motion.Speed
		wait = time.Duration(1+rand.Intn(9)) * time.Second
	}
	moveDist := math.Hypot(float64(destX-motion.X), float64(destY-motion.Y))
	if moveDist < 0.5 {
		return
	}
	duration := uint32((moveDist / float64(speed)) * 1000)
	if duration < 250 {
		duration = 250
	}
	s.broadcastMonsterMoveInInstance(motion.Map, motion.InstanceID, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, walk)
	motion.X, motion.Y, motion.Z = destX, destY, destZ
	motion.Moving = true
	motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
	motion.WaitUntil = motion.MoveEnds.Add(wait)
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
		if rows.Scan(&sp) == nil && sp > 0 {
			spells = append(spells, uint32(sp))
		}
	}
	return spells
}
