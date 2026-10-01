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
-- (blackrock_spire.h); the combat arms have no instance state.
-- Reset() _Reset() / JustDied _JustDied() are internal machinery
-- covered here by the cancel on combat events (halycon precedent).
-- SPELL_FRENZY (8269) and SPELL_KNOCKDOWN (13360) are defined in the
-- file's spell enum but never cast by the AI — omitted by design
-- (shadowvosh ice-armor precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_WHIRLWIND: self-cast WHIRLWIND 13736, 13s init -> 13s loop
-- (C++ urand(13s,15s) init / urand(13s,18s) loop — ranges use the
-- lower bound, halycon recurring-timer convention).
-- EVENT_CLEAVE: victim-cast CLEAVE 15284, 15s init -> 10s loop
-- (C++ urand(15s,17s) init / urand(10s,14s) loop — lower bound).
-- EVENT_MORTAL_STRIKE: victim-cast MORTAL_STRIKE 16856, 17s init ->
-- 14s loop (C++ urand(17s,19s) init / urand(14s,16s) loop — lower
-- bound). Victim casts are GetVictim nil-guarded (incarcerator
-- convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
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
-- DoZoneInCombat() has no spawn bridge (emberseer JustAppeared
-- precedent). MovementInform's despawn path gates on MotionMaster /
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
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", 13000, function() onWhirlwind(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 10000, function() onCleave(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
    end
    schedule(guid, "mortalstrike", 14000, function() onMortalStrike(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "whirlwind", 13000, function() onWhirlwind(creature, guid) end)
    schedule(guid, "cleave", 15000, function() onCleave(creature, guid) end)
    schedule(guid, "mortalstrike", 17000, function() onMortalStrike(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 23, onCombatEnd)
