-- Shadowfang Keep: Shadowfang Prisoner — Lua port of the
-- WaypointReached arms of
-- src/server/scripts/EasternKingdoms/ShadowfangKeep/shadowfang_keep.cpp
-- (class npc_shadowfang_prisoner :
-- CreatureScript("npc_shadowfang_prisoner");
-- struct npc_shadowfang_prisonerAI : public EscortAI).
-- Entries: 3849 Deathstalker Adamant (NPC_ADA) / 3850 Sorcerer
-- Ashcrombe (NPC_ASH, shadowfang_keep.h:39-40); TDB 335.26091
-- creature_template ScriptName npc_shadowfang_prisoner on both;
-- creature_text groups 0-4 on both.
-- Eluna creature events: 6 OnReachWP.
-- Ported arms (C++ order, shadowfang_keep.cpp:73-115), all gated on
-- GetPlayerForEscort() (Lua twin: GetEscortPlayer() nil-guard):
-- - wp0 Talk(SAY_FREE 0, player) — same id for both entries
--   (SAY_FREE_AS 0 / SAY_FREE_AD 0).
-- - wp10 Talk(SAY_OPEN_DOOR 1, player) — both entries.
-- - wp11 Ash-only DoCast(me, SPELL_UNLOCK 6421).
-- - wp12 Talk(SAY_POST_DOOR_AS 2 / SAY_POST1_DOOR_AD 2, player) +
--   instance->SetData(TYPE_FREE_NPC 1, DONE 3)
--   (shadowfang_keep.h:28; DONE 3, InstanceScript.h:73) via the
--   InstanceData:SetData bridge (instance_hooks.go:51),
--   nil-guarded through instanceDataOf (old_hillsbrad precedent).
--   NO instance_shadowfang_keep.lua exists, so the
--   Map:GetInstanceData gate is closed and the write is skipped
--   live — it is the faithful writer for a future instance port.
-- - wp13 Ada-only Talk(SAY_POST2_DOOR_AD 3, player).
-- Unported scope: the gossip driver (OnGossipHello gate on
-- TYPE_FREE_NPC != DONE + TYPE_RETHILGORE == DONE, OnGossipSelect
-- Start) — the escort driver, so this handler fires once that lands.

local ENTRY_ADA = 3849 -- NPC_ADA, shadowfang_keep.h:40
local ENTRY_ASH = 3850 -- NPC_ASH, shadowfang_keep.h:39

local SAY_FREE = 0
local SAY_OPEN_DOOR = 1
local SAY_POST_DOOR = 2
local SAY_POST2_DOOR_AD = 3

local SPELL_UNLOCK = 6421

local TYPE_FREE_NPC = 1 -- shadowfang_keep.h:28
local DONE = 3 -- InstanceScript.h:73

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ WaypointReached (shadowfang_keep.cpp:73-115).
local function prisonerReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local isAsh = creature:GetEntry() == ENTRY_ASH
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:Talk(SAY_FREE, player)
    elseif wp == 10 then
        creature:Talk(SAY_OPEN_DOOR, player)
    elseif wp == 11 then
        if isAsh then
            creature:CastSpell(creature, SPELL_UNLOCK)
        end
    elseif wp == 12 then
        creature:Talk(SAY_POST_DOOR, player)
        local data = instanceDataOf(creature)
        if data then
            data:SetData(TYPE_FREE_NPC, DONE)
        end
    elseif wp == 13 then
        if not isAsh then
            creature:Talk(SAY_POST2_DOOR_AD, player)
        end
    end
end

RegisterCreatureEvent(ENTRY_ADA, 6, prisonerReachWP)
RegisterCreatureEvent(ENTRY_ASH, 6, prisonerReachWP)
