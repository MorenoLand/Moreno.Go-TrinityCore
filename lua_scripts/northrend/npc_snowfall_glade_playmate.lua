-- Howling Fjord: Snowfall Glade Playmate — Lua port of
-- src/server/scripts/Events/childrens_week.cpp
-- (class npc_snowfall_glade_playmate : CreatureScript
-- ("npc_snowfall_glade_playmate")).
-- Entry: 34490 (creature_template ScriptName
-- npc_snowfall_glade_playmate, TDB 26091).
-- Eluna creature events: 23 OnReset, 4 OnDied.
-- Ported arms:
-- - MoveInLineOfSight + UpdateAI orphan phase machine
--   (childrens_week.cpp:279-291 sighting, :293-358 machine): the
--   sighting leg is a 1s GetPlayersInRange poll — the ev27 twin
--   (ev27 fires only from the aggro-engage branch; the orphan
--   trigger needs a proximity sighting, not an engage). Poll: any
--   player within 10y 2D with QUEST_PLAYMATE_WOLVAR (13951)
--   INCOMPLETE whose SPELL_ORPHAN_OUT (58818) aura was cast by the
--   wolvar orphan (33532) captures playerGUID/orphanGUID and starts
--   phase 1. Phase machine (1s pump granularity): p1 orphan
--   MovePoint 5y in front + Talk 1 (5000); p2 orphan faces me +
--   Talk 1 + DoCast(orphan, SPELL_SNOWBALL 21343) (5000); p3 Talk 2
--   (5000); p4 orphan Talk 2 + orphan CastSpell(me, 21343) (5000);
--   p5 orphan Talk 3 + GroupEventHappens(13951) + orphan
--   MoveFollow(player) + Reset. Player/orphan unresolvable on a
--   tick -> Reset (C++ UpdateAI null-guard). Timer-0 first step
--   fires on the sighting tick (C++-exact); later steps land on 1s
--   boundaries. creature_text 34490 groups 1-2 / 33532 groups 1-5,
--   10-13 verified in TDB 26091.
-- Unmodeled (documented-only, no bridges): none.

local ENTRY_SNOWFALL_GLADE_PLAYMATE = 34490

local QUEST_PLAYMATE_WOLVAR = 13951
local ORPHAN_WOLVAR = 33532
local SPELL_ORPHAN_OUT = 58818
local SPELL_SNOWBALL = 21343

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

-- C++ MoveInLineOfSight (childrens_week.cpp:279-291) + UpdateAI
-- (childrens_week.cpp:293-358), 1s granularity.
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
        -- incomplete and the orphan-out aura from the wolvar orphan.
        -- C++ fires on the first unit to move in; the poll picks the
        -- first list match instead (1s granularity twin).
        local sighted = false
        local players = creature:GetPlayersInRange(POLL_RANGE)
        for _, player in ipairs(players) do
            if player:GetQuestStatus(QUEST_PLAYMATE_WOLVAR)
                    == QUEST_STATUS_INCOMPLETE then
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                if dx * dx + dy * dy < SIGHT_RANGE * SIGHT_RANGE then
                    local og = orphanGUIDFor(player, ORPHAN_WOLVAR)
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
        -- C++ UpdateAI null-guard calls Reset(): back to phase 0,
        -- poll re-arms for the next player.
        states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
        armPump(guid, creature)
        return
    end
    if st.delay > 1000 then
        st.delay = st.delay - 1000
        armPump(guid, creature)
        return
    end
    if st.phase == 1 then
        moveOrphanInFront(creature, orphan)
        orphan:Talk(1)
        st.delay = 5000
    elseif st.phase == 2 then
        orphan:SetFacingToObject(creature)
        creature:Talk(1)
        creature:CastSpell(orphan, SPELL_SNOWBALL, false)
        st.delay = 5000
    elseif st.phase == 3 then
        creature:Talk(2)
        st.delay = 5000
    elseif st.phase == 4 then
        orphan:Talk(2)
        orphan:CastSpell(creature, SPELL_SNOWBALL, false)
        st.delay = 5000
    elseif st.phase == 5 then
        orphan:Talk(3)
        player:GroupEventHappens(QUEST_PLAYMATE_WOLVAR, creature)
        orphan:MoveFollow(player, PET_FOLLOW_DIST, PET_FOLLOW_ANGLE)
        -- C++ Reset(): back to phase 0, poll re-arms for the next player.
        states[guid] = { phase = 0, delay = 0, playerGUID = 0, orphanGUID = 0 }
        armPump(guid, creature)
        return
    else
        states[guid] = nil
        return
    end
    st.phase = st.phase + 1
    armPump(guid, creature)
end

-- C++ npc_snowfall_glade_playmateAI::Reset
-- (childrens_week.cpp:274-277) + Initialize
-- (childrens_week.cpp:266-272): timer/phase/GUIDs cleared. Go fires
-- ev23 on spawn, evade and death; the dead arm drops the state
-- (kerlonian idiom) and the pump restarts in phase 0.
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

RegisterCreatureEvent(ENTRY_SNOWFALL_GLADE_PLAYMATE, 23, onReset)
RegisterCreatureEvent(ENTRY_SNOWFALL_GLADE_PLAYMATE, 4, onDied)
