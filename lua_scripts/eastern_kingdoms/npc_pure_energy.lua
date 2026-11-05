-- Pure Energy (Magister's Terrace) --
-- Lua port of the npc_pure_energy CreatureScript in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/boss_vexallus.cpp
-- (AddSC_boss_vexallus registers boss_vexallus + npc_pure_energy).
-- Entry (Wowhead-verified 2026-11-05; no C++ constant exists):
-- 24745 — spell 44322 "Summon Pure Energy" (Effect #1: Summon (Pure
-- Energy), Used by NPC: Vexallus) links to npc=24745, whose page lists
-- Magisters' Terrace and Energy Feedback among its abilities; this
-- retires boss_vexallus.lua's "queued awaiting TDB evidence" note.
-- Verifiable numbers (file's own enum): SPELL_ENERGY_BOLT = 46156 /
-- SPELL_ENERGY_FEEDBACK = 44335 / SPELL_PURE_ENERGY_PASSIVE = 44326.
-- Eluna creature events: 4 OnDied, 5 OnSpawn.
-- Ported arms (C++ npc_pure_energyAI):
-- Constructor: SetDisplayId(Modelid2) — muru dark-fiend precedent:
-- event 5 + creature:SetDisplayId(creature:GetTemplateModelId(2)).
-- JustDied: killer->CastSpell(killer, SPELL_ENERGY_FEEDBACK, true) +
-- me->RemoveAurasDueToSpell(SPELL_PURE_ENERGY_PASSIVE). The ev4 killer
-- arg is the session player on the funneled path (fireCreatureDied)
-- or the killer creature motion on the server path
-- (fireCreatureDiedByCreature), nil when the attacker motion is gone —
-- so the cast keeps the C++ `if (killer)` nil-guard (anubarak scarab
-- precedent); both player and creature surfaces bridge CastSpell.
-- RemoveAura is the single-spell twin of RemoveAurasDueToSpell
-- (tick-218 triage; boss_illidan.lua precedent).
-- Not ported: SPELL_ENERGY_BOLT 46156 + EVENT_ENERGY_BOLT /
-- EVENT_ENERGY_FEEDBACK are enum-only in C++ (never scheduled).
local SPELL_ENERGY_FEEDBACK = 44335
local SPELL_PURE_ENERGY_PASSIVE = 44326

local ENTRY_PURE_ENERGY = 24745

local function onSpawn(_, creature)
    creature:SetDisplayId(creature:GetTemplateModelId(2))
end

local function onDied(_, creature, killer)
    if killer then
        killer:CastSpell(killer, SPELL_ENERGY_FEEDBACK, true)
    end
    creature:RemoveAura(SPELL_PURE_ENERGY_PASSIVE)
end

RegisterCreatureEvent(ENTRY_PURE_ENERGY, 5, onSpawn)
RegisterCreatureEvent(ENTRY_PURE_ENERGY, 4, onDied)
