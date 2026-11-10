-- The Blood Furnace instance script — Lua port of the Broggok
-- prisoner machine in
-- src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- instance_blood_furnace.cpp (class
-- instance_blood_furnace_InstanceMapScript, map 542) +
-- boss_broggok.cpp (go_broggok_lever, BroggokPrisionersAI).
-- RegisterMapEvent binds the map-level Eluna instance hook
-- (the instance_ulduar.lua precedent); the OnUnitDeath
-- handler below is the C++ OnUnitDeath twin
-- (instance_blood_furnace.cpp:92-96), fired by the Go
-- triggerInstanceUnitDeath sender (instance_hooks.go:525,
-- ahead of the creature's own ev4 handler, C++-exact).
-- Shared state lives in the BloodFurnace global (single Lua
-- state, runtime.go — the ColonelJulesSuccess precedent):
-- per-broggok-GUID { cells = {[prisonerGUID] = 5..8},
-- counts = {[5..8] = n} }, built by beginEncounter at
-- lever time and rebuilt by resetPrisons.
-- DELTA (documented, Go-side): C++ OnCreatureCreate stores
-- prisoners into cell sets at spawn (instance_blood_furnace.
-- cpp:68-90); Go fires OnCreatureCreate only for
-- dynamically-materialized motions, never for static
-- template spawns (instance_hooks.go
-- triggerInstanceCreatureCreate), so the cell sets are
-- built by a grid sweep at lever time instead — prisoners
-- are flagged NON_ATTACKABLE + immune at spawn (the ev5
-- StorePrisoner twin in the prisoner files), so they are
-- still in their cells when the sweep runs.
-- C++ SetBossState(DATA_BROGGOK, IN_PROGRESS) ->
-- ActivateCell(DATA_PRISON_CELL5) (:156-161) is invoked
-- explicitly by the lever handler (boss_broggok.lua):
-- SetBossState has no Lua instance-override hook.
-- HandleGameObject(door, true/false) rides
-- FindNearestGameObject + SetGoState (ACTIVE 0 / READY 1);
-- the door entries are unique per cell, so the nearest
-- match is the door.

local MAP_BLOOD_FURNACE = 542
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local NPC_BROGGOK = 17380 -- blood_furnace.h:51
local NPC_PRISONER1 = 17398 -- blood_furnace.h:54
local NPC_PRISONER2 = 17429 -- blood_furnace.h:55

local ACTION_ACTIVATE_BROGGOK = 1 -- blood_furnace.h:84

local GO_PRISON_DOOR_04 = 181819 -- blood_furnace.h:74
local GO_PRISON_CELL_DOOR_5 = 181821 -- blood_furnace.h:82
local GO_PRISON_CELL_DOOR_6 = 181818 -- blood_furnace.h:83
local GO_PRISON_CELL_DOOR_7 = 181820 -- blood_furnace.h:84
local GO_PRISON_CELL_DOOR_8 = 181817 -- blood_furnace.h:85

local CELL_DOORS = {
    [5] = GO_PRISON_CELL_DOOR_5,
    [6] = GO_PRISON_CELL_DOOR_6,
    [7] = GO_PRISON_CELL_DOOR_7,
    [8] = GO_PRISON_CELL_DOOR_8,
}

local GO_STATE_ACTIVE = 0 -- GameObjectMethods.h:90
local GO_STATE_READY = 1 -- GameObjectMethods.h:91

local UNIT_FIELD_FLAGS = 59 -- arlokk precedent
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- bosses_opera.lua:103

BloodFurnace = BloodFurnace or {}
BloodFurnace._state = BloodFurnace._state or {}

-- C++ StorePrisoner position boxes
-- (instance_blood_furnace.cpp:229-256): x 405-423 + z <= 17
-- splits y 106-123 -> cell 5, y 76-91 -> cell 6; x 490-506
-- + z <= 17 splits y 106-123 -> cell 7, y 76-91 -> cell 8;
-- anything else is not stored (nil).
local function classifyCell(x, y, z)
    if z > 17 then
        return nil
    end
    if x >= 405.0 and x <= 423.0 then
        if y >= 106.0 and y <= 123.0 then
            return 5
        end
        if y >= 76.0 and y <= 91.0 then
            return 6
        end
    elseif x >= 490.0 and x <= 506.0 then
        if y >= 106.0 and y <= 123.0 then
            return 7
        end
        if y >= 76.0 and y <= 91.0 then
            return 8
        end
    end
    return nil
end

-- The StorePrisoner position twin, for the prisoner files'
-- ev5 flag leg (only in-box prisoners are stored/flagged).
function BloodFurnace.cellAt(x, y, z)
    return classifyCell(x, y, z)
end

local function sweepPrisoners(broggok)
    local found = {}
    for _, entry in ipairs({ NPC_PRISONER1, NPC_PRISONER2 }) do
        local list = broggok:GetCreaturesInGrid(entry, 500)
        if list then
            for _, p in ipairs(list) do
                found[#found + 1] = p
            end
        end
    end
    return found
end

-- C++ StorePrisoner + counter init (instance_blood_furnace.
-- cpp:68-90/:185-198), run at lever time: sweep both
-- prisoner entries within 500 yd of Broggok (the
-- razorscale 500-yd arena precedent) and record each
-- in-box prisoner's cell + the per-cell counts.
function BloodFurnace.beginEncounter(broggok)
    local guid = broggok:GetGUIDLow()
    local cells = {}
    local counts = { [5] = 0, [6] = 0, [7] = 0, [8] = 0 }
    for _, p in ipairs(sweepPrisoners(broggok)) do
        local cell = classifyCell(p:GetX(), p:GetY(), p:GetZ())
        if cell then
            cells[p:GetGUIDLow()] = cell
            counts[cell] = counts[cell] + 1
        end
    end
    BloodFurnace._state[guid] = { cells = cells, counts = counts }
end

-- C++ ActivateCell (instance_blood_furnace.cpp:276-313):
-- open the cell door, then ActivatePrisoners — RemoveFlag
-- NON_ATTACKABLE + SetImmuneToAll(false) + DoZoneInCombat
-- per prisoner in the cell (:313-323). Cell 0 is
-- DATA_DOOR_4: open the rear door and send Broggok
-- DoAction(ACTION_ACTIVATE_BROGGOK) (:306-312).
function BloodFurnace.activateCell(broggok, cell)
    if cell == 0 then
        local door = broggok:FindNearestGameObject(GO_PRISON_DOOR_04, 500)
        if door then
            door:SetGoState(GO_STATE_ACTIVE)
        end
        broggok:DoAction(ACTION_ACTIVATE_BROGGOK)
        return
    end
    local door = broggok:FindNearestGameObject(CELL_DOORS[cell], 500)
    if door then
        door:SetGoState(GO_STATE_ACTIVE)
    end
    for _, p in ipairs(sweepPrisoners(broggok)) do
        if classifyCell(p:GetX(), p:GetY(), p:GetZ()) == cell then
            p:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            p:SetImmuneToAll(false)
            p:DoZoneInCombat()
        end
    end
end

-- C++ PrisonerDied (instance_blood_furnace.cpp:266-274):
-- decrement the dead prisoner's cell counter; at zero,
-- activate the next cell (5 -> 6 -> 7 -> 8 -> DOOR_4).
-- Deaths of prisoners with no recorded cell (never swept,
-- or outside every box) are ignored, as C++ ignores
-- GUIDs absent from the cell sets.
function BloodFurnace.prisonerDied(unit)
    local broggok = unit:FindNearestCreature(NPC_BROGGOK, 500)
    if not broggok then
        return
    end
    local st = BloodFurnace._state[broggok:GetGUIDLow()]
    if not st then
        return
    end
    local cell = st.cells[unit:GetGUIDLow()]
    if not cell then
        return
    end
    st.counts[cell] = st.counts[cell] - 1
    if st.counts[cell] == 0 then
        if cell == 5 then
            BloodFurnace.activateCell(broggok, 6)
        elseif cell == 6 then
            BloodFurnace.activateCell(broggok, 7)
        elseif cell == 7 then
            BloodFurnace.activateCell(broggok, 8)
        elseif cell == 8 then
            BloodFurnace.activateCell(broggok, 0)
        end
    end
end

-- C++ ResetPrisons (instance_blood_furnace.cpp:175-199):
-- per prisoner: respawn the dead, re-flag NON_ATTACKABLE,
-- SetImmuneToAll(true), EnterEvadeMode (ResetPrisoner
-- :202-211); close the four cell doors; rebuild the
-- counters from the live sweep (C++ re-counts from the
-- stored GUID sets).
function BloodFurnace.resetPrisons(broggok)
    for _, p in ipairs(sweepPrisoners(broggok)) do
        if not p:IsAlive() then
            p:Respawn()
        end
        p:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        p:SetImmuneToAll(true)
        p:EnterEvadeMode()
    end
    for cell = 5, 8 do
        local door = broggok:FindNearestGameObject(CELL_DOORS[cell], 500)
        if door then
            door:SetGoState(GO_STATE_READY)
        end
    end
    BloodFurnace.beginEncounter(broggok)
end

-- C++ OnUnitDeath (instance_blood_furnace.cpp:92-96):
-- only the two prisoner entries feed PrisonerDied.
RegisterMapEvent(MAP_BLOOD_FURNACE, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil or unit:GetObjectType() ~= "Creature" then
        return
    end
    local entry = unit:GetEntry()
    if entry == NPC_PRISONER1 or entry == NPC_PRISONER2 then
        BloodFurnace.prisonerDied(unit)
    end
end)
