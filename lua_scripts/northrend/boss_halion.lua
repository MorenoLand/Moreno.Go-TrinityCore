-- Halion (Ruby Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/boss_halion.cpp
-- (10 CreatureScripts: boss_halion (BossAI, DATA_HALION) +
-- boss_twilight_halion (BossAI, DATA_TWILIGHT_HALION) +
-- npc_halion_controller (ScriptedAI, NPC_HALION_CONTROLLER = 40146) +
-- npc_meteor_strike_flame / npc_meteor_strike_initial / npc_meteor_strike
-- / npc_combustion_consumption / npc_orb_carrier / npc_living_inferno /
-- npc_living_ember (ScriptedAI, zero Talk); 1 GameObjectScript
-- (go_twilight_portal); 15 SpellScriptLoaders
-- (spell_halion_meteor_strike_marker,
-- spell_halion_combustion_consumption x2, spell_halion_marks x2,
-- spell_halion_combustion_consumption_periodic,
-- spell_halion_damage_aoe_summon, spell_halion_twilight_realm_handlers
-- x2, spell_halion_summon_exit_portals, spell_halion_twilight_phasing,
-- spell_halion_twilight_cutter, spell_halion_clear_debuffs,
-- spell_halion_spawn_living_embers, spell_halion_blazing_aura); all
-- registered from inside AddSC_boss_halion() (line 1906); loader decl
-- 193 / call 388 per northrend_script_loader.cpp — the SIXTH and FINAL
-- group of the "// Ruby Sanctum" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_general_zarithrian() (call 387) —
-- verified from the loader this run; the checkpoint sequence
-- (zarithrian -> halion) is followed).
-- Entries: 39863 Halion (ruby_sanctum.h NPC_HALION, line 80) and 40142
-- Twilight Halion (ruby_sanctum.h NPC_TWILIGHT_HALION, line 81);
-- instance_ruby_sanctum.cpp creatureData binds NPC_HALION ->
-- DATA_HALION (line 54), NPC_TWILIGHT_HALION -> DATA_TWILIGHT_HALION
-- (line 55), NPC_HALION_CONTROLLER -> DATA_HALION_CONTROLLER (line 56)
-- — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_halion" hits boss_halion.cpp only (+ the loader
-- decl/call lines); this clone carries no sql/ tree, so ScriptName
-- bindings are DB-side by construction. Pre-existing lua coverage
-- AUDITED this run — COMPLETE (see accounting below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- boss_halion JustEngagedWith — Talk(SAY_AGGRO 2) + the three
-- phase-ungated timer arms (event 1 — the auriaya precedent):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- randtime(6s,10s) init / randtime(8s,10s) re-arm ->
-- creature:CastSpell(nil, SPELL_CLEAVE); EVENT_TAIL_LASH —
-- DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered, randtime(7s,12s)
-- init / randtime(11s,16s) re-arm -> creature:CastSpell(creature,
-- SPELL_TAIL_LASH) (the DoCastAOE self-cast lowering); EVENT_BREATH —
-- DoCastSelf(SPELL_FLAME_BREATH 74525) non-triggered, randtime(5s,15s)
-- init / randtime(16s,25s) re-arm -> creature:CastSpell(creature,
-- SPELL_FLAME_BREATH) (boss_halion.cpp:251-253/:335-344); the 75%
-- DamageTaken PHASE_TWO transition stops all three via
-- halionDamageTaken (event 9 — the bronjahm latch precedent); the
-- events.Reset / events.SetPhase(PHASE_ONE) / BossAI::JustEngagedWith /
-- DoCastSelf SPELL_TWILIGHT_PRECISION / EVENT_ACTIVATE_FIREWALL /
-- EVENT_FIERY_COMBUSTION / EVENT_METEOR_STRIKE ScheduleEvent /
-- SendEncounterUnit / controller->AI()->SetData(DATA_FIGHT_PHASE,
-- PHASE_ONE) legs have no bridges).
-- boss_halion JustDied — Talk(SAY_DEATH 5)
-- (event 4 — the sjonnir precedent; the _JustDied() instance leg and
-- the cross-creature twilightHalion->KillSelf() /
-- controller->KillSelf() legs have no bridges).
-- boss_twilight_halion KilledUnit — Talk(SAY_KILL 6) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent) + the UNGATED triggered victim-cast
-- SPELL_LEAVE_TWILIGHT_REALM 74812 ("victims should not be in the
-- twilight realm" — the arlokk CastSpell(victim, id, true) idiom).
-- Material halion has no KilledUnit arm — SAY_KILL lives on the
-- twilight AI only.
-- boss_twilight_halion timer arms — PORTED via the CreateLuaEvent
-- harness (GUID-keyed timers table, cancel on events 2/4/23 — the
-- krystallus / bjarngrim-lieutenant convention), armed from
-- twilightHalionEnterCombat (entry 40142 registered events 1/2/3/4/23):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- 3s init / randtime(7s,10s) re-arm (boss_halion.cpp:525-528) ->
-- creature:CastSpell(nil, SPELL_CLEAVE) — the DoCastVictim lowering.
-- EVENT_BREATH — DoCastSelf(SPELL_DARK_BREATH 74806) non-triggered,
-- 12s init / randtime(10s,14s) re-arm (:533-536) ->
-- creature:CastSpell(creature, SPELL_DARK_BREATH).
-- EVENT_TAIL_LASH — DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered,
-- 12s init (armed in the C++ constructor, :420) / randtime(12s,16s)
-- re-arm (:529-532) -> creature:CastSpell(creature, SPELL_TAIL_LASH)
-- — the DoCastAOE self-cast lowering (boss_ai.go convention).
-- DOCUMENTED-ONLY (in this header; entries 39863 and 40142
-- registered):
-- boss_halion DamageTaken — PORTED (event 9, pre-damage): the 75%
-- PHASE_TWO transition — Talk(SAY_PHASE_TWO 4) + stop of the three
-- phase-one timer arms on post-damage health < 75% max
-- (boss_halion.cpp:286-296; HealthBelowPctDamaged strict-<, signed,
-- unclamped — the zum_rah pre-damage latch precedent). PORTED this run:
-- the DoCastSelf(SPELL_TWILIGHT_PHASING 74808) leg at :293 is now cast
-- as creature:CastSpell(creature, SPELL_TWILIGHT_PHASING) at the latch
-- (non-triggered DoCastSelf lowering — the header's old "phasing-cast
-- rides absent bridges" claim was stale; CastSpell is bridged). The
-- never-die damage cap (damage = health-1 while not PHASE_THREE) rides
-- the unbridged phase machine and stays out; the UNIT_FLAG_NOT_
-- SELECTABLE set leg is bridgeable (SetFlag live) but UNPAIRED —
-- its clear rides the controller/instance phase-three action, so a
-- set-alone port would leave the boss untargetable through phase 3;
-- the CastStop / controller SetData(PHASE_TWO) legs ride absent
-- bridges (no CastStop / cross-creature-SetData methods — verified
-- in engine/world/lua_objects.go this run).
-- boss_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING 75509 — stays out: the trigger is the
-- unported npc_halion_controller's EVENT_TWILIGHT_MENDING cast
-- (boss_halion.cpp:732-735), so event 14 can never fire with
-- this ID (the old "SpellHit-15-never-fires" label was stale —
-- event 14 fires from the Go spell path).
-- boss_halion UpdateAI — Talk(SAY_METEOR_STRIKE 3) rides
-- EVENT_METEOR_STRIKE, phase-machine-gated (PHASE_ONE->TWO transition
-- via 75%-health DamageTaken + controller SetData, boss_halion.cpp:
-- 286-296); the EVENT_ACTIVATE_FIREWALL / EVENT_FIERY_COMBUSTION
-- DoCastSelf / CastSpell legs ride the same phase machine (SetPhase
-- :246/:288/:426/:477 + cross-creature controller SetData
-- :260/:296/:667 — no phase-machine bridge). EVENT_BREATH /
-- EVENT_CLEAVE / EVENT_TAIL_LASH are PORTED above (phase-ungated
-- engage arms, stopped by the 75% latch).
-- boss_twilight_halion JustEngagedWith — zero Talk; the four
-- phase-ungated timer arms are PORTED (see twilightHalionEnterCombat
-- below — EVENT_SOUL_CONSUMPTION's ctor schedule re-anchored on
-- engage). C++ UpdateAI has no phase early-return (boss_halion.cpp:
-- 511-544), so the timers run identically in PHASE_TWO and
-- PHASE_THREE — engage-armed CreateLuaEvent timers are faithful
-- across both. The SendEncounterUnit / DoCastSelf
-- SPELL_TWILIGHT_PRECISION legs ride absent bridges.
-- EVENT_SOUL_CONSUMPTION 74792 — PORTED (soulConsumptionTick, below):
-- SelectTarget(Random,1,0.0f,true,true,SPELL_TWILIGHT_REALM 74807) ->
-- CastSpell(target, SOUL_CONSUMPTION 74792, TRIGGERED_IGNORE_SET_FACING),
-- 15s init / 20s re-arm (:421/:537-540) — the stale "no random-target /
-- target-filter bridges" claim is retracted: the random-target leg rides
-- the threat-list skip-first pool (eck precedent, 2026-10-25), the aura
-- filter rides plr:HasAura (bridged,
-- engine/world/lua_player.go:203 — hakkar precedent), the triggered flag
-- rides the arlokk CastSpell(target, id, true) idiom.
-- Constructor-side legs (boss_twilight_halionAI ctor, :405-421): DUSK_
-- SHROUD 75476's triggered self-cast is bridgeable
-- (creature:CastSpell(creature, id, true)) but the ctor fires at
-- creature creation and this runtime's CREATURE_EVENT_ON_SPAWN (5)
-- fires only on the respawn path — never on first summon or grid
-- activation (engine/world/lua_creature_events.go fireCreatureSpawned
-- comment) — so there is no faithful construction-time hook; stays
-- out. COPY_DAMAGE auras stay out: the C++ uses explicit cross-creature
-- AddAura (halion->AddAura(SPELL_COPY_DAMAGE, me) to dodge
-- SPELL_ATTR6_CANT_TARGET_SELF) and the other body is unreachable from
-- Lua (no cross-creature bridge; the Lua AddAura bridge is self-only).
-- Health sync / phase-mask / react-state / combat-flag legs: only the
-- phase-mask leg rides an absent bridge (no creature SetPhaseMask method).
-- SetReactState and SetFlag ARE bridged (lua_creature_events.go:524;
-- e.g. SetFlag(59,0x300) in the incarcerator port) — the react-state /
-- IN_COMBAT-flag legs stay out for want of a construction-time hook, not
-- a bridge: CREATURE_EVENT_ON_SPAWN (5) fires on first grid activation
-- since 2026-10-19 but temp summons bypass that funnel
-- (creaturemotion.go), and the twilight phase machine is unmodeled
-- (75063's summon leg never fires), so entry 40142 never spawns;
-- TAIL_LASH's 12s constructor-arm is documented as a spawn->engage drift
-- on the first tick only (twilight halion spawns in combat, so the delta
-- is near-zero).
-- boss_twilight_halion DamageTaken — PORTED (event 9, pre-damage):
-- the 50% transition (boss_halion.cpp:469-482) — the stale
-- "(no-DamageTaken bridge)" claim is retracted (event 9 is bridged and
-- already used by the material 75% latch in this file): latch-gated
-- DoCastSelf(SPELL_TWILIGHT_DIVISION 75063) (non-triggered) +
-- Talk(SAY_PHASE_THREE 2), C++ arm order (cast then Talk); the
-- never-die cap (damage = health - 1 while not PHASE_THREE) is ported
-- with C++ evaluation order (cap applied before the 50% check, so the
-- crossing blow is capped; the cap drops in phase three so the boss is
-- killable — the shared fightPhaseThree latch, gurtogg onChangePhase
-- idiom); CastStop rides the absent CastStop bridge (benign); the
-- PHASE_THREE controller->AI()->SetData(DATA_TWILIGHT_DAMAGE_TAKEN)
-- routing leg rides the no-cross-AI bridge.
-- boss_halion EnterEvadeMode (:236) — forwards the evade to the
-- controller creature when why == EVADE_REASON_BOUNDARY or the phase
-- machine is in PHASE_ONE — stays out (EnterEvadeMode has no Lua
-- bridge, boss_ai.go:5753-5754; the cross-creature forward rides the
-- no-cross-AI bridge).
-- boss_twilight_halion SpellHit — Talk(SAY_REGENERATE 0) rides
-- SPELL_TWILIGHT_MENDING 75509 — stays out on the same unported-
-- controller trigger (boss_halion.cpp:732-735); the SPELL_TWILIGHT_
-- DIVISION -> controller DoAction(ACTION_MONITOR_CORPOREALITY)
-- leg rides the no-DoAction bridge.
-- boss_twilight_halion JustDied — zero Talk; the
-- DoCastSelf(SPELL_CLEAR_DEBUFFS 75396) arm at boss_halion.cpp:599-605
-- is npc_halion_controllerAI::JustDied (entry 40146, unregistered),
-- NOT twilight halion's — stays out (the halion->LowerPlayerDamageReq /
-- Unit::Kill / controller->KillSelf legs ride absent bridges).
-- boss_twilight_halion Reset — EMPTY in C++ (boss_halion.cpp:436);
-- the DoCastSelf(SPELL_CLEAR_DEBUFFS 75396) arm at :586-592 is
-- npc_halion_controllerAI::Reset (entry 40146, unregistered) — stays
-- out (the _summons / _events / Initialize legs ride absent bridges).
-- npc_halion_controller (40146) — its intro machine issues cross-AI
-- Talk(SAY_INTRO 1) on the summoned halion, plus the corporeality
-- machine's Talk(EMOTE_CORPOREALITY_TOT 4 / TIT 3 on twilight halion,
-- POT 8 / PIP 9 on material halion) and the orb-shoot machine's
-- cross-AI Talk(EMOTE_WARN_LASER 0) on the orb-carrier passenger and
-- Talk(SAY_SPHERE_PULSE 1) on twilight halion (no cross-AI-Talk
-- bridge); the DoCastSelf / scheduler / DoAction legs ride absent
-- bridges. Zero Talk on its own unit — NOT registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- The other 8 CreatureScripts / go_twilight_portal / the 15
-- SpellScriptLoaders — zero Talk (npc_orb_carrier /
-- npc_meteor_strike* / npc_combustion_consumption) or spell-model arms
-- (no-SpellScript / no-AuraScript bridge, the sindragosa
-- ice_tomb_target precedent) — NOT registered. npc_living_inferno
-- (40681) / npc_living_ember (40683): only their JustDied
-- DespawnOrUnsummon(1ms) legs port (event 4, below — 1ms ≈ immediate
-- creature:Despawn); the rest stays out (SetFlag IS bridged but no
-- CastStop / cross-creature-SetData bridges).
-- SAY_BERSERK 7 — dead text, no Talk() call in the file.
-- All 13 Talk() calls in the file accounted for (SAY_AGGRO 2 +
-- SAY_DEATH 5 + SAY_KILL 6 ported; SAY_PHASE_TWO 4 + SAY_REGENERATE 0
-- x2 + SAY_METEOR_STRIKE 3 + SAY_PHASE_THREE 2 + SAY_INTRO 1 +
-- EMOTE_CORPOREALITY_TOT/TIT/POT/PIP + EMOTE_WARN_LASER 0 +
-- SAY_SPHERE_PULSE 1 documented above).
-- Missing-cast audit 2026-10-16 (all 11): PORTED — 74808
-- SPELL_TWILIGHT_PHASING (:293, halionDamageTaken 75% latch); 75063
-- SPELL_TWILIGHT_DIVISION (:479, twilightHalionDamageTaken 50% latch —
-- the 2026-10-16 "twilight phase machine unmodeled" claim is retracted:
-- the transition is ported with the 50% latch + Talk). BLOCKED —
-- 75476 DUSK_SHROUD (:414, twilight ctor, no construction hook);
-- 74805 SUMMON_EXIT_PORTALS (:677), 76006 COSMETIC_FIRE_
-- PILLAR (:710), 76010 FIERY_EXPLOSION (:726) — npc_halion_controller
-- (40146, unregistered) DoAction/event arms, triggers instance-side;
-- 74648 AOE_DAMAGE (:1006), 74713 FIRE_AURA_1 (:1007), 74641 COUNTDOWN
-- (:1024), 40031 BIRTH_NO_VISUAL (:1025), 74718 FIRE_AURA_2 (:1079) —
-- npc_meteor_strike_initial / npc_meteor_strike (unregistered), all
-- hanging off the phase-machine-gated boss EVENT_METEOR_STRIKE arm.

local ENTRY_HALION = 39863
local ENTRY_TWILIGHT_HALION = 40142
local ENTRY_LIVING_INFERNO = 40681 -- ruby_sanctum.h:83
local ENTRY_LIVING_EMBER = 40683 -- ruby_sanctum.h:84

local SAY_INTRO    = 1 -- (documented-only, rides the controller's cross-AI intro)
local SAY_AGGRO    = 2
local SAY_PHASE_TWO = 4
local SAY_PHASE_THREE = 2
local SAY_DEATH    = 5
local SAY_KILL     = 6 -- twilight halion only

local SPELL_CLEAVE      = 74524
local SPELL_DARK_BREATH = 74806
local SPELL_FLAME_BREATH = 74525
local SPELL_TAIL_LASH   = 74531
local SPELL_LEAVE_TWILIGHT_REALM = 74812
local SPELL_TWILIGHT_PHASING = 74808 -- C++ enum :111 ("Phase spell from phase 1 to phase 2")
local SPELL_SOUL_CONSUMPTION = 74792 -- C++ enum :81
local SPELL_TWILIGHT_REALM = 74807 -- C++ enum :115 (SelectTarget aura filter)
local SPELL_TWILIGHT_DIVISION = 75063 -- C++ enum :103 ("Phase spell from phase 2 to phase 3")

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the krystallus / bjarngrim-lieutenant convention.
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

-- Material-halion phase-one timer arms (C++ UpdateAI :335-344):
-- EVENT_CLEAVE — DoCastVictim(SPELL_CLEAVE 74524) non-triggered,
-- randtime(6s,10s) init / randtime(8s,10s) re-arm.
local function matCleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "mat_cleave", {8000, 10000}, function()
        matCleaveTick(creature, guid)
    end)
end

-- EVENT_TAIL_LASH — DoCastAOE(SPELL_TAIL_LASH 74531) non-triggered,
-- randtime(7s,12s) init / randtime(11s,16s) re-arm — the DoCastAOE
-- self-cast lowering (boss_ai.go convention).
local function matTailLashTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    schedule(guid, "mat_tail_lash", {11000, 16000}, function()
        matTailLashTick(creature, guid)
    end)
end

-- EVENT_BREATH — DoCastSelf(SPELL_FLAME_BREATH 74525) non-triggered,
-- randtime(5s,15s) init / randtime(16s,25s) re-arm (distinct from the
-- twilight DARK_BREATH 74806 below).
local function matBreathTick(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_BREATH)
    schedule(guid, "mat_breath", {16000, 25000}, function()
        matBreathTick(creature, guid)
    end)
end

-- 75% phase-two latch (C++ boss_halion::DamageTaken :286-296 —
-- pre-damage event 9; the zum_rah latch precedent).
local phaseTwo = {}

-- Encounter-wide phase-three latch (C++ boss_halion.cpp:283/:469 —
-- the never-die damage cap drops in PHASE_THREE, i.e. the boss
-- becomes killable). The controller SetData(DATA_FIGHT_PHASE) phase
-- machine has no bridge, so the latch is set by the ported twilight
-- 50% transition (twilightHalionDamageTaken below — the gurtogg
-- onChangePhase idiom) and read by both never-die caps; cleared on
-- every combat-end / reset / died path of both halves.
local fightPhaseThree = false

-- C++ boss_halion::JustEngagedWith :242-262:
-- Talk(SAY_AGGRO); events.Reset(); events.SetPhase(PHASE_ONE);
-- BossAI::JustEngagedWith(who); me->AddAura(SPELL_TWILIGHT_PRECISION, me);
-- events.ScheduleEvent(EVENT_ACTIVATE_FIREWALL, 5s);
-- events.ScheduleEvent(EVENT_BREATH, randtime(5s,15s));
-- events.ScheduleEvent(EVENT_CLEAVE, randtime(6s,10s));
-- events.ScheduleEvent(EVENT_TAIL_LASH, randtime(7s,12s));
-- events.ScheduleEvent(EVENT_FIERY_COMBUSTION, randtime(15s,18s));
-- events.ScheduleEvent(EVENT_METEOR_STRIKE, 18s) + SendEncounterUnit +
-- controller->AI()->SetData(DATA_FIGHT_PHASE, PHASE_ONE). The three
-- phase-ungated timer arms port below; the firewall / fiery_combustion /
-- meteor_strike / precision-aura / encounter-frame / controller legs
-- have no bridges (documented above).
local function halionEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    fightPhaseThree = false
    schedule(guid, "mat_cleave", {6000, 10000}, function()
        matCleaveTick(creature, guid)
    end)
    schedule(guid, "mat_tail_lash", {7000, 12000}, function()
        matTailLashTick(creature, guid)
    end)
    schedule(guid, "mat_breath", {5000, 15000}, function()
        matBreathTick(creature, guid)
    end)
end

local function halionCombatEnd(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    fightPhaseThree = false
end

-- C++ boss_halion::DamageTaken :283-296:
-- Lethal-damage cap (damage = health - 1) applies while NOT in
-- PHASE_THREE (C++ evaluation order: the cap is applied BEFORE the
-- 75% check, so the latch above fires on the killing blow exactly as
-- in C++ — DamageTaken-class, fires on lethal by design); in
-- PHASE_THREE the cap drops and the boss is killable. The phase gate
-- rides the shared fightPhaseThree latch (set by the twilight 50%
-- transition below; the controller SetData(DATA_FIGHT_PHASE) phase
-- machine has no bridge — the gurtogg onChangePhase idiom).
-- Talk(SAY_PHASE_TWO) + the stop of the phase-one timer arms + the
-- DoCastSelf SPELL_TWILIGHT_PHASING 74808 cast port (non-triggered ->
-- creature:CastSpell(creature, SPELL_TWILIGHT_PHASING) — the DoCastSelf
-- lowering, same idiom as the BREATH self-casts); the flag set leg is
-- bridgeable (SetFlag live) but UNPAIRED — its clear rides the
-- controller/instance phase-three action; the CastStop / controller
-- SetData legs ride absent bridges (no CastStop / cross-creature
-- SetData methods — verified in engine/world/lua_objects.go this run).
local function halionDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- C++ :283 reads IsInPhase(PHASE_THREE) before the transition
    -- checks, so the phase-crossing blow is still capped.
    local wasPhaseThree = fightPhaseThree
    local health = creature:GetHealth()
    local capped = damage
    if damage >= health and not wasPhaseThree then
        capped = health - 1
    end
    if not phaseTwo[guid] then
        local postHealth = 0
        if health > capped then
            postHealth = health - capped
        end
        -- HealthBelowPctDamaged strict-< floor form (Unit.h:873):
        -- postDamage < floor(maxHealth * 75 / 100). The old
        -- cross-multiplied form fired 1 HP early when maxHealth * 75
        -- mod 100 != 0 (baltharus/anubarak 2026-10-25 precedent).
        if postHealth < math.floor(creature:GetMaxHealth() * 75 / 100) then
            phaseTwo[guid] = true
            creature:Talk(SAY_PHASE_TWO)
            creature:CastSpell(creature, SPELL_TWILIGHT_PHASING)
            cancelTimers(guid)
        end
    end
    -- C++ never-die cap (boss_halion.cpp:283): lethal damage while not
    -- in phase three is clamped to health - 1 (phases 1 AND 2); in
    -- phase three the cap drops and the boss is killable.
    if capped ~= damage then
        return false, capped
    end
end

-- C++ boss_halion::JustDied :267:
-- _JustDied(); Talk(SAY_DEATH) — the sjonnir precedent (the
-- _JustDied() instance leg and the cross-creature KillSelf legs have
-- no bridges).
local function halionDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    fightPhaseThree = false
    creature:Talk(SAY_DEATH)
end

-- C++ boss_twilight_halion::KilledUnit :439-445:
-- if (victim->GetTypeId() == TYPEID_PLAYER)
--     Talk(SAY_KILL) — the razuvious player-gated variant precedent;
-- me->CastSpell(victim, SPELL_LEAVE_TWILIGHT_REALM, true) — UNGATED
-- (fires for any victim type), triggered victim-cast — the arlokk idiom.
-- Material halion has no KilledUnit arm.
local function twilightHalionKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
    creature:CastSpell(victim, SPELL_LEAVE_TWILIGHT_REALM, true)
end

RegisterCreatureEvent(ENTRY_HALION, 1, halionEnterCombat)
RegisterCreatureEvent(ENTRY_HALION, 2, halionCombatEnd)
RegisterCreatureEvent(ENTRY_HALION, 4, halionDied)
RegisterCreatureEvent(ENTRY_HALION, 9, halionDamageTaken)
RegisterCreatureEvent(ENTRY_HALION, 23, halionCombatEnd)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 3, twilightHalionKilledUnit)

-- C++ UpdateAI EVENT_CLEAVE: DoCastVictim(SPELL_CLEAVE 74524)
-- non-triggered, 3s init / randtime(7s,10s) re-arm
-- (boss_halion.cpp:525-528) — no phase gating on the arm.
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {7000, 10000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_BREATH: DoCastSelf(SPELL_DARK_BREATH 74806)
-- non-triggered, 12s init / randtime(10s,14s) re-arm
-- (boss_halion.cpp:533-536) — no phase gating on the arm.
local function breathTick(creature, guid)
    creature:CastSpell(creature, SPELL_DARK_BREATH)
    schedule(guid, "breath", {10000, 14000}, function()
        breathTick(creature, guid)
    end)
end

-- C++ UpdateAI EVENT_TAIL_LASH: DoCastAOE(SPELL_TAIL_LASH 74531)
-- non-triggered, 12s init armed in the constructor (:420) /
-- randtime(12s,16s) re-arm (:529-532) — no phase gating on the arm.
-- The constructor arm is re-anchored on OnEnterCombat: twilight
-- halion spawns in combat (UNIT_FLAG_IN_COMBAT at :414), so the
-- spawn->engage delta on the first tick is near-zero — documented.
local function tailLashTick(creature, guid)
    creature:CastSpell(creature, SPELL_TAIL_LASH)
    schedule(guid, "tail_lash", {12000, 16000}, function()
        tailLashTick(creature, guid)
    end)
end

-- C++ boss_twilight_halion::JustEngagedWith:
-- events.SetPhase(PHASE_TWO); BossAI::JustEngagedWith(who);
-- me->AddAura(SPELL_TWILIGHT_PRECISION, me);
-- events.ScheduleEvent(EVENT_CLEAVE, 3s);
-- events.ScheduleEvent(EVENT_BREATH, 12s);
-- instance->SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE, me, 2)
-- — the four timer arms bridge (the TAIL_LASH arm moves here from
-- its constructor schedule; SOUL_CONSUMPTION's constructor schedule
-- is re-anchored the same way — tailLashTick precedent).
-- C++ boss_twilight_halionAI UpdateAI EVENT_SOUL_CONSUMPTION
-- (:537-540): SelectTarget(Random,1,0.0f,true,true,
-- SPELL_TWILIGHT_REALM 74807) -> CastSpell(target, SPELL_SOUL_CONSUMPTION
-- 74792, TRIGGERED_IGNORE_SET_FACING), 20s re-arm — re-armed
-- unconditionally (outside the target-if — sjonnir precedent). The
-- position-1 offset pops the MaxThreat-ordered first entry before
-- the player/aura filters (UnitAI.h offset-before-filter, eck
-- precedent); the aura filter rides plr:HasAura (player HasAura
-- bridged — engine/world/lua_player.go:203, hakkar precedent); the
-- triggered flag rides the arlokk CastSpell(target, id, true) idiom.
-- C++ SelectTarget(Random,1,0.0f,true,true,SPELL_TWILIGHT_REALM
-- 74807): threat list, position-1 offset pops the current victim
-- BEFORE the player/aura filters (C++ builds the list victim-first,
-- UnitAI.h
-- offset-before-filter; eck precedent), then Player + alive +
-- HasAura(74807). The old GetPlayersInWorld instance pool + victim
-- exclusion could pick players never engaged with the boss
-- (2026-10-25 fix, same class as the obsidian/baltharus pools).
local function randomTwilightPlayer(creature)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and t:GetObjectType() == "Player" and t:IsAlive()
                and t:HasAura(SPELL_TWILIGHT_REALM) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function soulConsumptionTick(creature, guid)
    local target = randomTwilightPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SOUL_CONSUMPTION, true)
    end
    schedule(guid, "soul_consumption", 20000, function()
        soulConsumptionTick(creature, guid)
    end)
end

local function twilightHalionEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    fightPhaseThree = false
    schedule(guid, "cleave", 3000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "breath", 12000, function()
        breathTick(creature, guid)
    end)
    schedule(guid, "tail_lash", 12000, function()
        tailLashTick(creature, guid)
    end)
    -- C++ ctor schedules EVENT_SOUL_CONSUMPTION at 15s (:421);
    -- re-anchored on OnEnterCombat — twilight halion spawns in combat
    -- (UNIT_FLAG_IN_COMBAT at :414), so the spawn->engage delta on the
    -- first tick is near-zero (tailLashTick precedent).
    schedule(guid, "soul_consumption", 15000, function()
        soulConsumptionTick(creature, guid)
    end)
end

local twilightPhaseThree = {}

local function twilightHalionClearPhase(guid)
    twilightPhaseThree[guid] = nil
end

local function twilightHalionLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    twilightHalionClearPhase(guid)
    fightPhaseThree = false
end

-- C++ boss_twilight_halion::Reset is EMPTY (boss_halion.cpp:436) —
-- the DoCastSelf(SPELL_CLEAR_DEBUFFS) arm at :586-592 is
-- npc_halion_controllerAI::Reset (entry 40146, unregistered). Event 23
-- only cancels timers.
local function twilightHalionReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    twilightHalionClearPhase(guid)
    fightPhaseThree = false
end

-- C++ boss_twilight_halion::JustDied: _events.Reset() +
-- _summons.DespawnAll() (boss_halion.cpp:448-466) — the
-- DoCastSelf(SPELL_CLEAR_DEBUFFS) arm at :599-605 is
-- npc_halion_controllerAI::JustDied (entry 40146, unregistered).
local function twilightHalionDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    twilightHalionClearPhase(guid)
    fightPhaseThree = false
end

-- C++ boss_twilight_halion::DamageTaken :469-482 (pre-damage event 9 —
-- the zum_rah latch precedent): lethal-damage cap (damage = health -
-- 1) while NOT in PHASE_THREE (C++ evaluation order: the cap is
-- applied BEFORE the 50% check, so the crossing blow is capped and the
-- boss survives it at 1 hp); HealthBelowPctDamaged(50, damage) &&
-- IsInPhase(PHASE_TWO) -> SetPhase(PHASE_THREE); me->CastStop();
-- DoCastSelf(SPELL_TWILIGHT_DIVISION 75063) (non-triggered);
-- Talk(SAY_PHASE_THREE 2). The PHASE_TWO gate is entered in
-- JustEngagedWith (twilightHalionEnterCombat below), so the latch below
-- is the faithful substitute; the transition also sets the shared
-- fightPhaseThree latch that drops both halves' never-die caps (the
-- gurtogg onChangePhase idiom). The CastStop leg rides the absent
-- CastStop bridge (benign: Go casts are packet-only); the PHASE_THREE
-- damage-routing leg (controller->AI()->SetData
-- (DATA_TWILIGHT_DAMAGE_TAKEN)) rides the no-cross-AI bridge.
local function twilightHalionDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- C++ :469 reads IsInPhase(PHASE_THREE) before the 50% check, so
    -- the phase-crossing blow is still capped.
    local wasPhaseThree = twilightPhaseThree[guid]
    local health = creature:GetHealth()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 or health == 0 then
        return
    end
    local capped = damage
    if damage >= health and not wasPhaseThree then
        capped = health - 1
    end
    -- HealthBelowPctDamaged strict-<, signed, unclamped
    -- (Unit.h:873; the zum_rah precedent) — evaluated on the capped
    -- damage, C++ :469-:475 order.
    -- HealthBelowPctDamaged strict-< floor form (Unit.h:873):
    -- postDamage < floor(maxHealth * 50 / 100), signed, unclamped
    -- (the zum_rah precedent) — evaluated on the capped damage,
    -- C++ :469-:475 order. The old cross-multiplied form fired 1 HP
    -- early when maxHealth * 50 mod 100 != 0 (baltharus precedent).
    if not wasPhaseThree and (health - capped) < math.floor(maxHealth * 50 / 100) then
        twilightPhaseThree[guid] = true
        fightPhaseThree = true
        creature:CastSpell(creature, SPELL_TWILIGHT_DIVISION)
        creature:Talk(SAY_PHASE_THREE)
    end
    -- C++ never-die cap (boss_halion.cpp:469): lethal damage while not
    -- in phase three is clamped to health - 1 (phases 1 AND 2); in
    -- phase three the cap drops and the boss is killable (the akama /
    -- illidan health-1 precedent).
    if capped ~= damage then
        return false, capped
    end
end

RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 1, twilightHalionEnterCombat)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 2, twilightHalionLeaveCombat)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 4, twilightHalionDied)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 9, twilightHalionDamageTaken)
RegisterCreatureEvent(ENTRY_TWILIGHT_HALION, 23, twilightHalionReset)

-- C++ npc_living_infernoAI::JustDied (boss_halion.cpp:1292-1296) and
-- npc_living_emberAI::JustDied (:1329-1333): me->DespawnOrUnsummon(1ms)
-- ports as immediate creature:Despawn() (1ms ≈ immediate, bridged).
local function livingAddJustDied(event, creature, killer)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_LIVING_INFERNO, 4, livingAddJustDied)
RegisterCreatureEvent(ENTRY_LIVING_EMBER, 4, livingAddJustDied)
