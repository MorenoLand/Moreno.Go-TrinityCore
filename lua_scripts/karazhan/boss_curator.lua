-- The Curator (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_curator.cpp
-- Creature entry 15691 (wowhead wotlk npc=15691/the-curator; TDB
-- creature_template ScriptName "boss_curator"). Instance data:
-- DATA_CURATOR = 5 (karazhan.h:35); the encounter never reads/writes
-- instance state beyond the BossAI admission handled by the luaBossAI
-- shim in engine/world/boss_ai.go.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (soft-enrage arm, damage untouched), 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Deviations from C++: the engine has no summon model, so
-- SPELL_SUMMON_ASTRAL_FLARE_* (30236/30239/30240/30241) are visual-only
-- casts and the Eluna events 19/22 never fire — the Astral Flare
-- npc_curator_astral_flare react-state/deselect plumbing (entry 17086) has
-- no Lua-modelable content and is not registered. Mana is tracked
-- Lua-side: each Astral Flare summon drains 10% of max mana exactly as
-- C++ ModifyPower(POWER_MANA, -GetMaxPower/10); the Evocate arm fires when
-- the drain leaves mana strictly below 10%, mirroring C++ GetPower*100/
-- GetMaxPower < 10. The Lua bridge exposes no power APIs, so the Evocation
-- channel's in-spell mana refill is approximated by restoring mana to 100%
-- after the 20s channel; in-combat mana regen is not modeled (evocates on
-- a pure drain clock: 10 summons, ~100s). ApplySpellImmune(arcane damage)
-- from Reset and InterruptNonMeleeSpells(false) before Evocation have no
-- bridge. Hateful Bolt's SelectTarget(MaxThreat, 1) — second-highest threat,
-- i.e. a non-tank — is approximated with a random alive player in 100 yd,
-- since the engine has no threat model.

local ENTRY = 15691

local SAY_AGGRO, SAY_SUMMON, SAY_EVOCATE, SAY_ENRAGE = 0, 1, 2, 3
local SAY_KILL, SAY_DEATH = 4, 5

local SPELL_HATEFUL_BOLT = 30383
local SPELL_EVOCATION = 30254
local SPELL_ARCANE_INFUSION = 30403
local SPELL_BERSERK = 26662
local SPELL_SUMMON_ASTRAL_FLARE = { 30236, 30239, 30240, 30241 }

local SPELL_EVOCATION_CHANNEL_MS = 20000

local timers = {}
local state = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

local function randomTargetInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onHatefulBolt(creature, guid)
    local target = randomTargetInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_HATEFUL_BOLT)
    end
    schedule(guid, "hateful", {7000, 15000}, function() onHatefulBolt(creature, guid) end)
end

local function onEvocateEnd(creature, guid)
    local st = state[guid]
    if st then
        st.mana = 100
    end
end

local function onAstralFlare(creature, guid)
    local st = state[guid]
    if st == nil then
        return
    end
    if math.random(2) == 1 then
        creature:Talk(SAY_SUMMON)
    end
    creature:CastSpell(creature, SPELL_SUMMON_ASTRAL_FLARE[math.random(#SPELL_SUMMON_ASTRAL_FLARE)], true)
    st.mana = st.mana - 10
    if st.mana < 10 then
        creature:Talk(SAY_EVOCATE)
        creature:CastSpell(creature, SPELL_EVOCATION)
        schedule(guid, "evocateend", SPELL_EVOCATION_CHANNEL_MS,
            function() onEvocateEnd(creature, guid) end)
    end
    schedule(guid, "astralflare", 10000, function() onAstralFlare(creature, guid) end)
end

local function onArcaneInfusion(creature)
    creature:CastSpell(creature, SPELL_ARCANE_INFUSION, true)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { infused = false, mana = 100 }
    creature:Talk(SAY_AGGRO)
    schedule(guid, "hateful", 12000, function() onHatefulBolt(creature, guid) end)
    schedule(guid, "astralflare", 10000, function() onAstralFlare(creature, guid) end)
    schedule(guid, "enrage", 720000, function()
        creature:Talk(SAY_ENRAGE)
        creature:CastSpell(creature, SPELL_BERSERK, true)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = state[guid]
    if st == nil then
        st = { infused = false, mana = 100 }
        state[guid] = st
    end
    -- Engine fires event 9 post-damage (combat.go reduces health before
    -- dispatching), while C++ DamageTaken fires pre-damage with
    -- HealthAbovePct(15) reading current health. Closest faithful match:
    -- latch when current (already post-damage) health pct is at or below
    -- 15, i.e. not strictly above 15 as C++ HealthAbovePct requires.
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth ~= 0 and not st.infused and health * 100 / maxHealth <= 15 then
        st.infused = true
        cancelKey(guid, "astralflare")
        schedule(guid, "infusion", 1, function() onArcaneInfusion(creature) end)
    end
    return false, damage
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
