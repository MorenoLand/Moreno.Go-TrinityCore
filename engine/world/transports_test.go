package world

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	_ "modernc.org/sqlite"
)

func TestContinentTransportLoadsRouteAndMoves(t *testing.T) {
	dbcDir := t.TempDir()
	const fieldCount = 9
	records := make([]byte, fieldCount*4*2)
	writeNode := func(index int, id, path, node, mapID uint32, x, y float32) {
		offset := index * fieldCount * 4
		values := make([]uint32, fieldCount)
		values[0], values[1], values[2], values[3] = id, path, node, mapID
		values[4], values[5] = math.Float32bits(x), math.Float32bits(y)
		for field, value := range values {
			binary.LittleEndian.PutUint32(records[offset+field*4:], value)
		}
	}
	writeNode(0, 1, 10, 0, 0, 0, 0)
	writeNode(1, 2, 10, 1, 0, 10, 0)
	header := make([]byte, 20)
	copy(header, "WDBC")
	binary.LittleEndian.PutUint32(header[4:8], 2)
	binary.LittleEndian.PutUint32(header[8:12], fieldCount)
	binary.LittleEndian.PutUint32(header[12:16], fieldCount*4)
	binary.LittleEndian.PutUint32(header[16:20], 1)
	if err := os.WriteFile(filepath.Join(dbcDir, "TaxiPathNode.dbc"), append(header, append(records, 0)...), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`CREATE TABLE transports (guid INTEGER PRIMARY KEY, entry INTEGER NOT NULL, name TEXT NOT NULL DEFAULT '', ScriptName TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE gameobject_template (entry INTEGER PRIMARY KEY, type INTEGER NOT NULL, name TEXT NOT NULL, data0 INTEGER NOT NULL, data1 INTEGER NOT NULL, displayId INTEGER NOT NULL, size REAL NOT NULL)`,
		`INSERT INTO transports VALUES (7, 9000, 'Test Ferry', '')`,
		`INSERT INTO gameobject_template VALUES (9000, 15, 'Test Ferry', 10, 10, 1234, 1)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	server := &Server{WorldStore: &database.Store{Name: "world", Backend: database.BackendSQLite, DB: db}, Data: wotlk.NewStore(dbcDir), Config: config.Default(), transports: make(map[uint32]*continentTransport), sessions: make(map[*session]struct{})}
	server.loadContinentTransports(context.Background())
	transport := server.transports[7]
	if transport == nil || len(transport.Points) != 2 || transport.Spawn.Type != GameObjectTypeMOTransport {
		t.Fatalf("transport=%+v", transport)
	}
	server.updateContinentTransports(transport.LastUpdate.Add(500 * time.Millisecond))
	if transport.Spawn.X < 4.9 || transport.Spawn.X > 5.1 {
		t.Fatalf("transport x=%v, want midpoint", transport.Spawn.X)
	}
	create := buildGameObjectUpdate(transport.Spawn)
	reader := protocol.NewReader(create)
	updateType, err := reader.ReadU8()
	if err != nil || updateType != protocol.UpdateCreateObject2 {
		t.Fatalf("create update type=%d err=%v", updateType, err)
	}
	if _, err := reader.ReadPackedGUID(); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadU8(); err != nil {
		t.Fatal(err)
	}
	flags, err := reader.ReadU16()
	if err != nil || flags != transportGameObjectUpdateFlags {
		t.Fatalf("transport flags=%x err=%v", flags, err)
	}
	movement := buildGameObjectMovementUpdate(transport.Spawn)
	if len(movement) == 0 || movement[0] != protocol.UpdateMovement {
		t.Fatalf("movement update=%x", movement)
	}
}

func TestContinentTransportMovesPassengerFromOffsets(t *testing.T) {
	server := &Server{Config: config.Default(), sessions: make(map[*session]struct{})}
	sess := &session{server: server, authed: true, playerLoaded: true, player: &playerState{GUID: 9, Map: 0, TransportGUID: gameObjectGUID(7, 9000), TransportX: 1, TransportY: 2, TransportZ: 3, TransportO: 0.5}}
	server.sessions[sess] = struct{}{}
	change := continentTransportMovement{OldSpawn: gameObjectSpawn{GUID: 7, Entry: 9000, Map: 0, X: 10, Y: 10, Z: 20, Orientation: 0}, Spawn: gameObjectSpawn{GUID: 7, Entry: 9000, Map: 0, X: 20, Y: 30, Z: 40, Orientation: math.Pi / 2}}
	server.broadcastTransportMovement(change)
	x, y, z, o := CalculatePassengerPosition(change.Spawn.X, change.Spawn.Y, change.Spawn.Z, change.Spawn.Orientation, 1, 2, 3, 0.5)
	if sess.player.X != x || sess.player.Y != y || sess.player.Z != z || sess.player.Orientation != o {
		t.Fatalf("passenger position=(%v,%v,%v,%v) want=(%v,%v,%v,%v)", sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, x, y, z, o)
	}
}

func TestContinentTransportTeleportsPassengerAcrossMaps(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	server := &Server{Config: config.Default(), sessions: make(map[*session]struct{})}
	sess := &session{server: server, conn: serverConn, authed: true, playerLoaded: true, player: &playerState{GUID: 9, Map: 0, TransportGUID: gameObjectGUID(7, 9000), TransportX: 1, TransportY: 2, TransportZ: 3, TransportO: 0.5}}
	server.sessions[sess] = struct{}{}
	change := continentTransportMovement{OldSpawn: gameObjectSpawn{GUID: 7, Entry: 9000, Map: 0, X: 10, Y: 10, Z: 20, Orientation: 0}, Spawn: gameObjectSpawn{GUID: 7, Entry: 9000, Map: 1, X: 20, Y: 30, Z: 40, Orientation: math.Pi / 2}}
	done := make(chan struct{})
	go func() {
		server.broadcastTransportMovement(change)
		close(done)
	}()
	opcode, payload, err := readServerFrame(clientConn, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != uint16(protocol.OpcodeSMSG_NEW_WORLD) {
		t.Fatalf("opcode=%x", opcode)
	}
	reader := protocol.NewReader(payload)
	mapID, err := reader.ReadU32()
	if err != nil || mapID != 1 {
		t.Fatalf("map=%d err=%v", mapID, err)
	}
	x, err := reader.ReadF32()
	if err != nil {
		t.Fatal(err)
	}
	y, err := reader.ReadF32()
	if err != nil {
		t.Fatal(err)
	}
	z, err := reader.ReadF32()
	if err != nil {
		t.Fatal(err)
	}
	o, err := reader.ReadF32()
	if err != nil {
		t.Fatal(err)
	}
	wantX, wantY, wantZ, wantO := CalculatePassengerPosition(change.Spawn.X, change.Spawn.Y, change.Spawn.Z, change.Spawn.Orientation, 1, 2, 3, 0.5)
	if x != wantX || y != wantY || z != wantZ || o != wantO || sess.player.Map != 1 {
		t.Fatalf("passenger=(%v,%v,%v,%v) map=%d want=(%v,%v,%v,%v) map=1", x, y, z, o, sess.player.Map, wantX, wantY, wantZ, wantO)
	}
	if _, _, err := readServerFrame(clientConn, nil); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestContinentTransportTransformsStaticPassengers(t *testing.T) {
	transport := &continentTransport{Spawn: gameObjectSpawn{GUID: 7, Entry: 9000, Map: 1, X: 20, Y: 30, Z: 40, Orientation: math.Pi / 2}, StaticCreatures: []creatureSpawn{{GUID: 8, Entry: 68, Map: 593, X: 1, Y: 2, Z: 3, Orientation: 0}}, StaticObjects: []gameObjectSpawn{{GUID: 9, Entry: 100, Map: 593, X: 4, Y: 5, Z: 6, Orientation: 0}}}
	creatures := transport.passengerCreatures()
	objects := transport.passengerObjects()
	if len(creatures) != 1 || len(objects) != 1 {
		t.Fatalf("passengers creatures=%d objects=%d", len(creatures), len(objects))
	}
	cx, cy, cz, co := CalculatePassengerPosition(20, 30, 40, math.Pi/2, 1, 2, 3, 0)
	if creatures[0].Map != 1 || creatures[0].TransportGUID != gameObjectGUID(7, 9000) || creatures[0].TransportX != 1 || creatures[0].TransportY != 2 || creatures[0].TransportZ != 3 || creatures[0].TransportO != 0 || creatures[0].X != cx || creatures[0].Y != cy || creatures[0].Z != cz || creatures[0].Orientation != co {
		t.Fatalf("creature passenger=%+v want=(%v,%v,%v,%v)", creatures[0], cx, cy, cz, co)
	}
	ox, oy, oz, oo := CalculatePassengerPosition(20, 30, 40, math.Pi/2, 4, 5, 6, 0)
	if objects[0].Map != 1 || objects[0].TransportGUID != gameObjectGUID(7, 9000) || objects[0].TransportX != 4 || objects[0].TransportY != 5 || objects[0].TransportZ != 6 || objects[0].TransportO != 0 || objects[0].X != ox || objects[0].Y != oy || objects[0].Z != oz || objects[0].Orientation != oo {
		t.Fatalf("object passenger=%+v want=(%v,%v,%v,%v)", objects[0], ox, oy, oz, oo)
	}
}
