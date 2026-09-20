package world

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	gameObjectTypeMask             uint32 = 0x00000021
	gameObjectUpdateFlags          uint16 = 0x0350
	gameObjectValuesCount                 = 18
	transportGameObjectUpdateFlags uint16 = 0x0252
	gameObjectDisplayID                   = 8
	gameObjectFlags                       = 9
	gameObjectParentRotation              = 10
	gameObjectDynamic                     = 14
	gameObjectFaction                     = 15
	gameObjectLevel                       = 16
	gameObjectBytes1                      = 17
)

// Game object types mirroring TrinityCore GameobjectTypes (GameObject.h:60).
const (
	GameObjectTypeDoor         uint8 = 0
	GameObjectTypeButton       uint8 = 1
	GameObjectTypeQuestGiver   uint8 = 2
	GameObjectTypeChest        uint8 = 3
	GameObjectTypeBinder       uint8 = 4
	GameObjectTypeGeneric      uint8 = 5
	GameObjectTypeTrap         uint8 = 6
	GameObjectTypeChair        uint8 = 7
	GameObjectTypeSpellFocus   uint8 = 8
	GameObjectTypeText         uint8 = 9
	GameObjectTypeGoober       uint8 = 10
	GameObjectTypeTransport    uint8 = 11
	GameObjectTypeAreaDamage   uint8 = 12
	GameObjectTypeCamera       uint8 = 13
	GameObjectTypeMapObject    uint8 = 14
	GameObjectTypeMOTransport  uint8 = 15
	GameObjectTypeDuelArbiter  uint8 = 16
	GameObjectTypeFishingNode  uint8 = 17
	GameObjectTypeRitual       uint8 = 18
	GameObjectTypeMailbox      uint8 = 19
	GameObjectTypeDOONotUse    uint8 = 20
	GameObjectTypeGuardPost    uint8 = 21
	GameObjectTypeSpellCaster  uint8 = 22
	GameObjectTypeMeetingStone uint8 = 23
	GameObjectTypeFlagStand    uint8 = 24
	GameObjectTypeFishingHole  uint8 = 25
	GameObjectTypeFlagDrop     uint8 = 29
)

// Game object states mirroring TrinityCore GOState (GameObject.h:35).
const (
	GameObjectStateActive            uint8 = 0 // open / active / pressed
	GameObjectStateReady             uint8 = 1 // closed / ready
	GameObjectStateActiveAlternative uint8 = 2
)

type dynamicGameObjectState struct {
	GUID            uint64
	LowGUID         uint32
	Entry           uint32
	OwnerGUID       uint64
	FishingHandled  bool
	FishingUses     uint32
	FishingMaxOpens uint32
	Map             uint32
	X               float32
	Y               float32
	Z               float32
	Orientation     float32
	State           uint8
	AnimProgress    uint8
	ArtKit          uint8
	Type            uint8
	DisplayID       uint32
	Size            float32
	Flags           uint32
	Faction         uint32
	Data1           uint32
	ParentRotation  [4]float32
	AutoCloseTimer  *time.Timer
	DespawnTimer    *time.Timer
	Hidden          bool
	IsRuntimeSpawn  bool
}

type gameObjectSpawn struct {
	GUID              uint32
	Entry             uint32
	Map               uint32
	X                 float32
	Y                 float32
	Z                 float32
	Orientation       float32
	RotationX         float32
	RotationY         float32
	RotationZ         float32
	RotationW         float32
	State             uint8
	AnimProgress      uint8
	ArtKit            uint8
	Type              uint8
	DisplayID         uint32
	Size              float32
	Flags             uint32
	Faction           uint32
	ParentRotation    [4]float32
	TransportProgress uint32
	TransportPeriod   uint32
	TransportGUID     uint64
	TransportX        float32
	TransportY        float32
	TransportZ        float32
	TransportO        float32
}

func (s *Server) buildNearbyGameObjectUpdates(ctx context.Context, state playerState, includeTransportRoots bool) (*protocol.Packet, int, error) {
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		return nil, 0, nil
	}
	query := `SELECT g.guid, g.id, g.map, g.position_x, g.position_y, g.position_z, g.orientation, g.rotation0, g.rotation1, g.rotation2, g.rotation3, g.state, g.animprogress, t.type, t.displayId, t.size, COALESCE(ta.flags, 0), COALESCE(ta.faction, 0), COALESCE(ta.artkit0, 0), COALESCE(ga.parent_rotation0, 0), COALESCE(ga.parent_rotation1, 0), COALESCE(ga.parent_rotation2, 0), COALESCE(ga.parent_rotation3, 1)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		LEFT JOIN gameobject_template_addon AS ta ON ta.entry = g.id
		LEFT JOIN gameobject_addon AS ga ON ga.guid = g.guid
		LEFT JOIN game_event_gameobject AS geg ON geg.guid = g.guid
		WHERE g.map = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ?
		AND (g.spawnMask = 0 OR (g.spawnMask & 1) <> 0)
		AND (? OR g.phaseMask = 0 OR (g.phaseMask & 1) <> 0)
		AND (geg.eventEntry IS NULL OR geg.eventEntry = 0)
		ORDER BY g.guid`
	isGM := state.ExtraFlags&playerExtraGMOn != 0 || state.PlayerFlags&playerFlagGM != 0
	// Event gameobjects spawn only while their event runs.
	goArgs := make([]any, 0, 4)
	goEventClause := gameEventSpawnClause("geg.eventEntry", s.activeEventList(ctx), &goArgs)
	query = strings.Replace(query, "AND (geg.eventEntry IS NULL OR geg.eventEntry = 0)", "AND "+goEventClause, 1)
	queryArgs := append([]any{state.Map, float64(state.X) - distance, float64(state.X) + distance, float64(state.Y) - distance, float64(state.Y) + distance, isGM}, goArgs...)
	rows, err := s.WorldStore.DB.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		fallbackQuery := `SELECT g.guid, g.id, g.map, g.position_x, g.position_y, g.position_z, g.orientation, g.rotation0, g.rotation1, g.rotation2, g.rotation3, g.state, g.animprogress, t.type, t.displayId, t.size, COALESCE(ta.flags, 0), COALESCE(ta.faction, 0), COALESCE(ta.artkit0, 0), COALESCE(ga.parent_rotation0, 0), COALESCE(ga.parent_rotation1, 0), COALESCE(ga.parent_rotation2, 0), COALESCE(ga.parent_rotation3, 1)
			FROM gameobject AS g
			JOIN gameobject_template AS t ON t.entry = g.id
			LEFT JOIN gameobject_template_addon AS ta ON ta.entry = g.id
			LEFT JOIN gameobject_addon AS ga ON ga.guid = g.guid
			WHERE g.map = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ?
			AND (g.spawnMask = 0 OR (g.spawnMask & 1) <> 0)
			AND (? OR g.phaseMask = 0 OR (g.phaseMask & 1) <> 0)
			ORDER BY g.guid`
		rows, err = s.WorldStore.DB.QueryContext(ctx, fallbackQuery, state.Map, float64(state.X)-distance, float64(state.X)+distance, float64(state.Y)-distance, float64(state.Y)+distance, isGM)
		if err != nil {
			if missingTable(err) {
				return nil, 0, nil
			}
			return nil, 0, err
		}
	}
	defer rows.Close()
	updates := protocol.NewUpdateData()
	count := 0
	for rows.Next() {
		var spawn gameObjectSpawn
		var guid, entry, mapID, stateValue, animProgress, objectType, displayID, flags, faction, artKit int64
		var x, y, z, orientation, rotationX, rotationY, rotationZ, rotationW, size, parentRotation0, parentRotation1, parentRotation2, parentRotation3 float64
		if err := rows.Scan(&guid, &entry, &mapID, &x, &y, &z, &orientation, &rotationX, &rotationY, &rotationZ, &rotationW, &stateValue, &animProgress, &objectType, &displayID, &size, &flags, &faction, &artKit, &parentRotation0, &parentRotation1, &parentRotation2, &parentRotation3); err != nil {
			return nil, count, err
		}
		if math.Hypot(x-float64(state.X), y-float64(state.Y)) > distance || !validMovementPosition(float32(x), float32(y), float32(z), float32(orientation)) {
			continue
		}
		spawn.GUID = uint32(guid)
		spawn.Entry = uint32(entry)
		spawn.Map = uint32(mapID)
		spawn.X, spawn.Y, spawn.Z, spawn.Orientation = float32(x), float32(y), float32(z), float32(orientation)
		spawn.RotationX, spawn.RotationY, spawn.RotationZ, spawn.RotationW = float32(rotationX), float32(rotationY), float32(rotationZ), float32(rotationW)
		spawn.State, spawn.AnimProgress, spawn.ArtKit, spawn.Type = uint8(stateValue), uint8(animProgress), uint8(artKit), uint8(objectType)
		spawn.DisplayID, spawn.Size, spawn.Flags, spawn.Faction = uint32(displayID), float32(size), uint32(flags), uint32(faction)
		spawn.ParentRotation = [4]float32{float32(parentRotation0), float32(parentRotation1), float32(parentRotation2), float32(parentRotation3)}
		s.objectsMu.RLock()
		if dyn, ok := s.dynamicGameObjects[gameObjectGUID(spawn.GUID, spawn.Entry)]; ok && dyn != nil {
			spawn.State = dyn.State
		}
		s.objectsMu.RUnlock()
		// Server-side visibility: script-hidden objects stay visible to
		// GMs the way SetVisible(false) units remain visible to GM seers.
		if !isGM && s.isGameObjectHidden(gameObjectGUID(spawn.GUID, spawn.Entry)) {
			continue
		}
		updates.AddUpdateBlock(buildGameObjectUpdate(spawn))
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, count, err
	}
	for _, passenger := range s.nearbyTransportObjectPassengers(state, distance) {
		updates.AddUpdateBlock(buildGameObjectUpdate(passenger))
		count++
	}
	s.objectsMu.RLock()
	for _, dyn := range s.dynamicGameObjects {
		if dyn == nil || dyn.Map != state.Map || dyn.Hidden || !dyn.IsRuntimeSpawn {
			continue
		}
		if math.Hypot(float64(dyn.X-state.X), float64(dyn.Y-state.Y)) > distance {
			continue
		}
		spawn := gameObjectSpawn{
			GUID:           dyn.LowGUID,
			Entry:          dyn.Entry,
			Map:            dyn.Map,
			X:              dyn.X,
			Y:              dyn.Y,
			Z:              dyn.Z,
			Orientation:    dyn.Orientation,
			RotationW:      1.0,
			State:          dyn.State,
			Type:           dyn.Type,
			DisplayID:      dyn.DisplayID,
			Size:           dyn.Size,
			ParentRotation: [4]float32{0, 0, 0, 1},
		}
		updates.AddUpdateBlock(buildGameObjectUpdate(spawn))
		count++
	}
	s.objectsMu.RUnlock()
	if includeTransportRoots {
		for _, transport := range s.nearbyTransportSpawns(state, distance) {
			updates.AddUpdateBlock(buildGameObjectUpdate(transport))
			count++
		}
	}
	if count == 0 {
		return nil, 0, nil
	}
	packet, err := updates.BuildPacket(0)
	return packet, count, err
}

func buildGameObjectUpdate(spawn gameObjectSpawn) []byte {
	if spawn.Type == GameObjectTypeMOTransport {
		return buildTransportGameObjectUpdate(spawn, true)
	}
	rawGUID := gameObjectGUID(spawn.GUID, spawn.Entry)
	values := make([]uint32, gameObjectValuesCount)
	values[0] = uint32(rawGUID)
	values[1] = uint32(rawGUID >> 32)
	values[2] = gameObjectTypeMask
	values[objectFieldEntry] = spawn.Entry
	values[4] = math.Float32bits(spawn.Size)
	values[gameObjectDisplayID] = spawn.DisplayID
	values[gameObjectFlags] = spawn.Flags
	values[gameObjectParentRotation] = math.Float32bits(spawn.ParentRotation[0])
	values[gameObjectParentRotation+1] = math.Float32bits(spawn.ParentRotation[1])
	values[gameObjectParentRotation+2] = math.Float32bits(spawn.ParentRotation[2])
	values[gameObjectParentRotation+3] = math.Float32bits(spawn.ParentRotation[3])
	values[gameObjectDynamic] = gameObjectDynamicValue(spawn)
	values[gameObjectFaction] = spawn.Faction
	values[gameObjectBytes1] = uint32(spawn.State) | uint32(spawn.Type)<<8 | uint32(spawn.ArtKit)<<16 | uint32(spawn.AnimProgress)<<24
	mask := protocol.NewUpdateMask(len(values))
	for index, value := range values {
		if value != 0 {
			_ = mask.Set(index)
		}
	}
	block := protocol.NewBuffer(256)
	block.WriteU8(protocol.UpdateCreateObject2)
	block.WritePackedGUID(rawGUID)
	block.WriteU8(5)
	block.WriteU16(gameObjectUpdateFlags)
	if spawn.TransportGUID != 0 {
		block.WritePackedGUID(spawn.TransportGUID)
	} else {
		block.WriteU8(0)
	}
	block.WriteF32(spawn.X)
	block.WriteF32(spawn.Y)
	block.WriteF32(spawn.Z)
	if spawn.TransportGUID != 0 {
		block.WriteF32(spawn.TransportX)
		block.WriteF32(spawn.TransportY)
		block.WriteF32(spawn.TransportZ)
	} else {
		block.WriteF32(spawn.X)
		block.WriteF32(spawn.Y)
		block.WriteF32(spawn.Z)
	}
	block.WriteF32(spawn.Orientation)
	block.WriteF32(0)
	block.WriteU32(spawn.GUID)
	block.WriteU64(packGameObjectRotation(spawn.RotationX, spawn.RotationY, spawn.RotationZ, spawn.RotationW))
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index, value := range values {
		if mask.Has(index) {
			block.WriteU32(value)
		}
	}
	return block.Bytes()
}

func buildGameObjectMovementUpdate(spawn gameObjectSpawn) []byte {
	if spawn.Type != GameObjectTypeMOTransport {
		return nil
	}
	return buildTransportGameObjectUpdate(spawn, false)
}

func buildTransportGameObjectUpdate(spawn gameObjectSpawn, create bool) []byte {
	rawGUID := gameObjectGUID(spawn.GUID, spawn.Entry)
	values := make([]uint32, gameObjectValuesCount)
	values[0] = uint32(rawGUID)
	values[1] = uint32(rawGUID >> 32)
	values[2] = gameObjectTypeMask
	values[objectFieldEntry] = spawn.Entry
	values[4] = math.Float32bits(spawn.Size)
	values[gameObjectDisplayID] = spawn.DisplayID
	values[gameObjectFlags] = spawn.Flags
	values[gameObjectParentRotation] = math.Float32bits(spawn.ParentRotation[0])
	values[gameObjectParentRotation+1] = math.Float32bits(spawn.ParentRotation[1])
	values[gameObjectParentRotation+2] = math.Float32bits(spawn.ParentRotation[2])
	values[gameObjectParentRotation+3] = math.Float32bits(spawn.ParentRotation[3])
	values[gameObjectDynamic] = gameObjectDynamicValue(spawn)
	values[gameObjectFaction] = spawn.Faction
	values[gameObjectBytes1] = uint32(spawn.State) | uint32(spawn.Type)<<8 | uint32(spawn.ArtKit)<<16 | uint32(spawn.AnimProgress)<<24
	block := protocol.NewBuffer(256)
	if create {
		block.WriteU8(protocol.UpdateCreateObject2)
	} else {
		block.WriteU8(protocol.UpdateMovement)
	}
	block.WritePackedGUID(rawGUID)
	if create {
		block.WriteU8(5)
	}
	block.WriteU16(transportGameObjectUpdateFlags)
	block.WriteF32(spawn.X)
	block.WriteF32(spawn.Y)
	block.WriteF32(spawn.Z)
	block.WriteF32(spawn.Orientation)
	block.WriteU32(spawn.GUID)
	block.WriteU32(spawn.TransportProgress)
	block.WriteU64(packGameObjectRotation(spawn.RotationX, spawn.RotationY, spawn.RotationZ, spawn.RotationW))
	if create {
		mask := protocol.NewUpdateMask(len(values))
		for index, value := range values {
			if value != 0 {
				_ = mask.Set(index)
			}
		}
		block.WriteU8(uint8(mask.BlockCount()))
		mask.AppendTo(block)
		for index, value := range values {
			if mask.Has(index) {
				block.WriteU32(value)
			}
		}
	}
	return block.Bytes()
}

func gameObjectGUID(guid, entry uint32) uint64 {
	return uint64(guid) | uint64(entry)<<24 | uint64(0xF110)<<48
}

func gameObjectDynamicValue(spawn gameObjectSpawn) uint32 {
	if (spawn.Type == GameObjectTypeTransport || spawn.Type == GameObjectTypeMOTransport) && spawn.TransportPeriod > 0 {
		progress := uint32(float64(spawn.TransportProgress%spawn.TransportPeriod) / float64(spawn.TransportPeriod) * 65535)
		return uint32(uint16(progress)) << 16
	}
	return 0xFFFF0000
}

func packGameObjectRotation(x, y, z, w float32) uint64 {
	norm := math.Sqrt(float64(x*x + y*y + z*z + w*w))
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		x, y, z, w = 0, 0, 0, 1
	} else {
		x, y, z, w = float32(float64(x)/norm), float32(float64(y)/norm), float32(float64(z)/norm), float32(float64(w)/norm)
	}
	const packYZ int64 = 1 << 20
	const packX int64 = packYZ << 1
	const packYZMask int64 = (packYZ << 1) - 1
	const packXMask int64 = (packX << 1) - 1
	wSign := int64(1)
	if w < 0 {
		wSign = -1
	}
	packedX := (int64(int32(float64(x)*float64(packX))) * wSign) & packXMask
	packedY := (int64(int32(float64(y)*float64(packYZ))) * wSign) & packYZMask
	packedZ := (int64(int32(float64(z)*float64(packYZ))) * wSign) & packYZMask
	return uint64(packedZ | packedY<<21 | packedX<<42)
}

// handleGameObjectUse processes CMSG_GAMEOBJ_USE (0x0B1).
// Reference: WorldSession::HandleGameObjectUseOpcode (SpellHandler.cpp:300) -> GameObject::Use (GameObject.cpp:1290).
func (s *session) handleGameObjectUse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return false
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil || guid == 0 {
		return false
	}

	entry := uint32((guid >> 24) & 0x00FFFFFF)
	s.updateAchievementCriteria(criteriaTypeUseGameObject, entry, 1)
	lowGUID := uint32(guid & 0x00FFFFFF)

	// Delegate Warsong Gulch flags to WSG state machine
	if s.server != nil && isWSGFlag(entry) {
		return s.server.handleWSGFlagUse(ctx, s, guid, entry)
	}

	// Delegate Arathi Basin banners to AB state machine
	if s.server != nil && isABBanner(entry) {
		return s.server.handleABBannerUse(ctx, s, guid, entry)
	}

	// Delegate Eye of the Storm flags and banners to EotS state machine
	if s.server != nil && isEOTSGameObject(entry) {
		return s.server.handleEOTSGameObjectUse(ctx, s, guid, entry)
	}

	// Delegate Alterac Valley banners to AV state machine
	if s.server != nil && isAVGameObject(entry) {
		return s.server.handleAVGameObjectUse(ctx, s, guid, entry)
	}

	// Delegate Strand of the Ancients objects to SA state machine
	if s.server != nil && isSAGameObject(entry) {
		return s.server.handleSAGameObjectUse(ctx, s, guid, entry)
	}

	// Delegate Isle of Conquest objects to IC state machine
	if s.server != nil && isICGameObject(entry) {
		return s.server.handleICGameObjectUse(ctx, s, guid, entry)
	}

	// Delegate Arena objects to Arena state machine
	if s.server != nil && isArenaGameObject(entry) {
		return s.server.handleArenaGameObjectUse(ctx, s, guid, entry)
	}

	// Delegate Wintergrasp objects to WG state machine
	if s.server != nil && isWGGameObject(entry) {
		return s.server.handleWGGameObjectUse(ctx, s, guid, entry)
	}

	if s.server == nil {
		return true
	}

	goState, err := s.server.getOrLoadGameObjectState(ctx, guid, lowGUID, entry)
	if err != nil || goState == nil {
		return false
	}

	// Range check (10.0 yards standard interaction distance)
	if goState.Map != s.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 10.0 {
		return true
	}

	switch goState.Type {
	case GameObjectTypeDoor:
		// Toggle door open/closed
		newState := GameObjectStateActive
		if goState.State == GameObjectStateActive {
			newState = GameObjectStateReady
		}
		s.server.setGameObjectState(guid, newState)
		s.server.broadcastGameObjectCustomAnim(goState.Map, guid, 0)
		if newState == GameObjectStateActive {
			s.server.scheduleGameObjectReset(guid, 10*time.Second)
		}

	case GameObjectTypeButton:
		// Press button
		s.server.setGameObjectState(guid, GameObjectStateActive)
		s.server.broadcastGameObjectCustomAnim(goState.Map, guid, 0)
		s.server.scheduleGameObjectReset(guid, 5*time.Second)

	case GameObjectTypeChest:
		s.handleLoot(ctx, payload)

	case GameObjectTypeFishingNode, GameObjectTypeFishingHole:
		s.updateAchievementCriteria(criteriaTypeFishInGameObject, entry, 1)
		if goState.Type == GameObjectTypeFishingNode {
			return s.handleFishingNodeUse(ctx, payload, goState)
		}
		return s.handleFishingHoleUse(ctx, payload, goState)

	case GameObjectTypeGoober:
		s.server.setGameObjectState(guid, GameObjectStateActive)
		s.server.broadcastGameObjectCustomAnim(goState.Map, guid, 0)
		if goState.Data1 > 0 {
			s.castSpellDirect(ctx, goState.Data1, s.playerGUID)
		}
		s.server.scheduleGameObjectReset(guid, 10*time.Second)
	}

	return true
}

// handleGameObjectReportUse processes CMSG_GAMEOBJ_REPORT_USE (0x481).
// Reference: WorldSession::HandleGameobjectReportUse (SpellHandler.cpp:318).
func (s *session) handleGameObjectReportUse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil {
		return false
	}
	_ = guid
	return true
}

func (s *Server) getOrLoadGameObjectState(ctx context.Context, guid uint64, lowGUID, entry uint32) (*dynamicGameObjectState, error) {
	if s == nil {
		return nil, nil
	}
	s.objectsMu.RLock()
	if dyn, ok := s.dynamicGameObjects[guid]; ok && dyn != nil {
		s.objectsMu.RUnlock()
		return dyn, nil
	}
	s.objectsMu.RUnlock()

	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return nil, nil
	}

	var goMap, goState, goType, displayID, data1 int64
	var goX, goY, goZ, goO, size float64
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, g.orientation, g.state,
		t.type, t.displayId, t.size, COALESCE(t.data1, 0)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.guid = ? AND g.id = ? LIMIT 1`, lowGUID, entry).Scan(&goMap, &goX, &goY, &goZ, &goO, &goState, &goType, &displayID, &size, &data1)
	if err != nil {
		return nil, err
	}

	dyn := &dynamicGameObjectState{
		GUID:        guid,
		LowGUID:     lowGUID,
		Entry:       entry,
		Map:         uint32(goMap),
		X:           float32(goX),
		Y:           float32(goY),
		Z:           float32(goZ),
		Orientation: float32(goO),
		State:       uint8(goState),
		Type:        uint8(goType),
		DisplayID:   uint32(displayID),
		Size:        float32(size),
		Data1:       uint32(data1),
	}
	if dyn.Type == GameObjectTypeFishingHole {
		var minOpens, maxOpens int64
		if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(data2, 0), COALESCE(data3, 0) FROM gameobject_template WHERE entry = ?", entry).Scan(&minOpens, &maxOpens); err == nil && maxOpens > 0 {
			if minOpens < 0 {
				minOpens = 0
			}
			if maxOpens < minOpens {
				maxOpens = minOpens
			}
			dyn.FishingMaxOpens = uint32(minOpens)
			if maxOpens > minOpens {
				dyn.FishingMaxOpens += uint32(rand.Int63n(maxOpens - minOpens + 1))
			}
		}
	}

	s.objectsMu.Lock()
	if s.dynamicGameObjects == nil {
		s.dynamicGameObjects = make(map[uint64]*dynamicGameObjectState)
	}
	s.dynamicGameObjects[guid] = dyn
	s.objectsMu.Unlock()

	return dyn, nil
}

func (s *Server) setGameObjectState(guid uint64, state uint8) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	if s.dynamicGameObjects == nil {
		return
	}
	if dyn, ok := s.dynamicGameObjects[guid]; ok && dyn != nil {
		dyn.State = state
	}
}

func (s *Server) scheduleGameObjectReset(guid uint64, delay time.Duration) {
	if s == nil {
		return
	}
	s.objectsMu.Lock()
	dyn, ok := s.dynamicGameObjects[guid]
	if !ok || dyn == nil {
		s.objectsMu.Unlock()
		return
	}
	if dyn.AutoCloseTimer != nil {
		dyn.AutoCloseTimer.Stop()
	}
	mapID := dyn.Map
	dyn.AutoCloseTimer = time.AfterFunc(delay, func() {
		s.objectsMu.Lock()
		if currentDyn, exists := s.dynamicGameObjects[guid]; exists && currentDyn != nil {
			currentDyn.State = GameObjectStateReady
		}
		s.objectsMu.Unlock()
		s.broadcastGameObjectResetState(mapID, guid)
	})
	s.objectsMu.Unlock()
}

func (s *Server) broadcastToMap(mapID uint32, opcode uint16, payload []byte) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for target := range s.sessions {
		if !target.authed || !target.playerLoaded || target.player == nil || target.player.Map != mapID {
			continue
		}
		_ = target.write(opcode, payload, true)
	}
}

func (s *Server) broadcastGameObjectCustomAnim(mapID uint32, guid uint64, anim uint32) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(12)
	buf.WriteU64(guid)
	buf.WriteU32(anim)
	s.broadcastToMap(mapID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_CUSTOM_ANIM), buf.Bytes())
}

func (s *Server) broadcastGameObjectResetState(mapID uint32, guid uint64) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	s.broadcastToMap(mapID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_RESET_STATE), buf.Bytes())
}

func (s *Server) broadcastGameObjectDespawn(mapID uint32, guid uint64) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(8)
	buf.WriteU64(guid)
	s.broadcastToMap(mapID, uint16(protocol.OpcodeSMSG_GAMEOBJECT_DESPAWN_ANIM), buf.Bytes())
}

func (s *Server) nextDynamicGameObjectLowGUID() uint32 {
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	s.nextDynamicGOGUID++
	if s.nextDynamicGOGUID < 1000000 {
		s.nextDynamicGOGUID = 1000000
	}
	return s.nextDynamicGOGUID
}

func (s *Server) spawnDynamicGameObject(dyn *dynamicGameObjectState) {
	if s == nil || dyn == nil {
		return
	}
	s.objectsMu.Lock()
	if s.dynamicGameObjects == nil {
		s.dynamicGameObjects = make(map[uint64]*dynamicGameObjectState)
	}
	s.dynamicGameObjects[dyn.GUID] = dyn
	s.objectsMu.Unlock()

	spawn := gameObjectSpawn{
		GUID:           dyn.LowGUID,
		Entry:          dyn.Entry,
		Map:            dyn.Map,
		X:              dyn.X,
		Y:              dyn.Y,
		Z:              dyn.Z,
		Orientation:    dyn.Orientation,
		RotationW:      1.0,
		State:          dyn.State,
		Type:           dyn.Type,
		DisplayID:      dyn.DisplayID,
		Size:           dyn.Size,
		ParentRotation: [4]float32{0, 0, 0, 1},
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(buildGameObjectUpdate(spawn))
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		s.broadcastToMap(dyn.Map, packet.Opcode, packet.Payload.Bytes())
	}
}

func isFishingSpell(spellID uint32) bool {
	switch spellID {
	case 7620, 7731, 7732, 18248, 33095, 51294:
		return true
	default:
		return false
	}
}

func (s *session) spawnFishingBobber(ctx context.Context, target protocol.SpellTargetData) {
	if s == nil || s.server == nil || s.player == nil || target.Flags&protocol.SpellTargetFlagDestLocation == 0 {
		return
	}
	entry := uint32(35591)
	displayID := uint32(0)
	size := float32(1)
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var display, sizeValue int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT displayId, size FROM gameobject_template WHERE entry = ? LIMIT 1", entry).Scan(&display, &sizeValue); err == nil {
			displayID = uint32(display)
			if sizeValue > 0 {
				size = float32(sizeValue)
			}
		}
	}
	lowGUID := s.server.nextDynamicGameObjectLowGUID()
	dyn := &dynamicGameObjectState{GUID: gameObjectGUID(lowGUID, entry), LowGUID: lowGUID, Entry: entry, OwnerGUID: s.playerGUID, Map: s.player.Map, X: target.Destination.X, Y: target.Destination.Y, Z: target.Destination.Z, Orientation: s.player.Orientation, State: GameObjectStateActive, Type: GameObjectTypeFishingNode, DisplayID: displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, IsRuntimeSpawn: true}
	dyn.AutoCloseTimer = time.AfterFunc(5*time.Second, func() {
		s.server.setGameObjectState(dyn.GUID, GameObjectStateReady)
		s.server.broadcastGameObjectCustomAnim(dyn.Map, dyn.GUID, 0)
	})
	dyn.DespawnTimer = time.AfterFunc(30*time.Second, func() { s.server.despawnDynamicGameObject(dyn.GUID) })
	s.server.spawnDynamicGameObject(dyn)
}

func (s *Server) despawnDynamicGameObject(guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	s.objectsMu.Lock()
	var mapID uint32
	var ownerGUID uint64
	var fishingHandled bool
	if dyn, ok := s.dynamicGameObjects[guid]; ok && dyn != nil {
		mapID = dyn.Map
		ownerGUID = dyn.OwnerGUID
		fishingHandled = dyn.FishingHandled
		if dyn.AutoCloseTimer != nil {
			dyn.AutoCloseTimer.Stop()
		}
		if dyn.DespawnTimer != nil {
			dyn.DespawnTimer.Stop()
		}
		delete(s.dynamicGameObjects, guid)
	}
	s.objectsMu.Unlock()

	if ownerGUID != 0 && !fishingHandled {
		if owner := s.findSessionByGUID(ownerGUID); owner != nil {
			_ = owner.write(uint16(protocol.OpcodeSMSG_FISH_ESCAPED), nil, true)
		}
	}
	s.broadcastGameObjectDespawn(mapID, guid)
}

func (s *Server) setGameObjectHidden(guid uint64, hidden bool) {
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	if s.hiddenGameObjects == nil {
		s.hiddenGameObjects = make(map[uint64]struct{})
	}
	if hidden {
		s.hiddenGameObjects[guid] = struct{}{}
	} else {
		delete(s.hiddenGameObjects, guid)
	}
}
