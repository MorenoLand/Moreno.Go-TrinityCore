-- The Black Morass instance script — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- instance_the_black_morass.cpp (356 lines; class
-- instance_the_black_morass : public InstanceMapScript, map 269,
-- TBMScriptName "instance_the_black_morass"; AddSC at end registers
-- it; kalimdor loader decl 45 / call 158 per
-- kalimdor_script_loader.cpp — right after AddSC_the_black_morass()).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_zulfarrak.lua precedent): the map binding covers every
-- Black Morass instance (engine/scripting/runtime.go:653), and its
-- presence opens the Map:GetInstanceData gate for map 269
-- (engine/world/instance_hooks.go:42-58) that the medivh, time-rift,
-- aeonus, deja and temporus scripts share.
-- Shared instanceData keys (one table per (map, instance), persistent
-- across fires): medivh (creature object, ON_CREATURE_CREATE bind),
-- medivhState (TYPE_MEDIVH: 0 NOT_STARTED / 1 IN_PROGRESS / 2 FAIL /
-- 3 DONE; nil reads as NOT_STARTED), riftState (TYPE_RIFT, same
-- scale; C++ stores both in m_auiEncounter[0]/[1]), shield
-- (DATA_SHIELD, nil reads as 100), portalCount (DATA_PORTAL_COUNT),
-- waveId (mRiftWaveId), currentRiftId (_currentRiftId),
-- nextPortalMs / portalPending (the EVENT_NEXT_PORTAL schedule),
-- riftwave_<guid> / pump_<guid> (per-rift wave state + pump timer id).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: Medivh (15608) GUID bind, stored as the creature
--   object (instance_the_black_morass.cpp:137; C++ stores the GUID,
--   Lua stores the object — the same handle the hooks receive).
-- - Update portal machine (C++ Update + ScheduleEventNextPortal +
--   DoSpawnPortal + SummonedPortalBoss + GetRiftWaveId): gated on
--   riftState == IN_PROGRESS; per ON_UPDATE diff tick the countdown
--   runs down, then portalCount++, WORLD_STATE_BM_RIFT broadcast,
--   portal spawn, and the next schedule from RiftWaves[waveId].
--   GetRiftWaveId is replicated with its side effects (count 6 sets
--   waveId 2 but returns 1; count 12 sets waveId 4 but returns 3;
--   count 18 returns 5) and its call order (spawn first, schedule
--   second). Portal pick is urand(0,2) skipping the current id.
--   Rift summon is TEMPSUMMON_CORPSE_DESPAWN (type 4) with the two
--   UNIT_FIELD_FLAGS bits; boss summon is TEMPSUMMON_TIMED_OR_DEAD_
--   DESPAWN (type 0, 10min) at a random near position (drakos inline
--   idiom: random angle + 10*rand radius) with Z from Map:GetHeight.
--   Aeonus gets 0.0 threat on Medivh, others 0.0 on the rift plus the
--   SPELL_RIFT_CHANNEL 31387 cast (rift -> boss, non-triggered).
-- - Time-rift wave pump (C++ npc_time_rift UpdateAI 15s arm, started
--   here at spawn because TempSummons get no spawn-time Lua hook):
--   per-rift wave/count state in instanceData, first tick at 15s,
--   self-rescheduling one-shots (temporus timer idiom), cancelled by
--   the rift's OnDied handler. DoSelectSummon reset rule and the
--   whelp-x3 arm are C++-exact, as are DoSummonAtRift's gate, summon
--   type (TIMED_DESPAWN_OUT_OF_COMBAT, 30s) and 0.0-threat-on-Medivh
--   leg (inlined in the pump: DoSummonAtRift is a C++ method, not a
--   Lua bridge).
-- - Time-rift death on channel break (C++ npc_time_rift UpdateAI,
--   the_black_morass.cpp:334-348: if not IsNonMeleeSpellCast(false)
--   -> setDeathState(JUST_DIED) + SetData(TYPE_RIFT, SPECIAL) when
--   IN_PROGRESS; IsNonMeleeSpellCast(false) counts channeled spells,
--   Unit.cpp:3182): (a) the Aeonus rift is never channeled
--   (DoSpawnPortal casts SPELL_RIFT_CHANNEL only for non-Aeonus
--   bosses), so it dies on its first AI tick — the rift is killed
--   in spawnPortal right after the boss summon, with no pump
--   started; the SPECIAL leg is a no-op there (portalCount 18).
--   (b) when DoSummonAtRift's medivhState gate fails on a real
--   summon attempt it removes all auras, breaking the channel, so
--   the rift dies on the same tick — the pump kills the rift after
--   a failed-gate tick instead of rescheduling (a 0-entry tick
--   returns before the gate check, so the channel survives it).
-- Unmodeled (documented-only, no bridges):
-- - OnPlayerEnter's WORLD_STATE_BM 0 send to the entering player:
--   no per-player world-state bridge exists (creature:UpdateWorldState
--   broadcasts to the whole instance); the start broadcast covers
--   players present at start.
-- - Quest credit on SetData TYPE_MEDIVH DONE (QUEST_OPENING_PORTAL
--   10297 + QUEST_MASTER_TOUCH 9836 AreaExploredOrEventHappens):
--   PORTED at both DONE writers (Aeonus JustDied in boss_aeonus.lua,
--   Medivh WIN leg in the_black_morass.lua); the old "no
--   quest-event-happens bridge" note was stale
--   (player:AreaExploredOrEventHappens live, lua_player.go:4457).
-- - CanProgressEvent's empty-instance Clear (shield/counts/wave
--   reset): ON_UPDATE fires only for instances with players, so the
--   empty state is never observed here.
-- - Z normalization's water leg: C++ takes max(map height, water
--   level); Map has GetHeight but no GetWaterLevel bridge, so Z is
--   height-only (rift areas are dry ground in practice).
-- - SaveToDB / instance-data persistence across restarts: the table
--   lives for the runtime lifetime only (instance-data-table delta).

local MAP_BLACK_MORASS = 269
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local STATE_NOT_STARTED = 0
local STATE_IN_PROGRESS = 1

local WORLD_STATE_BM_RIFT = 2784

local NPC_MEDIVH = 15608
local NPC_TIME_RIFT = 17838
local NPC_RIFT_KEEPER = 21104
local NPC_RIFT_LORD = 17839
local NPC_CRONO_LORD_DEJA = 17879
local NPC_TEMPORUS = 17880
local NPC_AEONUS = 17881

local SPELL_RIFT_CHANNEL = 31387

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- PortalLocation[tmp+1] (instance_the_black_morass.cpp:47-53).
local PortalLocation = {
    {-2041.06, 7042.08, 29.99, 1.30},
    {-1968.18, 7042.11, 21.93, 2.12},
    {-1885.82, 7107.36, 22.32, 3.07},
    {-1928.11, 7175.95, 22.11, 3.44},
}

-- RiftWaves[waveId+1] = {PortalBoss, NextPortalTime ms}
-- (instance_the_black_morass.cpp:62-69; RIFT_BOSS = 1 sentinel).
local RIFT_BOSS = 1
local RiftWaves = {
    {RIFT_BOSS, 0},
    {NPC_CRONO_LORD_DEJA, 0},
    {RIFT_BOSS, 120000},
    {NPC_TEMPORUS, 140000},
    {RIFT_BOSS, 120000},
    {NPC_AEONUS, 0},
}

-- C++ GetRiftWaveId, side effects included.
local function getRiftWaveId(data)
    local count = data.portalCount or 0
    if count == 6 then
        data.waveId = 2
        return 1
    elseif count == 12 then
        data.waveId = 4
        return 3
    elseif count == 18 then
        return 5
    end
    return data.waveId or 0
end

local function spawnPortal(data, mapObj)
    local medivh = data.medivh
    if not medivh then
        return
    end
    local tmp = math.random(0, 2)
    local current = data.currentRiftId or 0
    if tmp >= current then
        tmp = tmp + 1
    end
    data.currentRiftId = tmp
    local loc = PortalLocation[tmp + 1]
    local rift = medivh:SummonCreature(NPC_TIME_RIFT, loc[1], loc[2], loc[3], loc[4], 4, 0)
    if not rift then
        return
    end
    rift:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    rift:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    local wave = getRiftWaveId(data)
    local entry = RiftWaves[wave + 1][1]
    if entry == RIFT_BOSS then
        if math.random(0, 1) == 1 then
            entry = NPC_RIFT_KEEPER
        else
            entry = NPC_RIFT_LORD
        end
    end
    local angle = math.random() * 2 * math.pi
    local dist = 10 * math.random()
    local x = rift:GetX() + dist * math.cos(angle)
    local y = rift:GetY() + dist * math.sin(angle)
    local z = mapObj:GetHeight(x, y)
    if z == nil then
        z = rift:GetZ()
    end
    local boss = rift:SummonCreature(entry, x, y, z, rift:GetO(), 0, 600000)
    if boss then
        if entry == NPC_AEONUS then
            boss:AddThreat(medivh, 0.0)
        else
            boss:AddThreat(rift, 0.0)
            rift:CastSpell(boss, SPELL_RIFT_CHANNEL, false)
        end
    end
    -- C++ npc_time_rift UpdateAI (the_black_morass.cpp:334-348): a
    -- rift that is not casting dies on its first AI tick. The Aeonus
    -- rift is never channeled (no SPELL_RIFT_CHANNEL cast above),
    -- so it dies at once and never pumps; the SPECIAL leg is a
    -- no-op here (portalCount is 18, not < 7).
    if entry == NPC_AEONUS then
        rift:Kill()
        return
    end
    -- Time-rift wave pump state, started here (no spawn-time hook
    -- exists for TempSummons); the rift's OnDied handler cancels it.
    local guid = rift:GetGUIDLow()
    local waveCount = data.portalCount or 0
    local riftWave = 1
    if waveCount < 6 then
        riftWave = 0
    elseif waveCount > 12 then
        riftWave = 2
    end
    data["riftwave_" .. guid] = {wave = riftWave, count = 0}
    -- C++ npc_time_rift::DoSummonAtRift, inlined here (it is a C++
    -- method, not a Lua bridge): the medivhState gate, the near
    -- position with height Z, the 30s TIMED_DESPAWN_OUT_OF_COMBAT
    -- summon, and the 0.0 threat on Medivh.
    local function summonAtRift(summonEntry)
        if data.medivhState ~= STATE_IN_PROGRESS then
            rift:InterruptNonMeleeSpells()
            rift:RemoveAllAuras()
            return false
        end
        local angle = math.random() * 2 * math.pi
        local dist = 10 * math.random()
        local x = rift:GetX() + dist * math.cos(angle)
        local y = rift:GetY() + dist * math.sin(angle)
        local z = mapObj:GetHeight(x, y)
        if z == nil then
            z = rift:GetZ()
        end
        local summon = rift:SummonCreature(summonEntry, x, y, z, rift:GetO(), 3, 30000)
        if summon and data.medivh then
            summon:AddThreat(data.medivh, 0.0)
        end
        return true
    end
    local function pumpTick()
        local st = data["riftwave_" .. guid]
        if not st then
            return
        end
        if (st.count > 2 and st.wave < 1) or st.count > 3 then
            st.count = 0
        end
        local waves = {
            {17835, 21818, 17892, 0},
            {18994, 17892, 21818, 17835},
            {18994, 18995, 17892, 17835},
        }
        local summonEntry = waves[st.wave + 1][st.count + 1]
        st.count = st.count + 1
        local gateFailed = false
        if summonEntry ~= nil and summonEntry ~= 0 then
            local n = 1
            if summonEntry == 21818 then
                n = 3
            end
            for _ = 1, n do
                if not summonAtRift(summonEntry) then
                    gateFailed = true
                end
            end
        end
        -- C++ npc_time_rift UpdateAI (the_black_morass.cpp:334-348):
        -- the failed-gate summon removed all auras above, breaking
        -- the rift channel, so the rift dies on the same tick (the
        -- SPECIAL leg is a no-op: the gate only fails once the event
        -- has left IN_PROGRESS, and the FAIL leg already set rift
        -- NOT_STARTED). No reschedule; OnDied cleans the pump state.
        if gateFailed then
            rift:Kill()
            return
        end
        data["pump_" .. guid] = CreateLuaEvent(pumpTick, 15000, 1)
    end
    data["pump_" .. guid] = CreateLuaEvent(pumpTick, 15000, 1)
end

RegisterMapEvent(MAP_BLACK_MORASS, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == NPC_MEDIVH then
        instanceData.medivh = creature
    end
end)

RegisterMapEvent(MAP_BLACK_MORASS, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    if instanceData.riftState ~= STATE_IN_PROGRESS then
        return
    end
    if not instanceData.portalPending then
        return
    end
    instanceData.nextPortalMs = (instanceData.nextPortalMs or 0) - diff
    if instanceData.nextPortalMs > 0 then
        return
    end
    instanceData.portalCount = (instanceData.portalCount or 0) + 1
    if instanceData.medivh then
        instanceData.medivh:UpdateWorldState(WORLD_STATE_BM_RIFT, instanceData.portalCount)
    end
    spawnPortal(instanceData, mapObj)
    local wave = getRiftWaveId(instanceData)
    local nextTime = RiftWaves[wave + 1][2]
    if nextTime > 0 then
        instanceData.nextPortalMs = nextTime
        instanceData.portalPending = true
    else
        instanceData.portalPending = false
    end
end)
