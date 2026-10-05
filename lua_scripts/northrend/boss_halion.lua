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
-- player-gated variant precedent; the SPELL_LEAVE_TWILIGHT_REALM cast
-- leg has no-cast bridge). Material halion has no KilledUnit arm —
-- SAY_KILL lives on the twilight AI only.
-- DOCUMENTED-ONLY (in this header; only entries 39863 and 40142
-- registered):
-- boss_halion DamageTaken — Talk(SAY_PHASE_TWO 4) rides the 75% health
-- transition (no-DamageTaken bridge); the UNIT_FLAG_NOT_SELECTABLE /
-- DoCastSelf SPELL_TWILIGHT_PHASING / controller SetData(PHASE_TWO)
-- legs ride absent bridges.
-- boss_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING (the SpellHit-15-never-fires queue).
-- boss_halion UpdateAI — Talk(SAY_METEOR_STRIKE 3) rides
-- EVENT_METEOR_STRIKE (no-timer-bridge); the EVENT_BREATH /
-- EVENT_ACTIVATE_FIREWALL / EVENT_FIERY_COMBUSTION DoCastSelf /
-- CastSpell legs ride no-cast bridges.
-- boss_twilight_halion JustEngagedWith — zero Talk (not registered);
-- its scheduler / SendEncounterUnit legs ride absent bridges.
-- boss_twilight_halion DamageTaken — Talk(SAY_PHASE_THREE 2) rides the
-- 50% health transition (no-DamageTaken bridge); the
-- DoCastSelf SPELL_TWILIGHT_DIVISION / controller SetData legs ride
-- absent bridges.
-- boss_twilight_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING (the SpellHit-15-never-fires queue); the
-- ACTION_MONITOR_CORPOREALITY DoAction leg rides no-DoAction bridge.
-- boss_twilight_halion JustDied — zero Talk (halion->LowerPlayerDamageReq
-- / Unit::Kill / controller->KillSelf legs ride absent bridges).
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
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
-- Material halion has no KilledUnit arm.
local function twilightHalionKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

RegisterCreatureEvent(ENTRY_HALION, 1, halionEnterCombat)
RegisterCreatureEvent(ENTRY_HALION, 4, halionDied)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 3, twilightHalionKilledUnit)
