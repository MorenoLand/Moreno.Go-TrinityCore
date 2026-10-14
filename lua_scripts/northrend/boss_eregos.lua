-- Ley-Guardian Eregos (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_eregos.cpp
-- (boss_eregos (CreatureScript) via GetOculusAI<boss_eregosAI>
-- (BossAI, DATA_EREGOS); spell_eregos_planar_shift
-- (SpellScriptLoader -> AuraScript); achievement_gen_eregos_void
-- x3 (AchievementCriteriaScript: achievement_ruby_void /
-- achievement_emerald_void / achievement_amber_void); all
-- registered from inside AddSC_boss_eregos(), no separate loader
-- lines; loader decl 87 / call 282 per
-- northrend_script_loader.cpp — the FOURTH group of the
-- "// The Nexus: The Oculus" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_varos() (decl 86 / call 281);
-- the calls after it are AddSC_instance_oculus() (decl 88 /
-- call 283), AddSC_oculus() (decl 89 / call 284) — loader order
-- confirmed this run; the checkpoint sequence (varos ->
-- eregos) is followed).
-- Entry: 27656 Ley-Guardian Eregos (oculus.h NPC_EREGOS, line 44;
-- DATA_EREGOS = 3, line 34; instance_oculus.cpp OnCreatureCreate
-- binds case NPC_EREGOS (line 72) and GetGuidData cases
-- DATA_EREGOS (lines 209 / 249) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_eregos"
-- hits boss_eregos.cpp + the loader decl/call lines only; grep
-- for "spell_eregos_planar_shift" hits boss_eregos.cpp only;
-- grep for "achievement_gen_eregos_void" /
-- "achievement_ruby_void" / "achievement_emerald_void" /
-- "achievement_amber_void" hits boss_eregos.cpp only; zero sql/
-- hits. No eregos lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Driven by CreateLuaEvent
-- timers; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 1) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg has no
-- bridge — tharon_ja precedent; the drake-vehicle FindNearest-
-- Creature achievement-void legs (NPC_RUBY_DRAKE_VEHICLE 27756 /
-- NPC_EMERALD_DRAKE_VEHICLE 27692 / NPC_AMBER_DRAKE_VEHICLE
-- 27755, 500.0f) have no instance / creature-search bridge —
-- documented-only; GetData for the void booleans is consumed by
-- the achievement scripts below, also documented-only).
-- KilledUnit — Talk(SAY_KILL 3) (event 3 — C++-GATED
-- who->GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad
-- player-gate variant — tenth ported variant overall, seventh
-- identical to nalorakk / kelthuzad / gothik / thaddius /
-- keristrasza / urom).
-- JustDied — Talk(SAY_DEATH 4) (event 4; the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (_phase = PHASE_NORMAL, all three void
-- booleans = true) + _Reset() + DoAction(
-- ACTION_SET_NORMAL_EVENTS) — no instance bridge.
-- DoAction(ACTION_SET_NORMAL_EVENTS) — events.SetPhase(
-- PHASE_NORMAL) + ScheduleEvent x4 (EVENT_ARCANE_BARRAGE
-- 3s/10s, EVENT_ARCANE_VOLLEY 10s/25s, EVENT_ENRAGED_ASSAULT
-- 35s/50s, EVENT_SUMMON_LEY_WHELP 15s/30s, all PHASE_NORMAL)
-- (boss_eregos.cpp:148-157; seeded from Reset() at :107) —
-- the timer bridge is CreateLuaEvent (the stale "no
-- timer-event / phase bridge" claim is retracted); the ports
-- seed from OnEnterCombat(1) and cancel on 2/4/23 per the
-- krystallus/bjarngrim/halion convention. The PHASE_NORMAL
-- gating collapses to always-true: the planar-shift machine
-- that leaves NORMAL is unported, and the AuraScript-driven
-- re-arm of this DoAction (spell_eregos_planar_shift
-- AfterEffectRemove, :274-282) is unbridged.
-- UpdateAI — the arcane event machine (boss_eregos.cpp:204-
-- 239), PORTED arms (all non-triggered):
-- EVENT_ARCANE_BARRAGE DoCastVictim(SPELL_ARCANE_BARRAGE 50804),
-- 3s/10s init -> 3s/10s loop — PORTED as CreateLuaEvent
-- {3000, 10000} + CastSpell(nil, 50804).
-- EVENT_ARCANE_VOLLEY DoCastAOE(SPELL_ARCANE_VOLLEY 51153),
-- 10s/25s init -> 10s/25s loop — PORTED as CreateLuaEvent
-- {10000, 25000} + CastSpell(creature, 51153) (the established
-- DoCastAOE -> self-cast lowering, AGENTS.md 2026-10-13).
-- EVENT_ENRAGED_ASSAULT Talk(SAY_ENRAGE 2) +
-- DoCast(SPELL_ENRAGED_ASSAULT 51170) self-cast, 35s/50s init
-- -> 35s/50s loop — PORTED as CreateLuaEvent {35000,
-- 50000} + Talk(2) + CastSpell(creature, 51170).
-- EVENT_SUMMON_LEY_WHELP 3x DoCast(SPELL_SUMMON_LEY_WHELP
-- 51175) self-cast, 15s/30s init -> 15s/30s loop — PORTED
-- as CreateLuaEvent {15000, 30000} + 3x CastSpell(creature,
-- 51175); the C++ JustSummoned ignores ley whelps (:160-170
-- handles only planar anomalies), so the summon is
-- engine/spell-DB-delivered exactly like the C++ DoCast.
-- The UNIT_STATE_CASTING gates have no bridge (documented
-- deviation); DoMeleeAttackIfReady is engine-driven.
-- DamageTaken — heroic-only phase-shift machine: health
-- 60%..20% -> PHASE_FIRST_PLANAR (or < 20% ->
-- PHASE_SECOND_PLANAR), events.Reset() + Talk(SAY_SHIELD 5) +
-- DoCast(SPELL_PLANAR_SHIFT 51162) + summons.DespawnAll() +
-- DoCast(SPELL_PLANAR_ANOMALIES 57959) x6 — no health /
-- timer-event / cast / summon bridges.
-- JustSummoned — BossAI::JustSummoned + NPC_PLANAR_ANOMALY
-- 30879: CombatStop + REACT_PASSIVE + MoveRandom(100.0f) — no
-- motion / react-state bridges. SummonedCreatureDespawn —
-- NPC_PLANAR_ANOMALY: CastSpell(SPELL_PLANAR_BLAST 57976)
-- self — no cast bridge.
-- spell_eregos_planar_shift — AuraScript AfterEffectRemove
-- (EFFECT_0 SPELL_AURA_SCHOOL_IMMUNITY): target ToCreature
-- -> AI()->DoAction(ACTION_SET_NORMAL_EVENTS) (the
-- planar-shift end re-arms normal events) — no AuraScript
-- bridge (the keristrasza spell_intense_cold precedent); joins
-- the no-AuraScript-bridge queue.
-- achievement_gen_eregos_void x3 — OnCheck: target->GetAI()->
-- GetData(DATA_RUBY_VOID / DATA_EMERALD_VOID / DATA_AMBER_VOID)
-- (the drake-void achievements 2044 / 2045 / 2046) — no
-- achievement bridge (the kelthuzad / thaddius precedent);
-- joins the unmodeled-achievement queue.

local ENTRY_EREGOS = 27656

local SAY_AGGRO = 1
local SAY_KILL = 3
local SAY_DEATH = 4
local SAY_ENRAGE = 2

local SPELL_ARCANE_BARRAGE = 50804
local SPELL_ARCANE_VOLLEY = 51153
local SPELL_ENRAGED_ASSAULT = 51170
local SPELL_SUMMON_LEY_WHELP = 51175

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

-- C++ EVENT_ARCANE_BARRAGE (:215-218): DoCastVictim(50804)
-- non-triggered, 3s/10s init -> 3s/10s loop.
local function arcaneBarrageTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARCANE_BARRAGE)
    schedule(guid, "barrage", {3000, 10000}, function()
        arcaneBarrageTick(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_VOLLEY (:219-222): DoCastAOE(51153)
-- non-triggered, 10s/25s init -> 10s/25s loop.
local function arcaneVolleyTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_VOLLEY)
    schedule(guid, "volley", {10000, 25000}, function()
        arcaneVolleyTick(creature, guid)
    end)
end

-- C++ EVENT_ENRAGED_ASSAULT (:223-227): Talk(SAY_ENRAGE) +
-- DoCast(51170) self-cast, non-triggered, 35s/50s init ->
-- 35s/50s loop.
local function enragedAssaultTick(creature, guid)
    creature:Talk(SAY_ENRAGE)
    creature:CastSpell(creature, SPELL_ENRAGED_ASSAULT)
    schedule(guid, "enraged", {35000, 50000}, function()
        enragedAssaultTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_LEY_WHELP (:228-232): 3x DoCast(51175)
-- self-cast, non-triggered, 15s/30s init -> 15s/30s loop; the
-- summon is engine/spell-DB-delivered (C++ JustSummoned
-- ignores ley whelps).
local function leyWhelpTick(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_LEY_WHELP)
    creature:CastSpell(creature, SPELL_SUMMON_LEY_WHELP)
    creature:CastSpell(creature, SPELL_SUMMON_LEY_WHELP)
    schedule(guid, "whelps", {15000, 30000}, function()
        leyWhelpTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith(who) +
-- Talk(SAY_AGGRO) + drake-vehicle achievement-void legs (no
-- bridges) — the Talk arm is unconditional.
local function eregosEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "barrage", {3000, 10000}, function()
        arcaneBarrageTick(creature, guid)
    end)
    schedule(guid, "volley", {10000, 25000}, function()
        arcaneVolleyTick(creature, guid)
    end)
    schedule(guid, "enraged", {35000, 50000}, function()
        enragedAssaultTick(creature, guid)
    end)
    schedule(guid, "whelps", {15000, 30000}, function()
        leyWhelpTick(creature, guid)
    end)
end

local function eregosCombatEnd(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: gated on who->GetTypeId() == TYPEID_PLAYER —
-- the nalorakk / kelthuzad player-gate variant.
local function eregosTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) + _JustDied() (the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent).
local function eregosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_EREGOS, 1, eregosEnterCombat)
RegisterCreatureEvent(ENTRY_EREGOS, 2, eregosCombatEnd)
RegisterCreatureEvent(ENTRY_EREGOS, 3, eregosTargetDied)
RegisterCreatureEvent(ENTRY_EREGOS, 4, eregosDied)
RegisterCreatureEvent(ENTRY_EREGOS, 23, eregosCombatEnd)
