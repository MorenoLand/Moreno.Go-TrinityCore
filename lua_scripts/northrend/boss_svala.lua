-- Svala Sorrowgrave (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_svala.cpp
-- (621 lines; 6 scripts — boss_svala (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_SVALA_SORROWGRAVE = 0);
-- npc_ritual_channeler (CreatureScript via GetUtgardePinnacleAI
-- (ScriptedAI)); npc_spectator (CreatureScript via GetUtgardePinnacleAI
-- (ScriptedAI)); spell_paralyze_pinnacle (SpellScriptLoader, SpellScript
-- OnObjectAreaTargetSelect); npc_scourge_hulk (CreatureScript via
-- GetUtgardePinnacleAI (ScriptedAI)); achievement_incredible_hulk
-- (AchievementCriteriaScript OnCheck); all registered from inside
-- AddSC_boss_svala(); loader decl 132 / call 327 per
-- northrend_script_loader.cpp — the FIRST group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_utgarde_keep() (call 325); the checkpoint sequence
-- (utgarde_keep -> boss_svala) is followed).
-- Entries: 26668 Svala Sorrowgrave (utgarde_pinnacle.h
-- NPC_SVALA_SORROWGRAVE, line 53); 26555 Scourge Hulk
-- (boss_svala.cpp enum Creatures — not in utgarde_pinnacle.h —
-- registered this run for its ungated timer arms; entry-verifiable,
-- registration proceeds (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is DB-side
-- as usual). DATA_SVALA_SORROWGRAVE = 0 (utgarde_pinnacle.h line 31);
-- 27281 Ritual Channeler (boss_svala.cpp enum Creatures — not in
-- utgarde_pinnacle.h — registered this run for its heroic Reset aura;
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName binding is DB-side as usual). The intro form NPC_SVALA = 29281 (line
-- 59) UpdateEntry-transforms into 26668; its Talk arms ride the
-- EVENT_INTRO_* timer machine (see DOCUMENTED-ONLY).
-- Sole-source verified: whole-server-tree grep for all six script names
-- hits boss_svala.cpp only (+ the loader decl/call lines for
-- AddSC_boss_svala); the clone's sql/ tree (~/workspace/moreno-trinitycore/sql/,
-- characters/ + world/) carries zero svala hits (empty of bindings). The
-- lua was ported Oct 2 in d2645ec (Talk-only, stale "no-timer-bridge"
-- doctrine) and UPGRADED this run under the CreateLuaEvent doctrine
-- (engine/scripting/runtime.go:798 — the maiden_of_virtue / keleseth /
-- yogg_saron precedent): module-local GUID-keyed timers table, wipe on
-- events 2 (leave combat), 4 (death), 23 (evade). creature:CastSpell(nil,
-- spell) = DoCastVictim; creature:CastSpell(creature, spell) = DoCastSelf
-- / DoCastAOE (the selin_fireheart / mr_smite / boss_ai.go DoCastAOE
-- lowering precedent); the 3rd CastSpell arg is the C++ DoCast 3rd arg
-- (2-arg / 1-arg overloads = non-triggered); {minMs, maxMs} table delay =
-- randtime range. The 2026-10-02 "no-timer-bridge" header doctrine is
-- RETRACTED for this file: CreateLuaEvent is a real registered bridge,
-- so the ungated timer arms below are ported rather than parked.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_svala.cpp):
-- boss_svala JustEngagedWith (:176) — Talk(SAY_AGGRO 2) (event 1;
-- BossAI::JustEngagedWith passthrough unmodeled — the auriaya
-- engage-port precedent).
-- boss_svala KilledUnit (:209-212) — Talk(SAY_SLAY 3) player-gated
-- (event 3 — the razuvious player-gated variant precedent; the
-- nil-guard is the hodir-port convention — Eluna event 3 always
-- carries a victim, so C++-exactness is unaffected).
-- boss_svala JustDied (:217-221) — HandleEmoteCommand(EMOTE_ONESHOT_
-- FLYDEATH 445) is unconditional in C++ (outside the SACRIFICING
-- SetEquipmentSlots gate) and PerformEmote is bridged
-- (lua_creature_events.go:347, Unit::HandleEmoteCommand lowering) —
-- ported; Talk(SAY_DEATH 4) unconditional (event 4 — the sjonnir
-- JustDied-Talk precedent). The SACRIFICING-gated SetEquipmentSlots
-- leg stays out (phase-gated, gothik precedent); the _JustDied()
-- instance passthrough has no bridge.
-- npc_scourge_hulk (26555) — the scourge hulk's two spell-timer arms
-- are ungated in its UpdateAI (no phase groups, no DoAction legs —
-- the yogg_saron adds precedent), scheduled here on event 1 with the
-- maiden CreateLuaEvent convention; its C++ Reset re-seeds both
-- timers, matching combat-start scheduling (the xt002 precedent):
--   EVENT_MIGHTY_BLOW arm (:571-577): DoCast(victim, SPELL_MIGHTY_BLOW
--   48697), 2-arg = non-triggered -> CastSpell(victim, 48697); init
--   urand(4s,9s), re-arm urand(12s,17s).
--   EVENT_VOLATILE_INFECTION arm (:581-586): DoCastVictim
--   (SPELL_VOLATILE_INFECTION 56785), 1-arg = non-triggered ->
--   CastSpell(nil, 56785); init urand(10s,14s), re-arm urand(13s,17s);
--   C++-exact already: H_SPELL_VOLATILE_INFECTION 59228 is a dead
--   enum entry (defined :58, never referenced — the hulk fires
--   56785 unconditionally in both modes, anomalus precedent), so
--   there is no heroic arm to model; the old "no difficulty
--   bridge" rationale was doubly stale (creature:IsHeroic() is
--   bridged, ymiron/vazruden precedent).
-- DOCUMENTED DEVIATION: the C++ victim-stun gate
-- (!victim->HasUnitState(UNIT_STATE_STUNNED), :574-575 — "Prevent
-- knocking back a ritual player") has no bridge — the cast fires
-- unconditionally (the hodir UNIT_STATE_CASTING precedent).
-- npc_ritual_channeler (27281) Reset (:427-432) — the IsHeroic()-gated
-- DoCast(me, SPELL_SHADOWS_IN_THE_DARK 59407), 2-arg = non-triggered ->
-- CastSpell(creature, 59407) (event 23 — the Reset AddAura port
-- category; creature:IsHeroic() is bridged, lua_creature_events.go:260)
-- — the old "reset leg is unmodeled" header claim is STALE and
-- RETRACTED: the gating predicate is bridged and the arm is ungated
-- (no phase machine, no timers). The Initialize() paralyzeTimer re-seed
-- and the paralyze timer itself ride unbridged instance
-- GetGuidData(DATA_SACRIFICED_PLAYER) — stay out; no event-23 fire at
-- first grid spawn, so the spawn-time heroic aura stays unmodeled (the
-- palehoof freeze-visual precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond 26668 /
-- 26555 / 27281):
-- EVENT_SINISTER_STRIKE (:350-353) — DoCastVictim(SPELL_SINSTER_STRIKE
-- 15667), non-triggered, init 7s / re-arm {5s,9s} — and
-- EVENT_CALL_FLAMES (:354-358) — SelectTarget(Random, 0, 100.0f,
-- true) -> DoCast(target, SPELL_CALL_FLAMES 48258), non-triggered,
-- init {10s,20s} / re-arm {10s,20s} — are both scheduled with phase
-- group NORMAL (:228-229 / :347-348) and fire only while IsInPhase
-- (NORMAL); the NORMAL <-> SACRIFICING phase machine (HealthBelowPct
-- (50) latch -> SetPhase(SACRIFICING) + SetCombatMovement(false) +
-- MotionMaster legs, :244-252; SpellHitTarget
-- SPELL_RITUAL_STRIKE_EFF_1 48277 -> SetPhase(NORMAL), :223-233) —
-- the old "no health-pct / SpellHitTarget" rationale is stale
-- (both are bridged: GetHealthPct via the generic object fallback,
-- event-15 SpellHitTarget per boss_ai.go); the blocker is the phase
-- machine (SetPhase / SetCombatMovement / MoveChase), so porting on
-- combat-start
-- timers would fabricate SACRIFICING-phase casts (the gothik
-- phase-gating precedent; the AGENTS.md 2026-10-13 DoCastAOE-vs-
-- phase-gating counterpoint). Stay out.
-- EVENT_RITUAL_PREPARATION (:359-369) — SACRIFICING-gated; Talk
-- (SAY_SACRIFICE_PLAYER 5) is fused with the ritual summon machine
-- (SelectTarget sacrifice -> instance SetGuidData(DATA_SACRIFICED_
-- PLAYER) + DoCast(target, SPELL_RITUAL_PREPARATION 48267) +
-- SetCombatMovement(false) + DoCast(me, SPELL_RITUAL_OF_THE_SWORD
-- 48276)) — summon / instance-GUID / MotionMaster legs, no bridges
-- (the zarithrian Talk-fused-with-summon precedent). Stay out.
-- EVENT_SPAWN_RITUAL_CHANNELERS / EVENT_RITUAL_STRIKE /
-- EVENT_RITUAL_DISARM (:370-384) — all SACRIFICING-gated summon/
-- interrupt legs (SPELL_RITUAL_CHANNELER_{1,2,3} 48271/48274/48275,
-- SPELL_RITUAL_STRIKE_TRIGGER 48331, SPELL_RITUAL_DISARM 54159) — no
-- bridges. Stay out.
-- The whole intro Talk machine — Talk(SAY_SVALA_INTRO_0 0) on the
-- pre-transform entry (EVENT_INTRO_SVALA_TALK_0), Talk(SAY_SVALA_
-- INTRO_1 0) / Talk(SAY_SVALA_INTRO_2 1) on the boss entry
-- (EVENT_INTRO_SVALA_TALK_1 / EVENT_INTRO_SVALA_TALK_2), and the
-- cross-creature arthas->AI()->Talk(SAY_DIALOG_OF_ARTHAS_1 0 /
-- SAY_DIALOG_OF_ARTHAS_2 1) — the intro machine (MoveInLineOfSight
-- IDLE -> INTRO, summon NPC_ARTHAS 29280, MotionMaster takeoff/land,
-- UpdateEntry 29281 -> 26668) has no bridges. Stay out.
-- npc_spectator (26667) — MovementInform flee machine, no Talk arms,
-- no MovementInform bridge. Stay out.
-- spell_paralyze_pinnacle (SpellScript target filter) and
-- achievement_incredible_hulk (GetData(DATA_INCREDIBLE_HULK 2043))
-- — no SpellScript / GetData / achievement bridges. Stay out.

local ENTRY_SVALA_SORROWGRAVE = 26668
local ENTRY_SCOURGE_HULK = 26555
local ENTRY_RITUAL_CHANNELER = 27281

local SAY_AGGRO = 2
local SAY_SLAY = 3
local SAY_DEATH = 4
local EMOTE_ONESHOT_FLYDEATH = 445

local SPELL_MIGHTY_BLOW = 48697
local SPELL_VOLATILE_INFECTION = 56785
local SPELL_SHADOWS_IN_THE_DARK = 59407

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

-- C++ boss_svalaAI::JustEngagedWith (:176): BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function svalaEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit (:209-212): if (who->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent; the
-- nil-guard is the hodir-port convention (Eluna event 3 always carries a
-- victim, so C++-exactness is unaffected).
local function svalaKilledUnit(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    creature:Talk(SAY_SLAY)
end

-- C++ JustDied (:217-221): HandleEmoteCommand(EMOTE_ONESHOT_FLYDEATH
-- 445) unconditional — ported (PerformEmote bridged); Talk(SAY_DEATH) —
-- unconditional — the sjonnir JustDied-Talk precedent; SACRIFICING-gated
-- SetEquipmentSlots + _JustDied() instance passthrough stay out
-- (phase-gated / no bridge).
local function svalaJustDied(event, creature)
    creature:PerformEmote(EMOTE_ONESHOT_FLYDEATH)
    creature:Talk(SAY_DEATH)
end

-- C++ npc_ritual_channelerAI::Reset (:427-432): Initialize() (the
-- paralyzeTimer re-seed — the paralyze machine rides unbridged instance
-- GetGuidData(DATA_SACRIFICED_PLAYER)) + the IsHeroic()-gated DoCast(me,
-- SPELL_SHADOWS_IN_THE_DARK 59407), 2-arg = non-triggered ->
-- CastSpell(creature, 59407) (the Reset AddAura port category —
-- creature:IsHeroic() is bridged, lua_creature_events.go:260). No event-23
-- fire at first grid spawn, so the spawn-time heroic aura stays unmodeled
-- (the palehoof freeze-visual precedent).
local function channelerReset(event, creature)
    if creature:IsHeroic() then
        creature:CastSpell(creature, SPELL_SHADOWS_IN_THE_DARK)
    end
end

-- C++ npc_scourge_hulkAI mightyBlow arm (:571-577): DoCast(victim,
-- SPELL_MIGHTY_BLOW 48697), 2-arg = non-triggered; init urand(4s,9s),
-- re-arm urand(12s,17s). The !HasUnitState(UNIT_STATE_STUNNED) gate is
-- unmodeled (documented in the header).
local function mightyBlowTick(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MIGHTY_BLOW)
    end
    schedule(guid, "mightyblow", {12000, 17000}, function()
        mightyBlowTick(creature, guid)
    end)
end

-- C++ npc_scourge_hulkAI volatileInfection arm (:581-586):
-- DoCastVictim(SPELL_VOLATILE_INFECTION 56785), 1-arg = non-triggered;
-- init urand(10s,14s), re-arm urand(13s,17s). No heroic variant —
-- H_SPELL_VOLATILE_INFECTION 59228 is a dead enum entry, never
-- referenced in the C++ (56785 fires both modes, anomalus
-- precedent).
local function volatileInfectionTick(creature, guid)
    creature:CastSpell(nil, SPELL_VOLATILE_INFECTION)
    schedule(guid, "volatileinfection", {13000, 17000}, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- C++ npc_scourge_hulkAI::Reset re-seeds both timers (Initialize,
-- :540-544) — the xt002 precedent: combat-start scheduling carries the
-- Reset inits.
local function hulkEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mightyblow", {4000, 9000}, function()
        mightyBlowTick(creature, guid)
    end)
    schedule(guid, "volatileinfection", {10000, 14000}, function()
        volatileInfectionTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 on the hulk.
local function hulkCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 1, svalaEnterCombat)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 3, svalaKilledUnit)
RegisterCreatureEvent(ENTRY_SVALA_SORROWGRAVE, 4, svalaJustDied)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 1, hulkEnterCombat)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 2, hulkCancelCombatTimers)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 4, hulkCancelCombatTimers)
RegisterCreatureEvent(ENTRY_SCOURGE_HULK, 23, hulkCancelCombatTimers)
RegisterCreatureEvent(ENTRY_RITUAL_CHANNELER, 23, channelerReset)
