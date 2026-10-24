-- Temple of Ahn'Qiraj: Vem (Bug Trio) — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_bug_trio.cpp
-- (boss_vemAI : public BossAI(creature, DATA_BUG_TRIO); GetAI via
-- GetAQ40AI<boss_vemAI>(creature); registered in AddSC_bug_trio();
-- kalimdor loader decl 91 — fifth "// Temple of ahn'qiraj" group).
-- Entry: 15544 = NPC_VEM (worker-verified).
-- Ported: Charge 26561 on random player (15-27s -> 8-16s;
-- C++ SelectTarget Random is bridged via the maiden_of_virtue
-- idiom — GetPlayersInWorld filtered by map/instance/alive;
-- the AttackStart leg has no bridge); Knockback 26027 on victim
-- (8-20s -> 15-25s; the -80% threat modify is PORTED 2026-10-24);
-- Enrage 34624 is NOT ported: boss_bug_trio.cpp:219's enrage arm is
-- dead code in this tree — the else-branch decrements Charge_Timer
-- instead of Enrage_Timer, so Enrage_Timer never ticks down and the
-- cast never fires (side effect: Charge_Timer is double-decremented
-- per tick). If the upstream/local C++ is fixed, re-add a latched
-- 120s self-cast 34624 (akilzon precedent).
-- Documented-only: JustDied instance DATA_VEM_DEATH +
-- DATA_BUG_TRIO_DEATH + loot flag (no bridge).
local ENTRY = 15544

local SPELL_CHARGE = 26561
local SPELL_KNOCKBACK = 26027

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

local function onCharge(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_CHARGE)
    end
    schedule(guid, "charge", math.random(8000, 16000), function()
        onCharge(creature, guid)
    end)
end

local function onKnockback(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKBACK)
        if creature:GetThreat(victim) ~= 0 then
            creature:ModifyThreatByPercent(victim, -80)
        end
    end
    schedule(guid, "knockback", math.random(15000, 25000), function()
        onKnockback(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "charge", math.random(15000, 27000), function()
        onCharge(creature, guid)
    end)
    schedule(guid, "knockback", math.random(8000, 20000), function()
        onKnockback(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
