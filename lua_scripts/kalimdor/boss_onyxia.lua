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
-- OnDamageTaken, 14 OnHitBySpell, 15 OnSpellHitTarget, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; movement /
-- instance legs below):
-- - Engage (C++ JustEngagedWith): Talk SAY_AGGRO (0) + arm each timer
--   at its C++ ScheduleEvent cooldown ({10s,20s} / {15s,20s} /
--   {2s,5s} / {10s,20s}); BossAI::JustEngagedWith pulse + me-DoZone +
--   SetBossState(DATA_ONYXIA, IN_PROGRESS) PORTED (bridge live,
--   lua_creature_events.go:7099); DoStartTimedAchievement + the
--   CheckRequiredBosses SEQUENCE_BREAK leg stay blocked.
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
--   SpellHit breath-direction redirect travel (MovePoint-8;
--   the index write IS ported, onSpellHit below), the
--   trigger summon + SetFacingToObject facing (C++ :408 — the
--   facing leg is bridgeable via the atan2 twin,
--   devourer_of_souls precedent — but the trigger comes from
--   ObjectAccessor::GetCreature via triggerGUID, no
--   world-access bridge), the trigger kill and tank MoveChase
--   at landing — no movement / MotionMaster bridges.
-- - JustSummoned remainder (summons.Summon, lair-guard setActive) —
--   the ported summons carry DoZoneInCombat + the AttackStart random
--   target leg (EngageWithTarget twin, whelp/lair-guard spawns above)
--   and the lair guard also carries SetFarVisible(true); no setActive
--   bridge for the summoned object.
-- - Bellowing Roar floor-eruption leg (GameObjectInRangeCheck +
--   instance SetGuidData(DATA_FLOOR_ERUPTION_GUID) into the
--   instance-script BFS eruption machine) — no GO-enumeration /
--   instance-data bridges.
-- - SpellHit (breath-direction redirect, ev14 live): the
--   MovePoint-8 travel + SetSpeedRate(1.5) legs stay out (no
--   motion bridge); the MovePoint = LocIdEnd index write IS
--   ported (onSpellHit, position-twin class like onMovement).
--   SpellHitTarget (instance SetData(DATA_SHE_DEEP_BREATH_MORE, FAIL)
--   on player breath-spell hits) IS ported (onyxiaSpellHitTarget,
--   ev15): the ranges are Onyxia's own SPELL_BREATH_* direction
--   arrays, cast by her (DoCast(me, spellId), EVENT_DEEP_BREATH), so
--   the caster-side arm is live. The old "dead in C++" note is
--   retracted — the eruption spells the floor GOs cast are the 17731
--   trap, never these IDs.
-- - Initialize / Reset legs: SetCombatMovement(true) (:154-155) and
--   the _Reset() SetBossState(DATA_ONYXIA, NOT_STARTED) leg (:157,
--   BossAI::_Reset ScriptedCreature.cpp:499-511 — alive-gated and
--   != DONE-gated) are PORTED in onReset (bridges live,
--   lua_creature_events.go:4937/7099); the instance SetData
--   (DATA_ONYXIA_PHASE, PHASE_BREATH) leg is PORTED at the phase-2
--   latch (enterPhase2, shared-table write — the Reset call is a
--   C++ no-op, SetData only acts on PHASE_BREATH); the
--   DoStartTimedAchievement / DoStopTimedAchievement
--   ACHIEV_TIMED_START_EVENT 6601 legs stay out (no creature
--   timed-achievement bridge). The unconditional
--   SetReactState(REACT_AGGRESSIVE) leg (:158) IS ported in onReset,
--   and Reset re-rolls MovePoint = urand(0, 5) for the deep-breath row.
--   No JustDied override exists in C++ — but base BossAI::JustDied
--   (ScriptedCreature.h:330) runs _JustDied() by virtual dispatch when
--   the derived class does not override, so the ev4 DONE leg IS
--   ported in onDied (tick-276 "NO ev4" verdict corrected this tick;
--   Go sets no boss state natively on death).
-- - The C++ HasUnitState(UNIT_STATE_CASTING) gates are PORTED: every
--   timer arm defers 500ms while creature:IsCasting() (phase START/END
--   pre-loop + in-loop gates :346/:386, phase BREATH pre-loop +
--   in-loop gates :412/:478; the MOVEMENT arm's own C++ 500ms CASTING
--   re-arm :434 is the same defer). The 65% / 40% phase latches sit
--   before events.Update in C++ and stay ungated.
-- - The instance script itself (AddSC_instance_onyxias_lair: boss
--   boundaries, OnCreatureCreate GUID bind, OnGameObjectCreate /
--   OnGameObjectRemove floor-trap tracking, FloorEruption BFS,
--   SetBossState override, liftoff / whelp-counter / eruption Update,
--   both achievement criteria checks) — the bridgeable binds live in
--   lua_scripts/kalimdor/instance_onyxias_lair.lua (OnCreatureCreate
--   onyxia bind + whelp-spawner GO leg + the Update liftoff pump and
--   both achievement latches, ported this tick); the SetBossState
--   override is a dead no-op in C++ (it re-calls SetBossState(1,
--   IN_PROGRESS) and EncounterCount = 1, so id 1 >= bosses.size() 1 —
--   InstanceScript::SetBossState returns false, InstanceScript.cpp);
--   the floor-trap / eruption legs have no bridges, DOCUMENTED in
--   that file, not wired here.

local ENTRY = 10184

local DATA_ONYXIA = 0 -- onyxias_lair.h:28
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3
-- EncounterState (InstanceScript.h:68-76).
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
-- LocIdEnd per row (boss_onyxia.cpp:102-110) drives the SpellHit
-- redirect (onSpellHit below): the breath's destination row.
local BREATH_END = {
    [0] = 1, -- WEST_TO_EAST ends east
    [1] = 0, -- EAST_TO_WEST ends west
    [2] = 4, -- NW_TO_SE ends south-east
    [3] = 5, -- NE_TO_SW ends south-west
    [4] = 2, -- SE_TO_NW ends north-west
    [5] = 3, -- SW_TO_NE ends north-east
    -- Rows 6/7 (SOUTH_TO_NORTH / NORTH_TO_SOUTH) have no SpellHit
    -- leg: the C++ switch (boss_onyxia.cpp:200-205) covers only the
    -- six spells above, so no entry is needed here.
}
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
    if creature:IsCasting() then
        schedule(guid, "flamebreath", 500, function()
            onFlameBreath(creature, guid)
        end)
        return
    end
    castVictim(creature, SPELL_FLAME_BREATH)
    schedule(guid, "flamebreath", math.random(10000, 20000), function()
        onFlameBreath(creature, guid)
    end)
end

-- C++ EVENT_TAIL_SWEEP: DoCastAOE(68867), non-triggered, self-cast
-- (kazrogal/illidan DoCastAOE precedent); init {15s,20s} ->
-- repeat {15s,20s}.
local function onTailSweep(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "tailsweep", 500, function()
            onTailSweep(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "tailsweep", math.random(15000, 20000), function()
        onTailSweep(creature, guid)
    end)
end

-- C++ EVENT_CLEAVE: DoCastVictim(68868), non-triggered;
-- init {2s,5s} -> repeat {2s,5s} unconditional.
local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            onCleave(creature, guid)
        end)
        return
    end
    castVictim(creature, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(2000, 5000), function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_WING_BUFFET: DoCastVictim(18500), non-triggered;
-- init {10s,20s} -> repeat {15s,30s} unconditional (C++ :380 re-arms
-- (15s,30s) in both phase 1 and phase 3).
local function onWingBuffet(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wingbuffet", 500, function()
            onWingBuffet(creature, guid)
        end)
        return
    end
    castVictim(creature, SPELL_WING_BUFFET)
    schedule(guid, "wingbuffet", math.random(15000, 30000), function()
        onWingBuffet(creature, guid)
    end)
end

-- C++ EVENT_WHELP_SPAWN (phase BREATH): summon one whelp at each of
-- SpawnLocations[0] and [1], TEMPSUMMON_CORPSE_DESPAWN (Go type 4);
-- each successful summon counts (C++ JustSummoned increments
-- SummonWhelpCount) and gets DoZoneInCombat + AttackStart on a random
-- target (SelectTarget(Random, 0): alive threat-list units of any type,
-- fireball idiom; a non-player pick no-ops through the EngageWithTarget
-- funnel, ramstein precedent, leaving the DoZoneInCombat seeds); if the
-- count reaches RAID_MODE(20, 40) the count resets and the next spawn is
-- 90s out, else 500ms.
local function randomAliveThreatUnit(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        return candidates[math.random(#candidates)]
    end
    return nil
end
local function onWhelpSpawn(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "whelpspawn", 500, function()
            onWhelpSpawn(creature, guid)
        end)
        return
    end
    local st = stateFor(guid)
    for i = 1, 2 do
        local loc = SPAWN_LOCATIONS[i]
        local summoned = creature:SummonCreature(NPC_WHELP,
            loc[1], loc[2], loc[3], loc[4], 4, 0)
        if summoned then
            summoned:DoZoneInCombat()
            local target = randomAliveThreatUnit(creature)
            if target then
                summoned:EngageWithTarget(target)
            end
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
-- DoZoneInCombat + SetFarVisible(true) + AttackStart on a random target
-- (the shared C++ JustSummoned arm, same pick as the whelp leg above;
-- setActive has no bridge — Go dynamic motions are tracked by
-- default); 15s first, 30s repeats.
local function onLairGuard(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "lairguard", 500, function()
            onLairGuard(creature, guid)
        end)
        return
    end
    local loc = SPAWN_LOCATIONS[3]
    local summoned = creature:SummonCreature(NPC_LAIRGUARD,
        loc[1], loc[2], loc[3], loc[4], 4, 0)
    if summoned then
        summoned:DoZoneInCombat()
        local target = randomAliveThreatUnit(creature)
        if target then
            summoned:EngageWithTarget(target)
        end
        summoned:SetFarVisible(true)
    end
    schedule(guid, "lairguard", 30000, function()
        onLairGuard(creature, guid)
    end)
end

-- C++ EVENT_DEEP_BREATH (phase BREATH): Talk EMOTE_BREATH (4) +
-- DoCast(self, PointData->SpellId), non-triggered; 75s first and
-- repeats. A nil movePoint (the SetNextRandomPoint off-by-one,
-- below) skips only the cast: C++ Talks unconditionally inside the
-- !IsMoving branch and guards only the DoCast on PointData, so the
-- emote still fires; the 75s re-arm is unchanged.
local function onDeepBreath(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "deepbreath", 500, function()
            onDeepBreath(creature, guid)
        end)
        return
    end
    local st = stateFor(guid)
    creature:Talk(EMOTE_BREATH)
    if st.movePoint ~= nil then
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
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function()
            onFireball(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "movement", 500, function()
            onMovement(creature, guid)
        end)
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
    if creature:IsCasting() then
        schedule(guid, "bellowingroar", 500, function()
            onBellowingRoar(creature, guid)
        end)
        return
    end
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
-- C++ takeoff values. The instance SetData(DATA_ONYXIA_PHASE,
-- PHASE_BREATH) leg IS ported at this latch (the liftoff reset:
-- latch false, counter 0, timer 10000 — the Lua model has no
-- MovePoint-10 travel leg, so the latch is the arrival twin);
-- MovePoint-10/11 travel, the trigger summon and facing stay out.
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
    -- C++ MovementInform-10 SetData(DATA_ONYXIA_PHASE, PHASE_BREATH)
    -- (instance_onyxias_lair.cpp:163-168): the whelp latch clears,
    -- the hatch counter resets, the 10s liftoff timer starts. The
    -- instance Update pump consumes the timer
    -- (instance_onyxias_lair.lua); the shared-table write is the VoA
    -- precedent (no instance SetData bridge).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.manyWhelpsHandleIt = 0
            data.manyWhelpsCounter = 0
            data.onyxiaLiftoffTimer = 10000
        end
    end
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
    local guid = creature:GetGUIDLow()
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
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-542,
    -- base-first in boss_onyxia.cpp:163-165): instance->SetBossState
    -- (DATA_ONYXIA, IN_PROGRESS) ahead of the pulse / DoZoneInCombat
    -- arms below (creature:SetBossState, lua_creature_events.go:7099).
    creature:SetBossState(DATA_ONYXIA, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm (ScriptedCreature.cpp:540; base-first in boss_onyxia.cpp:163-165)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542; instance/setActive/ScheduleTasks legs out)
    local guid = creature:GetGUIDLow()
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
    cancelTimers(creature:GetGUIDLow())
    clearState(creature:GetGUIDLow())
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    clearState(guid)
    -- C++ boss_onyxiaAI::Reset (boss_onyxia.cpp:150-160): Initialize() +
    -- SetCombatMovement(true) + _Reset() + SetReactState(REACT_AGGRESSIVE)
    -- (:158) + the instance SetData(DATA_ONYXIA_PHASE, PHASE_START) call
    -- (:159), a no-op in C++ (SetData only acts on PHASE_BREATH), and
    -- the DoStopTimedAchievement(6601) leg, which stays out (no
    -- creature timed-achievement bridge). The react leg restores
    -- AGGRESSIVE after the phase-2 PASSIVE latch (or is vacuous
    -- before any phase transition).
    -- Initialize() also re-rolls MovePoint = urand(0, 5) for the
    -- deep-breath spell row, which the phase-2 port reads.
    local st = stateFor(guid)
    st.movePoint = math.random(0, 5)
    -- SetCombatMovement(true) (:154-155; the !IsCombatMovementAllowed
    -- guard is vacuous — setting true when already true is a no-op) and
    -- the _Reset() SetBossState(NOT_STARTED) leg (:157,
    -- BossAI::_Reset ScriptedCreature.cpp:499-511: alive-gated and
    -- != DONE-gated; Go fires ev23 on the death path before ev4,
    -- moroes/algalon precedent, so both gates are live here).
    creature:SetCombatMovement(true)
    if creature:IsAlive() and creature:GetBossState(DATA_ONYXIA) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_ONYXIA, BOSS_STATE_NOT_STARTED)
    end
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    clearState(creature:GetGUIDLow())
    -- C++: no JustDied override in boss_onyxia.cpp, so base
    -- BossAI::JustDied (ScriptedCreature.h:330) runs _JustDied() ->
    -- SetBossState(DATA_ONYXIA, DONE) (ScriptedCreature.cpp:513-520);
    -- the mal_ganis precedent. Without this leg the boss stayed
    -- IN_PROGRESS after the kill (Go writes boss state only via the
    -- Lua bridge, lua_creature_events.go:7099).
    creature:SetBossState(DATA_ONYXIA, BOSS_STATE_DONE)
end

-- C++ SpellHit (boss_onyxia.cpp:199-215): hit by one of the six
-- diagonal/east-west breath spells -> PointData = GetMoveData();
-- MovePoint = PointData->LocIdEnd; SetSpeedRate(MOVE_FLIGHT, 1.5);
-- MovePoint(8, MiddleRoomLocation). The travel + speed legs stay out
-- (no MovePoint bridge — verified: no methods["MovePoint"] in
-- lua_creature_events.go / lua_objects.go; SetSpeedRate IS bridged
-- at lua_creature_events.go:6065 but moves nothing without travel).
-- The index write PORTS as a position-twin leg, same class as
-- onMovement: the next MOVEMENT tick's SetNextRandomPoint excludes
-- the DESTINATION row, not the origin row. ev14 is live
-- (CreatureEventOnHitBySpell, runtime.go:381; fired per hit target
-- in the creature spell path, boss_ai.go:4282 — the self-cast breath
-- hits Onyxia herself, same loop as the ported ev15 arm; midnight
-- attumenSpellHit precedent for the (event, creature, caster,
-- spellId) signature). No phase gate in C++ — the filter is the
-- six-spell switch only; the north/south rows (18351/17086) are NOT
-- in it, so a north/south breath leaves the index untouched. Only
-- Onyxia casts these spells (deep breath, self-cast), so no caster
-- check is needed.
local function onSpellHit(event, creature, caster, spellId)
    if spellId ~= 18609 and spellId ~= 18576 and spellId ~= 18584 and
        spellId ~= 18617 and spellId ~= 18564 and spellId ~= 18596 then
        return
    end
    local st = onyxiaState[creature:GetGUIDLow()]
    if st == nil or st.movePoint == nil then
        return
    end
    local dest = BREATH_END[st.movePoint]
    if dest ~= nil then
        st.movePoint = dest
    end
end

-- C++ SpellHitTarget (boss_onyxia.cpp:276-294): a breath spell
-- hitting a player fails the She Deep Breaths More latch
-- (instance SetData(DATA_SHE_DEEP_BREATH_MORE, FAIL)). The ranges
-- are the SPELL_BREATH_* direction arrays (17086 N->S, 18351 S->N,
-- 18576 E->W, 18609 W->E, 18564 SE->NW, 18584 NW->SE, 18596 SW->NE,
-- 18617 NE->SW, boss_onyxia.cpp:56-79) plus their array variants;
-- Onyxia casts them herself (DoCast(me, spellId), EVENT_DEEP_BREATH
-- :420-428), so the caster-side ev15 arm is live. The latch lives in
-- the shared instanceData table (1/0; default 1 = C++ ctor true)
-- for the Go type-18 dispatch (checkOnyxiasLairCriteria); it is
-- never set back — C++ has no live writer for the IN_PROGRESS arm
-- (the SetBossState override re-call is a dead no-op, instance
-- header settles it).
local function onyxiaSpellHitTarget(event, creature, target, spellId)
    if not target or target:GetTypeId() ~= 4 then -- TYPEID_PLAYER
        return
    end
    if (spellId >= 17086 and spellId <= 17095) or spellId == 17097 or
        (spellId >= 18351 and spellId <= 18361) or
        (spellId >= 18564 and spellId <= 18576) or
        (spellId >= 18578 and spellId <= 18607) or spellId == 18609 or
        (spellId >= 18611 and spellId <= 18628) or
        (spellId >= 21132 and spellId <= 21133) or
        (spellId >= 21135 and spellId <= 21139) or
        (spellId >= 22191 and spellId <= 22202) or
        (spellId >= 22267 and spellId <= 22268) then
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data.sheDeepBreathMore = 0
            end
        end
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_KILL)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 14, onSpellHit)
RegisterCreatureEvent(ENTRY, 15, onyxiaSpellHitTarget)
RegisterCreatureEvent(ENTRY, 23, onReset)
