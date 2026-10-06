-- Ignis the Furnace Master (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_ignis.cpp
-- (boss_ignis (CreatureScript) via GetUlduarAI<boss_ignis_AI>
-- (BossAI), BOSS_IGNIS = 1 — registered from inside
-- AddSC_boss_ignis(); loader decl 112 / call 307 per
-- northrend_script_loader.cpp — the THIRD group of the "// Ulduar"
-- block in AddNorthrendScripts(), immediately after
-- AddSC_boss_flame_leviathan() (decl 111 / call 306); the call
-- after it is AddSC_boss_razorscale() (decl 113 / call 308) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_flame_leviathan -> boss_ignis) is followed).
-- Entry: 33118 Ignis (ulduar.h NPC_IGNIS, line 62 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_ignis"
-- hits boss_ignis.cpp (+ the loader decl/call lines for
-- AddSC_boss_ignis) only; whole-tree grep for
-- "npc_iron_construct", "npc_scorch_ground",
-- "spell_ignis_slag_pot" and "achievement_ignis_shattered"
-- hits boss_ignis.cpp only; zero sql/ hits for all five —
-- the audited artifact this run. No second ignis lua exists.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the six ScheduleEvent
-- legs and the DoStartTimedAchievement leg have no bridges —
-- boss_bookkeeping precedent for the passthrough, no timer /
-- timed-achievement bridge for the rest — the auriaya
-- engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 4) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 6) (event 4; the _JustDied()
-- passthrough has no bridge — the sjonnir JustDied-Talk
-- precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33118):
-- Reset / Initialize — _Reset() + vehicle passenger removal +
-- DoStopTimedAchievement — no bridges.
-- GetData(DATA_SHATTERED 29252926) — no GetData bridge
-- (consumer: achievement_ignis_shattered below).
-- JustSummoned — NPC_IRON_CONSTRUCT faction/react-state/flag/
-- immune/root legs + AttackStart/DoZoneInCombat + summons.Summon
-- — no faction / react-state / summon bridges.
-- DoAction(ACTION_REMOVE_BUFF 20) — RemoveAuraFromStack(SPELL_
-- STRENGHT 64473) + the GameTime <5s shattered window (producer:
-- npc_iron_construct DamageTaken) — no DoAction / aura-stack
-- bridges.
-- UpdateAI event machine — EVENT_JET Talk(EMOTE_JETS 7) +
-- DoCast(SPELL_FLAME_JETS 62680); EVENT_SLAG_POT Talk(SAY_
-- SLAG_POT 2) + random-target SelectTarget + DoCast(SPELL_GRAB
-- 62707) + DelayEvents(3s) -> EVENT_GRAB_POT (EnterVehicle) ->
-- EVENT_CHANGE_POT (SPELL_SLAG_POT 62717 + seat swap) ->
-- EVENT_END_POT (ExitVehicle); EVENT_SCORCH Talk(SAY_SCORCH 3)
-- + SummonCreature(NPC_GROUND_SCORCH 33221) + DoCast(SPELL_
-- SCORCH 62546); EVENT_CONSTRUCT Talk(SAY_SUMMON 1) + DoSummon
-- (NPC_IRON_CONSTRUCT) + SPELL_STRENGHT / SPELL_ACTIVATE_
-- CONSTRUCT; EVENT_BERSERK DoCast(SPELL_BERSERK 47008) +
-- Talk(SAY_BERSERK 5) — no timer-event / cast / summon /
-- target-selection / vehicle bridges; all timer-leg yells ride
-- the unbridgeable event machine.
-- npc_iron_construct (NPC_IRON_CONSTRUCT 33121 local enum —
-- entry-verifiable but bridge-blocked; the npc_spark_of_ionar
-- no-bridgeable-arms precedent — no registration): DamageTaken
-- shatter leg (HasAura(SPELL_BRITTLE 62382 / 67114) && damage
-- >= 5000 -> SPELL_SHATTER 62383 + DoAction(ACTION_REMOVE_BUFF)
-- to Ignis + DespawnOrUnsummon) — no damage-taken / DoAction
-- bridges; UpdateAI HEAT-stack / MOLTEN / BRITTLE / IsInWater
-- machine — no aura-stack / water-state bridges; no Talk arms
-- anywhere.
-- npc_scorch_ground (NPC_GROUND_SCORCH 33221 local enum —
-- entry-verifiable but bridge-blocked — no registration):
-- MoveInLineOfSight heat tracking + UpdateAI AddAura(SPELL_
-- HEAT 65667) 1s machine — no proximity / aura bridges; no
-- Talk arms anywhere.
-- spell_ignis_slag_pot (62717) — periodic-dummy AuraScript
-- (SPELL_SLAG_POT_DAMAGE 65722 on tick; OnRemove casts SPELL_
-- SLAG_IMBUED 62836 when target alive) — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue.
-- achievement_ignis_shattered — OnCheck: target AI GetData
-- (DATA_SHATTERED) != 0 — no GetData / achievement bridges;
-- joins the unmodeled-achievement queue.

local ENTRY_IGNIS = 33118

local SAY_AGGRO = 0
local SAY_SLAY = 4
local SAY_DEATH = 6

-- C++ JustEngagedWith: BossAI passthrough + ScheduleEvent legs
-- + DoStartTimedAchievement — only the Talk arm is bridgeable.
local function ignisEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function ignisTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + Talk(SAY_DEATH) —
-- only the Talk arm is bridgeable.
local function ignisDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_IGNIS, 1, ignisEnterCombat)
RegisterCreatureEvent(ENTRY_IGNIS, 3, ignisTargetDied)
RegisterCreatureEvent(ENTRY_IGNIS, 4, ignisDied)
