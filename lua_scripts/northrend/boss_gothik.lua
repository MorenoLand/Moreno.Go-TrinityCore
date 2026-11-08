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
-- BossAI::JustEngagedWith base arms PORTED 2026-11-03:
-- SetCombatPulseDelay(5) + me-form DoZoneInCombat
-- (ScriptedCreature.cpp:540/542; heigan/loatheb/razuvious
-- precedent) + SetBossState IN_PROGRESS PORTED 2026-11-05 (moroes
-- precedent) — the CheckRequiredBosses leg stays out; the
-- EVENT_SUMMON (25s) / EVENT_DOORS_UNLOCK (3m25s) / EVENT_PHASE_TWO
-- (4m30s) schedules are PORTED tick 406 below; EVENT_INTRO_2
-- Talk(SAY_INTRO_2 1) 4s, EVENT_INTRO_3 Talk(SAY_INTRO_3 2) 9s,
-- EVENT_INTRO_4 Talk(SAY_INTRO_4 3) 14s — one-shot timers scheduled
-- from JustEngagedWith with no phase mask, like C++; KilledUnit
-- Talk(SAY_KILL 6) (event 3 — C++-GATED: victim->GetTypeId() ==
-- TYPEID_PLAYER — nalorakk/kelthuzad player-gate variant); JustDied
-- Talk(SAY_DEATH 5) (event 4; _JustDied SetBossState DONE PORTED
-- 2026-11-05, moroes precedent — the instance-data gothikDone flag
-- for the dialogue pump is kept alongside; the DATA_GOTHIK_GATE
-- GO_STATE_ACTIVE leg is PORTED tick 406 below (FindNearestGameObject
-- + SetGoState). All timers cancelled on 2/4/23 (gargolmar precedent).
-- SAY_PHASE_TWO (4) + EMOTE_PHASE_TWO (7) are PORTED 2026-10-25
-- with the phase-two latch; EMOTE_GATE_OPENED (8) rides the unported
-- gate machine — not ported orphaned (vortex precedent).
-- Unmodeled (no bridges — documented, not wired):
-- boss_gothik: Reset — SetReactState(REACT_PASSIVE) PORTED
-- 2026-11-03 into the ev23 handler (boss_gothik.cpp:321; horsemen
-- Reset precedent) + _Reset SetBossState NOT_STARTED PORTED
-- 2026-11-05 (!= DONE-gated per ScriptedCreature.cpp:509-510,
-- moroes precedent); instance SetData(DATA_GOTHIK_GATE) GO_STATE_ACTIVE
-- + Initialize counters PORTED tick 406 below (gate GO driven directly
-- via FindNearestGameObject(181170) + SetGoState; no instance-SetData
-- bridge needed — instance_naxxramas.cpp:271-279 is a plain
-- gate->SetGoState(value) twin); JustSummoned — PORTED tick 406 below
-- (ev19: DoAction(GATE_OPENED/ACQUIRE_TARGET) + SetCombatPulseDelay(5),
-- else DespawnOrUnsummon; summons.Summon bookkeeping has no twin and
-- no observable effect); SummonedCreatureDespawn stays out
-- (summons.Despawn bookkeeping only); EVENT_SUMMON — PORTED tick 406
-- below (waves10/waves25 verbatim + CGUID_TRIGGER 127618 offsets +
-- DoSummon via GetCreaturesInGrid(16137, 150) + trigger:GetGUIDLow()
-- == targetDBGuid — GetGUIDLow IS the GetSpawnId() twin
-- (creatureWorldGUID packs the DB guid in the low 24 bits,
-- creatures.go:846; the old "no spawn-ID bridge" note was stale);
-- RAID_MODE selects on 25-man, so Is25ManRaid() is the selector, not
-- IsHeroic()); EVENT_DOORS_UNLOCK — PORTED tick 406 below
-- (_gateCanOpen latch + grid-sweep scan over the 7 minion entries for
-- alive && (!inCombat || inEvadeMode), anubarak sweep precedent);
-- EVENT_PHASE_TWO — PORTED 2026-10-25 as a timer-driven
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
-- C++-exact); CastStop ported 2026-11-05 (creature:StopSpellCast()
-- interrupts the pending Shadow Bolt 29317 twin cast).
-- EVENT_HARVEST — PORTED 2026-10-25: DoCastAOE(HARVEST_SOUL 28679,
-- triggered), 15s init/repeat, gated on the phase-two latch (no
-- phase-one casts — the old verdict's fabrication concern is met by
-- the latch, not by leaving the arm out); EVENT_BOLT — PORTED
-- 2026-10-25: DoCastVictim(SHADOW_BOLT 29317), 2s repeat, scheduled
-- only from EVENT_RESUME_ATTACK; EVENT_RESUME_ATTACK — PORTED
-- 2026-10-25: SetReactState(REACT_AGGRESSIVE) + BOLT at 0s; DoAction(
-- ACTION_MINION_EVADE) + OpenGate — PORTED tick 406 below (ev38
-- observable subset: gate-open when allowed; the EnterEvadeMode +
-- threat-empty arms have no bridge; OpenGate drives the gate GO +
-- Talk(EMOTE 8) + DoAction(GATE_OPENED) per summon via grid sweep);
-- DamageTaken — PORTED 2026-10-25 via ev9 numeric-return
-- (damage = 0 while the phase-two latch is unset; tesla/halazzi
-- precedent); EnterEvadeMode — NearTeleportTo(home) — no motion
-- bridge; the UpdateAI living/dead-side check (RectangleBoundary
-- livingSide/deadSide) + FindEligibleTarget player scan — no side /
-- visibility / threat bridges.
-- npc_gothik_minion_baseAI (shared): DamageTaken zeroing while the
-- gate is closed and the attacker is on the other side — PORTED tick
-- 406 below (ev9, per-minion gate latch + rectangle side check);
-- DoAction(ACTION_GATE_OPENED/ACTION_ACQUIRE_TARGET) — PORTED tick
-- 406 below (ev38: gate latch + FindEligibleTarget twin via
-- GetPlayersInRange + side check + IsValidAttackTarget + AddThreat +
-- AttackStart, which fires the minion's ev1 and arms the tick-405
-- timer kits; CanSeeOrDetect + isInAccessiblePlaceFor have no bridge
-- and are dropped); EnterEvadeMode — the Gothik notification leg is
-- PORTED tick 406 below (no-target branch grids Gothik + DoAction(2);
-- the evade motion itself has no bridge); JustDied — DoCastAOE(_deathNotify,
-- triggered) anchor spells (27892/27928/27935) — PORTED 2026-11-07
-- below (tick 404; tick-388 "entry-unverifiable" bar retracted —
-- entries are in the C++ enum, boss_gothik.cpp:94-96, and
-- RegisterCreatureEvent needs no boss_ai.go entry, palehoof-minion
-- precedent). Dormant until the EVENT_SUMMON wave machine lands
-- (krik'thir tick-390 dormant precedent); the trigger SpellHit
-- anchor machine downstream is PORTED tick 408 below.
-- npc_gothik_minion_livingtrainee (16124): DoCastAOE(SPELL_DEATH_
-- PLAGUE 55604) 2-arg (non-triggered), urandms(5,20)=5-20s
-- init/repeat — PORTED tick 405 below (ev1 arming, dormant until
-- the EVENT_SUMMON wave machine lands; the entry bar is retracted,
-- same as the tick-404 death anchors).
-- npc_gothik_minion_livingknight (16125): DoCastAOE(SPELL_SHADOW_MARK
-- 27825) 2-arg (non-triggered), urandms(5,10)=5-10s init,
-- urandms(15,20)=15-20s repeat — PORTED tick 405 below (ev1 arming,
-- dormant; entry bar retracted).
-- npc_gothik_minion_livingrider (16126): DoCastAOE(SPELL_SHADOW_BOLT_
-- VOLLEY 27831) 2-arg (non-triggered), urandms(5,10)=5-10s init,
-- urandms(10,15)=10-15s repeat — PORTED tick 405 below (ev1 arming,
-- dormant; entry bar retracted).
-- npc_gothik_minion_spectraltrainee (16127): DoCastAOE(SPELL_ARCANE_
-- EXPLOSION 27989) 2-arg (non-triggered), 2s init/repeat — PORTED
-- tick 405 below (ev1 arming, dormant; entry bar retracted — the
-- same dormant class as the living kits, same strip).
-- npc_gothik_minion_spectralknight (16148): DoCastAOE(SPELL_WHIRLWIND
-- 56408) 2-arg (non-triggered), urandms(15,25)=15-25s init,
-- urandms(20,25)=20-25s repeat — PORTED tick 405 below (ev1 arming,
-- dormant; entry bar retracted).
-- npc_gothik_minion_spectralrider (16150): Unholy Frenzy (55648/
-- RAID_MODE 27995) priority cast on a friendly knight > rider > horse
-- > Gothik without the buff (DoFindFriendlyMissingBuff) — PORTED
-- tick 405 below via GetFriendlyUnitsInRange + HasAura (the
-- DoFindFriendlyMissingBuff twin); DoCastVictim(SPELL_DRAIN_LIFE
-- 27994), urandms(8,12) = 8-12s init, urandms(10,15) = 10-15s
-- repeat — PORTED tick 405 below (ev1 arming, dormant; entry bar
-- retracted).
-- npc_gothik_minion_spectralhorse (16149): DoCastAOE(SPELL_STOMP
-- 27993) 2-arg (non-triggered), urandms(10,15)=10-15s init,
-- urandms(14,18)=14-18s repeat — PORTED tick 405 below (ev1 arming,
-- dormant; entry bar retracted).
-- npc_gothik_trigger (16137): SpellHit anchor machine (ANCHOR_1_x ->
-- ANCHOR_2_x -> SKULLS_x -> DoSummon) — PORTED tick 408 below (event 14;
-- SelectRandomSkullPile via GetCreaturesInGrid + GetGUIDLow, the
-- GetSpawnId() twin — the tick-404 blocker is retired).
-- spell_gothik_shadow_bolt_volley (SpellScript 27831: FilterTargets
-- removes shadow-marked targets) — no SpellScript binding bridge
-- (razelikh precedent) — joins the SpellScript/AuraScript queue.

local ENTRY_GOTHIK = 16060
local ENTRY_GOTHIK_TRIGGER = 16137

local BOSS_GOTHIK = 11 -- naxxramas.h NAXEncounter (ANUBREKHAN 0 ... GOTHIK 11)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

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

local ACTION_GATE_OPENED = 1
local ACTION_MINION_EVADE = 2
local ACTION_ACQUIRE_TARGET = 3
local EMOTE_GATE_OPENED = 8
local GO_GOTHIK_GATE = 181170 -- naxxramas.h:118
local CGUID_TRIGGER = 127618 -- boss_gothik.cpp:297

local waveCount = {}
local gateCanOpen = {}
local gateIsOpen = {}
local minionGateIsOpen = {}

-- Forward declarations (tharon_ja/gal_darah idiom): waveTick /
-- doorsUnlockTick / openGate are assigned in the appended block below,
-- after the ENTRY_* locals they reference; boss-handler closures read
-- them at call time. Plain assignment only — never re-`local function`.
local waveTick, doorsUnlockTick, openGate

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
-- ported (StopSpellCast before AttackStop, C++ arm order); AttackStop +
-- StopMoving + SetReactState(REACT_PASSIVE) + ResetAllThreat +
-- DoCastAOE alternating DEAD/LIVE (first cast DEAD,
-- _lastTeleportDead starts false) + cancel BOLT + resume at 2s.
local function teleportTick(creature, guid)
    if creature:GetHealth() < math.floor(creature:GetMaxHealth() * 30 / 100) then
        return
    end
    creature:StopSpellCast()
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

-- C++ JustEngagedWith (:327-339): Talk(SAY_INTRO_1) + schedules
-- EVENT_INTRO_2 (4s), EVENT_INTRO_3 (9s), EVENT_INTRO_4 (14s),
-- EVENT_SUMMON (25s), EVENT_DOORS_UNLOCK (3m25s), EVENT_PHASE_TWO
-- (4m30s) + SetData(DATA_GOTHIK_GATE, GO_STATE_READY) + _gateIsOpen =
-- false — the gate + summon / doors-unlock legs are PORTED tick 406
-- below; EVENT_PHASE_TWO is ported above as a timer-driven latch.
local function gothikEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_GOTHIK, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:538)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; heigan/loatheb/razuvious precedent)
    creature:SetMeleeAttack(false) -- boss_gothik.cpp:416: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    creature:Talk(SAY_INTRO_1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    waveCount[guid] = 0
    gateCanOpen[guid] = false
    gateIsOpen[guid] = false
    local gate = creature:FindNearestGameObject(GO_GOTHIK_GATE, 1000.0)
    if gate then gate:SetGoState(1) end -- GO_STATE_READY (:338)
    schedule(guid, "summon", 25000, function()
        waveTick(creature, guid)
    end)
    schedule(guid, "doorsunlock", 205000, function()
        doorsUnlockTick(creature, guid)
    end)
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
    -- C++ JustDied (:365-370): SetData(DATA_GOTHIK_GATE,
    -- GO_STATE_ACTIVE) + _gateIsOpen = false — PORTED tick 406.
    local guid = creature:GetGUID()
    gateIsOpen[guid] = false
    local gate = creature:FindNearestGameObject(GO_GOTHIK_GATE, 1000.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
    cancelTimers(guid)
    -- C++ JustDied (:365-370): _JustDied SetBossState DONE
    -- (ScriptedCreature.cpp:513-520; moroes precedent) alongside the
    -- instance-data flag for the dialogue pump below.
    creature:SetBossState(BOSS_GOTHIK, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.gothikDone = true
        end
    end
end

local function gothikReset(event, creature)
    -- C++ Reset (:319-325): SetReactState(REACT_PASSIVE) + _Reset
    -- (SetBossState NOT_STARTED when state != DONE,
    -- ScriptedCreature.cpp:499-511) + SetData(DATA_GOTHIK_GATE,
    -- GO_STATE_ACTIVE) + Initialize (_waveCount 0, _gateCanOpen
    -- false, _gateIsOpen true) — the gate + Initialize legs are
    -- PORTED tick 406 below (gate GO driven directly).
    if creature:GetBossState(BOSS_GOTHIK) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_GOTHIK, BOSS_STATE_NOT_STARTED)
    end
    creature:SetReactState(REACT_PASSIVE) -- boss_gothik.cpp:321 Reset arm
    local guid = creature:GetGUID()
    waveCount[guid] = 0
    gateCanOpen[guid] = false
    gateIsOpen[guid] = true
    local gate = creature:FindNearestGameObject(GO_GOTHIK_GATE, 1000.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE (:322)
    cancelTimers(guid)
end

RegisterCreatureEvent(ENTRY_GOTHIK, 1, gothikEnterCombat)
RegisterCreatureEvent(ENTRY_GOTHIK, 2, gothikLeaveCombat)
RegisterCreatureEvent(ENTRY_GOTHIK, 3, gothikTargetDied)
RegisterCreatureEvent(ENTRY_GOTHIK, 4, gothikDied)
RegisterCreatureEvent(ENTRY_GOTHIK, 23, gothikReset)

-- C++ npc_gothik_trigger::DamageTaken (boss_gothik.cpp:904):
-- damage = 0 unconditionally (the trigger is the SpellHit anchor —
-- immune to damage; the SpellHit anchor machine is PORTED tick 408
-- below). Event 9's second return rewrites damage
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
    -- APPROXIMATE: C++ :437-438 runs every UpdateAI tick (no Lua
    -- vehicle); damage-event-driven twin using the post-damage health
    -- estimate (GetHealth() - damage): open the gate once below 30%
    -- in phase two. openGate is idempotent.
    local guid = creature:GetGUID()
    local dmg = damage or 0
    if not gateIsOpen[guid] and creature:GetHealth() - dmg < math.floor(creature:GetMaxHealth() * 30 / 100) then
        openGate(creature, guid)
    end
end

RegisterCreatureEvent(ENTRY_GOTHIK, 9, gothikDamageTaken)

-- C++ npc_gothik_minion_baseAI::JustDied (boss_gothik.cpp:574-577):
-- if (_deathNotify) DoCastAOE(_deathNotify, true). The three living
-- minions pass their anchor spell as _deathNotify in their ctors
-- (:654 trainee 27892, :683 knight 27928, :712 rider 27935; enum
-- :74-76, entries :94-96); the four spectral minions pass none, so
-- they get no handler. DoCastAOE self-cast + triggered lowers to
-- creature:CastSpell(creature, spell, true) (harvestTick precedent
-- in this file). Live since tick 406 below (wave machine landed);
-- the trigger SpellHit anchor machine downstream stays out (blocker
-- retired — GetGUIDLow; follow-up unit).
local ENTRY_LIVE_TRAINEE = 16124
local ENTRY_LIVE_KNIGHT = 16125
local ENTRY_LIVE_RIDER = 16126

local SPELL_ANCHOR_1_TRAINEE = 27892 -- boss_gothik.cpp:74
local SPELL_ANCHOR_1_DK = 27928 -- boss_gothik.cpp:75
local SPELL_ANCHOR_1_RIDER = 27935 -- boss_gothik.cpp:76

local SPELL_ANCHOR_2_TRAINEE = 27893 -- boss_gothik.cpp:78
local SPELL_ANCHOR_2_DK = 27929 -- boss_gothik.cpp:79
local SPELL_ANCHOR_2_RIDER = 27936 -- boss_gothik.cpp:80

local SPELL_SKULLS_TRAINEE = 27915 -- boss_gothik.cpp:82
local SPELL_SKULLS_DK = 27931 -- boss_gothik.cpp:83
local SPELL_SKULLS_RIDER = 27937 -- boss_gothik.cpp:84

local function livingTraineeDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_ANCHOR_1_TRAINEE, true)
    cancelTimers(creature:GetGUID())
end

local function livingKnightDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_ANCHOR_1_DK, true)
    cancelTimers(creature:GetGUID())
end

local function livingRiderDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_ANCHOR_1_RIDER, true)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LIVE_TRAINEE, 4, livingTraineeDied)
RegisterCreatureEvent(ENTRY_LIVE_KNIGHT, 4, livingKnightDied)
RegisterCreatureEvent(ENTRY_LIVE_RIDER, 4, livingRiderDied)

-- C++ minion timer arms (tick 405): the tick-404 entry bar is
-- retracted for the timer legs too — the entries are in the C++
-- enum (boss_gothik.cpp:94-100) and RegisterCreatureEvent needs no
-- boss_ai.go entry (palehoof-minion precedent). All seven kits arm
-- on ev1: the minion base UpdateAI only runs _UpdateAI while the
-- minion has a victim (boss_gothik.cpp:619-624), so ev1 arming is
-- the C++-exact vehicle (palehoof-minion precedent); the 500ms
-- IsCasting defer is the timer-arm convention (palehoof miniboss
-- arms). Live since tick 406 below (wave machine landed — minions
-- reach ev1 via the DoAction acquire-target arm).
local SPELL_DEATH_PLAGUE = 55604
local SPELL_SHADOW_MARK = 27825
local SPELL_SHADOW_BOLT_VOLLEY = 27831
local SPELL_ARCANE_EXPLOSION = 27989
local SPELL_WHIRLWIND = 56408
local SPELL_DRAIN_LIFE = 27994
local SPELL_UNHOLY_FRENZY = 55648
local SPELL_UNHOLY_FRENZY_H = 27995
local SPELL_STOMP = 27993

local ENTRY_DEAD_TRAINEE = 16127
local ENTRY_DEAD_KNIGHT = 16148
local ENTRY_DEAD_RIDER = 16150
local ENTRY_DEAD_HORSE = 16149

-- C++ npc_gothik_triggerAI::SelectRandomSkullPile (boss_gothik.cpp:
-- 906-916): grid list of NPC_TRIGGER (16137) within 150.0f, return the
-- trigger whose GetSpawnId() == CGUID_TRIGGER + urand(8, 12); the DB
-- guids are unique so the scan is a key lookup (GetGUIDLow is the
-- GetSpawnId() twin, creatures.go:846; waveTick precedent).
local function selectRandomSkullPile(creature)
    local targetDBGuid = CGUID_TRIGGER + math.random(8, 12)
    for _, trigger in ipairs(creature:GetCreaturesInGrid(ENTRY_GOTHIK_TRIGGER, 150)) do
        if trigger:GetGUIDLow() == targetDBGuid then
            return trigger
        end
    end
    return nil
end

-- C++ npc_gothik_triggerAI::SpellHit (boss_gothik.cpp:918-954):
-- ANCHOR_1_x hits re-cast ANCHOR_2_x on self (DoCastAOE, triggered);
-- ANCHOR_2_x hits cast the SKULLS_x spell (triggered) at the
-- skull-pile trigger picked by SelectRandomSkullPile; SKULLS_x hits
-- summon the dead minion(s) at the hit trigger (DoSummon, 0.0f radius,
-- 15s TEMPSUMMON_CORPSE_TIMED_DESPAWN — SummonCreature type 5,
-- 15000ms, waveTick precedent). Event 14 args (event, creature,
-- caster, spellid) (boss_ai.go:3787-3790; midnight attumenSpellHit
-- precedent); the caster arg is unused (C++ ignores it).
local function gothikTriggerSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_ANCHOR_1_TRAINEE then
        creature:CastSpell(creature, SPELL_ANCHOR_2_TRAINEE, true)
    elseif spellId == SPELL_ANCHOR_1_DK then
        creature:CastSpell(creature, SPELL_ANCHOR_2_DK, true)
    elseif spellId == SPELL_ANCHOR_1_RIDER then
        creature:CastSpell(creature, SPELL_ANCHOR_2_RIDER, true)
    elseif spellId == SPELL_ANCHOR_2_TRAINEE then
        local target = selectRandomSkullPile(creature)
        if target then
            creature:CastSpell(target, SPELL_SKULLS_TRAINEE, true)
        end
    elseif spellId == SPELL_ANCHOR_2_DK then
        local target = selectRandomSkullPile(creature)
        if target then
            creature:CastSpell(target, SPELL_SKULLS_DK, true)
        end
    elseif spellId == SPELL_ANCHOR_2_RIDER then
        local target = selectRandomSkullPile(creature)
        if target then
            creature:CastSpell(target, SPELL_SKULLS_RIDER, true)
        end
    elseif spellId == SPELL_SKULLS_TRAINEE then
        creature:SummonCreature(ENTRY_DEAD_TRAINEE, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO(), 5, 15000)
    elseif spellId == SPELL_SKULLS_DK then
        creature:SummonCreature(ENTRY_DEAD_KNIGHT, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO(), 5, 15000)
    elseif spellId == SPELL_SKULLS_RIDER then
        creature:SummonCreature(ENTRY_DEAD_RIDER, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO(), 5, 15000)
        creature:SummonCreature(ENTRY_DEAD_HORSE, creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO(), 5, 15000)
    end
end

RegisterCreatureEvent(ENTRY_GOTHIK_TRIGGER, 14, gothikTriggerSpellHit)

-- C++ EVENT_SUMMON wave machine (boss_gothik.cpp:444-490).
-- Waves transcribed verbatim (:175-294); each wave is { { {entry,count}, ... },
-- timeToNext }; multi-entry waves are in std::set order (entry ascending),
-- not initializer order. RAID_MODE selects on 25-man, so Is25ManRaid()
-- (not IsHeroic) is the selector. Bounds check (:446-449): out of range
-- logs in C++ and breaks with no repeat and no increment.
local WAVES_10 = {
    { {{16124, 2}}, 20 }, { {{16124, 2}}, 20 }, { {{16124, 2}}, 10 },
    { {{16125, 1}}, 10 }, { {{16124, 2}}, 15 }, { {{16125, 1}}, 5 },
    { {{16124, 2}}, 20 }, { {{16124, 2}, {16125, 1}}, 10 },
    { {{16126, 1}}, 10 }, { {{16124, 2}}, 5 }, { {{16125, 1}}, 15 },
    { {{16124, 2}, {16126, 1}}, 10 }, { {{16125, 2}}, 10 },
    { {{16124, 2}}, 10 }, { {{16126, 1}}, 5 }, { {{16125, 1}}, 5 },
    { {{16124, 2}}, 20 }, { {{16124, 2}, {16125, 1}, {16126, 1}}, 15 },
    { {{16124, 2}}, 0 },
}
local WAVES_25 = {
    { {{16124, 3}}, 20 }, { {{16124, 3}}, 20 }, { {{16124, 3}}, 10 },
    { {{16125, 2}}, 10 }, { {{16124, 3}}, 15 }, { {{16125, 2}}, 5 },
    { {{16124, 3}}, 20 }, { {{16124, 3}, {16125, 2}}, 10 },
    { {{16124, 3}}, 10 }, { {{16126, 1}}, 5 }, { {{16124, 3}}, 15 },
    { {{16126, 1}}, 10 }, { {{16125, 2}}, 10 }, { {{16126, 1}}, 10 },
    { {{16124, 3}, {16126, 1}}, 5 }, { {{16124, 3}, {16125, 1}}, 5 },
    { {{16124, 3}, {16126, 1}}, 20 },
    { {{16124, 3}, {16125, 2}, {16126, 1}}, 0 },
}

local MINION_ENTRIES = { ENTRY_LIVE_TRAINEE, ENTRY_LIVE_KNIGHT, ENTRY_LIVE_RIDER,
    ENTRY_DEAD_TRAINEE, ENTRY_DEAD_KNIGHT, ENTRY_DEAD_RIDER, ENTRY_DEAD_HORSE }

-- C++ OpenGate (:373-388): idempotent guard + gate GO ACTIVE + Talk(EMOTE 8)
-- + DoAction(ACTION_GATE_OPENED) per summon (grid sweep = summons list).
openGate = function(creature, guid)
    if gateIsOpen[guid] then return end
    local gate = creature:FindNearestGameObject(GO_GOTHIK_GATE, 1000.0)
    if gate then gate:SetGoState(0) end -- GO_STATE_ACTIVE
    creature:Talk(EMOTE_GATE_OPENED)
    gateIsOpen[guid] = true
    for _, entry in ipairs(MINION_ENTRIES) do
        for _, summon in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            summon:DoAction(ACTION_GATE_OPENED)
        end
    end
end

-- C++ EVENT_DOORS_UNLOCK (:492-501).
doorsUnlockTick = function(creature, guid)
    gateCanOpen[guid] = true
    for _, entry in ipairs(MINION_ENTRIES) do
        for _, summon in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            if summon:IsAlive() and (not summon:IsInCombat() or summon:IsInEvadeMode()) then
                openGate(creature, guid)
                return
            end
        end
    end
end

-- C++ EVENT_SUMMON (:444-490). DoSummon(entry, trigger, 1.0f, 15s,
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN): the 1.0f radius jitter has no Lua
-- twin, so summons land at the exact trigger position (SummonCreature
-- type 5, 15000ms). Trigger spawn IDs: rider 127622+(i%2), knight
-- 127623+(i%3), trainee 127625-(i%3) (CGUID_TRIGGER 127618 + 4..7).
waveTick = function(creature, guid)
    if phaseTwo[guid] then return end -- PHASE_ONE mask (:331)
    local waves = creature:Is25ManRaid() and WAVES_25 or WAVES_10
    local waveIndex = waveCount[guid] or 0
    local wave = waves[waveIndex + 1]
    if not wave then return end -- bounds check: no repeat, no increment
    local triggers = {}
    for _, trigger in ipairs(creature:GetCreaturesInGrid(ENTRY_GOTHIK_TRIGGER, 150)) do
        triggers[trigger:GetGUIDLow()] = trigger -- GetSpawnId() twin
    end
    for _, pair in ipairs(wave[1]) do
        local entry, count = pair[1], pair[2]
        for i = 0, count - 1 do
            local targetDBGuid
            if entry == ENTRY_LIVE_RIDER then
                targetDBGuid = (CGUID_TRIGGER + 4) + (i % 2)
            elseif entry == ENTRY_LIVE_KNIGHT then
                targetDBGuid = (CGUID_TRIGGER + 5) + (i % 3)
            elseif entry == ENTRY_LIVE_TRAINEE then
                targetDBGuid = (CGUID_TRIGGER + 7) - (i % 3)
            end
            local trigger = targetDBGuid and triggers[targetDBGuid] or nil
            if trigger then
                creature:SummonCreature(entry, trigger:GetX(), trigger:GetY(),
                    trigger:GetZ(), trigger:GetO(), 5, 15000)
            end
        end
    end
    local timeToNext = wave[2]
    waveCount[guid] = waveIndex + 1
    if timeToNext > 0 then
        schedule(guid, "summon", timeToNext * 1000, function()
            waveTick(creature, guid)
        end)
    end
end

-- C++ boss_gothikAI::JustSummoned (:342-352). ev19 fires from
-- SummonCreature on the summoner; summons.Summon bookkeeping has no twin.
local function gothikJustSummoned(event, creature, summon)
    if not summon then return end
    if not creature:IsInCombat() then
        summon:DespawnOrUnsummon()
        return
    end
    if gateIsOpen[creature:GetGUID()] then
        summon:DoAction(ACTION_GATE_OPENED)
    else
        summon:DoAction(ACTION_ACQUIRE_TARGET)
    end
    summon:SetCombatPulseDelay(5)
end

-- C++ boss_gothikAI::DoAction(ACTION_MINION_EVADE) (:397-407), observable
-- subset only: the EnterEvadeMode arm and the threat-empty guard have no
-- bridge, so the only Lua-visible effect is opening the gate when allowed.
local function gothikDoAction(event, creature, action)
    if action ~= ACTION_MINION_EVADE then return end
    local guid = creature:GetGUID()
    if gateIsOpen[guid] then return end
    if gateCanOpen[guid] then
        openGate(creature, guid)
    end
end

RegisterCreatureEvent(ENTRY_GOTHIK, 19, gothikJustSummoned)
RegisterCreatureEvent(ENTRY_GOTHIK, 38, gothikDoAction)

-- C++ livingSide/deadSide (:134-135) + GetSide (:141-151, living checked
-- first, so Y exactly -3360.78 is living). SIDE_NONE (0) matches SIDE_NONE
-- in IsOnSameSide, C++-exact.
local function getSide(unit)
    local x, y = unit:GetX(), unit:GetY()
    if x >= 2633.84 and x <= 2750.49 then
        if y >= -3434.0 and y <= -3360.78 then return 1 end -- SIDE_LIVING
        if y >= -3360.78 and y <= -3285.0 then return 2 end -- SIDE_DEAD
    end
    return 0
end

local function isOnSameSide(a, b)
    return getSide(a) == getSide(b)
end

-- C++ FindEligibleTarget (:154-171): first map player passing the gates.
-- Lua delta: CanSeeOrDetect + isInAccessiblePlaceFor have no bridge and
-- are dropped; GetPlayersInRange() defaults (533yd, alive-only) stand in
-- for the map-wide iteration; player order is range-list order.
local function findEligibleTarget(creature, gateIsOpenFlag)
    for _, player in ipairs(creature:GetPlayersInRange()) do
        if (gateIsOpenFlag or isOnSameSide(creature, player))
            and creature:IsValidAttackTarget(player)
            and not player:IsGM() then
            return player
        end
    end
    return nil
end

-- C++ npc_gothik_minion_baseAI::DoAction (:598-612): GATE_OPENED sets the
-- minion's own latch and falls through to ACQUIRE_TARGET; no target ->
-- EnterEvadeMode (no bridge) -> Gothik DoAction(ACTION_MINION_EVADE)
-- (:630-636), ported as the notification only, via grid lookup (no
-- instance GetGuidData bridge). AttackStart engages the minion, firing
-- its ev1 — this is what arms the tick-405 timer kits.
local function minionDoAction(event, creature, action)
    local guid = creature:GetGUID()
    if action == ACTION_GATE_OPENED then
        minionGateIsOpen[guid] = true
    elseif action ~= ACTION_ACQUIRE_TARGET then
        return
    end
    local target = findEligibleTarget(creature, minionGateIsOpen[guid] == true)
    if target then
        creature:AddThreat(target, 1.0)
        creature:AttackStart(target)
    else
        for _, gothik in ipairs(creature:GetCreaturesInGrid(ENTRY_GOTHIK, 150)) do
            gothik:DoAction(ACTION_MINION_EVADE)
            break
        end
    end
end

-- C++ npc_gothik_minion_baseAI::DamageTaken (:585-590): damage = 0 while
-- the gate is closed and the attacker is on the other side (nil attacker
-- also zeroes). minionGateIsOpen is keyed per minion GUID and deliberately
-- not cleared on ev2/23 (C++ keeps the latch across evades; new spawns get
-- fresh GUIDs, so stale keys are never read).
local function minionDamageTaken(event, creature, attacker, damage)
    if not minionGateIsOpen[creature:GetGUID()]
        and (not attacker or not isOnSameSide(creature, attacker)) then
        return false, 0
    end
end

for _, entry in ipairs(MINION_ENTRIES) do
    RegisterCreatureEvent(entry, 38, minionDoAction)
    RegisterCreatureEvent(entry, 9, minionDamageTaken)
end

-- C++ npc_gothik_minion_livingtraineeAI::_UpdateAI (:656-666):
-- _deathPlagueTimer urandms(5,20) from the ctor (:654);
-- DoCastAOE(SPELL_DEATH_PLAGUE 55604) 2-arg = non-triggered,
-- re-roll urandms(5,20).
local function deathPlagueTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "deathplague", 500, function()
            deathPlagueTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DEATH_PLAGUE)
    schedule(guid, "deathplague", {5000, 20000}, function()
        deathPlagueTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_livingknightAI::_UpdateAI (:685-695):
-- _whirlwindTimer urandms(5,10) from the ctor (:683); the fired
-- cast is DoCastAOE(SPELL_SHADOW_MARK 27825) — the timer variable
-- is named for whirlwind but the arm casts Shadow Mark, ported as
-- written — re-roll urandms(15,20).
local function shadowMarkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowmark", 500, function()
            shadowMarkTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHADOW_MARK)
    schedule(guid, "shadowmark", {15000, 20000}, function()
        shadowMarkTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_livingriderAI::_UpdateAI (:714-726):
-- _boltVolleyTimer urandms(5,10) from the ctor (:712);
-- DoCastAOE(SPELL_SHADOW_BOLT_VOLLEY 27831) re-roll urandms(10,15);
-- the !UNIT_STATE_CASTING melee gate is engine-driven in Go.
local function boltVolleyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "boltvolley", 500, function()
            boltVolleyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHADOW_BOLT_VOLLEY)
    schedule(guid, "boltvolley", {10000, 15000}, function()
        boltVolleyTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_spectraltraineeAI::_UpdateAI (:744-752):
-- _explosionTimer 2s fixed from the ctor (:742); DoCastAOE
-- (SPELL_ARCANE_EXPLOSION 27989) re-roll 2s.
local function arcaneExplosionTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "arcaneexplosion", 500, function()
            arcaneExplosionTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ARCANE_EXPLOSION)
    schedule(guid, "arcaneexplosion", 2000, function()
        arcaneExplosionTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_spectralknightAI::_UpdateAI (:773-781):
-- _whirlwindTimer urandms(15,25) from the ctor (:771);
-- DoCastAOE(SPELL_WHIRLWIND 56408) re-roll urandms(20,25).
local function whirlwindTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "whirlwind", 500, function()
            whirlwindTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", {20000, 25000}, function()
        whirlwindTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_spectralhorseAI::_UpdateAI (:867-875):
-- _stompTimer urandms(10,15) from the ctor (:865); DoCastAOE
-- (SPELL_STOMP 27993) re-roll urandms(14,18).
local function stompTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stomp", 500, function()
            stompTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", {14000, 18000}, function()
        stompTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_spectralriderAI::_UpdateAI drain leg
-- (:840-847): _drainTimer urandms(8,12) from the ctor (:800);
-- DoCastVictim(SPELL_DRAIN_LIFE 27994) re-roll urandms(10,15).
local function drainLifeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "drainlife", 500, function()
            drainLifeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_DRAIN_LIFE)
    schedule(guid, "drainlife", {10000, 15000}, function()
        drainLifeTick(creature, guid)
    end)
end

-- C++ npc_gothik_minion_spectralriderAI::_UpdateAI frenzy leg
-- (:802-838): _frenzyTimer urandms(2,5) from the ctor (:800), 20s
-- cycle; while casting the timer resets to 0 (retry next tick —
-- the 500ms defer twin); else target priority knight > other rider
-- > horse > Gothik among friendlies within 30.0 missing
-- SPELLHELPER_UNHOLY_FRENZY (RAID_MODE 55648 / 27995, :90),
-- DoCast(target, ...) 2-arg = non-triggered. GetFriendlyUnitsInRange
-- is the DoFindFriendlyMissingBuff twin
-- (lua_creature_events.go:5905; alive, IsFriendlyTo, self
-- excluded); the HasAura filter is the missing-buff check
-- (creature:HasAura bridged, lua_creature_events.go:1309). The C++
-- early-break on first knight is folded into the priority pick
-- (same outcome; grid order vs GUID-sorted order may pick a
-- different knight when several qualify — documented).
local function frenzyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function()
            frenzyTick(creature, guid)
        end)
        return
    end
    local frenzy = SPELL_UNHOLY_FRENZY
    if creature:IsHeroic() then
        frenzy = SPELL_UNHOLY_FRENZY_H
    end
    local knightTarget = nil
    local riderTarget = nil
    local horseTarget = nil
    local gothikTarget = nil
    for _, unit in ipairs(creature:GetFriendlyUnitsInRange(30.0)) do
        if unit:GetObjectType() == "Creature" and not unit:HasAura(frenzy) then
            local entry = unit:GetEntry()
            if entry == ENTRY_DEAD_KNIGHT then
                knightTarget = unit
            elseif entry == ENTRY_DEAD_RIDER then
                riderTarget = unit
            elseif entry == ENTRY_DEAD_HORSE then
                horseTarget = unit
            elseif entry == ENTRY_GOTHIK then
                gothikTarget = unit
            end
        end
    end
    local target = knightTarget or riderTarget or horseTarget or gothikTarget
    if target then
        creature:CastSpell(target, frenzy)
    end
    schedule(guid, "frenzy", 20000, function()
        frenzyTick(creature, guid)
    end)
end

-- Shared ev1 arming per minion entry (ctor inits above).
local function livingTraineeEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "deathplague", {5000, 20000}, function()
        deathPlagueTick(creature, guid)
    end)
end

local function livingKnightEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowmark", {5000, 10000}, function()
        shadowMarkTick(creature, guid)
    end)
end

local function livingRiderEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "boltvolley", {5000, 10000}, function()
        boltVolleyTick(creature, guid)
    end)
end

local function spectralTraineeEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcaneexplosion", 2000, function()
        arcaneExplosionTick(creature, guid)
    end)
end

local function spectralKnightEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "whirlwind", {15000, 25000}, function()
        whirlwindTick(creature, guid)
    end)
end

local function spectralRiderEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "frenzy", {2000, 5000}, function()
        frenzyTick(creature, guid)
    end)
    schedule(guid, "drainlife", {8000, 12000}, function()
        drainLifeTick(creature, guid)
    end)
end

local function spectralHorseEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "stomp", {10000, 15000}, function()
        stompTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/4/23 on timer-carrying minions
-- (the gothikLeaveCombat twin; the spectral JustDied is a no-op in
-- C++ — base passes no _deathNotify — so ev4 rides this wipe).
local function minionCancelTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LIVE_TRAINEE, 1, livingTraineeEnterCombat)
RegisterCreatureEvent(ENTRY_LIVE_TRAINEE, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_LIVE_TRAINEE, 23, minionCancelTimers)
RegisterCreatureEvent(ENTRY_LIVE_KNIGHT, 1, livingKnightEnterCombat)
RegisterCreatureEvent(ENTRY_LIVE_KNIGHT, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_LIVE_KNIGHT, 23, minionCancelTimers)
RegisterCreatureEvent(ENTRY_LIVE_RIDER, 1, livingRiderEnterCombat)
RegisterCreatureEvent(ENTRY_LIVE_RIDER, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_LIVE_RIDER, 23, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_TRAINEE, 1, spectralTraineeEnterCombat)
RegisterCreatureEvent(ENTRY_DEAD_TRAINEE, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_TRAINEE, 4, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_TRAINEE, 23, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_KNIGHT, 1, spectralKnightEnterCombat)
RegisterCreatureEvent(ENTRY_DEAD_KNIGHT, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_KNIGHT, 4, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_KNIGHT, 23, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_RIDER, 1, spectralRiderEnterCombat)
RegisterCreatureEvent(ENTRY_DEAD_RIDER, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_RIDER, 4, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_RIDER, 23, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_HORSE, 1, spectralHorseEnterCombat)
RegisterCreatureEvent(ENTRY_DEAD_HORSE, 2, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_HORSE, 4, minionCancelTimers)
RegisterCreatureEvent(ENTRY_DEAD_HORSE, 23, minionCancelTimers)
