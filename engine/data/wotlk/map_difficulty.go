package wotlk

type MapDifficultyEntry struct {
	MapID           uint32
	Difficulty      uint32
	RaidDuration    uint32
	MaxPlayers      uint32
	HasErrorMessage bool
}

func (s *Store) MapDifficulty(mapID, difficulty uint32) (MapDifficultyEntry, bool, error) {
	file, err := s.File("MapDifficulty")
	if err != nil {
		return MapDifficultyEntry{}, false, err
	}
	var entry MapDifficultyEntry
	found := false
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			return MapDifficultyEntry{}, false, err
		}
		rowMap, err := record.Uint32(1)
		if err != nil {
			return MapDifficultyEntry{}, false, err
		}
		rowDifficulty, err := record.Uint32(2)
		if err != nil {
			return MapDifficultyEntry{}, false, err
		}
		if rowMap != mapID || rowDifficulty != difficulty {
			continue
		}
		message, err := record.String(3)
		if err != nil {
			return MapDifficultyEntry{}, false, err
		}
		raidDuration, err := record.Uint32(20)
		if err != nil {
			return MapDifficultyEntry{}, false, err
		}
		maxPlayers, err := record.Uint32(21)
		if err != nil {
			return MapDifficultyEntry{}, false, err
		}
		entry, found = MapDifficultyEntry{MapID: rowMap, Difficulty: rowDifficulty, RaidDuration: raidDuration, MaxPlayers: maxPlayers, HasErrorMessage: message != ""}, true
	}
	return entry, found, nil
}

func (s *Store) DownscaledMapDifficulty(mapID, difficulty uint32) (MapDifficultyEntry, uint32, bool, error) {
	entry, found, err := s.MapDifficulty(mapID, difficulty)
	if err != nil || found {
		return entry, difficulty, found, err
	}
	downscaled := difficulty
	if downscaled > 3 {
		downscaled -= 2
	} else {
		downscaled--
	}
	entry, found, err = s.MapDifficulty(mapID, downscaled)
	if err != nil || found {
		return entry, downscaled, found, err
	}
	downscaled--
	entry, found, err = s.MapDifficulty(mapID, downscaled)
	return entry, downscaled, found, err
}
