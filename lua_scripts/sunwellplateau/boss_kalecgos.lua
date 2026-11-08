-- Kalecgos (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_kalecgos.cpp
-- (boss_kalecgos, boss_kalecgos_human, boss_sathrovarr;
-- go_kalecgos_spectral_rift and the spell_kalecgos_tap_check /
-- spell_kalecgos_spectral_blast / spell_kalecgos_spectral_realm_
-- trigger / spell_kalecgos_spectral_realm_aura (OnRemove PORTED
-- 2026-11-08 tick 425, below; the trigger SpellScript stays out) /
-- spell_kalecgos_curse_of_boundless_agony SpellScript/AuraScript
-- classes documented below, not registered); sunwell_plateau.h:30
-- (DATA_KALECGOS = 0,
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
-- (summon bridge is live) stays out — the human is summoned only
-- to be wired into combat via the paired sathrovarr->SetInCombatWith
-- / kalecgosHuman->SetInCombatWith relay (:218-222, no cross-creature
-- bridge); the human never self-engages (UpdateAI early-returns
-- without a victim, DamageTaken nullifies non-Sathrovarr damage),
-- so a relay-less summon would be an idle invulnerable statue —
-- the human AI is registered on its own entry (DB-side spawn/summon,
-- creature_template binding). Arcane
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
-- creature DoAction, which now ports via grid sweep + 1s outro-start
-- (kalecgosOutroStart below); the OUTRO_1/2/3 movement chain
-- (SAY_OUTRO_1/2, gravity/liftoff/MovePoint/invisible/KillSelf arms)
-- stays out. The encounter-frame disengage leg ports in
-- kalecgosOutroStart below (bridge live 2026-11-05).
-- OnDamageTaken(9, pre-damage hook): lethal from anyone but
-- self -> damage rewritten to 0 (C++ damage = 0, C++-exact).
-- OnTargetDied(3): player victim + 50% roll -> Talk(SAY_EVIL_SLAY).
-- OnLeaveCombat(2)/OnReset(23): the EnterEvadeMode instance arms
-- (SendEncounterUnit disengage — PORTED on ev2 via
-- creature:SendEncounterUnit, bridge live 2026-11-05;
-- DoRemoveAurasDueToSpellOnPlayers(
-- SPELL_SPECTRAL_REALM_AURA 46021, true, true — the
-- includePets/includeControlled legs have no pet/controlled
-- iteration bridge),
-- spectral-realm, summons.DespawnAll, the sathrovarr
-- _DespawnAtEvade relay and the outro-phase early return) have no
-- bridges — modeled as timer/state cleanup only. DespawnPortals IS
-- ported on ev2 (kalecgosLeaveCombat, GetGameObjectsInGrid 250yd +
-- Despawn twin of C++ Delete()); the POINT_OUTRO_2 call stays out
-- with the outro machine.
-- Kalecgos human (24891). OnReset: say-phase = 1, arm revitalize
-- 45027 5s then 5s (non-triggered self-cast — C++ DoCast default is
-- triggered=false, vaelastrasz convention) and heroic strike 45026
-- 3s then 2s (non-triggered DoCastVictim; nil ticks cast nothing but
-- keep the schedule, jeklik convention). OnDamageTaken: the sath-
-- GUID damage-source arm (damage = 0 from anyone but sathrovarr,
-- C++ DamageTaken :375-376) rides attacker:GetEntry() (object.go
-- generic fallback) against ENTRY_SATHROVARR — the single-Sathrovarr
-- entry proxy for the GUID compare; health thresholds use the
-- pre-damage health (C++ DamageTaken evaluates HealthBelowPct before
-- the damage is applied): below 75% in phase 1 -> Talk(SAY_GOOD_
-- NEAR_DEATH_0), phase 2; below 50% in phase 2 -> phase 3, Talk(SAY_
-- GOOD_NEAR_DEATH_1); below 10% in phase 3 -> phase 4, Talk(SAY_GOOD_
-- NEAR_DEATH_2). OnDied(4): Talk(SAY_GOOD_DEATH).
-- Sathrovarr (24892). OnEnterCombat: per-GUID reset, Talk(SAY_SATH_
-- AGGRO); shadowbolt 45031 {7s,10s} then {7s,10s}: 20% Talk(SAY_
-- SATH_SPELL1) + DoCastAOE, repeat {7s,10s}. Agony curse 45032 20s:
-- triggered single-target cast on the current victim: the C++
-- CurseAgonySelector negates NonTankTargetSelector, so only the
-- current victim passes the filter (player + spectral-realm aura
-- 46021 + not carrying 45032/45034); the fallback DoCastVictim
-- targets the same victim, so the arm collapses to a victim cast
-- (the old random-player pick misread the selector negation);
-- the SPELLVALUE_MAX_TARGETS=1 clamp has no bridge but the pick
-- is already single-target. Repeat 20s. Corruption
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
-- DoRemoveAurasDueToSpellOnPlayers player leg
-- (SPELL_SPECTRAL_REALM_AURA 46021) is PORTED via
-- stripSpectralRealmAuras (GetPlayersInWorld + mapId/instanceId filter +
-- player:RemoveAura — the moroes precedent; the
-- includePets/includeControlled legs have no pet/controlled iteration
-- bridge — stay out) and the kalecgos ACTION_
-- START_OUTRO relay has no bridge — skipped. OnSpellHit(14): hit
-- by 46733 (tap check damage) -> triggered self-cast teleport back
-- 46020; the Unit::Kill(unitCaster, me) arm stays out — creature:Kill()
-- is self-killer only, and this arm credits the kill to the caster
-- (player kill chain), which no bridge expresses — skipped.
-- OnLeaveCombat(2)/OnReset(23): the kalecgos EnterEvadeMode relay
-- has no bridge — modeled as timer/state cleanup only.
-- Not registered: go_kalecgos_spectral_rift (GO 187055 — no
-- GameObjectAI bridge); spell_kalecgos_tap_check,
-- spell_kalecgos_spectral_blast, spell_kalecgos_spectral_realm_
-- trigger (standing SpellScript gaps; the spectral_realm_aura
-- OnRemove leg is PORTED below, tick 425).
-- spell_kalecgos_curse_of_boundless_
-- agony's OnPeriodic leg is PORTED below via RegisterAuraEvent
-- (tick 194); its OnRemove leg (ally cast) is PORTED below too
-- (tick 424, RegisterAuraEvent OnRemove, CANCEL gate); its OnApply
-- (instance boss-state gate) and OnEffectUpdatePeriodic (amount
-- doubling) legs are PORTED below too (tick 427, ev2 + the new
-- aura:Remove(mode) / aura:SetAmount bridges).
-- CAST-STATE GATE (ported 2026-11-04): the dragon's own UpdateAI
-- pre-loop gate (boss_kalecgos.cpp:259) + post-event check (:340),
-- the human's own UpdateAI gates (:402/:421), and sathrovarr's
-- BossAI::UpdateAI base gate (ScriptedCreature.cpp:578/:584) gate
-- every UpdateAI tick on UNIT_STATE_CASTING; each Lua tick defers
-- 500ms under its own key via the live creature:IsCasting() bridge
-- (lua_creature_events.go:1950), consuming no re-arm. DamageTaken/
-- TargetDied/Died/SpellHit hooks are not UpdateAI events and stay
-- ungated. Deliberate deviations (documented in the file): no instance-script model — boss admission via the
-- luaBossAI shim (the DATA_KALECGOS legs ARE ported — dragon ev1
-- IN_PROGRESS / ev23 NOT_STARTED / ev4 DONE via base dispatch and
-- sathrovarr ev1 / ev23 / ev4, creature:SetBossState bridge live
-- lua_creature_events.go:5917; only the DATA_KALECGOS_DRAGON/
-- DATA_KALECGOS_HUMAN/DATA_SATHROVARR instance-GUID bookkeeping
-- arms are skipped); no cross-creature bridge —
-- the human summon on pull, the sathrovarr GUID cache, the sathrovarr
-- JustDied -> kalecgos ACTION_START_OUTRO relay, the sathrovarr
-- check-timer kalecgos banish/tap-check arms, the kalecgos check-
-- timer sathrovarr banish/tap-check arm, the kalecgos<->sathrovarr
-- ACTION_ENRAGE relay and both evade relays are unmodeled; no
-- movement/react bridge — the outro liftoff/MovePoint/
-- invisible/KillSelf chain, the human engage positioning and the
-- enter-evade flag arms are unmodeled (the faction leg itself,
-- me->SetFaction(FACTION_FRIENDLY) :319, is now bridgeable via
-- creature:SetFaction (engine/world/lua_creature_events.go:2076),
-- but the chain's bearer is sathrovarr's cross-creature DoAction
-- relay — the OUTRO_START combat-stop legs now port via sweep
-- (kalecgosOutroStart); the movement chain stays out); the agony
-- NonTankTargetSelector filter collapses to the victim (ported
-- above); the spectral-blast selector lives in the unmodeled
-- SpellScript (remove_if polarity, not a SelectTarget pick); the
-- SPELLVALUE_MAX_TARGETS clamp and all SpellScript/
-- AuraScript targeting arms are unmodeled (standing gaps).

local ENTRY_KALECGOS = 24850
local ENTRY_KALECGOS_HUMAN = 24891
local ENTRY_SATHROVARR = 24892
local GO_SPECTRAL_RIFT = 187055

local DATA_KALECGOS = 0 -- sunwell_plateau.h:30
local BOSS_STATE_IN_PROGRESS = 1 -- InstanceScript.h:68-76 (EncounterState)

local SAY_EVIL_AGGRO = 0
local SAY_EVIL_SLAY = 1
local EMOTE_ENRAGE = 4
local REACT_PASSIVE = 0 -- midnight/curator precedent
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
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

local function belowPct(creature, pct)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(maxHealth * pct / 100)
end

-- ================= Kalecgos (24850) =================

-- C++ EVENT_ARCANE_BUFFET: 20% Talk(SAY_ARCANE_BUFFET) +
-- DoCastAOE(45018); repeat 8s.
local function kalecgosArcaneBuffet(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "arcanebuffet", 500, function()
            kalecgosArcaneBuffet(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "frostbreath", 500, function()
            kalecgosFrostBreath(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FROST_BREATH)
    schedule(guid, "frostbreath", 15000, function()
        kalecgosFrostBreath(creature, guid)
    end)
end

-- C++ EVENT_TAIL_LASH: DoCastAOE(45122); first 25s, then 15s.
local function kalecgosTailLash(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "taillash", 500, function()
            kalecgosTailLash(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    schedule(guid, "taillash", 15000, function()
        kalecgosTailLash(creature, guid)
    end)
end

-- C++ EVENT_WILD_MAGIC: triggered DoCastAOE(one of the six wild
-- magic spells, urand(0, 5)); first 10s, then 20s.
local function kalecgosWildMagic(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "wildmagic", 500, function()
            kalecgosWildMagic(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, WildMagicSpells[math.random(1, 6)], true)
    schedule(guid, "wildmagic", 20000, function()
        kalecgosWildMagic(creature, guid)
    end)
end

-- C++ EVENT_SPECTRAL_BLAST: triggered DoCastAOE(44869); first
-- {20s,25s}, repeat {20s,25s}.
local function kalecgosSpectralBlast(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "spectralblast", 500, function()
            kalecgosSpectralBlast(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "checktimer", 500, function()
            kalecgosCheckTimer(creature, guid)
        end)
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
-- The human-summon arm (summon bridge live) stays out — the arm's
-- entire purpose is the unbridged SetInCombatWith relay; summoning
-- without it guts the arm.
local function kalecgosEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    kalecgosState[guid] = { enraged = false, banished = false }
    creature:Talk(SAY_EVIL_AGGRO)
    creature:SendEncounterUnit(0, creature) -- ENCOUNTER_FRAME_ENGAGE (boss_kalecgos.cpp:213; C++ sends before Talk, base-first convention here)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    -- C++ JustEngagedWith (boss_kalecgos.cpp:211-222) calls
    -- BossAI::JustEngagedWith after SendEncounterUnit + Talk;
    -- _JustEngagedWith writes SetBossState(DATA_KALECGOS 0,
    -- IN_PROGRESS) (ScriptedCreature.cpp:537).
    creature:SetBossState(DATA_KALECGOS, 1)
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
    if victim:GetObjectType() == "Player" and math.random(1, 2) == 1 then
        creature:Talk(SAY_EVIL_SLAY)
    end
end

local function kalecgosResetState(guid)
    cancelTimers(guid)
    kalecgosState[guid] = nil
end

local function kalecgosLeaveCombat(event, creature)
    kalecgosResetState(creature:GetGUID())
    -- C++ EnterEvadeMode (boss_kalecgos.cpp:169): disengage before
    -- DespawnPortals; DoRemoveAurasDueToSpellOnPlayers /
    -- summons.DespawnAll / _DespawnAtEvade legs stay out (no bridges).
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    -- C++ EnterEvadeMode's DespawnPortals (boss_kalecgos.cpp:172-186):
    -- every spectral-rift GO within the default 250yd grid scan is
    -- Delete()d; Lua Despawn is the hide-in-instance twin (the
    -- MovementInform POINT_OUTRO_2 DespawnPortals stays out — the
    -- outro MovePoint machine is unported).
    for _, portal in ipairs(creature:GetGameObjectsInGrid(GO_SPECTRAL_RIFT, 250)) do
        portal:Despawn()
    end
end

local function kalecgosDied(event, creature, killer)
    -- No JustDied override on the dragon in C++ — base
    -- BossAI::JustDied runs by virtual dispatch
    -- (ScriptedCreature.h:330); _JustDied() writes
    -- SetBossState(DATA_KALECGOS 0, DONE). The dragon dies only
    -- via the outro KillSelf (MovementInform POINT_OUTRO_2).
    creature:SetBossState(DATA_KALECGOS, 3)
    kalecgosResetState(creature:GetGUID())
end

local function kalecgosReset(event, creature)
    -- C++ Reset (boss_kalecgos.cpp:150-158) calls _Reset(), which
    -- writes SetBossState(DATA_KALECGOS 0, NOT_STARTED), alive-gated
    -- and skipped when already DONE (ScriptedCreature.cpp:499-511);
    -- Go fires ev23 on the death path too, hence the gate.
    if creature:IsAlive() and creature:GetBossState(DATA_KALECGOS) ~= 3 then
        creature:SetBossState(DATA_KALECGOS, 0)
    end
    kalecgosResetState(creature:GetGUID())
end

-- C++ kalecgos DoAction(ACTION_START_OUTRO) (boss_kalecgos.cpp:192-194)
-- schedules EVENT_OUTRO_START 1s; its handler (:311-322) resets events,
-- sets PHASE_OUTRO, stops regen, goes passive, interrupts casts, drops
-- attackers, stops attacks, goes friendly, clears auras, disengages the
-- encounter frame, and schedules OUTRO_1 3s. The encounter-frame leg
-- ports here; the OUTRO_1/2/3 movement chain stays out. The combat-stop legs
-- port here. cancelTimers subsumes the phase reset: every combat timer
-- in this file runs through schedule(), so cancelling them is the
-- PHASE_OUTRO gate. Arm order is C++-exact.
local function kalecgosOutroStart(creature, guid)
    if creature:IsDead() then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "outro_start", 500, function()
            kalecgosOutroStart(creature, guid)
        end)
        return
    end
    cancelTimers(guid)
    creature:SetRegenerateHealth(false)
    creature:SetReactState(REACT_PASSIVE)
    creature:InterruptNonMeleeSpells(true)
    creature:RemoveAllAttackers()
    creature:AttackStop()
    creature:SetFaction(FACTION_FRIENDLY)
    creature:RemoveAllAuras()
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE (boss_kalecgos.cpp:321)
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
    if creature:IsCasting() then
        schedule(guid, "revitalize", 500, function()
            humanRevitalize(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_REVITALIZE)
    schedule(guid, "revitalize", 5000, function()
        humanRevitalize(creature, guid)
    end)
end

-- C++ EVENT_HEROIC_STRIKE: non-triggered DoCastVictim(45026); first
-- 3s, repeat 2s. Nil ticks cast nothing but keep the schedule
-- (jeklik convention).
local function humanHeroicStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "heroicstrike", 500, function()
            humanHeroicStrike(creature, guid)
        end)
        return
    end
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

-- C++ DamageTaken: damage = 0 from anyone but sathrovarr (C++ :375-
-- 376); attacker:GetEntry() (object.go generic fallback) against
-- ENTRY_SATHROVARR is the entry proxy for the sath-GUID compare.
-- The health-threshold Talks use pre-damage health (C++ HealthBelowPct
-- is evaluated before the damage is applied).
local function humanDamageTaken(event, creature, attacker, damage)
    if attacker == nil or attacker:GetEntry() ~= ENTRY_SATHROVARR then
        return false, 0
    end
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
    if creature:IsCasting() then
        schedule(guid, "shadowbolt", 500, function()
            sathrovarrShadowbolt(creature, guid)
        end)
        return
    end
    if math.random(1, 5) == 1 then
        creature:Talk(SAY_SATH_SPELL1)
    end
    creature:CastSpell(creature, SPELL_SHADOW_BOLT)
    schedule(guid, "shadowbolt", math.random(7000, 10000), function()
        sathrovarrShadowbolt(creature, guid)
    end)
end

-- C++ EVENT_AGONY_CURSE: triggered DoCast(45032) on the current
-- victim — CurseAgonySelector negates NonTankTargetSelector, so
-- only the victim passes (player + spectral-realm aura 46021 +
-- not carrying 45032/45034); the else-branch DoCastVictim targets
-- the same victim, so the arm is a victim cast either way;
-- repeat 20s.
local function sathrovarrAgonyCurse(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "agonycurse", 500, function()
            sathrovarrAgonyCurse(creature, guid)
        end)
        return
    end
    local target = creature:GetVictim()
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
    if creature:IsCasting() then
        schedule(guid, "corruptionstrike", 500, function()
            sathrovarrCorruptionStrike(creature, guid)
        end)
        return
    end
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
-- ACTION_ENRAGE relay is PORTED below via the 500yd dragon sweep,
-- the sathrovarrDied precedent), the _isBanished arm (the kalecgos
-- SPELL_BANISH check has no cross-creature bridge — skipped);
-- re-arm 1s.
local function sathrovarrCheckTimer(creature, guid)
    local state = sathrovarrState[guid]
    if state == nil then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "checktimer", 500, function()
            sathrovarrCheckTimer(creature, guid)
        end)
        return
    end
    if belowPct(creature, 10) and not state.enraged then
        state.enraged = true
        -- C++ boss_sathrovarrAI::ExecuteEvent EVENT_CHECK_TIMER
        -- :543-550: kalecgos->AI()->DoAction(ACTION_ENRAGE) — the
        -- dragon's arm (Talk EMOTE_ENRAGE + triggered self-cast
        -- 44807 + _isEnraged latch) inlined here; C++ reaches the
        -- dragon via instance lookup, Lua via the 500yd grid sweep
        -- (sathrovarrDied precedent). Unconditional Talk + cast, as
        -- in C++ (the relay does not check the dragon's latch); the
        -- dragon's own check timer then skips via the shared latch.
        for _, dragon in ipairs(creature:GetCreaturesInGrid(ENTRY_KALECGOS, 500)) do
            local dstate = kalecgosState[dragon:GetGUID()]
            if dstate then
                dstate.enraged = true
            end
            dragon:Talk(EMOTE_ENRAGE)
            dragon:CastSpell(dragon, SPELL_ENRAGE, true)
            break
        end
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
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    -- C++ JustEngagedWith (boss_kalecgos.cpp:465-469) calls
    -- BossAI::JustEngagedWith first, then Talk(SAY_SATH_AGGRO);
    -- _JustEngagedWith writes SetBossState(DATA_KALECGOS 0,
    -- IN_PROGRESS) (ScriptedCreature.cpp:537).
    creature:SetBossState(DATA_KALECGOS, 1)
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
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SATH_SLAY)
    end
end

local function sathrovarrResetState(guid)
    cancelTimers(guid)
    sathrovarrState[guid] = nil
end

-- C++ instance->DoRemoveAurasDueToSpellOnPlayers(
-- SPELL_SPECTRAL_REALM_AURA 46021, true, true) player leg
-- (boss_kalecgos.cpp:511, sathrovarr JustDied): strip the aura from every
-- player sharing the creature's map+instance — GetPlayersInWorld +
-- mapId/instanceId filter + player:RemoveAura (the moroes precedent;
-- C++-exact, no alive gate). The includePets/includeControlled legs have
-- no pet/controlled iteration bridge — stay out.
local function stripSpectralRealmAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_SPECTRAL_REALM_AURA)
        end
    end
end

local function sathrovarrDied(event, creature, killer)
    -- C++ JustDied (boss_kalecgos.cpp:507-516): _JustDied() first —
    -- SetBossState(DATA_KALECGOS 0, DONE) — then Talk, the aura
    -- strip and the outro relay (C++ arm order).
    creature:SetBossState(DATA_KALECGOS, 3)
    creature:Talk(SAY_SATH_DEATH)
    -- C++ JustDied (boss_kalecgos.cpp:507-516): Talk(SAY_SATH_DEATH)
    -- above; DoRemoveAurasDueToSpellOnPlayers(SPELL_SPECTRAL_REALM_AURA 46021,
    -- true, true) — the player leg is PORTED via stripSpectralRealmAuras
    -- (C++ arm order; the kalecgos ACTION_START_OUTRO relay ports
    -- below via grid sweep + 1s outro-start).
    stripSpectralRealmAuras(creature)
    sathrovarrResetState(creature:GetGUID())
    -- C++ sathrovarr JustDied relay (boss_kalecgos.cpp:513-514):
    -- kalecgos DoAction(ACTION_START_OUTRO) -> EVENT_OUTRO_START 1s.
    -- Sweep is the instance-lookup twin (rotface relayProfessor idiom:
    -- first alive match, unique entry; 500yd covers the platform +
    -- spectral area — the bridge filters only map+instance + 2D
    -- distance, no phase filter). The outro movement chain stays out.
    for _, kalecgos in ipairs(creature:GetCreaturesInGrid(ENTRY_KALECGOS, 500)) do
        if kalecgos:IsAlive() then
            local kguid = kalecgos:GetGUID()
            schedule(kguid, "outro_start", 1000, function() kalecgosOutroStart(kalecgos, kguid) end)
            break
        end
    end
end

-- C++ EnterEvadeMode: the kalecgos EnterEvadeMode relay has no
-- bridge — modeled as timer/state cleanup only.
local function sathrovarrLeaveCombat(event, creature)
    sathrovarrResetState(creature:GetGUID())
end

local function sathrovarrReset(event, creature)
    -- C++ Reset (boss_kalecgos.cpp:454-463) calls _Reset(), which
    -- writes SetBossState(DATA_KALECGOS 0, NOT_STARTED), alive-gated
    -- and skipped when already DONE (ScriptedCreature.cpp:499-511);
    -- Go fires ev23 on the death path too, hence the gate.
    if creature:IsAlive() and creature:GetBossState(DATA_KALECGOS) ~= 3 then
        creature:SetBossState(DATA_KALECGOS, 0)
    end
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

-- C++ spell_kalecgos_curse_of_boundless_agony::OnPeriodic
-- (boss_kalecgos.cpp:808-816): EFFECT_0 SPELL_AURA_PERIODIC_
-- DAMAGE. No PreventDefaultAction, so the Go default damage tick
-- still fires. The aura target casts the stage visual on itself,
-- triggered, by tick number: ticks 1-5 visual 1 (45083), ticks
-- 6-10 visual 2 (45084), tick 11+ visual 3 (45085). GetTickNumber
-- is the tick-191 bridge (first tick reads 1).
local SPELL_AGONY_CURSE_VISUAL_1 = 45083
local SPELL_AGONY_CURSE_VISUAL_2 = 45084
local SPELL_AGONY_CURSE_VISUAL_3 = 45085

local function agonyCurseTick(event, aura, target)
    local tick = aura:GetTickNumber()
    local visual = SPELL_AGONY_CURSE_VISUAL_3
    if tick <= 5 then
        visual = SPELL_AGONY_CURSE_VISUAL_1
    elseif tick <= 10 then
        visual = SPELL_AGONY_CURSE_VISUAL_2
    end
    target:CastSpell(target, visual, true)
    -- C++ HandleEffectPeriodicUpdate (boss_kalecgos.cpp:772-776):
    -- tick > 1 and tick % 5 == 1 -> SetAmount(GetAmount() * 2).
    -- PORTED 2026-11-08 (tick 427) via the new aura:SetAmount
    -- bridge (engine/world/lua_aura.go). Ordering: ev1 fires before
    -- the default damage tick (spells.go:41918), so the doubled
    -- amount applies to this tick — C++ runs the update hook around
    -- the periodic effect in the same tick, so at most one tick of
    -- ordering difference.
    if tick > 1 and tick % 5 == 1 then
        aura:SetAmount(aura:GetAmount() * 2)
    end
end

RegisterAuraEvent(SPELL_AGONY_CURSE, 1, agonyCurseTick)

-- C++ spell_kalecgos_curse_of_boundless_agony::OnRemove
-- (boss_kalecgos.cpp:777-785): EFFECT_0 SPELL_AURA_PERIODIC_DAMAGE.
-- On removal, unless the mode is CANCEL, the target casts
-- SPELL_AGONY_CURSE_ALLY 45034 on itself, triggered. Deviation:
-- the C++ CastSpellExtraArgs SPELLVALUE_MAX_TARGETS = 1 mod is
-- dropped — CastSpell has no spell-mod bridge.
local AURA_REMOVE_BY_CANCEL = 2 -- SpellAuraDefines.h:66

local function agonyCurseRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_CANCEL then
        return
    end
    if target ~= nil then
        target:CastSpell(target, SPELL_AGONY_CURSE_ALLY, true)
    end
end

RegisterAuraEvent(SPELL_AGONY_CURSE, 3, agonyCurseRemove)

-- C++ spell_kalecgos_curse_of_boundless_agony::OnApply
-- (boss_kalecgos.cpp:753-760): AfterEffectApply, EFFECT_0
-- SPELL_AURA_PERIODIC_DAMAGE. If the instance boss state for
-- DATA_KALECGOS is not IN_PROGRESS, the aura is removed with
-- AURA_REMOVE_BY_CANCEL. PORTED 2026-11-08 (tick 427) via ev2
-- (GetBossState is bridged, lua_creature_events.go:6195, reached
-- via aura:GetCaster()) + the new aura:Remove(mode) bridge
-- (engine/world/lua_aura.go), which carries mode 2 so the ev3
-- handler above sees the CANCEL guard — a mode-blind Remove()
-- would wrongly fire the 45034 ally-cast here.
local function agonyCurseApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if caster:GetBossState(DATA_KALECGOS) ~= BOSS_STATE_IN_PROGRESS then
        aura:Remove(AURA_REMOVE_BY_CANCEL)
    end
end

RegisterAuraEvent(SPELL_AGONY_CURSE, 2, agonyCurseApply)

-- C++ spell_kalecgos_spectral_realm_aura::OnRemove
-- (boss_kalecgos.cpp:724-730): AfterEffectRemove, EFFECT_0
-- SPELL_AURA_MOD_INVISIBILITY_DETECT, AURA_EFFECT_HANDLE_REAL, any
-- remove mode (no mode guard in C++). On removal the target drops
-- the spectral-realm reaction aura, then self-casts teleport back
-- (46020) and spectral exhaustion (44867), all triggered, C++ order.
-- PORTED 2026-11-08 (tick 425). DORMANT-BY-CONSTRUCTION (partly):
-- the sender, spell_kalecgos_spectral_realm_trigger
-- (boss_kalecgos.cpp:680-707, casting 46019/46021/44845/44852 on the
-- hit unit), is an unported SpellScript, so the 46021 aura currently
-- never lands in Go. The handler goes live the moment the trigger
-- ports — and it ALSO fires today through the sathrovarr JustDied
-- strip: stripSpectralRealmAuras calls player:RemoveAura(46021), and
-- the Go session remove path fires fireAuraRemove (spells.go:35238),
-- so any player carrying 46021 gets the teleport-back + exhaustion
-- legs on Sathrovarr's death, exactly as C++
-- RemoveAurasDueToSpellOnPlayers would.
-- Deviation (pre-existing, same as the zone_zuldrak.lua header note):
-- player:CastSpell ignores the triggered flag (session path), so the
-- player's 46020/44867 casts land non-triggered.
local SPELL_SPECTRAL_REALM_REACTION = 44852 -- boss_kalecgos.cpp:87
local SPELL_SPECTRAL_EXHAUSTION = 44867 -- boss_kalecgos.cpp:88
-- SPELL_TELEPORT_BACK = 46020 already defined above;
-- SPELL_SPECTRAL_REALM_AURA = 46021 already defined above.

local function spectralRealmRemove(event, aura, target)
    if target == nil then
        return
    end
    target:RemoveAura(SPELL_SPECTRAL_REALM_REACTION)
    target:CastSpell(target, SPELL_TELEPORT_BACK, true)
    target:CastSpell(target, SPELL_SPECTRAL_EXHAUSTION, true)
end

RegisterAuraEvent(SPELL_SPECTRAL_REALM_AURA, 3, spectralRealmRemove)
