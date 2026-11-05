-- Kalimdor: Thrall, Herald of War (Wrathgate cinematic) — Lua port of
-- src/server/scripts/Kalimdor/zone_orgrimmar.cpp
-- (class npc_thrall_herald_of_war : CreatureScript("npc_thrall_herald_of_war");
-- struct npc_thrall_herald_of_warAI : public ScriptedAI).
-- Entry: 31412 (NPC_THRALL_HERALD, zone_orgrimmar.cpp:506, C++-verified).
-- Eluna creature events: 23 OnReset, 4 OnDied.
-- Ported arms (C++ order, zone_orgrimmar.cpp:606-621):
-- - Reset: MovePath(me->GetSpawnId() * 10, true) via creature:MoveWaypoint()
--   (the assigned path — creature_addon.path_id, which the TC data model
--   stores as spawnId * 10; re-arms from node 1 after the 1s DoInitialize
--   hold, vs C++ immediate start), then the once-per-lifetime guard summons
--   below. The MoveWaypoint() call runs on every Reset (spawn + evade),
--   matching C++; only the summons are latched.
-- - Reset: once per creature lifetime, summon the 4 Kor'kron guards
--   (NPC_KORKRON_GUARD 31417) at GuardsSpawnPosition[0..3]; the
--   SummonCreature(entry, pos) overload defaults to
--   TEMPSUMMON_MANUAL_DESPAWN with no despawn timer, matching the Go
--   SummonCreature default (summonType 7). The spawnedGuards latch
--   is a per-GUID Lua table: Go fires ev23 on spawn, on evade and
--   on death, while C++ spawnedGuards is a ctor-only member that
--   Reset never clears — the latch blocks the evade/death re-fires
--   and ev4 clears it, so a respawn (new lifetime) summons again.
-- Unmodeled (documented-only, no bridges):
-- - The MovementInform point-2 re-path (zone_orgrimmar.cpp:622-629) —
--   POINT_MOTION_TYPE arrival has no bridge (event 28 never registered
--   in Go; ev6 is waypoint-only). The leg itself is MoveWaypoint-ready
--   once a point-motion arrival bridge exists.
-- - The DoAction scene machine (ACTION_START_SCENE: MoveIdle,
--   Jaina/Stormwind-portal summons, SetFacingToObject, NON_ATTACKABLE
--   flag, spawn-in cast, guard MovePoint legs, timed EVENT_HERALD_SCENE1+
--   chain with cross-AI Talk) — DoAction IS bridged (creature:DoAction
--   fires ev38) but the machine stays out pending its own triage; the
--   guards' GUID store serves that machine only.

local ENTRY_HERALD = 31412
local NPC_KORKRON_GUARD = 31417

local GUARD_POSITIONS = {
    { 1909.39, -4144.21, 40.6368, 0.042239 },
    { 1910.73, -4155.26, 40.6316, 0.615577 },
    { 1934.01, -4141.40, 40.6375, 3.61109 },
    { 1931.11, -4156.38, 40.6130, 2.19737 },
}

local guardsSpawned = {}

-- C++ npc_thrall_herald_of_warAI::Reset (zone_orgrimmar.cpp:606-621):
-- events.Reset (internal) -> MovePath(spawnId * 10, true), ported below
-- via MoveWaypoint() (assigned path) -> sceneInProgress = false
-- (scene-machine state, stays out) -> the once-per-lifetime guard
-- summons below, C++ order.
local function heraldReset(event, creature)
    creature:MoveWaypoint()
    local guid = creature:GetGUID()
    if guardsSpawned[guid] then
        return
    end
    guardsSpawned[guid] = true
    for _, pos in ipairs(GUARD_POSITIONS) do
        creature:SummonCreature(NPC_KORKRON_GUARD, pos[1], pos[2], pos[3], pos[4])
    end
end

local function heraldDied(event, creature, killer)
    guardsSpawned[creature:GetGUID()] = nil
end

RegisterCreatureEvent(ENTRY_HERALD, 23, heraldReset)
RegisterCreatureEvent(ENTRY_HERALD, 4, heraldDied)
