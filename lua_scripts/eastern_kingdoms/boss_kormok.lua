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
-- PORTED: the UpdateAI UNIT_STATE_CASTING queue gate
-- (creature:IsCasting is live - lua_creature_events.go CastingUntil
-- window; due arms defer 500ms, broodkeeper precedent). The stale
-- 'constant-false' claim is corrected; the DamageTaken latch
-- fires on the damage hook rather than the script's UpdateAI tick
-- (timing documented). Ported: the bone-mage script effect (2x
-- random of 27696/27697/27698/27699, triggered, in
-- kormokDamageTaken) and the bone-minion script effect (4x
-- SummonCreature 16119, ±7yd, 2min corpse despawn, in
-- onSummonBoneMinions); the ungated JustSummoned arm
-- (boss_kormok.cpp:75-78, summoned attacks Kormok's victim) is
-- ported on event 19 now that the minions arrive via a Lua summon.

local ENTRY_KORMOK = 16118
local NPC_BONE_MINION = 16119

local SPELL_SHADOWBOLT_VOLLEY = 20741
local SPELL_BONE_SHIELD = 27688
local SPELL_SUMMON_BONE_MINIONS = 27687
local SPELL_SUMMON_BONE_MAGES = 27695
local SUMMON_BONE_MAGE_SPELLS = { 27696, 27697, 27698, 27699 }

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
    if creature:IsCasting() then
        schedule(guid, "shadowbolt_volley", 500, function()
            onShadowboltVolley(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "bone_shield", 500, function()
            onBoneShield(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BONE_SHIELD)
    end
    schedule(guid, "bone_shield", 45000, function()
        onBoneShield(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_MINIONS: non-triggered DoCast(27687); re-arm 12s.
-- The minion SpellScript (boss_kormok.cpp:180-211) summons 4x bone
-- minion 16119 at caster ±7yd, TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN,
-- 2min — modeled directly (SummonCreature type 1, 120000ms;
-- orientation 0 falls back to the summoner's orientation).
local function onSummonBoneMinions(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "summon_bone_minions", 500, function()
            onSummonBoneMinions(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_BONE_MINIONS)
    for _ = 1, 4 do
        creature:SummonCreature(NPC_BONE_MINION,
            creature:GetX() + math.random(-7, 7),
            creature:GetY() + math.random(-7, 7),
            creature:GetZ(), 0, 1, 120000)
    end
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
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 25 / 100) then
        magesLatched[guid] = true
        creature:CastSpell(creature, SPELL_SUMMON_BONE_MAGES)
        -- The bone-mage SpellScript (boss_kormok.cpp:146-177) casts
        -- 2x a random of the 4 bone-mage summon spells, triggered,
        -- on the caster — modeled directly (urand(0,3) twice,
        -- duplicates allowed).
        for _ = 1, 2 do
            creature:CastSpell(creature, SUMMON_BONE_MAGE_SPELLS[math.random(1, 4)], true)
        end
    end
end

-- C++ JustSummoned (boss_kormok.cpp:75-78), ungated: the summoned
-- creature attacks Kormok's victim. Previously documented-only for
-- lack of a bearer (adds arrived via SpellScript effects); now
-- that the minions arrive via a Lua summon, event 19 fires.
local function kormokJustSummoned(event, creature, summoned)
    local victim = creature:GetVictim()
    if victim ~= nil then
        summoned:EngageWithTarget(victim)
    end
end

RegisterCreatureEvent(ENTRY_KORMOK, 1, kormokEnterCombat)
RegisterCreatureEvent(ENTRY_KORMOK, 2, kormokLeaveCombat)
RegisterCreatureEvent(ENTRY_KORMOK, 4, kormokDied)
RegisterCreatureEvent(ENTRY_KORMOK, 9, kormokDamageTaken)
RegisterCreatureEvent(ENTRY_KORMOK, 19, kormokJustSummoned)
RegisterCreatureEvent(ENTRY_KORMOK, 23, kormokReset)
