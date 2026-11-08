-- The Drak'Tharon Keep instance script — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/instance_drak_tharon_keep.cpp
-- (class instance_drak_tharon_keep : public InstanceMapScript, map 600).
-- RegisterMapEvent binds the map-level Eluna instance hook (the
-- instance_ulduar.lua precedent).
-- PORTED (tick 466) — OnUnitDeath (instance_drak_tharon_keep.cpp:149-153):
-- a dead NPC_CRYSTAL_HANDLER (26627) forwards DoAction(
-- ACTION_CRYSTAL_HANDLER_DIED) to Novos (drak_tharon_keep.h:52 = 15,
-- counted from DATA_TROLLGORE = 0). The Novos object rides the
-- ON_CREATURE_CREATE binding (the instance_ulduar.lua voice/sara/brain
-- precedent); the fire itself rides the Go-only INSTANCE_EVENT_ON_
-- UNIT_DEATH (9, engine/scripting/runtime.go — the C++ ZoneScript::
-- OnUnitDeath has no Eluna hook, fired from Unit::setDeathState,
-- Unit.cpp:8956, ahead of the JustDied hooks). The other instance
-- legs (GetGuidData GUID latching, InitializeNovosSummoner position
-- matching, SetBossState bookkeeping) stay out — no Go instance
-- script model beyond the instanceData table.
-- DOCUMENTED-ONLY: player deaths also fire INSTANCE_EVENT_ON_UNIT_DEATH
-- (9) now — the Go player-death sender (killPlayer /
-- completeSpiritOfRedemptionDeath, tick 467) — but no DTK leg reads
-- player deaths, so the handler below returns early for them (players
-- carry no GetEntry method; the guard must run before the entry read).

local MAP_DRAK_THARON_KEEP = 600

local ENTRY_NOVOS = 26631
local ENTRY_CRYSTAL_HANDLER = 26627

local ACTION_CRYSTAL_HANDLER_DIED = 15

local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

RegisterMapEvent(MAP_DRAK_THARON_KEEP, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == ENTRY_NOVOS then
        instanceData.novos = creature
    end
end)

RegisterMapEvent(MAP_DRAK_THARON_KEEP, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil or unit:GetObjectType() == "Player" or unit:GetEntry() ~= ENTRY_CRYSTAL_HANDLER then
        return
    end
    local novos = instanceData.novos
    if novos ~= nil then
        novos:DoAction(ACTION_CRYSTAL_HANDLER_DIED)
    end
end)
