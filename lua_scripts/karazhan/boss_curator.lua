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
-- no Lua-modelable content and is not registered. Mana is drained on
-- the live field: each Astral Flare summon does ModifyPower(POWER_MANA=0,
-- -floor(GetMaxPower/10)) (C++ boss_curator.cpp:118-120;
-- creature:ModifyPower bridged at lua_creature_events.go:603, clamping
-- at 0 and broadcasting UNIT_FIELD_POWER1 like C++); the Evocate arm
-- fires when floor(newPower*100/maxPower) < 10, the integer form of the
-- C++ check. The Evocation channel's in-spell mana refill is
-- approximated by creature:SetPower(0, GetMaxPower()) after the 20s
-- channel (SetPower clamps to max, lua_creature_events.go:578). The
-- engine models no in-combat mana regen, so the live field is the
-- deterministic drain clock (10 summons, ~100s) — no Lua-side model. ApplySpellImmune(arcane damage)
-- from Reset is bridged (creature:ApplySpellImmune, lua_creature_events.go)
-- and ported at onReset below; InterruptNonMeleeSpells(false) before
-- Evocation is bridged (no-op twin, lua_creature_events.go) and ported in onAstralFlare below. Hateful Bolt's SelectTarget(MaxThreat, 1) — second-highest threat,
-- i.e. a non-tank — is PORTED via creature:GetAITargets()[2] (threat-ordered,
-- lua_creature_events.go; the stale "no threat model" note is retracted).

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
    if per[key] then
        RemoveEventById(per[key])
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

-- C++ EVENT_HATEFUL_BOLT: SelectTarget(MaxThreat, 1), all defaults (no
-- range cap, playerOnly false, withTank true) — the second entry of the
-- threat-ordered GetAITargets list; nil when fewer than 2 entries (C++
-- returns nullptr when the threat list size <= offset). C++ applies no
-- dead filter, so none added (midnight precedent).
local function onHatefulBolt(creature, guid)
    local targets = creature:GetAITargets()
    local target = targets[2]
    if target then
        creature:CastSpell(target, SPELL_HATEFUL_BOLT)
    end
    schedule(guid, "hateful", {7000, 15000}, function() onHatefulBolt(creature, guid) end)
end

-- C++ Evocation (SPELL_EVOCATION 30254) refills mana in-spell; the 20s
-- channel end restores the live field to full (SetPower clamps to max).
local function onEvocateEnd(creature, guid)
    creature:SetPower(0, creature:GetMaxPower())
end

local function onAstralFlare(creature, guid)
    local st = state[guid]
    if st == nil then
        return
    end
    if math.random(2) == 1 then
        creature:Talk(SAY_SUMMON)
    end
    -- C++ EVENT_SUMMON_ASTRAL_FLARE (boss_curator.cpp:111-126): the
    -- summon visual is cast first, then ModifyPower(POWER_MANA,
    -- -floor(GetMaxPower/10)); the Evocate arm fires when
    -- GetPower*100/GetMaxPower < 10 (integer form). The whole block is
    -- skipped when floor(maxPower/10) == 0 (C++ int32-mana gate).
    local maxMana = creature:GetMaxPower()
    local drain = math.floor(maxMana / 10)
    if drain ~= 0 then
        creature:CastSpell(creature, SPELL_SUMMON_ASTRAL_FLARE[math.random(#SPELL_SUMMON_ASTRAL_FLARE)], true)
        local newMana = creature:ModifyPower(0, -drain)
        if math.floor(newMana * 100 / maxMana) < 10 then
            creature:Talk(SAY_EVOCATE)
            creature:InterruptNonMeleeSpells(false)
            creature:CastSpell(creature, SPELL_EVOCATION)
            schedule(guid, "evocateend", SPELL_EVOCATION_CHANNEL_MS,
                function() onEvocateEnd(creature, guid) end)
        end
    end
    schedule(guid, "astralflare", 10000, function() onAstralFlare(creature, guid) end)
end

local function onArcaneInfusion(creature)
    creature:CastSpell(creature, SPELL_ARCANE_INFUSION, true)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = { infused = false }
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
    -- C++ Reset() :65: ApplySpellImmune(0, IMMUNITY_DAMAGE,
    -- SPELL_SCHOOL_MASK_ARCANE=64, true) — arcane-damage immunity for
    -- the evocation mechanic (lua_creature_events.go bridge).
    creature:ApplySpellImmune(0, 3, 64, true)
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
        st = { infused = false }
        state[guid] = st
    end
    -- C++ DamageTaken reads !HealthAbovePct(15) on current health; the
    -- engine fires event 9 pre-damage, so `health` is already the C++
    -- value: latch when health <= floor(maxHealth*15/100), the integer
    -- form of !HealthAbovePct(15) (C++ fires at exactly the threshold).
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth ~= 0 and not st.infused and health <= math.floor(maxHealth * 15 / 100) then
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
