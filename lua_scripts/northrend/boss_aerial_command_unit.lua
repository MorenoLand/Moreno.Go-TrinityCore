-- Aerial Command Unit (Ulduar, Mimiron phase 3) -- Lua port of the
-- engine-fired arms of boss_aerial_command_unitAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1113-1280).
-- Entry (ulduar.h): NPC_AERIAL_COMMAND_UNIT = 33670.
--
-- PORTED (engine-fired hooks only, C++-exact spell IDs / semantics):
--   constructor (:1120): SetReactState(REACT_PASSIVE) + SetDisableGravity
--     (true), on spawn (event 5).
--   DamageTaken (:1127) phase-independent legs: on a lethal hit, clamp
--     damage to health-1 (Lua `return false, health - 1`, the lich-king
--     damage-rewrite form), SetReactState passive, AttackStop,
--     SetDisableGravity(true), triggered self-cast VEHICLE_DAMAGED.
--
-- DOCUMENTED-ONLY (no behavior fabricated):
--   DoAction (:1159) whole switch -- the sender chains are unported
--     (event-38 dispatch itself is bridged, creature:DoAction,
--     lua_creature_events.go:802), so every arm stays out even
--     where its legs alone would be bridgeable (DO_ENABLE_AERIAL is just
--     SetReactState aggressive; it can never be delivered in Lua):
--     DO_HARDMODE_AERIAL (firefighter latch + self-cast EMERGENCY_MODE +
--       EVENT_SUMMON_FIRE_BOTS 1s), DO_START_AERIAL (gravity off, hover on,
--       flag removal, SetImmuneToPC(false) -- no binding -- react
--       aggressive, phase set, JUNK 5s / ASSAULT 9s / BOMB 9s timers),
--     DO_DISABLE_AERIAL (CastStop -- bridged as StopSpellCast,
--       lua_creature_events.go:1010, vacuous here: no Lua cast state --
--       passive, AttackStop, MoveFall -- no binding -- DelayEvents 23s),
--     DO_ASSEMBLED_COMBAT
--       (flag removal, react aggressive, stand STAND, phase VOL7RON).
--   UpdateAI summon timers (:1233): EVENT_SUMMON_FIRE_BOTS (AOE trigger
--     64620, TRIGGERED_FULL_MASK, max targets 3, 45s re-arm),
--     EVENT_SUMMON_JUNK_BOT (AOE trigger 63820, max targets 1, 11-12s
--     re-arm), EVENT_SUMMON_ASSAULT_BOT (AOE trigger 64425, max targets 1,
--     30s re-arm), EVENT_SUMMON_BOMB_BOT (self 63811, 15-20s re-arm) --
--     all scheduled ONLY by the unbridged DoAction arms above. They are
--     NOT seeded on enter-combat: the vx_001/leviathan precedent is that
--     DoAction-scheduled timers stay out, and seeding a phase-3-only
--     summon scheduler on plain engage would fabricate phase-3 behavior
--     on any earlier aggro. (Contrast npc_mimiron_assault_bot, where
--     engage-seeding is exact because C++ schedules on JustEngagedWith.)
--   UpdateAI plasma-ball attack: DoSpellAttackIfReady (PLASMA_BALL_P1
--     63689 in phase 3, PLASMA_BALL_P2 65647 in phase 4) -- no bridge.
--   DamageTaken phase branches: PHASE_AERIAL_COMMAND_UNIT MovePoint to
--     WP_AERIAL_P4_POS -- out (no MovePoint / GetMotionMaster binding);
--     PHASE_VOL7RON stand-DEAD + IsEncounterFinished cross-creature
--     stand-state reads/kills/despawns + SELF_REPAIR 64383 -- out (phase
--     is EventMap state with no Lua read, and the finish check reads two
--     other creatures via the instance). SetAnimationTier(Ground) -- out
--     (no binding). events.Reset is moot in Lua (no Lua timers exist).
--   EnterEvadeMode (:1201, summons.DespawnAll) -- out: no DespawnAll
--     binding, and EnterEvadeMode itself has no Lua bridge (event 25
--     unused).
--   JustSummoned (:1206, firefighter EMERGENCY_MODE 64582 on summoned
--     assault/junk bots) -- out: the fireFigther latch is set only by the
--     unbridged DO_HARDMODE_AERIAL, so the arm's gate can never be true
--     in Lua; the BossAI summon-tracking leg is engine-side.
--   KilledUnit (:1213, SAY_AERIAL_SLAY 9 / SAY_V07TRON_SLAY 12 via the
--     mimiron controller) -- out: talks into the unported boss_mimiron
--     controller through the instance (no instance bridge).
--   MovementInform (:1220, WP_AERIAL_P4_POS facing + NOT_SELECTABLE flag
--     + CLEAR_ALL_DEBUFFS + DO_ACTIVATE_V0L7R0N_1) -- out: unreachable
--     without the unbridged DamageTaken MovePoint leg, and its tail is a
--     DoAction into the controller.
--
-- BRIDGE NOTES (checked against the CURRENT runtime, not memory):
--   SetDisableGravity, SetHover, SetReactState, AttackStop, SetStandState,
--   SetFacingTo, Talk, CastSpell(..., triggered) are all bound on the
--   handler surface -- their absence above is a dispatch/phase problem,
--   not a method problem. Unbound, verified by grep of
--   lua_creature_events.go / lua_objects.go / scripting/object.go:
--   SetImmuneToPC, SetAnimationTier, MoveFall, MovePoint, GetMotionMaster,
--   DespawnAll, GetVehicleBase, RemoveAurasDueToSpell. (CastStop's twin
--   creature:StopSpellCast IS bridged, lua_creature_events.go:1010.)

local NPC_AERIAL_COMMAND_UNIT = 33670

local SPELL_VEHICLE_DAMAGED = 63415

local REACT_PASSIVE = 0

-- C++ constructor (:1120): react passive + disable gravity.
local function aerialSpawn(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    creature:SetDisableGravity(true)
end

-- C++ DamageTaken (:1127), phase-independent legs in C++ order: lethal
-- clamp (damage = health - 1), react passive, AttackStop, gravity on,
-- triggered self-cast VEHICLE_DAMAGED. The phase branches (MovePoint /
-- stand-dead / self-repair) stay out -- see header. The C++ arm also
-- runs events.Reset, moot here (no Lua timers).
local function aerialDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if damage >= health then
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        creature:SetDisableGravity(true)
        creature:CastSpell(creature, SPELL_VEHICLE_DAMAGED, true)
        return false, health - 1
    end
end

RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 5, aerialSpawn)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 9, aerialDamageTaken)
