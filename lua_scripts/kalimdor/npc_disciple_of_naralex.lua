-- Wailing Caverns: Disciple of Naralex — Lua port of the
-- WaypointReached arms of
-- src/server/scripts/Kalimdor/WailingCaverns/wailing_caverns.cpp
-- (class npc_disciple_of_naralex :
-- CreatureScript("npc_disciple_of_naralex");
-- struct npc_disciple_of_naralexAI : public EscortAI).
-- Entry: 3678 (TDB 335.26091 creature_template ScriptName
-- npc_disciple_of_naralex; creature_text groups 0-10).
-- Eluna creature events: 6 OnReachWP.
-- Ported arms (C++ order, wailing_caverns.cpp:102-131):
-- - wp4 instance->SetData(TYPE_NARALEX_PART1 6, IN_PROGRESS 1).
-- - wp5 Talk(SAY_MUST_CONTINUE 3) + SetData(PART1, DONE 3).
-- - wp11 SetData(TYPE_NARALEX_PART2 7, IN_PROGRESS 1).
-- - wp19 Talk(SAY_BEYOND_THIS_CORRIDOR 6).
-- - wp24 SetData(TYPE_NARALEX_PART3 8, IN_PROGRESS 1)
--   (wailing_caverns.h:33-35; IN_PROGRESS 1 / DONE 3,
--   InstanceScript.h:71/73).
-- SetData writes ride the InstanceData:SetData bridge
-- (instance_hooks.go:51), nil-guarded through instanceDataOf
-- (old_hillsbrad precedent). NO instance_wailing_caverns.lua
-- exists, so the Map:GetInstanceData gate is closed and the writes
-- are skipped live — they are the faithful writers for a future
-- instance port.
-- Deliberate omissions (UpdateAI-machine state, not ev6 arms): the
-- eventProgress/currentEvent assignments on wp4/11/24 drive the
-- unported UpdateAI event machine (Part 1/2/3 summon + Talk +
-- kneel + awakening legs, wailing_caverns.cpp:160-260) — stays out.
-- Unported scope: the gossip driver (OnGossipSelect Start) — the
-- escort driver, so this handler fires once that lands.

local ENTRY_DISCIPLE = 3678

local SAY_MUST_CONTINUE = 3
local SAY_BEYOND_THIS_CORRIDOR = 6

local TYPE_NARALEX_PART1 = 6 -- wailing_caverns.h:33
local TYPE_NARALEX_PART2 = 7 -- wailing_caverns.h:34
local TYPE_NARALEX_PART3 = 8 -- wailing_caverns.h:35
local IN_PROGRESS = 1 -- InstanceScript.h:71
local DONE = 3 -- InstanceScript.h:73

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ WaypointReached (wailing_caverns.cpp:102-131).
local function naralexReachWP(event, creature)
    local wp = creature:GetCurrentWaypointId()
    local data = nil
    if wp == 4 or wp == 5 or wp == 11 or wp == 24 then
        data = instanceDataOf(creature)
    end
    if wp == 4 then
        if data then
            data:SetData(TYPE_NARALEX_PART1, IN_PROGRESS)
        end
    elseif wp == 5 then
        creature:Talk(SAY_MUST_CONTINUE)
        if data then
            data:SetData(TYPE_NARALEX_PART1, DONE)
        end
    elseif wp == 11 then
        if data then
            data:SetData(TYPE_NARALEX_PART2, IN_PROGRESS)
        end
    elseif wp == 19 then
        creature:Talk(SAY_BEYOND_THIS_CORRIDOR)
    elseif wp == 24 then
        if data then
            data:SetData(TYPE_NARALEX_PART3, IN_PROGRESS)
        end
    end
end

RegisterCreatureEvent(ENTRY_DISCIPLE, 6, naralexReachWP)
