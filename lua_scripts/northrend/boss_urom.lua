-- Mage-Lord Urom (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_urom.cpp
-- (boss_urom (CreatureScript) via GetOculusAI<boss_uromAI>
-- (BossAI, DATA_UROM); spell_urom_frostbomb (AuraScript, registered
-- from inside AddSC_boss_urom via RegisterSpellScript, no separate
-- loader lines); AddSC_boss_urom() at end registers both; loader
-- decl 85 / call 280 per northrend_script_loader.cpp — the SECOND
-- group of the "// The Nexus: The Oculus" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_drakos() (decl
-- 84 / call 279); the call after it is AddSC_boss_varos() — loader
-- order confirmed this run; the checkpoint sequence (drakos ->
-- urom) is followed).
-- Entry: 27655 Mage-Lord Urom (oculus.h NPC_UROM, line 43;
-- instance_oculus.cpp OnCreatureCreate binds case NPC_UROM (line
-- 67) — entry-verifiable, registration proceeds (the
-- nexus_commanders kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual. The instance leg also
-- gates SetPhaseMask(1, true) on GetBossState(DATA_VAROS) == DONE —
-- no instance bridge, documented only.
-- Sole-source verified: whole-server-tree grep for "boss_urom"
-- hits boss_urom.cpp + the loader decl/call lines only; grep for
-- "spell_urom_frostbomb" hits boss_urom.cpp only (registered from
-- inside AddSC_boss_urom, no separate loader lines); zero sql/
-- hits. No urom lua existed.
-- Eluna creature events: 3 OnTargetDied, 4 OnDied. No timers in the
-- ported arms; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- KilledUnit — Talk(SAY_PLAYER_KILL 7) (event 3 — C++-GATED
-- who->GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad
-- player-gate variant — ninth ported variant overall, sixth
-- identical to nalorakk / kelthuzad / gothik / thaddius /
-- keristrasza).
-- JustDied — Talk(SAY_DEATH 6) (event 4; the _JustDied bookkeeping
-- has no bridge — tharon_ja precedent; the DoCastSelf(
-- SPELL_DEATH_SPELL) leg has no cast bridge).
-- Unmodeled (no bridges — documented, not wired):
-- JustEngagedWith / StartAttack — the Talk arms are _platform-
-- state-gated, not stateless: on the first three engages
-- (_platform 0/1/2) Talk(SAY_SUMMON_1..3 0..2) + SummonCreature x4
-- (Group[_group[_platform]] phantasmal entries) + DoCast(
-- TeleportSpells[_platform]) (the platform leg); only on the fourth
-- engage (_platform 3 > 2) does Talk(SAY_AGGRO 3) + the event
-- schedule fire (the center leg). Porting either Talk uncondi-
-- tionally would be C++-inexact, so both are documented-only here
-- (no _platform / random-shuffle-group state bridge, no summon
-- bridge (summon STRAND absent), no cast bridge).
-- Reset — SetControlled(false, UNIT_STATE_ROOT) + SetDisableGravity
-- (false) + SetReactState(REACT_AGGRESSIVE) + DoCastSelf(
-- SPELL_EVOCATE 51602) + _Reset() — no react-state / gravity /
-- control / cast / instance bridges.
-- JustReachedHome — DoCastSelf(SPELL_EVOCATE 51602) — no cast
-- bridge.
-- EnterEvadeMode — gated on _platform > 2 (center only): _EnterEvade-
-- Mode + NearTeleportTo(1118.3101, 1080.3800, 508.3610, 4.25) or
-- MoveTargetedHome + Reset + events.Reset — no evade / teleport /
-- motion bridges.
-- AttackStart — z-position-gated DoStartNoMovement vs BossAI::
-- AttackStart — no position / movement bridges.
-- UpdateAI — the full teleport event machine: EVENT_TELEPORT (30s)
-- DelayEvents(10s) + REACT_PASSIVE + AttackStop + StopMoving +
-- SetDisableGravity(true) + SetCanFly(true) + SetControlled(true,
-- ROOT) + DoCast(SPELL_TELEPORT 51112); EVENT_CAST_EXPLOSION (2s)
-- Talk(EMOTE_ARCANE_EXPLOSION 4) + Talk(SAY_ARCANE_EXPLOSION 5) +
-- DoCastAOE(SPELL_EMPOWERED_ARCANE_EXPLOSION 51110); EVENT_TELEPORT_
-- BACK (DUNGEON_MODE 10s/8s) re-aggressive + NearTeleportTo(victim)
-- + AttackStart; EVENT_FROST_BOMB (5s) DoCastVictim(SPELL_FROSTBOMB
-- 51103) 5s repeat; EVENT_TIME_BOMB (20s) SelectTarget(Random) ->
-- DoCast(SPELL_TIME_BOMB 51121) 20s repeat — no timer-event / random-
-- target / cast / teleport / react-state / gravity bridges.
-- DamageTaken — _isInCenter-gated NearTeleportTo(1124.0432,
-- 1078.2109, 508.3597, 5.4623) on lethal damage — state- and
-- health-gated, no bridges.
-- SpellHit — SPELL_SUMMON_MENAGERIE(3) x3: SetHomePosition (three
-- platform coords) + LeaveCombat (RemoveAllAuras + CombatStop +
-- EngagementOver) + DoCastSelf(SPELL_EVOCATE 51602) — no SpellHit /
-- home-position / aura / cast bridges (joins the SpellHit-15-never-
-- fires queue with the other never-bridged SpellHit handlers).
-- spell_urom_frostbomb — AuraScript OnPeriodic (EFFECT_1 SPELL_AURA_
-- PERIODIC_DAMAGE): heroic + IsInCombat gate -> caster CastSpell(
-- target, SPELL_FROST_BUFFET 58025) — no AuraScript bridge (the
-- keristrasza spell_intense_cold precedent); joins the
-- no-AuraScript-bridge queue.

local ENTRY_UROM = 27655

local SAY_PLAYER_KILL = 7
local SAY_DEATH = 6

-- C++ KilledUnit: Talk(SAY_PLAYER_KILL) only when who->GetTypeId()
-- == TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function uromTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_PLAYER_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent; the DoCastSelf(SPELL_DEATH_SPELL)
-- leg has no cast bridge).
local function uromDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_UROM, 3, uromTargetDied)
RegisterCreatureEvent(ENTRY_UROM, 4, uromDied)
