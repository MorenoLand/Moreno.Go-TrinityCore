-- Un'Goro Crater: The Etymidian — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_the_etymidian : CreatureScript("npc_the_etymidian")).
-- Entry: 28092 (wowhead npc=28092/the-etymidian page, verified this
-- tick; no NPC_ constant exists in the C++ file — the telestra /
-- aeranas wowhead-only precedent, never guessed).
-- Eluna creature events: 34 OnQuestReward, 23 OnReset, 4 OnDied
-- (engine/world/quest_reward_grant.go fires it as (event, player,
-- creature, quest); quest:GetId() wired — luaQuest surface).
-- Ported arms:
-- - OnQuestReward (childrens_week.cpp:679-685, event 34): quest id
--   12547 (QUEST_THE_ACTIVATION_RUNE) -> Talk(SAY_ACTIVATION 0).
-- - MoveInLineOfSight + UpdateAI orphan phase machine
--   (childrens_week.cpp:687-767): the sighting leg is a 1s
--   GetPlayersInRange poll — the ev27 twin (ev27 fires only from
--   the aggro-engage branch; the orphan trigger needs a proximity
--   sighting, not an engage). Poll: any player within 10y 2D with
--   QUEST_MEETING_A_GREAT_ONE (13956) INCOMPLETE whose
--   SPELL_ORPHAN_OUT (58818) aura was cast by the oracle orphan
--   (33533) captures playerGUID/orphanGUID and starts phase 1.
--   Phase machine (5s steps, 1s pump granularity): p1 orphan
--   MovePoint 5y in front + Talk 7; p2 orphan faces me + Talk 8;
--   p3 Talk 9; p4 Talk 10; p5 orphan MoveFollow(player) +
--   GroupEventHappens(13956) + Reset. Player/orphan unresolvable
--   on a tick -> Reset (C++ UpdateAI null-guard). Timer-0 first
--   step fires on the sighting tick (C++-exact); later steps land
--   on 1s boundaries. creature_text 28092 group 0 / 33533 groups
--   1-14 verified in TDB 26091.
-- Unmodeled (documented-only, no bridges): none.

local ENTRY_THE_ETYMIDIAN = 28092

local SAY_ACTIVATION = 0

local QUEST_THE_ACTIVATION_RUNE = 12547

-- C++ OnQuestReward for quest 12547: Talk(0).
local function onQuestReward(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_THE_ACTIVATION_RUNE then
        return
    end
    creature:Talk(SAY_ACTIVATION)
end

RegisterCreatureEvent(ENTRY_THE_ETYMIDIAN, 34, onQuestReward)

local QUEST_MEETING_A_GREAT_ONE = 13956
local ORPHAN_ORACLE = 33533
local SPELL_ORPHAN_OUT = 58818

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

-- C++ MoveInLineOfSight (childrens_week.cpp:687-701) + UpdateAI
-- (childrens_week.cpp:704-767), 1s granularity.
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
            if player:GetQuestStatus(QUEST_MEETING_A_GREAT_ONE)
                    == QUEST_STATUS_INCOMPLETE then
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE then
                    local og = orphanGUIDFor(player, ORPHAN_ORACLE)
                    if og ~= nil then
                        st.phase = 1
                        st.playerGUID = player:GetGUID()
                        st.orphanGUID = og
                        st.delay = 0
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
        orphan:Talk(7)
        st.delay = 5000
    elseif st.phase == 2 then
        orphan:SetFacingToObject(creature)
        orphan:Talk(8)
        st.delay = 5000
    elseif st.phase == 3 then
        orphan:Talk(9)
        st.delay = 5000
    elseif st.phase == 4 then
        orphan:Talk(10)
        st.delay = 5000
    elseif st.phase == 5 then
        orphan:MoveFollow(player, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
        player:GroupEventHappens(QUEST_MEETING_A_GREAT_ONE, creature)
        states[guid] = nil
        return
    else
        states[guid] = nil
        return
    end
    st.phase = st.phase + 1
    armPump(guid, creature)
end

-- C++ npc_the_etymidianAI::Reset (childrens_week.cpp:674-677) +
-- Initialize (childrens_week.cpp:666-672): timer/phase/GUIDs cleared.
-- Go fires ev23 on spawn, evade and death; the dead arm drops the
-- state (kerlonian idiom) and the pump restarts in phase 0.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    if creature:IsDead() then
        states[guid] = nil
        return
    end
    states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
    armPump(guid, creature)
end

local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    states[guid] = nil
end

RegisterCreatureEvent(ENTRY_THE_ETYMIDIAN, 23, onReset)
RegisterCreatureEvent(ENTRY_THE_ETYMIDIAN, 4, onDied)
