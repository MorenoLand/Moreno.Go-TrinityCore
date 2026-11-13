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
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 23 OnReset. No
-- timers; melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady.
-- Ported arms:
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
-- - JustEngagedWith (chapter1.cpp:914-918): me->ExitVehicle(), no-arg
--   unseat at current position (lua_creature_events.go:3639); a
--   creature that is not seated is a no-op, matching C++.
-- - Reset (chapter1.cpp:919-930): FindNearestCreature(28782, 30.0);
--   restore the deathcharger faction from the per-GUID snapshot taken
--   in onDied before the 2096 flip (the literal RestoreFaction() has
--   no bridge; the snapshot is the template faction — the only flip is
--   onDied, and a charger with no snapshot was never flipped, so
--   skipping the restore is a faithful no-op); RemoveFlag SPELLCLICK
--   + SetFlag NOT_SELECTABLE; if not seated and the charger IsVehicle
--   with seat 0 empty (kit:GetPassenger(0) == nil), EnterVehicle
--   (lua_creature_events.go:3716, default seat -1 first-available).
--   The Oct-7 boss_ai.go audit line ("no vehicle bridge") is stale:
--   creature EnterVehicle / ExitVehicle / IsVehicle / GetVehicleKit
--   are all bridged (lua_creature_events.go:3716/3639/3494/3504).
-- Unmodeled (documented-only):
-- - The sibling scripts in chapter1.cpp are not in this file:
--   npc_dark_rider_of_acherus (TempSummon summoner chain, MoveChase,
--   DespawnOrUnsummon — bridges exist, entry unverified: spell 52266
--   summon target conflicts with 28654's registration here; round-44
--   entry check in flight), npc_salanar_the_horseman (gossip 9739
--   realm-of-shadows cast, MoveInLineOfSight vehicle machine — no
--   gossip/vehicle bridges), spell_deliver_stolen_horse (SpellScript
--   — no bridge).

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

-- Per-charger template-faction snapshot (C++ RestoreFaction() twin):
-- the charger starts at its template faction and the only flip is the
-- onDied SetFaction(2096) below, which snapshots first and never
-- overwrites. A Reset with no snapshot means the charger was never
-- flipped, so its faction is already the template faction.
local chargerFaction = {}

-- C++ npc_ros_dark_riderAI::JustDied (chapter1.cpp:932-944).
local function onDied(event, creature, killer)
    local deathcharger = creature:FindNearestCreature(ENTRY_DEATHCHARGER, 30.0)
    if not deathcharger or not killer then
        return
    end
    if killer:GetTypeId() == TYPEID_PLAYER and deathcharger:IsVehicle() then
        local guid = deathcharger:GetGUIDLow()
        if chargerFaction[guid] == nil then
            chargerFaction[guid] = deathcharger:GetFaction()
        end
        deathcharger:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_SPELLCLICK)
        deathcharger:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        deathcharger:SetFaction(FACTION_SCARLET_CRUSADE_2)
    end
end

-- C++ npc_ros_dark_riderAI::JustEngagedWith (chapter1.cpp:914-918).
local function onEnterCombat(_, creature)
    creature:ExitVehicle()
end

-- C++ npc_ros_dark_riderAI::Reset (chapter1.cpp:919-930).
local function onReset(_, creature)
    local deathcharger = creature:FindNearestCreature(ENTRY_DEATHCHARGER, 30.0)
    if not deathcharger then
        return
    end
    local faction = chargerFaction[deathcharger:GetGUIDLow()]
    if faction then
        deathcharger:SetFaction(faction)
    end
    deathcharger:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_SPELLCLICK)
    deathcharger:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    if not creature:IsOnVehicle() and deathcharger:IsVehicle() then
        local kit = deathcharger:GetVehicleKit()
        if kit and kit:GetPassenger(0) == nil then
            creature:EnterVehicle(deathcharger)
        end
    end
end

RegisterCreatureEvent(ENTRY_DARK_RIDER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_DARK_RIDER, 4, onDied)
RegisterCreatureEvent(ENTRY_DARK_RIDER, 23, onReset)
