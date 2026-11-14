-- npc_a_special_surprise (Scarlet Enclave, chapter2.cpp) --
-- Lua port of the npc_a_special_surprise struct in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter2.cpp
-- (lines 447-613).
-- Entries (verifiable from the C++ sources): chapter2.cpp:446
-- "used by 29032, 29061, 29065, 29067, 29068, 29070, 29074,
-- 29072, 29073, 29071 but signed for 29032".
-- Ported arms:
-- - Reset (chapter2.cpp:475-479): Initialize() + SetImmuneToPC
--   (true). Initialize zeroes the speech timers/counter and
--   clears PlayerGUID; the Lua file holds no such state (the
--   Talk machine below is unported), so that half is vacuous.
--   SetImmuneToPC(true) is C++ Unit::SetImmuneToPC
--   (Unit.cpp:8453): SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_IMMUNE_TO_PC 0x100) — the flag twin is real:
--   creature:SetFlag(59, 0x100) mutates motion.UnitFlags
--   (lua_creature_events.go:6811), which the engine reads in
--   the threat/aggro gates (threat.go, creaturemotion.go) with
--   the same 0x100 value (player_state.go:151). The combat-ref
--   ending half (Unit.cpp:8460-8468) has no Lua twin, but on
--   Reset there are no live PC combat refs to end (spawn has
--   none; evade already ended combat), so the flag set is the
--   whole observable leg. Go fires ev23 on spawn, evade and
--   death, matching the C++ Reset sites.
-- Unmodeled (documented-only, no bridges):
-- - MoveInLineOfSight quest-status trigger + 12-step Talk
--   machine (chapter2.cpp:480-613): the trigger needs a
--   quest-status check on sighting (ev27 fires only from the
--   aggro-engage branch); the machine needs stand-state /
--   setDeathState / Plaguefist Talk legs with no bridges.
-- - JustEngagedWith / UpdateAI speech-timer arms ride the same
--   unported machine.
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117 (OBJECT_END 0x6 + 0x35)
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132

local ENTRIES = {
    29032, 29061, 29065, 29067, 29068,
    29070, 29074, 29072, 29073, 29071,
}

-- C++ npc_a_special_surpriseAI::Reset (chapter2.cpp:475-479).
local function onReset(_, creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
end

for _, entry in ipairs(ENTRIES) do
    RegisterCreatureEvent(entry, 23, onReset)
end
