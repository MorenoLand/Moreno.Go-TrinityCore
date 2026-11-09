-- Varos Cloudstrider (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_varos.cpp
-- (boss_varos (CreatureScript) via GetOculusAI<boss_varosAI>
-- (BossAI, DATA_VAROS); npc_azure_ring_captain (CreatureScript) via
-- GetOculusAI<npc_azure_ring_captainAI> (ScriptedAI);
-- spell_varos_centrifuge_shield / spell_varos_energize_core_area_enemy /
-- spell_varos_energize_core_area_entry (SpellScriptLoader, registered
-- from inside AddSC_boss_varos, no separate loader lines);
-- AddSC_boss_varos() at end registers all five; loader decl 86 /
-- call 281 per northrend_script_loader.cpp — the THIRD group of the
-- "// The Nexus: The Oculus" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_urom() (decl 85 / call 280); the
-- call after it is AddSC_boss_eregos() — loader order confirmed
-- this run; the checkpoint sequence (urom -> varos) is followed).
-- Entry: 27447 Varos Cloudstrider (oculus.h NPC_VAROS, line 42;
-- DATA_VAROS = 1 (line 32); instance_oculus.cpp OnCreatureCreate
-- binds case NPC_VAROS (line 62) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual. The
-- instance GetGuidData case DATA_VAROS (lines 190 / 245) is the
-- same entry — no instance bridge, documented only.
-- Sole-source verified: whole-server-tree grep for "boss_varos"
-- hits boss_varos.cpp + the loader decl/call lines only; grep for
-- "npc_azure_ring_captain" / "spell_varos_centrifuge_shield" /
-- "spell_varos_energize_core_area_enemy" /
-- "spell_varos_energize_core_area_entry" hits boss_varos.cpp only
-- (registered from inside AddSC_boss_varos, no separate loader
-- lines); zero sql/ hits. No varos lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith base arms are PORTED in varosEnterCombat
-- (SetBossState(DATA_VAROS, IN_PROGRESS) + pulse + me-form DoZone,
-- base-first per C++; instance/setActive/ScheduleTasks legs out).
-- SetBossState PORTED 2026-11-05 (creature:SetBossState bridged,
-- lua_creature_events.go:5917; SetBossState census).
-- JustDied — Talk(SAY_DEATH 3) (event 4; the _JustDied
-- SetBossState(DONE) leg is PORTED in varosDied (base-first,
-- 2026-11-05); the DoCast(me, SPELL_DEATH_SPELL 50415, true) leg
-- STAYS OUT — the urom verdict: the triggered cast bridge exists,
-- but the engine's creature-cast path has no SPELL_EFFECT_SUMMON
-- (28) delivery leg).
-- Unmodeled (no bridges — documented, not wired):
-- InitializeAI — DoCast(me, SPELL_CENTRIFUGE_SHIELD 50053) gated
-- on instance->GetBossState(DATA_DRAKOS) != DONE — PORTED
-- 2026-11-05 in varosSpawn (event 5; the GATE is bridged,
-- creature:GetBossState, and the removal path is bridged too:
-- last construct death -> Varos RemoveAllAuras in
-- npc_centrifuge_construct.lua, so the shield can no longer
-- strand Varos passive + immune).
-- Reset — _Reset() SetBossState(NOT_STARTED, != DONE-gated)
-- PORTED 2026-11-05 in varosReset (event 23; _Reset() is first in
-- C++ Reset, so the state leg precedes the timer cancel) + the
-- three ScheduleEvent legs (AMPLIFY_MAGIC 20s/25s,
-- ENERGIZE_CORES_VISUAL 5s, CALL_AZURE 15s/30s) +
-- Initialize() member state — the timer bridge is CreateLuaEvent
-- (the stale "no timer-event bridge" claim is retracted); the
-- ports seed from OnEnterCombat(1) and cancel on 2/4/23 per the
-- krystallus/bjarngrim/halion convention. The out-of-combat Reset
-- seed is unmodeled (maiden precedent).
-- UpdateAI — the energize-cores / azure-captain / amplify-magic
-- event machine (boss_varos.cpp:119-155), PORTED arms:
-- EVENT_AMPLIFY_MAGIC DoCastVictim(SPELL_CALL_AMPLIFY_MAGIC 51054)
-- non-triggered, 20s/25s init -> 17s/20s loop — PORTED as
-- CreateLuaEvent + CastSpell(nil, 51054) (GetVictim nil-guard not
-- needed: CastSpell(nil) resolves the victim in-engine, bjarngrim
-- lieutenant precedent).
-- EVENT_ENERGIZE_CORES_VISUAL DoCast(me,
-- SPELL_ENERGIZE_CORES_VISUAL 62136) non-triggered, 5s loop —
-- PORTED as CreateLuaEvent + CastSpell(creature, 62136). The
-- firstCoreEnergize / coreEnergizeOrientation latch is unmodeled:
-- its only consumer is the spell_varos_energize_core_area
-- SpellScripts’ orientation filter (no SpellScript bridge).
-- EVENT_CALL_AZURE Talk(SAY_AZURE 1) + Talk(SAY_AZURE_EMOTE 2),
-- 15s/30s init -> 20s/25s loop — PORTED on a CreateLuaEvent
-- timer; the DoCast(me, SPELL_CALL_AZURE_RING_CAPTAIN 51002) leg
-- STAYS OUT: SPELL_EFFECT_SEND_EVENT driving the azure-captain
-- summon machinery — no summon/event-send bridge.
-- CASTING gates PORTED 2026-11-04 (500ms IsCasting defer, mal_ganis
-- idiom): own UpdateAI pre-loop :114 + in-loop :153 gates cover all
-- 3 ported ticks (amplify/energize-visual/azure — defer precedes
-- cast / cast / Talk legs; the event stays queued, no re-arm
-- consumed). The stale "UNIT_STATE_CASTING gates have no bridge"
-- claim is retracted — IsCasting is live
-- (lua_creature_events.go:1950).
-- EVENT_ENERGIZE_CORES DoCast(me, SPELL_ENERGIZE_CORES 50785)
-- one-shot 4s after each VISUAL then CancelEvent — STAYS OUT:
-- the spell’s area targeting is SpellScript-filtered (the cone
-- filter above, no bridge) so a bare cast would hit the wrong
-- targets — shazzrah no-op-cast precedent; the 4s chain dies
-- with this stay-out. The UNIT_STATE_CASTING gates have no bridge
-- (documented deviation); DoMeleeAttackIfReady is engine-driven.
-- npc_azure_ring_captain — ENTRY-UNVERIFIABLE, documented-only
-- (the entry-unverifiable queue, eleventh script group after
-- npc_unstable_sphere; the local enum holds only
-- NPC_AZURE_RING_GUARDIAN 28236 (summoned by instance code) — no
-- NPC_AZURE_RING_CAPTAIN enum anywhere in the C++ tree and zero
-- ScriptName sql/ hits, so there is no C++-evidenced entry to
-- register): Reset — SetWalk(true) + MOVEMENTFLAG_FLYING +
-- SetReactState(REACT_AGGRESSIVE); SpellHitTarget — SPELL_ICE_BEAM
-- 49549 -> target CastSpell(SPELL_SUMMON_ARCANE_BEAM 51017, true)
-- + me->DespawnOrUnsummon() — NOTE: SpellHitTarget (event 15) IS
-- dispatched in the current runtime (runtime.go:201 /
-- boss_ai.go:2052); the leg stays out because 51017 is a summon with
-- no spell-effect delivery leg (urom verdict) and DespawnOrUnsummon
-- has no Lua binding, not because the event never fires; UpdateAI — DoMeleeAttackIfReady only; MovementInform
-- (POINT_MOTION_TYPE / ACTION_CALL_DRAGON_EVENT) — MoveIdle +
-- DoCast(target, SPELL_ICE_BEAM) (the target-GUID leg); DoAction(
-- ACTION_CALL_DRAGON_EVENT) — varos via instance GetGuidData(
-- DATA_VAROS) -> SelectTarget(Random) -> REACT_PASSIVE +
-- MovePoint(victim + 20.0f z) (the ACTION_CALL_DRAGON_EVENT = 1
-- oculus.h leg) — no instance / motion / react-state / despawn /
-- random-target bridges (event 15 fires; the SpellHit legs stay out
-- on the summon/despawn blockers above).
-- spell_varos_centrifuge_shield — AuraScript OnApply / OnRemove
-- (EFFECT_0 SPELL_AURA_DUMMY): PORTED (tick 204, RegisterAuraEvent
-- OnApply = 2 / OnRemove = 3, below). SetImmuneToAll twin is
-- SetFlag/RemoveFlag IMMUNE_TO_PC|IMMUNE_TO_NPC (Unit.cpp:8440-8448;
-- keepCombat=true, so no combat-end leg; the
-- ValidateAttackersAndOwnTarget leg is unmodeled).
-- spell_varos_energize_core_area_enemy / spell_varos_energize_core_
-- area_entry — SpellScript FilterTargets (TARGET_UNIT_SRC_AREA_
-- ENEMY / TARGET_UNIT_SRC_AREA_ENTRY): caster must be NPC_VAROS,
-- keeps only targets within 1.0f of the AI's
-- GetCoreEnergizeOrientation() — no SpellScript bridge (the
-- standing SpellScript-check-handlers gap); both join the
-- no-SpellScript-bridge queue.

local ENTRY_VAROS = 27447

local SAY_AGGRO = 0
local SAY_DEATH = 3
local SAY_AZURE = 1
local SAY_AZURE_EMOTE = 2

local SPELL_ENERGIZE_CORES_VISUAL = 62136
local SPELL_CALL_AMPLIFY_MAGIC = 51054

local DATA_VAROS = 1 -- oculus.h:32 (OCDataTypes)
local DATA_DRAKOS = 0 -- oculus.h:31 (OCDataTypes)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

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

-- C++ EVENT_AMPLIFY_MAGIC (:145-148): DoCastVictim(51054)
-- non-triggered, 20s/25s init -> 17s/20s loop.
local function amplifyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "amplify", 500, function()
            amplifyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CALL_AMPLIFY_MAGIC)
    schedule(guid, "amplify", {17000, 20000}, function()
        amplifyTick(creature, guid)
    end)
end

-- C++ EVENT_ENERGIZE_CORES_VISUAL (:125-136): DoCast(me, 62136)
-- non-triggered, 5s loop. The orientation latch is unmodeled
-- (SpellScript-only consumer); the 4s EVENT_ENERGIZE_CORES chain
-- dies with the 50785 stay-out.
local function energizeVisualTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "energizeVisual", 500, function()
            energizeVisualTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ENERGIZE_CORES_VISUAL)
    schedule(guid, "energizeVisual", 5000, function()
        energizeVisualTick(creature, guid)
    end)
end

-- C++ EVENT_CALL_AZURE (:138-143): Talk(SAY_AZURE) +
-- Talk(SAY_AZURE_EMOTE), 15s/30s init -> 20s/25s loop. The 51002
-- summon leg stays out (no summon/event-send bridge).
local function azureTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "azure", 500, function()
            azureTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_AZURE)
    creature:Talk(SAY_AZURE_EMOTE)
    schedule(guid, "azure", {20000, 25000}, function()
        azureTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith(who) first
-- (boss_varos.cpp:95-100) — SetBossState(IN_PROGRESS)
-- (ScriptedCreature.cpp:537) + pulse + DoZone, then Talk(SAY_AGGRO)
-- — the Talk arm is unconditional.
local function varosEnterCombat(event, creature, target)
    -- C++ JustEngagedWith base call first (boss_varos.cpp:95-100):
    -- BossAI::JustEngagedWith arms (ScriptedCreature.cpp:537-542) --
    -- SetBossState(IN_PROGRESS) + SetCombatPulseDelay(5) + me-form
    -- DoZoneInCombat, before Talk (C++ arm order);
    -- instance/setActive/ScheduleTasks legs out.
    creature:SetBossState(DATA_VAROS, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "amplify", {20000, 25000}, function()
        amplifyTick(creature, guid)
    end)
    schedule(guid, "energizeVisual", 5000, function()
        energizeVisualTick(creature, guid)
    end)
    schedule(guid, "azure", {15000, 30000}, function()
        azureTick(creature, guid)
    end)
end

local function varosCombatEnd(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_varos.cpp:88-97): _Reset() FIRST —
-- SetBossState(NOT_STARTED), != DONE-gated (ScriptedCreature.cpp:510),
-- then the ScheduleEvent legs + Initialize() (member state; the timer
-- legs seed on engage per the krystallus convention).
local function varosReset(event, creature)
    if creature:GetBossState(DATA_VAROS) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_VAROS, BOSS_STATE_NOT_STARTED)
    end
    cancelTimers(creature:GetGUIDLow())
end

-- C++ JustDied: _JustDied() first (boss_varos.cpp:158-162) —
-- SetBossState(DONE) (ScriptedCreature.cpp:519), then Talk(SAY_DEATH)
-- + DoCast(me, SPELL_DEATH_SPELL 50415, true); the DoCast leg stays
-- out (urom verdict: no SPELL_EFFECT_SUMMON delivery leg on the
-- engine's creature-cast path).
local function varosDied(event, creature, killer)
    creature:SetBossState(DATA_VAROS, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUIDLow())
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.varosDone = true
        end
    end
end

local SPELL_CENTRIFUGE_SHIELD = 50053 -- oculus.h:82

-- C++ InitializeAI (boss_varos.cpp:76-81): DoCast(me,
-- SPELL_CENTRIFUGE_SHIELD 50053) gated on
-- instance->GetBossState(DATA_DRAKOS) != DONE. PORTED 2026-11-05
-- on ev5 (ON_SPAWN, the InitializeAI twin — first spawn +
-- respawn, never on evade, matching C++; the C++ Reset calls
-- Initialize(), not InitializeAI). The removal path ports the
-- same tick in npc_centrifuge_construct.lua (last construct death
-- -> Varos RemoveAllAuras, instance_oculus.cpp:126-136), so the
-- shield can no longer strand Varos passive + immune; the
-- apply/remove legs were already ported below (RegisterAuraEvent).
local function varosSpawn(event, creature)
    if creature:GetBossState(DATA_DRAKOS) ~= BOSS_STATE_DONE then
        creature:CastSpell(creature, SPELL_CENTRIFUGE_SHIELD)
    end
end

RegisterCreatureEvent(ENTRY_VAROS, 1, varosEnterCombat)
RegisterCreatureEvent(ENTRY_VAROS, 2, varosCombatEnd)
RegisterCreatureEvent(ENTRY_VAROS, 4, varosDied)
RegisterCreatureEvent(ENTRY_VAROS, 5, varosSpawn)
RegisterCreatureEvent(ENTRY_VAROS, 23, varosReset)

local UNIT_FLAG_UNK_6 = 0x00000040 -- UnitDefines.h:130
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132
local UNIT_FLAG_IMMUNE_TO_NPC = 0x00000200 -- UnitDefines.h:133
local UNIT_FLAG_SWIMMING = 0x00008000 -- UnitDefines.h:139
local REACT_PASSIVE = 0 -- UnitDefines.h
local REACT_AGGRESSIVE = 2 -- UnitDefines.h

-- C++ spell_varos_centrifuge_shield::OnApply (boss_varos.cpp:280-292):
-- Load admits creature casters only; the caster must carry any of the
-- sniffed flags (C++ HasFlag is an any-bit read, same as the Go twin)
-- before going passive + SWIMMING|UNK_6 + immune to all (true, true).
local function centrifugeShieldApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if not caster:HasFlag(59, UNIT_FLAG_SWIMMING + UNIT_FLAG_IMMUNE_TO_NPC + UNIT_FLAG_IMMUNE_TO_PC + UNIT_FLAG_UNK_6) then
        return
    end
    caster:SetReactState(REACT_PASSIVE)
    caster:SetFlag(59, UNIT_FLAG_SWIMMING + UNIT_FLAG_UNK_6)
    caster:SetFlag(59, UNIT_FLAG_IMMUNE_TO_PC + UNIT_FLAG_IMMUNE_TO_NPC)
end

RegisterAuraEvent(SPELL_CENTRIFUGE_SHIELD, 2, centrifugeShieldApply)

-- C++ spell_varos_centrifuge_shield::OnRemove (boss_varos.cpp:294-302):
-- react aggressive, SWIMMING|UNK_6 cleared, full immunity cleared.
local function centrifugeShieldRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    caster:SetReactState(REACT_AGGRESSIVE)
    caster:RemoveFlag(59, UNIT_FLAG_SWIMMING + UNIT_FLAG_UNK_6)
    caster:RemoveFlag(59, UNIT_FLAG_IMMUNE_TO_PC + UNIT_FLAG_IMMUNE_TO_NPC)
end

RegisterAuraEvent(SPELL_CENTRIFUGE_SHIELD, 3, centrifugeShieldRemove)
