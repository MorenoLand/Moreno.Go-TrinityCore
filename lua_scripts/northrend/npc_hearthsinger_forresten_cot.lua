-- Hearth singer Forresten (Culling of Stratholme) — Lua port of the
-- at_stratholme_inn_stairs_cot area trigger plus the SetGUID leg it
-- feeds, in
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/culling_of_stratholme.cpp
-- (class at_stratholme_inn_stairs_cot :200-236;
-- npc_hearthsinger_forresten_cotAI::SetGUID :161-176).
-- Trigger 5291 (TDB 335.26091 areatrigger_scripts row for
-- at_stratholme_inn_stairs_cot). Fire site:
-- engine/world/areatrigger.go:64 (TriggerServerEvent,
-- ServerEventAreaTrigger = 24); handlers registered via
-- RegisterServerEvent(24, fn) receive (event, player, triggerID)
-- (the sartura / at_scripts.lua precedent). The handler return is
-- discarded, so the C++ `return true` is moot here.
-- PORTED 2026-11-15 (at_* census): the old stay-out rested on "no
-- instance GetData read bridge / no SetGUID dispatch" — both are
-- stale: the progress gate reads the instance file's
-- instanceData.progress store (instance_culling_of_stratholme.lua,
-- nil = JUST_STARTED) and creature:SetGUID fires ev40
-- (lua_creature_events.go:889-916; the simon-bunny precedent
-- passes player:GetGUID() and resolves it with the global
-- GetPlayerByGUID). The rest of Forresten's machine — the
-- MoveInLineOfSight RP chain (:182-200) and the Belfast
-- SetData talk chain (:178-180) — stays out with it; only the
-- Belfast-takeover leg the trigger feeds is ported here.

local TRIGGER_STRATHOLME_INN_STAIRS = 5291 -- TDB areatrigger_scripts row

local NPC_FORREST = 30551 -- culling_of_stratholme.cpp:41
local NPC_BELFAST = 30571 -- culling_of_stratholme.cpp:42
local LINE_BELFAST_0 = 0 -- culling_of_stratholme.cpp:80

local JUST_STARTED = 0x00001 -- culling_of_stratholme.h:91
local CRATES_IN_PROGRESS = 0x00002 -- culling_of_stratholme.h:92

-- C++ :219-236 at_stratholme_inn_stairs_cot::OnTrigger.
RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_STRATHOLME_INN_STAIRS then
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
    if (data.progress or JUST_STARTED) > CRATES_IN_PROGRESS then
        return
    end
    local forrest = player:FindNearestCreature(NPC_FORREST, 200)
    if forrest then
        forrest:SetGUID(player:GetGUID(), 0)
    end
end)

-- C++ :161-176 npc_hearthsinger_forresten_cotAI::SetGUID. Per-
-- Forresten latch (the _hadBelfast member): the first SetGUID
-- takes Belfast over for a moment — StopMoving, face the invoker,
-- Talk(LINE_BELFAST_0). The facing rides the SetFacingTo bridge
-- with the atan2 angle (lua_creature_events.go:5609-5616, the
-- alexstrasza precedent); SetFacingToObject has no direct bridge.
local belfastTaken = {}

local function forrestSetGUID(event, creature, playerGuid, id)
    local guid = creature:GetGUIDLow()
    if belfastTaken[guid] then
        return
    end
    belfastTaken[guid] = true
    local belfast = creature:FindNearestCreature(NPC_BELFAST, 100)
    if not belfast then
        return
    end
    local invoker = GetPlayerByGUID(playerGuid)
    if not invoker then
        return
    end
    belfast:StopMoving()
    local angle = math.atan2(invoker:GetY() - belfast:GetY(), invoker:GetX() - belfast:GetX())
    if angle < 0 then
        angle = angle + 2 * math.pi
    end
    belfast:SetFacingTo(angle)
    belfast:Talk(LINE_BELFAST_0)
end

RegisterCreatureEvent(NPC_FORREST, 40, forrestSetGUID)
