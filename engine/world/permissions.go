package world

import (
	"context"
	"database/sql"
)

const permissionCommandGMChat uint32 = 372

const permissionInstantLogout uint32 = 1

// permissionSkipCheckOverSpeedPing mirrors rbac::RBAC_PERM_SKIP_CHECK_OVERSPEED_PING (RBAC.h).
const permissionSkipCheckOverSpeedPing uint32 = 23

// permissionSkipCheckChatChannelReq mirrors rbac::RBAC_PERM_SKIP_CHECK_CHAT_CHANNEL_REQ (RBAC.h:72).
const permissionSkipCheckChatChannelReq uint32 = 19

const permissionTwoSideInteractionChat uint32 = 25

// permissionTwoSideInteractionChannel mirrors rbac::RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL (RBAC.h:79).
const permissionTwoSideInteractionChannel uint32 = 26

const permissionTwoSideWhoList uint32 = 28

const permissionWhoSeeAllSecurityLevels uint32 = 35

// permissionOpcodeWorldTeleport mirrors rbac::RBAC_PERM_OPCODE_WORLD_TELEPORT (RBAC.h:95).
const permissionOpcodeWorldTeleport uint32 = 42

// permissionOpcodeWhois mirrors rbac::RBAC_PERM_OPCODE_WHOIS (RBAC.h:96).
const permissionOpcodeWhois uint32 = 43

func accountHasPermission(ctx context.Context, db *sql.DB, accountID, realmID uint32, security uint8, permissionID uint32) (bool, error) {
	granted := make(map[uint32]struct{})
	denied := make(map[uint32]struct{})
	rows, err := db.QueryContext(ctx, "SELECT permissionId, granted FROM rbac_account_permissions WHERE accountId = ? AND (realmId = ? OR realmId = -1) ORDER BY permissionId, realmId", accountID, realmID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var id uint32
		var allowed int64
		if err := rows.Scan(&id, &allowed); err != nil {
			_ = rows.Close()
			return false, err
		}
		if allowed != 0 {
			granted[id] = struct{}{}
		} else {
			denied[id] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	rows, err = db.QueryContext(ctx, "SELECT permissionId FROM rbac_default_permissions WHERE secId = ? AND (realmId = ? OR realmId = -1) ORDER BY permissionId", security, realmID)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return false, err
		}
		if _, blocked := denied[id]; !blocked {
			granted[id] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	links, err := loadPermissionLinks(ctx, db)
	if err != nil {
		return false, err
	}
	granted = expandPermissions(granted, links)
	denied = expandPermissions(denied, links)
	_, ok := granted[permissionID]
	if !ok {
		return false, nil
	}
	_, blocked := denied[permissionID]
	return !blocked, nil
}

func loadPermissionLinks(ctx context.Context, db *sql.DB) (map[uint32][]uint32, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, linkedId FROM rbac_linked_permissions ORDER BY id, linkedId")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := make(map[uint32][]uint32)
	for rows.Next() {
		var id, linked uint32
		if err := rows.Scan(&id, &linked); err != nil {
			return nil, err
		}
		if id == linked {
			continue
		}
		links[id] = append(links[id], linked)
	}
	return links, rows.Err()
}

func expandPermissions(seed map[uint32]struct{}, links map[uint32][]uint32) map[uint32]struct{} {
	result := make(map[uint32]struct{}, len(seed))
	queue := make([]uint32, 0, len(seed))
	for id := range seed {
		queue = append(queue, id)
	}
	for len(queue) != 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := result[id]; seen {
			continue
		}
		result[id] = struct{}{}
		queue = append(queue, links[id]...)
	}
	return result
}
