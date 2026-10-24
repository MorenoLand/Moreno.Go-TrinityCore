-- Temple of Ahn'Qiraj: Princess Yauj (Bug Trio) — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_bug_trio.cpp
-- (boss_yaujAI : public BossAI(creature, DATA_BUG_TRIO); GetAI via
-- GetAQ40AI<boss_yaujAI>(creature); registered in AddSC_bug_trio();
-- kalimdor loader decl 91 — fifth "// Temple of ahn'qiraj" group).
-- Entry: 15543 (no NPC_YAUJ constant exists in temple_of_ahnqiraj.h —
-- only NPC_VEM = 15544 and NPC_KRI = 15511; binding is DB-side via
-- RegisterTempleOfAhnQirajCreatureAI).
-- Ported: Fear 19408 on victim (12-24s -> 20s) + the ResetThreatList leg
-- (2026-10-21, creature:ResetAllThreat — fires ungated C++-exact).
-- Documented-only: Heal 25807 (target selection via instance
-- GetCreature(DATA_KRI/DATA_VEM) — no bridge; self-cast alone
-- would misrepresent the random 1-of-3); Vem-dead Enrage (instance
-- DATA_VEMISDEAD — no bridge); JustDied 10x summon (SummonCreature
-- — zero Lua usage) + instance DATA_BUG_TRIO_DEATH.
local ENTRY = 15543

local SPELL_FEAR = 19408

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

local function onFear(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FEAR)
    end
    -- C++ boss_bug_trio.cpp:290 — ResetThreatList() fires with the fear
    -- cast, ungated on victim (bridged lua_creature_events.go).
    creature:ResetAllThreat()
    schedule(guid, "fear", 20000, function()
        onFear(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fear", math.random(12000, 24000), function()
        onFear(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
