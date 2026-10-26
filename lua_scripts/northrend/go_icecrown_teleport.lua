-- Icecrown Citadel teleporters — Lua port of icecrown_citadel_teleportAI
-- OnGossipSelect in src/server/scripts/Northrend/IcecrownCitadel/
-- go_icecrown_citadel_teleport.cpp:53-80. TeleportSpells table (:29-38):
-- index 0..6 = 70781 / 70856 / 0 / 70857 / 70858 / 70859 / 70861
-- (icecrown_citadel.h:62-68); index 2 is a deliberate 0 (no spell).
-- Entries from icecrown_citadel.h:341-347, minus the Lich King transporter
-- (202223): its teleport rides the at_frozen_throne_teleport area trigger
-- (70860) under instance boss-state gates, not this gossip arm, so it is
-- left unregistered rather than risk hijacking a menu this script may not
-- own. No hello registration: the C++ class has no OnGossipHello, so the
-- native DB menu opens and only the select arm is ported.
-- Select hook args are (event, player, go, sender, action, code, listID):
-- listID is the C++ gossipListId (packet menu index), threaded by the Go
-- select hook as a trailing extension past Eluna's code argument.
-- C++-exact order: range check (menus untouched, return false) ->
-- ClearGossipMenu + CloseGossipMenu -> zero-spell check (return false) ->
-- combat check (SendCastResult SPELL_FAILED_AFFECTING_COMBAT = 1,
-- SharedDefines.h:983, return true) -> triggered player self-cast.
-- A Lua false return lets the native select arms proceed, matching the
-- C++ false paths; true vetoes them, matching the handled paths.

local TELEPORT_SPELLS = { 70781, 70856, 0, 70857, 70858, 70859, 70861 }

local GO_SCOURGE_TRANSPORTER_UPPERSPIRE = 202235
local GO_SCOURGE_TRANSPORTER_LIGHTSHAMMER = 202242
local GO_SCOURGE_TRANSPORTER_RAMPART = 202243
local GO_SCOURGE_TRANSPORTER_DEATHBRINGER = 202244
local GO_SCOURGE_TRANSPORTER_ORATORY = 202245
local GO_SCOURGE_TRANSPORTER_SINDRAGOSA = 202246

local SPELL_FAILED_AFFECTING_COMBAT = 1

local function onIcecrownTeleportSelect(_, player, _, _, _, _, listID)
    local spell = TELEPORT_SPELLS[listID + 1]
    if spell == nil then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    if spell == 0 then
        return false
    end
    if player:IsInCombat() then
        player:SendCastResult(spell, SPELL_FAILED_AFFECTING_COMBAT)
        return true
    end
    player:CastSpell(player, spell, true)
    return true
end

RegisterGameObjectGossipEvent(GO_SCOURGE_TRANSPORTER_UPPERSPIRE, 2, onIcecrownTeleportSelect)
RegisterGameObjectGossipEvent(GO_SCOURGE_TRANSPORTER_LIGHTSHAMMER, 2, onIcecrownTeleportSelect)
RegisterGameObjectGossipEvent(GO_SCOURGE_TRANSPORTER_RAMPART, 2, onIcecrownTeleportSelect)
RegisterGameObjectGossipEvent(GO_SCOURGE_TRANSPORTER_DEATHBRINGER, 2, onIcecrownTeleportSelect)
RegisterGameObjectGossipEvent(GO_SCOURGE_TRANSPORTER_ORATORY, 2, onIcecrownTeleportSelect)
RegisterGameObjectGossipEvent(GO_SCOURGE_TRANSPORTER_SINDRAGOSA, 2, onIcecrownTeleportSelect)
