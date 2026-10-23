package world

import (
	"context"
	"database/sql"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	petSaveAsDeleted        uint8 = 0
	petSaveAsCurrent        uint8 = 1
	petSaveNotInSlot        uint8 = 2
	petStorageSlotNotInSlot uint8 = 100
	petTypeSummon           uint8 = 0
	petTypeHunter           uint8 = 1

	// Pet talk event ids (PetDefines.h:80-81): the SMSG_PET_ACTION_SOUND
	// payload for the 10% summon-pet talk arms (PetHandler.cpp:216, 346, 816).
	petTalkSpecialSpell uint32 = 0
	petTalkAttack       uint32 = 1
	// AI_REACTION_HOSTILE (Unit.h): the SMSG_AI_REACTION growl payload.
	aiReactionHostile    uint32 = 2
	creatureTypeDemon    uint32 = 3
	creatureTypeUndead   uint32 = 6
	creatureTypeHumanoid uint32 = 7 // CREATURE_TYPE_HUMANOID (SharedDefines.h:2667)

	// creatureTypeMaskHumanoidOrUndead mirrors CREATURE_TYPEMASK_HUMANOID_OR_UNDEAD
	// (SharedDefines.h:2677): (1 << (CREATURE_TYPE_HUMANOID-1)) | (1 << (CREATURE_TYPE_UNDEAD-1)).
	creatureTypeMaskHumanoidOrUndead uint32 = (1 << (creatureTypeHumanoid - 1)) | (1 << (creatureTypeUndead - 1))

	// creatureTypeMaskDemonOrUndead mirrors CREATURE_TYPEMASK_DEMON_OR_UNDEAD
	// (SharedDefines.h:2678): (1 << (CREATURE_TYPE_DEMON-1)) | (1 << (CREATURE_TYPE_UNDEAD-1)).
	creatureTypeMaskDemonOrUndead uint32 = (1 << (creatureTypeDemon - 1)) | (1 << (creatureTypeUndead - 1))

	petActionPassive   uint8  = 0x01
	petActionDisabled  uint8  = 0x81
	petActionEnabled   uint8  = 0xC1
	petSpellPassive    uint32 = 0x00000040
	petSpellNoAutocast uint32 = 0x00020000
	petFollowDistance         = float32(1.0)

	unitFieldSummonedBy       = 14
	unitFieldCreatedBy        = 16
	unitFieldPetNumber        = 75
	unitFieldPetNameTimestamp = 76
	unitFieldPetExperience    = 77
	unitFieldPetNextLevelExp  = 78
	unitFieldCreatedBySpell   = 81
	petFocusMax               = 100
	// petEnergyMax mirrors Unit::GetCreatePowerValue(POWER_ENERGY)
	// (Unit.cpp:9686-9688): 100 for every unit. Only DK ghouls and risen
	// allies display it (Pet.cpp:930-939).
	petEnergyMax    = 100
	petHappinessMax = 1050000
	// petTameStartHappiness mirrors Pet::CreateBaseAtTamed (Pet.cpp:830):
	// a freshly tamed pet starts at 166500, not at max happiness, so it
	// deals the UNHAPPY 75% damage leg until fed (Pet.cpp:707-714).
	petTameStartHappiness = 166500
)

// petSpellActionType reports whether a bar slot type holds a spell action
// (Unit.h:594-598: UnitActionBarEntry::IsActionBarForSpell), as opposed to a
// command or reaction button.
func petSpellActionType(actionType uint8) bool {
	return actionType == petActionPassive || actionType == petActionDisabled || actionType == petActionEnabled
}

// makePetActionSlot packs a bar slot the way MAKE_UNIT_ACTION_BUTTON does
// (Unit.h:583): the action in the low 24 bits, the type in the high byte.
func makePetActionSlot(actionType uint8, action uint32) uint32 {
	return action&0x00FFFFFF | uint32(actionType)<<24
}

// loadPetActionBar reads the 10 packed action-bar slots from the pet's
// character_pet.abdata row. The string form is the GenerateActionBarData
// layout (Pet.cpp:485): "type action type action ..." with 20 tokens.
func loadPetActionBar(ctx context.Context, cdb interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, petID uint32, ownerGUID uint64) ([10]uint32, bool) {
	var slots [10]uint32
	var abdata string
	if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(abdata, '') FROM character_pet WHERE id = ? AND owner = ?", petID, ownerGUID).Scan(&abdata); err != nil {
		return slots, false
	}
	tokens := strings.Fields(abdata)
	if len(tokens) != 20 {
		return slots, false
	}
	for i := 0; i < 10; i++ {
		t, tErr := strconv.ParseUint(tokens[i*2], 10, 8)
		a, aErr := strconv.ParseUint(tokens[i*2+1], 10, 32)
		if tErr != nil || aErr != nil {
			return slots, false
		}
		slots[i] = makePetActionSlot(uint8(t), uint32(a))
	}
	return slots, true
}

// savePetActionBar persists the 10 packed slots back to the pet's abdata.
func savePetActionBar(ctx context.Context, cdb interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, petID uint32, ownerGUID uint64, slots [10]uint32) {
	var sb strings.Builder
	for i, slot := range slots {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(strconv.FormatUint(uint64(slot>>24), 10))
		sb.WriteByte(' ')
		sb.WriteString(strconv.FormatUint(uint64(slot&0x00FFFFFF), 10))
	}
	_, _ = cdb.ExecContext(ctx, "UPDATE character_pet SET abdata = ? WHERE id = ? AND owner = ?", sb.String(), petID, ownerGUID)
}

func (s *session) normalizePetActionBarSlot(slot uint32) uint32 {
	actionType := uint8(slot >> 24)
	action := slot & 0x00FFFFFF
	if actionType != petActionPassive && actionType != petActionDisabled && actionType != petActionEnabled {
		return slot
	}
	if action == 0 {
		return uint32(petActionPassive) << 24
	}
	if s == nil || s.server == nil || s.server.Data == nil {
		return slot
	}
	spell, found, err := s.server.Data.Spell(action)
	if err != nil {
		return slot
	}
	if !found {
		return uint32(petActionPassive) << 24
	}
	if spell.Attributes&petSpellPassive != 0 || spell.AttributesEx&petSpellNoAutocast != 0 {
		return action | uint32(petActionPassive)<<24
	}
	return slot
}

func (s *Server) nextPetLowGUID(mapID, instanceID uint32) uint32 {
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	if s.nextPetGUID == nil {
		s.nextPetGUID = make(map[instanceAdmissionKey]uint32)
	}
	key := instanceAdmissionKey{MapID: mapID, InstanceID: instanceID}
	if s.nextPetGUID[key] >= 0x00FFFFFF {
		return 0
	}
	s.nextPetGUID[key]++
	return s.nextPetGUID[key]
}

func (s *session) activePetNumber() uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	if s.player.PetNumber != 0 {
		return s.player.PetNumber
	}
	return uint32(s.player.PetGUID >> 24 & 0x00FFFFFF)
}

func (s *session) petNumberForGUID(guid uint64) uint32 {
	if s == nil || s.player == nil || s.player.PetGUID != guid || guid>>48 != 0xF140 {
		return 0
	}
	if s.player.PetNumber != 0 {
		return s.player.PetNumber
	}
	return uint32(guid >> 24 & 0x00FFFFFF)
}

func (s *session) petGUIDForNumber(number uint32) uint64 {
	if s != nil && s.player != nil && s.player.PetNumber == number && s.player.PetGUID != 0 {
		return s.player.PetGUID
	}
	return 0
}

func makePetGUID(number, counter uint32) uint64 {
	return uint64(0xF140)<<48 | uint64(number)<<24 | uint64(counter)
}

type petSpellRank struct {
	spellID  uint32
	minLevel uint32
	autocast bool
}

type petSpellChain []petSpellRank

var defaultPetSpells = map[uint32][]petSpellChain{
	// Imp (entry 416)
	416: {
		// Firebolt
		{
			{3110, 1, true},
			{7799, 8, true},
			{7800, 18, true},
			{7801, 28, true},
			{7802, 38, true},
			{11762, 48, true},
			{11763, 58, true},
			{27267, 68, true},
			{47964, 77, true},
		},
		// Blood Pact
		{
			{6307, 4, true},
			{7804, 14, true},
			{7805, 26, true},
			{11766, 38, true},
			{11767, 50, true},
			{27268, 62, true},
			{47982, 74, true},
		},
		// Phase Shift
		{
			{4511, 12, true},
		},
		// Fire Shield
		{
			{2947, 14, true},
			{8316, 24, true},
			{8317, 34, true},
			{11770, 44, true},
			{11771, 54, true},
			{27269, 64, true},
			{47983, 76, true},
		},
	},
	// Voidwalker (entry 1860)
	1860: {
		// Torment
		{
			{3716, 10, true},
			{7809, 20, true},
			{7810, 30, true},
			{7811, 40, true},
			{11774, 50, true},
			{11775, 60, true},
			{27270, 70, true},
			{47984, 77, true},
		},
		// Sacrifice
		{
			{7812, 16, false},
			{19438, 24, false},
			{19440, 32, false},
			{19441, 40, false},
			{19442, 48, false},
			{19443, 56, false},
			{27273, 64, false},
			{47985, 72, false},
			{47986, 80, false},
		},
		// Consume Shadows
		{
			{17767, 20, false},
			{17850, 28, false},
			{17851, 36, false},
			{17852, 44, false},
			{17853, 52, false},
			{17854, 60, false},
			{27272, 68, false},
			{47987, 76, false},
			{47988, 80, false},
		},
		// Suffering
		{
			{17735, 24, true},
			{17750, 34, true},
			{17751, 44, true},
			{17752, 54, true},
			{27271, 64, true},
			{33701, 72, true},
			{47989, 80, true},
		},
	},
	// Succubus (entry 1863)
	1863: {
		// Lash of Pain
		{
			{7814, 20, true},
			{7815, 28, true},
			{7816, 36, true},
			{11778, 44, true},
			{11779, 52, true},
			{11780, 60, true},
			{27274, 68, true},
			{47991, 76, true},
			{47992, 80, true},
		},
		// Soothing Kiss
		{
			{6360, 22, true},
			{7813, 32, true},
			{11784, 42, true},
			{11785, 52, true},
			{27275, 62, true},
		},
		// Seduction
		{
			{6358, 26, false},
		},
		// Lesser Invisibility
		{
			{7870, 32, false},
		},
	},
	// Felhunter (entry 417)
	417: {
		// Devour Magic
		{
			{19505, 30, true},
			{19731, 38, true},
			{19732, 46, true},
			{19733, 54, true},
			{19734, 62, true},
			{19735, 70, true},
			{27276, 74, true},
			{48011, 80, true},
		},
		// Spell Lock
		{
			{19647, 40, false},
			{19244, 50, false},
		},
		// Shadow Bite
		{
			{54049, 30, true},
			{54050, 42, true},
			{54051, 54, true},
			{54052, 66, true},
			{54053, 78, true},
		},
		// Fel Intelligence
		{
			{57564, 30, true},
			{57565, 42, true},
			{57566, 54, true},
			{57567, 66, true},
			{54424, 78, true},
		},
	},
	// Felguard (entry 17252)
	17252: {
		// Cleave
		{
			{30213, 50, true},
			{30219, 60, true},
			{30223, 70, true},
			{47994, 78, true},
		},
		// Intercept
		{
			{30151, 50, true},
			{30194, 60, true},
			{30198, 70, true},
			{47996, 78, true},
		},
		// Anguish
		{
			{30211, 50, true},
			{30148, 60, true},
			{30149, 70, true},
			{47993, 78, true},
		},
		// Demonic Frenzy
		{
			{32851, 50, true},
		},
	},
	// Water Elemental (entry 510)
	510: {
		{
			{31707, 50, true},
		},
		{
			{33395, 50, false},
		},
	},
	// Risen Ghoul (entry 26125)
	26125: {
		{
			{47468, 55, true},
		},
		{
			{47484, 55, false},
		},
		{
			{47481, 55, false},
		},
		{
			{47482, 55, false},
		},
	},
}

var defaultHunterPetSpells = []petSpellChain{
	// Growl
	{
		{2649, 1, true},
		{14916, 10, true},
		{14917, 20, true},
		{14918, 30, true},
		{14919, 40, true},
		{14920, 50, true},
		{14921, 60, true},
		{27047, 70, true},
		{48045, 80, true},
	},
	// Cower
	{
		{1742, 5, true},
		{1743, 15, true},
		{1744, 25, true},
		{1745, 35, true},
		{1746, 45, true},
		{1747, 55, true},
		{27048, 65, true},
		{48046, 75, true},
	},
}

type pSpell struct {
	id     uint32
	active uint8
}

// petLearnActiveState bridges the ACT_DECIDE arm of Pet::addSpell
// (Pet.cpp:1404-1411): an autocastable spell (SpellInfo::IsAutocastable,
// SpellInfo.cpp:1101-1108 — not passive, no
// SPELL_ATTR1_UNAUTOCASTABLE_BY_PET) stores ACT_DISABLED; everything else
// stores ACT_PASSIVE. pet_spell.active is packed verbatim into
// SMSG_PET_SPELLS (action << 24), so the wrong state makes a castable
// pet spell show as passive (unusable) or auto-cast from the start.
func (s *session) petLearnActiveState(spellID uint32) uint8 {
	if s != nil && s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found &&
			spell.Attributes&spellAttributePassive == 0 &&
			spell.AttributesEx&spellAttr1UnautocastableByPet == 0 {
			return petActionDisabled
		}
	}
	return petActionPassive
}

// learnPetFamilyPassives bridges Pet::LearnPetPassives (Pet.cpp:1883-1902):
// the pet's creature family (creature_template.family) resolves passive
// spells via the DBC-built sPetFamilySpellsStore twin
// (wotlk.Store.PetFamilySpells); ACT_DECIDE on a passive stores ACT_PASSIVE
// (0x01, UnitDefines.h:330; Pet::addSpell, Pet.cpp:1404-1411).
// INSERT OR REPLACE keeps the load-path re-learn (Pet.cpp:383) idempotent.
func (s *session) learnPetFamilyPassives(ctx context.Context, petID uint32, entry uint32) {
	if s == nil || s.server == nil || s.server.Data == nil || petID == 0 || entry == 0 {
		return
	}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	var family int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(family, 0) FROM creature_template WHERE entry = ?", entry).Scan(&family); err != nil || family <= 0 {
		return
	}
	passives, err := s.server.Data.PetFamilySpells(uint32(family))
	if err != nil {
		return
	}
	for _, spellID := range passives {
		_, _ = cdb.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, spellID, s.petLearnActiveState(spellID))
	}
}

func getPetSpellsForLevel(entry uint32, level uint32) []pSpell {
	var result []pSpell
	chains, ok := defaultPetSpells[entry]
	if !ok {
		chains = defaultHunterPetSpells
	}
	for _, chain := range chains {
		var bestRank *petSpellRank
		for i := range chain {
			if chain[i].minLevel <= level {
				bestRank = &chain[i]
			}
		}
		if bestRank != nil {
			active := uint8(petActionDisabled)
			if bestRank.autocast {
				active = uint8(petActionEnabled)
			}
			result = append(result, pSpell{id: bestRank.spellID, active: active})
		}
	}
	return result
}

func getPetSpellChains(entry uint32) []petSpellChain {
	chains, ok := defaultPetSpells[entry]
	if !ok {
		chains = defaultHunterPetSpells
	}
	return chains
}

func (s *session) generatePetName(ctx context.Context, entry uint32) string {
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry != 0 {
		wdb := s.server.WorldStore.DB
		var half0, half1 []string
		rows0, err0 := wdb.QueryContext(ctx, "SELECT word FROM pet_name_generation WHERE entry = ? AND half = 0", entry)
		if err0 == nil {
			defer rows0.Close()
			for rows0.Next() {
				var w string
				if rows0.Scan(&w) == nil {
					half0 = append(half0, w)
				}
			}
		}
		rows1, err1 := wdb.QueryContext(ctx, "SELECT word FROM pet_name_generation WHERE entry = ? AND half = 1", entry)
		if err1 == nil {
			defer rows1.Close()
			for rows1.Next() {
				var w string
				if rows1.Scan(&w) == nil {
					half1 = append(half1, w)
				}
			}
		}
		if len(half0) > 0 && len(half1) > 0 {
			now := time.Now().UnixNano()
			idx0 := int(now % int64(len(half0)))
			idx1 := int((now / 1000) % int64(len(half1)))
			return half0[idx0] + half1[idx1]
		}
		var cName string
		if err := wdb.QueryRowContext(ctx, "SELECT name FROM creature_template WHERE entry = ?", entry).Scan(&cName); err == nil && cName != "" {
			return cName
		}
	}
	return "Pet"
}

func (s *session) getPetStats(ctx context.Context, entry uint32, level uint32, petType uint8, petNumber uint32) (curHealth, maxHealth, curMana, maxMana uint32) {
	if level == 0 {
		level = 1
	}
	// Guardian::UpdateStats owner arms (StatSystem.cpp:1202-1257) folded
	// through Guardian::UpdateMaxHealth / UpdateMaxPower
	// (StatSystem.cpp:1337-1379): the pet's max health and mana grow with
	// the owner's stamina / intellect. The entry is captured before the
	// hunter-pet pet_levelstats remap below — the ghoul and health
	// multiplicator arms key on the real creature entry. Go applies these
	// at the pet stat-compute points (summon, level-up); C++ also
	// recomputes on owner stat changes, which Go has no hook for.
	healthBonus := s.petOwnerStaminaHealthBonus(entry, petType, petNumber)
	manaBonus := s.petOwnerIntellectManaBonus(entry)
	statEntry := entry
	if petType == 1 {
		statEntry = 1
	}
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && statEntry != 0 {
		var hp, mana int64
		err := s.server.WorldStore.DB.QueryRowContext(ctx,
			"SELECT hp, mana FROM pet_levelstats WHERE creature_entry = ? AND level = ?", statEntry, level).Scan(&hp, &mana)
		if err == nil && hp > 0 {
			maxHP := uint32(hp) + healthBonus
			maxMP := uint32(mana) + manaBonus
			return maxHP, maxHP, maxMP, maxMP
		}
	}
	hp := level*35 + healthBonus
	if hp < 100 {
		hp = 100
	}
	mana := level*30 + manaBonus
	if mana < 80 {
		mana = 80
	}
	return hp, hp, mana, mana
}

// petHealthMultiplicator mirrors the entry switch in
// Guardian::UpdateMaxHealth (StatSystem.cpp:1339-1350).
func petHealthMultiplicator(entry uint32) float64 {
	switch entry {
	case 416: // ENTRY_IMP
		return 8.4
	case 1860: // ENTRY_VOIDWALKER
		return 11.0
	case 1863: // ENTRY_SUCCUBUS
		return 9.1
	case 417: // ENTRY_FELHUNTER
		return 9.5
	case 17252: // ENTRY_FELGUARD
		return 11.0
	case 28017: // ENTRY_BLOODWORM
		return 1.0
	default:
		return 10.0
	}
}

// petOwnerStaminaScale is the owner-stamina scale arm of
// Guardian::UpdateStats(STAT_STAMINA) (StatSystem.cpp:1225-1249) without the
// health multiplicator: the raw ownersBonus added to the pet's UNIT_FIELD_STAT2
// by SetStat (StatSystem.cpp:1268). Ghouls and risen allies scale at 0.3
// (+ Ravenous Dead and Glyph of the Ghoul), warlock pets at 0.75, and
// everything else at 0.45 (+ Wild Hunt rank 1/2 effect-0 for hunter pets).
func (s *session) petOwnerStaminaScale(entry uint32, petType uint8, petNumber uint32) float64 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	var mod float64
	switch {
	case entry == 26125 || entry == 30230:
		// IsPetGhoul / IsRisenAlly (TemporarySummon.h:81-82,
		// StatSystem.cpp:1204-1218).
		mod = 0.3
		if pct := s.ravenousDeadStatMod(); pct != 0 {
			mod *= 1 + float64(pct)/100
		}
		if amt, ok := s.auraEffectAmount(58686, 0); ok {
			mod += float64(amt) / 100
		}
	case s.player.Class == 9 && petType == petTypeSummon:
		// Warlock pets (StatSystem.cpp:1227-1230): 75% of owner stamina.
		// C++ gates on IsPet(); Go's warlock character pets are demons.
		mod = 0.75
	default:
		mod = 0.45
		if petType == petTypeHunter && petNumber != 0 && s.server.Data != nil {
			// Wild Hunt (StatSystem.cpp:1234-1243): rank 1 (62758)
			// checked first, then rank 2 (62762); the stamina arm reads
			// EFFECT_0 (the bonus-damage arm reads EFFECT_1).
			if talent := s.petWildHuntTalent(petNumber); talent != 0 {
				if tspell, found, terr := s.server.Data.Spell(talent); terr == nil && found && len(tspell.Effects) > 0 {
					mod *= 1 + float64(tspell.Effects[0].CalcValue())/100
				}
			}
		}
	}
	return mod
}

// petOwnerStaminaHealthBonus mirrors the owner-stamina arm of
// Guardian::UpdateStats(STAT_STAMINA) (StatSystem.cpp:1225-1249) folded
// through Guardian::UpdateMaxHealth (StatSystem.cpp:1352-1356): the pet's max
// health gains ownersBonus * multiplicator.
func (s *session) petOwnerStaminaHealthBonus(entry uint32, petType uint8, petNumber uint32) uint32 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	var stamina uint32
	if len(s.player.Stats) > 2 {
		stamina = s.player.Stats[2]
	}
	return uint32(float64(stamina) * s.petOwnerStaminaScale(entry, petType, petNumber) * petHealthMultiplicator(entry))
}

// petOwnerIntellectScale is the intellect arm of Guardian::UpdateStats
// (StatSystem.cpp:1251-1257) without the mana multiplicator: the raw
// ownersBonus added to the pet's UNIT_FIELD_STAT3 by SetStat
// (StatSystem.cpp:1268). Warlock and mage pets gain 30% of the owner's
// intellect.
func (s *session) petOwnerIntellectScale() float64 {
	if s == nil || s.player == nil {
		return 0
	}
	if s.player.Class != 9 && s.player.Class != 8 {
		return 0
	}
	return 0.3
}

// petOwnerIntellectManaBonus mirrors the intellect arm of
// Guardian::UpdateStats (StatSystem.cpp:1251-1257) folded through
// Guardian::UpdateMaxPower (StatSystem.cpp:1359-1379): warlock and mage pets
// gain 30% of the owner's intellect, times the entry mana multiplicator
// (imp 4.95, voidwalker/succubus/felhunter/felguard 11.5, default 15.0) —
// the owner bonus flows through the same multiplicator as the aura feed in
// C++, not a flat 15.0.
func (s *session) petOwnerIntellectManaBonus(entry uint32) uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	var intellect uint32
	if len(s.player.Stats) > 3 {
		intellect = s.player.Stats[3]
	}
	return uint32(float64(intellect) * s.petOwnerIntellectScale() * petManaMultiplicator(entry))
}

// petGuardianFieldStats mirrors the UNIT_FIELD_STAT0..4 writes in
// Guardian::UpdateStats (StatSystem.cpp:1266-1282): SetStat(stat) stores
// int32(GetTotalStatValue(stat) + ownersBonus) — the aura-fed value, not the
// raw DB create-stat the broadcasts carried before. GetTotalStatValue(stat)
// (Unit.cpp: GetTotalStatValue = ((flat BASE_VALUE + createStat) * BASE_PCT +
// aura-29 flat TOTAL_VALUE) * aura-137 pct TOTAL_PCT) is the petGuardianStat
// fold (SpellAuraEffects.cpp:3379-3413, 3525-3566), fed into SetStat on every
// aura apply/remove through HandleStatFlatModifier/SetStatPctModifier ->
// UpdateUnitMod -> UpdateStats (Unit.cpp:9304-9310). The owner bonus arms are
// stamina (ghoul/risen 0.3 + Ravenous Dead + Glyph of the Ghoul, warlock pet
// 0.75, else 0.45 + Wild Hunt; StatSystem.cpp:1225-1249) and intellect
// (warlock/mage pet 0.30; :1251-1257). Strength, agility and spirit carry no
// owner bonus (the ghoul strength arm is commented out in the C++; agility
// and spirit have none). motion.Stats stays the raw DB create-stat (the
// petGuardianStat base); these fed values go to the UNIT_FIELD_STAT0+index
// broadcasts at the pet stat-compute points (summon, level-up, owner-stat
// change). Like the C++ int32 cast, negative totals wrap into uint32 bits.
func (s *session) petGuardianFieldStats(motion *creatureMotion, entry uint32, petType uint8, petNumber uint32) [5]uint32 {
	var out [5]uint32
	if s == nil || s.server == nil || motion == nil {
		return out
	}
	key := creatureAuraKeyForMotion(motion)
	var ownerStamina, ownerIntellect uint32
	if s.player != nil && len(s.player.Stats) > 3 {
		ownerStamina, ownerIntellect = s.player.Stats[2], s.player.Stats[3]
	}
	staminaBonus := float64(ownerStamina) * s.petOwnerStaminaScale(entry, petType, petNumber)
	intellectBonus := float64(ownerIntellect) * s.petOwnerIntellectScale()
	for stat := 0; stat < 5; stat++ {
		v := s.server.petGuardianStat(key, motion, stat)
		switch stat {
		case 2:
			v += staminaBonus
		case 3:
			v += intellectBonus
		}
		out[stat] = uint32(int32(v))
	}
	return out
}

// ravenousDeadStatMod mirrors the Ravenous Dead arm of the ghoul stat scale
// (StatSystem.cpp:1210-1214): the owner's SPELL_AURA_MOD_TOTAL_STAT_PERCENTAGE
// (137) Death-Knight-family aura with SpellIconID 3010 at effect 0 carries
// the talent, whose effect-1 CalcValue edits the scale via AddPct.
func (s *session) ravenousDeadStatMod() int32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask&1 == 0 {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found || auraSpell.SpellFamilyName != spellFamilyDeathKnight || auraSpell.SpellIconID != 3010 {
			continue
		}
		if len(auraSpell.Effects) == 0 || auraSpell.Effects[0].Aura != spellAuraModTotalStatPct {
			continue
		}
		if len(auraSpell.Effects) > 1 {
			return auraSpell.Effects[1].CalcValue()
		}
		return 0
	}
	return 0
}

func (s *session) getPetAttributes(ctx context.Context, entry, level uint32, petType uint8) [5]uint32 {
	attributes := [5]uint32{22, 22, 25, 28, 27}
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return attributes
	}
	if petType == 1 {
		entry = 1
	}
	var strength, agility, stamina, intellect, spirit int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT str, agi, sta, inte, spi FROM pet_levelstats WHERE creature_entry = ? AND level = ?", entry, level).Scan(&strength, &agility, &stamina, &intellect, &spirit); err == nil && strength >= 0 && agility >= 0 && stamina >= 0 && intellect >= 0 && spirit >= 0 {
		return [5]uint32{uint32(strength), uint32(agility), uint32(stamina), uint32(intellect), uint32(spirit)}
	}
	return attributes
}

func buildPetUpdate(petGUID uint64, petNumber, entry uint32, level uint32, modelID uint32, curHealth uint32, maxHealth uint32, curMana uint32, maxMana uint32, attributes [5]uint32, curHappiness uint32, ownerGUID uint64, faction uint32, petType uint8, createdBySpell, petExperience, petNextLevelXP uint32, boundingRadius, combatReach, scale, x, y, z, o float32, apBase, apMods uint32, apMult, minDmg, maxDmg float32) []byte {
	values := make([]uint32, creatureValuesCount)
	values[0] = uint32(petGUID)
	values[1] = uint32(petGUID >> 32)
	values[objectFieldType] = creatureTypeMask
	values[objectFieldEntry] = entry
	values[objectFieldScale] = math.Float32bits(scale)
	values[unitFieldHealth] = curHealth
	values[unitFieldMaxHealth] = maxHealth
	values[unitFieldPower1] = curMana
	values[unitFieldMaxPower1] = maxMana
	values[unitFieldBaseMana] = maxMana
	for index, value := range attributes {
		values[unitFieldStat0+index] = value
	}
	values[unitFieldLevel] = maxUint32(level, 1)
	values[unitFieldFaction] = faction
	values[unitFieldFlags] = unitFlagPlayerControlled
	values[unitFieldFlags2] = unitFlag2RegeneratePower
	values[unitModCastSpeed] = math.Float32bits(1)
	values[unitFieldHoverHeight] = math.Float32bits(1)
	values[unitFieldAttackTime] = 2000
	values[unitFieldAttackTimeOffhand] = 2000
	// Guardian::UpdateAttackPowerAndDamage (StatSystem.cpp:1468-1476):
	// UNIT_FIELD_ATTACK_POWER = (int32)base_attPower,
	// UNIT_FIELD_ATTACK_POWER_MODS = (int32)attPowerMod,
	// UNIT_FIELD_ATTACK_POWER_MULTIPLIER = attPowerMultiplier.
	values[unitFieldAttackPower] = apBase
	values[unitFieldAttackPowerMods] = apMods
	values[unitFieldAttackPowerMultiplier] = math.Float32bits(apMult)
	// Guardian::UpdateDamagePhysical (StatSystem.cpp:1479-1554) writes the
	// pet's weapon-damage range to UNIT_FIELD_MINDAMAGE/MAXDAMAGE at
	// stat-update time.
	values[unitFieldMinDamage] = math.Float32bits(minDmg)
	values[unitFieldMaxDamage] = math.Float32bits(maxDmg)
	values[unitFieldBoundingRadius] = math.Float32bits(boundingRadius)
	values[unitFieldCombatReach] = math.Float32bits(combatReach)
	values[unitFieldDisplayID] = modelID
	values[unitFieldNativeDisplayID] = modelID
	values[unitFieldCreatedBySpell] = createdBySpell
	values[unitFieldSummonedBy] = uint32(ownerGUID)
	values[unitFieldSummonedBy+1] = uint32(ownerGUID >> 32)
	values[unitFieldCreatedBy] = uint32(ownerGUID)
	values[unitFieldCreatedBy+1] = uint32(ownerGUID >> 32)
	values[unitFieldPetExperience] = petExperience
	petClass, powerType := uint32(8), uint32(0)
	if petType == 1 {
		petClass, powerType = 1, 2
		values[unitFieldPower1+2] = petFocusMax
		values[unitFieldMaxPower1+2] = petFocusMax
		values[unitFieldPower1+4] = curHappiness
		values[unitFieldMaxPower1+4] = petHappinessMax
	}
	// Pet::InitStatsForLevel (Pet.cpp:930-939): DK ghouls and risen allies
	// display POWER_ENERGY, full at summon (SetFullPower(POWER_ENERGY)).
	if entry == 26125 || entry == 30230 {
		powerType = petPowerTypeEnergy
		values[unitFieldPower1+petPowerTypeEnergy] = petEnergyMax
		values[unitFieldMaxPower1+petPowerTypeEnergy] = petEnergyMax
	}
	values[unitFieldBytes0] = petClass<<8 | powerType<<24
	values[unitFieldPetNumber] = petNumber
	values[unitFieldPetNameTimestamp] = uint32(time.Now().Unix())
	if petType == 1 {
		values[unitFieldPetNextLevelExp] = petNextLevelXP
	}

	mask := protocol.NewUpdateMask(len(values))
	for index, value := range values {
		if value != 0 {
			_ = mask.Set(index)
		}
	}
	block := protocol.NewBuffer(256)
	block.WriteU8(protocol.UpdateCreateObject2)
	block.WritePackedGUID(petGUID)
	block.WriteU8(3) // Type: Unit
	block.WriteU16(creatureUpdateFlags)
	block.WriteU32(0)
	block.WriteU16(0)
	block.WriteU32(uint32(time.Now().UnixMilli()))
	block.WriteF32(x)
	block.WriteF32(y)
	block.WriteF32(z)
	block.WriteF32(o)
	block.WriteU32(0)
	for _, speed := range []float32{2.5, 7.0, 4.5, 4.722222, 2.5, 7.0, 4.5, 3.141594, 3.14} {
		block.WriteF32(speed)
	}
	block.WriteU8(uint8(mask.BlockCount()))
	mask.AppendTo(block)
	for index, value := range values {
		if mask.Has(index) {
			block.WriteU32(value)
		}
	}
	return block.Bytes()
}

func (s *session) resetPetTalentsAtLogin(ctx context.Context) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player.AtLogin&uint32(atLoginResetPetTalents) == 0 {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if _, err := cdb.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ~16 WHERE guid = ?", s.playerGUID); err != nil {
		s.debug("pet talent reset flag update failed", "account", s.accountName, "error", err)
		return false
	}
	s.player.AtLogin &^= uint32(atLoginResetPetTalents)
	if s.server.Data == nil {
		return true
	}
	petTalents, err := s.server.Data.PetTalentSpells()
	if err != nil {
		s.debug("pet talent spell lookup failed", "account", s.accountName, "error", err)
		return true
	}
	rows, err := cdb.QueryContext(ctx, "SELECT ps.guid, ps.spell FROM pet_spell ps JOIN character_pet cp ON cp.id = ps.guid WHERE cp.owner = ?", s.playerGUID)
	if err != nil {
		s.debug("pet talent reset query failed", "account", s.accountName, "error", err)
		return true
	}
	type petSpell struct{ guid, spell uint32 }
	var removals []petSpell
	for rows.Next() {
		var removal petSpell
		if rows.Scan(&removal.guid, &removal.spell) == nil {
			if _, found := petTalents[removal.spell]; found {
				removals = append(removals, removal)
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		s.debug("pet talent reset rows failed", "account", s.accountName, "error", err)
		return true
	}
	for _, removal := range removals {
		if _, err := cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", removal.guid, removal.spell); err != nil {
			s.debug("pet talent spell removal failed", "account", s.accountName, "pet", removal.guid, "spell", removal.spell, "error", err)
		}
	}
	return true
}

// activePetLearnedTalentSpells returns the active pet's learned talent spell
// IDs (pet_spell rows whose spell is a pet talent per the DBC set). Used as
// the m_usedTalentCount>0 proxy for Player::ResetPetTalents (Player.cpp:9612)
// and its gossip-option visibility gate (Player.cpp:14462).
func (s *session) activePetLearnedTalentSpells(ctx context.Context) []uint32 {
	petNumber := s.activePetNumber()
	if petNumber == 0 || s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil {
		return nil
	}
	petTalents, err := s.server.Data.PetTalentSpells()
	if err != nil {
		s.debug("pet talent spell lookup failed", "account", s.accountName, "error", err)
		return nil
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell FROM pet_spell WHERE guid = ?", petNumber)
	if err != nil {
		s.debug("pet talent spell query failed", "account", s.accountName, "pet", petNumber, "error", err)
		return nil
	}
	defer rows.Close()
	var spells []uint32
	for rows.Next() {
		var spell uint32
		if rows.Scan(&spell) == nil {
			if _, found := petTalents[spell]; found {
				spells = append(spells, spell)
			}
		}
	}
	return spells
}

// activePetIsHunterType reports whether the active pet's stored type is
// HUNTER_PET (PetDefines.h:32) — the getPetType()!=HUNTER_PET arm of the
// GOSSIP_OPTION_UNLEARNPETTALENTS visibility gate (Player.cpp:14462).
func (s *session) activePetIsHunterType(ctx context.Context) bool {
	petNumber := s.activePetNumber()
	if petNumber == 0 || s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	var petType int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT PetType FROM character_pet WHERE id = ? AND owner = ?", petNumber, s.playerGUID).Scan(&petType); err != nil {
		return false
	}
	return petType == int64(petTypeHunter)
}

// activePetSpellCount returns the active pet's total stored spell count — the
// m_spells.size() arm of the GOSSIP_OPTION_UNLEARNPETTALENTS visibility gate
// (Player.cpp:14462). pet_spell rows are the pet's full spell list (template
// spells are written at creation), so COUNT(*) is the Go model of m_spells.
func (s *session) activePetSpellCount(ctx context.Context) int {
	petNumber := s.activePetNumber()
	if petNumber == 0 || s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var count int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM pet_spell WHERE guid = ?", petNumber).Scan(&count); err != nil {
		s.debug("pet spell count query failed", "account", s.accountName, "pet", petNumber, "error", err)
		return 0
	}
	return int(count)
}

// resetActivePetTalents mirrors Player::ResetPetTalents (Player.cpp:9612) +
// Pet::resetTalents (Pet.cpp:1629): the active hunter pet's learned talent
// spells are removed and the pet talent/spell frames resent. No-op when there
// is no active pet, the pet isn't a hunter pet, or it has no learned talent
// spells (the m_usedTalentCount == 0 early return, which sends no client
// packet). The creature-family PetTalentType check has no Go bridge — hunter
// pets always carry a talent type in practice.
func (s *session) resetActivePetTalents(ctx context.Context) {
	petNumber := s.activePetNumber()
	if petNumber == 0 || s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	var petType int64
	if err := cdb.QueryRowContext(ctx, "SELECT PetType FROM character_pet WHERE id = ? AND owner = ?", petNumber, s.playerGUID).Scan(&petType); err != nil || petType != int64(petTypeHunter) {
		return
	}
	talentSpells := s.activePetLearnedTalentSpells(ctx)
	if len(talentSpells) == 0 {
		return
	}
	// Pet::resetTalents (Pet.cpp:1629) clears AT_LOGIN_RESET_PET_TALENTS on
	// every interactive reset (Player::ResetPetTalents -> pet->resetTalents),
	// so a pending login-wipe set by `.reset pettalents` never double-wipes
	// re-learned talents at the next login.
	if _, err := cdb.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ~16 WHERE guid = ?", s.playerGUID); err != nil {
		s.debug("pet talent at-login flag clear failed", "account", s.accountName, "error", err)
	}
	if s.player != nil {
		s.player.AtLogin &^= uint32(atLoginResetPetTalents)
	}
	for _, spell := range talentSpells {
		if _, err := cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petNumber, spell); err != nil {
			s.debug("pet talent spell removal failed", "account", s.accountName, "pet", petNumber, "spell", spell, "error", err)
		}
	}
	// Pet::resetTalents ends with PetSpellInitialize + Player::ResetPetTalents
	// with SendTalentsInfoData(true): resend the pet spell list and the talent
	// frame.
	if petGUID := s.petGUIDForNumber(petNumber); petGUID != 0 {
		s.sendPetSpells(ctx, petNumber, 0, 1)
	}
	_ = s.sendTalentsInfo(true)
}

func ShouldTemporarilyUnsummonSavedPet(health, playerFlags, unitFlags, mountDisplay uint32, temporarySummon bool) bool {
	return temporarySummon || health == 0 || playerFlags&playerFlagGhost != 0 || unitFlags&unitFlagMount != 0 || mountDisplay != 0
}

func (s *session) spawnPet(ctx context.Context, petID uint32, entry uint32, name string, level uint32, modelID uint32, curHealth uint32, maxHealth uint32, curMana uint32, maxMana uint32, reactState uint8) {
	if s.player == nil || petID == 0 {
		return
	}
	if maxHealth > 0 && curHealth > maxHealth {
		curHealth = maxHealth
	}
	if maxMana > 0 && curMana > maxMana {
		curMana = maxMana
	}
	petCounter := s.server.nextPetLowGUID(s.player.Map, s.player.InstanceID)
	if petCounter == 0 {
		return
	}
	petGUID := makePetGUID(petID, petCounter)
	var createdBySpell, petType, petExperience, petHappiness int64
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(CreatedBySpell, 0), COALESCE(PetType, 0), COALESCE(exp, 0), COALESCE(curhappiness, 0) FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID).Scan(&createdBySpell, &petType, &petExperience, &petHappiness)
	}
	var creatureType int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", entry).Scan(&creatureType)
	}
	critter := uint32(creatureType) == creatureTypeCritter
	if !critter {
		s.player.PetGUID = petGUID
		s.player.PetNumber = petID
	} else {
		// Unit.cpp:6085-6087 (SetMinion add-arm): a SUMMON_TYPE_MINIPET summon
		// overwrites the player's critter GUID (UNIT_FIELD_CRITTER). A stale
		// tracked critter is destroy-wired out; C++ leaves the old TempSummon
		// to despawn on its own, Go has no motion registered for critters so
		// the client object is dropped here and its character_pet row removed.
		if old := s.player.CritterGUID; old != 0 && old != petGUID {
			s.destroyCritterObject(old)
			s.deleteCritterRow(ctx, s.player.CritterPetID)
		}
		s.player.CritterGUID = petGUID
		s.player.CritterPetID = petID
	}
	// Unit::SetMinion add-arm (Unit.cpp:6098-6100): a minion created by a
	// cooldown-started-on-event spell parks the owner's infinity (onHold)
	// cooldown. createdBySpell was read from character_pet above; a zero
	// value means no creating spell.
	if createdBySpell > 0 && createdBySpell <= int64(^uint32(0)) && s.server != nil && s.server.Data != nil {
		if minionSpell, found, err := s.server.Data.Spell(uint32(createdBySpell)); err == nil && found && s.server.spellIsCooldownStartedOnEvent(minionSpell) {
			s.startSpellCooldownOnHold(ctx, minionSpell, 0)
		}
	}
	if petHappiness < 0 {
		petHappiness = 0
	} else if petHappiness > int64(petHappinessMax) {
		petHappiness = int64(petHappinessMax)
	}
	petNextLevelXP := uint32(0)
	if petType == 1 && s.server != nil {
		petNextLevelXP = s.server.xpForLevel(ctx, uint32(level)+1) / 20
	}
	if !critter && createdBySpell > 0 && createdBySpell <= int64(^uint32(0)) {
		spellTarget := protocol.SpellTargetData{}
		if s.server != nil && s.server.Data != nil {
			if spellInfo, found, err := s.server.Data.Spell(uint32(createdBySpell)); err == nil && found {
				spellTarget = spellGoPacketTarget(spellInfo, spellTarget)
			}
		}
		packet := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 0, uint32(createdBySpell), spellCastFlagGo, uint32(time.Now().UnixMilli()), nil, nil, spellTarget)
		if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), packet, true); err != nil {
			s.debug("pet summon effect failed", "account", s.accountName, "petID", petID, "spell", createdBySpell, "error", err)
		} else if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), packet, s)
		}
	}

	if modelID == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry != 0 {
		var mID int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT modelid1 FROM creature_template WHERE entry = ?", entry).Scan(&mID); err == nil && mID > 0 {
			modelID = uint32(mID)
		}
	}
	if critter && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry != 0 {
		var displayID int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(modelid1, 0), NULLIF(modelid2, 0), NULLIF(modelid3, 0), NULLIF(modelid4, 0), 0) FROM creature_template WHERE entry = ?", entry).Scan(&displayID); err == nil && displayID > 0 {
			modelID = uint32(displayID)
		}
	}
	if modelID == 0 {
		switch entry {
		case 416:
			modelID = 4449
		case 1860:
			modelID = 1132
		case 1863:
			modelID = 4162
		case 417:
			modelID = 850
		case 17252:
			modelID = 14255
		case 510:
			modelID = 525
		case 26125:
			modelID = 24994
		default:
			modelID = 1132
		}
	}
	petBoundingRadius, petCombatReach := float32(0.306349), float32(1.5)
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && modelID != 0 {
		var boundingRadius, combatReach float64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT BoundingRadius, CombatReach FROM creature_model_info WHERE DisplayID = ?", modelID).Scan(&boundingRadius, &combatReach); err == nil {
			if boundingRadius > 0 {
				petBoundingRadius = float32(boundingRadius)
			}
			if combatReach > 0 {
				petCombatReach = float32(combatReach)
			}
		}
	}

	faction := uint32(35)
	if s.server != nil {
		faction = s.server.raceFaction(s.player.Race)
	}

	var petX, petY, petZ, petO float32
	if s.player != nil {
		petO = s.player.Orientation
		petX = s.player.X + (petCombatReach+petFollowDistance)*float32(math.Cos(float64(petO)+math.Pi/2))
		petY = s.player.Y + (petCombatReach+petFollowDistance)*float32(math.Sin(float64(petO)+math.Pi/2))
		petZ = s.player.Z
	}

	var updateBlock []byte
	if critter {
		stats := s.server.loadCreatureStats(ctx, entry)
		var npcFlags, dynamicFlags, rangeAttack int64
		var scale, hoverHeight, walkSpeed, runSpeed float64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(npcflag, 0), COALESCE(dynamicflags, 0), COALESCE(scale, 1), COALESCE(HoverHeight, 1), COALESCE(speed_walk, 1), COALESCE(speed_run, 1), COALESCE(RangeAttackTime, 2000) FROM creature_template WHERE entry = ?", entry).Scan(&npcFlags, &dynamicFlags, &scale, &hoverHeight, &walkSpeed, &runSpeed, &rangeAttack)
		}
		// Unit::SetMinion (Unit.cpp:6053-6057) sets
		// UNIT_FLAG_PLAYER_CONTROLLED on every player-owned minion;
		// SUMMON_TYPE_MINIPET summons are Minions, so vanity critters
		// broadcast it too. Non-critter pets hardcode it (pet_cooldowns.go)
		// and charmed creatures get it (creaturemotion.go) — only the
		// critter branch missed the OR.
		updateBlock = buildCreatureUpdate(ctx, creatureSpawn{GUID: uint32(petGUID), RawGUID: petGUID, Entry: entry, Map: s.player.Map, X: petX, Y: petY, Z: petZ, Orientation: petO, Model: modelID, Faction: faction, NPCFlags: uint32(npcFlags), UnitFlags: stats.UnitFlags | unitFlagPlayerControlled, UnitFlags2: unitFlag2RegeneratePower, DynamicFlags: uint32(dynamicFlags), CreatedBySpell: uint32(createdBySpell), Level: stats.Level, Health: stats.Health, MaxHealth: stats.MaxHealth, Scale: float32(scale), HoverHeight: float32(hoverHeight), BoundingRadius: petBoundingRadius, CombatReach: petCombatReach, WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: stats.AttackTime, RangedAttack: uint32(rangeAttack)}, s)
	} else {
		attributes := s.getPetAttributes(ctx, entry, level, uint8(petType))
		// The UNIT_FIELD_STAT0+index broadcast carries the aura-fed stats
		// (Guardian::UpdateStats -> SetStat); motion.Stats keeps the raw DB
		// create-stats as the petGuardianStat base.
		fieldStats := attributes
		s.registerPetMotion(ctx, petGUID, petID, uint8(petType), entry, level, faction, curHealth, maxHealth, curMana, maxMana, attributes, uint32(petHappiness), uint32(petExperience), petNextLevelXP, uint8(reactState), petCombatReach, petX, petY, petZ, petO)
		// Pet::InitStatsForLevel (Pet.cpp:885-890): base armor and the
		// native object scale are set from the pet's level at stat time.
		// The stored armor is the flat BASE_VALUE only — agility*2, the
		// aura flats and the 35% owner bonus fold live in petFullArmor
		// (Guardian::UpdateArmor, StatSystem.cpp:1316-1330).
		petArmor := s.petBaseArmor(ctx, entry, level, uint8(petType))
		petScale := s.petNativeObjectScale(ctx, entry, level, uint8(petType))
		var ownerArmor uint32
		if s.player != nil {
			ownerArmor = s.player.Armor
		}
		ownerArmorBonus := petOwnerArmorValue(uint8(petType), ownerArmor)
		var apBase, apMods uint32
		var apMult float32
		var minDmg, maxDmg float32
		s.server.motionMu.Lock()
		if pm := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, petGUID); pm != nil && pm.PetID == petID {
			pm.Armor = petArmor
			pm.PetOwnerArmorBonus = ownerArmorBonus
			// Guardian::UpdateStats (StatSystem.cpp:1266-1282) writes the
			// aura-fed stats to UNIT_FIELD_STAT0+index at summon via
			// UpdateAllStats — the broadcast carries the fed values, not
			// the raw DB create-stats (pet auras load right after, so the
			// aura feed is empty here; the owner bonus is not).
			fieldStats = s.petGuardianFieldStats(pm, entry, uint8(petType), uint32(petID))
			// Guardian::UpdateAttackPowerAndDamage (StatSystem.cpp:1468-1476)
			// writes the AP unit fields at stat-update time from the same
			// composition the melee path recomputes lazily.
			_, base, flat, mult := s.server.petGuardianAttackPower(s, pm)
			apBase, apMods, apMult = uint32(int32(base)), uint32(flat), float32(mult)
			// Guardian::UpdateDamagePhysical (StatSystem.cpp:1479-1554)
			// writes UNIT_FIELD_MINDAMAGE/MAXDAMAGE at the same stat-update
			// time.
			rangeMin, rangeMax := s.server.petPhysicalDamageRange(s, pm)
			minDmg, maxDmg = float32(rangeMin), float32(rangeMax)
			// Spell::EffectSummonType (SpellEffects.cpp:2214-2215) stamps the
			// creating spell's duration on the summon; TempSummon::InitStats
			// (TemporarySummon.cpp:168-173) banks it as m_lifetime/m_timer.
			// Only temporary-duration spells stamp — permanent pets keep a
			// zero lifetime and skip the expiry sweep.
			if createdBySpell > 0 && createdBySpell <= int64(^uint32(0)) {
				if lifetime := s.petSpellSummonLifetime(uint32(createdBySpell)); lifetime > 0 {
					pm.TempSummonLifetime = lifetime
					pm.TempSummonExpiresAt = time.Now().Add(lifetime)
					// TemporarySummon.cpp:175-183: spell-duration guardians
					// stamp m_type = TEMPSUMMON_TIMED_DESPAWN — the timer
					// decrements unconditionally, even in combat.
					pm.TempSummonType = 2
				}
			}
		}
		s.server.motionMu.Unlock()
		updateBlock = buildPetUpdate(petGUID, petID, entry, level, modelID, curHealth, maxHealth, curMana, maxMana, fieldStats, uint32(petHappiness), s.playerGUID, faction, uint8(petType), uint32(createdBySpell), uint32(petExperience), petNextLevelXP, petBoundingRadius, petCombatReach, petScale, petX, petY, petZ, petO, apBase, apMods, apMult, minDmg, maxDmg)
	}
	updates := protocol.NewUpdateData()
	updates.AddUpdateBlock(updateBlock)
	if packet, err := updates.BuildPacket(0); err == nil && packet != nil {
		_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToNearby(packet.Opcode, packet.Payload.Bytes(), s)
		}
	}

	if critter {
		s.debug("critter added to map", "account", s.accountName, "petID", petID, "entry", entry, "name", name)
		return
	}
	s.sendPlayerUpdate()
	s.loadPetAuras(ctx, petID, petGUID)
	s.applyOwnerPetAuras(ctx, entry, petGUID)
	if !s.updatePetOnLevelUp(ctx) {
		s.sendPetSpells(ctx, petID, entry, reactState)
	}

	s.debug("pet spawned", "account", s.accountName, "petID", petID, "entry", entry, "name", name, "level", level)
}

func (s *session) loadPetAuras(ctx context.Context, petID uint32, petGUID uint64) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil {
		return
	}
	fullState := true
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT casterGuid, spell, effectMask, recalculateMask, stackCount,
		amount0, amount1, amount2, base_amount0, base_amount1, base_amount2, maxDuration, remainTime, remainCharges, critChance, applyResilience
		FROM pet_aura WHERE guid = ? ORDER BY spell`, petID)
	if err != nil {
		if errorsMissingAuraTable(err) {
			fullState = false
			rows, err = s.server.CharactersStore.DB.QueryContext(ctx, `SELECT casterGuid, spell, effectMask, stackCount, amount0, maxDuration, remainTime, remainCharges
				FROM pet_aura WHERE guid = ? ORDER BY spell`, petID)
			if err != nil {
				return
			}
		} else {
			return
		}
	}
	defer rows.Close()
	petKey := creatureAuraKey{Map: s.player.Map, InstanceID: s.player.InstanceID, GUID: petGUID}
	loaded := make([]*activeAura, 0)
	var statAuraSpellID uint32
	var apAuraSpellID uint32
	var armorAuraSpellID uint32
	var dmgAuraSpellID uint32
	offlineMs := int64(0)
	var petSaveTime int64
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(savetime, 0) FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, petID).Scan(&petSaveTime)
	now := time.Now().Unix()
	if petSaveTime > 0 && now > petSaveTime {
		offlineMs = (now - petSaveTime) * 1000
	}
	s.server.auraMu.Lock()
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
	}
	if s.server.creatureAuras[petKey] == nil {
		s.server.creatureAuras[petKey] = make(map[uint32]struct{})
	}
	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[creatureAuraKey]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[petKey] == nil {
		s.server.activeCreatureAuras[petKey] = make(map[uint32]*activeAura)
	}
	for rows.Next() {
		var casterValue any
		var spellID, effectMask, recalculateMask, stackCount, maxDuration, remainTime, remainCharges int64
		var amounts, baseAmounts [3]int64
		var critChance float64
		var applyResilience bool
		var scanErr error
		if fullState {
			scanErr = rows.Scan(&casterValue, &spellID, &effectMask, &recalculateMask, &stackCount, &amounts[0], &amounts[1], &amounts[2], &baseAmounts[0], &baseAmounts[1], &baseAmounts[2], &maxDuration, &remainTime, &remainCharges, &critChance, &applyResilience)
		} else {
			scanErr = rows.Scan(&casterValue, &spellID, &effectMask, &stackCount, &amounts[0], &maxDuration, &remainTime, &remainCharges)
		}
		if scanErr != nil || spellID <= 0 || spellID > int64(^uint32(0)) {
			continue
		}
		casterGUID := uint64(0)
		if casterValue != nil {
			var casterOK bool
			casterGUID, casterOK = ParseAuraGUID(casterValue)
			if !casterOK {
				continue
			}
		}
		spell, found, spellErr := s.server.Data.Spell(uint32(spellID))
		if spellErr != nil || !found {
			continue
		}
		if casterGUID == 0 {
			casterGUID = petGUID
		}
		aura := &activeAura{SpellID: uint32(spellID), CasterGUID: casterGUID, TargetGUID: petGUID, TargetKey: petKey, EffectMask: uint8(effectMask) & 0x07, RecalculateMask: uint8(recalculateMask), CritChance: float32(critChance), ApplyResilience: applyResilience, Slot: uint8(len(s.server.activeCreatureAuras[petKey]) % 64), Positive: true, CasterLevel: s.auraStoredCasterLevel(casterGUID, spell), StoredCasterLevel: s.auraStoredCasterLevel(casterGUID, spell)}
		for index := range amounts {
			aura.Amounts[index] = int32(amounts[index])
			aura.BaseAmounts[index] = int32(baseAmounts[index])
		}
		aura.HideDuration = spell.AttributesEx5&spellAttr5HideDuration != 0
		if maxDuration > 0 {
			aura.DurationMs = clampAuraDuration(maxDuration)
		}
		if remainTime > 0 {
			aura.RemainingMs = clampAuraDuration(remainTime)
		}
		if stackCount > 0 {
			aura.StackCount = uint8(stackCount)
		}
		if remainCharges > 0 {
			aura.RemainingCharges = uint8(remainCharges)
		}
		for index, effect := range spell.Effects {
			if effect.Effect == 0 || effect.Aura == 0 || effectMask&(1<<uint(index)) == 0 {
				continue
			}
			aura.AuraType = effect.Aura
			aura.MiscValue = effect.MiscValue
			if aura.Amounts[index] > 0 {
				aura.Amount = uint32(aura.Amounts[index])
			}
			aura.PeriodMs = effect.AuraPeriod
			if aura.Amount == 0 && effect.BasePoints >= 0 {
				aura.Amount = uint32(effect.BasePoints)
			}
			aura.Positive = !isHarmfulAura(effect.Aura)
			aura.StackAmount = spell.StackAmount
			break
		}
		if spell.ProcCharges > 0 {
			if aura.RemainingCharges == 0 || aura.RemainingCharges > uint8(spell.ProcCharges) {
				aura.RemainingCharges = uint8(spell.ProcCharges)
			}
		} else {
			aura.RemainingCharges = 0
		}
		if aura.AuraType == 0 {
			continue
		}
		if aura.EffectMask == 0 {
			aura.EffectMask = 0x01
		}
		fadesWhileOffline := spell.AttributesEx4&0x00000004 != 0 && aura.SpellID != 15007
		if (!aura.Positive || fadesWhileOffline) && aura.RemainingMs > 0 && offlineMs > 0 {
			if offlineMs >= int64(aura.RemainingMs) {
				continue
			}
			aura.RemainingMs -= uint32(offlineMs)
		}
		if aura.DurationMs > 0 && aura.RemainingMs > aura.DurationMs {
			aura.RemainingMs = aura.DurationMs
		}
		s.server.creatureAuras[petKey][aura.SpellID] = struct{}{}
		s.server.activeCreatureAuras[petKey][aura.SpellID] = aura
		loaded = append(loaded, aura)
		if spellHasStatModAura(spell) {
			statAuraSpellID = uint32(spellID)
		}
		if spellHasPetAttackPowerAura(spell) {
			apAuraSpellID = uint32(spellID)
		}
		if spellHasCreatureArmorAura(spell) {
			armorAuraSpellID = uint32(spellID)
		}
		if spellHasPetWeaponDamageAura(spell) {
			dmgAuraSpellID = uint32(spellID)
		}
	}
	s.server.auraMu.Unlock()
	if len(loaded) == 0 {
		return
	}
	// Guardian::UpdateStats on pet aura load (StatSystem.cpp:1266-1282): the
	// DB-restored auras apply through the same UpdateStats path as live
	// applies in C++ (each _AddAura runs the stat handlers), so a restored
	// stamina/intellect aura must recompute the pet's aura-fed stats and
	// maxima — the summon broadcast above went out aura-free.
	if statAuraSpellID != 0 {
		s.server.recomputePetDerivedOnStatAuraChange(petKey, statAuraSpellID, true, nil)
	}
	// Guardian::UpdateAttackPowerAndDamage on the same load
	// (StatSystem.cpp:1274 / Unit.cpp:9336-9337): a restored strength, 99
	// or 166 aura recomputes the pet's AP unit fields — the summon
	// broadcast above went out aura-free.
	if apAuraSpellID != 0 {
		s.server.recomputePetAttackPowerOnAuraChange(petKey, apAuraSpellID)
	}
	// Guardian::UpdateArmor / Creature::UpdateArmor on the same load
	// (StatSystem.cpp:1317-1330, 1018-1022): a restored direct armor aura
	// rewrites UNIT_FIELD_RESISTANCES (armor) — the summon broadcast above
	// went out aura-free.
	if armorAuraSpellID != 0 {
		s.server.recomputeCreatureArmorOnAuraChange(petKey, armorAuraSpellID)
	}
	// Guardian::UpdateDamagePhysical on the same load (Unit.cpp:9335,
	// StatSystem.cpp:1479-1554): a restored 13/79 NORMAL-misc aura rewrites
	// UNIT_FIELD_MINDAMAGE/MAXDAMAGE — the summon broadcast above went out
	// aura-free.
	if dmgAuraSpellID != 0 {
		s.server.recomputePetDamageFieldsOnAuraChange(petKey, dmgAuraSpellID)
	}
	records := make([]protocol.AuraUpdateRecord, 0, len(loaded))
	for _, aura := range loaded {
		stackCount := aura.StackCount
		if aura.StackAmount == 0 {
			stackCount = aura.RemainingCharges
		}
		maxDuration, duration := aura.DurationMs, aura.RemainingMs
		if aura.HideDuration {
			maxDuration, duration = 0, 0
		}
		records = append(records, protocol.AuraUpdateRecord{CasterGUID: aura.CasterGUID, Slot: aura.Slot, SpellID: aura.SpellID, EffectMask: aura.EffectMask, Positive: aura.Positive, MaxDurationMs: maxDuration, DurationMs: duration, CasterLevel: aura.StoredCasterLevel, StackCount: stackCount})
		if aura.PeriodMs > 0 {
			s.scheduleCreaturePeriodicTick(aura, aura.PeriodMs)
		}
		if aura.DurationMs > 0 && aura.DurationMs < 18000000 {
			aura.Timer = time.AfterFunc(time.Duration(aura.RemainingMs)*time.Millisecond, func(spellID uint32, slot uint8, key creatureAuraKey) func() {
				return func() { s.expireCreatureAura(key, spellID, slot) }
			}(aura.SpellID, aura.Slot, petKey))
		}
	}
	for _, record := range records {
		packet := protocol.BuildAuraUpdateWithStackEffect(petGUID, record.CasterGUID, record.Slot, record.SpellID, false, record.Positive, record.MaxDurationMs, record.DurationMs, record.CasterLevel, record.StackCount, record.EffectMask)
		_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), packet, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), packet, s)
	}
}

func (s *session) unsummonPet(ctx context.Context, mode uint8) {
	if s.player == nil || s.player.PetGUID == 0 {
		return
	}
	petGUID := s.player.PetGUID
	petID := s.petNumberForGUID(petGUID)

	if petID != 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
		if err == nil {
			err = s.savePetState(ctx, tx, mode)
			if err == nil {
				err = tx.Commit()
			}
			if err != nil {
				_ = tx.Rollback()
			}
		}
		if err != nil {
			s.debug("pet state save failed", "account", s.accountName, "petID", petID, "mode", mode, "error", err)
		}
	}
	if s.server != nil {
		s.removeOwnerPetAuraSourcesOnPetChange(ctx, petGUID)
		s.server.clearCreatureAuras(creatureAuraKey{Map: s.player.Map, InstanceID: s.player.InstanceID, GUID: petGUID})
	}

	s.sendDestroyObject(petGUID, false)
	if s.server != nil {
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(petGUID)
		destroyBuf.WriteU8(0)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), s)
	}

	buf := protocol.NewBuffer(8)
	buf.WriteU64(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)

	// Pet::Remove (Pet.cpp:717) -> CombatStop (Unit.cpp:9845) ->
	// EndAllPvECombat (CombatManager.cpp:344-350) ->
	// RemoveMeFromThreatLists (ThreatManager.cpp:690-697): the dismissed
	// pet drops as a victim from every other unit's threat table
	// immediately, with SMSG_THREAT_REMOVE and top-victim reselect — the
	// motion delete below is not enough (stale victim until next tick).
	if s.server != nil && petGUID != 0 {
		s.server.removeThreatVictimFromAllLists(s.player.Map, s.player.InstanceID, petGUID)
	}
	if s.server != nil {
		s.server.motionMu.Lock()
		delete(s.server.motionMapLocked(s.player.Map, s.player.InstanceID), petGUID)
		s.server.motionMu.Unlock()
	}

	s.player.PetGUID = 0
	s.player.PetNumber = 0
	s.sendPlayerUpdate()
	// Unit::SetMinion remove-arm (Unit.cpp:6137-6139): removing a minion
	// created by a cooldown-started-on-event spell releases the owner's
	// infinity cooldown via SendCooldownEvent.
	if petID != 0 && s.server != nil && s.server.Data != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var createdBySpell int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(CreatedBySpell, 0) FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID).Scan(&createdBySpell); err == nil && createdBySpell > 0 && createdBySpell <= int64(^uint32(0)) {
			if minionSpell, found, serr := s.server.Data.Spell(uint32(createdBySpell)); serr == nil && found && s.server.spellIsCooldownStartedOnEvent(minionSpell) {
				s.sendSpellCooldownEvent(minionSpell)
			}
		}
	}
	s.debug("pet unsummoned", "account", s.accountName, "petID", petID, "mode", mode)
}

func (s *session) temporarilyUnsummonPet(ctx context.Context) {
	if s == nil || s.player == nil || s.player.PetGUID == 0 {
		return
	}
	petNumber := s.activePetNumber()
	if petNumber == 0 {
		return
	}
	temporaryPet := false
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var createdBySpell int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(CreatedBySpell, 0) FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, petNumber).Scan(&createdBySpell); err == nil && createdBySpell > 0 && createdBySpell <= int64(^uint32(0)) {
			temporaryPet = s.petSpellHasTemporaryDuration(uint32(createdBySpell))
		}
	}
	if temporaryPet {
		s.temporaryUnsummonedPetNumber = 0
	} else {
		s.temporaryUnsummonedPetNumber = petNumber
	}
	s.unsummonPet(ctx, petSaveAsCurrent)
}

func (s *session) petSpellHasTemporaryDuration(spellID uint32) bool {
	if s == nil || s.server == nil || s.server.Data == nil || spellID == 0 {
		return false
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found || spell.DurationIndex == 0 {
		return false
	}
	duration, found, err := s.server.Data.SpellDuration(spell.DurationIndex, 1)
	return err == nil && found && duration > 0
}

// petSpellSummonLifetime returns the TempSummon::m_lifetime twin
// (TemporarySummon.cpp:168-173, InitStats) for a spell that summons a
// temporary-duration guardian: the spell's SpellDuration.dbc duration
// (Spell::EffectSummonType, SpellEffects.cpp:2214-2215). Zero for spells
// that do not summon temporary guardians.
func (s *session) petSpellSummonLifetime(spellID uint32) time.Duration {
	if !s.petSpellHasTemporaryDuration(spellID) || s.server == nil || s.server.Data == nil {
		return 0
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return 0
	}
	duration, found, err := s.server.Data.SpellDuration(spell.DurationIndex, 1)
	if err != nil || !found || duration <= 0 {
		return 0
	}
	return time.Duration(duration) * time.Millisecond
}

// activePetIsTemporaryGuardian reports whether the player's active pet is
// a spell-duration guardian (TempSummon twin): C++ keeps such summons on
// owner death (TempSummon::Update, TemporarySummon.cpp:66-170, has no
// owner-death arm; only Totem::Update, Totem.cpp:47-57, unsummons on
// owner death). The motion stamp is the primary source; the
// CreatedBySpell duration discriminator backs it when the motion is not
// registered (e.g. mid-teleport).
func (s *session) activePetIsTemporaryGuardian(ctx context.Context) bool {
	if s == nil || s.player == nil || s.player.PetGUID == 0 || s.server == nil {
		return false
	}
	if motion := s.findCreatureMotion(s.player.PetGUID); motion != nil {
		return motion.TempSummonLifetime > 0
	}
	petID := s.activePetNumber()
	if petID == 0 || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	var createdBySpell int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(CreatedBySpell, 0) FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID).Scan(&createdBySpell); err != nil || createdBySpell <= 0 || createdBySpell > int64(^uint32(0)) {
		return false
	}
	return s.petSpellHasTemporaryDuration(uint32(createdBySpell))
}

// despawnTemporaryGuardian destroys the player's active spell-duration
// guardian without saving it: C++ TempSummons are never persisted, so the
// character_pet row is deleted rather than saved
// (TempSummon::UnSummon, TemporarySummon.cpp:250-304).
func (s *session) despawnTemporaryGuardian(ctx context.Context) {
	if s == nil || s.player == nil || s.player.PetGUID == 0 || s.server == nil {
		return
	}
	if motion := s.findCreatureMotion(s.player.PetGUID); motion != nil {
		s.server.expireTempSummonGuardian(ctx, motion)
		return
	}
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		if petID := s.activePetNumber(); petID != 0 {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ?", petID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid = ?", petID)
			_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_spell_cooldown WHERE guid = ?", petID)
		}
	}
	s.player.PetGUID = 0
	s.player.PetNumber = 0
	s.sendPlayerUpdate()
}

func (s *session) resummonTemporaryPet(ctx context.Context) {
	if s == nil || s.player == nil || s.server == nil || s.temporaryUnsummonedPetNumber == 0 || s.player.PetGUID != 0 {
		return
	}
	if ShouldTemporarilyUnsummonSavedPet(s.player.Health, s.player.PlayerFlags, s.player.UnitFlags, s.player.MountDisplayID, false) {
		return
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		s.temporaryUnsummonedPetNumber = 0
		return
	}
	var petID, entry, modelID, level, curHealth, curMana, petType, reactState, createdBySpell int64
	var name string
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT id, entry, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1), COALESCE(CreatedBySpell, 0) FROM character_pet WHERE owner = ? AND id = ? AND slot = 0", s.playerGUID, s.temporaryUnsummonedPetNumber).Scan(&petID, &entry, &modelID, &level, &name, &curHealth, &curMana, &petType, &reactState, &createdBySpell)
	if err != nil || petID == 0 || entry == 0 || level == 0 {
		s.temporaryUnsummonedPetNumber = 0
		return
	}
	if createdBySpell > 0 && createdBySpell <= int64(^uint32(0)) && s.petSpellHasTemporaryDuration(uint32(createdBySpell)) {
		s.temporaryUnsummonedPetNumber = 0
		return
	}
	petLevel := uint32(level)
	if petType == 0 && s.player.Level > 0 {
		petLevel = uint32(s.player.Level)
	}
	maxHealth, _, maxMana, _ := s.getPetStats(ctx, uint32(entry), petLevel, uint8(petType), uint32(petID))
	if maxHealth == 0 {
		maxHealth = uint32(curHealth)
	}
	if maxMana == 0 {
		maxMana = uint32(curMana)
	}
	petNumber := uint32(petID)
	s.temporaryUnsummonedPetNumber = 0
	s.spawnPet(ctx, petNumber, uint32(entry), name, petLevel, uint32(modelID), uint32(curHealth), maxHealth, uint32(curMana), maxMana, uint8(reactState))
}

func (s *session) sendPetSpells(ctx context.Context, petID uint32, entry uint32, reactState uint8) {
	if s.player == nil || petID == 0 {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		return
	}

	var abdata string
	var family uint16
	if entry == 0 {
		_ = cdb.QueryRowContext(ctx, "SELECT entry FROM character_pet WHERE id = ?", petID).Scan(&entry)
	}
	var petType int64
	if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(PetType, 0) FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID).Scan(&petType); err != nil {
		return
	}
	var creatureType int64
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry != 0 {
		var fam int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT family FROM creature_template WHERE entry = ?", entry).Scan(&fam); err == nil {
			family = uint16(fam)
		}
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", entry).Scan(&creatureType)
	}
	if uint32(creatureType) == creatureTypeCritter {
		return
	}
	permanent := petType == int64(petTypeHunter) || petType == int64(petTypeSummon) && (s.player.Class == 9 && uint32(creatureType) == creatureTypeDemon || s.player.Class == 6 && uint32(creatureType) == creatureTypeUndead)
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(abdata, '') FROM character_pet WHERE id = ?", petID).Scan(&abdata)

	petGUID := s.petGUIDForNumber(petID)
	if petGUID == 0 {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		return
	}
	buf := protocol.NewBuffer(64)
	buf.WriteU64(petGUID)
	buf.WriteU16(family)
	buf.WriteU32(0) // duration (0 = permanent)
	buf.WriteU8(reactState)
	buf.WriteU8(1)  // commandState (1 = FOLLOW)
	buf.WriteU16(0) // flags

	customSlots := [10]uint32{0x07000002, 0x07000001, 0x07000000, 0x01000000, 0x01000000, 0x01000000, 0x01000000, 0x06000002, 0x06000001, 0x06000000}
	if abdata != "" {
		tokens := strings.Fields(abdata)
		if len(tokens) == 20 {
			for i := 0; i < 10; i++ {
				t, typeErr := strconv.ParseUint(tokens[i*2], 10, 8)
				a, actionErr := strconv.ParseUint(tokens[i*2+1], 10, 32)
				if typeErr != nil || actionErr != nil {
					continue
				}
				customSlots[i] = uint32(a) | (uint32(t) << 24)
			}
		}
	}
	for i := range customSlots {
		customSlots[i] = s.normalizePetActionBarSlot(customSlots[i])
	}

	type petSpellInfo struct {
		spellID uint32
		active  uint8
	}
	var allSpells []petSpellInfo
	rows, sErr := cdb.QueryContext(ctx, "SELECT spell, active FROM pet_spell WHERE guid = ? ORDER BY spell", petID)
	if sErr == nil {
		defer rows.Close()
		for rows.Next() {
			var spID, act int64
			if rows.Scan(&spID, &act) == nil && spID > 0 && spID <= int64(^uint32(0)) {
				if s.server.Data != nil {
					_, found, spellErr := s.server.Data.Spell(uint32(spID))
					if spellErr == nil && !found {
						continue
					}
				}
				allSpells = append(allSpells, petSpellInfo{spellID: uint32(spID), active: uint8(act)})
			}
		}
	}

	for _, spell := range allSpells {
		for slot := 3; slot < 7; slot++ {
			if customSlots[slot]&0x00FFFFFF == 0 {
				customSlots[slot] = spell.spellID | (uint32(spell.active) << 24)
				break
			}
		}
	}
	for i := 0; i < 10; i++ {
		buf.WriteU32(customSlots[i])
	}

	// Additional spells list (populates client Spellbook Pet tab!)
	if permanent {
		buf.WriteU8(uint8(len(allSpells)))
		for _, sp := range allSpells {
			actType := uint32(sp.active)
			buf.WriteU32(sp.spellID | (actType << 24))
		}
	} else {
		buf.WriteU8(0)
	}

	type petCooldown struct {
		spell, category  uint32
		end, categoryEnd time.Time
	}
	cooldowns := make([]petCooldown, 0)
	now := time.Now()
	s.server.motionMu.Lock()
	if motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID); motion != nil {
		prunePetSpellCooldowns(motion, now)
		for spellID, end := range motion.SpellCooldowns {
			categoryID := motion.SpellCooldownCategories[spellID]
			cooldowns = append(cooldowns, petCooldown{spell: spellID, category: categoryID, end: end, categoryEnd: motion.SpellCooldownCategoryEnds[spellID]})
		}
	}
	s.server.motionMu.Unlock()
	sort.Slice(cooldowns, func(i, j int) bool { return cooldowns[i].spell < cooldowns[j].spell })
	buf.WriteU8(uint8(len(cooldowns)))
	for _, cooldown := range cooldowns {
		buf.WriteU32(cooldown.spell)
		buf.WriteU16(uint16(cooldown.category))
		cooldownMs := petCooldownMilliseconds(cooldown.end, now)
		categoryMs := petCooldownMilliseconds(cooldown.categoryEnd, now)
		if cooldownMs == 0 {
			buf.WriteU32(0)
			buf.WriteU32(0)
		} else if categoryMs > 0 {
			buf.WriteU32(0)
			buf.WriteU32(categoryMs)
		} else {
			buf.WriteU32(cooldownMs)
			buf.WriteU32(0)
		}
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
}

func (s *session) updatePetOnLevelUp(ctx context.Context) bool {
	if s.player == nil || s.player.PetGUID == 0 {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	petID := s.activePetNumber()
	var entry, currentLevel, petType, experience int64
	err := cdb.QueryRowContext(ctx, "SELECT entry, level, PetType, exp FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID).Scan(&entry, &currentLevel, &petType, &experience)
	if err != nil {
		return false
	}
	targetLevel := PetLevelForOwner(uint8(petType), uint32(currentLevel), uint32(s.player.Level))
	if targetLevel == uint32(currentLevel) {
		return false
	}
	petXP := uint32(experience)
	nextLevelXP := uint32(0)
	if petType == 1 {
		petXP = 0
		nextLevelXP = s.server.xpForLevel(ctx, targetLevel) / 20
	}
	if !s.applyPetLevel(ctx, petID, uint32(entry), uint8(petType), uint32(currentLevel), targetLevel, petXP, nextLevelXP) {
		return false
	}
	s.updatePetLevelSpells(ctx, petID, uint32(entry), uint32(currentLevel), targetLevel, uint8(petType), true)
	return true
}

func (s *session) handleSummonPet(ctx context.Context, spellID uint32, entry uint32) uint64 {
	if s.player == nil {
		return 0
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return 0
	}

	// If entry == 0 (e.g. Hunter Call Pet 883), summon current stabled or existing pet
	if entry == 0 {
		var petID, pEntry, modelID, level, petType, reactState, curHealth, curMana int64
		var petName string
		err := cdb.QueryRowContext(ctx,
			"SELECT id, entry, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? ORDER BY slot ASC LIMIT 1",
			s.playerGUID).Scan(&petID, &pEntry, &modelID, &level, &petName, &curHealth, &curMana, &petType, &reactState)
		if err != nil {
			s.debug("no pet found to call", "account", s.accountName)
			return 0
		}
		if s.player.PetGUID != 0 {
			if s.activePetNumber() == uint32(petID) {
				return 0 // already active
			}
			s.unsummonPet(ctx, petSaveNotInSlot)
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE character_pet SET slot = 0 WHERE id = ? AND owner = ?", petID, s.playerGUID)
		maxHP, _, maxMana, _ := s.getPetStats(ctx, uint32(pEntry), uint32(level), uint8(petType), uint32(petID))
		if maxHP == 0 {
			maxHP = uint32(curHealth)
		}
		if maxMana == 0 {
			maxMana = uint32(curMana)
		}
		s.spawnPet(ctx, uint32(petID), uint32(pEntry), petName, uint32(level), uint32(modelID), uint32(curHealth), maxHP, uint32(curMana), maxMana, uint8(reactState))
		return s.player.PetGUID
	}

	var petID, modelID, level, petType, reactState, curHealth, curMana int64
	var petName string
	err := cdb.QueryRowContext(ctx,
		"SELECT id, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? AND entry = ? LIMIT 1",
		s.playerGUID, entry).Scan(&petID, &modelID, &level, &petName, &curHealth, &curMana, &petType, &reactState)

	if err == nil && petID > 0 {
		if s.player.PetGUID != 0 {
			if s.activePetNumber() == uint32(petID) {
				return 0 // already active
			}
			s.unsummonPet(ctx, petSaveNotInSlot)
		}

		maxHP, _, maxMP, _ := s.getPetStats(ctx, entry, uint32(level), uint8(petType), uint32(petID))
		curHealth = int64(maxHP)
		curMana = int64(maxMP)

		_, _ = cdb.ExecContext(ctx,
			"UPDATE character_pet SET slot = 0, level = ?, curhealth = ?, curmana = ?, savetime = ? WHERE id = ? AND owner = ?",
			level, curHealth, curMana, time.Now().Unix(), petID, s.playerGUID)

		spells := getPetSpellsForLevel(entry, uint32(level))
		for _, sp := range spells {
			_, _ = cdb.ExecContext(ctx, "INSERT OR IGNORE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, sp.id, sp.active)
		}
		s.learnPetFamilyPassives(ctx, uint32(petID), entry)

		s.spawnPet(ctx, uint32(petID), entry, petName, uint32(level), uint32(modelID), uint32(curHealth), maxHP, uint32(curMana), maxMP, uint8(reactState))
		return s.player.PetGUID
	}

	if s.player.PetGUID != 0 {
		s.unsummonPet(ctx, petSaveNotInSlot)
	}

	var maxPetID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM character_pet").Scan(&maxPetID)
	newPetID := uint32(maxPetID + 1)

	petName = s.generatePetName(ctx, entry)
	playerLevel := uint32(s.player.Level)
	if playerLevel == 0 {
		playerLevel = 1
	}
	maxHP, _, maxMP, _ := s.getPetStats(ctx, entry, playerLevel, 0, newPetID)

	var model uint32
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var mID int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT modelid1 FROM creature_template WHERE entry = ?", entry).Scan(&mID)
		model = uint32(mID)
	}

	now := time.Now().Unix()
	_, _ = cdb.ExecContext(ctx,
		"INSERT INTO character_pet (id, entry, owner, modelid, CreatedBySpell, PetType, level, exp, Reactstate, name, renamed, slot, curhealth, curmana, curhappiness, savetime, abdata) VALUES (?, ?, ?, ?, ?, 0, ?, 0, 1, ?, 0, 0, ?, ?, 0, ?, '')",
		newPetID, entry, s.playerGUID, model, spellID, playerLevel, petName, maxHP, maxMP, now)

	spells := getPetSpellsForLevel(entry, playerLevel)
	for _, sp := range spells {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", newPetID, sp.id, sp.active)
	}
	s.learnPetFamilyPassives(ctx, newPetID, entry)

	s.spawnPet(ctx, newPetID, entry, petName, playerLevel, model, maxHP, maxHP, maxMP, maxMP, 1)
	return s.player.PetGUID
}

func (s *session) handleDismissPet(ctx context.Context, spellID uint32) {
	// Spell::EffectDismissPet (SpellEffects.cpp:4211) logs the unsummon of
	// the dismissed pet; capture the GUID before unsummonPet clears it. A
	// second (post-loop spellID 2641) call finds no pet and stays silent.
	var petGUID uint64
	if s.player != nil {
		petGUID = s.player.PetGUID
	}
	s.unsummonPet(ctx, petSaveNotInSlot)
	if petGUID != 0 {
		s.sendUnsummonObjectLog(spellID, petGUID)
	}
}

func (s *session) handleResurrectPet(ctx context.Context, spellID uint32, eff wotlk.SpellEffect) {
	if s.player == nil {
		return
	}
	var dmgSpell wotlk.Spell
	if s.server != nil && s.server.Data != nil {
		if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
			dmgSpell = sp
		}
	}
	damage := eff.CalcValueForLevel(dmgSpell, uint32(s.player.Level))
	if damage < 0 {
		return // SpellEffects.cpp:4761
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}

	var petID, entry, modelID, level, petType, reactState, curMana int64
	var petName string
	err := cdb.QueryRowContext(ctx,
		"SELECT id, entry, modelid, level, COALESCE(PetType, 0), name, COALESCE(Reactstate, 1), COALESCE(curmana, 0) FROM character_pet WHERE owner = ? ORDER BY slot ASC LIMIT 1",
		s.playerGUID).Scan(&petID, &entry, &modelID, &level, &petType, &petName, &reactState, &curMana)
	if err != nil || petID == 0 {
		return
	}

	maxHP, _, maxMP, _ := s.getPetStats(ctx, uint32(entry), uint32(level), uint8(petType), uint32(petID))
	// SpellEffects.cpp:4799: SetHealth(CountPctFromMaxHealth(damage)) — the
	// revive is a percent of max, not a full heal; mana is untouched (C++
	// sets health only).
	reviveHP := maxHP * uint32(damage) / 100
	if maxMP > 0 && uint32(curMana) > maxMP {
		curMana = int64(maxMP)
	}

	petNumber := uint32(petID)
	if s.player.PetGUID != 0 && s.activePetNumber() == petNumber {
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
		if motion != nil && motion.Health > 0 {
			// Pet already alive in the world: nothing to revive
			// (SpellEffects.cpp:4783).
			s.server.motionMu.Unlock()
			return
		}
		if motion != nil {
			// Dead pet still in the world: revive in place
			// (SpellEffects.cpp:4785-4799) — repositioned at the owner
			// (GetClosePoint has no Go bridge; the follow AI re-separates
			// to PET_FOLLOW_DIST on its next step), dynamic flags cleared
			// (UNIT_DYNFLAG_NONE), health set to the revive percent.
			motion.Health = reviveHP
			motion.DynamicFlags = 0
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			motion.X, motion.Y, motion.Z = s.player.X, s.player.Y, s.player.Z
			motion.Orientation = s.player.Orientation
			s.server.motionMu.Unlock()
			_, _ = cdb.ExecContext(ctx,
				"UPDATE character_pet SET slot = 0, curhealth = ?, savetime = ? WHERE id = ? AND owner = ?",
				reviveHP, time.Now().Unix(), petID, s.playerGUID)
			s.server.broadcastCreatureValuesUpdateInInstance(s.player.Map, s.player.InstanceID, s.player.PetGUID, map[int]uint32{
				unitFieldHealth:       reviveHP,
				unitFieldDynamicFlags: 0,
			})
			s.server.broadcastMonsterMoveStopInInstance(s.player.Map, s.player.InstanceID, s.player.PetGUID, motion.X, motion.Y, motion.Z)
			return
		}
		s.server.motionMu.Unlock()
		// PetGUID set but no world motion (despawned): fall through to the
		// DB summon leg below, matching SummonPet.
	}

	_, _ = cdb.ExecContext(ctx,
		"UPDATE character_pet SET slot = 0, curhealth = ?, savetime = ? WHERE id = ? AND owner = ?",
		reviveHP, time.Now().Unix(), petID, s.playerGUID)

	s.spawnPet(ctx, petNumber, uint32(entry), petName, uint32(level), uint32(modelID), reviveHP, maxHP, uint32(curMana), maxMP, uint8(reactState))
}

func (s *session) handleTameCreature(ctx context.Context, spellID uint32, targetGUID uint64) {
	if s.player == nil || s.player.Class != 3 || targetGUID == 0 { // Hunter only
		return
	}
	if s.player.PetGUID != 0 {
		return // already has an active pet
	}
	// Unit::InitTamedPet (Unit.cpp:11122-11125): taming fails when the
	// stable already holds a current or unslotted hunter pet — the old
	// check covered only the live pet.
	if s.hasStabledHunterPet(ctx) {
		return
	}
	// Spell::EffectTameCreature (SpellEffects.cpp:2972-2979): the target must
	// be a non-pet creature.
	if uint16(targetGUID>>48) != 0xF130 {
		return
	}
	var creatureLevel uint32
	if s.server != nil {
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
		if motion != nil {
			if motion.PetID != 0 && motion.OwnerGUID != 0 {
				s.server.motionMu.Unlock()
				return
			}
			creatureLevel = motion.Level
		}
		s.server.motionMu.Unlock()
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}

	targetEntry := uint32((targetGUID >> 24) & 0x00FFFFFF)
	if targetEntry == 0 {
		targetEntry = uint32(targetGUID & 0xFFFFFFFF)
	}

	s.sendDestroyObject(targetGUID, false)
	if s.server != nil {
		// Spell::EffectTameCreature (SpellEffects.cpp:2991):
		// DespawnOrUnsummon removes the creature server-side too — the old
		// code only sent SMSG_DESTROY_OBJECT and left the motion entry
		// re-targetable and ticking.
		s.server.motionMu.Lock()
		delete(s.server.motionMapLocked(s.player.Map, s.player.InstanceID), targetGUID)
		s.server.motionMu.Unlock()
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(targetGUID)
		destroyBuf.WriteU8(0)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), s)
	}

	playerLevel := uint32(s.player.Level)
	// Spell::EffectTameCreature (SpellEffects.cpp:2994): the tamed pet keeps
	// the higher of the wild creature's level and (caster level - 5); an
	// unresolvable creature level keeps the old caster-level behavior.
	petLevel := playerLevel
	if creatureLevel != 0 {
		petLevel = creatureLevel
		if playerLevel >= 5 && petLevel < playerLevel-5 {
			petLevel = playerLevel - 5
		}
	}
	s.createTamedPetFromEntry(ctx, cdb, spellID, targetEntry, petLevel)
}

// createTamedPetFromEntry builds a hunter pet from a creature entry and
// spawns it, mirroring the pet-creation tail of Unit::CreateTamedPetFrom
// (Unit.cpp:11103-11120) + Spell::EffectCreateTamedPet (SpellEffects.cpp:
// 5362-5396): character_pet INSERT (PET_SAVE_AS_CURRENT), pet_spell rows
// (PetSpellInitialize), family passives, and the world spawn (AddToMap +
// SetMinion). Shared by the wild-creature tame (effect 150) and the
// MiscValue-entry pet creation (effect 153).
func (s *session) createTamedPetFromEntry(ctx context.Context, cdb *sql.DB, spellID, creatureEntry, petLevel uint32) {
	if cdb == nil {
		return
	}
	var maxPetID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM character_pet").Scan(&maxPetID)
	newPetID := uint32(maxPetID + 1)

	var modelID uint32
	var cName string
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && creatureEntry != 0 {
		var mID int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name, modelid1 FROM creature_template WHERE entry = ?", creatureEntry).Scan(&cName, &mID)
		modelID = uint32(mID)
	}
	if cName == "" {
		cName = "Pet"
	}

	maxHP, _, maxMP, _ := s.getPetStats(ctx, creatureEntry, petLevel, 1, newPetID)
	now := time.Now().Unix()

	_, _ = cdb.ExecContext(ctx,
		"INSERT INTO character_pet (id, entry, owner, modelid, CreatedBySpell, PetType, level, exp, Reactstate, name, renamed, slot, curhealth, curmana, curhappiness, savetime, abdata) VALUES (?, ?, ?, ?, ?, 1, ?, 0, 1, ?, 0, 0, ?, ?, ?, ?, '')",
		newPetID, creatureEntry, s.playerGUID, modelID, spellID, petLevel, cName, maxHP, maxMP, petTameStartHappiness, now)

	spells := getPetSpellsForLevel(creatureEntry, petLevel)
	for _, sp := range spells {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", newPetID, sp.id, sp.active)
	}
	s.learnPetFamilyPassives(ctx, newPetID, creatureEntry)

	s.spawnPet(ctx, newPetID, creatureEntry, cName, petLevel, modelID, maxHP, maxHP, maxMP, maxMP, 1)
}

// hasStabledHunterPet mirrors the InitTamedPet refusal arm
// (Unit.cpp:11122-11125): taming fails when the stable holds a current
// (slot 0) or unslotted (slot 100) hunter pet. The live-pet case
// (PetGUID != 0) is checked by the callers.
func (s *session) hasStabledHunterPet(ctx context.Context) bool {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	rows, err := s.queryUnslottedStablePets(ctx, cdb)
	if err == nil {
		for _, r := range rows {
			if r.petType == 1 { // HUNTER_PET
				return true
			}
		}
	}
	var current int64
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&current); err == nil && current > 0 {
		return true
	}
	return false
}

// handleCreateTamedPet mirrors Spell::EffectCreateTamedPet
// (SpellEffects.cpp:5362-5396): the unit target must be a hunter player
// with no current pet; the pet is created from the effect's MiscValue
// creature entry at the caster's level (Unit::CreateTamedPetFrom,
// Unit.cpp:11103-11120). The target is session-bound — cross-player
// targets resolve through findSessionByGUID only when the target's
// session is live, else the caster's own target arm is the only
// modelable case.
func (s *session) handleCreateTamedPet(ctx context.Context, spellID uint32, targetGUID uint64, eff wotlk.SpellEffect) {
	if s.player == nil || s.player.Class != 3 || targetGUID == 0 || targetGUID != s.playerGUID { // Hunter only
		return
	}
	if s.player.PetGUID != 0 {
		return
	}
	if s.hasStabledHunterPet(ctx) {
		return
	}
	creatureEntry := uint32(0)
	if eff.MiscValue > 0 {
		creatureEntry = uint32(eff.MiscValue)
	}
	if creatureEntry == 0 {
		return
	}
	// Unit::CreateTamedPetFrom (Unit.cpp:11106-11108): a missing creature
	// template fails the creation.
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var exists int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&exists); err != nil || exists == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	s.createTamedPetFromEntry(ctx, cdb, spellID, creatureEntry, uint32(s.player.Level))
}

type petFeedState struct {
	PetGUID   uint64
	PetID     uint32
	ItemGUID  uint64
	ItemEntry uint32
	Benefit   uint32
}

func (s *session) checkPetFood(ctx context.Context, itemGUID uint64) (petFeedState, uint8) {
	if s == nil || s.player == nil || s.server == nil || itemGUID == 0 || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return petFeedState{}, spellFailedBadTargets
	}
	var itemEntry, itemCount, foodType, itemLevel int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT ii.itemEntry, ii.count FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.item = ? LIMIT 1`, s.playerGUID, int64(itemGUID)).Scan(&itemEntry, &itemCount)
	if err != nil || itemEntry <= 0 || itemCount <= 0 || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return petFeedState{}, spellFailedBadTargets
	}
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT FoodType, ItemLevel FROM item_template WHERE entry = ?", itemEntry).Scan(&foodType, &itemLevel); err != nil || foodType < 0 || itemLevel < 0 {
		return petFeedState{}, spellFailedBadTargets
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, s.player.PetGUID)
	petGUID, petID, petEntry, petLevel := uint64(0), uint32(0), uint32(0), uint32(0)
	petInCombat := false
	if motion != nil && motion.OwnerGUID == s.playerGUID {
		petGUID, petID, petEntry, petLevel = motion.GUID, motion.PetID, motion.Entry, motion.Level
		petInCombat = motion.InCombat || motion.UnitFlags&unitFlagInCombat != 0
	}
	s.server.motionMu.Unlock()
	if petGUID == 0 {
		return petFeedState{}, spellFailedNoPet
	}
	var family int64
	if s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT family FROM creature_template WHERE entry = ?", petEntry).Scan(&family) != nil || family <= 0 || s.server.Data == nil {
		return petFeedState{}, spellFailedWrongPetFood
	}
	foodMask, found, err := s.server.Data.CreatureFamilyPetFoodMask(uint32(family))
	if err != nil || !found || !PetFoodInDiet(uint32(foodType), foodMask) {
		return petFeedState{}, spellFailedWrongPetFood
	}
	benefit := PetFoodBenefitLevel(petLevel, uint32(itemLevel))
	if benefit == 0 {
		return petFeedState{}, spellFailedFoodLowLevel
	}
	if s.player.UnitFlags&unitFlagInCombat != 0 || petInCombat {
		return petFeedState{}, spellFailedAffectingCombat
	}
	return petFeedState{PetGUID: petGUID, PetID: petID, ItemGUID: itemGUID, ItemEntry: uint32(itemEntry), Benefit: benefit}, 0
}

func (s *session) handleFeedPet(ctx context.Context, spellID uint32, itemGUID uint64, triggerSpell uint32, effectIndex uint8) {
	feed, failure := s.checkPetFood(ctx, itemGUID)
	if failure != 0 || feed.PetGUID == 0 || triggerSpell == 0 || s.server.Data == nil {
		return
	}
	spell, found, err := s.server.Data.Spell(triggerSpell)
	if err != nil || !found || len(spell.Effects) == 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, feed.PetGUID)
	petAlive := motion != nil && motion.OwnerGUID == s.playerGUID && motion.Health != 0
	s.server.motionMu.Unlock()
	if !petAlive {
		return
	}
	log := protocol.NewBuffer(32)
	log.WritePackedGUID(s.playerGUID)
	log.WriteU32(spellID)
	log.WriteU32(1)
	log.WriteU32(101)
	log.WriteU32(1)
	log.WriteU32(feed.ItemEntry)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), s)
	}
	// SpellEffects.cpp:4190: the DestroyItemCount result is ignored — the
	// trigger spell fires even if the destroy failed.
	s.consumeInventoryItemByGUID(ctx, feed.ItemGUID, 1)
	s.castSpellDirectWithBasePoint(ctx, triggerSpell, feed.PetGUID, feed.Benefit)
	s.debug("pet fed", "account", s.accountName, "petID", s.activePetNumber(), "item", feed.ItemEntry, "benefit", feed.Benefit, "effect", effectIndex)
}

// teachPetSpell mirrors the pet arm of Spell::EffectLearnPetSpell
// (SpellEffects.cpp:3105-3127): pet->learnSpell + SavePetToDB(PET_SAVE_AS_CURRENT)
// + owner->PetSpellInitialize(). The DB write is the pet_spell row (the
// SavePetToDB spell-table arm, same triple the pet-talent and .pet learn
// paths use); PetSpellInitialize is sendPetSpells. An unknown spell id
// teaches nothing (C++ returns when the TriggerSpell lookup fails), and an
// already-known spell is a silent no-op (Pet::learnSpell's HasSpell gate).
func (s *session) teachPetSpell(ctx context.Context, spellID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil || spellID == 0 {
		return
	}
	if _, found, err := s.server.Data.Spell(spellID); err != nil || !found {
		return
	}
	motion := s.controlledPetMotion(s.player.PetGUID)
	if motion == nil {
		return
	}
	s.server.motionMu.Lock()
	for _, sp := range motion.Spells {
		if sp == spellID {
			s.server.motionMu.Unlock()
			return
		}
	}
	motion.Spells = append(motion.Spells, spellID)
	entry, reactState := motion.Entry, motion.ReactState
	s.server.motionMu.Unlock()
	petNumber := s.player.PetNumber
	if petNumber == 0 {
		petNumber = s.petNumberForGUID(s.player.PetGUID)
	}
	if petNumber != 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petNumber, spellID, s.petLearnActiveState(spellID))
	}
	learnedBuf := protocol.NewBuffer(4)
	learnedBuf.WriteU32(spellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learnedBuf.Bytes(), true)
	s.sendPetSpells(ctx, petNumber, entry, reactState)
}

var stableSlotPrices = []uint32{
	1000,    // 10s
	50000,   // 5g
	500000,  // 50g
	1500000, // 150g
}

// StableResultCode mirrors NPCHandler.cpp:48.
const (
	stableErrMoney       uint8 = 0x01 // STABLE_ERR_MONEY
	stableErrStable      uint8 = 0x06 // STABLE_ERR_STABLE
	stableSuccessStable  uint8 = 0x08 // STABLE_SUCCESS_STABLE
	stableSuccessUnslot  uint8 = 0x09 // STABLE_SUCCESS_UNSTABLE
	stableSuccessBuySlot uint8 = 0x0A // STABLE_SUCCESS_BUY_SLOT
	stableErrExotic      uint8 = 0x0C // STABLE_ERR_EXOTIC
)

// handleBuyStableSlot processes CMSG_BUY_STABLE_SLOT (0x272).
// Reference: WorldSession::HandleBuyStableSlot (NPCHandler.cpp:563).
func (s *session) handleBuyStableSlot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	// NPCHandler.cpp:566-570 (HandleBuyStableSlot): CheckStableMaster gates
	// the purchase; a failed check answers STABLE_ERR_STABLE.
	if !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:576-578 — the feign-death strip runs after the
	// CheckStableMaster gate.
	s.removeFakeDeath()

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	// NPCHandler.cpp:575-586: the slot price is StableSlotPrices[MaxStabledPets+1]
	// and the cap gate is MaxStabledPets < MAX_PET_STABLES (PetDefines.h:36);
	// MaxStabledPets is characters.stable_slots (loaded at player_state.go:2914,
	// saved with the character row). The purchase persists, and the Go price
	// table keeps the DBC order [10s, 5g, 50g, 150g].
	if s.player.StableSlots >= uint8(len(stableSlotPrices)) {
		s.sendPetStableResult(stableErrStable)
		return true
	}

	cost := stableSlotPrices[s.player.StableSlots]
	if s.player.Money < cost {
		s.sendPetStableResult(stableErrMoney)
		return true
	}

	s.player.Money -= cost
	s.questMoneyChanged(ctx)
	s.player.StableSlots++
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ?, stable_slots = ? WHERE guid = ?", s.player.Money, s.player.StableSlots, s.playerGUID)

	s.sendPetStableResult(stableSuccessBuySlot)
	s.sendPlayerUpdate()
	s.debug("stable slot purchased", "account", s.accountName, "slot", s.player.StableSlots)
	return true
}

// handleDismissCritter processes CMSG_DISMISS_CRITTER (0x48D).
// Reference: WorldSession::HandleDismissCritter (PetHandler.cpp:40-55): the
// GUID resolves like GetCreatureOrPetOrVehicle; only the player's tracked
// critter GUID (UNIT_FIELD_CRITTER) may be dismissed, and dismissal unsummons
// the temp summon. An unmatched GUID is a silent no-op (TC_LOG_DEBUG arm).
func (s *session) handleDismissCritter(ctx context.Context, payload []byte) bool {
	if s.player == nil {
		return true
	}
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	critterGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	// PetHandler.cpp:51: _player->GetCritterGUID() == pet->GetGUID() gate.
	if critterGUID == 0 || critterGUID != s.player.CritterGUID {
		s.debug("dismiss critter", "account", s.accountName, "critter", critterGUID, "ignored", true)
		return true
	}
	s.dismissCritter(ctx, critterGUID)
	return true
}

// dismissCritter wires the destroy path shared by CMSG_DISMISS_CRITTER and
// critter replacement at summon: despawn the client object, clear the tracked
// GUID (Unit.cpp:6111-6114 SetMinion remove-arm), and delete the vanity pet's
// character_pet row so a dismissed critter does not respawn at next login.
func (s *session) dismissCritter(ctx context.Context, critterGUID uint64) {
	s.destroyCritterObject(critterGUID)
	s.deleteCritterRow(ctx, s.player.CritterPetID)
	s.player.CritterGUID = 0
	s.player.CritterPetID = 0
	s.sendPlayerUpdate()
	s.debug("critter dismissed", "account", s.accountName, "critter", critterGUID)
}

// destroyCritterObject despawns a critter object for the session and nearby
// players, mirroring TempSummon::UnSummon's removal from world.
func (s *session) destroyCritterObject(critterGUID uint64) {
	if critterGUID == 0 {
		return
	}
	s.sendDestroyObject(critterGUID, false)
	if s.server != nil {
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(critterGUID)
		destroyBuf.WriteU8(0)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), s)
	}
}

// deleteCritterRow removes the vanity pet's character_pet row (and its
// pet_spell rows) created at summon; C++ has no row for temp-summon
// minipets, Go's summon path inserts one that would otherwise respawn the
// dismissed critter at login.
func (s *session) deleteCritterRow(ctx context.Context, petID uint32) {
	if petID == 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ?", petID)
	_, _ = cdb.ExecContext(ctx, "DELETE FROM character_pet WHERE id = ? AND owner = ?", petID, s.playerGUID)
}

// controlledPetMotion mirrors the GetFirstControlled gate shared by the pet
// command handlers (PetHandler.cpp:97 HandlePetAction, :433 HandlePetSetAction,
// :269 HandlePetStopAttack): the GUID must resolve to the player's active pet
// or to a creature the player currently charms; anything else is a silent drop.
func (s *session) controlledPetMotion(petGUID uint64) *creatureMotion {
	if s == nil || s.server == nil || s.player == nil || petGUID == 0 {
		return nil
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	if petGUID != s.player.PetGUID {
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, petGUID)
		if motion == nil || motion.CharmerGUID != s.playerGUID {
			return nil
		}
		return motion
	}
	return s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, petGUID)
}

// handlePetAbandon processes CMSG_PET_ABANDON (0x176).
// Reference: WorldSession::HandlePetAbandonOpcode (PetHandler.cpp:695-710).
func (s *session) handlePetAbandon(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	petNumber := s.petNumberForGUID(petGUID)
	if petNumber == 0 || s.server == nil || s.server.CharactersStore == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	// PetHandler.cpp:703-709: only hunter pets can be abandoned; the GUID
	// must resolve to the player's own pet (petNumberForGUID already requires
	// the active-pet GUID match).
	var petType int64
	if err := cdb.QueryRowContext(ctx, "SELECT PetType FROM character_pet WHERE id = ? AND owner = ?", petNumber, s.playerGUID).Scan(&petType); err != nil || petType != 1 {
		return true
	}
	// PetHandler.cpp:705-708: abandoning the active pet drains 50000
	// happiness first, clamped at zero.
	_, _ = cdb.ExecContext(ctx, "UPDATE character_pet SET curhappiness = CASE WHEN curhappiness > 50000 THEN curhappiness - 50000 ELSE 0 END WHERE id = ? AND owner = ?", petNumber, s.playerGUID)
	s.unsummonPet(ctx, petSaveAsDeleted)
	return true
}

// creatureHasNpcFlag resolves a spawned creature's template npcflag, used for
// stablemaster and other service guards.
func (s *session) creatureHasNpcFlag(ctx context.Context, guid uint64, flag uint32) bool {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || guid == 0 {
		return false
	}
	var npcFlag uint32
	err := s.server.WorldStore.DB.QueryRowContext(ctx,
		"SELECT COALESCE(t.npcflag, 0) FROM creature c JOIN creature_template t ON t.entry = c.id1 WHERE c.guid = ?", guid).Scan(&npcFlag)
	return err == nil && npcFlag&flag != 0
}

// npcFlagStablemaster mirrors UNIT_NPC_FLAG_STABLEMASTER (UnitDefines.h:208).
const npcFlagStablemaster = 0x00400000

// checkStableMaster mirrors WorldSession::CheckStableMaster
// (PetHandler.cpp:437-460): the player opening their own stable needs GM
// state OR SPELL_AURA_OPEN_STABLE (292); a stablemaster creature must pass
// Player::GetNPCIfCanInteractWith (Player.cpp:2314-2359: in-world, not in
// flight, alive, carries STABLEMASTER, not charmed, not hostile, within
// interact distance) via canInteractWithNPC.
func (s *session) checkStableMaster(ctx context.Context, guid uint64) bool {
	if guid == s.playerGUID {
		if s.player.ExtraFlags&playerExtraGMOn != 0 {
			return true
		}
		// PetHandler.cpp:442: a non-GM player under SPELL_AURA_OPEN_STABLE
		// (e.g. Call Stabled Pet) may open their own stable.
		for _, aura := range s.loadedAuras() {
			if aura != nil && aura.AuraType == spellAuraOpenStable {
				return true
			}
		}
		return false
	}
	return s.canInteractWithNPC(ctx, guid, uint64(npcFlagStablemaster))
}

// checkStabledPetTameable mirrors the exotic-pet arm shared by
// HandleStableSwapPet (NPCHandler.cpp:641-649) and HandleUnstablePet
// (NPCHandler.cpp:475-483): the stabled pet's template must be tameable by
// this player; an exotic-capable template answers STABLE_ERR_EXOTIC, any
// other failure (including the null-creatureInfo arm) STABLE_ERR_STABLE.
func (s *session) checkStabledPetTameable(ctx context.Context, creatureEntry uint32) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		s.sendPetStableResult(stableErrStable)
		return false
	}
	var ctype, family, typeFlags int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(type, 0), COALESCE(family, 0), COALESCE(type_flags, 0) FROM creature_template WHERE entry = ?", creatureEntry).Scan(&ctype, &family, &typeFlags); err != nil {
		s.sendPetStableResult(stableErrStable)
		return false
	}
	if !creatureTameable(ctype, family, typeFlags, s.canTameExoticPets()) {
		if creatureTameable(ctype, family, typeFlags, true) {
			s.sendPetStableResult(stableErrExotic)
		} else {
			s.sendPetStableResult(stableErrStable)
		}
		return false
	}
	return true
}

// sendPetStableResult mirrors WorldSession::SendPetStableResult.
func (s *session) sendPetStableResult(code uint8) {
	buf := protocol.NewBuffer(1)
	buf.WriteU8(code)
	_ = s.write(uint16(protocol.OpcodeSMSG_STABLE_RESULT), buf.Bytes(), true)
}

// unslottedStablePetRow is one PET_SAVE_NOT_IN_SLOT pet (PetStable::UnslottedPets,
// PetDefines.h:114) — in the DB-backed Go tree these are the character_pet
// rows with slot = petStorageSlotNotInSlot (100); Pet.cpp:140 / Player.cpp:20873
// push the dismissed current pet there.
type unslottedStablePetRow struct {
	petID   int64
	petType int64
	health  int64
}

// queryUnslottedStablePets lists the owner's unslotted (slot 100) pets.
func (s *session) queryUnslottedStablePets(ctx context.Context, cdb *sql.DB) ([]unslottedStablePetRow, error) {
	rows, err := cdb.QueryContext(ctx, "SELECT id, PetType, curhealth FROM character_pet WHERE owner = ? AND slot = ?", s.playerGUID, petStorageSlotNotInSlot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []unslottedStablePetRow
	for rows.Next() {
		var p unslottedStablePetRow
		if rows.Scan(&p.petID, &p.petType, &p.health) == nil {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

// summonStabledPet spawns the given character_pet row as the player's active
// pet, mirroring Pet::LoadPetFromDB's world spawn (Pet.cpp:351 SetMinion,
// :356 AddToMap) plus PetSpellInitialize for the stable swap/unstable
// handlers (NPCHandler.cpp:686+, :536+). The row must already sit in slot 0;
// the stat derivation matches the login summon (characters.go).
func (s *session) summonStabledPet(ctx context.Context, cdb *sql.DB, petNumber uint32) {
	var petID, entry, modelID, level, curHealth, curMana, petType, reactState int64
	var petName string
	if err := cdb.QueryRowContext(ctx, "SELECT id, entry, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, petNumber).Scan(&petID, &entry, &modelID, &level, &petName, &curHealth, &curMana, &petType, &reactState); err != nil {
		return
	}
	petLevel := uint32(level)
	maxHP, _, maxMP, _ := s.getPetStats(ctx, uint32(entry), petLevel, uint8(petType), uint32(petID))
	if maxHP == 0 {
		maxHP = uint32(curHealth)
	}
	if maxMP == 0 {
		maxMP = uint32(curMana)
	}
	if curHealth > int64(maxHP) {
		curHealth = int64(maxHP)
	}
	if curMana > int64(maxMP) {
		curMana = int64(maxMP)
	}
	s.learnPetFamilyPassives(ctx, uint32(petID), uint32(entry))
	s.spawnPet(ctx, uint32(petID), uint32(entry), petName, petLevel, uint32(modelID), uint32(curHealth), maxHP, uint32(curMana), maxMP, uint8(reactState))
	if s.player.PetGUID != 0 {
		_ = s.sendTalentsInfo(true)
	}
}

// handleStablePet processes CMSG_STABLE_PET (0x270).
// Reference: WorldSession::HandleStablePet (NPCHandler.cpp:410): a living
// player stables its active hunter pet (slot 0) into the first free stable
// slot; with no active pet the single unslotted hunter pet is stabled
// instead. Results flow through SMSG_STABLE_RESULT.
func (s *session) handleStablePet(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	if s.isDeadOrGhost() || !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:399-401 (HandleStablePet) — the feign-death strip runs
	// after the alive/CheckStableMaster gate.
	s.removeFakeDeath()
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:418-421 (HandleStablePet): GetPetStable() == nil —
	// _LoadPetStable (Player.cpp:26758) never initializes the stable when
	// the player has no stable slots and no pet rows at all — is a silent
	// return with no result packet.
	if s.player.StableSlots == 0 {
		var petRows int64
		if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_pet WHERE owner = ?", s.playerGUID).Scan(&petRows); err != nil || petRows == 0 {
			return true
		}
	}
	var freeSlot int64 = -1
	rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_pet WHERE owner = ? AND slot > 0 AND slot <= ? ORDER BY slot", s.playerGUID, s.player.StableSlots)
	if err == nil {
		taken := map[int64]struct{}{}
		for rows.Next() {
			var slot int64
			if rows.Scan(&slot) == nil {
				taken[slot] = struct{}{}
			}
		}
		rows.Close()
		// NPCHandler.cpp:443: the free-slot scan runs over freeSlot < MaxStabledPets
		// (slot = PET_SAVE_FIRST_STABLE_SLOT + freeSlot), i.e. slots 1..stable_slots.
		for slot := int64(1); slot <= int64(s.player.StableSlots); slot++ {
			if _, used := taken[slot]; !used {
				freeSlot = slot
				break
			}
		}
	}
	var currentPetType, currentHealth int64
	err = cdb.QueryRowContext(ctx, "SELECT PetType, curhealth FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&currentPetType, &currentHealth)
	if err == nil {
		// NPCHandler.cpp:425-429: a dead or non-hunter active pet errors.
		if currentPetType != int64(petTypeHunter) || currentHealth <= 0 {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		if freeSlot < 0 {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		// NPCHandler.cpp:424 (HandleStablePet): RemovePet despawns the
		// stabled pet (destroy object, SMSG_PET_SPELLS clear) and saves it
		// with the stable-slot mode; unsummonPet persists the live stats to
		// slot 0 and tears down the entity, then the row moves below.
		s.unsummonPet(ctx, petSaveAsCurrent)
		if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND slot = 0", freeSlot, time.Now().Unix(), s.playerGUID); err != nil {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		s.sendPetStableResult(stableSuccessStable)
		return true
	}
	// NPCHandler.cpp:430-442: with no active pet, the exactly-one unslotted
	// (PET_SAVE_NOT_IN_SLOT) living hunter pet is stabled instead of
	// answering STABLE_ERR_STABLE.
	unslotted, uerr := s.queryUnslottedStablePets(ctx, cdb)
	if uerr != nil || len(unslotted) != 1 || unslotted[0].petType != int64(petTypeHunter) || unslotted[0].health <= 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if freeSlot < 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND id = ?", freeSlot, time.Now().Unix(), s.playerGUID, unslotted[0].petID); err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	s.sendPetStableResult(stableSuccessStable)
	return true
}

// handleStableRevivePet processes CMSG_STABLE_REVIVE_PET (0x274).
// Reference: WorldSession::HandleStableRevivePet (NPCHandler.cpp:591-595):
// "Not implemented" in C++ — the handler logs and does nothing. The old Go
// revive behavior (reviving the dead current pet) was a deviation from C++,
// so this is a silent no-op.
func (s *session) handleStableRevivePet(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	return true
}

// handleStableSwapPet processes CMSG_STABLE_SWAP_PET (0x275).
// Reference: WorldSession::HandleStableSwapPet: swap the active pet with a
// stabled one by pet number.
func (s *session) handleStableSwapPet(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	petNumber, err := r.ReadU32()
	if err != nil {
		return true
	}
	if !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:616-618 (HandleStableSwapPet) — the feign-death strip
	// runs after the CheckStableMaster gate.
	s.removeFakeDeath()
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var stabledSlot int64
	var stabledEntry uint32
	err = cdb.QueryRowContext(ctx, "SELECT slot, entry FROM character_pet WHERE owner = ? AND id = ? AND slot > 0 AND slot < 100", s.playerGUID, petNumber).Scan(&stabledSlot, &stabledEntry)
	if err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if !s.checkStabledPetTameable(ctx, stabledEntry) {
		return true
	}
	// Reference: WorldSession::HandleStableSwapPet (NPCHandler.cpp:641+):
	// the displaced old pet moves into the requested stabled slot; a dead
	// or non-hunter active pet errors. With no active pet, the
	// exactly-one unslotted living hunter pet is moved there instead; any
	// other pet state (multiple unslotted pets) errors.
	var activePetType, activeHealth int64
	activeErr := cdb.QueryRowContext(ctx, "SELECT PetType, curhealth FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&activePetType, &activeHealth)
	if activeErr == nil {
		if activePetType != int64(petTypeHunter) || activeHealth <= 0 {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		// NPCHandler.cpp:659 (HandleStableSwapPet): RemovePet despawns the
		// displaced pet and saves it into the requested stabled slot;
		// unsummonPet persists the live stats to slot 0 and tears down the
		// entity, then the row moves to the stabled slot below.
		s.unsummonPet(ctx, petSaveAsCurrent)
		if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND slot = 0", stabledSlot, time.Now().Unix(), s.playerGUID); err != nil {
			s.sendPetStableResult(stableErrStable)
			return true
		}
	} else {
		unslotted, uerr := s.queryUnslottedStablePets(ctx, cdb)
		if uerr != nil {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		if len(unslotted) == 1 && unslotted[0].petType == int64(petTypeHunter) && unslotted[0].health > 0 {
			if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND id = ?", stabledSlot, time.Now().Unix(), s.playerGUID, unslotted[0].petID); err != nil {
				s.sendPetStableResult(stableErrStable)
				return true
			}
		} else if len(unslotted) > 0 {
			s.sendPetStableResult(stableErrStable)
			return true
		}
	}
	result, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = 0, savetime = ? WHERE owner = ? AND id = ? AND slot > 0 AND slot < 100", time.Now().Unix(), s.playerGUID, petNumber)
	if err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:686-699 (HandleStableSwapPet): the LoadPetFromDB
	// failure arm answers STABLE_ERR_STABLE; the Go DB model surfaces it as
	// zero affected rows on the slot-0 write.
	if rows, _ := result.RowsAffected(); rows == 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:686+: LoadPetFromDB spawns the swapped-in pet into the
	// world (Pet.cpp:351 SetMinion, :356 AddToMap) and re-initializes its
	// spells; summonStabledPet mirrors the login summon for the new slot-0
	// row.
	s.summonStabledPet(ctx, cdb, petNumber)
	s.sendPetStableResult(stableSuccessUnslot)
	return true
}

// handleUnstablePet processes CMSG_UNSTABLE_PET (0x272? see dispatch).
// Reference: WorldSession::HandleUnstablePet: move a stabled pet into the
// active slot; an occupied active slot must use the swap opcode instead.
func (s *session) handleUnstablePet(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	petNumber, err := r.ReadU32()
	if err != nil {
		return true
	}
	if !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:462-464 (HandleUnstablePet) — the feign-death strip
	// runs after the CheckStableMaster gate.
	s.removeFakeDeath()
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var stabledSlot int64
	var stabledEntry uint32
	if err := cdb.QueryRowContext(ctx, "SELECT slot, entry FROM character_pet WHERE owner = ? AND id = ? AND slot > 0 AND slot < 100", s.playerGUID, petNumber).Scan(&stabledSlot, &stabledEntry); err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if !s.checkStabledPetTameable(ctx, stabledEntry) {
		return true
	}
	// Reference: WorldSession::HandleUnstablePet (NPCHandler.cpp:469-482): a
	// summoned pet does not block the un-stabling — C++ stables the current
	// pet into the requested pet's slot (RemovePet with
	// PET_SAVE_FIRST_STABLE_SLOT + the slot index, i.e. the row's own slot)
	// and then loads the requested pet into slot 0. A dead or non-hunter
	// active pet is an error (STABLE_ERR_STABLE); only the exotic arms
	// answer STABLE_ERR_EXOTIC.
	var activePetType, activeHealth int64
	if err := cdb.QueryRowContext(ctx, "SELECT PetType, curhealth FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&activePetType, &activeHealth); err == nil {
		if activeHealth <= 0 || activePetType != int64(petTypeHunter) {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		// NPCHandler.cpp:505 (HandleUnstablePet): RemovePet despawns the
		// current pet and saves it into the requested stabled slot;
		// unsummonPet persists the live stats to slot 0 and tears down the
		// entity, then the row moves to the stabled slot below.
		s.unsummonPet(ctx, petSaveAsCurrent)
		if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND slot = 0", stabledSlot, time.Now().Unix(), s.playerGUID); err != nil {
			s.sendPetStableResult(stableErrStable)
			return true
		}
	} else {
		// NPCHandler.cpp: HandleUnstablePet unslotted arm: with no
		// summoned pet, the exactly-one unslotted living hunter pet is
		// moved into the requested stabled slot; other pet states error.
		unslotted, uerr := s.queryUnslottedStablePets(ctx, cdb)
		if uerr != nil {
			s.sendPetStableResult(stableErrStable)
			return true
		}
		if len(unslotted) == 1 && unslotted[0].petType == int64(petTypeHunter) && unslotted[0].health > 0 {
			if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND id = ?", stabledSlot, time.Now().Unix(), s.playerGUID, unslotted[0].petID); err != nil {
				s.sendPetStableResult(stableErrStable)
				return true
			}
		} else if len(unslotted) > 0 {
			s.sendPetStableResult(stableErrStable)
			return true
		}
	}
	result, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = 0, savetime = ? WHERE owner = ? AND id = ? AND slot > 0 AND slot < 100", time.Now().Unix(), s.playerGUID, petNumber)
	if err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	// NPCHandler.cpp:536+: LoadPetFromDB spawns the un-stabled pet into the
	// world (Pet.cpp:351 SetMinion, :356 AddToMap) and re-initializes its
	// spells; summonStabledPet mirrors the login summon for the new slot-0
	// row.
	s.summonStabledPet(ctx, cdb, petNumber)
	s.sendPetStableResult(stableSuccessUnslot)
	return true
}

// Pet command handlers below use the live creature-motion pet entity and
// persisted pet state. Reference:
// PetHandler.cpp HandlePetAction (73), HandlePetCancelAuraOpcode (215),
// HandlePetCastSpellOpcode (241), HandlePetLearnTalent (265), HandlePetRename
// (284), HandlePetSetAction (328), HandlePetSpellAutocastOpcode (365),

// sendPetActionBarTalkGrowl mirrors the talk/growl arm shared by the pet
// action-bar spell arm (PetHandler.cpp:337-349) and its no-spell-data
// fallback: a summon pet plays SMSG_PET_ACTION_SOUND with
// PET_TALK_SPECIAL_SPELL 10% of the time (pet != unit_target, so the
// self-cast exclusion is the target GUID check); every other cast answers
// the growl. Both are owner-only (Unit.cpp:10463-10486).
func (s *session) sendPetActionBarTalkGrowl(petGUID uint64, motion *creatureMotion, targetGUID uint64) {
	if motion != nil && motion.PetType == petTypeSummon && !motion.Charmed && petGUID != targetGUID && rand.Intn(101) < 10 {
		talkBuf := protocol.NewBuffer(12)
		talkBuf.WriteU64(petGUID)
		talkBuf.WriteU32(petTalkSpecialSpell)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_ACTION_SOUND), talkBuf.Bytes(), true)
	} else {
		reactionBuf := protocol.NewBuffer(12)
		reactionBuf.WriteU64(petGUID)
		reactionBuf.WriteU32(aiReactionHostile)
		_ = s.write(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), true)
	}
}

// handlePetAction processes CMSG_PET_ACTION (0x175).
// Reference: WorldSession::HandlePetAction (PetHandler.cpp:73-259).
func (s *session) handlePetAction(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	action, err := r.ReadU32()
	if err != nil {
		return true
	}
	var targetGUID uint64
	if len(payload) >= 20 {
		targetGUID, _ = r.ReadU64()
	}

	spellOrAction := action & 0x00FFFFFF
	actFlag := uint8((action >> 24) & 0xFF)

	const (
		actCommand  uint8 = 0x07
		actReaction uint8 = 0x06
		actDisabled uint8 = 0x81
		actEnabled  uint8 = 0xC1
		actPassive  uint8 = 0x01

		commandStay    uint32 = 0
		commandFollow  uint32 = 1
		commandAttack  uint32 = 2
		commandAbandon uint32 = 3

		reactPassive    uint32 = 0
		reactDefensive  uint32 = 1
		reactAggressive uint32 = 2
	)

	// PetHandler.cpp:97-102: the action target must be the player's first
	// controlled unit — any other GUID is a silent drop.
	motion := s.controlledPetMotion(petGUID)
	if motion == nil {
		return true
	}
	// PetHandler.cpp:104-111: on a dead pet the spell arms (ACT_ENABLED /
	// ACT_PASSIVE) survive only for spells carrying
	// SPELL_ATTR0_CASTABLE_WHILE_DEAD (unknown spell ids drop too).
	// Command, reaction and disabled arms take their normal paths — C++
	// drops nothing else for a dead pet.
	if motion.Health == 0 && (actFlag == actEnabled || actFlag == actPassive) {
		castableWhileDead := false
		if s.server != nil && s.server.Data != nil {
			if si, found, spellErr := s.server.Data.Spell(spellOrAction); spellErr == nil && found && si.Attributes&spellAttr0CastableWhileDead != 0 {
				castableWhileDead = true
			}
		}
		if !castableWhileDead {
			return true
		}
	}

	switch actFlag {
	case actCommand:
		switch spellOrAction {
		case commandAttack:
			// PetHandler.cpp:200-207: a pet cannot be ordered to attack while
			// its owner is pacified (SPELL_AURA_MOD_PACIFY).
			pacified := false
			for _, aura := range s.loadedAuras() {
				if aura != nil && aura.AuraType == spellAuraModPacify {
					pacified = true
					break
				}
			}
			if pacified {
				return true
			}
			// PetHandler.cpp:203-207: the order drops silently unless the owner
			// passes IsValidAttackTarget on the target (ownerAttackTargetValid
			// mirrors Object.cpp:2930+ with the session player as attacker) —
			// Go used to engage any target the GM gate allowed. PetHandler.cpp:210-213: no target unit -> the order
			// drops before any engage or AI reaction.
			if !s.ownerAttackTargetValid(ctx, targetGUID) {
				return true
			}
			// PetHandler.cpp:216-217: a pet already attacking that target
			// under a command-attack ignores the re-issue — no forced target
			// switch, no AI reaction.
			if motion.InCombat && motion.TargetGUID == targetGUID && motion.PetCommand == PetCommandAttack {
				return true
			}
			// PetHandler.cpp:197-198: when the order switches victims (or
			// the pet wasn't under a command-attack), the old attack stops
			// first — Unit::AttackStop broadcasts SMSG_ATTACK_STOP with the
			// old victim before the new AttackStart.
			if oldVictim := motion.TargetGUID; oldVictim != 0 {
				stopPkt := buildAttackStop(petGUID, oldVictim, false)
				_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
				if s.server != nil {
					s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, s)
				}
			}
			if s.server != nil {
				s.server.onPetCommandAttack(ctx, s.player.Map, s.player.InstanceID, petGUID, targetGUID)
			}
			// Reference: PetHandler.cpp:213-218 — a summon pet (warlock;
			// SUMMON_PET) ordered to attack plays the special attack talk
			// (SMSG_PET_ACTION_SOUND with PET_TALK_ATTACK) 10% of the time
			// when it is not attacking itself; everything else — the other
			// 90%, hunter pets, charmed creatures (motion.Charmed carries
			// the zero-value PetType, so it must be excluded explicitly) —
			// answers the growl. Both SendPetTalk and SendPetAIReaction are
			// owner-only (Unit.cpp:10463-10486 SendDirectMessage to the
			// owner); Go used to broadcast the reaction to nearby players.
			if motion.PetType == petTypeSummon && !motion.Charmed && petGUID != targetGUID && rand.Intn(101) < 10 {
				talkBuf := protocol.NewBuffer(12)
				talkBuf.WriteU64(petGUID)
				talkBuf.WriteU32(petTalkAttack)
				_ = s.write(uint16(protocol.OpcodeSMSG_PET_ACTION_SOUND), talkBuf.Bytes(), true)
			} else {
				reactionBuf := protocol.NewBuffer(12)
				reactionBuf.WriteU64(petGUID)
				reactionBuf.WriteU32(aiReactionHostile)
				_ = s.write(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), true)
			}
			s.debug("pet attack command", "account", s.accountName, "pet", petGUID, "target", targetGUID)
		case commandFollow:
			// PetHandler.cpp:164: the follow arm runs Unit::AttackStop ahead
			// of the follow move — Unit::AttackStop (Unit.cpp:5763-5789)
			// emits SMSG_ATTACK_STOP (SendMeleeAttackStop broadcasts to the
			// set) only when an attack was actually in progress, carrying
			// the real victim; the old unconditional victim-0 owner-only
			// packet never matched C++.
			followVictim := motion.TargetGUID
			if s.server != nil {
				s.server.onPetCommandFollow(ctx, s.player.Map, s.player.InstanceID, petGUID)
			}
			if followVictim != 0 {
				stopPkt := buildAttackStop(petGUID, followVictim, false)
				_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
				if s.server != nil {
					s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, s)
				}
			}
			s.debug("pet follow command", "account", s.accountName, "pet", petGUID)
		case commandStay:
			// PetHandler.cpp:151-162: the stay command sets the stay state
			// (onPetCommandStay keeps the victim) and sends no
			// SMSG_ATTACK_STOP — Go used to broadcast one and disengage.
			if s.server != nil {
				s.server.onPetCommandStay(s.player.Map, s.player.InstanceID, petGUID)
			}
			s.debug("pet stay command", "account", s.accountName, "pet", petGUID)
		case commandAbandon:
			// PetHandler.cpp:242-256: a charmed creature is un-charmed
			// (StopCastingCharm) rather than removed; hunter pets are
			// deleted outright while other summoned pets are merely
			// unsummoned (kept out of the active slot).
			if motion.CharmerGUID == s.playerGUID && s.server != nil {
				s.server.uncharmCreature(creatureAuraKey{Map: s.player.Map, InstanceID: s.player.InstanceID, GUID: petGUID}, s.playerGUID)
				s.debug("pet charm released via command", "account", s.accountName, "pet", petGUID)
				break
			}
			saveMode := uint8(petSaveAsDeleted)
			if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				var petType int64
				if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT PetType FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, s.petNumberForGUID(petGUID)).Scan(&petType); err == nil && petType != 1 {
					saveMode = uint8(petSaveNotInSlot)
				}
			}
			s.unsummonPet(ctx, saveMode)
			s.debug("pet abandoned via command", "account", s.accountName, "pet", petGUID)
		}
	case actReaction:
		if s.server != nil {
			s.server.onPetSetReaction(ctx, s.player.Map, s.player.InstanceID, petGUID, uint8(spellOrAction), s)
		}
		// Save react state (0 = passive, 1 = defensive, 2 = aggressive)
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_pet SET Reactstate = ? WHERE owner = ? AND slot = 0", spellOrAction, s.playerGUID)
		}
		s.debug("pet reaction state changed", "account", s.accountName, "pet", petGUID, "react", spellOrAction)
	case actDisabled, actEnabled, actPassive:
		// PetHandler.cpp:270-289: the spell arm drops silently ahead of the
		// AI reaction on unknown spell ids, spells carrying an enemy-area
		// implicit target (TARGET_UNIT_SRC_AREA_ENEMY/DEST_AREA_ENEMY/
		// DEST_DYNOBJ_ENEMY, SharedDefines.h:1452-1464), unlearned spells,
		// and passive spells — Go used to broadcast the reaction and a
		// SpellGo visual for all of them. The gates run only with spell
		// data present; without a DB the arm keeps its old visual-only
		// behavior.
		var spellInfo wotlk.Spell
		haveSpellInfo := false
		if s.server != nil && s.server.Data != nil {
			si, found, spellErr := s.server.Data.Spell(spellOrAction)
			if spellErr != nil || !found {
				return true
			}
			for i := range si.Effects {
				switch si.Effects[i].ImplicitTargetA {
				case 15, 16, 28:
					return true
				}
			}
			if si.Attributes&spellAttributePassive != 0 || !s.petKnowsSpell(ctx, motion, spellOrAction) {
				return true
			}
			spellInfo, haveSpellInfo = si, true
		}
		if !haveSpellInfo {
			s.sendPetActionBarTalkGrowl(petGUID, motion, targetGUID)
			if targetGUID != 0 && spellOrAction != 0 {
				castTimeStamp := uint32(time.Now().UnixMilli())
				spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
				goPkt := protocol.BuildSpellGo(petGUID, petGUID, 1, spellOrAction, spellCastFlagGo, castTimeStamp, []uint64{targetGUID}, nil, spellTarget)
				_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
				if s.server != nil {
					s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
				}
			}
			s.debug("pet cast spell action", "account", s.accountName, "pet", petGUID, "spell", spellOrAction, "target", targetGUID)
			break
		}
		// PetHandler.cpp:311-382: after the gates the arm casts for real
		// (new Spell(pet, spellInfo, TRIGGERED_NONE) + CheckPetCast +
		// prepare), not just the visual. CMSG_PET_ACTION carries no cast
		// count, so the failure packets use 0 (Spell::SendPetCastResult
		// writes the fresh Spell's m_cast_count).
		castTarget := protocol.SpellTargetData{UnitGUID: targetGUID}
		if targetGUID != 0 {
			castTarget.Flags = protocol.SpellTargetFlagUnitWireMask
		}
		// PetHandler.cpp:304-310: ahead of the cast the pet leaves its
		// stay/follow/return states and enters command-attack — the AI
		// resets the rest after AttackStart.
		var prevCommand uint8
		if s.server != nil {
			prevCommand = s.server.petCommandOnSpellCast(s.player.Map, s.player.InstanceID, petGUID)
		}
		if !s.checkPetCastGauntlet(ctx, motion, spellInfo, spellOrAction, castTarget, 0, false) {
			// PetHandler.cpp:380-382: the cast failure resets the
			// command-attack flag.
			if s.server != nil {
				s.server.restorePetCommand(s.player.Map, s.player.InstanceID, petGUID, prevCommand)
			}
			return true
		}
		// PetHandler.cpp:337-349: the talk/growl fires on SPELL_CAST_OK,
		// ahead of prepare.
		s.sendPetActionBarTalkGrowl(petGUID, motion, targetGUID)
		// The UNIT_NOT_INFRONT auto-turn (PetHandler.cpp:320-336) has no Go
		// twin — Go's motion model carries no facing. The failure-path
		// ResetCooldown (PetHandler.cpp:376-377, same arm at :837-838) needs
		// no twin: it runs only when !HasCooldown(spellid), and
		// SpellHistory::ResetCooldown(spellId, update) returns immediately
		// with no packet when the per-spell entry is absent
		// (SpellHistory.cpp:432-439) — the arm is dead code in C++, so
		// Go's omission is behaviorally exact.
		if !s.executePetSpellWithOptions(ctx, motion, spellInfo, 0, castTarget, false) {
			_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(0, spellOrAction, spellFailedBadTargets), true)
			// PetHandler.cpp:380-382: the cast failure resets the
			// command-attack flag.
			if s.server != nil {
				s.server.restorePetCommand(s.player.Map, s.player.InstanceID, petGUID, prevCommand)
			}
		} else if targetGUID != 0 && motion.TargetGUID != targetGUID && s.server != nil {
			// PetHandler.cpp:351-365: after SPELL_CAST_OK the pet
			// force-switches victim to the spell's unit target when it is
			// not friendly to the owner (possessed/vehicle pets excluded in
			// C++; Go's motion model has no such distinction).
			if !s.isFriendlyToTarget(targetGUID, s.server.findSessionByGUID(targetGUID)) {
				s.server.switchPetVictim(ctx, s.player.Map, s.player.InstanceID, petGUID, targetGUID)
			}
		}
		s.debug("pet cast spell action", "account", s.accountName, "pet", petGUID, "spell", spellOrAction, "target", targetGUID)
	}
	return true
}

func (s *session) handlePetCancelAura(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	spellID, _ := r.ReadU32()
	// SpellHandler.cpp HandlePetCancelAuraOpcode:503-508: an unknown spell id
	// drops the packet silently (the SpellInfo lookup happens first); the
	// gate runs only with spell data present.
	if s.server != nil && s.server.Data != nil {
		if _, found, _ := s.server.Data.Spell(spellID); !found {
			return true
		}
	}
	// SpellHandler.cpp:510-526: the target must be the player's guardian pet
	// or charmed creature — Go used to delete from any GUID's pet_aura row.
	motion := s.controlledPetMotion(petGUID)
	if motion == nil {
		return true
	}
	// SpellHandler.cpp:528-532: a dead pet answers SMSG_PET_ACTION_FEEDBACK
	// with FEEDBACK_PET_DEAD (PetDefines.h:73, Unit.cpp:10452 — a single
	// uint8 to the owner) and the aura stays.
	if motion.Health == 0 {
		feedback := protocol.NewBuffer(1)
		feedback.WriteU8(1) // FEEDBACK_PET_DEAD
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_ACTION_FEEDBACK), feedback.Bytes(), true)
		return true
	}
	// SpellHandler.cpp:534: pet->RemoveOwnedAura(spell, EMPTY, 0,
	// AURA_REMOVE_BY_CANCEL) — the live aura leaves the server's creature
	// aura maps alongside the pet_aura row (Go only cleared the row).
	petNumber := s.petNumberForGUID(petGUID)
	if s.server != nil {
		key := creatureAuraKeyForMotion(motion)
		s.server.auraMu.Lock()
		delete(s.server.creatureAuras[key], spellID)
		delete(s.server.activeCreatureAuras[key], spellID)
		s.server.auraMu.Unlock()
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid = ? AND spell = ?", petNumber, spellID)
		}
	}
	s.debug("pet cancel aura", "account", s.accountName, "pet", petNumber, "spell", spellID)
	return true
}

// checkPetCastGauntlet runs the Spell::CheckPetCast (Spell.cpp:6219-6254) +
// CheckCast(true) tail (Spell.cpp:6254) gauntlet shared by the client
// cast-spell path (HandlePetCastSpellOpcode, PetHandler.cpp:775-797) and the
// pet action-bar spell arm (PetHandler.cpp:273-382). Each failing arm sends
// SMSG_PET_CAST_FAILED to the owner (Spell::SendPetCastResult,
// Spell.cpp:SendPetCastResult) and the helper returns false; a true return
// means the cast may proceed to executePetSpellWithOptions
// (Spell::prepare/_cast). The cast-spell path resolves the target first
// (self-cast spells, then the owner's selection); the action-bar arm passes
// the packet's guid2 only (C++: unit_target =
// ObjectAccessor::GetUnit(guid2), possibly null) with no selection
// fallback.
// deathGate enables the CheckCast death arm: the CMSG_PET_CAST_SPELL path
// resolves the motion in any state and a dead pet reports
// SPELL_FAILED_CASTER_DEAD from the tail's head (Spell.cpp:5177-5179); the
// action-bar path keeps its pre-filtered motion and its documented dead-pet
// behavior, so the arm stays off there.
func (s *session) checkPetCastGauntlet(ctx context.Context, motion *creatureMotion, spell wotlk.Spell, spellID uint32, target protocol.SpellTargetData, castCount uint8, deathGate bool) bool {
	if s.isDeadOrGhost() {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedCasterDead), true)
		return false
	}
	// Spell::CheckPetCast (Spell.cpp:6230-6235): a spell that needs an explicit
	// unit target fails SPELL_FAILED_BAD_IMPLICIT_TARGETS when none was
	// provided (Spell.cpp:6227-6228's m_targets unit-target copy is already
	// covered by the resolution above; the m_targets.SetUnitTarget(target) half
	// is structural — target.UnitGUID carries the resolved target onward).
	if spellNeedsExplicitUnitTarget(spell) && target.UnitGUID == 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadImplicitTargets), true)
		return false
	}
	// Spell::CheckPetCast (Spell.cpp:6237-6247): power precedes cooldown, and
	// both precede the CheckCast(true) tail (Spell.cpp:6254) — a cast that
	// fails power and range together reports the power failure, like C++.
	if !s.checkPetSpellPower(motion, spell, castCount) {
		return false
	}
	now := time.Now()
	categoryID := spell.Category
	s.server.motionMu.Lock()
	prunePetSpellCooldowns(motion, now)
	spellCooldownEnd := motion.SpellCooldowns[spellID]
	categoryEnd := time.Time{}
	if categoryID != 0 {
		categoryEnd = motion.SpellCategoryCooldowns[categoryID]
	}
	s.server.motionMu.Unlock()
	if !spellCooldownEnd.IsZero() && !spellCooldownEnd.Before(now) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
		return false
	}
	if !categoryEnd.IsZero() && !categoryEnd.Before(now) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
		return false
	}
	// Spell::CheckPetCast GCD arm (Spell.cpp:6249-6252): a pet or charmed
	// creature — both carry CharmInfo in C++ (Pet.cpp:246, Charm()), and
	// both reach this handler via petMotionForCast — fails
	// SPELL_FAILED_NOT_READY while its global cooldown, keyed by the
	// spell's StartRecoveryCategory, is active. Unlike the CheckCast GCD
	// gate (5227) and TriggerGlobalCooldown (3196), this arm has no
	// TRIGGERED_IGNORE_GCD exemption: the triggered from-client aura leg
	// (PetHandler.cpp:791-796) still passes through CheckPetCast. C++
	// relative order: power, then IsReady (bridged above), then this arm,
	// then CheckCast(true).
	if spell.StartRecoveryCategory > 0 && s.server.petGlobalCooldownActive(motion, spell, now) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
		return false
	}
	// Spell::CheckCast (Spell.cpp:5177-5179): the death-state arm opens
	// CheckCast, ahead of every tail leg — a caster that is dead when the
	// tail runs fails SPELL_FAILED_CASTER_DEAD. (The passive/triggered
	// exemption has no pet-path trigger source: the triggered leg already
	// passed the aura gate above, and Go carries no SPELL_ATTR0_CASTABLE_WHILE_DEAD.)
	if deathGate && motion.Health == 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedCasterDead), true)
		return false
	}
	// CheckCast(true) tail (Spell.cpp:6254), modeled subset in C++ relative
	// order (CheckTarget legs ahead of CheckRange). The rest of the gauntlet
	// is out of scope: the pet path runs no CheckCast.
	// SpellInfo::CheckExplicitTarget faction arms (SpellInfo.cpp:1793-1811):
	// the enemy/ally/party/raid/minipet/passenger verdict on the explicit
	// unit target fails SPELL_FAILED_BAD_TARGETS. C++ relative order:
	// CheckExplicitTarget (5365) runs ahead of the CheckTarget legs below
	// (5372); the backstop in executePetSpellWithOptions re-checks this for
	// AI and triggered casts, which fizzle silently.
	if target.UnitGUID != 0 && s.server != nil && !s.server.checkCreatureExplicitTargetFaction(motion, s, spell, target.UnitGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
		return false
	}
	// SpellInfo::CheckTarget GM/invisibility leg (SpellInfo.cpp:1736-1743):
	// a GM-mode or GM-invisible player target fails with
	// SPELL_FAILED_BM_OR_INVISGOD. The pet caster's GetAffectingPlayer is the
	// session player, so the gate fires for harmful and helpful casts alike,
	// like C++; self-targets are exempt (unitTarget != caster).
	if target.UnitGUID != 0 && target.UnitGUID != motion.GUID && s.server != nil && s.server.gmAttackTargetBlocked(target.UnitGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBmOrInvisGod), true)
		return false
	}
	// SpellInfo::CheckTarget (SpellInfo.cpp:1636-1638):
	// SPELL_ATTR1_CANT_TARGET_SELF with caster == target fails
	// SPELL_FAILED_BAD_TARGETS. (The old isHarmfulSpell proxy is dropped:
	// C++ keys on the attribute, not harmfulness.)
	if spell.AttributesEx&spellAttr1CantTargetSelf != 0 && target.UnitGUID == motion.GUID {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
		return false
	}
	if target.UnitGUID != 0 && target.UnitGUID != motion.GUID {
		targetState, targetFound := s.getCombatTarget(ctx, target.UnitGUID)
		if !targetFound || targetState.Map != motion.Map {
			_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
			return false
		}
		if spell.RangeIndex > 0 {
			if rangeInfo, rangeFound, rangeErr := s.server.Data.SpellRange(spell.RangeIndex); rangeErr == nil && rangeFound && rangeInfo.MaxHostile > 0 {
				if distance3D(motion.X, motion.Y, motion.Z, targetState.X, targetState.Y, targetState.Z) > float64(rangeInfo.MaxHostile)+5 {
					_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, 97), true)
					return false
				}
			}
		}
	}
	return true
}

func (s *session) handlePetCastSpell(ctx context.Context, payload []byte) bool {
	if len(payload) < 13 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	castCount, err := r.ReadU8()
	if err != nil {
		return true
	}
	spellID, err := r.ReadU32()
	if err != nil {
		return true
	}
	castFlags, _ := r.ReadU8()
	target, targetErr := protocol.ReadSpellTargetData(r)
	if targetErr != nil {
		return true
	}
	if castFlags&0x02 != 0 {
		// HandleClientCastFlags (SpellHandler.cpp:50-72, via
		// PetHandler.cpp:775-778): elevation/speed feed
		// SelectImplicitTrajTargets (Spell.cpp:1626); the optional embedded
		// movement block routes through HandleMovementOpcodes so the
		// position update is not lost.
		if target.TrajElevation, err = r.ReadF32(); err != nil {
			return true
		}
		if target.TrajSpeed, err = r.ReadF32(); err != nil {
			return true
		}
		var hasMovementData uint8
		if hasMovementData, err = r.ReadU8(); err != nil {
			return true
		}
		if hasMovementData != 0 {
			var moveOpcode uint32
			if moveOpcode, err = r.ReadU32(); err != nil {
				return true
			}
			var movePayload []byte
			if movePayload, err = r.Read(r.Remaining()); err != nil {
				return true
			}
			if !s.handleMovement(ctx, moveOpcode, movePayload) {
				return false
			}
		}
	}
	if spellID == 0 || s.server == nil || s.server.Data == nil || s.player == nil {
		return true
	}
	// PetHandler.cpp:775-797: the alive gate is off here — a dead pet
	// runs through the Spell and fails at CheckCast's death arm (the
	// gauntlet's deathGate leg below), answering SMSG_PET_CAST_FAILED like
	// C++ instead of a silent drop.
	motion, ok := s.petMotionForCastAnyState(petGUID)
	if !ok {
		return true
	}
	spell, found, spellErr := s.server.Data.Spell(spellID)
	petKnows := s.petKnowsSpell(ctx, motion, spellID)
	// HandlePetCastSpellOpcode (PetHandler.cpp:791-796): an unlearned pet
	// spell is allowed when the pet carries a
	// SPELL_AURA_PERIODIC_TRIGGER_SPELL_FROM_CLIENT (48) aura whose effect
	// triggers it; the cast is fully triggered (TRIGGERED_FULL_MASK,
	// SpellDefines.h:153), which executePetSpellWithOptions carries.
	triggered := false
	if !petKnows && s.server != nil {
		petKey := creatureAuraKey{Map: s.player.Map, InstanceID: s.player.InstanceID, GUID: petGUID}
		s.server.auraMu.Lock()
		triggered = auraMapHasTriggerSpell(s, s.server.activeCreatureAuras[petKey], spellID)
		s.server.auraMu.Unlock()
	}
	// HandlePetCastSpellOpcode (PetHandler.cpp:775-797): an unknown spell id,
	// a passive spell, or an unlearned non-triggered spell ends the handler
	// SILENTLY — C++ logs (unknown id) and returns without any packet, so no
	// SMSG_PET_CAST_FAILED is sent on these three arms.
	if spellErr != nil || !found {
		s.debug("pet cast spell", "account", s.accountName, "pet", petGUID, "spell", spellID, "reason", "unknown spell")
		return true
	}
	if spell.Attributes&spellAttributePassive != 0 {
		return true
	}
	if !petKnows && !triggered {
		return true
	}
	// Spell::CheckPetCast (Spell.cpp:6219-6220): the UNIT_STATE_CASTING arm has
	// no Go bridge — creatureMotion carries no cast-in-progress state and pet
	// casts resolve instantly through executePetSpellWithOptions, so the
	// overlap the guard prevents cannot occur; the TRIGGERED_IGNORE_CAST_IN_PROGRESS
	// wrapper is vacuous here (this path never carries triggered flags).
	// Dead-owner arm (Spell.cpp:6222-6225): the owner on this path is always
	// the session player (petMotionForCast requires OwnerGUID/CharmerGUID ==
	// playerGUID); the IsGhouled() exemption has no Go model.
	if target.UnitGUID == 0 {
		if isSelfCastOnly(spell) {
			target.UnitGUID = motion.GUID
			target.Flags |= protocol.SpellTargetFlagUnitWireMask
		} else if s.selection != 0 {
			target.UnitGUID = s.selection
			target.Flags |= protocol.SpellTargetFlagUnitWireMask
		}
	}
	if !s.checkPetCastGauntlet(ctx, motion, spell, spellID, target, castCount, true) {
		return true
	}
	if !s.executePetSpellWithOptions(ctx, motion, spell, castCount, target, triggered) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
	} else {
		// Reference: PetHandler.cpp:808-815 — the real-cast talk/growl arm:
		// a summon pet (ToPet() excludes charmed creatures) plays
		// SMSG_PET_ACTION_SOUND with PET_TALK_SPECIAL_SPELL 10% of the
		// time; every other successful cast answers the growl. Unlike the
		// action-bar spell arm there is no self-target exclusion here.
		// Both are owner-only (Unit.cpp:10463-10486).
		if motion.PetType == petTypeSummon && !motion.Charmed && rand.Intn(101) < 10 {
			talkBuf := protocol.NewBuffer(12)
			talkBuf.WriteU64(petGUID)
			talkBuf.WriteU32(petTalkSpecialSpell)
			_ = s.write(uint16(protocol.OpcodeSMSG_PET_ACTION_SOUND), talkBuf.Bytes(), true)
		} else {
			reactionBuf := protocol.NewBuffer(12)
			reactionBuf.WriteU64(petGUID)
			reactionBuf.WriteU32(aiReactionHostile)
			_ = s.write(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), true)
		}
	}
	s.debug("pet cast spell", "account", s.accountName, "pet", petGUID, "spell", spellID, "castCount", castCount)
	return true
}

func (s *session) petMotionForCast(petGUID uint64) (*creatureMotion, bool) {
	return s.petMotionForCastWithAlive(petGUID, true)
}

// petMotionForCastAnyState is petMotionForCast without the alive gate: the
// CMSG_PET_CAST_SPELL path runs a dead pet through the Spell and fails at
// Spell::CheckCast's death arm (Spell.cpp:5177-5179) with
// SPELL_FAILED_CASTER_DEAD, so resolving the motion only when alive would
// swallow the SMSG_PET_CAST_FAILED the client gets in C++.
func (s *session) petMotionForCastAnyState(petGUID uint64) (*creatureMotion, bool) {
	return s.petMotionForCastWithAlive(petGUID, false)
}

func (s *session) petMotionForCastWithAlive(petGUID uint64, alive bool) (*creatureMotion, bool) {
	if s == nil || s.server == nil || petGUID == 0 {
		return nil, false
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, petGUID)
	if motion == nil || (alive && motion.Health == 0) || (motion.OwnerGUID != s.playerGUID && motion.CharmerGUID != s.playerGUID) {
		return nil, false
	}
	return motion, true
}

func (s *session) petKnowsSpell(ctx context.Context, motion *creatureMotion, spellID uint32) bool {
	if motion == nil {
		return false
	}
	for _, known := range motion.Spells {
		if known == spellID {
			return true
		}
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil || s.player.PetNumber == 0 {
		return false
	}
	var found int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT 1 FROM pet_spell WHERE guid = ? AND spell = ? LIMIT 1", s.player.PetNumber, spellID).Scan(&found)
	return err == nil && found != 0
}

// maxPetTalentRank mirrors MAX_PET_TALENT_RANK (DBCEnums.h:433): a requested
// pet talent rank must sit below 3 — Player::LearnPetTalent (Player.cpp:25578)
// rejects rank >= 3, where Go previously accepted ranks 3-4.
// maxPetTalentScanRanks mirrors MAX_TALENT_RANK (DBCEnums.h:432): the
// SpellRank scan width when finding the pet's current max rank of a talent
// (Player.cpp:25610). Kept separate from skills.go's typed maxTalentRank.
const maxPetTalentRank = 3
const maxPetTalentScanRanks = 5

// learnPetTalent mirrors Player::LearnPetTalent (Player.cpp:25563-25702),
// shared by CMSG_PET_LEARN_TALENT and CMSG_LEARN_PREVIEW_TALENTS_PET. The
// pet GUID must resolve to the session's live pet and the talent must
// exist. The requested rank must sit strictly above the best rank of that
// talent the pet already knows (C++: curtalent_maxrank >= talentRank+1 is a
// silent no-op), which also blocks rank downgrades; all lower known ranks
// are unlearned like Pet::learnSpell's rank-chain arm, and the learned
// spell is added to the live motion's spell list so later HasSpell checks
// see it. The prereq-talent and tier spent-points gates below mirror
// Player.cpp:25627-25673. Free-talent-point accounting
// (Pet::GetFreeTalentPoints) and the family PetTalentMask gate have no Go
// model yet (no usedTalentCount, no CreatureFamily entry) and stay
// documented deviations.
func (s *session) learnPetTalent(ctx context.Context, petNumber uint32, petGUID uint64, talentID uint32, talentRank uint32) bool {
	if petNumber == 0 || talentRank >= maxPetTalentRank || s == nil || s.server == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.server.Data == nil {
		return false
	}
	tEntry, ok, err := s.server.Data.Talent(talentID)
	if err != nil || !ok {
		return false
	}
	motion := s.controlledPetMotion(petGUID)
	if motion == nil {
		return false
	}
	known := [maxPetTalentScanRanks]bool{}
	for i := 0; i < maxPetTalentScanRanks; i++ {
		if tEntry.SpellRank[i] == 0 {
			continue
		}
		for _, sp := range motion.Spells {
			if sp == tEntry.SpellRank[i] {
				known[i] = true
				break
			}
		}
	}
	curMax := -1
	for i := maxPetTalentScanRanks - 1; i >= 0; i-- {
		if known[i] {
			curMax = i
			break
		}
	}
	if int(talentRank) <= curMax {
		return false
	}
	// Player.cpp:25627-25641: a talent with PrereqTalent learns only when the
	// pet knows some rank of the dependency at or above PrereqRank.
	if tEntry.PrereqTalent > 0 {
		if dep, depOK, depErr := s.server.Data.Talent(tEntry.PrereqTalent); depErr == nil && depOK {
			hasEnoughRank := false
			for rank := int(tEntry.PrereqRank); rank < maxPetTalentScanRanks && !hasEnoughRank; rank++ {
				if dep.SpellRank[rank] == 0 {
					continue
				}
				for _, sp := range motion.Spells {
					if sp == dep.SpellRank[rank] {
						hasEnoughRank = true
						break
					}
				}
			}
			if !hasEnoughRank {
				return false
			}
		}
	}
	// Player.cpp:25644-25673: talents past the first tier need
	// TierID*MAX_PET_TALENT_RANK points spent in the same tab (rank+1 per
	// known rank of each same-tab talent).
	if tEntry.TierID > 0 {
		var spentPoints uint32
		if tabTalents, tabErr := s.server.Data.TalentsByTab(tEntry.TabID); tabErr == nil {
			for _, tmp := range tabTalents {
				for rank := 0; rank < maxPetTalentScanRanks; rank++ {
					if tmp.SpellRank[rank] == 0 {
						continue
					}
					for _, sp := range motion.Spells {
						if sp == tmp.SpellRank[rank] {
							spentPoints += uint32(rank + 1)
							break
						}
					}
				}
			}
		}
		if spentPoints < tEntry.TierID*maxPetTalentRank {
			return false
		}
	}
	spellID := tEntry.SpellRank[talentRank]
	if spellID == 0 {
		s.debug("pet learn talent missing spell", "account", s.accountName, "talent", talentID, "rank", talentRank)
		return false
	}
	for _, sp := range motion.Spells {
		if sp == spellID {
			return false
		}
	}
	cdb := s.server.CharactersStore.DB
	// Pet::addSpell's talent arm (Pet.cpp) displaces the other known ranks of
	// the talent via removeSpell — silent, no SMSG_PET_UNLEARNED_SPELL (that
	// packet only goes out from Pet::unlearnSpell, never called here).
	var unlearned []uint32
	for i := 0; i < int(talentRank); i++ {
		if !known[i] {
			continue
		}
		oldSpell := tEntry.SpellRank[i]
		_, _ = cdb.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petNumber, oldSpell)
		unlearned = append(unlearned, oldSpell)
	}
	_, _ = cdb.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petNumber, spellID, s.petLearnActiveState(spellID))
	s.server.motionMu.Lock()
	for _, oldSpell := range unlearned {
		for j, sp := range motion.Spells {
			if sp == oldSpell {
				motion.Spells = append(motion.Spells[:j], motion.Spells[j+1:]...)
				break
			}
		}
	}
	motion.Spells = append(motion.Spells, spellID)
	s.server.motionMu.Unlock()
	learnedBuf := protocol.NewBuffer(4)
	learnedBuf.WriteU32(spellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learnedBuf.Bytes(), true)
	return true
}

func (s *session) handlePetLearnTalent(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	talentID, _ := r.ReadU32()
	rank, _ := r.ReadU32()
	petNumber := s.petNumberForGUID(petGUID)
	learned := s.learnPetTalent(ctx, petNumber, petGUID, talentID, rank)
	// PetHandler.cpp:852-861: SendTalentsInfoData(true) runs even when the
	// learn was a silent no-op. PetSpellInitialize (Pet::learnSpell) fires
	// only when the spell was actually added.
	s.sendTalentsInfo(true)
	if learned {
		s.sendPetSpells(ctx, petNumber, 0, 1)
	}
	s.debug("pet learn talent", "account", s.accountName, "pet", petNumber, "talent", talentID, "rank", rank)
	return true
}

func (s *session) handlePetNameQuery(ctx context.Context, payload []byte) bool {
	// HandleQueryPetName (PetHandler.cpp:393-404) reads the pet number then the
	// pet GUID with `recvData >> petguid` — a raw 8-byte read, not packed.
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	petNumber, err := r.ReadU32()
	if err != nil {
		return true
	}
	petGUID, err := r.ReadU64()
	if err != nil {
		return true
	}

	buf := protocol.NewBuffer(64)
	buf.WriteU32(petNumber)
	// PetHandler.cpp:404-418 SendQueryPetNameResponse: the GUID must resolve
	// to a live creature/pet/vehicle via ObjectAccessor; the session's active
	// pet is the only resolvable analog in this engine. An unresolvable GUID
	// writes the empty response: uint32(0) timestamp slot, uint8(0) declined.
	if resolved := s.petNumberForGUID(petGUID); resolved != 0 && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		var petName string
		var saveTime uint32
		if err := cdb.QueryRowContext(ctx,
			"SELECT name, savetime FROM character_pet WHERE id = ? AND owner = ? LIMIT 1", resolved, s.playerGUID).Scan(&petName, &saveTime); err == nil {
			buf.WriteCString(petName)
			buf.WriteU32(saveTime) // pet name timestamp
			// PetHandler.cpp:424-430: uint8(1) plus the 5 declined names when
			// the pet carries declined names, else uint8(0).
			var declined [5]string
			if err := cdb.QueryRowContext(ctx,
				"SELECT genitive, dative, accusative, instrumental, prepositional FROM character_pet_declinedname WHERE owner = ? AND id = ? LIMIT 1",
				s.playerGUID, resolved).Scan(&declined[0], &declined[1], &declined[2], &declined[3], &declined[4]); err == nil {
				buf.WriteU8(1)
				for _, d := range declined {
					buf.WriteCString(d)
				}
			} else {
				buf.WriteU8(0)
			}
			_ = s.write(uint16(protocol.OpcodeSMSG_PET_NAME_QUERY_RESPONSE), buf.Bytes(), true)
			return true
		}
	}
	buf.WriteU8(0)
	buf.WriteU32(0)
	buf.WriteU8(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_NAME_QUERY_RESPONSE), buf.Bytes(), true)
	return true
}

// handlePetRename processes CMSG_PET_RENAME (0x177).
// Reference: WorldSession::HandlePetRenameOpcode (PetHandler.cpp:284).
func (s *session) handlePetRename(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	newName, _ := r.ReadCString()
	// PetHandler.cpp:594-596: isdeclined byte, then 5 declined-name strings.
	isDeclined, _ := r.ReadU8()
	var declined [5]string
	if isDeclined != 0 {
		for i := range declined {
			declined[i], _ = r.ReadCString()
		}
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		petNumber := s.petNumberForGUID(petGUID)
		// PetHandler.cpp:591-596: only hunter pets can be renamed; the GUID
		// must resolve to the player's own pet (petNumberForGUID already
		// requires the active-pet GUID match) and the owner column must match.
		var petType, renamed int64
		_ = cdb.QueryRowContext(ctx, "SELECT PetType, renamed FROM character_pet WHERE id = ? AND owner = ?", petNumber, s.playerGUID).Scan(&petType, &renamed)
		if petNumber == 0 || petType != 1 {
			return true
		}
		// PetHandler.cpp:601 UNIT_CAN_BE_RENAMED gate: freshly tamed pets
		// carry the flag implicitly until first renamed (Pet.cpp:267/841,
		// persisted via the character_pet.renamed column); re-renames need
		// the flag from SPELL_EFFECT_ALLOW_RENAME_PET (159,
		// handleEffectAllowRenamePet).
		if renamed != 0 && !s.petRenameAllowed[petGUID] {
			return true
		}
		// PetHandler.cpp:599-603: the name goes through ObjectMgr::CheckPetName
		// (utf8 validity, MAX_PET_NAME=12, MinPetName=2 config default) plus the
		// reserved-name list; failures answer SMSG_PET_NAME_INVALID.
		if reason := s.checkPetName(ctx, newName); reason != petNameSuccess {
			s.sendPetNameInvalid(reason, newName, nil)
			return true
		}
		now := time.Now().Unix()
		_, _ = cdb.ExecContext(ctx,
			"UPDATE character_pet SET name = ?, renamed = 1, savetime = ? WHERE id = ? AND owner = ?",
			newName, now, petNumber, s.playerGUID)
		// PetHandler.cpp:624 removes the flag after a successful rename.
		delete(s.petRenameAllowed, petGUID)
		// PetHandler.cpp:672: the rename refreshes UNIT_FIELD_PET_NAME_TIMESTAMP
		// on the live pet — the client re-queries the pet name when the field
		// changes. Go builds the pet update block once at spawn, so push the
		// field explicitly with the same timestamp the name-query response
		// carries (savetime).
		if s.server != nil {
			s.server.broadcastCreatureValuesUpdateInInstance(s.player.Map, s.player.InstanceID, petGUID, map[int]uint32{unitFieldPetNameTimestamp: uint32(now)})
		}
		// PetHandler.cpp:628-645: when the client sends declined names they
		// must pass ObjectMgr::CheckDeclinedNames (ObjectMgr.cpp:8808) —
		// GetMainPartOfName's ending table (Util.cpp:466) pads every ending
		// with an embedded null wchar, so the suffix strip never matches a
		// real name and the function is an identity; the check therefore
		// passes exactly when all five declined forms equal the base name
		// (and are valid UTF-8). A mismatch answers
		// SMSG_PET_NAME_INVALID with PET_NAME_DECLENSION_DOESNT_MATCH_BASE_NAME
		// (16) and the five declined strings, before any DB write.
		if isDeclined != 0 && !checkPetDeclinedNames(newName, declined) {
			s.sendPetNameInvalid(petNameDeclension, newName, &declined)
			return true
		}
		// PetHandler.cpp:649-662: declined names persist in
		// character_pet_declinedname (id, owner, genitive..prepositional).
		if isDeclined != 0 {
			_, _ = cdb.ExecContext(ctx,
				"DELETE FROM character_pet_declinedname WHERE id = ?", petNumber)
			_, _ = cdb.ExecContext(ctx,
				"INSERT INTO character_pet_declinedname (id, owner, genitive, dative, accusative, instrumental, prepositional) VALUES (?, ?, ?, ?, ?, ?, ?)",
				petNumber, s.playerGUID, declined[0], declined[1], declined[2], declined[3], declined[4])
		}
	}
	return true
}

// Pet name invalid reasons mirror PetNameInvalidReason (SharedDefines.h:3708).
const (
	petNameSuccess    = 0
	petNameInvalid    = 1
	petNameNoName     = 2
	petNameTooShort   = 3
	petNameTooLong    = 4
	petNameMixedLang  = 6
	petNameProfane    = 7
	petNameReserved   = 8
	petNameDeclension = 16
)

// isExtendedLatinRune / isEastAsianRune port the Util.h character
// predicates (Util.h:109-166; isCyrillicRune already lives in
// characters.go) used by isValidString's
// strictMask==0 branch (ObjectMgr.cpp:8563-8571): at the
// CONFIG_STRICT_PET_NAMES default of 0 (World.cpp:802) every rune of the
// name must be extended-Latin, or every rune Cyrillic, or every rune
// East-Asian — a mixed-script name answers PET_NAME_MIXED_LANGUAGES.
func isExtendedLatinRune(r rune) bool {
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
		return true
	}
	if (r >= 0x00C0 && r <= 0x00D6) || (r >= 0x00D8 && r <= 0x00DE) {
		return true
	}
	if r == 0x00DF {
		return true
	}
	if (r >= 0x00E0 && r <= 0x00F6) || (r >= 0x00F8 && r <= 0x00FE) {
		return true
	}
	if r >= 0x0100 && r <= 0x012F {
		return true
	}
	return r == 0x1E9E
}

func isEastAsianRune(r rune) bool {
	if r >= 0x1100 && r <= 0x11F9 {
		return true
	}
	if r >= 0x3041 && r <= 0x30FF {
		return true
	}
	if r >= 0x3131 && r <= 0x318E {
		return true
	}
	if r >= 0x31F0 && r <= 0x31FF {
		return true
	}
	if r >= 0x3400 && r <= 0x4DB5 {
		return true
	}
	if r >= 0x4E00 && r <= 0x9FC3 {
		return true
	}
	if r >= 0xAC00 && r <= 0xD7A3 {
		return true
	}
	return r >= 0xFF01 && r <= 0xFFEE
}

func isValidPetNameCharset(name string) bool {
	allLatin, allCyrillic, allEastAsian := true, true, true
	for _, r := range name {
		if !isExtendedLatinRune(r) {
			allLatin = false
		}
		if !isCyrillicRune(r) {
			allCyrillic = false
		}
		if !isEastAsianRune(r) {
			allEastAsian = false
		}
	}
	return allLatin || allCyrillic || allEastAsian
}

// checkPetName mirrors the portable core of ObjectMgr::CheckPetName
// (ObjectMgr.cpp:8642): utf8 validity, MAX_PET_NAME (12) and MinPetName (2,
// World.cpp:818 config default) bounds, the strictMask==0 charset check
// (PET_NAME_MIXED_LANGUAGES), and the reserved_name list C++ loads from the
// characters DB (ObjectMgr.cpp:8482). The ValidateName profanity DBC checks
// have no Go bridge (no name-validation DBC model).
func (s *session) checkPetName(ctx context.Context, name string) uint32 {
	if !utf8.ValidString(name) {
		return petNameInvalid
	}
	if len([]rune(name)) > 12 {
		return petNameTooLong
	}
	if len([]rune(name)) < 2 {
		return petNameTooShort
	}
	if !isValidPetNameCharset(name) {
		return petNameMixedLang
	}
	if s != nil && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var n int64
		// ObjectMgr::IsReservedName (ObjectMgr.cpp:8515-8523) lowercases
		// both sides (wstrToLower) before the lookup; LOWER() on both sides
		// is the equivalent here.
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM reserved_name WHERE LOWER(name) = LOWER(?)", name).Scan(&n); err == nil && n > 0 {
			return petNameReserved
		}
	}
	return petNameSuccess
}

// checkPetDeclinedNames ports the effective behavior of
// ObjectMgr::CheckDeclinedNames (ObjectMgr.cpp:8808-8829): because
// GetMainPartOfName (Util.cpp:466-518) is an identity for real names (its
// ending table's embedded nulls defeat the suffix strip — see the
// handlePetRename comment), the x and y accumulators reduce to the same
// condition: every declined form must equal the base name, and every form
// must be valid UTF-8 (C++ returns false on Utf8toWStr failure). This is
// deliberately stricter than characters.go's checkDeclinedNames, whose
// table strips endings the C++ table never strips — the pet path follows
// the C++ text exactly.
func checkPetDeclinedNames(base string, declined [5]string) bool {
	for i := 0; i < 5; i++ {
		if !utf8.ValidString(declined[i]) || declined[i] != base {
			return false
		}
	}
	return true
}

// sendPetNameInvalid mirrors WorldSession::SendPetNameInvalid
// (PetHandler.cpp:836-850): SMSG_PET_NAME_INVALID carries the u32 reason, the
// name string, and the declined-name flag (with the 5 case strings when set).
func (s *session) sendPetNameInvalid(reason uint32, name string, declined *[5]string) {
	buf := protocol.NewBuffer(64)
	buf.WriteU32(reason)
	buf.WriteCString(name)
	if declined != nil {
		buf.WriteU8(1)
		for _, d := range declined {
			buf.WriteCString(d)
		}
	} else {
		buf.WriteU8(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_NAME_INVALID), buf.Bytes(), true)
}

// handlePetSetAction processes CMSG_PET_SET_ACTION (0x174).
// Reference: WorldSession::HandlePetSetAction (PetHandler.cpp:328).
func (s *session) handlePetSetAction(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	// PetHandler.cpp:433-438: the GUID must be the player's first controlled
	// pet — the Go action-bar model is the active pet's DB row, so charmed
	// creatures have no bridge here and are dropped as well.
	if s.player == nil || petGUID == 0 || petGUID != s.player.PetGUID {
		return true
	}
	// PetHandler.cpp:475: count is 2 only when the packet is exactly 24
	// bytes (8 guid + 2x(4 pos + 4 data)); a longer malformed packet still
	// carries a single pair in C++.
	count := 1
	if len(payload) == 24 {
		count = 2
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	// PetHandler.cpp:515-518: the whole mutation block runs inside the loop
	// over same-entry alive controlled units — an empty list (pet dead or
	// despawned) is a complete no-op, so the packet drops here.
	motion := s.controlledPetMotion(petGUID)
	if motion == nil || motion.Health == 0 {
		return true
	}

	var existingAbdata string
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(abdata, '') FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&existingAbdata)

	type actionSlot struct {
		actType uint8
		action  uint32
	}
	type petSetActionPair struct {
		pos     uint32
		actType uint8
		action  uint32
	}
	const (
		petActCommand  uint8 = 0x07
		petActReaction uint8 = 0x06
		petActEnabled  uint8 = 0xC1
		petActDisabled uint8 = 0x81
		petActPassive  uint8 = 0x01
	)

	slots := make([]actionSlot, 10)
	slots[0] = actionSlot{actType: petActCommand, action: 2}  // Attack
	slots[1] = actionSlot{actType: petActCommand, action: 1}  // Follow
	slots[2] = actionSlot{actType: petActCommand, action: 0}  // Stay
	slots[7] = actionSlot{actType: petActReaction, action: 2} // Aggressive
	slots[8] = actionSlot{actType: petActReaction, action: 1} // Defensive
	slots[9] = actionSlot{actType: petActReaction, action: 0} // Passive
	// CharmInfo::InitPetActionBar (Unit.cpp:9948-9950): the middle 4 slots
	// default to ACT_PASSIVE, not the zero value — a first-ever set-action
	// with missing/malformed abdata must seed "1 0" there like C++.
	for i := 3; i < 7; i++ {
		slots[i] = actionSlot{actType: petActPassive, action: 0}
	}

	if existingAbdata != "" {
		tokens := strings.Fields(existingAbdata)
		if len(tokens) == 20 {
			for i := 0; i < 10; i++ {
				t, _ := strconv.ParseUint(tokens[i*2], 10, 8)
				a, _ := strconv.ParseUint(tokens[i*2+1], 10, 32)
				slots[i] = actionSlot{actType: uint8(t), action: uint32(a)}
			}
		}
	}

	var pairs []petSetActionPair
	for i := 0; i < count; i++ {
		pos, pErr := r.ReadU32()
		data, dErr := r.ReadU32()
		if pErr != nil || dErr != nil {
			return true
		}
		// PetHandler.cpp:487-492: an out-of-range action-bar position drops
		// the whole packet instead of skipping the entry.
		if pos >= 10 {
			return true
		}
		aType := uint8((data >> 24) & 0xFF)
		aAction := data & 0x00FFFFFF
		pairs = append(pairs, petSetActionPair{pos: pos, actType: aType, action: aAction})
	}

	// PetHandler.cpp:494-501: command and reaction buttons can only be MOVED,
	// not removed — at moving count == 2, at removing count == 1, so a
	// single-entry command/reaction update drops the whole packet.
	moveCommand := false
	for _, p := range pairs {
		if p.actType == petActCommand || p.actType == petActReaction {
			if count == 1 {
				return true
			}
			moveCommand = true
		}
	}

	// PetHandler.cpp:519-534: at a command->spell swap the client removes the
	// spell first in another packet, so only command-move correctness is
	// checked: each moved command/reaction button must match the CURRENT bar
	// entry at the OTHER position, else the whole packet drops.
	if moveCommand {
		for i, p := range pairs {
			if p.actType == petActCommand || p.actType == petActReaction {
				other := slots[pairs[1-i].pos]
				if other.action != p.action || other.actType != p.actType {
					return true
				}
			}
		}
	}

	// PetHandler.cpp:547-551: a spell-type button (en/disable/cast) whose
	// action the pet does not know is skipped entirely — no bar entry, no
	// autocast toggle.
	for _, p := range pairs {
		if p.action != 0 && (p.actType == petActEnabled || p.actType == petActDisabled || p.actType == petActPassive) && !s.petKnowsSpell(ctx, motion, p.action) {
			continue
		}
		slots[p.pos] = actionSlot{actType: p.actType, action: p.action}

		// Synchronize pet_spell table when spell action buttons change active state
		petNumber := s.petNumberForGUID(petGUID)
		if p.action > 0 && petNumber > 0 {
			if p.actType == petActEnabled { // ACT_ENABLED (autocast enabled)
				// Pet::ToggleAutocast (Pet.cpp:1808-1824): pet_spell.active
				// stores the ActiveStates byte (ACT_ENABLED 0xC1 /
				// ACT_DISABLED 0x81), persisted verbatim (Pet.cpp:1194-1196).
				_, _ = cdb.ExecContext(ctx, "UPDATE pet_spell SET active = ? WHERE guid = ? AND spell = ?", petActionEnabled, petNumber, p.action)
				// PetHandler.cpp:553-577: the set-action path runs the same
				// ToggleAutocast arms as the autocast opcode, so the live
				// motion's autocast list follows the bar immediately — the
				// pet autocasts the spell without waiting for a relog.
				if s.server != nil {
					s.server.onPetToggleAutocast(s.player.Map, s.player.InstanceID, petGUID, p.action, true)
				}
			} else if p.actType == petActDisabled { // ACT_DISABLED (autocast disabled)
				_, _ = cdb.ExecContext(ctx, "UPDATE pet_spell SET active = ? WHERE guid = ? AND spell = ?", petActionDisabled, petNumber, p.action)
				if s.server != nil {
					s.server.onPetToggleAutocast(s.player.Map, s.player.InstanceID, petGUID, p.action, false)
				}
			}
		}
	}

	var sb strings.Builder
	for i, sl := range slots {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(strconv.FormatUint(uint64(sl.actType), 10))
		sb.WriteByte(' ')
		sb.WriteString(strconv.FormatUint(uint64(sl.action), 10))
	}
	_, _ = cdb.ExecContext(ctx, "UPDATE character_pet SET abdata = ? WHERE owner = ? AND slot = 0", sb.String(), s.playerGUID)
	s.debug("pet set action updated", "account", s.accountName, "pet", petGUID)
	return true
}

// syncPetSpellAutocast updates pet_spell.active and synchronizes the action bar slot in character_pet.abdata.
func (s *session) syncPetSpellAutocast(ctx context.Context, petID uint32, spellID uint32, active uint8) {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || petID == 0 || spellID == 0 {
		return
	}
	cdb := s.server.CharactersStore.DB

	// Pet::ToggleAutocast (Pet.cpp:1808-1824): pet_spell.active stores the
	// ActiveStates action type (ACT_ENABLED/ACT_DISABLED), not the raw
	// packet byte.
	dbActive := uint8(petActionDisabled)
	if active != 0 {
		dbActive = uint8(petActionEnabled)
	}
	res, err := cdb.ExecContext(ctx, "UPDATE pet_spell SET active = ? WHERE guid = ? AND spell = ?", dbActive, petID, spellID)
	if err == nil {
		if rows, _ := res.RowsAffected(); rows == 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, spellID, dbActive)
		}
	}

	// Update abdata if pet is currently active (slot = 0)
	var abdata string
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(abdata, '') FROM character_pet WHERE owner = ? AND id = ? AND slot = 0", s.playerGUID, petID).Scan(&abdata)
	if abdata != "" {
		tokens := strings.Fields(abdata)
		if len(tokens) == 20 {
			changed := false
			for i := 0; i < 10; i++ {
				action, _ := strconv.ParseUint(tokens[i*2+1], 10, 32)
				if uint32(action) == spellID {
					newType := uint8(0x81) // ACT_DISABLED
					if active != 0 {
						newType = 0xC1 // ACT_ENABLED
					}
					tokens[i*2] = strconv.FormatUint(uint64(newType), 10)
					changed = true
				}
			}
			if changed {
				newAbdata := strings.Join(tokens, " ")
				_, _ = cdb.ExecContext(ctx, "UPDATE character_pet SET abdata = ? WHERE owner = ? AND id = ?", newAbdata, s.playerGUID, petID)
			}
		}
	}

	if s.server != nil {
		petGUID := s.petGUIDForNumber(petID)
		s.server.onPetToggleAutocast(s.player.Map, s.player.InstanceID, petGUID, spellID, active != 0)
	}
}

// handlePetSpellAutocast processes CMSG_PET_SPELL_AUTOCAST (0x2F3).
// Reference: WorldSession::HandlePetSpellAutocastOpcode (PetHandler.cpp:694).
func (s *session) handlePetSpellAutocast(ctx context.Context, payload []byte) bool {
	// PetPackets.cpp:37: the pet GUID rides packed on the wire (2-9 bytes),
	// then the u32 spell id and the u8 autocast flag.
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadPackedGUID()
	if err != nil {
		return true
	}
	spellID, err := r.ReadU32()
	if err != nil {
		return true
	}
	state, err := r.ReadU8()
	if err != nil {
		return true
	}
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil {
		return true
	}
	// PetHandler.cpp:697-714: an unresolvable GUID, or one that isn't the
	// player's guardian pet or charmed creature, drops silently. The
	// pet-number gate admits the session's pet; charmed creatures have no
	// spell-bar model in Go, so their ToggleCreatureAutocast arm is a
	// documented delta.
	petNumber := s.petNumberForGUID(petGUID)
	addressed := s.controlledPetMotion(petGUID)
	if petNumber == 0 || addressed == nil {
		return true
	}
	spell, found, spellErr := s.server.Data.Spell(spellID)
	// PetHandler.cpp:711-714 + SpellInfo::IsAutocastable (SpellInfo.cpp:1101):
	// a missing spell or a passive / SPELL_ATTR1_UNAUTOCASTABLE_BY_PET spell
	// drops silently (petSpellPassive/petSpellNoAutocast mirror the flags).
	if spellErr != nil || !found || spell.Attributes&petSpellPassive != 0 || spell.AttributesEx&petSpellNoAutocast != 0 {
		return true
	}
	// PetHandler.cpp:716-741: the toggle fans out to every alive controlled
	// unit sharing the addressed pet's entry (e.g. both Spirit Wolves); the
	// loop aborts the whole handler — earlier units keep their toggles —
	// when any iterated unit hasn't learned the spell (the CharmInfo-missing
	// return is vacuous in Go's motion model). A dead addressed pet leaves
	// the list empty: no-op, no DB write.
	s.server.motionMu.Lock()
	var units []*creatureMotion
	for _, m := range s.server.motionMapLocked(s.player.Map, s.player.InstanceID) {
		if m == nil || m.Entry != addressed.Entry || m.Health == 0 ||
			(m.OwnerGUID != s.playerGUID && m.CharmerGUID != s.playerGUID) {
			continue
		}
		hasSpell := false
		for _, sp := range m.Spells {
			if sp == spellID {
				hasSpell = true
				break
			}
		}
		if !hasSpell {
			s.server.motionMu.Unlock()
			return true
		}
		units = append(units, m)
	}
	s.server.motionMu.Unlock()
	for _, m := range units {
		// Pet::ToggleAutocast persists (pet_spell + bar); the non-Pet arm
		// (CharmInfo::ToggleCreatureAutocast) is in-memory only, so siblings
		// without a pet number take the live toggle alone.
		if m.GUID == petGUID {
			s.syncPetSpellAutocast(ctx, petNumber, spellID, state)
		} else {
			s.server.onPetToggleAutocast(s.player.Map, s.player.InstanceID, m.GUID, spellID, state != 0)
		}
	}
	s.debug("pet spell autocast", "account", s.accountName, "pet", petNumber, "spell", spellID, "state", state)
	return true
}

// handlePetStopAttack processes CMSG_PET_STOP_ATTACK (0x2EA).
// Reference: WorldSession::HandlePetStopAttack (PetHandler.cpp:261-280).
func (s *session) handlePetStopAttack(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	// PetHandler.cpp:269-277: the GUID must resolve to the player's pet or
	// charmed creature and the unit must be alive; the stop is Unit::AttackStop
	// (victim clear + SMSG_ATTACK_STOP, threat kept), so the opcode path runs
	// without the threat leg.
	s.stopPetAttacks(petGUID, false)
	return true
}

// stopPetAttacks mirrors the pet leg of Unit::AttackStop (Unit.cpp): the
// pet's victim clears and SMSG_ATTACK_STOP goes out. Threat is NOT cleared:
// C++ Unit::AttackStop carries no threat leg (threat clear belongs to
// CombatStop). Extracted from handlePetStopAttack so death/duel paths can stop
// a controlled pet without going through the opcode handler; those paths pass
// clearThreat=true for the CombatStopWithPets leg.
func (s *session) stopPetAttacks(petGUID uint64, clearThreat bool) {
	motion := s.controlledPetMotion(petGUID)
	if motion == nil || motion.Health == 0 || s.server == nil {
		return
	}
	s.server.motionMu.Lock()
	victim := motion.TargetGUID
	motion.TargetGUID = 0
	motion.InCombat = false
	if clearThreat && motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	s.server.motionMu.Unlock()
	// Unit::AttackStop (Unit.cpp:5763-5789) early-returns with no packet when
	// the unit had no victim — the stop packet only goes out when an attack
	// was actually in progress.
	if victim != 0 {
		stopPkt := buildAttackStop(petGUID, victim, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, s)
	}
	s.debug("pet stop attack", "account", s.accountName, "pet", petGUID)
}

// stopOwnPetAttacks runs the pet leg of Unit::CombatStopWithPets for the
// session's own summoned pet (Player::GetPet analog: s.player.PetGUID).
func (s *session) stopOwnPetAttacks() {
	if s == nil || s.player == nil || s.player.PetGUID == 0 {
		return
	}
	s.stopPetAttacks(s.player.PetGUID, true)
}

// handleRequestPetInfo processes CMSG_REQUEST_PET_INFO (0x279).
// Reference: WorldSession::HandleRequestPetInfoOpcode (PetHandler.cpp:412) and Player::SendPetSpells (Player.cpp:21107-21145).
func (s *session) handleRequestPetInfo(ctx context.Context, payload []byte) bool {
	if s.player == nil {
		return true
	}
	// PetHandler.cpp:891-910 + Player::PetSpellInitialize (Player.cpp:21098):
	// the request keys off the live pet; with no live pet (and no charmed
	// unit, which has no spell-init model in Go) C++ sends nothing. The old
	// slot-0 DB row fallback answered for pets that weren't live.
	if s.controlledPetMotion(s.player.PetGUID) == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		return true
	}

	var petID, entry, reactState int64
	err := cdb.QueryRowContext(ctx, "SELECT id, entry, COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&petID, &entry, &reactState)
	if err != nil || petID == 0 {
		buf := protocol.NewBuffer(8)
		buf.WriteU64(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_SPELLS), buf.Bytes(), true)
		return true
	}

	s.sendPetSpells(ctx, uint32(petID), uint32(entry), uint8(reactState))
	return true
}

// handleListStabledPets processes MSG_LIST_STABLED_PETS (0x26F).
// Reference: WorldSession::HandleRequestStabledPets (NPCHandler.cpp:288)
// and WorldSession::SendStablePet (NPCHandler.cpp:310).
func (s *session) handleListStabledPets(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return true
	}
	// NPCHandler.cpp:293-295: not a stablemaster -> silent drop.
	if !s.checkStableMaster(ctx, npcGUID) {
		return true
	}
	// NPCHandler.cpp:298-300 (HandleRequestStabledPets) — the feign-death
	// strip runs after the CheckStableMaster gate, before the dismount arm.
	s.removeFakeDeath()
	// NPCHandler.cpp:299-303: opening the stable dismounts the player
	// (C++ removes SPELL_AURA_MOUNTED; stabling while mounted deleted the pet).
	if s.isPlayerMounted() {
		for _, aura := range s.loadedAuras() {
			if aura != nil && aura.AuraType == spellAuraMounted {
				s.removeAura(aura.SpellID)
			}
		}
		s.player.MountDisplayID = 0
		s.sendPlayerUpdate()
	}

	s.sendStabledPets(ctx, npcGUID)
	return true
}

// sendStabledPets builds and sends the MSG_LIST_STABLED_PETS packet.
// Reference: WorldSession::SendStablePet (NPCHandler.cpp:310): the guid field
// carries the stable owner (a stablemaster NPC normally; the player's own
// GUID when SPELL_AURA_OPEN_STABLE (292) opens the stable, per
// AuraEffect::HandleAuraOpenStable, SpellAuraEffects.cpp:4918).
func (s *session) sendStabledPets(ctx context.Context, npcGUID uint64) {
	type petInfo struct {
		ID    uint32
		Entry uint32
		Level uint32
		Name  string
		Flags uint8 // NPCHandler.cpp:344/362: 1 active, 2 stabled
	}
	var pets []petInfo
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		// NPCHandler.cpp:336-353: the current pet is listed first with the
		// active flag; with no current pet the single unslotted hunter pet
		// (GetUnslottedHunterPet, PetDefines.h:116-119) shows as active.
		var cur petInfo
		if err := cdb.QueryRowContext(ctx, "SELECT id, entry, level, name FROM character_pet WHERE owner = ? AND slot = 0 LIMIT 1", s.playerGUID).Scan(&cur.ID, &cur.Entry, &cur.Level, &cur.Name); err == nil {
			cur.Flags = 1
			pets = append(pets, cur)
		} else if unslotted, uerr := s.queryUnslottedStablePets(ctx, cdb); uerr == nil && len(unslotted) == 1 && unslotted[0].petType == int64(petTypeHunter) {
			var up petInfo
			if err := cdb.QueryRowContext(ctx, "SELECT id, entry, level, name FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, unslotted[0].petID).Scan(&up.ID, &up.Entry, &up.Level, &up.Name); err == nil {
				up.Flags = 1
				pets = append(pets, up)
			}
		}
		// NPCHandler.cpp:355-365: only StabledPets (slots 1..4) list with
		// flag 2; slot-100 unslotted rows never appear here.
		rows, err := cdb.QueryContext(ctx, "SELECT id, entry, level, name FROM character_pet WHERE owner = ? AND slot > 0 AND slot < 100 ORDER BY slot", s.playerGUID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p petInfo
				if err := rows.Scan(&p.ID, &p.Entry, &p.Level, &p.Name); err == nil {
					p.Flags = 2
					pets = append(pets, p)
				}
			}
		}
	}

	buf := protocol.NewBuffer(16 + len(pets)*32)
	buf.WriteU64(npcGUID)
	buf.WriteU8(uint8(len(pets)))
	// NPCHandler.cpp:328: the slots byte is the player's MaxStabledPets, i.e.
	// characters.stable_slots (bridged by handleBuyStableSlot).
	buf.WriteU8(s.player.StableSlots) // num slots
	for _, p := range pets {
		buf.WriteU32(p.ID)
		buf.WriteU32(p.Entry)
		buf.WriteU32(p.Level)
		buf.WriteCString(p.Name)
		buf.WriteU8(p.Flags)
	}
	_ = s.write(uint16(protocol.OpcodeMSG_LIST_STABLED_PETS), buf.Bytes(), true)
}

// handleLearnPreviewTalentsPet processes CMSG_LEARN_PREVIEW_TALENTS_PET (0x4C2).
// Reference: WorldSession::HandleLearnPreviewTalentsPet (PetHandler.cpp:864-884).
func (s *session) handleLearnPreviewTalentsPet(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	talentCount, _ := r.ReadU32()
	petNumber := s.petNumberForGUID(petGUID)

	// PetHandler.cpp:864-884: the client sends at most 24 talents, rounded up
	// to a 30-entry cap.
	const maxPreviewPetTalents = 30
	for i := uint32(0); i < talentCount && i < maxPreviewPetTalents && r.Remaining() >= 8; i++ {
		talentID, _ := r.ReadU32()
		rank, _ := r.ReadU32()
		// Pet::learnSpell (Pet.cpp:1486-1497) answers SMSG_PET_LEARNED_SPELL
		// and runs PetSpellInitialize (SMSG_PET_SPELLS) per learned spell;
		// learnPetTalent already sends the learned packet, so the spells
		// packet follows each successful learn like the single-learn path.
		if s.learnPetTalent(ctx, petNumber, petGUID, talentID, rank) {
			s.sendPetSpells(ctx, petNumber, 0, 1)
		}
	}
	// PetHandler.cpp:882: SendTalentsInfoData(true) runs unconditionally
	// after the loop, even when nothing was learned.
	s.sendTalentsInfo(true)
	s.debug("pet preview talents learned", "account", s.accountName, "pet", petGUID, "count", talentCount)
	return true
}
