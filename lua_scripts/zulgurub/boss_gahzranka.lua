-- Gahz'ranka (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_gahzranka.cpp
-- (boss_gahzrankaAI); zulgurub.h:38 (DATA_GAHZRANKA = 8, optional boss).
-- Creature entry: 15114 Gahz'ranka (C++ ScriptName "boss_gahzranka" per
-- AddSC_boss_gahzranka; wowhead npc=15114/gahzranka; zulgurub.h defines
-- no NPC_GAHZRANKA constant). Gahz'ranka is fished up with the Mudskunk
-- Lure (no summon model — he simply starts combat already spawned).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: enter combat arms the three timers — frost breath 16099
-- at 8s then {7s,11s} on the victim / massive geyser 22421 at 25s then
-- {22s,32s} on the victim / slam 24326 at 15s then {12s,20s} on the
-- victim. All three casts are C++-exact triggered DoCastVictim casts;
-- the re-arms are unconditional (nil-victim ticks cast nothing but
-- keep the schedule, jeklik convention). Reset cancels timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING gate (skip the
-- rest of the event queue while casting) has no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention); no instance-script
-- model — boss admission via the luaBossAI shim (_Reset/_JustDied/
-- BossAI::JustEngagedWith and the GetZulGurubAI bookkeeping arms
-- skipped); massive geyser's summon arm is upstream-broken ("Not
-- working. (summon)" in the C++ source) — the cast is still modeled
-- C++-exact since the AI fires it regardless.

local ENTRY_GAHZRANKA = 15114

local SPELL_FROSTBREATH = 16099
local SPELL_MASSIVEGEYSER = 22421
local SPELL_SLAM = 24326

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

-- C++ EVENT_FROSTBREATH: triggered DoCastVictim(16099); re-arm {7s,11s}.
local function onFrostBreath(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FROSTBREATH, true)
    end
    schedule(guid, "frostbreath", math.random(7000, 11000), function()
        onFrostBreath(creature, guid)
    end)
end

-- C++ EVENT_MASSIVEGEYSER: triggered DoCastVictim(22421); re-arm
-- {22s,32s}. Cast kept C++-exact; the summon effect is upstream-broken.
local function onMassiveGeyser(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MASSIVEGEYSER, true)
    end
    schedule(guid, "geyser", math.random(22000, 32000), function()
        onMassiveGeyser(creature, guid)
    end)
end

-- C++ EVENT_SLAM: triggered DoCastVictim(24326); re-arm {12s,20s}.
local function onSlam(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SLAM, true)
    end
    schedule(guid, "slam", math.random(12000, 20000), function()
        onSlam(creature, guid)
    end)
end

local function gahzrankaResetState(guid)
    cancelTimers(guid)
end

local function gahzrankaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "frostbreath", 8000, function()
        onFrostBreath(creature, guid)
    end)
    schedule(guid, "geyser", 25000, function()
        onMassiveGeyser(creature, guid)
    end)
    schedule(guid, "slam", 15000, function()
        onSlam(creature, guid)
    end)
end

local function gahzrankaLeaveCombat(event, creature)
    gahzrankaResetState(creature:GetGUID())
end

local function gahzrankaDied(event, creature, killer)
    gahzrankaResetState(creature:GetGUID())
end

local function gahzrankaReset(event, creature)
    -- C++ Reset's _Reset bookkeeping has no bridge (no instance-script
    -- model) — only the timer cancel is modeled.
    gahzrankaResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GAHZRANKA, 1, gahzrankaEnterCombat)
RegisterCreatureEvent(ENTRY_GAHZRANKA, 2, gahzrankaLeaveCombat)
RegisterCreatureEvent(ENTRY_GAHZRANKA, 4, gahzrankaDied)
RegisterCreatureEvent(ENTRY_GAHZRANKA, 23, gahzrankaReset)
