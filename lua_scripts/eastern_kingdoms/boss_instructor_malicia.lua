-- Instructor Malicia (10505), Scholomance — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/
-- boss_instructor_malicia.cpp (creature AI only; script name
-- "boss_instructor_malicia", loader AddSC_boss_instructormalicia
-- decl :113 / call :292). Entry has no NPC_ constant in
-- scholomance.h (DB-side ScriptName binding; classic
-- wowhead-verified npc=10505). Zero Talk lines in C++ (no _SAY
-- enum). Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- OnEnterCombat: call of the grave 17831 at 4s / corruption 11672
-- at 8s / renew 10929 at 32s / flash heal 10917 at 38s / healing
-- touch 9889 at 45s. Call of the grave is triggered DoCastVictim
-- -> GetVictim + CastSpell(victim, spell, true), nil-victim keeps
-- schedule (moroes/maiden/gandling triggered convention), re-arm
-- 65s. Corruption is triggered DoCast on
-- SelectTarget(Random, 0, 100, true) -> random alive player within
-- 100yd (offset 0 includes the victim; janalai/kelris/gandling
-- convention), nil pick casts nothing, re-arm 24s. Renew/flash
-- heal/healing touch are non-triggered self-casts. Flash heal and
-- healing touch chain: after each cast, if the per-GUID counter
-- is < 2, re-arm 5s / 5.5s and increment; otherwise reset the
-- counter and re-arm 30s (C++ "5 Flashheals will be cast" /
-- "3 Healing Touch will be cast" comments; counters cleared by
-- Initialize() on Reset).
--
-- Documented-unmodeled (no bridges): UpdateAI UNIT_STATE_CASTING
-- queue + post-event gates + BossAI::JustEngagedWith +
-- DATA_INSTRUCTORMALICIA=1 bookkeeping (scholomance.h:31)
-- (luaBossAI shim).

local ENTRY_MALICIA = 10505

local SPELL_CALL_OF_GRAVES = 17831
local SPELL_CORRUPTION = 11672
local SPELL_FLASH_HEAL = 10917
local SPELL_RENEW = 10929
local SPELL_HEALING_TOUCH = 9889

local timers = {}
local counters = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    counters[guid] = nil
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

-- C++ SelectTarget(Random, 0, 100, true): random alive player within
-- maxDist yards, victim included (janalai/kelris convention).
local function randomPlayerInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ triggered DoCastVictim -> nil-victim keeps schedule (moroes/
-- maiden triggered convention).
local function castOnVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell, true)
    end
end

-- C++ EVENT_CALLOFGRAVES: triggered DoCastVictim(17831); Repeat(65s).
local function onCallOfGraves(creature, guid)
    castOnVictim(creature, SPELL_CALL_OF_GRAVES)
    schedule(guid, "callOfGraves", 65000, function()
        onCallOfGraves(creature, guid)
    end)
end

-- C++ EVENT_CORRUPTION: triggered DoCast(random alive player within
-- 100yd, 11672); Repeat(24s). Nil pick casts nothing.
local function onCorruption(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_CORRUPTION, true)
    end
    schedule(guid, "corruption", 24000, function()
        onCorruption(creature, guid)
    end)
end

-- C++ EVENT_RENEW: non-triggered self-cast 10929; Repeat(10s).
local function onRenew(creature, guid)
    creature:CastSpell(creature, SPELL_RENEW, false)
    schedule(guid, "renew", 10000, function()
        onRenew(creature, guid)
    end)
end

-- C++ EVENT_FLASHHEAL: non-triggered self-cast 10917. Re-arm 5s and
-- increment while FlashCounter < 2; otherwise reset and re-arm 30s.
local function onFlashHeal(creature, guid)
    creature:CastSpell(creature, SPELL_FLASH_HEAL, false)
    local st = counters[guid] or { flash = 0, touch = 0 }
    counters[guid] = st
    if st.flash < 2 then
        st.flash = st.flash + 1
        schedule(guid, "flashHeal", 5000, function()
            onFlashHeal(creature, guid)
        end)
    else
        st.flash = 0
        schedule(guid, "flashHeal", 30000, function()
            onFlashHeal(creature, guid)
        end)
    end
end

-- C++ EVENT_HEALINGTOUCH: non-triggered self-cast 9889. Re-arm
-- 5500ms and increment while TouchCounter < 2; otherwise reset and
-- re-arm 30s.
local function onHealingTouch(creature, guid)
    creature:CastSpell(creature, SPELL_HEALING_TOUCH, false)
    local st = counters[guid] or { flash = 0, touch = 0 }
    counters[guid] = st
    if st.touch < 2 then
        st.touch = st.touch + 1
        schedule(guid, "healingTouch", 5500, function()
            onHealingTouch(creature, guid)
        end)
    else
        st.touch = 0
        schedule(guid, "healingTouch", 30000, function()
            onHealingTouch(creature, guid)
        end)
    end
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (shim) + the five events.
local function maliciaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "callOfGraves", 4000, function()
        onCallOfGraves(creature, guid)
    end)
    schedule(guid, "corruption", 8000, function()
        onCorruption(creature, guid)
    end)
    schedule(guid, "renew", 32000, function()
        onRenew(creature, guid)
    end)
    schedule(guid, "flashHeal", 38000, function()
        onFlashHeal(creature, guid)
    end)
    schedule(guid, "healingTouch", 45000, function()
        onHealingTouch(creature, guid)
    end)
end

local function maliciaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function maliciaDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _Reset + Initialize() (counters to 0).
local function maliciaReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MALICIA, 1, maliciaEnterCombat)
RegisterCreatureEvent(ENTRY_MALICIA, 2, maliciaLeaveCombat)
RegisterCreatureEvent(ENTRY_MALICIA, 4, maliciaDied)
RegisterCreatureEvent(ENTRY_MALICIA, 23, maliciaReset)
