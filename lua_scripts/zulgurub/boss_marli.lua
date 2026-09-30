-- High Priestess Mar'li (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_marli.cpp
-- (boss_marliAI + gob_spider_eggAI + npc_spawn_of_marliAI +
-- spell_hatch_spiders); zulgurub.h:32 (DATA_MARLI = 2, main boss),
-- :54 (NPC_PRIESTESS_MARLI = 14510). Creature entries: 14510 High
-- Priestess Mar'li (C++ ScriptName "boss_marli" per AddSC_boss_marli);
-- 15041 Spawn of Mar'li (C++ ScriptName "npc_spawn_of_marli",
-- NPC_SPIDER). The spider eggs (gameobject 179985, C++ ScriptName
-- "gob_spider_egg") are NOT registered — their only arms (egg
-- Respawn/UpdateObjectVisibility in Mar'li's Reset, and the
-- JustSummoned forward to Mar'li's AI) have no gameobject-summon
-- bridge.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++.
-- Fight shape: enter combat -> Talk(SAY_AGGRO=0), arm spider-spawn 1s
-- (Talk(SAY_SPIDER_SPAWN=2), phase -> 2, arm aspect of marli 24686 12s
-- then {13s,18s} on the victim (triggered DoCastVictim, C++-exact,
-- phase-2 only) / transform 45s x2 (C++ schedules the duplicate
-- EVENT_TRANSFORM twice at 45s, both PHASE_TWO — the second is inert,
-- so only the first-to-fire is modeled) / poison volley 24099 15s then
-- {10s,20s} on the victim (triggered DoCastVictim, C++-exact, all
-- phases) / hatch spider egg 24082 30s then {12s,17s} (non-triggered
-- self-cast; only the re-arm cycle is modeled — no summon model, so
-- eggs never hatch into spiders). Transform: Talk(SAY_TRANSFORM=1),
-- non-triggered self-cast 24084 (spider form), non-triggered
-- DoCastVictim(24110) envolwing web, threat wipe (no threat model —
-- skipped), phase -> 3; the phase-2 timers are canceled (C++ phase
-- mask); arm charge 22911 1.5s then every 8s on a mana-using random
-- player (up to 3 draws, not the current victim — C++ "not aggro
-- leader" is approximated as excluding the victim; no threat model) /
-- transform back 25s. Transform back: RemoveAura(24084), phase -> 2,
-- re-arm aspect 12s, transform 45s + {35s,60s} (C++-exact double
-- scheduling — first-to-fire wins, the other is canceled),
-- poison volley 15s, hatch egg {12s,17s}. Re-arms are unconditional
-- (nil-victim ticks cast nothing but keep the schedule, jeklik
-- convention). OnDied: Talk(SAY_DEATH=3). The UPDATEAI
-- UNIT_STATE_CASTING queue gate and the post-switch casting check have
-- no UNIT_STATE bridge — timers fire unconditionally (jeklik
-- convention). Spawn of Mar'li (15041): non-triggered self-cast 24312
-- (level up) every 3s while in combat (C++-exact); melee is
-- engine-driven; never spawns in this build (no summon model) but is
-- registered so any that exist fight correctly.
-- Deviations from C++: no instance-script model — boss admission via
-- the luaBossAI shim (_Reset/_JustDied/BossAI::JustEngagedWith, the
-- GetZulGurubAI bookkeeping, and the DATA_MARLI encounter-state arms
-- skipped); no summon model — the HATCH_EGGS 24083 AoE and the
-- HATCH_SPIDER_EGG 24082 casts spawn nothing (only their re-arm cycles
-- are modeled); no gameobject bridge — the Reset egg
-- Respawn/UpdateObjectVisibility scan and the gob_spider_egg
-- JustSummoned->Mar'li AI forward have no bearer; no threat model —
-- the transform -100% threat wipe and the charge "not aggro leader"
-- exclusion are approximated with the current victim; no
-- stat-modifier bridge — the DamageIncrease/Decrease ApplyStatPctModifier
-- hack (including the Reset PHASE_THREE rollback) is skipped; no
-- movement model — the charge AttackStart arm is skipped (cast lands,
-- the boss keeps its current victim); the summoned-spider
-- JustSummoned AttackStart-random arm has no bearer (nothing ever
-- summons); spell_hatch_spiders (SpellScript target-select sort/
-- resize) is a SpellScript — not modeled (standing gap).

local ENTRY_MARLI = 14510
local ENTRY_SPAWN = 15041

local SPELL_CHARGE = 22911
local SPELL_ASPECT_OF_MARLI = 24686
local SPELL_ENVOLWINGWEB = 24110
local SPELL_POISON_VOLLEY = 24099
local SPELL_SPIDER_FORM = 24084
local SPELL_LEVELUP = 24312

local SAY_AGGRO = 0
local SAY_TRANSFORM = 1
local SAY_SPIDER_SPAWN = 2
local SAY_DEATH = 3

local PHASE_TWO = 2
local PHASE_THREE = 3

local POWER_MANA = 0

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
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

local function marliState(guid)
    local st = state[guid]
    if not st then
        st = { phase = 1 }
        state[guid] = st
    end
    return st
end

local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ EVENT_POISON_VOLLEY: triggered DoCastVictim(24099); re-arm
-- {10s,20s}. Phase-all: fires in every phase, C++-exact.
local function onPoisonVolley(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_POISON_VOLLEY, true)
    end
    schedule(guid, "poisonvolley", math.random(10000, 20000), function()
        onPoisonVolley(creature, guid)
    end)
end

-- C++ EVENT_HATCH_SPIDER_EGG: non-triggered self-cast 24082; re-arm
-- {12s,17s}. Phase-all. No summon model, so the cast itself has no
-- bearer — only the re-arm cycle is modeled (jeklik six-bat
-- convention).
local function onHatchEgg(creature, guid)
    schedule(guid, "hatchegg", math.random(12000, 17000), function()
        onHatchEgg(creature, guid)
    end)
end

-- C++ EVENT_ASPECT_OF_MARLI: triggered DoCastVictim(24686); re-arm
-- {13s,18s} phase-TWO. Handler no-ops outside phase 2 (C++ phase
-- mask); canceled by the phase-3 transition, C++-exact.
local function onAspectOfMarli(creature, guid)
    local st = marliState(guid)
    if st.phase == PHASE_TWO then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_ASPECT_OF_MARLI, true)
        end
        schedule(guid, "aspect", math.random(13000, 18000), function()
            onAspectOfMarli(creature, guid)
        end)
    end
end

-- Forward declaration: onTransform is referenced by onTransformBack
-- and vice versa.
local onTransform
local onTransformBack

-- C++ EVENT_CHARGE_PLAYER: up to 3 tries at SelectTarget(Random, 1,
-- 100, true) — "not aggro leader" — breaking on a POWER_MANA target;
-- DoCast(target, 22911) non-triggered; re-arm 8s phase-THREE. Handler
-- no-ops outside phase 3 (C++ phase mask).
local function onChargePlayer(creature, guid)
    local st = marliState(guid)
    if st.phase ~= PHASE_THREE then
        return
    end
    local victim = creature:GetVictim()
    local target = nil
    for _ = 1, 3 do
        local candidates = {}
        for _, p in ipairs(alivePlayersInInstance(creature)) do
            if p ~= victim and creature:GetDistance(p) <= 100 then
                candidates[#candidates + 1] = p
            end
        end
        if #candidates == 0 then
            break
        end
        local pick = candidates[math.random(#candidates)]
        if pick:GetPowerType() == POWER_MANA then
            target = pick
            break
        end
    end
    if target then
        -- The AttackStart(target) arm has no movement-model bridge —
        -- the cast lands, the boss keeps its current victim.
        creature:CastSpell(target, SPELL_CHARGE)
    end
    schedule(guid, "charge", 8000, function()
        onChargePlayer(creature, guid)
    end)
end

-- Arms the phase-2 transform timer(s). C++ EVENT_TRANSFORM 45s
-- duplicate at combat start collapses to one (the second 45s
-- scheduling is inert). The transform-back arms two schedules (45s
-- and {35s,60s}, both PHASE_TWO) — first-to-fire wins, the sibling is
-- canceled.
local function armTransformTimers(creature, guid, first, second)
    schedule(guid, "transformA", first, function()
        cancelTimer(guid, "transformB")
        onTransform(creature, guid)
    end)
    if second then
        schedule(guid, "transformB", second, function()
            cancelTimer(guid, "transformA")
            onTransform(creature, guid)
        end)
    end
end

-- C++ EVENT_TRANSFORM: Talk(SAY_TRANSFORM), non-triggered self-cast
-- 24084 (spider form), non-triggered DoCastVictim(24110) envolwing
-- web; the +35% damage stat hack and the -100% threat wipe have no
-- bridge; phase -> 3 with the phase-2 timers dropped (C++ phase mask);
-- arm charge 1.5s and transform-back 25s, both phase-THREE.
onTransform = function(creature, guid)
    local st = marliState(guid)
    if st.phase ~= PHASE_TWO then
        return
    end
    creature:Talk(SAY_TRANSFORM)
    creature:CastSpell(creature, SPELL_SPIDER_FORM)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ENVOLWINGWEB)
    end
    cancelTimer(guid, "aspect")
    cancelTimer(guid, "transformB")
    st.phase = PHASE_THREE
    schedule(guid, "charge", 1500, function()
        onChargePlayer(creature, guid)
    end)
    schedule(guid, "transformback", 25000, function()
        onTransformBack(creature, guid)
    end)
end

-- C++ EVENT_TRANSFORM_BACK: RemoveAura(24084); the damage-decrease
-- stat hack has no bridge; phase -> 2; re-arm aspect 12s, transform
-- 45s + {35s,60s}, poison volley 15s, hatch egg {12s,17s}.
onTransformBack = function(creature, guid)
    local st = marliState(guid)
    if st.phase ~= PHASE_THREE then
        return
    end
    creature:RemoveAura(SPELL_SPIDER_FORM)
    cancelTimer(guid, "charge")
    st.phase = PHASE_TWO
    schedule(guid, "aspect", 12000, function()
        onAspectOfMarli(creature, guid)
    end)
    armTransformTimers(creature, guid, 45000, math.random(35000, 60000))
    schedule(guid, "poisonvolley", 15000, function()
        onPoisonVolley(creature, guid)
    end)
    schedule(guid, "hatchegg", math.random(12000, 17000), function()
        onHatchEgg(creature, guid)
    end)
end

-- C++ EVENT_SPAWN_START_SPIDERS (1s after engage): Talk
-- (SAY_SPIDER_SPAWN), the 24083 AoE hatch has no summon-model bearer
-- (skipped); arm aspect 12s, transform 45s (duplicate skipped — inert),
-- poison volley 15s, hatch egg 30s; phase -> 2.
local function onSpawnStartSpiders(creature, guid)
    creature:Talk(SAY_SPIDER_SPAWN)
    marliState(guid).phase = PHASE_TWO
    schedule(guid, "aspect", 12000, function()
        onAspectOfMarli(creature, guid)
    end)
    armTransformTimers(creature, guid, 45000)
    schedule(guid, "poisonvolley", 15000, function()
        onPoisonVolley(creature, guid)
    end)
    schedule(guid, "hatchegg", 30000, function()
        onHatchEgg(creature, guid)
    end)
end

local function marliEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    marliState(guid).phase = 1
    creature:Talk(SAY_AGGRO)
    schedule(guid, "spawnstart", 1000, function()
        onSpawnStartSpiders(creature, guid)
    end)
end

local function marliWipe(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = nil
end

RegisterCreatureEvent(ENTRY_MARLI, 1, marliEnterCombat)
RegisterCreatureEvent(ENTRY_MARLI, 2, marliWipe)
RegisterCreatureEvent(ENTRY_MARLI, 4, function(event, creature, killer)
    creature:Talk(SAY_DEATH)
    marliWipe(event, creature)
end)
RegisterCreatureEvent(ENTRY_MARLI, 23, marliWipe)

-- Spawn of Mar'li (npc_spawn_of_marli, C++ ScriptName for 15041):
-- non-triggered self-cast 24312 (level up) every 3s while in combat
-- (C++-exact); melee is engine-driven. Never spawns in this build (no
-- summon model) but is registered so any that exist fight correctly.
local function onSpawnLevelUp(creature, guid)
    creature:CastSpell(creature, SPELL_LEVELUP)
    schedule(guid, "levelup", 3000, function()
        onSpawnLevelUp(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_SPAWN, 1, function(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "levelup", 3000, function()
        onSpawnLevelUp(creature, guid)
    end)
end)
RegisterCreatureEvent(ENTRY_SPAWN, 2, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SPAWN, 4, function(event, creature, killer)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SPAWN, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
