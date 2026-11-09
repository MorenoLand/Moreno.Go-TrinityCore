-- Selin Fireheart (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_selin_fireheart.cpp
-- (boss_selin_fireheart — BossAI combat entry + fel-explosion scheduler +
-- KilledUnit talk + JustDied talk). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_felblood_kaelthas done:
-- boss class ported, entry 24664 registered; AddSC_boss_selin_fireheart
-- next in the Magister's Terrace block; boss_vexallus after;
-- instance_magisters_terrace.cpp stays blocked on the instance-script
-- model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h names
-- BOSS_SELIN_FIREHEART = 24723 in the MTCreatureIds enum (NPC_FEL_CRYSTAL
-- = 24722); the AI runs under BossAI(DATA_SELIN_FIREHEART = 0) via
-- GetMagistersTerraceAI (a GetInstanceAI retrieval wrapper only; the
-- ported arms have no instance state). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace), but the entry itself is
-- C++-verifiable so the port is registered (boss_felblood_kaelthas
-- precedent). Script name is the stringified class name
-- (CreatureScript("boss_selin_fireheart"); RegisterCreatureAIWithFactory
-- convention, felblood precedent).
-- Verifiable numbers (file's own enums/header): SAY_AGGRO = 0 /
-- SAY_ENERGY = 1 / SAY_EMPOWERED = 2 / SAY_KILL = 3 / SAY_DEATH = 4 /
-- EMOTE_CRYSTAL = 5; SPELL_FEL_CRYSTAL_DUMMY = 44329 /
-- SPELL_MANA_RAGE = 44320 (triggers 44321 via spell_script_target) /
-- SPELL_DRAIN_LIFE = 44294 / SPELL_FEL_EXPLOSION = 44314 /
-- SPELL_DRAIN_MANA = 46153 (heroic only); PHASE_NORMAL = 1 /
-- PHASE_DRAIN = 2; EVENT_FEL_EXPLOSION = 1 / EVENT_DRAIN_CRYSTAL /
-- EVENT_DRAIN_MANA / EVENT_DRAIN_LIFE / EVENT_EMPOWER;
-- ACTION_SWITCH_PHASE = 1.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied (C++ Eluna::KilledUnit — lua_creature_events.go fires it
-- with (event, creature, victim) from Unit::Kill), 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith schedule + UpdateAI event machine +
-- KilledUnit + JustDied):
-- Combat entry: Talk(SAY_AGGRO) (maiden-of-virtue convention) +
-- EVENT_FEL_EXPLOSION 2100ms -> 2s loop: DoCastAOE(44314) is
-- DoCast(nullptr, spell) — self-cast here via
-- creature:CastSpell(creature, spell) (boss_ai.go DoCastAOE
-- self-cast convention). Timers start on OnEnterCombat(1), cancelled
-- on OnLeaveCombat(2)/OnDied(4)/OnReset(23).
-- KilledUnit: Talk(SAY_KILL) via OnTargetDied(3); the C++ victim->
-- GetTypeId() == TYPEID_PLAYER gate is bridged by
-- victim:GetObjectType() == "Player" (lua_objects.go/lua_player.go
-- GetObjectType returns "Player"/"Creature"/"GameObject").
-- JustDied: Talk(SAY_DEATH) on OnDied(4) (najentus/halycon precedent).
-- Sub-10%-mana latch (maiden convention — checked from the PHASE_NORMAL
-- fel-explosion loop, GetPowerPct < 10, per-guid _scheduledEvents flag):
-- EVENT_DRAIN_LIFE randtime(3s,7s) -> 10s loop: random-player within
-- 20.0f (GetAITargets + alive/IsWithinDist filter) cast 44294 non-triggered; EVENT_DRAIN_CRYSTAL's
-- non-heroic 20s/25s (heroic 10s/15s) timing is modeled as the flag
-- re-arm (crystal selection itself stays out, above); heroic-only
-- EVENT_DRAIN_MANA: same drawn timer + 5s -> 10s loop: random threat-list
-- player within 45.0f cast 46153 non-triggered (stale "no difficulty /
-- SelectTarget bridge" claims retracted — IsHeroic bridged,
-- engine/world/lua_creature_events.go:260).
-- Deviations from C++: the UpdateAI pre-loop (:178) + in-loop
-- (:221) CASTING gates are ported as 500ms IsCasting defers under
-- each timer arm's own key (mal_ganis/rend idiom,
-- lua_creature_events.go:1950; no re-arm consumed). In-loop
-- same-tick co-expiry suppression is unmodeled (independent
-- callbacks, AV precedent). The fel-explosion arm's mana-latch
-- check defers with it (C++ skips the post-loop mana check on
-- a gated tick). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention). C++ gates the fel-explosion events on
-- PHASE_NORMAL (events.SetPhase(PHASE_DRAIN) stops them while Selin
-- drains a crystal); the drain phase has no bridges (below), so the
-- port's fel-explosion stays a straight loop — the faithful net
-- observable of the ported arms.
-- BossAI state legs PORTED below (creature:SetBossState bridge live
-- 2026-11-05, moroes/karazhan precedent): ev1 IN_PROGRESS (Talk-then-
-- base, boss_selin_fireheart.cpp:134-137), ev23 NOT_STARTED (Reset's
-- _Reset, alive + != DONE gated, ScriptedCreature.cpp:499-511), ev4
-- DONE (JustDied's _JustDied, ScriptedCreature.cpp:513-520). Reset's
-- crystal Respawn stays in onReset (GetCreaturesInGrid bridge);
-- CrystalGUID.Clear() is vacuous in the Lua model (drain legs
-- re-search the nearest crystal each time). DoAction(ACTION_SWITCH_PHASE) is
-- only reachable from npc_fel_crystal's JustDied via the instance
-- bridge — no caller exists on the Lua surface.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - the sub-10%-mana latch -> EVENT_DRAIN_LIFE now ported (below);
--   the latch's EVENT_DRAIN_CRYSTAL 20s/25s (heroic 10s/15s) leg is
--   modeled as a flag-reset only (SelectNearestCrystal needs
--   FindNearestCreature + MotionMaster, unbridged); the old
--   "GetPower not bridged on creatures" claim is retracted
--   (GetPowerPct is bridged on generic objects,
--   engine/scripting/object.go), and the stale "no difficulty
--   bridge" for the heroic fork is retracted (creature:IsHeroic()
--   bridged, engine/world/lua_creature_events.go:260). The whole
--   PHASE_DRAIN machine below stays out on missing motion/flag/kill
--   legs.
-- - EVENT_DRAIN_CRYSTAL -> SelectNearestCrystal():
--   FindNearestCreature(NPC_FEL_CRYSTAL = 24722, 250.0f) + Talk(
--   SAY_ENERGY) + Talk(EMOTE_CRYSTAL) + DoCast(crystal,
--   SPELL_FEL_CRYSTAL_DUMMY 44329) + GetClosePoint + SetWalk(false) +
--   MovePoint(1, ...) — no creature-list/MotionMaster bridges.
-- - MovementInform(POINT_MOTION_TYPE, 1): crystal RemoveFlag(
--   UNIT_FLAG_NOT_SELECTABLE) + crystal CastSpell(me, SPELL_MANA_RAGE
--   44320, triggered) + EVENT_EMPOWER 10s — no MovementInform bridge.
-- - EVENT_EMPOWER: Talk(SAY_EMPOWERED) + crystal KillSelf +
--   MotionMaster Clear + MoveChase(victim) — no MotionMaster bridge.
-- - EVENT_DRAIN_LIFE now ported (below); stale "no SelectTarget
--   bridge" claim retracted (maiden_of_virtue precedent).
-- - EVENT_DRAIN_MANA now ported (below, heroic-only): SelectTarget(
--   Random, 0, 45.0f) -> DoCast(46153) non-triggered, 10s loop;
--   stale "no SelectTarget / difficulty bridge" claims retracted.
-- - JustDied's ShatterRemainingCrystals PORTED in onDied
--   (GetCreaturesInGrid + Kill() self-kill twin).
-- - npc_fel_crystal (the file's second CreatureScript class): its only
--   arm is JustDied -> instance->GetCreature(DATA_SELIN_FIREHEART)->
--   AI()->DoAction(ACTION_SWITCH_PHASE) — PORTED below (GetInstanceCreature
--   + DoAction bridges, tick 236): ev5 SetGuidData(0) latch on Selin,
--   ev4 crystal relay, ev38 phase-switch re-arm.
local SAY_AGGRO = 0
local SAY_KILL = 3
local SAY_DEATH = 4

local SPELL_FEL_EXPLOSION = 44314
local SPELL_DRAIN_LIFE = 44294
local SPELL_DRAIN_MANA = 46153

local ENTRY_SELIN_FIREHEART = 24723
local ENTRY_FEL_CRYSTAL = 24722 -- NPC_FEL_CRYSTAL (magisters_terrace.h:61)

local timers = {}
local drainScheduled = {}

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

-- C++ SelectTarget(Random, 0, 20/45, true) (boss_selin_fireheart.cpp:199/:194):
-- threat-list players alive within range (UnitAI.h:162-176).
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, p in ipairs(creature:GetAITargets()) do
        if p:GetObjectType() == "Player" and not p:IsDead()
                and creature:GetDistance(p) - creature:GetCombatReach() - 1.5 < range then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5)
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onDrainLife(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "drain_life", 500, function() onDrainLife(creature, guid) end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 20)
    if target then
        creature:CastSpell(target, SPELL_DRAIN_LIFE)
    end
    schedule(guid, "drain_life", 10000, function() onDrainLife(creature, guid) end)
end

-- C++ EVENT_DRAIN_MANA (heroic-only): SelectTarget(Random, 0, 45.0f, true)
-- -> DoCast(46153), non-triggered; re-arm ScheduleEvent(10s).
local function onDrainMana(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "drain_mana", 500, function() onDrainMana(creature, guid) end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_DRAIN_MANA)
    end
    schedule(guid, "drain_mana", 10000, function() onDrainMana(creature, guid) end)
end

local function onDrainCrystalReset(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "drain_crystal", 500, function() onDrainCrystalReset(creature, guid) end)
        return
    end
    drainScheduled[guid] = false
end

local function checkManaLatch(creature, guid)
    if not drainScheduled[guid] and creature:GetPowerPct() < 10 then
        drainScheduled[guid] = true
        -- C++ draws the DRAIN_LIFE timer once (randtime(3s,7s)) and
        -- reuses it for the heroic DRAIN_MANA (timer + 5s).
        local timer = math.random(3000, 7000)
        schedule(guid, "drain_life", timer, function() onDrainLife(creature, guid) end)
        if creature:IsHeroic() then
            schedule(guid, "drain_crystal", {10000, 15000}, function() onDrainCrystalReset(creature, guid) end)
            schedule(guid, "drain_mana", timer + 5000, function() onDrainMana(creature, guid) end)
        else
            schedule(guid, "drain_crystal", {20000, 25000}, function() onDrainCrystalReset(creature, guid) end)
        end
    end
end

local function onFelExplosion(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fel_explosion", 500, function() onFelExplosion(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FEL_EXPLOSION)
    checkManaLatch(creature, guid)
    schedule(guid, "fel_explosion", 2000, function() onFelExplosion(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    drainScheduled[guid] = nil
    creature:Talk(SAY_AGGRO)
    -- Talk-then-base (boss_selin_fireheart.cpp:134-137 -> BossAI::
    -- JustEngagedWith, ScriptedCreature.cpp:527-542): SetBossState
    -- IN_PROGRESS, then pulse 5 + DoZoneInCombat.
    creature:SetBossState(0, 1) -- DATA_SELIN_FIREHEART, IN_PROGRESS
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    schedule(guid, "fel_explosion", 2100, function() onFelExplosion(creature, guid) end)
end

local function onLeaveCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    drainScheduled[guid] = nil
end

-- C++ Reset (boss_selin_fireheart.cpp:81-90): crystal respawn first,
-- then _Reset() -> SetBossState(NOT_STARTED), alive-gated and
-- != DONE-gated (ScriptedCreature.cpp:499-511). Go fires ev23 on the
-- death path before ev4 (maiden precedent), so both gates are live
-- here; the ev4 shatter below still runs after the respawn.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    drainScheduled[guid] = nil
    -- C++ Reset (:81-84): every fel crystal within 250.0f respawns
    -- (live crystals are a Respawn no-op, matching force-respawn).
    for _, crystal in ipairs(creature:GetCreaturesInGrid(ENTRY_FEL_CRYSTAL, 250)) do
        crystal:Respawn()
    end
    if creature:IsAlive() and creature:GetBossState(0) ~= 3 then
        creature:SetBossState(0, 0) -- DATA_SELIN_FIREHEART, NOT_STARTED
    end
end

local function onTargetDied(_, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:163-166): Talk, then _JustDied() ->
    -- SetBossState(DONE) (ScriptedCreature.cpp:513-520), then
    -- ShatterRemainingCrystals (:125-131).
    creature:SetBossState(0, 3) -- DATA_SELIN_FIREHEART, DONE
    -- C++ JustDied (:163-169) via ShatterRemainingCrystals (:125-131):
    -- every fel crystal within 250.0f kills itself.
    for _, crystal in ipairs(creature:GetCreaturesInGrid(ENTRY_FEL_CRYSTAL, 250)) do
        crystal:Kill()
    end
end

RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 4, onDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 23, onReset)

-- C++ boss_selin_fireheartAI spawn latch (tick-235 precedent: ev5
-- SetGuidData on the DATA key) — the crystal relay reads it back via
-- GetInstanceCreature.
local function onSelinSpawn(_, creature)
    creature:SetGuidData(0) -- DATA_SELIN_FIREHEART (magisters_terrace.h:31)
end

-- C++ npc_fel_crystalAI::JustDied (boss_selin_fireheart.cpp:267-272):
-- instance->GetCreature(DATA_SELIN_FIREHEART), if alive -> DoAction(1).
local function onCrystalDied(_, creature)
    local selin = creature:GetInstanceCreature(0)
    if selin and selin:IsAlive() then
        selin:DoAction(1) -- ACTION_SWITCH_PHASE
    end
end

-- C++ boss_selin_fireheartAI::DoAction ACTION_SWITCH_PHASE (:91-101):
-- SetPhase(PHASE_NORMAL), schedule EVENT_FEL_EXPLOSION in 2s,
-- AttackStart(GetVictim()) + MoveChase(GetVictim()). The phase flip has
-- no Lua model (drain legs already re-search the crystal); the timer
-- re-arm rides the file's fel_explosion schedule (cancel-and-replace).
local function onSelinDoAction(_, creature, action)
    if action ~= 1 then
        return
    end
    local guid = creature:GetGUIDLow()
    schedule(guid, "fel_explosion", 2000, function() onFelExplosion(creature, guid) end)
    local victim = creature:GetVictim()
    if victim then
        creature:AttackStart(victim)
        creature:MoveChase(victim)
    end
end

RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 5, onSelinSpawn)
RegisterCreatureEvent(ENTRY_FEL_CRYSTAL, 4, onCrystalDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 38, onSelinDoAction)
