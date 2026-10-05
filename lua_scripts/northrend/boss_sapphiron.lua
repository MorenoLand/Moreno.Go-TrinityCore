-- Sapphiron (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_sapphiron.cpp
-- (boss_sapphiron (CreatureScript; boss_sapphironAI BossAI, :123);
-- npc_sapphiron_blizzard (ScriptedAI, :429 — registered via
-- RegisterNaxxramasCreatureAI); go_sapphiron_birth (GameObjectScript,
-- :464; go_sapphiron_birthAI GameObjectAI, :469);
-- spell_sapphiron_change_blizzard_target / spell_sapphiron_icebolt
-- (SpellScriptLoader + AuraScript, :501 / :541);
-- spell_sapphiron_summon_blizzard (SpellScriptLoader + SpellScript,
-- :591); achievement_the_hundred_club (AchievementCriteriaScript,
-- :637); AddSC_boss_sapphiron registers all seven — owner .cpp is
-- 657 lines; loader decl 68 / call 263 per
-- northrend_script_loader.cpp — the TENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gluth()
-- (decl 67 / call 262), under the "// Naxxramas" marker; the call
-- after it is AddSC_boss_four_horsemen()).
-- Entry: 15989 Sapphiron (naxxramas.h NPC_SAPPHIRON :102 — kalecgos
-- pass; BOSS_SAPPHIRON :43, DATA_SAPPHIRON :76;
-- instance_naxxramas.cpp binds NPC_SAPPHIRON -> SapphironGUID
-- (:172) and DATA_SAPPHIRON -> SapphironGUID (:326); the
-- CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for each of the
-- seven script names hits boss_sapphiron.cpp only (loader carries
-- only the decl/call lines); zero sql/ hits. No sapphiron lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in
-- Go (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim (moroes precedent),
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent); the third CastSpell arg is the
-- explicit CastSpell(..., true) triggered flag (moroes shadowform
-- precedent). C++ randtime -> math.random (amanitar /
-- boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — CastSpell(me, SPELL_FROST_AURA 28531, triggered)
-- self-cast on combat start; the events.SetPhase(PHASE_GROUND) leg
-- has no phase bridge — documented, not wired
-- (BossAI::JustEngagedWith instance-bookkeeping leg has no bridge —
-- tharon_ja precedent).
-- EVENT_CHECK_RESISTS — DoCastSelf(Check Resists 60539), 0s init
-- (fired on combat start), 30s repeat; the SpellHitTarget resist
-- check and GetData(DATA_THE_HUNDRED_CLUB) legs have no bridges —
-- documented, not wired.
-- EVENT_CLEAVE — DoCastVictim(Cleave 19983), randtime(5s,15s) init,
-- randtime(5s,15s) repeat (moroes precedent); the PHASE_GROUND
-- gating only affects the unmodeled flight legs — scheduled ungated
-- (anubrekhan locust precedent).
-- EVENT_BERSERK — Talk(EMOTE_ENRAGE 3) + DoCastSelf(Berserk 26662),
-- 15min init, fires ONCE (C++ schedules it once, never rescheduled
-- — anubarak one-shot precedent).
-- JustDied — _JustDied() instance bookkeeping has no bridge
-- (tharon_ja precedent); CastSpell(me, SPELL_DIES 29357, triggered)
-- self-cast death visual ported as the event-4 arm.
-- Talk() sites: 4 total in C++ (zero spaced `Talk (` variant) —
-- ALL FOUR accounted: ported (1) — EVENT_BERSERK EMOTE_ENRAGE (3);
-- documented no-bridge orphaned (3) — EVENT_LIFTOFF
-- Talk(EMOTE_AIR_PHASE 0) rides the unbridged flight machine,
-- EVENT_BREATH Talk(EMOTE_BREATH 2) rides the unbridged
-- DoCastAOE(FROST_MISSILE 30101) leg, EVENT_LAND
-- Talk(EMOTE_GROUND_PHASE 1) rides the unbridged land/hover machine
-- (vortex precedent — not ported orphaned).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- InitializeAI — instance GetBossState(BOSS_SAPPHIRON) / GetData(
-- DATA_HAD_SAPPHIRON_BIRTH) legs + visibility/flag/react-state legs
-- (no instance / flag / react-state bridges).
-- Reset — flight-phase DoRemoveAurasDueToSpellOnPlayers(ICEBOLT)
-- (instance model), SetReactState / hover legs (no react-state /
-- hover bridges) + _Reset bookkeeping.
-- DamageTaken — air-phase death prevention (damage = health-1) — no
-- health/damage bridge (doomwalker precedent).
-- SpellHitTarget — SPELL_CHECK_RESISTS resist comparison — no
-- SpellHitTarget hook bridge (terestian precedent) + no resistance
-- bridge.
-- MovementInform (MovePoint 1 -> liftoff) — no motion bridge.
-- DoAction(ACTION_BIRTH) — cross-AI from go_sapphiron_birth — no
-- instance/cross-AI bridge.
-- GetGUID(DATA_BLIZZARD_TARGET) — summons-list filtering +
-- BlizzardTargetSelector random target — no summon-list /
-- random-target bridges (cairne/kazzak precedent).
-- EnterPhaseGround — phase-scheduler machinery for the unbridged
-- legs below (no phase bridge).
-- EVENT_TAIL — DoCastAOE(Tail Sweep 55697) — no DoCastAOE bridge
-- (terestian/shazzrah precedent).
-- EVENT_DRAIN — DoCastAOE(Life Drain 28542) — same bar.
-- EVENT_BLIZZARD — DoCastAOE(Summon Blizzard 28560) + RAID_MODE(
-- 20s, 7s) repeat — same bar + no difficulty bridge (kelidan
-- precedent).
-- EVENT_FLIGHT — HealthAbovePct(10) + REACT_PASSIVE + AttackStop +
-- MovePoint home — no health-pct / motion / react bridges.
-- EVENT_LIFTOFF — DoSummon(NPC_WING_BUFFET 17025) + hover/emote
-- legs + SelectTargetList random icebolt targets — no summon /
-- hover / random-target bridges.
-- EVENT_ICEBOLT — DoCast(target, Icebolt 28522) over the stored
-- GUID vector — the target selection is unbridged, so the leg is
-- not ported (cairne/kazzak precedent).
-- EVENT_BREATH — DoCastAOE(Frost Missile 30101, visual) — no
-- DoCastAOE bridge.
-- EVENT_EXPLOSION — DoCastAOE(Frost Breath 28524 +
-- Frost Breath Anti-Cheat 29318) + instance
-- DoRemoveAurasDueToSpellOnPlayers(ICEBOLT) — no DoCastAOE /
-- instance-aura bridges.
-- EVENT_LAND — buffet DespawnOrUnsummon (cross-AI, no bridge) +
-- hover/emote/phase legs (no hover / emote / phase bridges).
-- EVENT_BIRTH — visible/flag/react legs (no bridges).
-- npc_sapphiron_blizzard (NPC_BLIZZARD 16474, C++-exact from the
-- owner enum; ScriptedAI — no RegisterLuaBoss surface, razuvious
-- precedent): Reset REACT_PASSIVE + TaskScheduler DoCastSelf(
-- m_spells[0]) — port-pattern-ready in isolation (moroes precedent)
-- but bridge-blocked — joins the bridgeable-but-entry-blocked
-- queue; SetGUID/GetGUID(DATA_BLIZZARD_TARGET) cross-AI target
-- machinery — no cross-AI bridge.
-- NPC_WING_BUFFET 17025 is C++-exact from the owner enum
-- (entry-verifiable but summon STRAND absent — bridge-blocked
-- queue).
-- go_sapphiron_birth (GameObjectScript — no GO bridge): GO_ACTIVATED
-- -> cross-AI DoAction(ACTION_BIRTH) via instance GetGuidData(
-- DATA_SAPPHIRON) + SetData(DATA_HAD_SAPPHIRON_BIRTH, 1) — joins
-- the instance/GO queue.
-- spell_sapphiron_change_blizzard_target (AuraScript) /
-- spell_sapphiron_icebolt (AuraScript) /
-- spell_sapphiron_summon_blizzard (SpellScript) — no AuraScript /
-- SpellScript binding bridge (razelikh precedent) — join the
-- SpellScript/AuraScript queue.
-- achievement_the_hundred_club (DATA_THE_HUNDRED_CLUB 21462147 /
-- MAX_FROST_RESISTANCE 100) — no achievement-criteria bridge
-- (tharon_ja precedent) — joins that queue.
-- GO_ICEBLOCK 181247 rides the unported icebolt AuraScript.

local ENTRY_SAPPHIRON = 15989

local EMOTE_ENRAGE = 3

local SPELL_FROST_AURA = 28531
local SPELL_CLEAVE = 19983
local SPELL_BERSERK = 26662
local SPELL_DIES = 29357
local SPELL_CHECK_RESISTS = 60539

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

-- C++ EVENT_CHECK_RESISTS: DoCastSelf(Check Resists 60539), 0s init,
-- 30s repeat.
local function resistsTick(creature, guid)
    creature:CastSpell(creature, SPELL_CHECK_RESISTS)
    schedule(guid, "resists", 30000, function()
        resistsTick(creature, guid)
    end)
end

-- C++ EVENT_CLEAVE: DoCastVictim(Cleave 19983), randtime(5s,15s)
-- init, randtime(5s,15s) repeat.
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(5, 15) * 1000, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: Talk(EMOTE_ENRAGE 3) + DoCastSelf(Berserk
-- 26662), 15min init, fires ONCE (never rescheduled).
local function berserkTick(creature, guid)
    creature:Talk(EMOTE_ENRAGE)
    creature:CastSpell(creature, SPELL_BERSERK)
    local per = timers[guid]
    if per then
        per["berserk"] = nil
    end
end

-- C++ JustEngagedWith: CastSpell(me, SPELL_FROST_AURA 28531,
-- triggered) self-cast + schedules EVENT_CHECK_RESISTS (0s),
-- EVENT_CLEAVE (randtime 5s,15s), EVENT_BERSERK (15min). The
-- SetPhase(PHASE_GROUND) leg and the EnterPhaseGround unbridged-leg
-- schedules (TAIL / DRAIN / BLIZZARD / FLIGHT) have no bridges —
-- documented, not wired. No aggro Talk in C++.
local function sapphironEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_FROST_AURA, true)
    resistsTick(creature, guid)
    schedule(guid, "cleave", math.random(5, 15) * 1000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "berserk", 900000, function()
        berserkTick(creature, guid)
    end)
end

local function sapphironLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: _JustDied() bookkeeping (no bridge) + CastSpell(
-- me, SPELL_DIES 29357, triggered) self-cast death visual.
local function sapphironDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_DIES, true)
end

local function sapphironReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SAPPHIRON, 1, sapphironEnterCombat)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 2, sapphironLeaveCombat)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 4, sapphironDied)
RegisterCreatureEvent(ENTRY_SAPPHIRON, 23, sapphironReset)
