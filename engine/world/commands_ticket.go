package world

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ticket command port: ticket_commandscript (cs_ticket.cpp), the "ticket"
// root with 16 arms (assign, close, closedlist, comment, complete, delete,
// escalate, escalatedlist, list, onlinelist, reset, response[append|appendln],
// togglesystem, unassign, viewid, viewname). THIRTY-SEVENTH of 39 Commands
// groups (cs_script_loader.cpp decl 56 / call 101; call order tele(100) ->
// ticket(101)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487), so each arm gates exactly its own C++ permission
// (RBAC.h:610-628, 19 constants 742-760 in permissions.go; the root
// permission 742 covers the bare ".ticket").
//
// All arms work directly against the gm_ticket table (tickets.go), whose Go
// schema matches the C++ one. Assignment/conflict rules mirror the C++
// handlers: a ticket assigned to someone else cannot be commented/closed/
// completed/appended by another GM; close requires the ticket be assigned to
// the closer; unassign requires the invoker's security to cover the
// assignee's; escalate only works on unassigned, non-escalated tickets.
// `togglesystem` is native via a new Server.ticketsEnabled flag that gates
// ticket creation (tickets.go). LANG texts are inlined from the TDB enUS
// recall (no in-tree trinity_string seed).

// gmTicketRow mirrors the gm_ticket columns the command arms touch.
type gmTicketRow struct {
	id               uint32
	playerGuid       uint64
	name             string
	description      string
	createTime       int64
	lastModifiedTime int64
	closedBy         uint64
	assignedTo       uint64
	comment          string
	response         string
	completed        bool
	escalated        int64
	viewed           bool
}

// loadGMTicket fetches one ticket by id.
func (s *session) loadGMTicket(ctx context.Context, id uint32) (*gmTicketRow, bool) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return nil, false
	}
	t := &gmTicketRow{id: id}
	var closedBy, assignedTo, completed, viewed int64
	err := cdb.QueryRowContext(ctx, `SELECT playerGuid, name, description, createTime, lastModifiedTime,
		closedBy, assignedTo, comment, response, completed, escalated, viewed
		FROM gm_ticket WHERE id = ?`, id).Scan(
		&t.playerGuid, &t.name, &t.description, &t.createTime, &t.lastModifiedTime,
		&closedBy, &assignedTo, &t.comment, &t.response, &completed, &t.escalated, &viewed)
	if err != nil {
		return nil, false
	}
	t.closedBy, t.assignedTo = uint64(closedBy), uint64(assignedTo)
	t.completed, t.viewed = completed != 0, viewed != 0
	return t, true
}

// saveGMTicket persists the mutable columns.
func (s *session) saveGMTicket(ctx context.Context, t *gmTicketRow) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	completed := 0
	if t.completed {
		completed = 1
	}
	viewed := 0
	if t.viewed {
		viewed = 1
	}
	_, _ = cdb.ExecContext(ctx, `UPDATE gm_ticket SET lastModifiedTime = ?, closedBy = ?, assignedTo = ?,
		comment = ?, response = ?, completed = ?, escalated = ?, viewed = ? WHERE id = ?`,
		t.lastModifiedTime, t.closedBy, t.assignedTo, t.comment, t.response, completed, t.escalated, viewed, t.id)
}

// ticketAssignedName resolves the assignee's character name.
func (s *session) ticketAssignedName(ctx context.Context, guid uint64) string {
	if guid == 0 {
		return ""
	}
	var name string
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ?", guid).Scan(&name)
	}
	return name
}

// ticketFormat mirrors GmTicket::FormatMessageString for the list views.
func (s *session) ticketFormat(ctx context.Context, t *gmTicketRow) string {
	assigned := s.ticketAssignedName(ctx, t.assignedTo)
	status := "open"
	if t.closedBy != 0 {
		status = "closed"
	} else if t.completed {
		status = "completed"
	}
	return fmt.Sprintf("Ticket %d (%s) by %s: %s [assigned: %s]", t.id, status, t.name, t.description, assigned)
}

// handleCmdTicket dispatches the "ticket" root (cs_ticket.cpp:52-72).
func (s *session) handleCmdTicket(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandTicket) {
			return
		}
		s.sendSysMessage("Syntax: .ticket assign|close|closedlist|comment|complete|delete|escalate|escalatedlist|list|onlinelist|reset|response|togglesystem|unassign|viewid|viewname")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here.
	switch {
	case strings.HasPrefix("assign", sub) && sub != "unassign":
		s.handleTicketAssign(ctx, rest)
	case strings.HasPrefix("close", sub) && !strings.HasPrefix("closedlist", sub):
		s.handleTicketClose(ctx, rest)
	case strings.HasPrefix("closedlist", sub):
		s.handleTicketClosedList(ctx)
	case strings.HasPrefix("comment", sub):
		s.handleTicketComment(ctx, rest)
	case strings.HasPrefix("complete", sub):
		s.handleTicketComplete(ctx, rest)
	case strings.HasPrefix("delete", sub):
		s.handleTicketDelete(ctx, rest)
	case strings.HasPrefix("escalate", sub) && !strings.HasPrefix("escalatedlist", sub):
		s.handleTicketEscalate(ctx, rest)
	case strings.HasPrefix("escalatedlist", sub):
		s.handleTicketEscalatedList(ctx)
	case strings.HasPrefix("list", sub):
		s.handleTicketList(ctx, false)
	case strings.HasPrefix("onlinelist", sub):
		s.handleTicketList(ctx, true)
	case strings.HasPrefix("reset", sub):
		s.handleTicketReset(ctx)
	case strings.HasPrefix("response", sub):
		s.handleTicketResponse(ctx, rest)
	case strings.HasPrefix("togglesystem", sub):
		s.handleTicketToggleSystem(ctx)
	case strings.HasPrefix("unassign", sub):
		s.handleTicketUnassign(ctx, rest)
	case strings.HasPrefix("viewid", sub):
		s.handleTicketViewID(ctx, rest)
	case strings.HasPrefix("viewname", sub):
		s.handleTicketViewName(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .ticket assign|close|closedlist|comment|complete|delete|escalate|escalatedlist|list|onlinelist|reset|response|togglesystem|unassign|viewid|viewname")
	}
}

// ticketOpenByID loads the ticket and enforces the "exists and not closed"
// gate shared by most arms (LANG_COMMAND_TICKETNOTEXIST 2005).
func (s *session) ticketOpenByID(ctx context.Context, args []string, syntax string) (*gmTicketRow, bool) {
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return nil, false
	}
	t, ok := s.loadGMTicket(ctx, uint32(cAtoi(args[0])))
	if !ok || t.closedBy != 0 {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return nil, false
	}
	return t, true
}

// ticketAssignedNotTo mirrors the C++ "assigned to someone else" guard.
func (s *session) ticketAssignedNotTo(t *gmTicketRow) bool {
	return t.assignedTo != 0 && t.assignedTo != s.playerGUID
}

// handleTicketAssign mirrors HandleGMTicketAssignToCommand (cs_ticket.cpp:80).
func (s *session) handleTicketAssign(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketAssign) {
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .ticket assign <id> <name>")
		return
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket assign <id> <name>")
	if !ok {
		return
	}
	targetName := normalizePlayerName(args[1])
	var targetGuid uint64
	var targetAccount uint32
	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_ = cdb.QueryRowContext(ctx, "SELECT guid, account FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", targetName).Scan(&targetGuid, &targetAccount)
	}
	// Target must exist and have administrative rights (cs_ticket.cpp:103).
	canAssign := false
	if targetGuid != 0 && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		var sec uint8
		_ = s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT gmlevel FROM account WHERE id = ?", targetAccount).Scan(&sec)
		if hasPerm, err := accountHasPermission(ctx, s.server.AuthStore.DB, targetAccount, s.server.RealmID, sec, 32); err == nil && hasPerm { // RBAC_PERM_COMMANDS_BE_ASSIGNED_TICKET 32
			canAssign = true
		}
	}
	if !canAssign {
		s.sendSysMessage("Target must exist and have administrative rights.") // LANG_COMMAND_TICKETASSIGNERROR_A 2012
		return
	}
	if t.assignedTo == targetGuid {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned to that player.", t.id)) // LANG_COMMAND_TICKETASSIGNERROR_B 2013
		return
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
		return
	}
	t.assignedTo = targetGuid
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d assigned to %s.", t.id, targetName))
}

// handleTicketClose mirrors HandleGMTicketCloseByIdCommand (cs_ticket.cpp:130).
func (s *session) handleTicketClose(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketClose) {
		return
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket close <id>")
	if !ok || t.completed {
		if ok {
			s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		}
		return
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("You cannot close ticket %d.", t.id)) // LANG_COMMAND_TICKETCANNOTCLOSE 2016
		return
	}
	t.closedBy = s.playerGUID
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d closed.", t.id))
}

// handleTicketClosedList mirrors HandleGMTicketListClosedCommand.
func (s *session) handleTicketClosedList(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandTicketClosedlist) {
		return
	}
	s.ticketListQuery(ctx, "closedBy <> 0", "Closed tickets:")
}

// handleTicketComment mirrors HandleGMTicketCommentCommand (cs_ticket.cpp:171).
func (s *session) handleTicketComment(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketComment) {
		return
	}
	if len(args) < 2 {
		s.sendSysMessage("Syntax: .ticket comment <id> <comment>")
		return
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket comment <id> <comment>")
	if !ok {
		return
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
		return
	}
	t.comment = strings.Join(args[1:], " ")
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("%s added comment to ticket %d: %s", s.player.Name, t.id, t.comment)) // LANG_COMMAND_TICKETLISTADDCOMMENT 2024
}

// handleTicketComplete mirrors HandleGMTicketCompleteCommand (cs_ticket.cpp:210).
func (s *session) handleTicketComplete(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketComplete) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .ticket complete <id> [response]")
		return
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket complete <id> [response]")
	if !ok || t.completed {
		if ok {
			s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		}
		return
	}
	if len(args) > 1 {
		if s.ticketAssignedNotTo(t) {
			s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
			return
		}
		t.response += strings.Join(args[1:], " ")
	}
	t.completed = true
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d completed.", t.id))
}

// handleTicketDelete mirrors HandleGMTicketDeleteByIdCommand (cs_ticket.cpp:274).
func (s *session) handleTicketDelete(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketDelete) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .ticket delete <id>")
		return
	}
	t, ok := s.loadGMTicket(ctx, uint32(cAtoi(args[0])))
	if !ok {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return
	}
	if t.closedBy == 0 {
		s.sendSysMessage("Close the ticket first.") // LANG_COMMAND_TICKETCLOSEFIRST 2006
		return
	}
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM gm_ticket WHERE id = ?", t.id)
	}
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d deleted.", t.id))
}

// handleTicketEscalate mirrors HandleGMTicketEscalateCommand (cs_ticket.cpp:308).
func (s *session) handleTicketEscalate(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketEscalate) {
		return
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket escalate <id>")
	if !ok || t.completed || t.escalated != 0 {
		if ok {
			s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		}
		return
	}
	t.escalated = 1 // TICKET_IN_ESCALATION_QUEUE
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
	s.sendSysMessage(fmt.Sprintf("Ticket %d escalated.", t.id))
}

// handleTicketEscalatedList mirrors HandleGMTicketListEscalatedCommand.
func (s *session) handleTicketEscalatedList(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandTicketEscalatedlist) {
		return
	}
	s.ticketListQuery(ctx, "escalated = 1 AND closedBy = 0", "Escalated tickets:")
}

// ticketListQuery prints open/closed/escalated ticket lists.
func (s *session) ticketListQuery(ctx context.Context, where, header string) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	rows, err := cdb.QueryContext(ctx, "SELECT id FROM gm_ticket WHERE "+where+" ORDER BY id")
	if err != nil {
		return
	}
	defer rows.Close()
	s.sendSysMessage(header)
	empty := true
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if t, ok := s.loadGMTicket(ctx, id); ok {
			s.sendSysMessage(s.ticketFormat(ctx, t))
			empty = false
		}
	}
	if empty {
		s.sendSysMessage("Empty") // LANG_TICKET_LIST_EMPTY 2030-ish
	}
}

// handleTicketList mirrors HandleGMTicketListCommand /
// HandleGMTicketListOnlineCommand (cs_ticket.cpp:330-340).
func (s *session) handleTicketList(ctx context.Context, onlineOnly bool) {
	perm := permissionCommandTicketList
	if onlineOnly {
		perm = permissionCommandTicketOnlinelist
	}
	if s.miscDeny(ctx, perm) {
		return
	}
	if !onlineOnly {
		s.ticketListQuery(ctx, "closedBy = 0", "Open tickets:")
		return
	}
	// Restrict to tickets whose submitter is online.
	s.server.sessionsMu.RLock()
	var guids []uint64
	for sess := range s.server.sessions {
		if sess.player != nil {
			guids = append(guids, sess.playerGUID)
		}
	}
	s.server.sessionsMu.RUnlock()
	s.ticketListQueryOnline(ctx, guids, "Tickets from online players:")
}

// ticketListQueryOnline lists open tickets for the given player guids.
func (s *session) ticketListQueryOnline(ctx context.Context, guids []uint64, header string) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	s.sendSysMessage(header)
	empty := true
	for _, g := range guids {
		var id uint32
		if err := cdb.QueryRowContext(ctx, "SELECT id FROM gm_ticket WHERE playerGuid = ? AND closedBy = 0 LIMIT 1", g).Scan(&id); err != nil {
			continue
		}
		if t, ok := s.loadGMTicket(ctx, id); ok {
			s.sendSysMessage(s.ticketFormat(ctx, t))
			empty = false
		}
	}
	if empty {
		s.sendSysMessage("Empty")
	}
}

// handleTicketReset mirrors HandleGMTicketResetCommand (cs_ticket.cpp:343).
func (s *session) handleTicketReset(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandTicketReset) {
		return
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	var openCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM gm_ticket WHERE closedBy = 0").Scan(&openCount)
	if openCount > 0 {
		s.sendSysMessage("There are pending tickets.") // LANG_COMMAND_TICKETPENDING 2027
		return
	}
	_, _ = cdb.ExecContext(ctx, "DELETE FROM gm_ticket")
	s.sendSysMessage("Tickets reset.") // LANG_COMMAND_TICKETRESET 2028
}

// handleTicketToggleSystem mirrors HandleToggleGMTicketSystem (cs_ticket.cpp:358).
func (s *session) handleTicketToggleSystem(ctx context.Context) {
	if s.miscDeny(ctx, permissionCommandTicketTogglesystem) {
		return
	}
	enabled := !s.server.ticketsEnabled.Load()
	s.server.ticketsEnabled.Store(enabled)
	if enabled {
		s.sendSysMessage("Ticket system enabled.") // LANG_ALLOW_TICKETS 1134
	} else {
		s.sendSysMessage("Ticket system disabled.") // LANG_DISALLOW_TICKETS 1135
	}
}

// handleTicketUnassign mirrors HandleGMTicketUnAssignCommand (cs_ticket.cpp:366).
func (s *session) handleTicketUnassign(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketUnassign) {
		return
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket unassign <id>")
	if !ok {
		return
	}
	if t.assignedTo == 0 {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is not assigned.", t.id)) // LANG_COMMAND_TICKETNOTASSIGNED 2014
		return
	}
	// The invoker's security must cover the assignee's (cs_ticket.cpp:390).
	var assigneeSec uint8
	if sess := s.server.playerSessionForGUID(t.assignedTo); sess != nil {
		assigneeSec = sess.security
	} else if s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		var accountID uint32
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", t.assignedTo).Scan(&accountID)
		_ = s.server.AuthStore.DB.QueryRowContext(ctx, "SELECT gmlevel FROM account WHERE id = ?", accountID).Scan(&assigneeSec)
	}
	if assigneeSec > s.security {
		s.sendSysMessage("You cannot unassign a ticket assigned to a higher security level.") // LANG_COMMAND_TICKETUNASSIGNSECURITY 2015
		return
	}
	assignedName := s.ticketAssignedName(ctx, t.assignedTo)
	t.assignedTo = 0
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d unassigned from %s.", t.id, assignedName))
}

// handleTicketViewID mirrors HandleGMTicketGetByIdCommand (cs_ticket.cpp:419).
func (s *session) handleTicketViewID(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketViewid) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .ticket viewid <id>")
		return
	}
	t, ok := s.loadGMTicket(ctx, uint32(cAtoi(args[0])))
	if !ok || t.closedBy != 0 || t.completed {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return
	}
	t.viewed = true
	s.saveGMTicket(ctx, t)
	s.sendSysMessage(s.ticketFormat(ctx, t))
}

// handleTicketViewName mirrors HandleGMTicketGetByNameCommand (cs_ticket.cpp:447).
func (s *session) handleTicketViewName(ctx context.Context, args []string) {
	if s.miscDeny(ctx, permissionCommandTicketViewname) {
		return
	}
	if len(args) == 0 {
		s.sendSysMessage("Syntax: .ticket viewname <name>")
		return
	}
	name := normalizePlayerName(args[0])
	var guid uint64
	if sess := s.sessionForPlayerName(name); sess != nil && sess.player != nil {
		guid = sess.playerGUID
	} else if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", name).Scan(&guid)
	}
	if guid == 0 {
		s.sendSysMessage("Player not found.") // LANG_PLAYER_NOT_FOUND 499
		return
	}
	var id uint32
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil ||
		s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT id FROM gm_ticket WHERE playerGuid = ? AND closedBy = 0 LIMIT 1", guid).Scan(&id) != nil {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return
	}
	t, ok := s.loadGMTicket(ctx, id)
	if !ok {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return
	}
	t.viewed = true
	s.saveGMTicket(ctx, t)
	s.sendSysMessage(s.ticketFormat(ctx, t))
}

// handleTicketResponse dispatches the "ticket response" sub-table
// (cs_ticket.cpp:52-53): append, appendln.
func (s *session) handleTicketResponse(ctx context.Context, args []string) {
	if len(args) == 0 {
		if s.miscDeny(ctx, permissionCommandTicketResponse) {
			return
		}
		s.sendSysMessage("Syntax: .ticket response append|appendln <id> <text>")
		return
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	newLine := false
	switch {
	case strings.HasPrefix("appendln", sub):
		newLine = true
		if s.miscDeny(ctx, permissionCommandTicketResponseAppendln) {
			return
		}
	case strings.HasPrefix("append", sub):
		if s.miscDeny(ctx, permissionCommandTicketResponseAppend) {
			return
		}
	default:
		s.sendSysMessage("Syntax: .ticket response append|appendln <id> <text>")
		return
	}
	// Mirrors _HandleGMTicketResponseAppendCommand (cs_ticket.cpp:474).
	if len(rest) < 2 {
		s.sendSysMessage("Syntax: .ticket response append|appendln <id> <text>")
		return
	}
	t, ok := s.ticketOpenByID(ctx, rest, "Syntax: .ticket response append|appendln <id> <text>")
	if !ok {
		return
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
		return
	}
	t.response += strings.Join(rest[1:], " ")
	if newLine {
		t.response += "\n"
	}
	t.lastModifiedTime = time.Now().Unix()
	s.saveGMTicket(ctx, t)
}
