-- Blackrock Depths: High Justice Grimstone — Lua port of the
-- Ring-of-Law event machine of
-- src/server/scripts/EasternKingdoms/BlackrockMountain/BlackrockDepths/blackrock_depths.cpp
-- (class npc_grimstone : CreatureScript("npc_grimstone");
-- struct npc_grimstoneAI : public EscortAI).
-- Entry: 10096 (TDB 335.26091 creature_template ScriptName
-- npc_grimstone; creature_text groups 0-5).
-- Eluna creature events: 6 OnReachWP, 23 OnReset, 4 OnDied.
-- Ported arms (C++ order, blackrock_depths.cpp:138-360):
-- - Reset: SetFlag(UNIT_FIELD_FLAGS 59, UNIT_FLAG_NON_ATTACKABLE 0x2).
--   The Initialize() half zeroes only Ring-of-Law UpdateAI-machine
--   state (EventPhase/Event_Timer/MobCount/MobDeath_Timer/RingMobGUIDs/
--   RingBossGUID/CanWalk), so the flag-set is the whole observable
--   static leg. TDB 335.26091 creature_template unit_flags for
--   10096 is 832 (0x340, no 0x2 bit), so without this Grimstone spawns
--   attackable in Go; the flag-set has real targeting effect
--   (lua_creature_events.go SetFlag :6915, motion.UnitFlags).
--   Reset also re-arms the event pump (Initialize Event_Timer 1000,
--   phase 0); MobSpawnId is rolled once per spawn, kept across
--   resets exactly as C++ (ctor-only roll, :141-143).
-- - UpdateAI event machine (blackrock_depths.cpp:238-360), driven by
--   a CreateLuaEvent pump (barnes idiom: per-GUID state, re-arm with
--   the smallest live timer, cancel on Reset/Died). The pump fires
--   one UpdateAI(diff) step per arm: the MobDeath_Timer poll first
--   (2500ms; boss latched returns early, freezing the phase machine
--   while the boss lives — :245-282), then the Event_Timer phase
--   switch (phases 0-10, :283-355, ++EventPhase after the switch).
--   Phase 0 Talks SAY_TEXT5 (4), closes ARENA4, StartEscort on the
--   summoner (the at_ring_of_law player) + MoveWaypoint (wilda
--   idiom; C++ Start(false, false) needs no player, the Go escort
--   arms the summoner as escort player), CanWalk. Phases 1/4/7/10
--   unpause the escort (CanWalk), wp0/1/2/4 pause it
--   (SetEscortPaused, the CanWalk twin). Phase 2 waits 2000ms;
--   phase 3 opens ARENA1; phase 4 hides + summons ring mob 1;
--   phase 5 summons mobs 2-3; phase 6 summons mob 4 (MobDeath poll
--   arms when MobCount hits 4); phase 7 (mob-death poll sets the
--   5000ms timer) shows + closes ARENA1 + Talks SAY_TEXT6 (5);
--   phase 8 opens ARENA2; phase 9 hides + summons the ring boss
--   (random of 6); phase 10 (boss-death poll sets the 5000ms timer)
--   closes ARENA2, opens ARENA3 + ARENA4. RingMob/RingBoss arrays
--   :75-94 (8925-8933 / 9027-9032); summon points :187/:199.
-- - Doors ride FindNearestGameObject by GO entry (gruul whole-
--   instance idiom, 1000y; static GOs never fire the instance
--   OnGameObjectCreate event in Go, instance_hooks.go:488-503) +
--   SetGoState (netherspite precedent: ACTIVE 0 = open, READY 1 =
--   closed), the HandleGameObject twin (InstanceScript.cpp:479:
--   open ? GO_STATE_ACTIVE : GO_STATE_READY). GO entries
--   blackrock_depths.cpp:52-55 (ARENA1 161525, ARENA2 161522,
--   ARENA3 161524, ARENA4 161523). GetGuidData is NOT used: the
--   arena GUIDs are latched only by C++ OnGameObjectCreate and no
--   Lua writer latches them, so GetGuidData would read 0.
-- - WaypointReached wp0 Talk(SAY_TEXT1 0), wp1 Talk(SAY_TEXT2 1),
--   wp3 Talk(SAY_TEXT3 2), wp4 Talk(SAY_TEXT4 3)
--   (GrimstoneTexts :117-122); wp0/1/4 also pause + set the 5000ms
--   phase timer, wp2 pauses (CanWalk=false only — thrall-wp98
--   precedent for the no-Talk arm); wp5 instance->SetData(
--   TYPE_RING_OF_LAW 1, DONE 3) (blackrock_depths.h:28; DONE 3,
--   InstanceScript.h:73) via the InstanceData:SetData bridge
--   (instance_hooks.go:51, fires event 8), nil-guarded through
--   instanceDataOf (old_hillsbrad precedent). The store landed
--   2026-11-15 (at_* census): instance_blackrock_depths.lua keeps
--   instanceData.ringOfLaw (SetData TYPE_RING_OF_LAW 1), read back
--   by the at_ring_of_law gate; the DONE write saves.
-- Deliberate omissions (documented, no bridge loss):
-- - The wp5 UpdateEncounterStateForKilledCreature leg has no bridge.
-- - The C++ @todo quest end-boss variant (:127) is unimplemented
--   upstream; nothing to port.
-- - The event starts at the at_ring_of_law trigger, ported in
--   lua_scripts/eastern_kingdoms/at_ring_of_law.lua (RegisterServerEvent
--   24, trigger 1526; summons Grimstone as the AT player's TempSummon,
--   which is what GetSummonerUnit reads in phase 0).

local ENTRY_GRIMSTONE = 10096

local SAY_TEXT1 = 0
local SAY_TEXT2 = 1
local SAY_TEXT3 = 2
local SAY_TEXT4 = 3
local SAY_TEXT5 = 4
local SAY_TEXT6 = 5

local TYPE_RING_OF_LAW = 1 -- blackrock_depths.h:28
local DONE = 3 -- InstanceScript.h:73

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2 -- UnitDefines.h:125

local GO_ARENA1 = 161525 -- blackrock_depths.cpp:52
local GO_ARENA2 = 161522 -- blackrock_depths.cpp:53
local GO_ARENA3 = 161524 -- blackrock_depths.cpp:54
local GO_ARENA4 = 161523 -- blackrock_depths.cpp:55
local GO_STATE_ACTIVE = 0 -- netherspite precedent: open
local GO_STATE_READY = 1 -- netherspite precedent: closed
local GO_SEARCH_DIST = 1000 -- whole-instance radius; one door per entry

local RING_MOB = { 8925, 8926, 8927, 8928, 8933, 8932 } -- :75-84
local RING_BOSS = { 9027, 9028, 9029, 9030, 9031, 9032 } -- :85-94
local MAX_NPC_AMOUNT = 4 -- :72

local SUMMON_TYPE_DEAD_DESPAWN = 6 -- at_ring_of_law.lua precedent
local MOB_DEATH_POLL_MS = 2500 -- :193/:201/:249

-- Per-GUID Grimstone state (Initialize() locals, :144-174).
local grimstoneState = {}

local function cancelPump(guid)
    local st = grimstoneState[guid]
    if st and st.pumpEventId then
        RemoveEventById(st.pumpEventId)
        st.pumpEventId = nil
    end
end

local function instanceDataOf(creature)
    local mapObj = creature:GetMap()
    if not mapObj then
        return nil
    end
    return mapObj:GetInstanceData()
end

-- C++ npc_grimstoneAI::HandleGameObject (:238-241) via the entry
-- resolution above: open ? GO_STATE_ACTIVE : GO_STATE_READY.
local function setArenaDoor(creature, entry, open)
    local go = creature:FindNearestGameObject(entry, GO_SEARCH_DIST)
    if go then
        go:SetGoState(open and GO_STATE_ACTIVE or GO_STATE_READY)
    end
end

-- C++ CanWalk gate (:357-359): the escort only advances while
-- CanWalk; the Go twin pauses/resumes the waypoint generator.
local function setCanWalk(creature, st, walk)
    if st.canWalk == walk then
        return
    end
    st.canWalk = walk
    creature:SetEscortPaused(not walk)
end

-- C++ SummonRingMob (:185-195): summon at the ring point, latch the
-- low GUID at the pre-increment slot, arm the death poll on the 4th.
local function summonRingMob(creature, st)
    local mob = creature:SummonCreature(RING_MOB[st.mobSpawnId + 1],
        608.960, -235.322, -53.907, 1.857, SUMMON_TYPE_DEAD_DESPAWN)
    if mob then
        st.ringMobGUIDs[st.mobCount + 1] = mob:GetGUIDLow()
    end
    st.mobCount = st.mobCount + 1
    if st.mobCount == MAX_NPC_AMOUNT then
        st.mobDeathTimer = MOB_DEATH_POLL_MS
    end
end

-- C++ SummonRingBoss (:197-203): random boss, arm the death poll.
local function summonRingBoss(creature, st)
    local boss = creature:SummonCreature(RING_BOSS[math.random(1, 6)],
        644.300, -175.989, -53.739, 3.418, SUMMON_TYPE_DEAD_DESPAWN)
    if boss then
        st.ringBossGUID = boss:GetGUIDLow()
    end
    st.mobDeathTimer = MOB_DEATH_POLL_MS
end

-- Forward declaration: grimstoneTick re-arms through armPump, which
-- is assigned below. A later `local function` here would shadow this
-- slot — plain assignment only (tharon_ja/gal_darah idiom).
local armPump

-- C++ UpdateAI MobDeath_Timer block (:245-282). Returns true when the
-- boss leg ran: C++ returns from UpdateAI there, skipping the phase
-- switch this tick (both on boss death and while the boss lives).
local function mobDeathPoll(creature, st)
    st.mobDeathTimer = MOB_DEATH_POLL_MS
    if st.ringBossGUID then
        local boss = creature:GetCreatureByGUID(st.ringBossGUID)
        if boss and (not boss:IsAlive()) and boss:IsDead() then
            st.ringBossGUID = nil
            st.eventTimer = 5000
            st.mobDeathTimer = 0
        end
        return true
    end
    for i = 1, MAX_NPC_AMOUNT do
        local guid = st.ringMobGUIDs[i]
        if guid then
            local mob = creature:GetCreatureByGUID(guid)
            if mob and (not mob:IsAlive()) and mob:IsDead() then
                st.ringMobGUIDs[i] = nil
                st.mobCount = st.mobCount - 1
                -- All gone: set the timer to continue, discontinue this.
                if st.mobCount == 0 then
                    st.eventTimer = 5000
                    st.mobDeathTimer = 0
                end
            end
        end
    end
    return false
end

-- C++ UpdateAI Event_Timer phase switch (:283-355). ++EventPhase runs
-- after the switch exactly as C++ does it.
local function runPhase(creature, st)
    local phase = st.phase
    if phase == 0 then
        creature:Talk(SAY_TEXT5)
        setArenaDoor(creature, GO_ARENA4, false)
        local summoner = creature:GetSummonerUnit()
        if summoner then
            creature:StartEscort(summoner)
        end
        creature:MoveWaypoint()
        setCanWalk(creature, st, true)
        st.eventTimer = 0
    elseif phase == 1 then
        setCanWalk(creature, st, true)
        st.eventTimer = 0
    elseif phase == 2 then
        st.eventTimer = 2000
    elseif phase == 3 then
        setArenaDoor(creature, GO_ARENA1, true)
        st.eventTimer = 3000
    elseif phase == 4 then
        setCanWalk(creature, st, true)
        creature:SetVisible(false)
        summonRingMob(creature, st)
        st.eventTimer = 8000
    elseif phase == 5 then
        summonRingMob(creature, st)
        summonRingMob(creature, st)
        st.eventTimer = 8000
    elseif phase == 6 then
        summonRingMob(creature, st)
        st.eventTimer = 5000
    elseif phase == 7 then
        creature:SetVisible(true)
        setArenaDoor(creature, GO_ARENA1, false)
        creature:Talk(SAY_TEXT6)
        setCanWalk(creature, st, true)
        st.eventTimer = 5000
    elseif phase == 8 then
        setArenaDoor(creature, GO_ARENA2, true)
        st.eventTimer = 5000
    elseif phase == 9 then
        creature:SetVisible(false)
        summonRingBoss(creature, st)
        st.eventTimer = 0
    elseif phase == 10 then
        setArenaDoor(creature, GO_ARENA2, false)
        setArenaDoor(creature, GO_ARENA3, true)
        setArenaDoor(creature, GO_ARENA4, true)
        setCanWalk(creature, st, true)
        st.eventTimer = 0
    end
    st.phase = st.phase + 1
end

-- C++ npc_grimstoneAI::UpdateAI (:238-360): the death poll first,
-- then the phase switch; both timers count down by the armed delay.
local function grimstoneTick(creature, guid)
    local st = grimstoneState[guid]
    if not st then
        return
    end
    st.pumpEventId = nil
    if creature:IsDead() then
        return
    end
    local diff = st.nextDelay or 0
    local skipEvent = false
    if st.mobDeathTimer > 0 then
        if st.mobDeathTimer <= diff then
            skipEvent = mobDeathPoll(creature, st)
        else
            st.mobDeathTimer = st.mobDeathTimer - diff
        end
    end
    if not skipEvent and st.eventTimer > 0 then
        if st.eventTimer <= diff then
            runPhase(creature, st)
        else
            st.eventTimer = st.eventTimer - diff
        end
    end
    armPump(creature, guid)
end

-- Arm the pump for the smallest live timer; stop when both timers
-- are inactive (Event_Timer 0 stalls the machine in C++ too — the
-- waypoint arrivals re-arm it).
armPump = function(creature, guid)
    local st = grimstoneState[guid]
    if not st then
        return
    end
    cancelPump(guid)
    local delay = nil
    if st.eventTimer > 0 then
        delay = st.eventTimer
    end
    if st.mobDeathTimer > 0 and (delay == nil or st.mobDeathTimer < delay) then
        delay = st.mobDeathTimer
    end
    if delay == nil then
        return
    end
    st.nextDelay = delay
    st.pumpEventId = CreateLuaEvent(function() grimstoneTick(creature, guid) end, delay)
end

-- C++ npc_grimstoneAI::Reset (:177-182) + Initialize (:144-166).
local function grimstoneReset(_, creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    local guid = creature:GetGUIDLow()
    local st = grimstoneState[guid]
    local spawnId = (st and st.mobSpawnId) or math.random(0, 5)
    cancelPump(guid)
    grimstoneState[guid] = {
        phase = 0,
        eventTimer = 1000,
        mobCount = 0,
        mobDeathTimer = 0,
        mobSpawnId = spawnId,
        ringMobGUIDs = {},
        ringBossGUID = nil,
        canWalk = false,
        pumpEventId = nil,
        nextDelay = 0,
    }
    armPump(creature, guid)
end

local function grimstoneDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelPump(guid)
    grimstoneState[guid] = nil
end

-- C++ WaypointReached (blackrock_depths.cpp:205-236).
local function grimstoneReachWP(event, creature)
    local guid = creature:GetGUIDLow()
    local st = grimstoneState[guid]
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:Talk(SAY_TEXT1)
        if st then
            setCanWalk(creature, st, false)
            st.eventTimer = 5000
            armPump(creature, guid)
        end
    elseif wp == 1 then
        creature:Talk(SAY_TEXT2)
        if st then
            setCanWalk(creature, st, false)
            st.eventTimer = 5000
            armPump(creature, guid)
        end
    elseif wp == 2 then
        if st then
            setCanWalk(creature, st, false)
        end
    elseif wp == 3 then
        creature:Talk(SAY_TEXT3)
    elseif wp == 4 then
        creature:Talk(SAY_TEXT4)
        if st then
            setCanWalk(creature, st, false)
            st.eventTimer = 5000
            armPump(creature, guid)
        end
    elseif wp == 5 then
        local data = instanceDataOf(creature)
        if data then
            data:SetData(TYPE_RING_OF_LAW, DONE)
        end
    end
end

RegisterCreatureEvent(ENTRY_GRIMSTONE, 6, grimstoneReachWP)
RegisterCreatureEvent(ENTRY_GRIMSTONE, 23, grimstoneReset)
RegisterCreatureEvent(ENTRY_GRIMSTONE, 4, grimstoneDied)
