package world

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	_ "modernc.org/sqlite"
)

func TestPlayerCreateMask(t *testing.T) {
	if playerCreateMask(3) != 0x04 || playerCreateMask(11) != 0x400 || playerCreateMask(0) != 0 {
		t.Fatalf("masks are incorrect")
	}
}

func TestRestorePlayerHealthPreservesPersistedLowHealth(t *testing.T) {
	if got := restorePlayerHealth(1, 500, true, 100, 20); got != 1 {
		t.Fatalf("persisted low health=%d", got)
	}
	if got := restorePlayerHealth(0, 500, true, 100, 20); got != 0 {
		t.Fatalf("persisted dead health=%d", got)
	}
	if got := restorePlayerHealth(1, 500, false, 100, 20); got != 500 {
		t.Fatalf("unloaded low health=%d", got)
	}
	if got := restorePlayerHealth(700, 500, true, 100, 20); got != 500 {
		t.Fatalf("clamped health=%d", got)
	}
}

func TestRestoreLoadedDeathStateReconstructsGhost(t *testing.T) {
	state := &playerState{Health: 0, HealthLoaded: true}
	restoreLoadedDeathState(state)
	if state.Health != 1 || state.PlayerFlags&playerFlagGhost == 0 || state.PlayerFieldBytes&playerFieldByteReleaseTimer == 0 {
		t.Fatalf("death state=%+v", state)
	}
	resurrected := &playerState{Health: 0, HealthLoaded: true, AtLogin: uint32(atLoginResurrect)}
	restoreLoadedDeathState(resurrected)
	if resurrected.Health != 0 || resurrected.PlayerFlags&playerFlagGhost != 0 {
		t.Fatalf("at-login resurrect state=%+v", resurrected)
	}
}

func TestRestoreLoadedCorpseStateUsesPersistedCorpse(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE corpse (guid INTEGER, corpseType INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO corpse VALUES (9, 1)"); err != nil {
		t.Fatal(err)
	}
	sess := &session{server: &Server{CharactersStore: &database.Store{DB: db}}}
	state := &playerState{GUID: 9, Health: 0}
	sess.restoreLoadedCorpseState(context.Background(), state)
	if state.Health != 1 || state.PlayerFlags&playerFlagGhost == 0 || state.PlayerFieldBytes&playerFieldByteReleaseTimer == 0 {
		t.Fatalf("corpse state=%+v", state)
	}
}

func TestBuildInitialReputations(t *testing.T) {
	payload := buildInitialReputations(playerState{Reputations: []playerReputation{{ListID: 72, Standing: 42999, Flags: 1}}})
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 128 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	for index := uint32(0); index < count; index++ {
		flags, flagsErr := reader.ReadU8()
		standing, standingErr := reader.ReadU32()
		if flagsErr != nil || standingErr != nil {
			t.Fatalf("entry %d flags=%v standing=%v", index, flagsErr, standingErr)
		}
		if index == 72 && (flags != 1 || standing != 42999) {
			t.Fatalf("stormwind entry flags=%d standing=%d", flags, standing)
		}
	}
}

func TestPlayerFieldBytesExposeGrantableLevels(t *testing.T) {
	base := uint32(0xA5000008)
	if got := playerFieldBytesValue(playerState{PlayerFieldBytes: base, GrantableLevels: 2}); got != base|0x00000100 {
		t.Fatalf("grantable flag=%x", got)
	}
	if got := playerFieldBytesValue(playerState{PlayerFieldBytes: base | 0x00000100}); got != base {
		t.Fatalf("cleared grantable flag=%x", got)
	}
}

func TestFishingStepsLoadAndSave(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE character_fishingsteps (guid INTEGER PRIMARY KEY, fishingSteps INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO character_fishingsteps VALUES (7, 4)"); err != nil {
		t.Fatal(err)
	}
	sess := &session{server: &Server{CharactersStore: &database.Store{DB: db}}}
	state := playerState{GUID: 7}
	if err := sess.loadFishingSteps(context.Background(), &state); err != nil || state.FishingSteps != 4 {
		t.Fatalf("loaded fishing steps=%d err=%v", state.FishingSteps, err)
	}
	state.FishingSteps = 6
	if err := sess.saveFishingSteps(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	var stored int
	if err := db.QueryRow("SELECT fishingSteps FROM character_fishingsteps WHERE guid = 7").Scan(&stored); err != nil || stored != 6 {
		t.Fatalf("stored fishing steps=%d err=%v", stored, err)
	}
	state.FishingSteps = 0
	if err := sess.saveFishingSteps(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM character_fishingsteps WHERE guid = 7").Scan(&count); err != nil || count != 0 {
		t.Fatalf("deleted fishing steps count=%d err=%v", count, err)
	}
}

func TestInstanceStateLoad(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE characters (guid INTEGER PRIMARY KEY, instance_id INTEGER, instance_mode_mask INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO characters VALUES (7, 42, 33)"); err != nil {
		t.Fatal(err)
	}
	sess := &session{server: &Server{CharactersStore: &database.Store{DB: db}}}
	state := playerState{GUID: 7}
	if err := sess.loadInstanceState(context.Background(), &state); err != nil || state.InstanceID != 42 || state.InstanceModeMask != 33 || state.DungeonDifficulty != 1 || state.RaidDifficulty != 2 {
		t.Fatalf("instance state=%d/%d dungeon=%d raid=%d err=%v", state.InstanceID, state.InstanceModeMask, state.DungeonDifficulty, state.RaidDifficulty, err)
	}
}

func TestApplyOfflineRestBonusUsesReferenceBubblesAndCap(t *testing.T) {
	state := &playerState{Level: 20, LogoutTime: time.Now().Unix() - 3600, RestBonus: 0, LogoutResting: true}
	applyOfflineRestBonus(state)
	want := float32(xpCurve[20]) * 1.5 / 2
	if state.RestBonus <= 0 || state.RestBonus > want {
		t.Fatalf("rest bonus=%f want positive <= %f", state.RestBonus, want)
	}
	state.RestBonus = want * 2
	state.LogoutTime = time.Now().Unix() - 3600
	applyOfflineRestBonus(state)
	if state.RestBonus != want {
		t.Fatalf("rest cap=%f want=%f", state.RestBonus, want)
	}
}

func TestBuildBagCreateBlockIncludesContentsAndContainer(t *testing.T) {
	bagGUID := uint64(0x4000000000000019)
	itemGUID := uint64(0x4000000000000020)
	block := buildItemCreateBlockForLocation(bagGUID, 1725, 1, 26, 26, 4, map[uint32]uint64{0: itemGUID})
	reader := protocol.NewReader(block)
	if value, err := reader.ReadU8(); err != nil || value != protocol.UpdateCreateObject2 {
		t.Fatalf("update type=%d err=%v", value, err)
	}
	if value, err := reader.ReadPackedGUID(); err != nil || value != bagGUID {
		t.Fatalf("bag guid=%x err=%v", value, err)
	}
	if value, err := reader.ReadU8(); err != nil || value != 2 {
		t.Fatalf("object type=%d err=%v", value, err)
	}
	if flags, err := reader.ReadU16(); err != nil || flags != 0x0010 {
		t.Fatalf("update flags=%x err=%v", flags, err)
	}
	if lowguid, err := reader.ReadU32(); err != nil || lowguid != uint32(bagGUID&0xFFFFFFFF) {
		t.Fatalf("lowguid=%x err=%v", lowguid, err)
	}
	maskBlocks, err := reader.ReadU8()
	if err != nil {
		t.Fatal(err)
	}
	mask := make([]uint32, maskBlocks)
	for index := range mask {
		if mask[index], err = reader.ReadU32(); err != nil {
			t.Fatal(err)
		}
	}
	values := make(map[int]uint32)
	for index := 0; index < 76; index++ {
		if mask[index/32]&(1<<uint(index%32)) == 0 {
			continue
		}
		value, readErr := reader.ReadU32()
		if readErr != nil {
			t.Fatal(readErr)
		}
		values[index] = value
	}
	if values[8] != 26 || values[9] != 0 || values[64] != 4 || values[66] != uint32(itemGUID) || values[67] != uint32(itemGUID>>32) {
		t.Fatalf("bag fields=%x/%x/%d/%x/%x", values[8], values[9], values[64], values[66], values[67])
	}
}

func TestBuildPlayerUpdateKeepsMovementAndUpdateMaskAligned(t *testing.T) {
	server := &Server{}
	packet, err := server.buildPlayerUpdate(playerState{GUID: 26, Gender: 1, FacialStyle: 3, BankBagSlots: 4, RestState: 2, DrunkenState: 77, SheathState: 1, Level: 21, Map: 0, X: 1, Y: 2, Z: 3, Orientation: 4})
	if err != nil {
		t.Fatal(err)
	}
	payload := packet.Payload.Bytes()
	if packet.Opcode == uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		payload, err = protocol.DecompressUpdatePayload(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := protocol.NewReader(payload)
	if blocks, err := reader.ReadU32(); err != nil || blocks != 1 {
		t.Fatalf("blocks=%d err=%v", blocks, err)
	}
	if updateType, err := reader.ReadU8(); err != nil || updateType != protocol.UpdateCreateObject2 {
		t.Fatalf("update type=%d err=%v", updateType, err)
	}
	if guid, err := reader.ReadPackedGUID(); err != nil || guid != 26 {
		t.Fatalf("guid=%d err=%v", guid, err)
	}
	if objectType, err := reader.ReadU8(); err != nil || objectType != 4 {
		t.Fatalf("object type=%d err=%v", objectType, err)
	}
	if flags, err := reader.ReadU16(); err != nil || flags != 0x0061 {
		t.Fatalf("update flags=%x err=%v", flags, err)
	}
	if _, err := reader.ReadU32(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadU16(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadU32(); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		if _, err := reader.ReadF32(); err != nil {
			t.Fatal(err)
		}
	}
	if fallTime, err := reader.ReadU32(); err != nil || fallTime != 0 {
		t.Fatalf("fall time=%d err=%v", fallTime, err)
	}
	for index := 0; index < 9; index++ {
		if _, err := reader.ReadF32(); err != nil {
			t.Fatal(err)
		}
	}
	maskBlocks, err := reader.ReadU8()
	if err != nil || maskBlocks != 42 {
		t.Fatalf("mask blocks=%d err=%v", maskBlocks, err)
	}
	mask := make([]uint32, maskBlocks)
	for index := range mask {
		if mask[index], err = reader.ReadU32(); err != nil {
			t.Fatal(err)
		}
	}
	if mask[0]&0x3 != 0x3 {
		t.Fatalf("object guid mask=%x", mask[0])
	}
	values := make(map[int]uint32)
	for index := 0; index < playerValuesCount; index++ {
		if mask[index/32]&(1<<uint(index%32)) == 0 {
			continue
		}
		value, readErr := reader.ReadU32()
		if readErr != nil {
			t.Fatal(readErr)
		}
		values[index] = value
	}
	if values[0] != 26 || values[1] != 0 || values[2] != 0x19 {
		t.Fatalf("object values=%x/%x/%x", values[0], values[1], values[2])
	}
	if values[unitFieldBytes2] != 1 || values[unitFieldPlayerBytes2] != 0x02040003 || values[unitFieldPlayerBytes3] != 0x4D01 {
		t.Fatalf("player byte fields=%x/%x/%x", values[unitFieldBytes2], values[unitFieldPlayerBytes2], values[unitFieldPlayerBytes3])
	}
}

func TestBuildPlayerUpdateCharacterSheetFields(t *testing.T) {
	server := &Server{}
	state := playerState{
		GUID:      100,
		Race:      1,
		Class:     1,
		Level:     20,
		Health:    500,
		MaxHealth: 500,
		Powers:    [7]uint32{400},
		MaxPowers: [7]uint32{400},
		Talents:   map[uint32]uint8{1: 2, 2: 1}, // rank 2 (3 pts) + rank 1 (2 pts) = 5 pts
	}
	packet, err := server.buildPlayerUpdate(state)
	if err != nil {
		t.Fatal(err)
	}
	payload := packet.Payload.Bytes()
	if packet.Opcode == uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		payload, err = protocol.DecompressUpdatePayload(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := protocol.NewReader(payload)
	_, _ = reader.ReadU32() // blocks
	_, _ = reader.ReadU8()  // updateType
	_, _ = reader.ReadPackedGUID()
	_, _ = reader.ReadU8()  // obj type
	_, _ = reader.ReadU16() // flags
	_, _ = reader.ReadU32()
	_, _ = reader.ReadU16()
	_, _ = reader.ReadU32()
	for i := 0; i < 4; i++ {
		_, _ = reader.ReadF32()
	}
	_, _ = reader.ReadU32() // fall time
	for i := 0; i < 9; i++ {
		_, _ = reader.ReadF32()
	}
	maskBlocks, _ := reader.ReadU8()
	mask := make([]uint32, maskBlocks)
	for i := range mask {
		mask[i], _ = reader.ReadU32()
	}
	values := make(map[int]uint32)
	for i := 0; i < playerValuesCount; i++ {
		if mask[i/32]&(1<<uint(i%32)) == 0 {
			continue
		}
		val, err := reader.ReadU32()
		if err != nil {
			t.Fatal(err)
		}
		values[i] = val
	}

	// Attack speeds and damages
	if values[unitFieldRangedAttackTime] != 2000 {
		t.Errorf("ranged attack time expected 2000, got %d", values[unitFieldRangedAttackTime])
	}
	if values[unitModCastSpeed] != math.Float32bits(1.0) {
		t.Errorf("cast speed expected 1.0, got %f", math.Float32frombits(values[unitModCastSpeed]))
	}
	if values[playerFieldModDamageDonePct] != math.Float32bits(1.0) {
		t.Errorf("damage done pct expected 1.0, got %f", math.Float32frombits(values[playerFieldModDamageDonePct]))
	}

	// Base stats (20 + 20*2 = 60)
	if values[unitFieldStat0] != 60 || values[unitFieldStat1] != 60 {
		t.Errorf("expected stat0=60, got %d", values[unitFieldStat0])
	}
	if values[unitFieldResistances] != 120 {
		t.Errorf("expected armor=120, got %d", values[unitFieldResistances])
	}

	// Free talent points: level 20 has (20 - 9) = 11 total. Spent = (2+1) + (1+1) = 5. Free = 6.
	if values[playerCharacterPoints1] != 6 {
		t.Errorf("expected free talent points 6, got %d", values[playerCharacterPoints1])
	}
	if values[playerCharacterPoints2] != 5 {
		t.Errorf("expected spent talent points 5, got %d", values[playerCharacterPoints2])
	}

	// UNIT_FIELD_BYTES_0: Race, Class, Gender, PowerType (prevents client 0x007F69D1 crash)
	bytes0 := values[unitFieldBytes0]
	raceByte := uint8(bytes0 & 0xFF)
	classByte := uint8((bytes0 >> 8) & 0xFF)
	genderByte := uint8((bytes0 >> 16) & 0xFF)
	powerTypeByte := uint8((bytes0 >> 24) & 0xFF)
	if raceByte != 1 || classByte != 1 || genderByte != 0 || powerTypeByte != 1 {
		t.Errorf("expected bytes0=(race 1, class 1, gender 0, power 1), got race %d, class %d, gender %d, power %d", raceByte, classByte, genderByte, powerTypeByte)
	}

	// PLAYER_NEXT_LEVEL_XP: Must match xpCurve so client displays EXP bar
	if values[unitFieldNextLevelXP] != xpCurve[state.Level] {
		t.Errorf("expected unitFieldNextLevelXP %d, got %d", xpCurve[state.Level], values[unitFieldNextLevelXP])
	}
}

func TestIsAllowedClassSkill(t *testing.T) {
	// Warlock (class 9) must NOT have Plate (293), Leather (414), or Mage Frost (6)
	if isAllowedClassSkill(9, 293) {
		t.Error("warlock should not be allowed Plate Mail")
	}
	if isAllowedClassSkill(9, 414) {
		t.Error("warlock should not be allowed Leather")
	}
	if isAllowedClassSkill(9, 6) {
		t.Error("warlock should not be allowed Mage Frost")
	}
	// Warlock MUST be allowed Cloth (415) and Warlock Demo (354)
	if !isAllowedClassSkill(9, 415) {
		t.Error("warlock should be allowed Cloth")
	}
	if !isAllowedClassSkill(9, 354) {
		t.Error("warlock should be allowed Warlock Demo")
	}
	// Warrior (class 1) must be allowed Plate (293) and Leather (414)
	if !isAllowedClassSkill(1, 293) {
		t.Error("warrior should be allowed Plate Mail")
	}
	if !isAllowedClassSkill(1, 414) {
		t.Error("warrior should be allowed Leather")
	}
}
