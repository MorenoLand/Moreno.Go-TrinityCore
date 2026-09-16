package world

import (
	"context"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type continentTransport struct {
	Spawn           gameObjectSpawn
	Name            string
	PathID          uint32
	Speed           float32
	Points          []wotlk.TaxiSplinePoint
	Segment         int
	SegmentProgress float64
	LastUpdate      time.Time
}

type continentTransportMovement struct {
	OldSpawn gameObjectSpawn
	Spawn    gameObjectSpawn
}

func (s *Server) loadContinentTransports(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || s.Data == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT tr.guid, tr.entry, COALESCE(gt.name, ''), COALESCE(gt.data0, 0), COALESCE(gt.data1, 0), COALESCE(gt.displayId, 0), COALESCE(gt.size, 1) FROM transports AS tr JOIN gameobject_template AS gt ON gt.entry = tr.entry WHERE gt.type = 15 ORDER BY tr.guid`)
	if err != nil {
		if !missingTable(err) && s.Logger != nil {
			s.Logger.Warn("continent transport load failed", "error", err)
		}
		return
	}
	defer rows.Close()
	loaded := 0
	now := time.Now()
	for rows.Next() {
		var guid, entry, pathID, displayID int64
		var name string
		var speed, size float64
		if err := rows.Scan(&guid, &entry, &name, &pathID, &speed, &displayID, &size); err != nil || guid <= 0 || entry <= 0 || pathID <= 0 {
			continue
		}
		points, err := s.Data.TaxiPathPoints(uint32(pathID))
		if err != nil || len(points) < 2 {
			if s.Logger != nil {
				s.Logger.Warn("continent transport route unavailable", "entry", entry, "path", pathID, "error", err)
			}
			continue
		}
		if speed <= 0 {
			speed = 30
		}
		if size <= 0 {
			size = 1
		}
		transport := &continentTransport{Spawn: gameObjectSpawn{GUID: uint32(guid), Entry: uint32(entry), Map: transportPointMap(points[0]), X: points[0].X, Y: points[0].Y, Z: points[0].Z, Type: GameObjectTypeMOTransport, DisplayID: uint32(displayID), Size: float32(size), RotationW: 1, ParentRotation: [4]float32{0, 0, 0, 1}}, Name: name, PathID: uint32(pathID), Speed: float32(speed), Points: points, LastUpdate: now}
		transport.updatePosition()
		s.transportMu.Lock()
		if s.transports == nil {
			s.transports = make(map[uint32]*continentTransport)
		}
		s.transports[transport.Spawn.GUID] = transport
		s.transportMu.Unlock()
		loaded++
	}
	if s.Logger != nil {
		s.Logger.Info("continent transports loaded", "count", loaded)
	}
}

func transportPointMap(point wotlk.TaxiSplinePoint) uint32 {
	if point.MapID < 0 {
		return 0
	}
	return uint32(point.MapID)
}

func (t *continentTransport) segmentDuration(index int) float64 {
	if t == nil || len(t.Points) < 2 {
		return 0
	}
	from := t.Points[index%len(t.Points)]
	to := t.Points[(index+1)%len(t.Points)]
	distance := math.Sqrt(float64((to.X-from.X)*(to.X-from.X) + (to.Y-from.Y)*(to.Y-from.Y) + (to.Z-from.Z)*(to.Z-from.Z)))
	speed := float64(t.Speed)
	if speed <= 0 {
		speed = 30
	}
	travel := distance / speed * 1000
	if travel < 250 {
		travel = 250
	}
	return float64(from.Delay)*1000 + travel
}

func (t *continentTransport) updatePosition() {
	if t == nil || len(t.Points) < 2 {
		return
	}
	from := t.Points[t.Segment%len(t.Points)]
	to := t.Points[(t.Segment+1)%len(t.Points)]
	travel := t.segmentDuration(t.Segment) - float64(from.Delay)*1000
	progress := t.SegmentProgress - float64(from.Delay)*1000
	alpha := float64(0)
	if progress > 0 && travel > 0 && from.MapID == to.MapID {
		alpha = progress / travel
		if alpha > 1 {
			alpha = 1
		}
	}
	t.Spawn.Map = transportPointMap(from)
	t.Spawn.X = from.X + float32(float64(to.X-from.X)*alpha)
	t.Spawn.Y = from.Y + float32(float64(to.Y-from.Y)*alpha)
	t.Spawn.Z = from.Z + float32(float64(to.Z-from.Z)*alpha)
	if from.MapID == to.MapID && (to.X != from.X || to.Y != from.Y) {
		t.Spawn.Orientation = float32(math.Atan2(float64(to.Y-from.Y), float64(to.X-from.X)) + math.Pi)
	}
	t.Spawn.TransportProgress = t.pathProgress()
}

func (t *continentTransport) pathProgress() uint32 {
	if t == nil || len(t.Points) < 2 {
		return 0
	}
	var progress, total float64
	for i := range t.Points {
		duration := t.segmentDuration(i)
		total += duration
		if i < t.Segment {
			progress += duration
		}
	}
	progress += t.SegmentProgress
	if total > 0 {
		progress = math.Mod(progress, total)
	}
	if progress < 0 {
		return 0
	}
	return uint32(progress)
}

func (t *continentTransport) advance(now time.Time) bool {
	if t == nil || len(t.Points) < 2 {
		return false
	}
	if t.LastUpdate.IsZero() {
		t.LastUpdate = now
		t.updatePosition()
		return false
	}
	delta := now.Sub(t.LastUpdate).Seconds() * 1000
	t.LastUpdate = now
	if delta <= 0 {
		return false
	}
	if delta > 5000 {
		delta = 5000
	}
	for delta > 0 {
		remaining := t.segmentDuration(t.Segment) - t.SegmentProgress
		if remaining <= 0 {
			t.Segment = (t.Segment + 1) % len(t.Points)
			t.SegmentProgress = 0
			continue
		}
		if delta < remaining {
			t.SegmentProgress += delta
			delta = 0
			break
		}
		delta -= remaining
		t.Segment = (t.Segment + 1) % len(t.Points)
		t.SegmentProgress = 0
	}
	t.updatePosition()
	return true
}

func (s *Server) updateContinentTransports(now time.Time) {
	if s == nil {
		return
	}
	changes := make([]continentTransportMovement, 0)
	s.transportMu.Lock()
	for _, transport := range s.transports {
		if transport == nil {
			continue
		}
		oldSpawn := transport.Spawn
		if transport.advance(now) {
			changes = append(changes, continentTransportMovement{OldSpawn: oldSpawn, Spawn: transport.Spawn})
		}
	}
	s.transportMu.Unlock()
	for _, change := range changes {
		s.broadcastTransportMovement(change)
	}
}

func (s *Server) nearbyTransportSpawns(state playerState, distance float64) []gameObjectSpawn {
	if s == nil {
		return nil
	}
	result := make([]gameObjectSpawn, 0)
	s.transportMu.Lock()
	for _, transport := range s.transports {
		if transport == nil || transport.Spawn.Map != state.Map || math.Hypot(float64(transport.Spawn.X-state.X), float64(transport.Spawn.Y-state.Y)) > distance {
			continue
		}
		result = append(result, transport.Spawn)
	}
	s.transportMu.Unlock()
	return result
}

func (s *Server) broadcastTransportMovement(change continentTransportMovement) {
	if s == nil {
		return
	}
	distance := float64(s.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150
	}
	rawGUID := gameObjectGUID(change.Spawn.GUID, change.Spawn.Entry)
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for sess := range s.sessions {
		if !sess.authed || !sess.playerLoaded || sess.player == nil {
			continue
		}
		if sess.player.TransportGUID == rawGUID && change.OldSpawn.Map == change.Spawn.Map {
			x, y, z, o := CalculatePassengerPosition(change.Spawn.X, change.Spawn.Y, change.Spawn.Z, change.Spawn.Orientation, sess.player.TransportX, sess.player.TransportY, sess.player.TransportZ, sess.player.TransportO)
			sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation = x, y, z, o
		} else if sess.player.TransportGUID == rawGUID && change.OldSpawn.Map != change.Spawn.Map {
			x, y, z, o := CalculatePassengerPosition(change.Spawn.X, change.Spawn.Y, change.Spawn.Z, change.Spawn.Orientation, sess.player.TransportX, sess.player.TransportY, sess.player.TransportZ, sess.player.TransportO)
			sess.teleportTo(change.Spawn.Map, x, y, z, o)
			continue
		}
		oldNear := sess.player.Map == change.OldSpawn.Map && math.Hypot(float64(change.OldSpawn.X-sess.player.X), float64(change.OldSpawn.Y-sess.player.Y)) <= distance
		newNear := sess.player.Map == change.Spawn.Map && math.Hypot(float64(change.Spawn.X-sess.player.X), float64(change.Spawn.Y-sess.player.Y)) <= distance
		if oldNear && !newNear {
			updates := protocol.NewUpdateData()
			updates.AddOutOfRangeGUID(rawGUID)
			if packet, err := updates.BuildPacket(0); err == nil {
				_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
			}
			continue
		}
		if !newNear {
			continue
		}
		updates := protocol.NewUpdateData()
		if oldNear && change.OldSpawn.Map == change.Spawn.Map {
			updates.AddUpdateBlock(buildGameObjectMovementUpdate(change.Spawn))
		} else {
			updates.AddUpdateBlock(buildGameObjectUpdate(change.Spawn))
		}
		if packet, err := updates.BuildPacket(0); err == nil {
			_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
		}
	}
}
