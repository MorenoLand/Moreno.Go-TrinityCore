-- Illidari Council (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_illidari_council.cpp
-- (boss_illidari_council, boss_gathios_the_shatterer,
-- boss_high_nethermancer_zerevor, boss_lady_malande,
-- boss_veras_darkshadow; the npc_veras_vanish_effect AI and the
-- ten spell/aura scripts documented below, not registered);
-- black_temple.h:38 (DATA_ILLIDARI_COUNCIL = 7, eighth boss), :47-
-- :50 (DATA_GATHIOS_THE_SHATTERER = 14, DATA_HIGH_NETHERMANCER_
-- ZEREVOR = 15, DATA_LADY_MALANDE = 16, DATA_VERAS_DARKSHADOW =
-- 17), :78 (NPC_ILLIDARI_COUNCIL = 23426), :82-:85 (NPC_GATHIOS_
-- THE_SHATTERER = 22949, NPC_HIGH_NETHERMANCER_ZEREVOR = 22950,
-- NPC_LADY_MALANDE = 22951, NPC_VERAS_DARKSHADOW = 22952).
-- Creature entries: 23426 Illidari Council (C++ ScriptName
-- "boss_illidari_council"), 22949 Gathios the Shatterer (C++
-- ScriptName "boss_gathios_the_shatterer"), 22950 High
-- Nethermancer Zerevor (C++ ScriptName
-- "boss_high_nethermancer_zerevor"), 22951 Lady Malande (C++
-- ScriptName "boss_lady_malande"), 22952 Veras Darkshadow (C++
-- ScriptName "boss_veras_darkshadow"), all per
-- RegisterBlackTempleCreatureAI in AddSC_boss_illidari_council;
-- the creature_template ScriptName bindings are DB-side — no TDB
-- in this workspace, so only the C++-side naming is verified.
-- Talk lines used: SAY_COUNCIL_AGRO=0 (pull, random member),
-- SAY_COUNCIL_ENRAGE=1 (berserk), SAY_COUNCIL_SPECIAL=2 (special),
-- SAY_COUNCIL_SLAY=3 (kill, player-only), SAY_COUNCIL_COMNT=4
-- (random comment), SAY_COUNCIL_DEATH=5 (death).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (returns false,
-- newDamage; the boolean is consumed by the engine, the number
-- rewrites damage — reliquary convention), 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): Illidari Council
-- (23426): OnEnterCombat: _inCombat once-guard in per-GUID state
-- (cleared on Reset — C++-exact), triggered self-cast empyreal
-- balance 41499 (C++ DoCastSelf(..., true)) / empyreal
-- equivalency 41333 2s then 2s, triggered self-cast / berserk
-- 45078 15min, one-shot — the per-member triggered casts and the
-- Talk(SAY_COUNCIL_ENRAGE) arms target the four other members and
-- have no cross-creature bridge, so the arm fires empty
-- (documented); the random-member Talk(SAY_COUNCIL_AGRO) has no
-- cross-creature bridge — skipped. The SendEncounterUnit engage/
-- disengage, DoZoneInCombat, SetBoundary and SummonCreatureGroup
-- arms have no instance/summon bridges. OnDied: the four members'
-- LowerPlayerDamageReq + quiet-suicide 3617 casts have no cross-
-- creature bridge — skipped; the SetBossState(DONE) arm is
-- blocked on the instance-script model. OnLeaveCombat/OnReset:
-- cancel timers, drop per-GUID state; the summons.DespawnAll and
-- _DespawnAtEvade arms have no bridges (evade-side cleanup is
-- engine-side).
-- Gathios the Shatterer (22949): OnReset: triggered self-cast
-- balance of power 41341 (the SetCombatPulseDelay(0) arm has no
-- bridge). OnEnterCombat (the SetCombatPulseDelay(5), setActive
-- and DoZoneInCombat arms have no bridges): triggered self-cast
-- seal of blood 41459 (C++ ScheduleEvents one-time arm) / bless
-- {41450, 41451} 20s then {30s,45s} — the random-friendly-in-
-- 100-yd scan has no friendly enumeration bridge, so the cast is
-- skipped and only the cycle kept (hexlord friendly-scan
-- convention) / consecration 41541 10s then {30s,35s},
-- non-triggered self-cast (C++ DoCast default is triggered=false
-- — vaelastrasz convention) / hammer of justice 41468 10s then
-- 20s, non-triggered on a random alive player 10-40 yd excluding
-- the current victim (C++ HammerTargetSelector + position 1;
-- nil pick casts nothing but keeps the schedule, jeklik
-- convention) / judgement 41467 15s then 15s, non-triggered
-- DoCastVictim (nil falls back to the victim on the bridge; nil
-- ticks cast nothing but keep the schedule) / aura {41453
-- chromatic, 41452 devotion} 6s then 30s, non-triggered
-- self-cast. OnDamageTaken(9): the IllidariCouncilBossAI shared-
-- rule arm — lethal damage from anyone but self is rewritten to
-- health-1 (C++-exact). OnTargetDied(3): Talk(SAY_COUNCIL_SLAY),
-- TYPEID_PLAYER gate (terestian convention); the 30% random-
-- other-member Talk(SAY_COUNCIL_COMNT) has no cross-creature
-- bridge — skipped. OnDied(4): Talk(SAY_COUNCIL_DEATH).
-- OnLeaveCombat(2)/OnReset(23): cancel timers.
-- High Nethermancer Zerevor (22950): OnReset: triggered self-cast
-- 41341 + non-triggered self-cast dampen magic 41478, _canUse
-- ArcaneExplosion=true in per-GUID state. OnEnterCombat: one-time
-- non-triggered self-cast dampen magic 41478 (C++ ScheduleEvents
-- one-time arm — the recurring EVENT_DAMPEN_MAGIC only ever
-- arms from DoAction(ACTION_REFRESH_DAMPEN), which has no bearer:
-- the dampen-magic AuraScript that calls it has no bridge) /
-- flamestrike 41481 8s then 40s, non-triggered on a random alive
-- player in the instance (C++ SelectTarget(Random, 0) is
-- player-only — teron convention) + Talk(SAY_COUNCIL_SPECIAL) /
-- blizzard 41482 25s then {15s,40s}, same targeting, non-
-- triggered / arcane explosion 41524 5s then 1s gated check: if
-- _canUseArcaneExplosion and a random alive player is within 10
-- yd -> non-triggered self-cast 41524, clear the gate, arm the
-- 5s re-arm (EVENT_ARCANE_EXPLOSION_CHECK); the gate arm has no
-- bridge target scan — playersInInstance supplies it; the 1s
-- cycle re-arms unconditionally (C++-exact). Nil-target ticks
-- cast nothing but keep the schedule (jeklik convention). The
-- DoSpellAttackIfReady(41483 arcane bolt) filler has no spell-
-- attack bridge — unmodeled. DamageTaken/KilledUnit/Died/
-- LeaveCombat/Reset as Gathios.
-- Lady Malande (22951): OnEnterCombat: circle of healing 41455
-- 20s then {20s,35s}, non-triggered self-cast / reflective
-- shield 41475 25s then 40s, non-triggered self-cast +
-- Talk(SAY_COUNCIL_SPECIAL) / divine wrath 41472 32s then 20s,
-- non-triggered DoCastVictim. The HealReceived -> shared-rule
-- 41342 arm (with -addhealth BP0 on nil target) has no heal hook
-- bridge — unmodeled. The DoSpellAttackIfReady(41471 empowered
-- smite) filler has no spell-attack bridge — unmodeled.
-- DamageTaken/KilledUnit/Died/LeaveCombat/Reset as Gathios.
-- Veras Darkshadow (22952): OnEnterCombat: vanish 18s then 60s:
-- Talk(SAY_COUNCIL_SPECIAL), non-triggered self-cast vanish
-- 41476 + non-triggered self-cast deadly strike 41480
-- (C++-exact). The CanSeeAlways vanish-vision arm has no bridge
-- — unmodeled. DamageTaken/KilledUnit/Died/LeaveCombat/Reset as
-- Gathios.
-- Not registered (no script bridges, standing gaps): the four
-- member AIs share the IllidariCouncilBossAI base — its
-- EnterEvadeMode relay to the council controller has no
-- cross-creature bridge (evade-side cleanup is engine-side); the
-- npc_veras_vanish_effect AI (triggered self-cast 40031 birth +
-- 41510 envenom dummy 1s later) has no entry constant in the C++
-- tree and its ScriptName binds DB-side (garr firesworn
-- convention); spell scripts: empyreal_balance (41499), empyreal
-- _equivalency (41333), judgement (41467 target primer/choice +
-- Talk on finish), and aura scripts: balance_of_power (41341
-- absorb -> 41342 shared rule), deadly_strike (41480 periodic ->
-- 41485 deadly poison), deadly_poison (41485 remove -> 41487
-- envenom + 41509 visual), vanish (41476 remove -> 41479
-- teleport), reflective_shield (41475 absorb -> 33619 reflect),
-- seal (41469/41459 remove -> the other seal), dampen_magic
-- (41478 remove-by-expire/enemy -> ACTION_REFRESH_DAMPEN DoAction).
-- Deliberate deviations: the UpdateAI UNIT_STATE_CASTING queue
-- gate and the post-event casting check have no UNIT_STATE bridge
-- — timers fire unconditionally (jeklik convention); no
-- instance-script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith/JustDied/Reset, the DATA_ILLIDARI_
-- COUNCIL and DATA_GATHIOS/ZEREVOR/MALANDE/VERAS bookkeeping
-- arms skipped); Talk(SAY_COUNCIL_COMNT) cross-member comments
-- and the controller's Talk-on-random-member arms have no cross-
-- creature bridge — skipped; no creature enumeration bridge —
-- the controller's SendEncounterUnit arms and the member lookup
-- arms (instance->GetCreature(DATA_*)) have no bearer; no summon
-- bridge — SummonCreatureGroup(SUMMON_COUNCIL_GROUP) unmodeled;
-- the 41342 shared-rule Nil-target cast has no heal-hook bearer;
-- the boundary/SetCombatPulseDelay/setActive arms have no
-- bridges.
local ENTRY_ILLIDARI_COUNCIL = 23426
local ENTRY_GATHIOS = 22949
local ENTRY_ZEREVOR = 22950
local ENTRY_MALANDE = 22951
local ENTRY_VERAS = 22952

local SAY_COUNCIL_AGRO = 0
local SAY_COUNCIL_ENRAGE = 1
local SAY_COUNCIL_SPECIAL = 2
local SAY_COUNCIL_SLAY = 3
local SAY_COUNCIL_COMNT = 4
local SAY_COUNCIL_DEATH = 5

local SPELL_EMPYREAL_BALANCE = 41499
local SPELL_EMPYREAL_EQUIVALENCY = 41333
local SPELL_BERSERK = 45078
local SPELL_BALANCE_OF_POWER = 41341
local SPELL_QUIET_SUICIDE = 3617

local SPELL_FLAMESTRIKE = 41481
local SPELL_BLIZZARD = 41482
local SPELL_DAMPEN_MAGIC = 41478
local SPELL_ARCANE_EXPLOSION = 41524

local SPELL_CIRCLE_OF_HEALING = 41455
local SPELL_REFLECTIVE_SHIELD = 41475
local SPELL_DIVINE_WRATH = 41472

local SPELL_BLESS_PROTECTION = 41450
local SPELL_BLESS_SPELL_WARDING = 41451
local SPELL_CONSECRATION = 41541
local SPELL_HAMMER_OF_JUSTICE = 41468
local SPELL_SEAL_OF_BLOOD = 41459
local SPELL_CHROMATIC_AURA = 41453
local SPELL_DEVOTION_AURA = 41452
local SPELL_JUDGEMENT = 41467

local SPELL_VANISH = 41476
local SPELL_DEADLY_STRIKE = 41480

local Blesses = { SPELL_BLESS_PROTECTION, SPELL_BLESS_SPELL_WARDING }
local Auras = { SPELL_CHROMATIC_AURA, SPELL_DEVOTION_AURA }

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

local function bossState(guid)
    local st = state[guid]
    if not st then
        st = {}
        state[guid] = st
    end
    return st
end

-- supremus convention: alive players in the boss's instance.
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

-- teron convention: C++ SelectTarget(Random, 0) is a random alive
-- player in the instance, player-only.
local function randomInstancePlayer(creature)
    local candidates = playersInInstance(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(1, #candidates)]
end

-- C++ HammerTargetSelector: unit within 10-40 yd of the boss;
-- position 1 excludes the current victim (terestian/gurtogg
-- exclusion convention).
local function hammerJusticeTarget(creature)
    local victim = creature:GetVictim()
    local candidates = {}
    for _, p in ipairs(playersInInstance(creature)) do
        local d = creature:GetDistance(p)
        if d >= 10 and d <= 40
                and (not victim or p:GetGUID() ~= victim:GetGUID()) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(1, #candidates)]
end

------------------------------------------------------------------
-- Illidari Council (23426) — the controller.
------------------------------------------------------------------

-- C++ JustEngagedWith: _inCombat once-guard (cleared on Reset),
-- BossAI::JustEngagedWith skipped (no instance-script model);
-- the SendEncounterUnit/DoZoneInCombat/SendEncounterUnit arms have
-- no bridge. Arms: triggered self-cast 41499, the equivalency
-- cycle, the 15min berserk one-shot (its member casts have no
-- cross-creature bridge — the arm fires empty), and the random-
-- member Talk(SAY_COUNCIL_AGRO), skipped for the same reason.
local function councilEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local st = bossState(guid)
    if st.inCombat then
        return
    end
    st.inCombat = true
    creature:CastSpell(creature, SPELL_EMPYREAL_BALANCE, true)
    local function onEquivalency()
        creature:CastSpell(creature, SPELL_EMPYREAL_EQUIVALENCY, true)
        schedule(guid, "council_equivalency", 2000, onEquivalency)
    end
    schedule(guid, "council_equivalency", 2000, onEquivalency)
    -- C++ EVENT_BERSERK: per-member triggered 45078 casts +
    -- Talk(SAY_COUNCIL_ENRAGE) on each of the four members — no
    -- cross-creature bridge, so the one-shot fires empty.
    schedule(guid, "council_berserk", 900000, function() end)
end

local function councilResetState(guid)
    cancelTimers(guid)
    local st = state[guid]
    if st then
        st.inCombat = nil
    end
end

-- C++ JustDied: the members' LowerPlayerDamageReq + triggered
-- quiet-suicide 3617 casts have no cross-creature bridge —
-- skipped; the SetBossState(DONE) arm is blocked on the
-- instance-script model.
local function councilDied(event, creature, killer)
    councilResetState(creature:GetGUID())
end

local function councilLeaveCombat(event, creature)
    councilResetState(creature:GetGUID())
end

-- C++ Reset: _Reset + _inCombat=false + SummonCreatureGroup
-- (no summon bridge).
local function councilReset(event, creature)
    councilResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ILLIDARI_COUNCIL, 1, councilEnterCombat)
RegisterCreatureEvent(ENTRY_ILLIDARI_COUNCIL, 2, councilLeaveCombat)
RegisterCreatureEvent(ENTRY_ILLIDARI_COUNCIL, 4, councilDied)
RegisterCreatureEvent(ENTRY_ILLIDARI_COUNCIL, 23, councilReset)

------------------------------------------------------------------
-- Shared member arms (the IllidariCouncilBossAI base).
------------------------------------------------------------------

-- C++ DamageTaken: damage >= health and the attacker is not self
-- -> damage = health-1 (C++-exact, reliquary return convention).
local function memberDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        local attackerGUID = attacker and attacker:GetGUID() or nil
        if not attackerGUID or attackerGUID ~= creature:GetGUID() then
            return false, creature:GetHealth() - 1
        end
    end
end

-- C++ KilledUnit: player victim -> Talk(SAY_COUNCIL_SLAY). The
-- 30% random-other-member Talk(SAY_COUNCIL_COMNT) has no cross-
-- creature bridge — skipped.
local function memberTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_COUNCIL_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_COUNCIL_DEATH).
local function memberDied(event, creature, killer)
    creature:Talk(SAY_COUNCIL_DEATH)
    cancelTimers(creature:GetGUID())
end

local function memberLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ IllidariCouncilBossAI::Reset: events reset (cancelTimers),
-- triggered self-cast 41341 balance of power (the SetCombatPulse
-- Delay(0) arm has no bridge).
local function memberReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_BALANCE_OF_POWER, true)
end

------------------------------------------------------------------
-- Gathios the Shatterer (22949).
------------------------------------------------------------------

-- C++ EVENT_BLESS: random friendly unit within 100 yd -> random
-- of {41450, 41451}. No friendly enumeration bridge — the cast
-- is skipped and only the {30s,45s} cycle kept (hexlord
-- friendly-scan convention).
local function gathiosBless(creature, guid)
    schedule(guid, "gathios_bless", math.random(30000, 45000), function()
        gathiosBless(creature, guid)
    end)
end

-- C++ EVENT_CONSECRATION: non-triggered DoCastSelf(41541);
-- re-arm {30s,35s}.
local function gathiosConsecration(creature, guid)
    creature:CastSpell(creature, SPELL_CONSECRATION)
    schedule(guid, "gathios_consecration", math.random(30000, 35000), function()
        gathiosConsecration(creature, guid)
    end)
end

-- C++ EVENT_HAMMER_OF_JUSTICE: SelectTarget(Random, 1,
-- HammerTargetSelector) -> non-triggered DoCast(41468); re-arm
-- 20s. Nil pick casts nothing but keeps the schedule (jeklik
-- convention).
local function gathiosHammer(creature, guid)
    local target = hammerJusticeTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_HAMMER_OF_JUSTICE)
    end
    schedule(guid, "gathios_hammer", 20000, function()
        gathiosHammer(creature, guid)
    end)
end

-- C++ EVENT_JUDGEMENT: non-triggered DoCastVictim(41467); the
-- seal/judgement-choice arms live in the unmodeled judgement
-- SpellScript; re-arm 15s.
local function gathiosJudgement(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_JUDGEMENT)
    end
    schedule(guid, "gathios_judgement", 15000, function()
        gathiosJudgement(creature, guid)
    end)
end

-- C++ EVENT_AURA: non-triggered DoCastSelf(RAND(41453, 41452));
-- re-arm 30s.
local function gathiosAura(creature, guid)
    creature:CastSpell(creature, Auras[math.random(1, 2)])
    schedule(guid, "gathios_aura", 30000, function()
        gathiosAura(creature, guid)
    end)
end

-- C++ JustEngagedWith + ScheduleEvents: the SetCombatPulseDelay/
-- setActive/DoZoneInCombat arms have no bridges; triggered
-- self-cast seal of blood 41459 one-time; the five cycle arms.
local function gathiosEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_SEAL_OF_BLOOD, true)
    schedule(guid, "gathios_bless", 20000, function()
        gathiosBless(creature, guid)
    end)
    schedule(guid, "gathios_consecration", 10000, function()
        gathiosConsecration(creature, guid)
    end)
    schedule(guid, "gathios_hammer", 10000, function()
        gathiosHammer(creature, guid)
    end)
    schedule(guid, "gathios_judgement", 15000, function()
        gathiosJudgement(creature, guid)
    end)
    schedule(guid, "gathios_aura", 6000, function()
        gathiosAura(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_GATHIOS, 1, gathiosEnterCombat)
RegisterCreatureEvent(ENTRY_GATHIOS, 2, memberLeaveCombat)
RegisterCreatureEvent(ENTRY_GATHIOS, 3, memberTargetDied)
RegisterCreatureEvent(ENTRY_GATHIOS, 4, memberDied)
RegisterCreatureEvent(ENTRY_GATHIOS, 9, memberDamageTaken)
RegisterCreatureEvent(ENTRY_GATHIOS, 23, memberReset)

------------------------------------------------------------------
-- High Nethermancer Zerevor (22950).
------------------------------------------------------------------

-- C++ EVENT_FLAMESTRIKE: SelectTarget(Random, 0) -> non-triggered
-- DoCast(41481); Talk(SAY_COUNCIL_SPECIAL); re-arm 40s.
local function zerevorFlamestrike(creature, guid)
    local target = randomInstancePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_FLAMESTRIKE)
    end
    creature:Talk(SAY_COUNCIL_SPECIAL)
    schedule(guid, "zerevor_flamestrike", 40000, function()
        zerevorFlamestrike(creature, guid)
    end)
end

-- C++ EVENT_BLIZZARD: SelectTarget(Random, 0) -> non-triggered
-- DoCast(41482); re-arm {15s,40s}.
local function zerevorBlizzard(creature, guid)
    local target = randomInstancePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_BLIZZARD)
    end
    schedule(guid, "zerevor_blizzard", math.random(15000, 40000), function()
        zerevorBlizzard(creature, guid)
    end)
end

-- C++ EVENT_ARCANE_EXPLOSION: if _canUseArcaneExplosion and a
-- random alive player is within 10 yd -> non-triggered
-- DoCastSelf(41524), clear the gate, arm the 5s re-arm (EVENT_
-- ARCANE_EXPLOSION_CHECK); the 1s cycle re-arms unconditionally.
local function zerevorArcaneExplosion(creature, guid)
    local st = bossState(guid)
    local target = randomInstancePlayer(creature)
    if st.canUseArcaneExplosion and target and creature:GetDistance(target) <= 10 then
        creature:CastSpell(creature, SPELL_ARCANE_EXPLOSION)
        st.canUseArcaneExplosion = false
        schedule(guid, "zerevor_ae_check", 5000, function()
            bossState(guid).canUseArcaneExplosion = true
        end)
    end
    schedule(guid, "zerevor_ae", 1000, function()
        zerevorArcaneExplosion(creature, guid)
    end)
end

-- C++ JustEngagedWith + ScheduleEvents: one-time non-triggered
-- self-cast dampen magic 41478; the three cycle arms. The
-- recurring EVENT_DAMPEN_MAGIC only arms from DoAction(ACTION_
-- REFRESH_DAMPEN), which has no bearer (the dampen-magic
-- AuraScript has no bridge).
local function zerevorEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bossState(guid).canUseArcaneExplosion = true
    creature:CastSpell(creature, SPELL_DAMPEN_MAGIC)
    schedule(guid, "zerevor_flamestrike", 8000, function()
        zerevorFlamestrike(creature, guid)
    end)
    schedule(guid, "zerevor_blizzard", 25000, function()
        zerevorBlizzard(creature, guid)
    end)
    schedule(guid, "zerevor_ae", 5000, function()
        zerevorArcaneExplosion(creature, guid)
    end)
end

-- C++ Reset: base Reset + _canUseArcaneExplosion=true +
-- non-triggered self-cast 41478.
local function zerevorReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bossState(guid).canUseArcaneExplosion = true
    creature:CastSpell(creature, SPELL_BALANCE_OF_POWER, true)
    creature:CastSpell(creature, SPELL_DAMPEN_MAGIC)
end

RegisterCreatureEvent(ENTRY_ZEREVOR, 1, zerevorEnterCombat)
RegisterCreatureEvent(ENTRY_ZEREVOR, 2, memberLeaveCombat)
RegisterCreatureEvent(ENTRY_ZEREVOR, 3, memberTargetDied)
RegisterCreatureEvent(ENTRY_ZEREVOR, 4, memberDied)
RegisterCreatureEvent(ENTRY_ZEREVOR, 9, memberDamageTaken)
RegisterCreatureEvent(ENTRY_ZEREVOR, 23, zerevorReset)

------------------------------------------------------------------
-- Lady Malande (22951).
------------------------------------------------------------------

-- C++ EVENT_CIRCLE_OF_HEALING: non-triggered DoCastSelf(41455);
-- re-arm {20s,35s}.
local function malandeCircle(creature, guid)
    creature:CastSpell(creature, SPELL_CIRCLE_OF_HEALING)
    schedule(guid, "malande_circle", math.random(20000, 35000), function()
        malandeCircle(creature, guid)
    end)
end

-- C++ EVENT_REFLECTIVE_SHIELD: non-triggered DoCastSelf(41475) +
-- Talk(SAY_COUNCIL_SPECIAL); re-arm 40s. The reflect arm lives
-- in the unmodeled AuraScript.
local function malandeShield(creature, guid)
    creature:CastSpell(creature, SPELL_REFLECTIVE_SHIELD)
    creature:Talk(SAY_COUNCIL_SPECIAL)
    schedule(guid, "malande_shield", 40000, function()
        malandeShield(creature, guid)
    end)
end

-- C++ EVENT_DIVINE_WRATH: non-triggered DoCastVictim(41472);
-- re-arm 20s.
local function malandeWrath(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DIVINE_WRATH)
    end
    schedule(guid, "malande_wrath", 20000, function()
        malandeWrath(creature, guid)
    end)
end

-- C++ JustEngagedWith + ScheduleEvents: the three cycle arms.
-- The HealReceived -> shared-rule arm and the DoSpellAttackIfReady
-- (41471) filler have no bridges — unmodeled.
local function malandeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "malande_circle", 20000, function()
        malandeCircle(creature, guid)
    end)
    schedule(guid, "malande_shield", 25000, function()
        malandeShield(creature, guid)
    end)
    schedule(guid, "malande_wrath", 32000, function()
        malandeWrath(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_MALANDE, 1, malandeEnterCombat)
RegisterCreatureEvent(ENTRY_MALANDE, 2, memberLeaveCombat)
RegisterCreatureEvent(ENTRY_MALANDE, 3, memberTargetDied)
RegisterCreatureEvent(ENTRY_MALANDE, 4, memberDied)
RegisterCreatureEvent(ENTRY_MALANDE, 9, memberDamageTaken)
RegisterCreatureEvent(ENTRY_MALANDE, 23, memberReset)

------------------------------------------------------------------
-- Veras Darkshadow (22952).
------------------------------------------------------------------

-- C++ EVENT_VANISH: Talk(SAY_COUNCIL_SPECIAL), non-triggered
-- DoCastSelf(41476 vanish) + non-triggered DoCastSelf(41480
-- deadly strike); re-arm 60s. The vanish-teleport and deadly-
-- strike/poison/envenom arms live in the unmodeled AuraScripts.
local function verasVanish(creature, guid)
    creature:Talk(SAY_COUNCIL_SPECIAL)
    creature:CastSpell(creature, SPELL_VANISH)
    creature:CastSpell(creature, SPELL_DEADLY_STRIKE)
    schedule(guid, "veras_vanish", 60000, function()
        verasVanish(creature, guid)
    end)
end

-- C++ JustEngagedWith + ScheduleEvents: the vanish cycle. The
-- CanSeeAlways vanish-vision arm has no bridge — unmodeled.
local function verasEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "veras_vanish", 18000, function()
        verasVanish(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_VERAS, 1, verasEnterCombat)
RegisterCreatureEvent(ENTRY_VERAS, 2, memberLeaveCombat)
RegisterCreatureEvent(ENTRY_VERAS, 3, memberTargetDied)
RegisterCreatureEvent(ENTRY_VERAS, 4, memberDied)
RegisterCreatureEvent(ENTRY_VERAS, 9, memberDamageTaken)
RegisterCreatureEvent(ENTRY_VERAS, 23, memberReset)
