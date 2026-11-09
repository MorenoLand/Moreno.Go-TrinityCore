-- Kalimdor: Overlord Runthak, Orgrimmar citizen scene — Lua port of
-- src/server/scripts/Kalimdor/zone_orgrimmar.cpp
-- (class npc_overlord_runthak_orgrimmar : CreatureScript
-- ("npc_overlord_runthak_orgrimmar"); struct
-- npc_overlord_runthak_orgrimmarAI : public ScriptedAI).
-- Entry: 31431 (wowhead WotLK-verified 2026-11-05:
-- https://www.wowhead.com/wotlk/npc=31431/overlord-runthak —
-- "Added in Wrath of the Lich King", 4 quotes = the 4 SAY_RUNTHAK
-- Talk lines below, and the entry sits inside the 31423-31433
-- citizen block. The Classic quest-giver is the separate entry
-- 14392, "Added in Classic", 3 quotes; it carries no scene script
-- in this tree — zone_orgrimmar.cpp's citizen scene is the only
-- Runthak script in the C++ tree).
-- Eluna creature events: 23 OnReset, 4 OnDied, 7 OnAIUpdate.
-- Timers via CreateLuaEvent (aeonus convention: timers table keyed by
-- GUID, cancelled on ev23/ev4); the 13-step chain is one "scene" key
-- (only one step pending at a time, C++ EventMap order).
-- Ported arms (C++ order, zone_orgrimmar.cpp:344-356, 368-433):
-- - Reset: inProgress = false, events.Reset (internal) ->
--   MovePath(me->GetSpawnId() * 10, true) via creature:MoveWaypoint()
--   (assigned path, tick-228 thrall precedent). Runs on every Reset
--   (spawn + evade + scene reset), matching C++.
-- - MoveInLineOfSight trigger (zone_orgrimmar.cpp:358-366): a player
--   within 20yd, once (inProgress latch). No MoveInLOS bridge serves
--   this: ev27 fires only from the aggro-engage branch (2026-10-24
--   note), and this trigger is out-of-combat proximity. Twinned with
--   the ev7 poll (100ms world tick, every alive motion — hydross
--   precedent) + creature:GetPlayersInGrid(20) (live players, same
--   map+instance; the only player-lookup bridge, restless_soul
--   precedent). C++ IsWithinDist is 3D distance; GetPlayersInGrid
--   matches it (3D + reach). The poll returns immediately once the
--   scene is in progress, so the per-tick cost is one cheap latch
--   check after the first trigger.
-- - GetCitizenGuids (zone_orgrimmar.cpp:444-476): the 6 citizens
--   (gryshka 31433, olvia 31425, sana 31429, felika 31427, thathung
--   31430, kaja 31423) via 6x FindNearestCreature(entry, 20) — the
--   C++ grid visit is 20yd (Cell::VisitGridObjects 20.0f) even though
--   its checker tests 25yd, so 20yd is the effective range. Searched
--   once (C++ only searches while gryshkaGUID is empty); the objects
--   are stored per thrall GUID (opera raj.julianne precedent — no
--   GetCreatureByGUID bridge) and survive Reset like the C++ GUIDs;
--   ev4 clears them so a respawn (new lifetime) re-searches.
-- - Scene chain (EVENT_SCENE_1..13 + EVENT_RESET, delays 2s then
--   4.5s x9, 1.5s, 4.5s x2, 25s): Talk lines name-by-name
--   (CitizenTalk enum, zone_orgrimmar.cpp:316-330 comments);
--   cross-creature Talk via the stored objects (mandokir jindo:Talk
--   precedent); MoveIdle (StopMoving twin) + SetFacingToObject in
--   SCENE_1. The reset step calls Reset() (C++ EVENT_RESET), which
--   re-arms MoveWaypoint and clears the scene latch + timers.
-- Unmodeled: none in this script — the MoveInLineOfSight trigger is
-- the ev7/GetPlayersInGrid twin above, not a native ev27 arm.

local ENTRY_RUNTHAK = 31431

local NPC_GRYSHKA = 31433
local NPC_OLVIA = 31425
local NPC_SANA = 31429
local NPC_FELIKA = 31427
local NPC_THATHUNG = 31430
local NPC_KAJA = 31423

local CITIZEN_ENTRIES = {
    gryshka = NPC_GRYSHKA,
    olvia = NPC_OLVIA,
    sana = NPC_SANA,
    felika = NPC_FELIKA,
    thathung = NPC_THATHUNG,
    kaja = NPC_KAJA,
}

-- C++ EVENT_SCENE_1..13 delays (zone_orgrimmar.cpp:374-433):
-- step n runs DELAYS[n] after step n-1 (step 1: 2s after the trigger).
local SCENE_DELAYS = {
    2000, 4500, 4500, 4500, 4500, 4500, 4500, 4500,
    4500, 4500, 1500, 4500, 4500,
}
local SCENE_RESET_DELAY = 25000 -- EVENT_RESET, 25s

local inProgress = {}
local citizens = {}
local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ npc_overlord_runthak_orgrimmarAI::Reset
-- (zone_orgrimmar.cpp:350-356).
local function runthakReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    inProgress[guid] = nil
    creature:MoveWaypoint()
end

local function runthakDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    inProgress[guid] = nil
    citizens[guid] = nil
end

-- C++ UpdateAI EVENT_SCENE_1..13 + EVENT_RESET
-- (zone_orgrimmar.cpp:374-433). Forward-declared: the step closures
-- re-schedule the next step through this name (tharon_ja idiom).
local sceneStep

sceneStep = function(creature, guid, step)
    local folks = citizens[guid] or {}
    local function next(delay)
        schedule(guid, "scene", delay, function()
            sceneStep(creature, guid, step + 1)
        end)
    end
    local function talkCitizen(name, textId)
        local c = folks[name]
        if c and not c:IsDead() then
            c:Talk(textId)
        end
    end
    if step == 1 then
        -- GetCitizenGuids(); MoveIdle; face gryshka; gryshka Talk(0).
        if not citizens[guid] then
            local found = {}
            for name, entry in pairs(CITIZEN_ENTRIES) do
                found[name] = creature:FindNearestCreature(entry, 20)
            end
            citizens[guid] = found
            folks = found
        end
        creature:MoveIdle()
        local gryshka = folks.gryshka
        if gryshka and not gryshka:IsDead() then
            creature:SetFacingToObject(gryshka)
            gryshka:Talk(0)
        end
        next(SCENE_DELAYS[2])
    elseif step == 2 then
        talkCitizen("olvia", 0)
        next(SCENE_DELAYS[3])
    elseif step == 3 then
        talkCitizen("felika", 0)
        next(SCENE_DELAYS[4])
    elseif step == 4 then
        talkCitizen("thathung", 0)
        next(SCENE_DELAYS[5])
    elseif step == 5 then
        talkCitizen("sana", 0)
        next(SCENE_DELAYS[6])
    elseif step == 6 then
        talkCitizen("gryshka", 1)
        next(SCENE_DELAYS[7])
    elseif step == 7 then
        talkCitizen("kaja", 0)
        next(SCENE_DELAYS[8])
    elseif step == 8 then
        talkCitizen("felika", 1)
        next(SCENE_DELAYS[9])
    elseif step == 9 then
        talkCitizen("olvia", 1)
        next(SCENE_DELAYS[10])
    elseif step == 10 then
        creature:Talk(0)
        next(SCENE_DELAYS[11])
    elseif step == 11 then
        creature:Talk(1)
        next(SCENE_DELAYS[12])
    elseif step == 12 then
        creature:Talk(2)
        next(SCENE_DELAYS[13])
    elseif step == 13 then
        creature:Talk(3)
        schedule(guid, "reset", SCENE_RESET_DELAY, function()
            runthakReset(0, creature)
        end)
    end
end

-- C++ MoveInLineOfSight twin (zone_orgrimmar.cpp:358-366): ev7 poll,
-- a player within 20yd starts the scene once.
local function runthakAIUpdate(event, creature, diff)
    local guid = creature:GetGUIDLow()
    if inProgress[guid] then
        return
    end
    local players = creature:GetPlayersInGrid(20)
    if players and #players > 0 then
        inProgress[guid] = true
        schedule(guid, "scene", SCENE_DELAYS[1], function()
            sceneStep(creature, guid, 1)
        end)
    end
end

RegisterCreatureEvent(ENTRY_RUNTHAK, 23, runthakReset)
RegisterCreatureEvent(ENTRY_RUNTHAK, 4, runthakDied)
RegisterCreatureEvent(ENTRY_RUNTHAK, 7, runthakAIUpdate)
