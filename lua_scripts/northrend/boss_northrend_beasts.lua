-- Northrend Beasts (Trial of the Crusader) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_northrend_beasts.cpp
-- (boss_gormok / npc_snobold_vassal / npc_beasts_combat_stalker /
-- boss_acidmaw / boss_dreadscale / npc_jormungars_slime_pool /
-- boss_icehowl / npc_fire_bomb (boss_northrend_beastsAI, boss_jormungarAI,
-- ScriptedAI); spell_gormok_jump_to_hand / spell_gormok_ride_player /
-- spell_gormok_snobolled / spell_jormungars_paralytic_toxin /
-- spell_jormungars_burning_bile / spell_jormungars_slime_pool /
-- spell_jormungars_paralysis / spell_icehowl_arctic_breath /
-- spell_icehowl_trample / spell_icehowl_massive_crash (AuraScript /
-- SpellScript via RegisterSpellScript) + spell_jormungars_snakes_spray
-- ("spell_jormungars_burning_spray" / "spell_jormungars_paralytic_spray");
-- AddSC_boss_northrend_beasts at end registers all via
-- RegisterTrialOfTheCrusaderCreatureAI (=
-- RegisterCreatureAIWithFactory(ai_name, GetTrialOfTheCrusaderAI),
-- trial_of_the_crusader.h line 288) / RegisterSpellScript). The SIXTH
-- Trial of the Crusader group in northrend_script_loader.cpp order
-- (decl 54 / call 251, immediately after AddSC_boss_twin_valkyr(),
-- under the "// Trial of the Crusader" marker; the call after it is
-- AddSC_instance_trial_of_the_crusader(), the last of the group).
-- Entry: 34796 Gormok the Impaler (trial_of_the_crusader.h NPC_GORMOK —
-- kalecgos pass; instance_trial_of_the_crusader.cpp binds NPC_GORMOK
-- -> DATA_GORMOK_THE_IMPALER; the RegisterTrialOfTheCrusaderCreatureAI
-- ScriptName binding is instance-shimmed, the creature_template
-- binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_gormok",
-- "npc_snobold_vassal", "npc_beasts_combat_stalker", "boss_acidmaw",
-- "boss_dreadscale", "npc_jormungars_slime_pool", "boss_icehowl",
-- "npc_fire_bomb", "spell_gormok_jump_to_hand",
-- "spell_gormok_ride_player", "spell_gormok_snobolled",
-- "spell_jormungars_paralytic_toxin", "spell_jormungars_burning_bile",
-- "spell_jormungars_slime_pool", "spell_jormungars_burning_spray",
-- "spell_jormungars_paralytic_spray", "spell_jormungars_paralysis",
-- "spell_icehowl_arctic_breath", "spell_icehowl_trample" and
-- "spell_icehowl_massive_crash" hits boss_northrend_beasts.cpp only
-- (loader carries only the AddSC_boss_northrend_beasts decl/call
-- lines); zero sql/ hits. No northrend_beasts lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_gormok: JustEngagedWith -> ScheduleTasks() (event 1; the
-- boss_northrend_beastsAI::JustEngagedWith instance-bookkeeping leg,
-- SetCombatPulseDelay/setActive and the heroic
-- HandleWithHeroicEvents (cross-AI combatStalker DoAction
-- ACTION_START_JORMUNGARS) have no bridge — tharon_ja precedent);
-- EVENT_IMPALE — DoCastVictim(Impale 66331), 10s init, 10s repeat
-- (moroes precedent; gormok's ScheduleTasks carries no phase args and
-- C++ UpdateAI runs the events whenever UpdateVictim() holds, so no
-- phase gate is modeled); EVENT_STAGGERING_STOMP —
-- DoCastVictim(Staggering Stomp 66330), 15s init, 22s repeat
-- (moroes precedent). Gormok has no Talk arms on
-- JustEngagedWith/KilledUnit/JustDied. All timers cancelled on 2/4/23
-- (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- boss_gormok: Reset: events.SetPhase(PHASE_EVENT) (no phase bridge —
-- anubarak_trial precedent) + HandleInitialMovement
-- MoveAlongSplineChain(POINT_INITIAL_MOVEMENT, SPLINE_INITIAL_MOVEMENT)
-- (no motion bridge); MovementInform SPLINE_CHAIN ->
-- ScheduleEvent(EVENT_ENGAGE, 7s) (no MovementInform hook); EVENT_ENGAGE
-- — DoUseDoorOrButton(DATA_MAIN_GATE) + SetImmuneToPC(false) +
-- SetReactState(REACT_AGGRESSIVE) + SummonCreature(
-- NPC_BEASTS_COMBAT_STALKER) + DoZoneInCombat(x2) +
-- DoCastSelf(SPELL_TANKING_GORMOK 66415, triggered — SERVERSIDE spell,
-- "No idea what it does" per the C++ comment) (no door / immune /
-- react / summon-STRAND / zone bridges — the self-cast is
-- port-pattern-ready in isolation (phase_hunter precedent) but rides
-- the unbridged intro machine — jedoga over-cast bar); EVENT_THROW —
-- vehicle-passenger walk (GetVehicleKit()->GetPassenger(i) ->
-- ExitVehicle + RemoveFlag(NON_ATTACKABLE | NOT_SELECTABLE) +
-- cross-AI snobold DoAction(ACTION_DISABLE_FIRE_BOMB) + CastSpell(me,
-- SPELL_JUMP_TO_HAND 66342)) (no vehicle / cross-AI DoAction bridges);
-- PassengerBoarded seat GORMOK_HAND_SEAT (4) -> CastSpell(
-- SPELL_RISING_ANGER 66636) (no vehicle bridge); JustDied —
-- instance->SetData(TYPE_NORTHREND_BEASTS, GORMOK_DONE) +
-- cross-AI combatStalker DoAction(ACTION_GORMOK_DEAD) (no instance /
-- cross-AI bridges; no Talk); EnterEvadeMode — instance->SetData(
-- DATA_DESPAWN_SNOBOLDS) + TYPE_NORTHREND_BEASTS FAIL + combatStalker
-- DespawnOrUnsummon + summons.DespawnAll + me->DespawnOrUnsummon
-- (instance model + no despawn bridge — terestian precedent).
-- npc_snobold_vassal (34800 — header constant; instance OnCreatureCreate
-- tracks NPC_SNOBOLD_VASSAL GUIDs — entry-evidence exists but the
-- ScriptName binding is DB-side): the whole vehicle mount machine —
-- MountOnBoss EnterVehicle(gormok, seat) (no vehicle bridge);
-- SetGUID(DATA_NEW_TARGET) -> AttackStart + EVENT_BATTER /
-- EVENT_SNOBOLLED / EVENT_HEAD_CRACK (no SetGUID hook); DoAction
-- ACTION_ENABLE_FIRE_BOMB / ACTION_DISABLE_FIRE_BOMB /
-- ACTION_ACTIVE_SNOBOLD — cross-AI DoAction-gated (no bridge);
-- EVENT_FIRE_BOMB — SelectTarget(Random,0,0.0f,true) -> CastSpell(
-- SPELL_FIRE_BOMB 66313), 12s/25s init, 20s/30s repeat (no
-- random-target bridge — cairne/kazzak precedent); EVENT_BATTER /
-- EVENT_HEAD_CRACK — DoCast(vehicleBase, 66408 / 66407) else
-- DoCastVictim (DoCastVictim legs port-pattern-ready in isolation but
-- fire only via the vehicle/SetGUID/SetCombatMovement machine —
-- jedoga over-cast bar); EVENT_SNOBOLLED — DoCastSelf(
-- SPELL_SNOBOLLED 66406) (vehicle-gated); EVENT_CHECK_MOUNT —
-- MountOnBoss() 3s loop; JustDied — instance->SetData(
-- DATA_SNOBOLD_COUNT, DECREASE) (instance model); CanAIAttack /
-- AttackStart / AttackStartCaster movement legs (no motion bridges).
-- npc_beasts_combat_stalker (36549 — kalecgos pass; instance binds
-- NPC_BEASTS_COMBAT_STALKER -> DATA_BEASTS_COMBAT_STALKER): Reset —
-- SetReactState(REACT_PASSIVE) (no react bridge) + EVENT_BERSERK
-- IsHeroic() ? 9min : 15min (no difficulty bridge — kelidan
-- precedent); DoAction ACTION_START_JORMUNGARS / ACTION_GORMOK_DEAD /
-- ACTION_START_ICEHOWL / ACTION_JORMUNGARS_DEAD (no cross-AI DoAction
-- bridge); EVENT_BERSERK — instance->GetCreature(gormok / dreadscale /
-- acidmaw / icehowl)->CastSpell(Berserk 26662) (instance model);
-- EVENT_START_JORGMUNGARS / EVENT_START_ICEHOWL — cross-AI tirion
-- DoAction(ACTION_START_JORMUNGARS / ACTION_START_ICEHOWL) (no
-- cross-AI bridge).
-- boss_dreadscale (34799) / boss_acidmaw (35144) — the jormungar
-- submerge/phase machine (boss_jormungarAI): Reset phase legs
-- (SetPhase(PHASE_STATIONARY) — no phase bridge; SetControlled(ROOT) —
-- no root bridge); ScheduleTasks phase-gated events (EVENT_SPRAY /
-- EVENT_SWEEP PHASE_STATIONARY; EVENT_BITE / EVENT_SPEW /
-- EVENT_SLIME_POOL PHASE_MOBILE) — no phase bridge; EVENT_BITE —
-- DoCastVictim(biteSpell: 66879 Burning Bite / 66824 Paralytic Bite),
-- 5s init, 15s repeat — port-pattern-ready in isolation but
-- phase-gated (PHASE_MOBILE) behind the unbridged submerge machine:
-- wiring it without the machine means the worm never submerges and
-- the rotation never swaps — jedoga over-cast bar; EVENT_SPEW —
-- DoCastAOE(66821 Molten Spew / 66818 Acid Spew) (no DoCastAOE bridge
-- — terestian/shazzrah precedent); EVENT_SLIME_POOL — DoCastSelf(
-- SUMMON_SLIME_POOL 66883) (summon STRAND absent — no Lua-surface
-- consumer); EVENT_SPRAY — SelectTarget(Random,1,...,true) -> DoCast(
-- 66902 Burning Spray / 66901 Paralytic Spray) (no random-target
-- bridge); EVENT_SWEEP — DoCastAOE(SPELL_SWEEP 66794) (no DoCastAOE
-- bridge); EVENT_SUBMERGE -> Submerge() — SetReactState(REACT_PASSIVE)
-- + AttackStop + HandleEmoteCommand(EMOTE_ONESHOT_SUBMERGE) +
-- DoCastSelf(66948 / 66936) + SetSpeedRate(8.0 / 1.1111) +
-- DoCastSelf(GROUND_VISUAL_0 66969) + SetFlag(NON_ATTACKABLE |
-- NOT_SELECTABLE) + MovePoint + SetControlled(ROOT) (no react / emote /
-- speed / flag / motion / root bridges); EVENT_EMERGE -> Emerge() —
-- RemoveAurasDueToSpell + DoCastSelf(SPELL_EMERGE 66947) + DoCastAOE(
-- SPELL_HATE_TO_ZERO 63984) + RemoveFlag + SetReactState(AGGRESSIVE)
-- + SelectTarget(Random) + MoveChase + SetDisplayId(
-- modelStationary/modelMobile) + DoCastSelf(GROUND_VISUAL_1 68302)
-- (no aura / DoCastAOE / flag / react / random-target / motion /
-- display bridges); EVENT_ENGAGE — DoCloseDoorOrButton(DATA_MAIN_GATE)
-- + SetImmuneToPC(false) + SetPhase(PHASE_MOBILE) +
-- SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat (dreadscale, via
-- MovementInform — no hook; no door / immune / phase / react / zone
-- bridges); EVENT_SUMMON_ACIDMAW — SummonCreature(NPC_ACIDMAW) (no
-- summon bridge); JustSummoned summons bookkeeping (no summon
-- bridge); JustDied — instance->SetData(TYPE_NORTHREND_BEASTS,
-- SNAKES_DONE / SNAKES_SPECIAL) + DespawnOrUnsummon(x2) +
-- cross-AI otherWorm DoAction(ACTION_ENRAGE) + cross-AI combatStalker
-- DoAction(ACTION_JORMUNGARS_DEAD) (instance model + no despawn /
-- cross-AI bridges); DoAction ACTION_ENRAGE — DoCastSelf(
-- SPELL_ENRAGE 68335, triggered) + Talk(EMOTE_ENRAGE 0) +
-- RescheduleEvent(EVENT_EMERGE) — cross-AI DoAction-gated (no bridge);
-- UpdateAI stationary leg DoCastVictim(spitSpell: 66796 Fire Spit /
-- 66880 Acid Spit) (phase-gated, jedoga bar).
-- npc_jormungars_slime_pool — entry-unverifiable (summoned by
-- SUMMON_SLIME_POOL 66883; no NPC_ constant in C++ — entry DB-side):
-- Reset 1s-delayed DoCastSelf(SPELL_SLIME_POOL_EFFECT 66882) +
-- DoCastSelf(SPELL_PACIFY_SELF 19951) — port-pattern-ready
-- (phase_hunter precedent) but entry-blocked.
-- npc_fire_bomb — entry-unverifiable (spawned by SPELL_FIRE_BOMB
-- 66313 target summon; no NPC_ constant — entry DB-side): Reset
-- DoCastSelf(SPELL_FIRE_BOMB_AURA 66318) — port-pattern-ready but
-- entry-blocked.
-- boss_icehowl (34797 — kalecgos pass; instance binds NPC_ICEHOWL ->
-- DATA_ICEHOWL): the whole charge machine — ScheduleTasks events are
-- PHASE_COMBAT-gated (no phase bridge); EVENT_MASSIVE_CRASH —
-- SetReactState(REACT_PASSIVE) + AttackStop + SetPhase(PHASE_CHARGE)
-- + MoveJump (no react / phase / motion bridges); MovementInform
-- POINT_MIDDLE -> DoCastSelf(SPELL_MASSIVE_CRASH 66683) +
-- ScheduleEvent(EVENT_SELECT_CHARGE_TARGET, 4s) (no MovementInform
-- hook); EVENT_SELECT_CHARGE_TARGET — SelectTarget(Random,0,0.0f,true)
-- -> DoCast(SPELL_FURIOUS_CHARGE_SUMMON 66729) + SetTarget +
-- Talk(EMOTE_TRAMPLE_ROAR 0) (no random-target bridge);
-- EVENT_ICEHOWL_ROAR / EVENT_JUMP_BACK — DoCast(stalker, SPELL_ROAR
-- 66736 / SPELL_JUMP_BACK 66733) via instance->GetCreature(
-- DATA_FURIOUS_CHARGE) (instance model); EVENT_TRAMPLE — MoveCharge
-- (no charge-motion bridge); MovementInform POINT_ICEHOWL_CHARGE ->
-- SetPhase(PHASE_COMBAT) + RescheduleTasks + SetReactState(
-- REACT_AGGRESSIVE) + DoCastSelf(SPELL_TRAMPLE 66734) (no hook);
-- EVENT_FEROCIOUS_BUTT — DoCastVictim(66770), 8s init, 20s repeat —
-- port-pattern-ready in isolation but fires only in PHASE_COMBAT and
-- is suspended/rescheduled around the unbridged charge machine —
-- jedoga over-cast bar; EVENT_WHIRL — DoCastSelf(67345), 15s init,
-- 16s repeat — same bar; EVENT_ARCTIC_BREATH — SelectTarget(Random)
-- -> DoCast(66688) (no random-target bridge); DoAction ACTION_ENRAGE
-- — DoCastSelf(SPELL_FROTHING_RAGE 66759) + Talk(EMOTE_TRAMPLE_ENRAGE
-- 2) — gated via spell_icehowl_trample CheckTargets -> DoAction (no
-- SpellScript binding bridge); DoAction ACTION_TRAMPLE_FAIL —
-- DoCastSelf(SPELL_STAGGERED_DAZE 66758) + Talk(EMOTE_TRAMPLE_FAIL 1)
-- + DelayEvents(15s) — same gate; MovementInform POINT_INITIAL_MOVEMENT
-- -> EVENT_ENGAGE 3s: DoCloseDoorOrButton + SetImmuneToPC(false) +
-- SetPhase(PHASE_COMBAT) + SetReactState(AGGRESSIVE) + DoZoneInCombat
-- (no hook; no door / immune / phase / react / zone bridges).
-- The twelve spell scripts — no SpellScript / AuraScript binding bridge
-- (razelikh precedent); join the SpellScript/AuraScript queue:
-- spell_gormok_jump_to_hand (AuraScript 66342): OnRemove (snobold
-- caster) -> gormok SelectTarget(Random,0,SnobolledTargetSelector())
-- -> gormok->AI()->Talk(EMOTE_SNOBOLLED 0) + caster DoAction(
-- ACTION_ACTIVE_SNOBOLD) + caster CastSpell(target, RIDE_PLAYER 66245)
-- (vehicle / aura / random-target bridges absent);
-- spell_gormok_ride_player (AuraScript 66245): OnApply ->
-- caster->GetAI()->SetGUID(target GUID, DATA_NEW_TARGET) (no SetGUID
-- hook); spell_gormok_snobolled (AuraScript 66406): periodic removes
-- itself without RIDE_PLAYER aura; spell_jormungars_paralytic_toxin
-- (AuraScript 66823): OnApply acidmaw Talk(SAY_SPECIAL 1) + periodic
-- slow stacking -> PARALYSIS 66830 + OnRemove removes PARALYSIS;
-- spell_jormungars_burning_bile (SpellScript 66870): removes
-- MOD_DECREASE_SPEED auras; spell_jormungars_slime_pool (AuraScript
-- 66882): periodic trigger-spell with radius mod; spell_jormungars_
-- burning_spray / spell_jormungars_paralytic_spray (SpellScript
-- 66902 / 66901): cast 66869 Burning Bile / 66823 Paralytic Toxin on
-- hit unit; spell_jormungars_paralysis (AuraScript 66830): OnApply
-- removes itself when DATA_NORTHREND_BEASTS boss state != IN_PROGRESS
-- (instance model); spell_icehowl_arctic_breath (SpellScript 66688):
-- casts CalcValue spell on hit unit; spell_icehowl_trample (SpellScript
-- 66734): CheckTargets -> DoAction(TRAMPLE_FAIL) if empty else
-- DoAction(ENRAGE) (no SpellScript / cross-AI DoAction bridges);
-- spell_icehowl_massive_crash (AuraScript 66683): AfterEffectRemove ->
-- heroic player casts SURGE_OF_ADRENALINE 68667 (no difficulty /
-- aura bridges).

local ENTRY_GORMOK = 34796

local SPELL_IMPALE = 66331
local SPELL_STAGGERING_STOMP = 66330

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

-- C++ EVENT_IMPALE: DoCastVictim(Impale), 10s init, 10s repeat.
local function impaleTick(creature, guid)
    creature:CastSpell(nil, SPELL_IMPALE)
    schedule(guid, "impale", 10000, function()
        impaleTick(creature, guid)
    end)
end

-- C++ EVENT_STAGGERING_STOMP: DoCastVictim(Staggering Stomp),
-- 15s init, 22s repeat.
local function stompTick(creature, guid)
    creature:CastSpell(nil, SPELL_STAGGERING_STOMP)
    schedule(guid, "stomp", 22000, function()
        stompTick(creature, guid)
    end)
end

-- C++ boss_northrend_beastsAI::JustEngagedWith -> ScheduleTasks().
-- (C++ EVENT_THROW, the third scheduled task, is vehicle-bound —
-- documented-only above.)
local function enterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "impale", 10000, function()
        impaleTick(creature, guid)
    end)
    schedule(guid, "stomp", 15000, function()
        stompTick(creature, guid)
    end)
end

local function leaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function reset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GORMOK, 1, enterCombat)
RegisterCreatureEvent(ENTRY_GORMOK, 2, leaveCombat)
RegisterCreatureEvent(ENTRY_GORMOK, 23, reset)
