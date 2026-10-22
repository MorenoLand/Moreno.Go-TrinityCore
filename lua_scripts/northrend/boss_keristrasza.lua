-- Keristrasza (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_keristrasza.cpp
-- (boss_keristrasza (CreatureScript) via GetNexusAI<boss_keristraszaAI>
-- (BossAI, DATA_KERISTRASZA); containment_sphere (GameObjectScript) via
-- GetNexusAI<containment_sphereAI> (GameObjectAI); achievement_intense_
-- cold (AchievementCriteriaScript); spell_intense_cold (SpellScriptLoader);
-- AddSC_boss_keristrasza() at end registers all four; loader decl 81 /
-- call 276 per northrend_script_loader.cpp — the FIFTH group of the
-- "// The Nexus: Nexus" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_ormorok() (decl 80 / call 275); the call after it is
-- AddSC_instance_nexus() (decl 82 / call 277) — loader order confirmed
-- this run; the checkpoint sequence (ormorok -> keristrasza) is followed).
-- Entry: 26723 Keristrasza (nexus.h NPC_KERISTRASZA, line 44;
-- instance_nexus.cpp OnCreatureCreate binds case NPC_KERISTRASZA (line
-- 53) — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_keristrasza"
-- hits boss_keristrasza.cpp + the loader decl/call lines only; grep for
-- "containment_sphere" / "achievement_intense_cold" / "spell_intense_
-- cold" hits boss_keristrasza.cpp only (registered from inside
-- AddSC_boss_keristrasza, no separate loader lines); zero sql/ hits. No
-- keristrasza lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent (maiden_of_virtue precedent); melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) + DoCastAOE(SPELL_INTENSE_COLD
-- 48094) 2-arg (non-triggered) one-shot as creature:CastSpell(creature,
-- spell) (event 1; boss_ai.go DoCastAOE self-cast convention —
-- selin_fireheart / mr_smite precedent; the Oct-12 "no DoCastAOE
-- bridge" claim was wrong, retracted in a prior run) + the three
-- events.ScheduleEvent legs via CreateLuaEvent timers (maiden_
-- of_virtue precedent; the Oct-12 "no timer-event bridge" claim was
-- wrong, retracted this run): EVENT_CRYSTAL_FIRE_BREATH DoCastVictim
-- (48096) non-triggered, 14s init / 14s repeat; EVENT_CRYSTAL_CHAINS_
-- CRYSTALLIZE DoCast(me, SPELL_TAIL_SWEEP 50155) non-triggered
-- self-cast, DUNGEON_MODE(30s,11s) -> 30s normal init, 5s repeat
-- (ymiron normal-values precedent); EVENT_TAIL_SWEEP Talk(SAY_
-- CRYSTAL_NOVA 4) + normal SelectTarget(Random,0,100.0f,true) ->
-- DoCast(target, SPELL_CRYSTAL_CHAINS 50997) non-triggered, 5s
-- init / 30s normal repeat; heroic leg DoCast(me, SPELL_
-- CRYSTALLIZE 48179) 2-arg (non-triggered) self-cast when
-- creature:IsHeroic() (the "no IsHeroic bridge" claim was stale —
-- IsHeroic is bridged, engine/world/lua_creature_events.go:260;
-- lavanthor precedent), 11s heroic repeat (DUNGEON_MODE(30s,11s)).
-- The BossAI::JustEngagedWith instance-bookkeeping leg has no
-- bridge — tharon_ja precedent.
-- DamageTaken (event 9) — HealthBelowPctDamaged(25, damage)
-- one-shot latch (per-GUID, reset on 1/2/23; loken convention)
-- -> Talk(SAY_ENRAGE 2) + Talk(SAY_FRENZY 5) + DoCast(me,
-- SPELL_ENRAGE 8599) 2-arg non-triggered self-cast (the Oct-12
-- "no health-pct bridge" claim was wrong, retracted this run).
-- JustDied — Talk(SAY_DEATH 3) (event 4; the _JustDied bookkeeping has
-- no bridge — tharon_ja precedent).
-- KilledUnit — Talk(SAY_SLAY 1) (event 3 — C++-GATED: who->
-- GetTypeId() == TYPEID_PLAYER; the nalorakk / kelthuzad player-gate
-- variant — seventh ported variant, fifth identical to nalorakk /
-- kelthuzad / gothik / thaddius).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (_enrage=false, _intenseCold=true) + _intense_
-- ColdList.clear() + RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED) +
-- RemovePrison(CheckContainmentSpheres()) + _Reset() — the STUNNED
-- RemoveFlag is unconditional and PORTED below on event 23 (flag
-- bridges are live — the Oct-12 "no flag bridge" wording was stale);
-- Initialize() rides the enraged-latch reset, _intenseColdList.clear()
-- has no achievement bridge, and RemovePrison(CheckContainmentSpheres())
-- + _Reset() stay out (instance / GO-state gates — the
-- containment-sphere prison machine is documented-only; see
-- containment_sphere below).
-- (the Oct-12 "no timer-event / random-target / cast bridge" and
-- "no IsHeroic bridge" claims were wrong, retracted — the heroic
-- crystallize leg is ported above.)
-- SetGUID(DATA_INTENSE_COLD) — _intenseColdList push, consumed only by
-- achievement_intense_cold's OnCheck — no SetGUID / GetData bridge and
-- no achievement bridge.
-- containment_sphere — GameObjectScript: OnGossipHello — keristrasza
-- via instance->GetGuidData(DATA_KERISTRASZA) -> alive: SetFlag(GAME_
-- OBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE) + SetGoState(GO_STATE_ACTIVE)
-- + cross-AI CheckContainmentSpheres(true) (instance GetGuidData
-- ANOMALUS/ORMOROK/TELESTRAS_CONTAINMENT_SPHERE -> ObjectAccessor::
-- GetGameObject + GO_STATE_ACTIVE gate) -> RemovePrison(true)
-- (SetImmuneToPC(false) + RemoveFlag(NON_ATTACKABLE) +
-- RemoveAurasDueToSpell(FROZEN_PRISON 47854)) — joins the instance-
-- model-blocked gossip queue.
-- spell_intense_cold — AuraScript HandlePeriodicTick (EFFECT_1
-- SPELL_AURA_PERIODIC_DAMAGE): stack < 2 early-return; caster AI ->
-- SetGUID(target GUID, DATA_INTENSE_COLD) (caster should be the boss
-- but is the player in practice — the C++ @todo) — no AuraScript
-- bridge.
-- achievement_intense_cold — OnCheck: ENSURE_AI(boss_keristraszaAI)->
-- _intenseColdList not containing the player's GUID — no achievement
-- bridge (kelthuzad/thaddius precedent).

local ENTRY_KERISTRASZA = 26723

local SAY_AGGRO = 0
local SAY_SLAY  = 1
local SAY_ENRAGE = 2
local SAY_DEATH = 3
local SAY_CRYSTAL_NOVA = 4
local SAY_FRENZY = 5

local SPELL_INTENSE_COLD = 48094 -- boss_keristrasza.cpp:38
local SPELL_TAIL_SWEEP = 50155 -- boss_keristrasza.cpp:33
local SPELL_CRYSTAL_CHAINS = 50997 -- boss_keristrasza.cpp:34
local SPELL_ENRAGE = 8599 -- boss_keristrasza.cpp:35
local SPELL_CRYSTALFIRE_BREATH = 48096 -- boss_keristrasza.cpp:36
local SPELL_CRYSTALLIZE = 48179 -- boss_keristrasza.cpp:37

local timers = {}
local keristraszaEnraged = {}

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

-- maiden_of_virtue precedent: random alive player in the instance
-- within range (the C++ SelectTarget(Random, 0, range, true)
-- eligibility gate, playerOnly = true).
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

-- C++ EVENT_CRYSTAL_FIRE_BREATH (cpp:185-188): DoCastVictim(48096)
-- 1-arg (non-triggered), 14s init / 14s repeat.
local function crystalFireBreathTick(creature, guid)
    creature:CastSpell(nil, SPELL_CRYSTALFIRE_BREATH)
    schedule(guid, "crystalfirebreath", 14000, function()
        crystalFireBreathTick(creature, guid)
    end)
end

-- C++ EVENT_CRYSTAL_CHAINS_CRYSTALLIZE (cpp:189-192): DoCast(me,
-- 50155) 2-arg (non-triggered) self-cast, DUNGEON_MODE(30s,11s) ->
-- 30s normal init, 5s repeat.
local function crystalChainsCrystallizeTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_SWEEP)
    schedule(guid, "crystalchainscrystallize", 5000, function()
        crystalChainsCrystallizeTick(creature, guid)
    end)
end

-- C++ EVENT_TAIL_SWEEP (cpp:193-200): Talk(SAY_CRYSTAL_NOVA 4)
-- unconditional; if IsHeroic() DoCast(me, SPELL_CRYSTALLIZE 48179)
-- 2-arg (non-triggered) self-cast, else SelectTarget(Random,0,100.0f,
-- true) -> DoCast(target, 50997) 2-arg (non-triggered); 5s init /
-- 30s normal repeat, 11s heroic repeat (DUNGEON_MODE(30s,11s)).
-- creature:IsHeroic() is bridged (engine/world/lua_creature_events.go
-- :203; lavanthor precedent).
local function tailSweepTick(creature, guid)
    creature:Talk(SAY_CRYSTAL_NOVA)
    local heroic = creature:IsHeroic()
    if heroic then
        creature:CastSpell(creature, SPELL_CRYSTALLIZE)
    else
        local target = randomTargetInRange(creature, 100)
        if target then
            creature:CastSpell(target, SPELL_CRYSTAL_CHAINS)
        end
    end
    local repeatMs = heroic and 11000 or 30000
    schedule(guid, "tailsweep", repeatMs, function()
        tailSweepTick(creature, guid)
    end)
end

-- C++ DamageTaken (cpp:160-169): if (!_enrage &&
-- HealthBelowPctDamaged(25, damage)) { Talk(SAY_ENRAGE 2);
-- Talk(SAY_FRENZY 5); DoCast(me, SPELL_ENRAGE 8599) 2-arg
-- (non-triggered); _enrage = true }. The (health - damage)
-- pre-application convention (loken) — event 9 fires
-- pre-application.
local function keristraszaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if keristraszaEnraged[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * 25 / 100) then
        keristraszaEnraged[guid] = true
        creature:Talk(SAY_ENRAGE)
        creature:Talk(SAY_FRENZY)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ JustEngagedWith (:95-103): Talk(SAY_AGGRO) then DoCastAOE(SPELL_
-- INTENSE_COLD 48094) 2-arg (non-triggered) one-shot, then
-- BossAI::JustEngagedWith (instance bookkeeping — tharon_ja
-- precedent — no bridge). DoCastAOE lowers to self-cast
-- creature:CastSpell(creature, spell) (boss_ai.go convention —
-- selin_fireheart / mr_smite precedent; the Oct-12 "no DoCastAOE
-- bridge" claim was wrong, retracted in a prior run). The three
-- ScheduleEvent legs (cpp:101-103) ride CreateLuaEvent timers
-- (maiden_of_virtue precedent). The AuraScript (spell_intense_cold)
-- periodic SetGUID leg has no bridge — documented below, not wired.
local function keristraszaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    keristraszaEnraged[guid] = false
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_INTENSE_COLD)
    -- C++ JustEngagedWith (cpp:101-103): the three ScheduleEvent
    -- legs ride CreateLuaEvent timers (maiden precedent).
    schedule(guid, "crystalfirebreath", 14000, function()
        crystalFireBreathTick(creature, guid)
    end)
    -- C++ JustEngagedWith schedules EVENT_CRYSTAL_CHAINS_CRYSTALLIZE
    -- with DUNGEON_MODE(30s, 11s) (cpp:102) — heroic fires 19s sooner.
    schedule(guid, "crystalchainscrystallize", creature:IsHeroic() and 11000 or 30000, function()
        crystalChainsCrystallizeTick(creature, guid)
    end)
    schedule(guid, "tailsweep", 5000, function()
        tailSweepTick(creature, guid)
    end)
end

-- C++ Reset/Initialize: _enrage = false (cpp:78) + timers gone
-- with the events — cancel timers, drop the latch.
local function keristraszaLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    keristraszaEnraged[guid] = false
end

local function keristraszaReset(event, creature)
    keristraszaLeaveCombat(event, creature)
    -- C++ Reset (line 89): RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
    -- is unconditional — ported (the containment-sphere prison machine
    -- around it stays documented-only, instance/GO-gated).
    creature:RemoveFlag(59, 0x40000)
end

-- C++ JustDied: Talk(SAY_DEATH) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent).
local function keristraszaDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    keristraszaEnraged[guid] = false
    creature:Talk(SAY_DEATH)
end

-- C++ KilledUnit: Talk(SAY_SLAY) only when who->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function keristraszaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_KERISTRASZA, 1, keristraszaEnterCombat)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 2, keristraszaLeaveCombat)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 3, keristraszaTargetDied)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 4, keristraszaDied)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 9, keristraszaDamageTaken)
RegisterCreatureEvent(ENTRY_KERISTRASZA, 23, keristraszaReset)
