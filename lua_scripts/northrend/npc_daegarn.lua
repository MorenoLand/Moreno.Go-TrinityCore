-- Daegarn (Howling Fjord) --
-- Lua port of src/server/scripts/Northrend/zone_howling_fjord.cpp
-- (npc_daegarn — Reset 40s idle-Talk loop only).
-- Entry: 24151 'Daegarn' (TDB-verified, ScriptName npc_daegarn,
-- 1 spawn map 571 guid 103723).
-- C++ Reset (:279-291): the state clears (_eventInProgress,
-- _playerGUID, _scheduler.CancelAll, _summons.DespawnAll) are
-- vacuous in Lua (no scheduler, no summons, no event flags); the
-- live arm is the 40s repeating Talk(SAY_TEXT 0), cancelled by
-- OnQuestAccept quest 11300 (:375-403, _scheduler.CancelAll()).
-- The 20s stuck-check Reset path and the event-end Reset paths
-- have no Lua equivalent — no deviation in practice, since the
-- gladiator event never runs in Lua (see below).
-- JustSummoned :326 + SummonedCreatureDies :349 stay out: no fire
-- site — the only summon site is SummonGladiator, driven by the
-- unported OnQuestAccept quest-11300 chain.
-- Eluna creature events: 23 OnReset, 31 OnQuestAccept (ev23 fires
-- on spawn too — fireCreatureSpawned pre-fires OnReset, then
-- OnSpawn; the hanes ev31 latch precedent).
-- Bridges: creature:Talk lua_creature_events.go:1252; repeating
-- timer via CreateLuaEvent (lecraft/hanes pump precedent, repeat
-- count 0); quest:GetId() (hanes precedent).

local DAEGARN_ENTRY = 24151
local QUEST_DEFEAT_AT_RING = 11300
local SAY_TEXT = 0

local daegarnTalk = {}

local function cancelTalk(guid)
    local id = daegarnTalk[guid]
    if id then
        RemoveEventById(id)
        daegarnTalk[guid] = nil
    end
end

-- C++ Reset (:279-291): cancel-then-arm, matching CancelAll +
-- re-schedule — a re-Reset restarts the 40s loop, it never doubles.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTalk(guid)
    daegarnTalk[guid] = CreateLuaEvent(function()
        creature:Talk(SAY_TEXT)
    end, 40000, 0)
end

-- C++ OnQuestAccept (:375-403): quest 11300 cancels the scheduler.
local function onQuestAccept(_, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_DEFEAT_AT_RING then
        return
    end
    cancelTalk(creature:GetGUIDLow())
end

RegisterCreatureEvent(DAEGARN_ENTRY, 23, onReset)
RegisterCreatureEvent(DAEGARN_ENTRY, 31, onQuestAccept)
