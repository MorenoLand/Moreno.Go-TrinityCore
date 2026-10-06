-- Azshara: Depth Charge — Lua port of
-- src/server/scripts/Kalimdor/zone_azshara.cpp
-- (npc_depth_chargeAI : public ScriptedAI; registered in AddSC_azshara()).
-- Entry: 23025 (NPC_DEPTH_CHARGE, in-enum).
-- Ported: MoveInLineOfSight -> on a player within 5 yards
-- (IsWithinDistInMap), DoCast SPELL_DEPTH_CHARGE_TRAP 38576 on that
-- player, then despawn after the 1s WeMustDieTimer. The event-27 veto
-- (return true) suppresses the default aggro engage, matching the
-- C++ empty AttackStart/JustEngagedWith overrides.
-- Zero Talk() in C++.
-- Documented-only: Reset's SetHover/SetSwim/SetFlag NOT_SELECTABLE —
-- no Lua bridge for those unit flags; the trap's passive/invisible
-- posture is a template concern, not scripted here. The summoning leg
-- (SPELL_PERIODIC_DEPTH_CHARGE from npc_rizzle_sprysprocket) and the
-- rizzle AI itself are documented-only: no escort / quest-status /
-- gossip / MotionMaster bridges exist, and rizzle's own entry ID is
-- not C++-verifiable.
local ENTRY = 23025
local SPELL_DEPTH_CHARGE_TRAP = 38576
local PROXIMITY_YARDS = 5
local DESPAWN_DELAY_MS = 1000

local function onMoveInLOS(event, creature, player)
    if creature:IsWithinDistInMap(player, PROXIMITY_YARDS) then
        creature:CastSpell(player, SPELL_DEPTH_CHARGE_TRAP)
        CreateLuaEvent(function()
            creature:Despawn()
        end, DESPAWN_DELAY_MS)
    end
    return true
end

RegisterCreatureEvent(ENTRY, 27, onMoveInLOS)
