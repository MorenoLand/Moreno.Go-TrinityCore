-- npc_icc_orb_controller (Icecrown Citadel) — Lua port of the visual
-- channel of npc_icc_orb_controller
-- (src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp:662-774).
-- Reset 1s init grid sweep (10.0f, the 5 darkfallen entries, alive only)
-- then ScheduleVisualChannel: first visual 1s later, repeating on the
-- alternating 3s/21s cadence (C++ Repeat(isLongRepeat ? 21s : 3s) then
-- toggle). Each fire picks a random init-time minion still in the world
-- (C++ ObjectAccessor::GetUnit nil-guard) and casts BLOOD_ORB_VISUAL.
-- C++ minion->CastSpell(nullptr, 72099) is a self visual; Lua nil is a
-- victim-fallback no-op for a victimless minion (lua_creature_events.go
-- CastSpell), so the twin is self-cast (anubrekhan corpse precedent).
-- The grid sweep re-derives live objects each fire but membership stays
-- fixed to the init-time GUID set, so a despawned minion drops out
-- (GetUnit-nil) while a dead-but-present one still casts, as in C++.
-- Distance leg: C++ GetDistance(target) > 10.0f excludes (3D center
-- minus both reaches); Lua GetCreaturesInGrid is 2D center <= 10 +
-- both reaches — the same approximation the darkfallen emoteTick uses
-- for this identical search class. The GetWaypointPath leg has no
-- handler-surface bridge (darkfallen precedent: DB trash never paths).
-- PORTED: SetGUID ACTION_COMBAT (:729-748) + DoAction ACTION_EVADE
-- (:750-763) on the event-40 SetGUID bridge (creature:SetGUID /
-- GetCreatureByGUID, lua_creature_events.go) — the darkfallen side
-- stores the controller GUID (DATA_GUID :680, sent at init below)
-- and signals back on engage/evade (npc_darkfallen_icc.lua).
-- STAYS OUT: SpellHit ORB_CONTROLLER_ACTIVE 70293 (:720-725): ev14
-- hook exists but 70293 has no C++ caster anywhere in-tree (const :97
-- + check :722 only), so the hit can never fire in Go; the SetGUID leg
-- it would drive (go_empowering_blood_orb.lua, ported tick 504) stays
-- out with it — the GO object has no CastSpell bridge for the leg's
-- 70304 cast either.

local NPC_BLOOD_ORB_CONTROLLER = 38008 -- icecrown_citadel.h:248

local NPC_DARKFALLEN_BLOOD_KNIGHT = 37595
local NPC_DARKFALLEN_NOBLE = 37663
local NPC_DARKFALLEN_ARCHMAGE = 37664
local NPC_DARKFALLEN_ADVISOR = 37571
local NPC_DARKFALLEN_TACTICIAN = 37666

local SPELL_BLOOD_ORB_VISUAL = 72099
local SPELL_SIPHON_ESSENCE = 70299

local GO_EMPOWERING_BLOOD_ORB = 201741 -- icecrown_citadel.h:403
local GAMEOBJECT_FLAGS = 9 -- UpdateFields.h: OBJECT_END 6 + 3 (arlokk precedent)
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641

local DATA_GUID = 2 -- icecrown_citadel.cpp:188 (DATA_DAMNED_KILLS = 1, DATA_GUID)
local ACTION_EVADE = 2 -- icecrown_citadel.cpp:193-195
local ACTION_COMBAT = 3

local DARKFALLEN_ENTRIES = {
    NPC_DARKFALLEN_BLOOD_KNIGHT,
    NPC_DARKFALLEN_NOBLE,
    NPC_DARKFALLEN_ARCHMAGE,
    NPC_DARKFALLEN_ADVISOR,
    NPC_DARKFALLEN_TACTICIAN,
}

local timers = {}
local state = {} -- per-GUID: { minions = {guid,...}, isLongRepeat = bool, isInCombat = bool }

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
    if per then
        if per[key] then
            RemoveEventById(per[key])
        end
    else
        per = {}
        timers[guid] = per
    end
    per[key] = CreateLuaEvent(fn, delay, 0)
end

local function visualTick(creature, guid)
    local st = state[guid]
    if not st then
        return
    end
    local wanted = {}
    for _, g in ipairs(st.minions) do
        wanted[g] = true
    end
    local live = {}
    for _, entry in ipairs(DARKFALLEN_ENTRIES) do
        for _, c in ipairs(creature:GetCreaturesInGrid(entry, 10.0)) do
            if wanted[c:GetGUIDLow()] then
                live[#live + 1] = c
            end
        end
    end
    if #live > 0 then
        local minion = live[math.random(#live)]
        minion:CastSpell(minion, SPELL_BLOOD_ORB_VISUAL)
    end
    local delay = st.isLongRepeat and 21000 or 3000
    st.isLongRepeat = not st.isLongRepeat
    schedule(guid, "visual", delay, function()
        visualTick(creature, guid)
    end)
end

local function orbReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- State is created at Reset (not in the init task) so the SetGUID
    -- combat arm can latch isInCombat even if combat lands inside the
    -- 1s init window — C++ CancelAll then also kills the pending init
    -- and _minionGuids stays empty (:729-735).
    state[guid] = { minions = {}, isLongRepeat = false, isInCombat = false }
    schedule(guid, "init", 1000, function()
        local st = state[guid]
        if st == nil then
            return
        end
        local minions = {}
        for _, entry in ipairs(DARKFALLEN_ENTRIES) do
            for _, c in ipairs(creature:GetCreaturesInGrid(entry, 10.0)) do
                if c:IsAlive() then
                    minions[#minions + 1] = c:GetGUIDLow()
                    -- C++ Reset init (:680): hand the controller GUID to
                    -- each minion (DATA_GUID) so it can signal back.
                    c:SetGUID(guid, DATA_GUID)
                end
            end
        end
        if #minions == 0 then
            return
        end
        st.minions = minions
        schedule(guid, "visual", 1000, function()
            visualTick(creature, guid)
        end)
    end)
end

local function orbDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

-- npc_icc_orb_controller::SetGUID (icecrown_citadel.cpp:727-749):
-- ACTION_COMBAT from an engaging darkfallen (its own GUID): latch,
-- cancel the visual machine, zone-engage the darkfallen (one call
-- per out-of-combat minion in C++ — the Go DoZoneInCombat has no
-- target arg, it engages the caller, CreatureAI.cpp:73-104, so one
-- call is the same net effect), then a random minion self-casts
-- SIPHON_ESSENCE 70299 (C++ nil-victim cast; self-cast twin per the
-- visual-leg note above). A nil darkfallen (stale GUID) returns
-- before the loop, as in C++.
local function orbSetGUID(event, creature, guid, id)
    if id ~= ACTION_COMBAT then
        return
    end
    local cg = creature:GetGUIDLow()
    local st = state[cg]
    if st == nil or st.isInCombat then
        return
    end
    st.isInCombat = true
    cancelTimers(cg)
    if #st.minions == 0 then
        return
    end
    local darkfallen = creature:GetCreatureByGUID(guid)
    if darkfallen == nil then
        return
    end
    local anyOutOfCombat = false
    for _, g in ipairs(st.minions) do
        local minion = creature:GetCreatureByGUID(g)
        if minion ~= nil and not minion:IsInCombat() then
            anyOutOfCombat = true
            break
        end
    end
    if anyOutOfCombat then
        darkfallen:DoZoneInCombat()
    end
    local pick = st.minions[math.random(#st.minions)]
    local minion = creature:GetCreatureByGUID(pick)
    if minion ~= nil then
        minion:CastSpell(minion, SPELL_SIPHON_ESSENCE)
    end
end

-- npc_icc_orb_controller::DoAction ACTION_EVADE
-- (icecrown_citadel.cpp:751-763): the darkfallen evaded — drop the
-- latch, prune dead/despawned minions (UpdateValidGuids :705-718),
-- re-arm the visual channel with the 5s evade first-fire, and flag
-- the empowering blood orb NOT_SELECTABLE.
local function orbDoAction(event, creature, action)
    if action ~= ACTION_EVADE then
        return
    end
    local cg = creature:GetGUIDLow()
    local st = state[cg]
    if st == nil or not st.isInCombat then
        return
    end
    st.isInCombat = false
    local kept = {}
    for _, g in ipairs(st.minions) do
        local minion = creature:GetCreatureByGUID(g)
        if minion ~= nil and minion:IsAlive() then
            kept[#kept + 1] = g
        end
    end
    st.minions = kept
    schedule(cg, "visual", 5000, function()
        visualTick(creature, cg)
    end)
    local orb = creature:FindNearestGameObject(GO_EMPOWERING_BLOOD_ORB, 5.0)
    if orb ~= nil then
        orb:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
end

RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 5, orbReset)
RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 23, orbReset)
RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 4, orbDied)
RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 40, orbSetGUID)
RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 38, orbDoAction)
