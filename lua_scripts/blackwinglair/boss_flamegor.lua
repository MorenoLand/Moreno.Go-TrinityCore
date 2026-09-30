-- Flamegor (Blackwing Lair) — Lua port of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackwingLair/
-- boss_flamegor.cpp (boss_flamegorAI only); blackwing_lair.h:36
-- (DATA_FLAMEGOR = 5, sixth boss), :58 (NPC_FLAMEGOR = 11981).
-- Creature entry: 11981 Flamegor (C++ ScriptName "boss_flamegor" per
-- AddSC_boss_flamegor). One Talk line: EMOTE_FRENZY = 0, spoken on the
-- frenzy arm.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: arm
-- shadowflame 22539 {10s,20s} then {10s,20s} / wingbuffet 23339 30s
-- then 30s / frenzy 23342 first 10s then {8s,10s} — Talk(EMOTE_FRENZY)
-- + non-triggered self-cast 23342 (C++ DoCast default is
-- triggered=false — vaelastrasz convention, not the broodlord one).
-- The 23342 spell itself periodically triggers fire nova. Nil-victim
-- ticks cast nothing but keep the schedule (jeklik convention).
-- OnDied/OnLeaveCombat(2)/OnReset(23): cancel timers.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); no instance-script model —
-- boss admission via the luaBossAI shim (BossAI::JustEngagedWith and
-- the DATA_FLAMEGOR bookkeeping arms skipped); the wingbuffet-arm
-- GetThreat + ModifyThreatByPercent(victim, -75) has no bridge (no
-- threat model) — the cast still lands and the 30s re-arm is kept.

local ENTRY_FLAMEGOR = 11981

local SPELL_SHADOWFLAME = 22539
local SPELL_WINGBUFFET = 23339
local SPELL_FRENZY = 23342

local EMOTE_FRENZY = 0

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

-- C++ EVENT_FRENZY: Talk(EMOTE_FRENZY=0), non-triggered self-cast
-- 23342; re-arm {8s,10s}.
local function onFrenzy(creature, guid)
    creature:Talk(EMOTE_FRENZY)
    creature:CastSpell(creature, SPELL_FRENZY)
    schedule(guid, "frenzy", math.random(8000, 10000), function()
        onFrenzy(creature, guid)
    end)
end

local function flamegorEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowflame", math.random(10000, 20000), function()
        onShadowflame(creature, guid)
    end)
    schedule(guid, "wingbuffet", 30000, function()
        onWingBuffet(creature, guid)
    end)
    schedule(guid, "frenzy", 10000, function()
        onFrenzy(creature, guid)
    end)
end

local function flamegorResetState(guid)
    cancelTimers(guid)
end

local function flamegorLeaveCombat(event, creature)
    flamegorResetState(creature:GetGUID())
end

local function flamegorDied(event, creature, killer)
    flamegorResetState(creature:GetGUID())
end

local function flamegorReset(event, creature)
    flamegorResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FLAMEGOR, 1, flamegorEnterCombat)
RegisterCreatureEvent(ENTRY_FLAMEGOR, 2, flamegorLeaveCombat)
RegisterCreatureEvent(ENTRY_FLAMEGOR, 4, flamegorDied)
RegisterCreatureEvent(ENTRY_FLAMEGOR, 23, flamegorReset)
