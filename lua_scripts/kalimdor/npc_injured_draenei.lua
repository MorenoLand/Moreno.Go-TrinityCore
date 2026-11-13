-- Azuremyst Isle: Injured Draenei — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (class npc_injured_draenei : CreatureScript("npc_injured_draenei");
-- struct npc_injured_draeneiAI : public ScriptedAI).
-- Entry: 16971 (Wowhead WotLK Classic NPC page, verified 2026-11-13;
-- no entry constant in the C++ sources — the creature_template
-- ScriptName binding is DB-side). Note: the C++ SDComment cites
-- quest 9582 for this NPC, but current Wowhead resolves 9582 to
-- "Strength of One" and "Aiding the Injured" has no Wowhead match;
-- the NPC identification by name + Crash Site / Ammen Vale location
-- (level 3-5, Azuremyst Isle) is unambiguous.
-- Eluna creature events: 23 OnReset.
-- Ported arms (C++ order):
-- - Reset (zone_azuremyst_isle.cpp:293-306): SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_IN_COMBAT 0x00080000) + SetHealth(15% of max) +
--   urand(0, 1) -> SetStandState(UNIT_STAND_STATE_SIT 1 /
--   UNIT_STAND_STATE_SLEEP 3). This is the whole script: UpdateAI and
--   MoveInLineOfSight are both empty in C++. The IsDead guard is the
--   marli ev23 idiom: Go fires ev23 on spawn, evade and death, while
--   C++ Reset runs on spawn + evade only.

local ENTRY_INJURED_DRAENEI = 16971

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IN_COMBAT = 0x00080000

local UNIT_STAND_STATE_SIT = 1
local UNIT_STAND_STATE_SLEEP = 3

-- C++ npc_injured_draeneiAI::Reset (zone_azuremyst_isle.cpp:293-306).
local function onReset(event, creature)
    if creature:IsDead() then
        return
    end
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IN_COMBAT)
    creature:SetHealth(creature:GetMaxHealth() * 0.15)
    if math.random(1, 2) == 1 then
        creature:SetStandState(UNIT_STAND_STATE_SIT)
    else
        creature:SetStandState(UNIT_STAND_STATE_SLEEP)
    end
end

RegisterCreatureEvent(ENTRY_INJURED_DRAENEI, 23, onReset)
