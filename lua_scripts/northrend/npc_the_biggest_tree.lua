-- Grizzly Hills: The Biggest Tree — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_the_biggest_tree : CreatureScript
-- ("npc_the_biggest_tree")).
-- Entry: 34381 (creature_template ScriptName npc_the_biggest_tree,
-- TDB 26091).
-- Eluna creature events: 23 OnReset, 4 OnDied.
-- Ported arms:
-- - Constructor SetDisplayId(DISPLAY_INVISIBLE 11686)
--   (childrens_week.cpp:370): set on ev23 while alive (ev23 fires
--   on spawn; re-setting on evade is idempotent, C++ Reset never
--   touches the display).
-- - MoveInLineOfSight + UpdateAI orphan phase machine
--   (childrens_week.cpp:386-397 sighting, :399-453 machine): the
--   sighting leg is a 1s GetPlayersInRange poll — the ev27 twin
--   (ev27 fires only from the aggro-engage branch; the orphan
--   trigger needs a proximity sighting, not an engage). Poll: any
--   player within 10y 2D with QUEST_THE_BIGGEST_TREE_EVER (13929)
--   INCOMPLETE whose SPELL_ORPHAN_OUT (58818) aura was cast by the
--   oracle orphan (33533) captures playerGUID/orphanGUID and starts
--   phase 1. Phase machine (1s pump granularity; initial timer
--   1000, so phase 1 fires on the first tick after sighting,
--   C++-exact): p1 orphan MovePoint 5y in front, no Talk (2000);
--   p2 orphan faces me + Talk 4 (5000); p3 GroupEventHappens(13929)
--   + orphan MoveFollow(player) + Reset. Player/orphan
--   unresolvable on a tick -> Reset (C++ UpdateAI null-guard). The
--   tree itself never Talks (no creature_text rows for 34381 — the
--   Talk 4 is the orphan's, group 4 of 33533, verified in TDB
--   26091).
-- Unmodeled (documented-only, no bridges): none.

local ENTRY_THE_BIGGEST_TREE = 34381

local QUEST_THE_BIGGEST_TREE_EVER = 13929
local ORPHAN_ORACLE = 33533
local SPELL_ORPHAN_OUT = 58818

local DISPLAY_INVISIBLE = 11686

local QUEST_STATUS_INCOMPLETE = 3
local SIGHT_RANGE = 10.0
local POLL_RANGE = 25.0
local PET_FOLLOW_DIST = 1.0
local PET_FOLLOW_ANGLE = math.pi / 2

local timers = {}
local states = {}

-- Forward declaration: tick's closure calls orphanTick, assigned below
-- (tharon_ja/gal_darah idiom; a later `local function` would shadow
-- the slot and nil-call at runtime).
local orphanTick

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
        orphanTick(creature, guid)
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

-- C++ orphan->GetMotionMaster()->MovePoint(0, me + front * 5):
-- creature:MoveTo is the MovePoint twin (grizzly precedent).
local function moveOrphanInFront(creature, orphan)
    local o = creature:GetO()
    orphan:MoveTo(creature:GetX() + math.cos(o) * 5,
        creature:GetY() + math.sin(o) * 5, creature:GetZ())
end

-- C++ MoveInLineOfSight (childrens_week.cpp:386-397) + UpdateAI
-- (childrens_week.cpp:399-453), 1s granularity.
orphanTick = function(creature, guid)
    local st = states[guid]
    if not st then
        return
    end
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    if st.phase == 0 then
        -- Sighting poll: first player in range with the quest
        -- incomplete and the orphan-out aura from the oracle orphan.
        -- C++ fires on the first unit to move in; the poll picks the
        -- first list match instead (1s granularity twin).
        local sighted = false
        local players = creature:GetPlayersInRange(POLL_RANGE)
        for _, player in ipairs(players) do
            if player:GetQuestStatus(QUEST_THE_BIGGEST_TREE_EVER)
                    == QUEST_STATUS_INCOMPLETE then
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE then
                    local og = orphanGUIDFor(player, ORPHAN_ORACLE)
                    if og ~= nil then
                        st.phase = 1
                        st.playerGUID = player:GetGUID()
                        st.orphanGUID = og
                        st.delay = 1000
                        sighted = true
                        break
                    end
                end
            end
        end
        if not sighted or st.delay > 0 then
            armPump(guid, creature)
            return
        end
        -- delay 0: fall through and run phase 1 on the sighting tick.
    end
    local player = creature:GetPlayerByGUID(st.playerGUID)
    local orphan = creature:GetCreatureByGUID(st.orphanGUID)
    if player == nil or orphan == nil then
        states[guid] = nil
        return
    end
    if st.delay > 1000 then
        st.delay = st.delay - 1000
        armPump(guid, creature)
        return
    end
    if st.phase == 1 then
        moveOrphanInFront(creature, orphan)
        st.delay = 2000
    elseif st.phase == 2 then
        orphan:SetFacingToObject(creature)
        orphan:Talk(4)
        st.delay = 5000
    elseif st.phase == 3 then
        player:GroupEventHappens(QUEST_THE_BIGGEST_TREE_EVER, creature)
        orphan:MoveFollow(player, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
        states[guid] = nil
        return
    else
        states[guid] = nil
        return
    end
    st.phase = st.phase + 1
    armPump(guid, creature)
end

-- C++ npc_the_biggest_treeAI::Reset (childrens_week.cpp:381-384) +
-- Initialize (childrens_week.cpp:373-379): timer/phase/GUIDs cleared.
-- Go fires ev23 on spawn, evade and death; the dead arm drops the
-- state (kerlonian idiom) and the pump restarts in phase 0. The
-- constructor SetDisplayId leg rides ev23 while alive.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    creature:SetDisplayId(DISPLAY_INVISIBLE)
    states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
    armPump(guid, creature)
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    states[guid] = nil
end

RegisterCreatureEvent(ENTRY_THE_BIGGEST_TREE, 23, onReset)
RegisterCreatureEvent(ENTRY_THE_BIGGEST_TREE, 4, onDied)
