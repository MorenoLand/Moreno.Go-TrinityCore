-- Enslaved Proto-Drake (Utgarde Keep) -- Lua port of npc_enslaved_proto_drake
-- in src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/utgarde_keep.cpp
-- (Reset :274-280, UpdateAI :300-330). MovementInform (:282-288) and
-- SetData (:290-298) legs are STAY-OUT: no AnimationTier/MovePath bridge
-- and no SetData dispatch hook. Timer arms only.
-- Entry 24083 ultimowow-only (no C++ NPC_ constant for this trash mob;
-- UltimoWoW WotLK database lists "Enslaved Proto-Drake", level 70 Elite,
-- Dragonkin, Utgarde Keep).

local ENTRY_PROTO_DRAKE = 24083

local SPELL_REND = 43931
local SPELL_FLAME_BREATH = 50653
local SPELL_KNOCK_AWAY = 49722

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

-- C++ EVENT_REND (:315-316): DoCast(SPELL_REND), non-triggered;
-- re-arm {15s,20s}. CastSpell(nil, ...) = victim-cast.
local function rendTick(creature, guid)
    creature:CastSpell(nil, SPELL_REND)
    schedule(guid, "rend", {15000, 20000}, function()
        rendTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_BREATH (:319-320): DoCast(SPELL_FLAME_BREATH),
-- non-triggered; re-arm {11s,12s}.
local function flameBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_FLAME_BREATH)
    schedule(guid, "flame", {11000, 12000}, function()
        flameBreathTick(creature, guid)
    end)
end

-- C++ EVENT_KNOCKAWAY (:323-324): DoCast(SPELL_KNOCK_AWAY),
-- non-triggered; re-arm {7s,8500ms}.
local function knockAwayTick(creature, guid)
    creature:CastSpell(nil, SPELL_KNOCK_AWAY)
    schedule(guid, "knockaway", {7000, 8500}, function()
        knockAwayTick(creature, guid)
    end)
end

-- C++ Reset init (:277-279), armed here on EnterCombat, not Reset:
-- C++ EventMap timers do not advance without a victim (UpdateVictim
-- gate :301), while CreateLuaEvent runs in real time, so Reset-arming
-- would drift pre-combat. Init delays are C++-exact. The C++
-- HasUnitState(CASTING) gate (:309-311) is skipped per the caststate
-- axiom (handler IsCasting is constant-false; no in-flight cast state).
local function protoDrakeEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rend", {2000, 3000}, function()
        rendTick(creature, guid)
    end)
    schedule(guid, "flame", {5500, 7000}, function()
        flameBreathTick(creature, guid)
    end)
    schedule(guid, "knockaway", {3500, 6000}, function()
        knockAwayTick(creature, guid)
    end)
end

local function protoDrakeCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PROTO_DRAKE, 1, protoDrakeEnterCombat)
RegisterCreatureEvent(ENTRY_PROTO_DRAKE, 2, protoDrakeCancel)
RegisterCreatureEvent(ENTRY_PROTO_DRAKE, 4, protoDrakeCancel)
RegisterCreatureEvent(ENTRY_PROTO_DRAKE, 23, protoDrakeCancel)
