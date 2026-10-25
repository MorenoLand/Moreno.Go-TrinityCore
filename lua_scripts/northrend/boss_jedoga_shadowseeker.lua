-- Jedoga Shadowseeker (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_jedoga_shadowseeker.cpp
-- (four scripts — boss_jedoga_shadowseeker (BossAI),
-- npc_twilight_volunteer (ScriptedAI),
-- spell_random_lightning_visual_effect (SpellScript),
-- achievement_volunteer_work (AchievementCriteriaScript)).
-- Ahn'kahet dungeon-script unit per northrend_script_loader.cpp order
-- (decl 35 / call 225, next: AddSC_boss_volazj();
-- the Ahn'kahet dungeon boss roster closes with that unit, then
-- instance_ahnkahet (decl 37 / call 227):
-- elder_nadox 29309, taldaram 29308, amanitar 30258,
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
-- boss_jedoga_shadowseeker: Reset controller summons (event 23,
-- IsAlive-guarded — Go 23 also fires on death) + beam-visual casts
-- (jedogaReset; the group / phase / react legs stay below);
-- JustEngagedWith Talk(SAY_AGGRO 0)
-- (event 1 — the hook itself is bridged); the
-- RemoveAurasDueToSpell(SPELL_SPHERE_VISUAL 56075) /
-- RemoveAurasDueToSpell(SPELL_RANDOM_LIGHTNING_VISUAL 56327) legs are
-- PORTED (stale "no aura-removal bridge" claim retracted —
-- creature:RemoveAura bridged); SummonCreatureGroup(
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
-- bridge), SetReactState(REACT_PASSIVE) (react-state bridge exists
-- since 2026-10-20 — the arm stays unported because the intro Reset is
-- instance/phase-gated, drakkari_colossus precedent), SummonCreatureGroup(
-- SUMMON_GROUP_INITIATES) (no SummonCreatureGroup bridge — standing),
-- and
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
-- "random-target SelectTarget bridge absent (cairne/kazzak)" claim is
-- STALE (GetAITargets is the bridge, tharon_ja precedent) — the
-- blocker is gating, not bridge: both are POINT_GROUND-scheduled
-- phase-one rotation arms, and ungated engage timers would over-cast
-- during the unbridged intro/phase-two machines (gothik rule) —
-- documented-only.
-- DamageTaken 55% health crossing (events.Reset + SetPhase(
-- PHASE_TWO) + EVENT_START_PHASE_TWO 1s) — the old "no health-pct
-- bridge (doomwalker precedent)" claim was stale (event 9 carries
-- pre-damage damage; creature:GetHealthPct() is bridged), but the
-- arm stays documented-only: the payload is the whole unbridged
-- phase-two machine — SetReactState/InterruptNonMeleeSpells/
-- SetFlag(NOT_SELECTABLE) + the flying-sacrifice MovePoint/motion
-- + summon + cross-AI DoAction sequence (no phase / motion /
-- summon / cross-AI bridges; flag/react-state bridges exist since
-- 2026-10-20 but the payload is the unported phase machine).
-- Phase-two machine — EVENT_START_PHASE_TWO (SetReactState(
-- REACT_PASSIVE), AttackStop, InterruptNonMeleeSpells(true),
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE), MovePoint(
-- POINT_PHASE_TWO)), EVENT_FLY_DELAY (SetDisableGravity(true) +
-- MoveTakeoff(POINT_PHASE_TWO_FLY)), EVENT_CHOOSE_VOLUNTEER
-- (controller summon + SetFacingToObject — the me->
-- SetFacingToObject(controller) leg (C++ :337) is bridgeable via the
-- atan2 twin (devourer_of_souls precedent), but the arm stays
-- documented-only: the controller is created by SummonCreature (no
-- summon bridge) inside the unported phase-two machine; +
-- SPELL_SACRIFICE_VISUAL 56133 self-cast + Talk(SAY_CHOOSE 1) +
-- random volunteer pick via _volunteerGUIDS + ObjectAccessor::
-- GetCreature + cross-AI DoAction(ACTION_CHOSEN)),
-- EVENT_SUMMON_VOLUNTEER re-summon
-- machine, EVENT_END_PHASE_TWO (summons.DespawnEntry(controller) +
-- DoCastSelf(SPELL_HOVER_FALL_2 56157) + MoveLand(POINT_GROUND)):
-- summon STRAND + motion-master (MovePoint / MoveLand / MoveTakeoff
-- / SetDisableGravity) + flag / interrupt / stand-state + cross-AI
-- DoAction + ObjectAccessor bridges absent — documented-only.
-- DoAction(ACTION_SACRIFICE) (Talk(SAY_SACRIFICE 2) + DoCastAOE(
-- Sacrifice Beam 56150) — the self-cast lowering is port-pattern-
-- ready (arugal precedent), but the arm fires only through the
-- unbridged cross-AI DoAction + phase-two sacrifice machine
-- (gothik precedent) — + EVENT_END_PHASE_TWO re-schedule +
-- EVENT_SUMMON_VOLUNTEER reschedule): no DoAction bridge on the
-- Lua surface (drakkari_colossus standing precedent) —
-- documented-only.
-- JustSummoned (NPC_TWILIGHT_WORSHIPPER 30111 SetStandState(
-- UNIT_STAND_STATE_KNEEL) + summons.Summon — stand-state bridge exists
-- since 2026-10-21, but the arm stays out on the unported summon machine
-- / cross-AI bridges), SummonedCreatureDies (initiate _initiatesKilled == 15
-- latch -> SPELL_HOVER_FALL_1 56100 + EVENT_START_FIGHT_1 1s
-- (RemoveAurasDueToSpell(SPELL_BEAM_VISUAL_JEDOGA 56312) then
-- summons.DespawnEntry(controller) + SetDisableGravity(false) +
-- MoveLand(POINT_GROUND)) and the volunteer _volunteerWork=false
-- latch + PHASE_THREE flip; payload rides the unbridged summon /
-- movement / instance arms), EnterEvadeMode (summons.DespawnAll has
-- no bearer — the _DespawnAtEvade(15s) leg is ported in
-- jedogaLeaveCombat as immediate creature:Despawn (drakkari
-- precedent; C++ despawns immediately, the 15s is respawn-side —
-- ScriptedCreature.cpp:596-616; stale "no despawn bridge"
-- retracted),
-- MovementInform (all point choreography — no motion
-- bridges), GetData(DATA_VOLUNTEER_WORK) (no cross-AI GetData
-- bridge — achievement_volunteer_work needs it too).
-- npc_twilight_volunteer — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (30385 — ahnkahet.h NPC_TWILIGHT_VOLUNTEER, kalecgos pass): zero
-- registration. The entire AI rides absent bridges — DoAction(
-- ACTION_CHOSEN) initiation (no DoAction bridge), SPELL_PILLAR_OF_
-- LIGHTNING 56868 self-cast (port-pattern-ready: creature:CastSpell(
-- creature, spell)), RemoveAurasDueToSpell(SPELL_SPHERE_VISUAL_
-- VOLUNTEER 56102) (the old "no aura-removal bridge" claim was
-- stale — creature:RemoveAura is bridged, lua_creature_events.go:
-- 153 — but the arm fires only through the unbridged DoAction /
-- sacrifice machine, so it stays documented-only), Talk(SAY_CHOSEN
-- 0) /
-- Talk(SAY_SACRIFICED 1), stand-state machine, MovePoint(
-- POINT_SACRIFICE) choreography (no motion bridges), the sacrifice
-- payload (instance->GetCreature(DATA_JEDOGA_SHADOWSEEKER) +
-- cross-AI DoAction(ACTION_SACRIFICE) — instance-script model and
-- DoAction bridges absent), and DespawnOrUnsummon(5s) (stale "no
-- despawn bridge" wording: creature:Despawn() IS bridged, but the
-- arm rides the unbridged MovementInform/sacrifice choreography
-- machine on a zero-registration bearer — terestian precedent).
-- Joins the entry-verifiable-but-
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

local SPELL_SPHERE_VISUAL = 56075            -- boss_jedoga_shadowseeker.cpp:46
local ENTRY_JEDOGA_CONTROLLER = 30181 -- ahnkahet.h:59
local SPELL_BEAM_VISUAL_JEDOGA = 56312 -- boss_jedoga_shadowseeker.cpp:53
local SPELL_RANDOM_LIGHTNING_VISUAL = 56327  -- boss_jedoga_shadowseeker.cpp:43

local function jedogaEnterCombat(event, creature, target)
    -- cpp:162-165 — JustEngagedWith strips the two intro visual auras before
    -- anything else (arm order C++-exact); the stale "no aura-removal
    -- bridge" claim is retracted — creature:RemoveAura is bridged
    -- (engine/world/lua_creature_events.go:155). The SummonCreatureGroup /
    -- phase legs of the same handler stay documented-only.
    creature:RemoveAura(SPELL_SPHERE_VISUAL)
    creature:RemoveAura(SPELL_RANDOM_LIGHTNING_VISUAL)
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

-- C++ boss_jedoga_shadowseeker::Reset (:148-160): the two
-- NPC_JEDOGA_CONTROLLER summons + SPELL_BEAM_VISUAL_JEDOGA casts on
-- Jedoga are PORTED below, IsAlive-guarded (Go event 23 also fires on
-- death; C++ Reset never runs on death). The SummonCreatureGroup
-- (INITIATES), phase, react-state and EVENT_INTRO_SAY legs of the same
-- Reset stay documented-only above.
local function jedogaReset(event, creature)
    if creature:IsAlive() then
        local controller = creature:SummonCreature(ENTRY_JEDOGA_CONTROLLER, 402.7893, -748.5251, 29.39399, 2.024582)
        if controller then
            controller:CastSpell(creature, SPELL_BEAM_VISUAL_JEDOGA)
        end
        controller = creature:SummonCreature(ENTRY_JEDOGA_CONTROLLER, 420.1999, -727.0132, 28.88036, 2.042035)
        if controller then
            controller:CastSpell(creature, SPELL_BEAM_VISUAL_JEDOGA)
        end
    end
end

local function jedogaLeaveCombat(event, creature)
    -- C++ EnterEvadeMode: _DespawnAtEvade(Seconds(15)) (cpp:182-186;
    -- summons.DespawnAll is moot — nothing is modeled) — DespawnOrUnsummon
    -- is immediate at evade, the 15s is respawn-side
    -- (ScriptedCreature.cpp:596-616); drakkari precedent.
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 1, jedogaEnterCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 2, jedogaLeaveCombat)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 23, jedogaReset)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 3, jedogaTargetDied)
RegisterCreatureEvent(ENTRY_JEDOGA_SHADOWSEEKER, 4, jedogaDied)
