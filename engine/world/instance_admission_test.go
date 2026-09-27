package world

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
	_ "modernc.org/sqlite"
)

func admissionTestData(t *testing.T) (*wotlk.Store, uint32) {
	t.Helper()
	dir := filepath.Join("..", "..", "bin", "data", "dbc")
	if _, err := os.Stat(filepath.Join(dir, "MapDifficulty.dbc")); err != nil {
		t.Skip("MapDifficulty.dbc is unavailable")
	}
	data := wotlk.NewStore(dir)
	difficulty, found, err := data.MapDifficulty(33, 0)
	if err != nil || !found || difficulty.MaxPlayers == 0 {
		t.Skipf("MapDifficulty.dbc has no five-player row for map 33: found=%t err=%v", found, err)
	}
	return data, difficulty.MaxPlayers
}

func addAdmissionPeer(server *Server, guid uint64, mapID, instanceID uint32) *session {
	peer := &session{server: server, playerGUID: guid, player: &playerState{Map: mapID, InstanceID: instanceID}}
	peer.worldReady.Store(true)
	peer.worldInstance.Store(uint64(mapID)<<32 | uint64(instanceID))
	server.sessionsMu.Lock()
	if server.sessions == nil {
		server.sessions = make(map[*session]struct{})
	}
	server.sessions[peer] = struct{}{}
	server.sessionsMu.Unlock()
	return peer
}

func TestWorldportAdmissionCountsPlayersBotsAndReservations(t *testing.T) {
	data, maxPlayers := admissionTestData(t)
	const mapID, instanceID = uint32(33), uint32(41001)
	entry := wotlk.MapEntry{ID: mapID, InstanceType: 1, MaxPlayers: maxPlayers}
	selection := worldportInstanceSelection{MapID: mapID, InstanceID: instanceID, Difficulty: 0}
	t.Run("pending entrant reserves final place", func(t *testing.T) {
		server := &Server{Data: data}
		for i := uint32(1); i < maxPlayers; i++ {
			addAdmissionPeer(server, uint64(i), mapID, instanceID)
		}
		first := &session{server: server, player: &playerState{}}
		second := &session{server: server, player: &playerState{}}
		if !first.worldportInstanceHasRoom(selection, entry) {
			t.Fatal("first entrant was denied while one place remained")
		}
		if second.worldportInstanceHasRoom(selection, entry) {
			t.Fatal("second entrant was allowed to exceed capacity")
		}
	})
	t.Run("active owned bot counts only when configured", func(t *testing.T) {
		const botEntry uint32 = 900001
		manager := &NPCBotManager{extras: map[uint32]NpcBotExtras{botEntry: {}}}
		server := &Server{Data: data, Config: config.Config{NPCBots: config.NPCBotConfig{LimitDungeon: true}}, Features: &Features{NPCBots: manager}}
		owner := addAdmissionPeer(server, 100, mapID, instanceID)
		for i := uint32(0); i < maxPlayers-2; i++ {
			addAdmissionPeer(server, uint64(101+i), mapID, instanceID)
		}
		server.motionMu.Lock()
		motions := server.motionMapLocked(mapID, instanceID)
		motions[1] = &creatureMotion{GUID: 1, Entry: botEntry, OwnerGUID: owner.playerGUID}
		motions[2] = &creatureMotion{GUID: 2, Entry: botEntry, OwnerGUID: owner.playerGUID, PetID: 1}
		motions[3] = &creatureMotion{GUID: 3, Entry: botEntry, OwnerGUID: owner.playerGUID + 1}
		server.motionMu.Unlock()
		candidate := &session{server: server, player: &playerState{}}
		if !candidate.worldportInstanceAtCapacity(selection, entry) {
			t.Fatal("active owned NPCBot did not fill the final instance place")
		}
		server.Config.NPCBots.LimitDungeon = false
		if candidate.worldportInstanceAtCapacity(selection, entry) {
			t.Fatal("unlimited NPCBots were incorrectly counted against instance capacity")
		}
	})
}

func TestFullInstanceDenialSendsTransferAbort(t *testing.T) {
	trace := protocoltrace.NewRecorder("instance-denial-test")
	s := &session{server: &Server{TraceRecorder: trace}, player: &playerState{Health: 100}}
	s.sendAreaTriggerEntryFailure(context.Background(), 33, mapEntryCheck{Reason: mapEntryMaxPlayers})
	events := trace.Snapshot().Events
	if len(events) != 1 || events[0].Opcode != uint32(protocol.OpcodeSMSG_TRANSFER_ABORTED) {
		t.Fatalf("denial packets=%v", events)
	}
	payload, err := base64.StdEncoding.DecodeString(events[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	reader := protocol.NewReader(payload)
	mapID, _ := reader.ReadU32()
	reason, _ := reader.ReadU8()
	if mapID != 33 || reason != transferAbortMaxPlayers || len(payload) != 5 {
		t.Fatalf("transfer-abort payload=%x map=%d reason=%d", payload, mapID, reason)
	}
}

func newInstanceBindTestSession(t *testing.T) (*session, *protocoltrace.Recorder) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range []string{
		"CREATE TABLE instance (id INTEGER PRIMARY KEY, resettime INTEGER NOT NULL, completedEncounters INTEGER NOT NULL)",
		"INSERT INTO instance (id, resettime, completedEncounters) VALUES (41002, 4102444800, 37)",
		"CREATE TABLE character_instance (guid INTEGER NOT NULL, instance INTEGER NOT NULL, permanent INTEGER NOT NULL, extendState INTEGER NOT NULL)",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	trace := protocoltrace.NewRecorder("instance-admission-test")
	server := &Server{CharactersStore: &database.Store{DB: db, Backend: database.BackendSQLite}, TraceRecorder: trace}
	s := &session{server: server, playerLoaded: true, playerGUID: 22, player: &playerState{Map: 532, InstanceID: 41002, Health: 100}}
	return s, trace
}

func TestPermanentGroupLockWarningAcceptDeclineAndTimeout(t *testing.T) {
	ctx := context.Background()
	selection := worldportInstanceSelection{MapID: 532, InstanceID: 41002, Difficulty: 2, GroupPermanent: true}
	t.Run("accept stores permanent bind after warning", func(t *testing.T) {
		s, trace := newInstanceBindTestSession(t)
		s.sendWorldportGroupLockWarning(ctx, selection)
		if s.pendingBindTimer != 60000 || s.pendingBindInstanceID != uint64(selection.InstanceID) {
			t.Fatal("60-second pending bind was not set")
		}
		warningPayload, err := base64.StdEncoding.DecodeString(trace.Snapshot().Events[0].Payload)
		if err != nil {
			t.Fatal(err)
		}
		reader := protocol.NewReader(warningPayload)
		timer, _ := reader.ReadU32()
		encounters, _ := reader.ReadU32()
		extend, _ := reader.ReadU8()
		if timer != 60000 || encounters != 37 || extend != 0 {
			t.Fatalf("warning payload timer=%d encounters=%d extend=%d", timer, encounters, extend)
		}
		if !s.handleInstanceLockResponse(ctx, []byte{1}) {
			t.Fatal("accept response failed")
		}
		var permanent uint32
		if err := s.server.CharactersStore.DB.QueryRow("SELECT permanent FROM character_instance WHERE guid = ? AND instance = ?", s.playerGUID, selection.InstanceID).Scan(&permanent); err != nil || permanent != 1 {
			t.Fatalf("accepted permanent bind=%d err=%v", permanent, err)
		}
		events := trace.Snapshot().Events
		want := []uint32{uint32(protocol.OpcodeSMSG_INSTANCE_LOCK_WARNING_QUERY), uint32(protocol.OpcodeSMSG_INSTANCE_SAVE_CREATED), uint32(protocol.OpcodeSMSG_CALENDAR_RAID_LOCKOUT_ADDED)}
		if len(events) != len(want) {
			t.Fatalf("packet count=%d, want %d", len(events), len(want))
		}
		for i, opcode := range want {
			if events[i].Opcode != opcode {
				t.Fatalf("packet[%d]=%#x, want %#x", i, events[i].Opcode, opcode)
			}
		}
	})
	t.Run("decline creates no bind", func(t *testing.T) {
		s, _ := newInstanceBindTestSession(t)
		s.sendWorldportGroupLockWarning(ctx, selection)
		s.handleInstanceLockResponse(ctx, []byte{0})
		var count int
		if err := s.server.CharactersStore.DB.QueryRow("SELECT COUNT(*) FROM character_instance WHERE guid = ? AND instance = ?", s.playerGUID, selection.InstanceID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("declined permanent bind count=%d err=%v", count, err)
		}
		if _, _, _, pending := s.takePendingBind(); pending {
			t.Fatal("declined warning remained pending")
		}
	})
	t.Run("timeout binds only while still inside", func(t *testing.T) {
		s, _ := newInstanceBindTestSession(t)
		s.sendWorldportGroupLockWarning(ctx, selection)
		s.worldReady.Store(true)
		s.server.sessions = map[*session]struct{}{s: {}}
		s.server.updatePendingInstanceBinds(ctx, 61*time.Second)
		var permanent uint32
		if err := s.server.CharactersStore.DB.QueryRow("SELECT permanent FROM character_instance WHERE guid = ? AND instance = ?", s.playerGUID, selection.InstanceID).Scan(&permanent); err != nil || permanent != 1 {
			t.Fatalf("timed-out warning permanent bind=%d err=%v", permanent, err)
		}
		if _, _, _, pending := s.takePendingBind(); pending {
			t.Fatal("expired bind remained pending")
		}
	})
	t.Run("timeout after leaving does not bind", func(t *testing.T) {
		s, _ := newInstanceBindTestSession(t)
		s.sendWorldportGroupLockWarning(ctx, selection)
		s.worldReady.Store(true)
		s.player.InstanceID++
		s.server.sessions = map[*session]struct{}{s: {}}
		s.server.updatePendingInstanceBinds(ctx, 61*time.Second)
		var count int
		if err := s.server.CharactersStore.DB.QueryRow("SELECT COUNT(*) FROM character_instance WHERE guid = ? AND instance = ?", s.playerGUID, selection.InstanceID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("left-instance permanent bind count=%d err=%v", count, err)
		}
		if _, _, _, pending := s.takePendingBind(); pending {
			t.Fatal("left-instance bind remained pending")
		}
	})
}
