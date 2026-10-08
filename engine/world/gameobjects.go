package world

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
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

// Game object types mirroring TrinityCore GameobjectTypes (SharedDefines.h:1592).
const (
	GameObjectTypeDoor                 uint8 = 0
	GameObjectTypeButton               uint8 = 1
	GameObjectTypeQuestGiver           uint8 = 2
	GameObjectTypeChest                uint8 = 3
	GameObjectTypeBinder               uint8 = 4
	GameObjectTypeGeneric              uint8 = 5
	GameObjectTypeTrap                 uint8 = 6
	GameObjectTypeChair                uint8 = 7
	GameObjectTypeSpellFocus           uint8 = 8
	GameObjectTypeText                 uint8 = 9
	GameObjectTypeGoober               uint8 = 10
	GameObjectTypeTransport            uint8 = 11
	GameObjectTypeAreaDamage           uint8 = 12
	GameObjectTypeCamera               uint8 = 13
	GameObjectTypeMapObject            uint8 = 14
	GameObjectTypeMOTransport          uint8 = 15
	GameObjectTypeDuelArbiter          uint8 = 16
	GameObjectTypeFishingNode          uint8 = 17
	GameObjectTypeRitual               uint8 = 18
	GameObjectTypeMailbox              uint8 = 19
	GameObjectTypeDOONotUse            uint8 = 20
	GameObjectTypeGuardPost            uint8 = 21
	GameObjectTypeSpellCaster          uint8 = 22
	GameObjectTypeMeetingStone         uint8 = 23
	GameObjectTypeFlagStand            uint8 = 24
	GameObjectTypeFishingHole          uint8 = 25
	GameObjectTypeFlagDrop             uint8 = 26
	GameObjectTypeMiniGame             uint8 = 27
	GameObjectTypeCapturePoint         uint8 = 29
	GameObjectTypeAuraGenerator        uint8 = 30
	GameObjectTypeDungeonDifficulty    uint8 = 31
	GameObjectTypeBarberChair          uint8 = 32
	GameObjectTypeDestructibleBuilding uint8 = 33
	GameObjectTypeGuildBank            uint8 = 34
	GameObjectTypeTrapDoor             uint8 = 35
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
	SpellID         uint32 // creating spell, for Spell::cancel's RemoveGameObject(spellId) sweep (Spell.cpp:3255)
	FishingHandled  bool
	FishingUses     uint32
	FishingMaxOpens uint32
	Map             uint32
	InstanceID      uint32
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
	IconName        string
	Flags           uint32
	Faction         uint32
	Data1           uint32
	ParentRotation  [4]float32
	AutoCloseTimer  *time.Timer
	DespawnTimer    *time.Timer
	Hidden          bool
	IsRuntimeSpawn  bool
	ChairSlots      map[uint32]uint64 // chair slot index -> occupant player GUID (GAMEOBJECT_TYPE_CHAIR)
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

func (s *Server) buildNearbyGameObjectUpdates(ctx context.Context, state playerState, includeTransportRoots bool, observer *session) (*protocol.Packet, int, error) {
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
		if dyn := s.gameObjectState(state.Map, state.InstanceID, gameObjectGUID(spawn.GUID, spawn.Entry)); dyn != nil {
			spawn.State = dyn.State
		}
		// Server-side visibility: script-hidden objects stay visible to
		// GMs the way SetVisible(false) units remain visible to GM seers.
		if !isGM && s.isGameObjectHiddenInInstance(state.Map, state.InstanceID, gameObjectGUID(spawn.GUID, spawn.Entry)) {
			continue
		}
		updates.AddUpdateBlock(buildGameObjectUpdate(spawn))
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, count, err
	}
	for _, passenger := range s.nearbyTransportObjectPassengers(state, distance) {
		if observer != nil && !observer.markTransportPassengerVisible(gameObjectGUID(passenger.GUID, passenger.Entry), passenger.TransportGUID) {
			continue
		}
		updates.AddUpdateBlock(buildGameObjectUpdate(passenger))
		count++
	}
	for _, dyn := range s.gameObjectStatesInInstance(state.Map, state.InstanceID) {
		if dyn.Hidden || !dyn.IsRuntimeSpawn {
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
		return buildTransportGameObjectUpdate(spawn, protocol.UpdateCreateObject)
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
	if spawn.Type == GameObjectTypeMOTransport && spawn.TransportPeriod > 0 {
		values[gameObjectLevel] = spawn.TransportPeriod
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
	return buildTransportGameObjectUpdate(spawn, protocol.UpdateValues)
}

func buildTransportGameObjectUpdate(spawn gameObjectSpawn, updateType uint8) []byte {
	rawGUID := transportGUID(spawn.GUID)
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
	if spawn.Type == GameObjectTypeMOTransport && spawn.TransportPeriod > 0 {
		values[gameObjectLevel] = spawn.TransportPeriod
	}
	block := protocol.NewBuffer(256)
	if updateType == protocol.UpdateValues {
		block.WriteU8(protocol.UpdateValues)
		block.WritePackedGUID(rawGUID)
		mask := protocol.NewUpdateMask(len(values))
		_ = mask.Set(gameObjectDynamic)
		block.WriteU8(uint8(mask.BlockCount()))
		mask.AppendTo(block)
		block.WriteU32(values[gameObjectDynamic])
		return block.Bytes()
	}
	block.WriteU8(updateType)
	block.WritePackedGUID(rawGUID)
	block.WriteU8(5)
	block.WriteU16(transportGameObjectUpdateFlags)
	block.WriteF32(spawn.X)
	block.WriteF32(spawn.Y)
	block.WriteF32(spawn.Z)
	block.WriteF32(spawn.Orientation)
	block.WriteU32(spawn.GUID)
	block.WriteU32(spawn.TransportProgress)
	block.WriteU64(packGameObjectRotation(spawn.RotationX, spawn.RotationY, spawn.RotationZ, spawn.RotationW))
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
	return block.Bytes()
}

func gameObjectGUID(guid, entry uint32) uint64 {
	return uint64(guid) | uint64(entry)<<24 | uint64(0xF110)<<48
}

func transportGUID(guid uint32) uint64 {
	return uint64(guid) | uint64(0x1FC0)<<48
}

func gameObjectDynamicValue(spawn gameObjectSpawn) uint32 {
	if (spawn.Type == GameObjectTypeTransport || spawn.Type == GameObjectTypeMOTransport) && spawn.TransportPeriod > 0 {
		progress := uint16(uint32(float32(spawn.TransportProgress%spawn.TransportPeriod) / float32(spawn.TransportPeriod) * 65535))
		return uint32(progress) << 16
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
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return false
	}
	guid, err := readObjectGUID(payload)
	if err != nil || guid == 0 {
		return false
	}

	entry := uint32((guid >> 24) & 0x00FFFFFF)
	// ACHIEVEMENT_CRITERIA_TYPE_USE_GAMEOBJECT is updated by CMSG_GAMEOBJ_REPORT_USE
	// (HandleGameobjectReportUse, SpellHandler.cpp:318), not by CMSG_GAMEOBJ_USE.
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
	if spawn, ok := s.server.transportSpawnForGUID(guid); ok {
		return s.handleTransportUse(spawn)
	}

	goState, err := s.server.getOrLoadGameObjectState(ctx, guid, lowGUID, entry, s.player.Map, s.player.InstanceID)
	if err != nil || goState == nil {
		return false
	}

	// Range check: GameObject::GetInteractionDistance (GameObject.cpp:2639)
	// grants the fishing node (bobber, up to ~30y cast range) 100 yards and
	// the fishing hole 20.0+CONTACT_DISTANCE (20.5); every other type keeps
	// the 10.0-yard standard interaction distance.
	maxUseDist := 10.0
	switch goState.Type {
	case GameObjectTypeFishingNode:
		maxUseDist = 100.0
	case GameObjectTypeFishingHole:
		maxUseDist = 20.5
	}
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > maxUseDist {
		return true
	}

	// Player::GetGameObjectIfCanInteractWith (Player.cpp:2379): players cannot
	// interact with gameobjects that use the "Point" icon.
	if goState.IconName == "Point" {
		return true
	}

	// Eluna GOSSIP_EVENT_ON_HELLO (1) for the gameobject_gossip bindings,
	// fired at the head of GameObject::Use (GameObject.cpp:1502) after the
	// interact gates (HandleGameObjectUseOpcode, SpellHandler.cpp:300) and
	// before the type arms: a Lua false return skips the native arms, like
	// the C++ early return.
	if s.fireGameObjectGossipHelloHook(ctx, guid) {
		return true
	}

	switch goState.Type {
	case GameObjectTypeDoor:
		s.server.useDoorOrButton(goState.Map, goState.InstanceID, guid)

	case GameObjectTypeButton:
		// Press button
		s.server.setGameObjectStateInInstance(goState.Map, goState.InstanceID, guid, GameObjectStateActive)
		s.server.broadcastGameObjectCustomAnimInInstance(goState.Map, goState.InstanceID, guid, 0)
		s.server.scheduleGameObjectResetInInstance(goState.Map, goState.InstanceID, guid, 5*time.Second)

	case GameObjectTypeChest:
		// Chests open through the lock path: the client casts Opening and
		// Spell::EffectOpenLock answers Spell::SendLoot(guid, LOOT_SKINNING)
		// (SpellEffects.cpp:2031), so the window carries LOOT_SKINNING (not
		// LOOT_CORPSE) and SendLoot's shouldLootRelease LOOT_SKINNING arm
		// allows 20 yards (Player.cpp:8545-8550). The previous-loot release
		// and dead-player drop mirror the handleLoot head they replace
		// (Player.cpp:8526-8527, LootHandler.cpp:232-233).
		if s.isDeadOrGhost() {
			return true
		}
		if prev := s.activeLoot; prev != nil && prev.TargetGUID != guid {
			s.doLootRelease(prev)
		}
		return s.openGameObjectLoot(ctx, guid, lootTypeSkinning, 20.0)

	case GameObjectTypeFishingNode, GameObjectTypeFishingHole:
		if goState.Type == GameObjectTypeFishingNode {
			return s.handleFishingNodeUse(ctx, payload, goState)
		}
		// GameObject::Use GAMEOBJECT_TYPE_FISHINGHOLE arm (GameObject.cpp:1998):
		// only the pool use fires ACHIEVEMENT_CRITERIA_TYPE_FISH_IN_GAMEOBJECT —
		// the fishing-node arm never does.
		s.updateAchievementCriteria(criteriaTypeFishInGameObject, entry, 1)
		return s.handleFishingHoleUse(ctx, payload, goState)

	case GameObjectTypeGoober:
		// GameObject::Use GAMEOBJECT_TYPE_GOOBER arm (GameObject.cpp:1630-1696).
		tpl := s.loadGooberTemplate(ctx, entry)
		// Page text is shown before the quest-gate break, like C++.
		if tpl.pageID != 0 {
			buf := protocol.NewBuffer(8)
			buf.WriteU64(guid)
			_ = s.write(uint16(protocol.OpcodeSMSG_GAMEOBJECT_PAGETEXT), buf.Bytes(), true)
		}
		// The goober gossip menu needs a gameobject gossip source; Go's gossip
		// path is creature-only, so the gossipID arm is unmodeled (documented).
		// The eventId script-start arm has no Go event-script model (documented).
		// The quest gate: with a questId and no incomplete quest, C++ breaks
		// out of the switch — no kill credit, no state change, no spell.
		if tpl.questID != 0 {
			status, _ := s.characterQuestStatus(ctx, tpl.questID)
			if status != questStatusIncomplete {
				return true
			}
		}
		// Player::KillCreditGO: group members at group reward distance share it.
		s.creditQuestKills(ctx, entry, guid)
		if s.groupID != 0 && s.server != nil {
			inDungeon := s.isDungeonMap(s.player.Map)
			for _, m := range s.server.getGroupSessions(s.groupID) {
				if m == s || m.player == nil {
					continue
				}
				if m.player.Map == s.player.Map && m.player.InstanceID == s.player.InstanceID &&
					(inDungeon || distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= 100.0) {
					m.creditQuestKills(ctx, entry, guid)
				}
			}
		}
		// linkedTrapId has no Go trap model (documented).
		// GO_FLAG_IN_USE / GO_ACTIVATED loot state has no Go analog; the custom
		// anim goes out only when the template sets customAnim, otherwise the GO
		// state moves to ACTIVE (documented).
		if tpl.customAnim != 0 {
			s.server.broadcastGameObjectCustomAnimInInstance(goState.Map, goState.InstanceID, guid, uint32(goState.AnimProgress))
		} else {
			s.server.setGameObjectStateInInstance(goState.Map, goState.InstanceID, guid, GameObjectStateActive)
		}
		resetDelay := 10 * time.Second
		if tpl.autoCloseTime > 0 {
			resetDelay = time.Duration(tpl.autoCloseTime) * time.Second
		}
		s.server.scheduleGameObjectResetInInstance(goState.Map, goState.InstanceID, guid, resetDelay)
		// The spell is data10 in this TrinityCore layout (GameObjectData.h:171);
		// the old code cast data1 (the questId) as the spell. C++ casts it at
		// the end of Use() with the GO as caster; Go's castSpellDirect casts as
		// the player (documented).
		if tpl.spellID != 0 {
			s.castSpellDirect(ctx, tpl.spellID, s.playerGUID)
		}

	case GameObjectTypeTrap:
		// GameObject::Use GAMEOBJECT_TYPE_TRAP arm (GameObject.cpp:1536-1549;
		// template layout GameObjectData.h:120-138).
		tpl := s.loadGameObjectTemplateData(ctx, entry)
		if tpl[3] != 0 { // trap.spellId (data3)
			s.castSpellDirect(ctx, tpl[3], s.playerGUID)
		}
		// trap.cooldown (data5) feeds m_cooldownTime in C++; Go has no GO
		// cooldown-time model, so no cooldown is modeled (documented).
		if tpl[4] == 1 { // type == 1: deactivate after trigger
			// GO_JUST_DEACTIVATED has no Go analog; the instance copy is
			// hidden and a despawn goes out to viewers.
			s.server.setGameObjectHiddenInInstance(goState.Map, goState.InstanceID, guid, true)
			s.server.broadcastGameObjectDespawn(goState.Map, guid)
		}

	case GameObjectTypeChair:
		s.useGameObjectChair(ctx, goState, entry)

	case GameObjectTypeCamera:
		// GameObject::Use GAMEOBJECT_TYPE_CAMERA arm (GameObject.cpp:1697-1713;
		// template layout GameObjectData.h:220-228).
		tpl := s.loadGameObjectTemplateData(ctx, entry)
		if tpl[1] != 0 { // camera.cinematicId (data1)
			buf := protocol.NewBuffer(4)
			buf.WriteU32(tpl[1])
			_ = s.write(uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC), buf.Bytes(), true)
		}
		// camera.eventID (data2) starts event scripts; Go has no event-script
		// model (documented).

	case GameObjectTypeSpellCaster:
		// GameObject::Use GAMEOBJECT_TYPE_SPELLCASTER arm
		// (GameObject.cpp:1905-1925; template layout GameObjectData.h:267-277).
		tpl := s.loadGameObjectTemplateData(ctx, entry)
		if tpl[2] != 0 { // partyOnly (data2)
			// C++ gates on the GO owner's player being in the same raid;
			// only runtime-spawned GOs carry OwnerGUID in Go, and a null
			// owner fails the C++ gate the same way.
			owner := s.server.playerSessionForGUID(goState.OwnerGUID)
			if owner == nil || owner.player == nil {
				return true
			}
			if s.groupID == 0 || owner.groupID == 0 || s.groupID != owner.groupID {
				return true
			}
		}
		// RemoveAurasByType(SPELL_AURA_MOUNTED) has no Go mounted-aura model
		// (documented); AddUse() has no Go use-count model (documented).
		if tpl[0] != 0 { // spellcaster.spellId (data0)
			// C++ casts with the GO as caster at the end of Use(); Go's
			// castSpellDirect casts as the player (documented).
			s.castSpellDirect(ctx, tpl[0], s.playerGUID)
		}

	case GameObjectTypeMeetingStone:
		// GameObject::Use GAMEOBJECT_TYPE_MEETINGSTONE arm
		// (GameObject.cpp:1927-1953; template layout GameObjectData.h:279-284).
		tpl := s.loadGameObjectTemplateData(ctx, entry)
		target := s.server.playerSessionForGUID(s.selection)
		if target == nil || target == s || target.player == nil {
			return true
		}
		// C++ Player::IsInSameRaidWith = same group (Player.cpp:2543).
		if s.groupID == 0 || target.groupID == 0 || s.groupID != target.groupID {
			return true
		}
		// Both players must meet the stone's min level.
		if uint32(s.player.Level) < tpl[0] || uint32(target.player.Level) < tpl[0] {
			return true
		}
		spellID := uint32(59782) // Summoning Stone Effect
		if entry == 194097 {
			spellID = 61994 // Ritual of Summoning
		}
		s.castSpellDirect(ctx, spellID, s.playerGUID)

	case GameObjectTypeBarberChair:
		// GameObject::Use GAMEOBJECT_TYPE_BARBER_CHAIR arm
		// (GameObject.cpp:2049-2065; template layout GameObjectData.h:367-372).
		tpl := s.loadGameObjectTemplateData(ctx, entry)
		// C++ keeps combat/pet state with TELE_TO_NOT_LEAVE_* flags; Go's
		// teleportTo stops combat and may unsummon the pet (documented).
		s.teleportTo(goState.Map, goState.X, goState.Y, goState.Z, goState.Orientation)
		_ = s.write(uint16(protocol.OpcodeSMSG_ENABLE_BARBER_SHOP), nil, true)
		// UNIT_STAND_STATE_SIT_LOW_CHAIR (4) + chairheight (data0).
		s.player.StandState = 4 + uint8(tpl[0])
		s.sendPlayerUpdate()
	}

	return true
}

// handleGameObjectReportUse processes CMSG_GAMEOBJ_REPORT_USE (0x481).
// Reference: WorldSession::HandleGameobjectReportUse (SpellHandler.cpp:318).
// This is the opcode that carries ACHIEVEMENT_CRITERIA_TYPE_USE_GAMEOBJECT in
// C++; CMSG_GAMEOBJ_USE (HandleGameObjectUseOpcode) never updates it.
func (s *session) handleGameObjectReportUse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return false
	}
	guid, err := readObjectGUID(payload)
	if err != nil || guid == 0 {
		return false
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	lowGUID := uint32(guid & 0x00FFFFFF)
	goState, err := s.server.getOrLoadGameObjectState(ctx, guid, lowGUID, entry, s.player.Map, s.player.InstanceID)
	if err != nil || goState == nil {
		return false
	}
	// The GetGameObjectIfCanInteractWith gates (Player.cpp:2363): the "Point"
	// icon and interaction range gates apply here as in C++.
	if goState.IconName == "Point" {
		return true
	}
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 10.0 {
		return true
	}
	// Eluna GAMEOBJECT_EVENT_ON_USE (event 14), fired from
	// HandleGameobjectReportUse after the interact gates
	// (SpellHandler.cpp:318-336): (event, go, player) — cancel skips the
	// achievement-criteria update, like the C++ early return.
	if s.fireGameObjectEvent(ctx, guid, scripting.GameObjectEventOnUse, s.luaPlayer()) {
		return true
	}
	s.updateAchievementCriteria(criteriaTypeUseGameObject, entry, 1)
	return true
}

// gooberTemplateData carries the gameobject_template data columns used by the
// GameObject::Use GAMEOBJECT_TYPE_GOOBER arm (GameObjectData.h:171-197).
type gooberTemplateData struct {
	questID       uint32 // data1
	eventID       uint32 // data2
	autoCloseTime uint32 // data3
	customAnim    uint32 // data4
	pageID        uint32 // data7
	spellID       uint32 // data10
	linkedTrapID  uint32 // data12
	gossipID      uint32 // data19
}

// loadGooberTemplate fetches the template data fields the goober use-arm reads.
func (s *session) loadGooberTemplate(ctx context.Context, entry uint32) gooberTemplateData {
	var tpl gooberTemplateData
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return tpl
	}
	var questID, eventID, autoCloseTime, customAnim, pageID, spellID, linkedTrapID, gossipID int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(Data1, 0), COALESCE(Data2, 0), COALESCE(Data3, 0), COALESCE(Data4, 0),
			COALESCE(Data7, 0), COALESCE(Data10, 0), COALESCE(Data12, 0), COALESCE(Data19, 0)
		FROM gameobject_template WHERE entry = ? LIMIT 1`, entry).
		Scan(&questID, &eventID, &autoCloseTime, &customAnim, &pageID, &spellID, &linkedTrapID, &gossipID)
	if err != nil {
		return tpl
	}
	tpl.questID = uint32(questID)
	tpl.eventID = uint32(eventID)
	tpl.autoCloseTime = uint32(autoCloseTime)
	tpl.customAnim = uint32(customAnim)
	tpl.pageID = uint32(pageID)
	tpl.spellID = uint32(spellID)
	tpl.linkedTrapID = uint32(linkedTrapID)
	tpl.gossipID = uint32(gossipID)
	return tpl
}

// loadGameObjectTemplateData fetches gameobject_template data0..data7 for the
// GameObject::Use arms that read type-specific template fields
// (GameObjectData.h:110-372).
func (s *session) loadGameObjectTemplateData(ctx context.Context, entry uint32) [8]uint32 {
	var data [8]uint32
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return data
	}
	var d [8]int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(Data0, 0), COALESCE(Data1, 0), COALESCE(Data2, 0), COALESCE(Data3, 0),
			COALESCE(Data4, 0), COALESCE(Data5, 0), COALESCE(Data6, 0), COALESCE(Data7, 0)
		FROM gameobject_template WHERE entry = ? LIMIT 1`, entry).
		Scan(&d[0], &d[1], &d[2], &d[3], &d[4], &d[5], &d[6], &d[7])
	if err != nil {
		return data
	}
	for i := range d {
		data[i] = uint32(d[i])
	}
	return data
}

// useGameObjectChair mirrors the GAMEOBJECT_TYPE_CHAIR arm of GameObject::Use
// (GameObject.cpp:1550-1629): the player teleports to the nearest free slot
// along the chair's orthogonal axis and sits.
func (s *session) useGameObjectChair(ctx context.Context, goState *dynamicGameObjectState, entry uint32) {
	if s.server == nil || s.player == nil {
		return
	}
	tpl := s.loadGameObjectTemplateData(ctx, entry)
	slots := tpl[0] // chair.slots (data0)
	if slots == 0 {
		slots = 1 // C++ DB-error fallback: one default slot
	}
	height := tpl[1] // chair.height (data1)
	orthogonal := float64(goState.Orientation) + math.Pi/2
	slotPos := func(i uint32) (float32, float32) {
		relative := float64(goState.Size)*float64(i) - float64(goState.Size)*float64(slots-1)/2.0
		return goState.X + float32(relative*math.Cos(orthogonal)), goState.Y + float32(relative*math.Sin(orthogonal))
	}
	sv := s.server
	// Snapshot occupant GUIDs before resolving sessions so no session lock is
	// ever taken while holding objectsMu.
	sv.objectsMu.RLock()
	snapshot := make(map[uint32]uint64, slots)
	for i := uint32(0); i < slots; i++ {
		if og := goState.ChairSlots[i]; og != 0 {
			snapshot[i] = og
		}
	}
	sv.objectsMu.RUnlock()
	// A slot stays taken only while its occupant is online, in a chair sit
	// state (C++: IsSitState() && GetStandState() != UNIT_STAND_STATE_SIT),
	// and within 0.1 yards of the slot position.
	occupied := make(map[uint32]bool, len(snapshot))
	for i, og := range snapshot {
		sx, sy := slotPos(i)
		occ := sv.playerSessionForGUID(og)
		if occ == nil || occ.player == nil {
			continue
		}
		switch occ.player.StandState {
		case 2, 4, 5, 6: // SIT_CHAIR, SIT_LOW/MEDIUM/HIGH_CHAIR
		default:
			continue
		}
		dx := float64(occ.player.X - sx)
		dy := float64(occ.player.Y - sy)
		if math.Hypot(dx, dy) < 0.1 {
			occupied[i] = true
		}
	}
	var nearestSlot uint32
	nearestX, nearestY := goState.X, goState.Y
	foundFree := false
	lowestDist := math.MaxFloat64
	sv.objectsMu.Lock()
	if goState.ChairSlots == nil {
		goState.ChairSlots = make(map[uint32]uint64, slots)
	}
	for i := uint32(0); i < slots; i++ {
		if occupied[i] {
			continue
		}
		goState.ChairSlots[i] = 0
		foundFree = true
		sx, sy := slotPos(i)
		dx := float64(s.player.X - sx)
		dy := float64(s.player.Y - sy)
		if dist := math.Hypot(dx, dy); dist <= lowestDist {
			lowestDist = dist
			nearestSlot = i
			nearestX, nearestY = sx, sy
		}
	}
	if foundFree {
		goState.ChairSlots[nearestSlot] = s.playerGUID
	}
	sv.objectsMu.Unlock()
	if !foundFree {
		return
	}
	s.teleportTo(goState.Map, nearestX, nearestY, goState.Z, goState.Orientation)
	s.player.StandState = 4 + uint8(height) // UNIT_STAND_STATE_SIT_LOW_CHAIR + height
	s.sendPlayerUpdate()
}

func (s *Server) getOrLoadGameObjectState(ctx context.Context, guid uint64, lowGUID, entry, mapID, instanceID uint32) (*dynamicGameObjectState, error) {
	if s == nil {
		return nil, nil
	}
	if dyn := s.gameObjectState(mapID, instanceID, guid); dyn != nil {
		return dyn, nil
	}

	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return nil, nil
	}

	var goMap, goState, goType, displayID, data1 int64
	var goX, goY, goZ, goO, size float64
	var iconName string
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, g.orientation, g.state,
		t.type, t.displayId, t.size, COALESCE(t.data1, 0), COALESCE(t.IconName, '')
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.guid = ? AND g.id = ? LIMIT 1`, lowGUID, entry).Scan(&goMap, &goX, &goY, &goZ, &goO, &goState, &goType, &displayID, &size, &data1, &iconName)
	if err != nil {
		return nil, err
	}

	dyn := &dynamicGameObjectState{
		GUID:        guid,
		LowGUID:     lowGUID,
		Entry:       entry,
		Map:         uint32(goMap),
		InstanceID:  instanceID,
		X:           float32(goX),
		Y:           float32(goY),
		Z:           float32(goZ),
		Orientation: float32(goO),
		State:       uint8(goState),
		Type:        uint8(goType),
		DisplayID:   uint32(displayID),
		Size:        float32(size),
		IconName:    iconName,
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

	s.storeGameObjectState(dyn)

	return dyn, nil
}

func (s *Server) setGameObjectState(guid uint64, state uint8) {
	if s == nil {
		return
	}
	found := false
	s.objectsMu.Lock()
	if s.dynamicGameObjects == nil {
		s.objectsMu.Unlock()
		return
	}
	if dyn, ok := s.dynamicGameObjects[guid]; ok && dyn != nil {
		dyn.State = state
		found = true
	}
	s.objectsMu.Unlock()
	// Eluna GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED (event 10), fired from
	// GameObject::SetGoState (GameObject.cpp:2407): (event, go, state).
	// The hook runs after objectsMu is released — Lua handlers must never
	// run under the object lock.
	if found {
		s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnGOStateChange, uint32(state))
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
		// Routed through setGameObjectState so the Eluna
		// GAMEOBJECT_EVENT_ON_GO_STATE_CHANGED hook fires, matching C++
		// where the door auto-close goes through GameObject::SetGoState.
		s.setGameObjectState(guid, GameObjectStateReady)
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
		if !target.authed || !target.worldReady.Load() || target.player == nil || target.player.Map != mapID {
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
	s.storeGameObjectState(dyn)

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
		if dyn.InstanceID != 0 {
			s.broadcastToInstance(dyn.Map, dyn.InstanceID, packet.Opcode, packet.Payload.Bytes(), nil)
		} else {
			s.broadcastToMap(dyn.Map, packet.Opcode, packet.Payload.Bytes())
		}
	}
	// Eluna GAMEOBJECT_EVENT_ON_ADD (event 12), fired from
	// GameObject::AddToWorld (GameObject.cpp:233): (event, gameobject).
	// Only dynamic spawns have a live server-side object — static template
	// GOs are per-client update packets, so their map add has no Go
	// counterpart.
	s.triggerGameObjectEvent(context.Background(), dyn.GUID, scripting.GameObjectEventOnAdd)
	// Eluna INSTANCE_EVENT_ON_GAMEOBJECT_CREATE (event 6), fired from
	// GameObject::AddToWorld (GameObject.cpp:213) via the instance's zone
	// script. Instance maps only; the static-spawn delta is documented in
	// engine/world/instance_hooks.go.
	s.triggerInstanceGameObjectCreate(dyn)
}

func isFishingSpell(spellID uint32) bool {
	switch spellID {
	case 7620, 7731, 7732, 18248, 33095, 51294:
		return true
	default:
		return false
	}
}

func (s *session) spawnFishingBobber(ctx context.Context, target protocol.SpellTargetData, spellID uint32, durationMs int64) {
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
	dyn := &dynamicGameObjectState{GUID: gameObjectGUID(lowGUID, entry), LowGUID: lowGUID, Entry: entry, OwnerGUID: s.playerGUID, SpellID: spellID, Map: s.player.Map, InstanceID: s.player.InstanceID, X: target.Destination.X, Y: target.Destination.Y, Z: target.Destination.Z, Orientation: s.player.Orientation, State: GameObjectStateActive, Type: GameObjectTypeFishingNode, DisplayID: displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, IsRuntimeSpawn: true}
	// SpellEffects.cpp:4982-4999: the bobber's lifetime is the spell
	// duration shortened by a lastSec roll (urand(0,2) -> {3,7,13}s), and
	// the bite lands FISHING_BOBBER_READY_TIME (5s, GameObject.h:77) before
	// the timeout (GameObject.cpp:510-534) — the catch window is exactly
	// those 5s; a use before the bite answers FISH_NOT_HOOKED
	// (GameObject.cpp:1798-1803).
	lifetime := durationMs
	if shortened := durationMs - int64([]int32{3, 7, 13}[rand.Intn(3)])*1000 + 5000; shortened < lifetime {
		lifetime = shortened
	}
	if lifetime < 6000 {
		lifetime = 6000
	}
	biteAt := lifetime - 5000
	dyn.AutoCloseTimer = time.AfterFunc(time.Duration(biteAt)*time.Millisecond, func() {
		s.server.setGameObjectStateInInstance(dyn.Map, dyn.InstanceID, dyn.GUID, GameObjectStateReady)
		s.server.broadcastGameObjectCustomAnimInInstance(dyn.Map, dyn.InstanceID, dyn.GUID, 0)
	})
	dyn.DespawnTimer = time.AfterFunc(time.Duration(lifetime)*time.Millisecond, func() {
		if s.fishingBobberGUID == dyn.GUID {
			s.fishingBobberGUID = 0
		}
		s.server.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
	})
	s.fishingBobberGUID = dyn.GUID
	s.server.spawnDynamicGameObject(dyn)
}

// cancelFishingBobber mirrors the channeled arm of Spell::cancel
// (Spell.cpp:3251-3258) for fishing: cancelling the fishing channel
// deletes the bobber (Unit::RemoveGameObject(spellId, true),
// Unit.cpp:5263-5284 — the owner link is cleared before Delete, so no
// SMSG_FISH_ESCAPED goes out; that packet only fires on the bobber's
// bite-timeout arm, GameObject.cpp:585). Go arms no activeChannel for
// fishing (finishSpellCast returns before startChannel), so the
// channel-break paths never reach removeChannelGameObjects for it — the
// live bobber is tracked on the session and cancelled explicitly here.
func (s *session) cancelFishingBobber() {
	if s == nil || s.server == nil || s.fishingBobberGUID == 0 {
		return
	}
	guid := s.fishingBobberGUID
	s.fishingBobberGUID = 0
	var mapID, instanceID uint32
	found := false
	s.server.objectsMu.Lock()
	if st := s.server.gameObjectStateLocked(s.player.Map, s.player.InstanceID, guid); st != nil {
		st.FishingHandled = true
		mapID, instanceID = st.Map, st.InstanceID
		found = true
	}
	s.server.objectsMu.Unlock()
	if found {
		s.server.despawnDynamicGameObjectInInstance(mapID, instanceID, guid)
	}
}

// removeChannelGameObjects mirrors the channeled-spell arm of Spell::cancel
// (Spell.cpp:3251-3258): cancelling a channeled spell removes the game
// objects the channel summoned (Unit::RemoveGameObject(spellId, true),
// Unit.cpp:5263-5284). The owner link is cleared before the delete, so no
// SMSG_FISH_ESCAPED goes out on this path (that packet only fires on the
// bobber's bite-timeout arm, GameObject.cpp:585). The fishing bobber
// (EffectTransmitted FISHINGNODE arm, SpellEffects.cpp:4982-5000) is the
// only runtime GO a player channel summons in Go; without this an
// interrupted fishing channel (movement, new cast, damage abort) left the
// bobber in the world, still catchable at the 100-yard bobber use range.
func (s *session) removeChannelGameObjects(spellID uint32) {
	if s == nil || s.server == nil || spellID == 0 {
		return
	}
	type goRef struct {
		mapID      uint32
		instanceID uint32
		guid       uint64
	}
	var refs []goRef
	s.server.objectsMu.Lock()
	collect := func(state *dynamicGameObjectState) {
		if state != nil && state.IsRuntimeSpawn && state.OwnerGUID == s.playerGUID && state.SpellID == spellID {
			state.OwnerGUID = 0 // C++ clears the owner link before Delete (Unit.cpp:5273)
			refs = append(refs, goRef{mapID: state.Map, instanceID: state.InstanceID, guid: state.GUID})
		}
	}
	for _, state := range s.server.dynamicGameObjects {
		collect(state)
	}
	for _, byGUID := range s.server.instanceGameObjects {
		for _, state := range byGUID {
			collect(state)
		}
	}
	s.server.objectsMu.Unlock()
	for _, ref := range refs {
		// OwnerGUID was cleared above, so the despawn skips SMSG_FISH_ESCAPED
		// exactly like C++'s silent Delete.
		s.server.despawnDynamicGameObjectInInstance(ref.mapID, ref.instanceID, ref.guid)
	}
}

func (s *Server) despawnDynamicGameObject(guid uint64) {
	if s == nil || guid == 0 {
		return
	}
	s.objectsMu.Lock()
	var mapID uint32
	var ownerGUID uint64
	var fishingHandled bool
	removed := false
	if dyn, ok := s.dynamicGameObjects[guid]; ok && dyn != nil {
		mapID = dyn.Map
		ownerGUID = dyn.OwnerGUID
		fishingHandled = dyn.FishingHandled
		removed = true
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
	// Eluna GAMEOBJECT_EVENT_ON_REMOVE (event 13), fired from
	// GameObject::RemoveFromWorld (GameObject.cpp:244): (event, gameobject).
	// Only dynamic despawns have a live server-side object — static template
	// GOs are per-client update packets, so their map removal has no Go
	// counterpart.
	if removed {
		s.triggerGameObjectEvent(context.Background(), guid, scripting.GameObjectEventOnRemove)
	}
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
