-- Icecrown Citadel teleporters — Lua port of icecrown_citadel_teleportAI
-- OnGossipSelect in src/server/scripts/Northrend/IcecrownCitadel/
-- go_icecrown_citadel_teleport.cpp:53-80. TeleportSpells table (:29-38):
-- index 0..6 = 70781 / 70856 / 0 / 70857 / 70858 / 70859 / 70861
-- (icecrown_citadel.h:62-68); index 2 is a deliberate 0 (no spell).
-- Entries from icecrown_citadel.h:341-347, minus the Lich King transporter
-- (202223): its teleport rides the at_frozen_throne_teleport area trigger
-- (70860) under instance boss-state gates, not this gossip arm — PORTED
-- 2026-11-15 (at_* census) at the foot of this file, so it stays
-- unregistered here rather than risk hijacking a menu this script may not
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

-- C++ at_frozen_throne_teleport (go_icecrown_citadel_teleport.cpp:83-106),
-- trigger 5718 (TDB 335.26091 areatrigger_scripts row). Fire site:
-- engine/world/areatrigger.go:64 (ServerEventAreaTrigger = 24).
-- PORTED 2026-11-15 (at_* census). Boss-state gates read through the
-- first bound creature (the instance file's firstBoundCreature idiom):
-- creature:GetBossState reads the shared (map, instance) store
-- (lua_creature_events.go:7559). C++ order: combat check first
-- (SendCastResult SPELL_FAILED_AFFECTING_COMBAT = 1, same call as
-- the gossip arm above), then the boss gates, then the triggered
-- self-cast.
local TRIGGER_FROZEN_THRONE_TELEPORT = 5718 -- TDB areatrigger_scripts row
local FROZEN_THRONE_TELEPORT = 70860 -- icecrown_citadel.h:67

local DATA_PROFESSOR_PUTRICIDE = 6 -- icecrown_citadel.h:80
local DATA_BLOOD_QUEEN_LANA_THEL = 8 -- icecrown_citadel.h:82
local DATA_SINDRAGOSA = 11 -- icecrown_citadel.h:85
local DATA_THE_LICH_KING = 12 -- icecrown_citadel.h:86
local BOSS_STATE_DONE = 3 -- InstanceScript.h:68-76
local BOSS_STATE_IN_PROGRESS = 1

RegisterServerEvent(24, function(event, player, triggerId)
    if triggerId ~= TRIGGER_FROZEN_THRONE_TELEPORT then
        return
    end
    if player:IsInCombat() then
        player:SendCastResult(FROZEN_THRONE_TELEPORT, SPELL_FAILED_AFFECTING_COMBAT)
        return
    end
    local mapObj = player:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local caller = data.lordMarrowgar or data.ladyDeathwhisper
        or data.deathbringerSaurfang or data.festergut
        or data.rotface or data.professorPutricide
        or data.bloodQueenLanaThel or data.sindragosa
        or data.theLichKing
    if not caller then
        return
    end
    if caller:GetBossState(DATA_PROFESSOR_PUTRICIDE) == BOSS_STATE_DONE
        and caller:GetBossState(DATA_BLOOD_QUEEN_LANA_THEL) == BOSS_STATE_DONE
        and caller:GetBossState(DATA_SINDRAGOSA) == BOSS_STATE_DONE
        and caller:GetBossState(DATA_THE_LICH_KING) ~= BOSS_STATE_IN_PROGRESS then
        player:CastSpell(player, FROZEN_THRONE_TELEPORT, true)
    end
end)
