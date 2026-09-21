package wotlk

type SkillRaceClassInfoEntry struct {
	SkillID     uint32
	RaceMask    uint32
	ClassMask   uint32
	Flags       uint32
	SkillTierID uint32
}

func (s *Store) loadSkillRaceClassInfo() {
	file, err := s.File("SkillRaceClassInfo")
	if err != nil {
		s.srciErr = err
		return
	}
	result := make(map[uint32][]SkillRaceClassInfoEntry, file.Records())
	for index := 0; index < file.Records(); index++ {
		record, recordErr := file.Record(index)
		if recordErr != nil {
			continue
		}
		skillID, skillErr := record.Uint32(1)
		raceMask, raceErr := record.Uint32(2)
		classMask, classErr := record.Uint32(3)
		flags, flagsErr := record.Uint32(4)
		skillTierID, tierErr := record.Uint32(6)
		if skillErr == nil && raceErr == nil && classErr == nil && flagsErr == nil && tierErr == nil && skillID != 0 {
			result[skillID] = append(result[skillID], SkillRaceClassInfoEntry{SkillID: skillID, RaceMask: raceMask, ClassMask: classMask, Flags: flags, SkillTierID: skillTierID})
		}
	}
	s.srciMap = result
}

func (s *Store) SkillRaceClassInfo(skillID uint32, race, class uint8) (SkillRaceClassInfoEntry, bool, error) {
	s.srciOnce.Do(s.loadSkillRaceClassInfo)
	if s.srciErr != nil {
		return SkillRaceClassInfoEntry{}, false, s.srciErr
	}
	raceMask, classMask := uint32(0), uint32(0)
	if race > 0 && race <= 32 {
		raceMask = 1 << (race - 1)
	}
	if class > 0 && class <= 32 {
		classMask = 1 << (class - 1)
	}
	for _, entry := range s.srciMap[skillID] {
		if (entry.RaceMask == 0 || entry.RaceMask&raceMask != 0) && (entry.ClassMask == 0 || entry.ClassMask&classMask != 0) {
			return entry, true, nil
		}
	}
	return SkillRaceClassInfoEntry{}, false, nil
}
