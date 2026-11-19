-- Leryssa (Borean Tundra, Last Rites quest chain) -- Lua port of
-- npc_leryssaAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 951-1078;
-- AddSC at line 1889, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_LERYSSA = 25251 (:416). ScriptName
-- binding is DB-side (npc_hidden_cultist precedent); Leryssa is summoned
-- at fixed coords (3749.655, 3614.960, 473.323) by npc_thassarian
-- phase 6 (:599-608) and walks to (3741.970, 3571.440, 477.441) via
-- MovePoint(0). Eluna creature events: 28 OnMovementInform. Leryssa has
-- no combat timers — the whole AI is the MovementInform arm plus the
-- phase machine it arms on the second arrival.
-- Ported arms (C++-exact where bridges exist):
-- - MovementInform(POINT_MOTION_TYPE 8), first arrival (!bDone):
--   AddUnitState(UNIT_STATE_STUNNED 0x8, Unit.h:214) + CastSpell(me,
--   SPELL_STUN 46957, triggered) (:980-981) — the literal C++
--   stun write and triggered self-cast (lua_creature_events.go:2682
--   AddUnitState; :1126 CastSpell third-arg triggered twin).
--   46957 is NOT in the Go spell store (13:09 tick's own WDBC parse:
--   absent from the 1395-row seed Spell.dbc), so the cast leg stays
--   silent until it is seeded (seed axis parked per the 2026-11-19
--   user redirect); the state write is the observable half. The
--   bDone latch is per-GUID (C++ bDone is ctor-initialized false;
--   Leryssa has no Reset arm).
-- Unmodeled (documented-only, no bridges):
-- - Ctor RemoveStandFlags(UNIT_STAND_STATE_SIT) (:969): zero hits
--   for RemoveStandFlags in the engine — no bridge.
-- - FindNearestCreature(NPC_PRINCE_VALANAR 28189, 50.0f) ->
--   ENSURE_AI(npc_counselor_talbot) bCheck = true (:977-979):
--   FindNearestCreature IS bridged, but the write target is Talbot's
--   AI field and its only consumer (Talbot UpdateAI bCheck leg,
--   :876-886, which latches the leryssa/arlos GUIDs for JustDied)
--   is unported — a set with no reader in the Go model.
-- - ENSURE_AI(npc_thassarian) leryssaInPosition = true (:985-987):
--   cross-AI latch on the unported npc_thassarian escort/phase
--   machine; no cross-AI-ENSURE bridge (talbot/arlos header
--   precedent).
-- - Second arrival (else branch, :989-999): SetStandState(SIT) on
--   self + summoner (GetSummonerUnit IS bridged,
--   lua_creature_events.go:6366) and phase = 1 / phaseTimer = 1500,
--   arming the UpdateAI phase machine (:1001-1072: Leryssa Talk
--   SAY_LERYSSA_2/3/4, summoner Talk SAY_THASSARIAN_4/5/6/7,
--   phase 7 writes thassarian phase = 16). The machine stays out:
--   its trigger is Talbot's JustDied MovePoint leg (:937, unported
--   — RewardPlayerAndGroupAtEvent has no bridge), so the second
--   arrival never fires in the ported model, and the phase-7
--   thassarian phase write has no bridge. The summoner:Talk legs
--   are bridgeable in isolation (guard:Talk/boss:Talk precedents)
--   but ride the unported machine, so they stay with it.
-- Verifiable numbers (file's own enums): NPC_LERYSSA = 25251
-- (:416), NPC_PRINCE_VALANAR = 28189 (:414), SPELL_STUN = 46957
-- (:409), SAY_LERYSSA_1 = 0 / SAY_LERYSSA_2 = 1 / SAY_LERYSSA_3 = 2 /
-- SAY_LERYSSA_4 = 3 (:438-441).

local ENTRY_LERYSSA = 25251

local POINT_MOTION_TYPE = 8 -- karazhan.lua precedent
local UNIT_STATE_STUNNED = 8 -- Unit.h:214
local SPELL_STUN = 46957 -- zone_borean_tundra.cpp:409

-- C++ bDone latch, ctor-initialized false (:963-969); per-GUID
-- because the handler is stateless across summons.
local leryssaDone = {}

-- C++ MovementInform (zone_borean_tundra.cpp:971-999), first-arrival
-- arm only: stun self (state write + triggered 46957 self-cast),
-- latch bDone. The bCheck / leryssaInPosition cross-AI legs and the
-- second-arrival phase-machine arm stay out (see header).
local function onMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUIDLow()
    if leryssaDone[guid] then
        return
    end
    leryssaDone[guid] = true
    creature:AddUnitState(UNIT_STATE_STUNNED)
    creature:CastSpell(creature, SPELL_STUN, true)
end

RegisterCreatureEvent(ENTRY_LERYSSA, 28, onMovementInform)
