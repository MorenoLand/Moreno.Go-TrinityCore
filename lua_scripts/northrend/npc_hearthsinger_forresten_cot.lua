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
-- GetPlayerByGUID).
-- PORTED 2026-11-22: the MoveInLineOfSight RP chain (:182-200) and
-- the Belfast SetData talk chain (:178-180) below. The 2026-11-07
-- stay-out rested on "none of its three triggers is bridged" — all
-- three are live now: ev27 fires from the proximity arm
-- (stepCreatureMoveInLOSProximity, creaturemotion.go — friendly /
-- non-engaging creatures included, every 100ms tick in 50yd sight
-- range; the handler re-checks the C++ 10yd 2D gate and latches
-- _hadTalk), ev41 fires from SmartAI SET_DATA on non-SmartAI
-- targets (smartActionSetData, smart_ai.go — the C++
-- ai->SetData(field, data) twin, SmartScript.cpp:1173-1195), and
-- ev40 SetGUID was already ported above. Timers ride CreateLuaEvent
-- one-shots (the martha/chromie idiom in
-- culling_of_stratholme.lua); C++ EventMap.ScheduleEvent overwrites
-- a pending same-id event, so schedule() cancels a pending
-- same-key timer first. Cancel only on died (4) / remove (37):
-- C++ has no Reset override, so an evade must not cancel the pump
-- (simon-bunny idiom).

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

local NPC_JAMES = 30553 -- culling_of_stratholme.cpp:43
local NPC_FRAS = 30552 -- culling_of_stratholme.cpp:44
local NPC_MAL = 31017 -- culling_of_stratholme.cpp:45
local NPC_GRYAN = 30561 -- culling_of_stratholme.cpp:46

local EVENT_FORREST_1 = 1
local EVENT_JAMES_1 = 2
local EVENT_FRAS_1 = 3
local EVENT_BELFAST_MOVE = 100
local EVENT_BELFAST_1 = 101
local EVENT_MAL_1 = 102
local EVENT_GRYAN_1 = 103
local EVENT_MAL_2 = 104
local EVENT_MAL_3 = 105
local EVENT_JAMES_2 = 106
local EVENT_FORREST_2 = 107
local EVENT_FRAS_2 = 108

local LINE_FORREST_1 = 0
local LINE_FORREST_2 = 1
local LINE_JAMES_1 = 0
local LINE_JAMES_2 = 1
local LINE_FRAS_1 = 0
local LINE_FRAS_2 = 1
local LINE_MAL_1 = 0
local LINE_MAL_2 = 1
local LINE_MAL_3 = 2
local LINE_BELFAST_1 = 1
local LINE_GRYAN_1 = 0

local forrestTimers = {}
local forrestHadTalk = {}
local forrestTriggeringPlayer = {}

local function forrestCancelTimers(guid)
    local per = forrestTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        forrestTimers[guid] = nil
    end
    forrestHadTalk[guid] = nil
    forrestTriggeringPlayer[guid] = nil
end

local function forrestSchedule(guid, key, delay, fn)
    local per = forrestTimers[guid]
    if not per then
        per = {}
        forrestTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ UpdateAI talk arm (:140-157): talkerEntry 0 is Forresten
-- himself; otherwise the nearest creature of that entry in 80yd.
-- The line goes to the MoveInLineOfSight triggering player when
-- still resolvable, else to everyone (C++ Talk(line, nullptr)).
local function forrestTalk(creature, guid, talkerEntry, line)
    local talker = creature
    if talkerEntry ~= 0 then
        talker = creature:FindNearestCreature(talkerEntry, 80)
        if not talker then
            return
        end
    end
    local pguid = forrestTriggeringPlayer[guid]
    if pguid then
        local target = GetPlayerByGUID(pguid)
        if target then
            talker:Talk(line, target)
            return
        end
    end
    talker:Talk(line)
end

local function forrestTalkEvent(creature, guid, talkerEntry, line)
    return function()
        forrestTalk(creature, guid, talkerEntry, line)
    end
end

-- C++ UpdateAI EVENT_BELFAST_MOVE (:117-120): Belfast's AI takes
-- DoAction(100) — Belfast is SmartAI, so the DoAction bridge fires
-- its ACTION_DONE (72) row (SET_EMOTE_STATE + LINK SetRun +
-- MoveToPos 42), and its MOVEMENTINFORM install SET_DATAs
-- Forresten, which schedules the talk chain below.
local function forrestBelfastMove(creature)
    local belfast = creature:FindNearestCreature(NPC_BELFAST, 80)
    if belfast then
        belfast:DoAction(EVENT_BELFAST_MOVE)
    end
end

-- C++ MoveInLineOfSight (:182-200): once-gated; player within 10yd
-- 2D and instance progress <= CRATES_IN_PROGRESS. The ev27
-- proximity arm repeat-fires while a player stays in 50yd sight
-- range, so the 10yd gate + _hadTalk latch both live here.
local function forrestMoveInLOS(event, creature, who)
    if who:GetObjectType() ~= "Player" then
        return
    end
    local guid = creature:GetGUIDLow()
    if forrestHadTalk[guid] then
        return
    end
    local mapObj = who:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data and (data.progress or JUST_STARTED) > CRATES_IN_PROGRESS then
            return
        end
    end
    local dx = creature:GetX() - who:GetX()
    local dy = creature:GetY() - who:GetY()
    if dx * dx + dy * dy > 100.0 then
        return
    end
    forrestHadTalk[guid] = true
    forrestTriggeringPlayer[guid] = who:GetGUID()
    local offset = math.random(10, 30) * 1000
    forrestSchedule(guid, EVENT_FORREST_1, offset, forrestTalkEvent(creature, guid, 0, LINE_FORREST_1))
    forrestSchedule(guid, EVENT_JAMES_1, offset + 6000, forrestTalkEvent(creature, guid, NPC_JAMES, LINE_JAMES_1))
    forrestSchedule(guid, EVENT_FRAS_1, offset + 12000, forrestTalkEvent(creature, guid, NPC_FRAS, LINE_FRAS_1))
    forrestSchedule(guid, EVENT_BELFAST_MOVE, offset + 12000, function()
        forrestBelfastMove(creature)
    end)
end

-- C++ SetData (:178-180): schedules the post-Belfast talk chain
-- unconditionally (type/value ignored). C++ Seconds(0) -> 1ms here:
-- CreateLuaEvent needs delay >= 1 (malcolm precedent).
local function forrestSetData(event, creature, typ, data)
    local guid = creature:GetGUIDLow()
    forrestSchedule(guid, EVENT_BELFAST_1, 1, forrestTalkEvent(creature, guid, NPC_BELFAST, LINE_BELFAST_1))
    forrestSchedule(guid, EVENT_MAL_1, 6000, forrestTalkEvent(creature, guid, NPC_MAL, LINE_MAL_1))
    forrestSchedule(guid, EVENT_GRYAN_1, 12000, forrestTalkEvent(creature, guid, NPC_GRYAN, LINE_GRYAN_1))
    forrestSchedule(guid, EVENT_MAL_2, 18000, forrestTalkEvent(creature, guid, NPC_MAL, LINE_MAL_2))
    forrestSchedule(guid, EVENT_MAL_3, 20000, forrestTalkEvent(creature, guid, NPC_MAL, LINE_MAL_3))
    forrestSchedule(guid, EVENT_JAMES_2, 26000, forrestTalkEvent(creature, guid, NPC_JAMES, LINE_JAMES_2))
    forrestSchedule(guid, EVENT_FORREST_2, 32000, forrestTalkEvent(creature, guid, 0, LINE_FORREST_2))
    forrestSchedule(guid, EVENT_FRAS_2, 38000, forrestTalkEvent(creature, guid, NPC_FRAS, LINE_FRAS_2))
end

RegisterCreatureEvent(NPC_FORREST, 27, forrestMoveInLOS)
RegisterCreatureEvent(NPC_FORREST, 41, forrestSetData)
RegisterCreatureEvent(NPC_FORREST, 4, function(event, creature)
    forrestCancelTimers(creature:GetGUIDLow())
end)
RegisterCreatureEvent(NPC_FORREST, 37, function(event, creature)
    forrestCancelTimers(creature:GetGUIDLow())
end)
