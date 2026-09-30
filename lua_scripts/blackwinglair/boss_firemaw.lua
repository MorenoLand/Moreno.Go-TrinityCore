-- Firemaw (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_firemaw.cpp (boss_firemawAI only); blackwing_lair.h:34
-- (DATA_FIREMAW = 3, fourth boss), :56 (NPC_FIREMAW = 11983).
-- Creature entry: 11983 Firemaw (C++ ScriptName "boss_firemaw" per
-- AddSC_boss_firemaw). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: arm
-- shadowflame 22539 {10s,20s} then {10s,20s} / wingbuffet 23339 30s
-- then 30s / flamebuffet 23341 5s then 5s, all non-triggered
-- DoCastVictim. Nil-victim ticks cast nothing but keep the schedule
-- (jeklik convention). OnDied/OnLeaveCombat(2)/OnReset(23): cancel
-- timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the DATA_FIREMAW bookkeeping arms skipped); the wingbuffet-arm
-- GetThreat + ModifyThreatByPercent(victim, -75) has no bridge (no
-- threat model) — the cast still lands and the 30s re-arm is kept.

local ENTRY_FIREMAW = 11983

local SPELL_SHADOWFLAME = 22539
local SPELL_WINGBUFFET = 23339
local SPELL_FLAMEBUFFET = 23341

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

-- C++ EVENT_SHADOWFLAME: non-triggered DoCastVictim(22539);
-- re-arm {10s,20s}.
local function onShadowflame(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWFLAME)
    end
    schedule(guid, "shadowflame", math.random(10000, 20000), function()
        onShadowflame(creature, guid)
    end)
end

-- C++ EVENT_WINGBUFFET: non-triggered DoCastVictim(23339) + -75%
-- threat on the victim (threat arm has no bridge); re-arm 30s.
local function onWingBuffet(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WINGBUFFET)
    end
    schedule(guid, "wingbuffet", 30000, function()
        onWingBuffet(creature, guid)
    end)
end

-- C++ EVENT_FLAMEBUFFET: non-triggered DoCastVictim(23341); re-arm 5s.
local function onFlameBuffet(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAMEBUFFET)
    end
    schedule(guid, "flamebuffet", 5000, function()
        onFlameBuffet(creature, guid)
    end)
end

local function firemawEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowflame", math.random(10000, 20000), function()
        onShadowflame(creature, guid)
    end)
    schedule(guid, "wingbuffet", 30000, function()
        onWingBuffet(creature, guid)
    end)
    schedule(guid, "flamebuffet", 5000, function()
        onFlameBuffet(creature, guid)
    end)
end

local function firemawResetState(guid)
    cancelTimers(guid)
end

local function firemawLeaveCombat(event, creature)
    firemawResetState(creature:GetGUID())
end

local function firemawDied(event, creature, killer)
    firemawResetState(creature:GetGUID())
end

local function firemawReset(event, creature)
    firemawResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FIREMAW, 1, firemawEnterCombat)
RegisterCreatureEvent(ENTRY_FIREMAW, 2, firemawLeaveCombat)
RegisterCreatureEvent(ENTRY_FIREMAW, 4, firemawDied)
RegisterCreatureEvent(ENTRY_FIREMAW, 23, firemawReset)
