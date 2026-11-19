-- Leryssa (Borean Tundra, Last Rites quest chain) -- Lua port of
-- npc_leryssaAI in
-- src/server/scripts/Northrend/zone_borean_tundra.cpp (lines 951-1078;
-- AddSC at line 1889, zone-script block per northrend_script_loader.cpp).
-- Entry (C++-verifiable): NPC_LERYSSA = 25251 (:416). ScriptName
-- binding is DB-side (npc_hidden_cultist precedent); Leryssa is summoned
-- at fixed coords (3749.655, 3614.960, 473.323) by npc_thassarian
-- phase 6 (:599-608) and walks to (3741.970, 3571.440, 477.441) via
-- MovePoint(0). Eluna creature events: 4 OnDied, 28 OnMovementInform.
-- Leryssa has no combat timers — the whole AI is the MovementInform
-- arms plus the phase machine the second arrival arms.
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
-- - MovementInform, second arrival (else branch, :989-999):
--   SetStandState(SIT) on self + summoner (GetSummonerUnit IS
--   bridged, lua_creature_events.go:6366 — the exact
--   ToTempSummon()->GetSummonerUnit() twin) and phase = 1 /
--   phaseTimer = 1500, arming the UpdateAI phase machine. The
--   else branch re-arms on every later arrival, C++-exact (no
--   once-guard); the C++ trigger for the second arrival is
--   Talbot's JustDied MovePoint leg (:937), unported, so the arm
--   fires once that lands (same standing as Arlos's ev28, whose
--   trigger is the unported Thassarian phase 6).
-- - UpdateAI phase machine (:1001-1072) as a one-shot re-arm
--   CreateLuaEvent pump (Barnes idiom): 1500ms initial, 5000ms
--   per phase. Phase 1/3/5/7 summoner Talk(SAY_THASSARIAN_4/5/6/7)
--   via GetSummonerUnit (nil-guarded, C++-exact); phase 2/4/6
--   self Talk(SAY_LERYSSA_2/3/4). Phase 7 also writes the
--   summoner's scene phase = 16 — nil-guarded through the shared
--   ThassarianScene table (one shared lua.State, runtime.go
--   Load; cross-file globals precedented in boss_netherspite.lua):
--   the Thassarian phase machine is unported, so the write is
--   inert until it lands. The pump stops at phase 0 (C++ spins
--   idle there — stopping is the observable twin); cancelled on
--   Died (C++ UpdateAI never runs on a dead creature).
-- - First-arrival leryssaInPosition write (:985-987), nil-guarded
--   through the same ThassarianScene table keyed by the summoner's
--   GUIDLow: inert until the Thassarian machine reads it.
-- Unmodeled (documented-only, no bridges):
-- - Ctor RemoveStandFlags(UNIT_STAND_STATE_SIT) (:969): zero hits
--   for RemoveStandFlags in the engine — no bridge.
-- - FindNearestCreature(NPC_PRINCE_VALANAR 28189, 50.0f) ->
--   ENSURE_AI(npc_counselor_talbot) bCheck = true (:977-979):
--   FindNearestCreature IS bridged, but the write target is Talbot's
--   AI field and its only consumer (Talbot UpdateAI bCheck leg,
--   :876-886, which latches the leryssa/arlos GUIDs for JustDied)
--   is unported — the write needs the Talbot-side global + JustDied
--   tail to land with it, so it stays with that unit (NEXT).
-- Verifiable numbers (file's own enums): NPC_LERYSSA = 25251
-- (:416), NPC_PRINCE_VALANAR = 28189 (:414), SPELL_STUN = 46957
-- (:409), SAY_LERYSSA_1 = 0 / SAY_LERYSSA_2 = 1 / SAY_LERYSSA_3 = 2 /
-- SAY_LERYSSA_4 = 3 (:438-441), SAY_THASSARIAN_4 = 3 /
-- SAY_THASSARIAN_5 = 4 / SAY_THASSARIAN_6 = 5 / SAY_THASSARIAN_7 = 6
-- (:420-426), UNIT_STAND_STATE_SIT = 1 (UnitDefines.h:34).

local ENTRY_LERYSSA = 25251

local POINT_MOTION_TYPE = 8 -- karazhan.lua precedent
local UNIT_STATE_STUNNED = 8 -- Unit.h:214
local SPELL_STUN = 46957 -- zone_borean_tundra.cpp:409
local UNIT_STAND_STATE_SIT = 1 -- UnitDefines.h:34

local SAY_LERYSSA_2 = 1
local SAY_LERYSSA_3 = 2
local SAY_LERYSSA_4 = 3
local SAY_THASSARIAN_4 = 3
local SAY_THASSARIAN_5 = 4
local SAY_THASSARIAN_6 = 5
local SAY_THASSARIAN_7 = 6

-- C++ bDone latch, ctor-initialized false (:963-969); per-GUID
-- because the handler is stateless across summons.
local leryssaDone = {}
-- C++ phase / phaseTimer (:959-962, :1001-1072); pump id per GUID.
local leryssaPhase = {}
local leryssaPump = {}

-- Shared Thassarian scene table, defined by zone_borean_tundra.lua
-- (the Thassarian phase machine); nil until that file defines it.
-- Both scene writes below are nil-guarded and inert until then.
local function thassarianSceneOf(creature)
    local summoner = creature:GetSummonerUnit()
    if not summoner then
        return nil, nil
    end
    if not ThassarianScene then
        return summoner, nil
    end
    return summoner, ThassarianScene[summoner:GetGUIDLow()]
end

local function cancelPhasePump(guid)
    local id = leryssaPump[guid]
    if id then
        RemoveEventById(id)
        leryssaPump[guid] = nil
    end
end

-- C++ UpdateAI phase machine (zone_borean_tundra.cpp:1001-1072):
-- one-shot re-arm pump, 5000ms per phase after the 1500ms initial
-- arm from the second arrival.
local function leryssaPhaseTick(creature, guid)
    leryssaPump[guid] = nil
    local phase = leryssaPhase[guid]
    if not phase or phase == 0 then
        return
    end
    if phase == 1 then
        local summoner = creature:GetSummonerUnit()
        if summoner then
            summoner:Talk(SAY_THASSARIAN_4)
        end
        leryssaPhase[guid] = 2
    elseif phase == 2 then
        creature:Talk(SAY_LERYSSA_2)
        leryssaPhase[guid] = 3
    elseif phase == 3 then
        local summoner = creature:GetSummonerUnit()
        if summoner then
            summoner:Talk(SAY_THASSARIAN_5)
        end
        leryssaPhase[guid] = 4
    elseif phase == 4 then
        creature:Talk(SAY_LERYSSA_3)
        leryssaPhase[guid] = 5
    elseif phase == 5 then
        local summoner = creature:GetSummonerUnit()
        if summoner then
            summoner:Talk(SAY_THASSARIAN_6)
        end
        leryssaPhase[guid] = 6
    elseif phase == 6 then
        creature:Talk(SAY_LERYSSA_4)
        leryssaPhase[guid] = 7
    elseif phase == 7 then
        local summoner, scene = thassarianSceneOf(creature)
        if summoner then
            summoner:Talk(SAY_THASSARIAN_7)
            if scene then
                scene.phase = 16
            end
        end
        leryssaPhase[guid] = 0
        return
    end
    leryssaPump[guid] = CreateLuaEvent(
        function() leryssaPhaseTick(creature, guid) end, 5000)
end

-- C++ MovementInform (zone_borean_tundra.cpp:971-999).
local function onMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    local guid = creature:GetGUIDLow()
    if not leryssaDone[guid] then
        leryssaDone[guid] = true
        creature:AddUnitState(UNIT_STATE_STUNNED)
        creature:CastSpell(creature, SPELL_STUN, true)
        -- C++ :985-987 leryssaInPosition = true on the summoner.
        local _, scene = thassarianSceneOf(creature)
        if scene then
            scene.leryssaInPosition = true
        end
        return
    end
    -- C++ else branch (:989-999): sit self + summoner, arm the
    -- phase machine (phase 1, 1500ms).
    creature:SetStandState(UNIT_STAND_STATE_SIT)
    local summoner = creature:GetSummonerUnit()
    if summoner then
        summoner:SetStandState(UNIT_STAND_STATE_SIT)
    end
    leryssaPhase[guid] = 1
    cancelPhasePump(guid)
    leryssaPump[guid] = CreateLuaEvent(
        function() leryssaPhaseTick(creature, guid) end, 1500)
end

-- Leryssa has no C++ JustDied; the pump is cancelled here so a dead
-- summon leaves no spinning timer (C++ UpdateAI never runs dead).
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPhasePump(guid)
    leryssaDone[guid] = nil
    leryssaPhase[guid] = nil
end

RegisterCreatureEvent(ENTRY_LERYSSA, 4, onDied)
RegisterCreatureEvent(ENTRY_LERYSSA, 28, onMovementInform)
