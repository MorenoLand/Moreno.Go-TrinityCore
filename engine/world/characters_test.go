package world

import (
	"context"
	"database/sql"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func TestCharacterCreatePersistsReferenceDefaults(t *testing.T) {
	root, err := packageRoot()
	if err != nil {
		t.Fatal(err)
	}
	stores := makeMemoryStores(t, root)
	if _, err := stores.World.DB.Exec("INSERT INTO playercreateinfo (race, class, map, zone, position_x, position_y, position_z, orientation) VALUES (1, 1, 0, 12, 1.5, 2.5, 3.5, 0.5)"); err != nil {
		t.Fatal(err)
	}
	if _, err := stores.World.DB.Exec("INSERT INTO player_classlevelstats (class, level, basehp, basemana) VALUES (1, 1, 20, 0)"); err != nil {
		t.Fatal(err)
	}
	state := &session{server: NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), 1), accountID: 7, legitimate: make(map[uint64]struct{})}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	state.conn = serverConn
	payload := protocol.NewBuffer(16)
	payload.WriteCString("Newhero")
	for _, value := range []uint8{1, 1, 0, 0, 0, 0, 0, 0, 0} {
		payload.WriteU8(value)
	}
	done := make(chan bool, 1)
	go func() { done <- state.handleCharCreate(context.Background(), payload.Bytes()) }()
	opcode, response, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !<-done || opcode != uint16(protocol.OpcodeSMSG_CHAR_CREATE) || len(response) != 1 || response[0] != charCreateSuccess {
		t.Fatalf("opcode=%x response=%x", opcode, response)
	}
	var count int
	if err := stores.Characters.DB.QueryRow("SELECT COUNT(*) FROM characters WHERE account = 7 AND name = 'Newhero'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("characters=%d", count)
	}
	var health int
	if err := stores.Characters.DB.QueryRow("SELECT health FROM characters WHERE account = 7 AND name = 'Newhero'").Scan(&health); err != nil {
		t.Fatal(err)
	}
	if health != 220 {
		t.Fatalf("created character health=%d", health)
	}
	var realmCount int
	if err := stores.Auth.DB.QueryRow("SELECT numchars FROM realmcharacters WHERE acctid = 7 AND realmid = 1").Scan(&realmCount); err != nil {
		t.Fatal(err)
	}
	if realmCount != 1 {
		t.Fatalf("realm characters=%d", realmCount)
	}
}

func TestLoginSetTimeSpeedUsesUnixGameTime(t *testing.T) {
	now := time.Unix(1770000000, 0)
	reader := protocol.NewReader(buildLoginSetTimeSpeed(now))
	gameTime, err := reader.ReadU32()
	if err != nil {
		t.Fatal(err)
	}
	speed, err := reader.ReadF32()
	if err != nil {
		t.Fatal(err)
	}
	holidayOffset, err := reader.ReadU32()
	if err != nil {
		t.Fatal(err)
	}
	if gameTime != uint32(now.Unix()) || speed != 0.5 || holidayOffset != 0 {
		t.Fatalf("time packet gameTime=%d speed=%v holidayOffset=%d", gameTime, speed, holidayOffset)
	}
}

func TestInstanceDifficultyPacketPreservesDynamicFlag(t *testing.T) {
	reader := protocol.NewReader(buildInstanceDifficultyForMap(2, true))
	difficulty, err := reader.ReadU32()
	if err != nil {
		t.Fatal(err)
	}
	dynamic, err := reader.ReadU32()
	if err != nil {
		t.Fatal(err)
	}
	if difficulty != 2 || dynamic != 1 || reader.Remaining() != 0 {
		t.Fatalf("difficulty=%d dynamic=%d remaining=%d", difficulty, dynamic, reader.Remaining())
	}
}

func TestLoginRaidDifficultyUsesSavedPlayerStateWhenLeavingRaid(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{conn: serverConn, groupID: 5}
	done := make(chan error, 1)
	go func() { done <- sess.sendLoginRaidDifficulty(context.Background(), playerState{RaidDifficulty: 2}) }()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeMSG_SET_RAID_DIFFICULTY) || len(payload) != 12 {
		t.Fatalf("opcode=%x payload=%x", opcode, payload)
	}
	reader := protocol.NewReader(payload)
	difficulty, _ := reader.ReadU32()
	marker, _ := reader.ReadU32()
	isInGroup, _ := reader.ReadU32()
	if difficulty != 2 || marker != 1 || isInGroup != 1 {
		t.Fatalf("difficulty=%d marker=%d group=%d", difficulty, marker, isInGroup)
	}
}

func TestCompleteCinematicPersists(t *testing.T) {
	root, err := packageRoot()
	if err != nil {
		t.Fatal(err)
	}
	stores := makeMemoryStores(t, root)
	if _, err := stores.Characters.DB.Exec("INSERT INTO characters (guid, account, name, race, class, gender, level, map, position_x, position_y, position_z, orientation, taximask, cinematic) VALUES (9, 7, 'Cine', 1, 1, 0, 1, 0, 0, 0, 0, 0, '', 0)"); err != nil {
		t.Fatal(err)
	}
	server := NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	state := &session{server: server, accountID: 7, playerGUID: 9, playerLoaded: true, player: &playerState{GUID: 9}}
	if !state.handleCompleteCinematic(context.Background()) {
		t.Fatal("handleCompleteCinematic failed")
	}
	var cinematic int
	if err := stores.Characters.DB.QueryRow("SELECT cinematic FROM characters WHERE guid = 9").Scan(&cinematic); err != nil {
		t.Fatal(err)
	}
	if cinematic != 1 || state.player.Cinematic != 1 {
		t.Fatalf("cinematic=%d state=%d", cinematic, state.player.Cinematic)
	}
}

func TestTutorialFlags(t *testing.T) {
	root, err := packageRoot()
	if err != nil {
		t.Fatal(err)
	}
	stores := makeMemoryStores(t, root)
	server := NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	state := &session{server: server, accountID: 7, authed: true}

	state.loadTutorials(context.Background())
	if state.tutorials != [8]uint32{} {
		t.Fatalf("initial tutorials=%v", state.tutorials)
	}

	flagBuf := protocol.NewBuffer(4)
	flagBuf.WriteU32(22) // bit 22 of tutorial 0 (chat tutorial)
	if !state.handleTutorialFlag(context.Background(), flagBuf.Bytes()) {
		t.Fatal("handleTutorialFlag failed")
	}
	if state.tutorials[0] != (1 << 22) {
		t.Fatalf("tutorials[0]=%x", state.tutorials[0])
	}

	state.loadTutorials(context.Background())
	if state.tutorials[0] != (1 << 22) {
		t.Fatalf("persisted tutorials[0]=%x", state.tutorials[0])
	}

	if !state.handleTutorialClear(context.Background()) {
		t.Fatal("handleTutorialClear failed")
	}
	for i, v := range state.tutorials {
		if v != 0xFFFFFFFF {
			t.Fatalf("tutorials[%d]=%x", i, v)
		}
	}

	if !state.handleTutorialReset(context.Background()) {
		t.Fatal("handleTutorialReset failed")
	}
	for i, v := range state.tutorials {
		if v != 0 {
			t.Fatalf("tutorials[%d]=%x", i, v)
		}
	}
}

func TestLogoutSitRootStunAndCancel(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	srv := &Server{}
	sess := &session{server: srv, conn: serverConn, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1, StandState: 0, UnitFlags: 0}}
	ctx := context.Background()

	// Handle logout request
	go func() {
		sess.handleLogoutRequest(ctx)
	}()

	// 1. Read SMSG_LOGOUT_RESPONSE
	op1, _, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE) {
		t.Fatalf("expected SMSG_LOGOUT_RESPONSE, got op=%x err=%v", op1, err)
	}
	// 2. Read SMSG_FORCE_MOVE_ROOT
	op2, _, err := readServerFrame(clientConn, nil)
	if err != nil || op2 != uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT) {
		t.Fatalf("expected SMSG_FORCE_MOVE_ROOT, got op=%x err=%v", op2, err)
	}
	// 3. Read SMSG_UPDATE_OBJECT / SMSG_COMPRESSED_UPDATE_OBJECT
	op3, _, err := readServerFrame(clientConn, nil)
	if err != nil || (op3 != uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) && op3 != uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)) {
		t.Fatalf("expected update object, got op=%x err=%v", op3, err)
	}

	// Verify player state is sitting and stunned
	if sess.player.StandState != 1 {
		t.Fatalf("expected player StandState=1 (sit), got %d", sess.player.StandState)
	}
	if sess.player.UnitFlags&unitFlagStunned == 0 {
		t.Fatalf("expected player UnitFlags to have UNIT_FLAG_STUNNED (%x), got %x", unitFlagStunned, sess.player.UnitFlags)
	}

	// Handle logout cancel
	go func() {
		sess.handleLogoutCancel()
	}()

	// 4. Read SMSG_FORCE_MOVE_UNROOT
	op4, _, err := readServerFrame(clientConn, nil)
	if err != nil || op4 != uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT) {
		t.Fatalf("expected SMSG_FORCE_MOVE_UNROOT, got op=%x err=%v", op4, err)
	}
	// 5. Read update object
	op5, _, err := readServerFrame(clientConn, nil)
	if err != nil || (op5 != uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) && op5 != uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)) {
		t.Fatalf("expected update object, got op=%x err=%v", op5, err)
	}
	// 6. Read SMSG_LOGOUT_CANCEL_ACK
	op6, _, err := readServerFrame(clientConn, nil)
	if err != nil || op6 != uint16(protocol.OpcodeSMSG_LOGOUT_CANCEL_ACK) {
		t.Fatalf("expected SMSG_LOGOUT_CANCEL_ACK, got op=%x err=%v", op6, err)
	}

	// Verify player state restored: standing and not stunned
	if sess.player.StandState != 0 {
		t.Fatalf("expected player StandState=0 (stand), got %d", sess.player.StandState)
	}
	if sess.player.UnitFlags&unitFlagStunned != 0 {
		t.Fatalf("expected player UnitFlags to clear UNIT_FLAG_STUNNED, got %x", sess.player.UnitFlags)
	}
}

func TestPlayerStartAllSpellsConfigGating(t *testing.T) {
	cdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	cdb.SetMaxOpenConns(1)
	wdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer wdb.Close()

	if _, err := cdb.Exec(`CREATE TABLE character_spell (
		guid INTEGER NOT NULL,
		spell INTEGER NOT NULL,
		active INTEGER NOT NULL DEFAULT 1,
		disabled INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (guid, spell)
	)`); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`CREATE TABLE playercreateinfo_spell_custom (racemask INTEGER NOT NULL, classmask INTEGER NOT NULL, Spell INTEGER NOT NULL, Note TEXT)`,
		`CREATE TABLE playercreateinfo_action (race INTEGER NOT NULL, class INTEGER NOT NULL, button INTEGER NOT NULL, action INTEGER NOT NULL, type INTEGER NOT NULL)`,
		`CREATE TABLE playercreateinfo_cast_spell (raceMask INTEGER NOT NULL, classMask INTEGER NOT NULL, spell INTEGER NOT NULL, note TEXT)`,
		`INSERT INTO playercreateinfo_spell_custom VALUES (1, 1, 12294, 'Mortal Strike Rank 1')`,
		`INSERT INTO playercreateinfo_action VALUES (1, 1, 0, 1001, 0)`,
		`INSERT INTO playercreateinfo_cast_spell VALUES (1, 1, 1002, 'stance')`,
	} {
		if _, err := wdb.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}

	ctx := context.Background()

	// 1. When PlayerStartAllSpells = false (default TrinityCore parity)
	srvDefault := &Server{
		CharactersStore: cStore,
		WorldStore:      wStore,
		Config:          config.Config{PlayerStartAllSpells: false},
	}
	sessDefault := &session{server: srvDefault, playerGUID: 100}
	sessDefault.createStarterSpells(ctx, 100, 1, 1)
	var count int
	_ = cdb.QueryRow("SELECT COUNT(*) FROM character_spell WHERE guid = 100 AND spell = 12294").Scan(&count)
	if count != 0 {
		t.Fatalf("expected custom trainer spell 12294 NOT to be learned by default, got count=%d", count)
	}
	spells, err := sessDefault.loadLearnedSpells(ctx, 100, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, sp := range spells {
		if sp.ID == 12294 {
			t.Fatalf("expected custom trainer spell 12294 NOT in loaded spells with PlayerStartAllSpells=false")
		}
	}
	for _, id := range []uint32{1001, 1002} {
		found := false
		for _, sp := range spells {
			if sp.ID == id && sp.Active && !sp.Disabled {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected starter spell %d in loaded spells", id)
		}
	}

	// 2. When PlayerStartAllSpells = true (optional full spell learning mode)
	srvAll := &Server{
		CharactersStore: cStore,
		WorldStore:      wStore,
		Config:          config.Config{PlayerStartAllSpells: true},
	}
	sessAll := &session{server: srvAll, playerGUID: 200}
	sessAll.createStarterSpells(ctx, 200, 1, 1)
	countAll := 0
	_ = cdb.QueryRow("SELECT COUNT(*) FROM character_spell WHERE guid = 200 AND spell = 12294").Scan(&countAll)
	if countAll != 1 {
		t.Fatalf("expected custom trainer spell 12294 to be learned when PlayerStartAllSpells=true, got count=%d", countAll)
	}
	spellsAll, err := sessAll.loadLearnedSpells(ctx, 200, 1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sp := range spellsAll {
		if sp.ID == 12294 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected custom trainer spell 12294 in loaded spells with PlayerStartAllSpells=true")
	}
}

func TestLearnedSpellsHideFutureSpellLevels(t *testing.T) {
	cdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	cdb.SetMaxOpenConns(1)
	if _, err := cdb.Exec(`CREATE TABLE character_spell (guid INTEGER NOT NULL, spell INTEGER NOT NULL, active INTEGER NOT NULL DEFAULT 1, disabled INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (guid, spell))`); err != nil {
		t.Fatal(err)
	}
	if _, err := cdb.Exec(`INSERT INTO character_spell VALUES (1, 5001, 1, 0), (1, 5002, 1, 0)`); err != nil {
		t.Fatal(err)
	}
	dbcDir := t.TempDir()
	const fieldCount = 234
	records := make([]byte, fieldCount*4*2)
	for i, values := range [][2]uint32{{5001, 10}, {5002, 70}} {
		offset := i * fieldCount * 4
		binary.LittleEndian.PutUint32(records[offset:], values[0])
		binary.LittleEndian.PutUint32(records[offset+39*4:], values[1])
	}
	header := make([]byte, 20)
	copy(header, "WDBC")
	binary.LittleEndian.PutUint32(header[4:8], 2)
	binary.LittleEndian.PutUint32(header[8:12], fieldCount)
	binary.LittleEndian.PutUint32(header[12:16], fieldCount*4)
	binary.LittleEndian.PutUint32(header[16:20], 1)
	if err := os.WriteFile(filepath.Join(dbcDir, "Spell.dbc"), append(header, append(records, 0)...), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	server := &Server{CharactersStore: store, Config: config.Config{}, Data: wotlk.NewStore(dbcDir)}
	future, found, err := server.Data.Spell(5002)
	if err != nil || !found || future.SpellLevel != 70 {
		t.Fatalf("future spell=%+v found=%v err=%v", future, found, err)
	}
	sess := &session{server: server}
	spells, err := sess.loadLearnedSpells(context.Background(), 1, 1, 1, 21)
	if err != nil {
		t.Fatal(err)
	}
	for _, spell := range spells {
		if spell.ID == 5002 && spell.Active {
			t.Fatal("future spell remained active at level 21")
		}
	}
	var active int
	if err := cdb.QueryRow("SELECT active FROM character_spell WHERE guid = 1 AND spell = 5002").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("future spell database active=%d", active)
	}
	if _, err := cdb.Exec("UPDATE character_spell SET active = 1 WHERE guid = 1 AND spell = 5002"); err != nil {
		t.Fatal(err)
	}
	server.Config.PlayerStartAllSpells = true
	spells, err = sess.loadLearnedSpells(context.Background(), 1, 1, 1, 21)
	if err != nil {
		t.Fatal(err)
	}
	for _, spell := range spells {
		if spell.ID == 5002 && spell.Active {
			return
		}
	}
	t.Fatal("all-spells mode did not preserve future spell")
}

func TestStarterSpellsHideFutureSpellLevels(t *testing.T) {
	cdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	wdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer wdb.Close()
	if _, err := cdb.Exec("CREATE TABLE character_spell (guid INTEGER, spell INTEGER, active INTEGER, disabled INTEGER, PRIMARY KEY (guid, spell))"); err != nil {
		t.Fatal(err)
	}
	if _, err := wdb.Exec("CREATE TABLE playercreateinfo_cast_spell (raceMask INTEGER, classMask INTEGER, spell INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := wdb.Exec("INSERT INTO playercreateinfo_cast_spell VALUES (1, 1, 5003)"); err != nil {
		t.Fatal(err)
	}
	dbcDir := t.TempDir()
	const fieldCount = 234
	record := make([]byte, fieldCount*4)
	binary.LittleEndian.PutUint32(record[0:], 5003)
	binary.LittleEndian.PutUint32(record[39*4:], 70)
	header := make([]byte, 20)
	copy(header, "WDBC")
	binary.LittleEndian.PutUint32(header[4:8], 1)
	binary.LittleEndian.PutUint32(header[8:12], fieldCount)
	binary.LittleEndian.PutUint32(header[12:16], fieldCount*4)
	binary.LittleEndian.PutUint32(header[16:20], 1)
	if err := os.WriteFile(filepath.Join(dbcDir, "Spell.dbc"), append(header, append(record, 0)...), 0o644); err != nil {
		t.Fatal(err)
	}
	characters := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	world := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	sess := &session{server: &Server{CharactersStore: characters, WorldStore: world, Data: wotlk.NewStore(dbcDir)}, playerGUID: 1}
	spells, err := sess.loadLearnedSpells(context.Background(), 1, 1, 1, 21)
	if err != nil {
		t.Fatal(err)
	}
	for _, spell := range spells {
		if spell.ID == 5003 {
			t.Fatal("future starter spell was exposed below its required level")
		}
	}
}

func TestLearnedSpellsFallbackToTrainerRequiredLevel(t *testing.T) {
	cdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer cdb.Close()
	if _, err := cdb.Exec("CREATE TABLE character_spell (guid INTEGER, spell INTEGER, active INTEGER, disabled INTEGER, PRIMARY KEY (guid, spell))"); err != nil {
		t.Fatal(err)
	}
	if _, err := cdb.Exec("INSERT INTO character_spell VALUES (1, 5004, 1, 0)"); err != nil {
		t.Fatal(err)
	}
	wdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer wdb.Close()
	if _, err := wdb.Exec("CREATE TABLE trainer_spell (TrainerId INTEGER, SpellId INTEGER, MoneyCost INTEGER, ReqSkillLine INTEGER, ReqSkillRank INTEGER, ReqLevel INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := wdb.Exec("INSERT INTO trainer_spell VALUES (1, 5004, 0, 0, 0, 70)"); err != nil {
		t.Fatal(err)
	}
	dbcDir := t.TempDir()
	const fieldCount = 234
	record := make([]byte, fieldCount*4)
	binary.LittleEndian.PutUint32(record, 5004)
	header := make([]byte, 20)
	copy(header, "WDBC")
	binary.LittleEndian.PutUint32(header[4:8], 1)
	binary.LittleEndian.PutUint32(header[8:12], fieldCount)
	binary.LittleEndian.PutUint32(header[12:16], fieldCount*4)
	binary.LittleEndian.PutUint32(header[16:20], 1)
	if err := os.WriteFile(filepath.Join(dbcDir, "Spell.dbc"), append(header, append(record, 0)...), 0o644); err != nil {
		t.Fatal(err)
	}
	server := &Server{CharactersStore: &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}, WorldStore: &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}, Config: config.Config{}, Data: wotlk.NewStore(dbcDir)}
	sess := &session{server: server}
	spells, err := sess.loadLearnedSpells(context.Background(), 1, 1, 1, 21)
	if err != nil {
		t.Fatal(err)
	}
	for _, spell := range spells {
		if spell.ID == 5004 && spell.Active {
			t.Fatal("trainer-required spell remained active below its required level")
		}
	}
}

func TestCompleteLogoutCleansStateBeforeCompletionPacket(t *testing.T) {
	root, err := packageRoot()
	if err != nil {
		t.Fatal(err)
	}
	stores := makeMemoryStores(t, root)
	server := NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	if _, err := stores.Characters.DB.Exec("INSERT INTO item_instance (guid, itemEntry, owner_guid, count, enchantments) VALUES (700, 5001, 9, 1, '')"); err != nil {
		t.Fatal(err)
	}
	if _, err := stores.Characters.DB.Exec("INSERT INTO character_inventory (guid, bag, slot, item) VALUES (9, 0, 74, 700)"); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{server: server, conn: serverConn, accountID: 7, playerGUID: 9, playerLoaded: true, bgQueues: [2]bgQueueEntry{{Active: true, BgTypeID: 1}}, player: &playerState{GUID: 9, Name: "Logout", Level: 20, Health: 100, MaxHealth: 100}, activeAuras: map[uint32]*activeAura{123: {SpellID: 123, Slot: 0, CasterGUID: 9}}, auras: map[uint32]struct{}{123: {}}, auraSlots: map[uint32]uint8{123: 0}}
	done := make(chan error, 1)
	go func() { done <- sess.completeLogout(context.Background()) }()
	seenCompletion := false
	for {
		opcode, _, readErr := readServerFrame(clientConn, nil)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if opcode == uint16(protocol.OpcodeSMSG_LOGOUT_COMPLETE) {
			seenCompletion = true
			break
		}
		if seenCompletion {
			t.Fatalf("cleanup packet arrived after logout completion: 0x%x", opcode)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !seenCompletion || sess.playerLoaded || sess.player != nil {
		t.Fatalf("logout state loaded=%v player=%v", sess.playerLoaded, sess.player)
	}
	if sess.bgQueues[0].Active {
		t.Fatal("battleground queue remained active after logout")
	}
	var buybackRows, itemRows int
	if err := stores.Characters.DB.QueryRow("SELECT COUNT(*) FROM character_inventory WHERE guid = 9 AND bag = 0 AND slot BETWEEN 74 AND 85").Scan(&buybackRows); err != nil {
		t.Fatal(err)
	}
	if err := stores.Characters.DB.QueryRow("SELECT COUNT(*) FROM item_instance WHERE guid = 700").Scan(&itemRows); err != nil {
		t.Fatal(err)
	}
	if buybackRows != 0 || itemRows != 0 {
		t.Fatalf("buyback persisted after saved logout: inventory=%d items=%d", buybackRows, itemRows)
	}
}

func TestCompleteLogoutRepopsDeadPlayerBeforeSaving(t *testing.T) {
	root, err := packageRoot()
	if err != nil {
		t.Fatal(err)
	}
	stores := makeMemoryStores(t, root)
	server := NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	if _, err := stores.Characters.DB.Exec("INSERT INTO characters (guid, account, name, race, class, level, position_x, position_y, position_z, taximask, health) VALUES (9, 7, 'DeadLogout', 1, 1, 20, 1, 2, 3, '', 0)"); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{server: server, conn: serverConn, accountID: 7, playerGUID: 9, playerLoaded: true, deathTimer: time.Now().Add(time.Minute), player: &playerState{GUID: 9, Name: "DeadLogout", Race: 1, Level: 20, Map: 0, X: 1, Y: 2, Z: 3, Health: 0, MaxHealth: 100}, activeAuras: make(map[uint32]*activeAura), auras: make(map[uint32]struct{}), auraSlots: make(map[uint32]uint8)}
	done := make(chan error, 1)
	go func() { done <- sess.completeLogout(context.Background()) }()
	for {
		opcode, _, readErr := readServerFrame(clientConn, nil)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if opcode == uint16(protocol.OpcodeSMSG_LOGOUT_COMPLETE) {
			break
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var flags, health int64
	if err := stores.Characters.DB.QueryRow("SELECT playerFlags, health FROM characters WHERE guid = ?", 9).Scan(&flags, &health); err != nil {
		t.Fatal(err)
	}
	if uint32(flags)&playerFlagGhost == 0 || health != 1 {
		t.Fatalf("saved dead state flags=%x health=%d", flags, health)
	}
	var corpses int
	if err := stores.Characters.DB.QueryRow("SELECT COUNT(*) FROM corpse WHERE guid = ?", 9).Scan(&corpses); err != nil {
		t.Fatal(err)
	}
	if corpses != 1 {
		t.Fatalf("corpse rows=%d", corpses)
	}
}

func TestBuildUnlearnSpellsUsesKnownNextRank(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE spell_ranks (first_spell_id INTEGER, spell_id INTEGER, rank INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO spell_ranks VALUES (100, 100, 1), (100, 200, 2)"); err != nil {
		t.Fatal(err)
	}
	store := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: db}
	sess := &session{server: &Server{WorldStore: store}}
	payload := sess.buildUnlearnSpells(context.Background(), playerState{Spells: []learnedSpell{{ID: 100, Active: false}, {ID: 200, Active: true}}})
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	spellID, err := reader.ReadU32()
	if err != nil || spellID != 100 {
		t.Fatalf("spell=%d err=%v", spellID, err)
	}
}

func TestLogoutRejectsPlayerCombatFlag(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{conn: serverConn, playerLoaded: true, player: &playerState{GUID: 9, PlayerFlags: 0, UnitFlags: unitFlagInCombat}}
	done := make(chan bool, 1)
	go func() { done <- sess.handleLogoutRequest(context.Background()) }()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE) || len(payload) != 5 || payload[0] != 1 {
		t.Fatalf("opcode=%x payload=%x", opcode, payload)
	}
	if !<-done || !sess.logoutAt.IsZero() {
		t.Fatalf("logout result or deadline invalid result=%v deadline=%v", sess.playerLoaded, sess.logoutAt)
	}
}

func TestLogoutSecurityLevelDoesNotBypassCombatWithoutRBACPermission(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"CREATE TABLE rbac_account_permissions (accountId INTEGER, permissionId INTEGER, granted INTEGER, realmId INTEGER)",
		"CREATE TABLE rbac_default_permissions (secId INTEGER, permissionId INTEGER, realmId INTEGER)",
		"CREATE TABLE rbac_linked_permissions (id INTEGER, linkedId INTEGER)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	server := &Server{AuthStore: &database.Store{Name: "auth", Backend: database.BackendSQLite, DB: db}}
	sess := &session{server: server, conn: serverConn, accountID: 7, security: 3, playerLoaded: true, player: &playerState{GUID: 9, UnitFlags: unitFlagInCombat}}
	done := make(chan bool, 1)
	go func() { done <- sess.handleLogoutRequest(context.Background()) }()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE) || len(payload) != 5 || payload[0] != 1 {
		t.Fatalf("opcode=%x payload=%x", opcode, payload)
	}
	if !<-done || !sess.logoutAt.IsZero() {
		t.Fatal("security level bypassed combat logout without RBAC permission")
	}
}

func TestLogoutRejectsFallingAndDuelStates(t *testing.T) {
	for _, test := range []struct {
		name    string
		falling bool
		duel    uint64
		reason  uint32
	}{
		{name: "falling", falling: true, reason: 3},
		{name: "duel", duel: 77, reason: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close()
			defer clientConn.Close()
			sess := &session{conn: serverConn, playerLoaded: true, isFalling: test.falling, duelPartner: test.duel, player: &playerState{GUID: 9}}
			done := make(chan bool, 1)
			go func() { done <- sess.handleLogoutRequest(context.Background()) }()
			opcode, payload, err := readServerFrame(clientConn, nil)
			if err != nil {
				t.Fatal(err)
			}
			if opcode != uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE) || len(payload) != 5 {
				t.Fatalf("opcode=%x payload=%x", opcode, payload)
			}
			reader := protocol.NewReader(payload)
			reason, err := reader.ReadU32()
			if err != nil || reason != test.reason {
				t.Fatalf("reason=%d err=%v", reason, err)
			}
			instant, err := reader.ReadU8()
			if err != nil || instant != 0 {
				t.Fatalf("instant=%d err=%v", instant, err)
			}
			if !<-done || !sess.logoutAt.IsZero() {
				t.Fatal("invalid logout rejection state")
			}
		})
	}
}

func makeMemoryStores(t *testing.T, root string) *database.Set {
	t.Helper()
	open := func(name string) *database.Store {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		path := filepath.Join(root, "sql", "sqlite", name+".sql")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range database.SplitSQL(string(data)) {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		return &database.Store{Name: name, Backend: database.BackendSQLite, DB: db}
	}
	return &database.Set{Auth: open("auth"), Characters: open("characters"), World: open("world")}
}

func packageRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..")), nil
}
