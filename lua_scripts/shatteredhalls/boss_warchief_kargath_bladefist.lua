-- Warchief Kargath Bladefist (Shattered Halls, Hellfire
-- Citadel) — Lua port of src/server/scripts/Outland/
-- HellfireCitadel/ShatteredHalls/boss_warchief_kargath_
-- bladefist.cpp (boss_warchief_kargath_bladefist — the only
-- creature AI class AddSC registers; GetShatteredHallsAI is
-- an AI-factory helper, not a script).
-- Final boss in the Shattered Halls set per
-- outland_script_loader.cpp order (instance_shattered_halls.
-- lua ported 3fe088b3).
-- Entry: NPC_KARGATH_BLADEFIST = 16808 from shattered_halls.
-- h (SHCreatureIds), mapped in instance_shattered_halls.cpp
-- OnCreatureCreate (line 111) plus GetGuidData (line 170).
-- The creature_template ScriptName bindings are DB-side (TDB
-- present at ~/.cache/tc-db/TDB_full_world_335.26091_2026_09_09.sql;
-- 16808/17621/17622/17623/17695/17301 verified 2026-11-14, round-71).
-- Talk: SAY_AGGRO = 0 fires from the C++ JustEngagedWith
-- (C++-exact); SAY_SLAY = 1 fires from the C++ KilledUnit
-- override gated on the victim being a player (C++-exact);
-- SAY_DEATH = 2 fires from the C++ JustDied (the _JustDied
-- arm is instance-blocked); SAY_CALL_EXECUTIONER_A = 3 /
-- SAY_CALL_EXECUTIONER_H = 4 fire only from the DoAction
-- (ACTION_EXECUTIONER_TAUNT = 1, shattered_halls.h) arm,
-- which reads instance->GetData(DATA_TEAM_IN_INSTANCE) —
-- instance gated, unreachable in this model.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied,
-- 4 OnDied. A scheduler pump exists for the Assassins_Timer arm
-- only (see below); every other combat timer's fire arm is
-- blocked on a missing bridge, so nothing else observable remains
-- to tick; melee is engine-driven in Go (creature combat tick),
-- like C++.
-- Fight shape (C++-exact for all modeled arms): Kargath
-- (16808): OnEnterCombat(1) — SetBossState(2, 1) IN_PROGRESS
-- (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537) +
-- Talk(SAY_AGGRO) + the assassin one-shot (5s) + the guard
-- wave pump (30s init) + the resetcheck evade pump (5s);
-- the C++ JustEngagedWith override does nothing else, and the
-- Initialize() schedule (Blade_Dance_Timer 45000 /
-- Summon_Assistant_Timer 30000 / Assassins_Timer 5000 /
-- resetcheck_timer 5000 / summoned 2) drives bridge-blocked arms
-- except Assassins_Timer, so per-GUID state is kept only for the
-- assassin pump (fired flag + stored summon objects).
-- OnTargetDied(3): Talk(SAY_SLAY) gated on the killed unit
-- being a player (C++ `victim->GetTypeId() == TYPEID_PLAYER`
-- via the terestian/gurtogg 3-arg handler, C++-exact).
-- OnDied(4): Talk(SAY_DEATH) + SetBossState(2, 3) DONE (the
-- _JustDied twin) + the removeAdds assassin/guard sweep +
-- the Kargath-DONE executioner Reset relay (instance_
-- shattered_halls.cpp:153-157 twin, PORTED below via
-- FindNearestCreature + ev38). OnReset(23): the C++ Reset()
-- SetSpeedRate(MOVE_RUN, 2) arm, ported via creature:SetSpeedRate
-- (the SetWalk(false) leg is PORTED in kargathReset via creature:SetWalk; removeAdds/_Reset/
-- Initialize have no observable remainder in this model, omrogg
-- "no event 3" precedent) + SetBossState(2, 0) NOT_STARTED
-- (the _Reset twin, ScriptedCreature.cpp:499-510). Melee is
-- engine-driven.
-- Deliberate deviations (all await engine bridges): no
-- instance-script model for the boss-state bookkeeping —
-- boss admission via the luaBossAI shim (the BossAI ctor
-- DATA_KARGATH = 2 bookkeeping in Reset/JustEngagedWith/
-- JustDied is PORTED as the SetBossState writes above, not
-- skipped); the DoAction(ACTION_EXECUTIONER_TAUNT) arm stays
-- out — its only caller is the unported at_nethekurse_exit
-- areatrigger (shattered_halls.cpp:49), so no ev38 handler
-- would ever fire;
-- the Assassins_Timer 5s arm (four NPC_SHATTERED_ASSASSIN
-- 17695 summons at AssassEntrance/AssassExit with
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) is PORTED 2026-10-24
-- via a 5s one-shot pump — the stale "no summon bridge" claim
-- is retracted (SummonCreature is bridged; C++ JustSummoned
-- for assassins only tracks GUIDs, no AttackStart relay to
-- miss). The Summon_Assistant_Timer arm is PORTED 2026-10-24:
-- 30s init then urand(25s,35s) waves of `summoned` guards
-- (init 2, +1 on urand(0,9) < 2), urand guard entry at
-- AddsEntrance, each guard AttackStarted onto a random
-- threat target via summoned:EngageWithTarget; no movement bridge — the whole
-- blade-dance machine unmodeled: Blade_Dance_Timer 45s init
-- then 30s sets target_num = 5 / Wait_Timer = 1 / InBlade =
-- true / SetSpeedRate(MOVE_RUN, 4), the Wait_Timer arm drives
-- MovePoint(1) hops to random points around (210, -60) and
-- MovementInform(POINT_MOTION_TYPE, 1) fires the triggered
-- DoCast(me, SPELL_BLADE_DANCE 30739, true) with target_num--
-- until target_num hits 0, then InBlade = false /
-- SetSpeedRate(MOVE_RUN, 2) / MoveChase(victim) /
-- Blade_Dance_Timer = 30000 — none of it can run without a
-- movement bridge; the speed-rate arms (SetSpeedRate MOVE_RUN is
-- bridged, lua_creature_events.go:6065) stay out with the unbridged
-- MovePoint/MovementInform machine; the IsHeroic()-gated Charge_timer = 5000 arm at
-- blade-dance end and its DoCast(SelectTarget(Random, 0),
-- H_SPELL_CHARGE 25821) fire arm stay unmodeled — the "no
-- difficulty bridge" claim was stale and is retracted
-- (creature:IsHeroic() is bridged, lua_creature_events.go:4417),
-- but the arm still has no landing point: Charge_timer is only
-- ever armed by the blade-dance-end leg (no movement bridge —
-- MovePoint/MovementInform); the fire arm's random threat-list pick
-- rides the GetAITargets convention (gruul second-threat
-- precedent), so the arm's sole blocker is that MovementInform
-- trigger, not SelectTarget; porting the gate alone would
-- fabricate a fire arm with no trigger (thespia precedent);
-- the JustSummoned guard AttackStart relay is ported via
-- summoned:EngageWithTarget (random threat target); the
-- removeAdds sweep is ported for assassins AND guards via
-- stored summon objects; the resetcheck_timer 5s evade arm
-- (EnterEvadeMode when x is outside [205, 255]) is PORTED
-- 2026-11-10 via a 5s pump + creature:GetX() + EnterEvadeMode
-- (black stalker precedent — the old "no position/home bridge"
-- claim was stale); no
-- unit-state bridge — the C++ UpdateAI skips
-- DoMeleeAttackIfReady entirely while InBlade (no melee
-- during blade dance) and there is no bridge to suppress
-- engine melee; the ScriptData "SD%Complete: 90" is an
-- upstream caveat, documented, not bridged; no SpellScript/
-- AuraScript scripts in this file.

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local ENTRY_KARGATH_BLADEFIST = 16808
local ENTRY_SHATTERED_ASSASSIN = 17695
local ENTRY_SHATTERED_EXECUTIONER = 17301
-- Lua-side transport id (no C++ counterpart — the executioner
-- AI has no DoAction override): fired on the executioner when
-- Kargath dies, so its ev38 handler runs the C++ Reset arms
-- (instance_shattered_halls.cpp:155-156 calls executioner->
-- AI()->Reset() on every DATA_KARGATH state change; only the
-- DONE transition changes the executioner's observable state,
-- so the relay fires on death only).
local ACTION_KARGATH_DONE_RESET = 2
local ENTRY_HEARTHEN_GUARD = 17621
local ENTRY_SHARPSHOOTER_GUARD = 17622
local ENTRY_REAVER_GUARD = 17623
-- C++ AddsEntrance (boss_warchief_kargath_bladefist.cpp:64);
-- guard summon type 3, 30s (TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT).
local ADDS_ENTRANCE = { 306.036, -84.29, 1.93 }
-- C++ AssassEntrance/AssassExit (boss_warchief_kargath_bladefist.cpp:62-63);
-- SpawnAssassin (:195-198) summons at y +/- 8, orientation 0,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (bridge type 3), 30s.
local ASSASSIN_SPOTS = {
    {275.136, -84.29 + 8, 2.3},
    {275.136, -84.29 - 8, 2.3},
    {184.233, -84.29 + 8, 2.3},
    {184.233, -84.29 - 8, 2.3},
}

local kargathTimers = {}
local kargathResetTimers = {}
local kargathAssassins = {}
local kargathAssassinFired = {}
local kargathGuards = {}
local kargathGuardTimers = {}
local kargathSummoned = {}

local function cancelKargath(guid)
    local id = kargathTimers[guid]
    if id then
        RemoveEventById(id)
        kargathTimers[guid] = nil
    end
    local resetId = kargathResetTimers[guid]
    if resetId then
        RemoveEventById(resetId)
        kargathResetTimers[guid] = nil
    end
    local guardId = kargathGuardTimers[guid]
    if guardId then
        RemoveEventById(guardId)
        kargathGuardTimers[guid] = nil
    end
    kargathAssassinFired[guid] = nil
    kargathSummoned[guid] = nil
end

-- C++ removeAdds (:175-191) for the assassin list; the guard list
-- is swept by despawnGuards below (guards ported 2026-10-24).
local function despawnAssassins(guid)
    local list = kargathAssassins[guid]
    if list then
        for _, add in ipairs(list) do
            if add then
                add:Despawn()
            end
        end
        kargathAssassins[guid] = nil
    end
end

local function despawnGuards(guid)
    local list = kargathGuards[guid]
    if list then
        for _, add in ipairs(list) do
            if add then
                add:Despawn()
            end
        end
        kargathGuards[guid] = nil
    end
end

local function randomThreatTarget(creature)
    local alive = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

local GUARD_ENTRIES = {
    ENTRY_HEARTHEN_GUARD, ENTRY_SHARPSHOOTER_GUARD, ENTRY_REAVER_GUARD,
}

-- C++ Summon_Assistant_Timer (boss_warchief_kargath_bladefist.cpp:279-301):
-- every wave summons `summoned` guards (init 2, +1 on urand(0,9) < 2),
-- each a urand(0,2) guard entry at AddsEntrance; JustSummoned (:132-141)
-- AttackStarts each guard onto a random target, ported via
-- summoned:EngageWithTarget below; re-arm urand(25000, 35000).
local function kargathGuardTick(creature, guid)
    local summoned = kargathSummoned[guid] or 2
    local list = kargathGuards[guid] or {}
    for _ = 1, summoned do
        local guard = creature:SummonCreature(GUARD_ENTRIES[math.random(3)],
            ADDS_ENTRANCE[1], ADDS_ENTRANCE[2], ADDS_ENTRANCE[3], 0, 3, 30000)
        if guard then
            list[#list + 1] = guard
            local target = randomThreatTarget(creature)
            if target ~= nil then
                guard:EngageWithTarget(target)
            end
        end
    end
    kargathGuards[guid] = list
    if math.random(0, 9) < 2 then
        kargathSummoned[guid] = summoned + 1
    end
    kargathGuardTimers[guid] = CreateLuaEvent(function()
        kargathGuardTick(creature, guid)
    end, math.random(25000, 35000))
end

local function kargathEnterCombat(event, creature)
    creature:SetBossState(2, 1) -- DATA_KARGATH, IN_PROGRESS (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537; gruul precedent)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
    kargathAssassinFired[guid] = false
    kargathAssassins[guid] = {}
    kargathGuards[guid] = {}
    kargathSummoned[guid] = 2
    -- C++ resetcheck_timer (UpdateAI :342-353): 5s init then 5s —
    -- EnterEvadeMode when the boss's x is outside [205, 255]
    -- (uint32 truncation; math.floor matches for positive x, and
    -- negative x evades either way). Runs only in combat (the C++
    -- check sits past the !UpdateVictim return), so the pump
    -- starts here and dies on events 2/4/23.
    kargathResetTimers[guid] = CreateLuaEvent(function()
        local x = math.floor(creature:GetX())
        if x > 255 or x < 205 then
            creature:EnterEvadeMode()
        end
    end, 5000, 0)
    kargathGuardTimers[guid] = CreateLuaEvent(function()
        kargathGuardTick(creature, guid)
    end, 30000)
    -- C++ Assassins_Timer 5000 one-shot (UpdateAI :207-213):
    -- SpawnAssassin() fires once, then Assassins_Timer = 0.
    -- The 5s pump re-fires but is fired-guarded; canceled on
    -- events 2/4/23. C++ JustSummoned for assassins (:139-141)
    -- only tracks GUIDs (no AttackStart relay), so the summon
    -- arm is complete without a cross-creature bridge.
    kargathTimers[guid] = CreateLuaEvent(function()
        if kargathAssassinFired[guid] then
            return
        end
        kargathAssassinFired[guid] = true
        local list = kargathAssassins[guid] or {}
        for _, spot in ipairs(ASSASSIN_SPOTS) do
            local add = creature:SummonCreature(ENTRY_SHATTERED_ASSASSIN, spot[1], spot[2], spot[3], 0, 3, 30000)
            if add then
                list[#list + 1] = add
            end
        end
        kargathAssassins[guid] = list
    end, 5000)
end

local function kargathLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelKargath(guid)
    despawnAssassins(guid)
    despawnGuards(guid)
end

local function kargathDied(event, creature, killer)
    creature:SetBossState(2, 3) -- DATA_KARGATH, DONE (boss_warchief_kargath_bladefist.cpp:120-124: _JustDied then Talk)
    creature:Talk(SAY_DEATH)
    -- C++ instance SetBossState override (instance_shattered_halls.
    -- cpp:153-157): on DATA_KARGATH DONE the executioner's AI
    -- Reset runs (IMMUNE_TO_PC flag clear); no creature:Reset()
    -- bridge exists, so the relay fires its ev38 Reset action
    -- (boss_shattered_executioner.lua; nil-guarded, as in C++).
    local executioner = creature:FindNearestCreature(ENTRY_SHATTERED_EXECUTIONER, 1000.0)
    if executioner then
        executioner:DoAction(ACTION_KARGATH_DONE_RESET)
    end
    local guid = creature:GetGUIDLow()
    cancelKargath(guid)
    despawnAssassins(guid)
    despawnGuards(guid)
end

local function kargathReset(event, creature)
    -- C++ Reset() :114: SetSpeedRate(MOVE_RUN, 2) + SetWalk(false), both
    -- PORTED below; the rest of Reset (removeAdds/_Reset/Initialize)
    -- has no observable remainder in this model except the assassin
    -- sweep above (documented above).
    -- C++ _Reset (ScriptedCreature.cpp:499-510): SetBossState
    -- DATA_KARGATH, NOT_STARTED — ev23 fires on death too (engine
    -- reset-then-died order); the NOT_STARTED write lands before
    -- the ev4 DONE write, so the final state stays DONE (gruul
    -- precedent).
    creature:SetBossState(2, 0) -- DATA_KARGATH, NOT_STARTED
    creature:SetSpeedRate(1, 2)
    creature:SetWalk(false)
    local guid = creature:GetGUIDLow()
    cancelKargath(guid)
    despawnAssassins(guid)
    despawnGuards(guid)
end

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 1, kargathEnterCombat)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 2, kargathLeaveCombat)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 3, function(_, creature, victim)
    -- C++ `if (victim->GetTypeId() == TYPEID_PLAYER)
    -- Talk(SAY_SLAY)` — terestian/gurtogg 3-arg-handler
    -- convention (C++-exact).
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 4, kargathDied)

RegisterCreatureEvent(ENTRY_KARGATH_BLADEFIST, 23, kargathReset)
