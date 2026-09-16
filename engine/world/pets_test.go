package world

import (
	"context"
	"database/sql"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func TestPetNameQueryAndRename(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE character_pet (
			id INTEGER PRIMARY KEY,
			entry INTEGER NOT NULL DEFAULT 0,
			owner INTEGER NOT NULL DEFAULT 0,
			modelid INTEGER DEFAULT 0,
			CreatedBySpell INTEGER NOT NULL DEFAULT 0,
			PetType INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			exp INTEGER NOT NULL DEFAULT 0,
			Reactstate INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT 'Pet',
			renamed INTEGER NOT NULL DEFAULT 0,
			slot INTEGER NOT NULL DEFAULT 0,
			curhealth INTEGER NOT NULL DEFAULT 1,
			curmana INTEGER NOT NULL DEFAULT 0,
			curhappiness INTEGER NOT NULL DEFAULT 0,
			savetime INTEGER NOT NULL DEFAULT 0,
			abdata TEXT
		)`,
		"INSERT INTO character_pet (id, entry, owner, name, renamed, savetime) VALUES (42, 100, 1, 'Fluffy', 0, 1000)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	srv := &Server{CharactersStore: store, Config: config.Default()}
	sess := &session{server: srv, conn: serverConn, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1, Name: "Hero"}}
	ctx := context.Background()

	// 1. Query pet name
	qBuf := protocol.NewBuffer(12)
	qBuf.WriteU32(42) // petNumber
	qBuf.WriteU64(1)  // petGUID

	go func() {
		sess.handlePetNameQuery(ctx, qBuf.Bytes())
	}()

	op, data, err := readServerFrame(clientConn, nil)
	if err != nil || op != uint16(protocol.OpcodeSMSG_PET_NAME_QUERY_RESPONSE) {
		t.Fatalf("expected SMSG_PET_NAME_QUERY_RESPONSE, got op=%x err=%v", op, err)
	}
	r := protocol.NewReader(data)
	num, _ := r.ReadU32()
	name, _ := r.ReadCString()
	if num != 42 || name != "Fluffy" {
		t.Fatalf("expected pet 42 named 'Fluffy', got %d '%s'", num, name)
	}

	// 2. Rename pet
	rBuf := protocol.NewBuffer(32)
	rBuf.WriteU64(42) // petGUID with ID 42
	rBuf.WriteCString("Rex")
	if !sess.handlePetRename(ctx, rBuf.Bytes()) {
		t.Fatal("handlePetRename failed")
	}

	// Verify name updated in DB
	var dbName string
	var renamed int
	_ = db.QueryRow("SELECT name, renamed FROM character_pet WHERE id = 42").Scan(&dbName, &renamed)
	if dbName != "Rex" || renamed != 1 {
		t.Fatalf("expected pet renamed to 'Rex' (renamed=1), got '%s' (%d)", dbName, renamed)
	}
}

func TestRequestPetInfoWithActivePet(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE character_pet (
			id INTEGER PRIMARY KEY,
			entry INTEGER NOT NULL DEFAULT 0,
			owner INTEGER NOT NULL DEFAULT 0,
			modelid INTEGER DEFAULT 0,
			CreatedBySpell INTEGER NOT NULL DEFAULT 0,
			PetType INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			exp INTEGER NOT NULL DEFAULT 0,
			Reactstate INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT 'Pet',
			renamed INTEGER NOT NULL DEFAULT 0,
			slot INTEGER NOT NULL DEFAULT 0,
			curhealth INTEGER NOT NULL DEFAULT 1,
			curmana INTEGER NOT NULL DEFAULT 0,
			curhappiness INTEGER NOT NULL DEFAULT 0,
			savetime INTEGER NOT NULL DEFAULT 0,
			abdata TEXT
		)`,
		`CREATE TABLE pet_spell (
			guid INTEGER NOT NULL,
			spell INTEGER NOT NULL,
			active INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (guid, spell)
		)`,
		"INSERT INTO character_pet (id, entry, owner, modelid, level, name, slot, Reactstate) VALUES (42, 100, 1, 50, 10, 'Fluffy', 0, 1)",
		"INSERT INTO pet_spell (guid, spell, active) VALUES (42, 16827, 1)", // Claw, autocast
		"INSERT INTO pet_spell (guid, spell, active) VALUES (42, 17253, 0)", // Bite, manual
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	srv := &Server{CharactersStore: store, Config: config.Default()}
	sess := &session{server: srv, conn: serverConn, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1, Name: "Hero"}}
	ctx := context.Background()

	go func() {
		sess.handleRequestPetInfo(ctx, nil)
	}()

	op, data, err := readServerFrame(clientConn, nil)
	if err != nil || op != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS (0x%x), got op=0x%x err=%v", protocol.OpcodeSMSG_PET_SPELLS, op, err)
	}

	r := protocol.NewReader(data)
	petGUID, err := r.ReadU64()
	if err != nil {
		t.Fatalf("failed reading petGUID: %v", err)
	}
	expectedGUID := uint64(42) | (uint64(0xF140) << 48)
	if petGUID != expectedGUID {
		t.Fatalf("expected petGUID %x, got %x", expectedGUID, petGUID)
	}

	family, _ := r.ReadU16()
	duration, _ := r.ReadU32()
	react, _ := r.ReadU8()
	cmd, _ := r.ReadU8()
	flags, _ := r.ReadU16()

	if family != 0 || duration != 0 || react != 1 || cmd != 1 || flags != 0 {
		t.Fatalf("unexpected header values: family=%d dur=%d react=%d cmd=%d flags=%d", family, duration, react, cmd, flags)
	}

	// 10 action bar slots
	expectedSlots := []uint32{
		0x07000002,           // Attack
		0x07000001,           // Follow
		0x07000000,           // Stay
		16827 | (0xC1 << 24), // Spell 16827 (autocast)
		17253 | (0x81 << 24), // Spell 17253 (manual)
		0,                    // Empty
		0,                    // Empty
		0x06000002,           // Aggressive
		0x06000001,           // Defensive
		0x06000000,           // Passive
	}

	for i, exp := range expectedSlots {
		slotVal, _ := r.ReadU32()
		if slotVal != exp {
			t.Fatalf("slot %d: expected 0x%08X, got 0x%08X", i, exp, slotVal)
		}
	}
}

func TestPetActionAndSetAction(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE character_pet (
			id INTEGER PRIMARY KEY,
			entry INTEGER NOT NULL DEFAULT 0,
			owner INTEGER NOT NULL DEFAULT 0,
			modelid INTEGER DEFAULT 0,
			CreatedBySpell INTEGER NOT NULL DEFAULT 0,
			PetType INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			exp INTEGER NOT NULL DEFAULT 0,
			Reactstate INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT 'Pet',
			renamed INTEGER NOT NULL DEFAULT 0,
			slot INTEGER NOT NULL DEFAULT 0,
			curhealth INTEGER NOT NULL DEFAULT 1,
			curmana INTEGER NOT NULL DEFAULT 0,
			curhappiness INTEGER NOT NULL DEFAULT 0,
			savetime INTEGER NOT NULL DEFAULT 0,
			abdata TEXT
		)`,
		`CREATE TABLE pet_spell (
			guid INTEGER NOT NULL,
			spell INTEGER NOT NULL,
			active INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (guid, spell)
		)`,
		"INSERT INTO character_pet (id, entry, owner, modelid, level, name, slot, Reactstate) VALUES (42, 100, 1, 50, 10, 'Fluffy', 0, 1)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	srv := &Server{CharactersStore: store, Config: config.Default()}
	sess := &session{server: srv, conn: serverConn, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1, Name: "Hero"}}
	ctx := context.Background()

	// 1. Test Pet Attack Action emits SMSG_AI_REACTION
	petGUID := uint64(42) | (uint64(0xF140) << 48)
	targetGUID := uint64(12345)
	actBuf := protocol.NewBuffer(20)
	actBuf.WriteU64(petGUID)
	actBuf.WriteU32(0x07000002) // COMMAND_ATTACK
	actBuf.WriteU64(targetGUID)

	go func() {
		sess.handlePetAction(ctx, actBuf.Bytes())
	}()

	op, data, err := readServerFrame(clientConn, nil)
	if err != nil || op != uint16(protocol.OpcodeSMSG_AI_REACTION) {
		t.Fatalf("expected SMSG_AI_REACTION, got op=0x%x err=%v", op, err)
	}
	r := protocol.NewReader(data)
	g, _ := r.ReadU64()
	reactType, _ := r.ReadU32()
	if g != petGUID || reactType != 2 {
		t.Fatalf("expected ai reaction for pet %x type 2, got %x type %d", petGUID, g, reactType)
	}

	// 2. Test Pet Reaction Action updates Reactstate in DB
	reactBuf := protocol.NewBuffer(12)
	reactBuf.WriteU64(petGUID)
	reactBuf.WriteU32(0x06000002) // REACT_AGGRESSIVE = 2
	if !sess.handlePetAction(ctx, reactBuf.Bytes()) {
		t.Fatal("handlePetAction failed for reaction")
	}
	var newReact int
	_ = db.QueryRow("SELECT Reactstate FROM character_pet WHERE id = 42").Scan(&newReact)
	if newReact != 2 {
		t.Fatalf("expected Reactstate 2 in DB, got %d", newReact)
	}

	// 3. Test Pet Set Action saves abdata in DB
	setBuf := protocol.NewBuffer(16)
	setBuf.WriteU64(petGUID)
	setBuf.WriteU32(3)          // slot 3
	setBuf.WriteU32(0x07000000) // set slot 3 to Stay
	if !sess.handlePetSetAction(ctx, setBuf.Bytes()) {
		t.Fatal("handlePetSetAction failed")
	}

	var abdata string
	_ = db.QueryRow("SELECT abdata FROM character_pet WHERE id = 42").Scan(&abdata)
	if abdata == "" {
		t.Fatal("expected abdata to be saved in DB")
	}

	// 4. Test handleRequestPetInfo uses the saved abdata
	go func() {
		sess.handleRequestPetInfo(ctx, nil)
	}()

	op, data, err = readServerFrame(clientConn, nil)
	if err != nil || op != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS, got op=0x%x err=%v", op, err)
	}
	r = protocol.NewReader(data)
	_, _ = r.ReadU64() // petGUID
	_, _ = r.ReadU16() // family
	_, _ = r.ReadU32() // duration
	react, _ := r.ReadU8()
	if react != 2 { // Should now be Aggressive (2)
		t.Fatalf("expected reactState 2 from DB, got %d", react)
	}
	_, _ = r.ReadU8()  // cmd
	_, _ = r.ReadU16() // flags

	// Read slots 0..3
	s0, _ := r.ReadU32()
	s1, _ := r.ReadU32()
	s2, _ := r.ReadU32()
	s3, _ := r.ReadU32()
	if s3 != 0x07000000 {
		t.Fatalf("expected slot 3 to be 0x07000000 (Stay), got 0x%08X (s0=%x s1=%x s2=%x)", s3, s0, s1, s2)
	}
}

func TestPetSpellAutocastAndActionBarSync(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE character_pet (
			id INTEGER PRIMARY KEY,
			entry INTEGER NOT NULL DEFAULT 0,
			owner INTEGER NOT NULL DEFAULT 0,
			modelid INTEGER DEFAULT 0,
			CreatedBySpell INTEGER NOT NULL DEFAULT 0,
			PetType INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			exp INTEGER NOT NULL DEFAULT 0,
			Reactstate INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT 'Pet',
			renamed INTEGER NOT NULL DEFAULT 0,
			slot INTEGER NOT NULL DEFAULT 0,
			curhealth INTEGER NOT NULL DEFAULT 1,
			curmana INTEGER NOT NULL DEFAULT 0,
			curhappiness INTEGER NOT NULL DEFAULT 0,
			savetime INTEGER NOT NULL DEFAULT 0,
			abdata TEXT
		)`,
		`CREATE TABLE pet_spell (
			guid INTEGER NOT NULL,
			spell INTEGER NOT NULL,
			active INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (guid, spell)
		)`,
		// abdata with 10 slots: slot 3 is spell 16827 with actType 129 (0x81, disabled)
		"INSERT INTO character_pet (id, entry, owner, modelid, level, name, slot, Reactstate, abdata) VALUES (42, 100, 1, 50, 10, 'Fluffy', 0, 1, '7 2 7 1 7 0 129 16827 0 0 0 0 0 0 6 2 6 1 6 0')",
		"INSERT INTO pet_spell (guid, spell, active) VALUES (42, 16827, 0)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	srv := &Server{CharactersStore: store, Config: config.Default()}
	sess := &session{server: srv, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1, Name: "Hero"}}
	ctx := context.Background()

	petGUID := uint64(42) | (uint64(0xF140) << 48)

	// 1. Toggle autocast on for spell 16827 via handlePetSpellAutocast
	autoBuf := protocol.NewBuffer(13)
	autoBuf.WriteU64(petGUID)
	autoBuf.WriteU32(16827)
	autoBuf.WriteU8(1) // enable
	if !sess.handlePetSpellAutocast(ctx, autoBuf.Bytes()) {
		t.Fatal("handlePetSpellAutocast failed")
	}

	// Verify pet_spell table has active = 1
	var active int
	err = db.QueryRow("SELECT active FROM pet_spell WHERE guid = 42 AND spell = 16827").Scan(&active)
	if err != nil || active != 1 {
		t.Fatalf("expected pet_spell active=1, got %d (err: %v)", active, err)
	}

	// Verify abdata in character_pet updated slot 3 to 193 (0xC1)
	var abdata string
	_ = db.QueryRow("SELECT abdata FROM character_pet WHERE id = 42").Scan(&abdata)
	if !strings.Contains(abdata, "193 16827") {
		t.Fatalf("expected abdata to contain '193 16827' (ACT_ENABLED), got %q", abdata)
	}

	// 2. Modify action bar via handlePetSetAction: turn off autocast on slot 3 (0x81000000 | 16827)
	setBuf := protocol.NewBuffer(16)
	setBuf.WriteU64(petGUID)
	setBuf.WriteU32(3)                                // slot 3
	setBuf.WriteU32(uint32(0x81<<24) | uint32(16827)) // ACT_DISABLED | 16827
	if !sess.handlePetSetAction(ctx, setBuf.Bytes()) {
		t.Fatal("handlePetSetAction failed")
	}

	// Verify pet_spell table was updated to active = 0
	_ = db.QueryRow("SELECT active FROM pet_spell WHERE guid = 42 AND spell = 16827").Scan(&active)
	if active != 0 {
		t.Fatalf("expected pet_spell active=0 after set action, got %d", active)
	}
}

func TestPetCastSpellAndActionSpellGo(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	srv := &Server{Config: config.Default()}
	sess := &session{server: srv, conn: serverConn, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1, Name: "Hero"}}
	ctx := context.Background()

	petGUID := uint64(42) | (uint64(0xF140) << 48)
	targetGUID := uint64(99999)

	// 1. Test handlePetCastSpell emits SMSG_SPELL_GO
	castBuf := protocol.NewBuffer(32)
	castBuf.WriteU64(petGUID)
	castBuf.WriteU8(1)      // castCount
	castBuf.WriteU32(16827) // Claw
	castBuf.WriteU8(0)      // castFlags
	// Write SpellTargetData with targetGUID
	targetData := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnitWireMask, UnitGUID: targetGUID}
	protocol.WriteSpellTargetData(castBuf, targetData)

	go func() {
		sess.handlePetCastSpell(ctx, castBuf.Bytes())
	}()

	op, data, err := readServerFrame(clientConn, nil)
	if err != nil || op != uint16(protocol.OpcodeSMSG_SPELL_GO) {
		t.Fatalf("expected SMSG_SPELL_GO, got op=0x%x err=%v", op, err)
	}
	r := protocol.NewReader(data)
	casterGUID, _ := r.ReadPackedGUID()
	if casterGUID != petGUID {
		t.Fatalf("expected casterGUID %x, got %x", petGUID, casterGUID)
	}

	// 2. Test handlePetAction with spellOrAction and targetGUID emits SMSG_AI_REACTION followed by SMSG_SPELL_GO
	actBuf := protocol.NewBuffer(20)
	actBuf.WriteU64(petGUID)
	actBuf.WriteU32(uint32(0x81<<24) | 16827) // ACT_DISABLED | 16827
	actBuf.WriteU64(targetGUID)

	go func() {
		sess.handlePetAction(ctx, actBuf.Bytes())
	}()

	// First packet: SMSG_AI_REACTION
	op1, _, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_AI_REACTION) {
		t.Fatalf("expected SMSG_AI_REACTION, got op=0x%x err=%v", op1, err)
	}

	// Second packet: SMSG_SPELL_GO
	op2, _, err := readServerFrame(clientConn, nil)
	if err != nil || op2 != uint16(protocol.OpcodeSMSG_SPELL_GO) {
		t.Fatalf("expected SMSG_SPELL_GO, got op=0x%x err=%v", op2, err)
	}
}

func setupPetTestDatabases(t *testing.T) (*sql.DB, *sql.DB) {
	cdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	cdb.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE character_pet (
			id INTEGER PRIMARY KEY,
			entry INTEGER NOT NULL DEFAULT 0,
			owner INTEGER NOT NULL DEFAULT 0,
			modelid INTEGER DEFAULT 0,
			CreatedBySpell INTEGER NOT NULL DEFAULT 0,
			PetType INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			exp INTEGER NOT NULL DEFAULT 0,
			Reactstate INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT 'Pet',
			renamed INTEGER NOT NULL DEFAULT 0,
			slot INTEGER NOT NULL DEFAULT 0,
			curhealth INTEGER NOT NULL DEFAULT 1,
			curmana INTEGER NOT NULL DEFAULT 0,
			curhappiness INTEGER NOT NULL DEFAULT 0,
			savetime INTEGER NOT NULL DEFAULT 0,
			abdata TEXT
		)`,
		`CREATE TABLE character_pet_declinedname (
			id INTEGER NOT NULL DEFAULT 0,
			owner INTEGER NOT NULL DEFAULT 0,
			genitive TEXT NOT NULL,
			dative TEXT NOT NULL,
			accusative TEXT NOT NULL,
			instrumental TEXT NOT NULL,
			prepositional TEXT NOT NULL,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE pet_spell (
			guid INTEGER NOT NULL DEFAULT 0,
			spell INTEGER NOT NULL DEFAULT 0,
			active INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (guid, spell)
		)`,
		`CREATE TABLE pet_spell_cooldown (
			guid INTEGER NOT NULL DEFAULT 0,
			spell INTEGER NOT NULL DEFAULT 0,
			time INTEGER NOT NULL DEFAULT 0,
			categoryId INTEGER NOT NULL DEFAULT 0,
			categoryEnd INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (guid, spell)
		)`,
	} {
		if _, err := cdb.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	wdb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	wdb.SetMaxOpenConns(1)

	for _, stmt := range []string{
		`CREATE TABLE creature_template (
			entry INTEGER PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			family INTEGER NOT NULL DEFAULT 0,
			modelid1 INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE pet_name_generation (
			id INTEGER PRIMARY KEY,
			word TEXT NOT NULL,
			entry INTEGER NOT NULL DEFAULT 0,
			half INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE pet_levelstats (
			creature_entry INTEGER NOT NULL DEFAULT 0,
			level INTEGER NOT NULL DEFAULT 1,
			hp INTEGER NOT NULL DEFAULT 0,
			mana INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (creature_entry, level)
		)`,
		"INSERT INTO creature_template (entry, name, family, modelid1) VALUES (416, 'Imp', 23, 4449)",
		"INSERT INTO creature_template (entry, name, family, modelid1) VALUES (1860, 'Voidwalker', 16, 1132)",
		"INSERT INTO creature_template (entry, name, family, modelid1) VALUES (303, 'Boar', 1, 1234)",
		"INSERT INTO pet_name_generation (id, word, entry, half) VALUES (1, 'Aba', 416, 0), (2, 'tik', 416, 1)",
		"INSERT INTO pet_levelstats (creature_entry, level, hp, mana) VALUES (416, 1, 140, 48), (416, 8, 280, 150)",
	} {
		if _, err := wdb.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	return cdb, wdb
}

func isUpdateOpcode(op uint16) bool {
	return op == uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) || op == uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)
}

func TestPetSpellPacketIncludesSavedCooldowns(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()
	if _, err := cdb.Exec("INSERT INTO character_pet (id, entry, owner, level, name, slot, curhealth, curmana) VALUES (9, 416, 1001, 8, 'Imp', 0, 100, 20)"); err != nil {
		t.Fatal(err)
	}
	if _, err := cdb.Exec("INSERT INTO pet_spell_cooldown (guid, spell, time, categoryId, categoryEnd) VALUES (9, 3110, ?, 7, 0)", time.Now().Unix()+60); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{server: &Server{CharactersStore: &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}, WorldStore: &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}}, conn: serverConn, playerGUID: 1001, player: &playerState{GUID: 1001, Level: 8}}
	done := make(chan struct{})
	go func() {
		sess.sendPetSpells(context.Background(), 9, 416, 1)
		close(done)
	}()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil || opcode != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("opcode=%x err=%v", opcode, err)
	}
	<-done
	r := protocol.NewReader(payload)
	_, _ = r.ReadU64()
	_, _ = r.ReadU16()
	_, _ = r.ReadU32()
	_, _ = r.ReadU8()
	_, _ = r.ReadU8()
	_, _ = r.ReadU16()
	for i := 0; i < 10; i++ {
		_, _ = r.ReadU32()
	}
	additional, _ := r.ReadU8()
	for i := uint8(0); i < additional; i++ {
		_, _ = r.ReadU32()
	}
	cooldownCount, _ := r.ReadU8()
	spell, _ := r.ReadU32()
	category, _ := r.ReadU16()
	duration, _ := r.ReadU32()
	if cooldownCount != 1 || spell != 3110 || category != 7 || duration < 58000 || duration > 62000 {
		t.Fatalf("cooldowns=%d spell=%d category=%d duration=%d", cooldownCount, spell, category, duration)
	}
}

func TestSummonPet_WarlockDemon(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	srv := &Server{CharactersStore: cStore, WorldStore: wStore, Config: config.Default()}
	sess := &session{
		server:       srv,
		conn:         serverConn,
		playerGUID:   1001,
		playerLoaded: true,
		player:       &playerState{GUID: 1001, Name: "WarlockGuy", Level: 1, Class: 9, Race: 1},
	}
	ctx := context.Background()

	// Summon Imp (entry 416, spell 688)
	go func() {
		sess.handleSummonPet(ctx, 688, 416)
	}()

	// 1. First packet should be update for pet spawn
	op1, _, err := readServerFrame(clientConn, nil)
	if err != nil || !isUpdateOpcode(op1) {
		t.Fatalf("expected update for pet spawn, got op=0x%x err=%v", op1, err)
	}

	// 2. Second packet: update for player update (unitFieldSummon set)
	op2, _, err := readServerFrame(clientConn, nil)
	if err != nil || !isUpdateOpcode(op2) {
		t.Fatalf("expected update for player update, got op=0x%x err=%v", op2, err)
	}

	// 3. Third packet: SMSG_PET_SPELLS
	op3, pSpellsData, err := readServerFrame(clientConn, nil)
	if err != nil || op3 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS, got op=0x%x err=%v", op3, err)
	}

	// Verify SMSG_PET_SPELLS content
	r := protocol.NewReader(pSpellsData)
	petGUID, _ := r.ReadU64()
	family, _ := r.ReadU16()
	_, _ = r.ReadU32() // duration
	react, _ := r.ReadU8()
	cmd, _ := r.ReadU8()
	_, _ = r.ReadU16() // flags

	if petGUID == 0 {
		t.Fatalf("expected non-zero pet GUID")
	}
	if family != 23 { // Imp family = 23
		t.Errorf("expected Imp family 23, got %d", family)
	}
	if react != 1 || cmd != 1 {
		t.Errorf("expected react 1 cmd 1, got %d %d", react, cmd)
	}

	// Read 10 action bar slots
	var slots [10]uint32
	for i := 0; i < 10; i++ {
		slots[i], _ = r.ReadU32()
	}
	if slots[0] != 0x07000002 { // Attack
		t.Errorf("slot 0 expected Attack, got 0x%x", slots[0])
	}
	// Slot 3 should have Firebolt (3110) with autocast enabled (0xC1)
	expectedFireboltSlot := uint32(3110) | (uint32(0xC1) << 24)
	if slots[3] != expectedFireboltSlot {
		t.Errorf("slot 3 expected Firebolt (0x%x), got 0x%x", expectedFireboltSlot, slots[3])
	}

	// Read additional spells count (should populate spellbook pet tab!)
	addCount, _ := r.ReadU8()
	if addCount == 0 {
		t.Errorf("expected additional spells for pet spellbook, got 0")
	}

	// Verify database state: character_pet has slot 0 and pet_spell has spells
	var dbPetID, dbEntry, dbSlot int64
	var dbName string
	err = cdb.QueryRow("SELECT id, entry, slot, name FROM character_pet WHERE owner = 1001").Scan(&dbPetID, &dbEntry, &dbSlot, &dbName)
	if err != nil || dbSlot != 0 || dbEntry != 416 {
		t.Fatalf("expected character_pet entry 416 in slot 0, got err=%v entry=%d slot=%d", err, dbEntry, dbSlot)
	}
	if dbName != "Abatik" { // Aba + tik
		t.Errorf("expected generated name 'Abatik', got '%s'", dbName)
	}

	// Verify pet_spell has Firebolt
	var spCount int64
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = ? AND spell = 3110", dbPetID).Scan(&spCount)
	if spCount != 1 {
		t.Errorf("expected Firebolt in pet_spell table")
	}
}

func TestSummonPet_DismissAndCallPet(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	srv := &Server{CharactersStore: cStore, WorldStore: wStore, Config: config.Default()}
	sess := &session{
		server:       srv,
		conn:         serverConn,
		playerGUID:   1002,
		playerLoaded: true,
		player:       &playerState{GUID: 1002, Name: "HunterGuy", Level: 10, Class: 3, Race: 1},
	}
	ctx := context.Background()

	// Pre-insert a Hunter pet into character_pet
	_, _ = cdb.Exec("INSERT INTO character_pet (id, entry, owner, modelid, PetType, level, slot, curhealth, curmana, name) VALUES (55, 303, 1002, 1234, 1, 10, 0, 300, 100, 'Pumba')")
	sess.player.PetGUID = uint64(55) | (uint64(0xF140) << 48)

	// 1. Dismiss Pet (handleDismissPet)
	go func() {
		sess.handleDismissPet(ctx)
	}()

	// Should receive SMSG_DESTROY_OBJECT
	op1, _, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_DESTROY_OBJECT) {
		t.Fatalf("expected SMSG_DESTROY_OBJECT on dismiss, got op=0x%x err=%v", op1, err)
	}

	// Should receive SMSG_PET_SPELLS with 0 (closing pet bar)
	op2, data2, err := readServerFrame(clientConn, nil)
	if err != nil || op2 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS on dismiss, got op=0x%x err=%v", op2, err)
	}
	r2 := protocol.NewReader(data2)
	pGuid, _ := r2.ReadU64()
	if pGuid != 0 {
		t.Fatalf("expected pet GUID 0 in dismiss SMSG_PET_SPELLS, got %x", pGuid)
	}

	// Should receive player update clearing unitFieldSummon
	op3, _, err := readServerFrame(clientConn, nil)
	if err != nil || !isUpdateOpcode(op3) {
		t.Fatalf("expected update player update, got op=0x%x err=%v", op3, err)
	}

	if sess.player.PetGUID != 0 {
		t.Fatalf("expected player.PetGUID to be cleared after dismiss")
	}

	// Verify slot is now 100 (petSaveNotInSlot)
	var slot int64
	_ = cdb.QueryRow("SELECT slot FROM character_pet WHERE id = 55").Scan(&slot)
	if slot != 100 {
		t.Errorf("expected slot 100 in character_pet, got %d", slot)
	}

	// 2. Call Pet (spell 883, entry 0)
	go func() {
		sess.handleSummonPet(ctx, 883, 0)
	}()

	// Pet spawn update
	op4, _, _ := readServerFrame(clientConn, nil)
	if !isUpdateOpcode(op4) {
		t.Fatalf("expected pet spawn update on Call Pet, got 0x%x", op4)
	}
	// Player update
	op5, _, _ := readServerFrame(clientConn, nil)
	if !isUpdateOpcode(op5) {
		t.Fatalf("expected player update on Call Pet, got 0x%x", op5)
	}
	// SMSG_PET_SPELLS
	op6, _, _ := readServerFrame(clientConn, nil)
	if op6 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS on Call Pet, got 0x%x", op6)
	}

	// Verify pet is back in slot 0 and PetGUID is set
	_ = cdb.QueryRow("SELECT slot FROM character_pet WHERE id = 55").Scan(&slot)
	if slot != 0 {
		t.Errorf("expected slot 0 after Call Pet, got %d", slot)
	}
	if sess.player.PetGUID == 0 {
		t.Errorf("expected active PetGUID after Call Pet")
	}
}

func TestHunter_TamePetAndFeed(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	srv := &Server{CharactersStore: cStore, WorldStore: wStore, Config: config.Default()}
	sess := &session{
		server:       srv,
		conn:         serverConn,
		playerGUID:   1003,
		playerLoaded: true,
		player:       &playerState{GUID: 1003, Name: "Legolas", Level: 10, Class: 3, Race: 4}, // Hunter
	}
	ctx := context.Background()

	// Wild creature GUID (entry 303, low guid 77)
	wildGUID := creatureWorldGUID(77, 303)

	// 1. Tame Creature (spell 1515)
	go func() {
		sess.handleTameCreature(ctx, 1515, wildGUID)
	}()

	// Expect SMSG_DESTROY_OBJECT for the tamed creature
	op1, d1, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_DESTROY_OBJECT) {
		t.Fatalf("expected SMSG_DESTROY_OBJECT for tamed creature, got op=0x%x err=%v", op1, err)
	}
	rd1 := protocol.NewReader(d1)
	despawnedGUID, _ := rd1.ReadU64()
	if despawnedGUID != wildGUID {
		t.Errorf("expected despawned GUID %x, got %x", wildGUID, despawnedGUID)
	}

	// Expect pet spawn update
	op2, _, _ := readServerFrame(clientConn, nil)
	if !isUpdateOpcode(op2) {
		t.Fatalf("expected pet spawn update, got 0x%x", op2)
	}
	// Expect player update
	op3, _, _ := readServerFrame(clientConn, nil)
	if !isUpdateOpcode(op3) {
		t.Fatalf("expected player update, got 0x%x", op3)
	}
	// Expect SMSG_PET_SPELLS
	op4, _, _ := readServerFrame(clientConn, nil)
	if op4 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS, got 0x%x", op4)
	}

	// Verify pet created in character_pet with PetType = 1
	var petID, pType, level int64
	var name string
	err = cdb.QueryRow("SELECT id, PetType, level, name FROM character_pet WHERE owner = 1003").Scan(&petID, &pType, &level, &name)
	if err != nil || pType != 1 || name != "Boar" {
		t.Fatalf("expected Hunter pet 'Boar' (PetType=1), got err=%v type=%d name='%s'", err, pType, name)
	}

	// Verify default Hunter pet spells learned (Growl Rank 2 = 14916 for level 10, Cower = 1742)
	var growlCount, cowerCount int64
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = ? AND spell = 14916", petID).Scan(&growlCount)
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = ? AND spell = 1742", petID).Scan(&cowerCount)
	if growlCount != 1 || cowerCount != 1 {
		t.Errorf("expected Growl Rank 2 and Cower in pet_spell for tamed pet (growl=%d, cower=%d)", growlCount, cowerCount)
	}

	// 2. Feed Pet (handleFeedPet)
	sess.handleFeedPet(ctx, 5149)
	var happiness int64
	_ = cdb.QueryRow("SELECT curhappiness FROM character_pet WHERE id = ?", petID).Scan(&happiness)
	if happiness <= 0 {
		t.Errorf("expected happiness > 0 after feed pet, got %d", happiness)
	}
}

func TestPet_LevelUpSync(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	srv := &Server{CharactersStore: cStore, WorldStore: wStore, Config: config.Default()}
	sess := &session{
		server:       srv,
		conn:         serverConn,
		playerGUID:   1004,
		playerLoaded: true,
		player:       &playerState{GUID: 1004, Name: "WarlockGuy", Level: 1, Class: 9, Race: 1},
	}
	ctx := context.Background()

	// Player has Imp at level 1 with Firebolt Rank 1 (3110)
	_, _ = cdb.Exec("INSERT INTO character_pet (id, entry, owner, level, slot) VALUES (88, 416, 1004, 1, 0)")
	_, _ = cdb.Exec("INSERT INTO pet_spell (guid, spell, active) VALUES (88, 3110, 1)")
	sess.player.PetGUID = uint64(88) | (uint64(0xF140) << 48)

	// Player levels up to Level 8
	sess.player.Level = 8

	go func() {
		sess.updatePetOnLevelUp(ctx)
	}()

	// 1. Should receive SMSG_PET_UNLEARNED_SPELL for Firebolt Rank 1 (3110)
	op1, d1, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL) {
		t.Fatalf("expected SMSG_PET_UNLEARNED_SPELL, got op=0x%x err=%v", op1, err)
	}
	rd1 := protocol.NewReader(d1)
	unlearnSpell, _ := rd1.ReadU32()
	if unlearnSpell != 3110 {
		t.Errorf("expected unlearned spell 3110, got %d", unlearnSpell)
	}

	// 2. Should receive SMSG_PET_LEARNED_SPELL for Firebolt Rank 2 (7799)
	op2, d2, err := readServerFrame(clientConn, nil)
	if err != nil || op2 != uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL) {
		t.Fatalf("expected SMSG_PET_LEARNED_SPELL, got op=0x%x err=%v", op2, err)
	}
	rd2 := protocol.NewReader(d2)
	learnSpell1, _ := rd2.ReadU32()
	if learnSpell1 != 7799 {
		t.Errorf("expected learned spell 7799 (Firebolt rank 2), got %d", learnSpell1)
	}

	// 3. Should receive SMSG_PET_LEARNED_SPELL for Blood Pact Rank 1 (6307)
	op3, d3, err := readServerFrame(clientConn, nil)
	if err != nil || op3 != uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL) {
		t.Fatalf("expected SMSG_PET_LEARNED_SPELL for Blood Pact, got op=0x%x err=%v", op3, err)
	}
	rd3 := protocol.NewReader(d3)
	learnSpell2, _ := rd3.ReadU32()
	if learnSpell2 != 6307 {
		t.Errorf("expected learned spell 6307 (Blood Pact), got %d", learnSpell2)
	}

	// 4. SMSG_PET_SPELLS update
	op4, _, err := readServerFrame(clientConn, nil)
	if err != nil || op4 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS, got op=0x%x err=%v", op4, err)
	}

	// Verify pet_spell has 7799 and 6307, but NOT 3110
	var rank1Count, rank2Count int64
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = 88 AND spell = 3110").Scan(&rank1Count)
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = 88 AND spell = 7799").Scan(&rank2Count)
	if rank1Count != 0 {
		t.Errorf("expected old rank 1 (3110) deleted from pet_spell")
	}
	if rank2Count != 1 {
		t.Errorf("expected new rank 2 (7799) in pet_spell")
	}
}

func TestPet_Abandon(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	srv := &Server{CharactersStore: cStore, WorldStore: wStore, Config: config.Default()}
	sess := &session{
		server:       srv,
		conn:         serverConn,
		playerGUID:   1005,
		playerLoaded: true,
		player:       &playerState{GUID: 1005, Name: "HunterAbandon", Level: 20, Class: 3, Race: 4},
	}
	ctx := context.Background()

	_, _ = cdb.Exec("INSERT INTO character_pet (id, entry, owner, slot) VALUES (99, 303, 1005, 0)")
	_, _ = cdb.Exec("INSERT INTO pet_spell (guid, spell) VALUES (99, 2649)")
	sess.player.PetGUID = uint64(99) | (uint64(0xF140) << 48)

	// Abandon pet
	go func() {
		sess.handlePetAbandon(ctx, nil)
	}()

	// 1. SMSG_DESTROY_OBJECT
	op1, _, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_DESTROY_OBJECT) {
		t.Fatalf("expected SMSG_DESTROY_OBJECT on abandon, got op=0x%x err=%v", op1, err)
	}

	// 2. SMSG_PET_SPELLS with 0
	op2, d2, err := readServerFrame(clientConn, nil)
	if err != nil || op2 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS on abandon, got op=0x%x err=%v", op2, err)
	}
	r2 := protocol.NewReader(d2)
	pGuid, _ := r2.ReadU64()
	if pGuid != 0 {
		t.Fatalf("expected pet GUID 0 in abandon SMSG_PET_SPELLS, got %x", pGuid)
	}

	// 3. Player update
	op3, _, err := readServerFrame(clientConn, nil)
	if err != nil || !isUpdateOpcode(op3) {
		t.Fatalf("expected player update on abandon, got op=0x%x err=%v", op3, err)
	}

	// Verify database: character_pet and pet_spell rows deleted
	var pCount, sCount int64
	_ = cdb.QueryRow("SELECT COUNT(1) FROM character_pet WHERE id = 99").Scan(&pCount)
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = 99").Scan(&sCount)
	if pCount != 0 || sCount != 0 {
		t.Errorf("expected character_pet and pet_spell to be deleted, got pCount=%d sCount=%d", pCount, sCount)
	}
}

func TestPet_TalentsLearnAndPreview(t *testing.T) {
	cdb, wdb := setupPetTestDatabases(t)
	defer cdb.Close()
	defer wdb.Close()

	dir := t.TempDir()
	writeTalentDBC(t, dir, 201, 1, 0, 0, [5]uint32{53478, 53479, 0, 0, 0}, 0, 0)
	store := wotlk.NewStore(dir)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	cStore := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: cdb}
	wStore := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: wdb}
	srv := &Server{CharactersStore: cStore, WorldStore: wStore, Data: store, Config: config.Default()}
	sess := &session{
		server:       srv,
		conn:         serverConn,
		playerGUID:   1006,
		playerLoaded: true,
		player:       &playerState{GUID: 1006, Name: "HunterTalents", Level: 20, Class: 3, Race: 4},
	}
	ctx := context.Background()

	_, _ = cdb.Exec("INSERT INTO character_pet (id, entry, owner, slot, PetType, level) VALUES (70, 303, 1006, 0, 1, 20)")
	petGUID := uint64(70) | (uint64(0xF140) << 48)
	sess.player.PetGUID = petGUID

	// 1. Learn talent 201 rank 0
	learnBuf := protocol.NewBuffer(16)
	learnBuf.WriteU64(petGUID)
	learnBuf.WriteU32(201) // talentID
	learnBuf.WriteU32(0)   // rank 0

	go func() {
		sess.handlePetLearnTalent(ctx, learnBuf.Bytes())
	}()

	// Expect SMSG_PET_LEARNED_SPELL with 53478
	op1, d1, err := readServerFrame(clientConn, nil)
	if err != nil || op1 != uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL) {
		t.Fatalf("expected SMSG_PET_LEARNED_SPELL, got op=0x%x err=%v", op1, err)
	}
	rd1 := protocol.NewReader(d1)
	sp1, _ := rd1.ReadU32()
	if sp1 != 53478 {
		t.Errorf("expected learned spell 53478, got %d", sp1)
	}

	// Expect SMSG_TALENTS_INFO (pet=true)
	op2, _, err := readServerFrame(clientConn, nil)
	if err != nil || op2 != uint16(protocol.OpcodeSMSG_TALENTS_INFO) {
		t.Fatalf("expected SMSG_TALENTS_INFO, got op=0x%x err=%v", op2, err)
	}

	// Expect SMSG_PET_SPELLS
	op3, _, err := readServerFrame(clientConn, nil)
	if err != nil || op3 != uint16(protocol.OpcodeSMSG_PET_SPELLS) {
		t.Fatalf("expected SMSG_PET_SPELLS, got op=0x%x err=%v", op3, err)
	}

	// Verify pet_spell has 53478
	var hasRank0 int64
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = 70 AND spell = 53478").Scan(&hasRank0)
	if hasRank0 != 1 {
		t.Errorf("expected 53478 in pet_spell")
	}

	// 2. Learn talent 201 rank 1 (should replace rank 0)
	learnBuf2 := protocol.NewBuffer(16)
	learnBuf2.WriteU64(petGUID)
	learnBuf2.WriteU32(201)
	learnBuf2.WriteU32(1)

	go func() {
		sess.handlePetLearnTalent(ctx, learnBuf2.Bytes())
	}()

	// Expect SMSG_PET_UNLEARNED_SPELL with 53478
	op4, d4, err := readServerFrame(clientConn, nil)
	if err != nil || op4 != uint16(protocol.OpcodeSMSG_PET_UNLEARNED_SPELL) {
		t.Fatalf("expected SMSG_PET_UNLEARNED_SPELL, got op=0x%x err=%v", op4, err)
	}
	rd4 := protocol.NewReader(d4)
	unsp, _ := rd4.ReadU32()
	if unsp != 53478 {
		t.Errorf("expected unlearned spell 53478, got %d", unsp)
	}

	// Expect SMSG_PET_LEARNED_SPELL with 53479
	op5, d5, err := readServerFrame(clientConn, nil)
	if err != nil || op5 != uint16(protocol.OpcodeSMSG_PET_LEARNED_SPELL) {
		t.Fatalf("expected SMSG_PET_LEARNED_SPELL, got op=0x%x err=%v", op5, err)
	}
	rd5 := protocol.NewReader(d5)
	sp2, _ := rd5.ReadU32()
	if sp2 != 53479 {
		t.Errorf("expected learned spell 53479, got %d", sp2)
	}

	// Read TALENTS_INFO and PET_SPELLS
	_, _, _ = readServerFrame(clientConn, nil)
	_, _, _ = readServerFrame(clientConn, nil)

	// Verify pet_spell has 53479 and not 53478
	var hasRank1 int64
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = 70 AND spell = 53478").Scan(&hasRank0)
	_ = cdb.QueryRow("SELECT COUNT(1) FROM pet_spell WHERE guid = 70 AND spell = 53479").Scan(&hasRank1)
	if hasRank0 != 0 || hasRank1 != 1 {
		t.Errorf("expected rank 1 in pet_spell and rank 0 removed (r0=%d, r1=%d)", hasRank0, hasRank1)
	}
}
