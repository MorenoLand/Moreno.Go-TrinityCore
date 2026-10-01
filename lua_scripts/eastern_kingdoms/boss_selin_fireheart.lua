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
-- precedent); Reset's crystal Respawn + CrystalGUID.Clear() gate on
-- the creature-list bridge (below). DoAction(ACTION_SWITCH_PHASE) is
-- only reachable from npc_fel_crystal's JustDied via the instance
-- bridge — no caller exists on the Lua surface.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - the sub-10%-mana latch + whole drain machine: creature
--   GetPower(POWER_MANA)/GetMaxPower is not bridged on creatures
--   (only the player methods in lua_player.go); without it the
--   PHASE_DRAIN transition never fires.
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
-- - EVENT_DRAIN_LIFE: SelectTarget(Random, 0, 20.0f) -> DoCast(44294)
--   — no SelectTarget bridge (the_beast / doomwalker precedent).
-- - EVENT_DRAIN_MANA: SelectTarget(Random, 0, 45.0f) -> DoCast(46153)
--   — no SelectTarget bridge, and the whole arm is heroic-gated (no
--   difficulty bridge).
-- - JustDied's ShatterRemainingCrystals: GetCreatureListWithEntryInGrid
--   + KillSelf — no creature-list bridge.
-- - npc_fel_crystal (the file's second CreatureScript class): its only
--   arm is JustDied -> instance->GetCreature(DATA_SELIN_FIREHEART)->
--   AI()->DoAction(ACTION_SWITCH_PHASE) — the instance bridge is
--   absent; no .lua file written (stub-free precedent, grubbis).
local SAY_AGGRO = 0
local SAY_KILL = 3
local SAY_DEATH = 4

local SPELL_FEL_EXPLOSION = 44314

local ENTRY_SELIN_FIREHEART = 24723

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

local function onFelExplosion(creature, guid)
    creature:CastSpell(creature, SPELL_FEL_EXPLOSION)
    schedule(guid, "fel_explosion", 2000, function() onFelExplosion(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "fel_explosion", 2100, function() onFelExplosion(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(_, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 4, onDied)
RegisterCreatureEvent(ENTRY_SELIN_FIREHEART, 23, onCombatEnd)
