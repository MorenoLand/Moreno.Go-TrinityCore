-- Tribunal of the Ages controller (Halls of Stone) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/halls_of_stone.cpp
-- (npc_tribuna_controller (CreatureScript) via
-- GetHallsOfStoneAI<npc_tribuna_controllerAI> (ScriptedAI)).
-- Entry: 28234 Tribunal of the Ages (halls_of_stone.cpp:92
-- NPC_TRIBUNAL_OF_THE_AGES) — entry-verifiable, registered in
-- engine/world/boss_ai.go (the npc_arcane_overload precedent).
-- Eluna creature events: 4 OnDied, 5 OnSpawn, 23 OnReset, 38 OnDoAction.
-- Driven by CreateLuaEvent timers; the controller never enters combat
-- (ctor SetCombatMovement(false), ported on ev5).
-- Ported arms (C++-exact for all modeled arms):
-- UpdateAI Marnak loop (halls_of_stone.cpp:228-242): gated on
-- bMarnakActivated, 10s init (Initialize :150) / urand(30s,31s) loop:
-- SelectTarget(Random, 0, 100, true) -> SummonCreature
-- (NPC_DARK_MATTER_TARGET 28237 at the target's position,
-- TEMPSUMMON_TIMED_DESPAWN, 1s) + SetDisplayId(11686) +
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE 0x2) +
-- the summon CastSpell(target, SPELL_DARK_MATTER 51012, triggered).
-- The re-arm is unconditional (outside the target-if). The target pool
-- is GetPlayersInRange(100) (alive-only per the gothik default) — the
-- controller has no threat list, so the threat-pool twin does not apply.
-- UpdateAI Abedneum loop (:244-258): same shape, NPC_SEARING_GAZE_TARGET
-- 28265 + SPELL_SEARING_GAZE 51136.
-- Activation protocol (Lua-side, no C++ DoAction exists): Brann's escort
-- steps set the C++ bools directly (ENSURE_AI, :455/:477/:507); the Lua
-- twin is ev38 DoAction — action 1 Kaddrak, 2 Marnak, 3 Abedneum — driven
-- by the escort machine (next unit) via controller:DoAction(n). Each
-- activation seeds its 10s first-fire timer once (idempotent guard).
-- DELIBERATELY OMITTED — Kaddrak glare arm (:211-226): dead in C++.
-- KaddrakGUIDList is declared (:169), cleared in Reset (:180), and read
-- (:216), but never populated — its only fill site is inside the
-- commented-out UpdateFacesList (:185-208). Porting it would fabricate
-- casts C++ never performs; the activation flag is still accepted and
-- stored so the escort step stays faithful.
-- Reset (halls_of_stone.cpp:173-182): Initialize() (timers 1500/10000/
-- 10000, flags false) + HandleGameObject(DATA_GO_KADDRAK/MARNAK/
-- ABEDNEUM/SKY_FLOOR, false) — the GO legs are VACUOUS in Lua: the
-- instance file latches no GO GuidData (event 6 is dynamic-only, so the
-- static face/sky-floor GOs never land in the store) and GetGuidData
-- returns empty on both sides; documented, not fabricated.
-- SetCombatMovement(false) is the ctor leg (lua_creature_events.go:4410).

local ENTRY_TRIBUNA_CONTROLLER = 28234

local NPC_DARK_MATTER_TARGET = 28237
local NPC_SEARING_GAZE_TARGET = 28265

local SPELL_DARK_MATTER = 51012
local SPELL_SEARING_GAZE = 51136

-- Lua-side ev38 activation protocol (no C++ DoAction exists; Brann's
-- escort steps 8/12/18 drive these via controller:DoAction(n)).
local ACTION_ACTIVATE_KADDRAK = 1
local ACTION_ACTIVATE_MARNAK = 2
local ACTION_ACTIVATE_ABEDNEUM = 3

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

-- TEMPSUMMON_TIMED_DESPAWN = Go type 2 (creaturemotion.go
-- TempSummonType map).
local SUMMON_TYPE_TIMED_DESPAWN = 2

-- CreateLuaEvent timer harness (GUIDLow-keyed timers table, cancel on
-- events 4/23) — the maiden / bjarngrim-lieutenant convention.
local timers = {}

-- Per-controller activation state, keyed by GetGUIDLow() (never by
-- GetGUID() userdata — cross-handler lookups miss; the GUID-key fix
-- class). Kaddrak's flag is stored but drives no tick (dead arm, see
-- header); Marnak/Abedneum flags gate their loops.
local activated = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ SelectTarget(Random, 0, 100, true): random alive player within
-- 100yd (the controller has no threat list; GetPlayersInRange is
-- alive-only per the gothik default).
local function randomPlayerInRange(creature)
    local players = creature:GetPlayersInRange(100)
    if not players or #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ UpdateAI Marnak loop (halls_of_stone.cpp:228-242): summon a dark
-- matter target on a random player + triggered 51012, {30s,31s} re-arm.
local function marnakTick(creature, guid)
    local target = randomPlayerInRange(creature)
    if target then
        local summon = creature:SummonCreature(NPC_DARK_MATTER_TARGET,
            target:GetX(), target:GetY(), target:GetZ(), 0.0,
            SUMMON_TYPE_TIMED_DESPAWN, 1000)
        if summon then
            summon:SetDisplayId(11686)
            summon:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            summon:CastSpell(target, SPELL_DARK_MATTER, true)
        end
    end
    schedule(guid, "marnak", {30000, 31000}, function()
        marnakTick(creature, guid)
    end)
end

-- C++ UpdateAI Abedneum loop (halls_of_stone.cpp:244-258): same shape,
-- searing gaze target + triggered 51136, {30s,31s} re-arm.
local function abedneumTick(creature, guid)
    local target = randomPlayerInRange(creature)
    if target then
        local summon = creature:SummonCreature(NPC_SEARING_GAZE_TARGET,
            target:GetX(), target:GetY(), target:GetZ(), 0.0,
            SUMMON_TYPE_TIMED_DESPAWN, 1000)
        if summon then
            summon:SetDisplayId(11686)
            summon:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            summon:CastSpell(target, SPELL_SEARING_GAZE, true)
        end
    end
    schedule(guid, "abedneum", {30000, 31000}, function()
        abedneumTick(creature, guid)
    end)
end

-- C++ ctor (halls_of_stone.cpp:141-147): Initialize() +
-- SetCombatMovement(false).
local function controllerSpawned(event, creature)
    creature:SetCombatMovement(false)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    activated[guid] = nil
end

local function controllerDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    activated[guid] = nil
end

-- C++ Reset (halls_of_stone.cpp:173-182): Initialize() — timers back
-- to 1500/10000/10000, flags false. The HandleGameObject face/sky-floor
-- legs are vacuous in Lua (see header) and are not fabricated.
local function controllerReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    activated[guid] = nil
end

-- Lua-side activation protocol (see header): Brann's escort steps
-- 8/12/18 drive these. First-fire 10s per C++ Initialize; idempotent
-- (a second activation while the loop runs must not double-schedule).
local function controllerDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    local st = activated[guid]
    if not st then
        st = {}
        activated[guid] = st
    end
    if action == ACTION_ACTIVATE_KADDRAK then
        -- Dead arm in C++ (list never populated): flag stored, no tick.
        st.kaddrak = true
    elseif action == ACTION_ACTIVATE_MARNAK then
        if not st.marnak then
            st.marnak = true
            schedule(guid, "marnak", 10000, function()
                marnakTick(creature, guid)
            end)
        end
    elseif action == ACTION_ACTIVATE_ABEDNEUM then
        if not st.abedneum then
            st.abedneum = true
            schedule(guid, "abedneum", 10000, function()
                abedneumTick(creature, guid)
            end)
        end
    end
end

RegisterCreatureEvent(ENTRY_TRIBUNA_CONTROLLER, 4, controllerDied)
RegisterCreatureEvent(ENTRY_TRIBUNA_CONTROLLER, 5, controllerSpawned)
RegisterCreatureEvent(ENTRY_TRIBUNA_CONTROLLER, 23, controllerReset)
RegisterCreatureEvent(ENTRY_TRIBUNA_CONTROLLER, 38, controllerDoAction)
