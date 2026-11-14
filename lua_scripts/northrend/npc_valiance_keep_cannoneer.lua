-- Valiance Keep Cannoneer (Borean Tundra) -- Lua port of
-- src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (class npc_valiance_keep_cannoneer : CreatureScript("npc_valiance_keep_cannoneer");
-- struct npc_valiance_keep_cannoneerAI : public ScriptedAI).
-- Entry: 25306 'Valiance Keep Cannoneer' (6 static spawns on map
-- 571, TDB 26091; ScriptName 'npc_valiance_keep_cannoneer' binds).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 4 OnDied (timer
-- hygiene only — C++ has no JustDied override).
-- Ported arms (C++ order):
-- - Initialize / Reset (zone_borean_tundra.cpp:1411-1416, 1425-1428):
--   uiTimer = urand(13000, 18000). Re-rolled on every Reset, so
--   the timer re-arms after an evade exactly as C++ does.
-- - UpdateAI (zone_borean_tundra.cpp:1430-1446): on timer fire,
--   HandleEmoteCommand(EMOTE_ONESHOT_KNEEL) + FindNearestGameObject
--   (GO_VALIANCE_KEEP_CANNON_1 187560, 10y) else
--   (GO_VALIANCE_KEEP_CANNON_2 188692, 10y) + pCannon->Use(me),
--   then re-arm urand(13000, 18000). The timer leg precedes the
--   `if (!UpdateVictim()) return;` guard and nothing follows it,
--   so the whole UpdateAI is the timer — the port runs the pump
--   regardless of combat, as C++ does. Both cannon entries are
--   GAMEOBJECT_TYPE_GOOBER (type 10, TDB 26091; Data3 3000 =
--   autoCloseTime, Data10 42442 = template spell); the Go
--   serverActivateGameObjectUse twin runs the goober arm (state
--   flip, autoClose re-arm, template-spell cast).
-- Unmodeled (documented-only, no bridges):
-- - Template spell 42442 (cannon fire) is unseeded in Spell.dbc,
--   so the Go twin's cast leg stays silent until it is seeded
--   (wago read needed — same source as the Aran 16-spell seed).
--   The emote + state flip + autoClose legs all run regardless.

local ENTRY_VALIANCE_KEEP_CANNONEER = 25306

local GO_VALIANCE_KEEP_CANNON_1 = 187560
local GO_VALIANCE_KEEP_CANNON_2 = 188692

local EMOTE_ONESHOT_KNEEL = 16

local CANNON_RANGE = 10.0
local TIMER_MIN_MS = 13000
local TIMER_MAX_MS = 18000

local timers = {}

-- Forward declaration: armTimer's closure calls cannoneerTick, which
-- is assigned below. A later `local function` here would shadow this
-- slot — plain assignment only (tharon_ja/gal_darah idiom).
local cannoneerTick

local function cancelTimer(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function armTimer(guid, creature)
    cancelTimer(guid)
    timers[guid] = CreateLuaEvent(function()
        timers[guid] = nil
        cannoneerTick(creature, guid)
    end, math.random(TIMER_MIN_MS, TIMER_MAX_MS))
end

-- C++ npc_valiance_keep_cannoneerAI::UpdateAI
-- (zone_borean_tundra.cpp:1430-1446), timer leg only: kneel +
-- nearest cannon Use + re-arm. Runs out of combat and while
-- engaged alike — C++ checks the timer before UpdateVictim().
cannoneerTick = function(creature, guid)
    if creature:IsDead() then
        return
    end
    creature:PerformEmote(EMOTE_ONESHOT_KNEEL)
    local cannon = creature:FindNearestGameObject(
        GO_VALIANCE_KEEP_CANNON_1, CANNON_RANGE)
    if cannon == nil then
        cannon = creature:FindNearestGameObject(
            GO_VALIANCE_KEEP_CANNON_2, CANNON_RANGE)
    end
    if cannon ~= nil then
        cannon:Use()
    end
    armTimer(guid, creature)
end

-- C++ npc_valiance_keep_cannoneerAI::Reset
-- (zone_borean_tundra.cpp:1425-1428) + Initialize (:1411-1416):
-- re-roll the timer. Go fires ev23 on spawn, evade and death;
-- the dead arm drops the timer (beryl idiom).
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimer(guid)
    if creature:IsDead() then
        return
    end
    armTimer(guid, creature)
end

-- Timer hygiene only: C++ has no JustDied override for this AI.
local function onDied(event, creature)
    cancelTimer(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_VALIANCE_KEEP_CANNONEER, 5, onReset)
RegisterCreatureEvent(ENTRY_VALIANCE_KEEP_CANNONEER, 23, onReset)
RegisterCreatureEvent(ENTRY_VALIANCE_KEEP_CANNONEER, 4, onDied)
