-- Forgemaster Garfrost (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_forgemaster_garfrost.cpp
-- (356 lines incl. license; 3 scripts — boss_garfrost
-- (CreatureScript via GetPitOfSaronAI (BossAI), DATA_GARFROST = 0);
-- spell_garfrost_permafrost (SpellScriptLoader: BeforeHit saronite-rock
-- LoS check -> ApplySpellImmune + PreventHit legs; AfterHit immunity
-- restore); achievement_doesnt_go_to_eleven (AchievementCriteriaScript
-- OnCheck target->ToCreature AI()->GetData(ACHIEV_DOESNT_GO_TO_ELEVEN)
-- <= 10); all registered from inside AddSC_boss_garfrost(); loader
-- decl 162 / call 357 per northrend_script_loader.cpp — the THIRD group
-- of the "// Pit of Saron" block in AddNorthrendScripts(), immediately
-- after AddSC_pit_of_saron() (call 356); the calls after it are
-- AddSC_boss_ick() (call 358) / AddSC_boss_tyrannus() (call 359) —
-- verified from the loader this run; the checkpoint sequence
-- (pit_of_saron -> boss_garfrost) is followed).
-- Entries: 36494 Forgemaster Garfrost (pit_of_saron.h NPC_GARFROST,
-- line 49; instance_pit_of_saron.cpp OnCreatureCreate binds case
-- NPC_GARFROST, line 74) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual. DATA_GARFROST
-- = 0 (pit_of_saron.h line 28); DATA_TYRANNUS = 2 (line 30).
-- Sole-source verified: whole-server-tree grep for "boss_garfrost"
-- hits boss_forgemaster_garfrost.cpp only (+ the loader decl/call
-- lines); this clone carries no sql/ tree, so ScriptName bindings are
-- DB-side by construction. No garfrost lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill (player-gated),
-- 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough, the
-- DoCast(SPELL_PERMAFROST), CallForHelp(70.0f) and
-- events.ScheduleEvent(EVENT_THROW_SARONITE, 7s) legs have no bridge).
-- KilledUnit — Talk(SAY_SLAY 4) player-gated (event 3 — the razuvious
-- player-gated variant precedent; the seventh ported variant, fifth
-- identical to nalorakk/kelthuzad/gothik/thaddius).
-- JustDied — Talk(SAY_DEATH 3) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough and
-- me->RemoveAllGameObjects() legs have no bridge; the
-- tyrannus->AI()->Talk(SAY_TYRANNUS_DEATH 0) leg is instance-driven
-- Talk on another creature's AI via
-- instance->GetGuidData(DATA_TYRANNUS) — no bridge, the
-- instance_violet_hold ScheduleCyanigosaIntro precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 36494):
-- DamageTaken — the Phase 1->2 (!HealthAbovePct(66)) and Phase 2->3
-- (!HealthAbovePct(33)) transitions Talk(SAY_PHASE2 1) /
-- Talk(SAY_PHASE3 2) with events.DelayEvents(8s) +
-- DoCast(SPELL_THUNDERING_STOMP) + ScheduleEvent(EVENT_FORGE_JUMP,
-- 1500ms) — no health-pct / DelayEvents / movement bridges (the
-- anomalus HealthBelowPct machine precedent).
-- MovementInform — the EFFECT_MOTION_TYPE / POINT_FORGE forge-jump
-- landing legs (PHASE_TWO: DoCast(SPELL_FORGE_BLADE) +
-- SetEquipmentSlots(false, EQUIP_ID_SWORD 49345); PHASE_THREE:
-- RemoveAurasDueToSpell(SPELL_FORGE_BLADE_HELPER) +
-- DoCast(SPELL_FORGE_MACE) + SetEquipmentSlots(false, EQUIP_ID_MACE
-- 49344)) + ScheduleEvent(EVENT_RESUME_ATTACK, 5s) — no motion /
-- equipment / aura bridges.
-- SpellHitTarget — SPELL_PERMAFROST_HELPER aura-stack capture into
-- _permafrostStack consumed by GetData(ACHIEV_DOESNT_GO_TO_ELEVEN) —
-- no SpellHit / GetData bridges (the SpellHit-15-never-fires queue,
-- the zuramat / devourer_of_souls precedent).
-- UpdateAI — the whole EventMap machine (EVENT_THROW_SARONITE /
-- EVENT_CHILLING_WAVE / EVENT_DEEP_FREEZE / EVENT_FORGE_JUMP /
-- EVENT_RESUME_ATTACK), the SelectTarget(Random, 0) legs, and the Talk
-- legs that ride them — Talk(SAY_THROW_SARONITE 5, target) and
-- Talk(SAY_CAST_DEEP_FREEZE 6, target) — join the no-timer-bridge /
-- no-random-target-SelectTarget queues (the boss_toravon and anomalus
-- uiSparkTimer precedents); the HasUnitState(UNIT_STATE_CASTING)
-- gates, MoveJump(25.0f, 15.0f, POINT_FORGE), AttackStop/AttackStart
-- and DoMeleeAttackIfReady legs have no bridge.
-- spell_garfrost_permafrost (SpellScriptLoader) joins the
-- no-SpellScript-bridge queue (the boss_moragg optic-link precedent).
-- achievement_doesnt_go_to_eleven (OnCheck GetData leg) joins the
-- unmodeled-achievement / no-GetData-bridge queue (the cyanigosa /
-- ichoron precedent).

local ENTRY_GARFROST = 36494

local SAY_AGGRO = 0
local SAY_DEATH = 3
local SAY_SLAY = 4

-- C++ JustEngagedWith: BossAI::JustEngagedWith(who) (passthrough, no
-- bridge); Talk(SAY_AGGRO) — unconditional; the
-- DoCast(SPELL_PERMAFROST), CallForHelp(70.0f) and
-- events.ScheduleEvent(EVENT_THROW_SARONITE, 7s) legs have no bridge —
-- the auriaya engage-port precedent.
local function garfrostEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function garfrostKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk
-- precedent; the me->RemoveAllGameObjects() leg and the
-- tyrannus->AI()->Talk(SAY_TYRANNUS_DEATH) instance-driven Talk leg
-- have no bridge.
local function garfrostJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_GARFROST, 1, garfrostEnterCombat)
RegisterCreatureEvent(ENTRY_GARFROST, 3, garfrostKilledUnit)
RegisterCreatureEvent(ENTRY_GARFROST, 4, garfrostJustDied)
