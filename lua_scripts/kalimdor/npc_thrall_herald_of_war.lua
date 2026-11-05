-- Kalimdor: Thrall, Herald of War (Wrathgate cinematic) — Lua port of
-- src/server/scripts/Kalimdor/zone_orgrimmar.cpp
-- (class npc_thrall_herald_of_war : CreatureScript("npc_thrall_herald_of_war");
-- struct npc_thrall_herald_of_warAI : public ScriptedAI).
-- Entry: 31412 (NPC_THRALL_HERALD, zone_orgrimmar.cpp:506, C++-verified).
-- Eluna creature events: 23 OnReset, 4 OnDied, 38 OnDoAction.
-- Timers via CreateLuaEvent (aeonus convention: timers table keyed by
-- GUID, cancelled on ev23/ev4); the 15-step chain is one "scene" key
-- (only one step pending at a time, C++ EventMap order).
-- Ported arms (C++ order, zone_orgrimmar.cpp:606-621, 632-665, 677-800):
-- - Reset: MovePath(me->GetSpawnId() * 10, true) via creature:MoveWaypoint()
--   (the assigned path — creature_addon.path_id, which the TC data model
--   stores as spawnId * 10; re-arms from node 1 after the 1s DoInitialize
--   hold, vs C++ immediate start), then the once-per-lifetime guard summons
--   below. The MoveWaypoint() call runs on every Reset (spawn + evade +
--   scene reset), matching C++; only the summons are latched.
-- - Reset: once per creature lifetime, summon the 4 Kor'kron guards
--   (NPC_KORKRON_GUARD 31417) at GuardsSpawnPosition[0..3]; the
--   SummonCreature(entry, pos) overload defaults to
--   TEMPSUMMON_MANUAL_DESPAWN with no despawn timer, matching the Go
--   SummonCreature default (summonType 7). The spawnedGuards latch
--   is a per-GUID Lua table: Go fires ev23 on spawn, on evade and
--   on death, while C++ spawnedGuards is a ctor-only member that
--   Reset never clears — the latch blocks the evade/death re-fires
--   and ev4 clears it, so a respawn (new lifetime) summons again.
--   The summoned guard OBJECTS are stored (heraldGuards, opera
--   raj.julianne precedent — no GetCreatureByGUID bridge exists);
--   they survive Reset like the C++ guardsGUIDs and are re-stored
--   on the next lifetime.
-- - DoAction(ACTION_START_SCENE 0, ev38; terenasDoAction precedent):
--   sceneInProgress latch, MoveIdle (StopMoving twin), summon Jaina
--   (31418) + the Stormwind portal (31640) at PortalSpawnPosition,
--   SetFacingToObject(jaina), jaina self-cast SPELL_JAINA_SPAWNIN
--   55761 (non-triggered, C++ CastSpell(jaina, spell)), portal
--   SetFlag(UNIT_FIELD_FLAGS 59, NON_ATTACKABLE 0x2), guards run
--   (SetWalk(false)) to GuardsMovePosition[0..3] in EMOTE_STATE_READY2H
--   (375, creature:EmoteState, ragnaros precedent), sylvanas
--   (31419) via FindNearestCreature(25.0), then EVENT_HERALD_SCENE1
--   in 4s. All legs C++ order.
-- - Scene chain (EVENT_HERALD_SCENE1..15 + EVENT_HERALD_RESET,
--   delays 4s/3s/3.5s/5.5s/6.5s/10s/20s/4.5s/10s/8s/9s/6s/14s/10s/7s/60s):
--   Talk lines name-by-name (thrall 0-7, jaina 0-4, sylvanas 0-1,
--   zone_orgrimmar.cpp:519-548 enum comments); cross-creature Talk
--   via the stored objects (mandokir jindo:Talk precedent).
--   MovePoint(1/2, pos) via creature:MoveTo(pointId, x, y, z)
--   (shade_of_akama precedent); MoveTargetedHome (guards' return,
--   SCENE1) has NO bridge — twinned exactly with GetHomePosition()
--   + MoveTo(0, hx, hy, hz) (the guards' home IS their spawn
--   position; MoveTargetedHome moves to home). SetStandState(0)
--   (STAND), SetWalk, GetHomePosition, DespawnOrUnsummon(5000)
--   (jaina, C++ 5s) / DespawnOrUnsummon() (portal) all bridged.
--   The reset step calls Reset() (C++ EVENT_HERALD_RESET), which
--   re-arms MoveWaypoint and clears the scene latch + timers.
-- Unmodeled (documented-only, no bridges):
-- - The MovementInform point-2 re-path (zone_orgrimmar.cpp:622-629) —
--   POINT_MOTION_TYPE arrival has no bridge (event 28 never registered
--   in Go; ev6 is waypoint-only). The leg itself is MoveWaypoint-ready
--   once a point-motion arrival bridge exists.
-- - The area-trigger dispatch (areatrigger_orgrimmar_herald_of_war,
--   zone_orgrimmar.cpp:811-831: player alive + quest 13257 COMPLETE
--   + nearest thrall within 50yd -> thrall:DoAction(0)). The fire site
--   EXISTS (RegisterServerEvent(24) from engine/world/areatrigger.go,
--   at_scripts.lua convention; player:IsAlive, player:GetQuestStatus
--   (COMPLETE=1) and player:FindNearestCreature are all bridged) but
--   the trigger ID lives ONLY in the DB areatrigger_scripts row
--   (ObjectMgr.cpp:6653) — not in the C++ file, not in-tree (world.db
--   is a 0-byte placeholder, sql/ is schema-only) — and dispatching on
--   a guessed ID would invent an identifier (gelihast precedent,
--   at_scripts.lua). The scene machine above is complete and fires on
--   any DoAction(0); the trigger dispatch awaits a verified ID.

local ENTRY_HERALD = 31412
local NPC_KORKRON_GUARD = 31417
local NPC_JAINA_PROUDMOORE = 31418
local NPC_BANSHEE_SYLVANAS = 31419
local NPC_PORTAL_STORMWIND = 31640

local QUEST_HERALD_OF_WAR = 13257 -- trigger gate only (see header)
local SPELL_JAINA_SPAWNIN = 55761
local ACTION_START_SCENE = 0

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local EMOTE_STATE_READY2H = 375 -- SharedDefines.h:2013
local EMOTE_STATE_NONE = 0
local UNIT_STAND_STATE_STAND = 0

local GUARD_POSITIONS = {
    { 1909.39, -4144.21, 40.6368, 0.042239 },
    { 1910.73, -4155.26, 40.6316, 0.615577 },
    { 1934.01, -4141.40, 40.6375, 3.61109 },
    { 1931.11, -4156.38, 40.6130, 2.19737 },
}

-- C++ GuardsMovePosition (zone_orgrimmar.cpp:572-579).
local GUARD_MOVE_POSITIONS = {
    { 1917.46167, -4147.51416, 40.636799 },
    { 1916.18127, -4152.29590, 40.629120 },
    { 1926.43543, -4146.39746, 40.618534 },
    { 1926.51917, -4153.21680, 40.614975 },
}

-- C++ MiscMovePositions (zone_orgrimmar.cpp:581-586): [1] jaina,
-- [2] thrall, [3] sylvanas.
local MISC_MOVE_POSITIONS = {
    { 1921.71960, -4143.05127, 40.623356 },
    { 1921.15186, -4139.34375, 40.583084 },
    { 1918.73242, -4139.61963, 40.607685 },
}

-- C++ PortalSpawnPosition (zone_orgrimmar.cpp:588).
local PORTAL_POSITION = { 1921.75244, -4151.14844, 40.623848, 1.714324 }

-- C++ EVENT_HERALD_SCENE1..15 delays (zone_orgrimmar.cpp:665-800):
-- step n runs DELAYS[n] after step n-1 (step 1: 4s after DoAction).
local SCENE_DELAYS = {
    4000, 3000, 3500, 5500, 6500, 10000, 20000, 4500,
    10000, 8000, 9000, 6000, 14000, 10000, 7000,
}
local SCENE_RESET_DELAY = 60000 -- EVENT_HERALD_RESET, 1min

local guardsSpawned = {}
local heraldGuards = {}
local sceneInProgress = {}
local heraldScene = {}
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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ npc_thrall_herald_of_warAI::Reset (zone_orgrimmar.cpp:606-621):
-- events.Reset (internal) -> MovePath(spawnId * 10, true), ported below
-- via MoveWaypoint() (assigned path) -> sceneInProgress = false
-- (scene-machine state, cleared below) -> the once-per-lifetime guard
-- summons below, C++ order.
local function heraldReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sceneInProgress[guid] = nil
    heraldScene[guid] = nil
    creature:MoveWaypoint()
    if guardsSpawned[guid] then
        return
    end
    guardsSpawned[guid] = true
    local guards = {}
    heraldGuards[guid] = guards
    for i, pos in ipairs(GUARD_POSITIONS) do
        guards[i] = creature:SummonCreature(NPC_KORKRON_GUARD, pos[1], pos[2], pos[3], pos[4])
    end
end

local function heraldDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sceneInProgress[guid] = nil
    heraldScene[guid] = nil
    heraldGuards[guid] = nil
    guardsSpawned[guid] = nil
end

-- C++ UpdateAI EVENT_HERALD_SCENE1..15 + EVENT_HERALD_RESET
-- (zone_orgrimmar.cpp:677-800). Forward-declared: the step closures
-- re-schedule the next step through this name (tharon_ja idiom).
local sceneStep

sceneStep = function(creature, guid, step)
    local scene = heraldScene[guid] or {}
    local function next(delay)
        schedule(guid, "scene", delay, function()
            sceneStep(creature, guid, step + 1)
        end)
    end
    if step == 1 then
        -- Talk(SAY_THRALL_0); MovePoint(1, thrall move); jaina walks to
        -- her move point; guards MoveTargetedHome + EMOTESTATE NONE.
        creature:Talk(0)
        local pos = MISC_MOVE_POSITIONS[2]
        creature:MoveTo(1, pos[1], pos[2], pos[3])
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            jaina:SetWalk(true)
            pos = MISC_MOVE_POSITIONS[1]
            jaina:MoveTo(1, pos[1], pos[2], pos[3])
        end
        local guards = heraldGuards[guid]
        if guards then
            for _, guard in ipairs(guards) do
                if guard and not guard:IsDead() then
                    local hx, hy, hz = guard:GetHomePosition()
                    guard:MoveTo(0, hx, hy, hz)
                    guard:EmoteState(EMOTE_STATE_NONE)
                end
            end
        end
        next(SCENE_DELAYS[2])
    elseif step == 2 then
        -- Talk(SAY_THRALL_1); face jaina.
        creature:Talk(1)
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            creature:SetFacingToObject(jaina)
        end
        next(SCENE_DELAYS[3])
    elseif step == 3 then
        -- jaina Talk(SAY_JAINA_0).
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            jaina:Talk(0)
        end
        next(SCENE_DELAYS[4])
    elseif step == 4 then
        -- Talk(SAY_THRALL_2); sylvanas stands, walks to her move point.
        creature:Talk(2)
        local sylvanas = scene.sylvanas
        if sylvanas and not sylvanas:IsDead() then
            sylvanas:SetStandState(UNIT_STAND_STATE_STAND)
            sylvanas:SetWalk(true)
            local pos = MISC_MOVE_POSITIONS[3]
            sylvanas:MoveTo(1, pos[1], pos[2], pos[3])
        end
        next(SCENE_DELAYS[5])
    elseif step == 5 then
        -- sylvanas Talk(SAY_SYLVANAS_0).
        local sylvanas = scene.sylvanas
        if sylvanas and not sylvanas:IsDead() then
            sylvanas:Talk(0)
        end
        next(SCENE_DELAYS[6])
    elseif step == 6 then
        -- sylvanas Talk(SAY_SYLVANAS_1).
        local sylvanas = scene.sylvanas
        if sylvanas and not sylvanas:IsDead() then
            sylvanas:Talk(1)
        end
        next(SCENE_DELAYS[7])
    elseif step == 7 then
        creature:Talk(3)
        next(SCENE_DELAYS[8])
    elseif step == 8 then
        creature:Talk(4)
        next(SCENE_DELAYS[9])
    elseif step == 9 then
        creature:Talk(5)
        next(SCENE_DELAYS[10])
    elseif step == 10 then
        creature:Talk(6)
        next(SCENE_DELAYS[11])
    elseif step == 11 then
        -- jaina Talk(SAY_JAINA_1).
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            jaina:Talk(1)
        end
        next(SCENE_DELAYS[12])
    elseif step == 12 then
        -- jaina Talk(SAY_JAINA_2).
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            jaina:Talk(2)
        end
        next(SCENE_DELAYS[13])
    elseif step == 13 then
        -- jaina Talk(SAY_JAINA_3).
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            jaina:Talk(3)
        end
        next(SCENE_DELAYS[14])
    elseif step == 14 then
        -- jaina Talk(SAY_JAINA_4), walks home, despawns in 5s.
        local jaina = scene.jaina
        if jaina and not jaina:IsDead() then
            jaina:Talk(4)
            jaina:SetWalk(true)
            local hx, hy, hz = jaina:GetHomePosition()
            jaina:MoveTo(2, hx, hy, hz)
            jaina:DespawnOrUnsummon(5000)
        end
        next(SCENE_DELAYS[15])
    elseif step == 15 then
        -- Talk(SAY_THRALL_7); thrall + sylvanas walk home; portal despawns.
        creature:SetWalk(true)
        local hx, hy, hz = creature:GetHomePosition()
        creature:MoveTo(2, hx, hy, hz)
        creature:Talk(7)
        local sylvanas = scene.sylvanas
        if sylvanas and not sylvanas:IsDead() then
            sylvanas:SetWalk(true)
            hx, hy, hz = sylvanas:GetHomePosition()
            sylvanas:MoveTo(2, hx, hy, hz)
        end
        local portal = scene.portal
        if portal and not portal:IsDead() then
            portal:DespawnOrUnsummon()
        end
        schedule(guid, "reset", SCENE_RESET_DELAY, function()
            heraldReset(0, creature)
        end)
    end
end

-- C++ npc_thrall_herald_of_warAI::DoAction (zone_orgrimmar.cpp:632-665).
local function heraldDoAction(event, creature, action)
    local guid = creature:GetGUID()
    if action ~= ACTION_START_SCENE or sceneInProgress[guid] then
        return
    end
    sceneInProgress[guid] = true
    creature:MoveIdle()
    local scene = {}
    heraldScene[guid] = scene
    local jaina = creature:SummonCreature(NPC_JAINA_PROUDMOORE,
        PORTAL_POSITION[1], PORTAL_POSITION[2], PORTAL_POSITION[3], PORTAL_POSITION[4])
    if jaina then
        creature:SetFacingToObject(jaina)
        scene.jaina = jaina
        jaina:CastSpell(jaina, SPELL_JAINA_SPAWNIN)
    end
    local portal = creature:SummonCreature(NPC_PORTAL_STORMWIND,
        PORTAL_POSITION[1], PORTAL_POSITION[2], PORTAL_POSITION[3], PORTAL_POSITION[4])
    if portal then
        portal:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        scene.portal = portal
    end
    local guards = heraldGuards[guid]
    if guards then
        for i, guard in ipairs(guards) do
            if guard and not guard:IsDead() then
                guard:SetWalk(false)
                guard:EmoteState(EMOTE_STATE_READY2H)
                local pos = GUARD_MOVE_POSITIONS[i]
                guard:MoveTo(1, pos[1], pos[2], pos[3])
            end
        end
    end
    local sylvanas = creature:FindNearestCreature(NPC_BANSHEE_SYLVANAS, 25)
    if sylvanas then
        scene.sylvanas = sylvanas
    end
    schedule(guid, "scene", SCENE_DELAYS[1], function()
        sceneStep(creature, guid, 1)
    end)
end

RegisterCreatureEvent(ENTRY_HERALD, 23, heraldReset)
RegisterCreatureEvent(ENTRY_HERALD, 4, heraldDied)
RegisterCreatureEvent(ENTRY_HERALD, 38, heraldDoAction)
