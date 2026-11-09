-- Ichoron (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_ichoron.cpp
-- (7 scripts — boss_ichoron (CreatureScript via GetVioletHoldAI
-- (BossAI), DATA_ICHORON = 5); npc_ichor_globule (zero Talk arms);
-- spell_ichoron_drained / spell_ichoron_merge /
-- spell_ichoron_protective_bubble / spell_ichoron_splatter
-- (SpellScriptLoader Spell/AuraScripts); achievement_dehydration;
-- all registered from inside AddSC_boss_ichoron() — loader decl 147
-- / call 342, the "// Violet Hold" block in AddNorthrendScripts(),
-- northrend_script_loader.cpp, after AddSC_boss_erekem() (341);
-- AddSC_boss_lavanthor() (343) follows).
-- Entry: 29313 Ichoron (violet_hold.h NPC_ICHORON, line 98) —
-- entry-verifiable, registration proceeds; the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_ICHORON = 5
-- (violet_hold.h line 55).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). {minMs, maxMs} table delays carry the C++ Schedule /
-- Repeat randtime ranges (the toravon randtime convention).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / DoCast(me) (2-arg = non-triggered);
-- creature:CastSpell(target, spell) = DoCast(target, spell)
-- (2-arg = non-triggered); creature:CastSpell(creature, spell, true)
-- = DoCast(me, spell, true) (triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDeath, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_ichoron.cpp):
-- boss_ichoronAI::Reset (:98-101) — DoCast(me, SPELL_THREAT_PROC
-- 61732, true) (:100, triggered) -> CastSpell(creature, 61732, true);
-- fires on event 23 (evade/reset — and on spawn too, since
-- fireCreatureSpawned dispatches 23 then 5 on the spawn funnel,
-- creaturemotion.go:1262 + lua_creature_events.go:7264; the old
-- "no spawn-event bridge" note was stale); the BossAI::Reset
-- passthrough leg — _Reset() (ScriptedCreature.cpp:499-511) —
-- SetBossState(DATA_ICHORON 5, NOT_STARTED), != DONE-gated, via
-- creature:SetBossState (lua_creature_events.go:6419) — is ported in
-- ichoronReset BEFORE the threat-proc cast, per the C++ Reset order
-- (Initialize :95-97, BossAI::Reset :98, DoCast :100).
-- boss_ichoronAI::JustEngagedWith (:103-107) — Talk(SAY_AGGRO 0)
-- (:106) kept; seeds the three combat timers below (the
-- BossAI::JustEngagedWith base arms (:105) — SetBossState
-- (DATA_ICHORON 5, IN_PROGRESS) via creature:SetBossState
-- (lua_creature_events.go:5917), then pulse + me-zone — ported
-- base-first below).
-- ScheduleTasks water-bolt-volley leg (:213-217) — DoCastAOE
-- (SPELL_WATER_BOLT_VOLLEY 54241) (:215, 2-arg = non-triggered) =
-- self-cast per the in-tree DoCastAOE convention ->
-- CastSpell(creature, 54241); init {10000, 15000}, re-arm
-- {10000, 15000}.
-- ScheduleTasks water-blast leg (:219-224) — SelectTarget(Random, 0,
-- 50.0f) (:221) -> DoCast(target, SPELL_WATER_BLAST 54237) (:222,
-- 2-arg = non-triggered) -> CastSpell(target, 54237); init
-- {6000, 9000}, re-arm {6000, 9000}.
-- KilledUnit (:162-165) — if (victim->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY 1) — the razuvious player-gated variant precedent.
-- JustDied (:167-171) — Talk(SAY_DEATH 2) — the sjonnir
-- JustDied-Talk precedent; then _JustDied() (:170) — SetBossState
-- (DATA_ICHORON 5, DONE) via creature:SetBossState
-- (lua_creature_events.go:5917), ported in ichoronJustDied; the stale
-- "_JustDied() passthrough leg has no bridge" claim is retracted.
-- UpdateAI frenzy leg (:194-198) — !_isFrenzy && HealthBelowPct(25) +
-- no DRAINED aura -> Talk(SAY_ENRAGE 4) + DoCast(me, SPELL_FRENZY
-- 54312, true), one-flip _isFrenzy latch. The Oct-era "no
-- health-threshold bridge" claim is RETRACTED: creature:GetHealthPct()
-- is bridged via the generic object fallback (engine/scripting/
-- object.go:107 — elder_nadox retraction precedent) and HealthBelowPct
-- (25) strict-< is C++-exact as GetHealthPct() < 25; creature:HasAura
-- (59820) is bridged (engine/world/lua_objects.go:228). The per-tick
-- check rides a 1s re-armed CreateLuaEvent poll seeded on combat start
-- (the sindragosa 1s third-phase-check convention), latch table
-- frenzyFired[], wiped on events 2/4/23; the poll stops re-arming
-- once the latch fires.
-- DOCUMENTED DEVIATIONS:
-- The water-blast pool was corrected 2026-10-25: threat-list
-- any-unit alive within 50y (see randomThreatTargetInRange); the old
-- instance-player pool was player-only and distance-blind.
-- The UpdateAI HasUnitState(UNIT_STATE_CASTING) pre-check has no
-- bridge — casts fire unconditionally on their timers (the hodir
-- precedent).
-- DOCUMENTED-ONLY (in this header; entry 29313):
-- SAY_SPAWN (3) is never Talk()ed (dead enum text, the cyanigosa
-- precedent).
-- (Talk(SAY_ENRAGE 4) frenzy leg was documented-only in the Oct
-- sweep; RETRACTED — ported above per the health-threshold retraction.)
-- Talk(SAY_SHATTER 5) + Talk(EMOTE_SHATTER 7) ride the DoAction
-- ACTION_PROTECTIVE_BUBBLE_SHATTERED leg (:127-140: AOE splatter /
-- burst, DRAINED cast, 30% max-health damage leg, scheduler.DelayAll)
-- — PORTED 2026-11-08 (tick 485) as the ev38 receiver below; the
-- sender (spell_ichoron_protective_bubble shatter hook, :406) is
-- ported alongside it as a RegisterAuraEvent remove hook.
-- Talk(SAY_BUBBLE 6) rides the DoAction ACTION_DRAINED leg (:141-149:
-- HealthAbovePct(30) -> protective bubble recast, triggered) —
-- PORTED 2026-11-08 (tick 485) as the ev38 receiver below; the
-- sender (spell_ichoron_drained expire, :332) is ported alongside it
-- as a RegisterAuraEvent remove hook.
-- ACTION_WATER_GLOBULE_HIT (3% max-health heal, _dehydration = false)
-- — PORTED 2026-11-08 (tick 485) as the ev38 receiver below; the
-- sender stays out (spell_ichoron_merge SpellScript, :370 — no
-- SpellScript bridge), so the receiver is dormant until the merge
-- leg lands (the thorim ACTION_BERSERK dormant-receiver precedent).
-- The ScheduleTasks Async leg (:207-211: SHRINK 54297 +
-- PROTECTIVE_BUBBLE 54306 self-casts) seeds the bubble phase machine
-- — PORTED 2026-11-08 (tick 485) as engage casts in
-- ichoronEnterCombat: with the shatter hook + SHATTERED receiver
-- ported, the bubble now shatters, so the gothik-precedent stay-out
-- no longer applies.
-- The npc_ichor_globule SpellHit machine — stale "no SpellHit bridge" retracted (2026-10-21): ev14 IS bridged
-- (fires from the Go spell path, boss_ai.go:2125/2134 + pet_combat.go:5222 + spells.go:11765). PORTED 2026-11-08
-- (tick 485): the VISUAL 54260 trigger rides Ichoron's JustSummoned (:172-178); the arm's legs are now all
-- bridged — MoveFollow (lua_creature_events.go:5105), MovementInform FOLLOW_MOTION_TYPE (14) on ev28 with the
-- followed target's GUID counter as id (creaturemotion.go:4406-4410), the instance DATA_ICHORON object-GUID
-- read (creature:GetGuidData, lua_creature_events.go:6472 — Ichoron latches its own GUID in ichoronReset),
-- SPELL_MERGE 54269 self-cast + DespawnOrUnsummon(1ms) (lua_creature_events.go:6637). The merge SpellScript
-- itself (shrink stack -1 + DoAction WATER_GLOBULE_HIT) stays out — no SpellScript bridge — so the merge cast
-- is a dormant trigger and the heal/dehydration legs ride the ported ev38 receiver. The DamageTaken SPLASH
-- 59516 arm IS bridged (event 9, registered 2026-10-19 — the stale "no DamageTaken bridges" claim retracted).
-- The npc_ichor_globule ctor SetReactState(REACT_PASSIVE)
-- (boss_ichoron.cpp:248) stays out: globules spawn only from the unported
-- spell_ichoron_splatter AuraScript, and this runtime's event-5 spawn hook
-- never fires for temp summons — no construction-time hook (halion
-- precedent).
-- spell_ichoron_drained (59820 AuraScript) — PORTED 2026-11-08 (tick 485) via RegisterAuraEvent: apply (mode 2)
-- sets NOT_SELECTABLE + FEIGN_DEATH flags (:322-326), remove (mode 3) clears them and, on
-- AURA_REMOVE_BY_EXPIRE (4), fires DoAction ACTION_DRAINED (:332). Load() gate (owner entry 29313 or 32234,
-- NPC_DUMMY_ICHORON violet_hold.h:110) rides the target's GetEntry.
-- spell_ichoron_protective_bubble (54306 AuraScript) — PORTED 2026-11-08 (tick 485) via RegisterAuraEvent
-- remove (mode 3): charges <= 1 -> DoAction ACTION_PROTECTIVE_BUBBLE_SHATTERED (:406). The charges read rides
-- aura:GetStackAmount() (lua_aura.go:226 — auraWireStackCount returns RemainingCharges when the DBC StackAmount
-- is 0, dispel.go:995-1005); the bubble is charge-driven per C++ (GetCharges gate), so a DBC StackAmount > 0
-- would misread the gate — documented assumption, same class as the other DBC-dependent ports.
-- spell_ichoron_splatter (54259 AuraScript) — PORTED 2026-11-08 (tick 485) via RegisterAuraEvent: periodic
-- (mode 1) casts a random globule-summon spell (54258/54264/54265/54266/54267) triggered on the target and
-- returns true = PreventDefaultAction (:447-452); remove (mode 3) on EXPIRE adds 10 shrink stacks (:458-461)
-- via creature:GetAura + Aura:SetStackAmount.
-- spell_ichoron_merge (54269 SpellScript: shrink stack -1 + DoAction ACTION_WATER_GLOBULE_HIT :370) — no
-- SpellScript bridge, stays out.
-- achievement_dehydration (OnCheck AI()->GetData(DATA_DEHYDRATION))
-- — PORTED 2026-11-08 (tick 419): ev1 write leg below (Reset twin);
-- the Go case reads the boss motion's SetData slot. Its only clear is
-- the spell_ichoron_merge DoAction leg (unported globule chain), so
-- the latch stays 1 in the Go-modeled fight.
-- The JustReachedHome leg (boss_ichoron.cpp:109-113) is PORTED tick
-- 486 via ev24 + FindNearestGameObject(GO_ICHORON_DOOR 191722, 50)
-- + SetGoState(1) (HandleCells(5, false), InstanceScript.cpp:479-487);
-- the instance-SetData bridge itself stays unported.

local ENTRY_ICHORON = 29313
local DATA_ICHORON = 5
local GO_ICHORON_DOOR = 191722 -- violet_hold.h:129
local DATA_DEHYDRATION = 1 -- boss_ichoron.cpp:71
local ENTRY_ICHOR_GLOBULE = 29321
local SPELL_SPLASH = 59516

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2
local SAY_ENRAGE = 4

local SPELL_WATER_BLAST = 54237
local SPELL_WATER_GLOBULE_VISUAL = 54260
local SPELL_WATER_GLOBULE_TRANSFORM = 54268
local UNIT_FIELD_FLAGS = 59
local UNIT_FIELD_FLAGS_2 = 60
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_FLAG2_FEIGN_DEATH = 0x00000001
local SPELL_WATER_BOLT_VOLLEY = 54241
local SPELL_THREAT_PROC = 61732
local SPELL_FRENZY = 54312
local SPELL_DRAINED = 59820
local SPELL_SHRINK = 54297
local SPELL_PROTECTIVE_BUBBLE = 54306
local SPELL_SPLATTER = 54259
local SPELL_BURST = 54379
local SPELL_MERGE = 54269
local SPELL_WATER_GLOBULE_SUMMON_1 = 54258
local SPELL_WATER_GLOBULE_SUMMON_2 = 54264
local SPELL_WATER_GLOBULE_SUMMON_3 = 54265
local SPELL_WATER_GLOBULE_SUMMON_4 = 54266
local SPELL_WATER_GLOBULE_SUMMON_5 = 54267

local SAY_SHATTER = 5
local SAY_BUBBLE = 6
local EMOTE_SHATTER = 7

local ACTION_WATER_GLOBULE_HIT = 1
local ACTION_PROTECTIVE_BUBBLE_SHATTERED = 2
local ACTION_DRAINED = 3

local NPC_DUMMY_ICHORON = 32234
local AURA_REMOVE_BY_EXPIRE = 4
local FOLLOW_MOTION_TYPE = 14

local timers = {}
local timerFns = {}
local timerDelays = {}

-- C++ _isFrenzy latch (:185): one-flip Talk(SAY_ENRAGE 4) + FRENZY
-- (54312) cast guard.
local frenzyFired = {}

-- C++ BossAI summons set (boss_ichoron.cpp:181-186): SummonedCreatureDespawn
-- drops the summon, then strips SPELL_DRAINED when the set empties.
local ichoronSummons = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
        timerFns[guid] = nil
        timerDelays[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
        timerFns[guid] = {}
        timerDelays[guid] = {}
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
    timerFns[guid][key] = fn
    timerDelays[guid][key] = delay
end

-- C++ scheduler.DelayAll (boss_ichoron.cpp:139, SHATTERED leg): every
-- pending scheduler task fires delayMs later. The Lua mirror
-- re-seeds each pending timer with its last-used delay + delayMs
-- (the last-used delay, not the true remaining time — documented
-- approximation; randtime re-arms already carry that shape). Keys in
-- skip stay on their own clock: the frenzy poll models the
-- pre-scheduler UpdateAI health check, which DelayAll never touches.
local function delayAllTimers(guid, delayMs, skip)
    local per = timers[guid]
    if not per then
        return
    end
    for key, id in pairs(per) do
        if not (skip and skip[key]) then
            local fn = timerFns[guid] and timerFns[guid][key]
            local delay = timerDelays[guid] and timerDelays[guid][key]
            if fn and delay then
                RemoveEventById(id)
                local extra = delayMs
                if type(delay) == "table" then
                    schedule(guid, key, {delay[1] + extra, delay[2] + extra}, fn)
                else
                    schedule(guid, key, delay + extra, fn)
                end
            end
        end
    end
end

-- Cast-state gate (2026-11-04): BossAI ctor scheduler validator
-- (ScriptedCreature.cpp:493-496, !CASTING) — TaskScheduler::Dispatch
-- aborts while casting with tasks still queued, so each scheduler tick
-- below defers 500ms via creature:IsCasting()
-- (lua_creature_events.go:1950) without consuming its re-arm (the
-- mal_ganis idiom); volley/blast scheduler ticks carry the gate; the frenzy 1s poll models the pre-scheduler UpdateAI health check, ungated per C++; the shrink/bubble Async casts fire at engage, ungated per C++.

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 50.0f) — no
-- playerOnly flag, so any threat-listed unit within 50y qualifies
-- (UnitAI.h:170, playerOnly=false default); C++ IsWithinCombatRange
-- adds both combat reaches (Unit.cpp:583); Lua subtracts own reach
-- + target reach (1.5 if Player else target:GetCombatReach(), hadronox idiom). Alive filter: dead targets are offline
-- (UNIT_STATE_DIED fails _IsTargetAcceptable, Creature.cpp:2508)
-- and skipped by SelectTargetList, while Go SortedEntries returns
-- dead entries too (erekem/pandemonius precedent).
local function randomThreatTargetInRange(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        if t:IsAlive() and creature:GetDistance(t) - creature:GetCombatReach() - targetReach < maxDist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks water-bolt-volley leg (:213-217):
-- DoCastAOE(SPELL_WATER_BOLT_VOLLEY), 2-arg = non-triggered;
-- init 10-15s / re-arm 10-15s.
local function waterBoltVolleyTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "volley", 500, function()
            waterBoltVolleyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_WATER_BOLT_VOLLEY)
    schedule(guid, "volley", {10000, 15000}, function()
        waterBoltVolleyTick(creature, guid)
    end)
end

-- C++ ScheduleTasks water-blast leg (:219-224): DoCast(target,
-- SPELL_WATER_BLAST), 2-arg = non-triggered; init 6-9s / re-arm 6-9s.
local function waterBlastTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "blast", 500, function()
            waterBlastTick(creature, guid)
        end)
        return
    end
    local target = randomThreatTargetInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_WATER_BLAST)
    end
    schedule(guid, "blast", {6000, 9000}, function()
        waterBlastTick(creature, guid)
    end)
end

-- C++ UpdateAI frenzy leg (:194-198): !_isFrenzy && HealthBelowPct(25)
-- && !HasAura(SPELL_DRAINED 59820) -> Talk(SAY_ENRAGE 4), DoCast(me,
-- SPELL_FRENZY 54312, true), latch. HealthBelowPct(25) is strict-< the
-- floored integer threshold (the old GetHealthPct() < 25 fired up to 1
-- HP early — maulgar/ironaya precedent). The per-tick
-- UpdateAI check rides a 1s re-armed CreateLuaEvent poll seeded on
-- combat start, wiped on 2/4/23 (the sindragosa third-phase-check
-- 1s convention).
local function frenzyTick(creature, guid)
    if not frenzyFired[guid] then
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 25 / 100)
                and not creature:HasAura(SPELL_DRAINED) then
            creature:Talk(SAY_ENRAGE)
            creature:CastSpell(creature, SPELL_FRENZY, true)
            frenzyFired[guid] = true
        end
    end
    if not frenzyFired[guid] then
        schedule(guid, "frenzy", 1000, function()
            frenzyTick(creature, guid)
        end)
    end
end

-- C++ boss_ichoronAI::JustEngagedWith (:103-107): base arms (:105)
-- ported base-first below, then Talk(SAY_AGGRO); seeds the combat timers.
local function ichoronEnterCombat(event, creature, target)
    -- BossAI::JustEngagedWith (:105, base-first) — SetBossState
    -- (DATA_ICHORON 5, IN_PROGRESS) first (ScriptedCreature.cpp:527-542),
    -- then SetCombatPulseDelay(5) + me-form DoZoneInCombat.
    creature:SetBossState(DATA_ICHORON, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    -- C++ ScheduleTasks Async leg (:207-211): DoCast(me, SHRINK) +
    -- DoCast(me, PROTECTIVE_BUBBLE), both non-triggered self-casts,
    -- seeding the bubble phase machine.
    creature:CastSpell(creature, SPELL_SHRINK)
    creature:CastSpell(creature, SPELL_PROTECTIVE_BUBBLE)
    -- C++ Reset -> Initialize (boss_ichoron.cpp:92-97): _dehydration
    -- = true. Written at engage, not Reset: ev23 fires on death too,
    -- ahead of the kill-credit criteria read, so an ev23 write would
    -- clobber a future mid-fight clear; ev1 fires only at engage.
    -- The only clear is DoAction ACTION_WATER_GLOBULE_HIT (:119-125),
    -- called only from spell_ichoron_merge (:366-371) behind the
    -- unported globule chain, so the latch stays 1 in the Go-modeled
    -- fight. PORTED 2026-11-08 (tick 419).
    creature:SetData(DATA_DEHYDRATION, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frenzyFired[guid] = nil
    schedule(guid, "volley", {10000, 15000}, function()
        waterBoltVolleyTick(creature, guid)
    end)
    schedule(guid, "blast", {6000, 9000}, function()
        waterBlastTick(creature, guid)
    end)
    schedule(guid, "frenzy", 1000, function()
        frenzyTick(creature, guid)
    end)
end

-- C++ boss_ichoronAI::Reset (:98-101): BossAI::Reset clears the
-- scheduler (mirrored by cancelTimers); the _Reset() state leg —
-- SetBossState(DATA_ICHORON 5, NOT_STARTED), != DONE-gated — lands
-- BEFORE the threat-proc cast per the C++ Reset order (Initialize,
-- BossAI::Reset, DoCast); DoCast(me, SPELL_THREAT_PROC, true) —
-- triggered self-cast.
local function ichoronReset(event, creature)
    local guid = creature:GetGUIDLow()
    for key in pairs(ichoronSummons) do
        ichoronSummons[key] = nil
    end
    cancelTimers(guid)
    frenzyFired[guid] = nil
    -- Latch Ichoron's GUID under DATA_ICHORON for the globule merge
    -- gate (boss_ichoron.cpp:261-263 reads the instance
    -- DATA_ICHORON object GUID). ev23 fires on spawn as well as
    -- evade (creaturemotion.go:1262 + lua_creature_events.go:7264), so the latch lands at
    -- spawn; re-latching the same GUID on evade is a no-op.
    creature:SetGuidData(DATA_ICHORON)
    if creature:GetBossState(DATA_ICHORON) ~= 3 then
        creature:SetBossState(DATA_ICHORON, 0)
    end
    -- C++ Reset(): DoCast(me, SPELL_THREAT_PROC, true) is Reset-only
    -- (boss_ichoron.cpp). Alive-gated; the timer cancel rides the death
    -- leg deliberately.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_THREAT_PROC, true)
    end
end

-- Shared timer wipe for events 2/4.
local function ichoronCancelCombatTimers(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frenzyFired[guid] = nil
end

-- C++ KilledUnit (:162-165): player-gated Talk(SAY_SLAY) — the
-- razuvious player-gated variant precedent.
local function ichoronKilledUnit(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:167-171): Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent; then _JustDied() (:170) — SetBossState
-- (DATA_ICHORON 5, DONE) (ScriptedCreature.cpp:513-520), after the
-- death Talk per C++ order.
local function ichoronJustDied(event, creature)
    creature:Talk(SAY_DEATH)
    creature:SetBossState(DATA_ICHORON, 3)
end

-- C++ JustSummoned (boss_ichoron.cpp:173-179): ichor-globule entry gate
-- -> DoCast(summon, SPELL_WATER_GLOBULE_VISUAL 54260), non-triggered,
-- cast BY ichoron ON the summon (corrected 2026-10-25 from a summon
-- self-cast; the globule SpellHit arm only gates on spellId, so the
-- old form fired, but with the wrong caster);
-- the summons.Summon bookkeeping leg has no Lua twin. LATENT: globules
-- spawn only from the unported spell_ichoron_splatter AuraScript
-- periodic summon -- the arm fires correctly if a future path summons
-- via ichoron (anubrekhan latent precedent).
local function ichoronJustSummoned(_, creature, summoned)
    ichoronSummons[summoned:GetGUIDLow()] = true
    if summoned:GetEntry() == ENTRY_ICHOR_GLOBULE then
        creature:CastSpell(summoned, SPELL_WATER_GLOBULE_VISUAL)
    end
end

-- C++ boss_ichoronAI::JustReachedHome (:109-113): BossAI base +
-- instance->SetData(DATA_HANDLE_CELLS, DATA_ICHORON) ->
-- HandleCells(5, false) -> HandleGameObject(GO_ICHORON_DOOR, false)
-- -> GO_STATE_READY (1). Boss-side mirror via ev24.
local function ichoronReachedHome(event, creature)
    local door = creature:FindNearestGameObject(GO_ICHORON_DOOR, 50)
    if door then
        door:SetGoState(1)
    end
end

RegisterCreatureEvent(ENTRY_ICHORON, 1, ichoronEnterCombat)
RegisterCreatureEvent(ENTRY_ICHORON, 2, ichoronCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ICHORON, 3, ichoronKilledUnit)
RegisterCreatureEvent(ENTRY_ICHORON, 4, ichoronJustDied)
RegisterCreatureEvent(ENTRY_ICHORON, 4, ichoronCancelCombatTimers)
RegisterCreatureEvent(ENTRY_ICHORON, 23, ichoronReset)
RegisterCreatureEvent(ENTRY_ICHORON, 24, ichoronReachedHome)
RegisterCreatureEvent(ENTRY_ICHORON, 19, ichoronJustSummoned)

-- C++ SummonedCreatureDespawn (boss_ichoron.cpp:181-186): BossAI drops the
-- summon, then if summons.empty() -> me->RemoveAurasDueToSpell(SPELL_DRAINED).
local function ichoronSummonDespawned(_, creature, summon)
    ichoronSummons[summon:GetGUIDLow()] = nil
    if next(ichoronSummons) == nil then
        creature:RemoveAura(SPELL_DRAINED)
    end
end

RegisterCreatureEvent(ENTRY_ICHORON, 20, ichoronSummonDespawned)

-- C++ boss_ichoronAI::DoAction (boss_ichoron.cpp:110-151), ev38
-- dispatch (creature:DoAction, lua_creature_events.go:802). The
-- SHATTERED and DRAINED senders are the ported aura hooks below;
-- the WATER_GLOBULE_HIT sender (spell_ichoron_merge SpellScript)
-- stays out, so action 1 is dormant until the merge leg lands.
local function ichoronDoAction(event, creature, action)
    if action == ACTION_WATER_GLOBULE_HIT then
        -- :119-125: !IsAlive break, +3% max-health heal,
        -- _dehydration = false. ModifyHealth rides SetHealth
        -- (lua_creature_events.go:2598, clamps to max like C++
        -- Unit::SetHealth); CountPctFromMaxHealth floors (max*3/100).
        if not creature:IsAlive() then
            return
        end
        local maxHealth = creature:GetMaxHealth()
        local heal = math.floor(maxHealth * 3 / 100)
        creature:SetHealth(math.min(maxHealth, creature:GetHealth() + heal))
        creature:SetData(DATA_DEHYDRATION, 0)
    elseif action == ACTION_PROTECTIVE_BUBBLE_SHATTERED then
        -- :127-140: Talk SAY_SHATTER + EMOTE_SHATTER, DoCastAOE
        -- SPLATTER + BURST triggered, DoCast(me, DRAINED, true),
        -- 30% max-health damage leg, scheduler.DelayAll(15s).
        -- LowerPlayerDamageReq has no bridge and stays out
        -- (documented deviation). The damage leg never kills:
        -- health - min(damage, health - 1) == max(1, health - damage).
        creature:Talk(SAY_SHATTER)
        creature:Talk(EMOTE_SHATTER)
        creature:CastSpell(creature, SPELL_SPLATTER, true)
        creature:CastSpell(creature, SPELL_BURST, true)
        creature:CastSpell(creature, SPELL_DRAINED, true)
        local maxHealth = creature:GetMaxHealth()
        local damage = math.floor(maxHealth * 30 / 100)
        creature:SetHealth(math.max(1, creature:GetHealth() - damage))
        delayAllTimers(creature:GetGUIDLow(), 15000, { frenzy = true })
    elseif action == ACTION_DRAINED then
        -- :141-149: HealthAbovePct(30) (strict > the floored 30%
        -- threshold, the frenzy strict-< twin) -> Talk SAY_BUBBLE +
        -- DoCast(me, PROTECTIVE_BUBBLE, true).
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0 and creature:GetHealth() > math.floor(maxHealth * 30 / 100) then
            creature:Talk(SAY_BUBBLE)
            creature:CastSpell(creature, SPELL_PROTECTIVE_BUBBLE, true)
        end
    end
end

RegisterCreatureEvent(ENTRY_ICHORON, 38, ichoronDoAction)

-- C++ spell_ichoron_drained AuraScript (boss_ichoron.cpp:305-347),
-- via RegisterAuraEvent (aura events 1-7, runtime.go:497-503;
-- creature-side fire sites spells.go:39194/:44661/:44753,
-- boss_ai.go:9332). Handlers receive (event, aura, target); the
-- Load() owner-entry gate (29313 or 32234) rides target:GetEntry().
-- AURA_REMOVE_BY_EXPIRE = 4 (lua_aura.go:246 comment: NONE 0,
-- DEFAULT 1, CANCEL 2, ENEMY_SPELL 3, EXPIRE 4, DEATH 5).
local function ichoronAuraOwner(target)
    return target ~= nil and (target:GetEntry() == ENTRY_ICHORON
        or target:GetEntry() == NPC_DUMMY_ICHORON)
end

-- :322-326: apply sets NOT_SELECTABLE + FEIGN_DEATH flags.
-- UNIT_FIELD_FLAGS = 59, UNIT_FIELD_FLAGS_2 = 60 (lua_player.go:859);
-- UNIT_FLAG2_FEIGN_DEATH = 0x00000001 (UnitDefines.h:161).
local function drainedAuraApply(event, aura, target)
    if not ichoronAuraOwner(target) then
        return
    end
    target:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    target:SetFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_FEIGN_DEATH)
end

-- :328-339: remove clears both flags; on EXPIRE the target AI gets
-- DoAction(ACTION_DRAINED).
local function drainedAuraRemove(event, aura, target)
    if not ichoronAuraOwner(target) then
        return
    end
    target:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    target:RemoveFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_FEIGN_DEATH)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_EXPIRE then
        target:DoAction(ACTION_DRAINED)
    end
end

-- C++ spell_ichoron_protective_bubble AuraScript (:387-420): on
-- remove, charges <= 1 -> DoAction
-- ACTION_PROTECTIVE_BUBBLE_SHATTERED. The C++ commented-out
-- remove-mode gate stays commented out (any remove mode fires it).
-- Charges ride aura:GetStackAmount() (see header note).
local function bubbleAuraRemove(event, aura, target)
    if not ichoronAuraOwner(target) then
        return
    end
    if aura:GetStackAmount() <= 1 then
        target:DoAction(ACTION_PROTECTIVE_BUBBLE_SHATTERED)
    end
end

-- C++ spell_ichoron_splatter AuraScript (:422-470): periodic tick
-- casts a random globule-summon spell triggered on the target and
-- PreventDefaultAction()s (:447-452); on EXPIRE the SHRINK aura
-- gains 10 stacks (:458-461) via creature:GetAura +
-- Aura:SetStackAmount (ModStackAmount twin; nil-guarded like the
-- C++ GetAura null check).
local SPLATTER_SUMMONS = {
    SPELL_WATER_GLOBULE_SUMMON_1,
    SPELL_WATER_GLOBULE_SUMMON_2,
    SPELL_WATER_GLOBULE_SUMMON_3,
    SPELL_WATER_GLOBULE_SUMMON_4,
    SPELL_WATER_GLOBULE_SUMMON_5,
}

local function splatterAuraPeriodic(event, aura, target)
    if target == nil then
        return true
    end
    target:CastSpell(target, SPLATTER_SUMMONS[math.random(#SPLATTER_SUMMONS)], true)
    return true
end

local function splatterAuraRemove(event, aura, target)
    if target == nil or aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local shrink = target:GetAura(SPELL_SHRINK)
    if shrink then
        shrink:SetStackAmount(shrink:GetStackAmount() + 10)
    end
end

RegisterAuraEvent(SPELL_DRAINED, 2, drainedAuraApply)
RegisterAuraEvent(SPELL_DRAINED, 3, drainedAuraRemove)
RegisterAuraEvent(SPELL_PROTECTIVE_BUBBLE, 3, bubbleAuraRemove)
RegisterAuraEvent(SPELL_SPLATTER, 1, splatterAuraPeriodic)
RegisterAuraEvent(SPELL_SPLATTER, 3, splatterAuraRemove)

-- C++ npc_ichor_globule::DamageTaken (boss_ichoron.cpp:279-290):
-- if (!_splashTriggered && damage >= me->GetHealth()): latch,
-- DoCastAOE(SPELL_SPLASH 59516). DoCastAOE is self-cast per
-- UnitAI.h:322, so creature:CastSpell(creature, 59516). C++ uses the
-- direct lethal comparison and lets the damage through (the
-- keep-at-1hp retail ideal is unimplemented in C++) — no
-- postDamage guard. The latch resets only in the AI ctor, so the Lua
-- mirror wipes the GUID entry on 2/4/23. Inert today: globules spawn
-- from the unported spell_ichoron_splatter AuraScript.
local ichorSplashLatch = {}

local function ichorGlobuleDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    if ichorSplashLatch[guid] then
        return false, damage
    end
    if damage >= creature:GetHealth() then
        ichorSplashLatch[guid] = true
        creature:CastSpell(creature, SPELL_SPLASH)
    end
    return false, damage
end

local function ichorGlobuleCleanup(event, creature)
    ichorSplashLatch[creature:GetGUIDLow()] = nil
end

-- C++ npc_ichor_globuleAI::SpellHit (boss_ichoron.cpp:253-264):
-- caster->ToUnit() gate + spellId == WATER_GLOBULE_VISUAL ->
-- DoCast(me, WATER_GLOBULE_TRANSFORM) + RemoveFlag NOT_SELECTABLE.
-- The MoveFollow(unitCaster) leg stays out (no motion bridge).
-- Inert today like the globule DamageTaken port: globules spawn
-- from the unported spell_ichoron_splatter AuraScript.
local function ichorGlobuleSpellHit(event, creature, caster, spellId)
    if caster == nil or spellId ~= SPELL_WATER_GLOBULE_VISUAL then
        return
    end
    creature:CastSpell(creature, SPELL_WATER_GLOBULE_TRANSFORM)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    -- :261-263: MoveFollow(unitCaster, 0.0f, 0.0f); the merge leg
    -- rides the ev28 follow inform below.
    creature:MoveFollow(caster, 0, 0)
end

-- C++ npc_ichor_globule::MovementInform (boss_ichoron.cpp:266-274):
-- FOLLOW_MOTION_TYPE only, and only when the followed target is
-- Ichoron — the instance DATA_ICHORON object GUID's counter
-- (GetCounter() = low 32 bits) must equal the inform id. Go passes
-- uint32(targetGUID) as the id (creaturemotion.go:4406-4410), so the
-- Lua mirror compares against GetGuidData(DATA_ICHORON) % 2^32
-- (GUIDs stay below 2^53, so the modulo is exact). On match:
-- CastSpell(me, SPELL_MERGE) + DespawnOrUnsummon(1ms).
local function ichorGlobuleMovementInform(event, creature, type, id)
    if type ~= FOLLOW_MOTION_TYPE then
        return
    end
    if id ~= creature:GetGuidData(DATA_ICHORON) % 4294967296 then
        return
    end
    creature:CastSpell(creature, SPELL_MERGE)
    creature:DespawnOrUnsummon(1)
end

RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 2, ichorGlobuleCleanup)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 4, ichorGlobuleCleanup)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 9, ichorGlobuleDamageTaken)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 14, ichorGlobuleSpellHit)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 23, ichorGlobuleCleanup)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 28, ichorGlobuleMovementInform)
RegisterCreatureEvent(ENTRY_ICHOR_GLOBULE, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_ichoron.cpp:291: empty UpdateAI in C++ (never melees)
end)
