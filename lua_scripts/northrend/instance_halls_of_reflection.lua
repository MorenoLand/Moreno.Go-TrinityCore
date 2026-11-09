-- The Halls of Reflection instance script — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/HallsOfReflection/instance_halls_of_reflection.cpp
-- (class instance_halls_of_reflection : public InstanceMapScript, map 668).
-- RegisterMapEvent binds the map-level Eluna instance hooks (the
-- instance_pit_of_saron.lua / instance_forge_of_souls.lua precedent);
-- its presence opens the Map:GetInstanceData gate for map 668
-- (engine/world/instance_hooks.go) that the boss and zone files share
-- with this file.
-- Shared instanceData keys (one table per (map, instance)):
-- falric / marwyn / frostswornGeneral / jainaOrSylvanasIntro /
-- jainaOrSylvanasEscape / lichkingIntro / lichkingEscape / uther /
-- korelnOrLoralen / frostmourneAltarBunny / captain / icewallTarget —
-- the OnCreatureCreate binds (instance_halls_of_reflection.cpp:125-171);
-- gunshipCannons — the NPC_GUNSHIP_CANNON_HORDE 37593 list (:161-164)
-- (NPC_WORLD_TRIGGER 22515 joins it only when it has a transport, and
-- there is no GetTransport bridge — see below).
-- Ported arms (C++-exact unless noted):
-- - OnCreatureCreate: the binds above. Fires on materialization
--   (motionFor funnel, instance_hooks.go:311-320); a static template
--   spawn never materializes server-side and is never bound
--   (naxxramas LoadGrid delta).
-- - OnCreatureCreate conditional legs, via creature:GetBossState
--   (lua_creature_events.go:5936; the oculus precedent):
--   Koreln/Loralen (:128-132): RemoveFlag(UNIT_NPC_FLAGS,
--   UNIT_NPC_FLAG_QUESTGIVER) when GetBossState(DATA_MARWYN 1) !=
--   DONE — the flag is re-armed by the Marwyn DONE override leg
--   (boss_marwyn.lua, garfrost tyrannus-Talk precedent).
--   Frostsworn General (:145-148): SetPhaseMask(1, true) when
--   GetBossState(DATA_MARWYN 1) == DONE — same re-arm source.
-- - GetGuidData (instance_halls_of_reflection.cpp:660-700): the Lua
--   mirror is the instanceData binds above, read as instanceData.falric /
--   instanceData.marwyn / etc. (oculus precedent). Live consumers:
--   boss_marwyn.lua JustDied reads instanceData.frostmourneAltarBunny /
--   korelnOrLoralen / frostswornGeneral for the Marwyn DONE override
--   legs (the kelthuzad GetInstanceData precedent); the intro chain
--   (halls_of_reflection_intro.lua) reads instanceData.falric /
--   instanceData.marwyn for the LK_6 spawn-aura + visible legs and the
--   LK_7/8/9 intro Talks.
-- - OnPlayerEnter team latch (:111-123) + FillInitialWorldStates
--   (:322-326) — PORTED 2026-11-08 (tick 491) via the Go-only
--   InstanceEventOnPlayerEnter bridge (event 4, instance_hooks.go:479):
--   instanceData.teamInInstance latches player:GetTeam() (the PoS
--   precedent); world states 4884 = (intro DONE and marwyn not DONE)
--   and 4882 = _waveCount ride the oculus first-bound-creature caller
--   idiom. The Marwyn-DONE SpawnGunship / SpawnEscapeEvent legs stay
--   out (CreateTransport + instance SummonCreature, no bridges).
-- - SetData switch (event 8) — PORTED 2026-11-08 (tick 491) via the
--   Go-only InstanceEventOnSetData bridge (instance_hooks.go:51; the
--   PoS precedent): DATA_INTRO_EVENT stores _introState (the summon
--   legs stay out, no instance summon); DATA_WAVE_COUNT NOT_STARTED
--   with _waveCount != 0 runs EVENT_DO_WIPE; DATA_FROSTSWORN_GENERAL
--   stores the state (the ShadowThrone door leg stays out, door
--   model); DATA_QUEL_DELAR_EVENT IN_PROGRESS with state NOT_STARTED
--   casts SPELL_ESSENCE_OF_CAPTURED 70720 from the bound bunny
--   (the 2s Uther summon stays out, no instance summon).
-- - Wave machine (ProcessEvent EVENT_SPAWN_WAVES / EVENT_ADD_WAVE /
--   EVENT_DO_WIPE, :573-625/:605-640) — PORTED 2026-11-08 (tick 491):
--   SPAWN_WAVES (signal HOR_SETDATA_SPAWN_WAVES = 1001 from the
--   intro LK_9 leg) spawns all 8 waves from the boss summoners
--   (falric i<=3, marwyn i>3) at the C++-exact SpawnPos[34] with
--   RandomResize counts 3/4/4/5/4/4/5/5, SetData(0, i) wave ids,
--   then ADD_WAVE in 5s; ADD_WAVE activates the wave
--   (72130 + flag clear + SetImmuneToAll(false) + DoZoneInCombat)
--   or sends the boss DoAction(ACTION_ENTER_COMBAT -668001) at
--   wave 5/10; DO_WIPE (SetData DATA_WAVE_COUNT 0 from the
--   falric/marwyn Reset wipe leg) zeroes the count, cancels the
--   pending timer, hides both bosses, despawns the wave lists.
--   The FALRIC DONE ++_waveCount + 1min NEXT_WAVE leg rides the
--   Go-only signal HOR_SETDATA_NEXT_WAVE = 1000 sent from
--   boss_falric.lua JustDied after the DONE write (creature:
--   SetBossState fires no instance event, so the override leg
--   cannot live here). World states 4884/4882 update on every
--   machine step via the first-bound-creature caller.
-- - OnUnitDeath wave-clear check (:530-556) — PORTED 2026-11-08
--   (tick 491) via the Go-only InstanceEventOnUnitDeath bridge
--   (event 9, instance_hooks.go:525): wave entries read their
--   SetData(0) wave id; when every creature of that wave reads
--   dead, ++_waveCount + ADD_WAVE in 3s, with a per-wave pending
--   flag against a double fire (cleared by DO_WIPE).
-- Unmodeled (documented-only, no bridges):
-- - GetCreatureEntry team swap (:176-201): no bridge (there is no
--   GetGameObjectEntry instance event); the team latch itself is
--   ported above (OnPlayerEnter).
-- - SetBossState override (:332-419): DATA_FALRIC DONE (:342-348)
--   ++_waveCount + EVENT_NEXT_WAVE 1min is PORTED above via the
--   HOR_SETDATA_NEXT_WAVE signal from boss_falric.lua JustDied;
--   DATA_MARWYN DONE (:349-371) — the bunny cast 72900,
--   Koreln/Loralen questgiver flag, DoUpdateWorldState(4884, 0) and
--   general SetPhaseMask(1) legs are PORTED in boss_marwyn.lua
--   JustDied (they run inside the C++ SetBossState call, in order
--   after the DONE write); the HandleGameObject door legs ride the
--   door model (no door model in the Go boss-state store,
--   instance_boss_states.go:72); SpawnGunship() (:425-447) rides
--   CreateTransport (no bridge) + the team latch; SpawnEscapeEvent()
--   (:449-467) rides instance SummonCreature + the team latch.
--   DATA_THE_LICH_KING_ESCAPE IN_PROGRESS (:384-388) DoAction
--   (ACTION_START_PREFIGHT) rides the escape-leader AI, whose
--   gunship-arrival trigger (instance ProcessEvent
--   EVENT_GUNSHIP_ARRIVAL, transport arrival) has no Lua fire site;
--   DONE (:389-402) chest-flag / cave-in-door / LK 72830-cast legs
--   ride the same untriggerable chain (the chest GO flag removal
--   and DoUseDoorOrButton have no GO-state bridge either);
--   FAIL (:403-415) rides the LK escape EnterEvadeMode, whose FAIL
--   write IS ported in npc_the_lich_king_escape.lua (leave-combat
--   handler) — the instance-side despawn / icewall-delete / 30s
--   re-spawn legs stay out (no GO-delete / instance-summon bridge).
--   No ON_UPDATE poll: the only DONE legs with a live consumer run
--   at DONE time in the boss died handlers (palehoof precedent).
-- - SetData (:469-517): the switch is PORTED above (event 8). The
--   frostsworn-general door leg is documented in
--   npc_frostsworn_general.lua.
-- - SetGuidData(DATA_QUEL_DELAR_INVOKER) (:519-527): no instance
--   SetGuidData bridge from Lua (the invoker GUID is written only by
--   the Quel'Delar gossip path, unported).
-- - OnUnitDeath wave-clear check (:530-556) — PORTED above (event 9).
-- - OnCreatureRemove wave-list erase (:157-168): Lua has no
--   OnCreatureRemove instance event; the wave lists live in
--   instanceData.waveGuidList and the clear check reads IsAlive()
--   on the stored objects, so a stale entry reads dead and cannot
--   block a clear (lists are rebuilt by SPAWN_WAVES and cleared by
--   DO_WIPE).
-- - FillInitialWorldStates (:322-326) — PORTED above into the
--   OnPlayerEnter handler (same values, same trigger: the packet is
--   built when a player enters).
-- - OnGameObjectCreate (:203-276): Frostmourne phase-2 leg is gated
--   on GetData(DATA_INTRO_EVENT) (no bridge); the door legs ride the
--   door model; the gunship-stairs SetRespawnTime(DAY) leg has no
--   GO SetRespawnTime bridge (pit_of_saron precedent); the captain's
--   chest latch has no consumer (its flag removal rides the LK DONE
--   override above). No GO binds: every GO leg is unbridgeable or
--   gated on unbridged state, so latching the GUIDs would be dead
--   binds (pit_of_saron precedent).
-- - NPC_WORLD_TRIGGER (:156-161): joins GunshipCannonGUIDs only when
--   it has a transport (GetTransport, no bridge) — the bind is
--   skipped; the Horde cannon entry 37593 binds unconditionally.
-- - TheLichkingIntroGUID is bound but never read in C++ (declared
--   "unused", :657) — mirrored as a bind anyway, like the C++.
-- - JainaOrSylvanasIntroGUID is likewise never read in C++ — mirrored
--   as a bind anyway.
-- - ProcessEvent gunship-arrival legs (:573-625: captain Talks
--   SAY_CAPTAIN_FIRE / SAY_CAPTAIN_FINAL, cannon 70017 casts,
--   transport EnableMovement, stairs SetRespawnTime): no transport /
--   ProcessEvent bridges; the escape-leader DoAction legs they drive
--   are documented in the zone audit (halls_of_reflection.cpp
--   npc_jaina_or_sylvanas_escape_hor stays out: MovementInform +
--   transport + instance GetData machine).
-- - SetHeaders(DataHeader "HOR") / SetBossNumber(EncounterCount 3)
--   / SaveToDB / WriteSaveDataMore / ReadSaveDataMore: no bridges;
--   the boss-state store is written via creature:SetBossState
--   (lua_creature_events.go:5917) and read via creature:GetBossState
--   (lua_creature_events.go:5936).
-- No ON_UPDATE poll: see the SetBossState note above.

local MAP_HALLS_OF_REFLECTION = 668
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5

local DATA_MARWYN = 1 -- halls_of_reflection.h:38
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76

local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (baltharus precedent)
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002 -- UnitDefines.h:187

local NPC_JAINA_INTRO = 37221 -- halls_of_reflection.h:63
local NPC_SYLVANAS_INTRO = 37223 -- halls_of_reflection.h:64
local NPC_UTHER = 37225 -- halls_of_reflection.h:65
local NPC_THE_LICH_KING_INTRO = 37226 -- halls_of_reflection.h:66
local NPC_KORELN = 37582 -- halls_of_reflection.h:67
local NPC_LORALEN = 37779 -- halls_of_reflection.h:68
local NPC_FROSTMOURNE_ALTAR_BUNNY = 37704 -- halls_of_reflection.h:69
local NPC_FALRIC = 38112 -- halls_of_reflection.h:71
local NPC_MARWYN = 38113 -- halls_of_reflection.h:72
local NPC_FROSTSWORN_GENERAL = 36723 -- halls_of_reflection.h:79
local NPC_JAINA_ESCAPE = 36955 -- halls_of_reflection.h:82
local NPC_SYLVANAS_ESCAPE = 37554 -- halls_of_reflection.h:83
local NPC_THE_LICH_KING_ESCAPE = 36954 -- halls_of_reflection.h:84
local NPC_ICE_WALL_TARGET = 37014 -- halls_of_reflection.h:85
local NPC_GUNSHIP_CANNON_HORDE = 37593 -- halls_of_reflection.h:91
local NPC_JUSTIN_BARTLETT = 30344 -- halls_of_reflection.h:92
local NPC_KORM_BLACKSCAR = 30824 -- halls_of_reflection.h:93
local NPC_WORLD_TRIGGER = 22515 -- halls_of_reflection.h:95

local bossBinds = {
    [NPC_JAINA_INTRO] = "jainaOrSylvanasIntro",
    [NPC_SYLVANAS_INTRO] = "jainaOrSylvanasIntro",
    [NPC_KORELN] = "korelnOrLoralen",
    [NPC_LORALEN] = "korelnOrLoralen",
    [NPC_THE_LICH_KING_INTRO] = "lichkingIntro",
    [NPC_FALRIC] = "falric",
    [NPC_MARWYN] = "marwyn",
    [NPC_FROSTMOURNE_ALTAR_BUNNY] = "frostmourneAltarBunny",
    [NPC_FROSTSWORN_GENERAL] = "frostswornGeneral",
    [NPC_JAINA_ESCAPE] = "jainaOrSylvanasEscape",
    [NPC_SYLVANAS_ESCAPE] = "jainaOrSylvanasEscape",
    [NPC_THE_LICH_KING_ESCAPE] = "lichkingEscape",
    [NPC_JUSTIN_BARTLETT] = "captain",
    [NPC_KORM_BLACKSCAR] = "captain",
    [NPC_ICE_WALL_TARGET] = "icewallTarget",
    [NPC_UTHER] = "uther",
}

local INSTANCE_EVENT_ON_PLAYER_ENTER = 4
local INSTANCE_EVENT_ON_SET_DATA = 8
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local DATA_FALRIC = 0 -- halls_of_reflection.h:37
local DATA_MARWYN = 1 -- halls_of_reflection.h:38
local DATA_INTRO_EVENT = 3 -- halls_of_reflection.h:41
local DATA_FROSTSWORN_GENERAL = 4 -- halls_of_reflection.h:42
local DATA_WAVE_COUNT = 6 -- halls_of_reflection.h:45
local DATA_QUEL_DELAR_EVENT = 14 -- halls_of_reflection.h:55

local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local ACTION_ENTER_COMBAT = -668001 -- halls_of_reflection.h:130

local SPELL_SPIRIT_ACTIVATE = 72130 -- halls_of_reflection.h:158
local SPELL_ESSENCE_OF_CAPTURED = 70720 -- halls_of_reflection.h:177

local WORLD_STATE_HOR_WAVES_ENABLED = 4884 -- halls_of_reflection.h:188
local WORLD_STATE_HOR_WAVE_COUNT = 4882 -- halls_of_reflection.h:189

local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h (algalon precedent)
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local NPC_WAVE_MERCENARY = 38177 -- halls_of_reflection.h:73
local NPC_WAVE_FOOTMAN = 38173 -- halls_of_reflection.h:74
local NPC_WAVE_RIFLEMAN = 38176 -- halls_of_reflection.h:75
local NPC_WAVE_PRIEST = 38175 -- halls_of_reflection.h:76
local NPC_WAVE_MAGE = 38172 -- halls_of_reflection.h:77

local SUMMON_TYPE_DEAD_DESPAWN = 6 -- lua_creature_events.go:3699-3704

-- Go-only SetData signal types (no C++ SetData type means "next wave"
-- or "spawn waves": EVENT_NEXT_WAVE / EVENT_SPAWN_WAVES are internal
-- EventMap events, halls_of_reflection.h:139-140). Writers call
-- instanceData:SetData(type, 0); the event-8 handler runs the leg.
local HOR_SETDATA_NEXT_WAVE = 1000
local HOR_SETDATA_SPAWN_WAVES = 1001

-- C++ SpawnPos[34] (instance_halls_of_reflection.cpp:42-77); Lua
-- 1-indexed, posIndex advances once per wave-list entry whether or
-- not the summon lands (the ++posIndex sits outside the summon ifs).
local SPAWN_POS = {
    { 5309.577, 2042.668, 707.7781, 4.694936 },
    { 5295.885, 2040.342, 707.7781, 5.078908 },
    { 5340.836, 1992.458, 707.7781, 2.757620 },
    { 5325.072, 1977.597, 707.7781, 2.076942 },
    { 5277.365, 1993.229, 707.7781, 0.401426 },
    { 5275.479, 2001.135, 707.7781, 0.174533 },
    { 5302.448, 2042.222, 707.7781, 4.904375 },
    { 5343.293, 1999.384, 707.7781, 2.914700 },
    { 5295.635, 1973.757, 707.7781, 1.186824 },
    { 5311.031, 1972.229, 707.7781, 1.640610 },
    { 5275.076, 2008.724, 707.7781, 6.213372 },
    { 5316.701, 2041.550, 707.7781, 4.502949 },
    { 5344.150, 2007.168, 707.7781, 3.159046 },
    { 5319.158, 1973.998, 707.7781, 1.919862 },
    { 5302.247, 1972.415, 707.7781, 1.378810 },
    { 5277.739, 2016.882, 707.7781, 5.969026 },
    { 5322.964, 2040.288, 707.7781, 4.345870 },
    { 5343.467, 2015.951, 707.7781, 3.490659 },
    { 5313.820, 1978.146, 707.7781, 1.745329 },
    { 5279.649, 2004.656, 707.7781, 0.069814 },
    { 5306.057, 2037.002, 707.7781, 4.817109 },
    { 5337.865, 2003.403, 707.7781, 2.984513 },
    { 5299.434, 1979.009, 707.7781, 1.239184 },
    { 5312.752, 2037.122, 707.7781, 4.590216 },
    { 5335.724, 1996.859, 707.7781, 2.740167 },
    { 5280.632, 2012.156, 707.7781, 6.056293 },
    { 5320.369, 1980.125, 707.7781, 2.007129 },
    { 5306.572, 1977.474, 707.7781, 1.500983 },
    { 5336.599, 2017.278, 707.7781, 3.473205 },
    { 5282.897, 2019.597, 707.7781, 5.881760 },
    { 5318.704, 2036.108, 707.7781, 4.223697 },
    { 5280.513, 1997.842, 707.7781, 0.296706 },
    { 5337.833, 2010.057, 707.7781, 3.228859 },
    { 5299.250, 2035.998, 707.7781, 5.026548 },
}

-- First bound creature wins: the UpdateWorldState caller for a leg
-- (any creature in the instance reaches every player in it,
-- lua_creature_events.go:6395; the oculus precedent).
local function worldStateCaller(data)
    return data.falric or data.marwyn or data.jainaOrSylvanasIntro or data.jainaOrSylvanasEscape
end

local function updateWorldState(data, field, value)
    local caller = worldStateCaller(data)
    if caller then
        caller:UpdateWorldState(field, value)
    end
end

local function waveList(data, waveId)
    if data.waveGuidList == nil then
        data.waveGuidList = {}
    end
    if data.waveGuidList[waveId] == nil then
        data.waveGuidList[waveId] = {}
    end
    return data.waveGuidList[waveId]
end

local addWave -- forward: scheduleNextWave closes over it (tharon_ja idiom)

local function scheduleNextWave(data, delayMs)
    -- C++ EventMap never holds two EVENT_NEXT_WAVE at once on these
    -- paths; cancelling first keeps the single slot.
    if data.nextWaveTimer ~= nil then
        RemoveEventById(data.nextWaveTimer)
        data.nextWaveTimer = nil
    end
    data.nextWaveTimer = CreateLuaEvent(function() addWave(data) end, delayMs)
end

-- C++ ProcessEvent EVENT_SPAWN_WAVES (:573-604): _waveCount = 1, world
-- states, then all 8 waves at once from the boss summoners.
local function spawnWaves(data)
    data.waveCount = 1
    data.wavePending = {}
    updateWorldState(data, WORLD_STATE_HOR_WAVES_ENABLED, 1)
    updateWorldState(data, WORLD_STATE_HOR_WAVE_COUNT, 1)
    local possibility = { NPC_WAVE_MERCENARY, NPC_WAVE_FOOTMAN, NPC_WAVE_RIFLEMAN, NPC_WAVE_PRIEST, NPC_WAVE_MAGE }
    local posIndex = 1
    for i = 0, 7 do
        local tempList = {}
        for _, entry in ipairs(possibility) do
            tempList[#tempList + 1] = entry
        end
        local keep = 5
        if i == 0 then
            keep = 3
        elseif i < 6 and i ~= 3 then
            keep = 4
        end
        -- Trinity::Containers::RandomResize keeps `keep` random entries.
        while #tempList > keep do
            table.remove(tempList, math.random(#tempList))
        end
        local boss = nil
        if i <= 3 then
            boss = data.falric
        else
            boss = data.marwyn
        end
        for _, entry in ipairs(tempList) do
            if boss ~= nil then
                local pos = SPAWN_POS[posIndex]
                local temp = boss:SummonCreature(entry, pos[1], pos[2], pos[3], pos[4], SUMMON_TYPE_DEAD_DESPAWN, 0)
                if temp ~= nil then
                    temp:SetData(0, i)
                    local list = waveList(data, i)
                    list[#list + 1] = temp
                end
            end
            posIndex = posIndex + 1
        end
    end
    scheduleNextWave(data, 5000)
end

-- C++ ProcessEvent EVENT_ADD_WAVE (:605-640).
addWave = function(data)
    data.nextWaveTimer = nil
    local waveCount = data.waveCount or 0
    updateWorldState(data, WORLD_STATE_HOR_WAVES_ENABLED, 1)
    updateWorldState(data, WORLD_STATE_HOR_WAVE_COUNT, waveCount)
    -- HandleGameObject(EntranceDoorGUID, false) stays out (door model).
    if waveCount % 5 ~= 0 then
        local internalWaveId = waveCount - ((waveCount < 5) and 1 or 2)
        for _, temp in ipairs(waveList(data, internalWaveId)) do
            -- The dormant flag/immune state is template-side (no
            -- in-tree setter; the Well of Souls spawn aura is the
            -- visible half, npc_hor_gauntlet_trash.lua); the clear
            -- legs below are C++-exact.
            temp:CastSpell(temp, SPELL_SPIRIT_ACTIVATE, false)
            temp:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            temp:SetImmuneToAll(false)
            temp:DoZoneInCombat()
        end
    else
        local bossIndex = (waveCount / 5) - 1
        local boss = nil
        if bossIndex == 0 then
            boss = data.falric
        else
            boss = data.marwyn
        end
        -- Unbound boss: the boss state cannot be read (instanceData
        -- has no GetBossState); retry in 5s — the boss materializes
        -- on player proximity, and the wave cannot progress without
        -- players in the hall anyway.
        if boss == nil then
            scheduleNextWave(data, 5000)
            return
        end
        if boss:GetBossState(DATA_FALRIC + bossIndex) ~= BOSS_STATE_DONE then
            boss:DoAction(ACTION_ENTER_COMBAT)
        elseif waveCount ~= 10 then
            data.waveCount = waveCount + 1
            scheduleNextWave(data, 5000)
        end
    end
end

-- C++ ProcessEvent EVENT_DO_WIPE (:641-660): _waveCount = 0, events
-- reset, world states, both bosses hidden, wave NPCs despawned.
local function doWipe(data)
    data.waveCount = 0
    data.wavePending = {}
    if data.nextWaveTimer ~= nil then
        RemoveEventById(data.nextWaveTimer)
        data.nextWaveTimer = nil
    end
    updateWorldState(data, WORLD_STATE_HOR_WAVES_ENABLED, 1)
    updateWorldState(data, WORLD_STATE_HOR_WAVE_COUNT, 0)
    -- HandleGameObject(EntranceDoorGUID, true) stays out (door model).
    if data.falric ~= nil then
        data.falric:SetVisible(false)
    end
    if data.marwyn ~= nil then
        data.marwyn:SetVisible(false)
    end
    if data.waveGuidList ~= nil then
        for i = 0, 7 do
            local list = data.waveGuidList[i]
            if list ~= nil then
                for _, creature in ipairs(list) do
                    creature:DespawnOrUnsummon(1)
                end
            end
            data.waveGuidList[i] = nil
        end
    end
end

RegisterMapEvent(MAP_HALLS_OF_REFLECTION, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local entry = creature:GetEntry()
    if entry == NPC_GUNSHIP_CANNON_HORDE then
        if instanceData.gunshipCannons == nil then
            instanceData.gunshipCannons = {}
        end
        instanceData.gunshipCannons[#instanceData.gunshipCannons + 1] = creature
        return
    end
    -- C++ :156-161: NPC_WORLD_TRIGGER joins the cannon list only when
    -- it has a transport; no GetTransport bridge, so the bind is
    -- skipped (see header).
    if entry == NPC_WORLD_TRIGGER then
        return
    end
    local key = bossBinds[entry]
    if not key then
        return
    end
    instanceData[key] = creature
    -- C++ :128-132: Koreln/Loralen loses the questgiver flag while
    -- Marwyn is not DONE (re-armed by the Marwyn DONE leg).
    if entry == NPC_KORELN or entry == NPC_LORALEN then
        if creature:GetBossState(DATA_MARWYN) ~= BOSS_STATE_DONE then
            creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
        end
    -- C++ :145-148: the Frostsworn General phases in once Marwyn is DONE.
    elseif entry == NPC_FROSTSWORN_GENERAL then
        if creature:GetBossState(DATA_MARWYN) == BOSS_STATE_DONE then
            creature:SetPhaseMask(1, true)
        end
    end
end)

-- C++ OnPlayerEnter (:111-123): team latch. The Marwyn-DONE
-- SpawnGunship / SpawnEscapeEvent legs stay out (CreateTransport +
-- instance SummonCreature, no bridges). FillInitialWorldStates
-- (:322-326) rides the same trigger.
RegisterMapEvent(MAP_HALLS_OF_REFLECTION, INSTANCE_EVENT_ON_PLAYER_ENTER, function(event, instanceData, mapObj, player)
    if instanceData.teamInInstance == nil and player ~= nil then
        instanceData.teamInInstance = player:GetTeam()
    end
    local caller = worldStateCaller(instanceData)
    if caller ~= nil then
        local marwynDone = false
        if instanceData.marwyn ~= nil then
            marwynDone = instanceData.marwyn:GetBossState(DATA_MARWYN) == BOSS_STATE_DONE
        end
        local enabled = 0
        if (instanceData.introState or 0) == BOSS_STATE_DONE and not marwynDone then
            enabled = 1
        end
        caller:UpdateWorldState(WORLD_STATE_HOR_WAVES_ENABLED, enabled)
        caller:UpdateWorldState(WORLD_STATE_HOR_WAVE_COUNT, instanceData.waveCount or 0)
    end
end)

-- C++ SetData (:469-517) via the Go-only InstanceEventOnSetData bridge
-- (event 8). No table write happens in the bridge itself, so every leg
-- stores its own state above (the PoS precedent).
RegisterMapEvent(MAP_HALLS_OF_REFLECTION, INSTANCE_EVENT_ON_SET_DATA, function(event, instanceData, mapObj, type, data)
    if type == DATA_INTRO_EVENT then
        -- Summon legs stay out (instance SummonCreature has no bridge);
        -- the state store ports: the intro DONE leg and the world
        -- states read instanceData.introState.
        instanceData.introState = data
    elseif type == DATA_WAVE_COUNT then
        if (instanceData.waveCount or 0) ~= 0 and data == BOSS_STATE_NOT_STARTED then
            doWipe(instanceData)
        end
    elseif type == DATA_FROSTSWORN_GENERAL then
        -- HandleGameObject(ShadowThroneDoorGUID, true) stays out (door model).
        instanceData.frostswornGeneralState = data
    elseif type == DATA_QUEL_DELAR_EVENT then
        if data == BOSS_STATE_IN_PROGRESS and (instanceData.quelDelarState or 0) == BOSS_STATE_NOT_STARTED then
            if instanceData.frostmourneAltarBunny ~= nil then
                instanceData.frostmourneAltarBunny:CastSpell(instanceData.frostmourneAltarBunny, SPELL_ESSENCE_OF_CAPTURED)
            end
            -- EVENT_QUEL_DELAR_SUMMON_UTHER (2s Uther summon) stays out:
            -- instance SummonCreature has no bridge.
        end
        instanceData.quelDelarState = data
    elseif type == HOR_SETDATA_NEXT_WAVE then
        -- Falric DONE (boss_falric.lua JustDied, after the DONE write):
        -- ++_waveCount + EVENT_NEXT_WAVE in 1min.
        instanceData.waveCount = (instanceData.waveCount or 0) + 1
        scheduleNextWave(instanceData, 60000)
    elseif type == HOR_SETDATA_SPAWN_WAVES then
        -- Intro EVENT_INTRO_LK_9 (halls_of_reflection_intro.lua).
        spawnWaves(instanceData)
    end
    -- Other types fall through, the C++ default arm.
end)

-- C++ OnUnitDeath wave-clear check (:530-556) via the Go-only
-- InstanceEventOnUnitDeath bridge (event 9, fired from setDeathState
-- ahead of the JustDied hooks). Player deaths reach here too and are
-- filtered out; the C++ switch only handles the five wave entries.
RegisterMapEvent(MAP_HALLS_OF_REFLECTION, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit:GetObjectType() ~= "Creature" then
        return
    end
    local entry = unit:GetEntry()
    if entry ~= NPC_WAVE_MERCENARY and entry ~= NPC_WAVE_FOOTMAN and entry ~= NPC_WAVE_RIFLEMAN
            and entry ~= NPC_WAVE_PRIEST and entry ~= NPC_WAVE_MAGE then
        return
    end
    -- The wave id is the temp->AI()->SetData(0, i) write from
    -- SPAWN_WAVES, read back from the per-motion ScriptData store.
    local waveId = unit:GetData(0)
    if instanceData.wavePending == nil then
        instanceData.wavePending = {}
    end
    if instanceData.wavePending[waveId] then
        return
    end
    for _, npc in ipairs(waveList(instanceData, waveId)) do
        if npc:IsAlive() then
            return
        end
    end
    instanceData.wavePending[waveId] = true
    instanceData.waveCount = (instanceData.waveCount or 0) + 1
    scheduleNextWave(instanceData, 3000)
end)
