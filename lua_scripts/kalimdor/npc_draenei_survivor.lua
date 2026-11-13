-- Azuremyst Isle: Draenei Survivor — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (class npc_draenei_survivor : CreatureScript("npc_draenei_survivor");
-- struct npc_draenei_survivorAI : public ScriptedAI).
-- Entry: 16483 (Wowhead WotLK Classic NPC page, verified 2026-11-13;
-- no entry constant in the C++ sources — the creature_template
-- ScriptName binding is DB-side). Quest 9283 "Rescue the Survivors!".
-- Eluna creature events: 23 OnReset.
-- Ported arms (C++ order):
-- - Reset (zone_azuremyst_isle.cpp:88-98): DoCast(me,
--   SPELL_IRRIDATION 35046, true) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_PLAYER_CONTROLLED 0x00000008) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_IN_COMBAT 0x00080000) + SetHealth(10% of max) +
--   SetStandState(UNIT_STAND_STATE_SLEEP 3). The IsDead guard is the
--   marli ev23 idiom: Go fires ev23 on spawn, evade and death, while
--   C++ Reset runs on spawn + evade only. Initialize() timers need no
--   Lua twin — they are consumed only by the unported UpdateAI.
-- Unmodeled (documented-only, no bridges):
-- - SpellHit (zone_azuremyst_isle.cpp:114-127): the gate is a
--   SpellFamilyFlags test (Gift of the Naaru family), and the Go ev14
--   handler receives only a bare spell id — no SpellInfo bridge.
-- - MoveInLineOfSight SayHelp Talk + UpdateAI SayThanks/RunAway
--   machine (MovePoint + DespawnOrUnsummon legs): unported.

local ENTRY_DRAENEI_SURVIVOR = 16483

local SPELL_IRRIDATION = 35046

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_PLAYER_CONTROLLED = 0x00000008
local UNIT_FLAG_IN_COMBAT = 0x00080000

local UNIT_STAND_STATE_SLEEP = 3

-- C++ npc_draenei_survivorAI::Reset (zone_azuremyst_isle.cpp:88-98).
local function onReset(event, creature)
    if creature:IsDead() then
        return
    end
    creature:CastSpell(creature, SPELL_IRRIDATION, true)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PLAYER_CONTROLLED)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IN_COMBAT)
    creature:SetHealth(creature:GetMaxHealth() * 0.1)
    creature:SetStandState(UNIT_STAND_STATE_SLEEP)
end

RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 23, onReset)
