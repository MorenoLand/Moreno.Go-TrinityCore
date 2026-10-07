-- Nerub'enkan (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/boss_nerubenkan.cpp
-- (boss_nerubenkanAI : public ScriptedAI via GetStratholmeAI; registered
-- by AddSC_boss_nerubenkan in eastern_kingdoms_script_loader.cpp
-- (declaration line 128, call line 306; follows maleki :127/305)).
-- The file holds 1 script: boss_nerubenkan (pure timer-driven boss AI —
-- no Talk lines, no gossip/quest/vehicle arms, no SpellScript/AuraScript
-- loaders; C++ JustEngagedWith is empty).
-- Entry: no NPC_ constant in stratholme.h (DB-side ScriptName binding) —
-- 10437 independently cited (classic.wowhead.com/npc=10437/nerubenkan).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnEnterCombat: C++ Initialize()/Reset() — ENCASINGWEBS 7000ms,
--   PIERCEARMOR 19000ms, CRYPTSCARABS 3000ms, RAISEUNDEADSCARAB 3000ms.
-- - EVENT_ENCASINGWEBS: DoCastVictim(SPELL_ENCASINGWEBS 4962)
--   non-triggered -> GetVictim + CastSpell (jeklik convention,
--   nil-victim keeps schedule); 30000ms re-arm.
-- - EVENT_PIERCEARMOR: C++ `if (urand(0, 3) < 2)` (50% cast chance)
--   then DoCastVictim(SPELL_PIERCEARMOR 6016); re-arm is unconditional
--   35000ms either way.
-- - EVENT_CRYPTSCARABS: DoCastVictim(SPELL_CRYPT_SCARABS 31602);
--   20000ms re-arm.
-- - EVENT_RAISEUNDEADSCARAB: the timer shape is preserved (3000ms then
--   16000ms re-arm); the legs are documented-only — C++
--   RaiseUndeadScarab(me->GetVictim()) does DoSpawnCreature(10876,
--   irand(-9,9), irand(-9,9), 0, 0, TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN,
--   180s) + pUndeadScarab->AI()->AttackStart(victim) with no summon
--   bridge (herod precedent).
-- - OnDied: timer clear modeled; the C++ JustDied leg
--   `instance->SetData(TYPE_NERUB 3, IN_PROGRESS)` is documented-only
--   (no instance-script bridge — shadowfang keep precedent). The C++
--   sets IN_PROGRESS rather than DONE on death (70%-complete file
--   quirk); documented as-is.
-- Unmodeled (documented-only, no bridges):
-- - UpdateAI's `if (!UpdateVictim()) return` gate: engine-driven.
-- - GetStratholmeAI wrapper: plain AI factory template, no logic.
-- Verifiable numbers (file's own enums): SPELL_ENCASINGWEBS = 4962,
-- SPELL_PIERCEARMOR = 6016, SPELL_CRYPT_SCARABS = 31602,
-- SPELL_RAISEUNDEADSCARAB = 17235 (declared, used only as a comment
-- label in C++; the actual summon is spell-less DoSpawnCreature 10876);
-- TYPE_NERUB = 3 (stratholme.h :30).

local ENTRY_NERUBENKAN = 10437

local SPELL_ENCASINGWEBS = 4962
local SPELL_PIERCEARMOR = 6016
local SPELL_CRYPT_SCARABS = 31602

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

-- C++ DoCastVictim(SPELL, false) -> GetVictim + CastSpell, non-triggered
-- (jeklik convention); nil victim keeps the schedule.
local function doCastVictim(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell)
    end
end

-- C++ `if (urand(0, 3) < 2)` — 50% cast chance; the 35000ms re-arm is
-- unconditional and runs either way.
local function pierceArmorGate()
    return math.random(0, 3) < 2
end

-- C++ EncasingWebs: DoCastVictim(SPELL_ENCASINGWEBS); 30000ms re-arm.
local function onEncasingWebs(creature, guid)
    doCastVictim(creature, SPELL_ENCASINGWEBS)
    schedule(guid, "encasing_webs", 30000, function() onEncasingWebs(creature, guid) end)
end

-- C++ PierceArmor: gate, then DoCastVictim(SPELL_PIERCEARMOR);
-- 35000ms re-arm unconditional.
local function onPierceArmor(creature, guid)
    if pierceArmorGate() then
        doCastVictim(creature, SPELL_PIERCEARMOR)
    end
    schedule(guid, "pierce_armor", 35000, function() onPierceArmor(creature, guid) end)
end

-- C++ CryptScarabs: DoCastVictim(SPELL_CRYPT_SCARABS); 20000ms re-arm.
local function onCryptScarabs(creature, guid)
    doCastVictim(creature, SPELL_CRYPT_SCARABS)
    schedule(guid, "crypt_scarabs", 20000, function() onCryptScarabs(creature, guid) end)
end

-- C++ RaiseUndeadScarab timer (3000ms then 16000ms): DoSpawnCreature
-- 10876 + AttackStart legs have no summon bridge — timer shape
-- preserved, legs documented-only (jandice set_visibility precedent).
local function onRaiseUndeadScarab(creature, guid)
    schedule(guid, "raise_undead_scarab", 16000, function() onRaiseUndeadScarab(creature, guid) end)
end

-- C++ Initialize()/Reset(): ENCASINGWEBS 7000ms, PIERCEARMOR 19000ms,
-- CRYPTSCARABS 3000ms, RAISEUNDEADSCARAB 3000ms. JustEngagedWith is
-- empty in C++ — no Talk, no immediate casts.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "encasing_webs", 7000, function() onEncasingWebs(creature, guid) end)
    schedule(guid, "pierce_armor", 19000, function() onPierceArmor(creature, guid) end)
    schedule(guid, "crypt_scarabs", 3000, function() onCryptScarabs(creature, guid) end)
    schedule(guid, "raise_undead_scarab", 3000, function() onRaiseUndeadScarab(creature, guid) end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_NERUBENKAN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_NERUBENKAN, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_NERUBENKAN, 4, onDied)
RegisterCreatureEvent(ENTRY_NERUBENKAN, 23, onReset)
