-- General Zarithrian (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_general_zarithrian.cpp
-- (2 CreatureScripts: boss_general_zarithrian (BossAI,
-- DATA_GENERAL_ZARITHRIAN = 1) + npc_onyx_flamecaller (ScriptedAI,
-- NPC_ONYX_FLAMECALLER = 39814); both registered from inside
-- AddSC_boss_general_zarithrian() (line 276); loader decl 192 /
-- call 387 per northrend_script_loader.cpp — the FIFTH group of the
-- "// Ruby Sanctum" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_saviana_ragefire() (call 386) — verified from the
-- loader this run; the checkpoint sequence (saviana -> zarithrian) is
-- followed).
-- Entry: 39746 General Zarithrian (ruby_sanctum.h
-- NPC_GENERAL_ZARITHRIAN, line 72; instance_ruby_sanctum.cpp
-- creatureData binds NPC_GENERAL_ZARITHRIAN ->
-- DATA_GENERAL_ZARITHRIAN (line 52); DATA_GENERAL_ZARITHRIAN = 1
-- (ruby_sanctum.h line 32)) — entry-verifiable, registration proceeds
-- (the nexus_commanders kalecgos precedent); the ScriptName bindings
-- are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_general_zarithrian" hits boss_general_zarithrian.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_general_zarithrian JustEngagedWith — Talk(SAY_AGGRO 0)
-- (event 1 — the auriaya precedent; the BossAI::JustEngagedWith /
-- events.ScheduleEvent(EVENT_CLEAVE / EVENT_INTIDMDATING_ROAR /
-- EVENT_SUMMON_ADDS [/EVENT_SUMMON_ADDS2]) legs ride the maiden
-- CreateLuaEvent convention now (ported 2026-10-13/14).
-- boss_general_zarithrian KilledUnit — Talk(SAY_KILL 1) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_general_zarithrian JustDied — Talk(SAY_DEATH 3)
-- (event 4 — the sjonnir precedent; the _JustDied() instance leg has
-- no bridge).
-- DOCUMENTED-ONLY (in this header; only entry 39746 registered):
-- boss_general_zarithrian Reset / CanAIAttack — the
-- instance->GetBossState(DATA_SAVIANA_RAGEFIRE) /
-- instance->GetBossState(DATA_BALTHARUS_THE_WARBORN) legs (gating
-- Reset's RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) / SetImmuneToPC(false)
-- and CanAIAttack) ride the instance-script model (no instance
-- bridge).
-- boss_general_zarithrian JustSummoned — summons.Summon(summon)
-- add-registration leg (no summon-registry bridge).
-- boss_general_zarithrian EnterEvadeMode — summons.DespawnAll() +
-- _DespawnAtEvade() legs (no summon-registry / evade-model bridges).
-- boss_general_zarithrian UpdateAI — 2026-10-14 retraction re-audit:
-- stale "no-timer-bridge" claim RETRACTED (CreateLuaEvent is a real
-- bridge); Talk(SAY_ADDS 2) rides EVENT_SUMMON_ADDS unconditionally
-- (:128-129, Talk then [[fallthrough]]) and is now PORTED on
-- 15s-init / 45s-repeat CreateLuaEvent (addsTick, below — the maiden
-- precedent; the unbridged summon legs stay documented-only). The
-- EVENT_SUMMON_ADDS/EVENT_SUMMON_ADDS2 stalker
-- CastSpell(SPELL_SUMMON_FLAMECALLER 74398) legs ride
-- instance->GetGuidData(DATA_ZARITHRIAN_SPAWN_STALKER_*) with no
-- instance bridge — documented-only; EVENT_SUMMON_ADDS2 carries no
-- Talk. DoCastSelf(SPELL_INTIMIDATING_ROAR 74384) /
-- DoCastVictim(SPELL_CLEAVE_ARMOR 74367) now ported (roarTick /
-- cleaveTick, 2026-10-13); DoMeleeAttackIfReady has no melee-model
-- bridge.
-- npc_onyx_flamecaller (NPC_ONYX_FLAMECALLER = 39814) — REGISTERED
-- (entry-verifiable, ruby_sanctum.h:73 — the bronjahm
-- not-registered verdict is superseded: C++ AddSC registers the
-- script and a spawned 39814 gets this AI in Go too; dormant
-- in-engine until the stalker summon path lands): JustEngagedWith
-- seeds EVENT_BLAST_NOVA 17s + EVENT_LAVA_GOUT 3s (ported below);
-- BLAST_NOVA 74392 — DoCastAOE = self-cast (non-triggered),
-- Repeat(15s, 20s); LAVA_GOUT 74394 — DoCastVictim (non-triggered)
-- with the 3-cast latch (_lavaGoutCount >= 3 -> reset + 8s re-arm;
-- else cast, count++, 1s re-arm); zero Talk lines; Reset's
-- MoveAlongSplineChain (POINT_GENERAL_ROOM) (no-motion-master
-- bridge), MovementInform DoZoneInCombat (no-movement-inform
-- bridge), IsSummonedBy zarithrian->AI()->JustSummoned(me) (no
-- cross-AI-summon bridge) stay out.
-- All 4 Talk() calls in the file accounted for and ported
-- (SAY_AGGRO 0 + SAY_KILL 1 + SAY_ADDS 2 + SAY_DEATH 3).

local ENTRY_GENERAL_ZARITHRIAN = 39746
local ENTRY_ONYX_FLAMECALLER = 39814

local SAY_AGGRO = 0
local SAY_KILL  = 1
local SAY_ADDS  = 2
local SAY_DEATH = 3

local SPELL_CLEAVE_ARMOR    = 74367
local SPELL_INTIMIDATING_ROAR = 74384
local SPELL_BLAST_NOVA      = 74392
local SPELL_LAVA_GOUT       = 74394

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

-- C++ EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE_ARMOR 74367)
-- (non-triggered), 8s init, re-ScheduleEvent(15s) (fixed, not Repeat).
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE_ARMOR)
    schedule(guid, "cleave", 15000, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_INTIDMDATING_ROAR: DoCastSelf(SPELL_INTIMIDATING_ROAR 74384)
-- (non-triggered), 14s init, Repeat(35s, 40s).
local function intimidatingRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_INTIMIDATING_ROAR)
    schedule(guid, "roar", {35000, 40000}, function()
        intimidatingRoarTick(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_ADDS: Talk(SAY_ADDS 2) rides the event arm
-- (:141-142 — Talk then [[fallthrough]] into the stalker summon legs);
-- 15s init (JustEngagedWith :97), Repeat(45s) (:151). The stalker
-- CastSpell(SPELL_SUMMON_FLAMECALLER 74398) legs ride
-- instance->GetGuidData(DATA_ZARITHRIAN_SPAWN_STALKER_*) with no
-- instance bridge — documented-only; the Talk arm is unconditional in
-- the event and ports C++-exact on the maiden CreateLuaEvent
-- convention (the 2026-10-14 retraction of the stale no-timer-bridge
-- claim). EVENT_SUMMON_ADDS2 (25-man, 16s) carries no Talk.
local function addsTick(creature, guid)
    creature:Talk(SAY_ADDS)
    schedule(guid, "adds", 45000, function()
        addsTick(creature, guid)
    end)
end

-- C++ boss_general_zarithrian::JustEngagedWith:
-- BossAI::JustEngagedWith(who); Talk(SAY_AGGRO);
-- events.ScheduleEvent(EVENT_CLEAVE, 8s);
-- events.ScheduleEvent(EVENT_INTIDMDATING_ROAR, 14s);
-- events.ScheduleEvent(EVENT_SUMMON_ADDS, 15s) (:97); [+ 16s 25-man
-- EVENT_SUMMON_ADDS2] — the auriaya precedent; the cleave/roar
-- timer legs ride the maiden CreateLuaEvent convention now (see
-- below). The EVENT_SUMMON_ADDS stalker-triggered 74398 casts
-- (instance GUID lookups, no instance bridge) stay documented-only;
-- the Talk(SAY_ADDS 2) arm is now ported (addsTick, below).
local function zarithrianEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "cleave", 8000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "roar", 14000, function()
        intimidatingRoarTick(creature, guid)
    end)
    schedule(guid, "adds", 15000, function()
        addsTick(creature, guid)
    end)
end

local function zarithrianLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function zarithrianReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_general_zarithrian::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function zarithrianKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_general_zarithrian::JustDied:
-- _JustDied(); Talk(SAY_DEATH) — the sjonnir precedent (the
-- _JustDied() instance leg has no bridge).
local function zarithrianDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 1, zarithrianEnterCombat)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 2, zarithrianLeaveCombat)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 3, zarithrianKilledUnit)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 4, zarithrianDied)
RegisterCreatureEvent(ENTRY_GENERAL_ZARITHRIAN, 23, zarithrianReset)

-- npc_onyx_flamecaller (C++-exact for all bridgeable arms): the
-- entry-verifiable (39814, ruby_sanctum.h:73) add AI is REGISTERED —
-- the "not registered because it never spawns (bronjahm precedent)"
-- verdict is superseded by the strip rule that hunts
-- unregistered-but-ported scripts; C++ AddSC registers it and a
-- spawned 39814 gets this AI in Go too. Dormant in-engine until the
-- stalker summon path lands (unchanged). Zero Talk lines in C++.
local flamecallerState = {}

local function flamecallerCancel(guid)
    cancelTimers(guid)
    flamecallerState[guid] = nil
end

-- C++ EVENT_BLAST_NOVA (:226-229): DoCastAOE(SPELL_BLAST_NOVA 74392)
-- non-triggered self-cast; Repeat(15s, 20s).
local function flamecallerBlastNovaTick(creature, guid)
    creature:CastSpell(creature, SPELL_BLAST_NOVA)
    schedule(guid, "blast_nova", {15000, 20000}, function()
        flamecallerBlastNovaTick(creature, guid)
    end)
end

-- C++ EVENT_LAVA_GOUT (:246-256): _lavaGoutCount >= 3 -> count = 0 +
-- Repeat(8s); else DoCastVictim(SPELL_LAVA_GOUT 74394) non-triggered,
-- count++, Repeat(1s).
local function flamecallerLavaGoutTick(creature, guid)
    local st = flamecallerState[guid]
    if st and st.lavaGout >= 3 then
        st.lavaGout = 0
        schedule(guid, "lava_gout", 8000, function()
            flamecallerLavaGoutTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_LAVA_GOUT)
    if st then
        st.lavaGout = st.lavaGout + 1
    end
    schedule(guid, "lava_gout", 1000, function()
        flamecallerLavaGoutTick(creature, guid)
    end)
end

-- C++ npc_onyx_flamecaller::JustEngagedWith (:200-204):
-- ScheduleEvent(EVENT_BLAST_NOVA, 17s) + ScheduleEvent(EVENT_LAVA_GOUT,
-- 3s). Reset (:195-199): _events.Reset() + _lavaGoutCount = 0 (the
-- motion/react legs have no bridges).
local function flamecallerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    flamecallerState[guid] = { lavaGout = 0 }
    schedule(guid, "blast_nova", 17000, function()
        flamecallerBlastNovaTick(creature, guid)
    end)
    schedule(guid, "lava_gout", 3000, function()
        flamecallerLavaGoutTick(creature, guid)
    end)
end

local function flamecallerLeaveCombat(event, creature)
    flamecallerCancel(creature:GetGUID())
end

local function flamecallerDied(event, creature, killer)
    flamecallerCancel(creature:GetGUID())
end

local function flamecallerReset(event, creature)
    flamecallerCancel(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 1, flamecallerEnterCombat)
RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 2, flamecallerLeaveCombat)
RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 4, flamecallerDied)
RegisterCreatureEvent(ENTRY_ONYX_FLAMECALLER, 23, flamecallerReset)
