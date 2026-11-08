-- Leviathan Mk II (Ulduar, Mimiron phase 1) -- Lua port of one
-- engine-fired arm of boss_leviathan_mk_iiAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_LEVIATHAN_MKII = 33432.
-- Only the arm below is ported: the combat arms (napalm shell,
-- plasma blast, shock blast, proximity mine, flame suppressant)
-- are scheduled only by DoAction(DO_START_MKII /
-- DO_ASSEMBLED_COMBAT) from the unported boss_mimiron controller
-- machine (event-38 dispatch itself is bridged —
-- creature:DoAction, lua_creature_events.go:802 — so the blocker
-- is the sender chain, not dispatch), and the DamageTaken lethal
-- clamp is phase-gated
-- (phase-1 turret kill + MovePoint to idle vs phase-4 stand-dead
-- + self-repair) behind the same unbridged phase machine plus
-- the vehicle-kit and MovePoint legs. The GetData/SetData
-- setup-mine/bomb/rocket/firefighter latches ARE modeled below
-- (tick 417): the ctor/Reset defaults + the proximity-mine
-- SpellHitTarget clear leg (npc_mimiron_proximity_mine.lua, ev15);
-- the bomb-bot (63801) and rocket-strike (63041) clear legs are
-- SpellScripts with no bridge and stay out — dormant in Go, since
-- the bomb-bot summon (ACU EVENT_SUMMON_BOMB_BOT) and the rocket
-- strike (VX-001 EVENT_ROCKET_STRIKE) are DoAction-scheduled and
-- unported, so neither spell can fire; the firefighter SET leg is
-- BLOCKED (button-GO -> computer -> controller DoAction chain,
-- boss_mimiron.cpp:1688/:1427/:1453/:415/:738, unported), so the
-- latch below stays 0 and achievement_firefighter stays
-- fail-closed. KilledUnit talks into the unported
-- boss_mimiron controller; MovementInform drives the same
-- cross-creature activation chain.

local NPC_LEVIATHAN_MKII = 33432

local SPELL_FREEZE_ANIM = 16245

-- boss_mimiron.cpp:184-192 Data enum (boss_leviathan_mk_iiAI's
-- GetData/SetData slots, :766-786/:839-856).
local DATA_SETUP_MINE = 0
local DATA_SETUP_BOMB = 1
local DATA_SETUP_ROCKET = 2
local DATA_FIREFIGHTER = 4

-- ulduar.h UlduarData (DATA_COLOSSUS = 20, counting up): the
-- instance GUID key for this creature (instance_ulduar.cpp:817).
local DATA_LEVIATHAN_MK_II = 32

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
-- latch resets ARE modeled (tick 417): Reset re-arms them to the
-- ctor defaults (false/true/true/true), the write leg the Go
-- achievement gates (achievement_setup_boom/mine/rocket) read off
-- the killed MK II's SetData store.
local function leviathanMkIIReset(event, creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_FREEZE_ANIM)
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time re-arm would clobber the fight's latches
    -- before the gates read them; the spawn handler sets the ctor
    -- defaults.
    if creature:IsAlive() then
        creature:SetData(DATA_SETUP_MINE, 1)
        creature:SetData(DATA_SETUP_BOMB, 1)
        creature:SetData(DATA_SETUP_ROCKET, 1)
        creature:SetData(DATA_FIREFIGHTER, 0)
    end
end
-- defaults as Reset, plus the instance OnCreatureCreate latch
-- (instance_ulduar.cpp:346/:817): the MK II GUID goes in the
-- instance store under DATA_LEVIATHAN_MK_II so the proximity-mine
-- ev15 clear leg can resolve it via GetInstanceCreature
-- (auriaya ev5 precedent).
local function leviathanMkIISpawned(event, creature)
    creature:SetGuidData(DATA_LEVIATHAN_MK_II)
    creature:SetData(DATA_SETUP_MINE, 1)
    creature:SetData(DATA_SETUP_BOMB, 1)
    creature:SetData(DATA_SETUP_ROCKET, 1)
    creature:SetData(DATA_FIREFIGHTER, 0)
end

RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 5, leviathanMkIISpawned)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 23, leviathanMkIIReset)
