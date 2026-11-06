-- World: Item Scripts — Lua port of
-- src/server/scripts/World/item_scripts.cpp
-- (AddSC_item_scripts: item_only_for_flight, item_mysterious_egg,
-- item_disgusting_jar, item_petrov_cluster_bombs, item_dehta_trap_smasher,
-- item_captured_frog, item_generic_limit_chance_above_60).
-- Eluna item events: 2 OnUse, 4 OnExpire. A Lua false return blocks the
-- use (fireItemUseHook, item_hooks.go:367); the engine then sends
-- EQUIP_ERR_NONE (59) itself, the Eluna::OnUse tail twin. Returning true
-- (or nothing) lets the item's spell cast.
-- Ported arms (C++-exact):
-- - item_only_for_flight OnUse (item_scripts.cpp:38-80): the entries are
--   the C++ switch cases themselves (24538 / 34489 / 34475). 24538 is
--   disabled unless GetAreaId() == 3628; 34489 is disabled unless
--   GetZoneId() == 4080; 34475 always sends SPELL_FAILED_NOT_ON_GROUND
--   (171) for 45072, then falls through. In flight and not disabled the
--   use proceeds (return true); otherwise SendEquipError(39,
--   EQUIP_ERR_CANT_DO_RIGHT_NOW, ItemDefines.h:65) + return false.
--   Deviations: (1) C++ gates the 34475 SendCastResult on
--   GetSpellInfo(45072); Go's sendCastResult sends a bare-ID packet when
--   the DBC row is missing, so the gate is a no-op here — 45072 is a real
--   WotLK spell, the rows agree in practice. (2) IsInFlight is the
--   TaxiPath twin (Player::IsInFlight is the FLIGHT motion-master type;
--   IsFlying reads movement flags and is false for taxi passengers, so it
--   is NOT used here).
-- Unported (documented-only):
-- - item_mysterious_egg / item_disgusting_jar OnExpire: StoreNewItem has
--   no Go bridge (CanStoreNewItem exists, spells.go, but there is no
--   reusable item-grant path — createStarterOutfit is a one-off), and the
--   egg/jar entries are DB-side (item_template ScriptName, zero C++ consts).
-- - item_petrov_cluster_bombs / item_dehta_trap_smasher /
--   item_captured_frog OnUse: bridges live (GetZoneId/GetAreaId/
--   GetTransport/SendCastResult/GetQuestStatus/FindNearestCreature/
--   FindNearestGameObject/KilledMonsterCredit/DoAction/SetGoState) but the
--   item entries are DB-side (item_template ScriptName, zero C++ consts)
--   — pending entry verification (see CHECKPOINT tick 296 NEXT).
-- - item_generic_limit_chance_above_60 OnCastItemCombatSpell: no Go fire
--   site (ItemEvents has no combat-spell-proc hook; only OnUse 2,
--   OnQuestAccept 3 and OnExpire 4 fire). The "only used for 19169:
--   Nightfall" comment is the item entry, not a script binding.

local SPELL_ARCANE_CHARGES = 45072
local SPELL_FAILED_NOT_ON_GROUND = 171
local EQUIP_ERR_CANT_DO_RIGHT_NOW = 39

local ONLY_FOR_FLIGHT = {
    [24538] = { area = 3628 },
    [34489] = { zone = 4080 },
    [34475] = { arcaneCharges = true },
}

local function onOnlyForFlightUse(event, player, item)
    local entry = item:GetEntry()
    local cfg = ONLY_FOR_FLIGHT[entry]
    if not cfg then
        return true
    end
    local disabled = false
    if cfg.area and player:GetAreaId() ~= cfg.area then
        disabled = true
    end
    if cfg.zone and player:GetZoneId() ~= cfg.zone then
        disabled = true
    end
    if cfg.arcaneCharges then
        player:SendCastResult(SPELL_ARCANE_CHARGES, SPELL_FAILED_NOT_ON_GROUND)
    end
    if player:IsInFlight() and not disabled then
        return true
    end
    player:SendEquipError(EQUIP_ERR_CANT_DO_RIGHT_NOW, item)
    return false
end

RegisterItemEvent(24538, 2, onOnlyForFlightUse)
RegisterItemEvent(34489, 2, onOnlyForFlightUse)
RegisterItemEvent(34475, 2, onOnlyForFlightUse)
