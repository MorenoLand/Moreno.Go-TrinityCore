package world

import (
	"context"
	"errors"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

func ReplayCharacterLogin(ctx context.Context, server *Server, guid uint64) (protocoltrace.Trace, error) {
	if server == nil || server.CharactersStore == nil || server.CharactersStore.DB == nil || guid == 0 {
		return protocoltrace.Trace{}, errors.New("login replay requires a server and character database")
	}
	server.sessionsMu.RLock()
	activeSessions := len(server.sessions)
	server.sessionsMu.RUnlock()
	if activeSessions != 0 || server.TraceRecorder != nil {
		return protocoltrace.Trace{}, errors.New("login replay requires an isolated server instance")
	}
	var accountID int64
	if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID); err != nil || accountID <= 0 || accountID > int64(^uint32(0)) {
		return protocoltrace.Trace{}, errors.New("login replay character was not found")
	}
	session := &session{server: server, authed: true, accountID: uint32(accountID), playerGUID: guid, legitimate: map[uint64]struct{}{guid: {}}, characterNames: make(map[uint64]enumCharacter), auras: make(map[uint32]struct{}), auraSlots: make(map[uint32]uint8), channels: make(map[string]struct{}), scale: 1, breathTimer: -1, fatigueTimer: -1, schoolLockouts: make(map[uint32]int64)}
	if server.AuthStore != nil && server.AuthStore.DB != nil {
		var security int64
		if server.AuthStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(gmlevel), 0) FROM account_access WHERE id = ? AND RealmID IN (?, -1)", accountID, server.RealmID).Scan(&security) == nil && security > 0 && security <= 255 {
			session.security = uint8(security)
		}
	}
	recorder := protocoltrace.NewRecorder("morenocore-in-process-login-replay")
	server.TraceRecorder = recorder
	defer func() { server.TraceRecorder = nil }()
	server.sessionsMu.Lock()
	server.sessions[session] = struct{}{}
	server.sessionsMu.Unlock()
	defer func() {
		server.sessionsMu.Lock()
		delete(server.sessions, session)
		server.sessionsMu.Unlock()
	}()
	packet := protocol.NewBuffer(8)
	packet.WriteU64(guid)
	recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), packet.Bytes(), "isolated-character-login")
	if !session.handlePlayerLogin(ctx, packet.Bytes()) {
		return recorder.Snapshot(), errors.New("character login handler rejected the replay")
	}
	trace := recorder.Snapshot()
	session.logout()
	return trace, nil
}
