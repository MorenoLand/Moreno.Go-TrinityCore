package world

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	gmTicketQueueStatusEnabled    uint32 = 1
	gmTicketStatusDefault         uint32 = 10
	gmTicketStatusHasText         uint32 = 6
	gmTicketResponseAlreadyExist  uint32 = 1
	gmTicketResponseCreateSuccess uint32 = 2
	gmTicketResponseCreateError   uint32 = 3
	gmTicketResponseUpdateSuccess uint32 = 4
	gmTicketResponseUpdateError   uint32 = 5
	gmTicketResponseDeleted       uint32 = 9
)

// GMTicketResponse (TicketMgr.h:43): ALREADY_EXIST=1, CREATE_SUCCESS=2,
// CREATE_ERROR=3, UPDATE_SUCCESS=4, UPDATE_ERROR=5, TICKET_DELETED=9.

// sendGMTicketDefault mirrors TicketMgr::SendTicket(session, nullptr)
// (TicketMgr.cpp:446): the no-ticket SMSG_GMTICKET_GETTICKET state.
func (s *session) sendGMTicketDefault() {
	buf := protocol.NewBuffer(4)
	buf.WriteU32(gmTicketStatusDefault)
	_ = s.write(uint16(protocol.OpcodeSMSG_GMTICKET_GETTICKET), buf.Bytes(), true)
}

// sendQueryTimeResponse mirrors WorldSession::SendQueryTimeResponse
// (QueryHandler.cpp:83). The daily-quest reset delta has no Go model,
// so it is sent as 0 (documented delta).
func (s *session) sendQueryTimeResponse() {
	now := uint32(time.Now().Unix())
	buf := protocol.NewBuffer(8)
	buf.WriteU32(now)
	buf.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_QUERY_TIME_RESPONSE), buf.Bytes(), true)
}

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
	s.sendQueryTimeResponse()

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.player != nil {
		var ticketID uint32
		var message string
		var response string
		var needMoreHelp uint8
		var lastModifiedTime int64
		var completed int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx,
			"SELECT id, description, response, needMoreHelp, lastModifiedTime, completed FROM gm_ticket WHERE playerGuid = ? AND closedBy = 0 LIMIT 1",
			s.playerGUID).Scan(&ticketID, &message, &response, &needMoreHelp, &lastModifiedTime, &completed)
		if err == nil && ticketID > 0 {
			if completed != 0 {
				// TicketHandler.cpp:174: completed tickets answer with the
				// GM response (GmTicket::SendResponse, TicketMgr.cpp:147).
				s.sendGMResponseReceived(ticketID, message, response)
				return true
			}
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

	s.sendGMTicketDefault()
	return true
}

// sendGMResponseReceived mirrors GmTicket::SendResponse (TicketMgr.cpp:147):
// SMSG_GMRESPONSE_RECEIVED carries u32(1), the ticket id, the ticket message,
// then the GM response text in four 3999-byte null-terminated chunks.
func (s *session) sendGMResponseReceived(ticketID uint32, message, response string) {
	buf := protocol.NewBuffer(16 + len(message) + len(response))
	buf.WriteU32(1)
	buf.WriteU32(ticketID)
	buf.WriteCString(message)
	rest := response
	for i := 0; i < 4; i++ {
		chunk := rest
		if len(chunk) > 3999 {
			chunk = chunk[:3999]
			rest = rest[3999:]
		} else {
			rest = ""
		}
		for j := 0; j < len(chunk); j++ {
			buf.WriteU8(chunk[j])
		}
		buf.WriteU8(0)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_GMRESPONSE_RECEIVED), buf.Bytes(), true)
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

	// TicketHandler.cpp:69: an invalid hyperlink in the ticket text drops
	// the create.
	if !s.validateHyperlinksAndMaybeKick(ctx, message) {
		return true
	}

	// TicketHandler.cpp:38: a disabled ticket queue silently drops the
	// create — no response packet is sent on this arm.
	if s.server != nil && !s.server.ticketsEnabled.Load() {
		return true
	}

	// TicketHandler.cpp:41: LevelReq.Ticket gate with the LANG_TICKET_REQ
	// notification (no trinity_string bridge; plain text used).
	if s.server != nil && s.player != nil && uint32(s.player.Level) < s.server.Config.TicketLevelReq {
		s.sendNotification("You need to be level " + strconv.FormatUint(uint64(s.server.Config.TicketLevelReq), 10) + " to use the ticket system.")
		return true
	}

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.player != nil {
		cdb := s.server.CharactersStore.DB
		var existingID uint32
		var existingCompleted int64
		_ = cdb.QueryRowContext(ctx, "SELECT id, completed FROM gm_ticket WHERE playerGuid = ? AND closedBy = 0 LIMIT 1",
			s.playerGUID).Scan(&existingID, &existingCompleted)
		if existingID > 0 {
			if existingCompleted == 0 {
				// TicketHandler.cpp:52: an open ticket blocks creation;
				// the response stays CREATE_ERROR.
				buf := protocol.NewBuffer(4)
				buf.WriteU32(gmTicketResponseCreateError)
				return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_CREATE), buf.Bytes(), true) == nil
			}
			// TicketHandler.cpp:49: a completed ticket is closed first.
			_, _ = cdb.ExecContext(ctx, "UPDATE gm_ticket SET closedBy = ? WHERE id = ?", s.playerGUID, existingID)
		}
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

	// TicketHandler.cpp:135: an invalid hyperlink in the update text drops
	// the update.
	if !s.validateHyperlinksAndMaybeKick(ctx, message) {
		return true
	}

	// TicketHandler.cpp:135: the update only applies to an existing open
	// ticket; otherwise the response is UPDATE_ERROR.
	response := gmTicketResponseUpdateError
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		now := time.Now().Unix()
		res, _ := s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE gm_ticket SET description = ?, lastModifiedTime = ? WHERE playerGuid = ? AND closedBy = 0",
			message, now, s.playerGUID)
		if n, _ := res.RowsAffected(); n > 0 {
			response = gmTicketResponseUpdateSuccess
		}
	}

	buf := protocol.NewBuffer(4)
	buf.WriteU32(response)
	return s.write(uint16(protocol.OpcodeSMSG_GMTICKET_UPDATETEXT), buf.Bytes(), true) == nil
}

// handleGMTicketDelete processes CMSG_GMTICKET_DELETETICKET (0x217).
// Reference: WorldSession::HandleGMTicketDeleteOpcode (TicketHandler.cpp:155).
func (s *session) handleGMTicketDelete(ctx context.Context, payload []byte) bool {
	// TicketHandler.cpp:155: the delete only acts on an existing open
	// ticket, then re-sends the (now empty) ticket state.
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		res, _ := s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE gm_ticket SET closedBy = ? WHERE playerGuid = ? AND closedBy = 0",
			s.playerGUID, s.playerGUID)
		if n, _ := res.RowsAffected(); n > 0 {
			buf := protocol.NewBuffer(4)
			buf.WriteU32(gmTicketResponseDeleted)
			_ = s.write(uint16(protocol.OpcodeSMSG_GMTICKET_DELETETICKET), buf.Bytes(), true)
			s.sendGMTicketDefault()
		}
	}
	return true
}

// handleGMResponseResolve processes CMSG_GMRESPONSE_RESOLVE (0x4F0).
// Reference: WorldSession::HandleGMResponseResolve (TicketHandler.cpp:272).
func (s *session) handleGMResponseResolve(ctx context.Context, payload []byte) bool {
	// TicketHandler.cpp:272: the resolve only acts on an existing open
	// ticket; the survey prompt rolls against ChanceOfGMSurvey.
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		res, _ := s.server.CharactersStore.DB.ExecContext(ctx,
			"UPDATE gm_ticket SET closedBy = ?, completed = 1 WHERE playerGuid = ? AND closedBy = 0",
			s.playerGUID, s.playerGUID)
		if n, _ := res.RowsAffected(); n > 0 {
			var getSurvey uint8
			if rand.Float64()*100 < s.server.Config.ChanceOfGMSurvey {
				getSurvey = 1
			}

			bufStatus := protocol.NewBuffer(1)
			bufStatus.WriteU8(getSurvey)
			_ = s.write(uint16(protocol.OpcodeSMSG_GMRESPONSE_STATUS_UPDATE), bufStatus.Bytes(), true)

			bufDel := protocol.NewBuffer(4)
			bufDel.WriteU32(gmTicketResponseDeleted)
			_ = s.write(uint16(protocol.OpcodeSMSG_GMTICKET_DELETETICKET), bufDel.Bytes(), true)

			s.sendGMTicketDefault()
		}
	}
	return true
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
			// TicketHandler.cpp:209: the same sub-survey is never stored
			// twice for one survey.
			seen := make(map[uint32]bool)
			for i := 0; i < 10 && r.Remaining() >= 5; i++ {
				qID, _ := r.ReadU32()
				ans, _ := r.ReadU8()
				comm, _ := r.ReadCString()
				if qID == 0 || seen[qID] {
					continue
				}
				seen[qID] = true
				// TicketHandler.cpp:220: an invalid hyperlink in a
				// sub-survey comment drops the whole submit.
				if !s.validateHyperlinksAndMaybeKick(ctx, comm) {
					return true
				}
				_, _ = cdb.ExecContext(ctx, "INSERT INTO gm_subsurvey (surveyId, questionId, answer, answerComment) VALUES (?, ?, ?, ?)", surveyID, qID, ans, comm)
			}
			// TicketHandler.cpp:228: the trailing comment belongs to the
			// survey row itself.
			if finalComment, err := r.ReadCString(); err == nil && finalComment != "" {
				// TicketHandler.cpp:234: an invalid hyperlink in the
				// survey comment drops the submit (the sub-survey rows
				// above already landed, matching C++'s per-comment
				// return).
				if !s.validateHyperlinksAndMaybeKick(ctx, finalComment) {
					return true
				}
				_, _ = cdb.ExecContext(ctx, "UPDATE gm_survey SET comment = ? WHERE surveyId = ?", finalComment, surveyID)
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
