-- The Scarlet Enclave: Dark Rider of Acherus — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter1.cpp
-- (class npc_ros_dark_rider : CreatureScript("npc_ros_dark_rider") {
-- npc_ros_dark_riderAI : public ScriptedAI }; AddSC_the_scarlet_enclave_c1
-- registers it with the other 13 chapter1 scripts; EK loader decl :95 /
-- call :273 per kalimdor_script_loader.cpp — the EK loader block).
-- Entry: 28654 wowhead-verified (classic.wowhead.com/wotlk/npc=28654/
-- dark-rider-of-acherus; no NPC_ constant in the C++ tree — belnistrasz
-- precedent). The entry is pinned by the summon clue, not the name
-- alone: spell 52266 SPELL_CALL_DARK_RIDER lists "Effect #1 Summon
-- (Dark Rider of Acherus)" pointing at 28654; the other Wowhead
-- "Dark Rider of Acherus" hits (28768, 28909) do not match the summon.
-- Eluna creature events: 4 OnDied. No timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arm:
-- - JustDied (C++ JustDied, chapter1.cpp:932-944): FindNearestCreature(
--   28782 Acherus Deathcharger, 30.0); if a deathcharger is found AND
--   the killer is a player (killer:GetTypeId() == 4 TYPEID_PLAYER) AND
--   the deathcharger IsVehicle -> SetFlag(UNIT_NPC_FLAGS, SPELLCLICK)
--   + RemoveFlag(UNIT_FIELD_FLAGS, NOT_SELECTABLE) + SetFaction(
--   FACTION_SCARLET_CRUSADE_2 2096), C++ arm order. The C++
--   deathcharger->GetTypeId() == TYPEID_UNIT check is vacuous here:
--   FindNearestCreature returns creatures only. The !deathcharger /
--   !killer early return is C++-exact (kill.go defaults a nil killer
--   to the creature itself, so the killer gate still applies).
-- Unmodeled (documented-only, no bridges):
-- - JustEngagedWith (chapter1.cpp:915-918): me->ExitVehicle() —
--   ExitVehicle is bridged on the PLAYER surface only
--   (lua_player.go:2796); no creature ExitVehicle bridge.
-- - Reset (chapter1.cpp:920-930): the deathcharger faction restore +
--   SPELLCLICK clear + NOT_SELECTABLE set + EnterVehicle mount legs —
--   EnterVehicle / GetVehicleKit()->HasEmptySeat have no bridges.
--   The Oct-7 boss_ai.go audit line ("no vehicle bridge") is stale
--   ONLY for the JustDied arm: creature:IsVehicle IS bridged
--   (lua_creature_events.go:3113), which is all JustDied needs.
-- - The sibling scripts in chapter1.cpp are not in this file:
--   npc_dark_rider_of_acherus (TempSummon summoner chain, MoveChase,
--   DespawnOrUnsummon — no summon/movement bridges), npc_salanar_
--   the_horseman (gossip 9739 realm-of-shadows cast, MoveInLineOfSight
--   vehicle machine — no gossip/vehicle bridges), spell_deliver_
--   stolen_horse (SpellScript — no bridge).

local ENTRY_DARK_RIDER = 28654
local ENTRY_DEATHCHARGER = 28782

-- UnitDefines.h:210 UNIT_NPC_FLAG_SPELLCLICK = 0x01000000, on field 82
-- (UNIT_NPC_FLAGS, belnistrasz precedent); SharedDefines.h: UNIT_FLAG_
-- NOT_SELECTABLE = 0x02000000 on field 59 (UNIT_FIELD_FLAGS, curator
-- precedent); SharedDefines.h:266 FACTION_SCARLET_CRUSADE_2 = 2096.
local UNIT_NPC_FLAGS = 82
local UNIT_NPC_FLAG_SPELLCLICK = 0x01000000
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local FACTION_SCARLET_CRUSADE_2 = 2096

local TYPEID_PLAYER = 4

-- C++ npc_ros_dark_riderAI::JustDied (chapter1.cpp:932-944).
local function onDied(event, creature, killer)
    local deathcharger = creature:FindNearestCreature(ENTRY_DEATHCHARGER, 30.0)
    if not deathcharger or not killer then
        return
    end
    if killer:GetTypeId() == TYPEID_PLAYER and deathcharger:IsVehicle() then
        deathcharger:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_SPELLCLICK)
        deathcharger:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        deathcharger:SetFaction(FACTION_SCARLET_CRUSADE_2)
    end
end

RegisterCreatureEvent(ENTRY_DARK_RIDER, 4, onDied)
