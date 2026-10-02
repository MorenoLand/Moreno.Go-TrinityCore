-- Xevozz (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_xevozz.cpp
-- (291 lines incl. license; 3 scripts — boss_xevozz (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_XEVOZZ = 7); npc_ethereal_sphere
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), DoAction
-- ACTION_SUMMON Talk leg, unbridged); spell_xevozz_summon_players
-- (SpellScriptLoader SpellScript); all registered from inside
-- AddSC_boss_xevozz(); loader decl 150 / call 345 per
-- northrend_script_loader.cpp — the SIXTH group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_moragg() (call 344); the call after it is
-- AddSC_boss_zuramat() (call 346) — the checkpoint sequence
-- (boss_moragg -> boss_xevozz) is followed).
-- Entries: 29266 Xevozz (violet_hold.h NPC_XEVOZZ, line 96) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_XEVOZZ = 7
-- (violet_hold.h line 57).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_xevozz"
-- hits boss_xevozz.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No xevozz lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDeath.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; the BossAI::JustEngagedWith passthrough leg
-- has no bridge).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 29266):
-- SAY_SPAWN (3) and SAY_CHARGED (4) are never Talk()ed anywhere in the
-- file (dead enum text, the cyanigosa precedent; grep -c "Talk(" = 6
-- total, the 3 ported above plus the 3 below).
-- Talk(SAY_SUMMON_ENERGY 6) rides the SpellHit
-- SPELL_ARCANE_POWER / H_SPELL_ARCANE_POWER leg — no SpellHit bridge;
-- joins the no-SpellHit-bridge queue.
-- Talk(SAY_REPEAT_SUMMON 5) rides the ScheduleTasks ethereal-sphere
-- summon leg (5s -> 45-47s repeat, random summon-spell selection,
-- heroic 2.5s delayed second cast, 33-35s delayed DoAction
-- ACTION_SUMMON on the summons) — no timer bridge; joins the
-- no-timer-bridge queue (the boss_toravon precedent).
-- npc_ethereal_sphere (29271, not registered — DoAction only): its
-- Talk(SAY_ETHEREAL_SPHERE_SUMMON 0) rides the DoAction
-- ACTION_SUMMON leg — no DoAction bridge; joins the no-DoAction-
-- bridge queue. Its ScheduledTasks machine (1s scheduler, instance
-- GetCreature(DATA_XEVOZZ) within-3.0f -> ARCANE_POWER cast +
-- DespawnOrUnsummon), the Reset SPELL_POWER_BALL_VISUAL /
-- SPELL_POWER_BALL_DAMAGE_TRIGGER self-casts + 40s despawn, the
-- JustSummoned MoveFollow leg and the JustReachedHome
-- instance->SetData(DATA_HANDLE_CELLS, DATA_XEVOZZ) leg have no
-- bridges — the no-instance-GetCreature / no-MovementInform /
-- no-instance-SetData bridge queues.
-- The ScheduleTasks ARCANE_BARRAGE_VOLLEY AOE / ARCANE_BUFFET
-- random-target machine and the spell_xevozz_summon_players
-- SpellScript hook (OnEffectHitTarget SPELL_EFFECT_DUMMY ->
-- SPELL_MAGIC_PULL) join the no-timer-bridge / no-SpellScript-
-- bridge queues. The file carries a TODO ("Implement Ethereal Summon
-- Target").

local ENTRY_XEVOZZ = 29266

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ boss_xevozzAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function xevozzEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function xevozzKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; _JustDied()
-- (passthrough, no bridge) — the sjonnir JustDied-Talk precedent.
local function xevozzJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_XEVOZZ, 1, xevozzEnterCombat)
RegisterCreatureEvent(ENTRY_XEVOZZ, 3, xevozzKilledUnit)
RegisterCreatureEvent(ENTRY_XEVOZZ, 4, xevozzJustDied)
