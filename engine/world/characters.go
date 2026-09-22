package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	characterFlagHideHelm        uint32 = 0x00000400
	characterFlagHideCloak       uint32 = 0x00000800
	characterFlagGhost           uint32 = 0x00002000
	characterFlagRename          uint32 = 0x00004000
	characterFlagLockedByBilling uint32 = 0x01000000
	characterFlagDeclined        uint32 = 0x02000000
	playerFlagGhost              uint32 = 0x00000010
	playerFlagInPVP              uint32 = 0x00000200
	playerFlagPVPTimer           uint32 = 0x00040000
	playerFlagContestedPVP       uint32 = 0x00000100
	characterCustomizeNone       uint32 = 0
	characterCustomizeCustomize  uint32 = 0x00000001
	characterCustomizeFaction    uint32 = 0x00010000
	characterCustomizeRace       uint32 = 0x00100000
	atLoginRename                uint64 = 0x001
	atLoginResetSpells           uint64 = 0x002
	atLoginResetTalents          uint64 = 0x004 // AT_LOGIN_RESET_TALENTS (Player.h:459)
	atLoginCustomize             uint64 = 0x008
	atLoginResetPetTalents       uint64 = 0x010
	atLoginFirst                 uint64 = 0x020
	atLoginChangeFaction         uint64 = 0x040
	atLoginChangeRace            uint64 = 0x080
	atLoginResurrect             uint64 = 0x100
	inventorySlotBagEnd                 = 23
	charCreateSuccess                   = 47
	charCreateError                     = 48
	charCreateFailed                    = 49
	charCreateDisabled                  = 51
	charCreateServerLimit               = 53
	charCreateAccountLimit              = 54
	charCreateExpansion                 = 57
	charCreateExpansionClass            = 58
	charCreateLevelRequirement          = 59
	charCreateUniqueClassLimit          = 60
	charCreateNameInUse                 = 50
	charDeleteSuccess                   = 71
	charDeleteFailed                    = 72
)

type enumCharacter struct {
	GUID        uint64
	Name        string
	Race        uint8
	Class       uint8
	Gender      uint8
	Skin        uint8
	Face        uint8
	HairStyle   uint8
	HairColor   uint8
	FacialStyle uint8
	Level       uint8
	Zone        uint32
	Map         uint32
	X           float32
	Y           float32
	Z           float32
	GuildID     uint32
	PlayerFlags uint32
	AtLogin     uint16
	PetEntry    uint32
	PetDisplay  uint32
	PetLevel    uint32
	Equipment   string
	Banned      bool
}

func (s *session) handleCharEnum(ctx context.Context) bool {
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_DEL_EXPIRED_BANS"); err != nil {
		s.debug("character enumeration cleanup failed", "account", s.accountName, "error", err)
	}
	rows, err := s.server.CharactersStore.QueryStatement(ctx, "CHAR_SEL_ENUM", 0, s.accountID)
	if err != nil {
		s.debug("character enumeration query failed", "account", s.accountName, "error", err)
		return false
	}
	defer rows.Close()
	packet := protocol.NewBuffer(1 + 128)
	packet.WriteU8(0)
	s.legitimate = make(map[uint64]struct{})
	characters := make([]enumCharacter, 0)
	for rows.Next() {
		character, err := scanEnumCharacter(rows)
		if err != nil {
			s.debug("character enumeration scan failed", "account", s.accountName, "error", err)
			return false
		}
		if character.Race == 0 || character.Class == 0 || character.Gender > 2 {
			continue
		}
		if s.server.Data != nil {
			if valid, known, appearanceErr := s.server.Data.ValidateAppearance(character.Race, character.Class, character.Gender, character.HairStyle, character.HairColor, character.Face, character.FacialStyle, character.Skin); appearanceErr == nil && known && !valid {
				character.Skin, character.Face, character.HairStyle, character.HairColor, character.FacialStyle = 0, 0, 0, 0, 0
				character.AtLogin |= uint16(atLoginCustomize)
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET skin = 0, face = 0, hairStyle = 0, hairColor = 0, facialStyle = 0, at_login = at_login | 8 WHERE guid = ? AND account = ?", character.GUID, s.accountID)
			}
		}
		characters = append(characters, character)
	}
	if err := rows.Err(); err != nil {
		s.debug("character enumeration rows failed", "account", s.accountName, "error", err)
		return false
	}
	rows.Close()
	count := uint8(0)
	for _, character := range characters {
		s.loadEnumEquipment(ctx, &character)
		s.buildEnumCharacter(ctx, packet, character)
		if !character.Banned {
			s.legitimate[character.GUID] = struct{}{}
		}
		if count < 255 {
			count++
		}
	}
	if err := packet.Put(0, []byte{count}); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_CHAR_ENUM), packet.Bytes(), true); err != nil {
		s.debug("character enumeration response failed", "account", s.accountName, "error", err)
		return false
	}
	s.debug("character enumeration sent", "account", s.accountName, "count", count)
	return true
}

func (s *session) loadEnumEquipment(ctx context.Context, character *enumCharacter) {
	if character == nil {
		return
	}
	character.Equipment = s.loadEquipmentCache(ctx, character.GUID, character.Equipment)
}

func (s *session) loadEquipmentCache(ctx context.Context, guid uint64, cached string) string {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return cached
	}
	parts := make([]string, int(inventorySlotBagEnd)*2)
	for i := range parts {
		parts[i] = "0"
	}
	itemSlots := make(map[int64]int64)
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.slot, ii.guid, ii.itemEntry
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ci.bag = 0 AND ci.slot < ? ORDER BY ci.slot`, guid, inventorySlotBagEnd)
	if err != nil {
		return cached
	}
	defer rows.Close()
	for rows.Next() {
		var slot, itemGUID, itemEntry int64
		if rows.Scan(&slot, &itemGUID, &itemEntry) != nil || slot < 0 || slot >= int64(inventorySlotBagEnd) || itemEntry <= 0 {
			continue
		}
		parts[slot*2] = strconv.FormatInt(itemEntry, 10)
		itemSlots[itemGUID] = slot
	}
	if err := rows.Err(); err != nil {
		return cached
	}
	rows.Close()
	for itemGUID, slot := range itemSlots {
		var enchantments sql.NullString
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(enchantments, '') FROM item_instance WHERE guid = ?", itemGUID).Scan(&enchantments); err != nil || !enchantments.Valid {
			continue
		}
		parts[slot*2+1] = strconv.FormatUint(uint64(packVisibleEnchantments(enchantments.String)), 10)
	}
	return strings.Join(parts, " ")
}

func packVisibleEnchantments(raw string) uint32 {
	fields := strings.Fields(raw)
	var packed uint32
	for _, index := range []int{0, 3} {
		if index >= len(fields) {
			continue
		}
		value, err := strconv.ParseUint(fields[index], 10, 16)
		if err == nil {
			packed |= uint32(value) << uint(index/3*16)
		}
	}
	return packed
}

func (s *session) handleCharCreate(ctx context.Context, payload []byte) bool {
	b := protocol.NewReader(payload)
	name, err := b.ReadCString()
	if err != nil {
		return false
	}
	values := make([]uint8, 9)
	for i := range values {
		values[i], err = b.ReadU8()
		if err != nil {
			return false
		}
	}
	race, class, gender := values[0], values[1], values[2]
	if !validCharacterName(name) || race == 0 || class == 0 || gender > 2 {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateFailed)
	}
	if disabled := s.server.Config.CharacterCreatingDisabled; disabled != 0 {
		team := raceTeam(race)
		if (team == 1 && disabled&1 != 0) || (team == 2 && disabled&2 != 0) {
			return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateDisabled)
		}
	}
	raceAllowed, raceRequiredExpansion := s.server.raceDefinition(race)
	classAllowed, classRequiredExpansion := s.server.classDefinition(class)
	if !raceAllowed || s.server.Config.CharacterCreatingDisabledRaceMask&(uint32(1)<<(race-1)) != 0 || !classAllowed || s.server.Config.CharacterCreatingDisabledClassMask&(uint32(1)<<(class-1)) != 0 {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateDisabled)
	}
	if raceRequiredExpansion > uint32(s.accountExpansion) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateExpansion)
	}
	if classRequiredExpansion > uint32(s.accountExpansion) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateExpansionClass)
	}
	if class == 6 {
		if s.server.Config.DeathKnightsPerRealm == 0 {
			return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateUniqueClassLimit)
		}
		var deathKnights int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(guid) FROM characters WHERE account = ? AND class = ?", s.accountID, class).Scan(&deathKnights); err != nil {
			return false
		}
		if deathKnights >= int64(s.server.Config.DeathKnightsPerRealm) {
			return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateUniqueClassLimit)
		}
		if required := s.server.Config.CharacterCreatingMinLevelForDeathKnight; required > 0 {
			var maxLevel sql.NullInt64
			if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT MAX(level) FROM characters WHERE account = ? AND class <> ?", s.accountID, class).Scan(&maxLevel); err != nil {
				return false
			}
			if !maxLevel.Valid || maxLevel.Int64 < int64(required) {
				return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateLevelRequirement)
			}
		}
	}
	row, err := s.server.CharactersStore.QueryRowStatement(ctx, "CHAR_SEL_CHECK_NAME", name)
	if err != nil {
		return false
	}
	var exists int64
	if err := row.Scan(&exists); err == nil {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), 50)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false
	}
	var spawn struct {
		Map, Zone            uint32
		X, Y, Z, Orientation float32
	}
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT map, zone, position_x, position_y, position_z, orientation FROM playercreateinfo WHERE race = ? AND class = ?", race, class).Scan(&spawn.Map, &spawn.Zone, &spawn.X, &spawn.Y, &spawn.Z, &spawn.Orientation); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), 49)
		}
		return false
	}
	var accountCharacters int64
	if err := s.server.CharactersStore.QueryRowContext(ctx, "SELECT COUNT(guid) FROM characters WHERE account = ?", s.accountID).Scan(&accountCharacters); err != nil {
		return false
	}
	if accountCharacters >= int64(s.server.Config.CharactersPerAccount) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateAccountLimit)
	}
	var realmCharacterCount int64
	if err := s.server.CharactersStore.QueryRowContext(ctx, "SELECT COUNT(guid) FROM characters WHERE account = ?", s.accountID).Scan(&realmCharacterCount); err != nil {
		return false
	}
	if realmCharacterCount >= int64(s.server.Config.CharactersPerRealm) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateServerLimit)
	}
	var guid uint64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM characters").Scan(&guid); err != nil {
		return false
	}

	startLevel := uint8(1)
	if s.server.Config.StartPlayerLevel > 0 {
		startLevel = uint8(s.server.Config.StartPlayerLevel)
	}
	if class == 6 { // Death Knight
		if s.server.Config.StartDeathKnightPlayerLevel > 0 {
			startLevel = uint8(s.server.Config.StartDeathKnightPlayerLevel)
		} else {
			startLevel = 55
		}
	}
	startMoney := s.server.Config.StartPlayerMoney
	startHonor := s.server.Config.StartHonorPoints
	startArena := s.server.Config.StartArenaPoints

	args := []any{
		uint32(guid), s.accountID, name, race, class, gender,
		startLevel, uint32(0), startMoney,
		values[3], values[4], values[5], values[6], values[7],
		uint8(0), uint8(0), uint32(0), uint16(spawn.Map), uint32(0), uint8(0),
		spawn.X, spawn.Y, spawn.Z, spawn.Orientation,
		float32(0), float32(0), float32(0), float32(0), uint32(0), "",
		uint8(0), // cinematic = 0
		uint32(0), uint32(0), float32(0), uint32(0), uint8(0), uint32(0), uint32(0),
		uint16(0), uint8(0), uint16(atLoginFirst), uint16(spawn.Zone), uint32(0), "",
		startArena, startHonor, uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0),
		uint64(0), uint32(0), uint32(^uint32(0)), uint32(1),
		uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0),
		uint8(1), uint8(0), "", "", uint32(0), "", uint8(0), uint32(0),
	}
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_CHARACTER", args...); err != nil {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), 48)
	}
	_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_PLAYER_HOMEBIND", guid, spawn.Map, spawn.Zone, spawn.X, spawn.Y, spawn.Z)
	s.initializeCreatedPlayerStats(ctx, guid, race, class, startLevel)

	// Populate starter spells, skills, actions, equipment
	s.createStarterSpells(ctx, guid, race, class)
	s.createStarterSkills(ctx, guid, race, class)
	s.createStarterActions(ctx, guid, race, class)
	s.createStarterOutfit(ctx, guid, race, class, gender)

	var realmCharacters sql.NullInt64
	if err := s.server.AuthStore.QueryRowContext(ctx, "SELECT SUM(numchars) FROM realmcharacters WHERE acctid = ?", s.accountID).Scan(&realmCharacters); err != nil {
		return false
	}
	count := uint32(1)
	if realmCharacters.Valid && realmCharacters.Int64 > 0 {
		count = uint32(realmCharacters.Int64) + 1
	}
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_REP_REALM_CHARACTERS", count, s.accountID, s.server.RealmID); err != nil {
		return false
	}
	s.legitimate[guid] = struct{}{}
	return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), 47)
}

func (s *session) initializeCreatedPlayerStats(ctx context.Context, guid uint64, race, class, level uint8) {
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var baseHealth, baseMana int64
	_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT basehp, basemana FROM player_classlevelstats WHERE class = ? AND level = ?", class, level).Scan(&baseHealth, &baseMana)
	if baseHealth <= 0 {
		baseHealth = int64(20 + int(level)*15)
	}
	if baseMana <= 0 {
		baseMana = int64(80 + int(level)*20)
	}
	var str, agi, sta, inte, spi int64
	errStats := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT str, agi, sta, inte, spi FROM player_levelstats WHERE race = ? AND class = ? AND level = ?", race, class, level).Scan(&str, &agi, &sta, &inte, &spi)
	if errStats != nil {
		sta = int64(18 + int(level)*2)
		inte = int64(20 + int(level)*2)
	}
	totalHealth := baseHealth + sta*10
	totalMana := baseMana + inte*15
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET health = ?, power1 = ? WHERE guid = ?", totalHealth, totalMana, guid)
}

func (s *session) handleCharDelete(ctx context.Context, payload []byte) bool {
	b := protocol.NewReader(payload)
	guid, err := b.ReadU64()
	if err != nil {
		return false
	}
	if _, ok := s.legitimate[guid]; !ok {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), 72)
	}
	var accountID uint32
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID)
	if errors.Is(err, sql.ErrNoRows) || err != nil || accountID != s.accountID {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), 72)
	}
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_DEL_CHARACTER", guid); err != nil {
		return false
	}
	delete(s.legitimate, guid)
	return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), 71)
}

func (s *session) handlePlayerLogin(ctx context.Context, payload []byte) (success bool) {
	var guid uint64
	s.playerLoading = true
	defer func() {
		s.playerLoading = false
		if !success {
			s.debug("player login failed", "account", s.accountName, "guid", guid)
		}
	}()
	if s.playerLoaded {
		return false
	}
	b := protocol.NewReader(payload)
	guid, err := b.ReadU64()
	if err != nil {
		return false
	}
	if _, ok := s.legitimate[guid]; !ok {
		return false
	}
	s.playerGUID = guid
	s.loadAchievementState(ctx)
	state, err := s.loadPlayerState(ctx, guid)
	if err != nil {
		return false
	}
	firstLogin := state.AtLogin&uint32(atLoginFirst) != 0
	s.loadRandomBGStatus(ctx, guid)
	s.loadBattlegroundData(ctx, guid)
	s.loadInstanceTimeRestrictions(ctx)
	s.prepareLoginResurrection(ctx, &state)
	mounts, err := s.loadMountState(ctx, guid)
	if err != nil {
		return false
	}
	s.mounts = mounts
	s.loadBuybackState(ctx, guid)
	state.Buyback = s.buyback
	s.logoutHook = false
	s.questStatusSent = false
	s.logoutAt = time.Time{}
	s.attackTarget = 0
	s.autoRepeatSpell = 0
	s.autoRepeatTarget = 0
	s.inFlight = false
	s.gossip = nil
	s.gossipClosed = false
	s.breathTimer = -1
	s.fatigueTimer = -1
	s.player = &state
	s.restoreBattlegroundLoginQueue(state)
	s.visiblePlayersMu.Lock()
	s.visiblePlayers = nil
	s.visiblePlayersMu.Unlock()
	if state.PlayerFlags&playerFlagContestedPVP != 0 {
		s.contestedPVPEnd = time.Now().Add(30 * time.Second)
	} else {
		s.contestedPVPEnd = time.Time{}
	}
	s.playerLoaded = true
	difficulty := protocol.NewBuffer(12)
	difficulty.WriteU32(uint32(state.DungeonDifficulty))
	difficulty.WriteU32(1)
	difficulty.WriteU32(0)
	if err := s.write(uint16(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY), difficulty.Bytes(), true); err != nil {
		return false
	}

	// Stream core login verification and capabilities
	if err := s.write(uint16(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD), buildLoginVerifyWorld(state), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_ACCOUNT_DATA_TIMES), buildAccountDataTimesWithTimestamps(time.Now(), characterAccountDataMask, s.loadAccountDataTimes(ctx, guid, characterAccountDataMask)), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_FEATURE_SYSTEM_STATUS), buildFeatureSystemStatus(), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_MOTD), buildMotd(s.server.Config.Motd), true); err != nil {
		return false
	}
	s.sendGuildLoginInfo(ctx)
	if err := s.write(uint16(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES), buildLearnedDanceMoves(), true); err != nil {
		return false
	}
	if err := s.sendContactList(ctx, uint32(socialFlagFriend|socialFlagIgnored|socialFlagMuted)); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_BIND_POINT_UPDATE), buildBindPointUpdate(&state), true); err != nil {
		return false
	}
	if err := s.sendTalentsInfo(false); err != nil {
		return false
	}
	instanceDifficulty, dynamicDifficulty := s.loginInstanceDifficulty(ctx, state)
	if err := s.write(uint16(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY), buildInstanceDifficultyForMap(instanceDifficulty, dynamicDifficulty), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_INITIAL_SPELLS), buildInitialSpells(state), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS), s.buildUnlearnSpells(ctx, state), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_ACTION_BUTTONS), buildActionButtons(state.Actions), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_INITIALIZE_FACTIONS), buildInitialReputations(state), true); err != nil {
		return false
	}
	s.loadExploredZones(ctx)
	s.sendAllAchievementData()
	s.debug("world login stage", "stage", "equipment-set-list-start", "guid", guid)
	s.sendEquipmentSetList(ctx)
	s.debug("world login stage", "stage", "equipment-set-list-complete", "guid", guid)
	if err := s.write(uint16(protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED), buildLoginSetTimeSpeed(time.Now()), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_SET_FORCED_REACTIONS), buildForcedReactions(s.loadedAuras()), true); err != nil {
		return false
	}
	if err := s.sendResyncRunes(); err != nil {
		return false
	}
	// TrinityCore sends the first-login cinematic after the pre-map packet set and
	// before the player is added to the map.
	sendCinematic := false
	firstLoginCinematic := state.Cinematic == 0
	if state.Cinematic == 0 {
		cinematicID := s.getStartingCinematicID(state.Race, state.Class)
		if cinematicID > 0 {
			sendCinematic = true
		}
		state.Cinematic = 1
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET cinematic = 1 WHERE guid = ?", guid); err != nil {
			s.debug("cinematic state save failed", "account", s.accountName, "guid", guid, "error", err)
			return false
		}
	}
	if sendCinematic {
		cinematicID := s.getStartingCinematicID(state.Race, state.Class)
		if cinematicID > 0 {
			cinematicBuf := protocol.NewBuffer(4)
			cinematicBuf.WriteU32(cinematicID)
			if err := s.write(uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC), cinematicBuf.Bytes(), true); err != nil {
				return false
			}
		}
	}
	if firstLoginCinematic && s.server.Config.PlayerStartString != "" {
		s.sendSysMessage(s.server.Config.PlayerStartString)
	}
	s.lastFallZ = state.Z
	s.lastFallTime = 0
	updates, err := s.server.buildPlayerUpdate(state)
	if err != nil {
		return false
	}
	attachedTransport, err := s.server.buildAttachedTransportUpdate(ctx, state)
	if err != nil {
		return false
	}
	attachedTransportPassengers, err := s.server.buildAttachedTransportPassengerUpdates(ctx, state)
	if err != nil {
		return false
	}
	attachedTransportPlayers, attachedTransportPlayerGUIDs, err := s.server.buildAttachedTransportPlayerUpdates(state, s.playerGUID)
	if err != nil {
		return false
	}
	s.captureUpdatePackets = true
	s.capturedUpdatePackets = nil
	inventoryErr := s.sendInventoryItemsBeforeMap(ctx)
	capturedInventoryPackets := s.capturedUpdatePackets
	s.captureUpdatePackets = false
	s.capturedUpdatePackets = nil
	if inventoryErr != nil {
		s.debug("inventory load failed", "account", s.accountName, "guid", s.playerGUID, "error", inventoryErr)
		return false
	}
	initialUpdatePackets := make([]*protocol.Packet, 0, len(capturedInventoryPackets)+3)
	if attachedTransport != nil {
		initialUpdatePackets = append(initialUpdatePackets, attachedTransport)
	}
	initialUpdatePackets = append(initialUpdatePackets, capturedInventoryPackets...)
	initialUpdatePackets = append(initialUpdatePackets, updates)
	if attachedTransportPassengers != nil {
		initialUpdatePackets = append(initialUpdatePackets, attachedTransportPassengers)
	}
	if attachedTransportPlayers != nil {
		initialUpdatePackets = append(initialUpdatePackets, attachedTransportPlayers)
	}
	initialUpdate, err := protocol.MergeUpdatePackets(initialUpdatePackets...)
	if err != nil || initialUpdate == nil {
		return false
	}
	if err := s.write(initialUpdate.Opcode, initialUpdate.Payload.Bytes(), true); err != nil {
		return false
	}
	s.markVisiblePlayers(attachedTransportPlayerGUIDs)
	s.server.broadcastPlayerCreate(state, s)
	mapTransportUpdates, err := s.server.buildMapTransportUpdates(state, state.TransportGUID)
	if err != nil {
		return false
	}
	if mapTransportUpdates != nil {
		if err := s.write(mapTransportUpdates.Opcode, mapTransportUpdates.Payload.Bytes(), true); err != nil {
			return false
		}
	}
	sendNearbyObjects := func() bool {
		var nearbyPlayers, nearbyCreatures, nearbyGameObjects, nearbyCorpses *protocol.Packet
		var playerCount, creatureCount, goCount, corpseCount int
		var creatureErr, goErr, corpseErr error
		var wg sync.WaitGroup
		wg.Add(4)
		go func() {
			defer wg.Done()
			nearbyPlayers, playerCount = s.server.buildNearbyPlayerUpdates(s)
		}()
		go func() {
			defer wg.Done()
			nearbyCreatures, creatureCount, creatureErr = s.server.buildNearbyCreatureUpdates(ctx, state)
		}()
		go func() {
			defer wg.Done()
			nearbyGameObjects, goCount, goErr = s.server.buildNearbyGameObjectUpdates(ctx, state, false)
		}()
		go func() {
			defer wg.Done()
			nearbyCorpses, corpseCount, corpseErr = s.server.buildNearbyCorpseUpdates(ctx, state)
		}()
		wg.Wait()
		if creatureErr != nil {
			s.debug("nearby creature load failed", "account", s.accountName, "error", creatureErr)
			return false
		}
		if goErr != nil {
			s.debug("nearby gameobjects load failed", "account", s.accountName, "error", goErr)
			return false
		}
		if corpseErr != nil {
			s.debug("nearby corpses load failed", "account", s.accountName, "error", corpseErr)
			return false
		}
		packet, err := protocol.MergeUpdatePackets(nearbyPlayers, nearbyCreatures, nearbyGameObjects, nearbyCorpses)
		if err != nil {
			s.debug("nearby visibility merge failed", "account", s.accountName, "error", err)
			return false
		}
		if packet != nil {
			if err := s.write(packet.Opcode, packet.Payload.Bytes(), true); err != nil {
				return false
			}
			if nearbyCreatures != nil {
				s.debug("nearby creatures sent", "account", s.accountName, "count", creatureCount)
			}
			if nearbyPlayers != nil {
				s.debug("nearby players sent", "account", s.accountName, "count", playerCount)
			}
			if nearbyGameObjects != nil {
				s.debug("nearby gameobjects sent", "account", s.accountName, "count", goCount)
			}
			if nearbyCorpses != nil {
				s.debug("nearby corpses sent", "account", s.accountName, "count", corpseCount)
			}
		}
		return true
	}
	if !sendNearbyObjects() {
		return false
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventMapChange, s.luaPlayer())
	s.streamDynamicSpellObjects()
	zoneID, areaID := s.server.zoneAndAreaID(state.Map, state.X, state.Y, state.Z, state.Zone)
	state.Zone = zoneID
	s.player.Zone = state.Zone
	s.areaID = areaID
	auraChanged := s.updateAreaDependentAuras(ctx, zoneID, areaID)
	itemChanged := s.autoUnequipOffhandIfNeeded(ctx)
	if s.applyZoneState(ctx, &state, zoneID, areaID) || auraChanged || itemChanged {
		s.sendPlayerUpdate()
	}
	if state.Map == WGMapID && zoneID == WGZoneID {
		s.server.handleWGPlayerEnter(s)
	}
	s.server.ensureZoneWeather(ctx, zoneID, s)
	s.lastZoneUpdate = time.Now()
	s.updateLocalChannels(state.Zone)
	s.exploreZone(ctx, state.Zone)
	if err := s.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), buildInitWorldStates(state, areaID, s.server.Config.ArenaSeasonID, s.server.Config.ArenaSeasonInProgress), true); err != nil {
		return false
	}
	s.lastStreamX, s.lastStreamY, s.lastStreamZ = state.X, state.Y, state.Z
	if err := s.write(uint16(protocol.OpcodeSMSG_TIME_SYNC_REQ), buildTimeSyncRequest(0), true); err != nil {
		return false
	}
	s.timeSyncNextCounter = 1
	s.timeSyncDue = time.Now().Add(5 * time.Second)
	if err := s.sendLoginEffect(); err != nil {
		return false
	}
	if err := s.sendLoginMovementDirectStates(); err != nil {
		return false
	}
	if err := s.sendLoginFlightState(); err != nil {
		return false
	}
	if err := s.sendLoginFlightSpeed(); err != nil {
		return false
	}
	if err := s.sendLoginMovementStunAndCompoundStates(); err != nil {
		return false
	}
	s.sendLoadedAuras()
	if err := s.sendInventoryDurations(ctx); err != nil {
		s.debug("inventory duration load failed", "account", s.accountName, "guid", s.playerGUID, "error", err)
		return false
	}
	s.questStatusSent = true
	if !s.sendQuestgiverStatusMultiple(ctx) || !s.sendTaxiNodeStatusMultiple(ctx) {
		return false
	}
	if err := s.sendLoginRaidDifficulty(ctx, state); err != nil {
		return false
	}
	if s.sharingQuestID != 0 {
		if quest, questErr := s.loadQuestDetailData(ctx, s.sharingQuestID); questErr == nil {
			if err := s.write(uint16(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS), buildQuestGiverDetails(quest, s.playerGUID, 0), true); err != nil {
				return false
			}
		} else {
			s.sharingQuestID = 0
			s.sharingQuestSender = 0
		}
	}
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_CHAR_ONLINE", guid); err != nil {
		return false
	}
	s.debug("world login stage", "stage", "character-online-complete", "guid", guid)
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_ONLINE", s.accountID); err != nil {
		return false
	}
	s.debug("world login stage", "stage", "account-online-complete", "guid", guid)
	s.sendLoadedGroup()
	s.server.broadcastFriendStatus(s.playerGUID, friendsResultOnline, uint32(state.Zone), uint32(state.Level), uint32(state.Class))
	if state.PlayerFlags&playerFlagGhost != 0 {
		if !s.sendLoadedCorpse(ctx) {
			s.sendForcedMovement(uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK))
		}
	}
	s.continueTaxiFlight()
	if s.player.AtLogin&uint32(atLoginResetPetTalents) != 0 {
		s.resetPetTalentsAtLogin(ctx)
	}
	// Spawn active pet if one was active at logout (slot 0)
	if cdb := s.server.CharactersStore.DB; cdb != nil {
		var petID, entry, modelID, level, petType, reactState, curHealth, curMana int64
		var petName string
		if err := cdb.QueryRowContext(ctx,
			"SELECT id, entry, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1) FROM character_pet WHERE owner = ? AND slot = 0",
			s.playerGUID).Scan(&petID, &entry, &modelID, &level, &petName, &curHealth, &curMana, &petType, &reactState); err == nil && curHealth > 0 {
			petLevel := uint32(level)
			if petType == 0 && state.Level > 0 {
				petLevel = uint32(state.Level)
			}
			maxHP, _, maxMP, _ := s.getPetStats(ctx, uint32(entry), petLevel)
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
			s.spawnPet(ctx, uint32(petID), uint32(entry), petName, petLevel, uint32(modelID), uint32(curHealth), maxHP, uint32(curMana), maxMP, uint8(reactState))
			_ = s.sendTalentsInfo(true)
		}
	}
	if s.server.Config.GameType == 16 && s.security == 0 && s.player.ExtraFlags&playerExtraGMOn == 0 && s.player.PlayerFlags&playerFlagGM == 0 && s.player.PlayerFlags&playerFlagResting == 0 {
		s.player.PVPFlags |= 0x04
		s.sendPlayerUpdate()
	}
	if s.player.AtLogin&uint32(atLoginResetSpells) != 0 {
		if err := s.resetSpellsAtLogin(ctx); err == nil {
			s.player.AtLogin &^= uint32(atLoginResetSpells)
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ~2 WHERE guid = ?", s.playerGUID)
			}
			s.sendNotification("All spells have been reset.")
		}
	}
	if s.player.AtLogin&uint32(atLoginResetTalents) != 0 {
		if s.resetTalents(ctx, true) {
			_ = s.sendTalentsInfo(false)
			s.player.AtLogin &^= uint32(atLoginResetTalents)
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ~4 WHERE guid = ?", s.playerGUID)
			}
		}
	}
	if s.player.AtLogin&uint32(atLoginFirst) != 0 {
		s.player.AtLogin &^= uint32(atLoginFirst)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET at_login = at_login & ~32 WHERE guid = ?", s.playerGUID)
		}
		for _, spellID := range s.loadFirstLoginCastSpellIDs(ctx, s.player.Race, s.player.Class) {
			if s.server.Data == nil {
				continue
			}
			if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
				target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
				s.finishSpellCast(ctx, 0, spellID, spell, target)
			}
		}
		if s.server.Config.PlayerStartMapsExplored {
			for i := range s.player.ExploredZones {
				s.player.ExploredZones[i] = ^uint32(0)
			}
			s.persistExploredZones(ctx)
			s.sendPlayerUpdate()
		}
		if s.server.Config.PlayerStartAllReputation {
			s.applyStartAllReputation(ctx)
		}
	}
	if s.server.Config.AllFlightPaths {
		s.player.ExtraFlags |= playerExtraTaxiCheat
		s.persistExtraFlags()
	}
	s.expireOldMails(ctx)
	s.loadMailState(ctx)
	s.sendNewMailNotification(ctx)
	if s.player.StandState != 0 && s.player.UnitFlags&unitFlagStunned == 0 {
		s.player.StandState = 0
		s.sendPlayerUpdate()
	}
	if s.player.PlayerFlags&playerFlagGM != 0 || s.player.ExtraFlags&playerExtraGMOn != 0 {
		s.sendNotification("GM mode is ON")
	}
	s.playerLoading = false
	s.updateAchievementCriteria(criteriaTypeOnLogin, 0, 1)
	s.setAchievementCriteria(criteriaTypeKnownFactions, 0, uint32(len(state.Reputations)))
	if s.player.repopOnLogin {
		s.buildPlayerRepop(ctx)
		s.repopAtGraveyard(ctx)
		s.player.repopOnLogin = false
	}
	if s.player.ChosenTitle > 0 {
		s.updateAchievementCriteria(criteriaTypeOwnRank, s.player.ChosenTitle, 1)
	}
	s.debug("world login stage", "stage", "player-login-hooks-start", "guid", guid)
	if firstLogin {
		s.triggerPlayerEvent(ctx, scripting.PlayerEventFirstLogin, s.luaPlayer())
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventLogin, s.luaPlayer())
	s.debug("world login stage", "stage", "player-login-hooks-complete", "guid", guid)
	s.server.Features.OnPlayerLogin()
	if s.server.Config.SoloLFGAnnounce {
		message := protocol.BuildSystemChatMessage("This server is running |cff4CFF00Solo Dungeon Finder|r module.")
		if err := s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), message, true); err != nil {
			return false
		}
	}
	s.debug("player login complete", "account", s.accountName, "guid", s.playerGUID, "map", state.Map, "x", state.X, "y", state.Y, "z", state.Z)
	return true
}

func (s *session) loadRandomBGStatus(ctx context.Context, guid uint64) {
	s.randomBGWinner = false
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var found int64
	if s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT 1 FROM character_battleground_random WHERE guid = ? LIMIT 1", guid).Scan(&found) == nil {
		s.randomBGWinner = found != 0
	}
}

func (s *session) loadBattlegroundData(ctx context.Context, guid uint64) {
	s.bgData = battlegroundLoginData{}
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT instanceId, team, joinX, joinY, joinZ, joinO, joinMapId, taxiStart, taxiEnd, mountSpell
		FROM character_battleground_data WHERE guid = ?`, guid).Scan(&s.bgData.InstanceID, &s.bgData.Team, &s.bgData.JoinX, &s.bgData.JoinY, &s.bgData.JoinZ, &s.bgData.JoinO, &s.bgData.JoinMap, &s.bgData.TaxiStart, &s.bgData.TaxiEnd, &s.bgData.MountSpell)
}

func (s *session) loadInstanceTimeRestrictions(ctx context.Context) {
	s.instanceLockTimes = make(map[uint32]int64)
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT instanceId, releaseTime FROM account_instance_times WHERE accountId = ?", s.accountID)
	if err != nil {
		return
	}
	defer rows.Close()
	now := time.Now().Unix()
	for rows.Next() {
		var instanceID, releaseTime int64
		if rows.Scan(&instanceID, &releaseTime) != nil || instanceID <= 0 || releaseTime <= now {
			continue
		}
		s.instanceLockTimes[uint32(instanceID)] = releaseTime
	}
}

func (s *session) applyZoneState(ctx context.Context, state *playerState, zoneID, areaID uint32) bool {
	if s == nil || state == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	zone, found, err := s.server.Data.Area(zoneID)
	if err != nil || !found {
		return false
	}
	area := zone
	if areaID != 0 {
		if detail, areaFound, areaErr := s.server.Data.Area(areaID); areaErr == nil && areaFound {
			area = detail
		}
	}
	oldFlags, oldPlayerFlags := state.PVPFlags, state.PlayerFlags
	pvpRealm := s.server.Config.GameType == 1 || s.server.Config.GameType == 4 || s.server.Config.GameType == 6
	team := teamForRace(state.Race)
	hostile := s.hasPvPForcingQuest(ctx)
	switch zone.FactionGroupMask {
	case 2:
		hostile = hostile || (team != 0 && (pvpRealm || zone.Flags&wotlk.AreaFlagCapital != 0))
	case 4:
		hostile = hostile || (team != 1 && (pvpRealm || zone.Flags&wotlk.AreaFlagCapital != 0))
	case 0:
		inBattleground := false
		if entry, ok, mapErr := s.server.Data.Map(state.Map); mapErr == nil && ok {
			inBattleground = entry.IsBattleground() || entry.IsBattleArena()
		}
		hostile = hostile || pvpRealm || inBattleground || area.Flags&wotlk.AreaFlagWintergrasp != 0
	}
	sanctuary := area.Flags&wotlk.AreaFlagSanctuary != 0
	if sanctuary {
		state.PVPFlags |= 0x08
	} else {
		state.PVPFlags &^= 0x08
	}
	if area.Flags&wotlk.AreaFlagArena != 0 || s.server.Config.GameType == 6 {
		state.PVPFlags |= 0x04
	} else {
		state.PVPFlags &^= 0x04
	}
	if state.PlayerFlags&playerFlagInPVP != 0 || (hostile && state.PlayerFlags&playerFlagGM == 0) {
		state.PVPFlags |= 0x01
	} else if !hostile {
		state.PVPFlags &^= 0x01
	}
	s.pvpHostile = hostile
	if hostile || state.PlayerFlags&playerFlagInPVP != 0 {
		s.pvpEnd = time.Time{}
		state.PlayerFlags &^= playerFlagPVPTimer
	} else if state.PVPFlags&0x01 != 0 {
		if s.pvpEnd.IsZero() {
			s.pvpEnd = time.Now()
			state.PlayerFlags |= playerFlagPVPTimer
		}
	}
	if zone.Flags&wotlk.AreaFlagCapital != 0 && (!hostile || sanctuary) {
		state.PlayerFlags |= playerFlagResting
	}
	return state.PVPFlags != oldFlags || state.PlayerFlags != oldPlayerFlags
}

func (s *session) hasPvPForcingQuest(ctx context.Context) bool {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	var found int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT 1 FROM character_queststatus AS cqs JOIN quest_template AS qt ON qt.ID = cqs.quest WHERE cqs.guid = ? AND cqs.status IN (1, 3) AND (qt.Flags & 0x00002000) <> 0 LIMIT 1`, s.playerGUID).Scan(&found)
	return err == nil && found != 0
}

func (s *session) completeWorldPort(ctx context.Context) bool {
	if s == nil || s.server == nil || s.player == nil {
		return false
	}
	state := *s.player
	attachedTransport, err := s.server.buildAttachedTransportUpdate(ctx, state)
	if err != nil {
		return false
	}
	attachedPassengers, err := s.server.buildAttachedTransportPassengerUpdates(ctx, state)
	if err != nil {
		return false
	}
	attachedPlayers, attachedPlayerGUIDs, err := s.server.buildAttachedTransportPlayerUpdates(state, s.playerGUID)
	if err != nil {
		return false
	}
	playerUpdate, err := s.server.buildPlayerUpdate(state)
	if err != nil || playerUpdate == nil {
		return false
	}
	s.captureUpdatePackets = true
	s.capturedUpdatePackets = nil
	if err := s.sendInventoryItemsBeforeMap(ctx); err != nil {
		s.captureUpdatePackets = false
		s.capturedUpdatePackets = nil
		return false
	}
	inventoryPackets := s.capturedUpdatePackets
	s.captureUpdatePackets = false
	s.capturedUpdatePackets = nil
	initialPackets := make([]*protocol.Packet, 0, len(inventoryPackets)+3)
	if attachedTransport != nil {
		initialPackets = append(initialPackets, attachedTransport)
	}
	initialPackets = append(initialPackets, inventoryPackets...)
	initialPackets = append(initialPackets, playerUpdate)
	if attachedPassengers != nil {
		initialPackets = append(initialPackets, attachedPassengers)
	}
	if attachedPlayers != nil {
		initialPackets = append(initialPackets, attachedPlayers)
	}
	initialUpdate, err := protocol.MergeUpdatePackets(initialPackets...)
	if err != nil || initialUpdate == nil {
		return false
	}
	if err := s.write(initialUpdate.Opcode, initialUpdate.Payload.Bytes(), true); err != nil {
		return false
	}
	s.markVisiblePlayers(attachedPlayerGUIDs)
	s.server.broadcastPlayerCreate(state, s)
	if mapTransports, err := s.server.buildMapTransportUpdates(state, state.TransportGUID); err != nil {
		return false
	} else if mapTransports != nil {
		if err := s.write(mapTransports.Opcode, mapTransports.Payload.Bytes(), true); err != nil {
			return false
		}
	}
	nearbyPlayers, _ := s.server.buildNearbyPlayerUpdates(s)
	nearbyCreatures, _, creatureErr := s.server.buildNearbyCreatureUpdates(ctx, state)
	if creatureErr != nil {
		return false
	}
	nearbyGameObjects, _, gameObjectErr := s.server.buildNearbyGameObjectUpdates(ctx, state, false)
	if gameObjectErr != nil {
		return false
	}
	nearbyCorpses, _, corpseErr := s.server.buildNearbyCorpseUpdates(ctx, state)
	if corpseErr != nil {
		return false
	}
	if nearby, err := protocol.MergeUpdatePackets(nearbyPlayers, nearbyCreatures, nearbyGameObjects, nearbyCorpses); err != nil {
		return false
	} else if nearby != nil {
		if err := s.write(nearby.Opcode, nearby.Payload.Bytes(), true); err != nil {
			return false
		}
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventMapChange, s.luaPlayer())
	s.streamDynamicSpellObjects()
	s.updateZoneAndArea(ctx, true)
	if err := s.write(uint16(protocol.OpcodeSMSG_TIME_SYNC_REQ), buildTimeSyncRequest(0), true); err != nil {
		return false
	}
	s.timeSyncNextCounter = 1
	s.timeSyncDue = time.Now().Add(5 * time.Second)
	if err := s.sendLoginEffect(); err != nil {
		return false
	}
	if err := s.sendLoginMovementDirectStates(); err != nil {
		return false
	}
	if err := s.sendLoginFlightState(); err != nil {
		return false
	}
	if err := s.sendLoginFlightSpeed(); err != nil {
		return false
	}
	if err := s.sendLoginMovementStunAndCompoundStates(); err != nil {
		return false
	}
	s.sendLoadedAuras()
	if err := s.sendInventoryDurations(ctx); err != nil {
		return false
	}
	s.questStatusSent = true
	if !s.sendQuestgiverStatusMultiple(ctx) || !s.sendTaxiNodeStatusMultiple(ctx) {
		return false
	}
	if err := s.sendLoginRaidDifficulty(ctx, *s.player); err != nil {
		return false
	}
	s.lastStreamX, s.lastStreamY, s.lastStreamZ = s.player.X, s.player.Y, s.player.Z
	s.farTeleportPending = false
	return true
}

func (s *session) sendLoginMovementDirectStates() error {
	if s == nil || s.player == nil {
		return nil
	}
	const (
		auraWaterWalk   uint32 = 104
		auraFeatherFall uint32 = 105
		auraHover       uint32 = 106
	)
	auras := s.loadedAuras()
	for _, auraType := range []uint32{auraWaterWalk, auraFeatherFall, auraHover} {
		var opcode protocol.Opcode
		var broadcastOpcode protocol.Opcode
		var movementFlags uint32
		switch auraType {
		case auraFeatherFall:
			opcode = protocol.OpcodeSMSG_MOVE_FEATHER_FALL
			broadcastOpcode = protocol.OpcodeMSG_MOVE_FEATHER_FALL
			movementFlags = 0x20000000
		case auraWaterWalk:
			opcode = protocol.OpcodeSMSG_MOVE_WATER_WALK
			broadcastOpcode = protocol.OpcodeMSG_MOVE_WATER_WALK
			movementFlags = 0x10000000
		case auraHover:
			opcode = protocol.OpcodeSMSG_MOVE_SET_HOVER
			broadcastOpcode = protocol.OpcodeMSG_MOVE_HOVER
			movementFlags = 0x40000000
		}
		for _, aura := range auras {
			if aura == nil || aura.AuraType != auraType {
				continue
			}
			packet := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 4)
			packet.WritePackedGUID(s.playerGUID)
			packet.WriteU32(0)
			if err := s.write(uint16(opcode), packet.Bytes(), true); err != nil {
				return err
			}
			s.broadcastLoginMovementState(broadcastOpcode, movementFlags)
			break
		}
	}
	return nil
}

func (s *session) sendLoginMovementStunAndCompoundStates() error {
	if s == nil || s.player == nil {
		return nil
	}
	const (
		auraRoot  uint32 = 26
		auraStun  uint32 = 12
		auraWater uint32 = 104
		auraFall  uint32 = 105
		auraHover uint32 = 106
	)
	auras := s.loadedAuras()
	rooted, stunned := false, false
	for _, aura := range auras {
		if aura == nil {
			continue
		}
		rooted = rooted || aura.AuraType == auraRoot
		stunned = stunned || aura.AuraType == auraStun
	}
	if stunned {
		packet := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 4)
		packet.WritePackedGUID(s.playerGUID)
		packet.WriteU32(0)
		if err := s.write(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), packet.Bytes(), true); err != nil {
			return err
		}
	}
	state := protocol.NewBuffer(64)
	if rooted {
		state.WriteU8(uint8(2 + packedGUIDSize(s.playerGUID) + 4))
		state.WriteU16(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT))
		state.WritePackedGUID(s.playerGUID)
		state.WriteU32(0)
	}
	for _, auraType := range []uint32{auraFall, auraWater, auraHover} {
		var opcode protocol.Opcode
		switch auraType {
		case auraFall:
			opcode = protocol.OpcodeSMSG_MOVE_FEATHER_FALL
		case auraWater:
			opcode = protocol.OpcodeSMSG_MOVE_WATER_WALK
		case auraHover:
			opcode = protocol.OpcodeSMSG_MOVE_SET_HOVER
		}
		for _, aura := range auras {
			if aura == nil || aura.AuraType != auraType {
				continue
			}
			state.WriteU8(uint8(2 + packedGUIDSize(s.playerGUID) + 4))
			state.WriteU16(uint16(opcode))
			state.WritePackedGUID(s.playerGUID)
			state.WriteU32(0)
			break
		}
	}
	if state.Len() == 0 {
		return nil
	}
	packet := protocol.NewBuffer(state.Len() + 4)
	packet.WriteU32(uint32(state.Len()))
	packet.Write(state.Bytes())
	return s.write(uint16(protocol.OpcodeSMSG_MULTIPLE_MOVES), packet.Bytes(), true)
}

func (s *session) sendLoginFlightState() error {
	if s == nil || s.player == nil {
		return nil
	}
	auras := s.loadedAuras()
	for _, auraType := range []uint32{201, 207} {
		found := false
		for _, aura := range auras {
			if aura != nil && aura.AuraType == auraType {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		packet := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 4)
		packet.WritePackedGUID(s.playerGUID)
		packet.WriteU32(0)
		if err := s.write(uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY), packet.Bytes(), true); err != nil {
			return err
		}
		s.broadcastLoginMovementState(protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY, 0x01000000)
	}
	return nil
}

func (s *session) sendLoginFlightSpeed() error {
	if s == nil || s.player == nil {
		return nil
	}
	modifier := uint32(0)
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == 207 && aura.Amount > modifier {
			modifier = aura.Amount
		}
	}
	if modifier == 0 && s.mounts != nil {
		if preferred := s.mounts.PreferredFlightSpeed(true); preferred > 100 {
			modifier = uint32(preferred - 100)
		}
	}
	if modifier == 0 {
		return nil
	}
	speed := float32(7.0 * (1.0 + float64(modifier)/100.0))
	self := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 8)
	self.WritePackedGUID(s.playerGUID)
	self.WriteU32(0)
	self.WriteF32(speed)
	if err := s.write(uint16(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE), self.Bytes(), true); err != nil {
		return err
	}
	nearby := protocol.NewBuffer(96)
	nearby.WritePackedGUID(s.playerGUID)
	nearby.WriteU32(0x01000000)
	nearby.WriteU16(0)
	nearby.WriteU32(uint32(time.Now().UnixMilli()))
	nearby.WriteF32(s.player.X)
	nearby.WriteF32(s.player.Y)
	nearby.WriteF32(s.player.Z)
	nearby.WriteF32(s.player.Orientation)
	if s.player.TransportGUID != 0 {
		nearby.WritePackedGUID(s.player.TransportGUID)
		nearby.WriteF32(s.player.TransportX)
		nearby.WriteF32(s.player.TransportY)
		nearby.WriteF32(s.player.TransportZ)
		nearby.WriteF32(s.player.TransportO)
		nearby.WriteU32(0)
		nearby.WriteI8(s.player.TransportSeat)
	}
	nearby.WriteU32(0)
	for _, base := range []float32{2.5, 7.0, 4.5, 4.722222, 2.5, 7.0, 4.5, 3.141594, 3.14} {
		nearby.WriteF32(base)
	}
	nearby.WriteF32(speed)
	s.server.broadcastToNearby(uint16(protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED), nearby.Bytes(), s)
	return nil
}

func (s *session) broadcastLoginMovementState(opcode protocol.Opcode, movementFlags uint32) {
	if s == nil || s.server == nil || s.player == nil || opcode == 0 {
		return
	}
	packet := protocol.NewBuffer(80)
	packet.WritePackedGUID(s.playerGUID)
	if s.player.TransportGUID != 0 {
		movementFlags |= movementOnTransport
	}
	packet.WriteU32(movementFlags)
	packet.WriteU16(0)
	packet.WriteU32(uint32(time.Now().UnixMilli()))
	packet.WriteF32(s.player.X)
	packet.WriteF32(s.player.Y)
	packet.WriteF32(s.player.Z)
	packet.WriteF32(s.player.Orientation)
	if movementFlags&movementOnTransport != 0 {
		packet.WritePackedGUID(s.player.TransportGUID)
		packet.WriteF32(s.player.TransportX)
		packet.WriteF32(s.player.TransportY)
		packet.WriteF32(s.player.TransportZ)
		packet.WriteF32(s.player.TransportO)
		packet.WriteU32(0)
		packet.WriteI8(s.player.TransportSeat)
	}
	packet.WriteU32(0)
	for _, speed := range []float32{2.5, 7.0, 4.5, 4.722222, 2.5, 7.0, 4.5, 3.141594, 3.14} {
		packet.WriteF32(speed)
	}
	s.server.broadcastToNearby(uint16(opcode), packet.Bytes(), s)
}

func packedGUIDSize(guid uint64) int {
	size := 1
	for index := 0; index < 8; index++ {
		if byte(guid>>(8*index)) != 0 {
			size++
		}
	}
	return size
}

func (s *session) sendLoginEffect() error {
	if s == nil || s.player == nil {
		return nil
	}
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
	castFlags := spellCastFlagGo | spellCastFlagPending | protocol.SpellCastFlagPowerLeftSelf
	castTime := uint32(time.Now().UnixMilli())
	power := s.player.Powers[classPowerType(s.player.Class)]
	packet := protocol.BuildSpellGoWithPower(s.playerGUID, s.playerGUID, 0, 836, castFlags, castTime, []uint64{s.playerGUID}, nil, target, &power)
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), packet, true); err != nil {
		return err
	}
	if s.server != nil {
		nearbyFlags := castFlags &^ protocol.SpellCastFlagPowerLeftSelf
		nearby := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 0, 836, nearbyFlags, castTime, []uint64{s.playerGUID}, nil, target)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearby, s)
	}
	return nil
}

func buildLoginSetTimeSpeed(now time.Time) []byte {
	packet := protocol.NewBuffer(12)
	packet.WritePackedTime(now)
	packet.WriteF32(float32(0.01666667 * 30))
	packet.WriteU32(0)
	return packet.Bytes()
}

// handleNextCinematicCamera processes CMSG_NEXT_CINEMATIC_CAMERA (0x0FB).
// Reference: WorldSession::HandleNextCinematicCamera (MiscHandler.cpp:905).
func (s *session) handleNextCinematicCamera() bool {
	if s.player != nil {
		s.player.Cinematic = 1
		s.player.AtLogin &= ^uint32(atLoginFirst)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(), "UPDATE characters SET cinematic = 1, at_login = at_login & ~32 WHERE guid = ?", s.playerGUID)
		}
	}
	return true
}

func (s *session) handleOpeningCinematic() bool {
	if !s.playerLoaded || s.player == nil || s.player.Cinematic != 0 || s.player.XP != 0 {
		return true
	}
	cinematicID := s.getStartingCinematicID(s.player.Race, s.player.Class)
	if cinematicID == 0 {
		return true
	}
	s.player.Cinematic = 1
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(), "UPDATE characters SET cinematic = 1 WHERE guid = ?", s.playerGUID)
	}
	packet := protocol.NewBuffer(4)
	packet.WriteU32(cinematicID)
	return s.write(uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC), packet.Bytes(), true) == nil
}

func (s *session) handleCompleteCinematic(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	s.player.Cinematic = 1
	s.player.AtLogin &= ^uint32(atLoginFirst)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET cinematic = 1, at_login = at_login & ~32 WHERE guid = ?", s.playerGUID); err != nil {
			s.debug("cinematic completion save failed", "account", s.accountName, "guid", s.playerGUID, "error", err)
			return false
		}
	}
	s.debug("cinematic completed", "account", s.accountName, "guid", s.playerGUID)
	return true
}

func scanEnumCharacter(rows *sql.Rows) (enumCharacter, error) {
	var c enumCharacter
	var guid uint32
	var race, class, gender, skin, face, hairStyle, hairColor, facialStyle, level, zone, mapID int64
	var x, y, z float64
	var guildID, playerFlags, atLogin, petEntry, petDisplay, petLevel, banned sql.NullInt64
	var equipment sql.NullString
	err := rows.Scan(&guid, &c.Name, &race, &class, &gender, &skin, &face, &hairStyle, &hairColor, &facialStyle, &level, &zone, &mapID, &x, &y, &z, &guildID, &playerFlags, &atLogin, &petEntry, &petDisplay, &petLevel, &equipment, &banned)
	if err != nil {
		return c, err
	}
	c.GUID = uint64(guid)
	c.Race, c.Class, c.Gender, c.Skin, c.Face = uint8(race), uint8(class), uint8(gender), uint8(skin), uint8(face)
	c.HairStyle, c.HairColor, c.FacialStyle, c.Level = uint8(hairStyle), uint8(hairColor), uint8(facialStyle), uint8(level)
	c.Zone, c.Map = uint32(zone), uint32(mapID)
	c.X, c.Y, c.Z = float32(x), float32(y), float32(z)
	if guildID.Valid {
		c.GuildID = uint32(guildID.Int64)
	}
	if playerFlags.Valid {
		c.PlayerFlags = uint32(playerFlags.Int64)
	}
	if atLogin.Valid {
		c.AtLogin = uint16(atLogin.Int64)
	}
	if petEntry.Valid {
		c.PetEntry = uint32(petEntry.Int64)
	}
	if petDisplay.Valid {
		c.PetDisplay = uint32(petDisplay.Int64)
	}
	if petLevel.Valid {
		c.PetLevel = uint32(petLevel.Int64)
	}
	if equipment.Valid {
		c.Equipment = equipment.String
	}
	if banned.Valid {
		c.Banned = banned.Int64 != 0
	}
	return c, nil
}

func (s *session) buildEnumCharacter(ctx context.Context, packet *protocol.Buffer, c enumCharacter) {
	packet.WriteU64(c.GUID)
	packet.WriteCString(c.Name)
	packet.WriteU8(c.Race)
	packet.WriteU8(c.Class)
	packet.WriteU8(c.Gender)
	packet.WriteU8(c.Skin)
	packet.WriteU8(c.Face)
	packet.WriteU8(c.HairStyle)
	packet.WriteU8(c.HairColor)
	packet.WriteU8(c.FacialStyle)
	packet.WriteU8(c.Level)
	packet.WriteU32(c.Zone)
	packet.WriteU32(c.Map)
	packet.WriteF32(c.X)
	packet.WriteF32(c.Y)
	packet.WriteF32(c.Z)
	packet.WriteU32(c.GuildID)
	flags := uint32(0)
	if c.PlayerFlags&characterFlagHideHelm != 0 {
		flags |= characterFlagHideHelm
	}
	if c.PlayerFlags&characterFlagHideCloak != 0 {
		flags |= characterFlagHideCloak
	}
	if c.PlayerFlags&playerFlagGhost != 0 {
		flags |= characterFlagGhost
	}
	if c.AtLogin&uint16(atLoginRename) != 0 {
		flags |= characterFlagRename
	}
	if c.Banned {
		flags |= characterFlagLockedByBilling
	}
	flags |= characterFlagDeclined
	packet.WriteU32(flags)
	customize := characterCustomizeNone
	if c.AtLogin&uint16(atLoginCustomize) != 0 {
		customize = characterCustomizeCustomize
	} else if c.AtLogin&uint16(atLoginChangeFaction) != 0 {
		customize = characterCustomizeFaction
	} else if c.AtLogin&uint16(atLoginChangeRace) != 0 {
		customize = characterCustomizeRace
	}
	packet.WriteU32(customize)
	if c.AtLogin&uint16(atLoginFirst) != 0 {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	petDisplay, petLevel, petFamily := uint32(0), uint32(0), uint32(0)
	if c.PetEntry != 0 && c.PlayerFlags&playerFlagGhost == 0 && (c.Class == 3 || c.Class == 6 || c.Class == 9) {
		petDisplay, petLevel = c.PetDisplay, c.PetLevel
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var family int64
			if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(family, 0) FROM creature_template WHERE entry = ?", c.PetEntry).Scan(&family); err == nil && family > 0 {
				petFamily = uint32(family)
			}
		}
	}
	packet.WriteU32(petDisplay)
	packet.WriteU32(petLevel)
	packet.WriteU32(petFamily)
	equipment := strings.Fields(c.Equipment)
	for slot := 0; slot < inventorySlotBagEnd; slot++ {
		itemIndex := slot * 2
		if itemIndex >= len(equipment) {
			writeEmptyEnumEquipment(packet)
			continue
		}
		itemID, err := strconv.ParseUint(equipment[itemIndex], 10, 32)
		if err != nil || itemID == 0 || s.server == nil || s.server.WorldStore == nil {
			writeEmptyEnumEquipment(packet)
			continue
		}
		var displayID, inventoryType int64
		err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT displayid, InventoryType FROM item_template WHERE entry = ? LIMIT 1", itemID).Scan(&displayID, &inventoryType)
		if err != nil {
			writeEmptyEnumEquipment(packet)
			continue
		}
		enchantVisual := uint32(0)
		if itemIndex+1 < len(equipment) && s.server.Data != nil {
			if packed, parseErr := strconv.ParseUint(equipment[itemIndex+1], 10, 32); parseErr == nil {
				for _, enchantID := range []uint32{uint32(packed) & 0xFFFF, uint32(packed) >> 16} {
					if enchantID == 0 {
						continue
					}
					if enchant, found, enchantErr := s.server.Data.SpellItemEnchantment(enchantID); enchantErr == nil && found {
						enchantVisual = enchant.ItemVisual
						break
					}
				}
			}
		}
		packet.WriteU32(uint32(displayID))
		packet.WriteU8(uint8(inventoryType))
		packet.WriteU32(enchantVisual)
	}
}

func writeEmptyEnumEquipment(packet *protocol.Buffer) {
	packet.WriteU32(0)
	packet.WriteU8(0)
	packet.WriteU32(0)
}

func sendCharacterResult(s *session, opcode uint16, result uint8) bool {
	return s.write(opcode, []byte{result}, true) == nil
}

func validCharacterName(name string) bool {
	runes := []rune(name)
	if len(runes) < 2 || len(runes) > 12 {
		return false
	}
	for _, r := range runes {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func playableRace(race uint8) bool {
	switch race {
	case 1, 2, 3, 4, 5, 6, 7, 8, 10, 11:
		return true
	default:
		return false
	}
}

func (s *Server) raceDefinition(id uint8) (bool, uint32) {
	if s.Data != nil {
		if race, found, err := s.Data.Race(uint32(id)); err == nil && found {
			return wotlk.IsPlayableRace(race), race.RequiredExpansion
		}
	}
	return playableRace(id), raceExpansion(id)
}

func (s *Server) classDefinition(id uint8) (bool, uint32) {
	if s.Data != nil {
		if class, found, err := s.Data.Class(uint32(id)); err == nil && found {
			return class.ID != 0, class.RequiredExpansion
		}
	}
	return playableClass(id), classExpansion(id)
}

func playableClass(class uint8) bool {
	switch class {
	case 1, 2, 3, 4, 5, 6, 7, 8, 9, 11:
		return true
	default:
		return false
	}
}

func raceTeam(race uint8) uint8 {
	switch race {
	case 1, 3, 4, 7, 11:
		return 1
	case 2, 5, 6, 8, 10:
		return 2
	default:
		return 0
	}
}

func raceExpansion(race uint8) uint32 {
	if race == 10 || race == 11 {
		return 1
	}
	return 0
}

func classExpansion(class uint8) uint32 {
	if class == 6 {
		return 2
	}
	return 0
}

func (s *session) loadMountState(ctx context.Context, guid uint64) (*MountState, error) {
	var extraFlags int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT extra_flags FROM characters WHERE guid = ? AND account = ?", guid, s.accountID).Scan(&extraFlags); err != nil {
		return nil, err
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell FROM character_spell WHERE guid = ? AND active <> 0 AND disabled = 0 ORDER BY spell", guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spells := make([]LearnedMountSpell, 0)
	for rows.Next() {
		var spellID uint32
		if err := rows.Scan(&spellID); err != nil {
			return nil, err
		}
		if s.server.Data == nil {
			continue
		}
		spell, found, err := s.server.Data.Spell(spellID)
		if err != nil || !found {
			continue
		}
		for _, effect := range spell.Effects {
			if effect.Aura == wotlk.MountedFlightSpeedAura {
				spells = append(spells, LearnedMountSpell{ID: spellID, MountedFlightSpeed: int(effect.BasePoints + 1)})
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewMountState(uint32(extraFlags), spells), nil
}

func (s *session) LearnMountSpell(ctx context.Context, guid uint64, spellID uint32) error {
	if s.mounts == nil || s.server.Data == nil {
		return fmt.Errorf("mount data is unavailable")
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("spell %d not found", spellID)
	}
	for _, effect := range spell.Effects {
		if effect.Aura == wotlk.MountedFlightSpeedAura {
			s.mounts.LearnSpell(LearnedMountSpell{ID: spellID, MountedFlightSpeed: int(effect.BasePoints + 1)})
			_, err = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET extra_flags = ? WHERE guid = ? AND account = ?", s.mounts.ExtraFlags(), guid, s.accountID)
			return err
		}
	}
	return nil
}

func (s *session) getStartingCinematicID(race, class uint8) uint32 {
	if s != nil && s.server != nil && s.server.Data != nil {
		if cls, ok, _ := s.server.Data.Class(uint32(class)); ok && cls.CinematicSequence > 0 {
			return cls.CinematicSequence
		}
		if rc, ok, _ := s.server.Data.Race(uint32(race)); ok && rc.CinematicSequence > 0 {
			return rc.CinematicSequence
		}
	}
	if class == 6 { // Death Knight
		return 165
	}
	switch race {
	case 1: // Human
		return 81
	case 2: // Orc
		return 21
	case 3: // Dwarf
		return 41
	case 4: // Night Elf
		return 61
	case 5: // Undead
		return 2
	case 6: // Tauren
		return 141
	case 7: // Gnome
		return 101
	case 8: // Troll
		return 121
	case 10: // Blood Elf
		return 162
	case 11: // Draenei
		return 163
	default:
		return 0
	}
}

func (s *session) createStarterOutfit(ctx context.Context, guid uint64, race, class, gender uint8) {
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return
	}

	var itemIDs []uint32
	seen := make(map[uint32]bool)

	// 1. Get starter items from CharStartOutfit DBC
	if s.server.Data != nil {
		if outfit, err := s.server.Data.CharStartOutfit(race, class, gender); err == nil && len(outfit) > 0 {
			for _, id := range outfit {
				if id > 0 && !seen[id] {
					seen[id] = true
					itemIDs = append(itemIDs, id)
				}
			}
		}
	}

	// 2. Also check custom items from playercreateinfo_item
	rows, err := wdb.QueryContext(ctx, "SELECT itemid, amount FROM playercreateinfo_item WHERE (race = ? OR race = 0) AND (class = ? OR class = 0)", race, class)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var customID, amount int64
			if err := rows.Scan(&customID, &amount); err == nil && customID > 0 {
				id := uint32(customID)
				if !seen[id] {
					seen[id] = true
					itemIDs = append(itemIDs, id)
				}
			}
		}
	}

	if len(itemIDs) == 0 {
		return
	}

	occupiedSlots := make(map[uint8]bool)
	equippedSlots := make([]uint32, equipSlotEnd)
	firstBackpackSlot := uint8(23)

	for _, itemEntry := range itemIDs {
		var invType, buyCount, itemClass, itemSubclass int64
		err := wdb.QueryRowContext(ctx, "SELECT InventoryType, BuyCount, class, subclass FROM item_template WHERE entry = ?", itemEntry).Scan(&invType, &buyCount, &itemClass, &itemSubclass)
		if err != nil {
			continue
		}

		slot := inventoryTypeToSlot(uint8(invType))
		targetBag := uint8(0)
		targetSlot := uint8(0)

		if slot < equipSlotEnd && !occupiedSlots[slot] {
			targetSlot = slot
			occupiedSlots[slot] = true
			equippedSlots[slot] = itemEntry
		} else if slot == equipSlotFinger1 && !occupiedSlots[equipSlotFinger2] {
			targetSlot = equipSlotFinger2
			occupiedSlots[equipSlotFinger2] = true
			equippedSlots[equipSlotFinger2] = itemEntry
		} else if slot == equipSlotTrinket1 && !occupiedSlots[equipSlotTrinket2] {
			targetSlot = equipSlotTrinket2
			occupiedSlots[equipSlotTrinket2] = true
			equippedSlots[equipSlotTrinket2] = itemEntry
		} else if slot == equipSlotMainhand && !occupiedSlots[equipSlotOffhand] && (invType == 13 || invType == 22 || invType == 23) {
			targetSlot = equipSlotOffhand
			occupiedSlots[equipSlotOffhand] = true
			equippedSlots[equipSlotOffhand] = itemEntry
		} else {
			// Find first empty backpack slot in 23..38
			found := false
			for bpSlot := firstBackpackSlot; bpSlot <= 38; bpSlot++ {
				if !occupiedSlots[bpSlot] {
					targetSlot = bpSlot
					occupiedSlots[bpSlot] = true
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		count := int64(1)
		if buyCount > 1 {
			count = buyCount
		}

		var nextItemGUID uint64
		if err := cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) + 1 FROM item_instance").Scan(&nextItemGUID); err != nil || nextItemGUID == 0 {
			nextItemGUID = uint64(time.Now().UnixNano())
		}

		insItemQuery := "INSERT INTO item_instance (guid, itemEntry, owner_guid, creatorGuid, count, duration, charges, flags, enchantments, randomPropertyId, durability, playedTime, text) VALUES (?, ?, ?, 0, ?, 0, 0, 0, '', 0, 100, 0, '')"
		if _, err := cdb.ExecContext(ctx, insItemQuery, nextItemGUID, itemEntry, guid, count); err != nil {
			continue
		}

		insInvQuery := "INSERT INTO character_inventory (guid, bag, slot, item) VALUES (?, ?, ?, ?)"
		_, _ = cdb.ExecContext(ctx, insInvQuery, guid, targetBag, targetSlot, nextItemGUID)
	}

	// Update equipmentCache in characters table
	parts := make([]string, equipSlotEnd*2)
	for i := 0; i < int(equipSlotEnd); i++ {
		parts[i*2] = strconv.FormatUint(uint64(equippedSlots[i]), 10)
		parts[i*2+1] = "0"
	}
	cacheStr := strings.Join(parts, " ")
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET equipmentCache = ? WHERE guid = ?", cacheStr, guid)
}

func (s *session) createStarterSpells(ctx context.Context, guid uint64, race, class uint8) {
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return
	}

	// 1. Default racial & language spells
	spells := defaultRacialSpells(race)
	seen := make(map[uint32]bool)
	for _, sp := range spells {
		seen[sp.ID] = true
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", guid, sp.ID)
	}

	// 2. Racial traits
	racialTraits := map[uint8][]uint32{
		1:  {20599, 20598, 20597, 20595, 20864, 59752}, // Human
		2:  {20572, 20573, 20575, 20574},               // Orc
		3:  {20594, 20592, 20596, 2481, 59224},         // Dwarf
		4:  {58984, 20582, 20585, 20583},               // Night Elf
		5:  {7744, 20577, 5227, 20579},                 // Undead
		6:  {20549, 20550, 20552, 20551},               // Tauren
		7:  {20589, 20591, 20593, 20592},               // Gnome
		8:  {26297, 20555, 20557, 20558, 26290, 58943}, // Troll
		10: {28730, 25046, 50613, 28877, 822},          // Blood Elf
		11: {28880, 28875, 28877, 28878, 6562},         // Draenei
	}
	if traits, ok := racialTraits[race]; ok {
		for _, trait := range traits {
			if !seen[trait] {
				seen[trait] = true
				_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", guid, trait)
			}
		}
	}

	// Custom spells from playercreateinfo_spell_custom (only if PlayerStart.AllSpells enabled)
	raceMask, classMask := playerCreateMask(race), playerCreateMask(class)
	if s.server != nil && s.server.Config.PlayerStartAllSpells {
		rows, err := wdb.QueryContext(ctx, "SELECT Spell FROM playercreateinfo_spell_custom WHERE (racemask = 0 OR (racemask & ?) <> 0) AND (classmask = 0 OR (classmask & ?) <> 0)", raceMask, classMask)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var spellID int64
				if err := rows.Scan(&spellID); err == nil && spellID > 0 {
					id := uint32(spellID)
					if !seen[id] {
						seen[id] = true
						_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", guid, id)
					}
				}
			}
		}
	}
}

func (s *session) createStarterSkills(ctx context.Context, guid uint64, race, class uint8) {
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return
	}

	var racialLangSkill uint32
	switch race {
	case 1:
		racialLangSkill = 98 // Common
	case 2:
		racialLangSkill = 109 // Orcish
	case 3:
		racialLangSkill = 111 // Dwarven
	case 4:
		racialLangSkill = 113 // Darnassian
	case 5:
		racialLangSkill = 673 // Gutterspeak
	case 6:
		racialLangSkill = 115 // Taurahe
	case 7:
		racialLangSkill = 313 // Gnomish
	case 8:
		racialLangSkill = 315 // Troll
	case 10:
		racialLangSkill = 137 // Thalassian
	case 11:
		racialLangSkill = 759 // Draenei
	}
	if racialLangSkill != 0 {
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, 300, 300)", guid, racialLangSkill)
	}
	if race == 3 || race == 4 || race == 7 || race == 11 {
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, 98, 300, 300)", guid)
	} else if race == 5 || race == 6 || race == 8 || race == 10 {
		_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, 109, 300, 300)", guid)
	}

	raceMask, classMask := playerCreateMask(race), playerCreateMask(class)
	rows, err := wdb.QueryContext(ctx, "SELECT skill, rank FROM playercreateinfo_skills WHERE (raceMask = 0 OR (raceMask & ?) <> 0) AND (classMask = 0 OR (classMask & ?) <> 0)", raceMask, classMask)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var skillID, rank int64
			if err := rows.Scan(&skillID, &rank); err == nil && skillID > 0 {
				if !s.skillAllowed(race, class, uint16(skillID)) {
					continue
				}
				val := 1
				max := 1
				rangeType := s.skillRangeType(race, class, uint16(skillID))
				if rangeType == wotlk.SkillRangeLanguage {
					val = 300
					max = 300
				} else if rangeType == wotlk.SkillRangeMono {
					val = 1
					max = 1
				} else if rangeType == wotlk.SkillRangeLevel || isLevelScaledSkill(uint16(skillID)) {
					val = 1
					max = 5 // level 1 * 5
				}
				if rank > 0 {
					val = int(rank)
				}
				_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", guid, skillID, val, max)
			}
		}
	}
}

func (s *session) createStarterActions(ctx context.Context, guid uint64, race, class uint8) {
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return
	}
	rows, err := wdb.QueryContext(ctx, "SELECT button, action, type FROM playercreateinfo_action WHERE race = ? AND class = ?", race, class)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var button, action, kind int64
			if err := rows.Scan(&button, &action, &kind); err == nil {
				_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_action (guid, spec, button, action, type) VALUES (?, 0, ?, ?, ?)", guid, button, action, kind)
			}
		}
	}
}

// handleCharRename processes CMSG_CHAR_RENAME (0x2C7).
// Reference: WorldSession::HandleCharRenameOpcode (CharacterHandler.cpp:1111).
func (s *session) handleCharRename(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil {
		return false
	}
	newName, err := r.ReadCString()
	if err != nil {
		return false
	}

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET name = ? WHERE guid = ?", newName, guid)
	}

	buf := protocol.NewBuffer(1 + 8 + len(newName) + 1)
	buf.WriteU8(0) // RESPONSE_SUCCESS
	buf.WriteU64(guid)
	buf.WriteCString(newName)
	_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_RENAME), buf.Bytes(), true)
	s.debug("character renamed", "guid", guid, "name", newName)
	return true
}

// handleCharCustomize processes CMSG_CHAR_CUSTOMIZE (0x473).
// Reference: WorldSession::HandleCharCustomize (CharacterHandler.cpp:1230).
func (s *session) handleCharCustomize(ctx context.Context, payload []byte) bool {
	if len(payload) < 15 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	newName, _ := r.ReadCString()
	gender, _ := r.ReadU8()
	skin, _ := r.ReadU8()
	face, _ := r.ReadU8()
	hairStyle, _ := r.ReadU8()
	hairColor, _ := r.ReadU8()
	facialHair, _ := r.ReadU8()

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET name = ?, gender = ?, skin = ?, face = ?, hairStyle = ?, hairColor = ?, facialStyle = ? WHERE guid = ?",
			newName, gender, skin, face, hairStyle, hairColor, facialHair, guid)
	}

	buf := protocol.NewBuffer(16 + len(newName))
	buf.WriteU8(0) // RESPONSE_SUCCESS
	buf.WriteU64(guid)
	buf.WriteCString(newName)
	buf.WriteU8(gender)
	buf.WriteU8(skin)
	buf.WriteU8(face)
	buf.WriteU8(hairStyle)
	buf.WriteU8(hairColor)
	buf.WriteU8(facialHair)
	_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_CUSTOMIZE), buf.Bytes(), true)
	s.debug("character customized", "guid", guid, "name", newName)
	return true
}

func teamForRace(race uint8) uint32 {
	switch race {
	case 1, 3, 4, 7, 11: // Alliance: Human, Dwarf, Night Elf, Gnome, Draenei
		return 0
	case 2, 5, 6, 8, 10: // Horde: Orc, Undead, Tauren, Troll, Blood Elf
		return 1
	default:
		return 0
	}
}

// handleCharRaceChange processes CMSG_CHAR_RACE_CHANGE (0x4F8).
// Reference: WorldSession::HandleCharFactionOrRaceChange (CharacterHandler.cpp:1616).
func (s *session) handleCharRaceChange(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	newName, _ := r.ReadCString()
	gender, _ := r.ReadU8()
	skin, _ := r.ReadU8()
	face, _ := r.ReadU8()
	hairStyle, _ := r.ReadU8()
	hairColor, _ := r.ReadU8()
	facialHair, _ := r.ReadU8()
	race, _ := r.ReadU8()

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		var oldRace uint8
		_ = cdb.QueryRowContext(ctx, "SELECT race FROM characters WHERE guid = ?", guid).Scan(&oldRace)
		// Race change must stay on the same faction team (CharacterHandler.cpp:1687)
		if oldRace != 0 && teamForRace(oldRace) != teamForRace(race) {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(0x38) // CHAR_CREATE_CHARACTER_RACE_ONLY
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET name = ?, gender = ?, skin = ?, face = ?, hairStyle = ?, hairColor = ?, facialStyle = ?, race = ? WHERE guid = ?",
			newName, gender, skin, face, hairStyle, hairColor, facialHair, race, guid)
	}

	buf := protocol.NewBuffer(17 + len(newName))
	buf.WriteU8(0) // RESPONSE_SUCCESS
	buf.WriteU64(guid)
	buf.WriteCString(newName)
	buf.WriteU8(gender)
	buf.WriteU8(skin)
	buf.WriteU8(face)
	buf.WriteU8(hairStyle)
	buf.WriteU8(hairColor)
	buf.WriteU8(facialHair)
	buf.WriteU8(race)
	_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
	return true
}

// handleCharFactionChange processes CMSG_CHAR_FACTION_CHANGE (0x4D9).
// Reference: WorldSession::HandleCharFactionOrRaceChange (CharacterHandler.cpp:1616).
func (s *session) handleCharFactionChange(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	newName, _ := r.ReadCString()
	gender, _ := r.ReadU8()
	skin, _ := r.ReadU8()
	face, _ := r.ReadU8()
	hairStyle, _ := r.ReadU8()
	hairColor, _ := r.ReadU8()
	facialHair, _ := r.ReadU8()
	race, _ := r.ReadU8()

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		var oldRace uint8
		_ = cdb.QueryRowContext(ctx, "SELECT race FROM characters WHERE guid = ?", guid).Scan(&oldRace)
		newTeam := teamForRace(race)
		// Faction change must swap to the opposite faction team (CharacterHandler.cpp:1687)
		if oldRace != 0 && teamForRace(oldRace) == newTeam {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(0x37) // CHAR_CREATE_CHARACTER_SWAP_FACTION
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}

		// Resurrect character if dead
		_, _ = cdb.ExecContext(ctx, "UPDATE characters SET health = 1 WHERE guid = ? AND health = 0", guid)

		// Set capital city homebind and world coordinates (CharacterHandler.cpp:1907-1925)
		var spawnMap, spawnZone int64
		var spawnX, spawnY, spawnZ, spawnO float64
		if newTeam == 0 { // Alliance -> Stormwind City
			spawnMap, spawnZone = 0, 1519
			spawnX, spawnY, spawnZ, spawnO = -8867.68, 673.373, 97.9034, 0.0
		} else { // Horde -> Orgrimmar
			spawnMap, spawnZone = 1, 1637
			spawnX, spawnY, spawnZ, spawnO = 1633.33, -4439.11, 15.7588, 0.0
		}

		_, _ = cdb.ExecContext(ctx, "UPDATE character_homebind SET mapId = ?, zoneId = ?, posX = ?, posY = ?, posZ = ? WHERE guid = ?",
			spawnMap, spawnZone, spawnX, spawnY, spawnZ, guid)
		_, err := cdb.ExecContext(ctx, "UPDATE characters SET name = ?, gender = ?, skin = ?, face = ?, hairStyle = ?, hairColor = ?, facialStyle = ?, race = ?, map = ?, zone = ?, position_x = ?, position_y = ?, position_z = ?, orientation = ?, at_login = at_login & ~64 WHERE guid = ?",
			newName, gender, skin, face, hairStyle, hairColor, facialHair, race, spawnMap, spawnZone, spawnX, spawnY, spawnZ, spawnO, guid)
		if err != nil {
			_, _ = cdb.ExecContext(ctx, "UPDATE characters SET name = ?, gender = ?, skin = ?, face = ?, hairStyle = ?, hairColor = ?, facialStyle = ?, race = ? WHERE guid = ?",
				newName, gender, skin, face, hairStyle, hairColor, facialHair, race, guid)
		}

		// Delete friends list and social interactions
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_social WHERE guid = ? OR friend = ?", guid, guid)

		// Delete all active quests in progress (CharacterHandler.cpp:1959)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_queststatus WHERE guid = ?", guid)

		// Dual-DB conversion tables in WorldStore.DB
		if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			wdb := s.server.WorldStore.DB

			type idPair struct {
				oldID, newID uint32
			}

			// 1. Spell conversion (player_factionchange_spells)
			var spellPairs []idPair
			sRows, err := wdb.QueryContext(ctx, "SELECT alliance_id, horde_id FROM player_factionchange_spells")
			if err == nil {
				for sRows.Next() {
					var aID, hID uint32
					if err := sRows.Scan(&aID, &hID); err == nil {
						if newTeam == 0 {
							spellPairs = append(spellPairs, idPair{oldID: hID, newID: aID})
						} else {
							spellPairs = append(spellPairs, idPair{oldID: aID, newID: hID})
						}
					}
				}
				sRows.Close()
				for _, pair := range spellPairs {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE spell = ? AND guid = ?", pair.newID, guid)
					_, _ = cdb.ExecContext(ctx, "UPDATE character_spell SET spell = ? WHERE spell = ? AND guid = ?", pair.newID, pair.oldID, guid)
				}
			}

			// 2. Item conversion (player_factionchange_items)
			var itemPairs []idPair
			iRows, err := wdb.QueryContext(ctx, "SELECT alliance_id, horde_id FROM player_factionchange_items")
			if err == nil {
				for iRows.Next() {
					var aID, hID uint32
					if err := iRows.Scan(&aID, &hID); err == nil {
						if newTeam == 0 {
							itemPairs = append(itemPairs, idPair{oldID: hID, newID: aID})
						} else {
							itemPairs = append(itemPairs, idPair{oldID: aID, newID: hID})
						}
					}
				}
				iRows.Close()
				for _, pair := range itemPairs {
					_, _ = cdb.ExecContext(ctx, "UPDATE item_instance SET itemEntry = ? WHERE itemEntry = ? AND guid IN (SELECT item FROM character_inventory WHERE guid = ?)", pair.newID, pair.oldID, guid)
				}
			}

			// 3. Quest conversion (rewarded quests, player_factionchange_quests)
			var questPairs []idPair
			qRows, err := wdb.QueryContext(ctx, "SELECT alliance_id, horde_id FROM player_factionchange_quests")
			if err == nil {
				for qRows.Next() {
					var aID, hID uint32
					if err := qRows.Scan(&aID, &hID); err == nil {
						if newTeam == 0 {
							questPairs = append(questPairs, idPair{oldID: hID, newID: aID})
						} else {
							questPairs = append(questPairs, idPair{oldID: aID, newID: hID})
						}
					}
				}
				qRows.Close()
				for _, pair := range questPairs {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM character_queststatus_rewarded WHERE quest = ? AND guid = ?", pair.newID, guid)
					_, _ = cdb.ExecContext(ctx, "UPDATE character_queststatus_rewarded SET quest = ? WHERE quest = ? AND guid = ?", pair.newID, pair.oldID, guid)
				}
			}

			// 4. Achievement conversion (player_factionchange_achievement)
			var achPairs []idPair
			aRows, err := wdb.QueryContext(ctx, "SELECT alliance_id, horde_id FROM player_factionchange_achievement")
			if err == nil {
				for aRows.Next() {
					var aID, hID uint32
					if err := aRows.Scan(&aID, &hID); err == nil {
						if newTeam == 0 {
							achPairs = append(achPairs, idPair{oldID: hID, newID: aID})
						} else {
							achPairs = append(achPairs, idPair{oldID: aID, newID: hID})
						}
					}
				}
				aRows.Close()
				for _, pair := range achPairs {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM character_achievement WHERE achievement = ? AND guid = ?", pair.newID, guid)
					_, _ = cdb.ExecContext(ctx, "UPDATE character_achievement SET achievement = ? WHERE achievement = ? AND guid = ?", pair.newID, pair.oldID, guid)
				}
			}

			// 5. Reputation conversion (player_factionchange_reputations)
			var repPairs []idPair
			rRows, err := wdb.QueryContext(ctx, "SELECT alliance_id, horde_id FROM player_factionchange_reputations")
			if err == nil {
				for rRows.Next() {
					var aID, hID uint32
					if err := rRows.Scan(&aID, &hID); err == nil {
						if newTeam == 0 {
							repPairs = append(repPairs, idPair{oldID: hID, newID: aID})
						} else {
							repPairs = append(repPairs, idPair{oldID: aID, newID: hID})
						}
					}
				}
				rRows.Close()
			}
			// If table is empty, use standard racial defaults
			if len(repPairs) == 0 {
				defaults := [][2]uint32{
					{72, 76},   // Stormwind <-> Orgrimmar
					{47, 81},   // Ironforge <-> Thunder Bluff
					{69, 530},  // Darnassus <-> Darkspear Trolls
					{54, 68},   // Gnomeregan <-> Undercity
					{930, 911}, // Exodar <-> Silvermoon City
				}
				for _, d := range defaults {
					if newTeam == 0 {
						repPairs = append(repPairs, idPair{oldID: d[1], newID: d[0]})
					} else {
						repPairs = append(repPairs, idPair{oldID: d[0], newID: d[1]})
					}
				}
			}
			for _, pair := range repPairs {
				var standing int32
				err := cdb.QueryRowContext(ctx, "SELECT standing FROM character_reputation WHERE faction = ? AND guid = ?", pair.oldID, guid).Scan(&standing)
				if err == nil {
					_, _ = cdb.ExecContext(ctx, "DELETE FROM character_reputation WHERE faction = ? AND guid = ?", pair.newID, guid)
					_, _ = cdb.ExecContext(ctx, "UPDATE character_reputation SET faction = ? WHERE faction = ? AND guid = ?", pair.newID, pair.oldID, guid)
				}
			}
		}

		// 6. Language skills switch (CharacterHandler.cpp:1787-1840)
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_skills WHERE guid = ? AND skill IN (98, 109, 111, 759, 313, 113, 673, 115, 315, 137)", guid)
		if newTeam == 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO character_skills (guid, skill, value, max) VALUES (?, 98, 300, 300)", guid) // Common
		} else {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO character_skills (guid, skill, value, max) VALUES (?, 109, 300, 300)", guid) // Orcish
		}
		var racialSkill uint32
		switch race {
		case 3: // Dwarf
			racialSkill = 111
		case 11: // Draenei
			racialSkill = 759
		case 7: // Gnome
			racialSkill = 313
		case 4: // Night Elf
			racialSkill = 113
		case 5: // Undead
			racialSkill = 673
		case 6: // Tauren
			racialSkill = 115
		case 8: // Troll
			racialSkill = 315
		case 10: // Blood Elf
			racialSkill = 137
		}
		if racialSkill != 0 {
			_, _ = cdb.ExecContext(ctx, "INSERT INTO character_skills (guid, skill, value, max) VALUES (?, ?, 300, 300)", guid, racialSkill)
		}

		// 7. Update in-memory player state if active
		if s.player != nil && s.playerGUID == guid {
			s.player.Race = race
			s.player.Gender = gender
			s.player.Name = newName
			s.player.Skin = skin
			s.player.Face = face
			s.player.HairStyle = hairStyle
			s.player.HairColor = hairColor
			s.player.FacialStyle = facialHair
			s.player.Map = uint32(spawnMap)
			s.player.Zone = uint32(spawnZone)
			s.player.X = float32(spawnX)
			s.player.Y = float32(spawnY)
			s.player.Z = float32(spawnZ)
			s.player.Orientation = float32(spawnO)
			s.sendPlayerUpdate()
		}
	}

	buf := protocol.NewBuffer(17 + len(newName))
	buf.WriteU8(0) // RESPONSE_SUCCESS
	buf.WriteU64(guid)
	buf.WriteCString(newName)
	buf.WriteU8(gender)
	buf.WriteU8(skin)
	buf.WriteU8(face)
	buf.WriteU8(hairStyle)
	buf.WriteU8(hairColor)
	buf.WriteU8(facialHair)
	buf.WriteU8(race)
	_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
	return true
}

// handleCompleteMovie processes CMSG_COMPLETE_MOVIE (0x465).
// Reference: WorldSession::HandleCompleteMovie (MiscHandler.cpp:969).
func (s *session) handleCompleteMovie(ctx context.Context, payload []byte) bool {
	if s.player != nil {
		s.player.Movie = 0
		s.player.Cinematic = 1
		s.player.AtLogin &= ^uint32(atLoginFirst)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET cinematic = 1, at_login = at_login & ~32 WHERE guid = ?", s.playerGUID)
		}
	}
	s.debug("movie completed", "account", s.accountName)
	return true
}

// handleComplain processes CMSG_COMPLAIN (0x3C7).
// Reference: WorldSession::HandleComplainOpcode (MiscHandler.cpp:1151).
func (s *session) handleComplain(ctx context.Context, payload []byte) bool {
	buf := protocol.NewBuffer(1)
	buf.WriteU8(0) // complain result: 0 = complaint received
	return s.write(uint16(protocol.OpcodeSMSG_COMPLAIN_RESULT), buf.Bytes(), true) == nil
}

const (
	logoutDelay       = 20 * time.Second
	playerFlagResting = uint32(0x00000020)
	unitFlagStunned   = uint32(0x00040000)
)

func (s *session) setRooted(root bool) {
	if s.player == nil {
		return
	}
	buf := protocol.NewBuffer(16)
	buf.WritePackedGUID(s.playerGUID)
	buf.WriteU32(0) // movement counter
	if root {
		_ = s.write(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), buf.Bytes(), true)
	} else {
		_ = s.write(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT), buf.Bytes(), true)
	}
}

func (s *session) handleLogoutRequest(ctx context.Context) bool {
	if !s.playerLoaded {
		return true
	}
	s.releaseActiveLoot()
	inCombat := s.attackTarget != 0 || (s.player != nil && s.player.UnitFlags&unitFlagInCombat != 0)
	resting := s.player != nil && s.player.PlayerFlags&playerFlagResting != 0
	instantLogoutPermission := false
	if s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		var permissionErr error
		instantLogoutPermission, permissionErr = accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionInstantLogout)
		if permissionErr != nil {
			s.debug("logout permission lookup failed", "account", s.accountName, "permission", permissionInstantLogout, "error", permissionErr)
			instantLogoutPermission = false
		}
	}
	reason := uint32(0)
	if inCombat && !resting {
		reason = 1 // ERR_LOGOUT_IN_COMBAT
	} else if s.isFalling {
		reason = 3 // ERR_LOGOUT_FAILED_FALLING
	} else if s.duelPartner != 0 || s.hasAura(9454) {
		reason = 2 // ERR_LOGOUT_FAILED_DUEL
	}
	if reason != 0 {
		response := protocol.NewBuffer(5)
		response.WriteU32(reason)
		response.WriteU8(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE), response.Bytes(), true)
		return true
	}
	instant := (resting && !inCombat) || instantLogoutPermission || s.inFlight
	response := protocol.NewBuffer(5)
	response.WriteU32(0) // reason 0 = OK
	if instant {
		response.WriteU8(1)
	} else {
		response.WriteU8(0)
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE), response.Bytes(), true); err != nil {
		return false
	}
	if instant {
		if err := s.completeLogout(ctx); err != nil {
			s.debug("player logout failed", "account", s.accountName, "error", err)
		}
		return true
	}
	s.logoutAt = time.Now().Add(logoutDelay)

	// Reference MiscHandler.cpp:446-453:
	// SetStandState(UNIT_STAND_STATE_SIT), SetRooted(true), SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
	if s.player != nil {
		if s.player.StandState == 0 { // UNIT_STAND_STATE_STAND
			s.player.StandState = 1 // UNIT_STAND_STATE_SIT
		}
		s.player.UnitFlags |= unitFlagStunned
		s.setRooted(true)
		s.sendPlayerUpdate()
	}

	s.debug("player logout pending", "account", s.accountName, "delay_seconds", int(logoutDelay/time.Second))
	return true
}

func (s *session) handleLogoutCancel() bool {
	if !s.playerLoaded {
		return true
	}
	s.logoutAt = time.Time{}

	// Reference MiscHandler.cpp:471-483:
	// SetRooted(false), SetStandState(UNIT_STAND_STATE_STAND), RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
	if s.player != nil {
		s.player.StandState = 0 // UNIT_STAND_STATE_STAND
		s.player.UnitFlags &^= unitFlagStunned
		s.setRooted(false)
		s.sendPlayerUpdate()
	}

	s.debug("player logout cancelled", "account", s.accountName)
	return s.write(uint16(protocol.OpcodeSMSG_LOGOUT_CANCEL_ACK), nil, true) == nil
}

// handlePlayerLogout mirrors WorldSession::HandlePlayerLogoutOpcode, whose body
// is empty at the reference commit (MiscHandler.cpp:457): the client sends this
// packet when its own countdown finishes, but logout completion is already
// driven by the server-side pending-logout deadline, so the packet only needs
// to be consumed.
func (s *session) handlePlayerLogout() bool {
	return true
}

func (s *session) completeLogout(ctx context.Context) error {
	if !s.playerLoaded {
		return nil
	}
	s.stopSpellLifecycle()
	s.clearActiveAuras()
	s.stopTimedAchievements()
	s.broadcastGuildMemberLogout()
	s.triggerLogout(ctx)
	s.releaseActiveLoot()
	if s.player != nil && s.player.Health == 0 && s.player.PlayerFlags&playerFlagGhost == 0 && !s.deathTimer.IsZero() {
		s.buildPlayerRepop(ctx)
		s.repopAtGraveyard(ctx)
	}
	if s.playerLoaded && s.player != nil {
		s.handleLeaveBattlefield(ctx, nil)
	}
	if s.trade != nil {
		_ = s.handleCancelTrade(ctx)
	}
	if s.server != nil {
		s.server.removeSessionFromGroup(s)
		s.server.removeSessionChannels(s)
	}
	if s.player != nil && s.player.PetGUID != 0 {
		s.unsummonPet(ctx, petSaveAsCurrent)
	}
	var firstErr error
	if err := s.savePlayerState(ctx, 0); err != nil {
		firstErr = err
		_ = s.savePlayerPosition(ctx)
	}
	if err := s.clearBuybackState(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if !s.superseded {
		if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_ACCOUNT_ONLINE", s.accountID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if !s.superseded {
		if _, err := s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account SET online = 0 WHERE id = ?", s.accountID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_LOGOUT_COMPLETE), nil, true); err != nil {
		return err
	}
	s.playerLoaded = false
	s.player = nil
	s.logoutAt = time.Time{}
	s.debug("player logged out", "account", s.accountName, "guid", s.playerGUID)
	return firstErr
}

func (s *session) clearBuybackState(ctx context.Context) error {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.playerGUID == 0 {
		return nil
	}
	db := s.server.CharactersStore.DB
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot BETWEEN 74 AND 85", s.playerGUID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	items := make([]int64, 0, 12)
	for rows.Next() {
		var item int64
		if err := rows.Scan(&item); err != nil {
			_ = rows.Close()
			_ = tx.Rollback()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		_ = tx.Rollback()
		return err
	}
	_ = rows.Close()
	if _, err := tx.ExecContext(ctx, "DELETE FROM character_inventory WHERE guid = ? AND bag = 0 AND slot BETWEEN 74 AND 85", s.playerGUID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, "DELETE FROM item_instance WHERE guid = ?", item); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for index := range s.buyback {
		s.buyback[index] = nil
	}
	s.currentBuybackSlot = 0
	return nil
}

func (s *session) releaseActiveLoot() {
	loot := s.activeLoot
	if loot == nil {
		return
	}
	if loot.RoundRobinPlayer == s.playerGUID {
		loot.RoundRobinPlayer = 0
	}
	loot.removeViewer(s.playerGUID)
	s.activeLoot = nil
	if loot.Money == 0 && len(loot.Items) == 0 {
		s.clearCreatureLoot(loot)
	}
}

func (s *session) triggerLogout(ctx context.Context) {
	if !s.playerLoaded || s.logoutHook {
		return
	}
	s.logoutHook = true
	if s.server.Features != nil && s.server.Features.LFG != nil {
		s.server.Features.LFG.Leave(s.playerGUID)
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventLogout, s.luaPlayer())
}

func (s *session) savePlayerPosition(ctx context.Context) error {
	if !s.playerLoaded || s.player == nil {
		return nil
	}
	_, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_CHARACTER_POSITION", s.player.X, s.player.Y, s.player.Z, s.player.Orientation, s.player.Map, s.player.Zone, s.player.GUID)
	return err
}

func (s *session) savePlayerState(ctx context.Context, online uint32) error {
	if !s.playerLoaded || s.player == nil || s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	state := s.player
	taxi := make([]string, len(state.TaxiMask))
	for i, value := range state.TaxiMask {
		taxi[i] = strconv.FormatUint(uint64(value), 10)
	}
	titles := make([]string, len(state.KnownTitles))
	for i, value := range state.KnownTitles {
		titles[i] = strconv.FormatUint(uint64(value), 10)
	}
	explored := strings.Builder{}
	for _, value := range state.ExploredZones {
		fmt.Fprintf(&explored, "%02x%02x%02x%02x", byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
	}
	args := []any{state.Name, state.Race, state.Class, state.Gender, state.Level, state.XP, state.Money, state.Skin, state.Face, state.HairStyle, state.HairColor, state.FacialStyle, state.BankBagSlots, state.RestState, state.PlayerFlags, state.Map, state.InstanceID, state.InstanceModeMask, state.X, state.Y, state.Z, state.Orientation, state.TransportX, state.TransportY, state.TransportZ, state.TransportO, state.TransportGUID, strings.Join(taxi, " "), state.Cinematic, state.TotalPlayedTime, state.LevelPlayedTime, state.RestBonus, state.LogoutTime, state.LogoutResting, state.ResetTalentsCost, state.ResetTalentsTime, state.ExtraFlags, state.StableSlots, state.AtLogin, state.Zone, s.deathExpireTime, state.TaxiPath, state.ArenaPoints, state.TotalHonorPoints, state.TodayHonorPoints, state.YesterdayHonorPoints, state.TotalKills, state.TodayKills, state.YesterdayKills, state.ChosenTitle, state.KnownCurrency, state.WatchedFaction, state.DrunkenState, state.Health, state.Powers[0], state.Powers[1], state.Powers[2], state.Powers[3], state.Powers[4], state.Powers[5], state.Powers[6], s.latency.Load(), state.TalentGroupsCount, state.ActiveTalentGroup, explored.String(), state.Equipment, state.AmmoID, strings.Join(titles, " "), state.ActionBars, state.GrantableLevels, online, state.GUID}
	_, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_CHARACTER", args...)
	if err == nil {
		err = s.saveFishingSteps(ctx, state)
	}
	return err
}

func (s *session) saveFishingSteps(ctx context.Context, state *playerState) error {
	if s == nil || state == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	if state.FishingSteps == 0 {
		if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_fishingsteps WHERE guid = ?", state.GUID); err != nil && !missingTable(err) {
			return err
		}
		return nil
	}
	if _, err := s.server.CharactersStore.DB.ExecContext(ctx, "REPLACE INTO character_fishingsteps (guid, fishingSteps) VALUES (?, ?)", state.GUID, state.FishingSteps); err != nil && !missingTable(err) {
		return err
	}
	return nil
}

func isReadTimeout(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

const (
	globalAccountDataMask    uint32 = 0x15
	characterAccountDataMask uint32 = 0xEA
)

func buildLoginVerifyWorld(state playerState) []byte {
	packet := protocol.NewBuffer(20)
	packet.WriteI32(int32(state.Map))
	packet.WriteF32(state.X)
	packet.WriteF32(state.Y)
	packet.WriteF32(state.Z)
	packet.WriteF32(state.Orientation)
	return packet.Bytes()
}

func buildAccountDataTimes(now time.Time, mask uint32) []byte {
	return buildAccountDataTimesWithTimestamps(now, mask, nil)
}

func buildAccountDataTimesWithTimestamps(now time.Time, mask uint32, timestamps []uint32) []byte {
	packet := protocol.NewBuffer(29)
	packet.WriteU32(uint32(now.Unix()))
	packet.WriteU8(1)
	packet.WriteU32(mask)
	for index := uint32(0); index < 8; index++ {
		if mask&(1<<index) != 0 {
			var timestamp uint32
			if int(index) < len(timestamps) {
				timestamp = timestamps[index]
			}
			packet.WriteU32(timestamp)
		}
	}
	return packet.Bytes()
}

func (s *session) loadAccountDataTimes(ctx context.Context, guid uint64, mask uint32) []uint32 {
	timestamps := make([]uint32, 8)
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return timestamps
	}
	for index := uint32(0); index < 8; index++ {
		if mask&(1<<index) == 0 {
			continue
		}
		var timestamp int64
		var err error
		if globalAccountDataMask&(1<<index) != 0 {
			err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT time FROM account_data WHERE accountId = ? AND type = ?", s.accountID, index).Scan(&timestamp)
		} else {
			err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT time FROM character_account_data WHERE guid = ? AND type = ?", guid, index).Scan(&timestamp)
		}
		if err == nil && timestamp > 0 {
			timestamps[index] = uint32(timestamp)
		}
	}
	return timestamps
}

func buildRealmSplit(payload []byte) ([]byte, error) {
	reader := protocol.NewReader(payload)
	value, err := reader.ReadU32()
	if err != nil {
		return nil, err
	}
	packet := protocol.NewBuffer(17)
	packet.WriteU32(value)
	packet.WriteU32(0)
	packet.WriteCString("01/01/01")
	return packet.Bytes(), nil
}

func buildFeatureSystemStatus() []byte { return []byte{2, 0} }

func buildMotd(message string) []byte {
	lines := strings.Split(message, "@")
	packet := protocol.NewBuffer(4 + len(message) + len(lines))
	packet.WriteU32(uint32(len(lines)))
	for _, line := range lines {
		packet.WriteCString(line)
	}
	return packet.Bytes()
}

func buildLearnedDanceMoves() []byte {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(0)
	packet.WriteU32(0)
	return packet.Bytes()
}

func buildInitWorldStates(state playerState, areaID, arenaSeasonID uint32, arenaSeasonInProgress bool) []byte {
	season := int32(0)
	previousSeason := int32(0)
	if arenaSeasonInProgress {
		season = int32(arenaSeasonID)
		if arenaSeasonID > 0 {
			previousSeason = int32(arenaSeasonID - 1)
		}
	}
	worldStates := [][2]int32{{2264, 0}, {2263, 0}, {2262, 0}, {2261, 0}, {2260, 0}, {2259, 0}, {3191, season}, {3901, previousSeason}}
	if state.Map == 530 {
		worldStates = append(worldStates, [2]int32{2495, 0}, [2]int32{2493, 15}, [2]int32{2491, 15})
	} else if state.Map == 489 {
		worldStates = append(worldStates,
			[2]int32{1581, 0}, // WS_FLAG_CAPTURES_ALLIANCE
			[2]int32{1582, 0}, // WS_FLAG_CAPTURES_HORDE
			[2]int32{1545, 0},
			[2]int32{1546, 0},
			[2]int32{1547, 2},
			[2]int32{1601, 3}, // WS_FLAG_MAX_CAPTURES
			[2]int32{2338, 1}, // WS_FLAG_STATE_HORDE (1 = base)
			[2]int32{2339, 1}, // WS_FLAG_STATE_ALLIANCE (1 = base)
		)
	} else if state.Map == 529 {
		worldStates = append(worldStates,
			[2]int32{1767, 0},
			[2]int32{1768, 0},
			[2]int32{1769, 0},
			[2]int32{1770, 0},
			[2]int32{1772, 0},
			[2]int32{1773, 0},
			[2]int32{1774, 0},
			[2]int32{1775, 0},
			[2]int32{1776, 0}, // Resources Ally
			[2]int32{1777, 0}, // Resources Horde
			[2]int32{1778, 0},
			[2]int32{1779, 0},
			[2]int32{1780, 2000},
			[2]int32{1782, 0},
			[2]int32{1783, 0},
			[2]int32{1784, 0},
			[2]int32{1785, 0},
			[2]int32{1787, 0},
			[2]int32{1788, 0},
			[2]int32{1789, 0},
			[2]int32{1790, 0},
			[2]int32{1792, 0},
			[2]int32{1793, 0},
			[2]int32{1794, 0},
			[2]int32{1795, 0},
			[2]int32{1842, 1},
			[2]int32{1843, 1},
			[2]int32{1844, 1},
			[2]int32{1845, 1},
			[2]int32{1846, 1},
			[2]int32{1861, 2},
			[2]int32{1955, 1800},
		)
	} else if state.Map == 566 {
		worldStates = append(worldStates,
			[2]int32{2753, 0},
			[2]int32{2752, 0},
			[2]int32{2742, 0},
			[2]int32{2741, 0},
			[2]int32{2740, 0},
			[2]int32{2739, 0},
			[2]int32{2738, 0},
			[2]int32{2737, 0},
			[2]int32{2736, 0},
			[2]int32{2735, 0},
			[2]int32{2733, 0},
			[2]int32{2732, 0},
			[2]int32{2731, 1},
			[2]int32{2730, 0},
			[2]int32{2729, 0},
			[2]int32{2728, 1},
			[2]int32{2727, 0},
			[2]int32{2726, 0},
			[2]int32{2725, 1},
			[2]int32{2724, 0},
			[2]int32{2723, 0},
			[2]int32{2722, 1},
			[2]int32{2757, 1},
			[2]int32{2770, 1},
			[2]int32{2769, 1},
			[2]int32{2749, 0}, // Resources Ally
			[2]int32{2750, 0}, // Resources Horde
			[2]int32{2565, 142},
			[2]int32{2720, 0},
			[2]int32{2719, 0},
			[2]int32{2718, 0},
			[2]int32{3085, 379},
		)
	} else if state.Map == 571 && state.Zone == 4197 {
		worldStates = append(worldStates,
			[2]int32{3801, 1}, // WS_BATTLEFIELD_WG_ACTIVE (1 = peace)
			[2]int32{3803, 0}, // WS_BATTLEFIELD_WG_DEFENDER (0 = Alliance)
			[2]int32{3802, 1}, // WS_BATTLEFIELD_WG_ATTACKER (1 = Horde)
			[2]int32{3490, 0}, // WS_BATTLEFIELD_WG_VEHICLE_A
			[2]int32{3491, 8}, // WS_BATTLEFIELD_WG_MAX_VEHICLE_A
			[2]int32{3680, 0}, // WS_BATTLEFIELD_WG_VEHICLE_H
			[2]int32{3681, 8}, // WS_BATTLEFIELD_WG_MAX_VEHICLE_H
		)
	}
	worldStates = append(worldStates, initialZoneWorldStates(state.Zone)...)
	packet := protocol.NewBuffer(16 + len(worldStates)*8)
	packet.WriteI32(int32(state.Map))
	packet.WriteI32(int32(state.Zone))
	packet.WriteI32(int32(areaID))
	packet.WriteU16(uint16(len(worldStates)))
	for _, worldState := range worldStates {
		packet.WriteI32(worldState[0])
		packet.WriteI32(worldState[1])
	}
	return packet.Bytes()
}

func BuildInitialWorldStates(mapID, zoneID, areaID, arenaSeasonID uint32, arenaSeasonInProgress bool) []byte {
	return buildInitWorldStates(playerState{Map: mapID, Zone: zoneID}, areaID, arenaSeasonID, arenaSeasonInProgress)
}

func buildInstanceDifficulty(difficulty uint32) []byte {
	return buildInstanceDifficultyForMap(difficulty, false)
}

func buildInstanceDifficultyForMap(difficulty uint32, dynamic bool) []byte {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(difficulty)
	if dynamic {
		packet.WriteU32(1)
	} else {
		packet.WriteU32(0)
	}
	return packet.Bytes()
}

func (s *session) loginInstanceDifficulty(ctx context.Context, state playerState) (uint32, bool) {
	difficulty := uint32(0)
	dynamic := false
	instanceDifficulty, hasInstanceDifficulty := s.loadLoginInstanceDifficulty(ctx, state)
	if hasInstanceDifficulty {
		difficulty = instanceDifficulty
	}
	if s == nil || s.server == nil || s.server.Data == nil {
		return difficulty, dynamic
	}
	entry, ok, err := s.server.Data.Map(state.Map)
	if err != nil || !ok {
		return difficulty, dynamic
	}
	if entry.IsRaid() {
		if !hasInstanceDifficulty {
			difficulty = uint32(state.RaidDifficulty)
		}
		dynamic = entry.IsDynamicDifficultyMap() && difficulty >= 2
	} else if entry.IsDungeon() {
		if !hasInstanceDifficulty {
			difficulty = uint32(state.DungeonDifficulty)
		}
		dynamic = entry.IsDynamicDifficultyMap() && difficulty >= 1
	}
	return difficulty, dynamic
}

func (s *session) loadLoginInstanceDifficulty(ctx context.Context, state playerState) (uint32, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || state.InstanceID == 0 {
		return 0, false
	}
	var difficulty int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT difficulty FROM instance WHERE id = ?", state.InstanceID).Scan(&difficulty); err != nil || difficulty < 0 {
		return 0, false
	}
	return uint32(difficulty), true
}

func (s *session) sendLoginRaidDifficulty(ctx context.Context, state playerState) error {
	stored := uint8(0)
	if stored >= 4 {
		stored = 0
	}
	mapIsRaid := false
	if s != nil && s.server != nil && s.server.Data != nil {
		if entry, ok, err := s.server.Data.Map(state.Map); err == nil && ok {
			mapIsRaid = entry.IsRaid()
		}
	}
	forced := state.RaidDifficulty
	if mapIsRaid {
		if mapDifficulty, ok := s.loadLoginInstanceDifficulty(ctx, state); ok {
			if mapDifficulty >= 4 {
				mapDifficulty = 0
			}
			stored = uint8(mapDifficulty)
			forced = uint8(mapDifficulty)
		} else {
			stored = state.RaidDifficulty
		}
		if forced == state.RaidDifficulty {
			return nil
		}
	} else if state.RaidDifficulty == stored {
		return nil
	} else {
		forced = state.RaidDifficulty
	}
	isInGroup := uint32(0)
	if s.groupID != 0 {
		isInGroup = 1
	}
	packet := protocol.NewBuffer(12)
	packet.WriteU32(uint32(forced))
	packet.WriteU32(1)
	packet.WriteU32(isInGroup)
	return s.write(uint16(protocol.OpcodeMSG_SET_RAID_DIFFICULTY), packet.Bytes(), true)
}

func buildTimeSyncRequest(counter uint32) []byte {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(counter)
	return packet.Bytes()
}

func buildTutorialFlags(tutorials [8]uint32) []byte {
	packet := protocol.NewBuffer(32)
	for _, tut := range tutorials {
		packet.WriteU32(tut)
	}
	return packet.Bytes()
}

// handleSetPlayerDeclinedNames processes CMSG_SET_PLAYER_DECLINED_NAMES (0x419).
// Reference: WorldSession::HandleSetPlayerDeclinedNames (CharacterHandler.cpp:1198).
func (s *session) handleSetPlayerDeclinedNames(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()

	// Read player name and 5 declined name cases: genitive, dative, accusative, instrumental, prepositional
	name, _ := r.ReadCString()
	var declined [5]string
	for i := 0; i < 5; i++ {
		declined[i], _ = r.ReadCString()
	}

	result := uint32(0) // 0 = SUCCESS, 1 = ERROR
	cdb := s.server.CharactersStore.DB
	if cdb != nil && guid > 0 {
		var charName string
		err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid).Scan(&charName)
		if err != nil || (name != "" && charName != name) {
			result = 1
		} else {
			// Persist declined names to character_declinedname
			_, _ = cdb.ExecContext(ctx,
				`REPLACE INTO character_declinedname (guid, genitive, dative, accusative, instrumental, prepositional)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				guid, declined[0], declined[1], declined[2], declined[3], declined[4])
		}
	}

	buf := protocol.NewBuffer(12)
	buf.WriteU32(result)
	buf.WriteU64(guid)
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_PLAYER_DECLINED_NAMES_RESULT), buf.Bytes(), true)
	return true
}
