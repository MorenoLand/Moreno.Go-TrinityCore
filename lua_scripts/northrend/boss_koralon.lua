-- Koralon the Flame Watcher (Vault of Archavon) — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/boss_koralon.cpp
-- (230 lines; 4 scripts — boss_koralon, spell_koralon_meteor_fists,
-- spell_koralon_meteor_fists_damage, spell_flame_warder_meteor_fists,
-- all registered from inside AddSC_boss_koralon() — loader decl 141
-- / call 336, the "// Vault of Archavon" block in
-- AddNorthrendScripts(), northrend_script_loader.cpp).
-- Entry: 35013 Koralon the Flame Watcher (vault_of_archavon.h
-- NPC_KORALON, line 40; DATA_KORALON boss slot). Zero Talk lines in
-- the C++ — none ported.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade).
-- creature:CastSpell(creature, spell) = DoCastSelf / DoCast(me)
-- (2-arg = non-triggered; no 3rd-arg casts exist in this AI). All
-- delays below are fixed ms (C++ uses plain 15s/20s/30s/45s/75s
-- literals), so plain ms delays carry the C++-exact timing — no
-- {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_koralon.cpp):
-- boss_koralonAI::JustEngagedWith (:54-68) — the immediate DoCast(me,
-- SPELL_BURNING_FURY 66721) (:59, non-triggered) fires once before
-- the timers are scheduled, then seeds the four combat timers; the
-- BossAI::JustEngagedWith(who) passthrough is unmodeled.
-- EVENT_BURNING_FURY (:83-86) — DoCast(me, 66721) (:84,
-- non-triggered) -> CastSpell(creature, 66721); re-arm 20s (:85).
-- EVENT_BURNING_BREATH (:87-90) — DoCast(me, SPELL_BURNING_BREATH
-- 66665) (:88, non-triggered) -> CastSpell(creature, 66665); 15s
-- init (:62) / 45s re-arm (:89).
-- EVENT_METEOR_FISTS (:91-94) — DoCast(me, SPELL_METEOR_FISTS
-- 66725) (:92, non-triggered) -> CastSpell(creature, 66725); 75s
-- init (:63) / 45s re-arm (:93).
-- EVENT_FLAME_CINDER (:95-98) — DoCast(me, SPELL_FLAME_CINDER_A
-- 66684) (:96, non-triggered) -> CastSpell(creature, 66684); re-arm
-- 30s (:97). SPELL_FLAME_CINDER_B 66681 sits in the enum but is never
-- cast anywhere in the file.
-- DOCUMENTED DEVIATIONS:
-- The UpdateAI HasUnitState(UNIT_STATE_CASTING) gates (pre-check
-- :77 and the per-event :102 return) have no bridge — casts fire
-- unconditionally on their timers (the hodir UNIT_STATE_CASTING
-- precedent).
-- DOCUMENTED-ONLY (in this header; entry 35013 is the sole
-- registration):
-- spell_koralon_meteor_fists (AuraScript :133-159; OnEffectProc
-- SPELL_AURA_DUMMY hook firing SPELL_METEOR_FISTS_DAMAGE 66765) —
-- no SpellScript/AuraScript bridge.
-- spell_koralon_meteor_fists_damage (SpellScript :161-196; split
-- damage over chain targets) — no SpellScript bridge.
-- spell_flame_warder_meteor_fists (AuraScript :198-225; firing
-- SPELL_FW_METEOR_FISTS_DAMAGE 66809) — no SpellScript/AuraScript
-- bridge.
-- DoMeleeAttackIfReady() (:103) — no bridge.

local ENTRY_KORALON = 35013

local SPELL_BURNING_BREATH = 66665
local SPELL_BURNING_FURY = 66721
local SPELL_FLAME_CINDER_A = 66684
local SPELL_METEOR_FISTS = 66725

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

-- C++ EVENT_BURNING_FURY (:83-86): DoCast(me, SPELL_BURNING_FURY),
-- 2-arg = non-triggered; re-arm 20s.
local function burningFuryTick(creature, guid)
    creature:CastSpell(creature, SPELL_BURNING_FURY)
    schedule(guid, "burningfury", 20000, function()
        burningFuryTick(creature, guid)
    end)
end

-- C++ EVENT_BURNING_BREATH (:87-90): DoCast(me,
-- SPELL_BURNING_BREATH), 2-arg = non-triggered; re-arm 45s.
local function burningBreathTick(creature, guid)
    creature:CastSpell(creature, SPELL_BURNING_BREATH)
    schedule(guid, "burningbreath", 45000, function()
        burningBreathTick(creature, guid)
    end)
end

-- C++ EVENT_METEOR_FISTS (:91-94): DoCast(me, SPELL_METEOR_FISTS),
-- 2-arg = non-triggered; re-arm 45s.
local function meteorFistsTick(creature, guid)
    creature:CastSpell(creature, SPELL_METEOR_FISTS)
    schedule(guid, "meteorfists", 45000, function()
        meteorFistsTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_CINDER (:95-98): DoCast(me, SPELL_FLAME_CINDER_A),
-- 2-arg = non-triggered; re-arm 30s.
local function flameCinderTick(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_CINDER_A)
    schedule(guid, "flamecinder", 30000, function()
        flameCinderTick(creature, guid)
    end)
end

-- C++ boss_koralonAI::JustEngagedWith (:54-68): the immediate
-- DoCast(me, SPELL_BURNING_FURY) (:59) fires once before the timers
-- are scheduled (20s / 15s / 75s / 30s init); the BossAI passthrough
-- is unmodeled.
local function koralonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_BURNING_FURY)
    schedule(guid, "burningfury", 20000, function()
        burningFuryTick(creature, guid)
    end)
    schedule(guid, "burningbreath", 15000, function()
        burningBreathTick(creature, guid)
    end)
    schedule(guid, "meteorfists", 75000, function()
        meteorFistsTick(creature, guid)
    end)
    schedule(guid, "flamecinder", 30000, function()
        flameCinderTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function koralonCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KORALON, 1, koralonEnterCombat)
RegisterCreatureEvent(ENTRY_KORALON, 2, koralonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_KORALON, 4, koralonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_KORALON, 23, koralonCancelCombatTimers)
