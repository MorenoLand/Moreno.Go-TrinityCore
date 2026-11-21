-- Drek'Thar (Alterac Valley) --
-- Lua port of src/server/scripts/EasternKingdoms/AlteracValley/
-- boss_drekthar.cpp (boss_drektharAI — 5-event combat scheduler +
-- CheckInRoom home leash). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_drekthar follows
-- AddSC_boss_balinda; the Alterac Valley boss set continues with
-- galvangar, vanndar).
-- Entry (verifiable from the C++ sources): 11946 "Drek'Thar" in the
-- BattlegroundAV.h AV creature lists (line 1076, line 1298; neighbors
-- 11947/11948/11949 are Galvangar, Vanndar, Balinda — the other AV
-- boss-set members). The creature_template ScriptName binding is
-- DB-side (no TDB in this workspace), but the entry itself is
-- C++-verifiable so the port is registered (storm_cloud /
-- av_marshal / balinda precedent).
-- Eluna creature events: 5 OnSpawn, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_WHIRLWIND: victim-cast WHIRLWIND 15589, {1,20}s init ->
--   {8,18}s;
-- EVENT_WHIRLWIND2: victim-cast WHIRLWIND2 13736, {1,20}s init ->
--   {7,25}s;
-- EVENT_KNOCKDOWN: victim-cast KNOCKDOWN 19128, 12s init ->
--   {10,15}s;
-- EVENT_FRENZY: victim-cast FRENZY 8269 (cast on victim in C++),
--   6s init -> {20,30}s;
-- EVENT_RANDOM_YELL: Talk(SAY_RANDOM=3), {20,30}s init -> {20,30}s;
-- JustEngagedWith Talk(SAY_AGGRO=0);
-- JustAppeared Talk(SAY_RESPAWN=2) modeled on OnSpawn(5) — no
-- JustAppeared bridge on the Lua surface (kazzak precedent);
-- storm_cloud convention carries spawn-time Talk on OnSpawn(5).
-- Cast-state gates PORTED: the C++ UpdateAI pre-loop gate
-- (boss_drekthar.cpp:99) + in-loop gate (:130) ride the fire-time 500ms
-- IsCasting defer (mal_ganis idiom adapted to timer arms; IsCasting is
-- live, lua_creature_events.go:1950): a firing arm that finds the
-- creature casting re-schedules itself after 500ms and consumes no
-- re-arm, so the arm stays due; the random-yell Talk arm defers the same
-- way (it sits inside the gated C++ loop). The JustAppeared respawn Talk
-- is a separate hook and stays ungated per C++. The in-loop gate's
-- same-tick co-expiry suppression has no timer-arm model (independent
-- timers fire on separate callbacks) and is the only unmodeled sliver.
-- Unmodeled: none. CheckInRoom (home-position 2D distance > 50yd ->
-- EnterEvadeMode + Talk(SAY_EVADE=1)) PORTED 2026-11-21 — the bridge
-- is live (creature:EnterEvadeMode, lua_creature_events.go:6342); the
-- 1s poll below is the per-tick C++ check's twin (balinda precedent);
-- evade still re-arms through OnReset(23) when the engine leashes the
-- creature.
-- SPELL_SWEEPING_STRIKES=18765, SPELL_CLEAVE=20677,
-- SPELL_WINDFURY=35886, SPELL_STORMPIKE=51876 are declared in the C++
-- Spells enum (all marked "not sure") but never used by this AI.

local SPELL_WHIRLWIND = 15589
local SPELL_WHIRLWIND2 = 13736
local SPELL_KNOCKDOWN = 19128
local SPELL_FRENZY = 8269

local SAY_AGGRO = 0
local SAY_EVADE = 1
local SAY_RESPAWN = 2
local SAY_RANDOM = 3

local ENTRY_DREKTHAR = 11946

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function onWhirlwind(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "whirlwind", 500, function() onWhirlwind(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND)
    end
    schedule(guid, "whirlwind", {8000, 18000}, function() onWhirlwind(creature, guid) end)
end

local function onWhirlwind2(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "whirlwind2", 500, function() onWhirlwind2(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND2)
    end
    schedule(guid, "whirlwind2", {7000, 25000}, function() onWhirlwind2(creature, guid) end)
end

local function onKnockdown(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "knockdown", 500, function() onKnockdown(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKDOWN)
    end
    schedule(guid, "knockdown", {10000, 15000}, function() onKnockdown(creature, guid) end)
end

local function onFrenzy(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function() onFrenzy(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRENZY)
    end
    schedule(guid, "frenzy", {20000, 30000}, function() onFrenzy(creature, guid) end)
end

local function onRandomYell(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "random_yell", 500, function() onRandomYell(creature, guid) end)
        return
    end
    creature:Talk(SAY_RANDOM)
    schedule(guid, "random_yell", {20000, 30000}, function() onRandomYell(creature, guid) end)
end

-- C++ boss_drektharAI::CheckInRoom (boss_drekthar.cpp:80-90): every
-- UpdateAI tick, 2D distance from home > 50 yd -> EnterEvadeMode +
-- Talk(SAY_EVADE), return false (skips events + melee that tick).
-- Distance twin: GetHomePosition + GetX/Y math, minus GetCombatReach
-- (WorldObject::GetDistance2d(x, y), Object.cpp:1193-1197), clamped
-- >= 0 — the balinda precedent. Lua has no per-tick hook, so a 1s
-- poll is the twin (documented delta: up to 1s late); no IsCasting
-- defer — the leash precedes the C++ casting gate (:99). The evade
-- fires ev2/ev23 (timers cancelled), so return without re-arming.
local function onCheckInRoom(creature, guid)
    local hx, hy = creature:GetHomePosition()
    local dx = creature:GetX() - hx
    local dy = creature:GetY() - hy
    local dist = math.sqrt(dx * dx + dy * dy) - creature:GetCombatReach()
    if dist < 0 then dist = 0 end
    if dist > 50.0 then
        creature:EnterEvadeMode()
        creature:Talk(SAY_EVADE)
        return
    end
    schedule(guid, "check_in_room", 1000, function() onCheckInRoom(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "check_in_room", 1000, function() onCheckInRoom(creature, guid) end)
    schedule(guid, "whirlwind", {1000, 20000}, function() onWhirlwind(creature, guid) end)
    schedule(guid, "whirlwind2", {1000, 20000}, function() onWhirlwind2(creature, guid) end)
    schedule(guid, "knockdown", 12000, function() onKnockdown(creature, guid) end)
    schedule(guid, "frenzy", 6000, function() onFrenzy(creature, guid) end)
    schedule(guid, "random_yell", {20000, 30000}, function() onRandomYell(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onSpawn(_, creature)
    creature:Talk(SAY_RESPAWN)
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_DREKTHAR, 5, onSpawn)
RegisterCreatureEvent(ENTRY_DREKTHAR, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_DREKTHAR, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_DREKTHAR, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_DREKTHAR, 23, onReset)
