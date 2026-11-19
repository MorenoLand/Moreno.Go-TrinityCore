-- Image of the Lich King (Borean Tundra, Last Rites quest chain) --
-- Lua port of npc_image_lich_kingAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 758-790;
-- AddSC at line 1887, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_IMAGE_LICH_KING = 26203 (:412). ScriptName
-- binding is DB-side (npc_hidden_cultist precedent); Arthas is summoned
-- at fixed coords (3730.313, 3518.689, 473.324) by npc_thassarian
-- waypoint 3 (:507-516) and walks to (3737.375, 3564.841, 477.433) via
-- MovePoint(0). Eluna creature events: 28 OnMovementInform. The whole
-- AI is the MovementInform arm; there are no combat timers and no UpdateAI.
-- Ported arms (C++-exact where bridges exist):
-- - MovementInform(POINT_MOTION_TYPE 8): if summoned, the summoner's
--   Thassarian AI gets arthasInPosition = true (:776-780), written
--   through the shared ThassarianScene table (zone_borean_tundra.lua),
--   nil-guarded — inert until the machine file defines it.
-- Unmodeled (documented-only, no bridges):
-- - Reset() RestoreFaction (:767-770): no Lua bridge
--   (magister_aledis/marzon precedent).
-- Verifiable numbers (file's own enums): NPC_IMAGE_LICH_KING = 26203
-- (:412).

local ENTRY_IMAGE_LICH_KING = 26203

local POINT_MOTION_TYPE = 8 -- karazhan.lua precedent

-- C++ MovementInform (zone_borean_tundra.cpp:772-780): on
-- point-motion arrival, if summoned, the summoner's Thassarian AI
-- gets arthasInPosition = true.
local function onMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    if not creature:IsSummon() then
        return
    end
    local summoner = creature:GetSummonerUnit()
    if not summoner then
        return
    end
    if ThassarianScene then
        local scene = ThassarianScene[summoner:GetGUIDLow()]
        if scene then
            scene.arthasInPosition = true
        end
    end
end

RegisterCreatureEvent(ENTRY_IMAGE_LICH_KING, 28, onMovementInform)
