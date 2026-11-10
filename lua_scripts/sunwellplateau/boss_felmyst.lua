-- Felmyst (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_felmyst.cpp
-- (boss_felmyst, npc_felmyst_vapor, npc_felmyst_trail); sunwell_
-- plateau.h:33 (DATA_FELMYST = 2, third boss), :67 (NPC_FELMYST =
-- 25038), :69 (NPC_DEAD = 25268), :73 (NPC_VAPOR = 25265), :74
-- (NPC_VAPOR_TRAIL = 25267). Entries: 25038 Felmyst (C++
-- ScriptName "boss_felmyst"), 25265 vapor ("npc_felmyst_vapor"),
-- 25267 vapor trail ("npc_felmyst_trail") — entry+ScriptName
-- verifiable from the C++ sources;
-- the creature_template ScriptName bindings are DB-side — no TDB
-- in this workspace, so only the C++-side naming is verified.
-- Talk lines used: YELL_KILL=1 (kill), YELL_TAKEOFF=3 (liftoff),
-- YELL_BERSERK=4 (berserk), YELL_DEATH=5 (death); YELL_BIRTH=0
-- (JustAppeared) ports in the sibling file, and YELL_BREATH=2
-- (the DoTextEmote is commented out in C++) stays out.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 14 OnHitBySpell, 19
-- OnJustSummoned, 22 OnSummoned (vapor/trail ctor arms), 23 OnReset,
-- 28 OnMovementInform (flight steps 1/5/7). Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat(1):
-- triggered self-cast sunwell radiance 45769 + noxious fumes
-- 47002 (C++ DoCast(me, AURA_*, true)), arm cleave 19983 {5s,10s}
-- then {5s,10s}, non-triggered DoCastVictim (nil ticks cast
-- nothing but keep the schedule, jeklik convention) / corrosion
-- 45866 {10s,20s} then {20s,30s}, non-triggered DoCastVictim /
-- gas nova 45855 {15s,20s} then {20s,25s}, non-triggered self-cast
-- (C++ DoCast default is triggered=false — vaelastrasz
-- convention) / encapsulate 45661 {20s,25s} then {25s,30s}, non-
-- triggered on a threat-list player alive within 150 yd
-- (C++ SelectTarget(Random, 0, 150, true), player-only;
-- nil pick casts nothing but keeps the schedule) /
-- berserk 45078 10min one-shot: Talk(YELL_BERSERK), triggered
-- self-cast (C++ DoCast(me, SPELL_BERSERK, true)), then re-armed
-- at 10s only from the ground arm (the PHASE_FLIGHT arm fires
-- without re-arming, C++-exact — see felmystBerserkTick). EVENT_
-- FLIGHT 1min after EnterPhase(GROUND) starts the flight machine
-- (felmystFlightSequence below); landing re-arms the ground
-- schedule fresh (C++ EnterPhase(GROUND)).
-- OnTargetDied(3): Talk(YELL_KILL), no
-- TYPEID gate (C++-exact — the C++ KilledUnit Talk has no victim
-- gate). OnDied(4): Talk(YELL_DEATH) + SetBossState(DATA_FELMYST=2,
-- DONE) via creature:SetBossState (bridge live 2026-11-05).
-- OnReset(23): the
-- Initialize/events.Reset arms (phase state resets to PHASE_NONE in
-- flightState; timer cancel above is the Lua twin); the DisableGravity arm
-- ports via the creature:SetDisableGravity bridge (felmystResetState;
-- stale "no bridge" retracted); the bounding-radius (no creature
-- motion slot) and setActive(false) arms stay out, and the
-- SetBossState(NOT_STARTED) arm ports via creature:SetBossState
-- (bridge live 2026-11-05). OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Trail (25267): OnSummoned(22): NOT_SELECTABLE SetFlag + triggered
-- self-cast trail trigger 45399 + SetTarget(self) (C++ constructor
-- arms, C++-exact — the constructor runs once at AI creation; C++
-- Reset() is empty, so OnReset(23) would wrongly re-fire on evade,
-- and ev5 never fires for temp summons, so the ctor arms ride ev22;
-- stale "no flag bridge" retracted — SetFlag(59, x) is bridged);
-- the 0.01 bounding-radius arm has no bridge (bounding radius has
-- no creature motion slot; the combat-reach bridge covers only
-- UNIT_FIELD_COMBATREACH).
-- Vapor (25265, "npc_felmyst_vapor") is registered below: ctor
-- NOT_SELECTABLE flag + 0.8 run speed ride ev22 (temp summons
-- never fire ev5; C++ Reset() is empty), JustEngagedWith
-- DoZoneInCombat rides ev1, and the UpdateAI no-victim random-target
-- re-pick rides a 1s retarget loop started from the boss's
-- JustSummoned (ev19) and bounded by the vapor's 9s lifetime (no
-- per-tick UpdateAI hook exists in Lua).
-- NPC_DEAD (25268, Blazing Dead) has no AI class in this file — no
-- C++ ScriptName to verify a binding against, so it is not
-- registered (firesworn convention); its boss-side JustSummoned
-- (AttackStart random + DoZoneInCombat + triggered self-cast dead
-- passive 45415) and SpellHit arms port below (felmystSummoned /
-- felmystSpellHit).
-- Flight phase (PORTED below, felmystFlightSequence + ev28 driver):
-- EVENT_FLIGHT 1min after EnterPhase(GROUND); the 10-step
-- HandleFlightSequence runs on phase state (PHASE_NONE/GROUND/
-- FLIGHT) with MoveTo arrivals driving steps 1/5/7 through
-- MovementInform (ev28, POINT_MOTION_TYPE gate) and timers driving
-- the rest. Approximations, all documented at the arm: case 5's
-- GetContactPoint is computed in Lua from the target->felmyst ray
-- (math.atan2, the GetAbsoluteAngle twin): target + 81.5 (target
-- reach 1.5 + felmyst reach 10 + 70) along that ray, z = target Z
-- + 10 (Object.cpp:3334-3348); EVENT_SUMMON_FOG's
-- UpdateGroundPositionZ has no bridge, so the fog trail spawns at
-- the current Z; case 9's DoStartMovement chase has no informing
-- bridge (Go chase never fires ev28, creaturemotion.go:5725), so
-- the case 9->10 step rides a 3s timer approximating the chase
-- arrival that C++ informs for (ChaseMovementGenerator.cpp:136/
-- 150); the AttackStart/MoveInLineOfSight PHASE_FLIGHT gates have
-- no bridge (melee/aggro gating is engine-side). Ground timers due
-- during flight are dropped without re-arm (C++-exact: the flight
-- switch consumes them) and re-armed fresh by EnterPhase(GROUND)
-- on landing; berserk re-arms at 10s only from the ground arm (the
-- flight arm fires without re-arming, C++-exact). The DamageTaken
-- lethal-damage rewrite (phase != GROUND) ports on ev9. The
-- commented-out VAPOR_SELECT/VAPOR_SUMMON/VAPOR_FORCE arms stay
-- out (commented out in C++ itself). The SpellHit FOG_INFORM 45714 arm (summon
-- NPC_DEAD at the caster, copy health, triggered FOG_CHARM 45717
-- + FOG_CHARM2 45726 self-casts, lethal DealDamage on the caster)
-- ports below (felmystSpellHit, event 14; the summoned DEAD's
-- AttackStart/DoZoneInCombat/passive legs port in felmystSummoned,
-- event 19). The
-- DespawnSummons(NPC_VAPOR_TRAIL) arm ports in despawnTrails below
-- (in-flight: summon NPC_DEAD at each trail, then SetVisible(false)
-- + Despawn; at Reset phase is PHASE_NONE so no DEAD summon, C++-
-- exact). The
-- JustAppeared Talk(YELL_BIRTH) arm ports on event 5 in the sibling
-- eastern_kingdoms/boss_felmyst.lua (ev5 fires on initial spawn and
-- respawn via fireCreatureSpawned — the old "no spawn-fire bridge"
-- note is retracted). The AttackStart/MoveInLineOfSight PHASE_
-- FLIGHT gates have no bridge (melee/aggro gating is engine-
-- side). The UpdateAI gate is C++ IsNonMeleeSpellCast(false)
-- (boss_felmyst.cpp:421, after events.Update, before both phase
-- switches): while a non-melee cast is in flight the due ground
-- event does not execute. Ported via the live creature:IsCasting()
-- bridge (lua_creature_events.go:1950, bjarngrim precedent): each
-- ground tick defers 500ms under its own key, consuming no re-arm;
-- the stale "IsCasting is constant-false / timers fire
-- unconditionally" claim is retracted. The UpdateVictim evade-side
-- arm has no port (evade-side cleanup is engine-side).

local NPC_FELMYST = 25038
local NPC_VAPOR = 25265 -- sunwell_plateau.h:73
local NPC_VAPOR_TRAIL = 25267
local NPC_DEAD = 25268 -- sunwell_plateau.h:69

local SPELL_CLEAVE = 19983
local SPELL_CORROSION = 45866
local SPELL_GAS_NOVA = 45855
local SPELL_ENCAPSULATE_CHANNEL = 45661
local SPELL_BERSERK = 45078
local AURA_SUNWELL_RADIANCE = 45769
local AURA_NOXIOUS_FUMES = 47002
local SPELL_TRAIL_TRIGGER = 45399
local SPELL_DEAD_PASSIVE = 45415 -- boss_felmyst.cpp:70
local SPELL_FOG_INFORM = 45714 -- boss_felmyst.cpp:74
local SPELL_FOG_CHARM = 45717 -- boss_felmyst.cpp:75
local SPELL_FOG_CHARM2 = 45726 -- boss_felmyst.cpp:76
local SPELL_VAPOR_CHANNEL = 45389 -- boss_felmyst.cpp:63
local SPELL_VAPOR_TRIGGER = 45411 -- boss_felmyst.cpp:64
local SPELL_FOG_BREATH = 45495 -- boss_felmyst.cpp:71
local SPELL_FOG_TRIGGER = 45582 -- boss_felmyst.cpp:72
local SPELL_FOG_FORCE = 45782 -- boss_felmyst.cpp:73
local SPELL_VENGEANCE_OF_THE_BLUE_FLIGHT = 45839 -- instance_sunwell_plateau.cpp:92

local YELL_KILL = 1
local YELL_TAKEOFF = 3 -- boss_felmyst.cpp:39
local YELL_BERSERK = 4
local YELL_DEATH = 5

local EMOTE_ONESHOT_LIFTOFF = 254 -- SharedDefines.h:2003
local EMOTE_ONESHOT_LAND = 293 -- SharedDefines.h:2007
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:31

local PHASE_NONE = 0
local PHASE_GROUND = 1
local PHASE_FLIGHT = 2

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- Flight-phase state per felmyst GUID (C++ boss_felmystAI members:
-- phase, uiFlightCount, uiBreathCount, breathX, breathY).
local flightState = {}

local function getFlightState(guid)
    local st = flightState[guid]
    if st == nil then
        st = { phase = PHASE_NONE, flightCount = 0, breathCount = 0, breathX = 0, breathY = 0 }
        flightState[guid] = st
    end
    return st
end

-- Threat-list players alive within range (C++ SelectTarget
-- Random pool via GetAITargets).
local function randomThreatPlayerInRange(creature, maxDist)
    local targets = creature:GetAITargets()
    local out = {}
    for _, unit in ipairs(targets) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 < maxDist then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_CLEAVE: DoCastVictim(19983); repeat {5s,10s}.
local function felmystCleave(creature, guid)
    -- Ground events due during flight are dropped without re-arm
    -- (C++-exact: the PHASE_FLIGHT switch consumes them); the
    -- landing re-entry re-arms the schedule fresh.
    local st = flightState[guid]
    if st == nil or st.phase ~= PHASE_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            felmystCleave(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(5000, 10000), function()
        felmystCleave(creature, guid)
    end)
end

-- C++ EVENT_CORROSION: DoCastVictim(45866); repeat {20s,30s}.
local function felmystCorrosion(creature, guid)
    local st = flightState[guid]
    if st == nil or st.phase ~= PHASE_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "corrosion", 500, function()
            felmystCorrosion(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CORROSION)
    schedule(guid, "corrosion", math.random(20000, 30000), function()
        felmystCorrosion(creature, guid)
    end)
end

-- C++ EVENT_GAS_NOVA: DoCast(me, 45855); repeat {20s,25s}.
local function felmystGasNova(creature, guid)
    local st = flightState[guid]
    if st == nil or st.phase ~= PHASE_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "gasnova", 500, function()
            felmystGasNova(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_GAS_NOVA)
    schedule(guid, "gasnova", math.random(20000, 25000), function()
        felmystGasNova(creature, guid)
    end)
end

-- C++ EVENT_ENCAPSULATE: DoCast(threat-list player alive
-- within 150 yd, 45661); repeat {25s,30s}.
local function felmystEncapsulate(creature, guid)
    local st = flightState[guid]
    if st == nil or st.phase ~= PHASE_GROUND then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "encapsulate", 500, function()
            felmystEncapsulate(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 150)
    if target then
        creature:CastSpell(target, SPELL_ENCAPSULATE_CHANNEL)
    end
    schedule(guid, "encapsulate", math.random(25000, 30000), function()
        felmystEncapsulate(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(YELL_BERSERK) + triggered self-cast
-- 45078; re-armed at 10s only from the ground arm (see below).
local function felmystBerserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            felmystBerserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    -- C++ re-arms EVENT_BERSERK at 10s only from the PHASE_GROUND
    -- arm (boss_felmyst.cpp:428-431); the PHASE_FLIGHT arm fires
    -- without re-arming (:463-466), and landing does not re-schedule
    -- it, so a flight-phase berserk ends the chain, C++-exact.
    local st = flightState[guid]
    if st ~= nil and st.phase == PHASE_GROUND then
        schedule(guid, "berserk", 10000, function()
            felmystBerserkTick(creature, guid)
        end)
    end
end

-- Flight-machine helpers. Forward declarations first: the phase
-- entry points reference each other (enterGroundPhase schedules
-- enterFlightPhase; the sequence's landing step calls
-- enterGroundPhase back), so plain assignments below, never a
-- second `local function` (malown shadowing rule).
local enterFlightPhase
local felmystFlightSequence
local felmystSummonFog

-- C++ instance GetGuidData(DATA_PLAYER_GUID) fallback (boss_felmyst
-- .cpp:297/:323/:352): the instance getter returns the first map
-- player without SPELL_VENGEANCE_OF_THE_BLUE_FLIGHT 45839
-- (instance_sunwell_plateau.cpp:84-97). Go's instance guid store
-- never latches DATA_PLAYER_GUID (C++ computes it on demand), so
-- the kiljaeden/muru GetPlayersInWorld precedent carries it: first
-- in Go world order, which may differ from the C++ map-list order.
local function firstPlayerInMap(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:HasAura(SPELL_VENGEANCE_OF_THE_BLUE_FLIGHT) then
            return p
        end
    end
    return nil
end

-- C++ SelectTarget(Random, 0, 150, true) with the DATA_PLAYER_GUID
-- fallback, shared by flight steps 2/3/5.
local function randomFlightTarget(creature)
    return randomThreatPlayerInRange(creature, 150) or firstPlayerInMap(creature)
end

-- C++ SelectTarget(MaxThreat): GetAITargets returns threat order
-- (nefarian precedent), so the first entry is the max-threat unit.
local function maxThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets > 0 then
        return targets[1]
    end
    return nil
end

-- C++ DespawnSummons(NPC_VAPOR_TRAIL) (boss_felmyst.cpp:488-508):
-- every trail within 100 yd: in-flight it summons an NPC_DEAD at
-- the trail's position (5s out-of-combat), then SetVisible(false) +
-- DespawnOrUnsummon. At Reset phase is PHASE_NONE, so no summon.
local function despawnTrails(creature)
    local st = flightState[creature:GetGUIDLow()]
    local inFlight = st ~= nil and st.phase == PHASE_FLIGHT
    for _, trail in ipairs(creature:GetCreaturesInGrid(NPC_VAPOR_TRAIL, 100)) do
        if inFlight then
            creature:SummonCreature(NPC_DEAD, trail:GetX(), trail:GetY(), trail:GetZ(), 0, 3, 5000)
        end
        trail:SetVisible(false)
        trail:Despawn()
    end
end

-- C++ EnterPhase(PHASE_GROUND) (boss_felmyst.cpp:252-265): stop the
-- fog breath, stop moving, run speed 2.0, then the ground schedule
-- (cleave {5s,10s}, corrosion {10s,20s}, gas nova {15s,20s},
-- encapsulate {20s,25s}, flight 1min). Called from EnterCombat and
-- from the landing step; the berserk 10min one-shot is NOT
-- re-scheduled here (C++ schedules it only in JustEngagedWith).
local function enterGroundPhase(creature, guid)
    local st = getFlightState(guid)
    st.phase = PHASE_GROUND
    creature:StopSpellCast(SPELL_FOG_BREATH)
    creature:RemoveAura(SPELL_FOG_BREATH)
    creature:StopMoving()
    creature:SetSpeedRate(1, 2.0)
    schedule(guid, "cleave", math.random(5000, 10000), function()
        felmystCleave(creature, guid)
    end)
    schedule(guid, "corrosion", math.random(10000, 20000), function()
        felmystCorrosion(creature, guid)
    end)
    schedule(guid, "gasnova", math.random(15000, 20000), function()
        felmystGasNova(creature, guid)
    end)
    schedule(guid, "encapsulate", math.random(20000, 25000), function()
        felmystEncapsulate(creature, guid)
    end)
    schedule(guid, "flight", 60000, function()
        enterFlightPhase(creature, guid)
    end)
end

-- C++ EnterPhase(PHASE_FLIGHT) (boss_felmyst.cpp:266-271): gravity
-- on, counters zeroed, first sequence step in 1s. Ground timers
-- are NOT cancelled (they drop without re-arm on their phase gate
-- while the boss is airborne).
enterFlightPhase = function(creature, guid)
    local st = getFlightState(guid)
    st.phase = PHASE_FLIGHT
    st.flightCount = 0
    st.breathCount = 0
    creature:SetDisableGravity(true)
    schedule(guid, "flightseq", 1000, function()
        felmystFlightSequence(creature, guid)
    end)
end

-- C++ HandleFlightSequence case 2/3 shared leg (boss_felmyst.cpp:
-- 294-314, :320-342): random 150-yd player (DATA_PLAYER_GUID
-- fallback) -> vapor summon at the target +-5 yd, 9s timed
-- despawn; vapor AttackStart(target); InterruptNonMeleeSpells;
-- non-triggered VAPOR_CHANNEL 45389 felmyst->vapor; triggered
-- VAPOR_TRIGGER 45411 vapor->self. No target -> EnterEvadeMode
-- with the step NOT advanced (C++ returns before ++uiFlightCount).
local function summonVaporLeg(creature, guid)
    local target = randomFlightTarget(creature)
    if target == nil then
        creature:EnterEvadeMode()
        return false
    end
    local vapor = creature:SummonCreature(NPC_VAPOR,
        target:GetX() - 5 + math.random(0, 9),
        target:GetY() - 5 + math.random(0, 9),
        target:GetZ(), 0, 2, 9000)
    if vapor ~= nil then
        vapor:AttackStart(target)
        creature:InterruptNonMeleeSpells(false)
        creature:CastSpell(vapor, SPELL_VAPOR_CHANNEL)
        vapor:CastSpell(vapor, SPELL_VAPOR_TRIGGER, true)
    end
    return true
end

-- C++ HandleFlightSequence (boss_felmyst.cpp:274-410). Each arm sets
-- flightCount to the NEXT step explicitly (C++ increments after the
-- switch; the no-target evade legs return before that increment, so
-- they leave the step unchanged). MovePoint steps (1, 5, 7) schedule
-- nothing — their arrival re-drives the sequence via ev28 below.
felmystFlightSequence = function(creature, guid)
    local st = flightState[guid]
    if st == nil or st.phase ~= PHASE_FLIGHT then
        return
    end
    local step = st.flightCount
    if step == 0 then
        -- :281-289: clear motion, liftoff emote, stop, Talk(TAKEOFF),
        -- next step in 2s.
        creature:MoveClear()
        creature:PerformEmote(EMOTE_ONESHOT_LIFTOFF)
        creature:StopMoving()
        creature:Talk(YELL_TAKEOFF)
        st.flightCount = 1
        schedule(guid, "flightseq", 2000, function()
            felmystFlightSequence(creature, guid)
        end)
    elseif step == 1 then
        -- :290-292: MovePoint up 10 yd; arrival drives step 2.
        creature:MoveTo(0, creature:GetX() + 1, creature:GetY(), creature:GetZ() + 10)
        st.flightCount = 2
    elseif step == 2 or step == 3 then
        -- :294-314 / :320-342: vapor summon leg, next step in 10s.
        -- Case 3 despawns the trails first (:318).
        if step == 3 then
            despawnTrails(creature)
        end
        if summonVaporLeg(creature, guid) then
            st.flightCount = step + 1
            schedule(guid, "flightseq", 10000, function()
                felmystFlightSequence(creature, guid)
            end)
        end
    elseif step == 4 then
        -- :344-347: DespawnSummons(VAPOR_TRAIL), next step in 1ms.
        despawnTrails(creature)
        st.flightCount = 5
        schedule(guid, "flightseq", 1, function()
            felmystFlightSequence(creature, guid)
        end)
    elseif step == 5 then
        -- :348-366: breath target pick; GetContactPoint(me, 70) moves
        -- felmyst to 70 yd from the target (plus both combat reaches,
        -- 81.5 total) along the target->felmyst ray, z = target Z + 10
        -- (Object.cpp:3334-3348); arrival drives step 6.
        local target = randomFlightTarget(creature)
        if target == nil then
            creature:EnterEvadeMode()
            return
        end
        st.breathX = target:GetX()
        st.breathY = target:GetY()
        local angle = math.atan2(creature:GetY() - target:GetY(), creature:GetX() - target:GetX())
        -- GetContactPoint is target-based (Object.cpp:3334-3348):
        -- target + (target reach 1.5 + felmyst reach 10 + 70) along
        -- the target->felmyst ray, z = target Z + 10.
        creature:MoveTo(0,
            target:GetX() + 81.5 * math.cos(angle),
            target:GetY() + 81.5 * math.sin(angle),
            target:GetZ() + 10)
        st.flightCount = 6
    elseif step == 6 then
        -- :367-371: face the breath target, next step in 10s
        -- (the commented-out DoTextEmote stays out).
        creature:SetFacingTo(creature:GetAngle(st.breathX, st.breathY))
        st.flightCount = 7
        schedule(guid, "flightseq", 10000, function()
            felmystFlightSequence(creature, guid)
        end)
    elseif step == 7 then
        -- :372-381: fog-breath self-cast, mirror-move past the target
        -- (x = 2*breathX - x), fog summon in 1ms; the MovePoint
        -- arrival drives step 8.
        creature:CastSpell(creature, SPELL_FOG_BREATH, true)
        local x, y, z = creature:GetX(), creature:GetY(), creature:GetZ()
        creature:MoveTo(0, 2 * st.breathX - x, 2 * st.breathY - y, z)
        st.flightCount = 8
        schedule(guid, "summonfog", 1, function()
            felmystSummonFog(creature, guid)
        end)
    elseif step == 8 then
        -- :382-389: stop the breath, count it, loop to step 5 while
        -- fewer than 3 breaths, else continue to step 9, in 1ms.
        creature:StopSpellCast(SPELL_FOG_BREATH)
        creature:RemoveAura(SPELL_FOG_BREATH)
        st.breathCount = st.breathCount + 1
        if st.breathCount < 3 then
            st.flightCount = 5
        else
            st.flightCount = 9
        end
        schedule(guid, "flightseq", 1, function()
            felmystFlightSequence(creature, guid)
        end)
    elseif step == 9 then
        -- :390-399: chase the max-threat target; no target ->
        -- EnterEvadeMode with the step NOT advanced. DoStartMovement
        -- has no informing bridge (Go chase never fires ev28,
        -- creaturemotion.go:5725), so the chase-arrival inform that
        -- C++ fires (ChaseMovementGenerator.cpp:136/150) is a 3s
        -- timer approximation.
        local target = maxThreatTarget(creature)
        if target == nil then
            creature:EnterEvadeMode()
            return
        end
        st.flightCount = 10
        schedule(guid, "flightseq", 3000, function()
            felmystFlightSequence(creature, guid)
        end)
    elseif step == 10 then
        -- :400-406: land, re-enter ground phase (re-arms the ground
        -- schedule + the 1min flight timer), chase the max-threat
        -- target. C++ AttackStarts a possibly-null pick; Lua guards
        -- the nil (AttackStart(nil) would error).
        creature:SetDisableGravity(false)
        creature:PerformEmote(EMOTE_ONESHOT_LAND)
        enterGroundPhase(creature, guid)
        local target = maxThreatTarget(creature)
        if target ~= nil then
            creature:AttackStart(target)
        end
    end
end

-- C++ EVENT_SUMMON_FOG (boss_felmyst.cpp:467-483): every 1s while
-- airborne, summon a vapor trail at felmyst's position (10s timed
-- despawn), strip its trail-trigger aura, triggered FOG_TRIGGER
-- 45582 on it, triggered FOG_FORCE 45782 felmyst->trail.
-- UpdateGroundPositionZ has no bridge; the current Z is the
-- approximation. Stops (no re-arm) once the phase leaves FLIGHT —
-- the C++ twin is the ground switch consuming the pending event.
felmystSummonFog = function(creature, guid)
    local st = flightState[guid]
    if st == nil or st.phase ~= PHASE_FLIGHT then
        return
    end
    local fog = creature:SummonCreature(NPC_VAPOR_TRAIL,
        creature:GetX(), creature:GetY(), creature:GetZ(), 0, 2, 10000)
    if fog ~= nil then
        fog:RemoveAura(SPELL_TRAIL_TRIGGER)
        fog:CastSpell(fog, SPELL_FOG_TRIGGER, true)
        creature:CastSpell(fog, SPELL_FOG_FORCE, true)
    end
    schedule(guid, "summonfog", 1000, function()
        felmystSummonFog(creature, guid)
    end)
end

-- C++ MovementInform (boss_felmyst.cpp:238-242): while in flight,
-- any movement arrival schedules EVENT_FLIGHT_SEQUENCE in 1ms. Only
-- the MovePoint steps wait on it (their next-step values are 2, 6,
-- 8); timer-driven steps ignore stray informs.
local function felmystMovementInform(event, creature, type, id)
    if type ~= POINT_MOTION_TYPE then
        return
    end
    local st = flightState[creature:GetGUIDLow()]
    if st == nil or st.phase ~= PHASE_FLIGHT then
        return
    end
    if st.flightCount == 2 or st.flightCount == 6 or st.flightCount == 8 then
        local guid = creature:GetGUIDLow()
        schedule(guid, "flightseq", 1, function()
            felmystFlightSequence(creature, guid)
        end)
    end
end

-- C++ DamageTaken (boss_felmyst.cpp:244-248): lethal damage outside
-- the ground phase is rewritten to 0.
local function felmystDamageTaken(event, creature, attacker, damage)
    local st = flightState[creature:GetGUIDLow()]
    -- nil state is PHASE_NONE (C++ Initialize), which also rewrites.
    if (st == nil or st.phase ~= PHASE_GROUND) and damage >= creature:GetHealth() then
        return false, 0
    end
end

-- C++ npc_felmyst_vaporAI ctor (boss_felmyst.cpp:529-533):
-- NOT_SELECTABLE flag + 0.8 run speed. Rides ev22 like the trail
-- ctor (temp summons never fire ev5; C++ Reset() is empty).
local function vaporSpawn(event, creature)
    creature:SetFlag(59, 0x2000000) -- UNIT_FLAG_NOT_SELECTABLE
    creature:SetSpeedRate(1, 0.8)
end

-- C++ npc_felmyst_vaporAI::JustEngagedWith (boss_felmyst.cpp:536-540):
-- DoZoneInCombat. (The commented-out VAPOR_FORCE cast stays out.)
local function vaporEnterCombat(event, creature, target)
    creature:DoZoneInCombat()
end

-- C++ npc_felmyst_vaporAI::UpdateAI (boss_felmyst.cpp:542-547): no
-- victim -> random threat-list player within 100 yd, AttackStart.
-- Lua has no per-tick UpdateAI hook; a 1s loop bounded by the
-- vapor's 9s lifetime is the twin (started from felmystSummoned).
local function vaporRetarget(vapor, vaporGuid, remaining)
    if remaining <= 0 or not vapor:IsAlive() then
        timers[vaporGuid] = nil
        return
    end
    if vapor:GetVictim() == nil then
        local target = randomThreatPlayerInRange(vapor, 100)
        if target ~= nil then
            vapor:AttackStart(target)
        end
    end
    schedule(vaporGuid, "retarget", 1000, function()
        vaporRetarget(vapor, vaporGuid, remaining - 1)
    end)
end

-- C++ JustEngagedWith: triggered self-cast of the two pull auras
-- + the ground-phase schedule. The setActive arm has no bridge;
-- SetBossState(IN_PROGRESS) ports via creature:SetBossState
-- (bridge live 2026-11-05); the DoZoneInCombat arm is ported below
-- (bridge live 2026-10-23).
local function felmystEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ boss_felmyst.cpp:160-169: berserk schedule, setActive(true),
    -- DoZoneInCombat() (bridge live 2026-10-23), then the two aura self-casts.
    creature:DoZoneInCombat()
    creature:CastSpell(creature, AURA_SUNWELL_RADIANCE, true)
    creature:CastSpell(creature, AURA_NOXIOUS_FUMES, true)
    -- C++ EnterPhase(PHASE_GROUND) (boss_felmyst.cpp:167, :252-265):
    -- ground-phase schedule + 1min flight timer (enterGroundPhase).
    local st = getFlightState(guid)
    st.phase = PHASE_GROUND
    st.flightCount = 0
    st.breathCount = 0
    enterGroundPhase(creature, guid)
    creature:SetBossState(2, 1) -- DATA_FELMYST, IN_PROGRESS (boss_felmyst.cpp:169)
    schedule(guid, "berserk", 600000, function()
        felmystBerserkTick(creature, guid)
    end)
end

-- C++ KilledUnit: Talk(YELL_KILL) with no victim gate (C++-exact).
local function felmystTargetDied(event, creature, victim)
    creature:Talk(YELL_KILL)
end

-- C++ JustDied: Talk(YELL_DEATH) + SetBossState(DATA_FELMYST=2, DONE)
-- (boss_felmyst.cpp:194-199).
local function felmystDied(event, creature, killer)
    creature:Talk(YELL_DEATH)
    creature:SetBossState(2, 3)
    cancelTimers(creature:GetGUIDLow())
    flightState[creature:GetGUIDLow()] = nil
end

local function felmystResetState(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ boss_felmyst::Reset (boss_felmyst.cpp:143-156): Initialize()
    -- -> events.Reset() (both internal; timer cancel above is the Lua
    -- twin) -> me->SetDisableGravity(true) (:149) ->
    -- SetFloatValue(COMBATREACH, 10) (:151) — the BOUNDINGRADIUS leg
    -- (:150) has no creature motion slot and stays out, as does the
    -- setActive(false) leg; the SetBossState(NOT_STARTED) leg ports
    -- below (bridge live 2026-11-05). Phase resets to PHASE_NONE
    -- (Initialize, :133-137).
    local st = flightState[guid]
    if st ~= nil then
        st.phase = PHASE_NONE
        st.flightCount = 0
        st.breathCount = 0
    end
    creature:SetDisableGravity(true)
    creature:SetCombatReach(10)
    -- C++ DespawnSummons(NPC_VAPOR_TRAIL) (:153, :488-508): phase is
    -- PHASE_NONE here, so no NPC_DEAD summon — plain hide + despawn.
    despawnTrails(creature)
    -- C++ Reset tail (boss_felmyst.cpp:156): SetBossState(DATA_FELMYST, NOT_STARTED).
    -- Alive-gated: Go fires ev23 on the death path too, and C++ Reset
    -- never runs on death (JustDied owns the DONE set).
    if creature:IsAlive() then
        creature:SetBossState(2, 0)
    end
end

-- C++ npc_felmyst_trailAI constructor arms: triggered self-cast
-- trail trigger 45399 + SetTarget(self) (boss_felmyst.cpp:562-567).
-- OnSummoned(22): the constructor runs once at AI creation and C++
-- Reset() is empty, so ev23 would wrongly re-fire on evade; ev5 does
-- NOT fire for temp summons (summonTempCreature fires only ev19/ev22,
-- lua_creature_events.go:190-198 — trails spawn only as temp summons,
-- EVENT_SUMMON_FOG 10s), so the ctor arms ride ev22, which fires on
-- the summoned creature (fireCreaturePetSummoned).
local function trailSpawn(event, creature)
    -- C++ npc_felmyst_trailAI ctor arm order: SetFlag(UNIT_FIELD_FLAGS,
    -- UNIT_FLAG_NOT_SELECTABLE) -> DoCast(me, 45399, true) ->
    -- SetTarget(self) (boss_felmyst.cpp:563-567); stale "no flag
    -- bridge" retracted — creature:SetFlag(59, x) is bridged
    -- (lua_creature_events.go:968) with real targeting effect. The
    -- SetFloatValue(BOUNDINGRADIUS) leg stays unbridged.
    creature:SetFlag(59, 0x2000000) -- UNIT_FLAG_NOT_SELECTABLE
    creature:CastSpell(creature, SPELL_TRAIL_TRIGGER, true)
    creature:SetTarget(creature:GetGUID())
end

-- C++ SpellHit (boss_felmyst.cpp:201-221): hit by FOG_INFORM 45714 ->
-- summon NPC_DEAD (25268) at the caster, copy the caster's max health,
-- triggered self-casts FOG_CHARM 45717 + FOG_CHARM2 45726 on the summon,
-- then lethal DealDamage on the caster (full current health, school 0
-- = physical mask 1, C++ SPELL_SCHOOL_MASK_NORMAL). The summon type is
-- Go 3 = C++ TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 5s.
local function felmystSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_FOG_INFORM or caster == nil then
        return
    end
    local summon = creature:SummonCreature(NPC_DEAD, caster:GetX(), caster:GetY(), caster:GetZ(), 0, 3, 5000)
    if summon ~= nil then
        local maxHealth = caster:GetMaxHealth()
        summon:SetMaxHealth(maxHealth)
        summon:SetHealth(maxHealth)
        summon:CastSpell(summon, SPELL_FOG_CHARM, true)
        summon:CastSpell(summon, SPELL_FOG_CHARM2, true)
    end
    creature:DealDamage(caster, caster:GetHealth(), false, 0)
end

-- C++ JustSummoned (boss_felmyst.cpp:228-236): summon entry 25268 ->
-- AttackStart(random threat-list pick — C++ SelectTarget(Random) has
-- no playerOnly flag and no distance), DoZoneInCombat(summon),
-- triggered self-cast DEAD_PASSIVE 45415.
local function felmystSummoned(event, creature, summoned)
    if summoned:GetEntry() == NPC_DEAD then
        local targets = creature:GetAITargets()
        if #targets > 0 then
            summoned:AttackStart(targets[math.random(#targets)])
        end
        summoned:DoZoneInCombat()
        summoned:CastSpell(summoned, SPELL_DEAD_PASSIVE, true)
    elseif summoned:GetEntry() == NPC_VAPOR then
        -- The flight sequence already AttackStarts the vapor's pick;
        -- the UpdateAI no-victim re-pick rides the bounded retarget
        -- loop below (vaporRetarget, 1s x 9 = the vapor lifetime).
        local vaporGuid = summoned:GetGUIDLow()
        schedule(vaporGuid, "retarget", 1000, function()
            vaporRetarget(summoned, vaporGuid, 8)
        end)
    end
end

RegisterCreatureEvent(NPC_FELMYST, 1, felmystEnterCombat)
RegisterCreatureEvent(NPC_FELMYST, 2, felmystResetState)
RegisterCreatureEvent(NPC_FELMYST, 3, felmystTargetDied)
RegisterCreatureEvent(NPC_FELMYST, 4, felmystDied)
RegisterCreatureEvent(NPC_FELMYST, 9, felmystDamageTaken)
RegisterCreatureEvent(NPC_FELMYST, 14, felmystSpellHit)
RegisterCreatureEvent(NPC_FELMYST, 19, felmystSummoned)
RegisterCreatureEvent(NPC_FELMYST, 23, felmystResetState)
RegisterCreatureEvent(NPC_FELMYST, 28, felmystMovementInform)
RegisterCreatureEvent(NPC_VAPOR, 1, vaporEnterCombat)
RegisterCreatureEvent(NPC_VAPOR, 22, vaporSpawn)
RegisterCreatureEvent(NPC_VAPOR_TRAIL, 22, trailSpawn)
RegisterCreatureEvent(NPC_VAPOR_TRAIL, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_felmyst.cpp:576: empty UpdateAI in C++ (never melees)
end)

-- C++ npc_felmyst_trailAI::MoveInLineOfSight (boss_felmyst.cpp:574) is an
-- EMPTY override — it suppresses the default ScriptedAI::MoveInLineOfSight
-- aggro engage (the trail is a visual trigger, never attacks). Go's
-- fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models :561 C++-exact (the cyanigosa precedent).
local function trailMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(NPC_VAPOR_TRAIL, 27, trailMoveInLOS)
