-- Hadronox (Azjol-Nerub dungeon) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/AzjolNerub/boss_hadronox.cpp
-- (boss_hadronox). Azjol-Nerub dungeon-script unit per
-- northrend_script_loader.cpp order (decl 28 / call 230, immediately
-- after AddSC_boss_krik_thir(); the Azjol-Nerub dungeon block
-- continues here).
-- Entry: 28921 Hadronox (azjol_nerub.h NPC_HADRONOX —
-- kalecgos pass; the GetAzjolNerubAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); the BossAI _JustDied SetBossState(DATA_HADRONOX,
-- DONE) + instanceData.hadronoxDone flag is PORTED in hadronoxDied
-- below (tick 509; the old "no bridge" claim is stale —
-- creature:SetBossState, lua_creature_events.go:7099). The ev5
-- SetGuidData(DATA_HADRONOX) latch is ported in hadronoxSpawn.
-- Ported arms (C++-exact for the modeled arms):
-- boss_hadronox: EVENT_PIERCE_ARMOR DoCastVictim(Pierce Armor 53418),
-- randtime(4s,7s) init (C++ JustEngagedWith schedule), 10-15s repeat
-- (math.random — slad_ran precedent). The C++ JustEngagedWith arms are
-- independent schedulers: none of the unmodeled events gate PIERCE_ARMOR,
-- and the missing arms do not change its timing, so the arm stands alone
-- without the intro/movement machinery.
-- EVENT_LEECH_POISON — DoCastAOE(Leech Poison 53030) self-cast,
-- randtime(5s,7s) init, randtime(7s,9s) repeat (math.random); the
-- AuraScript's heal-on-death leg (53800) is PORTED below (tick 202,
-- RegisterAuraEvent OnRemove).
-- EVENT_WEB_GRAB — DoCastAOE(Web Grab 57731) self-cast,
-- randtime(13s,19s) init, randtime(20s,25s) repeat (math.random).
-- EVENT_ACID_CLOUD — DoCast(SelectTarget(Random, 0, 100.0f), ACID_CLOUD
-- 53400), randtime(7s,13s) init, randtime(16s,23s) repeat (math.random);
-- position=0 so no threat-head exclusion; threat-list pool via
-- GetAITargets (lua_creature_events.go:4559; midnight precedent).
-- npc_anub_ar_crusher DamageTaken (boss_hadronox.cpp:533-540):
-- pre-damage HealthBelowPctDamaged(25) latch -> Talk(2) +
-- DoCastSelf(SPELL_FRENZY 53801); latch wiped on event 4 (C++ ctor
-- reset). The crusher's engage arms (CRUSHER_SAY_AGGRO etc.) stay
-- unmodeled — they ride the unported pack-walk machine.
-- Unmodeled (no bridges — documented, not wired):
-- The whole pre-fight/step movement machine — InitializeAI
-- SetReactState(REACT_AGGRESSIVE)/SetFloatValue bounding/combat reach
-- (SetReactState IS bridged — frozen_core precedent,
-- lua_creature_events.go:4715; SetHomePosition IS bridged on the event
-- surface — 796 tick; the COMBATREACH leg IS ported (hadronoxSpawn,
-- ev5, SetCombatReach bridge) — only BOUNDINGRADIUS has no creature
-- motion slot and stays out)
-- + SetStep(0); SetStep: _lastPlayerCombatState latch, SetReactState(
-- REACT_PASSIVE), SetHomePosition(hadronoxStep[step]),
-- MotionMaster Clear + MovePoint — PORTED 2026-11-08 (tick 416) in
-- hadronoxSetStep below (MoveTo + ev28 MovementInform are bridged;
-- the old "motion-master bridge absent" claim is stale). The
-- DoAction(ACTION_HADRONOX_MOVE) driver (sender: crusher JustDied,
-- boss_hadronox.cpp:556; ev38 dispatch IS bridged,
-- lua_creature_events.go:845) rides the crusherDied relay below.
-- SummonCrusherPack(SUMMON_GROUP_CRUSHER_1/2/3): SummonCreatureGroup +
-- per-summon SetData(DATA_CRUSHER_PACK_ID) + DoAction(ACTION_PACK_WALK)
-- (JustAppeared leg; DoAction(ACTION_CRUSHER_ENGAGED) re-summon legs) —
-- summon STRAND + cross-AI SetData bridges absent; ev38 DoAction
-- dispatch IS bridged (lua_creature_events.go:845) but the senders
-- (SummonCrusherPack, boss_hadronox.cpp:192, and the crusher
-- JustEngagedWith, :527) ride the unported summon machine.
-- MovementInform: SetReactState(REACT_AGGRESSIVE) on EVERY step
-- arrival, but DoCastAOE(WEB_FRONT_DOORS 53177) +
-- DoCastAOE(WEB_SIDE_DOORS 53185) + _doorsWebbed latch + DoZoneInCombat
-- only at the final step — PORTED 2026-11-08 (tick 416) in
-- hadronoxMovementInform below (ev28 is bridged). Both spells' aura-
-- removal effects are PORTED below (RegisterSpellEvent ev5) — the
-- casts are the ported arm.
-- GetData(DATA_HADRONOX_ENTERED_COMBAT) + SetGUID(_anubar GUID list) +
-- ObjectAccessor cross-AI — cross-AI GetData/SetGUID bridges absent.
-- GetData(DATA_HADRONOX_WEBBED_DOORS) — PORTED 2026-11-08 (tick 416):
-- the latch rides hadronoxMovementInform; Go case in
-- engine/world/achievement_criteria_scripts.go.
-- CanAIAttack 70.0f home-distance leash — no leash bridge.
-- JustEngagedWith me->setActive(true) leg — no active-flag bridge.
-- EVENT_PLAYER_CHECK (1s): GetCombatManager PvE-ref IsControlledByPlayer
-- flip vs _lastPlayerCombatState -> instance->CheckRequiredBosses gate,
-- Point-motion cancel + AttackStart(victim), else EnterEvadeMode(
-- NO_HOSTILES) — instance-script model absent (standing), motion-master
-- bridge absent.
-- The DoAction machine (ev38 dispatch IS bridged,
-- creature:DoAction, lua_creature_events.go:845): the sender of
-- ACTION_CRUSHER_ENGAGED is the crusher pack-1 JustEngagedWith
-- (boss_hadronox.cpp:527) — pack-1 membership is stamped by the
-- unported SummonCrusherPack, so no Lua crusher can know it is
-- pack 1 — and the payload (instance SetBossState(DATA_HADRONOX,
-- IN_PROGRESS) + _enteredCombat latch + SummonCrusherPack(2/3))
-- needs the absent instance model.
-- EnterEvadeMode: GetCreatureListWithEntryInGrid(NPC_WORLDTRIGGER_LARGE
-- 23472) aura-scan (periodic-summon/web-door auras) + _DespawnAtEvade(25s)
-- legs, summons.DespawnAll (terestian precedent), _anubar ObjectAccessor::
-- GetCreature DespawnOrUnsummon — despawn + summon + ObjectAccessor
-- bridges absent.
-- UpdateAI CASTING gates PORTED: own UpdateAI pre-loop (:303) +
-- in-loop (:353) — pierce/leech/webgrab/acid defer 500ms while
-- casting, no repeat re-arm (mal_ganis idiom; IsCasting live,
-- lua_creature_events.go:2480). EVENT_PLAYER_CHECK stays out
-- (instance/motion). Crusher pack AI UpdateAI (:473) has NO
-- casting gate (plain while loop, zero HasUnitState hits), so
-- crusherSmashTick stays ungated per C++.
-- DamageTaken NPC safeguard (non-player damage, HealthBelowPct(70):
-- HealthBelowPctDamaged(5) -> damage=0 else damage *= (GetHealthPct()-5)/
-- 65) — Eluna event 9 (DamageTaken) IS bridged and GetHealth/
-- GetMaxHealth exist, so HealthBelowPct(70)/HealthBelowPctDamaged(5)
-- are now computable; the unit stays blocked on its unregistered
-- owner + unported DoAction-sender/MovePoint engage model, not on the
-- health/DamageTaken surface.
-- JustSummoned (boss_hadronox.cpp:372-376) summons.Summon leg — STAY OUT
-- (row 5): ev19 IS bridged and forwards (event, creature, summon)
-- (fireCreatureSummoned, lua_creature_events.go:8002; summonTempCreature
-- :190 fires it for group summons too), but no producer exists in Lua —
-- SummonCrusherPack (SummonCreatureGroup 1/2/3, :186-195) is unported and
-- Hadronox casts no summon spells (the 53064-53092 periodic summons are
-- cast by world trigger 23472, so their ev19 fires on the trigger, not
-- Hadronox). No live consumer either: the C++ consumers are BossAI
-- _Reset/_JustDied + EnterEvadeMode summons.DespawnAll
-- (ScriptedCreature.cpp:507/516; boss_hadronox.cpp:266), whose Lua twins
-- (hadronoxReset/hadronoxDied/hadronoxLeaveCombat) have nothing to
-- despawn. When the producer lands, SummonCreatureGroup returns the
-- summoned array, so it can populate the tracking table directly — ev19
-- adds no coverage; revisit only if a summon source outside the producer
-- appears.
-- npc_anub_ar_crusher — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED (28922 from
-- the file's Creatures enum — NPC_CRUSHER — kalecgos pass): zero
-- registration. The whole npc_hadronox_crusherPackAI engagement model is
-- unbridged: SetData(DATA_CRUSHER_PACK_ID) -> SetReactState(REACT_PASSIVE)
-- (cross-AI SetData pack-ID bridge absent — SetReactState itself is
-- bridged, lua_creature_events.go:4715), DoAction(
-- ACTION_PACK_WALK) -> MotionMaster MovePoint(_positions[pack]) + the
-- _doFacing SetFacingTo choreography (motion-master bridge absent;
-- sender is SummonCrusherPack, boss_hadronox.cpp:192; ev38 dispatch
-- IS bridged, lua_creature_events.go:845), MovementInform,
-- MoveInLineOfSight passive-engage leg
-- (CanStartAttack + IsWithinDistInMap(attackDistance + m_CombatDistance)
-- -> JustEngagedWith — pack-ID bridge absent; the distance leg is
-- Go's aggro-scan guard), group
-- pack-aggro JustEngagedWith (GetCreatureListWithEntryInGrid 40.0f,
-- same-pack SetReactState(AGGRESSIVE) + AttackStart — pack-ID bridge
-- absent; grid + EngageWithTarget are bridged), EnterEvadeMode ->
-- hadronox AI EnterEvadeMode
-- (cross-AI absent). EVENT_SMASH — PORTED (2026-11-03):
-- _JustEngagedWith schedules it unconditionally (all packs);
-- DoCastVictim(SMASH 53318) 8-12s init, 13-21s repeat. Timers do not
-- drive engagement (the engine does), so the port cannot make the
-- crusher engage where C++ holds it passive; it only gives an
-- already-in-combat crusher its C++ rotation — the same standing as
-- the DamageTaken port on this entry. The pack-1-only legs stay out:
-- Talk(CRUSHER_SAY_AGGRO 1) + instance DoAction(ACTION_CRUSHER_ENGAGED)
-- need the absent instance GetData(DATA_HADRONOX_ENTERED_COMBAT) model
-- + the unported sender (crusher JustEngagedWith,
-- boss_hadronox.cpp:527 — pack-1 membership is stamped by the
-- unported SummonCrusherPack); ev38 dispatch IS bridged
-- (lua_creature_events.go:845).
-- DamageTaken frenzy (HealthBelowPctDamaged(25) -> _hadFrenzy latch +
-- Talk(CRUSHER_EMOTE_FRENZY 2) + DoCastSelf(FRENZY 53801)) — the old
-- "no DamageTaken hook + no health-pct bridge" claim was stale
-- (event 9 carries pre-damage damage; the frenzy arm is PORTED below).
-- Hadronox's own 5% mitigation arm (boss_hadronox.cpp:361-369:
-- HealthBelowPct(70) / HealthBelowPctDamaged(5, damage) zeroing +
-- GetHealthPct() damage scaling) needs attacker->IsControlledByPlayer(),
-- which has no Lua-surface bridge (the Go mirror
-- creatureIsPlayerControlled, boss_ai.go:683, is unexported) — that
-- missing attacker-classification bridge is the true blocker, and a
-- GetObjectType()=="Player" approximation would wrongly reduce
-- pet/charm damage that C++ lets through at full). JustDied ->
-- hadronox AI
-- DoAction(ACTION_HADRONOX_MOVE) — the Talk leg ports in crusherDied
-- below (step-gated sweep); the SetStep movement machine stays out.
-- npc_anub_ar_crusher_champion / npc_anub_ar_crusher_crypt_fiend /
-- npc_anub_ar_crusher_necromancer + npc_anub_ar_champion /
-- npc_anub_ar_crypt_fiend / npc_anub_ar_necromancer — ENTRY UNVERIFIABLE
-- (no NPC constants in azjol_nerub.h or anywhere in the C++ sources; the
-- adds spawn from DB-side periodic-summon spell/summon-group data, not
-- C++ evidence — belnistrasz/willix precedent, no invented identifiers):
-- zero registration; they join the bridgeable-but-entry-blocked queue.
-- Port-pattern-ready rotation arms: crusher_champion/champion EVENT_REND
-- DoCastVictim(59343) 4-8s init, 12-16s repeat + EVENT_PUMMEL DoCastVictim
-- (59344) 15-19s init, 12-17s repeat (+foe champion EVENT_TAUNT
-- DoCastVictim(53798) 15-50s init, 15-50s repeat); crusher_crypt_fiend/
-- crypt_fiend EVENT_CRUSHING_WEBS DoCastVictim(59347) 4-8s init, 12-16s
-- repeat + EVENT_INFECTED_WOUND DoCastVictim(59348) 15-19s init, 16-25s
-- repeat (+foe TAUNT as above); crusher_necromancer/necromancer
-- EVENT_SHADOW_BOLT DoCastVictim(53333) 2-4s init, 2-5s repeat +
-- EVENT_ANIMATE_BONES DoCastVictim(urand(0,1) ? 53336 : 53334)
-- 37-45s init, 35-50s repeat (+foe TAUNT as above). The npc_hadronox_foeAI
-- MovementInform/MoveOut-Downstairs-Hadronox MovePoint sequence + the
-- SetGUID(_anubar) InitializeAI leg need motion-master + cross-AI
-- bridges anyway.
-- spell_hadronox_periodic_summon_champion / _crypt_fiend / _necromancer
-- (53035/53037/53036: HandleApply SetPeriodicTimer(2-17ms) + HandlePeriodic
-- top/bottom summon casts 53064-53092 gated on HavePlayers + boss-state
-- DONE aura-remove + caster Z >= 750.0f) — PORTED below
-- (RegisterAuraEvent ev1, the template's three instantiations share one
-- factory). The old "no SpellScript/AuraScript binding bridge" claim is
-- stale: AuraEventOnPeriodic (1) / OnApply (2) / OnRemove (3) are live
-- (runtime.go:575-577; muru 46041 ev1 precedent). Deviations: the
-- HandleApply 2-17ms first-tick re-arm has no SetPeriodicTimer bridge,
-- so the first tick fires at the DBC AuraPeriod; the HavePlayers gate
-- reads the map IsEmpty twin (globals.go:1223-1227, per-instance); the
-- auras themselves are DB-side per-spawn creature_addon auras on the
-- NPC_WORLDTRIGGER_LARGE (23472) triggers — Go never applies addon
-- auras and the per-spawn mapping is not in-tree, so no Lua application
-- leg is invented (bomb-bot precedent needs a 1:1 C++-documented
-- mapping); the handlers are DORMANT until the auras are applied, and
-- DBC-gated too (the 53064-53092 summon casts need Spell.dbc records).
-- PORTED (tick 202, RegisterAuraEvent OnRemove):
-- spell_hadronox_leeching_poison (53030 AuraScript: OnEffectRemove
-- AURA_REMOVE_BY_DEATH (non-guardian) -> caster CastSpell(caster,
-- LEECH_POISON_HEAL 53800, triggered)); the IsGuardian gate reads the
-- new unit:IsGuardian bridge (Map::SummonCreature mask rule).
-- spell_hadronox_web_doors (53177/53185 SpellScript: OnEffectHitTarget
-- removes the three periodic-summon auras from the target) — PORTED
-- below (RegisterSpellEvent ev5; muru summon-blood-elves precedent).
-- achievement_hadronox_denied — AchievementCriteriaScript (target ToCreature
-- -> AI GetData(DATA_HADRONOX_WEBBED_DOORS)) — no achievement-criteria
-- bridge (snakes precedent) + cross-AI GetData bridge absent —
-- documented-only.

local ENTRY_HADRONOX = 28921
local ENTRY_CRUSHER = 28922
local DATA_HADRONOX = 1 -- azjol_nerub.h ANDataTypes
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)

local SPELL_PIERCE_ARMOR = 53418
local SPELL_LEECH_POISON = 53030
local SPELL_LEECH_POISON_HEAL = 53800 -- boss_hadronox.cpp:64
local AURA_REMOVE_BY_DEATH = 5 -- SpellAuraDefines.h:68

-- C++ spell_hadronox_leeching_poison::HandleEffectRemove
-- (boss_hadronox.cpp:1058-1066): EFFECT_0 PERIODIC_LEECH, REAL. On
-- removal by death, if the target is not a guardian, the caster casts
-- Leech Poison Heal 53800 on itself, triggered.
local function leechingPoisonRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_DEATH then
        return
    end
    if target:IsGuardian() then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, SPELL_LEECH_POISON_HEAL, true)
end

RegisterAuraEvent(SPELL_LEECH_POISON, 3, leechingPoisonRemove)
-- C++ boss_hadronox.cpp:146-152 — the four SetStep waypoints
-- (0-based _step; Lua tables are 1-based, so index step+1).
local HADRONOX_STEPS = {
    { 515.5848, 544.2007, 673.6272 },
    { 562.191, 514.068, 696.4448 },
    { 610.3828, 518.6407, 695.9385 },
    { 530.42, 560.003, 733.0308 },
}
local SPELL_WEB_FRONT_DOORS = 53177 -- boss_hadronox.cpp:204
local SPELL_WEB_SIDE_DOORS = 53185 -- boss_hadronox.cpp:205
local DATA_HADRONOX_WEBBED_DOORS = 3 -- boss_hadronox.cpp:119
local POINT_MOTION_TYPE = 8 -- MovementDefines.h
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local SPELL_WEB_GRAB = 57731
local SPELL_ACID_CLOUD = 53400
local SPELL_CRUSHER_FRENZY = 53801
local SPELL_CRUSHER_SMASH = 53318
local CRUSHER_EMOTE_FRENZY = 2
local HADRONOX_EMOTE_MOVE = 1 -- boss_hadronox.cpp:132

-- C++ spell_hadronox_periodic_summon_template_AuraScript::HandlePeriodic
-- (boss_hadronox.cpp:960-979): EFFECT_0 PERIODIC_DUMMY. On each tick the
-- caster (the world trigger carrying the aura) casts the top summon spell
-- when its Z is >= 750.0f, else the bottom one, triggered; when the boss
-- state is DONE the aura removes itself; with no players in the instance
-- the tick is a no-op. One factory mirrors the C++ template (one class,
-- three instantiations bound to 53035/53037/53036 with their top/bottom
-- pairs). HandleApply (:953-957) only re-arms the first tick to 2-17s
-- (urandms(2, 17) = 2000-17000ms, Random.cpp:49-53) and has no
-- bridge: ev2 fires (AuraEventOnApply, lua_aura.go:1085) but the
-- aura object exposes no SetPeriodicTimer twin — no ev2 registration.
local HADRONOX_PERIODIC_SUMMONS = {
    [53035] = { 53064, 53090 }, -- champion: TOP 53064, BOTTOM 53090
    [53037] = { 53065, 53091 }, -- crypt fiend: TOP 53065, BOTTOM 53091
    [53036] = { 53066, 53092 }, -- necromancer: TOP 53066, BOTTOM 53092
}

local function periodicSummonTick(topSpell, bottomSpell)
    return function(event, aura, target)
        local caster = aura:GetCaster()
        if caster == nil then
            return
        end
        local map = caster:GetMap()
        if map ~= nil and map:IsEmpty() then
            return
        end
        if caster:GetBossState(DATA_HADRONOX) == BOSS_STATE_DONE then
            aura:Remove()
            return
        end
        if caster:GetZ() >= 750.0 then
            caster:CastSpell(caster, topSpell, true)
        else
            caster:CastSpell(caster, bottomSpell, true)
        end
    end
end

for periodicSpell, pair in pairs(HADRONOX_PERIODIC_SUMMONS) do
    RegisterAuraEvent(periodicSpell, 1, periodicSummonTick(pair[1], pair[2]))
end

-- C++ spell_hadronox_web_doors::HandleDummy (boss_hadronox.cpp:1094-1102,
-- registered OnEffectHitTarget EFFECT_0 SPELL_EFFECT_APPLY_AURA at :1106):
-- on effect 0, each hit unit loses the three periodic-summon auras. The
-- ev5 hook fires once per (effect, live hit target) before the default
-- (boss_ai.go:3435-3438), so the effectIndex == 0 filter is the C++
-- registration twin (muru summon-blood-elves precedent). DBC-gated only
-- (the 53177/53185 casts need Spell.dbc records).
local function webDoorsHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    target:RemoveAura(53035)
    target:RemoveAura(53037)
    target:RemoveAura(53036)
end

RegisterSpellEvent(SPELL_WEB_FRONT_DOORS, 5, webDoorsHit)
RegisterSpellEvent(SPELL_WEB_SIDE_DOORS, 5, webDoorsHit)

-- Crusher-death step counter for the HADRONOX_MOVE relay below (C++
-- _step, reset in Reset/ctor; NUM_STEPS = 4, Talk gated on _step < 3).
local hadronoxStep = {}

-- Engaged latch: the _lastPlayerCombatState twin (C++ EVENT_PLAYER_CHECK
-- :327-350 flips it on the 1s combat check; SetStep early-returns while
-- it is set). Set on ev1, cleared on ev2/ev4/ev23.
local hadronoxEngaged = {}

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

-- C++ EVENT_PIERCE_ARMOR: DoCastVictim; randtime(4s,7s) init, 10-15s repeat.
local function pierceArmorTick(creature, guid)
    -- Own UpdateAI pre-loop (:303) + in-loop (:353) CASTING gates:
    -- 500ms defer, no repeat re-arm (mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "piercearmor", 500, function()
            pierceArmorTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_PIERCE_ARMOR)
    local delay = math.random(10000, 15000)
    schedule(guid, "piercearmor", delay, function()
        pierceArmorTick(creature, guid)
    end)
end

-- C++ EVENT_LEECH_POISON: DoCastAOE; randtime(5s,7s) init,
-- randtime(7s,9s) repeat.
local function leechPoisonTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "leechpoison", 500, function()
            leechPoisonTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_LEECH_POISON)
    schedule(guid, "leechpoison", math.random(7000, 9000), function()
        leechPoisonTick(creature, guid)
    end)
end

-- C++ EVENT_WEB_GRAB: DoCastAOE; randtime(13s,19s) init,
-- randtime(20s,25s) repeat.
local function webGrabTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "webgrab", 500, function()
            webGrabTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_WEB_GRAB)
    schedule(guid, "webgrab", math.random(20000, 25000), function()
        webGrabTick(creature, guid)
    end)
end

-- C++ EVENT_ACID_CLOUD (boss_hadronox.cpp:314-317):
-- DoCast(SelectTarget(Random, 0, 100.0f), ACID_CLOUD 53400);
-- position=0 — no threat-head exclusion (contrast skeram position=1).
-- Threat-list pool via GetAITargets (lua_creature_events.go:4559;
-- midnight precedent), C++ dist=100.0f = max range; DoCast default
-- triggered=false (vaelastrasz convention). Init randtime(7s,13s)
-- (:229), repeat randtime(16s,23s) (math.random).
local function acidCloudTick(creature, guid)
    -- Same UpdateAI CASTING gates; defer precedes the target pick,
    -- so a deferred acid cloud picks nothing until it fires.
    if creature:IsCasting() then
        schedule(guid, "acidcloud", 500, function()
            acidCloudTick(creature, guid)
        end)
        return
    end
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        if not t:IsDead() and creature:GetDistance(t) - creature:GetCombatReach() - targetReach < 100.0 then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_ACID_CLOUD)
    end
    schedule(guid, "acidcloud", math.random(16000, 23000), function()
        acidCloudTick(creature, guid)
    end)
end

-- C++ boss_hadronox::InitializeAI (boss_hadronox.cpp:277-284):
-- BossAI::InitializeAI() -> SetReactState(REACT_AGGRESSIVE) ->
-- SetFloatValue(BOUNDINGRADIUS, 9.0f) -> SetFloatValue(COMBATREACH,
-- 9.0f) -> SetStep(0). Only the COMBATREACH leg is separable: it is
-- passive data (never re-written), so setting it on spawn strands
-- nothing, unlike the react/step legs that drive the movement
-- machine. Fires on ev5 (the constructor/spawn idiom, felmyst-trail
-- precedent) — ev5 runs once per spawn, never on evade, matching
-- InitializeAI's once-per-AI-creation timing; the BOUNDINGRADIUS leg
-- stays out (no creature motion slot, SetCombatReach comment).
local function hadronoxSpawn(event, creature)
    creature:SetCombatReach(9)
    -- C++ creatureData latching (instance_azjol_nerub.cpp:33:
    -- NPC_HADRONOX -> DATA_HADRONOX) — the instance-side
    -- OnCreatureCreate route is dead for static spawns
    -- (instance_hooks.go:553-562), so the latch rides ev5 here.
    creature:SetGuidData(DATA_HADRONOX)
    hadronoxStep[creature:GetGUIDLow()] = nil -- C++ ctor _step(0) twin
end

local function hadronoxEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    hadronoxEngaged[guid] = true
    cancelTimers(guid)
    schedule(guid, "piercearmor", math.random(4000, 7000), function()
        pierceArmorTick(creature, guid)
    end)
    schedule(guid, "leechpoison", math.random(5000, 7000), function()
        leechPoisonTick(creature, guid)
    end)
    schedule(guid, "webgrab", math.random(13000, 19000), function()
        webGrabTick(creature, guid)
    end)
    schedule(guid, "acidcloud", math.random(7000, 13000), function()
        acidCloudTick(creature, guid)
    end)
end

local function hadronoxLeaveCombat(event, creature)
    hadronoxEngaged[creature:GetGUIDLow()] = nil
    cancelTimers(creature:GetGUIDLow())
    -- C++ EnterEvadeMode: _DespawnAtEvade(25s) on self + aura triggers
    -- (cross-creature legs unbridged) + summons.DespawnAll() (moot —
    -- nothing modeled) (boss_hadronox.cpp:258-268); the 25s is
    -- respawn-side, the despawn itself is immediate
    -- (ScriptedCreature.cpp:616).
    creature:Despawn()
end

local function hadronoxDied(event, creature, killer)
    hadronoxEngaged[creature:GetGUIDLow()] = nil
    cancelTimers(creature:GetGUIDLow())
    -- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
    -- instance->SetBossState(DATA_HADRONOX, DONE). The instance
    -- flag is the ahnkahet-precedent twin (no boss-state-change
    -- event exists).
    creature:SetBossState(DATA_HADRONOX, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.hadronoxDone = true
    end
end

local function hadronoxReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    hadronoxEngaged[creature:GetGUIDLow()] = nil
    hadronoxStep[creature:GetGUIDLow()] = nil -- C++ Reset SetStep(0) twin
    -- No SetData(DATA_HADRONOX_WEBBED_DOORS) write here: C++ has no
    -- Reset() override at all — _doorsWebbed is cleared only in the
    -- ctor (:162) and InitializeAI (:284, spawn-time), and
    -- EnterEvadeMode (:258-270) never resets it, so a webbed wipe
    -- keeps "Hadronox Denied" failed until respawn/instance reset.
    -- A per-attempt re-arm here would invent attainability C++
    -- denies; a fresh motion reads 0, the ctor default.
end

-- C++ boss_hadronoxAI::SetStep (boss_hadronox.cpp:173-185): early-return
-- while engaged (_lastPlayerCombatState); else latch the step, go passive,
-- re-home to the step waypoint, stop attacking, and MovePoint there.
-- The arrival leg rides hadronoxMovementInform (ev28) below.
local function hadronoxSetStep(hadronox, step)
    if hadronoxEngaged[hadronox:GetGUIDLow()] then
        return
    end
    local pos = HADRONOX_STEPS[step + 1]
    hadronoxStep[hadronox:GetGUIDLow()] = step
    hadronox:SetReactState(REACT_PASSIVE)
    hadronox:SetHomePosition(pos[1], pos[2], pos[3], 0)
    hadronox:AttackStop()
    hadronox:MoveTo(0, pos[1], pos[2], pos[3])
end

-- C++ boss_hadronoxAI::MovementInform (boss_hadronox.cpp:197-208):
-- SetReactState(AGGRESSIVE) on EVERY step arrival; only at the final
-- step (_step == NUM_STEPS-1 == 3): web both doors + _doorsWebbed latch
-- (the achievement_hadronox_denied gate; Go case in
-- engine/world/achievement_criteria_scripts.go) + DoZoneInCombat. The
-- web spells' aura-removal effects ride the unported
-- spell_hadronox_web_doors SpellScript — the casts themselves are the
-- ported arm (the DoCastAOE self-cast convention).
local function hadronoxMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    creature:SetReactState(REACT_AGGRESSIVE)
    local step = hadronoxStep[creature:GetGUIDLow()] or 0
    if step < 3 then
        return
    end
    creature:CastSpell(creature, SPELL_WEB_FRONT_DOORS)
    creature:CastSpell(creature, SPELL_WEB_SIDE_DOORS)
    creature:SetData(DATA_HADRONOX_WEBBED_DOORS, 1)
    creature:DoZoneInCombat()
end

-- C++ npc_anub_ar_crusher::DamageTaken (boss_hadronox.cpp:533-540):
-- if (!_hadFrenzy && me->HealthBelowPctDamaged(25, damage)): latch,
-- Talk(CRUSHER_EMOTE_FRENZY 2), DoCastSelf(SPELL_FRENZY 53801). The
-- HealthBelowPctDamaged check is a pre-damage integer comparison
-- ((health - damage) < floor(maxHealth*25/100), Unit.h:873) and fires
-- on lethal hits — no postDamage guard. The C++ latch resets only in
-- the AI ctor, so the Lua mirror wipes the GUID entry on death (fresh
-- latch on respawn).
local crusherFrenzy = {}

local function crusherDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if crusherFrenzy[guid] then
        return false, damage
    end
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth ~= 0 and (health - damage) < math.floor(maxHealth * 25 / 100) then
        crusherFrenzy[guid] = true
        creature:Talk(CRUSHER_EMOTE_FRENZY)
        creature:CastSpell(creature, SPELL_CRUSHER_FRENZY)
    end
    return false, damage
end

-- C++ npc_anub_ar_crusher::_JustEngagedWith: EVENT_SMASH
-- DoCastVictim(SMASH 53318), 8-12s init, 13-21s repeat, scheduled
-- unconditionally (all packs).
local function crusherSmashTick(creature, guid)
    creature:CastSpell(nil, SPELL_CRUSHER_SMASH)
    schedule(guid, "smash", math.random(13000, 21000), function()
        crusherSmashTick(creature, guid)
    end)
end

local function crusherEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "smash", math.random(8000, 12000), function()
        crusherSmashTick(creature, guid)
    end)
end

local function crusherLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function crusherReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function crusherDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    crusherFrenzy[creature:GetGUIDLow()] = nil
    -- C++ npc_anub_ar_crusherAI::JustDied (boss_hadronox.cpp:554-557) ->
    -- hadronox DoAction(ACTION_HADRONOX_MOVE) (:248-254): if _step <
    -- NUM_STEPS-1 (3), SetStep(_step + 1) + Talk(HADRONOX_EMOTE_MOVE).
    -- The SetStep movement machine (react/home/MovePoint) stays out;
    -- the Talk leg ports via a 250yd grid sweep for the unique
    -- hadronox (instance-lookup twin, rotface relayProfessor idiom:
    -- first alive match), step-gated exactly like C++.
    for _, hadronox in ipairs(creature:GetCreaturesInGrid(ENTRY_HADRONOX, 250)) do
        if hadronox:IsAlive() then
            local step = hadronoxStep[hadronox:GetGUIDLow()] or 0
            if step < 3 then
                -- C++ DoAction(ACTION_HADRONOX_MOVE) :248-254 — the Talk
                -- fires even when SetStep early-returns (engaged); only
                -- the movement is suppressed, so Talk stays outside.
                hadronoxSetStep(hadronox, step + 1)
                hadronox:Talk(HADRONOX_EMOTE_MOVE)
            end
            break
        end
    end
end

RegisterCreatureEvent(ENTRY_HADRONOX, 5, hadronoxSpawn)
RegisterCreatureEvent(ENTRY_HADRONOX, 28, hadronoxMovementInform)
RegisterCreatureEvent(ENTRY_HADRONOX, 1, hadronoxEnterCombat)
RegisterCreatureEvent(ENTRY_HADRONOX, 2, hadronoxLeaveCombat)
RegisterCreatureEvent(ENTRY_HADRONOX, 4, hadronoxDied)
RegisterCreatureEvent(ENTRY_HADRONOX, 23, hadronoxReset)
RegisterCreatureEvent(ENTRY_CRUSHER, 1, crusherEnterCombat)
RegisterCreatureEvent(ENTRY_CRUSHER, 2, crusherLeaveCombat)
RegisterCreatureEvent(ENTRY_CRUSHER, 4, crusherDied)
RegisterCreatureEvent(ENTRY_CRUSHER, 9, crusherDamageTaken)
RegisterCreatureEvent(ENTRY_CRUSHER, 23, crusherReset)
