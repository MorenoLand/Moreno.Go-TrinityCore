package world

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

func ReplayCharacterLogin(ctx context.Context, server *Server, guid uint64) (protocoltrace.Trace, error) {
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, false)
}

func ReplayCharacterQuestRewardTwice(ctx context.Context, server *Server, guid uint64, questID uint32) (protocoltrace.Trace, error) {
	if questID == 0 {
		return protocoltrace.Trace{}, errors.New("quest reward guard replay requires a quest ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, questID, false)
}

func ReplayCharacterPetCritter(ctx context.Context, server *Server, guid uint64) (protocoltrace.Trace, error) {
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, true)
}

func ReplayCharacterPairLogin(ctx context.Context, server *Server, firstGUID, secondGUID uint64) (protocoltrace.Trace, error) {
	if server == nil || server.CharactersStore == nil || server.CharactersStore.DB == nil || firstGUID == 0 || secondGUID == 0 || firstGUID == secondGUID {
		return protocoltrace.Trace{}, errors.New("paired login replay requires two distinct characters and a character database")
	}
	server.sessionsMu.RLock()
	activeSessions := len(server.sessions)
	server.sessionsMu.RUnlock()
	if activeSessions != 0 || server.TraceRecorder != nil {
		return protocoltrace.Trace{}, errors.New("paired login replay requires an isolated server instance")
	}
	first, err := newReplayCharacterSession(ctx, server, firstGUID)
	if err != nil {
		return protocoltrace.Trace{}, err
	}
	second, err := newReplayCharacterSession(ctx, server, secondGUID)
	if err != nil {
		return protocoltrace.Trace{}, err
	}
	if first.accountID == second.accountID {
		return protocoltrace.Trace{}, errors.New("paired login replay characters belong to the same account")
	}
	recorder := protocoltrace.NewRecorder("morenocore-in-process-paired-login-replay")
	first.traceStatePrefix = fmt.Sprintf("recipient-guid=%d", firstGUID)
	second.traceStatePrefix = fmt.Sprintf("recipient-guid=%d", secondGUID)
	server.TraceRecorder = recorder
	defer func() { server.TraceRecorder = nil }()
	server.sessionsMu.Lock()
	server.sessions[first] = struct{}{}
	server.sessions[second] = struct{}{}
	server.sessionsMu.Unlock()
	defer func() {
		server.sessionsMu.Lock()
		delete(server.sessions, first)
		delete(server.sessions, second)
		server.sessionsMu.Unlock()
	}()
	defer func() {
		if first.playerLoaded {
			_ = first.completeLogout(ctx)
		}
		if second.playerLoaded {
			_ = second.completeLogout(ctx)
		}
	}()
	login := func(sess *session) error {
		packet := protocol.NewBuffer(8)
		packet.WriteU64(sess.playerGUID)
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), packet.Bytes(), "paired-character-login")
		if !sess.handlePlayerLogin(ctx, packet.Bytes()) {
			return fmt.Errorf("character login handler rejected GUID %d", sess.playerGUID)
		}
		if !sess.playerLoaded || !sess.worldReady.Load() || sess.player == nil || sess.player.GUID != sess.playerGUID {
			return fmt.Errorf("character GUID %d did not reach the world-ready boundary", sess.playerGUID)
		}
		return nil
	}
	if server.findSessionByGUID(secondGUID) != nil {
		return recorder.Snapshot(), errors.New("second character became visible before its login began")
	}
	if err := login(first); err != nil {
		return recorder.Snapshot(), err
	}
	if server.findSessionByGUID(secondGUID) != nil {
		return recorder.Snapshot(), errors.New("second character became visible before its create update")
	}
	if err := login(second); err != nil {
		return recorder.Snapshot(), err
	}
	if server.findSessionByGUID(firstGUID) != first || server.findSessionByGUID(secondGUID) != second {
		return recorder.Snapshot(), errors.New("both mapped characters were not available after paired login")
	}
	if err := validateWorldReadyFanout(server, second); err != nil {
		return recorder.Snapshot(), err
	}
	if err := second.completeLogout(ctx); err != nil {
		return recorder.Snapshot(), fmt.Errorf("complete second character logout: %w", err)
	}
	if err := first.completeLogout(ctx); err != nil {
		return recorder.Snapshot(), fmt.Errorf("complete first character logout: %w", err)
	}
	return recorder.Snapshot(), nil
}

func ReplayCharacterLFGTeleport(ctx context.Context, server *Server, guid uint64, dungeonID uint32) (protocoltrace.Trace, error) {
	if dungeonID == 0 {
		return protocoltrace.Trace{}, errors.New("LFG teleport replay requires a dungeon ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, 0, 0, dungeonID, 0, 0, 0, false)
}

func ReplayCharacterInstanceEntry(ctx context.Context, server *Server, guid uint64, mapID, instanceID uint32) (protocoltrace.Trace, error) {
	if mapID == 0 || instanceID == 0 {
		return protocoltrace.Trace{}, errors.New("instance-entry replay requires a dungeon map and instance ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, 0, 0, 0, mapID, instanceID, 0, false)
}

func ReplayCharacterPetCooldown(ctx context.Context, server *Server, guid uint64, spellID uint32) (protocoltrace.Trace, error) {
	if spellID == 0 {
		return protocoltrace.Trace{}, errors.New("pet cooldown replay requires a spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, spellID, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, false)
}

func ReplayCharacterPetPower(ctx context.Context, server *Server, guid uint64, spellID uint32) (protocoltrace.Trace, error) {
	if spellID == 0 {
		return protocoltrace.Trace{}, errors.New("pet power replay requires a spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, spellID, 0, 0, 0, 0, 0, 0, 0, 0, 0, false)
}

func ReplayCharacterPetXP(ctx context.Context, server *Server, guid uint64, earnedXP uint32) (protocoltrace.Trace, error) {
	if earnedXP == 0 {
		return protocoltrace.Trace{}, errors.New("pet XP replay requires an XP award")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, earnedXP, 0, 0, 0, 0, 0, 0, 0, 0, false)
}

func ReplayCharacterPetFeed(ctx context.Context, server *Server, guid uint64, feedSpell uint32, foodItemGUID uint64) (protocoltrace.Trace, error) {
	if feedSpell == 0 || foodItemGUID == 0 {
		return protocoltrace.Trace{}, errors.New("pet feed replay requires a spell and item GUID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, feedSpell, foodItemGUID, 0, 0, 0, 0, 0, 0, false)
}

func ReplayCharacterPetAura(ctx context.Context, server *Server, guid uint64, ownerSpellID uint32) (protocoltrace.Trace, error) {
	if ownerSpellID == 0 {
		return protocoltrace.Trace{}, errors.New("owner pet-aura replay requires a source spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, ownerSpellID, 0, 0, 0, 0, 0, false)
}

func ReplayCharacterPetFocusAura(ctx context.Context, server *Server, guid uint64, spellID uint32) (protocoltrace.Trace, error) {
	if spellID == 0 {
		return protocoltrace.Trace{}, errors.New("pet focus-aura replay requires a DBC spell ID")
	}
	return replayCharacterLogin(ctx, server, guid, 0, 0, 0, 0, 0, 0, spellID, 0, 0, 0, 0, false)
}

func validateWorldReadyFanout(server *Server, source *session) error {
	if server == nil || source == nil || source.player == nil || server.TraceRecorder == nil || !source.worldReady.Load() {
		return errors.New("world-ready fanout replay requires a mapped source session and trace recorder")
	}
	petitionDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return fmt.Errorf("open petition replay database: %w", err)
	}
	defer petitionDB.Close()
	if _, err := petitionDB.Exec("CREATE TABLE petition (ownerguid INTEGER, petitionguid INTEGER PRIMARY KEY, name TEXT, type INTEGER)"); err != nil {
		return fmt.Errorf("create petition replay schema: %w", err)
	}
	if _, err := petitionDB.Exec("CREATE TABLE petition_sign (petitionguid INTEGER, playerguid INTEGER)"); err != nil {
		return fmt.Errorf("create petition signature replay schema: %w", err)
	}
	fanoutServer := &Server{Config: server.Config, sessions: make(map[*session]struct{}), TraceRecorder: protocoltrace.NewRecorder("morenocore-world-ready-fanout")}
	fanoutServer.CharactersStore = &database.Store{Backend: database.BackendSQLite, DB: petitionDB}
	guid := source.playerGUID ^ (uint64(1) << 63)
	if guid == 0 || guid == source.playerGUID {
		guid = source.playerGUID + 1
	}
	senderGUID := guid ^ (uint64(1) << 62)
	sourcePeer := &session{server: fanoutServer, authed: true, playerLoaded: true, playerGUID: source.playerGUID, player: &playerState{GUID: source.playerGUID, Map: source.player.Map, InstanceID: source.player.InstanceID, Name: "WorldReadyReplaySource"}}
	sourcePeer.worldReady.Store(true)
	peer := &session{server: fanoutServer, authed: true, playerLoaded: true, playerGUID: guid, player: &playerState{GUID: guid, Map: source.player.Map, InstanceID: source.player.InstanceID, Name: "WorldReadyReplayPeer"}}
	petitionPeerGUID := guid ^ (uint64(1) << 61)
	petitionPeer := &session{server: fanoutServer, authed: true, playerLoaded: true, playerGUID: petitionPeerGUID, player: &playerState{GUID: petitionPeerGUID, Map: source.player.Map + 1, InstanceID: source.player.InstanceID, Name: "WorldReadyPetitionPeer"}}
	groupID := uint64(uint32(source.playerGUID) + 1)
	if groupID == 0 {
		groupID = 1
	}
	guildID := uint32(1)
	if source.player.GuildID != 1 {
		guildID = source.player.GuildID ^ (uint32(1) << 31)
		if guildID == 0 {
			guildID = 1
		}
	}
	peer.groupID, peer.player.GuildID = groupID, guildID
	sender := &session{server: fanoutServer, authed: true, playerLoaded: true, playerGUID: senderGUID, groupID: groupID, player: &playerState{GUID: senderGUID, Map: source.player.Map, InstanceID: source.player.InstanceID, Name: "WorldReadyReplaySender", GuildID: guildID}}
	group := &groupState{ID: groupID, LeaderGUID: senderGUID, Members: []groupMember{{GUID: senderGUID, Name: sender.player.Name}, {GUID: guid, Name: peer.player.Name}}}
	petitionGUID := uint64(0x7ffffffe)
	if _, err := petitionDB.Exec("INSERT INTO petition (ownerguid, petitionguid, name, type) VALUES (?, ?, ?, ?)", int64(senderGUID), int64(petitionGUID), "WorldReadyReplayGuild", 9); err != nil {
		return fmt.Errorf("insert petition replay row: %w", err)
	}
	if _, err := petitionDB.Exec("INSERT INTO petition_sign (petitionguid, playerguid) VALUES (?, ?)", int64(petitionGUID), int64(petitionPeerGUID)); err != nil {
		return fmt.Errorf("insert petition signature replay row: %w", err)
	}
	petitionPayload := protocol.NewBuffer(20)
	petitionPayload.WriteU32(0)
	petitionPayload.WriteU64(petitionGUID)
	petitionPayload.WriteU64(petitionPeerGUID)
	fanoutServer.sessionsMu.Lock()
	fanoutServer.sessions[sourcePeer] = struct{}{}
	fanoutServer.sessions[peer] = struct{}{}
	fanoutServer.sessions[petitionPeer] = struct{}{}
	fanoutServer.sessions[sender] = struct{}{}
	fanoutServer.sessionsMu.Unlock()
	defer func() {
		fanoutServer.sessionsMu.Lock()
		delete(fanoutServer.sessions, sourcePeer)
		delete(fanoutServer.sessions, peer)
		delete(fanoutServer.sessions, petitionPeer)
		delete(fanoutServer.sessions, sender)
		fanoutServer.sessionsMu.Unlock()
	}()
	if fanoutServer.findSessionByGUID(guid) != nil || fanoutServer.findSessionByName(peer.player.Name) != nil {
		return errors.New("pre-create peer leaked through online-session lookup")
	}
	countOpcode := func(opcode protocol.Opcode) int {
		count := 0
		for _, event := range fanoutServer.TraceRecorder.Snapshot().Events {
			if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(opcode) {
				count++
			}
		}
		return count
	}
	beforeAttack, beforeGroup, beforeGuild, beforePetition := countOpcode(protocol.OpcodeSMSG_ATTACK_START), countOpcode(protocol.OpcodeSMSG_GROUP_LIST), countOpcode(protocol.OpcodeSMSG_GUILD_EVENT), countOpcode(protocol.OpcodeSMSG_PETITION_SHOW_SIGNATURES)
	fanoutServer.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), buildAttackStart(source.playerGUID, guid), sourcePeer)
	fanoutServer.broadcastGroupList(group)
	sender.broadcastGuildMemberLogin()
	sender.handleOfferPetition(context.Background(), petitionPayload.Bytes())
	if got := countOpcode(protocol.OpcodeSMSG_ATTACK_START); got != beforeAttack {
		return fmt.Errorf("world-ready fanout sent attack-start to pre-create session: count %d -> %d", beforeAttack, got)
	}
	if got := countOpcode(protocol.OpcodeSMSG_GROUP_LIST); got != beforeGroup {
		return fmt.Errorf("world-ready fanout sent group-list to pre-create session: count %d -> %d", beforeGroup, got)
	}
	if got := countOpcode(protocol.OpcodeSMSG_GUILD_EVENT); got != beforeGuild {
		return fmt.Errorf("world-ready fanout sent guild event to pre-create session: count %d -> %d", beforeGuild, got)
	}
	if got := countOpcode(protocol.OpcodeSMSG_PETITION_SHOW_SIGNATURES); got != beforePetition {
		return fmt.Errorf("world-ready fanout sent petition signatures to pre-create session: count %d -> %d", beforePetition, got)
	}
	peer.worldReady.Store(true)
	petitionPeer.worldReady.Store(true)
	if fanoutServer.findSessionByGUID(guid) != peer || fanoutServer.findSessionByName(peer.player.Name) != peer {
		return errors.New("mapped peer was not available through online-session lookup")
	}
	fanoutServer.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), buildAttackStart(source.playerGUID, guid), sourcePeer)
	fanoutServer.broadcastGroupList(group)
	sender.broadcastGuildMemberLogin()
	sender.handleOfferPetition(context.Background(), petitionPayload.Bytes())
	if got := countOpcode(protocol.OpcodeSMSG_ATTACK_START); got != beforeAttack+1 {
		return fmt.Errorf("world-ready fanout did not reach the mapped peer: attack-start count %d -> %d", beforeAttack, got)
	}
	if got := countOpcode(protocol.OpcodeSMSG_GROUP_LIST); got != beforeGroup+1 {
		return fmt.Errorf("world-ready fanout did not reach the mapped peer: group-list count %d -> %d", beforeGroup, got)
	}
	if got := countOpcode(protocol.OpcodeSMSG_GUILD_EVENT); got != beforeGuild+1 {
		return fmt.Errorf("world-ready fanout did not reach the mapped peer: guild-event count %d -> %d", beforeGuild, got)
	}
	if got := countOpcode(protocol.OpcodeSMSG_PETITION_SHOW_SIGNATURES); got != beforePetition+1 {
		return fmt.Errorf("world-ready fanout did not reach the mapped peer: petition-signature count %d -> %d", beforePetition, got)
	}
	missingPetition := protocol.NewBuffer(20)
	missingPetition.WriteU32(0)
	missingPetition.WriteU64(petitionGUID + 1)
	missingPetition.WriteU64(petitionPeerGUID)
	sender.handleOfferPetition(context.Background(), missingPetition.Bytes())
	if got := countOpcode(protocol.OpcodeSMSG_PETITION_SHOW_SIGNATURES); got != beforePetition+1 {
		return fmt.Errorf("petition replay emitted signatures for a missing petition: count %d -> %d", beforePetition+1, got)
	}
	return nil
}

func newReplayCharacterSession(ctx context.Context, server *Server, guid uint64) (*session, error) {
	var accountID int64
	if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT account FROM characters WHERE guid = ?", guid).Scan(&accountID); err != nil || accountID <= 0 || accountID > int64(^uint32(0)) {
		return nil, errors.New("login replay character was not found")
	}
	sess := &session{server: server, authed: true, accountID: uint32(accountID), playerGUID: guid, legitimate: map[uint64]struct{}{guid: {}}, characterNames: make(map[uint64]enumCharacter), auras: make(map[uint32]struct{}), auraSlots: make(map[uint32]uint8), channels: make(map[string]struct{}), scale: 1, breathTimer: -1, fatigueTimer: -1, schoolLockouts: make(map[uint32]int64)}
	if server.AuthStore != nil && server.AuthStore.DB != nil {
		var security int64
		if server.AuthStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(gmlevel), 0) FROM account_access WHERE id = ? AND RealmID IN (?, -1)", accountID, server.RealmID).Scan(&security) == nil && security > 0 && security <= 255 {
			sess.security = uint8(security)
		}
	}
	return sess, nil
}

func replayCharacterLogin(ctx context.Context, server *Server, guid uint64, petCooldownSpell, petPowerSpell, petXPAward, petFeedSpell uint32, petFoodGUID uint64, petAuraSourceSpell, petFocusAuraSpell, lfgDungeonID, instanceEntryMapID, instanceEntryID, rewardedQuestID uint32, critterPetReplay bool) (protocoltrace.Trace, error) {
	if server == nil || server.CharactersStore == nil || server.CharactersStore.DB == nil || guid == 0 {
		return protocoltrace.Trace{}, errors.New("login replay requires a server and character database")
	}
	server.sessionsMu.RLock()
	activeSessions := len(server.sessions)
	server.sessionsMu.RUnlock()
	if activeSessions != 0 || server.TraceRecorder != nil {
		return protocoltrace.Trace{}, errors.New("login replay requires an isolated server instance")
	}
	session, err := newReplayCharacterSession(ctx, server, guid)
	if err != nil {
		return protocoltrace.Trace{}, err
	}
	recorder := protocoltrace.NewRecorder("morenocore-in-process-login-replay")
	server.TraceRecorder = recorder
	defer func() { server.TraceRecorder = nil }()
	server.sessionsMu.Lock()
	server.sessions[session] = struct{}{}
	server.sessionsMu.Unlock()
	defer func() {
		server.sessionsMu.Lock()
		delete(server.sessions, session)
		server.sessionsMu.Unlock()
	}()
	packet := protocol.NewBuffer(8)
	packet.WriteU64(guid)
	recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PLAYER_LOGIN), packet.Bytes(), "isolated-character-login")
	if !session.handlePlayerLogin(ctx, packet.Bytes()) {
		return recorder.Snapshot(), errors.New("character login handler rejected the replay")
	}
	if err := validateWorldReadyFanout(server, session); err != nil {
		session.logout()
		return recorder.Snapshot(), err
	}
	if rewardedQuestID != 0 {
		if err := session.replayQuestRewardTwice(ctx, rewardedQuestID); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
	}
	if critterPetReplay {
		if err := session.validateCritterPetLoginReplay(ctx); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
	}
	if petAuraSourceSpell != 0 {
		if err := session.validateOwnerPetAuraReplay(ctx, petAuraSourceSpell); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
		session.applyAuraWithDuration(petAuraSourceSpell, 0)
		if err := session.validateOwnerPetAuraReplay(ctx, petAuraSourceSpell); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
		if err := session.replayOwnerPetAuraChange(ctx, petAuraSourceSpell); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
		session.removeAura(petAuraSourceSpell)
		if err := session.validateOwnerPetAuraRemoval(ctx, petAuraSourceSpell); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
	}
	if petFocusAuraSpell != 0 {
		if err := session.replayPetFocusAura(ctx, petFocusAuraSpell); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
	}
	if lfgDungeonID != 0 {
		if session.player == nil {
			session.logout()
			return recorder.Snapshot(), errors.New("LFG teleport replay requires an active player")
		}
		before := *session.player
		wasDungeon := false
		if server.Data != nil {
			if mapInfo, found, err := server.Data.Map(before.Map); err == nil && found {
				wasDungeon = mapInfo.IsDungeon()
			}
		}
		_, _, _, wasBattlefield := battlegroundTypeForMap(before.Map)
		wasTaxiing := before.TaxiPath != ""
		session.teleportToLFGDungeon(lfgDungeonID)
		if !wasDungeon && !wasBattlefield && !wasTaxiing && (session.bgData.JoinMap != before.Map || session.bgData.JoinX != before.X || session.bgData.JoinY != before.Y || session.bgData.JoinZ != before.Z || session.bgData.JoinO != before.Orientation) {
			session.logout()
			return recorder.Snapshot(), fmt.Errorf("LFG return point map=%d position=(%v,%v,%v,%v), want map=%d position=(%v,%v,%v,%v)", session.bgData.JoinMap, session.bgData.JoinX, session.bgData.JoinY, session.bgData.JoinZ, session.bgData.JoinO, before.Map, before.X, before.Y, before.Z, before.Orientation)
		}
	}
	if instanceEntryID != 0 {
		if session.player == nil {
			session.logout()
			return recorder.Snapshot(), errors.New("instance-entry replay requires an active player")
		}
		oldMap, oldInstanceID := session.player.Map, session.player.InstanceID
		session.player.Map, session.player.InstanceID = instanceEntryMapID, instanceEntryID
		enteredAt := time.Now()
		session.recordInstanceEnterTime(ctx, enteredAt)
		session.player.Map, session.player.InstanceID = oldMap, oldInstanceID
		if session.instanceLockTimes[instanceEntryID] != enteredAt.Unix()+3600 {
			session.logout()
			return recorder.Snapshot(), fmt.Errorf("instance-entry replay release time=%d, want %d", session.instanceLockTimes[instanceEntryID], enteredAt.Unix()+3600)
		}
	}
	if petCooldownSpell != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay requires an active pet")
		}
		motion, motionFound := session.petMotionForCast(session.player.PetGUID)
		if !motionFound || !session.petKnowsSpell(ctx, motion, petCooldownSpell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay pet motion or known spell was not restored")
		}
		spell, spellFound, spellErr := server.Data.Spell(petCooldownSpell)
		if spellErr != nil || !spellFound || spell.Attributes&spellAttributePassive != 0 || motion.GUID != session.player.PetGUID || isHarmfulSpell(spell) && !isSelfCastOnly(spell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay spell cannot target the current pet")
		}
		categoryID, _, categoryErr := session.spellCooldownCategory(petCooldownSpell)
		if categoryErr != nil || categoryID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay spell has no DBC category")
		}
		server.motionMu.Lock()
		categoryEnd := motion.SpellCategoryCooldowns[categoryID]
		server.motionMu.Unlock()
		if !categoryEnd.After(time.Now()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet category cooldown was not restored from the database")
		}
		if !petSpellPacketHasCategoryCooldown(recorder.Snapshot(), session.player.PetGUID, categoryID) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown was missing from the live SMSG_PET_SPELLS packet")
		}
		cast := protocol.NewBuffer(24)
		cast.WriteU64(session.player.PetGUID)
		cast.WriteU8(1)
		cast.WriteU32(petCooldownSpell)
		cast.WriteU8(0)
		cast.WriteU32(protocol.SpellTargetFlagUnit)
		cast.WritePackedGUID(session.player.PetGUID)
		target, targetErr := protocol.ReadSpellTargetData(protocol.NewReader(cast.Bytes()[14:]))
		if targetErr != nil || target.UnitGUID != session.player.PetGUID {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown replay target did not encode the current pet GUID")
		}
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_PET_CAST_SPELL), cast.Bytes(), "pet-category-cooldown-check")
		if !session.handlePetCastSpell(ctx, cast.Bytes()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet cooldown cast handler rejected the replay")
		}
		trace := recorder.Snapshot()
		categoryExpiryMatches := petCooldownExpiryClearsCategory(server, motion, petCooldownSpell, categoryID)
		session.logout()
		if !petCastFailedNotReady(trace, petCooldownSpell) {
			return trace, errors.New("pet cast did not return SPELL_FAILED_NOT_READY for the persisted cooldown")
		}
		if !categoryExpiryMatches {
			return trace, errors.New("expired pet spell cooldown retained its category lock")
		}
		return trace, nil
	}
	if petPowerSpell != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay requires an active pet")
		}
		motion, motionFound := session.petMotionForCast(session.player.PetGUID)
		spell, spellFound, spellErr := server.Data.Spell(petPowerSpell)
		if !motionFound || spellErr != nil || !spellFound || !session.petKnowsSpell(ctx, motion, petPowerSpell) || spell.PowerType >= 7 || spell.PowerType == petPowerTypeHealth {
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay pet motion, known spell, or supported power type was not restored")
		}
		server.motionMu.Lock()
		_, maximum, _, validPower, _ := petSpellPowerState(motion, spell.PowerType)
		server.motionMu.Unlock()
		cost := ResolvePetSpellPowerCost(spell, maximum, motion.MaxHealth)
		if !validPower || cost == 0 || cost >= maximum {
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay spell has no affordable DBC resource cost")
		}
		server.motionMu.Lock()
		originalPower := motion.Powers[spell.PowerType]
		originalMana := motion.Mana
		originalLastSpell := motion.LastSpell
		originalSpellCooldowns := make(map[uint32]time.Time, len(motion.SpellCooldowns))
		for id, value := range motion.SpellCooldowns {
			originalSpellCooldowns[id] = value
		}
		originalCategoryCooldowns := make(map[uint32]time.Time, len(motion.SpellCategoryCooldowns))
		for id, value := range motion.SpellCategoryCooldowns {
			originalCategoryCooldowns[id] = value
		}
		originalCooldownCategories := make(map[uint32]uint32, len(motion.SpellCooldownCategories))
		for id, value := range motion.SpellCooldownCategories {
			originalCooldownCategories[id] = value
		}
		originalCategoryEnds := make(map[uint32]time.Time, len(motion.SpellCooldownCategoryEnds))
		for id, value := range motion.SpellCooldownCategoryEnds {
			originalCategoryEnds[id] = value
		}
		motion.Powers[spell.PowerType] = cost - 1
		if spell.PowerType == 0 {
			motion.Mana = cost - 1
		}
		server.motionMu.Unlock()
		if session.checkPetSpellPower(motion, spell, 0) {
			server.motionMu.Lock()
			motion.Powers[spell.PowerType], motion.Mana, motion.LastSpell = originalPower, originalMana, originalLastSpell
			motion.SpellCooldowns, motion.SpellCategoryCooldowns = originalSpellCooldowns, originalCategoryCooldowns
			motion.SpellCooldownCategories, motion.SpellCooldownCategoryEnds = originalCooldownCategories, originalCategoryEnds
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay accepted a cast with insufficient resource")
		}
		server.motionMu.Lock()
		motion.Powers[spell.PowerType] = maximum
		if spell.PowerType == 0 {
			motion.Mana = maximum
		}
		server.motionMu.Unlock()
		spell.Effects = [3]wotlk.SpellEffect{}
		target := protocol.SpellTargetData{Flags: protocol.SpellTargetFlagUnit, UnitGUID: motion.GUID}
		if !session.executePetSpell(ctx, motion, spell, 0, target) {
			server.motionMu.Lock()
			motion.Powers[spell.PowerType], motion.Mana, motion.LastSpell = originalPower, originalMana, originalLastSpell
			motion.SpellCooldowns, motion.SpellCategoryCooldowns = originalSpellCooldowns, originalCategoryCooldowns
			motion.SpellCooldownCategories, motion.SpellCooldownCategoryEnds = originalCooldownCategories, originalCategoryEnds
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet power replay spell executor rejected the cast")
		}
		trace := recorder.Snapshot()
		expectedPower := maximum - cost
		server.motionMu.Lock()
		actualPower := motion.Powers[spell.PowerType]
		motion.Powers[spell.PowerType], motion.Mana, motion.LastSpell = originalPower, originalMana, originalLastSpell
		motion.SpellCooldowns, motion.SpellCategoryCooldowns = originalSpellCooldowns, originalCategoryCooldowns
		motion.SpellCooldownCategories, motion.SpellCooldownCategoryEnds = originalCooldownCategories, originalCategoryEnds
		server.motionMu.Unlock()
		session.logout()
		if actualPower != expectedPower || !petSpellGoPowerMatches(trace, petPowerSpell, motion.GUID, expectedPower) || !petCastFailedWithResult(trace, petPowerSpell, petSpellFailedNoPower) {
			return trace, fmt.Errorf("pet resource replay state=%d want=%d; spell-go power or no-power rejection mismatch", actualPower, expectedPower)
		}
		return trace, nil
	}
	if petXPAward != 0 {
		if session.player == nil || session.player.PetGUID == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet XP replay requires an active pet")
		}
		server.motionMu.Lock()
		motion := server.creatureMotion[session.player.PetGUID]
		if motion == nil || motion.PetType != 1 || motion.Health == 0 {
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet XP replay requires a living hunter pet")
		}
		petID, oldLevel, oldXP, currentNextXP := motion.PetID, motion.Level, motion.Experience, motion.PetNextLevelXP
		server.motionMu.Unlock()
		maxLevel := server.Config.MaxPlayerLevel
		if ownerLevel := uint32(session.player.Level); ownerLevel < maxLevel {
			maxLevel = ownerLevel
		}
		xpForLevel := make([]uint32, len(xpCurve))
		copy(xpForLevel, xpCurve[:])
		for level := uint32(1); level < uint32(len(xpForLevel)); level++ {
			xpForLevel[level] = server.xpForLevel(ctx, level)
		}
		expectedLevel, expectedXP, _ := AdvanceHunterPetExperience(oldLevel, oldXP, petXPAward, maxLevel, currentNextXP, xpForLevel)
		eventsBefore := len(recorder.Snapshot().Events)
		session.giveHunterPetXP(ctx, petXPAward)
		trace := recorder.Snapshot()
		server.motionMu.Lock()
		actualLevel, actualXP := motion.Level, motion.Experience
		server.motionMu.Unlock()
		updatedPet := false
		for _, event := range trace.Events[eventsBefore:] {
			if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) {
				updatedPet = true
				break
			}
		}
		session.logout()
		var savedLevel, savedXP int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT level, exp FROM character_pet WHERE id = ? AND owner = ?", petID, guid).Scan(&savedLevel, &savedXP); err != nil {
			return recorder.Snapshot(), err
		}
		if expectedLevel == oldLevel && expectedXP == oldXP || actualLevel != expectedLevel || actualXP != expectedXP || uint32(savedLevel) != expectedLevel || uint32(savedXP) != expectedXP || !updatedPet {
			return recorder.Snapshot(), fmt.Errorf("pet XP replay mismatch expected=(%d,%d) actual=(%d,%d) saved=(%d,%d) update=%t", expectedLevel, expectedXP, actualLevel, actualXP, savedLevel, savedXP, updatedPet)
		}
		return recorder.Snapshot(), nil
	}
	if petFeedSpell != 0 {
		spell, found, err := server.Data.Spell(petFeedSpell)
		if err != nil || !found || !session.hasActiveSpell(petFeedSpell) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed replay spell was not loaded and learned")
		}
		var feedEffect *wotlk.SpellEffect
		for index := range spell.Effects {
			if spell.Effects[index].Effect == 101 {
				feedEffect = &spell.Effects[index]
				break
			}
		}
		if feedEffect == nil || feedEffect.TriggerSpell == 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed replay spell has no feed effect or triggered spell")
		}
		var originalItemCount int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE((SELECT count FROM item_instance WHERE guid = ?), 0)", int64(petFoodGUID)).Scan(&originalItemCount); err != nil || originalItemCount <= 0 {
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed replay food item was not present")
		}
		beforeFeed, failure := session.checkPetFood(ctx, petFoodGUID)
		if failure != 0 {
			session.logout()
			return recorder.Snapshot(), fmt.Errorf("pet feed fixture rejected with source cast result %d", failure)
		}
		server.motionMu.Lock()
		motion := server.creatureMotion[beforeFeed.PetGUID]
		if motion == nil || motion.Health == 0 {
			server.motionMu.Unlock()
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed replay requires a living pet")
		}
		beforeHappiness := motion.Happiness
		beforePetType, beforePetID, beforePowerType := motion.PetType, motion.PetID, motion.PowerType
		beforeMaxHappiness := motion.MaxPowers[4]
		petGUID := motion.GUID
		server.motionMu.Unlock()
		cast := protocol.NewBuffer(32)
		cast.WriteU8(1)
		cast.WriteU32(petFeedSpell)
		cast.WriteU8(0)
		protocol.WriteSpellTargetData(cast, protocol.SpellTargetData{Flags: protocol.SpellTargetFlagItemWireMask, ItemGUID: petFoodGUID})
		recorder.Record(protocoltrace.ClientToServer, uint32(protocol.OpcodeCMSG_CAST_SPELL), cast.Bytes(), "pet-feed-item-target")
		if !session.handleCastSpell(ctx, cast.Bytes()) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed cast handler rejected the replay packet")
		}
		server.motionMu.Lock()
		castMotion := server.creatureMotion[petGUID]
		castPetType, castPetID, castMaxHappiness := uint8(0), uint32(0), uint32(0)
		if castMotion != nil {
			castPetType, castPetID, castMaxHappiness = castMotion.PetType, castMotion.PetID, castMotion.MaxPowers[4]
		}
		server.motionMu.Unlock()
		deadline := time.Now().Add(2 * time.Second)
		var aura *activeAura
		for aura == nil && time.Now().Before(deadline) {
			server.auraMu.Lock()
			aura = server.activeCreatureAuras[petGUID][feedEffect.TriggerSpell]
			if aura != nil && aura.TickTimer != nil {
				aura.TickTimer.Stop()
				aura.TickTimer = nil
			}
			server.auraMu.Unlock()
			if aura == nil {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if aura == nil || aura.AuraType != 24 || aura.MiscValue != 4 || aura.Amount != beforeFeed.Benefit {
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed triggered happiness aura did not match the DBC benefit")
		}
		if !session.executePeriodicTickOnCreature(aura) {
			session.logout()
			return recorder.Snapshot(), errors.New("pet feed happiness aura tick was rejected")
		}
		trace := recorder.Snapshot()
		server.motionMu.Lock()
		motion = server.creatureMotion[petGUID]
		afterHappiness := uint32(0)
		afterPower, maxHappiness := uint32(0), uint32(0)
		afterPetType, afterPetID, afterPowerType := uint8(0), uint32(0), uint32(0)
		if motion != nil {
			afterHappiness = motion.Happiness
			afterPower, maxHappiness = motion.Powers[4], motion.MaxPowers[4]
			afterPetType, afterPetID, afterPowerType = motion.PetType, motion.PetID, motion.PowerType
		}
		server.motionMu.Unlock()
		var remainingCount, remainingInventoryRows int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE((SELECT count FROM item_instance WHERE guid = ?), 0)", int64(petFoodGUID)).Scan(&remainingCount); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_inventory WHERE guid = ? AND item = ?", guid, int64(petFoodGUID)).Scan(&remainingInventoryRows); err != nil {
			session.logout()
			return recorder.Snapshot(), err
		}
		session.logout()
		var savedHappiness int64
		if err := server.CharactersStore.DB.QueryRowContext(ctx, "SELECT curhappiness FROM character_pet WHERE id = ? AND owner = ?", beforeFeed.PetID, guid).Scan(&savedHappiness); err != nil {
			return recorder.Snapshot(), err
		}
		updatedPacket := false
		spellLogPacket := false
		for _, event := range trace.Events {
			if event.Direction != protocoltrace.ServerToClient {
				continue
			}
			if event.Opcode == uint32(protocol.OpcodeSMSG_UPDATE_OBJECT) {
				updatedPacket = true
			}
			if event.Opcode == uint32(protocol.OpcodeSMSG_SPELLLOGEXECUTE) {
				spellLogPacket = true
			}
		}
		expectedRemainingCount := originalItemCount - 1
		expectedInventoryRows := int64(1)
		if expectedRemainingCount == 0 {
			expectedInventoryRows = 0
		}
		if afterHappiness != beforeHappiness+beforeFeed.Benefit || uint32(savedHappiness) != afterHappiness || remainingCount != expectedRemainingCount || remainingInventoryRows != expectedInventoryRows || !updatedPacket || !spellLogPacket {
			return trace, fmt.Errorf("pet feed replay mismatch happiness=%d->%d power=%d/%d pet-before=(type:%d id:%d power:%d max-happiness:%d) pet-after-cast=(type:%d id:%d max-happiness:%d) pet-after-tick=(type:%d id:%d power:%d) aura=(target:%d type:%d misc:%d amount:%d) saved=%d item-count=%d->%d inventory-rows=%d update=%t spell-log=%t", beforeHappiness, afterHappiness, afterPower, maxHappiness, beforePetType, beforePetID, beforePowerType, beforeMaxHappiness, castPetType, castPetID, castMaxHappiness, afterPetType, afterPetID, afterPowerType, aura.TargetGUID, aura.AuraType, aura.MiscValue, aura.Amount, savedHappiness, originalItemCount, remainingCount, remainingInventoryRows, updatedPacket, spellLogPacket)
		}
		return trace, nil
	}
	if err := session.completeLogout(ctx); err != nil {
		return recorder.Snapshot(), fmt.Errorf("complete character logout: %w", err)
	}
	return recorder.Snapshot(), nil
}

func (s *session) validateCritterPetLoginReplay(ctx context.Context) error {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.player == nil {
		return errors.New("critter pet replay requires an active login session")
	}
	var petID, entry int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT id, entry FROM character_pet WHERE owner = ? AND slot = 0", s.playerGUID).Scan(&petID, &entry); err != nil {
		return fmt.Errorf("read active critter pet replay row: %w", err)
	}
	var creatureType int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT type FROM creature_template WHERE entry = ?", entry).Scan(&creatureType); err != nil {
		return fmt.Errorf("read critter pet replay template: %w", err)
	}
	if uint32(creatureType) != creatureTypeCritter {
		return fmt.Errorf("pet %d template type=%d, want critter type %d", petID, creatureType, creatureTypeCritter)
	}
	if s.player.PetGUID != 0 || s.player.PetNumber != 0 {
		return errors.New("critter login populated controlled-pet owner fields")
	}
	for _, event := range s.server.TraceRecorder.Snapshot().Events {
		if event.Direction == protocoltrace.ServerToClient && event.Opcode == uint32(protocol.OpcodeSMSG_PET_SPELLS) {
			return errors.New("critter login sent a controlled-pet spell bar")
		}
	}
	return nil
}

func (s *session) replayQuestRewardTwice(ctx context.Context, questID uint32) error {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.server.TraceRecorder == nil || questID == 0 {
		return errors.New("quest reward replay requires an isolated logged-in session")
	}
	state, err := s.loadQuestRewardPersistenceState(ctx, questID)
	if err != nil {
		return fmt.Errorf("load quest reward persistence state: %w", err)
	}
	if !state.Rewarded {
		return fmt.Errorf("quest %d does not use a permanent rewarded status", questID)
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		return fmt.Errorf("load quest reward replay data: %w", err)
	}
	if len(view.RequiredItems) != 0 {
		return fmt.Errorf("quest %d requires items and is not suitable for the isolated reward replay", questID)
	}
	var giverEntry uint32
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature_questender WHERE quest = ? ORDER BY id LIMIT 1", questID).Scan(&giverEntry); err != nil {
		return fmt.Errorf("quest %d has no creature quest ender for replay: %w", questID, err)
	}
	if giverEntry == 0 {
		return fmt.Errorf("quest %d has a zero creature quest ender", questID)
	}
	giverGUID := uint64(giverEntry) << 24
	if _, found := s.questGiverEnder(ctx, giverGUID, questID); !found {
		return fmt.Errorf("quest %d creature ender %d did not resolve", questID, giverEntry)
	}
	complete := protocol.NewBuffer(12)
	complete.WriteU64(giverGUID)
	complete.WriteU32(questID)
	if !s.handleQuestgiverCompleteQuest(ctx, complete.Bytes()) {
		return fmt.Errorf("quest %d complete request was rejected", questID)
	}
	events := s.server.TraceRecorder.Snapshot().Events
	offerReward := false
	for _, event := range events {
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		offerReward = offerReward || event.Opcode == uint32(protocol.OpcodeSMSG_QUEST_GIVER_OFFER_REWARD_MESSAGE)
	}
	if !offerReward {
		return fmt.Errorf("completed quest %d did not offer its first reward", questID)
	}
	choose := protocol.NewBuffer(16)
	choose.WriteU64(giverGUID)
	choose.WriteU32(questID)
	choose.WriteU32(0)
	if !s.handleQuestgiverChooseReward(ctx, choose.Bytes()) {
		return fmt.Errorf("quest %d first reward choice was rejected", questID)
	}
	status, err := s.characterQuestStatus(ctx, questID)
	if err != nil || status != 0 {
		return fmt.Errorf("quest %d remained active after reward: status=%d err=%v", questID, status, err)
	}
	for _, entry := range s.player.QuestLog {
		if entry.QuestID == questID {
			return fmt.Errorf("quest %d remained in the in-memory quest log after reward", questID)
		}
	}
	if !s.isQuestRewarded(ctx, questID) {
		return fmt.Errorf("quest %d did not persist its rewarded state after the first reward", questID)
	}
	readRewardState := func() ([4]int64, error) {
		var values [4]int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT money, xp, level FROM characters WHERE guid = ?", s.playerGUID).Scan(&values[0], &values[1], &values[2]); err != nil {
			return values, err
		}
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_inventory WHERE guid = ?", s.playerGUID).Scan(&values[3]); err != nil {
			return values, err
		}
		return values, nil
	}
	firstRewardState, err := readRewardState()
	if err != nil {
		return fmt.Errorf("read first quest reward state: %w", err)
	}
	eventStart := len(s.server.TraceRecorder.Snapshot().Events)
	if !s.handleQuestgiverCompleteQuest(ctx, complete.Bytes()) {
		return fmt.Errorf("quest %d reopened complete request was rejected", questID)
	}
	events = s.server.TraceRecorder.Snapshot().Events
	requestItems, offerReward := false, false
	for _, event := range events[eventStart:] {
		if event.Direction != protocoltrace.ServerToClient {
			continue
		}
		requestItems = requestItems || event.Opcode == uint32(protocol.OpcodeSMSG_QUESTGIVER_REQUEST_ITEMS)
		offerReward = offerReward || event.Opcode == uint32(protocol.OpcodeSMSG_QUEST_GIVER_OFFER_REWARD_MESSAGE)
	}
	if !requestItems || offerReward {
		return fmt.Errorf("quest %d re-opened with a reward offer after being claimed", questID)
	}
	request := protocol.NewBuffer(12)
	request.WriteU64(giverGUID)
	request.WriteU32(questID)
	eventStart = len(events)
	if !s.handleQuestgiverRequestReward(ctx, request.Bytes()) || !s.handleQuestgiverChooseReward(ctx, choose.Bytes()) {
		return fmt.Errorf("quest %d duplicate reward request was rejected by the handler", questID)
	}
	if len(s.server.TraceRecorder.Snapshot().Events) != eventStart {
		return fmt.Errorf("quest %d duplicate reward attempt emitted reward packets", questID)
	}
	secondRewardState, err := readRewardState()
	if err != nil {
		return fmt.Errorf("read repeated quest reward state: %w", err)
	}
	if firstRewardState != secondRewardState {
		return fmt.Errorf("quest %d changed reward state twice: first=%v second=%v", questID, firstRewardState, secondRewardState)
	}
	return nil
}

func (s *session) validateOwnerPetAuraReplay(ctx context.Context, ownerSpellID uint32) error {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil || s.player.PetGUID == 0 {
		return errors.New("owner pet-aura replay requires an active pet")
	}
	ownerSpell, found, err := s.server.Data.Spell(ownerSpellID)
	if err != nil || !found {
		return errors.New("owner pet-aura replay source spell is missing from DBC")
	}
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[s.player.PetGUID]
	s.server.motionMu.Unlock()
	if motion == nil {
		return errors.New("owner pet-aura replay pet motion is missing")
	}
	validated := 0
	for index, effect := range ownerSpell.Effects {
		if effect.Effect != 3 && (effect.Effect != 6 || effect.Aura != 4) {
			continue
		}
		key := ownerPetAuraKey{SpellID: ownerSpellID, EffectIndex: uint8(index)}
		s.ownerPetAuraMu.Lock()
		source, sourceFound := s.ownerPetAuraSources[key]
		s.ownerPetAuraMu.Unlock()
		if !sourceFound {
			continue
		}
		auraSpellID := source.auraForPet(motion.Entry)
		if auraSpellID == 0 {
			continue
		}
		s.server.auraMu.Lock()
		current := s.server.activeCreatureAuras[motion.GUID][auraSpellID]
		var aura activeAura
		if current != nil {
			aura = *current
		}
		s.server.auraMu.Unlock()
		if current == nil || !aura.OwnerPetAura || aura.OwnerPetAuraSourceSpell != ownerSpellID || aura.OwnerPetAuraSourceEffect != uint8(index) {
			return fmt.Errorf("owner pet-aura replay missing derived spell %d for source %d effect %d", auraSpellID, ownerSpellID, index)
		}
		auraSpell, found, err := s.server.Data.Spell(auraSpellID)
		if err != nil || !found || aura.EffectMask != PetAuraEffectMask(auraSpell) {
			return fmt.Errorf("owner pet-aura replay spell %d has incomplete effect mask %d", auraSpellID, aura.EffectMask)
		}
		for effectIndex, effect := range auraSpell.Effects {
			if aura.EffectMask&(1<<uint(effectIndex)) == 0 {
				continue
			}
			if auraSpellID == 35696 && effectIndex == 0 {
				expected := ResolveOwnerPetAuraAmount(auraSpellID, uint8(effectIndex), effect.BasePoints, aura.OwnerPetAuraSourceDamage, motion.Stats)
				if aura.Amounts[effectIndex] != expected {
					return fmt.Errorf("owner pet-aura replay Demonic Knowledge amount=%d, want %d", aura.Amounts[effectIndex], expected)
				}
			} else {
				minValue, maxValue := effect.CalcValueRangeForLevel(auraSpell, uint32(motion.Level))
				if aura.Amounts[effectIndex] < minValue || aura.Amounts[effectIndex] > maxValue {
					return fmt.Errorf("owner pet-aura replay spell %d effect %d amount=%d, want range [%d,%d]", auraSpellID, effectIndex, aura.Amounts[effectIndex], minValue, maxValue)
				}
			}
			if aura.BaseAmounts[effectIndex] != effect.BasePoints {
				return fmt.Errorf("owner pet-aura replay spell %d effect %d base=%d, want %d", auraSpellID, effectIndex, aura.BaseAmounts[effectIndex], effect.BasePoints)
			}
		}
		validated++
	}
	if validated == 0 {
		return fmt.Errorf("owner pet-aura replay source spell %d mapped no aura to pet entry %d", ownerSpellID, motion.Entry)
	}
	return nil
}

func (s *session) validateOwnerPetAuraRemoval(ctx context.Context, ownerSpellID uint32) error {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 {
		return errors.New("owner pet-aura removal replay requires an active pet")
	}
	motion, ok := s.petMotionForCast(s.player.PetGUID)
	if !ok {
		return errors.New("owner pet-aura removal replay pet motion is missing")
	}
	spell, found, err := s.server.Data.Spell(ownerSpellID)
	if err != nil || !found {
		return errors.New("owner pet-aura removal source spell is missing from DBC")
	}
	for index, effect := range spell.Effects {
		if effect.Effect != 3 && (effect.Effect != 6 || effect.Aura != 4) {
			continue
		}
		key := ownerPetAuraKey{SpellID: ownerSpellID, EffectIndex: uint8(index)}
		s.ownerPetAuraMu.Lock()
		_, sourceStillRegistered := s.ownerPetAuraSources[key]
		s.ownerPetAuraMu.Unlock()
		if sourceStillRegistered {
			return fmt.Errorf("owner pet-aura removal left source %d effect %d registered", ownerSpellID, index)
		}
		if source, found := s.readOwnerPetAuraSource(ctx, ownerSpellID, uint8(index)); found {
			if auraSpellID := source.auraForPet(motion.Entry); auraSpellID != 0 {
				s.server.auraMu.Lock()
				aura := s.server.activeCreatureAuras[motion.GUID][auraSpellID]
				stillActive := aura != nil && aura.OwnerPetAura && aura.OwnerPetAuraSourceSpell == ownerSpellID && aura.OwnerPetAuraSourceEffect == uint8(index)
				s.server.auraMu.Unlock()
				if stillActive {
					return fmt.Errorf("owner pet-aura removal left derived spell %d active", auraSpellID)
				}
			}
		}
	}
	return nil
}

func (s *session) replayOwnerPetAuraChange(ctx context.Context, ownerSpellID uint32) error {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 {
		return errors.New("owner pet-aura change replay requires an active pet")
	}
	motion, ok := s.petMotionForCast(s.player.PetGUID)
	if !ok {
		return errors.New("owner pet-aura change replay pet motion is missing")
	}
	s.ownerPetAuraMu.Lock()
	changeSources := make(map[ownerPetAuraKey]ownerPetAuraSource)
	for key, source := range s.ownerPetAuraSources {
		if key.SpellID == ownerSpellID && source.RemoveOnChange {
			changeSources[key] = source
		}
	}
	s.ownerPetAuraMu.Unlock()
	if len(changeSources) == 0 {
		return nil
	}
	s.removeOwnerPetAuraSourcesOnPetChange(ctx, motion.GUID)
	s.applyOwnerPetAuras(ctx, motion.Entry, motion.GUID)
	for key, source := range changeSources {
		s.ownerPetAuraMu.Lock()
		_, sourceStillRegistered := s.ownerPetAuraSources[key]
		s.ownerPetAuraMu.Unlock()
		if sourceStillRegistered {
			return fmt.Errorf("owner pet-aura change retained source spell %d effect %d", key.SpellID, key.EffectIndex)
		}
		if auraSpellID := source.auraForPet(motion.Entry); auraSpellID != 0 {
			s.server.auraMu.Lock()
			aura := s.server.activeCreatureAuras[motion.GUID][auraSpellID]
			stillActive := aura != nil && aura.OwnerPetAura && aura.OwnerPetAuraSourceSpell == key.SpellID && aura.OwnerPetAuraSourceEffect == key.EffectIndex
			s.server.auraMu.Unlock()
			if stillActive {
				return fmt.Errorf("owner pet-aura change left derived spell %d active", auraSpellID)
			}
		}
	}
	return nil
}

func (s *session) replayPetFocusAura(ctx context.Context, spellID uint32) error {
	if s == nil || s.server == nil || s.player == nil || s.player.PetGUID == 0 || s.server.TraceRecorder == nil {
		return errors.New("pet focus-aura replay requires a traced login with an active pet")
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return fmt.Errorf("pet focus-aura replay spell %d is missing from DBC", spellID)
	}
	effectIndex := -1
	for index, effect := range spell.Effects {
		if effect.Effect != 0 && effect.Aura == 110 && effect.MiscValue == 2 {
			effectIndex = index
			break
		}
	}
	if effectIndex < 0 {
		return fmt.Errorf("pet focus-aura replay spell %d has no DBC Focus regeneration percent effect", spellID)
	}
	petGUID := s.player.PetGUID
	s.server.motionMu.Lock()
	motion := s.server.creatureMotion[petGUID]
	if motion == nil || motion.Health == 0 {
		s.server.motionMu.Unlock()
		return errors.New("pet focus-aura replay requires a living pet")
	}
	oldPowerType, oldPowers, oldMaxPowers := motion.PowerType, motion.Powers, motion.MaxPowers
	oldFlags, oldFocusTimer, oldRefreshed := motion.UnitFlags2, motion.FocusRegenTimer, motion.Refreshed
	motion.PowerType, motion.Powers[2], motion.MaxPowers[2] = 2, 0, 100
	motion.UnitFlags2 |= unitFlag2RegeneratePower
	motion.FocusRegenTimer = petFocusRegenInterval
	s.server.motionMu.Unlock()
	s.server.auraMu.Lock()
	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[uint64]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[petGUID] == nil {
		s.server.activeCreatureAuras[petGUID] = make(map[uint32]*activeAura)
	}
	previousAura := s.server.activeCreatureAuras[petGUID][spellID]
	effect := spell.Effects[effectIndex]
	mask := uint8(1 << uint(effectIndex))
	aura := &activeAura{SpellID: spellID, AuraType: effect.Aura, EffectMask: mask, CasterGUID: petGUID, TargetGUID: petGUID, MiscValue: effect.MiscValue, Amount: 50, Positive: true, StackCount: 1}
	aura.Amounts[effectIndex], aura.BaseAmounts[effectIndex] = 50, effect.BasePoints
	s.server.activeCreatureAuras[petGUID][spellID] = aura
	s.server.auraMu.Unlock()
	defer func() {
		s.server.motionMu.Lock()
		motion.PowerType, motion.Powers, motion.MaxPowers = oldPowerType, oldPowers, oldMaxPowers
		motion.UnitFlags2, motion.FocusRegenTimer, motion.Refreshed = oldFlags, oldFocusTimer, oldRefreshed
		s.server.motionMu.Unlock()
		s.server.auraMu.Lock()
		if previousAura == nil {
			delete(s.server.activeCreatureAuras[petGUID], spellID)
		} else {
			s.server.activeCreatureAuras[petGUID][spellID] = previousAura
		}
		s.server.auraMu.Unlock()
	}()
	s.server.updatePetRuntime(time.Now(), petFocusRegenInterval)
	s.server.motionMu.Lock()
	actualPower := motion.Powers[2]
	s.server.motionMu.Unlock()
	if actualPower != 36 {
		return fmt.Errorf("pet focus aura tick=%d, want 36", actualPower)
	}
	packetFound := false
	trace := s.server.TraceRecorder.Snapshot()
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_POWER_UPDATE) {
			continue
		}
		payload, err := (protocoltrace.Trace{Events: []protocoltrace.Event{event}}).Payload(event)
		if err != nil {
			continue
		}
		reader := protocol.NewReader(payload)
		guid, guidErr := reader.ReadPackedGUID()
		powerType, typeErr := reader.ReadU8()
		power, powerErr := reader.ReadU32()
		if guidErr == nil && typeErr == nil && powerErr == nil && guid == petGUID && powerType == 2 && power == 36 {
			packetFound = true
			break
		}
	}
	if !packetFound {
		return errors.New("pet focus aura tick omitted the source SMSG_POWER_UPDATE")
	}
	return nil
}

func petCastFailedNotReady(trace protocoltrace.Trace, spellID uint32) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_CAST_FAILED) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err == nil && len(payload) >= 6 && binary.LittleEndian.Uint32(payload[1:5]) == spellID && payload[5] == spellFailedNotReady {
			return true
		}
	}
	return false
}

func petSpellPacketHasCategoryCooldown(trace protocoltrace.Trace, petGUID uint64, categoryID uint32) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_SPELLS) {
			continue
		}
		payload, err := (protocoltrace.Trace{Events: []protocoltrace.Event{event}}).Payload(event)
		if err != nil {
			continue
		}
		reader := protocol.NewReader(payload)
		guid, err := reader.ReadU64()
		if err != nil || guid != petGUID {
			continue
		}
		if _, err := reader.Read(10); err != nil {
			continue
		}
		if _, err := reader.Read(10 * 4); err != nil {
			continue
		}
		spellCount, err := reader.ReadU8()
		if err != nil || uint64(spellCount)*4 > uint64(reader.Remaining()) {
			continue
		}
		if _, err := reader.Read(int(spellCount) * 4); err != nil {
			continue
		}
		cooldownCount, err := reader.ReadU8()
		if err != nil || uint64(cooldownCount)*14 > uint64(reader.Remaining()) {
			continue
		}
		for index := uint8(0); index < cooldownCount; index++ {
			if _, err := reader.ReadU32(); err != nil {
				break
			}
			category, err := reader.ReadU16()
			if err != nil {
				break
			}
			if _, err := reader.ReadU32(); err != nil {
				break
			}
			categoryMs, err := reader.ReadU32()
			if err == nil && uint32(category) == categoryID && categoryMs > 0 {
				return true
			}
		}
	}
	return false
}

func petCooldownExpiryClearsCategory(server *Server, motion *creatureMotion, spellID, categoryID uint32) bool {
	if server == nil || motion == nil || spellID == 0 || categoryID == 0 {
		return false
	}
	server.motionMu.Lock()
	defer server.motionMu.Unlock()
	spellCooldowns := make(map[uint32]time.Time, len(motion.SpellCooldowns))
	for id, end := range motion.SpellCooldowns {
		spellCooldowns[id] = end
	}
	categoryCooldowns := make(map[uint32]time.Time, len(motion.SpellCategoryCooldowns))
	for id, end := range motion.SpellCategoryCooldowns {
		categoryCooldowns[id] = end
	}
	spellCategories := make(map[uint32]uint32, len(motion.SpellCooldownCategories))
	for id, category := range motion.SpellCooldownCategories {
		spellCategories[id] = category
	}
	spellCategoryEnds := make(map[uint32]time.Time, len(motion.SpellCooldownCategoryEnds))
	for id, end := range motion.SpellCooldownCategoryEnds {
		spellCategoryEnds[id] = end
	}
	now := time.Now()
	if motion.SpellCooldowns == nil {
		motion.SpellCooldowns = make(map[uint32]time.Time)
	}
	if motion.SpellCategoryCooldowns == nil {
		motion.SpellCategoryCooldowns = make(map[uint32]time.Time)
	}
	if motion.SpellCooldownCategories == nil {
		motion.SpellCooldownCategories = make(map[uint32]uint32)
	}
	if motion.SpellCooldownCategoryEnds == nil {
		motion.SpellCooldownCategoryEnds = make(map[uint32]time.Time)
	}
	motion.SpellCooldowns[spellID] = now.Add(-time.Second)
	motion.SpellCategoryCooldowns[categoryID] = now.Add(time.Minute)
	motion.SpellCooldownCategories[spellID] = categoryID
	motion.SpellCooldownCategoryEnds[spellID] = now.Add(time.Minute)
	prunePetSpellCooldowns(motion, now)
	_, categoryStillActive := motion.SpellCategoryCooldowns[categoryID]
	motion.SpellCooldowns, motion.SpellCategoryCooldowns = spellCooldowns, categoryCooldowns
	motion.SpellCooldownCategories, motion.SpellCooldownCategoryEnds = spellCategories, spellCategoryEnds
	return !categoryStillActive
}

func petCastFailedWithResult(trace protocoltrace.Trace, spellID uint32, result uint8) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_PET_CAST_FAILED) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err == nil && len(payload) >= 6 && binary.LittleEndian.Uint32(payload[1:5]) == spellID && payload[5] == result {
			return true
		}
	}
	return false
}

func petSpellGoPowerMatches(trace protocoltrace.Trace, spellID uint32, petGUID uint64, expectedPower uint32) bool {
	for _, event := range trace.Events {
		if event.Direction != protocoltrace.ServerToClient || event.Opcode != uint32(protocol.OpcodeSMSG_SPELL_GO) {
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(event.Payload)
		if err != nil {
			continue
		}
		reader := protocol.NewReader(payload)
		caster, err := reader.ReadPackedGUID()
		if err != nil || caster != petGUID {
			continue
		}
		if _, err := reader.ReadPackedGUID(); err != nil {
			continue
		}
		if _, err := reader.ReadU8(); err != nil {
			continue
		}
		id, err := reader.ReadU32()
		if err != nil || id != spellID {
			continue
		}
		flags, err := reader.ReadU32()
		if err != nil || flags&protocol.SpellCastFlagPowerLeftSelf == 0 {
			continue
		}
		if _, err := reader.ReadU32(); err != nil {
			continue
		}
		hitCount, err := reader.ReadU8()
		if err != nil {
			continue
		}
		valid := true
		for range hitCount {
			if _, err := reader.ReadU64(); err != nil {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		missCount, err := reader.ReadU8()
		if err != nil || missCount != 0 {
			continue
		}
		if _, err := protocol.ReadSpellTargetData(reader); err != nil {
			continue
		}
		power, err := reader.ReadU32()
		if err == nil && power == expectedPower {
			return true
		}
	}
	return false
}
