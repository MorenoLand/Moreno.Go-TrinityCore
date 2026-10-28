package wotlk

type DungeonEncounterEntry struct {
	ID         uint32
	MapID      uint32
	Difficulty uint32
	Bit        uint32
}

func (s *Store) DungeonEncounters() ([]DungeonEncounterEntry, error) {
	file, err := s.File("DungeonEncounter")
	if err != nil {
		return nil, err
	}
	var entries []DungeonEncounterEntry
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			return nil, err
		}
		id, err := record.Uint32(0)
		if err != nil {
			return nil, err
		}
		rowMap, err := record.Uint32(1)
		if err != nil {
			return nil, err
		}
		rowDifficulty, err := record.Uint32(2)
		if err != nil {
			return nil, err
		}
		bit, err := record.Uint32(4)
		if err != nil {
			return nil, err
		}
		entries = append(entries, DungeonEncounterEntry{ID: id, MapID: rowMap, Difficulty: rowDifficulty, Bit: bit})
	}
	return entries, nil
}
