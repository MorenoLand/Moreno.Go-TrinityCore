-- Pit of Saron trash (Northrend) — Lua port of the creature AIs in
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/pit_of_saron.cpp
-- (AddSC_pit_of_saron — loader decl/call in northrend_script_loader.cpp,
-- the "Pit of Saron" block after AddSC_instance_pit_of_saron()).
-- Entries (wowhead-verified, C++ ScriptName bindings are DB-side — this
-- clone carries no creature_template table; pit_of_saron.h lists no
-- constants for these four, unlike the boss entries):
--   36893 Ymirjar Flamebearer (npc_ymirjar_flamebearer, C++ :58-121)
--   36891 Iceborn Proto-Drake (npc_iceborn_protodrake, C++ :123-175)
--   36886 Geist Ambusher (npc_geist_ambusher, C++ :177-234)
-- Corroboration: NPC_YMIRJAR_DEATHBRINGER = 36892 (pit_of_saron.h:93)
-- sits between the drake and the flamebearer in the same 3688x block.
-- Convention (violet_hold_trash.lua / boss_maiden_of_virtue.lua):
-- module-local GUID-keyed timers table, wipe on events 2 (leave
-- combat), 4 (died), 23 (reset); CreateLuaEvent takes plain ms or a
-- {minMs, maxMs} table for randtime re-arms;
-- creature:CastSpell(nil, id) = DoCastVictim;
-- creature:CastSpell(target, id) = DoCast(target);
-- creature:CastSpell(creature, id) = DoCast(me)/DoCastAOE self-cast;
-- all casts here are non-triggered.
-- C++ SelectTarget pools (UnitAI.h:162-184, 214-280): Random,0 with no
-- playerOnly flag draws the threat list, any unit; Random,0,dist,true
-- draws threat-list players within dist. Go SortedEntries returns dead
-- entries too while C++ skips offline refs (dead = offline), so every
-- pool below alive-filters (the violet_hold 11:58 correction).
-- DOCUMENTED DEVIATIONS: the flamebearer EventMap
-- UNIT_STATE_CASTING validator has no bridge — timers fire
-- unconditionally (hodir precedent); DoMeleeAttackIfReady is
-- engine-driven.
-- DOCUMENTED-ONLY (this file's remaining classes):
-- npc_pit_of_saron_icicle (C++ :236-277) — entry unsourced (summoned
-- creature, no pit_of_saron.h constant; spell 69424's summon target is
-- DB-side) and its IsSummonedBy arm needs RemoveDynObject(69424),
-- which has no bridge (engine/world grep: zero hits); its 3650ms
-- self-casts (69426 triggered + 69428) are port-ready via the ev23
-- spawn funnel once the entry is sourced. The ctor SetDisplayId
-- (Modelid1) is bridgeable since the tharon_ja SetDisplayId port but
-- rides the same unsourced entry.
-- spell_pos_ice_shards (C++ :279-315) — SpellScript, no
-- SpellScript/AuraScript bridge; its instance SetData
-- (DATA_ICE_SHARDS_HIT) leg is instance-gated too.
-- at_pit_cavern_entrance / at_pit_cavern_end (C++ :317-355) —
-- AreaTriggerScripts; trigger delivery exists (server event 24,
-- areatrigger.go) but both arms are instance legs (Get/SetData
-- DATA_CAVERN_ACTIVE, GetGuidData tyrannus Talk relay, achievement
-- credit) with no instance-script bridge — the marrowgar
-- unbridged-area-trigger queue.
-- ScheduledIcicleSummons (C++ :48-60) — BasicEvent on the cavern
-- trigger (NPC_CAVERN_EVENT_TRIGGER 32780): 12% icicle summon else
-- re-schedule 1-20s / 20-35s; no BasicEvent bridge, instance-side.

local ENTRY_FLAMEBEARER = 36893
local ENTRY_PROTODRAKE = 36891
local ENTRY_GEIST = 36886

local SPELL_FIREBALL = 69583
local SPELL_HELLFIRE = 69586
local SPELL_TACTICAL_BLINK = 69584
local SPELL_FROST_BREATH = 69527
local SPELL_LEAPING_FACE_MAUL = 69504

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

-- C++ SelectTarget(Random, 0) with no playerOnly flag and no distance:
-- threat-list any-unit, alive, distance-blind.
local function randomThreatAny(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 0, dist, true): threat-list players, alive,
-- within dist.
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
            and creature:GetDistance(t) <= range then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- npc_ymirjar_flamebearer JustEngagedWith (C++ :74-78): fireball 4s,
-- tactical blink 15s.
local function fireballTick(creature, guid)
    local target = randomThreatAny(creature)
    if target then
        creature:CastSpell(target, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 5000, function() fireballTick(creature, guid) end)
end

local function blinkTick(creature, guid)
    local target = randomThreatAny(creature)
    if target then
        creature:CastSpell(target, SPELL_TACTICAL_BLINK)
    end
    creature:CastSpell(creature, SPELL_HELLFIRE)
    schedule(guid, "blink", 12000, function() blinkTick(creature, guid) end)
end

-- npc_iceborn_protodrake UpdateAI (C++ :152-166): frost breath victim
-- cast, 5s init, 10s repeat; no casting check in C++ (plain cooldown).
local function frostBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_FROST_BREATH)
    schedule(guid, "frostbreath", 10000, function() frostBreathTick(creature, guid) end)
end

-- npc_geist_ambusher UpdateAI (C++ :210-224): leaping face maul on a
-- random threat-list player within 5yd, 9s init, {9,14}s re-arm;
-- re-arm is unconditional (outside the target if).
local function leapTick(creature, guid)
    local target = randomThreatPlayerInRange(creature, 5.0)
    if target then
        creature:CastSpell(target, SPELL_LEAPING_FACE_MAUL)
    end
    schedule(guid, "leap", {9000, 14000}, function() leapTick(creature, guid) end)
end

local function trashEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local entry = creature:GetEntry()
    if entry == ENTRY_FLAMEBEARER then
        schedule(guid, "fireball", 4000, function() fireballTick(creature, guid) end)
        schedule(guid, "blink", 15000, function() blinkTick(creature, guid) end)
    elseif entry == ENTRY_PROTODRAKE then
        -- JustEngagedWith vehicle leg (C++ :145-148, RemoveAllPassengers)
        -- stays out: no vehicle bridge (census precedent).
        schedule(guid, "frostbreath", 5000, function() frostBreathTick(creature, guid) end)
    elseif entry == ENTRY_GEIST then
        -- JustEngagedWith (C++ :199-207): engager is a player AND
        -- farther than 5yd -> leap on the engager.
        if target ~= nil and target:GetObjectType() == "Player"
            and creature:GetDistance(target) > 5.0 then
            creature:CastSpell(target, SPELL_LEAPING_FACE_MAUL)
        end
        schedule(guid, "leap", 9000, function() leapTick(creature, guid) end)
    end
end

local function trashCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

local TRASH_ENTRIES = {
    ENTRY_FLAMEBEARER,
    ENTRY_PROTODRAKE,
    ENTRY_GEIST,
}

for _, entry in ipairs(TRASH_ENTRIES) do
    RegisterCreatureEvent(entry, 1, trashEnterCombat)
    RegisterCreatureEvent(entry, 2, trashCancelCombatTimers)
    RegisterCreatureEvent(entry, 4, trashCancelCombatTimers)
    RegisterCreatureEvent(entry, 23, trashCancelCombatTimers)
end
