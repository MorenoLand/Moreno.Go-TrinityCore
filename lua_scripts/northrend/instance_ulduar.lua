-- The Ulduar instance script — Lua port of the Yogg-Saron legs of
-- src/server/scripts/Northrend/Ulduar/Ulduar/instance_ulduar.cpp
-- (class instance_ulduar : public InstanceMapScript, map 603).
-- RegisterMapEvent binds the map-level Eluna instance hook (the
-- instance_naxxramas.lua precedent); its presence opens the
-- Map:GetInstanceData gate for map 603
-- (engine/world/instance_hooks.go) that the Yogg-Saron
-- EVENT_ILLUSION chain shares through this file's table.
-- Shared instanceData keys (one table per (map, instance)):
-- voice / sara / brain — the creature objects bound by
-- ON_CREATURE_CREATE, the C++ LoadObjectData latching
-- (instance_ulduar.cpp:109-111: NPC_VOICE_OF_YOGG_SARON ->
-- DATA_VOICE_OF_YOGG_SARON, NPC_SARA -> DATA_SARA,
-- NPC_BRAIN_OF_YOGG_SARON -> DATA_BRAIN_OF_YOGG_SARON);
-- illusion — the DATA_ILLUSION value (0 CHAMBER / 1 ICECROWN /
-- 2 STORMWIND, ulduar.h:508-510), written by the voice's
-- EVENT_ILLUSION arm and read by the brain's
-- ACTION_INDUCE_MADNESS / ACTION_TENTACLE_KILLED arms
-- (boss_yogg_saron.cpp:599-600/:1104/:1115), initialized to 0,
-- the C++ member default (instance_ulduar.cpp GetData returns
-- the `illusion` member, 0 before the first illusion).
-- keepersCount — the C++ keepersCount member (instance_ulduar.cpp
-- :180/:357-377, DATA_KEEPERS_COUNT), incremented at the
-- observation-ring keeper gossip summon (boss_yogg_saron.lua),
-- read by the Go lights-in-the-darkness gates; driveMeCrazy —
-- DATA_DRIVE_ME_CRAZY (Yogg Reset true, sanity OnRemove false,
-- boss_yogg_saron.lua), read by the Go DRIVE_ME_CRAZY gate;
-- maxArmorItemLevel / maxWeaponItemLevel — the Algalon
-- item-level scan (boss_algalon_the_observer.lua), read by the Go
-- HERALD_OF_TITANS gate. All are read by checkUlduarCriteria
-- (engine/world/achievement_criteria_scripts.go, tick 436).
-- coUAchivePlayerDeathMask — the C++ _CoUAchivePlayerDeathMask
-- member (instance_ulduar.cpp:528-540), set by the OnUnitDeath
-- handler below, read by the Go Conqueror-of-Ulduar gates;
-- anyCreature — the most recently created creature in the
-- instance (ON_CREATURE_CREATE latch), the GetBossState probe
-- for that handler (GetBossState reads Go instance state off
-- the creature's map/instance, lua_creature_events.go:6438 —
-- any creature in the instance works, so the Yogg-room voice
-- binding is not required); conSpeedAtoryStarted /
-- lumberjackedTimedStarted — the one-shot latches for the
-- OnUnitDeath timed-achievement starts below.
-- The mimiron keeper object is NOT bound here: YS keepers are
-- temp summons, which bypass the ON_CREATURE_CREATE funnel
-- (instance_hooks.go) — the gossip summon latches it instead
-- (boss_yogg_saron.lua, the C++ KeeperGUIDs[3] leg,
-- instance_ulduar.cpp:374-377). The GO brain-room doors
-- (194635-194637) have no latch: HandleGameObject has no bridge.
--
-- PORTED (tick 259) — observation-ring keeper summons +
-- lumberjacked credit (instance_ulduar.cpp SetBossState override
-- :630-644 + OnPlayerEnter :250-270, one ON_UPDATE poll below,
-- the instance_oculus.lua precedent):
--   Freya/Hodir/Thorim/Mimiron DONE -> SummonCreature the
--   observation-ring keeper (33241/33213/33242/33244) at
--   ObservationRingKeepersPos (boss_yogg_saron.cpp:386-392),
--   summoned by the bound voice object (the instance_eye_of_
--   eternity.lua bound-creature summon precedent — the map
--   script has no summon of its own). The C++ !KeeperGUIDs[i]
--   gate has a Lua twin only for mimiron (instanceData.mimiron,
--   latched by the gossip summon); for freya/hodir/thorim no
--   latch exists, and the gate is vacuous in the normal flow —
--   a YS keeper for a slot can only exist after its ring keeper
--   was gossiped, which requires this summon to have run.
--   Elders Brightleaf/Ironbranch/Stonebark all DONE + Freya NOT
--   DONE -> the Freya achieve trigger (33406, bound below)
--   self-casts SPELL_LUMBERJACKED_CREDIT 65296 triggered.
--   The poll subsumes both C++ sites (SetBossState override for
--   DONE-while-inside, OnPlayerEnter for DONE-before-entry /
--   reload) and stops once all five legs have fired.
-- PORTED (tick 467) — OnUnitDeath (instance_ulduar.cpp:528-574):
-- the player leg (CoU death mask), the Freya-trash Con Speed
-- Atory timed start (21597) and the elder Lumberjacked timed
-- start (21686) — handler at the end of this file. The player
-- leg rides the Go player-death sender (killPlayer /
-- completeSpiritOfRedemptionDeath -> INSTANCE_EVENT_ON_UNIT_DEATH
-- 9); the creature legs ride the tick-466 creature sender.
-- DOCUMENTED-ONLY: door data / boundaries (no door/boundary
-- machine), the remaining SetData dispatch (DATA_ILLUSION /
-- DATA_DRIVE_ME_CRAZY / DATA_KEEPERS_COUNT / the Herald scan are
-- live via the shared table slots above; the rest rides unbridged
-- instance hooks), the Algalon summon timer + _algalonTimer
-- world states, SaveToDB (the C++ per-bit SaveToDB in OnUnitDeath
-- has no Lua twin; the mask persists with the table).
local MAP_ULDUAR = 603
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_UNIT_DEATH = 9

local BOSS_STATE_DONE = 3
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_ALGALON = 13 -- ulduar.h:49; the C++ mask loop runs i < BOSS_ALGALON

local ACHIEVEMENT_TIMED_TYPE_EVENT = 1
local CRITERIA_CON_SPEED_ATORY = 21597 -- ulduar.h:348
local CRITERIA_LUMBERJACKED = 21686 -- ulduar.h:349

local ENTRY_VOICE_OF_YOGG_SARON = 33280 -- ulduar.h:203
local ENTRY_SARA = 33134 -- ulduar.h:197
local ENTRY_BRAIN_OF_YOGG_SARON = 33890 -- ulduar.h:218
local ENTRY_FREYA_ACHIEVE_TRIGGER = 33406 -- ulduar.h:169

local BOSS_FREYA = 9 -- ulduar.h:45
local BOSS_HODIR = 7 -- ulduar.h:43
local BOSS_THORIM = 8 -- ulduar.h:44
local BOSS_MIMIRON = 10 -- ulduar.h:46
local BOSS_BRIGHTLEAF = 14 -- ulduar.h:50
local BOSS_IRONBRANCH = 15 -- ulduar.h:51
local BOSS_STONEBARK = 16 -- ulduar.h:52

local SPELL_LUMBERJACKED_CREDIT = 65296 -- ulduar.h:493

-- Observation-ring keeper entries + ObservationRingKeepersPos
-- (boss_yogg_saron.cpp:386-392), indexed by boss DATA slot:
-- [0] Freya, [1] Hodir, [2] Thorim, [3] Mimiron.
local OBSERVATION_RING_KEEPERS = {
    { entry = 33241, pos = { 1945.682, 33.34201, 411.4408, 5.270895 } }, -- Freya
    { entry = 33213, pos = { 1945.761, -81.52171, 411.4407, 1.029744 } }, -- Hodir
    { entry = 33242, pos = { 2028.822, -65.73573, 411.4426, 2.460914 } }, -- Thorim
    { entry = 33244, pos = { 2028.766, 17.42014, 411.4446, 3.857178 } }, -- Mimiron
}
local OBSERVATION_RING_BOSS_DATA = { BOSS_FREYA, BOSS_HODIR, BOSS_THORIM, BOSS_MIMIRON }

local yoggBinds = {
    [ENTRY_VOICE_OF_YOGG_SARON] = "voice",
    [ENTRY_SARA] = "sara",
    [ENTRY_BRAIN_OF_YOGG_SARON] = "brain",
    [ENTRY_FREYA_ACHIEVE_TRIGGER] = "freyaTrigger",
}

RegisterMapEvent(MAP_ULDUAR, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    local key = yoggBinds[creature:GetEntry()]
    if key then
        instanceData[key] = creature
    end
    -- Any-creature probe for the OnUnitDeath player leg (see below):
    -- the most recently created creature in the instance; GetBossState
    -- reads Go instance state off its map/instance, so any motion
    -- works (method closures capture the motion pointer).
    instanceData.anyCreature = creature
    if instanceData.illusion == nil then
        instanceData.illusion = 0
    end
end)

-- ON_UPDATE poll (instance_oculus.lua precedent): the voice object
-- reads boss state for the whole instance (creature:GetBossState,
-- lua_creature_events.go:5936). Once-flags live in instanceData.
RegisterMapEvent(MAP_ULDUAR, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    local voice = instanceData.voice
    if not voice or not voice:IsAlive() then
        return
    end
    if not instanceData.keeperFlags then
        instanceData.keeperFlags = { false, false, false, false }
    end
    for i = 1, 4 do
        if not instanceData.keeperFlags[i] then
            if voice:GetBossState(OBSERVATION_RING_BOSS_DATA[i]) == BOSS_STATE_DONE then
                -- C++ !KeeperGUIDs[i] gate: only mimiron has a Lua
                -- latch (instanceData.mimiron, gossip summon).
                if i ~= 4 or not instanceData.mimiron then
                    instanceData.keeperFlags[i] = true
                    local keeper = OBSERVATION_RING_KEEPERS[i]
                    local pos = keeper.pos
                    voice:SummonCreature(keeper.entry, pos[1], pos[2], pos[3], pos[4], 7, 0)
                end
            end
        end
    end
    if not instanceData.lumberjackedDone then
        if voice:GetBossState(BOSS_BRIGHTLEAF) == BOSS_STATE_DONE
            and voice:GetBossState(BOSS_IRONBRANCH) == BOSS_STATE_DONE
            and voice:GetBossState(BOSS_STONEBARK) == BOSS_STATE_DONE
            and voice:GetBossState(BOSS_FREYA) ~= BOSS_STATE_DONE then
            local trigger = instanceData.freyaTrigger
            if trigger then
                instanceData.lumberjackedDone = true
                trigger:CastSpell(trigger, SPELL_LUMBERJACKED_CREDIT, true)
            end
        end
    end
end)

-- OnUnitDeath (instance_ulduar.cpp:528-574) — PORTED 2026-11-08
-- (tick 467). Player leg (:528-540): a player death while a boss
-- is IN_PROGRESS sets that boss's bit in _CoUAchivePlayerDeathMask,
-- read by the Go Conqueror-of-Ulduar gates via the shared
-- coUAchivePlayerDeathMask key (checkUlduarCriteria). Creature leg
-- (:549-572): the first Freya-trash death starts the Con Speed
-- Atory timed achievement (21597), the first elder death (Ironbranch
-- 32913 / Stonebark 32914 / Brightleaf 32915) starts the Lumberjacked
-- one (21686) — one-shot latches (conSpeedAtory / lumberjacked
-- members). The timed starts ride instanceData:StartTimedAchievement
-- (instance_hooks.go; no-op until Achievement.dbc ships the criteria,
-- the Kologarn 21687 precedent). NOTE: the elder leg's
-- lumberjackedTimedStarted latch is distinct from lumberjackedDone
-- above (the SPELL_LUMBERJACKED_CREDIT spell arm).
local FREYA_TRASH_ENTRIES = {
    [33354] = true, -- NPC_CORRUPTED_SERVITOR (ulduar.h:159)
    [33355] = true, -- NPC_MISGUIDED_NYMPH (:160)
    [33430] = true, -- NPC_GUARDIAN_LASHER (:161)
    [33431] = true, -- NPC_FOREST_SWARMER (:162)
    [33525] = true, -- NPC_MANGROVE_ENT (:163)
    [33526] = true, -- NPC_IRONROOT_LASHER (:164)
    [33527] = true, -- NPC_NATURES_BLADE (:165)
    [33528] = true, -- NPC_GUARDIAN_OF_LIFE (:166)
}
local ELDER_ENTRIES = {
    [32913] = true, -- NPC_IRONBRANCH (ulduar.h:136)
    [32914] = true, -- NPC_STONEBARK (:138)
    [32915] = true, -- NPC_BRIGHTLEAF (:137)
}

RegisterMapEvent(MAP_ULDUAR, INSTANCE_EVENT_ON_UNIT_DEATH, function(event, instanceData, mapObj, unit)
    if unit == nil then
        return
    end
    if unit:GetObjectType() == "Player" then
        local probe = instanceData.anyCreature or instanceData.voice
        if not probe then
            return
        end
        local mask = instanceData.coUAchivePlayerDeathMask or 0
        for i = 0, BOSS_ALGALON - 1 do
            if probe:GetBossState(i) == BOSS_STATE_IN_PROGRESS then
                mask = bit32.bor(mask, bit32.lshift(1, i))
            end
        end
        instanceData.coUAchivePlayerDeathMask = mask
        return
    end
    local entry = unit:GetEntry()
    if FREYA_TRASH_ENTRIES[entry] then
        if not instanceData.conSpeedAtoryStarted then
            instanceData.conSpeedAtoryStarted = true
            instanceData:StartTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT, CRITERIA_CON_SPEED_ATORY)
        end
    elseif ELDER_ENTRIES[entry] then
        if not instanceData.lumberjackedTimedStarted then
            instanceData.lumberjackedTimedStarted = true
            instanceData:StartTimedAchievement(ACHIEVEMENT_TIMED_TYPE_EVENT, CRITERIA_LUMBERJACKED)
        end
    end
end)
