-- Sartharion (Obsidian Sanctum) — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/ObsidianSanctum/boss_sartharion.cpp
-- (boss_sartharion (CreatureScript) via
-- GetObsidianSanctumAI<boss_sartharionAI> (BossAI, DATA_SARTHARION);
-- registered from inside AddSC_boss_sartharion(); loader decl 94 /
-- call 289 per northrend_script_loader.cpp — the FIRST group of the
-- "// Obsidian Sanctum" block in AddNorthrendScripts(), immediately
-- after AddSC_instance_eye_of_eternity() (decl 92 / call 287),
-- under the "// Obsidian Sanctum" marker; the call after it is
-- AddSC_obsidian_sanctum() (decl 95 / call 290) — loader order
-- confirmed this run; the checkpoint sequence
-- (instance_eye_of_eternity -> boss_sartharion) is followed).
-- Entry: 28860 Sartharion (obsidian_sanctum.h NPC_SARTHARION, line 40;
-- DATA_SARTHARION = 0, line 30; EncounterCount = 5, line 26);
-- instance_obsidian_sanctum.cpp OnCreatureCreate binds case
-- NPC_SARTHARION (line 51) — entry-verifiable, registration proceeds
-- (the nexus_commanders / malygos kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_sartharion"
-- hits boss_sartharion.cpp (+ the loader decl/call lines for
-- AddSC_boss_sartharion) only; zero sql/ hits. No sartharion lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers ride the CreateLuaEvent
-- convention (baltharus / maiden precedent): GUID-keyed timers
-- table, cancel on events 2/4/23; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_SARTHARION_AGGRO 0) + DoZoneInCombat
-- (PORTED 2026-10-23, C++ :170 in arm order after Talk) + SetBossState
-- (DATA_SARTHARION, IN_PROGRESS) PORTED 2026-11-05 (tick 254,
-- creature:SetBossState — the malygos precedent) + the FetchDragons
-- machine (PowerOf Tenebron / Shadron / Vesperon casts 61248 / 58105 /
-- 61251, WILL_OF_SARTHARION 61254, drake loot-mode ladder, MoveTo
-- (MovePoint twin) to the drake init positions, UNIT_FLAG_NON_ATTACKABLE
-- legs — PORTED 2026-11-05 (tick 254) via GetInstanceCreature +
-- CastSpell + AddLootMode; the INIT-arrival MovementInform machine
-- is PORTED 2026-11-07 (tick 394, dragonMovementInform in
-- obsidian_sanctum.lua, ev28 live tick 392)) + the
-- call-dragon one-shot seeds (30s/75s/120s, PORTED tick 254) and the
-- remaining ScheduleEvent calls (lava strike / flame tsunami); the
-- BossAI::JustEngagedWith bookkeeping (setActive / CheckRequiredBosses)
-- stays out (tharon_ja / malygos precedent).
-- EVENT_CLEAVE_ATTACK — DoCastVictim(SPELL_CLEAVE 56909) 1-arg
-- non-triggered, 7s init / {7s,10s} re-arm (:175 / :461-:464).
-- EVENT_FLAME_BREATH — Talk(SAY_SARTHARION_BREATH 2) +
-- DoCastVictim(SPELL_FLAME_BREATH 56908) non-triggered, 20s init /
-- {25s,35s} re-arm (:176 / :452-:456).
-- EVENT_TAIL_SWEEP — DoCastVictim(SPELL_TAIL_LASH 56910)
-- non-triggered, 20s init / {15s,20s} re-arm (:177 / :457-:460).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SARTHARION_SLAY 8) — PORTED (event 3, the
-- razuvious player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player").
-- JustDied — Talk(SAY_SARTHARION_DEATH 6) (event 4; PORTED 2026-11-05
-- (tick 254): SetBossState(DATA_SARTHARION, DONE) — the _JustDied twin,
-- malygos precedent — + the three drake despawns via
-- GetInstanceCreature (DespawnOrUnsummon substitutes for the unbridged
-- DisappearAndDie: no death-state/corpse leg; respawn stays governed by
-- DrakeRespawn's GetBossState ~= DONE gate)).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28860 — no bridgeable arms):
-- Reset / Initialize — Initialize flag clears (no Lua latches);
-- the HasAura(60639)->RemoveAura(60639) + SetHomePosition
-- (3246.57, 551.263, 58.6164, 4.66003) legs are PORTED in
-- sartharionReset 2026-10-20 (C++ :152-155 order exact);
-- DrakeRespawn PORTED 2026-11-05 (tick 254) in drakeRespawn below
-- (GetInstanceCreature + SetHomePosition + MoveHome, the
-- MoveTargetedHome twin, + Respawn + AI SetData(DATA_CAN_LOOT, 0) —
-- the store write nothing reads yet; the drake-side _canLoot read
-- stays out on the default-0 trap, see obsidian_sanctum.lua);
-- SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) PORTED in sartharionReset
-- (creature:SetBossState, malygos precedent).
-- JustReachedHome — _Reset() PORTED 2026-11-05 (tick 254) via ev24
-- (CreatureEventOnReachHome, fires on MoveHome arrival):
-- SetBossState(0, 0) when not DONE.
-- AddDrakeLootMode — LOOT_MODE_HARD_MODE_1/2/3 ladder (0x2/0x4/0x8)
-- PORTED 2026-11-05 (tick 254) in addDrakeLootMode below, called from
-- fetchDragons (HasLootMode/AddLootMode bridged).
-- FetchDragons — PORTED 2026-11-05 (tick 254) in fetchDragons below
-- (GetInstanceCreature + IsAlive / IsInCombat / GetVictim gates +
-- self-casts 61248 / 58105 / 61251 / 61254 + loot-mode ladder +
-- MoveTo(100, init) + NON_ATTACKABLE SetFlag; the INIT-arrival
-- MovementInform free-movement machine is PORTED 2026-11-07 (tick 394,
-- dragonMovementInform in obsidian_sanctum.lua).
-- CallDragon (EVENT_CALL_TENEBRON / SHADRON / VESPERON legs) —
-- PORTED 2026-11-05 (tick 254) in callDragon below (one-shot
-- 30s/75s/120s seeds; entry-switched Talk(SAY_SARTHARION_CALL_SHADRON
-- 3 / CALL_TENEBRON 4 / CALL_VESPERON 5) + AddAura power-of legs +
-- MoveTo(200, land); the LAND-arrival engage (MoveChase + AddThreat)
-- is PORTED 2026-11-07 (tick 394, dragonMovementInform in
-- obsidian_sanctum.lua).
-- GetData TWILIGHT_ACHIEVEMENTS (drakeCount) — no achievement
-- bridge; joins the unmodeled-achievement queue.
-- CastLavaStrikeOnTarget — Cell::VisitAllObjects fire-cyclone
-- search (NPC_FIRE_CYCLONE 30648) + random-cyclone CastSpell
-- (SPELL_LAVA_STRIKE 57571) on target (GetCreaturesInGrid IS
-- bridged — lua_creature_events.go:1204 — and the arm is PORTED in
-- lavaStrikeTick below (NPC_FIRE_CYCLONE has no C++ summon site, so
-- cyclones arrive via DB/spell data; the C++ empty-pool guard makes
-- the port a no-op when none is present; SPELL_EFFECT_SUMMON now has
-- a Go case, boss_ai.go creatureEffectSummonType).
-- UpdateAI — the nine-event machine's remaining legs:
-- EVENT_HARD_ENRAGE SPELL_PYROBUFFET 56916 triggered (dead arm —
-- declared but never scheduled; porting would fabricate casts);
-- EVENT_FLAME_TSUNAMI WHISPER_LAVA_CHURN 9 Talk leg — PORTED
-- (event 2, unconditional 30s/30s — the Talk precedes the summon
-- wave branch, so no summon bridge is needed); the NPC_FLAME_
-- TSUNAMI 30616 wave summons PORTED 2026-11-05 (tick 254) in
-- lavaChurnTick (SummonCreature type 2 + MoveTo(0, direction); the
-- C++ leap motion stays unmodeled — single straight-line spline);
-- EVENT_LAVA_STRIKE — PORTED (lavaStrikeTick,
-- including the 10%-health soft-enrage cadence shift via the
-- GetHealthPct sticky latch);
-- EVENT_CALL_* dragon calls PORTED (callDragon one-shot seeds) +
-- the 35%-health SPELL_BERSERK 61632 leg (Talk SAY_SARTHARION_BERSERK
-- 1) PORTED 2026-11-05 (tick 254) as a 1s berserkPollTick (GetHealthPct
-- + GetBossState drake gate; 1s poll vs per-tick C++ check, documented
-- granularity deviation).
-- Whisper arms (WHISPER_LAVA_CHURN 9) — ported below (event 2;
-- the 30s init Talk + unconditional 30s re-arm are bridgeable,
-- summon waves stay out).

local ENTRY_SARTHARION = 28860

local DATA_SARTHARION = 0
local DATA_TENEBRON = 1
local DATA_SHADRON = 2
local DATA_VESPERON = 3
local DATA_PORTAL_OPEN = 4
local DATA_CAN_LOOT = 0
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SAY_SARTHARION_AGGRO  = 0
local SAY_SARTHARION_BERSERK = 1
local SAY_SARTHARION_BREATH = 2
local SAY_SARTHARION_CALL_SHADRON = 3
local SAY_SARTHARION_CALL_TENEBRON = 4
local SAY_SARTHARION_CALL_VESPERON = 5
local SAY_SARTHARION_DEATH  = 6
local SAY_SARTHARION_SPECIAL = 7
local SAY_SARTHARION_SLAY   = 8
local WHISPER_LAVA_CHURN    = 9

local SPELL_BERSERK = 61632
local SPELL_CLEAVE       = 56909
local SPELL_FLAME_BREATH = 56908
local SPELL_TAIL_LASH    = 56910
local SPELL_LAVA_STRIKE  = 57571
local SPELL_WILL_OF_SARTHARION = 61254
local SPELL_POWER_OF_TENEBRON = 61248
local SPELL_POWER_OF_SHADRON = 58105
local SPELL_POWER_OF_VESPERON = 61251
local NPC_FIRE_CYCLONE   = 30648
local NPC_FLAME_TSUNAMI = 30616
local SPELL_TWILIGHT_REVENGE = 60639

local LOOT_MODE_HARD_MODE_1 = 0x2
local LOOT_MODE_HARD_MODE_2 = 0x4
local LOOT_MODE_HARD_MODE_3 = 0x8

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

local POINT_ID_INIT = 100
local POINT_ID_LAND = 200

local timers = {}

local softEnraged = {}

local berserked = {}

local drakeCount = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    softEnraged[guid] = nil
    berserked[guid] = nil
    drakeCount[guid] = nil
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

-- C++ EVENT_CLEAVE_ATTACK: DoCastVictim(SPELL_CLEAVE 56909)
-- (non-triggered), 7s init, re-arm {7s,10s}.
local function cleaveTick(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "cleave", {7000, 10000}, function()
        cleaveTick(creature, guid)
    end)
end

-- C++ EVENT_FLAME_BREATH: Talk(SAY_SARTHARION_BREATH 2) +
-- DoCastVictim(SPELL_FLAME_BREATH 56908) (non-triggered), 20s init,
-- re-arm {25s,35s}.
local function flameBreathTick(creature, guid)
    creature:Talk(SAY_SARTHARION_BREATH)
    creature:CastSpell(nil, SPELL_FLAME_BREATH)
    schedule(guid, "flamebreath", {25000, 35000}, function()
        flameBreathTick(creature, guid)
    end)
end

-- C++ EVENT_TAIL_SWEEP: DoCastVictim(SPELL_TAIL_LASH 56910)
-- (non-triggered), 20s init, re-arm {15s,20s}.
local function tailSweepTick(creature, guid)
    creature:CastSpell(nil, SPELL_TAIL_LASH)
    schedule(guid, "tailsweep", {15000, 20000}, function()
        tailSweepTick(creature, guid)
    end)
end

-- C++ EVENT_LAVA_STRIKE: SelectTarget(Random, 0) -> CastLavaStrikeOnTarget:
-- random NPC_FIRE_CYCLONE 30648 within 200y casts SPELL_LAVA_STRIKE 57571
-- (triggered) on the target; urand(0,5)==0 -> Talk(SAY_SARTHARION_SPECIAL 7).
-- 5s init; re-arm {5s,20s}, or {1400ms,2s} once soft-enraged (sticky
-- HealthBelowPct(10) latch, boss_sartharion.cpp:503-505). Empty cyclone
-- pool returns early (C++-exact guard); the re-arm is unconditional
-- (nil target still re-arms, C++ :465-475).
local function lavaStrikeTick(creature, guid)
    if creature:GetHealthPct() <= 10 then
        softEnraged[guid] = true
    end
    local targets = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            targets[#targets + 1] = t
        end
    end
    if #targets > 0 then
        local target = targets[math.random(#targets)]
        local cyclones = creature:GetCreaturesInGrid(NPC_FIRE_CYCLONE, 200.0)
        if #cyclones > 0 then
            cyclones[math.random(#cyclones)]:CastSpell(target, SPELL_LAVA_STRIKE, true)
        end
        -- C++ :469-470: the Talk rides the target-if, NOT the empty-pool
        -- early return inside CastLavaStrikeOnTarget (:402-403) — fixed
        -- 2026-11-05 (tick 254, S-D1: was nested inside the cyclone-if).
        if math.random(0, 5) == 0 then
            creature:Talk(SAY_SARTHARION_SPECIAL)
        end
    end
    if softEnraged[guid] then
        schedule(guid, "lavastrike", {1400, 2000}, function()
            lavaStrikeTick(creature, guid)
        end)
    else
        schedule(guid, "lavastrike", {5000, 20000}, function()
            lavaStrikeTick(creature, guid)
        end)
    end
end

-- C++ EVENT_FLAME_TSUNAMI: Talk(WHISPER_LAVA_CHURN 9) (unconditional),
-- 30s init, unconditional 30s re-arm. The summon-wave legs are PORTED
-- 2026-11-05 (tick 254): urand(0,1) picks the 3-right / 2-left wave
-- (boss_sartharion.cpp:429-449); each tsunami summons TEMPSUMMON_TIMED_
-- DESPAWN (type 2, 12s) then MovePoint(0, direction) — MoveTo is the
-- MovePoint twin; the C++ leap motion stays unmodeled (straight-line
-- spline). Spawn/direction pairs are {x, y, z, dirX, dirY, dirZ}.
local FLAME_RIGHT_WAVES = {
    { 3200.00, 573.211, 57.1551, 3289.28, 573.211, 57.1551 },
    { 3200.00, 532.211, 57.1551, 3289.28, 532.211, 57.1551 },
    { 3200.00, 491.211, 57.1551, 3289.28, 491.211, 57.1551 },
}
local FLAME_LEFT_WAVES = {
    { 3289.28, 511.711, 57.1551, 3200.00, 511.711, 57.1551 },
    { 3289.28, 552.711, 57.1551, 3200.00, 552.711, 57.1551 },
}

local function lavaChurnTick(creature, guid)
    creature:Talk(WHISPER_LAVA_CHURN)
    local waves = FLAME_LEFT_WAVES
    if math.random(0, 1) == 0 then
        waves = FLAME_RIGHT_WAVES
    end
    for _, lane in ipairs(waves) do
        local wave = creature:SummonCreature(NPC_FLAME_TSUNAMI, lane[1], lane[2], lane[3], 0.0, 2, 12000)
        if wave then
            wave:MoveTo(0, lane[4], lane[5], lane[6])
        end
    end
    schedule(guid, "lavachurn", 30000, function()
        lavaChurnTick(creature, guid)
    end)
end

-- C++ UpdateAI 35%-health leg (boss_sartharion.cpp:492-498): once, when
-- health drops to 35% or below while any drake boss state is not DONE,
-- Talk(SAY_SARTHARION_BERSERK 1) + DoCast(SPELL_BERSERK 61632). The C++
-- check runs every UpdateAI tick; the Lua port polls every 1s (documented
-- granularity deviation). Depends on the drake JustDied SetBossState(DONE)
-- legs in obsidian_sanctum.lua (same tick) — without them GetBossState
-- reads 5 forever and the berserk would fire with all drakes dead.
local function berserkPollTick(creature, guid)
    if berserked[guid] then
        return
    end
    if creature:GetHealthPct() <= 35
        and (creature:GetBossState(DATA_TENEBRON) ~= BOSS_STATE_DONE
            or creature:GetBossState(DATA_SHADRON) ~= BOSS_STATE_DONE
            or creature:GetBossState(DATA_VESPERON) ~= BOSS_STATE_DONE) then
        creature:Talk(SAY_SARTHARION_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK)
        berserked[guid] = true
        return
    end
    schedule(guid, "berserk", 1000, function()
        berserkPollTick(creature, guid)
    end)
end

-- C++ AddDrakeLootMode (boss_sartharion.cpp:210-219): the
-- LOOT_MODE_HARD_MODE_1/2/3 ladder, called from FetchDragons per
-- drake pulled while not in combat.
local function addDrakeLootMode(creature)
    if creature:HasLootMode(LOOT_MODE_HARD_MODE_2) then
        creature:AddLootMode(LOOT_MODE_HARD_MODE_3)
    elseif creature:HasLootMode(LOOT_MODE_HARD_MODE_1) then
        creature:AddLootMode(LOOT_MODE_HARD_MODE_2)
    else
        creature:AddLootMode(LOOT_MODE_HARD_MODE_1)
    end
end

-- Drake init positions (boss_sartharion.cpp:108-124).
local DRAKE_INIT = {
    [DATA_TENEBRON] = { power = SPELL_POWER_OF_TENEBRON, pos = { 3212.854, 575.597, 109.856 } },
    [DATA_SHADRON] = { power = SPELL_POWER_OF_SHADRON, pos = { 3293.238, 472.223, 106.968 } },
    [DATA_VESPERON] = { power = SPELL_POWER_OF_VESPERON, pos = { 3193.310, 472.861, 102.697 } },
}

-- C++ FetchDragons (boss_sartharion.cpp:283-347), called from
-- JustEngagedWith: per drake (tenebron/shadron/vesperon order), if alive
-- and without a victim — _canUseWill = true; if not in combat, sartharion
-- self-casts the drake's power aura (DoCast(me, POWER) targets me),
-- climbs the loot-mode ladder, and counts the drake; then the drake flies
-- to its init point (MoveTo is the MovePoint twin) and gains
-- UNIT_FLAG_NON_ATTACKABLE if absent. Ends with WILL_OF_SARTHARION
-- (61254) on self when any drake was pulled. drakeCount is stored per
-- GUID for the latent GetData(TWILIGHT_ACHIEVEMENTS) arm (no achievement
-- bridge yet). The INIT-arrival MovementInform free-movement machine
-- is PORTED 2026-11-07 (tick 394, dragonMovementInform in
-- obsidian_sanctum.lua).
local function fetchDragons(creature, guid)
    creature:ResetLootMode()
    local count = 0
    local canUseWill = false
    for dataId = DATA_TENEBRON, DATA_VESPERON do
        local info = DRAKE_INIT[dataId]
        local drake = creature:GetInstanceCreature(dataId)
        if drake and drake:IsAlive() and not drake:GetVictim() then
            canUseWill = true
            if not drake:IsInCombat() then
                creature:CastSpell(creature, info.power)
                addDrakeLootMode(creature)
                count = count + 1
            end
            drake:MoveTo(POINT_ID_INIT, info.pos[1], info.pos[2], info.pos[3])
            if not drake:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) then
                drake:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
            end
        end
    end
    drakeCount[guid] = count
    if canUseWill then
        creature:CastSpell(creature, SPELL_WILL_OF_SARTHARION)
    end
end

-- Drake land positions (boss_sartharion.cpp:108-124).
local DRAKE_LAND = {
    [DATA_TENEBRON] = { power = SPELL_POWER_OF_TENEBRON, talk = SAY_SARTHARION_CALL_TENEBRON, pos = { 3246.425, 565.367, 61.249 } },
    [DATA_SHADRON] = { power = SPELL_POWER_OF_SHADRON, talk = SAY_SARTHARION_CALL_SHADRON, pos = { 3271.669, 526.907, 61.931 } },
    [DATA_VESPERON] = { power = SPELL_POWER_OF_VESPERON, talk = SAY_SARTHARION_CALL_VESPERON, pos = { 3227.268, 533.238, 59.995 } },
}

-- C++ CallDragon (boss_sartharion.cpp:349-384): if the drake is alive and
-- without a victim — walk off, clear NON_ATTACKABLE, self-add its power
-- aura, fly to the land point (MoveTo is the MovePoint twin), and
-- sartharion Talks the call line. The LAND-arrival engage (MoveChase +
-- AddThreat, obsidian_sanctum.cpp:217-226) is PORTED 2026-11-07 (tick 394,
-- dragonMovementInform in obsidian_sanctum.lua).
local function callDragon(creature, dataId)
    local drake = creature:GetInstanceCreature(dataId)
    if not (drake and drake:IsAlive() and not drake:GetVictim()) then
        return
    end
    drake:SetWalk(false)
    if drake:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) then
        drake:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    end
    local info = DRAKE_LAND[dataId]
    if info then
        drake:AddAura(info.power)
        drake:MoveTo(POINT_ID_LAND, info.pos[1], info.pos[2], info.pos[3])
        creature:Talk(info.talk)
    end
end

-- Drake home positions for DrakeRespawn (boss_sartharion.cpp:228-278).
local DRAKE_HOME = {
    [DATA_TENEBRON] = { 3239.07, 657.235, 86.8775, 4.74729 },
    [DATA_SHADRON] = { 3363.06, 525.28, 98.362, 4.76475 },
    [DATA_VESPERON] = { 3145.68, 520.71, 89.7, 4.64258 },
}

-- C++ DrakeRespawn (boss_sartharion.cpp:220-281), called from Reset: per
-- drake, set the home position; if alive, clear NON_ATTACKABLE and send
-- it home (MoveHome is the MoveTargetedHome twin); else, if its boss
-- state is not DONE, respawn it, send it home, and write
-- AI()->SetData(DATA_CAN_LOOT, 0) (the store write nothing reads yet —
-- the drake-side _canLoot read stays out on the default-0 trap).
local function drakeRespawn(creature)
    for dataId = DATA_TENEBRON, DATA_VESPERON do
        local drake = creature:GetInstanceCreature(dataId)
        if drake then
            local home = DRAKE_HOME[dataId]
            drake:SetHomePosition(home[1], home[2], home[3], home[4])
            if drake:IsAlive() then
                if drake:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) then
                    drake:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
                end
                drake:MoveHome()
            elseif creature:GetBossState(dataId) ~= BOSS_STATE_DONE then
                drake:Respawn()
                drake:MoveHome()
                drake:SetData(DATA_CAN_LOOT, 0)
            end
        end
    end
end

-- C++ JustEngagedWith: Talk(AGGRO) + BossAI::JustEngagedWith (setActive /
-- CheckRequiredBosses / SetBossState IN_PROGRESS + SetCombatPulseDelay(5)
-- PORTED 2026-11-03; base me-form DoZoneInCombat subsumed by the :170 call)
-- + DoZoneInCombat (PORTED 2026-10-23 — creature:DoZoneInCombat, the old
-- "no bridge" claim was stale; bridge live at
-- engine/world/lua_creature_events.go:426) + FetchDragons (drake power-aura casts, loot
-- modes, drake init MovePoints, non-attackable flags) +
-- ScheduleEvent x9 — the Talk and the timer casts are
-- bridgeable; the rest are machine legs with no bridges.
-- C++ DrakeRespawn :237/:257/:277 tenebron/shadron/vesperon->AI()->SetData(DATA_CAN_LOOT, 0):
-- cross-AI write into the unported drake AIs (obsidian_sanctum.cpp dummy_dragonAI, :326 _canLoot gate).
-- No cross-AI SetData bridge and the target AIs are unported — dropped, documented.
local function sartharionEnterCombat(event, creature, target)
    creature:Talk(SAY_SARTHARION_AGGRO)
    creature:SetBossState(DATA_SARTHARION, BOSS_STATE_IN_PROGRESS) -- C++ :167-169 (BossAI base arm; setActive/instance legs have no bridge)
    creature:SetCombatPulseDelay(5) -- boss_sartharion.cpp:169 (BossAI base arm; setActive/instance legs have no bridge)
    creature:DoZoneInCombat() -- C++ :170, in arm order after Talk (live bridge lua_creature_events.go; 2026-10-23)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    fetchDragons(creature, guid) -- C++ :172, in arm order after DoZoneInCombat
    schedule(guid, "cleave", 7000, function()
        cleaveTick(creature, guid)
    end)
    schedule(guid, "flamebreath", 20000, function()
        flameBreathTick(creature, guid)
    end)
    schedule(guid, "tailsweep", 20000, function()
        tailSweepTick(creature, guid)
    end)
    schedule(guid, "lavachurn", 30000, function()
        lavaChurnTick(creature, guid)
    end)
    schedule(guid, "lavastrike", 5000, function()
        lavaStrikeTick(creature, guid)
    end)
    -- C++ EVENT_CALL_* one-shot seeds (:178-180; the case arms never
    -- re-arm) + the 35% berserk poll (1s; see berserkPollTick).
    schedule(guid, "callTenebron", 30000, function()
        callDragon(creature, DATA_TENEBRON)
    end)
    schedule(guid, "callShadron", 75000, function()
        callDragon(creature, DATA_SHADRON)
    end)
    schedule(guid, "callVesperon", 120000, function()
        callDragon(creature, DATA_VESPERON)
    end)
    schedule(guid, "berserk", 1000, function()
        berserkPollTick(creature, guid)
    end)
end

local function sartharionLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: victim->GetTypeId() == TYPEID_PLAYER gate,
-- then Talk(SAY_SARTHARION_SLAY) — the razuvious player-gated
-- variant precedent.
local function sartharionTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SARTHARION_SLAY)
    end
end

-- C++ JustDied: _JustDied() + Talk(DEATH) + the three drake
-- DisappearAndDie legs via instance GUIDs — state DONE first (the
-- _JustDied twin, malygos precedent), then Talk, then the drake
-- despawns (DespawnOrUnsummon substitutes for the unbridged
-- DisappearAndDie: no death-state/corpse leg; respawn stays governed
-- by DrakeRespawn's GetBossState ~= DONE gate).
local function sartharionDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(DATA_SARTHARION, BOSS_STATE_DONE)
    creature:Talk(SAY_SARTHARION_DEATH)
    for dataId = DATA_TENEBRON, DATA_VESPERON do
        local drake = creature:GetInstanceCreature(dataId)
        if drake and drake:IsAlive() then
            drake:DespawnOrUnsummon()
        end
    end
end

local function sartharionReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset :152-158, in order: the RemoveAurasDueToSpell
    -- (SPELL_TWILIGHT_REVENGE) leg is HasAura-gated (bridged, the
    -- nefarius precedent), then SetHomePosition(literals) (event-
    -- surface bridge); the Initialize flag clears have no Lua
    -- latches, DrakeRespawn + SetBossState are instance-gated.
    if creature:HasAura(SPELL_TWILIGHT_REVENGE) then
        creature:RemoveAura(SPELL_TWILIGHT_REVENGE)
    end
    creature:SetHomePosition(3246.57, 551.263, 58.6164, 4.66003)
    drakeRespawn(creature) -- C++ :157
    creature:SetBossState(DATA_PORTAL_OPEN, BOSS_STATE_NOT_STARTED) -- C++ :158
end

-- C++ instance OnCreatureCreate latches the Sartharion GUID (DATA 0).
-- The live latch route is ev5 SetGuidData (eredar-twins precedent);
-- the instance file (instance_obsidian_sanctum.lua) mirrors the latch
-- for dynamically-materialized spawns.
local function sartharionSpawn(event, creature)
    creature:SetGuidData(DATA_SARTHARION)
end

-- C++ JustReachedHome: _Reset() bookkeeping — SetBossState(NOT_STARTED)
-- when not DONE. Fires on MoveHome arrival (event 24).
local function sartharionReachedHome(event, creature)
    if creature:GetBossState(DATA_SARTHARION) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_SARTHARION, BOSS_STATE_NOT_STARTED)
    end
end

RegisterCreatureEvent(ENTRY_SARTHARION, 1, sartharionEnterCombat)
RegisterCreatureEvent(ENTRY_SARTHARION, 2, sartharionLeaveCombat)
RegisterCreatureEvent(ENTRY_SARTHARION, 3, sartharionTargetDied)
RegisterCreatureEvent(ENTRY_SARTHARION, 4, sartharionDied)
RegisterCreatureEvent(ENTRY_SARTHARION, 23, sartharionReset)
RegisterCreatureEvent(ENTRY_SARTHARION, 5, sartharionSpawn)
RegisterCreatureEvent(ENTRY_SARTHARION, 24, sartharionReachedHome)
