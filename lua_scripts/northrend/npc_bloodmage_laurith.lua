-- Bloodmage Laurith (Borean Tundra, quest 11719) --
-- Lua port of npc_bloodmage_laurithAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 1733-1806;
-- AddSC at line 1887, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_BLOODMAGE_LAURITH = 25381 (:1699).
-- Eluna creature events: 23 OnReset, 40 OnSetGUID, 4 OnDied (death cleanup).
-- The UpdateAI event machine rides a CreateLuaEvent chain (beryl/talbot
-- convention); melee is engine-driven (DoMeleeAttackIfReady).
-- Ported arms (C++ order, :1742-1804):
-- - Reset: _events.Reset() + _playerGUID.Clear() — cancel pending
--   talk/orientation timers, clear the latched player GUID. Death (ev4)
--   runs the same clear: C++ events die with the AI object.
-- - SetGUID (ev40, fired by the ported spell_q11719 bloodspore leg in
--   zone_borean_tundra.lua: bloodsporeRuinationHit calls
--   laurith:SetGUID(caster:GetGUIDLow(), 0); lua_creature_events.go:862-898
--   dispatches CREATURE_EVENT_ON_SET_GUID (40) with (event, creature,
--   guid, id), simon-bunny precedent): first SetGUID wins (the C++
--   _playerGUID empty gate); the creature faces the player
--   (SetFacingToObject, lua_creature_events.go:5209) and EVENT_TALK is
--   scheduled at 1s.
-- - EVENT_TALK (1s): Talk(SAY_BLOODMAGE_LAURITH 0, player) — the player
--   re-resolved via GetPlayerByGUID (the C++ ObjectAccessor::GetPlayer
--   twin, phase-hunter precedent); the latch clears, then
--   EVENT_RESET_ORIENTATION is scheduled at 5s.
-- - EVENT_RESET_ORIENTATION (5s): SetFacingTo(home orientation)
--   (GetHomePosition returns x, y, z, o — lua_creature_events.go:6871).
-- DELTA: C++ UpdateAI only advances the event machine while
-- UpdateVictim() is false (combat pauses it); the Lua chain runs
-- ungated — the only producer is the out-of-combat quest cast, so the
-- delta is behaviorally nil.

local ENTRY_BLOODMAGE_LAURITH = 25381

local SAY_BLOODMAGE_LAURITH = 0

local timers = {}
local playerGUIDs = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- Forward declarations (tharon_ja / gal_darah idiom): the SetGUID
-- handler schedules onTalk, which schedules onResetOrientation; both
-- are assigned below with plain assignments, never a second local function.
local onTalk
local onResetOrientation

-- C++ Reset() (:1742-1746): _events.Reset() + _playerGUID.Clear().
-- Death runs the same clear: the C++ event map dies with the AI.
local function clearState(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    playerGUIDs[guid] = nil
end

-- C++ SetGUID (:1748-1759): first SetGUID wins; face the player;
-- EVENT_TALK at 1s. The guid arrives as the low half (GetGUIDLow
-- convention); the player is resolved through GetPlayerByGUID.
local function onSetGUID(_, creature, playerGuid, id)
    local guid = creature:GetGUIDLow()
    if playerGUIDs[guid] ~= nil then
        return
    end
    playerGUIDs[guid] = playerGuid
    local player = GetPlayerByGUID(playerGuid)
    if player ~= nil then
        creature:SetFacingToObject(player)
    end
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per.talk = CreateLuaEvent(function() onTalk(creature, guid) end, 1000)
end

-- C++ EVENT_TALK (:1774-1780): Talk(SAY, player), latch clears,
-- EVENT_RESET_ORIENTATION at 5s. A vanished player skips only the
-- Talk; the clear and the schedule still run, exactly like C++.
onTalk = function(creature, guid)
    local playerGuid = playerGUIDs[guid]
    if playerGuid ~= nil then
        local player = GetPlayerByGUID(playerGuid)
        if player ~= nil then
            creature:Talk(SAY_BLOODMAGE_LAURITH, player)
        end
    end
    playerGUIDs[guid] = nil
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per.talk = nil
    per.orient = CreateLuaEvent(function() onResetOrientation(creature, guid) end, 5000)
end

-- C++ EVENT_RESET_ORIENTATION (:1781-1783): SetFacingTo(home orientation).
onResetOrientation = function(creature, guid)
    local _, _, _, homeO = creature:GetHomePosition()
    creature:SetFacingTo(homeO)
    local per = timers[guid]
    if per then
        per.orient = nil
    end
end

RegisterCreatureEvent(ENTRY_BLOODMAGE_LAURITH, 23, clearState)
RegisterCreatureEvent(ENTRY_BLOODMAGE_LAURITH, 40, onSetGUID)
RegisterCreatureEvent(ENTRY_BLOODMAGE_LAURITH, 4, clearState)
