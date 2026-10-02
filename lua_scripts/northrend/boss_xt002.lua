-- XT-002 Deconstructor (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_xt002.cpp
-- (boss_xt002 (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_xt002> (BossAI), BOSS_XT002 = 3 —
-- registered from inside AddSC_boss_xt002(); loader decl 114 /
-- call 309 per northrend_script_loader.cpp — the FIFTH group of
-- the "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_razorscale() (decl 113 / call 308); the call
-- after it is AddSC_boss_general_vezax() — loader order
-- confirmed this run; the checkpoint sequence
-- (boss_razorscale -> boss_xt002) is followed).
-- Entry: 33293 XT-002 (ulduar.h NPC_XT002, line 65 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- nineteen script names ("boss_xt002", "npc_xt002_heart",
-- "npc_scrapbot", "npc_pummeller", "npc_boombot",
-- "npc_life_spark", "npc_xt_void_zone",
-- "spell_xt002_searing_light_spawn_life_spark",
-- "spell_xt002_gravity_bomb_aura",
-- "spell_xt002_gravity_bomb_damage",
-- "spell_xt002_heart_overload_periodic",
-- "spell_xt002_energy_orb", "spell_xt002_tympanic_tantrum",
-- "spell_xt002_submerged", "spell_xt002_321_boombot_aura",
-- "spell_xt002_exposed_heart", "achievement_nerf_engineering",
-- "achievement_heartbreaker", "achievement_nerf_gravity_bombs")
-- hits boss_xt002.cpp (+ the loader decl/call lines for
-- AddSC_boss_xt002) only; zero sql/ hits for all nineteen. No
-- xt002 lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the five ScheduleEvent
-- legs and the DoStartTimedAchievement leg have no bridges —
-- the auriaya engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 4) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — the twenty-fourth
-- player-gated variant ported).
-- JustDied — Talk(SAY_DEATH 6) (event 4; the _JustDied()
-- passthrough + RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) legs have no bridges — the sjonnir
-- JustDied-Talk precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33293):
-- Reset / Initialize / EnterEvadeMode — _Reset() +
-- events.SetPhase(PHASE_1) + SetReactState(REACT_DEFENSIVE) +
-- DoStopTimedAchievement(MUST_DECONSTRUCT_FASTER 21027) +
-- summons.DespawnAll — no bridges.
-- DoAction(ACTION_ENTER_HARD_MODE — no DoAction bridge).
-- GetData(DATA_HARD_MODE 1 / DATA_HEALTH_RECOVERED 2 /
-- DATA_GRAVITY_BOMB_CASUALTY 3 — no GetData bridge; consumers:
-- achievement_heartbreaker / achievement_nerf_engineering /
-- achievement_nerf_gravity_bombs below); SetData
-- (DATA_TRANSFERED_HEALTH health-transfer + ModifyHealth leg;
-- DATA_GRAVITY_BOMB_CASUALTY — no SetData bridge).
-- ExposeHeart (events.SetPhase(PHASE_HEART) + react-state /
-- AttackStop + Talk(SAY_HEART_OPENED 1) + EVENT_SUBMERGE
-- scheduling — phase / react-state / scheduler bridges
-- missing); DisposeHeart (Talk(SAY_HEART_CLOSED 2) +
-- Talk(EMOTE_HEART_CLOSED 9) + hard-mode react-state +
-- RescheduleEvents + SPELL_STAND / SPELL_COOLDOWN_CREATURE_
-- SPECIAL_2 + heart DoAction(ACTION_DISPOSE_HEART) — no
-- DoAction / cast bridges).
-- PassengerBoarded (Talk(EMOTE_SCRAPBOT 11) on scrapbot entry —
-- no vehicle / PassengerBoarded bridge).
-- UpdateAI event machine — EVENT_SEARING_LIGHT
-- (DoCastSelf 63018); EVENT_GRAVITY_BOMB (DoCastSelf 63024);
-- EVENT_TYMPANIC_TANTRUM (Talk(SAY_TYMPANIC_TANTRUM 3) +
-- Talk(EMOTE_TYMPANIC_TANTRUM 10) + DoCastSelf 62776 +
-- DelayEvents); EVENT_PHASE_CHECK (HealthBelowPct leg ->
-- ExposeHeart); EVENT_SUBMERGE (DoCastSelf SPELL_SUBMERGE
-- 37751 + Talk(EMOTE_HEART_OPENED 8) + heart DoAction
-- (ACTION_START_PHASE_HEART)); EVENT_DISPOSE_HEART ->
-- DisposeHeart; EVENT_ENRAGE (Talk(SAY_BERSERK 5) + DoCastSelf
-- SPELL_ENRAGE 26662); EVENT_ENTER_HARD_MODE (SetFullHealth +
-- SPELL_HEARTBREAK 65737 + AddLootMode hard mode + _hardMode);
-- EVENT_RESUME_ATTACK — no timer-event / cast / phase /
-- react-state bridges; all timer-leg yells ride the
-- unbridgeable event machine.
-- npc_xt002_heart (NullCreatureAI — no Talk arms anywhere;
-- DoAction(ACTION_START_PHASE_HEART / ACTION_DISPOSE_HEART)
-- vehicle / cast legs; JustDied -> xt002 DoAction
-- (ACTION_ENTER_HARD_MODE) — no DoAction bridge — no
-- registration).
-- npc_scrapbot (no Talk arms; TaskScheduler MoveFollow /
-- SPELL_SCRAPBOT_RIDE_VEHICLE / SPELL_SCRAP_REPAIR machinery —
-- no scheduler / movement / vehicle bridges — no registration);
-- npc_pummeller (no Talk arms; TaskScheduler trample / arcing
-- smash / uppercut legs — no bridges — no registration);
-- npc_boombot (no Talk arms; DamageTaken boom leg + SPELL_BOOM
-- 62834 — no damage-taken bridge — no registration);
-- npc_life_spark (no Talk arms; SPELL_SHOCK 64230 scheduler
-- leg — no bridges — no registration);
-- npc_xt_void_zone (PassiveAI — no Talk arms; SPELL_CONSUMPTION
-- 64208 periodic scheduler leg — no bridges — no registration).
-- spell_xt002_searing_light_spawn_life_spark (63018/65121
-- periodic-trigger AuraScript OnRemove — no AuraScript
-- bridge; joins the no-AuraScript-bridge queue);
-- spell_xt002_gravity_bomb_aura (63024/64234 — OnPeriodic
-- SetData(DATA_GRAVITY_BOMB_CASUALTY) leg + OnRemove hard-mode
-- void-zone summon — no AuraScript bridge; joins the
-- no-AuraScript-bridge queue);
-- spell_xt002_gravity_bomb_damage (63025/64233 — lethal-hit
-- SetData(DATA_GRAVITY_BOMB_CASUALTY) leg — no SpellScript
-- bridge; joins the no-SpellScript-bridge queue);
-- spell_xt002_heart_overload_periodic (62791 serverside
-- dummy — random-toy-pile ENERGY_ORB / HEART_LIGHTNING_TETHER
-- legs — no SpellScript bridge; joins the
-- no-SpellScript-bridge queue); spell_xt002_energy_orb
-- (62826 — Talk(SAY_SUMMON 7) on the vehicle base + boombot /
-- pummeller / scrapbot RECHARGE chain — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue);
-- spell_xt002_tympanic_tantrum (62775 — target filter + damage
-- recalc — no SpellScript bridge; joins the
-- no-SpellScript-bridge queue); spell_xt002_submerged (37751
-- script-effect NOT_SELECTABLE / submerged stand state — no
-- SpellScript bridge; joins the no-SpellScript-bridge queue);
-- spell_xt002_321_boombot_aura (65032 — proc CheckProc on
-- scrapbot entry + achievement-credit cast — no AuraScript
-- bridge; joins the no-AuraScript-bridge queue);
-- spell_xt002_exposed_heart (63849 — proc damage tally ->
-- SetData(DATA_TRANSFERED_HEALTH) on remove — no AuraScript
-- bridge; joins the no-AuraScript-bridge queue).
-- achievement_nerf_engineering (OnCheck: !GetData
-- (DATA_HEALTH_RECOVERED)); achievement_heartbreaker (OnCheck:
-- GetData(DATA_HARD_MODE) != 0); achievement_nerf_gravity_bombs
-- (OnCheck: !GetData(DATA_GRAVITY_BOMB_CASUALTY)) — no GetData
-- / achievement bridges; all three join the
-- unmodeled-achievement queue.

local ENTRY_XT002 = 33293

local SAY_AGGRO = 0
local SAY_SLAY = 4
local SAY_DEATH = 6

-- C++ JustEngagedWith: BossAI passthrough + ScheduleEvent legs
-- + DoStartTimedAchievement — only the Talk arm is bridgeable.
local function xt002EnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function xt002TargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + Talk(SAY_DEATH) —
-- only the Talk arm is bridgeable.
local function xt002Died(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_XT002, 1, xt002EnterCombat)
RegisterCreatureEvent(ENTRY_XT002, 3, xt002TargetDied)
RegisterCreatureEvent(ENTRY_XT002, 4, xt002Died)
