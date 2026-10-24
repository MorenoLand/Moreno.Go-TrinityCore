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
-- has no bridge — tharon_ja precedent). The DoCastSelf(
-- SPELL_DEATH_SPELL 50415, true) leg STAYS OUT, decided 2026-10-14:
-- the triggered cast bridge exists (CastSpell(creature, id, true)),
-- but the engine's creature-cast path has no SPELL_EFFECT_SUMMON
-- (28) delivery leg (castCreatureSpellInternal, boss_ai.go: GO
-- packet + effect-50 Transmitted + spell_linked + event hooks only)
-- — the shazzrah no-op-cast precedent.
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
-- bridge (summon STRAND absent)).
-- Reset — SetControlled(false, UNIT_STATE_ROOT) ports via
-- creature:SetRooted(false) (uromReset); SetDisableGravity(false) has
-- no bridge and SetReactState(REACT_AGGRESSIVE) is vacuous here (Lua
-- never sets REACT_PASSIVE); the ungated DoCastSelf(
-- SPELL_EVOCATE 51602) leg is ported on OnReset(23) (the Reset bullet's
-- earlier "no cast bridge" line was stale — creature:CastSpell is
-- established in-tree); _Reset() has no bridge.
-- JustReachedHome — DoCastSelf(SPELL_EVOCATE 51602) — ported below
-- on event 24 (this tick; the old "no cast bridge" note was stale —
-- the Reset leg's port above already uses the established CastSpell
-- bridge).
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
-- DoCast(SPELL_TIME_BOMB 51121) 20s repeat — timer-event and cast
-- ARE bridged (this file ports Reset/JustReachedHome casts on
-- events 23/24), but the arm is gated on the center _platform leg
-- (:189-219 summon x4 + DoCast(TeleportSpells) cycle) with
-- teleport / react-state / gravity / controlled-state legs — no
-- bridges; unmodeled.
-- DamageTaken — _isInCenter-gated NearTeleportTo(1124.0432,
-- 1078.2109, 508.3597, 5.4623) on lethal damage — state- and
-- health-gated, no bridges.
-- SpellHit — SPELL_SUMMON_MENAGERIE(3) x3: SetHomePosition (three
-- platform coords) + LeaveCombat (RemoveAllAuras + CombatStop +
-- EngagementOver) + DoCastSelf(SPELL_EVOCATE 51602, true) — STAY-
-- OUT, decided 2026-10-14, re-keyed 2026-10-22: the arm's
-- LeaveCombat-teardown purpose is unbridged. creature:AttackStop()
-- was bridged 2026-10-22 (8feec4cd) but is NOT a substitute — the C++
-- arm needs full CombatStop (TargetGUID + ThreatMgr + InCombat clear,
-- Unit.cpp:5809; AttackStop stops the swing only) plus the
-- EngagementOver evade machine, both still unbridged (19:29 demand
-- verdict: zero clean CombatStop callers, bridge not built). No
-- aura-application bridge on the Lua creature object, and the trigger
-- path (StartAttack's self-cast of the menagerie spells) is itself
-- unported, so event 14 could never fire these IDs. SetHomePosition
-- IS bridged (lua_creature_events.go) but a lone home-position write
-- without the teardown would strand the add in combat.
-- spell_urom_frostbomb — AuraScript OnPeriodic (EFFECT_1 SPELL_AURA_
-- PERIODIC_DAMAGE): heroic + IsInCombat gate -> caster CastSpell(
-- target, SPELL_FROST_BUFFET 58025) — no AuraScript bridge (the
-- keristrasza spell_intense_cold precedent); joins the
-- no-AuraScript-bridge queue.

local ENTRY_UROM = 27655

local SAY_PLAYER_KILL = 7
local SAY_DEATH = 6

local SPELL_EVOCATE = 51602

-- C++ KilledUnit: Talk(SAY_PLAYER_KILL) only when who->GetTypeId()
-- == TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function uromTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_PLAYER_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent; the DoCastSelf(SPELL_DEATH_SPELL
-- 50415, true) leg stays out — no SPELL_EFFECT_SUMMON delivery leg
-- on the engine's creature-cast path, shazzrah precedent).
local function uromDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

-- C++ Reset (boss_urom.cpp:132-140): the SetDisableGravity(false) leg is
-- PORTED in uromReset below (bridge live, lua_creature_events.go:800),
-- and SetReactState(REACT_AGGRESSIVE) is vacuous here (Lua never sets
-- REACT_PASSIVE), but the DoCastSelf(SPELL_EVOCATE) leg is ungated and
-- port-pattern-ready — self-cast on the reset path (halion
-- twilightHalionReset precedent). The root leg ports via
-- creature:SetRooted(false) (lua_creature_events.go).
local function uromReset(event, creature)
    -- C++ boss_uromAI::Reset (boss_urom.cpp:132-139): _isInCenter=false,
    -- me->SetControlled(false, UNIT_STATE_ROOT) (ports via
    -- creature:SetRooted(false) below), me->SetDisableGravity(false)
    -- (:136), SetReactState(REACT_AGGRESSIVE) (vacuous — Lua never sets
    -- REACT_PASSIVE, documented above), DoCastSelf(SPELL_EVOCATE)
    -- (Reset-only, alive-gated; the death-leg ev23 fire is phantom),
    -- _Reset() bookkeeping.
    creature:SetRooted(false)
    creature:SetDisableGravity(false)
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_EVOCATE)
    end
end

-- C++ JustReachedHome (boss_urom.cpp:142-145): DoCastSelf(SPELL_EVOCATE)
-- — the same ungated self-cast as the Reset leg, fired at the home-move
-- completion (C++ HomeMovementGenerator::DoFinalize). Non-triggered,
-- matching the Reset port.
local function uromReachHome(event, creature)
    creature:CastSpell(creature, SPELL_EVOCATE)
end

RegisterCreatureEvent(ENTRY_UROM, 3, uromTargetDied)
RegisterCreatureEvent(ENTRY_UROM, 4, uromDied)
RegisterCreatureEvent(ENTRY_UROM, 23, uromReset)
RegisterCreatureEvent(ENTRY_UROM, 24, uromReachHome)
