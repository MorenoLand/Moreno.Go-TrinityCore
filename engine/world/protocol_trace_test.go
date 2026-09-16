package world

import (
	"bytes"
	"context"
	"database/sql"
	"net"
	"testing"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
	_ "modernc.org/sqlite"
)

func TestSessionWriteRecordsServerPacket(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	recorder := protocoltrace.NewRecorder("test")
	sess := &session{server: &Server{TraceRecorder: recorder}, conn: serverConn}
	payload := []byte{1, 2, 3}
	done := make(chan error, 1)
	go func() { done <- sess.write(uint16(protocol.OpcodeSMSG_PONG), payload, false) }()
	if _, _, err := readServerFrame(clientConn, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	trace := recorder.Snapshot()
	if len(trace.Events) != 1 {
		t.Fatalf("events=%d", len(trace.Events))
	}
	event := trace.Events[0]
	if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PONG) {
		t.Fatalf("unexpected event: %+v", event)
	}
	got, err := trace.Payload(event)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("payload=%x err=%v", got, err)
	}
}

func TestTraceRoundTripReplaysPacketAndDatabaseSuccessFailure(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	recorder := protocoltrace.NewRecorder("world-success-failure")
	store := &database.Store{Name: "characters", Backend: database.BackendSQLite, DB: db, TraceRecorder: recorder}
	if _, err := store.Exec(context.Background(), "CREATE TABLE trace_state (id INTEGER PRIMARY KEY, value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(context.Background(), "INSERT INTO trace_state (id, value) VALUES (?, ?)", 1, "private"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(context.Background(), "INSERT INTO missing_trace_state VALUES (1)"); err == nil {
		t.Fatal("expected failed database transition")
	}
	sess := &session{server: &Server{TraceRecorder: recorder}}
	if err := sess.write(uint16(protocol.OpcodeSMSG_PONG), []byte{1, 2, 3}, false); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := recorder.Snapshot().Write(&encoded); err != nil {
		t.Fatal(err)
	}
	trace, err := protocoltrace.Load(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	sink := &traceReplaySink{}
	if err := trace.Replay(sink); err != nil {
		t.Fatal(err)
	}
	if sink.events != 4 || sink.databaseEvents != 3 || !sink.failedDatabaseEvent || !sink.packetEvent {
		t.Fatalf("replayed events=%d database=%d failedDatabase=%v packet=%v", sink.events, sink.databaseEvents, sink.failedDatabaseEvent, sink.packetEvent)
	}
}

type traceReplaySink struct {
	events              int
	databaseEvents      int
	failedDatabaseEvent bool
	packetEvent         bool
}

func (s *traceReplaySink) Handle(direction protocoltrace.Direction, opcode uint32, payload []byte, state string) error {
	s.events++
	if state == "database" {
		s.databaseEvents++
		if bytes.Contains(payload, []byte(`"error"`)) {
			s.failedDatabaseEvent = true
		}
	} else if direction == protocoltrace.ServerToClient && opcode == uint32(protocol.OpcodeSMSG_PONG) && bytes.Equal(payload, []byte{1, 2, 3}) {
		s.packetEvent = true
	}
	return nil
}
