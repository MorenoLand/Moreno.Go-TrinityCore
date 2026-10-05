package world

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
)

// rbac command port (cs_rbac.cpp): 5 leaf arms — the "account" sub-table
// (list/grant/deny/revoke, RBAC 202-205) plus "list" (206). The bare roots'
// own perms (200 on "rbac", 201 on "rbac account") are DEAD in C++: both are
// 6-arg nullptr+subtable entries (ChatCommand.h deprecated overload drops the
// RBACPermissions param, delegating to the sub-only constructor), so bare
// ".rbac" / ".rbac account" print the help listing ungated — the tree's
// syntax-line stand-in, same pattern as the mmap/modify/npc/quest/pet ports.
// permissionCommandRBAC stays in permissions.go as documentation (unused const
// is legal). Trinity only checks the invoker leaf node (ChatCommand.cpp:487),
// so each arm gates exactly its own permission. All C++ table entries are
// console=true, which is moot here (Go commands are always sessioned), so the
// arms run in-game with the same RBAC gates.
//
// The Go tree has no RBACData object (RBAC.cpp): the granted/denied/default
// permission model is reimplemented at the auth DB level, which is where the
// C++ persists it anyway (rbac_account_permissions, rbac_default_permissions,
// rbac_permissions, rbac_linked_permissions). Grant/Deny/Revoke mirror
// RBACData::GrantPermission/DenyPermission/RevokePermission (RBAC.cpp:43-172)
// verbatim: unknown permission id -> WRONG_PARAMETER_ID, already-denied on
// grant -> IN_DENIED_LIST, already-granted on grant -> CANT_ADD_ALREADY_ADDED
// (and the deny mirrors), revoke of an unlisted id -> NOT_IN_LIST; the DB
// writes are the LOGIN_INS/DEL_RBAC_ACCOUNT_PERMISSION statements verbatim
// (INSERT ... ON DUPLICATE KEY UPDATE granted for MySQL, INSERT OR REPLACE
// for SQLite per tree convention).
//
// Deliberate delta: HandleRBACPermRevokeCommand dereferences
// GetRBACPermission(id) unconditionally, so a nonexistent id is a nullptr
// deref in C++; the Go port reports WRONG_PARAMETER_ID instead (matching the
// grant/deny arms' own existence check).

// Inlined enUS texts (Language.h ids; no in-tree trinity_string seed), TDB
// enUS recall per tree convention.
const (
	rbacWrongParamID      = "Wrong parameter id: %d."                                                                                 // LANG_RBAC_WRONG_PARAMETER_ID 63
	rbacWrongParamRealm   = "Wrong parameter realm: %d."                                                                              // LANG_RBAC_WRONG_PARAMETER_REALM 64
	rbacListHeaderGranted = "List of granted permissions of account %d (%s):"                                                         // LANG_RBAC_LIST_HEADER_GRANTED 65
	rbacListHeaderDenied  = "List of denied permissions of account %d (%s):"                                                          // LANG_RBAC_LIST_HEADER_DENIED 66
	rbacListHeaderBySec   = "List of default permissions of account %d (%s) by security level %d:"                                    // LANG_RBAC_LIST_HEADER_BY_SEC_LEVEL 67
	rbacListPermsHeader   = "List of permissions:"                                                                                    // LANG_RBAC_LIST_PERMISSIONS_HEADER 68
	rbacListLinkedHeader  = "List of linked permissions:"                                                                             // LANG_RBAC_LIST_PERMS_LINKED_HEADER 69
	rbacListEmpty         = "Empty"                                                                                                   // LANG_RBAC_LIST_EMPTY 70
	rbacListElement       = "%d - %s"                                                                                                 // LANG_RBAC_LIST_ELEMENT 71
	rbacGrantedInList     = "Permission %d (%s) granted in realm %d for account %d (%s). This permission is already granted in list." // LANG_RBAC_PERM_GRANTED_IN_LIST 72
	rbacGrantedInDenied   = "Permission %d (%s) granted in realm %d for account %d (%s). This permission is already denied in list."  // LANG_RBAC_PERM_GRANTED_IN_DENIED_LIST 73
	rbacPermGranted       = "Permission %d (%s) granted in realm %d for account %d (%s)."                                             // LANG_RBAC_PERM_GRANTED 74
	rbacDeniedInList      = "Permission %d (%s) denied in realm %d for account %d (%s). This permission is already denied in list."   // LANG_RBAC_PERM_DENIED_IN_LIST 75
	rbacDeniedInGranted   = "Permission %d (%s) denied in realm %d for account %d (%s). This permission is already granted in list."  // LANG_RBAC_PERM_DENIED_IN_GRANTED_LIST 76
	rbacPermDenied        = "Permission %d (%s) denied in realm %d for account %d (%s)."                                              // LANG_RBAC_PERM_DENIED 77
	rbacPermRevoked       = "Permission %d (%s) revoked in realm %d for account %d (%s)."                                             // LANG_RBAC_PERM_REVOKED 78
	rbacRevokedNotInList  = "Permission %d (%s) revoked in realm %d for account %d (%s). This permission is not in list."             // LANG_RBAC_PERM_REVOKED_NOT_IN_LIST 79
	rbacAccountNotExist   = "Account %s does not exist."                                                                              // LANG_ACCOUNT_NOT_EXIST 413
)

// rbacCommandParams mirrors RBACCommandData (cs_rbac.cpp:44-56): the parsed
// permission id, realm, and the target account.
type rbacCommandParams struct {
	id          uint32
	realmID     int32
	accountID   uint32
	accountName string
}

// readRBACParams mirrors ReadParams (cs_rbac.cpp:97-166). With checkParams
// the id/realm legs parse exactly like the C++ strtok walk (two tokens means
// id-from-param1 + selected player; three tokens means param1 is the account
// name). checkParams=false (the list arm) keeps the C++ defaults (id 0,
// realm -1) and only resolves the account. A false return means the C++
// returned nullptr (error message already sent where the C++ sends one).
func (s *session) readRBACParams(ctx context.Context, tokens []string, checkParams bool) (*rbacCommandParams, bool) {
	var p1, p2, p3 string
	if len(tokens) > 0 {
		p1 = tokens[0]
	}
	if len(tokens) > 1 {
		p2 = tokens[1]
	}
	if len(tokens) > 2 {
		p3 = tokens[2]
	}

	p := &rbacCommandParams{realmID: -1}
	useSelected := false
	if checkParams {
		if p3 == "" {
			// C++: if (param2) realmId = atoi(param2) — a missing second
			// token leaves realmId at -1 (all realms), it does not parse
			// to 0 (cs_rbac.cpp:110-116).
			if p2 != "" {
				p.realmID = int32(cAtoi(p2))
			}
			p.id = uint32(cAtoi(p1))
			useSelected = true
		} else {
			p.id = uint32(cAtoi(p2))
			p.realmID = int32(cAtoi(p3))
		}
		if p.id == 0 {
			s.sendSysMessage(fmt.Sprintf(rbacWrongParamID, p.id))
			return nil, false
		}
		if p.realmID < -1 || p.realmID == 0 {
			s.sendSysMessage(fmt.Sprintf(rbacWrongParamRealm, p.realmID))
			return nil, false
		}
	} else if p1 == "" {
		useSelected = true
	}

	if useSelected {
		// getSelectedPlayer (Chat.cpp:310): no selection resolves to the
		// invoker; only an unresolvable selection yields null (silent, the
		// handler reports the error).
		target := s
		if s.selection != 0 && s.server != nil {
			ts := s.server.playerSessionForGUID(s.selection)
			if ts == nil || ts.player == nil {
				return nil, false
			}
			target = ts
		}
		if target.player == nil {
			return nil, false
		}
		p.accountID = target.accountID
		p.accountName = s.accountNameByID(ctx, target.accountID)
	} else {
		p.accountName = upperOnlyLatin(p1)
		p.accountID = s.accountIDByName(ctx, p.accountName)
		if p.accountID == 0 {
			s.sendSysMessage(fmt.Sprintf(rbacAccountNotExist, p.accountName))
			return nil, false
		}
	}

	if checkParams && !s.canModifyAccount(ctx, p.accountID) {
		return nil, false
	}
	return p, true
}

// loadRBACAccountLists mirrors RBACData::LoadFromDB + LoadFromDBCallback
// (RBAC.cpp:174-221): the realm-scoped granted/denied rows from
// rbac_account_permissions, with the account's security-level default
// permissions folded into the granted set (denied rows win, as in
// GrantPermission's ordered checks). LOGIN_SEL_RBAC_ACCOUNT_PERMISSIONS and
// the rbac_default_permissions load from AccountMgr.cpp:474 verbatim.
func (s *session) loadRBACAccountLists(ctx context.Context, accountID uint32) (granted, denied map[uint32]struct{}, ok bool) {
	granted = make(map[uint32]struct{})
	denied = make(map[uint32]struct{})
	db := s.authDB()
	if db == nil || s.server == nil {
		return granted, denied, false
	}
	rows, err := db.QueryContext(ctx,
		"SELECT permissionId, granted FROM rbac_account_permissions WHERE accountId = ? AND (realmId = ? OR realmId = -1) ORDER BY permissionId, realmId",
		accountID, int32(s.server.RealmID))
	if err != nil {
		return granted, denied, false
	}
	for rows.Next() {
		var id uint32
		var allowed int64
		if err := rows.Scan(&id, &allowed); err != nil {
			_ = rows.Close()
			return granted, denied, false
		}
		if allowed != 0 {
			granted[id] = struct{}{}
		} else {
			denied[id] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return granted, denied, false
	}
	_ = rows.Close()

	secLevel := s.accountSecurityLevel(ctx, accountID)
	drows, err := db.QueryContext(ctx,
		"SELECT permissionId FROM rbac_default_permissions WHERE secId = ? AND (realmId = ? OR realmId = -1) ORDER BY permissionId",
		secLevel, int32(s.server.RealmID))
	if err != nil {
		return granted, denied, false
	}
	for drows.Next() {
		var id uint32
		if err := drows.Scan(&id); err != nil {
			_ = drows.Close()
			return granted, denied, false
		}
		if _, blocked := denied[id]; !blocked {
			granted[id] = struct{}{}
		}
	}
	if err := drows.Err(); err != nil {
		_ = drows.Close()
		return granted, denied, false
	}
	_ = drows.Close()
	return granted, denied, true
}

// rbacPermissionName mirrors sAccountMgr->GetRBACPermission: the name for a
// permission id, or false when the id does not exist
// (RBAC_ID_DOES_NOT_EXISTS).
func (s *session) rbacPermissionName(ctx context.Context, id uint32) (string, bool) {
	db := s.authDB()
	if db == nil {
		return "", false
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM rbac_permissions WHERE id = ?", id).Scan(&name); err != nil {
		return "", false
	}
	return name, true
}

// sortedRBACIDs renders the RBACPermissionContainer order (std::set<uint32>
// is ascending): LANG_RBAC_LIST_ELEMENT lines, or "Empty".
func (s *session) sortedRBACIDs(ctx context.Context, ids map[uint32]struct{}) {
	if len(ids) == 0 {
		s.sendSysMessage(rbacListEmpty)
		return
	}
	sorted := make([]uint32, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, id := range sorted {
		name, ok := s.rbacPermissionName(ctx, id)
		if !ok {
			continue
		}
		s.sendSysMessage(fmt.Sprintf(rbacListElement, id, name))
	}
}

// saveRBACPermission mirrors RBACData::SavePermission
// (RBAC.cpp:131-139): LOGIN_INS_RBAC_ACCOUNT_PERMISSION verbatim.
func (s *session) saveRBACPermission(ctx context.Context, p *rbacCommandParams, granted bool) {
	db := s.authDB()
	if db == nil {
		return
	}
	var g int64
	if granted {
		g = 1
	}
	if s.server.AuthStore.Backend == database.BackendSQLite {
		_, _ = db.ExecContext(ctx, "INSERT OR REPLACE INTO rbac_account_permissions (accountId, permissionId, granted, realmId) VALUES (?, ?, ?, ?)",
			p.accountID, p.id, g, p.realmID)
		return
	}
	_, _ = db.ExecContext(ctx, "INSERT INTO rbac_account_permissions (accountId, permissionId, granted, realmId) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE granted = VALUES(granted)",
		p.accountID, p.id, g, p.realmID)
}

// deleteRBACPermission mirrors the RevokePermission delete leg
// (RBAC.cpp:159-163): LOGIN_DEL_RBAC_ACCOUNT_PERMISSION verbatim.
func (s *session) deleteRBACPermission(ctx context.Context, p *rbacCommandParams) {
	db := s.authDB()
	if db == nil {
		return
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM rbac_account_permissions WHERE accountId = ? AND permissionId = ? AND (realmId = ? OR realmId = -1)",
		p.accountID, p.id, p.realmID)
}

// handleCmdRBAC dispatches the "rbac" root (cs_rbac.cpp:59-85) with Trinity
// per-level prefix matching. A bare ".rbac" prints the syntax line with no
// permission gate: the root's own 200 is dead in C++ (ChatCommand.h
// deprecated 6-arg overload). "rbac account" is a three-level path,
// dispatched two-level like the channel "set ownership" arm.
func (s *session) handleCmdRBAC(ctx context.Context, args []string) {
	const syntax = "Syntax: .rbac account list|grant|deny|revoke <args> | .rbac list [id]"
	if len(args) == 0 {
		s.sendSysMessage(syntax)
		return
	}
	if s.player == nil {
		s.sendSysMessage("You must be in game to use that command.")
		return
	}
	switch sub := strings.ToLower(args[0]); sub {
	case "account":
		if len(args) < 2 {
			s.sendSysMessage(syntax)
			return
		}
		switch lvl := strings.ToLower(args[1]); {
		case strings.HasPrefix("list", lvl):
			s.handleRBACAccountListCommand(ctx, args[2:])
		case strings.HasPrefix("grant", lvl):
			s.handleRBACAccountGrantCommand(ctx, args[2:])
		case strings.HasPrefix("deny", lvl):
			s.handleRBACAccountDenyCommand(ctx, args[2:])
		case strings.HasPrefix("revoke", lvl):
			s.handleRBACAccountRevokeCommand(ctx, args[2:])
		default:
			s.sendSysMessage(syntax)
		}
	case "list":
		if s.miscDeny(ctx, permissionCommandRBACList) {
			return
		}
		s.handleRBACListPermissionsCommand(ctx, args[1:])
	default:
		s.sendSysMessage(syntax)
	}
}

// handleRBACAccountGrantCommand mirrors HandleRBACPermGrantCommand
// (cs_rbac.cpp:169-205, RBAC 203): the RBACData::GrantPermission result
// ladder with the LANG_RBAC_PERM_GRANTED* messages.
func (s *session) handleRBACAccountGrantCommand(ctx context.Context, tokens []string) {
	if s.miscDeny(ctx, permissionCommandRBACAccPermGrant) {
		return
	}
	p, ok := s.readRBACParams(ctx, tokens, true)
	if !ok {
		return
	}
	name, ok := s.rbacPermissionName(ctx, p.id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf(rbacWrongParamID, p.id))
		return
	}
	granted, denied, ok := s.loadRBACAccountLists(ctx, p.accountID)
	if !ok {
		return
	}
	who := []any{p.id, name, p.realmID, p.accountID, p.accountName}
	switch {
	case containsID(denied, p.id):
		s.sendSysMessage(fmt.Sprintf(rbacGrantedInDenied, who...))
	case containsID(granted, p.id):
		s.sendSysMessage(fmt.Sprintf(rbacGrantedInList, who...))
	default:
		s.saveRBACPermission(ctx, p, true)
		s.sendSysMessage(fmt.Sprintf(rbacPermGranted, who...))
	}
}

// handleRBACAccountDenyCommand mirrors HandleRBACPermDenyCommand
// (cs_rbac.cpp:207-243, RBAC 204).
func (s *session) handleRBACAccountDenyCommand(ctx context.Context, tokens []string) {
	if s.miscDeny(ctx, permissionCommandRBACAccPermDeny) {
		return
	}
	p, ok := s.readRBACParams(ctx, tokens, true)
	if !ok {
		return
	}
	name, ok := s.rbacPermissionName(ctx, p.id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf(rbacWrongParamID, p.id))
		return
	}
	granted, denied, ok := s.loadRBACAccountLists(ctx, p.accountID)
	if !ok {
		return
	}
	who := []any{p.id, name, p.realmID, p.accountID, p.accountName}
	switch {
	case containsID(granted, p.id):
		s.sendSysMessage(fmt.Sprintf(rbacDeniedInGranted, who...))
	case containsID(denied, p.id):
		s.sendSysMessage(fmt.Sprintf(rbacDeniedInList, who...))
	default:
		s.saveRBACPermission(ctx, p, false)
		s.sendSysMessage(fmt.Sprintf(rbacPermDenied, who...))
	}
}

// handleRBACAccountRevokeCommand mirrors HandleRBACPermRevokeCommand
// (cs_rbac.cpp:245-274, RBAC 205).
func (s *session) handleRBACAccountRevokeCommand(ctx context.Context, tokens []string) {
	if s.miscDeny(ctx, permissionCommandRBACAccPermRevoke) {
		return
	}
	p, ok := s.readRBACParams(ctx, tokens, true)
	if !ok {
		return
	}
	name, ok := s.rbacPermissionName(ctx, p.id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf(rbacWrongParamID, p.id))
		return
	}
	granted, denied, ok := s.loadRBACAccountLists(ctx, p.accountID)
	if !ok {
		return
	}
	who := []any{p.id, name, p.realmID, p.accountID, p.accountName}
	if !containsID(granted, p.id) && !containsID(denied, p.id) {
		s.sendSysMessage(fmt.Sprintf(rbacRevokedNotInList, who...))
		return
	}
	s.deleteRBACPermission(ctx, p)
	s.sendSysMessage(fmt.Sprintf(rbacPermRevoked, who...))
}

// handleRBACAccountListCommand mirrors HandleRBACPermListCommand
// (cs_rbac.cpp:276-316, RBAC 202): the granted/denied/default-by-security
// sections with their LANG_RBAC_LIST_HEADER_* headers.
func (s *session) handleRBACAccountListCommand(ctx context.Context, tokens []string) {
	if s.miscDeny(ctx, permissionCommandRBACAccPermList) {
		return
	}
	p, ok := s.readRBACParams(ctx, tokens, false)
	if !ok {
		return
	}
	granted, denied, ok := s.loadRBACAccountLists(ctx, p.accountID)
	if !ok {
		return
	}
	s.sendSysMessage(fmt.Sprintf(rbacListHeaderGranted, p.accountID, p.accountName))
	s.sortedRBACIDs(ctx, granted)
	s.sendSysMessage(fmt.Sprintf(rbacListHeaderDenied, p.accountID, p.accountName))
	s.sortedRBACIDs(ctx, denied)
	secLevel := s.accountSecurityLevel(ctx, p.accountID)
	s.sendSysMessage(fmt.Sprintf(rbacListHeaderBySec, p.accountID, p.accountName, secLevel))
	s.sortedRBACIDs(ctx, s.rbacDefaultPermissions(ctx, secLevel))
}

// rbacDefaultPermissions mirrors
// sAccountMgr->GetRBACDefaultPermissions(secLevel) (AccountMgr.cpp:474):
// the realm-scoped default rows for the security level.
func (s *session) rbacDefaultPermissions(ctx context.Context, secLevel uint8) map[uint32]struct{} {
	ids := make(map[uint32]struct{})
	db := s.authDB()
	if db == nil || s.server == nil {
		return ids
	}
	rows, err := db.QueryContext(ctx,
		"SELECT permissionId FROM rbac_default_permissions WHERE secId = ? AND (realmId = ? OR realmId = -1) ORDER BY permissionId",
		secLevel, int32(s.server.RealmID))
	if err != nil {
		return ids
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			return ids
		}
		ids[id] = struct{}{}
	}
	return ids
}

// handleRBACListPermissionsCommand mirrors HandleRBACListPermissionsCommand
// (cs_rbac.cpp:318-355, RBAC 206): the full permission catalog, or a single
// permission with its linked permissions (rbac_linked_permissions).
func (s *session) handleRBACListPermissionsCommand(ctx context.Context, tokens []string) {
	db := s.authDB()
	if db == nil {
		return
	}
	var id uint32
	if len(tokens) > 0 {
		id = uint32(cAtoi(tokens[0]))
	}
	if id == 0 {
		s.sendSysMessage(rbacListPermsHeader)
		rows, err := db.QueryContext(ctx, "SELECT id, name FROM rbac_permissions ORDER BY id")
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var pid uint32
			var name string
			if err := rows.Scan(&pid, &name); err != nil {
				return
			}
			s.sendSysMessage(fmt.Sprintf(rbacListElement, pid, name))
		}
		return
	}
	name, ok := s.rbacPermissionName(ctx, id)
	if !ok {
		s.sendSysMessage(fmt.Sprintf(rbacWrongParamID, id))
		return
	}
	s.sendSysMessage(rbacListPermsHeader)
	s.sendSysMessage(fmt.Sprintf(rbacListElement, id, name))
	s.sendSysMessage(rbacListLinkedHeader)
	rows, err := db.QueryContext(ctx, "SELECT linkedId FROM rbac_linked_permissions WHERE id = ? ORDER BY linkedId", id)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var lid uint32
		if err := rows.Scan(&lid); err != nil {
			return
		}
		lname, ok := s.rbacPermissionName(ctx, lid)
		if !ok {
			continue
		}
		s.sendSysMessage(fmt.Sprintf(rbacListElement, lid, lname))
	}
}

// containsID reports set membership for the uint32 permission sets.
func containsID(set map[uint32]struct{}, id uint32) bool {
	_, ok := set[id]
	return ok
}
