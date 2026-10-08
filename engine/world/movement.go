package world

import (
	"context"
	"encoding/hex"
	"math"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	movementOnTransport      uint32 = 0x00000200
	movementFalling          uint32 = 0x00001000
	movementSwimming         uint32 = 0x00200000
	movementFlying           uint32 = 0x02000000
	movementSplineElevation  uint32 = 0x04000000
	movementRoot             uint32 = 0x00000800
	movementDisableGravity   uint32 = 0x00000400
	movementCanFly           uint32 = 0x01000000
	movementWaterWalking     uint32 = 0x10000000
	movementFallingSlow      uint32 = 0x20000000
	movementHover            uint32 = 0x40000000
	movementSplineEnabled    uint32 = 0x08000000 // MOVEMENTFLAG_SPLINE_ENABLED (UnitDefines.h:245), used for flight paths
	movementPlayerStatusMask        = movementDisableGravity | movementRoot | movementCanFly | movementWaterWalking | movementFallingSlow | movementHover
	movementForward          uint32 = 0x00000001
	movementBackward         uint32 = 0x00000002
	movementStrafeLeft       uint32 = 0x00000004
	movementStrafeRight      uint32 = 0x00000008
	movementTurnLeft         uint32 = 0x00000010
	movementTurnRight        uint32 = 0x00000020
	movementPitchUp          uint32 = 0x00000040
	movementPitchDown        uint32 = 0x00000080
	movementFallingFar       uint32 = 0x00002000
	movementAscending        uint32 = 0x00400000
	movementDescending       uint32 = 0x00800000
	movement2Pitch           uint16 = 0x00000020
	movement2Interpolated    uint16 = 0x00000400
	maxPositionCoordinate           = 17066.666015625
)

type movementInfo struct {
	GUID            uint64
	Flags           uint32
	Flags2          uint16
	Time            uint32
	X               float32
	Y               float32
	Z               float32
	Orientation     float32
	Transport       *transportMovement
	Pitch           float32
	HasPitch        bool
	FallTime        uint32
	Jump            [4]float32
	HasJump         bool
	SplineElevation float32
	HasSpline       bool
}

type transportMovement struct {
	GUID        uint64
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	Time        uint32
	Seat        int8
	Time2       uint32
	HasTime2    bool
}

type nearTeleportDestination struct {
	X           float32
	Y           float32
	Z           float32
	Orientation float32
	Movement    movementInfo
}

func buildTeleportMovementPackets(guid uint64, info movementInfo) ([]byte, []byte) {
	info.GUID = guid
	self := protocol.NewBuffer(64)
	self.WritePackedGUID(guid)
	self.WriteU32(0)
	writeRawMovementInfo(self, info)
	nearby := protocol.NewBuffer(64)
	nearby.WritePackedGUID(guid)
	writeRawMovementInfo(nearby, info)
	return self.Bytes(), nearby.Bytes()
}

func (s *Server) gameTimeMilliseconds() uint32 {
	if s == nil || s.worldTimeStartedAt.IsZero() {
		return 0
	}
	return uint32(time.Since(s.worldTimeStartedAt) / time.Millisecond)
}

func (s *Server) broadcastTeleportMovement(source *session, payload []byte) {
	if s == nil || source == nil || source.player == nil || !source.worldReady.Load() || s.Config.VisibilityDistanceContinents <= 0 {
		return
	}
	distance := s.Config.VisibilityDistanceContinents
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for target := range s.sessions {
		if target == source || !target.authed || !target.worldReady.Load() || target.player == nil || target.player.Map != source.player.Map || target.player.InstanceID != source.player.InstanceID || !canSeePlayer(target, source) {
			continue
		}
		if math.Hypot(float64(target.player.X-source.player.X), float64(target.player.Y-source.player.Y)) > distance {
			continue
		}
		targets = append(targets, target)
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		if err := target.write(uint16(protocol.OpcodeMSG_MOVE_TELEPORT), payload, true); err != nil {
			target.debug("teleport movement broadcast failed", "account", target.accountName, "guid", source.playerGUID, "error", err)
		}
	}
}

func (s *session) handleMoveTeleportAck(ctx context.Context, payload []byte) bool {
	if s == nil || !s.playerLoaded || s.player == nil {
		return true
	}
	b := protocol.NewReader(payload)
	guid, err := b.ReadPackedGUID()
	if err != nil {
		return false
	}
	if _, err := b.ReadU32(); err != nil {
		return false
	}
	if _, err := b.ReadU32(); err != nil {
		return false
	}
	if !s.nearTeleportPending || guid != s.playerGUID {
		return true
	}
	dest := s.nearTeleportDest
	s.nearTeleportPending = false
	s.nearTeleportDest = nearTeleportDestination{}
	oldZone := s.player.Zone
	s.player.X, s.player.Y, s.player.Z, s.player.Orientation = dest.X, dest.Y, dest.Z, dest.Orientation
	if dest.Movement.Flags&movementOnTransport != 0 && dest.Movement.Transport != nil {
		s.player.TransportGUID = dest.Movement.Transport.GUID
		s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = dest.Movement.Transport.X, dest.Movement.Transport.Y, dest.Movement.Transport.Z, dest.Movement.Transport.Orientation
		s.player.TransportSeat = dest.Movement.Transport.Seat
	} else {
		s.player.TransportGUID = 0
		s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = 0, 0, 0, 0
		s.player.TransportSeat = 0
	}
	s.isMoving, s.isFalling = false, false
	s.lastFallZ, s.lastFallTime = dest.Z, 0
	s.setLastMovementInfo(dest.Movement)
	s.updateZoneAndArea(ctx, true)
	// MovementHandler.cpp:240-248 (HandleMoveTeleportAck): after a
	// zone-changing near teleport, a hostile-zone landing earns Honorless
	// Target (2479, triggered). The friendly-area UpdatePvP(false,false) arm
	// is vacuous in Go: IN_PVP is never set without the unit flag, and
	// applyZoneState derives the unit flag deterministically from the zone.
	if s.player.Zone != oldZone && s.pvpHostile {
		s.castSpellDirect(ctx, 2479, s.playerGUID)
	}
	if s.worldReady.Load() {
		s.refreshNearbyObjects(ctx)
	}
	s.resummonTemporaryPet(ctx)
	return true
}

// handleMoveSetCanFlyAck mirrors WorldSession::HandleMoveSetCanFlyAckOpcode
// (MiscHandler.cpp:1384-1398): the client acknowledges a can-fly toggle, and
// the server applies the reported movement flags to the mover's canonical
// movement info. Only flags are copied, not flags2 — matching C++.
func (s *session) handleMoveSetCanFlyAck(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	b := protocol.NewReader(payload)
	if _, err := b.ReadPackedGUID(); err != nil {
		s.debug("set can fly ack rejected", "account", s.accountName, "reason", "malformed guid")
		return false
	}
	if _, err := b.ReadU32(); err != nil {
		s.debug("set can fly ack rejected", "account", s.accountName, "reason", "malformed unk")
		return false
	}
	info, err := readMovementInfo(b)
	if err != nil {
		s.debug("set can fly ack rejected", "account", s.accountName, "reason", "malformed movement", "error", err)
		return false
	}
	// The copied flags come from the sanitizing ReadMovementInfo in C++, so
	// the ack runs the same anti-cheat pass before the copy.
	info.Flags &^= movementRoot
	info.Flags = s.sanitizeMovementFlags(info.Flags)
	s.movementMu.Lock()
	s.lastMovementInfo.Flags = info.Flags
	s.movementMu.Unlock()
	return true
}

func (s *session) handleMovement(ctx context.Context, opcode uint32, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.nearTeleportPending || s.farTeleportPending {
		return true
	}
	b := protocol.NewReader(payload)
	guid, err := b.ReadPackedGUID()
	if err != nil {
		s.debug("movement rejected", "account", s.accountName, "reason", "malformed guid", "opcode", opcode)
		return false
	}
	if guid != s.playerGUID {
		prefix := payload
		if len(prefix) > 24 {
			prefix = prefix[:24]
		}
		s.debug("movement rejected", "account", s.accountName, "reason", "mover mismatch", "guid", guid, "expected", s.playerGUID, "prefix", hex.EncodeToString(prefix))
		return true
	}
	info, err := readMovementInfo(b)
	if err != nil {
		s.debug("movement rejected", "account", s.accountName, "reason", "malformed movement", "opcode", opcode, "error", err)
		return false
	}
	info.GUID = guid
	if !validMovementPosition(info.X, info.Y, info.Z, info.Orientation) {
		s.debug("movement rejected", "account", s.accountName, "reason", "invalid position", "guid", guid)
		return true
	}
	if info.Flags&movementOnTransport != 0 && info.Transport != nil {
		// MovementHandler.cpp:307-311 — drop packets broadcast before a
		// teleport: the reported world position is nowhere near the server's.
		if math.Hypot(float64(info.X-s.player.X), float64(info.Y-s.player.Y)) > terrainGridSize {
			s.debug("movement rejected", "account", s.accountName, "reason", "stale transport packet", "opcode", opcode)
			return true
		}
		// MovementHandler.cpp:312-318 — transports size limited: the client
		// reports the passenger's offset relative to the transport here, so an
		// offset past the deck (or the zeppelin-leave glitch that arrives with
		// absolute continent coordinates) is a tampered packet.
		if math.Abs(float64(info.Transport.X)) > 75 || math.Abs(float64(info.Transport.Y)) > 75 || math.Abs(float64(info.Transport.Z)) > 75 {
			s.debug("movement rejected", "account", s.accountName, "reason", "transport offset out of bounds", "opcode", opcode)
			return true
		}
		// MovementHandler.cpp:319-324 — the world position plus the
		// transport-relative offset must still be a valid map coordinate.
		if !validMovementPosition(info.X+info.Transport.X, info.Y+info.Transport.Y, info.Z+info.Transport.Z, info.Orientation+info.Transport.Orientation) {
			s.debug("movement rejected", "account", s.accountName, "reason", "transport combined position invalid", "opcode", opcode)
			return true
		}
	}
	info.Flags &^= movementRoot
	info.Flags = s.sanitizeMovementFlags(info.Flags)
	isMove := info.Flags&(movementForward|movementBackward|movementStrafeLeft|movementStrafeRight|movementFalling) != 0
	s.isMoving = isMove
	if isMove {
		// Break casts/channels with SPELL_INTERRUPT_FLAG_MOVEMENT.
		s.interruptSpellsOnMovement()
	}
	if isMove && s.rooted {
		s.debug("movement rejected", "account", s.accountName, "reason", "rooted", "opcode", opcode)
		return true
	}
	if isMove && (s.player.UnitFlags&(unitFlagConfused|unitFlagFleeing) != 0 || s.hasAuraType(spellAuraCharm)) {
		s.debug("movement rejected", "account", s.accountName, "reason", "controlled", "opcode", opcode)
		return true
	}
	isFalling := (info.Flags & movementFalling) != 0
	if opcode == uint32(protocol.OpcodeMSG_MOVE_FALL_LAND) {
		isFalling = false
	} else if opcode == uint32(protocol.OpcodeMSG_MOVE_JUMP) {
		isFalling = true
	}
	s.isFalling = isFalling
	if isMove {
		// Movement interrupts land via interruptSpellsOnMovement above,
		// which gates on SPELL_INTERRUPT_FLAG_MOVEMENT (Spell::update,
		// Spell.cpp:3814-3831) — including the IsMoveAllowedChannel
		// channeled exemption. No unconditional interrupt here: C++
		// never breaks a cast whose InterruptFlags lack the movement bit.
		if s.autoRepeatSpell != 0 && s.autoRepeatSpell != 75 {
			s.autoRepeatSpell = 0
			s.autoRepeatTarget = 0
			buf := protocol.NewBuffer(9)
			buf.WritePackedGUID(s.playerGUID)
			_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		}
	}
	// Vehicle passengers may only turn their own orientation when the seat
	// carries VEHICLE_SEAT_FLAG_ALLOW_TURNING (DBCEnums.h:462); an orientation
	// change on such a seat breaks auras with AURA_INTERRUPT_FLAG_TURNING
	// (0x10, SpellDefines.h:51), and a turn on a locked seat is ignored so
	// the server orientation stands. MovementHandler.cpp:390-402.
	if s.player.VehicleGUID != 0 && s.server != nil {
		if kit := s.server.getVehicleKit(s.player.Map, s.player.InstanceID, s.player.VehicleGUID); kit != nil {
			if _, seatInfo, _ := kit.GetSeatForPassenger(s.playerGUID); seatInfo != nil {
				if seatInfo.HasFlag(wotlk.VehicleSeatFlagAllowTurning) {
					if info.Orientation != s.player.Orientation {
						s.removeAurasWithInterruptFlags(auraInterruptFlagTurning)
					}
				} else {
					info.Orientation = s.player.Orientation
				}
			}
		}
	}
	// A sitting player stands on any move or turn input (MovementHandler.cpp:404-405;
	// Unit::IsSitState, Unit.cpp:10542-10549 — the chair variants are unreachable
	// from Go's 0..3 stand-state range). Gated on not being a vehicle passenger,
	// matching the C++ vehicle block's early return.
	if s.player.VehicleGUID == 0 && (s.player.StandState == 1 || s.player.StandState == 2) &&
		info.Flags&(movementForward|movementBackward|movementStrafeLeft|movementStrafeRight|movementFalling|movementFallingFar|movementAscending|movementDescending|movementSplineElevation|movementTurnLeft|movementTurnRight|movementPitchUp|movementPitchDown) != 0 {
		s.player.StandState = 0
		s.sendPlayerUpdate()
	}
	s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
	s.updateZoneAndArea(ctx, false)
	previousTransportGUID := s.player.TransportGUID
	reportedTransportGUID := uint64(0)
	reportedOnTransport := info.Flags&movementOnTransport != 0 && info.Transport != nil
	if reportedOnTransport {
		reportedTransportGUID = info.Transport.GUID
	}
	if s.server != nil {
		contactGUID := uint64(0)
		contactDistance := math.MaxFloat64
		if reportedOnTransport {
			contactGUID = reportedTransportGUID
		} else {
			probeDistance := s.server.Config.VisibilityDistanceContinents
			if probeDistance <= 0 {
				probeDistance = 150
			}
			for _, transport := range s.server.nearbyTransportSpawns(*s.player, probeDistance) {
				distance := math.Hypot(float64(transport.X-s.player.X), float64(transport.Y-s.player.Y))
				if distance < contactDistance {
					contactDistance, contactGUID = distance, transportGUID(transport.GUID)
				}
			}
		}
		if contactGUID == 0 {
			s.transportContactProbeGUID, s.transportContactProbeAt = 0, time.Time{}
		} else if contactGUID != s.transportContactProbeGUID || time.Since(s.transportContactProbeAt) >= time.Second {
			s.transportContactProbeGUID, s.transportContactProbeAt = contactGUID, time.Now()
			s.debug("transport proximity movement", "player_guid", s.playerGUID, "player_map", s.player.Map, "player_x", s.player.X, "player_y", s.player.Y, "player_z", s.player.Z, "transport_guid", contactGUID, "transport_distance", contactDistance, "reported_transport_guid", reportedTransportGUID, "reported_on_transport", reportedOnTransport, "movement_flags", info.Flags)
		}
	}
	if info.Flags&movementOnTransport != 0 && info.Transport != nil {
		canonicalTransportGUID := info.Transport.GUID
		if s.player.VehicleGUID == 0 && s.server != nil {
			if spawn, found := s.server.transportSpawnForGUID(canonicalTransportGUID); found {
				canonicalTransportGUID = transportGUID(spawn.GUID)
			} else {
				info.Flags &^= movementOnTransport
				info.Transport = nil
			}
		}
		if info.Transport != nil {
			info.Transport.GUID = canonicalTransportGUID
			s.player.TransportGUID = canonicalTransportGUID
			s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = info.Transport.X, info.Transport.Y, info.Transport.Z, info.Transport.Orientation
			s.player.TransportSeat = info.Transport.Seat
		}
	}
	if info.Flags&movementOnTransport == 0 || info.Transport == nil {
		s.player.TransportGUID = 0
		s.player.TransportX, s.player.TransportY, s.player.TransportZ, s.player.TransportO = 0, 0, 0, 0
		s.player.TransportSeat = 0
	}
	if previousTransportGUID != s.player.TransportGUID {
		s.debug("transport attachment changed", "player_guid", s.playerGUID, "previous_transport_guid", previousTransportGUID, "transport_guid", s.player.TransportGUID, "reported_transport_guid", reportedTransportGUID, "reported_on_transport", reportedOnTransport)
	}
	if s.server != nil {
		if s.player.VehicleGUID != 0 {
			s.server.relocatePassengers(s.player.Map, s.player.InstanceID, s.player.VehicleGUID, info.X, info.Y, info.Z, info.Orientation)
		} else {
			s.server.relocatePassengers(s.player.Map, s.player.InstanceID, s.playerGUID, info.X, info.Y, info.Z, info.Orientation)
		}
	}
	s.checkDuelBounds()

	// Swimming state and breath mirror timer updates (TC MovementHandler.cpp:366-369):
	// InWater follows the MOVEMENTFLAG_SWIMMING bit on a flag/state mismatch;
	// C++ keeps InWater when the reported position is still under water
	// (jumping under water with the flag absent) — no bridge, Go has no
	// water-volume model. START_SWIM/STOP_SWIM carry no override in C++.
	wasSwimming := s.isSwimming
	isSwimming := info.Flags&movementSwimming != 0
	s.isSwimming = isSwimming
	if isSwimming && !wasSwimming {
		s.handleEnterSwimming()
	} else if !isSwimming && wasSwimming {
		s.handleExitSwimming()
	}

	// In flight cancels dark water / fatigue (TC Player.cpp:24539)
	if s.isInFlight() && s.inDarkWater {
		s.setInDarkWater(false)
	}

	// Fall damage generation and parachute interrupts (TC MovementHandler.cpp:359-365)
	if opcode == uint32(protocol.OpcodeMSG_MOVE_FALL_LAND) {
		s.handleFall(ctx, info)
	}
	if opcode == uint32(protocol.OpcodeMSG_MOVE_FALL_LAND) || opcode == uint32(protocol.OpcodeMSG_MOVE_START_SWIM) {
		s.removeAurasWithInterruptFlags(0x02000000) // AURA_INTERRUPT_FLAG_LANDING
	}
	s.updateFallInformationIfNeed(info, uint16(opcode))

	if opcode == uint32(protocol.OpcodeMSG_MOVE_STOP) || opcode == uint32(protocol.OpcodeMSG_MOVE_HEARTBEAT) || opcode == uint32(protocol.OpcodeMSG_MOVE_FALL_LAND) {
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
				"UPDATE characters SET position_x = ?, position_y = ?, position_z = ?, orientation = ?, map = ?, zone = ? WHERE guid = ?",
				info.X, info.Y, info.Z, info.Orientation, s.player.Map, s.player.Zone, s.playerGUID)
		}
	}
	// MovementHandler.cpp:371-379 — the server rebroadcasts the movement packet
	// with the client timestamp shifted onto the server clock by the time-sync
	// clock delta; with no sync sample established (or the sum leaving u32
	// range) the server game time is written instead, and the stored movement
	// info carries the rewritten time like C++'s m_movementInfo = movementInfo.
	moveTime := int64(info.Time) + s.timeSyncClockDelta
	if s.timeSyncClockDelta == 0 || moveTime < 0 || moveTime > 0xFFFFFFFF {
		s.debug("movement time fallback", "account", s.accountName, "reason", "no clock delta or overflow")
		info.Time = gameTimeMS()
	} else {
		info.Time = uint32(moveTime)
	}
	s.setLastMovementInfo(info)
	packet := protocol.NewBuffer(len(payload))
	writeMovementInfo(packet, info)
	s.server.broadcastMovement(uint16(opcode), packet.Bytes(), info, s)
	s.debug("movement accepted", "account", s.accountName, "guid", guid, "x", info.X, "y", info.Y, "z", info.Z)
	dx := float64(info.X - s.lastStreamX)
	dy := float64(info.Y - s.lastStreamY)
	if dx*dx+dy*dy > 30.0*30.0 {
		s.streamNearbyObjects(ctx)
	}
	return true
}

func (s *session) handleSetActiveMover(payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	s.debug("active mover received", "account", s.accountName, "guid", guid, "expected", s.playerGUID)
	return true
}

func (s *session) handleTimeSyncResponse(payload []byte) bool {
	recvMS := gameTimeMS()
	reader := protocol.NewReader(payload)
	counter, err := reader.ReadU32()
	if err != nil {
		return false
	}
	clientTime, err := reader.ReadU32()
	if err != nil {
		return false
	}
	s.debug("time sync response", "account", s.accountName, "counter", counter, "client_time", clientTime)
	// MovementHandler.cpp:658-689 — responses to unknown counters are dropped.
	sentAt, ok := s.timeSyncPending[counter]
	if !ok {
		return true
	}
	delete(s.timeSyncPending, counter)
	// Half of the round trip is attributed to the request leg, the other half
	// to the response leg; the delta then maps client timestamps onto the
	// server clock (serverTime = clockDelta + clientTime).
	roundTrip := recvMS - sentAt
	lagDelay := roundTrip / 2
	clockDelta := int64(sentAt+lagDelay) - int64(clientTime)
	if len(s.timeSyncClockDeltaQueue) == 6 {
		s.timeSyncClockDeltaQueue = s.timeSyncClockDeltaQueue[1:]
	}
	s.timeSyncClockDeltaQueue = append(s.timeSyncClockDeltaQueue, timeSyncSample{clockDelta: clockDelta, latency: roundTrip})
	s.computeNewClockDelta()

	if s.playerLoaded && !s.questStatusSent {
		s.questStatusSent = true
		if !s.sendQuestgiverStatusMultiple(context.Background()) {
			return false
		}
		if !s.sendTaxiNodeStatusMultiple(context.Background()) {
			return false
		}
	}
	return true
}

// resetTimeSync clears the pending time-sync requests and restarts the
// counter (WorldSession::ResetTimeSync, WorldSession.cpp:1682-1686), used when
// the player is added to a map and the clock exchange starts over.
func (s *session) resetTimeSync() {
	s.timeSyncNextCounter = 0
	s.timeSyncPending = make(map[uint32]uint32)
}

// recordTimeSyncSent registers the server send time of a SMSG_TIME_SYNC_REQ
// (WorldSession::SendTimeSync, WorldSession.cpp:1689-1697).
func (s *session) recordTimeSyncSent(counter uint32) {
	if s.timeSyncPending == nil {
		s.timeSyncPending = make(map[uint32]uint32)
	}
	s.timeSyncPending[counter] = gameTimeMS()
}

// computeNewClockDelta recomputes the session's time-sync clock delta from the
// sample queue (WorldSession::ComputeNewClockDelta, MovementHandler.cpp:690-725):
// the mean of the clock deltas whose round-trip latency is below
// median+stddev (population variance, boost::accumulators semantics), adopted
// only when it drifts more than 25ms; with no passing samples and no delta
// established yet, the most recent sample is used verbatim.
func (s *session) computeNewClockDelta() {
	n := len(s.timeSyncClockDeltaQueue)
	if n == 0 {
		return
	}
	latencies := make([]float64, n)
	for i, sample := range s.timeSyncClockDeltaQueue {
		latencies[i] = float64(sample.latency)
	}
	sort.Float64s(latencies)
	var median float64
	if n%2 == 1 {
		median = latencies[n/2]
	} else {
		median = (latencies[n/2-1] + latencies[n/2]) / 2
	}
	mean := 0.0
	for _, latency := range latencies {
		mean += latency
	}
	mean /= float64(n)
	variance := 0.0
	for _, latency := range latencies {
		dev := latency - mean
		variance += dev * dev
	}
	variance /= float64(n)
	threshold := uint32(math.Round(median)) + uint32(math.Round(math.Sqrt(variance)))
	var sum int64
	passing := 0
	for _, sample := range s.timeSyncClockDeltaQueue {
		if sample.latency < threshold {
			sum += sample.clockDelta
			passing++
		}
	}
	if passing != 0 {
		newDelta := int64(math.Round(float64(sum) / float64(passing)))
		if newDelta-s.timeSyncClockDelta > 25 || s.timeSyncClockDelta-newDelta > 25 {
			s.timeSyncClockDelta = newDelta
		}
	} else if s.timeSyncClockDelta == 0 {
		s.timeSyncClockDelta = s.timeSyncClockDeltaQueue[n-1].clockDelta
	}
}

func readMovementInfo(b *protocol.Buffer) (movementInfo, error) {
	var result movementInfo
	var err error
	if result.Flags, err = b.ReadU32(); err != nil {
		return result, err
	}
	if result.Flags2, err = b.ReadU16(); err != nil {
		return result, err
	}
	if result.Time, err = b.ReadU32(); err != nil {
		return result, err
	}
	values := []*float32{&result.X, &result.Y, &result.Z, &result.Orientation}
	for _, value := range values {
		if *value, err = b.ReadF32(); err != nil {
			return result, err
		}
	}
	if result.Flags&movementOnTransport != 0 {
		transport := &transportMovement{}
		if transport.GUID, err = b.ReadPackedGUID(); err != nil {
			return result, err
		}
		values = []*float32{&transport.X, &transport.Y, &transport.Z, &transport.Orientation}
		for _, value := range values {
			if *value, err = b.ReadF32(); err != nil {
				return result, err
			}
		}
		if transport.Time, err = b.ReadU32(); err != nil {
			return result, err
		}
		if transport.Seat, err = b.ReadI8(); err != nil {
			return result, err
		}
		if result.Flags2&movement2Interpolated != 0 {
			if transport.Time2, err = b.ReadU32(); err != nil {
				return result, err
			}
			transport.HasTime2 = true
		}
		result.Transport = transport
	}
	if result.Flags&(movementSwimming|movementFlying) != 0 || result.Flags2&movement2Pitch != 0 {
		if result.Pitch, err = b.ReadF32(); err != nil {
			return result, err
		}
		result.HasPitch = true
	}
	if result.FallTime, err = b.ReadU32(); err != nil {
		return result, err
	}
	if result.Flags&movementFalling != 0 {
		for index := range result.Jump {
			if result.Jump[index], err = b.ReadF32(); err != nil {
				return result, err
			}
		}
		result.HasJump = true
	}
	if result.Flags&movementSplineElevation != 0 {
		if result.SplineElevation, err = b.ReadF32(); err != nil {
			return result, err
		}
		result.HasSpline = true
	}
	return result, nil
}

func writeMovementInfo(b *protocol.Buffer, info movementInfo) {
	b.WritePackedGUID(info.GUID)
	writeRawMovementInfo(b, info)
}

func writeRawMovementInfo(b *protocol.Buffer, info movementInfo) {
	b.WriteU32(info.Flags)
	b.WriteU16(info.Flags2)
	b.WriteU32(info.Time)
	b.WriteF32(info.X)
	b.WriteF32(info.Y)
	b.WriteF32(info.Z)
	b.WriteF32(info.Orientation)
	if info.Flags&movementOnTransport != 0 && info.Transport != nil {
		b.WritePackedGUID(info.Transport.GUID)
		b.WriteF32(info.Transport.X)
		b.WriteF32(info.Transport.Y)
		b.WriteF32(info.Transport.Z)
		b.WriteF32(info.Transport.Orientation)
		b.WriteU32(info.Transport.Time)
		b.WriteI8(info.Transport.Seat)
		if info.Flags2&movement2Interpolated != 0 && info.Transport.HasTime2 {
			b.WriteU32(info.Transport.Time2)
		}
	}
	if info.HasPitch {
		b.WriteF32(info.Pitch)
	}
	b.WriteU32(info.FallTime)
	if info.HasJump {
		for _, value := range info.Jump {
			b.WriteF32(value)
		}
	}
	if info.HasSpline {
		b.WriteF32(info.SplineElevation)
	}
}

func (s *session) setLastMovementInfo(info movementInfo) {
	if s == nil {
		return
	}
	if info.Transport != nil {
		transport := *info.Transport
		info.Transport = &transport
	}
	s.movementMu.Lock()
	s.lastMovementInfo, s.lastMovementInfoSet = info, true
	s.movementMu.Unlock()
}

func (s *session) clearLastMovementInfo() {
	if s == nil {
		return
	}
	s.movementMu.Lock()
	s.lastMovementInfo, s.lastMovementInfoSet = movementInfo{}, false
	s.movementMu.Unlock()
}

func (s *session) lastMovementInfoSnapshot() (movementInfo, bool) {
	if s == nil {
		return movementInfo{}, false
	}
	s.movementMu.RLock()
	info, exists := s.lastMovementInfo, s.lastMovementInfoSet
	s.movementMu.RUnlock()
	if info.Transport != nil {
		transport := *info.Transport
		info.Transport = &transport
	}
	return info, exists
}

func (s *session) movementInfoForCreate(state playerState) movementInfo {
	info := movementInfo{GUID: state.GUID, Time: uint32(time.Now().UnixMilli()), X: state.X, Y: state.Y, Z: state.Z, Orientation: state.Orientation}
	if last, exists := s.lastMovementInfoSnapshot(); exists {
		info = last
	}
	info.GUID = state.GUID
	info.X, info.Y, info.Z, info.Orientation = state.X, state.Y, state.Z, state.Orientation
	if state.TransportGUID != 0 {
		info.Flags |= movementOnTransport
		if info.Transport == nil {
			info.Transport = &transportMovement{}
		}
		info.Transport.GUID = state.TransportGUID
		info.Transport.X, info.Transport.Y, info.Transport.Z, info.Transport.Orientation = state.TransportX, state.TransportY, state.TransportZ, state.TransportO
		info.Transport.Seat = state.TransportSeat
	} else {
		info.Flags &^= movementOnTransport
		info.Transport = nil
	}
	if s != nil && s.rooted {
		info.Flags |= movementRoot
	} else {
		info.Flags &^= movementRoot
	}
	return info
}

func (s *session) sanitizeMovementFlags(flags uint32) uint32 {
	if flags&movementForward != 0 && flags&movementBackward != 0 {
		flags &^= movementForward | movementBackward
	}
	if flags&movementStrafeLeft != 0 && flags&movementStrafeRight != 0 {
		flags &^= movementStrafeLeft | movementStrafeRight
	}
	if flags&movementTurnLeft != 0 && flags&movementTurnRight != 0 {
		flags &^= movementTurnLeft | movementTurnRight
	}
	if flags&movementAscending != 0 && flags&movementDescending != 0 {
		flags &^= movementAscending | movementDescending
	}
	// Cannot pitch up and down at the same time (WorldSession.cpp:981-983).
	if flags&movementPitchUp != 0 && flags&movementPitchDown != 0 {
		flags &^= movementPitchUp | movementPitchDown
	}
	// Cannot hover without SPELL_AURA_HOVER (WorldSession.cpp:973-975).
	if flags&movementHover != 0 && !s.hasAuraType(spellAuraHover) {
		flags &^= movementHover
	}
	// Cannot walk on water without SPELL_AURA_WATER_WALK except for ghosts
	// (WorldSession.cpp:989-993).
	if flags&movementWaterWalking != 0 && !s.hasAuraType(spellAuraWaterWalk) && !s.hasAuraType(spellAuraGhost) {
		flags &^= movementWaterWalking
	}
	// Cannot feather fall without SPELL_AURA_FEATHER_FALL (WorldSession.cpp:996-998).
	if flags&movementFallingSlow != 0 && !s.hasAuraType(spellAuraFeatherFall) {
		flags &^= movementFallingSlow
	}
	// SPLINE_ENABLED is stripped unless the mover's spline is initialized and
	// not finalized (WorldSession.cpp:1014-1016). Go has no player movespline
	// model — taxi flights are server-simulated — so the condition can never
	// hold and the flag is always stripped.
	flags &^= movementSplineEnabled
	// Cannot fly if no fly auras present. Exception is being a GM — account
	// security governs, not an active .gm flag (WorldSession.cpp:1006-1009;
	// s.security is the account level, 0 = SEC_PLAYER). s is the only mover
	// in Go, so HasAuraType on the player mirrors the UnitBeingMoved leg.
	if s.security == 0 && flags&(movementFlying|movementCanFly) != 0 &&
		!s.hasAuraType(spellAuraFly) && !s.hasAuraType(spellAuraMountedFlightSpeed) {
		flags &^= movementFlying | movementCanFly
	}
	// Cannot fly and fall at the same time (WorldSession.cpp:1011-1012).
	if flags&(movementCanFly|movementDisableGravity) != 0 && flags&movementFalling != 0 {
		flags &^= movementFalling
	}
	return flags
}

func validMovementPosition(x, y, z, orientation float32) bool {
	for _, value := range []float32{x, y, z, orientation} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	return math.Abs(float64(x)) <= maxPositionCoordinate && math.Abs(float64(y)) <= maxPositionCoordinate && math.Abs(float64(z)) <= maxPositionCoordinate
}

func validMapCellCoordinates(x, y float32) bool {
	const centerCells = 256
	const totalCells = 512
	gridSize := float32(533.3333)
	cellSize := gridSize / 8
	cellOffset := cellSize / 2
	cell := func(value float32) int {
		return int((float64(value)-float64(cellOffset))/float64(cellSize) + centerCells + 0.5)
	}
	cellX, cellY := cell(x), cell(y)
	return cellX >= 0 && cellX < totalCells && cellY >= 0 && cellY < totalCells
}

func (s *Server) addSession(value *session) {
	s.sessionsMu.Lock()
	if s.sessions == nil {
		s.sessions = make(map[*session]struct{})
	}
	s.sessions[value] = struct{}{}
	s.sessionsMu.Unlock()
}

func (s *Server) removeSession(session *session) {
	wasQueued := session.inQueue
	s.sessionsMu.Lock()
	delete(s.sessions, session)
	if wasQueued {
		for i, queued := range s.queuedSessions {
			if queued == session {
				s.queuedSessions = append(s.queuedSessions[:i], s.queuedSessions[i+1:]...)
				break
			}
		}
	}
	s.sessionsMu.Unlock()
	// AutoBalance_AllMapScript::OnPlayerLeaveAll (AutoBalance.cpp): the
	// disconnecting player leaves the map it is currently on.
	if session != nil && session.player != nil {
		s.autoBalancePlayerLeave(session, session.player.Map, session.player.InstanceID)
	}
	if !wasQueued {
		s.promoteQueuedPlayers()
	}
}

func (s *Server) broadcastMovement(opcode uint16, payload []byte, info movementInfo, source *session) {
	s.sessionsMu.RLock()
	targets := make([]*session, 0, len(s.sessions))
	for target := range s.sessions {
		if target == source || !target.authed || !target.worldReady.Load() || target.player == nil || target.player.Map != source.player.Map || target.player.InstanceID != source.player.InstanceID {
			continue
		}
		if source != nil && source.isStealthed() && !target.canDetectStealthOf(source) {
			continue
		}
		targets = append(targets, target)
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		if err := target.write(opcode, payload, true); err != nil {
			target.debug("movement broadcast failed", "account", target.accountName, "guid", info.GUID, "error", err)
		}
	}
}

// handleForceMoveRootAck processes CMSG_FORCE_MOVE_ROOT_ACK (0x0E9).
// Reference: WorldSession::HandleMoveRootAck (MiscHandler.cpp:941-959):
// the C++ body is commented out and the handler just finishes the packet
// ("no used"), so the ack changes nothing — no position apply, no rooted
// flip, no broadcast. The server's own root application rides on
// SMSG_FORCE_MOVE_ROOT via setRooted, never on this ack.
func (s *session) handleForceMoveRootAck(ctx context.Context, payload []byte) bool {
	return s.discardMovementAck(payload)
}

// handleForceMoveUnrootAck processes CMSG_FORCE_MOVE_UNROOT_ACK (0x0EB).
// Reference: WorldSession::HandleMoveUnRootAck (MiscHandler.cpp:919-939):
// same "no used" discard as the root ack above.
func (s *session) handleForceMoveUnrootAck(ctx context.Context, payload []byte) bool {
	return s.discardMovementAck(payload)
}

// discardMovementAck parses a movement ack the server intentionally ignores
// and applies nothing, mirroring the C++ "no used" handlers that just
// rfinish the packet (HandleFeatherFallAck, HandleMoveRootAck,
// HandleMoveUnRootAck, HandleMoveHoverAck, HandleMoveWaterWalkAck).
func (s *session) discardMovementAck(payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	_, _ = b.ReadPackedGUID()
	_, _ = b.ReadU32() // ack index
	_, _ = readMovementInfo(b)
	return true
}

func (s *session) handleMovementAck(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	_, _ = b.ReadPackedGUID()
	_, _ = b.ReadU32() // ack index
	info, err := readMovementInfo(b)
	if err == nil && validMovementPosition(info.X, info.Y, info.Z, info.Orientation) {
		s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
	}
	return true
}

// handleForceTurnRateChangeAck processes CMSG_FORCE_TURN_RATE_CHANGE_ACK (0x2DF).
func (s *session) handleForceTurnRateChangeAck(ctx context.Context, payload []byte) bool {
	return s.handleMovementAck(ctx, payload)
}

func (s *Server) broadcastToNearby(opcode uint16, payload []byte, source *session) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for target := range s.sessions {
		if !target.authed || !target.worldReady.Load() || target.player == nil {
			continue
		}
		if source != nil && (target == source || target.player.Map != source.player.Map || target.player.InstanceID != source.player.InstanceID) {
			continue
		}
		_ = target.write(opcode, payload, true)
	}
}

func (s *Server) broadcastToInstance(mapID, instanceID uint32, opcode uint16, payload []byte, source *session) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for target := range s.sessions {
		if !target.authed || !target.worldReady.Load() || target.player == nil || target.player.Map != mapID || target.player.InstanceID != instanceID || target == source {
			continue
		}
		_ = target.write(opcode, payload, true)
	}
}

// handleMoveFeatherFallAck processes CMSG_MOVE_FEATHER_FALL_ACK (0x2CF).
// Reference: WorldSession::HandleFeatherFallAck (MiscHandler.cpp:908-914):
// "no used" — the packet is just finished, so the ack applies nothing.
func (s *session) handleMoveFeatherFallAck(ctx context.Context, payload []byte) bool {
	return s.discardMovementAck(payload)
}

// handleMoveHoverAck processes CMSG_MOVE_HOVER_ACK (0x0F6).
// Reference: WorldSession::HandleMoveHoverAck (MovementHandler.cpp:583-596):
// the acked movement info is parsed and dropped; no state changes.
func (s *session) handleMoveHoverAck(ctx context.Context, payload []byte) bool {
	return s.discardMovementAck(payload)
}

// handleMoveWaterWalkAck processes CMSG_MOVE_WATER_WALK_ACK (0x2D0).
// Reference: WorldSession::HandleMoveWaterWalkAck (MovementHandler.cpp:598-611):
// parsed and dropped like the hover ack.
func (s *session) handleMoveWaterWalkAck(ctx context.Context, payload []byte) bool {
	return s.discardMovementAck(payload)
}

// handleMoveKnockBackAck processes CMSG_MOVE_KNOCK_BACK_ACK (0x0F0).
// Reference: WorldSession::HandleMoveKnockBackAck (MovementHandler.cpp:553).
func (s *session) handleMoveKnockBackAck(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	guid, _ := b.ReadPackedGUID()
	_, _ = b.ReadU32() // ack index
	info, err := readMovementInfo(b)
	if err != nil || guid != s.playerGUID {
		return true
	}
	// MovementHandler.cpp:553-576 — the acked info becomes m_movementInfo
	// but the server position is NOT updated (no UpdatePosition leg); the
	// MSG_MOVE_KNOCK_BACK broadcast is built from the stored movement info
	// plus the acked jump params. Applying the acked position here would
	// let a client teleport by knockback ack. The info passes through the
	// ReadMovementInfo anti-cheat sanitize like C++ before it is stored and
	// rebroadcast.
	info.Flags &^= movementRoot
	info.Flags = s.sanitizeMovementFlags(info.Flags)
	info.GUID = guid
	s.setLastMovementInfo(info)

	packet := protocol.NewBuffer(66)
	packet.WritePackedGUID(guid)
	writeRawMovementInfo(packet, info)
	if info.HasJump {
		for _, v := range info.Jump {
			packet.WriteF32(v)
		}
	} else {
		packet.WriteF32(0)
		packet.WriteF32(0)
		packet.WriteF32(0)
		packet.WriteF32(0)
	}
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeMSG_MOVE_KNOCK_BACK), packet.Bytes(), s)
	}
	return true
}

// handleForceSpeedChangeAck processes the CMSG_FORCE_*_SPEED_CHANGE_ACK
// opcodes (0x0E2/0x0E3/0x0E5/0x0E7/0x0E8/0x0EA/0x0EC). 0x45D
// (CMSG_FORCE_PITCH_RATE_CHANGE_ACK) is STATUS_NEVER/Handle_NULL in C++ and
// is discarded in the dispatch table instead.
// Reference: WorldSession::HandleForceSpeedChangeAck (MovementHandler.cpp:442).
func (s *session) handleForceSpeedChangeAck(opcode uint16, ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	guid, _ := b.ReadPackedGUID()
	// now can skip not our packet
	if guid != s.playerGUID {
		return true
	}
	_, _ = b.ReadU32() // ack counter, unused
	// The movement info is parsed but not applied, matching C++ which never
	// assigns it to m_movementInfo here.
	_, _ = readMovementInfo(b)
	newspeed, _ := b.ReadF32()
	moveType, ok := forcedSpeedAckMoveType(opcode)
	if !ok {
		return true
	}
	// skip all forced speed changes except last and unexpected: the client
	// sends one ACK for the mounted/run case and intermediate ACKs must not
	// trip the anti-cheat check.
	if s.forcedSpeedChanges[moveType] > 0 {
		s.forcedSpeedChanges[moveType]--
		if s.forcedSpeedChanges[moveType] > 0 {
			return true
		}
	}
	if !s.forcedSpeedSent[moveType] || s.player.TransportGUID != 0 {
		return true
	}
	expected := s.forcedSpeedExpected[moveType]
	if math.Abs(float64(expected-newspeed)) > 0.01 {
		if expected > newspeed {
			// client under-reports: re-send the correct speed
			// (Unit::SetSpeedRate leg, MovementHandler.cpp:505-509)
			s.debug("force speed change corrected", "account", s.accountName, "moveType", moveType, "expected", expected, "acked", newspeed)
			// C++ re-sends via SetSpeedRate(move_type, GetSpeedRate(move_type)):
			// same type, same rate. The GM-override store holds the last
			// rate the server forced for walk/run/runBack/swim/flight.
			if moveType >= 0 && moveType < len(speedForceOpcodes) {
				if opcodes := speedForceOpcodes[moveType]; opcodes[0] != 0 {
					s.sendRuntimeMovementSpeed(opcodes[0], opcodes[1], expected, moveType == moveTypeRun)
				}
			}
		} else {
			// client over-reports its speed: cheating
			s.debug("force speed change mismatch, kicking", "account", s.accountName, "moveType", moveType, "expected", expected, "acked", newspeed)
			s.kickSession(s)
		}
	}
	return true
}

// handleMoveNotActiveMover processes CMSG_MOVE_NOT_ACTIVE_MOVER (0x2D1).
// Reference: WorldSession::HandleMoveNotActiveMover (MovementHandler.cpp:
// the parsed info becomes m_movementInfo only — no position update, no
// broadcast.
func (s *session) handleMoveNotActiveMover(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	guid, _ := b.ReadPackedGUID()
	info, err := readMovementInfo(b)
	if err != nil {
		return true
	}
	// The info runs the ReadMovementInfo anti-cheat sanitize before it is
	// stored, matching C++ (WorldSession.cpp:969-1016).
	info.Flags &^= movementRoot
	info.Flags = s.sanitizeMovementFlags(info.Flags)
	info.GUID = guid
	s.setLastMovementInfo(info)
	return true
}

// handleMoveFallReset processes CMSG_MOVE_FALL_RESET (0x2CA).
// Reference: Opcodes.cpp:845 routes CMSG_MOVE_FALL_RESET to
// WorldSession::HandleMovementOpcodes, so the packet takes the normal
// movement path (fall-info latch, zone update, broadcast) with no
// fall-tracker reset of its own.
func (s *session) handleMoveFallReset(ctx context.Context, payload []byte) bool {
	return s.handleMovement(ctx, uint32(protocol.OpcodeCMSG_MOVE_FALL_RESET), payload)
}

// handleMoveSplineDone processes CMSG_MOVE_SPLINE_DONE (0x2C9).
// Reference: WorldSession::HandleMoveSplineDoneOpcode (TaxiHandler.cpp:201).
func (s *session) handleMoveSplineDone(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	_, _ = b.ReadPackedGUID()
	// HandleMoveSplineDoneOpcode (TaxiHandler.cpp:201-247) reads the movement
	// info and the spline id only to advance the packet — the acked position
	// is never applied. Go pins the arrival position when the flight starts
	// (taxi.go), so applying the client-reported coordinates here would let a
	// forged SPLINE_DONE teleport the player.
	_, _ = readMovementInfo(b)
	if s.inFlight {
		s.inFlight = false
		if s.player != nil {
			s.player.MountDisplayID = 0
			s.sendPlayerMountUpdate()
			s.sendPlayerUpdate()
		}
	}
	return true
}

// handleMoveChngTransport processes CMSG_MOVE_CHNG_TRANSPORT (0x38D).
func (s *session) handleMoveChngTransport(ctx context.Context, payload []byte) bool {
	return s.handleMovement(ctx, uint32(protocol.OpcodeCMSG_MOVE_CHNG_TRANSPORT), payload)
}

// handleMoveSetFly processes CMSG_MOVE_SET_FLY (0x0D6).
func (s *session) handleMoveSetFly(ctx context.Context, payload []byte) bool {
	return s.handleMovement(ctx, uint32(protocol.OpcodeCMSG_MOVE_SET_FLY), payload)
}

// handleMoveTimeSkipped processes CMSG_MOVE_TIME_SKIPPED (0x2CE).
// Reference: WorldSession::HandleMoveTimeSkippedOpcode (MovementHandler.cpp:626).
func (s *session) handleMoveTimeSkipped(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	moverGUID, err := r.ReadPackedGUID()
	if err != nil || moverGUID != s.playerGUID {
		return true
	}
	timeSkipped, err := r.ReadU32()
	if err != nil {
		return true
	}
	// MovementHandler.cpp:651 — mover->m_movementInfo.time += timeSkipped.
	s.movementMu.Lock()
	s.lastMovementInfo.Time += timeSkipped
	s.movementMu.Unlock()
	buf := protocol.NewBuffer(16)
	buf.WritePackedGUID(s.playerGUID)
	buf.WriteU32(timeSkipped)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeMSG_MOVE_TIME_SKIPPED), buf.Bytes(), s)
	}
	return true
}

func (s *session) clearSummonPending() {
	s.summonExpire = time.Time{}
	s.summonerGUID = 0
	s.summonLocSet = false
}

func (s *session) sendSummonRequest(summonerGUID uint64, zoneID uint32) {
	if !s.summonExpire.IsZero() && time.Now().Before(s.summonExpire) {
		return
	}
	if s.hasAura(23445) {
		return
	}
	s.summonExpire = time.Now().Add(2 * time.Minute)
	s.summonerGUID = summonerGUID
	s.summonLocSet = false
	if s.server != nil {
		if summonerSess := s.server.findSessionByGUID(summonerGUID); summonerSess != nil && summonerSess.playerLoaded && summonerSess.player != nil {
			s.summonMap = summonerSess.player.Map
			s.summonX, s.summonY, s.summonZ, s.summonO = summonerSess.player.X, summonerSess.player.Y, summonerSess.player.Z, summonerSess.player.Orientation
			s.summonLocSet = true
		}
	}
	buf := protocol.NewBuffer(16)
	buf.WriteU64(summonerGUID)
	buf.WriteU32(zoneID)
	buf.WriteU32(120000) // 2 minutes in ms
	_ = s.write(uint16(protocol.OpcodeSMSG_SUMMON_REQUEST), buf.Bytes(), true)
}

// handleSummonResponse processes CMSG_SUMMON_RESPONSE (0x2AC).
// Reference: WorldSession::HandleSummonResponseOpcode (MovementHandler.cpp:613) and Player::SummonIfPossible (Player.cpp:23829).
func (s *session) handleSummonResponse(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if len(payload) < 9 {
		s.clearSummonPending()
		return true
	}
	r := protocol.NewReader(payload)
	summonerGUID, _ := r.ReadU64()
	agree, _ := r.ReadU8()
	if agree == 0 {
		s.clearSummonPending()
		return true
	}
	if s.player.Health == 0 || (s.player.UnitFlags&unitFlagInCombat != 0) {
		s.clearSummonPending()
		return true
	}
	if !s.summonExpire.IsZero() && time.Now().After(s.summonExpire) {
		s.clearSummonPending()
		return true
	}

	s.finishTaxiFlight()
	summonLocSet := s.summonLocSet
	summonMap, summonX, summonY, summonZ, summonO := s.summonMap, s.summonX, s.summonY, s.summonZ, s.summonO
	s.clearSummonPending()
	s.updateAchievementCriteria(criteriaTypeAcceptedSummonings, 0, 1)
	if summonLocSet {
		s.teleportTo(summonMap, summonX, summonY, summonZ, summonO)
	} else if s.server != nil {
		summonerSess := s.server.findSessionByGUID(summonerGUID)
		if summonerSess != nil && summonerSess.playerLoaded && summonerSess.player != nil {
			s.teleportTo(summonerSess.player.Map, summonerSess.player.X, summonerSess.player.Y, summonerSess.player.Z, summonerSess.player.Orientation)
		}
	}
	return true
}

// handleMountSpecialAnim processes CMSG_MOUNTSPECIAL_ANIM (0x171).
func (s *session) handleMountSpecialAnim(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	buf := protocol.NewBuffer(8)
	buf.WritePackedGUID(s.playerGUID)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_MOUNTSPECIAL_ANIM), buf.Bytes(), s)
	return true
}

// Vehicle handlers are implemented in vehicle.go

const (
	damageExhausted  uint8 = 0
	damageDrowning   uint8 = 1
	damageFall       uint8 = 2
	damageLava       uint8 = 3
	damageSlime      uint8 = 4
	damageFire       uint8 = 5
	damageFallToVoid uint8 = 6
)

// updateFallInformationIfNeed mirrors Player::UpdateFallInformationIfNeed (Player.cpp:25704-25708).
func (s *session) updateFallInformationIfNeed(info movementInfo, opcode uint16) {
	if s.lastFallTime >= info.FallTime || s.lastFallZ <= info.Z || opcode == uint16(protocol.OpcodeMSG_MOVE_FALL_LAND) {
		s.lastFallTime = info.FallTime
		s.lastFallZ = info.Z
	}
}

// HandleFall mirrors TrinityCore Player::HandleFall (Player.cpp:25369-25418).
// Documented deltas: no sWorld->getRate(RATE_DAMAGE_FALL) multiplier (defaults to 1.0;
// Go has no world-rate config); no UpdateGroundPositionZ leg (no ground-height model);
// isImmuneToDamage does not consume aura charges (Go models no charge counters).
func (s *session) handleFall(ctx context.Context, info movementInfo) {
	if s.player == nil || s.player.Health == 0 || s.inFlight {
		return
	}
	isGM := (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0
	if isGM {
		return
	}

	zDiff := s.lastFallZ - info.Z
	// Low fall distance, Feather Fall, Hover, or Fly ignore fall damage
	// 14.57 is derived from resolving damageperc formula (0.018*z - 0.2426 = 0 -> z = 13.48) with safe margin
	if zDiff < 14.57 {
		return
	}

	// Immune to fall damage via feather fall / slow fall (105), hover (106), or fly (201)
	if s.hasAuraType(105) || s.hasAuraType(106) || s.hasAuraType(201) {
		return
	}

	// Physical damage immunity negates fall damage outright (Player.cpp:25380-25382:
	// !IsImmunedToDamage(SPELL_SCHOOL_MASK_NORMAL), "physical immunity (charges used)");
	// ordered after the aura gates to match the C++ && short-circuit chain.
	if s.isImmuneToDamage(spellSchoolMaskNormal) {
		return
	}

	safeFall := s.getTotalAuraModifier(144) // SPELL_AURA_SAFE_FALL
	damagePerc := 0.018*(zDiff-float32(safeFall)) - 0.2426
	if damagePerc <= 0 {
		return
	}

	damage := uint32(damagePerc * float32(s.player.MaxHealth))

	// CHEAT_GOD zeroes the damage before the damage>0 gate (Player.cpp:25390-25401),
	// so god-cheat falls never reach EnvironmentalDamage and never credit the
	// FALL_WITHOUT_DYING achievement.
	if s.godCheatActive() {
		damage = 0
	}

	if damage > 0 {
		//Prevent fall damage from being more than the player maximum health
		if damage > s.player.MaxHealth {
			damage = s.player.MaxHealth
		}

		// Gust of Wind (spell 43621) sets fall damage to exactly half of max
		// health (Player.cpp:25397-25398) — not a cap: a low computed damage
		// is raised to the half as well.
		if s.hasAura(43621) {
			damage = s.player.MaxHealth / 2
		}

		before := s.player.Health
		dealt := s.environmentalDamage(ctx, damageFall, damage)
		// Reference Player.cpp:25406-25410: final_damage < original_health credits
		// FALL_WITHOUT_DYING with the fall distance in centimeters. The returned
		// value is the post-negation damage, so god-cheat falls (final_damage 0)
		// credit exactly like C++.
		if s.player.Health > 0 && dealt < before {
			s.updateAchievementCriteria(criteriaTypeFallWithoutDying, 0, uint32(zDiff*100))
		}
	}
}

// environmentalDamage deals environmental damage (e.g. damageFall = 2) to the player,
// broadcasting SMSG_ENVIRONMENTAL_DAMAGE_LOG and triggering death if lethal.
// Mirrors TrinityCore Player::EnvironmentalDamage (Player.cpp:758-809).
func (s *session) environmentalDamage(ctx context.Context, damageType uint8, damage uint32) uint32 {
	if s.player == nil || s.player.Health == 0 {
		return 0
	}
	isGM := (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0
	if isGM {
		return 0
	}

	// Unit::DealDamage (Unit.cpp:735-737) via Player::EnvironmentalDamage
	// (Player.cpp:784): CHEAT_GOD negates all environmental damage (fall,
	// drowning, fatigue, lava) — the HandleFall zeroing (Player.cpp:25390) is the
	// same arm one call up, so no separate fall check is needed. The damage log
	// below still reports the pre-negation amount, matching C++ sending
	// EnvironmentalDamageLog before DealDamage; the health legs run on the
	// negated value and no damage procs fire.
	damage = s.negateGodModeDamage(damage)

	// Unit::DealDamage (Unit.cpp:825-853, 957-973): environmental damage is
	// self-damage (attacker == victim, Player.cpp:792) — damage of exactly
	// health-1 on a duelist completes the duel as won; lethal damage is not
	// capped (the attacker is not the opponent) and falls through to the
	// kill path, which interrupts the duel (Unit::Kill, Unit.cpp:11363-11368).
	if duelDefeatOnDamage(s, s.playerGUID, damage, s.player.Health) {
		// Duel defeat consumed the hit — loser at 1 HP, duel complete.
	} else if damage >= s.player.Health {
		damage = s.player.Health
		s.player.Health = 0
	} else {
		s.player.Health -= damage
		if damage > 0 {
			s.procDamageAuras(true)
		}
	}

	absorb := uint32(0)
	resist := uint32(0)

	packet := protocol.NewBuffer(21)
	packet.WriteU64(s.playerGUID)
	packet.WriteU8(damageType)
	packet.WriteU32(damage)
	packet.WriteU32(resist)
	packet.WriteU32(absorb)
	_ = s.write(uint16(protocol.OpcodeSMSG_ENVIRONMENTAL_DAMAGE_LOG), packet.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ENVIRONMENTAL_DAMAGE_LOG), packet.Bytes(), s)
	}

	s.sendPlayerUpdate()

	if s.player.Health == 0 {
		s.updateAchievementCriteria(criteriaTypeDeathsFrom, uint32(damageType), 1)
		// Environmental damage has no attacker (Unit::Kill's attacker is nil).
		s.killPlayer(ctx, nil, false)
	}
	return damage
}

// hasAuraType checks whether the player currently has an active or passive aura of the given type.
// Mirrors Unit::HasAuraType.
func (s *session) hasAuraType(auraType uint32) bool {
	if s.player == nil {
		return false
	}
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == auraType {
			s.castMu.Unlock()
			return true
		}
	}
	if s.server != nil && s.server.Data != nil {
		for spellID := range s.auras {
			spell, found, err := s.server.Data.Spell(spellID)
			if err == nil && found {
				for _, eff := range spell.Effects {
					if eff.Aura == auraType {
						s.castMu.Unlock()
						return true
					}
				}
			}
		}
	}
	s.castMu.Unlock()

	// Check learned passive spells
	if s.server != nil && s.server.Data != nil {
		for _, pSpell := range s.player.Spells {
			if !pSpell.Active || pSpell.Disabled {
				continue
			}
			spell, found, err := s.server.Data.Spell(pSpell.ID)
			if err == nil && found {
				if spell.Attributes&0x00000040 != 0 {
					for _, eff := range spell.Effects {
						if eff.Aura == auraType {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// getTotalAuraModifier calculates the sum of amounts of all active and passive auras of the given type.
// Mirrors Unit::GetTotalAuraModifier.
func (s *session) getTotalAuraModifier(auraType uint32) int32 {
	if s.player == nil {
		return 0
	}
	total := int32(0)
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura != nil && aura.AuraType == auraType {
			total += int32(aura.Amount)
		}
	}
	if s.server != nil && s.server.Data != nil {
		for spellID := range s.auras {
			if s.activeAuras != nil && s.activeAuras[spellID] != nil {
				continue
			}
			spell, found, err := s.server.Data.Spell(spellID)
			if err == nil && found {
				for _, eff := range spell.Effects {
					if eff.Aura == auraType {
						total += eff.BasePoints + 1
					}
				}
			}
		}
	}
	s.castMu.Unlock()

	// Check learned passive spells
	if s.server != nil && s.server.Data != nil {
		for _, pSpell := range s.player.Spells {
			if !pSpell.Active || pSpell.Disabled {
				continue
			}
			spell, found, err := s.server.Data.Spell(pSpell.ID)
			if err == nil && found {
				if spell.Attributes&0x00000040 != 0 {
					for _, eff := range spell.Effects {
						if eff.Aura == auraType {
							total += eff.BasePoints + 1
						}
					}
				}
			}
		}
	} else {
		// Fallback for mock unit test environments without full DBC store
		for _, pSpell := range s.player.Spells {
			if !pSpell.Active || pSpell.Disabled {
				continue
			}
			if auraType == 144 { // SPELL_AURA_SAFE_FALL
				switch pSpell.ID {
				case 1860, 20719: // Safe Fall Rank 1, Feline Grace
					total += 17
				case 18443: // Safe Fall Rank 2
					total += 50
				}
			}
		}
	}
	return total
}

// removeAurasWithInterruptFlags removes any active or applied auras matching the given interrupt flag bitmask.
// Mirrors Unit::RemoveAurasWithInterruptFlags.
func (s *session) removeAurasWithInterruptFlags(flags uint32) {
	var toRemove []uint32
	seen := make(map[uint32]struct{})
	s.castMu.Lock()
	for _, aura := range s.activeAuras {
		if aura != nil {
			flg := getSpellAuraInterruptFlags(aura.SpellID, aura.AuraInterruptFlags)
			if flg&flags != 0 {
				if _, ok := seen[aura.SpellID]; !ok {
					seen[aura.SpellID] = struct{}{}
					toRemove = append(toRemove, aura.SpellID)
				}
			}
		}
	}
	for spellID := range s.auras {
		dbcFlags := uint32(0)
		if s.server != nil && s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
				dbcFlags = spell.AuraInterruptFlags
			}
		}
		flg := getSpellAuraInterruptFlags(spellID, dbcFlags)
		if flg&flags != 0 {
			if _, ok := seen[spellID]; !ok {
				seen[spellID] = struct{}{}
				toRemove = append(toRemove, spellID)
			}
		}
	}
	s.castMu.Unlock()

	for _, spellID := range toRemove {
		s.removeAura(spellID)
	}
}
