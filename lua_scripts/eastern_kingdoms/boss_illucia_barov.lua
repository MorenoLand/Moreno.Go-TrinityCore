-- Lady Illucia Barov (10502), Scholomance — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/
-- boss_illucia_barov.cpp (creature AI only; script name
-- "boss_illucia_barov", loader AddSC_boss_illuciabarov decl :112 /
-- call :290). Entry has no NPC_ constant in scholomance.h
-- (DB-side ScriptName binding; wowhead TBC-verified npc=10502).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
--
-- Modeled arms (C++-exact where bridges exist):
-- OnEnterCombat: curse of agony 18671 at 18s / shadow shock 17234 at
-- 9s / silence 12528 at 5s / fear 12542 at 30s. Curse/silence/fear are
-- triggered DoCastVictim -> GetVictim + CastSpell(victim, spell,
-- true), nil-victim keeps schedule (moroes/maiden/gandling triggered
-- convention), re-arms 30s/14s/30s. Shadow shock targets a random
-- alive player within 100yd (SelectTarget(Random, 0, 100, true),
-- janalai/kelris/gandling convention), triggered cast, nil pick casts
-- nothing, re-arm 12s.
--
-- Documented-unmodeled (no bridges): UpdateAI UNIT_STATE_CASTING queue
-- + post-event gates + BossAI::JustEngagedWith +
-- DATA_LADYILLUCIABAROV=2 bookkeeping (scholomance.h:32) (luaBossAI
-- shim). SPELL_DOMINATE 7645 is declared in C++ but marked UNUSED and
-- never cast.

local ENTRY_ILLUCIA = 10502

local SPELL_CURSE_OF_AGONY = 18671
local SPELL_SHADOW_SHOCK = 17234
local SPELL_SILENCE = 12528
local SPELL_FEAR = 12542

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

-- C++ SelectTarget(Random, 0, 100, true): random alive player within
-- maxDist yards (janalai/kelris convention).
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

-- C++ triggered DoCastVictim -> nil-victim keeps schedule (moroes/maiden
-- triggered convention).
local function castOnVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell, true)
    end
end

-- C++ EVENT_CURSEOFAGONY: triggered DoCastVictim(18671); Repeat(30s).
local function onCurseOfAgony(creature, guid)
    castOnVictim(creature, SPELL_CURSE_OF_AGONY)
    schedule(guid, "curseOfAgony", 30000, function()
        onCurseOfAgony(creature, guid)
    end)
end

-- C++ EVENT_SHADOWSHOCK: triggered DoCast(random alive player within
-- 100yd, 17234); Repeat(12s). Nil pick casts nothing.
local function onShadowShock(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_SHOCK, true)
    end
    schedule(guid, "shadowShock", 12000, function()
        onShadowShock(creature, guid)
    end)
end

-- C++ EVENT_SILENCE: triggered DoCastVictim(12528); Repeat(14s).
local function onSilence(creature, guid)
    castOnVictim(creature, SPELL_SILENCE)
    schedule(guid, "silence", 14000, function()
        onSilence(creature, guid)
    end)
end

-- C++ EVENT_FEAR: triggered DoCastVictim(12542); Repeat(30s).
local function onFear(creature, guid)
    castOnVictim(creature, SPELL_FEAR)
    schedule(guid, "fear", 30000, function()
        onFear(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (shim) + the four events.
local function illuciaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "curseOfAgony", 18000, function()
        onCurseOfAgony(creature, guid)
    end)
    schedule(guid, "shadowShock", 9000, function()
        onShadowShock(creature, guid)
    end)
    schedule(guid, "silence", 5000, function()
        onSilence(creature, guid)
    end)
    schedule(guid, "fear", 30000, function()
        onFear(creature, guid)
    end)
end

local function illuciaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function illuciaDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: _Reset (no extra legs in this script).
local function illuciaReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ILLUCIA, 1, illuciaEnterCombat)
RegisterCreatureEvent(ENTRY_ILLUCIA, 2, illuciaLeaveCombat)
RegisterCreatureEvent(ENTRY_ILLUCIA, 4, illuciaDied)
RegisterCreatureEvent(ENTRY_ILLUCIA, 23, illuciaReset)
