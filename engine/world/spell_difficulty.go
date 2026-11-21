package world

// spellDifficultyMaxMode mirrors MAX_DIFFICULTY (DBCEnums.h:296): a
// SpellDifficulty row carries four variant slots, indexed by the map spawn
// mode (dungeon normal/heroic, raid 10N/25N/10H/25H = 0..3).
const spellDifficultyMaxMode = 4

// spellDifficultyHeroicMode mirrors DUNGEON_DIFFICULTY_HEROIC
// (DBCEnums.h:283): modes above it fall back two slots when their variant
// is empty (SpellMgr.cpp:202-206).
const spellDifficultyHeroicMode = 1

// spellIDForDifficulty mirrors SpellMgr::GetSpellIdForDifficulty
// (SpellMgr.cpp:176-233): the difficulty variant of spellID for the spawn
// mode of (mapID, instanceID), or spellID itself when the spell has no
// difficulty entry, the map is not a dungeon, the mode is out of range,
// or the variant slot is empty. Callers pass the caster's map and
// instance; the mode comes from the instance's difficulty pair.
func (s *Server) spellIDForDifficulty(spellID uint32, mapID uint32, instanceID uint32) uint32 {
	if s == nil || s.Data == nil || spellID == 0 {
		return spellID
	}
	if _, found, err := s.Data.Spell(spellID); err != nil || !found {
		return spellID
	}
	mapEntry, found, err := s.Data.Map(mapID)
	if err != nil || !found || !mapEntry.IsDungeon() {
		return spellID
	}
	mode := s.spellDifficultyMode(mapID, instanceID)
	if mode >= spellDifficultyMaxMode {
		return spellID
	}
	entryID, found, err := s.Data.SpellDifficultyIDForSpell(spellID)
	if err != nil || !found || entryID == 0 {
		return spellID
	}
	if mapped := s.spellDifficultyVariant(entryID, mode); mapped != 0 {
		return mapped
	}
	if mode > spellDifficultyHeroicMode {
		if mapped := s.spellDifficultyVariant(entryID, mode-2); mapped != 0 {
			return mapped
		}
	}
	return spellID
}

// spellDifficultyVariant reads one variant slot of a SpellDifficulty row;
// a missing row or failed load reads as empty (0).
func (s *Server) spellDifficultyVariant(entryID uint32, mode uint32) uint32 {
	if s == nil || s.Data == nil {
		return 0
	}
	variant, found, err := s.Data.SpellDifficulty(entryID, mode)
	if err != nil || !found {
		return 0
	}
	return variant
}

// spellDifficultyMode mirrors Map::GetSpawnMode for a creature caster: the
// instance's difficulty pair (autoBalanceInstanceDifficulty precedent —
// first session on the map+instance), raid maps taking the raid half,
// dungeon maps the dungeon half. No session present reads as normal (0).
func (s *Server) spellDifficultyMode(mapID uint32, instanceID uint32) uint32 {
	if s == nil {
		return 0
	}
	dungeonDiff, raidDiff := s.autoBalanceInstanceDifficulty(mapID, instanceID)
	if raidMapID(s, mapID) {
		return uint32(raidDiff)
	}
	return uint32(dungeonDiff)
}
