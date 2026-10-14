-- Sindragosa (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sindragosa.cpp
-- (1556 lines incl. license; 5 CreatureScripts
-- (boss_sindragosa (BossAI, DATA_SINDRAGOSA = 11), npc_ice_tomb
-- (ScriptedAI), npc_spinestalker (ScriptedAI), npc_rimefang_icc
-- (ScriptedAI), npc_sindragosa_trash (ScriptedAI, shared AI for
-- NPC_FROSTWARDEN_HANDLER / NPC_FROSTWING_WHELP)) +
-- 13 Sindragosa-specific SpellScript/AuraScript legs
-- (spell_sindragosa_s_fury, spell_sindragosa_unchained_magic,
-- spell_sindragosa_frost_breath, spell_sindragosa_instability,
-- spell_sindragosa_frost_beacon, spell_sindragosa_ice_tomb_trap,
-- spell_sindragosa_icy_grip, spell_sindragosa_mystic_buffet,
-- spell_rimefang_icy_blast, spell_frostwarden_handler_order_whelp,
-- spell_frostwarden_handler_focus_fire,
-- spell_frostwarden_handler_focus_fire_aura (AuraScript),
-- spell_sindragosa_ice_tomb_target) + 2 generic
-- RegisterSpellScriptWithArgs spell_trigger_spell_from_caster legs
-- + 1 AreaTriggerScript
-- (at_sindragosa_lair) + 1 AchievementCriteriaScript
-- (achievement_all_you_can_eat); all registered from inside
-- AddSC_boss_sindragosa(); loader decl 182 / call 377 per
-- northrend_script_loader.cpp — the TWELFTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_valithria_dreamwalker() (call 376) — verified
-- from the loader this run; the checkpoint sequence
-- (valithria_dreamwalker -> sindragosa) is followed).
-- Entry: 36853 Sindragosa (icecrown_citadel.h NPC_SINDRAGOSA, line
-- 304; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_SINDRAGOSA, line 300; also summoned via instance
-- SummonCreature(NPC_SINDRAGOSA, SindragosaSpawnPos), line 492) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the RegisterIcecrownCitadelCreatureAI
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_sindragosa" hits boss_sindragosa.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_sindragosa JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 —
-- the auriaya engage-port precedent; the instance
-- CheckRequiredBosses gate / EVADE_REASON_SEQUENCE_BREAK leg /
-- DoCastSpellOnPlayers LIGHT_S_HAMMER_TELEPORT leg / DoCastSelf
-- FROST_AURA+PERMAEATING_CHILL legs / SetBossState(IN_PROGRESS) /
-- SetCombatPulseDelay / setActive / SetFarVisible / DoZoneInCombat
-- legs have no bridges).
-- boss_sindragosa KilledUnit — Talk(SAY_KILL 8) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_sindragosa JustDied — Talk(SAY_DEATH 10) (event 4 — the
-- sjonnir JustDied-Talk precedent; the DoCastAOE
-- FROST_INFUSION_CREDIT heroic leg has no-cast bridge).
-- DOCUMENTED-ONLY (in this header; only entry 36853 registered):
-- PORTED (2026-10-13): JustEngagedWith — DoCastSelf(
-- SPELL_FROST_AURA 70084) + DoCastSelf(SPELL_PERMAEATING_CHILL 70109)
-- non-triggered (:277-278, maiden engage-cast convention);
-- EVENT_BERSERK — single-fire 10min — Talk(EMOTE_BERSERK_RAID 11) +
-- Talk(SAY_BERSERK 9) + DoCastSelf(SPELL_BERSERK 26662) non-triggered
-- (:242/:458-460), bare timer across all phases.
-- PORTED (2026-10-13 retraction re-audit): Reset() —
-- DoCastSelf(SPELL_TANK_MARKER 71039, triggered) (:241);
-- DamageTaken 35% latch — Talk(SAY_PHASE_2 6) +
-- DoCastSelf(SPELL_MYSTIC_BUFFET 70128, triggered) via a 1s
-- one-shot timer (the EVENT_THIRD_PHASE_CHECK leg :403-410 /
-- :548-553; CancelEvent(EVENT_AIR_PHASE) is moot — no air-phase
-- timers exist in this port; RescheduleEvent(EVENT_ICY_GRIP) rides
-- the unbridged icy-grip leg); EVENT_ICE_TOMB — CastSpell(nil,
-- SPELL_ICE_TOMB_TARGET 69712, triggered) init {7s,10s} / re-arm
-- {16s,23s} (:515-518/:551 — the air-phase retry loop is
-- vacuously satisfied with no air-phase machine; the SPELLVALUE_
-- MAX_TARGETS(1) spell-mod leg has no bridge, default cast
-- faithful).
-- boss_sindragosa UpdateAI — the land/air phase machine gates cleave /
-- tail-smash / frost-breath / icy-grip / unchained-magic /
-- frost-bomb legs (phase-machine rule — porting bare timers would
-- fabricate air-phase casts) — all unbridged.
-- SAY_RESPITE_FOR_A_TORMENTED_SOUL 4 is declared in the text enum
-- but never referenced by a Talk() call — dead text evidence.
-- boss_sindragosa DoAction — ACTION_START_FROSTWYRM intro machine
-- (no-DoAction bridge); JustReachedHome SetBossState FAIL leg
-- (no instance bridge, no event-24 port precedent).
-- spell_sindragosa_ice_tomb_target::HandleSindragosaTalk —
-- creatureCaster->AI()->Talk(EMOTE_WARN_FROZEN_ORB 7, GetHitUnit())
-- is cross-AI Talk (no cross-AI-Talk bridge).
-- npc_ice_tomb (36980) / npc_spinestalker (37534) /
-- npc_rimefang_icc (37533) / npc_sindragosa_trash (37531
-- NPC_FROSTWARDEN_HANDLER, 37532 NPC_FROSTWING_WHELP) — zero Talk
-- lines — NOT registered (the bronjahm npc_corrupted_soul_fragment
-- precedent); ice tomb's SetGUID / DoAction
-- ACTION_TRIGGER_ASPHYXIATION / GUID-bookkeeping legs, spinestalker
-- / rimefang intro DoAction machines, and the trash Reset /
-- JustAppeared / SetData instance legs ride bridges that do not
-- exist.
-- at_sindragosa_lair joins the no-AreaTrigger bridge queue (the
-- boss_anubrekhan at_anubrekhan_entrance precedent);
-- achievement_all_you_can_eat joins the
-- no-achievement-criteria-bridge queue (the lana'thel achievement
-- precedent); the remaining 12 Sindragosa-specific SpellScript /
-- AuraScript legs + the 2 generic spell_trigger_spell_from_caster
-- with-args legs join the no-SpellScript-bridge /
-- no-AuraScript-bridge queues (the boss_moragg optic-link
-- precedent).
-- All 11 Talk() calls in the file accounted for (11 =
-- SAY_AGGRO 0 (1) + SAY_DEATH 10 (1) + SAY_KILL 8 (1) +
-- EMOTE_BERSERK_RAID 11 (1, scheduler) + SAY_BERSERK 9 (1,
-- scheduler) + SAY_UNCHAINED_MAGIC 1 (1, scheduler) +
-- EMOTE_WARN_BLISTERING_COLD 2 (1, scheduler) +
-- SAY_BLISTERING_COLD 3 (1, scheduler) + SAY_AIR_PHASE 5 (1,
-- scheduler) + SAY_PHASE_2 6 (1, scheduler) +
-- EMOTE_WARN_FROZEN_ORB 7 (1, cross-AI spell leg)).

local ENTRY_SINDRAGOSA = 36853

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_DEATH = 10
local SAY_BERSERK = 9
local EMOTE_BERSERK_RAID = 11
local SAY_PHASE_2 = 6

local SPELL_TANK_MARKER = 71039
local SPELL_MYSTIC_BUFFET = 70128
local SPELL_ICE_TOMB_TARGET = 69712

local sindragosaThirdPhase = {}

local SPELL_FROST_AURA = 70084
local SPELL_PERMAEATING_CHILL = 70109
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

-- C++ EVENT_BERSERK: 10min single-fire — Talk(EMOTE_BERSERK_RAID) +
-- Talk(SAY_BERSERK) + DoCastSelf(SPELL_BERSERK) non-triggered
-- (:242/:458-460), bare timer across all phases.
local function onBerserk(creature, guid)
    creature:Talk(EMOTE_BERSERK_RAID)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ EVENT_ICE_TOMB: CastSpell(nullptr, SPELL_ICE_TOMB_TARGET,
-- TRIGGERED_FULL_MASK) — victim-less cast (:515-518), re-arm
-- {16s,23s}.
local function onIceTomb(creature, guid)
    creature:CastSpell(nil, SPELL_ICE_TOMB_TARGET, true)
    schedule(guid, "ice_tomb", {16000, 23000}, function() onIceTomb(creature, guid) end)
end

-- C++ EVENT_THIRD_PHASE_CHECK: Talk(SAY_PHASE_2) +
-- DoCastSelf(SPELL_MYSTIC_BUFFET, true); seeds EVENT_ICE_TOMB
-- {7s,10s} — the _isInAirPhase leg is vacuously true (no air-phase
-- machine in this port).
local function onThirdPhaseCheck(creature, guid)
    creature:Talk(SAY_PHASE_2)
    creature:CastSpell(creature, SPELL_MYSTIC_BUFFET, true)
    schedule(guid, "ice_tomb", {7000, 10000}, function() onIceTomb(creature, guid) end)
end

-- C++ boss_sindragosa::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch (:269-275) has no bridge.
local function sindragosaJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ :277-278 — DoCastSelf(SPELL_FROST_AURA) +
    -- DoCastSelf(SPELL_PERMAEATING_CHILL), non-triggered.
    creature:CastSpell(creature, SPELL_FROST_AURA)
    creature:CastSpell(creature, SPELL_PERMAEATING_CHILL)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
end

-- C++ Reset() _Reset() timer machinery — cancel/re-arm on combat
-- events (the maiden convention).
local function sindragosaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function sindragosaReset(event, creature)
    cancelTimers(creature:GetGUID())
    sindragosaThirdPhase[creature:GetGUID()] = nil
    -- C++ Reset(): DoCastSelf(SPELL_TANK_MARKER, true) (:241),
    -- unconditional one-shot.
    creature:CastSpell(creature, SPELL_TANK_MARKER, true)
end

-- C++ boss_sindragosa::DamageTaken: !_isThirdPhase && !HealthAbovePct(35)
-- — single-flip latch (gothik precedent); schedules
-- EVENT_THIRD_PHASE_CHECK 1s (:403-410).
local function sindragosaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if not sindragosaThirdPhase[guid] and creature:GetHealthPct() < 35 then
        sindragosaThirdPhase[guid] = true
        schedule(guid, "third_phase_check", 1000, function() onThirdPhaseCheck(creature, guid) end)
    end
    return true, damage
end

-- C++ boss_sindragosa::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function sindragosaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_sindragosa::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function sindragosaJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_SINDRAGOSA, 1, sindragosaJustEngagedWith)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 2, sindragosaLeaveCombat)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 3, sindragosaKilledUnit)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 4, sindragosaJustDied)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 9, sindragosaDamageTaken)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 23, sindragosaReset)
