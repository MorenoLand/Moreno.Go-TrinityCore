-- Zuramat the Obliterator (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_zuramat.cpp
-- (232 lines incl. license; 3 scripts — boss_zuramat (CreatureScript via
-- GetVioletHoldAI (BossAI), DATA_ZURAMAT = 8); npc_void_sentry
-- (CreatureScript via GetVioletHoldAI (ScriptedAI), zero Talk arms,
-- ported below: ev5/ev22/ev19/ev20/ev4); achievement_void_dance (AchievementCriteriaScript OnCheck
-- AI()->GetData(DATA_VOID_DANCE)); all registered from inside
-- AddSC_boss_zuramat(); loader decl 151 / call 346 per
-- northrend_script_loader.cpp — the SEVENTH group of the
-- "// Violet Hold" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_xevozz() (call 345); the call after it is
-- AddSC_instance_violet_hold() (call 347) — the checkpoint sequence
-- (boss_xevozz -> boss_zuramat) is followed).
-- Entries: 29314 Zuramat the Obliterator (violet_hold.h NPC_ZURAMAT,
-- line 101) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual. DATA_ZURAMAT
-- = 8 (violet_hold.h line 58). NPC_VOID_SENTRY = 29364 (line 102,
-- zero Talk — not registered); NPC_VOID_SENTRY_BALL = 29365 (line
-- 103).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_zuramat"
-- hits boss_zuramat.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). {minMs, maxMs} table delays carry the C++ Schedule /
-- Repeat randtime ranges (VOID_SHIFT uses fixed 9s/15s; SHROUD uses
-- {18s,20s} ranges).
-- creature:CastSpell(creature, spell) = DoCast(me) (2-arg =
-- non-triggered); creature:CastSpell(target, spell) = DoCast(target,
-- spell) (2-arg = non-triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDeath, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent); seeds the two combat timers below; the
-- BossAI::JustEngagedWith base arms (:76) — SetBossState(DATA_ZURAMAT
-- 8, IN_PROGRESS) via creature:SetBossState
-- (lua_creature_events.go:5917), then pulse + me-zone — ported
-- base-first below.
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- JustDied — Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent); then _JustDied() (:110) — SetBossState
-- (DATA_ZURAMAT 8, DONE), ported in zuramatJustDied; the stale
-- "_JustDied() passthrough leg has no bridge" claim is retracted.
-- boss_zuramatAI::Reset (:68-72) — Initialize() (_voidDance = true,
-- achievement latch only) + BossAI::Reset() — _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_ZURAMAT 8,
-- NOT_STARTED), != DONE-gated, ported in zuramatReset (new ev23,
-- split from ev2 — leave-combat does not call Reset in C++).
-- ScheduleTasks VOID_SHIFT — SelectTarget(Random, 0, 60.0f, true) ->
-- DoCast(target, SPELL_VOID_SHIFT 54361), 2-arg = non-triggered;
-- init 9s / Repeat 15s — unconditional re-arm. Random-player pool
-- corrected 2026-10-25: threat-list players within 60 yd (see
-- randomThreatPlayerInRange).
-- ScheduleTasks SHROUD_OF_DARKNESS — DoCast(me, SPELL_SHROUD_OF_
-- DARKNESS 54524), 2-arg = non-triggered; init {18s,20s} / Repeat
-- {18s,20s} — unconditional re-arm.
-- Cast-state gate (2026-11-04): BossAI ctor scheduler validator
-- (ScriptedCreature.cpp:493-496, !CASTING) — TaskScheduler::Dispatch
-- aborts while casting with tasks still queued, so each scheduler tick
-- below defers 500ms via creature:IsCasting()
-- (lua_creature_events.go:1950) without consuming its re-arm (the
-- mal_ganis idiom). Zuramat's own UpdateAI (:119) calls
-- scheduler.Update directly, but the validator lives on the scheduler
-- object itself, so it gates despite the BossAI::UpdateAI bypass
-- (the volazj/xevozz precedent); the unported summon leg is not a
-- scheduler tick in Lua, so nothing else gates.
-- DOCUMENTED DEVIATIONS: none on the ported arms — both timers carry
-- C++-exact timing and targeting. The UpdateAI
-- DoMeleeAttackIfReady() leg has no bridge — melee is unmodeled.
-- DOCUMENTED-ONLY (in this header; entry 29314 is the sole
-- registration):
-- SAY_SPAWN (3), SAY_SHIELD (4) and SAY_WHISPER (5) are never Talk()ed
-- anywhere in the file (dead enum text, the cyanigosa precedent).
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS,
-- DATA_ZURAMAT) leg joins the no-instance-SetData bridge queue (the
-- erekem/ichoron/lavanthor precedent).
-- The ScheduleTasks SUMMON_VOID_SENTRY leg (DoCast(me, SPELL_SUMMON_
-- VOID_SENTRY 54369), 4s init / 7-10s Repeat) stays out: it is a
-- summon leg, and casting it would spawn sentries whose boss-side
-- SummonedCreatureDespawn relay (boss_zuramat.cpp:89-96, ev21) is
-- unported in Lua. Joins the no-SummonList bridge queue. The sentry
-- class itself IS ported below (ev5 REACT_PASSIVE, ev22 IsSummonedBy
-- triggered self-cast 58650, ev19 ball latch + REACT_PASSIVE, ev20
-- prune, ev4 DespawnAll) — the old "sentry AI unbridged" claim was
-- stale; a sentry only exists in-game once the boss-side summon leg
-- lands. The boss-side SummonedCreatureDies
-- _voidDance = false flip (boss_zuramat.cpp:86-90) — PORTED tick 417
-- via event 21 (zuramatSummonedDied below); Reset writes 1
-- (Initialize, :62-66). The old "write-only" rationale is stale —
-- the SetData store is the achievement_void_dance read leg.
-- achievement_void_dance (OnCheck AI()->GetData(DATA_VOID_DANCE))
-- — PORTED tick 417 via the SetData write leg + Go gate
-- (bossLatchData, achievement_criteria_scripts.go).

local ENTRY_ZURAMAT = 29314
local DATA_ZURAMAT = 8
local DATA_VOID_DANCE = 2153 -- boss_zuramat.cpp:48 (Misc enum)
local NPC_VOID_SENTRY = 29364 -- violet_hold.h:102
local NPC_VOID_SENTRY_BALL = 29365 -- violet_hold.h:103
local SPELL_SUMMON_VOID_SENTRY_BALL = 58650 -- boss_zuramat.cpp:32

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_VOID_SHIFT = 54361
local SPELL_SHROUD_OF_DARKNESS = 54524

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

-- C++ SelectTarget(Random, 0, 60.0f, true) draws the threat list
-- (UnitAI.h:162-184, playerOnly=true), not instance players: random
-- alive threat-list player within 60y (nefarian/vaelastrasz
-- precedent; corrected 2026-10-25 — the old instance-wide pool was
-- the same bug class as moragg/erekem/xevozz, and the old
-- threat-list-vs-player-pool caveat below is now closed). Alive
-- filter: dead targets are offline (UNIT_STATE_DIED fails
-- _IsTargetAcceptable, Creature.cpp:2508) and skipped by
-- SelectTargetList, while Go SortedEntries returns dead entries
-- too. C++ IsWithinCombatRange adds both combat reaches
-- (Unit.cpp:583); player reach 1.5 (ObjectDefines.h:40),
-- reach-exact.
local function randomThreatPlayerInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks VOID_SHIFT: SelectTarget(Random, 0, 60.0f, true)
-- -> DoCast(target, 54361), 2-arg = non-triggered; init 9s / Repeat
-- 15s — re-armed unconditionally.
local function voidShiftTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "voidshift", 500, function()
            voidShiftTick(creature, guid)
        end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 60)
    if target then
        creature:CastSpell(target, SPELL_VOID_SHIFT)
    end
    schedule(guid, "voidshift", 15000, function()
        voidShiftTick(creature, guid)
    end)
end

-- C++ ScheduleTasks SHROUD_OF_DARKNESS: DoCast(me, 54524), 2-arg =
-- non-triggered; init {18s,20s} / Repeat {18s,20s} — re-armed
-- unconditionally.
local function shroudTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shroud", 500, function()
            shroudTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHROUD_OF_DARKNESS)
    schedule(guid, "shroud", {18000, 20000}, function()
        shroudTick(creature, guid)
    end)
end

-- C++ boss_zuramatAI::JustEngagedWith: Talk(SAY_AGGRO) — unconditional
-- (the auriaya engage-port precedent); the two ScheduleTasks timers
-- seed their combat timers below; the BossAI::JustEngagedWith
-- base arms (:76) ported base-first below.
local function zuramatEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- BossAI::JustEngagedWith (:76, base-first) — SetBossState
    -- (DATA_ZURAMAT 8, IN_PROGRESS) first (ScriptedCreature.cpp:527-542),
    -- then SetCombatPulseDelay(5) + me-form DoZoneInCombat.
    creature:SetBossState(DATA_ZURAMAT, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "voidshift", 9000, function()
        voidShiftTick(creature, guid)
    end)
    schedule(guid, "shroud", {18000, 20000}, function()
        shroudTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function zuramatCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function zuramatKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) — unconditional; then _JustDied()
-- (:110) — SetBossState(DATA_ZURAMAT 8, DONE)
-- (ScriptedCreature.cpp:513-520), after the death Talk per C++ order.
local function zuramatJustDied(event, creature)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_ZURAMAT, 3)
end

-- C++ boss_zuramatAI::Reset (:68-72): Initialize() (_voidDance =
-- true, achievement latch only — its only consumer is the unported
-- achievement_void_dance) + BossAI::Reset() — _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_ZURAMAT 8,
-- NOT_STARTED) only when the stored state != DONE. Split from ev2
-- (the keleseth/svala precedent): ev2 leave-combat does not call
-- Reset in C++, so it stays wipe-only.
local function zuramatReset(event, creature)
    if creature:GetBossState(DATA_ZURAMAT) ~= 3 then
        creature:SetBossState(DATA_ZURAMAT, 0)
    end
    cancelTimers(creature:GetGUID())
    -- C++ Reset -> Initialize: _voidDance = true ->
    -- GetData(DATA_VOID_DANCE) = 1 (boss_zuramat.cpp:62-66/:99-103);
    -- achievement_void_dance read leg (tick 417). Alive-gated:
    -- ev23 fires on death too (fireCreatureDied runs Reset before
    -- the kill-credit criteria read, kill.go:1091/:1400), and a
    -- death-time re-arm would clobber the fight's latch before the
    -- achievement gate reads it.
    if creature:IsAlive() then
        creature:SetData(DATA_VOID_DANCE, 1)
    end
end

-- C++ SummonedCreatureDies (boss_zuramat.cpp:86-90): a summoned Void
-- Sentry dying clears _voidDance -> GetData returns 0. Event 21 fires
-- on the summoner with (event, creature, summon, killer)
-- (lua_creature_events.go fireCreatureDied; amanitar precedent). The
-- sentry ball (29365) is summoned by the sentry, not Zuramat, so the
-- entry check also excludes it.
local function zuramatSummonedDied(event, creature, summon, killer)
    if summon:GetEntry() == NPC_VOID_SENTRY then
        creature:SetData(DATA_VOID_DANCE, 0)
    end
end

-- npc_void_sentry (boss_zuramat.cpp:164-197): ctor REACT_PASSIVE;
-- IsSummonedBy (:172-175) triggered self-cast
-- SPELL_SUMMON_VOID_SENTRY_BALL (58650); JustSummoned (:177-181)
-- latches the ball + REACT_PASSIVE; DoAction(1) (:189-192) despawns
-- all latched balls; JustDied (:194-197) fires DoAction(1). The
-- _summons list is a module-local GUID-keyed table (the halazzi latch
-- idiom); ev20 prunes balls that despawn on their own. Entry-keyed
-- dispatch (runtime.go:825-827) — no RegisterLuaBoss line needed
-- (ichor-globule / emalon-minion precedent). The sentry only exists
-- in-game once Zuramat's SUMMON_VOID_SENTRY leg lands (stays out,
-- header), so these handlers are dormant until then.
local sentryBalls = {} -- sentry GUID -> {ball creature objects}

local function sentrySpawn(event, creature)
    creature:SetReactState(0) -- REACT_PASSIVE
end

local function sentrySummonedBy(event, creature, summoner)
    creature:CastSpell(creature, SPELL_SUMMON_VOID_SENTRY_BALL, true)
end

local function sentrySummoned(event, creature, summoned)
    if summoned:GetEntry() == NPC_VOID_SENTRY_BALL then
        summoned:SetReactState(0) -- REACT_PASSIVE
        local guid = creature:GetGUID()
        sentryBalls[guid] = sentryBalls[guid] or {}
        sentryBalls[guid][#sentryBalls[guid] + 1] = summoned
    end
end

local function sentryBallDespawned(event, creature, summon)
    local balls = sentryBalls[creature:GetGUID()]
    if balls then
        for i, ball in ipairs(balls) do
            if ball == summon then
                table.remove(balls, i)
                break
            end
        end
    end
end

local function sentryDied(event, creature, killer)
    local guid = creature:GetGUID()
    local balls = sentryBalls[guid]
    if balls then
        for _, ball in ipairs(balls) do
            ball:Despawn()
        end
    end
    sentryBalls[guid] = nil
end

RegisterCreatureEvent(NPC_VOID_SENTRY, 5, sentrySpawn)
RegisterCreatureEvent(NPC_VOID_SENTRY, 22, sentrySummonedBy)
RegisterCreatureEvent(NPC_VOID_SENTRY, 19, sentrySummoned)
RegisterCreatureEvent(NPC_VOID_SENTRY, 20, sentryBallDespawned)
RegisterCreatureEvent(NPC_VOID_SENTRY, 4, sentryDied)

RegisterCreatureEvent(ENTRY_ZURAMAT, 1, zuramatEnterCombat)
RegisterCreatureEvent(ENTRY_ZURAMAT, 21, zuramatSummonedDied)
RegisterCreatureEvent(ENTRY_ZURAMAT, 2, zuramatCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ZURAMAT, 3, zuramatKilledUnit)
RegisterCreatureEvent(ENTRY_ZURAMAT, 4, zuramatJustDied)
RegisterCreatureEvent(ENTRY_ZURAMAT, 4, zuramatCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ZURAMAT, 23, zuramatReset)
