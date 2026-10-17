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
-- boss_moraggAI::JustEngagedWith (:57-60) — seeds the combat timers;
-- the BossAI::JustEngagedWith passthrough leg has no bridge.
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
-- triggerSpell, aurEff)) — no AuraScript bridge. Fidelity gap: the
-- ported RAY_OF_PAIN / RAY_OF_SUFFERING auras apply but their
-- scripted periodic triggers will not fire.
-- spell_moragg_optic_link (AuraScript: tick >= 4 -> OPTIC_LINK_LEVEL_2
-- 54394, tick >= 8 -> OPTIC_LINK_LEVEL_3 54395, always
-- OPTIC_LINK_LEVEL_1 54393; damage ramp 750 -> 1500 -> 3000 at ticks
-- 1/4/8) — no AuraScript bridge; the optic-link visual stages and
-- damage ramp stay out.
-- The JustReachedHome instance->SetData(DATA_HANDLE_CELLS,
-- DATA_MORAGG) leg — no instance-script bridge.

local ENTRY_MORAGG = 29316

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, ...) — alive
-- players on the same map/instance (the keleseth / thorim
-- precedent).
local function randomPlayer(creature)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Same pool with the C++ SelectTarget 50.0f distance filter
-- (creature:IsWithinDist is the bridged distance check — the
-- maiden / kiljaeden / marwyn 100.0f precedent).
local function randomPlayerInRange(creature, dist)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:IsWithinDist(plr, dist) then
            candidates[#candidates + 1] = plr
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
    creature:CastSpell(nil, SPELL_CORROSIVE_SALIVA)
    schedule(guid, "saliva", 10000, function()
        corrosiveSalivaTick(creature, guid)
    end)
end

-- C++ boss_moraggAI::JustEngagedWith (:57-60): the Async ray
-- self-casts (:74-78) fire once at combat start, then the two combat
-- timers are seeded; the BossAI::JustEngagedWith passthrough leg has
-- no bridge.
local function moraggEnterCombat(event, creature)
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

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function moraggCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MORAGG, 1, moraggEnterCombat)
RegisterCreatureEvent(ENTRY_MORAGG, 2, moraggCancelCombatTimers)
RegisterCreatureEvent(ENTRY_MORAGG, 4, moraggCancelCombatTimers)
RegisterCreatureEvent(ENTRY_MORAGG, 23, moraggCancelCombatTimers)
