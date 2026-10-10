package world

import (
	"bytes"
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
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/version"
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
	playerFlagOutOfBounds        uint32 = 0x00004000 // PLAYER_FLAGS_IS_OUT_OF_BOUNDS (Player.h:347) — cleared by Player::RepopAtGraveyard
	playerFlagInPVP              uint32 = 0x00000200
	playerFlagPVPTimer           uint32 = 0x00040000
	playerFlagTaxiBenchmark      uint32 = 0x00020000 // PLAYER_FLAGS_TAXI_BENCHMARK (Player.h:350)
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
	charCreatePvpTeamsViolation         = 52
	charCreateServerLimit               = 53
	charCreateAccountLimit              = 54
	charCreateExpansion                 = 57
	charCreateExpansionClass            = 58
	charCreateLevelRequirement          = 59
	charCreateUniqueClassLimit          = 60
	charCreateNameInUse                 = 50
	// SendCharRename/SendCharCustomize/SendCharFactionChange result codes
	// (SharedDefines.h:3447-3463, :3428-3431).
	charCreateCharacterArenaLeader = 64
	charCreateCharacterSwapFaction = 66
	charCreateCharacterRaceOnly    = 67
	charNameFailure                = 88
	charNameNoName                 = 89
	charNameReserved               = 95
	charDeleteSuccess              = 71
	charDeleteFailed               = 72
	charDeleteFailedGuildLeader    = 74
	charDeleteFailedArenaCaptain   = 75
	// Player::DeleteFromDB delete methods (Player.h:785-786).
	charDeleteRemove = 0
	charDeleteUnlink = 1
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

func CharacterCreationDefaults() (uint32, uint32) {
	return ^uint32(0), 0
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
	s.characterNames = make(map[uint64]enumCharacter)
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
		s.characterNames[character.GUID] = character
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
	return strings.Join(parts, " ") + " "
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
	// normalizePlayerName (CharacterHandler.cpp:377): the name C++ validates
	// and stores is the normalized form (lowercased, first letter upper);
	// empty/invalid-UTF-8 answers CHAR_NAME_NO_NAME.
	if !utf8.ValidString(name) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charNameNoName)
	}
	name = normalizePlayerName(name)
	if name == "" {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charNameNoName)
	}
	values := make([]uint8, 9)
	for i := range values {
		values[i], err = b.ReadU8()
		if err != nil {
			return false
		}
	}
	race, class, gender := values[0], values[1], values[2]
	if race == 0 || class == 0 || gender > 2 {
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
	if !raceAllowed || !classAllowed {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateFailed)
	}
	if raceRequiredExpansion > uint32(s.accountExpansion) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateExpansion)
	}
	if classRequiredExpansion > uint32(s.accountExpansion) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateExpansionClass)
	}
	if s.server.Config.CharacterCreatingDisabledRaceMask&(uint32(1)<<(race-1)) != 0 {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateDisabled)
	}
	if s.server.Config.CharacterCreatingDisabledClassMask&(uint32(1)<<(class-1)) != 0 {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateDisabled)
	}
	// ObjectMgr::CheckPlayerName approximation (CharacterHandler.cpp:381-386):
	// validCharacterName covers the length/alphabet arms; runs after the
	// creation-mask gates, == C++ order.
	if !validCharacterName(name) {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreateFailed)
	}
	// ObjectMgr::IsReservedName (ObjectMgr.cpp:8515-8524): lowercased exact
	// match against the reserved_name rows, gated on
	// RBAC_PERM_SKIP_CHECK_CHARACTER_CREATION_RESERVEDNAME
	// (CharacterHandler.cpp:392). Answers CHAR_NAME_RESERVED (95); the gate
	// sits between the CheckPlayerName-equivalent (validCharacterName just
	// above) and the death-knight arm below, == C++ order.
	if !s.skipReservedNameCheck {
		var reserved int
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM reserved_name WHERE name = ?", strings.ToLower(name)).Scan(&reserved); err == nil && reserved > 0 {
			return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charNameReserved)
		}
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
	if s.server.Config.GameType == 1 || s.server.Config.GameType == 4 || s.server.Config.GameType == 8 {
		var accRace uint8
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT race FROM characters WHERE account = ? LIMIT 1", s.accountID).Scan(&accRace)
		if err == nil {
			var accTeam uint8
			if accRace > 0 {
				accTeam = raceTeam(accRace)
			}
			if accTeam != raceTeam(race) {
				return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), charCreatePvpTeamsViolation)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return false
		}
	}
	guid, err := s.server.allocateCharacterGUID(ctx)
	if err != nil {
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

	watchedFaction, drunkenness := CharacterCreationDefaults()
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
		uint64(0), watchedFaction, drunkenness, uint32(1),
		uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0),
		uint8(1), uint8(0), "", "", uint32(0), "", uint8(0), uint32(0),
	}
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_CHARACTER", args...); err != nil {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_CREATE), 48)
	}
	if err := s.ensureStartingGuild(ctx, guid); err != nil {
		s.debug("starting guild assignment failed", "account", s.accountName, "guid", guid, "error", err)
	}
	_, _ = s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_PLAYER_HOMEBIND", guid, spawn.Map, spawn.Zone, spawn.X, spawn.Y, spawn.Z)
	s.initializeCreatedPlayerStats(ctx, guid, race, class, startLevel)

	// Populate starter spells, skills, actions, equipment
	s.createStarterSpells(ctx, guid, race, class)
	s.createStarterSkills(ctx, guid, race, class, startLevel)
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
	// WorldSession::HandleCharDeleteOpcode (CharacterHandler.cpp:641-666): a
	// loaded character cannot be deleted (silent return, no result packet);
	// guild leaders and arena team captains are rejected with dedicated codes.
	if s.playerLoaded && s.playerGUID == guid {
		return true
	}
	var guildLeaderCount int
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild WHERE leaderguid = ?", guid).Scan(&guildLeaderCount); err == nil && guildLeaderCount > 0 {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), charDeleteFailedGuildLeader)
	}
	var arenaCaptainCount int
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM arena_team WHERE captainGuid = ?", guid).Scan(&arenaCaptainCount); err == nil && arenaCaptainCount > 0 {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), charDeleteFailedArenaCaptain)
	}
	var accountID uint32
	var charClass, charLevel uint8
	err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account, class, level FROM characters WHERE guid = ?", guid).Scan(&accountID, &charClass, &charLevel)
	if errors.Is(err, sql.ErrNoRows) || err != nil || accountID != s.accountID {
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), 72)
	}
	// Player::DeleteFromDB (Player.cpp:4204-4221): the delete method comes
	// from CharDelete.Method (0 = REMOVE, 1 = UNLINK); a character below the
	// configured minimum level always takes REMOVE. This is the
	// HandleCharDeleteOpcode path, so deleteFinally is never set.
	deleteMethod := s.server.Config.CharDeleteMethod
	if deleteMethod == charDeleteUnlink {
		minLevel := s.server.Config.CharDeleteMinLevel
		if charClass == 6 { // CLASS_DEATH_KNIGHT (Player.h:784)
			minLevel = s.server.Config.CharDeleteDeathKnightMinLevel
		}
		if uint32(charLevel) < minLevel {
			deleteMethod = charDeleteRemove
		}
	}
	tx, err := s.server.CharactersStore.DB.BeginTx(ctx, nil)
	if err != nil {
		return false
	}
	defer tx.Rollback()
	// CalendarMgr::RemoveAllPlayerEventsAndInvites (CalendarMgr.cpp:285-293):
	// events created by the deleted character are dropped with the removed
	// alert (SendCalendarEventRemovedAlert, u8(1) + event id + packed time) to
	// online relatives; the calendar mail arm is skipped because the remover
	// is empty on this path. The row deletes themselves run inside
	// deleteCharacterOwnedState below, before the character row goes away.
	type removedCalendarEvent struct {
		id        uint64
		eventTime uint32
	}
	var removedCalEvents []removedCalendarEvent
	if calRows, calErr := tx.QueryContext(ctx, "SELECT id, eventtime FROM calendar_events WHERE creator = ?", guid); calErr == nil {
		for calRows.Next() {
			var ev removedCalendarEvent
			if calRows.Scan(&ev.id, &ev.eventTime) == nil {
				removedCalEvents = append(removedCalEvents, ev)
			}
		}
		calRows.Close()
	}
	for _, ev := range removedCalEvents {
		remBuf := protocol.NewBuffer(16)
		remBuf.WriteU8(1)
		remBuf.WriteU64(ev.id)
		remBuf.WritePackedTime(time.Unix(int64(ev.eventTime), 0))
		for _, t := range calendarEventRelativeSessions(ctx, s.server, ev.id) {
			_ = t.write(uint16(protocol.OpcodeSMSG_CALENDAR_EVENT_REMOVED_ALERT), remBuf.Bytes(), true)
		}
	}
	// Player::DeleteFromDB pre-switch prelude (Player.cpp:4228-4246) plus the
	// HandleCharDeleteOpcode calendar arm (CharacterHandler.cpp:699): shared
	// by both delete methods.
	if err := deleteCharacterPrelude(ctx, tx, guid); err != nil {
		return false
	}
	if deleteMethod == charDeleteUnlink {
		// Player::DeleteFromDB CHAR_DELETE_UNLINK (Player.cpp:4537-4543,
		// CHAR_UPD_DELETE_INFO): the character is unlinked from the account,
		// the name is freed for reuse and the row shows as deleted in-game.
		// Owned state, mails, pets and social rows are kept, and npcbot
		// owners are not reset (REMOVE-only per Player.cpp:4532).
		if _, err := tx.ExecContext(ctx, "UPDATE characters SET deleteInfos_Name = name, deleteInfos_Account = account, deleteDate = UNIX_TIMESTAMP(), name = '', account = 0 WHERE guid = ?", guid); err != nil {
			return false
		}
		if err := tx.Commit(); err != nil {
			return false
		}
		delete(s.legitimate, guid)
		return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), charDeleteSuccess)
	}
	// Player::DeleteFromDB (Player.cpp:4349): online players who had the
	// deleted character on their social list get FRIEND_REMOVED. Capture the
	// contact list before the wipe below deletes the rows.
	var deletedContacts []uint64
	if rows, qerr := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT friend FROM character_social WHERE guid = ?", guid); qerr == nil {
		for rows.Next() {
			var contactGUID uint64
			if rows.Scan(&contactGUID) == nil {
				deletedContacts = append(deletedContacts, contactGUID)
			}
		}
		rows.Close()
	}
	// Player::DeleteFromDB CHAR_DELETE_REMOVE (Player.cpp:4253): the deleted
	// character's COD mails carrying items are returned to their senders
	// before any owned-state rows are wiped, so the re-homed items survive
	// the item_instance-by-owner cleanup below.
	if err := s.deleteCharacterReturnMails(ctx, tx, guid, accountID); err != nil {
		return false
	}
	if err := deleteCharacterOwnedState(ctx, tx, guid, s.server.Config.DeletedCharacterTicketTrace); err != nil {
		return false
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM characters WHERE guid = ?", guid); err != nil {
		return false
	}
	if err := tx.Commit(); err != nil {
		return false
	}
	s.releaseNpcBotOwnerCache(guid)
	for _, contactGUID := range deletedContacts {
		if sess := s.server.findSessionByGUID(contactGUID); sess != nil && sess.worldReady.Load() {
			_ = sess.sendFriendStatus(friendsResultRemoved, guid, "")
		}
	}
	delete(s.legitimate, guid)
	return sendCharacterResult(s, uint16(protocol.OpcodeSMSG_CHAR_DELETE), 71)
}

func (s *session) handlePlayerLogin(ctx context.Context, payload []byte) (success bool) {
	var guid uint64
	var loginSelection worldportInstanceSelection
	var loginInstanceCommitted, loginAdmissionReserved, loginAdmissionCommitted bool
	s.playerLoading = true
	s.worldReady.Store(false)
	s.setPendingBind(0, 0, 0, 0)
	defer func() {
		s.playerLoading = false
		if success {
			s.initialLoginPending = false
		} else {
			s.initialLoginPending = false
			s.worldReady.Store(false)
			if s.server != nil && loginAdmissionReserved && !loginAdmissionCommitted {
				s.server.releaseWorldportAdmission(instanceAdmissionKey{MapID: loginSelection.MapID, InstanceID: loginSelection.InstanceID})
			}
			if s.server != nil && loginSelection.Reserved && !loginInstanceCommitted {
				s.server.releaseWorldportInstanceID(loginSelection.InstanceID)
			}
		}
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
	s.initialLoginPending = true
	mapEntry, mapFound, err := s.server.Data.Map(state.Map)
	if err != nil || !mapFound {
		return false
	}
	loginSelection = worldportInstanceSelection{MapID: state.Map, InstanceID: state.InstanceID}
	if mapEntry.IsDungeon() {
		var canCreate bool
		loginSelection, canCreate, err = s.resolveWorldportInstance(ctx, state, mapEntry)
		if err != nil {
			return false
		}
		if !canCreate {
			if loginSelection.Reserved {
				s.server.releaseWorldportInstanceID(loginSelection.InstanceID)
				loginSelection.Reserved = false
			}
			if err := s.recoverLoginInstanceMapFailure(ctx, &state); err != nil {
				return false
			}
			mapEntry, mapFound, err = s.server.Data.Map(state.Map)
			if err != nil || !mapFound {
				return false
			}
			loginSelection = worldportInstanceSelection{MapID: state.Map, InstanceID: state.InstanceID}
		} else {
			state.InstanceID = loginSelection.InstanceID
		}
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
	s.nearTeleportPending = false
	s.nearTeleportDest = nearTeleportDestination{}
	s.clearLastMovementInfo()
	s.rooted = false
	s.playerLocked = false
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
	s.contestedPVPEnd = time.Time{}
	s.playerLoaded = true
	if err := s.ensureStartingGuild(ctx, guid); err != nil {
		s.debug("starting guild assignment failed", "account", s.accountName, "guid", guid, "error", err)
	}
	if mapEntry.IsDungeon() {
		if !s.worldportInstanceHasRoom(loginSelection, mapEntry) {
			s.sendTransferAborted(state.Map, transferAbortMaxPlayers, 0)
			if loginSelection.Reserved {
				s.server.releaseWorldportInstanceID(loginSelection.InstanceID)
				loginSelection.Reserved = false
			}
			if err := s.recoverLoginInstanceMapFailure(ctx, &state); err != nil {
				return false
			}
			mapEntry, mapFound, err = s.server.Data.Map(state.Map)
			if err != nil || !mapFound || mapEntry.IsDungeon() {
				return false
			}
			loginSelection = worldportInstanceSelection{MapID: state.Map, InstanceID: state.InstanceID}
			s.player = &state
		} else {
			loginAdmissionReserved = !s.isMapAdmissionGM()
		}
	}
	difficulty := protocol.NewBuffer(12)
	difficulty.WriteU32(uint32(state.DungeonDifficulty))
	difficulty.WriteU32(1)
	difficulty.WriteU32(0)
	// Item-dependent passive auras (Player::_ApplyAllItemMods first loop,
	// Player.cpp:8361-8380): runs before the equip-spell leg like the C++
	// loop order; aura packets go out before LOGIN_VERIFY_WORLD.
	s.applyItemDependentAuras(ctx)
	// Login equip-spell re-application (Player::_ApplyAllItemMods equip leg,
	// Player.cpp:8385-8401): runs inside LoadFromDB before SendDungeonDifficulty
	// in C++; the cast merges with the DB-loaded auras, resetting equip-aura
	// durations to full, and the aura packets go out before LOGIN_VERIFY_WORLD.
	s.applyLoginItemEquipSpells(ctx)
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
	if s.server.Config.ServerLoginInfo {
		s.sendSysMessage(version.String())
	}
	s.sendGuildLoginInfo(ctx)
	if err := s.write(uint16(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES), buildLearnedDanceMoves(), true); err != nil {
		return false
	}
	s.loadExploredZones(ctx)
	if err := s.sendInitialPacketsBeforeAddToMap(ctx, state); err != nil {
		return false
	}
	s.lastFallZ = state.Z
	s.lastFallTime = 0
	if state.Cinematic == 0 {
		if !s.handleOpeningCinematic() {
			return false
		}
		if s.server.Config.PlayerStartString != "" {
			s.sendSysMessage(s.server.Config.PlayerStartString)
		}
	}
	updates, err := s.server.buildPlayerUpdate(state)
	if err != nil {
		return false
	}
	attachedTransport, err := s.server.buildAttachedTransportUpdate(ctx, state)
	if err != nil {
		return false
	}
	attachedTransportPassengers, err := s.server.buildAttachedTransportPassengerUpdates(ctx, state, s)
	if err != nil {
		return false
	}
	attachedTransportPlayers, attachedTransportPlayerGUIDs, err := s.server.buildAttachedTransportPlayerUpdates(state, s.playerGUID, s)
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
	if s.server.TraceRecorder != nil {
		payload := initialUpdate.Payload.Bytes()
		if initialUpdate.Opcode == uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
			payload, err = protocol.DecompressUpdatePayload(payload)
			if err != nil {
				return false
			}
		}
		if len(s.loginCreateBlock) == 0 || !bytes.Contains(payload, s.loginCreateBlock) {
			return false
		}
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
	if loginSelection.CreateSave || loginSelection.BindPlayer || loginSelection.BindGroup || loginSelection.UnbindPlayerInstanceID != 0 {
		if err := s.persistWorldportInstance(ctx, loginSelection); err != nil {
			return false
		}
		loginInstanceCommitted = true
		if loginSelection.Reserved {
			s.server.releaseWorldportInstanceID(loginSelection.InstanceID)
		}
	}
	s.commitWorldportAdmission(loginSelection, loginAdmissionReserved)
	loginAdmissionCommitted = true
	s.sendWorldportGroupLockWarning(ctx, loginSelection)
	s.recordInstanceEnterTime(ctx, time.Now())
	sendNearbyObjects := func() bool {
		var nearbyPlayers, nearbyCreatures, nearbyGameObjects, nearbyCorpses *protocol.Packet
		var nearbyPlayerGUIDs []uint64
		var playerCount, creatureCount, goCount, corpseCount int
		var creatureErr, goErr, corpseErr error
		var wg sync.WaitGroup
		wg.Add(4)
		go func() {
			defer wg.Done()
			nearbyPlayers, playerCount, nearbyPlayerGUIDs = s.server.buildNearbyPlayerUpdatesWithCreated(s)
		}()
		go func() {
			defer wg.Done()
			nearbyCreatures, creatureCount, creatureErr = s.server.buildNearbyCreatureUpdates(ctx, state, s.currentPlayerPhaseMask(), s)
		}()
		go func() {
			defer wg.Done()
			nearbyGameObjects, goCount, goErr = s.server.buildNearbyGameObjectUpdates(ctx, state, false, s)
		}()
		go func() {
			defer wg.Done()
			nearbyCorpses, corpseCount, corpseErr = s.server.buildNearbyCorpseUpdates(ctx, state, s.currentPlayerPhaseMask())
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
		s.sendVisiblePlayerAuras(nearbyPlayerGUIDs)
		s.sendVisibleCreatureAuras(state)
		return true
	}
	if !sendNearbyObjects() {
		return false
	}
	if s.loadedCorpseBones {
		s.spawnLoadedCorpseBones(ctx)
		s.loadedCorpseBones = false
	}
	s.triggerPlayerEvent(ctx, scripting.PlayerEventMapChange, s.luaPlayer())
	s.triggerMapEntryEvent(ctx)
	s.fireInstancePlayerEnter(ctx)
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
	s.exploreArea(ctx, areaID)
	if err := s.write(uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES), buildInitWorldStates(state, areaID, s.server.Config.ArenaSeasonID, s.server.Config.ArenaSeasonInProgress), true); err != nil {
		return false
	}
	s.lastStreamX, s.lastStreamY, s.lastStreamZ = state.X, state.Y, state.Z
	s.resetTimeSync()
	if err := s.write(uint16(protocol.OpcodeSMSG_TIME_SYNC_REQ), buildTimeSyncRequest(0), true); err != nil {
		return false
	}
	s.recordTimeSyncSent(0)
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
	if err := s.sendLoginMovementStunAndCompoundStates(); err != nil {
		return false
	}
	s.sendLoadedAuras()
	s.sendLoginCharmControl()
	s.sendPlayerCollisionHeight()
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
	if err := s.sendSharedQuestDetails(ctx); err != nil {
		return false
	}
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_CHAR_ONLINE", guid); err != nil {
		return false
	}
	s.debug("world login stage", "stage", "character-online-complete", "guid", guid)
	if _, err := s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_ACCOUNT_ONLINE", s.accountID); err != nil {
		return false
	}
	s.gameTimeStartedAt = time.Now()
	s.debug("world login stage", "stage", "account-online-complete", "guid", guid)
	s.sendLoadedGroup()
	s.server.broadcastFriendStatus(s.playerGUID, friendsResultOnline, uint32(state.Zone), uint32(state.Level), uint32(state.Class))
	if state.PlayerFlags&playerFlagGhost != 0 && !s.player.repopOnLogin {
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
		var petID, entry, modelID, level, petType, reactState, curHealth, curMana, createdBySpell int64
		var petName string
		if err := cdb.QueryRowContext(ctx,
			"SELECT id, entry, modelid, level, name, curhealth, curmana, COALESCE(PetType, 0), COALESCE(Reactstate, 1), COALESCE(CreatedBySpell, 0) FROM character_pet WHERE owner = ? AND slot = 0",
			s.playerGUID).Scan(&petID, &entry, &modelID, &level, &petName, &curHealth, &curMana, &petType, &reactState, &createdBySpell); err == nil {
			temporarySummon := false
			if createdBySpell > 0 && s.server.Data != nil {
				if spell, found, spellErr := s.server.Data.Spell(uint32(createdBySpell)); spellErr == nil && found && spell.DurationIndex > 0 {
					if duration, durationFound, durationErr := s.server.Data.SpellDuration(spell.DurationIndex, 1); durationErr == nil && durationFound && duration > 0 {
						temporarySummon = true
					}
				}
			}
			if !ShouldTemporarilyUnsummonSavedPet(state.Health, state.PlayerFlags, state.UnitFlags, state.MountDisplayID, temporarySummon) {
				petLevel := uint32(level)
				if petType == 0 && state.Level > 0 {
					petLevel = uint32(state.Level)
				}
				maxHP, _, maxMP, _ := s.getPetStats(ctx, uint32(entry), petLevel, uint8(petType))
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
				if s.player.PetGUID != 0 {
					_ = s.sendTalentsInfo(true)
				}
			}
		}
	}
	if s.server.Config.GameType == 16 && s.security == 0 && s.player.ExtraFlags&playerExtraGMOn == 0 && s.player.PlayerFlags&playerFlagGM == 0 && s.player.PlayerFlags&playerFlagResting == 0 {
		s.player.PVPFlags |= 0x04
		s.sendPlayerUpdate()
	}
	if s.player.PlayerFlags&playerFlagContestedPVP != 0 {
		s.contestedPVPEnd = time.Now().Add(30 * time.Second)
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
			if _, found, err := s.server.Data.Spell(spellID); err == nil && found {
				s.castFirstLoginSpell(ctx, spellID, s.playerGUID)
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
		s.buildPlayerRepop(ctx, false)
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
	// AutoBalance_PlayerScript::OnLogin (AutoBalance.cpp): announce the
	// module on every login, gated on AutoBalanceAnnounce.enable.
	if s.server.Config.AutoBalance.AnnounceEnable {
		message := protocol.BuildSystemChatMessage("This server is running the |cff4CFF00AutoBalance |rmodule.")
		if err := s.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), message, true); err != nil {
			return false
		}
	}
	s.debug("player login complete", "account", s.accountName, "guid", s.playerGUID, "map", state.Map, "x", state.X, "y", state.Y, "z", state.Z)
	// AutoBalance_AllMapScript::OnPlayerEnterAll (AutoBalance.cpp): the
	// login lands the player on the map; the matching leave fires in
	// removeSession on disconnect.
	s.server.autoBalancePlayerEnter(s, state.Map, state.InstanceID)
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
	wasSanctuary := oldFlags&0x08 != 0
	if sanctuary {
		state.PVPFlags |= 0x08
		if !wasSanctuary {
			s.stopPvPCombatForSanctuary()
		}
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
	resting := playerRestingInZoneArea(uint8(team), zone.Flags, area.Flags, hostile, sanctuary) || s.inTavernResting()
	s.setRestingFlag(state, resting)
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
	// Player::RemoveFromWorld (Player.cpp:1976-1979): a far teleport releases
	// the active loot, which also clears the UNIT_FLAG_LOOTING bit set at open.
	s.releaseActiveLoot()
	originOrientation := s.farTeleportOriginOrientation
	s.worldReady.Store(false)
	s.farTeleportPending = false
	state := *s.player
	if !s.validTrinityMapID(state.Map) || (!s.initialLoginPending && !validTrinityMapCoordinates(state.X, state.Y, state.Z, state.Orientation)) {
		return false
	}
	mapEntry, found, err := s.server.Data.Map(state.Map)
	if err != nil || !found {
		return false
	}
	fallbackHomebind := func() bool {
		if !s.validTrinityMapLocation(state.HomebindMap, state.HomebindX, state.HomebindY, state.HomebindZ, originOrientation) {
			return false
		}
		return s.teleportTo(state.HomebindMap, state.HomebindX, state.HomebindY, state.HomebindZ, originOrientation)
	}
	selection, canCreate, err := s.resolveWorldportInstance(ctx, state, mapEntry)
	if err != nil || !canCreate {
		return fallbackHomebind()
	}
	instanceCommitted := false
	if selection.Reserved {
		defer func() {
			if !instanceCommitted {
				s.server.releaseWorldportInstanceID(selection.InstanceID)
			}
		}()
	}
	state.InstanceID = selection.InstanceID
	if !s.worldportInstanceHasRoom(selection, mapEntry) {
		return fallbackHomebind()
	}
	if !s.initialLoginPending && mapEntry.IsRaid() && (s.server.instanceEncounterInProgress(state.Map, selection.InstanceID) || s.instanceEncounterHooked(ctx, state.Map, selection.InstanceID)) {
		return fallbackHomebind()
	}
	admissionReserved := mapEntry.IsDungeon() && !s.isMapAdmissionGM()
	admissionCommitted := false
	if admissionReserved {
		defer func() {
			if !admissionCommitted {
				s.server.releaseWorldportAdmission(instanceAdmissionKey{MapID: mapEntry.ID, InstanceID: selection.InstanceID})
			}
		}()
	}
	s.player.InstanceID = selection.InstanceID
	if err := s.sendInitialPacketsBeforeAddToMap(ctx, state); err != nil {
		return false
	}
	attachedTransport, err := s.server.buildAttachedTransportUpdate(ctx, state)
	if err != nil {
		return false
	}
	attachedPassengers, err := s.server.buildAttachedTransportPassengerUpdates(ctx, state, s)
	if err != nil {
		return false
	}
	attachedPlayers, attachedPlayerGUIDs, err := s.server.buildAttachedTransportPlayerUpdates(state, s.playerGUID, s)
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
	if selection.CreateSave || selection.BindPlayer || selection.BindGroup || selection.UnbindPlayerInstanceID != 0 {
		if err := s.persistWorldportInstance(ctx, selection); err != nil {
			return false
		}
		instanceCommitted = true
		if selection.Reserved {
			s.server.releaseWorldportInstanceID(selection.InstanceID)
		}
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
	s.commitWorldportAdmission(selection, admissionReserved)
	admissionCommitted = true
	s.sendWorldportGroupLockWarning(ctx, selection)
	nearbyPlayers, _, nearbyPlayerGUIDs := s.server.buildNearbyPlayerUpdatesWithCreated(s)
	nearbyCreatures, _, creatureErr := s.server.buildNearbyCreatureUpdates(ctx, state, s.currentPlayerPhaseMask(), s)
	if creatureErr != nil {
		return false
	}
	nearbyGameObjects, _, gameObjectErr := s.server.buildNearbyGameObjectUpdates(ctx, state, false, s)
	if gameObjectErr != nil {
		return false
	}
	nearbyCorpses, _, corpseErr := s.server.buildNearbyCorpseUpdates(ctx, state, s.currentPlayerPhaseMask())
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
	s.sendVisiblePlayerAuras(nearbyPlayerGUIDs)
	s.sendVisibleCreatureAuras(state)
	s.triggerPlayerEvent(ctx, scripting.PlayerEventMapChange, s.luaPlayer())
	s.triggerMapEntryEvent(ctx)
	s.fireInstancePlayerEnter(ctx)
	s.streamDynamicSpellObjects()
	// WorldSession::HandleMoveWorldportAck mount-allow leg (MovementHandler.cpp):
	// allowMount = !IsDungeon() || IsBattlegroundOrArena(), overridden by the
	// instance_template.allowMount row when present; a worldport into a
	// disallowing dungeon strips SPELL_AURA_MOUNTED. Same computation as
	// checkMountedCast (spells.go); removeAura drives the wasMounted dismount
	// broadcast (the C++ Dismount tail). No mount leg exists in the near
	// teleport ack (same map, mountability cannot change).
	allowMount := true
	if entry, found, err := s.server.Data.Map(s.player.Map); err == nil && found {
		allowMount = !entry.IsDungeon() || teleIsBattlegroundOrArena(entry)
	}
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var dbAllow int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT allowMount FROM instance_template WHERE map = ?", s.player.Map).Scan(&dbAllow); err == nil {
			allowMount = dbAllow != 0
		}
	}
	if !allowMount {
		for _, aura := range s.loadedAuras() {
			if aura != nil && aura.AuraType == spellAuraMounted {
				s.removeAura(aura.SpellID)
			}
		}
	}
	s.updateZoneAndArea(ctx, true)
	// MovementHandler.cpp:186-196 (HandleMoveWorldPortAck): a far teleport
	// into a hostile zone earns Honorless Target (2479, triggered) — unlike
	// the near-teleport arm there is no zone-change gate here, only
	// hostility. The UpdatePvP(false,false) arm is vacuous in Go (see the
	// near-teleport note).
	if s.pvpHostile {
		s.castSpellDirect(ctx, 2479, s.playerGUID)
	}
	s.recordInstanceEnterTime(ctx, time.Now())
	s.resetTimeSync()
	if err := s.write(uint16(protocol.OpcodeSMSG_TIME_SYNC_REQ), buildTimeSyncRequest(0), true); err != nil {
		return false
	}
	s.recordTimeSyncSent(0)
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
	if err := s.sendLoginMovementStunAndCompoundStates(); err != nil {
		return false
	}
	s.sendLoadedAuras()
	s.sendLoginCharmControl()
	s.sendPlayerCollisionHeight()
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
	if err := s.sendSharedQuestDetails(ctx); err != nil {
		return false
	}
	s.resummonTemporaryPet(ctx)
	s.lastStreamX, s.lastStreamY, s.lastStreamZ = s.player.X, s.player.Y, s.player.Z
	s.farTeleportPending = false
	s.initialLoginPending = false
	// AutoBalance_AllMapScript::OnPlayerEnterAll/OnPlayerLeaveAll
	// (AutoBalance.cpp): the completed far teleport leaves the origin
	// instance and enters the new one.
	s.server.autoBalanceTransferInstance(s, s.farTeleportOriginMap, s.farTeleportOriginInstanceID, s.player.Map, s.player.InstanceID)
	s.farTeleportOriginMap, s.farTeleportOriginInstanceID = 0, 0
	return true
}

func (s *session) sendSharedQuestDetails(ctx context.Context) error {
	if s == nil || s.sharingQuestID == 0 {
		return nil
	}
	quest, err := s.loadQuestDetailData(ctx, s.sharingQuestID)
	if errors.Is(err, sql.ErrNoRows) {
		s.sharingQuestID, s.sharingQuestSender = 0, 0
		return nil
	}
	if err != nil {
		return err
	}
	return s.write(uint16(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS), buildQuestGiverDetails(quest, s.playerGUID, s.sharingQuestSender), true)
}

func (s *session) sendInitialPacketsBeforeAddToMap(ctx context.Context, state playerState) error {
	if err := s.sendContactList(ctx, uint32(socialFlagFriend|socialFlagIgnored|socialFlagMuted)); err != nil {
		return err
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_BIND_POINT_UPDATE), buildBindPointUpdate(&state), true); err != nil {
		return err
	}
	if err := s.sendTalentsInfo(false); err != nil {
		return err
	}
	instanceDifficulty, dynamicDifficulty := s.loginInstanceDifficulty(ctx, state)
	if err := s.write(uint16(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY), buildInstanceDifficultyForMap(instanceDifficulty, dynamicDifficulty), true); err != nil {
		return err
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_INITIAL_SPELLS), buildInitialSpells(state), true); err != nil {
		return err
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS), s.buildUnlearnSpells(ctx, state), true); err != nil {
		return err
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_ACTION_BUTTONS), buildActionButtons(state.Actions), true); err != nil {
		return err
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_INITIALIZE_FACTIONS), buildInitialReputations(state), true); err != nil {
		return err
	}
	s.sendAllAchievementData()
	s.debug("world login stage", "stage", "equipment-set-list-start", "guid", s.playerGUID)
	s.sendEquipmentSetList(ctx)
	s.debug("world login stage", "stage", "equipment-set-list-complete", "guid", s.playerGUID)
	if err := s.write(uint16(protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED), buildLoginSetTimeSpeed(time.Now()), true); err != nil {
		return err
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_SET_FORCED_REACTIONS), buildForcedReactions(s.loadedAuras()), true); err != nil {
		return err
	}
	return s.sendResyncRunes()
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
			if !s.loginAuraHasEffectType(aura, auraType) {
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

func (s *session) loginAuraHasEffectType(aura *activeAura, auraType uint32) bool {
	if aura == nil {
		return false
	}
	if aura.AuraType == auraType {
		return true
	}
	if s == nil || s.server == nil || s.server.Data == nil || aura.EffectMask == 0 {
		return false
	}
	spell, found, err := s.server.Data.Spell(aura.SpellID)
	if err != nil || !found {
		return false
	}
	for index, effect := range spell.Effects {
		if aura.EffectMask&(1<<uint(index)) != 0 && effect.Effect != 0 && effect.Aura == auraType {
			return true
		}
	}
	return false
}

func (s *session) sendLoginCharmControl() {
	if s == nil || s.player == nil || !s.hasAuraType(spellAuraCharm) {
		return
	}
	s.sendClientControl(s.playerGUID, false)
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
		rooted = rooted || s.loginAuraHasEffectType(aura, auraRoot)
		stunned = stunned || s.loginAuraHasEffectType(aura, auraStun)
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
			if !s.loginAuraHasEffectType(aura, auraType) {
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
	if !s.hasActiveFlightCapability() {
		return nil
	}
	packet := protocol.NewBuffer(packedGUIDSize(s.playerGUID) + 4)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(0)
	if err := s.write(uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY), packet.Bytes(), true); err != nil {
		return err
	}
	s.broadcastLoginMovementState(protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY, 0x01000000)
	s.sendRuntimeMovementSpeed(protocol.OpcodeSMSG_FORCE_FLIGHT_SPEED_CHANGE, protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED, s.mountedFlightSpeed(), false)
	return nil
}

func (s *session) broadcastLoginMovementState(opcode protocol.Opcode, movementFlags uint32) {
	if s == nil || s.server == nil || s.player == nil || opcode == 0 {
		return
	}
	info := s.movementInfoForCreate(*s.player)
	var stateFlag uint32
	switch opcode {
	case protocol.OpcodeMSG_MOVE_WATER_WALK:
		stateFlag = 0x10000000
	case protocol.OpcodeMSG_MOVE_FEATHER_FALL:
		stateFlag = 0x20000000
	case protocol.OpcodeMSG_MOVE_HOVER:
		stateFlag = 0x40000000
	case protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY:
		stateFlag = 0x01000000
	}
	if stateFlag == 0 {
		info.Flags = movementFlags
	} else {
		info.Flags = info.Flags&^stateFlag | movementFlags&stateFlag
	}
	if s.player.TransportGUID != 0 {
		info.Flags |= movementOnTransport
	}
	info.Time = uint32(time.Now().UnixMilli())
	s.setLastMovementInfo(info)
	packet := protocol.NewBuffer(112)
	writeMovementInfo(packet, info)
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
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	spell, found, err := s.server.Data.Spell(836)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: s.playerGUID}
	target = spellGoPacketTarget(spell, target)
	castFlags := spellCastFlagGo | spellCastFlagPending | protocol.SpellCastFlagPowerLeftSelf
	if spell.StartRecoveryTime == 0 {
		castFlags |= protocol.SpellCastFlagNoGCD
	}
	if spell.PowerType == 0xFFFFFFFE {
		castFlags &^= protocol.SpellCastFlagPowerLeftSelf
	}
	castTime := gameTimeMS()
	var power uint32
	var remainingPower *uint32
	if castFlags&protocol.SpellCastFlagPowerLeftSelf != 0 {
		if spell.PowerType >= uint32(len(s.player.Powers)) {
			return fmt.Errorf("login effect 836 power type %d is out of range", spell.PowerType)
		}
		power = s.player.Powers[spell.PowerType]
		remainingPower = &power
	}
	packet := protocol.BuildSpellGoWithPower(s.playerGUID, s.playerGUID, 0, 836, castFlags, castTime, []uint64{s.playerGUID}, nil, target, remainingPower)
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), packet, true); err != nil {
		return err
	}
	nearbyFlags := castFlags &^ protocol.SpellCastFlagPowerLeftSelf
	nearby := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, 0, 836, nearbyFlags, castTime, []uint64{s.playerGUID}, nil, target)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearby, s)
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
	return true
}

func (s *session) handleOpeningCinematic() bool {
	// Reference: WorldSession::HandleOpeningCinematic (CharacterHandler.cpp:2223):
	// only players that have not yet gained any experience can use this
	// (PLAYER_XP must be 0); the Go Cinematic flag additionally prevents
	// repeats after the first play.
	if !s.playerLoaded || s.player == nil || s.player.Cinematic != 0 || s.player.XP != 0 {
		return true
	}
	s.player.Cinematic = 1
	cinematicID := s.getStartingCinematicID(s.player.Race, s.player.Class)
	if cinematicID == 0 {
		return true
	}
	packet := protocol.NewBuffer(4)
	packet.WriteU32(cinematicID)
	return s.write(uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC), packet.Bytes(), true) == nil
}

func (s *session) handleCompleteCinematic(ctx context.Context) bool {
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
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, "SELECT spell FROM character_spell WHERE guid = ? ORDER BY spell", guid)
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

		nextItemGUID := uint64(s.server.generateItemGUID())
		if nextItemGUID == 0 {
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
	cacheStr := strings.Join(parts, " ") + " "
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

func (s *session) createStarterSkills(ctx context.Context, guid uint64, race, class, level uint8) {
	cdb := s.server.CharactersStore.DB
	wdb := s.server.WorldStore.DB
	if cdb == nil || wdb == nil {
		return
	}
	current := make(map[uint16]playerSkill)
	for _, skill := range defaultRacialSkills(race, class) {
		current[skill.Skill] = skill
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
	rows, err := wdb.QueryContext(ctx, "SELECT skill, rank FROM playercreateinfo_skills WHERE (raceMask = 0 OR (raceMask & ?) <> 0) AND (classMask = 0 OR (classMask & ?) <> 0) ORDER BY skill", raceMask, classMask)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var skillID, rank int64
			if rows.Scan(&skillID, &rank) != nil || skillID <= 0 || skillID > 65535 || rank < 0 || rank > 65535 || !s.skillAllowed(race, class, uint16(skillID)) {
				continue
			}
			if _, found := current[uint16(skillID)]; found {
				continue
			}
			skill, valid := s.defaultPlayerSkill(race, class, uint16(skillID), uint16(rank), level, current)
			if !valid {
				continue
			}
			_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_skills (guid, skill, value, max) VALUES (?, ?, ?, ?)", guid, skillID, skill.Value, skill.Max)
			current[skill.Skill] = skill
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
// Reference: WorldSession::HandleCharRenameOpcode/HandleCharRenameCallBack
// (CharacterHandler.cpp:1111-1196), WorldSession::SendCharRename (:2159).
func (s *session) handleCharRename(ctx context.Context, payload []byte) bool {
	if len(payload) < 9 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, err := r.ReadU64()
	if err != nil {
		return false
	}
	rawName, err := r.ReadCString()
	if err != nil {
		return false
	}
	// SendCharRename appends guid+name only on success; failures send the u8 code alone.
	// The name echoed back is the normalized one C++ stored (renameInfo->Name),
	// not the raw packet string (:2159-2165).
	sendRename := func(code uint8, success bool, name string) {
		buf := protocol.NewBuffer(1 + 8 + len(name) + 1)
		buf.WriteU8(code)
		if success {
			buf.WriteU64(guid)
			buf.WriteCString(name)
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_RENAME), buf.Bytes(), true)
	}
	if !utf8.ValidString(rawName) {
		sendRename(charNameNoName, false, "")
		return true
	}
	newName := normalizePlayerName(rawName)
	if newName == "" {
		sendRename(charNameNoName, false, "")
		return true
	}
	// ObjectMgr::CheckPlayerName approximation (validCharacterName covers the
	// length/alphabet arms).
	if !validCharacterName(newName) {
		sendRename(charNameFailure, false, "")
		return true
	}
	store := s.server.CharactersStore
	if store == nil || store.DB == nil {
		sendRename(charCreateError, false, "")
		return true
	}
	// IsReservedName arm (CharacterHandler.cpp:1133): gated on
	// RBAC_PERM_SKIP_CHECK_CHARACTER_CREATION_RESERVEDNAME; answers
	// CHAR_NAME_RESERVED (95) ahead of the account/at-login/free-name check,
	// == C++ order (reserved :1133, CHAR_SEL_FREE_NAME :1141).
	if !s.skipReservedNameCheck {
		var reserved int
		if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM reserved_name WHERE name = ?", strings.ToLower(newName)).Scan(&reserved); err == nil && reserved > 0 {
			sendRename(charNameReserved, false, "")
			return true
		}
	}
	// The character must belong to this account, carry AT_LOGIN_RENAME, and
	// the new name must be free (CHAR_SEL_FREE_NAME, :1141).
	var atLogin uint64
	if err := store.DB.QueryRowContext(ctx, "SELECT at_login FROM characters WHERE guid = ? AND account = ?", guid, s.accountID).Scan(&atLogin); err != nil {
		sendRename(charCreateError, false, "")
		return true
	}
	if atLogin&atLoginRename == 0 {
		sendRename(charCreateError, false, "")
		return true
	}
	if row, err := store.QueryRowStatement(ctx, database.StatementID("CHAR_SEL_CHECK_NAME"), newName); err == nil {
		var one int
		if row.Scan(&one) == nil {
			sendRename(charCreateNameInUse, false, "")
			return true
		}
	}
	_, _ = store.ExecStatement(ctx, database.StatementID("CHAR_UPD_CHAR_NAME_AT_LOGIN"), newName, atLogin&^atLoginRename, guid)
	_, _ = store.ExecStatement(ctx, database.StatementID("CHAR_DEL_DECLINED_NAME"), guid)
	sendRename(0, true, newName)
	s.debug("character renamed", "guid", guid, "name", newName)
	return true
}

// handleCharCustomize processes CMSG_CHAR_CUSTOMIZE (0x473).
// Reference: WorldSession::HandleCharCustomize/HandleCharCustomizeCallback
// (CharacterHandler.cpp:1363-1490), WorldSession::SendCharCustomize (:2171).
// Wire order (handler reads, :1376-1383): guid, name, gender, skin,
// hairColor, hairStyle, facialHair, face — the Go parse previously read
// face/hairStyle/hairColor/facialHair scrambled.
func (s *session) handleCharCustomize(ctx context.Context, payload []byte) bool {
	if len(payload) < 15 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	// Customizing another account's character is a cheat: C++ kicks the
	// session (CharacterHandler.cpp:1368-1374).
	store := s.server.CharactersStore
	if store == nil || store.DB == nil {
		return true
	}
	var legit int
	if err := store.DB.QueryRowContext(ctx, "SELECT 1 FROM characters WHERE guid = ? AND account = ?", guid, s.accountID).Scan(&legit); err != nil {
		s.debug("customize cheat attempt", "guid", guid)
		s.kickSession(s)
		return true
	}
	rawName, _ := r.ReadCString()
	gender, _ := r.ReadU8()
	skin, _ := r.ReadU8()
	hairColor, _ := r.ReadU8()
	hairStyle, _ := r.ReadU8()
	facialHair, _ := r.ReadU8()
	face, _ := r.ReadU8()
	// SendCharCustomize appends the appearance fields only on success.
	sendCustomize := func(code uint8, success bool) {
		buf := protocol.NewBuffer(16 + len(rawName))
		buf.WriteU8(code)
		if success {
			buf.WriteU64(guid)
			buf.WriteCString(rawName)
			buf.WriteU8(gender)
			buf.WriteU8(skin)
			buf.WriteU8(face)
			buf.WriteU8(hairStyle)
			buf.WriteU8(hairColor)
			buf.WriteU8(facialHair)
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_CUSTOMIZE), buf.Bytes(), true)
	}
	var oldName string
	var race, class, oldGender uint8
	var atLogin uint64
	row, err := store.QueryRowStatement(ctx, database.StatementID("CHAR_SEL_CHAR_CUSTOMIZE_INFO"), guid)
	if err != nil || row.Scan(&oldName, &race, &class, &oldGender, &atLogin) != nil {
		sendCustomize(charCreateError, false)
		return true
	}
	// Player::ValidateAppearance (:1413) — unknown combos are a bridge gap
	// (ValidateAppearance returns known=false without DBC data); treat a
	// known-invalid combo as CHAR_CREATE_ERROR.
	if valid, known, aerr := s.server.Data.ValidateAppearance(race, class, oldGender, hairStyle, hairColor, face, facialHair, skin); aerr == nil && known && !valid {
		sendCustomize(charCreateError, false)
		return true
	}
	if atLogin&atLoginCustomize == 0 {
		sendCustomize(charCreateError, false)
		return true
	}
	if !utf8.ValidString(rawName) || normalizePlayerName(rawName) == "" {
		sendCustomize(charNameNoName, false)
		return true
	}
	newName := normalizePlayerName(rawName)
	if !validCharacterName(newName) {
		sendCustomize(charNameFailure, false)
		return true
	}
	// ObjectMgr::IsReservedName (ObjectMgr.cpp:8515-8524): lowercased exact
	// match against the reserved_name rows C++ loads from the characters DB
	// (ObjectMgr.cpp:8482), gated on
	// RBAC_PERM_SKIP_CHECK_CHARACTER_CREATION_RESERVEDNAME
	// (CharacterHandler.cpp:1445-1448). C++ answers CHAR_NAME_RESERVED (95)
	// on a match; the gate sits between the name-validity check and the
	// name-in-use check in both trees.
	if !s.skipReservedNameCheck {
		var reserved int
		if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM reserved_name WHERE name = ?", strings.ToLower(newName)).Scan(&reserved); err == nil && reserved > 0 {
			sendCustomize(charNameReserved, false)
			return true
		}
	}
	if nrow, nerr := store.QueryRowStatement(ctx, database.StatementID("CHAR_SEL_CHECK_NAME"), newName); nerr == nil {
		var one int
		if nrow.Scan(&one) == nil && newName != oldName {
			sendCustomize(charCreateNameInUse, false)
			return true
		}
	}
	// Player::Customize + name/at_login update (:1451-1470).
	_, _ = store.DB.ExecContext(ctx, "UPDATE characters SET name = ?, gender = ?, skin = ?, face = ?, hairStyle = ?, hairColor = ?, facialStyle = ?, at_login = ? WHERE guid = ?",
		newName, gender, skin, face, hairStyle, hairColor, facialHair, atLogin&^atLoginCustomize, guid)
	_, _ = store.ExecStatement(ctx, database.StatementID("CHAR_DEL_DECLINED_NAME"), guid)
	sendCustomize(0, true)
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
// Wire order (:1635-1642): guid, name, gender, skin, hairColor, hairStyle,
// facialHair, face, race — the Go parse previously read face/hair fields scrambled.
func (s *session) handleCharRaceChange(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	newName, _ := r.ReadCString()
	gender, _ := r.ReadU8()
	skin, _ := r.ReadU8()
	hairColor, _ := r.ReadU8()
	hairStyle, _ := r.ReadU8()
	facialHair, _ := r.ReadU8()
	face, _ := r.ReadU8()
	race, _ := r.ReadU8()

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		var atLogin uint64
		var oldRace, class uint8
		if err := cdb.QueryRowContext(ctx, "SELECT at_login, race, class FROM characters WHERE guid = ? AND account = ?", guid, s.accountID).Scan(&atLogin, &oldRace, &class); err != nil {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(charCreateError)
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}
		// usedLoginFlag = AT_LOGIN_CHANGE_RACE (CharacterHandler.cpp:1681-1685).
		if atLogin&atLoginChangeRace == 0 {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(charCreateError)
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}
		// sObjectMgr->GetPlayerInfo(race, class): race must be playable for the
		// class combination (the class arm has no Go DBC bridge).
		if playable, _ := s.server.raceDefinition(race); !playable {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(charCreateError)
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}
		// Race change must stay on the same faction team (CharacterHandler.cpp:1687)
		if oldRace != 0 && teamForRace(oldRace) != teamForRace(race) {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(charCreateCharacterRaceOnly)
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
// Wire order (:1635-1642): guid, name, gender, skin, hairColor, hairStyle,
// facialHair, face, race — the Go parse previously read face/hair fields scrambled.
func (s *session) handleCharFactionChange(ctx context.Context, payload []byte) bool {
	if len(payload) < 16 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	newName, _ := r.ReadCString()
	gender, _ := r.ReadU8()
	skin, _ := r.ReadU8()
	hairColor, _ := r.ReadU8()
	hairStyle, _ := r.ReadU8()
	facialHair, _ := r.ReadU8()
	face, _ := r.ReadU8()
	race, _ := r.ReadU8()

	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		var atLogin uint64
		var oldRace uint8
		if err := cdb.QueryRowContext(ctx, "SELECT at_login, race FROM characters WHERE guid = ? AND account = ?", guid, s.accountID).Scan(&atLogin, &oldRace); err != nil {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(charCreateError)
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}
		// Failures answer the u8 code alone; success appends guid/name/appearance
		// (SendCharFactionChange, CharacterHandler.cpp:2189-2206).
		sendFactionChange := func(code uint8) bool {
			buf := protocol.NewBuffer(1)
			buf.WriteU8(code)
			_ = s.write(uint16(protocol.OpcodeSMSG_CHAR_FACTION_CHANGE), buf.Bytes(), true)
			return true
		}
		// usedLoginFlag = AT_LOGIN_CHANGE_FACTION (CharacterHandler.cpp:1681-1685).
		if atLogin&atLoginChangeFaction == 0 {
			return sendFactionChange(charCreateError)
		}
		newTeam := teamForRace(race)
		// Faction change must swap to the opposite faction team
		// (CharacterHandler.cpp:1687) — ahead of the name arms, == C++ order.
		if oldRace != 0 && teamForRace(oldRace) == newTeam {
			return sendFactionChange(charCreateCharacterSwapFaction)
		}
		// CONFIG_CHARACTER_CREATING_DISABLED_RACEMASK
		// (CharacterHandler.cpp:1695-1702): mirrors the char-create mask check.
		if s.server.Config.CharacterCreatingDisabledRaceMask&(uint32(1)<<(race-1)) != 0 {
			return sendFactionChange(charCreateError)
		}
		// normalizePlayerName (CharacterHandler.cpp:1709-1713): empty or
		// invalid-UTF-8 answers CHAR_NAME_NO_NAME.
		if !utf8.ValidString(newName) {
			return sendFactionChange(charNameNoName)
		}
		newName = normalizePlayerName(newName)
		if newName == "" {
			return sendFactionChange(charNameNoName)
		}
		// ObjectMgr::CheckPlayerName approximation
		// (CharacterHandler.cpp:1715-1719): validCharacterName covers the
		// length/alphabet arms.
		if !validCharacterName(newName) {
			return sendFactionChange(charNameFailure)
		}
		// IsReservedName arm (CharacterHandler.cpp:1722-1726): gated on
		// RBAC_PERM_SKIP_CHECK_CHARACTER_CREATION_RESERVEDNAME; answers
		// CHAR_NAME_RESERVED (95) on the normalized name == C++ order.
		if !s.skipReservedNameCheck {
			var reserved int
			if err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM reserved_name WHERE name = ?", strings.ToLower(newName)).Scan(&reserved); err == nil && reserved > 0 {
				return sendFactionChange(charNameReserved)
			}
		}
		// Name already taken by another character
		// (CharacterHandler.cpp:1729-1736, CHAR_CREATE_NAME_IN_USE).
		var takenBy uint64
		if err := cdb.QueryRowContext(ctx, "SELECT guid FROM characters WHERE name = ?", newName).Scan(&takenBy); err == nil && takenBy != guid {
			return sendFactionChange(charCreateNameInUse)
		}
		// Arena team captain cannot faction change
		// (CharacterHandler.cpp:1738-1742, CHAR_CREATE_CHARACTER_ARENA_LEADER).
		var isCaptain int
		if err := cdb.QueryRowContext(ctx, "SELECT 1 FROM arena_team WHERE captainGuid = ?", uint32(guid)).Scan(&isCaptain); err == nil {
			return sendFactionChange(charCreateCharacterArenaLeader)
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
	if s.player == nil || s.player.Movie == 0 {
		return true
	}
	s.player.Movie = 0
	// No OnMovieComplete consumer: no script implements it in the C++ tree, so the
	// sScriptMgr->OnMovieComplete arm (MiscHandler.cpp:969-975) has no bridge target.
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
	unitFlagSilenced  = uint32(0x00002000)
	unitFlagPacified  = uint32(0x00020000)
	unitFlagStunned   = uint32(0x00040000)
	unitFlagConfused  = uint32(0x00400000)
	unitFlagFleeing   = uint32(0x00800000)
	unitFlagDisarmed  = uint32(0x00200000) // UNIT_FLAG_DISARMED (UnitDefines.h:145)
)

func (s *session) setRooted(root bool) {
	if s.player == nil {
		return
	}
	s.rooted = root
	buf := protocol.NewBuffer(16)
	buf.WritePackedGUID(s.playerGUID)
	buf.WriteU32(0) // movement counter
	if root {
		_ = s.write(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), buf.Bytes(), true)
	} else {
		_ = s.write(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT), buf.Bytes(), true)
	}
}

func (s *session) canFreeMoveForLogout(ignoreLogoutLock bool) bool {
	if s == nil || s.player == nil || s.inFlight || s.player.VehicleGUID != 0 || s.player.UnitFlags&(unitFlagConfused|unitFlagFleeing) != 0 {
		return false
	}
	if ignoreLogoutLock {
		return !s.hasAuraType(spellAuraRoot) && !s.hasAuraType(spellAuraStun)
	}
	return !s.rooted && s.player.UnitFlags&unitFlagStunned == 0
}

func playerLogoutDecision(inCombat, resting, inFlight, instantPermission, falling, dueling bool) (uint32, bool) {
	instant := (resting && !inCombat) || inFlight || instantPermission
	reason := uint32(0)
	if inCombat && !resting {
		reason = 1
	} else if falling {
		reason = 3
	} else if dueling {
		reason = 2
	}
	return reason, instant
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
	reason, instant := playerLogoutDecision(inCombat, resting, s.inFlight, instantLogoutPermission, s.isFalling, s.duelPartner != 0 || s.hasAura(9454))
	if reason != 0 {
		s.logoutAt = time.Time{}
		response := protocol.NewBuffer(5)
		response.WriteU32(reason)
		if instant {
			response.WriteU8(1)
		} else {
			response.WriteU8(0)
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE), response.Bytes(), true)
		return true
	}
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
	if s.canFreeMoveForLogout(s.logoutFlagsApplied) {
		if s.player.StandState == 0 { // UNIT_STAND_STATE_STAND
			s.player.StandState = 1 // UNIT_STAND_STATE_SIT
		}
		s.player.UnitFlags |= unitFlagStunned
		s.setRooted(true)
		s.logoutFlagsApplied = true
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
	ackErr := s.write(uint16(protocol.OpcodeSMSG_LOGOUT_CANCEL_ACK), nil, true)

	// Reference MiscHandler.cpp:471-483:
	// SetRooted(false), SetStandState(UNIT_STAND_STATE_STAND), RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
	if s.canFreeMoveForLogout(s.logoutFlagsApplied) {
		s.player.StandState = 0 // UNIT_STAND_STATE_STAND
		s.player.UnitFlags &^= unitFlagStunned
		s.setRooted(false)
		s.sendPlayerUpdate()
	}
	s.logoutFlagsApplied = false

	s.debug("player logout cancelled", "account", s.accountName)
	return ackErr == nil
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
	return s.completeLogoutWithPacket(ctx, true)
}

func (s *session) completeLogoutWithPacket(ctx context.Context, sendLogoutComplete bool) error {
	if !s.playerLoaded {
		s.clearActiveAuras()
		s.logoutAt = time.Time{}
		s.logoutFlagsApplied = false
		if s.accountID != 0 && !s.superseded && s.server != nil && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
			_, err := s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account SET online = 0 WHERE id = ?", s.accountID)
			return err
		}
		return nil
	}
	s.worldReady.Store(false)
	s.stopSpellLifecycle()
	s.stopTimedAchievements()
	s.stopPlayerCombat()
	s.broadcastGuildMemberLogout()
	s.triggerLogout(ctx)
	s.releaseActiveLoot()
	if s.player != nil && s.player.Health == 0 && s.player.PlayerFlags&playerFlagGhost == 0 && !s.deathTimer.IsZero() {
		// WorldSession::LogoutPlayer sets m_playerLogout before the repop
		// (WorldSession.cpp:502), so WorldSession::isLogingOut() holds here and
		// the MOVE_UNROOT leg is skipped (Player.cpp:4662-4663).
		s.buildPlayerRepop(ctx, true)
		s.repopAtGraveyard(ctx)
	}
	if s.playerLoaded && s.player != nil {
		s.handleLeaveBattlefield(ctx, nil)
	}
	if s.trade != nil {
		// Reference: Player::CleanupsBeforeDelete (Player.cpp:471) -> TradeCancel(false):
		// the logging-out player is NOT answered (sendback=false) — only the
		// trader gets TRADE_STATUS_TRADE_CANCELED. Using handleCancelTrade here
		// (sendback=true) would also send the packet to the disconnecting client.
		s.cancelTrade(false)
	}
	if s.server != nil {
		s.server.removeSessionFromGroup(s)
		s.server.removeSessionChannels(s)
		s.server.broadcastFriendStatus(s.playerGUID, friendsResultOffline, 0, 0, 0)
	}
	if s.player != nil && s.player.PetGUID != 0 {
		s.unsummonPet(ctx, petSaveAsCurrent)
	}
	var firstErr error
	if err := s.savePlayerState(ctx, 0, true); err != nil {
		firstErr = err
		_ = s.savePlayerPosition(ctx)
	}
	s.clearActiveAuras()
	if err := s.clearBuybackState(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if !s.superseded {
		if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_ACCOUNT_ONLINE", s.accountID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if !sendLogoutComplete && !s.superseded {
		if _, err := s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account SET online = 0 WHERE id = ?", s.accountID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if sendLogoutComplete {
		if err := s.write(uint16(protocol.OpcodeSMSG_LOGOUT_COMPLETE), nil, true); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.playerLoaded = false
	s.player = nil
	s.logoutAt = time.Time{}
	s.logoutFlagsApplied = false
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

// lootReleaseCleanupAllowed mirrors the gate ahead of the fully-looted
// legs of WorldSession::DoLootRelease's creature arm (LootHandler.cpp:349-352):
// the cleanup only runs when the creature still exists on the map
// (lootAllowed — a despawned creature's map lookup fails) and is within
// INTERACTION_DISTANCE of the player. A release after a far teleport or a
// despawn silently skips the dynflag/loot-clear legs; the release legs
// (loot-GUID clear, release response, UNIT_FLAG_LOOTING removal) already ran
// unconditionally above. Gameobject loot no longer uses this gate —
// handleLootRelease routes GO targets through releaseGameObjectLoot, which
// applies the DoLootRelease GameObject arm's own distance exemptions
// (owned GO / fishing hole).
func (s *session) lootReleaseCleanupAllowed(loot *activeLootState) bool {
	if s == nil || s.server == nil || loot == nil {
		return false
	}
	if high := uint16(loot.TargetGUID >> 48); high == 0xF110 {
		return true
	}
	guid := uint32(loot.TargetGUID & 0x00FFFFFF)
	entry := uint32((loot.TargetGUID >> 24) & 0x00FFFFFF)
	motion := s.server.findCreatureMotion(loot.MapID, loot.InstanceID, creatureWorldGUID(guid, entry))
	if motion == nil || s.player == nil {
		return false
	}
	// WorldSession::DoLootRelease creature arm (LootHandler.cpp:351-354): the
	// cleanup legs only run when the creature's alive state matches the loot
	// type — a pickpocket window released after the mark died, or a corpse
	// window on a creature that's alive again, skips the dynflag/loot-clear
	// legs. CLASS_ROGUE = 4 (SharedDefines.h); mirrors the take-time gate in
	// handleAutostoreLootItem (LootHandler.cpp:87-97).
	isRoguePickpocket := s.player.Class == 4 && loot.LootType == lootTypePickpocketing
	if (motion.Health != 0) != isRoguePickpocket {
		return false
	}
	return distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z) <= 5.0+1.5+1.5
}

func (s *session) releaseActiveLoot() {
	s.releaseActiveLootCleanup(s.lootReleaseCleanupAllowed(s.activeLoot))
}

// releaseActiveLootCleanup runs the unconditional release legs (loot-GUID
// clear, UNIT_FLAG_LOOTING removal, viewer removal, round-robin reset);
// cleanupAllowed gates only the fully-looted cleanup legs (dynflag clear,
// loot-state row delete), which the gameobject arm computes with its own
// distance gate instead of lootReleaseCleanupAllowed.
func (s *session) releaseActiveLootCleanup(cleanupAllowed bool) {
	// WorldSession::DoLootRelease (LootHandler.cpp:265): the UNIT_FLAG_LOOTING
	// bit set at loot open is removed on every release arm; clearing ahead of
	// the loot==nil return keeps a stuck flag from surviving any release path.
	if s.player != nil && s.player.UnitFlags&unitFlagLooting != 0 {
		s.player.UnitFlags &^= unitFlagLooting
		s.sendPlayerUpdate()
	}
	loot := s.activeLoot
	if loot == nil {
		return
	}
	if loot.RoundRobinPlayer == s.playerGUID {
		loot.RoundRobinPlayer = 0
	}
	loot.removeViewer(s.playerGUID)
	s.activeLoot = nil
	// WorldSession::DoLootRelease (LootHandler.cpp:349-352): the corpse is
	// only stripped when Loot::isLooted (Loot.h:236) — gold == 0 and
	// unlootedCount == 0, which counts the quest items (Loot.cpp:316) and
	// one entry per viewer per free-for-all row (Loot.cpp:271-293);
	// lootFullyLooted mirrors it.
	if lootFullyLooted(loot) && cleanupAllowed {
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
	x, y, z, orientation := s.player.X, s.player.Y, s.player.Z, s.player.Orientation
	if s.nearTeleportPending {
		x, y, z, orientation = s.nearTeleportDest.X, s.nearTeleportDest.Y, s.nearTeleportDest.Z, s.nearTeleportDest.Orientation
	}
	_, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_CHARACTER_POSITION", x, y, z, orientation, s.player.Map, s.player.Zone, s.player.GUID)
	return err
}

func (s *session) savePlayerState(ctx context.Context, online uint32, logout bool) error {
	if !s.playerLoaded || s.player == nil || s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	s.updatePlayedTime(time.Now())
	stateCopy := *s.player
	if s.nearTeleportPending {
		stateCopy.X, stateCopy.Y, stateCopy.Z, stateCopy.Orientation = s.nearTeleportDest.X, s.nearTeleportDest.Y, s.nearTeleportDest.Z, s.nearTeleportDest.Orientation
		if s.nearTeleportDest.Movement.Flags&movementOnTransport != 0 && s.nearTeleportDest.Movement.Transport != nil {
			stateCopy.TransportGUID = s.nearTeleportDest.Movement.Transport.GUID
			stateCopy.TransportX, stateCopy.TransportY, stateCopy.TransportZ, stateCopy.TransportO = s.nearTeleportDest.Movement.Transport.X, s.nearTeleportDest.Movement.Transport.Y, s.nearTeleportDest.Movement.Transport.Z, s.nearTeleportDest.Movement.Transport.Orientation
			stateCopy.TransportSeat = s.nearTeleportDest.Movement.Transport.Seat
		} else {
			stateCopy.TransportGUID = 0
			stateCopy.TransportX, stateCopy.TransportY, stateCopy.TransportZ, stateCopy.TransportO = 0, 0, 0, 0
			stateCopy.TransportSeat = 0
		}
	}
	state := &stateCopy
	state.LogoutResting = IsPlayerRestingForLogout(state.PlayerFlags)
	taxi := make([]string, len(state.TaxiMask))
	for i, value := range state.TaxiMask {
		taxi[i] = strconv.FormatUint(uint64(value), 10)
	}
	titles := make([]string, len(state.KnownTitles))
	for i, value := range state.KnownTitles {
		titles[i] = strconv.FormatUint(uint64(value), 10)
	}
	explored := serializeExploredZones(state.ExploredZones)
	transportLow := uint64(0)
	if state.TransportGUID != 0 {
		transportLow = state.TransportGUID & 0xFFFFFFFF
	}
	args := []any{state.Name, state.Race, state.Class, state.Gender, state.Level, state.XP, state.Money, state.Skin, state.Face, state.HairStyle, state.HairColor, state.FacialStyle, state.BankBagSlots, state.RestState, state.PlayerFlags, state.Map, state.InstanceID, state.InstanceModeMask, state.X, state.Y, state.Z, state.Orientation, state.TransportX, state.TransportY, state.TransportZ, state.TransportO, transportLow, strings.Join(taxi, " "), state.Cinematic, state.TotalPlayedTime, state.LevelPlayedTime, state.RestBonus, state.LogoutTime, state.LogoutResting, state.ResetTalentsCost, state.ResetTalentsTime, state.ExtraFlags, state.StableSlots, state.AtLogin, state.Zone, s.deathExpireTime, state.TaxiPath, state.ArenaPoints, state.TotalHonorPoints, state.TodayHonorPoints, state.YesterdayHonorPoints, state.TotalKills, state.TodayKills, state.YesterdayKills, state.ChosenTitle, state.KnownCurrency, state.WatchedFaction, state.DrunkenState, state.Health, state.Powers[0], state.Powers[1], state.Powers[2], state.Powers[3], state.Powers[4], state.Powers[5], state.Powers[6], s.latency.Load(), state.TalentGroupsCount, state.ActiveTalentGroup, explored, state.Equipment, state.AmmoID, strings.Join(titles, " ") + " ", state.ActionBars, state.GrantableLevels, online, state.GUID}
	tx, err := s.server.CharactersStore.Begin(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_UPD_CHARACTER", args...); err != nil {
		return err
	}
	if err = s.saveFishingSteps(ctx, tx, state); err != nil {
		return err
	}
	if err = s.savePlayerAuras(ctx, tx, state); err != nil {
		return err
	}
	if err = s.savePlayerSpellCooldowns(ctx, tx, state); err != nil {
		return err
	}
	if err = s.savePetState(ctx, tx); err != nil {
		return err
	}
	if err = s.saveBattlegroundData(ctx, tx, state); err != nil {
		return err
	}
	if err = s.saveInstanceTimeRestrictions(ctx, tx); err != nil {
		return err
	}
	if err = s.saveCharacterStats(ctx, tx, state, logout); err != nil {
		return err
	}
	return tx.Commit()
}

func IsPlayerRestingForLogout(playerFlags uint32) bool {
	return playerFlags&playerFlagResting != 0
}

func (s *session) updatePlayedTime(now time.Time) {
	if s == nil || s.player == nil || s.gameTimeStartedAt.IsZero() || !now.After(s.gameTimeStartedAt) {
		return
	}
	elapsed := uint32(now.Sub(s.gameTimeStartedAt) / time.Second)
	if elapsed == 0 {
		return
	}
	s.player.TotalPlayedTime += elapsed
	s.player.LevelPlayedTime += elapsed
	s.gameTimeStartedAt = s.gameTimeStartedAt.Add(time.Duration(elapsed) * time.Second)
}

func (s *session) saveFishingSteps(ctx context.Context, tx *sql.Tx, state *playerState) error {
	if s == nil || tx == nil || state == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	if state.FishingSteps == 0 {
		if _, err := tx.ExecContext(ctx, "DELETE FROM character_fishingsteps WHERE guid = ?", state.GUID); err != nil && !missingTable(err) {
			return err
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, "REPLACE INTO character_fishingsteps (guid, fishingSteps) VALUES (?, ?)", state.GUID, state.FishingSteps); err != nil && !missingTable(err) {
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
			[2]int32{1780, 1600}, // BG_AB_OP_RESOURCES_MAX
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
			[2]int32{1955, 1400}, // BG_AB_OP_RESOURCES_WARNING
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
	mapDifficulty, mapDifficultyFound := s.loadLoginInstanceDifficulty(ctx, state)
	if !s.raidMapDifficultyInitialized {
		if mapDifficultyFound && mapDifficulty < 4 {
			s.raidMapDifficulty = uint8(mapDifficulty)
		}
		s.raidMapDifficultyInitialized = true
	}
	stored := s.raidMapDifficulty
	if stored >= 4 {
		stored = 0
		s.raidMapDifficulty = 0
	}
	mapIsRaid := false
	if s != nil && s.server != nil && s.server.Data != nil {
		if entry, ok, err := s.server.Data.Map(state.Map); err == nil && ok {
			mapIsRaid = entry.IsRaid()
		}
	}
	forced := state.RaidDifficulty
	if mapIsRaid {
		if mapDifficultyFound {
			if mapDifficulty >= 4 {
				mapDifficulty = 0
			}
			if uint8(mapDifficulty) != state.RaidDifficulty {
				stored = uint8(mapDifficulty)
				s.raidMapDifficulty = stored
				forced = stored
			} else {
				return nil
			}
		} else {
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
// Reference: WorldSession::HandleSetPlayerDeclinedNames (CharacterHandler.cpp:1198),
// WorldSession::SendSetPlayerDeclinedNamesResult (:2208: u32 result + u64 guid),
// ObjectMgr::CheckDeclinedNames (ObjectMgr.cpp:8808), GetMainPartOfName
// (Util.cpp:466), isCyrillicCharacter (Util.h:141).
func (s *session) handleSetPlayerDeclinedNames(ctx context.Context, payload []byte) bool {
	if len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()

	// Wire order (CharacterHandler.cpp:1200-1235): guid, name, then the 5
	// declined cases (genitive, dative, accusative, instrumental, prepositional).
	name, _ := r.ReadCString()
	var declined [5]string
	for i := 0; i < 5; i++ {
		declined[i], _ = r.ReadCString()
	}

	result := uint32(1) // DECLINED_NAMES_RESULT_ERROR
	cdb := s.server.CharactersStore.DB
	if cdb != nil && guid > 0 {
		var charName string
		if err := cdb.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid).Scan(&charName); err == nil {
			if normalized, ok := declinedNamesAcceptable(charName, name, declined); ok {
				_, _ = cdb.ExecContext(ctx,
					`REPLACE INTO character_declinedname (guid, genitive, dative, accusative, instrumental, prepositional)
					 VALUES (?, ?, ?, ?, ?, ?)`,
					guid, normalized[0], normalized[1], normalized[2], normalized[3], normalized[4])
				result = 0 // DECLINED_NAMES_RESULT_SUCCESS
			}
		}
	}

	buf := protocol.NewBuffer(12)
	buf.WriteU32(result)
	buf.WriteU64(guid)
	_ = s.write(uint16(protocol.OpcodeSMSG_SET_PLAYER_DECLINED_NAMES_RESULT), buf.Bytes(), true)
	return true
}

// declinedNamesAcceptable mirrors the HandleSetPlayerDeclinedNames gates
// (CharacterHandler.cpp:1204-1247): declined names are accepted only for
// Cyrillic names, the packet name must equal the stored name, every case is
// normalized (empty after normalization fails), and the morphological
// consistency check (CheckDeclinedNames) must pass. The returned forms are the
// normalized ones C++ stores (declinedname.name[i] is normalized in place,
// :1228-1234).
func declinedNamesAcceptable(storedName, packetName string, declined [5]string) ([5]string, bool) {
	normalized := [5]string{}
	if !utf8.ValidString(storedName) {
		return normalized, false
	}
	first, _ := utf8.DecodeRuneInString(storedName)
	if !isCyrillicRune(first) {
		return normalized, false
	}
	if packetName != storedName {
		return normalized, false
	}
	for i := 0; i < 5; i++ {
		if !utf8.ValidString(declined[i]) {
			return normalized, false
		}
		normalized[i] = normalizePlayerName(declined[i])
		if normalized[i] == "" {
			return normalized, false
		}
	}
	if !checkDeclinedNames(storedName, normalized) {
		return normalized, false
	}
	return normalized, true
}

// isCyrillicRune mirrors isCyrillicCharacter (Util.h:141-148).
func isCyrillicRune(r rune) bool {
	return (r >= 0x0410 && r <= 0x044F) || r == 0x0401 || r == 0x0451
}

// declinedNameMainPart mirrors GetMainPartOfName (Util.cpp:466-513): supported
// only for Cyrillic names; strips the first matching declension ending of
// dropEnds[declension] (0 = nominative, 1-5 the declined cases).
func declinedNameMainPart(name []rune, declension int) []rune {
	if len(name) == 0 || !isCyrillicRune(name[0]) || declension < 0 || declension > 5 {
		return name
	}
	dropEnds := [6][][]rune{
		{{0x0430}, {0x043E}, {0x044F}, {0x0435}, {0x044C}, {0x0439}},
		{{0x0430}, {0x044F}, {0x044B}, {0x0438}},
		{{0x0435}, {0x0443}, {0x044E}, {0x0438}},
		{{0x0443}, {0x044E}, {0x043E}, {0x0435}, {0x044C}, {0x044F}, {0x0430}},
		{{0x043E, 0x0439}, {0x0451, 0x0439}, {0x0435, 0x0439}, {0x043E, 0x043C}, {0x0451, 0x043C}, {0x0435, 0x043C}, {0x044E}},
		{{0x0435}, {0x0438}},
	}
	for _, ending := range dropEnds[declension] {
		if len(ending) > len(name) {
			continue
		}
		tail := name[len(name)-len(ending):]
		match := true
		for i := range ending {
			if tail[i] != ending[i] {
				match = false
				break
			}
		}
		if match {
			return name[:len(name)-len(ending)]
		}
	}
	return name
}

// checkDeclinedNames mirrors ObjectMgr::CheckDeclinedNames (ObjectMgr.cpp:8808-8830):
// every declined case must share the nominative's main part (x), or every case
// must equal the base name (y).
func checkDeclinedNames(ownName string, declined [5]string) bool {
	own := []rune(ownName)
	mainPart := string(declinedNameMainPart(own, 0))
	x, y := true, true
	for i := 0; i < 5; i++ {
		if mainPart != string(declinedNameMainPart([]rune(declined[i]), i+1)) {
			x = false
		}
		if ownName != declined[i] {
			y = false
		}
	}
	return x || y
}
