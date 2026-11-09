-- Professor Putricide (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_professor_putricide.cpp
-- (1517 lines incl. license; 3 CreatureScripts
-- (boss_professor_putricide (BossAI, DATA_PROFESSOR_PUTRICIDE = 6),
-- npc_volatile_ooze (npc_putricide_oozeAI, ScriptedAI),
-- npc_gas_cloud (npc_putricide_oozeAI, ScriptedAI)) + 16 SpellScript
-- classes (19 RegisterSpellScript registrations — the 16 below +
-- gaseous_bloat, ooze_tank_protection, mutated_plague registered as
-- spell scripts; spell_putricide_mutation_init registered as a
-- SpellScript + AuraScript pair)
-- (spell_putricide_ooze_channel, spell_putricide_slime_puddle,
-- spell_putricide_slime_puddle_aura, spell_putricide_unstable_experiment,
-- spell_putricide_ooze_eruption_searcher, spell_putricide_choking_gas_bomb,
-- spell_putricide_unbound_plague, spell_putricide_eat_ooze,
-- spell_putricide_mutation_init, spell_putricide_mutated_transformation,
-- spell_putricide_mutated_transformation_dmg, spell_putricide_regurgitated_ooze,
-- spell_putricide_clear_aura_effect_value, spell_stinky_precious_decimate,
-- spell_abomination_mutated_transformation, spell_putricide_choking_gas_filter)
-- + 5 AuraScripts (spell_putricide_gaseous_bloat,
-- spell_putricide_ooze_tank_protection, spell_putricide_mutated_plague,
-- spell_putricide_mutation_init_aura, spell_putricide_mutated_transformation_dismiss);
-- all registered from inside AddSC_boss_professor_putricide();
-- loader decl 177 / call 372 per northrend_script_loader.cpp — the
-- SEVENTH group of the "// Icecrown Citadel" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_rotface()
-- (call 371); the call after it is AddSC_boss_valithria_dreamwalker()
-- (call 376) — verified from the loader this run; the checkpoint
-- sequence (boss_rotface -> boss_professor_putricide) is followed).
-- Entry: 36678 Professor Putricide (icecrown_citadel.h
-- NPC_PROFESSOR_PUTRICIDE, line 234;
-- icecrown_citadel.h DATA_PROFESSOR_PUTRICIDE = 6, line 80;
-- instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_PROFESSOR_PUTRICIDE, line 244) — entry-verifiable,
-- registration proceeds (the nexus_commanders kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_professor_putricide" hits
-- boss_professor_putricide.cpp (+ the loader decl/call lines)
-- only; this clone carries no sql/ tree, so ScriptName bindings
-- are DB-side by construction. No putricide lua existed.
--
-- DOCUMENTED-BLOCKED (no registration beyond entry 36678):
-- boss_professor_putricide UpdateAI — PORTED (2026-10-13):
-- UpdateAI UNIT_STATE_CASTING gates (:572 pre-loop, :697 in-loop)
-- PORTED 2026-11-04: slimePuddleTick + onBerserk defer 500ms while
-- IsCasting (mal_ganis idiom); DamageTaken :347 CASTING return guards
-- only the unported phase latches — checked, no gate ported there.
-- EVENT_BERSERK single-fire 10min all phases — Talk(SAY_BERSERK
-- 12) + DoCastSelf(SPELL_BERSERK2 47008) non-triggered
-- (:266/:598-600); the old "no-timer-bridge" claim for this arm
-- was stale doctrine (maiden CreateLuaEvent convention).
-- Talk(EMOTE_UNSTABLE_EXPERIMENT 5) rides EVENT_UNSTABLE_EXPERIMENT
-- (:612-615) — PORTED 2026-11-09 (unstableExperimentTick + ev5
-- RegisterSpellEvent handler below, dormant until Spell.dbc ships).
-- (:612-615) — OUT: P1&&P2-gated, cancelled at the P2->P3
-- transition (:531) — phase machine (gothik precedent). Talk(
-- EMOTE_MALLEABLE_GOO 9) ×2 rides EVENT_MALLEABLE_GOO (:630-650)
-- — OUT: the random-target leg is bridged (GetPlayersInWorld +
-- math.random, nightbane/midnight precedent); the true blockers are
-- the P1->P2 tear-gas phase machine (boss_professor_putricide.cpp:524-525)
-- and the Is25ManRaid branch (:631; Is25ManRaid is bridged, so the
-- tear-gas phase machine is the sole blocker).
-- Talk(EMOTE_CHOKING_GAS_BOMB 10) rides EVENT_CHOKING_GAS_BOMB
-- (:652-655) — OUT: P2/P3 phase-gated (:526). Talk(
-- SAY_FESTERGUT_DEATH 1) rides EVENT_FESTERGUT_DIES (:579-581)
-- — relay-PORTED from festergut JustDied (Talk + EnterEvadeMode,
-- boss_festergut.lua:499-509); Talk(SAY_ROTFACE_DEATH 3) rides
-- EVENT_ROTFACE_DIES (:590-592) — relay-PORTED Talk-only from rotface
-- JustDied (boss_rotface.lua:533-541); its EnterEvadeMode leg is
-- missing there on a stale "no bridge" claim (see DoAction above).
-- Talk(SAY_TRANSFORM_1 7) / Talk(SAY_TRANSFORM_2 8) ride
-- the EVENT_PHASE_TRANSITION legs (:669-690) — OUT: phase machine.
-- EVENT_SLIME_PUDDLE (:602-609) PORTED 2026-10-14: SelectTargetList(
-- 2, Random, 0, 0.0f, playerOnly=TRUE) -> up to 2 random players x
-- DoCast(target, SPELL_SLIME_PUDDLE_TRIGGER 70341) non-triggered,
-- 10s init (:267), Repeat(35s) OUTSIDE the target-empty guard
-- (re-arm unconditional); the stale "no SelectTarget bridge" claim
-- for this arm is retracted — C++-exact since 2026-10-18 via the
-- handler-surface GetAITargets twin (lua_creature_events.go): threat
-- list filtered playerOnly + alive, replacing the retired
-- GetPlayersInWorld 100yd approx (maiden/jaraxxus/curator precedent).
-- EVENT_TEAR_GAS (:617-619)
-- and EVENT_RESUME_ATTACK (:621-628) OUT: phase-transition machine
-- (MovePoint + tear-gas chain). EVENT_UNBOUND_PLAGUE (:657-663)
-- OUT: heroic-gated (:270) + NonTankTargetSelector condition leg
-- (:658). EVENT_MUTATED_PLAGUE (:665-667) OUT: P3-gated (:530).
-- EVENT_FESTERGUT_GOO (:583-588) OUT: heroic-gated (:431) +
-- PHASE_FESTERGUT-driven + triggered MAX_TARGETS leg (:584).
-- EVENT_ROTFACE_OOZE_FLOOD (:594-596) OUT: DoAction-phase-driven.
-- The _baseSpeed SetSpeedRate legs (MovementInform waypoints
-- :377/:384/:391, DoAction ACTION_FESTERGUT/ROTFACE_COMBAT
-- :426/:442, DoAction ACTION_CHANGE_PHASE :483) ride the unported
-- phase machine — the SetSpeedRate
-- bridge exists (lua_creature_events.go) but the machine gates it.
-- DamageTaken 80%/35% latches (:347-367) PORTED 2026-11-09: ev9 +
-- HealthAbovePct + SetReactState(PASSIVE) + DoAction(
-- ACTION_CHANGE_PHASE -366780), local phase flip in the latch (C++
-- flips _phase inside DoAction :523-531; the old ":512-528" cite was
-- wrong — that range is the heroic variable-cast block + phase
-- switch). The downstream tear-gas/MovementInform machine stays out;
-- the latch trigger itself is portable (the header's "feeds the full
-- machine" classification is split: trigger PORT, machine STAY-OUT).
-- Header corrected 2026-10-14: the old "no-timer-bridge" blanket
-- was disproved (CreateLuaEvent is a real registered bridge,
-- engine/scripting/runtime.go:798); each arm is now classified
-- individually against the C++.
-- boss_professor_putricide DoAction — tick-556 verdict (2026-11-09):
-- all six FESTERGUT/ROTFACE arms STAY OUT of the ev38 receiver (no
-- code change); only ACTION_CHANGE_PHASE ports (below). A receiver
-- branch for any of the six would be dead code: no Lua sender
-- dispatches ev38 to Putricide with any of them (festergut/rotface
-- engage, death and inhale handlers relay directly or dispatch
-- nothing — verified in boss_festergut.lua / boss_rotface.lua this
-- tick; no "-36626x" DoAction exists anywhere in lua_scripts/).
-- ACTION_FESTERGUT_COMBAT (-366260, icecrown_citadel.h:490; :424-432):
-- SetPhase (no bridge) + MovePoint(POINT_FESTERGUT :427, unbridged)
-- + EngagementStart (no Lua method) gate it; SetSpeedRate (:426,
-- bridged lua_creature_events.go:5492) + SetReactState(PASSIVE :429,
-- bridged :4266) serve only the unported watch-position move, and
-- the heroic EVENT_FESTERGUT_GOO schedule (:431) rides the OUT
-- handler — no live leg.
-- ACTION_FESTERGUT_GAS (-366261; :433-436): BOTH legs already
-- relay-ported from festergut onInhaleBlight 3rd (boss_festergut.lua:
-- 320-326: Talk(SAY_FESTERGUT_GASEOUS_BLIGHT 0) + triggered self-cast
-- SPELL_RELEASE_GAS_VISUAL 69125); the MovementInform POINT_FESTERGUT
-- arrival sender (:378) is moot. (Corrects the old ":290-296" cite.)
-- ACTION_FESTERGUT_DEATH (-366262; :437-439): downstream
-- EVENT_FESTERGUT_DIES (:579-581, Talk(1) + EnterEvadeMode) already
-- relay-ported from festergut JustDied (boss_festergut.lua:499-509,
-- 4s timer on the festergut GUID).
-- ACTION_ROTFACE_COMBAT (-366270; :440-471): SetPhase + MovePoint
-- (POINT_ROTFACE :443) + EngagementStart gate it; the ooze-flood
-- dummy selection (:448-469) is individually bridgeable —
-- GetCreaturesInGrid(NPC_PUDDLE_STALKER 37013, 50) + GetZ height
-- filter (stalker Z > rotface Z + 5.0, :210-221) + GetDistance sort
-- (object.go generic fallback) + math.random pick without
-- replacement into _oozeFloodDummyGUIDs[0..3] — but its sole
-- consumer ACTION_ROTFACE_OOZE is unreachable (below), so the array
-- would be dead state.
-- ACTION_ROTFACE_OOZE (-366271; :472-478): Talk(SAY_ROTFACE_OOZE_FLOOD
-- 2) + dummy self-cast oozeFloodSpells[stage] {69782, 69796, 69798,
-- 69801} (:168); senders unported — MovementInform POINT_ROTFACE
-- arrival (:385, no ev28) + EVENT_ROTFACE_OOZE_FLOOD self-DoAction
-- (:594-596, scheduled only from :386).
-- ACTION_ROTFACE_DEATH (-366272; :479-481): downstream
-- EVENT_ROTFACE_DIES (:590-592, Talk(3) + EnterEvadeMode)
-- relay-ported in part from rotface JustDied (boss_rotface.lua:
-- 533-541, 4500ms timer, Talk only) — the EnterEvadeMode leg is
-- MISSING there on a stale "no bridge" claim: EnterEvadeMode IS
-- bridged (lua_creature_events.go:5051; festergut's relay already
-- uses it). Flag: rotface.lua needs that leg added (not this file).
-- ACTION_CHANGE_PHASE (-366780) PORTED 2026-11-09 via the ev38 receiver:
-- heroic Talk(6) + double triggered Unstable Experiment + 25-man
-- variable split; the non-heroic tear-gas leg, DelayEvents, AttackStop,
-- SetSpeedRate, MovePoint(POINT_TABLE) and scheduler changes stay out
-- with the phase machine.
-- boss_professor_putricide JustEngagedWith instance legs —
-- instance->DoCastSpellOnPlayers(LIGHT_S_HAMMER_TELEPORT) stays out
-- (no bridge); instance->SetBossState(DATA_PROFESSOR_PUTRICIDE,
-- IN_PROGRESS) PORTED 2026-11-06 via creature:SetBossState (the
-- DoZoneInCombat(me) :275 leg is
-- PORTED in the event-1 handler — live bridge
-- engine/world/lua_creature_events.go, 2026-10-23); JustDied DoCastAOE(
-- SPELL_UNHOLY_INFUSION_CREDIT, true) leg PORTED (Is25ManRaid() +
-- HasAura(SPELL_SHADOWS_FATE) gate, creature:Is25ManRaid() bridged;
-- the old "no 25-man bridge" claim retracted);
-- JustReachedHome instance->SetBossState(FAIL) PORTED 2026-11-06
-- via creature:SetBossState on ev24 OnReachHome (phase gate
-- un-evaluable, see putricideReachHome); the MovementInform
-- cross-boss IN_PROGRESS legs (:376/:383, festergut/rotface delayed
-- gate close) stay out — the MovementInform sender is unported.
-- npc_volatile_ooze / npc_gas_cloud (npc_putricide_oozeAI, lines
-- 718-810) — zero Talk lines — REGISTERED 2026-11-09 (tick 498):
-- the bronjahm no-Talk precedent is retracted for AI-bearing adds
-- (the ball-of-flame precedent, tick 497). Base legs PORTED below:
-- Reset (ev23 GetBossState gate + DoZoneInCombat + triggered
-- DoCastAOE aura twin), SpellHitTarget ev15 (hit -> 1s timer +
-- REACT_PASSIVE), SpellHit ev14 (71618 hit -> 1s timer), 1s poller
-- (HasAura(71618) freeze, IsCasting() approx for IsNonMeleeSpellCast
-- — no bridge), volatile CastMainSpell self-cast 70447. Gas-cloud
-- CastMainSpell stays out: 70672 with SPELLVALUE_AURA_STACK 10 has
-- no cast-mod bridge (zero hits; boss_ai.go:18227 precedent). Evade
-- rides ev23 (Go fires ev23 before ev2 on the evade path,
-- creaturemotion.go:4699 — C++ EnterEvadeMode ends in Reset(),
-- CreatureAI.cpp:214-236), so no separate ev2 handler; the poller is
-- cancelled on ev4.
-- The 20 spell scripts / 5 aura scripts: the blanket
-- "no-SpellScript-bridge / no-AuraScript-bridge" queue claims are
-- STALE as of 2026-11-09 — RegisterSpellEvent exists
-- (engine/scripting/runtime.go:994,1303; GetCaster/GetEffectValue/
-- PreventHitDefaultEffect on the spell object) and RegisterAuraEvent
-- exists (runtime.go:1002,1302; OnPeriodic/OnApply/OnRemove/OnProc).
-- Each script is now judged per-hook, not blanket-blocked; the
-- per-hook verdicts from the tick-496 triage (unstable-experiment
-- ev5, volatile-ooze AI, slime-puddle / clear-aura-effect-value /
-- ooze-eruption-searcher / stinky-precious-decimate candidates) are
-- recorded in the checkpoint — the unstable-experiment ev5 gate check
-- PASSED tick 497 (boss_ai.go:3417 fire site, no 70351 default case,
-- GetEffectValue/GetCreaturesInGrid twins live) and the chain is
-- ported below; the ev14/ev15 gate check for non-boss creatures
-- PASSED tick 498 (general creature cast path, boss_ai.go:3996-4069;
-- ev14 args (event, creature, caster, spellId), ev15 (event,
-- creature, target, spellId)) and the ooze AI is ported below.
-- The four spell-script candidates, one hook-fire check each,
-- tick 498: slime-puddle PORTED (ev7 OnObjectAreaTargetSelect,
-- lua_spell.go:738, spellKnown-gated — dormant until Spell.dbc
-- ships; binding 70341 is the TC name-match, spell_script_names
-- is DB-side in this clone); clear-aura-effect-value PORTED on
-- 71620 only (the sole C++-verifiable binding — other bindings
-- DB-side); stinky-precious-decimate PORTED on 71123 (festergut
-- :56 / rotface :77); ooze-eruption-searcher STAYS OUT — ev5 has
-- no fire site for aura-periodic dummy effects (fireSpellEffectHitTarget
-- callers are the cast path only, boss_ai.go:3417/3420).
-- DOCUMENTED-ENTRY EVIDENCE (not registered): icecrown_citadel.h —
-- NPC_VOLATILE_OOZE = 37697 (line 238); NPC_GAS_CLOUD = 37562
-- (line 237); instance_icecrown_citadel.cpp OnCreatureCreate binds
-- both (lines 247–248).
-- All 15 Talk() calls in the file accounted for (15 =
-- SAY_FESTERGUT_GASEOUS_BLIGHT 0 (1, DoAction ACTION_FESTERGUT_GAS)
-- + SAY_FESTERGUT_DEATH 1 (1, EVENT_FESTERGUT_DIES) +
-- SAY_ROTFACE_OOZE_FLOOD 2 (1, DoAction ACTION_ROTFACE_OOZE) +
-- SAY_ROTFACE_DEATH 3 (1, EVENT_ROTFACE_DIES) + SAY_AGGRO 4 (1)
-- + EMOTE_UNSTABLE_EXPERIMENT 5 (1, scheduler) +
-- SAY_PHASE_TRANSITION_HEROIC 6 (1, DoAction ACTION_CHANGE_PHASE)
-- + SAY_TRANSFORM_1 7 (1, EVENT_PHASE_TRANSITION) +
-- SAY_TRANSFORM_2 8 (1, EVENT_PHASE_TRANSITION) +
-- EMOTE_MALLEABLE_GOO 9 (2, scheduler) + EMOTE_CHOKING_GAS_BOMB
-- 10 (1, scheduler) + SAY_KILL 11 (1) + SAY_BERSERK 12 (1,
-- scheduler) + SAY_DEATH 13 (1)).

local ENTRY_PROFESSOR_PUTRICIDE = 36678

local DATA_PROFESSOR_PUTRICIDE = 6 -- icecrown_citadel.h:80
local DATA_ROTFACE = 5 -- icecrown_citadel.h:79
local DATA_FESTERGUT = 4 -- icecrown_citadel.h:78
local BOSS_STATE_NOT_STARTED = 0 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2
local BOSS_STATE_DONE = 3

local SAY_AGGRO = 4
local SAY_KILL = 11
local SAY_DEATH = 13
local SAY_BERSERK = 12
local EMOTE_UNSTABLE_EXPERIMENT = 5 -- boss_professor_putricide.cpp:47

local SPELL_OOZE_TANK_PROTECTION = 71770
local SPELL_MUTATED_PLAGUE_CLEAR = 72618
local SPELL_SHADOWS_FATE = 71169
local SPELL_UNHOLY_INFUSION_CREDIT = 71518
local SPELL_BERSERK2 = 47008
local SPELL_SLIME_PUDDLE_TRIGGER = 70341
local SPELL_UNSTABLE_EXPERIMENT = 70351 -- boss_professor_putricide.cpp:71
local SPELL_OOZE_VARIABLE = 74118 -- boss_professor_putricide.cpp:81
local SPELL_GAS_VARIABLE = 74119 -- boss_professor_putricide.cpp:82
local ACTION_CHANGE_PHASE = -366780 -- boss_professor_putricide.cpp:498
local SAY_PHASE_TRANSITION_HEROIC = 6
local SPELL_MALLEABLE_GOO_H = 72296
local SPELL_GROW_STACKER = 70345
local SPELL_SLIME_PUDDLE_AURA = 70343
local SPELL_GROW = 70347
local SPELL_CHOKING_GAS_BOMB_PERIODIC = 71259
local SPELL_CHOKING_GAS_EXPLOSION_TRIGGER = 71280
local SPELL_TEAR_GAS_CREATURE = 71618 -- boss_professor_putricide.cpp:76
local SPELL_TEAR_GAS_CANCEL = 71620 -- boss_professor_putricide.cpp:77
local SPELL_OOZE_ERUPTION_SEARCH_PERIODIC = 70457 -- boss_professor_putricide.cpp:104
local SPELL_OOZE_ERUPTION = 70492 -- boss_professor_putricide.cpp:102
local SPELL_VOLATILE_OOZE_ADHESIVE = 70447 -- boss_professor_putricide.cpp:103
local SPELL_GASEOUS_BLOAT_PROC = 70215 -- boss_professor_putricide.cpp:99
local SPELL_GASEOUS_BLOAT = 70672 -- boss_professor_putricide.cpp:97
local SPELL_EXPUNGED_GAS = 70701 -- boss_professor_putricide.cpp:100
local SPELL_DECIMATE = 71123 -- boss_festergut.cpp:56 / boss_rotface.cpp:77
local NPC_ABOMINATION_WING_MAD_SCIENTIST_STALKER = 37824 -- icecrown_citadel.h:235

local NPC_MALLEABLE_OOZE_STALKER = 38556
local NPC_GROWING_OOZE_PUDDLE = 37690
local NPC_GAS_CLOUD = 37562
local NPC_VOLATILE_OOZE = 37697
local NPC_CHOKING_GAS_BOMB = 38159
local NPC_MUTATED_ABOMINATION_10 = 37672
local NPC_MUTATED_ABOMINATION_25 = 38285

local timers = {}

-- Per-GUID combat phase (PHASE_COMBAT_1/2/3 = 1/2/3): the Lua model
-- of C++ _phase for the DamageTaken latches only — the downstream
-- tear-gas/MovementInform machine is unported, so the latch itself
-- flips the phase (C++ flips it inside DoAction ACTION_CHANGE_PHASE
-- :523-531), which also stops the latch re-firing on every hit.
local phase = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- Per-GUID experiment stage (C++ _experimentState, DATA_EXPERIMENT_STAGE
-- = 1, :172/:546/:561-562): 0 = OOZE, 1 = GAS; Reset sets OOZE
-- (:241), each Unstable Experiment cast toggles it before selecting
-- the target (:971-972). Lua-local — creature:GetData is the
-- per-motion latch, not the boss-AI data store.
local experimentStage = {}

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

-- C++ EVENT_SLIME_PUDDLE: SelectTargetList(2, Random, 0, 0.0f,
-- playerOnly=TRUE) -> DoCast(target, SPELL_SLIME_PUDDLE_TRIGGER
-- 70341) non-triggered per target, 10s init (:267), Repeat(35s)
-- outside the target-empty guard (re-arm unconditional — the
-- ScheduleEvent at :608 runs even when the list is empty).
-- Threat-list pool via creature:GetAITargets() (handler twin,
-- lua_creature_events.go) filtered playerOnly + alive — C++-exact,
-- retargeted 2026-10-18 off the retired GetPlayersInWorld 100yd
-- approx (maiden/jaraxxus/curator precedent). The 30s phase-
-- transition event delay (events.DelayEvents(30s), ACTION_CHANGE_PHASE)
-- still rides the unported phase machine — unmodeled.
local function randomPlayers(creature, count)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    local targets = {}
    for i = 1, math.min(count, #candidates) do
        targets[i] = table.remove(candidates, math.random(#candidates))
    end
    return targets
end

local function slimePuddleTick(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :572 (+ in-loop :697, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "slimepuddle", 500, function() slimePuddleTick(creature, guid) end)
        return
    end
    for _, target in ipairs(randomPlayers(creature, 2)) do
        creature:CastSpell(target, SPELL_SLIME_PUDDLE_TRIGGER)
    end
    schedule(guid, "slimepuddle", 35000, function()
        slimePuddleTick(creature, guid)
    end)
end

-- C++ EVENT_UNSTABLE_EXPERIMENT (:612-615): Talk(EMOTE_UNSTABLE_
-- EXPERIMENT 5) + DoCast(me, SPELL_UNSTABLE_EXPERIMENT 70351)
-- non-triggered, re-arm {35s,40s}; init {30s,35s} at engage (:268);
-- cancelled on entering phase 3 (:531, in putricideDoAction below).
-- The 70351 payload rides the ev5 RegisterSpellEvent handler below.
local function unstableExperimentTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "unstable", 500, function() unstableExperimentTick(creature, guid) end)
        return
    end
    creature:Talk(EMOTE_UNSTABLE_EXPERIMENT)
    creature:CastSpell(creature, SPELL_UNSTABLE_EXPERIMENT)
    schedule(guid, "unstable", math.random(35000, 40000), function()
        unstableExperimentTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: 10min single-fire, all phases — Talk(SAY_BERSERK) +
-- DoCast(me, SPELL_BERSERK2) non-triggered (:266/:598-600).
local function onBerserk(creature, guid)
    -- C++ boss UpdateAI pre-loop gate :572 (+ in-loop :697, single-tick
    -- no-op per the marrowgar precedent): 500ms IsCasting defer, no
    -- re-arm consumed (the mal_ganis idiom).
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK2)
end

-- C++ boss_professor_putricide::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch has no bridge.
local function putricideJustEngagedWith(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phase[guid] = 1 -- C++ :271 SetPhase(PHASE_COMBAT_1)
    creature:Talk(SAY_AGGRO)
    -- C++ :274 — DoCast(me, SPELL_OOZE_TANK_PROTECTION, true)
    -- (triggered self-cast).
    creature:CastSpell(creature, SPELL_OOZE_TANK_PROTECTION, true)
    creature:DoZoneInCombat() -- boss_professor_putricide.cpp:275 (JustEngagedWith arm; bridge live engine/world/lua_creature_events.go, 2026-10-23)
    creature:SetCombatPulseDelay(5) -- boss_professor_putricide.cpp:276 (JustEngagedWith arm)
    -- C++ :277 — instance->SetBossState(DATA_PROFESSOR_PUTRICIDE,
    -- IN_PROGRESS), last arm of JustEngagedWith (C++ arm order).
    -- PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_PROFESSOR_PUTRICIDE, BOSS_STATE_IN_PROGRESS)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
    -- C++ :267 — events.ScheduleEvent(EVENT_SLIME_PUDDLE, 10s).
    schedule(guid, "slimepuddle", 10000, function() slimePuddleTick(creature, guid) end)
    -- C++ :268 — events.ScheduleEvent(EVENT_UNSTABLE_EXPERIMENT, 30s, 35s).
    schedule(guid, "unstable", math.random(30000, 35000), function() unstableExperimentTick(creature, guid) end)
end

-- C++ Reset/_Reset() timer machinery — cancel/re-arm on combat
-- events (the maiden convention).
local function putricideLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phase[guid] = nil
end

-- C++ boss_professor_putricide::JustReachedHome (:282-286):
-- _JustReachedHome() then SetWalk(false), then SetBossState(FAIL)
-- gated on PHASE_COMBAT_1/2/3. The phase machine is unported and
-- reach-home only fires post-combat in the Lua model, so the gate is
-- always true here; the FAIL leg is PORTED 2026-11-06 via
-- creature:SetBossState on ev24 OnReachHome (the marrowgar ev24
-- precedent).
local function putricideReachHome(event, creature)
    -- C++ :283 — me->SetWalk(false), after _JustReachedHome and
    -- before the SetBossState(FAIL) gate. PORTED 2026-11-09
    -- (lua_creature_events.go:3577).
    creature:SetWalk(false)
    creature:SetBossState(DATA_PROFESSOR_PUTRICIDE, BOSS_STATE_FAIL)
end

-- C++ Reset :236 instance->SetData(DATA_NAUSEA_ACHIEVEMENT, true) and ExtraEffect :1382 set it false:
-- achievement criteria, no achievement bridge (the lich-king no-achievement-criteria-bridge precedent).
-- DATA_EXPERIMENT_STAGE (:971-972) toggles inside
-- spell_putricide_unstable_experiment — PORTED 2026-11-09 (ev5
-- RegisterSpellEvent handler below; stage in the Lua-local
-- experimentStage table, Reset to OOZE). No blanket
-- no-SpellScript-bridge queue claim (RegisterSpellEvent exists).
local function putricideReset(event, creature)
    -- C++ Reset :234-235 — instance->SetBossState(
    -- DATA_PROFESSOR_PUTRICIDE, NOT_STARTED), gated on NOT being in
    -- PHASE_ROTFACE/PHASE_FESTERGUT. The phase machine is unported and
    -- Putricide never enters those phases in the Lua model, so the
    -- gate is always true here (C++ arm order: before events.Reset
    -- :238). PORTED 2026-11-06 via creature:SetBossState. No != DONE
    -- gate: the C++ call is explicit, not _Reset().
    creature:SetBossState(DATA_PROFESSOR_PUTRICIDE, BOSS_STATE_NOT_STARTED)
    cancelTimers(creature:GetGUIDLow())
    phase[creature:GetGUIDLow()] = 1 -- C++ :240 SetPhase(PHASE_COMBAT_1)
    experimentStage[creature:GetGUIDLow()] = nil -- C++ :241 _experimentState = EXPERIMENT_STATE_OOZE (0)
    -- C++ boss_professor_putricide.cpp:242: me->SetReactState(REACT_DEFENSIVE)
    -- (Reset: flip follows events.Reset).
    creature:SetReactState(1) -- REACT_DEFENSIVE
    -- C++ :243 — me->SetWalk(false), after SetReactState. PORTED
    -- 2026-11-09 (lua_creature_events.go:3577).
    creature:SetWalk(false)
    -- C++ :246-249: the RemoveFlag(NOT_SELECTABLE) + SetImmuneToPC(false)
    -- arms ride the Rotface+Festergut DONE instance boss-state gate —
    -- PORTED 2026-11-06: creature:GetBossState is live
    -- (lua_creature_events.go:5936), so the gate is evaluable (the old
    -- "has no bridge" claim is retracted); SetImmuneToPC(false) via the
    -- RemoveFlag(59, 0x100) twin (the blood-prince-council precedent).
    if creature:GetBossState(DATA_ROTFACE) == BOSS_STATE_DONE
        and creature:GetBossState(DATA_FESTERGUT) == BOSS_STATE_DONE then
        creature:RemoveFlag(59, 0x02000000) -- UNIT_FLAG_NOT_SELECTABLE
        creature:RemoveFlag(59, 0x100) -- UNIT_FLAG_IMMUNE_TO_PC
    end
end

-- C++ boss_professor_putricide::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function putricideKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_professor_putricide::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function putricideJustDied(event, creature, killer)
    -- C++ _JustDied() resets the EventMap (boss_professor_putricide.cpp:
    -- JustDied opens with it): cancel the CreateLuaEvent timers — they
    -- have no death auto-cancel, so the self-re-arming slime-puddle leg
    -- would keep firing on the corpse (festergut/rotface death-cancel
    -- precedent).
    cancelTimers(creature:GetGUIDLow())
    phase[creature:GetGUIDLow()] = nil
    -- C++ JustDied :296 — _JustDied() sets DONE, first arm (before
    -- Talk). PORTED 2026-11-06 via creature:SetBossState.
    creature:SetBossState(DATA_PROFESSOR_PUTRICIDE, BOSS_STATE_DONE)
    creature:Talk(SAY_DEATH)
    -- C++ :299-300 — Is25ManRaid() + HasAura(SPELL_SHADOWS_FATE)-gated
    -- DoCastAOE(SPELL_UNHOLY_INFUSION_CREDIT, true) via self-cast
    -- (DoCastAOE = self-cast convention; C++ order kept).
    if creature:Is25ManRaid() and creature:HasAura(SPELL_SHADOWS_FATE) then
        creature:CastSpell(creature, SPELL_UNHOLY_INFUSION_CREDIT, true)
    end
    -- C++ :302 — DoCast(SPELL_MUTATED_PLAGUE_CLEAR), single-arg DoCast
    -- resolves via AISpellInfo, which defaults to AITARGET_SELF
    -- (CreatureAIImpl.h AISpellInfoType ctor), so self-cast.
    creature:CastSpell(creature, SPELL_MUTATED_PLAGUE_CLEAR)
end

-- C++ boss_professor_putricide::JustSummoned (boss_professor_putricide.cpp:305-344):
-- per-entry summon arms. Event 19 (Eluna JustSummoned) fires on the summoner
-- as (event, creature, summoned) — the magtheridon channelerJustSummoned
-- precedent.
local function putricideJustSummoned(event, creature, summon)
    -- C++ opens with summons.Summon(summon) summon-list tracking — no
    -- summon-list bridge, dropped (same as the Reset summons.DespawnAll
    -- arm, which is also unmodeled in putricideReset).
    local entry = summon:GetEntry()
    if entry == NPC_MALLEABLE_OOZE_STALKER then
        -- C++ :311 DoCast(summon, SPELL_MALLEABLE_GOO_H), non-triggered;
        -- the return skips the :342-343 IsInCombat/DoZoneInCombat tail.
        creature:CastSpell(summon, SPELL_MALLEABLE_GOO_H)
        return
    elseif entry == NPC_GROWING_OOZE_PUDDLE then
        -- C++ :314-318: triggered self-casts; the 7x SPELL_GROW loop is
        -- sniff-confirmed (blizzard casts it 7 times initially).
        summon:CastSpell(summon, SPELL_GROW_STACKER, true)
        summon:CastSpell(summon, SPELL_SLIME_PUDDLE_AURA, true)
        for _ = 1, 7 do
            summon:CastSpell(summon, SPELL_GROW, true)
        end
    elseif entry == NPC_GAS_CLOUD then
        -- C++ :320-323: summon->ModifyAuraState(AURA_STATE_UNKNOWN22, true)
        -- has no Lua bridge (no ModifyAuraState on the creature event
        -- surface; engine/world/aura_state.go keeps the mask computed with
        -- no manual override — the twin_valkyr Reset STAY-OUT precedent), so
        -- only the REACT_PASSIVE arm ports.
        summon:SetReactState(0) -- REACT_PASSIVE
    elseif entry == NPC_VOLATILE_OOZE then
        -- C++ :325-328: same shape as gas cloud (AURA_STATE_UNKNOWN19).
        summon:SetReactState(0) -- REACT_PASSIVE
    elseif entry == NPC_CHOKING_GAS_BOMB then
        -- C++ :331-333 triggered self-casts; the return skips the tail.
        summon:CastSpell(summon, SPELL_CHOKING_GAS_BOMB_PERIODIC, true)
        summon:CastSpell(summon, SPELL_CHOKING_GAS_EXPLOSION_TRIGGER, true)
        return
    elseif entry == NPC_MUTATED_ABOMINATION_10 or entry == NPC_MUTATED_ABOMINATION_25 then
        -- C++ :334-337: plain return, skips the tail.
        return
    end
    -- C++ :342-343: the tail every non-returning branch shares —
    -- DoZoneInCombat(summon) rides the summon-object receiver (mandokir
    -- precedent).
    if creature:IsInCombat() then
        summon:DoZoneInCombat()
    end
end

-- C++ boss_professor_putricide::DamageTaken (:347-367): while casting
-- return; PHASE_COMBAT_1 + !HealthAbovePct(80) -> SetReactState(
-- REACT_PASSIVE) + DoAction(ACTION_CHANGE_PHASE); PHASE_COMBAT_2 +
-- !HealthAbovePct(35) -> same. ev9 fires pre-damage
-- (lua_creature_events.go:6955-6972), so HealthAbovePct reads the C++
-- value and NO postDamage guard applies — C++ has none (the
-- HealthAbovePct arm fires on lethal hits too, unlike UpdateAI
-- HealthBelowPct arms). The local phase flips here because C++ flips
-- _phase inside DoAction (:523-531) and the downstream machine is
-- unported; the flip also stops the latch re-firing on every hit.
-- Damage untouched: the handler returns nothing (curator precedent).
local function putricideDamageTaken(event, creature, attacker, damage)
    if creature:IsCasting() then
        return
    end
    local guid = creature:GetGUIDLow()
    local ph = phase[guid] or 1
    if ph == 1 then
        if creature:HealthAbovePct(80) then
            return
        end
        creature:SetReactState(0) -- REACT_PASSIVE
        phase[guid] = 2
        creature:DoAction(ACTION_CHANGE_PHASE)
    elseif ph == 2 then
        if creature:HealthAbovePct(35) then
            return
        end
        creature:SetReactState(0) -- REACT_PASSIVE
        phase[guid] = 3
        creature:DoAction(ACTION_CHANGE_PHASE)
    end
end

-- C++ DoAction ACTION_CHANGE_PHASE (:482-534) receiver. The
-- non-heroic tear-gas leg, DelayEvents, AttackStop, SetSpeedRate,
-- MovePoint(POINT_TABLE) and the scheduler changes ride the unported
-- phase machine and stay out. The heroic legs port: Talk(
-- SAY_PHASE_TRANSITION_HEROIC 6) + two triggered Unstable Experiment
-- self-casts + the 25-man variable split — threat-list players, a
-- random ceil(half) cast Ooze Variable on themselves, the rest Gas
-- Variable (:493-519, GetAITargets + math.random twin).
local function putricideDoAction(event, creature, action)
    if action ~= ACTION_CHANGE_PHASE then
        return
    end
    -- C++ :531 — entering PHASE_COMBAT_3 cancels EVENT_UNSTABLE_
    -- EXPERIMENT; the latch flips phase[guid] to 3 before dispatching
    -- this DoAction, so the gate reads the new phase.
    if (phase[creature:GetGUIDLow()] or 1) == 3 then
        cancelTimer(creature:GetGUIDLow(), "unstable")
    end
    if not creature:IsHeroic() then
        return
    end
    creature:Talk(SAY_PHASE_TRANSITION_HEROIC)
    creature:CastSpell(creature, SPELL_UNSTABLE_EXPERIMENT, true)
    creature:CastSpell(creature, SPELL_UNSTABLE_EXPERIMENT, true)
    if creature:Is25ManRaid() then
        local targets = {}
        for _, unit in ipairs(creature:GetAITargets()) do
            if unit:GetObjectType() == "Player" and not unit:IsDead() then
                targets[#targets + 1] = unit
            end
        end
        local half = math.floor(#targets / 2)
        while half < #targets do
            local target = table.remove(targets, math.random(#targets))
            target:CastSpell(target, SPELL_OOZE_VARIABLE, true)
        end
        for _, target in ipairs(targets) do
            target:CastSpell(target, SPELL_GAS_VARIABLE, true)
        end
    end
end

RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 1, putricideJustEngagedWith)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 2, putricideLeaveCombat)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 3, putricideKilledUnit)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 4, putricideJustDied)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 9, putricideDamageTaken)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 23, putricideReset)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 24, putricideReachHome)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 19, putricideJustSummoned)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 38, putricideDoAction)

-- C++ spell_putricide_unstable_experiment::HandleScript (:963-988,
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT): the stage
-- toggles BEFORE selection (old stage picks the target and the effect
-- value); stage 0 picks the first stalker with a neighbour within
-- 10yd, stage 1 the first singleton — "2 of them are spawned at green
-- place" (:977); no match leaves the LAST stalker in the list as the
-- target; empty list leaves no target. The cast is
-- caster:CastSpell(target, Effects[stage].CalcValue(), true) —
-- spell:GetEffectValue(stage) is the CalcValue twin (lua_spell.go),
-- 0 until Spell.dbc ships, so the leg is dormant until then.
-- Gate check PASSED this tick: ev5 (SpellEventOnEffectHitTarget)
-- fires from the creature cast path (boss_ai.go:3417/3420, per
-- (effect, live hit target), caster-targeted effect takes a full
-- share); the effectIndex == 0 gate is load-bearing — Go fires once
-- per non-zero effect, C++ registers EFFECT_0 only, so without it the
-- stage would toggle twice per cast. No 70351 case exists in
-- creatureEffectScriptEffect (boss_ai.go:7642), so the Go SCRIPT_EFFECT
-- default is a no-op and there is no double-fire. GetCreaturesInGrid
-- is the GetCreatureListWithEntryInGrid twin (lua_creature_events.go,
-- includes self, GUID-sorted — the C++ grid-visit order is
-- unspecified, so the pick among several matching stalkers may differ
-- from C++; the stage/neighbour rule itself is exact).
local function unstableExperimentScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    local guid = caster:GetGUIDLow()
    local stage = experimentStage[guid] or 0
    experimentStage[guid] = 1 - stage
    local stalkers = caster:GetCreaturesInGrid(NPC_ABOMINATION_WING_MAD_SCIENTIST_STALKER, 200)
    if #stalkers == 0 then
        return
    end
    local target = nil
    for _, stalker in ipairs(stalkers) do
        target = stalker
        local near = stalker:GetCreaturesInGrid(NPC_ABOMINATION_WING_MAD_SCIENTIST_STALKER, 10)
        if (stage == 0 and #near > 1) or (stage == 1 and #near == 1) then
            break
        end
    end
    if target == nil then
        return
    end
    local value = spell:GetEffectValue(stage)
    if value <= 0 then
        return
    end
    caster:CastSpell(target, value, true)
end
RegisterSpellEvent(SPELL_UNSTABLE_EXPERIMENT, 5, unstableExperimentScript)

-- C++ npc_putricide_oozeAI (boss_professor_putricide.cpp:718-781),
-- shared base of npc_volatile_ooze (:783-791, aura 70457, hit-target
-- 70492, CastMainSpell self-cast 70447 non-triggered) and
-- npc_gas_cloud (:793-810, aura 70215, hit-target 70701, CastMainSpell
-- 70672 with SPELLVALUE_AURA_STACK 10 — no cast-mod bridge, stays
-- out). Per-GUID _newTargetSelectTimer: the 1s poller below is the
-- UpdateAI model (1000ms timer, 1s granularity — the escape-trash
-- precedent); DoMeleeAttackIfReady has no bridge and is a no-op.
local oozeSelect = {}

local oozeAuraSpell = {
    [NPC_VOLATILE_OOZE] = SPELL_OOZE_ERUPTION_SEARCH_PERIODIC,
    [NPC_GAS_CLOUD] = SPELL_GASEOUS_BLOAT_PROC,
}

local oozeHitTargetSpell = {
    [NPC_VOLATILE_OOZE] = SPELL_OOZE_ERUPTION,
    [NPC_GAS_CLOUD] = SPELL_EXPUNGED_GAS,
}

local function oozePollTick(creature, guid, entry)
    -- C++ UpdateAI (:757-779): no victim and no pending timer ->
    -- return; no pending timer and not casting a non-melee spell ->
    -- timer = 1000 (IsCasting() is the approx twin — IsNonMeleeSpellCast
    -- has no bridge); pending timer + tear-gas aura -> frozen, no
    -- decrement; timer expiry -> CastMainSpell.
    if not oozeSelect[guid] then
        if creature:GetVictim() == nil then
            schedule(guid, "oozepoll", 1000, function() oozePollTick(creature, guid, entry) end)
            return
        end
        if not creature:IsCasting() then
            oozeSelect[guid] = true
        end
    end
    if not oozeSelect[guid] then
        schedule(guid, "oozepoll", 1000, function() oozePollTick(creature, guid, entry) end)
        return
    end
    if creature:HasAura(SPELL_TEAR_GAS_CREATURE) then
        schedule(guid, "oozepoll", 1000, function() oozePollTick(creature, guid, entry) end)
        return
    end
    oozeSelect[guid] = false
    if entry == NPC_VOLATILE_OOZE then
        -- C++ npc_volatile_ooze::CastMainSpell (:787-790).
        creature:CastSpell(creature, SPELL_VOLATILE_OOZE_ADHESIVE)
    end
    -- Gas-cloud CastMainSpell (:803-807) stays out (SPELLVALUE_AURA_STACK).
    schedule(guid, "oozepoll", 1000, function() oozePollTick(creature, guid, entry) end)
end

-- C++ npc_putricide_oozeAI::Reset (:739-746): boss not IN_PROGRESS ->
-- DespawnOrUnsummon (the fall-through DoZoneInCombat/DoCastAOE arms
-- are dead-on-despawn, dropped); else DoZoneInCombat + triggered
-- DoCastAOE(aura) via the self-cast twin + start the 1s poller.
-- Evade rides this same handler (Go fires ev23 before ev2 on the
-- evade path; C++ EnterEvadeMode ends in Reset()).
local function oozeReset(event, creature)
    local guid = creature:GetGUIDLow()
    local entry = creature:GetEntry()
    local aura = oozeAuraSpell[entry]
    if aura == nil then
        return
    end
    if creature:GetBossState(DATA_PROFESSOR_PUTRICIDE) ~= BOSS_STATE_IN_PROGRESS then
        cancelTimers(guid)
        oozeSelect[guid] = nil
        creature:DespawnOrUnsummon()
        return
    end
    oozeSelect[guid] = nil
    creature:DoZoneInCombat()
    creature:CastSpell(creature, aura, true)
    schedule(guid, "oozepoll", 1000, function() oozePollTick(creature, guid, entry) end)
end

-- C++ npc_putricide_oozeAI::JustDied — no override in C++; the
-- poller is a Lua artifact, cancelled here so it never fires on the
-- corpse (the putricide death-cancel precedent).
local function oozeJustDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    oozeSelect[guid] = nil
end

-- C++ npc_putricide_oozeAI::SpellHit (:748-752): tear-gas-creature
-- hit -> _newTargetSelectTimer = 1000 (unconditional reset).
local function oozeSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_TEAR_GAS_CREATURE then
        oozeSelect[creature:GetGUIDLow()] = true
    end
end

-- C++ npc_putricide_oozeAI::SpellHitTarget (:727-737): no pending
-- timer and the hit spell is the entry's hit-target spell ->
-- _newTargetSelectTimer = 1000 + REACT_PASSIVE until next selection.
local function oozeSpellHitTarget(event, creature, target, spellId)
    local guid = creature:GetGUIDLow()
    if not oozeSelect[guid] and spellId == oozeHitTargetSpell[creature:GetEntry()] then
        oozeSelect[guid] = true
        creature:SetReactState(0) -- REACT_PASSIVE
    end
end

RegisterCreatureEvent(NPC_VOLATILE_OOZE, 23, oozeReset)
RegisterCreatureEvent(NPC_VOLATILE_OOZE, 4, oozeJustDied)
RegisterCreatureEvent(NPC_VOLATILE_OOZE, 14, oozeSpellHit)
RegisterCreatureEvent(NPC_VOLATILE_OOZE, 15, oozeSpellHitTarget)
RegisterCreatureEvent(NPC_GAS_CLOUD, 23, oozeReset)
RegisterCreatureEvent(NPC_GAS_CLOUD, 4, oozeJustDied)
RegisterCreatureEvent(NPC_GAS_CLOUD, 14, oozeSpellHit)
RegisterCreatureEvent(NPC_GAS_CLOUD, 15, oozeSpellHitTarget)

-- C++ spell_putricide_slime_puddle::ScaleRange (:927-935):
-- OnObjectAreaTargetSelect EFFECT_0 TARGET_UNIT_DEST_AREA_ENEMY +
-- EFFECT_1 TARGET_UNIT_DEST_AREA_ENTRY — remove targets farther than
-- 2.5x the caster's scale (2D exact distance; the file-local
-- ExactDistanceCheck :911-923 returns dist > _dist, and remove_if
-- drops those). Go ev7 fires per selecting effect (lua_spell.go:738),
-- spellKnown-gated — dormant until Spell.dbc ships. Binding 70341 is
-- the TC name-match (spell_script_names is DB-side in this clone).
local function slimePuddleFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 and effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local limit = 2.5 * caster:GetScale()
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetExactDistance2d(caster) > limit then
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(SPELL_SLIME_PUDDLE_TRIGGER, 7, slimePuddleFilter)

-- C++ spell_putricide_clear_aura_effect_value::HandleScript
-- (:1392-1409): OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT
-- — remove the aura whose id is the effect-0 value (difficulty-mapped;
-- no bridge, raw value used), plus the effect-1 value when the script
-- spell is SPELL_TEAR_GAS_CANCEL (71620 — the sole C++-verifiable
-- binding; other bindings are DB-side). Dormant until Spell.dbc ships
-- (values 0 -> guarded no-op). No 71620 case in
-- creatureEffectScriptEffect (boss_ai.go:7642), so no double-fire.
local function clearAuraEffectValue(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value <= 0 then
        return
    end
    hitTarget:RemoveAura(value)
    local value2 = spell:GetEffectValue(1)
    if value2 > 0 then
        hitTarget:RemoveAura(value2)
    end
end
RegisterSpellEvent(SPELL_TEAR_GAS_CANCEL, 5, clearAuraEffectValue)

-- C++ spell_stinky_precious_decimate::HandleScript (:1416-1430):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT — if the hit
-- unit's health % is above the effect-0 value, set its health to
-- maxHealth * value / 100. Spell 71123 from boss_festergut.cpp:56 /
-- boss_rotface.cpp:77 (DoCastVictim SPELL_DECIMATE). Dormant until
-- Spell.dbc ships (value 0 -> guarded no-op; without the guard a 0
-- value would zero the target's health). No 71123 case in
-- creatureEffectScriptEffect (boss_ai.go:7642), so no double-fire.
local function stinkyPreciousDecimate(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value <= 0 then
        return
    end
    if hitTarget:GetHealthPct() > value then
        hitTarget:SetHealth(math.floor(hitTarget:GetMaxHealth() * value / 100))
    end
end
RegisterSpellEvent(SPELL_DECIMATE, 5, stinkyPreciousDecimate)
