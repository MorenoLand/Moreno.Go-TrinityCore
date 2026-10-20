-- Mana Feeder (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp
-- (npc_mana_feederAI). Creature entry 16491 (wowhead wotlk
-- npc=16491/mana-feeder; TDB creature_template ScriptName
-- "npc_mana_feeder" via AddSC_karazhan).
-- Eluna creature events: 23 OnReset (the C++ Reset arms run on the
-- reset hook). Melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Deviations from C++: none — the six ApplySpellImmune arms (Nature,
-- Arcane, Fire, Shadow, Frost, Holy) ride the creature:ApplySpellImmune
-- bridge (lua_creature_events.go). The arcane-protector arm of the same
-- file is untouched by this script.

local ENTRY = 16491

local SPELL_MANA_BITE = 29908

local function onReset(event, creature)
    -- C++ Reset: DoCast(me, SPELL_MANA_BITE) is non-triggered
    -- (DoCast default triggered=false — vaelastrasz convention).
    -- Reset-only (karazhan.cpp); alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_MANA_BITE)
    end
    -- C++ Reset :655-660: ApplySpellImmune(0, IMMUNITY_SCHOOL,
    -- SPELL_SCHOOL_MASK_*, true) x6 — nature/arcane/fire/shadow/frost/
    -- holy school immunities, in C++ order.
    creature:ApplySpellImmune(0, 2, 8, true)
    creature:ApplySpellImmune(0, 2, 64, true)
    creature:ApplySpellImmune(0, 2, 4, true)
    creature:ApplySpellImmune(0, 2, 32, true)
    creature:ApplySpellImmune(0, 2, 16, true)
    creature:ApplySpellImmune(0, 2, 2, true)
end

RegisterCreatureEvent(ENTRY, 23, onReset)
