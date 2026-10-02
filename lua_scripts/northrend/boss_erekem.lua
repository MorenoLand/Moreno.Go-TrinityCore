-- Erekem (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_erekem.cpp
-- (303 lines; 2 scripts — boss_erekem (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_EREKEM = 4);
-- npc_erekem_guard (CreatureScript via GetVioletHoldAI (ScriptedAI),
-- zero Talk arms); both registered from inside AddSC_boss_erekem();
-- loader decl 146 / call 341 per northrend_script_loader.cpp — the
-- SECOND group of the "// Violet Hold" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_cyanigosa()
-- (call 340); the call after it is AddSC_boss_ichoron() (call 342) —
-- the checkpoint sequence (boss_cyanigosa -> boss_erekem) is followed).
-- Entries: 29315 Erekem (violet_hold.h NPC_EREKEM, line 104) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_EREKEM = 4
-- (violet_hold.h line 54). Guard entry 29395 NPC_EREKEM_GUARD
-- (violet_hold.h line 105) carries zero Talk arms — no registration.
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_erekem"
-- hits boss_erekem.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No erekem lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough and
-- the DoCast(me, SPELL_EARTH_SHIELD) self-cast leg have no bridges).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 29315):
-- SAY_SPAWN (3) / SAY_ADD_KILLED (4) / SAY_BOTH_ADDS_KILLED (5) are
-- never Talk()ed anywhere in the file — a whole-VioletHold-tree grep
-- shows no Talk() site for any of them (dead enum text, the cyanigosa
-- precedent; grep -c "Talk(" = 3 total, all ported above).
-- The ScheduleTasks scheduler machine (EARTH_SHIELD 20s / BLOODLUST
-- 2s-then-35-45s / LIGHTNING_BOLT 2s-then-2.5s / CHAIN_HEAL 10s-then-3s
-- or 8-11s / EARTH_SHOCK 2-8s-then-8-13s / the Break-Bonds 0s-then-500ms
-- or 10s guard-aura-scan leg), the UpdateAI _phase machine
-- (CheckGuardAlive -> SetCanDualWield(true) + Windfury trigger-cast,
-- DoSpellAttackIfReady(STORMSTRIKE) / DoMeleeAttackIfReady legs),
-- CheckGuardAuras / CheckGuardAlive / GetChainHealTarget
-- (instance->GetGuidData(DATA_EREKEM_GUARD_1/2) + ObjectAccessor cross-
-- creature machine), MovementInform POINT_INTRO SetFacingTo leg,
-- JustReachedHome instance->SetData(DATA_HANDLE_CELLS, DATA_EREKEM)
-- leg and the npc_erekem_guard spell-timer machine (STRIKE / HOWLING
-- SCREECH / GUSHING_WOUND legs — zero Talk arms) — no timer /
-- instance-GetGuidData / cross-creature / MovementInform /
-- SetData bridges; join the respective bridge queues.

local ENTRY_EREKEM = 29315

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_erekemAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent); the DoCast(me, SPELL_EARTH_SHIELD) leg has no
-- bridge.
local function erekemEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function erekemKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function erekemJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_EREKEM, 1, erekemEnterCombat)
RegisterCreatureEvent(ENTRY_EREKEM, 3, erekemKilledUnit)
RegisterCreatureEvent(ENTRY_EREKEM, 4, erekemJustDied)
