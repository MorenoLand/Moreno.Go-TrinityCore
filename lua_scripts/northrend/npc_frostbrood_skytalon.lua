-- Frostbrood Skytalon (Icecrown) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_frostbrood_skytalonAI — SpellHit arms only).
-- Entry: 31583 'Frostbrood Skytalon' (TDB-verified, ScriptName
-- npc_frostbrood_skytalon; summon-only, zero static spawns — the
-- 31137 SmartAI twin is the ambient one). The summoned entry is
-- confirmed by C++: Spells/spell_quest.cpp:1802, enum
-- Quest13291_13292_13239_13261Data: NPC_SKYTALON = 31583 (quests
-- 13291/13292/13239/13261 — Borrowed Technology / The Solution
-- Solution / Volatility).
-- C++ SpellHit (:763-775): 59335 SPELL_EXPLOSION -> DoCast(me,
-- 54690 SPELL_IMMOLATION); 59319 SPELL_RIDE -> DoCastAOE(59375
-- SPELL_PING_BUNNY) + events.ScheduleEvent(EVENT_FLY_AWAY, 100ms).
-- DoCast and DoCastAOE both port as self-cast
-- creature:CastSpell(creature, spell) (DoCastAOE self-cast
-- convention; both non-triggered, C++ default).
-- The EVENT_FLY_AWAY leg stays out: UpdateAI MovePoint to a
-- GetNearPoint2D random point — no GetNearPoint2D bridge (only
-- Go-internal references). Documented delta, not a blocker for
-- the two cast arms.
-- IsSummonedBy / MovementInform (SPELL_GRAB 59318) / VehicleAI
-- UpdateAI are out of scope; ev14 fires on the creature via the
-- spell-hit dispatch, independent of the VehicleAI base.
-- Eluna creature event: 14 OnHitBySpell (lecraft onSpellHit arg
-- order: event, creature, caster, spellId).

local FROSTBROOD_SKYTALON_ENTRY = 31583
local SPELL_EXPLOSION = 59335
local SPELL_IMMOLATION = 54690
local SPELL_RIDE = 59319
local SPELL_PING_BUNNY = 59375

local function onSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_EXPLOSION then
        creature:CastSpell(creature, SPELL_IMMOLATION)
    elseif spellId == SPELL_RIDE then
        creature:CastSpell(creature, SPELL_PING_BUNNY)
    end
end

RegisterCreatureEvent(FROSTBROOD_SKYTALON_ENTRY, 14, onSpellHit)
