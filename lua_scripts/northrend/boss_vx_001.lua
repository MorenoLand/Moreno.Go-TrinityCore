-- VX-001 (Ulduar, Mimiron phase 2) -- Lua port of boss_vx_001AI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_VX_001 = 33651, NPC_BURST_TARGET = 34211.
-- DoAction actions (boss_mimiron.cpp:280-299): DO_START_VX001 = 3,
-- DO_HARDMODE_VX001 = 4, DO_ASSEMBLED_COMBAT = 12. Phases
-- (:310-314): PHASE_VX_001 = 2, PHASE_VOL7RON = 4 (Lua-side phase
-- string in vxState; no EventMap phase bridge exists).
--
-- PORTED (this run, event 38 handler vx001DoAction):
--   DoAction DO_HARDMODE_VX001 (:982-987): _fireFighter = true;
--   self-cast SPELL_EMERGENCY_MODE 64582; schedule FROST_BOMB 1s and
--   FLAME_SUPPRESSANT_VX 6s; then FALLS THROUGH to the DO_START_VX001
--   legs, same order as C++ ([[fallthrough]]).
--   DoAction DO_START_VX001 (:988-1000): RemoveFlag(NON_ATTACKABLE |
--   NOT_SELECTABLE); RemoveAura(SPELL_FREEZE_ANIM 16245) (the
--   RemoveAurasDueToSpell twin); EmoteState(0) (EMOTE_ONESHOT_NONE
--   clears; the SetUInt32Value(UNIT_NPC_EMOTESTATE) twin,
--   lua_creature_events.go:4342); self-cast SPELL_HEAT_WAVE_AURA
--   63679; phase = VX_001; schedule ROCKET_STRIKE 20s, SPINNING_UP
--   {30s,35s}, RAPID_BURST 500ms. DEVIATION: SetImmuneToPC(false)
--   has NO bridge — the leg is skipped (VX-001 is spawned
--   non-attackable and the flag clear is the effective twin).
--   DoAction DO_ASSEMBLED_COMBAT (:1001-1010): SetStandState(STAND);
--   RemoveFlag as above; phase = VOL7RON; cancel the phase-1-only
--   pumps (rapidBurst, flameSuppressant — the C++ phase-grouped
--   schedules drop them); schedule ROCKET_STRIKE 20s, SPINNING_UP
--   {30s,35s}, HAND_PULSE 500ms; if _fireFighter, FROST_BOMB 1s.
--   Senders are LIVE in boss_mimiron.lua: vx001Activation9 sends 3/4
--   (DO_START/HARDMODE_VX001, :567) and vol7ronActivation7 sends 12
--   (:639); the old "DoAction sender chains are unported" note is
--   RETRACTED.
--   UpdateAI pumps (:1044-1093), self-re-arming, C++ delays exact:
--   RAPID_BURST (:1062-1064): random alive player within 120yd
--   (GetPlayersInRange(120), alive-only by default — the razorscale /
--   gothik precedent; C++ SelectTarget(Random, 0, 120, true) reads
--   the threat list, so off-threat players are the standing delta);
--   cast SPELL_SUMMON_BURST_TARGET 64840 on them; re-arm 3s.
--   ROCKET_STRIKE (:1066-1069): DoCastAOE phase-dependent — VX phase
--   SPELL_ROCKET_STRIKE_SINGLE 64402, VOL7RON phase
--   SPELL_ROCKET_STRIKE_BOTH 65034; re-arm {20s,25s}. EVENT_RELOAD
--   (10s) is NOT scheduled: its legs read the vehicle kit
--   (GetVehicleKit()->GetPassenger + SetDisplayId), which has no
--   bridge — display-only, no combat effect.
--   HAND_PULSE (:1077-1079): random alive player within 120yd; cast
--   RAND(SPELL_HAND_PULSE_LEFT 64348, SPELL_HAND_PULSE_RIGHT 64352);
--   re-arm {1500ms,3s}.
--   FROST_BOMB (:1081-1083): DoCastAOE SPELL_SCRIPT_EFFECT_FROST_BOMB
--   64623; re-arm 45s.
--   SPINNING_UP (:1085-1088): DoCastAOE SPELL_SPINNING_UP 63414;
--   DelayEvents(14s) on all currently-armed timer keys (the ymiron
--   delayTimers idiom — the spinning-up key itself is not yet
--   re-armed at that point, matching C++); re-arm {55s,65s}.
--   FLAME_SUPPRESSANT_VX (:1090-1092): DoCastAOE
--   SPELL_FLAME_SUPPRESSANT_VX 65192; re-arm {10s,12s}.
--   C++ UpdateAI CASTING gate (:1051-1057): every pump checks
--   creature:IsCasting() first and on true re-schedules the same
--   key in 500ms WITHOUT taking the re-arm decision (the
--   amanitar / razorscale idiom); IsCasting reads motion.CastingUntil
--   (lua_creature_events.go:2208).
--   Ctor (:936-941, ev5 spawn handler): SetDisableGravity(true),
--   EmoteState(451) (EMOTE_STATE_SPECIAL_UNARMED, SharedDefines.h:
--   2088), SetReactState(REACT_PASSIVE 0); per-GUID state reset
--   (fireFighter=false, phase=nil) — the ctor runs once per AI
--   instance, so the spawn handler is its twin.
--   Reset (ev23): cancel timers + clear phase (BossAI::_Reset
--   events.Reset()); _fireFighter is NOT cleared (C++ member, set
--   only by the ctor and DoAction). JustDied (ev4): cancel timers
--   only.
--
-- PORTED (tick 195, RegisterAuraEvent):
--   spell_mimiron_rapid_burst (63382) HandleDummyTick
--   (boss_mimiron.cpp:2212-2217): EFFECT_1 PERIODIC_DUMMY; the caster
--   (VX-001) casts RAPID_BURST_LEFT 63387 on the aura target on odd
--   ticks and RAPID_BURST_RIGHT 64019 on even ticks, triggered by the
--   aura effect. No PreventDefaultAction in C++, so no return here.
--   The AfterRemove arm (:2207-2210, target TempSummon DespawnOrUnsummon)
--   stays out: no aura apply/remove hook exists on the Lua surface.
--
-- DamageTaken (:943-975) PORTED tick 561 below: the 1-HP latch, the
-- PHASE_VX_001 branch (HALF_HEAL / TORSO_DISABLED / DO_ACTIVATE_AERIAL
-- relay) and the PHASE_VOL7RON branch (stand-DEAD, IsEncounterFinished
-- finish path, SELF_REPAIR). Stay-outs inside it: RemoveAllAurasExceptType
-- (no bridge) and EnterEvadeMode (:1013-1016, summons.DespawnAll) — no
-- summon list is kept here.
-- The rapid-burst arms stay dormant until the EVENT_RAPID_BURST
-- timer lands, since burst targets are created only by it.
-- KilledUnit (:1030-1034) PORTED tick 559 below: player-gated, the
-- controller (FindNearestCreature twin of the instance ref) Talks
-- SAY_VX001_SLAY 6 in PHASE_VX_001, SAY_V07TRON_SLAY 12 in VOL7RON.

local NPC_VX_001 = 33651
local NPC_BURST_TARGET = 34211
local NPC_MIMIRON = 33350 -- ulduar.h (controller, KilledUnit Talk target)

-- boss_mimiron.cpp:36-47 Yells (controller Talk ids).
local SAY_VX001_SLAY = 6
local SAY_V07TRON_SLAY = 12

local DO_START_VX001 = 3
local DO_HARDMODE_VX001 = 4
local DO_ASSEMBLED_COMBAT = 12

local SPELL_RAPID_BURST = 63382
local SPELL_RAPID_BURST_LEFT = 63387
local SPELL_RAPID_BURST_RIGHT = 64019
local SPELL_RAPID_BURST_TARGET_ME = 64841
local SPELL_SUMMON_BURST_TARGET = 64840

local SPELL_EMERGENCY_MODE = 64582
local SPELL_FREEZE_ANIM = 16245
local SPELL_HEAT_WAVE_AURA = 63679
local SPELL_ROCKET_STRIKE_SINGLE = 64402
local SPELL_ROCKET_STRIKE_BOTH = 65034
local SPELL_SPINNING_UP = 63414
local SPELL_SCRIPT_EFFECT_FROST_BOMB = 64623
local SPELL_FLAME_SUPPRESSANT_VX = 65192
local SPELL_HAND_PULSE_LEFT = 64348
local SPELL_HAND_PULSE_RIGHT = 64352
local SPELL_VEHICLE_DAMAGED = 63415
local SPELL_HALF_HEAL = 64188
local SPELL_TORSO_DISABLED = 64120
local SPELL_SELF_REPAIR = 64383

-- boss_mimiron.cpp:280-299 Actions enum: DO_ACTIVATE_AERIAL = 5 (the
-- Mimiron-controller relay the PHASE_VX_001 DamageTaken branch sends).
local DO_ACTIVATE_AERIAL = 5

local NPC_LEVIATHAN_MKII = 33432
local NPC_AERIAL_COMMAND_UNIT = 33670

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local EMOTE_ONESHOT_NONE = 0
local EMOTE_STATE_SPECIAL_UNARMED = 451
local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_DEAD = 7 -- UnitDefines.h:40
local REACT_PASSIVE = 0

local timers = {}

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
    -- concrete due time is recorded for delayTimers.
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

-- C++ EventMap::DelayEvents with no key list: delay every currently
-- armed timer (the SPINNING_UP arm, :1087, delays the whole map).
local function delayAllTimers(guid, extraMs)
    local per = timers[guid]
    if not per then
        return
    end
    local keys = {}
    for k in pairs(per) do
        keys[#keys + 1] = k
    end
    delayTimers(guid, keys, extraMs)
end

-- Per-GUID AI-instance state: the Lua twin of the boss_vx_001AI
-- members _fireFighter and the EventMap phase (PHASE_VX_001 /
-- PHASE_VOL7RON). Created on spawn (ctor twin), dropped on death is
-- unnecessary — ev5 resets it on respawn.
local vxState = {}

local function vxStateFor(guid)
    local st = vxState[guid]
    if not st then
        st = { fireFighter = false, phase = nil }
        vxState[guid] = st
    end
    return st
end

-- C++ SelectTarget(Random, 0, 120, true) (UnitAI.h:170): random over
-- the threat list, alive-only, 120yd. Lua twin: GetPlayersInRange(120)
-- (alive-only by default) + math.random — the razorscale / gothik
-- precedent; off-threat players are the standing delta.
local function randomPlayerInRange(creature, range)
    local players = creature:GetPlayersInRange(range)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_RAPID_BURST arm (:1062-1064): random alive player within
-- 120yd -> DoCast(SPELL_SUMMON_BURST_TARGET 64840); re-arm 3s.
local function rapidBurstPump(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "rapidBurst", 500, function()
            rapidBurstPump(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 120)
    if target then
        creature:CastSpell(target, SPELL_SUMMON_BURST_TARGET)
    end
    schedule(guid, "rapidBurst", 3000, function()
        rapidBurstPump(creature, guid)
    end)
end

-- C++ EVENT_ROCKET_STRIKE arm (:1066-1069): DoCastAOE phase-dependent
-- (SINGLE 64402 in PHASE_VX_001, BOTH 65034 in PHASE_VOL7RON);
-- re-arm {20s,25s}. EVENT_RELOAD (10s) is NOT scheduled — its legs
-- read the vehicle kit (GetVehicleKit()->GetPassenger + SetDisplayId),
-- display-only, no bridge.
local function rocketStrikeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "rocketStrike", 500, function()
            rocketStrikeTick(creature, guid)
        end)
        return
    end
    local st = vxState[guid]
    local spell = SPELL_ROCKET_STRIKE_SINGLE
    if st and st.phase == "VOL7RON" then
        spell = SPELL_ROCKET_STRIKE_BOTH
    end
    creature:CastSpell(creature, spell)
    schedule(guid, "rocketStrike", {20000, 25000}, function()
        rocketStrikeTick(creature, guid)
    end)
end

-- C++ EVENT_SPINNING_UP arm (:1085-1088): DoCastAOE(SPELL_SPINNING_UP
-- 63414); DelayEvents(14s) on the whole map; re-arm {55s,65s}.
local function spinningUpTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "spinningUp", 500, function()
            spinningUpTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SPINNING_UP)
    delayAllTimers(guid, 14000)
    schedule(guid, "spinningUp", {55000, 65000}, function()
        spinningUpTick(creature, guid)
    end)
end

-- C++ EVENT_FROST_BOMB arm (:1081-1083): DoCastAOE(SPELL_SCRIPT_EFFECT_
-- FROST_BOMB 64623); re-arm 45s.
local function frostBombTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostBomb", 500, function()
            frostBombTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SCRIPT_EFFECT_FROST_BOMB)
    schedule(guid, "frostBomb", 45000, function()
        frostBombTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_SUPPRESSANT_VX arm (:1090-1092): DoCastAOE(SPELL_
-- FLAME_SUPPRESSANT_VX 65192); re-arm {10s,12s}, phase-grouped to
-- PHASE_VX_001 (cancelled on DO_ASSEMBLED_COMBAT).
local function flameSuppressantTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flameSuppressant", 500, function()
            flameSuppressantTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FLAME_SUPPRESSANT_VX)
    schedule(guid, "flameSuppressant", {10000, 12000}, function()
        flameSuppressantTick(creature, guid)
    end)
end

-- C++ EVENT_HAND_PULSE arm (:1077-1079): random alive player within
-- 120yd -> DoCast(RAND(SPELL_HAND_PULSE_LEFT 64348, SPELL_HAND_PULSE_
-- RIGHT 64352)); re-arm {1500ms,3s}, phase-grouped to PHASE_VOL7RON.
local function handPulseTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "handPulse", 500, function()
            handPulseTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 120)
    if target then
        local spell = SPELL_HAND_PULSE_LEFT
        if math.random(2) == 2 then
            spell = SPELL_HAND_PULSE_RIGHT
        end
        creature:CastSpell(target, spell)
    end
    schedule(guid, "handPulse", {1500, 3000}, function()
        handPulseTick(creature, guid)
    end)
end

-- C++ boss_vx_001AI::DoAction (:978-1001), event 38 args:
-- (event, creature, action). Action 4 falls through to action 3 in
-- C++ ([[fallthrough]]), so the hardmode legs run first and the
-- start legs run for both 3 and 4.
local function vx001DoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    local st = vxStateFor(guid)
    if action == DO_HARDMODE_VX001 then
        st.fireFighter = true
        creature:CastSpell(creature, SPELL_EMERGENCY_MODE)
        schedule(guid, "frostBomb", 1000, function()
            frostBombTick(creature, guid)
        end)
        schedule(guid, "flameSuppressant", 6000, function()
            flameSuppressantTick(creature, guid)
        end)
    end
    if action == DO_START_VX001 or action == DO_HARDMODE_VX001 then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        -- SetImmuneToPC(false) has NO bridge — leg skipped (see header).
        creature:RemoveAura(SPELL_FREEZE_ANIM)
        creature:EmoteState(EMOTE_ONESHOT_NONE)
        creature:CastSpell(creature, SPELL_HEAT_WAVE_AURA)
        st.phase = "VX_001"
        schedule(guid, "rocketStrike", 20000, function()
            rocketStrikeTick(creature, guid)
        end)
        schedule(guid, "spinningUp", {30000, 35000}, function()
            spinningUpTick(creature, guid)
        end)
        schedule(guid, "rapidBurst", 500, function()
            rapidBurstPump(creature, guid)
        end)
    elseif action == DO_ASSEMBLED_COMBAT then
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        st.phase = "VOL7RON"
        cancelTimerKey(guid, "rapidBurst")
        cancelTimerKey(guid, "flameSuppressant")
        schedule(guid, "rocketStrike", 20000, function()
            rocketStrikeTick(creature, guid)
        end)
        schedule(guid, "spinningUp", {30000, 35000}, function()
            spinningUpTick(creature, guid)
        end)
        schedule(guid, "handPulse", 500, function()
            handPulseTick(creature, guid)
        end)
        if st.fireFighter then
            schedule(guid, "frostBomb", 1000, function()
                frostBombTick(creature, guid)
            end)
        end
    end
end

-- C++ boss_vx_001AI ctor (:936-941): SetDisableGravity(true),
-- SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_STATE_SPECIAL_UNARMED
-- 451), SetReactState(REACT_PASSIVE), _fireFighter = false. Event 5
-- args: (event, creature); the ctor runs once per AI instance, so the
-- spawn handler resets the per-GUID state.
local function vx001Spawn(event, creature)
    local guid = creature:GetGUIDLow()
    vxState[guid] = { fireFighter = false, phase = nil }
    creature:SetDisableGravity(true)
    creature:EmoteState(EMOTE_STATE_SPECIAL_UNARMED)
    creature:SetReactState(REACT_PASSIVE)
end

-- C++ BossAI::Reset -> events.Reset(): cancel timers, clear phase.
-- _fireFighter is a member, untouched by Reset.
local function vx001Reset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local st = vxState[guid]
    if st then
        st.phase = nil
    end
end

local function vx001JustDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ IsEncounterFinished (boss_mimiron.cpp:352-377): all three
-- vehicles stand-DEAD -> Kill + DespawnOrUnsummon(120s) each, then the
-- Mimiron controller's JustDied. FindNearestCreature(500) is the twin
-- of the C++ instance GUID reads; GetStandState is bridged
-- (lua_creature_events.go:1168). The caller's own stand state must
-- already be DEAD when this runs (C++ sets it before the check).
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
    local killer = attacker or creature
    -- The two non-self components are killed first; self is killed
    -- last so the rest of this handler still runs.
    local selfGuid = creature:GetGUIDLow()
    for _, part in ipairs({ mkii, vx001, aerial }) do
        if part:GetGUIDLow() ~= selfGuid then
            part:Kill(killer)
        end
    end
    for _, part in ipairs({ mkii, vx001, aerial }) do
        part:DespawnOrUnsummon(120000)
    end
    creature:Kill(killer)
    -- C++ calls mimiron->AI()->JustDied(who) directly while Mimiron
    -- lives; Lua has no cross-creature JustDied call, so Kill fires
    -- the ev4 handler (boss_mimiron.lua mimironDied), which IS the C++
    -- JustDied port (kiljaeden precedent). OUTTRO_3 despawns him 21s
    -- later, matching the C++ end state.
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        mimiron:Kill(killer)
    end
    return true
end

-- C++ boss_vx_001AI::DamageTaken (:944-975), event 9 args:
-- (event, creature, attacker, damage). Only lethal hits act (damage
-- >= health); the lich-king damage-rewrite form returns false,
-- health - 1. Common legs in C++ order: AttackStop, triggered
-- self-cast VEHICLE_DAMAGED 63415. RemoveAllAurasExceptType
-- (CONTROL_VEHICLE, MOD_INCREASE_HEALTH_PERCENT) has NO bridge --
-- leg skipped (the RemoveAllAuras bridge would strip the
-- vehicle-control aura too). PHASE_VX_001 (:953-961): CastStop,
-- NON_ATTACKABLE flag, HALF_HEAL 64188 + TORSO_DISABLED 64120
-- self-casts, Mimiron DoAction DO_ACTIVATE_AERIAL (5) -- the
-- aerialActivation chain in boss_mimiron.lua is live. PHASE_VOL7RON
-- (:962-975): stand-DEAD, NON_ATTACKABLE, IsEncounterFinished
-- (returns early when the encounter is finished), CastStop,
-- SELF_REPAIR 64383. The C++ arm ends with events.Reset, so the
-- pumps are cancelled on a lethal hit -- except on the VOL7RON
-- finish-path early return, where C++ returns before the Reset (the
-- kills fire the components' JustDied arms, which cancel theirs).
-- The health > 0 guard is the midnight form (ev9 can fire on an
-- already-dead creature).
local function vx001DamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health == 0 or damage < health then
        return
    end
    local guid = creature:GetGUIDLow()
    creature:AttackStop()
    creature:CastSpell(creature, SPELL_VEHICLE_DAMAGED, true)
    local st = vxState[guid]
    local phase = st and st.phase
    if phase == "VX_001" then
        creature:StopSpellCast()
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:CastSpell(creature, SPELL_HALF_HEAL)
        creature:CastSpell(creature, SPELL_TORSO_DISABLED)
        local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
        if mimiron ~= nil then
            mimiron:DoAction(DO_ACTIVATE_AERIAL)
        end
    elseif phase == "VOL7RON" then
        creature:SetStandState(UNIT_STAND_STATE_DEAD)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        if isEncounterFinished(creature, attacker) then
            return false, health - 1
        end
        creature:StopSpellCast()
        creature:CastSpell(creature, SPELL_SELF_REPAIR)
    end
    cancelTimers(guid)
    return false, health - 1
end

-- C++ JustSummoned (1022-1027): a summoned burst target casts
-- RAPID_BURST_TARGET_ME on VX-001. Event 19 args: (event, creature, summoned);
-- the summoned object is a full handler creature, so it can cast.
local function vx001JustSummoned(event, creature, summoned)
    if summoned ~= nil and summoned:GetEntry() == NPC_BURST_TARGET then
        summoned:CastSpell(creature, SPELL_RAPID_BURST_TARGET_ME)
    end
end

-- C++ boss_vx_001AI::KilledUnit (:1030-1034): player-gated; the Mimiron
-- controller Talks SAY_VX001_SLAY 6 while this AI's EventMap is in
-- PHASE_VX_001, else SAY_V07TRON_SLAY 12 (VOL7RON). Lua phase mirror
-- is vxState[guid].phase (nil before START reads as not-VX_001, matching
-- C++ IsInPhase false before SetPhase). FindNearestCreature is the twin
-- of the C++ instance->GetCreature(BOSS_MIMIRON) ref.
local function vx001TargetDied(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        local st = vxState[creature:GetGUIDLow()]
        local inPhase = st ~= nil and st.phase == "VX_001"
        mimiron:Talk(inPhase and SAY_VX001_SLAY or SAY_V07TRON_SLAY)
    end
end

-- C++ SpellHit (1036-1040): if the hitting caster is a burst target and
-- VX-001 is not casting, VX-001 casts RAPID_BURST on it. Event 14 args:
-- (event, creature, caster, spellId); GetEntry resolves via the generic
-- object fallback. IsCasting reads the live CastingUntil window
-- (lua_creature_events.go:2208), so the gate works as in C++.
local function vx001SpellHit(event, creature, caster, spellId)
    if caster ~= nil and caster:GetEntry() == NPC_BURST_TARGET and not creature:IsCasting() then
        creature:CastSpell(caster, SPELL_RAPID_BURST)
    end
end

local function rapidBurstTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        local spell = SPELL_RAPID_BURST_LEFT
        if aura:GetTickNumber() % 2 == 0 then
            spell = SPELL_RAPID_BURST_RIGHT
        end
        caster:CastSpell(target, spell, true)
    end
end

RegisterCreatureEvent(NPC_VX_001, 5, vx001Spawn)
RegisterCreatureEvent(NPC_VX_001, 9, vx001DamageTaken)
RegisterCreatureEvent(NPC_VX_001, 38, vx001DoAction)
RegisterCreatureEvent(NPC_VX_001, 23, vx001Reset)
RegisterCreatureEvent(NPC_VX_001, 3, vx001TargetDied)
RegisterCreatureEvent(NPC_VX_001, 4, vx001JustDied)
RegisterCreatureEvent(NPC_VX_001, 19, vx001JustSummoned)
RegisterCreatureEvent(NPC_VX_001, 14, vx001SpellHit)
RegisterAuraEvent(SPELL_RAPID_BURST, 1, rapidBurstTick)
