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
-- re-engage (the auriaya engage-port precedent); BossAI::JustEngagedWith
-- SetBossState(DATA_INGVAR 2, IN_PROGRESS) PORTED tick 260 via
-- creature:SetBossState (lua_creature_events.go:5917).
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
-- StopMoving; the EVENT_SUMMON_BANSHEE summon leg is PORTED this tick
-- (direct SummonCreature of Annhylde at the boss +30z — the spell
-- script's SetDest has no bridge); Reset re-arms the flags (kologarn
-- precedent). The
-- PHASE_EVENT damage = 0 rewrite IS bridged (numeric second return,
-- fireCreatureDamageTaken; the anubarak-spike / halazzi precedent) —
-- the latch returns false, 0 and any further PHASE_EVENT damage is
-- zeroed too (C++ boss_ingvar_the_plunderer.cpp:139).
-- JustDied — _JustDied() SetBossState(DATA_INGVAR 2, DONE) PORTED tick 260
-- via creature:SetBossState (lua_creature_events.go:5917), first per C++
-- :180, + Talk(SAY_DEATH 2) unconditional (event 4, both entries —
-- the real (undead-phase) death yell; the sjonnir JustDied-Talk
-- precedent).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3, both entries — no
-- phase gate in C++; the razuvious player-gated variant precedent; the
-- lua victim nil-guard follows the hodir port convention — Eluna event
-- 3 always carries a victim, C++-exactness unaffected).
-- Cast-state gates (2026-11-04): own UpdateAI pre-loop :206 + in-loop
-- :262 CASTING returns ported as 500ms IsCasting defer (live,
-- lua_creature_events.go:1950) on all four PHASE_HUMAN timer arms; the
-- stale "timers fire unconditionally" deviation above is retracted;
-- DamageTaken feign latch ungated per C++ (separate hook); melee stays
-- engine-driven, like C++ DoMeleeAttackIfReady.
-- PORTED THIS TICK (phase-2 machine; the "no bridges" doctrine above
-- is RETRACTED — DoAction ev38, MoveTo/MovementInform ev28,
-- GetInstanceCreature, SummonCreature, UpdateEntry are all live):
-- EVENT_SUMMON_BANSHEE (DoCast(me, 42912), DamageTaken :127 / arm
-- :237-238) as a direct SummonCreature(24068) at the boss +30z, armed
-- off the returned object; npc_annhylde_the_caller (24068) Reset
-- MovePoint(1, z-15) + MovementInform id 1 (YELL_RESURRECT + 42862 /
-- 42857 casts + RESURRECT_1 8s) + RESURRECT_1 (RemoveAura feign death
-- + 42704 heal) + RESURRECT_2 (RemoveAura 42862 + DoAction(0) +
-- MovePoint(2, z+15) -> DespawnOrUnsummon); DoAction(ACTION_START_
-- PHASE_2 = 0) / StartZombiePhase (RemoveAura feign death + triggered
-- 42796 + UpdateEntry 23980 + JUST_TRANSFORMED 500ms); EVENT_JUST_
-- TRANSFORMED (flag clear + ScheduleSecondPhase + SAY_AGGRO +
-- DoZoneInCombat); all PHASE_UNDEAD arms — EVENT_DARK_SMASH (victim
-- 42723, init {14s,18s} / repeat {12s,16s}), EVENT_DREADFUL_ROAR (self
-- 42729, init 0ms fired directly / repeat {18s,22s}), EVENT_WOE_STRIKE
-- (victim 42730, init {10s,14s} / repeat {10s,14s}), EVENT_SHADOW_AXE
-- (random non-tank player, 42748, init 30s / repeat 30s).
-- Deviations: RemoveAurasDueToSpell(42862) has no bridge — RemoveAura
-- (42862) is the single-aura equivalent; UpdateEntry rebinding in Go
-- is unverified (undead entry registers ev3/ev4/ev23 only; ev1/ev9
-- deliberately absent); ingvarReset UpdateEntry is IsAlive-guarded
-- (ev23 pre-fires on death); Annhylde despawned on ingvarReset via the
-- summon latch (C++ _Reset summons.DespawnAll()).
-- DOCUMENTED-ONLY (in this header; no registrations):
-- npc_ingvar_throw_dummy (NPC_THROW_TARGET 23996, line 57) — no Talk arms
-- (FindNearestCreature / MoveCharge Reset legs at lines 384-395,
-- MovementInform EFFECT_MOTION_TYPE charge leg at lines 396-402:
-- CastSpell(me, SPELL_SHADOW_AXE_PERIODIC_DAMAGE 42750, true) +
-- DespawnOrUnsummon) — MoveCharge has no bridge; no registration. The
-- 42748 shadow-axe cast itself is ported (shadowAxeTick) and rides the
-- Go spell system.
-- spell_ingvar_summon_banshee (SpellScript OnDestinationTargetSelect,
-- lines 407-427) — no spell-destination-target bridge; stays in the
-- no-SpellScript-bridge queue (the summon itself is ported directly).
-- spell_ingvar_woe_strike (AuraScript proc, SPELL_WOE_STRIKE_EFFECT
-- 42739, lines 430-476) — PORTED this run (woeStrikeProc below,
-- RegisterAuraEvent event 6; the anetheron OnProc precedent). The
-- CheckProc heal > 0 gate is carried by the Go heal proc legs
-- (procs.go procSpellHealTakenAuraTriggers early-returns on heal == 0),
-- so the handler casts unconditionally and returns true
-- (PreventDefaultAction); the hook's damage arg is always 0 on heal
-- events, so no damage gate. DORMANT until the phase-2 EVENT_WOE_STRIKE
-- arm applies the aura and the DBC proc entry for 42730 ships.

local ENTRY_INGVAR = 23954
local ENTRY_INGVAR_UNDEAD = 23980
local DATA_INGVAR = 2 -- utgarde_keep.h:33

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_CLEAVE = 42724
local SPELL_SMASH = 42669
local SPELL_STAGGERING_ROAR = 42708
local SPELL_ENRAGE = 42705
local SPELL_INGVAR_FEIGN_DEATH = 42795 -- boss_ingvar_the_plunderer.cpp (DamageTaken, triggered self-cast)
local SPELL_WOE_STRIKE = 42730 -- boss_ingvar_the_plunderer.cpp:85
local SPELL_WOE_STRIKE_EFFECT = 42739 -- boss_ingvar_the_plunderer.cpp:86
local SPELL_SUMMON_BANSHEE = 42912 -- boss_ingvar_the_plunderer.cpp:74
local SPELL_INGVAR_TRANSFORM = 42796 -- boss_ingvar_the_plunderer.cpp:96
local SPELL_DARK_SMASH = 42723 -- boss_ingvar_the_plunderer.cpp:80
local SPELL_DREADFUL_ROAR = 42729 -- boss_ingvar_the_plunderer.cpp:81
local SPELL_SHADOW_AXE_SUMMON = 42748 -- boss_ingvar_the_plunderer.cpp:87
local SPELL_SCOURG_RESURRECTION_HEAL = 42704 -- boss_ingvar_the_plunderer.cpp:92
local SPELL_SCOURG_RESURRECTION_BEAM = 42857 -- boss_ingvar_the_plunderer.cpp:93
local SPELL_SCOURG_RESURRECTION_DUMMY = 42862 -- boss_ingvar_the_plunderer.cpp:94

local ENTRY_ANNHYLDE = 24068 -- utgarde_keep.h NPC_ANNHYLDE_THE_CALLER
local ACTION_START_PHASE_2 = 0 -- boss_ingvar_the_plunderer.cpp:110 (enum Misc, first enumerator)
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:36

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

-- Annhylde latches: home position per Annhylde GUID (for the MovePoint(2)
-- return leg) and the live Annhylde per Ingvar GUID (despawned on reset).
local annhyldeHome = {}
local annhyldeByIngvar = {}

-- Forward declaration (tharon_ja / gal_darah idiom): ingvarDamageTaken
-- below schedules summonBansheeTick, defined after it — a later
-- `local function` re-declare would shadow this slot and nil-call, so
-- the definition below is a plain assignment, never re-local.
local summonBansheeTick

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
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function()
            cleaveTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {6000, 12000}, function() cleaveTick(creature, guid) end)
end

-- C++ EVENT_SMASH (lines 227-228): DoCastAOE(SPELL_SMASH 42669) lowers
-- to self-cast, 1-arg = non-triggered -> CastSpell(creature, 42669);
-- init 12-17s (line 168), repeat 12-16s (line 228).
local function smashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "smash", 500, function()
            smashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SMASH)
    schedule(guid, "smash", {12000, 16000}, function() smashTick(creature, guid) end)
end

-- C++ EVENT_STAGGERING_ROAR (lines 219-220): DoCast(me,
-- SPELL_STAGGERING_ROAR 42708), 2-arg = non-triggered ->
-- CastSpell(creature, 42708); init 18-21s (line 166), repeat 18-22s
-- (line 220).
local function roarTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "roar", 500, function()
            roarTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STAGGERING_ROAR)
    schedule(guid, "roar", {18000, 22000}, function() roarTick(creature, guid) end)
end

-- C++ EVENT_ENRAGE (lines 223-224): DoCast(me, SPELL_ENRAGE 42705),
-- 2-arg = non-triggered -> CastSpell(creature, 42705); init 7-14s
-- (line 167), repeat 7-14s (line 224).
local function enrageTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "enrage", 500, function()
            enrageTick(creature, guid)
        end)
        return
    end
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
    local state = getState(creature:GetGUIDLow())
    if state.engaged then
        return
    end
    state.engaged = true
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- BossAI::JustEngagedWith (:161, after the phase gate the engaged
    -- latch mirrors) — SetBossState(DATA_INGVAR 2, IN_PROGRESS) PORTED
    -- tick 260 via creature:SetBossState (lua_creature_events.go:5917),
    -- before SetCombatPulseDelay(5) + me-form DoZoneInCombat (C++
    -- BossAI::_JustEngagedWith order, ScriptedCreature.cpp:527-544).
    creature:SetBossState(DATA_INGVAR, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
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
    local state = getState(creature:GetGUIDLow())
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
        cancelTimers(creature:GetGUIDLow())
        creature:RemoveAllAuras()
        -- C++ DamageTaken transition legs (lines 127-136): triggered
        -- feign-death self-cast + SetFlag(NON_ATTACKABLE |
        -- NOT_SELECTABLE) + SetImmuneToPC(true) (= SetFlag(59, 0x100),
        -- Unit.cpp:8455-8463; EVENT_SUMMON_BANSHEE leg ported this tick
        -- via the 3s summonBansheeTick schedule below).
        creature:StopMoving() -- boss_ingvar_the_plunderer.cpp:130
        creature:CastSpell(creature, SPELL_INGVAR_FEIGN_DEATH, true)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:Talk(SAY_DEATH)
        -- C++ :127 EVENT_SUMMON_BANSHEE 3s (PHASE_EVENT): DoCast(me,
        -- SPELL_SUMMON_BANSHEE 42912) — the SpellScript relocates the
        -- summon dest +30z; no SpellScript bridge, so Annhylde is
        -- summoned directly (summonBansheeTick below).
        local guid = creature:GetGUIDLow()
        schedule(guid, "summonbanshee", 3000, function()
            summonBansheeTick(creature, guid)
        end)
        -- C++ DamageTaken zeroes the killing blow in the same call
        -- (PHASE_EVENT rewrite) — without the rewrite Go applies the
        -- lethal hit and the undead-phase machine never starts.
        return false, 0
    end
end

local function ingvarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ JustDied: _JustDied() SetBossState(DATA_INGVAR 2, DONE) PORTED
-- tick 260 via creature:SetBossState (lua_creature_events.go:5917),
-- first per C++ :180; Talk(SAY_DEATH) — unconditional — the sjonnir
-- JustDied-Talk precedent. Fires on the real (undead-phase) death; the
-- feign-death yell rides event 9 above.
local function ingvarJustDied(event, creature)
    creature:SetBossState(DATA_INGVAR, 3)
    cancelTimers(creature:GetGUIDLow())
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
    cancelTimers(creature:GetGUIDLow())
    ingvarState[creature:GetGUIDLow()] = nil
    -- C++ _Reset() summons.DespawnAll(): the latched Annhylde goes with
    -- it (she is the boss's summon via the 42912 leg).
    local annhylde = annhyldeByIngvar[creature:GetGUIDLow()]
    if annhylde ~= nil then
        annhylde:Despawn()
        annhyldeByIngvar[creature:GetGUIDLow()] = nil
    end
    -- C++ Reset (lines 113-115): if (GetEntry() != NPC_INGVAR)
    -- UpdateEntry(NPC_INGVAR) — reverts a phase-2 wipe's undead form
    -- (Creature::UpdateEntry twin, bridged 2026-10-22). Alive-guarded:
    -- ev23 pre-fires on death (fireCreatureDied runs Reset before the
    -- ev4 JustDied leg) and C++ Reset never runs on a corpse — an
    -- undead-form corpse must not revert to human entry.
    if creature:IsAlive() and creature:GetEntry() ~= ENTRY_INGVAR then
        creature:UpdateEntry(ENTRY_INGVAR)
    end
    -- C++ Reset (lines 113-118): RemoveFlag(NON_ATTACKABLE |
    -- NOT_SELECTABLE) + SetImmuneToPC(false) (= RemoveFlag(59, 0x100))
    -- re-arms the feign-death flags so a post-feign wipe re-engages
    -- clean (kologarn evade re-arm precedent).
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    -- C++ Reset (line 119): _Reset() — SetBossState(DATA_INGVAR 2,
    -- NOT_STARTED) PORTED tick 260 via creature:SetBossState
    -- (lua_creature_events.go:5917); placed after the flag re-arms per
    -- C++ order (Reset body :113-117 runs before _Reset() :119).
    creature:SetBossState(DATA_INGVAR, 0)
end


-- C++ EVENT_DARK_SMASH (lines 242-243): DoCastVictim(SPELL_DARK_SMASH
-- 42723), 1-arg = non-triggered -> CastSpell(nil, 42723); init 14-18s
-- (line 187), repeat 12-16s (line 243).
local function darkSmashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "darksmash", 500, function()
            darkSmashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_DARK_SMASH)
    schedule(guid, "darksmash", {12000, 16000}, function() darkSmashTick(creature, guid) end)
end

-- C++ EVENT_DREADFUL_ROAR (lines 246-247): DoCast(me,
-- SPELL_DREADFUL_ROAR 42729), 2-arg = non-triggered ->
-- CastSpell(creature, 42729); init 0ms (line 188), repeat 18-22s
-- (line 247).
local function dreadfulRoarTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "dreadfulroar", 500, function()
            dreadfulRoarTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_DREADFUL_ROAR)
    schedule(guid, "dreadfulroar", {18000, 22000}, function() dreadfulRoarTick(creature, guid) end)
end

-- C++ EVENT_WOE_STRIKE (lines 250-251): DoCastVictim(SPELL_WOE_STRIKE
-- 42730), 1-arg = non-triggered -> CastSpell(nil, 42730); init 10-14s
-- (line 189), repeat 10-14s (line 251).
local function woeStrikeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "woestrike", 500, function()
            woeStrikeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_WOE_STRIKE)
    schedule(guid, "woestrike", {10000, 14000}, function() woeStrikeTick(creature, guid) end)
end

-- C++ EVENT_SHADOW_AXE (lines 254-256): SelectTarget(Random, 1, 0.0f,
-- true) — random NON-tank player (offset 1 skips the top-threat entry)
-- -> DoCast(target, SPELL_SHADOW_AXE_SUMMON 42748), 2-arg =
-- non-triggered; init 30s (line 190), repeat 30s (line 256). The
-- summoned npc_ingvar_throw_dummy AI stays out (MoveCharge unbridged);
-- the 42748 cast itself rides the Go spell system.
local function shadowAxeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowaxe", 500, function()
            shadowAxeTick(creature, guid)
        end)
        return
    end
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    -- C++ offset 1 skips the top-threat (tank) entry; with one or zero
    -- players there is no valid target. GetAITargets is threat-ordered.
    if #candidates > 1 then
        creature:CastSpell(candidates[math.random(2, #candidates)], SPELL_SHADOW_AXE_SUMMON)
    end
    schedule(guid, "shadowaxe", 30000, function() shadowAxeTick(creature, guid) end)
end

-- C++ EVENT_JUST_TRANSFORMED (lines 231-236): RemoveFlag(NON_ATTACKABLE
-- | NOT_SELECTABLE) + SetImmuneToPC(false) + ScheduleSecondPhase +
-- Talk(SAY_AGGRO) + DoZoneInCombat. Runs 500ms after StartZombiePhase.
local function justTransformedTick(creature, guid)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    -- C++ ScheduleSecondPhase (lines 185-191): phase UNDEAD; dark smash
    -- {14s,18s}, dreadful roar 0ms, woe strike {10s,14s}, shadow axe 30s.
    schedule(guid, "darksmash", {14000, 18000}, function() darkSmashTick(creature, guid) end)
    -- 0ms init: fire the arm directly (no 0-delay timer convention).
    dreadfulRoarTick(creature, guid)
    schedule(guid, "woestrike", {10000, 14000}, function() woeStrikeTick(creature, guid) end)
    schedule(guid, "shadowaxe", 30000, function() shadowAxeTick(creature, guid) end)
    creature:Talk(SAY_AGGRO)
    creature:DoZoneInCombat()
end

-- C++ DoAction(ACTION_START_PHASE_2) (lines 143-155) -> StartZombiePhase:
-- RemoveAura(feign death) + triggered transform self-cast 42796 +
-- UpdateEntry(NPC_INGVAR_UNDEAD 23980) + EVENT_JUST_TRANSFORMED 500ms.
-- Fired by Annhylde's EVENT_RESURRECT_2 via ev38 (DoAction bridge,
-- lua_creature_events.go:827). UpdateEntry rebinding in Go is unverified
-- — the undead entry's ev3/ev4/ev23 handlers are registered below and
-- ev1/ev9 are deliberately absent for 23980 (C++ JustEngagedWith
-- returns early in PHASE_UNDEAD; phase 2 has no DamageTaken arm).
local function ingvarDoAction(event, creature, action)
    if action ~= ACTION_START_PHASE_2 then
        return
    end
    creature:RemoveAura(SPELL_INGVAR_FEIGN_DEATH)
    creature:CastSpell(creature, SPELL_INGVAR_TRANSFORM, true)
    creature:UpdateEntry(ENTRY_INGVAR_UNDEAD)
    local guid = creature:GetGUIDLow()
    schedule(guid, "justtransformed", 500, function()
        justTransformedTick(creature, guid)
    end)
end

-- C++ EVENT_RESURRECT_2 (lines 346-352): ingvar RemoveAurasDueToSpell
-- (42862) + DoAction(ACTION_START_PHASE_2); Annhylde MovePoint(2, x, y,
-- z+15). RemoveAurasDueToSpell has no bridge — RemoveAura(42862) is the
-- single-aura equivalent. DoAction(0) fires the ingvar ev38 handler.
local function resurrect2Tick(creature, guid)
    local ingvar = creature:GetInstanceCreature(DATA_INGVAR)
    if ingvar ~= nil then
        ingvar:RemoveAura(SPELL_SCOURG_RESURRECTION_DUMMY)
        ingvar:DoAction(ACTION_START_PHASE_2)
    end
    local home = annhyldeHome[guid]
    if home ~= nil then
        creature:MoveTo(2, home.x, home.y, home.z + 15)
    end
end

-- C++ EVENT_RESURRECT_1 (lines 336-344): ingvar RemoveAura(feign death)
-- + ingvar self-cast SCOURG_RESURRECTION_HEAL 42704 (non-triggered) +
-- EVENT_RESURRECT_2 3s.
local function resurrect1Tick(creature, guid)
    local ingvar = creature:GetInstanceCreature(DATA_INGVAR)
    if ingvar ~= nil then
        ingvar:RemoveAura(SPELL_INGVAR_FEIGN_DEATH)
        ingvar:CastSpell(ingvar, SPELL_SCOURG_RESURRECTION_HEAL)
    end
    schedule(guid, "resurrect2", 3000, function()
        resurrect2Tick(creature, guid)
    end)
end

-- C++ npc_annhylde_the_caller::MovementInform (lines 300-323):
-- POINT_MOTION_TYPE only; id 1 -> Talk(YELL_RESURRECT 0) + ingvar
-- RemoveAura(42912) + ingvar self-cast 42862 (triggered) + Annhylde
-- cast 42857 on ingvar + EVENT_RESURRECT_1 8s; id 2 ->
-- DespawnOrUnsummon.
local function annhyldeMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    if id == 1 then
        creature:Talk(0) -- YELL_RESURRECT (boss_ingvar_the_plunderer.cpp:22)
        local ingvar = creature:GetInstanceCreature(DATA_INGVAR)
        if ingvar ~= nil then
            ingvar:RemoveAura(SPELL_SUMMON_BANSHEE)
            ingvar:CastSpell(ingvar, SPELL_SCOURG_RESURRECTION_DUMMY, true)
            creature:CastSpell(ingvar, SPELL_SCOURG_RESURRECTION_BEAM)
        end
        local guid = creature:GetGUIDLow()
        schedule(guid, "resurrect1", 8000, function()
            resurrect1Tick(creature, guid)
        end)
    elseif id == 2 then
        local ingvar = creature:GetInstanceCreature(DATA_INGVAR)
        if ingvar ~= nil then
            annhyldeByIngvar[ingvar:GetGUIDLow()] = nil
        end
        annhyldeHome[creature:GetGUIDLow()] = nil
        creature:DespawnOrUnsummon()
    end
end

-- C++ npc_annhylde_the_caller::Reset (lines 293-298): _events.Reset +
-- GetPosition + MovePoint(1, x, y, z - 15.0f). Armed off the summon
-- return object (temp summons may never fire ev23; the keleseth
-- decrepify precedent); ev23 covers any respawn-path Reset.
local function annhyldeReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    local x, y, z = creature:GetX(), creature:GetY(), creature:GetZ()
    annhyldeHome[guid] = { x = x, y = y, z = z }
    creature:MoveTo(1, x, y, z - 15)
end

-- No C++ JustDied arm for Annhylde — timer hygiene only, so a slain
-- Annhylde cannot keep driving the resurrect machine on a corpse.
local function annhyldeDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
    annhyldeHome[creature:GetGUIDLow()] = nil
    local ingvar = creature:GetInstanceCreature(DATA_INGVAR)
    if ingvar ~= nil then
        annhyldeByIngvar[ingvar:GetGUIDLow()] = nil
    end
end

-- C++ EVENT_SUMMON_BANSHEE (lines 237-238): DoCast(me,
-- SPELL_SUMMON_BANSHEE 42912) — the SpellScript relocates the summon
-- dest +30z and summons Annhylde (24068); no SpellScript bridge, so the
-- summon is direct at the boss position +30z (the keleseth tomb
-- precedent). Annhylde's Reset/MovePoint leg is armed off the returned
-- object (temp summons may never fire ev23).
summonBansheeTick = function(creature, guid)
    local annhylde = creature:SummonCreature(ENTRY_ANNHYLDE, creature:GetX(), creature:GetY(), creature:GetZ() + 30, creature:GetO())
    if annhylde ~= nil then
        annhyldeByIngvar[guid] = annhylde
        annhyldeReset(0, annhylde)
    end
end

-- C++ instance_utgarde_keep.cpp:57/:140 OnCreatureCreate latches the
-- Ingvar GUID; the Lua mirror is the ev5 SetGuidData (sjonnir precedent)
-- so Annhylde's GetInstanceCreature(DATA_INGVAR) resolves.
local function ingvarSpawned(event, creature)
    creature:SetGuidData(DATA_INGVAR)
end

-- C++ spell_ingvar_woe_strike::HandleProc
-- (boss_ingvar_the_plunderer.cpp:460-467): OnEffectProc, EFFECT_1
-- SPELL_AURA_PROC_TRIGGER_SPELL. CheckProc requires a heal event with
-- heal > 0; HandleProc PreventDefaultActions and the aura holder casts
-- SPELL_WOE_STRIKE_EFFECT 42739 on the healer (eventInfo.GetActor()),
-- triggered (the aurEff overload).
local function woeStrikeProc(event, aura, target, actor, procTarget, damage)
    if target == nil or actor == nil then
        return true
    end
    target:CastSpell(actor, SPELL_WOE_STRIKE_EFFECT, true)
    return true
end

RegisterAuraEvent(SPELL_WOE_STRIKE, 6, woeStrikeProc)

RegisterCreatureEvent(ENTRY_INGVAR, 1, ingvarEnterCombat)
RegisterCreatureEvent(ENTRY_INGVAR, 2, ingvarLeaveCombat)
RegisterCreatureEvent(ENTRY_INGVAR, 5, ingvarSpawned)
RegisterCreatureEvent(ENTRY_INGVAR, 9, ingvarDamageTaken)
RegisterCreatureEvent(ENTRY_INGVAR, 3, ingvarKilledUnit)
RegisterCreatureEvent(ENTRY_INGVAR, 4, ingvarJustDied)
RegisterCreatureEvent(ENTRY_INGVAR, 23, ingvarReset)
RegisterCreatureEvent(ENTRY_INGVAR, 38, ingvarDoAction)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 3, ingvarKilledUnit)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 4, ingvarJustDied)
RegisterCreatureEvent(ENTRY_INGVAR_UNDEAD, 23, ingvarReset)
RegisterCreatureEvent(ENTRY_ANNHYLDE, 4, annhyldeDied)
RegisterCreatureEvent(ENTRY_ANNHYLDE, 23, annhyldeReset)
RegisterCreatureEvent(ENTRY_ANNHYLDE, 28, annhyldeMovementInform)
