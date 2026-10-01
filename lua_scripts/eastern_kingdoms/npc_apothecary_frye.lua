-- Apothecary Frye (Shadowfang Keep)
-- Lua port of src/server/scripts/EasternKingdoms/ShadowfangKeep/
-- boss_apothecary_hummel.cpp
-- (npc_apothecary_fryeAI : public npc_apothecary_genericAI —
-- ScriptedAI via GetShadowfangKeepAI -> GetInstanceAI; registered by
-- AddSC_boss_apothecary_hummel in eastern_kingdoms_script_loader.cpp
-- (declaration line 125, call line 303)). The Shadowfang Keep block
-- is OPEN.
-- Entry (verifiable from the C++ sources): NPC_APOTHECARY_FRYE =
-- 36272 in the ApothecaryMisc enum in boss_apothecary_hummel.cpp
-- (kalecgos precedent). Whole-server-tree grep confirms
-- boss_apothecary_hummel.cpp as the only source of
-- "apothecary_hummel"/"ApothecaryHummel" for the script (loader
-- lines only otherwise).
-- Eluna creature events: 4 OnDied (the C++ overrides no other
-- event with a bridged arm — melee is engine-driven).
-- Verifiable numbers (file's own enums): SAY_FRYE_DEATH = 0.
-- Ported arm (C++ JustDied): Talk(SAY_FRYE_DEATH) -> creature:
-- Talk(0) on OnDied (najentus convention; the yell stands alone,
-- marzon precedent — C++-exact for the modeled arm).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - The npc_apothecary_genericAI DoAction legs: ACTION_START_EVENT
--   MovePoint(FryeMovePos -196.2483, 2197.224, 79.9315) with
--   EMOTE_STATE_USE_STANDING on MovementInform — no MotionMaster
--   bridge (selin_fireheart precedent); ACTION_START_FIGHT
--   SetImmuneToAll(false) + DoZoneInCombat — no immunity/combat
--   bridges; SetImmuneToPC/SetFaction(FACTION_MONSTER) — no
--   bridges.
-- - The GetShadowfangKeepAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).

local NPC_APOTHECARY_FRYE = 36272

local SAY_FRYE_DEATH = 0

local function onDied(_, creature)
    creature:Talk(SAY_FRYE_DEATH)
end

RegisterCreatureEvent(NPC_APOTHECARY_FRYE, 4, onDied)
