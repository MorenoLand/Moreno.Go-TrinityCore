-- General Arlos (Borean Tundra, Last Rites quest chain) -- Lua port of
-- npc_general_arlosAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 790-820;
-- AddSC at line 1890, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_GENERAL_ARLOS = 25250 (:415). ScriptName
-- binding is DB-side (npc_hidden_cultist precedent); Arlos is summoned
-- at fixed coords (3745.527, 3615.655, 473.322) by npc_thassarian
-- (:594-598) and walks to (3735.570, 3572.420, 477.441) via MovePoint(0).
-- Eluna creature events: 28 OnMovementInform. Arlos has no combat
-- timers and no UpdateAI — the whole AI is the MovementInform arm.
-- Ported arms (C++-exact where bridges exist):
-- - MovementInform(POINT_MOTION_TYPE 8): AddUnitState(UNIT_STATE_STUNNED
--   0x8, Unit.h:214) — the literal C++ state write
--   (lua_creature_events.go:2682; torloth UNIT_STATE_ROOT precedent).
--   The C++ also casts SPELL_STUN 46957 on self, triggered (:808),
--   and Leryssa removes that aura on Talbot's death (:934); 46957 is
--   NOT in the Go spell store (grep-0 in tools/spelldbc_seed/main.go
--   and the engine tree), so the cast leg stays out — the seed axis
--   is parked per the 2026-11-19 user redirect. The state write is
--   the observable half: Arlos stands stunned at his waypoint until
--   Talbot dies and kills him (KillSelf leg in
--   zone_borean_tundra.lua thassarianDied).
-- Unmodeled (documented-only, no bridges):
-- - ENSURE_AI(npc_thassarian) arlosInPosition = true (:811): cross-AI
--   latch on the unported npc_thassarian escort/phase machine; no
--   cross-AI-ENSURE bridge (talbot header precedent).
-- Verifiable numbers (file's own enums): NPC_GENERAL_ARLOS = 25250
-- (:415), SPELL_STUN = 46957 (:409).

local ENTRY_GENERAL_ARLOS = 25250

local POINT_MOTION_TYPE = 8 -- karazhan.lua precedent
local UNIT_STATE_STUNNED = 8 -- Unit.h:214

-- C++ MovementInform (zone_borean_tundra.cpp:803-814): on point-motion
-- arrival, stun self; the thassarian latch leg stays out (see header).
local function onMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    creature:AddUnitState(UNIT_STATE_STUNNED)
end

RegisterCreatureEvent(ENTRY_GENERAL_ARLOS, 28, onMovementInform)
