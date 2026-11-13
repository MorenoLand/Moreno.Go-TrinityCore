-- Earthen Ring Flamecaller (The Slave Pens, Midsummer Fire
-- Festival) — Lua port of the npc_earthen_ring_flamecaller AI in
-- src/server/scripts/Outland/CoilfangReservoir/TheSlavePens/
-- boss_ahune.cpp (boss_ahune.cpp:477-586) with its enums
-- (boss_ahune.cpp:100-166) and the_slave_pens.h (SPDataTypes).
-- Entry 25754 NPC_EARTHEN_RING_FLAMECALLER (the_slave_pens.h).
-- The flamecallers are the three static spawns the Ahune bunny
-- drives: ACTION_START_EVENT MovePoints them to FlameCallerSpots,
-- then SHAMANS_LOOK_FOR_OPENING / FOUND_OPENING casts and the
-- bunny's ResetFlameCallers evade relay reach them through the
-- instance guid store (instance_the_slave_pens.lua latches
-- DATA_FLAMECALLER_000/001/002 = 7/8/9 in spawn order). Those
-- bunny-side triggers are unported (npc_ahune_bunny is not a
-- registered script); every arm below is the flamecaller's own
-- and fires whenever its trigger arrives, from any caster.
--
-- PORTED — MovementInform (ev28, POINT_MOTION_TYPE = 8, marzon
-- precedent): pointId 0/1/2 records _mySpot and faces the
-- FlameCallerSpots orientation, then the channel self-cast fires
-- for ANY point arrival (C++ DoCastSelf sits after the switch,
-- boss_ahune.cpp:487-515). SetFacingTo is the SetOrientation
-- twin (lua_creature_events.go:4844 — SetOrientation itself is
-- the raw slot write with no packet; SetFacingTo is its slot
-- half plus the turn broadcast).
-- PORTED — SpellHit (ev14, attumen precedent, args (event,
-- creature, caster, spellId)): SHAMANS_LOOK_FOR_OPENING (46422)
-- schedules the look-for-opening chain at 17s; FOUND_OPENING
-- (46421) talks EMOTE_RETREAT (0).
-- PORTED — DoAction (ev38, lescovar precedent):
-- ACTION_EMOTE_RESURFACE (-2575400) talks EMOTE_RESURFACE (1).
-- PORTED — look-for-opening chain (UpdateAI EventMap,
-- boss_ahune.cpp:540-566, via one-shot CreateLuaEvent stages):
-- stage 0 LookOpening(true, 0) at 17s, stage 1 (true, 1) +26s,
-- stage 2 (true, 2) +25s, stop LookOpening(false, _mySpot) +27s.
-- LookOpening (boss_ahune.cpp:568-586) returns unless _mySpot
-- == spot; both bunnies resolve through the instance guid store
-- (creature:GetInstanceCreature, DATA_BONFIRE_BUNNY_000 + spot
-- = 10 + spot, DATA_BEAM_BUNNY_000 + spot = 13 + spot).
-- Activate: DoCast(bonfire, FIND_OPENING_TRIGGER 46341,
-- non-triggered) + bonfire:CastSpell(beam, FIND_OPENING_VISUAL
-- 45964, triggered). Deactivate: DoCastSelf(FIND_OPENING_
-- CHANNEL 46345) + bonfire CastStop + beam RemoveAurasDueToSpell
-- (FIND_OPENING_BEAM_END 46333). CastStop has no direct bridge:
-- bonfire:InterruptSpell(1) (GENERIC) is the twin (renataki
-- precedent); RemoveAurasDueToSpell rides creature:RemoveAura
-- (mushroom precedent).
-- PORTED — Reset (ev23): C++ Reset() is _events.Reset(), so a
-- reset cancels any pending chain stage (per-GUID timer id).
--
-- DELTA: a second SHAMANS_LOOK_FOR_OPENING hit while a chain is
-- running restarts the chain from stage 0 (C++ EventMap would
-- double-schedule it); the casts are idempotent, so the restart
-- is the faithful-enough Lua idiom.

local NPC_EARTHEN_RING_FLAMECALLER = 25754

local POINT_MOTION_TYPE = 8

local SPELL_FIND_OPENING_VISUAL = 45964
local SPELL_FIND_OPENING_BEAM_END = 46333
local SPELL_FIND_OPENING_TRIGGER = 46341
local SPELL_FIND_OPENING_CHANNEL = 46345
local SPELL_FOUND_OPENING = 46421
local SPELL_SHAMANS_LOOK_FOR_OPENING = 46422

local EMOTE_RETREAT = 0
local EMOTE_RESURFACE = 1

local ACTION_EMOTE_RESURFACE = -2575400

local DATA_BONFIRE_BUNNY_000 = 10
local DATA_BEAM_BUNNY_000 = 13

-- boss_ahune.cpp:164-172 FlameCallerSpots orientations.
local FLAMECALLER_ORIENTATIONS = { 5.427049, 5.336747, 5.460842 }

-- boss_ahune.cpp:542-557 chain delays (ms).
local LOOKFOROPENING_0_MS = 17000
local LOOKFOROPENING_1_MS = 26000
local LOOKFOROPENING_2_MS = 25000
local STOP_LOOKING_MS = 27000

local spots = {}
local timers = {}

local function cancelChain(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetState(guid)
    cancelChain(guid)
    spots[guid] = nil
end

-- C++ LookOpening (boss_ahune.cpp:568-586).
local function lookOpening(creature, guid, activate, spot)
    if (spots[guid] or 0) ~= spot then
        return
    end
    local bonfire = creature:GetInstanceCreature(DATA_BONFIRE_BUNNY_000 + spot)
    local beam = creature:GetInstanceCreature(DATA_BEAM_BUNNY_000 + spot)
    if bonfire == nil or beam == nil then
        return
    end
    if activate then
        -- C++ DoCast(bonfireBunny, ...) — the flamecaller casts on
        -- the bonfire bunny; the bunny is the target, not the caster.
        creature:CastSpell(bonfire, SPELL_FIND_OPENING_TRIGGER)
        bonfire:CastSpell(beam, SPELL_FIND_OPENING_VISUAL, true)
    else
        creature:CastSpell(creature, SPELL_FIND_OPENING_CHANNEL)
        bonfire:InterruptSpell(1) -- C++ CastStop twin (see header)
        beam:RemoveAura(SPELL_FIND_OPENING_BEAM_END)
    end
end

local function scheduleStage(creature, guid, stage)
    local delays = { LOOKFOROPENING_0_MS, LOOKFOROPENING_1_MS, LOOKFOROPENING_2_MS, STOP_LOOKING_MS }
    timers[guid] = CreateLuaEvent(function()
        timers[guid] = nil
        if stage <= 2 then
            lookOpening(creature, guid, true, stage)
            scheduleStage(creature, guid, stage + 1)
        else
            lookOpening(creature, guid, false, spots[guid] or 0)
        end
    end, delays[stage + 1])
end

-- C++ MovementInform (boss_ahune.cpp:487-515).
RegisterCreatureEvent(NPC_EARTHEN_RING_FLAMECALLER, 28, function(_, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUIDLow()
    if pointId >= 0 and pointId <= 2 then
        spots[guid] = pointId
        creature:SetFacingTo(FLAMECALLER_ORIENTATIONS[pointId + 1])
    end
    creature:CastSpell(creature, SPELL_FIND_OPENING_CHANNEL)
end)

-- C++ SpellHit (boss_ahune.cpp:517-530).
RegisterCreatureEvent(NPC_EARTHEN_RING_FLAMECALLER, 14, function(_, creature, caster, spellId)
    if spellId == SPELL_SHAMANS_LOOK_FOR_OPENING then
        local guid = creature:GetGUIDLow()
        cancelChain(guid)
        scheduleStage(creature, guid, 0)
    elseif spellId == SPELL_FOUND_OPENING then
        creature:Talk(EMOTE_RETREAT)
    end
end)

-- C++ DoAction (boss_ahune.cpp:532-536).
RegisterCreatureEvent(NPC_EARTHEN_RING_FLAMECALLER, 38, function(_, creature, action)
    if action == ACTION_EMOTE_RESURFACE then
        creature:Talk(EMOTE_RESURFACE)
    end
end)

-- C++ Reset (boss_ahune.cpp:482-485): _events.Reset().
RegisterCreatureEvent(NPC_EARTHEN_RING_FLAMECALLER, 23, function(_, creature)
    resetState(creature:GetGUIDLow())
end)
