-- Anub'arak (Azjol-Nerub dungeon) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/AzjolNerub/boss_anubarak.cpp
-- (eight scripts — boss_anub_arak (BossAI),
-- npc_anubarak_anub_ar_darter / npc_anubarak_anub_ar_assassin /
-- npc_anubarak_anub_ar_guardian / npc_anubarak_anub_ar_venomancer
-- (ScriptedAI), npc_anubarak_impale_target (NullCreatureAI),
-- spell_anubarak_pound / spell_anubarak_carrion_beetles (AuraScript
-- via RegisterSpellScript); AddSC_boss_anub_arak at end registers
-- all via GetAzjolNerubAI / RegisterSpellScript).
-- Azjol-Nerub dungeon-script unit per northrend_script_loader.cpp
-- order (decl 29 / call 231, immediately after AddSC_boss_hadronox();
-- next: AddSC_instance_azjol_nerub() — the Azjol-Nerub dungeon boss
-- roster closes with that unit: krik_thir 28684, hadronox 28921,
-- anub_arak 29120).
-- Entry: 29120 Anub'arak (azjol_nerub.h NPC_ANUBARAK — kalecgos
-- pass; the GetAzjolNerubAI ScriptName binding is instance-shimmed,
-- the creature_template binding DB-side as usual). Sole-source
-- verified: whole-server-tree grep for "boss_anub_arak",
-- "npc_anubarak_anub_ar_darter", "npc_anubarak_anub_ar_assassin",
-- "npc_anubarak_anub_ar_guardian", "npc_anubarak_anub_ar_venomancer",
-- "npc_anubarak_impale_target", "spell_anubarak_pound" and
-- "spell_anubarak_carrion_beetles" hits boss_anubarak.cpp only
-- (loader carries only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention) —
-- no bridged casts here anyway; the submerge machine that gates
-- them is entirely unbridged.
-- Ported arms (C++-exact for all modeled arms):
-- boss_anub_arak: JustEngagedWith Talk(SAY_AGGRO 0) (event 1 — the
-- hook itself is bridged; its other legs — the two DATA_ANUBARAK_
-- WALL GO-state doors (instance->GetGameObject, GO_STATE_ACTIVE —
-- instance-script model absent, standing blocker), the
-- DoStartTimedAchievement(ACHIEV_GOTTA_GO_START_EVENT 20382)
-- (instance model absent), events.SetPhase(PHASE_EMERGE) (no phase
-- bridge on the Lua surface — gal_darah / moorabi precedent),
-- EVENT_CLOSE_DOOR (5s GO-state flip — instance model absent),
-- SummonCreatureGroup(SUMMON_GROUP_WORLD_TRIGGER_GUARDIAN) and the
-- DoSummon(NPC_WORLD_TRIGGER 22515, TEMPSUMMON_MANUAL_DESPAWN)
-- world-trigger setup (summon STRAND absent — standing; the EnterEvadeMode
-- failure legs ride the same absent bridge) — documented-only below);
-- KilledUnit Talk(SAY_SLAY 1) player-gated (event 3,
-- victim:GetTypeId() == TYPEID_PLAYER — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- SAY_INTRO (5) is defined in the Yells enum but never fired by any
-- C++ code in this file — nothing to port.
-- Unmodeled (no bridges — documented, not wired):
-- Reset legs: RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE
-- | UNIT_FLAG_NOT_SELECTABLE) (no unit-flag bridge), DoStopTimedAchievement(
-- ACHIEV_GOTTA_GO_START_EVENT) (instance model absent).
-- The whole combat rotation is phase-gated through events.SetPhase /
-- phase-group scheduling (PHASE_EMERGE / PHASE_SUBMERGE) and the
-- phase flips are driven by unbridged arms (DamageTaken
-- HealthBelowPctDamaged(_nextSubmerge) submerge gate — no health-pct
-- bridge (doomwalker precedent) + no DamageTaken hook
-- (npc_unkor_the_ruthless precedent); SpellHit(SPELL_SUBMERGE
-- 53421) submerge payload — SpellHit event 15 never fires
-- (standing); DoAction(ACTION_PET_DIED) re-emerge — no DoAction
-- bridge (drakkari_colossus precedent) + cross-AI SetGUID/GUID_TYPE_
-- PET bridge absent) — no phase bridge (gal_darah / moorabi
-- precedent), so ungated timers would over-cast versus C++ (Pound
-- firing during the unbridged submerge phase — jedoga precedent):
-- nothing phase-gated is emulated.
-- EVENT_POUND — DoCastVictim(Pound 59433), 2-4s init, 26-32s repeat
-- (bridged in isolation — creature:CastSpell(nil, spell) =
-- DoCastVictim, moroes precedent — but phase-gated as above) —
-- documented-only.
-- EVENT_LEECHING_SWARM — Talk(SAY_LOCUST 3) + DoCastAOE(Leeching
-- Swarm 53467), 5-7s init, 25-28s repeat — no DoCastAOE bridge
-- (terestian / shazzrah precedent) + phase-gated — documented-only.
-- EVENT_CARRION_BEETLES — DoCastAOE(Carrion Beetles 53520), 14-17s
-- init, 24-27s repeat — no DoCastAOE bridge + phase-gated —
-- documented-only.
-- EVENT_IMPALE — DoCast(impaleTarget, Impale Damage 53454, triggered)
-- 4s after SetGUID(GUID_TYPE_IMPALE) — the impale target arrives
-- via cross-AI SetGUID from npc_anubarak_impale_target (no cross-AI
-- SetGUID bridge) — documented-only.
-- EVENT_SUBMERGE — Talk(SAY_SUBMERGE 4) + DoCastSelf(Submerge
-- 53421), rescheduled 0s by DamageTaken at the 75/50/25% crossings
-- — no health-pct + no DamageTaken hook bridges — documented-only.
-- EVENT_DARTER / EVENT_ASSASSIN / EVENT_GUARDIAN / EVENT_VENOMANCER
-- (the submerge pet waves — trigger->CastSpell(trigger,
-- SPELL_SUMMON_DARTER 53599 / _ASSASSIN 53609 / _GUARDIAN 53614 /
-- _VENOMANCER 53615, true) with the _assassinCount/_guardianCount/
-- _venomancerCount 20s-decrement bookkeeping) — the world-trigger
-- GUIDs come from the unbridged summon setup; the grid-scan
-- GetCreatureListWithEntryInGrid(NPC_WORLD_TRIGGER) +
-- SelectRandomContainerElement + ObjectAccessor::GetCreature cross-AI
-- legs need absent bridges; the EnterEvadeMode failure legs ride the
-- same absent bridges — documented-only.
-- SetGUID(GUID_TYPE_PET -> JustSummoned, GUID_TYPE_IMPALE ->
-- EVENT_IMPALE 4s) / DoAction(ACTION_PET_DIED -> last-pet-died
-- re-emerge: RemoveAurasDueToSpell(SUBMERGE 53421 / IMPALE_AURA
-- 53456) (no aura-removal bridge — aeranas precedent), RemoveFlag,
-- DoCastSelf(EMERGE 53500), SetPhase(PHASE_EMERGE) + re-armed POUND/
-- LEECHING_SWARM/CARRION_BEETLES (phase-gated); ACTION_PET_EVADE ->
-- EnterEvadeMode) — no cross-AI SetGUID / DoAction bridges —
-- documented-only.
-- EnterEvadeMode: summons.DespawnAll (no despawn bridge — terestian
-- precedent), _DespawnAtEvade() — documented-only.
-- DamageTaken: HasAura(SUBMERGE) damage=0 / the
-- HealthBelowPctDamaged(_nextSubmerge, damage) -> RescheduleEvent(
-- EVENT_SUBMERGE, 0s, 0, PHASE_EMERGE) gate with the _nextSubmerge
-- -= 25 stepping — no DamageTaken hook + no health-pct bridge —
-- documented-only.
-- SpellHit(SPELL_SUBMERGE 53421) payload: SetFlag(NON_ATTACKABLE |
-- NOT_SELECTABLE) (no flag bridge), RemoveAurasDueToSpell(
-- LEECHING_SWARM) (no aura-removal bridge), DoCastSelf(IMPALE_AURA
-- 53456, true) (bridged in isolation — creature:CastSpell(creature,
-- spell) — but the trigger is event 15), SetPhase(PHASE_SUBMERGE)
-- + the 50%/25%/0% _nextSubmerge pet-count machine (_assassinCount
-- 4->6->6, _guardianCount 2, _venomancerCount 0->2->2, third wave
-- arms EVENT_DARTER) — event 15 never fires (standing) —
-- documented-only.
-- UpdateAI UNIT_STATE_CASTING skip — no unit-state bridge.
-- npc_anubarak_anub_ar_darter / _assassin / _guardian / _venomancer
-- / npc_anubarak_impale_target — ENTRY UNVERIFIABLE (no NPC
-- constants in azjol_nerub.h or anywhere in the C++ sources; the
-- pets are summoned from DB-side summon-spell data (SPELL_SUMMON_
-- DARTER 53599 etc.), not C++ evidence — belnistrasz/willix
-- precedent, no invented identifiers): zero registration; they
-- join the bridgeable-but-entry-blocked queue. Port-pattern-ready
-- rotation arms — guardian EVENT Sunder Armor DoCastVictim(53618)
-- 6s init, 12s repeat; venomancer EVENT Poison Bolt DoCastVictim(
-- 53617) 5s init, 2-3s repeat; assassin JustEngagedWith backstab
-- machine (DoCastVictim(SPELL_BACKSTAB 52540) gated on
-- victim->isInBack(me) — no positional-facing bridge anyway,
-- krik_thir skirmisher precedent). Additionally unbridged:
-- the pet-template InitializeAI leg (cross-AI SetGUID(GUID_TYPE_PET)
-- -> JustSummoned — SetGUID bridge absent; GetCreature(DATA_ANUBARAK)
-- instance legs — instance model absent), JustDied
-- ACTION_PET_DIED (DoAction bridge absent), EnterEvadeMode
-- ACTION_PET_EVADE (DoAction bridge absent); darter InitializeAI
-- DoCastAOE(SPELL_DART 59349) (no DoCastAOE bridge); assassin
-- InitializeAI MoveJump choreography (GetRandomPositionAround
-- 10-30f + CreatureBoundary IsInBounds loop + MotionMaster MoveJump —
-- no motion-master / boundary bridges) + DoCastSelf(
-- ASSASSIN_VISUAL 53611, triggered) (bridged in isolation —
-- creature:CastSpell(creature, spell) — but gated by the unbridged
-- jump) + MovementInform(EVENT_JUMP) -> RemoveAurasDueToSpell(
-- ASSASSIN_VISUAL) (no aura-removal bridge) + DoZoneInCombat;
-- impale_target InitializeAI DoCastSelf(IMPALE_VISUAL 53455) +
-- DespawnOrUnsummon(6s) + cross-AI SetGUID(GUID_TYPE_IMPALE)
-- (no despawn / SetGUID bridges).
-- spell_anubarak_pound (AuraScript 59433: AfterApply SPELL_AURA_FLY
-- EFFECT_2 -> caster CastSpell(target, SPELL_POUND_DAMAGE 59432,
-- triggered)) / spell_anubarak_carrion_beetles (AuraScript 53520:
-- OnEffectPeriodic SPELL_AURA_PERIODIC_DUMMY -> caster double-cast
-- SPELL_CARRION_BEETLE 53521) — no SpellScript/AuraScript binding
-- bridge on the Lua surface (razelikh precedent) — documented-only.

local ENTRY_ANUBARAK = 29120

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local function anubarakEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

local function anubarakTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function anubarakDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ANUBARAK, 1, anubarakEnterCombat)
RegisterCreatureEvent(ENTRY_ANUBARAK, 3, anubarakTargetDied)
RegisterCreatureEvent(ENTRY_ANUBARAK, 4, anubarakDied)
