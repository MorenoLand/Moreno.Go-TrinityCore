-- Hazza'rah (Zul'Gurub, Edge of Madness) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_hazzarah.cpp
-- (boss_hazzarahAI); zulgurub.h:39 (DATA_EDGE_OF_MADNESS = 9, optional
-- event), :57 (NPC_NIGHTMARE_ILLUSION = 15163). Creature entry: 15083
-- Hazza'rah (C++ ScriptName "boss_hazzarah" per AddSC_boss_hazzarah;
-- classic.wowhead.com/npc=15083/hazzarah). Hazza'rah is summoned by the
-- Edge of Madness brazier event (no summon model — she simply starts
-- combat already spawned).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: enter combat arms the three timers — mana burn 26046 at
-- {4s,10s} then {8s,16s} on the victim (triggered DoCastVictim,
-- C++-exact) / sleep 24664 at {10s,18s} then {12s,20s} on the victim
-- (triggered DoCastVictim, C++-exact) / illusions at {10s,18s} then
-- {15s,25s} (re-arm only; no summon model — the three nightmare
-- illusions 15163 never exist, jeklik six-bat convention). The re-arms
-- are unconditional (nil-victim ticks cast nothing but keep the
-- schedule, jeklik convention). Reset cancels timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate (skip the
-- rest of the event queue while casting) has no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-script
-- model — boss admission via the luaBossAI shim (_Reset/_JustDied/
-- BossAI::JustEngagedWith and the GetZulGurubAI bookkeeping arms
-- skipped); the illusion summon-and-AttackStart arm has no bearer (no
-- summon model, no movement model) — only the 15-25s re-arm is modeled;
-- no Talk lines exist in the C++ AI.

local ENTRY_HAZZARAH = 15083

local SPELL_MANABURN = 26046
local SPELL_SLEEP = 24664

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

-- C++ EVENT_MANABURN: triggered DoCastVictim(26046); re-arm {8s,16s}.
local function onManaBurn(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MANABURN)
    end
    schedule(guid, "manaburn", math.random(8000, 16000), function()
        onManaBurn(creature, guid)
    end)
end

-- C++ EVENT_SLEEP: triggered DoCastVictim(24664); re-arm {12s,20s}.
local function onSleep(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SLEEP)
    end
    schedule(guid, "sleep", math.random(12000, 20000), function()
        onSleep(creature, guid)
    end)
end

-- C++ EVENT_ILLUSIONS: summon 3 NPC_NIGHTMARE_ILLUSION (15163) on
-- random players with AttackStart. No summon model — only the {15s,25s}
-- re-arm is modeled (jeklik six-bat convention).
local function onIllusions(creature, guid)
    schedule(guid, "illusions", math.random(15000, 25000), function()
        onIllusions(creature, guid)
    end)
end

local function hazzarahResetState(guid)
    cancelTimers(guid)
end

local function hazzarahEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "manaburn", math.random(4000, 10000), function()
        onManaBurn(creature, guid)
    end)
    schedule(guid, "sleep", math.random(10000, 18000), function()
        onSleep(creature, guid)
    end)
    schedule(guid, "illusions", math.random(10000, 18000), function()
        onIllusions(creature, guid)
    end)
end

local function hazzarahLeaveCombat(event, creature)
    hazzarahResetState(creature:GetGUID())
end

local function hazzarahDied(event, creature, killer)
    hazzarahResetState(creature:GetGUID())
end

local function hazzarahReset(event, creature)
    -- C++ Reset's _Reset bookkeeping has no bridge (no instance-script
    -- model) — only the timer cancel is modeled.
    hazzarahResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_HAZZARAH, 1, hazzarahEnterCombat)
RegisterCreatureEvent(ENTRY_HAZZARAH, 2, hazzarahLeaveCombat)
RegisterCreatureEvent(ENTRY_HAZZARAH, 4, hazzarahDied)
RegisterCreatureEvent(ENTRY_HAZZARAH, 23, hazzarahReset)
