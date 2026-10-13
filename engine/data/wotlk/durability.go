package wotlk

// DurabilityCostsEntry mirrors DBCStructure.h (server/shared/DataStores):
// ID (0), WeaponSubClassCost[21] (1-21), ArmorSubClassCost[8] (22-29).
type DurabilityCostsEntry struct {
	ID                 uint32
	WeaponSubClassCost [21]uint32
	ArmorSubClassCost  [8]uint32
}

// DurabilityCosts returns the DurabilityCosts.dbc row keyed by item level.
// Player::DurabilityRepair (Player.cpp:5024) looks the row up with
// sDurabilityCostsStore.LookupEntry(ditemProto->ItemLevel).
func (s *Store) DurabilityCosts(itemLevel uint32) (DurabilityCostsEntry, bool, error) {
	file, err := s.File("DurabilityCosts")
	if err != nil {
		return DurabilityCostsEntry{}, false, err
	}
	record, ok := file.Find(itemLevel)
	if !ok {
		return DurabilityCostsEntry{}, false, nil
	}
	entry := DurabilityCostsEntry{ID: itemLevel}
	for i := 0; i < 21; i++ {
		if entry.WeaponSubClassCost[i], err = record.Uint32(1 + i); err != nil {
			return DurabilityCostsEntry{}, false, err
		}
	}
	for i := 0; i < 8; i++ {
		if entry.ArmorSubClassCost[i], err = record.Uint32(22 + i); err != nil {
			return DurabilityCostsEntry{}, false, err
		}
	}
	return entry, true, nil
}

// DurabilityQuality returns the DurabilityQuality.dbc Data float for id.
// Player::DurabilityRepair (Player.cpp:5030-5031) keys the lookup with
// (ditemProto->Quality+1)*2.
func (s *Store) DurabilityQuality(id uint32) (float32, bool, error) {
	file, err := s.File("DurabilityQuality")
	if err != nil {
		return 0, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return 0, false, nil
	}
	data, err := record.Float32(1)
	if err != nil {
		return 0, false, err
	}
	return data, true, nil
}
