-- Guardian of Blizzard (non-attackable guardian) --
-- Lua port of src/server/scripts/World/npcs_special.cpp:1126-1160
-- (npc_guardianAI). Entry 5764 TDB-verified (ScriptName npc_guardian).
-- Eluna creature events: 5 OnSpawn, 23 OnReset.
-- Ported arm (C++-exact): Reset (:1126) — SetFlag(UNIT_FIELD_FLAGS
-- 59, UNIT_FLAG_NON_ATTACKABLE 0x2). That is the whole port; the
-- empty JustEngagedWith (:1131) registers nothing and UpdateAI
-- (:1135) stays out — isAttackReady / resetAttackTimer have no
-- bridge (a 1s pump would cast on the wrong cadence).

local ENTRY_GUARDIAN = 5764

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

local function onSpawnOrReset(_, creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
end

RegisterCreatureEvent(ENTRY_GUARDIAN, 5, onSpawnOrReset)
RegisterCreatureEvent(ENTRY_GUARDIAN, 23, onSpawnOrReset)
