-- Ebonroc (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_ebonroc.cpp (boss_ebonrocAI only); blackwing_lair.h:35
-- (DATA_EBONROC = 4, fifth boss), :57 (NPC_EBONROC = 14601).
-- Creature entry: 14601 Ebonroc (C++ ScriptName "boss_ebonroc" per
-- AddSC_boss_ebonroc). No Talk lines in the C++ AI.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: arm
-- shadowflame 22539 {10s,20s} then {10s,20s} / wingbuffet 23339 30s
-- then 30s / shadowofebonroc 23340 {8s,10s} then {8s,10s}, all
-- non-triggered DoCastVictim (C++ DoCastVictim default is
-- triggered=false — vaelastrasz convention, not the broodlord one).
-- Nil-victim ticks cast nothing but keep the schedule (jeklik
-- convention). OnDied/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the DATA_EBONROC bookkeeping arms skipped); the wingbuffet-arm
-- GetThreat + ModifyThreatByPercent(victim, -75) has no bridge (no
-- threat model) — the cast still lands and the 30s re-arm is kept.

local ENTRY_EBONROC = 14601

local SPELL_SHADOWFLAME = 22539
local SPELL_WINGBUFFET = 23339
local SPELL_SHADOWOFEBONROC = 23340

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

-- C++ EVENT_SHADOWOFEBONROC: non-triggered DoCastVictim(23340);
-- re-arm {8s,10s}.
local function onShadowOfEbonroc(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWOFEBONROC)
    end
    schedule(guid, "shadowofebonroc", math.random(8000, 10000), function()
        onShadowOfEbonroc(creature, guid)
    end)
end

local function ebonrocEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowflame", math.random(10000, 20000), function()
        onShadowflame(creature, guid)
    end)
    schedule(guid, "wingbuffet", 30000, function()
        onWingBuffet(creature, guid)
    end)
    schedule(guid, "shadowofebonroc", math.random(8000, 10000), function()
        onShadowOfEbonroc(creature, guid)
    end)
end

local function ebonrocResetState(guid)
    cancelTimers(guid)
end

local function ebonrocLeaveCombat(event, creature)
    ebonrocResetState(creature:GetGUID())
end

local function ebonrocDied(event, creature, killer)
    ebonrocResetState(creature:GetGUID())
end

local function ebonrocReset(event, creature)
    ebonrocResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_EBONROC, 1, ebonrocEnterCombat)
RegisterCreatureEvent(ENTRY_EBONROC, 2, ebonrocLeaveCombat)
RegisterCreatureEvent(ENTRY_EBONROC, 4, ebonrocDied)
RegisterCreatureEvent(ENTRY_EBONROC, 23, ebonrocReset)
