-- Blackwing Lair instance script — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- instance_blackwing_lair.cpp (InstanceMapScript
-- "instance_blackwing_lair", map 469) and blackwing_lair.h
-- (BWLEncounter, BWLEvents, BWLMisc). RegisterMapEvent binds the
-- map-level Eluna instance hooks (gruul / onyxia precedent).
--
-- PORTED — the Razorgore egg machine: SetData(DATA_EGG_EVENT = 2)
-- switch on INSTANCE_EVENT_ON_SET_DATA (8, Go-only; fired by every
-- instanceData:SetData write, including the boss file's Reset/JustDied
-- NOT_STARTED writes and the egg-destroy spell's SPECIAL writes),
-- the EVENT_RAZOR_SPAWN / EVENT_RAZOR_PHASE_TWO event machine on
-- INSTANCE_EVENT_ON_UPDATE (3, diff in ms), and the
-- spell_egg_event SpellScript (boss_razorgore.cpp) as
-- RegisterSpellEvent(19873, 2) OnHit -> SetData(DATA_EGG_EVENT,
-- SPECIAL). Timers run as an ev3 diff accumulator (the C++ EventMap
-- Update(diff) shape): spawn waves 45s after IN_PROGRESS then every
-- {12s,17s}, phase two 1s after the 15th counted egg.
--
-- C++-exact quirks kept: the first SPECIAL starts the event and its
-- nested SetData(IN_PROGRESS) resets EggCount to 0, so phase two
-- needs 16 destroyed eggs (1 to start + 15 counted); SetData(DONE)
-- has no switch case in C++ and is a no-op (EggEvent stays
-- IN_PROGRESS); the spawn wave summons urand(2,5) adds from Entry[5]
-- at random SummonPosition[8] slots, each DoZoneInCombat. The
-- phase-two event cancels the spawn timer again before sending
-- DoAction(ACTION_PHASE_TWO = 1) to Razorgore (ev38 dispatch live,
-- lua_creature_events.go:802; boss_razorgore.lua doChangePhase).
--
-- The Razorgore ref is latched on INSTANCE_EVENT_ON_CREATURE_CREATE
-- (5) for entry 12435 and lazily resolved via players'
-- FindNearestCreature (gruul findGO idiom) because static spawns
-- never fire ev5 in Go (instance_hooks.go:553-560) — the live
-- Razorgore is a static spawn. The cached ref is re-resolved if it
-- stops being alive. Summoning rides the latched Razorgore
-- (creature:SummonCreature, default type 7 = C++ Map::SummonCreature
-- manual-despawn default); there is no map:SummonCreature bridge.
--
-- STAY-OUTS: SetBossState override arms (no boss-state-change event
-- in Go — gruul precedent): Razorgore DONE egg phase-masking +
-- SetData NOT_STARTED (the boss ev4 already writes the SetData leg),
-- Nefarian NOT_STARTED despawn + FAIL 15min respawn
-- (EVENT_RESPAWN_NEFARIUS never schedules). OnGameObjectCreate egg
-- arm / EggList: the list is only consumed by the unportable DONE
-- arm, so it is not built. OnUnitDeath Razorgore DONE arm: the boss
-- ev4 already writes SetBossState DONE. CheckRequiredBosses gating:
-- no bridge. Door data (8 portcullis rows): needs boss-written
-- instanceData flags in all 8 boss files (gruul pattern) — separate
-- unit, not this one. The SPECIAL arm's
-- razor->RemoveAurasDueToSpell(42013) + DoRemoveAurasDueToSpellOnPlayers
-- (42013) legs have no bridge (creature RemoveAurasDueToSpell is
-- unbridged; no instance player-aura removal) — the phase-two timer
-- still schedules. The OnCreatureCreate blackwing-add legs
-- (12422/12458/12416/12459 -> razorAI->JustSummoned) are ported as
-- the razorSummons registry (wave adds recorded at summon time,
-- dynamic creations on ev5): Razorgore's _Reset/_JustDied
-- DespawnAll runs over it (boss_razorgore.lua). Residual: the
-- initial static trash of those entries never fires ev5 in Go, so
-- a wipe leaves it standing where C++ despawns it (no
-- find-all-by-entry bridge); the wave-mage entry 12420 is not in
-- the C++ switch and is never recorded, by design. The orb gossip
-- (go_orb_of_domination) stays out: GO entry is DB-only and
-- instance GetData has no bridge.

local MAP_BLACKWING_LAIR = 469

local INSTANCE_EVENT_ON_UPDATE = 3
local INSTANCE_EVENT_ON_CREATURE_CREATE = 5
local INSTANCE_EVENT_ON_SET_DATA = 8

-- blackwing_lair.h BWLMisc: ACTION_PHASE_TWO = 1, DATA_EGG_EVENT = 2.
local ACTION_PHASE_TWO = 1
local DATA_EGG_EVENT = 2

local NPC_RAZORGORE = 12435
local SPELL_EGG_DESTROY = 19873

-- EncounterState (InstanceScript.h): NOT_STARTED 0, IN_PROGRESS 1,
-- SPECIAL 4.
local ENCOUNTER_NOT_STARTED = 0
local ENCOUNTER_IN_PROGRESS = 1
local ENCOUNTER_SPECIAL = 4

-- instance_blackwing_lair.cpp: Entry[5] summon table (the mage
-- 12420 is summon-table-only; it is not a BWLCreatureIds entry).
local ENTRY = {12422, 12458, 12416, 12420, 12459}

-- instance_blackwing_lair.cpp: SummonPosition[8].
local SUMMON_POSITION = {
    {-7661.207520, -1043.268188, 407.199554, 6.280452},
    {-7644.145020, -1065.628052, 407.204956, 0.501492},
    {-7624.260742, -1095.196899, 407.205017, 0.544694},
    {-7608.501953, -1116.077271, 407.199921, 0.816443},
    {-7531.841797, -1063.765381, 407.199615, 2.874187},
    {-7547.319336, -1040.971924, 407.205078, 3.789175},
    {-7568.547852, -1013.112488, 407.204926, 3.773467},
    {-7584.175781, -989.6691289, 407.199585, 4.527447},
}

-- Per-instance state lives on the shared instanceData table:
-- eggEvent (0/1, C++ EggEvent member), eggCount (C++ EggCount),
-- razorSpawnMs / razorPhaseTwoMs (C++ _events timers, ms, nil when
-- idle), razor (latched Razorgore creature, onyxia precedent),
-- razorSummons (guidLow set for BossAI::JustSummoned — see below).

-- instance_blackwing_lair.cpp OnCreatureCreate: the four blackwing
-- entries JustSummoned into Razorgore's summons list (the mage 12420
-- is NOT in the C++ switch, so wave mages are never recorded).
local SUMMONED_ENTRIES = { [12422] = true, [12458] = true, [12416] = true, [12459] = true }

local function recordSummon(instanceData, creature)
    local registry = instanceData.razorSummons
    if not registry then
        registry = {}
        instanceData.razorSummons = registry
    end
    registry[creature:GetGUIDLow()] = true
end

local function razorOf(instanceData, mapObj)
    local razor = instanceData.razor
    if razor then
        if razor:IsAlive() then
            return razor
        end
        instanceData.razor = nil
    end
    if not mapObj then
        return nil
    end
    local players = mapObj:GetPlayers()
    if not players then
        return nil
    end
    for _, player in ipairs(players) do
        local found = player:FindNearestCreature(NPC_RAZORGORE, 1000)
        if found then
            instanceData.razor = found
            return found
        end
    end
    return nil
end

-- C++ EVENT_RAZOR_SPAWN (instance_blackwing_lair.cpp Update):
-- urand(2,5) adds from Entry[urand(0,4)] at SummonPosition[urand(0,7)],
-- each DoZoneInCombat; re-arms {12s,17s} (caller).
local function spawnWave(instanceData, mapObj)
    local razor = razorOf(instanceData, mapObj)
    if not razor then
        return
    end
    for _ = 1, math.random(2, 5) do
        local entry = ENTRY[math.random(1, 5)]
        local pos = SUMMON_POSITION[math.random(1, 8)]
        local summoned = razor:SummonCreature(entry,
            pos[1], pos[2], pos[3], pos[4])
        if summoned then
            summoned:DoZoneInCombat()
            -- C++ OnCreatureCreate JustSummoned leg (see recordSummon):
            -- wave adds of the four entries join the summons list; the
            -- mage (12420) is not in the C++ switch and is not recorded.
            if SUMMONED_ENTRIES[entry] then
                recordSummon(instanceData, summoned)
            end
        end
    end
end

-- C++ SetData(DATA_EGG_EVENT) switch (instance_blackwing_lair.cpp):
-- IN_PROGRESS arms the spawn timer (45s) and zeroes the count;
-- NOT_STARTED cancels the spawn timer and zeroes the count;
-- SPECIAL counts a destroyed egg and, at 15, schedules phase two
-- (1s) and cancels the spawn timer, then — while the event has not
-- started — nests the IN_PROGRESS leg, which zeroes the count
-- again. The nested leg is called directly (same effect as the C++
-- nested SetData call, without a re-entrant ev8 dispatch). The
-- C++ SetData(DONE) inside the 15-egg arm has no switch case and
-- is a no-op, so it is not written; the RemoveAurasDueToSpell
-- (42013) legs stay out (no bridge — see header).
local function onSetData(event, instanceData, mapObj, type, data)
    if type ~= DATA_EGG_EVENT then
        return
    end
    local eggEvent = instanceData.eggEvent or ENCOUNTER_NOT_STARTED
    if data == ENCOUNTER_IN_PROGRESS then
        instanceData.razorSpawnMs = 45000
        instanceData.eggEvent = ENCOUNTER_IN_PROGRESS
        instanceData.eggCount = 0
    elseif data == ENCOUNTER_NOT_STARTED then
        instanceData.razorSpawnMs = nil
        instanceData.eggEvent = ENCOUNTER_NOT_STARTED
        instanceData.eggCount = 0
    elseif data == ENCOUNTER_SPECIAL then
        instanceData.eggCount = (instanceData.eggCount or 0) + 1
        if instanceData.eggCount == 15 then
            instanceData.razorPhaseTwoMs = 1000
            instanceData.razorSpawnMs = nil
        end
        if eggEvent == ENCOUNTER_NOT_STARTED then
            instanceData.razorSpawnMs = 45000
            instanceData.eggEvent = ENCOUNTER_IN_PROGRESS
            instanceData.eggCount = 0
        end
    end
end

-- C++ Update (instance_blackwing_lair.cpp): the EventMap tick.
-- EVENT_RAZOR_SPAWN re-arms {12s,17s}; EVENT_RAZOR_PHASE_TWO
-- cancels the spawn timer and sends DoAction(ACTION_PHASE_TWO)
-- to Razorgore. The Nefarian respawn leg never schedules (its
-- SetBossState FAIL sender is unported — see header).
RegisterMapEvent(MAP_BLACKWING_LAIR, INSTANCE_EVENT_ON_UPDATE, function(event, instanceData, mapObj, diff)
    local spawnMs = instanceData.razorSpawnMs
    if spawnMs then
        spawnMs = spawnMs - diff
        if spawnMs <= 0 then
            spawnWave(instanceData, mapObj)
            spawnMs = math.random(12000, 17000)
        end
        instanceData.razorSpawnMs = spawnMs
    end
    local phaseMs = instanceData.razorPhaseTwoMs
    if phaseMs then
        phaseMs = phaseMs - diff
        if phaseMs <= 0 then
            instanceData.razorPhaseTwoMs = nil
            instanceData.razorSpawnMs = nil
            local razor = razorOf(instanceData, mapObj)
            if razor then
                razor:DoAction(ACTION_PHASE_TWO)
            end
        else
            instanceData.razorPhaseTwoMs = phaseMs
        end
    end
end)

-- C++ OnCreatureCreate (instance_blackwing_lair.cpp): latch
-- Razorgore (12435), and record the four blackwing-add entries into
-- the summons registry (BossAI::JustSummoned, ScriptedCreature.cpp:
-- 559-561) — dynamic creations only; the initial static trash never
-- fires ev5 in Go, so it is not recorded (see header).
RegisterMapEvent(MAP_BLACKWING_LAIR, INSTANCE_EVENT_ON_CREATURE_CREATE, function(event, instanceData, mapObj, creature)
    if creature:GetEntry() == NPC_RAZORGORE then
        instanceData.razor = creature
    end
    if SUMMONED_ENTRIES[creature:GetEntry()] then
        recordSummon(instanceData, creature)
    end
end)

RegisterMapEvent(MAP_BLACKWING_LAIR, INSTANCE_EVENT_ON_SET_DATA, onSetData)

-- C++ spell_egg_event SpellScript (boss_razorgore.cpp): OnHit ->
-- instance->SetData(DATA_EGG_EVENT, SPECIAL). The hit target is
-- the egg GO; the caster (mind-controlled Razorgore, or the
-- controlling player) reaches the instance data via its map.
RegisterSpellEvent(SPELL_EGG_DESTROY, 2, function(event, spell, hitTarget)
    local caster = spell:GetCaster()
    if not caster then
        return
    end
    local mapObj = caster:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data:SetData(DATA_EGG_EVENT, ENCOUNTER_SPECIAL)
    end
end)
