package world

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/crypto"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

func TestAuthSessionAndPing(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"CREATE TABLE account (id INTEGER PRIMARY KEY, username TEXT NOT NULL, session_key_auth BLOB, last_ip TEXT, locked INTEGER, lock_country TEXT, os TEXT, online INTEGER NOT NULL DEFAULT 0)",
		"CREATE TABLE account_banned (id INTEGER NOT NULL, bandate INTEGER NOT NULL, unbandate INTEGER NOT NULL, active INTEGER NOT NULL)",
		"CREATE TABLE account_access (AccountID INTEGER NOT NULL, SecurityLevel INTEGER NOT NULL, RealmID INTEGER NOT NULL)",
		"CREATE TABLE rbac_account_permissions (accountId INTEGER, permissionId INTEGER, granted INTEGER, realmId INTEGER)",
		"CREATE TABLE rbac_default_permissions (secId INTEGER, permissionId INTEGER, realmId INTEGER)",
		"CREATE TABLE rbac_linked_permissions (id INTEGER, linkedId INTEGER)",
		"CREATE TABLE character_banned (guid INTEGER NOT NULL, active INTEGER NOT NULL)",
		"CREATE TABLE character_pet (owner INTEGER NOT NULL, slot INTEGER NOT NULL, entry INTEGER, modelid INTEGER, level INTEGER)",
		"CREATE TABLE character_spell (guid INTEGER NOT NULL, spell INTEGER NOT NULL, active INTEGER NOT NULL, disabled INTEGER NOT NULL)",
		"CREATE TABLE guild_member (guid INTEGER NOT NULL, guildid INTEGER NOT NULL)",
		"CREATE TABLE creature_template (entry INTEGER PRIMARY KEY, name TEXT NOT NULL, subname TEXT NOT NULL DEFAULT '', IconName TEXT NOT NULL DEFAULT '', type_flags INTEGER NOT NULL DEFAULT 0, type INTEGER NOT NULL DEFAULT 0, family INTEGER NOT NULL DEFAULT 0, rank INTEGER NOT NULL DEFAULT 0, KillCredit1 INTEGER NOT NULL DEFAULT 0, KillCredit2 INTEGER NOT NULL DEFAULT 0, modelid1 INTEGER NOT NULL DEFAULT 0, modelid2 INTEGER NOT NULL DEFAULT 0, modelid3 INTEGER NOT NULL DEFAULT 0, modelid4 INTEGER NOT NULL DEFAULT 0, HealthModifier REAL NOT NULL DEFAULT 1, ManaModifier REAL NOT NULL DEFAULT 1, RacialLeader INTEGER NOT NULL DEFAULT 0, movementId INTEGER NOT NULL DEFAULT 0)",
		"INSERT INTO creature_template (entry, name, modelid1) VALUES (68, 'Stormwind Guard', 3167)",
		"CREATE TABLE characters (guid INTEGER PRIMARY KEY, account INTEGER NOT NULL, name TEXT NOT NULL, race INTEGER NOT NULL, class INTEGER NOT NULL, gender INTEGER NOT NULL, skin INTEGER NOT NULL, face INTEGER NOT NULL, hairStyle INTEGER NOT NULL, hairColor INTEGER NOT NULL, facialStyle INTEGER NOT NULL, level INTEGER NOT NULL, zone INTEGER NOT NULL, map INTEGER NOT NULL, position_x REAL NOT NULL, position_y REAL NOT NULL, position_z REAL NOT NULL, orientation REAL NOT NULL, playerFlags INTEGER NOT NULL, extra_flags INTEGER NOT NULL DEFAULT 0, at_login INTEGER NOT NULL, cinematic INTEGER NOT NULL DEFAULT 0, equipmentCache TEXT, deleteInfos_Name TEXT, online INTEGER NOT NULL DEFAULT 0, death_expire_time INTEGER NOT NULL DEFAULT 0)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	key := bytes.Repeat([]byte{0x42}, crypto.SRP6SessionKeyLength)
	if _, err := db.Exec("INSERT INTO account (id, username, session_key_auth, last_ip, locked, lock_country, os) VALUES (7, 'test', ?, '127.0.0.1', 0, '00', 'Win')", key); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO characters (guid, account, name, race, class, gender, skin, face, hairStyle, hairColor, facialStyle, level, zone, map, position_x, position_y, position_z, orientation, playerFlags, at_login, equipmentCache) VALUES (99, 7, 'Tester', 1, 1, 0, 0, 0, 0, 0, 0, 1, 12, 0, 1.5, 2.5, 3.5, 0.5, 0, 32, '')"); err != nil {
		t.Fatal(err)
	}
	store := &database.Store{Name: "world", Backend: database.BackendSQLite, DB: db}
	stores := &database.Set{Auth: store, Characters: store, World: store}
	server := NewServer(stores, slog.New(slog.NewTextHandler(io.Discard, nil)), 1)
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go server.Handle(context.Background(), serverConn)
	challengeHeader := make([]byte, 4)
	if _, err := io.ReadFull(clientConn, challengeHeader); err != nil {
		t.Fatal(err)
	}
	challengeSize := int(binary.BigEndian.Uint16(challengeHeader[:2])) - 2
	challengePayload := make([]byte, challengeSize)
	if _, err := io.ReadFull(clientConn, challengePayload); err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint16(challengeHeader[2:]) != opcodeAuthChallenge || len(challengePayload) != 40 {
		t.Fatalf("challenge header=%x payload=%d", challengeHeader, len(challengePayload))
	}
	authSeed := challengePayload[4:8]
	localChallenge := []byte{1, 2, 3, 4}
	h := sha1.New()
	_, _ = h.Write([]byte("TEST"))
	_, _ = h.Write(make([]byte, 4))
	_, _ = h.Write(localChallenge)
	_, _ = h.Write(authSeed)
	_, _ = h.Write(key)
	payload := protocol.NewBuffer(96)
	payload.WriteU32(12340)
	payload.WriteU32(0)
	payload.WriteCString("TEST")
	payload.WriteU32(0)
	payload.Write(localChallenge)
	payload.WriteU32(0)
	payload.WriteU32(0)
	payload.WriteU32(1)
	payload.WriteU64(0)
	payload.Write(h.Sum(nil))
	if err := writeClientFrame(clientConn, opcodeAuthSession, payload.Bytes(), nil); err != nil {
		t.Fatal(err)
	}
	clientCrypt, err := crypto.NewClientAuthCrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	responseOpcode, responsePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if responseOpcode != opcodeAuthResponse || len(responsePayload) < 11 || responsePayload[0] != authOK || responsePayload[10] != 2 {
		t.Fatalf("auth response opcode=%x payload=%x", responseOpcode, responsePayload)
	}
	addonOpcode, addonPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if addonOpcode != uint16(protocol.OpcodeSMSG_ADDON_INFO) || !bytes.Equal(addonPayload, []byte{0, 0, 0, 0}) {
		t.Fatalf("addon info opcode=%x payload=%x", addonOpcode, addonPayload)
	}
	cacheOpcode, cachePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if cacheOpcode != uint16(protocol.OpcodeSMSG_CLIENTCACHE_VERSION) || !bytes.Equal(cachePayload, []byte{0, 0, 0, 0}) {
		t.Fatalf("client cache opcode=%x payload=%x", cacheOpcode, cachePayload)
	}
	tutorialOpcode, tutorialPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if tutorialOpcode != uint16(protocol.OpcodeSMSG_TUTORIAL_FLAGS) || len(tutorialPayload) != 32 {
		t.Fatalf("tutorial opcode=%x payload=%d", tutorialOpcode, len(tutorialPayload))
	}
	ping := protocol.NewBuffer(8)
	ping.WriteU32(123)
	ping.WriteU32(45)
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_PING), ping.Bytes(), clientCrypt); err != nil {
		t.Fatal(err)
	}
	pongOpcode, pongPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if pongOpcode != opcodePong || !bytes.Equal(pongPayload, []byte{123, 0, 0, 0}) {
		t.Fatalf("pong opcode=%x payload=%x", pongOpcode, pongPayload)
	}
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_CHAR_ENUM), nil, clientCrypt); err != nil {
		t.Fatal(err)
	}
	charOpcode, charPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if charOpcode != uint16(protocol.OpcodeSMSG_CHAR_ENUM) {
		t.Fatalf("character opcode=%x", charOpcode)
	}
	characters := protocol.NewReader(charPayload)
	count, err := characters.ReadU8()
	if err != nil || count != 1 {
		t.Fatalf("character count=%d err=%v", count, err)
	}
	guid, err := characters.ReadU64()
	if err != nil || guid != 99 {
		t.Fatalf("character guid=%d err=%v", guid, err)
	}
	if name, err := characters.ReadCString(); err != nil || name != "Tester" {
		t.Fatalf("character name=%q err=%v", name, err)
	}
	loginPayload := protocol.NewBuffer(8)
	loginPayload.WriteU64(99)
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), loginPayload.Bytes(), clientCrypt); err != nil {
		t.Fatal(err)
	}
	dungeonOpcode, dungeonPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if dungeonOpcode != uint16(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY) || len(dungeonPayload) != 12 {
		t.Fatalf("dungeon difficulty opcode=%x payload=%d", dungeonOpcode, len(dungeonPayload))
	}
	verifyOpcode, verifyPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if verifyOpcode != uint16(protocol.OpcodeSMSG_LOGIN_VERIFY_WORLD) || len(verifyPayload) != 20 {
		t.Fatalf("verify world opcode=%x payload=%d", verifyOpcode, len(verifyPayload))
	}
	accountDataOpcode, accountDataPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if accountDataOpcode != uint16(protocol.OpcodeSMSG_ACCOUNT_DATA_TIMES) || len(accountDataPayload) != 29 {
		t.Fatalf("account data opcode=%x payload=%d", accountDataOpcode, len(accountDataPayload))
	}
	featureOpcode, featurePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if featureOpcode != uint16(protocol.OpcodeSMSG_FEATURE_SYSTEM_STATUS) || !bytes.Equal(featurePayload, []byte{2, 0}) {
		t.Fatalf("feature opcode=%x payload=%x", featureOpcode, featurePayload)
	}
	motdOpcode, motdPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if motdOpcode != uint16(protocol.OpcodeSMSG_MOTD) || len(motdPayload) < 5 {
		t.Fatalf("motd opcode=%x payload=%d", motdOpcode, len(motdPayload))
	}
	danceOpcode, dancePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if danceOpcode != uint16(protocol.OpcodeSMSG_LEARNED_DANCE_MOVES) || len(dancePayload) != 8 {
		t.Fatalf("dance opcode=%x payload=%d", danceOpcode, len(dancePayload))
	}
	instanceOpcode, instancePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if instanceOpcode != uint16(protocol.OpcodeSMSG_INSTANCE_DIFFICULTY) || len(instancePayload) != 8 {
		t.Fatalf("instance difficulty opcode=%x payload=%d", instanceOpcode, len(instancePayload))
	}
	contactOpcode, _, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if contactOpcode != uint16(protocol.OpcodeSMSG_CONTACT_LIST) {
		t.Fatalf("contact list opcode=%x", contactOpcode)
	}
	bindOpcode, bindPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if bindOpcode != uint16(protocol.OpcodeSMSG_BIND_POINT_UPDATE) || len(bindPayload) != 20 {
		t.Fatalf("bind point update opcode=%x payload=%d", bindOpcode, len(bindPayload))
	}
	talentsOpcode, _, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if talentsOpcode != uint16(protocol.OpcodeSMSG_TALENTS_INFO) {
		t.Fatalf("talents opcode=%x", talentsOpcode)
	}
	initialSpellsOpcode, initialSpellsPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if initialSpellsOpcode != uint16(protocol.OpcodeSMSG_INITIAL_SPELLS) || len(initialSpellsPayload) < 5 {
		t.Fatalf("initial spells opcode=%x payload=%d", initialSpellsOpcode, len(initialSpellsPayload))
	}
	unlearnOpcode, unlearnPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if unlearnOpcode != uint16(protocol.OpcodeSMSG_SEND_UNLEARN_SPELLS) || len(unlearnPayload) != 4 {
		t.Fatalf("unlearn opcode=%x payload=%d", unlearnOpcode, len(unlearnPayload))
	}
	actionOpcode, actionPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if actionOpcode != uint16(protocol.OpcodeSMSG_ACTION_BUTTONS) || len(actionPayload) != 577 {
		t.Fatalf("action opcode=%x payload=%d", actionOpcode, len(actionPayload))
	}
	reputationOpcode, reputationPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if reputationOpcode != uint16(protocol.OpcodeSMSG_INITIALIZE_FACTIONS) || len(reputationPayload) != 644 {
		t.Fatalf("reputation opcode=%x payload=%d", reputationOpcode, len(reputationPayload))
	}
	// Reference login order sends the achievement dump right after the
	// faction state (AchievementMgr::SendAllAchievementData).
	achOpcode, achPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if achOpcode != uint16(protocol.OpcodeSMSG_ALL_ACHIEVEMENT_DATA) || len(achPayload) < 4 {
		t.Fatalf("achievements opcode=%x payload=%d", achOpcode, len(achPayload))
	}
	if got := binary.LittleEndian.Uint32(achPayload[len(achPayload)-4:]); got != 0xFFFFFFFF {
		t.Fatalf("achievement block missing -1 terminator: %x", got)
	}
	timeOpcode, timePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if timeOpcode != uint16(protocol.OpcodeSMSG_LOGIN_SET_TIME_SPEED) || len(timePayload) != 12 {
		t.Fatalf("time opcode=%x payload=%d", timeOpcode, len(timePayload))
	}
	forcedOpcode, forcedPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if forcedOpcode != uint16(protocol.OpcodeSMSG_SET_FORCED_REACTIONS) || len(forcedPayload) != 4 {
		t.Fatalf("forced reactions opcode=%x payload=%d", forcedOpcode, len(forcedPayload))
	}
	cinOpcode, cinPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if cinOpcode != uint16(protocol.OpcodeSMSG_TRIGGER_CINEMATIC) || len(cinPayload) != 4 {
		t.Fatalf("cinematic opcode=%x payload=%d", cinOpcode, len(cinPayload))
	}
	updateOpcode, updatePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if updateOpcode == uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		updatePayload, err = protocol.DecompressUpdatePayload(updatePayload)
		if err != nil {
			t.Fatal(err)
		}
	} else if updateOpcode != uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) {
		t.Fatalf("update opcode=%x", updateOpcode)
	}
	updates := protocol.NewReader(updatePayload)
	if blocks, err := updates.ReadU32(); err != nil || blocks != 1 {
		t.Fatalf("update blocks=%d err=%v", blocks, err)
	}
	if updateType, err := updates.ReadU8(); err != nil || updateType != protocol.UpdateCreateObject2 {
		t.Fatalf("update type=%d err=%v", updateType, err)
	}
	if updateGUID, err := updates.ReadPackedGUID(); err != nil || updateGUID != 99 {
		t.Fatalf("update guid=%d err=%v", updateGUID, err)
	}
	worldStateOpcode, worldStatePayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if worldStateOpcode != uint16(protocol.OpcodeSMSG_INIT_WORLD_STATES) || len(worldStatePayload) < 14 {
		t.Fatalf("world states opcode=%x payload=%d", worldStateOpcode, len(worldStatePayload))
	}
	timeSyncOpcode, timeSyncPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if timeSyncOpcode != uint16(protocol.OpcodeSMSG_TIME_SYNC_REQ) || len(timeSyncPayload) != 4 {
		t.Fatalf("time sync opcode=%x payload=%d", timeSyncOpcode, len(timeSyncPayload))
	}
	loginEffectOpcode, loginEffectPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if loginEffectOpcode != uint16(protocol.OpcodeSMSG_SPELL_GO) {
		t.Fatalf("login effect opcode=%x", loginEffectOpcode)
	}
	loginEffectReader := protocol.NewReader(loginEffectPayload)
	if _, err := loginEffectReader.ReadPackedGUID(); err != nil {
		t.Fatal(err)
	}
	if _, err := loginEffectReader.ReadPackedGUID(); err != nil {
		t.Fatal(err)
	}
	if _, err := loginEffectReader.ReadU8(); err != nil {
		t.Fatal(err)
	}
	spellID, err := loginEffectReader.ReadU32()
	if err != nil || spellID != 836 {
		t.Fatalf("login effect spell=%d err=%v", spellID, err)
	}
	questStatusOpcode, questStatusPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if questStatusOpcode != uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE) || len(questStatusPayload) != 4 {
		t.Fatalf("quest status opcode=%x payload=%d", questStatusOpcode, len(questStatusPayload))
	}
	chatOpcode, chatPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if chatOpcode != uint16(protocol.OpcodeSMSG_MESSAGECHAT) || len(chatPayload) == 0 {
		t.Fatalf("chat opcode=%x payload=%d", chatOpcode, len(chatPayload))
	}
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_TIME_SYNC_RESP), []byte{0, 0, 0, 0, 0, 0, 0, 0}, clientCrypt); err != nil {
		t.Fatal(err)
	}
	chat := protocol.NewBuffer(16)
	chat.WriteU32(chatSay)
	chat.WriteU32(7)
	chat.WriteCString("network hello")
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_MESSAGECHAT), chat.Bytes(), clientCrypt); err != nil {
		t.Fatal(err)
	}
	chatOpcode, chatPayload, err = readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if chatOpcode != uint16(protocol.OpcodeSMSG_MESSAGECHAT) || len(chatPayload) == 0 {
		t.Fatalf("network chat opcode=%x payload=%d", chatOpcode, len(chatPayload))
	}
	query := protocol.NewBuffer(16)
	query.WriteU32(68)
	query.WriteU64(creatureWorldGUID(1, 68))
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_CREATURE_QUERY), query.Bytes(), clientCrypt); err != nil {
		t.Fatal(err)
	}
	queryOpcode, queryPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if queryOpcode != uint16(protocol.OpcodeSMSG_CREATURE_QUERY_RESPONSE) {
		t.Fatalf("creature query opcode=%x", queryOpcode)
	}
	queryReader := protocol.NewReader(queryPayload)
	if entry, err := queryReader.ReadU32(); err != nil || entry != 68 {
		t.Fatalf("creature query entry=%d err=%v", entry, err)
	}
	if name, err := queryReader.ReadCString(); err != nil || name != "Stormwind Guard" {
		t.Fatalf("creature query name=%q err=%v", name, err)
	}
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST), nil, clientCrypt); err != nil {
		t.Fatal(err)
	}
	logoutOpcode, logoutPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if logoutOpcode != uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE) || len(logoutPayload) != 5 || binary.LittleEndian.Uint32(logoutPayload[:4]) != 0 || logoutPayload[4] != 0 {
		t.Fatalf("logout response opcode=%x payload=%x", logoutOpcode, logoutPayload)
	}
	rootOpcode, _, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if rootOpcode != uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT) {
		t.Fatalf("expected SMSG_FORCE_MOVE_ROOT (%x), got %x", uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT), rootOpcode)
	}
	updateOpcode, _, err = readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if updateOpcode != uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) && updateOpcode != uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		t.Fatalf("expected SMSG_UPDATE_OBJECT (%x) or SMSG_COMPRESSED_UPDATE_OBJECT (%x), got %x", uint16(protocol.OpcodeSMSG_UPDATE_OBJECT), uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT), updateOpcode)
	}
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_LOGOUT_CANCEL), nil, clientCrypt); err != nil {
		t.Fatal(err)
	}
	unrootOpcode, _, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if unrootOpcode != uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT) {
		t.Fatalf("expected SMSG_FORCE_MOVE_UNROOT (%x), got %x", uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT), unrootOpcode)
	}
	updateCancelOpcode, _, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if updateCancelOpcode != uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) && updateCancelOpcode != uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		t.Fatalf("expected SMSG_UPDATE_OBJECT (%x) or SMSG_COMPRESSED_UPDATE_OBJECT (%x), got %x", uint16(protocol.OpcodeSMSG_UPDATE_OBJECT), uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT), updateCancelOpcode)
	}
	cancelOpcode, cancelPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if cancelOpcode != uint16(protocol.OpcodeSMSG_LOGOUT_CANCEL_ACK) || len(cancelPayload) != 0 {
		t.Fatalf("logout cancel opcode=%x payload=%x", cancelOpcode, cancelPayload)
	}
	if _, err := db.Exec("INSERT INTO account_access (AccountID, SecurityLevel, RealmID) VALUES (7, 0, -1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO rbac_account_permissions (accountId, permissionId, granted, realmId) VALUES (7, 1, 1, -1)"); err != nil {
		t.Fatal(err)
	}
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST), nil, clientCrypt); err != nil {
		t.Fatal(err)
	}
	instantLogoutOpcode, instantLogoutPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if instantLogoutOpcode != uint16(protocol.OpcodeSMSG_LOGOUT_RESPONSE) || len(instantLogoutPayload) != 5 || binary.LittleEndian.Uint32(instantLogoutPayload[:4]) != 0 || instantLogoutPayload[4] != 1 {
		t.Fatalf("instant logout response opcode=%x payload=%x", instantLogoutOpcode, instantLogoutPayload)
	}
	for {
		completeOpcode, completePayload, readErr := readServerFrame(clientConn, clientCrypt)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if completeOpcode != uint16(protocol.OpcodeSMSG_LOGOUT_COMPLETE) {
			continue
		}
		if len(completePayload) != 0 {
			t.Fatalf("logout complete payload=%x", completePayload)
		}
		break
	}
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_CHAR_ENUM), nil, clientCrypt); err != nil {
		t.Fatal(err)
	}
	reloginEnumOpcode, reloginEnumPayload, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if reloginEnumOpcode != uint16(protocol.OpcodeSMSG_CHAR_ENUM) || len(reloginEnumPayload) < 1 {
		t.Fatalf("same-session char enum opcode=%x payload=%d", reloginEnumOpcode, len(reloginEnumPayload))
	}
	deletePayload := protocol.NewBuffer(8)
	deletePayload.WriteU64(99)
	if err := writeClientFrame(clientConn, uint32(protocol.OpcodeCMSG_CHAR_DELETE), deletePayload.Bytes(), clientCrypt); err != nil {
		t.Fatal(err)
	}
	deleteOpcode, deleteResponse, err := readServerFrame(clientConn, clientCrypt)
	if err != nil {
		t.Fatal(err)
	}
	if deleteOpcode != uint16(protocol.OpcodeSMSG_CHAR_DELETE) || !bytes.Equal(deleteResponse, []byte{71}) {
		t.Fatalf("delete opcode=%x response=%x", deleteOpcode, deleteResponse)
	}
}

func TestHandleNullOpcodes(t *testing.T) {
	srv := &Server{}
	sess := &session{server: srv, authed: true, playerLoaded: true, player: &playerState{GUID: 1}}

	// Verify sample Handle_NULL opcodes are accepted without error
	nullOpcodes := []protocol.Opcode{
		protocol.OpcodeCMSG_ACTIVE_PVP_CHEAT,
		protocol.OpcodeCMSG_ARENA_TEAM_CREATE,
		protocol.OpcodeCMSG_BOT_DETECTED,
		protocol.OpcodeCMSG_CHANNEL_SILENCE_ALL,
		protocol.OpcodeCMSG_GODMODE,
		protocol.OpcodeCMSG_PETGODMODE,
		protocol.OpcodeCMSG_BEASTMASTER,
		protocol.OpcodeCMSG_COOLDOWN_CHEAT,
		protocol.OpcodeMSG_MOVE_FEATHER_FALL,
	}

	for _, op := range nullOpcodes {
		name := opcodeName(uint32(op))
		if name == "" {
			t.Fatalf("expected opcode name for %v", op)
		}
		if !sess.authed {
			t.Fatalf("session should be authed")
		}
	}
}

func writeClientFrame(w io.Writer, opcode uint32, payload []byte, crypt interface{ EncryptSend([]byte) error }) error {
	size := len(payload) + 4
	header := make([]byte, 6)
	binary.BigEndian.PutUint16(header[:2], uint16(size))
	binary.LittleEndian.PutUint32(header[2:], opcode)
	if crypt != nil {
		if err := crypt.EncryptSend(header); err != nil {
			return err
		}
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}

func readServerFrame(r io.Reader, crypt interface{ DecryptRecv([]byte) error }) (uint16, []byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	if crypt != nil {
		if err := crypt.DecryptRecv(header); err != nil {
			return 0, nil, err
		}
	}
	size := int(binary.BigEndian.Uint16(header[:2]))
	payload := make([]byte, size-2)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return binary.LittleEndian.Uint16(header[2:]), payload, nil
}
