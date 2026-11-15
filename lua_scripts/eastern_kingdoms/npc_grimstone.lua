-- Blackrock Depths: High Justice Grimstone — Lua port of the
-- WaypointReached arms of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/blackrock_depths.cpp
-- (class npc_grimstone : CreatureScript("npc_grimstone");
-- struct npc_grimstoneAI : public EscortAI).
-- Entry: 10096 (TDB 335.26091 creature_template ScriptName
-- npc_grimstone; creature_text groups 0-5).
-- Eluna creature events: 6 OnReachWP.
-- Ported arms (C++ order, blackrock_depths.cpp:205-236):
-- - WaypointReached wp0 Talk(SAY_TEXT1 0), wp1 Talk(SAY_TEXT2 1),
--   wp3 Talk(SAY_TEXT3 2), wp4 Talk(SAY_TEXT4 3)
--   (GrimstoneTexts :117-122).
-- - WaypointReached wp5 instance->SetData(TYPE_RING_OF_LAW 1, DONE 3)
--   (blackrock_depths.h:28; DONE 3, InstanceScript.h:68-76) via the
--   InstanceData:SetData bridge (instance_hooks.go:51, fires event 8),
--   nil-guarded through instanceDataOf (old_hillsbrad precedent).
--   The store landed 2026-11-15 (at_* census):
--   instance_blackrock_depths.lua keeps instanceData.ringOfLaw
--   (SetData TYPE_RING_OF_LAW 1), read back by the at_ring_of_law
--   gate; the DONE write saves. The wp5
--   UpdateEncounterStateForKilledCreature leg has no bridge.
-- Deliberate omissions (documented, no bridge loss):
-- - wp2 has no observable ev6 arm (CanWalk=false only — Ring-of-Law
--   UpdateAI-machine state; thrall-wp98 precedent).
-- - The CanWalk/Event_Timer assignments on wp0/1/4 are the same
--   UpdateAI machine's state, not ported.
-- Unported scope: the Ring-of-Law UpdateAI event machine
-- (blackrock_depths.cpp:275-360: EventPhase 0-8, ring-mob/boss
-- summons, HandleGameObject arena-door legs, SAY_TEXT5/6) — it is
-- also the escort driver (phase 0 calls Start), so this handler
-- fires once that machine lands; the gossip/area-trigger start
-- (at_ring_of_law) is with it.

local ENTRY_GRIMSTONE = 10096

local SAY_TEXT1 = 0
local SAY_TEXT2 = 1
local SAY_TEXT3 = 2
local SAY_TEXT4 = 3

local TYPE_RING_OF_LAW = 1 -- blackrock_depths.h:28
local DONE = 3 -- InstanceScript.h:73

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ WaypointReached (blackrock_depths.cpp:205-236).
local function grimstoneReachWP(event, creature)
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:Talk(SAY_TEXT1)
    elseif wp == 1 then
        creature:Talk(SAY_TEXT2)
    elseif wp == 3 then
        creature:Talk(SAY_TEXT3)
    elseif wp == 4 then
        creature:Talk(SAY_TEXT4)
    elseif wp == 5 then
        local data = instanceDataOf(creature)
        if data then
            data:SetData(TYPE_RING_OF_LAW, DONE)
        end
    end
end

RegisterCreatureEvent(ENTRY_GRIMSTONE, 6, grimstoneReachWP)
