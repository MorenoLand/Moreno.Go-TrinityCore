-- High Priestess Mar'li (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_marli.cpp
-- (boss_marliAI + gob_spider_eggAI + npc_spawn_of_marliAI +
-- spell_hatch_spiders); zulgurub.h:32 (DATA_MARLI = 2, main boss),
-- :54 (NPC_PRIESTESS_MARLI = 14510). Creature entries: 14510 High
-- Priestess Mar'li (C++ ScriptName "boss_marli" per AddSC_boss_marli);
-- 15041 Spawn of Mar'li (C++ ScriptName "npc_spawn_of_marli",
-- NPC_SPIDER). The spider eggs (gameobject 179985, C++ ScriptName
-- "gob_spider_egg") are NOT registered — the gob_spider_egg
-- JustSummoned forward to Mar'li's AI has no gameobject-summon
-- bridge, but Mar'li's Reset egg Respawn scan IS ported (marliReset,
-- GetGameObjectsInGrid 250yd; the UpdateObjectVisibility(true) leg
-- has no bridge).
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
-- phases; each transform-back adds a fresh concurrent 15s cycle —
-- C++ ScheduleEvent never dedups) / hatch spider egg 24082 30s then
-- {12s,17s} (non-triggered self-cast, ISSUED 2026-11-13 — DBC verdict
-- below: 24082 is SPELL_EFFECT_ACTIVATE_OBJECT (86),
-- TARGET_GAMEOBJECT_NEARBY_ENTRY (40), Open (MiscValue 8) per TDB
-- spell_dbc — NOT a summon, so the eggs hatch by GO Use, not by
-- spawning; the cast reaches the Go activate-object arm but stays
-- dormant until the ownerless-Open engine leg lands; the cycle is
-- canceled at transform and re-armed at transform-back, C++-exact).
-- Transform: Talk(SAY_TRANSFORM=1),
-- non-triggered self-cast 24084 (spider form), non-triggered
-- DoCastVictim(24110) envolwing web, threat wipe (PORTED 2026-10-24),
-- phase -> 3; the phase-2 timers are canceled (C++ phase
-- mask); arm charge 22911 1.5s then every 8s on a random threat-list player
-- (up to 3 draws breaking on a mana user; C++ Random,1 skips the
-- first MaxThreat entry before filters — "not aggro leader" is the
-- threat head, not the victim; the C++ charges the last draw even
-- when all three miss
-- a mana user, C++-exact) /
-- transform back 25s. Transform back: RemoveAura(24084), phase -> 2,
-- re-arm aspect 12s, transform 45s + {35s,60s} (C++-exact double
-- scheduling — first-to-fire wins, the other is canceled),
-- poison volley 15s, hatch egg {12s,17s}. Re-arms are unconditional
-- (nil-victim ticks cast nothing but keep the schedule, jeklik
-- convention). OnDied: Talk(SAY_DEATH=3). The UPDATEAI
-- UNIT_STATE_CASTING queue gate and the post-switch casting check have
-- no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — timers fire unconditionally (jeklik
-- convention). Spawn of Mar'li (15041): non-triggered self-cast 24312
-- (level up) every 3s while in combat (C++-exact); melee is
-- engine-driven; never spawns in this build (no summon model) but is
-- registered so any that exist fight correctly.
-- Deviations from C++: no instance-script model — boss admission via
-- the luaBossAI shim (a no-op BossAI slot; the SetBossState legs are
-- ported below via creature:SetBossState, bridge live 2026-11-05, and
-- the GetZulGurubAI bookkeeping stays out); no hatch model — the HATCH_EGGS 24083 AoE and the
-- HATCH_SPIDER_EGG 24082 casts are not issued (only their re-arm cycles
-- are modeled): DBC verdict 2026-11-13, 24082 is ACTIVATE_OBJECT/Open on
-- nearby egg GOs (TDB spell_dbc, column-mapped: Effect_1 86,
-- ImplicitTargetA_1 40, MiscValue_1 8) and 24083 is the same effect
-- (wowhead wotlk: Activate Object, Radius 100y, Max targets 4) —
-- neither summons; the Go Open leg needs a player owner session
-- (boss_ai.go:8337) plus Spell.dbc data — an ownerless boss caster
-- no-ops on both counts, so the eggs never hatch until that chain
-- lands (goober Use -> summon 15041 -> egg JustSummoned forward); the Reset egg Respawn scan is PORTED (marliReset,
-- alive-gated vancleef idiom) and the gob_spider_egg
-- JustSummoned->Mar'li AI forward has no bearer; the transform -100% threat wipe is PORTED 2026-10-24;
-- the charge "not aggro leader" exclusion is the Random,1 skip-first
-- threat entry (offset-before-filter, UnitAI.h:173-174); no
-- the DamageIncrease/Decrease ApplyStatPctModifier hack (including
-- the Reset PHASE_THREE rollback) is PORTED (ApplyDamagePctModifier,
-- 2026-10-26); no
-- movement model — the charge AttackStart arm is skipped (cast lands,
-- the boss keeps its current victim); the summoned-spider
-- JustSummoned AttackStart-random arm has no bearer (nothing ever
-- hatches the eggs yet); spell_hatch_spiders (SpellScript target-select sort/
-- resize) is a SpellScript — not modeled (standing gap; note the
-- script hooks TARGET_GAMEOBJECT_DEST_AREA (52) while the DBC target
-- is NEARBY_ENTRY (40), so the hook likely never fires in C++ either).

local ENTRY_MARLI = 14510
local ENTRY_SPAWN = 15041
local GO_SPIDER_EGG = 179985

local SPELL_CHARGE = 22911
local SPELL_ASPECT_OF_MARLI = 24686
local SPELL_ENVOLWINGWEB = 24110
local SPELL_POISON_VOLLEY = 24099
local SPELL_SPIDER_FORM = 24084
local SPELL_LEVELUP = 24312
local SPELL_HATCH_EGGS = 24083
local SPELL_HATCH_SPIDER_EGG = 24082

local SAY_AGGRO = 0
local SAY_TRANSFORM = 1
local SAY_SPIDER_SPAWN = 2
local SAY_DEATH = 3

local PHASE_TWO = 2
local PHASE_THREE = 3

local POWER_MANA = 0

local timers = {}
local state = {}
local summons = {}

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

-- C++ SelectTarget(Random, 1, 100, true) (boss_marli.cpp:187):
-- UnitAI.h builds the list victim-first, so offset 1 pops the
-- victim ("not aggro leader" = the current victim) BEFORE the
-- player/alive/dist filters (akilzon idiom); GetAITargets is
-- threat-sorted, so a positional i > 1 drop would cut the threat
-- head instead under fixate/taunt. Then a random surviving
-- threat-list player within 100 yd is picked.
local function randomThreatPlayerSkipFirstInRange(creature, maxDist)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local out = {}
    local reach = creature:GetCombatReach()
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - reach - 1.5 < maxDist then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ JustSummoned (boss_marli.cpp:117-121): SelectTarget(Random, 0,
-- 0.f, true) — a random alive threat-list player, ANY distance, no
-- skip-first (unlike the charge arm's Random,1). GetAITargets is
-- threat-sorted; no positional drop, just player/alive filters.
local function randomThreatPlayer(creature)
    local out = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            out[#out + 1] = unit
        end
    end
    if #out == 0 then
        return nil
    end
    return out[math.random(#out)]
end

-- C++ EVENT_POISON_VOLLEY: triggered DoCastVictim(24099); re-arm
-- {10s,20s}. Phase-all: fires in every phase, C++-exact. C++
-- ScheduleEvent never dedups — every TRANSFORM_BACK schedules a fresh
-- 15s cycle alongside the surviving pre-transform cycle, so the volley
-- accumulates one concurrent cycle per transform-back (malown
-- duplicate-schedule precedent). Each cycle carries its own timer key.
local function onPoisonVolley(creature, guid, key)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_POISON_VOLLEY, true)
    end
    schedule(guid, key, math.random(10000, 20000), function()
        onPoisonVolley(creature, guid, key)
    end)
end

-- Arms one independent 15s -> {10s,20s} poison cycle. Called at combat
-- start and at every transform-back, C++-exact.
local function armPoisonCycle(creature, guid)
    local st = marliState(guid)
    st.poisonCycles = (st.poisonCycles or 0) + 1
    local key = "poisonvolley" .. st.poisonCycles
    schedule(guid, key, 15000, function()
        onPoisonVolley(creature, guid, key)
    end)
end

-- C++ EVENT_HATCH_SPIDER_EGG: non-triggered self-cast 24082; re-arm
-- {12s,17s}. Phase-all EXCEPT the transform CancelEvent kills the
-- cycle (re-armed fresh {12s,17s} at transform-back, C++-exact). The
-- cast is ISSUED (24082 is ACTIVATE_OBJECT/Open on nearby egg GOs,
-- see header DBC verdict); it reaches the Go activate-object arm but
-- stays dormant until the ownerless-Open engine leg lands (the arm
-- delegates Open to an owner session, wild casters no-op).
local function onHatchEgg(creature, guid)
    creature:CastSpell(creature, SPELL_HATCH_SPIDER_EGG)
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
-- the C++ charges the LAST draw even when all three miss a mana user
-- (a nil draw just clears the target for the next try, C++-exact);
-- DoCast(target, 22911) non-triggered; re-arm 8s phase-THREE. Handler
-- no-ops outside phase 3 (C++ phase mask).
local function onChargePlayer(creature, guid)
    local st = marliState(guid)
    if st.phase ~= PHASE_THREE then
        return
    end
    local target = nil
    for _ = 1, 3 do
        target = randomThreatPlayerSkipFirstInRange(creature, 100)
        if target and target:GetPowerType() == POWER_MANA then
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
-- web; the +35% damage stat hack has no bridge and the -100%
-- threat wipe is PORTED 2026-10-24 (gate + cut, C++ order); phase -> 3 with the phase-2 timers dropped (C++ phase mask);
-- arm charge 1.5s and transform-back 25s, both phase-THREE.
onTransform = function(creature, guid)
    local st = marliState(guid)
    if st.phase ~= PHASE_TWO then
        return
    end
    creature:Talk(SAY_TRANSFORM)
    creature:CastSpell(creature, SPELL_SPIDER_FORM)
    -- C++ EVENT_TRANSFORM (boss_marli.cpp:171): spider form +35%
    -- weapon damage via ApplyStatPctModifier (TOTAL_PCT).
    creature:ApplyDamagePctModifier(35)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_ENVOLWINGWEB)
        if creature:GetThreat(victim) ~= 0 then
            creature:ModifyThreatByPercent(victim, -100)
        end
    end
    cancelTimer(guid, "aspect")
    cancelTimer(guid, "hatchegg") -- C++ CancelEvent(EVENT_HATCH_SPIDER_EGG)
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
-- stat hack is PORTED below (inverse of +35%); phase -> 2; re-arm
-- aspect 12s, transform 45s + {35s,60s}, poison volley 15s, hatch
-- egg {12s,17s}.
onTransformBack = function(creature, guid)
    local st = marliState(guid)
    if st.phase ~= PHASE_THREE then
        return
    end
    creature:RemoveAura(SPELL_SPIDER_FORM)
    -- C++ EVENT_TRANSFORM_BACK (boss_marli.cpp:208): the rollback is
    -- the inverse of +35% (100/1.35-100), restoring the group to 1.0.
    creature:ApplyDamagePctModifier(100 / 1.35 - 100)
    cancelTimer(guid, "charge")
    st.phase = PHASE_TWO
    schedule(guid, "aspect", 12000, function()
        onAspectOfMarli(creature, guid)
    end)
    armTransformTimers(creature, guid, 45000, math.random(35000, 60000))
    armPoisonCycle(creature, guid)
    schedule(guid, "hatchegg", math.random(12000, 17000), function()
        onHatchEgg(creature, guid)
    end)
end

-- C++ EVENT_SPAWN_START_SPIDERS (1s after engage): Talk
-- (SAY_SPIDER_SPAWN), the 24083 AoE hatch (DoCastAOE, non-triggered —
-- self-cast, DoCastAOE convention) is ISSUED; arm aspect 12s,
-- transform 45s (duplicate skipped — inert),
-- poison volley 15s, hatch egg 30s; phase -> 2.
local function onSpawnStartSpiders(creature, guid)
    creature:Talk(SAY_SPIDER_SPAWN)
    creature:CastSpell(creature, SPELL_HATCH_EGGS)
    marliState(guid).phase = PHASE_TWO
    schedule(guid, "aspect", 12000, function()
        onAspectOfMarli(creature, guid)
    end)
    armTransformTimers(creature, guid, 45000)
    armPoisonCycle(creature, guid)
    schedule(guid, "hatchegg", 30000, function()
        onHatchEgg(creature, guid)
    end)
end

local function marliEnterCombat(event, creature, target)
    -- C++ JustEngagedWith (boss_marli.cpp:112-114) calls
    -- BossAI::JustEngagedWith first: SetBossState(DATA_MARLI 2,
    -- IN_PROGRESS) (ScriptedCreature.cpp:537).
    creature:SetBossState(2, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    marliState(guid).phase = 1
    creature:Talk(SAY_AGGRO)
    schedule(guid, "spawnstart", 1000, function()
        onSpawnStartSpiders(creature, guid)
    end)
end

local function marliWipe(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    summons[guid] = nil
end

-- C++ Reset's spider-egg scan (boss_marli.cpp:91-99): every spider
-- egg within the default 250yd grid scan is Respawn()d (+
-- UpdateObjectVisibility(true), which has no bridge — the Respawn
-- unhide is the modeled state change). Alive-gated (vancleef
-- precedent): C++ Reset runs at spawn and on evade (CreatureAI::
-- _EnterEvadeMode ends with Reset()), never on death — while Go's
-- 23 also fires on death (fireCreatureDied, Eluna parity).
local function marliReset(event, creature)
    -- C++ Reset (boss_marli.cpp:92): the PHASE_THREE damage rollback
    -- runs before the egg scan; the alive gate restores C++
    -- Reset-never-runs-on-death (Go fires 23 before 4 on death).
    -- summons.DespawnAll (:102) is PORTED below: tracked summons
    -- (ev19) are despawned before the wipe clears the list.
    local guid = creature:GetGUIDLow()
    local wasPhaseThree = marliState(guid).phase == PHASE_THREE
    local tracked = summons[guid]
    if tracked then
        for _, summon in pairs(tracked) do
            summon:Despawn()
        end
    end
    marliWipe(event, creature)
    if creature:IsAlive() then
        if wasPhaseThree then
            creature:ApplyDamagePctModifier(100 / 1.35 - 100)
        end
        for _, egg in ipairs(creature:GetGameObjectsInGrid(GO_SPIDER_EGG, 250)) do
            egg:Respawn()
        end
        -- C++ Reset tail (boss_marli.cpp:103): _Reset() runs last,
        -- writing SetBossState(DATA_MARLI 2, NOT_STARTED), skipped when
        -- already DONE (ScriptedCreature.cpp:499-511).
        if creature:GetBossState(2) ~= 3 then
            creature:SetBossState(2, 0)
        end
    end
end

RegisterCreatureEvent(ENTRY_MARLI, 1, marliEnterCombat)
RegisterCreatureEvent(ENTRY_MARLI, 2, marliWipe)
RegisterCreatureEvent(ENTRY_MARLI, 4, function(event, creature, killer)
    -- C++ JustDied (boss_marli.cpp:106-108): _JustDied() first, writing
    -- SetBossState(DATA_MARLI 2, DONE) (ScriptedCreature.cpp:513-519),
    -- then Talk(SAY_DEATH).
    creature:SetBossState(2, 3)
    creature:Talk(SAY_DEATH)
    marliWipe(event, creature)
end)
RegisterCreatureEvent(ENTRY_MARLI, 23, marliReset)

-- C++ JustSummoned (boss_marli.cpp:117-121): creature->AI()->
-- AttackStart(SelectTarget(Random, 0, 0.f, true)) + summons.Summon
-- (creature). Fires for any spider the hatch chain summons (ev19 is
-- live on the summon path, lua_creature_events.go:190); dormant
-- until the egg GO chain hatches one (24082/24083 reach the Go
-- activate-object arm but the ownerless-Open leg is still missing).
local function marliJustSummoned(event, creature, summoned)
    if summoned:GetEntry() ~= ENTRY_SPAWN then
        return
    end
    local guid = creature:GetGUIDLow()
    local tracked = summons[guid]
    if not tracked then
        tracked = {}
        summons[guid] = tracked
    end
    tracked[summoned:GetGUIDLow()] = summoned
    local target = randomThreatPlayer(creature)
    if target then
        summoned:AttackStart(target)
    end
end

RegisterCreatureEvent(ENTRY_MARLI, 19, marliJustSummoned)

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
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "levelup", 3000, function()
        onSpawnLevelUp(creature, guid)
    end)
end)
RegisterCreatureEvent(ENTRY_SPAWN, 2, function(event, creature)
    cancelTimers(creature:GetGUIDLow())
end)
RegisterCreatureEvent(ENTRY_SPAWN, 4, function(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end)
RegisterCreatureEvent(ENTRY_SPAWN, 23, function(event, creature)
    cancelTimers(creature:GetGUIDLow())
end)
