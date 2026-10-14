-- Blood Prince Council (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_blood_prince_council.cpp
-- (1369 lines incl. license; 8 CreatureScripts
-- (boss_blood_council_controller (BossAI);
-- boss_prince_keleseth_icc / boss_prince_taldaram_icc /
-- boss_prince_valanar_icc (BloodPrincesBossAI : BossAI);
-- npc_blood_queen_lana_thel (PassiveAI); npc_ball_of_flame
-- (ScriptedAI); npc_kinetic_bomb (ScriptedAI); npc_dark_nucleus
-- (ScriptedAI)) + 6 SpellScripts + 5 AuraScripts + 1
-- AreaTriggerScript (at_blood_prince_council_start_intro); all
-- registered from inside AddSC_boss_blood_prince_council();
-- loader decl 178 / call 373 per northrend_script_loader.cpp — the
-- EIGHTH group of the "// Icecrown Citadel" block in
-- AddNorthrendScripts(), immediately after
-- AddSC_boss_professor_putricide() (call 372); the call after it is
-- AddSC_boss_blood_queen_lana_thel() (call 374) — verified from the
-- loader during the Oct-13 Talk-only port.
-- Entries: 37972 Prince Keleseth (icecrown_citadel.h NPC_PRINCE_KELESETH,
-- line 245), 37973 Prince Taldaram (NPC_PRINCE_TALDARAM, line 246),
-- 37970 Prince Valanar (NPC_PRINCE_VALANAR, line 247);
-- instance_icecrown_citadel.cpp OnCreatureCreate binds all three
-- (case NPC_PRINCE_KELESETH line 255; DATA_PRINCE_* cases line 789) —
-- entry-verifiable; Go registration already exists
-- (engine/world/boss_ai.go, eighth ICC group):
-- RegisterLuaBoss("boss_prince_keleseth_icc", 37972),
-- RegisterLuaBoss("boss_prince_taldaram_icc", 37973),
-- RegisterLuaBoss("boss_prince_valanar_icc", 37970) — no new
-- registration needed. The CreatureScript ScriptName bindings are
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_blood_prince_council" hits
-- boss_blood_prince_council.cpp (+ the loader decl/call lines) only;
-- this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction.
--
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent ({min,max} table =
-- randtime range), GUID-keyed, cancelled on events 2/4/23; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
--
-- DOCTRINE RETRACTION (2026-10-13): the earlier port of this file
-- ported Talk arms only under the "no-timer-bridge" doctrine. That
-- doctrine is disproved and stale — CreateLuaEvent is a real registered
-- bridge (engine/scripting/runtime.go:798), and timer-driven cast arms
-- port on the maiden_of_virtue / moroes / curator / nightbane convention
-- (see boss_falric.lua for the exact idiom): timers table keyed by
-- creature GUID, cancelled on Eluna events 2 (leave combat), 4 (death),
-- 23 (reset); CastSpell(nil, spellId) = victim-cast (bridge falls back
-- to TargetGUID); CastSpell(creature, spellId) = self-cast (the
-- DoCastAOE convention, boss_ai.go DoCastAOE); {min,max} delay =
-- randtime range; TRIGGERED casts pass true as the third CastSpell arg.
-- None of the ported arms here is a TRIGGERED cast (every ported C++
-- DoCast is 1-2 arg, non-triggered). _isEmpowered state is NOT modeled:
-- it flips only via the controller's unbridged invocation rotation
-- (SpellHit -> ACTION_CAST_INVOCATION and the controller's cross-AI
-- ACTION_REMOVE_INVOCATION DoAction), so every empowered-variant arm
-- stays out (the gothik precedent).
--
-- PORTED ARMS (C++-exact for all modeled arms):
-- BloodPrincesBossAI::JustEngagedWith (:458-462) — the scheduler seeds
-- only (SetCombatPulseDelay/setActive/DoZoneInCombat have no bridge):
-- keleseth: EVENT_BERSERK 600000ms (:622), EVENT_SHADOW_RESONANCE
-- {10000,15000}ms (:623), EVENT_SHADOW_LANCE 2000ms (:624);
-- taldaram: EVENT_BERSERK 600000ms (:689), EVENT_GLITTERING_SPARKS
-- {12000,15000}ms (:690), EVENT_CONJURE_FLAME 20000ms (:691);
-- valanar: EVENT_BERSERK 600000ms (:763).
-- keleseth EVENT_BERSERK (:658-661) — DoCastSelf(SPELL_BERSERK 50334)
-- non-triggered + Talk(EMOTE_KELESETH_BERSERK 4), cast before Talk
-- (C++ order), one-shot.
-- keleseth EVENT_SHADOW_RESONANCE (:662-666) —
-- Talk(SAY_KELESETH_SPECIAL 2) BEFORE
-- DoCastSelf(SPELL_SHADOW_RESONANCE 71943) (C++ order), non-triggered;
-- re-arm {10000,15000}ms.
-- keleseth EVENT_SHADOW_LANCE (:667-670) —
-- DoCastVictim(SPELL_SHADOW_LANCE 71405) = victim-cast; re-arm 2000ms.
-- taldaram EVENT_BERSERK (:724-727) — DoCastSelf(SPELL_BERSERK 50334) +
-- Talk(EMOTE_TALDARAM_BERSERK 5), cast before Talk (C++ order),
-- one-shot.
-- taldaram EVENT_GLITTERING_SPARKS (:728-731) —
-- DoCastVictim(SPELL_GLITTERING_SPARKS 71806) = victim-cast;
-- re-arm {15000,50000}ms.
-- taldaram EVENT_CONJURE_FLAME (:732-744) —
-- DoCastSelf(SPELL_CONJURE_FLAME 71718) non-triggered +
-- Talk(SAY_TALDARAM_SPECIAL 2) AFTER the cast (C++ order);
-- re-arm {20000,30000}ms.
-- valanar EVENT_BERSERK (:814-817) — DoCastSelf(SPELL_BERSERK 50334) +
-- Talk(SAY_VALANAR_BERSERK 5), cast before Talk (C++ order), one-shot.
-- BloodPrincesBossAI::KilledUnit (:505-521) — Talk(kill text)
-- victim->GetTypeId() == TYPEID_PLAYER gated, entry-switched (event 3,
-- the razuvious player-gated variant precedent): keleseth
-- SAY_KELESETH_KILL 3, taldaram SAY_TALDARAM_KILL 4, valanar
-- SAY_VALANAR_KILL 4.
-- BloodPrincesBossAI::JustDied (:469-486) — Talk(death text)
-- entry-switched (event 4, the sjonnir JustDied-Talk precedent):
-- keleseth SAY_KELESETH_DEATH 5, taldaram EMOTE_TALDARAM_DEATH 6,
-- valanar SAY_VALANAR_DEATH 6.
--
-- DOCUMENTED-ONLY (no registration beyond the three prince entries):
-- boss_blood_council_controller — zero own Talk lines; its
-- JustEngagedWith (:254-292, CheckRequiredBosses / DoZoneInCombat /
-- SetBossState / DoCastSelf(SPELL_INVOCATION_OF_BLOOD_VALANAR 70952) /
-- the roll_chance_i(50) _invocationOrder build / EVENT_INVOCATION_OF_BLOOD
-- seed :289) has no instance bridge; UpdateAI EVENT_INVOCATION_OF_BLOOD
-- (:378-398) cross-AI DoAction + Talk(textId) on the rotated prince is
-- the invocation rotation machine — no bridge; NOT registered (the
-- bronjahm npc_corrupted_soul_fragment precedent).
-- BloodPrincesBossAI::DoAction (:549-580) — Talk(SelectInvocationSay())
-- + DoCastSelf(SPELL_INVOCATION_VISUAL_ACTIVE 71596, true) ride
-- ACTION_CAST_INVOCATION, which arrives via SpellHit (:535-539) from the
-- controller's unbridged invocation cast (no bridge); the
-- ACTION_REMOVE_INVOCATION health/aura reset rides the controller's
-- cross-AI DoAction (no bridge).
-- BloodPrincesBossAI::DamageTaken (:494-501) — non-empowered damage
-- nullification (damage = 0 unless _isEmpowered) is gated on the
-- unbridged invocation machine — unmodeled (event 9 is read-only in
-- the bridge); all three princes take full damage in the port
-- (deviation, documented below).
-- keleseth "// does not melee" (:676) — melee is engine-driven in Go
-- (deviation, documented below).
-- keleseth EVENT_SHADOW_LANCE empowered variant
-- DoCastVictim(SPELL_EMPOWERED_SHADOW_LANCE 71815) — _isEmpowered-gated
-- (modeled as base variant 71405 always; deviation, documented below).
-- taldaram EVENT_CONJURE_FLAME empowered variant
-- DoCastSelf(SPELL_CONJURE_EMPOWERED_FLAME 72040) re-arm
-- {15000,25000}ms (:735-737) — _isEmpowered-gated, out (modeled as
-- base 71718 with the base re-arm {20000,30000}ms).
-- valanar EVENT_KINETIC_BOMB (:818-824) — SelectTarget(Random, 1, 0.0f,
-- true) DoCast(target, SPELL_KINETIC_BOMB_TARGET 72053) +
-- Talk(SAY_VALANAR_SPECIAL 2) — random-target SelectTarget cast, out.
-- valanar EVENT_SHOCK_VORTEX (:825-838) — empowered leg
-- DoCastSelf(SPELL_EMPOWERED_SHOCK_VORTEX 72039) +
-- Talk(EMOTE_VALANAR_SHOCK_VORTEX 3) is _isEmpowered-gated; the base
-- leg DoCast(target, SPELL_SHOCK_VORTEX 72037) is random-target
-- SelectTarget — both out.
-- keleseth/taldaram/valanar JustSummoned (:634-642, :697-710,
-- :770-789) — summon reposition / random-target summon + chase /
-- movement-and-cast machines — no bridge, out.
-- npc_blood_queen_lana_thel — Talk(SAY_INTRO_1 0) rides
-- ACTION_START_INTRO (:866-880, no-DoAction bridge);
-- Talk(SAY_INTRO_2 1) rides EVENT_INTRO_1 (:896-905, intro scheduler
-- leg, no-timer-bridge) — NOT registered.
-- npc_ball_of_flame / npc_kinetic_bomb / npc_dark_nucleus — zero Talk
-- lines; Reset one-shot aura casts (:919-924, :987-988, :1047) are
-- aura/visual setup, not timer arms; the explode/despawn/chase legs
-- ride movement informs and scheduler DoActions — no bridge, NOT
-- registered.
-- UpdateAI UNIT_STATE_CASTING gates (:371-372, :652-653, :717-718,
-- :798-799) — no unit-state bridge; timers fire unconditionally (the
-- maiden precedent).
-- Heroic-gated engage casts: keleseth :628-629
-- DoCastSelf(SPELL_SHADOW_PRISON 72998, true) +
-- DoCastSelf(SPELL_SHADOW_PRISON_DUMMY 73001); taldaram :693 and
-- valanar :767 DoCastSelf(SPELL_SHADOW_PRISON 72998, true) — no
-- IsHeroic bridge (the rotface heroic-gated precedent).
-- The 6 spell scripts join the no-SpellScript-bridge queue, the 5 aura
-- scripts the no-AuraScript-bridge queue, and
-- at_blood_prince_council_start_intro the no-AreaTrigger bridge queue
-- (the boss_anubrekhan at_anubrekhan_entrance precedent).
--
-- DEVIATIONS FROM C++ (documented; every modeled value C++-exact):
-- _isEmpowered never flips (invocation machine unbridged): keleseth
-- shadow lance always casts the base 71405 (never empowered 71815);
-- taldaram conjure flame always casts base 71718 with the base re-arm
-- {20000,30000}ms (never empowered 72040 / {15000,25000}ms).
-- BloodPrincesBossAI::DamageTaken's non-empowered nullification is
-- unmodeled — all three princes take full damage in the port.
-- keleseth's C++ "// does not melee" is unmodeled — Go melees.

local ENTRY_KELESETH = 37972
local ENTRY_TALDARAM = 37973
local ENTRY_VALANAR = 37970

-- Texts enum: SAY_KELESETH_SPECIAL = 2, EMOTE_KELESETH_BERSERK = 4,
-- EMOTE_TALDARAM_BERSERK = 5, SAY_TALDARAM_SPECIAL = 2,
-- SAY_VALANAR_BERSERK = 5.
local SAY_KELESETH_SPECIAL = 2
local EMOTE_KELESETH_BERSERK = 4
local EMOTE_TALDARAM_BERSERK = 5
local SAY_TALDARAM_SPECIAL = 2
local SAY_VALANAR_BERSERK = 5

-- Spells enum: SPELL_BERSERK = 50334 (ScriptedAI/BossAI; verified in
-- this tree), SPELL_SHADOW_RESONANCE = 71943,
-- SPELL_SHADOW_LANCE = 71405, SPELL_GLITTERING_SPARKS = 71806,
-- SPELL_CONJURE_FLAME = 71718.
local SPELL_BERSERK = 50334
local SPELL_SHADOW_RESONANCE = 71943
local SPELL_SHADOW_LANCE = 71405
local SPELL_GLITTERING_SPARKS = 71806
local SPELL_CONJURE_FLAME = 71718

-- entry-switched kill texts (C++ BloodPrincesBossAI::KilledUnit :505):
-- keleseth SAY_KELESETH_KILL = 3, taldaram SAY_TALDARAM_KILL = 4,
-- valanar SAY_VALANAR_KILL = 4.
local KILL_TEXT = {
    [ENTRY_KELESETH] = 3,
    [ENTRY_TALDARAM] = 4,
    [ENTRY_VALANAR] = 4,
}

-- entry-switched death texts (C++ BloodPrincesBossAI::JustDied :469):
-- keleseth SAY_KELESETH_DEATH = 5, taldaram EMOTE_TALDARAM_DEATH = 6,
-- valanar SAY_VALANAR_DEATH = 6.
local DEATH_TEXT = {
    [ENTRY_KELESETH] = 5,
    [ENTRY_TALDARAM] = 6,
    [ENTRY_VALANAR] = 6,
}

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

-- bronjahm-style: re-scheduling a key replaces the pending timer, so
-- each named arm has exactly one live timer at a time.
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

-- C++ :658-661 keleseth EVENT_BERSERK: DoCastSelf(SPELL_BERSERK)
-- non-triggered + Talk(EMOTE_KELESETH_BERSERK), cast before Talk (C++
-- order), one-shot (no repeat).
local function onKelesethBerserk(creature)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_KELESETH_BERSERK)
end

-- C++ :662-666 keleseth EVENT_SHADOW_RESONANCE:
-- Talk(SAY_KELESETH_SPECIAL) BEFORE DoCastSelf(SPELL_SHADOW_RESONANCE)
-- (C++ order), non-triggered; re-arm {10000,15000}ms.
local function onKelesethResonance(creature, guid)
    creature:Talk(SAY_KELESETH_SPECIAL)
    creature:CastSpell(creature, SPELL_SHADOW_RESONANCE)
    schedule(guid, "resonance", {10000, 15000}, function() onKelesethResonance(creature, guid) end)
end

-- C++ :667-670 keleseth EVENT_SHADOW_LANCE:
-- _isEmpowered ? DoCastVictim(SPELL_EMPOWERED_SHADOW_LANCE) :
-- DoCastVictim(SPELL_SHADOW_LANCE) = victim-cast; re-arm 2000ms. The
-- empowered variant (71815) is _isEmpowered-gated (unbridged machine)
-- — modeled as the base variant always (documented deviation).
local function onKelesethLance(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_LANCE)
    schedule(guid, "lance", 2000, function() onKelesethLance(creature, guid) end)
end

-- C++ :724-727 taldaram EVENT_BERSERK: DoCastSelf(SPELL_BERSERK) +
-- Talk(EMOTE_TALDARAM_BERSERK), cast before Talk (C++ order),
-- one-shot.
local function onTaldaramBerserk(creature)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(EMOTE_TALDARAM_BERSERK)
end

-- C++ :728-731 taldaram EVENT_GLITTERING_SPARKS:
-- DoCastVictim(SPELL_GLITTERING_SPARKS) = victim-cast;
-- re-arm {15000,50000}ms.
local function onTaldaramSparks(creature, guid)
    creature:CastSpell(nil, SPELL_GLITTERING_SPARKS)
    schedule(guid, "sparks", {15000, 50000}, function() onTaldaramSparks(creature, guid) end)
end

-- C++ :732-744 taldaram EVENT_CONJURE_FLAME: DoCastSelf(
-- SPELL_CONJURE_FLAME) non-triggered + Talk(SAY_TALDARAM_SPECIAL)
-- AFTER the cast (C++ order); re-arm {20000,30000}ms. The empowered
-- variant DoCastSelf(SPELL_CONJURE_EMPOWERED_FLAME 72040) re-arm
-- {15000,25000}ms is _isEmpowered-gated (unbridged machine) — modeled
-- as the base variant always (documented deviation).
local function onTaldaramConjure(creature, guid)
    creature:CastSpell(creature, SPELL_CONJURE_FLAME)
    creature:Talk(SAY_TALDARAM_SPECIAL)
    schedule(guid, "conjure", {20000, 30000}, function() onTaldaramConjure(creature, guid) end)
end

-- C++ :814-817 valanar EVENT_BERSERK: DoCastSelf(SPELL_BERSERK) +
-- Talk(SAY_VALANAR_BERSERK), cast before Talk (C++ order), one-shot.
local function onValanarBerserk(creature)
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_VALANAR_BERSERK)
end

-- C++ boss_prince_keleseth_icc::ScheduleEvents :622-624 —
-- EVENT_BERSERK 600000ms, EVENT_SHADOW_RESONANCE {10000,15000}ms,
-- EVENT_SHADOW_LANCE 2000ms (the IsHeroic shadow-prison legs :628-629
-- have no IsHeroic bridge). JustEngagedWith (:458-462) otherwise has
-- no bridgeable leg.
local function kelesethEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 600000, function() onKelesethBerserk(creature) end)
    schedule(guid, "resonance", {10000, 15000}, function() onKelesethResonance(creature, guid) end)
    schedule(guid, "lance", 2000, function() onKelesethLance(creature, guid) end)
end

-- C++ boss_prince_taldaram_icc::ScheduleEvents :689-691 —
-- EVENT_BERSERK 600000ms, EVENT_GLITTERING_SPARKS {12000,15000}ms,
-- EVENT_CONJURE_FLAME 20000ms (the IsHeroic shadow-prison leg :693 has
-- no IsHeroic bridge).
local function taldaramEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 600000, function() onTaldaramBerserk(creature) end)
    schedule(guid, "sparks", {12000, 15000}, function() onTaldaramSparks(creature, guid) end)
    schedule(guid, "conjure", 20000, function() onTaldaramConjure(creature, guid) end)
end

-- C++ boss_prince_valanar_icc::ScheduleEvents :763 —
-- EVENT_BERSERK 600000ms (EVENT_KINETIC_BOMB and EVENT_SHOCK_VORTEX
-- are random-target/_isEmpowered-gated, out; the IsHeroic shadow-prison
-- leg :767 has no IsHeroic bridge).
local function valanarEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 600000, function() onValanarBerserk(creature) end)
end

-- C++ BloodPrincesBossAI::KilledUnit :505-521:
-- if (victim->GetTypeId() == TYPEID_PLAYER) switch (me->GetEntry())
-- Talk(kill text) — the razuvious player-gated variant precedent.
local function princesKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(KILL_TEXT[creature:GetEntry()])
    end
end

-- C++ BloodPrincesBossAI::JustDied :469-486:
-- switch (me->GetEntry()) Talk(death text) — the sjonnir
-- JustDied-Talk precedent.
local function princesJustDied(event, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(DEATH_TEXT[creature:GetEntry()])
end

local function princesLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function princesReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KELESETH, 1, kelesethEnterCombat)
RegisterCreatureEvent(ENTRY_TALDARAM, 1, taldaramEnterCombat)
RegisterCreatureEvent(ENTRY_VALANAR, 1, valanarEnterCombat)

RegisterCreatureEvent(ENTRY_KELESETH, 2, princesLeaveCombat)
RegisterCreatureEvent(ENTRY_TALDARAM, 2, princesLeaveCombat)
RegisterCreatureEvent(ENTRY_VALANAR, 2, princesLeaveCombat)

RegisterCreatureEvent(ENTRY_KELESETH, 3, princesKilledUnit)
RegisterCreatureEvent(ENTRY_TALDARAM, 3, princesKilledUnit)
RegisterCreatureEvent(ENTRY_VALANAR, 3, princesKilledUnit)

RegisterCreatureEvent(ENTRY_KELESETH, 4, princesJustDied)
RegisterCreatureEvent(ENTRY_TALDARAM, 4, princesJustDied)
RegisterCreatureEvent(ENTRY_VALANAR, 4, princesJustDied)

RegisterCreatureEvent(ENTRY_KELESETH, 23, princesReset)
RegisterCreatureEvent(ENTRY_TALDARAM, 23, princesReset)
RegisterCreatureEvent(ENTRY_VALANAR, 23, princesReset)
