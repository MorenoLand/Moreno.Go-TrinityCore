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
-- threat-list player within 30 yd skipping the first entry in
-- MaxThreat order (UnitAI.h:162-184); a one-entry threat list casts
-- nothing but keeps the schedule; the 20s re-arm is unconditional,
-- matching C++ Repeat outside the if). The
-- UpdateAI below-50% latch (Talk(SAY_SPECIALAE 1) + non-triggered
-- DoCastSelf arcane bubble 9438, once via the _healthAbove50Pct bool) is
-- modeled on the pre-damage hook: (pre-damage health - incoming damage)
-- below the floored threshold (C++ HealthBelowPct(50) in UpdateAI reads
-- post-damage current health, boss_arcanist_doan.cpp:103 — so damage IS
-- subtracted, HealthBelowPctDamaged semantics, magmus precedent; the
-- golemagg 704fb8e no-subtraction rule applies only where C++ reads
-- pre-damage health in AI::DamageTaken) and not yet latched -> Talk(1)
-- + non-triggered self-cast 9438 + non-triggered DoCastAOE detonation
-- 9435 (self-cast lowering, selin_fireheart convention); per-GUID Lua
-- state holds the once-guard (C++-exact: the C++ arm uses the
-- _healthAbove50Pct bool, not an aura query — a HasAura re-cast
-- guard would wrongly re-arm the bubble on dispel), cleared on 2/4/23.
-- OnDied/OnLeaveCombat/OnReset: cancel timers, clear the latch.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers fire
-- unconditionally (jeklik convention); the below-50% latch fires on
-- the damage hook rather than the next UpdateAI tick (tick timing
-- deviation, documented); BossAI::JustEngagedWith and the
-- DATA_ARCANIST_DOAN encounter bookkeeping have no instance-script
-- bridge — boss admission via the luaBossAI shim; C++ DoCastSelf(9438)
-- default is non-triggered,
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

-- C++ SelectTarget(Random, 1, 30.0f, true): random alive player from
-- the threat list within 30 yd, popping the victim
-- (UnitAI.h:162-184 — the list starts with the current victim,
-- then the sorted threat list, and the first <offset> entries are
-- popped before the predicate filter; GetAITargets returns
-- threat-sorted order, so the victim exclusion below — not a
-- positional drop — is the pop, lua_creature_events.go:1912).
local function randomThreatPlayerSkippingFirstInRange(creature, maxDist)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local players = {}
    local aliveSeen = 0
    for _, t in ipairs(creature:GetAITargets()) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            players[#players + 1] = t
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

-- C++ EVENT_POLYMORPH: DoCast(13323) on a random alive threat-list
-- player within 30 yd skipping the first MaxThreat entry; re-arm
-- 20s unconditionally.
local function onPolymorph(creature, guid)
    local target = randomThreatPlayerSkippingFirstInRange(creature, 30)
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

-- C++ UpdateAI below-50% arm: post-damage current health strictly
-- below 50% (boss_arcanist_doan.cpp:103) and not yet latched ->
-- Talk(SAY_SPECIALAE) + non-triggered DoCastSelf (arcane bubble 9438).
-- Modeled on the pre-damage hook (event 9) as (health - damage) <
-- floor(max*50/100) — HealthBelowPctDamaged semantics (magmus
-- precedent); no-subtraction would fire up to one hit late. The
-- DoCastAOE(detonation 9435) arm rides the latch as non-triggered
-- self-cast (selin_fireheart DoCastAOE convention).
local function doanDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if bubbleLatched[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local health = creature:GetHealth()
    -- C++ reads HealthBelowPct(50) in UpdateAI (post-damage current
    -- health, boss_arcanist_doan.cpp:103), so the pre-damage hook
    -- subtracts the incoming damage (HealthBelowPctDamaged semantics,
    -- magmus precedent). The golemagg 704fb8e no-subtraction rule does
    -- NOT apply here: there C++ reads pre-damage health in AI::DamageTaken,
    -- here it reads post-damage health in UpdateAI.
    -- C++ reads post-damage health in UpdateAI, so the arm never
    -- fires on the killing blow (nefarian postDamage > 0 rule).
    if health - damage > 0 and health - damage < math.floor(maxHealth * 50 / 100) then
        bubbleLatched[guid] = true
        creature:Talk(SAY_SPECIALAE)
        creature:CastSpell(creature, SPELL_ARCANE_BUBBLE)
        -- C++ UpdateAI below-50% arm: non-triggered DoCastAOE(detonation
        -- 9435) rides the same latch after the bubble. 9435 carries no
        -- SpellScript, so the stale "no DoCastAOE bridge" claim was
        -- wrong — it lowers to non-triggered self-cast (selin_fireheart
        -- DoCastAOE convention).
        creature:CastSpell(creature, SPELL_DETONATION)
    end
end

RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 1, doanEnterCombat)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 2, doanLeaveCombat)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 4, doanDied)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 9, doanDamageTaken)
RegisterCreatureEvent(ENTRY_ARCANIST_DOAN, 23, doanReset)
