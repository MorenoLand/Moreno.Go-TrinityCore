-- The Four Horsemen (Naxxramas, Military Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_four_horsemen.cpp
-- (boss_four_horsemen_baron / _thane / _lady / _sir, all on
-- boss_four_horsemen_baseAI (BossAI); AddSC_boss_four_horsemen
-- also registers spell_four_horsemen_mark (AuraScript) and
-- spell_four_horsemen_consumption (SpellScript), unmodeled).
-- Entries: 16064 Thane Korth'azz, 16065 Lady Blaumeux,
-- 30549 Baron Rivendare, 16063 Sir Zeliek (naxxramas.h
-- NAXCreaturesIds; 25-man difficulty entries 30603 / 30601 /
-- 30600 / 30602 are NOT registered, base-entry-only
-- convention like anubrekhan/maexxna).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 15 OnSpellHitTarget, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go,
-- like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf
-- (DoCastAOE is lowered to self-cast, selin/mr_smite
-- precedent); the trailing true is the triggered port.
-- Ported arms (C++-exact for all modeled arms):
-- Reset (event 23): SetReactState(2) AGGRESSIVE +
-- SetCombatMovement(false) + SetCombatPulseDelay(0)
-- (C++ Reset order; ResetLootMode / summons.DespawnAll
-- have no bridge — see below); JustEngagedWith Talk
-- (SAY_AGGRO 0) + SetCombatPulseDelay(5) (the DoAction
-- BEGIN_FIGHTING pulse leg) + per-horseman BeginFighting
-- schedules (see below — the corner-run gating is
-- unmodeled, so the fight starts on engage instead);
-- Baron BeginFighting: SetCombatMovement(true) +
-- SetReactState(2) + EVENT_BERSERK 10min one-shot
-- (DoCastAOE BERSERK 26662, triggered, self-cast) +
-- EVENT_MARK 24s (DoCastAOE BARON_MARK 28834, triggered,
-- self-cast, 12s repeat) + EVENT_UNHOLYSHADOW
-- randtime(3s,7s) (DoCastVictim UNHOLY_SHADOW 28882,
-- randtime(10s,30s) repeat); SpellHitTarget (event 15)
-- UNHOLY_SHADOW hit -> Talk(SAY_SPECIAL 2);
-- Thane BeginFighting: SetCombatMovement(true) +
-- SetReactState(2) + BERSERK 10min + MARK 24s
-- (THANE_MARK 28832, triggered, self-cast, 12s repeat) +
-- METEOR randtime(10s,25s) (random player within 20yd,
-- DoCast METEOR 28884, randtime(13s,17s) repeat,
-- _shouldSay re-armed on cast); SpellHitTarget METEOR
-- hit + _shouldSay -> Talk(SAY_SPECIAL 2), latch cleared;
-- Lady BeginFighting: BERSERK 10min + MARK 24s
-- (LADY_MARK 28833, triggered, self-cast, 15s repeat) +
-- VOIDZONE randtime(5s,10s) (random player within 45yd,
-- DoCast VOID_ZONE 28863, triggered, + Talk(SAY_SPECIAL 2),
-- randtime(12s,18s) repeat) + 2s filler pump (nearest
-- target within 45yd -> DoCast SHADOW_BOLT 57374, else
-- DoCastAOE UNYIELDING_PAIN 57381 self-cast + Talk
-- (EMOTE_RAGECAST 7) — C++ casts this every UpdateAI tick,
-- the 2s pump is the bronjahm filler precedent, the
-- engine cast time being the real gate in C++);
-- Sir BeginFighting: BERSERK 10min + MARK 24s
-- (SIR_MARK 28835, triggered, self-cast, 15s repeat) +
-- HOLYWRATH randtime(13s,18s) (nearest player within
-- 45yd, DoCast HOLY_WRATH 28883, triggered, _shouldSay
-- re-armed on cast, randtime(10s,18s) repeat) + 2s filler
-- pump (nearest target within 45yd -> DoCast HOLY_BOLT
-- 57376, else DoCastAOE CONDEMNATION 57377 self-cast +
-- Talk(EMOTE_RAGECAST 7)); SpellHitTarget HOLY_WRATH
-- hit + _shouldSay -> Talk(SAY_SPECIAL 2), latch cleared.
-- KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- commanders precedent); JustDied Talk(SAY_DEATH 4).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Distance filters use creature:GetDistance (amanitar /
-- bpc precedent); the 45yd/20yd bounds are C++-exact.
-- Unmodeled (no bridges — documented, not wired): the
-- whole encounter choreography — BeginEncounter
-- (instance boss state + required-boss check, all four
-- PASSIVE + AttackStop + SetFarVisible + AddThreat 0 to
-- every alive non-GM player, DoAction BEGIN_MOVEMENT),
-- the MovePoint corner paths + MovementInform chain
-- (no motion bridge), CheckIsMovementFinished /
-- BEGIN_FIGHTING gating (no cross-AI DoAction/GetData
-- bridge — the fight therefore starts on engage rather
-- than after the run, a timing deviation, not a spell
-- deviation); BeginFighting AttackStart legs: baron/thane
-- threat-nonempty AttackStart(current victim) ported
-- 2026-10-24 via EngageWithTarget(GetVictim()) in C++
-- order; threat-empty nearest-player + AddThreat leg stays
-- out (no player-search bridge on an empty threat list;
-- ResetEncounter unbridged); lady/sir have no BeginFighting
-- AttackStart leg in C++ (their threat-empty EnterEvadeMode
-- legs stay unbridged);
-- JustDied instance legs (boss-state check, death-time
-- latches, CheckIsMovementFinished, EncounterCleared
-- DoCastAOE ENCOUNTER_CREDIT 59450, timed-kill
-- achievement data); SpellHitTarget on lady voidzone
-- (C++ has no SpellHitTarget arm for lady — only the
-- direct Talk on cast, ported above); the mark
-- AuraScript stack-damage table (SPELL_MARK_DAMAGE
-- 28836) and consumption SpellScript (no SpellScript
-- bridge); Reset ResetLootMode + summons.DespawnAll
-- (no bridges); C++ Talk(SAY_AGGRO) fires once for the
-- whole encounter (instance-gated) — Lua has no instance
-- state, so each horseman Talks on its own engage.

local ENTRY_THANE = 16064
local ENTRY_LADY = 16065
local ENTRY_BARON = 30549
local ENTRY_SIR = 16063

local SAY_AGGRO = 0
local SAY_SPECIAL = 2
local SAY_SLAY = 3
local SAY_DEATH = 4
local EMOTE_RAGECAST = 7

local SPELL_BERSERK = 26662
local SPELL_BARON_MARK = 28834
local SPELL_THANE_MARK = 28832
local SPELL_LADY_MARK = 28833
local SPELL_SIR_MARK = 28835
local SPELL_UNHOLY_SHADOW = 28882
local SPELL_METEOR = 28884
local SPELL_VOID_ZONE = 28863
local SPELL_SHADOW_BOLT = 57374
local SPELL_UNYIELDING_PAIN = 57381
local SPELL_HOLY_WRATH = 28883
local SPELL_HOLY_BOLT = 57376
local SPELL_CONDEMNATION = 57377

local timers = {}
local shouldSay = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    shouldSay[guid] = nil
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

-- C++ SelectTarget dist gate is IsWithinCombatRange (Unit.cpp:
-- 583-598: 3D dist < nominal + both combat reaches; player reach is
-- DEFAULT_PLAYER_COMBAT_REACH 1.5, ObjectDefines.h:40 — player
-- scale is 1.0 in practice, so the constant is C++-exact here).
local PLAYER_COMBAT_REACH = 1.5

local function randomPlayerInRange(creature, maxDist)
    local candidates = {}
    local bound = maxDist + creature:GetCombatReach() + PLAYER_COMBAT_REACH
    for _, target in ipairs(creature:GetAITargets()) do
        if target:GetObjectType() == "Player"
                and not target:IsDead()
                and creature:GetDistance(target) < bound then
            candidates[#candidates + 1] = target
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function nearestInRange(creature, maxDist, playerOnly)
    local best = nil
    local bestDist = nil
    local casterReach = creature:GetCombatReach()
    for _, target in ipairs(creature:GetAITargets()) do
        if not target:IsDead()
                and (not playerOnly or target:GetObjectType() == "Player") then
            local targetReach = PLAYER_COMBAT_REACH
            if target:GetObjectType() ~= "Player" then
                targetReach = target:GetCombatReach()
            end
            local d = creature:GetDistance(target)
            if d < maxDist + casterReach + targetReach
                    and (bestDist == nil or d < bestDist) then
                best = target
                bestDist = d
            end
        end
    end
    return best
end

local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

local function markTick(creature, guid, spell, repeatMs)
    creature:CastSpell(creature, spell, true)
    schedule(guid, "mark", repeatMs, function()
        markTick(creature, guid, spell, repeatMs)
    end)
end

local function baronUnholyTick(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        creature:CastSpell(victim, SPELL_UNHOLY_SHADOW)
    end
    schedule(guid, "unholy", math.random(10000, 30000), function()
        baronUnholyTick(creature, guid)
    end)
end

local function thaneMeteorTick(creature, guid)
    local target = randomPlayerInRange(creature, 20)
    if target ~= nil then
        creature:CastSpell(target, SPELL_METEOR)
        shouldSay[guid] = true
    end
    schedule(guid, "meteor", math.random(13000, 17000), function()
        thaneMeteorTick(creature, guid)
    end)
end

local function ladyVoidTick(creature, guid)
    local target = randomPlayerInRange(creature, 45)
    if target ~= nil then
        creature:CastSpell(target, SPELL_VOID_ZONE, true)
        creature:Talk(SAY_SPECIAL)
    end
    schedule(guid, "void", math.random(12000, 18000), function()
        ladyVoidTick(creature, guid)
    end)
end

local function ladyBoltTick(creature, guid)
    local target = nearestInRange(creature, 45, false)
    if target ~= nil then
        creature:CastSpell(target, SPELL_SHADOW_BOLT)
    else
        creature:CastSpell(creature, SPELL_UNYIELDING_PAIN)
        creature:Talk(EMOTE_RAGECAST)
    end
    schedule(guid, "bolt", 2000, function()
        ladyBoltTick(creature, guid)
    end)
end

local function sirWrathTick(creature, guid)
    local target = nearestInRange(creature, 45, true)
    if target ~= nil then
        creature:CastSpell(target, SPELL_HOLY_WRATH, true)
        shouldSay[guid] = true
    end
    schedule(guid, "wrath", math.random(10000, 18000), function()
        sirWrathTick(creature, guid)
    end)
end

local function sirBoltTick(creature, guid)
    local target = nearestInRange(creature, 45, false)
    if target ~= nil then
        creature:CastSpell(target, SPELL_HOLY_BOLT)
    else
        creature:CastSpell(creature, SPELL_CONDEMNATION)
        creature:Talk(EMOTE_RAGECAST)
    end
    schedule(guid, "bolt", 2000, function()
        sirBoltTick(creature, guid)
    end)
end

local function beginFighting(creature, guid, which)
    creature:SetCombatPulseDelay(5)
    if which == "baron" or which == "thane" then
        creature:SetCombatMovement(true)
        creature:SetReactState(2)
        local victim = creature:GetVictim()
        if victim ~= nil then
            creature:EngageWithTarget(victim)
        end
    end
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature, guid)
    end)
    if which == "baron" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_BARON_MARK, 12000)
        end)
        schedule(guid, "unholy", math.random(3000, 7000), function()
            baronUnholyTick(creature, guid)
        end)
    elseif which == "thane" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_THANE_MARK, 12000)
        end)
        schedule(guid, "meteor", math.random(10000, 25000), function()
            thaneMeteorTick(creature, guid)
        end)
    elseif which == "lady" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_LADY_MARK, 15000)
        end)
        schedule(guid, "void", math.random(5000, 10000), function()
            ladyVoidTick(creature, guid)
        end)
        schedule(guid, "bolt", 2000, function()
            ladyBoltTick(creature, guid)
        end)
    elseif which == "sir" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_SIR_MARK, 15000)
        end)
        schedule(guid, "wrath", math.random(13000, 18000), function()
            sirWrathTick(creature, guid)
        end)
        schedule(guid, "bolt", 2000, function()
            sirBoltTick(creature, guid)
        end)
    end
end

local function makeHandlers(which)
    local function enterCombat(event, creature)
        local guid = creature:GetGUID()
        shouldSay[guid] = (which == "thane" or which == "sir")
        creature:Talk(SAY_AGGRO)
        beginFighting(creature, guid, which)
    end
    local function leaveCombat(event, creature)
        cancelTimers(creature:GetGUID())
    end
    local function targetDied(event, creature, victim)
        if victim ~= nil and victim:GetObjectType() == "Player" then
            creature:Talk(SAY_SLAY)
        end
    end
    local function died(event, creature, killer)
        creature:Talk(SAY_DEATH)
        cancelTimers(creature:GetGUID())
    end
    local function reset(event, creature)
        cancelTimers(creature:GetGUID())
        creature:SetReactState(2)
        creature:SetCombatMovement(false)
        creature:SetCombatPulseDelay(0)
    end
    return enterCombat, leaveCombat, targetDied, died, reset
end

local function spellHitTarget(event, creature, target, spellId)
    local guid = creature:GetGUID()
    if spellId == SPELL_UNHOLY_SHADOW then
        creature:Talk(SAY_SPECIAL)
    elseif spellId == SPELL_METEOR and shouldSay[guid] then
        creature:Talk(SAY_SPECIAL)
        shouldSay[guid] = false
    elseif spellId == SPELL_HOLY_WRATH and shouldSay[guid] then
        creature:Talk(SAY_SPECIAL)
        shouldSay[guid] = false
    end
end

local baronEnter, baronLeave, baronTargetDied, baronDied, baronReset =
    makeHandlers("baron")
local thaneEnter, thaneLeave, thaneTargetDied, thaneDied, thaneReset =
    makeHandlers("thane")
local ladyEnter, ladyLeave, ladyTargetDied, ladyDied, ladyReset =
    makeHandlers("lady")
local sirEnter, sirLeave, sirTargetDied, sirDied, sirReset =
    makeHandlers("sir")

RegisterCreatureEvent(ENTRY_BARON, 1, baronEnter)
RegisterCreatureEvent(ENTRY_BARON, 2, baronLeave)
RegisterCreatureEvent(ENTRY_BARON, 3, baronTargetDied)
RegisterCreatureEvent(ENTRY_BARON, 4, baronDied)
RegisterCreatureEvent(ENTRY_BARON, 15, spellHitTarget)
RegisterCreatureEvent(ENTRY_BARON, 23, baronReset)

RegisterCreatureEvent(ENTRY_THANE, 1, thaneEnter)
RegisterCreatureEvent(ENTRY_THANE, 2, thaneLeave)
RegisterCreatureEvent(ENTRY_THANE, 3, thaneTargetDied)
RegisterCreatureEvent(ENTRY_THANE, 4, thaneDied)
RegisterCreatureEvent(ENTRY_THANE, 15, spellHitTarget)
RegisterCreatureEvent(ENTRY_THANE, 23, thaneReset)

RegisterCreatureEvent(ENTRY_LADY, 1, ladyEnter)
RegisterCreatureEvent(ENTRY_LADY, 2, ladyLeave)
RegisterCreatureEvent(ENTRY_LADY, 3, ladyTargetDied)
RegisterCreatureEvent(ENTRY_LADY, 4, ladyDied)
RegisterCreatureEvent(ENTRY_LADY, 23, ladyReset)

RegisterCreatureEvent(ENTRY_SIR, 1, sirEnter)
RegisterCreatureEvent(ENTRY_SIR, 2, sirLeave)
RegisterCreatureEvent(ENTRY_SIR, 3, sirTargetDied)
RegisterCreatureEvent(ENTRY_SIR, 4, sirDied)
RegisterCreatureEvent(ENTRY_SIR, 15, spellHitTarget)
RegisterCreatureEvent(ENTRY_SIR, 23, sirReset)
