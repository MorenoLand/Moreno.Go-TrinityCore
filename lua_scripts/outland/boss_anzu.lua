-- Anzu (Sethekk Halls, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/SethekkHalls/boss_anzu.cpp
-- (1 CreatureScript — boss_anzu (BossAI via GetSethekkHallsAI,
-- DATA_ANZU = 2); registered from inside AddSC_boss_anzu(); loader
-- decl 32 / call 155 per outland_script_loader.cpp — the third group
-- of the "// Auchindoun - Sekketh Halls" sub-block in
-- AddOutlandScripts(), immediately after AddSC_boss_talon_king_ikiss()
-- (call 154); the checkpoint sequence
-- (darkweaver_syth -> talon_king_ikiss -> anzu) is followed).
-- Entry: 23035 NPC_ANZU (sethekk_halls.h:58). ScriptName bindings are
-- DB-side as usual. Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_anzu" hits boss_anzu.cpp (+ the loader decl/call lines)
-- only. No boss_anzu lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset (event 3 unused — no KilledUnit arm).
-- Ported arms (C++-exact for all modeled arms):
-- boss_anzuAI::JustEngagedWith — schedules EVENT_PARALYZING_SCREECH
-- (14s) + EVENT_CYCLONE_OF_FEATHERS (5s) (event 1); the
-- BossAI::JustEngagedWith instance leg (SetBossState IN_PROGRESS)
-- rides the no-instance bridge — documented-only.
-- EVENT_PARALYZING_SCREECH — DoCastVictim(SPELL_PARALYZING_SCREECH
-- 40184) 2-arg (non-triggered) → creature:CastSpell(nil, 40184)
-- (nil = victim fallback, the CastSpell bridge); 14s init, 25s flat
-- repeat.
-- EVENT_CYCLONE_OF_FEATHERS — SelectTarget(Random, 0) (playerOnly
-- default true) → DoCast(target, SPELL_CYCLONE_OF_FEATHERS 40321)
-- 2-arg (non-triggered); threat-list pool via GetAITargets with the
-- "Player" filter (ikiss convention); 5s init, 21s flat repeat.
-- boss_anzuAI::DamageTaken — HealthBelowPctDamaged(33/66, damage)
-- one-shot latches (event 9 fires pre-damage — moroes/syth
-- precedent): Talk(SAY_SUMMON_BROOD 0) + one-shot EVENT_SUMMON 3s.
-- C++ checks 33 BEFORE 66 (boss_anzu.cpp:102-113) — order kept; a
-- single hit crossing BOTH thresholds fires BOTH arms (Talk twice +
-- two EVENT_SUMMON schedules — EventMap::ScheduleEvent is a multimap
-- insert with no same-ID cancel, EventMap.cpp:44), so one-shot keys
-- carry a nonce instead of replacing like the syth helper.
-- EVENT_SUMMON — 7x me->SummonCreature(NPC_BROOD_OF_ANZU 23132,
-- PosSummonBrood[i], TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 46s)
-- rides the no-SummonCreature bridge — documented-only; the
-- DoCast(me, SPELL_BANISH_SELF 42354) 2-arg (non-triggered) self-cast
-- and the 12s one-shot EVENT_SPELL_BOMB schedule are ported.
-- EVENT_SPELL_BOMB — one-shot: SelectTarget(Random, 0) →
-- GetPowerType() == POWER_MANA (0) → DoCast(target, SPELL_SPELL_BOMB
-- 40303) 2-arg (non-triggered) + Talk(SAY_SPELL_BOMB 1, target)
-- (whisperTarget leg — bridged, lua_creature_events.go:102).
-- Non-mana targets fall through with no cast (one-shot either way —
-- the C++ never re-arms).
-- boss_anzuAI::Reset — events.Reset() + latch re-init (event 23; the
-- _Reset() call is commented out — no instance leg); merged with
-- event 2 (blackheart precedent — ScriptedAI::EnterEvadeMode calls
-- Reset()).
-- boss_anzuAI::JustDied — _JustDied() is instance-only (SetBossState
-- DONE) — documented-only; event 4 cancels the timer table (C++
-- UpdateAI stops on death).
-- All timers cancelled on 2/4/23; latches reset on 2/23.
-- DOCUMENTED-ONLY (in this header; only entry 23035 registered):
-- SPELL_FLESH_RIP 40199 — declared in the enum (boss_anzu.cpp:41)
-- but never referenced anywhere in the file (dead enum entry).
-- Brood of Anzu 23132 — no CreatureScript anywhere in the C++ tree
-- (whole-tree grep hits boss_anzu.cpp + sethekk_halls.h + loader
-- only); 7 fixed PosSummonBrood positions (boss_anzu.cpp:54-60) with
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT 46s — unscripted summons;
-- entry ID and positions named here.
-- DoMeleeAttackIfReady — documented-only.

local ENTRY_ANZU = 23035 -- NPC_ANZU (sethekk_halls.h:58)

local SAY_SUMMON_BROOD = 0 -- boss_anzu.cpp:31
local SAY_SPELL_BOMB   = 1 -- boss_anzu.cpp:32

local SPELL_PARALYZING_SCREECH  = 40184 -- boss_anzu.cpp:37
local SPELL_SPELL_BOMB          = 40303 -- boss_anzu.cpp:38
local SPELL_CYCLONE_OF_FEATHERS = 40321 -- boss_anzu.cpp:39
local SPELL_BANISH_SELF         = 42354 -- boss_anzu.cpp:40
-- SPELL_FLESH_RIP 40199 (boss_anzu.cpp:41) is never referenced in
-- the C++ file — documented-only.

local POWER_MANA = 0 -- npc_nether_drake.lua precedent

local timers = {}
local anzuState = {} -- guid -> { under33 = bool, under66 = bool }
local nonce = 0

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- Replace-on-key (syth convention) for repeating arms: the fired id
-- is removed first (no-op on an already-fired id).
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

-- Append-on-key for one-shots: C++ EventMap::ScheduleEvent
-- (EventMap.cpp:44) inserts into a multimap and never cancels a
-- pending same-ID event, so the double-latch crossing schedules two
-- genuine EVENT_SUMMONs.
local function scheduleOnce(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    nonce = nonce + 1
    per[key .. "#" .. nonce] = CreateLuaEvent(fn, delay)
end

-- C++ SelectTarget(Random, 0) (playerOnly defaults FALSE per
-- UnitAI.h:170 — pets/totems eligible) — threat-list pool via
-- GetAITargets (the Player filter was dropped 2026-10-19).
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        candidates[#candidates + 1] = t
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_PARALYZING_SCREECH (boss_anzu.cpp:128-130):
-- DoCastVictim(SPELL_PARALYZING_SCREECH) 2-arg (non-triggered) →
-- creature:CastSpell(nil, spell) (nil = victim fallback); 14s init,
-- 25s flat repeat.
local function paralyzingScreechTick(creature, guid)
    creature:CastSpell(nil, SPELL_PARALYZING_SCREECH)
    schedule(guid, "paralyzing_screech", 25000, function()
        paralyzingScreechTick(creature, guid)
    end)
end

-- C++ EVENT_CYCLONE_OF_FEATHERS (boss_anzu.cpp:132-135):
-- SelectTarget(Random, 0) → DoCast(target, SPELL_CYCLONE_OF_FEATHERS)
-- 2-arg (non-triggered); 5s init, 21s flat repeat.
local function cycloneOfFeathersTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CYCLONE_OF_FEATHERS)
    end
    schedule(guid, "cyclone_of_feathers", 21000, function()
        cycloneOfFeathersTick(creature, guid)
    end)
end

-- C++ EVENT_SPELL_BOMB (boss_anzu.cpp:145-159): one-shot —
-- SelectTarget(Random, 0) → POWER_MANA gate → DoCast(target,
-- SPELL_SPELL_BOMB) 2-arg (non-triggered) + Talk(SAY_SPELL_BOMB,
-- target); non-mana targets fall through with no cast.
local function spellBombTick(creature, guid)
    local target = randomThreatTarget(creature)
    if target and target:GetPowerType() == POWER_MANA then
        creature:CastSpell(target, SPELL_SPELL_BOMB)
        creature:Talk(SAY_SPELL_BOMB, target)
    end
end

-- C++ EVENT_SUMMON (boss_anzu.cpp:137-144): the 7x SummonCreature leg
-- rides the no-SummonCreature bridge (documented-only); the
-- DoCast(me, SPELL_BANISH_SELF) self-cast and the 12s one-shot
-- EVENT_SPELL_BOMB schedule are ported.
local function summonTick(creature, guid)
    creature:CastSpell(creature, SPELL_BANISH_SELF)
    scheduleOnce(guid, "spell_bomb", 12000, function()
        spellBombTick(creature, guid)
    end)
end

-- C++ boss_anzuAI::DamageTaken (boss_anzu.cpp:100-115): the C++
-- formula in this tree is int64(health)-int64(damage) <
-- int64(CountPctFromMaxHealth(pct)) (Unit.h:873), CountPctFromMaxHealth
-- = floor of the float product (Util.h:66-69) — the moroes/syth
-- integer-floor projection form is outcome-identical, overkill
-- included; C++ checks 33 BEFORE 66 — order kept.
local function anzuDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- Lazy init: C++ Initialize() runs in the constructor too, so a
    -- fresh spawn has all-false latches even if the engine never
    -- fired 23 for it (syth precedent).
    local state = anzuState[guid]
    if state == nil then
        state = { under33 = false, under66 = false }
        anzuState[guid] = state
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = math.max(health - damage, 0)
    if (not state.under33) and postDamage < math.floor(maxHealth * 33 / 100) then
        state.under33 = true
        creature:Talk(SAY_SUMMON_BROOD)
        scheduleOnce(guid, "summon", 3000, function()
            summonTick(creature, guid)
        end)
    end
    if (not state.under66) and postDamage < math.floor(maxHealth * 66 / 100) then
        state.under66 = true
        creature:Talk(SAY_SUMMON_BROOD)
        scheduleOnce(guid, "summon", 3000, function()
            summonTick(creature, guid)
        end)
    end
end

-- C++ boss_anzuAI::JustEngagedWith (boss_anzu.cpp:88-93): the
-- BossAI::JustEngagedWith instance leg rides the no-instance bridge;
-- the two timer arms are ported (event 1, auriaya precedent).
local function anzuEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "paralyzing_screech", 14000, function()
        paralyzingScreechTick(creature, guid)
    end)
    schedule(guid, "cyclone_of_feathers", 5000, function()
        cycloneOfFeathersTick(creature, guid)
    end)
end

-- C++ boss_anzuAI::Reset (boss_anzu.cpp:81-86): events.Reset() +
-- latch re-init (the _Reset() call is commented out — no instance
-- leg); merged with event 2 (blackheart precedent —
-- ScriptedAI::EnterEvadeMode calls Reset()).
local function anzuLeaveCombatOrReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    anzuState[guid] = { under33 = false, under66 = false }
end

-- C++ boss_anzuAI::JustDied (boss_anzu.cpp:95-98): _JustDied() is
-- instance-only — documented-only; event 4 exists only to cancel the
-- timer table (C++ UpdateAI stops on death).
local function anzuDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    anzuState[creature:GetGUID()] = nil
end

RegisterCreatureEvent(ENTRY_ANZU, 1, anzuEnterCombat)
RegisterCreatureEvent(ENTRY_ANZU, 2, anzuLeaveCombatOrReset)
RegisterCreatureEvent(ENTRY_ANZU, 4, anzuDied)
RegisterCreatureEvent(ENTRY_ANZU, 9, anzuDamageTaken)
RegisterCreatureEvent(ENTRY_ANZU, 23, anzuLeaveCombatOrReset)
