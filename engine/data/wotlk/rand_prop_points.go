package wotlk

type RandPropPointsEntry struct {
	ID       uint32
	Epic     [5]uint32
	Superior [5]uint32
	Good     [5]uint32
}

func (s *Store) RandPropPoints(id uint32) (RandPropPointsEntry, bool, error) {
	file, err := s.File("RandPropPoints")
	if err != nil {
		return RandPropPointsEntry{}, false, err
	}
	record, found := file.Find(id)
	if !found {
		return RandPropPointsEntry{}, false, nil
	}
	entry := RandPropPointsEntry{ID: id}
	for index := range entry.Epic {
		if entry.Epic[index], err = record.Uint32(1 + index); err != nil {
			return RandPropPointsEntry{}, false, err
		}
		if entry.Superior[index], err = record.Uint32(6 + index); err != nil {
			return RandPropPointsEntry{}, false, err
		}
		if entry.Good[index], err = record.Uint32(11 + index); err != nil {
			return RandPropPointsEntry{}, false, err
		}
	}
	return entry, true, nil
}
