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
-- cancels below. The DamageTaken lethal clamp stays OUT: phase-gated
-- (phase-1 turret kill + MovePoint to idle vs phase-4 stand-dead
-- + self-repair) behind the unbridged phase machine plus
-- the vehicle-kit and MovePoint legs. The GetData/SetData
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

local timers = {}

-- Per-GUID assembled latch: the Lua mirror of the EventMap phase
-- (PHASE_LEVIATHAN_MK_II -> PHASE_VOL7RON on DO_ASSEMBLED_COMBAT),
-- read by the KilledUnit arm; cleared on Reset/spawn/death.
local mkiiAssembled = {}

-- GetCurrTime is wall-clock ms (Eluna global, engine globals.go:941);
-- os.clock() is CPU time and stalls while the server idles, which would
-- corrupt the due/remaining math used by delayTimers (thekal idiom).
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

-- C++ EventMap::DelayEvents (thekal idiom): push the named armed timers
-- out by extraMs, preserving each timer's remaining time.
local function delayTimers(guid, keys, extraMs)
    local per = timers[guid]
    if not per then
        return
    end
    local now = nowMs()
    for _, key in ipairs(keys) do
        local entry = per[key]
        if entry then
            RemoveEventById(entry.id)
            local remaining = entry.due - now
            if remaining < 0 then
                remaining = 0
            end
            per[key] = armEntry(guid, key, remaining + extraMs, entry.fn)
        end
    end
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
-- (instance boss-state reset) has no bridge and stays out; the
-- creature-local legs port C++-exact, in C++ order: set
-- NON_ATTACKABLE | NOT_SELECTABLE, react passive, self-cast
-- FREEZE_ANIM. The _fireFighter/_setupMine/_setupBomb/_setupRocket
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

RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 3, leviathanMkIITargetDied)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 4, leviathanMkIIJustDied)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 5, leviathanMkIISpawned)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 23, leviathanMkIIReset)
RegisterCreatureEvent(NPC_LEVIATHAN_MKII, 38, leviathanMkIIDoAction)
