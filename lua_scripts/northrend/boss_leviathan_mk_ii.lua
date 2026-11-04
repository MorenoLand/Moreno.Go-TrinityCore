-- Leviathan Mk II (Ulduar, Mimiron phase 1) -- Lua port of one
-- engine-fired arm of boss_leviathan_mk_iiAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_LEVIATHAN_MKII = 33432.
-- Only the arm below is ported: the combat arms (napalm shell,
-- plasma blast, shock blast, proximity mine, flame suppressant)
-- are scheduled only by DoAction(DO_START_MKII /
-- DO_ASSEMBLED_COMBAT), which has no cross-creature dispatch
-- bridge, and the DamageTaken lethal clamp is phase-gated
-- (phase-1 turret kill + MovePoint to idle vs phase-4 stand-dead
-- + self-repair) behind the same unbridged phase machine plus
-- the vehicle-kit and MovePoint legs. The GetData/SetData
-- setup-mine/bomb/rocket latches are consumed only by the
-- unported spell scripts. KilledUnit talks into the unported
-- boss_mimiron controller; MovementInform drives the same
-- cross-creature activation chain.

local NPC_LEVIATHAN_MKII = 33432

local SPELL_FREEZE_ANIM = 16245

-- UnitDefines.h:125/149; SetFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:5056).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local REACT_PASSIVE = 0

-- C++ boss_leviathan_mk_iiAI::Reset (:827-837): BossAI::_Reset
-- (instance boss-state reset) has no bridge and stays out; the
-- creature-local legs port C++-exact, in C++ order: set
-- NON_ATTACKABLE | NOT_SELECTABLE, react passive, self-cast
-- FREEZE_ANIM. The _fireFighter/_setupMine/_setupBomb/_setupRocket
-- latch resets are moot in Lua (their only writers are the
-- unbridged DoAction/SetData arms) and are not modeled.
local function leviathanMkIIReset(event, creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_FREEZE_ANIM)
end

RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 23, leviathanMkIIReset)
