-- Warchief Rend Blackhand (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_rend_blackhand.cpp
-- (boss_rend_blackhand — BossAI combat scheduler). Boss-script unit
-- per eastern_kingdoms_script_loader.cpp order (boss_gyth done,
-- registered for 10339; AddSC_boss_rend_blackhand next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h:70 names
-- NPC_WARCHIEF_REND_BLACKHAND = 10429 in the BRS creatures enum; the
-- AI runs under BossAI(DATA_WARCHIEF_REND_BLACKHAND = 10). The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (gyth / pyroguard_emberseer precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h). SetBossState legs (BossAI DATA_WARCHIEF_REND_
-- BLACKHAND = 10, blackrock_spire.h:40; < EncounterCount 23):
-- ev1 IN_PROGRESS via base BossAI::JustEngagedWith (boss_rend_
-- blackhand.cpp:186, base call first; _JustEngagedWith writes
-- IN_PROGRESS at ScriptedCreature.cpp:537); ev23 NOT_STARTED via
-- Reset's _Reset() (:176-182; _Reset gates on alive + state != DONE,
-- ScriptedCreature.cpp:499-511) — Reset's gythEvent/victorGUID/
-- portcullisGUID clears are pre-fight state, unmodeled; ev4 DONE via
-- JustDied's _JustDied() (:198-203; ScriptedCreature.cpp:513-520) —
-- JustDied's victor SetData(1, 2) arm stays documented-only below.
-- Bridge live (lua_creature_events.go:5917; gyth convention).
-- SPELL_FRENZY (8269) and SPELL_KNOCKDOWN (13360) are defined in the
-- file's spell enum but never cast by the AI — omitted by design
-- (shadowvosh ice-armor precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_WHIRLWIND: self-cast WHIRLWIND 13736, {13s,15s} init ->
-- {13s,18s} loop (C++ ScheduleEvent(EVENT_WHIRLWIND, 13s, 15s) init /
-- (EVENT_WHIRLWIND, 13s, 18s) loop, inclusive; CreateLuaEvent {min,max}
-- table is inclusive).
-- EVENT_CLEAVE: victim-cast CLEAVE 15284, {15s,17s} init ->
-- {10s,14s} loop (C++ inclusive ranges; CreateLuaEvent {min,max}).
-- EVENT_MORTAL_STRIKE: victim-cast MORTAL_STRIKE 16856, {17s,19s}
-- init -> {14s,16s} loop (C++ inclusive ranges; CreateLuaEvent
-- {min,max}). Victim casts are GetVictim nil-guarded (incarcerator
-- convention).
-- CASTING gates ported (combat branch only): C++ UpdateAI pre-loop
-- gate (:422) + in-loop gate (:443) are a 500ms IsCasting defer under
-- each arm's own key (live, lua_creature_events.go:1950; mal_ganis
-- idiom) -- a gated arm fires nothing and consumes no re-arm. The
-- gyth-event pre-fight branch has no gate and stays unmodeled. The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).
-- Documented-only (no bridges on the Lua surface): the whole
-- pre-fight gyth event chain gates on FindNearestCreature(
-- NPC_LORD_VICTOR_NEFARIUS) + ObjectAccessor::GetCreature(victorGUID),
-- FindNearestGameObject(GO_DR_PORTCULLIS) + GetGameObject(
-- portcullisGUID), victor SetFacingTo / emotes / Talk, MotionMaster
-- MovePath(REND_PATH_1 / NEFARIUS_PATH_1), me->NearTeleportTo x2,
-- SummonCreature(NPC_GYTH 10429-era Gyth 10339), and the area-trigger
-- SetData arm itself (the_beast / gyth precedent). The wave tables
-- are fully commented out in C++ (EVENT_WAVE_1..6 bodies are
-- no-ops aside from the portcullis UseDoorOrButton), so nothing is
-- lost by the absence of a spawn bridge. JustDied's
-- FindNearestCreature(NPC_LORD_VICTOR_NEFARIUS)->AI()->SetData(1, 2)
-- (creature-list precedent). IsSummonedBy's SetImmuneToPC(false) +
-- DoZoneInCombat() bridge live (2026-10-23) — but the summon (IsSummonedBy)
-- is unmodeled (the Nefarius summon chain is documented-only); porting to
-- OnSpawn(5) would fire it on natural respawns, which C++ never does.
-- MovementInform's despawn path gates on MotionMaster /
-- DespawnOrUnsummon (unmodeled).

local SPELL_WHIRLWIND = 13736
local SPELL_CLEAVE = 15284
local SPELL_MORTAL_STRIKE = 16856

local ENTRY_WARCHIEF_REND_BLACKHAND = 10429

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
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", {13000, 18000}, function() onWhirlwind(creature, guid) end)
end

local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onCleave(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {10000, 14000}, function() onCleave(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mortalstrike", 500, function() onMortalStrike(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
    end
    schedule(guid, "mortalstrike", {14000, 16000}, function() onMortalStrike(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_rend_blackhand.cpp:186 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_rend_blackhand.cpp:186 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_WARCHIEF_REND_BLACKHAND=10 (blackrock_spire.h:40).
    creature:SetBossState(10, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "whirlwind", {13000, 15000}, function() onWhirlwind(creature, guid) end)
    schedule(guid, "cleave", {15000, 17000}, function() onCleave(creature, guid) end)
    schedule(guid, "mortalstrike", {17000, 19000}, function() onMortalStrike(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied (boss_rend_blackhand.cpp:198-203): _JustDied()
-- bookkeeping (timer cancel) + SetBossState(DATA_WARCHIEF_REND_
-- BLACKHAND=10, DONE). The victor SetData(1, 2) arm stays unmodeled.
local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(10, 3)
end

-- C++ Reset (boss_rend_blackhand.cpp:176-182): _Reset() first, then
-- the gythEvent/victorGUID/portcullisGUID clears (unmodeled pre-fight
-- state). The _Reset() leg is alive-gated (Go fires ev23 on the death
-- path too) and skipped when already DONE (ScriptedCreature.cpp:499-511).
local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsAlive() and creature:GetBossState(10) ~= 3 then
        creature:SetBossState(10, 0)
    end
end

RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 4, onDied)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 23, onReset)
