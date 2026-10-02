-- Tharon'ja (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_tharon_ja.cpp
-- (boss_tharon_ja (BossAI), spell_tharon_ja_clear_gift_of_tharon_ja
-- (SpellScript); AddSC_boss_tharon_ja at end registers all via
-- GetDrakTharonKeepAI / RegisterSpellScript). The fourth and last
-- Drak'Tharon Keep group in northrend_script_loader.cpp order
-- (decl 42 / call 237, immediately after AddSC_boss_king_dred();
-- Drak'Tharon Keep block now CLOSED).
-- Entry: 26632 Tharon'ja (drak_tharon_keep.h NPC_THARON_JA —
-- kalecgos pass; the GetDrakTharonKeepAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_tharon_ja"
-- and "spell_tharon_ja_clear_gift_of_tharon_ja" hits
-- boss_tharon_ja.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ JustEngagedWith/KilledUnit/JustDied
-- hooks fire unconditionally (the phase machine never gates them),
-- so the three Talk arms are C++-exact. No timers are scheduled:
-- every C++ scheduled arm is phase-gated by the unbridged phase
-- machine (GOING_FLESH / GOING_SKELETAL call events.Reset()), so
-- ungated emulation would over-cast vs C++ — jedoga precedent;
-- anub_arak EVENT_POUND precedent (bridged in isolation but
-- phase-gated → documented-only).
-- Ported arms:
-- boss_tharon_ja: JustEngagedWith Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith leg is instance bookkeeping, no bridge);
-- KilledUnit Talk(SAY_KILL 1) player-gated (event 3,
-- who->GetTypeId()==TYPEID_PLAYER — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 4) (event 4; _JustDied instance bookkeeping has
-- no bridge).
-- Unmodeled (no bridges — documented, not wired):
-- The phase machine — EVENT_DECAY_FLESH (DoCastAOE(Decay Flesh
-- 49356), 20s) -> EVENT_GOING_FLESH (+6s: Talk(SAY_FLESH 2) +
-- me->SetDisplayId(MODEL_FLESH 27073) + DoCastAOE(Gift of Tharon'ja
-- 52509, true) + DoCast(me, SPELL_FLESH_VISUAL 52582, true) +
-- DoCast(me, SPELL_DUMMY 49551, true) + events.Reset() + phase-2
-- schedule) -> EVENT_RETURN_FLESH (20s: DoCastAOE(Return Flesh
-- 53463)) -> EVENT_GOING_SKELETAL (+6s: Talk(SAY_SKELETON 3) +
-- me->RestoreDisplayId() + DoCastAOE(Clear Gift of Tharon'ja 53242,
-- true) + events.Reset() + phase-1 re-arm). The phase transitions'
-- meaningful legs are the DoCastAOE casts — no DoCastAOE bridge
-- (terestian/shazzrah precedent); SetDisplayId/RestoreDisplayId
-- have no display bridge. Per the genuinely-bridgeable bar
-- (king_dred EVENT_RAPTOR_CALL), the chain is not armed: firing
-- the GOING_FLESH Talk fragment alone would play the flesh yell
-- without its C++ phase payload.
-- Phase-1 rotation (all gated by GOING_FLESH's events.Reset()):
-- EVENT_CURSE_OF_LIFE — SelectTarget(Random) -> DoCast(Curse of
-- Life 49527), 1s init, 10-15s repeat — random-target SelectTarget
-- bridge absent (cairne/kazzak precedent); EVENT_RAIN_OF_FIRE —
-- SelectTarget(Random) -> DoCast(Rain of Fire 49518), 14-18s init,
-- 14-18s repeat — random-target SelectTarget bridge absent;
-- EVENT_SHADOW_VOLLEY — DoCastVictim(Shadow Volley 49528), 8-10s
-- init, 8-10s repeat — bridged in isolation (moroes precedent) but
-- phase-gated (anub_arak EVENT_POUND precedent).
-- Phase-2 rotation (all gated by GOING_SKELETAL's events.Reset()):
-- EVENT_LIGHTNING_BREATH — SelectTarget(Random) -> DoCast(
-- Lightning Breath 49537), 3-4s init, 6-7s repeat —
-- random-target SelectTarget bridge absent; EVENT_EYE_BEAM —
-- SelectTarget(Random) -> DoCast(Eye Beam 49544), 4-8s init, 4-6s
-- repeat — random-target SelectTarget bridge absent;
-- EVENT_POISON_CLOUD — DoCastAOE(Poison Cloud 49548), 6-7s init,
-- 10-12s repeat — no DoCastAOE bridge.
-- Reset — _Reset() (instance bookkeeping, no bridge) +
-- me->RestoreDisplayId() (no display bridge).
-- JustDied — DoCastAOE(CLEAR_GIFT_OF_THARON_JA 53242, true) +
-- DoCastAOE(SPELL_ACHIEVEMENT_CHECK 61863, true) — no DoCastAOE
-- bridge.
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) skip (twice — top of
-- UpdateAI and inside the event loop) — no unit-state bridge.
-- spell_tharon_ja_clear_gift_of_tharon_ja — SpellScript (53242:
-- EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT -> target->RemoveAura(
-- SPELL_GIFT_OF_THARON_JA 52509)) — no SpellScript binding bridge
-- (razelikh precedent) — documented-only.
-- C++ "Known Issues" note carried: spells 49356 (Decay Flesh) and
-- 53463 (Return Flesh) will be interrupted for an unknown reason.

local ENTRY_THARON_JA = 26632

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 4

local function tharonJaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

local function tharonJaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function tharonJaDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_THARON_JA, 1, tharonJaEnterCombat)
RegisterCreatureEvent(ENTRY_THARON_JA, 3, tharonJaTargetDied)
RegisterCreatureEvent(ENTRY_THARON_JA, 4, tharonJaDied)
