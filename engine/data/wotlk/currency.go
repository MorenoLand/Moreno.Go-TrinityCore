package wotlk

func (s *Store) CurrencyBitIndex(itemID uint32) (uint32, bool, error) {
	if itemID == 0 {
		return 0, false, nil
	}
	file, err := s.File("CurrencyTypes")
	if err != nil {
		return 0, false, err
	}
	for index := 0; index < file.Records(); index++ {
		record, err := file.Record(index)
		if err != nil {
			continue
		}
		entry, err := record.Uint32(1)
		if err != nil || entry != itemID {
			continue
		}
		bitIndex, err := record.Uint32(3)
		return bitIndex, err == nil, err
	}
	return 0, false, nil
}
