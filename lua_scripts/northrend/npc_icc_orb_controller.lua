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
-- STAYS OUT: SetGUID ACTION_COMBAT (:729-748, no cross-creature AI
-- SetGUID bridge — CancelAll, DoZoneInCombat, SIPHON_ESSENCE 70299 on a
-- random minion); DoAction ACTION_EVADE (:750-763, no DoAction bridge —
-- UpdateValidGuids prune + 5s visual re-arm + orb NOT_SELECTABLE flag);
-- SpellHit ORB_CONTROLLER_ACTIVE 70293 (:720-725, ev14 hook exists but
-- the leg's only effect is a SetGUID into the unported
-- go_empowering_blood_orb GO script); the init-time SetGUID(DATA_GUID)
-- to each minion (:680) has no Lua receiver on the darkfallen side
-- (darkfallen header documents the TriggerGuid gap).

local NPC_BLOOD_ORB_CONTROLLER = 38008 -- icecrown_citadel.h:248

local NPC_DARKFALLEN_BLOOD_KNIGHT = 37595
local NPC_DARKFALLEN_NOBLE = 37663
local NPC_DARKFALLEN_ARCHMAGE = 37664
local NPC_DARKFALLEN_ADVISOR = 37571
local NPC_DARKFALLEN_TACTICIAN = 37666

local SPELL_BLOOD_ORB_VISUAL = 72099

local DARKFALLEN_ENTRIES = {
    NPC_DARKFALLEN_BLOOD_KNIGHT,
    NPC_DARKFALLEN_NOBLE,
    NPC_DARKFALLEN_ARCHMAGE,
    NPC_DARKFALLEN_ADVISOR,
    NPC_DARKFALLEN_TACTICIAN,
}

local timers = {}
local state = {} -- per-GUID: { minions = {guid,...}, isLongRepeat = bool }

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
            if wanted[c:GetGUID()] then
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
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = nil
    schedule(guid, "init", 1000, function()
        local minions = {}
        for _, entry in ipairs(DARKFALLEN_ENTRIES) do
            for _, c in ipairs(creature:GetCreaturesInGrid(entry, 10.0)) do
                if c:IsAlive() then
                    minions[#minions + 1] = c:GetGUID()
                end
            end
        end
        if #minions == 0 then
            return
        end
        state[guid] = { minions = minions, isLongRepeat = false }
        schedule(guid, "visual", 1000, function()
            visualTick(creature, guid)
        end)
    end)
end

local function orbDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    state[guid] = nil
end

RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 5, orbReset)
RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 23, orbReset)
RegisterCreatureEvent(NPC_BLOOD_ORB_CONTROLLER, 4, orbDied)
