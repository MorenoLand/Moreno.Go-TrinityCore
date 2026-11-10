-- Hellfire Peninsula: Colonel Jules — Lua port of
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp
-- (class npc_colonel_jules : CreatureScript("npc_colonel_jules")).
-- Entry: 22432 (C++ ExorcismMisc enum NPC_COLONEL_JULES;
-- wowhead npc=22432/colonel-jules confirms).
-- Eluna creature events: 23 OnReset, 28 OnMovementInform,
-- 38 DoAction; gossip event 1 OnGossipHello.
-- Ported arms:
-- - DoAction ACTION_JULES_HOVER (2) (:438-451, event 38): AddAura
--   prone 39283 + threatens 39284, SetCanFly(true), run speed 0.2,
--   SetFacingTo(3.207566 = exorcismPos[2] orientation), MoveJump to
--   exorcismPos[2], success latch cleared, EVENT_SUMMON_SKULL in 10s.
--   Bridges: AddAura (lua_creature_events.go:2643), SetCanFly (:3646),
--   SetSpeedRate (:5629, MOVE_RUN=1 only), SetFacingTo (:4834),
--   MoveJump (:5317, x/y/z + zSpeed + maxHeight).
-- - DoAction ACTION_JULES_FLIGHT (3) (:452-460): RemoveAura prone
--   39283, AddAura upright 39294 + vomits 39295, waypoint chain from
--   the current point (Initialize point 3) via MoveTo(point).
-- - DoAction ACTION_JULES_MOVE_HOME (4) (:461-467): skull timer
--   cancelled, run speed 1.0, MoveTo(11, exorcismPos[2]).
-- - MovementInform (:483-501, event 28): POINT_MOTION_TYPE 8 only;
--   id 3-10 advances the waypoint chain, id 8 summons 2x foul purge
--   22506 at exorcismPos[8], id 10 wraps the chain back to point 3.
--   Chain note: the C++ UpdateAI wpreached leg re-issues the current
--   point, but that re-issue lands before the arrival (it cancels the
--   in-flight spline), so each waypoint arrives exactly once — the Lua
--   leg issues id+1 directly on arrival, the same observable sequence
--   (2 purges per loop). A Go zero-length spline still informs
--   (creaturemotion.go:696), so re-issuing the arrived point here
--   would double-fire. Bridges: event 28 (runtime.go:397),
--   MoveTo (:4974), SummonCreature (:3819).
-- - EVENT_SUMMON_SKULL (:512-520): urand(1,3) darkness-released 22507
--   at Jules +1.5z, TEMPSUMMON_MANUAL_DESPAWN, re-armed 10-15s.
--   JustSummoned (:475-479): each summon gets MoveRandom(10.0),
--   applied to the SummonCreature return value (netherspite /
--   malchezaar precedent). Bridges: GetX/GetY/GetZ (object.go:185),
--   MoveRandom (:5136).
-- - Reset (:417-429, event 23): skull timer cancelled, summons
--   despawned by entry grid sweep (moroes precedent — no
--   GetCreatureByGUID bridge), point re-init 3, success latch cleared.
-- - OnGossipHello (:525-532, gossip event 1): when the exorcism
--   `success` latch is set (Barada step 21),
--   KilledMonsterCredit(NPC_COLONEL_JULES 22432) — the quest-10935
--   completion credit — then the gossip menu. Bridges:
--   KilledMonsterCredit (lua_player.go:4541), GossipSendMenu +
--   GetGossipTextId (kodo precedent). The latch lives in the shared
--   table below: all lua_scripts files load into ONE Lua state
--   (engine/scripting/runtime.go LoadScripts), so Barada's step-21 /
--   EVENT_RESET writes land here; Reset (event 23) clears it per
--   C++ Reset -> Initialize (success = false).
-- - HOVER also clears the C++ member bool wpreached only via the
--   chain itself: MOVE_HOME sets it false and the chain stops because
--   no further id 3-10 arrival fires; the MoveJump inform is
--   EFFECT_MOTION_TYPE, filtered by the POINT check above.

-- Shared with npc_barada.lua: per-Jules-GUID exorcism success latch.
ColonelJulesSuccess = ColonelJulesSuccess or {}

local ENTRY_COLONEL_JULES = 22432
local NPC_DARKNESS_RELEASED = 22507
local NPC_FOUL_PURGE = 22506

local ACTION_JULES_HOVER = 2
local ACTION_JULES_FLIGHT = 3
local ACTION_JULES_MOVE_HOME = 4

local SPELL_JULES_GOES_PRONE = 39283
local SPELL_JULES_THREATENS_AURA = 39284
local SPELL_JULES_GOES_UPRIGHT = 39294
local SPELL_JULES_VOMITS_AURA = 39295

local MOVE_RUN = 1
local POINT_MOTION_TYPE = 8

local TEMPSUMMON_CORPSE_TIMED_DESPAWN = 5
local TEMPSUMMON_MANUAL_DESPAWN = 7

-- exorcismPos[2..10] (zone_hellfire_peninsula.cpp:362-377):
-- [2] Jules bed, [3..10] the flight circle.
local EXORCISM_POS = {
    [2] = { -710.332, 2754.394, 102.948 },
    [3] = { -714.261, 2747.754, 103.391 },
    [4] = { -713.113, 2750.194, 103.391 },
    [5] = { -710.385, 2750.896, 103.391 },
    [6] = { -708.309, 2750.062, 103.391 },
    [7] = { -707.401, 2747.696, 103.391 },
    [8] = { -708.591, 2745.266, 103.391 },
    [9] = { -710.597, 2744.035, 103.391 },
    [10] = { -713.089, 2745.302, 103.391 },
}

local timers = {}
local state = {} -- per-GUID: { point = n } (C++ Initialize point 3)

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
    per[key] = CreateLuaEvent(fn, delay, 0)
end

-- C++ EVENT_SUMMON_SKULL (zone_hellfire_peninsula.cpp:512-520).
local function skullTick(creature, guid)
    if state[guid] == nil then
        return
    end
    local count = math.random(1, 3)
    for _ = 1, count do
        local skull = creature:SummonCreature(NPC_DARKNESS_RELEASED,
            creature:GetX(), creature:GetY(), creature:GetZ() + 1.5,
            0, TEMPSUMMON_MANUAL_DESPAWN)
        if skull then
            -- C++ JustSummoned (:475-479): MoveRandom(10.0).
            skull:MoveRandom(10.0)
        end
    end
    schedule(guid, "skull", math.random(10000, 15000),
        function() skullTick(creature, guid) end)
end

-- C++ Reset summons.DespawnAll() (:425): entry grid sweep, moroes
-- precedent — Lua has no GUID->creature lookup.
local function despawnSummons(creature)
    for _, entry in ipairs({ NPC_DARKNESS_RELEASED, NPC_FOUL_PURGE }) do
        for _, summon in ipairs(creature:GetCreaturesInGrid(entry, 60)) do
            summon:Despawn()
        end
    end
end

-- C++ MovementInform + the UpdateAI wpreached leg
-- (zone_hellfire_peninsula.cpp:483-501, :505-510, event 28).
local function onMovementInform(event, creature, type, id)
    if type ~= POINT_MOTION_TYPE then
        return
    end
    if id < 3 or id > 10 then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = state[guid]
    if st == nil then
        return
    end
    if id == 8 then
        -- C++ MovementInform id 8: 2x DoSummon foul purge at
        -- exorcismPos[8] (CreatureAI::DoSummon defaults: 30s,
        -- TEMPSUMMON_CORPSE_TIMED_DESPAWN).
        local p = EXORCISM_POS[8]
        for _ = 1, 2 do
            local purge = creature:SummonCreature(NPC_FOUL_PURGE,
                p[1], p[2], p[3], 0,
                TEMPSUMMON_CORPSE_TIMED_DESPAWN, 30000)
            if purge then
                -- C++ JustSummoned (:475-479): MoveRandom(10.0).
                purge:MoveRandom(10.0)
            end
        end
    end
    -- At arrival the C++ point already holds id+1 (incremented when
    -- id was issued); id 10 wraps it to 3. Issue the next waypoint.
    local nextPoint = id == 10 and 3 or id + 1
    st.point = nextPoint
    local p = EXORCISM_POS[nextPoint]
    creature:MoveTo(nextPoint, p[1], p[2], p[3])
end

-- C++ DoAction (zone_hellfire_peninsula.cpp:434-469, event 38).
local function onDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_JULES_HOVER then
        creature:AddAura(SPELL_JULES_GOES_PRONE)
        creature:AddAura(SPELL_JULES_THREATENS_AURA)
        creature:SetCanFly(true)
        creature:SetSpeedRate(MOVE_RUN, 0.2)
        creature:SetFacingTo(3.207566)
        local bed = EXORCISM_POS[2]
        creature:MoveJump(bed[1], bed[2], bed[3], 2.0, 2.0)
        ColonelJulesSuccess[guid] = nil
        schedule(guid, "skull", 10000,
            function() skullTick(creature, guid) end)
    elseif action == ACTION_JULES_FLIGHT then
        creature:RemoveAura(SPELL_JULES_GOES_PRONE)
        creature:AddAura(SPELL_JULES_GOES_UPRIGHT)
        creature:AddAura(SPELL_JULES_VOMITS_AURA)
        local st = state[guid]
        if st == nil then
            st = { point = 3 }
            state[guid] = st
        end
        local p = EXORCISM_POS[st.point]
        creature:MoveTo(st.point, p[1], p[2], p[3])
    elseif action == ACTION_JULES_MOVE_HOME then
        cancelTimers(guid)
        creature:SetSpeedRate(MOVE_RUN, 1.0)
        local bed = EXORCISM_POS[2]
        creature:MoveTo(11, bed[1], bed[2], bed[3])
    end
end

-- C++ Reset -> Initialize (zone_hellfire_peninsula.cpp:417-429):
-- timers + summons cleared, point re-init 3, success latch cleared.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    despawnSummons(creature)
    state[guid] = { point = 3 }
    ColonelJulesSuccess[guid] = nil
end

-- C++ OnGossipHello (zone_hellfire_peninsula.cpp:525-532).
local function julesGossipHello(event, player, creature)
    if ColonelJulesSuccess[creature:GetGUIDLow()] then
        player:KilledMonsterCredit(ENTRY_COLONEL_JULES)
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

RegisterCreatureEvent(ENTRY_COLONEL_JULES, 23, onReset)
RegisterCreatureEvent(ENTRY_COLONEL_JULES, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_COLONEL_JULES, 38, onDoAction)
RegisterCreatureGossipEvent(ENTRY_COLONEL_JULES, 1, julesGossipHello)
