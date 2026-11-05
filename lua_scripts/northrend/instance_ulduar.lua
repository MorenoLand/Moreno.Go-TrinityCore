-- The Ulduar instance script — Lua port of the Yogg-Saron legs of
-- src/server/scripts/Northrend/Ulduar/Ulduar/instance_ulduar.cpp
-- (class instance_ulduar : public InstanceMapScript, map 603).
-- RegisterMapEvent binds the map-level Eluna instance hook (the
-- instance_naxxramas.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 603
-- (engine/world/instance_hooks.go) that the Yogg-Saron
-- EVENT_ILLUSION chain shares through this file's table.
-- Shared instanceData keys (one table per (map, instance)):
-- voice / sara / brain — the creature objects bound by
-- ON_CREATURE_CREATE, the C++ LoadObjectData latching
-- (instance_ulduar.cpp:109-111: NPC_VOICE_OF_YOGG_SARON ->
-- DATA_VOICE_OF_YOGG_SARON, NPC_SARA -> DATA_SARA,
-- NPC_BRAIN_OF_YOGG_SARON -> DATA_BRAIN_OF_YOGG_SARON);
-- illusion — the DATA_ILLUSION value (0 CHAMBER / 1 ICECROWN /
-- 2 STORMWIND, ulduar.h:508-510), written by the voice's
-- EVENT_ILLUSION arm and read by the brain's
-- ACTION_INDUCE_MADNESS / ACTION_TENTACLE_KILLED arms
-- (boss_yogg_saron.cpp:599-600/:1104/:1115), initialized to 0,
-- the C++ member default (instance_ulduar.cpp GetData returns
-- the `illusion` member, 0 before the first illusion).
-- The mimiron keeper object is NOT bound here: YS keepers are
-- temp summons, which bypass the ON_CREATURE_CREATE funnel
-- (instance_hooks.go) — the gossip summon latches it instead
-- (boss_yogg_saron.lua, the C++ KeeperGUIDs[3] leg,
-- instance_ulduar.cpp:374-377). The GO brain-room doors
-- (194635-194637) have no latch: HandleGameObject has no bridge.
local MAP_ULDUAR = 603
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local ENTRY_VOICE_OF_YOGG_SARON = 33280 -- ulduar.h:203
local ENTRY_SARA = 33134 -- ulduar.h:197
local ENTRY_BRAIN_OF_YOGG_SARON = 33890 -- ulduar.h:218

local yoggBinds = {
    [ENTRY_VOICE_OF_YOGG_SARON] = "voice",
    [ENTRY_SARA] = "sara",
    [ENTRY_BRAIN_OF_YOGG_SARON] = "brain",
}

RegisterMapEvent(MAP_ULDUAR, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = yoggBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
    if instanceData.illusion == nil then
        instanceData.illusion = 0
    end
end)
