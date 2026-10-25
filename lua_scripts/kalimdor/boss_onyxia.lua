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
-- DB-side. Eluna creature events: 1
-- OnEnterCombat, 2 OnLeaveCombat, 3 OnKilledUnit, 4 OnDied, 9
-- OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
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
--   repeat {15s,30s} (C++ :380 re-arms (15s,30s) after every fire in
--   both phases 1 and 3; the phase-3 landing re-seeds it at
--   {15s,30s} via enterPhase3).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds):
--   Talk SAY_KILL (1); C++ has no victim gate (unused param), so
--   the port is ungated — faithful, not the terestian_illhoof
--   player-gated convention.
-- - Phase transitions PORTED on DamageTaken (event 9, nefarian
--   form: C++ checks HealthBelowPct in UpdateAI, post-damage, so
--   the Lua twin uses post = health - damage with a post > 0
--   guard; a hit crossing both gates runs phase 2 then phase 3
--   in C++ order):
--   - 65% phase 2: SetReactState(PASSIVE), AttackStop,
--     SetCombatMovement(false), SetCanFly(true) +
--     SetDisableGravity(true) (the MovementInform-10 legs), Talk
--     SAY_PHASE_2_TRANS (2; C++ talks at takeoff after the
--     MovePoint-10 travel, Lua talks at the latch — no travel
--     leg), phase-1 timers cancelled, phase-2 timers seeded at
--     their C++ takeoff values (whelp 5s, lair guard 15s, deep
--     breath 75s, movement 10s, fireball 18s).
--   - Phase-2 timers: whelp spawns (2 per fire at SpawnLocations
--     [0]/[1], Go summon type 4 = C++ TEMPSUMMON_CORPSE_DESPAWN,
--     DoZoneInCombat each, count reset + 90s re-arm at
--     RAID_MODE(20, 40) else 500ms), lair guard (SpawnLocations
--     [2], 30s repeats), deep breath (Talk EMOTE_BREATH 4 +
--     self-cast of the current MoveData row's spell, 75s),
--     fireball (random alive threat-list unit, any type, 8s
--     repeats), movement (position twin only — the 25s tick
--     advances the MoveData index via SetNextRandomPoint,
--     including its 1/8 off-by-one to an invalid row, after
--     which the tick stops and deep breath casts nothing).
--   - 40% phase 3: SetCanFly(false) + SetDisableGravity(false),
--     SetReactState(AGGRESSIVE), SetCombatMovement(true), Talk
--     SAY_PHASE_3_TRANS (3), Bellowing Roar 18431 at the landing
--     cadence (5s first, 30s repeats), phase-1 timers re-seeded
--     at their C++ landing values (wing buffet {15s,30s}).
-- Unmodeled (documented-only, no bridges):
-- - Phase-2/3 travel legs: MovePoint 10 to Phase2Location,
--   MoveTakeoff 11 to Phase2Floating, MovePoint cascade (ids 8 /
--   10 / 11), MovePoint 9 to home+12, flight speed legs,
--   SpellHit breath-direction redirect (MovePoint-8), the
--   trigger summon + SetFacingToObject facing (C++ :408 — the
--   facing leg is bridgeable via the atan2 twin,
--   devourer_of_souls precedent — but the trigger comes from
--   ObjectAccessor::GetCreature via triggerGUID, no
--   world-access bridge), the trigger kill and tank MoveChase
--   at landing — no movement / MotionMaster bridges.
-- - JustSummoned remainder (AttackStart on a random target,
--   summons.Summon, lair-guard setActive) —
--   the ported summons carry DoZoneInCombat, and the lair guard
--   also carries SetFarVisible(true); no
--   AttackStart / setActive bridge for the summoned object.
-- - Bellowing Roar floor-eruption leg (GameObjectInRangeCheck +
--   instance SetGuidData(DATA_FLOOR_ERUPTION_GUID) into the
--   instance-script BFS eruption machine) — no GO-enumeration /
--   instance-data bridges.
-- - SpellHit (breath-direction redirect, ev14 live) stays out on the
--   flight machine (MovePoint-8 redirect, no motion bridge).
--   SpellHitTarget (instance SetData(DATA_SHE_DEEP_BREATH_MORE, FAIL)
--   on player eruption-spell hits) is dead in C++: eruption spells
--   are cast by floor-eruption gameobjects (instance_onyxias_lair.cpp
--   FloorEruption), never by onyxia, so onyxia's caster-side ev15
--   arm never fires — not an instance-bridge gap.
-- - Initialize / Reset legs (SetCombatMovement, _Reset, instance
--   SetData(DATA_ONYXIA_PHASE), DoStartTimedAchievement /
--   DoStopTimedAchievement ACHIEV_TIMED_START_EVENT 6601) — no
--   instance / achievement / BossAI-Reset bridges; the unconditional
--   SetReactState(REACT_AGGRESSIVE) leg (:158) IS ported in onReset,
--   and Reset re-rolls MovePoint = urand(0, 5) for the deep-breath row.
-- - The C++ HasUnitState(UNIT_STATE_CASTING) early-return gates in
--   UpdateAI — no live casting-state read bridge (IsCasting is constant-false — Go casts are fire-and-forget) (mal_ganis precedent).
-- - The instance script itself (AddSC_instance_onyxias_lair: boss
--   boundaries, OnCreatureCreate GUID bind, OnGameObjectCreate /
--   OnGameObjectRemove floor-trap tracking, FloorEruption BFS,
--   SetBossState, liftoff / whelp-counter / eruption Update, both
--   achievement criteria checks) — instance-script-model blocked
--   (culling_of_stratholme precedent), DOCUMENTED not wired.

local ENTRY = 10184

local SAY_AGGRO = 0
local SAY_KILL  = 1
local SAY_PHASE_2_TRANS = 2
local SAY_PHASE_3_TRANS = 3
local EMOTE_BREATH = 4

local SPELL_WING_BUFFET  = 18500
local SPELL_FLAME_BREATH = 18435
local SPELL_CLEAVE       = 68868
local SPELL_TAIL_SWEEP   = 68867
local SPELL_FIREBALL     = 18392
local SPELL_BELLOWING_ROAR = 18431

local NPC_WHELP     = 11262
local NPC_LAIRGUARD = 36561

-- C++ MoveData[8] (boss_onyxia.cpp): LocId -> deep-breath spell.
-- LocIds 0..7 are west/east/north-west/north-east/south-east/
-- south-west/south/north; the spell is what DEEP_BREATH casts on self.
local BREATH_SPELLS = {
    [0] = 18609, -- SPELL_BREATH_WEST_TO_EAST
    [1] = 18576, -- SPELL_BREATH_EAST_TO_WEST
    [2] = 18584, -- SPELL_BREATH_NW_TO_SE
    [3] = 18617, -- SPELL_BREATH_NE_TO_SW
    [4] = 18564, -- SPELL_BREATH_SE_TO_NW
    [5] = 18596, -- SPELL_BREATH_SW_TO_NE
    [6] = 18351, -- SPELL_BREATH_SOUTH_TO_NORTH
    [7] = 17086, -- SPELL_BREATH_NORTH_TO_SOUTH
}

-- C++ SpawnLocations[3]: whelps x2, lair guard x1.
local SPAWN_LOCATIONS = {
    { -30.127, -254.463, -89.440, 0.0 },
    { -30.817, -177.106, -89.258, 0.0 },
    { -145.950, -212.831, -68.659, 0.0 },
}

local timers = {}
local onyxiaState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function clearState(guid)
    onyxiaState[guid] = nil
end

local function stateFor(guid)
    local st = onyxiaState[guid]
    if not st then
        st = { phase2 = false, phase3 = false, whelps = 0, movePoint = nil }
        onyxiaState[guid] = st
    end
    return st
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
-- init {10s,20s} -> repeat {15s,30s} unconditional (C++ :380 re-arms
-- (15s,30s) in both phase 1 and phase 3).
local function onWingBuffet(creature, guid)
    castVictim(creature, SPELL_WING_BUFFET)
    schedule(guid, "wingbuffet", math.random(15000, 30000), function()
        onWingBuffet(creature, guid)
    end)
end

-- C++ EVENT_WHELP_SPAWN (phase BREATH): summon one whelp at each of
-- SpawnLocations[0] and [1], TEMPSUMMON_CORPSE_DESPAWN (Go type 4);
-- each successful summon counts (C++ JustSummoned increments
-- SummonWhelpCount) and gets DoZoneInCombat (the AttackStart random
-- target leg has no bridge); if the count reaches RAID_MODE(20, 40)
-- the count resets and the next spawn is 90s out, else 500ms.
local function onWhelpSpawn(creature, guid)
    local st = stateFor(guid)
    for i = 1, 2 do
        local loc = SPAWN_LOCATIONS[i]
        local summoned = creature:SummonCreature(NPC_WHELP,
            loc[1], loc[2], loc[3], loc[4], 4, 0)
        if summoned then
            summoned:DoZoneInCombat()
            st.whelps = st.whelps + 1
        end
    end
    if st.whelps >= (creature:Is25ManRaid() and 40 or 20) then
        st.whelps = 0
        schedule(guid, "whelpspawn", 90000, function()
            onWhelpSpawn(creature, guid)
        end)
    else
        schedule(guid, "whelpspawn", 500, function()
            onWhelpSpawn(creature, guid)
        end)
    end
end

-- C++ EVENT_LAIR_GUARD (phase BREATH): summon one lair guard at
-- SpawnLocations[2], TEMPSUMMON_CORPSE_DESPAWN (Go type 4), with
-- DoZoneInCombat + SetFarVisible(true) (the C++ JustSummoned
-- lair-guard legs; setActive has no bridge — Go dynamic motions
-- are tracked by default); 15s first, 30s repeats.
local function onLairGuard(creature, guid)
    local loc = SPAWN_LOCATIONS[3]
    local summoned = creature:SummonCreature(NPC_LAIRGUARD,
        loc[1], loc[2], loc[3], loc[4], 4, 0)
    if summoned then
        summoned:DoZoneInCombat()
        summoned:SetFarVisible(true)
    end
    schedule(guid, "lairguard", 30000, function()
        onLairGuard(creature, guid)
    end)
end

-- C++ EVENT_DEEP_BREATH (phase BREATH): Talk EMOTE_BREATH (4) +
-- DoCast(self, PointData->SpellId), non-triggered; 75s first and
-- repeats. A nil movePoint (the SetNextRandomPoint off-by-one,
-- below) skips the cast but still re-arms, C++-exact.
local function onDeepBreath(creature, guid)
    local st = stateFor(guid)
    if st.movePoint ~= nil then
        creature:Talk(EMOTE_BREATH)
        creature:CastSpell(creature, BREATH_SPELLS[st.movePoint])
    end
    schedule(guid, "deepbreath", 75000, function()
        onDeepBreath(creature, guid)
    end)
end

-- C++ EVENT_FIREBALL (phase BREATH): DoCast(random target, 18392),
-- non-triggered; 18s first, 8s repeats. C++ SelectTarget(Random, 0)
-- defaults playerOnly=false/dist 0, so the pool is alive threat-list
-- units of any type (volazj idiom); IsMoving is always false in the
-- Lua model, so the 1s moving re-arm never applies.
local function onFireball(creature, guid)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 8000, function()
        onFireball(creature, guid)
    end)
end

-- C++ EVENT_MOVEMENT (phase BREATH), position twin only: the
-- MovePoint travel has no bridge, but the point index drives
-- DEEP_BREATH's spell, so the tick models SetNextRandomPoint
-- exactly: iTemp = urand(0, 7); if iTemp >= MovePoint then iTemp++.
-- That yields 8 (no MoveData row) with probability 1/8 per tick, and
-- C++ then returns without re-arming MOVEMENT and leaves PointData
-- nil, so deep breath stops casting for the rest of phase 2; the
-- Lua tick mirrors it by stopping here. 10s first, 25s repeats.
local function onMovement(creature, guid)
    local st = stateFor(guid)
    if st.movePoint == nil then
        return
    end
    local iTemp = math.random(0, 7)
    if iTemp >= st.movePoint then
        iTemp = iTemp + 1
    end
    st.movePoint = iTemp
    if st.movePoint > 7 then
        st.movePoint = nil
        return
    end
    schedule(guid, "movement", 25000, function()
        onMovement(creature, guid)
    end)
end

-- C++ EVENT_BELLOWING_ROAR (phase END): DoCastVictim(18431),
-- non-triggered; the phase-3 entry schedules 30s but the landing
-- re-schedules 5s, and the Lua model has no travel leg, so the
-- landing value (5s first, 30s repeats) is the effective cadence.
local function onBellowingRoar(creature, guid)
    castVictim(creature, SPELL_BELLOWING_ROAR)
    schedule(guid, "bellowingroar", 30000, function()
        onBellowingRoar(creature, guid)
    end)
end

-- C++ phase transition at HealthBelowPct(65), checked in UpdateAI
-- (post-damage, never on the killing blow — nefarian form, so the
-- Lua DamageTaken twin uses post = health - damage with a
-- post > 0 guard). Ported legs: SetReactState(PASSIVE),
-- AttackStop, SetCombatMovement(false), SetCanFly(true) +
-- SetDisableGravity(true) (the MovementInform-10 legs), Talk
-- SAY_PHASE_2_TRANS (C++ talks at takeoff after the MovePoint-10
-- travel; the Lua model has no travel leg, so the Talk lands at
-- the latch), phase-1 timers cancelled (C++ consumes them as
-- no-ops in the BREATH branch), phase-2 timers seeded at their
-- C++ takeoff values. MovePoint-10/11 travel, the trigger summon
-- and facing, and instance SetData(DATA_ONYXIA_PHASE) stay out.
local function enterPhase2(creature, guid)
    local st = stateFor(guid)
    st.phase2 = true
    if st.movePoint == nil then
        st.movePoint = math.random(0, 5)
    end
    cancelTimers(guid)
    creature:SetReactState(0)
    creature:AttackStop()
    creature:SetCombatMovement(false)
    creature:SetCanFly(true)
    creature:SetDisableGravity(true)
    creature:Talk(SAY_PHASE_2_TRANS)
    schedule(guid, "whelpspawn", 5000, function()
        onWhelpSpawn(creature, guid)
    end)
    schedule(guid, "lairguard", 15000, function()
        onLairGuard(creature, guid)
    end)
    schedule(guid, "deepbreath", 75000, function()
        onDeepBreath(creature, guid)
    end)
    schedule(guid, "movement", 10000, function()
        onMovement(creature, guid)
    end)
    schedule(guid, "fireball", 18000, function()
        onFireball(creature, guid)
    end)
end

-- C++ phase transition at HealthBelowPct(40) in the BREATH branch
-- (same UpdateAI/nefarian form as phase 2). Ported legs:
-- SetCanFly(false) + SetDisableGravity(false) (landing legs),
-- SetReactState(AGGRESSIVE), SetCombatMovement(true),
-- Talk SAY_PHASE_3_TRANS, Bellowing Roar at the landing cadence,
-- phase-1 timers re-seeded at their C++ landing values (wing
-- buffet {15s,30s}, not the engage {10s,20s}). Phase-2 timers are
-- cancelled (C++ consumes them as no-ops in the START/END
-- branch). The MovePoint-9 landing travel, trigger kill, tank
-- MoveChase, and floor-eruption GO search stay out.
local function enterPhase3(creature, guid)
    local st = stateFor(guid)
    st.phase3 = true
    cancelTimers(guid)
    creature:SetCanFly(false)
    creature:SetDisableGravity(false)
    creature:SetReactState(2)
    creature:SetCombatMovement(true)
    creature:Talk(SAY_PHASE_3_TRANS)
    schedule(guid, "bellowingroar", 5000, function()
        onBellowingRoar(creature, guid)
    end)
    schedule(guid, "flamebreath", math.random(10000, 20000), function()
        onFlameBreath(creature, guid)
    end)
    schedule(guid, "tailsweep", math.random(15000, 20000), function()
        onTailSweep(creature, guid)
    end)
    schedule(guid, "cleave", math.random(2000, 5000), function()
        onCleave(creature, guid)
    end)
    schedule(guid, "wingbuffet", math.random(15000, 30000), function()
        onWingBuffet(creature, guid)
    end)
end

-- C++ UpdateAI health gates as a DamageTaken twin (ev9 fires before
-- damage is applied, so post-damage health is health - damage;
-- the post > 0 guard keeps the UpdateAI form from firing on the
-- killing blow). A single hit crossing both gates runs phase 2
-- then phase 3 in C++ order (65% tick, then 40% tick).
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = stateFor(guid)
    if st.phase3 then
        return
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local post = health - damage
    if post <= 0 then
        return
    end
    if not st.phase2 and post < math.floor(maxHealth * 65 / 100) then
        enterPhase2(creature, guid)
    end
    if st.phase2 and not st.phase3
            and post < math.floor(maxHealth * 40 / 100) then
        enterPhase3(creature, guid)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- Fresh fight: phase flags and whelp count reset, but the
    -- Reset-rolled MovePoint survives (C++ Initialize runs at Reset,
    -- not at engage; enterPhase2 re-rolls only if Reset never ran).
    local st = stateFor(guid)
    st.phase2 = false
    st.phase3 = false
    st.whelps = 0
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
    clearState(creature:GetGUID())
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
    -- C++ boss_onyxiaAI::Reset (boss_onyxia.cpp:150-160): Initialize() +
    -- SetCombatMovement(true) + _Reset() + SetReactState(REACT_AGGRESSIVE)
    -- (:158) + instance DATA_ONYXIA_PHASE / achievement legs (no
    -- instance / achievement bridges). The react leg restores
    -- AGGRESSIVE after the phase-2 PASSIVE latch (or is vacuous
    -- before any phase transition).
    -- Initialize() also re-rolls MovePoint = urand(0, 5) for the
    -- deep-breath spell row, which the phase-2 port reads.
    local st = stateFor(guid)
    st.movePoint = math.random(0, 5)
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_KILL)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
