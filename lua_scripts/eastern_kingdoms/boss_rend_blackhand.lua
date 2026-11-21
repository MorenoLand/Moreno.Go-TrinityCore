-- Warchief Rend Blackhand (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_rend_blackhand.cpp
-- (boss_rend_blackhand — BossAI combat scheduler). Boss-script unit
-- per eastern_kingdoms_script_loader.cpp order (boss_gyth done,
-- registered for 10339; AddSC_boss_rend_blackhand next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h:70 names
-- NPC_WARCHIEF_REND_BLACKHAND = 10429 in the BRS creatures enum; the
-- AI runs under BossAI(DATA_WARCHIEF_REND_BLACKHAND = 10). The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (gyth / pyroguard_emberseer precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h). SetBossState legs (BossAI DATA_WARCHIEF_REND_
-- BLACKHAND = 10, blackrock_spire.h:40; < EncounterCount 23):
-- ev1 IN_PROGRESS via base BossAI::JustEngagedWith (boss_rend_
-- blackhand.cpp:186, base call first; _JustEngagedWith writes
-- IN_PROGRESS at ScriptedCreature.cpp:537); ev23 NOT_STARTED via
-- Reset's _Reset() (:176-182; _Reset gates on alive + state != DONE,
-- ScriptedCreature.cpp:499-511) — Reset's gythEvent/victorGUID/
-- portcullisGUID clears are pre-fight state, unmodeled; ev4 DONE via
-- JustDied's _JustDied() (:198-203; ScriptedCreature.cpp:513-520) —
-- JustDied's victor SetData(1, 2) arm stays documented-only below.
-- Bridge live (lua_creature_events.go:5917; gyth convention).
-- SPELL_FRENZY (8269) and SPELL_KNOCKDOWN (13360) are defined in the
-- file's spell enum but never cast by the AI — omitted by design
-- (shadowvosh ice-armor precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_WHIRLWIND: self-cast WHIRLWIND 13736, {13s,15s} init ->
-- {13s,18s} loop (C++ ScheduleEvent(EVENT_WHIRLWIND, 13s, 15s) init /
-- (EVENT_WHIRLWIND, 13s, 18s) loop, inclusive; CreateLuaEvent {min,max}
-- table is inclusive).
-- EVENT_CLEAVE: victim-cast CLEAVE 15284, {15s,17s} init ->
-- {10s,14s} loop (C++ inclusive ranges; CreateLuaEvent {min,max}).
-- EVENT_MORTAL_STRIKE: victim-cast MORTAL_STRIKE 16856, {17s,19s}
-- init -> {14s,16s} loop (C++ inclusive ranges; CreateLuaEvent
-- {min,max}). Victim casts are GetVictim nil-guarded (incarcerator
-- convention).
-- CASTING gates ported (combat branch only): C++ UpdateAI pre-loop
-- gate (:422) + in-loop gate (:443) are a 500ms IsCasting defer under
-- each arm's own key (live, lua_creature_events.go:1950; mal_ganis
-- idiom) -- a gated arm fires nothing and consumes no re-arm. The
-- gyth-event pre-fight branch has no gate. The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).
-- PORTED (2026-11-21): the pre-fight gyth event chain (boss_rend_
-- blackhand.cpp:205-411, UpdateAI gythEvent branch). Trigger: ev41
-- OnSetData (type AREATRIGGER = 1, data AREATRIGGER_BLACKROCK_STADIUM
-- = 2026) from at_blackrock_stadium.lua (RegisterServerEvent 24,
-- trigger 2026, TDB-verified in at_census_2026-11-15.md); the gythEvent
-- latch, victor GUID + portcullis GUID latches ride a per-guid Lua
-- table (victor/portcullis objects captured at trigger time, the
-- ObjectAccessor::GetCreature/GetGameObject re-resolve twin).
-- All 32 pre-fight events ported: Nefarius Talk 0-8 + Rend Talk 1-2
-- (creature:Talk), SetFacingToObject / SetFacingTo, PerformEmote
-- (TALK 1 / QUESTION 6 / ROAR 15 / POINT 25, SharedDefines.h),
-- MovePath(REND_PATH_1 1379680, false) + victor MovePath(NEFARIUS_
-- PATH_1 1379670, true), NearTeleportTo x2 (creature:NearTeleport),
-- SummonCreature(NPC_GYTH 10339, GythLoc, CORPSE_TIMED_DESPAWN 5 =
-- the C++ SummonCreature default; the Go bridge defaults to MANUAL
-- 7), portcullis UseDoorOrButton
-- on each EVENT_WAVE_1..6 (the wave spawn tables are fully commented
-- out in C++, so the wave bodies are the door toggle only).
-- EVENT_TURN_TO_PLAYER's SelectNearestPlayer(60) rides victor:
-- GetPlayersInGrid(60) + nearest-by-distance (alive-only, as C++).
-- C++ ScheduleEvent(..., 0s) legs (EVENT_TURN_TO_PLAYER) use a 1ms
-- Lua delay -- CreateLuaEvent rejects delay 0 (runtime.go parseTimerArgs).
-- DELTAS: (1) C++ runs both branches off ONE EventMap and calls
-- events.Update(diff) twice per tick while gythEvent + in combat; the
-- Lua port uses independent timer tables (combat vs pre-fight), so a
-- mid-chain engage advances the chain at wall-clock rate, not 2x.
-- (2) C++ MovementInform node 11 despawns with a 7-day respawn delay;
-- Go DespawnOrUnsummon takes ms only (respawn delay has no twin).
-- (3) IsSummonedBy's SetImmuneToPC(false) has no engine model
-- (boss_ai.go:16589); ev22 ports DoZoneInCombat() only.
-- (4) victor->AI()->SetData(1, 1)/(1, 2) are no-op dispatches in C++
-- (Nefarius has no AI script); the Lua SetData calls fire ev41 on
-- entry 10162 with no handler, same no-op.
-- SPELL_FRENZY (8269) and SPELL_KNOCKDOWN (13360) are defined in the
-- file's spell enum but never cast by the AI -- omitted by design
-- (shadowvosh ice-armor precedent).

local SPELL_WHIRLWIND = 13736
local SPELL_CLEAVE = 15284
local SPELL_MORTAL_STRIKE = 16856

local ENTRY_WARCHIEF_REND_BLACKHAND = 10429

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

local NPC_LORD_VICTOR_NEFARIUS = 10162
local NPC_GYTH = 10339
local GO_DR_PORTCULLIS = 175185
local NEFARIUS_PATH_1 = 1379670
local REND_PATH_1 = 1379680
local AREATRIGGER = 1 -- blackrock_spire.h:88
local AREATRIGGER_BLACKROCK_STADIUM = 2026 -- blackrock_spire.h:90
local EMOTE_ONESHOT_TALK = 1 -- SharedDefines.h:1938
local EMOTE_ONESHOT_QUESTION = 6 -- SharedDefines.h:1943
local EMOTE_ONESHOT_ROAR = 15 -- SharedDefines.h:1950
local EMOTE_ONESHOT_POINT = 25 -- SharedDefines.h:1960
local SUMMON_CORPSE_TIMED_DESPAWN = 5 -- Go 0-based TempSummonType; C++ Creature::SummonCreature default

-- Pre-fight gyth-event state, keyed by Rend GUID: the C++ victorGUID /
-- portcullisGUID latches (instance_blackrock_spire.cpp ObjectAccessor
-- re-resolve twin: objects captured at trigger time) + the gythEvent
-- latch. Separate timer table from the combat timers: C++ runs both
-- branches off one EventMap and never cancels the chain on engage, so
-- onEnterCombat must not touch these (they die on 2/4/23 via Reset).
local preFight = {}
local pfTimers = {}

local function pfCancelTimers(guid)
    local per = pfTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        pfTimers[guid] = nil
    end
end

local function pfSchedule(guid, key, delay, fn)
    local per = pfTimers[guid]
    if not per then
        per = {}
        pfTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

-- Forward declarations for the pre-fight chain (tharon_ja / gal_darah
-- idiom): the chain references later handlers from earlier ones, so the
-- locals are declared up front and assigned below, never re-declared
-- with `local function` (that would shadow the forward slot with nil).
local onStart1, onStart2, onStart3, onTurnToRend, onTurnToPlayer,
    onTurnToFacing1, onTurnToFacing2, onTurnToFacing3,
    onWavesEmote1, onWavesEmote2,
    onWavesText1, onWavesText2, onWavesText3, onWavesText4, onWavesText5,
    onWavesCompleteText1, onWavesCompleteText2, onWavesCompleteText3,
    onPathRend, onPathNefarius, onTeleport1, onTeleport2

local function pfVictor(guid)
    local state = preFight[guid]
    if state then
        return state.victor
    end
    return nil
end

-- C++ EVENT_TURN_TO_PLAYER (boss_rend_blackhand.cpp:248-252):
-- victor->SelectNearestPlayer(60.0f) -> SetFacingToObject(player).
-- GetPlayersInGrid returns alive players only, as C++.
local function onTurnToPlayerFn(creature, guid)
    local victor = pfVictor(guid)
    if not victor then
        return
    end
    local best = nil
    local bestDist = nil
    for _, plr in ipairs(victor:GetPlayersInGrid(60.0)) do
        local d = victor:GetDistance(plr)
        if bestDist == nil or d < bestDist then
            best = plr
            bestDist = d
        end
    end
    if best then
        victor:SetFacingToObject(best)
    end
end

-- C++ EVENT_WAVE_1..6 (boss_rend_blackhand.cpp:349-388): the spawn
-- tables are commented out, so each wave body is the portcullis
-- UseDoorOrButton only.
local function usePortcullis(creature, guid)
    local state = preFight[guid]
    if state and state.portcullis then
        state.portcullis:UseDoorOrButton()
    end
end

onStart1 = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(0) -- SAY_NEFARIUS_0
    end
    pfSchedule(guid, "start2", 4000, function() onStart2(creature, guid) end)
end

onStart2 = function(creature, guid)
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    local victor = pfVictor(guid)
    if victor then
        victor:PerformEmote(EMOTE_ONESHOT_POINT)
    end
    pfSchedule(guid, "start3", 4000, function() onStart3(creature, guid) end)
end

onStart3 = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(1) -- SAY_NEFARIUS_1
    end
    pfSchedule(guid, "wave1", 2000, function() usePortcullis(creature, guid) end)
    pfSchedule(guid, "turnrend", 4000, function() onTurnToRend(creature, guid) end)
    pfSchedule(guid, "wavestext1", 20000, function() onWavesText1(creature, guid) end)
end

onTurnToRend = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:SetFacingToObject(creature)
        victor:PerformEmote(EMOTE_ONESHOT_TALK)
    end
end

onTurnToPlayer = function(creature, guid)
    onTurnToPlayerFn(creature, guid)
end

onTurnToFacing1 = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:SetFacingTo(1.518436)
    end
end

onTurnToFacing2 = function(creature, guid)
    creature:SetFacingTo(1.658063)
end

onTurnToFacing3 = function(creature, guid)
    creature:SetFacingTo(1.500983)
end

onWavesEmote1 = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:PerformEmote(EMOTE_ONESHOT_QUESTION)
    end
end

onWavesEmote2 = function(creature, guid)
    creature:PerformEmote(EMOTE_ONESHOT_ROAR)
end

onWavesText1 = function(creature, guid)
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(2) -- SAY_NEFARIUS_2
    end
    creature:PerformEmote(EMOTE_ONESHOT_TALK)
    pfSchedule(guid, "turnfacing1", 4000, function() onTurnToFacing1(creature, guid) end)
    pfSchedule(guid, "wavesemote1", 5000, function() onWavesEmote1(creature, guid) end)
    pfSchedule(guid, "wave2", 2000, function() usePortcullis(creature, guid) end)
    pfSchedule(guid, "wavestext2", 20000, function() onWavesText2(creature, guid) end)
end

onWavesText2 = function(creature, guid)
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(3) -- SAY_NEFARIUS_3
    end
    pfSchedule(guid, "turnfacing1", 4000, function() onTurnToFacing1(creature, guid) end)
    pfSchedule(guid, "wave3", 2000, function() usePortcullis(creature, guid) end)
    pfSchedule(guid, "wavestext3", 20000, function() onWavesText3(creature, guid) end)
end

onWavesText3 = function(creature, guid)
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(4) -- SAY_NEFARIUS_4
    end
    pfSchedule(guid, "turnfacing1", 4000, function() onTurnToFacing1(creature, guid) end)
    pfSchedule(guid, "wave4", 2000, function() usePortcullis(creature, guid) end)
    pfSchedule(guid, "wavestext4", 20000, function() onWavesText4(creature, guid) end)
end

onWavesText4 = function(creature, guid)
    creature:Talk(0) -- SAY_BLACKHAND_1 (boss_rend_blackhand.cpp:33; 0, not 1)
    pfSchedule(guid, "wavesemote2", 4000, function() onWavesEmote2(creature, guid) end)
    pfSchedule(guid, "turnfacing3", 8000, function() onTurnToFacing3(creature, guid) end)
    pfSchedule(guid, "wave5", 2000, function() usePortcullis(creature, guid) end)
    pfSchedule(guid, "wavestext5", 20000, function() onWavesText5(creature, guid) end)
end

onWavesText5 = function(creature, guid)
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(5) -- SAY_NEFARIUS_5
    end
    pfSchedule(guid, "turnfacing1", 4000, function() onTurnToFacing1(creature, guid) end)
    pfSchedule(guid, "wave6", 2000, function() usePortcullis(creature, guid) end)
    pfSchedule(guid, "wavescomplete1", 20000, function() onWavesCompleteText1(creature, guid) end)
end

onWavesCompleteText1 = function(creature, guid)
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(6) -- SAY_NEFARIUS_6
    end
    pfSchedule(guid, "turnfacing1", 4000, function() onTurnToFacing1(creature, guid) end)
    pfSchedule(guid, "wavescomplete2", 13000, function() onWavesCompleteText2(creature, guid) end)
end

onWavesCompleteText2 = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(7) -- SAY_NEFARIUS_7
    end
    creature:Talk(1) -- SAY_BLACKHAND_2 (boss_rend_blackhand.cpp:34; 1, not 2)
    pfSchedule(guid, "pathrend", 1000, function() onPathRend(creature, guid) end)
    pfSchedule(guid, "wavescomplete3", 4000, function() onWavesCompleteText3(creature, guid) end)
end

onWavesCompleteText3 = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:Talk(8) -- SAY_NEFARIUS_8
    end
    pfSchedule(guid, "pathnefarius", 1000, function() onPathNefarius(creature, guid) end)
    pfSchedule(guid, "pathrend", 1000, function() onPathRend(creature, guid) end)
end

onPathNefarius = function(creature, guid)
    local victor = pfVictor(guid)
    if victor then
        victor:MovePath(NEFARIUS_PATH_1, true)
    end
end

onPathRend = function(creature, guid)
    creature:MovePath(REND_PATH_1, false)
end

onTeleport1 = function(creature, guid)
    creature:NearTeleport(194.2993, -474.0814, 121.4505, -0.01225555)
    pfSchedule(guid, "teleport2", 50000, function() onTeleport2(creature, guid) end)
end

onTeleport2 = function(creature, guid)
    creature:NearTeleport(216.485, -434.93, 110.888, -0.01225555)
    creature:SummonCreature(NPC_GYTH, 211.762, -397.5885, 111.1817, 4.747295, SUMMON_CORPSE_TIMED_DESPAWN)
end

-- C++ SetData (boss_rend_blackhand.cpp:205-224): type == AREATRIGGER
-- && data == AREATRIGGER_BLACKROCK_STADIUM, gythEvent latch, victor
-- (5.0f) + portcullis (50.0f) latches, EVENT_TURN_TO_PLAYER 0s +
-- EVENT_START_1 1s.
local function onSetData(_, creature, setType, setValue)
    if setType ~= AREATRIGGER or setValue ~= AREATRIGGER_BLACKROCK_STADIUM then
        return
    end
    local guid = creature:GetGUIDLow()
    local state = preFight[guid]
    if state and state.active then
        return
    end
    local victor = creature:FindNearestCreature(NPC_LORD_VICTOR_NEFARIUS, 5.0, true)
    local portcullis = creature:FindNearestGameObject(GO_DR_PORTCULLIS, 50.0)
    preFight[guid] = { active = true, victor = victor, portcullis = portcullis }
    pfSchedule(guid, "turnplayer", 1, function() onTurnToPlayer(creature, guid) end)
    pfSchedule(guid, "start1", 1000, function() onStart1(creature, guid) end)
end

-- C++ MovementInform (boss_rend_blackhand.cpp:226-243): WAYPOINT_MOTION
-- _TYPE node 5 -> EVENT_TELEPORT_1 in 2s; node 11 -> Gyth SetData(1, 1)
-- + DespawnOrUnsummon(1s, 7_days).
local function onMovementInform(_, creature, moveType, pointId)
    if moveType ~= 2 then -- WAYPOINT_MOTION_TYPE
        return
    end
    local guid = creature:GetGUIDLow()
    if pointId == 5 then
        pfSchedule(guid, "teleport1", 2000, function() onTeleport1(creature, guid) end)
    elseif pointId == 11 then
        local gyth = creature:FindNearestCreature(NPC_GYTH, 10.0, true)
        if gyth then
            gyth:SetData(1, 1)
        end
        creature:DespawnOrUnsummon(1000)
    end
end

-- C++ IsSummonedBy (boss_rend_blackhand.cpp:193-197): SetImmuneToPC
-- (false) has no engine model (boss_ai.go:16589); DoZoneInCombat only.
local function onSummoned(_, creature)
    creature:DoZoneInCombat()
end

local function onWhirlwind(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "whirlwind", 500, function() onWhirlwind(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", {13000, 18000}, function() onWhirlwind(creature, guid) end)
end

local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onCleave(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {10000, 14000}, function() onCleave(creature, guid) end)
end

local function onMortalStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mortalstrike", 500, function() onMortalStrike(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MORTAL_STRIKE)
    end
    schedule(guid, "mortalstrike", {14000, 16000}, function() onMortalStrike(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_rend_blackhand.cpp:186 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_rend_blackhand.cpp:186 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_WARCHIEF_REND_BLACKHAND=10 (blackrock_spire.h:40).
    creature:SetBossState(10, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "whirlwind", {13000, 15000}, function() onWhirlwind(creature, guid) end)
    schedule(guid, "cleave", {15000, 17000}, function() onCleave(creature, guid) end)
    schedule(guid, "mortalstrike", {17000, 19000}, function() onMortalStrike(creature, guid) end)
end

-- C++ LeaveCombat clears nothing: the pre-fight EventMap survives
-- target death / combat end (only Reset/ev23 clears it), so the chain
-- keeps running if the players wipe mid-intro. Combat timers only.
local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ JustDied (boss_rend_blackhand.cpp:198-203): _JustDied()
-- bookkeeping (timer cancel) + SetBossState(DATA_WARCHIEF_REND_
-- BLACKHAND=10, DONE) + victor SetData(1, 2).
local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    pfCancelTimers(guid)
    preFight[guid] = nil
    creature:SetBossState(10, 3)
    local victor = creature:FindNearestCreature(NPC_LORD_VICTOR_NEFARIUS, 75.0, true)
    if victor then
        victor:SetData(1, 2)
    end
end

-- C++ Reset (boss_rend_blackhand.cpp:176-182): _Reset() first, then
-- the gythEvent/victorGUID/portcullisGUID clears (unmodeled pre-fight
-- state). The _Reset() leg is alive-gated (Go fires ev23 on the death
-- path too) and skipped when already DONE (ScriptedCreature.cpp:499-511).
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    pfCancelTimers(guid)
    preFight[guid] = nil
    if creature:IsAlive() and creature:GetBossState(10) ~= 3 then
        creature:SetBossState(10, 0)
    end
end

RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 4, onDied)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 6, onMovementInform)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 22, onSummoned)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 23, onReset)
RegisterCreatureEvent(ENTRY_WARCHIEF_REND_BLACKHAND, 41, onSetData)
