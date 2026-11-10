-- Shadowmoon Valley: Torloth the Magnificent — Lua port of
-- src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (class npc_torloth_the_magnificent :
-- CreatureScript("npc_torloth_the_magnificent")).
-- Entry: 22076 (wowhead npc=22076/torloth-the-magnificent,
-- verified 2026-11-07; "Torloth the Magnificent <The Crimson
-- Sigil>", Shadowmoon Valley; no NPC_ constant in the C++ file).
-- Eluna creature events: 4 JustDied, 23 Reset.
-- Ported arms:
-- - Reset (zone_shadowmoon_valley.cpp:772-779, event 23):
--   AddUnitState(UNIT_STATE_ROOT) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_NON_ATTACKABLE) + SetTarget(Empty); Initialize()'s
--   GUID-latch/Timers clears have no Lua bearer (cinematic machine
--   stays out).
-- - JustDied (zone_shadowmoon_valley.cpp:875-893, event 4): nil
--   killer returns; TYPEID_PLAYER (4) killer ->
--   killer:GroupEventHappens(10781, creature); TYPEID_UNIT (3)
--   killer -> killer:GetOwner() -> owner
--   GroupEventHappens(10781, creature). Bridges: ev4 killer 3rd
--   arg (lua_creature_events.go:3108), GetTypeId (object.go:160),
--   GetOwner (lua_creature_events.go:627, returns the owning
--   player object or nil), GroupEventHappens (lua_player.go:4511).
-- - JustDied :895-896: LordIllidan->AI()->EnterEvadeMode() —
--   PORTED via the EnterEvadeMode bridge (lua_creature_events.go,
--   triggerCreatureEvade twin). The LordIllidanGUID latch is
--   replaced by a live FindNearestCreature(22083, 50.0) — entry
--   22083 from zone_shadowmoon_valley.cpp:1262 (the file's own
--   Illidan lookup radius), nil-guarded like the C++ GetCreature
--   check; the 50.0 radius matches that lookup.

local ENTRY_TORLOTH = 22076
local ENTRY_LORD_ILLIDAN = 22083

local QUEST_BATTLE_OF_THE_CRIMSON_WATCH = 10781

local TYPEID_UNIT = 3
local TYPEID_PLAYER = 4

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- UnitDefines.h:125
local UNIT_STATE_ROOT = 1024 -- Unit.h:221

-- C++ Reset (zone_shadowmoon_valley.cpp:772-779): Initialize() +
-- AddUnitState(UNIT_STATE_ROOT) + SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NON_ATTACKABLE) + SetTarget(Empty). Initialize() only
-- clears the cinematic machine's GUID latches + Timers flag, which
-- have no Lua bearer (the TorlothAnim machine stays out); the three
-- bridged legs port here (AddUnitState lua_creature_events.go:2291,
-- ROOT bit sets the ScriptRooted gate).
local function onReset(event, creature)
    creature:AddUnitState(UNIT_STATE_ROOT)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:SetTarget()
end

-- C++ JustDied leg (a): quest-10781 group credit via the killer
-- (player) or the killer's owner (pet/guardian); leg (b) below
-- runs after it, in C++ order (:895-896).
local function onDied(event, creature, killer)
    if not killer then
        return
    end
    if killer:GetTypeId() == TYPEID_PLAYER then
        killer:GroupEventHappens(QUEST_BATTLE_OF_THE_CRIMSON_WATCH, creature)
    elseif killer:GetTypeId() == TYPEID_UNIT then
        local owner = killer:GetOwner()
        if owner then
            owner:GroupEventHappens(QUEST_BATTLE_OF_THE_CRIMSON_WATCH, creature)
        end
    end
    -- C++ JustDied leg (b): LordIllidan->AI()->EnterEvadeMode()
    -- — the GUID latch is a live nearest-creature lookup (see
    -- header); nil-guarded like the C++ GetCreature check.
    local illidan = creature:FindNearestCreature(ENTRY_LORD_ILLIDAN, 50.0)
    if illidan then
        illidan:EnterEvadeMode()
    end
end

RegisterCreatureEvent(ENTRY_TORLOTH, 4, onDied)
RegisterCreatureEvent(ENTRY_TORLOTH, 23, onReset)
