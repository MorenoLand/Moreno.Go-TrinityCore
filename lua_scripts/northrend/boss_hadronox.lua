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
-- Ported arms (C++-exact for the modeled arms):
-- boss_hadronox: EVENT_PIERCE_ARMOR DoCastVictim(Pierce Armor 53418),
-- randtime(4s,7s) init (C++ JustEngagedWith schedule), 10-15s repeat
-- (math.random — slad_ran precedent). The C++ JustEngagedWith arms are
-- independent schedulers: none of the unmodeled events gate PIERCE_ARMOR,
-- and the missing arms do not change its timing, so the arm stands alone
-- without the intro/movement machinery.
-- EVENT_LEECH_POISON — DoCastAOE(Leech Poison 53030) self-cast,
-- randtime(5s,7s) init, randtime(7s,9s) repeat (math.random); the
-- AuraScript's heal-on-death leg (53800) stays out (no AuraScript
-- bridge).
-- EVENT_WEB_GRAB — DoCastAOE(Web Grab 57731) self-cast,
-- randtime(13s,19s) init, randtime(20s,25s) repeat (math.random).
-- EVENT_ACID_CLOUD — DoCast(SelectTarget(Random, 0, 100.0f), ACID_CLOUD
-- 53400), randtime(7s,13s) init, randtime(16s,23s) repeat (math.random);
-- position=0 so no threat-head exclusion; threat-list pool via
-- GetAITargets (lua_creature_events.go:345; midnight precedent).
-- npc_anub_ar_crusher DamageTaken (boss_hadronox.cpp:533-540):
-- pre-damage HealthBelowPctDamaged(25) latch -> Talk(2) +
-- DoCastSelf(SPELL_FRENZY 53801); latch wiped on event 4 (C++ ctor
-- reset). The crusher's engage arms (CRUSHER_SAY_AGGRO etc.) stay
-- unmodeled — they ride the unported pack-walk machine.
-- Unmodeled (no bridges — documented, not wired):
-- The whole pre-fight/step movement machine — InitializeAI
-- SetReactState(REACT_AGGRESSIVE)/SetFloatValue bounding/combat reach
-- (SetReactState IS bridged — frozen_core precedent,
-- lua_creature_events.go:524; SetHomePosition IS bridged on the event
-- surface — 796 tick; SetFloatValue has NO Lua bridge — verified
-- absent from lua_creature_events.go/object.go)
-- + SetStep(0); SetStep: _lastPlayerCombatState latch, SetReactState(
-- REACT_PASSIVE), SetHomePosition(hadronoxStep[step]),
-- MotionMaster Clear + MovePoint — motion-master bridge absent; the
-- DoAction(ACTION_HADRONOX_MOVE) driver behind the no-DoAction bridge
-- (drakkari_colossus precedent) — so Talk(HADRONOX_EMOTE_MOVE 1) is
-- unbridged too. A partial InitializeAI port would strand the boss
-- passive at step 0 — the machine stays out as a unit.
-- SummonCrusherPack(SUMMON_GROUP_CRUSHER_1/2/3): SummonCreatureGroup +
-- per-summon SetData(DATA_CRUSHER_PACK_ID) + DoAction(ACTION_PACK_WALK)
-- (JustAppeared leg; DoAction(ACTION_CRUSHER_ENGAGED) re-summon legs) —
-- summon STRAND + cross-AI SetData/DoAction bridges absent.
-- MovementInform final-step: SetReactState(REACT_AGGRESSIVE) +
-- DoCastAOE(WEB_FRONT_DOORS 53177) + DoCastAOE(WEB_SIDE_DOORS 53185)
-- + _doorsWebbed latch + DoZoneInCombat — stays out: the whole arm
-- fires on the unbridged motion-master step machine, and both
-- spells' effects are delivered by the spell_hadronox_web_doors
-- SpellScript (aura removal — shazzrah precedent).
-- GetData(DATA_HADRONOX_ENTERED_COMBAT/DATA_HADRONOX_WEBBED_DOORS) +
-- SetGUID(_anubar GUID list) + ObjectAccessor cross-AI — cross-AI
-- GetData/SetGUID bridges absent.
-- CanAIAttack 70.0f home-distance leash — no leash bridge.
-- JustEngagedWith me->setActive(true) leg — no active-flag bridge.
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
-- 65) — Eluna event 9 (DamageTaken) IS bridged and GetHealth/
-- GetMaxHealth exist, so HealthBelowPct(70)/HealthBelowPctDamaged(5)
-- are now computable; the unit stays blocked on its unregistered
-- owner + unbridged DoAction/MovePoint engage model, not on the
-- health/DamageTaken surface.
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
-- Talk(CRUSHER_EMOTE_FRENZY 2) + DoCastSelf(FRENZY 53801)) — the old
-- "no DamageTaken hook + no health-pct bridge" claim was stale
-- (event 9 carries pre-damage damage; the frenzy arm is PORTED below).
-- Hadronox's own 5% mitigation arm (boss_hadronox.cpp:361-369:
-- HealthBelowPct(70) / HealthBelowPctDamaged(5, damage) zeroing +
-- GetHealthPct() damage scaling) needs attacker->IsControlledByPlayer(),
-- which has no Lua-surface bridge (the Go mirror
-- creatureIsPlayerControlled, boss_ai.go:894, is unexported) — that
-- missing attacker-classification bridge is the true blocker, and a
-- GetObjectType()=="Player" approximation would wrongly reduce
-- pet/charm damage that C++ lets through at full). JustDied ->
-- hadronox AI
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
local ENTRY_CRUSHER = 28922

local SPELL_PIERCE_ARMOR = 53418
local SPELL_LEECH_POISON = 53030
local SPELL_WEB_GRAB = 57731
local SPELL_ACID_CLOUD = 53400
local SPELL_CRUSHER_FRENZY = 53801
local CRUSHER_EMOTE_FRENZY = 2

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

-- C++ EVENT_LEECH_POISON: DoCastAOE; randtime(5s,7s) init,
-- randtime(7s,9s) repeat.
local function leechPoisonTick(creature, guid)
    creature:CastSpell(creature, SPELL_LEECH_POISON)
    schedule(guid, "leechpoison", math.random(7000, 9000), function()
        leechPoisonTick(creature, guid)
    end)
end

-- C++ EVENT_WEB_GRAB: DoCastAOE; randtime(13s,19s) init,
-- randtime(20s,25s) repeat.
local function webGrabTick(creature, guid)
    creature:CastSpell(creature, SPELL_WEB_GRAB)
    schedule(guid, "webgrab", math.random(20000, 25000), function()
        webGrabTick(creature, guid)
    end)
end

-- C++ EVENT_ACID_CLOUD (boss_hadronox.cpp:314-317):
-- DoCast(SelectTarget(Random, 0, 100.0f), ACID_CLOUD 53400);
-- position=0 — no threat-head exclusion (contrast skeram position=1).
-- Threat-list pool via GetAITargets (lua_creature_events.go:345;
-- midnight precedent), C++ dist=100.0f = max range; DoCast default
-- triggered=false (vaelastrasz convention). Init randtime(7s,13s)
-- (:229), repeat randtime(16s,23s) (math.random).
local function acidCloudTick(creature, guid)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if creature:IsWithinDist(t, 100.0) then
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

local function hadronoxEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
    -- C++ EnterEvadeMode: _DespawnAtEvade(25s) on self + aura triggers
    -- (cross-creature legs unbridged) + summons.DespawnAll() (moot —
    -- nothing modeled) (boss_hadronox.cpp:258-268); the 25s is
    -- respawn-side, the despawn itself is immediate
    -- (ScriptedCreature.cpp:616).
    creature:Despawn()
end

local function hadronoxDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function hadronoxReset(event, creature)
    cancelTimers(creature:GetGUID())
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
    local guid = creature:GetGUID()
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

local function crusherDied(event, creature, killer)
    crusherFrenzy[creature:GetGUID()] = nil
end

RegisterCreatureEvent(ENTRY_HADRONOX, 1, hadronoxEnterCombat)
RegisterCreatureEvent(ENTRY_HADRONOX, 2, hadronoxLeaveCombat)
RegisterCreatureEvent(ENTRY_HADRONOX, 4, hadronoxDied)
RegisterCreatureEvent(ENTRY_HADRONOX, 23, hadronoxReset)
RegisterCreatureEvent(ENTRY_CRUSHER, 4, crusherDied)
RegisterCreatureEvent(ENTRY_CRUSHER, 9, crusherDamageTaken)
