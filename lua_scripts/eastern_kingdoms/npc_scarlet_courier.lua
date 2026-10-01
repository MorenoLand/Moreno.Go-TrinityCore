-- npc_scarlet_courier (Scarlet Enclave, chapter2.cpp) --
-- Lua port of the npc_scarlet_courier struct in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter2.cpp
-- (lines 340-428).
-- Entry (verifiable from the C++ sources): chapter2.cpp's own
-- ScarletCourierEnum (line 336) names NPC_SCARLET_COURIER = 29076.
-- Whole-server-tree grep confirms chapter2.cpp as the only source
-- of "npc_scarlet_courier".
-- Verifiable numbers (file's own enum): SAY_TREE1 = 0 /
-- SAY_TREE2 = 1; SPELL_SHOOT = 52818 (never referenced by the AI
-- body); GO_INCONSPICUOUS_TREE = 191144; the stage machine ticks on
-- a 3000ms timer (Initialize/UpdateAI); MovementInform id == 1
-- advances stage 1 -> 2.
-- Bridges used: creature Talk(textId) (nefarian precedent);
-- OnEnterCombat (1) as the JustEngagedWith equivalent (maiden /
-- unworthy_initiate convention).
-- Ported arms:
-- - JustEngagedWith -> Talk(SAY_TREE2 = 1) on OnEnterCombat(1).
-- Documented deviations: the C++ arm also does me->Dismount() (no
-- Dismount bridge on the Lua creature surface) and sets uiStage = 0
-- (stops the stage machine — the machine's every leg is unbridged
-- below, so no state is modeled).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's me->Mount(14338) ("not sure about this id" per the
--   C++ comment): no Mount bridge (koltira precedent).
-- - The out-of-combat stage machine (Initialize: uiStage = 1,
--   uiStage_timer = 3000): stage 1's FindNearestGameObject(
--   GO_INCONSPICUOUS_TREE 191144, 40.0f) (no nearest-gameobject /
--   ObjectAccessor bridge — dark-rider precedent) + Talk(SAY_TREE1
--   = 0) + SetWalk(true) (no walk-state bridge) + tree->
--   GetContactPoint (no contact-point bridge) + me->
--   GetMotionMaster()->MovePoint(1, x, y, z) (no MotionMaster
--   bridge — selin_fireheart precedent); Talk(0) exists on the
--   surface but its only trigger is the unbridged stage leg — a
--   standalone timer port would be a fabricated trigger (kalecgos
--   precedent), so none of it is ported.
-- - MovementInform(POINT_ID 1) -> uiStage = 2 (no MovementInform
--   bridge — kalecgos precedent).
-- - Stage 2's tree->GetOwner() + AttackStart(unit): no gameobject-
--   owner bridge and no AttackStart bridge (unworthy_initiate
--   precedent) — strands the entire stage-2 arm.
-- - SPELL_SHOOT (52818) has no C++-verifiable use in the AI body
--   (nothing schedules a shoot arm), so it stays an enum literal.
local SAY_TREE2 = 1
local ENTRY_COURIER = 29076

local function onEnterCombat(_, creature)
    creature:Talk(SAY_TREE2)
end

RegisterCreatureEvent(ENTRY_COURIER, 1, onEnterCombat)
