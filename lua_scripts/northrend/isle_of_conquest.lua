-- Isle of Conquest — Lua port of
-- src/server/scripts/Northrend/IsleOfConquest/isle_of_conquest.cpp
-- (spell_ioc_parachute_ic only; the rest of the file's
-- creature/spell scripts are not ported here).
-- spell_ioc_gunship_portal (C++ :142-180) STAYS OUT: Load() gates
-- on a player caster, and a player-cast ev5 DOES fire (spells.go
-- fireSpellEffectHitTarget on the player path — the 2026-11-11
-- "no fire site" note was stale); the stay-out rests on there
-- being no player:TeleportTo bridge (TeleportNear exits the vehicle,
-- lua_player.go:4741-4744, the opposite of
-- TELE_TO_NOT_LEAVE_TRANSPORT) and no GetWorldLocation bridge;
-- the spell ID itself is DB-bound (spell_script_names) and
-- unrecoverable in this clone.

-- C++ spell_ioc_parachute_ic_AuraScript::HandleTriggerSpell
-- (isle_of_conquest.cpp:189-203, aura 66656 SPELL_PARACHUTE,
-- BattlegroundIC.h:709): PreventDefaultAction; a player target
-- whose fallTime exceeds 2000ms and who is not on a transport
-- casts the parachute (66657 SPELL_PARACHUTE_IC) triggered on
-- itself. GetFallTime is the new player bridge (lua_player.go —
-- Player::m_movementInfo.fallTime, the client-reported ms since
-- the fall started, mirrored by the session's lastFallTime);
-- GetTransport is the new player bridge (lua_player.go —
-- Player::GetTransport, nil when not riding). Returns true, the
-- PreventDefaultAction twin, so the Go default trigger cast does
-- not also fire. Load()'s caster-is-player gate is unmodeled —
-- the aura only exists on players.
local SPELL_PARACHUTE = 66656
local SPELL_PARACHUTE_IC = 66657

local function parachuteTick(event, aura, target)
    if target:GetFallTime() > 2000 and target:GetTransport() == nil then
        target:CastSpell(target, SPELL_PARACHUTE_IC, true)
    end
    return true
end

RegisterAuraEvent(SPELL_PARACHUTE, 1, parachuteTick)
