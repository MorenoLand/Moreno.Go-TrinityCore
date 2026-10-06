-- Ingvar the Plunderer (Utgarde Keep) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/boss_ingvar_the_plunderer.cpp
-- (489 lines; 5 scripts — boss_ingvar_the_plunderer (CreatureScript via
-- GetUtgardeKeepAI (BossAI), DATA_INGVAR = 2); npc_annhylde_the_caller
-- (CreatureScript via GetUtgardeKeepAI (ScriptedAI)); npc_ingvar_throw_dummy
-- (CreatureScript via GetUtgardeKeepAI (ScriptedAI));
-- spell_ingvar_summon_banshee (SpellScriptLoader, SpellScript
-- OnDestinationTargetSelect); spell_ingvar_woe_strike (SpellScriptLoader,
-- AuraScript proc); all registered from inside
-- AddSC_boss_ingvar_the_plunderer(); loader decl 128 / call 323 per
-- northrend_script_loader.cpp — the THIRD group of the "// Utgarde Keep -
-- Utgarde Keep" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_skarvald_dalronn() (decl 127 / call 322); the checkpoint
-- sequence (boss_skarvald_dalronn -> boss_ingvar_the_plunderer) is
-- followed).
-- Entries: 23954 Ingvar the Plunderer (utgarde_keep.h NPC_INGVAR, line 49)
-- / 23980 Ingvar the Plunderer undead form (NPC_INGVAR_UNDEAD, line 56) —
-- entry-verifiable, registration proceeds (the nexus_commanders / malygos /
-- sartharion kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual. DATA_INGVAR = 2 (utgarde_keep.h line 33).
-- Sole-source verified: whole-server-tree grep for all five script names
-- hits boss_ingvar_the_plunderer.cpp only (+ the loader decl/call lines
-- for AddSC_boss_ingvar_the_plunderer); the clone's sql/ tree
-- (~/workspace/moreno-trinitycore/sql/, characters/ + world/) carries
-- zero ingvar hits (empty of bindings), so ScriptName bindings are
-- DB-side by construction. This header is the audited-artifact record
-- for the existing port (9df78d1, Oct 2 10:06 UTC) — re-verified this
-- run; the first-pass "no ingvar lua existed" claim was stale.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1, human entry only); the
-- C++ early return on PHASE_EVENT / PHASE_UNDEAD is modeled with the
-- engaged latch below — yell once per pull cycle, silent on the phase-2
-- re-engage (the auriaya engage-port precedent).
-- DamageTaken — Talk(SAY_DEATH 2) when damage >= GetHealth() while in
-- PHASE_HUMAN (the feign-death yell) (event 9, human entry only); the
-- feignDead latch mirrors the PHASE_HUMAN -> PHASE_EVENT transition so
-- the yell fires exactly once (the moroes threshold+latch precedent).
-- JustDied — Talk(SAY_DEATH 2) unconditional (event 4, both entries —
-- the real (undead-phase) death yell; the sjonnir JustDied-Talk
-- precedent).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3, both entries — no
-- phase gate in C++; the razuvious player-gated variant precedent; the
-- lua victim nil-guard follows the hodir port convention — Eluna event
-- 3 always carries a victim, C++-exactness unaffected).
-- DOCUMENTED-ONLY (in this header; no registrations beyond entries 23954
-- / 23980):
-- EVENT_JUST_TRANSFORMED Talk(SAY_AGGRO) (the phase-2 aggro yell) and the
-- EVENT_CLEAVE / EVENT_SMASH / EVENT_STAGGERING_ROAR / EVENT_ENRAGE /
-- EVENT_DARK_SMASH / EVENT_DREADFUL_ROAR / EVENT_WOE_STRIKE /
-- EVENT_SHADOW_AXE spell casts + ShadowAxe target selection — the timer
-- event machine has no bridge; joins the no-timer-bridge queue.
-- The feign-death DamageTaken legs (RemoveAllAuras, DoCast
-- SPELL_INGVAR_FEIGN_DEATH, non-attackable/immune flags) and the damage =
-- 0 rewrite in PHASE_EVENT — no bridges.
-- DoAction(ACTION_START_PHASE_2) / StartZombiePhase (UpdateEntry to 23980,
-- transform cast) — no DoAction bridge.
-- npc_annhylde_the_caller (24068 NPC_ANNHYLDE_THE_CALLER, line 58) —
-- Talk(YELL_RESURRECT 0) in MovementInform, plus the cross-creature
-- resurrect machine (instance GetGuidData(DATA_INGVAR), beams, heals,
-- DoAction(ACTION_START_PHASE_2)) — MovementInform / instance-GuidData /
-- cross-creature bridges missing; no registration.
-- npc_ingvar_throw_dummy (NPC_THROW_TARGET 23996, line 57) — no Talk arms
-- (charge / periodic-damage / despawn machine) — no bridges; no
-- registration.
-- spell_ingvar_summon_banshee (SpellScript OnDestinationTargetSelect) /
-- spell_ingvar_woe_strike (AuraScript proc) — no SpellScript / AuraScript
-- bridges; join the no-SpellScript / no-AuraScript-bridge queues.

local ENTRY_INGVAR = 23954
local ENTRY_INGVAR_UNDEAD = 23980

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- Per-creature state mirroring the AI members / EventMap phases:
-- engaged — C++ JustEngagedWith sets PHASE_HUMAN on the first engage;
-- re-engages in PHASE_EVENT / PHASE_UNDEAD return early with no yell.
-- feignDead — C++ DamageTaken transitions PHASE_HUMAN -> PHASE_EVENT on
-- the feign-death Talk(SAY_DEATH); the latch keeps the yell one-shot.
-- C++ Reset() (_Reset -> events.Reset()) clears the phases; event 23
-- does the same here.
local ingvarState = {}

local function getState(guid)
    local state = ingvarState[guid]
    if state == nil then
        state = { engaged = false, feignDead = false }
        ingvarState[guid] = state
    end
    return state
end

-- C++ boss_ingvar_the_plundererAI::JustEngagedWith: if in PHASE_EVENT or
-- PHASE_UNDEAD return; BossAI::JustEngagedWith(who) (passthrough, no
-- bridge); Talk(SAY_AGGRO); SetPhase(PHASE_HUMAN). The engaged latch
-- models the phase gate — C++-exact: one yell per pull cycle, silent on
-- the phase-2 re-engage (the auriaya engage-port precedent).
local function ingvarEnterCombat(event, creature, target)
    local state = getState(creature:GetGUID())
    if state.engaged then
        return
    end
    state.engaged = true
    creature:Talk(SAY_AGGRO)
end

-- C++ DamageTaken: if (damage >= me->GetHealth() &&
-- events.IsInPhase(PHASE_HUMAN)) { <unbridgeable feign-death legs>;
-- Talk(SAY_DEATH); } — the engaged gate mirrors the PHASE_HUMAN
-- requirement, the feignDead latch the one-shot transition (the moroes
-- threshold+latch precedent); the PHASE_EVENT damage = 0 rewrite has no
-- bridge. Registered on the human entry only — the arm is unreachable
-- for the undead entry in C++ (PHASE_HUMAN is never re-entered).
local function ingvarDamageTaken(event, creature, attacker, damage)
    local state = getState(creature:GetGUID())
    if state.feignDead or not state.engaged then
        return
    end
    if damage >= creature:GetHealth() then
        state.feignDead = true
        creature:Talk(SAY_DEATH)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge); Talk(SAY_DEATH) —
-- unconditional — the sjonnir JustDied-Talk precedent. Fires on the real
-- (undead-phase) death; the feign-death yell rides event 9 above.
local function ingvarJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY)
-- — no phase gate — the razuvious player-gated variant precedent; the
-- nil-guard follows the hodir port convention (Eluna event 3 always
-- carries a victim; C++-exactness unaffected).
local function ingvarKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function ingvarReset(event, creature)
    ingvarState[creature:GetGUID()] = nil
end

RegisterCreatureEvent(ENTRY_INGVAR, 1, ingvarEnterCombat)
RegisterCreatureEvent(ENTRY_INGVAR, 9, ingvarDamageTaken)
RegisterCreatureEvent(ENTRY_INGVAR, 3, ingvarKilledUnit)
RegisterCreatureEvent(ENTRY_INGVAR, 4, ingvarJustDied)
RegisterCreatureEvent(ENTRY_INGVAR, 23, ingvarReset)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 3, ingvarKilledUnit)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 4, ingvarJustDied)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 23, ingvarReset)
