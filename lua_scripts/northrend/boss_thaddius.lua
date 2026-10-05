-- Thaddius (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (boss_thaddius (BossAI, BOSS_THADDIUS); npc_stalagg /
-- npc_feugen / npc_tesla (ScriptedAI); spell_thaddius_polarity_charge /
-- spell_thaddius_polarity_shift / spell_thaddius_magnetic_pull
-- (SpellScripts via SpellScriptLoader); at_thaddius_entrance
-- (OnlyOnceAreaTriggerScript); achievement_thaddius_shocking
-- (AchievementCriteriaScript); AddSC_boss_thaddius registers all nine;
-- loader decl 73 / call 268 per northrend_script_loader.cpp — the
-- FIFTEENTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_gothik() (decl 72 / call 267), under the "// Naxxramas"
-- marker; the call after it is AddSC_naxxramas() (decl 74 / call 269),
-- then AddSC_instance_naxxramas() (decl 75 / call 270) — loader order
-- confirmed this run; the checkpoint sequence (gothik -> thaddius) is
-- followed).
-- Entry: 15928 Thaddius (naxxramas.h NPC_THADDIUS line 98 — kalecgos
-- pass; instance_naxxramas.cpp binds NPC_THADDIUS -> ThaddiusGUID
-- (line 164) and DATA_THADDIUS -> ThaddiusGUID (line 324); the
-- CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the nine
-- script names hits boss_thaddius.cpp only (loader carries only the
-- decl/call lines); zero sql/ hits. No thaddius lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. Victim
-- gating: C++ KilledUnit checks TYPEID_PLAYER — Lua expresses it via
-- victim:GetObjectType() (nalorakk / kelthuzad / gothik player-gate
-- precedent). C++ has no JustEngagedWith override for Thaddius —
-- SAY_AGGRO (1) fires from EVENT_TRANSITION_3, not from combat start —
-- so no event 1 handler here. There are no modelable timers:
-- EVENT_SHIFT is DoCastAOE (no bridge), and EVENT_CHAIN /
-- EVENT_BERSERK are scheduled only from EVENT_TRANSITION_3, which
-- belongs to the unbridged phase machine (gothik's EVENT_BOLT
-- trigger-blocked precedent — porting them from combat start would
-- deviate from C++: pre-transition Thaddius casts nothing).
-- Ported arms (C++-exact for all modeled arms):
-- boss_thaddius: KilledUnit Talk(SAY_SLAY 2) (event 3 — C++-GATED:
-- victim->GetTypeId() == TYPEID_PLAYER — nalorakk/kelthuzad/gothik
-- player-gate variant; fifth ported variant of this gate); JustDied
-- Talk(SAY_DEATH 4) (event 4; the _JustDied bookkeeping +
-- setActive/SetFarVisible + the stalagg/feugen cross-AI legs via
-- DATA_STALAGG/DATA_FEUGEN have no bridge — tharon_ja precedent).
-- SAY_GREET (0) fires from at_thaddius_entrance — no area-trigger
-- bridge — documented-only. SAY_ELECT (3) + EMOTE_POLARITY_SHIFTED (6)
-- ride the unbridged EVENT_SHIFT leg (vortex precedent).
-- Unmodeled (no bridges — documented, not wired):
-- boss_thaddius: InitializeAI SetPhase(PHASE_NOT_ENGAGED) +
-- SetCombatMovement(false) — no phase / combat-movement bridges;
-- Reset() — empty in C++; EnterEvadeMode — ballLightningEnabled /
-- PHASE_TRANSITION/PHASE_THADDIUS legs + BeginResetEncounter — no
-- phase / instance bridges; CanAIAttack — the
-- ballLightningEnabled/IsWithinMeleeRange gate — no bridge;
-- JustAppeared — ResetEncounter (phase reset + REACT_PASSIVE +
-- respawn of spawn IDs 130958/130959) — instance model — no bridge;
-- DoAction — the whole cross-AI machine (ACTION_FEUGEN_RESET/
-- ACTION_STALAGG_RESET -> BeginResetEncounter; ACTION_FEUGEN_AGGRO/
-- ACTION_STALAGG_AGGRO -> SetPhase(PHASE_PETS) + boss-state
-- IN_PROGRESS + setActive/SetFarVisible on thaddius/stalagg/feugen;
-- ACTION_FEUGEN_DIED/ACTION_STALAGG_DIED -> cross-AI reviving-FX
-- action + 5s EVENT_REVIVE_FEUGEN/EVENT_REVIVE_STALAGG or
-- Transition(); ACTION_POLARITY_CROSSED -> shockingEligibility=false)
-- — no cross-AI / instance / phase bridges; Transition() —
-- SetPhase(PHASE_TRANSITION) + NOT_SELECTABLE removal + the
-- EVENT_TRANSITION_1/2/3 schedules — no phase / flag bridges;
-- BeginResetEncounter — DoRemoveAurasDueToSpellOnPlayers(28059/
-- 28084) + DespawnOrUnsummon + flag/immunity/active legs + cross-AI
-- ACTION_BEGIN_RESET_ENCOUNTER — instance model — no bridge;
-- EVENT_REVIVE_FEUGEN / EVENT_REVIVE_STALAGG — cross-AI DoAction —
-- no bridge; EVENT_TRANSITION_1 — cross-AI ACTION_TRANSITION —
-- no bridge; EVENT_TRANSITION_2 — CastSpell(me, THADDIUS_SPARK_
-- VISUAL 28136, triggered) + cross-AI ACTION_TRANSITION_2 — no
-- cross-AI bridge; EVENT_TRANSITION_3 — spark visual + aura/flag/
-- immunity legs + DoZoneInCombat + cross-AI ACTION_TRANSITION_3 +
-- SetPhase(PHASE_THADDIUS) + Talk(SAY_AGGRO 1) + the
-- EVENT_ENGAGE/EVENT_ENABLE_BALL_LIGHTNING/EVENT_SHIFT/EVENT_CHAIN/
-- EVENT_BERSERK schedules — no phase / flag / cross-AI bridges, so
-- SAY_AGGRO rides the unportable leg (vortex — not ported orphaned);
-- EVENT_ENABLE_BALL_LIGHTNING / EVENT_ENGAGE — no react-state /
-- phase bridges; EVENT_SHIFT — CastStop + DoCastAOE(SPELL_POLARITY_
-- SHIFT 28089) + the EVENT_SHIFT_TALK/EVENT_SHIFT schedules — no
-- DoCastAOE bridge (terestian/shazzrah precedent); EVENT_SHIFT_TALK
-- — Talk(SAY_ELECT 3) + Talk(EMOTE_POLARITY_SHIFTED 6) — rides the
-- unportable leg (vortex); EVENT_CHAIN — DoCastVictim(SPELL_CHAIN_
-- LIGHTNING 28167), 10s/20s init, randtime(10s,20s) repeat with the
-- polarity-shift-cast delay leg — moroes-ready in isolation but
-- trigger-blocked behind the phase machine (gothik precedent —
-- documented-only); EVENT_BERSERK — DoCast(me, SPELL_BERSERK 27680),
-- 6min — phase_hunter-ready in isolation but trigger-blocked behind
-- the phase machine — documented-only; the UpdateAI ball-lightning
-- leg — SelectTarget(Random) -> DoCast(SPELL_BALL_LIGHTNING 28299)
-- when out of melee range and ballLightningUnlocked — no phase /
-- ball-lightning-state bridges — documented-only.
-- The remaining scripts (npc_stalagg / npc_feugen / npc_tesla /
-- spell_thaddius_polarity_charge / spell_thaddius_polarity_shift /
-- spell_thaddius_magnetic_pull / at_thaddius_entrance /
-- achievement_thaddius_shocking) are documented in npc_stalagg.lua,
-- npc_feugen.lua and below: tesla's DamageTaken zeroing (no damage
-- bridge); the three SpellScripts (no SpellScript binding bridge —
-- razelikh precedent — SpellScript/AuraScript queue); the area
-- trigger (no area-trigger bridge); the achievement (no achievement
-- bridge — kelthuzad precedent).

local ENTRY_THADDIUS = 15928

local SAY_SLAY = 2
local SAY_DEATH = 4

-- C++ KilledUnit: Talk(SAY_SLAY 2) only when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad / gothik player-gate variant).
local function thaddiusTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH 4) (the _JustDied + setActive/
-- SetFarVisible + the stalagg/feugen cross-AI legs via
-- DATA_STALAGG/DATA_FEUGEN have no bridge — tharon_ja precedent).
local function thaddiusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_THADDIUS, 3, thaddiusTargetDied)
RegisterCreatureEvent(ENTRY_THADDIUS, 4, thaddiusDied)
