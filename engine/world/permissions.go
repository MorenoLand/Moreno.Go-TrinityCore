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
