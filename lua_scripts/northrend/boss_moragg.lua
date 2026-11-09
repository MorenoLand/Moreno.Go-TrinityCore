-- Moragg (Violet Hold) — Lua port of
-- src/server/scripts/Northrend/VioletHold/boss_moragg.cpp
-- (3 scripts — boss_moragg (CreatureScript via GetVioletHoldAI
-- (BossAI), DATA_MORAGG = 3); spell_moragg_ray / spell_moragg_optic_link
-- (SpellScriptLoader AuraScripts); all registered from inside
-- AddSC_boss_moragg() — loader decl 149 / call 344, the "// Violet
-- Hold" block in AddNorthrendScripts(), northrend_script_loader.cpp,
-- after AddSC_boss_lavanthor() (343); AddSC_boss_xevozz() (345)
-- follows).
-- Entry: 29316 Moragg (violet_hold.h NPC_MORAGG, line 106) —
-- entry-verifiable, registration proceeds; the CreatureScript
-- ScriptName binding is DB-side as usual. DATA_MORAGG = 3
-- (violet_hold.h line 53).
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade). C++ Schedule(init) literals become plain ms delays.
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCast(me, spell) (2-arg = non-triggered);
-- creature:CastSpell(target, spell) = DoCast(target, spell) (2-arg =
-- non-triggered).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Zero Talk lines in the C++ — none ported.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_moragg.cpp):
-- boss_moraggAI::JustEngagedWith (:52-55) — base arms (:54) —
-- SetBossState(DATA_MORAGG 3, IN_PROGRESS) via creature:SetBossState
-- (lua_creature_events.go:5917), then pulse + me-zone — ported
-- base-first below; seeds the combat timers.
-- BossAI::JustDied (no override in the file) — _JustDied()
-- (ScriptedCreature.cpp:513-520) — SetBossState(DATA_MORAGG 3,
-- DONE), ported in moraggJustDied (new ev4).
-- BossAI::Reset (:47-50, base only) — _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_MORAGG 3,
-- NOT_STARTED), != DONE-gated, ported in moraggReset (new ev23,
-- split from ev2 — leave-combat does not call Reset in C++).
-- ScheduleTasks Async leg (:74-78) — DoCast(me, SPELL_RAY_OF_PAIN
-- 54438) + DoCast(me, SPELL_RAY_OF_SUFFERING 54442), both 2-arg =
-- non-triggered — the two immediate self-casts fire once at combat
-- start before the timers (the toravon JustEngagedWith frozen-mallet
-- precedent).
-- ScheduleTasks optic-link leg (:80-85) — SelectTarget(Random, 0,
-- 50.0f, true) (player-only) -> DoCast(target, SPELL_OPTIC_LINK
-- 54396) (:83, 2-arg = non-triggered) -> CastSpell(target, 54396);
-- init 15s, re-arm 25s.
-- ScheduleTasks corrosive-saliva leg (:87-91) — DoCastVictim
-- (SPELL_CORROSIVE_SALIVA 54527) (:89, 1-arg = non-triggered) ->
-- CastSpell(nil, 54527); init 5s, re-arm 10s.
-- DOCUMENTED DEVIATIONS:
-- Optic-link pool corrected 2026-10-25: threat-list players, not
-- instance players (see randomPlayerInRange).
-- (retracted 2026-10-17: the "no distance bridge" claim was stale —
-- creature:IsWithinDist is bridged (engine/scripting/object.go:167,
-- the maiden / kiljaeden / marwyn precedent); the optic-link pool is
-- now filtered at the C++ 50.0f distance.)
-- FIDELITY NOTE: C++ UpdateAI (:64-69) gates on UpdateVictim() only —
-- there is NO HasUnitState(UNIT_STATE_CASTING) pre-check in
-- boss_moragg.cpp; Lua's unconditional timer firing matches the C++
-- exactly (earlier header wrongly claimed a phantom gate).
-- DOCUMENTED-ONLY (in this header; entry 29316):
-- spell_moragg_ray (AuraScript: OnEffectPeriodic on the ray auras —
-- picks SelectTarget(Random, 0, 45.0f, true) and CastSpell(target,
-- triggerSpell, aurEff)) — PORTED tick 424 below via
-- RegisterAuraEvent (54438 + 54442, 1); the "no AuraScript bridge"
-- block is stale (tick 188).
-- spell_moragg_optic_link (AuraScript OnPeriodic: tick >= 4 ->
-- OPTIC_LINK_LEVEL_2 54394, tick >= 8 -> OPTIC_LINK_LEVEL_3
-- 54395, always OPTIC_LINK_LEVEL_1 54393) — PORTED below via
-- RegisterAuraEvent (tick 194); the "no AuraScript bridge" block
-- is stale (tick 188). The OnUpdate damage ramp (750 -> 1500 ->
-- 3000 at ticks 1/4/8) is PORTED tick 486 inside opticLinkTick:
-- no OnEffectUpdatePeriodic event exists, but Go runs the ev1 hook
-- at the head of PeriodicTick before the damage switch reads
-- aura.Amount (spells.go:42121), and SetAmount writes the live
-- amount (lua_aura.go:583) — same-tick damage matches C++ exactly.
-- The JustReachedHome leg (boss_moragg.cpp:57-61) is PORTED tick
-- 486 via ev24 + FindNearestGameObject(GO_MORAGG_DOOR 191606, 50)
-- + SetGoState(1) (HandleCells(3, false), InstanceScript.cpp:479-487);
-- the instance-SetData bridge itself stays unported.

local ENTRY_MORAGG = 29316
local DATA_MORAGG = 3
local GO_MORAGG_DOOR = 191606 -- violet_hold.h:134

local SPELL_CORROSIVE_SALIVA = 54527
local SPELL_OPTIC_LINK = 54396
local SPELL_RAY_OF_PAIN = 54438
local SPELL_RAY_OF_SUFFERING = 54442

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

-- Cast-state gate (2026-11-04): BossAI ctor scheduler validator
-- (ScriptedCreature.cpp:493-496, !CASTING) — TaskScheduler::Dispatch
-- aborts while casting with tasks still queued, so each scheduler tick
-- below defers 500ms via creature:IsCasting()
-- (lua_creature_events.go:1950) without consuming its re-arm (the
-- mal_ganis idiom); optic-link/saliva scheduler ticks carry the gate; engage ray Async casts fire at combat start while the boss is not casting, ungated in effect.


-- C++ SelectTarget(Random, 0, 50.0f, true) draws the threat list
-- (UnitAI.h:162-184, playerOnly=true), not instance players: random
-- alive threat-list player within 50y (nefarian/vaelastrasz
-- precedent; the old instance-wide pool could optic-link a player
-- with zero threat). C++ IsWithinCombatRange adds both combat
-- reaches (Unit.cpp:583); plain GetDistance is the standing
-- granularity delta.
local function randomPlayerInRange(creature, dist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < dist then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ScheduleTasks optic-link leg (:80-85): SelectTarget Random, 0,
-- 50.0f, true (:81) — random alive player within 50 yd; DoCast(target,
-- SPELL_OPTIC_LINK), 2-arg = non-triggered; init 15s, re-arm 25s.
local function opticLinkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "opticlink", 500, function()
            opticLinkTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 50)
    if target then
        creature:CastSpell(target, SPELL_OPTIC_LINK)
    end
    schedule(guid, "opticlink", 25000, function()
        opticLinkTick(creature, guid)
    end)
end

-- C++ ScheduleTasks corrosive-saliva leg (:87-91): DoCastVictim
-- (SPELL_CORROSIVE_SALIVA), 1-arg = non-triggered; init 5s, re-arm
-- 10s.
local function corrosiveSalivaTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "saliva", 500, function()
            corrosiveSalivaTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_CORROSIVE_SALIVA)
    schedule(guid, "saliva", 10000, function()
        corrosiveSalivaTick(creature, guid)
    end)
end

-- C++ boss_moraggAI::JustEngagedWith (:52-55): base arms (:54)
-- ported base-first below — SetBossState(DATA_MORAGG 3, IN_PROGRESS)
-- first (ScriptedCreature.cpp:527-542), then pulse + me-zone; the
-- Async ray self-casts (:74-78) fire once at combat start, then the
-- two combat timers are seeded.
local function moraggEnterCombat(event, creature)
    creature:SetBossState(DATA_MORAGG, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:CastSpell(creature, SPELL_RAY_OF_PAIN)
    creature:CastSpell(creature, SPELL_RAY_OF_SUFFERING)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "opticlink", 15000, function()
        opticLinkTick(creature, guid)
    end)
    schedule(guid, "saliva", 5000, function()
        corrosiveSalivaTick(creature, guid)
    end)
end

-- C++ BossAI::JustDied (no override in boss_moragg.cpp):
-- _JustDied() (ScriptedCreature.cpp:513-520) — SetBossState
-- (DATA_MORAGG 3, DONE).
local function moraggJustDied(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(DATA_MORAGG, 3)
end

-- C++ BossAI::Reset (:47-50, base only): _Reset()
-- (ScriptedCreature.cpp:499-511) — SetBossState(DATA_MORAGG 3,
-- NOT_STARTED) only when the stored state != DONE. Split from ev2
-- (the keleseth/svala precedent): ev2 leave-combat does not call
-- Reset in C++, so it stays wipe-only.
local function moraggReset(event, creature)
    if creature:GetBossState(DATA_MORAGG) ~= 3 then
        creature:SetBossState(DATA_MORAGG, 0)
    end
    cancelTimers(creature:GetGUID())
end

-- Shared timer wipe for events 2/4 (the svala / yogg_saron
-- convention).
local function moraggCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_moraggAI::JustReachedHome (:57-61): BossAI base +
-- instance->SetData(DATA_HANDLE_CELLS, DATA_MORAGG) ->
-- HandleCells(3, false) -> HandleGameObject(GO_MORAGG_DOOR, false)
-- -> GO_STATE_READY (1) (InstanceScript.cpp:479-487).
-- Boss-side mirror via ev24 + FindNearestGameObject + SetGoState.
local function moraggReachedHome(event, creature)
    local door = creature:FindNearestGameObject(GO_MORAGG_DOOR, 50)
    if door then
        door:SetGoState(1)
    end
end

RegisterCreatureEvent(ENTRY_MORAGG, 1, moraggEnterCombat)
RegisterCreatureEvent(ENTRY_MORAGG, 2, moraggCancelCombatTimers)
RegisterCreatureEvent(ENTRY_MORAGG, 4, moraggJustDied)
RegisterCreatureEvent(ENTRY_MORAGG, 4, moraggCancelCombatTimers)
RegisterCreatureEvent(ENTRY_MORAGG, 23, moraggReset)
RegisterCreatureEvent(ENTRY_MORAGG, 24, moraggReachedHome)

-- C++ spell_moragg_optic_link::OnPeriodic
-- (boss_moragg.cpp:199-210): EFFECT_0 SPELL_AURA_PERIODIC_DAMAGE.
-- No PreventDefaultAction, so the Go default damage tick still
-- fires (the ramp lives in the unported OnUpdate leg). The caster
-- casts the level spells on the aura target, triggered by the aura
-- (the aurEff overload = triggered cast): level 1 (54393) every
-- tick, level 2 (54394) from tick 4, level 3 (54395) from tick 8.
-- GetTickNumber is the tick-191 bridge (first tick reads 1).
local SPELL_OPTIC_LINK_LEVEL_1 = 54393
local SPELL_OPTIC_LINK_LEVEL_2 = 54394
local SPELL_OPTIC_LINK_LEVEL_3 = 54395

local function opticLinkTick(event, aura, target)
    local caster = aura:GetCaster()
    if not caster then
        return
    end
    local tick = aura:GetTickNumber()
    -- C++ OnUpdate (boss_moragg.cpp:157-172): tick 1 +250 (base 500
    -- -> 750), tick 4 x2 (-> 1500), tick 8 x2 (-> 3000). The ev1
    -- hook runs before the same tick's damage read (see header).
    if tick == 1 then
        aura:SetAmount(aura:GetAmount() + 250)
    elseif tick == 4 or tick == 8 then
        aura:SetAmount(aura:GetAmount() * 2)
    end
    if tick >= 8 then
        caster:CastSpell(target, SPELL_OPTIC_LINK_LEVEL_3, true)
    end
    if tick >= 4 then
        caster:CastSpell(target, SPELL_OPTIC_LINK_LEVEL_2, true)
    end
    caster:CastSpell(target, SPELL_OPTIC_LINK_LEVEL_1, true)
end

RegisterAuraEvent(SPELL_OPTIC_LINK, 1, opticLinkTick)

-- C++ spell_moragg_ray::OnPeriodic (boss_moragg.cpp:110-121):
-- EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL — PreventDefaultAction;
-- the aura target (Moragg, who self-cast the ray) picks a random
-- alive threat-list player within 45 yd (SelectTarget Random, 0,
-- 45.0f, true) and casts the DBC trigger of EFFECT_0
-- (aura:GetTriggerSpell(0)) triggered on them. Returns true, the
-- PreventDefaultAction twin, so the Go default trigger cast does
-- not also fire. One handler for both ray auras (54438 Ray of Pain,
-- 54442 Ray of Suffering); the nonzero gate is the GetTriggerSpell
-- convention — with no Spell.dbc in tree the read is 0 and the
-- handler no-ops instead of firing a bogus 0-id cast. PORTED tick
-- 424 via RegisterAuraEvent (54438 + 54442, 1); randomPlayerInRange
-- is the shared threat-list helper above (shahraz generic-periodic
-- precedent for the trigger + return-true form).
local function moraggRayTick(event, aura, target)
    local picked = randomPlayerInRange(target, 45)
    if picked then
        local trigger = aura:GetTriggerSpell(0)
        if trigger ~= 0 then
            target:CastSpell(picked, trigger, true)
        end
    end
    return true
end

RegisterAuraEvent(SPELL_RAY_OF_PAIN, 1, moraggRayTick)
RegisterAuraEvent(SPELL_RAY_OF_SUFFERING, 1, moraggRayTick)
