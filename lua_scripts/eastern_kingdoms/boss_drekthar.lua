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
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally and the in-loop
-- casting-state gate and the per-event in-loop re-check are dropped
-- (maiden precedent).
-- Unmodeled: CheckInRoom (home-position 2D distance > 50yd ->
-- EnterEvadeMode + Talk(SAY_EVADE=1)) — no HomePosition bridge on the
-- luaMotionCreature surface (X/Y/Z reads only; GetDistance2d needs two
-- objects — av_marshal / balinda precedent); evade still re-arms
-- through OnReset(23) when the engine leashes the creature.
-- SPELL_SWEEPING_STRIKES=18765, SPELL_CLEAVE=20677,
-- SPELL_WINDFURY=35886, SPELL_STORMPIKE=51876 are declared in the C++
-- Spells enum (all marked "not sure") but never used by this AI.

local SPELL_WHIRLWIND = 15589
local SPELL_WHIRLWIND2 = 13736
local SPELL_KNOCKDOWN = 19128
local SPELL_FRENZY = 8269

local SAY_AGGRO = 0
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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onWhirlwind(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND)
    end
    schedule(guid, "whirlwind", {8000, 18000}, function() onWhirlwind(creature, guid) end)
end

local function onWhirlwind2(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND2)
    end
    schedule(guid, "whirlwind2", {7000, 25000}, function() onWhirlwind2(creature, guid) end)
end

local function onKnockdown(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKDOWN)
    end
    schedule(guid, "knockdown", {10000, 15000}, function() onKnockdown(creature, guid) end)
end

local function onFrenzy(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRENZY)
    end
    schedule(guid, "frenzy", {20000, 30000}, function() onFrenzy(creature, guid) end)
end

local function onRandomYell(creature, guid)
    creature:Talk(SAY_RANDOM)
    schedule(guid, "random_yell", {20000, 30000}, function() onRandomYell(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "whirlwind", {1000, 20000}, function() onWhirlwind(creature, guid) end)
    schedule(guid, "whirlwind2", {1000, 20000}, function() onWhirlwind2(creature, guid) end)
    schedule(guid, "knockdown", 12000, function() onKnockdown(creature, guid) end)
    schedule(guid, "frenzy", 6000, function() onFrenzy(creature, guid) end)
    schedule(guid, "random_yell", {20000, 30000}, function() onRandomYell(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onSpawn(_, creature)
    creature:Talk(SAY_RESPAWN)
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_DREKTHAR, 5, onSpawn)
RegisterCreatureEvent(ENTRY_DREKTHAR, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_DREKTHAR, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_DREKTHAR, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_DREKTHAR, 23, onReset)
