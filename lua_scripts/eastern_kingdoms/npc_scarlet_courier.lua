-- npc_scarlet_courier (Scarlet Enclave, chapter2.cpp) --
-- Lua port of the npc_scarlet_courier struct in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter2.cpp
-- (lines 340-428).
-- Entry (verifiable from the C++ sources): chapter2.cpp's own
-- ScarletCourierEnum (line 336) names NPC_SCARLET_COURIER = 29076.
-- Whole-server-tree grep confirms chapter2.cpp as the only source
-- of "npc_scarlet_courier".
-- Verifiable numbers (file's own enum): SAY_TREE1 = 0 /
-- SAY_TREE2 = 1; SPELL_SHOOT = 52818 (never referenced by the AI
-- body); GO_INCONSPICUOUS_TREE = 191144; the stage machine ticks on
-- a 3000ms timer (Initialize/UpdateAI); MovementInform id == 1
-- advances stage 1 -> 2.
-- Ported arms:
-- - Reset -> Mount(14338) + stage machine init (stage 1, 3000ms
--   one-shot) on OnReset(23).
-- - JustEngagedWith -> Talk(SAY_TREE2 = 1) + Dismount() + stage = 0
--   on OnEnterCombat(1) (C++ Talk then Dismount order); the stage
--   timer is cancelled with it.
-- - Stage 1 (3s after Reset): SetWalk(true); FindNearestGameObject(
--   GO_INCONSPICUOUS_TREE 191144, 40.0f) -> Talk(SAY_TREE1 = 0) +
--   tree:GetContactPoint(courier) + MoveTo(1, x, y, z). No tree
--   found ends the machine (stage = 0), exactly like C++.
-- - MovementInform(POINT_MOTION_TYPE, 1) -> stage 2 on ev28 (the
--   MoveTo arrival gate), then stage 2 fires 3000ms after arrival.
-- - Stage 2: FindNearestGameObject(191144, 40.0f) -> tree:GetOwner()
--   -> EngageWithTarget(owner) (C++ AttackStart). No tree or no
--   owner ends the machine.
-- Deltas: the C++ UpdateAI 3s poll rides a CreateLuaEvent one-shot
-- per stage (the timer only runs while stage != 0 and out of combat;
-- engage zeroes the stage and cancels the timer, so the C++
-- !IsInCombat gate is subsumed); GetContactPoint's LoS sweep and
-- UpdateAllowedPositionZ legs are out (the Go bridge returns the
-- first point with z = tree Z); SPELL_SHOOT (52818) has no
-- C++-verifiable use in the AI body (nothing schedules a shoot arm),
-- so it stays an enum literal.
local SAY_TREE1 = 0
local SAY_TREE2 = 1
local GO_INCONSPICUOUS_TREE = 191144 -- chapter2.cpp:335
local ENTRY_COURIER = 29076
local POINT_MOTION_TYPE = 8 -- runtime.go:393 (POINT MotionType id)
local POINT_ID = 1 -- chapter2.cpp:405-411 (MovePoint id / inform id)

local timers = {}
local stage = {}

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

-- C++ UpdateAI stage machine (chapter2.cpp:390-419): while stage != 0
-- and out of combat, every 3000ms stage 1 walks to the tree's contact
-- point and stage 2 attacks the tree's owner; either stage ends the
-- machine (stage = 0) once it fires.
local function stageTick(creature, guid)
    local s = stage[guid]
    if not s or s == 0 then
        return
    end
    if s == 1 then
        creature:SetWalk(true)
        local tree = creature:FindNearestGameObject(GO_INCONSPICUOUS_TREE, 40)
        if tree then
            creature:Talk(SAY_TREE1)
            local x, y, z = tree:GetContactPoint(creature)
            if x ~= nil then
                creature:MoveTo(POINT_ID, x, y, z)
            end
        end
        stage[guid] = 0
    elseif s == 2 then
        local tree = creature:FindNearestGameObject(GO_INCONSPICUOUS_TREE, 40)
        if tree then
            local owner = tree:GetOwner()
            if owner then
                creature:EngageWithTarget(owner)
            end
        end
        stage[guid] = 0
    end
end

-- C++ Reset: me->Mount(14338) ("not sure about this id" per the
-- C++ comment) + Initialize (stage 1, 3000ms timer).
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Mount(14338)
    stage[guid] = 1
    schedule(guid, "stage", 3000, function()
        stageTick(creature, guid)
    end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    stage[guid] = 0
    creature:Talk(SAY_TREE2)
    creature:Dismount() -- C++ JustEngagedWith: Talk then Dismount.
end

local function onLeaveCombat(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    stage[guid] = 0
end

local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    stage[guid] = 0
end

-- C++ MovementInform (chapter2.cpp:381-387): POINT_MOTION_TYPE +
-- id 1 -> stage 2; the stage-2 leg fires 3000ms after arrival (the
-- C++ timer is frozen while stage == 0, so it resumes from 3000).
local function onMovementInform(_, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE or id ~= POINT_ID then
        return
    end
    local guid = creature:GetGUIDLow()
    stage[guid] = 2
    schedule(guid, "stage", 3000, function()
        stageTick(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_COURIER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_COURIER, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_COURIER, 4, onDied)
RegisterCreatureEvent(ENTRY_COURIER, 23, onReset)
RegisterCreatureEvent(ENTRY_COURIER, 28, onMovementInform)
