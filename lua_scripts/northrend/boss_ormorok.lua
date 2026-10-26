-- Ormorok the Tree-Shaper (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_ormorok.cpp
-- (boss_ormorok (CreatureScript) via GetNexusAI<boss_ormorokAI>
-- (BossAI, DATA_ORMOROK); npc_crystal_spike_trigger
-- (CreatureScript); spell_crystal_spike (SpellScriptLoader);
-- AddSC_boss_ormorok() at end registers all three; loader decl 80 /
-- call 275 per northrend_script_loader.cpp — the FOURTH group of the
-- "// The Nexus: Nexus" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_anomalus() (decl 79 / call 274); the call after it
-- is AddSC_boss_keristrasza() (decl 81 / call 276).
-- Entry: 26794 Ormorok the Tree-Shaper (wowhead-verified 2026-10-25:
-- https://www.wowhead.com/npc=26794/ormorok-the-tree-shaper — no
-- NPC_ORMOROK const in the C++ tree, no instance_nexus.cpp
-- OnCreatureCreate case, same entry-block class as telestra).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent (maiden_of_virtue precedent), cancelled on 2/4/23;
-- melee is engine-driven in Go, like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 1) + schedules EVENT_CRYSTAL_
-- SPIKES (12s) + EVENT_TRAMPLE (10s) + EVENT_SPELL_REFLECTION
-- (30s) + heroic EVENT_CRYSTALLINE_TANGLER (15s, IsHeroic
-- bridged — lavanthor precedent); the BossAI::JustEngagedWith
-- instance leg has no bridge (tharon_ja precedent).
-- EVENT_CRYSTAL_SPIKES — Talk(SAY_CRYSTAL_SPIKES 4) + DoCastVictim
-- (SPELL_CRYSTAL_SPIKES 47958, 1-arg DoCast = victim cast) as
-- creature:CastSpell(nil, spell), 12s init / 12s repeat.
-- EVENT_TRAMPLE — DoCast(me, SPELL_TRAMPLE 48016) self-cast, 10s
-- init / 10s repeat.
-- EVENT_SPELL_REFLECTION — Talk(SAY_REFLECT 3) + DoCast(me, SPELL_
-- SPELL_REFLECTION 47981) self-cast, 30s init / 30s repeat.
-- EVENT_CRYSTALLINE_TANGLER (heroic only) — SelectTarget(Random, 0,
-- OrmorokTanglerPredicate) -> DoCast(target, SPELL_SUMMON_
-- CRYSTALLINE_TANGLER 61564): the predicate overload draws the
-- threat list with no playerOnly filter (UnitAI.h:176), so the pool
-- is GetAITargets + alive (C++ skips offline refs) + GetDistance2d
-- >= 5.0; 15s init / 15s unconditional re-arm.
-- DamageTaken — HealthBelowPct(25) one-shot latch -> Talk(SAY_FRENZY
-- 6) + DoCast(me, SPELL_FRENZY 48017) self-cast. NOTE the latch
-- reads CURRENT health, not post-damage: C++ uses HealthBelowPct,
-- not HealthBelowPctDamaged, and DamageTaken fires pre-application
-- (Unit.cpp:697-702), so the Lua twin is GetHealth() < floor(max *
-- 25 / 100) with NO damage subtraction (keristrasza's Damaged form
-- is the contrasting case). The latch therefore fires on the first
-- hit taken while already below 25%, one hit after the crossing —
-- that is the C++ behavior, preserved exactly.
-- JustDied — Talk(SAY_DEATH 2) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent).
-- KilledUnit — Talk(SAY_KILL 5) player-gated (nalorakk / kelthuzad
-- variant).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — BossAI::Reset + Initialize() (frenzy=false) — the latch
-- reset is ported on 2/4/23; the BossAI leg has no bridge.
-- UpdateAI HasUnitState(UNIT_STATE_CASTING) gate — no casting-state
-- bridge; Lua timers fire regardless of cast state (accepted
-- approximation, maiden_of_virtue class).
-- npc_crystal_spike_trigger — IsSummonedBy entry switch (initial
-- 27101: SetFacingToObject + aura 47941; trigger 27079: aura +
-- GetData(DATA_COUNT)+1 from the owner) + FindNearestGameObject(GO_
-- CRYSTAL_SPIKE_TRAP 188537, 1.0f) Use/Delete + 2s DespawnOrUnsummon
-- — no IsSummonedBy / GO-Use / GetData-cross-AI bridges.
-- spell_crystal_spike — AuraScript periodic: entry-gated summon
-- spells (47936 / urand 47942/47943) while GetData(DATA_COUNT) <
-- MAX_COUNT 5 — no AuraScript bridge.

local ENTRY_ORMOROK = 26794

local SAY_AGGRO = 1
local SAY_DEATH = 2
local SAY_REFLECT = 3
local SAY_CRYSTAL_SPIKES = 4
local SAY_KILL = 5
local SAY_FRENZY = 6

local SPELL_SPELL_REFLECTION = 47981
local SPELL_TRAMPLE = 48016
local SPELL_FRENZY = 48017
local SPELL_SUMMON_CRYSTALLINE_TANGLER = 61564
local SPELL_CRYSTAL_SPIKES = 47958

local timers = {}
local ormorokFrenzy = {}

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

-- C++ EVENT_CRYSTAL_SPIKES (boss_ormorok.cpp:165-169): Talk(SAY_
-- CRYSTAL_SPIKES 4) + DoCast(SPELL_CRYSTAL_SPIKES 47958) 1-arg =
-- victim cast, 12s init / 12s repeat.
local function crystalSpikesTick(creature, guid)
    creature:Talk(SAY_CRYSTAL_SPIKES)
    creature:CastSpell(nil, SPELL_CRYSTAL_SPIKES)
    schedule(guid, "spikes", 12000, function()
        crystalSpikesTick(creature, guid)
    end)
end

-- C++ EVENT_TRAMPLE (:160-163): DoCast(me, SPELL_TRAMPLE 48016)
-- self-cast, 10s init / 10s repeat.
local function trampleTick(creature, guid)
    creature:CastSpell(creature, SPELL_TRAMPLE)
    schedule(guid, "trample", 10000, function()
        trampleTick(creature, guid)
    end)
end

-- C++ EVENT_SPELL_REFLECTION (:170-174): Talk(SAY_REFLECT 3) +
-- DoCast(me, SPELL_SPELL_REFLECTION 47981) self-cast, 30s init /
-- 30s repeat.
local function spellReflectionTick(creature, guid)
    creature:Talk(SAY_REFLECT)
    creature:CastSpell(creature, SPELL_SPELL_REFLECTION)
    schedule(guid, "reflection", 30000, function()
        spellReflectionTick(creature, guid)
    end)
end

-- C++ EVENT_CRYSTALLINE_TANGLER (:175-179): SelectTarget(Random, 0,
-- OrmorokTanglerPredicate) -> DoCast(target, SPELL_SUMMON_
-- CRYSTALLINE_TANGLER 61564); predicate = GetDistance2d >= 5.0f
-- (both reaches, Object.cpp:1187; branch idiom),
-- threat-list pool, no playerOnly filter (UnitAI.h:176); re-arm
-- 15s unconditional.
local function tanglerTick(creature, guid)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        local targetReach = 1.5
        if t:GetObjectType() ~= "Player" then
            targetReach = t:GetCombatReach()
        end
        if t:IsAlive() and creature:GetDistance2d(t) - creature:GetCombatReach() - targetReach >= 5.0 then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)],
            SPELL_SUMMON_CRYSTALLINE_TANGLER)
    end
    schedule(guid, "tangler", 15000, function()
        tanglerTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:105-118): schedules + Talk(SAY_AGGRO 1);
-- the BossAI::JustEngagedWith instance leg has no bridge.
local function ormorokEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ormorokFrenzy[guid] = false
    creature:Talk(SAY_AGGRO)
    schedule(guid, "spikes", 12000, function()
        crystalSpikesTick(creature, guid)
    end)
    schedule(guid, "trample", 10000, function()
        trampleTick(creature, guid)
    end)
    schedule(guid, "reflection", 30000, function()
        spellReflectionTick(creature, guid)
    end)
    if creature:IsHeroic() then
        schedule(guid, "tangler", 15000, function()
            tanglerTick(creature, guid)
        end)
    end
end

local function ormorokLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ormorokFrenzy[guid] = false
end

-- C++ DamageTaken (:120-128): if (!frenzy && HealthBelowPct(25)) —
-- CURRENT-health form, see header note — Talk(SAY_FRENZY 6) +
-- DoCast(me, SPELL_FRENZY 48017) self-cast, one-shot latch.
local function ormorokDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if ormorokFrenzy[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() < math.floor(maxHealth * 25 / 100) then
        ormorokFrenzy[guid] = true
        creature:Talk(SAY_FRENZY)
        creature:CastSpell(creature, SPELL_FRENZY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH 2) (the _JustDied bookkeeping has no
-- bridge — tharon_ja precedent).
local function ormorokDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ormorokFrenzy[guid] = false
    creature:Talk(SAY_DEATH)
end

-- C++ KilledUnit: Talk(SAY_KILL 5) only when who->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk / kelthuzad player-gate variant).
local function ormorokTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function ormorokReset(event, creature)
    ormorokLeaveCombat(event, creature)
end

RegisterCreatureEvent(ENTRY_ORMOROK, 1, ormorokEnterCombat)
RegisterCreatureEvent(ENTRY_ORMOROK, 2, ormorokLeaveCombat)
RegisterCreatureEvent(ENTRY_ORMOROK, 3, ormorokTargetDied)
RegisterCreatureEvent(ENTRY_ORMOROK, 4, ormorokDied)
RegisterCreatureEvent(ENTRY_ORMOROK, 9, ormorokDamageTaken)
RegisterCreatureEvent(ENTRY_ORMOROK, 23, ormorokReset)
