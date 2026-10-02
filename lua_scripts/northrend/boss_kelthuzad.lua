-- Kel'Thuzad (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_kelthuzad.cpp
-- (boss_kelthuzad (BossAI, BOSS_KELTHUZAD); npc_kelthuzad_skeleton /
-- npc_kelthuzad_banshee / npc_kelthuzad_abomination (ScriptedAI via the
-- shared npc_kelthuzad_minionAI base); npc_kelthuzad_guardian
-- (ScriptedAI); spell_kelthuzad_chains / spell_kelthuzad_detonate_mana
-- (AuraScripts via RegisterSpellScript); spell_kelthuzad_frost_blast
-- (AuraScript, direct registration); at_kelthuzad_center
-- (AreaTriggerScript); achievement_just_cant_get_enough
-- (AchievementCriteriaScript); AddSC_boss_kelthuzad registers all ten;
-- loader decl 64 / call 259 per northrend_script_loader.cpp — the
-- SIXTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_razuvious(), under the "// Naxxramas" marker; the call
-- after it is AddSC_boss_loatheb()).
-- Entry: 15990 Kel'Thuzad (naxxramas.h NPC_KEL_THUZAD — kalecgos pass;
-- instance_naxxramas.cpp OnCreatureCreate binds NPC_KEL_THUZAD ->
-- KelthuzadGUID and GetGuidData(DATA_KELTHUZAD) returns it; the
-- CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual). The minion/guardian
-- entries (NPC_SKELETON1 16427 / NPC_SKELETON2 23561 / NPC_ABOMINATION1
-- 16428 / NPC_ABOMINATION2 23562 / NPC_BANSHEE1 16429 / NPC_BANSHEE2
-- 23563 / NPC_GUARDIAN 16441) live only in this file's local enum — no
-- NPC_ constants in naxxramas.h, no instance bindings — ScriptName->
-- entry binding is DB-side, so nothing registers for them (grobbulus
-- precedent).
-- Sole-source verified: whole-server-tree grep for "boss_kelthuzad",
-- "npc_kelthuzad_skeleton", "npc_kelthuzad_banshee",
-- "npc_kelthuzad_abomination", "npc_kelthuzad_guardian",
-- "spell_kelthuzad_chains", "spell_kelthuzad_detonate_mana",
-- "spell_kelthuzad_frost_blast", "at_kelthuzad_center",
-- "achievement_just_cant_get_enough", "KelThuzadCharmedPlayerAI" hits
-- boss_kelthuzad.cpp only (loader carries only the decl/call lines);
-- zero sql/ hits. No kelthuzad lua existed.
-- Eluna creature events: 3 OnTargetDied, 4 OnDied. Melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ has no JustEngagedWith override for Kel'Thuzad (SAY_AGGRO 7
-- fires on the EVENT_DESPAWN_MINIONS timer leg, not on combat start),
-- so no event 1 handler. No timers are modeled, so no cancel bookkeeping.
-- Victim gating: C++ KilledUnit checks TYPEID_PLAYER (nalorakk
-- precedent); Lua expresses it via victim:GetObjectType().
-- Ported arms (C++-exact for all modeled arms):
-- boss_kelthuzad: KilledUnit Talk(SAY_SLAY 8) (event 3 — C++-GATED:
-- Talk fires only when victim->GetTypeId() == TYPEID_PLAYER —
-- nalorakk precedent); JustDied Talk(SAY_DEATH 9) (event 4; the
-- JustDied guardian DoAction(ACTION_KELTHUZAD_DIED) cross-AI leg and
-- the _JustDied instance bookkeeping have no bridge — tharon_ja
-- precedent).
-- Unmodeled (no bridges — documented, not wired):
-- SpellHit — SPELL_CHAINS_DUMMY 28408 -> Talk(SAY_CHAINS 10) +
-- SelectTargetList(3, Random) -> DoCast(CHains 28410) — SpellHit never
-- fires in the Lua surface (SpellHit ruling); EVENT_SKELETON /
-- EVENT_BANSHEE / EVENT_ABOMINATION — SummonCreature (summon STRAND
-- absent); EVENT_DESPAWN_MINIONS — summons DespawnOrUnsummon +
-- Talk(SAY_AGGRO 7) — summon/despawn bridges absent (Talk not ported
-- orphaned — vortex precedent); EVENT_PHASE_TWO — CastStop +
-- SetPhase(PHASE_TWO) + NOT_SELECTABLE/immune/ReactState/ThreatList
-- legs + Talk(EMOTE_PHASE_TWO 13) + the five phase-two event schedules
-- — no phase / react-state / threat bridges; EVENT_FROSTBOLT_VOLLEY —
-- DoCastAOE(SPELL_FROSTBOLT_VOLLEY 28479) — no DoCastAOE bridge
-- (terestian/shazzrah precedent); EVENT_SHADOW_FISSURE —
-- SelectTarget(Random, 0, 0.0f, true) -> DoCast(SHADOW_FISSURE 27810) —
-- no random-target SelectTarget bridge (cairne/kazzak precedent);
-- EVENT_DETONATE_MANA — random mana-user SelectTarget ->
-- DoCast(DETONATE_MANA 27819) — no random-target bridge;
-- EVENT_FROST_BLAST — SelectTarget(Random, 1) -> DoCast(FROST_BLAST
-- 27808) — no random-target bridge; EVENT_CHAINS —
-- DoCastAOE(SPELL_CHAINS_DUMMY 28408), 25-man only — no DoCastAOE
-- bridge + no difficulty bridge (kelidan precedent);
-- EVENT_TRANSITION_REPLY — lich-king cross-AI Talk(SAY_ANSWER_REQUEST
-- 3) via DATA_LICH_KING + the four KT portal GO states — instance
-- model / no GO-state bridge; EVENT_TRANSITION_SUMMON —
-- SummonCreatureGroup(_guardianGroups) — no summon bridge;
-- phase-three UpdateAI leg — HealthBelowPct(45) Talk(SAY_REQUEST_AID
-- 12) + transition scheduling — no health-pct bridge
-- (doomwalker/shadow-labyrinth precedents) + summon bridge;
-- UpdateAI frostbolt filler — DoCastVictim(SPELL_FROSTBOLT_SINGLE
-- 28478) — phase-gated on !PHASE_ONE and the UNIT_STATE_CASTING check
-- — no phase / unit-state bridges; DamageTaken — damage = 0 in
-- PHASE_ONE — no phase bridge; Reset — SetReactState(REACT_PASSIVE) +
-- NOT_SELECTABLE + SetImmuneToPC(true) — no react-state / immunity
-- bridges; EnterEvadeMode — portal GO-state legs — instance model;
-- DoAction(ACTION_BEGIN_ENCOUNTER) — instance->SetBossState(BOSS_KELTHUZAD,
-- IN_PROGRESS) + DoCastAOE(SPELL_VISUAL_CHANNEL 29423) +
-- Talk(SAY_SUMMON_MINIONS 14) + SummonCreatureGroup ×7 + minion
-- pocket IDs — instance / summon / SetData bridges absent;
-- DoAction(ACTION_ABOMINATION_DIED) — cross-AI (guardians' entries are
-- DB-side, no registration) — documented-only;
-- GetAIForCharmedPlayer -> KelThuzadCharmedPlayerAI
-- (SimpleCharmedPlayerAI) — no charmed-player AI bridge.
-- npc_kelthuzad_skeleton / npc_kelthuzad_banshee /
-- npc_kelthuzad_abomination (ScriptedAI; entries 16427/23561,
-- 16429/23563, 16428/23562 — entry-unverifiable from C++ evidence,
-- DB-side binding): UpdateRandomMovement hack (no motion bridge) +
-- melee; abomination _woundTimer DoCastVictim(SPELL_MORTAL_WOUND 28467)
-- — port-pattern-ready in isolation (moroes precedent) but
-- entry-blocked — joins the bridgeable-but-entry-blocked queue;
-- abomination JustDied — cross-AI DoAction(ACTION_ABOMINATION_DIED)
-- via DATA_KELTHUZAD — instance / cross-AI bridges absent; minion
-- JustEngagedWith pocket-aggro legs — no react-state / grid bridges;
-- MoveInLineOfSight — no bridge; SetData(DATA_MINION_POCKET_ID) —
-- no bridge. npc_kelthuzad_guardian (16441 — entry-unverifiable,
-- entry-blocked): ACTION_JUST_SUMMONED visibility/react/combat-pulse
-- legs (no visibility/react bridges); UpdateAI _bloodTapTimer
-- DoCastVictim(SPELL_BLOOD_TAP 28470) — moroes-ready in isolation but
-- entry-blocked — joins the bridgeable-but-entry-blocked queue;
-- ACTION_KELTHUZAD_DIED Talk(EMOTE_GUARDIAN_FLEE 0) + flee/
-- DespawnOrUnsummon(30s) / MoveTargetedHome legs — no motion/despawn
-- bridges; EMOTE_GUARDIAN_APPEAR 1 — visibility bridge absent.
-- spell_kelthuzad_chains (AuraScript 28410): scale-mod apply/remove
-- handlers — no AuraScript binding bridge (razelikh precedent) — joins
-- the SpellScript/AuraScript queue.
-- spell_kelthuzad_detonate_mana (AuraScript 27819): periodic mana-drain
-- -> SPELL_MANA_DETONATION_DAMAGE 27820 — no AuraScript binding bridge
-- — joins the SpellScript/AuraScript queue.
-- spell_kelthuzad_frost_blast (AuraScript 27808): PeriodicTick ->
-- SPELL_FROST_BLAST_DMG 29879 with 26%-max-health scaling — no
-- AuraScript binding bridge — joins the SpellScript/AuraScript queue.
-- at_kelthuzad_center (AreaTriggerScript): NOT_STARTED-gated
-- DoAction(ACTION_BEGIN_ENCOUNTER) via DATA_KELTHUZAD — no
-- area-trigger bridge (anubrekhan precedent) — documented-only.
-- achievement_just_cant_get_enough: DATA_ABOMINATION_DEATH_COUNT >= 18
-- via DATA_KELTHUZAD GetData — no achievement bridge — documented-only.

local ENTRY_KEL_THUZAD = 15990

local SAY_SLAY = 8
local SAY_DEATH = 9

-- C++ KilledUnit: Talk(SAY_SLAY) when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk precedent).
local function kelthuzadTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function kelthuzadDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_KEL_THUZAD, 3, kelthuzadTargetDied)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 4, kelthuzadDied)
