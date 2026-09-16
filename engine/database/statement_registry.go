package database

import (
	"context"
	"database/sql"
	"fmt"
)

type StatementRegistry struct {
	definitions map[StatementID]StatementDefinition
}

func NewStatementRegistry(definitions []StatementDefinition) (*StatementRegistry, error) {
	registry := &StatementRegistry{definitions: make(map[StatementID]StatementDefinition, len(definitions))}
	for _, definition := range definitions {
		if definition.ID == "" || definition.SQL == "" {
			return nil, fmt.Errorf("statement definition is incomplete")
		}
		if _, exists := registry.definitions[definition.ID]; exists {
			return nil, fmt.Errorf("duplicate statement %s", definition.ID)
		}
		registry.definitions[definition.ID] = definition
	}
	return registry, nil
}

func (r *StatementRegistry) Get(id StatementID) (StatementDefinition, bool) {
	if r == nil {
		return StatementDefinition{}, false
	}
	definition, ok := r.definitions[id]
	return definition, ok
}

func (r *StatementRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.definitions)
}

var sqliteStatementOverrides = map[StatementID]string{
	"LOGIN_DEL_EXPIRED_IP_BANS":                  "DELETE FROM ip_banned WHERE unbandate <> bandate AND unbandate <= unixepoch()",
	"LOGIN_UPD_EXPIRED_ACCOUNT_BANS":             "UPDATE account_banned SET active = 0 WHERE active = 1 AND unbandate <> bandate AND unbandate <= unixepoch()",
	"LOGIN_SEL_IP_INFO":                          "SELECT unbandate > unixepoch() OR unbandate = bandate AS banned, NULL as country FROM ip_banned WHERE ip = ?",
	"LOGIN_SEL_IP_BANNED_ALL":                    "SELECT ip, bandate, unbandate, bannedby, banreason FROM ip_banned WHERE (bandate = unbandate OR unbandate > unixepoch()) ORDER BY unbandate",
	"LOGIN_SEL_IP_BANNED_BY_IP":                  "SELECT ip, bandate, unbandate, bannedby, banreason FROM ip_banned WHERE (bandate = unbandate OR unbandate > unixepoch()) AND ip LIKE '%' || ? || '%' ORDER BY unbandate",
	"LOGIN_SEL_ACCOUNT_BANNED_BY_FILTER":         "SELECT account.id, username FROM account, account_banned WHERE account.id = account_banned.id AND active = 1 AND username LIKE '%' || ? || '%' GROUP BY account.id",
	"LOGIN_UPD_LOGONPROOF":                       "UPDATE account SET session_key_auth = ?, last_ip = ?, last_login = CURRENT_TIMESTAMP, locale = ?, failed_logins = 0, os = ? WHERE UPPER(username) = UPPER(?)",
	"LOGIN_SEL_LOGONCHALLENGE":                   "SELECT a.id, UPPER(a.username), a.locked, a.lock_country, a.last_ip, a.failed_logins, COALESCE(ab.unbandate > unixepoch() OR ab.unbandate = ab.bandate, 0), COALESCE(ab.unbandate = ab.bandate, 0), COALESCE(aa.SecurityLevel, 0), a.totp_secret, a.salt, a.verifier FROM account a LEFT JOIN account_access aa ON a.id = aa.AccountID LEFT JOIN account_banned ab ON ab.id = a.id AND ab.active = 1 WHERE UPPER(a.username) = UPPER(?)",
	"LOGIN_SEL_RECONNECTCHALLENGE":               "SELECT a.id, UPPER(a.username), a.locked, a.lock_country, a.last_ip, a.failed_logins, COALESCE(ab.unbandate > unixepoch() OR ab.unbandate = ab.bandate, 0), COALESCE(ab.unbandate = ab.bandate, 0), COALESCE(aa.SecurityLevel, 0), a.session_key_auth FROM account a LEFT JOIN account_access aa ON a.id = aa.AccountID LEFT JOIN account_banned ab ON ab.id = a.id AND ab.active = 1 WHERE UPPER(a.username) = UPPER(?) AND a.session_key_auth IS NOT NULL",
	"LOGIN_SEL_ACCOUNT_INFO_BY_NAME":             "SELECT a.id, a.session_key_auth, a.last_ip, a.locked, a.lock_country, a.expansion, a.mutetime, a.locale, a.recruiter, a.os, COALESCE(aa.SecurityLevel, 0), COALESCE(ab.unbandate > unixepoch() OR ab.unbandate = ab.bandate, 0), r.id FROM account a LEFT JOIN account_access aa ON a.id = aa.AccountID AND aa.RealmID IN (-1, ?) LEFT JOIN account_banned ab ON a.id = ab.id AND ab.active = 1 LEFT JOIN account r ON a.id = r.recruiter WHERE a.username = ? AND a.session_key_auth IS NOT NULL ORDER BY aa.RealmID DESC LIMIT 1",
	"LOGIN_SEL_REALM_CHARACTER_COUNTS":           "SELECT realmid, numchars FROM realmcharacters WHERE acctid = ?",
	"LOGIN_UPD_LAST_IP":                          "UPDATE account SET last_ip = ? WHERE UPPER(username) = UPPER(?)",
	"LOGIN_UPD_LAST_ATTEMPT_IP":                  "UPDATE account SET last_attempt_ip = ? WHERE UPPER(username) = UPPER(?)",
	"LOGIN_UPD_FAILEDLOGINS":                     "UPDATE account SET failed_logins = failed_logins + 1 WHERE UPPER(username) = UPPER(?)",
	"LOGIN_INS_IP_AUTO_BANNED":                   "INSERT INTO ip_banned (ip, bandate, unbandate, bannedby, banreason) VALUES (?, unixepoch(), unixepoch()+?, 'Trinity Auth', 'Failed login autoban')",
	"LOGIN_INS_ACCOUNT_AUTO_BANNED":              "INSERT INTO account_banned (id, bandate, unbandate, bannedby, banreason, active) VALUES (?, unixepoch(), unixepoch()+?, 'Trinity Auth', 'Failed login autoban', 1)",
	"LOGIN_INS_IP_BANNED":                        "INSERT INTO ip_banned (ip, bandate, unbandate, bannedby, banreason) VALUES (?, unixepoch(), unixepoch()+?, ?, ?)",
	"LOGIN_INS_ACCOUNT_BANNED":                   "INSERT INTO account_banned (id, bandate, unbandate, bannedby, banreason, active) VALUES (?, unixepoch(), unixepoch()+?, ?, ?, 1)",
	"LOGIN_INS_ACCOUNT":                          "INSERT INTO account(username, salt, verifier, reg_mail, email, joindate) VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP)",
	"LOGIN_INS_ALDL_IP_LOGGING":                  "INSERT INTO logs_ip_actions (account_id, character_guid, realm_id, type, ip, systemnote, unixtime, time) VALUES (?, ?, ?, ?, (SELECT last_ip FROM account WHERE id = ?), ?, unixepoch(), CURRENT_TIMESTAMP)",
	"LOGIN_INS_FACL_IP_LOGGING":                  "INSERT INTO logs_ip_actions (account_id, character_guid, realm_id, type, ip, systemnote, unixtime, time) VALUES (?, ?, ?, ?, (SELECT last_attempt_ip FROM account WHERE id = ?), ?, unixepoch(), CURRENT_TIMESTAMP)",
	"LOGIN_INS_CHAR_IP_LOGGING":                  "INSERT INTO logs_ip_actions (account_id, character_guid, realm_id, type, ip, systemnote, unixtime, time) VALUES (?, ?, ?, ?, ?, ?, unixepoch(), CURRENT_TIMESTAMP)",
	"LOGIN_INS_FALP_IP_LOGGING":                  "INSERT INTO logs_ip_actions (account_id, character_guid, realm_id, type, ip, systemnote, unixtime, time) VALUES (?, 0, 0, 1, ?, ?, unixepoch(), CURRENT_TIMESTAMP)",
	"LOGIN_INS_ACCOUNT_MUTE":                     "INSERT INTO account_muted VALUES (?, unixepoch(), ?, ?, ?)",
	"LOGIN_INS_RBAC_ACCOUNT_PERMISSION":          "INSERT INTO rbac_account_permissions (accountId, permissionId, granted, realmId) VALUES (?, ?, ?, ?) ON CONFLICT(accountId, permissionId, realmId) DO UPDATE SET granted = excluded.granted",
	"LOGIN_SEL_PINFO":                            "SELECT a.username, aa.SecurityLevel, a.email, a.reg_mail, a.last_ip, strftime('%Y-%m-%d %H:%M:%S', a.last_login), a.mutetime, a.mutereason, a.muteby, a.failed_logins, a.locked, a.OS FROM account a LEFT JOIN account_access aa ON (a.id = aa.AccountID AND (aa.RealmID = ? OR aa.RealmID = -1)) WHERE a.id = ?",
	"CHAR_DEL_EXPIRED_BANS":                      "UPDATE character_banned SET active = 0 WHERE unbandate <= unixepoch() AND unbandate <> bandate",
	"CHAR_DEL_ITEM_BOP_TRADE":                    "DELETE FROM item_soulbound_trade_data WHERE itemGuid = ?",
	"CHAR_DEL_GUILD_MEMBER_WITHDRAW":             "DELETE FROM guild_member_withdraw",
	"CHAR_DEL_ALL_GM_TICKETS":                    "DELETE FROM gm_ticket",
	"CHAR_DEL_EXPIRED_CHAR_INSTANCE_BY_MAP_DIFF": "DELETE FROM character_instance WHERE EXISTS (SELECT 1 FROM instance WHERE instance.id = character_instance.instance AND (character_instance.extendState = 0 OR character_instance.permanent = 0) AND instance.map = ? AND instance.difficulty = ?)",
	"CHAR_DEL_GROUP_INSTANCE_BY_MAP_DIFF":        "DELETE FROM group_instance WHERE EXISTS (SELECT 1 FROM instance WHERE instance.id = group_instance.instance AND instance.map = ? AND instance.difficulty = ?)",
	"CHAR_SEL_CHAR_CREATE_INFO":                  "SELECT level, race, class FROM characters WHERE account = ? LIMIT ?",
	"CHAR_INS_CHARACTER_BAN":                     "INSERT INTO character_banned (guid, bandate, unbandate, bannedby, banreason, active) VALUES (?, unixepoch(), unixepoch()+?, ?, ?, 1)",
	"CHAR_DEL_CHARACTER_BAN":                     "DELETE FROM character_banned WHERE guid IN (SELECT guid FROM characters WHERE account = ?)",
	"CHAR_SEL_GUID_BY_NAME_FILTER":               "SELECT guid, name FROM characters WHERE name LIKE '%' || ? || '%'",
	"CHAR_SEL_CHARACTER_SPELLCOOLDOWNS":          "SELECT spell, item, time, categoryId, categoryEnd FROM character_spell_cooldown WHERE guid = ? AND time > unixepoch()",
	"CHAR_INS_AUCTION_BIDDERS":                   "INSERT OR IGNORE INTO auctionbidders (id, bidderguid) VALUES (?, ?)",
	"CHAR_INS_CHAR_QUESTSTATUS_REWARDED":         "INSERT OR IGNORE INTO character_queststatus_rewarded (guid, quest, active) VALUES (?, ?, 1)",
	"CHAR_SEL_PET_SPELL_COOLDOWN":                "SELECT spell, time, categoryId, categoryEnd FROM pet_spell_cooldown WHERE guid = ? AND time > unixepoch()",
	"CHAR_INS_GUILD_BANK_RIGHT":                  "INSERT INTO guild_bank_right (guildid, TabId, rid, gbright, SlotPerDay) VALUES (?, ?, ?, ?, ?) ON CONFLICT(guildid, TabId, rid) DO UPDATE SET gbright = excluded.gbright, SlotPerDay = excluded.SlotPerDay",
	"CHAR_INS_GUILD_MEMBER_WITHDRAW":             "INSERT INTO guild_member_withdraw (guid, tab0, tab1, tab2, tab3, tab4, tab5, money) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(guid) DO UPDATE SET tab0 = excluded.tab0, tab1 = excluded.tab1, tab2 = excluded.tab2, tab3 = excluded.tab3, tab4 = excluded.tab4, tab5 = excluded.tab5, money = excluded.money",
	"CHAR_UPD_CHANNEL":                           "INSERT INTO channels (name, team, announce, ownership, password, bannedList, lastUsed) VALUES (?, ?, ?, ?, ?, ?, unixepoch()) ON CONFLICT(name, team) DO UPDATE SET announce = excluded.announce, ownership = excluded.ownership, password = excluded.password, bannedList = excluded.bannedList, lastUsed = excluded.lastUsed",
	"CHAR_UPD_CHANNEL_USAGE":                     "UPDATE channels SET lastUsed = unixepoch() WHERE name = ? AND team = ?",
	"CHAR_DEL_OLD_CHANNELS":                      "DELETE FROM channels WHERE ownership = 1 AND lastUsed + ? < unixepoch()",
	"CHAR_INS_GM_SURVEY":                         "INSERT INTO gm_survey (guid, surveyId, mainSurvey, comment, createTime) VALUES (?, ?, ?, ?, unixepoch())",
	"CHAR_UPD_DELETE_INFO":                       "UPDATE characters SET deleteInfos_Name = name, deleteInfos_Account = account, deleteDate = unixepoch(), name = '', account = 0 WHERE guid = ?",
	"CHAR_SEL_CHAR_DEL_INFO_BY_NAME":             "SELECT guid, deleteInfos_Name, deleteInfos_Account, deleteDate FROM characters WHERE deleteDate IS NOT NULL AND deleteInfos_Name LIKE '%' || ? || '%'",
	"CHAR_INS_DESERTER_TRACK":                    "INSERT INTO battleground_deserters (guid, type, datetime) VALUES (?, ?, CURRENT_TIMESTAMP)",
	"CHAR_INS_PVPSTATS_BATTLEGROUND":             "INSERT INTO pvpstats_battlegrounds (id, winner_faction, bracket_id, type, date) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)",
	"CHAR_SEL_PVPSTATS_FACTIONS_OVERALL":         "SELECT winner_faction, COUNT(*) AS count FROM pvpstats_battlegrounds WHERE julianday('now') - julianday(date) < 7 GROUP BY winner_faction ORDER BY winner_faction ASC",
	"CHAR_INS_QUEST_TRACK":                       "INSERT INTO quest_tracker (id, character_guid, quest_accept_time, core_hash, core_revision) VALUES (?, ?, CURRENT_TIMESTAMP, ?, ?)",
	"CHAR_UPD_CHAR_INVENTORY_FACTION_CHANGE":     "UPDATE item_instance SET itemEntry = ? WHERE itemEntry = ? AND guid IN (SELECT item FROM character_inventory WHERE guid = ?)",
	"CHAR_UPD_EXPIRE_CHAR_INSTANCE_BY_MAP_DIFF":  "UPDATE character_instance SET extendState = extendState - 1 WHERE EXISTS (SELECT 1 FROM instance WHERE instance.id = character_instance.instance AND instance.map = ? AND instance.difficulty = ?)",
	"CHAR_UPD_QUEST_TRACK_GM_COMPLETE":           "UPDATE quest_tracker SET completed_by_gm = 1 WHERE rowid IN (SELECT rowid FROM quest_tracker WHERE id = ? AND character_guid = ? ORDER BY quest_accept_time DESC LIMIT 1)",
	"CHAR_UPD_QUEST_TRACK_COMPLETE_TIME":         "UPDATE quest_tracker SET quest_complete_time = CURRENT_TIMESTAMP WHERE rowid IN (SELECT rowid FROM quest_tracker WHERE id = ? AND character_guid = ? ORDER BY quest_accept_time DESC LIMIT 1)",
	"CHAR_UPD_QUEST_TRACK_ABANDON_TIME":          "UPDATE quest_tracker SET quest_abandon_time = CURRENT_TIMESTAMP WHERE rowid IN (SELECT rowid FROM quest_tracker WHERE id = ? AND character_guid = ? ORDER BY quest_accept_time DESC LIMIT 1)",
	"CHAR_SEL_CHECK_NAME":                        "SELECT 1 FROM characters WHERE UPPER(name) = UPPER(?)",
}

func StatementSQL(id StatementID, backend Backend) (string, error) {
	for _, definition := range AllStatements() {
		if definition.ID != id {
			continue
		}
		if backend == BackendSQLite {
			if override, ok := sqliteStatementOverrides[id]; ok {
				return override, nil
			}
		}
		return definition.SQL, nil
	}
	return "", fmt.Errorf("unknown statement %s", id)
}

func (s *Store) QueryRowStatement(ctx context.Context, id StatementID, args ...any) (*sql.Row, error) {
	query, err := StatementSQL(id, s.Backend)
	if err != nil {
		s.recordDatabaseEvent("query_row", string(id), len(args), err)
		return nil, err
	}
	s.recordDatabaseEvent("query_row", string(id), len(args), nil)
	return s.DB.QueryRowContext(ctx, query, args...), nil
}

func (s *Store) QueryStatement(ctx context.Context, id StatementID, args ...any) (*sql.Rows, error) {
	query, err := StatementSQL(id, s.Backend)
	if err != nil {
		s.recordDatabaseEvent("query", string(id), len(args), err)
		return nil, err
	}
	rows, queryErr := s.DB.QueryContext(ctx, query, args...)
	s.recordDatabaseEvent("query", string(id), len(args), queryErr)
	return rows, queryErr
}

func (s *Store) ExecStatement(ctx context.Context, id StatementID, args ...any) (sql.Result, error) {
	query, err := StatementSQL(id, s.Backend)
	if err != nil {
		s.recordDatabaseEvent("exec", string(id), len(args), err)
		return nil, err
	}
	result, execErr := s.DB.ExecContext(ctx, query, args...)
	s.recordDatabaseResult("exec", string(id), len(args), result, execErr)
	return result, execErr
}
