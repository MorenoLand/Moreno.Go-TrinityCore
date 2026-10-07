-- Bloodmage Thalnos (Scarlet Monastery, Graveyard wing) — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletMonastery/
-- boss_bloodmage_thalnos.cpp (boss_bloodmage_thalnos AI only);
-- scarlet_monastery.h names DATA_BLOODMAGE_THALNOS (:37) which the BossAI
-- constructor consumes.
-- Creature entry: 4543 Bloodmage Thalnos (no NPC_ constant exists in
-- the C++ tree — RegisterScarletMonasteryCreatureAI binds the
-- creature_template ScriptName DB-side; entry wowhead-cited:
-- www.wowhead.com/cata/npc=4543/bloodmage-thalnos).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (pre-damage hook), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat
-- Talk(SAY_AGGRO 0) + flame shock 8053 10s->{10s,15s} / shadow bolt
-- 1106 2s->2s / flame spike 8814 8s->30s / fire nova 16079 40s->40s
-- (all non-triggered DoCastVictim -> GetVictim + CastSpell; nil-victim
-- ticks cast nothing but keep the schedule — jeklik convention).
-- KilledUnit: Talk(SAY_KILL 2) only when the victim is a player
-- (C++ TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
-- The DamageTaken arm fires once when post-damage health drops strictly
-- below 35% (C++ HealthBelowPctDamaged(35, damage) — damaged class,
-- NOT the golemagg 704fb8e current-health bug class): Talk(SAY_HEALTH
-- 1); per-GUID Lua state holds the once-guard, cleared on 2/4/23. No
-- spell cast in the latch — the C++ arm only talks. OnDied/OnLeaveCombat/
-- OnReset: cancel timers, clear the latch.
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate has no
-- UNIT_STATE bridge — timers fire unconditionally (jeklik convention);
-- the DamageTaken latch fires on the damage hook rather than the
-- script's UpdateAI tick (timing documented); BossAI::JustEngagedWith
-- and the DATA_BLOODMAGE_THALNOS encounter bookkeeping have no
-- instance-script bridge — boss admission via the luaBossAI shim.
-- (Wowpedia lists the health yell at 50% and the Sunwell bug tracker
-- reports extra spell arms, but the C++ source — authoritative here —
-- is a 35% latch and the four timers above.)

local ENTRY_BLOODMAGE_THALNOS = 4543

local SAY_AGGRO = 0
local SAY_HEALTH = 1
local SAY_KILL = 2

local SPELL_FLAME_SHOCK = 8053
local SPELL_SHADOW_BOLT = 1106
local SPELL_FLAME_SPIKE = 8814
local SPELL_FIRE_NOVA = 16079

local timers = {}
local healthYellDone = {}

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

-- C++ EVENT_FLAME_SHOCK: non-triggered DoCastVictim(8053);
-- re-arm {10s,15s} (C++ Repeat inclusive).
local function onFlameShock(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAME_SHOCK)
    end
    schedule(guid, "flame_shock", math.random(10000, 15000), function()
        onFlameShock(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BOLT: non-triggered DoCastVictim(1106); re-arm 2s.
local function onShadowBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_BOLT)
    end
    schedule(guid, "shadow_bolt", 2000, function()
        onShadowBolt(creature, guid)
    end)
end

-- C++ EVENT_FLAME_SPIKE: non-triggered DoCastVictim(8814); re-arm 30s.
local function onFlameSpike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FLAME_SPIKE)
    end
    schedule(guid, "flame_spike", 30000, function()
        onFlameSpike(creature, guid)
    end)
end

-- C++ EVENT_FIRE_NOVA: non-triggered DoCastVictim(16079); re-arm 40s.
local function onFireNova(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIRE_NOVA)
    end
    schedule(guid, "fire_nova", 40000, function()
        onFireNova(creature, guid)
    end)
end

local function thalnosResetState(guid)
    cancelTimers(guid)
    healthYellDone[guid] = nil
end

local function thalnosEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    thalnosResetState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "flame_shock", 10000, function()
        onFlameShock(creature, guid)
    end)
    schedule(guid, "shadow_bolt", 2000, function()
        onShadowBolt(creature, guid)
    end)
    schedule(guid, "flame_spike", 8000, function()
        onFlameSpike(creature, guid)
    end)
    schedule(guid, "fire_nova", 40000, function()
        onFireNova(creature, guid)
    end)
end

local function thalnosLeaveCombat(event, creature)
    thalnosResetState(creature:GetGUID())
end

local function thalnosDied(event, creature, killer)
    thalnosResetState(creature:GetGUID())
end

local function thalnosReset(event, creature)
    thalnosResetState(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_KILL 2) only when the victim is a player
-- (TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
local function thalnosTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_KILL)
    end
end

-- C++ DamageTaken arm: once, when post-damage health drops strictly
-- below 35% (HealthBelowPctDamaged(35, damage) — the damaged class, so
-- the incoming damage IS subtracted here): Talk(SAY_HEALTH 1). Modeled
-- on the pre-damage hook (event 9); the C++ arm casts no spell.
local function thalnosDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if healthYellDone[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 35 then
        healthYellDone[guid] = true
        creature:Talk(SAY_HEALTH)
    end
end

RegisterCreatureEvent(ENTRY_BLOODMAGE_THALNOS, 1, thalnosEnterCombat)
RegisterCreatureEvent(ENTRY_BLOODMAGE_THALNOS, 2, thalnosLeaveCombat)
RegisterCreatureEvent(ENTRY_BLOODMAGE_THALNOS, 3, thalnosTargetDied)
RegisterCreatureEvent(ENTRY_BLOODMAGE_THALNOS, 4, thalnosDied)
RegisterCreatureEvent(ENTRY_BLOODMAGE_THALNOS, 9, thalnosDamageTaken)
RegisterCreatureEvent(ENTRY_BLOODMAGE_THALNOS, 23, thalnosReset)
