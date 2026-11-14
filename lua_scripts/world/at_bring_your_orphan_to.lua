-- Childrens Week area triggers — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class at_bring_your_orphan_to : AreaTriggerScript
-- ("at_bring_your_orphan_to"), childrens_week.cpp:934-987).
-- Fire site: engine/world/areatrigger.go:64 (TriggerServerEvent ctx,
-- scripting.ServerEventAreaTrigger (=24), luaPlayer, triggerID);
-- handlers registered via RegisterServerEvent(24, fn) receive
-- (event, player, triggerId) (at_hor_triggers.lua precedent).
-- Ported arm:
-- - OnTrigger (childrens_week.cpp:942-986): dead players and players
--   without SPELL_ORPHAN_OUT (58818) get nothing; otherwise the
--   trigger ID selects a quest/orphan pair and a player with the
--   matching orphan out and the quest INCOMPLETE gets
--   AreaExploredOrEventHappens. Per-trigger pairs (C++-exact):
--   3551 Down at the Docks -> quest 910 + orcish orphan 14444;
--   3549 Gateway to the Frontier -> quest 911 + orcish; 3547
--   Lordaeron Throne Room -> quest 1800 + orcish; 3546 Bought of
--   Eternals -> quest 1479 + human orphan 14305; 3552 Spooky
--   Lighthouse -> quest 1687 + human; 3548 Stonewrought Dam ->
--   quest 1558 + human; 4356 Dark Portal -> team-gated (Alliance
--   quest 10952 + draenei orphan 22818 / Horde quest 10951 +
--   blood-elf orphan 22817).
-- No native credit arm exists for these triggers:
-- areatrigger_involvedrelation holds NO rows for any of the 7 IDs
-- (verified vs TDB 26091), so the Go native arm
-- (areatrigger.go:3) can never credit them and this handler is the
-- only credit path. The C++ `return true` has no Lua twin — Go
-- discards the handler return (areatrigger.go:64, "never vetoes");
-- the ported payload is only the quest-credit leg, which is
-- unaffected.

local SPELL_ORPHAN_OUT = 58818

local ORPHAN_HUMAN = 14305
local ORPHAN_ORCISH = 14444
local ORPHAN_BLOOD_ELF = 22817
local ORPHAN_DRAENEI = 22818

local QUEST_DOWN_AT_THE_DOCKS = 910
local QUEST_GATEWAY_TO_THE_FRONTIER = 911
local QUEST_BOUGHT_OF_ETERNALS = 1479
local QUEST_SPOOKY_LIGHTHOUSE = 1687
local QUEST_STONEWROUGHT_DAM = 1558
local QUEST_DARK_PORTAL_H = 10951
local QUEST_DARK_PORTAL_A = 10952
local QUEST_LORDAERON_THRONE_ROOM = 1800

local AT_DOWN_AT_THE_DOCKS = 3551
local AT_GATEWAY_TO_THE_FRONTIER = 3549
local AT_LORDAERON_THRONE_ROOM = 3547
local AT_BOUGHT_OF_ETERNALS = 3546
local AT_SPOOKY_LIGHTHOUSE = 3552
local AT_STONEWROUGHT_DAM = 3548
local AT_DARK_PORTAL = 4356

local TEAM_ALLIANCE = 0

local QUEST_STATUS_INCOMPLETE = 3

-- C++ getOrphanGUID (childrens_week.cpp:136-144): the SPELL_ORPHAN_OUT
-- aura on the player, when its caster is the expected orphan entry,
-- yields the orphan's GUID; otherwise empty.
local function getOrphanGUID(player, orphanEntry)
    local aura = player:GetAura(SPELL_ORPHAN_OUT)
    if aura == nil then
        return nil
    end
    local caster = aura:GetCaster()
    if caster == nil or caster:GetEntry() ~= orphanEntry then
        return nil
    end
    return caster:GetGUIDLow()
end

-- C++ at_bring_your_orphan_to::OnTrigger (childrens_week.cpp:942-986).
local function onAreaTrigger(event, player, triggerId)
    if player:IsDead() or not player:HasAura(SPELL_ORPHAN_OUT) then
        return
    end
    local questId = 0
    local orphanId = 0
    if triggerId == AT_DOWN_AT_THE_DOCKS then
        questId = QUEST_DOWN_AT_THE_DOCKS
        orphanId = ORPHAN_ORCISH
    elseif triggerId == AT_GATEWAY_TO_THE_FRONTIER then
        questId = QUEST_GATEWAY_TO_THE_FRONTIER
        orphanId = ORPHAN_ORCISH
    elseif triggerId == AT_LORDAERON_THRONE_ROOM then
        questId = QUEST_LORDAERON_THRONE_ROOM
        orphanId = ORPHAN_ORCISH
    elseif triggerId == AT_BOUGHT_OF_ETERNALS then
        questId = QUEST_BOUGHT_OF_ETERNALS
        orphanId = ORPHAN_HUMAN
    elseif triggerId == AT_SPOOKY_LIGHTHOUSE then
        questId = QUEST_SPOOKY_LIGHTHOUSE
        orphanId = ORPHAN_HUMAN
    elseif triggerId == AT_STONEWROUGHT_DAM then
        questId = QUEST_STONEWROUGHT_DAM
        orphanId = ORPHAN_HUMAN
    elseif triggerId == AT_DARK_PORTAL then
        if player:GetTeam() == TEAM_ALLIANCE then
            questId = QUEST_DARK_PORTAL_A
            orphanId = ORPHAN_DRAENEI
        else
            questId = QUEST_DARK_PORTAL_H
            orphanId = ORPHAN_BLOOD_ELF
        end
    end
    if questId ~= 0 and orphanId ~= 0
            and getOrphanGUID(player, orphanId) ~= nil
            and player:GetQuestStatus(questId) == QUEST_STATUS_INCOMPLETE then
        player:AreaExploredOrEventHappens(questId)
    end
end

RegisterServerEvent(24, onAreaTrigger)
