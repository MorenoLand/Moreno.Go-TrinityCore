-- Gluth (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_gluth.cpp
-- (boss_gluth (BossAI, BOSS_GLUTH); spell_gluth_decimate +
-- spell_gluth_zombiechow_search (SpellScripts via SpellScriptLoader);
-- npc_zombie_chow (ScriptedAI); AddSC_boss_gluth registers all four;
-- loader decl 67 / call 262 per northrend_script_loader.cpp — the
-- NINTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_noth(), under the "// Naxxramas" marker; the call after
-- it is AddSC_boss_gothik() (decl 72 / call 267)).
-- Entry: 15932 Gluth (naxxramas.h NPC_GLUTH — kalecgos pass;
-- instance_naxxramas.cpp binds NPC_GLUTH -> GluthGUID and DATA_GLUTH
-- -> GluthGUID; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the four
-- script names hits boss_gluth.cpp only (loader carries only the
-- decl/call lines); zero sql/ hits. No gluth lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent),
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ randtime -> math.random (amanitar
-- / boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_gluth: EVENT_WOUND — DoCastVictim(Mortal Wound 54378), 10s
-- init, 10s repeat (moroes precedent); the STATE_GLUTH_EATING
-- deferral arm (events.Repeat(3s)) has no state-machine bridge —
-- documented, not wired. EVENT_ENRAGE — Talk(EMOTE_ENRAGE 2) +
-- DoCastSelf(Enrage 28371), randtime(16s,22s) init, randtime(16s,22s)
-- repeat (phase_hunter self-cast precedent); the eating-state 5s
-- deferral arm has no state bridge — documented, not wired.
-- EVENT_BERSERK — Talk(EMOTE_BERSERKER 4) + DoCastSelf(Berserk
-- 26662), 8min init, 5min repeat. JustEngagedWith schedules the three
-- ported events (BossAI::JustEngagedWith instance-bookkeeping leg has
-- no bridge — tharon_ja precedent); no aggro Talk in C++.
-- EMOTE_DECIMATE (1), EMOTE_SPOTS_ONE (0), EMOTE_DEVOURS_ALL (3)
-- ride unported legs (DoCastAOE / summon / motion-master) — not
-- ported orphaned (vortex precedent). No KilledUnit Talk, no
-- JustDied Talk in C++ for gluth. All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — _Reset + SetReactState(REACT_AGGRESSIVE) + SetSpeed(
-- MOVE_RUN, 12.0f) — no react-state / speed bridges; SummonedCreatureDies
-- — summons.Despawn — no summon bridge; EVENT_DECIMATE — Talk(
-- EMOTE_DECIMATE 1) + DoCastAOE(Decimate 28374) + the 20x
-- EVENT_SEARCH_ZOMBIE_MULTI scheduling machine — no DoCastAOE bridge
-- (terestian/shazzrah precedent) and no summon/zombie-list bridge;
-- EVENT_SUMMON — SummonCreatureGroup(SUMMON_GROUP_CHOW_10MAN/
-- SUMMON_GROUP_CHOW_25MAN) — no summon bridge + Is25ManRaid difficulty
-- gate (kelidan precedent); EVENT_SEARCH_ZOMBIE_SINGLE — summons
-- list iteration + cross-AI SetData(TOBE_EATEN) + REACT_PASSIVE +
-- AttackStop + MoveCloserAndStop — no summon / cross-AI / react-state
-- / motion bridges; EVENT_KILL_ZOMBIE_SINGLE — DoCast(ZOMBIE_CHOW_
-- SEARCH_SINGLE 28239) + react-state / speed restore legs — no
-- bridges; EVENT_SEARCH_ZOMBIE_MULTI — DoCastAOE(ZOMBIE_CHOW_SEARCH_
-- MULTI 28404) — no DoCastAOE bridge; DoAction(ACTION_DECIMATE_EVENT)
-- — cross-AI SetData(DECIMATED) over summons — no bridges; MovementInform
-- — MoveIdle — no motion bridge; the whole zombie state machine
-- (STATE_GLUTH_EATING / zombie GUID persistence) — no state bridge.
-- spell_gluth_decimate (28374/54426): HandleScriptEffect -> Decimate
-- damage 28375 via CastSpellBP0 + HandleEvent -> DoAction(
-- ACTION_DECIMATE_EVENT) — no SpellScript binding bridge (razelikh
-- precedent) — joins the SpellScript/AuraScript queue.
-- spell_gluth_zombiechow_search (28239/28404): AfterHit -> 5%-max-
-- health heal — same, SpellScript queue.
-- npc_zombie_chow (ScriptedAI — entries 16360/30303 appear only in a
-- C++ comment; no NPC_ constants in naxxramas.h, no instance
-- bindings — entry-unverifiable from C++ evidence — no registration):
-- ctor DoCastSelf(Infected Wound 29307) — port-pattern-ready in
-- isolation (phase_hunter self-cast precedent) but entry-blocked —
-- joins the bridgeable-but-entry-blocked queue; the DATA_GLUTH GUID
-- leg, decimated-state MovePoint/StopMoving walk-leg, to-be-eaten
-- mechanic-immune leg (IMMUNITY_MECHANIC/GRIP) — no instance / motion
-- / react-state / immunity bridges.

local ENTRY_GLUTH = 15932

local EMOTE_ENRAGE = 2
local EMOTE_BERSERKER = 4

local SPELL_MORTAL_WOUND = 54378
local SPELL_ENRAGE = 28371
local SPELL_BERSERK = 26662

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

-- C++ EVENT_WOUND: DoCastVictim(Mortal Wound 54378), 10s init, 10s
-- repeat.
local function woundTick(creature, guid)
    creature:CastSpell(nil, SPELL_MORTAL_WOUND)
    schedule(guid, "wound", 10000, function()
        woundTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE: Talk(EMOTE_ENRAGE 2) + DoCastSelf(Enrage 28371),
-- randtime(16s,22s) init, randtime(16s,22s) repeat.
local function enrageTick(creature, guid)
    creature:Talk(EMOTE_ENRAGE)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", math.random(16, 22) * 1000, function()
        enrageTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(EMOTE_BERSERKER 4) + DoCastSelf(Berserk
-- 26662), 8min init, 5min repeat (hard-enrage buff refresh).
local function berserkTick(creature, guid)
    creature:Talk(EMOTE_BERSERKER)
    creature:CastSpell(creature, SPELL_BERSERK)
    schedule(guid, "berserk", 300000, function()
        berserkTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: schedules EVENT_WOUND (10s), EVENT_ENRAGE
-- (randtime 16s,22s), EVENT_BERSERK (8min). No aggro Talk in C++.
local function gluthEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "wound", 10000, function()
        woundTick(creature, guid)
    end)
    schedule(guid, "enrage", math.random(16, 22) * 1000, function()
        enrageTick(creature, guid)
    end)
    schedule(guid, "berserk", 480000, function()
        berserkTick(creature, guid)
    end)
end

local function gluthLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function gluthDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function gluthReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GLUTH, 1, gluthEnterCombat)
RegisterCreatureEvent(ENTRY_GLUTH, 2, gluthLeaveCombat)
RegisterCreatureEvent(ENTRY_GLUTH, 4, gluthDied)
RegisterCreatureEvent(ENTRY_GLUTH, 23, gluthReset)
