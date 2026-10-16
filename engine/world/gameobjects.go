package world

import (
	"context"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
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
	PhaseMask       uint32 // gameobject.phaseMask (InSamePhase gate, GameObject.cpp:2760)
	Data1           uint32
	ParentRotation  [4]float32
	AutoCloseTimer  *time.Timer
	DespawnTimer    *time.Timer
	// SpawnState is the state the GO was created with: the m_prevGoState twin
	// (GameObject.h:314). C++ sets it once in Create (GameObject.cpp:131,349)
	// and ResetDoorOrButton restores it (GameObject.cpp:1428) — it is NOT the
	// pre-use state.
	SpawnState      uint8
	RitualOwnerGUID uint64              // summoning-ritual owner for ownerless GOs (m_ritualOwnerGUID twin, GameObject.cpp:1803)
	RitualUsers     map[uint64]struct{} // summoning-ritual unique participants (m_unique_users twin)
	CooldownUntilMS int64               // template trap/goober Use-cooldown gate (m_cooldownTime twin, GameObject.cpp:1513)
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
		// Destructible-building runtime overrides (damage/repair/
		// destruction-state spell effects): static GOs have no live state
		// object, so their flags/display/anim-progress live in the
		// destructible health record.
		if over := s.destructibleSpawnOverride(state.Map, state.InstanceID, spawn.GUID); over != nil {
			spawn.Flags = over.Flags
			spawn.DisplayID = over.DisplayID
			spawn.AnimProgress = over.AnimProgress
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
			AnimProgress:   dyn.AnimProgress,
			ArtKit:         dyn.ArtKit,
			Type:           dyn.Type,
			DisplayID:      dyn.DisplayID,
			Size:           dyn.Size,
			Flags:          dyn.Flags,
			Faction:        dyn.Faction,
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

// goCannotBeUsedUnderImmunity mirrors GameObjectTemplate::CannotBeUsedUnderImmunity
// (GameObjectData.h:497-513): per-type noDamageImmune template fields; chests
// can never be opened while immune (C++ hardcodes true for GAMEOBJECT_TYPE_CHEST).
func goCannotBeUsedUnderImmunity(goType uint8, tpl [24]uint32) bool {
	switch goType {
	case GameObjectTypeDoor: // door.noDamageImmune (data3)
		return tpl[3] != 0
	case GameObjectTypeButton: // button.noDamageImmune (data4)
		return tpl[4] != 0
	case GameObjectTypeQuestGiver: // questgiver.noDamageImmune (data5)
		return tpl[5] != 0
	case GameObjectTypeChest:
		return true
	case GameObjectTypeGoober: // goober.noDamageImmune (data11)
		return tpl[11] != 0
	case GameObjectTypeFlagStand: // flagstand.noDamageImmune (data5)
		return tpl[5] != 0
	case GameObjectTypeFlagDrop: // flagdrop.noDamageImmune (data3)
		return tpl[3] != 0
	}
	return false
}

// goIsUsableMounted mirrors GameObjectTemplate::IsUsableMounted
// (GameObjectData.h:436-449): mailbox is always usable mounted, barber chairs
// never, the rest read their type's allowMounted template field.
func goIsUsableMounted(goType uint8, tpl [24]uint32) bool {
	switch goType {
	case GameObjectTypeMailbox:
		return true
	case GameObjectTypeBarberChair:
		return false
	case GameObjectTypeQuestGiver: // questgiver.allowMounted (data8)
		return tpl[8] != 0
	case GameObjectTypeText: // text.allowMounted (data3)
		return tpl[3] != 0
	case GameObjectTypeGoober: // goober.allowMounted (data17)
		return tpl[17] != 0
	case GameObjectTypeSpellCaster: // spellcaster.allowMounted (data3)
		return tpl[3] != 0
	}
	return true
}

// handleGameObjectUse processes CMSG_GAMEOBJ_USE (0x0B1).
// Reference: WorldSession::HandleGameObjectUseOpcode (SpellHandler.cpp:300) -> GameObject::Use (GameObject.cpp:1290).
// admitGameObjectUse applies the GameObject::Use template-cooldown gate
// (GameObject.cpp:1513-1518): trap.cooldown / goober.cooldown
// (GameObjectTemplate::GetCooldown, GameObjectData.h:582) arm m_cooldownTime,
// and a use inside the window is dropped before the type switch; the window
// re-arms on every admitted use. Other types have no template cooldown, so
// no gate. Returns false when the use is blocked.
func (s *session) admitGameObjectUse(entry uint32, goState *dynamicGameObjectState) bool {
	if s == nil || s.server == nil || goState == nil {
		return true
	}
	if goState.Type != GameObjectTypeTrap && goState.Type != GameObjectTypeGoober {
		return true
	}
	cd := s.server.gameObjectTemplateCooldown(entry, goState.Type)
	if cd <= 0 {
		return true
	}
	now := time.Now().UnixMilli()
	s.server.objectsMu.Lock()
	blocked := now < goState.CooldownUntilMS
	if !blocked {
		goState.CooldownUntilMS = now + int64(cd)*1000
	}
	s.server.objectsMu.Unlock()
	return !blocked
}

// useGooberGameObject runs the GameObject::Use GAMEOBJECT_TYPE_GOOBER arm
// (GameObject.cpp:1630-1696). It is shared by the CMSG_GAMEOBJ_USE path and
// the lockpick path: Spell::SendLoot answers a lockpicked goober with
// gameObjTarget->Use(player) directly (SpellEffects.cpp:1892-1895) — the
// gossip-hello hook, template cooldown gate, and this arm — instead of a
// loot window.
func (s *session) useGooberGameObject(ctx context.Context, guid uint64, entry uint32, goState *dynamicGameObjectState) {
	// GameObject::Use GAMEOBJECT_TYPE_GOOBER arm (GameObject.cpp:1630-1696).
	tpl := s.loadGooberTemplate(ctx, entry)
	// GameObject::Use GAMEOBJECT_TYPE_GOOBER arm (GameObject.cpp): the
	// pageId and gossipID arms are exclusive — the page shows or the gossip
	// menu opens, never both.
	if tpl.pageID != 0 {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(guid)
		_ = s.write(uint16(protocol.OpcodeSMSG_GAMEOBJECT_PAGETEXT), buf.Bytes(), true)
	} else if tpl.gossipID != 0 {
		// PrepareGossipMenu(gossipID) with showQuests=false, then
		// SendPreparedGossip (GameObject.cpp GOOBER arm): the menu lists
		// only GOSSIP_OPTION_GOSSIP options for goober sources
		// (Player.cpp:14474-14484), and the arm continues into the quest
		// gate / kill credit / spell below rather than returning.
		s.gossipClosed = false
		gossipMenu, gerr := s.prepareGameObjectGossip(ctx, guid, entry, tpl.gossipID, s.gameObjectDefaultGossipMenuID(ctx, entry, GameObjectTypeGoober), false)
		if gerr != nil {
			s.debug("goober gossip load failed", "account", s.accountName, "entry", entry, "menu", tpl.gossipID, "error", gerr)
		} else {
			s.gossip = gossipMenu
			if serr := s.sendGossipMenu(); serr != nil {
				s.debug("goober gossip response failed", "account", s.accountName, "entry", entry, "error", serr)
			}
		}
	}
	// The eventId script-start arm has no Go event-script model (documented).
	// The quest gate: with a questId and no incomplete quest, C++ breaks
	// out of the switch — no kill credit, no state change, no spell.
	if tpl.questID != 0 {
		status, _ := s.characterQuestStatus(ctx, tpl.questID)
		if status != questStatusIncomplete {
			return
		}
	}
	// Player::KillCreditGO: group members at group reward distance share
	// it (GameObject.cpp:1666-1672; Player::IsAtGroupRewardDistance,
	// Player.cpp:24160-24173 — the distance anchor is the GO, and a dead
	// member's distance anchors on their corpse via groupRewardAnchorPos).
	s.creditQuestGOActivation(ctx, entry, guid)
	if s.groupID != 0 && s.server != nil {
		inDungeon := s.isDungeonMap(goState.Map)
		for _, m := range s.server.getGroupSessions(s.groupID) {
			if m == s || m.player == nil {
				continue
			}
			if m.player.Map != goState.Map || m.player.InstanceID != goState.InstanceID {
				continue
			}
			mx, my, mz := groupRewardAnchorPos(m)
			if inDungeon || distance3D(goState.X, goState.Y, goState.Z, mx, my, mz) <= s.server.Config.MaxGroupXPDistance {
				m.creditQuestGOActivation(ctx, entry, guid)
			}
		}
	}
	// linkedTrapId has no Go trap model (documented).
	// GO_FLAG_IN_USE is transient here (C++ sets it at use and the
	// GO_ACTIVATED update arm drops it at the next tick), so it is not
	// modeled; the state moves to ACTIVE with the custom anim, or to
	// ACTIVE directly (documented).
	if tpl.customAnim != 0 {
		s.server.broadcastGameObjectCustomAnimInInstance(goState.Map, goState.InstanceID, guid, uint32(goState.AnimProgress))
	} else {
		s.server.setGameObjectStateInInstance(goState.Map, goState.InstanceID, guid, GameObjectStateActive)
	}
	// m_cooldownTime = GetGameTimeMS() + GetAutoCloseTime()
	// (GameObject.cpp:1689): the raw template value is consumed as
	// milliseconds against the ms game clock. With no autoCloseTime the
	// GO_ACTIVATED update arm fires at the next tick and only re-readies
	// the state when the template carries a lockId or autoCloseTime —
	// otherwise the goober stays ACTIVE and is immediately reusable, so
	// no reset is scheduled.
	if tpl.autoCloseTime > 0 {
		s.server.scheduleGameObjectResetInInstance(goState.Map, goState.InstanceID, guid, time.Duration(tpl.autoCloseTime)*time.Millisecond, false)
	}
	// The spell is data10 in this TrinityCore layout (GameObjectData.h:171);
	// the old code cast data1 (the questId) as the spell. C++ casts it at
	// the end of Use() with the GO as caster; Go's castSpellDirect casts as
	// the player (documented).
	if tpl.spellID != 0 {
		s.castSpellDirect(ctx, tpl.spellID, s.playerGUID)
	}

}

// gameObjectInteractionDistance mirrors GameObject::GetInteractionDistance
// (GameObject.cpp:2639-2672): the locked-GO spell-range leg lives in the
// lock-spell path; the flat 10.0-yard standard here covers only the
// unlocked/mailbox/guildbank non-blizzlike-10.0 arms.
func gameObjectInteractionDistance(goType uint8) float64 {
	switch goType {
	case GameObjectTypeAreaDamage:
		return 0.0
	case GameObjectTypeQuestGiver, GameObjectTypeText, GameObjectTypeFlagStand, GameObjectTypeFlagDrop, GameObjectTypeMiniGame:
		return 5.5555553
	case GameObjectTypeBinder, GameObjectTypeMailbox, GameObjectTypeGuildBank:
		return 10.0
	case GameObjectTypeChair, GameObjectTypeBarberChair:
		return 3.0
	case GameObjectTypeFishingNode:
		return 100.0
	case GameObjectTypeFishingHole:
		return 20.5 // 20.0 + CONTACT_DISTANCE
	case GameObjectTypeCamera, GameObjectTypeMapObject, GameObjectTypeDungeonDifficulty, GameObjectTypeDestructibleBuilding, GameObjectTypeDoor:
		return 5.0
	default:
		return 5.0 // INTERACTION_DISTANCE
	}
}

func (s *session) handleGameObjectUse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return false
	}
	guid, err := readObjectGUID(payload)
	if err != nil || guid == 0 {
		return false
	}
	// Player::GetGameObjectIfCanInteractWith (Player.cpp:2371): a player in
	// flight cannot interact with gameobjects — the gate fires inside
	// HandleGameObjectUseOpcode (SpellHandler.cpp:300) before obj->Use, so
	// the BG-flag delegate arms are gated too.
	if s.isInFlight() {
		return true
	}
	// Player::GetGameObjectIfCanInteractWith (Player.cpp:2366): !IsInWorld()
	// returns nullptr — a player still on a loading screen (far teleport in
	// flight, worldReady false) silently ignores the opcode.
	if !s.worldReady.Load() {
		return true
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
		// HandleGameObjectUseOpcode (SpellHandler.cpp:304): a null
		// GetGameObjectIfCanInteractWith result silently returns — stale
		// GUIDs occur naturally (despawned GOs), and dropping the
		// connection for one was a Go-only client kill.
		return true
	}

	// GameObject::IsWithinDistInMap (GameObject.cpp:2760-2763): IsInMap &&
	// InSamePhase && AtInteractDistance — the range gate below is the
	// AtInteractDistance leg (GetInteractionDistance table).
	maxUseDist := gameObjectInteractionDistance(goState.Type)
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > maxUseDist {
		return true
	}
	// InSamePhase leg (Object.h:324): a zero stored mask is the C++
	// PHASEMASK_NORMAL default. A forged opcode could otherwise loot or
	// quest-credit a phased-out GO.
	goPhase := goState.PhaseMask
	if goPhase == 0 {
		goPhase = 1
	}
	if goPhase&s.currentPlayerPhaseMask() == 0 {
		return true
	}

	// Player::GetGameObjectIfCanInteractWith (Player.cpp:2379): players cannot
	// interact with gameobjects that use the "Point" icon.
	if goState.IconName == "Point" {
		return true
	}

	// GameObject::Use head arms (GameObject.cpp:1491-1509): the
	// CannotBeUsedUnderImmunity gate, the mounted dismount, and
	// PlayerTalkClass->ClearMenus() all run before the gossip hooks and the
	// template-cooldown gate, for every GO type. The immunity gate reads the
	// raw UNIT_FLAG_IMMUNE flag in C++; Go models the same immunity through
	// live auras, so isTotalImmune() is the established twin (used the same
	// way by canUseBattlegroundObject).
	goTpl := s.loadGameObjectTemplateData(ctx, entry)
	if goCannotBeUsedUnderImmunity(goState.Type, goTpl) && s.isTotalImmune() {
		return true
	}
	if !goIsUsableMounted(goState.Type, goTpl) {
		s.removeAurasByType(spellAuraMounted)
	}
	s.gossip = nil

	// Eluna GOSSIP_EVENT_ON_HELLO (1) for the gameobject_gossip bindings,
	// fired at the head of GameObject::Use (GameObject.cpp:1502) after the
	// interact gates (HandleGameObjectUseOpcode, SpellHandler.cpp:300) and
	// before the type arms: a Lua false return skips the native arms, like
	// the C++ early return.
	if s.fireGameObjectGossipHelloHook(ctx, guid) {
		return true
	}

	if !s.admitGameObjectUse(entry, goState) {
		return true
	}

	switch goState.Type {
	case GameObjectTypeDoor:
		s.server.useDoorOrButton(goState.Map, goState.InstanceID, guid, false, 0)

	case GameObjectTypeButton:
		// GameObject::Use GAMEOBJECT_TYPE_BUTTON arm (GameObject.cpp:1520):
		// doors and buttons share the identical arm — UseDoorOrButton(0,
		// false, user) — so the button runs the same IN_USE gate, state
		// flip and template autoCloseTime (data2) reset as doors.
		s.server.useDoorOrButton(goState.Map, goState.InstanceID, guid, false, 0)

	case GameObjectTypeQuestGiver:
		// GameObject::Use GAMEOBJECT_TYPE_QUESTGIVER arm (GameObject.cpp:1525):
		// PrepareGossipMenu(this, questgiver.gossipID, true) + SendPreparedGossip(this).
		s.useQuestGiverGameObject(ctx, guid, entry)

	case GameObjectTypeChest:
		// GameObject::Use has no GAMEOBJECT_TYPE_CHEST arm (the
		// GameObject.cpp:1484-2087 switch skips type 3): right-clicking a
		// chest is a silent no-op in C++. Chest loot opens only through the
		// lock path — the client casts Opening and Spell::EffectOpenLock
		// answers Spell::SendLoot(guid, LOOT_SKINNING)
		// (SpellEffects.cpp:2023-2031), which carries the CanOpenLock
		// lock/skill/key gate (Spell.cpp:7793). The old arm opened loot
		// here directly, bypassing that gate for locked chests.

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
		s.useGooberGameObject(ctx, guid, entry, goState)
	case GameObjectTypeTrap:
		// GameObject::Use GAMEOBJECT_TYPE_TRAP arm (GameObject.cpp:1536-1549;
		// template layout GameObjectData.h:120-138).
		tpl := s.loadGameObjectTemplateData(ctx, entry)
		if tpl[3] != 0 { // trap.spellId (data3)
			s.castSpellDirect(ctx, tpl[3], s.playerGUID)
		}
		if tpl[4] == 1 { // type == 1: deactivate after trigger
			// GO_JUST_DEACTIVATED has no Go analog; the instance copy is
			// hidden and a despawn goes out to viewers.
			s.server.setGameObjectHiddenInInstance(goState.Map, goState.InstanceID, guid, true)
			s.server.broadcastGameObjectDespawn(goState.Map, guid)
		}
		// The C++ TRAP arm re-arms m_cooldownTime with the template cooldown
		// or a 4-second default (GameObject.cpp:1543); admitGameObjectUse only
		// re-armed when data5 != 0, so a zero-cooldown trap stayed instantly
		// reusable in Go.
		trapCD := tpl[5] // trap.cooldown (data5), seconds
		if trapCD == 0 {
			trapCD = 4
		}
		s.server.objectsMu.Lock()
		goState.CooldownUntilMS = time.Now().UnixMilli() + int64(trapCD)*1000
		s.server.objectsMu.Unlock()

	case GameObjectTypeChair:
		s.useGameObjectChair(ctx, goState, entry)

	case GameObjectTypeRitual:
		s.useSummoningRitual(ctx, goState, entry)

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
		// RemoveAurasByType(SPELL_AURA_MOUNTED) (GameObject.cpp:1918) runs
		// unconditionally in the arm — even when allowMounted (data3) kept
		// the player mounted through the Use() head's !IsUsableMounted gate.
		s.removeAurasByType(spellAuraMounted)
		// AddUse() has no Go use-count model (documented).
		if tpl[0] != 0 { // spellcaster.spellId (data0)
			// Use() tail: spellCaster defaults to the user, so the user
			// casts on themselves (GameObject.cpp:1487,2092); Go's
			// castSpellDirect casts as the player on the player — exact.
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

	case GameObjectTypeFlagStand, GameObjectTypeFlagDrop:
		// GameObject::Use GAMEOBJECT_TYPE_FLAGSTAND / FLAGDROP arms
		// (GameObject.cpp:1958-2041). Flag entries owned by the battleground
		// delegates (WSG/EotS flags, AB banners) never reach here; this
		// covers the rest (e.g. the Arathi Basin flagstand entries
		// 15001-15005).
		s.useFlagstandGameObject(ctx, guid, entry, goState)

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

// useQuestGiverGameObject mirrors the GAMEOBJECT_TYPE_QUESTGIVER arm of
// GameObject::Use (GameObject.cpp:1525-1534): PrepareGossipMenu with the
// template's gossipID (data3) and the quest list, then SendPreparedGossip —
// a zero gossipID with a non-empty quest menu sends the quest list directly
// (Player::SendPreparedGossip, Player.cpp:14542).
func (s *session) useQuestGiverGameObject(ctx context.Context, guid uint64, entry uint32) {
	if s.player == nil {
		return
	}
	tpl := s.loadGameObjectTemplateData(ctx, entry)
	gossipID := tpl[3] // questgiver.gossipID (GameObjectData.h:75)
	// PlayerTalkClass->ClearMenus() runs at the head of GameObject::Use.
	s.gossip = nil
	s.gossipClosed = false
	menu, err := s.prepareGameObjectGossip(ctx, guid, entry, gossipID, gossipID, true)
	if err != nil {
		s.debug("questgiver go gossip load failed", "account", s.accountName, "entry", entry, "error", err)
		s.gossipClosed = true
		_ = s.write(uint16(protocol.OpcodeSMSG_GOSSIP_COMPLETE), nil, true)
		return
	}
	if menu.MenuID == 0 && len(menu.Quests) > 0 {
		s.sendPreparedGameObjectQuest(ctx, guid, entry, menu.Quests)
		return
	}
	s.gossip = menu
	if err := s.sendGossipMenu(); err != nil {
		s.debug("questgiver go gossip response failed", "account", s.accountName, "entry", entry, "error", err)
	}
}

// sameRaidGroup mirrors Player::IsInSameRaidWith (Player.cpp:2543): the same
// player, or two players in the same non-zero group.
func sameRaidGroup(a, b *session) bool {
	if a == nil || b == nil {
		return false
	}
	if a == b {
		return true
	}
	return a.groupID != 0 && a.groupID == b.groupID
}

// useSummoningRitual mirrors the GAMEOBJECT_TYPE_SUMMONING_RITUAL arm of
// GameObject::Use (GameObject.cpp:1803-1904; template layout
// GameObjectData.h:236-257: data0 reqParticipants, data1 spellId, data2
// animSpell, data3 ritualPersistent, data4 casterTargetSpell, data5
// casterTargetSpellTargets, data6 castersGrouped). A zero reqParticipants
// template means the ritual can never complete, like C++ (the count check
// can never match).
func (s *session) useSummoningRitual(ctx context.Context, goState *dynamicGameObjectState, entry uint32) {
	if s == nil || s.server == nil || s.player == nil || goState == nil {
		return
	}
	tpl := s.loadGameObjectTemplateData(ctx, entry)
	reqParticipants, ritualSpell, animSpell := tpl[0], tpl[1], tpl[2]
	persistent, casterTargetSpell, casterTargetTargets, castersGrouped := tpl[3], tpl[4], tpl[5], tpl[6]
	if reqParticipants == 0 {
		return
	}
	srv := s.server
	var ownerSess *session
	if goState.OwnerGUID != 0 {
		ownerSess = srv.playerSessionForGUID(goState.OwnerGUID)
	}
	srv.objectsMu.Lock()
	// Ritual owner is set for GOs without an owner (not summoned); the first
	// user becomes the owner (GameObject.cpp:1816-1820).
	if goState.RitualOwnerGUID == 0 && ownerSess == nil {
		goState.RitualOwnerGUID = s.playerGUID
	}
	ritualOwnerGUID := goState.RitualOwnerGUID
	srv.objectsMu.Unlock()
	var ritualOwnerSess *session
	if ritualOwnerGUID != 0 {
		ritualOwnerSess = srv.playerSessionForGUID(ritualOwnerGUID)
	}
	casterSess := s
	if ownerSess != nil && ownerSess.player != nil {
		// Accept only use by a player from the same group as the owner,
		// excluding the owner itself (its unique use was added by the
		// summoning spell effect); the owner must still be channeling.
		if ownerSess == s {
			return
		}
		if castersGrouped != 0 && !sameRaidGroup(s, ownerSess) {
			return
		}
		if ownerSess.activeChannel == nil {
			return
		}
		casterSess = ownerSess
	} else {
		if ritualOwnerSess != s && castersGrouped != 0 && !sameRaidGroup(s, ritualOwnerSess) {
			return
		}
	}
	// GameObject::AddUniqueUse (GameObject.cpp:897): the unique-user set, not
	// the raw use count, gates completion.
	srv.objectsMu.Lock()
	if goState.RitualUsers == nil {
		goState.RitualUsers = make(map[uint64]struct{})
	}
	goState.RitualUsers[s.playerGUID] = struct{}{}
	uniqueCount := uint32(len(goState.RitualUsers))
	var targetUsers []uint64
	if casterTargetSpell != 0 && casterTargetSpell != 1 {
		targetUsers = make([]uint64, 0, len(goState.RitualUsers))
		for u := range goState.RitualUsers {
			targetUsers = append(targetUsers, u)
		}
	}
	srv.objectsMu.Unlock()
	if animSpell != 0 {
		// CastSpell(player, animSpell, true): triggered, like Go's
		// castSpellDirect, which is triggered by convention.
		s.castSpellDirect(ctx, animSpell, s.playerGUID)
	}
	if uniqueCount != reqParticipants {
		return
	}
	if ritualOwnerSess != nil {
		casterSess = ritualOwnerSess
	}
	spellID := ritualSpell
	if spellID == 62330 {
		// The GO stores a nonexistent spell; replaced by the expected one,
		// always triggered (GameObject.cpp:1856-1862).
		spellID = 61993
	}
	for i := uint32(0); i < casterTargetTargets && len(targetUsers) > 0; i++ {
		// casterTargetSpell is cast at a random GO user (GameObject.cpp:1867-1871).
		casterSess.castSpellDirect(ctx, casterTargetSpell, targetUsers[rand.Intn(len(targetUsers))])
	}
	if ownerSess != nil {
		// Finish the owner's spell (GameObject.cpp:1874).
		ownerSess.finishChannel()
	}
	if persistent == 0 {
		// SetLootState(GO_JUST_DEACTIVATED): the Update arm deletes a
		// summoned ritual (GameObject.cpp GO_JUST_DEACTIVATED arm).
		if goState.IsRuntimeSpawn {
			srv.despawnDynamicGameObjectInInstance(goState.Map, goState.InstanceID, goState.GUID)
		} else {
			srv.setGameObjectHiddenInInstance(goState.Map, goState.InstanceID, goState.GUID, true)
			srv.broadcastGameObjectDespawn(goState.Map, goState.GUID)
		}
	} else {
		// Reset the ritual for the next round (GameObject.cpp:1879-1884).
		srv.objectsMu.Lock()
		goState.RitualOwnerGUID = 0
		goState.RitualUsers = nil
		srv.objectsMu.Unlock()
	}
	if spellID != 0 {
		// spellCaster->CastSpell(user, spellId, triggered): the completing
		// player is the target. Go's castSpellDirect is triggered by
		// convention, so the animSpell/61993 triggered arms and the plain
		// final cast all go out the same way (documented).
		casterSess.castSpellDirect(ctx, spellID, s.playerGUID)
	}
}

// useFlagstandGameObject mirrors the GAMEOBJECT_TYPE_FLAGSTAND and
// GAMEOBJECT_TYPE_FLAGDROP arms of GameObject::Use (GameObject.cpp:1958-2041).
// The use runs the Player::CanUseBattlegroundObject gate
// (Player.cpp:24691-24708: GO faction friendliness, no total damage immunity,
// no recently-dropped-flag debuff, player alive), then the vehicle gate, then
// strips stealth/invisibility auras, then dispatches to the battleground's
// EventPlayerClickedOnFlag. C++ resolves the AB node by proximity
// (BattlegroundAB.cpp:418-430), which the flagstand entries 15001-15005 rely
// on since the banner delegates only intercept the 180087-180119 entries.
func (s *session) useFlagstandGameObject(ctx context.Context, guid uint64, entry uint32, goState *dynamicGameObjectState) {
	if s == nil || s.server == nil || s.player == nil || goState == nil {
		return
	}
	if !s.canUseBattlegroundObject(guid) {
		return
	}
	// player->GetVehicle() (GameObject.cpp:1967, 2008).
	if s.player.VehicleGUID != 0 {
		return
	}
	// RemoveAurasByType(SPELL_AURA_MOD_STEALTH / SPELL_AURA_MOD_INVISIBILITY)
	// (GameObject.cpp:1973-1974, 2013-2014).
	s.removeAurasByType(spellAuraStealth)
	s.removeAurasByType(spellAuraInvisibility)

	// Battleground* bg = player->GetBattleground(): Go dispatches on the BG
	// map the player stands on. Only Arathi Basin has flagstand entries
	// outside the delegates' lists; other maps' flag entries are all
	// intercepted above, so a non-AB flagstand here is a silent no-op.
	if s.player.Map != ABMapID {
		return
	}
	ab := s.server.getOrCreateABState(s.player.Map)
	nodeID, ok := abNodeIDNearPlayer(ab, s)
	if !ok {
		return
	}
	s.server.clickABNode(ab, nodeID, s)
}

// handleGameObjectReportUse processes CMSG_GAMEOBJ_REPORT_USE (0x481).
// Reference: WorldSession::HandleGameobjectReportUse (SpellHandler.cpp:318).
// This is the opcode that carries ACHIEVEMENT_CRITERIA_TYPE_USE_GAMEOBJECT in
// C++; CMSG_GAMEOBJ_USE (HandleGameObjectUseOpcode) never updates it.
func (s *session) handleGameObjectReportUse(ctx context.Context, payload []byte) bool {
	// STATUS_LOGGEDIN dispatch never reaches the handler without a player
	// (WorldSession.cpp:305-330: requeue/skip, conn open), and truncated
	// reads throw ByteBufferException -> per-packet drop (WorldSession.cpp:425-432)
	// — conn stays open in both cases, so drop (true), never close (false).
	if !s.playerLoaded || s.player == nil || len(payload) == 0 {
		return true
	}
	guid, err := readObjectGUID(payload)
	if err != nil || guid == 0 {
		return true
	}
	// Player::GetGameObjectIfCanInteractWith (Player.cpp:2371): a player in
	// flight cannot interact with gameobjects — the gate fires inside
	// HandleGameobjectReportUse (SpellHandler.cpp:318) before the
	// achievement-criteria update.
	if s.isInFlight() {
		return true
	}
	// Player::GetGameObjectIfCanInteractWith (Player.cpp:2366): !IsInWorld()
	// silently ignores the opcode (see the CMSG_GAMEOBJ_USE arm).
	if !s.worldReady.Load() {
		return true
	}
	entry := uint32((guid >> 24) & 0x00FFFFFF)
	lowGUID := uint32(guid & 0x00FFFFFF)
	goState, err := s.server.getOrLoadGameObjectState(ctx, guid, lowGUID, entry, s.player.Map, s.player.InstanceID)
	if err != nil || goState == nil {
		// HandleGameobjectReportUse (SpellHandler.cpp:318): a null
		// GetGameObjectIfCanInteractWith result silently returns — same
		// stale-GUID disconnect fix as the CMSG_GAMEOBJ_USE arm.
		return true
	}
	// The GetGameObjectIfCanInteractWith gates (Player.cpp:2363): the "Point"
	// icon, InSamePhase and interaction range gates apply here as in C++.
	if goState.IconName == "Point" {
		return true
	}
	goPhase := goState.PhaseMask
	if goPhase == 0 {
		goPhase = 1
	}
	if goPhase&s.currentPlayerPhaseMask() == 0 {
		return true
	}
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > gameObjectInteractionDistance(goState.Type) {
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

// loadGameObjectTemplateData fetches gameobject_template data0..data23 for the
// GameObject::Use arms that read type-specific template fields
// (GameObjectData.h:110-372); the wide tail covers goober (noDamageImmune at
// data11, allowMounted at data17) and questgiver.allowMounted (data8).
func (s *session) loadGameObjectTemplateData(ctx context.Context, entry uint32) [24]uint32 {
	var data [24]uint32
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return data
	}
	var d [24]int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(Data0, 0), COALESCE(Data1, 0), COALESCE(Data2, 0), COALESCE(Data3, 0),
			COALESCE(Data4, 0), COALESCE(Data5, 0), COALESCE(Data6, 0), COALESCE(Data7, 0),
			COALESCE(Data8, 0), COALESCE(Data9, 0), COALESCE(Data10, 0), COALESCE(Data11, 0),
			COALESCE(Data12, 0), COALESCE(Data13, 0), COALESCE(Data14, 0), COALESCE(Data15, 0),
			COALESCE(Data16, 0), COALESCE(Data17, 0), COALESCE(Data18, 0), COALESCE(Data19, 0),
			COALESCE(Data20, 0), COALESCE(Data21, 0), COALESCE(Data22, 0), COALESCE(Data23, 0)
		FROM gameobject_template WHERE entry = ? LIMIT 1`, entry).
		Scan(&d[0], &d[1], &d[2], &d[3], &d[4], &d[5], &d[6], &d[7],
			&d[8], &d[9], &d[10], &d[11], &d[12], &d[13], &d[14], &d[15],
			&d[16], &d[17], &d[18], &d[19], &d[20], &d[21], &d[22], &d[23])
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

	var goMap, goState, goType, displayID, data1, phaseMask int64
	var goX, goY, goZ, goO, size float64
	var iconName string
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, g.orientation, g.state,
		t.type, t.displayId, t.size, COALESCE(t.data1, 0), COALESCE(t.IconName, ''), COALESCE(g.phaseMask, 0)
		FROM gameobject AS g
		JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.guid = ? AND g.id = ? LIMIT 1`, lowGUID, entry).Scan(&goMap, &goX, &goY, &goZ, &goO, &goState, &goType, &displayID, &size, &data1, &iconName, &phaseMask)
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
		SpawnState:  uint8(goState),
		Type:        uint8(goType),
		DisplayID:   uint32(displayID),
		Size:        float32(size),
		IconName:    iconName,
		PhaseMask:   uint32(phaseMask),
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
	// m_prevGoState twin: the spawn state is captured once at creation
	// (GameObject.cpp:349), never per-use.
	dyn.SpawnState = dyn.State
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

// handleEffectTransmitted mirrors Spell::EffectTransmitted
// (SpellEffects.cpp:4927-5045): the generic (non-fishing, non-ritual,
// non-duel-arbiter) arm spawns the effect's MiscValue gameobject entry as a
// runtime object owned by the caster, despawning after the spell's DBC
// duration. The fishing-node arm is already bridged (spawnFishingBobber,
// called before the effect dispatch); the summoning-ritual AddUniqueUse /
// AddGameObject cancel linkage and the linked-trap spawn have no Go model
// and stay documented. Position: C++ prefers the spell dest target, then a
// radius/random-range GetClosePoint; Go uses the dest target when present
// and the caster position otherwise (the random-range arm is unbridged).
func (s *session) handleEffectTransmitted(ctx context.Context, target protocol.SpellTargetData, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	entry := uint32(0)
	if eff.MiscValue > 0 {
		entry = uint32(eff.MiscValue)
	}
	if entry == 0 {
		return
	}
	tmpl, err := gobjectTemplateByEntry(ctx, s.server.WorldStore.DB, entry)
	if err != nil || tmpl == nil {
		return
	}
	if tmpl.goType == GameObjectTypeFishingNode {
		return
	}
	x, y, z := s.player.X, s.player.Y, s.player.Z
	if tmpl.goType != GameObjectTypeRitual && target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		x, y, z = target.Destination.X, target.Destination.Y, target.Destination.Z
	}
	lowGUID := s.server.nextDynamicGameObjectLowGUID()
	newGUID := gameObjectGUID(lowGUID, entry)
	size := tmpl.size
	if size <= 0 {
		size = 1.0
	}
	dyn := &dynamicGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: entry, OwnerGUID: s.playerGUID, SpellID: spell.ID, Map: s.player.Map, InstanceID: s.player.InstanceID, X: x, Y: y, Z: z, Orientation: s.player.Orientation, State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, IsRuntimeSpawn: true}
	// SpellEffects.cpp:5019: the object's respawn time is the spell duration;
	// a runtime spawn with a positive duration despawns when it elapses.
	if spell.DurationIndex > 0 && s.server.Data != nil {
		lvl := s.player.Level
		if lvl == 0 {
			lvl = 80
		}
		if dur, ok, derr := s.server.Data.SpellDuration(spell.DurationIndex, uint32(lvl)); derr == nil && ok && dur > 0 {
			dyn.DespawnTimer = time.AfterFunc(time.Duration(dur)*time.Millisecond, func() {
				s.server.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
			})
		}
	}
	s.sendSummonObjectLog(spell.ID, spellEffectTransmitted, newGUID)
	s.server.spawnDynamicGameObject(dyn)
}

// creatureEffectTransmitted mirrors Spell::EffectTransmitted
// (SpellEffects.cpp:4927-5045) for a creature caster: the generic arm
// spawns the effect's MiscValue gameobject entry as a runtime object owned
// by the caster creature, despawning after the spell's DBC duration. The
// fishing-node, summoning-ritual and linked-trap arms have no Go model and
// stay documented, matching the player-path bridge (handleEffectTransmitted
// above). Position: C++ prefers the spell dest target, then a
// radius/random-range GetClosePoint; Go uses the dest target when present
// and the creature position otherwise (the random-range arm is unbridged).
func (s *Server) creatureEffectTransmitted(ctx context.Context, m *creatureMotion, target protocol.SpellTargetData, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if s == nil || m == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	entry := uint32(0)
	if eff.MiscValue > 0 {
		entry = uint32(eff.MiscValue)
	}
	if entry == 0 {
		return
	}
	tmpl, err := gobjectTemplateByEntry(ctx, s.WorldStore.DB, entry)
	if err != nil || tmpl == nil {
		return
	}
	if tmpl.goType == GameObjectTypeFishingNode {
		return
	}
	x, y, z := m.X, m.Y, m.Z
	if tmpl.goType != GameObjectTypeRitual && target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		x, y, z = target.Destination.X, target.Destination.Y, target.Destination.Z
	}
	lowGUID := s.nextDynamicGameObjectLowGUID()
	newGUID := gameObjectGUID(lowGUID, entry)
	size := tmpl.size
	if size <= 0 {
		size = 1.0
	}
	dyn := &dynamicGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: entry, OwnerGUID: m.GUID, SpellID: spell.ID, Map: m.Map, InstanceID: m.InstanceID, X: x, Y: y, Z: z, Orientation: m.Orientation, State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, IsRuntimeSpawn: true}
	// SpellEffects.cpp:5019: the object's respawn time is the spell duration;
	// a runtime spawn with a positive duration despawns when it elapses.
	if spell.DurationIndex > 0 && s.Data != nil {
		lvl := m.Level
		if lvl == 0 {
			lvl = 80
		}
		if dur, ok, derr := s.Data.SpellDuration(spell.DurationIndex, uint32(lvl)); derr == nil && ok && dur > 0 {
			dyn.DespawnTimer = time.AfterFunc(time.Duration(dur)*time.Millisecond, func() {
				s.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
			})
		}
	}
	s.spawnDynamicGameObject(dyn)
	// The summon-object spell log carries the caster creature's GUID,
	// broadcast to the instance (the player-path bridge writes it to the
	// caster session and nearby players instead).
	log := protocol.NewBuffer(32)
	log.WritePackedGUID(m.GUID)
	log.WriteU32(spell.ID)
	log.WriteU32(1)
	log.WriteU32(spellEffectTransmitted)
	log.WriteU32(1)
	log.WritePackedGUID(newGUID)
	s.broadcastToInstance(m.Map, m.InstanceID, uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), nil)
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
