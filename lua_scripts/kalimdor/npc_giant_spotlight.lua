-- npc_giant_spotlight (Moonglade) — Lua port of npc_giant_spotlightAI
-- from src/server/scripts/Kalimdor/zone_moonglade.cpp (:496-532,
-- constants :398-410). Omen's JustDied self-cast of
-- SPELL_OMEN_SUMMON_SPOTLIGHT 26392 (Omen's Moonlight) summons
-- this creature; the entry is DB-side (no TDB in this workspace)
-- and is Wowhead-verified: spell 26392 Effect #1 summons
-- Giant Spotlight, NPC 15902.
-- Event 23 fires on spawn and on leave-combat in Go (matching
-- the C++ Reset call sites) and also on death, so the handler
-- guards on IsAlive (the prowlerReset precedent): C++ Reset
-- never runs on a corpse, and the spotlight has no JustDied
-- arm to twin. The C++ Reset schedules EVENT_DESPAWN at 5min
-- and the UpdateAI fires it once (no re-schedule), so the port
-- is a one-shot 300000ms CreateLuaEvent (2-arg form = one
-- repeat, akama idiom), GUID-keyed, cancelled on events 2/4.
-- C++ UpdateAI arm order (:516-528): FindNearestGameObject
-- GO_ELUNE_TRAP_1 180876 -> RemoveFromWorld, then
-- GO_ELUNE_TRAP_2 180877 -> RemoveFromWorld, then
-- FindNearestCreature(NPC_OMEN 15467) -> DespawnOrUnsummon,
-- then self DespawnOrUnsummon.
-- Deliberate deviations: C++ FindNearestCreature passes
-- alive=false, so a dead Omen within 5yd is also swept; the
-- Lua bridge returns live creatures only, so a dead Omen's
-- corpse is left to its own corpse timer. C++ Reset also runs
-- on evade; the spotlight never enters combat, so the
-- event-2 cancel is vacuous and the event-23 re-arm covers
-- the spawn case. GO RemoveFromWorld twins as the in-tree
-- GO:RemoveFromWorld hide-in-instance (lua_objects.go:1407).
-- The spotlight's own UpdateAI has no other arms; the
-- constructor sets no state.

local ENTRY_GIANT_SPOTLIGHT = 15902
local ENTRY_OMEN = 15467
local GO_ELUNE_TRAP_1 = 180876
local GO_ELUNE_TRAP_2 = 180877
local DESPAWN_DELAY = 300000

local timers = {}

local function cancelTimer(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function despawnSweep(creature, guid)
    timers[guid] = nil
    local trap = creature:FindNearestGameObject(GO_ELUNE_TRAP_1, 5.0)
    if trap ~= nil then
        trap:RemoveFromWorld()
    end
    trap = creature:FindNearestGameObject(GO_ELUNE_TRAP_2, 5.0)
    if trap ~= nil then
        trap:RemoveFromWorld()
    end
    local omen = creature:FindNearestCreature(ENTRY_OMEN, 5.0)
    if omen ~= nil then
        omen:Despawn()
    end
    creature:Despawn()
end

local function spotlightReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimer(guid)
    if not creature:IsAlive() then
        return
    end
    timers[guid] = CreateLuaEvent(function()
        despawnSweep(creature, guid)
    end, DESPAWN_DELAY)
end

local function spotlightLeaveCombat(_, creature)
    cancelTimer(creature:GetGUID())
end

local function spotlightDied(_, creature)
    cancelTimer(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GIANT_SPOTLIGHT, 23, spotlightReset)
RegisterCreatureEvent(ENTRY_GIANT_SPOTLIGHT, 2, spotlightLeaveCombat)
RegisterCreatureEvent(ENTRY_GIANT_SPOTLIGHT, 4, spotlightDied)
