-- Nagrand Kil Sorrow / Warmaul banner mobs (Outland zone script) --
-- Lua port of src/server/scripts/Outland/zone_nagrand.cpp
-- (npc_nagrand_banner — the CreatureScript AI class
-- AddSC_nagrand registers; its GetAI switches on creature
-- entry, so all five combat variants live here, C++-like).
-- Third zone script in the Outland set per
-- outland_script_loader.cpp order (blades_edge_mountains,
-- hellfire_peninsula, nagrand, netherstorm,
-- shadowmoon_valley, terokkar_forest).
-- Entries (all verifiable from the C++ sources — the
-- zone_nagrand.cpp enum): NPC_KIL_SORROW_SPELLBINDER =
-- 17146, NPC_KIL_SORROW_CULTIST = 17147, NPC_KIL_SORROW_
-- DEATHSWORN = 17148, NPC_GISELDA_THE_CRONE = 18391,
-- NPC_WARMAUL_SHAMAN = 18064. NPC_WARMAUL_REAVER = 17138
-- (the only other entry constant in the file) falls into
-- the C++ default case — the base npc_nagrand_bannerAI —
-- whose only arms are the SpellHit bannered latch and
-- engine melee: the latch's only observable is the
-- ConditionScript (no bridge, see below), so it is a
-- no-op model — documented only, unregistered
-- (flame_patch precedent). The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk: no SAY_/EMOTE_ enums in any of the five AI classes
-- — no talks fire anywhere in this model (C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 9 OnDamageTaken, 23 OnReset. No event 3 — the C++
-- overrides none of KilledUnit.
-- Timers via CreateLuaEvent (per-GUID scheduler pumps);
-- melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady. C++ DoCast default is triggered=
-- false (vaelastrasz convention). The C++ TaskScheduler's
-- SetValidator (!HasUnitState(UNIT_STATE_CASTING)) has no
-- cast-state bridge — the arms fire unconditionally
-- (netherspite precedent).
-- npc_maghar_captive / npc_kurenai_captive (the other two
-- CreatureScripts AddSC_nagrand registers) are EscortAI:
-- the waypoint machine, the OnQuestAccept escort start /
-- quest fail+credit, the waypoint SummonCreature ambushes
-- and the JustSummoned move/attack choreography sit behind
-- the no-escort / no-quest-accept / no-summon / no-movement
-- bridges (omor / hellfire_peninsula precedents) —
-- documented only. condition_nagrand_banner is a
-- ConditionScript — no ConditionScript bridge — documented
-- only (its only input is the bannered latch of the base
-- AI, also unmodeled).
-- Fight shape (C++-exact for all modeled arms):
-- Kil Sorrow spellbinder (17146): OnEnterCombat(1):
-- per-GUID reset (has_fled = false, interrupt_cooldown =
-- 20000) + 1s scheduler pump (a port of UpdateAI in C++
-- arm order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim || !GetVictim early return collapses into
-- the pump). Pump: arcane-missiles arm — 0s init then
-- 2400-3800ms: non-triggered DoCastVictim(SPELL_ARCANE_
-- MISSILES 34447) (C++-exact), re-arm 2400+rand()%1400;
-- chains-of-ice arm — 3-6s init then 20-25s: TRIGGERED
-- DoCast(target, SPELL_CHAINS_OF_ICE 22744, true) on a
-- random alive player in the instance (C++
-- SelectTarget(Random, 0), thespia precedent; nil pick
-- casts nothing, C++-exact), re-arm 20000+rand()%5000.
-- The interrupt arm (victim UNIT_STATE_CASTING &&
-- interrupt_cooldown > 25000 -> TRIGGERED DoCastVictim
-- SPELL_COUNTERSPELL 31999) has no victim cast-state
-- bridge — unmodeled (netherspite precedent); the
-- DamageTaken flee latch (me->DoFleeToGetAssistance at 15%
-- — HealthBelowPctDamaged(15)) has no flee bridge —
-- unmodeled, the latch lands nowhere.
-- Kil Sorrow cultist (17147): OnEnterCombat(1): per-GUID
-- reset + 1s pump. Pump: mind-sear arm — 4.5s init then
-- 7-11s: non-triggered DoCastVictim(SPELL_MIND_SEAR 32000)
-- (C++-exact), re-arm 7000+rand()%4000.
-- Kil Sorrow deathsworn (17148): no timers — no pump.
-- OnEnterCombat(1): used_bloodthirst = false.
-- OnDamageTaken(9): the C++ DamageTaken latch —
-- HealthBelowPctDamaged(50, damage) — health after this
-- hit strictly below 50% (shahraz strict-fraction
-- precedent): used_bloodthirst = true + non-triggered
-- DoCastVictim(SPELL_BLOODTHIRST 31996) (C++-exact).
-- Giselda the crone (18391): no timers — no pump.
-- OnEnterCombat(1): used_transform = false.
-- OnDamageTaken(9): the C++ DamageTaken latch —
-- HealthBelowPctDamaged(65, damage) — health after this
-- hit strictly below 65% (shahraz strict-fraction
-- precedent): used_transform = true + non-triggered
-- DoCastVictim(SPELL_GISELDA_TRANSFORM_DND 33316)
-- (C++-exact).
-- Warmaul shaman (18064): OnEnterCombat(1): per-GUID
-- reset (used_healing = false) + 1s pump. Pump:
-- scorching-totem arm — 2s one-shot: non-triggered
-- DoCastSelf(SPELL_SCORCHING_TOTEM 15038) (C++-exact,
-- no repeat); frost-shock arm — 6s init then 12s:
-- non-triggered DoCastVictim(SPELL_FROST_SHOCK 12548)
-- (C++-exact), re-arm 12000. OnDamageTaken(9): the C++
-- DamageTaken latch — HealthBelowPctDamaged(50, damage)
-- — health after this hit strictly below 50% (shahraz
-- strict-fraction precedent): used_healing = true +
-- non-triggered DoCastSelf(SPELL_HEALING_WAVE 11986)
-- (C++-exact). Melee is engine-driven.
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23): cancel the pump,
-- drop per-GUID state (the C++ Reset() observable
-- remainder — scheduler.CancelAll() — lands as the pump
-- cancel; the re-latch lands on OnEnterCombat, gargolmar
-- precedent; Eluna's On_Reset fires ahead of OnDied and
-- OnSpawn — millhouse note — so both hooks land the same
-- reset).
-- Deliberate deviations (all await engine bridges): no
-- escort / quest-accept / summon / movement bridges — the
-- maghar and kurenai captive AIs documented only; no
-- ConditionScript bridge — condition_nagrand_banner
-- documented only; no cast-state bridge — the TaskScheduler
-- validator and the spellbinder interrupt arm unmodeled;
-- no flee bridge — the spellbinder 15% DoFleeToGetAssistance
-- latch unmodeled; no SpellHit bridge — the base banner
-- AI's bannered latch (SPELL_PLANT_WARMAUL_OGRE_BANNER
-- 32307 / SPELL_PLANT_KIL_SORROW_BANNER 32314) unmodeled
-- (its only observable is the ConditionScript).

local ENTRY_KIL_SORROW_SPELLBINDER = 17146
local ENTRY_KIL_SORROW_CULTIST     = 17147
local ENTRY_KIL_SORROW_DEATHSWORN  = 17148
local ENTRY_GISELDA_THE_CRONE      = 18391
local ENTRY_WARMAUL_SHAMAN         = 18064

local SPELL_ARCANE_MISSILES        = 34447
local SPELL_CHAINS_OF_ICE          = 22744
local SPELL_MIND_SEAR              = 32000
local SPELL_BLOODTHIRST            = 31996
local SPELL_GISELDA_TRANSFORM_DND  = 33316
local SPELL_SCORCHING_TOTEM        = 15038
local SPELL_FROST_SHOCK            = 12548
local SPELL_HEALING_WAVE           = 11986

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    states[guid] = nil
end

local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0): any alive player in the instance
-- (thespia precedent).
local function randomAlivePlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ UpdateAI in arm order for the spellbinder — 1s
-- granularity exact for all C++ timers; the !UpdateVictim
-- || !GetVictim early return collapses into the pump.
local function spellbinderTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Arcane missiles arm: 0s init then 2400-3800ms,
    -- non-triggered DoCastVictim (C++-exact).
    if st.missiles <= 1000 then
        creature:CastSpell(nil, SPELL_ARCANE_MISSILES)
        st.missiles = 2400 + math.random(0, 1400)
    else
        st.missiles = st.missiles - 1000
    end

    -- Chains of ice arm: 3-6s init then 20-25s, TRIGGERED
    -- on a random alive player in the instance (C++-exact).
    if st.chains <= 1000 then
        local target = randomAlivePlayer(creature)
        if target then
            creature:CastSpell(target, SPELL_CHAINS_OF_ICE, true)
        end
        st.chains = 20000 + math.random(0, 5000)
    else
        st.chains = st.chains - 1000
    end
end

-- C++ UpdateAI in arm order for the cultist — 1s
-- granularity exact; the !UpdateVictim early return
-- collapses into the pump.
local function cultistTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Mind sear arm: 4.5s init then 7-11s, non-triggered
    -- DoCastVictim (C++-exact).
    if st.mindSear <= 1000 then
        creature:CastSpell(nil, SPELL_MIND_SEAR)
        st.mindSear = 7000 + math.random(0, 4000)
    else
        st.mindSear = st.mindSear - 1000
    end
end

-- C++ UpdateAI in arm order for the warmaul shaman — 1s
-- granularity exact; the !UpdateVictim early return
-- collapses into the pump.
local function shamanTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Scorching totem arm: 2s one-shot, non-triggered
    -- DoCastSelf (C++-exact, no repeat).
    if not st.totemDone then
        if st.totem <= 1000 then
            creature:CastSpell(creature, SPELL_SCORCHING_TOTEM)
            st.totemDone = true
        else
            st.totem = st.totem - 1000
        end
    end

    -- Frost shock arm: 6s init then 12s, non-triggered
    -- DoCastVictim (C++-exact).
    if st.frostShock <= 1000 then
        creature:CastSpell(nil, SPELL_FROST_SHOCK)
        st.frostShock = 12000
    else
        st.frostShock = st.frostShock - 1000
    end
end

local function startPump(guid, creature, tick)
    cancelPump(guid)
    timers[guid] = CreateLuaEvent(function()
        tick(creature, guid)
    end, 1000, 0)
end

-- C++ JustEngagedWith per entry: re-latch the ctor /
-- Initialize() values, start the combat pump where the
-- AI has timers (gargolmar precedent).
RegisterCreatureEvent(ENTRY_KIL_SORROW_SPELLBINDER, 1, function(_, creature)
    local guid = creature:GetGUID()
    states[guid] = {
        missiles = 0,
        chains = 3000 + math.random(0, 3000),
    }
    startPump(guid, creature, spellbinderTick)
end)

RegisterCreatureEvent(ENTRY_KIL_SORROW_CULTIST, 1, function(_, creature)
    local guid = creature:GetGUID()
    states[guid] = {
        mindSear = 4500,
    }
    startPump(guid, creature, cultistTick)
end)

RegisterCreatureEvent(ENTRY_KIL_SORROW_DEATHSWORN, 1, function(_, creature)
    states[creature:GetGUID()] = { usedBloodthirst = false }
end)

RegisterCreatureEvent(ENTRY_GISELDA_THE_CRONE, 1, function(_, creature)
    states[creature:GetGUID()] = { usedTransform = false }
end)

RegisterCreatureEvent(ENTRY_WARMAUL_SHAMAN, 1, function(_, creature)
    local guid = creature:GetGUID()
    states[guid] = {
        usedHealing = false,
        totem = 2000,
        totemDone = false,
        frostShock = 6000,
    }
    startPump(guid, creature, shamanTick)
end)

-- C++ DamageTaken per entry — the strict-fraction pct
-- latches (shahraz precedent): health after this hit
-- strictly below the pct threshold.
RegisterCreatureEvent(ENTRY_KIL_SORROW_DEATHSWORN, 9, function(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = states[guid]
    if not st or st.usedBloodthirst then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 50 then
        st.usedBloodthirst = true
        creature:CastSpell(nil, SPELL_BLOODTHIRST)
    end
end)

RegisterCreatureEvent(ENTRY_GISELDA_THE_CRONE, 9, function(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = states[guid]
    if not st or st.usedTransform then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 65 then
        st.usedTransform = true
        creature:CastSpell(nil, SPELL_GISELDA_TRANSFORM_DND)
    end
end)

RegisterCreatureEvent(ENTRY_WARMAUL_SHAMAN, 9, function(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = states[guid]
    if not st or st.usedHealing then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 50 then
        st.usedHealing = true
        creature:CastSpell(creature, SPELL_HEALING_WAVE)
    end
end)

-- C++ Reset() (called on evade): fullReset — the
-- scheduler.CancelAll() arm lands as the pump cancel; the
-- re-latch lands on OnEnterCombat.
local cleanupEntries = {
    ENTRY_KIL_SORROW_SPELLBINDER,
    ENTRY_KIL_SORROW_CULTIST,
    ENTRY_KIL_SORROW_DEATHSWORN,
    ENTRY_GISELDA_THE_CRONE,
    ENTRY_WARMAUL_SHAMAN,
}
for _, entry in ipairs(cleanupEntries) do
    RegisterCreatureEvent(entry, 2, function(_, creature)
        fullReset(creature:GetGUID())
    end)
    -- Eluna's On_Reset fires ahead of OnDied and OnSpawn,
    -- so this lands the same Reset() reset as the evade
    -- hook (millhouse note).
    RegisterCreatureEvent(entry, 4, function(_, creature)
        fullReset(creature:GetGUID())
    end)
    RegisterCreatureEvent(entry, 23, function(_, creature)
        fullReset(creature:GetGUID())
    end)
end
