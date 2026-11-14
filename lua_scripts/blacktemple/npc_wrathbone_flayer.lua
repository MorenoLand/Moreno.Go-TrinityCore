-- Wrathbone Flayer (Black Temple) --
-- Lua port of src/server/scripts/Outland/BlackTemple/black_temple.cpp
-- (npc_wrathbone_flayer — Reset channeler machine + JustEngagedWith
-- combat timers). Entry 22953 'Wrathbone Flayer', TDB-verified
-- (ScriptName npc_wrathbone_flayer, 2 spawns map 564).
-- Eluna creature events: 23 OnReset, 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied.
-- C++ Reset (black_temple.cpp:73-79): schedule EVENT_GET_CHANNELERS
-- 3s; Initialize() (_enteredCombat = false); clear the bloodmage /
-- deathshaper GUID lists. The events it seeds are the UpdateAI
-- out-of-combat branch (:99-161): GET_CHANNELERS gathers Blood Mages
-- (22945) + Deathshapers (22882) in 15y, respawns dead ones, then
-- schedules SET_CHANNELERS 3s; SET_CHANNELERS casts
-- SPELL_SUMMON_CHANNEL (40094) on each listed creature, re-arms 12s.
-- The gather fires once per Reset; the channel repeats every 12s.
-- C++ JustEngagedWith (:83-88): schedule EVENT_CLEAVE 5s +
-- EVENT_IGNORED 7s; _enteredCombat = true (which stops the
-- out-of-combat event branch — the channeler loop ends on engage).
-- The engage arms are the UpdateAI combat branch (:163-180): CLEAVE
-- DoCastVictim(15496), re-arm {1s,2s}; IGNORED random-target
-- DoCast(39544), re-arm 10s.
-- C++ JustDied (:81) is empty — no ev4 arm, but ev4 cancels the
-- timers (a dead flayer runs no timers; spawn re-seeds).
-- Bridges (all live, verified this round): GetCreaturesInRange
-- (lua_worldobject_range.go:623; args range/entry/hostile/dead, dead 0
-- = both — C++ gathers dead ones too); creature:Respawn
-- (lua_creature_events.go:3394) + creature:IsDead (lua_objects.go:352);
-- GetCreatureByGUID (:4383); creature:CastSpell(nil, spell) =
-- DoCastVictim (felmyst convention); GetPlayersInRange
-- (lua_worldobject_range.go:608; default range 533.33y, dead default 1
-- = alive-only).
-- Documented deltas: the channel cast ports as self-cast
-- creature:CastSpell(c, 40094) (C++ CastSpell(nullptr, 40094));
-- IGNORED picks a random player in range, not a random threat-list
-- target — there is NO SelectTarget bridge (only SelectVictim, :4614,
-- no-arg); the range-list pick is the in-tree substitute.

local WRATHBONE_FLAYER_ENTRY = 22953
local NPC_BLOOD_MAGE = 22945
local NPC_DEATHSHAPER = 22882
local SPELL_CLEAVE = 15496
local SPELL_IGNORED = 39544
local SPELL_SUMMON_CHANNEL = 40094

local timers = {}
local flayerState = {}

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
    if per == nil then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function cancelOutOfCombat(guid)
    local per = timers[guid]
    if per then
        for _, key in ipairs({ "gather", "channel" }) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
end

-- C++ EVENT_SET_CHANNELERS: cast 40094 on each listed creature,
-- re-arm 12s. GUIDs re-resolve at fire time (C++ ObjectAccessor
-- lookup per fire).
local function channelFire(creature, guid)
    local st = flayerState[guid]
    if not st then
        return
    end
    for _, list in ipairs({ st.bloodmages, st.deathshapers }) do
        for _, targetGuid in ipairs(list) do
            local target = creature:GetCreatureByGUID(targetGuid)
            if target then
                target:CastSpell(target, SPELL_SUMMON_CHANNEL)
            end
        end
    end
    schedule(guid, "channel", 12000, function() channelFire(creature, guid) end)
end

-- C++ EVENT_GET_CHANNELERS: gather both entries in 15y, respawn
-- dead ones, store GUIDs, schedule SET_CHANNELERS 3s.
local function gatherFire(creature, guid)
    local mages = creature:GetCreaturesInRange(15, NPC_BLOOD_MAGE, 0, 0)
    local shapers = creature:GetCreaturesInRange(15, NPC_DEATHSHAPER, 0, 0)
    local bloodmages = {}
    local deathshapers = {}
    for _, mage in ipairs(mages) do
        if mage:IsDead() then
            mage:Respawn()
        end
        bloodmages[#bloodmages + 1] = mage:GetGUIDLow()
    end
    for _, shaper in ipairs(shapers) do
        if shaper:IsDead() then
            shaper:Respawn()
        end
        deathshapers[#deathshapers + 1] = shaper:GetGUIDLow()
    end
    flayerState[guid] = { bloodmages = bloodmages, deathshapers = deathshapers }
    schedule(guid, "channel", 3000, function() channelFire(creature, guid) end)
end

-- C++ Reset (:73-79): schedule GET_CHANNELERS 3s, clear the lists.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    flayerState[guid] = { bloodmages = {}, deathshapers = {} }
    schedule(guid, "gather", 3000, function() gatherFire(creature, guid) end)
end

-- C++ EVENT_CLEAVE: DoCastVictim(15496), re-arm urand(1s, 2s).
local function cleaveFire(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(1000, 2000), function() cleaveFire(creature, guid) end)
end

-- C++ EVENT_IGNORED: random-target DoCast(39544), re-arm 10s.
local function ignoredFire(creature, guid)
    local players = creature:GetPlayersInRange()
    if #players > 0 then
        creature:CastSpell(players[math.random(#players)], SPELL_IGNORED)
    end
    schedule(guid, "ignored", 10000, function() ignoredFire(creature, guid) end)
end

-- C++ JustEngagedWith (:83-88): schedule CLEAVE 5s + IGNORED 7s;
-- the out-of-combat channeler loop ends on engage.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelOutOfCombat(guid)
    schedule(guid, "cleave", 5000, function() cleaveFire(creature, guid) end)
    schedule(guid, "ignored", 7000, function() ignoredFire(creature, guid) end)
end

local function onLeaveCombatOrDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    flayerState[guid] = nil
end

RegisterCreatureEvent(WRATHBONE_FLAYER_ENTRY, 23, onReset)
RegisterCreatureEvent(WRATHBONE_FLAYER_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(WRATHBONE_FLAYER_ENTRY, 2, onLeaveCombatOrDied)
RegisterCreatureEvent(WRATHBONE_FLAYER_ENTRY, 4, onLeaveCombatOrDied)
