-- Kormok (Scholomance) — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/boss_kormok.cpp
-- (boss_kormok AI only). Summoned boss for the Lord Valthalak's Amulet
-- (Tier 0.5) questline; not one of the 8 encounters tracked in
-- scholomance.h (no DATA_ entry — no encounter-data bookkeeping in
-- C++). No NPC_ constant in scholomance.h — RegisterLuaBoss binds the
-- creature_template ScriptName "boss_kormok" DB-side; entry
-- wotlk.cavernoftime-verified: 16118 (level 60 elite, Scholomance).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- shadowbolt volley 20741 (non-triggered DoCastVictim, 10s->15s) /
-- bone shield 27688 (non-triggered DoCastVictim — the C++ casts it on
-- the victim, faithfully mirrored — 2s->45s) / summon bone minions
-- 27687 (non-triggered DoCast self, 15s->12s); nil-victim ticks cast
-- nothing but keep the schedule (jeklik convention). The DamageTaken
-- arm fires once when post-damage health drops strictly below 25%
-- (C++ HealthBelowPctDamaged(25, damage) — damaged class, NOT the
-- golemagg 704fb8e current-health bug class): non-triggered
-- DoCast(self, summon bone mages 27695); per-GUID Lua once-guard
-- (C++ bool Mages, cleared on 2/4/23). Zero Talk lines in C++ (no
-- _SAY enum).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate and
-- the post-event casting check have no UNIT_STATE bridge — timers
-- fire unconditionally (jeklik convention); the DamageTaken latch
-- fires on the damage hook rather than the script's UpdateAI tick
-- (timing documented). Documented-only (no bridges): JustSummoned
-- AttackStart(victim) on the adds (no summoned-attack-start bridge —
-- herod precedent); spell_kormok_summon_bone_mages 27695 script
-- effect (2x random of the 4 bone-mage summon spells
-- 27696/27697/27698/27699); spell_kormok_summon_bone_minions 27687
-- script effect (4x SummonCreature(NPC_BONE_MINION 16119,
-- scholomance.h:43, ±7yd, 2min corpse despawn)) — no SpellScript
-- bridge (nightbane precedent).

local ENTRY_KORMOK = 16118

local SPELL_SHADOWBOLT_VOLLEY = 20741
local SPELL_BONE_SHIELD = 27688
local SPELL_SUMMON_BONE_MINIONS = 27687
local SPELL_SUMMON_BONE_MAGES = 27695

local timers = {}
local magesLatched = {}

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

-- C++ EVENT_SHADOWBOLT_VOLLEY: non-triggered DoCastVictim(20741);
-- re-arm 15s.
local function onShadowboltVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWBOLT_VOLLEY)
    end
    schedule(guid, "shadowbolt_volley", 15000, function()
        onShadowboltVolley(creature, guid)
    end)
end

-- C++ EVENT_BONE_SHIELD: non-triggered DoCastVictim(27688); re-arm
-- 45s.
local function onBoneShield(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BONE_SHIELD)
    end
    schedule(guid, "bone_shield", 45000, function()
        onBoneShield(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_MINIONS: non-triggered DoCast(27687); re-arm 12s.
-- The SpellScript effect (summoning the bone minions) has no
-- SpellScript bridge, so the spell itself is cast on self.
local function onSummonBoneMinions(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_BONE_MINIONS)
    schedule(guid, "summon_bone_minions", 12000, function()
        onSummonBoneMinions(creature, guid)
    end)
end

local function kormokResetState(guid)
    cancelTimers(guid)
    magesLatched[guid] = nil
end

local function kormokEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    kormokResetState(guid)
    schedule(guid, "shadowbolt_volley", 10000, function()
        onShadowboltVolley(creature, guid)
    end)
    schedule(guid, "bone_shield", 2000, function()
        onBoneShield(creature, guid)
    end)
    schedule(guid, "summon_bone_minions", 15000, function()
        onSummonBoneMinions(creature, guid)
    end)
end

local function kormokLeaveCombat(event, creature)
    kormokResetState(creature:GetGUID())
end

local function kormokDied(event, creature, killer)
    kormokResetState(creature:GetGUID())
end

local function kormokReset(event, creature)
    kormokResetState(creature:GetGUID())
end

-- C++ DamageTaken arm: once, when post-damage health drops strictly
-- below 25% (HealthBelowPctDamaged(25, damage) — the damaged class,
-- so the incoming damage IS subtracted here), non-triggered
-- DoCast(me, summon bone mages 27695). Modeled on the pre-damage
-- hook (event 9) with a per-GUID once-guard (C++ bool Mages).
local function kormokDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if magesLatched[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 25 then
        magesLatched[guid] = true
        creature:CastSpell(creature, SPELL_SUMMON_BONE_MAGES)
    end
end

RegisterCreatureEvent(ENTRY_KORMOK, 1, kormokEnterCombat)
RegisterCreatureEvent(ENTRY_KORMOK, 2, kormokLeaveCombat)
RegisterCreatureEvent(ENTRY_KORMOK, 4, kormokDied)
RegisterCreatureEvent(ENTRY_KORMOK, 9, kormokDamageTaken)
RegisterCreatureEvent(ENTRY_KORMOK, 23, kormokReset)
