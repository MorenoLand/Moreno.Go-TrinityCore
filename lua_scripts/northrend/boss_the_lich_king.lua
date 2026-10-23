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
-- boss_the_lich_king DoAction — Talk(SAY_LK_INTRO_1 0) rides
-- ACTION_START_ENCOUNTER; Talk(SAY_LK_FROSTMOURNE_ESCAPE 8, non-heroic)
-- rides ACTION_TELEPORT_BACK (no-DoAction bridge; the krick
-- ACTION_OUTRO precedent).
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
-- 72259 —> SetZoneOverrideLight) also stays out: no zone-light
-- bridge on the Lua surface.
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
-- movement machines, DoCast summon legs, phase SetPhase legs, and the
-- JustDied MoveFall / light-override legs ride bridges that do not
-- exist; the SPELL_PLAY_MOVIE (73159) and SetDisableGravity(false)
-- JustDied legs ARE ported (ev4 lichKingJustDied).
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
-- ACTION_CONTINUE_INTRO (no-DoAction bridge); Talk(SAY_TIRION_INTRO_2
-- 1) (EVENT_INTRO_TALK_1) and Talk(SAY_TIRION_OUTRO_1 2)
-- (EVENT_OUTRO_TALK_1) are scheduler-driven on the PHASE_INTRO /
-- PHASE_OUTRO machines (CreateLuaEvent exists, but the unbridged
-- phase/machine drivers gate them — gothik counterpoint); the
-- OnGossipSelect intro kick, MovementInform, and SpellHit legs have no
-- bridges (the bronjahm npc_corrupted_soul_fragment precedent).
-- npc_terenas_menethil (NPC_TERENAS_MENETHIL_FROSTMOURNE 36823,
-- NPC_TERENAS_MENETHIL_FROSTMOURNE_H 39217) — NOT registered:
-- Talk(SAY_TERENAS_INTRO_1..3 0..2) (EVENT_FROSTMOURNE_TALK_1..3,
-- DoAction ACTION_FROSTMOURNE_INTRO scheduler) and
-- Talk(SAY_TERENAS_OUTRO_1..2 0..1) (EVENT_OUTRO_TERENAS_TALK_1..2,
-- IsSummonedBy scheduler) — no-DoAction bridge; the timer bridge
-- exists but the phase/DoAction/summon-machine drivers gate every arm
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
-- + DoCastSelf(SPELL_BOSS_HITTIN_YA 73878, triggered) (:1374)
-- (the curator engage-cast precedent);
-- npc_spirit_warden (36824) — SOUL_RIP 69397 victim-cast
-- {12s,15s}/{23s,27s} (:1833/:1855-1856) + Reset one-shot
-- 1-arg DoCast(SPELL_DARK_HUNGER 69383) non-triggered (:1835) —
-- target resolved from DBC AISpellInfo (UnitAI.cpp:113), kept as
-- self-cast (self-buff aura).
-- npc_valkyr_shadowguard / npc_strangulate_vehicle /
-- npc_spirit_bomb / npc_broken_frostmourne — zero Talk lines —
-- NOT registered (the bronjahm npc_corrupted_soul_fragment
-- precedent); the valkyr grab/life-siphon (SpellScript +
-- MovementInform), strangulate vehicle, bomb and frostmourne UpdateAI
-- machines ride unbridged legs.
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
-- and the ice-sphere :750 / defile :761 legs act on unregistered adds;
-- spirit_bomb/valkyr/strangulate stay unregistered as documented.)
-- SpellScript legs: ice_burst_target_search REACT_PASSIVE (:2212-2221)
-- and restore_soul REACT_PASSIVE on wicked spirits (:2632-2641) — no
-- SpellScript bridge; both add AIs unregistered (restore_soul also
-- rides the no-DoAction-bridge ACTION_TELEPORT_BACK).

local ENTRY_THE_LICH_KING = 36597
local NPC_FROSTMOURNE_TRIGGER = 38584 -- boss_the_lich_king.h:331
local ENTRY_SHAMBLING_HORROR = 37698
local ENTRY_RAGING_SPIRIT = 36701
local ENTRY_SPIRIT_WARDEN = 36824

local SAY_LK_KILL = 10
local SAY_LK_BERSERK = 11

local SPELL_BERSERK2 = 47008
local SPELL_HORROR_SHOCKWAVE = 72149
local SPELL_HORROR_ENRAGE = 72143
local SPELL_HORROR_FRENZY = 28747
local SPELL_SOUL_SHRIEK = 69242
local SPELL_PLAGUE_AVOIDANCE = 72846
local SPELL_RAGING_SPIRIT_VISUAL = 69197
local SPELL_BOSS_HITTIN_YA = 73878
local SPELL_DARK_HUNGER = 69383
local SPELL_SOUL_RIP = 69397
local SPELL_KILL_FROSTMOURNE_PLAYERS = 75127
local SPELL_PLAY_MOVIE = 73159

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
    creature:Talk(SAY_LK_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK2)
end

-- C++ boss_the_lich_king::JustEngagedWith schedules EVENT_BERSERK
-- (:583); the phase-one machine arms are unbridged. No C++ Talk on
-- engage. Event 1 registered for the berserk timer only.
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
    cancelTimers(creature:GetGUID())
end

-- C++ boss_the_lich_king::JustDied (boss_the_lich_king.cpp:549-562):
-- _JustDied() (instance boss-state machine, unported) then
-- DoCastAOE(SPELL_PLAY_MOVIE 73159, false). DoCastAOE is self-cast
-- per UnitAI.h:322, so creature:CastSpell(creature, SPELL_PLAY_MOVIE).
-- The MoveFall, zone light/weather, and 25-man
-- quest-credit legs ride unbridged machines.
local function lichKingJustDied(event, creature)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_PLAY_MOVIE)
    creature:SetDisableGravity(false) -- C++ :553, after the movie cast
    -- C++ :555-556: FindNearestCreature(NPC_FROSTMOURNE_TRIGGER, 50.0f)
    -- -> DespawnOrUnsummon (FindNearestCreature and Despawn both bridged;
    -- stale "unbridged machines" claim retracted).
    local frostmourne = creature:FindNearestCreature(NPC_FROSTMOURNE_TRIGGER, 50.0)
    if frostmourne then
        frostmourne:Despawn()
    end
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

RegisterCreatureEvent(ENTRY_THE_LICH_KING, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 2, lichKingLeaveCombat)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 3, lichKingKilledUnit)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 4, lichKingJustDied)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 23, lichKingReset)

-- C++ npc_shambling_horror_icc: Reset seeds EVENT_SHOCKWAVE {20s,25s}
-- and EVENT_ENRAGE {11s,14s} (:1303-1304).
local function onHorrorShockwave(creature, guid)
    creature:CastSpell(creature, SPELL_HORROR_SHOCKWAVE)
    schedule(guid, "horror_shockwave", {20000, 25000}, function() onHorrorShockwave(creature, guid) end)
end

local function onHorrorEnrage(creature, guid)
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
-- true) + DoCastSelf(SPELL_RAGING_SPIRIT_VISUAL, true) +
-- DoCastSelf(SPELL_BOSS_HITTIN_YA 73878, true) one-shots,
-- EVENT_SOUL_SHRIEK {12s,15s} (:1367-1374/:1420-1421); the
-- Reset REACT_PASSIVE (:1365) -> EVENT_SET_AGRESSIVE 2s (:1424) leg is
-- PORTED (2026-10-22) via the file's CreateLuaEvent pump.
-- C++ DoAction(ACTION_DISABLE_RAGING) (:1377-1386, _events.Reset() on
-- Frostmourne-phase entry, issued by the boss AI :1066) has no bridge —
-- the cross-AI DoAction rides the unbridged phase machine, so the Soul
-- Shriek timer keeps firing where C++ goes silent (gothik counterpoint).
local function onSoulShriek(creature, guid)
    creature:CastSpell(creature, SPELL_SOUL_SHRIEK)
    schedule(guid, "soul_shriek", {12000, 15000}, function() onSoulShriek(creature, guid) end)
end

local function ragingSpiritJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetReactState(REACT_PASSIVE)
    schedule(guid, "raging_aggressive", 2000, function() creature:SetReactState(REACT_AGGRESSIVE) end)
    creature:CastSpell(creature, SPELL_PLAGUE_AVOIDANCE, true)
    creature:CastSpell(creature, SPELL_RAGING_SPIRIT_VISUAL, true)
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

RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 1, shamblingHorrorJustEngagedWith)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 2, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 9, shamblingHorrorDamageTaken)
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 23, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 1, ragingSpiritJustEngagedWith)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 2, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 23, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 1, spiritWardenJustEngagedWith)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 2, spiritWardenLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 23, spiritWardenLeaveCombat)
