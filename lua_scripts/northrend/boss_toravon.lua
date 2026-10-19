-- Toravon the Ice Watcher (Vault of Archavon) — Lua port of
-- src/server/scripts/Northrend/VaultOfArchavon/boss_toravon.cpp
-- (170 lines; 4 scripts — boss_toravon, npc_frozen_orb_stalker,
-- npc_frozen_orb, spell_toravon_random_aggro, all registered from
-- inside AddSC_boss_toravon() — loader decl 142 / call 337, the
-- "// Vault of Archavon" block in AddNorthrendScripts(),
-- northrend_script_loader.cpp).
-- Entry: 38433 Toravon the Ice Watcher (vault_of_archavon.h
-- NPC_TORAVON, line 41; DATA_TORAVON boss slot). Zero Talk lines in
-- the C++ — none ported.
-- The stale "no-timer-bridge" doctrine is superseded by the
-- CreateLuaEvent bridge (engine/scripting/runtime.go:798 — the
-- archavon / maiden_of_virtue precedent): module-local GUID-keyed
-- timers table, wipe on events 2 (leave combat), 4 (death), 23
-- (evade).
-- creature:CastSpell(nil, spell) = DoCastVictim; creature:CastSpell
-- (creature, spell) = DoCastSelf / me->CastSpell(me, ...)
-- (2-arg = non-triggered); creature:CastSpell(target, spell) =
-- DoCast(target, spell) (2-arg = non-triggered). All delays below
-- are fixed ms (C++ uses plain 7s/12s/25s literals plus Repeat 32s /
-- 38s), so plain ms delays carry the C++-exact timing — no
-- {minMs, maxMs} randtime table.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_toravon.cpp):
-- boss_toravon::JustEngagedWith (:50-60) — the immediate DoCastSelf
-- (SPELL_FROZEN_MALLET 71993) (:54, non-triggered) fires once before
-- the timers are scheduled, then seeds the three combat timers; the
-- BossAI::JustEngagedWith(who) passthrough is unmodeled.
-- EVENT_FROZEN_ORB (:76-82) — me->CastSpell(me, SPELL_FROZEN_ORB
-- 72091, AddSpellMod(SPELLVALUE_MAX_TARGETS, RAID_MODE(1, 3)))
-- (:79) -> CastSpell(creature, 72091); the max-targets raid-mode leg
-- is unbridged (documented below); 12s init (:56) / Repeat 32s
-- (:80).
-- EVENT_WHITEOUT (:83-86) — DoCastSelf(SPELL_WHITEOUT 72034) (:84,
-- non-triggered) -> CastSpell(creature, 72034); 25s init (:57) /
-- Repeat 38s (:85).
-- EVENT_FREEZING_GROUND (:87-91) — SelectTarget(Random, 1) (:88 —
-- offset 1 skips the first MaxThreat-order entry; playerOnly=false
-- default, so the threat list is picked whole) -> DoCast(target,
-- SPELL_FREEZING_GROUND 72090) (:89, 2-arg = non-triggered) ->
-- CastSpell(target, 72090); 7s init (:58) / Repeat 38s (:90).
-- Random-threat pool: GetAITargets (the bridged threat list; midnight
-- precedent).
-- DOCUMENTED DEVIATIONS:
-- EVENT_FROZEN_ORB's AddSpellMod(SPELLVALUE_MAX_TARGETS,
-- RAID_MODE(1, 3)) leg (:79) has no bridge — the cast fires with no
-- max-targets cap.
-- EVENT_FREEZING_GROUND's SelectTarget position-1 exclusion (:88) —
-- the offset now skips the threat head of the GetAITargets pool (the
-- C++ MaxThreat-order semantic, UnitAI.h); the stale halion
-- victim-exclusion idiom is retracted.
-- The UpdateAI HasUnitState(UNIT_STATE_CASTING) gates (pre-check
-- :67 and the per-event :95 return) have no bridge — casts fire
-- unconditionally on their timers (the hodir UNIT_STATE_CASTING
-- precedent).
-- DOCUMENTED-ONLY (in this header; entry 38433 is the sole
-- registration):
-- npc_frozen_orb_stalker::Reset (:104-108) — DoCastSelf
-- (FROZEN_ORB_STALKER_AURA 72094): Reset-hook self-cast, no entry
-- wiring (the stalker entry id is not named in this file).
-- npc_frozen_orb::IsSummonedBy (:111-133) — the three triggered
-- DoCastSelf legs (SPELL_FROZEN_ORB_AURA 72067, SPELL_FROZEN_ORB_DMG
-- 72081, SPELL_RANDOM_AGGRO 72084) plus the instance->
-- GetCreature(DATA_TORAVON) leg (JustSummoned passthrough /
-- DoZoneInCombat / DespawnOrUnsummon): instance-script bridge
-- missing.
-- spell_toravon_random_aggro (SpellScript :137-160 — 46523 Random
-- Aggro: ResetAllThreat + 1M threat on a random non-tank player) —
-- no SpellScript bridge.
-- DoMeleeAttackIfReady() (:97) — no bridge.

local ENTRY_TORAVON = 38433

local SPELL_FREEZING_GROUND = 72090
local SPELL_FROZEN_ORB = 72091
local SPELL_WHITEOUT = 72034
local SPELL_FROZEN_MALLET = 71993

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

-- C++ SelectTarget(SelectTargetMethod::Random, 1) — random threat-listed
-- unit excluding the threat head: offset 1 skips the first entry in
-- MaxThreat order (UnitAI.h); playerOnly=false default, so non-players
-- qualify (midnight precedent).
local function randomNonTankThreatTarget(creature)
    local targets = creature:GetAITargets()
    -- C++ shortcut: threat list size <= offset ignores ALL elements.
    if #targets <= 1 then
        return nil
    end
    return targets[1 + math.random(#targets - 1)]
end

-- C++ EVENT_FROZEN_ORB (:76-82): me->CastSpell(me,
-- SPELL_FROZEN_ORB) — the SPELLVALUE_MAX_TARGETS RAID_MODE(1, 3)
-- leg is unbridged (documented in the header); Repeat 32s.
local function frozenOrbTick(creature, guid)
    creature:CastSpell(creature, SPELL_FROZEN_ORB)
    schedule(guid, "frozenorb", 32000, function()
        frozenOrbTick(creature, guid)
    end)
end

-- C++ EVENT_WHITEOUT (:83-86): DoCastSelf(SPELL_WHITEOUT), 2-arg =
-- non-triggered; Repeat 38s.
local function whiteoutTick(creature, guid)
    creature:CastSpell(creature, SPELL_WHITEOUT)
    schedule(guid, "whiteout", 38000, function()
        whiteoutTick(creature, guid)
    end)
end

-- C++ EVENT_FREEZING_GROUND (:87-91): DoCast(target,
-- SPELL_FREEZING_GROUND), 2-arg = non-triggered; re-arm via Repeat
-- 38s (:90, unconditional).
local function freezingGroundTick(creature, guid)
    local target = randomNonTankThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_FREEZING_GROUND)
    end
    schedule(guid, "freezingground", 38000, function()
        freezingGroundTick(creature, guid)
    end)
end

-- C++ boss_toravon::JustEngagedWith (:50-60): the immediate
-- DoCastSelf(SPELL_FROZEN_MALLET) (:54) fires once before the timers
-- are scheduled (12s / 25s / 7s init); the BossAI passthrough is
-- unmodeled.
local function toravonEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_FROZEN_MALLET)
    schedule(guid, "frozenorb", 12000, function()
        frozenOrbTick(creature, guid)
    end)
    schedule(guid, "whiteout", 25000, function()
        whiteoutTick(creature, guid)
    end)
    schedule(guid, "freezingground", 7000, function()
        freezingGroundTick(creature, guid)
    end)
end

-- Shared timer wipe for events 2/23/4 (the svala / yogg_saron
-- convention).
local function toravonCancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_TORAVON, 1, toravonEnterCombat)
RegisterCreatureEvent(ENTRY_TORAVON, 2, toravonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TORAVON, 4, toravonCancelCombatTimers)
RegisterCreatureEvent(ENTRY_TORAVON, 23, toravonCancelCombatTimers)
