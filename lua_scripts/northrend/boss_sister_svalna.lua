-- Sister Svalna (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sister_svalna.cpp
-- (1509 lines incl. license; boss_sister_svalna (CreatureScript,
-- BossAI, DATA_SISTER_SVALNA = 9) + npc_crok_scourgebane (EscortAI)
-- + npc_argent_captainAI base with npc_captain_arnath /
-- npc_captain_brandon / npc_captain_grondel / npc_captain_rupert
-- (ScriptedAI) + npc_frostwing_ymirjar_vrykul (ScriptedAI) +
-- npc_impaling_spear (CreatureAI) + 3 SpellScripts
-- (spell_svalna_caress_of_death (spell_trigger_spell_from_caster),
-- spell_svalna_revive_champion, spell_svalna_remove_spear) + 1
-- AreaTriggerScript (at_icc_start_frostwing_gauntlet); all
-- registered from inside AddSC_boss_sister_svalna(); loader decl
-- 180 / call 375 per northrend_script_loader.cpp — the TENTH
-- group of the "// Icecrown Citadel" block in
-- AddNorthrendScripts(), immediately after
-- AddSC_boss_blood_queen_lana_thel() (call 374); the call after
-- it is AddSC_boss_valithria_dreamwalker() (call 376) — verified
-- from the loader this run; the checkpoint sequence
-- (blood_queen_lana_thel -> sister_svalna) is followed).
-- Entry: 37126 Sister Svalna (icecrown_citadel.h
-- NPC_SISTER_SVALNA, line 283; icecrown_citadel.h
-- DATA_SISTER_SVALNA = 9, line 83;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_SISTER_SVALNA, line 288) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_sister_svalna" hits boss_sister_svalna.cpp (+ the
-- loader decl/call lines) only; this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction. No svalna
-- lua existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 37126):
-- boss_sister_svalna JustEngagedWith — crok->AI()->
-- Talk(SAY_CROK_COMBAT_SVALNA 5) is cross-AI Talk (no
-- cross-AI-Talk bridge); the DoCastSelf(SPELL_DIVINE_SURGE) leg
-- has no-cast bridge; scheduler legs have no-timer-bridge.
-- boss_sister_svalna DoAction — Talk(SAY_SVALNA_CAPTAIN_DEATH 5)
-- rides ACTION_CAPTAIN_DIES (no-DoAction bridge; the krick
-- ACTION_OUTRO precedent); the ACTION_KILL_CAPTAIN /
-- ACTION_START_GAUNTLET / ACTION_RESURRECT_CAPTAINS /
-- ACTION_RESET_EVENT legs ride instance / cast / bookkeeping
-- bridges that do not exist.
-- boss_sister_svalna SpellHit — Talk(EMOTE_SVALNA_BROKEN_SHIELD
-- 8, caster) on SPELL_HURL_SPEAR rides the SpellHit arm
-- (event 15 never fires; the anub_arak precedent).
-- boss_sister_svalna SpellHitTarget — Talk(EMOTE_SVALNA_IMPALE
-- 7, unitTarget) on SPELL_IMPALING_SPEAR (no SpellHitTarget
-- bridge).
-- boss_sister_svalna UpdateAI — Talk(SAY_SVALNA_EVENT_START 0)
-- rides EVENT_SVALNA_START; Talk(SAY_SVALNA_RESURRECT_CAPTAINS
-- 2) rides EVENT_SVALNA_RESURRECT (with the DoCast
-- SPELL_REVIVE_CHAMPION leg); Talk(SAY_SVALNA_AGGRO 3) rides
-- EVENT_SVALNA_COMBAT (no-timer-bridge; the boss_toravon
-- precedent); the impaling-spear / aether-shield DoCast legs
-- have no-cast bridge.
-- boss_sister_svalna JustDied — the _JustDied / captain
-- CaptainSurviveTalk cross-AI event legs (Talk(
-- SAY_CAPTAIN_SURVIVE_TALK 4) on the 4 captains via
-- ObjectAccessor GUID lookup) ride the absent instance /
-- cross-AI bridges.
-- boss_sister_svalna JustReachedHome / MovementInform —
-- SetReactState / SetDisableGravity / SetHover legs, no Talk of
-- Svalna's own (no MovementInform bridge).
-- npc_crok_scourgebane (EscortAI) — all Talk arms ride
-- waypoint / intro / gauntlet-event legs (SAY_CROK_INTRO_1 0,
-- SAY_CROK_INTRO_3 1, SAY_CROK_COMBAT_WP_0 2,
-- SAY_CROK_COMBAT_WP_1 3, SAY_CROK_FINAL_WP 4,
-- SAY_CROK_COMBAT_SVALNA 5, SAY_CROK_WEAKENING_GAUNTLET 6,
-- SAY_CROK_WEAKENING_SVALNA 7, SAY_CROK_DEATH 8) — no escort
-- bridge; joins the escort queue (NOT registered).
-- npc_frostwing_ymirjar_vrykul — zero Talk lines — NOT
-- registered (the bronjahm npc_corrupted_soul_fragment
-- precedent); the 5 ymirjar entries (37132 / 38125 / 37127 /
-- 37134 / 37133, OnCreatureCreate lines 452–456) join the
-- entry-evidence queue.
-- npc_impaling_spear — zero Talk lines — NOT registered (the
-- bronjahm npc_corrupted_soul_fragment precedent).
-- The 3 spell scripts join the no-SpellScript-bridge queue
-- (the boss_moragg optic-link precedent);
-- at_icc_start_frostwing_gauntlet joins the no-AreaTrigger
-- bridge queue (the boss_anubrekhan at_anubrekhan_entrance
-- precedent).
-- The 4 argent captains live in npc_argent_captains.lua.
-- All svalna Talk() calls accounted for (10 = SAY_SVALNA_KILL
-- 4 (1) + SAY_SVALNA_KILL_CAPTAIN 1 (1) + SAY_SVALNA_DEATH 6
-- (1) + SAY_SVALNA_CAPTAIN_DEATH 5 (1, DoAction) +
-- EMOTE_SVALNA_BROKEN_SHIELD 8 (1, SpellHit) +
-- EMOTE_SVALNA_IMPALE 7 (1, SpellHitTarget) +
-- SAY_SVALNA_EVENT_START 0 (1, scheduler) +
-- SAY_SVALNA_RESURRECT_CAPTAINS 2 (1, scheduler) +
-- SAY_SVALNA_AGGRO 3 (1, scheduler)).

local ENTRY_SISTER_SVALNA = 37126

local NPC_CAPTAIN_ARNATH = 37122
local NPC_CAPTAIN_BRANDON = 37123
local NPC_CAPTAIN_GRONDEL = 37124
local NPC_CAPTAIN_RUPERT = 37125

local SAY_SVALNA_KILL_CAPTAIN = 1
local SAY_SVALNA_KILL = 4
local SAY_SVALNA_DEATH = 6

local function isArgentCaptain(entry)
    return entry == NPC_CAPTAIN_ARNATH or entry == NPC_CAPTAIN_BRANDON
        or entry == NPC_CAPTAIN_GRONDEL or entry == NPC_CAPTAIN_RUPERT
end

-- C++ boss_sister_svalna::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SVALNA_KILL);
-- if (victim->GetTypeId() == TYPEID_UNIT and GetEntry() in the 4
-- captain entries) Talk(SAY_SVALNA_KILL_CAPTAIN) — the razuvious
-- player-gated variant precedent plus victim:GetEntry() for
-- creatures (the razuvious DK-understudy leg).
local function svalnaKilledUnit(event, creature, victim)
    if victim then
        local otype = victim:GetObjectType()
        if otype == "Player" then
            creature:Talk(SAY_SVALNA_KILL)
        elseif otype == "Creature" and isArgentCaptain(victim:GetEntry()) then
            creature:Talk(SAY_SVALNA_KILL_CAPTAIN)
        end
    end
end

-- C++ boss_sister_svalna::JustDied: Talk(SAY_SVALNA_DEATH) — the
-- sjonnir JustDied-Talk precedent.
local function svalnaJustDied(event, creature, killer)
    creature:Talk(SAY_SVALNA_DEATH)
end

RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 3, svalnaKilledUnit)
RegisterCreatureEvent(ENTRY_SISTER_SVALNA, 4, svalnaJustDied)
