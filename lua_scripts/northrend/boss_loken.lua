-- Loken (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_loken.cpp
-- (boss_loken (CreatureScript) via GetHallsOfLightningAI<boss_lokenAI>
-- (BossAI); spell_loken_pulsing_shockwave (SpellScriptLoader) —
-- registered from inside AddSC_boss_loken(); loader decl 99 /
-- call 294 per northrend_script_loader.cpp — the SECOND group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_bjarngrim() (decl 98 / call 293), under the
-- "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_boss_ionar() (decl 100 / call 295) — loader order
-- confirmed this run; the checkpoint sequence
-- (boss_bjarngrim -> boss_loken) is followed).
-- Entry: 28923 Loken (halls_of_lightning.h NPC_LOKEN, line 42;
-- DATA_LOKEN = 3, line 34); instance_halls_of_lightning.cpp
-- OnCreatureCreate binds case NPC_LOKEN (line 60) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_loken"
-- hits boss_loken.cpp + the loader decl/call lines for
-- AddSC_boss_loken() only; whole-server-tree grep for
-- "spell_loken_pulsing_shockwave" hits boss_loken.cpp only; zero
-- sql/ hits for either. No loken lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (lua_creature_events.go fires
-- (event, creature, attacker, damage) pre-application — the moroes
-- convention). Timers via CreateLuaEvent (GUID-keyed, cancelled on
-- events 2/4/23); melee is engine-driven in Go.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 2) (event 1; the BossAI
-- bookkeeping passthrough, events.SetPhase(PHASE_NORMAL), the three
-- ScheduleEvent legs, and
-- instance->DoStartTimedAchievement(ACHIEV_TIMELY_DEATH_START_EVENT
-- 20384) have no bridges — the boss_bookkeeping precedent).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY 4) — PORTED (event 3; the razuvious player-gated
-- variant precedent: victim nil-guard, victim:GetObjectType() ==
-- "Player") — sixteenth player-gated variant ported.
-- JustDied — PORTED (event 4; arm order: Talk(SAY_DEATH 8) then
-- instance->DoRemoveAurasDueToSpellOnPlayers(SPELL_PULSING_SHOCKWAVE_AURA
-- 59414), boss_loken.cpp:118, via GetPlayersInWorld + mapId/instanceId
-- filter + player:RemoveAura — the moroes precedent; the _JustDied boss
-- bookkeeping passthrough rides the Go native layer).
-- EVENT_ARC_LIGHTNING — PORTED (see UpdateAI note above).
-- EVENT_LIGHTNING_NOVA — PORTED 2026-10-17 (Talk(SAY_NOVA 3) +
-- Talk(EMOTE_NOVA 9) + DoCastAOE(SPELL_LIGHTNING_NOVA 52960) self-cast
-- non-triggered + RemoveAurasDueToSpell(SPELL_PULSING_SHOCKWAVE 52961)
-- via the RemoveAura bridge + reschedule self {20s,21s}; init 20s).
-- EVENT_RESUME_PULSING_SHOCKWAVE — PORTED 2026-10-17 (DoCast(me,
-- SPELL_PULSING_SHOCKWAVE_AURA 59414, TRIGGERED) + DoCast(me,
-- SPELL_PULSING_SHOCKWAVE 52961, TRIGGERED); the ClearUnitState
-- (UNIT_STATE_CASTING) leg is a net no-op in Go — packet-only casts,
-- no creature cast state; re-seeded at DUNGEON_MODE(5s, 4s) after each
-- nova via creature:IsHeroic(); init 1s at engage).
-- EVENT_INTRO_DIALOGUE (Talk(SAY_INTRO_2 1) +
-- events.SetPhase(PHASE_NORMAL)) — the phase flip has no Lua phase
-- model (dropped); the MoveInLineOfSight intro (Talk(SAY_INTRO_1 0)
-- gated on _isIntroDone + IsValidAttackTarget + IsWithinDistInMap
-- 40.0f, schedules EVENT_INTRO_DIALOGUE at 20s) is PORTED below on
-- event 27 with a per-GUID constructor latch (ossirian convention).
-- DamageTaken — the health-pct one-shot machine (event 9): C++
-- DamageTaken arms Talk(SAY_75HEALTH 5 / SAY_50HEALTH 6 /
-- SAY_25HEALTH 7) when HealthBelowPctDamaged(100 - 25 *
-- _healthAmountModifier, damage), then ++_healthAmountModifier;
-- _healthAmountModifier resets to 1 in Reset()/Initialize
-- (boss_loken.cpp:94/:180-198). PORTED as a per-GUID latch with
-- the moroes pre-application convention: (health - damage)
-- decides, one arm per DamageTaken call like the C++ if, latch
-- reset on events 1/2/23. The "no DamageTaken bridge" header
-- claim was stale (event 9 fires on the Lua surface) and is
-- retracted.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28923):
-- Instance script (instance_halls_of_lightning.cpp:87-93): SetBossState
-- DATA_LOKEN DONE -> LokenGlobe (GO_LOKEN_THRONE, bound on
-- OnGameObjectCreate) SendCustomAnim(0) — stays out: no Lua instance
-- bridge, and the sender is the instance boss-state transition, not
-- Loken's own JustDied, so no event-4 twin is valid here.
-- boss_loken Reset / Initialize (_healthAmountModifier = 1 flag +
-- instance->DoStopTimedAchievement(ACHIEV_TIMELY_DEATH_START_EVENT
-- 20384) — no timed-achievement bridge); JustEngagedWith's
-- DoStartTimedAchievement leg — no achievement bridge.
-- MoveInLineOfSight intro (Talk(SAY_INTRO_1 0) gated on
-- _isIntroDone + IsValidAttackTarget + IsWithinDistInMap 40.0f,
-- schedules EVENT_INTRO_DIALOGUE at 20s) — PORTED below (event 27).
-- EVENT_INTRO_DIALOGUE (Talk(SAY_INTRO_2 1) +
-- events.SetPhase(PHASE_NORMAL)) — Talk ported on the 20s timer; the
-- phase flip has no Lua phase model.
-- UpdateAI timer machine: EVENT_ARC_LIGHTNING — PORTED
-- (SelectTarget(Random, 0) -> DoCast(SPELL_ARC_LIGHTNING 52921)
-- non-triggered, 15s init / {15s,16s} re-arm, boss_loken.cpp:150-153
-- / :109; the player-only pool documents the playerOnly=FALSE
-- approximation, lavanthor precedent). EVENT_LIGHTNING_NOVA and
-- EVENT_RESUME_PULSING_SHOCKWAVE are PORTED (see the PORTED ARMS
-- note above — the "no aura-removal bridge" and "ClearUnitState has
-- no bridge" stay-out rationales are superseded 2026-10-17:
-- creature:RemoveAura is bridged and the clear-unit-state is a net
-- no-op).

-- spell_loken_pulsing_shockwave (SpellScript OnEffectHitTarget
-- CalculateDamage: damage *= distance2d when > 1.0f,
-- SPELL_PULSING_SHOCKWAVE 52961 EFFECT_0) — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue.

local ENTRY_LOKEN = 28923

local SAY_INTRO_1 = 0 -- boss_loken.cpp:34
local SAY_INTRO_2 = 1 -- :35
local SAY_AGGRO = 2
local SAY_SLAY = 4
local SAY_DEATH = 8
local SAY_75HEALTH = 5
local SAY_50HEALTH = 6
local SAY_25HEALTH = 7
local SAY_NOVA = 3 -- boss_loken.cpp:36
local EMOTE_NOVA = 9 -- :43

local SPELL_ARC_LIGHTNING = 52921
local SPELL_LIGHTNING_NOVA = 52960 -- :47
local SPELL_PULSING_SHOCKWAVE = 52961 -- :49
local SPELL_PULSING_SHOCKWAVE_AURA = 59414 -- :50

local timers = {}

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

-- C++ SelectTarget(Random, 0) — the bridged, threat-ordered
-- GetAITargets list (lua_creature_events.go:345); playerOnly defaults
-- FALSE (UnitAI.h:170), so pets/totems are eligible — no Player filter
-- (the midnight randomTargetAnywhere precedent). dist 0 = unlimited
-- range.
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- forward declaration: lokenEnterCombat calls this before the local
-- def below; a `local function` there would leave the call bound to
-- a nil global (the postmaster_malown / festergut bomb class).
local lokenResetMod

-- C++ JustEngagedWith: BossAI bookkeeping + Talk(AGGRO) +
-- SetPhase(PHASE_NORMAL) + three ScheduleEvent legs +
-- DoStartTimedAchievement — only the Talk arm is bridgeable.
-- C++ UpdateAI EVENT_ARC_LIGHTNING (boss_loken.cpp:150-153):
-- SelectTarget(Random, 0) -> DoCast(SPELL_ARC_LIGHTNING 52921),
-- 2-arg = non-triggered; init 15s (:109), unconditional re-arm
-- {15s,16s} (:153).
local function arcTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_ARC_LIGHTNING)
    end
    schedule(guid, "arc", math.random(15000, 16000), function()
        arcTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_RESUME_PULSING_SHOCKWAVE (boss_loken.cpp
-- :163-166): DoCast(me, SPELL_PULSING_SHOCKWAVE_AURA 59414, TRIGGERED)
-- + me->ClearUnitState(UNIT_STATE_CASTING) (net no-op in Go —
-- packet-only casts, no creature cast state, engine/world/lua_objects.go)
-- + DoCast(me, SPELL_PULSING_SHOCKWAVE 52961, TRIGGERED). One-shot:
-- seeded at engage (1s) and re-seeded by each nova; no self re-arm.
local function resumeTick(creature, guid)
    creature:CastSpell(creature, SPELL_PULSING_SHOCKWAVE_AURA, true)
    creature:CastSpell(creature, SPELL_PULSING_SHOCKWAVE, true)
end

-- C++ UpdateAI EVENT_LIGHTNING_NOVA (boss_loken.cpp:155-161):
-- Talk(SAY_NOVA 3) + Talk(EMOTE_NOVA 9) +
-- DoCastAOE(SPELL_LIGHTNING_NOVA 52960) (self-cast per the boss_ai.go
-- DoCastAOE lowering, 1-arg = non-triggered) +
-- RemoveAurasDueToSpell(SPELL_PULSING_SHOCKWAVE 52961) + reschedule
-- EVENT_RESUME_PULSING_SHOCKWAVE at DUNGEON_MODE(5s, 4s) (heroic 4s,
-- creature:IsHeroic()) + reschedule self at {20s,21s}; init 20s. The
-- earlier "no aura-removal bridge" header claim was stale
-- (creature:RemoveAura is bridged, engine/world/lua_objects.go:228).
local function novaTick(creature, guid)
    creature:Talk(SAY_NOVA)
    creature:Talk(EMOTE_NOVA)
    creature:CastSpell(creature, SPELL_LIGHTNING_NOVA)
    creature:RemoveAura(SPELL_PULSING_SHOCKWAVE)
    schedule(guid, "resume", creature:IsHeroic() and 4000 or 5000, function()
        resumeTick(creature, guid)
    end)
    schedule(guid, "nova", {20000, 21000}, function()
        novaTick(creature, guid)
    end)
end

local function lokenEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    lokenResetMod(creature:GetGUID())
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arc", 15000, function()
        arcTick(creature, guid)
    end)
    schedule(guid, "nova", 20000, function()
        novaTick(creature, guid)
    end)
    schedule(guid, "resume", 1000, function()
        resumeTick(creature, guid)
    end)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER gate, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function lokenTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ DamageTaken (boss_loken.cpp:180-198):
-- if (HealthBelowPctDamaged(100 - 25*_healthAmountModifier, damage))
-- { Talk(SAY_75/50/25HEALTH by case 1/2/3); ++_healthAmountModifier; }
-- _healthAmountModifier starts at 1 in Reset()/Initialize (:94), so
-- the latch resets on combat events; one arm per call like the C++
-- if. (health - damage) decides per the moroes pre-application
-- convention.
local lokenHealthMod = {}

local SAY_BY_MOD = { [1] = SAY_75HEALTH, [2] = SAY_50HEALTH, [3] = SAY_25HEALTH }

local function lokenDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local mod = lokenHealthMod[guid] or 1
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) < math.floor(maxHealth * (100 - 25 * mod) / 100) then
        local say = SAY_BY_MOD[mod]
        if say then
            creature:Talk(say)
        end
        lokenHealthMod[guid] = mod + 1
    end
end

lokenResetMod = function(guid)
    lokenHealthMod[guid] = 1
end

-- C++ JustDied: Talk(DEATH) + _JustDied() +
-- DoRemoveAurasDueToSpellOnPlayers(PULSING_SHOCKWAVE_AURA 59414,
-- true, true) — Talk and the player-strip both ported (moroes
-- precedent: no dead-player filter, C++ iterates every instance
-- player); the includePets/includeControlled legs have no
-- pet/controlled iteration bridge and stay unmodeled.
local function lokenDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_PULSING_SHOCKWAVE_AURA)
        end
    end
    cancelTimers(creature:GetGUID())
end

-- C++ MoveInLineOfSight: Talk(SAY_INTRO_1 0) on first player sight
-- within 40yd, then EVENT_INTRO_DIALOGUE at 20s -> Talk(SAY_INTRO_2 1)
-- (boss_loken.cpp:128-137/:169-172). _isIntroDone is
-- constructor-latched (never cleared in Reset), so the latch is
-- per-GUID and never cleared on 2/23 (ossirian/vashj convention);
-- the 20s dialogue timer rides the GUID-keyed schedule table so it
-- dies on evade/death like the C++ event map. The phase flip
-- (SetPhase(PHASE_NORMAL)) has no Lua phase model — dropped. The Go
-- fire site is the aggro-engage branch, a subset of C++'s any-unit
-- LOS trigger — acceptable for a one-shot intro (ossirian
-- precedent).
local lokenIntroDone = {}

local function lokenMoveInLOS(event, creature, unit)
    local guid = creature:GetGUID()
    local targetReach = 1.5
    if unit:GetObjectType() ~= "Player" then
        targetReach = unit:GetCombatReach()
    end
    if not lokenIntroDone[guid] and unit:GetDistance(creature) - creature:GetCombatReach() - targetReach < 40 then
        lokenIntroDone[guid] = true
        creature:Talk(SAY_INTRO_1)
        schedule(guid, "introDialogue", 20000, function()
            creature:Talk(SAY_INTRO_2)
        end)
    end
end

RegisterCreatureEvent(ENTRY_LOKEN, 1, lokenEnterCombat)
RegisterCreatureEvent(ENTRY_LOKEN, 2, function(e, c) lokenResetMod(c:GetGUID()) cancelTimers(c:GetGUID()) end)
RegisterCreatureEvent(ENTRY_LOKEN, 3, lokenTargetDied)
RegisterCreatureEvent(ENTRY_LOKEN, 4, lokenDied)
RegisterCreatureEvent(ENTRY_LOKEN, 9, lokenDamageTaken)
RegisterCreatureEvent(ENTRY_LOKEN, 23, function(e, c) lokenResetMod(c:GetGUID()) cancelTimers(c:GetGUID()) end)
RegisterCreatureEvent(ENTRY_LOKEN, 27, lokenMoveInLOS)
