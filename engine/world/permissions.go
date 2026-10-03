package world

import (
	"context"
	"database/sql"
)

const permissionCommandGMChat uint32 = 372

const permissionInstantLogout uint32 = 1

// permissionSkipQueue mirrors rbac::RBAC_PERM_SKIP_QUEUE (RBAC.h:55).
const permissionSkipQueue uint32 = 2

const permissionSkipCheckDisableMap uint32 = 20

// permissionSkipCheckOverSpeedPing mirrors rbac::RBAC_PERM_SKIP_CHECK_OVERSPEED_PING (RBAC.h).
const permissionSkipCheckOverSpeedPing uint32 = 23

// permissionSkipCheckChatChannelReq mirrors rbac::RBAC_PERM_SKIP_CHECK_CHAT_CHANNEL_REQ (RBAC.h:72).
const permissionSkipCheckChatChannelReq uint32 = 19

const permissionTwoSideInteractionChat uint32 = 25

// permissionTwoSideInteractionChannel mirrors rbac::RBAC_PERM_TWO_SIDE_INTERACTION_CHANNEL (RBAC.h:79).
const permissionTwoSideInteractionChannel uint32 = 26

const permissionTwoSideWhoList uint32 = 28

const permissionWhoSeeAllSecurityLevels uint32 = 35

// permissionAllowTwoSideTrade mirrors rbac::RBAC_PERM_ALLOW_TWO_SIDE_TRADE (RBAC.h:104).
const permissionAllowTwoSideTrade uint32 = 51

const permissionRestoreSavedGMState uint32 = 39

// permissionOpcodeWorldTeleport mirrors rbac::RBAC_PERM_OPCODE_WORLD_TELEPORT (RBAC.h:95).
const permissionOpcodeWorldTeleport uint32 = 42

// permissionCommandCheatCasttime mirrors rbac::RBAC_PERM_COMMAND_CHEAT_CASTTIME (RBAC.h:206).
const permissionCommandCheatCasttime uint32 = 292

// permissionCommandCheatCooldown mirrors rbac::RBAC_PERM_COMMAND_CHEAT_COOLDOWN (RBAC.h:207).
const permissionCommandCheatCooldown uint32 = 293

// permissionCommandCheatExplore mirrors rbac::RBAC_PERM_COMMAND_CHEAT_EXPLORE (RBAC.h:208).
const permissionCommandCheatExplore uint32 = 294

// permissionCommandCheatGod mirrors rbac::RBAC_PERM_COMMAND_CHEAT_GOD (RBAC.h:209).
const permissionCommandCheatGod uint32 = 295

// permissionCommandCheatPower mirrors rbac::RBAC_PERM_COMMAND_CHEAT_POWER (RBAC.h:210).
const permissionCommandCheatPower uint32 = 296

// permissionCommandCheatStatus mirrors rbac::RBAC_PERM_COMMAND_CHEAT_STATUS (RBAC.h:211).
const permissionCommandCheatStatus uint32 = 297

// permissionCommandCheatTaxi mirrors rbac::RBAC_PERM_COMMAND_CHEAT_TAXI (RBAC.h:212).
const permissionCommandCheatTaxi uint32 = 298

// permissionCommandCheatWaterwalk mirrors rbac::RBAC_PERM_COMMAND_CHEAT_WATERWALK (RBAC.h:213).
const permissionCommandCheatWaterwalk uint32 = 299

// permissionOpcodeWhois mirrors rbac::RBAC_PERM_OPCODE_WHOIS (RBAC.h:96).
const permissionOpcodeWhois uint32 = 43

// permissionCommandMailbox mirrors rbac::RBAC_PERM_COMMAND_MAILBOX (RBAC.h:645).
const permissionCommandMailbox uint32 = 777

// permissionCommandAchievementAdd mirrors rbac::RBAC_PERM_COMMAND_ACHIEVEMENT_ADD (RBAC.h:145).
const permissionCommandAchievementAdd uint32 = 231

// permissionCommandArenaCaptain mirrors rbac::RBAC_PERM_COMMAND_ARENA_CAPTAIN (RBAC.h:147).
const permissionCommandArenaCaptain uint32 = 233

// permissionCommandArenaCreate mirrors rbac::RBAC_PERM_COMMAND_ARENA_CREATE (RBAC.h:148).
const permissionCommandArenaCreate uint32 = 234

// permissionCommandArenaDisband mirrors rbac::RBAC_PERM_COMMAND_ARENA_DISBAND (RBAC.h:149).
const permissionCommandArenaDisband uint32 = 235

// permissionCommandArenaInfo mirrors rbac::RBAC_PERM_COMMAND_ARENA_INFO (RBAC.h:150).
const permissionCommandArenaInfo uint32 = 236

// permissionCommandArenaLookup mirrors rbac::RBAC_PERM_COMMAND_ARENA_LOOKUP (RBAC.h:151).
const permissionCommandArenaLookup uint32 = 237

// permissionCommandArenaRename mirrors rbac::RBAC_PERM_COMMAND_ARENA_RENAME (RBAC.h:152).
const permissionCommandArenaRename uint32 = 238

// permissionCommandBanAccount mirrors rbac::RBAC_PERM_COMMAND_BAN_ACCOUNT (RBAC.h:154).
const permissionCommandBanAccount uint32 = 240

// permissionCommandBanCharacter mirrors rbac::RBAC_PERM_COMMAND_BAN_CHARACTER (RBAC.h:155).
const permissionCommandBanCharacter uint32 = 241

// permissionCommandBanIP mirrors rbac::RBAC_PERM_COMMAND_BAN_IP (RBAC.h:156).
const permissionCommandBanIP uint32 = 242

// permissionCommandBanPlayerAccount mirrors rbac::RBAC_PERM_COMMAND_BAN_PLAYERACCOUNT (RBAC.h:157).
const permissionCommandBanPlayerAccount uint32 = 243

// permissionCommandBanInfoAccount mirrors rbac::RBAC_PERM_COMMAND_BANINFO_ACCOUNT (RBAC.h:159).
const permissionCommandBanInfoAccount uint32 = 245

// permissionCommandBanInfoCharacter mirrors rbac::RBAC_PERM_COMMAND_BANINFO_CHARACTER (RBAC.h:160).
const permissionCommandBanInfoCharacter uint32 = 246

// permissionCommandBanInfoIP mirrors rbac::RBAC_PERM_COMMAND_BANINFO_IP (RBAC.h:161).
const permissionCommandBanInfoIP uint32 = 247

// permissionCommandBanListAccount mirrors rbac::RBAC_PERM_COMMAND_BANLIST_ACCOUNT (RBAC.h:163).
const permissionCommandBanListAccount uint32 = 249

// permissionCommandBanListCharacter mirrors rbac::RBAC_PERM_COMMAND_BANLIST_CHARACTER (RBAC.h:164).
const permissionCommandBanListCharacter uint32 = 250

// permissionCommandBanListIP mirrors rbac::RBAC_PERM_COMMAND_BANLIST_IP (RBAC.h:165).
const permissionCommandBanListIP uint32 = 251

// permissionCommandUnBanAccount mirrors rbac::RBAC_PERM_COMMAND_UNBAN_ACCOUNT (RBAC.h:167).
const permissionCommandUnBanAccount uint32 = 253

// permissionCommandUnBanCharacter mirrors rbac::RBAC_PERM_COMMAND_UNBAN_CHARACTER (RBAC.h:168).
const permissionCommandUnBanCharacter uint32 = 254

// permissionCommandUnBanIP mirrors rbac::RBAC_PERM_COMMAND_UNBAN_IP (RBAC.h:169).
const permissionCommandUnBanIP uint32 = 255

// permissionCommandUnBanPlayerAccount mirrors rbac::RBAC_PERM_COMMAND_UNBAN_PLAYERACCOUNT (RBAC.h:170).
const permissionCommandUnBanPlayerAccount uint32 = 256

// permissionCommandBfStart mirrors rbac::RBAC_PERM_COMMAND_BF_START (RBAC.h:172).
const permissionCommandBfStart uint32 = 258

// permissionCommandBfStop mirrors rbac::RBAC_PERM_COMMAND_BF_STOP (RBAC.h:173).
const permissionCommandBfStop uint32 = 259

// permissionCommandBfSwitch mirrors rbac::RBAC_PERM_COMMAND_BF_SWITCH (RBAC.h:174).
const permissionCommandBfSwitch uint32 = 260

// permissionCommandBfTimer mirrors rbac::RBAC_PERM_COMMAND_BF_TIMER (RBAC.h:175).
const permissionCommandBfTimer uint32 = 261

// permissionCommandBfEnable mirrors rbac::RBAC_PERM_COMMAND_BF_ENABLE (RBAC.h:176).
const permissionCommandBfEnable uint32 = 262

// permissionCommandCast mirrors rbac::RBAC_PERM_COMMAND_CAST (RBAC.h:181).
const permissionCommandCast uint32 = 267

// permissionCommandCastBack mirrors rbac::RBAC_PERM_COMMAND_CAST_BACK (RBAC.h:182).
const permissionCommandCastBack uint32 = 268

// permissionCommandCastDist mirrors rbac::RBAC_PERM_COMMAND_CAST_DIST (RBAC.h:183).
const permissionCommandCastDist uint32 = 269

// permissionCommandCastSelf mirrors rbac::RBAC_PERM_COMMAND_CAST_SELF (RBAC.h:184).
const permissionCommandCastSelf uint32 = 270

// permissionCommandCastTarget mirrors rbac::RBAC_PERM_COMMAND_CAST_TARGET (RBAC.h:185).
const permissionCommandCastTarget uint32 = 271

// permissionCommandCastDest mirrors rbac::RBAC_PERM_COMMAND_CAST_DEST (RBAC.h:186).
const permissionCommandCastDest uint32 = 272

// permissionCommandCharacterCustomize mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_CUSTOMIZE (RBAC.h:188).
const permissionCommandCharacterCustomize uint32 = 274

// permissionCommandCharacterChangeFaction mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_CHANGEFACTION (RBAC.h:189).
const permissionCommandCharacterChangeFaction uint32 = 275

// permissionCommandCharacterChangeRace mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_CHANGERACE (RBAC.h:190).
const permissionCommandCharacterChangeRace uint32 = 276

// permissionCommandCharacterDeletedDelete mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_DELETED_DELETE (RBAC.h:192).
const permissionCommandCharacterDeletedDelete uint32 = 278

// permissionCommandCharacterDeletedList mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_DELETED_LIST (RBAC.h:193).
const permissionCommandCharacterDeletedList uint32 = 279

// permissionCommandCharacterDeletedRestore mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_DELETED_RESTORE (RBAC.h:194).
const permissionCommandCharacterDeletedRestore uint32 = 280

// permissionCommandCharacterDeletedOld mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_DELETED_OLD (RBAC.h:195).
const permissionCommandCharacterDeletedOld uint32 = 281

// permissionCommandCharacterErase mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_ERASE (RBAC.h:196).
const permissionCommandCharacterErase uint32 = 282

// permissionCommandCharacterLevel mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_LEVEL (RBAC.h:197).
const permissionCommandCharacterLevel uint32 = 283

// permissionCommandCharacterRename mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_RENAME (RBAC.h:198).
const permissionCommandCharacterRename uint32 = 284

// permissionCommandCharacterReputation mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_REPUTATION (RBAC.h:199).
const permissionCommandCharacterReputation uint32 = 285

// permissionCommandCharacterTitles mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_TITLES (RBAC.h:200).
const permissionCommandCharacterTitles uint32 = 286

// permissionCommandLevelup mirrors rbac::RBAC_PERM_COMMAND_LEVELUP (RBAC.h:201).
const permissionCommandLevelup uint32 = 287

// permissionCommandPDumpLoad mirrors rbac::RBAC_PERM_COMMAND_PDUMP_LOAD (RBAC.h:203).
const permissionCommandPDumpLoad uint32 = 289

// permissionCommandPDumpWrite mirrors rbac::RBAC_PERM_COMMAND_PDUMP_WRITE (RBAC.h:204).
const permissionCommandPDumpWrite uint32 = 290

// permissionCommandCharacterChangeAccount mirrors rbac::RBAC_PERM_COMMAND_CHARACTER_CHANGEACCOUNT (RBAC.h:566).
const permissionCommandCharacterChangeAccount uint32 = 698

// permissionCommandPDumpCopy mirrors rbac::RBAC_PERM_COMMAND_PDUMP_COPY (RBAC.h:745).
const permissionCommandPDumpCopy uint32 = 880

// permissionReceiveGlobalGMTextMessage mirrors rbac::RBAC_PERM_RECEIVE_GLOBAL_GM_TEXTMESSAGE (RBAC.h:97).
const permissionReceiveGlobalGMTextMessage uint32 = 44

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
