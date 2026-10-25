-- Outland: Earthmender Wilda (Coilskar Cistern escort) — Lua port of
-- src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (class npc_earthmender_wilda : CreatureScript("npc_earthmender_wilda");
-- struct npc_earthmender_wildaAI : public EscortAI).
-- Entry: 21027 (Wowhead retail + TBC Classic NPC pages agree on
-- npc=21027; no NPC_WILDA constant exists in the C++ tree — the
-- creature_template ScriptName binding is DB-side).
-- Eluna creature events: 31 OnQuestAccept, 1 OnEnterCombat,
-- 2 OnLeaveCombat, 4 OnDied, 23 OnReset.
-- Ported arms:
-- - OnQuestAccept (zone_shadowmoon_valley.cpp, event 31): quest id
--   10451 (QUEST_ESCAPE_COILSCAR) -> Talk(SAY_WIL_START 0) +
--   SetFaction(1726 FACTION_EARTHEN_RING, SharedDefines.h:254),
--   C++ order: Talk, SetFaction, Start. EscortAI::Start alone has
--   no bridge — documented-only.
-- - JustEngagedWith: rand32()%5 gate + non-player target gate +
--   urand(0,1) -> Talk(SAY_WIL_AGGRO 1); the C++ Talk targets the
--   attacker, the bridge broadcasts (willix precedent).
-- - UpdateAI healing leg: at-or-below 30% health, 15s timer ->
--   DoCast(me, SPELL_HEALING_WAVE 12491), non-triggered; the timer
--   freezes above 30% (C++ never resets it outside Reset) and the
--   first sub-30% tick casts immediately (timer starts at 0).
--   The pump runs only between enter-combat and
--   leave-combat/died/reset, matching the C++ UpdateVictim gate.
-- Unmodeled (documented-only, no bridges):
-- - WaypointReached (waypoints 13/14/15/19/20/26/27/33/34/37/38/39/
--   43/44/50): Talk arms + DoSpawnAssassin (NPC_COILSKAR_ASSASSIN
--   21044) + waypoint-50 GroupEventHappens quest credit — the
--   waypoint machine has no bridge.
-- - JustSummoned AttackStart on the assassin — no AttackStart bridge.

local ENTRY_WILDA = 21027

local SAY_WIL_START = 0
local SAY_WIL_AGGRO = 1

local QUEST_ESCAPE_COILSCAR = 10451
local FACTION_EARTHEN_RING = 1726
local SPELL_HEALING_WAVE = 12491

local healTimer = {}

local function cancelHeal(guid)
    local id = healTimer[guid]
    if id then
        RemoveEventById(id)
        healTimer[guid] = nil
    end
end

-- C++ OnQuestAccept (zone_shadowmoon_valley.cpp, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(1726); the
-- EscortAI::Start alone has no bridge.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ESCAPE_COILSCAR then
        return
    end
    creature:Talk(SAY_WIL_START)
    creature:SetFaction(FACTION_EARTHEN_RING)
end

-- C++ JustEngagedWith: rand32()%5 gate, then only a non-player
-- attacker, then urand(0,1) -> Talk(1). The heal pump starts on
-- every enter-combat, ungated by the Talk gates.
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    if not healTimer[guid] then
        local remaining = 0
        local function tick()
            if creature:IsDead() then
                healTimer[guid] = nil
                return
            end
            if creature:GetHealthPct() <= 30 then
                if remaining <= 0 then
                    creature:CastSpell(creature, SPELL_HEALING_WAVE)
                    remaining = 15000
                else
                    remaining = remaining - 1000
                end
            end
            healTimer[guid] = CreateLuaEvent(tick, 1000)
        end
        healTimer[guid] = CreateLuaEvent(tick, 1000)
    end
    if math.random(0, 4) ~= 0 then
        return
    end
    if not target or target:GetObjectType() == "Player" then
        return
    end
    if math.random(0, 1) == 1 then
        creature:Talk(SAY_WIL_AGGRO)
    end
end

local function onLeaveCombat(event, creature)
    cancelHeal(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_WILDA, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_WILDA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_WILDA, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_WILDA, 4, onLeaveCombat)
RegisterCreatureEvent(ENTRY_WILDA, 23, onLeaveCombat)
