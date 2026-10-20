-- Stalagg (Naxxramas) — Lua port of the npc_stalagg slice of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (npc_stalagg via GetNaxxramasAI<npc_stalaggAI> (ScriptedAI);
-- AddSC_boss_thaddius registers it alongside boss_thaddius,
-- npc_feugen, npc_tesla, the three polarity/magnetic-pull
-- SpellScripts, at_thaddius_entrance and achievement_thaddius_
-- shocking; loader decl 73 / call 268 per
-- northrend_script_loader.cpp — the FIFTEENTH Naxxramas group in
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
-- zone-combat bridge (stand/react/flag bridges exist since the
-- 2026-10-20 standstate strip; the revive TRIGGER is the unbridged
-- thaddius DoAction machine);
-- DoAction(ACTION_TRANSITION) — KillSelf + the coil CastStop +
-- coil->AI()->Talk(EMOTE_TESLA_OVERLOAD 1) — no cross-AI bridge;
-- DoAction(ACTION_TRANSITION_2) — coil->CastSpell(thaddius,
-- SPELL_SHOCK_VISUAL 28159) via DATA_THADDIUS — no cross-AI bridge;
-- DoAction(ACTION_TRANSITION_3) — coil GO GO_STATE_READY +
-- DespawnOrUnsummon(0s, 7_days) — no bridges; DamageTaken — the
-- feign-death machine (EMOTE_FEIGN_DEATH 3 Talk + thaddius->
-- AI()->DoAction(ACTION_STALAGG_DIED) + NOT_SELECTABLE + auras
-- wipe + REACT_PASSIVE + root + STAND_DEAD + damage = health-1,
-- and the isFeignDeath damage = 0 arm): the Talk(3) + damage =
-- health-1 + damage = 0 legs ARE ported (see stalaggDamageTaken
-- below — event 9 numeric-return rewrite, halazzi precedent); the
-- cross-AI DoAction + SetControlled(root) legs stay out (the
-- stand/react/flag/aura legs ARE ported since the 2026-10-20
-- standstate strip); the latch clears on death (event 4) and ev23
-- restores fresh-spawn state (Go-lifecycle analog of the C++
-- despawn/respawn cycle — see stalaggReset); the
-- C++ ACTION_STALAGG_REVIVED revive machine is unbridged, so a
-- feigned stalagg stays feigned until killed/despawned — partial
-- port, documented; SpellHit — the whole tesla-beam
-- machine (SPELL_STALAGG_TESLA_PERIODIC 28098 -> overload state +
-- creatureCaster Talk(EMOTE_TESLA_LINK_BREAKS 0) + CastSpell(target,
-- SPELL_TESLA_SHOCK 28099, triggered)) / the refresh-beam recast of
-- SPELL_STALAGG_CHAIN_VISUAL 28096) — event 14 (OnHitBySpell) DOES
-- fire (boss_ai.go); the arm stays out because Talk/CastStop/
-- CastSpell all run on creatureCaster, which the event does not
-- pass (no caster bridge), and SetImmuneToPC + the random-target
-- leg are unbridged; UpdateAI's isFeignDeath powerSurge hold is
-- modeled via the Lua latch (see powerSurgeTick).

local ENTRY_STALAGG = 15929

local SAY_STALAGG_AGGRO = 0
local SAY_STALAGG_SLAY = 1

local SPELL_STALAGG_POWERSURGE = 28134

local timers = {}

-- Feign-death latch, declared before every function that touches it
-- (powerSurgeTick's isFeignDeath gate reads it).
local stalaggFeign = {}

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
-- 28134), 10s init, urandms(25,30) repeat. While feigned the C++
-- holds the timer at 0 (powerSurgeTimer = 0u, "delay until potential
-- revive", boss_thaddius.cpp:644) — modeled via the Lua latch: no
-- cast, re-arm on the 1s beat; the revive never fires in this port,
-- so the cast stays suppressed while feigned.
local function powerSurgeTick(creature, guid)
    if stalaggFeign[guid] then
        schedule(guid, "powersurge", 1000, function()
            powerSurgeTick(creature, guid)
        end)
        return
    end
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
    stalaggFeign[creature:GetGUID()] = nil
end

-- UnitDefines.h react states / SharedDefines.h stand states and unit flags
-- (engine/world/lua_creature_events.go SetReactState / SetStandState /
-- SetFlag bridges).
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2
local UNIT_FLAG_NOT_SELECTABLE = 0x200
local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_DEAD = 7

local function stalaggReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ has no Reset override: on evade stalagg runs EnterEvadeMode ->
    -- thaddius DoAction(ACTION_STALAGG_RESET) -> BeginResetEncounter ->
    -- DespawnOrUnsummon (boss_thaddius.cpp:490-501) and is force-respawned
    -- by thaddius with a fresh AI (isFeignDeath=false, template stand /
    -- flags / react state). Go has no despawn model, so ev23 restores the
    -- fresh-spawn equivalent and clears the Lua latch; without this a
    -- feigned stalagg would stay damage-immune across evades.
    stalaggFeign[guid] = nil
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_AGGRESSIVE)
end

-- C++ npc_stalagg::DamageTaken (boss_thaddius.cpp:572-602), portable
-- legs: lethal damage -> Talk(EMOTE_FEIGN_DEATH 3) +
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) +
-- RemoveAllAuras() + SetReactState(REACT_PASSIVE) + AttackStop() +
-- SetStandState(UNIT_STAND_STATE_DEAD) + damage = health-1; once latched,
-- all further damage = 0. The cross-AI DoAction(ACTION_STALAGG_DIED) +
-- SetControlled(UNIT_STATE_ROOT) / refreshBeam legs have no bridges —
-- documented, not wired. Event 9's second return rewrites damage
-- (fireCreatureDamageTaken; halazzi precedent). The C++ latch resets
-- in the AI ctor; the Lua mirror wipes the GUID entry on death
-- (event 4) and restores fresh-spawn state on reset (event 23, see
-- above). The unbridged ACTION_STALAGG_REVIVED machine means a
-- feigned stalagg never revives mid-fight in this port.
local EMOTE_FEIGN_DEATH = 3

local function stalaggDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if damage < creature:GetHealth() then
        return false, damage
    end
    if stalaggFeign[guid] then
        return false, 0
    end
    stalaggFeign[guid] = true
    creature:Talk(EMOTE_FEIGN_DEATH)
    creature:SetFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveAllAuras()
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:SetStandState(UNIT_STAND_STATE_DEAD)
    return false, creature:GetHealth() - 1
end

RegisterCreatureEvent(ENTRY_STALAGG, 1, stalaggEnterCombat)
RegisterCreatureEvent(ENTRY_STALAGG, 2, stalaggLeaveCombat)
RegisterCreatureEvent(ENTRY_STALAGG, 3, stalaggTargetDied)
RegisterCreatureEvent(ENTRY_STALAGG, 4, stalaggDied)
RegisterCreatureEvent(ENTRY_STALAGG, 9, stalaggDamageTaken)
RegisterCreatureEvent(ENTRY_STALAGG, 23, stalaggReset)
