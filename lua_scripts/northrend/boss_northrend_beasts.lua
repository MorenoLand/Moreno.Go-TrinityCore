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
-- boss_northrend_beastsAI::JustEngagedWith HandleInstanceProgress leg
-- is PORTED tick 405 U2 (GORMOK_IN_PROGRESS count init +
-- SetBossState IN_PROGRESS in enterCombat); setActive and the heroic
-- HandleWithHeroicEvents (cross-AI combatStalker DoAction
-- ACTION_START_JORMUNGARS) have no bridge — tharon_ja precedent;
-- the base Reset's SetCombatPulseDelay(0) arm and JustEngagedWith's
-- SetCombatPulseDelay(5) arm are PORTED (creature:SetCombatPulseDelay
-- bridge, lua_creature_events.go; events 23/1, C++ arm order);
-- EVENT_IMPALE — DoCastVictim(Impale 66331), 10s init, 10s repeat
-- (moroes precedent; gormok's ScheduleTasks carries no phase args and
-- C++ UpdateAI runs the events whenever UpdateVictim() holds, so no
-- phase gate is modeled); EVENT_STAGGERING_STOMP —
-- DoCastVictim(Staggering Stomp 66330), 15s init, 22s repeat
-- (moroes precedent). Gormok has no Talk arms on
-- JustEngagedWith/KilledUnit/JustDied. CASTING gates PORTED (500ms
-- IsCasting defer, mal_ganis idiom): boss_northrend_beastsAI::UpdateAI
-- pre-loop :314 + in-loop :320 gates cover impale/stomp; the defer
-- re-queues without consuming the repeat. All timers cancelled on 2/23
-- (gargolmar precedent; 23 fires before 4 on the kill path, so the
-- unregistered event 4 is benign).
-- Unmodeled (no bridges — documented, not wired):
-- boss_gormok: Reset: events.SetPhase(PHASE_EVENT) (no phase bridge —
-- anubarak_trial precedent) + HandleInitialMovement
-- MoveAlongSplineChain(POINT_INITIAL_MOVEMENT, SPLINE_INITIAL_MOVEMENT)
-- (no motion bridge); Reset SetReactState(REACT_PASSIVE)
-- (boss_northrend_beasts.cpp:217) also stays out: SetReactState IS bridged
-- (frozen_core precedent), but the paired re-arm (EVENT_ENGAGE ->
-- SetReactState(REACT_AGGRESSIVE), :358) rides the unbridged MovementInform
-- intro machine (MovementInform SPLINE_CHAIN -> EVENT_ENGAGE 7s, :345-348)
-- — porting PASSIVE alone would leave the boss passive forever (paired-arm rule).
-- ScheduleEvent(EVENT_ENGAGE, 7s) (no MovementInform hook); EVENT_ENGAGE
-- — DoUseDoorOrButton(DATA_MAIN_GATE) + SetImmuneToPC(false) +
-- SetReactState(REACT_AGGRESSIVE) + SummonCreature(
-- NPC_BEASTS_COMBAT_STALKER) + stalker SetCombatPulseDelay(5) +
-- DoZoneInCombat(x2) + events.SetPhase(PHASE_COMBAT) +
-- DoCastSelf(SPELL_TANKING_GORMOK 66415, triggered — SERVERSIDE spell,
-- "No idea what it does" per the C++ comment) (no door / immune /
-- phase / zone bridges and no MovementInform hook — the summon and
-- react legs ARE bridged in isolation but ride the unbridged intro
-- machine — the self-cast is
-- port-pattern-ready in isolation (phase_hunter precedent) but rides
-- the unbridged intro machine — jedoga over-cast bar); EVENT_THROW —
-- vehicle-passenger walk (GetVehicleKit()->GetPassenger(i) ->
-- ExitVehicle + RemoveFlag(NON_ATTACKABLE | NOT_SELECTABLE) +
-- cross-AI snobold DoAction(ACTION_DISABLE_FIRE_BOMB) + CastSpell(me,
-- SPELL_JUMP_TO_HAND 66342)) (no vehicle / cross-AI DoAction bridges);
-- PassengerBoarded seat GORMOK_HAND_SEAT (4) -> CastSpell(
-- SPELL_RISING_ANGER 66636) (no vehicle bridge); JustDied —
-- PORTED tick 405 U2 (disengage was already ported; GORMOK_DONE
-- count leg now via beastsCountDeath; the Tirion DoAction(
-- ACTION_START_JORMUNGARS) leg has no Tirion Lua; the combatStalker
-- DoAction(ACTION_GORMOK_DEAD) leg stays out — no stalker ev38
-- receiver; no Talk arms); EnterEvadeMode — PORTED tick 406 U3 via
-- ev2 (beastsEvaded, all four entries): snobold grid-sweep despawn
-- (:283) + disengage (:284) + FAIL write (:285-287) + self-despawn
-- (:291); the combatStalker despawn (:288-289) stays out (no
-- SetGuidData(24) latch, stalker only exists via the unported
-- EVENT_ENGAGE summon), summons.DespawnAll (:290) stays out (no
-- summons-enumeration bridge), and the instance FAIL cascade's
-- vehicle-removal leg stays out (no vehicle bridge; SnoboldCount = 0
-- is PORTED tick 411 U3 in beastsEvaded — it resets the achievement
-- counter mirror) — the Tirion wipe DoAction + heroic TrialCounter
-- decrement legs are PORTED tick 406 U2 in beastsEvaded
-- (sendToFordring + instanceData.trialCounter; receiver
-- npc_tirion_fordring.lua, tick 406 U2).
-- npc_snobold_vassal (34800 — header constant; instance OnCreatureCreate
-- tracks NPC_SNOBOLD_VASSAL GUIDs — entry-evidence exists but the
-- ScriptName binding is DB-side): the whole vehicle mount machine —
-- MountOnBoss EnterVehicle(gormok, seat) (no vehicle bridge);
-- SetGUID(DATA_NEW_TARGET) -> AttackStart + EVENT_BATTER /
-- EVENT_SNOBOLLED / EVENT_HEAD_CRACK (no SetGUID hook); DoAction
-- ACTION_ENABLE_FIRE_BOMB / ACTION_DISABLE_FIRE_BOMB /
-- ACTION_ACTIVE_SNOBOLD — cross-AI DoAction-gated (no bridge);
-- EVENT_FIRE_BOMB — SelectTarget(Random,0,0.0f,true) -> CastSpell(
-- SPELL_FIRE_BOMB 66313), 12s/25s init, 20s/30s repeat — the
-- random-target leg is bridged (GetPlayersInWorld + math.random,
-- nightbane/midnight precedent); it stays out on the true blockers:
-- the MountOnBoss vehicle mount machine (no vehicle bridge) and the
-- cross-AI DoAction ENABLE/DISABLE cancel-rearm
-- (boss_northrend_beasts.cpp:441) — porting it alone would
-- over-cast (jedoga bar); EVENT_BATTER /
-- EVENT_HEAD_CRACK — DoCast(vehicleBase, 66408 / 66407) else
-- DoCastVictim (DoCastVictim legs port-pattern-ready in isolation but
-- fire only via the vehicle/SetGUID/SetCombatMovement machine —
-- jedoga over-cast bar); EVENT_SNOBOLLED — DoCastSelf(
-- SPELL_SNOBOLLED 66406) (vehicle-gated); EVENT_CHECK_MOUNT —
-- MountOnBoss() 3s loop; JustDied — instance->SetData(
-- DATA_SNOBOLD_COUNT, DECREASE) PORTED tick 411 U3 (snoboldDied, ev4)
-- + ctor INCREASE via ev5 (snoboldSpawned) + FAIL reset in
-- beastsEvaded — the "ZERO readers" claim is STALE: the reader is
-- CheckAchievementCriteriaMeet (UPPER_BACK_PAIN, instance :626-631),
-- wired to the Go DATA_TYPE_INSTANCE_SCRIPT gate tick 411 U3. The
-- mirror is dormant until the vehicle machine lands (snobolds arrive
-- as Gormok's passengers); CanAIAttack /
-- AttackStart / AttackStartCaster movement legs (no motion bridges).
-- npc_beasts_combat_stalker (36549 — kalecgos pass; instance binds
-- NPC_BEASTS_COMBAT_STALKER -> DATA_BEASTS_COMBAT_STALKER): Reset —
-- SetReactState(REACT_PASSIVE) (SetReactState IS bridged —
-- frozen_core precedent; the arm stays out behind the
-- instance / combat-stalker model) + EVENT_BERSERK
-- IsHeroic() ? 9min : 15min (stale "no difficulty bridge" retracted:
-- IsHeroic() IS bridged, lua_creature_events.go:260 — the arm stays
-- out behind the instance/combat-stalker model, not difficulty);
-- DoAction ACTION_START_JORMUNGARS / ACTION_GORMOK_DEAD /
-- ACTION_START_ICEHOWL / ACTION_JORMUNGARS_DEAD (stale "no cross-AI
-- DoAction bridge" retracted tick 405 U2: DoAction IS bridged,
-- lua_creature_events.go:827, event 38 — the stalker has no ev38
-- receiver in Lua and its arms only cancel timers of the unported
-- Tirion machine, so the stay-out is receiver-side, not bridge-side);
-- EVENT_BERSERK — instance->GetCreature(gormok / dreadscale /
-- acidmaw / icehowl)->CastSpell(Berserk 26662) (instance model);
-- EVENT_START_JORGMUNGARS / EVENT_START_ICEHOWL — cross-AI tirion
-- DoAction(ACTION_START_JORMUNGARS / ACTION_START_ICEHOWL) (no
-- cross-AI bridge).
-- boss_dreadscale (34799) / boss_acidmaw (35144) — the jormungar
-- submerge/phase machine (boss_jormungarAI): Reset phase legs
-- (SetPhase(PHASE_STATIONARY) — no phase bridge; SetControlled(ROOT) —
-- root IS bridged (creature:SetRooted, lua_creature_events.go) but the arm
-- stays out — the submerge machine needs MovePoint/motion/react/flag/phase
-- legs with no bridges); ScheduleTasks phase-gated events (EVENT_SPRAY /
-- EVENT_SWEEP PHASE_STATIONARY; EVENT_BITE / EVENT_SPEW /
-- EVENT_SLIME_POOL PHASE_MOBILE) — no phase bridge; EVENT_BITE —
-- DoCastVictim(biteSpell: 66879 Burning Bite / 66824 Paralytic Bite),
-- 5s init, 15s repeat — port-pattern-ready in isolation but
-- phase-gated (PHASE_MOBILE) behind the unbridged submerge machine:
-- wiring it without the machine means the worm never submerges and
-- the rotation never swaps — jedoga over-cast bar; EVENT_SPEW —
-- DoCastAOE(66821 Molten Spew / 66818 Acid Spew) (stays out:
-- PHASE_MOBILE-gated behind the unbridged submerge machine — gothik
-- precedent); EVENT_SLIME_POOL — DoCastSelf(
-- SUMMON_SLIME_POOL 66883) (summon STRAND absent — no Lua-surface
-- consumer); EVENT_SPRAY — SelectTarget(Random,1,...,true) -> DoCast(
-- 66902 Burning Spray / 66901 Paralytic Spray) (random-target leg is
-- bridged; true blocker: PHASE_STATIONARY gated behind the unbridged
-- submerge/emerge machine (boss_northrend_beasts.cpp:683)); EVENT_SWEEP — DoCastAOE(SPELL_SWEEP 66794) (stays out:
-- PHASE_STATIONARY-gated behind the unbridged submerge machine);
-- EVENT_SUBMERGE -> Submerge() — SetReactState(REACT_PASSIVE)
-- + AttackStop + HandleEmoteCommand(EMOTE_ONESHOT_SUBMERGE) +
-- DoCastSelf(66948 / 66936) + SetSpeedRate(8.0 / 1.1111) +
-- DoCastSelf(GROUND_VISUAL_0 66969) + SetFlag(NON_ATTACKABLE |
-- NOT_SELECTABLE) + MovePoint + SetControlled(ROOT) (no react / emote /
-- speed / flag / motion / phase bridges — SetRooted is bridged but the
-- paired submerge-machine legs are not); EVENT_EMERGE -> Emerge() —
-- RemoveAurasDueToSpell + DoCastSelf(SPELL_EMERGE 66947) + DoCastAOE(
-- SPELL_HATE_TO_ZERO 63984) + RemoveFlag + SetReactState(AGGRESSIVE)
-- + SelectTarget(Random) + MoveChase + SetDisplayId(
-- modelStationary/modelMobile) + DoCastSelf(GROUND_VISUAL_1 68302)
-- (63984's effect is the spell_yogg_saron_hate_to_zero SpellScript —
-- shazzrah precedent; the rest rides the unbridged submerge-machine
-- legs); SetControlled(true, UNIT_STATE_ROOT) at
-- boss_northrend_beasts.cpp:779 (Emerge()'s wasMobile leg) rides the
-- same machine: stays out. EVENT_ENGAGE — DoCloseDoorOrButton(DATA_MAIN_GATE)
-- + SetImmuneToPC(false) + SetPhase(PHASE_MOBILE) +
-- SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat (dreadscale, via
-- MovementInform — no hook; no door / immune / phase / zone
-- bridges); EVENT_SUMMON_ACIDMAW — SummonCreature(NPC_ACIDMAW) (summon
-- leg bridged; blocked by the submerge/phase machine, not the bridge);
-- JustSummoned summons bookkeeping (no summon bridge); JustDied —
-- PORTED tick 405 U2 (shared boss_jormungarAI :701-720, both worm
-- entries via wormDied + ev5 SetGuidData latches): disengage +
-- SNAKES_SPECIAL / SNAKES_DONE count legs via beastsCountDeath +
-- surviving-worm DoAction(ACTION_ENRAGE) relay via the live event-38
-- bridge + both-corpses DespawnOrUnsummon; the combatStalker DoAction(
-- ACTION_JORMUNGARS_DEAD) leg stays out (no stalker ev38 receiver)
-- and the 10s NotOneButTwoJormungarsTimer gates only unbridged
-- achievement criteria; DoAction ACTION_ENRAGE — PORTED tick 405 U2
-- (ev38 wormDoAction: DoCastSelf(SPELL_ENRAGE 68335, triggered) +
-- Talk(EMOTE_ENRAGE 0); the RescheduleEvent(EVENT_EMERGE) leg rides
-- the unbridged submerge machine and stays out);
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
-- + MoveJump (react/attack-stop bridged; phase + motion unbridged);
--   MovementInform
-- POINT_MIDDLE -> DoCastSelf(SPELL_MASSIVE_CRASH 66683) +
-- ScheduleEvent(EVENT_SELECT_CHARGE_TARGET, 4s) (no MovementInform
-- hook); EVENT_SELECT_CHARGE_TARGET — SelectTarget(Random,0,0.0f,true)
-- -> DoCast(SPELL_FURIOUS_CHARGE_SUMMON 66729) + SetTarget +
-- Talk(EMOTE_TRAMPLE_ROAR 0) (random-target leg is bridged; true
-- blocker: the unbridged charge machine — MoveJump ->
-- MovementInform -> MoveCharge — no motion/movement hooks);
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
-- 16s repeat — same bar; EVENT_ARCTIC_BREATH — SelectTarget(Random,0,0.0f,true) ->
-- DoCast(66688), 20s init / 24s re-arm (:951/:1050-1053) — stays OUT:
-- PHASE_COMBAT-gated (ScheduleTasks phase arg), suspended across the
-- unbridged charge phase by the phase switch — jedoga over-cast bar;
-- stale "no random-target bridge" retracted; DoAction ACTION_ENRAGE
-- — DoCastSelf(SPELL_FROTHING_RAGE 66759) + Talk(EMOTE_TRAMPLE_ENRAGE
-- 2) — gated via spell_icehowl_trample CheckTargets -> DoAction (no
-- SpellScript binding bridge); DoAction ACTION_TRAMPLE_FAIL —
-- DoCastSelf(SPELL_STAGGERED_DAZE 66758) + Talk(EMOTE_TRAMPLE_FAIL 1)
-- + DelayEvents(15s) — same gate; MovementInform POINT_INITIAL_MOVEMENT
-- -> EVENT_ENGAGE 3s: DoCloseDoorOrButton + SetImmuneToPC(false) +
-- SetPhase(PHASE_COMBAT) + SetReactState(AGGRESSIVE) + DoZoneInCombat
-- (no hook; no door / immune / phase / zone bridges). JustDied —
-- PORTED tick 405 U2: boss_icehowl has NO JustDied override, it
-- inherits the base (:294-306) else-branch SetData ICEHOWL_DONE ->
-- beastsCountDeath (count 0 -> SetBossState DATA_NORTHREND_BEASTS
-- DONE) + disengage; no Talk arms.
-- The twelve spell scripts — no SpellScript / AuraScript binding bridge
-- (razelikh precedent); eleven join the SpellScript/AuraScript queue
-- (slime_pool PORTED below, ev1 + CastSpellWithRadiusMod):
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
-- 66882): periodic trigger-spell with radius mod — PORTED below (ev1 +
-- CastSpellWithRadiusMod); spell_jormungars_
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
local ENTRY_DREADSCALE = 34799
local ENTRY_ACIDMAW = 35144
local ENTRY_ICEHOWL = 34797
local ENTRY_SNOBOLD_VASSAL = 34800 -- trial_of_the_crusader.h:175 (NPC_SNOBOLD_VASSAL)

local SPELL_IMPALE = 66331
local SPELL_STAGGERING_STOMP = 66330
local SPELL_ENRAGE = 68335 -- boss_northrend_beasts.cpp:77 (worm DoAction ACTION_ENRAGE)
local SPELL_SLIME_POOL_EFFECT = 66882 -- boss_northrend_beasts.cpp:96

-- trial_of_the_crusader.h: DATA_ACIDMAW 7 / DATA_DREADSCALE 8 /
-- DATA_NORTHREND_BEASTS 0 (boss id for the beasts encounter).
local DATA_ACIDMAW = 7
local DATA_DREADSCALE = 8
local DATA_NORTHREND_BEASTS = 0
local DATA_ANUBARAK = 5 -- trial_of_the_crusader.h:37
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_FAIL = 2 -- EncounterState FAIL (InstanceScript.h:68-76)
local BOSS_STATE_DONE = 3

local EMOTE_ENRAGE = 0 -- boss_northrend_beasts.cpp:37 (Yells; worm Talk)
local ACTION_ENRAGE = 4 -- boss_northrend_beasts.cpp:119 (MyActions)
local DATA_FORDRING = 12 -- trial_of_the_crusader.h:46
local ENTRY_FORDRING = 34996 -- trial_of_the_crusader.h:156 (NPC_TIRION_FORDRING)
local ACTION_NORTHREND_BEASTS_WIPE = 5 -- trial_of_the_crusader.h:102 (TCRActions)
local ACTION_NORTHREND_BEASTS_DEFEATED = 6 -- trial_of_the_crusader.h:103

-- boss_jormungarAI cross-refs (boss_northrend_beasts.cpp:696-698,
-- :897/:921): each worm resolves the OTHER via the instance GUID
-- store, and latches its own GUID under its own DATA_ id on spawn.
local WORM_DATA = { [ENTRY_DREADSCALE] = DATA_DREADSCALE, [ENTRY_ACIDMAW] = DATA_ACIDMAW }
local OTHER_WORM_DATA = { [ENTRY_DREADSCALE] = DATA_ACIDMAW, [ENTRY_ACIDMAW] = DATA_DREADSCALE }

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

-- Fordring relay: C++ instance arms call tirion->AI()->DoAction(n)
-- (instance_trial_of_the_crusader.cpp:343/:411); GetInstanceCreature +
-- creature:DoAction(n) are bridged (DoAction fires ev38 on the
-- target, lua_creature_events.go:827/:6224). Receiver
-- npc_tirion_fordring.lua registers Fordring 34996 with an ev38 DoAction
-- handler (tick 406 U2).
local function sendToFordring(creature, action)
    local fordring = creature:GetInstanceCreature(DATA_FORDRING)
    if fordring == nil then fordring = creature:FindNearestCreature(ENTRY_FORDRING, 200) end
    if fordring ~= nil then fordring:DoAction(action) end
end

-- C++ instance SetData(TYPE_NORTHREND_BEASTS, ...) machine
-- (instance_trial_of_the_crusader.cpp:376-413): NorthrendBeastsCount
-- starts at 4 on GORMOK_IN_PROGRESS (:380-383) and
-- HandleNorthrendBeastsDone (:330-345) decrements once per
-- GORMOK_DONE / SNAKES_SPECIAL / SNAKES_DONE / ICEHOWL_DONE, firing
-- SetBossState(DATA_NORTHREND_BEASTS, DONE) at 0. The DONE-time
-- stalker-despawn / vehicle-removal legs stay out (no bridges); the
-- Tirion DoAction(ACTION_NORTHREND_BEASTS_DEFEATED) leg is PORTED
-- tick 406 U2 via sendToFordring in the count-0 branch below.
-- Counter lives in the shared instanceData table (the
-- boss_faction_champions.lua championsKilled pattern, same instance).
local function beastsCountDeath(creature)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    local remaining = (data.northrendBeastsRemaining or 4) - 1
    data.northrendBeastsRemaining = remaining
    if remaining <= 0 then
        creature:SetBossState(DATA_NORTHREND_BEASTS, BOSS_STATE_DONE)
        sendToFordring(creature, ACTION_NORTHREND_BEASTS_DEFEATED)
    end
end

-- C++ npc_snobold_vassal ctor (:401) / JustDied (:430): instance->
-- SetData(DATA_SNOBOLD_COUNT, INCREASE/DECREASE). Lua mirror in the
-- shared instanceData table (snoboldCount, default 0; beastsCountDeath
-- pattern above) — the reader is the DATA_TYPE_INSTANCE_SCRIPT
-- achievement gate (Go, checkTrialOfTheCrusaderCriteria): UPPER_BACK_
-- PAIN needs SnoboldCount >= 2 (10-player) / >= 4 (25-player).
-- The ctor has no Lua hook; ev5 OnSpawn is the spawn twin (fires on
-- summon/respawn via fireCreatureSpawned, creaturemotion.go:1262 —
-- never on evade, unlike ev23, so no double-count on leash). The FAIL
-- reset (SnoboldCount = 0, instance :412) rides beastsEvaded below.
-- Dormant until the vehicle machine lands: snobolds arrive as Gormok's
-- vehicle passengers (boss_northrend_beasts.cpp:378 / EVENT_THROW),
-- and the intro summon is unported — no snobold materializes in Go
-- today, so the count stays 0 and the gate stays fail-closed.
local function snoboldSpawned(event, creature)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    data.snoboldCount = (data.snoboldCount or 0) + 1
end

local function snoboldDied(event, creature, killer)
    local mapObj = creature:GetMap()
    if mapObj == nil then
        return
    end
    local data = mapObj:GetInstanceData()
    if data == nil then
        return
    end
    local count = (data.snoboldCount or 0) - 1
    if count < 0 then
        count = 0
    end
    data.snoboldCount = count
end

-- C++ EVENT_IMPALE: DoCastVictim(Impale), 10s init, 10s repeat.
local function impaleTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "impale", 500, function()
            impaleTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_IMPALE)
    schedule(guid, "impale", 10000, function()
        impaleTick(creature, guid)
    end)
end

-- C++ EVENT_STAGGERING_STOMP: DoCastVictim(Staggering Stomp),
-- 15s init, 22s repeat.
local function stompTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stomp", 500, function()
            stompTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_STAGGERING_STOMP)
    schedule(guid, "stomp", 22000, function()
        stompTick(creature, guid)
    end)
end

-- C++ boss_northrend_beastsAI::JustEngagedWith -> ScheduleTasks().
-- (C++ EVENT_THROW, the third scheduled task, is vehicle-bound —
-- documented-only above.)
local function enterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- boss_northrend_beasts.cpp:251 (base JustEngagedWith arm 1; setActive leg has no bridge)
    -- C++ HandleInstanceProgress GORMOK_IN_PROGRESS
    -- (boss_northrend_beasts.cpp:264-266 -> instance :380-383):
    -- NorthrendBeastsCount = 4 + SetBossState(DATA_NORTHREND_BEASTS,
    -- IN_PROGRESS). The HandlePlayerVehicle(true) leg has no bridge.
    -- Re-init on every Gormok engage, exactly like C++ (each
    -- GORMOK_IN_PROGRESS re-arms the count); Go's DONE-never-backwards
    -- guard keeps a finished encounter DONE. This init is the
    -- prerequisite for the JustDied count machine (beastsCountDeath).
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.northrendBeastsRemaining = 4
        end
    end
    creature:SetBossState(DATA_NORTHREND_BEASTS, BOSS_STATE_IN_PROGRESS)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "impale", 10000, function()
        impaleTick(creature, guid)
    end)
    schedule(guid, "stomp", 15000, function()
        stompTick(creature, guid)
    end)
    -- C++ JustEngagedWith (:255): encounter-frame engage after
    -- ScheduleTasks/HandleInstanceProgress (the two schedule() calls
    -- above are the ScheduleTasks legs).
    creature:SendEncounterUnit(0, creature) -- ENCOUNTER_FRAME_ENGAGE
end

-- C++ boss_northrend_beastsAI::EnterEvadeMode
-- (boss_northrend_beasts.cpp:280-291), shared by all four beasts.
-- Go's triggerCreatureEvade (engine/world/creaturemotion.go:4664)
-- fires event 23 then event 2 ahead of the base evade, only when the
-- creature was in combat, so event 2 IS the EnterEvadeMode hook here;
-- death never reaches it, so the FAIL leg cannot fire on a kill. The
-- base legs (CreatureAI.cpp:214-236: MoveTargetedHome + Reset) are
-- engine-covered and are NOT re-implemented. The handler must return
-- nothing: returning true vetoes the whole base evade
-- (creaturemotion.go:4699-4704). Order is C++ order: snobolds, then
-- disengage, then the FAIL write, then self-despawn last (the
-- creature object is unusable for map lookups after Despawn).
-- Caveat: the wasInCombat gate means a pre-combat evade (intro
-- leash, before any engage) sets FAIL in C++ but fires no ev2 in Lua.
local function beastsEvaded(event, creature)
    -- :283 instance->SetData(DATA_DESPAWN_SNOBOLDS, 0)
    -- (instance_trial_of_the_crusader.cpp:418-422: despawn every
    -- tracked snobold GUID). Lua mirror: entry-exact grid scan —
    -- every snobold lives in-arena and no other 34800 exists in the
    -- instance; 200 yd covers the whole floor, same map+instance only
    -- (lua_creature_events.go:5949 GetCreaturesInGrid; anubarak
    -- mass-despawn precedent).
    for _, snobold in ipairs(creature:GetCreaturesInGrid(ENTRY_SNOBOLD_VASSAL, 200)) do
        snobold:DespawnOrUnsummon()
    end
    -- :284 SendEncounterUnit(ENCOUNTER_FRAME_DISENGAGE, me).
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    -- :285-287 the heroic double-attempt guard + SetData FAIL, whose
    -- instance cascade (instance :407-413) is SetBossState FAIL.
    -- Guard mirrors C++ GetData(TYPE_NORTHREND_BEASTS) != FAIL on the
    -- boss-state store (the Lua instanceData table tracks no TYPE_
    -- value); DONE-never-backwards is the engine backstop
    -- (instance_boss_states.go:190-196). The cascade's
    -- HandlePlayerVehicle(false) leg has no vehicle bridge and
    -- SnoboldCount = 0 is PORTED tick 411 U3 above (it resets the
    -- achievement counter mirror, whose reader is the
    -- instance-criteria gate). The Tirion
    -- DoAction(ACTION_NORTHREND_BEASTS_WIPE) leg is PORTED tick 406 U2
    -- via sendToFordring below (ev38; receiver npc_tirion_fordring.lua),
    -- and the heroic TrialCounter decrement of the generic SetBossState
    -- FAIL arm (instance :301-311, --TrialCounter at :305) is PORTED
    -- as the shared instanceData.trialCounter field (default 50,
    -- floored at 0; one of four FAIL writers — jaraxxus / champions /
    -- valkyr also decrement it).
    -- The TrialCounter == 0 -> despawn-Anubarak leg (:313-317) is
    -- PORTED below (tick 408 U1).
    if creature:GetBossState(DATA_NORTHREND_BEASTS) ~= BOSS_STATE_FAIL then
        creature:SetBossState(DATA_NORTHREND_BEASTS, BOSS_STATE_FAIL)
        -- SnoboldCount = 0 (instance :412, FAIL cascade; unconditional,
        -- unlike the heroic-only TrialCounter decrement below). The
        -- count mirror's reader is the tick-411 instance-criteria gate
        -- (UPPER_BACK_PAIN), so this is no longer a dead write.
        local failMap = creature:GetMap()
        if failMap then
            local failData = failMap:GetInstanceData()
            if failData then
                failData.snoboldCount = 0
            end
        end
        sendToFordring(creature, ACTION_NORTHREND_BEASTS_WIPE)
        if creature:IsHeroic() then
            local mapObj = creature:GetMap()
            if mapObj then
                local data = mapObj:GetInstanceData()
                if data then
                    local counter = (data.trialCounter or 50) - 1
                    if counter < 0 then
                        counter = 0
                    end
                    data.trialCounter = counter
                    -- TrialCounter == 0 -> despawn Anub'arak (instance :313-317).
                    if counter == 0 then
                        local anubarak = creature:GetInstanceCreature(DATA_ANUBARAK)
                        if anubarak then
                            anubarak:DespawnOrUnsummon()
                        end
                    end
                end
            end
        end
    end
    -- :288-289 combatStalker DespawnOrUnsummon stays out: no
    -- SetGuidData(24) latch exists and the stalker only ever exists
    -- via the unported EVENT_ENGAGE summon. :290 summons.DespawnAll
    -- stays out: no summons-enumeration bridge on the Lua surface.
    -- :291 me->DespawnOrUnsummon (midnight attumen ev2 precedent).
    creature:DespawnOrUnsummon()
end

local function leaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    beastsEvaded(event, creature)
end

-- C++ boss_northrend_beastsAI::JustDied (:294-306), Gormok branch:
-- disengage (:295) + instance->SetData(TYPE_NORTHREND_BEASTS,
-- GORMOK_DONE) (:299) + combatStalker DoAction(ACTION_GORMOK_DEAD)
-- (:300-301). The SetData leg is PORTED tick 405 U2 via
-- beastsCountDeath (count 4 -> 3; the instance-side Tirion DoAction(
-- ACTION_START_JORMUNGARS) leg has no Tirion Lua and stays out).
-- The stalker DoAction leg stays out: no stalker ev38 receiver, and
-- its arms only cancel EVENT_START_JORMUNGARS timers of the unported
-- Tirion machine.
local function gormokDied(event, creature, killer)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    cancelTimers(creature:GetGUID())
    beastsCountDeath(creature)
end

local function reset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetCombatPulseDelay(0) -- boss_northrend_beasts.cpp:218 (base Reset arm; SetReactState leg stays out on the paired-arm rule — see header)
end

-- C++ instance OnCreatureCreate latching
-- (instance_trial_of_the_crusader.cpp:44-45): NPC_ACIDMAW ->
-- DATA_ACIDMAW, NPC_DREADSCALE -> DATA_DREADSCALE, so each worm's
-- JustDied can resolve the other via GetInstanceCreature
-- (auriaya/sartharion ev5 SetGuidData precedent).
local function wormSpawned(event, creature)
    creature:SetGuidData(WORM_DATA[creature:GetEntry()])
end

-- C++ boss_jormungarAI::DoAction ACTION_ENRAGE (:723-731):
-- DoCastSelf(SPELL_ENRAGE, triggered) + Talk(EMOTE_ENRAGE). The
-- PHASE_SUBMERGED EVENT_EMERGE reschedule leg rides the unbridged
-- submerge machine (no phase bridge) and stays out.
local function wormDoAction(event, creature, action)
    if action ~= ACTION_ENRAGE then
        return
    end
    creature:CastSpell(creature, SPELL_ENRAGE, true)
    creature:Talk(EMOTE_ENRAGE)
end

-- C++ boss_jormungarAI::JustDied (:701-720), shared by
-- boss_dreadscale (otherWormEntry NPC_ACIDMAW, :897) and boss_acidmaw
-- (otherWormEntry NPC_DREADSCALE, :921). Disengage first (gormokDied
-- precedent — C++ has it last at :719, but the despawns below would
-- strand the Go motion lookup); the nil-otherWorm path is C++-exact
-- (GetCreature nil -> disengage only). The NotOneButTwoJormungarsTimer
-- (10s, instance :394/:556) gates only the SNAKES_DONE achievement
-- criteria, which has no bridge, so the timer itself stays out.
local function wormDied(event, creature, killer)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    local otherWorm = creature:GetInstanceCreature(OTHER_WORM_DATA[creature:GetEntry()])
    if otherWorm == nil then
        return
    end
    if otherWorm:IsAlive() then
        -- SNAKES_SPECIAL (:715-716): the surviving worm enrages via
        -- the live cross-AI DoAction bridge (event 38; the
        -- illidari-council / selin precedent).
        beastsCountDeath(creature)
        otherWorm:DoAction(ACTION_ENRAGE)
    else
        -- SNAKES_DONE (:706-711): both corpses despawn (self first,
        -- then the other worm); the combatStalker DoAction(
        -- ACTION_JORMUNGARS_DEAD) leg stays out (no stalker ev38
        -- receiver; its arms only cancel EVENT_START_ICEHOWL timers
        -- of the unported Tirion machine).
        beastsCountDeath(creature)
        creature:DespawnOrUnsummon()
        otherWorm:DespawnOrUnsummon()
    end
end

-- C++ boss_icehowl has NO JustDied override — it inherits
-- boss_northrend_beastsAI::JustDied (:294-306), else-branch:
-- instance->SetData(TYPE_NORTHREND_BEASTS, ICEHOWL_DONE) (:304).
-- No Talk arms. The count hits 0 here, firing SetBossState DONE.
local function icehowlDied(event, creature, killer)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    beastsCountDeath(creature)
end

RegisterCreatureEvent(ENTRY_GORMOK, 1, enterCombat)
RegisterCreatureEvent(ENTRY_GORMOK, 2, leaveCombat)
RegisterCreatureEvent(ENTRY_GORMOK, 4, gormokDied)
RegisterCreatureEvent(ENTRY_GORMOK, 23, reset)
RegisterCreatureEvent(ENTRY_DREADSCALE, 4, wormDied)
RegisterCreatureEvent(ENTRY_DREADSCALE, 5, wormSpawned)
RegisterCreatureEvent(ENTRY_DREADSCALE, 38, wormDoAction)
RegisterCreatureEvent(ENTRY_DREADSCALE, 2, beastsEvaded)
RegisterCreatureEvent(ENTRY_ACIDMAW, 4, wormDied)
RegisterCreatureEvent(ENTRY_ACIDMAW, 5, wormSpawned)
RegisterCreatureEvent(ENTRY_ACIDMAW, 38, wormDoAction)
RegisterCreatureEvent(ENTRY_ACIDMAW, 2, beastsEvaded)
RegisterCreatureEvent(ENTRY_ICEHOWL, 4, icehowlDied)
RegisterCreatureEvent(ENTRY_ICEHOWL, 2, beastsEvaded)
RegisterCreatureEvent(ENTRY_SNOBOLD_VASSAL, 5, snoboldSpawned)
RegisterCreatureEvent(ENTRY_SNOBOLD_VASSAL, 4, snoboldDied)

-- C++ spell_jormungars_slime_pool::PeriodicTick (boss_northrend_beasts.cpp:1221-1228):
-- EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL on 66882. PreventDefaultAction,
-- so the Go default trigger-spell cast is skipped (return true). No
-- total-tick guard in C++ — the radius mod grows off a fixed 60-tick
-- denominator: ((tick/60) * 0.9 + 0.1) * 10000 * 2/3 (C++ int32
-- truncation). The trigger spell id comes from the aura's own
-- Effects[0].TriggerSpell (DBC-side, read via aura:GetTriggerSpell(0);
-- 0 without Spell.dbc means no cast). The slime pool (which self-casts
-- 66882 in Reset) casts the trigger spell on itself (C++
-- CastSpell(nullptr, ...) — self-centered area, DoCastAOE convention).
-- DORMANT until Spell.dbc ships: the trigger spell id and the 66882
-- aura itself are DBC-side.
local function slimePoolTick(event, aura, target)
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    local mod = math.floor(((aura:GetTickNumber() / 60) * 0.9 + 0.1) * 10000 * 2 / 3)
    target:CastSpellWithRadiusMod(target, triggerSpell, true, mod)
    return true
end

RegisterAuraEvent(SPELL_SLIME_POOL_EFFECT, 1, slimePoolTick)
