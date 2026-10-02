-- Jedoga Shadowseeker (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_jedoga_shadowseeker.cpp
-- (four scripts — boss_jedoga_shadowseeker (BossAI),
-- npc_twilight_volunteer (ScriptedAI),
-- spell_random_lightning_visual_effect (SpellScript),
-- achievement_volunteer_work (AchievementCriteriaScript)).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 35 / call 225, immediately before AddSC_boss_volazj();
-- next: instance_ahnkahet — the Ahn'kahet dungeon boss roster closes
-- with this unit: elder_nadox 29309, taldaram 29308, amanitar 30258,
-- volazj 29311, jedoga 29310).
-- Entry: 29310 Jedoga Shadowseeker (ahnkahet.h
-- NPC_JEDOGA_SHADOWSEEKER — kalecgos pass; the
-- RegisterAhnKahetCreatureAI ScriptName binding is instance-shimmed,
-- the creature_template binding DB-side as usual). Sole-source
-- verified: whole-server-tree grep for "boss_jedoga_shadowseeker",
-- "npc_twilight_volunteer", "spell_random_lightning_visual_effect"
-- and "achievement_volunteer_work" hits boss_jedoga_shadowseeker.cpp
-- only (loader carries only the decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- boss_jedoga_shadowseeker: JustEngagedWith Talk(SAY_AGGRO 0)
-- (event 1 — the hook itself is bridged; its other legs —
-- RemoveAurasDueToSpell(SPELL_SPHERE_VISUAL 56075) /
-- RemoveAurasDueToSpell(SPELL_RANDOM_LIGHTNING_VISUAL 56327) (no
-- aura-removal bridge — aeranas precedent), SummonCreatureGroup(
-- SUMMON_GROUP_WORSHIPPERS) (summon STRAND absent — standing),
-- events.SetPhase(PHASE_ONE) (no phase bridge on the Lua surface —
-- moorabi precedent), and the 25 NPC_TWILIGHT_VOLUNTEER 30385
-- summons with MovePoint(POINT_INITIAL_POSITION) choreography
-- (summon STRAND + motion-master bridges absent) — are
-- documented-only below);
-- KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 4) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- Unmodeled (no bridges — documented, not wired):
-- Reset intro machine: _Reset() (instance-script model absent —
-- standing blocker), events.SetPhase(PHASE_INTRO) (no phase
-- bridge), SetReactState(REACT_PASSIVE) (no react-state bridge —
-- drakkari_colossus precedent), SummonCreatureGroup(
-- SUMMON_GROUP_INITIATES) (summon STRAND absent), the two
-- NPC_JEDOGA_CONTROLLER 30181 summons with SPELL_BEAM_VISUAL_
-- JEDOGA 56312 beam-visual casts (summon STRAND absent), and
-- EVENT_INTRO_SAY (Talk(SAY_PREACHING 5), Minutes(2) repeat —
-- instance->GetBossState(DATA_PRINCE_TALDARAM) == DONE gate needs
-- the absent instance-script model).
-- The whole combat rotation is phase-gated through
-- events.SetPhase / IsInPhase (PHASE_INTRO / PHASE_ONE / PHASE_TWO
-- / PHASE_THREE) and the phase flips are driven by unbridged arms
-- (intro MovementInform(POINT_GROUND) -> REACT_AGGRESSIVE +
-- DoZoneInCombat; DamageTaken HealthBelowPct(55) -> PHASE_TWO, no
-- health-pct bridge — doomwalker precedent; EVENT_CHOOSE_
-- VOLUNTEER / volunteer-DoAction sacrifice machine -> PHASE_THREE;
-- EVENT_END_PHASE_TWO -> MoveLand(POINT_GROUND) re-arm) — no phase
-- bridge on the Lua surface (gal_darah / moorabi precedent), so
-- nothing phase-gated is emulated; ungated timers would over-cast
-- versus C++ (e.g. Cyclone Strike firing during the unbridged
-- phase-two flying sacrifice sequence — the intro gating alone
-- makes the engage-timer reading unfaithful).
-- EVENT_CYCLONE_STRIKE — DoCastSelf(Cyclone Strike 56855),
-- MovementInform(POINT_GROUND)-scheduled 3s init, 15-30s repeat;
-- bridged in isolation (creature:CastSpell(creature, spell) —
-- moorabi precedent) but gated as above — documented-only.
-- EVENT_LIGHTNING_BOLT — SelectTarget(SelectTargetMethod::Random,
-- 0, 100.0f, true) -> DoCast(target, Lightning Bolt 56891), 7s
-- init, 15-30s repeat; EVENT_THUNDERSHOCK — same selector ->
-- DoCast(target, Thundershock 56926), 12s init, 15-30s repeat: the
-- random-target SelectTarget bridge is absent (cairne/kazzak
-- precedent), so both casts are unmodelable — documented-only.
-- DamageTaken 55% health crossing (events.Reset + SetPhase(
-- PHASE_TWO) + EVENT_START_PHASE_TWO 1s) — no health-pct bridge
-- (doomwalker precedent) — documented-only.
-- Phase-two machine — EVENT_START_PHASE_TWO (SetReactState(
-- REACT_PASSIVE), AttackStop, InterruptNonMeleeSpells(true),
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE), MovePoint(
-- POINT_PHASE_TWO)), EVENT_FLY_DELAY (SetDisableGravity(true) +
-- MoveTakeoff(POINT_PHASE_TWO_FLY)), EVENT_CHOOSE_VOLUNTEER
-- (controller summon + SetFacingToObject + SPELL_SACRIFICE_VISUAL
-- 56133 self-cast + Talk(SAY_CHOOSE 1) + random volunteer pick via
-- _volunteerGUIDS + ObjectAccessor::GetCreature + cross-AI
-- DoAction(ACTION_CHOSEN)), EVENT_SUMMON_VOLUNTEER re-summon
-- machine, EVENT_END_PHASE_TWO (summons.DespawnEntry(controller) +
-- DoCastSelf(SPELL_HOVER_FALL_2 56157) + MoveLand(POINT_GROUND)):
-- summon STRAND + motion-master (MovePoint / MoveLand / MoveTakeoff
-- / SetDisableGravity) + flag / interrupt / stand-state + cross-AI
-- DoAction + ObjectAccessor bridges absent — documented-only.
-- DoAction(ACTION_SACRIFICE) (Talk(SAY_SACRIFICE 2) + DoCastAOE(
-- Sacrifice Beam 56150) — no DoCastAOE bridge (terestian / shazzrah
-- precedent) — + EVENT_END_PHASE_TWO re-schedule + EVENT_SUMMON_
-- VOLUNTEER reschedule): no DoAction bridge on the Lua surface
-- (drakkari_colossus standing precedent) — documented-only.
-- JustSummoned (NPC_TWILIGHT_WORSHIPPER 30111 SetStandState(
-- UNIT_STAND_STATE_KNEEL) + summons.Summon — no stand-state / cross-
-- AI bridges), SummonedCreatureDies (initiate _initiatesKilled == 15
-- latch -> SPELL_HOVER_FALL_1 56100 + EVENT_START_FIGHT_1 1s
-- (RemoveAurasDueToSpell(SPELL_BEAM_VISUAL_JEDOGA 56312) then
-- summons.DespawnEntry(controller) + SetDisableGravity(false) +
-- MoveLand(POINT_GROUND)) and the volunteer _volunteerWork=false
-- latch + PHASE_THREE flip; payload rides the unbridged summon /
-- movement / instance arms), EnterEvadeMode (summons.DespawnAll —
-- no despawn bridge, terestian precedent; _DespawnAtEvade(15s) —
-- no bridge), MovementInform (all point choreography — no motion
-- bridges), GetData(DATA_VOLUNTEER_WORK) (no cross-AI GetData
-- bridge — achievement_volunteer_work needs it too).
-- npc_twilight_volunteer — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (30385 — ahnkahet.h NPC_TWILIGHT_VOLUNTEER, kalecgos pass): zero
-- registration. The entire AI rides absent bridges — DoAction(
-- ACTION_CHOSEN) initiation (no DoAction bridge), SPELL_PILLAR_OF_
-- LIGHTNING 56868 self-cast (port-pattern-ready: creature:CastSpell(
-- creature, spell)), RemoveAurasDueToSpell(SPELL_SPHERE_VISUAL_
-- VOLUNTEER 56102) (no aura-removal bridge), Talk(SAY_CHOSEN 0) /
-- Talk(SAY_SACRIFICED 1), stand-state machine, MovePoint(
-- POINT_SACRIFICE) choreography (no motion bridges), the sacrifice
-- payload (instance->GetCreature(DATA_JEDOGA_SHADOWSEEKER) +
-- cross-AI DoAction(ACTION_SACRIFICE) — instance-script model and
-- DoAction bridges absent), and DespawnOrUnsummon(5s) (no despawn
-- bridge — terestian precedent). Joins the entry-verifiable-but-
-- bridge-blocked queue (amanitar mushrooms precedent).
-- spell_random_lightning_visual_effect (SpellScript 56328 — On
-- DestinationTargetSelect EFFECT_0 relocate -19z): no SpellScript
-- binding bridge (razelikh precedent) — documented-only.
-- achievement_volunteer_work (AchievementCriteriaScript — GetAI()->
-- GetData(DATA_VOLUNTEER_WORK)): no achievement-criteria bridge on
-- the Lua surface (snakes precedent) + the cross-AI GetData bridge
-- is absent anyway — documented-only.

local ENTRY_JEDOGA_SHADOWSEEKER = 29310

local SAY_AGGRO = 0
local SAY_SLAY = 3
local SAY_DEATH = 4

local function jedogaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

local function jedogaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function jedogaDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 1, jedogaEnterCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 3, jedogaTargetDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 4, jedogaDied)
