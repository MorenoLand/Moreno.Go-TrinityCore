package world

import (
	"context"
	"database/sql"
)

const permissionCommandGMChat uint32 = 372

// permissionCommandGM mirrors rbac::RBAC_PERM_COMMAND_GM (RBAC.h:243).
const permissionCommandGM uint32 = 371

// permissionCommandGMFly mirrors rbac::RBAC_PERM_COMMAND_GM_FLY (RBAC.h:245).
const permissionCommandGMFly uint32 = 373

// permissionCommandGMIngame mirrors rbac::RBAC_PERM_COMMAND_GM_INGAME (RBAC.h:246).
const permissionCommandGMIngame uint32 = 374

// permissionCommandGMList mirrors rbac::RBAC_PERM_COMMAND_GM_LIST (RBAC.h:247).
const permissionCommandGMList uint32 = 375

// permissionCommandGMVisible mirrors rbac::RBAC_PERM_COMMAND_GM_VISIBLE (RBAC.h:248).
const permissionCommandGMVisible uint32 = 376

// permissionCommandGO mirrors rbac::RBAC_PERM_COMMAND_GO (RBAC.h:249).
const permissionCommandGO uint32 = 377

// permissionCommandGObjectActivate mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_ACTIVATE (RBAC.h:256).
const permissionCommandGObjectActivate uint32 = 388

// permissionCommandGObjectAdd mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_ADD (RBAC.h:257).
const permissionCommandGObjectAdd uint32 = 389

// permissionCommandGObjectAddTemp mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_ADD_TEMP (RBAC.h:258).
const permissionCommandGObjectAddTemp uint32 = 390

// permissionCommandGObjectDelete mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_DELETE (RBAC.h:259).
const permissionCommandGObjectDelete uint32 = 391

// permissionCommandGObjectInfo mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_INFO (RBAC.h:260).
const permissionCommandGObjectInfo uint32 = 392

// permissionCommandGObjectMove mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_MOVE (RBAC.h:261).
const permissionCommandGObjectMove uint32 = 393

// permissionCommandGObjectNear mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_NEAR (RBAC.h:262).
const permissionCommandGObjectNear uint32 = 394

// permissionCommandGObjectSetPhase mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_SET_PHASE (RBAC.h:264).
const permissionCommandGObjectSetPhase uint32 = 396

// permissionCommandGObjectSetState mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_SET_STATE (RBAC.h:265).
const permissionCommandGObjectSetState uint32 = 397

// permissionCommandGObjectTarget mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_TARGET (RBAC.h:266).
const permissionCommandGObjectTarget uint32 = 398

// permissionCommandGObjectTurn mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_TURN (RBAC.h:267).
const permissionCommandGObjectTurn uint32 = 399

// permissionCommandGObjectSpawnGroup mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_SPAWNGROUP (RBAC.h:725).
const permissionCommandGObjectSpawnGroup uint32 = 858

// permissionCommandGObjectDespawnGroup mirrors rbac::RBAC_PERM_COMMAND_GOBJECT_DESPAWNGROUP (RBAC.h:726).
const permissionCommandGObjectDespawnGroup uint32 = 859

// permissionCommandListRespawns mirrors rbac::RBAC_PERM_COMMAND_LIST_RESPAWNS (RBAC.h:727).
const permissionCommandListRespawns uint32 = 860

// permissionCommandListSpawnpoints mirrors rbac::RBAC_PERM_COMMAND_LIST_SPAWNPOINTS (RBAC.h:733).
const permissionCommandListSpawnpoints uint32 = 866

// permissionCommandAnnounce mirrors rbac::RBAC_PERM_COMMAND_ANNOUNCE (RBAC.h:330).
const permissionCommandAnnounce uint32 = 462

// permissionCommandChannelSetOwnership mirrors rbac::RBAC_PERM_COMMAND_CHANNEL_SET_OWNERSHIP (RBAC.h:333).
const permissionCommandChannelSetOwnership uint32 = 465

// permissionCommandGMAnnounce mirrors rbac::RBAC_PERM_COMMAND_GMANNOUNCE (RBAC.h:334).
const permissionCommandGMAnnounce uint32 = 466

// permissionCommandGMNameAnnounce mirrors rbac::RBAC_PERM_COMMAND_GMNAMEANNOUNCE (RBAC.h:335).
const permissionCommandGMNameAnnounce uint32 = 467

// permissionCommandGMNotify mirrors rbac::RBAC_PERM_COMMAND_GMNOTIFY (RBAC.h:336).
const permissionCommandGMNotify uint32 = 468

// permissionCommandNameAnnounce mirrors rbac::RBAC_PERM_COMMAND_NAMEANNOUNCE (RBAC.h:337).
const permissionCommandNameAnnounce uint32 = 469

// permissionCommandNotify mirrors rbac::RBAC_PERM_COMMAND_NOTIFY (RBAC.h:338).
const permissionCommandNotify uint32 = 470

// permissionCommandWhispers mirrors rbac::RBAC_PERM_COMMAND_WHISPERS (RBAC.h:339).
const permissionCommandWhispers uint32 = 471

// permissionCommandsAppearInGMList mirrors rbac::RBAC_PERM_COMMANDS_APPEAR_IN_GM_LIST (RBAC.h:87).
const permissionCommandsAppearInGMList uint32 = 34

// permissionChatUseStaffBadge mirrors rbac::RBAC_PERM_CHAT_USE_STAFF_BADGE (RBAC.h:90).
const permissionChatUseStaffBadge uint32 = 37

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

// permissionCommandDeserterBGAdd mirrors rbac::RBAC_PERM_COMMAND_DESERTER_BG_ADD (RBAC.h:216).
const permissionCommandDeserterBGAdd uint32 = 343

// permissionCommandDeserterBGRemove mirrors rbac::RBAC_PERM_COMMAND_DESERTER_BG_REMOVE (RBAC.h:217).
const permissionCommandDeserterBGRemove uint32 = 344

// permissionCommandDeserterInstanceAdd mirrors rbac::RBAC_PERM_COMMAND_DESERTER_INSTANCE_ADD (RBAC.h:219).
const permissionCommandDeserterInstanceAdd uint32 = 346

// permissionCommandDeserterInstanceRemove mirrors rbac::RBAC_PERM_COMMAND_DESERTER_INSTANCE_REMOVE (RBAC.h:220).
const permissionCommandDeserterInstanceRemove uint32 = 347

// permissionCommandDisableAddAchievementCriteria mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_ACHIEVEMENT_CRITERIA (RBAC.h:221).
const permissionCommandDisableAddAchievementCriteria uint32 = 350

// permissionCommandDisableAddBattleground mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_BATTLEGROUND (RBAC.h:222).
const permissionCommandDisableAddBattleground uint32 = 351

// permissionCommandDisableAddMap mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_MAP (RBAC.h:223).
const permissionCommandDisableAddMap uint32 = 352

// permissionCommandDisableAddMMap mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_MMAP (RBAC.h:224).
const permissionCommandDisableAddMMap uint32 = 353

// permissionCommandDisableAddOutdoorPvP mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_OUTDOORPVP (RBAC.h:225).
const permissionCommandDisableAddOutdoorPvP uint32 = 354

// permissionCommandDisableAddQuest mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_QUEST (RBAC.h:226).
const permissionCommandDisableAddQuest uint32 = 355

// permissionCommandDisableAddSpell mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_SPELL (RBAC.h:227).
const permissionCommandDisableAddSpell uint32 = 356

// permissionCommandDisableAddVMap mirrors rbac::RBAC_PERM_COMMAND_DISABLE_ADD_VMAP (RBAC.h:228).
const permissionCommandDisableAddVMap uint32 = 357

// 358 previously used, do not reuse.

// permissionCommandDisableRemoveAchievementCriteria mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_ACHIEVEMENT_CRITERIA (RBAC.h:230).
const permissionCommandDisableRemoveAchievementCriteria uint32 = 359

// permissionCommandDisableRemoveBattleground mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_BATTLEGROUND (RBAC.h:231).
const permissionCommandDisableRemoveBattleground uint32 = 360

// permissionCommandDisableRemoveMap mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_MAP (RBAC.h:232).
const permissionCommandDisableRemoveMap uint32 = 361

// permissionCommandDisableRemoveMMap mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_MMAP (RBAC.h:233).
const permissionCommandDisableRemoveMMap uint32 = 362

// permissionCommandDisableRemoveOutdoorPvP mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_OUTDOORPVP (RBAC.h:234).
const permissionCommandDisableRemoveOutdoorPvP uint32 = 363

// permissionCommandDisableRemoveQuest mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_QUEST (RBAC.h:235).
const permissionCommandDisableRemoveQuest uint32 = 364

// permissionCommandDisableRemoveSpell mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_SPELL (RBAC.h:236).
const permissionCommandDisableRemoveSpell uint32 = 365

// permissionCommandDisableRemoveVMap mirrors rbac::RBAC_PERM_COMMAND_DISABLE_REMOVE_VMAP (RBAC.h:237).
const permissionCommandDisableRemoveVMap uint32 = 366

// permissionCommandEventInfo mirrors rbac::RBAC_PERM_COMMAND_EVENT_INFO (RBAC.h:239).
const permissionCommandEventInfo uint32 = 367

// permissionCommandEventActivelist mirrors rbac::RBAC_PERM_COMMAND_EVENT_ACTIVELIST (RBAC.h:240).
const permissionCommandEventActivelist uint32 = 368

// permissionCommandEventStart mirrors rbac::RBAC_PERM_COMMAND_EVENT_START (RBAC.h:241).
const permissionCommandEventStart uint32 = 369

// permissionCommandEventStop mirrors rbac::RBAC_PERM_COMMAND_EVENT_STOP (RBAC.h:242).
const permissionCommandEventStop uint32 = 370

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

// permissionCommandGuild mirrors rbac::RBAC_PERM_COMMAND_GUILD (RBAC.h:269).
const permissionCommandGuild uint32 = 401

// permissionCommandGuildCreate mirrors rbac::RBAC_PERM_COMMAND_GUILD_CREATE (RBAC.h:270).
const permissionCommandGuildCreate uint32 = 402

// permissionCommandGuildDelete mirrors rbac::RBAC_PERM_COMMAND_GUILD_DELETE (RBAC.h:271).
const permissionCommandGuildDelete uint32 = 403

// permissionCommandGuildInvite mirrors rbac::RBAC_PERM_COMMAND_GUILD_INVITE (RBAC.h:272).
const permissionCommandGuildInvite uint32 = 404

// permissionCommandGuildUninvite mirrors rbac::RBAC_PERM_COMMAND_GUILD_UNINVITE (RBAC.h:273).
const permissionCommandGuildUninvite uint32 = 405

// permissionCommandGuildRank mirrors rbac::RBAC_PERM_COMMAND_GUILD_RANK (RBAC.h:274).
const permissionCommandGuildRank uint32 = 406

// permissionCommandGuildRename mirrors rbac::RBAC_PERM_COMMAND_GUILD_RENAME (RBAC.h:275).
const permissionCommandGuildRename uint32 = 407

// permissionCommandGuildInfo mirrors rbac::RBAC_PERM_COMMAND_GUILD_INFO (RBAC.h:662).
const permissionCommandGuildInfo uint32 = 794

// permissionCommandHonor mirrors rbac::RBAC_PERM_COMMAND_HONOR (RBAC.h:276).
const permissionCommandHonor uint32 = 408

// permissionCommandHonorAdd mirrors rbac::RBAC_PERM_COMMAND_HONOR_ADD (RBAC.h:277).
const permissionCommandHonorAdd uint32 = 409

// permissionCommandHonorAddKill mirrors rbac::RBAC_PERM_COMMAND_HONOR_ADD_KILL (RBAC.h:278).
const permissionCommandHonorAddKill uint32 = 410

// permissionCommandHonorUpdate mirrors rbac::RBAC_PERM_COMMAND_HONOR_UPDATE (RBAC.h:279).
const permissionCommandHonorUpdate uint32 = 411

// permissionCommandGroupLeader mirrors rbac::RBAC_PERM_COMMAND_GROUP_LEADER (RBAC.h:341).
const permissionCommandGroupLeader uint32 = 473

// permissionCommandGroupDisband mirrors rbac::RBAC_PERM_COMMAND_GROUP_DISBAND (RBAC.h:342).
const permissionCommandGroupDisband uint32 = 474

// permissionCommandGroupRemove mirrors rbac::RBAC_PERM_COMMAND_GROUP_REMOVE (RBAC.h:343).
const permissionCommandGroupRemove uint32 = 475

// permissionCommandGroupJoin mirrors rbac::RBAC_PERM_COMMAND_GROUP_JOIN (RBAC.h:344).
const permissionCommandGroupJoin uint32 = 476

// permissionCommandGroupList mirrors rbac::RBAC_PERM_COMMAND_GROUP_LIST (RBAC.h:345).
const permissionCommandGroupList uint32 = 477

// permissionCommandGroupSummon mirrors rbac::RBAC_PERM_COMMAND_GROUP_SUMMON (RBAC.h:346).
const permissionCommandGroupSummon uint32 = 478

// permissionCommandGroupSet mirrors rbac::RBAC_PERM_COMMAND_GROUP_SET (RBAC.h:728).
const permissionCommandGroupSet uint32 = 861

// permissionCommandGroupAssistant mirrors rbac::RBAC_PERM_COMMAND_GROUP_ASSISTANT (RBAC.h:729).
const permissionCommandGroupAssistant uint32 = 862

// permissionCommandGroupMainTank mirrors rbac::RBAC_PERM_COMMAND_GROUP_MAINTANK (RBAC.h:730).
const permissionCommandGroupMainTank uint32 = 863

// permissionCommandGroupMainAssist mirrors rbac::RBAC_PERM_COMMAND_GROUP_MAINASSIST (RBAC.h:731).
const permissionCommandGroupMainAssist uint32 = 864

// permissionCommandInstance mirrors rbac::RBAC_PERM_COMMAND_INSTANCE (RBAC.h:280).
const permissionCommandInstance uint32 = 412

// permissionCommandInstanceListBinds mirrors rbac::RBAC_PERM_COMMAND_INSTANCE_LISTBINDS (RBAC.h:281).
const permissionCommandInstanceListBinds uint32 = 413

// permissionCommandInstanceUnbind mirrors rbac::RBAC_PERM_COMMAND_INSTANCE_UNBIND (RBAC.h:282).
const permissionCommandInstanceUnbind uint32 = 414

// permissionCommandInstanceStats mirrors rbac::RBAC_PERM_COMMAND_INSTANCE_STATS (RBAC.h:283).
const permissionCommandInstanceStats uint32 = 415

// permissionCommandInstanceSaveData mirrors rbac::RBAC_PERM_COMMAND_INSTANCE_SAVEDATA (RBAC.h:284).
const permissionCommandInstanceSaveData uint32 = 416

// permissionCommandInstanceSetBossState mirrors rbac::RBAC_PERM_COMMAND_INSTANCE_SET_BOSS_STATE (RBAC.h:663).
const permissionCommandInstanceSetBossState uint32 = 795

// permissionCommandInstanceGetBossState mirrors rbac::RBAC_PERM_COMMAND_INSTANCE_GET_BOSS_STATE (RBAC.h:664).
const permissionCommandInstanceGetBossState uint32 = 796

// permissionCommandLearn mirrors rbac::RBAC_PERM_COMMAND_LEARN (RBAC.h:285).
const permissionCommandLearn uint32 = 417

// 418 previously used, do not reuse (RBAC.h:286).

// permissionCommandLearnAllMy mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_MY (RBAC.h:287).
const permissionCommandLearnAllMy uint32 = 419

// permissionCommandLearnAllMyClass mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_MY_CLASS (RBAC.h:288).
const permissionCommandLearnAllMyClass uint32 = 420

// permissionCommandLearnMyPetTalents mirrors rbac::RBAC_PERM_COMMAND_LEARN_MY_PETTALENTS (RBAC.h:289).
const permissionCommandLearnMyPetTalents uint32 = 421

// permissionCommandLearnAllMySpells mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_MY_SPELLS (RBAC.h:290).
const permissionCommandLearnAllMySpells uint32 = 422

// permissionCommandLearnAllTalents mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_TALENTS (RBAC.h:291).
const permissionCommandLearnAllTalents uint32 = 423

// permissionCommandLearnAllGM mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_GM (RBAC.h:292).
const permissionCommandLearnAllGM uint32 = 424

// permissionCommandLearnAllCrafts mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_CRAFTS (RBAC.h:293).
const permissionCommandLearnAllCrafts uint32 = 425

// permissionCommandLearnAllDefault mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_DEFAULT (RBAC.h:294).
const permissionCommandLearnAllDefault uint32 = 426

// permissionCommandLearnAllLang mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_LANG (RBAC.h:295).
const permissionCommandLearnAllLang uint32 = 427

// permissionCommandLearnAllRecipes mirrors rbac::RBAC_PERM_COMMAND_LEARN_ALL_RECIPES (RBAC.h:296).
const permissionCommandLearnAllRecipes uint32 = 428

// permissionCommandUnlearn mirrors rbac::RBAC_PERM_COMMAND_UNLEARN (RBAC.h:297).
const permissionCommandUnlearn uint32 = 429

// permissionCommandLfgPlayer mirrors rbac::RBAC_PERM_COMMAND_LFG_PLAYER (RBAC.h:299).
const permissionCommandLfgPlayer uint32 = 431

// permissionCommandLfgGroup mirrors rbac::RBAC_PERM_COMMAND_LFG_GROUP (RBAC.h:300).
const permissionCommandLfgGroup uint32 = 432

// permissionCommandLfgQueue mirrors rbac::RBAC_PERM_COMMAND_LFG_QUEUE (RBAC.h:301).
const permissionCommandLfgQueue uint32 = 433

// permissionCommandLfgClean mirrors rbac::RBAC_PERM_COMMAND_LFG_CLEAN (RBAC.h:302).
const permissionCommandLfgClean uint32 = 434

// permissionCommandLfgOptions mirrors rbac::RBAC_PERM_COMMAND_LFG_OPTIONS (RBAC.h:303).
const permissionCommandLfgOptions uint32 = 435

// permissionCommandListCreature mirrors rbac::RBAC_PERM_COMMAND_LIST_CREATURE (RBAC.h:305).
const permissionCommandListCreature uint32 = 437

// permissionCommandListItem mirrors rbac::RBAC_PERM_COMMAND_LIST_ITEM (RBAC.h:306).
const permissionCommandListItem uint32 = 438

// permissionCommandListObject mirrors rbac::RBAC_PERM_COMMAND_LIST_OBJECT (RBAC.h:307).
const permissionCommandListObject uint32 = 439

// permissionCommandListAuras mirrors rbac::RBAC_PERM_COMMAND_LIST_AURAS (RBAC.h:308).
const permissionCommandListAuras uint32 = 440

// permissionCommandListMail mirrors rbac::RBAC_PERM_COMMAND_LIST_MAIL (RBAC.h:309).
const permissionCommandListMail uint32 = 441

// permissionCommandLookup mirrors rbac::RBAC_PERM_COMMAND_LOOKUP (RBAC.h:310).
const permissionCommandLookup uint32 = 442

// permissionCommandLookupArea mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_AREA (RBAC.h:311).
const permissionCommandLookupArea uint32 = 443

// permissionCommandLookupCreature mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_CREATURE (RBAC.h:312).
const permissionCommandLookupCreature uint32 = 444

// permissionCommandLookupEvent mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_EVENT (RBAC.h:313).
const permissionCommandLookupEvent uint32 = 445

// permissionCommandLookupFaction mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_FACTION (RBAC.h:314).
const permissionCommandLookupFaction uint32 = 446

// permissionCommandLookupItem mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_ITEM (RBAC.h:315).
const permissionCommandLookupItem uint32 = 447

// permissionCommandLookupItemSet mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_ITEMSET (RBAC.h:316).
const permissionCommandLookupItemSet uint32 = 448

// permissionCommandLookupObject mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_OBJECT (RBAC.h:317).
const permissionCommandLookupObject uint32 = 449

// permissionCommandLookupQuest mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_QUEST (RBAC.h:318).
const permissionCommandLookupQuest uint32 = 450

// permissionCommandLookupPlayer mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_PLAYER (RBAC.h:319).
const permissionCommandLookupPlayer uint32 = 451

// permissionCommandLookupPlayerIP mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_PLAYER_IP (RBAC.h:320).
const permissionCommandLookupPlayerIP uint32 = 452

// permissionCommandLookupPlayerAccount mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_PLAYER_ACCOUNT (RBAC.h:321).
const permissionCommandLookupPlayerAccount uint32 = 453

// permissionCommandLookupPlayerEmail mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_PLAYER_EMAIL (RBAC.h:322).
const permissionCommandLookupPlayerEmail uint32 = 454

// permissionCommandLookupSkill mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_SKILL (RBAC.h:323).
const permissionCommandLookupSkill uint32 = 455

// permissionCommandLookupSpell mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_SPELL (RBAC.h:324).
const permissionCommandLookupSpell uint32 = 456

// permissionCommandLookupSpellID mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_SPELL_ID (RBAC.h:325).
const permissionCommandLookupSpellID uint32 = 457

// permissionCommandLookupTaxinode mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_TAXINODE (RBAC.h:326).
const permissionCommandLookupTaxinode uint32 = 458

// permissionCommandLookupTele mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_TELE (RBAC.h:327).
const permissionCommandLookupTele uint32 = 459

// permissionCommandLookupTitle mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_TITLE (RBAC.h:328).
const permissionCommandLookupTitle uint32 = 460

// permissionCommandLookupMap mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_MAP (RBAC.h:329).
const permissionCommandLookupMap uint32 = 461

// permissionCommandLookupMapID mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_MAP_ID (RBAC.h:741).
const permissionCommandLookupMapID uint32 = 875

// permissionCommandLookupItemID mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_ITEM_ID (RBAC.h:742).
const permissionCommandLookupItemID uint32 = 876

// permissionCommandLookupQuestID mirrors rbac::RBAC_PERM_COMMAND_LOOKUP_QUEST_ID (RBAC.h:743).
const permissionCommandLookupQuestID uint32 = 877

// permissionCommandAddItem mirrors rbac::RBAC_PERM_COMMAND_ADDITEM (RBAC.h:356).
const permissionCommandAddItem uint32 = 488

// permissionCommandAddItemSet mirrors rbac::RBAC_PERM_COMMAND_ADDITEMSET (RBAC.h:357).
const permissionCommandAddItemSet uint32 = 489

// permissionCommandAppear mirrors rbac::RBAC_PERM_COMMAND_APPEAR (RBAC.h:358).
const permissionCommandAppear uint32 = 490

// permissionCommandAura mirrors rbac::RBAC_PERM_COMMAND_AURA (RBAC.h:359).
const permissionCommandAura uint32 = 491

// permissionCommandBank mirrors rbac::RBAC_PERM_COMMAND_BANK (RBAC.h:360).
const permissionCommandBank uint32 = 492

// permissionCommandBindSight mirrors rbac::RBAC_PERM_COMMAND_BINDSIGHT (RBAC.h:361).
const permissionCommandBindSight uint32 = 493

// permissionCommandCombatStop mirrors rbac::RBAC_PERM_COMMAND_COMBATSTOP (RBAC.h:362).
const permissionCommandCombatStop uint32 = 494

// permissionCommandComeToMe mirrors rbac::RBAC_PERM_COMMAND_COMETOME (RBAC.h:363).
const permissionCommandComeToMe uint32 = 495

// permissionCommandCommands mirrors rbac::RBAC_PERM_COMMAND_COMMANDS (RBAC.h:364).
const permissionCommandCommands uint32 = 496

// permissionCommandCooldown mirrors rbac::RBAC_PERM_COMMAND_COOLDOWN (RBAC.h:365).
const permissionCommandCooldown uint32 = 497

// permissionCommandDamage mirrors rbac::RBAC_PERM_COMMAND_DAMAGE (RBAC.h:366).
const permissionCommandDamage uint32 = 498

// permissionCommandDev mirrors rbac::RBAC_PERM_COMMAND_DEV (RBAC.h:367).
const permissionCommandDev uint32 = 499

// permissionCommandDie mirrors rbac::RBAC_PERM_COMMAND_DIE (RBAC.h:368).
const permissionCommandDie uint32 = 500

// permissionCommandDismount mirrors rbac::RBAC_PERM_COMMAND_DISMOUNT (RBAC.h:369).
const permissionCommandDismount uint32 = 501

// permissionCommandDistance mirrors rbac::RBAC_PERM_COMMAND_DISTANCE (RBAC.h:370).
const permissionCommandDistance uint32 = 502

// permissionCommandFlushArenaPoints mirrors rbac::RBAC_PERM_COMMAND_FLUSHARENAPOINTS (RBAC.h:371).
const permissionCommandFlushArenaPoints uint32 = 503

// permissionCommandFreeze mirrors rbac::RBAC_PERM_COMMAND_FREEZE (RBAC.h:372).
const permissionCommandFreeze uint32 = 504

// permissionCommandUnAura mirrors rbac::RBAC_PERM_COMMAND_UNAURA (RBAC.h:397).
const permissionCommandUnAura uint32 = 529

// permissionCommandUnBindSight mirrors rbac::RBAC_PERM_COMMAND_UNBINDSIGHT (RBAC.h:398).
const permissionCommandUnBindSight uint32 = 530

// permissionCommandGPS mirrors rbac::RBAC_PERM_COMMAND_GPS (RBAC.h:373).
const permissionCommandGPS uint32 = 505

// permissionCommandGUID mirrors rbac::RBAC_PERM_COMMAND_GUID (RBAC.h:374).
const permissionCommandGUID uint32 = 506

// permissionCommandHelp mirrors rbac::RBAC_PERM_COMMAND_HELP (RBAC.h:375).
const permissionCommandHelp uint32 = 507

// permissionCommandHideArea mirrors rbac::RBAC_PERM_COMMAND_HIDEAREA (RBAC.h:376).
const permissionCommandHideArea uint32 = 508

// permissionCommandItemMove mirrors rbac::RBAC_PERM_COMMAND_ITEMMOVE (RBAC.h:377).
const permissionCommandItemMove uint32 = 509

// permissionCommandKick mirrors rbac::RBAC_PERM_COMMAND_KICK (RBAC.h:378).
const permissionCommandKick uint32 = 510

// permissionCommandLinkGrave mirrors rbac::RBAC_PERM_COMMAND_LINKGRAVE (RBAC.h:379).
const permissionCommandLinkGrave uint32 = 511

// permissionCommandListFreeze mirrors rbac::RBAC_PERM_COMMAND_LISTFREEZE (RBAC.h:380).
const permissionCommandListFreeze uint32 = 512

// permissionCommandMaxSkill mirrors rbac::RBAC_PERM_COMMAND_MAXSKILL (RBAC.h:381).
const permissionCommandMaxSkill uint32 = 513

// permissionCommandMovegens mirrors rbac::RBAC_PERM_COMMAND_MOVEGENS (RBAC.h:382).
const permissionCommandMovegens uint32 = 514

// permissionCommandMute mirrors rbac::RBAC_PERM_COMMAND_MUTE (RBAC.h:383).
const permissionCommandMute uint32 = 515

// permissionCommandNearGrave mirrors rbac::RBAC_PERM_COMMAND_NEARGRAVE (RBAC.h:384).
const permissionCommandNearGrave uint32 = 516

// permissionCommandPInfo mirrors rbac::RBAC_PERM_COMMAND_PINFO (RBAC.h:385).
const permissionCommandPInfo uint32 = 517

// permissionCommandPlayAll mirrors rbac::RBAC_PERM_COMMAND_PLAYALL (RBAC.h:386).
const permissionCommandPlayAll uint32 = 518

// permissionCommandPossess mirrors rbac::RBAC_PERM_COMMAND_POSSESS (RBAC.h:387).
const permissionCommandPossess uint32 = 519

// permissionCommandRecall mirrors rbac::RBAC_PERM_COMMAND_RECALL (RBAC.h:388).
const permissionCommandRecall uint32 = 520

// permissionCommandRepairItems mirrors rbac::RBAC_PERM_COMMAND_REPAIRITEMS (RBAC.h:389).
const permissionCommandRepairItems uint32 = 521

// permissionCommandRespawn mirrors rbac::RBAC_PERM_COMMAND_RESPAWN (RBAC.h:390).
const permissionCommandRespawn uint32 = 522

// permissionCommandRevive mirrors rbac::RBAC_PERM_COMMAND_REVIVE (RBAC.h:391).
const permissionCommandRevive uint32 = 523

// permissionCommandSaveAll mirrors rbac::RBAC_PERM_COMMAND_SAVEALL (RBAC.h:392).
const permissionCommandSaveAll uint32 = 524

// permissionCommandSave mirrors rbac::RBAC_PERM_COMMAND_SAVE (RBAC.h:393).
const permissionCommandSave uint32 = 525

// permissionCommandSetSkill mirrors rbac::RBAC_PERM_COMMAND_SETSKILL (RBAC.h:394).
const permissionCommandSetSkill uint32 = 526

// permissionCommandShowArea mirrors rbac::RBAC_PERM_COMMAND_SHOWAREA (RBAC.h:395).
const permissionCommandShowArea uint32 = 527

// permissionCommandSummon mirrors rbac::RBAC_PERM_COMMAND_SUMMON (RBAC.h:396).
const permissionCommandSummon uint32 = 528

// permissionCommandUnFreeze mirrors rbac::RBAC_PERM_COMMAND_UNFREEZE (RBAC.h:399).
const permissionCommandUnFreeze uint32 = 531

// permissionCommandUnMute mirrors rbac::RBAC_PERM_COMMAND_UNMUTE (RBAC.h:400).
const permissionCommandUnMute uint32 = 532

// permissionCommandUnPossess mirrors rbac::RBAC_PERM_COMMAND_UNPOSSESS (RBAC.h:401).
const permissionCommandUnPossess uint32 = 533

// permissionCommandUnstuck mirrors rbac::RBAC_PERM_COMMAND_UNSTUCK (RBAC.h:402).
const permissionCommandUnstuck uint32 = 534

// permissionCommandChangeWeather mirrors rbac::RBAC_PERM_COMMAND_WCHANGE (RBAC.h:403).
const permissionCommandChangeWeather uint32 = 535

// permissionCommandMuteHistory mirrors rbac::RBAC_PERM_COMMAND_MUTEHISTORY (RBAC.h:500).
const permissionCommandMuteHistory uint32 = 632

// permissionCommandMailBox mirrors rbac::RBAC_PERM_COMMAND_MAILBOX (RBAC.h:645).
const permissionCommandMailBox uint32 = 777

// permissionCommandPvPstats mirrors rbac::RBAC_PERM_COMMAND_PVPSTATS (RBAC.h:665).
const permissionCommandPvPstats uint32 = 797

// permissionCommandsSaveWithoutDelay mirrors rbac::RBAC_PERM_COMMANDS_SAVE_WITHOUT_DELAY (RBAC.h:83).
const permissionCommandsSaveWithoutDelay uint32 = 30

// permissionCommandsUseUnstuckWithArgs mirrors rbac::RBAC_PERM_COMMANDS_USE_UNSTUCK_WITH_ARGS (RBAC.h:84).
const permissionCommandsUseUnstuckWithArgs uint32 = 31

// permissionResurrectWithFullHPS mirrors rbac::RBAC_PERM_RESURRECT_WITH_FULL_HPS (RBAC.h:91).
const permissionResurrectWithFullHPS uint32 = 38

// permissionCommandMMap mirrors rbac::RBAC_PERM_COMMAND_MMAP (RBAC.h:404).
const permissionCommandMMap uint32 = 536

// permissionCommandMMapLoadedTiles mirrors rbac::RBAC_PERM_COMMAND_MMAP_LOADEDTILES (RBAC.h:405).
const permissionCommandMMapLoadedTiles uint32 = 537

// permissionCommandMMapLoc mirrors rbac::RBAC_PERM_COMMAND_MMAP_LOC (RBAC.h:406).
const permissionCommandMMapLoc uint32 = 538

// permissionCommandMMapPath mirrors rbac::RBAC_PERM_COMMAND_MMAP_PATH (RBAC.h:407).
const permissionCommandMMapPath uint32 = 539

// permissionCommandMMapStats mirrors rbac::RBAC_PERM_COMMAND_MMAP_STATS (RBAC.h:408).
const permissionCommandMMapStats uint32 = 540

// permissionCommandMMapTestArea mirrors rbac::RBAC_PERM_COMMAND_MMAP_TESTAREA (RBAC.h:409).
const permissionCommandMMapTestArea uint32 = 541

// permissionCommandMorph mirrors rbac::RBAC_PERM_COMMAND_MORPH (RBAC.h:410).
const permissionCommandMorph uint32 = 542

// permissionCommandDeMorph mirrors rbac::RBAC_PERM_COMMAND_DEMORPH (RBAC.h:411).
const permissionCommandDeMorph uint32 = 543

// permissionCommandModify mirrors rbac::RBAC_PERM_COMMAND_MODIFY (RBAC.h:412).
const permissionCommandModify uint32 = 544

// permissionCommandModifyArenaPoints mirrors rbac::RBAC_PERM_COMMAND_MODIFY_ARENAPOINTS (RBAC.h:413).
const permissionCommandModifyArenaPoints uint32 = 545

// permissionCommandModifyBit mirrors rbac::RBAC_PERM_COMMAND_MODIFY_BIT (RBAC.h:414).
const permissionCommandModifyBit uint32 = 546

// permissionCommandModifyDrunk mirrors rbac::RBAC_PERM_COMMAND_MODIFY_DRUNK (RBAC.h:415).
const permissionCommandModifyDrunk uint32 = 547

// permissionCommandModifyEnergy mirrors rbac::RBAC_PERM_COMMAND_MODIFY_ENERGY (RBAC.h:416).
const permissionCommandModifyEnergy uint32 = 548

// permissionCommandModifyFaction mirrors rbac::RBAC_PERM_COMMAND_MODIFY_FACTION (RBAC.h:417).
const permissionCommandModifyFaction uint32 = 549

// permissionCommandModifyGender mirrors rbac::RBAC_PERM_COMMAND_MODIFY_GENDER (RBAC.h:418).
const permissionCommandModifyGender uint32 = 550

// permissionCommandModifyHonor mirrors rbac::RBAC_PERM_COMMAND_MODIFY_HONOR (RBAC.h:419).
const permissionCommandModifyHonor uint32 = 551

// permissionCommandModifyHP mirrors rbac::RBAC_PERM_COMMAND_MODIFY_HP (RBAC.h:420).
const permissionCommandModifyHP uint32 = 552

// permissionCommandModifyMana mirrors rbac::RBAC_PERM_COMMAND_MODIFY_MANA (RBAC.h:421).
const permissionCommandModifyMana uint32 = 553

// permissionCommandModifyMoney mirrors rbac::RBAC_PERM_COMMAND_MODIFY_MONEY (RBAC.h:422).
const permissionCommandModifyMoney uint32 = 554

// permissionCommandModifyMount mirrors rbac::RBAC_PERM_COMMAND_MODIFY_MOUNT (RBAC.h:423).
const permissionCommandModifyMount uint32 = 555

// permissionCommandModifyPhase mirrors rbac::RBAC_PERM_COMMAND_MODIFY_PHASE (RBAC.h:424).
const permissionCommandModifyPhase uint32 = 556

// permissionCommandModifyRage mirrors rbac::RBAC_PERM_COMMAND_MODIFY_RAGE (RBAC.h:425).
const permissionCommandModifyRage uint32 = 557

// permissionCommandModifyReputation mirrors rbac::RBAC_PERM_COMMAND_MODIFY_REPUTATION (RBAC.h:426).
const permissionCommandModifyReputation uint32 = 558

// permissionCommandModifyRunicPower mirrors rbac::RBAC_PERM_COMMAND_MODIFY_RUNICPOWER (RBAC.h:427).
const permissionCommandModifyRunicPower uint32 = 559

// permissionCommandModifyScale mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SCALE (RBAC.h:428).
const permissionCommandModifyScale uint32 = 560

// permissionCommandModifySpeed mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPEED (RBAC.h:429).
const permissionCommandModifySpeed uint32 = 561

// permissionCommandModifySpeedAll mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPEED_ALL (RBAC.h:430).
const permissionCommandModifySpeedAll uint32 = 562

// permissionCommandModifySpeedBackwalk mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPEED_BACKWALK (RBAC.h:431).
const permissionCommandModifySpeedBackwalk uint32 = 563

// permissionCommandModifySpeedFly mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPEED_FLY (RBAC.h:432).
const permissionCommandModifySpeedFly uint32 = 564

// permissionCommandModifySpeedWalk mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPEED_WALK (RBAC.h:433).
const permissionCommandModifySpeedWalk uint32 = 565

// permissionCommandModifySpeedSwim mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPEED_SWIM (RBAC.h:434).
const permissionCommandModifySpeedSwim uint32 = 566

// permissionCommandModifySpell mirrors rbac::RBAC_PERM_COMMAND_MODIFY_SPELL (RBAC.h:435).
const permissionCommandModifySpell uint32 = 567

// permissionCommandModifyStandState mirrors rbac::RBAC_PERM_COMMAND_MODIFY_STANDSTATE (RBAC.h:436).
const permissionCommandModifyStandState uint32 = 568

// permissionCommandModifyTalentPoints mirrors rbac::RBAC_PERM_COMMAND_MODIFY_TALENTPOINTS (RBAC.h:437).
const permissionCommandModifyTalentPoints uint32 = 569

// permissionCommandModifyXP mirrors rbac::RBAC_PERM_COMMAND_MODIFY_XP (RBAC.h:666).
const permissionCommandModifyXP uint32 = 798

// permissionCommandNPCAdd mirrors rbac::RBAC_PERM_COMMAND_NPC_ADD (RBAC.h:439).
const permissionCommandNPCAdd uint32 = 571

// permissionCommandNPCAddFormation mirrors rbac::RBAC_PERM_COMMAND_NPC_ADD_FORMATION (RBAC.h:440).
const permissionCommandNPCAddFormation uint32 = 572

// permissionCommandNPCAddItem mirrors rbac::RBAC_PERM_COMMAND_NPC_ADD_ITEM (RBAC.h:441).
const permissionCommandNPCAddItem uint32 = 573

// permissionCommandNPCAddMove mirrors rbac::RBAC_PERM_COMMAND_NPC_ADD_MOVE (RBAC.h:442).
const permissionCommandNPCAddMove uint32 = 574

// permissionCommandNPCAddTemp mirrors rbac::RBAC_PERM_COMMAND_NPC_ADD_TEMP (RBAC.h:443).
const permissionCommandNPCAddTemp uint32 = 575

// permissionCommandNPCDelete mirrors rbac::RBAC_PERM_COMMAND_NPC_DELETE (RBAC.h:444).
const permissionCommandNPCDelete uint32 = 576

// permissionCommandNPCDeleteItem mirrors rbac::RBAC_PERM_COMMAND_NPC_DELETE_ITEM (RBAC.h:445).
const permissionCommandNPCDeleteItem uint32 = 577

// permissionCommandNPCFollow mirrors rbac::RBAC_PERM_COMMAND_NPC_FOLLOW (RBAC.h:446).
const permissionCommandNPCFollow uint32 = 578

// permissionCommandNPCFollowStop mirrors rbac::RBAC_PERM_COMMAND_NPC_FOLLOW_STOP (RBAC.h:447).
const permissionCommandNPCFollowStop uint32 = 579

// permissionCommandNPCSet mirrors rbac::RBAC_PERM_COMMAND_NPC_SET (RBAC.h:448).
const permissionCommandNPCSet uint32 = 580

// permissionCommandNPCSetAllowMove mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_ALLOWMOVE (RBAC.h:449).
const permissionCommandNPCSetAllowMove uint32 = 581

// permissionCommandNPCSetEntry mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_ENTRY (RBAC.h:450).
const permissionCommandNPCSetEntry uint32 = 582

// permissionCommandNPCSetFactionID mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_FACTIONID (RBAC.h:451).
const permissionCommandNPCSetFactionID uint32 = 583

// permissionCommandNPCSetFlag mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_FLAG (RBAC.h:452).
const permissionCommandNPCSetFlag uint32 = 584

// permissionCommandNPCSetLevel mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_LEVEL (RBAC.h:453).
const permissionCommandNPCSetLevel uint32 = 585

// permissionCommandNPCSetLink mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_LINK (RBAC.h:454).
const permissionCommandNPCSetLink uint32 = 586

// permissionCommandNPCSetModel mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_MODEL (RBAC.h:455).
const permissionCommandNPCSetModel uint32 = 587

// permissionCommandNPCSetMoveType mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_MOVETYPE (RBAC.h:456).
const permissionCommandNPCSetMoveType uint32 = 588

// permissionCommandNPCSetPhase mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_PHASE (RBAC.h:457).
const permissionCommandNPCSetPhase uint32 = 589

// permissionCommandNPCSetSpawnDist mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_SPAWNDIST (RBAC.h:458).
const permissionCommandNPCSetSpawnDist uint32 = 590

// permissionCommandNPCSetSpawnTime mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_SPAWNTIME (RBAC.h:459).
const permissionCommandNPCSetSpawnTime uint32 = 591

// permissionCommandNPCSetData mirrors rbac::RBAC_PERM_COMMAND_NPC_SET_DATA (RBAC.h:460).
const permissionCommandNPCSetData uint32 = 592

// permissionCommandNPCInfo mirrors rbac::RBAC_PERM_COMMAND_NPC_INFO (RBAC.h:461).
const permissionCommandNPCInfo uint32 = 593

// permissionCommandNPCNear mirrors rbac::RBAC_PERM_COMMAND_NPC_NEAR (RBAC.h:462).
const permissionCommandNPCNear uint32 = 594

// permissionCommandNPCMove mirrors rbac::RBAC_PERM_COMMAND_NPC_MOVE (RBAC.h:463).
const permissionCommandNPCMove uint32 = 595

// permissionCommandNPCPlayEmote mirrors rbac::RBAC_PERM_COMMAND_NPC_PLAYEMOTE (RBAC.h:464).
const permissionCommandNPCPlayEmote uint32 = 596

// permissionCommandNPCSay mirrors rbac::RBAC_PERM_COMMAND_NPC_SAY (RBAC.h:465).
const permissionCommandNPCSay uint32 = 597

// permissionCommandNPCTextEmote mirrors rbac::RBAC_PERM_COMMAND_NPC_TEXTEMOTE (RBAC.h:466).
const permissionCommandNPCTextEmote uint32 = 598

// permissionCommandNPCWhisper mirrors rbac::RBAC_PERM_COMMAND_NPC_WHISPER (RBAC.h:467).
const permissionCommandNPCWhisper uint32 = 599

// permissionCommandNPCYell mirrors rbac::RBAC_PERM_COMMAND_NPC_YELL (RBAC.h:468).
const permissionCommandNPCYell uint32 = 600

// permissionCommandNPCTame mirrors rbac::RBAC_PERM_COMMAND_NPC_TAME (RBAC.h:469).
const permissionCommandNPCTame uint32 = 601

// permissionCommandNPCEvade mirrors rbac::RBAC_PERM_COMMAND_NPC_EVADE (RBAC.h:704).
const permissionCommandNPCEvade uint32 = 837

// permissionCommandNPCSpawnGroup mirrors rbac::RBAC_PERM_COMMAND_NPC_SPAWNGROUP (RBAC.h:723).
const permissionCommandNPCSpawnGroup uint32 = 856

// permissionCommandNPCDespawnGroup mirrors rbac::RBAC_PERM_COMMAND_NPC_DESPAWNGROUP (RBAC.h:724).
const permissionCommandNPCDespawnGroup uint32 = 857

// permissionCommandNPCShowLoot mirrors rbac::RBAC_PERM_COMMAND_NPC_SHOWLOOT (RBAC.h:732).
const permissionCommandNPCShowLoot uint32 = 865

// permissionCommandQuest mirrors rbac::RBAC_PERM_COMMAND_QUEST (RBAC.h:470).
const permissionCommandQuest uint32 = 602

// permissionCommandQuestAdd mirrors rbac::RBAC_PERM_COMMAND_QUEST_ADD (RBAC.h:471).
const permissionCommandQuestAdd uint32 = 603

// permissionCommandQuestComplete mirrors rbac::RBAC_PERM_COMMAND_QUEST_COMPLETE (RBAC.h:472).
const permissionCommandQuestComplete uint32 = 604

// permissionCommandQuestRemove mirrors rbac::RBAC_PERM_COMMAND_QUEST_REMOVE (RBAC.h:473).
const permissionCommandQuestRemove uint32 = 605

// permissionCommandQuestReward mirrors rbac::RBAC_PERM_COMMAND_QUEST_REWARD (RBAC.h:474).
const permissionCommandQuestReward uint32 = 606

// permissionCommandRBAC mirrors rbac::RBAC_PERM_COMMAND_RBAC (RBAC.h:114).
const permissionCommandRBAC uint32 = 200

// permissionCommandRBACAcc mirrors rbac::RBAC_PERM_COMMAND_RBAC_ACC (RBAC.h:115).
const permissionCommandRBACAcc uint32 = 201

// permissionCommandRBACAccPermList mirrors rbac::RBAC_PERM_COMMAND_RBAC_ACC_PERM_LIST (RBAC.h:116).
const permissionCommandRBACAccPermList uint32 = 202

// permissionCommandRBACAccPermGrant mirrors rbac::RBAC_PERM_COMMAND_RBAC_ACC_PERM_GRANT (RBAC.h:117).
const permissionCommandRBACAccPermGrant uint32 = 203

// permissionCommandRBACAccPermDeny mirrors rbac::RBAC_PERM_COMMAND_RBAC_ACC_PERM_DENY (RBAC.h:118).
const permissionCommandRBACAccPermDeny uint32 = 204

// permissionCommandRBACAccPermRevoke mirrors rbac::RBAC_PERM_COMMAND_RBAC_ACC_PERM_REVOKE (RBAC.h:119).
const permissionCommandRBACAccPermRevoke uint32 = 205

// permissionCommandRBACList mirrors rbac::RBAC_PERM_COMMAND_RBAC_LIST (RBAC.h:120).
const permissionCommandRBACList uint32 = 206

// permissionCommandPet mirrors rbac::RBAC_PERM_COMMAND_PET (RBAC.h:347).
const permissionCommandPet uint32 = 479

// permissionCommandPetCreate mirrors rbac::RBAC_PERM_COMMAND_PET_CREATE (RBAC.h:348).
const permissionCommandPetCreate uint32 = 480

// permissionCommandPetLearn mirrors rbac::RBAC_PERM_COMMAND_PET_LEARN (RBAC.h:349).
const permissionCommandPetLearn uint32 = 481

// permissionCommandPetUnlearn mirrors rbac::RBAC_PERM_COMMAND_PET_UNLEARN (RBAC.h:350).
const permissionCommandPetUnlearn uint32 = 482

// permissionCommandPetLevel mirrors rbac::RBAC_PERM_COMMAND_PET_LEVEL (RBAC.h:705).
const permissionCommandPetLevel uint32 = 838

// permissionCommandReload mirrors rbac::RBAC_PERM_COMMAND_RELOAD (RBAC.h).
const permissionCommandReload uint32 = 607

// permissionCommandReloadAccessRequirement mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ACCESS_REQUIREMENT (RBAC.h).
const permissionCommandReloadAccessRequirement uint32 = 608

// permissionCommandReloadAchievementCriteriaData mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ACHIEVEMENT_CRITERIA_DATA (RBAC.h).
const permissionCommandReloadAchievementCriteriaData uint32 = 609

// permissionCommandReloadAchievementReward mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ACHIEVEMENT_REWARD (RBAC.h).
const permissionCommandReloadAchievementReward uint32 = 610

// permissionCommandReloadAll mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL (RBAC.h).
const permissionCommandReloadAll uint32 = 611

// permissionCommandReloadAllAchievement mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_ACHIEVEMENT (RBAC.h).
const permissionCommandReloadAllAchievement uint32 = 612

// permissionCommandReloadAllArea mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_AREA (RBAC.h).
const permissionCommandReloadAllArea uint32 = 613

// permissionCommandReloadBroadcastText mirrors rbac::RBAC_PERM_COMMAND_RELOAD_BROADCAST_TEXT (RBAC.h).
const permissionCommandReloadBroadcastText uint32 = 614

// permissionCommandReloadAllGossip mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_GOSSIP (RBAC.h).
const permissionCommandReloadAllGossip uint32 = 615

// permissionCommandReloadAllItem mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_ITEM (RBAC.h).
const permissionCommandReloadAllItem uint32 = 616

// permissionCommandReloadAllLocales mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_LOCALES (RBAC.h).
const permissionCommandReloadAllLocales uint32 = 617

// permissionCommandReloadAllLoot mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_LOOT (RBAC.h).
const permissionCommandReloadAllLoot uint32 = 618

// permissionCommandReloadAllNpc mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_NPC (RBAC.h).
const permissionCommandReloadAllNpc uint32 = 619

// permissionCommandReloadAllQuest mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_QUEST (RBAC.h).
const permissionCommandReloadAllQuest uint32 = 620

// permissionCommandReloadAllScripts mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_SCRIPTS (RBAC.h).
const permissionCommandReloadAllScripts uint32 = 621

// permissionCommandReloadAllSpell mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ALL_SPELL (RBAC.h).
const permissionCommandReloadAllSpell uint32 = 622

// permissionCommandReloadAreatriggerInvolvedrelation mirrors rbac::RBAC_PERM_COMMAND_RELOAD_AREATRIGGER_INVOLVEDRELATION (RBAC.h).
const permissionCommandReloadAreatriggerInvolvedrelation uint32 = 623

// permissionCommandReloadAreatriggerTavern mirrors rbac::RBAC_PERM_COMMAND_RELOAD_AREATRIGGER_TAVERN (RBAC.h).
const permissionCommandReloadAreatriggerTavern uint32 = 624

// permissionCommandReloadAreatriggerTeleport mirrors rbac::RBAC_PERM_COMMAND_RELOAD_AREATRIGGER_TELEPORT (RBAC.h).
const permissionCommandReloadAreatriggerTeleport uint32 = 625

// permissionCommandReloadAuctions mirrors rbac::RBAC_PERM_COMMAND_RELOAD_AUCTIONS (RBAC.h).
const permissionCommandReloadAuctions uint32 = 626

// permissionCommandReloadAutobroadcast mirrors rbac::RBAC_PERM_COMMAND_RELOAD_AUTOBROADCAST (RBAC.h).
const permissionCommandReloadAutobroadcast uint32 = 627

// permissionCommandReloadConditions mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CONDITIONS (RBAC.h).
const permissionCommandReloadConditions uint32 = 629

// permissionCommandReloadConfig mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CONFIG (RBAC.h).
const permissionCommandReloadConfig uint32 = 630

// permissionCommandReloadBattlegroundTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_BATTLEGROUND_TEMPLATE (RBAC.h).
const permissionCommandReloadBattlegroundTemplate uint32 = 631

// permissionCommandReloadCreatureLinkedRespawn mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_LINKED_RESPAWN (RBAC.h).
const permissionCommandReloadCreatureLinkedRespawn uint32 = 633

// permissionCommandReloadCreatureLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadCreatureLootTemplate uint32 = 634

// permissionCommandReloadCreatureOnkillReputation mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_ONKILL_REPUTATION (RBAC.h).
const permissionCommandReloadCreatureOnkillReputation uint32 = 635

// permissionCommandReloadCreatureQuestender mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_QUESTENDER (RBAC.h).
const permissionCommandReloadCreatureQuestender uint32 = 636

// permissionCommandReloadCreatureQueststarter mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_QUESTSTARTER (RBAC.h).
const permissionCommandReloadCreatureQueststarter uint32 = 637

// permissionCommandReloadCreatureSummonGroups mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_SUMMON_GROUPS (RBAC.h).
const permissionCommandReloadCreatureSummonGroups uint32 = 638

// permissionCommandReloadCreatureTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_TEMPLATE (RBAC.h).
const permissionCommandReloadCreatureTemplate uint32 = 639

// permissionCommandReloadCreatureText mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_TEXT (RBAC.h).
const permissionCommandReloadCreatureText uint32 = 640

// permissionCommandReloadDisables mirrors rbac::RBAC_PERM_COMMAND_RELOAD_DISABLES (RBAC.h).
const permissionCommandReloadDisables uint32 = 641

// permissionCommandReloadDisenchantLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_DISENCHANT_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadDisenchantLootTemplate uint32 = 642

// permissionCommandReloadEventScripts mirrors rbac::RBAC_PERM_COMMAND_RELOAD_EVENT_SCRIPTS (RBAC.h).
const permissionCommandReloadEventScripts uint32 = 643

// permissionCommandReloadFishingLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_FISHING_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadFishingLootTemplate uint32 = 644

// permissionCommandReloadGraveyardZone mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GRAVEYARD_ZONE (RBAC.h).
const permissionCommandReloadGraveyardZone uint32 = 645

// permissionCommandReloadGameTele mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GAME_TELE (RBAC.h).
const permissionCommandReloadGameTele uint32 = 646

// permissionCommandReloadGameobjectQuestender mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GAMEOBJECT_QUESTENDER (RBAC.h).
const permissionCommandReloadGameobjectQuestender uint32 = 647

// permissionCommandReloadGameobjectQuestLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GAMEOBJECT_QUEST_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadGameobjectQuestLootTemplate uint32 = 648

// permissionCommandReloadGameobjectQueststarter mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GAMEOBJECT_QUESTSTARTER (RBAC.h).
const permissionCommandReloadGameobjectQueststarter uint32 = 649

// permissionCommandReloadGmTickets mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GM_TICKETS (RBAC.h).
const permissionCommandReloadGmTickets uint32 = 650

// permissionCommandReloadGossipMenu mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GOSSIP_MENU (RBAC.h).
const permissionCommandReloadGossipMenu uint32 = 651

// permissionCommandReloadGossipMenuOption mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GOSSIP_MENU_OPTION (RBAC.h).
const permissionCommandReloadGossipMenuOption uint32 = 652

// permissionCommandReloadItemEnchantmentTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ITEM_ENCHANTMENT_TEMPLATE (RBAC.h).
const permissionCommandReloadItemEnchantmentTemplate uint32 = 653

// permissionCommandReloadItemLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ITEM_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadItemLootTemplate uint32 = 654

// permissionCommandReloadItemSetNames mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ITEM_SET_NAMES (RBAC.h).
const permissionCommandReloadItemSetNames uint32 = 655

// permissionCommandReloadLfgDungeonRewards mirrors rbac::RBAC_PERM_COMMAND_RELOAD_LFG_DUNGEON_REWARDS (RBAC.h).
const permissionCommandReloadLfgDungeonRewards uint32 = 656

// permissionCommandReloadAchievementRewardLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ACHIEVEMENT_REWARD_LOCALE (RBAC.h).
const permissionCommandReloadAchievementRewardLocale uint32 = 657

// permissionCommandReloadCretureTemplateLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CRETURE_TEMPLATE_LOCALE (RBAC.h).
const permissionCommandReloadCretureTemplateLocale uint32 = 658

// permissionCommandReloadCretureTextLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CRETURE_TEXT_LOCALE (RBAC.h).
const permissionCommandReloadCretureTextLocale uint32 = 659

// permissionCommandReloadGameobjectTemplateLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GAMEOBJECT_TEMPLATE_LOCALE (RBAC.h).
const permissionCommandReloadGameobjectTemplateLocale uint32 = 660

// permissionCommandReloadGossipMenuOptionLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_GOSSIP_MENU_OPTION_LOCALE (RBAC.h).
const permissionCommandReloadGossipMenuOptionLocale uint32 = 661

// permissionCommandReloadItemTemplateLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ITEM_TEMPLATE_LOCALE (RBAC.h).
const permissionCommandReloadItemTemplateLocale uint32 = 662

// permissionCommandReloadItemSetNameLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_ITEM_SET_NAME_LOCALE (RBAC.h).
const permissionCommandReloadItemSetNameLocale uint32 = 663

// permissionCommandReloadNpcTextLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_NPC_TEXT_LOCALE (RBAC.h).
const permissionCommandReloadNpcTextLocale uint32 = 664

// permissionCommandReloadPageTextLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_PAGE_TEXT_LOCALE (RBAC.h).
const permissionCommandReloadPageTextLocale uint32 = 665

// permissionCommandReloadPointsOfInterestLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_POINTS_OF_INTEREST_LOCALE (RBAC.h).
const permissionCommandReloadPointsOfInterestLocale uint32 = 666

// permissionCommandReloadQuestTemplateLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_QUEST_TEMPLATE_LOCALE (RBAC.h).
const permissionCommandReloadQuestTemplateLocale uint32 = 667

// permissionCommandReloadMailLevelReward mirrors rbac::RBAC_PERM_COMMAND_RELOAD_MAIL_LEVEL_REWARD (RBAC.h).
const permissionCommandReloadMailLevelReward uint32 = 668

// permissionCommandReloadMailLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_MAIL_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadMailLootTemplate uint32 = 669

// permissionCommandReloadMillingLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_MILLING_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadMillingLootTemplate uint32 = 670

// permissionCommandReloadNpcSpellclickSpells mirrors rbac::RBAC_PERM_COMMAND_RELOAD_NPC_SPELLCLICK_SPELLS (RBAC.h).
const permissionCommandReloadNpcSpellclickSpells uint32 = 671

// permissionCommandReloadTrainer mirrors rbac::RBAC_PERM_COMMAND_RELOAD_TRAINER (RBAC.h).
const permissionCommandReloadTrainer uint32 = 672

// permissionCommandReloadNpcVendor mirrors rbac::RBAC_PERM_COMMAND_RELOAD_NPC_VENDOR (RBAC.h).
const permissionCommandReloadNpcVendor uint32 = 673

// permissionCommandReloadPageText mirrors rbac::RBAC_PERM_COMMAND_RELOAD_PAGE_TEXT (RBAC.h).
const permissionCommandReloadPageText uint32 = 674

// permissionCommandReloadPickpocketingLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_PICKPOCKETING_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadPickpocketingLootTemplate uint32 = 675

// permissionCommandReloadPointsOfInterest mirrors rbac::RBAC_PERM_COMMAND_RELOAD_POINTS_OF_INTEREST (RBAC.h).
const permissionCommandReloadPointsOfInterest uint32 = 676

// permissionCommandReloadProspectingLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_PROSPECTING_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadProspectingLootTemplate uint32 = 677

// permissionCommandReloadQuestPoi mirrors rbac::RBAC_PERM_COMMAND_RELOAD_QUEST_POI (RBAC.h).
const permissionCommandReloadQuestPoi uint32 = 678

// permissionCommandReloadQuestTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_QUEST_TEMPLATE (RBAC.h).
const permissionCommandReloadQuestTemplate uint32 = 679

// permissionCommandReloadRbac mirrors rbac::RBAC_PERM_COMMAND_RELOAD_RBAC (RBAC.h).
const permissionCommandReloadRbac uint32 = 680

// permissionCommandReloadReferenceLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_REFERENCE_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadReferenceLootTemplate uint32 = 681

// permissionCommandReloadReservedName mirrors rbac::RBAC_PERM_COMMAND_RELOAD_RESERVED_NAME (RBAC.h).
const permissionCommandReloadReservedName uint32 = 682

// permissionCommandReloadReputationRewardRate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_REPUTATION_REWARD_RATE (RBAC.h).
const permissionCommandReloadReputationRewardRate uint32 = 683

// permissionCommandReloadSpilloverTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPILLOVER_TEMPLATE (RBAC.h).
const permissionCommandReloadSpilloverTemplate uint32 = 684

// permissionCommandReloadSkillDiscoveryTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SKILL_DISCOVERY_TEMPLATE (RBAC.h).
const permissionCommandReloadSkillDiscoveryTemplate uint32 = 685

// permissionCommandReloadSkillExtraItemTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SKILL_EXTRA_ITEM_TEMPLATE (RBAC.h).
const permissionCommandReloadSkillExtraItemTemplate uint32 = 686

// permissionCommandReloadSkillFishingBaseLevel mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SKILL_FISHING_BASE_LEVEL (RBAC.h).
const permissionCommandReloadSkillFishingBaseLevel uint32 = 687

// permissionCommandReloadSkinningLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SKINNING_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadSkinningLootTemplate uint32 = 688

// permissionCommandReloadSmartScripts mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SMART_SCRIPTS (RBAC.h).
const permissionCommandReloadSmartScripts uint32 = 689

// permissionCommandReloadSpellRequired mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_REQUIRED (RBAC.h).
const permissionCommandReloadSpellRequired uint32 = 690

// permissionCommandReloadSpellArea mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_AREA (RBAC.h).
const permissionCommandReloadSpellArea uint32 = 691

// permissionCommandReloadSpellBonusData mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_BONUS_DATA (RBAC.h).
const permissionCommandReloadSpellBonusData uint32 = 692

// permissionCommandReloadSpellGroup mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_GROUP (RBAC.h).
const permissionCommandReloadSpellGroup uint32 = 693

// permissionCommandReloadSpellLearnSpell mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_LEARN_SPELL (RBAC.h).
const permissionCommandReloadSpellLearnSpell uint32 = 694

// permissionCommandReloadSpellLootTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_LOOT_TEMPLATE (RBAC.h).
const permissionCommandReloadSpellLootTemplate uint32 = 695

// permissionCommandReloadSpellLinkedSpell mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_LINKED_SPELL (RBAC.h).
const permissionCommandReloadSpellLinkedSpell uint32 = 696

// permissionCommandReloadSpellPetAuras mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_PET_AURAS (RBAC.h).
const permissionCommandReloadSpellPetAuras uint32 = 697

// permissionCommandReloadSpellProc mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_PROC (RBAC.h).
const permissionCommandReloadSpellProc uint32 = 699

// permissionCommandReloadSpellScripts mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_SCRIPTS (RBAC.h).
const permissionCommandReloadSpellScripts uint32 = 700

// permissionCommandReloadSpellTargetPosition mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_TARGET_POSITION (RBAC.h).
const permissionCommandReloadSpellTargetPosition uint32 = 701

// permissionCommandReloadSpellThreats mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_THREATS (RBAC.h).
const permissionCommandReloadSpellThreats uint32 = 702

// permissionCommandReloadSpellGroupStackRules mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SPELL_GROUP_STACK_RULES (RBAC.h).
const permissionCommandReloadSpellGroupStackRules uint32 = 703

// permissionCommandReloadTrinityString mirrors rbac::RBAC_PERM_COMMAND_RELOAD_TRINITY_STRING (RBAC.h).
const permissionCommandReloadTrinityString uint32 = 704

// permissionCommandReloadWaypointScripts mirrors rbac::RBAC_PERM_COMMAND_RELOAD_WAYPOINT_SCRIPTS (RBAC.h).
const permissionCommandReloadWaypointScripts uint32 = 706

// permissionCommandReloadWaypointData mirrors rbac::RBAC_PERM_COMMAND_RELOAD_WAYPOINT_DATA (RBAC.h).
const permissionCommandReloadWaypointData uint32 = 707

// permissionCommandReloadVehicleAccesory mirrors rbac::RBAC_PERM_COMMAND_RELOAD_VEHICLE_ACCESORY (RBAC.h).
const permissionCommandReloadVehicleAccesory uint32 = 708

// permissionCommandReloadVehicleTemplateAccessory mirrors rbac::RBAC_PERM_COMMAND_RELOAD_VEHICLE_TEMPLATE_ACCESSORY (RBAC.h).
const permissionCommandReloadVehicleTemplateAccessory uint32 = 709

// permissionCommandReloadCharacterTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CHARACTER_TEMPLATE (RBAC.h).
const permissionCommandReloadCharacterTemplate uint32 = 842

// permissionCommandReloadQuestGreeting mirrors rbac::RBAC_PERM_COMMAND_RELOAD_QUEST_GREETING (RBAC.h).
const permissionCommandReloadQuestGreeting uint32 = 843

// permissionCommandReloadSceneTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_SCENE_TEMPLATE (RBAC.h).
const permissionCommandReloadSceneTemplate uint32 = 850

// permissionCommandReloadAreatriggerTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_AREATRIGGER_TEMPLATE (RBAC.h).
const permissionCommandReloadAreatriggerTemplate uint32 = 851

// permissionCommandReloadConversationTemplate mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CONVERSATION_TEMPLATE (RBAC.h).
const permissionCommandReloadConversationTemplate uint32 = 853

// permissionCommandReloadQuestGreetingLocale mirrors rbac::RBAC_PERM_COMMAND_RELOAD_QUEST_GREETING_LOCALE (RBAC.h).
const permissionCommandReloadQuestGreetingLocale uint32 = 867

// permissionCommandReloadCreatureMovementOverride mirrors rbac::RBAC_PERM_COMMAND_RELOAD_CREATURE_MOVEMENT_OVERRIDE (RBAC.h).
const permissionCommandReloadCreatureMovementOverride uint32 = 873
