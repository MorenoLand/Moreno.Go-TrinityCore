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
func (s *Store) TaxiPathEntry(pathID uint32) (from, to, price uint32, found bool, err error) {
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
// discovers nodes the DBC knows (sTaxiNodesStore.LookupEntry).
func (s *Store) TaxiNodeExists(node uint32) bool {
	network, err := s.taxiNetwork()
	if err != nil {
		return false
	}
	_, ok := network.byID[node]
	return ok
}

// TaxiPathLinks returns the TaxiPath.dbc row id and price for a direct
// hop between two nodes (0 when no such path exists).
func (s *Store) TaxiPathLinks(from, to uint32) (uint32, uint32, bool, error) {
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
func (s *Store) TaxiPathPoints(pathID uint32) ([]TaxiSplinePoint, error) {
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
func (s *Store) TaxiNode(id uint32) (TaxiNode, bool, error) {
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
	network, err := s.taxiNetwork()
	if err != nil {
		return 0, err
	}
	teamIndex := 0
	if teamAlliance {
		teamIndex = 1
	}
	if index, ok := network.byID[node]; ok {
		if entry := network.nodes[index].MountCreatureID[teamIndex]; entry != 0 {
			return entry, nil
		}
		if allowedAltTeam {
			return network.nodes[index].MountCreatureID[1-teamIndex], nil
		}
	}
	return 0, nil
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
