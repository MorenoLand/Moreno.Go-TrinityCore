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
-- DOCUMENTED-ONLY (in this header): registered entries are 36597 (boss),
-- 38995 (tirion), 38584 (broken frostmourne), 36823/39217 (terenas),
-- 37698/36701/36824/36609/37799/39137/38757/36598 (adds).
-- boss_the_lich_king DoAction — PORTED 2026-11-06 (lichKingDoAction,
-- ev38): ACTION_START_ENCOUNTER writes IN_PROGRESS + Talk(SAY_LK_INTRO_1 0)
-- + SetStandState(STAND) + EVENT_INTRO_MOVE_1 4s (intro machine
-- PORTED 2026-11-08); ACTION_START_ATTACK schedules EVENT_START_ATTACK
-- 5s (PORTED 2026-11-08; sender is the unported Quake SpellScript);
-- the START_ENCOUNTER sender (Tirion MovementInform POINT_TIRION_INTRO)
-- is PORTED 2026-11-08 — the receiver is live end-to-end now.
-- Talk(SAY_LK_FROSTMOURNE_ESCAPE 8, non-heroic) rides
-- ACTION_TELEPORT_BACK (sender terenas EVENT_TELEPORT_BACK PORTED
-- 2026-11-08). SetZoneMusic has no bridge; the strangulate-vehicle
-- summons relay has no summon-list bridge (the vehicle ev38 receiver
-- IS ported).
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
-- Frostmourne-room player casts it on the LK) — the OnRemove legs
-- are PORTED 2026-11-08 (tick 425, below, dormant: the Harvest Soul
-- cast itself is unported), so the trigger path now exists in dormant
-- form and event 14 would fire once the cast ports (the rotface /
-- infinite-corruptor trigger-path precedent). The old "event 15 never
-- fires" rationale conflated the two events: event 15
-- (OnSpellHitTarget) fires on the CASTER's bindings and was never the
-- right hook for this boss-side arm. Corrected 2026-10-17.
-- The second SpellHit arm (:818-820, REMORSELESS_WINTER_1/2 68981/
-- 72259 —> SetZoneOverrideLight + LIGHT_SNOW weather + shadow-trap
-- despawn) is PORTED 2026-11-08 (lichKingSpellHit, ev14): the trigger
-- path is live via MovementInform POINT_CENTER_1/2 (ported tick 441),
-- and both map legs are bridged (map:SetZoneOverrideLight +
-- map:SetZoneWeather — the JustDied FOG legs in this file).
-- boss_the_lich_king MovementInform — Talk(SAY_LK_REMORSELESS_WINTER
-- 4) on POINT_CENTER_1 / POINT_CENTER_2 PORTED 2026-11-08 via ev28
-- (lua_creature_events.go:4699/:5725); tirion->AI()->
-- Talk(SAY_TIRION_OUTRO_2) on POINT_LK_OUTRO_2 PORTED 2026-11-08
-- (FindNearestCreature sweep + :Talk on the found creature — the
-- stale "no cross-AI-Talk bridge" claim is retired; the mandokir
-- jindo:Talk precedent).
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
-- PHASE_FROSTMOURNE) had no Lua model, not because timers lack a
-- bridge (the gothik counterpoint: no invented phase-agnostic
-- timers) — the model LANDED 2026-11-08 (lkPhase, ev9 DamageTaken
-- transitions, ev28 MovementInform, EVENT_QUAKE phase sets); the
-- intro machine + FROSTMOURNE phase set PORTED 2026-11-08
-- (onIntroMove1..3 / onHarvestSouls); the JustDied
-- MoveFall leg still does,
-- while the SPELL_PLAY_MOVIE (73159), SetDisableGravity(false), and
-- zone-light + zone-weather JustDied legs ARE ported (ev4
-- lichKingJustDied).
-- MISSING-CAST TRIAGE (2026-10-16; C++ line cites from
-- boss_the_lich_king.cpp) — all 12 BLOCKED, none ported:
-- 68981 SPELL_REMORSELESS_WINTER_1 (:849) / 72259
-- SPELL_REMORSELESS_WINTER_2 (:863) — DoCastSelf in MovementInform
-- case POINT_CENTER_1 / POINT_CENTER_2 (Remorseless Winter channel
-- opener, transition-phase entry) PORTED 2026-11-08 via ev28
-- (lichKingMovementInform) — the "no MovementInform bridge" claim
-- was stale (ev28 landed, lua_creature_events.go:4699/:5725).
-- 73213 SPELL_EMOTE_SHOUT_NO_SHEATH (:929) — DoCastSelf non-triggered
-- in case EVENT_EMOTE_CAST_SHOUT, scheduled +7s/+18s in PHASE_INTRO;
-- PORTED 2026-11-08 (onEmoteCastShout, PHASE_INTRO-gated on the
-- lkPhase model — no invented phase-agnostic timers).
-- 70372 SPELL_SUMMON_SHAMBLING_HORROR (:947, PHASE_ONE, 1min re-arm),
-- 70358 SPELL_SUMMON_DRUDGE_GHOULS (:952, PHASE_ONE, 30s re-arm),
-- 70541 SPELL_INFEST (:956, PHASE_ONE seeded, re-arms PHASE_ONE or
-- PHASE_TWO by current phase), 69409 SPELL_SOUL_REAPER (:973,
-- DoCastVictim, PHASE_TWO_THREE, 33-35s re-arm) — phase-gated timer
-- arms PORTED 2026-11-08 (onSummonHorror / onSummonGhoul / onInfest /
-- onSoulReaper, phase-gated on the lkPhase model — no invented
-- phase-agnostic timers).
-- 71797 SPELL_LIGHTS_BLESSING (:1263, case EVENT_OUTRO_BLESS) — cast
-- by npc_tirion_fordring_tft on itself (outro); PORTED 2026-11-08
-- (tirionFns.outroBless, PHASE_OUTRO-gated on the tirionPhase model).
-- 74352 SPELL_WINGS_OF_THE_DAMNED (:1455, val'kyr Reset, non-triggered)
-- PORTED 2026-11-08 via ev22 IsSummonedBy (valkyrIsSummonedBy below);
-- 73501 SPELL_VALKYR_TRANSFORM (:1522, val'kyr MovementInform
-- POINT_SIPHON), 73488 SPELL_LIFE_SIPHON (:1562, val'kyr
-- EVENT_LIFE_SIPHON DoCastVictim) — cast by npc_valkyr_shadowguard,
-- not the boss; the grab/life-siphon machine PORTED 2026-11-08
-- (valkyrMovementInform ev28 + valkyrDamageTaken ev9 below).
-- 69382 SPELL_LIGHTS_FAVOR (:1704, terenas DoAction
-- ACTION_FROSTMOURNE_INTRO) — cast by npc_terenas_menethil on itself;
-- PORTED 2026-11-08 (terenasDoAction below).
-- npc_tirion_fordring_tft (NPC_HIGHLORD_TIRION_FORDRING_LK 38995) —
-- REGISTERED 2026-11-08 (tirionFns table: Reset ev23, JustReachedHome
-- ev24, MovementInform ev28, DoAction ev38, SpellHit ev14, gossip
-- select; the intro/outro scheduler arms ride the tirionPhase model +
-- CreateLuaEvent). Talk(SAY_TIRION_INTRO_1 0) rides DoAction
-- ACTION_CONTINUE_INTRO (sender LK MovementInform POINT_LK_INTRO_3,
-- PORTED 2026-11-08); Talk(SAY_TIRION_INTRO_2 1) (EVENT_INTRO_TALK_1)
-- and Talk(SAY_TIRION_OUTRO_1 2) (EVENT_OUTRO_TALK_1) are
-- scheduler-driven on the PHASE_INTRO / PHASE_OUTRO machines; the
-- OnGossipSelect intro kick gates on listID == 0 (the Go hook does not
-- forward menuId — the barrettGossipSelect precedent). Stay-outs:
-- SpellHit BROKEN_FROSTMOURNE_KNOCK LoadEquipment(1) (no bridge);
-- UpdateAI DoMeleeAttackIfReady (no victim model for a non-combat NPC).
-- npc_terenas_menethil (NPC_TERENAS_MENETHIL_FROSTMOURNE 36823,
-- NPC_TERENAS_MENETHIL_FROSTMOURNE_H 39217) — DamageTaken PORTED
-- (terenasDamageTaken, ev9, this tick); DoAction ACTION_TELEPORT_BACK
-- PORTED 2026-11-05 (tick 215) via terenasDoAction (ev38) + the
-- spiritWardenDied JustDied relay (ev4); DoAction
-- ACTION_FROSTMOURNE_INTRO + EVENT_FROSTMOURNE_TALK_1..3 +
-- EVENT_DESTROY_SOUL + EVENT_TELEPORT_BACK PORTED 2026-11-08
-- (terenasDoAction / onTerenasFrostmourneTalk1..3 /
-- onTerenasDestroySoul / onTerenasTeleportBack). The heroic sender
-- (LK EVENT_FROSTMOURNE_HEROIC, boss_the_lich_king.cpp:1073) is
-- ported; the normal sender (instance EVENT_TELEPORT_TO_FROSTMOURNE,
-- instance_icecrown_citadel.cpp:1463) is unported.
-- Talk(SAY_TERENAS_OUTRO_1..2 0..1) (EVENT_OUTRO_TERENAS_TALK_1..2,
-- IsSummonedBy hook) PORTED 2026-11-08 (terenasIsSummonedBy, ev22 +
-- onTerenasOutroTalk1/2): the outro summon chain is live —
-- LK POINT_LK_OUTRO_2 -> frostmourne DoAction ACTION_SUMMON_TERENAS
-- -> 6s DoCastAOE SPELL_SUMMON_TERENAS 72420 (DBC summon effect,
-- dormant until Spell.dbc ships). The Frostmourne-room summon fires
-- the same ev22, but the FROSTMOURNE_INTRO DoAction cancelTimers the
-- guid right after — the C++ _events.Reset() twin — so no outro Talk
-- is fabricated there (the old gothik-counterpoint worry is retired).
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
-- npc_valkyr_shadowguard IS registered (Reset via ev22 IsSummonedBy,
-- PORTED 2026-11-08 — the val'kyr never engages, so ev1 would never
-- fire); npc_spirit_bomb — zero Talk lines — NOT registered (the
-- bronjahm npc_corrupted_soul_fragment precedent); npc_broken_frostmourne
-- IS registered (IsSummonedBy ev22 + DoAction ev38, PORTED 2026-11-08
-- — knock-back + summon-terenas arms; the JUMP SpellScript sender of
-- ACTION_BREAK_FROSTMOURNE stays unported); the valkyr
-- grab/life-siphon machine PORTED 2026-11-08 (see below) and the bomb
-- UpdateAI machine rides unbridged legs. npc_strangulate_vehicle IS
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
-- (2, MovementInform) + SAY_TIRION_OUTRO_2 (1, cross-AI MovementInform,
-- PORTED 2026-11-08) + SAY_LK_INTRO_2 1 (1, scheduler) + SAY_LK_INTRO_3 2 (1,
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
-- ACTION_DISABLE_RAGING REACT_PASSIVE leg (:1377-1386) PORTED
-- 2026-11-08 (ragingSpiritDoAction, ev38) and its sender
-- (EVENT_HARVEST_SOULS summon sweep) PORTED 2026-11-08
-- (onHarvestSouls below).
-- STAYS OUT (no new bridges built):
-- LK Reset REACT_PASSIVE (:529-532) + SetImmuneToPC (:530) PORTED
-- 2026-11-08 (lichKingReset — SetImmuneToPC rides SetImmuneToAll,
-- the mandokir twin); the EVENT_FINISH_INTRO REACT_AGGRESSIVE
-- (:941-946) leg PORTED 2026-11-08 (onFinishIntro below).
-- LK DamageTaken REACT_PASSIVE legs (:690/:700/:709, 70%/40%/10%
-- phase transitions) PORTED 2026-11-08 (lichKingDamageTaken, ev9 +
-- ev28 MovementInform — the "no MovementInform bridge" claim was
-- stale).
-- LK EVENT_HARVEST_SOULS REACT_PASSIVE (:1044) + EVENT_START_ATTACK
-- REACT_AGGRESSIVE (:1028-1031) PORTED 2026-11-08 (onHarvestSouls /
-- onStartAttack, PHASE_FROSTMOURNE/PHASE_THREE-gated on the lkPhase
-- model).
-- JustSummoned REACT_PASSIVE + LichKingStartMovementEvent 5s ->
-- REACT_AGGRESSIVE + AttackStart (:739-741/:447-466, shambling horror /
-- drudge ghoul): cross-creature timed BasicEvent machine;
-- AttackStart/SelectTarget have no bridge. VileSpiritActivateEvent
-- REACT_AGGRESSIVE (:472-484) + JustSummoned vile-spirit leg (:774-781)
-- PORTED 2026-11-08 (lichKingJustSummoned + onVileSpiritActivate):
-- flight-speed leg stays out (SetSpeedRate rejects MOVE_FLIGHT, no
-- script flight-rate field); the Frostmourne-phase activate gate is
-- PORTED 2026-11-08 (lkPhase read in lichKingJustSummoned);
-- EVENT_VILE_SPIRITS 79.5s/:1034 timer + DoCastAOE 70498 PORTED
-- 2026-11-08 (onVileSpirits, PHASE_THREE-gated on the lkPhase model).
-- JustSummoned legs :750/:761 (ice-sphere :750 PORTED in
-- lichKingJustSummoned — SelectTarget approximated by the alive
-- same-map/instance random-player pool, MoveFollow PORTED in
-- lichKingJustSummoned; defile :761 PORTED (lichKingJustSummoned);
-- spirit_bomb stays unregistered as documented; strangulate
-- PORTED below (ev22 + UpdateAI timers + ev38).
-- SpellScript legs: ice_burst_target_search CheckTargetCount (:2212-2221)
-- PORTED 2026-11-08 (event 7, this file below) and restore_soul
-- REACT_PASSIVE on wicked spirits (:2632-2641) PORTED tick 446
-- (event 4); the LK-side ACTION_TELEPORT_BACK arm rides ev38 dispatch
-- (bridged) with the restore_soul sender ported tick 446; cf.
-- terenas-side ACTION_TELEPORT_BACK, ported tick 215 via ev38.

-- CAST-STATE GATES PORTED (2026-11-04): boss :900/:1149
-- (TRANSITION/OUTRO/FROSTMOURNE-exempt in C++; phase-aware in Lua
-- since the 2026-11-08 phase model) gates the berserk one-shot; horror :1323 (pre-loop
-- only) gates shockwave/enrage; raging :1412 (pre-loop only) gates
-- soul-shriek + the set-aggressive flip; warden :1843 UpdateAI is a
-- plain loop (zero CASTING hits) so soul-rip stays ungated;
-- valkyr :1544/:1576 gates cover the grab machine (timer + target
-- search ported tick 447; MovementInform drop legs still out);
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
local ACTION_START_ATTACK = 2 -- boss_the_lich_king.cpp:337

local SPELL_BERSERK2 = 47008
local SPELL_HORROR_SHOCKWAVE = 72149
local SPELL_HORROR_ENRAGE = 72143
local SPELL_HORROR_FRENZY = 28747
local SPELL_SOUL_SHRIEK = 69242
local SPELL_PLAGUE_AVOIDANCE = 72846
local SPELL_RAGING_SPIRIT_VISUAL = 69197
local SPELL_RAGING_SPIRIT_VISUAL_CLONE = 69198 -- boss_the_lich_king.cpp:109
local SPELL_BOSS_HITTIN_YA = 73878
local SPELL_VILE_SPIRIT_MOVE_SEARCH = 70501 -- boss_the_lich_king.cpp:133
local SPELL_VILE_SPIRIT_DAMAGE_SEARCH = 70502 -- boss_the_lich_king.cpp:134
local SPELL_WINGS_OF_THE_DAMNED = 74352 -- boss_the_lich_king.cpp:122
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
-- Phase-model + timer-arm constants (2026-11-08, tick 441), grouped in
-- ONE table: the file sits at Lua's 200-local main-chunk limit, so
-- these 48 ids ride a single local (the algalon table-const precedent).
local LK = {
    PHASE_INTRO = 1, PHASE_ONE = 2, PHASE_TWO = 3, PHASE_THREE = 4,
    PHASE_TRANSITION = 5, PHASE_FROSTMOURNE = 6, PHASE_OUTRO = 7,
    POINT_CENTER_1 = 1, -- boss_the_lich_king.cpp:316
    POINT_CENTER_2 = 2, -- boss_the_lich_king.cpp:317
    POINT_LK_INTRO_1 = 4, -- :319
    POINT_LK_INTRO_2 = 5, -- :320
    POINT_LK_INTRO_3 = 6, -- :321
    INTRO_1_X = 432.0851, INTRO_1_Y = -2123.673, INTRO_1_Z = 864.6582, -- :302-306
    INTRO_2_X = 457.8351, INTRO_2_Y = -2123.423, INTRO_2_Z = 841.1582,
    INTRO_3_X = 465.0730, INTRO_3_Y = -2123.470, INTRO_3_Z = 840.8569,
    TIRION_SPAWN_X = 505.2118, TIRION_SPAWN_Y = -2124.353, TIRION_SPAWN_Z = 840.9403, TIRION_SPAWN_O = 3.141593, -- :299
    TERENAS_HEROIC_X = 495.7080, TERENAS_HEROIC_Y = -2523.760, TERENAS_HEROIC_Z = 1050.0, TERENAS_HEROIC_O = 0.0, -- :311
    SAY_LK_INTRO_2 = 1, SAY_LK_INTRO_3 = 2, -- :37-38
    SPELL_ICE_LOCK = 71614, -- :83
    SPELL_EMOTE_SIT_NO_SHEATH = 73220, -- :80
    SPELL_HARVEST_SOULS = 73654, -- :137
    SPELL_SUMMON_SPIRIT_BOMB_1 = 73581, -- :158
    SPELL_SUMMON_SPIRIT_BOMB_2 = 74299, -- :159
    SPELL_TRIGGER_VILE_SPIRIT_HEROIC = 73582, -- :146
    NPC_WORLD_TRIGGER_INFINITE_AOI = 36171, -- icecrown_citadel.h:329
    EMOTE_ONESHOT_POINT_NO_SHEATHE = 397, -- SharedDefines.h:2035
    SHEATH_STATE_MELEE = 1,
    POINT_LK_OUTRO_1 = 9, -- boss_the_lich_king.cpp:324
    CENTER_X = 503.6282, CENTER_Y = -2124.655, CENTER_Z = 840.8569, -- :298
    SPELL_REMORSELESS_WINTER_1 = 68981, -- :84
    SPELL_REMORSELESS_WINTER_2 = 72259, -- :85
    SPELL_QUAKE = 72262, -- :111
    SPELL_SUMMON_SHAMBLING_HORROR = 70372, -- :86
    SPELL_SUMMON_DRUDGE_GHOULS = 70358, -- :88
    SPELL_NECROTIC_PLAGUE = 70337, -- :90
    SPELL_SHADOW_TRAP = 73539, -- :93
    SPELL_PAIN_AND_SUFFERING = 72133, -- :101
    SPELL_SUMMON_ICE_SPHERE = 69104, -- :102
    SPELL_RAGING_SPIRIT = 69200, -- :107
    SPELL_DEFILE = 72762, -- :114
    SPELL_SUMMON_VALKYR = 69037, -- :117
    SPELL_INFEST = 70541, -- :89
    SPELL_SOUL_REAPER = 69409, -- :120
    SPELL_VILE_SPIRITS = 70498, -- :132
    SPELL_SHADOW_TRAP_AURA = 73525, -- :94
    SPELL_SOUL_RIP_DAMAGE = 69398, -- :156
    SPELL_DARK_HUNGER_HEAL = 69384, -- :153
    SPELL_HARVEST_SOUL = 68980, -- :136
    SPELL_FURY_OF_FROSTMOURNE = 72350, -- :163
    SPELL_FURY_OF_FROSTMOURNE_NO_REZ = 72351, -- :164
    SPELL_EMOTE_SHOUT_NO_SHEATH = 73213, -- :82
    SPELL_EMOTE_QUESTION_NO_SHEATH = 73330, -- :165
    SPELL_RAISE_DEAD = 71769, -- :166
    SPELL_HARVEST_SOUL_VALKYR = 68985, -- :119
    SAY_LK_REMORSELESS_WINTER = 4, -- :40
    SAY_LK_QUAKE = 5, -- :41
    SAY_LK_SUMMON_VALKYR = 6, -- :43
    SAY_LK_HARVEST_SOUL = 7, -- :44
    EMOTE_DEFILE_WARNING = 12, -- :49
    EMOTE_NECROTIC_PLAGUE_WARNING = 13, -- :50
    SAY_LK_OUTRO_1 = 14, SAY_LK_OUTRO_2 = 15, SAY_LK_OUTRO_3 = 16,
    SAY_LK_OUTRO_4 = 17, SAY_LK_OUTRO_5 = 18,
    SAY_LK_OUTRO_6 = 19, SAY_LK_OUTRO_7 = 20, SAY_LK_OUTRO_8 = 21, -- :52-54
    EMOTE_ONESHOT_TALK_NO_SHEATHE = 396, -- SharedDefines.h:2034
    -- Tirion TFT + outro-chain consts (2026-11-08, tick 443), same one-table
    -- local: the file sits near Lua's 200-local main-chunk limit.
    POINT_TIRION_INTRO = 3, -- :318
    POINT_TIRION_CHARGE = 7, -- :322
    POINT_TIRION_OUTRO_1 = 10, -- :325
    POINT_LK_OUTRO_2 = 12, -- :327
    TIRION_INTRO_X = 489.2970, TIRION_INTRO_Y = -2124.840, TIRION_INTRO_Z = 840.8569, -- :300
    TIRION_CHARGE_X = 482.9019, TIRION_CHARGE_Y = -2124.479, TIRION_CHARGE_Z = 840.8570, -- :301
    OUTRO_POS_1_X = 493.6286, OUTRO_POS_1_Y = -2124.569, OUTRO_POS_1_Z = 840.8569, -- :308
    OUTRO_FLYING_X = 508.9897, OUTRO_FLYING_Y = -2124.561, OUTRO_FLYING_Z = 845.3565, -- :309
    SAY_TIRION_INTRO_1 = 0, SAY_TIRION_INTRO_2 = 1, -- :62-63
    SAY_TIRION_OUTRO_1 = 2, SAY_TIRION_OUTRO_2 = 3, -- :64-65
    SAY_TERENAS_OUTRO_1 = 0, SAY_TERENAS_OUTRO_2 = 1, -- :67-68
    SPELL_LIGHTS_BLESSING = 71797, -- :167
    SPELL_JUMP = 71809, -- :168
    SPELL_JUMP_2 = 72431, -- :169
    MOVIE_FALL_OF_THE_LICH_KING = 16, -- :364
    NPC_WICKED_SPIRIT = 39190, -- icecrown_citadel.h:326
    SPELL_BROKEN_FROSTMOURNE_KNOCK = 72405, -- :175
    SPELL_SUMMON_TERENAS = 72420, -- :177
    SPELL_MASS_RESURRECTION = 72429, -- :178
    SPELL_SUMMON_BROKEN_FROSTMOURNE = 74081, -- :171
    SPELL_SUMMON_BROKEN_FROSTMOURNE_2 = 72406, -- :172
    SPELL_SUMMON_BROKEN_FROSTMOURNE_3 = 73017, -- :173
    SPELL_SOUL_BARRAGE = 72305, -- :176
    EQUIP_ASHBRINGER_GLOWING = 50442, -- :361
    EQUIP_BROKEN_FROSTMOURNE = 50840, -- :362
    EQUIP_UNEQUIP = 0, -- CreatureAI.h:76
    ACTION_CONTINUE_INTRO = 1, ACTION_OUTRO = 3, -- :336/:338
    ACTION_PLAY_MUSIC = 4, ACTION_BREAK_FROSTMOURNE = 5, -- :339-340
    ACTION_SUMMON_TERENAS = 6, ACTION_FINISH_OUTRO = 7, -- :341-342
    EMOTE_STATE_READY2H = 375, -- SharedDefines.h:2013
    EMOTE_ONESHOT_NONE = 0, -- SharedDefines.h:1937
    LIGHT_SNOWSTORM = 2490, -- :349
    WEATHER_STATE_LIGHT_SNOW = 6, -- Weather.h:53
    UNIT_NPC_FLAG_GOSSIP = 0x00000001, -- UnitDefines.h:186
    -- Target-select consts (2026-11-08, tick 447), same one-table
    -- local: the file sits at 199 main-chunk locals.
    SPELL_ICE_BURST_TARGET_SEARCH = 69109, -- :104
    SPELL_ICE_BURST = 69108, -- :106
    SPELL_SHADOW_TRAP_KNOCKBACK = 73529, -- :95
    SPELL_VALKYR_TARGET_SEARCH = 69030, -- :122
    SPELL_CHARGE = 74399, -- :124
    SPELL_SPIRIT_BURST = 70503, -- :135
    SPELL_DEFILE_GROW = 72756, -- :116
    DATA_VILE = 45814622, -- :370
    -- Val'kyr drop-machine + quake consts (2026-11-08, tick 448), same
    -- one-table local: the file sits at 199 main-chunk locals.
    GO_ARTHAS_PLATFORM = 202161, -- icecrown_citadel.h:431
    POINT_DROP_PLAYER = 8, -- :323
    POINT_SIPHON = 14, -- :329
    POINT_CHARGE = 1003, -- :330
    SPELL_EJECT_ALL_PASSENGERS = 68576, -- :128
    SPELL_VALKYR_CARRY = 74445, -- :125
    SPELL_VALKYR_TRANSFORM = 73501, -- :129
    SPELL_LIFE_SIPHON = 73488, -- :126
    SPELL_LIFE_SIPHON_HEAL = 73489, -- :127
}

-- Target-select state (2026-11-08, tick 447): the SpellScript _target
-- picks live on the LK table (no new locals — the file sits at 199
-- main-chunk locals), keyed by caster GUID; vileMoveTarget holds the
-- picked GUID, valkyrTarget holds the picked unit object (EFFECT_1
-- ReplaceTarget re-adds it), valkyrGrabbed is the _grabbedPlayer
-- latch (the AI SetGUID write has no bridge).
LK.vileMoveTarget = {}
LK.valkyrTarget = {}
LK.valkyrGrabbed = {}
LK.valkyrDropPoint = {}
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
local ACTION_DISABLE_RAGING = 9 -- boss_the_lich_king.cpp:344
local ACTION_FROSTMOURNE_INTRO = -36823 -- icecrown_citadel.h:512
local SPELL_LIGHTS_FAVOR = 69382 -- boss_the_lich_king.cpp:149
local SPELL_RESTORE_SOULS = 73650 -- :151
local SAY_TERENAS_INTRO_1 = 0 -- :71
local SAY_TERENAS_INTRO_2 = 1 -- :72
local SAY_TERENAS_INTRO_3 = 2 -- :73
local ENTRY_TIRION_LK = 38995 -- icecrown_citadel.h:315 (NPC_HIGHLORD_TIRION_FORDRING_LK)
-- POINT_MOTION_TYPE was a NIL global in this file until 2026-11-08:
-- lichKingMovementInform compared moveType against it, so the guard
-- always returned and every ev28 arm (CENTER_1/2, INTRO_1..3,
-- OUTRO_1 — ticks 441/442) was dead at runtime. MovementDefines.h:36.
local POINT_MOTION_TYPE = 8
-- Tirion's own EventMap phase (Phases enum, boss_the_lich_king.cpp:287-293,
-- shared with the boss): INTRO 1 / OUTRO 7. The lkPhase-model twin.
local tirionPhase = {}
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

-- C++ EventMap phases (boss_the_lich_king.cpp:287-293): INTRO 1, ONE 2,
-- TWO 3, THREE 4, TRANSITION 5, FROSTMOURNE 6, OUTRO 7. The Lua phase
-- model (PORTED 2026-11-08): lkPhase[guid] is set to PHASE_ONE at
-- engage (:578), to PHASE_TRANSITION by DamageTaken (:687-705), to
-- PHASE_TWO/PHASE_THREE by EVENT_QUAKE/EVENT_QUAKE_2 (:1009/:1016),
-- to PHASE_OUTRO by the 10% DamageTaken leg (:707-728). Phase-gated
-- handlers check lkPhase and drop without re-arming — the C++
-- EventMap::ExecuteEvent phase-mismatch erase (EventMap.cpp:81-82:
-- a due event whose phase bits miss the current phase is erased,
-- not deferred). The FROSTMOURNE phase is set by onHarvestSouls
-- (:1043, PORTED 2026-11-08) and cleared by onStartAttack (:1030);
-- the vile-spirit activate gate reads it.
local lkPhase = {}

-- Forward declaration (the tharon_ja/gal_darah idiom): onVileSpiritActivate
-- is defined with the JustSummoned section below but is referenced by
-- onHarvestSouls above it; plain assignment at the definition site,
-- never a second `local function` (the malown shadowing time-bomb).
local onVileSpiritActivate

-- C++ EventMap::DelayEvents(50s, EVENT_GROUP_VILE_SPIRITS) twin
-- (boss_the_lich_king.cpp:1050): the group holds only
-- EVENT_VILE_SPIRITS, so the delay is a remaining-time shift of the
-- pending "vile_spirits" timer. The due time is recorded at each arm
-- (CENTER_2 seed + re-arm, the thekal delayTimers precedent,
-- wall-clock ms via GetCurrTime); delayVileSpirits below no-ops when
-- the timer is not armed (C++ DelayEvents on an empty group is
-- likewise a no-op).
local vileSpiritsDue = {}

-- Random alive player from the threat list (the file's
-- randomPlayerAnywhere idiom, threat-ordered); skipFirst drops the
-- tank (C++ SelectTarget offset 1), excludeAuras drops carriers of
-- any listed aura. Returns nil when the pool is empty.
local function randomAlivePlayer(creature, skipFirst, excludeAuras)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    local start = 1
    if skipFirst then
        start = 2
    end
    if #candidates < start then
        return nil
    end
    local pool = {}
    for i = start, #candidates do
        local t = candidates[i]
        local excluded = false
        if excludeAuras then
            for _, aura in ipairs(excludeAuras) do
                if t:HasAura(aura) then
                    excluded = true
                    break
                end
            end
        end
        if not excluded then
            pool[#pool + 1] = t
        end
    end
    if #pool == 0 then
        return nil
    end
    return pool[math.random(#pool)]
end

-- C++ EVENT_BERSERK: 15min single-fire — Talk(SAY_LK_BERSERK) +
-- DoCastSelf(SPELL_BERSERK2) non-triggered (:583/:1141-1143), bare
-- timer across all phases (EVENT_GROUP_BERSERK carries no phase).
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :900 (+ in-loop :1149): 500ms
    -- IsCasting defer, cast not consumed (mal_ganis idiom) — exempt
    -- while in TRANSITION/OUTRO/FROSTMOURNE, where C++ keeps
    -- executing events while channeling (phase model landed 2026-11-08).
    local phase = lkPhase[guid]
    if creature:IsCasting() and phase ~= LK.PHASE_TRANSITION and phase ~= LK.PHASE_OUTRO and phase ~= LK.PHASE_FROSTMOURNE then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(SAY_LK_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK2)
end

-- C++ EVENT_SUMMON_SHAMBLING_HORROR (:947, PHASE_ONE): DoCastSelf
-- SPELL_SUMMON_SHAMBLING_HORROR 70372, re-arm 1min. Casting defers
-- 500ms (the :900 gate, mal_ganis idiom); a phase mismatch drops
-- without re-arm (EventMap erase).
local function onSummonHorror(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_ONE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "horror", 500, function() onSummonHorror(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_SUMMON_SHAMBLING_HORROR)
    schedule(guid, "horror", 60000, function() onSummonHorror(creature, guid) end)
end

-- C++ EVENT_SUMMON_DRUDGE_GHOUL (:952, PHASE_ONE): DoCastSelf
-- SPELL_SUMMON_DRUDGE_GHOULS 70358, re-arm 30s.
local function onSummonGhoul(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_ONE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "ghoul", 500, function() onSummonGhoul(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_SUMMON_DRUDGE_GHOULS)
    schedule(guid, "ghoul", 30000, function() onSummonGhoul(creature, guid) end)
end

-- C++ EVENT_INFEST (:956, PHASE_ONE seeded, re-arms PHASE_ONE or
-- PHASE_TWO by current phase): DoCastSelf SPELL_INFEST 70541,
-- re-arm {21s, 24s}. The aura leg (infestTick) is ported below.
local function onInfest(creature, guid)
    local phase = lkPhase[guid]
    if phase ~= LK.PHASE_ONE and phase ~= LK.PHASE_TWO then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "infest", 500, function() onInfest(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_INFEST)
    schedule(guid, "infest", {21000, 24000}, function() onInfest(creature, guid) end)
end

-- C++ NECROTIC_PLAGUE_LK/PLR RAID_MODE ids (:188-189); the player
-- aura is what the target check excludes (the boss-side id never
-- lands on a player candidate).
local NECROTIC_PLAGUE_PLR_IDS = {70338, 73785, 73786, 73787}

-- C++ EVENT_NECROTIC_PLAGUE (:960-963, PHASE_ONE): random target,
-- offset 1 (tank skipped), NecroticPlagueTargetCheck (:373-390) —
-- player, targetable, no PLAGUE_AVOIDANCE 72846, no plague aura —
-- Talk(EMOTE_NECROTIC_PLAGUE_WARNING 13, target) + DoCast
-- SPELL_NECROTIC_PLAGUE 70337, re-arm {30s, 33s} regardless of
-- target found.
local function onNecroticPlague(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_ONE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "necrotic", 500, function() onNecroticPlague(creature, guid) end)
        return
    end
    local exclude = {SPELL_PLAGUE_AVOIDANCE, NECROTIC_PLAGUE_PLR_IDS[1], NECROTIC_PLAGUE_PLR_IDS[2], NECROTIC_PLAGUE_PLR_IDS[3], NECROTIC_PLAGUE_PLR_IDS[4]}
    local target = randomAlivePlayer(creature, true, exclude)
    if target then
        creature:Talk(LK.EMOTE_NECROTIC_PLAGUE_WARNING, target)
        creature:CastSpell(target, LK.SPELL_NECROTIC_PLAGUE)
    end
    schedule(guid, "necrotic", {30000, 33000}, function() onNecroticPlague(creature, guid) end)
end

-- C++ EVENT_SHADOW_TRAP (:967-971, PHASE_ONE, heroic-only seed):
-- random target (SpellTargetSelector, approximated by the alive
-- same-map random-player pool) + DoCast SPELL_SHADOW_TRAP 73539,
-- re-arm 15500ms.
local function onShadowTrap(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_ONE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "shadow_trap", 500, function() onShadowTrap(creature, guid) end)
        return
    end
    local target = randomAlivePlayer(creature, false, nil)
    if target then
        creature:CastSpell(target, LK.SPELL_SHADOW_TRAP)
    end
    schedule(guid, "shadow_trap", 15500, function() onShadowTrap(creature, guid) end)
end

-- C++ EVENT_PAIN_AND_SUFFERING (:990-994, PHASE_TRANSITION): random
-- player + CastSpell SPELL_PAIN_AND_SUFFERING 72133, re-arm
-- {1500ms, 4s}. No IsCasting defer — C++ keeps executing events
-- while channeling Remorseless Winter in TRANSITION (:900 gate).
local function onPainAndSuffering(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_TRANSITION then
        return
    end
    local target = randomAlivePlayer(creature, false, nil)
    if target then
        creature:CastSpell(target, LK.SPELL_PAIN_AND_SUFFERING)
    end
    schedule(guid, "pain_suffering", {1500, 4000}, function() onPainAndSuffering(creature, guid) end)
end

-- C++ EVENT_SUMMON_ICE_SPHERE (:995-998, PHASE_TRANSITION):
-- DoCastAOE SPELL_SUMMON_ICE_SPHERE 69104, re-arm {7500ms, 8500ms}.
-- No IsCasting defer (TRANSITION-exempt, :900).
local function onSummonIceSphere(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_TRANSITION then
        return
    end
    creature:CastSpell(creature, LK.SPELL_SUMMON_ICE_SPHERE)
    schedule(guid, "ice_sphere", {7500, 8500}, function() onSummonIceSphere(creature, guid) end)
end

-- C++ EVENT_SUMMON_RAGING_SPIRIT (:999-1003, PHASE_TRANSITION):
-- random player + CastSpell SPELL_RAGING_SPIRIT 69200, re-arm
-- {22s, 23s}. No IsCasting defer (TRANSITION-exempt, :900).
local function onSummonRagingSpirit(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_TRANSITION then
        return
    end
    local target = randomAlivePlayer(creature, false, nil)
    if target then
        creature:CastSpell(target, LK.SPELL_RAGING_SPIRIT)
    end
    schedule(guid, "raging_spirit", {22000, 23000}, function() onSummonRagingSpirit(creature, guid) end)
end

-- C++ EVENT_SUMMON_RAGING_SPIRIT_2 (:1004-1008, PHASE_TRANSITION,
-- CENTER_2 seed): same cast, then schedules the base
-- EVENT_SUMMON_RAGING_SPIRIT at 18s (not itself).
local function onSummonRagingSpirit2(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_TRANSITION then
        return
    end
    local target = randomAlivePlayer(creature, false, nil)
    if target then
        creature:CastSpell(target, LK.SPELL_RAGING_SPIRIT)
    end
    schedule(guid, "raging_spirit", 18000, function() onSummonRagingSpirit(creature, guid) end)
end

-- C++ EVENT_QUAKE / EVENT_QUAKE_2 (:1009-1022, PHASE_TRANSITION):
-- SetPhase(PHASE_TWO / PHASE_THREE) + DoCastAOE SPELL_QUAKE 72262 +
-- Talk(SAY_LK_QUAKE 5). No IsCasting defer (TRANSITION-exempt,
-- :900); the C++ ClearUnitState(UNIT_STATE_CASTING) leg has no
-- bridge. SetZoneMusic stays out (no bridge).
local function onQuake(creature, guid, nextPhase)
    if lkPhase[guid] ~= LK.PHASE_TRANSITION then
        return
    end
    lkPhase[guid] = nextPhase
    creature:CastSpell(creature, LK.SPELL_QUAKE)
    creature:Talk(LK.SAY_LK_QUAKE)
end

-- C++ EVENT_SUMMON_VALKYR (:1023-1028, PHASE_TWO): Talk(SAY_LK_SUMMON_VALKYR 6)
-- + DoCastAOE(SUMMON_VALKYR 69037, triggered), re-arm {45s, 50s}.
local function onSummonValkyr(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_TWO then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "valkyr", 500, function() onSummonValkyr(creature, guid) end)
        return
    end
    creature:Talk(LK.SAY_LK_SUMMON_VALKYR)
    creature:CastSpell(creature, LK.SPELL_SUMMON_VALKYR, true)
    schedule(guid, "valkyr", {45000, 50000}, function() onSummonValkyr(creature, guid) end)
end

-- C++ EVENT_DEFILE (:976-983, PHASE_TWO_THREE): random player
-- excluding SPELL_HARVEST_SOUL_VALKYR 68985 carriers (val'kyr-held)
-- + Talk(EMOTE_DEFILE_WARNING 12) + DoCast SPELL_DEFILE 72762,
-- re-arm {32s, 35s}.
local function onDefile(creature, guid)
    local phase = lkPhase[guid]
    if phase ~= LK.PHASE_TWO and phase ~= LK.PHASE_THREE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "defile", 500, function() onDefile(creature, guid) end)
        return
    end
    local target = randomAlivePlayer(creature, false, {LK.SPELL_HARVEST_SOUL_VALKYR})
    if target then
        creature:Talk(LK.EMOTE_DEFILE_WARNING)
        creature:CastSpell(target, LK.SPELL_DEFILE)
    end
    schedule(guid, "defile", {32000, 35000}, function() onDefile(creature, guid) end)
end

-- C++ EVENT_SOUL_REAPER (:972-975, PHASE_TWO_THREE): DoCastVictim
-- SPELL_SOUL_REAPER 69409, re-arm {33s, 35s}. The aura leg
-- (soulReaperTick) is ported below.
local function onSoulReaper(creature, guid)
    local phase = lkPhase[guid]
    if phase ~= LK.PHASE_TWO and phase ~= LK.PHASE_THREE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "soul_reaper", 500, function() onSoulReaper(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, LK.SPELL_SOUL_REAPER)
    end
    schedule(guid, "soul_reaper", {33000, 35000}, function() onSoulReaper(creature, guid) end)
end

-- C++ EVENT_VILE_SPIRITS (:1034-1038, PHASE_THREE): DoCastAOE
-- SPELL_VILE_SPIRITS 70498, re-arm {35s, 40s} (EVENT_GROUP_VILE_SPIRITS).
-- SetZoneMusic stays out (no bridge).
local function onVileSpirits(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_THREE then
        vileSpiritsDue[guid] = nil
        return
    end
    if creature:IsCasting() then
        vileSpiritsDue[guid] = GetCurrTime() + 500
        schedule(guid, "vile_spirits", 500, function() onVileSpirits(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_VILE_SPIRITS)
    -- The {35s, 40s} re-arm is rolled here (not inside CreateLuaEvent)
    -- The {35s, 40s} re-arm is rolled here (not inside CreateLuaEvent)
    -- so the due time is known for DelayEvents (vileSpiritsDue above).
    local delay = math.random(35000, 40000)
    vileSpiritsDue[guid] = GetCurrTime() + delay
    schedule(guid, "vile_spirits", delay, function() onVileSpirits(creature, guid) end)
end

-- C++ EVENT_HARVEST_SOUL (:984-989, PHASE_THREE, non-heroic):
-- Talk(SAY_LK_HARVEST_SOUL 7) + random target (SpellTargetSelector,
-- approximated by the alive random-player pool) + DoCast
-- SPELL_HARVEST_SOUL 68980, re-arm 75s. The heroic
-- EVENT_HARVEST_SOULS arm sets PHASE_FROSTMOURNE (:1039-1069,
-- PORTED 2026-11-08, onHarvestSouls below) and is seeded on heroic
-- from the POINT_CENTER_2 MovementInform arm.
local function onHarvestSoul(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_THREE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "harvest_soul", 500, function() onHarvestSoul(creature, guid) end)
        return
    end
    creature:Talk(LK.SAY_LK_HARVEST_SOUL)
    local target = randomAlivePlayer(creature, false, nil)
    if target then
        creature:CastSpell(target, LK.SPELL_HARVEST_SOUL)
    end
    schedule(guid, "harvest_soul", 75000, function() onHarvestSoul(creature, guid) end)
end

-- C++ EventMap::DelayEvents(50s, EVENT_GROUP_VILE_SPIRITS)
-- (boss_the_lich_king.cpp:1050): shift the pending "vile_spirits"
-- timer out by extraMs, preserving its remaining time (the thekal
-- delayTimers precedent). No-ops when the timer is not armed.
local function delayVileSpirits(guid, creature, extraMs)
    local per = timers[guid]
    if not per or not per["vile_spirits"] then
        return
    end
    local due = vileSpiritsDue[guid]
    if not due then
        return
    end
    local remaining = due - GetCurrTime()
    if remaining < 0 then
        remaining = 0
    end
    local delay = remaining + extraMs
    vileSpiritsDue[guid] = GetCurrTime() + delay
    schedule(guid, "vile_spirits", delay, function() onVileSpirits(creature, guid) end)
end

-- C++ EVENT_START_ATTACK (:1029-1032): REACT_AGGRESSIVE + if in
-- PHASE_FROSTMOURNE, SetPhase(PHASE_THREE). No phase bits on the
-- schedule (fires in any phase); the 49s schedule only exists via
-- EVENT_HARVEST_SOULS, the 5s one via DoAction ACTION_START_ATTACK
-- (ported below; its sender is the unported Quake SpellScript).
-- PORTED 2026-11-08.
local function onStartAttack(creature, guid)
    creature:SetReactState(REACT_AGGRESSIVE)
    if lkPhase[guid] == LK.PHASE_FROSTMOURNE then
        lkPhase[guid] = LK.PHASE_THREE
    end
end

-- C++ TriggerWickedSpirit (boss_the_lich_king.cpp:489-516): a
-- BasicEvent on the world-trigger spawner, counter 13: every 3s it
-- casts SPELL_TRIGGER_VILE_SPIRIT_HEROIC 73582 on nil target with
-- SPELLVALUE_MAX_TARGETS 1 (no bridge for the clamp — the
-- mother_shahraz fatal-attraction precedent casts without it and
-- documents the delta), re-arming while the counter lasts (13 casts
-- total). PORTED 2026-11-08.
local function onTriggerWickedSpirit(spawner, guid, remaining)
    spawner:CastSpell(nil, LK.SPELL_TRIGGER_VILE_SPIRIT_HEROIC)
    if remaining > 1 then
        schedule(guid, "wicked_spirit", 3000, function() onTriggerWickedSpirit(spawner, guid, remaining - 1) end)
    end
end

-- C++ EVENT_FROSTMOURNE_HEROIC (:1070-1087): summon Terenas
-- (heroic entry 39217) at TerenasSpawnHeroic, 50s timed despawn +
-- DoAction(ACTION_FROSTMOURNE_INTRO), then the nearest world trigger
-- (36171) in 100yd casts SPELL_SUMMON_SPIRIT_BOMB_1/2 (triggered)
-- and starts the 3s TriggerWickedSpirit chain. The C++ sorts all
-- triggers in range by distance and takes the front — the nearest,
-- which is what FindNearestCreature returns. setActive(true) on
-- Terenas and on the spawner has no bridge. PORTED 2026-11-08.
local function onFrostmourneHeroic(creature, guid)
    local terenas = creature:SummonCreature(ENTRY_TERENAS_HEROIC, LK.TERENAS_HEROIC_X, LK.TERENAS_HEROIC_Y, LK.TERENAS_HEROIC_Z, LK.TERENAS_HEROIC_O, 2, 50000)
    if terenas then
        terenas:DoAction(ACTION_FROSTMOURNE_INTRO)
        local spawner = terenas:FindNearestCreature(LK.NPC_WORLD_TRIGGER_INFINITE_AOI, 100.0)
        if spawner then
            spawner:SetFarVisible(true)
            spawner:CastSpell(spawner, LK.SPELL_SUMMON_SPIRIT_BOMB_1, true)
            spawner:CastSpell(spawner, LK.SPELL_SUMMON_SPIRIT_BOMB_2, true)
            local spawnerGuid = spawner:GetGUID()
            schedule(spawnerGuid, "wicked_spirit", 3000, function() onTriggerWickedSpirit(spawner, spawnerGuid, 13) end)
        end
    end
end

-- C++ EVENT_HARVEST_SOULS (:1039-1069, PHASE_THREE, heroic only):
-- Talk(SAY_LK_HARVEST_SOUL 7) + DoCastAOE(SPELL_HARVEST_SOULS 73654)
-- + re-arm {100s, 110s} + SetPhase(PHASE_FROSTMOURNE) +
-- REACT_PASSIVE + AttackStop + DelayEvents(50s, vile-spirits group)
-- + RescheduleEvent(DEFILE, 50s) + RescheduleEvent(SOUL_REAPER,
-- {57s, 62s}) + EVENT_START_ATTACK 49s + EVENT_FROSTMOURNE_HEROIC
-- 6.5s + the summon sweep (vile spirits: kill events + 56.5s
-- activate re-arm + REACT_PASSIVE + CombatStop(true) +
-- RemoveAurasDueToSpell(70501/70502) + MoveTargetedHome; raging
-- spirits: DoAction(ACTION_DISABLE_RAGING)). The IsCasting defer
-- applies (phase is still PHASE_THREE at fire time; the :900 gate
-- exempts FROSTMOURNE only after the set). SetZoneMusic stays out
-- (no bridge). The sweep rides GetCreaturesInRange (500yd, the
-- frozen-throne platform; C++ iterates the LK summons list, which
-- has no Lua twin). PORTED 2026-11-08.
local function onHarvestSouls(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_THREE then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "harvest_souls", 500, function() onHarvestSouls(creature, guid) end)
        return
    end
    creature:Talk(LK.SAY_LK_HARVEST_SOUL)
    creature:CastSpell(creature, LK.SPELL_HARVEST_SOULS)
    schedule(guid, "harvest_souls", {100000, 110000}, function() onHarvestSouls(creature, guid) end)
    lkPhase[guid] = LK.PHASE_FROSTMOURNE
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    delayVileSpirits(guid, creature, 50000)
    schedule(guid, "defile", 50000, function() onDefile(creature, guid) end)
    schedule(guid, "soul_reaper", {57000, 62000}, function() onSoulReaper(creature, guid) end)
    schedule(guid, "start_attack", 49000, function() onStartAttack(creature, guid) end)
    schedule(guid, "frostmourne_heroic", 6500, function() onFrostmourneHeroic(creature, guid) end)
    local spirits = creature:GetCreaturesInRange(500, ENTRY_VILE_SPIRIT)
    if spirits then
        for _, spirit in ipairs(spirits) do
            local spiritGuid = spirit:GetGUID()
            cancelTimers(spiritGuid)
            schedule(spiritGuid, "vile_activate", 56500, function() onVileSpiritActivate(spirit) end)
            spirit:SetReactState(REACT_PASSIVE)
            spirit:CombatStop(true)
            spirit:RemoveAura(SPELL_VILE_SPIRIT_MOVE_SEARCH)
            spirit:RemoveAura(SPELL_VILE_SPIRIT_DAMAGE_SEARCH)
            spirit:MoveHome()
        end
    end
    local ragings = creature:GetCreaturesInRange(500, ENTRY_RAGING_SPIRIT)
    if ragings then
        for _, raging in ipairs(ragings) do
            raging:DoAction(ACTION_DISABLE_RAGING)
        end
    end
end

-- C++ EVENT_OUTRO_TALK_1 (:1088-1091, PHASE_OUTRO): Talk(SAY_LK_OUTRO_1 14)
-- + DoCastAOE(SPELL_FURY_OF_FROSTMOURNE_NO_REZ 72351, triggered).
local function onOutroTalk1(creature, guid)
    creature:Talk(LK.SAY_LK_OUTRO_1)
    creature:CastSpell(creature, LK.SPELL_FURY_OF_FROSTMOURNE_NO_REZ, true)
end

-- C++ EVENT_OUTRO_EMOTE_TALK (:1096-1098, PHASE_OUTRO):
-- HandleEmoteCommand(EMOTE_ONESHOT_TALK_NO_SHEATHE 396).
local function onOutroEmoteTalk(creature, guid)
    creature:PerformEmote(LK.EMOTE_ONESHOT_TALK_NO_SHEATHE)
end

-- C++ EVENT_OUTRO_TALK_2 (:1092-1095, PHASE_OUTRO): Talk(SAY_LK_OUTRO_2 15)
-- + DoCastAOE(SPELL_EMOTE_QUESTION_NO_SHEATH 73330).
local function onOutroTalk2(creature, guid)
    creature:Talk(LK.SAY_LK_OUTRO_2)
    creature:CastSpell(creature, LK.SPELL_EMOTE_QUESTION_NO_SHEATH)
end

-- C++ EVENT_OUTRO_TALK_3 (:1099-1103, PHASE_OUTRO): Talk(SAY_LK_OUTRO_3 16).
-- The SetFacingToObject(tirion) leg is PORTED 2026-11-08 (FindNearestCreature
-- sweep twin; the stale "instance GetGuidData has no Lua creature fetch"
-- claim is retired — the sweep is the spiritWardenDied precedent).
local function onOutroTalk3(creature, guid)
    local tirion = creature:FindNearestCreature(ENTRY_TIRION_LK, 500)
    if tirion then
        creature:SetFacingToObject(tirion)
    end
    creature:Talk(LK.SAY_LK_OUTRO_3)
end

-- C++ EVENT_EMOTE_CAST_SHOUT (:928, PHASE_OUTRO; scheduled :724): DoCastSelf
-- SPELL_EMOTE_SHOUT_NO_SHEATH 73213.
local function onOutroShout(creature, guid)
    creature:CastSpell(creature, LK.SPELL_EMOTE_SHOUT_NO_SHEATH)
end

-- C++ EVENT_OUTRO_MOVE_CENTER (:1104-1106, PHASE_OUTRO): MovePoint
-- POINT_LK_OUTRO_1 to CenterPosition; arrival continues the chain
-- via MovementInform (onOutroTalk4 / onOutroRaiseDead / onOutroTalk5).
local function onOutroMoveCenter(creature, guid)
    creature:MoveTo(LK.POINT_LK_OUTRO_1, LK.CENTER_X, LK.CENTER_Y, LK.CENTER_Z)
end

-- C++ EVENT_OUTRO_TALK_4 (:1107-1110, PHASE_OUTRO): SetFacingTo
-- (0.01745329) + Talk(SAY_LK_OUTRO_4 17).
local function onOutroTalk4(creature, guid)
    creature:SetFacingTo(0.01745329)
    creature:Talk(LK.SAY_LK_OUTRO_4)
end

-- C++ EVENT_OUTRO_RAISE_DEAD (:1111-1115, PHASE_OUTRO): DoCastAOE
-- SPELL_RAISE_DEAD 71769 + ClearUnitState (no bridge) + SetZoneMusic
-- (no bridge).
local function onOutroRaiseDead(creature, guid)
    creature:CastSpell(creature, LK.SPELL_RAISE_DEAD)
end

-- C++ EVENT_OUTRO_TALK_5 (:1116-1120, PHASE_OUTRO): Talk(SAY_LK_OUTRO_5 18).
-- The tirion->AI()->DoAction(ACTION_OUTRO) leg is PORTED 2026-11-08
-- (tirion 38995 registered this tick; the FindNearestCreature sweep
-- is the spiritWardenDied precedent).
local function onOutroTalk5(creature, guid)
    creature:Talk(LK.SAY_LK_OUTRO_5)
    local tirion = creature:FindNearestCreature(ENTRY_TIRION_LK, 500)
    if tirion then
        tirion:DoAction(LK.ACTION_OUTRO)
    end
end

-- C++ EVENT_OUTRO_TALK_6 (:1121-1127, PHASE_OUTRO, scheduled by
-- ACTION_BREAK_FROSTMOURNE 2500ms): Talk(SAY_LK_OUTRO_6 19) +
-- tirion SetFacingToObject(me) (sweep twin) + CastSpell(nil,
-- SPELL_SUMMON_BROKEN_FROSTMOURNE_3 73017, triggered) +
-- SetEquipmentSlots(false, EQUIP_UNEQUIP) (all three slots 0).
-- PORTED 2026-11-08.
local function onOutroTalk6(creature, guid)
    creature:Talk(LK.SAY_LK_OUTRO_6)
    local tirion = creature:FindNearestCreature(ENTRY_TIRION_LK, 500)
    if tirion then
        tirion:SetFacingToObject(creature)
    end
    creature:CastSpell(nil, LK.SPELL_SUMMON_BROKEN_FROSTMOURNE_3, true)
    creature:SetEquipmentSlots(0, 0, 0)
end

-- C++ EVENT_OUTRO_TALK_7 (:1135-1137, PHASE_OUTRO, scheduled by
-- ACTION_FINISH_OUTRO 7s): Talk(SAY_LK_OUTRO_7 20). PORTED 2026-11-08.
local function onOutroTalk7(creature, guid)
    creature:Talk(LK.SAY_LK_OUTRO_7)
end

-- C++ EVENT_OUTRO_TALK_8 (:1138-1140, PHASE_OUTRO, scheduled by
-- ACTION_FINISH_OUTRO 17s): Talk(SAY_LK_OUTRO_8 21). PORTED 2026-11-08.
local function onOutroTalk8(creature, guid)
    creature:Talk(LK.SAY_LK_OUTRO_8)
end

-- C++ EVENT_OUTRO_SOUL_BARRAGE (:1128-1134, PHASE_OUTRO, scheduled
-- 5s by the frostmourne-trigger JustSummoned): CastSpell(nil,
-- SPELL_SOUL_BARRAGE 72305, triggered) + SendSound SOUND_PAIN (no
-- bridge, stays out) + SetDisableGravity(true) + MovePoint
-- POINT_LK_OUTRO_2 to OutroFlying; arrival continues the chain via
-- MovementInform (tirion OUTRO_2 + frostmourne SUMMON_TERENAS).
-- PORTED 2026-11-08.
local function onOutroSoulBarrage(creature, guid)
    creature:CastSpell(nil, LK.SPELL_SOUL_BARRAGE, true)
    creature:SetDisableGravity(true)
    creature:MoveTo(LK.POINT_LK_OUTRO_2, LK.OUTRO_FLYING_X, LK.OUTRO_FLYING_Y, LK.OUTRO_FLYING_Z)
end

-- C++ boss_the_lich_king::DoAction ACTION_START_ENCOUNTER (:615-622):
-- SetBossState IN_PROGRESS + Talk(SAY_LK_INTRO_1) + SetZoneMusic
-- (no bridge) + SetStandState(STAND) + EVENT_INTRO_MOVE_1 4s (the
-- intro-movement machine, PORTED 2026-11-08 below). The sender is
-- Tirion's intro chain (unregistered), so the receiver is dormant
-- until that chain lands — the krick ACTION_OUTRO receiver precedent.
-- ACTION_START_ATTACK (:623-625): EVENT_START_ATTACK 5s (the
-- onStartAttack arm, PORTED 2026-11-08; its sender is the unported
-- Quake SpellScript, so this arm is dormant too).
-- ACTION_TELEPORT_BACK (:654-661): Talk(SAY_LK_FROSTMOURNE_ESCAPE 8,
-- non-heroic) + strangulate-vehicle summons relay (no summon-list
-- bridge — the vehicle ev38 receiver IS ported); the sender is the
-- terenas EVENT_TELEPORT_BACK (PORTED 2026-11-08, onTerenasTeleportBack
-- below) / restore_soul SpellScript chain (unported).
-- ACTION_BREAK_FROSTMOURNE (:630-636): CastSpell(nil,
-- SPELL_SUMMON_BROKEN_FROSTMOURNE 74081 + _2 72406, triggered) +
-- SetEquipmentSlots(false, EQUIP_BROKEN_FROSTMOURNE 50840) +
-- EVENT_OUTRO_TALK_6 2500ms (PHASE_OUTRO). PORTED 2026-11-08; the
-- sender is the unported spell_the_lich_king_jump SpellScript (:2755),
-- so the receiver is dormant — the START_ATTACK precedent.
-- ACTION_FINISH_OUTRO (:642-645): EVENT_OUTRO_TALK_7 7s +
-- EVENT_OUTRO_TALK_8 17s (PHASE_OUTRO). PORTED 2026-11-08; the sender
-- is the terenas EVENT_OUTRO_TERENAS_TALK_2 (PORTED 2026-11-08,
-- onTerenasOutroTalk2 below).
-- ACTION_PLAY_MUSIC (:626-628) is SetZoneMusic only (no bridge) — no
-- Lua arm; the sender (Tirion EVENT_OUTRO_REMOVE_ICE) still sends it.
-- Forward declaration (the tharon_ja/gal_darah idiom): onIntroMove1
-- is defined with the intro handlers below but is referenced by the
-- START_ENCOUNTER arm above it; plain assignment at the definition
-- site, never a second `local function` (the malown shadowing
-- time-bomb).
local onIntroMove1

local function lichKingDoAction(event, creature, action)
    if action == ACTION_START_ENCOUNTER then
        local guid = creature:GetGUID()
        creature:SetBossState(DATA_THE_LICH_KING, BOSS_STATE_IN_PROGRESS)
        creature:Talk(SAY_LK_INTRO_1)
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        schedule(guid, "intro_move_1", 4000, function() onIntroMove1(creature, guid) end)
    elseif action == ACTION_START_ATTACK then
        local guid = creature:GetGUID()
        schedule(guid, "start_attack", 5000, function() onStartAttack(creature, guid) end)
    elseif action == ACTION_TELEPORT_BACK then
        if not creature:IsHeroic() then
            creature:Talk(SAY_LK_FROSTMOURNE_ESCAPE)
        end
    elseif action == LK.ACTION_BREAK_FROSTMOURNE then
        local guid = creature:GetGUID()
        creature:CastSpell(nil, LK.SPELL_SUMMON_BROKEN_FROSTMOURNE, true)
        creature:CastSpell(nil, LK.SPELL_SUMMON_BROKEN_FROSTMOURNE_2, true)
        creature:SetEquipmentSlots(LK.EQUIP_BROKEN_FROSTMOURNE, 0xFFFFFFFF, 0xFFFFFFFF)
        schedule(guid, "outro_talk_6", 2500, function() onOutroTalk6(creature, guid) end)
    elseif action == LK.ACTION_FINISH_OUTRO then
        local guid = creature:GetGUID()
        schedule(guid, "outro_talk_7", 7000, function() onOutroTalk7(creature, guid) end)
        schedule(guid, "outro_talk_8", 17000, function() onOutroTalk8(creature, guid) end)
    end
end

-- C++ boss_the_lich_king::JustEngagedWith (:570-588): SetPhase
-- (PHASE_ONE) + EVENT_SUMMON_SHAMBLING_HORROR 20s / EVENT_SUMMON_DRUDGE_GHOUL
-- 10s / EVENT_INFEST 5s / EVENT_NECROTIC_PLAGUE {30s, 33s} (all
-- PHASE_ONE) + EVENT_BERSERK 15min + EVENT_SHADOW_TRAP 15500ms
-- (heroic only, PHASE_ONE). PORTED 2026-11-08 (phase model). No C++
-- Talk on engage, and JustEngagedWith writes NO boss state —
-- IN_PROGRESS rides DoAction ACTION_START_ENCOUNTER (ported above),
-- so no state leg is added here (the fabricated-leg trap). The
-- CheckRequiredBosses evade + LIGHT_S_HAMMER_TELEPORT legs stay out
-- (no bridge).
local function lichKingJustEngagedWith(event, creature, target)
    creature:SetCombatPulseDelay(5) -- boss_the_lich_king.cpp:575 (JustEngagedWith arm; the setActive leg still has no bridge)
    creature:DoZoneInCombat() -- boss_the_lich_king.cpp:576 (JustEngagedWith arm; bridge live engine/world/lua_creature_events.go, 2026-10-23)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    lkPhase[guid] = LK.PHASE_ONE
    schedule(guid, "horror", 20000, function() onSummonHorror(creature, guid) end)
    schedule(guid, "ghoul", 10000, function() onSummonGhoul(creature, guid) end)
    schedule(guid, "infest", 5000, function() onInfest(creature, guid) end)
    schedule(guid, "necrotic", {30000, 33000}, function() onNecroticPlague(creature, guid) end)
    schedule(guid, "berserk", 900000, function() onBerserk(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "shadow_trap", 15500, function() onShadowTrap(creature, guid) end)
    end
end

-- C++ EVENT_FINISH_INTRO (:939-946, PHASE_INTRO): SetWalk(false) +
-- SetImmuneToPC(false) + SetReactState(REACT_AGGRESSIVE) +
-- SetPhase(PHASE_ONE) + DoZoneInCombat(). The Go DoZoneInCombat twin
-- seeds threat but does not fire ev1 (lua_creature_events.go:1996),
-- so the engage seeding runs here directly — the C++ FINISH_INTRO
-- -> DoZoneInCombat -> JustEngagedWith chain; a later first attack
-- re-fires ev1 and re-seeds (the file's existing ev1 cancel+reseed
-- behavior). SetImmuneToPC rides SetImmuneToAll (the mandokir twin;
-- sets 0x100|0x200 vs C++ 0x100 only — no NPC attacks the Lich King
-- during the intro). PORTED 2026-11-08.
local function onFinishIntro(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:SetWalk(false)
    creature:SetImmuneToAll(false)
    creature:SetReactState(REACT_AGGRESSIVE)
    lkPhase[guid] = LK.PHASE_ONE
    lichKingJustEngagedWith(1, creature, nil)
end

-- C++ EVENT_EMOTE_CAST_SHOUT (:928-930, PHASE_INTRO):
-- DoCastSelf(SPELL_EMOTE_SHOUT_NO_SHEATH 73213). 500ms IsCasting
-- defer (the :900 gate; INTRO is not exempt). PORTED 2026-11-08.
local function onEmoteCastShout(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "intro_shout_defer", 500, function() onEmoteCastShout(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_EMOTE_SHOUT_NO_SHEATH)
end

-- C++ EVENT_INTRO_EMOTE_1 (:931-933, PHASE_INTRO):
-- HandleEmoteCommand(EMOTE_ONESHOT_POINT_NO_SHEATHE 397).
-- PORTED 2026-11-08.
local function onIntroEmote1(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:PerformEmote(LK.EMOTE_ONESHOT_POINT_NO_SHEATHE)
end

-- C++ EVENT_INTRO_CAST_FREEZE (:934-938, PHASE_INTRO):
-- Talk(SAY_LK_INTRO_3 2) + DoCastAOE(SPELL_ICE_LOCK 71614) +
-- EVENT_FINISH_INTRO 1s. 500ms IsCasting defer (the :900 gate).
-- PORTED 2026-11-08.
local function onIntroCastFreeze(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "intro_freeze_defer", 500, function() onIntroCastFreeze(creature, guid) end)
        return
    end
    creature:Talk(LK.SAY_LK_INTRO_3)
    creature:CastSpell(creature, LK.SPELL_ICE_LOCK)
    schedule(guid, "finish_intro", 1000, function() onFinishIntro(creature, guid) end)
end

-- C++ EVENT_INTRO_TALK_1 (:919-927, PHASE_INTRO):
-- Talk(SAY_LK_INTRO_2 1) + HandleEmoteCommand(
-- EMOTE_ONESHOT_TALK_NO_SHEATHE) + EVENT_EMOTE_CAST_SHOUT 7s +
-- EVENT_INTRO_EMOTE_1 13s + EVENT_EMOTE_CAST_SHOUT 18s +
-- EVENT_INTRO_CAST_FREEZE 31s. The two shouts share one handler
-- under distinct keys (schedule keys must be unique). PORTED 2026-11-08.
local function onIntroTalk1(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:Talk(LK.SAY_LK_INTRO_2)
    creature:PerformEmote(LK.EMOTE_ONESHOT_TALK_NO_SHEATHE)
    schedule(guid, "intro_shout_1", 7000, function() onEmoteCastShout(creature, guid) end)
    schedule(guid, "intro_emote_1", 13000, function() onIntroEmote1(creature, guid) end)
    schedule(guid, "intro_shout_2", 18000, function() onEmoteCastShout(creature, guid) end)
    schedule(guid, "intro_cast_freeze", 31000, function() onIntroCastFreeze(creature, guid) end)
end

-- C++ EVENT_INTRO_MOVE_2 (:913-915, PHASE_INTRO):
-- MovePoint(POINT_LK_INTRO_2, LichKingIntro[1]). PORTED 2026-11-08.
local function onIntroMove2(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:MoveTo(LK.POINT_LK_INTRO_2, LK.INTRO_2_X, LK.INTRO_2_Y, LK.INTRO_2_Z)
end

-- C++ EVENT_INTRO_MOVE_3 (:916-918, PHASE_INTRO):
-- MovePoint(POINT_LK_INTRO_3, LichKingIntro[2]). PORTED 2026-11-08.
local function onIntroMove3(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:MoveTo(LK.POINT_LK_INTRO_3, LK.INTRO_3_X, LK.INTRO_3_Y, LK.INTRO_3_Z)
end

-- C++ EVENT_INTRO_MOVE_1 (:907-912, PHASE_INTRO): SetSheath(MELEE)
-- + RemoveAurasDueToSpell(SPELL_EMOTE_SIT_NO_SHEATH 73220) +
-- SetWalk(true) + MovePoint(POINT_LK_INTRO_1, LichKingIntro[0]).
-- The phase gate is the C++ PHASE_INTRO event bit (the phase model
-- landed 2026-11-08); no IsCasting defer (no cast in the arm).
-- PORTED 2026-11-08.
onIntroMove1 = function(creature, guid)
    if lkPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:SetSheath(LK.SHEATH_STATE_MELEE)
    creature:RemoveAura(LK.SPELL_EMOTE_SIT_NO_SHEATH)
    creature:SetWalk(true)
    creature:MoveTo(LK.POINT_LK_INTRO_1, LK.INTRO_1_X, LK.INTRO_1_Y, LK.INTRO_1_Z)
end

-- C++ _Reset() timer machinery — cancel/re-arm on combat events
-- (the maiden convention). C++ boss_the_lich_king::EnterEvadeMode
-- (:594): DoCastAOE(SPELL_KILL_FROSTMOURNE_PLAYERS 75127) non-triggered,
-- ungated — modeled as self-cast (file's DoCastAOE idiom, line 104).
-- The tirion-despawn / strangulate-vehicle teleport-back / summons
-- DespawnAll / _DespawnAtEvade legs have no bridge — documented.
local function lichKingLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    lkPhase[creature:GetGUID()] = nil
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
    creature:SetImmuneToAll(true) -- C++ Reset :530 SetImmuneToPC(true) (mandokir twin; 0x100|0x200 vs C++ 0x100 only)
    creature:SetReactState(REACT_PASSIVE) -- C++ Reset :531
    lkPhase[creature:GetGUID()] = LK.PHASE_INTRO -- C++ Reset :533 SetPhase(PHASE_INTRO)
    -- C++ Reset :543: clear any light override (0, 5000ms).
    creature:GetMap():SetZoneOverrideLight(AREA_ICECROWN_CITADEL, LIGHT_DEFAULT, 0, 5000)
    -- C++ Reset :547-548: summon Tirion (38995) at TirionSpawn when
    -- the instance holds none — FindNearestCreature proxy (the
    -- terenasDamageTaken precedent); TEMPSUMMON_MANUAL_DESPAWN.
    if not creature:FindNearestCreature(ENTRY_TIRION_LK, 100.0) then
        creature:SummonCreature(ENTRY_TIRION_LK, LK.TIRION_SPAWN_X, LK.TIRION_SPAWN_Y, LK.TIRION_SPAWN_Z, LK.TIRION_SPAWN_O, 7, 0)
    end
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
    lkPhase[creature:GetGUID()] = nil
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
    if victim:GetObjectType() == "Player" and not creature:IsInEvadeMode() and lkPhase[creature:GetGUID()] ~= LK.PHASE_OUTRO then
        creature:Talk(SAY_LK_KILL)
    end
end

-- C++ boss_the_lich_king::SpellHit (:818-820): on REMORSELESS_WINTER_1
-- 68981 / REMORSELESS_WINTER_2 72259 — SetZoneOverrideLight
-- (LIGHT_SNOWSTORM 2490, 5000ms) + SetZoneWeather (LIGHT_SNOW 6,
-- 0.5) + summons.DespawnEntry(NPC_SHADOW_TRAP 39137). PORTED
-- 2026-11-08 (ev14, the attumenSpellHit precedent): the trigger path
-- is live — the casts ride MovementInform POINT_CENTER_1/2 (ported
-- tick 441, self-cast, so ev14 fires on the boss) — and both map legs
-- are bridged (the JustDied FOG legs in this file). The shadow-trap
-- despawn rides the 500yd GetCreaturesInRange sweep (the onHarvestSouls
-- summon-list twin; C++ iterates the LK summons list, which has no Lua
-- twin). The HARVESTED_SOUL arm (:813-816) stays out: its 72679 cast
-- targets via originalCaster + DBC implicit targets, so it cannot reach
-- the boss in Go (header note, harvestSoulRemove above).
local function lichKingSpellHit(event, creature, caster, spellId)
    if spellId ~= LK.SPELL_REMORSELESS_WINTER_1 and spellId ~= LK.SPELL_REMORSELESS_WINTER_2 then
        return
    end
    creature:GetMap():SetZoneOverrideLight(AREA_ICECROWN_CITADEL, LIGHT_DEFAULT, LK.LIGHT_SNOWSTORM, 5000)
    creature:GetMap():SetZoneWeather(AREA_ICECROWN_CITADEL, LK.WEATHER_STATE_LIGHT_SNOW, 0.5)
    local traps = creature:GetCreaturesInRange(500, ENTRY_SHADOW_TRAP)
    if traps then
        for _, trap in ipairs(traps) do
            trap:Despawn()
        end
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

-- C++ VileSpiritActivateEvent (boss_the_lich_king.cpp:472-484):
-- REACT_AGGRESSIVE + self-cast SPELL_VILE_SPIRIT_MOVE_SEARCH 70501
-- (triggered) + SPELL_VILE_SPIRIT_DAMAGE_SEARCH 70502 (triggered,
-- nullptr target). The casts are dormant until Spell.dbc ships, and
-- the searcher spells' script effects (random-target AttackStart +
-- 100k threat; area target-count DATA_VILE write + Spirit Burst +
-- 3s despawn) ride the unbridged SpellScript — the neck_deep
-- achievement writer stays out (its 70502 area radius is a DBC fact).
-- The 56.5s Harvest-Souls re-arm (:1058-1069) is PORTED 2026-11-08
-- (onHarvestSouls below); it reuses the "vile_activate" key via
-- schedule(), replacing the pending activate.
onVileSpiritActivate = function(spirit)
    spirit:SetReactState(REACT_AGGRESSIVE)
    spirit:CastSpell(spirit, SPELL_VILE_SPIRIT_MOVE_SEARCH, true)
    spirit:CastSpell(nil, SPELL_VILE_SPIRIT_DAMAGE_SEARCH, true)
end

-- No C++ Reset/LeaveCombat arms exist for the vile spirit (it has no
-- creature script; all behavior is LK-side JustSummoned + BasicEvents),
-- so only JustDied cancels the pending activate — mirroring the
-- m_Events teardown when the summon despawns/dies.
local function vileSpiritDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
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
    -- soul-barrage schedule is PORTED 2026-11-08 (onOutroSoulBarrage,
    -- the phase model landed tick 441).
    elseif summoned:GetEntry() == NPC_FROSTMOURNE_TRIGGER then
        summoned:CastSpell(summoned, SPELL_BROKEN_FROSTMOURNE, true)
        creature:GetMap():SetZoneOverrideLight(AREA_ICECROWN_CITADEL, LIGHT_DEFAULT, LIGHT_SOULSTORM, 10000)
        creature:GetMap():SetZoneWeather(AREA_ICECROWN_CITADEL, WEATHER_STATE_BLACKSNOW, 0.5)
        local guid = creature:GetGUID()
        schedule(guid, "outro_soul_barrage", 5000, function() onOutroSoulBarrage(creature, guid) end)
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
    -- C++ :774-781: vile spirit REACT_PASSIVE + flight speed 0.5 +
    -- MoveRandom(10.0) + VileSpiritActivateEvent at 15s (unless in the
    -- Frostmourne phase). The flight-speed leg stays out: SetSpeedRate
    -- rejects MOVE_FLIGHT and Go models flight speed as a constant 7.0
    -- with no script-rate field (creaturemotion.go:679), so the spirit
    -- wanders at base speed; the Go MoveRandom is a single walk spline,
    -- not the repeating C++ wander generator. The Frostmourne-phase gate
    -- is PORTED 2026-11-08 (lkPhase read below — a spirit summoned
    -- during Frostmourne gets no 15s activate, per C++ :780).
    elseif summoned:GetEntry() == ENTRY_VILE_SPIRIT then
        summoned:SetReactState(0) -- REACT_PASSIVE
        summoned:MoveRandom(10.0)
        -- C++ :780: no VileSpiritActivateEvent while in PHASE_FROSTMOURNE
        -- (phase model landed 2026-11-08; the phase is only ever written
        -- by the unported EVENT_HARVEST_SOULS, so the gate is dormant).
        if lkPhase[creature:GetGUID()] ~= LK.PHASE_FROSTMOURNE then
            local spiritGuid = summoned:GetGUID()
            schedule(spiritGuid, "vile_activate", 15000, function() onVileSpiritActivate(summoned) end)
        end
    end
end

-- C++ boss_the_lich_king::DamageTaken (:685-728): health thresholds
-- on PRE-damage health — ev9 fires pre-application
-- (lua_creature_events.go:303; C++ Unit::DealDamage calls
-- DamageTaken at Unit.cpp:701, before ModifyHealth) — so
-- GetHealth() here IS the C++ HealthAbovePct operand; no
-- post-damage guard (C++ DamageTaken arms fire on lethal hits too,
-- the killing blow then runs JustDied's cancelTimers). 70% in
-- PHASE_ONE / 40% in PHASE_TWO: SetPhase(PHASE_TRANSITION) +
-- REACT_PASSIVE + AttackStop + InterruptNonMeleeSpells +
-- MovePoint(POINT_CENTER_1/2, CenterPosition). 10% in PHASE_THREE:
-- REACT_PASSIVE + AttackStop + events.Reset() (cancelTimers) +
-- SetPhase(PHASE_OUTRO) + CastSpell(SPELL_FURY_OF_FROSTMOURNE
-- 72350) + SetWalk(true) + the outro-talk schedule (:707-728);
-- summons.DespawnAll + SetZoneMusic stay out (no bridge). PORTED
-- 2026-11-08 (phase model; the "no MovementInform bridge" claim
-- below is stale — ev28 landed, lua_creature_events.go:4699/:5725).
local function lichKingDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local phase = lkPhase[guid]
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local health = creature:GetHealth()
    if phase == LK.PHASE_ONE and health <= math.floor(maxHealth * 70 / 100) then
        lkPhase[guid] = LK.PHASE_TRANSITION
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        creature:InterruptNonMeleeSpells()
        creature:MoveTo(LK.POINT_CENTER_1, LK.CENTER_X, LK.CENTER_Y, LK.CENTER_Z)
        return
    end
    if phase == LK.PHASE_TWO and health <= math.floor(maxHealth * 40 / 100) then
        lkPhase[guid] = LK.PHASE_TRANSITION
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        creature:InterruptNonMeleeSpells()
        creature:MoveTo(LK.POINT_CENTER_2, LK.CENTER_X, LK.CENTER_Y, LK.CENTER_Z)
        return
    end
    if phase == LK.PHASE_THREE and health <= math.floor(maxHealth * 10 / 100) then
        lkPhase[guid] = LK.PHASE_OUTRO
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        cancelTimers(guid)
        creature:InterruptNonMeleeSpells()
        creature:CastSpell(creature, LK.SPELL_FURY_OF_FROSTMOURNE)
        creature:SetWalk(true)
        schedule(guid, "outro_talk_1", 2600, function() onOutroTalk1(creature, guid) end)
        schedule(guid, "outro_emote_1", 6600, function() onOutroEmoteTalk(creature, guid) end)
        schedule(guid, "outro_emote_2", 17600, function() onOutroEmoteTalk(creature, guid) end)
        schedule(guid, "outro_emote_3", 27600, function() onOutroEmoteTalk(creature, guid) end)
        schedule(guid, "outro_talk_2", 34600, function() onOutroTalk2(creature, guid) end)
        schedule(guid, "outro_talk_3", 43600, function() onOutroTalk3(creature, guid) end)
        schedule(guid, "outro_shout", 54600, function() onOutroShout(creature, guid) end)
        schedule(guid, "outro_emote_4", 58600, function() onOutroEmoteTalk(creature, guid) end)
        schedule(guid, "outro_move_center", 69600, function() onOutroMoveCenter(creature, guid) end)
        return
    end
end

-- C++ boss_the_lich_king::MovementInform (:826-890, POINT_MOTION_TYPE
-- only): POINT_CENTER_1 — SetFacingTo(0) + Talk(SAY_LK_REMORSELESS_WINTER 4)
-- + DoCastSelf(SPELL_REMORSELESS_WINTER_1 68981) + EVENT_QUAKE 62500ms /
-- EVENT_PAIN_AND_SUFFERING 4s / EVENT_SUMMON_ICE_SPHERE 8s /
-- EVENT_SUMMON_RAGING_SPIRIT 6s (PHASE_TRANSITION) + EVENT_SUMMON_VALKYR 78s /
-- EVENT_INFEST 70s / EVENT_DEFILE 97s / EVENT_SOUL_REAPER 94s (PHASE_TWO).
-- POINT_CENTER_2 — same Talk + SPELL_REMORSELESS_WINTER_2 72259 +
-- EVENT_QUAKE_2 62500ms / PAIN_AND_SUFFERING 6s / ICE_SPHERE 8s /
-- EVENT_SUMMON_RAGING_SPIRIT_2 5s (PHASE_TRANSITION) + EVENT_DEFILE 95500ms /
-- EVENT_SOUL_REAPER 99500ms / EVENT_VILE_SPIRITS 79500ms /
-- EVENT_HARVEST_SOUL(S) 73500ms (PHASE_THREE; heroic seeds HARVEST_SOULS,
-- PORTED 2026-11-08, onHarvestSouls below).
-- POINT_LK_OUTRO_1 — EVENT_OUTRO_TALK_4 1ms / EVENT_OUTRO_RAISE_DEAD 1s /
-- EVENT_OUTRO_TALK_5 29s (PHASE_OUTRO). SetZoneMusic + CENTER_2
-- summons.DespawnEntry(VALKYR) stay out (no bridge); POINT_LK_INTRO_1..3
-- PORTED 2026-11-08 (onIntroMove1..3 arms below); POINT_LK_OUTRO_2
-- PORTED 2026-11-08 (tirion Talk SAY_TIRION_OUTRO_2 3 via the
-- FindNearestCreature sweep — the stale "no cross-AI-Talk bridge" claim
-- is retired: :Talk runs on any creature object, the mandokir
-- jindo:Talk precedent — + frostmourne DoAction ACTION_SUMMON_TERENAS).
-- PORTED 2026-11-08 via ev28 (POINT_MOTION_TYPE 8).
local function lichKingMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == LK.POINT_LK_INTRO_1 then
        -- C++ :833-836: schedule for next update cycle, current update
        -- must finalize movement (1ms, PHASE_INTRO). PORTED 2026-11-08.
        schedule(guid, "intro_move_2", 1, function() onIntroMove2(creature, guid) end)
    elseif id == LK.POINT_LK_INTRO_2 then
        -- C++ :837-839: same 1ms next-cycle schedule. PORTED 2026-11-08.
        schedule(guid, "intro_move_3", 1, function() onIntroMove3(creature, guid) end)
    elseif id == LK.POINT_LK_INTRO_3 then
        -- C++ :840-844: tirion->AI()->DoAction(ACTION_CONTINUE_INTRO)
        -- PORTED 2026-11-08 (tirion 38995 registered this tick; the
        -- FindNearestCreature sweep is the spiritWardenDied precedent)
        -- + EVENT_INTRO_TALK_1 9s.
        local tirion = creature:FindNearestCreature(ENTRY_TIRION_LK, 500)
        if tirion then
            tirion:DoAction(LK.ACTION_CONTINUE_INTRO)
        end
        schedule(guid, "intro_talk_1", 9000, function() onIntroTalk1(creature, guid) end)
    elseif id == LK.POINT_CENTER_1 then
        creature:SetFacingTo(0.0)
        creature:Talk(LK.SAY_LK_REMORSELESS_WINTER)
        creature:CastSpell(creature, LK.SPELL_REMORSELESS_WINTER_1)
        schedule(guid, "quake", 62500, function() onQuake(creature, guid, LK.PHASE_TWO) end)
        schedule(guid, "pain_suffering", 4000, function() onPainAndSuffering(creature, guid) end)
        schedule(guid, "ice_sphere", 8000, function() onSummonIceSphere(creature, guid) end)
        schedule(guid, "raging_spirit", 6000, function() onSummonRagingSpirit(creature, guid) end)
        schedule(guid, "valkyr", 78000, function() onSummonValkyr(creature, guid) end)
        schedule(guid, "infest", 70000, function() onInfest(creature, guid) end)
        schedule(guid, "defile", 97000, function() onDefile(creature, guid) end)
        schedule(guid, "soul_reaper", 94000, function() onSoulReaper(creature, guid) end)
    elseif id == LK.POINT_CENTER_2 then
        creature:SetFacingTo(0.0)
        creature:Talk(LK.SAY_LK_REMORSELESS_WINTER)
        creature:CastSpell(creature, LK.SPELL_REMORSELESS_WINTER_2)
        schedule(guid, "quake", 62500, function() onQuake(creature, guid, LK.PHASE_THREE) end)
        schedule(guid, "pain_suffering", 6000, function() onPainAndSuffering(creature, guid) end)
        schedule(guid, "ice_sphere", 8000, function() onSummonIceSphere(creature, guid) end)
        schedule(guid, "raging_spirit_2", 5000, function() onSummonRagingSpirit2(creature, guid) end)
        schedule(guid, "defile", 95500, function() onDefile(creature, guid) end)
        schedule(guid, "soul_reaper", 99500, function() onSoulReaper(creature, guid) end)
        schedule(guid, "vile_spirits", 79500, function() onVileSpirits(creature, guid) end)
        vileSpiritsDue[guid] = GetCurrTime() + 79500
        if not creature:IsHeroic() then
            schedule(guid, "harvest_soul", 73500, function() onHarvestSoul(creature, guid) end)
        else
            schedule(guid, "harvest_souls", 73500, function() onHarvestSouls(creature, guid) end)
        end
    elseif id == LK.POINT_LK_OUTRO_1 then
        schedule(guid, "outro_talk_4", 1, function() onOutroTalk4(creature, guid) end)
        schedule(guid, "outro_raise_dead", 1000, function() onOutroRaiseDead(creature, guid) end)
        schedule(guid, "outro_talk_5", 29000, function() onOutroTalk5(creature, guid) end)
    elseif id == LK.POINT_LK_OUTRO_2 then
        -- C++ :879-884: tirion->AI()->Talk(SAY_TIRION_OUTRO_2 3) +
        -- frostmourne->AI()->DoAction(ACTION_SUMMON_TERENAS), both via
        -- the FindNearestCreature sweep (50yd for the trigger).
        -- PORTED 2026-11-08.
        local tirion = creature:FindNearestCreature(ENTRY_TIRION_LK, 500)
        if tirion then
            tirion:Talk(LK.SAY_TIRION_OUTRO_2)
        end
        local frostmourne = creature:FindNearestCreature(NPC_FROSTMOURNE_TRIGGER, 50.0)
        if frostmourne then
            frostmourne:DoAction(LK.ACTION_SUMMON_TERENAS)
        end
    end
end

RegisterCreatureEvent(ENTRY_THE_LICH_KING, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 2, lichKingLeaveCombat)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 3, lichKingKilledUnit)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 4, lichKingJustDied)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 19, lichKingJustSummoned)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 9, lichKingDamageTaken)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 23, lichKingReset)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 28, lichKingMovementInform)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 38, lichKingDoAction)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 14, lichKingSpellHit)

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
-- Frostmourne-phase entry, issued by the boss AI :1066) PORTED
-- (2026-11-08, ragingSpiritDoAction, ev38 below): the receiver dispatch
-- is live in this file (lichKingDoAction/terenasDoAction precedent) —
-- the earlier "no DoAction bridge" note conflated receiver and sender.
-- The sender (EVENT_HARVEST_SOULS summon sweep) PORTED 2026-11-08
-- (onHarvestSouls below); until it fires, the Soul Shriek timer keeps
-- firing where C++ goes silent (gothik counterpoint).
local ragingFrostmournePhase = {}

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
    -- C++ EVENT_SET_AGRESSIVE Frostmourne-phase branch (:1427-1430):
    -- back to PHASE_THREE + re-arm SOUL_SHRIEK {12s,15s}.
    if ragingFrostmournePhase[guid] then
        ragingFrostmournePhase[guid] = nil
        schedule(guid, "soul_shriek", {12000, 15000}, function() onSoulShriek(creature, guid) end)
    end
end

-- C++ npc_raging_spirit::DoAction(ACTION_DISABLE_RAGING) (:1377-1386):
-- _events.Reset() + SetPhase(PHASE_FROSTMOURNE) + EVENT_SET_AGRESSIVE
-- in 52s + REACT_PASSIVE + AttackStop + InterruptNonMeleeSpells(true),
-- in C++ order. The Lua InterruptNonMeleeSpells takes no args (the
-- delayed=true arg is voided by the bridge). cancelTimers is the
-- _events.Reset() twin; the phase flag drives the EVENT_SET_AGRESSIVE
-- branch in onRagingAggressive above.
local function ragingSpiritDoAction(event, creature, action)
    if action ~= ACTION_DISABLE_RAGING then
        return
    end
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ragingFrostmournePhase[guid] = true
    schedule(guid, "raging_aggressive", 52000, function() onRagingAggressive(creature, guid) end)
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:InterruptNonMeleeSpells()
end

local function ragingSpiritJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ragingFrostmournePhase[guid] = nil
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
    ragingFrostmournePhase[creature:GetGUID()] = nil
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

-- C++ npc_valkyr_shadowguard::Reset (:1455-1460): REACT_PASSIVE +
-- DoCastSelf(SPELL_WINGS_OF_THE_DAMNED 74352, non-triggered); the ctor
-- (:1447) sets combat movement off. Keyed to ev22 IsSummonedBy (the
-- strangulate precedent — ev22 is live for creature summons): the
-- val'kyr never engages, so ev1 would never fire.
-- C++ npc_valkyr_shadowguard EVENT_GRAB_PLAYER (:1551-1556): every
-- 2s while no player is grabbed, DoCastAOE(SPELL_VALKYR_TARGET_SEARCH
-- 69030, self-cast twin per the harvest-souls precedent); the first
-- arm is the 2500ms IsSummonedBy schedule (:1461). The _grabbedPlayer
-- latch rides LK.valkyrGrabbed (set by the event-7 SelectTarget
-- handler when it picks — the AI SetGUID write has no bridge, and
-- the MovementInform drop legs that clear it stay out). PORTED
-- 2026-11-08. DORMANT until Spell.dbc ships: the 69030 cast is
-- DBC-gated (creature path needs spellKnown).
-- Assigned on the LK table, not a local: the file sits at 199
-- main-chunk locals and a named local would touch the 200-local
-- load limit (the tirionFns table-field precedent, tick 443).
LK.onValkyrGrab = function(creature, guid)
    if LK.valkyrGrabbed[guid] then
        return
    end
    creature:CastSpell(creature, LK.SPELL_VALKYR_TARGET_SEARCH, true)
    if not LK.valkyrGrabbed[guid] then
        schedule(guid, "valkyrGrab", 2000, function() LK.onValkyrGrab(creature, guid) end)
    end
end

local function valkyrIsSummonedBy(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetCombatMovement(false)
    creature:CastSpell(creature, SPELL_WINGS_OF_THE_DAMNED)
    local guid = creature:GetGUID()
    schedule(guid, "valkyrGrab", 2500, function() LK.onValkyrGrab(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 1, shamblingHorrorJustEngagedWith)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 2, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 9, shamblingHorrorDamageTaken)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 23, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 1, ragingSpiritJustEngagedWith)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 2, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 23, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 38, ragingSpiritDoAction)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 1, spiritWardenJustEngagedWith)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 2, spiritWardenLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 4, spiritWardenDied)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 23, spiritWardenLeaveCombat)
RegisterCreatureEvent(ENTRY_VALKYR_SHADOWGUARD, 22, valkyrIsSummonedBy)
-- C++ npc_valkyr_shadowguard death: the EventMap dies with the AI —
-- cancel the grab timer and drop the target-select state. Inline:
-- the file sits at 199 main-chunk locals.
RegisterCreatureEvent(ENTRY_VALKYR_SHADOWGUARD, 4, function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    LK.valkyrGrabbed[guid] = nil
    LK.valkyrTarget[guid] = nil
    LK.valkyrDropPoint[guid] = nil
end)

-- C++ npc_valkyr_shadowguard::MovementInform
-- (boss_the_lich_king.cpp:1486-1534) + DamageTaken (:1464-1477) +
-- ScheduleHeroicEvents (:1479-1484) + UpdateAI EVENT_MOVE_TO_DROP_POS
-- (:1558-1560) / EVENT_MOVE_TO_CENTER (:1565-1570) / EVENT_LIFE_SIPHON
-- (:1561-1564). PORTED 2026-11-08. The POINT_CHARGE arrival rides the
-- creatureEffectCharge MovementInform bridge (boss_ai.go, tick 448:
-- the Go charge snaps, so the (POINT_MOTION_TYPE 8, EVENT_CHARGE
-- 1003) inform fires at the snap).
-- POINT_CHARGE: if the grabbed player still resolves, strip
-- NOT_SELECTABLE, take the nearest world trigger (36171) within 150yd
-- more than 5yd above the Arthas platform (HeightDifferenceCheck,
-- reverse=true: keep triggerZ - platformZ > 5), cast VALKYR_CARRY
-- (74445) on the player, stash the trigger as the drop point, and
-- schedule the drop move 1.5s out; no player -> despawn (the platform
-- / trigger-miss early returns are C++-exact). GetCreaturesInRange
-- (range, entry) is the GetCreatureListWithEntryInGrid twin; the
-- nearest pick is the necrotic-jump sort idiom.
-- POINT_DROP_PLAYER: EJECT_ALL_PASSENGERS (68576, DoCastAOE twin) +
-- heroic ScheduleHeroicEvents (reset + MOVE_TO_CENTER 1ms; the
-- ClearUnitState(UNIT_STATE_EVADE) leg has no bridge) else despawn
-- 1s. The grab latch clears on the drop: it doubles as the
-- DamageTaken passenger proxy (the C++ HasAuraType(CONTROL_VEHICLE)
-- gate), and after the eject there is no passenger.
-- POINT_SIPHON: VALKYR_TRANSFORM (73501) self-cast +
-- REACT_AGGRESSIVE + DoZoneInCombat + LIFE_SIPHON (73488,
-- victim-cast) 2s, repeating every 2.5s; the 73489 heal leg stays
-- out — its AfterHit BP0 needs hit damage, no Go model.
-- DamageTaken: heroic-only, while a player is grabbed, pre-damage
-- health below 50% (the shambling-horror HealthBelowPctDamaged
-- idiom) -> eject + ScheduleHeroicEvents. The passenger gate rides
-- the valkyrGrabbed latch, set at pick time (one charge-flight
-- window early — noted, not fabricated).
-- Assigned on the LK table, not locals: the file sits at 199
-- main-chunk locals.
LK.valkyrMovementInform = function(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUID()
    if id == LK.POINT_CHARGE then
        local target = LK.valkyrTarget[guid]
        if target == nil then
            creature:DespawnOrUnsummon(0)
            return
        end
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        local platform = creature:FindNearestGameObject(LK.GO_ARTHAS_PLATFORM, 150)
        if platform == nil then
            return
        end
        local platformZ = platform:GetZ()
        local triggers = creature:GetCreaturesInRange(150, LK.NPC_WORLD_TRIGGER_INFINITE_AOI)
        local nearest = nil
        local nearestDist = nil
        for _, trigger in ipairs(triggers) do
            if trigger:GetZ() - platformZ > 5.0 then
                local dist = creature:GetDistance(trigger)
                if nearest == nil or dist < nearestDist then
                    nearest = trigger
                    nearestDist = dist
                end
            end
        end
        if nearest == nil then
            return
        end
        creature:CastSpell(target, LK.SPELL_VALKYR_CARRY)
        LK.valkyrDropPoint[guid] = {x = nearest:GetX(), y = nearest:GetY(), z = nearest:GetZ()}
        schedule(guid, "valkyrMoveToDrop", 1500, function()
            local drop = LK.valkyrDropPoint[guid]
            if drop ~= nil then
                creature:MoveTo(LK.POINT_DROP_PLAYER, drop.x, drop.y, drop.z)
            end
        end)
    elseif id == LK.POINT_DROP_PLAYER then
        creature:CastSpell(creature, LK.SPELL_EJECT_ALL_PASSENGERS)
        LK.valkyrGrabbed[guid] = nil
        LK.valkyrTarget[guid] = nil
        LK.valkyrDropPoint[guid] = nil
        if creature:IsHeroic() then
            cancelTimers(guid)
            schedule(guid, "valkyrMoveToCenter", 1, function() LK.onValkyrMoveToCenter(creature, guid) end)
        else
            creature:DespawnOrUnsummon(1000)
        end
    elseif id == LK.POINT_SIPHON then
        creature:CastSpell(creature, LK.SPELL_VALKYR_TRANSFORM)
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:DoZoneInCombat()
        schedule(guid, "valkyrLifeSiphon", 2000, function() LK.onValkyrLifeSiphon(creature, guid) end)
    end
end

LK.onValkyrMoveToCenter = function(creature, guid)
    local angle = math.random() * 2 * math.pi
    local r = math.random() * 4.0
    local _, _, homeZ = creature:GetHomePosition()
    creature:MoveTo(LK.POINT_SIPHON, LK.CENTER_X + r * math.cos(angle), LK.CENTER_Y + r * math.sin(angle), homeZ)
end

LK.onValkyrLifeSiphon = function(creature, guid)
    creature:CastSpell(nil, LK.SPELL_LIFE_SIPHON)
    schedule(guid, "valkyrLifeSiphon", 2500, function() LK.onValkyrLifeSiphon(creature, guid) end)
end

LK.valkyrDamageTaken = function(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if not creature:IsHeroic() or LK.valkyrGrabbed[guid] == nil then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 50 / 100) then
        creature:CastSpell(creature, LK.SPELL_EJECT_ALL_PASSENGERS)
        LK.valkyrGrabbed[guid] = nil
        LK.valkyrTarget[guid] = nil
        LK.valkyrDropPoint[guid] = nil
        cancelTimers(guid)
        schedule(guid, "valkyrMoveToCenter", 1, function() LK.onValkyrMoveToCenter(creature, guid) end)
    end
end

RegisterCreatureEvent(ENTRY_VALKYR_SHADOWGUARD, 28, LK.valkyrMovementInform)
RegisterCreatureEvent(ENTRY_VALKYR_SHADOWGUARD, 9, LK.valkyrDamageTaken)
RegisterCreatureEvent(ENTRY_VILE_SPIRIT, 4, vileSpiritDied)

-- C++ EVENT_FROSTMOURNE_TALK_1 (:1776-1780): Talk(SAY_TERENAS_INTRO_1 0)
-- + heroic DoCastAOE(SPELL_RESTORE_SOULS 73650). PORTED 2026-11-08.
local function onTerenasFrostmourneTalk1(creature, guid)
    creature:Talk(SAY_TERENAS_INTRO_1)
    if creature:IsHeroic() then
        creature:CastSpell(creature, SPELL_RESTORE_SOULS)
    end
end

-- C++ EVENT_FROSTMOURNE_TALK_2 (:1781-1783):
-- Talk(SAY_TERENAS_INTRO_2 1). PORTED 2026-11-08.
local function onTerenasFrostmourneTalk2(creature, guid)
    creature:Talk(SAY_TERENAS_INTRO_2)
end

-- C++ EVENT_FROSTMOURNE_TALK_3 (:1784-1785, non-heroic only):
-- Talk(SAY_TERENAS_INTRO_3 2). PORTED 2026-11-08.
local function onTerenasFrostmourneTalk3(creature, guid)
    creature:Talk(SAY_TERENAS_INTRO_3)
end

-- C++ EVENT_TELEPORT_BACK (:1807-1810): lichKing->AI()->
-- DoAction(ACTION_TELEPORT_BACK) — LK resolved via FindNearestCreature
-- (the terenasDamageTaken precedent) instead of instance GetGuidData.
-- The old "no cross-AI DoAction send bridge" claim is stale:
-- creature:DoAction sends ev38 (spiritWardenDied precedent).
-- PORTED 2026-11-08.
local function onTerenasTeleportBack(creature)
    local lichKing = creature:FindNearestCreature(ENTRY_THE_LICH_KING, 500)
    if lichKing then
        lichKing:DoAction(ACTION_TELEPORT_BACK)
    end
end

-- C++ EVENT_DESTROY_SOUL (:1800-1806, non-heroic only): SetFlag
-- NOT_SELECTABLE + nearest spirit warden (20yd) casts DESTROY_SOUL
-- (non-triggered) + DoCast(SPELL_TERENAS_LOSES_INSIDE) +
-- EVENT_TELEPORT_BACK 1s. PORTED 2026-11-08.
local function onTerenasDestroySoul(creature, guid)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    local warden = creature:FindNearestCreature(ENTRY_SPIRIT_WARDEN, 20.0)
    if warden then
        warden:CastSpell(nil, SPELL_DESTROY_SOUL)
    end
    creature:CastSpell(creature, SPELL_TERENAS_LOSES_INSIDE)
    schedule(guid, "terenas_teleport_back", 1000, function() onTerenasTeleportBack(creature) end)
end

-- C++ npc_terenas_menethil::DamageTaken (:1733-1752): lethal damage
-- is clamped to health-1 (sara second-return precedent); then, only
-- when not heroic and without SPELL_TERENAS_LOSES_INSIDE, SetFlag
-- NOT_SELECTABLE + DoCast(TERENAS_LOSES_INSIDE, self-cast per the
-- warden 1-arg precedent) + EVENT_TELEPORT_BACK 1s (PORTED
-- 2026-11-08, onTerenasTeleportBack below) + nearest spirit warden
-- (20yd) casts DESTROY_SOUL (non-triggered) + 2s despawn + terenas
-- 2s despawn. The EVENT_DESTROY_SOUL 1min leg rides the
-- DoAction/summon machine (PORTED 2026-11-08 for the heroic sender).
local function terenasDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    local newDamage = damage
    if health > 0 and damage >= health then
        newDamage = health - 1
        if not creature:HasAura(SPELL_TERENAS_LOSES_INSIDE) and not creature:IsHeroic() then
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:CastSpell(creature, SPELL_TERENAS_LOSES_INSIDE)
            local guid = creature:GetGUID()
            schedule(guid, "terenas_teleport_back", 1000, function() onTerenasTeleportBack(creature) end)
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

-- C++ npc_terenas_menethil::DoAction ACTION_FROSTMOURNE_INTRO
-- (:1699-1713): setActive(true) (no bridge) + SetFarVisible(false) +
-- non-heroic SetHealth(max/2) + DoCastSelf(SPELL_LIGHTS_FAVOR 69382)
-- + _events.Reset() + EVENT_FROSTMOURNE_TALK_1 2s +
-- EVENT_FROSTMOURNE_TALK_2 11s + non-heroic EVENT_DESTROY_SOUL 1min
-- + EVENT_FROSTMOURNE_TALK_3 25s. The sender is LK
-- EVENT_FROSTMOURNE_HEROIC (PORTED 2026-11-08, onFrostmourneHeroic
-- above) on heroic; on normal it rides the unported instance
-- EVENT_TELEPORT_TO_FROSTMOURNE. PORTED 2026-11-08.
-- ACTION_TELEPORT_BACK (:1714-1717): CastSpell(nullptr,
-- SPELL_RESTORE_SOUL, TRIGGERED_NONE) + DespawnOrUnsummon(3s).
-- PORTED 2026-11-05 (tick 215), census row 159, via the
-- spiritWardenDied JustDied relay above. (The terenas UpdateAI
-- EVENT_TELEPORT_BACK leg that sends the Lich King DoAction is
-- PORTED 2026-11-08, onTerenasTeleportBack above.)
local function terenasDoAction(event, creature, action)
    if action == ACTION_FROSTMOURNE_INTRO then
        local guid = creature:GetGUID()
        creature:SetFarVisible(false)
        if not creature:IsHeroic() then
            creature:SetHealth(math.floor(creature:GetMaxHealth() / 2))
        end
        creature:CastSpell(creature, SPELL_LIGHTS_FAVOR)
        cancelTimers(guid)
        schedule(guid, "frostmourne_talk_1", 2000, function() onTerenasFrostmourneTalk1(creature, guid) end)
        schedule(guid, "frostmourne_talk_2", 11000, function() onTerenasFrostmourneTalk2(creature, guid) end)
        if not creature:IsHeroic() then
            schedule(guid, "destroy_soul", 60000, function() onTerenasDestroySoul(creature, guid) end)
            schedule(guid, "frostmourne_talk_3", 25000, function() onTerenasFrostmourneTalk3(creature, guid) end)
        end
        return
    end
    if action ~= ACTION_TELEPORT_BACK then
        return
    end
    creature:CastSpell(nil, SPELL_RESTORE_SOUL)
    creature:DespawnOrUnsummon(3000)
end

RegisterCreatureEvent(ENTRY_TERENAS, 38, terenasDoAction)
RegisterCreatureEvent(ENTRY_TERENAS_HEROIC, 38, terenasDoAction)

-- C++ npc_terenas_menethil::IsSummonedBy (:1763-1773): _events.Reset()
-- + SetPhase(PHASE_OUTRO) + SetFacingToObject(lichKing) +
-- EVENT_OUTRO_TERENAS_TALK_1 2s / EVENT_OUTRO_TERENAS_TALK_2 14s
-- (PHASE_OUTRO). PORTED 2026-11-08 (ev22, the val'kyr / strangulate
-- precedent). No phase table: the Frostmourne-room summon fires this
-- hook too, but the FROSTMOURNE_INTRO DoAction (terenasDoAction
-- above) cancelTimers the guid immediately after the summon — the
-- C++ _events.Reset() twin — so the outro talks never fire there.
-- The summon itself rides the DBC effect of SPELL_SUMMON_TERENAS
-- 72420 (frostmourne trigger), dormant until Spell.dbc ships.
local function onTerenasOutroTalk1(creature, guid)
    creature:Talk(LK.SAY_TERENAS_OUTRO_1)
end

-- C++ EVENT_OUTRO_TERENAS_TALK_2 (:1786-1800): Talk
-- SAY_TERENAS_OUTRO_2 1 + DoCastAOE(SPELL_MASS_RESURRECTION 72429) +
-- lichKing DoAction(ACTION_FINISH_OUTRO) + SetImmuneToNPC(false) (no
-- bridge, stays out) + tirion AttackStart(lichKing).
local function onTerenasOutroTalk2(creature, guid)
    creature:Talk(LK.SAY_TERENAS_OUTRO_2)
    creature:CastSpell(creature, LK.SPELL_MASS_RESURRECTION)
    local lichKing = creature:FindNearestCreature(ENTRY_THE_LICH_KING, 500)
    if lichKing then
        lichKing:DoAction(LK.ACTION_FINISH_OUTRO)
        local tirion = creature:FindNearestCreature(ENTRY_TIRION_LK, 500)
        if tirion then
            tirion:AttackStart(lichKing)
        end
    end
end

local function terenasIsSummonedBy(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local lichKing = creature:FindNearestCreature(ENTRY_THE_LICH_KING, 500)
    if lichKing then
        creature:SetFacingToObject(lichKing)
    end
    schedule(guid, "terenas_outro_talk_1", 2000, function() onTerenasOutroTalk1(creature, guid) end)
    schedule(guid, "terenas_outro_talk_2", 14000, function() onTerenasOutroTalk2(creature, guid) end)
end

RegisterCreatureEvent(ENTRY_TERENAS, 22, terenasIsSummonedBy)
RegisterCreatureEvent(ENTRY_TERENAS_HEROIC, 22, terenasIsSummonedBy)

-- C++ npc_tirion_fordring_tft (boss_the_lich_king.cpp:1161-1300,
-- entry 38995) — REGISTERED 2026-11-08. The 13 handlers ride ONE
-- table local (tirionFns): the file sits near Lua's 200-local
-- main-chunk limit, and table-field lookup happens at call time, so
-- the malown-class forward-reference time-bomb cannot occur here.
-- The sender chain is live end-to-end now: LK Reset summons Tirion
-- (lichKingReset above) -> gossip kick -> MovementInform INTRO ->
-- LK DoAction START_ENCOUNTER -> LK MovementInform INTRO_3 ->
-- Tirion DoAction CONTINUE_INTRO -> intro scheduler; outro: LK
-- OUTRO_TALK_5 -> Tirion DoAction ACTION_OUTRO -> outro scheduler.
local tirionFns = {}

-- C++ Reset (:1166-1171): _events.Reset() + remove the gossip flag
-- when the Lich King is DONE. Keyed to ev23 (the lichKingReset twin).
tirionFns.reset = function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    tirionPhase[guid] = nil
    if creature:GetBossState(DATA_THE_LICH_KING) == BOSS_STATE_DONE then
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(LK.UNIT_NPC_FLAG_GOSSIP)))
    end
end

-- C++ MovementInform (:1173-1190, POINT_MOTION_TYPE only):
-- POINT_TIRION_INTRO -> EmoteState(READY2H 375) + LK DoAction
-- ACTION_START_ENCOUNTER (sweep twin); POINT_TIRION_OUTRO_1 ->
-- EVENT_OUTRO_JUMP 1ms (PHASE_OUTRO).
tirionFns.movementInform = function(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    if id == LK.POINT_TIRION_INTRO then
        creature:EmoteState(LK.EMOTE_STATE_READY2H)
        local lichKing = creature:FindNearestCreature(ENTRY_THE_LICH_KING, 500)
        if lichKing then
            lichKing:DoAction(ACTION_START_ENCOUNTER)
        end
    elseif id == LK.POINT_TIRION_OUTRO_1 then
        local guid = creature:GetGUID()
        schedule(guid, "tirion_outro_jump", 1, function() tirionFns.outroJump(creature, guid) end)
    end
end

-- C++ DoAction (:1192-1209): ACTION_CONTINUE_INTRO -> Talk
-- SAY_TIRION_INTRO_1 0 + EVENT_INTRO_TALK_1 34s (PHASE_INTRO);
-- ACTION_OUTRO -> SetPhase(PHASE_OUTRO) + EVENT_OUTRO_TALK_1 7s /
-- EVENT_OUTRO_BLESS 18s / EVENT_OUTRO_REMOVE_ICE 23s /
-- EVENT_OUTRO_MOVE_1 25s (PHASE_OUTRO).
tirionFns.doAction = function(event, creature, action)
    local guid = creature:GetGUID()
    if action == LK.ACTION_CONTINUE_INTRO then
        creature:Talk(LK.SAY_TIRION_INTRO_1)
        schedule(guid, "tirion_intro_talk_1", 34000, function() tirionFns.introTalk1(creature, guid) end)
    elseif action == LK.ACTION_OUTRO then
        tirionPhase[guid] = LK.PHASE_OUTRO
        schedule(guid, "tirion_outro_talk_1", 7000, function() tirionFns.outroTalk1(creature, guid) end)
        schedule(guid, "tirion_outro_bless", 18000, function() tirionFns.outroBless(creature, guid) end)
        schedule(guid, "tirion_outro_remove_ice", 23000, function() tirionFns.outroRemoveIce(creature, guid) end)
        schedule(guid, "tirion_outro_move_1", 25000, function() tirionFns.outroMove1(creature, guid) end)
    end
end

-- C++ SpellHit (:1211-1216): SPELL_ICE_LOCK 71614 -> SetFacingTo
-- (3.085098). The SPELL_BROKEN_FROSTMOURNE_KNOCK 72405 leg
-- (LoadEquipment(1), remove Ashbringer glow) stays out: no
-- LoadEquipment bridge (only a comment mention,
-- lua_creature_events.go:6557).
tirionFns.spellHit = function(event, creature, caster, spellId)
    if spellId == LK.SPELL_ICE_LOCK then
        creature:SetFacingTo(3.085098)
    end
end

-- C++ OnGossipSelect (:1218-1227): template GossipMenuId == menuId
-- && !gossipListId -> SetPhase(PHASE_INTRO) + remove the gossip flag
-- + SetWalk(true) + MovePoint(POINT_TIRION_INTRO). The Go hook does
-- not forward menuId (gossip.go:1305); per-entry recovery (the
-- barrettGossipSelect precedent — Tirion carries exactly one menu)
-- gates on listID == 0. The GossipComplete close packet is the
-- barrett sendActionToTirion twin (C++ returns false with the menu
-- cleared server-side).
tirionFns.gossipSelect = function(event, player, creature, sender, action, code, listID)
    if listID ~= 0 then
        return
    end
    local guid = creature:GetGUID()
    tirionPhase[guid] = LK.PHASE_INTRO
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(LK.UNIT_NPC_FLAG_GOSSIP)))
    creature:SetWalk(true)
    creature:MoveTo(LK.POINT_TIRION_INTRO, LK.TIRION_INTRO_X, LK.TIRION_INTRO_Y, LK.TIRION_INTRO_Z)
    player:GossipComplete()
end

-- C++ JustReachedHome (:1229-1232): EmoteState(EMOTE_ONESHOT_NONE 0).
-- Keyed to ev24 (the anubarak tribute ev24 precedent).
tirionFns.reachedHome = function(event, creature)
    creature:EmoteState(LK.EMOTE_ONESHOT_NONE)
end

-- C++ EVENT_INTRO_TALK_1 (:1240-1244, PHASE_INTRO): Talk
-- SAY_TIRION_INTRO_2 1 + EVENT_INTRO_EMOTE_1 2s +
-- EVENT_INTRO_CHARGE 5s.
tirionFns.introTalk1 = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:Talk(LK.SAY_TIRION_INTRO_2)
    schedule(guid, "tirion_intro_emote_1", 2000, function() tirionFns.introEmote1(creature, guid) end)
    schedule(guid, "tirion_intro_charge", 5000, function() tirionFns.introCharge(creature, guid) end)
end

-- C++ EVENT_INTRO_EMOTE_1 (:1245-1247, PHASE_INTRO):
-- HandleEmoteCommand(EMOTE_ONESHOT_POINT_NO_SHEATHE 397).
tirionFns.introEmote1 = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:PerformEmote(LK.EMOTE_ONESHOT_POINT_NO_SHEATHE)
end

-- C++ EVENT_INTRO_CHARGE (:1248-1251, PHASE_INTRO): SetWalk(false) +
-- MovePoint(POINT_TIRION_CHARGE); arrival triggers the LK
-- START_ATTACK chain via the unported Quake SpellScript.
tirionFns.introCharge = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_INTRO then
        return
    end
    creature:SetWalk(false)
    creature:MoveTo(LK.POINT_TIRION_CHARGE, LK.TIRION_CHARGE_X, LK.TIRION_CHARGE_Y, LK.TIRION_CHARGE_Z)
end

-- C++ EVENT_OUTRO_TALK_1 (:1259-1261, PHASE_OUTRO): Talk
-- SAY_TIRION_OUTRO_1 2.
tirionFns.outroTalk1 = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_OUTRO then
        return
    end
    creature:Talk(LK.SAY_TIRION_OUTRO_1)
end

-- C++ EVENT_OUTRO_BLESS (:1262-1264, PHASE_OUTRO): DoCastSelf
-- SPELL_LIGHTS_BLESSING 71797. 500ms IsCasting defer (the :1294
-- UpdateAI casting gate; mal_ganis idiom).
tirionFns.outroBless = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_OUTRO then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "tirion_outro_bless", 500, function() tirionFns.outroBless(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_LIGHTS_BLESSING)
end

-- C++ EVENT_OUTRO_REMOVE_ICE (:1265-1272, PHASE_OUTRO):
-- RemoveAurasDueToSpell(SPELL_ICE_LOCK 71614) — creature:RemoveAura
-- is the bridged twin (the moorabi precedent; RemoveAurasDueToSpell
-- has no binding, boss_mimiron.lua:81) +
-- SetEquipmentSlots(false, EQUIP_ASHBRINGER_GLOWING 50442; off-hand /
-- ranged EQUIP_NO_CHANGE) + SetFacingToObject(lichKing) + lichKing
-- DoAction(ACTION_PLAY_MUSIC) — the LK receiver is SetZoneMusic only
-- (no bridge), the send itself is the ported leg.
tirionFns.outroRemoveIce = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_OUTRO then
        return
    end
    creature:RemoveAura(LK.SPELL_ICE_LOCK)
    creature:SetEquipmentSlots(LK.EQUIP_ASHBRINGER_GLOWING, 0xFFFFFFFF, 0xFFFFFFFF)
    local lichKing = creature:FindNearestCreature(ENTRY_THE_LICH_KING, 500)
    if lichKing then
        creature:SetFacingToObject(lichKing)
        lichKing:DoAction(LK.ACTION_PLAY_MUSIC)
    end
end

-- C++ EVENT_OUTRO_MOVE_1 (:1273-1276, PHASE_OUTRO): MovePoint
-- POINT_TIRION_OUTRO_1 to OutroPosition1; arrival schedules
-- EVENT_OUTRO_JUMP 1ms (MovementInform above).
tirionFns.outroMove1 = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_OUTRO then
        return
    end
    creature:MoveTo(LK.POINT_TIRION_OUTRO_1, LK.OUTRO_POS_1_X, LK.OUTRO_POS_1_Y, LK.OUTRO_POS_1_Z)
end

-- C++ EVENT_OUTRO_JUMP (:1277-1279, PHASE_OUTRO): DoCastAOE
-- SPELL_JUMP 71809 (self-cast per the DoCastAOE convention).
-- 500ms IsCasting defer (the :1294 gate).
tirionFns.outroJump = function(creature, guid)
    if tirionPhase[guid] ~= LK.PHASE_OUTRO then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "tirion_outro_jump", 500, function() tirionFns.outroJump(creature, guid) end)
        return
    end
    creature:CastSpell(creature, LK.SPELL_JUMP)
end

RegisterCreatureEvent(ENTRY_TIRION_LK, 23, tirionFns.reset)
RegisterCreatureEvent(ENTRY_TIRION_LK, 24, tirionFns.reachedHome)
RegisterCreatureEvent(ENTRY_TIRION_LK, 28, tirionFns.movementInform)
RegisterCreatureEvent(ENTRY_TIRION_LK, 38, tirionFns.doAction)
RegisterCreatureEvent(ENTRY_TIRION_LK, 14, tirionFns.spellHit)
RegisterCreatureGossipEvent(ENTRY_TIRION_LK, 2, tirionFns.gossipSelect)

-- C++ npc_broken_frostmourne (boss_the_lich_king.cpp:1916-1967,
-- entry NPC_FROSTMOURNE_TRIGGER 38584) — REGISTERED 2026-11-08.
-- One table local (frostmourneFns), same 200-local-budget reason as
-- tirionFns above. The summon itself rides the DBC effect of
-- SPELL_SUMMON_BROKEN_FROSTMOURNE_3 73017 (LK EVENT_OUTRO_TALK_6),
-- so the ev22/ev38 arms are dormant until Spell.dbc ships — the
-- val'kyr / strangulate summoned-arms-ahead-of-the-cast precedent.
local frostmourneFns = {}

-- C++ IsSummonedBy (:1923-1927): SetPhase(PHASE_OUTRO) +
-- EVENT_OUTRO_KNOCK_BACK 3s (PHASE_OUTRO).
frostmourneFns.isSummonedBy = function(event, creature)
    local guid = creature:GetGUID()
    schedule(guid, "frostmourne_knock_back", 3000, function() frostmourneFns.knockBack(creature, guid) end)
end

-- C++ DoAction (:1930-1934): ACTION_SUMMON_TERENAS ->
-- EVENT_OUTRO_SUMMON_TERENAS 6s (PHASE_OUTRO). The sender is the LK
-- POINT_LK_OUTRO_2 arrival (lichKingMovementInform above).
frostmourneFns.doAction = function(event, creature, action)
    if action ~= LK.ACTION_SUMMON_TERENAS then
        return
    end
    local guid = creature:GetGUID()
    schedule(guid, "frostmourne_summon_terenas", 6000, function() frostmourneFns.summonTerenas(creature, guid) end)
end

-- C++ EVENT_OUTRO_KNOCK_BACK (:1948-1950): DoCastAOE
-- SPELL_BROKEN_FROSTMOURNE_KNOCK 72405 (self-cast per the DoCastAOE
-- convention; hits Tirion, whose SpellHit arm faces him — the
-- LoadEquipment leg there stays out, no bridge).
frostmourneFns.knockBack = function(creature, guid)
    creature:CastSpell(creature, LK.SPELL_BROKEN_FROSTMOURNE_KNOCK)
end

-- C++ EVENT_OUTRO_SUMMON_TERENAS (:1951-1953): DoCastAOE
-- SPELL_SUMMON_TERENAS 72420 (self-cast per the DoCastAOE
-- convention; DBC summon effect, dormant until Spell.dbc ships).
frostmourneFns.summonTerenas = function(creature, guid)
    creature:CastSpell(creature, LK.SPELL_SUMMON_TERENAS)
end

RegisterCreatureEvent(NPC_FROSTMOURNE_TRIGGER, 22, frostmourneFns.isSummonedBy)
RegisterCreatureEvent(NPC_FROSTMOURNE_TRIGGER, 38, frostmourneFns.doAction)

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
-- or terenas EVENT_TELEPORT_BACK (:1807-1810, PORTED 2026-11-08,
-- onTerenasTeleportBack above), so this fires once that relay lands.
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
-- cast (70541, PHASE_ONE timer) PORTED 2026-11-08 (onInfest,
-- phase-gated on the lkPhase model; const moved to the top block).

local function infestTick(event, aura, target)
    if target:HealthAbovePct(90) then
        aura:Remove()
        return true
    end
end

RegisterAuraEvent(LK.SPELL_INFEST, 1, infestTick)

-- C++ spell_the_lich_king_soul_reaper::OnPeriodic
-- (boss_the_lich_king.cpp:2321-2326): EFFECT_1 SPELL_AURA_
-- PERIODIC_DAMAGE. No PreventDefaultAction, so the Go default
-- damage tick still fires. The aura target casts SPELL_SOUL_
-- REAPER_BUFF 69410 on the caster (the Lich King), triggered.
-- The boss-side Soul Reaper cast (69409, PHASE_TWO_THREE timer)
-- PORTED 2026-11-08 (onSoulReaper, phase-gated on the lkPhase model;
-- const moved to the top block).
local SPELL_SOUL_REAPER_BUFF = 69410

local function soulReaperTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        target:CastSpell(caster, SPELL_SOUL_REAPER_BUFF, true)
    end
end

RegisterAuraEvent(LK.SPELL_SOUL_REAPER, 1, soulReaperTick)

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

-- C++ spell_the_lich_king_harvest_soul::OnRemove
-- (boss_the_lich_king.cpp:2538-2558) and
-- spell_the_lich_king_in_frostmourne_room::OnRemove
-- (boss_the_lich_king.cpp:2688-2709): identical bodies, both
-- AfterEffectRemove, EFFECT_0, AURA_EFFECT_HANDLE_REAL
-- (PERIODIC_DAMAGE for 68980, DUMMY for 74276). On remove by
-- death only, the target casts SPELL_HARVESTED_SOUL 72679 on
-- nil target with the Lich King GUID as original caster.
-- PORTED 2026-11-08 (tick 425). DORMANT: the senders are unported —
-- the LK phase-3 Harvest Soul cast (phase machine timer) for 68980
-- and the Frostmourne-room teleport chain for 74276 — so neither aura
-- is ever applied and the hook cannot fire yet. The remove hook fires
-- on the player funnel (spells.go:35238) with the harvested player as
-- target, and aura:GetRemoveMode() reads the removed record's
-- snapshot (lua_aura.go:244, DEATH = 5).
-- DELTAS, all exactness-preserving: (1) the C++ cast IS triggered —
-- the LK GUID passes through the CastSpellExtraArgs (ObjectGuid) ctor,
-- which sets TRIGGERED_FULL_MASK (SpellDefines.h:167) — so the Lua
-- cast passes true; but the player session path ignores the triggered
-- arg (lua_player.go:2857) and a nil target falls back to self
-- (:2867-2870), so the cast still fires. (2) The original-caster LK
-- GUID has no Lua twin: plain CastSpell takes only (target, spell,
-- triggered), and CastCustomSpell accepts an 8th originalCaster arg
-- but both paths ignore it, so it is omitted rather than passed as
-- dead text; the player target has no GetGuidData bridge either. The
-- 72679 targeting rides that originalCaster + DBC implicit targets
-- (no Spell.dbc), so the boss-side SAY_LK_FROSTMOURNE_KILL arm still
-- can't fire from this cast in Go — as the file header already
-- documents. (3) Both C++ Load() guards (instance script non-null)
-- have no twin and are omitted; both auras are only ever applied
-- inside ICC / the Frostmourne room. No power drain exists in
-- either C++ body, so ModifyPower is not needed for this port.
-- HARVEST_SOUL is RAID_MODE (68980, 74325, 74296, 74297,
-- boss_the_lich_king.cpp:193); the one C++ class covers all four,
-- so all four register the same handler (the rotface 4-variant
-- precedent). AURA_REMOVE_BY_DEATH = 5 already defined above.
local SPELL_HARVEST_SOUL_25N = 74325 -- boss_the_lich_king.cpp:193 (HARVEST_SOUL RAID_MODE, 25N)
-- SPELL_HARVEST_SOUL_10H = 74296 and SPELL_HARVEST_SOUL_25H = 74297
-- already defined above.
local SPELL_IN_FROSTMOURNE_ROOM = 74276 -- boss_the_lich_king.cpp:143
local SPELL_HARVESTED_SOUL = 72679 -- boss_the_lich_king.cpp:145

local function harvestSoulRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_DEATH then
        target:CastSpell(nil, SPELL_HARVESTED_SOUL, true)
    end
end

RegisterAuraEvent(LK.SPELL_HARVEST_SOUL, 3, harvestSoulRemove)
RegisterAuraEvent(SPELL_HARVEST_SOUL_25N, 3, harvestSoulRemove)
RegisterAuraEvent(SPELL_HARVEST_SOUL_10H, 3, harvestSoulRemove)
RegisterAuraEvent(SPELL_HARVEST_SOUL_25H, 3, harvestSoulRemove)
RegisterAuraEvent(SPELL_IN_FROSTMOURNE_ROOM, 3, harvestSoulRemove)

-- C++ spell_the_lich_king_necrotic_plague::OnRemove
-- (boss_the_lich_king.cpp:2003-2027): AfterEffectRemove, EFFECT_0
-- PERIODIC_DAMAGE, REAL. On remove by enemy spell, expire, or death
-- the target casts the jump (70338) on itself with the caster GUID
-- (TRIGGERED_FULL_MASK via the ObjectGuid ctor, SpellDefines.h:167)
-- and the caster (Lich King) self-casts Plague Siphon (74074),
-- triggered. PORTED 2026-11-08. DELTA: the SPELLVALUE_MAX_TARGETS 1
-- mod has no bridge — the jump SpellScript target-select (nearest,
-- resize 1) is unbridged, so the DBC target selection applies; same
-- delta as the jump-aura remove above.
local function necroticPlagueRemove(event, aura, target)
    local mode = aura:GetRemoveMode()
    if mode ~= AURA_REMOVE_BY_ENEMY_SPELL and mode ~= AURA_REMOVE_BY_EXPIRE and mode ~= AURA_REMOVE_BY_DEATH then
        return
    end
    target:CastSpell(target, SPELL_NECROTIC_PLAGUE_JUMP, true)
    local caster = aura:GetCaster()
    if caster ~= nil and caster:GetObjectType() == "Creature" then
        caster:CastSpell(caster, SPELL_PLAGUE_SIPHON, true)
    end
end

RegisterAuraEvent(LK.SPELL_NECROTIC_PLAGUE, 3, necroticPlagueRemove)

-- C++ spell_the_lich_king_shadow_trap_visual::OnRemove
-- (boss_the_lich_king.cpp:2141-2146): AfterEffectRemove, EFFECT_0
-- DUMMY, REAL. On remove by expire only, the trap casts the shadow
-- trap aura (73525) on itself, triggered. PORTED 2026-11-08. LIVE:
-- the trap spawn-visual cast (73530, lichKingJustSummoned :743-745)
-- is ported above, so the hook fires once the visual expires; the
-- 73525 effect resolution rides Spell.dbc.
local function shadowTrapVisualRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_EXPIRE then
        target:CastSpell(target, LK.SPELL_SHADOW_TRAP_AURA, true)
    end
end

RegisterAuraEvent(SPELL_SHADOW_TRAP_VISUAL, 3, shadowTrapVisualRemove)

-- C++ spell_the_lich_king_vile_spirits::OnPeriodic
-- (boss_the_lich_king.cpp:2436-2441): EFFECT_0 PERIODIC_DUMMY. On
-- every tick in 25-man, or every 5th tick ((tick-1)%5==0) otherwise,
-- the target (Lich King) casts the effect's trigger spell on nil
-- with the aura-effect + caster-GUID args (triggered). PORTED
-- 2026-11-08. DORMANT until Spell.dbc ships (the trigger-spell id
-- rides the DBC effect; GetTriggerSpell(0) reads 0 without it) and
-- until the EVENT_VILE_SPIRITS cast lands the aura (timer PORTED
-- 2026-11-08, tick 441).
local function vileSpiritsTick(event, aura, target)
    local tick = aura:GetTickNumber()
    if not target:Is25ManRaid() and (tick - 1) % 5 ~= 0 then
        return
    end
    local trigger = aura:GetTriggerSpell(0)
    if trigger ~= 0 then
        target:CastSpell(nil, trigger, true)
    end
end

RegisterAuraEvent(LK.SPELL_VILE_SPIRITS, 1, vileSpiritsTick)

-- C++ spell_the_lich_king_soul_rip::OnPeriodic
-- (boss_the_lich_king.cpp:2591-2605): EFFECT_0 PERIODIC_DUMMY.
-- PreventDefaultAction, then the caster (spirit warden) casts Soul
-- Rip Damage (69398) on the target with BP0 = 5000 * tick number
-- (aurEff args: triggered, original caster = caster GUID). PORTED
-- 2026-11-08. LIVE: the warden's Reset + EVENT_SOUL_RIP casts are
-- ported above, so the aura lands and the hook fires.
local function soulRipTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster ~= nil then
        caster:CastCustomSpell(target, LK.SPELL_SOUL_RIP_DAMAGE, true, 5000 * aura:GetTickNumber())
    end
    return true
end

RegisterAuraEvent(SPELL_SOUL_RIP, 1, soulRipTick)

-- C++ spell_the_lich_king_lights_favor::OnPeriodic + CalculateBonus
-- (boss_the_lich_king.cpp:2564-2579): EFFECT_0 PERIODIC_HEAL tick ->
-- EFFECT_1 (MOD_DAMAGE_PERCENT_DONE) RecalculateAmount(caster), and
-- CalculateBonus sets the amount to the caster's health pct
-- (canBeRecalculated = true). The Go aura object carries SetAmount
-- (lua_aura.go, the write side of GetAmount), so the periodic leg
-- ports as a direct write of effect 1; the creature damage fold
-- reads MOD_DAMAGE_PERCENT_DONE amounts (creature_auras.go:485), so
-- the leg is LIVE once the 69382 aura lands (cast ported at :1704).
-- No PreventDefault: the C++ periodic heal still ticks. Handler is
-- inline: the file sits at 199 main-chunk locals.
RegisterAuraEvent(SPELL_LIGHTS_FAVOR, 1, function(event, aura, target)
    local caster = aura:GetCaster()
    if caster ~= nil then
        aura:SetAmount(caster:GetHealthPct(), 1)
    end
end)

-- C++ spell_the_lich_king_dark_hunger::HandleProc
-- (boss_the_lich_king.cpp:2672-2683): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction, then the holder (spirit warden) self-casts
-- Dark Hunger Heal (69384) with BP0 = damage dealt / 2 (integer
-- division; zero damage returns after the prevent). PORTED
-- 2026-11-08. LIVE: the warden's Reset self-cast is ported above,
-- so the aura is live and the proc hook (ev6) fires from the
-- creature proc loop (boss_ai.go:10233).
local function darkHungerProc(event, aura, target, actor, procTarget, damage)
    if damage ~= nil and damage ~= 0 then
        target:CastCustomSpell(target, LK.SPELL_DARK_HUNGER_HEAL, true, math.floor(damage / 2))
    end
    return true
end

RegisterAuraEvent(SPELL_DARK_HUNGER, 6, darkHungerProc)

-- C++ spell_the_lich_king_trigger_vile_spirit::ActivateSpirit
-- (boss_the_lich_king.cpp:2732-2742): OnHit. GetHitCreature() nil-guard,
-- then VileSpiritActivateEvent(target).Execute(0, 0) — the activate is
-- the already-ported onVileSpiritActivate (:1461). PORTED 2026-11-08 on
-- the new RegisterSpellEvent bridge (SpellEventOnHit = 2, fired from
-- the creature cast path's DoDamageAndTriggers twin). The handler is
-- inline: the file sits at 199 main-chunk locals and a named local
-- would touch the 200-local load limit. DORMANT until Spell.dbc ships:
-- the 73582 cast is DBC-gated (creature path needs spellKnown), and
-- the Frostmourne-phase 3s re-cast sender (C++ :499) is still TODO.
RegisterSpellEvent(LK.SPELL_TRIGGER_VILE_SPIRIT_HEROIC, 2, function(event, spell, target)
    if target ~= nil and target:GetObjectType() == "Creature" then
        onVileSpiritActivate(target)
    end
end)

-- C++ spell_the_lich_king_quake::HandleSendEvent
-- (boss_the_lich_king.cpp:2185-2191): OnEffectHit EFFECT_1
-- SEND_EVENT -> caster AI DoAction(ACTION_START_ATTACK), which the
-- LK ev38 handler turns into the 5s EVENT_START_ATTACK schedule.
-- The effectIndex == 1 gate mirrors the EFFECT_1 registration (the
-- Go fire runs once per non-zero effect). The Load() instance-script
-- gate is always true in ICC; the FilterTargets platform-height
-- select (OnObjectAreaTargetSelect) stays out — the event-7 bridge
-- is live (tick 447) but the leg needs the Arthas-platform GO's Z
-- (HeightDifferenceCheck, no GO-position bridge in Lua). Handler
-- is inline: the file sits at 199 main-chunk locals. DORMANT until
-- Spell.dbc ships: the 72262 DoCastAOE is DBC-gated (creature path
-- needs spellKnown).
RegisterSpellEvent(LK.SPELL_QUAKE, 4, function(event, spell, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil and caster:GetObjectType() == "Creature" then
        caster:DoAction(ACTION_START_ATTACK)
    end
end)

-- C++ spell_the_lich_king_quake::FilterTargets
-- (boss_the_lich_king.cpp:2180-2184): OnObjectAreaTargetSelect
-- EFFECT_0 TARGET_UNIT_SRC_AREA_ENTRY -> drop targets more than 5yd
-- above the Arthas platform (HeightDifferenceCheck, reverse=false:
-- remove when unitZ - platformZ > 5). The platform GO rides
-- FindNearestGameObject(GO_ARTHAS_PLATFORM 202161) — the instance
-- GetGuidData(DATA_ARTHAS_PLATFORM 38) GUID has no Lua fetch, and the
-- GO object carries its Z via the object.go GetZ fallback. Handler is
-- inline: the file sits at 199 main-chunk locals. DORMANT until
-- Spell.dbc ships: the 72262 cast is DBC-gated.
RegisterSpellEvent(LK.SPELL_QUAKE, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local platform = caster:FindNearestGameObject(LK.GO_ARTHAS_PLATFORM, 150)
    if platform == nil then
        return
    end
    local platformZ = platform:GetZ()
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetZ() - platformZ > 5.0 then
            targets:Remove(target)
        end
    end
end)

-- C++ spell_the_lich_king_jump::HandleScript
-- (boss_the_lich_king.cpp:2749-2758): OnEffectHitTarget EFFECT_0
-- SCRIPT_EFFECT -> hit unit RemoveAurasDueToSpell(SPELL_RAISE_DEAD)
-- + cast SPELL_JUMP_2 triggered on nil + hit creature DoAction
-- (ACTION_BREAK_FROSTMOURNE). RemoveAura is the RemoveAurasDueToSpell
-- twin (moorabi precedent, tick 443); the DoAction rides the
-- GetHitCreature gate (GetObjectType check). The effectIndex == 0
-- gate mirrors the EFFECT_0 registration. Handler is inline: the
-- file sits at 199 main-chunk locals. DORMANT until Spell.dbc ships:
-- the 71809 DoCastAOE and the 72431 cast are DBC-gated.
RegisterSpellEvent(LK.SPELL_JUMP, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    target:RemoveAura(LK.SPELL_RAISE_DEAD)
    target:CastSpell(nil, LK.SPELL_JUMP_2, true)
    if target:GetObjectType() == "Creature" then
        target:DoAction(LK.ACTION_BREAK_FROSTMOURNE)
    end
end)

-- C++ spell_the_lich_king_play_movie::HandleScript
-- (boss_the_lich_king.cpp:2784-2799): OnEffectHitTarget EFFECT_0
-- SCRIPT_EFFECT -> hit player SendMovieStart
-- (MOVIE_FALL_OF_THE_LICH_KING). The Validate() movie-store lookup
-- has no Go twin (the id rides the const). The effectIndex == 0
-- gate mirrors the EFFECT_0 registration. Handler is inline: the
-- file sits at 199 main-chunk locals. DORMANT until Spell.dbc ships:
-- the 73159 DoCastAOE is DBC-gated.
RegisterSpellEvent(SPELL_PLAY_MOVIE, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    target:SendMovieStart(LK.MOVIE_FALL_OF_THE_LICH_KING)
end)

-- C++ spell_the_lich_king_restore_soul::HandleScript
-- (boss_the_lich_king.cpp:2621-2644): OnEffectHit EFFECT_0
-- APPLY_AURA -> LK DoAction(ACTION_TELEPORT_BACK) + nearest world
-- trigger (36171, 50yd) RemoveAllAuras + wicked spirits in 200yd:
-- KillAllEvents + REACT_PASSIVE + EnterEvadeMode. The LK lookup
-- rides FindNearestCreature (the onTerenasTeleportBack precedent);
-- KillAllEvents rides the file cancelTimers twin; EnterEvadeMode is
-- bridged (lua_creature_events.go:4823 — the 2026-10-21 "no bridge"
-- note is stale). The effectIndex == 0 gate mirrors the EFFECT_0
-- registration. Handler is inline: the file sits at 199
-- main-chunk locals. DORMANT until Spell.dbc ships: the 72595 cast
-- (Terenas ACTION_TELEPORT_BACK, non-triggered) is DBC-gated.
RegisterSpellEvent(SPELL_RESTORE_SOUL, 4, function(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local lichKing = caster:FindNearestCreature(ENTRY_THE_LICH_KING, 500)
    if lichKing then
        lichKing:DoAction(ACTION_TELEPORT_BACK)
    end
    local spawner = caster:FindNearestCreature(LK.NPC_WORLD_TRIGGER_INFINITE_AOI, 50.0)
    if spawner then
        spawner:RemoveAllAuras()
    end
    local spirits = caster:GetCreaturesInRange(200, LK.NPC_WICKED_SPIRIT)
    if spirits then
        for _, spirit in ipairs(spirits) do
            cancelTimers(spirit:GetGUID())
            spirit:SetReactState(REACT_PASSIVE)
            spirit:EnterEvadeMode()
        end
    end
end)

-- C++ spell_the_lich_king_restore_soul::RemoveAura
-- (boss_the_lich_king.cpp:2646-2652): BeforeHit -> on a hit (missInfo
-- == 0) remove the harvest-souls teleport aura from the hit unit
-- (heroic 73655, else 72546). The difficulty rides the caster's
-- IsHeroic (player objects have no IsHeroic; same map/difficulty as
-- the caster). RemoveAura is the RemoveAurasDueToSpell twin (moorabi
-- precedent, tick 443). Handler is inline: the file sits at 199
-- main-chunk locals.
RegisterSpellEvent(SPELL_RESTORE_SOUL, 6, function(event, spell, target, missInfo)
    if missInfo ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    local heroic = caster ~= nil and caster:IsHeroic()
    if heroic then
        target:RemoveAura(SPELL_HARVEST_SOULS_TELEPORT)
    else
        target:RemoveAura(SPELL_HARVEST_SOUL_TELEPORT)
    end
end)

-- C++ spell_the_lich_king_ice_burst_target_search::CheckTargetCount
-- (boss_the_lich_king.cpp:2205-2221): OnObjectAreaTargetSelect
-- EFFECT_0 TARGET_UNIT_SRC_AREA_ENEMY -> when the area list is
-- non-empty, the caster casts SPELL_ICE_BURST (69108, triggered),
-- goes REACT_PASSIVE, stops attacking and despawns in 500ms. The
-- effectIndex == 0 gate mirrors the EFFECT_0 registration; the
-- TYPEID_UNIT guard mirrors the C++ ToCreature() check. Handler is
-- inline: the file sits at 199 main-chunk locals. DORMANT until
-- Spell.dbc ships: the 69109 cast (ice sphere :752) is DBC-gated.
RegisterSpellEvent(LK.SPELL_ICE_BURST_TARGET_SEARCH, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 or targets:GetCount() == 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    caster:CastSpell(caster, LK.SPELL_ICE_BURST, true)
    caster:SetReactState(REACT_PASSIVE)
    caster:AttackStop()
    caster:DespawnOrUnsummon(500)
end)

-- C++ spell_the_lich_king_shadow_trap_periodic::CheckTargetCount
-- (boss_the_lich_king.cpp:2156-2168): OnObjectAreaTargetSelect
-- EFFECT_0 TARGET_UNIT_SRC_AREA_ENEMY -> when the area list is
-- non-empty, the caster casts SPELL_SHADOW_TRAP_KNOCKBACK (73529,
-- triggered, nil target). Registered on SPELL_SHADOW_TRAP_AURA 73525:
-- the trap's visual OnRemove casts it (ported tick 444), so this
-- fires at arming time when a player stands in the trap. Handler is
-- inline: the file sits at 199 main-chunk locals. DORMANT until
-- Spell.dbc ships: the 73525 cast is DBC-gated.
RegisterSpellEvent(LK.SPELL_SHADOW_TRAP_AURA, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 or targets:GetCount() == 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil then
        caster:CastSpell(nil, LK.SPELL_SHADOW_TRAP_KNOCKBACK, true)
    end
end)

-- C++ spell_the_lich_king_necrotic_plague_jump::SelectTarget
-- (boss_the_lich_king.cpp:2042-2049): OnObjectAreaTargetSelect
-- EFFECT_0 TARGET_UNIT_SRC_AREA_ENTRY -> sort the area list by
-- distance to the caster, keep only the nearest (resize(1); the
-- size < 2 early return leaves a 0/1 list untouched). The BeforeHit
-- CheckAura leg (:2053-2059) is a dead write in C++: _hadAura is
-- set but never read anywhere in the file, so it is not ported.
-- Handler is inline: the file sits at 199 main-chunk locals.
-- DORMANT until Spell.dbc ships: the 70338 cast (plague jump-aura
-- OnRemove) is DBC-gated.
RegisterSpellEvent(SPELL_NECROTIC_PLAGUE_JUMP, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if #list < 2 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    table.sort(list, function(a, b) return caster:GetDistance(a) < caster:GetDistance(b) end)
    for i = 2, #list do
        targets:Remove(list[i])
    end
end)

-- C++ spell_the_lich_king_defile::CorrectRange
-- (boss_the_lich_king.cpp:2264-2269): OnObjectAreaTargetSelect
-- EFFECT_0 and EFFECT_1 TARGET_UNIT_SRC_AREA_ENEMY -> drop targets
-- further than 10.0 * caster scale from the caster, and targets
-- carrying SPELL_HARVEST_SOUL_VALKYR (68985). No effectIndex gate:
-- the C++ leg is registered for both effects. GetDistance2d is the
-- C++ ExactDistanceCheck (2D); GetScale is Unit::GetObjectScale.
-- Handler is inline: the file sits at 199 main-chunk locals.
-- DORMANT until Spell.dbc ships: the 72762 cast is DBC-gated.
RegisterSpellEvent(LK.SPELL_DEFILE, 7, function(event, spell, effectIndex, targets)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local maxDist = 10.0 * caster:GetScale()
    for _, target in ipairs(targets:GetTargets()) do
        if caster:GetDistance2d(target) > maxDist or target:HasAura(LK.SPELL_HARVEST_SOUL_VALKYR) then
            targets:Remove(target)
        end
    end
end)

-- C++ spell_the_lich_king_defile::ChangeDamageAndGrow
-- (boss_the_lich_king.cpp:2271-2279): OnHit -> the SetHitDamage leg
-- is a NO-OP in C++ and ports as nothing: OnHit fires in
-- PreprocessTarget (Spell.cpp:2377, DoProcessTargetContainer pass
-- 1, before the effect loop) with TargetInfo::Damage = 0 (Spell.h),
-- so GetHitDamage() returns 0 and SetHitDamage(0 * caster scale)
-- leaves the seed at 0; EffectSchoolDamage then ADDS the rolled
-- damage on top (m_damage += damage, SpellEffects.cpp:788). No
-- scaling call is added — the Go bridge now models GetHitDamage
-- (0 at OnHit), and a ported SetHitDamage(GetHitDamage() * scale)
-- would write 0 and change nothing, exactly like C++. Only the
-- second leg ports: the caster casts SPELL_DEFILE_GROW (72756,
-- triggered) on itself. OnHit fires once per hit target in both
-- C++ and Go, so the grow-cast multiplicity matches. Handler is
-- inline: the file sits at 199 main-chunk locals. DORMANT until
-- Spell.dbc ships: the 72762 cast is DBC-gated.
RegisterSpellEvent(LK.SPELL_DEFILE, 2, function(event, spell, target)
    local caster = spell:GetCaster()
    if caster ~= nil then
        caster:CastSpell(caster, LK.SPELL_DEFILE_GROW, true)
    end
end)

-- C++ spell_the_lich_king_life_siphon::TriggerHeal
-- (boss_the_lich_king.cpp:2405-2411): AfterHit -> the hit unit casts
-- SPELL_LIFE_SIPHON_HEAL (73489) on the caster, TRIGGERED, BP0 =
-- GetHitDamage() * 10. GetHitDamage is the final post-mitigation
-- damage (C++ m_damage = damageInfo.damage, Spell.cpp:2547), which
-- the Go hit-damage bridge threads from the damage arms. No damage
-- guard: C++ casts unconditionally on a hit (a 0-damage hit heals
-- 0). The nil-target guard covers the miss fires (C++ fires
-- AfterHit with a nil hit unit on misses, Spell.cpp:2659 outside
-- the _spellHitTarget if). CastCustomSpell is the AddSpellBP0 twin
-- (saurfang 72202 precedent). Handler is inline: the file sits at
-- 199 main-chunk locals. DORMANT until Spell.dbc ships: the 73488
-- cast is DBC-gated.
RegisterSpellEvent(LK.SPELL_LIFE_SIPHON, 3, function(event, spell, target)
    if target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    target:CastCustomSpell(caster, LK.SPELL_LIFE_SIPHON_HEAL, true, spell:GetHitDamage() * 10)
end)

-- C++ spell_the_lich_king_vile_spirit_move_target_search::SelectTarget
-- (boss_the_lich_king.cpp:2509-2515): OnObjectAreaTargetSelect
-- EFFECT_0 TARGET_UNIT_SRC_AREA_ENEMY -> pick one random target
-- into _target. The list itself is NOT narrowed (the C++ leg only
-- records the pick); the OnEffectHitTarget HandleScript below gates
-- on it. The pick rides LK.vileMoveTarget keyed by caster GUID.
-- Handler is inline: the file sits at 199 main-chunk locals.
-- DORMANT until Spell.dbc ships: the 70501 cast (vile-spirit
-- activate :480) is DBC-gated.
RegisterSpellEvent(SPELL_VILE_SPIRIT_MOVE_SEARCH, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if #list == 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    LK.vileMoveTarget[caster:GetGUID()] = list[math.random(#list)]:GetGUID()
end)

-- C++ spell_the_lich_king_vile_spirit_move_target_search::HandleScript
-- (boss_the_lich_king.cpp:2517-2526): OnEffectHitTarget EFFECT_0
-- SCRIPT_EFFECT -> only the picked target gets AttackStart + 100000
-- threat; every other hit target is a no-op. PreventHitDefaultEffect
-- is a no-op in Go (a SCRIPT_EFFECT default is unhandled unless a
-- case exists, tick 446 contract). The effectIndex == 0 gate mirrors
-- the EFFECT_0 registration. Handler is inline: the file sits at 199
-- main-chunk locals.
RegisterSpellEvent(SPELL_VILE_SPIRIT_MOVE_SEARCH, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or target:GetGUID() ~= LK.vileMoveTarget[caster:GetGUID()] then
        return
    end
    caster:AttackStart(target)
    caster:AddThreat(target, 100000)
end)

-- C++ spell_the_lich_king_vile_spirit_damage_target_search::CheckTargetCount
-- (boss_the_lich_king.cpp:2539-2557): OnObjectAreaTargetSelect
-- EFFECT_0 TARGET_UNIT_SRC_AREA_ENEMY -> when the area list is
-- non-empty, the summoner AI gets SetData(DATA_VILE, 1), the caster
-- casts SPELL_SPIRIT_BURST (70503, triggered, nil target), despawns
-- in 3s and gains UNIT_FLAG_NOT_SELECTABLE. The Load() TYPEID_UNIT
-- gate holds (the caster is the vile-spirit creature). The summoner
-- rides FindNearestCreature (no GetSummoner bridge; the LK lookup
-- is the restore-soul precedent). The DATA_VILE write has no Go
-- reader yet (neck_deep achievement case unimplemented — the writer
-- lands now so the reader can follow). Handler is inline: the file
-- sits at 199 main-chunk locals. DORMANT until Spell.dbc ships: the
-- 70502 cast (vile-spirit activate :481) is DBC-gated.
RegisterSpellEvent(SPELL_VILE_SPIRIT_DAMAGE_SEARCH, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 or targets:GetCount() == 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local lichKing = caster:FindNearestCreature(ENTRY_THE_LICH_KING, 200.0)
    if lichKing ~= nil then
        lichKing:SetData(LK.DATA_VILE, 1)
    end
    caster:CastSpell(nil, LK.SPELL_SPIRIT_BURST, true)
    caster:DespawnOrUnsummon(3000)
    caster:SetFlag(59, UNIT_FLAG_NOT_SELECTABLE)
end)

-- C++ spell_the_lich_king_valkyr_target_search::SelectTarget
-- (boss_the_lich_king.cpp:2345-2358) + ReplaceTarget (:2360-2365):
-- OnObjectAreaTargetSelect EFFECT_0 / EFFECT_1
-- TARGET_UNIT_SRC_AREA_ENEMY -> EFFECT_0 drops targets carrying
-- the searcher aura itself, picks one random survivor into _target,
-- narrows the list to it and writes the GUID to the caster AI
-- (SetGUID has no bridge — the pick rides LK.valkyrTarget and the
-- _grabbedPlayer latch rides LK.valkyrGrabbed, read by the grab
-- timer above); EFFECT_1 clears the list and re-adds _target.
-- Handler is inline: the file sits at 199 main-chunk locals.
-- DORMANT until Spell.dbc ships: the 69030 cast (val'kyr grab
-- timer) is DBC-gated.
RegisterSpellEvent(LK.SPELL_VALKYR_TARGET_SEARCH, 7, function(event, spell, effectIndex, targets)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local guid = caster:GetGUID()
    if effectIndex == 0 then
        for _, target in ipairs(targets:GetTargets()) do
            if target:HasAura(LK.SPELL_VALKYR_TARGET_SEARCH) then
                targets:Remove(target)
            end
        end
        local list = targets:GetTargets()
        if #list == 0 then
            return
        end
        local picked = list[math.random(#list)]
        LK.valkyrTarget[guid] = picked
        LK.valkyrGrabbed[guid] = true
        for _, target in ipairs(list) do
            if target ~= picked then
                targets:Remove(target)
            end
        end
    elseif effectIndex == 1 then
        targets:Clear()
        local picked = LK.valkyrTarget[guid]
        if picked ~= nil then
            targets:Add(picked)
        end
    end
end)

-- C++ spell_the_lich_king_valkyr_target_search::HandleScript
-- (boss_the_lich_king.cpp:2367-2371): OnEffectHitTarget EFFECT_0
-- SCRIPT_EFFECT -> the caster casts SPELL_CHARGE (74399, triggered)
-- on the hit unit. The EFFECT_0 list is narrowed to _target by the
-- select above, so this fires once with the picked player.
-- PreventHitDefaultEffect is a no-op in Go (tick 446 contract).
-- Handler is inline: the file sits at 199 main-chunk locals.
RegisterSpellEvent(LK.SPELL_VALKYR_TARGET_SEARCH, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil then
        caster:CastSpell(target, LK.SPELL_CHARGE, true)
    end
end)

-- C++ spell_the_lich_king_raging_spirit::HandleScript
-- (boss_the_lich_king.cpp:2229-2243): OnEffectHitTarget EFFECT_0
-- SCRIPT_EFFECT -> hit unit casts GetEffectValue() on itself
-- (triggered). The value is the effect-0 DBC BasePoints of 69200
-- via the new spell:GetEffectValue bridge (CalcValueForLevel
-- twin of SpellEffectInfo::CalcValue); it is 0 until Spell.dbc
-- ships, so the port guards on a positive value and fires nothing
-- meanwhile. PreventHitDefaultEffect is a no-op in Go (tick 446
-- contract). Handler is inline: the file sits at 199 main-chunk
-- locals. DORMANT until Spell.dbc ships: the 69200 cast (raging
-- spirit schedule) is DBC-gated.
RegisterSpellEvent(LK.SPELL_RAGING_SPIRIT, 5, function(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local value = spell:GetEffectValue(effectIndex)
    if value <= 0 then
        return
    end
    target:CastSpell(target, value, true)
end)

-- C++ spell_the_lich_king_summon_spirit_bomb::HandleScript
-- (boss_the_lich_king.cpp:2710-2724): OnEffectHitTarget EFFECT_0
-- SCRIPT_EFFECT -> hit unit casts GetEffectValue() on nil
-- (triggered). Both bomb spells (73581/74299) share the script
-- name, so one table-field handler serves both registrations; a
-- table field costs no main-chunk local (file sits at 199). The
-- value is the effect-0 DBC BasePoints via spell:GetEffectValue,
-- 0 until Spell.dbc ships — guarded on positive.
-- PreventHitDefaultEffect is a no-op in Go (tick 446 contract).
-- DORMANT until Spell.dbc ships: the 73581/74299 casts (world
-- trigger 36171) are DBC-gated.
LK.onSummonSpiritBomb = function(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local value = spell:GetEffectValue(effectIndex)
    if value <= 0 then
        return
    end
    target:CastSpell(nil, value, true)
end
RegisterSpellEvent(LK.SPELL_SUMMON_SPIRIT_BOMB_1, 5, LK.onSummonSpiritBomb)
RegisterSpellEvent(LK.SPELL_SUMMON_SPIRIT_BOMB_2, 5, LK.onSummonSpiritBomb)
