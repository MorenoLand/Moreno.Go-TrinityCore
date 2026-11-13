-- Wintergrasp spell scripts — Lua port of
-- src/server/scripts/Northrend/zone_wintergrasp.cpp (spell scripts only).

local SPELL_WINTERGRASP_DEFENDER_TELEPORT = 54640 -- zone_wintergrasp.cpp:496 (class spell_wintergrasp_defender_teleport)
local SPELL_WINTERGRASP_TELEPORT_TRIGGER = 54643 -- zone_wintergrasp.cpp:493 (enum WgTeleport)
local AREA_WINTERGRASP = 4197 -- SharedDefines.h:3834
local SPELL_FAILED_BAD_TARGETS = 12 -- SharedDefines.h:994
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_wintergrasp_defender_teleport::CheckCast (zone_wintergrasp.cpp:505-513):
-- fail BAD_TARGETS when the explicit target is a player who is in
-- Wintergrasp on the non-defender team, or who already has the
-- teleport-trigger aura (54643). A nil / non-player target skips the
-- gate (SPELL_CAST_OK), as does a missing battlefield — the Go twin of
-- the always-registered C++ battlefield is getOrCreateWGState, so the
-- defender read never misses. The 54643 trigger HandleDummy leg stays
-- out: it only writes the spell's explicit target destination
-- (SetExplTargetDest, zone_wintergrasp.cpp:538-541) and the Lua spell
-- object has no dest write (the ev5 player-path site itself is live
-- since a910c39d). The second 54640 row (spell_gen_cast_target_to_target)
-- has no C++ loader in this tree. Binding 54640 is the TDB 26091
-- spell_script_names row (spell_wintergrasp_defender_teleport); no AC
-- spell_script_names row exists for 54640 (grep count 0).
-- RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast).
local function wgDefenderTeleportCheckCast(event, spell)
    local target = spell:GetTarget()
    if target == nil or target:GetObjectType() ~= "Player" then
        return SPELL_CAST_OK
    end
    if (target:GetZoneId() == AREA_WINTERGRASP and target:GetTeam() ~= target:GetWGDefenderTeam()) or target:HasAura(SPELL_WINTERGRASP_TELEPORT_TRIGGER) then
        return SPELL_FAILED_BAD_TARGETS
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_WINTERGRASP_DEFENDER_TELEPORT, 8, wgDefenderTeleportCheckCast)

local SPELL_WINTERGRASP_GRAB_PASSENGER = 61178 -- zone_wintergrasp.cpp:440 (class spell_wintergrasp_grab_passenger); battleground_wg.go WGSpellGrabPassenger
local SPELL_RIDE_WG_VEHICLE = 60968 -- zone_wintergrasp.cpp:61

-- C++ spell_wintergrasp_grab_passenger::HandleScript
-- (zone_wintergrasp.cpp:449-453): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT — the hit player casts Ride WG
-- Vehicle (60968) on the caster, non-triggered. ev5 fires once
-- per (effect, live hit target) from the creature cast path
-- (xevozzSummonPlayers precedent). DORMANT: Go never casts
-- 61178 (WGSpellGrabPassenger unused) and no Spell.dbc.
local function wgGrabPassenger(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetObjectType() ~= "Player" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:CastSpell(caster, SPELL_RIDE_WG_VEHICLE)
end

RegisterSpellEvent(SPELL_WINTERGRASP_GRAB_PASSENGER, 5, wgGrabPassenger)

local SPELL_BUILD_CATAPULT_FORCE = 56664 -- zone_wintergrasp.cpp:58
local SPELL_BUILD_DEMOLISHER_FORCE = 56659 -- zone_wintergrasp.cpp:59
local SPELL_BUILD_SIEGE_VEHICLE_FORCE_ALLIANCE = 56662 -- zone_wintergrasp.cpp:57
local SPELL_BUILD_SIEGE_VEHICLE_FORCE_HORDE = 61409 -- zone_wintergrasp.cpp:56

-- C++ spell_wintergrasp_force_building::HandleScript
-- (zone_wintergrasp.cpp:425-429): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT — the hit unit casts GetEffectValue()
-- on itself, non-triggered. PreventHitDefaultEffect omitted per the
-- azuregos precedent: the player-path spell object has no such method
-- and the player funnel never consults the veto. Do NOT use the
-- battleground_wg.go WGSpellBuild*Force constants (56663/56575/56661/
-- 61408) — those are the effect-value build spells, each one off from
-- its force spell; the handler casts GetEffectValue(), not a constant.
-- DORMANT: no Spell.dbc; the only cast site (demolisher-engineer
-- gossip DoCast, zone_wintergrasp.cpp:161-168) is unported in Go.
local function wgForceBuilding(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, spell:GetEffectValue(0), false)
end

RegisterSpellEvent(SPELL_BUILD_CATAPULT_FORCE, 5, wgForceBuilding)
RegisterSpellEvent(SPELL_BUILD_DEMOLISHER_FORCE, 5, wgForceBuilding)
RegisterSpellEvent(SPELL_BUILD_SIEGE_VEHICLE_FORCE_ALLIANCE, 5, wgForceBuilding)
RegisterSpellEvent(SPELL_BUILD_SIEGE_VEHICLE_FORCE_HORDE, 5, wgForceBuilding)
