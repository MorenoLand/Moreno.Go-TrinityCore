-- The Lich King (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_the_lich_king.cpp
-- (2899 lines incl. license; 10 CreatureScripts
-- (boss_the_lich_king (BossAI, DATA_THE_LICH_KING = 12),
-- npc_tirion_fordring_tft (ScriptedAI),
-- npc_shambling_horror_icc (ScriptedAI), npc_raging_spirit
-- (ScriptedAI), npc_valkyr_shadowguard (ScriptedAI),
-- npc_strangulate_vehicle (ScriptedAI), npc_terenas_menethil
-- (ScriptedAI), npc_spirit_warden (ScriptedAI), npc_spirit_bomb
-- (CreatureAI), npc_broken_frostmourne (CreatureAI)) + 31
-- SpellScript/AuraScript legs (spell_the_lich_king_infest;
-- spell_the_lich_king_necrotic_plague;
-- spell_the_lich_king_necrotic_plague_jump +
-- spell_the_lich_king_necrotic_plague_jump_aura (pair);
-- spell_the_lich_king_shadow_trap_visual;
-- spell_the_lich_king_shadow_trap_periodic; spell_the_lich_king_quake;
-- spell_the_lich_king_ice_burst_target_search;
-- spell_the_lich_king_raging_spirit; spell_the_lich_king_defile;
-- spell_the_lich_king_summon_into_air;
-- spell_the_lich_king_soul_reaper;
-- spell_the_lich_king_valkyr_target_search;
-- spell_the_lich_king_cast_back_to_caster;
-- spell_the_lich_king_life_siphon; spell_the_lich_king_vile_spirits;
-- spell_the_lich_king_vile_spirits_visual;
-- spell_the_lich_king_vile_spirit_move_target_search;
-- spell_the_lich_king_vile_spirit_damage_target_search;
-- spell_the_lich_king_harvest_soul;
-- spell_the_lich_king_lights_favor; spell_the_lich_king_soul_rip;
-- spell_the_lich_king_restore_soul; spell_the_lich_king_dark_hunger;
-- spell_the_lich_king_in_frostmourne_room;
-- spell_the_lich_king_summon_spirit_bomb;
-- spell_the_lich_king_trigger_vile_spirit; spell_the_lich_king_jump;
-- spell_the_lich_king_jump_remove_aura; spell_the_lich_king_play_movie;
-- spell_the_lich_king_harvest_souls_teleport) + 1 generic
-- RegisterSpellScriptWithArgs spell_trigger_spell_from_caster leg
-- ("spell_the_lich_king_mass_resurrection", SPELL_MASS_RESURRECTION_REAL)
-- + 2 AchievementCriteriaScripts (achievement_been_waiting_long_time,
-- achievement_neck_deep_in_vile); all registered from inside
-- AddSC_boss_the_lich_king(); loader decl 183 / call 378 per
-- northrend_script_loader.cpp — the THIRTEENTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sindragosa() (call 377) — verified from the loader
-- this run; the checkpoint sequence (sindragosa -> the_lich_king) is
-- followed).
-- Entry: 36597 The Lich King (icecrown_citadel.h NPC_THE_LICH_KING,
-- line 314; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_THE_LICH_KING, line 314) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- RegisterIcecrownCitadelCreatureAI ScriptName bindings are DB-side
-- as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_the_lich_king" hits boss_the_lich_king.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction.
-- Eluna creature events: 3 OnKill.
-- Ported arms (C++-exact for all modeled arms):
-- boss_the_lich_king KilledUnit — Talk(SAY_LK_KILL 10) gated on
-- victim->GetTypeId() == TYPEID_PLAYER && !me->IsInEvadeMode()
-- (event 3 — the razuvious player-gated variant + the yogg_saron
-- IsInEvadeMode precedent; the IsInEvadeMode bridge was re-verified
-- this run (engine/world/lua_objects.go:623) and the evade leg ported
-- 2026-10-16; the !events.IsInPhase(PHASE_OUTRO) leg still has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; only entry 36597 registered):
-- boss_the_lich_king DoAction — PORTED 2026-11-06 (lichKingDoAction,
-- ev38): ACTION_START_ENCOUNTER writes IN_PROGRESS + Talk(SAY_LK_INTRO_1 0)
-- + SetStandState(STAND); Talk(SAY_LK_FROSTMOURNE_ESCAPE 8, non-heroic)
-- rides ACTION_TELEPORT_BACK. Both receivers are dormant — the sender
-- chains (Tirion intro / terenas EVENT_TELEPORT_BACK + restore_soul
-- SpellScript) are unported — the krick ACTION_OUTRO receiver precedent.
-- SetZoneMusic has no bridge; the strangulate-vehicle summons relay has
-- no summon-list bridge (the vehicle ev38 receiver IS ported).
-- No FAIL leg exists in C++ for the boss: no JustReachedHome override
-- (the :1228 one belongs to npc_tirion_fordring_tft) and the
-- EnterEvadeMode override (:594) calls _DespawnAtEvade() directly
-- without the BossAI base — correctly absent.
-- boss_the_lich_king SpellHit — Talk(SAY_LK_FROSTMOURNE_KILL 9) on
-- SPELL_HARVESTED_SOUL 72679 (:813-816), gated me->IsInCombat() &&
-- !IsHeroic() — stays unmodeled on trigger-path grounds. The C++ arm
-- fires when the boss is HIT BY the spell, i.e. Eluna event 14
-- (CreatureEventOnHitBySpell — BRIDGED: boss_ai.go:2052-2096
-- creature-caster path + spells.go:11709-11730 player-caster path; the
-- attumen/terestian/halazzi precedent; the 2026-10-17 "no event-14
-- bridge" correction was itself wrong and is retracted here). But the
-- SOLE caster of 72679 is the harvest-soul / in-frostmourne-room
-- AuraScript OnRemove (C++ :2547-2552 / :2692-2697: the dead
-- Frostmourne-room player casts it on the LK) — no AuraScript bridge
-- exists, so event 14 would never fire for it in Go (the rotface /
-- infinite-corruptor trigger-path precedent). The old "event 15 never
-- fires" rationale conflated the two events: event 15
-- (OnSpellHitTarget) fires on the CASTER's bindings and was never the
-- right hook for this boss-side arm. Corrected 2026-10-17.
-- The second SpellHit arm (:818-820, REMORSELESS_WINTER_1/2 68981/
-- 72259 —> SetZoneOverrideLight + LIGHT_SNOW weather) also stays out
-- on trigger-path grounds: the casts that would hit the boss ride
-- MovementInform POINT_CENTER_1/2 (unported, see the MISSING-CAST
-- triage below), so event 14 would never fire for them in Go. Both
-- legs are now bridged (map:SetZoneOverrideLight +
-- map:SetZoneWeather); porting the arm without its trigger would
-- fabricate it.
-- boss_the_lich_king MovementInform — Talk(SAY_LK_REMORSELESS_WINTER
-- 4) on POINT_CENTER_1 / POINT_CENTER_2; tirion->AI()->
-- Talk(SAY_TIRION_OUTRO_2) is cross-AI Talk on POINT_LK_OUTRO_2 (no
-- MovementInform bridge; no cross-AI-Talk bridge).
-- boss_the_lich_king UpdateAI scheduler Talk legs — Talk(SAY_LK_INTRO_2
-- 1) (EVENT_INTRO_TALK_1); Talk(SAY_LK_INTRO_3 2)
-- (EVENT_INTRO_CAST_FREEZE); Talk(EMOTE_NECROTIC_PLAGUE_WARNING 13,
-- target) (EVENT_NECROTIC_PLAGUE); Talk(EMOTE_DEFILE_WARNING 12)
-- (EVENT_DEFILE); Talk(SAY_LK_HARVEST_SOUL 7) (EVENT_HARVEST_SOUL,
-- EVENT_HARVEST_SOULS); Talk(SAY_LK_QUAKE 5) (EVENT_QUAKE,
-- EVENT_QUAKE_2); Talk(SAY_LK_SUMMON_VALKYR 6) (EVENT_SUMMON_VALKYR);
-- Talk(SAY_LK_OUTRO_1..8 14..21) (EVENT_OUTRO_TALK_1..8);
-- Talk(SAY_LK_BERSERK 11) (EVENT_BERSERK) — all timer-driven;
-- the "no-timer-bridge" note is RETRACTED this run (2026-10-16) —
-- the timer bridge (CreateLuaEvent) exists and is used in this file;
-- the gated arms stay unported because the phase machine
-- (PHASE_INTRO/PHASE_ONE/PHASE_TWO/PHASE_TRANSITION/PHASE_OUTRO/
-- PHASE_FROSTMOURNE) has no Lua model, not because timers lack a
-- bridge (the gothik counterpoint: no invented phase-agnostic
-- timers); the intro/outro
-- movement machines, DoCast summon legs, and phase SetPhase legs ride
-- bridges that do not exist; the JustDied MoveFall leg still does,
-- while the SPELL_PLAY_MOVIE (73159), SetDisableGravity(false), and
-- zone-light + zone-weather JustDied legs ARE ported (ev4
-- lichKingJustDied).
-- MISSING-CAST TRIAGE (2026-10-16; C++ line cites from
-- boss_the_lich_king.cpp) — all 12 BLOCKED, none ported:
-- 68981 SPELL_REMORSELESS_WINTER_1 (:849) / 72259
-- SPELL_REMORSELESS_WINTER_2 (:863) — DoCastSelf in MovementInform
-- case POINT_CENTER_1 / POINT_CENTER_2 (Remorseless Winter channel
-- opener, transition-phase entry); no MovementInform bridge — same
-- unbridged arm as the documented SAY_LK_REMORSELESS_WINTER legs.
-- 73213 SPELL_EMOTE_SHOUT_NO_SHEATH (:929) — DoCastSelf non-triggered
-- in case EVENT_EMOTE_CAST_SHOUT, scheduled +7s/+18s in PHASE_INTRO;
-- porting on a bare engage timer would fabricate intro-phase casts
-- mid-combat (no phase machine).
-- 70372 SPELL_SUMMON_SHAMBLING_HORROR (:947, PHASE_ONE, 1min re-arm),
-- 70358 SPELL_SUMMON_DRUDGE_GHOULS (:952, PHASE_ONE, 30s re-arm),
-- 70541 SPELL_INFEST (:956, PHASE_ONE seeded, re-arms PHASE_ONE or
-- PHASE_TWO by current phase), 69409 SPELL_SOUL_REAPER (:973,
-- DoCastVictim, PHASE_TWO_THREE, 33-35s re-arm) — phase-gated timer
-- arms (JustEngagedWith :578-581 seeds the PHASE_ONE set); a
-- phase-agnostic engage timer would fabricate phase-2/3/transition
-- casts — the gothik counterpoint, no invented phase machines.
-- 71797 SPELL_LIGHTS_BLESSING (:1263, case EVENT_OUTRO_BLESS) — cast
-- by npc_tirion_fordring_tft on itself (outro); tirion not registered
-- in this file — wrong creature.
-- 74352 SPELL_WINGS_OF_THE_DAMNED (:1455, val'kyr Reset, non-triggered),
-- 73501 SPELL_VALKYR_TRANSFORM (:1522, val'kyr MovementInform
-- POINT_SIPHON), 73488 SPELL_LIFE_SIPHON (:1562, val'kyr
-- EVENT_LIFE_SIPHON DoCastVictim) — cast by npc_valkyr_shadowguard,
-- not the boss; val'kyr not registered in this file — wrong creature.
-- 69382 SPELL_LIGHTS_FAVOR (:1704, terenas DoAction
-- ACTION_FROSTMOURNE_INTRO) — cast by npc_terenas_menethil on itself;
-- terenas not registered in this file — wrong creature.
-- npc_tirion_fordring_tft (NPC_HIGHLORD_TIRION_FORDRING_LK 38995) —
-- NOT registered: Talk(SAY_TIRION_INTRO_1 0) rides DoAction
-- ACTION_CONTINUE_INTRO (ev38 dispatch bridged; sender LK MovementInform POINT_LK_INTRO_3 (boss_the_lich_king.cpp:839-843) unported — no MovementInform bridge — and receiver tirion unregistered); Talk(SAY_TIRION_INTRO_2
-- 1) (EVENT_INTRO_TALK_1) and Talk(SAY_TIRION_OUTRO_1 2)
-- (EVENT_OUTRO_TALK_1) are scheduler-driven on the PHASE_INTRO /
-- PHASE_OUTRO machines (CreateLuaEvent exists, but the unbridged
-- phase/machine drivers gate them — gothik counterpoint); the
-- OnGossipSelect intro kick, MovementInform, and SpellHit legs have no
-- bridges (the bronjahm npc_corrupted_soul_fragment precedent).
-- npc_terenas_menethil (NPC_TERENAS_MENETHIL_FROSTMOURNE 36823,
-- NPC_TERENAS_MENETHIL_FROSTMOURNE_H 39217) — DamageTaken PORTED
-- (terenasDamageTaken, ev9, this tick); DoAction ACTION_TELEPORT_BACK
-- PORTED 2026-11-05 (tick 215) via terenasDoAction (ev38) + the
-- spiritWardenDied JustDied relay (ev4). Talks/scheduler legs below
-- remain NOT registered as stated:
-- Talk(SAY_TERENAS_INTRO_1..3 0..2) (EVENT_FROSTMOURNE_TALK_1..3,
-- DoAction ACTION_FROSTMOURNE_INTRO scheduler) and
-- Talk(SAY_TERENAS_OUTRO_1..2 0..1) (EVENT_OUTRO_TERENAS_TALK_1..2,
-- IsSummonedBy hook) — ev38 dispatch bridged; intro talks gated on
-- ACTION_FROSTMOURNE_INTRO senders LK EVENT_FROSTMOURNE_HEROIC
-- (boss_the_lich_king.cpp:1073) + instance EVENT_TELEPORT_TO_FROSTMOURNE
-- (instance_icecrown_citadel.cpp:1463), both unported; outro talks gated on the
-- unported outro summon chain (broken-frostmourne ACTION_SUMMON_TERENAS,
-- :883/:1931) — ev22 IsSummonedBy dispatch IS live for creature summons
-- (lua_creature_events.go:191-199 + :6799-6820; the "fires only for
-- player pets (pet_cooldowns.go:73)" claim was stale — corrected by
-- the strangulate-vehicle ev22 port below); a bare ev22 port
-- would fabricate outro Talks in the Frostmourne room (the FROSTMOURNE_INTRO
-- cancel never fires in Lua)
-- (gothik counterpoint).
-- PORTED (2026-10-17 timer re-sweep; in-house spot-verified vs C++
-- :1308-1313 + runtime before applying; delegated audit reconciled):
-- npc_shambling_horror_icc's heroic-only DamageTaken frenzy latch —
-- the "no difficulty bridge" header claim was STALE (IsHeroic is
-- bridged, lua_creature_events.go:260; event 9 DamageTaken fires
-- pre-application with attacker+damage, lua_creature_events.go:303).
-- HealthBelowPctDamaged(20, damage) ≡ post-damage health below
-- floor(maxHealth*20/100) (algalon precedent); the _frenzied latch
-- never clears (ctor-only init, Reset doesn't touch it) so the
-- GUID-keyed table is never cleared — not in the leave-combat or
-- reset handlers. No damage return needed.
-- (37698) — SHOCKWAVE 72149 self-cast {20s,25s} init/re-arm
-- (:1303/:1331-1332); ENRAGE 72143 self-cast {11s,14s}/{20s,25s}
-- (:1304/:1335-1337 — the 1s cast-fail recheck leg is unmodelable,
-- documented); the heroic-only DamageTaken frenzy latch
-- (:1308-1313, SPELL_FRENZY 28747 triggered at
-- HealthBelowPctDamaged(20))
-- npc_raging_spirit (36701) — SOUL_SHRIEK 69242 DoCastAOE
-- {12s,15s} init/re-arm, modeled as self-cast (the DoCastAOE
-- self-cast convention :1367/:1420-1421) + Reset one-shots
-- DoCastSelf(SPELL_PLAGUE_AVOIDANCE 72846, triggered) +
-- DoCastSelf(SPELL_RAGING_SPIRIT_VISUAL 69197, triggered) (:1368-1369)
-- + summoner (Lich King) CastSpell(me, SPELL_RAGING_SPIRIT_VISUAL_CLONE
-- 69198, triggered) (:1371-1372, via creature:GetSummonerUnit, bridged
-- 2026-11-05) + DoCastSelf(SPELL_BOSS_HITTIN_YA 73878, triggered) (:1374)
-- (the curator engage-cast precedent);
-- npc_spirit_warden (36824) — SOUL_RIP 69397 victim-cast
-- {12s,15s}/{23s,27s} (:1833/:1855-1856) + Reset one-shot
-- 1-arg DoCast(SPELL_DARK_HUNGER 69383) non-triggered (:1835) —
-- target resolved from DBC AISpellInfo (UnitAI.cpp:113), kept as
-- self-cast (self-buff aura).
-- npc_valkyr_shadowguard / npc_spirit_bomb /
-- npc_broken_frostmourne — zero Talk lines —
-- NOT registered (the bronjahm npc_corrupted_soul_fragment
-- precedent); the valkyr grab/life-siphon (SpellScript +
-- MovementInform), bomb and frostmourne UpdateAI
-- machines ride unbridged legs. npc_strangulate_vehicle IS
-- registered — IsSummonedBy + UpdateAI timer legs + DoAction
-- ACTION_TELEPORT_BACK PORTED 2026-11-05 (see below).
-- The 31 spell/aura legs + the generic spell_trigger_spell_from_caster
-- with-args leg join the no-SpellScript-bridge / no-AuraScript-bridge
-- queues (the boss_moragg optic-link precedent);
-- achievement_been_waiting_long_time and
-- achievement_neck_deep_in_vile join the no-achievement-criteria-bridge
-- queue (the lana'thel achievement precedent).
-- All 33 Talk() calls in the file accounted for (33 = SAY_LK_KILL 10
-- (1, PORTED — the only bridgeable arm) + SAY_LK_INTRO_1 0 (1,
-- DoAction) + SAY_LK_FROSTMOURNE_ESCAPE 8 (1, DoAction) +
-- SAY_LK_FROSTMOURNE_KILL 9 (1, SpellHit) + SAY_LK_REMORSELESS_WINTER 4
-- (2, MovementInform) + SAY_TIRION_OUTRO_2 (1, cross-AI MovementInform)
-- + SAY_LK_INTRO_2 1 (1, scheduler) + SAY_LK_INTRO_3 2 (1,
-- scheduler) + EMOTE_NECROTIC_PLAGUE_WARNING 13 (1, scheduler) +
-- EMOTE_DEFILE_WARNING 12 (1, scheduler) + SAY_LK_HARVEST_SOUL 7 (2,
-- scheduler) + SAY_LK_QUAKE 5 (2, scheduler) + SAY_LK_SUMMON_VALKYR 6
-- (1, scheduler) + SAY_LK_OUTRO_1..8 14..21 (8, scheduler) +
-- PORTED (2026-10-13): EVENT_BERSERK — 15min single-fire —
-- Talk(SAY_LK_BERSERK 11) + DoCastSelf(SPELL_BERSERK2 47008)
-- non-triggered (:583/:1141-1143), bare timer across all phases.
-- SAY_TIRION_INTRO_1 0 (1, tirion
-- DoAction) + SAY_TIRION_INTRO_2 1 (1, tirion scheduler) +
-- SAY_TIRION_OUTRO_1 2 (1, tirion scheduler) + SAY_TERENAS_INTRO_1..3
-- 0..2 (3, terenas scheduler) + SAY_TERENAS_OUTRO_1..2 0..1 (2,
-- terenas scheduler)).

-- REACTSTATE TRIAGE (2026-10-22; C++ cites from
-- boss_the_lich_king.cpp) — all 21 hunt flags:
-- PORTED: npc_raging_spirit Reset REACT_PASSIVE (:1365) +
-- EVENT_SET_AGRESSIVE 2s -> REACT_AGGRESSIVE (:1424) — see the
-- ragingSpiritJustEngagedWith comment above; the DoAction
-- ACTION_DISABLE_RAGING REACT_PASSIVE leg (:1377-1386) stays out (no
-- DoAction bridge, documented above).
-- STAYS OUT (no new bridges built):
-- LK Reset REACT_PASSIVE (:529-532) — pairs with EVENT_FINISH_INTRO
-- REACT_AGGRESSIVE (:941-946); the intro machine (SetImmuneToPC,
-- DoZoneInCombat, phase SetPhase, MovementInform-driven) is unported,
-- so Reset PASSIVE alone would strand the boss passive permanently.
-- LK DamageTaken REACT_PASSIVE legs (:690/:700/:709, 70%/40%/10%
-- phase transitions) — gated on the unported phase machine and paired
-- with MovePoint/AttackStop movement legs (no MovementInform bridge).
-- LK EVENT_HARVEST_SOULS REACT_PASSIVE (:1044) + EVENT_START_ATTACK
-- REACT_AGGRESSIVE (:1028-1031) — unported PHASE_FROSTMOURNE/PHASE_THREE
-- machine + unported Harvest Souls cast.
-- JustSummoned REACT_PASSIVE + LichKingStartMovementEvent 5s ->
-- REACT_AGGRESSIVE + AttackStart (:739-741/:447-466, shambling horror /
-- drudge ghoul): cross-creature timed BasicEvent machine;
-- AttackStart/SelectTarget have no bridge. (VileSpiritActivateEvent
-- REACT_AGGRESSIVE (:472-484) + JustSummoned legs :750/:761/:777/1059
-- (ice-sphere :750 PORTED in lichKingJustSummoned — SelectTarget
-- approximated by the alive same-map/instance random-player pool,
-- MoveFollow PORTED in lichKingJustSummoned; defile :761 PORTED (lichKingJustSummoned);
-- spirit_bomb/valkyr stay unregistered as documented; strangulate
-- PORTED below (ev22 + UpdateAI timers + ev38).
-- SpellScript legs: ice_burst_target_search REACT_PASSIVE (:2212-2221)
-- and restore_soul REACT_PASSIVE on wicked spirits (:2632-2641) — no
-- SpellScript bridge; both add AIs unregistered (restore_soul also rides LK-side ACTION_TELEPORT_BACK — ev38 dispatch
-- bridged, but sender restore_soul SpellScript (boss_the_lich_king.cpp:2629)
-- unported, no SpellScript bridge, and the LK-side arm itself unported; cf.
-- terenas-side ACTION_TELEPORT_BACK, ported tick 215 via ev38.)

-- CAST-STATE GATES PORTED (2026-11-04): boss :900/:1149
-- (TRANSITION/OUTRO/FROSTMOURNE-exempt in C++, unconditional in Lua —
-- no phase model) gates the berserk one-shot; horror :1323 (pre-loop
-- only) gates shockwave/enrage; raging :1412 (pre-loop only) gates
-- soul-shriek + the set-aggressive flip; warden :1843 UpdateAI is a
-- plain loop (zero CASTING hits) so soul-rip stays ungated;
-- valkyr :1544/:1576 gates cover the unported grab machine only;
-- tirion :1240 gate covers unported intro/outro talks only;
-- strangulate/terenas/bomb/frostmourne UpdateAIs ungated (zero hits);
-- terenas 2s despawns are DespawnOrUnsummon, not scheduler events.
local ENTRY_THE_LICH_KING = 36597
local NPC_FROSTMOURNE_TRIGGER = 38584 -- boss_the_lich_king.h:331
local ENTRY_SHAMBLING_HORROR = 37698
local ENTRY_RAGING_SPIRIT = 36701
local ENTRY_SPIRIT_WARDEN = 36824
local ENTRY_ICE_SPHERE = 36633 -- icecrown_citadel.h:321 (NPC_ICE_SPHERE)
local ENTRY_DRUDGE_GHOUL = 37695 -- icecrown_citadel.h:320
local ENTRY_VALKYR_SHADOWGUARD = 36609 -- icecrown_citadel.h:324 (NPC_VALKYR_SHADOWGUARD)
local ENTRY_VILE_SPIRIT = 37799 -- icecrown_citadel.h:325 (NPC_VILE_SPIRIT)
local ENTRY_SHADOW_TRAP = 39137 -- icecrown_citadel.h:332
local ENTRY_DEFILE = 38757 -- icecrown_citadel.h:323
local ENTRY_TERENAS = 36823 -- icecrown_citadel.h:316 (NPC_TERENAS_MENETHIL_FROSTMOURNE)
local ENTRY_TERENAS_HEROIC = 39217 -- icecrown_citadel.h:318
local ENTRY_STRANGULATE_VEHICLE = 36598 -- icecrown_citadel.h:327 (NPC_STRANGULATE_VEHICLE)
local AREA_ICECROWN_CITADEL = 4812 -- icecrown_citadel.h:540
local WEATHER_STATE_FOG = 1 -- Weather.h:48
local WEATHER_STATE_BLACKSNOW = 106 -- Weather.h:61
local LIGHT_DEFAULT = 2488 -- boss_the_lich_king.cpp:349
local LIGHT_FOG = 2509 -- boss_the_lich_king.cpp:352
local LIGHT_SOULSTORM = 2508 -- boss_the_lich_king.cpp:351

local SAY_LK_KILL = 10
local SAY_LK_BERSERK = 11
local SAY_LK_INTRO_1 = 0 -- boss_the_lich_king.cpp:38
local SAY_LK_FROSTMOURNE_ESCAPE = 8 -- boss_the_lich_king.cpp:45
local DATA_THE_LICH_KING = 12 -- icecrown_citadel.h:86
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3
local ACTION_START_ENCOUNTER = 0 -- boss_the_lich_king.cpp:335 (EncounterActions)

local SPELL_BERSERK2 = 47008
local SPELL_HORROR_SHOCKWAVE = 72149
local SPELL_HORROR_ENRAGE = 72143
local SPELL_HORROR_FRENZY = 28747
local SPELL_SOUL_SHRIEK = 69242
local SPELL_PLAGUE_AVOIDANCE = 72846
local SPELL_RAGING_SPIRIT_VISUAL = 69197
local SPELL_RAGING_SPIRIT_VISUAL_CLONE = 69198 -- boss_the_lich_king.cpp:109
local SPELL_BOSS_HITTIN_YA = 73878
local SPELL_ICE_SPHERE = 69090 -- boss_the_lich_king.cpp:103
local SPELL_ICE_BURST_TARGET_SEARCH = 69109 -- boss_the_lich_king.cpp:104
local SPELL_ICE_PULSE = 69091 -- boss_the_lich_king.cpp:105
local SPELL_DARK_HUNGER = 69383
local SPELL_SOUL_RIP = 69397
local SPELL_KILL_FROSTMOURNE_PLAYERS = 75127
local SPELL_PLAY_MOVIE = 73159
local SPELL_RISEN_WITCH_DOCTOR_SPAWN = 69639 -- boss_the_lich_king.cpp:87
local SPELL_SHADOW_TRAP_VISUAL = 73530 -- boss_the_lich_king.cpp:96
local SPELL_DEFILE_AURA = 72743 -- boss_the_lich_king.cpp:115
local SPELL_BROKEN_FROSTMOURNE = 72398 -- boss_the_lich_king.cpp:174
local SPELL_TERENAS_LOSES_INSIDE = 72572 -- boss_the_lich_king.cpp:157
local SPELL_DESTROY_SOUL = 74086 -- boss_the_lich_king.cpp:154
local SPELL_RESTORE_SOUL = 72595 -- boss_the_lich_king.cpp:150
local SPELL_HARVEST_SOUL_VEHICLE = 68984 -- boss_the_lich_king.cpp:138
local SPELL_HARVEST_SOUL_VISUAL = 71372 -- boss_the_lich_king.cpp:139
local SPELL_HARVEST_SOUL_TELEPORT = 72546 -- boss_the_lich_king.cpp:140
local SPELL_HARVEST_SOULS_TELEPORT = 73655 -- boss_the_lich_king.cpp:141
local SPELL_HARVEST_SOUL_TELEPORT_BACK = 72597 -- boss_the_lich_king.cpp:142
local SPELL_HARVEST_SOUL_10H = 74296 -- boss_the_lich_king.cpp:193 (HARVEST_SOUL RAID_MODE, 10H)
local SPELL_HARVEST_SOUL_25H = 74297 -- boss_the_lich_king.cpp:193 (HARVEST_SOUL RAID_MODE, 25H)
local ACTION_TELEPORT_BACK = 8 -- boss_the_lich_king.cpp:343 (EncounterActions)
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local EMOTE_ONESHOT_EMERGE = 449 -- SharedDefines.h:2086

-- creature:SetReactState is bridged (REACT_PASSIVE=0, REACT_AGGRESSIVE=2).
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

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

-- C++ EVENT_BERSERK: 15min single-fire — Talk(SAY_LK_BERSERK) +
-- DoCastSelf(SPELL_BERSERK2) non-triggered (:583/:1141-1143), bare
-- timer across all phases (EVENT_GROUP_BERSERK carries no phase).
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :900 (+ in-loop :1149, TRANSITION/OUTRO/FROSTMOURNE-exempt — no phase model in Lua, gate applies unconditionally): 500ms IsCasting defer, cast not consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(SAY_LK_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK2)
end

-- C++ boss_the_lich_king::DoAction ACTION_START_ENCOUNTER (:615-622):
-- SetBossState IN_PROGRESS + Talk(SAY_LK_INTRO_1) + SetZoneMusic
-- (no bridge) + SetStandState(STAND) + EVENT_INTRO_MOVE_1 4s (the
-- unported intro-movement machine). The sender is Tirion's intro
-- chain (unregistered), so the receiver is dormant until that chain
-- lands — the krick ACTION_OUTRO receiver precedent. PORTED 2026-11-06.
-- ACTION_TELEPORT_BACK (:654-661): Talk(SAY_LK_FROSTMOURNE_ESCAPE 8,
-- non-heroic) + strangulate-vehicle summons relay (no summon-list
-- bridge — the vehicle ev38 receiver IS ported); the sender is the
-- terenas EVENT_TELEPORT_BACK / restore_soul SpellScript chain, both
-- unported, so this arm is dormant too.
local function lichKingDoAction(event, creature, action)
    if action == ACTION_START_ENCOUNTER then
        creature:SetBossState(DATA_THE_LICH_KING, BOSS_STATE_IN_PROGRESS)
        creature:Talk(SAY_LK_INTRO_1)
        creature:SetStandState(UNIT_STAND_STATE_STAND)
    elseif action == ACTION_TELEPORT_BACK then
        if not creature:IsHeroic() then
            creature:Talk(SAY_LK_FROSTMOURNE_ESCAPE)
        end
    end
end

-- C++ boss_the_lich_king::JustEngagedWith schedules EVENT_BERSERK
-- (:583); the phase-one machine arms are unbridged. No C++ Talk on
-- engage, and JustEngagedWith writes NO boss state — IN_PROGRESS rides
-- DoAction ACTION_START_ENCOUNTER (ported above), so no state leg is
-- added here (the fabricated-leg trap). Event 1 registered for the
-- berserk timer only.
local function lichKingJustEngagedWith(event, creature, target)
    creature:SetCombatPulseDelay(5) -- boss_the_lich_king.cpp:575 (JustEngagedWith arm; the setActive leg still has no bridge)
    creature:DoZoneInCombat() -- boss_the_lich_king.cpp:576 (JustEngagedWith arm; bridge live engine/world/lua_creature_events.go, 2026-10-23)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 900000, function() onBerserk(creature, guid) end)
end

-- C++ _Reset() timer machinery — cancel/re-arm on combat events
-- (the maiden convention). C++ boss_the_lich_king::EnterEvadeMode
-- (:594): DoCastAOE(SPELL_KILL_FROSTMOURNE_PLAYERS 75127) non-triggered,
-- ungated — modeled as self-cast (file's DoCastAOE idiom, line 104).
-- The tirion-despawn / strangulate-vehicle teleport-back / summons
-- DespawnAll / _DespawnAtEvade legs have no bridge — documented.
local function lichKingLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_KILL_FROSTMOURNE_PLAYERS)
end

local function lichKingReset(event, creature)
    -- C++ Reset :530 — _Reset() first: SetBossState NOT_STARTED only
    -- when the stored state != DONE (the rotface precedent — ev23
    -- fires on spawn/death/evade too, and a DONE boss must stay
    -- DONE). PORTED 2026-11-06 via creature:SetBossState.
    if creature:GetBossState(DATA_THE_LICH_KING) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_THE_LICH_KING, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUID())
    -- C++ Reset :543: clear any light override (0, 5000ms).
    creature:GetMap():SetZoneOverrideLight(AREA_ICECROWN_CITADEL, LIGHT_DEFAULT, 0, 5000)
end

-- C++ boss_the_lich_king::JustDied (boss_the_lich_king.cpp:549-562):
-- _JustDied() first (:551, SetBossState DONE — PORTED 2026-11-06 via
-- creature:SetBossState) then DoCastAOE(SPELL_PLAY_MOVIE 73159, false).
-- DoCastAOE is self-cast per UnitAI.h:322, so
-- creature:CastSpell(creature, SPELL_PLAY_MOVIE).
-- The MoveFall and 25-man quest-credit legs ride unbridged machines;
-- the FOG zone-light + zone-weather legs are ported below, in C++
-- order (map:SetZoneOverrideLight, then map:SetZoneWeather).
local function lichKingJustDied(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ :551 — _JustDied() first: SetBossState DONE, before the
    -- PLAY_MOVIE cast (C++ order). PORTED 2026-11-06 via
    -- creature:SetBossState.
    creature:SetBossState(DATA_THE_LICH_KING, BOSS_STATE_DONE)
    creature:CastSpell(creature, SPELL_PLAY_MOVIE)
    creature:SetDisableGravity(false) -- C++ :553, after the movie cast
    -- C++ :555-556: FindNearestCreature(NPC_FROSTMOURNE_TRIGGER, 50.0f)
    -- -> DespawnOrUnsummon (FindNearestCreature and Despawn both bridged;
    -- stale "unbridged machines" claim retracted).
    local frostmourne = creature:FindNearestCreature(NPC_FROSTMOURNE_TRIGGER, 50.0)
    if frostmourne then
        frostmourne:Despawn()
    end
    -- C++ :557-558: SetZoneOverrideLight FOG + SetZoneWeather FOG,
    -- after the frostmourne despawn as in C++ order.
    creature:GetMap():SetZoneOverrideLight(AREA_ICECROWN_CITADEL, LIGHT_DEFAULT, LIGHT_FOG, 5000)
    creature:GetMap():SetZoneWeather(AREA_ICECROWN_CITADEL, WEATHER_STATE_FOG, 0.0)
end

-- C++ boss_the_lich_king::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER &&
--     !me->IsInEvadeMode() && !events.IsInPhase(PHASE_OUTRO))
--     Talk(SAY_LK_KILL) — the razuvious player-gated variant precedent
-- (the evade leg ported 2026-10-16; the phase leg has no bridge).
local function lichKingKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" and not creature:IsInEvadeMode() then
        creature:Talk(SAY_LK_KILL)
    end
end

-- C++ boss_the_lich_king::JustSummoned NPC_ICE_SPHERE case
-- (boss_the_lich_king.cpp:746-756): random-player gate, then
-- SetReactState(REACT_PASSIVE) + CastSpell(summon, SPELL_ICE_SPHERE)
-- + CastSpell(summon, SPELL_ICE_BURST_TARGET_SEARCH) +
-- CastSpell(target, SPELL_ICE_PULSE), all non-triggered, in C++
-- order; MoveFollow stays out (no motion bridge). No target ->
-- DespawnOrUnsummon. C++ SelectTarget(Random, 0, 0.0f, player-only)
-- (boss_the_lich_king.cpp:748) over the threat list: offset 0, so the
-- tank stays in the pool and nothing is excluded; dead entries are
-- offline refs, skipped. The old map-player pool is retired.
local function randomPlayerAnywhere(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function lichKingJustSummoned(_, creature, summoned)
    -- C++ boss_the_lich_king::JustSummoned (:737-741): horror/ghoul
    -- spawn visual (triggered) + REACT_PASSIVE + emerge emote, in C++
    -- order. The 5s LichKingStartMovementEvent leg stays out (timed
    -- BasicEvent + AttackStart/SelectTarget have no bridge).
    if summoned:GetEntry() == ENTRY_SHAMBLING_HORROR or summoned:GetEntry() == ENTRY_DRUDGE_GHOUL then
        summoned:CastSpell(summoned, SPELL_RISEN_WITCH_DOCTOR_SPAWN, true)
        summoned:SetReactState(0)
        summoned:PerformEmote(EMOTE_ONESHOT_EMERGE)
    -- C++ :743-745: shadow trap spawn visual (triggered).
    elseif summoned:GetEntry() == ENTRY_SHADOW_TRAP then
        summoned:CastSpell(summoned, SPELL_SHADOW_TRAP_VISUAL, true)
    -- C++ :761-763: defile REACT_PASSIVE + defile aura (non-triggered).
    elseif summoned:GetEntry() == ENTRY_DEFILE then
        summoned:SetReactState(0)
        summoned:CastSpell(summoned, SPELL_DEFILE_AURA)
    -- C++ :766-772: broken-frostmourne visual (triggered; the C++
    -- nullptr target reads as a self-cast) + soulstorm light +
    -- BLACKSNOW zone weather, in C++ order; the 5s PHASE_OUTRO
    -- soul-barrage schedule stays out (phase machine).
    elseif summoned:GetEntry() == NPC_FROSTMOURNE_TRIGGER then
        summoned:CastSpell(summoned, SPELL_BROKEN_FROSTMOURNE, true)
        creature:GetMap():SetZoneOverrideLight(AREA_ICECROWN_CITADEL, LIGHT_DEFAULT, LIGHT_SOULSTORM, 10000)
        creature:GetMap():SetZoneWeather(AREA_ICECROWN_CITADEL, WEATHER_STATE_BLACKSNOW, 0.5)
    elseif summoned:GetEntry() == ENTRY_ICE_SPHERE then
        local target = randomPlayerAnywhere(creature)
        if target then
            summoned:SetReactState(0) -- REACT_PASSIVE
            summoned:CastSpell(summoned, SPELL_ICE_SPHERE)
            summoned:CastSpell(summoned, SPELL_ICE_BURST_TARGET_SEARCH)
            summoned:CastSpell(target, SPELL_ICE_PULSE)
            summoned:MoveFollow(target, 0.0, 0.0)
        else
            summoned:Despawn()
        end
    end
end

RegisterCreatureEvent(ENTRY_THE_LICH_KING, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 2, lichKingLeaveCombat)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 3, lichKingKilledUnit)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 4, lichKingJustDied)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 19, lichKingJustSummoned)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 23, lichKingReset)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 38, lichKingDoAction)

-- C++ boss_the_lich_king::SummonedCreatureDies (:795-808): the six add
-- entries flip to TEMPSUMMON_CORPSE_DESPAWN on death so the corpse
-- despawns at once instead of lingering to the prune. The Go bridge
-- takes the 0-based TempSummonType scheme (CORPSE_DESPAWN = 4,
-- pet_combat.go checkDeadTempSummon case 1,4); the C++ header enum is
-- 1-based (CORPSE_DESPAWN = 5 there) — do NOT "correct" this to 5.
local function lichKingSummonedCreatureDies(_, _, summoned)
    if not summoned then
        return
    end
    local entry = summoned:GetEntry()
    if entry == ENTRY_SHAMBLING_HORROR or entry == ENTRY_DRUDGE_GHOUL
        or entry == ENTRY_ICE_SPHERE or entry == ENTRY_VALKYR_SHADOWGUARD
        or entry == ENTRY_RAGING_SPIRIT or entry == ENTRY_VILE_SPIRIT then
        summoned:SetTempSummonType(4)
    end
end

RegisterCreatureEvent(ENTRY_THE_LICH_KING, 21, lichKingSummonedCreatureDies)

-- C++ npc_shambling_horror_icc: Reset seeds EVENT_SHOCKWAVE {20s,25s}
-- and EVENT_ENRAGE {11s,14s} (:1303-1304).
local function onHorrorShockwave(creature, guid)
    -- C++ horror UpdateAI pre-loop gate :1323 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "horror_shockwave", 500, function() onHorrorShockwave(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_HORROR_SHOCKWAVE)
    schedule(guid, "horror_shockwave", {20000, 25000}, function() onHorrorShockwave(creature, guid) end)
end

local function onHorrorEnrage(creature, guid)
    -- C++ horror UpdateAI pre-loop gate :1323 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom; distinct from the 1s cast-fail recheck, still unmodelable).
    if creature:IsCasting() then
        schedule(guid, "horror_enrage", 500, function() onHorrorEnrage(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_HORROR_ENRAGE)
    schedule(guid, "horror_enrage", {20000, 25000}, function() onHorrorEnrage(creature, guid) end)
end

local function shamblingHorrorJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "horror_shockwave", {20000, 25000}, function() onHorrorShockwave(creature, guid) end)
    schedule(guid, "horror_enrage", {11000, 14000}, function() onHorrorEnrage(creature, guid) end)
end

local function shamblingHorrorLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_shambling_horror_icc DamageTaken (:1308-1313): first time,
-- heroic only, when post-damage health drops below 20% of max,
-- latch and DoCastSelf(SPELL_FRENZY 28747, true). The latch is
-- ctor-only — never cleared, so the GUID-keyed table survives
-- leave-combat/reset (algalon threshold+latch precedent).
local horrorFrenzied = {}

local function shamblingHorrorDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if horrorFrenzied[guid] then
        return
    end
    if not creature:IsHeroic() then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 20 / 100) then
        horrorFrenzied[guid] = true
        creature:CastSpell(creature, SPELL_HORROR_FRENZY, true)
    end
end

-- C++ npc_raging_spirit: Reset — DoCastSelf(SPELL_PLAGUE_AVOIDANCE,
-- true) + DoCastSelf(SPELL_RAGING_SPIRIT_VISUAL, true) + summoner
-- CastSpell(me, SPELL_RAGING_SPIRIT_VISUAL_CLONE 69198, true) (:1371-1372,
-- via creature:GetSummonerUnit, bridged 2026-11-05) +
-- DoCastSelf(SPELL_BOSS_HITTIN_YA 73878, true) one-shots,
-- EVENT_SOUL_SHRIEK {12s,15s} (:1367-1374/:1420-1421); the
-- Reset REACT_PASSIVE (:1365) -> EVENT_SET_AGRESSIVE 2s (:1424) leg is
-- PORTED (2026-10-22) via the file's CreateLuaEvent pump.
-- C++ DoAction(ACTION_DISABLE_RAGING) (:1377-1386, _events.Reset() on
-- Frostmourne-phase entry, issued by the boss AI :1066) has no bridge —
-- the cross-AI DoAction rides the unbridged phase machine, so the Soul
-- Shriek timer keeps firing where C++ goes silent (gothik counterpoint).
local function onSoulShriek(creature, guid)
    -- C++ raging UpdateAI pre-loop gate :1412 (no in-loop return): 500ms IsCasting defer, no re-arm consumed (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "soul_shriek", 500, function() onSoulShriek(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_SOUL_SHRIEK)
    schedule(guid, "soul_shriek", {12000, 15000}, function() onSoulShriek(creature, guid) end)
end

-- C++ raging UpdateAI pre-loop gate :1412 (no in-loop return):
-- 500ms IsCasting defer, flip not consumed (mal_ganis idiom).
local function onRagingAggressive(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "raging_aggressive", 500, function() onRagingAggressive(creature, guid) end)
        return
    end
    creature:SetReactState(REACT_AGGRESSIVE)
end

local function ragingSpiritJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetReactState(REACT_PASSIVE)
    schedule(guid, "raging_aggressive", 2000, function() onRagingAggressive(creature, guid) end)
    creature:CastSpell(creature, SPELL_PLAGUE_AVOIDANCE, true)
    creature:CastSpell(creature, SPELL_RAGING_SPIRIT_VISUAL, true)
    -- C++ Reset (:1371-1372): the summoner (the Lich King) casts the
    -- visual clone on the spirit — creature:GetSummonerUnit, bridged
    -- 2026-11-05; C++ order kept (between the two self-casts).
    local summoner = creature:GetSummonerUnit()
    if summoner then
        summoner:CastSpell(creature, SPELL_RAGING_SPIRIT_VISUAL_CLONE, true)
    end
    creature:CastSpell(creature, SPELL_BOSS_HITTIN_YA, true)
    schedule(guid, "soul_shriek", {12000, 15000}, function() onSoulShriek(creature, guid) end)
end

local function ragingSpiritLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_spirit_warden: Reset — 1-arg DoCast(SPELL_DARK_HUNGER)
-- one-shot, EVENT_SOUL_RIP {12s,15s} (:1833-1835/:1855-1856);
-- fires in Reset in C++, keyed to event 1 (engage) here per the
-- Reset re-key convention (target resolves via AISpellInfo).
local function onSoulRip(creature, guid)
    creature:CastSpell(nil, SPELL_SOUL_RIP)
    schedule(guid, "soul_rip", {23000, 27000}, function() onSoulRip(creature, guid) end)
end

local function spiritWardenJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_DARK_HUNGER)
    schedule(guid, "soul_rip", {12000, 15000}, function() onSoulRip(creature, guid) end)
end

local function spiritWardenLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_spirit_warden::JustDied (boss_the_lich_king.cpp:1838-1841):
-- instance GetGuidData(DATA_TERENAS_MENETHIL) -> terenas DoAction
-- (ACTION_TELEPORT_BACK). The instance GUID fetch rides the
-- FindNearestCreature sweep (500yd, thorim preAddDied precedent);
-- only one terenas of each entry exists in the Frostmourne room.
local function spiritWardenDied(event, creature, killer)
    local terenas = creature:FindNearestCreature(ENTRY_TERENAS, 500)
    if not terenas then
        terenas = creature:FindNearestCreature(ENTRY_TERENAS_HEROIC, 500)
    end
    if terenas then
        terenas:DoAction(ACTION_TELEPORT_BACK)
    end
end

RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 1, shamblingHorrorJustEngagedWith)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 2, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 9, shamblingHorrorDamageTaken)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 23, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 1, ragingSpiritJustEngagedWith)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 2, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 23, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 1, spiritWardenJustEngagedWith)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 2, spiritWardenLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 4, spiritWardenDied)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 23, spiritWardenLeaveCombat)

-- C++ npc_terenas_menethil::DamageTaken (:1733-1752): lethal damage
-- is clamped to health-1 (sara second-return precedent); then, only
-- when not heroic and without SPELL_TERENAS_LOSES_INSIDE, SetFlag
-- NOT_SELECTABLE + DoCast(TERENAS_LOSES_INSIDE, self-cast per the
-- warden 1-arg precedent) + nearest spirit warden (20yd) casts
-- DESTROY_SOUL (non-triggered) + 2s despawn + terenas 2s despawn.
-- The 1s EVENT_TELEPORT_BACK leg stays out (boss DoAction cross-AI,
-- no bridge); the EVENT_DESTROY_SOUL 1min leg rides the unported
-- DoAction/summon machine. Terenas Talks stay DoAction/scheduler
-- gated as documented in the header.
local function terenasDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local newDamage = damage
    if health > 0 and damage >= health then
        newDamage = health - 1
        if not creature:HasAura(SPELL_TERENAS_LOSES_INSIDE) and not creature:IsHeroic() then
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:CastSpell(creature, SPELL_TERENAS_LOSES_INSIDE)
            local guid = creature:GetGUID()
            local warden = creature:FindNearestCreature(ENTRY_SPIRIT_WARDEN, 20.0)
            if warden then
                warden:CastSpell(nil, SPELL_DESTROY_SOUL)
                schedule(guid, "warden_despawn", 2000, function() warden:Despawn() end)
            end
            schedule(guid, "terenas_despawn", 2000, function() creature:Despawn() end)
        end
    end
    return false, newDamage
end

RegisterCreatureEvent(ENTRY_TERENAS, 9, terenasDamageTaken)
RegisterCreatureEvent(ENTRY_TERENAS_HEROIC, 9, terenasDamageTaken)

-- C++ npc_terenas_menethil::DoAction ACTION_TELEPORT_BACK
-- (boss_the_lich_king.cpp:1714-1717): CastSpell(nullptr,
-- SPELL_RESTORE_SOUL, TRIGGERED_NONE) + DespawnOrUnsummon(3s).
-- PORTED 2026-11-05 (tick 215), census row 159, via the
-- spiritWardenDied JustDied relay above. (The terenas UpdateAI
-- EVENT_TELEPORT_BACK leg that sends the Lich King DoAction stays
-- out — LK-side instance-GUID + phase machine.)
local function terenasDoAction(event, creature, action)
    if action ~= ACTION_TELEPORT_BACK then
        return
    end
    creature:CastSpell(nil, SPELL_RESTORE_SOUL)
    creature:DespawnOrUnsummon(3000)
end

RegisterCreatureEvent(ENTRY_TERENAS, 38, terenasDoAction)
RegisterCreatureEvent(ENTRY_TERENAS_HEROIC, 38, terenasDoAction)

-- C++ npc_strangulate_vehicle (boss_the_lich_king.cpp:1590-1680,
-- entry 36598) — PORTED 2026-11-05 (ev22 shape-2): IsSummonedBy +
-- the UpdateAI timer legs + DoAction ACTION_TELEPORT_BACK. ev22
-- fires on the summoned creature with (event, creature, summoner)
-- from the summonTempCreature fire site
-- (engine/world/lua_creature_events.go:191-199 + :6799-6820:
-- creature summons reach ev22, not just player pets — the
-- "pet-only" header claim below is retracted; the hodir /
-- marrowgar bone-spike ev22 precedent). The live trigger is the
-- LK's Harvest Soul cast (68980, DBC summon effect), itself
-- unported (phase-THREE timer); the vehicle arms follow the
-- raging-spirit / spirit-warden precedent of porting the summoned
-- creature's arms ahead of the LK-side cast.
-- C++ npc_strangulate_vehicle UpdateAI EVENT_DESPAWN_SELF
-- (:1670-1674): the lichKing->AI()->SummonedCreatureDespawn(me) leg
-- is BossAI summons-list bookkeeping with no Lua equivalent (the
-- vehicle is not in the LK's SummonedCreatureDies list either,
-- :795-808); DespawnOrUnsummon(1ms).
local function onVehicleDespawnSelf(creature, guid)
    creature:DespawnOrUnsummon(1)
end

-- C++ npc_strangulate_vehicle UpdateAI EVENT_MOVE_TO_LICH_KING
-- (:1660-1669): if farther than 10yd from the Lich King, MovePoint(0)
-- to a near position 7.5-12.5yd from the LK on the LK->vehicle angle.
-- GetNearPosition / GetAbsoluteAngle have no bridge — computed here
-- with math.atan2 + GetX/GetY at the LK's Z (the SetFacingToObject
-- comment's atan2 twin, lua_creature_events.go:4341); the LK is
-- resolved via FindNearestCreature (the terenasDamageTaken precedent)
-- instead of instance GetGuidData. GetExactDist rides the object.go
-- generic fallback (center-distance model; the C++ combat-reach
-- subtraction is unmodeled, per the object.go comment).
local function onVehicleMoveToLichKing(creature, guid)
    local lichKing = creature:FindNearestCreature(ENTRY_THE_LICH_KING, 100.0)
    if not lichKing then
        return
    end
    if creature:GetExactDistance(lichKing) > 10.0 then
        local angle = math.atan2(creature:GetY() - lichKing:GetY(), creature:GetX() - lichKing:GetX())
        local dist = math.random() * 5.0 + 7.5
        creature:MoveTo(0, lichKing:GetX() + dist * math.cos(angle), lichKing:GetY() + dist * math.sin(angle), lichKing:GetZ())
    end
end

-- C++ npc_strangulate_vehicle UpdateAI EVENT_TELEPORT (:1639-1658):
-- MotionMaster Clear + MoveIdle, then the summoner (the harvested
-- player, via GetSummonerUnit) casts HARVEST_SOUL_VISUAL on itself
-- (nil target), exits the vehicle, and casts HARVEST_SOUL_TELEPORT
-- (non-heroic) or HARVEST_SOULS_TELEPORT + RemoveAurasDueToSpell
-- (HARVEST_SOUL) (heroic); EVENT_DESPAWN_SELF in 65s. Deltas:
-- MoveIdle is the StopMoving twin (no separate Clear bridge);
-- RemoveAurasDueToSpell has no bridge — player:RemoveAura(spell) is
-- the per-spell twin, with the heroic HARVEST_SOUL id picked by
-- Is25ManRaid (74296 10H / 74297 25H, RAID_MODE :193); the player
-- CastSpell path drops the triggered flag (lua_player.go:2752);
-- C++ ExitVehicle(summoner) passes the summoner as the exit position,
-- the Go bridge takes no args.
local function onVehicleTeleport(creature, guid)
    creature:MoveIdle()
    local summoner = creature:GetSummonerUnit()
    if summoner then
        summoner:CastSpell(nil, SPELL_HARVEST_SOUL_VISUAL, true)
        summoner:ExitVehicle()
        if not creature:IsHeroic() then
            summoner:CastSpell(summoner, SPELL_HARVEST_SOUL_TELEPORT, true)
        else
            summoner:CastSpell(summoner, SPELL_HARVEST_SOULS_TELEPORT, true)
            if creature:Is25ManRaid() then
                summoner:RemoveAura(SPELL_HARVEST_SOUL_25H)
            else
                summoner:RemoveAura(SPELL_HARVEST_SOUL_10H)
            end
        end
    end
    schedule(guid, "vehicle_despawn_self", 65000, function() onVehicleDespawnSelf(creature, guid) end)
end

-- C++ npc_strangulate_vehicle::IsSummonedBy (:1594-1609): face the
-- summoner + DoCast(summoner, SPELL_HARVEST_SOUL_VEHICLE 68984) +
-- schedule EVENT_MOVE_TO_LICH_KING 2s + EVENT_TELEPORT 6s. The
-- lichKing->AI()->JustSummoned(me) leg is the LK-side bookkeeping arm
-- (boss_the_lich_king.cpp:784-786, summons.Summon only) — no Lua
-- equivalent, and lichKingJustSummoned has no vehicle branch.
local function strangulateVehicleSummoned(_, creature, summoner)
    if not summoner then
        return
    end
    creature:SetFacingToObject(summoner)
    creature:CastSpell(summoner, SPELL_HARVEST_SOUL_VEHICLE)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "vehicle_move_to_lich_king", 2000, function() onVehicleMoveToLichKing(creature, guid) end)
    schedule(guid, "vehicle_teleport", 6000, function() onVehicleTeleport(creature, guid) end)
end

local function strangulateVehicleLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_strangulate_vehicle::DoAction ACTION_TELEPORT_BACK
-- (:1611-1630): the vehicle casts HARVEST_SOUL_TELEPORT_BACK on the
-- summoner + the summoner removes HARVEST_SOULS_TELEPORT, then the LK
-- SummonedCreatureDespawn bookkeeping leg (no Lua equivalent) +
-- DespawnOrUnsummon. The sender is the LK-side ACTION_TELEPORT_BACK
-- relay (LK DoAction :642-646, unported) via EnterEvadeMode (:599-600)
-- or terenas EVENT_TELEPORT_BACK (:1807-1810, unported), so this
-- fires once that relay lands.
local function strangulateVehicleDoAction(event, creature, action)
    if action ~= ACTION_TELEPORT_BACK then
        return
    end
    local summoner = creature:GetSummonerUnit()
    if summoner then
        creature:CastSpell(summoner, SPELL_HARVEST_SOUL_TELEPORT_BACK)
        summoner:RemoveAura(SPELL_HARVEST_SOULS_TELEPORT)
    end
    creature:DespawnOrUnsummon()
end

RegisterCreatureEvent(ENTRY_STRANGULATE_VEHICLE, 2, strangulateVehicleLeaveCombat)
RegisterCreatureEvent(ENTRY_STRANGULATE_VEHICLE, 22, strangulateVehicleSummoned)
RegisterCreatureEvent(ENTRY_STRANGULATE_VEHICLE, 23, strangulateVehicleLeaveCombat)
RegisterCreatureEvent(ENTRY_STRANGULATE_VEHICLE, 38, strangulateVehicleDoAction)

-- C++ spell_the_lich_king_infest::OnPeriodic
-- (boss_the_lich_king.cpp:2003-2009): EFFECT_0 SPELL_AURA_
-- PERIODIC_DAMAGE. When the owner is above 90% health the handler
-- PreventDefaultActions the tick and removes the aura (return true
-- is the PreventDefaultAction twin; aura:Remove() is the Remove
-- twin). The OnUpdate leg (amount * 1.15 from the 2nd tick) stays
-- out — no OnEffectUpdatePeriodic bridge. The boss-side Infest
-- cast (70541, PHASE_ONE timer) stays out — phase machine.
local SPELL_INFEST = 70541

local function infestTick(event, aura, target)
    if target:HealthAbovePct(90) then
        aura:Remove()
        return true
    end
end

RegisterAuraEvent(SPELL_INFEST, 1, infestTick)

-- C++ spell_the_lich_king_soul_reaper::OnPeriodic
-- (boss_the_lich_king.cpp:2321-2326): EFFECT_1 SPELL_AURA_
-- PERIODIC_DAMAGE. No PreventDefaultAction, so the Go default
-- damage tick still fires. The aura target casts SPELL_SOUL_
-- REAPER_BUFF 69410 on the caster (the Lich King), triggered.
-- The boss-side Soul Reaper cast (69409, PHASE_TWO_THREE timer)
-- stays out — phase machine.
local SPELL_SOUL_REAPER = 69409
local SPELL_SOUL_REAPER_BUFF = 69410

local function soulReaperTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        target:CastSpell(caster, SPELL_SOUL_REAPER_BUFF, true)
    end
end

RegisterAuraEvent(SPELL_SOUL_REAPER, 1, soulReaperTick)

-- C++ spell_the_lich_king_necrotic_plague_jump_aura::OnApply
-- (boss_the_lich_king.cpp:2078-2083): EFFECT_0 PERIODIC_DAMAGE,
-- REAL_OR_REAPPLY. The caster (the Lich King) records the plague
-- stack in its AI via SetData(DATA_PLAGUE_STACK = 70337, stack);
-- C++ keeps the running max (_necroticPlagueStack = max(value, old)),
-- reset only on encounter Reset, and the value is read by the "Been
-- Waiting a Long Time" achievement check (stack >= 30,
-- boss_the_lich_king.cpp:2831). The Go twin is the creature ScriptData
-- store (creature:SetData/GetData, lua_creature_events.go), so the
-- max() is done here in Lua: plain SetData would overwrite with a
-- fresh stack-1 jump and lose the peak. DELTA: the Go apply hook is
-- fresh-only (C++ REAL_OR_REAPPLY also fires on re-apply); each jump
-- casts 70338 on a new target, so the per-jump fresh apply is the
-- live path — same pre-existing delta as Renew (tick 199).
-- PORTED (tick 203, OnRemove below): OnRemove (:2085-2100) and the
-- OnDispel/AfterDispel pair (:2102-2125). No AddSpellMod bridge exists
-- (CastCustomSpell takes bp0/bp1/bp2 only, no aura-stack mod), so the
-- stack mod rides aura:SetStackAmount on the fresh aura right after
-- the re-cast (stack + 1 on expire/death, same stack on dispel).
-- DELTAS: (1) Go has no original-caster slot, so the jump aura's
-- caster is the host, not the Lich King — the host re-casts on itself
-- instead of the C++ area-entry selection of the nearest new host
-- (jump SpellScript SelectTarget unbridged), and the LK-side
-- DATA_PLAGUE_STACK peak / Plague Siphon legs only fire while the
-- expiring aura's caster is still the Lich King (first jump); later
-- jumps grow the live stack but the recorded peak freezes.
-- (2) The C++ AfterDispel BP1 marker (AURA_REMOVE_BY_ENEMY_SPELL)
-- is unconsumed — the jump SpellScript never reads it — so it is
-- dropped. (3) The C++ _lastAmount reapply-vs-dispel discriminator
-- is unneeded: Go merges never fire the remove hook, so mode
-- ENEMY_SPELL here is a genuine dispel.
local SPELL_NECROTIC_PLAGUE_JUMP = 70338 -- boss_the_lich_king.cpp:91
local DATA_PLAGUE_STACK = 70337 -- boss_the_lich_king.cpp:369

local function necroticPlagueJumpApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    -- Jump re-casts attribute the fresh aura to the host (Go has no
    -- original-caster slot), so only a creature caster (the Lich King)
    -- carries the ScriptData store; a player host has no GetData.
    if caster:GetObjectType() ~= "Creature" then
        return
    end
    local stack = aura:GetStackAmount()
    if stack > caster:GetData(DATA_PLAGUE_STACK) then
        caster:SetData(DATA_PLAGUE_STACK, stack)
    end
end

RegisterAuraEvent(SPELL_NECROTIC_PLAGUE_JUMP, 2, necroticPlagueJumpApply)

local SPELL_PLAGUE_SIPHON = 74074 -- boss_the_lich_king.cpp:92
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67
local AURA_REMOVE_BY_DEATH = 5 -- SpellAuraDefines.h:68
local AURA_REMOVE_BY_ENEMY_SPELL = 3 -- SpellAuraDefines.h:66

-- C++ spell_the_lich_king_necrotic_plague_jump_aura::OnRemove
-- (boss_the_lich_king.cpp:2085-2100): on expire or death the target
-- re-casts the jump on itself with stack + 1 and the caster (Lich
-- King) self-casts Plague Siphon, triggered. The AfterDispel leg
-- (:2107-2125) re-casts with the same stack on a genuine dispel.
local function necroticPlagueJumpRemove(event, aura, target)
    local mode = aura:GetRemoveMode()
    local stack = aura:GetStackAmount()
    local caster = aura:GetCaster()
    local newStack = stack
    if mode == AURA_REMOVE_BY_EXPIRE or mode == AURA_REMOVE_BY_DEATH then
        newStack = stack + 1
    elseif mode ~= AURA_REMOVE_BY_ENEMY_SPELL then
        return
    end
    target:CastSpell(target, SPELL_NECROTIC_PLAGUE_JUMP, true)
    local fresh = target:GetAura(SPELL_NECROTIC_PLAGUE_JUMP)
    if fresh ~= nil then
        fresh:SetStackAmount(newStack)
    end
    if caster ~= nil and caster:GetObjectType() == "Creature" then
        if newStack > caster:GetData(DATA_PLAGUE_STACK) then
            caster:SetData(DATA_PLAGUE_STACK, newStack)
        end
        caster:CastSpell(caster, SPELL_PLAGUE_SIPHON, true)
    end
end

RegisterAuraEvent(SPELL_NECROTIC_PLAGUE_JUMP, 3, necroticPlagueJumpRemove)
