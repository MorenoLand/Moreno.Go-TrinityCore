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
-- - item_petrov_cluster_bombs OnUse (item_scripts.cpp:130-154), entry
--   33098 (Wowhead wotlk/item=33098, Use spell 42406): outside Howling
--   Fjord (zone 495) the use proceeds; on a transport in the Shattered
--   Straits (area 4064) it proceeds too; otherwise EQUIP_ERR_NONE (59) +
--   SendCastResult(42406, 60 SPELL_FAILED_NOT_HERE) + return false.
--   Same GetSpellInfo no-op-gate deviation as 45072 above.
-- - item_dehta_trap_smasher OnUse (item_scripts.cpp:203-232), entry
--   35228 (Wowhead wotlk/item=35228, provided for quest 11876): quest
--   incomplete + calf 25850 within 5yd + a mammoth-trap GO within 11yd
--   (enum order) -> calf DoAction(1), trap SetGoState(READY 1),
--   KilledMonsterCredit(25850), return false; any missing leg lets the
--   use proceed (return true).
-- - item_captured_frog OnUse (item_scripts.cpp:238-258), entry 53510
--   (Wowhead wotlk/item=53510, Use: attune at Vanira's Sentry Totem):
--   quest 25444 incomplete + totem 40187 within 10yd -> proceed;
--   incomplete without the totem -> EQUIP_ERR_OUT_OF_RANGE (25);
--   any other quest state -> EQUIP_ERR_CANT_DO_RIGHT_NOW (39); both
--   block. CORRECTION: quest 25444 is "Da Perfect Spies" (Horde,
--   Durotar) per Wowhead, not "The Perfect Spies"/Sholazar Basin — the
--   C++ cites only the quest ID, so no code impact.
-- Unported (documented-only):
-- - item_mysterious_egg / item_disgusting_jar OnExpire: StoreNewItem has
--   no Go bridge (CanStoreNewItem exists, spells.go, but there is no
--   reusable item-grant path — createStarterOutfit is a one-off), and the
--   egg/jar entries are DB-side (item_template ScriptName, zero C++ consts).
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

local QUEST_CANNOT_HELP_THEMSELVES = 11876
local QUEST_STATUS_INCOMPLETE = 3
local ENTRY_TRAPPED_MAMMOTH_CALF = 25850
local GO_STATE_READY = 1

local MAMMOTH_TRAPS = {
    188022, 188024, 188025, 188026, 188027, 188028, 188029, 188030,
    188031, 188032, 188033, 188034, 188035, 188036, 188037, 188038,
    188039, 188040, 188041, 188042, 188043, 188044,
}

local SPELL_PETROV_BOMB = 42406
local SPELL_FAILED_NOT_HERE = 60
local EQUIP_ERR_NONE = 59
local EQUIP_ERR_OUT_OF_RANGE = 25

local QUEST_THE_PERFECT_SPIES = 25444
local ENTRY_VANIRAS_SENTRY_TOTEM = 40187

-- item_petrov_cluster_bombs OnUse (item_scripts.cpp:130-154): outside
-- Howling Fjord the use proceeds (return true); on a transport in the
-- Shattered Straits it proceeds too; otherwise EQUIP_ERR_NONE +
-- SendCastResult(42406, SPELL_FAILED_NOT_HERE) + return false.
local function onPetrovUse(event, player, item)
    if player:GetZoneId() ~= 495 then -- ZONE_ID_HOWLING
        return true
    end
    if player:GetTransport() == nil or player:GetAreaId() ~= 4064 then -- AREA_ID_SHATTERED_STRAITS
        player:SendEquipError(EQUIP_ERR_NONE, item)
        player:SendCastResult(SPELL_PETROV_BOMB, SPELL_FAILED_NOT_HERE)
        return false
    end
    return true
end

-- item_dehta_trap_smasher OnUse (item_scripts.cpp:203-232): quest 11876
-- incomplete + a trapped mammoth calf within 5yd + a mammoth trap GO
-- within 11yd (enum order) -> DoAction(1) on the calf (its 1.5s
-- MovePoint leg), trap SetGoState(READY), KilledMonsterCredit(25850),
-- return false (handled). Any missing leg lets the use proceed.
local function onDehtaTrapSmasherUse(event, player, item)
    if player:GetQuestStatus(QUEST_CANNOT_HELP_THEMSELVES) ~= QUEST_STATUS_INCOMPLETE then
        return true
    end
    local mammoth = player:FindNearestCreature(ENTRY_TRAPPED_MAMMOTH_CALF, 5.0)
    if not mammoth then
        return true
    end
    for _, trapEntry in ipairs(MAMMOTH_TRAPS) do
        local trap = player:FindNearestGameObject(trapEntry, 11.0)
        if trap then
            mammoth:DoAction(1)
            trap:SetGoState(GO_STATE_READY)
            player:KilledMonsterCredit(ENTRY_TRAPPED_MAMMOTH_CALF)
            return false
        end
    end
    return true
end

-- item_captured_frog OnUse (item_scripts.cpp:238-258): quest 25444
-- incomplete + Vanira's Sentry Totem within 10yd -> the use proceeds
-- (return true); incomplete without the totem -> EQUIP_ERR_OUT_OF_RANGE;
-- any other quest state -> EQUIP_ERR_CANT_DO_RIGHT_NOW; both block.
local function onCapturedFrogUse(event, player, item)
    if player:GetQuestStatus(QUEST_THE_PERFECT_SPIES) == QUEST_STATUS_INCOMPLETE then
        if player:FindNearestCreature(ENTRY_VANIRAS_SENTRY_TOTEM, 10.0) then
            return true
        end
        player:SendEquipError(EQUIP_ERR_OUT_OF_RANGE, item)
    else
        player:SendEquipError(EQUIP_ERR_CANT_DO_RIGHT_NOW, item)
    end
    return false
end

RegisterItemEvent(33098, 2, onPetrovUse)
RegisterItemEvent(35228, 2, onDehtaTrapSmasherUse)
RegisterItemEvent(53510, 2, onCapturedFrogUse)
