-- npc_arcane_overload (Eye of Eternity) — Lua port of
-- src/server/scripts/Northrend/Nexus/EyeOfEternity/boss_malygos.cpp
-- (npc_arcane_overload (CreatureScript) via
-- GetEyeOfEternityAI<npc_arcane_overloadAI>, registered from inside
-- AddSC_boss_malygos()).
-- Entry: 30282 NPC_ARCANE_OVERLOAD (eye_of_eternity.h :55; the C++
-- enum comment :126 confirms "cast by npc Arcane Overload ID:
-- 30282" for SPELL_ARCANE_OVERLOAD_1).
-- Eluna creature events: 14 OnHitBySpell — args
-- (event, creature, caster, spellid) (boss_ai.go target-side
-- SpellHit leg; muru-portal in-tree precedent). The boolean veto
-- of the Eluna hook is a no-op vs C++ (the hook's return does not
-- gate the base), so the arm is purely the cast pair.
-- Ported arms (C++-exact):
-- SpellHit SPELL_ARCANE_BOMB_TRIGGER (56430): C++ :1478-:1484 =
-- DoCastAOE(SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE 56431, triggered) +
-- DoCastSelf(SPELL_ARCANE_OVERLOAD_1 56432, triggered), in C++
-- order, both with the exact triggered flag. DoCastAOE ≡ self-cast
-- creature:CastSpell(creature, spell) (established in-tree
-- convention; selin_fireheart.lua / mr_smite.lua precedents), so
-- the AOE knockback is cast on self the same way the C++ AOEs it
-- from the caster position.
-- Caveat: Go fires event 14 once per non-NONE spell effect per
-- creature target (boss_ai.go Spell::TargetInfo::DoTargetSpellHit
-- leg), while C++ SpellHit fires once per spell hit — a
-- multi-effect trigger spell could double-fire the arm. 56430's
-- effect layout is single-arm; the deviation is theoretical for
-- this entry, documented not widened.
-- Unmodeled (no bridges — documented, not wired):
-- IsSummonedBy: instance->SetGUID(DATA_LAST_OVERLOAD_GUID) — no
-- instance bridge. DoAction: phase-gated DespawnOrUnsummon
-- (PHASE_TWO -> 6s despawn; phase 0 -> immediate; the phase-machine
-- GetData DATA_PHASE gate rides with it) — no phase / despawn
-- bridges. Constructor _instance NULL-check leg is C++-only.

local ENTRY_ARCANE_OVERLOAD = 30282

local SPELL_ARCANE_BOMB_TRIGGER = 56430
local SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE = 56431
local SPELL_ARCANE_OVERLOAD_1 = 56432

-- C++ npc_arcane_overloadAI::SpellHit (boss_malygos.cpp :1478-:1484):
-- if (spellInfo->Id == SPELL_ARCANE_BOMB_TRIGGER) {
--     DoCastAOE(SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE, true);
--     DoCastSelf(SPELL_ARCANE_OVERLOAD_1, true); }
local function overloadHitBySpell(event, creature, caster, spellid)
    if spellid == SPELL_ARCANE_BOMB_TRIGGER then
        creature:CastSpell(creature, SPELL_ARCANE_BOMB_KNOCKBACK_DAMAGE, true)
        creature:CastSpell(creature, SPELL_ARCANE_OVERLOAD_1, true)
    end
end

RegisterCreatureEvent(ENTRY_ARCANE_OVERLOAD, 14, overloadHitBySpell)
