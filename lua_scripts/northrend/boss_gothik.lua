-- Gothik the Harvester (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_gothik.cpp
-- (boss_gothik (BossAI, BOSS_GOTHIK); npc_gothik_minion_livingtrainee /
-- npc_gothik_minion_livingknight / npc_gothik_minion_livingrider /
-- npc_gothik_minion_spectraltrainee / npc_gothik_minion_spectralknight /
-- npc_gothik_minion_spectralrider / npc_gothik_minion_spectralhorse
-- (ScriptedAI via shared npc_gothik_minion_baseAI); npc_gothik_trigger
-- (ScriptedAI); spell_gothik_shadow_bolt_volley (SpellScript via
-- SpellScriptLoader); AddSC_boss_gothik registers all ten; loader
-- decl 72 / call 267 per northrend_script_loader.cpp — the FOURTEENTH
-- Naxxramas group under the "// Naxxramas" block comment (:58/:253),
-- immediately after AddSC_boss_heigan() (decl 71 / call 266); the call
-- after it is AddSC_boss_thaddius() (decl 73 / call 268) — loader order
-- verified in the audit run.
-- Entry: 16060 Gothik the Harvester (naxxramas.h NPC_GOTHIK — kalecgos
-- pass; instance_naxxramas.cpp binds NPC_GOTHIK -> GothikGUID and
-- DATA_GOTHIK -> GothikGUID; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- The seven minion entries (16124/16125/16126 living, 16127/16148/
-- 16150/16149 spectral) + NPC_TRIGGER 16137 exist only in the .cpp's
-- local Creatures enum — no NPC_ constants in naxxramas.h, no instance
-- bindings — entry-unverifiable from C++ evidence, no registration,
-- no lua.
-- Sole-source verified: whole-server-tree grep for each of the ten
-- script names hits boss_gothik.cpp only (loader carries only the
-- decl/call lines); zero sql/ hits. No gothik lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. Victim gating: C++ KilledUnit checks
-- TYPEID_PLAYER — Lua expresses it via victim:GetObjectType()
-- (nalorakk / kelthuzad player-gate precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_gothik: JustEngagedWith Talk(SAY_INTRO_1 0) (event 1;
-- BossAI::JustEngagedWith instance-bookkeeping leg has no bridge —
-- tharon_ja precedent; the EVENT_SUMMON / EVENT_DOORS_UNLOCK /
-- EVENT_PHASE_TWO scheduling legs are documented below); EVENT_INTRO_2
-- Talk(SAY_INTRO_2 1) 4s, EVENT_INTRO_3 Talk(SAY_INTRO_3 2) 9s,
-- EVENT_INTRO_4 Talk(SAY_INTRO_4 3) 14s — one-shot timers scheduled
-- from JustEngagedWith with no phase mask, like C++; KilledUnit
-- Talk(SAY_KILL 6) (event 3 — C++-GATED: victim->GetTypeId() ==
-- TYPEID_PLAYER — nalorakk/kelthuzad player-gate variant); JustDied
-- Talk(SAY_DEATH 5) (event 4; the _JustDied bookkeeping + the
-- DATA_GOTHIK_GATE GO_STATE_ACTIVE leg have no bridge — tharon_ja
-- precedent). All timers cancelled on 2/4/23 (gargolmar precedent).
-- SAY_PHASE_TWO (4) + EMOTE_PHASE_TWO (7) + EMOTE_GATE_OPENED (8)
-- ride unported legs (phase-two / gate machines) — not ported
-- orphaned (vortex precedent).
-- Unmodeled (no bridges — documented, not wired):
-- boss_gothik: Reset — SetReactState(REACT_PASSIVE) + instance->
-- SetData(DATA_GOTHIK_GATE, GO_STATE_ACTIVE) — no react-state / GO-
-- state / instance bridges; JustSummoned / SummonedCreatureDespawn —
-- no summon bridge; EVENT_SUMMON — the waves10/waves25 wave tables +
-- CGUID_TRIGGER (127618) spawn-offset layout (rider center-back/north,
-- knight north/center-front/south, trainee south/center-front/north)
-- + DoSummon via trigger spawn IDs — no summon bridge + RAID_MODE
-- difficulty gate (kelidan precedent); EVENT_DOORS_UNLOCK —
-- summon-iteration gate machine — no summon bridge; EVENT_PHASE_TWO
-- — SetPhase(PHASE_TWO) + SetReactState(REACT_PASSIVE) +
-- ResetThreatList + DoCastAOE(SPELL_TELEPORT_LIVE 28026) 2-arg
-- (non-triggered) + Talk(
-- SAY_PHASE_TWO 4) + Talk(EMOTE_PHASE_TWO 7) + the EVENT_TELEPORT/
-- EVENT_HARVEST/EVENT_RESUME_ATTACK schedules — stays OUT: no phase /
-- react-state / threat-list bridges (the Oct-12 "no DoCastAOE bridge"
-- claim is superseded by the phase-machine verdict);
-- EVENT_TELEPORT — HealthBelowPct(30) gate + SetReactState +
-- ResetThreatList + DoCastAOE(SPELL_TELEPORT_LIVE/SPELL_TELEPORT_DEAD
-- 28026/28025) — stays OUT: no health-pct / react-state / threat
-- bridges;
-- EVENT_HARVEST — DoCastAOE(SPELL_HARVEST_SOUL 28679, triggered),
-- 15s init/repeat — stays OUT: scheduled only in PHASE_TWO via
-- EVENT_PHASE_TWO (:502-506) behind the unbridged wave machine; the
-- phase machine has no bridge (porting from combat start would
-- fabricate phase-one casts — the Oct-12 "no DoCastAOE bridge" claim
-- is superseded by the phase-gating verdict); EVENT_BOLT —
-- DoCastVictim(SPELL_SHADOW_BOLT 29317), 2s repeat — moroes-ready in
-- isolation but its only scheduling legs are EVENT_RESUME_ATTACK from
-- the unbridged EVENT_PHASE_TWO / EVENT_TELEPORT machine (porting it
-- from combat start would deviate from C++ — phase-one Gothik casts
-- no shadow bolts) — documented-only; EVENT_RESUME_ATTACK —
-- SetReactState(REACT_AGGRESSIVE) — no bridge; DoAction(
-- ACTION_MINION_EVADE) + OpenGate — instance SetData + summon-list
-- iteration + cross-AI DoAction — no instance / summon / cross-AI
-- bridges; DamageTaken — damage = 0 while not in PHASE_TWO — no
-- phase bridge; EnterEvadeMode — NearTeleportTo(home) — no motion
-- bridge; the UpdateAI living/dead-side check (RectangleBoundary
-- livingSide/deadSide) + FindEligibleTarget player scan — no side /
-- visibility / threat bridges.
-- npc_gothik_minion_baseAI (shared): DamageTaken zeroing while the
-- gate is closed and the attacker is on the other side — no bridges;
-- DoAction(ACTION_GATE_OPENED/ACTION_ACQUIRE_TARGET) — gate state +
-- FindEligibleTarget — no bridges; EnterEvadeMode — cross-AI
-- DoAction(ACTION_MINION_EVADE) on Gothik via DATA_GOTHIK — no
-- instance / cross-AI bridges; JustDied — DoCastAOE(_deathNotify,
-- triggered) anchor spells (27892/27928/27935) — stays OUT: the
-- entries are local-enum-only (boss_gothik.cpp:94-100), entry-
-- unverifiable, no Go registration (npc_stinky_icc precedent).
-- npc_gothik_minion_livingtrainee (16124): DoCastAOE(SPELL_DEATH_
-- PLAGUE 55604) 2-arg (non-triggered), urandms(5,20)=5-20s
-- init/repeat — stays OUT: entry-blocked (same local-enum bar; the
-- Oct-12 "no DoCastAOE bridge" claim is superseded).
-- npc_gothik_minion_livingknight (16125): DoCastAOE(SPELL_SHADOW_MARK
-- 27825) 2-arg (non-triggered), urandms(5,10)=5-10s init,
-- urandms(15,20)=15-20s repeat — stays OUT: entry-blocked.
-- npc_gothik_minion_livingrider (16126): DoCastAOE(SPELL_SHADOW_BOLT_
-- VOLLEY 27831) 2-arg (non-triggered), urandms(5,10)=5-10s init,
-- urandms(10,15)=10-15s repeat — stays OUT: entry-blocked.
-- npc_gothik_minion_spectraltrainee (16127): DoCastAOE(SPELL_ARCANE_
-- EXPLOSION 27989) 2-arg (non-triggered), 2s init/repeat — stays OUT:
-- entry-blocked.
-- npc_gothik_minion_spectralknight (16148): DoCastAOE(SPELL_WHIRLWIND
-- 56408) 2-arg (non-triggered), urandms(15,25)=15-25s init,
-- urandms(20,25)=20-25s repeat — stays OUT: entry-blocked.
-- npc_gothik_minion_spectralrider (16150): Unholy Frenzy (55648/
-- RAID_MODE 27995) priority cast on a friendly knight > rider > horse
-- > Gothik without the buff (DoFindFriendlyMissingBuff) — no
-- targeted-friendly-cast bridge; DoCastVictim(SPELL_DRAIN_LIFE 27994),
-- urandms(8,12) = 8-12s init, urandms(10,15) = 10-15s repeat —
-- moroes-ready in isolation but entry-blocked — joins the
-- bridgeable-but-entry-blocked queue.
-- npc_gothik_minion_spectralhorse (16149): DoCastAOE(SPELL_STOMP
-- 27993) 2-arg (non-triggered), urandms(10,15)=10-15s init,
-- urandms(14,18)=14-18s repeat — stays OUT: entry-blocked.
-- npc_gothik_trigger (16137): SpellHit anchor machine (ANCHOR_1_x ->
-- ANCHOR_2_x -> SKULLS_x -> DoSummon) — event 14 fires but the arm
-- roots at a DoSummon skull-pile selection via CGUID_TRIGGER +
-- urand(8,12) — no summon bridge; stays OUT.
-- spell_gothik_shadow_bolt_volley (SpellScript 27831: FilterTargets
-- removes shadow-marked targets) — no SpellScript binding bridge
-- (razelikh precedent) — joins the SpellScript/AuraScript queue.

local ENTRY_GOTHIK = 16060

local SAY_INTRO_1 = 0
local SAY_INTRO_2 = 1
local SAY_INTRO_3 = 2
local SAY_INTRO_4 = 3
local SAY_DEATH = 5
local SAY_KILL = 6

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ JustEngagedWith: Talk(SAY_INTRO_1) + schedules EVENT_INTRO_2
-- (4s), EVENT_INTRO_3 (9s), EVENT_INTRO_4 (14s). The EVENT_SUMMON /
-- EVENT_DOORS_UNLOCK / EVENT_PHASE_TWO scheduling legs are unbridged
-- (summon / phase bridges absent) — documented, not wired.
local function gothikEnterCombat(event, creature, target)
    creature:Talk(SAY_INTRO_1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "intro2", 4000, function()
        creature:Talk(SAY_INTRO_2)
    end)
    schedule(guid, "intro3", 9000, function()
        creature:Talk(SAY_INTRO_3)
    end)
    schedule(guid, "intro4", 14000, function()
        creature:Talk(SAY_INTRO_4)
    end)
end

local function gothikLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_KILL 6) only when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function gothikTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: Talk(SAY_DEATH 5) (the _JustDied + DATA_GOTHIK_GATE
-- legs have no bridge — tharon_ja precedent).
local function gothikDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

local function gothikReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GOTHIK, 1, gothikEnterCombat)
RegisterCreatureEvent(ENTRY_GOTHIK, 2, gothikLeaveCombat)
RegisterCreatureEvent(ENTRY_GOTHIK, 3, gothikTargetDied)
RegisterCreatureEvent(ENTRY_GOTHIK, 4, gothikDied)
RegisterCreatureEvent(ENTRY_GOTHIK, 23, gothikReset)
