-- Broodlord Lashlayer (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_broodlord_lashlayer.cpp (boss_broodlordAI); blackwing_lair.h:33
-- (DATA_BROODLORD_LASHLAYER = 2, third boss), :55 (NPC_BROODLORD =
-- 12017). Creature entry: 12017 Broodlord Lashlayer (C++ ScriptName
-- "boss_broodlord" per AddSC_boss_broodlord). The go_suppression_device
-- GameObjectScript from the same C++ file is not registered: the
-- RegisterGameObjectEvent bridge and the GO GetGoState/SetGoState surface
-- exist (lua_objects.go:1080/1095), but the device's GameObjectAI
-- OnAIUpdate/OnLootState legs have no engine fire sites (creature event
-- 7 fires, but GameObjectEventOnAIUpdate = 1 and OnLootState = 9 are
-- const-only, zero Trigger sites — re-verified 2026-10-24); the device's
-- own suppression-cast machine is unmodeled, but the JustDied
-- deactivation itself is PORTED below on the GetGameObjectsInGrid bridge
-- (2026-10-26 stale-bridge re-triage; the 2026-10-23 "unbridged grid
-- enumeration" claim is now stale).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO=0), arm cleave 26350 8s then every 7s / blast wave
-- 23331 12s then {8s,16s} / mortal strike 24573 20s then {25s,35s} /
-- knockback 25778 30s then {15s,30s}, all non-triggered DoCastVictim
-- (C++ DoCast default is triggered=false — vaelastrasz convention),
-- plus the 1s boundary check. Nil-victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnDied: Talk is not used by C++ — JustDied only runs
-- the suppression-device deactivation (PORTED below). OnLeave
-- Combat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate has no
-- UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the
-- luaBossAI shim (_Reset/BossAI::JustEngagedWith and the
-- DATA_BROODLORD_LASHLAYER bookkeeping arms skipped); the
-- knockback-arm ModifyThreatByPercent(victim, -50) PORTED 2026-10-24
-- (threat bridge live) — the cast still lands and the 15-30s re-arm is kept;
-- the EVENT_CHECK home-distance > 150 yd leash check: gate +
-- Talk(SAY_LEASH=1) PORTED 2026-10-24 (GetHomePosition + GetX/Y/Z math
-- minus GetCombatReach twins GetDistance(Position); Talk once-latched
-- per engagement) — the EnterEvadeMode(EVADE_REASON_BOUNDARY) leg stays
-- out (no evade bridge), so the 1s re-arm cycle keeps running after
-- the yell; the JustDied GetGameObjectListWithEntryInGrid
-- suppression-device scan + DoAction(ACTION_DEACTIVATE) arm PORTED
-- 2026-10-26 (GetGameObjectsInGrid bridge, lua_creature_events.go:
-- 2989; Deactivate() = SetGoState(GO_STATE_ACTIVE) + SetFlag
-- GAMEOBJECT_FLAGS GO_FLAG_NOT_SELECTABLE — the _active latch and
-- the CancelEvent legs are moot in-model since the device's own
-- suppression-cast machine is unported, and the SetGoState/flag
-- writes are idempotent, so repeat deactivation is harmless); the
-- device's own cast/OnLootState machine stays unmodeled;
-- the whole go_suppression_device machine (0-5s first suppression
-- aura 22247 cast, 5s cycle, disarm-deactivate, 30-120s re-activate,
-- DoAction deactivation) is not modeled (standing gameobject gap).

local ENTRY_BROODLORD = 12017

local SAY_AGGRO = 0
local SAY_LEASH = 1 -- Talk ported once-latched in onCheck; EnterEvadeMode leg stays out.

local SPELL_CLEAVE = 26350
local SPELL_BLASTWAVE = 23331
local SPELL_MORTALSTRIKE = 24573
local SPELL_KNOCKBACK = 25778
local GO_SUPPRESSION_DEVICE = 179784 -- blackwing_lair.h:75
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent: OBJECT_END 6 + 3
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_STATE_ACTIVE = 0 -- netherspite precedent: READY 1, ACTIVE 0
-- SPELL_SUPPRESSION_AURA = 22247: unmodeled — go_suppression_device
-- has no gameobject bridge.

local timers = {}
local leashTalked = {}

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

-- C++ EVENT_CLEAVE: non-triggered DoCastVictim(26350); re-arm 7s.
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 7000, function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_BLASTWAVE: non-triggered DoCastVictim(23331); re-arm {8s,16s}.
local function onBlastwave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLASTWAVE)
    end
    schedule(guid, "blastwave", math.random(8000, 16000), function()
        onBlastwave(creature, guid)
    end)
end

-- C++ EVENT_MORTALSTRIKE: non-triggered DoCastVictim(24573);
-- re-arm {25s,35s}.
local function onMortalStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTALSTRIKE)
    end
    schedule(guid, "mortalstrike", math.random(25000, 35000), function()
        onMortalStrike(creature, guid)
    end)
end

-- C++ EVENT_KNOCKBACK: non-triggered DoCastVictim(25778) + -50% threat on
-- the victim (threat arm PORTED 2026-10-24); re-arm {15s,30s}.
local function onKnockback(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKBACK)
        -- C++ arm order: cast, then GetThreat gate + -50%.
        if creature:GetThreat(victim) ~= 0 then
            creature:ModifyThreatByPercent(victim, -50)
        end
    end
    schedule(guid, "knockback", math.random(15000, 30000), function()
        onKnockback(creature, guid)
    end)
end

-- C++ EVENT_CHECK (boss_broodlord_lashlayer.cpp:117-123): GetDistance(home)
-- > 150 yd -> Talk(SAY_LEASH) + EnterEvadeMode; 1s re-arm unconditional.
-- Distance twin: GetHomePosition + GetX/Y/Z math, minus GetCombatReach
-- (WorldObject::GetDistance(Position), Object.cpp:1175-1179), clamped
-- >= 0. Talk is once-latched per engagement (C++ evades at once, so the
-- yell fires once; without an evade bridge the latch stops re-yells);
-- the EnterEvadeMode leg stays out (no bridge).
local function onCheck(creature, guid)
    if not leashTalked[guid] then
        local hx, hy, hz = creature:GetHomePosition()
        local dx = creature:GetX() - hx
        local dy = creature:GetY() - hy
        local dz = creature:GetZ() - hz
        local dist = math.sqrt(dx * dx + dy * dy + dz * dz) - creature:GetCombatReach()
        if dist < 0 then dist = 0 end
        if dist > 150.0 then
            creature:Talk(SAY_LEASH)
            leashTalked[guid] = true
        end
    end
    schedule(guid, "check", 1000, function()
        onCheck(creature, guid)
    end)
end

local function broodlordEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "cleave", 8000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "blastwave", 12000, function()
        onBlastwave(creature, guid)
    end)
    schedule(guid, "mortalstrike", 20000, function()
        onMortalStrike(creature, guid)
    end)
    schedule(guid, "knockback", 30000, function()
        onKnockback(creature, guid)
    end)
    schedule(guid, "check", 1000, function()
        onCheck(creature, guid)
    end)
end

local function broodlordResetState(guid)
    cancelTimers(guid)
    leashTalked[guid] = nil
end

local function broodlordLeaveCombat(event, creature)
    broodlordResetState(creature:GetGUID())
end

local function broodlordDied(event, creature, killer)
    -- C++ JustDied (boss_broodlord_lashlayer.cpp:78-86): scan
    -- GO_SUPPRESSION_DEVICE in 200yd + DoAction(ACTION_DEACTIVATE)
    -- per device; Deactivate() = SetGoState(ACTIVE) + SetFlag
    -- NOT_SELECTABLE (the _active latch + CancelEvent legs are moot:
    -- the device cast machine itself is unported, and both writes
    -- are idempotent). The _JustDied() instance bookkeeping stays
    -- out (no instance-script model).
    for _, device in ipairs(creature:GetGameObjectsInGrid(GO_SUPPRESSION_DEVICE, 200)) do
        device:SetGoState(GO_STATE_ACTIVE)
        device:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    broodlordResetState(creature:GetGUID())
end

local function broodlordReset(event, creature)
    broodlordResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BROODLORD, 1, broodlordEnterCombat)
RegisterCreatureEvent(ENTRY_BROODLORD, 2, broodlordLeaveCombat)
RegisterCreatureEvent(ENTRY_BROODLORD, 4, broodlordDied)
RegisterCreatureEvent(ENTRY_BROODLORD, 23, broodlordReset)
