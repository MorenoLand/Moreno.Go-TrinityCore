-- Vectus (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_vectus.cpp
-- (boss_vectusAI only; CreatureScript name "boss_vectus"; EK loader
-- AddSC_boss_vectus decl :120 / call :298 follows theravenian
-- :119/:297). No NPC_ constant in scholomance.h —
-- RegisterLuaBoss binds the creature_template ScriptName
-- "boss_vectus" DB-side; entry wowhead-verified: 10432.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 9 DamageTaken (pre-damage hook), 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- fire shield 19626 at 2s / blast wave 16046 at 14s (both
-- non-triggered DoCast(me) -> creature:CastSpell(creature, spell),
-- jeklik convention); DamageTaken: post-damage health strictly
-- below 25% (HealthBelowPctDamaged(25, damage) — the damaged
-- class, damage IS subtracted, NOT the golemagg 704fb8e bug
-- class): NO once-guard in C++ — every damaging hit below 25%
-- re-fires: non-triggered DoCast(me, frenzy 8269) +
-- Talk(EMOTE_FRENZY 0) + ScheduleEvent(EVENT_FRENZY, 24s),
-- so the frenzy timer re-arms 24s on each sub-25% hit.
-- UpdateAI EVENT_FRENZY: non-triggered self-cast frenzy 8269 +
-- Talk(EMOTE_FRENZY 0), re-arm 24s. Zero Talk lines beyond
-- EMOTE_FRENZY.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue
-- gate and the post-event casting check have no UNIT_STATE
-- bridge — timers fire unconditionally (jeklik convention).
-- Documented-only (no bridges): GetScholomanceAI template
-- instance validation (luaBossAI shim); BossAI::JustEngagedWith
-- bookkeeping via luaBossAI shim (Vectus is not one of the 8
-- tracked Scholomance encounters — no DATA_ constant in
-- scholomance.h); SPELL_FLAMESTRIKE 18399 is declared but
-- UNUSED in C++ (dead enum entry, like gandling's DOMINATE).

local ENTRY_VECTUS = 10432

local SPELL_FLAMESTRIKE = 18399 -- unused in C++ (declared only)
local SPELL_BLAST_WAVE = 16046
local SPELL_FIRE_SHIELD = 19626
local SPELL_FRENZY = 8269

local EMOTE_FRENZY = 0

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

-- C++ DoCast(me, spell) non-triggered -> creature:CastSpell(creature, spell).
local function onSelfTimer(creature, guid, spell, key, rearm, fn)
    creature:CastSpell(creature, spell)
    schedule(guid, key, rearm, function()
        fn(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: non-triggered self-cast frenzy + Talk(EMOTE_FRENZY);
-- re-arms 24s. Also fired from DamageTaken below 25% (no once-guard,
-- each sub-25% hit re-arms the timer via schedule()).
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY)
    creature:Talk(EMOTE_FRENZY)
    schedule(guid, "frenzy", 24000, function()
        onFrenzy(creature, guid)
    end)
end

local function onFireShield(creature, guid)
    onSelfTimer(creature, guid, SPELL_FIRE_SHIELD, "fire_shield", 90000, onFireShield)
end

local function onBlastWave(creature, guid)
    onSelfTimer(creature, guid, SPELL_BLAST_WAVE, "blast_wave", 12000, onBlastWave)
end

local function vectusResetState(guid)
    cancelTimers(guid)
end

local function vectusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    vectusResetState(guid)
    schedule(guid, "fire_shield", 2000, function()
        onFireShield(creature, guid)
    end)
    schedule(guid, "blast_wave", 14000, function()
        onBlastWave(creature, guid)
    end)
end

local function vectusLeaveCombat(event, creature)
    vectusResetState(creature:GetGUID())
end

local function vectusDied(event, creature, killer)
    vectusResetState(creature:GetGUID())
end

local function vectusReset(event, creature)
    vectusResetState(creature:GetGUID())
end

-- C++ DamageTaken arm: no once-guard — fires on every DamageTaken
-- where post-damage health drops strictly below 25%
-- (HealthBelowPctDamaged(25, damage)). Modeled on the pre-damage
-- hook (event 9): non-triggered self-cast frenzy + Talk +
-- 24s schedule (schedule() replaces any pending frenzy timer,
-- matching C++ events.ScheduleEvent).
local function vectusDamageTaken(event, creature, attacker, damage)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 25 then
        onFrenzy(creature, creature:GetGUID())
    end
end

RegisterCreatureEvent(ENTRY_VECTUS, 1, vectusEnterCombat)
RegisterCreatureEvent(ENTRY_VECTUS, 2, vectusLeaveCombat)
RegisterCreatureEvent(ENTRY_VECTUS, 4, vectusDied)
RegisterCreatureEvent(ENTRY_VECTUS, 9, vectusDamageTaken)
RegisterCreatureEvent(ENTRY_VECTUS, 23, vectusReset)
