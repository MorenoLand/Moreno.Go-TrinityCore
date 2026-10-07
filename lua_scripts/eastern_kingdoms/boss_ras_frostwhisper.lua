-- Ras Frostwhisper (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_ras_frostwhisper.cpp
-- (boss_rasfrostAI only; CreatureScript name "boss_boss_ras_frostwhisper"
-- — C++ double-"boss" naming quirk, preserved verbatim; EK loader
-- AddSC_boss_rasfrost decl :118 / call :296). No NPC_ constant in
-- scholomance.h — RegisterLuaBoss binds the creature_template
-- ScriptName "boss_boss_ras_frostwhisper" DB-side; entry
-- wowhead-verified: 10508. Eluna creature events: 1 OnEnterCombat, 2
-- OnLeaveCombat, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnReset:
-- non-triggered self-cast ice armor 18100. OnEnterCombat: ice armor
-- 2s->3min (self) / frostbolt 21369 8s->8s (random alive player
-- within 40yd incl. victim, jeklik convention — nil pick casts
-- nothing but keeps the schedule) / chill nova 18099 12s->14s
-- (non-triggered DoCastVictim) / freeze 18763 18s->24s
-- (non-triggered DoCastVictim) / fear 26070 45s->30s
-- (non-triggered DoCastVictim). Non-triggered DoCastVictim ->
-- GetVictim + CastSpell(victim, spell); nil-victim ticks cast
-- nothing but keep the schedule. EVENT_FROSTVOLLEY (8398) has a
-- switch case but is never scheduled in C++ — dead code, not
-- modeled. Zero Talk lines in C++ (no _SAY enum).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- and the post-event casting check have no UNIT_STATE bridge —
-- timers fire unconditionally (jeklik convention).
-- Documented-only (no bridges): GetScholomanceAI template instance
-- validation (luaBossAI shim); no SetData bookkeeping in C++.

local ENTRY_RAS_FROSTWHISPER = 10508

local SPELL_FROSTBOLT = 21369
local SPELL_ICE_ARMOR = 18100
local SPELL_FREEZE = 18763
local SPELL_FEAR = 26070
local SPELL_CHILL_NOVA = 18099

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

-- C++ SelectTarget(Random, 0, 40, true): random alive player within
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

-- C++ EVENT_ICE_ARMOR: non-triggered DoCast(me, 18100); re-arm 3min.
local function onIceArmor(creature, guid)
    creature:CastSpell(creature, SPELL_ICE_ARMOR)
    schedule(guid, "ice_armor", 180000, function()
        onIceArmor(creature, guid)
    end)
end

-- C++ EVENT_FROSTBOLT: non-triggered DoCast(random target, 21369);
-- re-arm 8s. Nil pick casts nothing.
local function onFrostbolt(creature, guid)
    local target = randomPlayerInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_FROSTBOLT)
    end
    schedule(guid, "frostbolt", 8000, function()
        onFrostbolt(creature, guid)
    end)
end

-- C++ EVENT_CHILL_NOVA: non-triggered DoCastVictim(18099); re-arm
-- 14s. Nil-victim tick casts nothing but keeps the schedule.
local function onChillNova(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHILL_NOVA)
    end
    schedule(guid, "chill_nova", 14000, function()
        onChillNova(creature, guid)
    end)
end

-- C++ EVENT_FREEZE: non-triggered DoCastVictim(18763); re-arm 24s.
-- Nil-victim tick casts nothing but keeps the schedule.
local function onFreeze(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FREEZE)
    end
    schedule(guid, "freeze", 24000, function()
        onFreeze(creature, guid)
    end)
end

-- C++ EVENT_FEAR: non-triggered DoCastVictim(26070); re-arm 30s.
-- Nil-victim tick casts nothing but keeps the schedule.
local function onFear(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FEAR)
    end
    schedule(guid, "fear", 30000, function()
        onFear(creature, guid)
    end)
end

local function rasFrostwhisperResetState(guid)
    cancelTimers(guid)
end

-- C++ Reset: events.Reset() + non-triggered DoCast(me, 18100).
local function rasFrostwhisperReset(event, creature)
    local guid = creature:GetGUID()
    rasFrostwhisperResetState(guid)
    creature:CastSpell(creature, SPELL_ICE_ARMOR)
end

local function rasFrostwhisperEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    rasFrostwhisperResetState(guid)
    schedule(guid, "ice_armor", 2000, function()
        onIceArmor(creature, guid)
    end)
    schedule(guid, "frostbolt", 8000, function()
        onFrostbolt(creature, guid)
    end)
    schedule(guid, "chill_nova", 12000, function()
        onChillNova(creature, guid)
    end)
    schedule(guid, "freeze", 18000, function()
        onFreeze(creature, guid)
    end)
    schedule(guid, "fear", 45000, function()
        onFear(creature, guid)
    end)
end

local function rasFrostwhisperLeaveCombat(event, creature)
    rasFrostwhisperResetState(creature:GetGUID())
end

local function rasFrostwhisperDied(event, creature, killer)
    rasFrostwhisperResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_RAS_FROSTWHISPER, 1, rasFrostwhisperEnterCombat)
RegisterCreatureEvent(ENTRY_RAS_FROSTWHISPER, 2, rasFrostwhisperLeaveCombat)
RegisterCreatureEvent(ENTRY_RAS_FROSTWHISPER, 4, rasFrostwhisperDied)
RegisterCreatureEvent(ENTRY_RAS_FROSTWHISPER, 23, rasFrostwhisperReset)
