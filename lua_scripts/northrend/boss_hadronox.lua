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
-- precedent); BossAI _Reset/_JustDied instance bookkeeping has no bridge.
-- Ported arms (C++-exact for the one modeled arm):
-- boss_hadronox: EVENT_PIERCE_ARMOR DoCastVictim(Pierce Armor 53418),
-- randtime(4s,7s) init (C++ JustEngagedWith schedule), 10-15s repeat
-- (math.random — slad_ran precedent). The C++ JustEngagedWith arms are
-- independent schedulers: none of the unmodeled events gate PIERCE_ARMOR,
-- and the missing arms do not change its timing, so the arm stands alone
-- without the intro/movement machinery.
-- Unmodeled (no bridges — documented, not wired):
-- The whole pre-fight/step movement machine — InitializeAI
-- SetReactState(REACT_AGGRESSIVE)/SetFloatValue bounding/combat reach
-- (no react-state or float-value bridges — drakkari_colossus precedent)
-- + SetStep(0); SetStep: _lastPlayerCombatState latch, SetReactState(
-- REACT_PASSIVE), SetHomePosition(hadronoxStep[step]),
-- MotionMaster Clear + MovePoint — motion-master bridge absent; the
-- DoAction(ACTION_HADRONOX_MOVE) driver behind the no-DoAction bridge
-- (drakkari_colossus precedent) — so Talk(HADRONOX_EMOTE_MOVE 1) is
-- unbridged too.
-- SummonCrusherPack(SUMMON_GROUP_CRUSHER_1/2/3): SummonCreatureGroup +
-- per-summon SetData(DATA_CRUSHER_PACK_ID) + DoAction(ACTION_PACK_WALK)
-- (JustAppeared leg; DoAction(ACTION_CRUSHER_ENGAGED) re-summon legs) —
-- summon STRAND + cross-AI SetData/DoAction bridges absent.
-- MovementInform final-step: SetReactState(REACT_AGGRESSIVE) +
-- DoCastAOE(WEB_FRONT_DOORS 53177) + DoCastAOE(WEB_SIDE_DOORS 53185)
-- + _doorsWebbed latch + DoZoneInCombat — motion-master bridge absent
-- anyway; no DoCastAOE bridge (terestian/shazzrah precedent).
-- GetData(DATA_HADRONOX_ENTERED_COMBAT/DATA_HADRONOX_WEBBED_DOORS) +
-- SetGUID(_anubar GUID list) + ObjectAccessor cross-AI — cross-AI
-- GetData/SetGUID bridges absent.
-- CanAIAttack 70.0f home-distance leash — no leash bridge.
-- JustEngagedWith me->setActive(true) leg — no active-flag bridge.
-- EVENT_LEECH_POISON (DoCastAOE(SPELL_LEECH_POISON 53030), 5-7s init,
-- 7-9s repeat) — no DoCastAOE bridge.
-- EVENT_ACID_CLOUD (DoCast(SelectTarget(Random, 0, 100.0f), ACID_CLOUD
-- 53400), 7-13s init, 16-23s repeat) — random-target SelectTarget
-- bridge absent (cairne/kazzak precedent).
-- EVENT_WEB_GRAB (DoCastAOE(SPELL_WEB_GRAB 57731), 13-19s init,
-- 20-25s repeat) — no DoCastAOE bridge.
-- EVENT_PLAYER_CHECK (1s): GetCombatManager PvE-ref IsControlledByPlayer
-- flip vs _lastPlayerCombatState -> instance->CheckRequiredBosses gate,
-- Point-motion cancel + AttackStart(victim), else EnterEvadeMode(
-- NO_HOSTILES) — instance-script model absent (standing), motion-master
-- bridge absent.
-- The DoAction machine (no DoAction bridge): ACTION_CRUSHER_ENGAGED
-- (instance SetBossState(DATA_HADRONOX, IN_PROGRESS) + _enteredCombat
-- latch + SummonCrusherPack(2/3)) — instance model absent.
-- EnterEvadeMode: GetCreatureListWithEntryInGrid(NPC_WORLDTRIGGER_LARGE
-- 23472) aura-scan (periodic-summon/web-door auras) + _DespawnAtEvade(25s)
-- legs, summons.DespawnAll (terestian precedent), _anubar ObjectAccessor::
-- GetCreature DespawnOrUnsummon — despawn + summon + ObjectAccessor
-- bridges absent.
-- UpdateAI: UpdateVictim gate + HasUnitState(UNIT_STATE_CASTING) skip —
-- unit-state bridge absent.
-- DamageTaken NPC safeguard (non-player damage, HealthBelowPct(70):
-- HealthBelowPctDamaged(5) -> damage=0 else damage *= (GetHealthPct()-5)/
-- 65) — no DamageTaken hook (npc_unkor_the_ruthless precedent) + no
-- health-pct bridge (doomwalker precedent).
-- JustSummoned summons.Summon leg — summon STRAND absent.
-- npc_anub_ar_crusher — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED (28922 from
-- the file's Creatures enum — NPC_CRUSHER — kalecgos pass): zero
-- registration. The whole npc_hadronox_crusherPackAI engagement model is
-- unbridged: SetData(DATA_CRUSHER_PACK_ID) -> SetReactState(REACT_PASSIVE)
-- (react-state + cross-AI SetData bridges absent), DoAction(
-- ACTION_PACK_WALK) -> MotionMaster MovePoint(_positions[pack]) + the
-- _doFacing SetFacingTo choreography (motion-master + DoAction bridges
-- absent), MovementInform, MoveInLineOfSight passive-engage leg
-- (CanStartAttack + IsWithinDistInMap(attackDistance + m_CombatDistance)
-- -> JustEngagedWith — react-state + distance bridges absent), group
-- pack-aggro JustEngagedWith (GetCreatureListWithEntryInGrid 40.0f,
-- same-pack SetReactState(AGGRESSIVE) + AttackStart — grid + cross-AI
-- bridges absent), EnterEvadeMode -> hadronox AI EnterEvadeMode
-- (cross-AI absent). Port-pattern-ready _JustEngagedWith arms — Talk(
-- CRUSHER_SAY_AGGRO 1) (bridgeable in isolation) + EVENT_SMASH DoCastVictim
-- (SMASH 53318) 8-12s init, 13-21s repeat + pack-1-only instance DoAction(
-- ACTION_CRUSHER_ENGAGED) (DoAction bridge absent) — stay unported:
-- wiring the engage arms without the passive pack model leaves the
-- creature engine-engaging players where C++ holds it REACT_PASSIVE
-- (amanitar mushrooms precedent — the C++-exact port bar fails).
-- DamageTaken frenzy (HealthBelowPctDamaged(25) -> _hadFrenzy latch +
-- Talk(CRUSHER_EMOTE_FRENZY 2) + DoCastSelf(FRENZY 53801)) — no
-- DamageTaken hook + no health-pct bridge. JustDied -> hadronox AI
-- DoAction(ACTION_HADRONOX_MOVE) — DoAction bridge absent.
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
-- DONE aura-remove + caster Z >= 750.0f) — no SpellScript/AuraScript
-- binding bridge on the Lua surface (razelikh precedent) —
-- documented-only.
-- spell_hadronox_leeching_poison (53030 AuraScript: OnEffectRemove
-- AURA_REMOVE_BY_DEATH (non-guardian) -> caster CastSpell(caster,
-- LEECH_POISON_HEAL 53800, triggered)) — no AuraScript binding bridge.
-- spell_hadronox_web_doors (53177/53185 SpellScript: OnEffectHitTarget
-- removes the three periodic-summon auras from the target) — no
-- SpellScript binding bridge.
-- achievement_hadronox_denied — AchievementCriteriaScript (target ToCreature
-- -> AI GetData(DATA_HADRONOX_WEBBED_DOORS)) — no achievement-criteria
-- bridge (snakes precedent) + cross-AI GetData bridge absent —
-- documented-only.

local ENTRY_HADRONOX = 28921

local SPELL_PIERCE_ARMOR = 53418

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
    creature:CastSpell(nil, SPELL_PIERCE_ARMOR)
    local delay = math.random(10000, 15000)
    schedule(guid, "piercearmor", delay, function()
        pierceArmorTick(creature, guid)
    end)
end

local function hadronoxEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "piercearmor", math.random(4000, 7000), function()
        pierceArmorTick(creature, guid)
    end)
end

local function hadronoxLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function hadronoxDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function hadronoxReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_HADRONOX, 1, hadronoxEnterCombat)
RegisterCreatureEvent(ENTRY_HADRONOX, 2, hadronoxLeaveCombat)
RegisterCreatureEvent(ENTRY_HADRONOX, 4, hadronoxDied)
RegisterCreatureEvent(ENTRY_HADRONOX, 23, hadronoxReset)
