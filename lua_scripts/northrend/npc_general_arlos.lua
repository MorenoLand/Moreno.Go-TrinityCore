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
--   0x8, Unit.h:214) + CastSpell(me, SPELL_STUN 46957, triggered)
--   (:807-808) — the literal C++ state write and triggered self-cast
--   (lua_creature_events.go:2682; :1126 CastSpell third-arg triggered
--   twin). 46957 is NOT in the Go spell store (13:09 tick's own WDBC
--   parse: absent from the 1395-row seed Spell.dbc), so the cast leg
--   stays silent until it is seeded (seed axis parked per the
--   2026-11-19 user redirect); the state write is the observable half.
--   Leryssa removes that aura on Talbot's death (:934); Arlos himself
--   is killed by the KillSelf leg in TALBOT's JustDied (:933), not by
--   thassarianDied — thassarianDied only despawns Talbot/Arthas.
-- Unmodeled (documented-only, no bridges):
-- - ENSURE_AI(npc_thassarian) arlosInPosition = true (:811): cross-AI
--   latch on the unported npc_thassarian escort/phase machine; no
--   cross-AI-ENSURE bridge (talbot header precedent).
-- Verifiable numbers (file's own enums): NPC_GENERAL_ARLOS = 25250
-- (:415), SPELL_STUN = 46957 (:409).

local ENTRY_GENERAL_ARLOS = 25250

local POINT_MOTION_TYPE = 8 -- karazhan.lua precedent
local UNIT_STATE_STUNNED = 8 -- Unit.h:214
local SPELL_STUN = 46957 -- zone_borean_tundra.cpp:409

-- C++ MovementInform (zone_borean_tundra.cpp:803-814): on point-motion
-- arrival, stun self (state write + triggered 46957 self-cast); the
-- thassarian latch leg stays out (see header).
local function onMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    creature:AddUnitState(UNIT_STATE_STUNNED)
    creature:CastSpell(creature, SPELL_STUN, true)
end

RegisterCreatureEvent(ENTRY_GENERAL_ARLOS, 28, onMovementInform)
