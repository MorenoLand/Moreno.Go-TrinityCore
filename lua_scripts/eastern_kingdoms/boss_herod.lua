-- Herod (Scarlet Monastery, Armory wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_herod.cpp (boss_herod AI only);
-- scarlet_monastery.h names DATA_HEROD (:40) which the BossAI
-- constructor consumes.
-- Creature entry: 3975 Herod (no NPC_ constant exists in the C++
-- tree — RegisterScarletMonasteryCreatureAI binds the
-- creature_template ScriptName DB-side; entry wowhead-cited:
-- www.wowhead.com/cata/npc=3975/herod).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat
-- Talk(SAY_AGGRO 0) + non-triggered self-cast rushing charge 8260
-- (DoCast(me, ...) -> creature:CastSpell(creature, ...), same
-- non-triggered semantics as thalnos) + cleave 15496 on a 12s->12s
-- timer + whirlwind 8989 on a 60s->30s timer (both non-triggered
-- DoCastVictim -> GetVictim + CastSpell; nil-victim ticks cast
-- nothing but keep the schedule — jeklik convention). Whirlwind's
-- first cast also fires Talk(SAY_WHIRLWIND 1).
-- KilledUnit: Talk(SAY_KILL 3) only when the victim is a player
-- (C++ TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
-- The DamageTaken enrage latch fires once when post-damage health
-- drops strictly below 30% (C++ HealthBelowPctDamaged(30, damage) —
-- damaged class, the same class as the thalnos/azshir latches, NOT
-- the golemagg 704fb8e current-health bug class): Talk(EMOTE_ENRAGE
-- 4) + Talk(SAY_ENRAGE 2) + non-triggered self-cast frenzy 8269;
-- per-GUID Lua state holds the once-guard, cleared on 2/4/23.
-- OnDied/OnLeaveCombat/OnReset: cancel timers, clear the latch.
-- Deviations from C++: JustDied's 20x SummonCreature(NPC_SCARLET_
-- TRAINEE 6575, timed-or-dead 10min around ScarletTraineePos
-- 1939.18/-431.58/17.09/6.22) and the npc_scarlet_trainee EscortAI
-- (urand 1-6s start timer -> Start(true, true)) are documented-only
-- — no summon or EscortAI bridges (nightbane/npc_barnes precedent).
-- The UpdateAI UNIT_STATE_CASTING queue gate has no UNIT_STATE
-- bridge — timers fire unconditionally (jeklik convention); the
-- DamageTaken latch fires on the damage hook rather than the
-- script's UpdateAI tick (timing documented); BossAI::JustEngagedWith
-- and the DATA_HEROD encounter bookkeeping have no instance-script
-- bridge — boss admission via the luaBossAI shim.

local ENTRY_HEROD = 3975

local SAY_AGGRO = 0
local SAY_WHIRLWIND = 1
local SAY_ENRAGE = 2
local SAY_KILL = 3
local EMOTE_ENRAGE = 4

local SPELL_RUSHINGCHARGE = 8260
local SPELL_CLEAVE = 15496
local SPELL_WHIRLWIND = 8989
local SPELL_FRENZY = 8269

local timers = {}
local enrageDone = {}

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

-- C++ EVENT_CLEAVE: non-triggered DoCastVictim(15496); Repeat(12s).
local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 12000, function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_WHIRLWIND: Talk(SAY_WHIRLWIND 1) + non-triggered
-- DoCastVictim(8989); Repeat(30s) after a 60s first timer.
local function onWhirlwind(creature, guid)
    local victim = creature:GetVictim()
    creature:Talk(SAY_WHIRLWIND)
    if victim then
        creature:CastSpell(victim, SPELL_WHIRLWIND)
    end
    schedule(guid, "whirlwind", 30000, function()
        onWhirlwind(creature, guid)
    end)
end

local function herodResetState(guid)
    cancelTimers(guid)
    enrageDone[guid] = nil
end

local function herodEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    herodResetState(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ DoCast(me, SPELL_RUSHINGCHARGE) — non-triggered self cast.
    creature:CastSpell(creature, SPELL_RUSHINGCHARGE)
    schedule(guid, "cleave", 12000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "whirlwind", 60000, function()
        onWhirlwind(creature, guid)
    end)
end

local function herodLeaveCombat(event, creature)
    herodResetState(creature:GetGUID())
end

local function herodDied(event, creature, killer)
    herodResetState(creature:GetGUID())
end

local function herodReset(event, creature)
    herodResetState(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_KILL 3) only when the victim is a player
-- (TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
local function herodTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_KILL)
    end
end

-- C++ DamageTaken enrage latch: once, when post-damage health drops
-- strictly below 30% (HealthBelowPctDamaged(30, damage) — the damaged
-- class, so the incoming damage IS subtracted here):
-- Talk(EMOTE_ENRAGE 4) + Talk(SAY_ENRAGE 2) + non-triggered
-- self-cast frenzy 8269. Modeled on the pre-damage hook (event 9).
local function herodDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if enrageDone[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 30 then
        enrageDone[guid] = true
        creature:Talk(EMOTE_ENRAGE)
        creature:Talk(SAY_ENRAGE)
        creature:CastSpell(creature, SPELL_FRENZY)
    end
end

RegisterCreatureEvent(ENTRY_HEROD, 1, herodEnterCombat)
RegisterCreatureEvent(ENTRY_HEROD, 2, herodLeaveCombat)
RegisterCreatureEvent(ENTRY_HEROD, 3, herodTargetDied)
RegisterCreatureEvent(ENTRY_HEROD, 4, herodDied)
RegisterCreatureEvent(ENTRY_HEROD, 9, herodDamageTaken)
RegisterCreatureEvent(ENTRY_HEROD, 23, herodReset)
