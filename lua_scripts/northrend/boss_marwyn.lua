-- Marwyn (Halls of Reflection) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/boss_marwyn.cpp
-- (2 scripts — boss_marwyn (CreatureScript via boss_horAI (custom
-- HallsOfReflection base AI wrapping GetHallsOfReflectionAI),
-- DATA_MARWYN = 1) + spell_marwyn_shared_suffering (SpellScriptLoader,
-- AuraScript hook); both registered from inside AddSC_boss_marwyn();
-- loader decl 169 / call 364 per northrend_script_loader.cpp — the
-- FOURTH and LAST group of the "// Halls of Reflection" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_falric() (call
-- 363) — verified from the loader this run; the checkpoint sequence
-- (boss_falric -> boss_marwyn) is followed; the block CLOSES here).
-- Entry: 38113 Marwyn (halls_of_reflection.h NPC_MARWYN, line 72;
-- instance_halls_of_reflection.cpp OnCreatureCreate binds case
-- NPC_MARWYN, line 133) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_marwyn"
-- hits boss_marwyn.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No marwyn lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill (player-gated), 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_marwyn JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 — the auriaya
-- engage-port precedent; the DoZoneInCombat, instance->SetBossState
-- IN_PROGRESS and four ScheduleEvent legs have no bridge).
-- boss_marwyn KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the
-- razuvious player-gated variant precedent; the eleventh ported
-- variant, ninth identical to
-- nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric).
-- boss_marwyn JustDied — Talk(SAY_DEATH 2) (event 4 — the sjonnir
-- JustDied-Talk precedent; the events.Reset and instance->SetBossState
-- DONE legs have no bridge). Melee engine-driven.
-- DOCUMENTED-ONLY (in this header; only entry 38113 registered):
-- boss_marwyn Reset — boss_horAI::Reset passthrough — no bridge by
-- construction.
-- boss_marwyn JustEngagedWith scheduler legs — EVENT_OBLITERATE 8s-13s
-- / EVENT_WELL_OF_CORRUPTION 12s / EVENT_CORRUPTED_FLESH 20s /
-- EVENT_SHARED_SUFFERING 14s-15s — no-timer-bridge queue (the
-- boss_toravon precedent).
-- boss_marwyn UpdateAI — the whole EventMap machine (OBLITERATE
-- victim-cast; WELL_OF_CORRUPTION / SHARED_SUFFERING random-target
-- casts; CORRUPTED_FLESH AoE with Talk(SAY_CORRUPTED_FLESH 3) riding the
-- timer leg) — no-timer-bridge / no-random-target-SelectTarget queues;
-- the UNIT_STATE_CASTING gate, DoCast / DoCastVictim / DoCastAOE and
-- DoMeleeAttackIfReady legs have no bridge.
-- spell_marwyn_shared_suffering (AuraScript — AfterEffectRemove
-- remaining-damage forward to SPELL_SHARED_SUFFERING_DISPEL 72373) —
-- no-AuraScript bridge (the boss_moragg optic-link precedent).

local ENTRY_MARWYN = 38113

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_marwynAI::JustEngagedWith: Talk(SAY_AGGRO) — the auriaya
-- engage-port precedent.
local function marwynJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_marwynAI::KilledUnit: if (who->GetTypeId() != TYPEID_PLAYER)
-- return; Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function marwynKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_marwynAI::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent.
local function marwynJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_MARWYN, 1, marwynJustEngagedWith)
RegisterCreatureEvent(ENTRY_MARWYN, 3, marwynKilledUnit)
RegisterCreatureEvent(ENTRY_MARWYN, 4, marwynJustDied)
