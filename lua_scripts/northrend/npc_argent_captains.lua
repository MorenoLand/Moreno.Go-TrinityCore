-- Argent Captains (Icecrown Citadel, Frostwing Halls gauntlet) —
-- Lua port of the npc_argent_captainAI base in
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sister_svalna.cpp
-- (npc_captain_arnath / npc_captain_brandon / npc_captain_grondel
-- / npc_captain_rupert, all deriving from npc_argent_captainAI
-- (ScriptedAI); bridgeable arms come from the shared base, per
-- the boss_blood_prince_council 3-entries-1-file precedent).
-- Entries: 37122 Captain Arnath / 37123 Captain Brandon /
-- 37124 Captain Grondel / 37125 Captain Rupert
-- (icecrown_citadel.h NPC_CAPTAIN_ARNATH..RUPERT, lines 270–273;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds all four,
-- lines 282–286) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName bindings are DB-side as usual.
-- DOCUMENTED-ENTRY EVIDENCE (not registered): the undead
-- variants 37491 / 37493 / 37494 / 37495 (icecrown_citadel.h
-- lines 275–277) share the same AI instance — C++ switches
-- entry via me->UpdateEntry() on SPELL_REVIVE_CHAMPION hit, the
-- AI is not replaced — so the base-entry registration covers
-- them; their creature_template ScriptName bindings are DB-side
-- by construction (this clone carries no sql/ tree).
--
-- DOCUMENTED-BLOCKED (no registration beyond the 4 entries):
-- npc_argent_captainAI JustDied — Talk(SAY_CAPTAIN_DEATH 0) on
-- first death / Talk(SAY_CAPTAIN_SECOND_DEATH 3) on second death
-- is gated on the per-AI _firstDeath flag, reset by DoAction
-- ACTION_RESET_EVENT (no-DoAction bridge); no per-creature
-- state port precedent exists in the lua set — a file-scope
-- local would be wrong (one flag shared by all four captains) —
-- so the arm is documented here rather than wired.
-- npc_argent_captainAI SpellHit — Talk(SAY_CAPTAIN_RESURRECTED
-- 1) rides the SPELL_REVIVE_CHAMPION SpellHit arm (event 15
-- never fires; the anub_arak precedent); the UpdateEntry /
-- DoCastSelf(SPELL_UNDEATH) legs have no bridge.
-- npc_argent_captainAI DoAction — ACTION_START_GAUNTLET /
-- ACTION_RESET_EVENT follow / active / bookkeeping legs (no
-- DoAction bridge).
-- npc_argent_captainAI JustEngagedWith — SetHomePosition /
-- DoZoneInCombat legs (no bridge).
-- The captain spell-event UpdateAI legs (arnath flash heal /
-- PW shield / smite / dominate mind; brandon crusader strike /
-- divine shield / judgement / hammer of betrayal; grondel
-- charge / mortal strike / sunder armor / conflagration; rupert
-- fel iron bomb / machine gun / rocket launch) are timer-driven
-- with no-cast bridge (no-timer-bridge; the boss_toravon
-- precedent).
-- All base-AI Talk() calls accounted for (4 = SAY_CAPTAIN_DEATH
-- 0 (1) + SAY_CAPTAIN_SECOND_DEATH 3 (1) + SAY_CAPTAIN_KILL 2
-- (1) + SAY_CAPTAIN_RESURRECTED 1 (1, SpellHit)); the 4
-- derived structs add no Talk of their own.

local ENTRY_CAPTAIN_ARNATH = 37122
local ENTRY_CAPTAIN_BRANDON = 37123
local ENTRY_CAPTAIN_GRONDEL = 37124
local ENTRY_CAPTAIN_RUPERT = 37125

local SAY_CAPTAIN_KILL = 2

-- C++ npc_argent_captainAI::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_CAPTAIN_KILL)
-- — the razuvious player-gated variant precedent.
local function captainKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_CAPTAIN_KILL)
    end
end

RegisterCreatureEvent(ENTRY_CAPTAIN_ARNATH, 3, captainKilledUnit)
RegisterCreatureEvent(ENTRY_CAPTAIN_BRANDON, 3, captainKilledUnit)
RegisterCreatureEvent(ENTRY_CAPTAIN_GRONDEL, 3, captainKilledUnit)
RegisterCreatureEvent(ENTRY_CAPTAIN_RUPERT, 3, captainKilledUnit)
