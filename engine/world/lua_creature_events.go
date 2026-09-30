package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
)

// luaMotionCreature builds a scripting creature object from a live motion
// snapshot. Unlike the session-bound luaCreature (which resolves spawn rows
// from the world DB), this is server-level so the combat-lifecycle triggers
// (aggro, evade, death) can hand Lua a usable creature without a session.
// The generic object methods (GetName, GetEntry, GetGUID, GetHealth,
// GetMaxHealth, GetLevel, GetX/Y/Z, IsAlive, IsDead, ...) read these fields,
// so Eluna-style handlers get the standard read surface for free.
func (s *Server) luaMotionCreature(motion *creatureMotion) *scripting.Object {
	if s == nil || motion == nil {
		return nil
	}
	state := luaCreatureState{
		GUID:      motion.GUID,
		Entry:     motion.Entry,
		Name:      motion.Name,
		Health:    motion.Health,
		MaxHealth: motion.MaxHealth,
		Level:     motion.Level,
		Map:       motion.Map,
		X:         motion.X,
		Y:         motion.Y,
		Z:         motion.Z,
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetVictim"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		targetGUID := motion.TargetGUID
		s.motionMu.Unlock()
		if targetGUID == 0 {
			return []any{nil}, nil
		}
		if player := s.findPlayer(targetGUID); player != nil {
			return []any{player}, nil
		}
		if target := s.findCreatureMotion(motion.Map, motion.InstanceID, targetGUID); target != nil {
			return []any{s.luaMotionCreature(target)}, nil
		}
		return []any{nil}, nil
	}
	methods["IsInCombat"] = func(_ context.Context, _ []any) ([]any, error) {
		s.motionMu.Lock()
		inCombat := motion.InCombat
		s.motionMu.Unlock()
		return []any{inCombat}, nil
	}
	return &scripting.Object{Type: "Creature", Fields: map[string]any{
		"Name": state.Name, "GUID": state.GUID, "Entry": state.Entry,
		"Map": state.Map, "MapId": state.Map,
		"X": state.X, "Y": state.Y, "Z": state.Z,
		"Health": state.Health, "MaxHealth": state.MaxHealth, "Level": state.Level,
		"InWorld": true,
	}, Methods: methods}
}

// fireCreatureLuaEvent dispatches an Eluna RegisterCreatureEvent hook for the
// creature's entry. Eluna argument order: (event, creature, ...) with the
// per-event extra arguments appended after the creature. Never fires when
// the scripting runtime is disabled or the creature is gone; Lua errors are
// logged by the runtime, never propagated. Returns true when any handler
// returns boolean true, matching Eluna's CallAllFunctionsBool
// (lua_isboolean-checked) OR-semantics, so START_HOOK_WITH_RETVAL call
// sites can veto the default action; no bindings or a disabled runtime
// behaves like C++'s RETVAL=false.
func (s *Server) fireCreatureLuaEvent(ctx context.Context, motion *creatureMotion, event int, extra ...any) bool {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return false
	}
	args := make([]any, 0, len(extra)+2)
	args = append(args, event, creature)
	args = append(args, extra...)
	results, _ := s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, event, args...)
	return luaHookVeto(results)
}

// luaHookVeto ORs handler return values the way Eluna's CallAllFunctionsBool
// does: only an actual boolean true flips the result (HookHelpers.h checks
// lua_isboolean before lua_toboolean), so a handler returning 1, "yes" or an
// object does not veto.
func luaHookVeto(results []any) bool {
	for _, r := range results {
		if b, ok := r.(bool); ok && b {
			return true
		}
	}
	return false
}

// fireCreatureDamageTaken dispatches Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9)
// for the creature before damage is applied. C++ (Eluna::DamageTaken,
// CreatureHooks.cpp:114-141, fired from Unit::DealDamage, Unit.cpp:697-702)
// passes (event, creature, attacker, damage); each handler may return a
// boolean veto and, as the second return, a replacement damage. The boolean
// gates only the empty ScriptedAI::DamageTaken base (UnitAI.h, the sole
// implementation), a provable no-op like the SpellHit ruling, so Go consumes
// only the damage rewrite. A numeric second return rewrites damage AND the
// argument the next handler sees (the damageIndex/ReplaceArgument loop); the
// final damage is the last numeric rewrite. Non-numeric, negative, or >
// MaxUint32 returns are ignored, mirroring lua_isnumber and
// CHECKVAL<uint32>'s range errors (LuaEngine.cpp:770-788); fractional values
// truncate toward zero like C++'s static_cast<unsigned int>. Both binding
// families fire: entry handlers first, then the unique handlers for this
// creature's GUID/instance, matching SetupStack's merged call list
// (HookHelpers.h:37-39) with the rewritten damage threaded through both
// passes. Returns the damage to apply.
func (s *Server) fireCreatureDamageTaken(ctx context.Context, motion *creatureMotion, attacker *scripting.Object, damage uint32) uint32 {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return damage
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return damage
	}
	args := []any{scripting.CreatureEventOnDamageTaken, creature, attacker, damage}
	// The rewritten damage is threaded through the update closure, so the
	// per-handler return pairs need no post-processing.
	update := func(returns []any) {
		if len(returns) < 2 {
			return
		}
		if n, ok := returns[1].(float64); ok && n >= 0 && n <= 4294967295 {
			damage = uint32(n)
			args[3] = damage
		}
	}
	_, _ = s.Features.Scripts.TriggerCreatureEvent2Updated(ctx, motion.Entry, scripting.CreatureEventOnDamageTaken, args, update)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent2Updated(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDamageTaken, args, update)
	return damage
}

// fireCreatureCorpseRemoved dispatches Eluna CREATURE_EVENT_ON_CORPSE_REMOVED
// (26) with the pending respawn delay. C++ (Eluna::CorpseRemoved,
// CreatureHooks.cpp:236-265, fired from Creature::RemoveCorpse via
// ai->CorpseRemoved, Creature.cpp:393/429) passes (event, creature,
// respawnDelay); each handler may return a boolean veto and, as the second
// return, a replacement delay. The boolean gates only ScriptedAI::
// CorpseRemoved, which has no override — the sole implementation is the empty
// CreatureAI base (CreatureAI.h:190), a provable no-op like the SpellHit
// ruling, so Go consumes only the delay rewrite. A numeric second return
// rewrites the delay AND the argument the next handler sees (the
// respawnDelayIndex/ReplaceArgument loop); the final delay is the last
// numeric rewrite. Non-numeric, negative, or > MaxUint32 returns are ignored,
// mirroring lua_isnumber and CHECKVAL<uint32>'s range errors
// (LuaEngine.cpp:770-788); fractional values truncate toward zero like C++'s
// static_cast<unsigned int>. Both binding families fire: entry handlers
// first, then the unique handlers for this creature's GUID/instance, matching
// SetupStack's merged call list (HookHelpers.h:37-39) with the rewritten delay
// threaded through both passes. Returns the delay to use.
func (s *Server) fireCreatureCorpseRemoved(ctx context.Context, motion *creatureMotion, delay uint32) uint32 {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return delay
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return delay
	}
	args := []any{scripting.CreatureEventOnCorpseRemoved, creature, delay}
	// The rewritten delay is threaded through the update closure, so the
	// per-handler return pairs need no post-processing.
	update := func(returns []any) {
		if len(returns) < 2 {
			return
		}
		if n, ok := returns[1].(float64); ok && n >= 0 && n <= 4294967295 {
			delay = uint32(n)
			args[2] = delay
		}
	}
	_, _ = s.Features.Scripts.TriggerCreatureEvent2Updated(ctx, motion.Entry, scripting.CreatureEventOnCorpseRemoved, args, update)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent2Updated(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnCorpseRemoved, args, update)
	return delay
}

// fireCreatureMoveInLOS dispatches Eluna CREATURE_EVENT_ON_MOVE_IN_LOS (27)
// when the creature is about to aggro a player it has sighted. C++
// (Eluna::MoveInLineOfSight, CreatureHooks.cpp:268-274, reached from
// ElunaCreatureAI::MoveInLineOfSight, ElunaCreatureAI.h:212-216, which the
// grid relocation worker drives via CreatureUnitRelocationWorker,
// GridNotifiers.cpp:129-141, whenever a detectable unit relocates into the
// creature's sight range) passes (event, creature, unit) through
// CallAllFunctionsBool: a boolean true from any handler vetoes the "normal
// action", which for ElunaCreatureAI is ScriptedAI::MoveInLineOfSight — the
// default aggro engage. Go's fire site is the per-tick aggro scan's engage
// branch (stepCreatureMotion): the hook fires where the default aggro would
// happen, and a veto skips exactly that engage, mirroring C++'s
// `if (!sEluna->MoveInLineOfSight(me, who)) ScriptedAI::MoveInLineOfSight
// (who)`. Both binding families fire — entry handlers first, then the
// unique handlers for this creature's GUID/instance — matching SetupStack's
// merged call list (HookHelpers.h:37-39); the veto ORs across both passes
// like CallAllFunctionsBool. No handlers or a disabled runtime behaves like
// C++'s RETVAL=false default. Returns true when the aggro must be skipped.
func (s *Server) fireCreatureMoveInLOS(ctx context.Context, motion *creatureMotion, sess *session) bool {
	if s == nil || motion == nil || sess == nil || s.Features == nil || s.Features.Scripts == nil {
		return false
	}
	creature := s.luaMotionCreature(motion)
	who := sess.luaPlayer()
	if creature == nil || who == nil {
		return false
	}
	args := []any{scripting.CreatureEventOnMoveInLOS, creature, who}
	results, _ := s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnMoveInLOS, args...)
	uniqueResults, _ := s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnMoveInLOS, args...)
	return luaHookVeto(results) || luaHookVeto(uniqueResults)
}

// fireCreatureDied dispatches Eluna CREATURE_EVENT_ON_DIED (4) for a
// creature killed through the session kill funnel. C++
// (Eluna::JustDied, CreatureHooks.cpp:143-152, reached from
// ElunaCreatureAI::JustDied, ElunaCreatureAI.h:108-113) calls On_Reset
// first — CREATURE_EVENT_ON_RESET (23) is a void hook (START_HOOK at
// CreatureHooks.cpp:277-282), so its returns are discarded — then fires
// ON_DIED with (event, creature, killer). Both binding families fire,
// entry then unique, matching SetupStack's merged call list
// (HookHelpers.h:37-39). The boolean veto gates only
// ScriptedAI::JustDied: the empty CreatureAI base for plain creatures and
// BossAI::_JustDied (summon despawn + instance DONE) for bosses — Go has
// no summon-tracking model to gate and instance encounter clearing already
// runs unconditionally in the kill paths, so the veto is a provable no-op
// like the SpellHit ruling and is discarded. The killer is the session's
// luaPlayer: every Go kill path (melee, spell, pet) funnels through
// session.onCreatureKilled; C++ would push the pet unit on pet kills, a
// nuance Go's funnel has no model for.
func (s *session) fireCreatureDied(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || s.server == nil || s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	sv := s.server
	creature := sv.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	resetArgs := []any{scripting.CreatureEventOnReset, creature}
	_, _ = sv.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
	_, _ = sv.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
	diedArgs := []any{scripting.CreatureEventOnDied, creature, s.luaPlayer()}
	_, _ = sv.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnDied, diedArgs...)
	_, _ = sv.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnDied, diedArgs...)
}

// fireCreatureSpawned dispatches Eluna CREATURE_EVENT_ON_SPAWN (5) when a
// respawn timer restores a creature. C++ (Eluna::JustRespawned,
// CreatureHooks.cpp:210-216, reached in Trinity from
// ElunaCreatureAI::JustAppeared, ElunaCreatureAI.h:169-175, which the
// creature grid-update drives via m_triggerJustAppeared, Creature.
// cpp:699-704) calls On_Reset first — CREATURE_EVENT_ON_RESET (23) is a
// void hook (START_HOOK at CreatureHooks.cpp:277-282), returns discarded —
// then fires ON_SPAWN with (event, creature). Both binding families fire,
// entry then unique, matching SetupStack's merged call list
// (HookHelpers.h:37-39). The boolean veto gates only
// CreatureAI::JustAppeared's TempSummon follow-on-spawn terms (CreatureAI.
// cpp:189-205) — Go has no TempSummon model, so it is a provable no-op and
// is discarded. Go fires this only on the respawn path; C++ also fires it
// on first grid activation of a fresh spawn, which has no Go fire site.
func (s *Server) fireCreatureSpawned(ctx context.Context, motion *creatureMotion) {
	if s == nil || motion == nil || s.Features == nil || s.Features.Scripts == nil {
		return
	}
	creature := s.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	resetArgs := []any{scripting.CreatureEventOnReset, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnReset, resetArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnReset, resetArgs...)
	spawnArgs := []any{scripting.CreatureEventOnSpawn, creature}
	_, _ = s.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, scripting.CreatureEventOnSpawn, spawnArgs...)
	_, _ = s.Features.Scripts.TriggerUniqueCreatureEvent(ctx, motion.GUID, motion.InstanceID, scripting.CreatureEventOnSpawn, spawnArgs...)
}

// luaQuest builds the Eluna Quest userdata surface used as the quest
// argument of the quest hooks. It mirrors the GetQuest global's quest
// object in engine/scripting/globals.go (ID/Name/Title/Level/MinLevel/Flags
// plus the GetId/GetLevel/GetMinLevel/GetFlags/GetNextQuestId/GetPrevQuestId/
// GetType/HasFlag/IsDaily/IsRepeatable methods); a missing quest_template
// row yields a bare ID-only object so the hook still fires with the correct
// argument count (the GetQuest global pushes nil instead).
func (s *session) luaQuest(ctx context.Context, questID uint32) *scripting.Object {
	var id, level, minLevel, flags, nextID, prevID, questType int64
	var title string
	if s != nil && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT ID, COALESCE(LogTitle, ''), COALESCE(QuestLevel, 0), COALESCE(MinLevel, 0), COALESCE(Flags, 0), COALESCE(RewardNextQuest, 0), COALESCE(PrevQuestId, 0), COALESCE(Type, 0) FROM quest_template WHERE ID = ?`, questID).Scan(&id, &title, &level, &minLevel, &flags, &nextID, &prevID, &questType)
	}
	if id == 0 {
		id = int64(questID)
	}
	methods := map[string]scripting.ObjectMethod{}
	methods["GetId"] = luaNoArgs(func() any { return uint32(id) })
	methods["GetLevel"] = luaNoArgs(func() any { return uint32(level) })
	methods["GetMinLevel"] = luaNoArgs(func() any { return uint32(minLevel) })
	methods["GetFlags"] = luaNoArgs(func() any { return uint32(flags) })
	methods["GetNextQuestId"] = luaNoArgs(func() any { return int32(nextID) })
	methods["GetPrevQuestId"] = luaNoArgs(func() any { return int32(prevID) })
	methods["GetType"] = luaNoArgs(func() any { return uint32(questType) })
	methods["HasFlag"] = func(_ context.Context, args []any) ([]any, error) {
		var flag uint32
		if len(args) > 0 {
			if n, ok := args[0].(uint32); ok {
				flag = n
			}
		}
		return []any{uint32(flags)&flag != 0}, nil
	}
	methods["IsDaily"] = luaNoArgs(func() any { return uint32(flags)&0x1000 != 0 })
	methods["IsRepeatable"] = luaNoArgs(func() any { return uint32(flags)&0x9000 != 0 })
	return &scripting.Object{Type: "Quest", Fields: map[string]any{
		"ID": uint32(id), "Name": title, "Title": title,
		"Level": uint32(level), "MinLevel": uint32(minLevel), "Flags": uint32(flags),
	}, Methods: methods}
}

// fireCreatureQuestHook dispatches an Eluna RegisterCreatureEvent quest hook
// for the creature questgiver: ON_QUEST_ACCEPT (31), ON_QUEST_REWARD (34) and
// ON_DIALOG_STATUS (35). C++ fires these only from the TYPEID_UNIT arms
// (Player.cpp:15119, QuestHandler.cpp:332, Player.cpp:16293); gameobject and
// item givers route to the GameObjectEvents and ItemQuestEvents families, so
// non-creature GUIDs are skipped here. Unlike the combat hooks, the quest
// hooks push the player first — Eluna argument order is (event, player,
// creature, quest[, opt]) per CreatureHooks.cpp. The return values of these
// hooks are discarded by every C++ call site, so Go discards them too.
// Reference: Eluna::OnQuestAccept / OnQuestReward / GetDialogStatus.
func (s *session) fireCreatureQuestHook(ctx context.Context, giverGUID uint64, event int, extra ...any) {
	if s == nil || s.server == nil || uint16(giverGUID>>48) != 0xF130 {
		return
	}
	if s.server.Features == nil || s.server.Features.Scripts == nil {
		return
	}
	motion := s.findCreatureMotion(giverGUID)
	if motion == nil {
		return
	}
	creature := s.server.luaMotionCreature(motion)
	if creature == nil {
		return
	}
	args := make([]any, 0, len(extra)+3)
	args = append(args, event, s.luaPlayer(), creature)
	args = append(args, extra...)
	_, _ = s.server.Features.Scripts.TriggerCreatureEvent(ctx, motion.Entry, event, args...)
}
