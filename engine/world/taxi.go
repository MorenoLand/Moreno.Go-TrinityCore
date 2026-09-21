package world

import (
	"context"
	"database/sql"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	unitNPCFlagFlightmaster uint32 = 0x00002000
	playerExtraTaxiCheat    uint32 = 0x00000008
	taxiMaskSize                   = 14
)

// TrinityCore Player.h extra flags and transient PLAYER_FLAGS bits.
const (
	playerExtraGMOn            uint32 = 0x00000001
	playerExtraGMInvisible     uint32 = 0x00000010
	playerExtraGMChat          uint32 = 0x00000020
	playerFlagAFK              uint32 = 0x00000002
	playerFlagDND              uint32 = 0x00000004
	playerFlagGM               uint32 = 0x00000008
	playerFlagAllowOnlyAbility uint32 = 0x00000001
)

// setTaxiMaskNode mirrors PlayerTaxi::SetTaximaskNode; returns true when the
// bit transitioned from unknown to known.
func (s *session) setTaxiMaskNode(node uint32) bool {
	if s.player == nil || node == 0 {
		return false
	}
	field := (node - 1) / 32
	if field >= taxiMaskSize {
		return false
	}
	bit := uint32(1) << ((node - 1) % 32)
	if s.player.TaxiMask[field]&bit != 0 {
		return false
	}
	s.player.TaxiMask[field] |= bit
	return true
}

func (s *session) isTaxiMaskNodeKnown(node uint32) bool {
	if s.player == nil || node == 0 {
		return false
	}
	field := (node - 1) / 32
	if field >= taxiMaskSize {
		return false
	}
	return s.player.TaxiMask[field]&(1<<((node-1)%32)) != 0
}

// isTaxiCheater mirrors Player::isTaxiCheater (PLAYER_EXTRA_TAXICHEAT).
func (s *session) isTaxiCheater() bool {
	return s.player != nil && s.player.ExtraFlags&playerExtraTaxiCheat != 0
}

// saveTaxiMask persists the mask in TrinityCore's space separated text form.
func (s *session) saveTaxiMask(ctx context.Context) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil {
		return
	}
	parts := make([]string, taxiMaskSize)
	for i, v := range s.player.TaxiMask {
		parts[i] = strconv.FormatUint(uint64(v), 10)
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET taximask = ? WHERE guid = ?", strings.Join(parts, " "), s.playerGUID)
}

// loadTaxiMask mirrors PlayerTaxi::LoadTaxiMask (space separated words).
func (s *session) loadTaxiMask(raw sql.NullString) {
	if s == nil || s.player == nil || !raw.Valid {
		return
	}
	tokens := strings.Fields(raw.String)
	for i := 0; i < taxiMaskSize && i < len(tokens); i++ {
		if v, err := strconv.ParseUint(tokens[i], 10, 32); err == nil {
			s.player.TaxiMask[i] = uint32(v)
		} else {
			s.player.TaxiMask[i] = 0
		}
	}
}

// nearestCreatureTaxiNode finds the flight node nearest to the creature, as
// ObjectMgr::GetNearestTaxiNode does using the unit's world position.
func (s *session) nearestCreatureTaxiNode(ctx context.Context, guid uint64) (uint32, bool) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0, false
	}
	if !s.canInteractWithNPC(ctx, guid, uint64(unitNPCFlagFlightmaster)) {
		return 0, false
	}
	creature := s.luaCreature(ctx, guid)
	if creature == nil {
		return 0, false
	}
	if objectUint32OrZero(creature, "NPCFlags")&unitNPCFlagFlightmaster == 0 {
		return 0, false
	}
	mapID := objectUint32OrZero(creature, "Map")
	x, okX := objectFloat32Field(creature, "X")
	y, okY := objectFloat32Field(creature, "Y")
	z, okZ := objectFloat32Field(creature, "Z")
	if !okX || !okY || !okZ || s.server.Data == nil {
		return 0, false
	}
	node, err := s.server.Data.NearestTaxiNode(x, y, z, mapID, s.playerAlliance())
	if err != nil {
		return 0, false
	}
	return node, node != 0
}

// playerAlliance reports the player team like Player::GetTeam.
func (s *session) playerAlliance() bool {
	if s.player == nil {
		return false
	}
	switch s.player.Race {
	case 1, 3, 4, 7, 11: // Human, Dwarf, NightElf, Gnome, Draenei
		return true
	}
	return false
}

func (s *session) handleTaxiNodeStatusQuery(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	// TrinityCore SendTaxiStatus: ignore non-flightmasters and units with no
	// resolvable node instead of answering "known" for everything.
	node, ok := s.nearestCreatureTaxiNode(ctx, guid)
	if !ok {
		return true
	}
	known := uint8(0)
	if s.isTaxiMaskNodeKnown(node) || s.isTaxiCheater() {
		known = 1
	}
	packet := protocol.NewBuffer(9)
	packet.WriteU64(guid)
	packet.WriteU8(known)
	_ = s.write(uint16(protocol.OpcodeSMSG_TAXINODE_STATUS), packet.Bytes(), true)
	return true
}

func (s *session) sendTaxiNodeStatusMultiple(ctx context.Context) bool {
	if s == nil || s.server == nil || s.player == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.Data == nil {
		return true
	}
	distance := float64(s.server.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0)
		FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (COALESCE(t.npcflag, 0) & ?) <> 0`, s.player.Map, float64(s.player.X)-distance, float64(s.player.X)+distance, float64(s.player.Y)-distance, float64(s.player.Y)+distance, unitNPCFlagFlightmaster)
	if err != nil {
		return true
	}
	type flightmaster struct {
		guid, entry uint32
		x, y, z     float32
		faction     uint32
	}
	masters := make([]flightmaster, 0)
	for rows.Next() {
		var guid, entry, faction int64
		var x, y, z float64
		if rows.Scan(&guid, &entry, &x, &y, &z, &faction) == nil {
			masters = append(masters, flightmaster{guid: uint32(guid), entry: uint32(entry), x: float32(x), y: float32(y), z: float32(z), faction: uint32(faction)})
		}
	}
	rows.Close()
	if rows.Err() != nil {
		return false
	}
	player := playerPos{Map: s.player.Map, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	for _, master := range masters {
		if s.server.isHostileFaction(master.faction, player) {
			continue
		}
		node, nodeErr := s.server.Data.NearestTaxiNode(master.x, master.y, master.z, s.player.Map, s.playerAlliance())
		if nodeErr != nil || node == 0 {
			continue
		}
		packet := protocol.NewBuffer(9)
		packet.WriteU64(creatureWorldGUID(master.guid, master.entry))
		if s.isTaxiMaskNodeKnown(node) || s.isTaxiCheater() {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		if err := s.write(uint16(protocol.OpcodeSMSG_TAXINODE_STATUS), packet.Bytes(), true); err != nil {
			return false
		}
	}
	return true
}

func (s *session) handleTaxiQueryAvailableNodes(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	// HandleTaxiQueryAvailableNodes: learn unknown node, otherwise show the
	// flight map for known nodes.
	if s.learnNewTaxiNode(ctx, guid) {
		return true
	}
	return s.sendTaxiMenu(ctx, guid)
}

// learnNewTaxiNode mirrors WorldSession::SendLearnNewTaxiNode: discovers the
// nearest node, stores it and notifies the client; true when learned.
func (s *session) learnNewTaxiNode(ctx context.Context, guid uint64) bool {
	node, ok := s.nearestCreatureTaxiNode(ctx, guid)
	if !ok {
		return true // avoid a second failing node search in SendTaxiMenu
	}
	if !s.isTaxiMaskNodeKnown(node) && s.setTaxiMaskNode(node) {
		s.saveTaxiMask(ctx)
		_ = s.write(uint16(protocol.OpcodeSMSG_NEW_TAXI_PATH), nil, true)
		status := protocol.NewBuffer(9)
		status.WriteU64(guid)
		status.WriteU8(1)
		_ = s.write(uint16(protocol.OpcodeSMSG_TAXINODE_STATUS), status.Bytes(), true)
		return true
	}
	return false
}

func (s *session) sendTaxiMenu(ctx context.Context, flightMasterGUID uint64) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}
	creature := s.luaCreature(ctx, flightMasterGUID)
	if creature == nil {
		return true
	}
	if objectUint32OrZero(creature, "NPCFlags")&unitNPCFlagFlightmaster == 0 {
		return true
	}
	mapID := objectUint32OrZero(creature, "Map")
	x, _ := objectFloat32Field(creature, "X")
	y, _ := objectFloat32Field(creature, "Y")
	z, _ := objectFloat32Field(creature, "Z")
	curNode := uint32(0)
	if s.server.Data != nil {
		if node, err := s.server.Data.NearestTaxiNode(x, y, z, mapID, s.playerAlliance()); err == nil {
			curNode = node
		}
	}
	if curNode == 0 {
		return true
	}
	// SendTaxiMenu never announces SMSG_NEW_TAXI_PATH; the map carries the
	// player's known nodes (or the whole network for taxi cheaters).
	packet := protocol.NewBuffer(4 + 8 + 4 + 8*taxiMaskSize)
	packet.WriteU32(1)
	packet.WriteU64(flightMasterGUID)
	packet.WriteU32(curNode)
	cheater := s.isTaxiCheater()
	networkMask := [taxiMaskSize]uint32{}
	if s.server.Data != nil {
		networkMask, _ = s.server.Data.TaxiNetworkMask()
	}
	for i := 0; i < taxiMaskSize; i++ {
		if cheater {
			packet.WriteU32(networkMask[i])
		} else {
			packet.WriteU32(s.player.TaxiMask[i])
		}
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_SHOWTAXINODES), packet.Bytes(), true)
	s.debug("taxi menu sent", "account", s.accountName, "master", flightMasterGUID, "node", curNode)
	return true
}

// ActivateTaxiReply codes from TrinityCore SharedDefines.h.
const (
	taxiErrOK             uint32 = 0
	taxiErrUnspecified    uint32 = 1
	taxiErrNoSuchPath     uint32 = 2
	taxiErrNotEnoughMoney uint32 = 3
	taxiErrTooFarAway     uint32 = 4
	taxiErrNoVendorNearby uint32 = 5
	taxiErrNotVisited     uint32 = 6
)

func (s *session) handleActivateTaxi(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	reply := func(code uint32) bool {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(code)
		_ = s.write(uint16(protocol.OpcodeSMSG_ACTIVATETAXIREPLY), packet.Bytes(), true)
		return true
	}

	var sourceNode, destNode uint32
	if len(payload) >= 20 {
		// CMSG_ACTIVATETAXIEXPRESS: guid (8), nodeCount (4), nodes... (nodeCount * 4)
		nodeCount, err := reader.ReadU32()
		if err != nil || nodeCount < 2 || len(payload) < int(12+nodeCount*4) {
			return reply(taxiErrNoSuchPath)
		}
		var nodes []uint32
		for i := uint32(0); i < nodeCount; i++ {
			node, err := reader.ReadU32()
			if err != nil {
				return reply(taxiErrNoSuchPath)
			}
			if !s.isTaxiMaskNodeKnown(node) && !s.isTaxiCheater() {
				return reply(taxiErrNotVisited)
			}
			nodes = append(nodes, node)
		}
		sourceNode = nodes[0]
		destNode = nodes[len(nodes)-1]
	} else {
		// CMSG_ACTIVATETAXI: guid (8), source (4), dest (4)
		sourceNode, err = reader.ReadU32()
		if err != nil {
			return false
		}
		destNode, err = reader.ReadU32()
		if err != nil {
			return false
		}
		if !s.isTaxiMaskNodeKnown(sourceNode) && !s.isTaxiCheater() {
			return reply(taxiErrNotVisited)
		}
	}

	// Player::ActivateTaxiPathTo validation order: vendor/nearest node,
	// known source node, existing path, money.
	nearest, ok := s.nearestCreatureTaxiNode(ctx, guid)
	if !ok {
		return reply(taxiErrNoVendorNearby)
	}
	if s.server.Data == nil {
		return reply(taxiErrUnspecified)
	}
	pathID, price, found, err := s.server.Data.TaxiPathLinks(sourceNode, destNode)
	if err != nil || !found {
		return reply(taxiErrNoSuchPath)
	}
	if uint64(s.player.Money) < uint64(price) {
		return reply(taxiErrNotEnoughMoney)
	}
	if price > 0 {
		s.player.Money -= price
		s.updateAchievementCriteria(criteriaTypeGoldSpentTravel, 0, uint32(price))
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		}
		s.sendPlayerMoneyUpdate()
	}
	// SendDoFlight: mount and run the TaxiPathNode spline as a flying
	// monster move (Flying 0x2000 | Catmullrom 0x40000 per MoveSplineFlag).
	mount := uint32(0)
	if mountDisplay, err := s.server.Data.TaxiNodeMount(sourceNode, s.playerAlliance()); err == nil {
		mount = mountDisplay
	}
	s.startTaxiFlight(pathID, mount, nearest == sourceNode)
	s.updateAchievementCriteria(criteriaTypeFlightPathsTaken, 0, 1)
	s.debug("taxi flight activated", "account", s.accountName, "master", guid, "source", sourceNode, "dest", destNode, "cost", price)
	return reply(taxiErrOK)
}

func taxiResumeStartNode(points []wotlk.TaxiSplinePoint, mapID uint32, x, y, z float32) int {
	if len(points) < 2 {
		return 0
	}
	distNext := float64((points[0].X-x)*(points[0].X-x) + (points[0].Y-y)*(points[0].Y-y) + (points[0].Z-z)*(points[0].Z-z))
	for index := 1; index < len(points); index++ {
		point, previous := points[index], points[index-1]
		if point.MapID != int32(mapID) {
			continue
		}
		distPrevious := distNext
		distNext = float64((point.X-x)*(point.X-x) + (point.Y-y)*(point.Y-y) + (point.Z-z)*(point.Z-z))
		distNodes := float64((point.X-previous.X)*(point.X-previous.X) + (point.Y-previous.Y)*(point.Y-previous.Y) + (point.Z-previous.Z)*(point.Z-previous.Z))
		if distNext+distPrevious < distNodes {
			return index
		}
	}
	return 0
}

func (s *session) continueTaxiFlight() {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return
	}
	nodes := strings.Fields(s.player.TaxiPath)
	if len(nodes) < 3 {
		return
	}
	source, sourceErr := strconv.ParseUint(nodes[1], 10, 32)
	destination, destinationErr := strconv.ParseUint(nodes[2], 10, 32)
	if sourceErr != nil || destinationErr != nil {
		return
	}
	pathID, _, found, err := s.server.Data.TaxiPathLinks(uint32(source), uint32(destination))
	if err != nil || !found {
		return
	}
	mount, err := s.server.Data.TaxiNodeMount(uint32(source), s.playerAlliance())
	if err != nil || mount == 0 {
		return
	}
	points, err := s.server.Data.TaxiPathPoints(pathID)
	if err != nil || len(points) == 0 {
		return
	}
	startNode := taxiResumeStartNode(points, s.player.Map, s.player.X, s.player.Y, s.player.Z)
	s.startTaxiFlightFrom(pathID, mount, startNode)
}

// startTaxiFlight mounts the player and broadcasts the taxi spline.
func (s *session) startTaxiFlight(pathID, mountDisplay uint32, takeoff bool) {
	s.startTaxiFlightFrom(pathID, mountDisplay, 0)
}

func (s *session) startTaxiFlightFrom(pathID, mountDisplay uint32, startNode int) {
	points, err := s.server.Data.TaxiPathPoints(pathID)
	if err != nil || len(points) == 0 {
		return
	}
	if startNode > 0 && startNode < len(points) {
		points = points[startNode:]
	}
	if mountDisplay != 0 {
		s.player.MountDisplayID = mountDisplay
		s.sendPlayerMountUpdate()
	}
	packet := protocol.NewBuffer(32 + len(points)*12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU8(0) // MOVEMENTFLAG2_UNK7
	packet.WriteF32(s.player.X)
	packet.WriteF32(s.player.Y)
	packet.WriteF32(s.player.Z)
	packet.WriteU32(uint32(time.Now().UnixMilli())) // SplineID
	packet.WriteU8(0)                               // MonsterMoveNormal
	packet.WriteU32(0x00042000)                     // Flying | Catmullrom
	total := float64(0)
	prevX, prevY, prevZ := s.player.X, s.player.Y, s.player.Z
	for _, point := range points {
		total += math.Sqrt(float64((point.X-prevX)*(point.X-prevX) + (point.Y-prevY)*(point.Y-prevY) + (point.Z-prevZ)*(point.Z-prevZ)))
		prevX, prevY, prevZ = point.X, point.Y, point.Z
	}
	// Intercontinental flight speed baseline; the client scales the spline
	// by the provided duration.
	speed := 28.0
	duration := uint32((total / speed) * 1000)
	if duration < 500 {
		duration = 500
	}
	packet.WriteU32(duration)
	packet.WriteU32(uint32(len(points)))
	for _, point := range points {
		packet.WriteF32(point.X)
		packet.WriteF32(point.Y)
		packet.WriteF32(point.Z)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_MONSTER_MOVE), packet.Bytes(), true)
	last := points[len(points)-1]
	s.player.X, s.player.Y, s.player.Z = last.X, last.Y, last.Z
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(), "UPDATE characters SET position_x = ?, position_y = ?, position_z = ? WHERE guid = ?", last.X, last.Y, last.Z, s.playerGUID)
	}
	s.inFlight = true
	// Dismount when the flight window elapses.
	time.AfterFunc(time.Duration(duration)*time.Millisecond, func() {
		s.inFlight = false
		if s.currentPlayer() != nil {
			s.player.MountDisplayID = 0
			s.sendPlayerMountUpdate()
		}
	})
}

// isInFlight mirrors Unit::IsInFlight (UNIT_STATE_IN_FLIGHT).
func (s *session) isInFlight() bool {
	return s.inFlight
}

// finishTaxiFlight mirrors Player::FinishTaxiFlight.
func (s *session) finishTaxiFlight() {
	if !s.inFlight {
		return
	}
	s.inFlight = false
	if s.currentPlayer() != nil && s.player.MountDisplayID != 0 {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
	}
}

// initTaxiNodesForLevel mirrors PlayerTaxi::InitTaxiNodesForLevel for the
// race/team starting nodes available without discovering them first.
func (s *session) initTaxiNodesForLevel() {
	if s.player == nil {
		return
	}
	if s.player.Class == 6 && s.server != nil && s.server.Data != nil {
		if nodes, err := s.server.Data.OldContinentsTaxiNodes(); err == nil {
			for _, node := range nodes {
				s.setTaxiMaskNode(node)
			}
		}
	}
	// Race specific initial known nodes: capital and taxi hub.
	switch s.player.Race {
	case 1: // Human
		s.setTaxiMaskNode(2)
	case 3: // Dwarf
		s.setTaxiMaskNode(6)
	case 4: // Night Elf
		s.setTaxiMaskNode(26)
		s.setTaxiMaskNode(27)
	case 7: // Gnome
		s.setTaxiMaskNode(6)
	case 11: // Draenei
		s.setTaxiMaskNode(94)
	case 2: // Orc
		s.setTaxiMaskNode(23)
	case 5: // Undead
		s.setTaxiMaskNode(11)
	case 6: // Tauren
		s.setTaxiMaskNode(22)
	case 8: // Troll
		s.setTaxiMaskNode(23)
	case 10: // Blood Elf
		s.setTaxiMaskNode(82)
	}
	// New continent starting node (Dalaran area by team).
	if s.playerAlliance() {
		s.setTaxiMaskNode(100)
	} else {
		s.setTaxiMaskNode(99)
	}
	if s.player.Level >= 68 {
		s.setTaxiMaskNode(213)
	}
}

// objectFloat32Field reads a float object field set by luaCreature.
func objectFloat32Field(object *scripting.Object, name string) (float32, bool) {
	value, ok := object.Fields[name]
	if !ok {
		return 0, false
	}
	switch value := value.(type) {
	case float32:
		return value, true
	case float64:
		return float32(value), true
	case uint32:
		return float32(value), true
	case int64:
		return float32(value), true
	default:
		return 0, false
	}
}

// handleEnableTaxi processes CMSG_ENABLETAXI (0x493).
func (s *session) handleEnableTaxi(ctx context.Context, payload []byte) bool {
	return s.handleTaxiNodeStatusQuery(ctx, payload)
}

// handleSetTaxiBenchmarkMode processes CMSG_SET_TAXI_BENCHMARK_MODE (0x389).
// Reference: WorldSession::HandleSetTaxiBenchmarkOpcode (MiscHandler.cpp:1403).
func (s *session) handleSetTaxiBenchmarkMode(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	mode := payload[0]
	const playerFlagTaxiBenchmark uint32 = 0x04000000
	if mode != 0 {
		s.player.PlayerFlags |= playerFlagTaxiBenchmark
	} else {
		s.player.PlayerFlags &^= playerFlagTaxiBenchmark
	}
	s.sendPlayerUpdate()
	return true
}
