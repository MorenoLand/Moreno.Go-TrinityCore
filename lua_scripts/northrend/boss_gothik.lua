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
-- bindings; NPC_TRIGGER 16137 verified at boss_gothik.cpp:102 and its
-- DamageTaken immunity arm is now registered (see below) — the rest
-- stay unregistered, no lua.
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
-- SAY_PHASE_TWO (4) + EMOTE_PHASE_TWO (7) are PORTED 2026-10-25
-- with the phase-two latch; EMOTE_GATE_OPENED (8) rides the unported
-- gate machine — not ported orphaned (vortex precedent).
-- Unmodeled (no bridges — documented, not wired):
-- boss_gothik: Reset — SetReactState(REACT_PASSIVE) + instance->
-- SetData(DATA_GOTHIK_GATE, GO_STATE_ACTIVE) — instance / GO-state
-- blocked (no bridges); the react-state leg is bridgeable in isolation
-- but Reset is unported, so it stays out with the rest; JustSummoned /
-- SummonedCreatureDespawn — summons.Summon/summons.Despawn + cross-AI
-- DoAction + instance GetGuidData (summon-list/cross-AI/instance
-- blocked, not 'no summon bridge'); EVENT_SUMMON — the waves10/waves25
-- wave tables + CGUID_TRIGGER (127618) spawn-offset layout (rider
-- center-back/north, knight north/center-front/south, trainee
-- south/center-front/north) + DoSummon via trigger spawn IDs (DB
-- spawn-ID trigger lookups via trigger->GetSpawnId(), :462-480 — no
-- spawn-ID bridge, coords not in C++; DoSummon itself is a plain
-- SummonCreature twin; the RAID_MODE parenthetical is stale —
-- IsHeroic() is bridged); EVENT_DOORS_UNLOCK — iterates the `summons`
-- LIST via ObjectAccessor::GetCreature checking IsAlive/IsInCombat
-- (summon-list/ObjectAccessor bridge blocker, not the SummonCreature
-- bridge); EVENT_PHASE_TWO — PORTED 2026-10-25 as a timer-driven
-- latch (the flip is scheduled unconditionally at engage + 4m30s,
-- the ONLY phase flip in the file — heigan dancing-latch precedent):
-- SetPhase twin + teleport 20s / harvest 15s / resume 2s schedules +
-- Talk(SAY_PHASE_TWO 4) + Talk(EMOTE_PHASE_TWO 7) +
-- SetReactState(REACT_PASSIVE) + ResetAllThreat + DoCastAOE
-- TELEPORT_LIVE (non-triggered self-cast). The wave machine stays out
-- — it gates only the adds, not these boss arms; the old
-- phase-machine verdict is superseded for the boss arms (the
-- Oct-12 "no DoCastAOE bridge" claim was already superseded).
-- EVENT_TELEPORT — PORTED 2026-10-25: HealthBelowPct(30) floor-form
-- gate + AttackStop + StopMoving + SetReactState(REACT_PASSIVE) +
-- ResetAllThreat + alternating DEAD/LIVE DoCastAOE + BOLT cancel +
-- resume 2s + 20s repeat inside the gate (arm dies below 30%,
-- C++-exact); CastStop has no bridge (engine-wide grep) and stays out.
-- EVENT_HARVEST — PORTED 2026-10-25: DoCastAOE(HARVEST_SOUL 28679,
-- triggered), 15s init/repeat, gated on the phase-two latch (no
-- phase-one casts — the old verdict's fabrication concern is met by
-- the latch, not by leaving the arm out); EVENT_BOLT — PORTED
-- 2026-10-25: DoCastVictim(SHADOW_BOLT 29317), 2s repeat, scheduled
-- only from EVENT_RESUME_ATTACK; EVENT_RESUME_ATTACK — PORTED
-- 2026-10-25: SetReactState(REACT_AGGRESSIVE) + BOLT at 0s; DoAction(
-- ACTION_MINION_EVADE) + OpenGate — instance SetData + summon-list
-- iteration + cross-AI DoAction — no instance / summon / cross-AI
-- bridges; DamageTaken — PORTED 2026-10-25 via ev9 numeric-return
-- (damage = 0 while the phase-two latch is unset; tesla/halazzi
-- precedent); EnterEvadeMode — NearTeleportTo(home) — no motion
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
-- roots at the ANCHOR_2_x skull-pile selection via CGUID_TRIGGER +
-- urand(8,12) (no spawn-ID bridge — DB lookup); the SKULLS_x ->
-- DoSummon(NPC_DEAD_TRAINEE/KNIGHT/RIDER/HORSE, me, 0.0f, 15s) legs
-- (:941-952) and the ANCHOR_1_x -> DoCastAOE(ANCHOR_2_x, true) legs
-- are bridgeable in isolation but unreachable — stays OUT.
-- spell_gothik_shadow_bolt_volley (SpellScript 27831: FilterTargets
-- removes shadow-marked targets) — no SpellScript binding bridge
-- (razelikh precedent) — joins the SpellScript/AuraScript queue.

local ENTRY_GOTHIK = 16060
local ENTRY_GOTHIK_TRIGGER = 16137

local SAY_INTRO_1 = 0
local SAY_INTRO_2 = 1
local SAY_INTRO_3 = 2
local SAY_INTRO_4 = 3
local SAY_DEATH = 5
local SAY_KILL = 6
local SAY_PHASE_TWO = 4
local EMOTE_PHASE_TWO = 7

local SPELL_TELEPORT_DEAD = 28025
local SPELL_TELEPORT_LIVE = 28026
local SPELL_HARVEST_SOUL = 28679
local SPELL_SHADOW_BOLT = 29317

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local timers = {}

local phaseTwo = {}
local lastTeleportDead = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    phaseTwo[guid] = nil
    lastTeleportDead[guid] = nil
end

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
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

-- C++ EVENT_BOLT: DoCastVictim(SHADOW_BOLT 29317), 2s repeat,
-- scheduled only from EVENT_RESUME_ATTACK (phase-two machine).
local function boltTick(creature, guid)
    creature:CastSpell(nil, SPELL_SHADOW_BOLT)
    schedule(guid, "bolt", 2000, function()
        boltTick(creature, guid)
    end)
end

-- C++ EVENT_RESUME_ATTACK: SetReactState(REACT_AGGRESSIVE) +
-- EVENT_BOLT at 0s (direct call = 0s-schedule twin, sapphiron
-- check-resists precedent).
local function resumeTick(creature, guid)
    creature:SetReactState(REACT_AGGRESSIVE)
    boltTick(creature, guid)
end

-- C++ EVENT_HARVEST: DoCastAOE(HARVEST_SOUL 28679, triggered),
-- 15s init/repeat.
local function harvestTick(creature, guid)
    creature:CastSpell(creature, SPELL_HARVEST_SOUL, true)
    schedule(guid, "harvest", 15000, function()
        harvestTick(creature, guid)
    end)
end

-- C++ EVENT_TELEPORT: 20s repeat, only while NOT HealthBelowPct(30)
-- (health < floor(max*30/100), Unit.h:872 + CalculatePct trunc; the
-- Repeat sits inside the gate, so the arm dies below 30%): CastStop
-- has no bridge (engine-wide grep) and stays out; AttackStop +
-- StopMoving + SetReactState(REACT_PASSIVE) + ResetAllThreat +
-- DoCastAOE alternating DEAD/LIVE (first cast DEAD,
-- _lastTeleportDead starts false) + cancel BOLT + resume at 2s.
local function teleportTick(creature, guid)
    if creature:GetHealth() < math.floor(creature:GetMaxHealth() * 30 / 100) then
        return
    end
    creature:AttackStop()
    creature:StopMoving()
    creature:SetReactState(REACT_PASSIVE)
    creature:ResetAllThreat()
    if lastTeleportDead[guid] then
        creature:CastSpell(creature, SPELL_TELEPORT_LIVE)
        lastTeleportDead[guid] = false
    else
        creature:CastSpell(creature, SPELL_TELEPORT_DEAD)
        lastTeleportDead[guid] = true
    end
    cancelKey(guid, "bolt")
    schedule(guid, "resume", 2000, function()
        resumeTick(creature, guid)
    end)
    schedule(guid, "teleport", 20000, function()
        teleportTick(creature, guid)
    end)
end

-- C++ EVENT_PHASE_TWO (engage + 4m30s, the ONLY phase flip in the
-- file — JustEngagedWith schedules it unconditionally, so a
-- timer-driven latch is C++-exact, heigan dancing-latch precedent):
-- SetPhase twin + teleport 20s / harvest 15s / resume 2s + Talks +
-- SetReactState(REACT_PASSIVE) + ResetAllThreat + DoCastAOE
-- TELEPORT_LIVE (non-triggered self-cast). The wave machine (summon
-- tables, gate state, side checks) stays out — it gates only the
-- adds, not these boss arms.
local function phaseTwoTick(creature, guid)
    phaseTwo[guid] = true
    lastTeleportDead[guid] = false
    schedule(guid, "teleport", 20000, function()
        teleportTick(creature, guid)
    end)
    schedule(guid, "harvest", 15000, function()
        harvestTick(creature, guid)
    end)
    schedule(guid, "resume", 2000, function()
        resumeTick(creature, guid)
    end)
    creature:Talk(SAY_PHASE_TWO)
    creature:Talk(EMOTE_PHASE_TWO)
    creature:SetReactState(REACT_PASSIVE)
    creature:ResetAllThreat()
    creature:CastSpell(creature, SPELL_TELEPORT_LIVE)
end

-- C++ JustEngagedWith: Talk(SAY_INTRO_1) + schedules EVENT_INTRO_2
-- (4s), EVENT_INTRO_3 (9s), EVENT_INTRO_4 (14s), EVENT_SUMMON (25s),
-- EVENT_DOORS_UNLOCK (3m25s), EVENT_PHASE_TWO (4m30s). The
-- EVENT_SUMMON / EVENT_DOORS_UNLOCK legs stay out (wave tables ride
-- DB spawn-ID trigger lookups + summon-list iteration, no bridges);
-- EVENT_PHASE_TWO is ported above as a timer-driven latch.
local function gothikEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_gothik.cpp:416: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
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
    schedule(guid, "phaseTwo", 270000, function()
        phaseTwoTick(creature, guid)
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

-- C++ npc_gothik_trigger::DamageTaken (boss_gothik.cpp:904):
-- damage = 0 unconditionally (the trigger is the SpellHit anchor —
-- immune to damage; the SpellHit anchor machine itself stays
-- documented-only). Event 9's second return rewrites damage
-- (fireCreatureDamageTaken; halazzi precedent).
local function gothikTriggerDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_GOTHIK_TRIGGER, 9, gothikTriggerDamageTaken)

-- C++ boss_gothik::DamageTaken (boss_gothik.cpp:390-394): damage = 0
-- while not in PHASE_TWO. Event 9's second return rewrites damage
-- (fireCreatureDamageTaken; tesla/halazzi precedent); no return once
-- phase two has flipped leaves damage untouched.
local function gothikDamageTaken(event, creature, attacker, damage)
    if not phaseTwo[creature:GetGUID()] then
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_GOTHIK, 9, gothikDamageTaken)
