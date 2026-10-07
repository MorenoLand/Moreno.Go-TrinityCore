-- Arcanist Doan (Scarlet Monastery, Library wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_arcanist_doan.cpp (boss_arcanist_doan AI only); scarlet_monastery.h
-- names DATA_ARCANIST_DOAN (:39, index 3 of the SM encounter list) which
-- the BossAI constructor consumes.
-- Creature entry: 6487 Arcanist Doan (no NPC_ constant exists in the C++
-- tree — RegisterScarletMonasteryCreatureAI binds the creature_template
-- ScriptName DB-side; entry wowhead-cited:
-- classic.wowhead.com/npc=6487/arcanist-doan, also cited by the
-- TrinityCore issue "Scarlet Monastery Libary, mobs and boss under the
-- map" via wowhead.com/classic/npc=6487/arcanist-doan).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO 0) + silence 8988 15s then {15s,20s} (non-triggered
-- DoCastVictim -> GetVictim + CastSpell; re-arm via
-- math.random(15000, 20000) — sulfuron convention) / arcane explosion
-- 9433 3s then 8s (non-triggered DoCastVictim; nil-victim ticks cast
-- nothing but keep the schedule — jeklik convention) / polymorph 13323
-- 30s then 20s (C++ SelectTarget(Random, 1, 30.0f, true): random alive
-- player within 30 yd skipping position 0 (the current victim) —
-- majordomo victim-excluding convention + opera alivePlayersInRange;
-- a sole-victim tick casts nothing but keeps the schedule; the 20s
-- re-arm is unconditional, matching C++ Repeat outside the if). The
-- UpdateAI below-50% latch (Talk(SAY_SPECIALAE 1) + non-triggered
-- DoCastSelf arcane bubble 9438, once via the _healthAbove50Pct bool) is
-- modeled on the pre-damage hook (moroes/chromaggus/golemagg
-- convention): current (pre-damage) health strictly below 50% (C++
-- HealthBelowPct(50) reads current health in UpdateAI — golemagg
-- 704fb8e bug class, so the latch does NOT subtract the incoming
-- damage) and not yet latched -> Talk(1) + non-triggered self-cast
-- 9438; per-GUID Lua state holds the once-guard (no HasAura bridge),
-- cleared on 2/4/23. OnDied/OnLeaveCombat/OnReset: cancel timers,
-- clear the latch.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers fire
-- unconditionally (jeklik convention); the below-50% latch fires on
-- the damage hook rather than the next UpdateAI tick (tick timing
-- deviation, documented); BossAI::JustEngagedWith and the
-- DATA_ARCANIST_DOAN encounter bookkeeping have no instance-script
-- bridge — boss admission via the luaBossAI shim; DoCastAOE(detona-
-- tion 9435) has no bridge (shazzrah gate precedent — the explosion arm
-- is documented-only); C++ DoCastSelf(9438) default is non-triggered,
-- matching the ported cast.

local ENTRY_ARCANIST_DOAN = 6487

local SAY_AGGRO = 0
local SAY_SPECIALAE = 1

local SPELL_SILENCE = 8988
local SPELL_ARCANE_EXPLOSION = 9433
local SPELL_DETONATION = 9435
local SPELL_ARCANE_BUBBLE = 9438
local SPELL_POLYMORPH = 13323

local timers = {}
local bubbleLatched = {}

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

-- C++ SelectTarget(Random, 1, 30.0f, true): random alive player within
-- 30 yd, skipping position 0 (the current victim — majordomo
-- convention); alivePlayersInRange supplies the distance+alive legs
-- (opera convention).
local function randomAlivePlayerExcludingVictimInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local victimGUID = victim and victim:GetGUID() or 0
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and p:GetGUID() ~= victimGUID
                and creature:GetDistance(p) <= maxDist then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_SILENCE: non-triggered DoCastVictim(8988); re-arm
-- {15s,20s}.
local function onSilence(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SILENCE)
    end
    schedule(guid, "silence", math.random(15000, 20000), function()
        onSilence(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_EXPLOSION: non-triggered DoCastVictim(9433);
-- re-arm 8s.
local function onArcaneExplosion(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ARCANE_EXPLOSION)
    end
    schedule(guid, "explosion", 8000, function()
        onArcaneExplosion(creature, guid)
    end)
end

-- C++ EVENT_POLYMORPH: DoCast(13323) on a random alive player within
-- 30 yd skipping position 0; re-arm 20s unconditionally.
local function onPolymorph(creature, guid)
    local target = randomAlivePlayerExcludingVictimInRange(creature, 30)
    if target then
        creature:CastSpell(target, SPELL_POLYMORPH)
    end
    schedule(guid, "polymorph", 20000, function()
        onPolymorph(creature, guid)
    end)
end

local function doanResetState(guid)
    cancelTimers(guid)
    bubbleLatched[guid] = nil
end

local function doanEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    doanResetState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "silence", 15000, function()
        onSilence(creature, guid)
    end)
    schedule(guid, "explosion", 3000, function()
        onArcaneExplosion(creature, guid)
    end)
    schedule(guid, "polymorph", 30000, function()
        onPolymorph(creature, guid)
    end)
end

local function doanLeaveCombat(event, creature)
    doanResetState(creature:GetGUID())
end

local function doanDied(event, creature, killer)
    doanResetState(creature:GetGUID())
end

local function doanReset(event, creature)
    doanResetState(creature:GetGUID())
end

-- C++ UpdateAI below-50% arm: current health strictly below 50% and
-- not yet latched -> Talk(SAY_SPECIALAE) + non-triggered DoCastSelf
-- (arcane bubble 9438). Modeled on the pre-damage hook (event 9) with
-- current-health semantics (golemagg 704fb8e bug class: HealthBelowPct
-- reads current health, so no damage subtraction). The
-- DoCastAOE(detonation 9435) arm has no bridge (shazzrah gate
-- precedent) — documented-only.
local function doanDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if bubbleLatched[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() * 100 / maxHealth < 50 then
        bubbleLatched[guid] = true
        creature:Talk(SAY_SPECIALAE)
        creature:CastSpell(creature, SPELL_ARCANE_BUBBLE)
    end
end

RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 1, doanEnterCombat)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 2, doanLeaveCombat)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 4, doanDied)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 9, doanDamageTaken)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 23, doanReset)
