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
-- 20.0f (GetPlayersInWorld + map/instance/alive/IsWithinDist filter,
-- maiden precedent) cast 44294 non-triggered; EVENT_DRAIN_CRYSTAL's
-- non-heroic 20s/25s (heroic 10s/15s) timing is modeled as the flag
-- re-arm (crystal selection itself stays out, above); heroic-only
-- EVENT_DRAIN_MANA: same drawn timer + 5s -> 10s loop: random-player
-- within 45.0f cast 46153 non-triggered (stale "no difficulty /
-- SelectTarget bridge" claims retracted — IsHeroic bridged,
-- engine/world/lua_creature_events.go:260).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so the fel-explosion timer fires unconditionally and
-- the in-loop casting gate is dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention). C++ gates the fel-explosion events on
-- PHASE_NORMAL (events.SetPhase(PHASE_DRAIN) stops them while Selin
-- drains a crystal); the drain phase has no bridges (below), so the
-- port's fel-explosion stays a straight loop — the faithful net
-- observable of the ported arms.
-- Reset() _Reset() / JustDied() _JustDied() / DoAction(
-- ACTION_SWITCH_PHASE)'s phase reset are internal BossAI/event
-- machinery covered by the cancel/re-arm on combat events (halycon
-- precedent); Reset's crystal Respawn PORTED in onCombatEnd
-- (GetCreaturesInGrid bridge); CrystalGUID.Clear() is vacuous in the
-- Lua model (drain legs re-search the nearest crystal each time). DoAction(ACTION_SWITCH_PHASE) is
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
--   AI()->DoAction(ACTION_SWITCH_PHASE) — the instance bridge is
--   absent; no .lua file written (stub-free precedent, grubbis).
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

local function randomTargetInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onDrainLife(creature, guid)
    local target = randomTargetInRange(creature, 20)
    if target then
        creature:CastSpell(target, SPELL_DRAIN_LIFE)
    end
    schedule(guid, "drain_life", 10000, function() onDrainLife(creature, guid) end)
end

-- C++ EVENT_DRAIN_MANA (heroic-only): SelectTarget(Random, 0, 45.0f)
-- -> DoCast(46153), non-triggered; re-arm ScheduleEvent(10s).
local function onDrainMana(creature, guid)
    local target = randomTargetInRange(creature, 45)
    if target then
        creature:CastSpell(target, SPELL_DRAIN_MANA)
    end
    schedule(guid, "drain_mana", 10000, function() onDrainMana(creature, guid) end)
end

local function onDrainCrystalReset(_, guid)
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
    creature:CastSpell(creature, SPELL_FEL_EXPLOSION)
    checkManaLatch(creature, guid)
    schedule(guid, "fel_explosion", 2000, function() onFelExplosion(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    drainScheduled[guid] = nil
    creature:Talk(SAY_AGGRO)
    schedule(guid, "fel_explosion", 2100, function() onFelExplosion(creature, guid) end)
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    drainScheduled[guid] = nil
    -- C++ Reset (:81-84): every fel crystal within 250.0f respawns
    -- (live crystals are a Respawn no-op, matching force-respawn).
    for _, crystal in ipairs(creature:GetCreaturesInGrid(ENTRY_FEL_CRYSTAL, 250)) do
        crystal:Respawn()
    end
end

local function onTargetDied(_, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (:163-169) via ShatterRemainingCrystals (:125-131):
    -- every fel crystal within 250.0f kills itself.
    for _, crystal in ipairs(creature:GetCreaturesInGrid(ENTRY_FEL_CRYSTAL, 250)) do
        crystal:Kill()
    end
end

RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 4, onDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 23, onCombatEnd)
