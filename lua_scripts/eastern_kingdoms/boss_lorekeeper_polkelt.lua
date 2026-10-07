-- Lorekeeper Polkelt (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_lorekeeper_polkelt.cpp
-- (boss_lorekeeperpolkeltAI only; CreatureScript name
-- "boss_lorekeeper_polkelt", EK loader AddSC_boss_lorekeeperpolkelt
-- decl :117 / call :295). No NPC_ constant in scholomance.h —
-- RegisterLuaBoss binds the creature_template ScriptName
-- "boss_lorekeeper_polkelt" DB-side; entry classic.wowhead-verified:
-- 10901. Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- volatile infection 24928 (triggered DoCastVictim, 38s->32s) / dark
-- plague 18270 (triggered DoCastVictim, 8s->8s) / corrosive acid 23313
-- (triggered DoCastVictim, 45s->25s) / noxious catalyst 18151
-- (triggered DoCastVictim, 35s->38s). Triggered DoCastVictim ->
-- GetVictim + CastSpell(victim, spell, true); nil-victim ticks cast
-- nothing but keep the schedule (moroes/maiden convention). Zero
-- Talk lines in C++ (no _SAY enum).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers
-- fire unconditionally (jeklik convention). Documented-only (no
-- bridges): BossAI::JustEngagedWith + DATA_LOREKEEPERPOLKELT=4
-- bookkeeping (scholomance.h:34) (luaBossAI shim).

local ENTRY_LOREKEEPER_POLKELT = 10901

local SPELL_VOLATILE_INFECTION = 24928
local SPELL_DARK_PLAGUE = 18270
local SPELL_CORROSIVE_ACID = 23313
local SPELL_NOXIOUS_CATALYST = 18151

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

-- C++ EVENT_VOLATILEINFECTION: triggered DoCastVictim(24928);
-- re-arm 32s.
local function onVolatileInfection(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_VOLATILE_INFECTION, true)
    end
    schedule(guid, "volatile_infection", 32000, function()
        onVolatileInfection(creature, guid)
    end)
end

-- C++ EVENT_DARKPLAGUE: triggered DoCastVictim(18270); re-arm 8s.
local function onDarkPlague(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DARK_PLAGUE, true)
    end
    schedule(guid, "dark_plague", 8000, function()
        onDarkPlague(creature, guid)
    end)
end

-- C++ EVENT_CORROSIVEACID: triggered DoCastVictim(23313); re-arm 25s.
local function onCorrosiveAcid(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CORROSIVE_ACID, true)
    end
    schedule(guid, "corrosive_acid", 25000, function()
        onCorrosiveAcid(creature, guid)
    end)
end

-- C++ EVENT_NOXIOUSCATALYST: triggered DoCastVictim(18151); re-arm
-- 38s.
local function onNoxiousCatalyst(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_NOXIOUS_CATALYST, true)
    end
    schedule(guid, "noxious_catalyst", 38000, function()
        onNoxiousCatalyst(creature, guid)
    end)
end

local function lorekeeperPolkeltResetState(guid)
    cancelTimers(guid)
end

local function lorekeeperPolkeltEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    lorekeeperPolkeltResetState(guid)
    schedule(guid, "volatile_infection", 38000, function()
        onVolatileInfection(creature, guid)
    end)
    schedule(guid, "dark_plague", 8000, function()
        onDarkPlague(creature, guid)
    end)
    schedule(guid, "corrosive_acid", 45000, function()
        onCorrosiveAcid(creature, guid)
    end)
    schedule(guid, "noxious_catalyst", 35000, function()
        onNoxiousCatalyst(creature, guid)
    end)
end

local function lorekeeperPolkeltLeaveCombat(event, creature)
    lorekeeperPolkeltResetState(creature:GetGUID())
end

local function lorekeeperPolkeltDied(event, creature, killer)
    lorekeeperPolkeltResetState(creature:GetGUID())
end

local function lorekeeperPolkeltReset(event, creature)
    lorekeeperPolkeltResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LOREKEEPER_POLKELT, 1, lorekeeperPolkeltEnterCombat)
RegisterCreatureEvent(ENTRY_LOREKEEPER_POLKELT, 2, lorekeeperPolkeltLeaveCombat)
RegisterCreatureEvent(ENTRY_LOREKEEPER_POLKELT, 4, lorekeeperPolkeltDied)
RegisterCreatureEvent(ENTRY_LOREKEEPER_POLKELT, 23, lorekeeperPolkeltReset)
