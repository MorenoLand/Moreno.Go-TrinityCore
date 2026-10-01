-- Captain Galvangar (Alterac Valley) --
-- Lua port of src/server/scripts/EasternKingdoms/AlteracValley/
-- boss_galvangar.cpp (boss_galvangarAI — 5-event combat scheduler +
-- CheckInRoom home leash + ACTION_BUFF_YELL). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_galvangar follows
-- AddSC_boss_drekthar; the Alterac Valley boss set continues with
-- vanndar).
-- Entry (verifiable from the C++ sources): 11947 "Captain Galvangar" in
-- the BattlegroundAV.h AV creature lists (line 1183 spawn data,
-- lines 1267/1300; neighbors 11946/11948/11949 are Drek'Thar, Vanndar,
-- Balinda — the other AV boss-set members). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace), but the
-- entry itself is C++-verifiable so the port is registered
-- (storm_cloud / av_marshal / balinda / drekthar precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_CLEAVE: victim-cast CLEAVE 15284, {1,9}s init -> {10,16}s;
-- EVENT_FRIGHTENING_SHOUT: victim-cast FRIGHTENING_SHOUT 19134 (cast
--   on victim in C++), {2,19}s init -> {10,15}s;
-- EVENT_WHIRLWIND1: victim-cast WHIRLWIND1 15589, {1,13}s init ->
--   {6,10}s;
-- EVENT_WHIRLWIND2: victim-cast WHIRLWIND2 13736, {5,20}s init ->
--   {10,25}s;
-- EVENT_MORTAL_STRIKE: victim-cast MORTAL_STRIKE 16856, {5,20}s init
--   -> {10,30}s;
-- JustEngagedWith Talk(SAY_AGGRO=0).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and the
-- in-loop casting-state gate and the per-event in-loop re-check are
-- dropped (maiden precedent).
-- Unmodeled: CheckInRoom (home-position 2D distance > 50yd ->
-- EnterEvadeMode + Talk(SAY_EVADE=1)) — no HomePosition bridge on the
-- luaMotionCreature surface (X/Y/Z reads only; GetDistance2d needs two
-- objects — av_marshal / balinda / drekthar precedent); evade still
-- re-arms through OnReset(23) when the engine leashes the creature.
-- Unmodeled: DoAction(ACTION_BUFF_YELL=-30001) -> Talk(SAY_BUFF=2) —
-- driven by the BattlegroundAV script; no Battleground-to-Lua
-- DoAction dispatch bridge (balinda precedent).

local SPELL_CLEAVE = 15284
local SPELL_FRIGHTENING_SHOUT = 19134
local SPELL_WHIRLWIND1 = 15589
local SPELL_WHIRLWIND2 = 13736
local SPELL_MORTAL_STRIKE = 16856

local SAY_AGGRO = 0

local ENTRY_GALVANGAR = 11947

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

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {10000, 16000}, function() onCleave(creature, guid) end)
end

local function onFrighteningShout(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRIGHTENING_SHOUT)
    end
    schedule(guid, "frightening_shout", {10000, 15000}, function() onFrighteningShout(creature, guid) end)
end

local function onWhirlwind1(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND1)
    end
    schedule(guid, "whirlwind1", {6000, 10000}, function() onWhirlwind1(creature, guid) end)
end

local function onWhirlwind2(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND2)
    end
    schedule(guid, "whirlwind2", {10000, 25000}, function() onWhirlwind2(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
    end
    schedule(guid, "mortal_strike", {10000, 30000}, function() onMortalStrike(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "cleave", {1000, 9000}, function() onCleave(creature, guid) end)
    schedule(guid, "frightening_shout", {2000, 19000}, function() onFrighteningShout(creature, guid) end)
    schedule(guid, "whirlwind1", {1000, 13000}, function() onWhirlwind1(creature, guid) end)
    schedule(guid, "whirlwind2", {5000, 20000}, function() onWhirlwind2(creature, guid) end)
    schedule(guid, "mortal_strike", {5000, 20000}, function() onMortalStrike(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GALVANGAR, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_GALVANGAR, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_GALVANGAR, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_GALVANGAR, 23, onReset)
