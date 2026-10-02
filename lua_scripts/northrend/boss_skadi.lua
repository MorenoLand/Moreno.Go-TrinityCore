-- Skadi the Ruthless (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_skadi.cpp
-- (993 lines; 15 scripts — boss_skadi (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_SKADI_THE_RUTHLESS = 2);
-- npc_grauf (CreatureScript via GetUtgardePinnacleAI (ScriptedAI),
-- DATA_GRAUF = 13); npc_ymirjar_warrior / npc_ymirjar_witch_doctor /
-- npc_ymirjar_harpooner (CreatureScript via GetUtgardePinnacleAI
-- (npc_skadi_trashAI : ScriptedAI)); spell_freezing_cloud_area_left /
-- spell_freezing_cloud_area_right / spell_freezing_cloud_damage /
-- spell_skadi_reset_check / spell_skadi_launch_harpoon /
-- spell_skadi_poisoned_spear (SpellScriptLoader); spell_skadi_ride_vehicle
-- (bare AuraScript via RegisterSpellScript);
-- spell_summon_gauntlet_mobs_periodic (SpellScriptLoader, AuraScript);
-- achievement_girl_love_to_skadi (AchievementCriteriaScript, OnCheck
-- GetData DATA_LOVE_TO_SKADI); at_skadi_gaunlet (AreaTriggerScript,
-- OnTrigger -> DoAction(ACTION_START_ENCOUNTER)); all registered from
-- inside AddSC_boss_skadi(); loader decl 134 / call 329 per
-- northrend_script_loader.cpp — the THIRD group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_palehoof() (call 328); the checkpoint sequence
-- (boss_palehoof -> boss_skadi) is followed).
-- Entries: 26693 Skadi the Ruthless (utgarde_pinnacle.h
-- NPC_SKADI_THE_RUTHLESS, line 55) — entry-verifiable, registration
-- proceeds (the nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is DB-side as usual.
-- DATA_SKADI_THE_RUTHLESS = 2 (utgarde_pinnacle.h line 33).
-- Sole-source verified: whole-server-tree grep for "boss_skadi" /
-- "npc_grauf" / "at_skadi_gaunlet" hits boss_skadi.cpp only (+ the loader
-- decl/call lines); this clone carries no sql/ tree, so ScriptName
-- bindings are DB-side by construction. No skadi lua existed.
-- Eluna creature events: 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- KilledUnit — Talk(SAY_KILL 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 3) unconditional (event 4 — the sjonnir
-- JustDied-Talk precedent; the _JustDied() passthrough leg has no
-- bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 26693):
-- Talk(SAY_AGGRO 0) rides DoAction(ACTION_START_ENCOUNTER) (area-trigger
-- DoAction machine), not JustEngagedWith — event 1 deliberately NOT
-- registered (registering it would deviate from C++: ground-phase
-- combat entry yells nothing there); joins the no-DoAction-bridge queue.
-- Talk(SAY_DRAKE_BREATH 6) (ACTION_DRAKE_BREATH) and
-- Talk(SAY_DRAKE_DEATH 5) (ACTION_GAUNTLET_END) ride the DoAction /
-- cross-creature machine (instance GetCreature(DATA_SKADI_THE_RUTHLESS),
-- passenger-board handoff, harpoon-hit counter ACTION_HARPOON_HIT,
-- _loveSkadi / _harpoonHit state, the flying-phase scheduler legs) — no
-- DoAction / cross-creature / timer-event bridges; join the respective
-- bridge queues.
-- npc_grauf (26893, utgarde_pinnacle.h line 70) — Talk(EMOTE_ON_RANGE 0)
-- / Talk(EMOTE_BREATH 0) ride MovementInform SPLINE_CHAIN_MOTION_TYPE +
-- 1ms timer legs; no MovementInform / timer bridges — no registration.
-- The ymirjar trash machines (warrior / witch-doctor / harpooner —
-- hamstring/strike, shadow-bolt/shrink, net/throw spell timers, harpoon
-- summon on death) carry zero Talk arms — no registrations.
-- spell_freezing_cloud_area_left / right, spell_freezing_cloud_damage,
-- spell_skadi_reset_check, spell_skadi_launch_harpoon,
-- spell_skadi_poisoned_spear, spell_skadi_ride_vehicle,
-- spell_summon_gauntlet_mobs_periodic — SpellScript / AuraScript hooks
-- only — join the no-SpellScript / no-AuraScript-bridge queues.
-- achievement_girl_love_to_skadi (OnCheck: GetData(DATA_LOVE_TO_SKADI)
-- == 1) — no GetData / achievement bridges; joins the
-- unmodeled-achievement queue.
-- at_skadi_gaunlet — AreaTriggerScript, no bridge; documented only.

local ENTRY_SKADI_THE_RUTHLESS = 26693

local SAY_KILL = 1
local SAY_DEATH = 3

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL)
-- — the razuvious player-gated variant precedent.
local function skadiKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ JustDied: _JustDied() (passthrough, no bridge);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function skadiJustDied(event, creature)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 3, skadiKilledUnit)
RegisterCreatureEvent(ENTRY_SKADI_THE_RUTHLESS, 4, skadiJustDied)
