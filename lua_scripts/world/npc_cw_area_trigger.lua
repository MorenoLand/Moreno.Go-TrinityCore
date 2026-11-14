-- Childrens Week area triggers — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_cw_area_trigger : CreatureScript("npc_cw_area_trigger")).
-- Entries (TDB 26091 creature_template ScriptName npc_cw_area_trigger,
-- verified this tick): 22872 Caverns of Time, 22851 Exodar 01,
-- 22905 Exodar 02, 22838 Aeris Landing, 22831 Auchindoun,
-- 22829 Sporeggar, 22839 Throne of Elements, 22866 Silvermoon 01.
-- Eluna creature events: 23 OnReset, 4 OnDied.
-- Ported arms:
-- - Constructor (childrens_week.cpp:1006-1009): SetDisplayId(11686,
--   DISPLAY_INVISIBLE) rides ev23 while alive (biggest_tree
--   precedent — the constructor leg has no Lua hook of its own).
-- - MoveInLineOfSight (childrens_week.cpp:1011-1077): the sighting
--   leg is a 1s GetPlayersInRange poll — the ev27 twin (ev27 fires
--   only from the aggro-engage branch; the trigger needs a
--   proximity sighting on a non-hostile invisible NPC, not an
--   engage). Poll: any player within 20y 2D with SPELL_ORPHAN_OUT
--   (58818) whose orphan-out aura was cast by the entry's orphan
--   and whose quest is INCOMPLETE gets AreaExploredOrEventHappens.
--   Per-entry quest/orphan (C++-exact): 22872 team-gated
--   (Alliance 10962 + draenei 22818 / Horde 10963 + blood elf
--   22817); 22851 quest 10956 + draenei; 22905 quest 10968 +
--   draenei; 22838 quest 10954 + draenei; 22831 quest 10950 +
--   draenei; 22829 quest 10945 + blood elf; 22839 quest 10953 +
--   blood elf. 22866 Silvermoon 01 is its own leg: quest 11975
--   incomplete + blood-elf orphan -> AreaExploredOrEventHappens;
--   once COMPLETE, nearest samuro (25151, 20y) performs one random
--   emote (WAVE 3 / ROAR 15 / FLEX 23 / SALUTE 66 / DANCE 94,
--   SharedDefines.h; PerformEmote is the HandleEmoteCommand twin).
--   The generic credit tail is skipped for 22866 (C++-exact — its
--   case sets no questId/orphanId).
-- Unmodeled (documented-only, no bridges): none.

local SPELL_ORPHAN_OUT = 58818
local DISPLAY_INVISIBLE = 11686

local ORPHAN_BLOOD_ELF = 22817
local ORPHAN_DRAENEI = 22818

local NPC_SAMURO = 25151

local QUEST_TIME_TO_VISIT_THE_CAVERNS_A = 10962
local QUEST_TIME_TO_VISIT_THE_CAVERNS_H = 10963
local QUEST_THE_SEAT_OF_THE_NARUU = 10956
local QUEST_CALL_ON_THE_FARSEER = 10968
local QUEST_JHEEL_IS_AT_AERIS_LANDING = 10954
local QUEST_AUCHINDOUN_AND_THE_RING = 10950
local QUEST_HCHUU_AND_THE_MUSHROOM_PEOPLE = 10945
local QUEST_VISIT_THE_THRONE_OF_ELEMENTS = 10953
local QUEST_NOW_WHEN_I_GROW_UP = 11975

local NPC_CAVERNS_OF_TIME_CW_TRIGGER = 22872
local NPC_EXODAR_01_CW_TRIGGER = 22851
local NPC_EXODAR_02_CW_TRIGGER = 22905
local NPC_AERIS_LANDING_CW_TRIGGER = 22838
local NPC_AUCHINDOUN_CW_TRIGGER = 22831
local NPC_SPOREGGAR_CW_TRIGGER = 22829
local NPC_THRONE_OF_ELEMENTS_CW_TRIGGER = 22839
local NPC_SILVERMOON_01_CW_TRIGGER = 22866

local TEAM_ALLIANCE = 0

local QUEST_STATUS_INCOMPLETE = 3
local QUEST_STATUS_COMPLETE = 1
local SIGHT_RANGE = 20.0
local POLL_RANGE = 25.0

local EMOTES_SAMURO = { 3, 15, 23, 66, 94 }

local ENTRIES = {
    NPC_CAVERNS_OF_TIME_CW_TRIGGER,
    NPC_EXODAR_01_CW_TRIGGER,
    NPC_EXODAR_02_CW_TRIGGER,
    NPC_AERIS_LANDING_CW_TRIGGER,
    NPC_AUCHINDOUN_CW_TRIGGER,
    NPC_SPOREGGAR_CW_TRIGGER,
    NPC_THRONE_OF_ELEMENTS_CW_TRIGGER,
    NPC_SILVERMOON_01_CW_TRIGGER,
}

local timers = {}

-- Forward declaration: tick's closure calls triggerTick, assigned
-- below (tharon_ja/gal_darah idiom; a later `local function` would
-- shadow the slot and nil-call at runtime).
local triggerTick

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function armPump(guid, creature)
    timers[guid] = CreateLuaEvent(function()
        timers[guid] = nil
        triggerTick(creature, guid)
    end, 1000)
end

-- C++ getOrphanGUID (childrens_week.cpp:136-144): the SPELL_ORPHAN_OUT
-- aura on the player, when its caster is the expected orphan entry,
-- yields the orphan's GUID; otherwise empty.
local function orphanGUIDFor(player, orphanEntry)
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

-- C++ MoveInLineOfSight (childrens_week.cpp:1011-1077), 1s granularity.
triggerTick = function(creature, guid)
    if creature:IsDead() then
        return
    end
    local entry = creature:GetEntry()
    local players = creature:GetPlayersInRange(POLL_RANGE)
    for _, player in ipairs(players) do
        local dx = player:GetX() - creature:GetX()
        local dy = player:GetY() - creature:GetY()
        if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE
                and player:HasAura(SPELL_ORPHAN_OUT) then
            if entry == NPC_SILVERMOON_01_CW_TRIGGER then
                if player:GetQuestStatus(QUEST_NOW_WHEN_I_GROW_UP)
                        == QUEST_STATUS_INCOMPLETE
                        and orphanGUIDFor(player, ORPHAN_BLOOD_ELF) ~= nil then
                    player:AreaExploredOrEventHappens(QUEST_NOW_WHEN_I_GROW_UP)
                    if player:GetQuestStatus(QUEST_NOW_WHEN_I_GROW_UP)
                            == QUEST_STATUS_COMPLETE then
                        local samuro = creature:FindNearestCreature(NPC_SAMURO, 20.0)
                        if samuro then
                            samuro:PerformEmote(EMOTES_SAMURO[math.random(#EMOTES_SAMURO)])
                        end
                    end
                end
            else
                local questId = 0
                local orphanId = 0
                if entry == NPC_CAVERNS_OF_TIME_CW_TRIGGER then
                    if player:GetTeam() == TEAM_ALLIANCE then
                        questId = QUEST_TIME_TO_VISIT_THE_CAVERNS_A
                        orphanId = ORPHAN_DRAENEI
                    else
                        questId = QUEST_TIME_TO_VISIT_THE_CAVERNS_H
                        orphanId = ORPHAN_BLOOD_ELF
                    end
                elseif entry == NPC_EXODAR_01_CW_TRIGGER then
                    questId = QUEST_THE_SEAT_OF_THE_NARUU
                    orphanId = ORPHAN_DRAENEI
                elseif entry == NPC_EXODAR_02_CW_TRIGGER then
                    questId = QUEST_CALL_ON_THE_FARSEER
                    orphanId = ORPHAN_DRAENEI
                elseif entry == NPC_AERIS_LANDING_CW_TRIGGER then
                    questId = QUEST_JHEEL_IS_AT_AERIS_LANDING
                    orphanId = ORPHAN_DRAENEI
                elseif entry == NPC_AUCHINDOUN_CW_TRIGGER then
                    questId = QUEST_AUCHINDOUN_AND_THE_RING
                    orphanId = ORPHAN_DRAENEI
                elseif entry == NPC_SPOREGGAR_CW_TRIGGER then
                    questId = QUEST_HCHUU_AND_THE_MUSHROOM_PEOPLE
                    orphanId = ORPHAN_BLOOD_ELF
                elseif entry == NPC_THRONE_OF_ELEMENTS_CW_TRIGGER then
                    questId = QUEST_VISIT_THE_THRONE_OF_ELEMENTS
                    orphanId = ORPHAN_BLOOD_ELF
                end
                if questId ~= 0 and orphanId ~= 0
                        and orphanGUIDFor(player, orphanId) ~= nil
                        and player:GetQuestStatus(questId) == QUEST_STATUS_INCOMPLETE then
                    player:AreaExploredOrEventHappens(questId)
                end
            end
        end
    end
    armPump(guid, creature)
end

-- The constructor SetDisplayId leg rides ev23 while alive; Go fires
-- ev23 on spawn, evade and death (biggest_tree precedent).
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    if creature:IsDead() then
        return
    end
    creature:SetDisplayId(DISPLAY_INVISIBLE)
    armPump(guid, creature)
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
end

for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 23, onReset)
    RegisterCreatureEvent(entry, 4, onDied)
end
