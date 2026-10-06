-- Temple of Ahn'Qiraj: Kri (Bug Trio) — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_bug_trio.cpp
-- (boss_kriAI : public BossAI(creature, DATA_BUG_TRIO); GetAI via
-- GetAQ40AI<boss_kriAI>(creature); registered in AddSC_bug_trio();
-- kalimdor loader decl 91 — fifth "// Temple of ahn'qiraj" group).
-- Entry: 15511 = NPC_KRI (worker-verified).
-- Ported: Cleave 26350 on victim (4-8s -> 5-12s, jeklik
-- GetVictim + CastSpell convention); Toxic Volley 25812 on victim
-- (6-12s -> 10-15s); Poison Cloud 38718 on victim at <5% health
-- (event 9, latched — C++ Death flag).
-- Documented-only: Vem-dead Enrage 34624 (instance DATA_VEMISDEAD
-- — no bridge); JustDied loot-flag + instance DATA_BUG_TRIO_DEATH
-- (no bridge).
local ENTRY = 15511

local SPELL_CLEAVE = 26350
local SPELL_TOXIC_VOLLEY = 25812
local SPELL_POISON_CLOUD = 38718

local timers = {}
local kriState = {}

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

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", math.random(5000, 12000), function()
        onCleave(creature, guid)
    end)
end

local function onVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TOXIC_VOLLEY)
    end
    schedule(guid, "volley", math.random(10000, 15000), function()
        onVolley(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kriState[guid] = { deathCloud = false }
    schedule(guid, "cleave", math.random(4000, 8000), function()
        onCleave(creature, guid)
    end)
    schedule(guid, "volley", math.random(6000, 12000), function()
        onVolley(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kriState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kriState[guid] = { deathCloud = false }
end

-- C++: !HealthAbovePct(5) && !Death -> Poison Cloud, latched.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = kriState[guid]
    if state == nil or state.deathCloud then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 5 then
        state.deathCloud = true
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_POISON_CLOUD)
        end
    end
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kriState[guid] = nil
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
