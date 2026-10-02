-- Frostsworn General (Halls of Reflection) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/halls_of_reflection.cpp
-- (2916 lines; 25 scripts: 16 CreatureScripts
-- (npc_jaina_or_sylvanas_intro_hor, npc_jaina_or_sylvanas_escape_hor,
-- npc_the_lich_king_escape_hor, npc_ghostly_priest, npc_phantom_mage,
-- npc_phantom_hallucination, npc_shadowy_mercenary, npc_spectral_footman,
-- npc_tortured_rifleman, npc_frostsworn_general, npc_spiritual_reflection,
-- npc_raging_ghoul, npc_risen_witch_doctor, npc_lumbering_abomination,
-- npc_uther_quel_delar, npc_quel_delar_sword), 5 AreaTriggerScripts
-- (at_hor_intro_start, at_hor_waves_restarter, at_hor_impenetrable_door,
-- at_hor_shadow_throne, at_hor_uther_quel_delar_start), 3
-- SpellScriptLoaders (spell_hor_start_halls_of_reflection_quest_ae,
-- spell_hor_evasion, spell_hor_gunship_cannon_fire) + spell_hor_quel_delars_will
-- (SpellScript); all registered from inside AddSC_halls_of_reflection();
-- loader decl 167 / call 362 per northrend_script_loader.cpp — the SECOND
-- group of the "// Halls of Reflection" block in AddNorthrendScripts(),
-- immediately after AddSC_instance_halls_of_reflection() (call 361); the
-- calls after it are AddSC_boss_falric() (call 363) / AddSC_boss_marwyn()
-- (call 364) — verified from the loader this run; the checkpoint sequence
-- (instance_halls_of_reflection -> halls_of_reflection) is followed).
-- Entries: 36723 Frostsworn General (halls_of_reflection.h
-- NPC_FROSTSWORN_GENERAL, line 79;
-- instance_halls_of_reflection.cpp OnCreatureCreate binds case
-- NPC_FROSTSWORN_GENERAL, line 141) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. All other zone-script creatures
-- carry zero bridgeable arms and are NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent): the eight gauntlet-trash scripts
-- (ghostly_priest, phantom_mage, phantom_hallucination, shadowy_mercenary,
-- spectral_footman, tortured_rifleman) and the four escape-trash scripts
-- (raging_ghoul, risen_witch_doctor, lumbering_abomination,
-- spiritual_reflection) expose only scheduler-schedule legs in
-- JustEngagedWith / Reset / IsSummonedBy / JustDied (DoCastAOE at most)
-- with no Talk of their own; npc_jaina_or_sylvanas_intro_hor's Talk legs
-- all ride the instance SetData intro-event machine (no instance bridge);
-- npc_jaina_or_sylvanas_escape_hor has no Talk of its own (JustDied only
-- evades the Lich King); npc_the_lich_king_escape_hor's KilledUnit is a
-- DoPlaySoundToSet leg with no Talk; npc_uther_quel_delar and
-- npc_quel_delar_sword's Talk legs ride timer/event machines
-- (EVENT_UTHER_1..9, EVENT_QUEL_DELAR_INIT) with no bridges.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_halls_of_reflection" hits halls_of_reflection.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No frostsworn lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- npc_frostsworn_general JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 —
-- the auriaya engage-port precedent).
-- npc_frostsworn_general JustDied — Talk(SAY_DEATH 1) (event 4 — the
-- sjonnir JustDied-Talk precedent).
-- DOCUMENTED-ONLY (in this header; only entry 36723 registered):
-- npc_frostsworn_general Reset / JustEngagedWith / JustDied instance
-- legs — _instance->SetData(DATA_FROSTSWORN_GENERAL, NOT_STARTED /
-- IN_PROGRESS / DONE) — no instance bridge.
-- npc_frostsworn_general UpdateAI — EVENT_SHIELD / EVENT_SPIKE /
-- EVENT_CLONE scheduler machine — no-timer-bridge (the boss_toravon
-- precedent); DoZoneInCombat has no bridge.
-- The jaina/sylvanas intro Talk machine (SAY_JAINA_INTRO_1..11,
-- SAY_SYLVANAS_INTRO_1..8, SAY_UTHER_INTRO_A2/H2_*, SAY_LK_INTRO_1..3) and
-- the escape Talk machine all ride instance SetData / DoAction legs —
-- no instance-script bridge (the instance_violet_hold precedent); the
-- instance_processEvent captain Talk legs (SAY_CAPTAIN_FIRE /
-- SAY_CAPTAIN_FINAL) were already documented in the
-- instance_halls_of_reflection audit.
-- npc_the_lich_king_escape_hor KilledUnit — DoPlaySoundToSet
-- (SOUND_LK_SLAY_1/2) has no sound bridge (the npc_fel_geyser
-- precedent).
-- npc_uther_quel_delar UpdateAI EVENT_UTHER_1..9 Talk machine
-- (SAY_UTHER_QUEL_DELAR_1..5) — timer-driven, no-timer-bridge; the
-- DamageTaken clamp leg and the ACTION_UTHER_START_SCREAM /
-- ACTION_UTHER_OUTRO DoAction legs have no bridges.
-- npc_quel_delar_sword — no own Talk; EVENT_QUEL_DELAR_INIT /
-- EVENT_QUEL_DELAR_FLIGHT_INIT / Takeoff legs are timer/motion-driven;
-- JustDied only DoActions uther (no-DoAction bridge).
-- The gauntlet-trash scripts (ghostly_priest, phantom_mage,
-- phantom_hallucination, shadowy_mercenary, spectral_footman,
-- tortured_rifleman) and escape-trash scripts (raging_ghoul,
-- risen_witch_doctor, lumbering_abomination, spiritual_reflection) — all
-- legs scheduler / cast / motion-driven (no-timer-bridge,
-- no-random-target-SelectTarget, no-motion queues), zero Talk.
-- The five AreaTriggerScripts (at_hor_intro_start,
-- at_hor_waves_restarter, at_hor_impenetrable_door, at_hor_shadow_throne,
-- at_hor_uther_quel_delar_start) join the unbridged-area-trigger queue
-- (the pit_of_saron cavern triggers precedent).
-- The four spell scripts (spell_hor_start_halls_of_reflection_quest_ae,
-- spell_hor_evasion, spell_hor_gunship_cannon_fire,
-- spell_hor_quel_delars_will) join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent).

local ENTRY_FROSTSWORN_GENERAL = 36723

local SAY_AGGRO = 0
local SAY_DEATH = 1

-- C++ npc_frostsworn_generalAI::JustEngagedWith: Talk(SAY_AGGRO) — the
-- auriaya engage-port precedent.
local function frostswornGeneralJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ npc_frostsworn_generalAI::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent.
local function frostswornGeneralJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 1, frostswornGeneralJustEngagedWith)
RegisterCreatureEvent(ENTRY_FROSTSWORN_GENERAL, 4, frostswornGeneralJustDied)
