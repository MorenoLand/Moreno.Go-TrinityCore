-- Culling of Stratholme instance script — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- instance_culling_of_stratholme.cpp (864 lines; class
-- instance_culling_of_stratholme : public InstanceMapScript, map 595 —
-- instance_culling_of_stratholme.cpp:24; AddSC at end registers it;
-- kalimdor loader decl/call per kalimdor_script_loader.cpp, in the
-- "// Caverns of Time" group). DataHeader "CS", EncounterCount = 5
-- (culling_of_stratholme.h:20/22); DATA ids / progress states / actions
-- / entries in culling_of_stratholme.h:104-176.
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_the_black_morass.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 595
-- (engine/world/instance_hooks.go) that the CoS boss files share.
-- instanceData holds ONLY scalars (it is marshalled by
-- Map:SaveInstanceData — engine/scripting/instance_data.go rejects
-- userdata, so creature/GO binds must never land here; the BM file
-- stores medivh in instanceData and never saves, this file saves):
-- progress (COSProgressStates bitmask, nil reads as JUST_STARTED),
-- guardianTimeout (unix seconds, 0 = none), arthasDied (transient flag
-- set by npc_arthas_stratholme.lua onDied, consumed by the ON_UPDATE
-- pump), plus the transient one-shot arthas RP flags (utherFinished,
-- startWaves, reachTownHall, townHallDone, gauntletReached, gauntletDone,
-- malganisDone, arthasGone; gossip flags startPurge, startTownHall,
-- toGauntlet, startGauntlet, startMalganis), all consumed by the
-- ON_UPDATE pump below. Per-instance runtime state (binds, wave spawns, event queue,
-- sent world states, crate reveals) lives in the file-local `per`
-- table, weak-keyed by the instanceData table object itself — the
-- runtime keeps Lua object identity stable across hooks
-- (instance_hooks.go instanceDataObject), so the key is stable for the
-- instance lifetime and never collides across concurrent instances.
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: chromie 26527 / corruptor 32273 / guardian 32281
--   / bunny 28960 / crate helpers 27827 (list) / arthas 26499 / crier
--   27913 binds (instance_culling_of_stratholme.cpp:598-631). The
--   setActive(true) legs have no bridge (no SetActive on the creature
--   surface) — binds are still live; grid sweep materializes the
--   static spawns near players (creaturemotion.go:4953).
-- - OnGameObjectCreate: hidden passage 188686 bind + GoState by current
--   progress (:633-643; GO_STATE_READY 0 / ACTIVE 1).
-- - OnLoad (2): restores instanceData.progress through
--   GetStableStateFor + force transition, and re-arms the guardian
--   tick when a guardian timeout is saved (:344-366).
-- - OnUpdate (3): the C++ EventMap pump (events.Update(diff) +
--   ExecuteEvent loop) with diff in ms — guardian tick, respawn
--   arthas, crier call-to-gates, scourge wave, crier announce-wave
--   (:465-594) — plus the arthas RP flag consumption below (SetData
--   guards + InitiateArthasEvent legs).
-- - SetInstanceProgress(state, force) (:649-792): spawn-group legs stay
--   out (no SpawnGroupSpawn/Despawn bridge — engine-wide grep); the
--   arthas leg issues DoAction(-ACTION_PROGRESS_UPDATE) (ev38 dispatch
--   into npc_arthas_stratholme.lua's RP machine) or despawns /
--   respawns him 1s later at the snapback position
--   (npc_arthas.cpp:521-547 ArthasSnapbackPositions); world states per
--   :730-752; CRATES_DONE bunny credit cast (58109, triggered) + 5s
--   crier timer; WAVES_IN_PROGRESS wave reset + SpawnInfiniteCorruptor;
--   the force leg resets the event queue, re-arms the arthas respawn
--   and re-runs SpawnInfiniteCorruptor, then SaveInstanceData (the
--   SaveToDB twin — engine/scripting/globals.go:1447). The force leg's
--   DeleteRespawnTimes + dead/temp-sweep over the instance object store
--   has no bridge (no map object enumeration).
-- - GetStableStateFor (:193-224) as a lookup table.
-- - SetData legs with Lua writers, as second handlers in this file
--   (hooks append — runtime.go registerHook — so these coexist with the
--   first handlers in northrend/culling_of_stratholme.lua):
--   - DATA_CRATES_START (chromie start gossip EXPLAIN_2 action 1004,
--     culling_of_stratholme.cpp:407-411 AdvanceDungeon leg, gated on
--     JUST_STARTED; the northrend handler owns the menu + AddItem legs)
--     and DATA_SKIP_TO_PURGE (SKIP_1 action 1005, :398-401
--     AdvanceDungeonFar leg, gated on progress <= CRATES_DONE).
--   - DATA_CRATE_REVEALED (crate helper ev14 spell 49590,
--     culling_of_stratholme.cpp:1410-1428 SetData leg + the
--     MissingPlagueCrates count — the northrend handler owns the fluff
--     relay; this handler keeps its own revealed set and drives the
--     CRATES_REVEALED world state / CRATES_DONE transition).
--   - DATA_NOTIFY_DEATH / OnUnitDeath (:431-459): ev4 on the seven
--     wave-mob entries + meathook 26529 / salramm 26530 (second ev4
--     handlers; the boss files own Talk/SetBossState), membership in
--     the wave-spawn set, marker clear + next-wave schedule (20s after
--     the meathook wave, 1s otherwise) or WAVES_DONE.
-- - SpawnInfiniteCorruptor (:780-790): heroic-only, guardianTimeout == 0,
--   corruptor boss state neither DONE nor FAIL — summons rift 28409 /
--   guardian 32281 / corruptor 32273 at the C++ positions, sets the
--   25-minute timeout, schedules the first guardian tick at 6s.
-- - SetBossState DATA_INFINITE_CORRUPTOR DONE override (:452-459):
--   the cancel-tick + hide-world-states half runs as a guard at the
--   top of the guardian-tick handler (boss files call
--   creature:SetBossState directly, bypassing any override).
-- - Summoner: wave/arthas/corruptor summons ride a bound creature
--   (crier > chromie > arthas > bunny > any live wave spawn), falling
--   back to the first player in the instance (player:SummonCreature
--   twin, lua_player.go:6696) — C++ instance->SummonCreature has no
--   summoner, and no map-level summon bridge exists.
-- Unmodeled (documented-only, no bridges / no writers):
-- - SetData DATA_GM_OVERRIDE / DATA_GM_RECALL (chromie GM menu,
--   unported) and DATA_UTHER_START (chromie middle, unregistered AI)
--   still have no writers. The arthas RP SetData legs
--   (DATA_UTHER_FINISHED / DATA_START_WAVES / DATA_REACH_TOWN_HALL /
--   DATA_TOWN_HALL_DONE / DATA_GAUNTLET_REACHED / DATA_GAUNTLET_DONE /
--   DATA_MALGANIS_DONE) ARE ported: npc_arthas_stratholme.lua writes
--   one-shot instanceData flags and the ON_UPDATE pump below consumes
--   them with the exact SetData guards; the SetGuidData
--   InitiateArthasEvent legs are ported the same way (gossip flags +
--   arthas:DoAction(-ACTION_START_RP_EVENTn) — the SetGUID legs arrive
--   as negative DoAction values, since no SetGUID Lua dispatch exists).
-- - DATA_ARTHAS_DIED: PORTED via the arthasDied flag — arthas onDied in
--   npc_arthas_stratholme.lua sets instanceData.arthasDied and the pump
--   regresses to the stable state with force (the _arthasGUID clear is
--   the per-instance arthas bind drop below).
-- - OnPlayerEnter world-state send (FillInitialWorldStates): no
--   per-player world-state bridge exists (creature:UpdateWorldState
--   broadcasts to the whole instance) — the BM delta, standing.
-- - SetHeaders(DataHeader "CS") / EncounterCount 5 / doorData
--   (GO_MALGANIS_GATE_2 187723 / GO_EXIT_GATE 191788): no bridges.

local MAP_CULLING_OF_STRATHOLME = 595
local INSTANCE_EVENT_ON_LOAD = 2
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_GAMEOBJECT_CREATE = 6

-- COSProgressStates (culling_of_stratholme.h:104-122).
local JUST_STARTED = 0x00001
local CRATES_IN_PROGRESS = 0x00002
local CRATES_DONE = 0x00004
local UTHER_TALK = 0x00008
local PURGE_PENDING = 0x00010
local PURGE_STARTING = 0x00020
local WAVES_IN_PROGRESS = 0x00040
local WAVES_DONE = 0x00080
local TOWN_HALL_PENDING = 0x00100
local TOWN_HALL = 0x00200
local TOWN_HALL_COMPLETE = 0x00400
local GAUNTLET_TRANSITION = 0x00800
local GAUNTLET_PENDING = 0x01000
local GAUNTLET_IN_PROGRESS = 0x02000
local GAUNTLET_COMPLETE = 0x04000
local MALGANIS_IN_PROGRESS = 0x08000
local COMPLETE = 0x10000

-- DATA_* boss ids (culling_of_stratholme.h:124-130).
local DATA_INFINITE_CORRUPTOR = 4
local BOSS_STATE_DONE = 3
local BOSS_STATE_FAIL = 2

-- COSWorldStates (instance_culling_of_stratholme.cpp:97-108).
local WORLDSTATE_SHOW_CRATES = 3479
local WORLDSTATE_CRATES_REVEALED = 3480
local WORLDSTATE_WAVE_COUNT = 3504
local WORLDSTATE_WAVE_MARKER_ES = 3581
local WORLDSTATE_WAVE_MARKER_FL = 3582
local WORLDSTATE_WAVE_MARKER_KS = 3583
local WORLDSTATE_WAVE_MARKER_MR = 3584
local WORLDSTATE_WAVE_MARKER_TH = 3585
local WORLDSTATE_TIME_GUARDIAN = 3931
local WORLDSTATE_TIME_GUARDIAN_SHOW = 3932

local NPC_CHROMIE = 26527
local NPC_INFINITE_CORRUPTOR = 32273
local NPC_GUARDIAN_OF_TIME = 32281
local NPC_TIME_RIFT = 28409
local NPC_LORDAERON_CRIER = 27913
local NPC_DEVOURING_GHOUL = 28249
local NPC_ENRAGED_GHOUL = 27729
local NPC_NECROMANCER = 28200
local NPC_CRYPT_FIEND = 27734
local NPC_ACOLYTE = 27731
local NPC_CRYPT_STALKER = 28199
local NPC_ABOMINATION = 27736
local NPC_MEATHOOK = 26529
local NPC_SALRAMM = 26530
local NPC_GENERIC_BUNNY = 28960
local NPC_CRATE_HELPER = 27827
local NPC_ARTHAS = 26499

local GO_HIDDEN_PASSAGE = 188686
local GO_STATE_READY = 0
local GO_STATE_ACTIVE = 1

local SPELL_CRATES_KILL_CREDIT = 58109
local SPELL_ARCANE_DISRUPTION = 49590

local NUM_PLAGUE_CRATES = 5
local NUM_SCOURGE_WAVES = 10
local MAX_SPAWNS_PER_WAVE = 6
local WAVE_MEATHOOK = 5
local WAVE_SALRAMM = 10

local EVENT_GUARDIAN_TICK = 1
local EVENT_RESPAWN_ARTHAS = 2
local EVENT_CRIER_CALL_TO_GATES = 3
local EVENT_SCOURGE_WAVE = 4
local EVENT_CRIER_ANNOUNCE_WAVE = 5

-- Crier Talk lines (instance_culling_of_stratholme.cpp:84-89); the
-- announce-wave leg Talks _currentSpawnLoc itself (1..5).
local CRIER_SAY_CALL_TO_GATES = 0
local CHROMIE_WHISPER_GUARDIAN_1 = 0
local CHROMIE_WHISPER_GUARDIAN_2 = 1
local CHROMIE_WHISPER_GUARDIAN_3 = 2

local ACTION_PROGRESS_UPDATE = 1
local ACTION_CORRUPTOR_LEAVE = 2
local ACTION_START_RP_EVENT2 = 4
local ACTION_START_RP_EVENT3 = 5
local ACTION_START_RP_EVENT4_1 = 6
local ACTION_START_RP_EVENT4_2 = 7
local ACTION_START_RP_EVENT5 = 8

local SUMMON_MANUAL_DESPAWN = 7

local CorruptorPos = { 2331.642, 1273.273, 132.9524, 3.717551 }
local GuardianPos = { 2321.489, 1268.383, 132.8507, 0.418879 }
local CorruptorRiftPos = { 2443.626, 1280.450, 133.0066, 1.727876 }

-- HeroicWaves[wave] (instance_culling_of_stratholme.cpp:166-179).
local HeroicWaves = {
    { NPC_DEVOURING_GHOUL, NPC_DEVOURING_GHOUL, NPC_DEVOURING_GHOUL },
    { NPC_DEVOURING_GHOUL, NPC_ENRAGED_GHOUL, NPC_NECROMANCER },
    { NPC_DEVOURING_GHOUL, NPC_ENRAGED_GHOUL, NPC_NECROMANCER, NPC_CRYPT_FIEND },
    { NPC_NECROMANCER, NPC_CRYPT_FIEND, NPC_ACOLYTE, NPC_ACOLYTE, NPC_ACOLYTE, NPC_ACOLYTE },
    { 0 },
    { NPC_DEVOURING_GHOUL, NPC_NECROMANCER, NPC_CRYPT_FIEND, NPC_CRYPT_STALKER },
    { NPC_DEVOURING_GHOUL, NPC_ENRAGED_GHOUL, NPC_ENRAGED_GHOUL, NPC_ABOMINATION },
    { NPC_DEVOURING_GHOUL, NPC_ENRAGED_GHOUL, NPC_NECROMANCER, NPC_ABOMINATION },
    { NPC_DEVOURING_GHOUL, NPC_NECROMANCER, NPC_CRYPT_FIEND, NPC_ABOMINATION },
    { 0 },
}

-- WaveLocations[spawnLoc] (instance_culling_of_stratholme.cpp:192-283):
-- 1 King's Square, 2 Market Row, 3 Festival Lane, 4 Elders' Square,
-- 5 Town Hall.
local WaveLocations = {
    { marker = WORLDSTATE_WAVE_MARKER_KS, points = {
        { 2131.474, 1352.615, 131.372, 6.10960 },
        { 2131.463, 1357.127, 131.587, 5.95173 },
        { 2129.795, 1345.093, 131.194, 0.17905 },
        { 2136.235, 1347.894, 131.628, 0.20262 },
        { 2138.219, 1356.240, 132.169, 5.95173 },
        { 2140.584, 1351.624, 132.142, 6.08525 },
    } },
    { marker = WORLDSTATE_WAVE_MARKER_MR, points = {
        { 2226.364, 1331.808, 127.0193, 3.298672 },
        { 2229.934, 1329.146, 127.057, 3.24605 },
        { 2225.028, 1327.269, 127.791, 3.03792 },
        { 2223.844, 1335.282, 127.749, 3.47774 },
        { 2222.192, 1330.859, 127.526, 3.18793 },
        { 2225.865, 1331.029, 127.007, 3.18793 },
    } },
    { marker = WORLDSTATE_WAVE_MARKER_FL, points = {
        { 2183.596, 1238.823, 136.551, 2.16377 },
        { 2181.420, 1237.357, 136.565, 2.16377 },
        { 2178.692, 1237.446, 136.694, 1.99098 },
        { 2184.980, 1242.458, 136.772, 2.59181 },
        { 2176.873, 1240.463, 136.420, 2.10094 },
        { 2181.523, 1244.298, 136.338, 2.38997 },
    } },
    { marker = WORLDSTATE_WAVE_MARKER_ES, points = {
        { 2267.003, 1168.055, 137.821, 2.79050 },
        { 2264.392, 1162.145, 137.910, 2.39937 },
        { 2262.785, 1166.648, 138.053, 2.71353 },
        { 2265.214, 1170.771, 137.972, 2.80385 },
        { 2259.745, 1159.360, 138.198, 2.34047 },
        { 2264.222, 1171.708, 138.047, 2.82742 },
    } },
    { marker = WORLDSTATE_WAVE_MARKER_TH, points = {
        { 2351.656, 1218.682, 130.062, 4.63383 },
        { 2354.921, 1218.425, 130.280, 4.63383 },
        { 2347.516, 1216.976, 130.491, 5.02496 },
        { 2356.508, 1216.656, 130.445, 4.29061 },
        { 2346.674, 1216.739, 130.576, 5.32341 },
        { 2351.728, 1214.561, 130.255, 4.61891 },
    } },
}

-- ArthasSnapbackPositions (npc_arthas.cpp:521-547): position only — the
-- ReactState / HasGossip legs ride the unported AdvanceToState machine.
local ArthasSnapback = {
    [JUST_STARTED] = { 1983.857, 1287.043, 145.5596, 3.0892330 },
    [CRATES_IN_PROGRESS] = { 1983.857, 1287.043, 145.5596, 3.0892330 },
    [CRATES_DONE] = { 1983.857, 1287.043, 145.5596, 3.0892330 },
    [UTHER_TALK] = { 1983.857, 1287.043, 145.5596, 3.0892330 },
    [PURGE_PENDING] = { 2047.948, 1287.598, 142.8568, 3.176499 },
    [PURGE_STARTING] = { 2047.948, 1287.598, 142.8568, 3.176499 },
    [WAVES_IN_PROGRESS] = { 2091.994, 1277.257, 140.4707, 1.134464 },
    [WAVES_DONE] = { 2091.994, 1277.257, 140.4707, 1.134464 },
    [TOWN_HALL_PENDING] = { 2366.240, 1195.253, 132.0441, 3.159046 },
    [TOWN_HALL] = { 2366.240, 1195.253, 132.0441, 3.159046 },
    [TOWN_HALL_COMPLETE] = { 2425.898, 1118.842, 148.0759, 6.073746 },
    [GAUNTLET_TRANSITION] = { 2425.898, 1118.842, 148.0759, 6.073746 },
    [GAUNTLET_PENDING] = { 2534.988, 1126.163, 130.8621, 2.844887 },
    [GAUNTLET_IN_PROGRESS] = { 2534.988, 1126.163, 130.8621, 2.844887 },
    [GAUNTLET_COMPLETE] = { 2363.440, 1404.906, 128.7869, 2.775074 },
    [MALGANIS_IN_PROGRESS] = { 2363.440, 1404.906, 128.7869, 2.775074 },
    [COMPLETE] = { 2301.055, 1478.977, 128.1299, 1.758816 },
}

-- GetStableStateFor (instance_culling_of_stratholme.cpp:193-224).
local StableStateFor = {
    [JUST_STARTED] = JUST_STARTED,
    [CRATES_IN_PROGRESS] = CRATES_IN_PROGRESS,
    [CRATES_DONE] = CRATES_DONE,
    [UTHER_TALK] = PURGE_PENDING,
    [PURGE_PENDING] = PURGE_PENDING,
    [PURGE_STARTING] = PURGE_PENDING,
    [WAVES_IN_PROGRESS] = PURGE_PENDING,
    [WAVES_DONE] = TOWN_HALL_PENDING,
    [TOWN_HALL_PENDING] = TOWN_HALL_PENDING,
    [TOWN_HALL] = TOWN_HALL_PENDING,
    [TOWN_HALL_COMPLETE] = GAUNTLET_PENDING,
    [GAUNTLET_TRANSITION] = GAUNTLET_PENDING,
    [GAUNTLET_PENDING] = GAUNTLET_PENDING,
    [GAUNTLET_IN_PROGRESS] = GAUNTLET_PENDING,
    [GAUNTLET_COMPLETE] = GAUNTLET_COMPLETE,
    [MALGANIS_IN_PROGRESS] = GAUNTLET_COMPLETE,
    [COMPLETE] = COMPLETE,
}

local WaveMarkers = {
    WORLDSTATE_WAVE_MARKER_ES, WORLDSTATE_WAVE_MARKER_FL,
    WORLDSTATE_WAVE_MARKER_KS, WORLDSTATE_WAVE_MARKER_MR,
    WORLDSTATE_WAVE_MARKER_TH,
}

-- Per-instance runtime state, weak-keyed by the instanceData table (see
-- header). st.events is the C++ EventMap: {id, remainingMs} list.
local per = setmetatable({}, { __mode = "k" })

local function stFor(instanceData)
    local st = per[instanceData]
    if not st then
        st = {
            events = {},
            sentWorldStates = {},
            waveSpawns = {},
            crateHelpers = {},
            revealedCrates = {},
            waveCount = 0,
            currentSpawnLoc = 0,
        }
        per[instanceData] = st
    end
    return st
end

local function progressOf(instanceData)
    return instanceData.progress or JUST_STARTED
end

local function scheduleEvent(st, id, ms)
    for _, ev in ipairs(st.events) do
        if ev.id == id then
            ev.remainingMs = ms
            return
        end
    end
    st.events[#st.events + 1] = { id = id, remainingMs = ms }
end

local function cancelEvent(st, id)
    for i, ev in ipairs(st.events) do
        if ev.id == id then
            table.remove(st.events, i)
            return
        end
    end
end

-- SetWorldState + PropagateWorldStateUpdate, merged: only changed values
-- are sent (the _sentWorldStates diff, :793-826), via any bound
-- creature (UpdateWorldState broadcasts instance-wide).
local function senderOf(st)
    if st.crier and st.crier:IsAlive() then
        return st.crier
    end
    if st.chromie and st.chromie:IsAlive() then
        return st.chromie
    end
    if st.arthas and st.arthas:IsAlive() then
        return st.arthas
    end
    if st.bunny and st.bunny:IsAlive() then
        return st.bunny
    end
    for _, spawn in pairs(st.waveSpawns) do
        if spawn and spawn:IsAlive() then
            return spawn
        end
    end
    return nil
end

local function setWorldState(st, id, value)
    if st.sentWorldStates[id] == value then
        return
    end
    st.sentWorldStates[id] = value
    local sender = senderOf(st)
    if sender then
        sender:UpdateWorldState(id, value)
    end
end

-- SummonCreature twin: C++ instance->SummonCreature has no summoner; in
-- Go the summon rides a bound creature, else the first player in the
-- instance (player:SummonCreature, lua_player.go:6696).
local function summonAt(st, mapObj, entry, pos)
    local summoner = senderOf(st)
    if summoner then
        return summoner:SummonCreature(entry, pos[1], pos[2], pos[3], pos[4],
            SUMMON_MANUAL_DESPAWN, 0)
    end
    if mapObj then
        local players = mapObj:GetPlayers()
        if players and players[1] then
            return players[1]:SummonCreature(entry, pos[1], pos[2], pos[3],
                pos[4], SUMMON_MANUAL_DESPAWN, 0)
        end
    end
    return nil
end

local function bossStateOf(st, bossId)
    local sender = senderOf(st)
    if sender then
        return sender:GetBossState(bossId)
    end
    return nil
end

local function setBossState(st, bossId, state)
    local sender = senderOf(st)
    if sender then
        sender:SetBossState(bossId, state)
    end
end

local setProgress -- forward: event handlers below call it

local function spawnInfiniteCorruptor(instanceData, mapObj, st)
    if (instanceData.guardianTimeout or 0) ~= 0 then
        return
    end
    if not mapObj or not mapObj:IsHeroic() then
        return
    end
    local bossState = bossStateOf(st, DATA_INFINITE_CORRUPTOR)
    if bossState == BOSS_STATE_DONE or bossState == BOSS_STATE_FAIL then
        return
    end
    summonAt(st, mapObj, NPC_TIME_RIFT, CorruptorRiftPos)
    summonAt(st, mapObj, NPC_GUARDIAN_OF_TIME, GuardianPos)
    summonAt(st, mapObj, NPC_INFINITE_CORRUPTOR, CorruptorPos)
    instanceData.guardianTimeout = os.time() + 25 * 60
    scheduleEvent(st, EVENT_GUARDIAN_TICK, 6000)
end

-- SetInstanceProgress (instance_culling_of_stratholme.cpp:649-792).
setProgress = function(instanceData, mapObj, state, force)
    local st = stFor(instanceData)
    instanceData.progress = state

    -- Spawn-group legs stay out (no bridge); the arthas leg below is the
    -- live half of the spawn management.
    if state > CRATES_DONE then
        local arthas = st.arthas
        if arthas then
            if force then
                arthas:DespawnOrUnsummon()
                st.arthas = nil
            else
                arthas:DoAction(-ACTION_PROGRESS_UPDATE)
            end
        end
        if not st.arthas then
            scheduleEvent(st, EVENT_RESPAWN_ARTHAS, 1000)
        end
    elseif st.arthas then
        st.arthas:DespawnOrUnsummon()
        st.arthas = nil
    end

    -- World states (:730-752).
    if state == CRATES_IN_PROGRESS then
        setWorldState(st, WORLDSTATE_SHOW_CRATES, 1)
        setWorldState(st, WORLDSTATE_CRATES_REVEALED, 0)
    elseif state == CRATES_DONE then
        setWorldState(st, WORLDSTATE_SHOW_CRATES, 1)
        setWorldState(st, WORLDSTATE_CRATES_REVEALED, NUM_PLAGUE_CRATES)
    else
        setWorldState(st, WORLDSTATE_SHOW_CRATES, 0)
        if state == JUST_STARTED then
            setWorldState(st, WORLDSTATE_CRATES_REVEALED, 0)
        else
            setWorldState(st, WORLDSTATE_CRATES_REVEALED, NUM_PLAGUE_CRATES)
        end
    end
    if state == WAVES_DONE then
        setWorldState(st, WORLDSTATE_WAVE_COUNT, NUM_SCOURGE_WAVES)
    else
        setWorldState(st, WORLDSTATE_WAVE_COUNT, 0)
    end

    -- Hidden Passage GO state (:755-756).
    if st.passage then
        if state <= GAUNTLET_TRANSITION then
            st.passage:SetGoState(GO_STATE_READY)
        else
            st.passage:SetGoState(GO_STATE_ACTIVE)
        end
    end

    if state == CRATES_DONE then
        if st.bunny then
            st.bunny:CastSpell(nil, SPELL_CRATES_KILL_CREDIT, true)
        end
        scheduleEvent(st, EVENT_CRIER_CALL_TO_GATES, 5000)
    elseif state == WAVES_IN_PROGRESS then
        st.waveCount = 0
        st.currentSpawnLoc = 0
        st.waveSpawns = {}
        scheduleEvent(st, EVENT_SCOURGE_WAVE, 1000)
        spawnInfiniteCorruptor(instanceData, mapObj, st)
    end

    if force then
        -- events.Reset + respawn-arthas + SpawnInfiniteCorruptor; the
        -- DeleteRespawnTimes + dead/temp-sweep legs have no bridge.
        st.events = {}
        spawnInfiniteCorruptor(instanceData, mapObj, st)
        scheduleEvent(st, EVENT_RESPAWN_ARTHAS, 1000)
    end

    if mapObj then
        mapObj:SaveInstanceData()
    end
end

local function guardianTick(instanceData, mapObj, st)
    -- SetBossState-override twin (:452-459): a DONE corruptor cancels the
    -- tick and hides the timer states (boss files write the state
    -- directly, so the guard lives here, not in an override).
    local bossState = bossStateOf(st, DATA_INFINITE_CORRUPTOR)
    if bossState == BOSS_STATE_DONE then
        cancelEvent(st, EVENT_GUARDIAN_TICK)
        setWorldState(st, WORLDSTATE_TIME_GUARDIAN_SHOW, 0)
        setWorldState(st, WORLDSTATE_TIME_GUARDIAN, 0)
        return
    end
    if not mapObj or not mapObj:IsHeroic() then
        return
    end
    local secondsToDeath = (instanceData.guardianTimeout or 0) - os.time()
    if secondsToDeath <= 0 then
        instanceData.guardianTimeout = 0
        setWorldState(st, WORLDSTATE_TIME_GUARDIAN_SHOW, 0)
        setWorldState(st, WORLDSTATE_TIME_GUARDIAN, 0)
        if st.corruptor then
            st.corruptor:DoAction(-ACTION_CORRUPTOR_LEAVE)
            if st.guardian then
                st.guardian:Kill(st.corruptor)
            end
        end
        setBossState(st, DATA_INFINITE_CORRUPTOR, BOSS_STATE_FAIL)
        return
    end
    local minutes = math.floor((secondsToDeath - 1) / 60)
    local seconds = ((secondsToDeath - 1) % 60) + 1
    if st.chromie then
        if minutes == 24 and seconds >= 45 then
            st.chromie:Talk(CHROMIE_WHISPER_GUARDIAN_1)
        end
        if minutes == 4 and seconds < 45 then
            st.chromie:Talk(CHROMIE_WHISPER_GUARDIAN_2)
        end
        if minutes == 0 then
            st.chromie:Talk(CHROMIE_WHISPER_GUARDIAN_3)
        end
    end
    setWorldState(st, WORLDSTATE_TIME_GUARDIAN_SHOW, 1)
    setWorldState(st, WORLDSTATE_TIME_GUARDIAN, minutes + 1)
    if minutes == 4 and seconds > 30 then
        scheduleEvent(st, EVENT_GUARDIAN_TICK, (seconds - 30) * 1000)
    else
        scheduleEvent(st, EVENT_GUARDIAN_TICK, seconds * 1000)
    end
end

local function scourgeWave(instanceData, mapObj, st)
    if progressOf(instanceData) ~= WAVES_IN_PROGRESS then
        return
    end
    st.waveCount = st.waveCount + 1
    setWorldState(st, WORLDSTATE_WAVE_COUNT, st.waveCount)
    local spawnLoc = math.random(1, 5)
    while spawnLoc == st.currentSpawnLoc do
        spawnLoc = math.random(1, 5)
    end
    local location = WaveLocations[spawnLoc]
    if st.waveCount == WAVE_MEATHOOK then
        local spawn = summonAt(st, mapObj, NPC_MEATHOOK, location.points[1])
        if spawn then
            st.waveSpawns[spawn:GetGUID()] = spawn
        end
    elseif st.waveCount == WAVE_SALRAMM then
        local spawn = summonAt(st, mapObj, NPC_SALRAMM, location.points[1])
        if spawn then
            st.waveSpawns[spawn:GetGUID()] = spawn
        end
    elseif mapObj and mapObj:IsHeroic() then
        local wave = HeroicWaves[st.waveCount]
        for i = 1, MAX_SPAWNS_PER_WAVE do
            local entry = wave[i]
            if entry and entry ~= 0 then
                local spawn = summonAt(st, mapObj, entry, location.points[i])
                if spawn then
                    st.waveSpawns[spawn:GetGUID()] = spawn
                end
            end
        end
    else
        for i = 1, 2 do
            local spawn = summonAt(st, mapObj, NPC_DEVOURING_GHOUL,
                location.points[i])
            if spawn then
                st.waveSpawns[spawn:GetGUID()] = spawn
            end
        end
    end
    for _, marker in ipairs(WaveMarkers) do
        setWorldState(st, marker, 0)
    end
    setWorldState(st, location.marker, 1)
    scheduleEvent(st, EVENT_CRIER_ANNOUNCE_WAVE, 2000)
    st.currentSpawnLoc = spawnLoc
end

-- Arthas RP SetData legs (instance_culling_of_stratholme.cpp SetData
-- :317-403): one-shot instanceData flags written by
-- npc_arthas_stratholme.lua, consumed here with the exact C++ from-state
-- guards; each flag is cleared whether or not its guard passed.
local function consumeSetDataFlag(instanceData, mapObj, name, from, to)
    if instanceData[name] then
        instanceData[name] = nil
        if progressOf(instanceData) == from then
            setProgress(instanceData, mapObj, to, false)
        end
    end
end

-- InitiateArthasEvent legs (instance_culling_of_stratholme.cpp
-- SetGuidData :405-448 + InitiateArthasEvent :647-654): the gossip
-- AdvanceDungeon flags, consumed with the exact from->to guard;
-- setProgress runs first (AdvanceToState via DoAction(-1)), then the
-- RP-start leg arrives as arthas:DoAction(-ACTION_START_RP_EVENTn).
local function consumeGossipFlag(instanceData, mapObj, st, name, from, to, rpAction)
    if instanceData[name] then
        instanceData[name] = nil
        if progressOf(instanceData) == from then
            setProgress(instanceData, mapObj, to, false)
            if st.arthas then
                st.arthas:DoAction(rpAction)
            end
        end
    end
end

local function onUpdateTick(instanceData, mapObj, st, diff)
    -- DATA_ARTHAS_DIED (npc_arthas_stratholme.lua onDied flag): regress
    -- to the last stable state, forced.
    if instanceData.arthasDied then
        instanceData.arthasDied = nil
        st.arthas = nil
        setProgress(instanceData, mapObj,
            StableStateFor[progressOf(instanceData)] or JUST_STARTED, true)
        return
    end
    -- Arthas RP despawn legs set arthasGone (npc_arthas_stratholme.lua):
    -- drop the stale bind so setProgress below schedules
    -- EVENT_RESPAWN_ARTHAS 1s later at the new state's snapback.
    if instanceData.arthasGone then
        instanceData.arthasGone = nil
        st.arthas = nil
    end
    consumeSetDataFlag(instanceData, mapObj, "utherFinished", UTHER_TALK, PURGE_PENDING)
    consumeSetDataFlag(instanceData, mapObj, "startWaves", PURGE_STARTING, WAVES_IN_PROGRESS)
    consumeSetDataFlag(instanceData, mapObj, "reachTownHall", WAVES_DONE, TOWN_HALL_PENDING)
    consumeSetDataFlag(instanceData, mapObj, "townHallDone", TOWN_HALL, TOWN_HALL_COMPLETE)
    consumeSetDataFlag(instanceData, mapObj, "gauntletReached", GAUNTLET_TRANSITION, GAUNTLET_PENDING)
    consumeSetDataFlag(instanceData, mapObj, "gauntletDone", GAUNTLET_IN_PROGRESS, GAUNTLET_COMPLETE)
    consumeSetDataFlag(instanceData, mapObj, "malganisDone", MALGANIS_IN_PROGRESS, COMPLETE)
    consumeGossipFlag(instanceData, mapObj, st, "startPurge", PURGE_PENDING, PURGE_STARTING, -ACTION_START_RP_EVENT2)
    consumeGossipFlag(instanceData, mapObj, st, "startTownHall", TOWN_HALL_PENDING, TOWN_HALL, -ACTION_START_RP_EVENT3)
    consumeGossipFlag(instanceData, mapObj, st, "toGauntlet", TOWN_HALL_COMPLETE, GAUNTLET_TRANSITION, -ACTION_START_RP_EVENT4_1)
    consumeGossipFlag(instanceData, mapObj, st, "startGauntlet", GAUNTLET_PENDING, GAUNTLET_IN_PROGRESS, -ACTION_START_RP_EVENT4_2)
    consumeGossipFlag(instanceData, mapObj, st, "startMalganis", GAUNTLET_COMPLETE, MALGANIS_IN_PROGRESS, -ACTION_START_RP_EVENT5)
    local due = {}
    for _, ev in ipairs(st.events) do
        ev.remainingMs = ev.remainingMs - diff
        if ev.remainingMs <= 0 then
            due[#due + 1] = ev.id
        end
    end
    for _, id in ipairs(due) do
        cancelEvent(st, id)
        if id == EVENT_GUARDIAN_TICK then
            guardianTick(instanceData, mapObj, st)
        elseif id == EVENT_RESPAWN_ARTHAS then
            local pos = ArthasSnapback[progressOf(instanceData)]
            if pos then
                summonAt(st, mapObj, NPC_ARTHAS, pos)
            end
        elseif id == EVENT_CRIER_CALL_TO_GATES then
            if progressOf(instanceData) == CRATES_DONE and st.crier then
                st.crier:Talk(CRIER_SAY_CALL_TO_GATES)
            end
        elseif id == EVENT_SCOURGE_WAVE then
            scourgeWave(instanceData, mapObj, st)
        elseif id == EVENT_CRIER_ANNOUNCE_WAVE then
            if progressOf(instanceData) == WAVES_IN_PROGRESS and st.crier then
                st.crier:Talk(st.currentSpawnLoc)
            end
        end
    end
end

-- OnUnitDeath (:431-459) twin: ev4 on the wave-mob entries (second
-- handlers — the boss files own Talk/SetBossState on meathook/salramm).
local function onWaveMobDied(event, creature)
    local data = creature:GetMap():GetInstanceData()
    if not data then
        return
    end
    local st = stFor(data)
    if progressOf(data) ~= WAVES_IN_PROGRESS then
        return
    end
    local guid = creature:GetGUID()
    if not st.waveSpawns[guid] then
        return
    end
    st.waveSpawns[guid] = nil
    for _ in pairs(st.waveSpawns) do
        return
    end
    for _, marker in ipairs(WaveMarkers) do
        setWorldState(st, marker, 0)
    end
    if st.waveCount < NUM_SCOURGE_WAVES then
        if st.waveCount == WAVE_MEATHOOK then
            scheduleEvent(st, EVENT_SCOURGE_WAVE, 20000)
        else
            scheduleEvent(st, EVENT_SCOURGE_WAVE, 1000)
        end
    else
        setProgress(data, creature:GetMap(), WAVES_DONE, false)
    end
end

local waveMobEntries = {
    NPC_DEVOURING_GHOUL, NPC_ENRAGED_GHOUL, NPC_NECROMANCER,
    NPC_CRYPT_FIEND, NPC_ACOLYTE, NPC_CRYPT_STALKER, NPC_ABOMINATION,
    NPC_MEATHOOK, NPC_SALRAMM,
}
for _, entry in ipairs(waveMobEntries) do
    RegisterCreatureEvent(entry, 4, onWaveMobDied)
end

-- DATA_CRATE_REVEALED (:371-378) twin: second ev14 on the crate helper
-- (the northrend file owns the fluff relay); this handler keeps its own
-- revealed set and drives the CRATES_REVEALED world state / CRATES_DONE
-- transition.
RegisterCreatureEvent(NPC_CRATE_HELPER, 14, function(event, creature, caster, spellId)
    if spellId ~= SPELL_ARCANE_DISRUPTION then
        return
    end
    local data = creature:GetMap():GetInstanceData()
    if not data then
        return
    end
    local st = stFor(data)
    local guid = creature:GetGUID()
    if st.revealedCrates[guid] then
        return
    end
    st.revealedCrates[guid] = true
    local count = 0
    for _ in pairs(st.revealedCrates) do
        count = count + 1
    end
    if count >= NUM_PLAGUE_CRATES then
        setProgress(data, creature:GetMap(), CRATES_DONE, false)
    else
        setWorldState(st, WORLDSTATE_CRATES_REVEALED, count)
    end
end)

-- Chromie-start SetData writers (culling_of_stratholme.cpp:364-411):
-- second gossip-select handler (the northrend file owns the menu,
-- teleport and AddItem legs). EXPLAIN_2 (1004) -> AdvanceDungeon:
-- DATA_CRATES_START gated on JUST_STARTED. SKIP_1 (1005) ->
-- AdvanceDungeonFar: DATA_SKIP_TO_PURGE gated on progress <= CRATES_DONE.
local GOSSIP_ACTION_INFO_DEF = 1000
RegisterCreatureGossipEvent(NPC_CHROMIE, 2, function(event, player, creature, sender, action)
    local data = creature:GetMap():GetInstanceData()
    if not data then
        return false
    end
    if action == GOSSIP_ACTION_INFO_DEF + 4 then
        if progressOf(data) == JUST_STARTED then
            setProgress(data, creature:GetMap(), CRATES_IN_PROGRESS, false)
        end
    elseif action == GOSSIP_ACTION_INFO_DEF + 5 then
        if progressOf(data) <= CRATES_DONE then
            setProgress(data, creature:GetMap(), PURGE_PENDING, false)
        end
    end
    return false
end)

RegisterMapEvent(MAP_CULLING_OF_STRATHOLME, INSTANCE_EVENT_ON_CREATURE_CREATE,
    function(event, instanceData, mapObj, creature)
        local st = stFor(instanceData)
        local entry = creature:GetEntry()
        if entry == NPC_CHROMIE then
            st.chromie = creature
        elseif entry == NPC_INFINITE_CORRUPTOR then
            st.corruptor = creature
        elseif entry == NPC_GUARDIAN_OF_TIME then
            st.guardian = creature
        elseif entry == NPC_GENERIC_BUNNY then
            st.bunny = creature
        elseif entry == NPC_CRATE_HELPER then
            st.crateHelpers[#st.crateHelpers + 1] = creature
        elseif entry == NPC_ARTHAS then
            st.arthas = creature
        elseif entry == NPC_LORDAERON_CRIER then
            st.crier = creature
        end
    end)

RegisterMapEvent(MAP_CULLING_OF_STRATHOLME, INSTANCE_EVENT_ON_GAMEOBJECT_CREATE,
    function(event, instanceData, mapObj, go)
        if go:GetEntry() == GO_HIDDEN_PASSAGE then
            local st = stFor(instanceData)
            st.passage = go
            if progressOf(instanceData) <= GAUNTLET_TRANSITION then
                go:SetGoState(GO_STATE_READY)
            else
                go:SetGoState(GO_STATE_ACTIVE)
            end
        end
    end)

-- ReadSaveDataMore (:344-366): regress to the stable state (forced) and
-- re-arm the guardian tick from the saved timeout.
RegisterMapEvent(MAP_CULLING_OF_STRATHOLME, INSTANCE_EVENT_ON_LOAD,
    function(event, instanceData, mapObj)
        local st = stFor(instanceData)
        if instanceData.progress then
            setProgress(instanceData, mapObj,
                StableStateFor[instanceData.progress] or JUST_STARTED, true)
        end
        if (instanceData.guardianTimeout or 0) ~= 0 then
            scheduleEvent(st, EVENT_GUARDIAN_TICK, 0)
        end
    end)

RegisterMapEvent(MAP_CULLING_OF_STRATHOLME, INSTANCE_EVENT_ON_UPDATE,
    function(event, instanceData, mapObj, diff)
        onUpdateTick(instanceData, mapObj, stFor(instanceData), diff)
    end)
