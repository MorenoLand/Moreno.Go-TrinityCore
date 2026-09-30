-- High Priest Venoxis (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_venoxis.cpp
-- (boss_venoxisAI); zulgurub.h:31 (DATA_VENOXIS = 1, main boss; no
-- NPC_VENOXIS constant — verified by grep). Creature entry: 14507 High
-- Priest Venoxis (C++ ScriptName "boss_venoxis" per AddSC_boss_venoxis;
-- classicdb.ch/?npc=14507). The parasitic serpent 14884 (Misc enum
-- NPC_PARASITIC_SERPENT) has no C++ CreatureScript in this file and is
-- not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. No aggro Talk in the C++ AI.
-- Fight shape: enter combat arms thrash 3391 5s then {10s,20s} (all
-- phases) / holy nova 23858 5s then {45s,75s} (phase one: casts only
-- when 3 or more alive players are within melee range, the C++
-- top-10-threat scan approximated without a threat model) / dispel
-- magic 23859 35s then {15s,20s} self-cast (phase one) / holy fire
-- 23860 10s then {45s,60s} on a random alive player (phase one) /
-- renew 23895 30s then {25s,30s} self-cast (phase one) / holy wrath
-- 23979 60s then {45s,60s} on a random alive player (phase one).
-- DamageTaken (pre-damage, C++-exact with no adjustment): not above 50%
-- health -> transform armed 100ms (once) / else not above 20% health ->
-- frenzy armed 100ms (once). Transform: non-triggered self-cast 23849 +
-- Talk(SAY_VENOXIS_TRANSFORM=1); the phase-one timers are canceled (C++
-- SetPhase(PHASE_TWO) drops the phase-1 EventMap entries, jeklik
-- convention); arms venom spit 23862 5s then {5s,15s} / poison cloud
-- 23861 10s then {15s,20s} / parasitic serpent 23866 30s then 15s, all
-- on a random alive player (phase two). Frenzy: triggered self-cast
-- 8269. Death: Talk(SAY_VENOXIS_DEATH=2).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate has no
-- UNIT_STATE bridge — timers fire unconditionally (jeklik convention);
-- no instance-script model — boss admission via the luaBossAI shim
-- (_Reset/_JustDied/BossAI::JustEngagedWith, GetZulGurubAI bookkeeping
-- and the DATA_VENOXIS encounter-state arms skipped); DoZoneInCombat has
-- no bridge — zone adds are not pulled; RemoveAllAuras and the
-- REACT_PASSIVE/REACT_AGGRESSIVE arms have no bridge — reset is
-- timer/state-only (jindo convention); ResetThreatList has no bridge
-- (no threat model); the holy-nova SelectTarget(MaxThreat) scan has no
-- bearer — alive players in the instance are scanned instead with the
-- 5-yd melee check standing in for IsWithinMeleeRange; SelectTarget
-- (Random) targets are random alive players in the instance; the
-- parasitic-serpent summons 14884 never spawn (no summon model) — only
-- the 15s re-arm cycle is modeled (jeklik six-bat convention).

local ENTRY_VENOXIS = 14507

local SPELL_THRASH = 3391
local SPELL_DISPEL_MAGIC = 23859
local SPELL_RENEW = 23895
local SPELL_HOLY_NOVA = 23858
local SPELL_HOLY_FIRE = 23860
local SPELL_HOLY_WRATH = 23979

local SPELL_POISON_CLOUD = 23861
local SPELL_VENOM_SPIT = 23862
local SPELL_SUMMON_PARASITIC_SERPENT = 23866

local SPELL_VENOXIS_TRANSFORM = 23849
local SPELL_FRENZY = 8269

local TALK_VENOXIS_TRANSFORM = 1
local TALK_VENOXIS_DEATH = 2

local PHASE_ONE = 1
local PHASE_TWO = 2

-- Phase-1 timer keys canceled when entering phase two (C++ SetPhase
-- drops the phase-1 EventMap entries C++-exact).
local PHASE_ONE_TIMER_KEYS = { "holynova", "dispelmagic", "renew", "holyfire", "holywrath" }

local timers = {}
local venoxisState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    venoxisState[guid] = nil
end

local function cancelTimerKeys(guid, keys)
    local per = timers[guid]
    if not per then
        return
    end
    for _, key in ipairs(keys) do
        if per[key] then
            RemoveEventById(per[key])
            per[key] = nil
        end
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

local function alivePlayersInInstance(creature)
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

local function randomAlivePlayer(creature)
    local players = alivePlayersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_THRASH: triggered self-cast 3391; fires in all phases;
-- re-arm {10s,20s}.
local function onThrash(creature, guid)
    creature:CastSpell(creature, SPELL_THRASH, true)
    schedule(guid, "thrash", math.random(10000, 20000), function()
        onThrash(creature, guid)
    end)
end

-- C++ EVENT_HOLY_NOVA: phase one; counts the top-10 threat targets
-- within melee range — with no threat model this scans the alive players
-- in the instance within 5 yd instead — and non-triggered DoCastVictim
-- (23858) only when 3 or more qualify; the re-arm sits outside the
-- count gate, so it fires {45s,75s} C++-exact either way. Nil-victim
-- ticks cast nothing but keep the schedule (jeklik convention).
local function onHolyNova(creature, guid)
    local inMelee = 0
    for _, p in ipairs(alivePlayersInInstance(creature)) do
        if creature:GetDistance(p) <= 5 then
            inMelee = inMelee + 1
        end
    end
    if inMelee >= 3 then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_HOLY_NOVA)
        end
    end
    schedule(guid, "holynova", math.random(45000, 75000), function()
        onHolyNova(creature, guid)
    end)
end

-- C++ EVENT_DISPEL_MAGIC: non-triggered self-cast 23859, phase one;
-- re-arm {15s,20s}.
local function onDispelMagic(creature, guid)
    creature:CastSpell(creature, SPELL_DISPEL_MAGIC)
    schedule(guid, "dispelmagic", math.random(15000, 20000), function()
        onDispelMagic(creature, guid)
    end)
end

-- C++ EVENT_RENEW: non-triggered self-cast 23895, phase one;
-- re-arm {25s,30s}.
local function onRenew(creature, guid)
    creature:CastSpell(creature, SPELL_RENEW)
    schedule(guid, "renew", math.random(25000, 30000), function()
        onRenew(creature, guid)
    end)
end

-- C++ EVENT_HOLY_FIRE: non-triggered DoCast(23860) on a random alive
-- player, phase one; nil-target ticks cast nothing but keep the
-- schedule. Re-arm {45s,60s}.
local function onHolyFire(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_HOLY_FIRE)
    end
    schedule(guid, "holyfire", math.random(45000, 60000), function()
        onHolyFire(creature, guid)
    end)
end

-- C++ EVENT_HOLY_WRATH: non-triggered DoCast(23979) on a random alive
-- player, phase one; nil-target ticks cast nothing but keep the
-- schedule. Re-arm {45s,60s}.
local function onHolyWrath(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_HOLY_WRATH)
    end
    schedule(guid, "holywrath", math.random(45000, 60000), function()
        onHolyWrath(creature, guid)
    end)
end

-- C++ EVENT_VENOM_SPIT: non-triggered DoCast(23862) on a random alive
-- player, phase two; nil-target ticks cast nothing but keep the
-- schedule. Re-arm {5s,15s}.
local function onVenomSpit(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_VENOM_SPIT)
    end
    schedule(guid, "venomspit", math.random(5000, 15000), function()
        onVenomSpit(creature, guid)
    end)
end

-- C++ EVENT_POISON_CLOUD: non-triggered DoCast(23861) on a random alive
-- player, phase two; nil-target ticks cast nothing but keep the
-- schedule. Re-arm {15s,20s}.
local function onPoisonCloud(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_POISON_CLOUD)
    end
    schedule(guid, "poisoncloud", math.random(15000, 20000), function()
        onPoisonCloud(creature, guid)
    end)
end

-- C++ EVENT_PARASITIC_SERPENT: non-triggered DoCast(23866) on a random
-- alive player, phase two; the parasitic serpents 14884 never spawn (no
-- summon model — only the re-arm cycle is modeled, jeklik six-bat
-- convention). Re-arm 15s.
local function onParasiticSerpent(creature, guid)
    local target = randomAlivePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SUMMON_PARASITIC_SERPENT)
    end
    schedule(guid, "parasiticserpent", 15000, function()
        onParasiticSerpent(creature, guid)
    end)
end

-- C++ EVENT_TRANSFORM: non-triggered self-cast 23849 +
-- Talk(SAY_VENOXIS_TRANSFORM=1); ResetThreatList has no bridge (no
-- threat model); the phase-one timers are canceled (C++
-- SetPhase(PHASE_TWO) drops the phase-1 EventMap entries C++-exact);
-- arms the phase-two timers.
local function onTransform(creature, guid)
    local st = venoxisState[guid]
    creature:CastSpell(creature, SPELL_VENOXIS_TRANSFORM)
    creature:Talk(TALK_VENOXIS_TRANSFORM)
    cancelTimerKeys(guid, PHASE_ONE_TIMER_KEYS)
    if st then
        st.phase = PHASE_TWO
    end
    schedule(guid, "venomspit", 5000, function()
        onVenomSpit(creature, guid)
    end)
    schedule(guid, "poisoncloud", 10000, function()
        onPoisonCloud(creature, guid)
    end)
    schedule(guid, "parasiticserpent", 30000, function()
        onParasiticSerpent(creature, guid)
    end)
end

-- C++ EVENT_FRENZY: triggered self-cast 8269 at 20% health, once.
local function onFrenzy(creature, guid)
    creature:CastSpell(creature, SPELL_FRENZY, true)
end

-- C++ DamageTaken: the engine event-9 hook fires pre-damage (thekal
-- convention — verified Unit::DealDamage calls DamageTaken before
-- ModifyHealth), so the HealthAbovePct checks are C++-exact with no
-- adjustment. The if/else-if chain is C++-exact: a single hit cannot arm
-- both the transform and the frenzy.
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = venoxisState[guid]
    if not st then
        return
    end
    local healthPct = creature:GetHealthPct()
    if not st.transformed and healthPct <= 50 then
        st.transformed = true
        schedule(guid, "transform", 100, function()
            onTransform(creature, guid)
        end)
    elseif not st.frenzied and healthPct <= 20 then
        st.frenzied = true
        schedule(guid, "frenzy", 100, function()
            onFrenzy(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_VENOXIS, 1, function(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ Reset: _Reset, RemoveAllAuras / REACT_PASSIVE have no bridge —
    -- state and timers only (jindo convention). C++ JustEngagedWith:
    -- BossAI::JustEngagedWith bookkeeping and DoZoneInCombat have no
    -- bridge.
    venoxisState[guid] = { phase = PHASE_ONE, transformed = false, frenzied = false }
    schedule(guid, "thrash", 5000, function()
        onThrash(creature, guid)
    end)
    schedule(guid, "holynova", 5000, function()
        onHolyNova(creature, guid)
    end)
    schedule(guid, "dispelmagic", 35000, function()
        onDispelMagic(creature, guid)
    end)
    schedule(guid, "holyfire", 10000, function()
        onHolyFire(creature, guid)
    end)
    schedule(guid, "renew", 30000, function()
        onRenew(creature, guid)
    end)
    schedule(guid, "holywrath", 60000, function()
        onHolyWrath(creature, guid)
    end)
end)

RegisterCreatureEvent(ENTRY_VENOXIS, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_VENOXIS, 4, function(event, creature)
    -- C++ JustDied: _JustDied, Talk(SAY_VENOXIS_DEATH), RemoveAllAuras
    -- (no bridge).
    creature:Talk(TALK_VENOXIS_DEATH)
    cancelTimers(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_VENOXIS, 9, onDamageTaken)

RegisterCreatureEvent(ENTRY_VENOXIS, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
