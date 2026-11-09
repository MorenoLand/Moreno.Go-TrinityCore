-- The Violet Hold instance script — Lua port of
-- src/server/scripts/Northrend/VioletHold/instance_violet_hold.cpp
-- (class instance_violet_hold : public InstanceMapScript, map 608).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_vault_of_archavon.lua / instance_utgarde_keep.lua
-- precedent); its presence opens the Map:GetInstanceData gate for map
-- 608 (engine/world/instance_hooks.go) that the boss files share with
-- this file.
-- Shared instanceData keys (one table per (map, instance)):
-- xevozz / lavanthor / ichoron / zuramat / erekem / moragg /
-- cyanigosa / sinclari / sinclariTrigger — the nine creatureData
-- binds (instance_violet_hold.cpp:196-208, LoadObjectData).
-- waveCount / eventState / doorIntegrity / defenseless /
-- firstBossId / secondBossId — the instance members below, stored as
-- plain numbers (nil reads as the C++ ctor default: waveCount 0,
-- eventState NOT_STARTED 0, doorIntegrity 100, defenseless true,
-- first/secondBossId 0).
-- defenseless — 1/0 number (instance_violet_hold.cpp:474-475
-- DATA_DEFENSELESS 14, C++ ctor default true): cleared to 0 by the
-- activation-crystal gossip handler
-- (npc_violet_hold_activation_crystal.lua, tick 417); the
-- door-integrity clear (Destroy Door Seal 58040 AuraScript,
-- violet_hold.cpp:1385) stays out (no AuraScript bridge) and the wipe
-- re-arm (instance :893) stays out with CheckEventState below.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the nine boss/Sinclari binds above. Fires on
--   materialization (motionFor funnel, instance_hooks.go:311-320); a
--   static template spawn never materializes server-side and is never
--   bound (naxxramas LoadGrid delta).
-- - GetGuidData (instance_violet_hold.cpp: no LoadObjectData override
--   beyond the base store): the Lua mirror is the instanceData binds
--   above, read as instanceData.<boss> (oculus precedent); the C++
--   consumers ride unported cross-creature machines in the boss files
--   — the Erekem guard legs (boss_erekem.cpp:137-166/:226-241) and the
--   Sinclari-trigger Talks (violet_hold.cpp portal AIs) — so the GUID
--   legs as consumed stay out there. The Erekem-guard GUID slots
--   themselves ride npc_erekem_guard.lua (SetGuidData, tick 485).
-- - SetBossState-override DONE legs (instance_violet_hold.cpp:328-357)
--   — PORTED 2026-11-08 (tick 487) via INSTANCE_EVENT_ON_UNIT_DEATH
--   (9): the Go-only unit-death hook fires inside setDeathState,
--   ahead of the CREATURE_EVENT_ON_DIED handlers
--   (instance_hooks.go:506-525), and the override legs read only
--   WaveCount — never the boss's own state — so firing before the
--   boss died handler's SetBossState(DATA_X, DONE) is order-exact
--   against C++ (_JustDied -> SetBossState -> override):
--   - DATA_MORAGG..DATA_ZURAMAT (3..8) DONE: WaveCount == 6 ->
--     SetBossState(DATA_1ST_BOSS, DONE), WaveCount == 12 ->
--     SetBossState(DATA_2ND_BOSS, DONE), then SetData(DATA_WAVE_COUNT,
--     WaveCount + 1) via the instanceData:SetData bridge below.
--     (The 1ST/2ND_BOSS DONE legs' UpdateEncounterStateForKilled-
--     Creature call stays out — no Go bridge, whole-tree grep.)
--   - DATA_CYANIGOSA (2) DONE -> the MAIN_EVENT_STATE DONE leg
--     below, run from the dying unit (the guaranteed opener) rather
--     than the SetData funnel, so it lands even when Sinclari never
--     bound; the stored eventState is written directly to avoid
--     double-firing the OnSetData DONE arm.
--   - DORMANT START: the alias/wave legs only fire once the event
--     started (waveCount 1..18); with the Sinclari intro unported
--     (gossip menuID not forwarded, see violet_hold.lua) waveCount
--     stays 0 and the legs count boss deaths instead — harmless, and
--     the count self-corrects to 1 the moment an IN_PROGRESS write
--     lands.
-- - SetData switch (instance_violet_hold.cpp:365-461) — PORTED
--   2026-11-08 (tick 487) via the Go-only InstanceEventOnSetData
--   bridge (event 8, instance_hooks.go:43-63; the Eye of Eternity
--   precedent: the writer calls instanceData:SetData(type, data) and
--   this handler runs the switch leg; nested dispatch is safe):
--   - DATA_WAVE_COUNT (10): store waveCount. The AddWave schedule
--     (45s boss wave / 5s trash wave) stays out: AddWave itself is
--     unported (portal AI + MoveSmoothPath intros, below).
--   - DATA_DOOR_INTEGRITY (11): store doorIntegrity, defenseless =
--     0, push WORLD_STATE_VH_PRISON_STATE via bound Sinclari when
--     present. DORMANT: the only writer is the Destroy Door Seal
--     AuraScript (no AuraScript bridge).
--   - DATA_MAIN_EVENT_STATE (9): IN_PROGRESS stores eventState and
--     waveCount = 1, pushes WAVE_COUNT / PRISON_STATE / SHOW via
--     bound Sinclari, clears the crystal NOT_SELECTABLE flags; the
--     AddWave async call and the 3s CheckEventState repeat stay out
--     (AddWave / CheckEventState, below). NOT_STARTED opens the main
--     door, pushes SHOW 0 / WAVE_COUNT / PRISON_STATE, sets the
--     crystal flags. DONE opens the main door, pushes SHOW 0, sets
--     the crystal flags, and fires Sinclari DoAction
--     ACTION_SINCLARI_OUTRO (dormant receiver — the Sinclari AI is
--     unported, so the ev38 fire is a no-op until it lands). The
--     DONE arm has no live writer: the live DONE path is the
--     OnUnitDeath cyanigosa leg above.
--   - DATA_START_BOSS_ENCOUNTER (13) / DATA_HANDLE_CELLS (28): no
--     arms — StartBossEncounter stays out (MoveSmoothPath) and the
--     cell-door legs ride the boss files directly (tick 486:
--     FindNearestGameObject + SetGoState, bypassing the instance).
-- - FillInitialWorldStates (instance_violet_hold.cpp:299-305) —
--   PORTED 2026-11-08 (tick 487) via INSTANCE_EVENT_ON_PLAYER_ENTER
--   (4): SHOW = eventState == IN_PROGRESS and 1 or 0, PRISON_STATE
--   = doorIntegrity, WAVE_COUNT = waveCount, pushed via bound
--   Sinclari (creature:UpdateWorldState broadcasts to the instance;
--   the values are idempotent, so the broadcast re-send to the
--   players already inside is harmless). Nil-guarded: unbound
--   Sinclari skips the push.
-- Unmodeled (documented-only, no bridges):
-- - OnCreatureCreate Erekem-guard slot-fill
--   (instance_violet_hold.cpp:234-247, DATA_EREKEM_GUARD_1/2): the
--   legs live in npc_erekem_guard.lua (SetGuidData, tick 485), not
--   here — the GetGuidData override's only consumers are the Erekem
--   guard legs.
-- - OnGameObjectCreate GO_ACTIVATION_CRYSTAL slot-fill (:266-283):
--   static GOs never pass the Go GO-create funnel (the
--   halls-of-lightning globe-latch precedent); the slots' only
--   consumers are the MAIN_EVENT_STATE crystal-flag legs above,
--   which read the crystals live via GetGameObjectsInGrid instead.
-- - DoorData (gameObjectData :211-222, 9 cell/main doors): no door
--   machine; the cell legs ride the boss files (tick 486) and the
--   main-door legs ride the MAIN_EVENT_STATE arms above.
-- - MinionData (:225-229, NPC_EREKEM_GUARD -> DATA_EREKEM): no
--   minion registry in Go.
-- - CheckRequiredBosses (:313-336, wave-6/12 + First/SecondBossId
--   gates, cyanigosa wave-18 gate): no bridge (the koralon header
--   precedent); FirstBossId/SecondBossId are picked by the unported
--   AddWave machine.
-- - Update (:841-860, no-players -> FAIL + CheckEventState, door
--   integrity 0 -> FAIL) / CheckEventState (:862-881,
--   ResetBossEncounter + SetData MAIN_EVENT_STATE NOT_STARTED) /
--   ResetBossEncounter (:695-739, respawn + UpdateEntry dummy swap +
--   MoveTargetedHome + SetImmuneToAll): no OnUpdate tick fires for a
--   player-less instance in Go (updateInstanceScriptTicks only ticks
--   populated instances), so the no-players leg is unreachable; the
--   3s CheckEventState poll has no driver; ResetBossEncounter needs
--   MoveTargetedHome (unbridged — Respawn / UpdateEntry /
--   SetImmuneToAll / DespawnOrUnsummon / Sinclari EnterEvadeMode
--   are bridged, but the homeward leg is not, so the reset is not
--   portable as a whole).
-- - StartBossEncounter (:625-693) / AddWave (:741-781) /
--   ScheduleCyanigosaIntro (:883-905) / SpawnPortal (:560-578):
--   MoveSmoothPath is unbridged and the portal/saboteur/Sinclari
--   AIs in violet_hold.cpp are unported (the portal AI has no Lua
--   file, so summoned portals would be inert); MoveJump /
--   creature:SummonCreature / instance SetData ARE bridged, but the
--   machines they would feed have no live trigger.
-- - ProcessEvent (:928-936, EVENT_ACTIVATE_CRYSTAL 20001 -> summon
--   defense system): no instance-script bridge — but the trigger is
--   the crystal gossip (spell 57804's SEND_EVENT effect has no C++
--   raiser in-tree and no Go twin), so the summon rides the gossip
--   handler directly (npc_violet_hold_activation_crystal.lua, tick
--   487: go:SummonCreature). The Defenseless=false leg already rode
--   that handler (tick 417).
-- - SetHeaders(DataHeader "VH") / SetBossNumber(EncounterCount 9) /
--   LoadObjectData / LoadMinionData: no bridges; the boss-state store
--   is written via creature:SetBossState
--   (lua_creature_events.go:6419) and read via creature:GetBossState
--   (lua_creature_events.go:6438).
-- - SaveToDB / WriteSaveDataMore / ReadSaveDataMore (FirstBossId /
--   SecondBossId): the table lives for the runtime lifetime only
--   (black-morass delta).
-- No ON_UPDATE poll: the 3s CheckEventState repeat and the AddWave
-- schedule have no driver (see above); the OnUpdate event (3) is
-- deliberately unbound so a player-less instance costs no ticks.

local MAP_VIOLET_HOLD = 608
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_SET_DATA = 8
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local NPC_XEVOZZ = 29266 -- violet_hold.h:96
local NPC_LAVANTHOR = 29312 -- violet_hold.h:97
local NPC_ICHORON = 29313 -- violet_hold.h:98
local NPC_ZURAMAT = 29314 -- violet_hold.h:101
local NPC_EREKEM = 29315 -- violet_hold.h:104
local NPC_MORAGG = 29316 -- violet_hold.h:106
local NPC_CYANIGOSA = 31134 -- violet_hold.h:116
local NPC_SINCLARI = 30658 -- violet_hold.h:118
local NPC_SINCLARI_TRIGGER = 32204 -- violet_hold.h:119

-- violet_hold.h VHData enum (DATA_1ST_BOSS 0 .. DATA_HANDLE_CELLS 28).
local DATA_1ST_BOSS = 0
local DATA_2ND_BOSS = 1
local DATA_CYANIGOSA = 2
local DATA_MORAGG = 3
local DATA_EREKEM = 4
local DATA_ICHORON = 5
local DATA_LAVANTHOR = 6
local DATA_XEVOZZ = 7
local DATA_ZURAMAT = 8
local DATA_MAIN_EVENT_STATE = 9
local DATA_WAVE_COUNT = 10
local DATA_DOOR_INTEGRITY = 11
local DATA_START_BOSS_ENCOUNTER = 13
local DATA_DEFENSELESS = 14

-- EncounterState (InstanceScript.h:68-76).
local ENCOUNTER_NOT_STARTED = 0
local ENCOUNTER_IN_PROGRESS = 1
local ENCOUNTER_DONE = 3

local GO_MAIN_DOOR = 191723 -- violet_hold.h:125
local GO_ACTIVATION_CRYSTAL = 193611 -- violet_hold.h:134

local WORLD_STATE_VH_SHOW = 3816 -- violet_hold.h:140
local WORLD_STATE_VH_PRISON_STATE = 3815 -- violet_hold.h:141
local WORLD_STATE_VH_WAVE_COUNT = 3810 -- violet_hold.h:142

local GAMEOBJECT_FLAGS = 9 -- arlokk precedent: OBJECT_END 6 + 3
local GO_FLAG_LOCKED = 2 -- SharedDefines.h:1638
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641

local ACTION_SINCLARI_OUTRO = 1 -- violet_hold.h:152

local BOSS_ENTRIES = {
    [NPC_MORAGG] = DATA_MORAGG,
    [NPC_EREKEM] = DATA_EREKEM,
    [NPC_ICHORON] = DATA_ICHORON,
    [NPC_LAVANTHOR] = DATA_LAVANTHOR,
    [NPC_XEVOZZ] = DATA_XEVOZZ,
    [NPC_ZURAMAT] = DATA_ZURAMAT,
}

local bossBinds = {
    [NPC_XEVOZZ] = "xevozz",
    [NPC_LAVANTHOR] = "lavanthor",
    [NPC_ICHORON] = "ichoron",
    [NPC_ZURAMAT] = "zuramat",
    [NPC_EREKEM] = "erekem",
    [NPC_MORAGG] = "moragg",
    [NPC_CYANIGOSA] = "cyanigosa",
    [NPC_SINCLARI] = "sinclari",
    [NPC_SINCLARI_TRIGGER] = "sinclariTrigger",
}

-- The opener is a bound creature in the instance (Sinclari for the
-- SetData arms, the dying boss for the OnUnitDeath arm); a nil
-- opener skips the world-object legs but the stored state still
-- lands. Ranges cover the room from its centre (the hall spans
-- roughly x 1858-1936 / y 753-850); only one main door and five
-- crystals exist in the instance, so a wide range cannot pick a
-- wrong object.
local function updateWorld(opener, field, value)
    if opener then
        opener:UpdateWorldState(field, value)
    end
end

local function openMainDoor(opener)
    if not opener then
        return
    end
    local door = opener:FindNearestGameObject(GO_MAIN_DOOR, 150)
    if door then
        door:SetGoState(0) -- GO_STATE_ACTIVE
        door:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_LOCKED)
    end
end

local function setCrystals(opener, selectable)
    if not opener then
        return
    end
    for _, crystal in ipairs(opener:GetGameObjectsInGrid(GO_ACTIVATION_CRYSTAL, 150)) do
        if selectable then
            crystal:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        else
            crystal:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
    end
end

-- C++ SetData DATA_MAIN_EVENT_STATE DONE
-- (instance_violet_hold.cpp:436-453): main door open, SHOW 0,
-- crystals NOT_SELECTABLE, Sinclari DoAction OUTRO.
local function mainEventStateDone(instanceData, opener)
    openMainDoor(opener)
    updateWorld(opener, WORLD_STATE_VH_SHOW, 0)
    setCrystals(opener, false)
    local sinclari = instanceData.sinclari
    if sinclari then
        sinclari:DoAction(ACTION_SINCLARI_OUTRO)
    end
end

-- C++ SetData DATA_MAIN_EVENT_STATE NOT_STARTED
-- (instance_violet_hold.cpp:418-434): main door open, SHOW 0,
-- WAVE_COUNT / PRISON_STATE, crystals NOT_SELECTABLE.
local function mainEventStateNotStarted(instanceData, opener)
    openMainDoor(opener)
    updateWorld(opener, WORLD_STATE_VH_SHOW, 0)
    updateWorld(opener, WORLD_STATE_VH_WAVE_COUNT, instanceData.waveCount or 0)
    updateWorld(opener, WORLD_STATE_VH_PRISON_STATE, instanceData.doorIntegrity or 100)
    setCrystals(opener, false)
end

-- C++ SetData DATA_MAIN_EVENT_STATE IN_PROGRESS
-- (instance_violet_hold.cpp:395-416): WAVE_COUNT / PRISON_STATE /
-- SHOW pushes (of the pre-start values — WaveCount is still 0 here),
-- then WaveCount = 1, crystals selectable. The AddWave async call
-- and the 3s CheckEventState repeat stay out (see header).
local function mainEventStateInProgress(instanceData, opener)
    updateWorld(opener, WORLD_STATE_VH_WAVE_COUNT, instanceData.waveCount or 0)
    updateWorld(opener, WORLD_STATE_VH_PRISON_STATE, instanceData.doorIntegrity or 100)
    updateWorld(opener, WORLD_STATE_VH_SHOW, 1)
    instanceData.waveCount = 1
    setCrystals(opener, true)
end

RegisterMapEvent(MAP_VIOLET_HOLD, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = bossBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
end)

-- C++ instance_violet_hold::SetData (instance_violet_hold.cpp:365-461),
-- via the Go-only InstanceEventOnSetData bridge (event 8). The
-- writer calls instanceData:SetData(type, data); this handler runs
-- the switch leg. No table write happens in the bridge itself, so
-- every leg stores its own state below.
RegisterMapEvent(MAP_VIOLET_HOLD, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, data)
    if type == DATA_WAVE_COUNT then
        instanceData.waveCount = data
        -- The AddWave schedule (45s boss wave / 5s trash wave,
        -- instance_violet_hold.cpp:373-380) stays out: AddWave is
        -- unported (see header).
    elseif type == DATA_DOOR_INTEGRITY then
        instanceData.doorIntegrity = data
        instanceData.defenseless = 0
        updateWorld(instanceData.sinclari, WORLD_STATE_VH_PRISON_STATE, data)
    elseif type == DATA_MAIN_EVENT_STATE then
        instanceData.eventState = data
        if data == ENCOUNTER_IN_PROGRESS then
            mainEventStateInProgress(instanceData, instanceData.sinclari)
        elseif data == ENCOUNTER_NOT_STARTED then
            mainEventStateNotStarted(instanceData, instanceData.sinclari)
        elseif data == ENCOUNTER_DONE then
            -- No live writer: the live DONE path is the OnUnitDeath
            -- cyanigosa leg below, which passes the dying unit as
            -- the opener. This arm runs only if a future writer
            -- calls SetData(MAIN_EVENT_STATE, DONE).
            mainEventStateDone(instanceData, instanceData.sinclari)
        end
    end
    -- DATA_START_BOSS_ENCOUNTER (13): no arm — StartBossEncounter
    -- stays out (MoveSmoothPath, see header). DATA_HANDLE_CELLS
    -- (28): no arm — the cell-door legs ride the boss files (tick
    -- 486). Other types fall through, the C++ default arm.
end)

-- C++ instance_violet_hold::SetBossState override DONE legs
-- (instance_violet_hold.cpp:328-357), via the Go-only
-- INSTANCE_EVENT_ON_UNIT_DEATH (9). Players carry no GetEntry, so
-- the type check runs before the entry read (the naxxramas
-- precedent); trash, portals and guards fall through untouched.
RegisterMapEvent(MAP_VIOLET_HOLD, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil or unit:GetObjectType() == "Player" then
        return
    end
    local entry = unit:GetEntry()
    if BOSS_ENTRIES[entry] then
        local waveCount = instanceData.waveCount or 0
        if waveCount == 6 then
            unit:SetBossState(DATA_1ST_BOSS, ENCOUNTER_DONE)
        elseif waveCount == 12 then
            unit:SetBossState(DATA_2ND_BOSS, ENCOUNTER_DONE)
        end
        -- Fires the OnSetData WAVE_COUNT arm above, which stores the
        -- count; the AddWave schedule leg stays out (see header).
        instanceData:SetData(DATA_WAVE_COUNT, waveCount + 1)
        return
    end
    if entry == NPC_CYANIGOSA then
        -- The SetBossState DATA_CYANIGOSA DONE -> SetData
        -- MAIN_EVENT_STATE DONE leg, run from the dying unit: the
        -- stored state is written directly (no SetData call) so the
        -- OnSetData DONE arm above does not double-fire.
        instanceData.eventState = ENCOUNTER_DONE
        mainEventStateDone(instanceData, unit)
    end
end)

-- C++ FillInitialWorldStates (instance_violet_hold.cpp:299-305),
-- via INSTANCE_EVENT_ON_PLAYER_ENTER (4). Nil-guarded on the
-- Sinclari bind; the broadcast re-send to players already inside
-- carries the same values.
RegisterMapEvent(MAP_VIOLET_HOLD, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    local sinclari = instanceData.sinclari
    if not sinclari then
        return
    end
    sinclari:UpdateWorldState(WORLD_STATE_VH_SHOW, (instanceData.eventState or 0) == ENCOUNTER_IN_PROGRESS and 1 or 0)
    sinclari:UpdateWorldState(WORLD_STATE_VH_PRISON_STATE, instanceData.doorIntegrity or 100)
    sinclari:UpdateWorldState(WORLD_STATE_VH_WAVE_COUNT, instanceData.waveCount or 0)
end)
