-- Frost Freeze Trap (Icecrown Citadel coldflame-jets trap) — Lua port of
-- npc_frost_freeze_trap (src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp:512-545).
-- Entry: NPC_FROST_FREEZE_TRAP 37744 (icecrown_citadel.h:219).
-- Ported: DoAction(1000/11000) -> EVENT_ACTIVATE_TRAP in 1s/11s, then
-- DoCastSelf(SPELL_COLDFLAME_JETS 70460, non-triggered) with a 22s re-arm
-- (icecrown_citadel.cpp:517-544). SetCombatMovement(false) rides Reset
-- (lua_creature_events.go:4410; constructor leg, :515-516).
-- LIVE since 2026-11-12: the only DoAction caller is the area trigger
-- (icecrown_citadel.cpp:1540-1542), ported in at_icc_saurfang_portal.lua
-- (trigger 5698, TDB-verified); the ev38 handler below fires from any
-- creature:DoAction call on the trap entry.

local NPC_FROST_FREEZE_TRAP = 37744

local SPELL_COLDFLAME_JETS = 70460 -- icecrown_citadel.cpp:85

local timers = {}

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
    per[key] = CreateLuaEvent(fn, delay, 0)
end

-- C++ npc_frost_freeze_trap::UpdateAI (icecrown_citadel.cpp:535-544):
-- EVENT_ACTIVATE_TRAP casts coldflame jets and re-arms after 22s.
local function activateTrap(creature)
    if not creature:IsAlive() then
        return
    end
    creature:CastSpell(creature, SPELL_COLDFLAME_JETS)
    schedule(creature:GetGUIDLow(), "activate", 22000, function()
        activateTrap(creature)
    end)
end

-- C++ npc_frost_freeze_trap::DoAction (icecrown_citadel.cpp:517-528):
-- action 1000/11000 schedules the activation after that many ms.
local function frostFreezeTrapDoAction(event, creature, action)
    if action == 1000 or action == 11000 then
        schedule(creature:GetGUIDLow(), "activate", action, function()
            activateTrap(creature)
        end)
    end
end

local function frostFreezeTrapReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetCombatMovement(false)
end

RegisterCreatureEvent(NPC_FROST_FREEZE_TRAP, 38, frostFreezeTrapDoAction)
RegisterCreatureEvent(NPC_FROST_FREEZE_TRAP, 5, frostFreezeTrapReset)
RegisterCreatureEvent(NPC_FROST_FREEZE_TRAP, 23, frostFreezeTrapReset)
