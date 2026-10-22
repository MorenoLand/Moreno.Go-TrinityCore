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
-- EVENT_BERSERK single-fire 10min all phases — Talk(SAY_BERSERK
-- 12) + DoCastSelf(SPELL_BERSERK2 47008) non-triggered
-- (:266/:598-600); the old "no-timer-bridge" claim for this arm
-- was stale doctrine (maiden CreateLuaEvent convention).
-- Talk(EMOTE_UNSTABLE_EXPERIMENT 5) rides EVENT_UNSTABLE_EXPERIMENT
-- (:612-615) — OUT: P1&&P2-gated, cancelled at the P2->P3
-- transition (:531) — phase machine (gothik precedent). Talk(
-- EMOTE_MALLEABLE_GOO 9) ×2 rides EVENT_MALLEABLE_GOO (:630-650)
-- — OUT: the random-target leg is bridged (GetPlayersInWorld +
-- math.random, nightbane/midnight precedent); the true blockers are
-- the P1->P2 tear-gas phase machine (boss_professor_putricide.cpp:524-525)
-- and the Is25ManRaid branch (:631; no Is25Man bridge).
-- Talk(EMOTE_CHOKING_GAS_BOMB 10) rides EVENT_CHOKING_GAS_BOMB
-- (:652-655) — OUT: P2/P3 phase-gated (:526). Talk(
-- SAY_FESTERGUT_DEATH 1) rides EVENT_FESTERGUT_DIES (:579-581)
-- and Talk(SAY_ROTFACE_DEATH 3) rides EVENT_ROTFACE_DIES
-- (:590-592) — OUT: Talk-only + EnterEvadeMode, DoAction-phase
-- driven. Talk(SAY_TRANSFORM_1 7) / Talk(SAY_TRANSFORM_2 8) ride
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
-- DamageTaken 80%/35% latches (:512-528) OUT: machine-free in form
-- but feed the full tear-gas/MovementInform phase machine.
-- Header corrected 2026-10-14: the old "no-timer-bridge" blanket
-- was disproved (CreateLuaEvent is a real registered bridge,
-- engine/scripting/runtime.go:798); each arm is now classified
-- individually against the C++.
-- boss_professor_putricide DoAction — Talk(
-- SAY_FESTERGUT_GASEOUS_BLIGHT 0) rides ACTION_FESTERGUT_GAS,
-- Talk(SAY_ROTFACE_OOZE_FLOOD 2) rides ACTION_ROTFACE_OOZE, and
-- Talk(SAY_PHASE_TRANSITION_HEROIC 6) rides ACTION_CHANGE_PHASE —
-- no-DoAction bridge (the krick ACTION_OUTRO precedent).
-- boss_professor_putricide JustEngagedWith instance legs —
-- instance->DoCastSpellOnPlayers(LIGHT_S_HAMMER_TELEPORT) /
-- DoZoneInCombat / instance->SetBossState(DATA_PROFESSOR_PUTRICIDE,
-- IN_PROGRESS) — no instance bridge; JustDied DoCastAOE(
-- SPELL_UNHOLY_INFUSION_CREDIT) leg: cast bridge exists
-- (creature:CastSpell self-cast triggered) but the arm is gated on
-- Is25ManRaid() — no 25-man bridge, stays out;
-- JustReachedHome instance->SetBossState(FAIL) — no instance
-- bridge.
-- npc_volatile_ooze / npc_gas_cloud (npc_putricide_oozeAI, lines
-- 718–810) — zero Talk lines — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent); the aura-application /
-- explode legs have no bridge.
-- The 20 spell scripts join the no-SpellScript-bridge queue (the
-- boss_moragg optic-link precedent) and the 5 aura scripts the
-- no-AuraScript-bridge queue.
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

local SAY_AGGRO = 4
local SAY_KILL = 11
local SAY_DEATH = 13
local SAY_BERSERK = 12

local SPELL_OOZE_TANK_PROTECTION = 71770
local SPELL_MUTATED_PLAGUE_CLEAR = 72618
local SPELL_BERSERK2 = 47008
local SPELL_SLIME_PUDDLE_TRIGGER = 70341

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
    for _, target in ipairs(randomPlayers(creature, 2)) do
        creature:CastSpell(target, SPELL_SLIME_PUDDLE_TRIGGER)
    end
    schedule(guid, "slimepuddle", 35000, function()
        slimePuddleTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: 10min single-fire, all phases — Talk(SAY_BERSERK) +
-- DoCast(me, SPELL_BERSERK2) non-triggered (:266/:598-600).
local function onBerserk(creature, guid)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK2)
end

-- C++ boss_professor_putricide::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent; the
-- CheckRequiredBosses evade branch has no bridge.
local function putricideJustEngagedWith(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    -- C++ :274 — DoCast(me, SPELL_OOZE_TANK_PROTECTION, true)
    -- (triggered self-cast).
    creature:CastSpell(creature, SPELL_OOZE_TANK_PROTECTION, true)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
    -- C++ :267 — events.ScheduleEvent(EVENT_SLIME_PUDDLE, 10s).
    schedule(guid, "slimepuddle", 10000, function() slimePuddleTick(creature, guid) end)
end

-- C++ Reset/_Reset() timer machinery — cancel/re-arm on combat
-- events (the maiden convention).
local function putricideLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset :236 instance->SetData(DATA_NAUSEA_ACHIEVEMENT, true) and ExtraEffect :1382 set it false:
-- achievement criteria, no achievement bridge (the lich-king no-achievement-criteria-bridge precedent).
-- DATA_EXPERIMENT_STAGE (:971-972) toggles inside the unbridged spell_putricide_unstable_experiment
-- SpellScript and is only read there — the whole chain joins the no-SpellScript-bridge queue. Dropped, documented.
local function putricideReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ boss_professor_putricide.cpp:242: me->SetReactState(REACT_DEFENSIVE)
    -- (Reset: flip follows events.Reset).
    creature:SetReactState(1) -- REACT_DEFENSIVE
    -- C++ :246-249: the RemoveFlag(NOT_SELECTABLE) + SetImmuneToPC(false)
    -- arms ride the Rotface+Festergut DONE instance boss-state gate, which
    -- has no bridge, so they stay out with it.
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
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    -- C++ :302 — DoCast(SPELL_MUTATED_PLAGUE_CLEAR), single-arg DoCast
    -- resolves via AISpellInfo, which defaults to AITARGET_SELF
    -- (CreatureAIImpl.h AISpellInfoType ctor), so self-cast.
    creature:CastSpell(creature, SPELL_MUTATED_PLAGUE_CLEAR)
end

RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 1, putricideJustEngagedWith)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 2, putricideLeaveCombat)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 3, putricideKilledUnit)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 4, putricideJustDied)
RegisterCreatureEvent(ENTRY_PROFESSOR_PUTRICIDE, 23, putricideReset)
