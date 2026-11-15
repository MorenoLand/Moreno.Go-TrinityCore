-- Karazhan: Barnes — Lua port of the WaypointReached arms (+ the
-- Reset arm they depend on) of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp
-- (class npc_barnes : CreatureScript("npc_barnes");
-- struct npc_barnesAI : public EscortAI).
-- Entry: 16812 (TDB 335.26091 creature_template ScriptName
-- npc_barnes). This is the first slice of the Barnes escort-port
-- assembly scoped in karazhan.lua (that file's Barnes bullet still
-- says "awaits the escort-port assembly" — this file is that
-- assembly's ev6 + Reset landing).
-- Eluna creature events: 6 OnReachWP, 23 OnReset.
-- Ported arms (C++ order):
-- - Reset (karazhan.cpp:169-173): m_uiEventId = instance
--   GetData(DATA_OPERA_PERFORMANCE) — the instance ctor rolls it
--   1-3 once per instance, and instance uint32 GetData has no Lua
--   bridge, so the twin is math.random(1, 3) cached per
--   (map, instance) on first Reset (karazhan.lua prescription);
--   cache-if-absent so Go's spawn/evade/death Reset fires never
--   re-roll it. Initialize() locals reset per GUID (TalkCount 0,
--   TalkTimer 2000, WipeTimer 5000, PerformanceReady false).
-- - WaypointReached wp0 (karazhan.cpp:186-190): DoCast(me,
--   SPELL_TUXEDO 32616, false) + stage-door use.
-- - WaypointReached wp4 (:194-208): TalkCount = 0,
--   SetEscortPaused(true), SummonCreature(NPC_SPOTLIGHT 19525, me
--   pos, TIMED_OR_DEAD_DESPAWN 1min) + SetFlag(UNIT_FIELD_FLAGS 59,
--   UNIT_FLAG_NOT_SELECTABLE 0x02000000) + spotlight:CastSpell
--   (spotlight, SPELL_SPOTLIGHT 25824, false).
-- - WaypointReached wp8 (:209-212): stage-door use +
--   PerformanceReady = true.
-- - WaypointReached wp9 (:213-217): PrepareEncounter + curtains
--   use. PrepareEncounter (:241-286): OZ index 0/count 4 (four
--   spawns, indices 0-3), HOOD index 4/count 5, RAJ index 5/count 6
--   — count is the EXCLUSIVE loop bound (for (; index < count)),
--   so HOOD spawns only index 4 (Grandmother) and RAJ only index 5
--   (Julianne); the Lua ranges below are 1-based {start, count}
--   with count 1 for both. Spawns {17535,-10896} Dorothee /
--   {17546,-10891} Roar / {17547,-10884} Tinhead /
--   {17543,-10902} Strawman / {17603,-10892} Grandmother /
--   {17534,-10900} Julianne, SPAWN_Y -1758, SPAWN_Z 90.5,
--   SPAWN_O 4.738, TEMPSUMMON_TIMED_OR_DEAD_DESPAWN 2h, each summon
--   then SetUInt32Value(UNIT_FIELD_FLAGS, 0) + SetFlag
--   NON_ATTACKABLE (0x2, UnitDefines.h:125); RaidWiped = false.
-- - Doors ride GetGuidData + GetGameObjectByGUID + GO
--   UseDoorOrButton (lua_creature_events.go:7593/:4671,
--   lua_objects.go:4688; legoso precedent): DATA_GO_STAGEDOORLEFT
--   19, DATA_GO_CURTAINS 18 (karazhan.h:46-47), nil-guarded.
-- Deliberate omissions / unported scope:
-- - The TalkCount/TalkTimer dialogue machine (UpdateAI
--   :288-310; Oz/Hood/RAJ textids 0-11, timers 5000-18000) — the
--   wp4 pause starts it, but the machine itself is UpdateAI scope.
-- - The gossip driver (OnGossipHello menus 8970/8975/8978,
--   OnGossipSelect StartEvent / GM event override) + StartEvent
--   (instance SetBossState IN_PROGRESS, OZ-deathcount reset) —
--   the escort driver, so this handler fires once that lands.
-- - The PerformanceReady raid-wipe poll (UpdateAI :312-342,
--   EnterEvadeMode on wipe) — UpdateAI scope; the wp8 latch is
--   kept per GUID for it.

local ENTRY_BARNES = 16812

local SPELL_TUXEDO = 32616 -- karazhan.cpp:46
local SPELL_SPOTLIGHT = 25824 -- karazhan.cpp:45
local NPC_SPOTLIGHT = 19525 -- karazhan.cpp:61

local DATA_GO_CURTAINS = 18 -- karazhan.h:46
local DATA_GO_STAGEDOORLEFT = 19 -- karazhan.h:47

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- UnitDefines.h:125

local SUMMON_TIMED_OR_DEAD_DESPAWN = 0
local SUMMON_SPOTLIGHT_MS = 60000
local SUMMON_ENCOUNTER_MS = 7200000 -- 2h

local EVENT_OZ = 1 -- karazhan.h:64
local EVENT_HOOD = 2 -- karazhan.h:65
local EVENT_RAJ = 3 -- karazhan.h:66

local SPAWN_Y = -1758
local SPAWN_Z = 90.5
local SPAWN_O = 4.738

-- Spawns (karazhan.cpp:114-122): {entry, x}.
local SPAWNS = {
    { 17535, -10896 }, -- Dorothee
    { 17546, -10891 }, -- Roar
    { 17547, -10884 }, -- Tinhead
    { 17543, -10902 }, -- Strawman
    { 17603, -10892 }, -- Grandmother
    { 17534, -10900 }, -- Julianne
}

-- PrepareEncounter index/count per event (karazhan.cpp:247-263).
-- C++ counts are exclusive loop bounds, not spawn counts: OZ loops
-- indices 0-3 (4 spawns), HOOD only index 4, RAJ only index 5.
local ENCOUNTER_RANGE = {
    [EVENT_OZ] = { 1, 4 },
    [EVENT_HOOD] = { 5, 1 },
    [EVENT_RAJ] = { 6, 1 },
}

-- Per-instance opera event id (the instance-ctor roll twin) +
-- per-GUID Barnes state (Initialize() locals).
local operaEventByInstance = {}
local barnesState = {}

local function instanceKey(creature)
    return creature:GetMapId() .. ":" .. creature:GetInstanceId()
end

local function useDoor(creature, dataId)
    local guid = creature:GetGuidData(dataId)
    if guid == 0 or guid == nil then
        return
    end
    local go = creature:GetGameObjectByGUID(guid)
    if go then
        go:UseDoorOrButton()
    end
end

-- C++ Reset (karazhan.cpp:169-173) + Initialize() (:154-164).
local function onBarnesReset(event, creature)
    local guid = creature:GetGUIDLow()
    local key = instanceKey(creature)
    if operaEventByInstance[key] == nil then
        operaEventByInstance[key] = math.random(EVENT_OZ, EVENT_RAJ)
    end
    barnesState[guid] = {
        eventId = operaEventByInstance[key],
        talkCount = 0,
        performanceReady = false,
        raidWiped = false,
    }
end

-- C++ PrepareEncounter (karazhan.cpp:241-286).
local function prepareEncounter(creature, eventId)
    local range = ENCOUNTER_RANGE[eventId]
    if not range then
        return
    end
    for i = range[1], range[1] + range[2] - 1 do
        local spawn = SPAWNS[i]
        local summoned = creature:SummonCreature(spawn[1], spawn[2],
            SPAWN_Y, SPAWN_Z, SPAWN_O,
            SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_ENCOUNTER_MS)
        if summoned then
            -- C++ clears the flags first ("in case database has bad
            -- flags"), then sets NON_ATTACKABLE.
            summoned:SetUInt32Value(UNIT_FIELD_FLAGS, 0)
            summoned:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        end
    end
end

-- C++ WaypointReached (karazhan.cpp:184-218).
local function barnesReachWP(event, creature)
    local guid = creature:GetGUIDLow()
    local st = barnesState[guid]
    if not st then
        -- No Reset seen yet (e.g. handler loaded mid-instance):
        -- roll the per-instance event id the same way Reset does.
        onBarnesReset(event, creature)
        st = barnesState[guid]
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:CastSpell(creature, SPELL_TUXEDO)
        useDoor(creature, DATA_GO_STAGEDOORLEFT)
    elseif wp == 4 then
        st.talkCount = 0
        creature:SetEscortPaused(true)
        local spotlight = creature:SummonCreature(NPC_SPOTLIGHT,
            creature:GetX(), creature:GetY(), creature:GetZ(), 0,
            SUMMON_TIMED_OR_DEAD_DESPAWN, SUMMON_SPOTLIGHT_MS)
        if spotlight then
            spotlight:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            spotlight:CastSpell(spotlight, SPELL_SPOTLIGHT)
        end
    elseif wp == 8 then
        useDoor(creature, DATA_GO_STAGEDOORLEFT)
        st.performanceReady = true
    elseif wp == 9 then
        prepareEncounter(creature, st.eventId)
        st.raidWiped = false
        useDoor(creature, DATA_GO_CURTAINS)
    end
end

RegisterCreatureEvent(ENTRY_BARNES, 23, onBarnesReset)
RegisterCreatureEvent(ENTRY_BARNES, 6, barnesReachWP)
