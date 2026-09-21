package wotlk

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/dbc"
)

type Store struct {
	Dir         string
	mu          sync.RWMutex
	files       map[string]*dbc.File
	wmoAreaOnce sync.Once
	wmoAreas    map[wmoAreaKey]uint32
	wmoAreaErr  error

	taxiOnce sync.Once
	taxi     *taxiNetwork
	taxiErr  error

	slaOnce    sync.Once
	slaMap     map[uint32][]SkillLineAbilityEntry
	slaBySkill map[uint32][]SkillLineAbilityEntry
	slaErr     error
	srciOnce   sync.Once
	srciMap    map[uint32][]SkillRaceClassInfoEntry
	srciErr    error
}

type wmoAreaKey struct{ root, adt, group int32 }

const MountedFlightSpeedAura uint32 = 207
const MapFlagDynamicDifficulty uint32 = 0x100

type Race struct {
	ID                uint32
	Flags             uint32
	FactionID         uint32
	MaleDisplayID     uint32
	FemaleDisplayID   uint32
	CinematicSequence uint32
	Alliance          uint32
	RequiredExpansion uint32
}

type Class struct {
	ID                uint32
	SpellClassSet     uint32
	CinematicSequence uint32
	RequiredExpansion uint32
}

type Reputation struct {
	ID             uint32
	ReputationList int32
	BaseStanding   int32
	DefaultFlags   uint8
}

type FactionTemplate struct {
	ID           uint32
	Faction      uint32
	Flags        uint32
	FactionGroup uint32
	FriendGroup  uint32
	EnemyGroup   uint32
	Enemies      [4]uint32
	Friends      [4]uint32
}

type SpellEffect struct {
	Effect          uint32
	BasePoints      int32
	Aura            uint32
	AuraPeriod      uint32
	ImplicitTargetA uint32
	ImplicitTargetB uint32
	RadiusIndex     uint32
	MiscValue       int32
	TriggerSpell    uint32
}

type Spell struct {
	ID                    uint32
	AreaGroupID           int32
	DispelType            uint32 // Spell.dbc field 2 = DispelType (DBCStructure.h:1394)
	Mechanic              uint32 // Spell.dbc field 3 = Mechanic (DBCStructure.h:1395)
	Attributes            uint32
	AttributesEx          uint32 // Spell.dbc field 5 = AttributesEx (DBCStructure.h:1397)
	AttributesEx1         uint32 // Spell.dbc field 6 = AttributesExB (DBCStructure.h:1398)
	AttributesEx3         uint32 // Spell.dbc field 7 = AttributesExC (DBCStructure.h:1399)
	AttributesEx4         uint32 // Spell.dbc field 8 = AttributesExD (DBCStructure.h:1402)
	AttributesEx5         uint32 // Spell.dbc field 9 = AttributesExE (DBCStructure.h:1403)
	SchoolMask            uint32
	Targets               uint32
	FacingCasterFlags     uint32 // Spell.dbc field 19 = FacingCasterFlags (DBCStructure.h:1409)
	CastingTimeIndex      uint32
	RecoveryTime          uint32
	ProcCharges           uint32 // Spell.dbc field 36 = ProcCharges (DBCStructure.h:1426)
	StackAmount           uint32 // Spell.dbc field 49 = CumulativeAura (DBCStructure.h:1440)
	PowerType             uint32
	ManaCost              uint32
	ManaCostPct           uint32
	RangeIndex            uint32
	InterruptFlags        uint32
	AuraInterruptFlags    uint32
	ChannelInterrupt      uint32
	DurationIndex         uint32
	SpellLevel            uint32
	PreventionType        uint32 // Spell.dbc field 214 = PreventionType (DBCStructure.h:1484)
	StartRecoveryCategory uint32 // Spell.dbc field 210 = StartRecoveryCategory (DBCStructure.h:1480)
	StartRecoveryTime     uint32 // Spell.dbc field 211 = StartRecoveryTime (DBCStructure.h:1481)
	EquippedItemClass     int32  // Spell.dbc field 68 = EquippedItemClass (DBCStructure.h:1443), -1 = any
	EquippedItemSubClass  uint32 // Spell.dbc field 69 = EquippedItemSubclass (DBCStructure.h:1444)
	EquippedItemInvTypes  uint32 // Spell.dbc field 70 = EquippedItemInvTypes (DBCStructure.h:1445)
	Speed                 float32
	Effects               [3]SpellEffect
}

type SpellRangeEntry struct {
	ID          uint32
	MinHostile  float32
	MinFriendly float32
	MaxHostile  float32
	MaxFriendly float32
	Flags       uint32
}

type LFGDungeon struct {
	ID             uint32
	MinLevel       uint32
	MaxLevel       uint32
	MapID          int32
	Difficulty     uint32
	Flags          uint32
	TypeID         uint32
	ExpansionLevel uint32
	GroupID        uint32
}

func (d LFGDungeon) Entry() uint32 {
	return d.ID + (d.TypeID << 24)
}

// WorldSafeLoc mirrors WorldSafeLocsEntry (DBCStructure.h): fields ID (u32),
// continent/map (u32), and x/y/z (f32); locale name fields follow and are not
// used by the server.
type WorldSafeLoc struct {
	ID    uint32
	MapID uint32
	X     float32
	Y     float32
	Z     float32
}

// AreaTableEntry mirrors AreaTableEntry (DBCStructure.h): fields ID (u32),
// ContinentID/map (u32), ParentAreaID/zone (u32), AreaBit (u32), Flags (u32),
// and FactionGroupMask (u32).
type AreaTableEntry struct {
	ID               uint32
	ContinentID      uint32
	ParentAreaID     uint32
	AreaBit          uint32
	Flags            uint32
	FactionGroupMask uint32
	Name             string
}

// TalentEntry mirrors TrinityCore's TalentEntry (DBCStructure.h:1663).
// Layout: "niiiiiiiixxxxixxixxxxxx" (23 fields).
type TalentEntry struct {
	ID           uint32
	TabID        uint32
	TierID       uint32
	ColumnIndex  uint32
	SpellRank    [5]uint32
	PrereqTalent uint32
	PrereqRank   uint32
}

// MapEntry mirrors TrinityCore's MapEntry (DBCStructure.h:1072, format "nxiixssssssssssssssssxix...").
type MapEntry struct {
	ID           uint32
	InstanceType uint32
	Flags        uint32
	MapName      string
	AreaTableID  uint32
	CorpseMapID  int32
	CorpseX      float32
	CorpseY      float32
	ExpansionID  uint32
	RaidOffset   uint32
	MaxPlayers   uint32
}

func (m MapEntry) IsDungeon() bool {
	return m.InstanceType == 1 || m.InstanceType == 2
}

func (m MapEntry) IsNonRaidDungeon() bool {
	return m.InstanceType == 1
}

func (m MapEntry) IsRaid() bool {
	return m.InstanceType == 2
}

func (m MapEntry) IsDynamicDifficultyMap() bool {
	return m.Flags&MapFlagDynamicDifficulty != 0
}

func (m MapEntry) IsBattleground() bool {
	return m.InstanceType == 3
}

func (m MapEntry) IsBattleArena() bool {
	return m.InstanceType == 4
}

const (
	AreaFlagSlaveCapital uint32 = 0x00000008 // AREA_FLAG_SLAVE_CAPITAL (DBCEnums.h:250)
	AreaFlagArena        uint32 = 0x00000080
	AreaFlagCapital      uint32 = 0x00000100
	AreaFlagSanctuary    uint32 = 0x00000800
	AreaFlagWintergrasp  uint32 = 0x01000000
	AreaFlagWintergrasp2 uint32 = 0x08000000 // AREA_FLAG_WINTERGRASP_2 (DBCEnums.h:274)
)

func NewStore(dir string) *Store {
	return &Store{Dir: dir, files: make(map[string]*dbc.File)}
}

func (s *Store) File(name string) (*dbc.File, error) {
	s.mu.RLock()
	file := s.files[name]
	s.mu.RUnlock()
	if file != nil {
		return file, nil
	}
	loaded, err := dbc.Open(filepath.Join(s.Dir, name+".dbc"))
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", name, err)
	}
	s.mu.Lock()
	if existing := s.files[name]; existing != nil {
		file = existing
	} else {
		s.files[name] = loaded
		file = loaded
	}
	s.mu.Unlock()
	return file, nil
}

func (s *Store) WMOArea(root, adt, group int32) (uint32, bool, error) {
	s.wmoAreaOnce.Do(func() {
		s.wmoAreas = make(map[wmoAreaKey]uint32)
		file, err := s.File("WMOAreaTable")
		if err != nil {
			s.wmoAreaErr = err
			return
		}
		for i := 0; i < file.Records(); i++ {
			record, recordErr := file.Record(i)
			if recordErr != nil {
				continue
			}
			wmoID, wmoErr := record.Int32(1)
			nameSetID, nameErr := record.Int32(2)
			groupID, groupErr := record.Int32(3)
			areaID, areaErr := record.Uint32(10)
			if wmoErr == nil && nameErr == nil && groupErr == nil && areaErr == nil && areaID != 0 {
				s.wmoAreas[wmoAreaKey{root: wmoID, adt: nameSetID, group: groupID}] = areaID
			}
		}
	})
	if s.wmoAreaErr != nil {
		return 0, false, s.wmoAreaErr
	}
	areaID, found := s.wmoAreas[wmoAreaKey{root: root, adt: adt, group: group}]
	return areaID, found, nil
}

func (s *Store) Race(id uint32) (Race, bool, error) {
	file, err := s.File("ChrRaces")
	if err != nil {
		return Race{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return Race{}, false, nil
	}
	flags, err := record.Uint32(1)
	if err != nil {
		return Race{}, false, err
	}
	factionID, err := record.Uint32(2)
	if err != nil {
		return Race{}, false, err
	}
	maleDisplayID, err := record.Uint32(4)
	if err != nil {
		return Race{}, false, err
	}
	femaleDisplayID, err := record.Uint32(5)
	if err != nil {
		return Race{}, false, err
	}
	cinematic, _ := record.Uint32(12)
	alliance, err := record.Uint32(13)
	if err != nil {
		return Race{}, false, err
	}
	requiredExpansion, err := record.Uint32(68)
	if err != nil {
		return Race{}, false, err
	}
	return Race{ID: id, Flags: flags, FactionID: factionID, MaleDisplayID: maleDisplayID, FemaleDisplayID: femaleDisplayID, CinematicSequence: cinematic, Alliance: alliance, RequiredExpansion: requiredExpansion}, true, nil
}

func (s *Store) ValidateAppearance(race, class, gender, hairID, hairColor, faceID, facialHair, skinColor uint8) (bool, bool, error) {
	sections, err := s.File("CharSections")
	if err != nil {
		return true, false, err
	}
	facial, err := s.File("CharacterFacialHairStyles")
	if err != nil {
		return true, false, err
	}
	sectionValid := func(sectionType, variation, color uint32) bool {
		for index := 0; index < sections.Records(); index++ {
			record, recordErr := sections.Record(index)
			if recordErr != nil {
				continue
			}
			r, rErr := record.Uint32(1)
			g, gErr := record.Uint32(2)
			base, baseErr := record.Uint32(3)
			flags, flagsErr := record.Uint32(7)
			variationIndex, variationErr := record.Uint32(8)
			colorIndex, colorErr := record.Uint32(9)
			if rErr == nil && gErr == nil && baseErr == nil && flagsErr == nil && variationErr == nil && colorErr == nil && r == uint32(race) && g == uint32(gender) && base == sectionType && variationIndex == variation && colorIndex == color {
				return class == 6 || flags&0x04 == 0
			}
		}
		return false
	}
	if !sectionValid(0, 0, uint32(skinColor)) || !sectionValid(1, uint32(faceID), uint32(skinColor)) || !sectionValid(3, uint32(hairID), uint32(hairColor)) {
		return false, true, nil
	}
	excludeFacial := race == 6 || race == 11 || (gender == 1 && race != 4 && race != 5)
	if !excludeFacial && !sectionValid(2, uint32(facialHair), uint32(hairColor)) {
		return false, true, nil
	}
	for index := 0; index < facial.Records(); index++ {
		record, recordErr := facial.Record(index)
		if recordErr != nil {
			continue
		}
		r, rErr := record.Uint32(0)
		g, gErr := record.Uint32(1)
		v, vErr := record.Uint32(2)
		if rErr == nil && gErr == nil && vErr == nil && r == uint32(race) && g == uint32(gender) && v == uint32(facialHair) {
			return true, true, nil
		}
	}
	return false, true, nil
}

func (s *Store) Class(id uint32) (Class, bool, error) {
	file, err := s.File("ChrClasses")
	if err != nil {
		return Class{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return Class{}, false, nil
	}
	spellClassSet, err := record.Uint32(56)
	if err != nil {
		return Class{}, false, err
	}
	cinematic, err := record.Uint32(58)
	if err != nil {
		return Class{}, false, err
	}
	requiredExpansion, err := record.Uint32(59)
	if err != nil {
		return Class{}, false, err
	}
	return Class{ID: id, SpellClassSet: spellClassSet, CinematicSequence: cinematic, RequiredExpansion: requiredExpansion}, true, nil
}

func (s *Store) Reputation(id uint32, race, class uint8) (Reputation, bool, error) {
	file, err := s.File("Faction")
	if err != nil {
		return Reputation{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return Reputation{}, false, nil
	}
	list, err := record.Int32(1)
	if err != nil {
		return Reputation{}, false, err
	}
	rep := Reputation{ID: id, ReputationList: list}
	if list < 0 {
		return rep, true, nil
	}
	raceMask := uint32(1) << uint(race-1)
	classMask := uint32(1) << uint(class-1)
	for i := 0; i < 4; i++ {
		raceMaskValue, raceErr := record.Uint32(2 + i)
		classMaskValue, classErr := record.Uint32(6 + i)
		base, baseErr := record.Int32(10 + i)
		flags, flagsErr := record.Uint32(14 + i)
		if raceErr != nil || classErr != nil || baseErr != nil || flagsErr != nil {
			return Reputation{}, false, fmt.Errorf("read faction %d reputation masks: %w", id, firstDBCError(raceErr, classErr, baseErr, flagsErr))
		}
		if (raceMaskValue&raceMask != 0 || (raceMaskValue == 0 && classMaskValue != 0)) && (classMaskValue&classMask != 0 || classMaskValue == 0) {
			rep.BaseStanding = base
			rep.DefaultFlags = uint8(flags)
			break
		}
	}
	return rep, true, nil
}

func (s *Store) Reputations(race, class uint8) ([]Reputation, error) {
	file, err := s.File("Faction")
	if err != nil {
		return nil, err
	}
	result := make([]Reputation, 0, 128)
	for index := 0; index < file.Records(); index++ {
		record, recordErr := file.Record(index)
		if recordErr != nil {
			continue
		}
		id, idErr := record.Uint32(0)
		if idErr != nil {
			continue
		}
		reputation, found, repErr := s.Reputation(id, race, class)
		if repErr != nil {
			return nil, repErr
		}
		if found && reputation.ReputationList >= 0 && reputation.ReputationList < 128 {
			result = append(result, reputation)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ReputationList < result[j].ReputationList })
	return result, nil
}

func (s *Store) FactionTemplate(id uint32) (FactionTemplate, bool, error) {
	file, err := s.File("FactionTemplate")
	if err != nil {
		return FactionTemplate{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return FactionTemplate{}, false, nil
	}
	template := FactionTemplate{ID: id}
	values := []*uint32{&template.Faction, &template.Flags, &template.FactionGroup, &template.FriendGroup, &template.EnemyGroup}
	for field, destination := range values {
		if *destination, err = record.Uint32(field + 1); err != nil {
			return FactionTemplate{}, false, err
		}
	}
	for index := range template.Enemies {
		if template.Enemies[index], err = record.Uint32(6 + index); err != nil {
			return FactionTemplate{}, false, err
		}
		if template.Friends[index], err = record.Uint32(10 + index); err != nil {
			return FactionTemplate{}, false, err
		}
	}
	return template, true, nil
}

func firstDBCError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CharStartOutfit(race, class, gender uint8) ([]uint32, error) {
	file, err := s.File("CharStartOutfit")
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(file.RecordCount); i++ {
		record, err := file.Record(i)
		if err != nil {
			continue
		}
		data := record.Data()
		if len(data) >= 104 && file.RecordSize == 296 {
			// In 3.3.5a CharStartOutfit.dbc (format dbbbXiiii...):
			// Offset 0..3: ID (uint32)
			// Offset 4: Race (uint8)
			// Offset 5: Class (uint8)
			// Offset 6: Gender (uint8)
			// Offset 7: OutfitID (uint8)
			// Offset 8..103: 24 Item IDs (int32)
			r := data[4]
			c := data[5]
			g := data[6]
			if r == race && c == class && g == gender {
				var items []uint32
				for j := 0; j < 24; j++ {
					offset := 8 + j*4
					itemID := int32(binary.LittleEndian.Uint32(data[offset : offset+4]))
					if itemID > 0 {
						items = append(items, uint32(itemID))
					}
				}
				return items, nil
			}
		} else {
			r, _ := record.Uint32(1)
			c, _ := record.Uint32(2)
			g, _ := record.Uint32(3)
			if uint8(r) == race && uint8(c) == class && uint8(g) == gender {
				var items []uint32
				for j := 5; j <= 28; j++ {
					itemID, err := record.Int32(j)
					if err == nil && itemID > 0 {
						items = append(items, uint32(itemID))
					}
				}
				return items, nil
			}
		}
	}
	return nil, nil
}

func (s *Store) Spell(id uint32) (Spell, bool, error) {
	file, err := s.File("Spell")
	if err != nil {
		return Spell{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return Spell{}, false, nil
	}
	spell := Spell{ID: id, EquippedItemClass: -1}
	values := []struct {
		field int
		dest  *uint32
	}{
		{2, &spell.DispelType}, // Spell.dbc field 2 = DispelType (DBCStructure.h:1394)
		{3, &spell.Mechanic},   // Spell.dbc field 3 = Mechanic (DBCStructure.h:1395)
		{4, &spell.Attributes},
		{225, &spell.SchoolMask}, // Spell.dbc field 225 = SchoolMask (DBCStructure.h:1492)
		{16, &spell.Targets},
		{19, &spell.FacingCasterFlags}, // Spell.dbc field 19 = FacingCasterFlags (DBCStructure.h:1409)
		{28, &spell.CastingTimeIndex},
		{29, &spell.RecoveryTime},
		{36, &spell.ProcCharges},
		{49, &spell.StackAmount},
		{41, &spell.PowerType},
		{42, &spell.ManaCost},
		{204, &spell.ManaCostPct}, // Spell.dbc field 204 = ManaCostPct (DBCStructure.h:1476)
		{46, &spell.RangeIndex},
		{5, &spell.AttributesEx},        // Spell.dbc field 5 = AttributesEx (DBCStructure.h:1397)
		{6, &spell.AttributesEx1},       // Spell.dbc field 6 = AttributesExB (DBCStructure.h:1398)
		{7, &spell.AttributesEx3},       // Spell.dbc field 7 = AttributesExC (DBCStructure.h:1399)
		{8, &spell.AttributesEx4},       // Spell.dbc field 8 = AttributesExD (DBCStructure.h:1402)
		{9, &spell.AttributesEx5},       // Spell.dbc field 9 = AttributesExE (DBCStructure.h:1403)
		{31, &spell.InterruptFlags},     // DBCStructure.h:1421
		{32, &spell.AuraInterruptFlags}, // DBCStructure.h:1422
		{33, &spell.ChannelInterrupt},
		{40, &spell.DurationIndex},
		{39, &spell.SpellLevel},
		{214, &spell.PreventionType},        // Spell.dbc field 214 = PreventionType (DBCStructure.h:1484)
		{205, &spell.StartRecoveryCategory}, // Spell.dbc field 205 = StartRecoveryCategory
		{206, &spell.StartRecoveryTime},     // Spell.dbc field 206 = StartRecoveryTime
	}
	for _, value := range values {
		if *value.dest, err = record.Uint32(value.field); err != nil {
			return Spell{}, false, err
		}
	}
	if areaGroupID, areaGroupErr := record.Int32(224); areaGroupErr == nil {
		spell.AreaGroupID = areaGroupID
	}
	if itemClass, itemErr := record.Int32(68); itemErr == nil {
		spell.EquippedItemClass = itemClass
	}
	if subClass, subErr := record.Uint32(69); subErr == nil {
		spell.EquippedItemSubClass = subClass
	}
	if invTypes, invErr := record.Uint32(70); invErr == nil {
		spell.EquippedItemInvTypes = invTypes
	}
	if speed, speedErr := record.Float32(47); speedErr == nil {
		spell.Speed = speed
	}
	for i := range spell.Effects {
		effect, err := record.Uint32(71 + i)
		if err != nil {
			return Spell{}, false, err
		}
		basePoints, err := record.Int32(80 + i)
		if err != nil {
			return Spell{}, false, err
		}
		aura, err := record.Uint32(95 + i)
		if err != nil {
			return Spell{}, false, err
		}
		implicitTargetA, err := record.Uint32(86 + i)
		if err != nil {
			return Spell{}, false, err
		}
		implicitTargetB, err := record.Uint32(89 + i)
		if err != nil {
			return Spell{}, false, err
		}
		radiusIndex, err := record.Uint32(92 + i)
		if err != nil {
			return Spell{}, false, err
		}
		miscValue, err := record.Int32(110 + i)
		if err != nil {
			return Spell{}, false, err
		}
		auraPeriod, err := record.Uint32(98 + i) // EffectAuraPeriod, DBCStructure.h:1492 area (98-100)
		if err != nil {
			return Spell{}, false, err
		}
		triggerSpell, err := record.Uint32(116 + i)
		if err != nil {
			return Spell{}, false, err
		}
		spell.Effects[i] = SpellEffect{Effect: effect, BasePoints: basePoints, Aura: aura, AuraPeriod: auraPeriod, ImplicitTargetA: implicitTargetA, ImplicitTargetB: implicitTargetB, RadiusIndex: radiusIndex, MiscValue: miscValue, TriggerSpell: triggerSpell}
	}
	return spell, true, nil
}

func (s *Store) SpellCastTime(id uint32) (int32, bool, error) {
	if id == 0 {
		return 0, true, nil
	}
	file, err := s.File("SpellCastTimes")
	if err != nil {
		return 0, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return 0, false, nil
	}
	base, err := record.Int32(1)
	if err != nil {
		return 0, false, err
	}
	return base, true, nil
}

func (s *Store) SpellRange(id uint32) (SpellRangeEntry, bool, error) {
	file, err := s.File("SpellRange")
	if err != nil {
		return SpellRangeEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return SpellRangeEntry{}, false, nil
	}
	entry := SpellRangeEntry{ID: id}
	values := []*float32{&entry.MinHostile, &entry.MinFriendly, &entry.MaxHostile, &entry.MaxFriendly}
	for index, value := range values {
		if *value, err = record.Float32(index + 1); err != nil {
			return SpellRangeEntry{}, false, err
		}
	}
	if entry.Flags, err = record.Uint32(5); err != nil {
		return SpellRangeEntry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) parseLFGDungeon(id uint32, record dbc.Record) (LFGDungeon, bool, error) {
	result := LFGDungeon{ID: id}
	values := []struct {
		field int
		dest  *uint32
	}{
		{18, &result.MinLevel},
		{19, &result.MaxLevel},
		{24, &result.Difficulty},
		{25, &result.Flags},
		{26, &result.TypeID},
		{29, &result.ExpansionLevel},
		{31, &result.GroupID},
	}
	var err error
	for _, value := range values {
		if *value.dest, err = record.Uint32(value.field); err != nil {
			return LFGDungeon{}, false, err
		}
	}
	mapID, err := record.Int32(23)
	if err != nil {
		return LFGDungeon{}, false, err
	}
	result.MapID = mapID
	return result, true, nil
}

func (s *Store) LFGDungeon(id uint32) (LFGDungeon, bool, error) {
	file, err := s.File("LFGDungeons")
	if err != nil {
		return LFGDungeon{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return LFGDungeon{}, false, nil
	}
	return s.parseLFGDungeon(id, record)
}

func (s *Store) LFGDungeons() ([]LFGDungeon, error) {
	file, err := s.File("LFGDungeons")
	if err != nil {
		return nil, err
	}
	dungeons := make([]LFGDungeon, 0, file.Records())
	for i := 0; i < file.Records(); i++ {
		record, err := file.Record(i)
		if err != nil {
			continue
		}
		id := record.Uint32Unchecked(0)
		dungeon, ok, err := s.parseLFGDungeon(id, record)
		if err == nil && ok {
			dungeons = append(dungeons, dungeon)
		}
	}
	return dungeons, nil
}

func IsSupportedLFGType(typeID uint32) bool {
	return typeID == 1 || typeID == 2 || typeID == 5 || typeID == 6
}

func IsPlayableRace(race Race) bool {
	return race.Alliance != 2 && race.Flags&1 == 0
}

func HasMountedFlightSpeed(spell Spell, speed int32) bool {
	for _, effect := range spell.Effects {
		if effect.Aura == MountedFlightSpeedAura && effect.BasePoints+1 == speed {
			return true
		}
	}
	return false
}

// WorldSafeLoc loads one WorldSafeLocs.dbc record by id. The record layout is
// "nifff" (DBCfmt.h WorldSafeLocsEntryfmt): id, map, x, y, z, then locale
// string references that the server does not use.
func (s *Store) WorldSafeLoc(id uint32) (WorldSafeLoc, bool, error) {
	file, err := s.File("WorldSafeLocs")
	if err != nil {
		return WorldSafeLoc{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return WorldSafeLoc{}, false, nil
	}
	mapID, err := record.Uint32(1)
	if err != nil {
		return WorldSafeLoc{}, false, err
	}
	x, err := record.Float32(2)
	if err != nil {
		return WorldSafeLoc{}, false, err
	}
	y, err := record.Float32(3)
	if err != nil {
		return WorldSafeLoc{}, false, err
	}
	z, err := record.Float32(4)
	if err != nil {
		return WorldSafeLoc{}, false, err
	}
	return WorldSafeLoc{ID: id, MapID: mapID, X: x, Y: y, Z: z}, true, nil
}

// Area loads one AreaTable.dbc record by id. Record layout is "niiiixxxxxissssssssssssssssxiiiiixxx".
func (s *Store) Area(id uint32) (AreaTableEntry, bool, error) {
	file, err := s.File("AreaTable")
	if err != nil {
		return AreaTableEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return AreaTableEntry{}, false, nil
	}
	continentID, err := record.Uint32(1)
	if err != nil {
		return AreaTableEntry{}, false, err
	}
	parentAreaID, err := record.Uint32(2)
	if err != nil {
		return AreaTableEntry{}, false, err
	}
	areaBit, err := record.Uint32(3)
	if err != nil {
		return AreaTableEntry{}, false, err
	}
	flags, err := record.Uint32(4)
	if err != nil {
		return AreaTableEntry{}, false, err
	}
	factionGroupMask, _ := record.Uint32(28)
	name, _ := record.String(11)
	return AreaTableEntry{
		ID:               id,
		ContinentID:      continentID,
		ParentAreaID:     parentAreaID,
		AreaBit:          areaBit,
		Flags:            flags,
		FactionGroupMask: factionGroupMask,
		Name:             name,
	}, true, nil
}

func (s *Store) AreaGroupAllows(id, zoneID, areaID uint32) (bool, bool, error) {
	if id == 0 {
		return true, true, nil
	}
	file, err := s.File("AreaGroup")
	if err != nil {
		return false, false, err
	}
	for depth := 0; depth < 32 && id != 0; depth++ {
		record, found := file.Find(id)
		if !found {
			return false, false, nil
		}
		for field := 1; field <= 6; field++ {
			area, fieldErr := record.Uint32(field)
			if fieldErr != nil {
				return false, true, fieldErr
			}
			if area != 0 && (area == zoneID || area == areaID) {
				return true, true, nil
			}
		}
		next, nextErr := record.Uint32(7)
		if nextErr != nil {
			return false, true, nextErr
		}
		id = next
	}
	return false, true, nil
}

// Talent loads a talent record by ID from Talent.dbc.
func (s *Store) Talent(id uint32) (TalentEntry, bool, error) {
	file, err := s.File("Talent")
	if err != nil {
		return TalentEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return TalentEntry{}, false, nil
	}
	tabID, err := record.Uint32(1)
	if err != nil {
		return TalentEntry{}, false, err
	}
	tierID, err := record.Uint32(2)
	if err != nil {
		return TalentEntry{}, false, err
	}
	col, err := record.Uint32(3)
	if err != nil {
		return TalentEntry{}, false, err
	}
	var ranks [5]uint32
	for i := 0; i < 5; i++ {
		ranks[i], _ = record.Uint32(4 + i)
	}
	prereqTalent, _ := record.Uint32(13)
	prereqRank, _ := record.Uint32(16)
	return TalentEntry{
		ID:           id,
		TabID:        tabID,
		TierID:       tierID,
		ColumnIndex:  col,
		SpellRank:    ranks,
		PrereqTalent: prereqTalent,
		PrereqRank:   prereqRank,
	}, true, nil
}

// TalentBySpell scans Talent.dbc for a talent record that grants the given spell.
func (s *Store) TalentBySpell(spellID uint32) (uint32, uint8, bool) {
	if spellID == 0 {
		return 0, 0, false
	}
	file, err := s.File("Talent")
	if err != nil {
		return 0, 0, false
	}
	for i := 0; i < file.Records(); i++ {
		rec, err := file.Record(i)
		if err != nil {
			continue
		}
		for r := 0; r < 5; r++ {
			sp, _ := rec.Uint32(4 + r)
			if sp == spellID {
				id, _ := rec.Uint32(0)
				return id, uint8(r), true
			}
		}
	}
	return 0, 0, false
}

func (s *Store) PetTalentSpells() (map[uint32]struct{}, error) {
	talents, err := s.File("Talent")
	if err != nil {
		return nil, err
	}
	tabs, err := s.File("TalentTab")
	if err != nil {
		return nil, err
	}
	spells := make(map[uint32]struct{})
	for i := 0; i < talents.Records(); i++ {
		record, recordErr := talents.Record(i)
		if recordErr != nil {
			continue
		}
		tabID, tabErr := record.Uint32(1)
		if tabErr != nil {
			continue
		}
		tab, found := tabs.Find(tabID)
		if !found {
			continue
		}
		petMask, maskErr := tab.Uint32(21)
		if maskErr != nil || petMask == 0 {
			continue
		}
		for rank := 0; rank < 5; rank++ {
			spellID, spellErr := record.Uint32(4 + rank)
			if spellErr == nil && spellID != 0 {
				spells[spellID] = struct{}{}
			}
		}
	}
	return spells, nil
}

// SpellDuration mirrors SpellInfo::GetDuration from SpellDuration.dbc
// (DBCStructure.h:1524, format "niii"): Duration + DurationPerLevel * (level-1)
// clamped to MaxDuration when positive. A negative base duration (-1) means
// infinite and is returned as-is.
func (s *Store) SpellDuration(id uint32, level uint32) (int32, bool, error) {
	if id == 0 {
		return 0, true, nil
	}
	file, err := s.File("SpellDuration")
	if err != nil {
		return 0, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return 0, false, nil
	}
	base, err := record.Int32(1)
	if err != nil {
		return 0, false, err
	}
	if base < 0 {
		return base, true, nil // -1 infinite
	}
	if level == 0 {
		level = 1
	}
	perLevel, err := record.Int32(2)
	if err != nil {
		return 0, false, err
	}
	maxDuration, err := record.Int32(3)
	if err != nil {
		return 0, false, err
	}
	duration := base + perLevel*int32(level-1)
	if maxDuration > 0 && duration > maxDuration {
		duration = maxDuration
	}
	return duration, true, nil
}

func (s *Store) SpellRadius(id, level uint32) (float32, bool, error) {
	if id == 0 {
		return 0, true, nil
	}
	file, err := s.File("SpellRadius")
	if err != nil {
		return 0, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return 0, false, nil
	}
	radius, err := record.Float32(1)
	if err != nil {
		return 0, false, err
	}
	perLevel, err := record.Float32(2)
	if err != nil {
		return 0, false, err
	}
	maxRadius, err := record.Float32(3)
	if err != nil {
		return 0, false, err
	}
	if level > 0 {
		radius += perLevel * float32(level)
		if maxRadius > 0 && radius > maxRadius {
			radius = maxRadius
		}
	}
	return radius, true, nil
}

func (s *Store) Map(id uint32) (MapEntry, bool, error) {
	file, err := s.File("Map")
	if err != nil {
		return MapEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return MapEntry{}, false, nil
	}
	instType, err := record.Uint32(2)
	if err != nil {
		return MapEntry{}, false, err
	}
	flags, _ := record.Uint32(3)
	name, _ := record.String(5)
	areaTableID, _ := record.Uint32(22)
	corpseMapID, _ := record.Int32(59)
	corpseX, _ := record.Float32(60)
	corpseY, _ := record.Float32(61)
	expansionID, _ := record.Uint32(63)
	raidOffset, _ := record.Uint32(64)
	maxPlayers, _ := record.Uint32(65)

	return MapEntry{
		ID:           id,
		InstanceType: instType,
		Flags:        flags,
		MapName:      name,
		AreaTableID:  areaTableID,
		CorpseMapID:  corpseMapID,
		CorpseX:      corpseX,
		CorpseY:      corpseY,
		ExpansionID:  expansionID,
		RaidOffset:   raidOffset,
		MaxPlayers:   maxPlayers,
	}, true, nil
}

// GemPropertiesEntry represents a record from GemProperties.dbc.
type GemPropertiesEntry struct {
	ID        uint32
	EnchantID uint32
	Type      uint32
}

type SpellItemEnchantmentEntry struct {
	ID         uint32
	ItemVisual uint32
}

func (s *Store) SpellItemEnchantment(id uint32) (SpellItemEnchantmentEntry, bool, error) {
	file, err := s.File("SpellItemEnchantment")
	if err != nil {
		return SpellItemEnchantmentEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return SpellItemEnchantmentEntry{}, false, nil
	}
	visual, err := record.Uint32(31)
	if err != nil {
		return SpellItemEnchantmentEntry{}, false, err
	}
	return SpellItemEnchantmentEntry{ID: id, ItemVisual: visual}, true, nil
}

// GemProperties loads a record by ID from GemProperties.dbc.
func (s *Store) GemProperties(id uint32) (GemPropertiesEntry, bool, error) {
	file, err := s.File("GemProperties")
	if err != nil {
		return GemPropertiesEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return GemPropertiesEntry{}, false, nil
	}
	enchantID, err := record.Uint32(1)
	if err != nil {
		return GemPropertiesEntry{}, false, err
	}
	gemType, err := record.Uint32(4)
	if err != nil {
		return GemPropertiesEntry{}, false, err
	}
	return GemPropertiesEntry{
		ID:        id,
		EnchantID: enchantID,
		Type:      gemType,
	}, true, nil
}

type GlyphPropertiesEntry struct {
	ID             uint32
	SpellID        uint32
	GlyphSlotFlags uint32
	SpellIconID    uint32
}

func (s *Store) GlyphProperties(id uint32) (GlyphPropertiesEntry, bool, error) {
	file, err := s.File("GlyphProperties")
	if err != nil {
		return GlyphPropertiesEntry{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return GlyphPropertiesEntry{}, false, nil
	}
	spellID, err := record.Uint32(1)
	if err != nil {
		return GlyphPropertiesEntry{}, false, err
	}
	slotFlags, err := record.Uint32(2)
	if err != nil {
		return GlyphPropertiesEntry{}, false, err
	}
	iconID, err := record.Uint32(3)
	if err != nil {
		return GlyphPropertiesEntry{}, false, err
	}
	return GlyphPropertiesEntry{ID: id, SpellID: spellID, GlyphSlotFlags: slotFlags, SpellIconID: iconID}, true, nil
}

func (s *Store) GlyphSlots() ([6]uint32, error) {
	file, err := s.File("GlyphSlot")
	if err != nil {
		return [6]uint32{}, err
	}
	var slots [6]uint32
	for i := 0; i < file.Records(); i++ {
		record, recordErr := file.Record(i)
		if recordErr != nil {
			continue
		}
		id, idErr := record.Uint32(0)
		tooltip, tooltipErr := record.Uint32(2)
		if idErr == nil && tooltipErr == nil && tooltip >= 1 && tooltip <= 6 {
			slots[tooltip-1] = id
		}
	}
	return slots, nil
}

func (s *Store) GlyphSlotType(id uint32) (uint32, bool, error) {
	file, err := s.File("GlyphSlot")
	if err != nil {
		return 0, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return 0, false, nil
	}
	typeFlags, err := record.Uint32(1)
	if err != nil {
		return 0, false, err
	}
	return typeFlags, true, nil
}

// SkillLineAbilityEntry represents a record from SkillLineAbility.dbc.
type SkillLineAbilityEntry struct {
	ID                uint32
	SkillLine         uint32
	Spell             uint32
	RaceMask          uint32
	ClassMask         uint32
	MinSkillLineRank  uint32
	SupercededBySpell uint32
	AcquireMethod     uint32
	TrivialRankHigh   uint32
	TrivialRankLow    uint32
}

func (s *Store) loadSkillLineAbilities() {
	file, err := s.File("SkillLineAbility")
	if err != nil {
		s.slaErr = err
		return
	}
	m := make(map[uint32][]SkillLineAbilityEntry, file.Records())
	bySkill := make(map[uint32][]SkillLineAbilityEntry)
	for i := 0; i < file.Records(); i++ {
		rec, err := file.Record(i)
		if err != nil {
			continue
		}
		id, _ := rec.Uint32(0)
		skillLine, _ := rec.Uint32(1)
		spell, _ := rec.Uint32(2)
		raceMask, _ := rec.Uint32(3)
		classMask, _ := rec.Uint32(4)
		minSkillLineRank, _ := rec.Uint32(7)
		supercededBySpell, _ := rec.Uint32(8)
		acquireMethod, _ := rec.Uint32(9)
		trivialRankHigh, _ := rec.Uint32(10)
		trivialRankLow, _ := rec.Uint32(11)
		if spell > 0 {
			entry := SkillLineAbilityEntry{
				ID:                id,
				SkillLine:         skillLine,
				Spell:             spell,
				RaceMask:          raceMask,
				ClassMask:         classMask,
				MinSkillLineRank:  minSkillLineRank,
				SupercededBySpell: supercededBySpell,
				AcquireMethod:     acquireMethod,
				TrivialRankHigh:   trivialRankHigh,
				TrivialRankLow:    trivialRankLow,
			}
			m[spell] = append(m[spell], entry)
			bySkill[skillLine] = append(bySkill[skillLine], entry)
		}
	}
	s.slaMap = m
	s.slaBySkill = bySkill
}

// SkillLineAbilities returns all SkillLineAbility records for the given spell.
func (s *Store) SkillLineAbilities(spellID uint32) ([]SkillLineAbilityEntry, bool, error) {
	s.slaOnce.Do(s.loadSkillLineAbilities)
	if s.slaErr != nil {
		return nil, false, s.slaErr
	}
	abilities, ok := s.slaMap[spellID]
	return abilities, ok, nil
}

func (s *Store) SkillLineAbilitiesForSkill(skillID uint32) ([]SkillLineAbilityEntry, bool, error) {
	s.slaOnce.Do(s.loadSkillLineAbilities)
	if s.slaErr != nil {
		return nil, false, s.slaErr
	}
	abilities, ok := s.slaBySkill[skillID]
	return abilities, ok, nil
}

// SpellName returns the name and subtext/rank for a spell from Spell.dbc.
func (s *Store) SpellName(id uint32) (string, string, bool, error) {
	file, err := s.File("Spell")
	if err != nil {
		return "", "", false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return "", "", false, nil
	}
	name, _ := record.String(136)
	rank, _ := record.String(153)
	return name, rank, true, nil
}

// AreaTableInfo mirrors the fields of AreaTableEntry used by exploration
// (DBCStructure.h:176): AreaBit (field 3) positions the zone in the
// PLAYER_EXPLORED_ZONES bitfield, ExplorationLevel (field 10) gates XP.
func (s *Store) AreaTableInfo(id uint32) (areaBit, explorationLevel int32, found bool, err error) {
	file, err := s.File("AreaTable")
	if err != nil {
		return 0, 0, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return 0, 0, false, nil
	}
	areaBit, err = record.Int32(3)
	if err != nil {
		return 0, 0, false, err
	}
	explorationLevel, err = record.Int32(10)
	if err != nil {
		return 0, 0, false, err
	}
	return areaBit, explorationLevel, true, nil
}

// WorldMapOverlayAreas mirrors WorldMapOverlayEntry (DBCStructure.h:1897):
// fields 2-5 carry up to four AreaTable ids covered by the overlay.
func (s *Store) WorldMapOverlayAreas(id uint32) ([4]uint32, bool, error) {
	file, err := s.File("WorldMapOverlay")
	if err != nil {
		return [4]uint32{}, false, err
	}
	record, ok := file.Find(id)
	if !ok {
		return [4]uint32{}, false, nil
	}
	var areas [4]uint32
	for i := 0; i < 4; i++ {
		area, err := record.Uint32(2 + i)
		if err != nil {
			return [4]uint32{}, false, err
		}
		areas[i] = area
	}
	return areas, true, nil
}
