-- Hellfire Peninsula: Colonel Jules — Lua port of
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp
-- (class npc_colonel_jules : CreatureScript("npc_colonel_jules")).
-- Entry: 22432 (C++ ExorcismMisc enum NPC_COLONEL_JULES;
-- wowhead npc=22432/colonel-jules confirms).
-- Eluna creature events: 38 DoAction.
-- Ported arm:
-- - DoAction ACTION_JULES_MOVE_HOME (4)
--   (zone_hellfire_peninsula.cpp:461-467, event 38): SetSpeedRate
--   (MOVE_RUN 1, 1.0) + MoveTo(11, exorcismPos[2] -710.332,
--   2754.394, 102.948). Bridges: DoAction receiver (event 38,
--   lua_creature_events.go:827), SetSpeedRate
--   (lua_creature_events.go:5124, MOVE_RUN=1 only), MoveTo
--   (lua_creature_events.go:4624, id + x/y/z).
-- Unmodeled (documented-only): the C++ arm also clears the
-- member bool wpreached and cancels EVENT_SUMMON_SKULL — both
-- are vacuous here (no Jules timers exist in this file; the skull
-- summon machine is unported). The ACTION_JULES_HOVER (2) and
-- ACTION_JULES_FLIGHT (3) arms stay out (AddAura / SetCanFly /
-- MoveJump / waypoint-chain legs, no bridges).

local ENTRY_COLONEL_JULES = 22432

local ACTION_JULES_MOVE_HOME = 4

local MOVE_RUN = 1

-- C++ DoAction case ACTION_JULES_MOVE_HOME: run-speed reset +
-- move back to the exorcism bed position.
local function onDoAction(event, creature, action)
    if action ~= ACTION_JULES_MOVE_HOME then
        return
    end
    creature:SetSpeedRate(MOVE_RUN, 1.0)
    creature:MoveTo(11, -710.332, 2754.394, 102.948)
end

RegisterCreatureEvent(ENTRY_COLONEL_JULES, 38, onDoAction)
