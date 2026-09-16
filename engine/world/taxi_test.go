package world

import (
	"context"
	"math"
	"testing"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func TestTaxiResumeStartNodeMatchesReferenceMidpointSearch(t *testing.T) {
	points := []wotlk.TaxiSplinePoint{{MapID: 0, X: 0, Y: 0, Z: 0}, {MapID: 0, X: 10, Y: 0, Z: 0}, {MapID: 0, X: 20, Y: 0, Z: 0}, {MapID: 1, X: 0, Y: 0, Z: 0}}
	if got := taxiResumeStartNode(points, 0, 16, 0, 0); got != 2 {
		t.Fatalf("start node=%d want=2", got)
	}
	if got := taxiResumeStartNode(points, 1, math.SmallestNonzeroFloat32, 0, 0); got != 3 {
		t.Fatalf("cross-map start node=%d want=3", got)
	}
}

func TestTaxiMenuAndActivation(t *testing.T) {
	srv := &Server{}
	sess := &session{server: srv, playerGUID: 1, playerLoaded: true, player: &playerState{GUID: 1}}

	flightMasterGUID := creatureWorldGUID(1, 404)

	// 1. Status query
	statusBuf := protocol.NewBuffer(8)
	statusBuf.WriteU64(flightMasterGUID)
	if !sess.handleTaxiNodeStatusQuery(context.Background(), statusBuf.Bytes()) {
		t.Fatal("handleTaxiNodeStatusQuery failed")
	}

	// 2. Query available nodes
	queryBuf := protocol.NewBuffer(8)
	queryBuf.WriteU64(flightMasterGUID)
	if !sess.handleTaxiQueryAvailableNodes(context.Background(), queryBuf.Bytes()) {
		t.Fatal("handleTaxiQueryAvailableNodes failed")
	}

	// 3. Activate taxi flight (classic 16-byte CMSG_ACTIVATETAXI)
	actBuf := protocol.NewBuffer(16)
	actBuf.WriteU64(flightMasterGUID)
	actBuf.WriteU32(1) // source
	actBuf.WriteU32(2) // dest
	if !sess.handleActivateTaxi(context.Background(), actBuf.Bytes()) {
		t.Fatal("handleActivateTaxi failed")
	}

	// 4. Activate taxi flight express (WotLK 20-byte CMSG_ACTIVATETAXIEXPRESS)
	expBuf := protocol.NewBuffer(20)
	expBuf.WriteU64(flightMasterGUID)
	expBuf.WriteU32(2) // nodeCount = 2
	expBuf.WriteU32(1) // node 1 (source)
	expBuf.WriteU32(2) // node 2 (dest)
	if !sess.handleActivateTaxi(context.Background(), expBuf.Bytes()) {
		t.Fatal("handleActivateTaxi (express) failed")
	}
}
