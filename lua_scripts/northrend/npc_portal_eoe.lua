-- npc_portal_eoe (Eye of Eternity) — Lua port of
-- src/server/scripts/Northrend/Nexus/EyeOfEternity/boss_malygos.cpp
-- (class npc_portal_eoe : public CreatureScript, NPC_PORTAL_TRIGGER).
-- Entry: 30118 (eye_of_eternity.h NPC_PORTAL_TRIGGER, line 51);
-- registered via engine/world/boss_ai.go:RegisterLuaBoss("npc_portal_eoe", 30118).
-- Ported arms (C++-exact unless noted):
-- SpellHit SPELL_PORTAL_OPENED (61236) -> phase-one-gated DoCast
-- SPELL_SUMMON_POWER_PARK (56142, triggered) (boss_malygos.cpp:1032-1040,
-- event 14 — the arcane-overload SpellHit precedent). The DATA_PHASE
-- == PHASE_ONE gate has no Lua phase model; the twin is Malygos
-- in-combat + above 50% health (the vortexTick latch in
-- boss_malygos.lua): out of combat before the encounter starts
-- (the portal-beam path casts 61236 in PHASE_NOT_STARTED and must
-- NOT summon), in combat above 50% in phase one, at or below 50%
-- from phase two on. Malygos resolves via the instanceData bind
-- (instance_eye_of_eternity.lua). PORTED 2026-11-08 (tick 432).
-- DORMANT: the ev14 hit tail in the creature-cast path is gated on
-- the spell being known (spellKnown, castCreatureSpellInternalWith-
-- BasePoints, boss_ai.go:3805 — Spell.dbc lookup); no Spell.dbc
-- exists on this VM, so the handler never fires until Spell.dbc
-- ships. The PowerSparksHandling cast that feeds it lands today
-- (the cast itself needs no DBC row); only this response waits.
-- Unmodeled (documented-only, no bridges):
-- UpdateAI per-tick aura maintenance (boss_malygos.cpp:1042-1052):
-- no per-tick UpdateAI bridge — the SPELL_PORTAL_VISUAL_CLOSED
-- (55949) re-cast when neither visual aura is present + the
-- non-phase-one SPELL_PORTAL_OPENED removal stay out.

local NPC_PORTAL_TRIGGER = 30118 -- eye_of_eternity.h:51
local SPELL_PORTAL_OPENED = 61236 -- eye_of_eternity.h:84
local SPELL_SUMMON_POWER_PARK = 56142 -- boss_malygos.cpp:108

local function portalSpellHit(event, creature, caster, spellid)
    if spellid ~= SPELL_PORTAL_OPENED then
        return
    end
    local mapObj = creature:GetMap()
    if not mapObj then
        return
    end
    local instanceData = mapObj:GetInstanceData()
    if not instanceData then
        return
    end
    local malygos = instanceData.malygos
    if not malygos then
        return
    end
    if not malygos:IsInCombat() or malygos:GetHealthPct() <= 50 then
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_POWER_PARK, true)
end

RegisterCreatureEvent(NPC_PORTAL_TRIGGER, 14, portalSpellHit)
