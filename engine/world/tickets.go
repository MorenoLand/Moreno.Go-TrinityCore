package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	gmTicketQueueStatusEnabled    uint32 = 1
	gmTicketStatusDefault         uint32 = 10
	gmTicketStatusHasText         uint32 = 6
	gmTicketResponseCreateSuccess uint32 = 1
	gmTicketResponseCreateError   uint32 = 2
	gmTicketResponseUpdateSuccess uint32 = 1
	gmTicketResponseDeleted       uint32 = 9
)

// handleGMTicketSystemStatus processes CMSG_GMTICKET_SYSTEMSTATUS (0x21A).
// Reference: WorldSession::HandleGMTicketSystemStatusOpcode (TicketHandler.cpp:185).
func (s *session) handleGMTicketSystemStatus(ctx context.Context, payload []byte) bool {
	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketQueueStatusEnabled)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_SYSTEMSTATUS), buf.Bytes(), true) == nil
}

// handleGMTicketGetTicket processes CMSG_GMTICKET_GETTICKET (0x211).
// Reference: WorldSession::HandleGMTicketGetTicketOpcode (TicketHandler.cpp:170) and TicketMgr::SendTicket (TicketMgr.cpp:446).
func (s *session) handleGMTicketGetTicket(ctx context.Context, payload []byte) bool {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.player != nil {
		var ticketID uint32
		var message string
		var needMoreHelp uint8
		var lastModifiedTime int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT id, description, needMoreHelp, lastModifiedTime FROM gm_ticket WHERE playerGuid = ? AND closedBy = 0 LIMIT 1",
			s.playerGUID).Scan(&ticketID, &message, &needMoreHelp, &lastModifiedTime)
		if err == nil && ticketID > 0 {
			age := float32(time.Now().Unix() - lastModifiedTime)
			if age < 0 {
				age = 0
			}
			buf := protocol.NewBuffer(32 + len(message))
			buf.WriteU32(gmTicketStatusHasText)
			buf.WriteU32(ticketID)
			buf.WriteCString(message)
			buf.WriteU8(needMoreHelp)
			buf.WriteF32(age)
			buf.WriteF32(0) // oldest ticket age
			buf.WriteF32(0) // last change age
			buf.WriteU8(0)  // escalated
			buf.WriteU8(0)  // viewed
			return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_GETTICKET), buf.Bytes(), true) == nil
		}
	}

	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketStatusDefault)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_GETTICKET), buf.Bytes(), true) == nil
}

// handleGMTicketCreate processes CMSG_GMTICKET_CREATE (0x205).
// Reference: WorldSession::HandleGMTicketCreateOpcode (TicketHandler.cpp:34).
func (s *session) handleGMTicketCreate(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	mapId, _ := r.ReadU32()
	x, _ := r.ReadF32()
	y, _ := r.ReadF32()
	z, _ := r.ReadF32()
	message, _ := r.ReadCString()
	needResponse, _ := r.ReadU32()
	needMoreHelpBool, _ := r.ReadU8()

	// The GM ticket system can be toggled with `.ticket togglesystem`
	// (cs_ticket.cpp HandleToggleGMTicketSystem); disabled means no new
	// tickets are accepted.
	if s.server != nil && !s.server.ticketsEnabled.Load() {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(gmTicketResponseCreateError)
		return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_CREATE), buf.Bytes(), true) == nil
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.player != nil {
		cdb := s.server.CharactersStore.DB
		var nextID uint32 = 1
		_ = cdb.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) + 1 FROM gm_ticket").Scan(&nextID)
		now := time.Now().Unix()
		playerName := s.player.Name
		if playerName == "" {
			playerName = s.accountName
		}
		needMoreHelp := 0
		if needMoreHelpBool != 0 {
			needMoreHelp = 1
		}
		_, _ = cdb.ExecContext(ctx,
			`INSERT INTO gm_ticket (id, type, playerGuid, name, description, createTime, mapId, posX, posY, posZ, lastModifiedTime, closedBy, assignedTo, comment, response, completed, escalated, viewed, needMoreHelp, resolvedBy)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, '', '', 0, 0, 0, ?, 0)`,
			nextID, needResponse, s.playerGUID, playerName, message, now, mapId, x, y, z, now, needMoreHelp)
	}

	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketResponseCreateSuccess)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_CREATE), buf.Bytes(), true) == nil
}

// handleGMTicketUpdate processes CMSG_GMTICKET_UPDATETEXT (0x207).
// Reference: WorldSession::HandleGMTicketUpdateOpcode (TicketHandler.cpp:130).
func (s *session) handleGMTicketUpdate(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	message, _ := r.ReadCString()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		now := time.Now().Unix()
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE gm_ticket SET description = ?, lastModifiedTime = ? WHERE playerGuid = ? AND closedBy = 0",
			message, now, s.playerGUID)
	}

	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketResponseUpdateSuccess)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_UPDATETEXT), buf.Bytes(), true) == nil
}

// handleGMTicketDelete processes CMSG_GMTICKET_DELETETICKET (0x217).
// Reference: WorldSession::HandleGMTicketDeleteOpcode (TicketHandler.cpp:155).
func (s *session) handleGMTicketDelete(ctx context.Context, payload []byte) bool {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE gm_ticket SET closedBy = ? WHERE playerGuid = ? AND closedBy = 0",
			s.playerGUID, s.playerGUID)
	}

	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketResponseDeleted)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_DELETETICKET), buf.Bytes(), true) == nil
}

// handleGMResponseResolve processes CMSG_GMRESPONSE_RESOLVE (0x4F0).
// Reference: WorldSession::HandleGMResponseResolve (TicketHandler.cpp:272).
func (s *session) handleGMResponseResolve(ctx context.Context, payload []byte) bool {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE gm_ticket SET closedBy = ?, completed = 1 WHERE playerGuid = ? AND closedBy = 0",
			s.playerGUID, s.playerGUID)
	}

	bufStatus := protocol.NewBuffer(1)
	bufStatus.WriteU8(0) // getSurvey = 0
	_ = s.write(uint16(protocol.OpcodeSMSG_GMRESPONSE_STATUS_UPDATE), bufStatus.Bytes(), true)

	bufDel := protocol.NewBuffer(4)
	bufDel.WriteU32(gmTicketResponseDeleted)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_DELETETICKET), bufDel.Bytes(), true) == nil
}

// handleGMSurveySubmit processes CMSG_GMSURVEY_SUBMIT (0x32A).
// Reference: WorldSession::HandleGMSurveySubmit (TicketHandler.cpp:194).
func (s *session) handleGMSurveySubmit(ctx context.Context, payload []byte) bool {
	if len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	mainSurvey, _ := r.ReadU32()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		now := time.Now().Unix()
		res, err := cdb.ExecContext(ctx, "INSERT INTO gm_survey (guid, mainSurvey, comment, createTime) VALUES (?, ?, '', ?)", s.playerGUID, mainSurvey, now)
		if err == nil {
			surveyID, _ := res.LastInsertId()
			for i := 0; i < 10 && r.Remaining() >= 5; i++ {
				qID, _ := r.ReadU32()
				ans, _ := r.ReadU8()
				comm, _ := r.ReadCString()
				if qID > 0 {
					_, _ = cdb.ExecContext(ctx, "INSERT INTO gm_subsurvey (surveyId, questionId, answer, answerComment) VALUES (?, ?, ?, ?)", surveyID, qID, ans, comm)
				}
			}
		}
	}
	s.debug("gm survey submit", "account", s.accountName, "mainSurvey", mainSurvey)
	return true
}

// handleGMReportLag processes CMSG_GM_REPORT_LAG (0x502).
// Reference: WorldSession::HandleReportLag (TicketHandler.cpp:248).
func (s *session) handleGMReportLag(ctx context.Context, payload []byte) bool {
	if len(payload) < 20 {
		return true
	}
	r := protocol.NewReader(payload)
	lagType, _ := r.ReadU32()
	mapID, _ := r.ReadU32()
	x, _ := r.ReadF32()
	y, _ := r.ReadF32()
	z, _ := r.ReadF32()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		now := time.Now().Unix()
		_, _ = cdb.ExecContext(ctx, "INSERT INTO lag_reports (guid, lagType, mapId, posX, posY, posZ, latency, createTime) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			s.playerGUID, lagType, mapID, x, y, z, s.latency.Load(), now)
	}
	s.debug("gm report lag", "account", s.accountName, "lagType", lagType, "map", mapID)
	return true
}

// handleGmTicketSystemToggle processes CMSG_GMTICKETSYSTEM_TOGGLE (0x29A).
func (s *session) handleGmTicketSystemToggle(ctx context.Context, payload []byte) bool {
	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketQueueStatusEnabled)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_SYSTEMSTATUS), buf.Bytes(), true) == nil
}

// handleBug processes CMSG_BUG (0x1CA).
// Reference: WorldSession::HandleBugOpcode (MiscHandler.cpp:551).
func (s *session) handleBug(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	suggestion, err := r.ReadU32()
	if err != nil {
		return false
	}
	contentLen, err := r.ReadU32()
	if err != nil {
		return false
	}
	content, err := r.ReadString(int(contentLen))
	if err != nil {
		return false
	}
	typeLen, err := r.ReadU32()
	if err != nil {
		return false
	}
	typeStr, err := r.ReadString(int(typeLen))
	if err != nil {
		return false
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		// CHAR_INS_BUG_REPORT: INSERT INTO bugreport (type, content) VALUES(?, ?)
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "INSERT INTO bugreport (type, content) VALUES (?, ?)", typeStr, content)
	}

	s.debug("bug report received", "account", s.accountName, "suggestion", suggestion, "type", typeStr, "len", len(content))
	return true
}
