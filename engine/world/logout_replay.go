package world

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

func ReplayCharacterLogout(ctx context.Context, server *Server, guid uint64, cancel bool) (protocoltrace.Trace, error) {
	return replayCharacterLogout(ctx, server, guid, cancel, 0)
}

func ReplayCharacterSwitch(ctx context.Context, server *Server, guid, nextGUID uint64) (protocoltrace.Trace, error) {
	if guid == nextGUID {
		return protocoltrace.Trace{}, errors.New("character-switch replay requires two distinct GUIDs")
	}
	return replayCharacterLogout(ctx, server, guid, false, nextGUID)
}

func replayCharacterLogout(ctx context.Context, server *Server, guid uint64, cancel bool, switchGUID uint64) (protocoltrace.Trace, error) {
	if server == nil || server.CharactersStore == nil || server.CharactersStore.DB == nil || guid == 0 {
		return protocoltrace.Trace{}, errors.New("logout replay requires a server and character database")
	}
	if cancel && switchGUID != 0 {
		return protocoltrace.Trace{}, errors.New("logout-switch replay cannot cancel the first character logout")
	}
	server.sessionsMu.RLock()
	activeSessions := len(server.sessions)
	server.sessionsMu.RUnlock()
	if activeSessions != 0 || server.TraceRecorder != nil {
		return protocoltrace.Trace{}, errors.New("logout replay requires an isolated server instance")
	}
	sess, err := newReplayCharacterSession(ctx, server, guid)
	if err != nil {
		return protocoltrace.Trace{}, err
	}
	recorder := protocoltrace.NewRecorder("morenocore-in-process-logout-replay")
	server.TraceRecorder = recorder
	server.sessionsMu.Lock()
	if server.sessions == nil {
		server.sessions = make(map[*session]struct{})
	}
	server.sessions[sess] = struct{}{}
	server.sessionsMu.Unlock()
	defer func() {
		if sess.playerLoaded {
			sess.logout()
		}
		server.sessionsMu.Lock()
		delete(server.sessions, sess)
		server.sessionsMu.Unlock()
		server.TraceRecorder = nil
	}()
	login := protocol.NewBuffer(8)
	login.WriteU64(guid)
	recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), login.Bytes(), "logout-replay-login")
	if !sess.handlePlayerLogin(ctx, login.Bytes()) {
		return recorder.Snapshot(), errors.New("character login handler rejected the logout replay")
	}
	if err := sess.validateSavedInventoryDurationPayloads(ctx, recorder.Snapshot()); err != nil {
		return recorder.Snapshot(), err
	}
	if err := sess.validateLoginMovementAuraPackets(recorder.Snapshot()); err != nil {
		return recorder.Snapshot(), err
	}
	if err := validateWorldReadyFanout(server, sess); err != nil {
		return recorder.Snapshot(), err
	}
	if sess.player == nil {
		return recorder.Snapshot(), errors.New("logout replay requires a loaded player")
	}
	inCombat := sess.attackTarget != 0 || sess.player.UnitFlags&unitFlagInCombat != 0
	resting := sess.player.PlayerFlags&playerFlagResting != 0
	instantExpected := (resting && !inCombat) || sess.inFlight
	if inCombat && !resting || sess.isFalling || sess.duelPartner != 0 || sess.hasAura(9454) {
		return recorder.Snapshot(), errors.New("logout replay character is in a source-rejected logout state")
	}
	if cancel && instantExpected {
		return recorder.Snapshot(), errors.New("instant logout cannot be canceled")
	}
	if switchGUID != 0 && instantExpected {
		return recorder.Snapshot(), errors.New("character-switch replay requires a delayed first logout")
	}
	requestLogout := func(statePrefix string) error {
		if sess.player == nil || sess.player.PlayerFlags&playerFlagResting != 0 || (sess.attackTarget != 0 || sess.player.UnitFlags&unitFlagInCombat != 0) || sess.inFlight || sess.isFalling || sess.duelPartner != 0 || sess.hasAura(9454) {
			return errors.New("delayed logout replay has a source-rejected state")
		}
		sess.traceStatePrefix = statePrefix
		authStore := server.AuthStore
		server.AuthStore = nil
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST), nil, statePrefix+" logout-request")
		requestOK := sess.handleLogoutRequest(ctx)
		server.AuthStore = authStore
		if !requestOK || sess.logoutAt.IsZero() || !sess.logoutFlagsApplied || !sess.rooted {
			return errors.New("normal logout request did not start a rooted countdown")
		}
		return nil
	}
	completeDelayedLogout := func() error {
		if sess.logoutAt.IsZero() {
			return errors.New("logout replay has no pending countdown to expire")
		}
		sess.logoutAt = time.Now().Add(-time.Second)
		return sess.completeLogout(ctx)
	}
	if cancel {
		if err := requestLogout(""); err != nil {
			return recorder.Snapshot(), err
		}
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_LOGOUT_CANCEL), nil, "logout-cancel")
		if !sess.handleLogoutCancel() {
			return recorder.Snapshot(), errors.New("logout cancel handler rejected the replay")
		}
		if !sess.logoutAt.IsZero() || sess.logoutFlagsApplied || sess.rooted {
			return recorder.Snapshot(), errors.New("logout cancel did not clear the countdown and root state")
		}
		sess.logout()
	} else if instantExpected {
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST), nil, "logout-request")
		requestOK := sess.handleLogoutRequest(ctx)
		if !requestOK || sess.playerLoaded || sess.player != nil {
			return recorder.Snapshot(), errors.New("source instant logout did not complete immediately")
		}
	} else {
		if err := requestLogout(""); err != nil {
			return recorder.Snapshot(), err
		}
		if err := completeDelayedLogout(); err != nil {
			return recorder.Snapshot(), fmt.Errorf("complete delayed logout: %w", err)
		}
	}
	if switchGUID != 0 {
		var nextAccount int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", switchGUID).Scan(&nextAccount); err != nil || nextAccount != int64(sess.accountID) {
			return recorder.Snapshot(), errors.New("character-switch replay requires a second character on the same account")
		}
		sess.traceStatePrefix = "character-switch"
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_CHAR_ENUM), nil, "character-switch-enum")
		if !sess.handleCharEnum(ctx) {
			return recorder.Snapshot(), errors.New("same-session character enumeration was rejected")
		}
		login = protocol.NewBuffer(8)
		login.WriteU64(switchGUID)
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), login.Bytes(), "character-switch-login")
		if !sess.handlePlayerLogin(ctx, login.Bytes()) {
			return recorder.Snapshot(), errors.New("same-session second character login was rejected")
		}
		if err := sess.validateSavedInventoryDurationPayloads(ctx, recorder.Snapshot()); err != nil {
			return recorder.Snapshot(), err
		}
		if err := sess.validateLoginMovementAuraPackets(recorder.Snapshot()); err != nil {
			return recorder.Snapshot(), err
		}
		if err := validateWorldReadyFanout(server, sess); err != nil {
			return recorder.Snapshot(), err
		}
		if err := requestLogout("character-switch"); err != nil {
			return recorder.Snapshot(), fmt.Errorf("request logout for switched character: %w", err)
		}
		if err := completeDelayedLogout(); err != nil {
			return recorder.Snapshot(), fmt.Errorf("logout switched character: %w", err)
		}
	}
	if sess.playerLoaded || sess.player != nil {
		return recorder.Snapshot(), errors.New("logout completion retained the player session")
	}
	checkGUIDs := []uint64{guid}
	if switchGUID != 0 {
		checkGUIDs = append(checkGUIDs, switchGUID)
	}
	for _, checkGUID := range checkGUIDs {
		var characterOnline int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT online FROM characters WHERE guid = ?", checkGUID).Scan(&characterOnline); err != nil {
			return recorder.Snapshot(), fmt.Errorf("read post-logout character %d online state: %w", checkGUID, err)
		}
		if characterOnline != 0 {
			return recorder.Snapshot(), fmt.Errorf("post-logout character %d online=%d, want 0", checkGUID, characterOnline)
		}
	}
	if server.AuthStore != nil && server.AuthStore.DB != nil {
		var accountOnline int64
		if err := server.AuthStore.DB.QueryRowContext(ctx, "SELECT online FROM account WHERE id = ?", sess.accountID).Scan(&accountOnline); err != nil {
			return recorder.Snapshot(), fmt.Errorf("read post-logout account online state: %w", err)
		}
		if accountOnline != 0 {
			return recorder.Snapshot(), fmt.Errorf("post-logout account online=%d, want 0", accountOnline)
		}
	}
	if err := validateLogoutReplayTrace(recorder.Snapshot(), cancel, switchGUID != 0, instantExpected); err != nil {
		return recorder.Snapshot(), err
	}
	return recorder.Snapshot(), nil
}

func validateLogoutReplayTrace(trace protocoltrace.Trace, cancel, switched, instant bool) error {
	expectedLogoutCount := 1
	if switched {
		expectedLogoutCount = 2
	}
	responseIndexes, rootIndexes, completeIndexes := make([]int, 0, expectedLogoutCount), make([]int, 0, expectedLogoutCount), make([]int, 0, expectedLogoutCount)
	ackIndex, unrootIndex := -1, -1
	ackCount, completeCount := 0, 0
	for index, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		switch event.Opcode {
		case uint32(protocol.OpcodeSMSG_LOGOUT_RESPONSE):
			responseIndexes = append(responseIndexes, index)
			payload, err := trace.Payload(event)
			if err != nil {
				return err
			}
			wantInstant := byte(0)
			if instant {
				wantInstant = 1
			}
			if len(payload) != 5 || binary.LittleEndian.Uint32(payload[:4]) != 0 || payload[4] != wantInstant {
				return fmt.Errorf("unexpected delayed logout response payload=%x", payload)
			}
		case uint32(protocol.OpcodeSMSG_FORCE_MOVE_ROOT):
			rootIndexes = append(rootIndexes, index)
		case uint32(protocol.OpcodeSMSG_LOGOUT_CANCEL_ACK):
			ackCount++
			ackIndex = index
		case uint32(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT):
			unrootIndex = index
		case uint32(protocol.OpcodeSMSG_LOGOUT_COMPLETE):
			completeCount++
			completeIndexes = append(completeIndexes, index)
		}
	}
	if len(responseIndexes) != expectedLogoutCount || len(rootIndexes) != expectedLogoutCount {
		if instant && !cancel && !switched && ackCount == 0 && unrootIndex < 0 && len(responseIndexes) == 1 && len(rootIndexes) == 0 && completeCount == 1 && len(completeIndexes) == 1 && completeIndexes[0] > responseIndexes[0] {
			return nil
		}
		return fmt.Errorf("logout response/root count responses=%d roots=%d want=%d", len(responseIndexes), len(rootIndexes), expectedLogoutCount)
	}
	if cancel {
		if ackCount != 1 || ackIndex <= rootIndexes[0] || unrootIndex <= ackIndex || completeCount != 0 {
			return fmt.Errorf("logout cancel order/count root=%d ack=%d unroot=%d complete-count=%d", rootIndexes[0], ackIndex, unrootIndex, completeCount)
		}
		return nil
	}
	if ackCount != 0 || completeCount != expectedLogoutCount || len(completeIndexes) != expectedLogoutCount || unrootIndex >= 0 {
		return fmt.Errorf("logout completion count responses=%d completes=%d ack-count=%d unroot=%d want=%d", len(responseIndexes), completeCount, ackCount, unrootIndex, expectedLogoutCount)
	}
	for index := range responseIndexes {
		if rootIndexes[index] <= responseIndexes[index] || completeIndexes[index] <= rootIndexes[index] {
			return fmt.Errorf("logout packet order response=%d root=%d complete=%d", responseIndexes[index], rootIndexes[index], completeIndexes[index])
		}
	}
	return nil
}
