package wotlk

import (
	"fmt"
	"sort"
)

// TaxiNode mirrors TrinityCore TaxiNodesEntry (3.3.5): ID, ContinentID,
// position and MountCreatureID[4] (index 0 = Horde, 1 = Alliance).
type TaxiNode struct {
	ID              uint32
	ContinentID     int32
	X               float32
	Y               float32
	Z               float32
	MountCreatureID [4]uint32
	HasOutgoingPath bool
}

const (
	taxiDKSpecialMount = 32981 // Ebon Hold flight, valid for both factions
	taxiMaskSize       = 14
)

type taxiNetwork struct {
	nodes []TaxiNode
	mask  [taxiMaskSize]uint32
	byID  map[uint32]int
}

func (s *Store) taxiNetwork() (*taxiNetwork, error) {
	s.taxiOnce.Do(func() {
		network := &taxiNetwork{byID: make(map[uint32]int)}
		nodesFile, err := s.File("TaxiNodes")
		if err != nil {
			s.taxiErr = err
			return
		}
		// TaxiPath.dbc: ID, FromTaxiNode, ToTaxiNode, price. Nodes carrying a
		// priced outgoing path belong to the visible flight network, matching
		// ObjectMgr's sTaxiNodesMask build (spell paths are price 0 and skip).
		pathSources := make(map[uint32]struct{})
		pathsFile, err := s.File("TaxiPath")
		if err != nil {
			s.taxiErr = err
			return
		}
		for i := 0; i < pathsFile.Records(); i++ {
			record, err := pathsFile.Record(i)
			if err != nil {
				s.taxiErr = err
				return
			}
			price, err := record.Uint32(3)
			if err != nil {
				s.taxiErr = err
				return
			}
			if price == 0 {
				continue
			}
			from, err := record.Uint32(1)
			if err != nil {
				s.taxiErr = err
				return
			}
			pathSources[from] = struct{}{}
		}
		for i := 0; i < nodesFile.Records(); i++ {
			record, err := nodesFile.Record(i)
			if err != nil {
				s.taxiErr = err
				return
			}
			node := TaxiNode{}
			if node.ID, err = record.Uint32(0); err != nil {
				s.taxiErr = err
				return
			}
			continent, err := record.Int32(1)
			if err != nil {
				s.taxiErr = err
				return
			}
			node.ContinentID = continent
			if node.X, err = record.Float32(2); err != nil {
				s.taxiErr = err
				return
			}
			if node.Y, err = record.Float32(3); err != nil {
				s.taxiErr = err
				return
			}
			if node.Z, err = record.Float32(4); err != nil {
				s.taxiErr = err
				return
			}
			for mount := 0; mount < 4; mount++ {
				if node.MountCreatureID[mount], err = record.Uint32(5 + mount); err != nil {
					s.taxiErr = err
					return
				}
			}
			if _, ok := pathSources[node.ID]; ok {
				node.HasOutgoingPath = true
			}
			if node.ID != 0 && node.HasOutgoingPath {
				field := (node.ID - 1) / 32
				if field < taxiMaskSize {
					network.mask[field] |= 1 << ((node.ID - 1) % 32)
				}
			}
			network.byID[node.ID] = len(network.nodes)
			network.nodes = append(network.nodes, node)
		}
		s.taxi = network
	})
	if s.taxiErr != nil {
		return nil, s.taxiErr
	}
	if s.taxi == nil {
		return nil, fmt.Errorf("taxi network unavailable")
	}
	return s.taxi, nil
}

// TaxiNetworkMask returns the mask of nodes belonging to the flight network.
func (s *Store) TaxiNetworkMask() ([taxiMaskSize]uint32, error) {
	network, err := s.taxiNetwork()
	if err != nil {
		return [taxiMaskSize]uint32{}, err
	}
	return network.mask, nil
}

// FactionTaxiMask mirrors the sHordeTaxiNodesMask / sAllianceTaxiNodesMask /
// sDeathKnightTaxiNodesMask build (DBCStores.cpp:589-632): nodes on the valid
// flight network bucketed by MountCreatureID — [0] serves Horde, [1] serves
// Alliance, and the 32981 Ebon Hold mount serves death knights of either
// faction (nodes 315 and 333 are DK-fixed like C++).
func (s *Store) FactionTaxiMask(horde bool, deathKnight bool) ([taxiMaskSize]uint32, error) {
	network, err := s.taxiNetwork()
	if err != nil {
		return [taxiMaskSize]uint32{}, err
	}
	var mask [taxiMaskSize]uint32
	for _, node := range network.nodes {
		if node.ID == 0 || !node.HasOutgoingPath {
			continue
		}
		hordeMount, allianceMount := node.MountCreatureID[0], node.MountCreatureID[1]
		if node.ID == 315 || node.ID == 333 {
			allianceMount = taxiDKSpecialMount
		}
		field := (node.ID - 1) / 32
		if field >= taxiMaskSize {
			continue
		}
		sub := uint32(1) << ((node.ID - 1) % 32)
		teamMount := hordeMount
		if !horde {
			teamMount = allianceMount
		}
		if teamMount != 0 && teamMount != taxiDKSpecialMount {
			mask[field] |= sub
		}
		if deathKnight && (hordeMount == taxiDKSpecialMount || allianceMount == taxiDKSpecialMount) {
			mask[field] |= sub
		}
	}
	return mask, nil
}

// DeathKnightTaxiMask mirrors the sDeathKnightTaxiNodesMask build
// (DBCStores.cpp:589-632): the words of nodes whose MountCreatureID is the
// death-knight special mount (32981) in either slot, on the valid flight
// network. It is ORed into the faction mask word-by-word at the
// CharacterHandler.cpp:1856 faction-change site through a uint8 cast, so
// callers must truncate each word to its low 8 bits themselves.
func (s *Store) DeathKnightTaxiMask() ([taxiMaskSize]uint32, error) {
	network, err := s.taxiNetwork()
	if err != nil {
		return [taxiMaskSize]uint32{}, err
	}
	var mask [taxiMaskSize]uint32
	for _, node := range network.nodes {
		if node.ID == 0 || !node.HasOutgoingPath {
			continue
		}
		if node.MountCreatureID[0] != taxiDKSpecialMount && node.MountCreatureID[1] != taxiDKSpecialMount {
			continue
		}
		field := (node.ID - 1) / 32
		if field >= taxiMaskSize {
			continue
		}
		mask[field] |= uint32(1) << ((node.ID - 1) % 32)
	}
	return mask, nil
}

// NearestTaxiNode mirrors ObjectMgr::GetNearestTaxiNode: closest network node
// on the map whose mount list serves the team (0 = Horde, 1 = Alliance; the
// 32981 Ebon Hold mount serves everyone).
func (s *Store) NearestTaxiNode(x, y, z float32, mapID uint32, teamAlliance bool) (uint32, error) {
	network, err := s.taxiNetwork()
	if err != nil {
		return 0, err
	}
	teamIndex := 0
	if teamAlliance {
		teamIndex = 1
	}
	found := false
	best := float64(0)
	id := uint32(0)
	for _, node := range network.nodes {
		if node.ContinentID != int32(mapID) {
			continue
		}
		if node.MountCreatureID[teamIndex] == 0 && node.MountCreatureID[0] != taxiDKSpecialMount {
			continue
		}
		field := (node.ID - 1) / 32
		if node.ID == 0 || field >= taxiMaskSize || network.mask[field]&(1<<((node.ID-1)%32)) == 0 {
			continue
		}
		dx := float64(node.X - x)
		dy := float64(node.Y - y)
		dz := float64(node.Z - z)
		dist2 := dx*dx + dy*dy + dz*dz
		if !found || dist2 < best {
			found = true
			best = dist2
			id = node.ID
		}
	}
	return id, nil
}

// TaxiPathEntry returns the TaxiPath.dbc row for a path id: the from/to
// taxi nodes and the path price. Used by Spell::EffectSendTaxi
// (SpellEffects.cpp:4656), which takes the path id from the effect's
// MiscValue (Player::ActivateTaxiPathTo(taxi_path_id), Player.cpp:21689).
// A script-registered path (GlobalMethods::AddTaxiPath) wins over the DBC
// row, matching sTaxiPathStore.SetEntry overwriting the loaded entry.
func (s *Store) TaxiPathEntry(pathID uint32) (from, to, price uint32, found bool, err error) {
	if path, ok := s.taxiOverlayPath(pathID); ok {
		return path.from, path.to, path.price, true, nil
	}
	pathsFile, err := s.File("TaxiPath")
	if err != nil {
		return 0, 0, 0, false, err
	}
	for i := 0; i < pathsFile.Records(); i++ {
		record, recErr := pathsFile.Record(i)
		if recErr != nil {
			return 0, 0, 0, false, recErr
		}
		id := record.Uint32Unchecked(0)
		if id != pathID {
			continue
		}
		from, err = record.Uint32(1)
		if err != nil {
			return 0, 0, 0, false, err
		}
		to, err = record.Uint32(2)
		if err != nil {
			return 0, 0, 0, false, err
		}
		price, err = record.Uint32(3)
		if err != nil {
			return 0, 0, 0, false, err
		}
		return from, to, price, true, nil
	}
	return 0, 0, 0, false, nil
}

// TaxiNodeExists reports whether a taxi node id has a TaxiNodes.dbc row.
// Used by Spell::EffectDiscoverTaxi (SpellEffects.cpp:5390), which only
// discovers nodes the DBC knows (sTaxiNodesStore.LookupEntry). LookupEntry
// also finds runtime-added nodes, so script-registered nodes count too.
func (s *Store) TaxiNodeExists(node uint32) bool {
	if _, ok := s.taxiOverlayNode(node); ok {
		return true
	}
	network, err := s.taxiNetwork()
	if err != nil {
		return false
	}
	_, ok := network.byID[node]
	return ok
}

// TaxiPathLinks returns the TaxiPath.dbc row id and price for a direct
// hop between two nodes (0 when no such path exists). A script-registered
// link wins, matching sTaxiPathSetBySource overwritten by AddTaxiPath.
func (s *Store) TaxiPathLinks(from, to uint32) (uint32, uint32, bool, error) {
	if link, ok := s.taxiOverlayLink(from, to); ok {
		return link.pathID, link.price, true, nil
	}
	pathsFile, err := s.File("TaxiPath")
	if err != nil {
		return 0, 0, false, err
	}
	for i := 0; i < pathsFile.Records(); i++ {
		record, err := pathsFile.Record(i)
		if err != nil {
			return 0, 0, false, err
		}
		pathFrom, err := record.Uint32(1)
		if err != nil {
			return 0, 0, false, err
		}
		pathTo, err := record.Uint32(2)
		if err != nil {
			return 0, 0, false, err
		}
		if pathFrom != from || pathTo != to {
			continue
		}
		price, err := record.Uint32(3)
		if err != nil {
			return 0, 0, false, err
		}
		return record.Uint32Unchecked(0), price, true, nil
	}
	return 0, 0, false, nil
}

// TaxiSplinePoint is one TaxiPathNode.dbc vertex of a flight path.
type TaxiSplinePoint struct {
	MapID            int32
	X                float32
	Y                float32
	Z                float32
	Flags            uint32
	Delay            uint32
	ArrivalEventID   uint32
	DepartureEventID uint32
}

// TaxiPathPoints loads the ordered spline vertices for a TaxiPath id.
// A script-registered path returns its waypoints in registration order,
// matching sTaxiPathNodesByPath[pathId] filled by AddTaxiPath.
func (s *Store) TaxiPathPoints(pathID uint32) ([]TaxiSplinePoint, error) {
	if path, ok := s.taxiOverlayPath(pathID); ok {
		return append([]TaxiSplinePoint(nil), path.points...), nil
	}
	nodesFile, err := s.File("TaxiPathNode")
	if err != nil {
		return nil, err
	}
	type indexed struct {
		index int32
		point TaxiSplinePoint
	}
	var points []indexed
	for i := 0; i < nodesFile.Records(); i++ {
		record, err := nodesFile.Record(i)
		if err != nil {
			return nil, err
		}
		nodePath, err := record.Uint32(1)
		if err != nil || nodePath != pathID {
			continue
		}
		nodeIndex, err := record.Int32(2)
		if err != nil {
			continue
		}
		point := TaxiSplinePoint{}
		point.MapID, _ = record.Int32(3)
		if point.X, err = record.Float32(4); err != nil {
			continue
		}
		if point.Y, err = record.Float32(5); err != nil {
			continue
		}
		if point.Z, err = record.Float32(6); err != nil {
			continue
		}
		point.Flags, _ = record.Uint32(7)
		delay, err := record.Uint32(8)
		if err == nil {
			point.Delay = delay
		}
		point.ArrivalEventID, _ = record.Uint32(9)
		point.DepartureEventID, _ = record.Uint32(10)
		points = append(points, indexed{index: nodeIndex, point: point})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].index < points[j].index })
	result := make([]TaxiSplinePoint, len(points))
	for i, p := range points {
		result[i] = p.point
	}
	return result, nil
}

// TaxiNode returns the TaxiNodes.dbc record for a node id, mirroring
// sTaxiNodesStore.LookupEntry (cs_go.cpp HandleGoTaxinodeCommand).
// LookupEntry also finds runtime-added nodes, so the overlay is checked.
func (s *Store) TaxiNode(id uint32) (TaxiNode, bool, error) {
	if node, ok := s.taxiOverlayNode(id); ok {
		return node, true, nil
	}
	network, err := s.taxiNetwork()
	if err != nil {
		return TaxiNode{}, false, err
	}
	if index, ok := network.byID[id]; ok {
		return network.nodes[index], true, nil
	}
	return TaxiNode{}, false, nil
}

// TaxiNodeMountEntry returns the mount creature ENTRY serving the team at a
// node, mirroring the selection arm of ObjectMgr::GetTaxiMountDisplayId
// (ObjectMgr.cpp:6752-6765): MountCreatureID[1] for Alliance, [0] otherwise,
// falling back to the other team's entry only when allowedAltTeam is set
// (the Acherus spell/DK hack). The entry is resolved to a display id by the
// world layer, which owns the creature_template table.
func (s *Store) TaxiNodeMountEntry(node uint32, teamAlliance, allowedAltTeam bool) (uint32, error) {
	if overlayNode, ok := s.taxiOverlayNode(node); ok {
		return taxiNodeMountEntry(overlayNode, teamAlliance, allowedAltTeam), nil
	}
	network, err := s.taxiNetwork()
	if err != nil {
		return 0, err
	}
	if index, ok := network.byID[node]; ok {
		return taxiNodeMountEntry(network.nodes[index], teamAlliance, allowedAltTeam), nil
	}
	return 0, nil
}

// taxiNodeMountEntry is the mount-entry selection shared by the DBC and
// overlay node records: MountCreatureID[1] for Alliance, [0] otherwise,
// falling back to the other team's entry only when allowedAltTeam is set.
func taxiNodeMountEntry(node TaxiNode, teamAlliance, allowedAltTeam bool) uint32 {
	teamIndex := 0
	if teamAlliance {
		teamIndex = 1
	}
	if entry := node.MountCreatureID[teamIndex]; entry != 0 {
		return entry
	}
	if allowedAltTeam {
		return node.MountCreatureID[1-teamIndex]
	}
	return 0
}

func (s *Store) OldContinentsTaxiNodes() ([]uint32, error) {
	network, err := s.taxiNetwork()
	if err != nil {
		return nil, err
	}
	nodes := make([]uint32, 0)
	for _, node := range network.nodes {
		if node.ID == 0 || !node.HasOutgoingPath || !(node.ContinentID < 2 || node.ID == 82 || node.ID == 83 || node.ID == 93 || node.ID == 94) {
			continue
		}
		nodes = append(nodes, node.ID)
	}
	return nodes, nil
}

func (s *Store) TaxiMask() ([14]uint32, error) {
	network, err := s.taxiNetwork()
	if err != nil {
		return [14]uint32{}, err
	}
	return network.mask, nil
}

// taxiOverlay is the runtime-mutable half of the C++ taxi DBC stores:
// GlobalMethods::AddTaxiPath writes sTaxiNodesStore, sTaxiPathNodesByPath,
// sTaxiPathSetBySource and sTaxiPathStore directly, and every lookup above
// consults the overlay first, so a registered path shadows the DBC row of
// the same id exactly like SetEntry does. Entries are only ever added:
// re-registering a path id allocates fresh node ids and leaves the old
// nodes and links in place, matching the C++ store behavior. The overlay
// deliberately never touches the taxi mask: C++ builds sTaxiNodesMask once
// at load, so script-added nodes are not mask-known either.
type taxiOverlayPath struct {
	from, to uint32
	price    uint32
	points   []TaxiSplinePoint
}

type taxiOverlayLink struct {
	pathID uint32
	price  uint32
}

type taxiOverlay struct {
	nodes     map[uint32]TaxiNode
	paths     map[uint32]taxiOverlayPath
	links     map[[2]uint32]taxiOverlayLink
	nextNode  uint32
	pathCount uint32
	sized     bool
}

// TaxiPathWaypoint is one GlobalMethods::AddTaxiPath waypoint: the map id
// and position are mandatory, Flags and Delay optional (both default 0).
type TaxiPathWaypoint struct {
	MapID   uint32
	X, Y, Z float32
	Flags   uint32
	Delay   uint32
}

// RegisterTaxiPath is GlobalMethods::AddTaxiPath (GlobalMethods.h, Trinity
// branch): it registers waypoints as a new taxi path and returns the used
// path id. Fewer than two waypoints registers nothing and returns the
// requested id (0 for the default arm), matching the C++ nodes.size() < 2
// early return, which returns the pathId argument untouched. A zero pathId
// allocates sTaxiPathNodesByPath.size() — the TaxiPath.dbc row count,
// grown by prior registrations — and node ids start at the C++ static
// 500, advancing once per waypoint across all registrations. MountCreatureID
// follows the C++ assignment: [0] is the Horde mount, [1] the Alliance one.
func (s *Store) RegisterTaxiPath(pathID uint32, waypoints []TaxiPathWaypoint, mountA, mountH, price uint32) uint32 {
	if len(waypoints) < 2 {
		return pathID
	}
	s.taxiOverlayMu.Lock()
	defer s.taxiOverlayMu.Unlock()
	if s.taxiOverlay == nil {
		s.taxiOverlay = &taxiOverlay{
			nodes:    make(map[uint32]TaxiNode),
			paths:    make(map[uint32]taxiOverlayPath),
			links:    make(map[[2]uint32]taxiOverlayLink),
			nextNode: 500,
		}
	}
	overlay := s.taxiOverlay
	if !overlay.sized {
		overlay.pathCount = s.taxiPathRowCount()
		overlay.sized = true
	}
	if pathID == 0 {
		pathID = overlay.pathCount
	}
	if pathID+1 > overlay.pathCount {
		overlay.pathCount = pathID + 1
	}
	startNode := overlay.nextNode
	points := make([]TaxiSplinePoint, len(waypoints))
	for i, waypoint := range waypoints {
		nodeID := overlay.nextNode
		overlay.nextNode++
		overlay.nodes[nodeID] = TaxiNode{
			ID:              nodeID,
			ContinentID:     int32(waypoint.MapID),
			X:               waypoint.X,
			Y:               waypoint.Y,
			Z:               waypoint.Z,
			MountCreatureID: [4]uint32{mountH, mountA},
		}
		points[i] = TaxiSplinePoint{
			MapID: int32(waypoint.MapID),
			X:     waypoint.X,
			Y:     waypoint.Y,
			Z:     waypoint.Z,
			Flags: waypoint.Flags,
			Delay: waypoint.Delay,
		}
	}
	endNode := overlay.nextNode - 1
	overlay.paths[pathID] = taxiOverlayPath{from: startNode, to: endNode, price: price, points: points}
	overlay.links[[2]uint32{startNode, endNode}] = taxiOverlayLink{pathID: pathID, price: price}
	return pathID
}

// taxiPathRowCount is sTaxiPathStore.GetNumRows for the default pathId arm.
func (s *Store) taxiPathRowCount() uint32 {
	pathsFile, err := s.File("TaxiPath")
	if err != nil {
		return 0
	}
	return uint32(pathsFile.Records())
}

func (s *Store) taxiOverlayNode(id uint32) (TaxiNode, bool) {
	s.taxiOverlayMu.RLock()
	defer s.taxiOverlayMu.RUnlock()
	if s.taxiOverlay == nil {
		return TaxiNode{}, false
	}
	node, ok := s.taxiOverlay.nodes[id]
	return node, ok
}

func (s *Store) taxiOverlayPath(id uint32) (taxiOverlayPath, bool) {
	s.taxiOverlayMu.RLock()
	defer s.taxiOverlayMu.RUnlock()
	if s.taxiOverlay == nil {
		return taxiOverlayPath{}, false
	}
	path, ok := s.taxiOverlay.paths[id]
	return path, ok
}

func (s *Store) taxiOverlayLink(from, to uint32) (taxiOverlayLink, bool) {
	s.taxiOverlayMu.RLock()
	defer s.taxiOverlayMu.RUnlock()
	if s.taxiOverlay == nil {
		return taxiOverlayLink{}, false
	}
	link, ok := s.taxiOverlay.links[[2]uint32{from, to}]
	return link, ok
}
