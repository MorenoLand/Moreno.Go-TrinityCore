-- Baltharus the Warborn (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_baltharus_the_warborn.cpp
-- (368 lines incl. license; 2 CreatureScripts (boss_baltharus_the_warborn
-- (BossAI, DATA_BALTHARUS_THE_WARBORN = 0) +
-- npc_baltharus_the_warborn_clone (BossAI, DATA_BALTHARUS_CLONE = 19));
-- all registered from inside AddSC_boss_baltharus_the_warborn()
-- (line 363); loader decl 190 / call 385 per
-- northrend_script_loader.cpp — the THIRD group of the
-- "// Ruby Sanctum" block in AddNorthrendScripts(), immediately after
-- AddSC_ruby_sanctum() (call 384) — verified from the loader this
-- run; the checkpoint sequence (ruby_sanctum -> baltharus) is
-- followed).
-- Entry: 39751 Baltharus the Warborn (ruby_sanctum.h
-- NPC_BALTHARUS_THE_WARBORN, line 67; instance_ruby_sanctum.cpp
-- creatureData binds NPC_BALTHARUS_THE_WARBORN (line 49)) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_baltharus_the_warborn" hits boss_baltharus_the_warborn.cpp
-- only (+ the loader decl/call lines); this clone carries no sql/ tree,
-- so ScriptName bindings are DB-side by construction.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 5 OnSpawn (boss GUID latch), 9 DamageTaken, 19 JustSummoned,
-- 23 OnReset (boss + clone + xerestrasza).
-- Ported arms (C++-exact for all modeled arms):
-- boss_baltharus_the_warborn JustEngagedWith — InterruptNonMeleeSpells
-- (false) + BossAI base (event 1): SetBossState(DATA_BALTHARUS_THE_WARBORN
-- 0, IN_PROGRESS 1) PORTED this tick (creature:SetBossState, bridge live
-- lua_creature_events.go:5917; base order Interrupt -> SetBossState ->
-- pulse 5 -> DoZoneInCombat -> schedules -> Talk last, C++-exact) +
-- Talk(SAY_AGGRO 1) (the auriaya precedent for Talk; setActive has no
-- bridge, ScheduleTasks is an empty base no-op, ScriptedCreature.h:326;
-- CheckRequiredBosses has no prior-boss gate to enforce here).
-- boss_baltharus_the_warborn JustDied — _JustDied() SetBossState(DONE 3)
-- PORTED this tick (event 4, first arm, moroes precedent) + Talk(SAY_DEATH 4)
-- (the sjonnir precedent); the xerestrasza->AI()->DoAction(
-- ACTION_BALTHARUS_DEATH) cross-AI leg ports in baltharusDied below
-- (sweep + timed Talk chain).
-- boss_baltharus_the_warborn Reset — _Reset() SetBossState(NOT_STARTED 0,
-- != DONE-gated, ScriptedCreature.cpp:499-511) PORTED this tick on ev23
-- (alive-gated: Go fires ev23 on the death path before ev4, moroes/algalon
-- precedent); _cloneCount = 0 rides cloneCount[guid] = nil.
-- boss_baltharus_the_warborn EnterEvadeMode — _DespawnAtEvade()
-- SetBossState(FAIL 2) + DespawnOrUnsummon(0s, 30s) PORTED this tick on
-- ev2 (blood-prince/shahraz ev2 evade-FAIL precedent; summons.DespawnAll
-- rides the clone grid sweep). NO ev24 reach-home FAIL: Baltharus has no
-- JustReachedHome override — BossAI::_JustReachedHome is only
-- setActive(false) (ScriptedCreature.cpp:522-525, no bridge, no state
-- change) — the FAIL leg lives in EnterEvadeMode, not on reach-home
-- (contrast saurfang, whose C++ JustReachedHome sets FAIL explicitly).
-- boss_baltharus_the_warborn KilledUnit — Talk(SAY_KILL 2) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header):
-- boss_baltharus_the_warborn DoAction ACTION_CLONE — ported inline
-- (self-call from the boss's own UpdateAI scheduler: DoCastSelf
-- CLEAR_DEBUFFS 34098 triggered + CLONE 74511 triggered +
-- REPELLING_WAVE 74509 + Talk(SAY_CLONE 3)); the EVENT_CLONE timer
-- rides the DamageTaken 50% latch below.
-- boss_baltharus_the_warborn UpdateAI scheduler — Talk(SAY_BALTHARUS_INTRO
-- 0) rides EVENT_INTRO_TALK, DOCUMENTED-ONLY: the leg rides
-- DoAction(ACTION_INTRO_BALTHARUS) issued by at_baltharus_plateau (area
-- trigger, no bridge to fire it; the DoAction bridge itself exists as
-- ev38, lua_creature_events.go:804 — the issuer is the gap, not the
-- dispatch). EVENT_SUMMONS_ATTACK + the JustSummoned clone-health arm
-- (:136-141: summon->SetHealth(me->GetHealth()) + EVENT_SUMMONS_ATTACK
-- 2s) PORTED this tick via ev19 (baltharusJustSummoned): creature
-- spell-effect summons run creatureEffectSummonType (boss_ai.go:3298) ->
-- summonTempCreature -> motionFor (ev23+ev5 on the summon) + ev19 on the
-- summoner (lua_creature_events.go:190) — the old "clone never
-- Lua-materializes / no direct SummonCreature arm exists" notes are
-- retracted (SummonCreature is bridged, lua_creature_events.go:96).
-- EVENT_CLEAVE / EVENT_BLADE_TEMPEST / EVENT_ENERVATING_BRAND ported
-- 2026-10-13 on the maiden CreateLuaEvent convention (see below);
-- ENERVATING_BRAND target count is raid-moded 2/4 via Is25ManRaid this
-- tick (the "no difficulty bridge" note was stale — the bridge lives at
-- lua_creature_events.go:3651 and this file already uses it in
-- DamageTaken); EVENT_CLONE ported 2026-10-14 via the DamageTaken latch
-- below; the clone's own AI (entry 39899) is registered (see below).
-- npc_baltharus_the_warborn_clone (39899) — zero Talk lines —
-- REGISTERED: the boss's ported SPELL_CLONE 74511 cast spawns it
-- in-engine via the spell-effect summon funnel, so it enters combat with
-- the clone scheduler (:255-264: CLEAVE 11s / BLADE_TEMPEST 15s /
-- ENERVATING_BRAND 10s; arms :296-308 — pure timer legs, no
-- phase/summon/DoAction legs) once the 2s EVENT_SUMMONS_ATTACK
-- DoZoneInCombat fires (baltharusJustSummoned, this tick).
-- Deviation: blade tempest is DoCastVICTIM on the clone (:300-303,
-- victim-cast, NOT the boss's DoCastSelf). The DamageTaken
-- DATA_BALTHARUS_SHARED_HEALTH write (:266-271) and the UpdateAI
-- shared-health read (:285) stay DOCUMENTED-ONLY: instance->SetData /
-- GetData have no bridge (creature:SetData is the per-AI ScriptData
-- twin, lua_creature_events.go:779, not the instance store) and the
-- per-tick UpdateAI read has no Lua bearer. The JustDied
-- Unit::Kill(baltharus) leg (:273-279) is PORTED this tick (cloneDied:
-- boss GUID latched by its ev5 SetGuidData, sjonnir precedent, resolved
-- via GetInstanceCreature + creature:Kill). The clone carries NO
-- SetBossState legs: its Reset (:249-253), JustEngagedWith (:255-264)
-- and JustDied (:273-279) never call the BossAI base arms, and its
-- EnterEvadeMode (:264) is empty.
-- spell_baltharus_enervating_brand_trigger (SpellScript, OnHit
-- HandleSiphonedMight) — PORTED this tick via ev2 (below): the
-- triggering spell is statically 74502, aura:GetCaster() is bridged.
-- All 5 Talk() calls in the file accounted for (SAY_AGGRO 1 +
-- SAY_DEATH 4 + SAY_KILL 2 + SAY_CLONE 3 ported; SAY_BALTHARUS_INTRO 0
-- documented above).

local ENTRY_BALTHARUS_THE_WARBORN = 39751
local ENTRY_BALTHARUS_CLONE = 39899

-- ruby_sanctum.h:31 DATA_BALTHARUS_THE_WARBORN = 0 (the brief's "= 1"
-- is wrong — verified from the header this tick); :52
-- DATA_BALTHARUS_CLONE = 19 (clone AI BossAI id; the clone never calls
-- the base arms, so no SetBossState leg carries it). EncounterState
-- (InstanceScript.h:68-76): NOT_STARTED 0, IN_PROGRESS 1, FAIL 2, DONE 3.
local DATA_BALTHARUS_THE_WARBORN = 0
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

-- UnitDefines.h react states (engine/world/lua_creature_events.go SetReactState bridge).
local REACT_DEFENSIVE = 1

local SAY_AGGRO = 1
local SAY_KILL  = 2
local SAY_CLONE = 3
local SAY_DEATH = 4

local ENTRY_XERESTRASZA = 40429 -- ruby_sanctum.h:102 NPC_XERESTRASZA
local SAY_XERESTRASZA_EVENT = 0 -- ruby_sanctum.cpp:29-36
local SAY_XERESTRASZA_EVENT_1 = 1
local SAY_XERESTRASZA_EVENT_2 = 2
local SAY_XERESTRASZA_EVENT_3 = 3
local SAY_XERESTRASZA_EVENT_4 = 4
local SAY_XERESTRASZA_EVENT_5 = 5
local SAY_XERESTRASZA_EVENT_6 = 6
local SAY_XERESTRASZA_EVENT_7 = 7
local XERESTRASZA_MOVE_X = 3151.236 -- ruby_sanctum.cpp:56 xerestraszaMovePos
local XERESTRASZA_MOVE_Y = 379.8733
local XERESTRASZA_MOVE_Z = 86.31996
local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (nefarian precedent)
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002 -- UnitDefines.h:187

local SPELL_CLEAVE           = 40504
local SPELL_BLADE_TEMPEST    = 75125
local SPELL_ENERVATING_BRAND = 74502
local SPELL_ENERVATING_BRAND_TRIGGER = 74505 -- boss_baltharus_the_warborn.cpp:328
local SPELL_SIPHONED_MIGHT = 74507 -- boss_baltharus_the_warborn.cpp:38
local SPELL_CLEAR_DEBUFFS    = 34098
local SPELL_CLONE            = 74511
local SPELL_REPELLING_WAVE   = 74509
local SPELL_SPAWN_EFFECT     = 64195

local timers = {}
local cloneCount = {}

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

-- C++ passes withTank=false on both boss (:207-211) and clone
-- (:304-308) ENERVATING_BRAND picks (SelectTarget(Random, 0, 45.0f,
-- true, false, -SPELL_ENERVATING_BRAND)): threat-list players within
-- 45yd without the brand aura, excluding the last victim
-- (UnitAI.cpp:256: _exception = GetThreatManager().GetLastVictim()).
-- The old GetPlayersInWorld instance pool could pick players never
-- engaged with the boss (2026-10-25 fix, same class as the
-- archavon/nexus/ahnkahet/obsidian threat-pool fixes); victim-GUID
-- exclusion is the standing approximation of the last-victim
-- exception (erekem precedent).
local function randomTargetsNoBrand(creature, range)
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < range
                and not t:HasAura(SPELL_ENERVATING_BRAND)
                and t:GetGUID() ~= victimGuid then
            candidates[#candidates + 1] = t
        end
    end
    return candidates
end

-- C++ EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE 40504) (non-triggered),
-- 13s init, Repeat(20s, 24s).
local function cleaveTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            cleaveTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {20000, 24000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_BLADE_TEMPEST: DoCastSelf(SPELL_BLADE_TEMPEST 75125)
-- (non-triggered), 18s init, Repeat(24s) (:204 boss AI; :301 is the clone AI).
local function bladeTempestTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bladetempest", 500, function()
            bladeTempestTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BLADE_TEMPEST)
    schedule(guid, "bladetempest", 24000, function()
        bladeTempestTick(creature, guid)
    end)
end

-- C++ clone EVENT_BLADE_TEMPEST (:300-303): DoCastVictim
-- (SPELL_BLADE_TEMPEST 75125) (non-triggered), 15s init, Repeat(24s) —
-- VICTIM-cast, the deviation from the boss's DoCastSelf (:204).
local function cloneBladeTempestTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "clonebladetempest", 500, function()
            cloneBladeTempestTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_BLADE_TEMPEST)
    schedule(guid, "clonebladetempest", 24000, function()
        cloneBladeTempestTick(creature, guid)
    end)
end

-- C++ EVENT_ENERVATING_BRAND: DoCast(target, SPELL_ENERVATING_BRAND 74502)
-- (non-triggered) x RAID_MODE(2,4,2,4) random targets in 45.0f excluding
-- SPELL_ENERVATING_BRAND holders; 13s init, Repeat(26s). The count is
-- raid-moded 2 (10-man) / 4 (25-man) via Is25ManRaid (bridge live
-- lua_creature_events.go:3651); the aura-exclusion is bridged via
-- HasAura (the netherspite precedent); targets are picked without
-- replacement (table.remove) — C++ re-runs SelectTarget per iteration
-- with the -SPELL_ENERVATING_BRAND filter, so the same target is never
-- picked twice (the putricide randomPlayers precedent).
local function enervatingBrandTick(creature, guid)
    local candidates = randomTargetsNoBrand(creature, 45)
    if creature:IsCasting() then
        schedule(guid, "enervatingbrand", 500, function()
            enervatingBrandTick(creature, guid)
        end)
        return
    end
    local count = 2
    if creature:Is25ManRaid() then
        count = 4
    end
    for i = 1, count do
        if #candidates == 0 then
            break
        end
        local target = table.remove(candidates, math.random(#candidates))
        creature:CastSpell(target, SPELL_ENERVATING_BRAND)
    end
    schedule(guid, "enervatingbrand", 26000, function()
        enervatingBrandTick(creature, guid)
    end)
end

-- C++ boss_baltharus_the_warborn::JustEngagedWith:
-- me->InterruptNonMeleeSpells(false); BossAI::JustEngagedWith(who);
-- events.Reset(); events.SetPhase(PHASE_COMBAT);
-- events.ScheduleEvent(EVENT_CLEAVE, Seconds(13), 0, PHASE_COMBAT);
-- events.ScheduleEvent(EVENT_ENERVATING_BRAND, Seconds(13), 0, PHASE_COMBAT);
-- events.ScheduleEvent(EVENT_BLADE_TEMPEST, Seconds(18), 0, PHASE_COMBAT);
-- Talk(SAY_AGGRO) last; the engage InterruptNonMeleeSpells(false)
-- (:111) is PORTED first in baltharusEnterCombat below (gal_darah
-- precedent — the old "leg out" note rested on the stale pre-pending-model
-- no-op-stub reading of the bridge, retracted; the bridge at
-- lua_creature_events.go:1995 cancels pending/channel casts).
-- C++ boss_baltharus_the_warborn::JustEngagedWith also schedules
-- EVENT_CLEAVE / EVENT_ENERVATING_BRAND (13s) / EVENT_BLADE_TEMPEST
-- (18s) on PHASE_COMBAT (the intro phase carries only the intro Talk,
-- so no gating is lost); timers ride the maiden CreateLuaEvent
-- convention; the UpdateAI CASTING gates (boss :189/:223, clone
-- :289/:314) are ported as 500ms IsCasting defer per cast tick,
-- including the 1ms EVENT_CLONE pump (stale maiden deviation
-- retracted); the non-cast EVENT_SUMMONS_ATTACK leg defers on the boss
-- the same way (summonsAttackTick, 2026-11-09 audit — the three
-- cleave/blade ticks were also moved from a post-cast IsCasting check
-- to the top-of-tick defer this audit, since casting first consumed
-- the event while another cast was in progress).
local function baltharusEnterCombat(event, creature, target)
    -- C++ JustEngagedWith (boss_baltharus_the_warborn.cpp:110-120):
    -- InterruptNonMeleeSpells(false) first, then the BossAI base arms
    -- (ScriptedCreature.cpp:527-544) — SetBossState(IN_PROGRESS) +
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat; setActive has no
    -- bridge, ScheduleTasks is an empty base no-op. events.Reset() +
    -- SetPhase(PHASE_COMBAT) + the timer seeds follow; Talk(SAY_AGGRO)
    -- fires last in C++ (after the seeds) — the Lua handler matches that
    -- order this tick.
    creature:InterruptNonMeleeSpells(false)
    creature:SetBossState(DATA_BALTHARUS_THE_WARBORN, BOSS_STATE_IN_PROGRESS)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "cleave", 13000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "enervatingbrand", 13000, function()
        enervatingBrandTick(creature, guid)
    end)
    schedule(guid, "bladetempest", 18000, function()
        bladeTempestTick(creature, guid)
    end)
    creature:Talk(SAY_AGGRO)
end

-- C++ summons.DespawnAll() (boss EnterEvadeMode :171-173, _JustDied
-- :513-521): the only summon is the clone; grid sweep is the
-- instance-lookup twin (rotface relayProfessor idiom: unique entry, 100yd).
local function despawnClones(creature)
    for _, clone in ipairs(creature:GetCreaturesInGrid(ENTRY_BALTHARUS_CLONE, 100)) do
        clone:Despawn()
    end
end

local function baltharusLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    despawnClones(creature)
    cancelTimers(guid)
    cloneCount[guid] = nil
    -- C++ EnterEvadeMode (boss_baltharus_the_warborn.cpp:171-176):
    -- summons.DespawnAll() + events.Reset() + _DespawnAtEvade() ->
    -- DespawnOrUnsummon(0s, 30s) + SetBossState(FAIL)
    -- (ScriptedCreature.cpp:596-617) — PORTED this tick on ev2 (the
    -- deathwhisper/blood-prince ev2 evade-FAIL precedent; shahraz
    -- precedent for the Despawn half). State write lands before the
    -- removal; the 30s respawn delay is not expressible via the bridge
    -- (Eluna takes no forceRespawnTimer arg) — respawn rides the DB
    -- spawntime, and the respawn ev23 re-writes NOT_STARTED.
    creature:SetBossState(DATA_BALTHARUS_THE_WARBORN, BOSS_STATE_FAIL)
    creature:Despawn()
end

local function baltharusReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    cloneCount[guid] = nil
    -- C++ Reset (boss_baltharus_the_warborn.cpp:78-86): _Reset() first
    -- (BossAI::_Reset, ScriptedCreature.cpp:499-511) — SetBossState
    -- NOT_STARTED only when the stored state != DONE. PORTED this tick
    -- via creature:SetBossState/GetBossState (bridge live
    -- lua_creature_events.go:5917/5932). Alive-gated: Go fires ev23 on
    -- the death path before ev4 (moroes/algalon precedent) and C++
    -- Reset never runs on death; the != DONE gate keeps a DONE boss
    -- DONE across respawn. The remaining Reset legs stay out:
    -- SetPhase(PHASE_INTRO) (phase machine; only gates the unported
    -- intro Talk), instance->SetData(DATA_BALTHARUS_SHARED_HEALTH)
    -- (no instance-SetData bridge; creature:SetData is the per-AI
    -- ScriptData twin), and the BARRIER_CHANNEL cast on the channel
    -- target (DATA_CRYSTAL_CHANNEL_TARGET never latched — no spawn
    -- handler for entry 26712 exists).
    if creature:IsAlive()
            and creature:GetBossState(DATA_BALTHARUS_THE_WARBORN) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BALTHARUS_THE_WARBORN, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ boss_baltharus_the_warborn::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent.
local function baltharusKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_baltharus_the_warborn::DamageTaken -> EVENT_CLONE (1ms) ->
-- DoAction(ACTION_CLONE): DoCastSelf(SPELL_CLEAR_DEBUFFS 34098, true) +
-- DoCastSelf(SPELL_CLONE 74511, true) + DoCastSelf(SPELL_REPELLING_WAVE
-- 74509) + Talk(SAY_CLONE 3). Thresholds are difficulty-split (C++
-- DamageTaken :143-169; Is25ManRaid bridged, blood-queen precedent):
-- 10-man _cloneCount == 0 and HealthBelowPctDamaged(50, damage);
-- 25-man _cloneCount == 0 and HealthBelowPctDamaged(66, damage), then
-- _cloneCount == 1 and HealthBelowPctDamaged(33, damage) — the elseif
-- order matters, a single blow crossing both 25-man thresholds fires
-- only the 66 arm. Fired as the 1ms scheduled clone leg; the C++
-- event-9 hook is pre-damage, so GetHealth() here is the pre-damage
-- value C++ compares against.
-- C++ EVENT_CLONE runs in UpdateAI under the :189 pre-loop +
-- :223 in-loop CASTING gates (DamageTaken only schedules it at 1ms),
-- so the Lua 1ms pump defers 500ms while casting, re-arm unconsumed.
local function baltharusCloneTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "clone", 500, function()
            baltharusCloneTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CLEAR_DEBUFFS, true)
    creature:CastSpell(creature, SPELL_CLONE, true)
    creature:CastSpell(creature, SPELL_REPELLING_WAVE)
    creature:Talk(SAY_CLONE)
end

local function baltharusDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if not cloneCount[guid] then
        cloneCount[guid] = 0
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- C++ HealthBelowPctDamaged int64 form (Unit.h:873): postDamage <
    -- CountPctFromMaxHealth(pct) = floor(maxHealth * pct / 100). The old
    -- cross-multiplied real-number form fired 1 HP early when
    -- maxHealth * pct mod 100 != 0 (anubarak 2026-10-25 precedent). No
    -- postDamage > 0 guard: DamageTaken arms fire on lethal hits.
    local postDamage = creature:GetHealth() - damage
    local fire = false
    if creature:Is25ManRaid() then
        if cloneCount[guid] == 0
                and postDamage < math.floor(maxHealth * 66 / 100) then
            cloneCount[guid] = 1
            fire = true
        elseif cloneCount[guid] == 1
                and postDamage < math.floor(maxHealth * 33 / 100) then
            cloneCount[guid] = 2
            fire = true
        end
    elseif cloneCount[guid] == 0
            and postDamage < math.floor(maxHealth * 50 / 100) then
        cloneCount[guid] = 1
        fire = true
    end
    if fire then
        schedule(guid, "clone", 1, function()
            baltharusCloneTick(creature, guid)
        end)
    end
end

-- C++ EVENT_SUMMONS_ATTACK (:213-215) runs inside the boss UpdateAI
-- gated loop (:189 pre-loop + :223 in-loop CASTING gates), so the 2s
-- leg defers 500ms on the BOSS (the scheduling creature) while it is
-- casting, re-arm unconsumed — the non-cast-leg precedent.
local function summonsAttackTick(creature, summon, guid)
    if creature:IsCasting() then
        schedule(guid, "summonsattack", 500, function()
            summonsAttackTick(creature, summon, guid)
        end)
        return
    end
    if summon:IsAlive() then
        summon:DoZoneInCombat()
    end
end

-- C++ boss_baltharus_the_warborn::JustSummoned (:136-141):
-- summons.Summon(summon) (summon-list, no bridge) +
-- summon->SetHealth(me->GetHealth()) +
-- events.ScheduleEvent(EVENT_SUMMONS_ATTACK, 2s). PORTED this tick via
-- ev19: the clone materializes through the spell-effect summon funnel
-- (creatureEffectSummonType, boss_ai.go:3298), so ev19 fires on the
-- boss with the clone as `summon` (lua_creature_events.go:190) and the
-- health copy + 2s DoZoneInCombat both have bearers. The 2s timer rides
-- the boss's timer table so boss ev2/ev4/ev23 cancel it, matching C++
-- events.Reset; the IsAlive guard covers a clone that died inside the
-- 2s window (DoZoneInCombat itself is nil-safe,
-- lua_creature_events.go:1957).
local function baltharusJustSummoned(event, creature, summon)
    if summon:GetEntry() ~= ENTRY_BALTHARUS_CLONE then
        return
    end
    summon:SetHealth(creature:GetHealth())
    local guid = creature:GetGUIDLow()
    schedule(guid, "summonsattack", 2000, function()
        summonsAttackTick(creature, summon, guid)
    end)
end

-- C++ boss_baltharus_the_warborn::JustDied (:122-129):
-- _JustDied(); Talk(SAY_DEATH);
-- if (Creature* xerestrasza = instance->GetCreature(DATA_XERESTRASZA))
--     xerestrasza->AI()->DoAction(ACTION_BALTHARUS_DEATH) — the sjonnir
-- precedent (the _JustDied() SetBossState(DONE) leg is PORTED this tick,
-- first arm, moroes precedent; the DoAction leg ports below via grid
-- sweep + timed Talk chain).
local function baltharusDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    despawnClones(creature)
    creature:SetBossState(DATA_BALTHARUS_THE_WARBORN, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ JustDied relay (boss_baltharus_the_warborn.cpp:127-128) ->
    -- xerestrasza DoAction(ACTION_BALTHARUS_DEATH) (ruby_sanctum.cpp:80-99):
    -- setActive(true) (no Lua bridge, stays out), SetFarVisible(true),
    -- Talk(EVENT 0), SetWalk(true), MovePoint(0, xerestraszaMovePos),
    -- then Talk 1..7 at 16/25/32/42/51/61/69s, the questgiver flag at
    -- 69s, and SetFarVisible(false) (setActive(false) stays out with
    -- setActive(true)). Sweep is the instance-lookup twin (rotface
    -- relayProfessor idiom: first alive match, unique entry, 100yd).
    for _, xerestrasza in ipairs(creature:GetCreaturesInGrid(ENTRY_XERESTRASZA, 100)) do
        if xerestrasza:IsAlive() then
            local xguid = xerestrasza:GetGUIDLow()
            xerestrasza:SetFarVisible(true)
            xerestrasza:Talk(SAY_XERESTRASZA_EVENT)
            xerestrasza:SetWalk(true)
            xerestrasza:MoveTo(0, XERESTRASZA_MOVE_X, XERESTRASZA_MOVE_Y, XERESTRASZA_MOVE_Z)
            schedule(xguid, "xere_event_1", 16000, function() xerestrasza:Talk(SAY_XERESTRASZA_EVENT_1) end)
            schedule(xguid, "xere_event_2", 25000, function() xerestrasza:Talk(SAY_XERESTRASZA_EVENT_2) end)
            schedule(xguid, "xere_event_3", 32000, function() xerestrasza:Talk(SAY_XERESTRASZA_EVENT_3) end)
            schedule(xguid, "xere_event_4", 42000, function() xerestrasza:Talk(SAY_XERESTRASZA_EVENT_4) end)
            schedule(xguid, "xere_event_5", 51000, function() xerestrasza:Talk(SAY_XERESTRASZA_EVENT_5) end)
            schedule(xguid, "xere_event_6", 61000, function() xerestrasza:Talk(SAY_XERESTRASZA_EVENT_6) end)
            schedule(xguid, "xere_event_7", 69000, function()
                xerestrasza:Talk(SAY_XERESTRASZA_EVENT_7)
                xerestrasza:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
                xerestrasza:SetFarVisible(false)
            end)
            break
        end
    end
end

-- C++ instance OnCreatureCreate latching (InstanceScript::AddObject):
-- the boss GUID goes in the instance store under
-- DATA_BALTHARUS_THE_WARBORN so the clone's JustDied relay can resolve
-- it via GetInstanceCreature (sjonnir precedent). Fires on first spawn
-- and on respawn (motionFor).
local function baltharusSpawned(event, creature)
    creature:SetGuidData(DATA_BALTHARUS_THE_WARBORN)
end

RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 1, baltharusEnterCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 2, baltharusLeaveCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 3, baltharusKilledUnit)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 4, baltharusDied)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 5, baltharusSpawned)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 9, baltharusDamageTaken)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 19, baltharusJustSummoned)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 23, baltharusReset)

-- C++ npc_xerestrasza::Reset (ruby_sanctum.cpp:70-74): _events.Reset() +
-- RemoveFlag(UNIT_NPC_FLAGS, GOSSIP_OPTION_QUESTGIVER). The talk-chain
-- timers above are keyed by xerestrasza's GUID in this file's table, so
-- Reset cancels any pending chain event and clears the questgiver flag
-- the 69s event set (the setActive legs stay out, as in baltharusDied).
local function xerestraszaReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_QUESTGIVER)
end

RegisterCreatureEvent(ENTRY_XERESTRASZA, 23, xerestraszaReset)

-- C++ npc_baltharus_the_warborn_clone::JustEngagedWith (:255-264):
-- DoZoneInCombat(); events.Reset();
-- events.ScheduleEvent(EVENT_CLEAVE, 11s);
-- events.ScheduleEvent(EVENT_BLADE_TEMPEST, 15s);
-- events.ScheduleEvent(EVENT_ENERVATING_BRAND, 10s). The DoZoneInCombat
-- leg is PORTED 2026-10-23 (creature:DoZoneInCombat, first in arm order;
-- bridge live engine/world/lua_creature_events.go). The clone's cleave
-- is DoCastVictim (:296-299) = the boss's cleaveTick; brand (:304-308)
-- is the same no-brand pool = enervatingBrandTick.
local function cloneEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- C++ :257, first in arm order (live bridge lua_creature_events.go; 2026-10-23)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "cleave", 11000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "clonebladetempest", 15000, function()
        cloneBladeTempestTick(creature, guid)
    end)
    schedule(guid, "enervatingbrand", 10000, function()
        enervatingBrandTick(creature, guid)
    end)
end

local function cloneLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_baltharus_the_warborn_clone::Reset
-- (boss_baltharus_the_warborn.cpp:249-253): DoCastSelf
-- (SPELL_SPAWN_EFFECT 64195) (non-triggered) + SetReactState
-- (REACT_DEFENSIVE) — the react-state leg rides the SetReactState bridge
-- (engine/world/lua_creature_events.go).
local function cloneReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:CastSpell(creature, SPELL_SPAWN_EFFECT)
    creature:SetReactState(REACT_DEFENSIVE)
end

-- C++ npc_baltharus_the_warborn_clone::JustDied (:273-279): _JustDied()
-- is NOT called (no SetBossState leg for the clone) — Unit::Kill(killer,
-- baltharus) only. PORTED this tick: the boss GUID is latched by its ev5
-- SetGuidData (sjonnir precedent), resolved via GetInstanceCreature
-- (lua_creature_events.go:5986), killed via creature:Kill(killer)
-- (lua_creature_events.go:2971 — fires the boss's ev23-then-ev4 chain,
-- i.e. its JustDied with SetBossState DONE + Talk + xerestrasza relay).
-- Nil when the boss was never latched (the C++ nullptr twin). Timers
-- are cancelled so the clone's scheduler doesn't fire on the corpse.
local function cloneDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    local baltharus = creature:GetInstanceCreature(DATA_BALTHARUS_THE_WARBORN)
    if baltharus ~= nil then
        baltharus:Kill(killer)
    end
end

RegisterCreatureEvent(ENTRY_BALTHARUS_CLONE, 1, cloneEnterCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_CLONE, 2, cloneLeaveCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_CLONE, 4, cloneDied)
RegisterCreatureEvent(ENTRY_BALTHARUS_CLONE, 23, cloneReset)

-- C++ spell_baltharus_enervating_brand_trigger::HandleSiphonedMight
-- (boss_baltharus_the_warborn.cpp:328-360): OnHit. The triggering
-- spell is the Enervating Brand aura itself (74502, the aura whose
-- periodic trigger casts 74505), so GetTriggeringSpell()->Id is the
-- static 74502 — no GetTriggeringSpell bridge is needed (none exists
-- in engine/). The hit unit casts Siphoned Might 74507, triggered,
-- on the brand aura's caster (Baltharus, via aura:GetCaster(),
-- lua_aura.go:207). Dormant until Spell.dbc ships (the aura's
-- periodic trigger effect is DBC-side).
local function enervatingBrandSiphonedMight(event, spell, target)
    if target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local aura = caster:GetAura(SPELL_ENERVATING_BRAND)
    if aura == nil then
        return
    end
    local baltharus = aura:GetCaster()
    if baltharus == nil then
        return
    end
    target:CastSpell(baltharus, SPELL_SIPHONED_MIGHT, true)
end
RegisterSpellEvent(SPELL_ENERVATING_BRAND_TRIGGER, 2, enervatingBrandSiphonedMight)
