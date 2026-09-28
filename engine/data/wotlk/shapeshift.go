package wotlk

type ShapeshiftForm struct {
	ID                 uint32
	Flags              uint32
	CreatureType       int32
	CombatRoundTime    uint32
	CreatureDisplayIDs [4]uint32
	PresetSpellIDs     [8]uint32
}

func (s *Store) ShapeshiftForm(id uint32) (ShapeshiftForm, bool, error) {
	file, err := s.File("SpellShapeshiftForm")
	if err != nil {
		return ShapeshiftForm{}, false, err
	}
	record, found := file.Find(id)
	if !found {
		return ShapeshiftForm{}, false, nil
	}
	form := ShapeshiftForm{ID: id}
	if form.Flags, err = record.Uint32(19); err != nil {
		return ShapeshiftForm{}, false, err
	}
	if form.CreatureType, err = record.Int32(20); err != nil {
		return ShapeshiftForm{}, false, err
	}
	if form.CombatRoundTime, err = record.Uint32(22); err != nil {
		return ShapeshiftForm{}, false, err
	}
	for i := range form.CreatureDisplayIDs {
		if form.CreatureDisplayIDs[i], err = record.Uint32(23 + i); err != nil {
			return ShapeshiftForm{}, false, err
		}
	}
	for i := range form.PresetSpellIDs {
		if form.PresetSpellIDs[i], err = record.Uint32(27 + i); err != nil {
			return ShapeshiftForm{}, false, err
		}
	}
	return form, true, nil
}
