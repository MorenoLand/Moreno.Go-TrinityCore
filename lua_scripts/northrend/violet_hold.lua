-- Violet Hold — Lua port of
-- src/server/scripts/Northrend/VioletHold/violet_hold.cpp
-- (spell_violet_hold_portal_periodic only; the rest of the file's
-- creature/instance scripts are not ported here).

-- C++ spell_violet_hold_portal_periodic_AuraScript::PeriodicTick
-- (violet_hold.cpp:1410-1420, aura 58008): PreventDefaultAction;
-- the portal's AI gets SetData(DATA_PORTAL_PERIODIC_TICK 1,
-- tickNumber). creature:SetData is the CreatureAI::SetData
-- virtual-dispatch twin (lua_creature_events.go:767 — the
-- per-creature ScriptData store, each handler interpreting its own
-- types). Returns true, the PreventDefaultAction twin, so the Go
-- default trigger cast does not also fire.
-- LATENT: the consumer — npc_violet_hold_teleportation_portalAI::
-- SetData (:737-772, tick 1 summons a portal guardian/keeper +
-- Sinclari Talk, later ticks summon 3-4 azure invaders) — is
-- unported (instance GetData(DATA_WAVE_COUNT) / GetCreature
-- (DATA_SINCLARI_TRIGGER) / DoSummon legs have no bridges), so
-- this write sits in ScriptData until the portal AI is ported.
local SPELL_PORTAL_PERIODIC = 58008
local DATA_PORTAL_PERIODIC_TICK = 1

local function portalPeriodicTick(event, aura, target)
    target:SetData(DATA_PORTAL_PERIODIC_TICK, aura:GetTickNumber())
    return true
end

RegisterAuraEvent(SPELL_PORTAL_PERIODIC, 1, portalPeriodicTick)
