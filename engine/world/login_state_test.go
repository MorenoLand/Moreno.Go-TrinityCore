package world

import (
	"context"
	"database/sql"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func TestSendNewMailNotificationForUnreadDueMail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE mail (receiver INTEGER, deliver_time INTEGER, expire_time INTEGER, checked INTEGER)"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := db.Exec("INSERT INTO mail VALUES (9, ?, ?, 0), (9, ?, ?, 1), (9, ?, ?, 0)", now-1, now+3600, now-1, now+3600, now+3600, now+7200); err != nil {
		t.Fatal(err)
	}
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{server: &Server{CharactersStore: store}, conn: serverConn, playerGUID: 9}
	done := make(chan struct{})
	go func() {
		sess.sendNewMailNotification(context.Background())
		close(done)
	}()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	<-done
	if opcode != uint16(protocol.OpcodeSMSG_RECEIVED_MAIL) || len(payload) != 4 || binary.LittleEndian.Uint32(payload) != 0 {
		t.Fatalf("opcode=%x payload=%x", opcode, payload)
	}
}

func TestSendNewMailNotificationSkipsReadAndFutureMail(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE mail (receiver INTEGER, deliver_time INTEGER, expire_time INTEGER, checked INTEGER)"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := db.Exec("INSERT INTO mail VALUES (9, ?, ?, 1), (9, ?, ?, 0)", now-1, now+3600, now+3600, now+7200); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	sess := &session{server: &Server{CharactersStore: store}, conn: serverConn, playerGUID: 9}
	done := make(chan struct{})
	go func() {
		sess.sendNewMailNotification(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("mail notification blocked without a due unread message")
	}
}

func TestLoadMailStateTracksFutureDelivery(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE mail (receiver INTEGER, deliver_time INTEGER, expire_time INTEGER, checked INTEGER)"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := db.Exec("INSERT INTO mail VALUES (9, ?, ?, 0)", now+120, now+7200); err != nil {
		t.Fatal(err)
	}
	sess := &session{server: &Server{CharactersStore: &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}}, playerGUID: 9}
	sess.loadMailState(context.Background())
	if sess.unreadMails != 0 || sess.nextMailDelivery < now+119 || sess.nextMailDelivery > now+121 {
		t.Fatalf("mail state unread=%d next=%d now=%d", sess.unreadMails, sess.nextMailDelivery, now)
	}
}

func TestLoadAndSendPersistentAura(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE character_aura (
		guid INTEGER, casterGuid INTEGER, itemGuid INTEGER, spell INTEGER, effectMask INTEGER,
		stackCount INTEGER, amount0 INTEGER, maxDuration INTEGER, remainTime INTEGER, remainCharges INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO character_aura VALUES (9, 0, 0, 123, 1, 2, 50, 60000, 30000, 0)"); err != nil {
		t.Fatal(err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	sess := &session{server: &Server{CharactersStore: store}, conn: serverConn, playerGUID: 9}
	state := &playerState{GUID: 9, Level: 20}
	if err := sess.loadPlayerAuras(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	aura, ok := sess.activeAuras[123]
	if !ok || aura.CasterGUID != 9 || aura.RemainingMs != 30000 || aura.DurationMs != 60000 || aura.Amount != 50 || aura.StackCount != 2 {
		t.Fatalf("aura=%+v present=%v", aura, ok)
	}
	go sess.sendLoadedAuras()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeSMSG_AURA_UPDATE_ALL) || len(payload) == 0 {
		t.Fatalf("opcode=%x payload=%x", opcode, payload)
	}
	sess.clearActiveAuras()
}

func TestLoadGhostAuraRestoresGhostFlag(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE character_aura (
		guid INTEGER, casterGuid INTEGER, itemGuid INTEGER, spell INTEGER, effectMask INTEGER,
		stackCount INTEGER, amount0 INTEGER, maxDuration INTEGER, remainTime INTEGER, remainCharges INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO character_aura VALUES (9, 9, 0, 8326, 1, 1, 0, -1, -1, 0)"); err != nil {
		t.Fatal(err)
	}
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db}
	sess := &session{server: &Server{CharactersStore: store}, playerGUID: 9}
	state := &playerState{GUID: 9, Level: 20}
	if err := sess.loadPlayerAuras(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if state.PlayerFlags&playerFlagGhost == 0 {
		t.Fatal("ghost aura did not restore player ghost flag")
	}
}

func TestSendLoginMovementStatesUsesReferenceCompoundPacket(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{conn: serverConn, playerGUID: 9, player: &playerState{GUID: 9}, activeAuras: map[uint32]*activeAura{
		1: {SpellID: 1, AuraType: 26},
		2: {SpellID: 2, AuraType: 104},
		3: {SpellID: 3, AuraType: 105},
		4: {SpellID: 4, AuraType: 106},
		5: {SpellID: 5, AuraType: 12},
	}}
	go func() {
		if err := sess.sendLoginMovementStates(); err != nil {
			t.Error(err)
		}
	}()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeSMSG_MULTIPLE_MOVES) {
		t.Fatalf("opcode=%x", opcode)
	}
	reader := protocol.NewReader(payload)
	size, err := reader.ReadU32()
	if err != nil || int(size) != reader.Remaining() {
		t.Fatalf("compound size=%d remaining=%d err=%v", size, reader.Remaining(), err)
	}
	want := []uint16{uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), uint16(protocol.OpcodeSMSG_MOVE_FEATHER_FALL), uint16(protocol.OpcodeSMSG_MOVE_WATER_WALK), uint16(protocol.OpcodeSMSG_MOVE_SET_HOVER)}
	for _, expected := range want {
		length, err := reader.ReadU8()
		if err != nil || length != uint8(2+packedGUIDSize(9)+4) {
			t.Fatalf("subpacket length=%d err=%v", length, err)
		}
		subOpcode, err := reader.ReadU16()
		if err != nil || subOpcode != expected {
			t.Fatalf("subpacket opcode=%x want=%x err=%v", subOpcode, expected, err)
		}
		guid, err := reader.ReadPackedGUID()
		if err != nil || guid != 9 {
			t.Fatalf("subpacket guid=%d err=%v", guid, err)
		}
		if counter, err := reader.ReadU32(); err != nil || counter != 0 {
			t.Fatalf("subpacket counter=%d err=%v", counter, err)
		}
	}
}

func TestSendLoginFlightStateUsesReferenceCanFlyPacket(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	sess := &session{conn: serverConn, playerGUID: 9, player: &playerState{GUID: 9}, activeAuras: map[uint32]*activeAura{1: {SpellID: 201, AuraType: 201}}}
	done := make(chan error, 1)
	go func() { done <- sess.sendLoginFlightState() }()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeSMSG_MOVE_SET_CAN_FLY) {
		t.Fatalf("opcode=%x", opcode)
	}
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadPackedGUID()
	if err != nil || guid != 9 {
		t.Fatalf("guid=%d err=%v", guid, err)
	}
	if counter, err := reader.ReadU32(); err != nil || counter != 0 || reader.Remaining() != 0 {
		t.Fatalf("counter=%d remaining=%d err=%v", counter, reader.Remaining(), err)
	}
}

func TestBuildForcedReactionsUsesForceReactionAuras(t *testing.T) {
	payload := buildForcedReactions([]*activeAura{{AuraType: 139, MiscValue: 72, Amount: 4}, {AuraType: 139, MiscValue: 47, Amount: 5}})
	reader := protocol.NewReader(payload)
	count, err := reader.ReadU32()
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	faction, _ := reader.ReadU32()
	rank, _ := reader.ReadU32()
	if faction != 47 || rank != 5 {
		t.Fatalf("first reaction faction=%d rank=%d", faction, rank)
	}
	faction, _ = reader.ReadU32()
	rank, _ = reader.ReadU32()
	if faction != 72 || rank != 4 || reader.Remaining() != 0 {
		t.Fatalf("second reaction faction=%d rank=%d remaining=%d", faction, rank, reader.Remaining())
	}
}
