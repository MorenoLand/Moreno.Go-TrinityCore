-- Mimiron (Ulduar, phase-machine controller) -- Lua port of the
-- engine-fired arms of boss_mimironAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 384-682).
-- Entry (ulduar.h): NPC_MIMIRON = 33350.
--
-- PORTED (engine-fired hooks only, C++-exact spell IDs / delays / order):
--   constructor (:391) react-passive leg, on spawn (event 5).
--   Reset (:456): RemoveFlag NON_ATTACKABLE + self-cast WELD, on reset (23).
--   JustEngagedWith (:424): SetFlag NON_ATTACKABLE + WELD removal, on enter
--     combat (1). See DEVIATION note below for the unmodeled vehicle gate.
--   JustDied (:442) + EVENT_OUTTRO_1/2/3 chain: CombatStop, gravity off,
--     SLEEP_VISUAL_1, DESPAWN_ASSAULT_BOTS (DoCastAOE = self-cast
--     convention), then 7s OUTTRO_1 (SLEEP_VISUAL_1 removal, SLEEP_VISUAL_2,
--     faction friendly), +3s OUTTRO_2 (Talk V07TRON_DEATH), +11s OUTTRO_3
--     (TELEPORT_VISUAL, NOT_SELECTABLE flag, DespawnOrUnsummon 1s).
--
-- PORTED (tick 190, RegisterAuraEvent):
--   spell_mimiron_summon_flames_spread (64562) HandleTick — PreventDefaultAction;
--   if the caster has SPELL_FLAMES_PERIODIC_TRIGGER (64561) it casts
--   SPELL_SUMMON_FLAMES_SPREAD (64564) triggered on the aura target
--   (boss_mimiron.cpp:2615-2627). The EVENT_SUMMON_FLAMES boss-AI arm
--   stays out (see below).
--
-- PORTED (tick 193, RegisterAuraEvent):
--   spell_mimiron_weld (63339) HandleTick — the aura target (the
--   player riding in Mimiron's vehicle) casts a random RepairSpell
--   on the vehicle base every fifth tick and faces it every tick
--   (boss_mimiron.cpp:2760-2772); the passenger GetVehicleBase twin
--   is target:IsOnVehicle():GetOwner() (see below).
-- DOCUMENTED-ONLY (no behavior fabricated):
--   DoAction (:397) whole switch: DO_ACTIVATE_VX001 / DO_ACTIVATE_AERIAL /
--     DO_ACTIVATE_V0L7R0N_1 / DO_ACTIVATE_V0L7R0N_2 / DO_ACTIVATE_HARD_MODE --
--     the callers are the unported component AIs in this same file
--     (:807/:817/:960/:1229/:1453); event-38 dispatch itself is
--     bridged (creature:DoAction, lua_creature_events.go:802); the
--     VX001/AERIAL/VOL7RON activation chains they schedule
--     (EVENT_VX001_ACTIVATION_1-9, EVENT_AERIAL_ACTIVATION_1-6,
--     EVENT_VOL7RON_ACTIVATION_1-7) stay out with them, as do their
--     vehicle-kit legs (GetVehicleBase/GetVehicleCreatureBase have no
--     binding on the handler surface), MovePoint legs (no MovePoint
--     binding), elevator/button GameObject legs (no GO bridge), and the
--     instance-GUID creature lookups (no instance bridge).
--   JustEngagedWith vehicle-base leg: DoCast SEAT_6 on the vehicle base --
--     out (no GetVehicleBase binding); button GO flag leg -- out (no GO
--     bridge); EVENT_SUMMON_FLAMES -- out (scheduled only when the
--     _fireFighter latch is set, and the latch is set only by the
--     unbridged DO_ACTIVATE_HARD_MODE); EVENT_INTRO_1/2/3 -- out (INTRO_1
--     Talk text is latch-gated SAY_HARDMODE_ON vs SAY_MKII_ACTIVATE,
--     INTRO_2 legs are vehicle-base legs, INTRO_3 is a DoAction).
--   Reset instance/GO/cross-creature legs: DONE early-return, aerial
--     EnterEvadeMode (no bridge; event 25 unused), BossAI _Reset
--     (instance boss-state, no bridge), elevator/button GO legs, computer
--     DoAction, SEAT_3 cast on MKII via instance GUID -- all out.
--   JustDied legs: SetBossState DONE -- out (no instance boss-state
--     bridge); ExitVehicle + compensating MoveJump -- out (no ExitVehicle
--     binding, so the exit-vehicle repositioning is unmodeled; no in-tree
--     Lua MoveJump precedent to copy the speed mapping from).
--   OUTTRO_2 cache summon -- out: the GO id is latch-gated
--     (GO_CACHE_OF_INNOVATION_FIREFIGHTER 194957 vs GO_CACHE_OF_INNOVATION
--     194789, RAID_MODE hero variants 194958/194956) and the _fireFighter
--     latch is unbridged, so the summon cannot pick its entry; the
--     latch-independent Talk leg IS ported. Computer DoAction -- out.
--
-- DEVIATIONS (checked against the CURRENT runtime, not memory):
--   * JustEngagedWith's `if (!me->GetVehicleBase()) return` gate is not
--     modeled: no GetVehicleBase binding exists on the creature handler
--     surface (grep of lua_creature_events.go / lua_objects.go /
--     scripting/object.go). The self legs below fire unconditionally.
--     (The weld aura arm below does NOT use this gate: its target is a
--     passenger, and the passenger twin target:IsOnVehicle():GetOwner()
--     — the Vehicle object the player rides, whose owner is the base
--     unit (lua_player.go IsOnVehicle, lua_vehicle.go GetOwner) — is
--     live. The boss's own vehicle-base gate stays unmodeled.)
--   * RemoveAurasDueToSpell has no binding; creature:RemoveAura (bridged,
--     lua_creature_events.go:1307) is the single-spell removal twin and
--     is used for the WELD / SLEEP_VISUAL_1 removals.
--   * The OUTTRO chain runs ungated: C++ UpdateAI's CASTING gate is not
--     modeled -- mimiron casts nothing else during the outtro, so the
--     gate cannot change the chain.
--   * No berserk: SAY_BERSERK (14) has no scheduling event in this class.

local NPC_MIMIRON = 33350

local SPELL_WELD = 63339
local SPELL_FLAMES_PERIODIC_TRIGGER = 64561 -- boss_mimiron.cpp:131
local SPELL_SUMMON_FLAMES_SPREAD_TRIGGER = 64562 -- boss_mimiron.cpp:132
local SPELL_SUMMON_FLAMES_SPREAD = 64564 -- boss_mimiron.cpp:134
local SPELL_SLEEP_VISUAL_1 = 64393
local SPELL_SLEEP_VISUAL_2 = 64394
local SPELL_DESPAWN_ASSAULT_BOTS = 64463
local SPELL_TELEPORT_VISUAL = 41232

local SAY_V07TRON_DEATH = 13

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local REACT_PASSIVE = 0
local FACTION_FRIENDLY = 35

-- GUID-keyed CreateLuaEvent timers (C++ JustDied schedules the OUTTRO
-- chain on the EventMap; the frost-bomb / proximity-mine precedent).
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

-- Forward declarations for the cyclic OUTTRO chain (tharon_ja/gal_darah
-- idiom: plain assignments below, never `local function` re-declares).
local outtro1, outtro2, outtro3

-- C++ constructor (:391): SetReactState(REACT_PASSIVE).
local function mimironSpawn(event, creature)
    creature:SetReactState(REACT_PASSIVE)
end

-- C++ Reset (:456), creature-local legs in C++ order: RemoveFlag
-- NON_ATTACKABLE, then self-cast WELD. _Reset clears the event machine,
-- so Lua timers are cancelled here too.
local function mimironReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:CastSpell(creature, SPELL_WELD)
end

-- C++ JustEngagedWith (:424), creature-local legs in C++ order:
-- SetFlag NON_ATTACKABLE, remove WELD. Vehicle-base cast, button GO leg,
-- SUMMON_FLAMES and INTRO chain stay out (see header).
local function mimironEnterCombat(event, creature, target)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveAura(SPELL_WELD)
end

-- C++ UpdateAI EVENT_OUTTRO_3: TELEPORT_VISUAL, NOT_SELECTABLE flag,
-- DespawnOrUnsummon(1s).
outtro3 = function(creature, guid)
    creature:CastSpell(creature, SPELL_TELEPORT_VISUAL)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:DespawnOrUnsummon(1000)
end

-- C++ UpdateAI EVENT_OUTTRO_2: Talk SAY_V07TRON_DEATH, then OUTTRO_3
-- in 11s. Cache summon + computer DoAction stay out (see header).
outtro2 = function(creature, guid)
    creature:Talk(SAY_V07TRON_DEATH)
    schedule(guid, "outtro3", 11000, function()
        outtro3(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_OUTTRO_1: remove SLEEP_VISUAL_1, cast
-- SLEEP_VISUAL_2, faction friendly, then OUTTRO_2 in 3s.
outtro1 = function(creature, guid)
    creature:RemoveAura(SPELL_SLEEP_VISUAL_1)
    creature:CastSpell(creature, SPELL_SLEEP_VISUAL_2)
    creature:SetFaction(FACTION_FRIENDLY)
    schedule(guid, "outtro2", 3000, function()
        outtro2(creature, guid)
    end)
end

-- C++ JustDied (:442), creature-local legs in C++ order: events.Reset
-- (Lua timers cancelled), CombatStop(true), SetDisableGravity(false),
-- self-cast SLEEP_VISUAL_1, self-cast DESPAWN_ASSAULT_BOTS (DoCastAOE),
-- then OUTTRO_1 in 7s. SetBossState, ExitVehicle, MoveJump stay out.
local function mimironDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CombatStop(true)
    creature:SetDisableGravity(false)
    creature:CastSpell(creature, SPELL_SLEEP_VISUAL_1)
    creature:CastSpell(creature, SPELL_DESPAWN_ASSAULT_BOTS)
    schedule(guid, "outtro1", 7000, function()
        outtro1(creature, guid)
    end)
end

RegisterCreatureEvent(NPC_MIMIRON, 5, mimironSpawn)
RegisterCreatureEvent(NPC_MIMIRON, 23, mimironReset)
RegisterCreatureEvent(NPC_MIMIRON, 1, mimironEnterCombat)
RegisterCreatureEvent(NPC_MIMIRON, 4, mimironDied)

-- C++ spell_mimiron_summon_flames_spread_AuraScript::HandleTick
-- (boss_mimiron.cpp:2615-2627): PreventDefaultAction; if the aura
-- caster has SPELL_FLAMES_PERIODIC_TRIGGER (64561) it casts
-- SPELL_SUMMON_FLAMES_SPREAD (64564) triggered on the aura target.
-- Returns true, the PreventDefaultAction twin, so the Go default
-- trigger cast does not also fire.
local function summonFlamesSpreadTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster and caster:HasAura(SPELL_FLAMES_PERIODIC_TRIGGER) then
        caster:CastSpell(target, SPELL_SUMMON_FLAMES_SPREAD, true)
    end
    return true
end

RegisterAuraEvent(SPELL_SUMMON_FLAMES_SPREAD_TRIGGER, 1, summonFlamesSpreadTick)

-- C++ spell_mimiron_weld_AuraScript::HandleTick
-- (boss_mimiron.cpp:2760-2772): the aura target (the player riding in
-- Mimiron's vehicle) casts one of the four RepairSpells on the vehicle
-- base every fifth tick (GetTickNumber() % 5 == 0) and faces the
-- vehicle every tick. GetVehicleBase has no direct binding; the
-- passenger twin is target:IsOnVehicle():GetOwner() — the Vehicle
-- object the player rides, whose owner is the base unit
-- (lua_player.go IsOnVehicle, lua_vehicle.go GetOwner). The repair
-- cast is non-triggered (C++ passes no triggered flag) and rides the
-- player session path like every other non-triggered player cast. No
-- PreventDefaultAction: the handler runs alongside the Go default
-- trigger cast, like C++.
local SPELL_WELD_REPAIR_SPELLS = { 52391, 63313, 63314, 63316 } -- boss_mimiron.cpp:334-340 (SPELL_SEAT_1/2/3/5)

local function weldTick(event, aura, target)
    local vehicle = target:IsOnVehicle()
    if vehicle then
        local base = vehicle:GetOwner()
        if base then
            if aura:GetTickNumber() % 5 == 0 then
                target:CastSpell(base, SPELL_WELD_REPAIR_SPELLS[math.random(1, 4)])
            end
            target:SetFacingToObject(base)
        end
    end
end

RegisterAuraEvent(SPELL_WELD, 1, weldTick)
