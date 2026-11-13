-- Leviathan Mk II (Ulduar, Mimiron phase 1) -- Lua port of the DoAction
-- receiver + combat pumps of boss_leviathan_mk_iiAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_LEVIATHAN_MKII = 33432.
-- Ported: the DoAction receiver (event 38, boss_mimiron.cpp:733-761)
-- + the five UpdateAI combat pumps (:860-920). Action 0 DO_START_MKII
-- sets REACT_AGGRESSIVE and schedules napalm 3s / plasma 15s / mine 5s /
-- shock 18s; action 1 DO_HARDMODE_MKII sets the firefighter latch,
-- self-casts EMERGENCY_MODE 64582 + EMERGENCY_MODE_TURRET 65101
-- (DoCastAOE = self-cast convention), schedules flame suppressant 60s,
-- then falls through to the START legs in C++ order; action 12
-- DO_ASSEMBLED_COMBAT stands, clears NON_ATTACKABLE | NOT_SELECTABLE,
-- sets aggressive, cancels the phase-1-only pumps (napalm / plasma /
-- flame suppressant) and reschedules mine 15s / shock 45s (schedule()
-- replaces by key, matching C++ RescheduleEvent). Event-38 dispatch is
-- bridged (creature:DoAction, lua_creature_events.go:802). Sender chain
-- is live for 0/1: boss_mimiron.lua intro3 sends DO_HARDMODE_MKII /
-- DO_START_MKII via GetInstanceCreature(32) (tick 557). Action 12 is
-- live too: vol7ronActivation7 sends it to MKII via the same instance
-- GUID (tick 557 fix — C++ :639 loops DATA 32..34).
-- SetPhase legs (PHASE_LEVIATHAN_MK_II / PHASE_VOL7RON) have no Lua
-- mirror; the phase-1-only re-arm legs are covered by the action-12
-- cancels below. DamageTaken (:698-731) PORTED tick 561 below: lethal
-- clamp (return false, health - 1), NON_ATTACKABLE flag, triggered
-- VEHICLE_DAMAGED 63415, AttackStop, react passive; phase-1
-- (mkiiAssembled unset = PHASE_LEVIATHAN_MK_II mirror): CastStop,
-- turret KillSelf via GetVehicleKit()->GetPassenger(3) (bridged,
-- lua_vehicle.go:74), SetSpeedRate RUN 1.5, MoveTo(1, idle) — the
-- MovePoint twin (lua_creature_events.go:4875), which fires the ev28
-- arrival below; VOL7RON (assembled): SetStandState DEAD,
-- IsEncounterFinished cross-creature stand-state check (all three
-- DEAD -> Kill + DespawnOrUnsummon 120s each + controller Kill to run
-- its JustDied outtro, the frozen-core Kill-to-deliver-JustDied twin —
-- C++ calls JustDied directly without killing, so the controller death
-- is the documented deviation), else CastStop + SELF_REPAIR 64383.
-- RemoveAllAurasExceptType has NO bridge — leg skipped (auras persist;
-- the component is passive/non-attackable at 1 HP).
-- MovementInform (:795-822) PORTED tick 561 (P1_IDLE) + this tick (P4):
-- P1_IDLE arrival sets NOT_SELECTABLE, HALF_HEAL 64188, controller
-- DoAction(DO_ACTIVATE_VX001); P4_POS_1/2/4 arrivals schedule the 1ms
-- EVENT_MOVE_POINT_2/3/5 legs (MoveTo P4_POS_2/3/5); P4_POS_3 arrival
-- sends controller DoAction(DO_ACTIVATE_V0L7R0N_2).
-- The GetData/SetData
-- setup-mine/bomb/rocket/firefighter latches ARE modeled below
-- (tick 417): the ctor/Reset defaults + the proximity-mine
-- SpellHitTarget clear leg (npc_mimiron_proximity_mine.lua, ev15);
-- the bomb-bot (63801) and rocket-strike (63041) clear legs are
-- SpellScripts with no bridge and stay out; the summons/spells are
-- now ported (ACU EVENT_SUMMON_BOMB_BOT + VX-001 EVENT_ROCKET_STRIKE,
-- tick 557), so the spells can fire while their clear legs remain
-- unported. The firefighter SET leg is
-- PORTED via DoAction action 1 below (SetData(DATA_FIREFIGHTER, 1) —
-- the controller-side _fireFighter latch rides boss_mimiron.lua's
-- st.fireFighter, which picks DO_HARDMODE_MKII at intro3), so the
-- latch below reads 1 on hard mode and achievement_firefighter can
-- pass. KilledUnit (boss_mimiron.cpp:788-793) PORTED tick 559 below:
-- player-gated, the controller Talks SAY_MKII_SLAY 3 in phase 1,
-- SAY_V07TRON_SLAY 12 once assembled (mkiiAssembled latch = the
-- EventMap PHASE_VOL7RON mirror);
-- JustSummoned (boss_mimiron.cpp:783-785) is pure summons.Summon
-- bookkeeping with no consumer in this file (no summon-list DespawnAll
-- leg is modeled; Reset/JustDied run timer-cancel only) — documented-only.
-- MovementInform drives the same cross-creature activation chain.

local NPC_LEVIATHAN_MKII = 33432
local NPC_MIMIRON = 33350 -- ulduar.h (controller, KilledUnit Talk target)

-- boss_mimiron.cpp:36-47 Yells (controller Talk ids).
local SAY_MKII_SLAY = 3
local SAY_V07TRON_SLAY = 12

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

-- ulduar.h:46 BossData: MKII's BossAI _bossId is BOSS_MIMIRON (its
-- _Reset() sets boss state 10 NOT_STARTED).
local BOSS_MIMIRON = 10

-- UnitDefines.h:125/149; SetFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:5056).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

-- boss_mimiron.cpp:281-298 Actions enum (0-based): DO_START_MKII 0,
-- DO_HARDMODE_MKII 1, DO_ASSEMBLED_COMBAT 12.
local DO_START_MKII = 0
local DO_HARDMODE_MKII = 1
local DO_ASSEMBLED_COMBAT = 12

-- boss_mimiron.cpp:86-174 spell ids used by the DoAction/UpdateAI arms.
local SPELL_EMERGENCY_MODE = 64582
local SPELL_EMERGENCY_MODE_TURRET = 65101
local SPELL_FORCE_CAST_NAPALM_SHELL = 64539
local SPELL_SCRIPT_EFFECT_PLASMA_BLAST = 64542
local SPELL_PROXIMITY_MINES = 63027
local SPELL_SHOCK_BLAST = 63631
local SPELL_FLAME_SUPPRESSANT_MK = 64570
local SPELL_VEHICLE_DAMAGED = 63415 -- boss_mimiron.cpp:172
local SPELL_SELF_REPAIR = 64383 -- boss_mimiron.cpp:175
local SPELL_HALF_HEAL = 64188 -- boss_mimiron.cpp:178

local NPC_VX_001 = 33651
local NPC_AERIAL_COMMAND_UNIT = 33670

local UNIT_STAND_STATE_DEAD = 7 -- UnitDefines.h:40
local MKII_SEAT_CANNON = 3 -- boss_mimiron.cpp:329
local WP_MKII_P1_IDLE = 1 -- boss_mimiron.cpp:318
local WP_MKII_P4_POS_1 = 2 -- boss_mimiron.cpp:319
local WP_MKII_P4_POS_2 = 3
local WP_MKII_P4_POS_3 = 4
local WP_MKII_P4_POS_4 = 5
local WP_MKII_P4_POS_5 = 6
local MOVE_RUN = 1
local DO_ACTIVATE_VX001 = 2 -- boss_mimiron.cpp:283
local DO_ACTIVATE_V0L7R0N_2 = 11 -- boss_mimiron.cpp:297
local POINT_MOTION_TYPE = 8
-- VehicleRelocation[WP_MKII_P1_IDLE], boss_mimiron.cpp:345.
local MKII_P1_IDLE_POS = { 2792.070, 2596.320, 364.3136 }
-- VehicleRelocation[WP_MKII_P4_POS_2/3/5], boss_mimiron.cpp:347-348/350
-- (the P4_POS_1/4 destinations are the controller's MoveTo legs in
-- boss_mimiron.lua, so only the MKII-driven destinations live here).
local MKII_P4_POS_2 = { 2768.195, 2573.095, 364.0636 }
local MKII_P4_POS_3 = { 2763.820, 2568.870, 364.3136 }
local MKII_P4_POS_5 = { 2744.610, 2569.380, 364.3136 }
local timers = {}

-- Per-GUID assembled latch: the Lua mirror of the EventMap phase
-- (PHASE_LEVIATHAN_MK_II -> PHASE_VOL7RON on DO_ASSEMBLED_COMBAT),
-- read by the KilledUnit arm; cleared on Reset/spawn/death.
local mkiiAssembled = {}

-- GetCurrTime is wall-clock ms (Eluna global, engine globals.go:941);
-- os.clock() is CPU time and stalls while the server idles, which would
-- corrupt the due/remaining math used by timeUntil (thekal idiom).
local function nowMs()
    return GetCurrTime()
end

local function armEntry(guid, key, delayMs, fn)
    local entry = { due = nowMs() + delayMs, fn = fn }
    entry.id = CreateLuaEvent(function()
        local per = timers[guid]
        if per and per[key] == entry then
            per[key] = nil
        end
        fn()
    end, delayMs)
    return entry
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, entry in pairs(per) do
            RemoveEventById(entry.id)
        end
        timers[guid] = nil
    end
end

local function cancelTimerKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key].id)
        per[key] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key].id)
    end
    -- {minMs, maxMs} table delay = randtime range, resolved here so the
    -- concrete due time is recorded for timeUntil.
    local delayMs = delay
    if type(delay) == "table" then
        delayMs = math.random(delay[1], delay[2])
    end
    per[key] = armEntry(guid, key, delayMs, fn)
end

-- C++ EventMap::GetTimeUntilEvent twin: ms until the named timer fires,
-- nil when unarmed (cancelled or never scheduled).
local function timeUntil(guid, key)
    local per = timers[guid]
    local entry = per and per[key]
    if not entry then
        return nil
    end
    return entry.due - nowMs()
end

-- Napalm/plasma cross-reschedule each other, so both are forward-declared
-- and assigned with plain `= function` (tharon_ja/gal_darah idiom) —
-- a later `local function` same-name would shadow the forward slot and
-- leave the pumps calling nil.
local napalmTick, plasmaTick

-- C++ EVENT_NAPALM_SHELL (:905-912): DoCastAOE(SPELL_FORCE_CAST_NAPALM_
-- SHELL 64539); re-arm {6s,15s}; if time-until-plasma < 2s, reschedule
-- plasma to 2s.
napalmTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "napalm", 500, function()
            napalmTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FORCE_CAST_NAPALM_SHELL)
    schedule(guid, "napalm", {6000, 15000}, function()
        napalmTick(creature, guid)
    end)
    local plasmaIn = timeUntil(guid, "plasma")
    if plasmaIn and plasmaIn < 2000 then
        schedule(guid, "plasma", 2000, function()
            plasmaTick(creature, guid)
        end)
    end
end

-- C++ EVENT_PLASMA_BLAST (:875-881): DoCastVictim(SPELL_SCRIPT_EFFECT_
-- PLASMA_BLAST 64542); re-arm {30s,45s}; if time-until-napalm < 9s,
-- reschedule napalm to 9s.
plasmaTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "plasma", 500, function()
            plasmaTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_SCRIPT_EFFECT_PLASMA_BLAST)
    schedule(guid, "plasma", {30000, 45000}, function()
        plasmaTick(creature, guid)
    end)
    local napalmIn = timeUntil(guid, "napalm")
    if napalmIn and napalmIn < 9000 then
        schedule(guid, "napalm", 9000, function()
            napalmTick(creature, guid)
        end)
    end
end

-- C++ EVENT_PROXIMITY_MINE (:871-874): DoCastAOE(SPELL_PROXIMITY_MINES
-- 63027); re-arm 35s.
local function mineTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mine", 500, function()
            mineTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_PROXIMITY_MINES)
    schedule(guid, "mine", 35000, function()
        mineTick(creature, guid)
    end)
end

-- C++ EVENT_SHOCK_BLAST (:883-886): DoCastAOE(SPELL_SHOCK_BLAST 63631);
-- re-arm {34s,36s}.
local function shockTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shock", 500, function()
            shockTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHOCK_BLAST)
    schedule(guid, "shock", {34000, 36000}, function()
        shockTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_SUPPRESSANT_MK (:887-890): DoCastAOE(SPELL_FLAME_
-- SUPPRESSANT_MK 64570); re-arm 60s.
local function flameSuppressantTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flamesuppressant", 500, function()
            flameSuppressantTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FLAME_SUPPRESSANT_MK)
    schedule(guid, "flamesuppressant", 60000, function()
        flameSuppressantTick(creature, guid)
    end)
end

-- C++ boss_leviathan_mk_iiAI::DoAction (:733-761, event 38).
local function leviathanMkIIDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == DO_HARDMODE_MKII then
        -- :737-742: _fireFighter = true (SetData mirror — C++ SetData has
        -- no FIREFIGHTER case; the latch is written here, where C++ sets
        -- the field); self-cast EMERGENCY_MODE 64582; DoCastAOE
        -- EMERGENCY_MODE_TURRET 65101 (self-cast convention); schedule
        -- FLAME_SUPPRESSANT_MK 60s (phase-1-only group); fall through.
        creature:SetData(DATA_FIREFIGHTER, 1)
        creature:CastSpell(creature, SPELL_EMERGENCY_MODE)
        creature:CastSpell(creature, SPELL_EMERGENCY_MODE_TURRET)
        schedule(guid, "flamesuppressant", 60000, function()
            flameSuppressantTick(creature, guid)
        end)
    end
    if action == DO_HARDMODE_MKII or action == DO_START_MKII then
        -- :744-750: SetReactState aggressive; SetPhase(PHASE_LEVIATHAN_
        -- MK_II) has no Lua mirror (see header); napalm 3s / plasma 15s /
        -- mine 5s / shock 18s (mine/shock carry no phase group in C++).
        creature:SetReactState(REACT_AGGRESSIVE)
        schedule(guid, "napalm", 3000, function()
            napalmTick(creature, guid)
        end)
        schedule(guid, "plasma", 15000, function()
            plasmaTick(creature, guid)
        end)
        schedule(guid, "mine", 5000, function()
            mineTick(creature, guid)
        end)
        schedule(guid, "shock", 18000, function()
            shockTick(creature, guid)
        end)
    elseif action == DO_ASSEMBLED_COMBAT then
        -- :752-760: SetStandState STAND; RemoveFlag NON_ATTACKABLE |
        -- NOT_SELECTABLE; SetReactState aggressive; SetPhase(PHASE_VOL7RON)
        -- mirrored by the mkiiAssembled latch (KilledUnit phase read).
        mkiiAssembled[guid] = true
        creature:SetStandState(0) -- UNIT_STAND_STATE_STAND
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        creature:SetReactState(REACT_AGGRESSIVE)
        cancelTimerKey(guid, "napalm")
        cancelTimerKey(guid, "plasma")
        cancelTimerKey(guid, "flamesuppressant")
        schedule(guid, "mine", 15000, function()
            mineTick(creature, guid)
        end)
        schedule(guid, "shock", 45000, function()
            shockTick(creature, guid)
        end)
    end
end

-- C++ boss_leviathan_mk_iiAI::Reset (:827-837): BossAI::_Reset
-- (ScriptedCreature.cpp:499-511) ports the instance boss-state leg
-- via creature:SetBossState (bridged, lua_creature_events.go:7099;
-- alive-gated + skipped when DONE, C++-exact); the summons.DespawnAll
-- half of _Reset stays out (no bridge). The creature-local legs port
-- C++-exact, in C++ order: set NON_ATTACKABLE | NOT_SELECTABLE, react
-- passive, self-cast FREEZE_ANIM. The _fireFighter/_setupMine/
-- _setupBomb/_setupRocket
-- latch resets ARE modeled (tick 417): Reset re-arms them to the
-- ctor defaults (false/true/true/true), the write leg the Go
-- achievement gates (achievement_setup_boom/mine/rocket) read off
-- the killed MK II's SetData store.
local function leviathanMkIIReset(event, creature)
    -- C++ Reset -> _Reset() (ScriptedCreature.cpp:507): events.Reset()
    -- cancels all scheduled pumps.
    cancelTimers(creature:GetGUIDLow())
    mkiiAssembled[creature:GetGUIDLow()] = nil
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
    creature:SetReactState(REACT_PASSIVE)
    creature:CastSpell(creature, SPELL_FREEZE_ANIM)
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time re-arm would clobber the fight's latches
    -- before the gates read them; the spawn handler sets the ctor
    -- defaults.
    if creature:IsAlive() then
        -- BossAI::_Reset (ScriptedCreature.cpp:509-510): the boss-state
        -- leg runs only when alive and the encounter is not DONE.
        if creature:GetBossState(BOSS_MIMIRON) ~= 3 then
            creature:SetBossState(BOSS_MIMIRON, 0)
        end
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
    mkiiAssembled[creature:GetGUIDLow()] = nil
    creature:SetData(DATA_SETUP_MINE, 1)
    creature:SetData(DATA_SETUP_BOMB, 1)
    creature:SetData(DATA_SETUP_ROCKET, 1)
    creature:SetData(DATA_FIREFIGHTER, 0)
end

-- C++ _JustDied / events.Reset: the pumps die with the AI instance.
local function leviathanMkIIJustDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
    mkiiAssembled[creature:GetGUIDLow()] = nil
end

-- C++ boss_leviathan_mk_iiAI::KilledUnit (:788-793): player-gated; the
-- Mimiron controller Talks SAY_MKII_SLAY 3 while this AI's EventMap is
-- in PHASE_LEVIATHAN_MK_II, else SAY_V07TRON_SLAY 12 once assembled
-- (DO_ASSEMBLED_COMBAT, phase VOL7RON). FindNearestCreature is the
-- twin of the C++ instance->GetCreature(BOSS_MIMIRON) ref — the
-- controller stands in the same room.
local function leviathanMkIITargetDied(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        local assembled = mkiiAssembled[creature:GetGUIDLow()]
        mimiron:Talk(assembled and SAY_V07TRON_SLAY or SAY_MKII_SLAY)
    end
end

-- C++ IsEncounterFinished (boss_mimiron.cpp:357-382): all three
-- components at UNIT_STAND_STATE_DEAD -> Unit::Kill(who) each,
-- DespawnOrUnsummon 120s each, then the controller's JustDied arm.
-- Lua twin: the controller JustDied is delivered by killing the
-- controller (the frozen-core Kill-to-deliver-JustDied precedent);
-- C++ calls JustDied directly without killing, so the controller death
-- is the documented deviation. The two non-self components are killed
-- first; self is killed last so the rest of this handler still runs.
-- FindNearestCreature is the twin of the C++ instance GUID lookups
-- (all three stand in the same room); a missing component reads as
-- not-finished, matching the C++ nil-GUID early-out.
local function isEncounterFinished(creature, attacker)
    local mkii = creature:FindNearestCreature(NPC_LEVIATHAN_MKII, 500)
    local vx001 = creature:FindNearestCreature(NPC_VX_001, 500)
    local aerial = creature:FindNearestCreature(NPC_AERIAL_COMMAND_UNIT, 500)
    if mkii == nil or vx001 == nil or aerial == nil then
        return false
    end
    if mkii:GetStandState() ~= UNIT_STAND_STATE_DEAD or vx001:GetStandState() ~= UNIT_STAND_STATE_DEAD or aerial:GetStandState() ~= UNIT_STAND_STATE_DEAD then
        return false
    end
    local selfGuid = creature:GetGUIDLow()
    for _, part in ipairs({ mkii, vx001, aerial }) do
        if part:GetGUIDLow() ~= selfGuid then
            part:Kill(attacker)
        end
    end
    for _, part in ipairs({ mkii, vx001, aerial }) do
        part:DespawnOrUnsummon(120000)
    end
    creature:Kill(attacker)
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        mimiron:Kill(attacker)
    end
    return true
end

-- C++ boss_leviathan_mk_iiAI::DamageTaken (:698-731), in C++ order.
-- The health > 0 guard is the midnight form (ev9 can fire on an
-- already-dead creature). RemoveAllAurasExceptType has no bridge — leg
-- skipped, see header. The VOL7RON early-return skips events.Reset()
-- exactly as in C++ (the kills above fire the components' JustDied
-- arms, which cancel their timers).
local function leviathanMkIIDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health == 0 or damage < health then
        return
    end
    local guid = creature:GetGUIDLow()
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:CastSpell(creature, SPELL_VEHICLE_DAMAGED, true)
    creature:AttackStop()
    creature:SetReactState(REACT_PASSIVE)
    if not mkiiAssembled[guid] then
        -- :707-717 PHASE_LEVIATHAN_MK_II: CastStop; turret KillSelf;
        -- SetSpeedRate RUN 1.5; MovePoint to the idle position (MoveTo
        -- twin; the arrival leg below fires on ev28).
        creature:StopSpellCast()
        local kit = creature:GetVehicleKit()
        local turret = nil
        if kit ~= nil then
            turret = kit:GetPassenger(MKII_SEAT_CANNON)
        end
        if turret ~= nil then
            turret:Kill()
        end
        creature:SetSpeedRate(MOVE_RUN, 1.5)
        creature:MoveTo(WP_MKII_P1_IDLE, MKII_P1_IDLE_POS[1], MKII_P1_IDLE_POS[2], MKII_P1_IDLE_POS[3])
    else
        -- :718-728 PHASE_VOL7RON: stand dead; if the encounter is
        -- finished return early (no Reset, no self-repair); else
        -- CastStop + SELF_REPAIR.
        creature:SetStandState(UNIT_STAND_STATE_DEAD)
        if isEncounterFinished(creature, attacker) then
            return false, health - 1
        end
        creature:StopSpellCast()
        creature:CastSpell(creature, SPELL_SELF_REPAIR)
    end
    cancelTimers(guid)
    return false, health - 1
end

-- C++ boss_leviathan_mk_iiAI::MovementInform (:795-822), in C++ order.
-- Event 28 args: (event, creature, motionType, id); the four_horsemen
-- ev28 form. P4 legs: P4_POS_1 arrival schedules EVENT_MOVE_POINT_2 in
-- 1ms (MovePoint to P4_POS_2); P4_POS_2 -> MOVE_POINT_3 (P4_POS_3);
-- P4_POS_3 -> controller DoAction DO_ACTIVATE_V0L7R0N_2 (receiver live
-- in boss_mimiron.lua); P4_POS_4 -> MOVE_POINT_5 (P4_POS_5); P4_POS_5
-- arrival is the C++ default (nothing). The 1ms schedule is the C++
-- ScheduleEvent 1ms twin; the move-point timers die with Reset/JustDied
-- via cancelTimers, matching C++ events.Reset.
local function leviathanMkIIMovementInform(event, creature, motionType, id)
    if motionType ~= POINT_MOTION_TYPE then
        return
    end
    if id == WP_MKII_P1_IDLE then
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:CastSpell(creature, SPELL_HALF_HEAL)
        local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
        if mimiron ~= nil then
            mimiron:DoAction(DO_ACTIVATE_VX001)
        end
    elseif id == WP_MKII_P4_POS_1 then
        local guid = creature:GetGUIDLow()
        schedule(guid, "movepoint2", 1, function()
            creature:MoveTo(WP_MKII_P4_POS_2, MKII_P4_POS_2[1], MKII_P4_POS_2[2], MKII_P4_POS_2[3])
        end)
    elseif id == WP_MKII_P4_POS_2 then
        local guid = creature:GetGUIDLow()
        schedule(guid, "movepoint3", 1, function()
            creature:MoveTo(WP_MKII_P4_POS_3, MKII_P4_POS_3[1], MKII_P4_POS_3[2], MKII_P4_POS_3[3])
        end)
    elseif id == WP_MKII_P4_POS_3 then
        local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
        if mimiron ~= nil then
            mimiron:DoAction(DO_ACTIVATE_V0L7R0N_2)
        end
    elseif id == WP_MKII_P4_POS_4 then
        local guid = creature:GetGUIDLow()
        schedule(guid, "movepoint5", 1, function()
            creature:MoveTo(WP_MKII_P4_POS_5, MKII_P4_POS_5[1], MKII_P4_POS_5[2], MKII_P4_POS_5[3])
        end)
    end
end

RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 9, leviathanMkIIDamageTaken)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 28, leviathanMkIIMovementInform)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 3, leviathanMkIITargetDied)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 4, leviathanMkIIJustDied)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 5, leviathanMkIISpawned)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 23, leviathanMkIIReset)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 38, leviathanMkIIDoAction)
