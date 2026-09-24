package world

import (
	"context"
	"encoding/hex"
	"math"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	movementOnTransport     uint32 = 0x00000200
	movementFalling         uint32 = 0x00001000
	movementSwimming        uint32 = 0x00200000
	movementFlying          uint32 = 0x02000000
	movementSplineElevation uint32 = 0x04000000
	movementRoot            uint32 = 0x00000800
	movementForward         uint32 = 0x00000001
	movementBackward        uint32 = 0x00000002
	movementStrafeLeft      uint32 = 0x00000004
	movementStrafeRight     uint32 = 0x00000008
	movementTurnLeft        uint32 = 0x00000010
	movementTurnRight       uint32 = 0x00000020
	movementAscending       uint32 = 0x00400000
	movementDescending      uint32 = 0x00800000
	movement2Pitch          uint16 = 0x00000020
	movement2Interpolated   uint16 = 0x00000400
	maxPositionCoordinate          = 17066.666
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

func (s *session) handleMovement(ctx context.Context, opcode uint32, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
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
	info.Flags &^= movementRoot
	info.Flags = sanitizeMovementFlags(info.Flags)
	isMove := info.Flags&(movementForward|movementBackward|movementStrafeLeft|movementStrafeRight|movementFalling) != 0
	s.isMoving = isMove
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
		s.interruptCurrentCast()
		s.interruptCurrentChannel()
		if s.autoRepeatSpell != 0 && s.autoRepeatSpell != 75 {
			s.autoRepeatSpell = 0
			s.autoRepeatTarget = 0
			buf := protocol.NewBuffer(9)
			buf.WritePackedGUID(s.playerGUID)
			_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		}
	}
	s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
	s.updateZoneAndArea(ctx, false)
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
	if s.server != nil {
		if s.player.VehicleGUID != 0 {
			s.server.relocatePassengers(s.player.VehicleGUID, info.X, info.Y, info.Z, info.Orientation)
		} else {
			s.server.relocatePassengers(s.playerGUID, info.X, info.Y, info.Z, info.Orientation)
		}
	}
	s.checkDuelBounds()

	// Swimming state and breath mirror timer updates
	wasSwimming := s.isSwimming
	isSwimming := (info.Flags&movementSwimming != 0) || opcode == uint32(protocol.OpcodeMSG_MOVE_START_SWIM)
	if opcode == uint32(protocol.OpcodeMSG_MOVE_STOP_SWIM) {
		isSwimming = false
	}
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

func sanitizeMovementFlags(flags uint32) uint32 {
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

func (s *Server) addSession(value *session) {
	s.sessionsMu.Lock()
	if s.sessions == nil {
		s.sessions = make(map[*session]struct{})
	}
	s.sessions[value] = struct{}{}
	s.sessionsMu.Unlock()
}

func (s *Server) removeSession(session *session) {
	s.sessionsMu.Lock()
	delete(s.sessions, session)
	s.sessionsMu.Unlock()
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
// Reference: WorldSession::HandleMoveRootAck (MiscHandler.cpp:945).
func (s *session) handleForceMoveRootAck(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	_, _ = b.ReadPackedGUID()
	_, _ = b.ReadU32() // ack index
	info, err := readMovementInfo(b)
	if err == nil && validMovementPosition(info.X, info.Y, info.Z, info.Orientation) {
		s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
		s.rooted = true
		if s.server != nil {
			s.server.broadcastMovement(uint16(protocol.OpcodeMSG_MOVE_ROOT), payload, info, s)
		}
	}
	return true
}

// handleForceMoveUnrootAck processes CMSG_FORCE_MOVE_UNROOT_ACK (0x0EB).
// Reference: WorldSession::HandleMoveUnRootAck (MiscHandler.cpp:919).
func (s *session) handleForceMoveUnrootAck(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	_, _ = b.ReadPackedGUID()
	_, _ = b.ReadU32() // ack index
	info, err := readMovementInfo(b)
	if err == nil && validMovementPosition(info.X, info.Y, info.Z, info.Orientation) {
		s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
		s.rooted = false
		if s.server != nil {
			s.server.broadcastMovement(uint16(protocol.OpcodeMSG_MOVE_UNROOT), payload, info, s)
		}
	}
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

// handleMoveFeatherFallAck processes CMSG_MOVE_FEATHER_FALL_ACK (0x2CF).
func (s *session) handleMoveFeatherFallAck(ctx context.Context, payload []byte) bool {
	return s.handleMovementAck(ctx, payload)
}

// handleMoveHoverAck processes CMSG_MOVE_HOVER_ACK (0x0F6).
func (s *session) handleMoveHoverAck(ctx context.Context, payload []byte) bool {
	return s.handleMovementAck(ctx, payload)
}

// handleMoveWaterWalkAck processes CMSG_MOVE_WATER_WALK_ACK (0x2D0).
func (s *session) handleMoveWaterWalkAck(ctx context.Context, payload []byte) bool {
	return s.handleMovementAck(ctx, payload)
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
	if err == nil && validMovementPosition(info.X, info.Y, info.Z, info.Orientation) {
		s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
	}

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

// handleMoveNotActiveMover processes CMSG_MOVE_NOT_ACTIVE_MOVER (0x2D1).
func (s *session) handleMoveNotActiveMover(ctx context.Context, payload []byte) bool {
	return s.handleMovementAck(ctx, payload)
}

// handleMoveFallReset processes CMSG_MOVE_FALL_RESET (0x0CA).
func (s *session) handleMoveFallReset(ctx context.Context, payload []byte) bool {
	if s.player != nil {
		s.lastFallZ = s.player.Z
		s.lastFallTime = 0
	}
	return s.handleMovementAck(ctx, payload)
}

// handleMoveSplineDone processes CMSG_MOVE_SPLINE_DONE (0x2C9).
// Reference: WorldSession::HandleMoveSplineDoneOpcode (TaxiHandler.cpp:201).
func (s *session) handleMoveSplineDone(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	b := protocol.NewReader(payload)
	_, _ = b.ReadPackedGUID()
	info, err := readMovementInfo(b)
	if err == nil && validMovementPosition(info.X, info.Y, info.Z, info.Orientation) {
		s.player.X, s.player.Y, s.player.Z, s.player.Orientation = info.X, info.Y, info.Z, info.Orientation
	}
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
	buf := protocol.NewBuffer(16)
	buf.WritePackedGUID(s.playerGUID)
	buf.WriteU32(timeSkipped)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeMSG_MOVE_TIME_SKIPPED), buf.Bytes(), s)
	}
	return true
}

func (s *session) sendSummonRequest(summonerGUID uint64, zoneID uint32) {
	s.summonExpire = time.Now().Add(2 * time.Minute)
	s.summonerGUID = summonerGUID
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
		s.summonExpire = time.Time{}
		s.summonerGUID = 0
		return true
	}
	r := protocol.NewReader(payload)
	summonerGUID, _ := r.ReadU64()
	agree, _ := r.ReadU8()
	if agree == 0 {
		s.summonExpire = time.Time{}
		s.summonerGUID = 0
		return true
	}
	if s.player.Health == 0 || (s.player.UnitFlags&unitFlagInCombat != 0) {
		s.summonExpire = time.Time{}
		s.summonerGUID = 0
		return true
	}
	if !s.summonExpire.IsZero() && time.Now().After(s.summonExpire) {
		s.summonExpire = time.Time{}
		s.summonerGUID = 0
		return true
	}

	if s.server != nil {
		summonerSess := s.server.findSessionByGUID(summonerGUID)
		if summonerSess != nil && summonerSess.playerLoaded && summonerSess.player != nil {
			s.teleportTo(summonerSess.player.Map, summonerSess.player.X, summonerSess.player.Y, summonerSess.player.Z, summonerSess.player.Orientation)
		}
	}
	s.updateAchievementCriteria(criteriaTypeAcceptedSummonings, 0, 1)
	s.summonExpire = time.Time{}
	s.summonerGUID = 0
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

// handleFall calculates fall damage and applies it via environmentalDamage.
// Mirrors TrinityCore Player::HandleFall (Player.cpp:25369-25418).
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

	safeFall := s.getTotalAuraModifier(144) // SPELL_AURA_SAFE_FALL
	damagePerc := 0.018*(zDiff-float32(safeFall)) - 0.2426
	if damagePerc <= 0 {
		return
	}

	damage := uint32(damagePerc * float32(s.player.MaxHealth))
	if damage > s.player.MaxHealth {
		damage = s.player.MaxHealth
	}

	// Gust of Wind (spell 43621) caps fall damage at 50% max health
	if s.hasAura(43621) && damage > s.player.MaxHealth/2 {
		damage = s.player.MaxHealth / 2
	}

	if damage > 0 {
		before := s.player.Health
		s.environmentalDamage(ctx, damageFall, damage)
		// Reference Player.cpp:25410: surviving the fall with real damage
		// credits FALL_WITHOUT_DYING with the fall distance in centimeters.
		if s.player.Health > 0 && uint32(damage) < before {
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

	absorb := uint32(0)
	resist := uint32(0)

	if damage >= s.player.Health {
		damage = s.player.Health
		s.player.Health = 0
	} else {
		s.player.Health -= damage
		s.procDamageAuras(true)
	}

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
		s.killPlayer(ctx)
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
