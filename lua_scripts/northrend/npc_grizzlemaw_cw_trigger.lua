-- Grizzly Hills: Grizzlemaw Childrens Week trigger — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_grizzlemaw_cw_trigger :
-- CreatureScript("npc_grizzlemaw_cw_trigger")).
-- Entry: 36209 (TDB 26091 creature_template ScriptName
-- npc_grizzlemaw_cw_trigger, verified this tick).
-- Eluna creature events: 23 OnReset, 4 OnDied.
-- Ported arms:
-- - Constructor (childrens_week.cpp:1094-1097): SetDisplayId(11686,
--   DISPLAY_INVISIBLE) rides ev23 while alive (biggest_tree
--   precedent — the constructor leg has no Lua hook of its own).
-- - MoveInLineOfSight (childrens_week.cpp:1099-1110): the sighting
--   leg is a 1s GetPlayersInRange poll — the ev27 twin (ev27 fires
--   only from the aggro-engage branch; the trigger needs a
--   proximity sighting on a non-hostile invisible NPC, not an
--   engage). Poll: any player within 10y 2D with QUEST_HOME_OF_THE_
--   BEAR_MEN (13930) INCOMPLETE whose SPELL_ORPHAN_OUT (58818) aura
--   was cast by the wolvar orphan (33532) gets
--   AreaExploredOrEventHappens(13930) and the orphan Talks 10.
--   The incomplete guard bounds re-fire: once the explore credit
--   completes the quest the poll stops crediting.
--   creature_text 33532 group 10 verified in TDB 26091.
-- Unmodeled (documented-only, no bridges): none.

local ENTRY_GRIZZLEMAW_CW_TRIGGER = 36209

local QUEST_HOME_OF_THE_BEAR_MEN = 13930
local ORPHAN_WOLVAR = 33532
local SPELL_ORPHAN_OUT = 58818
local DISPLAY_INVISIBLE = 11686

local TEXT_WOLVAR_ORPHAN_10 = 10

local QUEST_STATUS_INCOMPLETE = 3
local SIGHT_RANGE = 10.0
local POLL_RANGE = 25.0

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

-- C++ MoveInLineOfSight (childrens_week.cpp:1099-1110), 1s granularity.
triggerTick = function(creature, guid)
    if creature:IsDead() then
        return
    end
    local players = creature:GetPlayersInRange(POLL_RANGE)
    for _, player in ipairs(players) do
        if player:GetQuestStatus(QUEST_HOME_OF_THE_BEAR_MEN)
                == QUEST_STATUS_INCOMPLETE then
            local dx = player:GetX() - creature:GetX()
            local dy = player:GetY() - creature:GetY()
            if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE then
                local og = orphanGUIDFor(player, ORPHAN_WOLVAR)
                if og ~= nil then
                    local orphan = creature:GetCreatureByGUID(og)
                    if orphan ~= nil then
                        player:AreaExploredOrEventHappens(QUEST_HOME_OF_THE_BEAR_MEN)
                        orphan:Talk(TEXT_WOLVAR_ORPHAN_10)
                    end
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

RegisterCreatureEvent(ENTRY_GRIZZLEMAW_CW_TRIGGER, 23, onReset)
RegisterCreatureEvent(ENTRY_GRIZZLEMAW_CW_TRIGGER, 4, onDied)
