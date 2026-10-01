-- Kalecgos (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_kalecgos.cpp
-- (boss_kalecgos, boss_kalecgos_human, boss_sathrovarr;
-- go_kalecgos_spectral_rift and the spell_kalecgos_tap_check /
-- spell_kalecgos_spectral_blast / spell_kalecgos_spectral_realm_
-- trigger / spell_kalecgos_spectral_realm_aura / spell_kalecgos_
-- curse_of_boundless_agony SpellScript/AuraScript classes documented
-- below, not registered); sunwell_plateau.h:30 (DATA_KALECGOS = 0,
-- first boss), :41 (DATA_KALECGOS_DRAGON), :42 (DATA_KALECGOS_HUMAN),
-- :43 (DATA_SATHROVARR), :89 (NPC_KALECGOS = 24850), :90 (NPC_
-- KALECGOS_HUMAN = 24891), :91 (NPC_SATHROVARR = 24892), :135 (GO_
-- SPECTRAL_RIFT = 187055).
-- Creature entries: 24850 Kalecgos (C++ ScriptName "boss_kalecgos"),
-- 24891 Kalecgos human form (C++ ScriptName "boss_kalecgos_human"),
-- 24892 Sathrovarr the Corruptor (C++ ScriptName "boss_sathrovarr"),
-- all per RegisterSunwellPlateauCreatureAI in AddSC_boss_kalecgos;
-- the creature_template ScriptName bindings are DB-side — no TDB in
-- this workspace, so only the C++-side naming is verified. Talk
-- lines used: Kalecgos SAY_EVIL_AGGRO=0 (pull), SAY_EVIL_SLAY=1
-- (kill, player-only, 50%), EMOTE_ENRAGE=4 (enrage), SAY_ARCANE_
-- BUFFET=6 (arcane buffet, 20%); Sathrovarr SAY_SATH_AGGRO=0 (pull),
-- SAY_SATH_SLAY=1 (kill, player-only), SAY_SATH_DEATH=2 (death),
-- SAY_SATH_SPELL1=3 (shadowbolt, 20%), SAY_SATH_SPELL2=4 (corruption
-- strike, 20%); human SAY_GOOD_NEAR_DEATH_0/1/2=0/1/2 (75%/50%/10%
-- thresholds), SAY_GOOD_DEATH=3 (death). SAY_OUTRO_1=2 and SAY_OUTRO_
-- 2=3 belong to the unmodeled outro movement chain.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (returns false,
-- newDamage; the boolean is consumed by the engine, the number
-- rewrites damage — reliquary convention), 14 OnHitBySpell, 23
-- OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): Kalecgos (24850).
-- OnEnterCombat: per-GUID reset (_isEnraged=false, _isBanished=false),
-- Talk(SAY_EVIL_AGGRO); the SummonCreature(NPC_KALECGOS_HUMAN) arm
-- has no summon bridge — the human AI is registered on its own
-- entry (DB-side spawn/summon, creature_template binding). Arcane
-- buffet 45018 8s: 20% Talk(SAY_ARCANE_BUFFET) + DoCastAOE
-- (self-position cast — vaelastrasz convention), repeat 8s. Frost
-- breath 44799 15s: DoCastAOE, repeat 15s. Tail lash 45122 25s then
-- 15s: DoCastAOE, repeat 15s. Wild magic 10s then 20s: triggered
-- DoCastAOE one of {45001, 45002, 45004, 45006, 45010, 44978},
-- repeat 20s. Spectral blast 44869 {20s,25s} then {20s,25s}:
-- triggered DoCastAOE, repeat {20s,25s}; the NonTankTargetSelector/
-- aura-filter targeting and the teleport arms live in the unmodeled
-- spectral-blast SpellScript (standing SpellScript gap). Check
-- timer 1s: !_isEnraged and health strictly below 10% -> _isEnraged=
-- true, Talk(EMOTE_ENRAGE), triggered self-cast enrage 44807 (the
-- C++ DoAction(ACTION_ENRAGE) arm, inlined); health strictly below
-- 1%: the sathrovarr SPELL_BANISH/SPELL_TAP_CHECK arm has no cross-
-- creature bridge — skipped; !_isBanished -> _isBanished=true,
-- triggered self-cast banish 44836, cancel all timers, re-arm only
-- the check timer 1s (C++ events.Reset + Repeat, C++-exact). The
-- ACTION_START_OUTRO arm (schedules EVENT_OUTRO_START 1s) has no
-- bearer — it is delivered by sathrovarr's JustDied via cross-
-- creature DoAction, which has no bridge; the outro chain (OUTRO_
-- START/1/2/3, SAY_OUTRO_1/2, gravity/liftoff/MovePoint/faction/
-- RemoveAllAuras/invisible/KillSelf arms) is unmodeled beyond this
-- note. OnDamageTaken(9, pre-damage hook): lethal from anyone but
-- self -> damage rewritten to 0 (C++ damage = 0, C++-exact).
-- OnTargetDied(3): player victim + 50% roll -> Talk(SAY_EVIL_SLAY).
-- OnLeaveCombat(2)/OnReset(23): the EnterEvadeMode instance arms
-- (SendEncounterUnit disengage, DoRemoveAurasDueToSpellOnPlayers
-- spectral-realm, summons.DespawnAll, DespawnPortals, the sathrovarr
-- _DespawnAtEvade relay and the outro-phase early return) have no
-- bridges — modeled as timer/state cleanup only.
-- Kalecgos human (24891). OnReset: say-phase = 1, arm revitalize
-- 45027 5s then 5s (non-triggered self-cast — C++ DoCast default is
-- triggered=false, vaelastrasz convention) and heroic strike 45026
-- 3s then 2s (non-triggered DoCastVictim; nil ticks cast nothing but
-- keep the schedule, jeklik convention). OnDamageTaken: the sath-
-- GUID damage-source arm (damage = 0 from anyone but sathrovarr) has
-- no instance-creature bridge — skipped; health thresholds use the
-- pre-damage health (C++ DamageTaken evaluates HealthBelowPct before
-- the damage is applied): below 75% in phase 1 -> Talk(SAY_GOOD_
-- NEAR_DEATH_0), phase 2; below 50% in phase 2 -> phase 3, Talk(SAY_
-- GOOD_NEAR_DEATH_1); below 10% in phase 3 -> phase 4, Talk(SAY_GOOD_
-- NEAR_DEATH_2). OnDied(4): Talk(SAY_GOOD_DEATH).
-- Sathrovarr (24892). OnEnterCombat: per-GUID reset, Talk(SAY_SATH_
-- AGGRO); shadowbolt 45031 {7s,10s} then {7s,10s}: 20% Talk(SAY_
-- SATH_SPELL1) + DoCastAOE, repeat {7s,10s}. Agony curse 45032 20s:
-- triggered single-target cast on a random alive player in the
-- instance carrying the spectral-realm aura 46021 and not carrying
-- 45032/45034 (C++ CurseAgonySelector, aura-filter arms C++-exact;
-- the NonTankTargetSelector exclusion has no threat bridge —
-- documented); fallback DoCastVictim (nil falls back to the victim
-- on the bridge); the SPELLVALUE_MAX_TARGETS=1 clamp has no bridge
-- but the pick is already single-target. Repeat 20s. Corruption
-- strike 45029 13s: 20% Talk(SAY_SATH_SPELL2) + non-triggered
-- DoCastVictim, repeat 13s. Check timer 1s: health strictly below
-- 10% and !_isEnraged -> _isEnraged=true (the kalecgos ACTION_
-- ENRAGE relay has no cross-creature bridge — skipped); health
-- strictly below 1%: the kalecgos SPELL_BANISH check arm has no
-- cross-creature bridge — skipped; !_isBanished -> _isBanished=true,
-- triggered self-cast banish 44836 (C++-exact; no events.Reset on
-- this side). OnDamageTaken(9): lethal from anyone but self ->
-- damage rewritten to 0 (C++-exact). OnTargetDied(3): player victim
-- -> Talk(SAY_SATH_SLAY); the killed-kalecgos-human evade relay has
-- no bridge — skipped. OnDied(4): Talk(SAY_SATH_DEATH); the
-- DoRemoveAurasDueToSpellOnPlayers arm and the kalecgos ACTION_
-- START_OUTRO relay have no bridges — skipped. OnSpellHit(14): hit
-- by 46733 (tap check damage) -> triggered self-cast teleport back
-- 46020; the Unit::Kill(caster) arm has no kill bridge — skipped.
-- OnLeaveCombat(2)/OnReset(23): the kalecgos EnterEvadeMode relay
-- has no bridge — modeled as timer/state cleanup only.
-- Not registered: go_kalecgos_spectral_rift (GO 187055 — no
-- GameObjectAI bridge); spell_kalecgos_tap_check,
-- spell_kalecgos_spectral_blast, spell_kalecgos_spectral_realm_
-- trigger, spell_kalecgos_spectral_realm_aura and spell_kalecgos_
-- curse_of_boundless_agony (standing SpellScript/AuraScript gaps).
-- Deliberate deviations (documented in the file): the UpdateAI
-- UNIT_STATE_CASTING queue gate and the post-event casting check
-- have no UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention); no instance-script model — boss admission via the
-- luaBossAI shim (BossAI::JustEngagedWith/JustDied/Reset and the
-- DATA_KALECGOS/DATA_KALECGOS_DRAGON/DATA_KALECGOS_HUMAN/DATA_
-- SATHROVARR bookkeeping arms skipped); no cross-creature bridge —
-- the human summon on pull, the sathrovarr GUID cache, the sathrovarr
-- JustDied -> kalecgos ACTION_START_OUTRO relay, the sathrovarr
-- check-timer kalecgos banish/tap-check arms, the kalecgos check-
-- timer sathrovarr banish/tap-check arm, the kalecgos<->sathrovarr
-- ACTION_ENRAGE relay and both evade relays are unmodeled; no
-- movement/faction/react bridge — the outro liftoff/MovePoint/
-- invisible/KillSelf chain, the human engage positioning and the
-- enter-evade flag arms are unmodeled; no threat bridge — the
-- NonTankTargetSelector exclusion in both target filters is
-- unmodeled; the SPELLVALUE_MAX_TARGETS clamp and all SpellScript/
-- AuraScript targeting arms are unmodeled (standing gaps).

local ENTRY_KALECGOS = 24850
local ENTRY_KALECGOS_HUMAN = 24891
local ENTRY_SATHROVARR = 24892

local SAY_EVIL_AGGRO = 0
local SAY_EVIL_SLAY = 1
local EMOTE_ENRAGE = 4
local SAY_ARCANE_BUFFET = 6

local SAY_SATH_AGGRO = 0
local SAY_SATH_SLAY = 1
local SAY_SATH_DEATH = 2
local SAY_SATH_SPELL1 = 3
local SAY_SATH_SPELL2 = 4

local SAY_GOOD_NEAR_DEATH_0 = 0
local SAY_GOOD_NEAR_DEATH_1 = 1
local SAY_GOOD_NEAR_DEATH_2 = 2
local SAY_GOOD_DEATH = 3

local SPELL_SPECTRAL_BLAST = 44869
local SPELL_ARCANE_BUFFET = 45018
local SPELL_FROST_BREATH = 44799
local SPELL_TAIL_LASH = 45122
local SPELL_BANISH = 44836
local SPELL_ENRAGE = 44807
local SPELL_HEROIC_STRIKE = 45026
local SPELL_REVITALIZE = 45027
local SPELL_SHADOW_BOLT = 45031
local SPELL_AGONY_CURSE = 45032
local SPELL_AGONY_CURSE_ALLY = 45034
local SPELL_CORRUPTION_STRIKE = 45029
local SPELL_TAP_CHECK_DAMAGE = 46733
local SPELL_TELEPORT_BACK = 46020
local SPELL_SPECTRAL_REALM_AURA = 46021

local WildMagicSpells = { 45001, 45002, 45004, 45006, 45010, 44978 }

local timers = {}
local kalecgosState = {}
local humanSayPhase = {}
local sathrovarrState = {}

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

-- Alive players sharing the creature's map+instance (teron
-- convention).
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

local function belowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false
    end
    return creature:GetHealth() * 100 / maxHealth < pct
end

-- ================= Kalecgos (24850) =================

-- C++ EVENT_ARCANE_BUFFET: 20% Talk(SAY_ARCANE_BUFFET) +
-- DoCastAOE(45018); repeat 8s.
local function kalecgosArcaneBuffet(creature, guid)
    if math.random(1, 5) == 1 then
        creature:Talk(SAY_ARCANE_BUFFET)
    end
    creature:CastSpell(creature, SPELL_ARCANE_BUFFET)
    schedule(guid, "arcanebuffet", 8000, function()
        kalecgosArcaneBuffet(creature, guid)
    end)
end

-- C++ EVENT_FROST_BREATH: DoCastAOE(44799); repeat 15s.
local function kalecgosFrostBreath(creature, guid)
    creature:CastSpell(creature, SPELL_FROST_BREATH)
    schedule(guid, "frostbreath", 15000, function()
        kalecgosFrostBreath(creature, guid)
    end)
end

-- C++ EVENT_TAIL_LASH: DoCastAOE(45122); first 25s, then 15s.
local function kalecgosTailLash(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    schedule(guid, "taillash", 15000, function()
        kalecgosTailLash(creature, guid)
    end)
end

-- C++ EVENT_WILD_MAGIC: triggered DoCastAOE(one of the six wild
-- magic spells, urand(0, 5)); first 10s, then 20s.
local function kalecgosWildMagic(creature, guid)
    creature:CastSpell(creature, WildMagicSpells[math.random(1, 6)], true)
    schedule(guid, "wildmagic", 20000, function()
        kalecgosWildMagic(creature, guid)
    end)
end

-- C++ EVENT_SPECTRAL_BLAST: triggered DoCastAOE(44869); first
-- {20s,25s}, repeat {20s,25s}.
local function kalecgosSpectralBlast(creature, guid)
    creature:CastSpell(creature, SPELL_SPECTRAL_BLAST, true)
    schedule(guid, "spectralblast", math.random(20000, 25000), function()
        kalecgosSpectralBlast(creature, guid)
    end)
end

-- C++ EVENT_CHECK_TIMER: the ACTION_ENRAGE arm (inlined here),
-- the _isBanished arm with events.Reset, re-arm 1s.
local function kalecgosCheckTimer(creature, guid)
    local state = kalecgosState[guid]
    if state == nil then
        return
    end
    if not state.enraged and belowPct(creature, 10) then
        state.enraged = true
        creature:Talk(EMOTE_ENRAGE)
        creature:CastSpell(creature, SPELL_ENRAGE, true)
    end
    if belowPct(creature, 1) then
        if not state.banished then
            state.banished = true
            creature:CastSpell(creature, SPELL_BANISH, true)
            cancelTimers(guid)
        end
    end
    schedule(guid, "checktimer", 1000, function()
        kalecgosCheckTimer(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_EVIL_AGGRO) + the six initial arms.
-- The human-summon arm has no summon bridge.
local function kalecgosEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kalecgosState[guid] = { enraged = false, banished = false }
    creature:Talk(SAY_EVIL_AGGRO)
    schedule(guid, "arcanebuffet", 8000, function()
        kalecgosArcaneBuffet(creature, guid)
    end)
    schedule(guid, "frostbreath", 15000, function()
        kalecgosFrostBreath(creature, guid)
    end)
    schedule(guid, "wildmagic", 10000, function()
        kalecgosWildMagic(creature, guid)
    end)
    schedule(guid, "taillash", 25000, function()
        kalecgosTailLash(creature, guid)
    end)
    schedule(guid, "spectralblast", math.random(20000, 25000), function()
        kalecgosSpectralBlast(creature, guid)
    end)
    schedule(guid, "checktimer", 1000, function()
        kalecgosCheckTimer(creature, guid)
    end)
end

-- C++ KilledUnit: player victim + 50% roll -> Talk(SAY_EVIL_SLAY).
local function kalecgosTargetDied(event, creature, victim)
    if victim:IsPlayer() and math.random(1, 2) == 1 then
        creature:Talk(SAY_EVIL_SLAY)
    end
end

local function kalecgosResetState(guid)
    cancelTimers(guid)
    kalecgosState[guid] = nil
end

local function kalecgosLeaveCombat(event, creature)
    kalecgosResetState(creature:GetGUID())
end

local function kalecgosDied(event, creature, killer)
    kalecgosResetState(creature:GetGUID())
end

local function kalecgosReset(event, creature)
    kalecgosResetState(creature:GetGUID())
end

-- C++ DamageTaken: lethal from anyone but self -> damage = 0
-- (C++-exact).
local function kalecgosDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health > 0 and damage >= health
            and (attacker == nil or attacker:GetGUID() ~= creature:GetGUID()) then
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_KALECGOS, 1, kalecgosEnterCombat)
RegisterCreatureEvent(ENTRY_KALECGOS, 2, kalecgosLeaveCombat)
RegisterCreatureEvent(ENTRY_KALECGOS, 3, kalecgosTargetDied)
RegisterCreatureEvent(ENTRY_KALECGOS, 4, kalecgosDied)
RegisterCreatureEvent(ENTRY_KALECGOS, 9, kalecgosDamageTaken)
RegisterCreatureEvent(ENTRY_KALECGOS, 23, kalecgosReset)

-- ================= Kalecgos human (24891) =================

-- C++ EVENT_REVITALIZE: non-triggered DoCastSelf(45027); repeat 5s.
local function humanRevitalize(creature, guid)
    creature:CastSpell(creature, SPELL_REVITALIZE)
    schedule(guid, "revitalize", 5000, function()
        humanRevitalize(creature, guid)
    end)
end

-- C++ EVENT_HEROIC_STRIKE: non-triggered DoCastVictim(45026); first
-- 3s, repeat 2s. Nil ticks cast nothing but keep the schedule
-- (jeklik convention).
local function humanHeroicStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_HEROIC_STRIKE)
    end
    schedule(guid, "heroicstrike", 2000, function()
        humanHeroicStrike(creature, guid)
    end)
end

-- C++ JustDied: Talk(SAY_GOOD_DEATH).
local function humanDied(event, creature, killer)
    creature:Talk(SAY_GOOD_DEATH)
    cancelTimers(creature:GetGUID())
    humanSayPhase[creature:GetGUID()] = nil
end

local function humanResetState(guid)
    cancelTimers(guid)
    humanSayPhase[guid] = nil
end

local function humanLeaveCombat(event, creature)
    humanResetState(creature:GetGUID())
end

local function humanReset(event, creature)
    local guid = creature:GetGUID()
    humanResetState(guid)
    humanSayPhase[guid] = 1
end

-- C++ OnEnterCombat: the human engages sathrovarr on spawn (no
-- bridge for the spawn/summon or the cross-creature combat setup) —
-- the fight arms start on pull, like the boss-side convention.
local function humanEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    humanSayPhase[guid] = 1
    schedule(guid, "revitalize", 5000, function()
        humanRevitalize(creature, guid)
    end)
    schedule(guid, "heroicstrike", 3000, function()
        humanHeroicStrike(creature, guid)
    end)
end

-- C++ DamageTaken: the sath-GUID damage-source rewrite has no
-- instance-creature bridge — skipped; the health-threshold Talks use
-- pre-damage health (C++ HealthBelowPct is evaluated before the
-- damage is applied).
local function humanDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local phase = humanSayPhase[guid] or 1
    if belowPct(creature, 75) and phase == 1 then
        creature:Talk(SAY_GOOD_NEAR_DEATH_0)
        humanSayPhase[guid] = 2
    elseif belowPct(creature, 50) and phase == 2 then
        creature:Talk(SAY_GOOD_NEAR_DEATH_1)
        humanSayPhase[guid] = 3
    elseif belowPct(creature, 10) and phase == 3 then
        creature:Talk(SAY_GOOD_NEAR_DEATH_2)
        humanSayPhase[guid] = 4
    end
end

RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 1, humanEnterCombat)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 2, humanLeaveCombat)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 4, humanDied)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 9, humanDamageTaken)
RegisterCreatureEvent(ENTRY_KALECGOS_HUMAN, 23, humanReset)

-- ================= Sathrovarr (24892) =================

-- C++ EVENT_SHADOWBOLT: 20% Talk(SAY_SATH_SPELL1) +
-- DoCastAOE(45031); first {7s,10s}, repeat {7s,10s}.
local function sathrovarrShadowbolt(creature, guid)
    if math.random(1, 5) == 1 then
        creature:Talk(SAY_SATH_SPELL1)
    end
    creature:CastSpell(creature, SPELL_SHADOW_BOLT)
    schedule(guid, "shadowbolt", math.random(7000, 10000), function()
        sathrovarrShadowbolt(creature, guid)
    end)
end

-- C++ EVENT_AGONY_CURSE: triggered DoCast(45032) on a random alive
-- player in the instance carrying the spectral-realm aura and not
-- carrying 45032/45034, else DoCastVictim; repeat 20s. The
-- NonTankTargetSelector exclusion has no threat bridge — unmodeled.
local function sathrovarrAgonyCurse(creature, guid)
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        if p:HasAura(SPELL_SPECTRAL_REALM_AURA)
                and not p:HasAura(SPELL_AGONY_CURSE)
                and not p:HasAura(SPELL_AGONY_CURSE_ALLY) then
            candidates[#candidates + 1] = p
        end
    end
    local target = nil
    if #candidates > 0 then
        target = candidates[math.random(#candidates)]
    else
        target = creature:GetVictim()
    end
    if target then
        creature:CastSpell(target, SPELL_AGONY_CURSE, true)
    end
    schedule(guid, "agonycurse", 20000, function()
        sathrovarrAgonyCurse(creature, guid)
    end)
end

-- C++ EVENT_CORRUPTION_STRIKE: 20% Talk(SAY_SATH_SPELL2) +
-- non-triggered DoCastVictim(45029); repeat 13s.
local function sathrovarrCorruptionStrike(creature, guid)
    if math.random(1, 5) == 1 then
        creature:Talk(SAY_SATH_SPELL2)
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CORRUPTION_STRIKE)
    end
    schedule(guid, "corruptionstrike", 13000, function()
        sathrovarrCorruptionStrike(creature, guid)
    end)
end

-- C++ EVENT_CHECK_TIMER: the _isEnraged arm (the kalecgos
-- ACTION_ENRAGE relay has no cross-creature bridge — skipped), the
-- _isBanished arm (the kalecgos SPELL_BANISH check has no
-- cross-creature bridge — skipped); re-arm 1s.
local function sathrovarrCheckTimer(creature, guid)
    local state = sathrovarrState[guid]
    if state == nil then
        return
    end
    if belowPct(creature, 10) and not state.enraged then
        state.enraged = true
    end
    if belowPct(creature, 1) and not state.banished then
        state.banished = true
        creature:CastSpell(creature, SPELL_BANISH, true)
    end
    schedule(guid, "checktimer", 1000, function()
        sathrovarrCheckTimer(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_SATH_AGGRO) + the four initial arms.
local function sathrovarrEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sathrovarrState[guid] = { enraged = false, banished = false }
    creature:Talk(SAY_SATH_AGGRO)
    schedule(guid, "shadowbolt", math.random(7000, 10000), function()
        sathrovarrShadowbolt(creature, guid)
    end)
    schedule(guid, "agonycurse", 20000, function()
        sathrovarrAgonyCurse(creature, guid)
    end)
    schedule(guid, "corruptionstrike", 13000, function()
        sathrovarrCorruptionStrike(creature, guid)
    end)
    schedule(guid, "checktimer", 1000, function()
        sathrovarrCheckTimer(creature, guid)
    end)
end

-- C++ KilledUnit: player victim -> Talk(SAY_SATH_SLAY). The killed-
-- kalecgos-human evade relay has no bridge — skipped.
local function sathrovarrTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_SATH_SLAY)
    end
end

local function sathrovarrResetState(guid)
    cancelTimers(guid)
    sathrovarrState[guid] = nil
end

-- C++ JustDied: Talk(SAY_SATH_DEATH). The
-- DoRemoveAurasDueToSpellOnPlayers arm and the kalecgos ACTION_
-- START_OUTRO relay have no bridges — skipped.
local function sathrovarrDied(event, creature, killer)
    creature:Talk(SAY_SATH_DEATH)
    sathrovarrResetState(creature:GetGUID())
end

-- C++ EnterEvadeMode: the kalecgos EnterEvadeMode relay has no
-- bridge — modeled as timer/state cleanup only.
local function sathrovarrLeaveCombat(event, creature)
    sathrovarrResetState(creature:GetGUID())
end

local function sathrovarrReset(event, creature)
    sathrovarrResetState(creature:GetGUID())
end

-- C++ DamageTaken: lethal from anyone but self -> damage = 0
-- (C++-exact).
local function sathrovarrDamageTaken(event, creature, attacker, damage)
    local health = creature:GetHealth()
    if health > 0 and damage >= health
            and (attacker == nil or attacker:GetGUID() ~= creature:GetGUID()) then
        return false, 0
    end
end

-- C++ SpellHit: hit by 46733 (tap check damage) -> triggered
-- self-cast teleport back 46020. The Unit::Kill(caster) arm has no
-- kill bridge — skipped.
local function sathrovarrSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_TAP_CHECK_DAMAGE then
        creature:CastSpell(creature, SPELL_TELEPORT_BACK, true)
    end
end

RegisterCreatureEvent(ENTRY_SATHROVARR, 1, sathrovarrEnterCombat)
RegisterCreatureEvent(ENTRY_SATHROVARR, 2, sathrovarrLeaveCombat)
RegisterCreatureEvent(ENTRY_SATHROVARR, 3, sathrovarrTargetDied)
RegisterCreatureEvent(ENTRY_SATHROVARR, 4, sathrovarrDied)
RegisterCreatureEvent(ENTRY_SATHROVARR, 9, sathrovarrDamageTaken)
RegisterCreatureEvent(ENTRY_SATHROVARR, 14, sathrovarrSpellHit)
RegisterCreatureEvent(ENTRY_SATHROVARR, 23, sathrovarrReset)
