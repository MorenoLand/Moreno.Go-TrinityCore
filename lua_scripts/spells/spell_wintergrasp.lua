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
-- out: the player path has no OnEffectHitTarget site (death-gate
-- precedent). Binding 54640 is the AC spell_script_names row; the
-- second 54640 row (spell_gen_cast_target_to_target) has no C++ loader
-- and is a stale TDB row. RegisterSpellEvent OnCheckCast = 8
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
