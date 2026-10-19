-- XT-002 Deconstructor (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_xt002.cpp
-- (boss_xt002 (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_xt002> (BossAI), BOSS_XT002 = 3
-- (ulduar.h:39); registered from inside AddSC_boss_xt002() — the
-- FIFTH group of the "// Ulduar" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_razorscale(); the call after it is
-- AddSC_boss_general_vezax() — loader order confirmed).
-- Entry: 33293 XT-002 (ulduar.h NPC_XT002, line 65 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Talk ids (XT002Yells, boss_xt002.cpp:117-131, verified this
-- run): SAY_AGGRO 0, SAY_HEART_OPENED 1, SAY_HEART_CLOSED 2,
-- SAY_TYMPANIC_TANTRUM 3, SAY_SLAY 4, SAY_BERSERK 5, SAY_DEATH 6,
-- SAY_SUMMON 7, EMOTE_HEART_OPENED 8, EMOTE_HEART_CLOSED 9,
-- EMOTE_TYMPANIC_TANTRUM 10, EMOTE_SCRAPBOT 11.
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
-- AddSC_boss_xt002) only; zero sql/ hits for all nineteen —
-- the audited artifact this run. No second xt002 lua exists.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timer-driven casts ride the
-- CreateLuaEvent convention (baltharus / maiden / auriaya
-- precedent): module-local GUID-keyed timers table, cancelTimers
-- on events 2/4/23, schedule(guid, key, delayMs, fn);
-- creature:CastSpell(nil, spell) = DoCastVictim (2-arg,
-- non-triggered), creature:CastSpell(creature, spell) =
-- DoCastSelf / DoCastAOE, third arg true = triggered,
-- {minMs, maxMs} = randtime re-arm. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_xt002.cpp):
-- JustEngagedWith (:186-197) — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the phase-gated
-- ScheduleEvent legs and the DoStartTimedAchievement leg have no
-- bridges — the auriaya engage-port precedent).
-- EVENT_ENRAGE (:362-365) — Talk(SAY_BERSERK 5) + DoCastSelf
-- (SPELL_ENRAGE 26662) 2-arg non-triggered, 600s init (:192), no
-- re-arm, no phase gate (scheduled bare in JustEngagedWith —
-- ExposeHeart never cancels it) — Talk-before-cast order kept.
-- npc_life_spark (entry 34004 — upstream 3.3.5 Life Spark; the
-- entry also exists in this repo's world_trainer.sql; no entry
-- constant in ulduar.h/boss_xt002.cpp — ScriptName binding is
-- DB-side, same precedent as the 33293 registration above):
-- Reset (:624-628) — DoCastSelf(SPELL_ARCANE_POWER_STATE
-- 49411) 2-arg non-triggered + scheduler wipe (event 23).
-- JustEngagedWith (:630-638) — DoCastSelf
-- (SPELL_STATIC_CHARGED 64227) 2-arg non-triggered at engage
-- (event 1) + the 12s repeating _scheduler leg casting
-- DoCastVictim(SPELL_SHOCK 64230) 2-arg non-triggered
-- (creature:CastSpell(nil, spell) = DoCastVictim per the file
-- convention); wiped on events 2/4/23 like the scheduler.
-- KilledUnit (:204-208) — player-gated Talk(SAY_SLAY 4) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied (:210-215) — Talk(SAY_DEATH 6) (event 4; the
-- _JustDied() passthrough + RemoveFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) legs have no bridges — the sjonnir
-- JustDied-Talk precedent); also wipes the enrage timer.
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
-- The heart-exposure phase machine — ExposeHeart
-- (events.SetPhase(PHASE_HEART) + react-state / AttackStop +
-- Talk(SAY_HEART_OPENED 1) + EVENT_SUBMERGE scheduling + heart
-- DoAction(ACTION_START_PHASE_HEART) via instance GUID — no
-- phase / DoAction / instance bridges); DisposeHeart
-- (Talk(SAY_HEART_CLOSED 2) + Talk(EMOTE_HEART_CLOSED 9) +
-- hard-mode react-state + RescheduleEvents + SPELL_STAND /
-- SPELL_COOLDOWN_CREATURE_SPECIAL_2 + heart DoAction
-- (ACTION_DISPOSE_HEART)); RescheduleEvents re-arms the PHASE_1
-- timers at different delays — the whole PHASE_1/PHASE_HEART
-- machine stays out (the gothik phase-gating precedent).
-- EVENT_SEARING_LIGHT (:332-335) — DoCastSelf
-- (SPELL_SEARING_LIGHT 63018), 9s/11s init (:190), re-arm
-- {16s,20s}, PHASE_1-gated + GROUP_SEARING_GRAVITY — porting on
-- a continuous timer would fabricate casts during the 30s
-- PHASE_HEART exposures (gothik precedent); phase machine —
-- left out.
-- EVENT_GRAVITY_BOMB (:336-339) — DoCastSelf
-- (SPELL_GRAVITY_BOMB 63024), 18s/21s init (:191), re-arm
-- {16s,20s}, PHASE_1-gated — same blocker; left out.
-- EVENT_TYMPANIC_TANTRUM (:340-346) — Talk(SAY_TYMPANIC_TANTRUM
-- 3) + Talk(EMOTE_TYMPANIC_TANTRUM 10) + events.DelayEvents(10s,
-- GROUP_SEARING_GRAVITY) + DoCastSelf(SPELL_TYMPANIC_TANTRUM
-- 62776), 60s init (:193), re-arm 1min, PHASE_1-gated and
-- canceled by ExposeHeart — phase machine; left out.
-- EVENT_PHASE_CHECK (:347-351) — HealthBelowPct leg ->
-- ExposeHeart, 1s repeat — phase machine; left out.
-- EVENT_SUBMERGE (:352-358) — DoCastSelf(SPELL_SUBMERGE 37751)
-- + Talk(EMOTE_HEART_OPENED 8) + heart DoAction
-- (ACTION_START_PHASE_HEART) + EVENT_DISPOSE_HEART in 30s —
-- phase / DoAction / instance machine; left out.
-- EVENT_DISPOSE_HEART / EVENT_ENTER_HARD_MODE (SetFullHealth +
-- DoCastSelf(SPELL_HEARTBREAK 65737, TRIGGERED) +
-- AddLootMode(LOOT_MODE_HARD_MODE_1) + DisposeHeart(true)) /
-- EVENT_RESUME_ATTACK — hard-mode / phase / DoAction machine;
-- left out.
-- PassengerBoarded (Talk(EMOTE_SCRAPBOT 11) on scrapbot entry —
-- no vehicle / PassengerBoarded bridge).
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
-- 62834 — the damage=0 rewrite is bridgeable (event 9 numeric return,
-- moroes/hodir precedent) but the add never spawns in Go
-- (spell_xt002_energy_orb 62826 — no SpellScript bridge) and Reset
-- carries unbridged legs (SetFloatValue min/max-damage hack,
-- SetReactState, instance gate, MoveFollow, DoAction, despawn) —
-- no registration);
-- npc_xt_void_zone (PassiveAI — no Talk arms; SPELL_CONSUMPTION
-- 64208 periodic scheduler leg + SpellInfo BP0 machine — no
-- bridges — no registration).
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
local ENTRY_LIFE_SPARK = 34004

local SAY_AGGRO = 0
local SAY_SLAY = 4
local SAY_BERSERK = 5
local SAY_DEATH = 6

local SPELL_ENRAGE = 26662
local SPELL_ARCANE_POWER_STATE = 49411
local SPELL_STATIC_CHARGED = 64227
local SPELL_SHOCK = 64230

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

-- C++ EVENT_ENRAGE (:362-365): Talk(SAY_BERSERK 5) + DoCastSelf
-- (SPELL_ENRAGE 26662) (2-arg non-triggered); 600s init (:192),
-- no re-arm, no phase gate — Talk-before-cast order kept.
local function enrageTick(creature)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_ENRAGE)
end

-- C++ JustEngagedWith (:186-197): BossAI passthrough + Talk(AGGRO)
-- + the phase-gated ScheduleEvent legs + DoStartTimedAchievement —
-- only the Talk arm and the phase-free EVENT_ENRAGE timer leg are
-- bridgeable (event 1).
local function xt002EnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "enrage", 600000, function()
        enrageTick(creature)
    end)
end

-- C++ KilledUnit (:204-208): who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function xt002TargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:210-215): _JustDied() passthrough + Talk
-- (SAY_DEATH) + RemoveFlag NOT_SELECTABLE — only the Talk arm is
-- bridgeable; the enrage timer is wiped with it.
local function xt002Died(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

-- Shared timer wipe for events 2/23.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_XT002, 1, xt002EnterCombat)
RegisterCreatureEvent(ENTRY_XT002, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_XT002, 3, xt002TargetDied)
RegisterCreatureEvent(ENTRY_XT002, 4, xt002Died)
RegisterCreatureEvent(ENTRY_XT002, 23, cancelCombatTimers)

-- C++ npc_life_spark::JustEngagedWith (:630-638): DoCastSelf
-- (SPELL_STATIC_CHARGED 64227) (2-arg non-triggered) at engage,
-- then the 12s repeating _scheduler leg casting DoCastVictim
-- (SPELL_SHOCK 64230) (2-arg non-triggered; creature:CastSpell
-- (nil, spell) = DoCastVictim per the file convention).
local function lifeSparkShockTick(creature)
    creature:CastSpell(nil, SPELL_SHOCK)
    local guid = creature:GetGUID()
    schedule(guid, "shock", 12000, function()
        lifeSparkShockTick(creature)
    end)
end

local function lifeSparkEnterCombat(event, creature, target)
    creature:CastSpell(creature, SPELL_STATIC_CHARGED)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shock", 12000, function()
        lifeSparkShockTick(creature)
    end)
end

-- C++ npc_life_spark::Reset (:624-628): DoCastSelf
-- (SPELL_ARCANE_POWER_STATE 49411) (2-arg non-triggered) +
-- scheduler wipe.
local function lifeSparkReset(event, creature)
    creature:CastSpell(creature, SPELL_ARCANE_POWER_STATE)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_LIFE_SPARK, 1, lifeSparkEnterCombat)
RegisterCreatureEvent(ENTRY_LIFE_SPARK, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_LIFE_SPARK, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_LIFE_SPARK, 23, lifeSparkReset)
