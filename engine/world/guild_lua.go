package world

import (
	"context"
	"fmt"
	"math"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// guildLuaRankNone mirrors GUILD_RANK_NONE (Guild.h:53), the Eluna default
// for Guild:AddMember — resolved to the lowest rank id like C++.
const guildLuaRankNone = 0xFF

// attachGuildMutations registers the Eluna mutating Guild methods on a Lua
// Guild object: SetLeader/SetBankTabText/SendPacket/SendPacketToRanked/
// Disband/AddMember/DeleteMember/SetMemberRank (GuildMethods.h). Each method
// re-checks the guild row, so a guild disbanded since the object's snapshot
// was taken degrades to the C++ no-op instead of mutating a stale copy. The
// methods run on the scripting goroutine; DB and session writes are safe from
// there.
func attachGuildMutations(srv *Server, s *session, obj *scripting.Object, guildID uint32) {
	if srv == nil || obj == nil || obj.Methods == nil || guildID == 0 {
		return
	}
	live := func(ctx context.Context) bool {
		if srv.CharactersStore == nil || srv.CharactersStore.DB == nil {
			return false
		}
		var one int
		return srv.CharactersStore.DB.QueryRowContext(ctx, "SELECT 1 FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&one) == nil
	}

	// SetLeader: Guild::HandleSetLeader (Guild.cpp:1363) as called by Eluna
	// (GuildMethods.h:163): the argument player's session is the caller, so
	// only the current leader passes the _IsLeader gate — anyone else gets
	// GUILD_COMMAND_CHANGE_LEADER + ERR_GUILD_PERMISSIONS. The old leader
	// must be a member and the new leader (the player's own name) must be a
	// member; _SetLeaderGUID sets the guid + rank 0, then the old leader is
	// demoted to GR_OFFICER, and GE_LEADER_CHANGED is broadcast.
	obj.Methods["SetLeader"] = func(ctx context.Context, args []any) ([]any, error) {
		playerObj, err := guildLuaObjectArg(args, 0, "player")
		if err != nil {
			return nil, err
		}
		playerGUID, ok := objectUint64Field(playerObj, "GUID")
		if !ok || playerGUID == 0 {
			return nil, nil
		}
		sess := srv.findSessionByGUID(playerGUID)
		if sess == nil || sess.player == nil || !live(ctx) {
			return nil, nil
		}
		db := srv.CharactersStore.DB
		var leaderGUID int64
		if err := db.QueryRowContext(ctx, "SELECT leaderguid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&leaderGUID); err != nil || uint64(leaderGUID) != playerGUID {
			sess.sendGuildCommandResult(guildCmdChangeLeader, "", errGuildPermissions)
			return nil, nil
		}
		var oldMember int64
		if err := db.QueryRowContext(ctx, "SELECT guid FROM guild_member WHERE guildid = ? AND guid = ? LIMIT 1", guildID, playerGUID).Scan(&oldMember); err != nil {
			return nil, nil
		}
		newName := sess.player.Name
		var newGUID int64
		if err := db.QueryRowContext(ctx, `SELECT gm.guid FROM guild_member gm
			JOIN characters c ON c.guid = gm.guid
			WHERE gm.guildid = ? AND c.name = ? LIMIT 1`, guildID, newName).Scan(&newGUID); err != nil || newGUID == 0 {
			return nil, nil
		}
		// _SetLeaderGUID (Guild.cpp:2521): guid + rank 0 via ChangeRank.
		_, _ = db.ExecContext(ctx, "UPDATE guild SET leaderguid = ? WHERE guildid = ?", newGUID, guildID)
		_, _ = db.ExecContext(ctx, "UPDATE guild_member SET `rank` = 0 WHERE guid = ? AND guildid = ?", newGUID, guildID)
		if ns := srv.findSessionByGUID(uint64(newGUID)); ns != nil && ns.player != nil {
			ns.player.GuildRank = 0
			ns.sendPlayerUpdate()
		}
		// HandleSetLeader (Guild.cpp:1376): old leader drops to GR_OFFICER.
		_, _ = db.ExecContext(ctx, "UPDATE guild_member SET `rank` = 1 WHERE guid = ? AND guildid = ?", playerGUID, guildID)
		sess.player.GuildRank = 1
		sess.sendPlayerUpdate()
		s.broadcastGuildEventTo(guildID, guildEventPayload(guildEventLeaderChanged, 0, newName, newName))
		return nil, nil
	}

	// SetBankTabText: Guild::SetBankTabText (Guild.cpp:2379) — SetText (utf8
	// truncated, persisted) then SendText broadcast to every online member;
	// a GetBankTab miss writes and broadcasts nothing.
	obj.Methods["SetBankTabText"] = func(ctx context.Context, args []any) ([]any, error) {
		tabID, err := guildLuaUint8Arg(args, 0)
		if err != nil {
			return nil, err
		}
		text, err := guildLuaStringArg(args, 1)
		if err != nil {
			return nil, err
		}
		if !live(ctx) {
			return nil, nil
		}
		db := srv.CharactersStore.DB
		text = truncateBankTabText(text)
		res, err := db.ExecContext(ctx, "UPDATE guild_bank_tab SET TabText = ? WHERE guildid = ? AND TabId = ?", text, guildID, tabID)
		if err != nil {
			return nil, nil
		}
		if n, _ := res.RowsAffected(); n > 0 {
			buf := protocol.NewBuffer(64 + len(text))
			buf.WriteU8(tabID)
			buf.WriteCString(text)
			srv.broadcastGuildRawPackets(guildID, uint16(protocol.OpcodeMSG_QUERY_GUILD_BANK_TEXT), buf.Bytes(), nil)
		}
		return nil, nil
	}

	// SendPacket: Guild::BroadcastPacket (Guild.cpp:2155) — raw packet fan-out
	// to every online member (Member::FindConnectedPlayer per member).
	obj.Methods["SendPacket"] = func(ctx context.Context, args []any) ([]any, error) {
		packet, err := guildLuaPacketArg(args, 0)
		if err != nil {
			return nil, err
		}
		if live(ctx) {
			srv.broadcastGuildRawPackets(guildID, uint16(packet.Opcode), packet.Data, nil)
		}
		return nil, nil
	}

	// SendPacketToRanked: Guild::BroadcastPacketToRank (Guild.cpp:2147) —
	// the fan-out filtered to members at the given rank.
	obj.Methods["SendPacketToRanked"] = func(ctx context.Context, args []any) ([]any, error) {
		packet, err := guildLuaPacketArg(args, 0)
		if err != nil {
			return nil, err
		}
		rankID, err := guildLuaUint8Arg(args, 1)
		if err != nil {
			return nil, err
		}
		if live(ctx) {
			srv.broadcastGuildRawPackets(guildID, uint16(packet.Opcode), packet.Data, &rankID)
		}
		return nil, nil
	}

	// Disband: Guild::Disband (Guild.cpp:1141) — the shared helper fires
	// OnGuildDisband and the per-member OnGuildRemoveMember(isDisbanding)
	// before any row is removed, like the CMSG_GUILD_DISBAND arm.
	obj.Methods["Disband"] = func(ctx context.Context, _ []any) ([]any, error) {
		if live(ctx) {
			s.disbandGuildWithLuaEvents(ctx, guildID)
		}
		return nil, nil
	}

	// AddMember: Guild::AddMember (Guild.cpp:2195) — the player must be
	// guildless, the character row must exist (the C++ offline "Player must
	// exist" arm), GUILD_RANK_NONE resolves to the lowest rank, and
	// ScriptMgr::OnGuildAddMember fires after the store+broadcast.
	obj.Methods["AddMember"] = func(ctx context.Context, args []any) ([]any, error) {
		playerObj, err := guildLuaObjectArg(args, 0, "player")
		if err != nil {
			return nil, err
		}
		rankID, err := guildLuaUint8ArgDefault(args, 1, guildLuaRankNone)
		if err != nil {
			return nil, err
		}
		playerGUID, ok := objectUint64Field(playerObj, "GUID")
		if !ok || playerGUID == 0 || !live(ctx) {
			return nil, nil
		}
		db := srv.CharactersStore.DB
		if rankID == guildLuaRankNone {
			rankID = guildLowestRankID(ctx, db, guildID)
		}
		var name string
		if err := db.QueryRowContext(ctx, "SELECT name FROM characters WHERE guid = ? LIMIT 1", playerGUID).Scan(&name); err != nil || name == "" {
			return nil, nil
		}
		if !s.guildAddMember(ctx, db, guildID, playerGUID, rankID, name) {
			return nil, nil
		}
		var playerArg *scripting.Object
		if ts := srv.findSessionByGUID(playerGUID); ts != nil {
			playerArg = ts.luaPlayer()
		}
		s.fireGuildEvent(ctx, scripting.GuildEventOnAddMember, s.luaGuildObject(ctx, guildID), playerArg, uint32(rankID))
		return nil, nil
	}

	// DeleteMember: Guild::DeleteMember (Guild.cpp:2277) — the Eluna
	// isDisbanding arg skips the leader-replacement arm (the C++ !isDisbanding
	// gate); ScriptMgr::OnGuildRemoveMember fires before the row is removed.
	obj.Methods["DeleteMember"] = func(ctx context.Context, args []any) ([]any, error) {
		playerObj, err := guildLuaObjectArg(args, 0, "player")
		if err != nil {
			return nil, err
		}
		isDisbanding, err := guildLuaBoolArgDefault(args, 1, false)
		if err != nil {
			return nil, err
		}
		playerGUID, ok := objectUint64Field(playerObj, "GUID")
		if !ok || playerGUID == 0 || !live(ctx) {
			return nil, nil
		}
		var playerArg *scripting.Object
		if ts := srv.findSessionByGUID(playerGUID); ts != nil {
			playerArg = ts.luaPlayer()
		}
		s.fireGuildEvent(ctx, scripting.GuildEventOnRemoveMember, s.luaGuildObject(ctx, guildID), playerArg, isDisbanding)
		s.guildDeleteMember(ctx, srv.CharactersStore.DB, guildID, playerGUID, isDisbanding)
		return nil, nil
	}

	// SetMemberRank: Guild::ChangeMemberRank (Guild.cpp:2336) — the rank must
	// exist (newRank <= lowest rank id); Member::ChangeRank updates the row
	// and the online player's rank field.
	obj.Methods["SetMemberRank"] = func(ctx context.Context, args []any) ([]any, error) {
		playerObj, err := guildLuaObjectArg(args, 0, "player")
		if err != nil {
			return nil, err
		}
		newRank, err := guildLuaUint8Arg(args, 1)
		if err != nil {
			return nil, err
		}
		playerGUID, ok := objectUint64Field(playerObj, "GUID")
		if !ok || playerGUID == 0 || !live(ctx) {
			return nil, nil
		}
		db := srv.CharactersStore.DB
		if newRank > guildLowestRankID(ctx, db, guildID) {
			return nil, nil
		}
		var member int64
		if err := db.QueryRowContext(ctx, "SELECT guid FROM guild_member WHERE guildid = ? AND guid = ? LIMIT 1", guildID, playerGUID).Scan(&member); err != nil {
			return nil, nil
		}
		_, _ = db.ExecContext(ctx, "UPDATE guild_member SET `rank` = ? WHERE guid = ? AND guildid = ?", newRank, playerGUID, guildID)
		if ts := srv.findSessionByGUID(playerGUID); ts != nil && ts.player != nil && ts.player.GuildID == guildID {
			ts.player.GuildRank = newRank
			ts.sendPlayerUpdate()
		}
		return nil, nil
	}
}

// broadcastGuildRawPackets is the raw-packet twin of broadcastGuildEventTo:
// Guild::BroadcastPacket's fan-out (Guild.cpp:2155) with an optional
// BroadcastPacketToRank rank filter (Guild.cpp:2147). Only sessions whose
// player is world-ready and in this guild receive the packet, the Go shape of
// Member::FindConnectedPlayer per member.
func (s *Server) broadcastGuildRawPackets(guildID uint32, opcode uint16, payload []byte, rankID *uint8) {
	ranks := map[uint64]uint8{}
	if rankID != nil && s.CharactersStore != nil && s.CharactersStore.DB != nil {
		if rows, err := s.CharactersStore.DB.Query("SELECT guid, `rank` FROM guild_member WHERE guildid = ?", guildID); err == nil && rows != nil {
			for rows.Next() {
				var guid, rank int64
				if rows.Scan(&guid, &rank) == nil {
					ranks[uint64(guid)] = uint8(rank)
				}
			}
			rows.Close()
		}
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for target := range s.sessions {
		if !target.worldReady.Load() || target.player == nil || target.player.GuildID != guildID {
			continue
		}
		if rankID != nil {
			if r, ok := ranks[target.playerGUID]; !ok || r != *rankID {
				continue
			}
		}
		_ = target.write(opcode, payload, true)
	}
}

// guildLuaObjectArg mirrors groupLuaObjectArg for the guild mutations: the
// Eluna CHECKOBJ<Player> argument.
func guildLuaObjectArg(args []any, index int, what string) (*scripting.Object, error) {
	if index >= len(args) {
		return nil, fmt.Errorf("argument %d is required", index+1)
	}
	obj, ok := args[index].(*scripting.Object)
	if !ok || obj == nil {
		return nil, fmt.Errorf("argument %d must be a %s object", index+1, what)
	}
	return obj, nil
}

// guildLuaUint8Arg coerces a Lua unsigned argument like Eluna's
// CHECKVAL<uint8>, erroring out of range.
func guildLuaUint8Arg(args []any, index int) (uint8, error) {
	n, err := groupLuaUint64Arg(args, index)
	if err != nil {
		return 0, err
	}
	if n > math.MaxUint8 {
		return 0, fmt.Errorf("argument %d out of uint8 range", index+1)
	}
	return uint8(n), nil
}

// guildLuaUint8ArgDefault is the optional-argument twin of
// guildLuaUint8Arg (Eluna's CHECKVAL with a default, like AddMember's rank).
func guildLuaUint8ArgDefault(args []any, index int, def uint8) (uint8, error) {
	if index >= len(args) {
		return def, nil
	}
	return guildLuaUint8Arg(args, index)
}

// guildLuaBoolArgDefault is the optional-argument twin of groupLuaBoolArg
// (Eluna's CHECKVAL<bool> with a default, like DeleteMember's isDisbanding).
func guildLuaBoolArgDefault(args []any, index int, def bool) (bool, error) {
	if index >= len(args) {
		return def, nil
	}
	return groupLuaBoolArg(args, index)
}

// guildLuaStringArg reads a required Lua string argument.
func guildLuaStringArg(args []any, index int) (string, error) {
	if index >= len(args) {
		return "", fmt.Errorf("argument %d is required", index+1)
	}
	str, ok := args[index].(string)
	if !ok {
		return "", fmt.Errorf("argument %d must be a string", index+1)
	}
	return str, nil
}

// guildLuaPacketArg reads the Eluna CHECKOBJ<WorldPacket> argument.
func guildLuaPacketArg(args []any, index int) (*scripting.Packet, error) {
	if index >= len(args) {
		return nil, fmt.Errorf("argument %d is required", index+1)
	}
	packet, ok := args[index].(*scripting.Packet)
	if !ok || packet == nil {
		return nil, fmt.Errorf("argument %d must be a WorldPacket", index+1)
	}
	return packet, nil
}
