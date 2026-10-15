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
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent; the !me->IsInEvadeMode() and
-- !events.IsInPhase(PHASE_OUTRO) legs have no bridges).
-- DOCUMENTED-ONLY (in this header; only entry 36597 registered):
-- boss_the_lich_king DoAction — Talk(SAY_LK_INTRO_1 0) rides
-- ACTION_START_ENCOUNTER; Talk(SAY_LK_FROSTMOURNE_ESCAPE 8, non-heroic)
-- rides ACTION_TELEPORT_BACK (no-DoAction bridge; the krick
-- ACTION_OUTRO precedent).
-- boss_the_lich_king SpellHit — Talk(SAY_LK_FROSTMOURNE_KILL 9) on
-- SPELL_HARVESTED_SOUL (event 15 never fires; the anub_arak
-- precedent).
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
-- Talk(SAY_LK_BERSERK 11) (EVENT_BERSERK) — all timer-driven
-- (no-timer-bridge; the boss_toravon precedent); the intro/outro
-- movement machines, DoCast summon legs, phase SetPhase legs, and the
-- JustDied SPELL_PLAY_MOVIE / MoveFall / light-override legs ride
-- bridges that do not exist.
-- npc_tirion_fordring_tft (NPC_HIGHLORD_TIRION_FORDRING_LK 38995) —
-- NOT registered: Talk(SAY_TIRION_INTRO_1 0) rides DoAction
-- ACTION_CONTINUE_INTRO (no-DoAction bridge); Talk(SAY_TIRION_INTRO_2
-- 1) (EVENT_INTRO_TALK_1) and Talk(SAY_TIRION_OUTRO_1 2)
-- (EVENT_OUTRO_TALK_1) are scheduler-driven (no-timer-bridge); the
-- OnGossipSelect intro kick, MovementInform, and SpellHit legs have no
-- bridges (the bronjahm npc_corrupted_soul_fragment precedent).
-- npc_terenas_menethil (NPC_TERENAS_MENETHIL_FROSTMOURNE 36823,
-- NPC_TERENAS_MENETHIL_FROSTMOURNE_H 39217) — NOT registered:
-- Talk(SAY_TERENAS_INTRO_1..3 0..2) (EVENT_FROSTMOURNE_TALK_1..3,
-- DoAction ACTION_FROSTMOURNE_INTRO scheduler) and
-- Talk(SAY_TERENAS_OUTRO_1..2 0..1) (EVENT_OUTRO_TERENAS_TALK_1..2,
-- IsSummonedBy scheduler) — no-DoAction + no-timer bridges.
-- PORTED (2026-10-13 retraction re-audit): npc_shambling_horror_icc
-- (37698) — SHOCKWAVE 72149 self-cast {20s,25s} init/re-arm
-- (:1303/:1331-1332); ENRAGE 72143 self-cast {11s,14s}/{20s,25s}
-- (:1304/:1335-1337 — the 1s cast-fail recheck leg is unmodelable,
-- documented); the heroic-only DamageTaken frenzy latch (:1308-1313,
-- SPELL_FRENZY 28747 triggered at HealthBelowPctDamaged(20)) stays
-- parked — no difficulty bridge (the tharon_ja heroic-59228 precedent);
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

local ENTRY_THE_LICH_KING = 36597
local ENTRY_SHAMBLING_HORROR = 37698
local ENTRY_RAGING_SPIRIT = 36701
local ENTRY_SPIRIT_WARDEN = 36824

local SAY_LK_KILL = 10
local SAY_LK_BERSERK = 11

local SPELL_BERSERK2 = 47008
local SPELL_HORROR_SHOCKWAVE = 72149
local SPELL_HORROR_ENRAGE = 72143
local SPELL_SOUL_SHRIEK = 69242
local SPELL_PLAGUE_AVOIDANCE = 72846
local SPELL_RAGING_SPIRIT_VISUAL = 69197
local SPELL_BOSS_HITTIN_YA = 73878
local SPELL_DARK_HUNGER = 69383
local SPELL_SOUL_RIP = 69397
local SPELL_KILL_FROSTMOURNE_PLAYERS = 75127

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

-- C++ boss_the_lich_king::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER &&
--     !me->IsInEvadeMode() && !events.IsInPhase(PHASE_OUTRO))
--     Talk(SAY_LK_KILL) — the razuvious player-gated variant precedent
-- (the evade and phase legs have no bridges).
local function lichKingKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_LK_KILL)
    end
end

RegisterCreatureEvent(ENTRY_THE_LICH_KING, 1, lichKingJustEngagedWith)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 2, lichKingLeaveCombat)
RegisterCreatureEvent(ENTRY_THE_LICH_KING, 3, lichKingKilledUnit)
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

-- C++ npc_raging_spirit: Reset — DoCastSelf(SPELL_PLAGUE_AVOIDANCE,
-- true) + DoCastSelf(SPELL_RAGING_SPIRIT_VISUAL, true) +
-- DoCastSelf(SPELL_BOSS_HITTIN_YA 73878, true) one-shots,
-- EVENT_SOUL_SHRIEK {12s,15s} (:1367-1374/:1420-1421); the
-- REACT_PASSIVE -> EVENT_SET_AGRESSIVE 2s leg has no bridge.
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
RegisterCreatureEvent(ENTRY_SHAMBLING_HORROR, 23, shamblingHorrorLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 1, ragingSpiritJustEngagedWith)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 2, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_RAGING_SPIRIT, 23, ragingSpiritLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 1, spiritWardenJustEngagedWith)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 2, spiritWardenLeaveCombat)
RegisterCreatureEvent(ENTRY_SPIRIT_WARDEN, 23, spiritWardenLeaveCombat)
