package world

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	petSaveAsDeleted        uint8 = 0
	petSaveAsCurrent        uint8 = 1
	petSaveNotInSlot        uint8 = 2
	petStorageSlotNotInSlot uint8 = 100

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
	petHappinessMax           = 1050000
)

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

func (s *Server) nextPetLowGUID() uint32 {
	s.objectsMu.Lock()
	defer s.objectsMu.Unlock()
	s.nextPetGUID++
	if s.nextPetGUID == 0 {
		s.nextPetGUID = 1
	}
	return s.nextPetGUID
}

func (s *session) activePetNumber() uint32 {
	if s == nil || s.player == nil {
		return 0
	}
	if s.player.PetNumber != 0 {
		return s.player.PetNumber
	}
	return uint32(s.player.PetGUID & 0xFFFFFFFF)
}

func (s *session) petNumberForGUID(guid uint64) uint32 {
	if s != nil && s.player != nil && s.player.PetGUID == guid && s.player.PetNumber != 0 {
		return s.player.PetNumber
	}
	return uint32(guid & 0xFFFFFFFF)
}

func (s *session) petGUIDForNumber(number uint32) uint64 {
	if s != nil && s.player != nil && s.player.PetNumber == number && s.player.PetGUID != 0 {
		return s.player.PetGUID
	}
	return uint64(number) | (uint64(0xF140) << 48)
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
			active := uint8(0)
			if bestRank.autocast {
				active = 1
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

func (s *session) getPetStats(ctx context.Context, entry uint32, level uint32, petType uint8) (curHealth, maxHealth, curMana, maxMana uint32) {
	if level == 0 {
		level = 1
	}
	if petType == 1 {
		entry = 1
	}
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry != 0 {
		var hp, mana int64
		err := s.server.WorldStore.DB.QueryRowContext(ctx,
			"SELECT hp, mana FROM pet_levelstats WHERE creature_entry = ? AND level = ?", entry, level).Scan(&hp, &mana)
		if err == nil && hp > 0 {
			return uint32(hp), uint32(hp), uint32(mana), uint32(mana)
		}
	}
	hp := level * 35
	if hp < 100 {
		hp = 100
	}
	mana := level * 30
	if mana < 80 {
		mana = 80
	}
	return hp, hp, mana, mana
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

func buildPetUpdate(petGUID uint64, petNumber, entry uint32, level uint32, modelID uint32, curHealth uint32, maxHealth uint32, curMana uint32, maxMana uint32, attributes [5]uint32, curHappiness uint32, ownerGUID uint64, faction uint32, petType uint8, createdBySpell, petExperience, petNextLevelXP uint32, boundingRadius, combatReach, x, y, z, o float32) []byte {
	values := make([]uint32, creatureValuesCount)
	values[0] = uint32(petGUID)
	values[1] = uint32(petGUID >> 32)
	values[objectFieldType] = creatureTypeMask
	values[objectFieldEntry] = entry
	values[objectFieldScale] = math.Float32bits(1.0)
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
	values[unitFieldBoundingRadius] = math.Float32bits(boundingRadius)
	values[unitFieldCombatReach] = math.Float32bits(combatReach)
	values[unitFieldDisplayID] = modelID
	values[unitFieldNativeDisplayID] = modelID
	values[unitFieldSummonedBy] = uint32(ownerGUID)
	values[unitFieldSummonedBy+1] = uint32(ownerGUID >> 32)
	values[unitFieldCreatedBy] = uint32(ownerGUID)
	values[unitFieldCreatedBy+1] = uint32(ownerGUID >> 32)
	values[unitFieldCreatedBySpell] = createdBySpell
	values[unitFieldPetExperience] = petExperience
	petClass, powerType := uint32(8), uint32(0)
	if petType == 1 {
		petClass, powerType = 1, 2
		values[unitFieldPower1+2] = petFocusMax
		values[unitFieldMaxPower1+2] = petFocusMax
		values[unitFieldPower1+4] = curHappiness
		values[unitFieldMaxPower1+4] = petHappinessMax
	}
	values[unitFieldBytes0] = petClass << 8
	values[unitFieldBytes0] |= powerType << 24
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
	petGUID := uint64(s.server.nextPetLowGUID()) | (uint64(0xF140) << 48)
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
		packet := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 0, uint32(createdBySpell), spellCastFlagGo, uint32(time.Now().UnixMilli()), nil, nil, protocol.SpellTargetData{})
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
		updateBlock = buildCreatureUpdate(creatureSpawn{GUID: uint32(petGUID), RawGUID: petGUID, Entry: entry, Map: s.player.Map, X: petX, Y: petY, Z: petZ, Orientation: petO, Model: modelID, Faction: faction, NPCFlags: uint32(npcFlags), UnitFlags: stats.UnitFlags, UnitFlags2: unitFlag2RegeneratePower, DynamicFlags: uint32(dynamicFlags), CreatedBySpell: uint32(createdBySpell), Level: stats.Level, Health: stats.Health, MaxHealth: stats.MaxHealth, Scale: float32(scale), HoverHeight: float32(hoverHeight), BoundingRadius: petBoundingRadius, CombatReach: petCombatReach, WalkSpeed: float32(walkSpeed), RunSpeed: float32(runSpeed), AttackTime: stats.AttackTime, RangedAttack: uint32(rangeAttack)})
	} else {
		attributes := s.getPetAttributes(ctx, entry, level, uint8(petType))
		s.registerPetMotion(ctx, petGUID, petID, uint8(petType), entry, level, faction, curHealth, maxHealth, curMana, maxMana, attributes, uint32(petHappiness), uint32(petExperience), petNextLevelXP, uint8(reactState), petCombatReach, petX, petY, petZ, petO)
		updateBlock = buildPetUpdate(petGUID, petID, entry, level, modelID, curHealth, maxHealth, curMana, maxMana, attributes, uint32(petHappiness), s.playerGUID, faction, uint8(petType), uint32(createdBySpell), uint32(petExperience), petNextLevelXP, petBoundingRadius, petCombatReach, petX, petY, petZ, petO)
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
	loaded := make([]*activeAura, 0)
	offlineMs := int64(0)
	var petSaveTime int64
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(savetime, 0) FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, petID).Scan(&petSaveTime)
	now := time.Now().Unix()
	if petSaveTime > 0 && now > petSaveTime {
		offlineMs = (now - petSaveTime) * 1000
	}
	s.server.auraMu.Lock()
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[uint64]map[uint32]struct{})
	}
	if s.server.creatureAuras[petGUID] == nil {
		s.server.creatureAuras[petGUID] = make(map[uint32]struct{})
	}
	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[uint64]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[petGUID] == nil {
		s.server.activeCreatureAuras[petGUID] = make(map[uint32]*activeAura)
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
		aura := &activeAura{SpellID: uint32(spellID), CasterGUID: casterGUID, TargetGUID: petGUID, EffectMask: uint8(effectMask) & 0x07, RecalculateMask: uint8(recalculateMask), CritChance: float32(critChance), ApplyResilience: applyResilience, Slot: uint8(len(s.server.activeCreatureAuras[petGUID]) % 64), Positive: true, CasterLevel: s.player.Level}
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
				aura.Amount = uint32(effect.BasePoints + 1)
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
		s.server.creatureAuras[petGUID][aura.SpellID] = struct{}{}
		s.server.activeCreatureAuras[petGUID][aura.SpellID] = aura
		loaded = append(loaded, aura)
	}
	s.server.auraMu.Unlock()
	if len(loaded) == 0 {
		return
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
		records = append(records, protocol.AuraUpdateRecord{CasterGUID: aura.CasterGUID, Slot: aura.Slot, SpellID: aura.SpellID, EffectMask: aura.EffectMask, Positive: aura.Positive, MaxDurationMs: maxDuration, DurationMs: duration, CasterLevel: aura.CasterLevel, StackCount: stackCount})
		if aura.PeriodMs > 0 {
			s.scheduleCreaturePeriodicTick(aura, aura.PeriodMs)
		}
		if aura.DurationMs > 0 && aura.DurationMs < 18000000 {
			aura.Timer = time.AfterFunc(time.Duration(aura.RemainingMs)*time.Millisecond, func(spellID uint32, slot uint8) func() {
				return func() { s.expireCreatureAura(petGUID, spellID, slot) }
			}(aura.SpellID, aura.Slot))
		}
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE_ALL), protocol.BuildAuraUpdateAll(petGUID, records), true)
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
		s.server.clearCreatureAuras(petGUID)
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

	if s.server != nil {
		s.server.motionMu.Lock()
		if s.server.creatureMotion != nil {
			delete(s.server.creatureMotion, petGUID)
		}
		s.server.motionMu.Unlock()
	}

	s.player.PetGUID = 0
	s.player.PetNumber = 0
	s.sendPlayerUpdate()
	s.debug("pet unsummoned", "account", s.accountName, "petID", petID, "mode", mode)
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
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && entry != 0 {
		var fam int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT family FROM creature_template WHERE entry = ?", entry).Scan(&fam); err == nil {
			family = uint16(fam)
		}
	}
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(abdata, '') FROM character_pet WHERE id = ?", petID).Scan(&abdata)

	petGUID := s.petGUIDForNumber(petID)
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
	buf.WriteU8(uint8(len(allSpells)))
	for _, sp := range allSpells {
		actType := uint32(sp.active)
		buf.WriteU32(sp.spellID | (actType << 24))
	}

	type petCooldown struct {
		spell, category  uint32
		end, categoryEnd time.Time
	}
	cooldowns := make([]petCooldown, 0)
	now := time.Now()
	s.server.motionMu.Lock()
	if motion := s.server.creatureMotion[s.player.PetGUID]; motion != nil {
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

func (s *session) handleSummonPet(ctx context.Context, spellID uint32, entry uint32) {
	if s.player == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
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
			return
		}
		if s.player.PetGUID != 0 {
			if s.activePetNumber() == uint32(petID) {
				return // already active
			}
			s.unsummonPet(ctx, petSaveNotInSlot)
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE character_pet SET slot = 0 WHERE id = ? AND owner = ?", petID, s.playerGUID)
		maxHP, _, maxMana, _ := s.getPetStats(ctx, uint32(pEntry), uint32(level), uint8(petType))
		if maxHP == 0 {
			maxHP = uint32(curHealth)
		}
		if maxMana == 0 {
			maxMana = uint32(curMana)
		}
		s.spawnPet(ctx, uint32(petID), uint32(pEntry), petName, uint32(level), uint32(modelID), uint32(curHealth), maxHP, uint32(curMana), maxMana, uint8(reactState))
		return
	}

	var petID, modelID, level, petType, reactState, curHealth, curMana int64
	var petName string
	err := cdb.QueryRowContext(ctx,
		"SELECT id, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? AND entry = ? LIMIT 1",
		s.playerGUID, entry).Scan(&petID, &modelID, &level, &petName, &curHealth, &curMana, &petType, &reactState)

	if err == nil && petID > 0 {
		if s.player.PetGUID != 0 {
			if s.activePetNumber() == uint32(petID) {
				return // already active
			}
			s.unsummonPet(ctx, petSaveNotInSlot)
		}

		maxHP, _, maxMP, _ := s.getPetStats(ctx, entry, uint32(level), uint8(petType))
		curHealth = int64(maxHP)
		curMana = int64(maxMP)

		_, _ = cdb.ExecContext(ctx,
			"UPDATE character_pet SET slot = 0, level = ?, curhealth = ?, curmana = ?, savetime = ? WHERE id = ? AND owner = ?",
			level, curHealth, curMana, time.Now().Unix(), petID, s.playerGUID)

		spells := getPetSpellsForLevel(entry, uint32(level))
		for _, sp := range spells {
			_, _ = cdb.ExecContext(ctx, "INSERT OR IGNORE INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, sp.id, sp.active)
		}

		s.spawnPet(ctx, uint32(petID), entry, petName, uint32(level), uint32(modelID), uint32(curHealth), maxHP, uint32(curMana), maxMP, uint8(reactState))
		return
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
	maxHP, _, maxMP, _ := s.getPetStats(ctx, entry, playerLevel, 0)

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

	s.spawnPet(ctx, newPetID, entry, petName, playerLevel, model, maxHP, maxHP, maxMP, maxMP, 1)
}

func (s *session) handleDismissPet(ctx context.Context) {
	s.unsummonPet(ctx, petSaveNotInSlot)
}

func (s *session) handleResurrectPet(ctx context.Context, spellID uint32) {
	if s.player == nil {
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}

	var petID, entry, modelID, level, petType, reactState int64
	var petName string
	err := cdb.QueryRowContext(ctx,
		"SELECT id, entry, modelid, level, COALESCE(PetType, 0), name, COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? ORDER BY slot ASC LIMIT 1",
		s.playerGUID).Scan(&petID, &entry, &modelID, &level, &petType, &petName, &reactState)
	if err != nil || petID == 0 {
		return
	}

	maxHP, _, maxMP, _ := s.getPetStats(ctx, uint32(entry), uint32(level), uint8(petType))
	_, _ = cdb.ExecContext(ctx,
		"UPDATE character_pet SET slot = 0, curhealth = ?, curmana = ?, savetime = ? WHERE id = ? AND owner = ?",
		maxHP, maxMP, time.Now().Unix(), petID, s.playerGUID)

	if s.player.PetGUID != 0 && s.activePetNumber() == uint32(petID) {
		return
	}

	s.spawnPet(ctx, uint32(petID), uint32(entry), petName, uint32(level), uint32(modelID), maxHP, maxHP, maxMP, maxMP, uint8(reactState))
}

func (s *session) handleTameCreature(ctx context.Context, spellID uint32, targetGUID uint64) {
	if s.player == nil || s.player.Class != 3 || targetGUID == 0 { // Hunter only
		return
	}
	if s.player.PetGUID != 0 {
		return // already has an active pet
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
		destroyBuf := protocol.NewBuffer(9)
		destroyBuf.WriteU64(targetGUID)
		destroyBuf.WriteU8(0)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_DESTROY_OBJECT), destroyBuf.Bytes(), s)
	}

	var maxPetID int64
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM character_pet").Scan(&maxPetID)
	newPetID := uint32(maxPetID + 1)

	playerLevel := uint32(s.player.Level)
	petLevel := playerLevel
	var modelID uint32
	var cName string
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil && targetEntry != 0 {
		var mID int64
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name, modelid1 FROM creature_template WHERE entry = ?", targetEntry).Scan(&cName, &mID)
		modelID = uint32(mID)
	}
	if cName == "" {
		cName = "Pet"
	}

	maxHP, _, maxMP, _ := s.getPetStats(ctx, targetEntry, petLevel, 1)
	now := time.Now().Unix()

	_, _ = cdb.ExecContext(ctx,
		"INSERT INTO character_pet (id, entry, owner, modelid, CreatedBySpell, PetType, level, exp, Reactstate, name, renamed, slot, curhealth, curmana, curhappiness, savetime, abdata) VALUES (?, ?, ?, ?, ?, 1, ?, 0, 1, ?, 0, 0, ?, ?, 1050000, ?, '')",
		newPetID, targetEntry, s.playerGUID, modelID, spellID, petLevel, cName, maxHP, maxMP, now)

	spells := getPetSpellsForLevel(targetEntry, petLevel)
	for _, sp := range spells {
		_, _ = cdb.ExecContext(ctx, "INSERT INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", newPetID, sp.id, sp.active)
	}

	s.spawnPet(ctx, newPetID, targetEntry, cName, petLevel, modelID, maxHP, maxHP, maxMP, maxMP, 1)
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
	motion := s.server.creatureMotion[s.player.PetGUID]
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
	motion := s.server.creatureMotion[feed.PetGUID]
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
	if !s.consumeInventoryItemByGUID(ctx, feed.ItemGUID, 1) {
		return
	}
	s.castSpellDirectWithBasePoint(ctx, triggerSpell, feed.PetGUID, feed.Benefit)
	s.debug("pet fed", "account", s.accountName, "petID", s.activePetNumber(), "item", feed.ItemEntry, "benefit", feed.Benefit, "effect", effectIndex)
}

var stableSlotPrices = []uint32{
	500,     // 5s
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
)

// handleBuyStableSlot processes CMSG_BUY_STABLE_SLOT (0x272).
// Reference: WorldSession::HandleBuyStableSlot (NPCHandler.cpp:563).
func (s *session) handleBuyStableSlot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64() // npcGUID

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}

	var purchasedCount int
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_pet WHERE owner = ? AND slot > 0", s.playerGUID).Scan(&purchasedCount)

	if purchasedCount >= len(stableSlotPrices) {
		res := protocol.NewBuffer(1)
		res.WriteU8(stableErrStable)
		_ = s.write(uint16(protocol.OpcodeSMSG_STABLE_RESULT), res.Bytes(), true)
		return true
	}

	cost := stableSlotPrices[purchasedCount]
	if s.player.Money < cost {
		res := protocol.NewBuffer(1)
		res.WriteU8(stableErrMoney)
		_ = s.write(uint16(protocol.OpcodeSMSG_STABLE_RESULT), res.Bytes(), true)
		return true
	}

	s.player.Money -= cost
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)

	res := protocol.NewBuffer(1)
	res.WriteU8(stableSuccessBuySlot)
	_ = s.write(uint16(protocol.OpcodeSMSG_STABLE_RESULT), res.Bytes(), true)
	s.sendPlayerUpdate()
	s.debug("stable slot purchased", "account", s.accountName, "slot", purchasedCount+1)
	return true
}

// handleDismissCritter processes CMSG_DISMISS_CRITTER (0x48D).
func (s *session) handleDismissCritter(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	critterGUID, _ := r.ReadU64()
	s.debug("dismiss critter", "account", s.accountName, "critter", critterGUID)
	return true
}

// handlePetAbandon processes CMSG_PET_ABANDON (0x176).
// Reference: WorldSession::HandlePetAbandonOpcode (PetHandler.cpp:52).
func (s *session) handlePetAbandon(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
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

// checkStableMaster mirrors WorldSession::CheckStableMaster: the player
// opening their own stable requires GM state, otherwise the target creature
// must be a stablemaster.
func (s *session) checkStableMaster(ctx context.Context, guid uint64) bool {
	if guid == s.playerGUID {
		return s.player.ExtraFlags&playerExtraGMOn != 0
	}
	return s.creatureHasNpcFlag(ctx, guid, npcFlagStablemaster)
}

// sendPetStableResult mirrors WorldSession::SendPetStableResult.
func (s *session) sendPetStableResult(code uint8) {
	buf := protocol.NewBuffer(1)
	buf.WriteU8(code)
	_ = s.write(uint16(protocol.OpcodeSMSG_STABLE_RESULT), buf.Bytes(), true)
}

// handleStablePet processes CMSG_STABLE_PET (0x270).
// Reference: WorldSession::HandleStablePet (NPCHandler.cpp:410): a living
// player stables its active hunter pet (slot 0) into the first free stable
// slot; results flow through SMSG_STABLE_RESULT.
func (s *session) handleStablePet(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	if s.player.Health == 0 || !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var currentPetType, currentHealth int64
	err = cdb.QueryRowContext(ctx, "SELECT PetType, curhealth FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&currentPetType, &currentHealth)
	if err != nil || currentPetType != 1 || currentHealth <= 0 { // only living hunter pets
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var freeSlot int64 = -1
	rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_pet WHERE owner = ? AND slot > 0 ORDER BY slot", s.playerGUID)
	if err == nil {
		taken := map[int64]struct{}{}
		for rows.Next() {
			var slot int64
			if rows.Scan(&slot) == nil {
				taken[slot] = struct{}{}
			}
		}
		rows.Close()
		for slot := int64(1); slot <= 4; slot++ {
			if _, used := taken[slot]; !used {
				freeSlot = slot
				break
			}
		}
	}
	if freeSlot < 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ?, savetime = ? WHERE owner = ? AND slot = 0", freeSlot, time.Now().Unix(), s.playerGUID); err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	s.sendPetStableResult(stableSuccessStable)
	return true
}

// handleStableRevivePet processes CMSG_STABLE_REVIVE_PET (0x274).
// Reference: WorldSession::HandleStableRevivePet (NPCHandler.cpp:455): the
// stablemaster revives the player's dead current pet.
func (s *session) handleStableRevivePet(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	if !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var level int64
	if err := cdb.QueryRowContext(ctx, "SELECT level FROM character_pet WHERE owner = ? AND slot = 0 AND curhealth <= 0", s.playerGUID).Scan(&level); err != nil {
		// nothing dead to revive is still a successful interaction
		s.sendPetStableResult(stableSuccessUnslot)
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET curhealth = 1 WHERE owner = ? AND slot = 0", s.playerGUID); err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	s.sendPetStableResult(stableSuccessUnslot)
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
		return false
	}
	petNumber, err := r.ReadU32()
	if err != nil {
		return false
	}
	if !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var stabledSlot int64
	err = cdb.QueryRowContext(ctx, "SELECT slot FROM character_pet WHERE owner = ? AND id = ? AND slot > 0", s.playerGUID, petNumber).Scan(&stabledSlot)
	if err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = ? WHERE owner = ? AND slot = 0", stabledSlot, s.playerGUID); err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if _, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = 0, savetime = ? WHERE owner = ? AND id = ?", time.Now().Unix(), s.playerGUID, petNumber); err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
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
		return false
	}
	petNumber, err := r.ReadU32()
	if err != nil {
		return false
	}
	if !s.checkStableMaster(ctx, npcGUID) {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	var active int64
	if err := cdb.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&active); err != nil || active > 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	result, err := cdb.ExecContext(ctx, "UPDATE character_pet SET slot = 0, savetime = ? WHERE owner = ? AND id = ? AND slot > 0", time.Now().Unix(), s.playerGUID, petNumber)
	if err != nil {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		s.sendPetStableResult(stableErrStable)
		return true
	}
	s.sendPetStableResult(stableSuccessUnslot)
	return true
}

// Pet command handlers below use the live creature-motion pet entity and
// persisted pet state. Reference:
// PetHandler.cpp HandlePetAction (73), HandlePetCancelAuraOpcode (215),
// HandlePetCastSpellOpcode (241), HandlePetLearnTalent (265), HandlePetRename
// (284), HandlePetSetAction (328), HandlePetSpellAutocastOpcode (365),

// handlePetAction processes CMSG_PET_ACTION (0x175).
// Reference: WorldSession::HandlePetAction (PetHandler.cpp:73-259).
func (s *session) handlePetAction(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	action, err := r.ReadU32()
	if err != nil {
		return false
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

		aiReactionHostile uint32 = 2
	)

	switch actFlag {
	case actCommand:
		switch spellOrAction {
		case commandAttack:
			if s.server != nil {
				s.server.onPetCommandAttack(petGUID, targetGUID)
			}
			// Send hostile AI reaction (plays pet attack sound/growl)
			reactionBuf := protocol.NewBuffer(12)
			reactionBuf.WriteU64(petGUID)
			reactionBuf.WriteU32(aiReactionHostile)
			_ = s.write(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), true)
			if s.server != nil {
				s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), s)
			}
			s.debug("pet attack command", "account", s.accountName, "pet", petGUID, "target", targetGUID)
		case commandFollow:
			if s.server != nil {
				s.server.onPetCommandFollow(petGUID)
			}
			stopPkt := buildAttackStop(petGUID, 0, false)
			_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
			s.debug("pet follow command", "account", s.accountName, "pet", petGUID)
		case commandStay:
			if s.server != nil {
				s.server.onPetCommandStay(petGUID)
			}
			stopPkt := buildAttackStop(petGUID, 0, false)
			_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_STOP), stopPkt, true)
			s.debug("pet stay command", "account", s.accountName, "pet", petGUID)
		case commandAbandon:
			s.unsummonPet(ctx, petSaveAsDeleted)
			s.debug("pet abandoned via command", "account", s.accountName, "pet", petGUID)
		}
	case actReaction:
		if s.server != nil {
			s.server.onPetSetReaction(petGUID, uint8(spellOrAction))
		}
		// Save react state (0 = passive, 1 = defensive, 2 = aggressive)
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_pet SET Reactstate = ? WHERE owner = ? AND slot = 0", spellOrAction, s.playerGUID)
		}
		s.debug("pet reaction state changed", "account", s.accountName, "pet", petGUID, "react", spellOrAction)
	case actDisabled, actEnabled, actPassive:
		if targetGUID != 0 {
			reactionBuf := protocol.NewBuffer(12)
			reactionBuf.WriteU64(petGUID)
			reactionBuf.WriteU32(aiReactionHostile)
			_ = s.write(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), true)
			if s.server != nil {
				s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AI_REACTION), reactionBuf.Bytes(), s)
			}
			if spellOrAction != 0 {
				castTimeStamp := uint32(time.Now().UnixMilli())
				spellTarget := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
				goPkt := protocol.BuildSpellGo(petGUID, petGUID, 1, spellOrAction, spellCastFlagGo, castTimeStamp, []uint64{targetGUID}, nil, spellTarget)
				_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
				if s.server != nil {
					s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, s)
				}
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
	petNumber := s.petNumberForGUID(petGUID)
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM pet_aura WHERE guid = ? AND spell = ?", petNumber, spellID)
	}
	s.debug("pet cancel aura", "account", s.accountName, "pet", petNumber, "spell", spellID)
	return true
}

func (s *session) handlePetCastSpell(ctx context.Context, payload []byte) bool {
	if len(payload) < 13 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	castCount, err := r.ReadU8()
	if err != nil {
		return false
	}
	spellID, err := r.ReadU32()
	if err != nil {
		return false
	}
	castFlags, _ := r.ReadU8()
	target, targetErr := protocol.ReadSpellTargetData(r)
	if targetErr != nil {
		return false
	}
	if castFlags&0x02 != 0 {
		_, _ = r.ReadF32()
		_, _ = r.ReadF32()
	}
	if spellID == 0 || s.server == nil || s.server.Data == nil || s.player == nil {
		return true
	}
	motion, ok := s.petMotionForCast(petGUID)
	if !ok {
		return true
	}
	spell, found, spellErr := s.server.Data.Spell(spellID)
	if spellErr != nil || !found || spell.Attributes&spellAttributePassive != 0 || !s.petKnowsSpell(ctx, motion, spellID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
		return true
	}
	if target.UnitGUID == 0 {
		if isSelfCastOnly(spell) {
			target.UnitGUID = motion.GUID
			target.Flags |= protocol.SpellTargetFlagUnitWireMask
		} else if s.selection != 0 {
			target.UnitGUID = s.selection
			target.Flags |= protocol.SpellTargetFlagUnitWireMask
		}
	}
	if isHarmfulSpell(spell) && target.UnitGUID == motion.GUID && !isSelfCastOnly(spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
		return true
	}
	if target.UnitGUID != 0 && target.UnitGUID != motion.GUID {
		targetState, targetFound := s.getCombatTarget(ctx, target.UnitGUID)
		if !targetFound || targetState.Map != motion.Map {
			_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
			return true
		}
		if spell.RangeIndex > 0 {
			if rangeInfo, rangeFound, rangeErr := s.server.Data.SpellRange(spell.RangeIndex); rangeErr == nil && rangeFound && rangeInfo.MaxHostile > 0 {
				if distance3D(motion.X, motion.Y, motion.Z, targetState.X, targetState.Y, targetState.Z) > float64(rangeInfo.MaxHostile)+5 {
					_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, 97), true)
					return true
				}
			}
		}
	}
	if !s.checkPetSpellPower(motion, spell, castCount) {
		return true
	}
	now := time.Now()
	categoryID, _, categoryErr := s.spellCooldownCategory(spellID)
	s.server.motionMu.Lock()
	prunePetSpellCooldowns(motion, now)
	spellCooldownEnd := motion.SpellCooldowns[spellID]
	categoryEnd := time.Time{}
	if categoryErr == nil && categoryID != 0 {
		categoryEnd = motion.SpellCategoryCooldowns[categoryID]
	}
	s.server.motionMu.Unlock()
	if !spellCooldownEnd.IsZero() && !spellCooldownEnd.Before(now) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
		return true
	}
	if !categoryEnd.IsZero() && !categoryEnd.Before(now) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedNotReady), true)
		return true
	}
	if !s.executePetSpell(ctx, motion, spell, castCount, target) {
		_ = s.write(uint16(protocol.OpcodeSMSG_PET_CAST_FAILED), buildCastFailed(castCount, spellID, spellFailedBadTargets), true)
	}
	s.debug("pet cast spell", "account", s.accountName, "pet", petGUID, "spell", spellID, "castCount", castCount)
	return true
}

func (s *session) petMotionForCast(petGUID uint64) (*creatureMotion, bool) {
	if s == nil || s.server == nil || petGUID == 0 {
		return nil, false
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.creatureMotion[petGUID]
	if motion == nil {
		low := uint32(petGUID & 0x00FFFFFF)
		entry := uint32((petGUID >> 24) & 0x00FFFFFF)
		motion = s.server.creatureMotion[creatureWorldGUID(low, entry)]
	}
	if motion == nil || motion.Health == 0 || (motion.OwnerGUID != s.playerGUID && motion.CharmerGUID != s.playerGUID) {
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

func (s *session) handlePetLearnTalent(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	talentID, _ := r.ReadU32()
	rank, _ := r.ReadU32()
	petNumber := s.petNumberForGUID(petGUID)
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.server.Data != nil {
		if tEntry, ok, err := s.server.Data.Talent(talentID); err == nil && ok && rank < 5 {
			spellID := tEntry.SpellRank[rank]
			if spellID != 0 {
				if rank > 0 && tEntry.SpellRank[rank-1] != 0 {
					oldSpell := tEntry.SpellRank[rank-1]
					_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petNumber, oldSpell)
					unlearnBuf := protocol.NewBuffer(4)
					unlearnBuf.WriteU32(oldSpell)
					_ = s.write(uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL), unlearnBuf.Bytes(), true)
				}
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, 1)", petNumber, spellID)
				learnedBuf := protocol.NewBuffer(4)
				learnedBuf.WriteU32(spellID)
				_ = s.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learnedBuf.Bytes(), true)
				s.sendTalentsInfo(true)
				s.sendPetSpells(ctx, petNumber, 0, 1)
			}
		}
	}
	s.debug("pet learn talent", "account", s.accountName, "pet", petNumber, "talent", talentID, "rank", rank)
	return true
}

func (s *session) handlePetNameQuery(ctx context.Context, payload []byte) bool {
	if len(payload) < 5 {
		return true
	}
	r := protocol.NewReader(payload)
	petNumber, err := r.ReadU32()
	if err != nil {
		return false
	}
	if _, err := r.ReadPackedGUID(); err != nil {
		return false
	}

	petName := ""
	var saveTime uint32
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT name, savetime FROM character_pet WHERE id = ? LIMIT 1", petNumber).Scan(&petName, &saveTime)
	}

	buf := protocol.NewBuffer(32 + len(petName))
	buf.WriteU32(petNumber)
	buf.WriteCString(petName)
	buf.WriteU32(saveTime) // timestamp
	buf.WriteU8(0)         // declined
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
	if newName == "" {
		return true
	}
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		petNumber := s.petNumberForGUID(petGUID)
		now := time.Now().Unix()
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE character_pet SET name = ?, renamed = 1, savetime = ? WHERE id = ? AND owner = ?",
			newName, now, petNumber, s.playerGUID)
	}
	return true
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
		return false
	}
	count := 1
	if len(payload) >= 24 {
		count = 2
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}

	var existingAbdata string
	_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(abdata, '') FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&existingAbdata)

	type actionSlot struct {
		actType uint8
		action  uint32
	}
	slots := make([]actionSlot, 10)
	slots[0] = actionSlot{actType: 0x07, action: 2} // Attack
	slots[1] = actionSlot{actType: 0x07, action: 1} // Follow
	slots[2] = actionSlot{actType: 0x07, action: 0} // Stay
	slots[7] = actionSlot{actType: 0x06, action: 2} // Aggressive
	slots[8] = actionSlot{actType: 0x06, action: 1} // Defensive
	slots[9] = actionSlot{actType: 0x06, action: 0} // Passive

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

	for i := 0; i < count; i++ {
		pos, pErr := r.ReadU32()
		data, dErr := r.ReadU32()
		if pErr != nil || dErr != nil || pos >= 10 {
			continue
		}
		aType := uint8((data >> 24) & 0xFF)
		aAction := data & 0x00FFFFFF
		slots[pos] = actionSlot{actType: aType, action: aAction}

		// Synchronize pet_spell table when spell action buttons change active state
		petNumber := s.petNumberForGUID(petGUID)
		if aAction > 0 && petNumber > 0 {
			if aType == 0xC1 { // ACT_ENABLED (autocast enabled)
				_, _ = cdb.ExecContext(ctx, "UPDATE pet_spell SET active = 1 WHERE guid = ? AND spell = ?", petNumber, aAction)
			} else if aType == 0x81 { // ACT_DISABLED (autocast disabled)
				_, _ = cdb.ExecContext(ctx, "UPDATE pet_spell SET active = 0 WHERE guid = ? AND spell = ?", petNumber, aAction)
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

	// Update pet_spell table
	res, err := cdb.ExecContext(ctx, "UPDATE pet_spell SET active = ? WHERE guid = ? AND spell = ?", active, petID, spellID)
	if err == nil {
		if rows, _ := res.RowsAffected(); rows == 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO pet_spell (guid, spell, active) VALUES (?, ?, ?)", petID, spellID, active)
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
		s.server.onPetToggleAutocast(petGUID, spellID, active != 0)
	}
}

// handlePetSpellAutocast processes CMSG_PET_SPELL_AUTOCAST (0x2F3).
// Reference: WorldSession::HandlePetSpellAutocastOpcode (PetHandler.cpp:694).
func (s *session) handlePetSpellAutocast(ctx context.Context, payload []byte) bool {
	if len(payload) < 13 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	spellID, err := r.ReadU32()
	if err != nil {
		return false
	}
	state, err := r.ReadU8()
	if err != nil {
		return false
	}
	petNumber := s.petNumberForGUID(petGUID)
	s.syncPetSpellAutocast(ctx, petNumber, spellID, state)
	s.debug("pet spell autocast", "account", s.accountName, "pet", petNumber, "spell", spellID, "state", state)
	return true
}

// handlePetStopAttack processes CMSG_PET_STOP_ATTACK (0x2EA).
// Reference: WorldSession::HandlePetStopAttack (PetHandler.cpp:401).
func (s *session) handlePetStopAttack(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	s.debug("pet stop attack", "account", s.accountName, "pet", petGUID)
	return true
}

// handleRequestPetInfo processes CMSG_REQUEST_PET_INFO (0x279).
// Reference: WorldSession::HandleRequestPetInfoOpcode (PetHandler.cpp:412) and Player::SendPetSpells (Player.cpp:21107-21145).
func (s *session) handleRequestPetInfo(ctx context.Context, payload []byte) bool {
	if s.player == nil {
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
// Reference: WorldSession::HandleListStabledPetsOpcode (NPCHandler.cpp:520).
func (s *session) handleListStabledPets(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	npcGUID, _ := r.ReadU64()

	type petInfo struct {
		ID    uint32
		Entry uint32
		Level uint32
		Name  string
		Slot  uint8
	}
	var pets []petInfo
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT id, entry, level, name, slot FROM character_pet WHERE owner = ? AND slot > 0 ORDER BY slot", s.playerGUID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p petInfo
				if err := rows.Scan(&p.ID, &p.Entry, &p.Level, &p.Name, &p.Slot); err == nil {
					pets = append(pets, p)
				}
			}
		}
	}

	buf := protocol.NewBuffer(16 + len(pets)*32)
	buf.WriteU64(npcGUID)
	buf.WriteU8(uint8(len(pets)))
	buf.WriteU8(4) // num slots
	for _, p := range pets {
		buf.WriteU32(p.ID)
		buf.WriteU32(p.Entry)
		buf.WriteU32(p.Level)
		buf.WriteCString(p.Name)
		buf.WriteU8(p.Slot)
	}
	_ = s.write(uint16(protocol.OpcodeMSG_LIST_STABLED_PETS), buf.Bytes(), true)
	return true
}

// handleLearnPreviewTalentsPet processes CMSG_LEARN_PREVIEW_TALENTS_PET (0x4C2).
// Reference: WorldSession::HandleLearnPreviewTalentsPet (PetHandler.cpp:430).
func (s *session) handleLearnPreviewTalentsPet(ctx context.Context, payload []byte) bool {
	if len(payload) < 12 {
		return true
	}
	r := protocol.NewReader(payload)
	petGUID, _ := r.ReadU64()
	talentCount, _ := r.ReadU32()
	petNumber := s.petNumberForGUID(petGUID)

	for i := uint32(0); i < talentCount && r.Remaining() >= 8; i++ {
		talentID, _ := r.ReadU32()
		rank, _ := r.ReadU32()
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.server.Data != nil {
			if tEntry, ok, err := s.server.Data.Talent(talentID); err == nil && ok && rank < 5 {
				spellID := tEntry.SpellRank[rank]
				if spellID != 0 {
					if rank > 0 && tEntry.SpellRank[rank-1] != 0 {
						oldSpell := tEntry.SpellRank[rank-1]
						_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM pet_spell WHERE guid = ? AND spell = ?", petNumber, oldSpell)
						unlearnBuf := protocol.NewBuffer(4)
						unlearnBuf.WriteU32(oldSpell)
						_ = s.write(uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL), unlearnBuf.Bytes(), true)
					}
					_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "INSERT OR REPLACE INTO pet_spell (guid, spell, active) VALUES (?, ?, 1)", petNumber, spellID)
					learnedBuf := protocol.NewBuffer(4)
					learnedBuf.WriteU32(spellID)
					_ = s.write(uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL), learnedBuf.Bytes(), true)
				}
			}
		}
	}
	s.sendTalentsInfo(true)
	s.sendPetSpells(ctx, petNumber, 0, 1)
	s.debug("pet preview talents learned", "account", s.accountName, "pet", petGUID, "count", talentCount)
	return true
}
