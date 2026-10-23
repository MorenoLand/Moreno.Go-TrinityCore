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
-- (PORTED 2026-10-23, C++ :170 in arm order after Talk) + the three timer
-- casts (event 1; the BossAI::JustEngagedWith bookkeeping
-- (setActive / CheckRequiredBosses / SetBossState IN_PROGRESS),
-- and the FetchDragons machine (PowerOf Tenebron /
-- Shadron / Vesperon casts 61248 / 58105 / 61251, WILL_OF_SARTHARION
-- 61254, drake loot-mode ladder, MotionMaster MovePoints to the
-- drake init positions, UNIT_FLAG_NON_ATTACKABLE legs — all gated
-- on instance GUIDs and the MotionMaster) plus the remaining
-- ScheduleEvent calls (lava strike / flame tsunami / call tenebron /
-- shadron / vesperon) have no bridges — the tharon_ja / malygos
-- BossAI-bookkeeping and no-cast / no-timer-event precedents).
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
-- JustDied — Talk(SAY_SARTHARION_DEATH 6) (event 4; the _JustDied
-- bookkeeping has no bridge — tharon_ja precedent; the three drake
-- kills via instance->GetGuidData(DATA_TENEBRON / DATA_SHADRON /
-- DATA_VESPERON) (NPC_TENEBRON 30452 / NPC_SHADRON 30451 /
-- NPC_VESPERON 30449) have no instance bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 28860 — no bridgeable arms):
-- Reset / Initialize — Initialize flag clears (no Lua latches);
-- the HasAura(60639)->RemoveAura(60639) + SetHomePosition
-- (3246.57, 551.263, 58.6164, 4.66003) legs are PORTED in
-- sartharionReset 2026-10-20 (C++ :152-155 order exact);
-- DrakeRespawn (instance-boss-state-gated drake respawn +
-- MoveTargetedHome + UNIT_FLAG_NON_ATTACKABLE legs — no
-- instance / motion bridges);
-- SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) — no instance bridge.
-- JustReachedHome — _Reset() bookkeeping — no bridge.
-- AddDrakeLootMode — LOOT_MODE_HARD_MODE_1/2/3 ladder — no
-- loot-mode bridge.
-- FetchDragons — instance GUID fetch + IsInCombat / GetVictim
-- gates + casts + MovePoints + loot modes — no instance /
-- motion / cast bridges (drake entries verifiable — 30452 /
-- 30451 / 30449 — but the arms are bridge-blocked, so no
-- separate registration).
-- CallDragon (EVENT_CALL_TENEBRON / SHADRON / VESPERON legs) —
-- entry-switched Talk(SAY_SARTHARION_CALL_SHADRON 3 /
-- CALL_TENEBRON 4 / CALL_VESPERON 5) + AddAura power-of legs +
-- MovePoint(POINT_ID_LAND) legs — no timer-event / cast /
-- motion bridges.
-- GetData TWILIGHT_ACHIEVEMENTS (drakeCount) — no achievement
-- bridge; joins the unmodeled-achievement queue.
-- CastLavaStrikeOnTarget — Cell::VisitAllObjects fire-cyclone
-- search (NPC_FIRE_CYCLONE 30648) + random-cyclone CastSpell
-- (SPELL_LAVA_STRIKE 57571) on target (GetCreaturesInGrid IS
-- bridged — lua_creature_events.go:1204 — but the arm stays out:
-- NPC_FIRE_CYCLONE has no script summon site and SPELL_EFFECT_SUMMON
-- has no Go case, so the cyclones never materialize; EVENT_LAVA_STRIKE
-- rides the unported nine-event machine).
-- UpdateAI — the nine-event machine's remaining legs:
-- EVENT_HARD_ENRAGE SPELL_PYROBUFFET 56916 triggered (dead arm —
-- declared but never scheduled; porting would fabricate casts);
-- EVENT_FLAME_TSUNAMI WHISPER_LAVA_CHURN 9 Talk leg — PORTED
-- (event 2, unconditional 30s/30s — the Talk precedes the summon
-- wave branch, so no summon bridge is needed); the NPC_FLAME_
-- TSUNAMI 30616 wave summons + MovePoints stay out (no summon /
-- motion bridges); EVENT_LAVA_STRIKE
-- random-target lava strike + urand SAY_SARTHARION_SPECIAL 7;
-- EVENT_CALL_* dragon calls) + the 35%-health SPELL_BERSERK 61632
-- leg (Talk SAY_SARTHARION_BERSERK 1) gated on the three drake
-- boss states + the 10%-health soft-enrage lava-strike cadence
-- shift — no timer-event / summon / target-search / instance /
-- health bridges.
-- Whisper arms (WHISPER_LAVA_CHURN 9) — ported below (event 2;
-- the 30s init Talk + unconditional 30s re-arm are bridgeable,
-- summon waves stay out).

local ENTRY_SARTHARION = 28860

local SAY_SARTHARION_AGGRO  = 0
local SAY_SARTHARION_BREATH = 2
local SAY_SARTHARION_DEATH  = 6
local SAY_SARTHARION_SLAY   = 8
local WHISPER_LAVA_CHURN    = 9

local SPELL_CLEAVE       = 56909
local SPELL_FLAME_BREATH = 56908
local SPELL_TAIL_LASH    = 56910
local SPELL_TWILIGHT_REVENGE = 60639

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

-- C++ EVENT_FLAME_TSUNAMI: Talk(WHISPER_LAVA_CHURN 9) (unconditional),
-- 30s init, unconditional 30s re-arm. The summon-wave legs stay out
-- (no summon / motion bridges); only the Talk is ported.
local function lavaChurnTick(creature, guid)
    creature:Talk(WHISPER_LAVA_CHURN)
    schedule(guid, "lavachurn", 30000, function()
        lavaChurnTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (setActive /
-- CheckRequiredBosses / SetBossState IN_PROGRESS) + Talk(AGGRO)
-- + DoZoneInCombat (PORTED 2026-10-23 — creature:DoZoneInCombat, the old
-- "no bridge" claim was stale; bridge live at
-- engine/world/lua_creature_events.go:426) + FetchDragons (drake power-aura casts, loot
-- modes, drake init MovePoints, non-attackable flags) +
-- ScheduleEvent x9 — the Talk and the three timer casts are
-- bridgeable; the rest are machine legs with no bridges.
-- C++ DrakeRespawn :237/:257/:277 tenebron/shadron/vesperon->AI()->SetData(DATA_CAN_LOOT, 0):
-- cross-AI write into the unported drake AIs (obsidian_sanctum.cpp dummy_dragonAI, :326 _canLoot gate).
-- No cross-AI SetData bridge and the target AIs are unported — dropped, documented.
local function sartharionEnterCombat(event, creature, target)
    creature:Talk(SAY_SARTHARION_AGGRO)
    creature:DoZoneInCombat() -- C++ :170, in arm order after Talk (live bridge lua_creature_events.go; 2026-10-23)
    local guid = creature:GetGUID()
    cancelTimers(guid)
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
-- DisappearAndDie legs via instance GUIDs — only the Talk arm
-- is bridgeable.
local function sartharionDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_SARTHARION_DEATH)
end

local function sartharionReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ Reset :152-155, in order: the RemoveAurasDueToSpell
    -- (SPELL_TWILIGHT_REVENGE) leg is HasAura-gated (bridged, the
    -- nefarius precedent), then SetHomePosition(literals) (event-
    -- surface bridge); the Initialize flag clears have no Lua
    -- latches, DrakeRespawn + SetBossState are instance-gated.
    if creature:HasAura(SPELL_TWILIGHT_REVENGE) then
        creature:RemoveAura(SPELL_TWILIGHT_REVENGE)
    end
    creature:SetHomePosition(3246.57, 551.263, 58.6164, 4.66003)
end

RegisterCreatureEvent(ENTRY_SARTHARION, 1, sartharionEnterCombat)
RegisterCreatureEvent(ENTRY_SARTHARION, 2, sartharionLeaveCombat)
RegisterCreatureEvent(ENTRY_SARTHARION, 3, sartharionTargetDied)
RegisterCreatureEvent(ENTRY_SARTHARION, 4, sartharionDied)
RegisterCreatureEvent(ENTRY_SARTHARION, 23, sartharionReset)
