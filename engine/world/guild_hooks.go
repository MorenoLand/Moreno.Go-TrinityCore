package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaGuildObject builds the Lua Guild object for the Eluna GUILD_EVENT_*
// hooks, the Go model of Eluna::GuildHooks' Push(guild): ID, Name, LeaderGUID
// and MemberCount, the same surface the GetGuildByName / GetGuildByLeaderGUID
// globals expose (scripting.NewGuildObject). Returns nil when the scripts
// runtime is absent or the guild row is gone; callers fire the hooks before
// the rows are deleted, like C++ Guild::Disband calling OnGuildDisband before
// the data removal (Guild.cpp:1144).
func (s *session) luaGuildObject(ctx context.Context, guildID uint32) *scripting.Object {
	if guildID == 0 || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil ||
		s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return nil
	}
	cdb := s.server.CharactersStore.DB
	var id, leaderGUID int64
	var name string
	if err := cdb.QueryRowContext(ctx, "SELECT guildid, name, leaderguid FROM guild WHERE guildid = ? LIMIT 1", guildID).Scan(&id, &name, &leaderGUID); err != nil || id == 0 {
		return nil
	}
	var memberCount int64
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(1) FROM guild_member WHERE guildid = ?", guildID).Scan(&memberCount)
	return scripting.NewGuildObject(uint32(id), name, uint64(leaderGUID), uint32(memberCount))
}

// fireGuildEvent dispatches an Eluna GUILD_EVENT_* hook
// (RegisterGuildEvent) as (event, guild, ...args), mirroring
// Eluna::GuildHooks (GuildHooks.cpp) via ScriptMgr::OnGuild*
// (ScriptMgr.cpp:2246-2333). A nil guild object means no handler can see the
// guild, so the event does not fire — C++ always has a live Guild*. These
// hooks never cancel: the C++ call sites use CallAllFunctions, not the Bool
// form.
func (s *session) fireGuildEvent(ctx context.Context, event int, guild *scripting.Object, args ...any) {
	if guild == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	if _, err := s.server.Features.Scripts.TriggerGuildEvent(ctx, event, append([]any{guild}, args...)...); err != nil {
		s.debug("lua guild event failed", "account", s.accountName, "event", event, "error", err)
	}
}

// fireGuildMoneyEvent dispatches GUILD_EVENT_ON_MONEY_WITHDRAW (7) /
// GUILD_EVENT_ON_MONEY_DEPOSIT (8) as (event, guild, player, amount[, isRepair])
// and returns the amount after the C++ ReplaceArgument chaining
// (GuildHooks.cpp OnMemberWitdrawMoney / OnMemberDepositMoney): each
// handler's single numeric return rewrites the amount, and the rewritten
// argument is what the next handler sees; the final amount is what the
// transfer uses. Non-numeric, negative, or > MaxUint32 returns are ignored,
// mirroring lua_isnumber and CHECKVAL<uint32> (LuaEngine.cpp:770-788);
// fractional values truncate toward zero like C++'s static_cast<unsigned
// int>. extra carries isRepair for the withdraw hook only (deposit passes
// none), matching the C++ argument shapes.
func (s *session) fireGuildMoneyEvent(ctx context.Context, event int, guild *scripting.Object, player *scripting.Object, amount uint32, extra ...any) uint32 {
	if guild == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return amount
	}
	args := append([]any{event, guild, player, amount}, extra...)
	update := func(returns []any) {
		if len(returns) < 1 {
			return
		}
		if n, ok := guildMoneyReturn(returns[0]); ok {
			amount = n
			args[3] = n
		}
	}
	if _, err := s.server.Features.Scripts.TriggerGuildEventUpdated(ctx, event, args, update); err != nil {
		s.debug("lua guild money event failed", "account", s.accountName, "event", event, "error", err)
	}
	return amount
}

func guildMoneyReturn(v any) (uint32, bool) {
	switch n := v.(type) {
	case float64:
		if n >= 0 && n <= 4294967295 {
			return uint32(n), true
		}
	case float32:
		if n >= 0 && n <= 4294967295 {
			return uint32(n), true
		}
	case int:
		if n >= 0 && uint64(n) <= 4294967295 {
			return uint32(n), true
		}
	case int64:
		if n >= 0 && n <= 4294967295 {
			return uint32(n), true
		}
	case uint64:
		if n <= 4294967295 {
			return uint32(n), true
		}
	case uint32:
		return n, true
	}
	return 0, false
}
