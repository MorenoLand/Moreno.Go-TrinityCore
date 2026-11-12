-- Pit of Saron: at_tyrannus_event_starter — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_scourgelord_tyrannus.cpp
-- (at_tyrannus_event_starter, :541-563). Trigger ID 5633 is the TDB
-- areatrigger_scripts row (25101 + 26091 agree); no in-file ID exists
-- in C++ — the ID is read from the DB dumps, not guessed (the
-- at_zumrah 962 precedent, d8b1a516; the 12:21 TDB-ID ruling
-- overturns the d9946689 gelihast stay-out for this sender).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers receive (event, player,
-- triggerId).
-- C++ :544-562: a GM player or a missing instance returns false; if
-- the Tyrannus boss state is neither IN_PROGRESS nor DONE, the bound
-- Tyrannus creature runs DoAction(ACTION_START_INTRO) and the
-- trigger returns true. The DoAction receiver + intro machine are
-- ported in boss_scourgelord_tyrannus.lua (ev38, tick 490) — this
-- trigger was its only missing sender.
-- Deviations: the boss-state gate reads the bound Tyrannus creature
-- itself (instance_pit_of_saron.lua binds it as instanceData.tyrannus,
-- the GetGuidData mirror); an unbound Tyrannus is the C++ null
-- GetGuidData return, a no-op. Go reads TO_BE_DECIDED (5) for an
-- unset boss where C++ reads NOT_STARTED (0) — both are neither
-- IN_PROGRESS nor DONE, so the gate passes identically.

local TRIGGER_TYRANNUS_EVENT_STARTER = 5633 -- TDB areatrigger_scripts

local DATA_TYRANNUS = 2 -- pit_of_saron.h:34
local ACTION_START_INTRO = 1 -- boss_scourgelord_tyrannus.cpp:107

local BOSS_STATE_IN_PROGRESS = 1 -- InstanceScript.h:68-76
local BOSS_STATE_DONE = 3

-- C++ :544-562 at_tyrannus_event_starter::OnTrigger.
local function onTyrannusEventStarter(player)
    if player:IsGM() then
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local tyrannus = data.tyrannus
    if not tyrannus then
        return
    end
    local state = tyrannus:GetBossState(DATA_TYRANNUS)
    if state == BOSS_STATE_IN_PROGRESS or state == BOSS_STATE_DONE then
        return
    end
    tyrannus:DoAction(ACTION_START_INTRO)
end

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId == TRIGGER_TYRANNUS_EVENT_STARTER then
        onTyrannusEventStarter(player)
    end
end)
