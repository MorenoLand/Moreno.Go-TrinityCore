-- Lord Alexei Barov (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_lord_alexei_barov.cpp
-- (boss_lordalexeibarovAI only; CreatureScript name
-- "boss_lord_alexei_barov", EK loader AddSC_boss_lordalexeibarov decl
-- :116 / call :294). No NPC_ constant in scholomance.h —
-- RegisterLuaBoss binds the creature_template ScriptName
-- "boss_lord_alexei_barov" DB-side; entry wotlk.wowhead-verified:
-- 10504. Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnReset: self-cast
-- unholy aura 17467 only if the aura is absent (no HasAura bridge —
-- per-guid Lua latch, kirtonos/arlokk convention; cleared on 2/4/23
-- when the aura drops). OnEnterCombat: immolate 20294 (triggered
-- DoCast on a random alive player within 100yd incl. victim, 7s->12s)
-- / veil of shadow 17820 (triggered DoCastVictim, 15s->20s);
-- nil-victim/nil-pick ticks cast nothing but keep the schedule
-- (moroes/maiden + janalai/kelris convention). Zero Talk lines in
-- C++ (no _SAY enum).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers
-- fire unconditionally. Documented-only (no bridges):
-- BossAI::JustEngagedWith + DATA_LORDALEXEIBAROV=3 bookkeeping
-- (scholomance.h:33) (luaBossAI shim).

local ENTRY_LORD_ALEXEI_BAROV = 10504

local SPELL_IMMOLATE = 20294
local SPELL_VEIL_OF_SHADOW = 17820
local SPELL_UNHOLY_AURA = 17467

local timers = {}
local auraApplied = {}

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

-- C++ EVENT_IMMOLATE: triggered DoCast(random target, 20294);
-- re-arm 12s. Nil pick casts nothing.
local function onImmolate(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_IMMOLATE, true)
    end
    schedule(guid, "immolate", 12000, function()
        onImmolate(creature, guid)
    end)
end

-- C++ EVENT_VEILOFSHADOW: triggered DoCastVictim(17820); re-arm
-- 20s. Nil-victim tick casts nothing but keeps the schedule
-- (moroes/maiden triggered convention).
local function onVeilOfShadow(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_VEIL_OF_SHADOW, true)
    end
    schedule(guid, "veil_of_shadow", 20000, function()
        onVeilOfShadow(creature, guid)
    end)
end

local function lordAlexeiBarovResetState(guid)
    cancelTimers(guid)
    auraApplied[guid] = nil
end

-- C++ Reset: _Reset() + non-triggered DoCast(me, SPELL_UNHOLY_AURA)
-- only if !me->HasAura(SPELL_UNHOLY_AURA). No HasAura bridge — the
-- latch models "aura present" (kirtonos convention); it clears on
-- 2/4/23 when the aura is lost, so re-apply after the next reset.
local function lordAlexeiBarovReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if not auraApplied[guid] then
        creature:CastSpell(creature, SPELL_UNHOLY_AURA, false)
        auraApplied[guid] = true
    end
end

-- C++ JustEngagedWith: schedule immolate 7s / veil of shadow 15s.
local function lordAlexeiBarovEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    schedule(guid, "immolate", 7000, function()
        onImmolate(creature, guid)
    end)
    schedule(guid, "veil_of_shadow", 15000, function()
        onVeilOfShadow(creature, guid)
    end)
end

local function lordAlexeiBarovLeaveCombat(event, creature)
    lordAlexeiBarovResetState(creature:GetGUID())
end

local function lordAlexeiBarovDied(event, creature, killer)
    lordAlexeiBarovResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LORD_ALEXEI_BAROV, 1, lordAlexeiBarovEnterCombat)
RegisterCreatureEvent(ENTRY_LORD_ALEXEI_BAROV, 2, lordAlexeiBarovLeaveCombat)
RegisterCreatureEvent(ENTRY_LORD_ALEXEI_BAROV, 4, lordAlexeiBarovDied)
RegisterCreatureEvent(ENTRY_LORD_ALEXEI_BAROV, 23, lordAlexeiBarovReset)
