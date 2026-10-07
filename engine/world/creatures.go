package world

import (
	"context"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	creatureTypeMask        uint32 = 0x00000009
	creatureUpdateFlags     uint16 = 0x0060
	creatureValuesCount            = 148
	unitVirtualItemSlotID          = 56
	unitFieldMountDisplayID        = 69
	unitFieldBytes1                = 74
	unitFieldDynamicFlags          = 79
	unitFieldNPCFlags              = 82
	unitNPCEmoteState              = 83
	unitFieldBytes2                = 122
)

type creatureSpawn struct {
	GUID             uint32
	RawGUID          uint64
	Entry            uint32
	Map              uint32
	X                float32
	Y                float32
	Z                float32
	Orientation      float32
	Model            uint32
	Faction          uint32
	NPCFlags         uint32
	UnitFlags        uint32
	UnitFlags2       uint32
	DynamicFlags     uint32
	CreatedBySpell   uint32
	Level            uint32
	Health           uint32
	MaxHealth        uint32
	RegenerateHealth bool
	Mana             uint32
	Scale            float32
	HoverHeight      float32
	BoundingRadius   float32
	CombatReach      float32
	WalkSpeed        float32
	RunSpeed         float32
	AttackTime       uint32
	RangedAttack     uint32
	Mount            uint32
	Bytes1           uint32
	Bytes2           uint32
	Emote            uint32
	Item1            uint32
	Item2            uint32
	Item3            uint32
	TransportGUID    uint64
	TransportX       float32
	TransportY       float32
	TransportZ       float32
	TransportO       float32
}

func creatureSpawnHealth(regenerate bool, current, maximum uint32) uint32 {
	if regenerate {
		return maximum
	}
	return current
}

func (s *Server) buildNearbyCreatureUpdates(ctx context.Context, state playerState, phaseMask uint32, observer *session) (*protocol.Packet, int, error) {
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		return nil, 0, nil
	}
	isGM := state.ExtraFlags&playerExtraGMOn != 0 || state.PlayerFlags&playerFlagGM != 0
	// seesHidden is the SERVERSIDE_VISIBILITY_GM detect arm of
	// WorldObject::CanSeeOrDetect (Object.cpp:1622-1630), distinct from the
	// GM_ON phase-anywhere arm above: Player::SetGMVisible(true) resets the
	// detect to SEC_PLAYER while GM_ON stays set, so a GM who just ran
	// `.gm visible on` stops seeing serverside-hidden creatures (spirit
	// healers/guides, GHOST_VISIBILITY creatures) even though the phase
	// bypass still applies (Player.cpp:2504-2523).
	seesHidden := observer != nil && observer.gmVisibilityDetect != 0
	isGhost := state.Health > 0 && state.PlayerFlags&playerFlagGhost != 0
	// Event creatures spawn only while their event runs; game_event_npcflag
	// flags OR into the template npcflag during events (guards gaining
	// seasonal gossip/questgiver flags).
	var selectArgs []any
	npcFlagExpr := "0"
	if flagClause, ok := s.gameEventNPCFlagClause(ctx, &selectArgs); ok {
		npcFlagExpr = flagClause
	}
	var eventArgs []any
	eventClause := gameEventSpawnClause("gec.eventEntry", s.activeEventList(ctx), &eventArgs)
	fullQuery := `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, c.orientation,
		COALESCE(NULLIF(c.modelid, 0), NULLIF(t.modelid1, 0), NULLIF(t.modelid2, 0), NULLIF(t.modelid3, 0), NULLIF(t.modelid4, 0), 1),
		t.faction, (t.npcflag | ` + npcFlagExpr + `), t.unit_flags, t.dynamicflags,
		t.maxlevel, c.curhealth, t.RegenHealth, c.curmana, t.scale, t.HoverHeight, t.speed_walk, t.speed_run, t.BaseAttackTime, t.RangeAttackTime,
		COALESCE(ca.mount, cta.mount, 0),
		COALESCE(ca.bytes1, cta.bytes1, 0),
		COALESCE(ca.bytes2, cta.bytes2, 0),
		COALESCE(ca.emote, cta.emote, 0),
		COALESCE(eq.ItemID1, 0),
		COALESCE(eq.ItemID2, 0),
		COALESCE(eq.ItemID3, 0)
		FROM creature AS c
		JOIN creature_template AS t ON t.entry = c.id
		LEFT JOIN creature_addon AS ca ON ca.guid = c.guid
		LEFT JOIN creature_template_addon AS cta ON cta.entry = c.id
		LEFT JOIN creature_equip_template AS eq ON eq.CreatureID = c.id AND eq.ID = COALESCE(NULLIF(c.equipment_id, 0), 1)
		LEFT JOIN game_event_creature AS gec ON gec.guid = c.guid
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (? OR (c.phaseMask & ?) <> 0)
		AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(t.npcflag, 0) & 0xC000) = 0))
		AND ` + eventClause + `
		ORDER BY c.guid`
	queryArgs := make([]any, 0, len(selectArgs)+9+len(eventArgs))
	queryArgs = append(queryArgs, selectArgs...)
	queryArgs = append(queryArgs, state.Map, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance, isGM, phaseMask, seesHidden, isGhost)
	queryArgs = append(queryArgs, eventArgs...)
	rows, err := s.WorldStore.DB.QueryContext(ctx, fullQuery, queryArgs...)
	if err != nil {
		fallbackQuery := `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, c.orientation,
			COALESCE(NULLIF(c.modelid, 0), NULLIF(t.modelid1, 0), 1),
			t.faction, t.npcflag, t.unit_flags, t.dynamicflags,
			t.maxlevel, c.curhealth, t.RegenHealth, c.curmana, t.scale, t.speed_walk, t.speed_run, t.BaseAttackTime, t.RangeAttackTime
			FROM creature AS c
			JOIN creature_template AS t ON t.entry = c.id
			WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
			AND (? OR (c.phaseMask & ?) <> 0)
			AND (? OR ? OR ((COALESCE(t.flags_extra, 0) & 0x400) = 0 AND (COALESCE(t.npcflag, 0) & 0xC000) = 0))
			ORDER BY c.guid`
		rows, err = s.WorldStore.DB.QueryContext(ctx, fallbackQuery, state.Map, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance, isGM, phaseMask, seesHidden, isGhost)
		if err != nil {
			if missingTable(err) {
				return nil, 0, nil
			}
			return nil, 0, err
		}
		spawns := make([]creatureSpawn, 0)
		count := 0
		for rows.Next() {
			var guid, entry, mapID, model, faction, npcFlags, unitFlags, dynamicFlags, level, health, regenerateHealth, mana, attackTime, rangedAttack int64
			var x, y, z, orientation, scale, walkSpeed, runSpeed float64
			if err := rows.Scan(&guid, &entry, &mapID, &x, &y, &z, &orientation, &model, &faction, &npcFlags, &unitFlags, &dynamicFlags, &level, &health, &regenerateHealth, &mana, &scale, &walkSpeed, &runSpeed, &attackTime, &rangedAttack); err != nil {
				return nil, count, err
			}
			if math.Hypot(x-float64(state.X), y-float64(state.Y)) > distance || !validMovementPosition(float32(x), float32(y), float32(z), float32(orientation)) {
				continue
			}
			spawn := creatureSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation), Model: uint32(model), Faction: uint32(faction), NPCFlags: uint32(npcFlags), UnitFlags: uint32(unitFlags), DynamicFlags: uint32(dynamicFlags), Level: uint32(level), Health: uint32(health), RegenerateHealth: regenerateHealth != 0, Mana: uint32(mana), Scale: float32(scale), HoverHeight: 1, WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: uint32(attackTime), RangedAttack: uint32(rangedAttack)}
			spawns = append(spawns, spawn)
			count++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, count, err
		}
		transportSpawns := filterNewTransportCreaturePassengers(observer, s.nearbyTransportCreaturePassengers(state, distance))
		spawns = append(spawns, transportSpawns...)
		count += len(transportSpawns)
		if count == 0 {
			return nil, 0, nil
		}
		updates := protocol.NewUpdateData()
		for index := range spawns {
			stats := s.loadCreatureStats(ctx, spawns[index].Entry)
			spawns[index].BoundingRadius, spawns[index].CombatReach, spawns[index].MaxHealth = stats.BoundingRadius, stats.CombatReach, stats.MaxHealth
			spawns[index].Health = creatureSpawnHealth(spawns[index].RegenerateHealth, spawns[index].Health, stats.MaxHealth)
			s.applyCreatureMotionToSpawn(state, &spawns[index])
			updates.AddUpdateBlock(buildCreatureUpdate(spawns[index]))
		}
		packet, err := updates.BuildPacket(0)
		return packet, count, err
	}
	spawns := make([]creatureSpawn, 0)
	count := 0
	for rows.Next() {
		var guid, entry, mapID, model, faction, npcFlags, unitFlags, dynamicFlags, level, health, regenerateHealth, mana, attackTime, rangedAttack, mount, bytes1, bytes2, emote, item1, item2, item3 int64
		var x, y, z, orientation, scale, hoverHeight, walkSpeed, runSpeed float64
		if err := rows.Scan(&guid, &entry, &mapID, &x, &y, &z, &orientation, &model, &faction, &npcFlags, &unitFlags, &dynamicFlags, &level, &health, &regenerateHealth, &mana, &scale, &hoverHeight, &walkSpeed, &runSpeed, &attackTime, &rangedAttack, &mount, &bytes1, &bytes2, &emote, &item1, &item2, &item3); err != nil {
			return nil, count, err
		}
		if math.Hypot(x-float64(state.X), y-float64(state.Y)) > distance || !validMovementPosition(float32(x), float32(y), float32(z), float32(orientation)) {
			continue
		}
		spawn := creatureSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation), Model: uint32(model), Faction: uint32(faction), NPCFlags: uint32(npcFlags), UnitFlags: uint32(unitFlags), DynamicFlags: uint32(dynamicFlags), Level: uint32(level), Health: uint32(health), RegenerateHealth: regenerateHealth != 0, Mana: uint32(mana), Scale: float32(scale), HoverHeight: float32(hoverHeight), WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: uint32(attackTime), RangedAttack: uint32(rangedAttack), Mount: uint32(mount), Bytes1: uint32(bytes1), Bytes2: uint32(bytes2), Emote: uint32(emote), Item1: uint32(item1), Item2: uint32(item2), Item3: uint32(item3)}
		spawns = append(spawns, spawn)
		count++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, count, err
	}
	transportSpawns := filterNewTransportCreaturePassengers(observer, s.nearbyTransportCreaturePassengers(state, distance))
	spawns = append(spawns, transportSpawns...)
	count += len(transportSpawns)
	if count == 0 {
		return nil, 0, nil
	}
	updates := protocol.NewUpdateData()
	for index := range spawns {
		stats := s.loadCreatureStats(ctx, spawns[index].Entry)
		spawns[index].BoundingRadius, spawns[index].CombatReach, spawns[index].MaxHealth = stats.BoundingRadius, stats.CombatReach, stats.MaxHealth
		spawns[index].Health = creatureSpawnHealth(spawns[index].RegenerateHealth, spawns[index].Health, stats.MaxHealth)
		s.applyCreatureMotionToSpawn(state, &spawns[index])
		updates.AddUpdateBlock(buildCreatureUpdate(spawns[index]))
	}
	packet, err := updates.BuildPacket(0)
	return packet, count, err
}

func (s *Server) applyCreatureMotionToSpawn(state playerState, spawn *creatureSpawn) {
	if s == nil || spawn == nil {
		return
	}
	if spawn.TransportGUID != 0 {
		return
	}
	if motion := s.findCreatureMotion(state.Map, state.InstanceID, creatureWorldGUID(spawn.GUID, spawn.Entry)); motion != nil {
		spawn.X, spawn.Y, spawn.Z, spawn.Orientation = motion.X, motion.Y, motion.Z, motion.Orientation
		spawn.Health, spawn.UnitFlags, spawn.DynamicFlags, spawn.Faction = motion.Health, motion.UnitFlags, motion.DynamicFlags, motion.Faction
	}
}

func filterNewTransportCreaturePassengers(observer *session, spawns []creatureSpawn) []creatureSpawn {
	if observer == nil {
		return spawns
	}
	visible := make([]creatureSpawn, 0, len(spawns))
	for _, spawn := range spawns {
		guid := spawn.RawGUID
		if guid == 0 {
			guid = creatureWorldGUID(spawn.GUID, spawn.Entry)
		}
		if observer.markTransportPassengerVisible(guid, spawn.TransportGUID) {
			visible = append(visible, spawn)
		}
	}
	return visible
}

func buildCreatureUpdate(spawn creatureSpawn) []byte {
	values := make([]uint32, creatureValuesCount)
	rawGUID := spawn.RawGUID
	if rawGUID == 0 {
		rawGUID = creatureWorldGUID(spawn.GUID, spawn.Entry)
	}
	values[0] = uint32(rawGUID)
	values[1] = uint32(rawGUID >> 32)
	values[2] = creatureTypeMask
	values[objectFieldEntry] = spawn.Entry
	values[unitFieldHealth] = spawn.Health
	values[unitFieldLevel] = maxUint32(spawn.Level, 1)
	values[unitFieldFaction] = spawn.Faction
	if spawn.Item1 != 0 {
		values[unitVirtualItemSlotID] = spawn.Item1
	}
	if spawn.Item2 != 0 {
		values[unitVirtualItemSlotID+1] = spawn.Item2
	}
	if spawn.Item3 != 0 {
		values[unitVirtualItemSlotID+2] = spawn.Item3
	}
	values[unitFieldFlags] = spawn.UnitFlags
	values[unitFieldFlags2] = spawn.UnitFlags2
	values[unitFieldDynamicFlags] = spawn.DynamicFlags
	values[unitFieldCreatedBySpell] = spawn.CreatedBySpell
	values[unitModCastSpeed] = math.Float32bits(1)
	if spawn.Health == 0 {
		values[unitFieldDynamicFlags] = 1 // UNIT_DYNFLAG_LOOTABLE
	}
	values[unitFieldNPCFlags] = spawn.NPCFlags
	values[unitFieldAttackTime] = maxUint32(spawn.AttackTime, 2000)
	values[unitFieldAttackTimeOffhand] = maxUint32(spawn.AttackTime, 2000)
	boundingRadius := spawn.BoundingRadius
	if boundingRadius <= 0 {
		boundingRadius = 0.306349
	}
	combatReach := spawn.CombatReach
	if combatReach <= 0 {
		combatReach = 1.5
	}
	values[unitFieldBoundingRadius] = math.Float32bits(boundingRadius)
	values[unitFieldCombatReach] = math.Float32bits(combatReach)
	values[unitFieldDisplayID] = spawn.Model
	values[unitFieldNativeDisplayID] = spawn.Model
	hoverHeight := spawn.HoverHeight
	if hoverHeight <= 0 {
		hoverHeight = 1
	}
	values[unitFieldHoverHeight] = math.Float32bits(hoverHeight)
	if spawn.Mount != 0 {
		values[unitFieldMountDisplayID] = spawn.Mount
	}
	if spawn.Bytes1 != 0 {
		values[unitFieldBytes1] = spawn.Bytes1
	}
	if spawn.Emote != 0 {
		values[unitNPCEmoteState] = spawn.Emote
	}
	if spawn.Bytes2 != 0 {
		values[unitFieldBytes2] = spawn.Bytes2
	}
	if spawn.MaxHealth > 0 {
		values[unitFieldMaxHealth] = spawn.MaxHealth
	} else if spawn.Health > 0 {
		values[unitFieldMaxHealth] = spawn.Health
	} else {
		values[unitFieldMaxHealth] = maxUint32(spawn.Level*30, 100)
	}
	values[unitFieldHealth+1] = spawn.Mana
	values[unitFieldMaxPower1] = spawn.Mana
	if spawn.Scale > 0 {
		values[objectFieldScale] = math.Float32bits(spawn.Scale)
	}
	mask := protocol.NewUpdateMask(len(values))
	for index, value := range values {
		if value != 0 {
			_ = mask.Set(index)
		}
	}
	block := protocol.NewBuffer(256)
	block.WriteU8(protocol.UpdateCreateObject2)
	block.WritePackedGUID(rawGUID)
	block.WriteU8(3)
	block.WriteU16(creatureUpdateFlags)
	if spawn.TransportGUID != 0 {
		block.WriteU32(movementOnTransport)
	} else {
		block.WriteU32(0)
	}
	block.WriteU16(0)
	block.WriteU32(uint32(time.Now().UnixMilli()))
	block.WriteF32(spawn.X)
	block.WriteF32(spawn.Y)
	block.WriteF32(spawn.Z)
	block.WriteF32(spawn.Orientation)
	if spawn.TransportGUID != 0 {
		block.WritePackedGUID(spawn.TransportGUID)
		block.WriteF32(spawn.TransportX)
		block.WriteF32(spawn.TransportY)
		block.WriteF32(spawn.TransportZ)
		block.WriteF32(spawn.TransportO)
		block.WriteU32(0)
		block.WriteI8(-1)
	}
	block.WriteU32(0)
	// Create-block speed order (Object.cpp:337-345): WALK, RUN, RUN_BACK, SWIM,
	// SWIM_BACK, FLIGHT, FLIGHT_BACK, TURN_RATE, PITCH_RATE; bases from
	// baseMoveSpeed (Unit.cpp:88-98). Only WALK and RUN carry the template
	// multipliers: m_speed_rate defaults to 1.0 (Unit.cpp:376) and
	// Creature::Initialize sets only WALK/RUN from the template while SWIM and
	// FLIGHT are set to 1.0 (Creature.cpp:529-532); the back-speed UpdateSpeed
	// arms only apply debuffs (Unit.cpp:8675-8677), so the template multipliers
	// never reach RUN_BACK/SWIM/SWIM_BACK/FLIGHT/FLIGHT_BACK at spawn.
	// Documented delta: a later UpdateSpeed(MOVE_SWIM/MOVE_FLIGHT) would fold
	// the template speed_run into the rate (Unit.cpp:8750); Go never re-sends
	// creature speeds after create, so the init-time rates stand.
	for _, speed := range []float32{2.5 * spawn.WalkSpeed, 7 * spawn.RunSpeed, 4.5, 4.722222, 2.5, 7, 4.5, 3.141594, 3.14} {
		block.WriteF32(speed)
	}
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index, value := range values {
		if mask.Has(index) {
			block.WriteU32(value)
		}
	}
	return block.Bytes()
}

func buildMonsterMove(rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32) []byte {
	packet := protocol.NewBuffer(64)
	packet.WritePackedGUID(rawGUID)
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(startX)
	packet.WriteF32(startY)
	packet.WriteF32(startZ)
	packet.WriteU32(uint32(time.Now().UnixMilli())) // SplineID
	packet.WriteU8(0)                               // MonsterMoveNormal
	packet.WriteU32(0)                              // SplineFlags (Linear)
	packet.WriteU32(duration)                       // Duration in ms
	packet.WriteU32(1)                              // Points count
	packet.WriteF32(destX)
	packet.WriteF32(destY)
	packet.WriteF32(destZ)
	return packet.Bytes()
}

func creatureWorldGUID(guid, entry uint32) uint64 {
	return uint64(guid) | uint64(entry)<<24 | uint64(0xF130)<<48
}

func maxUint32(value, fallback uint32) uint32 {
	if value == 0 {
		return fallback
	}
	return value
}

func (s *Server) broadcastMonsterMove(mapID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32) {
	s.broadcastMonsterMoveMode(mapID, rawGUID, startX, startY, startZ, destX, destY, destZ, duration, false)
}

func (s *Server) broadcastMonsterMoveMode(mapID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, walk bool) {
	// Documented delta vs C++ (audited 2026-10-07): WaypointMovementGenerator
	// selects walk speed via init.SetWalk (velocity only — MoveSplineInitArgs.h:58,
	// MoveSplineInit.cpp:111) and never calls Creature::SetWalk, so C++ sends no
	// SMSG_SPLINE_MOVE_SET_WALK_MODE for walk waypoint nodes (the packet is sent
	// only by Creature::SetWalk, Creature.cpp:3017, i.e. persistent walk state).
	// Go sends the mode packet per walk node so walk-speed splines animate as
	// walk instead of run-in-place; deliberate client-visible improvement.
	packet := buildMonsterMove(rawGUID, startX, startY, startZ, destX, destY, destZ, duration)
	modeOpcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
	if walk {
		modeOpcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
	}
	modePacket := protocol.NewBuffer(16)
	modePacket.WritePackedGUID(rawGUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		if math.Hypot(float64(startX-sess.player.X), float64(startY-sess.player.Y)) <= distance {
			_ = sess.write(uint16(modeOpcode), modePacket.Bytes(), true)
			_ = sess.write(uint16(protocol.OpcodeSMSG_MONSTER_MOVE), packet, true)
		}
	}
}

func (s *Server) broadcastMonsterMoveInInstance(mapID, instanceID uint32, rawGUID uint64, startX, startY, startZ, destX, destY, destZ float32, duration uint32, walk bool) {
	packet := buildMonsterMove(rawGUID, startX, startY, startZ, destX, destY, destZ, duration)
	modeOpcode := protocol.OpcodeSMSG_SPLINE_MOVE_SET_RUN_MODE
	if walk {
		modeOpcode = protocol.OpcodeSMSG_SPLINE_MOVE_SET_WALK_MODE
	}
	modePacket := protocol.NewBuffer(16)
	modePacket.WritePackedGUID(rawGUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150.0
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		if math.Hypot(float64(startX-sess.player.X), float64(startY-sess.player.Y)) <= distance {
			_ = sess.write(uint16(modeOpcode), modePacket.Bytes(), true)
			_ = sess.write(uint16(protocol.OpcodeSMSG_MONSTER_MOVE), packet, true)
		}
	}
}

func (s *Server) buildCreatureValuesUpdate(guid uint64, fields map[int]uint32) (*protocol.Packet, error) {
	values := make([]uint32, creatureValuesCount)
	mask := protocol.NewUpdateMask(creatureValuesCount)
	for index, value := range fields {
		if index < 0 || index >= creatureValuesCount {
			continue
		}
		values[index] = value
		if err := mask.Set(index); err != nil {
			return nil, err
		}
	}
	block := protocol.NewBuffer(64 + len(fields)*4)
	block.WriteU8(protocol.UpdateValues)
	block.WritePackedGUID(guid)
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index := 0; index < creatureValuesCount; index++ {
		if mask.Has(index) {
			block.WriteU32(values[index])
		}
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(block.Bytes())
	return updates.BuildPacket(0)
}

func (s *Server) broadcastCreatureValuesUpdate(mapID uint32, guid uint64, fields map[int]uint32) {
	packet, err := s.buildCreatureValuesUpdate(guid, fields)
	if err != nil || packet == nil {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
}

func (s *Server) broadcastCreatureValuesUpdateInInstance(mapID, instanceID uint32, guid uint64, fields map[int]uint32) {
	packet, err := s.buildCreatureValuesUpdate(guid, fields)
	if err != nil || packet == nil {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID || sess.player.InstanceID != instanceID {
			continue
		}
		_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
	}
}
