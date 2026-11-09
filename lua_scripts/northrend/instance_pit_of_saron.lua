-- The Pit of Saron instance script — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/instance_pit_of_saron.cpp
-- (class instance_pit_of_saron : public InstanceMapScript, map 658).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_forge_of_souls.lua / instance_violet_hold.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 658
-- (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- garfrost / krick / ick / tyrannus / rimefang / tyrannusEvents —
-- the OnCreatureCreate binds (instance_pit_of_saron.cpp:71-100);
-- jainaOrSylvanas1 — the part-1 leader bind (:97-100, one GUID for
-- either Jaina or Sylvanas part 1, team decided at spawn);
-- cavernTriggers — the NPC_CAVERN_EVENT_TRIGGER 32780 vector
-- (:94-96), a list, not a single GUID.
-- teamInInstance — the _teamInInstance latch (:62-67, ctor default 0
-- = ALLIANCE; nil reads as 0, the C++ default); shardsHit /
-- cavernActive — the DATA_ICE_SHARDS_HIT (9) / DATA_CAVERN_ACTIVE
-- (10) stores (:221-235), plain numbers, nil reads as 0.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound
--   (naxxramas LoadGrid delta).
-- - GetGuidData (instance_pit_of_saron.cpp:238-262): the Lua mirror
--   is the instanceData binds above, read as instanceData.garfrost /
--   instanceData.tyrannus / etc. (oculus precedent). Live consumer:
--   boss_forgemaster_garfrost.lua JustDied reads instanceData.tyrannus
--   for the SAY_TYRANNUS_DEATH Talk (the kelthuzad GetInstanceData
--   precedent); the ick outro relay keeps its own 500yd sweep.
-- - OnPlayerEnter team latch (:62-67) — PORTED 2026-11-08 (tick 490)
--   via INSTANCE_EVENT_ON_PLAYER_ENTER (4, instance_hooks.go:479):
--   if teamInInstance is unset, latch player:GetTeam()
--   (lua_player.go:346; 0 Alliance / 1 Horde, the C++ TeamId). The
--   OnCreatureCreate first-player latch (:64-70) stays out: the hook
--   args carry no player handle, and the global GetPlayers is
--   server-wide, not instance-scoped (engine/scripting/globals.go:1256).
-- - SetBossState override DONE-spawn legs (:157-190) — PORTED
--   2026-11-08 (tick 490) in the boss JustDied handlers (order-exact:
--   C++ fires the override inside SetBossState, which the boss files
--   call before Talk(DEATH) for garfrost and after it for tyrannus):
--   garfrost DONE summons Martin Victus 1 (37591, Alliance) / Gorkun
--   Ironskull 1 (37592, Horde) at SlaveLeaderPos; tyrannus DONE
--   summons Jaina part 2 (38188, Alliance) / Sylvanas part 2 (38189,
--   Horde) at EventLeaderPos2 — both TEMPSUMMON_MANUAL_DESPAWN (7),
--   team read as instanceData.teamInInstance or 0 (the C++ ctor
--   default). The legs ride the boss handlers, not an instance hook:
--   Go has no OnBossStateChange instance event, and the VH OnUnitDeath
--   precedent does not apply (those legs only read state; these
--   summon from the instance).
-- - SetData switch (:221-235) — PORTED 2026-11-08 (tick 490) via the
--   Go-only InstanceEventOnSetData bridge (event 8,
--   instance_hooks.go:43-63; the violet-hold / eye-of-eternity
--   precedent: the writer calls instanceData:SetData(type, data) and
--   this handler runs the switch leg; no table write happens in the
--   bridge itself, so every leg stores its own state below):
--   DATA_ICE_SHARDS_HIT (9): store shardsHit. DORMANT: the only
--   writer is the spell_pos_ice_shards SpellScript (pit_of_saron.cpp
--   :297, no SpellScript bridge), so no live writer exists yet.
--   DATA_CAVERN_ACTIVE (10): store cavernActive; data ~= 0 runs
--   HandleCavernEventTrigger(true) (:265-276), data == 0 runs it with
--   false. DORMANT: the only writers are the cavern area triggers
--   (pit_of_saron.cpp:317-355, trigger IDs live only in
--   AreaTrigger.dbc), so no live writer exists yet — but the handler
--   is live the moment one appears.
-- - HandleCavernEventTrigger (:265-276) + ScheduledIcicleSummons
--   (pit_of_saron.cpp:48-60) — PORTED 2026-11-08 (tick 490) as a
--   per-trigger CreateLuaEvent chain over the bound cavernTriggers:
--   first tick at 1000ms (:270, CalculateTime(1s)); each tick rolls
--   12% — on success the trigger casts SPELL_ICICLE_SUMMON 69424
--   triggered on itself and re-schedules {20s,35s}, else re-schedules
--   {1s,20s} (randtime legs, C++ order). Deactivation cancels every
--   trigger timer (the KillAllEvents(false) twin). C++ stacks a second
--   chain on a double activation; Lua replaces per-trigger (the only
--   writer guards re-activation upstream, pit_of_saron.cpp:326-327).
-- Unmodeled (documented-only, no bridges):
-- - GetCreatureEntry team swap (:104-148): no entry-remap fire site
--   in Go — spawns are static template rows and OnCreatureCreate
--   fires post-materialization with the DB entry already fixed, so a
--   Horde-side spawn (Sylvanas part 1, Kilara, ...) stays Horde-side
--   for an Alliance group. Known fidelity gap, not fixable from Lua.
-- - DoorData (:33-38: ice-wall passage doors on DATA_GARFROST /
--   DATA_ICK, portcullis on DATA_TYRANNUS): no door model in the Go
--   boss-state store (instance_boss_states.go:72).
-- - The area-trigger writers of DATA_CAVERN_ACTIVE / the
--   Don't-Look-Up credit (pit_of_saron.cpp:317-355): server event 24
--   IS bridged (areatrigger.go:64), but the trigger IDs live only in
--   AreaTrigger.dbc / the DB areatrigger_template — neither is
--   in-tree, and this clone carries no sql/ tree — so no handler can
--   dispatch on them. The at_pit_cavern_end achievement credit
--   (DoUpdateAchievementCriteria BE_SPELL_TARGET 72845) has no Lua
--   bridge either.
-- - _jainaOrSylvanas2GUID is NEVER assigned in the C++ file (only
--   declared :286 and returned :257) — GetGuidData(
--   DATA_JAINA_SYLVANAS_2) always returns Empty; no bind for it.
-- - SetHeaders(DataHeader "POS") / SetBossNumber(EncounterCount 3)
--   / SaveToDB: no bridges; the boss-state store is written via
--   creature:SetBossState (lua_creature_events.go:5917) and read via
--   creature:GetBossState (lua_creature_events.go:5936).
-- No ON_UPDATE poll: the cavern machine rides CreateLuaEvent timers,
-- not a poll.

local MAP_PIT_OF_SARON = 658
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_SET_DATA = 8

-- pit_of_saron.h POSDataTypes (:26-40).
local DATA_ICE_SHARDS_HIT = 9
local DATA_CAVERN_ACTIVE = 10

local NPC_GARFROST = 36494 -- pit_of_saron.h:49
local NPC_KRICK = 36477 -- pit_of_saron.h:50
local NPC_ICK = 36476 -- pit_of_saron.h:51
local NPC_TYRANNUS = 36658 -- pit_of_saron.h:52
local NPC_RIMEFANG = 36661 -- pit_of_saron.h:53
local NPC_TYRANNUS_EVENTS = 36794 -- pit_of_saron.h:55
local NPC_SYLVANAS_PART1 = 36990 -- pit_of_saron.h:57
local NPC_JAINA_PART1 = 36993 -- pit_of_saron.h:59
local NPC_SYLVANAS_PART2 = 38189 -- pit_of_saron.h:58
local NPC_JAINA_PART2 = 38188 -- pit_of_saron.h:60
local NPC_CAVERN_EVENT_TRIGGER = 32780 -- pit_of_saron.h:94

local SPELL_ICICLE_SUMMON = 69424 -- pit_of_saron.h:105

local bossBinds = {
    [NPC_GARFROST] = "garfrost",
    [NPC_KRICK] = "krick",
    [NPC_ICK] = "ick",
    [NPC_TYRANNUS] = "tyrannus",
    [NPC_RIMEFANG] = "rimefang",
    [NPC_TYRANNUS_EVENTS] = "tyrannusEvents",
    [NPC_SYLVANAS_PART1] = "jainaOrSylvanas1",
    [NPC_JAINA_PART1] = "jainaOrSylvanas1",
    -- C++ instance_pit_of_saron.cpp:97-100 binds the part-2 entries
    -- into the SAME _jainaOrSylvanas1GUID slot (a C++ quirk — the slot
    -- is only read by the krick outro, which runs before any part-2
    -- summon exists, so the overwrite is vacuous there too).
    [NPC_SYLVANAS_PART2] = "jainaOrSylvanas1",
    [NPC_JAINA_PART2] = "jainaOrSylvanas1",
}

RegisterMapEvent(MAP_PIT_OF_SARON, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == NPC_CAVERN_EVENT_TRIGGER then
        if instanceData.cavernTriggers == nil then
            instanceData.cavernTriggers = {}
        end
        instanceData.cavernTriggers[#instanceData.cavernTriggers + 1] = creature
        return
    end
    local key = bossBinds[entry]
    if key then
        instanceData[key] = creature
    end
end)

-- C++ instance_pit_of_saron.cpp:62-67 OnPlayerEnter: if (!_teamInInstance)
-- _teamInInstance = player->GetTeam(). The nil check mirrors the C++
-- latch (a second player of the other team cannot flip it).
RegisterMapEvent(MAP_PIT_OF_SARON, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if instanceData.teamInInstance == nil and player ~= nil then
        instanceData.teamInInstance = player:GetTeam()
    end
end)

-- C++ instance_pit_of_saron.cpp:265-276 HandleCavernEventTrigger +
-- pit_of_saron.cpp:48-60 ScheduledIcicleSummons, as a per-trigger
-- CreateLuaEvent chain. First tick at 1000ms (:270, CalculateTime(1s));
-- each tick rolls 12% — on success the trigger casts SPELL_ICICLE_SUMMON
-- triggered on itself and re-schedules {20s,35s}, else re-schedules
-- {1s,20s} (randtime legs, C++ order). A timer whose trigger left the
-- cavernActive state returns without re-arming (the deactivation twin
-- for a chain already in flight).
local cavernTimers = {}

local function scheduleIcicleTick(instanceData, trigger, delay)
    local guid = trigger:GetGUIDLow()
    if cavernTimers[guid] then
        RemoveEventById(cavernTimers[guid])
    end
    cavernTimers[guid] = CreateLuaEvent(function()
        cavernTimers[guid] = nil
        if instanceData.cavernActive ~= 1 then
            return
        end
        local rolled = math.random(100) <= 12
        if rolled then
            trigger:CastSpell(trigger, SPELL_ICICLE_SUMMON, true)
            scheduleIcicleTick(instanceData, trigger, {20000, 35000})
        else
            scheduleIcicleTick(instanceData, trigger, {1000, 20000})
        end
    end, delay)
end

local function handleCavernEventTrigger(instanceData, activate)
    if activate then
        if instanceData.cavernTriggers == nil then
            return
        end
        for _, trigger in ipairs(instanceData.cavernTriggers) do
            scheduleIcicleTick(instanceData, trigger, 1000)
        end
    else
        for guid, id in pairs(cavernTimers) do
            RemoveEventById(id)
            cavernTimers[guid] = nil
        end
    end
end

-- C++ instance_pit_of_saron.cpp:221-235 SetData, via the Go-only
-- InstanceEventOnSetData bridge (event 8). No table write happens in
-- the bridge itself, so every leg stores its own state above.
RegisterMapEvent(MAP_PIT_OF_SARON, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, data)
    if type == DATA_ICE_SHARDS_HIT then
        instanceData.shardsHit = data
    elseif type == DATA_CAVERN_ACTIVE then
        instanceData.cavernActive = data
        if data ~= 0 then
            handleCavernEventTrigger(instanceData, true)
        else
            handleCavernEventTrigger(instanceData, false)
        end
    end
    -- Other types fall through, the C++ default arm.
end)
