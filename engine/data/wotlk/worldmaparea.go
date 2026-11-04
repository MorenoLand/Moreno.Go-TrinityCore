package wotlk

// WorldMapAreaEntry mirrors TrinityCore WorldMapAreaEntry
// (DBCStructure.h:1880) under WorldMapAreaEntryfmt "xinxffffixx"
// (DBCfmt.h:141): column 0 is the DBC row id, 1 MapID, 2 AreaID, 3 the area
// name, 4-7 the LocLeft/LocRight/LocTop/LocBottom bounds, 8 DisplayMapID.
// The store key is AreaID (the 'n' field in the format string), not the row
// id, so a lookup by area scans the file instead of using Find (which
// indexes column 0).
type WorldMapAreaEntry struct {
	ID           uint32 // 0
	MapID        uint32 // 1
	AreaID       uint32 // 2
	LocLeft      float32
	LocRight     float32
	LocTop       float32
	LocBottom    float32
	DisplayMapID int32 // 8
}

// WorldMapArea returns the WorldMapArea.dbc row keyed by area id, mirroring
// sWorldMapAreaStore.LookupEntry(zone) as used by Zone2MapCoordinates
// (DBCStores.cpp:776). DBCFileLoader::AutoProduceData overwrites duplicate
// keys in record order, so the last matching row wins.
func (s *Store) WorldMapArea(areaID uint32) (WorldMapAreaEntry, bool, error) {
	file, err := s.File("WorldMapArea")
	if err != nil {
		return WorldMapAreaEntry{}, false, err
	}
	var entry WorldMapAreaEntry
	found := false
	for i := 0; i < file.Records(); i++ {
		record, err := file.Record(i)
		if err != nil {
			continue
		}
		id, err := record.Uint32(2)
		if err != nil || id != areaID {
			continue
		}
		entry.ID, _ = record.Uint32(0)
		entry.MapID, _ = record.Uint32(1)
		entry.AreaID = id
		entry.LocLeft, _ = record.Float32(4)
		entry.LocRight, _ = record.Float32(5)
		entry.LocTop, _ = record.Float32(6)
		entry.LocBottom, _ = record.Float32(7)
		entry.DisplayMapID, _ = record.Int32(8)
		found = true
	}
	return entry, found, nil
}
