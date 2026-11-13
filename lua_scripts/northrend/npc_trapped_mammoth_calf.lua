-- Northrend: Trapped Mammoth Calf — Lua port of
-- src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (class npc_trapped_mammoth_calf : CreatureScript("npc_trapped_mammoth_calf");
-- struct npc_trapped_mammoth_calfAI : public ScriptedAI).
-- Entry: 25850 (NPC_TRAPPED_MAMMOTH_CALF, World/item_scripts.cpp:167,
-- quest-11876 item script — the GO_MAMMOTH_TRAP_1..22 lists there match
-- this file's enum entry-for-entry, cross-confirming the binding).
-- Eluna creature events: 23 OnReset, 38 OnDoAction, 2 OnLeaveCombat,
-- 4 OnDied, 28 OnMovementInform.
-- Ported arms (C++ order, zone_borean_tundra.cpp:1330-1343):
-- - Reset: scan the 22 mammoth-trap GOs in enum order; the first found
--   within 11.0 yards gets SetGoState(0) (GO_STATE_ACTIVE), then return.
--   The IsDead guard is the marli ev23 idiom: Go fires ev23 on spawn,
--   evade and death, while C++ Reset runs on spawn + evade only.
-- - DoAction(1) + UpdateAI MovePoint leg (zone_borean_tundra.cpp:1345-1359):
--   DoAction sets the latch; the 1500ms pump MovePoints to a random point
--   within 10yd (area-uniform, current Z — no terrain-height bridge),
--   then clears the latch. The trigger is the now-ported
--   item_dehta_trap_smasher OnUse (lua_scripts/world/item_scripts.lua,
--   entry 35228).
-- - MovementInform (zone_borean_tundra.cpp:1367-1382): on
--   POINT_MOTION_TYPE arrival, DisappearAndDie (Despawn twin, halazzi
--   precedent) + the trap scan in enum order, first hit within 11yd
--   gets SetLootState(GO_JUST_DEACTIVATED). Event 28 is the
--   MovementInform bridge (kalecgos precedent); the scan runs before
--   the despawn because Despawn drops the motion record the scan
--   reads — C++ order is unobservable here (purely positional).

local ENTRY_CALF = 25850

local POINT_MOTION_TYPE = 8 -- MovementDefines.h:36
local GO_JUST_DEACTIVATED = 3 -- GameObject.h LootState

local MAMMOTH_TRAPS = {
    188022, 188024, 188025, 188026, 188027, 188028, 188029, 188030,
    188031, 188032, 188033, 188034, 188035, 188036, 188037, 188038,
    188039, 188040, 188041, 188042, 188043, 188044,
}

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

-- C++ npc_trapped_mammoth_calfAI::Reset (zone_borean_tundra.cpp:1330-1343):
-- Initialize() (timer/latch state) -> trap scan in enum order, first hit
-- activated, then return.
local function calfReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsDead() then
        return
    end
    for _, trapEntry in ipairs(MAMMOTH_TRAPS) do
        local trap = creature:FindNearestGameObject(trapEntry, 11.0)
        if trap then
            trap:SetGoState(0) -- GO_STATE_ACTIVE
            return
        end
    end
end

-- C++ npc_trapped_mammoth_calfAI::UpdateAI (zone_borean_tundra.cpp:1345-1359):
-- once DoAction(1) sets bStarted, the 1500ms timer fires MovePoint(0) to
-- a random point within 10yd, then bStarted clears.
local function calfRun(creature, guid)
    timers[guid] = nil
    if creature:IsDead() then
        return
    end
    local angle = math.random() * 2 * math.pi
    local dist = math.sqrt(math.random()) * 10.0
    local x = creature:GetX() + dist * math.cos(angle)
    local y = creature:GetY() + dist * math.sin(angle)
    creature:MoveTo(0, x, y, creature:GetZ())
end

-- C++ npc_trapped_mammoth_calfAI::DoAction (zone_borean_tundra.cpp:1361-1365):
-- param 1 sets bStarted.
local function onCalfDoAction(event, creature, action)
    if action ~= 1 then
        return
    end
    local guid = creature:GetGUIDLow()
    schedule(guid, "calf_run", 1500, function() calfRun(creature, guid) end)
end

-- C++ npc_trapped_mammoth_calfAI::MovementInform (zone_borean_tundra.cpp:1367-1382):
-- on POINT_MOTION_TYPE arrival, DisappearAndDie + the trap scan in
-- enum order, first hit within 11yd gets SetLootState(GO_JUST_DEACTIVATED).
-- The scan runs before the despawn (see header): Despawn drops the motion
-- record FindNearestGameObject reads.
local function calfMovementInform(event, creature, type, id)
    if type ~= POINT_MOTION_TYPE then
        return
    end
    for _, trapEntry in ipairs(MAMMOTH_TRAPS) do
        local trap = creature:FindNearestGameObject(trapEntry, 11.0)
        if trap then
            trap:SetLootState(GO_JUST_DEACTIVATED)
            break
        end
    end
    creature:Despawn()
end

local function onCalfCancel(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_CALF, 23, calfReset)
RegisterCreatureEvent(ENTRY_CALF, 38, onCalfDoAction)
RegisterCreatureEvent(ENTRY_CALF, 2, onCalfCancel)
RegisterCreatureEvent(ENTRY_CALF, 4, onCalfCancel)
RegisterCreatureEvent(ENTRY_CALF, 28, calfMovementInform)
