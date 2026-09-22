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

// creatureMotion tracks live server-side creature movement state, the role
// TrinityCore's MotionMaster fills: home position (for random wander around
// spawn), current position, waypoint path/point and the next move deadline.
type creatureMotion struct {
	GUID        uint64
	Entry       uint32
	Map         uint32
	HomeX       float32
	HomeY       float32
	HomeZ       float32
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	Speed       float32 // yd/s walk speed used for wander
	RunSpeed    float32 // yd/s run speed used for pursuit
	MoveType    uint32  // 1 random, 2 waypoint
	Wander      float64

	Faction         uint32
	Level           uint32
	UnitFlags       uint32
	FlagsExtra      uint32
	ReactState      uint8
	ReactStateKnown bool
	AttackTime      uint32
	CombatReach     float32

	Armor       uint32
	Resistances [7]uint32
	MinDamage   float32
	MaxDamage   float32

	Health    uint32
	MaxHealth uint32

	TargetGUID     uint64
	InCombat       bool
	LastAttack     time.Time
	LastSpell      time.Time
	Spells         []uint32
	NextSpellIdx   int
	SpellCooldowns map[uint32]time.Time

	ThreatMgr  *ThreatManager
	BossAI     BossAI
	ScriptName string
	Name       string

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

func (s *Server) motionFor(ctx context.Context, guid, entry, mapID uint32, x, y, z float32, moveType uint32, wander float64, walkSpeed float32, currentHealth uint32) *creatureMotion {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		s.creatureMotion = make(map[uint64]*creatureMotion)
	}
	key := creatureWorldGUID(guid, entry)
	motion := s.creatureMotion[key]
	if motion == nil || motion.Entry != entry {
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
			HomeX:           x,
			HomeY:           y,
			HomeZ:           z,
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
			ReactState:      st.ReactState,
			ReactStateKnown: st.ReactStateKnown,
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
		s.creatureMotion[key] = motion
	}
	motion.Refreshed = time.Now()
	return motion
}

func (s *Server) triggerCreatureAggro(ctx context.Context, creatureGUID, playerGUID uint64) {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		s.creatureMotion = make(map[uint64]*creatureMotion)
	}
	guid := uint32(creatureGUID & 0x00FFFFFF)
	entry := uint32((creatureGUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(guid, entry)
	motion := s.creatureMotion[creatureGUID]
	if motion == nil {
		motion = s.creatureMotion[stdKey]
	}
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
				ReactState:      st.ReactState,
				ReactStateKnown: st.ReactStateKnown,
				Name:            name,
				ScriptName:      scriptName,
			}
			s.creatureMotion[creatureGUID] = motion
			s.creatureMotion[stdKey] = motion
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
			s.broadcastAIReaction(motion.Map, creatureGUID, 2) // AI_REACTION_HOSTILE
			startPkt := buildAttackStart(creatureGUID, playerGUID)
			if playerSess := s.findSessionByGUID(playerGUID); playerSess != nil {
				_ = playerSess.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
				s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, playerSess)
			} else {
				s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, nil)
			}
			if motion.BossAI != nil {
				motion.BossAI.OnAggro(ctx, s, motion, playerGUID)
			}
		}
		motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
		motion.InCombat = true
		motion.Moving = false
	}
}

func (s *Server) charmCreature(ctx context.Context, creatureGUID, charmerGUID uint64, charmerRace uint8) ([]uint32, uint8, uint8, bool) {
	if s == nil || creatureGUID == 0 || charmerGUID == 0 {
		return nil, 0, 0, false
	}
	s.motionMu.Lock()
	motion := s.creatureMotion[creatureGUID]
	if motion == nil {
		low := uint32(creatureGUID & 0x00FFFFFF)
		entry := uint32((creatureGUID >> 24) & 0x00FFFFFF)
		motion = s.creatureMotion[creatureWorldGUID(low, entry)]
	}
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
	s.broadcastCreatureValuesUpdate(mapID, rawGUID, map[int]uint32{unitFieldFlags: flags, unitFieldFaction: faction})
	return spells, reactState, commandState, true
}

func (s *Server) uncharmCreature(creatureGUID, charmerGUID uint64) {
	if s == nil || creatureGUID == 0 {
		return
	}
	s.motionMu.Lock()
	motion := s.creatureMotion[creatureGUID]
	if motion == nil {
		low := uint32(creatureGUID & 0x00FFFFFF)
		entry := uint32((creatureGUID >> 24) & 0x00FFFFFF)
		motion = s.creatureMotion[creatureWorldGUID(low, entry)]
	}
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
	s.broadcastCreatureValuesUpdate(mapID, rawGUID, map[int]uint32{unitFieldFlags: flags, unitFieldFaction: faction})
}

// isCreatureEvading returns true if the creature is currently evading back to spawn.
// During evade mode, the creature is immune to all attacks, spells, and threat.
func (s *Server) isCreatureEvading(guid uint64) bool {
	if s == nil {
		return false
	}
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	if s.creatureMotion == nil {
		return false
	}
	if motion, ok := s.creatureMotion[guid]; ok && motion != nil {
		return motion.Evading
	}
	low := uint32(guid & 0x00FFFFFF)
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(low, entry)
	if motion, ok := s.creatureMotion[stdKey]; ok && motion != nil {
		return motion.Evading
	}
	return false
}

// triggerCreatureEvade resets a creature's combat state, clears threat & auras,
// restores health to max, and routes it back to its spawn position with Evading = true.
// Reference: TrinityCore Creature::EnterEvadeMode (Creature.cpp).
func (s *Server) triggerCreatureEvade(ctx context.Context, motion *creatureMotion, now time.Time) {
	if motion == nil {
		return
	}
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	stopPkt := buildAttackStop(motion.GUID, motion.TargetGUID, false)
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, nil)
	s.broadcastThreatClear(motion.Map, motion.GUID)
	if motion.BossAI != nil {
		motion.BossAI.OnEvade(ctx, s, motion)
	}
	motion.InCombat = false
	motion.TargetGUID = 0
	motion.Health = motion.MaxHealth
	if s != nil {
		s.clearCreatureAuras(motion.GUID)
		s.broadcastCreatureValuesUpdate(motion.Map, motion.GUID, map[int]uint32{
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
		s.broadcastMonsterMove(motion.Map, motion.GUID, motion.X, motion.Y, motion.Z, motion.HomeX, motion.HomeY, motion.HomeZ, duration)
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
		if sess.playerLoaded && sess.player != nil {
			isGM := (sess.player.ExtraFlags&playerExtraGMOn != 0) || (sess.player.PlayerFlags&playerFlagGM != 0)
			isDead := (sess.player.Health == 0 && sess.player.MaxHealth > 0) || sess.player.PlayerFlags&playerFlagGhost != 0
			players = append(players, playerPos{
				Map:             sess.player.Map,
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
	query := `SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, c.MovementType, c.wander_distance,
		COALESCE(NULLIF(t.speed_walk, 0), 1.0), COALESCE(NULLIF(t.speed_run, 0), 1.14286),
		COALESCE(t.faction, 0), COALESCE(t.maxlevel, 1), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), COALESCE(NULLIF(t.BaseAttackTime, 0), 2000),
		c.curhealth
		FROM creature AS c
		JOIN creature_template AS t ON t.entry = c.id
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (? OR c.phaseMask = 0 OR (c.phaseMask & 1) <> 0)
		AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(t.npcflag, 0) & 0xC000) = 0))`
	seenCreatures := make(map[uint32]struct{})
	for _, p := range players {
		rows, err := s.WorldStore.DB.QueryContext(ctx, query, p.Map, float64(p.X)-distance, float64(p.X)+distance, float64(p.Y)-distance, float64(p.Y)+distance, p.IsGM, p.IsGM, p.IsDead)
		if err != nil {
			continue
		}
		for rows.Next() {
			var guid, entry, moveType, faction, level, unitFlags, flagsExtra, attackTime, curHealth int64
			var x, y, z, wander, walkSpeed, runSpeed float64
			if err := rows.Scan(&guid, &entry, &x, &y, &z, &moveType, &wander, &walkSpeed, &runSpeed, &faction, &level, &unitFlags, &flagsExtra, &attackTime, &curHealth); err != nil {
				continue
			}
			if _, dup := seenCreatures[uint32(guid)]; dup {
				continue
			}
			seenCreatures[uint32(guid)] = struct{}{}
			if curHealth <= 0 {
				continue
			}
			walkVelocity := creatureWalkVelocity(walkSpeed)
			motion := s.motionFor(ctx, uint32(guid), uint32(entry), p.Map, float32(x), float32(y), float32(z), uint32(moveType), wander, walkVelocity, uint32(curHealth))
			motion.Faction = uint32(faction)
			motion.Level = uint32(level)
			motion.UnitFlags = uint32(unitFlags)
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
				s.broadcastHighestThreatUpdate(motion.Map, motion.GUID, nextVictim, entries)
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
				s.broadcastHighestThreatUpdate(motion.Map, motion.GUID, nextVictim, entries)
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
				s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, target.Sess)
			}

			schoolMask := uint8(1)
			damage, hasDBCSpell := uint32(0), false
			if s != nil && s.Data != nil {
				if spellInfo, found, err := s.Data.Spell(spellID); err == nil && found {
					if spellInfo.SchoolMask != 0 {
						schoolMask = uint8(spellInfo.SchoolMask)
					}
					damage, hasDBCSpell = creatureSpellDamage(spellInfo)
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
					s.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), logPkt, target.Sess)
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
				s.broadcastMonsterMoveMode(motion.Map, motion.GUID, motion.X, motion.Y, motion.Z, target.X, target.Y, target.Z, duration, false)
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
			outcome, hitInfo, targetState := rollMeleeOutcome(uint8(motion.Level), targetLevel, false, isPlayerVictim, false, canBlock, canParry, canDodge)
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
				s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACKERSTATEUPDATE), asuPkt, target.Sess)
			}
			target.Sess.sendPlayerUpdate()
			if s != nil {
				s.triggerPetDefensive(target.GUID, motion.GUID)
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
		if p.Map != motion.Map || p.IsGM || p.IsDead || (motion.FlagsExtra&0x00000400 != 0) || isCreaturePassive(motion) || creatureCombatDisabled(motion.UnitFlags, motion.FlagsExtra) {
			continue
		}
		dist := float32(math.Hypot(float64(p.X-motion.X), float64(p.Y-motion.Y)))
		if !canCreatureDetectStealthOfPlayer(motion, p.Sess, dist) {
			continue
		}
		aggroDist := float32(15.0)
		if s.isHostileFaction(motion.Faction, p) && dist <= aggroDist {
			s.debug("creature aggro", "creature_guid", motion.GUID, "creature_entry", motion.Entry, "faction", motion.Faction, "unit_flags", motion.UnitFlags, "flags_extra", motion.FlagsExtra, "player_guid", p.GUID, "player_zone", p.Sess.player.Zone)
			motion.InCombat = true
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(motion.GUID)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			motion.ThreatMgr.AddThreat(p.GUID, 100.0, true)
			motion.TargetGUID = p.GUID
			motion.Moving = false
			s.broadcastAIReaction(motion.Map, motion.GUID, 2) // AI_REACTION_HOSTILE
			p.Sess.lastCombatTime = now
			if p.Sess.player != nil && p.Sess.player.UnitFlags&unitFlagInCombat == 0 {
				p.Sess.player.UnitFlags |= unitFlagInCombat
				p.Sess.sendPlayerUpdate()
			}
			startPkt := buildAttackStart(motion.GUID, p.GUID)
			_ = p.Sess.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
			s.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, p.Sess)
			if motion.BossAI != nil {
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
	s.broadcastMonsterMoveMode(motion.Map, motion.GUID, motion.X, motion.Y, motion.Z, destX, destY, destZ, duration, walk)
	motion.X, motion.Y, motion.Z = destX, destY, destZ
	motion.Moving = true
	motion.MoveEnds = now.Add(time.Duration(duration) * time.Millisecond)
	motion.WaitUntil = motion.MoveEnds.Add(wait)
}

func creatureSpellDamage(spell wotlk.Spell) (uint32, bool) {
	var damage uint32
	found := false
	for _, effect := range spell.Effects {
		switch effect.Effect {
		case 2, 17, 31, 58, 87:
			found = true
			value := effect.BasePoints + 1
			if value > 0 {
				damage += uint32(value)
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
		if sess.playerLoaded && sess.player != nil {
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
		if sess.playerLoaded && sess.player != nil && !sess.pvpEnd.IsZero() && !now.Before(sess.pvpEnd) {
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
	for key, motion := range s.creatureMotion {
		if now.Sub(motion.Refreshed) > motionTTL {
			delete(s.creatureMotion, key)
		}
	}
}

// stopCreatureMotion immediately halts creature movement and broadcasts MonsterMoveStop.
func (s *Server) stopCreatureMotion(mapID uint32, guid uint64, x, y, z float32) {
	s.motionMu.Lock()
	if s.creatureMotion != nil {
		if motion, ok := s.creatureMotion[guid]; ok && motion != nil {
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
	}
	s.motionMu.Unlock()
	s.broadcastMonsterMoveStop(mapID, guid, x, y, z)
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
		if !sess.playerLoaded || sess.player == nil || sess.player.Map != mapID {
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
		if !sess.playerLoaded || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		_ = sess.write(uint16(protocol.OpcodeSMSG_AI_REACTION), buf.Bytes(), true)
	}
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
