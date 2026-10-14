-- Blood-Queen Lana'thel (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_blood_queen_lana_thel.cpp
-- (843 lines incl. license; 1 CreatureScript
-- (boss_blood_queen_lana_thel (BossAI, DATA_BLOOD_QUEEN_LANA_THEL = 8))
-- + 5 SpellScripts (spell_blood_queen_vampiric_bite,
-- spell_blood_queen_bloodbolt, spell_blood_queen_pact_of_the_darkfallen,
-- spell_blood_queen_pact_of_the_darkfallen_dmg_target,
-- spell_blood_queen_twilight_bloodbolt) + 3 AuraScripts
-- (spell_blood_queen_frenzied_bloodthirst,
-- spell_blood_queen_essence_of_the_blood_queen,
-- spell_blood_queen_pact_of_the_darkfallen_dmg) + 2
-- AchievementCriteriaScripts (achievement_once_bitten_twice_shy_n,
-- achievement_once_bitten_twice_shy_v); all registered from inside
-- AddSC_boss_blood_queen_lana_thel(); loader decl 179 / call 374 per
-- northrend_script_loader.cpp — the NINTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_blood_prince_council() (call 373); the call after
-- it is AddSC_boss_sister_svalna() (call 375) — verified from the
-- loader this run; the checkpoint sequence (blood_prince_council ->
-- blood_queen_lana_thel) is followed).
-- Entry: 37955 Blood-Queen Lana'thel (icecrown_citadel.h
-- NPC_BLOOD_QUEEN_LANA_THEL, line 266; icecrown_citadel.h
-- DATA_BLOOD_QUEEN_LANA_THEL = 8, line 82;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_BLOOD_QUEEN_LANA_THEL, line 270) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_blood_queen_lana_thel" hits
-- boss_blood_queen_lana_thel.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings
-- are DB-side by construction. No lana'thel lua existed.
-- (The npc_blood_queen_lana_thel PassiveAI referenced by the
-- council file's intro legs lives in boss_blood_prince_council.cpp
-- and was documented there, not here.)
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 37955):
-- boss_blood_queen_lana_thel JustEngagedWith — instance legs
-- (instance->CheckRequiredBosses / DoCastSpellOnPlayers(
-- LIGHT_S_HAMMER_TELEPORT) / DoZoneInCombat / instance->
-- SetBossState(DATA_BLOOD_QUEEN_LANA_THEL, IN_PROGRESS)) have no
-- instance bridge; PORTED (2026-10-13): the three triggered
-- self-cast auras (:188-190) — curator engage-cast precedent.
-- boss_blood_queen_lana_thel JustDied — DoCastAOE(
-- SPELL_BLOOD_INFUSION_CREDIT) leg has no-cast bridge;
-- instance->SetData(DATA_BLOOD_QUICKENING_STATE, DONE) /
-- RewardPlayerAndGroupAtEvent / FindNearestCreature minchar legs
-- have no instance bridge.
-- boss_blood_queen_lana_thel JustReachedHome — Talk(SAY_WIPE 9)
-- rides with the unbridged instance->SetBossState(FAIL) leg; no
-- event-24 port precedent exists in the lua set, so it is
-- documented here rather than wired.
-- boss_blood_queen_lana_thel MovementInform — Talk(SAY_AIR_PHASE
-- 7) rides the POINT_AIR movement-inform leg (no MovementInform
-- bridge); the air-phase machine (MovePoint / SetDisableGravity /
-- SetReactState / scheduler reschedule legs) has no bridge.
-- PORTED (2026-10-13 retraction re-audit): EVENT_BERSERK —
-- single-fire 330s — Talk(EMOTE_BERSERK_RAID 12) +
-- Talk(SAY_BERSERK 10) + DoCastSelf(SPELL_BERSERK 26662)
-- non-triggered (:160/:351-354); the "no-timer-bridge" block for
-- this arm was stale doctrine (the boss_toravon precedent).
-- boss_blood_queen_lana_thel UpdateAI — 2026-10-14 retraction
-- re-audit: the stale "no-timer-bridge" claim for the remaining
-- scheduler arms is RETRACTED (CreateLuaEvent is a real bridge,
-- engine/scripting/runtime.go:798); each arm is classified against
-- the C++ individually (gothik precedent). Talk(SAY_VAMPIRIC_BITE 1)
-- rides EVENT_VAMPIRIC_BITE (:356-368) — OUT: Talk fires inside the
-- SelectRandomTarget-found branch (:365, random-target machine, no
-- bridge). Talk(SAY_PACT_OF_THE_DARKFALLEN 6) rides
-- EVENT_PACT_OF_THE_DARKFALLEN (:407-420) — OUT: Talk fires only if
-- targets.size() > 1 after SelectRandomTarget + RandomResize(2/3 by
-- Is25ManRaid, :409-413, random-target machine, no bridge).
-- Talk(EMOTE_SWARMING_SHADOWS 5, target) / Talk(SAY_SWARMING_SHADOWS
-- 4) ride EVENT_SWARMING_SHADOWS (:421-428) — OUT: Talk fires only if
-- SelectRandomTarget returns a player (random-target machine, no
-- bridge; the emote's target leg has no targeted-Talk bridge either).
-- The Blood Mirror / Delirious Slash / Twilight Bloodbolt DoCast legs
-- have no-cast bridge.
-- boss_blood_queen_lana_thel DoAction — ACTION_KILL_MINCHAR leg
-- (no-DoAction bridge; the krick ACTION_OUTRO precedent); the
-- EnterEvadeMode / SetGUID vampire-bloodbolt GUID-set bookkeeping
-- rides GUID / cross-AI bridges that do not exist; the quest-credit
-- legs ride the absent quest bridge.
-- spell_blood_queen_frenzied_bloodthirst (AuraScript) —
-- bloodQueen->AI()->Talk(EMOTE_BLOODTHIRST 3, GetTarget()) OnApply
-- and bloodQueen->AI()->Talk(SAY_MIND_CONTROL 2) OnRemove are
-- cross-AI Talk (no cross-AI-Talk bridge); the remaining AuraScript
-- + 5 SpellScript + 2 AchievementCriteriaScript legs join the
-- no-AuraScript-bridge / no-SpellScript-bridge /
-- no-achievement-criteria-bridge queues (the boss_moragg
-- optic-link precedent).
-- All 13 Talk() calls in the file accounted for (13 =
-- SAY_AGGRO 0 (1) + SAY_VAMPIRIC_BITE 1 (1, scheduler) +
-- SAY_MIND_CONTROL 2 (1, cross-AI aura leg) +
-- EMOTE_BLOODTHIRST 3 (1, cross-AI aura leg) +
-- SAY_SWARMING_SHADOWS 4 (1, scheduler) + EMOTE_SWARMING_SHADOWS
-- 5 (1, scheduler) + SAY_PACT_OF_THE_DARKFALLEN 6 (1, scheduler)
-- + SAY_AIR_PHASE 7 (1, MovementInform) + SAY_KILL 8 (1) +
-- SAY_WIPE 9 (1, JustReachedHome) + SAY_BERSERK 10 (1, scheduler)
-- + SAY_DEATH 11 (1) + EMOTE_BERSERK_RAID 12 (1, scheduler)).

local ENTRY_BLOOD_QUEEN_LANA_THEL = 37955

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_DEATH = 11
local SAY_BERSERK = 10
local EMOTE_BERSERK_RAID = 12

local SPELL_BERSERK = 26662

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ EVENT_BERSERK: 330s single-fire — Talk(EMOTE_BERSERK_RAID) +
-- Talk(SAY_BERSERK) + DoCastSelf(SPELL_BERSERK) non-triggered
-- (:160/:351-354), ungated one-shot.
local function onBerserk(creature, guid)
    creature:Talk(EMOTE_BERSERK_RAID)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

local SPELL_SHROUD_OF_SORROW = 70986
local SPELL_FRENZIED_BLOODTHIRST_VISUAL = 71949
local SPELL_CLEAR_ALL_STATUS_AILMENTS = 70939

-- C++ boss_blood_queen_lana_thel::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch (:174-180) has no bridge (same
-- accepted deviation as the Talk).
local function lanaThelJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ :188-190 — triggered self-cast auras (curator engage-cast
    -- precedent): DoCast(me, SPELL_SHROUD_OF_SORROW, true) +
    -- DoCast(me, SPELL_FRENZIED_BLOODTHIRST_VISUAL, true) +
    -- DoCastSelf(SPELL_CLEAR_ALL_STATUS_AILMENTS, true).
    creature:CastSpell(creature, SPELL_SHROUD_OF_SORROW, true)
    creature:CastSpell(creature, SPELL_FRENZIED_BLOODTHIRST_VISUAL, true)
    creature:CastSpell(creature, SPELL_CLEAR_ALL_STATUS_AILMENTS, true)
    schedule(guid, "berserk", 330000, function() onBerserk(creature, guid) end)
end

-- C++ Reset() timer machinery — cancel/re-arm on combat events
-- (the maiden convention).
local function lanaThelLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function lanaThelReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_blood_queen_lana_thel::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function lanaThelKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_blood_queen_lana_thel::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function lanaThelJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 1, lanaThelJustEngagedWith)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 2, lanaThelLeaveCombat)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 23, lanaThelReset)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 3, lanaThelKilledUnit)
RegisterCreatureEvent(ENTRY_BLOOD_QUEEN_LANA_THEL, 4, lanaThelJustDied)
