-- Ingvar the Plunderer (Utgarde Keep) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/boss_ingvar_the_plunderer.cpp
-- (489 lines; 5 scripts — boss_ingvar_the_plunderer (CreatureScript via
-- GetUtgardeKeepAI (BossAI), DATA_INGVAR = 2); npc_annhylde_the_caller
-- (CreatureScript via GetUtgardeKeepAI (ScriptedAI)); npc_ingvar_throw_dummy
-- (CreatureScript via GetUtgardeKeepAI (ScriptedAI));
-- spell_ingvar_summon_banshee (SpellScriptLoader, SpellScript
-- OnDestinationTargetSelect); spell_ingvar_woe_strike (SpellScriptLoader,
-- AuraScript proc); all registered from inside
-- AddSC_boss_ingvar_the_plunderer(); loader decl 128 / call 323 per
-- northrend_script_loader.cpp — the THIRD group of the "// Utgarde Keep -
-- Utgarde Keep" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_skarvald_dalronn() (decl 127 / call 322); the checkpoint
-- sequence (boss_skarvald_dalronn -> boss_ingvar_the_plunderer) is
-- followed).
-- Entries: 23954 Ingvar the Plunderer (utgarde_keep.h NPC_INGVAR, line 49)
-- / 23980 Ingvar the Plunderer undead form (NPC_INGVAR_UNDEAD, line 56) —
-- entry-verifiable, registration proceeds (the nexus_commanders / malygos /
-- sartharion kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual. DATA_INGVAR = 2 (utgarde_keep.h line 33).
-- Sole-source verified: whole-server-tree grep for all five script names
-- hits boss_ingvar_the_plunderer.cpp only (+ the loader decl/call lines
-- for AddSC_boss_ingvar_the_plunderer); the clone's sql/ tree
-- (~/workspace/moreno-trinitycore/sql/, characters/ + world/) carries
-- zero ingvar hits (empty of bindings), so ScriptName bindings are
-- DB-side by construction. This header is the audited-artifact record
-- for the existing port (9df78d1, Oct 2 10:06 UTC) — re-verified this
-- run; the first-pass "no ingvar lua existed" claim was stale.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Timer bridge: CreateLuaEvent harness (engine/scripting/runtime.go:798),
-- GUID-keyed timers table, cancel on events 2/4/23 (the
-- maiden_of_virtue / moroes / curator / nightbane / hodir precedent) —
-- the Oct-2 "no-timer-bridge" doctrine is STALE and disproved.
-- CastSpell(nil, spell) = DoCastVictim; CastSpell(creature, spell) =
-- DoCastSelf; DoCastAOE lowers to self-cast (boss_ai.go DoCastAOE
-- lowering; the selin_fireheart / mr_smite precedent). The 3rd CastSpell
-- arg is the C++ DoCast 3rd arg (2-arg / 1-arg overloads =
-- non-triggered). {min,max} table delay = randtime range. Phase gating:
-- the four PHASE_HUMAN arms are scheduled unconditionally in
-- JustEngagedWith (C++ lines 165-168); the EventMap phase gate is
-- modeled in Lua by canceling all timers on the feign-death
-- DamageTaken latch (PHASE_HUMAN -> PHASE_EVENT, C++ lines 124-138),
-- after which the human arms never fire again in C++ either.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1, human entry only) +
-- the four JustEngagedWith ScheduleEvent legs (C++ lines 165-168); the
-- C++ early return on PHASE_EVENT / PHASE_UNDEAD is modeled with the
-- engaged latch below — yell once per pull cycle, silent on the phase-2
-- re-engage (the auriaya engage-port precedent); BossAI passthrough has
-- no bridge.
-- EVENT_CLEAVE (id 1; init 6-12s at C++ line 165, arm at 215-216):
-- DoCastVictim(SPELL_CLEAVE 42724), 1-arg = non-triggered ->
-- CastSpell(nil, 42724); repeat 6-12s (line 216).
-- EVENT_SMASH (id 2; init 12-17s at C++ line 168, arm at 227-228):
-- DoCastAOE(SPELL_SMASH 42669), 1-arg = non-triggered ->
-- CastSpell(creature, 42669); repeat 12-16s (line 228).
-- EVENT_STAGGERING_ROAR (id 3; init 18-21s at C++ line 166, arm at
-- 219-220): DoCast(me, SPELL_STAGGERING_ROAR 42708), 2-arg =
-- non-triggered -> CastSpell(creature, 42708); repeat 18-22s (line 220).
-- EVENT_ENRAGE (id 4; init 7-14s at C++ line 167, arm at 223-224):
-- DoCast(me, SPELL_ENRAGE 42705), 2-arg = non-triggered ->
-- CastSpell(creature, 42705); repeat 7-14s (line 224).
-- DamageTaken — Talk(SAY_DEATH 2) when damage >= GetHealth() while in
-- PHASE_HUMAN (the feign-death yell) (event 9, human entry only); the
-- feignDead latch mirrors the PHASE_HUMAN -> PHASE_EVENT transition so
-- the yell fires exactly once (the moroes threshold+latch precedent);
-- the latch also cancels the human timers (the phase gate above) and
-- ports the DamageTaken transition legs: RemoveAllAuras + triggered
-- self-cast SPELL_INGVAR_FEIGN_DEATH 42795 + SetFlag(NON_ATTACKABLE |
-- NOT_SELECTABLE) + SetImmuneToPC(true) (= SetFlag(59, 0x100)) +
-- StopMoving; the EVENT_SUMMON_BANSHEE summon leg stays out (no summon
-- bridge); Reset re-arms the flags (kologarn precedent). The
-- PHASE_EVENT damage = 0 rewrite IS bridged (numeric second return,
-- fireCreatureDamageTaken; the anubarak-spike / halazzi precedent) —
-- the latch returns false, 0 and any further PHASE_EVENT damage is
-- zeroed too (C++ boss_ingvar_the_plunderer.cpp:139).
-- JustDied — Talk(SAY_DEATH 2) unconditional (event 4, both entries —
-- the real (undead-phase) death yell; the sjonnir JustDied-Talk
-- precedent).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3, both entries — no
-- phase gate in C++; the razuvious player-gated variant precedent; the
-- lua victim nil-guard follows the hodir port convention — Eluna event
-- 3 always carries a victim, C++-exactness unaffected).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally — melee stays
-- engine-driven, like C++ DoMeleeAttackIfReady (the hodir /
-- maiden_of_virtue precedent).
-- DOCUMENTED-ONLY (in this header; no registrations beyond entries 23954
-- / 23980):
-- All PHASE_UNDEAD arms — EVENT_DARK_SMASH (DoCastVictim
-- SPELL_DARK_SMASH 42723, init 14-18s line 187 / arm lines 242-243,
-- repeat 12-16s), EVENT_DREADFUL_ROAR (DoCast(me,
-- SPELL_DREADFUL_ROAR 42729, init 0ms line 188 / arm lines 246-247,
-- repeat 18-22s), EVENT_WOE_STRIKE (DoCastVictim SPELL_WOE_STRIKE
-- 42730, init 10-14s line 189 / arm lines 250-251, repeat 10-14s),
-- EVENT_SHADOW_AXE (SelectTarget(Random,1) DoCast(target,
-- SPELL_SHADOW_AXE_SUMMON 42748), init 30s line 190 / arm lines
-- 254-256, repeat 30s) — scheduled only from ScheduleSecondPhase (lines
-- 185-191), which runs on EVENT_JUST_TRANSFORMED (lines 231-236) after
-- StartZombiePhase, which runs on DoAction(ACTION_START_PHASE_2) (lines
-- 143-155), which is invoked by npc_annhylde_the_caller's
-- EVENT_RESURRECT_2 arm (line 349) — DoAction / MotionMaster /
-- instance-GuidData legs, no bridges; the whole phase-2 machine (the
-- phase-machine gating doctrine, gothik precedent) is unbridgeable; the
-- SHADOW_AXE arm additionally has a summon leg.
-- EVENT_SUMMON_BANSHEE (DoCast(me, SPELL_SUMMON_BANSHEE 42912),
-- scheduled at PHASE_EVENT from DamageTaken line 127 / arm lines
-- 237-238) — PHASE_EVENT-gated summon leg; unbridgeable.
-- EVENT_JUST_TRANSFORMED Talk(SAY_AGGRO) + ScheduleSecondPhase +
-- DoZoneInCombat (lines 231-236) — phase-machine / zone-in legs, no
-- bridges; the phase-2 aggro yell is DoAction-gated.
-- DoAction(ACTION_START_PHASE_2) / StartZombiePhase (RemoveAura feign
-- death, DoCast(me, SPELL_INGVAR_TRANSFORM 42796, true), UpdateEntry to
-- 23980 — lines 149-155) — DoAction / UpdateEntry / cross-entry legs,
-- no bridges.
-- npc_annhylde_the_caller (24068 NPC_ANNHYLDE_THE_CALLER, line 58) —
-- Talk(YELL_RESURRECT 0) in MovementInform (line 304), plus the
-- cross-creature resurrect machine (MovementInform legs at lines 302-316:
-- instance GetGuidData(DATA_INGVAR), SPELL_SCOURG_RESURRECTION_DUMMY
-- 42862 / BEAM 42857 casts; EVENT_RESURRECT_1 at lines 336-344:
-- SPELL_SCOURG_RESURRECTION_HEAL 42704 cast; EVENT_RESURRECT_2 at lines
-- 346-350: RemoveAuras SPELL_SCOURG_RESURRECTION_DUMMY 42862 +
-- DoAction(ACTION_START_PHASE_2), MovePoint legs at lines 318-320/352) —
-- MovementInform / instance-GuidData / MotionMaster / DoAction bridges
-- missing; no registration.
-- npc_ingvar_throw_dummy (NPC_THROW_TARGET 23996, line 57) — no Talk arms
-- (FindNearestCreature / MoveCharge Reset legs at lines 384-395,
-- MovementInform EFFECT_MOTION_TYPE charge leg at lines 396-402:
-- CastSpell(me, SPELL_SHADOW_AXE_PERIODIC_DAMAGE 42750, true) +
-- DespawnOrUnsummon) — MotionMaster / despawn legs, no bridges; no
-- registration.
-- spell_ingvar_summon_banshee (SpellScript OnDestinationTargetSelect,
-- lines 407-427) / spell_ingvar_woe_strike (AuraScript proc,
-- SPELL_WOE_STRIKE_EFFECT 42739, lines 430-476) — no SpellScript /
-- AuraScript bridges; join the no-SpellScript / no-AuraScript-bridge
-- queues.

local ENTRY_INGVAR = 23954
local ENTRY_INGVAR_UNDEAD = 23980

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_CLEAVE = 42724
local SPELL_SMASH = 42669
local SPELL_STAGGERING_ROAR = 42708
local SPELL_ENRAGE = 42705
local SPELL_INGVAR_FEIGN_DEATH = 42795 -- boss_ingvar_the_plunderer.cpp (DamageTaken, triggered self-cast)

local UNIT_FIELD_FLAGS = 59 -- UnitDefines.h
local UNIT_FLAG_NON_ATTACKABLE = 0x00000002 -- UnitDefines.h
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h
local UNIT_FLAG_IMMUNE_TO_PC = 0x100 -- UnitDefines.h (Unit.cpp SetImmuneToPC)

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

-- Per-creature state mirroring the AI members / EventMap phases:
-- engaged — C++ JustEngagedWith sets PHASE_HUMAN on the first engage;
-- re-engages in PHASE_EVENT / PHASE_UNDEAD return early with no yell.
-- feignDead — C++ DamageTaken transitions PHASE_HUMAN -> PHASE_EVENT on
-- the feign-death Talk(SAY_DEATH); the latch keeps the yell one-shot.
-- C++ Reset() (_Reset -> events.Reset()) clears the phases; event 23
-- does the same here.
local ingvarState = {}

local function getState(guid)
    local state = ingvarState[guid]
    if state == nil then
        state = { engaged = false, feignDead = false }
        ingvarState[guid] = state
    end
    return state
end

-- C++ EVENT_CLEAVE (lines 215-216): DoCastVictim(SPELL_CLEAVE 42724),
-- 1-arg = non-triggered -> CastSpell(nil, 42724); init 6-12s (line
-- 165), repeat 6-12s (line 216).
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {6000, 12000}, function() cleaveTick(creature, guid) end)
end

-- C++ EVENT_SMASH (lines 227-228): DoCastAOE(SPELL_SMASH 42669) lowers
-- to self-cast, 1-arg = non-triggered -> CastSpell(creature, 42669);
-- init 12-17s (line 168), repeat 12-16s (line 228).
local function smashTick(creature, guid)
    creature:CastSpell(creature, SPELL_SMASH)
    schedule(guid, "smash", {12000, 16000}, function() smashTick(creature, guid) end)
end

-- C++ EVENT_STAGGERING_ROAR (lines 219-220): DoCast(me,
-- SPELL_STAGGERING_ROAR 42708), 2-arg = non-triggered ->
-- CastSpell(creature, 42708); init 18-21s (line 166), repeat 18-22s
-- (line 220).
local function roarTick(creature, guid)
    creature:CastSpell(creature, SPELL_STAGGERING_ROAR)
    schedule(guid, "roar", {18000, 22000}, function() roarTick(creature, guid) end)
end

-- C++ EVENT_ENRAGE (lines 223-224): DoCast(me, SPELL_ENRAGE 42705),
-- 2-arg = non-triggered -> CastSpell(creature, 42705); init 7-14s
-- (line 167), repeat 7-14s (line 224).
local function enrageTick(creature, guid)
    creature:CastSpell(creature, SPELL_ENRAGE)
    schedule(guid, "enrage", {7000, 14000}, function() enrageTick(creature, guid) end)
end

-- C++ boss_ingvar_the_plundererAI::JustEngagedWith (lines 159-170): if
-- in PHASE_EVENT or PHASE_UNDEAD return; BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO); SetPhase(PHASE_HUMAN);
-- the four ScheduleEvent legs become CreateLuaEvent timers below. The
-- engaged latch models the phase gate — C++-exact: one yell per pull
-- cycle, silent on the phase-2 re-engage (the auriaya engage-port
-- precedent).
local function ingvarEnterCombat(event, creature, target)
    local state = getState(creature:GetGUID())
    if state.engaged then
        return
    end
    state.engaged = true
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "cleave", {6000, 12000}, function() cleaveTick(creature, guid) end)
    schedule(guid, "smash", {12000, 17000}, function() smashTick(creature, guid) end)
    schedule(guid, "roar", {18000, 21000}, function() roarTick(creature, guid) end)
    schedule(guid, "enrage", {7000, 14000}, function() enrageTick(creature, guid) end)
end

-- C++ DamageTaken (lines 122-140): if (damage >= me->GetHealth() &&
-- events.IsInPhase(PHASE_HUMAN)) { RemoveAllAuras + triggered
-- feign-death self-cast 42795 + SetFlag(NON_ATTACKABLE | NOT_SELECTABLE)
-- + SetFlag(0x100) (all ported below; StopMoving / summon legs
-- unbridged); Talk(SAY_DEATH); } — the engaged gate mirrors the PHASE_HUMAN
-- requirement, the feignDead latch the one-shot transition (the moroes
-- threshold+latch precedent); cancelTimers mirrors the EventMap phase
-- gate — the human timers never fire again once PHASE_EVENT starts.
-- The PHASE_EVENT damage = 0 rewrite IS bridged (return false, 0 —
-- the anubarak-spike / halazzi precedent; see header).
local function ingvarDamageTaken(event, creature, attacker, damage)
    local state = getState(creature:GetGUID())
    if state.feignDead then
        -- C++ zeroes ALL damage in PHASE_EVENT
        -- (boss_ingvar_the_plunderer.cpp:139) — a hit already in flight
        -- must not kill the feigning boss.
        return false, 0
    end
    if not state.engaged then
        return
    end
    if damage >= creature:GetHealth() then
        state.feignDead = true
        cancelTimers(creature:GetGUID())
        creature:RemoveAllAuras()
        -- C++ DamageTaken transition legs (lines 127-136): triggered
        -- feign-death self-cast + SetFlag(NON_ATTACKABLE |
        -- NOT_SELECTABLE) + SetImmuneToPC(true) (= SetFlag(59, 0x100),
        -- Unit.cpp:8455-8463; EVENT_SUMMON_BANSHEE summon leg stays
        -- out — no summon bridge).
        creature:StopMoving() -- boss_ingvar_the_plunderer.cpp:130
        creature:CastSpell(creature, SPELL_INGVAR_FEIGN_DEATH, true)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:Talk(SAY_DEATH)
        -- C++ DamageTaken zeroes the killing blow in the same call
        -- (PHASE_EVENT rewrite) — without the rewrite Go applies the
        -- lethal hit and the undead-phase machine never starts.
        return false, 0
    end
end

local function ingvarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied: _JustDied() (passthrough, no bridge); Talk(SAY_DEATH) —
-- unconditional — the sjonnir JustDied-Talk precedent. Fires on the real
-- (undead-phase) death; the feign-death yell rides event 9 above.
local function ingvarJustDied(event, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY)
-- — no phase gate — the razuvious player-gated variant precedent; the
-- nil-guard follows the hodir port convention (Eluna event 3 always
-- carries a victim; C++-exactness unaffected).
local function ingvarKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local function ingvarReset(event, creature)
    cancelTimers(creature:GetGUID())
    ingvarState[creature:GetGUID()] = nil
    -- C++ Reset (lines 113-115): if (GetEntry() != NPC_INGVAR)
    -- UpdateEntry(NPC_INGVAR) — reverts a phase-2 wipe's undead form
    -- (Creature::UpdateEntry twin, bridged 2026-10-22). The
    -- StartZombiePhase UpdateEntry(NPC_INGVAR_UNDEAD) rides the unbridged
    -- DoAction(ACTION_START_PHASE_2) leg (documented in the header).
    if creature:GetEntry() ~= ENTRY_INGVAR then
        creature:UpdateEntry(ENTRY_INGVAR)
    end
    -- C++ Reset (lines 113-118): RemoveFlag(NON_ATTACKABLE |
    -- NOT_SELECTABLE) + SetImmuneToPC(false) (= RemoveFlag(59, 0x100))
    -- re-arms the feign-death flags so a post-feign wipe re-engages
    -- clean (kologarn evade re-arm precedent).
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
end

RegisterCreatureEvent(ENTRY_INGVAR, 1, ingvarEnterCombat)
RegisterCreatureEvent(ENTRY_INGVAR, 2, ingvarLeaveCombat)
RegisterCreatureEvent(ENTRY_INGVAR, 9, ingvarDamageTaken)
RegisterCreatureEvent(ENTRY_INGVAR, 3, ingvarKilledUnit)
RegisterCreatureEvent(ENTRY_INGVAR, 4, ingvarJustDied)
RegisterCreatureEvent(ENTRY_INGVAR, 23, ingvarReset)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 3, ingvarKilledUnit)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 4, ingvarJustDied)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 23, ingvarReset)
