-- Mana Feeder (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/karazhan.cpp
-- (npc_mana_feederAI). Creature entry 16491 (wowhead wotlk
-- npc=16491/mana-feeder; TDB creature_template ScriptName
-- "npc_mana_feeder" via AddSC_karazhan).
-- Eluna creature events: 23 OnReset (the C++ Reset arms run on the
-- reset hook). Melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Deviations from C++: the six ApplySpellImmune arms (Nature, Arcane,
-- Fire, Shadow, Frost, Holy) have no bridge — the Go engine has no
-- school-immunity model — so they are documented, not ported. The
-- arcane-protector arm of the same file is untouched by this script.

local ENTRY = 16491

local SPELL_MANA_BITE = 29908

local function onReset(event, creature)
    -- C++ Reset: DoCast(me, SPELL_MANA_BITE) is non-triggered
    -- (DoCast default triggered=false — vaelastrasz convention).
    creature:CastSpell(creature, SPELL_MANA_BITE)
end

RegisterCreatureEvent(ENTRY, 23, onReset)
