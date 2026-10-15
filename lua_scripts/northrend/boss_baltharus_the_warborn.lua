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
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_baltharus_the_warborn JustEngagedWith — Talk(SAY_AGGRO 1)
-- (event 1 — the auriaya precedent; the InterruptNonMeleeSpells /
-- BossAI::JustEngagedWith / event-scheduling legs have no bridges).
-- boss_baltharus_the_warborn JustDied — Talk(SAY_DEATH 4) (event 4 —
-- the sjonnir precedent; the _JustDied() instance leg has no instance
-- bridge and the xerestrasza->AI()->DoAction(ACTION_BALTHARUS_DEATH)
-- cross-AI leg has no cross-AI-DoAction bridge).
-- boss_baltharus_the_warborn KilledUnit — Talk(SAY_KILL 2) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header; only entry 39751 registered):
-- boss_baltharus_the_warborn DoAction ACTION_CLONE — ported inline
-- (self-call from the boss's own UpdateAI scheduler: DoCastSelf
-- CLEAR_DEBUFFS 34098 triggered + CLONE 74511 triggered +
-- REPELLING_WAVE 74509 + Talk(SAY_CLONE 3)); the EVENT_CLONE timer
-- rides the DamageTaken 50% latch below.
-- boss_baltharus_the_warborn UpdateAI scheduler — Talk(SAY_BALTHARUS_INTRO
-- 0) rides EVENT_INTRO_TALK (no-timer-bridge; the EVENT_INTRO_TALK leg
-- itself rides DoAction(ACTION_INTRO_BALTHARUS) issued by
-- at_baltharus_plateau — no cross-AI-DoAction bridge; the
-- EVENT_CLEAVE / EVENT_BLADE_TEMPEST / EVENT_ENERVATING_BRAND /
-- EVENT_SUMMONS_ATTACK legs have no summon bridge). EVENT_CLEAVE /
-- EVENT_BLADE_TEMPEST / EVENT_ENERVATING_BRAND ported 2026-10-13 on
-- the maiden CreateLuaEvent convention (see below); EVENT_CLONE
-- ported 2026-10-14 via the DamageTaken latch below.
-- npc_baltharus_the_warborn_clone (39899) — zero Talk lines (NOT
-- registered, the bronjahm npc_corrupted_soul_fragment precedent); the
-- DamageTaken DATA_BALTHARUS_SHARED_HEALTH leg and the JustDied
-- Unit::Kill(baltharus) leg ride instance/actor bridges that don't
-- exist.
-- spell_baltharus_enervating_brand_trigger (SpellScript, OnHit
-- HandleSiphonedMight: aura-caster CastSpell SPELL_SIPHONED_MIGHT) —
-- joins the standing no-SpellScript-bridge queue.
-- All 5 Talk() calls in the file accounted for (SAY_AGGRO 1 +
-- SAY_DEATH 4 + SAY_KILL 2 + SAY_CLONE 3 ported; SAY_BALTHARUS_INTRO 0
-- documented above).

local ENTRY_BALTHARUS_THE_WARBORN = 39751

local SAY_AGGRO = 1
local SAY_KILL  = 2
local SAY_CLONE = 3
local SAY_DEATH = 4

local SPELL_CLEAVE           = 40504
local SPELL_BLADE_TEMPEST    = 75125
local SPELL_ENERVATING_BRAND = 74502
local SPELL_CLEAR_DEBUFFS    = 34098
local SPELL_CLONE            = 74511
local SPELL_REPELLING_WAVE   = 74509

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

local function randomTargetsNoBrand(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:IsWithinDist(plr, range)
                and not plr:HasAura(SPELL_ENERVATING_BRAND) then
            candidates[#candidates + 1] = plr
        end
    end
    return candidates
end

-- C++ EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE 40504) (non-triggered),
-- 13s init, Repeat(20s, 24s).
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {20000, 24000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_BLADE_TEMPEST: DoCastVictim(SPELL_BLADE_TEMPEST 75125)
-- (non-triggered), 18s init, Repeat(24s); the spell is a self-centered
-- AoE, so the victim target is C++-form-exact (the cleaveTick idiom).
local function bladeTempestTick(creature, guid)
    creature:CastSpell(nil, SPELL_BLADE_TEMPEST)
    schedule(guid, "bladetempest", 24000, function()
        bladeTempestTick(creature, guid)
    end)
end

-- C++ EVENT_ENERVATING_BRAND: DoCast(target, SPELL_ENERVATING_BRAND 74502)
-- (non-triggered) x RAID_MODE(2,4,2,4) random targets in 45.0f excluding
-- SPELL_ENERVATING_BRAND holders; 13s init, Repeat(26s). Deviation: no
-- difficulty bridge, count fixed at the 10-man value of 2 (the
-- jaraxxus/kelidan precedent); the aura-exclusion is bridged via
-- HasAura (the netherspite precedent); targets are picked without
-- replacement (table.remove) — C++ re-runs SelectTarget per iteration
-- with the -SPELL_ENERVATING_BRAND filter, so the same target is never
-- picked twice (the putricide randomPlayers precedent).
local function enervatingBrandTick(creature, guid)
    local candidates = randomTargetsNoBrand(creature, 45)
    for i = 1, 2 do
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
-- Talk(SAY_AGGRO) — the auriaya precedent (the non-Talk legs have no
-- bridges).
-- C++ boss_baltharus_the_warborn::JustEngagedWith also schedules
-- EVENT_CLEAVE / EVENT_ENERVATING_BRAND (13s) / EVENT_BLADE_TEMPEST
-- (18s) on PHASE_COMBAT (the intro phase carries only the intro Talk,
-- so no gating is lost); timers ride the maiden CreateLuaEvent
-- convention (no UNIT_STATE_CASTING gate in Go, so timers fire
-- unconditionally — the documented maiden deviation).
local function baltharusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
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
end

local function baltharusLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    cloneCount[guid] = nil
end

local function baltharusReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    cloneCount[guid] = nil
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
-- 74509) + Talk(SAY_CLONE 3). 10-man thresholds only (no difficulty
-- bridge — the jaraxxus/kelidan precedent): _cloneCount == 0 and
-- HealthBelowPctDamaged(50, damage), fired as the 1ms scheduled clone
-- leg; the C++ event-9 hook is pre-damage, so GetHealth() here is the
-- pre-damage value C++ compares against.
local function baltharusDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if not cloneCount[guid] then
        cloneCount[guid] = 0
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if cloneCount[guid] == 0
            and (creature:GetHealth() - damage) * 100 < maxHealth * 50 then
        cloneCount[guid] = 1
        schedule(guid, "clone", 1, function()
            creature:CastSpell(creature, SPELL_CLEAR_DEBUFFS, true)
            creature:CastSpell(creature, SPELL_CLONE, true)
            creature:CastSpell(creature, SPELL_REPELLING_WAVE)
            creature:Talk(SAY_CLONE)
        end)
    end
end

-- C++ boss_baltharus_the_warborn::JustDied:
-- _JustDied();
-- Talk(SAY_DEATH);
-- if (Creature* xerestrasza = instance->GetCreature(DATA_XERESTRASZA))
--     xerestrasza->AI()->DoAction(ACTION_BALTHARUS_DEATH) — the sjonnir
-- precedent (the _JustDied() instance leg and the cross-AI DoAction leg
-- have no bridges).
local function baltharusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 1, baltharusEnterCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 2, baltharusLeaveCombat)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 3, baltharusKilledUnit)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 4, baltharusDied)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 9, baltharusDamageTaken)
RegisterCreatureEvent(ENTRY_BALTHARUS_THE_WARBORN, 23, baltharusReset)
