package world

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// ticket command port: ticket_commandscript (cs_ticket.cpp), the "ticket"
// root with 16 arms (assign, close, closedlist, comment, complete, delete,
// escalate, escalatedlist, list, onlinelist, reset, response[append|appendln],
// togglesystem, unassign, viewid, viewname). THIRTY-EIGHTH of 40 Commands
// groups (cs_script_loader.cpp decl 56 / call 101; call order tele(100) ->
// ticket(101)). Trinity checks permission only on the invoker leaf node
// (ChatCommand.cpp:487); the "ticket" root and the "response" node both use
// the deprecated 6-arg nullptr+subtable overload (ChatCommand.h:262-264
// drops the RBACPermissions param, pure SubCommandEntry containers), so the
// root perm 742 and the response-node perm 754 are DEAD in C++ — bare
// ".ticket" and bare ".ticket response" print their help listing ungated
// (same dead-root-perm pattern as the mmap/modify/npc/quest/pet/rbac/reload/
// reset ports); permissionCommandTicket/permissionCommandTicketResponse stay
// in permissions.go as documentation. All live arm perms are exact
// (RBAC.h:610-628: 743-753, 755-760).
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
//
// Documented no-bridge/deltas: GmTicket::FormatMessageString is rendered as
// a compact one-line summary rather than the multi-line C++ layout;
// comment/response text joins the tokenized args with single spaces (C++
// takes the raw rest-of-line via strtok, so leading/multiple spaces are not
// preserved — the framework tokenizes before the handler runs). The
// submitter packets ARE bridged: close/delete send SMSG_GMTICKET_DELETETICKET,
// complete sends SMSG_GMRESPONSE_RECEIVED, escalate sends SMSG_GMTICKET_GETTICKET
// (sendGMTicketState, tickets.go).

// gmTicketRow mirrors the gm_ticket columns the command arms touch.
type gmTicketRow struct {
	id               uint32
	ticketType       int64
	resolvedBy       uint64
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
	needMoreHelp     uint8
}

// loadGMTicket fetches one ticket by id.
func (s *session) loadGMTicket(ctx context.Context, id uint32) (*gmTicketRow, bool) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return nil, false
	}
	t := &gmTicketRow{id: id}
	var ticketType, closedBy, assignedTo, completed, viewed, resolvedBy, needMoreHelp int64
	err := cdb.QueryRowContext(ctx, `SELECT type, playerGuid, name, description, createTime, lastModifiedTime,
		closedBy, assignedTo, comment, response, completed, escalated, viewed, resolvedBy, needMoreHelp
		FROM gm_ticket WHERE id = ?`, id).Scan(
		&ticketType, &t.playerGuid, &t.name, &t.description, &t.createTime, &t.lastModifiedTime,
		&closedBy, &assignedTo, &t.comment, &t.response, &completed, &t.escalated, &viewed, &resolvedBy, &needMoreHelp)
	if err != nil {
		return nil, false
	}
	t.closedBy, t.assignedTo = uint64(closedBy), uint64(assignedTo)
	t.completed, t.viewed = completed != 0, viewed != 0
	t.ticketType, t.resolvedBy = ticketType, uint64(resolvedBy)
	t.needMoreHelp = uint8(needMoreHelp)
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
	_, _ = cdb.ExecContext(ctx, `UPDATE gm_ticket SET type = ?, lastModifiedTime = ?, closedBy = ?, assignedTo = ?,
		comment = ?, response = ?, completed = ?, escalated = ?, viewed = ?, resolvedBy = ? WHERE id = ?`,
		t.ticketType, t.lastModifiedTime, t.closedBy, t.assignedTo, t.comment, t.response, completed, t.escalated, viewed, t.resolvedBy, t.id)
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
	// IsClosed() is type-based (TicketMgr.h:97), not closedBy-based: a
	// character-deleted ticket (type 2, CharacterDatabase.cpp:328) has closedBy 0
	// and still counts as closed.
	if t.ticketType != 0 {
		status = "closed"
	} else if t.completed {
		status = "completed"
	}
	return fmt.Sprintf("Ticket %d (%s) by %s: %s [assigned: %s]", t.id, status, t.name, t.description, assigned)
}

// handleCmdTicket dispatches the "ticket" root (cs_ticket.cpp:52-72).
// The bool reports C++ handler success for the whole family: RBAC denial
// and the C++ silent-false parse paths (empty args, missing token, bad
// name) return false so dispatchCommand shows the node's help listing
// (ChatCommand.cpp:296-309); paths that print a message return true.
func (s *session) handleCmdTicket(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Dead root perm (see header): bare ".ticket" prints the syntax
		// listing with no gate, == C++.
		s.sendSysMessage("Syntax: .ticket assign|close|closedlist|comment|complete|delete|escalate|escalatedlist|list|onlinelist|reset|response|togglesystem|unassign|viewid|viewname")
		return true
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	// The Trinity parser prefix-matches command names at every nesting
	// level; match the arm the same way here — except an exact name wins
	// over a longer name it prefixes (ChatCommand.cpp:265-285: the
	// ambiguity check runs only for non-exact matches), so bare "close"
	// must reach the close arm, not closedlist, and "escalate" must reach
	// the escalate arm, not escalatedlist. A proper prefix matching two
	// or more arms ("c", "cl", "r", "v", ...) is ambiguous in C++ and
	// prints the syntax line instead of silently resolving.
	{
		ticketArms := []string{"assign", "close", "closedlist", "comment", "complete", "delete", "escalate", "escalatedlist", "list", "onlinelist", "reset", "response", "togglesystem", "unassign", "viewid", "viewname"}
		exact, matches := false, 0
		for _, name := range ticketArms {
			if name == sub {
				exact = true
			}
			if strings.HasPrefix(name, sub) {
				matches++
			}
		}
		if !exact && matches > 1 {
			s.sendSysMessage("Syntax: .ticket assign|close|closedlist|comment|complete|delete|escalate|escalatedlist|list|onlinelist|reset|response|togglesystem|unassign|viewid|viewname")
			return true
		}
	}
	switch {
	case strings.HasPrefix("assign", sub) && sub != "unassign":
		return s.handleTicketAssign(ctx, rest)
	case strings.HasPrefix("close", sub):
		return s.handleTicketClose(ctx, rest)
	case strings.HasPrefix("closedlist", sub):
		return s.handleTicketClosedList(ctx)
	case strings.HasPrefix("comment", sub):
		return s.handleTicketComment(ctx, rest)
	case strings.HasPrefix("complete", sub):
		return s.handleTicketComplete(ctx, rest)
	case strings.HasPrefix("delete", sub):
		return s.handleTicketDelete(ctx, rest)
	case strings.HasPrefix("escalate", sub):
		return s.handleTicketEscalate(ctx, rest)
	case strings.HasPrefix("escalatedlist", sub):
		return s.handleTicketEscalatedList(ctx)
	case strings.HasPrefix("list", sub):
		return s.handleTicketList(ctx, false)
	case strings.HasPrefix("onlinelist", sub):
		return s.handleTicketList(ctx, true)
	case strings.HasPrefix("reset", sub):
		return s.handleTicketReset(ctx)
	case strings.HasPrefix("response", sub):
		return s.handleTicketResponse(ctx, rest)
	case strings.HasPrefix("togglesystem", sub):
		return s.handleTicketToggleSystem(ctx)
	case strings.HasPrefix("unassign", sub):
		return s.handleTicketUnassign(ctx, rest)
	case strings.HasPrefix("viewid", sub):
		return s.handleTicketViewID(ctx, rest)
	case strings.HasPrefix("viewname", sub):
		return s.handleTicketViewName(ctx, rest)
	default:
		s.sendSysMessage("Syntax: .ticket assign|close|closedlist|comment|complete|delete|escalate|escalatedlist|list|onlinelist|reset|response|togglesystem|unassign|viewid|viewname")
	}
	return true
}

// ticketOpenByID loads the ticket and enforces the "exists and not closed"
// gate shared by most arms (LANG_COMMAND_TICKETNOTEXIST 2005).
func (s *session) ticketOpenByID(ctx context.Context, args []string, syntax string) (*gmTicketRow, bool) {
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return nil, false
	}
	t, ok := s.loadGMTicket(ctx, uint32(cAtoi(args[0])))
	// GmTicket::IsClosed() is type-based (TicketMgr.h:97): a ticket whose
	// submitter's character was deleted carries type 2 with closedBy 0
	// (CharacterDatabase.cpp:328), and is closed in C++.
	if !ok || t.ticketType != 0 {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return nil, false
	}
	return t, true
}

// ticketAssignedNotTo mirrors the C++ "assigned to someone else" guard.
func (s *session) ticketAssignedNotTo(t *gmTicketRow) bool {
	return t.assignedTo != 0 && t.assignedTo != s.playerGUID
}

// sendTicketDeletedToSubmitter mirrors the SMSG_GMTICKET_DELETETICKET leg of
// the close/delete arms (cs_ticket.cpp:163-168, 296-301): the online
// submitter is told the ticket is gone (GMTICKET_RESPONSE_TICKET_DELETED).
func (s *session) sendTicketDeletedToSubmitter(t *gmTicketRow) {
	if s.server == nil {
		return
	}
	if sess := s.server.playerSessionForGUID(t.playerGuid); sess != nil {
		buf := protocol.NewBuffer(4)
		buf.WriteU32(gmTicketResponseDeleted)
		_ = sess.write(uint16(protocol.OpcodeSMSG_GMTICKET_DELETETICKET), buf.Bytes(), true)
	}
}

// handleTicketAssign mirrors HandleGMTicketAssignToCommand (cs_ticket.cpp:80).
func (s *session) handleTicketAssign(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketAssign) {
		return false
	}
	if len(args) < 2 {
		return false
	}
	// normalizePlayerName fails on invalid UTF-8 (ObjectMgr.cpp:141-156),
	// and the handler returns false silently there (cs_ticket.cpp:98-99).
	if !utf8.ValidString(args[1]) {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket assign <id> <name>")
	if !ok {
		return true
	}
	targetName := normalizePlayerName(args[1])
	var targetGuid uint64
	var targetAccount uint32
	cdb := s.server.CharactersStore.DB
	if cdb != nil {
		_ = cdb.QueryRowContext(ctx, "SELECT guid, account FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", targetName).Scan(&targetGuid, &targetAccount)
	}
	// Target must exist and have administrative rights (cs_ticket.cpp:103).
	// The security level comes from account_access, not account.gmlevel:
	// AccountMgr::GetSecurity (AccountMgr.cpp:285-293) reads
	// LOGIN_GET_GMLEVEL_BY_REALMID, and gmlevel is a legacy column that
	// stays 0 for real GMs.
	canAssign := false
	var sec uint8
	if targetGuid != 0 && s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		sec = s.accountSecurityLevel(ctx, targetAccount)
		if hasPerm, err := accountHasPermission(ctx, s.server.AuthStore.DB, targetAccount, s.server.RealmID, sec, 32); err == nil && hasPerm { // RBAC_PERM_COMMANDS_BE_ASSIGNED_TICKET 32
			canAssign = true
		}
	}
	if !canAssign {
		s.sendSysMessage("Target must exist and have administrative rights.") // LANG_COMMAND_TICKETASSIGNERROR_A 2012
		return true
	}
	if t.assignedTo == targetGuid {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned to that player.", t.id)) // LANG_COMMAND_TICKETASSIGNERROR_B 2013
		return true
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
		return true
	}
	t.assignedTo = targetGuid
	// GmTicket::SetAssignedTo (TicketMgr.h:115-122): assigning to an admin
	// while the ticket sits in the escalation queue moves it to
	// TICKET_ESCALATED_ASSIGNED (3); any assignment of an unassigned ticket
	// marks it TICKET_ASSIGNED (1). IsAdminAccount (AccountMgr.cpp:404) is
	// SEC_ADMINISTRATOR(3) <= gmlevel <= SEC_CONSOLE.
	isAdmin := sec >= 3
	switch {
	case isAdmin && t.escalated == 2: // TICKET_IN_ESCALATION_QUEUE
		t.escalated = 3 // TICKET_ESCALATED_ASSIGNED
	case t.escalated == 0: // TICKET_UNASSIGNED
		t.escalated = 1 // TICKET_ASSIGNED
	}
	s.saveGMTicket(ctx, t)
	s.noteTicketsChanged()
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d assigned to %s.", t.id, targetName))
	return true
}

// handleTicketClose mirrors HandleGMTicketCloseByIdCommand (cs_ticket.cpp:130).
func (s *session) handleTicketClose(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketClose) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket close <id>")
	if !ok || t.completed {
		if ok {
			s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		}
		return true
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("You cannot close ticket %d.", t.id)) // LANG_COMMAND_TICKETCANNOTCLOSE 2016
		return true
	}
	// ResolveAndCloseTicket (TicketMgr.cpp:393-404): SetClosedBy also
	// flips type to TICKET_TYPE_CLOSED, and SetResolvedBy records the
	// closer — without the type write the submitter-facing layer
	// (tickets.go, type = 0 filter) still treats the ticket as open.
	t.closedBy = s.playerGUID
	t.ticketType = 1 // TICKET_TYPE_CLOSED (TicketMgr.h:86)
	t.resolvedBy = s.playerGUID
	s.saveGMTicket(ctx, t)
	s.noteTicketsChanged()
	// cs_ticket.cpp:163-168: the online submitter is told the ticket is
	// gone (SMSG_GMTICKET_DELETETICKET, GMTICKET_RESPONSE_TICKET_DELETED).
	s.sendTicketDeletedToSubmitter(t)
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d closed.", t.id))
	return true
}

// handleTicketClosedList mirrors HandleGMTicketListClosedCommand.
func (s *session) handleTicketClosedList(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketClosedlist) {
		return false
	}
	s.ticketListQuery(ctx, "type <> 0", "Closed tickets:")
	return true
}

// handleTicketComment mirrors HandleGMTicketCommentCommand (cs_ticket.cpp:171).
func (s *session) handleTicketComment(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketComment) {
		return false
	}
	if len(args) < 2 {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket comment <id> <comment>")
	if !ok {
		return true
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
		return true
	}
	t.comment = strings.Join(args[1:], " ")
	s.saveGMTicket(ctx, t)
	s.noteTicketsChanged()
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("%s added comment to ticket %d: %s", s.player.Name, t.id, t.comment)) // LANG_COMMAND_TICKETLISTADDCOMMENT 2024
	return true
}

// handleTicketComplete mirrors HandleGMTicketCompleteCommand (cs_ticket.cpp:210).
func (s *session) handleTicketComplete(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketComplete) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket complete <id> [response]")
	if !ok || t.completed {
		if ok {
			s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		}
		return true
	}
	if len(args) > 1 {
		if s.ticketAssignedNotTo(t) {
			s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
			return true
		}
		t.response += strings.Join(args[1:], " ")
	}
	t.completed = true
	t.resolvedBy = s.playerGUID
	// cs_ticket.cpp:256-258: the GM response goes to the online submitter
	// (GmTicket::SendResponse, TicketMgr.cpp:147) before the ticket is
	// marked completed and saved.
	if s.server != nil {
		if sess := s.server.playerSessionForGUID(t.playerGuid); sess != nil {
			sess.sendGMResponseReceived(t.id, t.description, t.response)
		}
	}
	s.saveGMTicket(ctx, t)
	s.noteTicketsChanged()
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d completed.", t.id))
	return true
}

// handleTicketDelete mirrors HandleGMTicketDeleteByIdCommand (cs_ticket.cpp:274).
func (s *session) handleTicketDelete(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketDelete) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	t, ok := s.loadGMTicket(ctx, uint32(cAtoi(args[0])))
	if !ok {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return true
	}
	// IsClosed() is type-based (TicketMgr.h:97): a character-deleted
	// ticket (type 2) is closed even though closedBy is 0.
	if t.ticketType == 0 {
		s.sendSysMessage("Close the ticket first.") // LANG_COMMAND_TICKETCLOSEFIRST 2006
		return true
	}
	// cs_ticket.cpp:296-301: the online submitter is told the ticket is
	// gone (SMSG_GMTICKET_DELETETICKET, GMTICKET_RESPONSE_TICKET_DELETED).
	s.sendTicketDeletedToSubmitter(t)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM gm_ticket WHERE id = ?", t.id)
	}
	s.noteTicketsChanged()
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d deleted.", t.id))
	return true
}

// handleTicketEscalate mirrors HandleGMTicketEscalateCommand (cs_ticket.cpp:308).
func (s *session) handleTicketEscalate(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketEscalate) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket escalate <id>")
	if !ok || t.completed || t.escalated != 0 { // != TICKET_UNASSIGNED (TicketMgr.h:60)
		if ok {
			s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		}
		return true
	}
	t.escalated = 2 // TICKET_IN_ESCALATION_QUEUE (TicketMgr.h:62)
	s.saveGMTicket(ctx, t)
	// cs_ticket.cpp:322-323: the refreshed ticket state goes to the online
	// submitter (TicketMgr::SendTicket) before the last-change clock
	// advances, so the packet still carries the pre-escalation age.
	if s.server != nil {
		if sess := s.server.playerSessionForGUID(t.playerGuid); sess != nil {
			sess.sendGMTicketState(ctx, t)
		}
	}
	s.noteTicketsChanged()
	// C++ sends no message to the invoking GM here, so this arm stays
	// silent on the GM side, == C++.
	return true
}

// handleTicketEscalatedList mirrors HandleGMTicketListEscalatedCommand.
func (s *session) handleTicketEscalatedList(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketEscalatedlist) {
		return false
	}
	s.ticketListQuery(ctx, "escalated = 2 AND type = 0", "Escalated tickets:") // TICKET_IN_ESCALATION_QUEUE (TicketMgr.h:62)
	return true
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
func (s *session) handleTicketList(ctx context.Context, onlineOnly bool) bool {
	perm := permissionCommandTicketList
	if onlineOnly {
		perm = permissionCommandTicketOnlinelist
	}
	if !s.commandAllowed(ctx, perm) {
		return false
	}
	if !onlineOnly {
		// TicketMgr::ShowList (TicketMgr.cpp:421): !IsClosed() && !IsCompleted().
		s.ticketListQuery(ctx, "type = 0 AND completed = 0", "Open tickets:")
		return true
	}
	// Restrict to tickets whose submitter is online.
	s.server.sessionsMu.RLock()
	var guids []uint64
	for sess := range s.server.sessions {
		if sess.player != nil && sess.worldReady.Load() {
			guids = append(guids, sess.playerGUID)
		}
	}
	s.server.sessionsMu.RUnlock()
	s.ticketListQueryOnline(ctx, guids, "Tickets from online players:")
	return true
}

// ticketListQueryOnline lists open tickets for the given player guids.
func (s *session) ticketListQueryOnline(ctx context.Context, guids []uint64, header string) {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return
	}
	s.sendSysMessage(header)
	// ShowList (TicketMgr.cpp:421-428) walks the id-ordered ticket map;
	// the online sessions iterate in map order, so collect the ids and
	// print them id-ascending like C++.
	var ids []uint32
	for _, g := range guids {
		var id uint32
		if err := cdb.QueryRowContext(ctx, "SELECT id FROM gm_ticket WHERE playerGuid = ? AND type = 0 AND completed = 0 LIMIT 1", g).Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	empty := true
	for _, id := range ids {
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
func (s *session) handleTicketReset(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketReset) {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	var openCount int64
	// GetOpenTicketCount counts !IsClosed() (TicketMgr.h:226): type-based,
	// so character-deleted tickets (type 2) do not block the reset.
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM gm_ticket WHERE type = 0").Scan(&openCount)
	if openCount > 0 {
		s.sendSysMessage("There are pending tickets.") // LANG_COMMAND_TICKETPENDING 2027
		return true
	}
	_, _ = cdb.ExecContext(ctx, "DELETE FROM gm_ticket")
	s.sendSysMessage("Tickets reset.") // LANG_COMMAND_TICKETRESET 2028
	return true
}

// handleTicketToggleSystem mirrors HandleToggleGMTicketSystem (cs_ticket.cpp:358).
func (s *session) handleTicketToggleSystem(ctx context.Context) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketTogglesystem) {
		return false
	}
	enabled := !s.server.ticketsEnabled.Load()
	s.server.ticketsEnabled.Store(enabled)
	if enabled {
		s.sendSysMessage("Ticket system enabled.") // LANG_ALLOW_TICKETS 1134
	} else {
		s.sendSysMessage("Ticket system disabled.") // LANG_DISALLOW_TICKETS 1135
	}
	return true
}

// handleTicketUnassign mirrors HandleGMTicketUnAssignCommand (cs_ticket.cpp:366).
func (s *session) handleTicketUnassign(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketUnassign) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, args, "Syntax: .ticket unassign <id>")
	if !ok {
		return true
	}
	if t.assignedTo == 0 {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is not assigned.", t.id)) // LANG_COMMAND_TICKETNOTASSIGNED 2014
		return true
	}
	// The invoker's security must cover the assignee's (cs_ticket.cpp:390).
	var assigneeSec uint8
	if sess := s.server.playerSessionForGUID(t.assignedTo); sess != nil {
		assigneeSec = sess.security
	} else if s.server.AuthStore != nil && s.server.AuthStore.DB != nil {
		var accountID uint32
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", t.assignedTo).Scan(&accountID)
		// AccountMgr::GetSecurity (AccountMgr.cpp:285-293) reads
		// account_access, not the legacy account.gmlevel column.
		assigneeSec = s.accountSecurityLevel(ctx, accountID)
	}
	if assigneeSec > s.security {
		s.sendSysMessage("You cannot unassign a ticket assigned to a higher security level.") // LANG_COMMAND_TICKETUNASSIGNSECURITY 2015
		return true
	}
	assignedName := s.ticketAssignedName(ctx, t.assignedTo)
	t.assignedTo = 0
	// GmTicket::SetUnassigned: ASSIGNED -> UNASSIGNED,
	// ESCALATED_ASSIGNED -> IN_ESCALATION_QUEUE; clearing assignedTo
	// alone strands an escalated ticket (escalated = 3 matches no list
	// filter and no re-assign transition).
	switch t.escalated {
	case 1: // TICKET_ASSIGNED
		t.escalated = 0 // TICKET_UNASSIGNED
	case 3: // TICKET_ESCALATED_ASSIGNED
		t.escalated = 2 // TICKET_IN_ESCALATION_QUEUE
	}
	s.saveGMTicket(ctx, t)
	s.noteTicketsChanged()
	s.server.sendGlobalGMMessage(ctx, fmt.Sprintf("Ticket %d unassigned from %s.", t.id, assignedName))
	return true
}

// handleTicketViewID mirrors HandleGMTicketGetByIdCommand (cs_ticket.cpp:419).
func (s *session) handleTicketViewID(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketViewid) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	t, ok := s.loadGMTicket(ctx, uint32(cAtoi(args[0])))
	// IsClosed() is type-based (TicketMgr.h:97).
	if !ok || t.ticketType != 0 || t.completed {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return true
	}
	t.viewed = true
	s.saveGMTicket(ctx, t)
	s.sendSysMessage(s.ticketFormat(ctx, t))
	s.ticketShowDetail(t)
	return true
}

// ticketShowDetail prints the comment/response legs of the C++ detailed
// view (FormatMessageString), which the one-line summary omits.
func (s *session) ticketShowDetail(t *gmTicketRow) {
	if t.comment != "" {
		s.sendSysMessage(fmt.Sprintf("Comment: %s", t.comment))
	}
	if t.response != "" {
		s.sendSysMessage(fmt.Sprintf("Response: %s", t.response))
	}
}

// handleTicketViewName mirrors HandleGMTicketGetByNameCommand (cs_ticket.cpp:447).
func (s *session) handleTicketViewName(ctx context.Context, args []string) bool {
	if !s.commandAllowed(ctx, permissionCommandTicketViewname) {
		return false
	}
	if len(args) == 0 {
		return false
	}
	// normalizePlayerName fails on invalid UTF-8 (ObjectMgr.cpp:141-156),
	// and the handler returns false silently there (cs_ticket.cpp:452-453).
	if !utf8.ValidString(args[0]) {
		return false
	}
	name := normalizePlayerName(args[0])
	var guid uint64
	if sess := s.sessionForPlayerName(name); sess != nil && sess.player != nil {
		guid = sess.playerGUID
	} else if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT guid FROM characters WHERE UPPER(name) = UPPER(?) LIMIT 1", name).Scan(&guid)
	}
	if guid == 0 {
		s.sendSysMessage("No players found.") // LANG_NO_PLAYERS_FOUND 330 (cs_ticket.cpp:462)
		return true
	}
	var id uint32
	// GetTicketByPlayer (TicketMgr.h:196-203) takes the lowest-id
	// !IsClosed() ticket: type-based, not closedBy-based.
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil ||
		s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT id FROM gm_ticket WHERE playerGuid = ? AND type = 0 ORDER BY id ASC LIMIT 1", guid).Scan(&id) != nil {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return true
	}
	t, ok := s.loadGMTicket(ctx, id)
	if !ok {
		s.sendSysMessage("Ticket does not exist.") // LANG_COMMAND_TICKETNOTEXIST 2005
		return true
	}
	t.viewed = true
	s.saveGMTicket(ctx, t)
	s.sendSysMessage(s.ticketFormat(ctx, t))
	s.ticketShowDetail(t)
	return true
}

// handleTicketResponse dispatches the "ticket response" sub-table
// (cs_ticket.cpp:52-53): append, appendln.
func (s *session) handleTicketResponse(ctx context.Context, args []string) bool {
	if len(args) == 0 {
		// Dead response-node perm (see header): bare ".ticket response"
		// prints the syntax listing with no gate, == C++.
		s.sendSysMessage("Syntax: .ticket response append|appendln <id> <text>")
		return true
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	newLine := false
	// C++ prefix matching (ChatCommand.cpp:268-285): exact match wins, a
	// proper prefix of "append" is also a prefix of "appendln" and is
	// ambiguous (LANG_CMD_AMBIGUOUS 194; the syntax line is the tree's
	// stand-in), longer "appendl*" prefixes resolve to appendln.
	switch {
	case sub == "append":
		if !s.commandAllowed(ctx, permissionCommandTicketResponseAppend) {
			return false
		}
	case sub == "appendln":
		newLine = true
		if !s.commandAllowed(ctx, permissionCommandTicketResponseAppendln) {
			return false
		}
	case strings.HasPrefix("append", sub):
		s.sendSysMessage("Syntax: .ticket response append|appendln <id> <text>")
		return true
	case strings.HasPrefix("appendln", sub):
		newLine = true
		if !s.commandAllowed(ctx, permissionCommandTicketResponseAppendln) {
			return false
		}
	default:
		s.sendSysMessage("Syntax: .ticket response append|appendln <id> <text>")
		return true
	}
	// Mirrors _HandleGMTicketResponseAppendCommand (cs_ticket.cpp:474).
	if len(rest) < 2 {
		return false
	}
	t, ok := s.ticketOpenByID(ctx, rest, "Syntax: .ticket response append|appendln <id> <text>")
	if !ok {
		return true
	}
	if s.ticketAssignedNotTo(t) {
		s.sendSysMessage(fmt.Sprintf("Ticket %d is already assigned.", t.id)) // LANG_COMMAND_TICKETALREADYASSIGNED 2007
		return true
	}
	t.response += strings.Join(rest[1:], " ")
	if newLine {
		t.response += "\n"
	}
	s.saveGMTicket(ctx, t)
	return true
}
