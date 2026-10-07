-- Aku'mai (Blackfathom Deeps) — Lua port of
-- src/server/scripts/Kalimdor/BlackfathomDeeps/boss_aku_mai.cpp
-- (boss_aku_maiAI : public BossAI(creature, DATA_AKU_MAI); AddSC_boss_aku_mai
-- registers the one script; kalimdor loader decl 23 / call 136 per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms boss_aku_mai.cpp as the sole
-- source of the "boss_aku_mai" script name (loader lines only otherwise).
-- Entry: blackfathom_deeps.h BFDCreatureIds names NO Aku'mai boss entry and
-- instance_blackfathom_deeps.cpp's OnCreatureCreate cases only Kelris 4832 /
-- Lorgus 12902 — zero corroborating C++ usage of any Aku'mai entry anywhere
-- in src/server. Registered on the DB ScriptName "boss_aku_mai" tie plus
-- wowhead corroboration (www.wowhead.com/npc=4829/akumai, Classic Aku'mai) —
-- kalecgos-level evidence, jeklik precedent; the creature_template
-- ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 9 OnDamageTaken
-- (fires pre-damage), 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms:
-- - Reset (23, C++-exact for the C++ Initialize() leg): IsEnraged flag
--   cleared; the BossAI _Reset() instance bookkeeping leg has no bridge.
-- - JustEngagedWith: Poison Cloud 3815 DoCastVictim (jeklik GetVictim +
--   CastSpell convention, non-triggered), init {5s,9s} -> repeat {25s,50s}
--   (C++-exact).
-- - DamageTaken: HealthBelowPctDamaged(30, damage) -> non-triggered
--   self-cast Frenzied Rage 3490 (C++ DoCast(me, spell) uses default
--   CastSpellExtraArgs -> TRIGGERED_NONE; CastSpell(self, 3490, false)
--   convention), one-shot via the IsEnraged flag (C++-exact; jeklik
--   pre-damage subtraction makes the projected-health check exact).
-- Unmodeled (documented-only, no bridges): the BossAI _Reset() /
-- _JustReachedHome() / _JustDied() instance bookkeeping legs — no
-- instance-script model (admission only via the luaBossAI shim); the
-- GetBlackfathomDeepsAI -> GetInstanceAI leg (instance-script model
-- blocked, standing). C++ has no KilledUnit / Death / text arms.

local ENTRY_AKU_MAI = 4829

local SPELL_POISON_CLOUD = 3815
local SPELL_FRENZIED_RAGE = 3490

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

local function akuMaiState(guid)
    local st = state[guid]
    if not st then
        st = { enraged = false }
        state[guid] = st
    end
    return st
end

-- C++ EVENT_POISON_CLOUD: DoCastVictim, non-triggered (jeklik convention).
local function onPoisonCloud(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_POISON_CLOUD)
    end
    schedule(guid, "poisoncloud", math.random(25000, 50000), function()
        onPoisonCloud(creature, guid)
    end)
end

local function akuMaiResetState(guid)
    cancelTimers(guid)
    state[guid] = { enraged = false }
end

-- C++ Reset: Initialize() (IsEnraged = false) + _Reset() (no bridge).
local function akuMaiOnReset(event, creature)
    akuMaiResetState(creature:GetGUID())
end

local function akuMaiLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (no bridge) +
-- ScheduleEvent(EVENT_POISON_CLOUD, 5s, 9s).
local function akuMaiEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    akuMaiResetState(guid)
    schedule(guid, "poisoncloud", math.random(5000, 9000), function()
        onPoisonCloud(creature, guid)
    end)
end

-- C++ DamageTaken: !IsEnraged && HealthBelowPctDamaged(30, damage) ->
-- DoCast(me, SPELL_FRENZIED_RAGE) (triggered), IsEnraged = true.
-- Eluna event 9 fires pre-damage, so the projected-health check subtracts
-- the incoming damage (jeklik precedent — C++-exact).
local function akuMaiDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = akuMaiState(guid)
    if st.enraged then
        return
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (health - damage) * 100 < 30 * maxHealth then
        st.enraged = true
        creature:CastSpell(creature, SPELL_FRENZIED_RAGE, false)
    end
end

RegisterCreatureEvent(ENTRY_AKU_MAI, 1, akuMaiEnterCombat)
RegisterCreatureEvent(ENTRY_AKU_MAI, 2, akuMaiLeaveCombat)
RegisterCreatureEvent(ENTRY_AKU_MAI, 9, akuMaiDamageTaken)
RegisterCreatureEvent(ENTRY_AKU_MAI, 23, akuMaiOnReset)
