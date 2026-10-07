-- High Inquisitor Fairbanks (Scarlet Monastery, Cathedral wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_high_inquisitor_fairbanks.cpp; scarlet_monastery.h names
-- DATA_HIGH_INQUISITOR_FAIRBANKS (index 5) which the BossAI
-- constructor consumes.
-- Creature entry: 4542 High Inquisitor Fairbanks (no NPC_ constant
-- exists in the C++ tree — RegisterScarletMonasteryCreatureAI binds
-- the creature_template ScriptName DB-side; entry wowhead-cited:
-- www.wowhead.com/classic/npc=4542/high-inquisitor-fairbanks).
-- (The adjacent 3974 guess was disproven by the wowhead classic
-- entry before this port was written.)
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady. C++ has zero Talk lines
-- (no _SAY enum) — Fairbanks casts silently.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat arms
-- curse of blood 8282 10s->25s (non-triggered DoCastVictim ->
-- GetVictim + CastSpell; nil-victim ticks cast nothing but keep the
-- schedule — jeklik convention) + dispel magic 15090 30s->30s +
-- fear 12096 40s->40s + sleep 8399 25s->30s.
-- C++ EVENT_DIPEL_MAGIC selects via
-- HighInquisitorFairbanksDispelMagicTargetSelector: TYPEID_PLAYER
-- within 30 yd of the boss WITH a dispellable DISPEL_MAGIC aura —
-- the aura leg has no bridge (shazzrah curse precedent), so the Lua
-- picks a random alive player within 30 yd and skips the cast when
-- none is in range (C++ DoCast is likewise conditional on the
-- selector finding a target; the 30s repeat is unconditional).
-- C++ EVENT_FEAR: SelectTarget(Random, 1, 20.f, true) — random alive
-- player within 20 yd skipping position 0 — modeled with the
-- majordomo victim-excluding helper restricted to 20 yd; a
-- sole-victim tick casts nothing but keeps the 40s schedule.
-- C++ EVENT_SLEEP: SelectTarget(MaxThreat, 0, 30.f, true, false) —
-- top-threat player within 30 yd, approximated by the current
-- victim (jeklik convention); nil-victim ticks keep the 30s
-- schedule.
-- The DamageTaken latch fires when post-damage health drops strictly
-- below 25% (C++ HealthBelowPctDamaged(25, damage) — the damaged
-- class, the same class as the thalnos/azshir latches, NOT the
-- golemagg 704fb8e current-health bug class): a per-GUID once-guard
-- self-casts power word: shield 11647 once per combat, and a 30s
-- heal cooldown gates the self-cast heal 12039 (C++ _healTimer:
-- Reset(0s) in Reset means the heal is available on the first
-- sub-25% dip, then locked for 30s — modeled by clearing the
-- cooldown via a one-shot timer; per-GUID state is cleared on
-- 2/4/23). C++ also gates the heal on !IsNonMeleeSpellCast(false);
-- the Lua has no casting-state bridge, so the heal fires
-- unconditionally (UpdateAI UNIT_STATE_CASTING-gate precedent).
-- The latch is modeled on the pre-damage hook (event 9) rather than
-- the DamageTaken call inside DealDamage (documented timing
-- deviation — the engine hook is the only DamageTaken-visible leg).
-- Deviations from C++: SetStandState(UNIT_STAND_STATE_DEAD) in Reset
-- and SetStandState(UNIT_STAND_STATE_STAND) in JustEngagedWith
-- (the secret-chamber corpse visual) have no stand-state bridge
-- (npc_barnes precedent) — documented-only; BossAI::JustEngagedWith
-- and the DATA_HIGH_INQUISITOR_FAIRBANKS encounter bookkeeping have
-- no instance-script bridge — boss admission via the luaBossAI shim.

local ENTRY_HIGH_INQUISITOR_FAIRBANKS = 4542

local SPELL_CURSEOFBLOOD = 8282
local SPELL_DISPEL_MAGIC = 15090
local SPELL_FEAR = 12096
local SPELL_HEAL = 12039
local SPELL_POWERWORDSHIELD = 11647
local SPELL_SLEEP = 8399

local HEAL_COOLDOWN_MS = 30000

local timers = {}
local shieldDone = {}
local healOnCooldown = {}

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

-- Random alive player in the same map+instance within maxDist of the
-- creature, optionally excluding the given GUID (C++ SelectTarget
-- Random-position / in-range legs).
local function randomAlivePlayerInRange(creature, maxDist, excludeGUID)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local players = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist
                and (not excludeGUID or p:GetGUID() ~= excludeGUID) then
            players[#players + 1] = p
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_CURSE_BLOOD: non-triggered DoCastVictim(8282);
-- Repeat(25s).
local function onCurseBlood(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSEOFBLOOD)
    end
    schedule(guid, "curse", 25000, function()
        onCurseBlood(creature, guid)
    end)
end

-- C++ EVENT_DIPEL_MAGIC: DoCast(pick, 15090) where the pick is the
-- custom selector; nil pick casts nothing. Repeat(30s) regardless.
local function onDispelMagic(creature, guid)
    local target = randomAlivePlayerInRange(creature, 30, nil)
    if target then
        creature:CastSpell(target, SPELL_DISPEL_MAGIC)
    end
    schedule(guid, "dispel", 30000, function()
        onDispelMagic(creature, guid)
    end)
end

-- C++ EVENT_FEAR: DoCast(pick, 12096) on a random alive player within
-- 20 yd excluding the victim; nil pick casts nothing. Repeat(40s)
-- regardless.
local function onFear(creature, guid)
    local victim = creature:GetVictim()
    local target = randomAlivePlayerInRange(creature, 20,
        victim and victim:GetGUID() or nil)
    if target then
        creature:CastSpell(target, SPELL_FEAR)
    end
    schedule(guid, "fear", 40000, function()
        onFear(creature, guid)
    end)
end

-- C++ EVENT_SLEEP: non-triggered DoCast(8399) on the top-threat
-- target, approximated by the current victim; nil-victim ticks keep
-- the schedule. Repeat(30s).
local function onSleep(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SLEEP)
    end
    schedule(guid, "sleep", 30000, function()
        onSleep(creature, guid)
    end)
end

local function fairbanksResetState(guid)
    cancelTimers(guid)
    shieldDone[guid] = nil
    healOnCooldown[guid] = nil
end

local function fairbanksEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    fairbanksResetState(guid)
    schedule(guid, "curse", 10000, function()
        onCurseBlood(creature, guid)
    end)
    schedule(guid, "dispel", 30000, function()
        onDispelMagic(creature, guid)
    end)
    schedule(guid, "fear", 40000, function()
        onFear(creature, guid)
    end)
    schedule(guid, "sleep", 25000, function()
        onSleep(creature, guid)
    end)
end

local function fairbanksLeaveCombat(event, creature)
    fairbanksResetState(creature:GetGUID())
end

local function fairbanksDied(event, creature, killer)
    fairbanksResetState(creature:GetGUID())
end

local function fairbanksReset(event, creature)
    fairbanksResetState(creature:GetGUID())
end

-- C++ DamageTaken latch (damaged class): when post-damage health is
-- strictly below 25%, self-cast power word: shield 11647 once per
-- combat, then self-cast heal 12039 if the 30s heal cooldown has
-- passed (C++ _healTimer starts reset(0s), so the first dip always
-- heals). The !IsNonMeleeSpellCast(false) gate has no bridge.
local function fairbanksDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth >= 25 then
        return
    end
    if not shieldDone[guid] then
        shieldDone[guid] = true
        creature:CastSpell(creature, SPELL_POWERWORDSHIELD)
    end
    if not healOnCooldown[guid] then
        healOnCooldown[guid] = true
        schedule(guid, "healCooldown", HEAL_COOLDOWN_MS, function()
            healOnCooldown[guid] = nil
        end)
        creature:CastSpell(creature, SPELL_HEAL)
    end
end

RegisterCreatureEvent(ENTRY_HIGH_INQUISITOR_FAIRBANKS, 1, fairbanksEnterCombat)
RegisterCreatureEvent(ENTRY_HIGH_INQUISITOR_FAIRBANKS, 2, fairbanksLeaveCombat)
RegisterCreatureEvent(ENTRY_HIGH_INQUISITOR_FAIRBANKS, 4, fairbanksDied)
RegisterCreatureEvent(ENTRY_HIGH_INQUISITOR_FAIRBANKS, 9, fairbanksDamageTaken)
RegisterCreatureEvent(ENTRY_HIGH_INQUISITOR_FAIRBANKS, 23, fairbanksReset)
