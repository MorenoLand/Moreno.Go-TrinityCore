-- Gri'lek of the Iron Blade (Zul'Gurub, Edge of Madness) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_grilek.cpp
-- (boss_grilekAI); zulgurub.h:39 (DATA_EDGE_OF_MADNESS = 9, optional
-- event). Creature entry: 15082 Gri'lek (C++ ScriptName "boss_grilek"
-- per AddSC_boss_grilek; classic.wowhead.com/npc=15082; zulgurub.h
-- defines no NPC_GRILEK constant). Gri'lek is summoned by the brazier
-- event (no summon model — he simply starts combat already spawned).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: enter combat arms the two timers — avatar 24646 at
-- {15s,25s} then {25s,35s} (non-triggered self-cast, C++-exact
-- DoCast(me)) / ground tremor 6524 at {15s,25s} then {12s,16s} on the
-- victim (triggered DoCastVictim, C++-exact). The re-arms are
-- unconditional (nil-victim ticks cast nothing but keep the schedule,
-- jeklik convention). Reset cancels timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate (skip the
-- rest of the event queue while casting) has no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-script
-- model — boss admission via the luaBossAI shim (_Reset/_JustDied/
-- BossAI::JustEngagedWith and the GetZulGurubAI bookkeeping arms
-- skipped); the avatar ModifyThreatByPercent(-50) victim arm and the
-- AttackStart-on-random-player arm have no bridge (no threat model, no
-- movement model) — the avatar cast still lands C++-exact, but the
-- boss keeps attacking its current victim; no Talk lines exist in the
-- C++ AI.

local ENTRY_GRILEK = 15082

local SPELL_AVATAR = 24646
local SPELL_GROUND_TREMOR = 6524

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

-- C++ EVENT_AVATAR: non-triggered DoCast(me, 24646); re-arm {25s,35s}.
local function onAvatar(creature, guid)
    creature:CastSpell(creature, SPELL_AVATAR)
    schedule(guid, "avatar", math.random(25000, 35000), function()
        onAvatar(creature, guid)
    end)
end

-- C++ EVENT_GROUND_TREMOR: triggered DoCastVictim(6524); re-arm
-- {12s,16s}.
local function onGroundTremor(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GROUND_TREMOR)
    end
    schedule(guid, "tremor", math.random(12000, 16000), function()
        onGroundTremor(creature, guid)
    end)
end

local function grilekResetState(guid)
    cancelTimers(guid)
end

local function grilekEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "avatar", math.random(15000, 25000), function()
        onAvatar(creature, guid)
    end)
    schedule(guid, "tremor", math.random(15000, 25000), function()
        onGroundTremor(creature, guid)
    end)
end

local function grilekLeaveCombat(event, creature)
    grilekResetState(creature:GetGUID())
end

local function grilekDied(event, creature, killer)
    grilekResetState(creature:GetGUID())
end

local function grilekReset(event, creature)
    -- C++ Reset's _Reset bookkeeping has no bridge (no instance-script
    -- model) — only the timer cancel is modeled.
    grilekResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GRILEK, 1, grilekEnterCombat)
RegisterCreatureEvent(ENTRY_GRILEK, 2, grilekLeaveCombat)
RegisterCreatureEvent(ENTRY_GRILEK, 4, grilekDied)
RegisterCreatureEvent(ENTRY_GRILEK, 23, grilekReset)
