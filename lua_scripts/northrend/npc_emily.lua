-- Emily (Grizzly Hills) --
-- Lua port of src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- (npc_emilyAI — JustEngagedWith arm only). Zone-script unit per
-- northrend_script_loader.cpp order (dragonblight done; grizzly_hills:
-- npc_mrfloppy, npc_outhouse_bunny, npc_tallhorn_stag,
-- npc_amberpine_woodsman, npc_wounded_skirmisher,
-- npc_venture_co_straggler, npc_lake_frog, npc_rocket_propelled_warhead,
-- spell_shredder_delivery, spell_infected_worgen_bite,
-- spell_vehicle_warhead_fuse, spell_warhead_detonate, spell_z_check,
-- spell_warhead_fuse documented-only).
-- Entry (Floppy enum, verifiable from the C++ sources): 26588
-- (NPC_EMILY). The creature_template ScriptName binding is DB-side
-- (no TDB in this workspace).
-- Eluna creature events: 1 OnEnterCombat. The C++ overrides no
-- KilledUnit / JustDied (no events 3/4); no SpellHit / DoAction /
-- gossip arms (no events 14/27/31); no OnReset work worth wiring
-- (GUID clears only).
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): Talk(SAY_RANDOMAGGRO 4, no target — C++ calls
-- Talk(4) with no who) — the event-1 auriaya precedent. The AI
-- class is EscortAI-based, but the JustEngagedWith arm is a bare
-- self-Talk, fully covered by the event-1 Talk bridge.
-- Unmodeled (no bridges — documented, not wired):
-- npc_emily: WaypointReached escort cinematic (Talk SAY_WORGHAGGRO1
-- 0 / SAY_WORGRAGGRO3 2 / SAY_VICTORY2 6 / SAY_VICTORY3 7 /
-- SAY_VICTORY4 8 / SAY_QUEST_COMPLETE 12 + SummonCreature
-- 26586/26590 + DoCast 47184 + EnterVehicle + faction swaps +
-- MovePoint/MoveFollow + DisappearAndDie legs) — no-escort /
-- no-timer / no-summon / no-cast / no-movement / no-faction bridges
-- (thassarian/borean_tundra precedent); OnQuestAccept Talk
-- SAY_QUEST_ACCEPT 11 + Start escort — no-quest-accept bridge.
-- npc_mrfloppy: JustEngagedWith Talk arms are all cross-creature
-- (Emily->AI()->Talk SAY_WORGHAGGRO2 1 / SAY_WORGRAGGRO4 3 /
-- SAY_RANDOMAGGRO 4, entry-gated on 26586/26590) — no cross-creature
-- Talk bridge; no JustDied / KilledUnit arms.
-- npc_outhouse_bunny (Doing Your Duty 12227): SpellHit
-- SPELL_OUTHOUSE_GROANS 48382 -> emote + DoCast
-- SPELL_CAMERA_SHAKE 47533 / SPELL_DUST_FIELD 48329 — no-SpellHit /
-- no-cast bridges.
-- npc_tallhorn_stag + npc_amberpine_woodsman (Amberpine woodsman
-- skinning event): zero Talk; UpdateAI NPC_NPC_FLAGS /
-- UNIT_NPC_EMOTESTATE state machine (0.2f FindNearestCreature
-- 26363) — no-quest-state bridge.
-- npc_wounded_skirmisher (Overwhelmed 12288): SpellHit
-- SPELL_RENEW_SKIRMISHER 48812 -> Talk(SAY_RANDOM) + DoCast
-- SPELL_KILL_CREDIT 48813 — no-SpellHit bridge (fizzule precedent).
-- npc_venture_co_straggler (Smoke 'Em Out 12323/12324): Talk(SAY_SEO)
-- rides EVENT_STRAGGLER_2 timer + MovePoint chain + DisappearAndDie;
-- SpellHit SPELL_SMOKE_BOMB 49075 -> pacify + event chain; EVENT_CHOP
-- DoCastVictim SPELL_CHOP 43410 — no-timer / no-SpellHit / no-cast /
-- no-movement bridges.
-- npc_lake_frog: Talk SAY_MAIDEN_0 / SAY_MAIDEN_1 ride
-- EVENT_LAKEFROG_2/5 timers (SetEntry 33220 maiden transform,
-- SPELL_MAIDEN_OF_ASHWOOD_LAKE_TRANSFORM 62550, gossip-flag
-- toggling, DespawnOrUnsummon) — no-timer / no-entry-update /
-- no-gossip bridges; ReceiveEmote TEXT_EMOTE_KISS + item 44986 /
-- auras 62537/62574/62581 legs — no-emote / no-item bridges.
-- npc_rocket_propelled_warhead: VehicleAI PassengerBoarded DoCast
-- SPELL_VEHICLE_WARHEAD_FUSE + DoAction MovePoint chain — no
-- PassengerBoarded / no-DoAction / no-movement bridges.
-- spell_shredder_delivery / spell_infected_worgen_bite /
-- spell_vehicle_warhead_fuse / spell_warhead_detonate /
-- spell_z_check / spell_warhead_fuse: all SpellScript/AuraScript
-- — no SpellScript/AuraScript bridge anywhere in the model
-- (sindragosa ice_tomb_target precedent).

local ENTRY_EMILY = 26588
local SAY_RANDOMAGGRO = 4 -- There's a big meanie attacking Mr. Floppy! Help!

-- C++ JustEngagedWith: Talk(SAY_RANDOMAGGRO) (no target).
local function emilyEnterCombat(event, creature, target)
    creature:Talk(SAY_RANDOMAGGRO)
end

RegisterCreatureEvent(ENTRY_EMILY, 1, emilyEnterCombat)
