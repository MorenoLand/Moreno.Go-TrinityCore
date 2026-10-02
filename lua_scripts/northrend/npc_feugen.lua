-- Feugen (Naxxramas) — Lua port of the npc_feugen slice of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (npc_feugen via GetNaxxramasAI<npc_feugenAI> (ScriptedAI);
-- AddSC_boss_thaddius registers it alongside boss_thaddius,
-- npc_stalagg, npc_tesla, the three polarity/magnetic-pull
-- SpellScripts, at_thaddius_entrance and achievement_thaddius_
-- shocking; loader decl 73 / call 268 per
-- northrend_script_loader.cpp — the ELEVENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gothik()).
-- Entry: 15930 Feugen (naxxramas.h NPC_FEUGEN line 99 —
-- kalecgos pass; instance_naxxramas.cpp binds NPC_FEUGEN ->
-- FeugenGUID (line 167) and DATA_FEUGEN -> FeugenGUID (line
-- 321); the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "npc_feugen"
-- hits boss_thaddius.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits. No feugen lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. Victim gating: C++ KilledUnit checks
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType()
-- (nalorakk / kelthuzad / gothik / thaddius player-gate precedent).
-- DoCast default is triggered=false: creature:CastSpell(creature,
-- spell) = DoCastSelf (phase_hunter / gluth precedent).
-- Ported arms (C++-exact for all modeled arms):
-- npc_feugen: JustEngagedWith Talk(SAY_FEUGEN_AGGRO 0) (event 1;
-- the thaddius->AI()->DoAction(ACTION_FEUGEN_AGGRO) cross-AI leg +
-- the stalagg AddThreat(who, 0.0f) leg have no bridge — documented,
-- not wired); UpdateAI staticFieldTimer — DoCastSelf(SPELL_FEUGEN_
-- STATICFIELD 28135), 6s init, 6s repeat; KilledUnit Talk(
-- SAY_FEUGEN_SLAY 1) (event 3 — C++-GATED: victim->GetTypeId() ==
-- TYPEID_PLAYER — nalorakk/kelthuzad player-gate variant). The
-- magneticPullTimer DoCastSelf(SPELL_MAGNETIC_PULL 54517) leg is
-- documented-only: the pull's whole effect lives in
-- spell_thaddius_magnetic_pull's OnCast HandleCast (threat-table
-- swap + SPELL_MAGNETIC_PULL_EFFECT 28337), and there is no
-- SpellScript binding bridge (razelikh precedent) — wiring the
-- orphaned cast would deviate from C++ (gluth's DoCast(28239/
-- 28404)-into-SpellScript documented-only precedent). All timers
-- cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- npc_feugen: InitializeAI — the tesla-coil GO SetGoState(GO_STATE_
-- ACTIVE) leg + timer inits — no GO-state bridge; the ctor
-- SetBoundary(instance->GetBossBoundary(BOSS_THADDIUS)) — no
-- boundary bridge; EnterEvadeMode — thaddius->AI()->DoAction(
-- ACTION_FEUGEN_RESET) — no cross-AI bridge; BeginResetEncounter —
-- coil GO GO_STATE_READY + DespawnOrUnsummon(0s, 7_days) — no
-- GO/despawn bridges; DoAction(ACTION_BEGIN_RESET_ENCOUNTER) —
-- routes to BeginResetEncounter — no bridge; DoAction(
-- ACTION_FEUGEN_REVIVED) — the full revive machine (SetFullHealth
-- + STAND + REACT_AGGRESSIVE + NOT_SELECTABLE removal + root
-- release + Talk(EMOTE_FEIGN_REVIVE 4) + refreshBeam + the stalagg
-- AddThreat victim-steal leg + timer re-init) — no stand-state /
-- react-state / flag / cross-AI bridges; DoAction(
-- ACTION_TRANSITION) — KillSelf + the coil CastStop + coil->AI()->
-- Talk(EMOTE_TESLA_OVERLOAD 1) — no cross-AI bridge; DoAction(
-- ACTION_TRANSITION_2) — coil->CastSpell(thaddius, SPELL_SHOCK_
-- VISUAL 28159) via DATA_THADDIUS — no cross-AI bridge; DoAction(
-- ACTION_TRANSITION_3) — coil GO GO_STATE_READY +
-- DespawnOrUnsummon(0s, 7_days) — no bridges; DamageTaken — the
-- feign-death machine (EMOTE_FEIGN_DEATH 3 Talk + thaddius->
-- AI()->DoAction(ACTION_FEUGEN_DIED) + NOT_SELECTABLE + auras
-- wipe + REACT_PASSIVE + root + STAND_DEAD + damage = health-1,
-- and the isFeignDeath damage = 0 arm) — no damage / stand-state /
-- react-state / cross-AI bridges; SpellHit — the whole tesla-beam
-- machine (SPELL_FEUGEN_TESLA_PERIODIC 28110 -> overload state +
-- creatureCaster Talk(EMOTE_TESLA_LINK_BREAKS 0) + CastSpell(target,
-- SPELL_TESLA_SHOCK 28099, triggered) / the refresh-beam recast of
-- SPELL_FEUGEN_CHAIN_VISUAL 28111) — SpellHit never fires in the
-- Lua surface (SpellHit ruling); UpdateAI's isFeignDeath early-
-- return — no bridge.

local ENTRY_FEUGEN = 15930

local SAY_FEUGEN_AGGRO = 0
local SAY_FEUGEN_SLAY = 1

local SPELL_FEUGEN_STATICFIELD = 28135

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

-- C++ UpdateAI staticFieldTimer: DoCastSelf(SPELL_FEUGEN_STATICFIELD
-- 28135), 6s init, 6s repeat. The isFeignDeath early-return has no
-- bridge (the feign-death machine is unmodeled) — documented, not
-- wired.
local function staticFieldTick(creature, guid)
    creature:CastSpell(creature, SPELL_FEUGEN_STATICFIELD)
    schedule(guid, "staticfield", 6000, function()
        staticFieldTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_FEUGEN_AGGRO 0) + the static-field
-- schedule (6s, from InitializeAI). The magneticPullTimer (20s)
-- leg is documented-only (see header). The thaddius cross-AI
-- DoAction(ACTION_FEUGEN_AGGRO) + stalagg AddThreat legs have no
-- bridge — documented, not wired.
local function feugenEnterCombat(event, creature, target)
    creature:Talk(SAY_FEUGEN_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "staticfield", 6000, function()
        staticFieldTick(creature, guid)
    end)
end

local function feugenLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_FEUGEN_SLAY 1) only when victim->
-- GetTypeId() == TYPEID_PLAYER (nalorakk / kelthuzad player-gate
-- variant).
local function feugenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_FEUGEN_SLAY)
    end
end

local function feugenDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function feugenReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FEUGEN, 1, feugenEnterCombat)
RegisterCreatureEvent(ENTRY_FEUGEN, 2, feugenLeaveCombat)
RegisterCreatureEvent(ENTRY_FEUGEN, 3, feugenTargetDied)
RegisterCreatureEvent(ENTRY_FEUGEN, 4, feugenDied)
RegisterCreatureEvent(ENTRY_FEUGEN, 23, feugenReset)
