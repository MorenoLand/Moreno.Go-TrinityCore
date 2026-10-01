-- Onyxia's Lair: Onyxia — Lua port of
-- src/server/scripts/Kalimdor/OnyxiasLair/boss_onyxia.cpp (class
-- boss_onyxia : public CreatureScript { boss_onyxiaAI : public
-- BossAI(creature, DATA_ONYXIA) }; GetAI via GetOnyxiaAI<boss_onyxiaAI>
-- (OnyxiaScriptName "instance_onyxias_lair", map 249 gate);
-- AddSC_boss_onyxia at end registers the one script; kalimdor loader
-- decl 66 / call 179 per kalimdor_script_loader.cpp — the FIRST
-- "// Onyxia's Lair" loader group, right after
-- AddSC_instance_maraudon()). Whole-server-tree "onyxia" grep hits
-- only the OnyxiasLair dir files + loader + an "Onyxia Scale Cloak"
-- SpellEffects.cpp comment + TrialOfTheChampion's SPELL_MEMORY_ONYXIA
-- (unrelated spell name). Entry: onyxias_lair.h:62 names
-- NPC_ONYXIA = 10184, GUID-bound in
-- instance_onyxias_lair.cpp OnCreatureCreate (case NPC_ONYXIA ->
-- onyxiaGUID) — kalecgos check PASSES (stronger tie than meathook's
-- summon-strength bind); creature_template ScriptName binding stays
-- DB-side. lua_scripts/kalimdor/ holds no onyxia lua (overnight
-- window did not outrun this one). Eluna creature events: 1
-- OnEnterCombat, 2 OnLeaveCombat, 3 OnKilledUnit, 4 OnDied, 23
-- OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; movement /
-- instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer
--   at its C++ ScheduleEvent cooldown ({10s,20s} / {15s,20s} /
--   {2s,5s} / {10s,20s}); the DoStartTimedAchievement and
--   BossAI::JustEngagedWith instance legs are blocked (standing).
-- - Flame Breath (EVENT_FLAME_BREATH): DoCastVictim
--   (SPELL_FLAME_BREATH 18435), non-triggered; init {10s,20s} ->
--   repeat {10s,20s} unconditional (C++ reschedules every fire).
-- - Tail Sweep (EVENT_TAIL_SWEEP): DoCastAOE(SPELL_TAIL_SWEEP 68867),
--   non-triggered (DoCastAOE resolves to self-cast —
--   kazrogal/illidan precedent); init {15s,20s} -> repeat {15s,20s}.
-- - Cleave (EVENT_CLEAVE): DoCastVictim(SPELL_CLEAVE 68868),
--   non-triggered; init {2s,5s} -> repeat {2s,5s}.
-- - Wing Buffet (EVENT_WING_BUFFET): DoCastVictim
--   (SPELL_WING_BUFFET 18500), non-triggered; init {10s,20s} ->
--   repeat {10s,20s} (C++ re-arms it {15s,30s} on the phase-3
--   landing, but the landing needs the movement machine — see
--   below — so the phase-1 cadence holds for the whole fight).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds):
--   Talk SAY_KILL (1); C++ has no victim gate (unused param), so
--   the port is ungated — faithful, not the terestian_illhoof
--   player-gated convention.
-- Unmodeled (documented-only, no bridges):
-- - HealthBelowPct(65) phase-2 transition (MovePoint 10 to
--   Phase2Location, SetReactState(PASSIVE), AttackStop,
--   SetCombatMovement(false), instance SetData(DATA_ONYXIA_PHASE))
--   and HealthBelowPct(40) phase-3 transition (MovePoint 9 to
--   home+12, Talk SAY_PHASE_3_TRANS (3), re-arm Bellowing Roar 30s)
--   — no movement / MotionMaster / react-state / instance-data
--   bridges. The four timer arms above therefore run for the full
--   fight; the flight phases are unreachable in the Lua engine.
-- - The whole phase-2 flight machine: MoveData[8] table /
--   GetMoveData / SetNextRandomPoint, MoveTakeoff 11, MovePoint
--   cascade (ids 8/10/11), SetCanFly / SetDisableGravity / flight
--   speed legs, SpellHit MovePoint-8 redirect, SetFacingToObject
--   trigger facing, EVENT_DEEP_BREATH (PointData->SpellId via
--   DoCast(me)), EVENT_MOVEMENT (25s cadence), EVENT_FIREBALL
--   (random-target 18392), EVENT_LAIR_GUARD / EVENT_WHELP_SPAWN
--   summons (NPC_LAIRGUARD 36561 / NPC_WHELP 11262, 90s / 500ms
--   cadence, RAID_MODE(20,40) whelp cap) — no movement /
--   MotionMaster / summon bridges (summon STRAND list, standing).
-- - JustSummoned (DoZoneInCombat, AttackStart on a random target,
--   SummonWhelpCount, summons.Summon, lair-guard setActive /
--   SetFarVisible) — no summon bridges.
-- - MovementInform arms (phase-2 takeoff, phase-3 landing Talk
--   SAY_PHASE_2_TRANS (2), trigger kill, tank MoveChase) — no
--   MovementInform / movement bridges.
-- - Bellowing Roar floor-eruption leg (GameObjectInRangeCheck +
--   instance SetGuidData(DATA_FLOOR_ERUPTION_GUID) into the
--   instance-script BFS eruption machine) — no GO-enumeration /
--   instance-data bridges.
-- - SpellHit (breath-direction redirect) and SpellHitTarget
--   (instance SetData(DATA_SHE_DEEP_BREATH_MORE, FAIL) on player
--   eruption-spell hits) — SpellHit-15-never-fires standing queue /
--   no instance bridge.
-- - Initialize / Reset legs (SetCombatMovement, SetReactState,
--   _Reset, instance SetData(DATA_ONYXIA_PHASE),
--   DoStartTimedAchievement / DoStopTimedAchievement
--   ACHIEV_TIMED_START_EVENT 6601) — no instance / achievement /
--   BossAI-Reset bridges.
-- - The C++ HasUnitState(UNIT_STATE_CASTING) early-return gates in
--   UpdateAI — no casting-state bridge (mal_ganis precedent).
-- - The instance script itself (AddSC_instance_onyxias_lair: boss
--   boundaries, OnCreatureCreate GUID bind, OnGameObjectCreate /
--   OnGameObjectRemove floor-trap tracking, FloorEruption BFS,
--   SetBossState, liftoff / whelp-counter / eruption Update, both
--   achievement criteria checks) — instance-script-model blocked
--   (culling_of_stratholme precedent), DOCUMENTED not wired.

local ENTRY = 10184

local SAY_AGGRO = 0
local SAY_KILL  = 1

local SPELL_WING_BUFFET  = 18500
local SPELL_FLAME_BREATH = 18435
local SPELL_CLEAVE       = 68868
local SPELL_TAIL_SWEEP   = 68867

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

local function castVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ EVENT_FLAME_BREATH: DoCastVictim(18435), non-triggered;
-- init {10s,20s} -> repeat {10s,20s} unconditional.
local function onFlameBreath(creature, guid)
    castVictim(creature, SPELL_FLAME_BREATH)
    schedule(guid, "flamebreath", math.random(10000, 20000), function()
        onFlameBreath(creature, guid)
    end)
end

-- C++ EVENT_TAIL_SWEEP: DoCastAOE(68867), non-triggered, self-cast
-- (kazrogal/illidan DoCastAOE precedent); init {15s,20s} ->
-- repeat {15s,20s}.
local function onTailSweep(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tailsweep", math.random(15000, 20000), function()
        onTailSweep(creature, guid)
    end)
end

-- C++ EVENT_CLEAVE: DoCastVictim(68868), non-triggered;
-- init {2s,5s} -> repeat {2s,5s} unconditional.
local function onCleave(creature, guid)
    castVictim(creature, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(2000, 5000), function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_WING_BUFFET: DoCastVictim(18500), non-triggered;
-- init {10s,20s} -> repeat {10s,20s} unconditional (the {15s,30s}
-- phase-3 re-arm needs the movement machine — unmodeled, see
-- header).
local function onWingBuffet(creature, guid)
    castVictim(creature, SPELL_WING_BUFFET)
    schedule(guid, "wingbuffet", math.random(10000, 20000), function()
        onWingBuffet(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "flamebreath", math.random(10000, 20000), function()
        onFlameBreath(creature, guid)
    end)
    schedule(guid, "tailsweep", math.random(15000, 20000), function()
        onTailSweep(creature, guid)
    end)
    schedule(guid, "cleave", math.random(2000, 5000), function()
        onCleave(creature, guid)
    end)
    schedule(guid, "wingbuffet", math.random(10000, 20000), function()
        onWingBuffet(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_KILL)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
