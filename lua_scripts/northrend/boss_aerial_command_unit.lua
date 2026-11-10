-- Aerial Command Unit (Ulduar, Mimiron phase 3) -- Lua port of the
-- engine-fired arms of boss_aerial_command_unitAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1113-1280).
-- Entry (ulduar.h): NPC_AERIAL_COMMAND_UNIT = 33670.
--
-- PORTED (engine-fired hooks only, C++-exact spell IDs / semantics):
--   constructor (:1120): SetReactState(REACT_PASSIVE) + SetDisableGravity
--     (true), on spawn (event 5); per-GUID fireFighter latch reset here.
--   DamageTaken (:1127) phase-independent legs: on a lethal hit, clamp
--     damage to health-1 (Lua `return false, health - 1`, the lich-king
--     damage-rewrite form), SetReactState passive, AttackStop,
--     SetDisableGravity(true), triggered self-cast VEHICLE_DAMAGED;
--     phase branches PORTED tick 561 below (MoveTo P4 pos / VOL7RON
--     stand-DEAD + IsEncounterFinished + SELF_REPAIR).
--   DoAction (:1159) whole switch (event 38, `aerialDoAction`): the
--     6/7/12 senders are LIVE in boss_mimiron.lua -- aerialActivation6
--     sends DO_START_AERIAL (6) / DO_HARDMODE_AERIAL (7),
--     vol7ronActivation7 sends DO_ASSEMBLED_COMBAT (12). Arms, in C++
--     order:
--     7 DO_HARDMODE_AERIAL: fireFighter latch + self-cast EMERGENCY_MODE
--       64582 + EVENT_SUMMON_FIRE_BOTS in 1s, then falls through to the
--       START legs below (same order as C++).
--     6 DO_START_AERIAL: SetDisableGravity(false), SetHover(true),
--       RemoveFlag NON_ATTACKABLE|NOT_SELECTABLE, react aggressive,
--       JUNK_BOT 5s / ASSAULT_BOT 9s / BOMB_BOT 9s. SetImmuneToPC(false)
--       has NO binding -- leg skipped (deviation, see below).
--     8 DO_DISABLE_AERIAL: StopSpellCast, react passive, AttackStop,
--       MoveFall (bridged, lua_creature_events.go:3391), DelayEvents 23s
--       on the four armed summon keys (ymiron delayTimers idiom).
--     9 DO_ENABLE_AERIAL: react aggressive.
--     12 DO_ASSEMBLED_COMBAT: RemoveFlag as above, react aggressive,
--       SetStandState STAND, all four summon pumps cancelled (phase-3-only
--       in C++; the cancel is the Lua twin of the C++ phase change to
--       PHASE_VOL7RON, which has no Lua read).
--     Actions 8/9 are now SENT by the magnetic-core aura handlers
--     ported tick 561 below (RegisterAuraEvent ev2/ev3 + RegisterSpellEvent
--     ev7 filter on SPELL_MAGNETIC_CORE 64436). DORMANT: the aura is
--     applied only by the npc_mimiron_magnetic_core Reset DoCastAOE
--     (boss_mimiron.cpp:1661-1674), whose entry has no C++ constant and
--     whose spawn path is not in boss_mimiron.cpp -- the core NPC file
--     waits on a verified entry (devouring-flame standard: never
--     register on an unverified entry).
--   UpdateAI summon pumps (:1233, self-re-arming; C++ delays exact):
--     firebots: DoCastAOE SUMMON_FIRE_BOT_TRIGGER 64620, triggered,
--       re-arm 45s. junkbot: DoCastAOE SUMMON_JUNK_BOT_TRIGGER 63820,
--       triggered, re-arm {11s,12s}. assaultbot: DoCastAOE
--       SUMMON_ASSAULT_BOT_TRIGGER 64425, triggered, re-arm 30s. bombbot:
--       self-cast SUMMON_BOMB_BOT 63811, re-arm {15s,20s}. C++ UpdateAI
--       CASTING gate: on IsCasting, re-schedule the same key in 500ms
--       WITHOUT taking the re-arm decision (amanitar/razorscale idiom).
--     DELTA: C++ adds SPELLVALUE_MAX_TARGETS (3 fire / 1 junk / 1
--       assault) via CastSpellExtraArgs -- no Lua twin; the triggers
--       cast without the cap.
--   JustSummoned (:1206, ev19 `aerialJustSummoned`): if the per-GUID
--     fireFighter latch is set and the summoned entry is NPC_ASSAULT_BOT
--     34057 or NPC_JUNK_BOT 33855, summoned self-casts EMERGENCY_MODE
--     64582. (BossAI summon-tracking leg is engine-side.)
--   Reset (ev23) / JustDied (ev4): cancel all Lua timers (C++
--     events.Reset twin; the C++ Reset arm is otherwise engine-side).
--
-- DOCUMENTED-ONLY (no behavior fabricated):
--   UpdateAI plasma-ball attack: DoSpellAttackIfReady (PLASMA_BALL_P1
--     63689 in phase 3, PLASMA_BALL_P2 65647 in phase 4) -- no bridge.
--   DamageTaken phase branches PORTED tick 561 below: PHASE_AERIAL_
--     COMMAND_UNIT MovePoint to WP_AERIAL_P4_POS via the MoveTo twin
--     (MKII precedent, lua_creature_events.go:4875); PHASE_VOL7RON
--     stand-DEAD + IsEncounterFinished + SELF_REPAIR 64383, phase read
--     from the acuState mirror (set in DoAction 6/12). Stay-outs inside
--     it: SetAnimationTier(Ground) (no binding) and GetMotionMaster()->
--     Clear() (no binding; MoveTo re-issues the motion). The C++ lethal
--     leg also runs events.Reset; the Lua arm cancels the summon pumps
--     on the lethal hit (tick 557) to match.
--   EnterEvadeMode (:1201, summons.DespawnAll) -- out: DespawnAll has no
--     binding, and there is NO engine-fired evade event hook (the
--     creature-event list has no OnEvade; 25 is unassigned). CORRECTION
--     of the old header note: creature:EnterEvadeMode() as a callable
--     method IS bridged (lua_creature_events.go:5051, triggerCreatureEvade
--     leash twin, creaturemotion.go:4664) -- but that is the invoke side,
--     not a callback for the C++ override, so the arm still stays out.
--   KilledUnit (:1213, SAY_AERIAL_SLAY 9 / SAY_V07TRON_SLAY 12 via the
--     mimiron controller) PORTED tick 559 below: player-gated, the
--     controller (FindNearestCreature twin of the instance ref) Talks
--     per the Lua phase mirror in acuState (AERIAL_COMMAND_UNIT vs
--     VOL7RON, set in DoAction 6/12).
--   MovementInform (:1220, WP_AERIAL_P4_POS facing + NOT_SELECTABLE flag
--     + CLEAR_ALL_DEBUFFS + DO_ACTIVATE_V0L7R0N_1) PORTED tick 561
--     below: reachable now the DamageTaken MovePoint leg lands via
--     MoveTo (ev28 arrival); the DO_ACTIVATE_V0L7R0N_1 receiver is
--     live in boss_mimiron.lua.
--
-- BRIDGE NOTES (checked against the CURRENT runtime, not memory):
--   SetDisableGravity, SetHover, SetReactState, AttackStop, SetStandState,
--   SetFacingTo, Talk, CastSpell(..., triggered), StopSpellCast
--   (lua_creature_events.go:1073), MoveFall (lua_creature_events.go:3391),
--   IsCasting, GetGUIDLow, GetEntry are all bound on the handler
--   surface. Unbound, verified by grep of lua_creature_events.go /
--   lua_objects.go / scripting/object.go: SetImmuneToPC,
--   SetAnimationTier, MovePoint, GetMotionMaster, DespawnAll,
--   GetVehicleBase, RemoveAurasDueToSpell, SPELLVALUE_MAX_TARGETS.
--   DoAction dispatch itself is event 38 (runtime.go:411,
--   lua_creature_events.go:827).

local NPC_AERIAL_COMMAND_UNIT = 33670
local NPC_ASSAULT_BOT = 34057
local NPC_JUNK_BOT = 33855
local NPC_MIMIRON = 33350 -- ulduar.h (controller, KilledUnit Talk target)
local NPC_LEVIATHAN_MKII = 33432
local NPC_VX_001 = 33651

-- boss_mimiron.cpp:36-47 Yells (controller Talk ids).
local SAY_AERIAL_SLAY = 9
local SAY_V07TRON_SLAY = 12

local SPELL_VEHICLE_DAMAGED = 63415
local SPELL_EMERGENCY_MODE = 64582
local SPELL_SUMMON_FIRE_BOT_TRIGGER = 64620
local SPELL_SUMMON_JUNK_BOT_TRIGGER = 63820
local SPELL_SUMMON_ASSAULT_BOT_TRIGGER = 64425
local SPELL_SUMMON_BOMB_BOT = 63811
local SPELL_SELF_REPAIR = 64383 -- boss_mimiron.cpp:175
local SPELL_CLEAR_ALL_DEBUFFS = 34098 -- boss_mimiron.cpp:179
local SPELL_MAGNETIC_CORE = 64436 -- boss_mimiron.cpp:176
local SPELL_MAGNETIC_CORE_VISUAL = 64438 -- boss_mimiron.cpp:177

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2 -- UnitDefines.h: REACT_DEFENSIVE=1, REACT_AGGRESSIVE=2 (tick 557 fix: was 1)
local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_DEAD = 7 -- UnitDefines.h:40

-- enum Actions (boss_mimiron.cpp:283-298): DO_START_MKII 0,
-- DO_HARDMODE_MKII 1, DO_ACTIVATE_VX001 2, DO_START_VX001 3,
-- DO_HARDMODE_VX001 4, DO_ACTIVATE_AERIAL 5, then the ACU actions.
local DO_START_AERIAL = 6
local DO_HARDMODE_AERIAL = 7
local DO_DISABLE_AERIAL = 8
local DO_ENABLE_AERIAL = 9
local DO_ASSEMBLED_COMBAT = 12
local DO_ACTIVATE_V0L7R0N_1 = 10 -- boss_mimiron.cpp:296
local WP_AERIAL_P4_POS = 7 -- boss_mimiron.cpp:325 (Waypoints enum)
local POINT_MOTION_TYPE = 8
-- VehicleRelocation[WP_AERIAL_P4_POS], boss_mimiron.cpp:345.
local ACU_P4_POS = { 2744.62, 2569.41, 382.0 }
local ACU_P4_ORIENTATION = 3.054326

local timers = {}
local acuState = {}

-- Timer block copied from boss_ymiron.lua: GetCurrTime is wall-clock ms
-- (Eluna global, engine globals.go:941); os.clock() is CPU time and stalls
-- while the server idles, which would corrupt the due/remaining math used
-- by delayTimers (thekal idiom).
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

-- C++ UpdateAI (:1248): DoCastAOE SUMMON_FIRE_BOT_TRIGGER, triggered,
-- max targets 3 (no Lua twin -- see header delta); re-arm 45s.
local function summonFireBots(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "firebots", 500, function()
            summonFireBots(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_FIRE_BOT_TRIGGER, true)
    schedule(guid, "firebots", 45000, function()
        summonFireBots(creature, guid)
    end)
end

-- C++ UpdateAI (:1252): DoCastAOE SUMMON_JUNK_BOT_TRIGGER, triggered,
-- max targets 1 (no Lua twin); re-arm {11s,12s}.
local function summonJunkBot(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "junkbot", 500, function()
            summonJunkBot(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_JUNK_BOT_TRIGGER, true)
    schedule(guid, "junkbot", { 11000, 12000 }, function()
        summonJunkBot(creature, guid)
    end)
end

-- C++ UpdateAI (:1256): DoCastAOE SUMMON_ASSAULT_BOT_TRIGGER, triggered,
-- max targets 1 (no Lua twin); re-arm 30s.
local function summonAssaultBot(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "assaultbot", 500, function()
            summonAssaultBot(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_ASSAULT_BOT_TRIGGER, true)
    schedule(guid, "assaultbot", 30000, function()
        summonAssaultBot(creature, guid)
    end)
end

-- C++ UpdateAI (:1260): self-cast SUMMON_BOMB_BOT (not triggered, not
-- AOE); re-arm {15s,20s}.
local function summonBombBot(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bombbot", 500, function()
            summonBombBot(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_BOMB_BOT)
    schedule(guid, "bombbot", { 15000, 20000 }, function()
        summonBombBot(creature, guid)
    end)
end

-- C++ constructor (:1120): react passive + disable gravity. The
-- fireFighter latch is a per-instance member in C++; Lua resets it per
-- spawn (the ACU is summoned fresh on each phase-3 activation).
local function aerialSpawn(event, creature)
    acuState[creature:GetGUIDLow()] = { fireFighter = false, phase = nil }
    creature:SetReactState(REACT_PASSIVE)
    creature:SetDisableGravity(true)
end

-- C++ IsEncounterFinished (boss_mimiron.cpp:357-382) — the MKII/VX-001
-- twin: all three components at UNIT_STAND_STATE_DEAD -> Unit::Kill
-- each, DespawnOrUnsummon 120s each, then the controller's JustDied
-- arm. Lua delivers the controller JustDied by killing the controller
-- (C++ calls JustDied directly without killing; documented deviation).
-- FindNearestCreature is the twin of the C++ instance GUID lookups.
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
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        mimiron:Kill(killer)
    end
    return true
end

-- C++ DamageTaken (:1127-1157), in C++ order. The health > 0 guard is
-- the midnight form (ev9 can fire on an already-dead creature).
-- SetAnimationTier(Ground) has no bridge -- leg skipped, see header.
-- PHASE_AERIAL_COMMAND_UNIT: MovePoint to WP_AERIAL_P4_POS via the
-- MoveTo twin (the ev28 arrival below fires on completion);
-- GetMotionMaster()->Clear() has no bridge (MoveTo re-issues).
-- PHASE_VOL7RON: stand-DEAD, IsEncounterFinished finish path (early
-- return, no Reset), else CastStop + SELF_REPAIR. The C++ arm ends
-- with events.Reset, so the summon pumps are cancelled on a lethal
-- hit -- except on the VOL7RON finish-path early return, where C++
-- returns before the Reset (the kills fire the components' JustDied
-- arms, which cancel theirs).
local function aerialDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health == 0 or damage < health then
        return
    end
    local guid = creature:GetGUIDLow()
    creature:SetReactState(REACT_PASSIVE)
    creature:AttackStop()
    creature:SetDisableGravity(true)
    creature:CastSpell(creature, SPELL_VEHICLE_DAMAGED, true)
    local st = acuState[guid]
    local phase = st and st.phase
    if phase == "AERIAL_COMMAND_UNIT" then
        creature:MoveTo(WP_AERIAL_P4_POS, ACU_P4_POS[1], ACU_P4_POS[2], ACU_P4_POS[3])
    elseif phase == "VOL7RON" then
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

-- C++ MovementInform (:1220-1230), WP_AERIAL_P4_POS arrival only.
-- Event 28 args: (event, creature, motionType, id); the four_horsemen
-- ev28 form. The DO_ACTIVATE_V0L7R0N_1 receiver is live in
-- boss_mimiron.lua.
local function aerialMovementInform(event, creature, motionType, id)
    if motionType ~= POINT_MOTION_TYPE or id ~= WP_AERIAL_P4_POS then
        return
    end
    creature:SetFacingTo(ACU_P4_ORIENTATION)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:CastSpell(creature, SPELL_CLEAR_ALL_DEBUFFS)
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        mimiron:DoAction(DO_ACTIVATE_V0L7R0N_1)
    end
end

-- C++ DoAction (:1159) whole switch, in C++ order. Action 7 falls
-- through to the action-6 legs exactly as in C++ ([[fallthrough]]).
local function aerialDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == DO_HARDMODE_AERIAL then
        local st = acuState[guid]
        if not st then
            st = { fireFighter = false }
            acuState[guid] = st
        end
        st.fireFighter = true
        creature:CastSpell(creature, SPELL_EMERGENCY_MODE)
        schedule(guid, "firebots", 1000, function()
            summonFireBots(creature, guid)
        end)
    end
    if action == DO_HARDMODE_AERIAL or action == DO_START_AERIAL then
        local st = acuState[guid]
        if not st then
            st = { fireFighter = false, phase = nil }
            acuState[guid] = st
        end
        -- SetPhase(PHASE_AERIAL_COMMAND_UNIT) mirror, read by KilledUnit.
        st.phase = "AERIAL_COMMAND_UNIT"
        creature:SetDisableGravity(false)
        creature:SetHover(true)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        -- SetImmuneToPC(false): NO bridge -- leg skipped, see header.
        creature:SetReactState(REACT_AGGRESSIVE)
        schedule(guid, "junkbot", 5000, function()
            summonJunkBot(creature, guid)
        end)
        schedule(guid, "assaultbot", 9000, function()
            summonAssaultBot(creature, guid)
        end)
        schedule(guid, "bombbot", 9000, function()
            summonBombBot(creature, guid)
        end)
        return
    end
    if action == DO_DISABLE_AERIAL then
        creature:StopSpellCast()
        creature:SetReactState(REACT_PASSIVE)
        creature:AttackStop()
        creature:MoveFall()
        delayTimers(guid, { "firebots", "junkbot", "assaultbot", "bombbot" }, 23000)
        return
    end
    if action == DO_ENABLE_AERIAL then
        creature:SetReactState(REACT_AGGRESSIVE)
        return
    end
    if action == DO_ASSEMBLED_COMBAT then
        local st = acuState[guid]
        if not st then
            st = { fireFighter = false, phase = nil }
            acuState[guid] = st
        end
        -- SetPhase(PHASE_VOL7RON) mirror, read by KilledUnit.
        st.phase = "VOL7RON"
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        creature:SetReactState(REACT_AGGRESSIVE)
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        cancelTimerKey(guid, "firebots")
        cancelTimerKey(guid, "junkbot")
        cancelTimerKey(guid, "assaultbot")
        cancelTimerKey(guid, "bombbot")
        return
    end
end

-- C++ JustSummoned (:1206): firefighter EMERGENCY_MODE on summoned
-- assault/junk bots. (BossAI summon-tracking leg is engine-side.)
local function aerialJustSummoned(event, creature, summoned)
    local st = acuState[creature:GetGUIDLow()]
    if st and st.fireFighter then
        local entry = summoned:GetEntry()
        if entry == NPC_ASSAULT_BOT or entry == NPC_JUNK_BOT then
            summoned:CastSpell(summoned, SPELL_EMERGENCY_MODE)
        end
    end
end

-- C++ Reset / death: cancel all Lua timers (events.Reset twin).
local function aerialReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    local st = acuState[creature:GetGUIDLow()]
    if st then
        st.phase = nil
    end
end

-- C++ boss_aerial_command_unitAI::KilledUnit (:1213-1218): player-gated;
-- the Mimiron controller Talks SAY_AERIAL_SLAY 9 while this AI's EventMap
-- is in PHASE_AERIAL_COMMAND_UNIT, else SAY_V07TRON_SLAY 12 (VOL7RON).
-- Lua phase mirror is acuState[guid].phase (nil before START reads as
-- not-AERIAL, matching C++ IsInPhase false before SetPhase).
-- FindNearestCreature is the twin of the C++ instance->GetCreature(BOSS_MIMIRON) ref.
local function aerialTargetDied(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    local mimiron = creature:FindNearestCreature(NPC_MIMIRON, 500)
    if mimiron ~= nil then
        local st = acuState[creature:GetGUIDLow()]
        local inPhase = st ~= nil and st.phase == "AERIAL_COMMAND_UNIT"
        mimiron:Talk(inPhase and SAY_AERIAL_SLAY or SAY_V07TRON_SLAY)
    end
end

local function aerialDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ spell_mimiron_magnetic_core_AuraScript (boss_mimiron.cpp:1920-1952).
-- OnApply (EFFECT_1, SPELL_AURA_MOD_DAMAGE_PERCENT_TAKEN, REAL):
-- creature target -> AI()->DoAction(DO_DISABLE_AERIAL) + self-cast
-- MAGNETIC_CORE_VISUAL 64438 triggered. PORTED tick 561 via
-- RegisterAuraEvent ev2 (runtime.go:422-500; the illidan
-- akamaDoorChannelRemove precedent: target:DoAction on the aura
-- target). The hook fires on the player and creature funnels, so the
-- C++ ToCreature() gate is the GetObjectType() check below.
local function magneticCoreApply(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    target:DoAction(DO_DISABLE_AERIAL)
    target:CastSpell(target, SPELL_MAGNETIC_CORE_VISUAL, true)
end

-- C++ OnRemove (EFFECT_1, same effect): DoAction(DO_ENABLE_AERIAL) +
-- RemoveAurasDueToSpell(VISUAL) -- the bridged RemoveAura(spellID)
-- twin. The OnRemoveSelf EFFECT_0 DUMMY leg (TempSummon
-- DespawnOrUnsummon) is PORTED this tick: IsSummon() is bridged
-- (lua_creature_events.go:922, motion.IsTempSummon — the ToTempSummon
-- gate) and Despawn() on a summon is the DespawnOrUnsummon twin
-- (lua_creature_events.go:3061). The Go ev3 hook fires once per
-- removal (not per effect), so both C++ AfterEffectRemove arms share
-- this one fire; for the ACU (not a summon) the despawn leg no-ops,
-- matching C++.
local function magneticCoreRemove(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    target:DoAction(DO_ENABLE_AERIAL)
    target:RemoveAura(SPELL_MAGNETIC_CORE_VISUAL)
    if target:IsSummon() then
        target:Despawn()
    end
end

-- C++ spell_mimiron_magnetic_core_SpellScript::FilterTargets
-- (:1901-1913, EFFECT_1): drop targets with a vehicle base or the
-- NON_ATTACKABLE flag. PORTED tick 561 via RegisterSpellEvent ev7
-- (the skadi freezing-cloud precedent: targets:GetTargets() /
-- targets:Remove). The vehicle-base half is PORTED this tick:
-- GetVehicleBase() is non-null exactly when the unit rides a vehicle,
-- and IsOnVehicle() is that twin on both unit types (creature,
-- lua_creature_events.go:3252; player, lua_player.go:2801 — the weld
-- precedent uses target:IsOnVehicle():GetOwner() for the same
-- GetVehicleBase lookup); a vehicle base itself rides nothing, so its
-- IsOnVehicle() is nil, matching C++.
local function magneticCoreTargetSelect(event, spell, effectIndex, targets)
    if effectIndex ~= 1 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) or target:IsOnVehicle() ~= nil then
            targets:Remove(target)
        end
    end
end

RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 5, aerialSpawn)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 9, aerialDamageTaken)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 28, aerialMovementInform)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 19, aerialJustSummoned)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 23, aerialReset)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 3, aerialTargetDied)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 4, aerialDied)
RegisterCreatureEvent(NPC_AERIAL_COMMAND_UNIT, 38, aerialDoAction)

RegisterAuraEvent(SPELL_MAGNETIC_CORE, 2, magneticCoreApply)
RegisterAuraEvent(SPELL_MAGNETIC_CORE, 3, magneticCoreRemove)
RegisterSpellEvent(SPELL_MAGNETIC_CORE, 7, magneticCoreTargetSelect)
