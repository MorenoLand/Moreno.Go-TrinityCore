-- Blackhand Incarcerator (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_pyroguard_emberseer.cpp
-- (npc_blackhand_incarcerator — ScriptedAI combat scheduler; the
-- file's other class, boss_pyroguard_emberseer, is ported in
-- boss_pyroguard_emberseer.lua). Boss/zone-script unit per
-- eastern_kingdoms_script_loader.cpp order (registered together with
-- the boss under AddSC_boss_pyroguardemberseer).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_BLACKHAND_INCARCERATOR = 10316 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (npc_phalanx / boss_the_beast precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI combat
-- event machine):
-- EVENT_STRIKE: victim-cast STRIKE 15580, 8s init -> 14s loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- C++ init ScheduleEvent(EVENT_STRIKE, 8s, 16s) and loop Repeat(
-- Seconds(14), Seconds(23)) are urand ranges; the recurring-timer
-- convention uses the lower bound for both (halycon precedent).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (maiden
-- precedent). The C++ scheduler is driven from JustEngagedWith and
-- drained by UpdateAI; the port schedules from OnEnterCombat(1) and
-- cancels on 2/4/23 (maiden convention).
-- EVENT_ENCAGE: random-player ENCAGE 16045 — SelectTarget(Random, 0,
-- 100, true) is playerOnly, ported via the keleseth GetPlayersInWorld
-- precedent (same map/instance, alive, within 100yd); urand(10s, 20s)
-- init -> urand(6s, 12s) loop, re-arm unconditional, TRIGGERED
-- DoCast(target, 16045, true). Nil-guard around the cast: C++ calls
-- DoCast directly on the SelectTarget result (fizzles on null); the
-- port guards instead of casting on nil.
-- Unmodeled (documented-only): JustAppeared's
-- DoCast(SPELL_ENCAGE_EMBERSEER 15282) and JustReachedHome's self-cast
-- 15282 + SetImmuneToAll(true) have no spawn/evade bridges on the Lua
-- surface. The JustEngagedWith grid loop (GetCreatureListWithEntryInGrid
-- NPC_BLACKHAND_INCARCERATOR 10316, 60.0f -> DoZoneInCombat each —
-- "Had to do this because CallForHelp will ignore any npcs without
-- LOS") has no creature-list / zone-in-combat bridges.

local SPELL_STRIKE = 15580
local SPELL_ENCAGE = 16045

local ENTRY_BLACKHAND_INCARCERATOR = 10316

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

local function onStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_STRIKE, true)
    end
    schedule(guid, "strike", 14000, function() onStrike(creature, guid) end)
end

-- C++ SelectTarget(Random, 0, dist, true) playerOnly=true arms ride the
-- keleseth GetPlayersInWorld precedent.
local function randomPlayerInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:IsWithinDist(plr, range) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_ENCAGE: SelectTarget(Random, 0, 100, true) playerOnly,
-- DoCast(target, 16045, true) TRIGGERED, ScheduleEvent(10s, 20s) init,
-- Repeat(6s, 12s) outside any guard (re-arm unconditional).
local function onEncage(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_ENCAGE, true)
    end
    schedule(guid, "encage", {6000, 12000}, function() onEncage(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "strike", 8000, function() onStrike(creature, guid) end)
    schedule(guid, "encage", {10000, 20000}, function() onEncage(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BLACKHAND_INCARCERATOR, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BLACKHAND_INCARCERATOR, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_BLACKHAND_INCARCERATOR, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_BLACKHAND_INCARCERATOR, 23, onCombatEnd)
