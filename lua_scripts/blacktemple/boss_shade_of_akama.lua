-- Shade of Akama (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_shade_of_akama.cpp
-- (boss_shade_of_akama only; npc_akama_shade, npc_ashtongue_
-- channeler, npc_creature_generator_akama, npc_ashtongue_sorcerer,
-- npc_ashtongue_defender, npc_ashtongue_rogue, npc_ashtongue_
-- elementalist, npc_ashtongue_spiritbinder and npc_ashtongue_broken
-- documented below, not registered); black_temple.h:33 (DATA_SHADE_
-- OF_AKAMA = 2, third boss), :42 (DATA_AKAMA_SHADE = 9), :73 (NPC_
-- SHADE_OF_AKAMA = 22841). Creature entry: 22841 Shade of Akama
-- (C++ ScriptName "boss_shade_of_akama" per
-- AddSC_boss_shade_of_akama). No Talk lines in the boss AI.
-- Eluna creature events: 2 OnLeaveCombat, 4 OnDied, 14 OnHitBySpell
-- (SpellHit), 23 OnReset. Melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnDied: non-
-- triggered self-cast Shade of Akama trigger 40955 (C++ DoCast
-- default is triggered=false — vaelastrasz convention). OnHitBy-
-- Spell(14): hit by 40902 (Akama soul retrieve) -> non-triggered
-- self-cast soul expel channel 40927. OnLeaveCombat(2)/OnReset(23):
-- the C++ Reset arms (immune/non-selectable/stun-emote/walk flags,
-- EVENT_INITIALIZE_SPAWNERS, SummonCreatureGroup) and the evade-
-- side despawn arms have no bridges — nothing modeled.
-- The companion AIs in the same C++ file are not registered — the
-- C++ ScriptNames bind DB-side in creature_template (no TDB in this
-- workspace), so the entries cannot be verified against the bindings
-- from the C++ sources alone (garr firesworn convention; the NPC_
-- ASHTONGUE_CHANNELER=23421 / NPC_ASHTONGUE_BROKEN=23319 /
-- NPC_CREATURE_SPAWNER_AKAMA=23210 header constants do not change
-- this — supremus-volcano convention): npc_akama_shade (gossip
-- intro, soul-channel sequence, chain lightning 39945 / destructive
-- poison 40874 / soul retrieve 40902, broken-free outro sequence),
-- npc_ashtongue_channeler (soul channel 40401 keepalive on the
-- shade while it carries UNIT_FLAG_NOT_SELECTABLE), npc_creature_
-- generator_akama (wave-B 42035 and sorcerer/defender summon
-- scheduler, left/right sides), npc_ashtongue_sorcerer (shade
-- channeler -> Akama attacker switch), npc_ashtongue_defender
-- (debilitating strike 41178 / heroic strike 41975 / shield bash
-- 41180 / windfury 38229), npc_ashtongue_rogue (debilitating poison
-- 41978 / eviscerate 41177), npc_ashtongue_elementalist (rain of
-- fire 42023 / lightning bolt 42024), npc_ashtongue_spiritbinder
-- (spirit heal 42317 / spirit mend 42025 / chain heal 42027) and
-- npc_ashtongue_broken (outro walk/emote/hail/faction switch).
-- The spell_shade_soul_channel_serverside and spell_shade_soul_
-- channel AuraScripts have no AuraScript bridge (standing AuraScript
-- gap).
-- Deviations from C++: the UpdateAI UNIT_STATE_CASTING queue gate
-- has no UNIT_STATE bridge — arms fire unconditionally (jeklik
-- convention); no instance-script model — the DATA_SHADE_OF_AKAMA /
-- DATA_AKAMA_SHADE creature lookups, the SetBossState DONE arm, the
-- akama DoAction(ACTION_SHADE_OF_AKAMA_DEAD) arm and the spawner
-- DoAction(ACTION_START/STOP/DESPAWN_ALL_SPAWNS) arms have no
-- bridge; no MovementInform bridge — the chase-complete phase-two
-- gate and the EVENT_ADD_THREAT arm (threat 41602 every 3.5s) are
-- unmodeled; no flag/immune bridge — the SetImmuneToPC / UNIT_FLAG_
-- NOT_SELECTABLE phase-one arms, the EMOTE_STATE stun/none arms,
-- SetWalk, faction and emotestate arms have no bridge; no summon
-- model — the channeler spawns and summons.DespawnEntry(NPC_
-- ASHTONGUE_CHANNELER) have no bridge; the EVENT_INITIALIZE_SPAWNERS
-- grid-creature-list arm has no bridge; the EVENT_EVADE_CHECK arm
-- (EnterEvadeModeIfNeeded / IsInBoundary) has no evade or boundary
-- bridge; the soul-channel SpellHit(40447) arms beyond the event
-- scheduling (emote state, AttackStart on Akama) have no bridge.

local ENTRY_SHADE_OF_AKAMA = 22841

local SPELL_SHADE_OF_AKAMA_TRIGGER = 40955
local SPELL_AKAMA_SOUL_RETRIEVE = 40902
local SPELL_AKAMA_SOUL_EXPEL_CHANNEL = 40927

-- C++ JustDied: non-triggered self-cast of the shade trigger (the
-- akama DoAction, spawner DoActions, channeler despawn and
-- SetBossState DONE arms have no instance/summon bridges).
local function shadeDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_SHADE_OF_AKAMA_TRIGGER)
end

-- C++ SpellHit: hit by the soul-retrieve spell -> non-triggered
-- self-cast of the expel channel. The soul-channel (40447) SpellHit
-- arms are entirely instance/flag/evade-driven and unmodeled (see
-- deviations) — the hit itself is ignored here.
local function shadeSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_AKAMA_SOUL_RETRIEVE then
        creature:CastSpell(creature, SPELL_AKAMA_SOUL_EXPEL_CHANNEL)
    end
end

-- C++ Reset/EnterEvadeMode arms have no bridges — nothing modeled.
local function shadeLeaveCombat(event, creature)
end

local function shadeReset(event, creature)
end

RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 2, shadeLeaveCombat)
RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 4, shadeDied)
RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 14, shadeSpellHit)
RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 23, shadeReset)
