package world

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type continentTransport struct {
	Spawn           gameObjectSpawn
	Name            string
	TransportMapID  uint32
	PathID          uint32
	Path            *TransportTrajectory
	PathProgress    uint32
	StaticCreatures []creatureSpawn
	StaticObjects   []gameObjectSpawn
	LastUpdate      time.Time
}

type continentTransportMovement struct {
	OldSpawn   gameObjectSpawn
	Spawn      gameObjectSpawn
	Passengers []creatureSpawn
}

func (s *Server) loadContinentTransports(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || s.Data == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT tr.guid, tr.entry, COALESCE(gt.name, ''), COALESCE(gt.data0, 0), COALESCE(gt.data1, 0), COALESCE(gt.data2, 0), COALESCE(gt.data6, 0), COALESCE(gt.displayId, 0), COALESCE(gt.size, 1), COALESCE(gt.data8, 0), COALESCE(ta.flags, 0), COALESCE(ta.faction, 0) FROM transports AS tr JOIN gameobject_template AS gt ON gt.entry = tr.entry LEFT JOIN gameobject_template_addon AS ta ON ta.entry = gt.entry WHERE gt.type = 15 ORDER BY tr.guid`)
	if err != nil && isMissingColumn(err) {
		rows, err = s.WorldStore.DB.QueryContext(ctx, `SELECT tr.guid, tr.entry, COALESCE(gt.name, ''), COALESCE(gt.data0, 0), COALESCE(gt.data1, 0), COALESCE(gt.data2, 0), 0, COALESCE(gt.displayId, 0), COALESCE(gt.size, 1), COALESCE(gt.data8, 0), COALESCE(ta.flags, 0), COALESCE(ta.faction, 0) FROM transports AS tr JOIN gameobject_template AS gt ON gt.entry = tr.entry LEFT JOIN gameobject_template_addon AS ta ON ta.entry = gt.entry WHERE gt.type = 15 ORDER BY tr.guid`)
	}
	if err != nil {
		if !missingTable(err) && s.Logger != nil {
			s.Logger.Warn("continent transport load failed", "error", err)
		}
		return
	}
	defer rows.Close()
	loaded := 0
	now := time.Now()
	for rows.Next() {
		var guid, entry, pathID, transportMapID, displayID, canBeStopped, flags, faction int64
		var name string
		var speed, acceleration, size float64
		if err := rows.Scan(&guid, &entry, &name, &pathID, &speed, &acceleration, &transportMapID, &displayID, &size, &canBeStopped, &flags, &faction); err != nil || guid <= 0 || entry <= 0 || pathID <= 0 {
			continue
		}
		points, err := s.Data.TaxiPathPoints(uint32(pathID))
		path, pathErr := NewTransportTrajectory(points, float32(speed), float32(acceleration))
		if err != nil || pathErr != nil {
			if s.Logger != nil {
				if err == nil {
					err = pathErr
				}
				s.Logger.Warn("continent transport route unavailable", "entry", entry, "path", pathID, "error", err)
			}
			continue
		}
		if size <= 0 {
			size = 1
		}
		mapID, x, y, z, orientation := path.Position(0)
		state := uint8(1)
		if canBeStopped != 0 {
			state = 0
		}
		transport := &continentTransport{Spawn: gameObjectSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: mapID, X: x, Y: y, Z: z, Orientation: orientation, State: state, AnimProgress: 255, Type: GameObjectTypeMOTransport, DisplayID: uint32(displayID), Size: float32(size), Flags: uint32(flags), Faction: uint32(faction), RotationW: 1, ParentRotation: [4]float32{0, 0, 0, 1}}, Name: name, TransportMapID: uint32(transportMapID), PathID: uint32(pathID), Path: path, LastUpdate: now}
		s.loadTransportPassengers(ctx, transport)
		transport.updatePosition()
		if s.Logger != nil {
			s.Logger.Debug("transport passengers loaded", "transport_guid", transportGUID(transport.Spawn.GUID), "entry", transport.Spawn.Entry, "map", transport.Spawn.Map, "transport_map_id", transport.TransportMapID, "path_id", transport.PathID, "creature_count", len(transport.StaticCreatures), "object_count", len(transport.StaticObjects))
		}
		s.transportMu.Lock()
		if s.transports == nil {
			s.transports = make(map[uint32]*continentTransport)
		}
		s.transports[transport.Spawn.GUID] = transport
		s.transportMu.Unlock()
		s.relocateTransportCreatureMotions(ctx, transport.passengerCreatures())
		loaded++
	}
	if s.Logger != nil {
		s.Logger.Info("continent transports loaded", "count", loaded)
	}
}

func (s *Server) transportSpawnForGUID(guid uint64) (gameObjectSpawn, bool) {
	if s == nil {
		return gameObjectSpawn{}, false
	}
	s.transportMu.Lock()
	defer s.transportMu.Unlock()
	for _, transport := range s.transports {
		if transport == nil {
			continue
		}
		rawGUID := transportGUID(transport.Spawn.GUID)
		if guid == uint64(transport.Spawn.GUID) || guid == rawGUID || guid == gameObjectGUID(transport.Spawn.GUID, transport.Spawn.Entry) {
			return transport.Spawn, true
		}
	}
	return gameObjectSpawn{}, false
}

func (s *session) handleTransportUse(spawn gameObjectSpawn) bool {
	if s == nil || s.player == nil || s.server == nil {
		return true
	}
	rawGUID := transportGUID(spawn.GUID)
	if s.player.TransportGUID == rawGUID {
		s.player.TransportGUID = 0
		s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = 0, 0, 0, 0
		s.player.TransportSeat = 0
		s.sendPlayerUpdate()
		s.sendTransportMovement(false)
		s.debug("transport passenger removed", "guid", rawGUID, "entry", spawn.Entry)
		return true
	}
	if s.player.Map != spawn.Map || distance3D(s.player.X, s.player.Y, s.player.Z, spawn.X, spawn.Y, spawn.Z) > 12 {
		return true
	}
	transportX, transportY, transportZ, transportO := CalculatePassengerOffset(spawn.X, spawn.Y, spawn.Z, spawn.Orientation, s.player.X, s.player.Y, s.player.Z, s.player.Orientation)
	s.player.TransportGUID = rawGUID
	s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = transportX, transportY, transportZ, transportO
	s.player.TransportSeat = -1
	s.sendPlayerUpdate()
	s.sendTransportMovement(true)
	s.debug("transport passenger added", "guid", rawGUID, "entry", spawn.Entry, "x", transportX, "y", transportY, "z", transportZ)
	return true
}

func (s *session) sendTransportMovement(attached bool) {
	if s == nil || s.player == nil || s.server == nil {
		return
	}
	info := movementInfo{GUID: s.playerGUID, Time: uint32(time.Now().UnixMilli()), X: s.player.X, Y: s.player.Y, Z: s.player.Z, Orientation: s.player.Orientation}
	if attached && s.player.TransportGUID != 0 {
		info.Flags = movementOnTransport
		info.Transport = &transportMovement{GUID: s.player.TransportGUID, X: s.player.TransportX, Y: s.player.TransportY, Z: s.player.TransportZ, Orientation: s.player.TransportO, Seat: s.player.TransportSeat}
	}
	packet := protocol.NewBuffer(96)
	writeMovementInfo(packet, info)
	_ = s.write(uint16(protocol.OpcodeMSG_MOVE_HEARTBEAT), packet.Bytes(), true)
	s.server.broadcastMovement(uint16(protocol.OpcodeMSG_MOVE_HEARTBEAT), packet.Bytes(), info, s)
}

func (s *Server) loadTransportPassengers(ctx context.Context, transport *continentTransport) bool {
	if s == nil || transport == nil || transport.TransportMapID == 0 || s.WorldStore == nil || s.WorldStore.DB == nil {
		return false
	}
	respawning := make(map[uint32]struct{})
	now := time.Now()
	s.motionMu.Lock()
	for guid, respawn := range s.creatureRespawns {
		if respawn.Map == transport.Spawn.Map && respawn.InstanceID == 0 && respawn.At.After(now) {
			respawning[guid] = struct{}{}
		}
	}
	s.motionMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		rows, err := s.CharactersStore.DB.QueryContext(ctx, `SELECT spawnId FROM respawn WHERE type = 0 AND mapId = ? AND instanceId = 0 AND respawnTime > ?`, transport.Spawn.Map, now.Unix())
		if err == nil {
			for rows.Next() {
				var guid uint32
				if rows.Scan(&guid) == nil {
					respawning[guid] = struct{}{}
				}
			}
			rows.Close()
		}
	}
	creatures, err := s.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, c.orientation,
		COALESCE(NULLIF(c.modelid, 0), NULLIF(t.modelid1, 0), 1), t.faction, t.npcflag, t.unit_flags, t.dynamicflags,
		t.maxlevel, c.curhealth, t.RegenHealth, c.curmana, t.scale, t.HoverHeight, t.speed_walk, t.speed_run, t.BaseAttackTime, t.RangeAttackTime,
		COALESCE(ca.mount, cta.mount, 0), COALESCE(ca.bytes1, cta.bytes1, 0), COALESCE(ca.bytes2, cta.bytes2, 0), COALESCE(ca.emote, cta.emote, 0),
		COALESCE(eq.ItemID1, 0), COALESCE(eq.ItemID2, 0), COALESCE(eq.ItemID3, 0)
		FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		LEFT JOIN creature_addon AS ca ON ca.guid = c.guid
		LEFT JOIN creature_template_addon AS cta ON cta.entry = c.id
		LEFT JOIN creature_equip_template AS eq ON eq.CreatureID = c.id AND eq.ID = COALESCE(NULLIF(c.equipment_id, 0), 1)
		WHERE c.map = ? ORDER BY c.guid`, transport.TransportMapID)
	creaturesLoaded := err == nil
	if err == nil {
		for creatures.Next() {
			var spawn creatureSpawn
			var guid, entry, model, faction, npcFlags, unitFlags, dynamicFlags, level, health, regenerateHealth, mana, attackTime, rangedAttack, mount, bytes1, bytes2, emote, item1, item2, item3 int64
			var x, y, z, orientation, scale, hoverHeight, walkSpeed, runSpeed float64
			if creatures.Scan(&guid, &entry, &x, &y, &z, &orientation, &model, &faction, &npcFlags, &unitFlags, &dynamicFlags, &level, &health, &regenerateHealth, &mana, &scale, &hoverHeight, &walkSpeed, &runSpeed, &attackTime, &rangedAttack, &mount, &bytes1, &bytes2, &emote, &item1, &item2, &item3) != nil {
				continue
			}
			if _, waitingRespawn := respawning[uint32(guid)]; waitingRespawn {
				continue
			}
			spawn = creatureSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: transport.TransportMapID, X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation), Model: uint32(model), Faction: uint32(faction), NPCFlags: uint32(npcFlags), UnitFlags: uint32(unitFlags), DynamicFlags: uint32(dynamicFlags), Level: uint32(level), Health: uint32(health), RegenerateHealth: regenerateHealth != 0, Mana: uint32(mana), Scale: float32(scale), HoverHeight: float32(hoverHeight), WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: uint32(attackTime), RangedAttack: uint32(rangedAttack), Mount: uint32(mount), Bytes1: uint32(bytes1), Bytes2: uint32(bytes2), Emote: uint32(emote), Item1: uint32(item1), Item2: uint32(item2), Item3: uint32(item3)}
			spawn.TransportGUID = transportGUID(transport.Spawn.GUID)
			spawn.TransportX, spawn.TransportY, spawn.TransportZ, spawn.TransportO = spawn.X, spawn.Y, spawn.Z, spawn.Orientation
			transport.StaticCreatures = append(transport.StaticCreatures, spawn)
		}
		creatures.Close()
	}
	objects, err := s.WorldStore.DB.QueryContext(ctx, `SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z, g.orientation, g.rotation0, g.rotation1, g.rotation2, g.rotation3,
		g.state, g.animprogress, t.type, t.displayId, t.size, COALESCE(ta.flags, 0), COALESCE(ta.faction, 0), COALESCE(ta.artkit0, 0),
		COALESCE(ga.parent_rotation0, 0), COALESCE(ga.parent_rotation1, 0), COALESCE(ga.parent_rotation2, 0), COALESCE(ga.parent_rotation3, 1)
		FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id
		LEFT JOIN gameobject_template_addon AS ta ON ta.entry = g.id
		LEFT JOIN gameobject_addon AS ga ON ga.guid = g.guid
		WHERE g.map = ? ORDER BY g.guid`, transport.TransportMapID)
	objectsLoaded := err == nil
	if err == nil {
		for objects.Next() {
			var spawn gameObjectSpawn
			var guid, entry, state, animProgress, objectType, displayID, flags, faction, artKit int64
			var x, y, z, orientation, rotationX, rotationY, rotationZ, rotationW, size, parentRotation0, parentRotation1, parentRotation2, parentRotation3 float64
			if objects.Scan(&guid, &entry, &x, &y, &z, &orientation, &rotationX, &rotationY, &rotationZ, &rotationW, &state, &animProgress, &objectType, &displayID, &size, &flags, &faction, &artKit, &parentRotation0, &parentRotation1, &parentRotation2, &parentRotation3) != nil {
				continue
			}
			spawn = gameObjectSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: transport.TransportMapID, X: float32(x), Y: float32(y), Z: float32(z), Orientation: float32(orientation), RotationX: float32(rotationX), RotationY: float32(rotationY), RotationZ: float32(rotationZ), RotationW: float32(rotationW), State: uint8(state), AnimProgress: uint8(animProgress), Type: uint8(objectType), DisplayID: uint32(displayID), Size: float32(size), Flags: uint32(flags), Faction: uint32(faction), ArtKit: uint8(artKit), ParentRotation: [4]float32{float32(parentRotation0), float32(parentRotation1), float32(parentRotation2), float32(parentRotation3)}}
			transport.StaticObjects = append(transport.StaticObjects, spawn)
		}
		objects.Close()
	}
	return creaturesLoaded && objectsLoaded
}

func (s *Server) reloadTransportPassengers(ctx context.Context, guid uint32) ([]creatureSpawn, bool) {
	if s == nil {
		return nil, false
	}
	s.transportMu.Lock()
	current := s.transports[guid]
	if current == nil {
		s.transportMu.Unlock()
		return nil, false
	}
	replacement := *current
	replacement.StaticCreatures = nil
	replacement.StaticObjects = nil
	s.transportMu.Unlock()
	if !s.loadTransportPassengers(ctx, &replacement) {
		return nil, false
	}
	replacement.updatePosition()
	if s.Logger != nil {
		s.Logger.Debug("transport passengers reloaded", "transport_guid", transportGUID(replacement.Spawn.GUID), "entry", replacement.Spawn.Entry, "map", replacement.Spawn.Map, "transport_map_id", replacement.TransportMapID, "path_id", replacement.PathID, "path_progress", replacement.PathProgress, "creature_count", len(replacement.StaticCreatures), "object_count", len(replacement.StaticObjects))
	}
	s.transportMu.Lock()
	current = s.transports[guid]
	if current == nil || current.Spawn.Map != replacement.Spawn.Map || current.PathProgress != replacement.PathProgress {
		s.transportMu.Unlock()
		return nil, false
	}
	current.StaticCreatures = replacement.StaticCreatures
	current.StaticObjects = replacement.StaticObjects
	passengers := current.passengerCreatures()
	s.transportMu.Unlock()
	return passengers, true
}

func (t *continentTransport) updatePosition() {
	if t == nil || t.Path == nil {
		return
	}
	t.Spawn.Map, t.Spawn.X, t.Spawn.Y, t.Spawn.Z, t.Spawn.Orientation = t.Path.Position(t.PathProgress)
	t.Spawn.TransportProgress = t.PathProgress
	t.Spawn.TransportPeriod = t.Path.Period()
}

func (t *continentTransport) advance(now time.Time) bool {
	if t == nil || t.Path == nil || t.Path.Period() == 0 {
		return false
	}
	if t.LastUpdate.IsZero() {
		t.LastUpdate = now
		t.updatePosition()
		return false
	}
	delta := now.Sub(t.LastUpdate).Milliseconds()
	t.LastUpdate = now
	if delta <= 0 {
		return false
	}
	t.PathProgress += uint32(delta)
	t.updatePosition()
	return true
}

func (s *Server) updateContinentTransports(now time.Time) {
	if s == nil {
		return
	}
	changes := make([]continentTransportMovement, 0)
	s.transportMu.Lock()
	for _, transport := range s.transports {
		if transport == nil {
			continue
		}
		oldSpawn := transport.Spawn
		if transport.advance(now) {
			changes = append(changes, continentTransportMovement{OldSpawn: oldSpawn, Spawn: transport.Spawn, Passengers: transport.passengerCreatures()})
		}
	}
	s.transportMu.Unlock()
	for _, change := range changes {
		if change.OldSpawn.Map != change.Spawn.Map {
			s.unloadTransportCreatureMotions(change.OldSpawn.Map, transportGUID(change.Spawn.GUID), change.Passengers)
			if passengers, ok := s.reloadTransportPassengers(context.Background(), change.Spawn.GUID); ok {
				change.Passengers = passengers
			}
		}
		s.relocateTransportCreatureMotions(context.Background(), change.Passengers)
		s.broadcastTransportMovement(change)
	}
}

func (s *Server) nearbyTransportSpawns(state playerState, distance float64) []gameObjectSpawn {
	if s == nil {
		return nil
	}
	result := make([]gameObjectSpawn, 0)
	s.transportMu.Lock()
	for _, transport := range s.transports {
		if transport == nil || transport.Spawn.Map != state.Map || math.Hypot(float64(transport.Spawn.X-state.X), float64(transport.Spawn.Y-state.Y)) > distance {
			continue
		}
		result = append(result, transport.Spawn)
	}
	s.transportMu.Unlock()
	return result
}

func (s *Server) attachedTransportSnapshot(state playerState) (continentTransport, bool) {
	if s == nil || state.TransportGUID == 0 {
		return continentTransport{}, false
	}
	var transport continentTransport
	s.transportMu.Lock()
	defer s.transportMu.Unlock()
	for _, candidate := range s.transports {
		if candidate == nil || (state.TransportGUID != uint64(candidate.Spawn.GUID) && state.TransportGUID != transportGUID(candidate.Spawn.GUID) && state.TransportGUID != gameObjectGUID(candidate.Spawn.GUID, candidate.Spawn.Entry)) {
			continue
		}
		transport = *candidate
		transport.StaticCreatures = append([]creatureSpawn(nil), candidate.StaticCreatures...)
		transport.StaticObjects = append([]gameObjectSpawn(nil), candidate.StaticObjects...)
		return transport, true
	}
	return continentTransport{}, false
}

func (s *session) markTransportPassengerVisible(guid, transportGUID uint64) bool {
	if s == nil || guid == 0 || transportGUID == 0 {
		return false
	}
	s.visibleTransportPassengersMu.Lock()
	defer s.visibleTransportPassengersMu.Unlock()
	if s.visibleTransportPassengers == nil {
		s.visibleTransportPassengers = make(map[uint64]uint64)
	}
	if _, found := s.visibleTransportPassengers[guid]; found {
		return false
	}
	s.visibleTransportPassengers[guid] = transportGUID
	return true
}

func (s *session) clearTransportPassengerVisible(guid, transportGUID uint64) bool {
	if s == nil || guid == 0 || transportGUID == 0 {
		return false
	}
	s.visibleTransportPassengersMu.Lock()
	defer s.visibleTransportPassengersMu.Unlock()
	if s.visibleTransportPassengers[guid] != transportGUID {
		return false
	}
	delete(s.visibleTransportPassengers, guid)
	return true
}

func (s *session) clearTransportPassengerVisibility(transportGUID uint64) []uint64 {
	if s == nil || transportGUID == 0 {
		return nil
	}
	s.visibleTransportPassengersMu.Lock()
	defer s.visibleTransportPassengersMu.Unlock()
	guids := make([]uint64, 0)
	for guid, parent := range s.visibleTransportPassengers {
		if parent == transportGUID {
			guids = append(guids, guid)
			delete(s.visibleTransportPassengers, guid)
		}
	}
	sort.Slice(guids, func(i, j int) bool { return guids[i] < guids[j] })
	return guids
}

func (s *session) resetTransportPassengerVisibility() {
	if s == nil {
		return
	}
	s.visibleTransportPassengersMu.Lock()
	s.visibleTransportPassengers = nil
	s.visibleTransportPassengersMu.Unlock()
}

func (s *Server) updateTransportPassengerVisibility(ctx context.Context, observer *session, transport continentTransport) {
	if s == nil || observer == nil || observer.player == nil {
		return
	}
	transportGUID := transportGUID(transport.Spawn.GUID)
	distance := float64(s.Config.VisibilityDistanceContinents)
	if observer.player.Map != transport.Spawn.Map {
		guids := observer.clearTransportPassengerVisibility(transportGUID)
		if len(guids) == 0 {
			return
		}
		updates := protocol.NewUpdateData()
		for _, guid := range guids {
			updates.AddOutOfRangeGUID(guid)
		}
		if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
			_ = observer.write(packet.Opcode, packet.Payload.Bytes(), true)
		}
		return
	}
	updates := protocol.NewUpdateData()
	creatureGUIDs := make([]uint64, 0, len(transport.StaticCreatures))
	creaturePositions := make([]string, 0, len(transport.StaticCreatures))
	objectGUIDs := make([]uint64, 0, len(transport.StaticObjects))
	removedGUIDs := make([]uint64, 0)
	for _, passenger := range transport.passengerCreatures() {
		guid := passenger.RawGUID
		if guid == 0 {
			guid = creatureWorldGUID(passenger.GUID, passenger.Entry)
		}
		if !validMovementPosition(passenger.X, passenger.Y, passenger.Z, passenger.Orientation) || distance <= 0 || passenger.Map != observer.player.Map || math.Hypot(float64(passenger.X-observer.player.X), float64(passenger.Y-observer.player.Y)) > distance {
			if observer.clearTransportPassengerVisible(guid, transportGUID) {
				updates.AddOutOfRangeGUID(guid)
				removedGUIDs = append(removedGUIDs, guid)
			}
			continue
		}
		if !observer.markTransportPassengerVisible(guid, transportGUID) {
			continue
		}
		creatureGUIDs = append(creatureGUIDs, guid)
		creaturePositions = append(creaturePositions, fmt.Sprintf("%d@%.2f,%.2f,%.2f/%.2f,%.2f,%.2f,%.2f", guid, passenger.X, passenger.Y, passenger.Z, passenger.TransportX, passenger.TransportY, passenger.TransportZ, passenger.TransportO))
		stats := s.loadCreatureStats(ctx, passenger.Entry)
		passenger.BoundingRadius, passenger.CombatReach, passenger.MaxHealth = stats.BoundingRadius, stats.CombatReach, stats.MaxHealth
		passenger.Health = creatureSpawnHealth(passenger.RegenerateHealth, passenger.Health, stats.MaxHealth)
		updates.AddUpdateBlock(buildCreatureUpdate(passenger))
	}
	for _, passenger := range transport.passengerObjects() {
		guid := gameObjectGUID(passenger.GUID, passenger.Entry)
		if !validMovementPosition(passenger.X, passenger.Y, passenger.Z, passenger.Orientation) || distance <= 0 || passenger.Map != observer.player.Map || math.Hypot(float64(passenger.X-observer.player.X), float64(passenger.Y-observer.player.Y)) > distance {
			if observer.clearTransportPassengerVisible(guid, transportGUID) {
				updates.AddOutOfRangeGUID(guid)
				removedGUIDs = append(removedGUIDs, guid)
			}
			continue
		}
		if !observer.markTransportPassengerVisible(guid, transportGUID) {
			continue
		}
		objectGUIDs = append(objectGUIDs, guid)
		updates.AddUpdateBlock(buildGameObjectUpdate(passenger))
	}
	if !updates.HasData() {
		return
	}
	rollbackVisibility := func() {
		for _, guid := range creatureGUIDs {
			observer.clearTransportPassengerVisible(guid, transportGUID)
		}
		for _, guid := range objectGUIDs {
			observer.clearTransportPassengerVisible(guid, transportGUID)
		}
		for _, guid := range removedGUIDs {
			observer.markTransportPassengerVisible(guid, transportGUID)
		}
	}
	packet, err := updates.BuildPacket(0)
	if err != nil || packet == nil {
		rollbackVisibility()
		if err != nil {
			observer.debug("transport passenger update build failed", "player_guid", observer.playerGUID, "transport_guid", transportGUID, "error", err)
		}
		return
	}
	sent, err := observer.writeWithStatus(packet.Opcode, packet.Payload.Bytes(), true)
	if err != nil || !sent {
		rollbackVisibility()
		if err != nil {
			observer.debug("transport passenger update write failed", "player_guid", observer.playerGUID, "transport_guid", transportGUID, "creature_guids", creatureGUIDs, "object_guids", objectGUIDs, "removed_guids", removedGUIDs, "error", err)
		}
		return
	}
	observer.debug("transport passengers streamed", "player_guid", observer.playerGUID, "player_map", observer.player.Map, "player_x", observer.player.X, "player_y", observer.player.Y, "player_z", observer.player.Z, "transport_guid", transportGUID, "transport_map", transport.Spawn.Map, "transport_x", transport.Spawn.X, "transport_y", transport.Spawn.Y, "transport_z", transport.Spawn.Z, "transport_orientation", transport.Spawn.Orientation, "path_progress", transport.PathProgress, "creature_guids", creatureGUIDs, "creature_positions", creaturePositions, "object_guids", objectGUIDs, "removed_guids", removedGUIDs, "write_bytes", len(packet.Payload.Bytes()))
}

func (s *Server) buildAttachedTransportUpdate(ctx context.Context, state playerState) (*protocol.Packet, error) {
	transport, found := s.attachedTransportSnapshot(state)
	if !found {
		return nil, nil
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildTransportGameObjectUpdate(transport.Spawn, protocol.UpdateCreateObject))
	return updates.BuildPacket(0)
}

func (s *Server) buildAttachedTransportPassengerUpdates(ctx context.Context, state playerState, observer *session) (*protocol.Packet, error) {
	transport, found := s.attachedTransportSnapshot(state)
	if !found {
		return nil, nil
	}
	updates := protocol.NewUpdateData()
	for _, passenger := range transport.passengerCreatures() {
		guid := passenger.RawGUID
		if guid == 0 {
			guid = creatureWorldGUID(passenger.GUID, passenger.Entry)
		}
		if observer != nil && !observer.markTransportPassengerVisible(guid, transportGUID(transport.Spawn.GUID)) {
			continue
		}
		stats := s.loadCreatureStats(ctx, passenger.Entry)
		passenger.BoundingRadius, passenger.CombatReach, passenger.MaxHealth = stats.BoundingRadius, stats.CombatReach, stats.MaxHealth
		passenger.Health = creatureSpawnHealth(passenger.RegenerateHealth, passenger.Health, stats.MaxHealth)
		updates.AddUpdateBlock(buildCreatureUpdate(passenger))
	}
	for _, passenger := range transport.passengerObjects() {
		if observer != nil && !observer.markTransportPassengerVisible(gameObjectGUID(passenger.GUID, passenger.Entry), transportGUID(transport.Spawn.GUID)) {
			continue
		}
		updates.AddUpdateBlock(buildGameObjectUpdate(passenger))
	}
	if !updates.HasData() {
		return nil, nil
	}
	return updates.BuildPacket(0)
}

func (s *Server) buildAttachedTransportPlayerUpdates(state playerState, exclude uint64, observer *session) (*protocol.Packet, []uint64, error) {
	transport, found := s.attachedTransportSnapshot(state)
	if !found {
		return nil, nil, nil
	}
	rawGUID := transportGUID(transport.Spawn.GUID)
	players := make([]playerState, 0)
	runtimeSessions := make(map[uint64]*session)
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil || sess.player.GUID == exclude || sess.player.Map != state.Map || sess.player.InstanceID != state.InstanceID {
			continue
		}
		if sess.player.TransportGUID != rawGUID && sess.player.TransportGUID != uint64(transport.Spawn.GUID) {
			continue
		}
		players = append(players, *sess.player)
		runtimeSessions[sess.player.GUID] = sess
	}
	s.sessionsMu.RUnlock()
	sort.Slice(players, func(i, j int) bool { return players[i].GUID < players[j].GUID })
	packets := make([]*protocol.Packet, 0, len(players))
	guids := make([]uint64, 0, len(players))
	for _, passenger := range players {
		runtime := runtimeSessions[passenger.GUID]
		partyMember := observer != nil && observer.groupID != 0 && runtime != nil && runtime.groupID == observer.groupID
		recipient := state
		packet, err := s.buildPlayerUpdateForRecipient(passenger, false, partyMember, runtime, &recipient)
		if err != nil {
			return nil, nil, err
		}
		if packet != nil {
			packets = append(packets, packet)
			guids = append(guids, passenger.GUID)
		}
	}
	if len(packets) == 0 {
		return nil, nil, nil
	}
	packet, err := protocol.MergeUpdatePackets(packets...)
	return packet, guids, err
}

func (s *Server) buildMapTransportUpdates(state playerState, exclude uint64) (*protocol.Packet, error) {
	if s == nil {
		return nil, nil
	}
	spawns := make([]gameObjectSpawn, 0)
	s.transportMu.Lock()
	for _, transport := range s.transports {
		if transport == nil || transport.Spawn.Map != state.Map {
			continue
		}
		rawGUID := transportGUID(transport.Spawn.GUID)
		if rawGUID == exclude {
			continue
		}
		spawns = append(spawns, transport.Spawn)
	}
	s.transportMu.Unlock()
	sort.Slice(spawns, func(i, j int) bool {
		return transportGUID(spawns[i].GUID) < transportGUID(spawns[j].GUID)
	})
	if len(spawns) == 0 {
		return nil, nil
	}
	updates := protocol.NewUpdateData()
	for _, spawn := range spawns {
		updates.AddUpdateBlock(buildTransportGameObjectUpdate(spawn, protocol.UpdateCreateObject))
	}
	return updates.BuildPacket(0)
}

func (t *continentTransport) passengerCreatures() []creatureSpawn {
	if t == nil {
		return nil
	}
	result := make([]creatureSpawn, 0, len(t.StaticCreatures))
	for _, local := range t.StaticCreatures {
		spawn := local
		spawn.TransportGUID = transportGUID(t.Spawn.GUID)
		spawn.Map = t.Spawn.Map
		spawn.X, spawn.Y, spawn.Z, spawn.Orientation = CalculatePassengerPosition(t.Spawn.X, t.Spawn.Y, t.Spawn.Z, t.Spawn.Orientation, local.TransportX, local.TransportY, local.TransportZ, local.TransportO)
		result = append(result, spawn)
	}
	return result
}

func (t *continentTransport) passengerObjects() []gameObjectSpawn {
	if t == nil {
		return nil
	}
	result := make([]gameObjectSpawn, 0, len(t.StaticObjects))
	for _, local := range t.StaticObjects {
		spawn := local
		spawn.TransportGUID = transportGUID(t.Spawn.GUID)
		spawn.TransportX, spawn.TransportY, spawn.TransportZ, spawn.TransportO = local.X, local.Y, local.Z, local.Orientation
		spawn.Map = t.Spawn.Map
		spawn.X, spawn.Y, spawn.Z, spawn.Orientation = CalculatePassengerPosition(t.Spawn.X, t.Spawn.Y, t.Spawn.Z, t.Spawn.Orientation, local.X, local.Y, local.Z, local.Orientation)
		result = append(result, spawn)
	}
	return result
}

func (s *Server) nearbyTransportCreaturePassengers(state playerState, distance float64) []creatureSpawn {
	result := make([]creatureSpawn, 0)
	s.transportMu.Lock()
	defer s.transportMu.Unlock()
	for _, transport := range s.transports {
		if transport == nil {
			continue
		}
		for _, passenger := range transport.passengerCreatures() {
			if distance <= 0 || passenger.Map != state.Map || math.Hypot(float64(passenger.X-state.X), float64(passenger.Y-state.Y)) > distance {
				continue
			}
			result = append(result, passenger)
		}
	}
	return result
}

func (s *Server) nearbyTransportObjectPassengers(state playerState, distance float64) []gameObjectSpawn {
	result := make([]gameObjectSpawn, 0)
	s.transportMu.Lock()
	defer s.transportMu.Unlock()
	for _, transport := range s.transports {
		if transport == nil {
			continue
		}
		for _, passenger := range transport.passengerObjects() {
			if distance <= 0 || passenger.Map != state.Map || math.Hypot(float64(passenger.X-state.X), float64(passenger.Y-state.Y)) > distance {
				continue
			}
			result = append(result, passenger)
		}
	}
	return result
}

func (s *Server) broadcastTransportMovement(change continentTransportMovement) {
	if s == nil {
		return
	}
	rawGUID := transportGUID(change.Spawn.GUID)
	transport, hasPassengers := s.attachedTransportSnapshot(playerState{TransportGUID: rawGUID})
	if hasPassengers {
		transport.Spawn = change.Spawn
	}
	type passengerTeleport struct {
		session    *session
		x, y, z, o float32
	}
	teleports := make([]passengerTeleport, 0)
	refresh := make([]*session, 0)
	stream := make([]*session, 0)
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if !sess.authed || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.TransportGUID == rawGUID && change.OldSpawn.Map == change.Spawn.Map {
			x, y, z, o := CalculatePassengerPosition(change.Spawn.X, change.Spawn.Y, change.Spawn.Z, change.Spawn.Orientation, sess.player.TransportX, sess.player.TransportY, sess.player.TransportZ, sess.player.TransportO)
			sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation = x, y, z, o
			dx, dy := float64(x-sess.lastStreamX), float64(y-sess.lastStreamY)
			if dx*dx+dy*dy > 30*30 {
				stream = append(stream, sess)
			}
		} else if sess.player.TransportGUID == rawGUID && change.OldSpawn.Map != change.Spawn.Map {
			x, y, z, o := CalculatePassengerPosition(change.Spawn.X, change.Spawn.Y, change.Spawn.Z, change.Spawn.Orientation, sess.player.TransportX, sess.player.TransportY, sess.player.TransportZ, sess.player.TransportO)
			teleports = append(teleports, passengerTeleport{session: sess, x: x, y: y, z: z, o: o})
			continue
		}
		if change.OldSpawn.Map != change.Spawn.Map && sess.player.Map == change.OldSpawn.Map {
			updates := protocol.NewUpdateData()
			updates.AddOutOfRangeGUID(rawGUID)
			for _, guid := range sess.clearTransportPassengerVisibility(rawGUID) {
				updates.AddOutOfRangeGUID(guid)
			}
			if packet, err := updates.BuildPacket(0); err == nil {
				_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
			}
			continue
		}
		if sess.player.Map != change.Spawn.Map {
			continue
		}
		updates := protocol.NewUpdateData()
		if change.OldSpawn.Map == change.Spawn.Map {
			updates.AddUpdateBlock(buildGameObjectMovementUpdate(change.Spawn))
		} else {
			updates.AddUpdateBlock(buildTransportGameObjectUpdate(change.Spawn, protocol.UpdateCreateObject))
		}
		if packet, err := updates.BuildPacket(0); err == nil {
			_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
		}
		if hasPassengers {
			refresh = append(refresh, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, passenger := range teleports {
		passenger.session.resetTransportPassengerVisibility()
		passenger.session.teleportTo(change.Spawn.Map, passenger.x, passenger.y, passenger.z, passenger.o)
	}
	for _, sess := range refresh {
		s.updateTransportPassengerVisibility(context.Background(), sess, transport)
	}
	for _, sess := range stream {
		sess.streamNearbyObjects(context.Background())
	}
}
