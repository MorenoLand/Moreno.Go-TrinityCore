-- Stalagg (Naxxramas) — Lua port of the npc_stalagg slice of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (npc_stalagg via GetNaxxramasAI<npc_stalaggAI> (ScriptedAI);
-- AddSC_boss_thaddius registers it alongside boss_thaddius,
-- npc_feugen, npc_tesla, the three polarity/magnetic-pull
-- SpellScripts, at_thaddius_entrance and achievement_thaddius_
-- shocking; loader decl 73 / call 268 per
-- northrend_script_loader.cpp — the ELEVENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gothik()).
-- Entry: 15929 Stalagg (naxxramas.h NPC_STALAGG line 100 —
-- kalecgos pass; instance_naxxramas.cpp binds NPC_STALAGG ->
-- StalaggGUID (line 170) and DATA_STALAGG -> StalaggGUID
-- (line 323); the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "npc_stalagg"
-- hits boss_thaddius.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits. No stalagg lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. Victim gating: C++ KilledUnit checks
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType()
-- (nalorakk / kelthuzad / gothik / thaddius player-gate precedent).
-- DoCast default is triggered=false: creature:CastSpell(creature,
-- spell) = DoCastSelf (phase_hunter / gluth precedent).
-- Ported arms (C++-exact for all modeled arms):
-- npc_stalagg: JustEngagedWith Talk(SAY_STALAGG_AGGRO 0) (event 1;
-- the thaddius->AI()->DoAction(ACTION_STALAGG_AGGRO) cross-AI leg +
-- the feugen AddThreat(who, 0.0f) leg have no bridge — documented,
-- not wired); UpdateAI powerSurgeTimer — DoCastSelf(SPELL_STALAGG_
-- POWERSURGE 28134), 10s init, urandms(25,30) repeat (math.random
-- 25-30 per amanitar/black_knight/gluth precedent); KilledUnit
-- Talk(SAY_STALAGG_SLAY 1) (event 3 — C++-GATED: victim->
-- GetTypeId() == TYPEID_PLAYER — nalorakk/kelthuzad player-gate
-- variant). All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- npc_stalagg: InitializeAI — the tesla-coil GO SetGoState(GO_STATE_
-- ACTIVE) leg + powerSurgeTimer init — no GO-state bridge; the ctor
-- SetBoundary(instance->GetBossBoundary(BOSS_THADDIUS)) — no
-- boundary bridge; EnterEvadeMode — thaddius->AI()->DoAction(
-- ACTION_STALAGG_RESET) — no cross-AI bridge; BeginResetEncounter —
-- coil GO GO_STATE_READY + DespawnOrUnsummon(0s, 7_days) — no
-- GO/despawn bridges; DoAction(ACTION_BEGIN_RESET_ENCOUNTER) —
-- routes to BeginResetEncounter — no bridge; DoAction(
-- ACTION_STALAGG_REVIVED) — the full revive machine (SetFullHealth
-- + STAND + REACT_AGGRESSIVE + NOT_SELECTABLE removal + root
-- release + Talk(EMOTE_FEIGN_REVIVE 4) + refreshBeam + DoZoneInCombat
-- + the !IsEngaged() -> BeginResetEncounter leg) — no
-- stand-state / react-state / flag / zone-combat bridges;
-- DoAction(ACTION_TRANSITION) — KillSelf + the coil CastStop +
-- coil->AI()->Talk(EMOTE_TESLA_OVERLOAD 1) — no cross-AI bridge;
-- DoAction(ACTION_TRANSITION_2) — coil->CastSpell(thaddius,
-- SPELL_SHOCK_VISUAL 28159) via DATA_THADDIUS — no cross-AI bridge;
-- DoAction(ACTION_TRANSITION_3) — coil GO GO_STATE_READY +
-- DespawnOrUnsummon(0s, 7_days) — no bridges; DamageTaken — the
-- feign-death machine (EMOTE_FEIGN_DEATH 3 Talk + thaddius->
-- AI()->DoAction(ACTION_STALAGG_DIED) + NOT_SELECTABLE + auras
-- wipe + REACT_PASSIVE + root + STAND_DEAD + damage = health-1,
-- and the isFeignDeath damage = 0 arm) — no damage / stand-state /
-- react-state / cross-AI bridges; SpellHit — the whole tesla-beam
-- machine (SPELL_STALAGG_TESLA_PERIODIC 28098 -> overload state +
-- creatureCaster Talk(EMOTE_TESLA_LINK_BREAKS 0) + CastSpell(target,
-- SPELL_TESLA_SHOCK 28099, triggered) / the refresh-beam recast of
-- SPELL_STALAGG_CHAIN_VISUAL 28096) — SpellHit never fires in the
-- Lua surface (SpellHit ruling); UpdateAI's isFeignDeath gate —
-- no bridge.

local ENTRY_STALAGG = 15929

local SAY_STALAGG_AGGRO = 0
local SAY_STALAGG_SLAY = 1

local SPELL_STALAGG_POWERSURGE = 28134

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

-- C++ UpdateAI powerSurgeTimer: DoCastSelf(SPELL_STALAGG_POWERSURGE
-- 28134), 10s init, urandms(25,30) repeat. The isFeignDeath delay
-- leg has no bridge (the feign-death machine is unmodeled) —
-- documented, not wired.
local function powerSurgeTick(creature, guid)
    creature:CastSpell(creature, SPELL_STALAGG_POWERSURGE)
    schedule(guid, "powersurge", math.random(25, 30) * 1000, function()
        powerSurgeTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_STALAGG_AGGRO 0) + the powerSurge
-- schedule (10s, from InitializeAI). The thaddius cross-AI
-- DoAction(ACTION_STALAGG_AGGRO) + feugen AddThreat legs have no
-- bridge — documented, not wired.
local function stalaggEnterCombat(event, creature, target)
    creature:Talk(SAY_STALAGG_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "powersurge", 10000, function()
        powerSurgeTick(creature, guid)
    end)
end

local function stalaggLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_STALAGG_SLAY 1) only when victim->
-- GetTypeId() == TYPEID_PLAYER (nalorakk / kelthuzad player-gate
-- variant).
local function stalaggTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_STALAGG_SLAY)
    end
end

local function stalaggDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function stalaggReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_STALAGG, 1, stalaggEnterCombat)
RegisterCreatureEvent(ENTRY_STALAGG, 2, stalaggLeaveCombat)
RegisterCreatureEvent(ENTRY_STALAGG, 3, stalaggTargetDied)
RegisterCreatureEvent(ENTRY_STALAGG, 4, stalaggDied)
RegisterCreatureEvent(ENTRY_STALAGG, 23, stalaggReset)
