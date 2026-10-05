-- Heigan the Unclean (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_heigan.cpp
-- (boss_heigan (BossAI); spell_heigan_eruption (SpellScriptLoader);
-- achievement_safety_dance (AchievementCriteriaScript);
-- AddSC_boss_heigan registers all three; loader decl 71 / call 266 per
-- northrend_script_loader.cpp — the THIRTEENTH Naxxramas group in
-- AddNorthrendScripts(), under the "// Naxxramas" marker; the call
-- before it is AddSC_boss_faerlina() (decl 70 / call 265), the call
-- after is AddSC_boss_gothik() (decl 72 / call 267 — lua exists,
-- audit-and-close when its group comes up)).
-- Entry: 15936 Heigan the Unclean (naxxramas.h NPC_HEIGAN :97 —
-- kalecgos pass; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-scripts-tree grep for "boss_heigan" /
-- "spell_heigan_eruption" / "achievement_safety_dance" hits
-- boss_heigan.cpp only (loader carries only the decl/call lines);
-- zero sql/ hits. No heigan lua existed. Talk() count = 6
-- grep-confirmed (zero spaced "Talk (" variant) — ALL SIX accounted
-- below. BOSS_HEIGAN (naxxramas.h :34), DATA_HEIGAN (:73).
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- Melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady (jedoga bar — engagement-only leg,
-- documented). No timers are scheduled by this port: every C++
-- scheduled arm rides an unbridged leg (below), so there is nothing to
-- cancel on 2/4/23 (gargolmar precedent — vacuous here).
-- Ported arms (C++-exact for all modeled arms):
-- boss_heigan: JustEngagedWith Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg, the eruption-tile
-- GO enumeration, and the four event schedules all ride unbridged
-- legs — tharon_ja precedent); KilledUnit Talk(SAY_SLAY 1) (event 3 —
-- C++-GATED: the Talk fires only when victim->GetTypeId()==
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType() ==
-- "Player" (nalorakk / kelthuzad player-gate precedent); the
-- _safetyDance=false latch has no GetData bridge — documented
-- below); JustDied Talk(SAY_DEATH 3) (event 4; _JustDied instance
-- bookkeeping has no bridge — tharon_ja precedent).
-- Unmodeled (no bridges — documented, not wired):
-- EVENT_DISRUPT (init randtime(15s,20s), repeat 11s) —
-- DoCastAOE(SPELL_SPELL_DISRUPTION 29310) — no DoCastAOE bridge
-- (terestian/shazzrah precedent).
-- EVENT_FEVER (init randtime(10s,20s), repeat randtime(20s,25s)) —
-- DoCastAOE(SPELL_DECREPIT_FEVER 29998, 25-man 55011) — no DoCastAOE
-- bridge + no difficulty bridge (kelidan precedent).
-- EVENT_DANCE (90s, PHASE_FIGHT-only) — the whole phase machine rides
-- unbridged legs: SetPhase(PHASE_DANCE), SetReactState(REACT_PASSIVE)
-- (no react-state bridge), AttackStop/StopMoving (no motion bridges),
-- DoCast(SPELL_TELEPORT_SELF 30211), DoCastAOE(SPELL_PLAGUE_CLOUD
-- 29350) (no DoCastAOE bridge) — so Talk(SAY_TAUNT 2) and
-- Talk(EMOTE_DANCE 4) join it documented-only: arming the Talks alone
-- would fire taunts outside the dance context (genuinely-bridgeable
-- bar fails — king_dred RAPTOR_CALL precedent).
-- EVENT_DANCE_END (45s after dance start, PHASE_DANCE-only) —
-- Talk(EMOTE_DANCE_END 5) rides the same unbridged phase machine
-- (CastStop / SetReactState(REACT_AGGRESSIVE) / DoZoneInCombat have
-- no bridges) — documented-only.
-- EVENT_ERUPT (init 15s, rescheduled 10s in dance / 15s out) — the
-- entire arm rides GO bridges: TeleportCheaters, _eruptTiles
-- populated from the map GO-by-spawnId store (firstEruptionDBGUID
-- 84980, 4 sections of 15/25/23/13), ObjectAccessor GO lookup +
-- tile->CastSpell(trap spell) — no GO-spawnId-store / ObjectAccessor /
-- GO-cast bridges — documented-only.
-- Reset's SetReactState(REACT_AGGRESSIVE) (no react-state bridge) +
-- _Reset() (instance bookkeeping — tharon_ja precedent).
-- GetData(DATA_SAFETY_DANCE 19962139) + the _safetyDance latch — no
-- GetData bridge (drakkari_colossus precedent).
-- spell_heigan_eruption (SpellScriptLoader, eruption lethal-hit ->
-- instance GetGuidData(DATA_HEIGAN) -> AI()->KilledUnit) — no
-- SpellScript binding bridge (razelikh precedent) + instance /
-- cross-AI bridges absent.
-- achievement_safety_dance — no achievement-criteria bridge (snakes
-- precedent) + cross-AI GetData absent.

local ENTRY_HEIGAN = 15936

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 3

local function heiganEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

local function heiganTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function heiganDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_HEIGAN, 1, heiganEnterCombat)
RegisterCreatureEvent(ENTRY_HEIGAN, 3, heiganTargetDied)
RegisterCreatureEvent(ENTRY_HEIGAN, 4, heiganDied)
