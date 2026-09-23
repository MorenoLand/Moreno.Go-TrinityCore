package world

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

func ReplayCharacterLogin(ctx context.Context, server *Server, guid uint64) (protocoltrace.Trace, error) {
	return replayCharacterLogin(ctx, server, guid, 0)
}

func ReplayCharacterPetCooldown(ctx context.Context, server *Server, guid uint64, spellID uint32) (protocoltrace.Trace, error) {
	if spellID == 0 {
		return protocoltrace.Trace{}, errors.New("pet cooldown replay requires a spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, spellID)
}

func replayCharacterLogin(ctx context.Context, server *Server, guid uint64, petCooldownSpell uint32) (protocoltrace.Trace, error) {
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
	if petCooldownSpell != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay requires an active pet")
		}
		motion, motionFound := session.petMotionForCast(session.player.PetGUID)
		if !motionFound || !session.petKnowsSpell(ctx, motion, petCooldownSpell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay pet motion or known spell was not restored")
		}
		spell, spellFound, spellErr := server.Data.Spell(petCooldownSpell)
		if spellErr != nil || !spellFound || spell.Attributes&spellAttributePassive != 0 || motion.GUID != session.player.PetGUID || isHarmfulSpell(spell) && !isSelfCastOnly(spell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay spell cannot target the current pet")
		}
		categoryID, _, categoryErr := session.spellCooldownCategory(petCooldownSpell)
		if categoryErr != nil || categoryID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay spell has no DBC category")
		}
		server.motionMu.Lock()
		categoryEnd := motion.SpellCategoryCooldowns[categoryID]
		server.motionMu.Unlock()
		if !categoryEnd.After(time.Now()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet category cooldown was not restored from the database")
		}
		cast := protocol.NewBuffer(24)
		cast.WriteU64(session.player.PetGUID)
		cast.WriteU8(1)
		cast.WriteU32(petCooldownSpell)
		cast.WriteU8(0)
		cast.WriteU32(protocol.SpellTargetFlagUnit)
		cast.WritePackedGUID(session.player.PetGUID)
		target, targetErr := protocol.ReadSpellTargetData(protocol.NewReader(cast.Bytes()[14:]))
		if targetErr != nil || target.UnitGUID != session.player.PetGUID {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay target did not encode the current pet GUID")
		}
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PET_CAST_SPELL), cast.Bytes(), "pet-category-cooldown-check")
		if !session.handlePetCastSpell(ctx, cast.Bytes()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown cast handler rejected the replay")
		}
		trace := recorder.Snapshot()
		session.logout()
		if !petCastFailedNotReady(trace, petCooldownSpell) {
			return trace, errors.New("pet cast did not return SPELL_FAILED_NOT_READY for the persisted cooldown")
		}
		return trace, nil
	}
	trace := recorder.Snapshot()
	session.logout()
	return trace, nil
}

func petCastFailedNotReady(trace protocoltrace.Trace, spellID uint32) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_CAST_FAILED) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err == nil && len(payload) >= 6 && binary.LittleEndian.Uint32(payload[1:5]) == spellID && payload[5] == spellFailedNotReady {
			return true
		}
	}
	return false
}
