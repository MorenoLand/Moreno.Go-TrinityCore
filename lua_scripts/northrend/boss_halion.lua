-- Halion (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_halion.cpp
-- (10 CreatureScripts: boss_halion (BossAI, DATA_HALION) +
-- boss_twilight_halion (BossAI, DATA_TWILIGHT_HALION) +
-- npc_halion_controller (ScriptedAI, NPC_HALION_CONTROLLER = 40146) +
-- npc_meteor_strike_flame / npc_meteor_strike_initial / npc_meteor_strike
-- / npc_combustion_consumption / npc_orb_carrier / npc_living_inferno /
-- npc_living_ember (ScriptedAI, zero Talk); 1 GameObjectScript
-- (go_twilight_portal); 15 SpellScriptLoaders
-- (spell_halion_meteor_strike_marker,
-- spell_halion_combustion_consumption x2, spell_halion_marks x2,
-- spell_halion_combustion_consumption_periodic,
-- spell_halion_damage_aoe_summon, spell_halion_twilight_realm_handlers
-- x2, spell_halion_summon_exit_portals, spell_halion_twilight_phasing,
-- spell_halion_twilight_cutter, spell_halion_clear_debuffs,
-- spell_halion_spawn_living_embers, spell_halion_blazing_aura); all
-- registered from inside AddSC_boss_halion() (line 1906); loader decl
-- 193 / call 388 per northrend_script_loader.cpp — the SIXTH and FINAL
-- group of the "// Ruby Sanctum" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_general_zarithrian() (call 387) —
-- verified from the loader this run; the checkpoint sequence
-- (zarithrian -> halion) is followed).
-- Entries: 39863 Halion (ruby_sanctum.h NPC_HALION, line 80) and 40142
-- Twilight Halion (ruby_sanctum.h NPC_TWILIGHT_HALION, line 81);
-- instance_ruby_sanctum.cpp creatureData binds NPC_HALION ->
-- DATA_HALION (line 54), NPC_TWILIGHT_HALION -> DATA_TWILIGHT_HALION
-- (line 55), NPC_HALION_CONTROLLER -> DATA_HALION_CONTROLLER (line 56)
-- — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_halion" hits boss_halion.cpp only (+ the loader
-- decl/call lines); this clone carries no sql/ tree, so ScriptName
-- bindings are DB-side by construction. Pre-existing lua coverage
-- AUDITED this run — COMPLETE (see accounting below).
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_halion JustEngagedWith — Talk(SAY_AGGRO 2)
-- (event 1 — the auriaya precedent; the events.Reset /
-- events.SetPhase(PHASE_ONE) / BossAI::JustEngagedWith / DoCastSelf
-- SPELL_TWILIGHT_PRECISION / EVENT_ACTIVATE_FIREWALL / EVENT_BREATH /
-- EVENT_CLEAVE / EVENT_TAIL_LASH / EVENT_FIERY_COMBUSTION /
-- EVENT_METEOR_STRIKE ScheduleEvent / SendEncounterUnit /
-- controller->AI()->SetData(DATA_FIGHT_PHASE, PHASE_ONE) legs have no
-- bridges).
-- boss_halion JustDied — Talk(SAY_DEATH 5)
-- (event 4 — the sjonnir precedent; the _JustDied() instance leg and
-- the cross-creature twilightHalion->KillSelf() /
-- controller->KillSelf() legs have no bridges).
-- boss_twilight_halion KilledUnit — Talk(SAY_KILL 6) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent) + the UNGATED triggered victim-cast
-- SPELL_LEAVE_TWILIGHT_REALM 74812 ("victims should not be in the
-- twilight realm" — the arlokk CastSpell(victim, id, true) idiom).
-- Material halion has no KilledUnit arm — SAY_KILL lives on the
-- twilight AI only.
-- boss_twilight_halion timer arms — PORTED via the CreateLuaEvent
-- harness (GUID-keyed timers table, cancel on events 2/4/23 — the
-- krystallus / bjarngrim-lieutenant convention), armed from
-- twilightHalionEnterCombat (entry 40142 registered events 1/2/3/4/23):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- 3s init / randtime(7s,10s) re-arm (boss_halion.cpp:525-528) ->
-- creature:CastSpell(nil, SPELL_CLEAVE) — the DoCastVictim lowering.
-- EVENT_BREATH — DoCastSelf(SPELL_DARK_BREATH 74806) non-triggered,
-- 12s init / randtime(10s,14s) re-arm (:533-536) ->
-- creature:CastSpell(creature, SPELL_DARK_BREATH).
-- EVENT_TAIL_LASH — DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered,
-- 12s init (armed in the C++ constructor, :420) / randtime(12s,16s)
-- re-arm (:529-532) -> creature:CastSpell(creature, SPELL_TAIL_LASH)
-- — the DoCastAOE self-cast lowering (boss_ai.go convention).
-- DOCUMENTED-ONLY (in this header; only entries 39863 and 40142
-- registered):
-- boss_halion DamageTaken — Talk(SAY_PHASE_TWO 4) rides the 75% health
-- transition (no-DamageTaken bridge); the UNIT_FLAG_NOT_SELECTABLE /
-- DoCastSelf SPELL_TWILIGHT_PHASING / controller SetData(PHASE_TWO)
-- legs ride absent bridges.
-- boss_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING (the SpellHit-15-never-fires queue).
-- boss_halion UpdateAI — Talk(SAY_METEOR_STRIKE 3) rides
-- EVENT_METEOR_STRIKE, phase-machine-gated (PHASE_ONE->TWO transition
-- via 75%-health DamageTaken + controller SetData, boss_halion.cpp:
-- 286-296); the EVENT_BREATH / EVENT_ACTIVATE_FIREWALL /
-- EVENT_FIERY_COMBUSTION DoCastSelf / CastSpell legs ride the same
-- phase machine (SetPhase :246/:288/:426/:477 + cross-creature
-- controller SetData :260/:296/:667 — no phase-machine bridge).
-- boss_twilight_halion JustEngagedWith — zero Talk; the three
-- phase-ungated timer arms are PORTED (see twilightHalionEnterCombat
-- below). C++ UpdateAI has no phase early-return (boss_halion.cpp:
-- 511-544), so the timers run identically in PHASE_TWO and
-- PHASE_THREE — engage-armed CreateLuaEvent timers are faithful
-- across both. The SendEncounterUnit / DoCastSelf
-- SPELL_TWILIGHT_PRECISION legs ride absent bridges.
-- EVENT_SOUL_CONSUMPTION 74792 stays out: SelectTarget Random with
-- SPELL_TWILIGHT_REALM aura filter + TRIGGERED_IGNORE_SET_FACING —
-- no random-target / target-filter bridges (no-bridge queues).
-- Constructor-side legs (DUSK_SHROUD / COPY_DAMAGE auras, health
-- sync, phase-mask / react-state / combat flags) ride absent
-- bridges; TAIL_LASH's 12s constructor-arm is documented as a
-- spawn->engage drift on the first tick only (twilight halion
-- spawns in combat, so the delta is near-zero).
-- boss_twilight_halion DamageTaken — Talk(SAY_PHASE_THREE 2) rides the
-- 50% health transition (no-DamageTaken bridge); the
-- DoCastSelf SPELL_TWILIGHT_DIVISION / controller SetData legs ride
-- absent bridges.
-- boss_twilight_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING (the SpellHit-15-never-fires queue); the
-- ACTION_MONITOR_CORPOREALITY DoAction leg rides no-DoAction bridge.
-- boss_twilight_halion JustDied — zero Talk; the DoCastSelf
-- (SPELL_CLEAR_DEBUFFS 75396) cast leg is PORTED (twilightHalionDied,
-- boss_halion.cpp:599-605 — the halion->LowerPlayerDamageReq /
-- Unit::Kill / controller->KillSelf legs ride absent bridges).
-- boss_twilight_halion Reset — the DoCastSelf
-- (SPELL_CLEAR_DEBUFFS 75396) cast leg is PORTED (twilightHalionReset,
-- event 23, boss_halion.cpp:586-592 — the _summons / _events /
-- Initialize legs ride absent bridges).
-- npc_halion_controller (40146) — its intro machine issues cross-AI
-- Talk(SAY_INTRO 1) on the summoned halion, plus the corporeality
-- machine's Talk(EMOTE_CORPOREALITY_TOT 4 / TIT 3 on twilight halion,
-- POT 8 / PIP 9 on material halion) and the orb-shoot machine's
-- cross-AI Talk(EMOTE_WARN_LASER 0) on the orb-carrier passenger and
-- Talk(SAY_SPHERE_PULSE 1) on twilight halion (no cross-AI-Talk
-- bridge); the DoCastSelf / scheduler / DoAction legs ride absent
-- bridges. Zero Talk on its own unit — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- The other 8 CreatureScripts / go_twilight_portal / the 15
-- SpellScriptLoaders — zero Talk (npc_orb_carrier /
-- npc_meteor_strike* / npc_combustion_consumption / npc_living_inferno
-- / npc_living_ember) or spell-model arms (no-SpellScript /
-- no-AuraScript bridge, the sindragosa ice_tomb_target precedent) —
-- NOT registered.
-- SAY_BERSERK 7 — dead text, no Talk() call in the file.
-- All 13 Talk() calls in the file accounted for (SAY_AGGRO 2 +
-- SAY_DEATH 5 + SAY_KILL 6 ported; SAY_PHASE_TWO 4 + SAY_REGENERATE 0
-- x2 + SAY_METEOR_STRIKE 3 + SAY_PHASE_THREE 2 + SAY_INTRO 1 +
-- EMOTE_CORPOREALITY_TOT/TIT/POT/PIP + EMOTE_WARN_LASER 0 +
-- SAY_SPHERE_PULSE 1 documented above).

local ENTRY_HALION = 39863
local ENTRY_TWILIGHT_HALION = 40142

local SAY_INTRO    = 1 -- (documented-only, rides the controller's cross-AI intro)
local SAY_AGGRO    = 2
local SAY_DEATH    = 5
local SAY_KILL     = 6 -- twilight halion only

-- C++ boss_halion::JustEngagedWith:
-- Talk(SAY_AGGRO); events.Reset(); events.SetPhase(PHASE_ONE);
-- BossAI::JustEngagedWith(who); me->AddAura(SPELL_TWILIGHT_PRECISION, me);
-- events.ScheduleEvent(EVENT_ACTIVATE_FIREWALL, 5s); [breath/cleave/
-- tail_lash/fiery_combustion/meteor_strike] — the auriaya precedent
-- (the non-Talk legs have no bridges).
local function halionEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_halion::JustDied:
-- _JustDied(); Talk(SAY_DEATH) — the sjonnir precedent (the
-- _JustDied() instance leg and the cross-creature KillSelf legs have
-- no bridges).
local function halionDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

-- C++ boss_twilight_halion::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent;
-- me->CastSpell(victim, SPELL_LEAVE_TWILIGHT_REALM, true) — UNGATED
-- (fires for any victim type), triggered victim-cast — the arlokk idiom.
-- Material halion has no KilledUnit arm.
local function twilightHalionKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
    creature:CastSpell(victim, SPELL_LEAVE_TWILIGHT_REALM, true)
end

RegisterCreatureEvent(ENTRY_HALION, 1, halionEnterCombat)
RegisterCreatureEvent(ENTRY_HALION, 4, halionDied)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 3, twilightHalionKilledUnit)

local SPELL_CLEAVE      = 74524
local SPELL_DARK_BREATH = 74806
local SPELL_TAIL_LASH   = 74531
local SPELL_CLEAR_DEBUFFS = 75396
local SPELL_LEAVE_TWILIGHT_REALM = 74812

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the krystallus / bjarngrim-lieutenant convention.
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

-- C++ UpdateAI EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE 74524)
-- non-triggered, 3s init / randtime(7s,10s) re-arm
-- (boss_halion.cpp:525-528) — no phase gating on the arm.
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {7000, 10000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_BREATH: DoCastSelf(SPELL_DARK_BREATH 74806)
-- non-triggered, 12s init / randtime(10s,14s) re-arm
-- (boss_halion.cpp:533-536) — no phase gating on the arm.
local function breathTick(creature, guid)
    creature:CastSpell(creature, SPELL_DARK_BREATH)
    schedule(guid, "breath", {10000, 14000}, function()
        breathTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_TAIL_LASH: DoCastAOE(SPELL_TAIL_LASH 74531)
-- non-triggered, 12s init armed in the constructor (:420) /
-- randtime(12s,16s) re-arm (:529-532) — no phase gating on the arm.
-- The constructor arm is re-anchored on OnEnterCombat: twilight
-- halion spawns in combat (UNIT_FLAG_IN_COMBAT at :414), so the
-- spawn->engage delta on the first tick is near-zero — documented.
local function tailLashTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    schedule(guid, "tail_lash", {12000, 16000}, function()
        tailLashTick(creature, guid)
    end)
end

-- C++ boss_twilight_halion::JustEngagedWith:
-- events.SetPhase(PHASE_TWO); BossAI::JustEngagedWith(who);
-- me->AddAura(SPELL_TWILIGHT_PRECISION, me);
-- events.ScheduleEvent(EVENT_CLEAVE, 3s);
-- events.ScheduleEvent(EVENT_BREATH, 12s);
-- instance->SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE, me, 2)
-- — only the three timer arms bridge (the TAIL_LASH arm moves here
-- from its constructor schedule; SOUL_CONSUMPTION's random-target
-- filter arm has no bridge and stays out).
local function twilightHalionEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "cleave", 3000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "breath", 12000, function()
        breathTick(creature, guid)
    end)
    schedule(guid, "tail_lash", 12000, function()
        tailLashTick(creature, guid)
    end)
end

local function twilightHalionLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_twilight_halion::Reset: _summons.DespawnAll() +
-- _events.Reset() + Initialize() + DoCastSelf(SPELL_CLEAR_DEBUFFS)
-- (non-triggered, boss_halion.cpp:586-592) — the summon/timer/
-- Initialize legs have no bridges; the cast is the C++-exact tail
-- leg. Split from event 2 so the cast fires once on the reset path.
local function twilightHalionReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_CLEAR_DEBUFFS)
end

-- C++ boss_twilight_halion::JustDied: _events.Reset() +
-- _summons.DespawnAll() + DoCastSelf(SPELL_CLEAR_DEBUFFS)
-- (non-triggered, boss_halion.cpp:599-605).
local function twilightHalionDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_CLEAR_DEBUFFS)
end

RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 1, twilightHalionEnterCombat)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 2, twilightHalionLeaveCombat)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 4, twilightHalionDied)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 23, twilightHalionReset)
