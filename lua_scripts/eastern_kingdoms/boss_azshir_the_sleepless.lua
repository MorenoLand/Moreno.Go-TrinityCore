-- Azshir the Sleepless (Scarlet Monastery, Graveyard wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_azshir_the_sleepless.cpp (boss_azshir_the_sleepless AI only);
-- scarlet_monastery.h names DATA_AZSHIR (:44) which the BossAI
-- constructor consumes. Azshir is a rare-spawn Graveyard boss (no Talk
-- lines at all in the C++ AI — no _SAY enum).
-- Creature entry: 6490 Azshir the Sleepless (no NPC_ constant exists in
-- the C++ tree — RegisterScarletMonasteryCreatureAI binds the
-- creature_template ScriptName DB-side; entry wowhead-cited:
-- classic.wowhead.com/npc=6490/azshir-the-sleepless).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat: call
-- of the grave 17831 30s->30s / terrify 7399 20s->20s (both
-- non-triggered DoCastVictim -> GetVictim + CastSpell; nil-victim
-- ticks cast nothing but keep the schedule — jeklik convention). The
-- DamageTaken arm fires once when post-damage health drops strictly
-- below 50% (C++ HealthBelowPctDamaged(50, damage) — damaged class,
-- NOT the golemagg 704fb8e current-health bug class): non-triggered
-- DoCastVictim(soul siphon 7290) + 20s re-arming EVENT_SOUL_SIPHON
-- timer; per-GUID Lua state holds the once-guard (no HasAura bridge),
-- cleared on 2/4/23. OnDied/OnLeaveCombat/OnReset: cancel timers,
-- clear the latch.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers
-- fire unconditionally (jeklik convention); the DamageTaken latch
-- fires on the damage hook rather than the script's UpdateAI tick
-- (timing documented); BossAI::JustEngagedWith and the DATA_AZSHIR
-- encounter bookkeeping have no instance-script bridge — boss
-- admission via the luaBossAI shim.

local ENTRY_AZSHIR_THE_SLEEPLESS = 6490

local SPELL_CALL_OF_THE_GRAVE = 17831
local SPELL_TERRIFY = 7399
local SPELL_SOUL_SIPHON = 7290

local timers = {}
local siphonLatched = {}

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

-- C++ EVENT_CALL_OF_GRAVE: non-triggered DoCastVictim(17831);
-- re-arm 30s.
local function onCallOfGrave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CALL_OF_THE_GRAVE)
    end
    schedule(guid, "call_of_grave", 30000, function()
        onCallOfGrave(creature, guid)
    end)
end

-- C++ EVENT_TERRIFY: non-triggered DoCastVictim(7399); re-arm 20s.
local function onTerrify(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TERRIFY)
    end
    schedule(guid, "terrify", 20000, function()
        onTerrify(creature, guid)
    end)
end

-- C++ EVENT_SOUL_SIPHON: non-triggered DoCastVictim(7290); re-arm
-- 20s.
local function onSoulSiphon(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SOUL_SIPHON)
    end
    schedule(guid, "soul_siphon", 20000, function()
        onSoulSiphon(creature, guid)
    end)
end

local function azshirResetState(guid)
    cancelTimers(guid)
    siphonLatched[guid] = nil
end

local function azshirEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    azshirResetState(guid)
    schedule(guid, "call_of_grave", 30000, function()
        onCallOfGrave(creature, guid)
    end)
    schedule(guid, "terrify", 20000, function()
        onTerrify(creature, guid)
    end)
end

local function azshirLeaveCombat(event, creature)
    azshirResetState(creature:GetGUID())
end

local function azshirDied(event, creature, killer)
    azshirResetState(creature:GetGUID())
end

local function azshirReset(event, creature)
    azshirResetState(creature:GetGUID())
end

-- C++ DamageTaken arm: once, when post-damage health drops strictly
-- below 50% (HealthBelowPctDamaged(50, damage) — the damaged class, so
-- the incoming damage IS subtracted here), non-triggered
-- DoCastVictim(soul siphon 7290) + 20s re-arming timer. Modeled on
-- the pre-damage hook (event 9).
local function azshirDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if siphonLatched[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 50 then
        siphonLatched[guid] = true
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_SOUL_SIPHON)
        end
        schedule(guid, "soul_siphon", 20000, function()
            onSoulSiphon(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_AZSHIR_THE_SLEEPLESS, 1, azshirEnterCombat)
RegisterCreatureEvent(ENTRY_AZSHIR_THE_SLEEPLESS, 2, azshirLeaveCombat)
RegisterCreatureEvent(ENTRY_AZSHIR_THE_SLEEPLESS, 4, azshirDied)
RegisterCreatureEvent(ENTRY_AZSHIR_THE_SLEEPLESS, 9, azshirDamageTaken)
RegisterCreatureEvent(ENTRY_AZSHIR_THE_SLEEPLESS, 23, azshirReset)
