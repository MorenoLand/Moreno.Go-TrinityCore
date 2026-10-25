-- Patchwerk (Naxxramas, Construct Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_patchwerk.cpp
-- (boss_patchwerk (BossAI); AddSC_boss_patchwerk registers only it).
-- Entry: 16028 Patchwerk (creature_template table, cross-checked:
-- Anub'Rekhan 15956 / Faerlina 15953 / Thaddius 15928 / Maexxna 15952 /
-- Gluth 15932 / Heigan 15936 / Sapphiron 15989 / Kel'Thuzad 15990 all
-- match naxxramas.h NAXCreaturesIds; 25-man difficulty entry 29324 is
-- NOT registered, base-entry-only convention like anubrekhan/maexxna).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith Talk(SAY_AGGRO 0) (event 1; BossAI::
-- JustEngagedWith instance-bookkeeping leg has no bridge — tharon_ja
-- precedent) + EVENT_HATEFUL 1s + EVENT_BERSERK 6min; EVENT_HATEFUL
-- threat-list algorithm (threat-ordered GetAITargets = C++ threat
-- list order, curator precedent; GetVictim = currentVictim):
-- first non-victim entry is secondThreat (no availability check on
-- the first entry, C++-exact); when no secondThreat yet or 25-man,
-- the next available entry becomes thirdThreat if secondThreat is
-- set, else secondThreat; when still no thirdThreat on 25-man, the
-- next available entry becomes thirdThreat with NO victim check
-- (C++-exact quirk); pick is currentVictim when no secondThreat,
-- secondThreat when no thirdThreat, else the higher-HP of
-- second/thirdThreat; AddThreat(victim, 1000) + second/thirdThreat
-- 1000 each (AddThreat bridge is self-threat-only, which is exactly
-- C++ AddThreat here); DoCast(pHatefulTarget, HATEFUL_STRIKE
-- 28308, triggered); 1s repeat. 25-man via creature:Is25ManRaid()
-- (C++ :137,141). Frenzy
-- latch (UpdateAI HealthBelowPct(5) arm) is folded into the 1s
-- hateful pump: DoCastSelf(FRENZY 28131, triggered) + Talk(EMOTE_
-- FRENZY 4) once (C++ checks every UpdateAI tick; 1s granularity
-- documented). HealthBelowPct(5) = health < floor(max*5/100)
-- (Unit.h:872 + CalculatePct trunc, Util.h:66); the float
-- GetHealthPct form fired 1 HP early on odd max (telestra
-- precedent). EVENT_BERSERK DoCastSelf(BERSERK 26662,
-- triggered) + Talk(EMOTE_BERSERK 3), one-shot, then EVENT_SLIME
-- 2s: DoCastAOE(SLIME_BOLT 32309, triggered) lowered to self-cast
-- (DoCastAOE self-cast convention, selin/mr_smite precedent),
-- 2s repeat. KilledUnit Talk(SAY_SLAY 1) at 1-in-5
-- (C++ rand32() % 5 == 0). JustDied Talk(SAY_DEATH 2).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired): Reset/JustDied/
-- JustEngagedWith instance bookkeeping (_Reset/_JustDied,
-- DoStartTimedAchievement/DoStopTimedAchievement for achievement
-- 10286 MAKE_QUICK_WERK_OF_HIM); the empty-threat-list
-- EnterEvadeMode leg (EnterEvadeMode unbridged, 2026-10-21
-- lesson — the pump re-arms instead of evading when the list is
-- empty); melee engine-driven.

local ENTRY_PATCHWERK = 16028

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local EMOTE_BERSERK = 3
local EMOTE_FRENZY = 4

local SPELL_HATEFUL_STRIKE = 28308
local SPELL_FRENZY = 28131
local SPELL_BERSERK = 26662
local SPELL_SLIME_BOLT = 32309

local HATEFUL_THREAT_AMT = 1000

local timers = {}
local enraged = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    enraged[guid] = nil
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

local function hatefulTick(creature, guid)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    if victim ~= nil and #targets > 0 then
        local victimGuid = victim:GetGUID()
        local second, third = nil, nil
        if targets[1]:GetGUID() ~= victimGuid then
            second = targets[1]
        end
        local twentyFive = creature:Is25ManRaid()
        if (second == nil or twentyFive) and targets[2] ~= nil
                and not targets[2]:IsDead() then
            if targets[2]:GetGUID() ~= victimGuid then
                if second ~= nil then
                    third = targets[2]
                else
                    second = targets[2]
                end
            end
            if third == nil and twentyFive and targets[3] ~= nil
                    and not targets[3]:IsDead() then
                third = targets[3]
            end
        end
        local hatefulTarget = victim
        if second ~= nil and third == nil then
            hatefulTarget = second
        elseif third ~= nil then
            if second:GetHealth() < third:GetHealth() then
                hatefulTarget = third
            else
                hatefulTarget = second
            end
        end
        creature:AddThreat(victim, HATEFUL_THREAT_AMT)
        if second ~= nil then
            creature:AddThreat(second, HATEFUL_THREAT_AMT)
        end
        if third ~= nil then
            creature:AddThreat(third, HATEFUL_THREAT_AMT)
        end
        creature:CastSpell(hatefulTarget, SPELL_HATEFUL_STRIKE, true)
    end
    if not enraged[guid] and creature:GetHealth() < math.floor(creature:GetMaxHealth() * 5 / 100) then
        creature:CastSpell(creature, SPELL_FRENZY, true)
        creature:Talk(EMOTE_FRENZY)
        enraged[guid] = true
    end
    schedule(guid, "hateful", 1000, function()
        hatefulTick(creature, guid)
    end)
end

local function slimeTick(creature, guid)
    creature:CastSpell(creature, SPELL_SLIME_BOLT, true)
    schedule(guid, "slime", 2000, function()
        slimeTick(creature, guid)
    end)
end

local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    creature:Talk(EMOTE_BERSERK)
    schedule(guid, "slime", 2000, function()
        slimeTick(creature, guid)
    end)
end

local function patchwerkEnterCombat(event, creature)
    local guid = creature:GetGUID()
    enraged[guid] = false
    creature:Talk(SAY_AGGRO)
    schedule(guid, "hateful", 1000, function()
        hatefulTick(creature, guid)
    end)
    schedule(guid, "berserk", 360000, function()
        berserkTick(creature, guid)
    end)
end

local function patchwerkLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function patchwerkTargetDied(event, creature, victim)
    if math.random(5) == 1 then
        creature:Talk(SAY_SLAY)
    end
end

local function patchwerkDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function patchwerkReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PATCHWERK, 1, patchwerkEnterCombat)
RegisterCreatureEvent(ENTRY_PATCHWERK, 2, patchwerkLeaveCombat)
RegisterCreatureEvent(ENTRY_PATCHWERK, 3, patchwerkTargetDied)
RegisterCreatureEvent(ENTRY_PATCHWERK, 4, patchwerkDied)
RegisterCreatureEvent(ENTRY_PATCHWERK, 23, patchwerkReset)
