-- Ashenvale: Ruul Snowhoof — Lua port of
-- src/server/scripts/Kalimdor/zone_ashenvale.cpp
-- (class npc_ruul_snowhoof : CreatureScript("npc_ruul_snowhoof");
-- struct npc_ruul_snowhoofAI : public EscortAI).
-- Entry: 12818 (Wowhead Classic NPC page, verified this tick;
-- no entry constant in the C++ sources — the creature_template
-- ScriptName binding is DB-side).
-- Eluna creature events: 6 OnReachWP, 23 OnReset, 31 OnQuestAccept.
-- Ported arms (C++ order):
-- - Reset (zone_ashenvale.cpp:74-78): the nearest cage GO
--   (GO_CAGE 178147, 20yd) gets SetGoState(1) (GO_STATE_READY,
--   netherspite precedent). The IsDead guard is the marli ev23
--   idiom: Go fires ev23 on spawn, evade and death, while C++
--   Reset runs on spawn + evade only. Reset also latches the
--   spawn-time faction (the creature_template faction C++
--   WaypointReached 27 restores) — first Reset only, so an
--   evade-time Reset never overwrites it with the escort 113.
-- - OnQuestAccept (zone_ashenvale.cpp:87-94): quest id 6482
--   (QUEST_FREEDOM_TO_RUUL) -> SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, SharedDefines.h:240) +
--   StartEscort(player), C++ order (muglash precedent — the escort
--   bridges landed ticks 372-373; the old "no escort bridge" claim
--   is stale) + MoveWaypoint() (StartEscort only latches the escort
--   player; the walk rides MoveWaypoint + OnReachWP (6) — willix
--   precedent). No Talk leg in C++ here.
-- - WaypointReached (zone_ashenvale.cpp:97-131): the escort-player
--   nil-guard at the top (GetEscortPlayer); wp0 SetStandState(0)
--   (C++ SetUInt32Value(UNIT_FIELD_BYTES_1, 0) — the stand byte is
--   the only live byte for this NPC) + cage SetGoState(0)
--   (GO_STATE_ACTIVE); wp13/wp19 summon the Thistlefur wave
--   (3922 totemic / 3921 ursa / 3926 pathfinder at the C++
--   RuulSnowhoofSummonsCoord rows, TEMPSUMMON_DEAD_DESPAWN 6,
--   60s) with AttackStart(Ruul) inline at the summon site (the
--   C++ JustSummoned leg :82-85 — willix precedent; ev19 stays
--   unregistered, the arm is covered inline); wp27 restores the
--   latched template faction + RemoveAura(20514) (C++
--   RemoveAurasDueToSpell(RUUL_SHAPECHANGE) — the Go aura store
--   holds one aura per spell per unit, so RemoveAura is the exact
--   twin) + Talk(0, player) + player:GroupEventHappens(6482) +
--   StopEscort (the C++ escort ends at the last waypoint; Go paths
--   repeat by default — willix/thrall precedent).
-- Unmodeled (documented-only, no bridges):
-- - EnterEvadeMode shapechange re-apply (RUUL_SHAPECHANGE):
--   EnterEvadeMode IS bridged (lua_creature_events.go:5638); the
--   re-apply leg is outside the 8-hook set, port status unaudited.

local ENTRY_RUUL_SNOWHOOF = 12818

local GO_CAGE = 178147
local GO_STATE_READY = 1
local GO_STATE_ACTIVE = 0

local QUEST_FREEDOM_TO_RUUL = 6482
local FACTION_ESCORTEE_N_NEUTRAL_PASSIVE = 113

local NPC_THISTLEFUR_TOTEMIC = 3922
local NPC_THISTLEFUR_URSA = 3921
local NPC_THISTLEFUR_PATHFINDER = 3926
local RUUL_SHAPECHANGE = 20514
local SAY_FINISH = 0

-- C++ RuulSnowhoofSummonsCoord[6] (zone_ashenvale.cpp:55-64):
-- wp13 wave rows 0-2, wp19 wave rows 3-5.
local SUMMON_COORDS = {
    { 3449.218018, -587.825073, 174.978867, 4.714445 },
    { 3446.384521, -587.830872, 175.186279, 4.714445 },
    { 3444.218994, -587.835327, 175.380600, 4.714445 },
    { 3508.344482, -492.024261, 186.929031, 4.145029 },
    { 3506.265625, -490.531006, 186.740128, 4.239277 },
    { 3503.682373, -489.393799, 186.629684, 4.349232 },
}
local SUMMON_ENTRIES = { NPC_THISTLEFUR_TOTEMIC, NPC_THISTLEFUR_URSA, NPC_THISTLEFUR_PATHFINDER }
local SUMMON_TYPE_DEAD_DESPAWN = 6
local SUMMON_MS = 60000

-- Spawn-time template faction per creature GUID (C++ wp27 restores
-- GetCreatureTemplate()->faction; no template-faction read bridge
-- exists, so Reset latches it — first Reset only).
local templateFaction = {}

-- C++ npc_ruul_snowhoofAI::Reset (zone_ashenvale.cpp:74-78).
local function ruulReset(event, creature)
    if creature:IsDead() then
        return
    end
    local guid = creature:GetGUIDLow()
    if templateFaction[guid] == nil then
        templateFaction[guid] = creature:GetFaction()
    end
    local cage = creature:FindNearestGameObject(GO_CAGE, 20.0)
    if cage then
        cage:SetGoState(GO_STATE_READY)
    end
end

-- C++ npc_ruul_snowhoofAI::OnQuestAccept (zone_ashenvale.cpp:87-94):
-- SetFaction + EscortAI::Start, C++ order.
local function ruulQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_FREEDOM_TO_RUUL then
        return
    end
    creature:SetFaction(FACTION_ESCORTEE_N_NEUTRAL_PASSIVE)
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ npc_ruul_snowhoofAI::WaypointReached (zone_ashenvale.cpp:97-131).
local function onReachWP(event, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:SetStandState(0)
        local cage = creature:FindNearestGameObject(GO_CAGE, 20.0)
        if cage then
            cage:SetGoState(GO_STATE_ACTIVE)
        end
    elseif wp == 13 or wp == 19 then
        local base = wp == 13 and 1 or 4
        for i = 1, 3 do
            local pos = SUMMON_COORDS[base + i - 1]
            local summoned = creature:SummonCreature(SUMMON_ENTRIES[i], pos[1], pos[2], pos[3], pos[4], SUMMON_TYPE_DEAD_DESPAWN, SUMMON_MS)
            if summoned then
                summoned:AttackStart(creature)
            end
        end
    elseif wp == 27 then
        local guid = creature:GetGUIDLow()
        local faction = templateFaction[guid]
        if faction then
            creature:SetFaction(faction)
        end
        creature:RemoveAura(RUUL_SHAPECHANGE)
        creature:Talk(SAY_FINISH, player)
        player:GroupEventHappens(QUEST_FREEDOM_TO_RUUL, creature)
        creature:StopEscort()
    end
end

RegisterCreatureEvent(ENTRY_RUUL_SNOWHOOF, 6, onReachWP)
RegisterCreatureEvent(ENTRY_RUUL_SNOWHOOF, 23, ruulReset)
RegisterCreatureEvent(ENTRY_RUUL_SNOWHOOF, 31, ruulQuestAccept)
