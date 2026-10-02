-- City/faction guards — Lua port of src/server/scripts/World/npc_guard.cpp
-- (242 lines incl. license; 2 CreatureScripts — npc_guard_generic
-- (GuardAI) + npc_guard_shattrath_faction (GuardAI); all registered
-- from inside AddSC_npc_guard() (line 237); loader decl 26 / call 47
-- per src/server/scripts/World/world_script_loader.cpp — the group
-- immediately after AddSC_go_scripts() (call 46) in AddWorldScripts()
-- — verified from the loader this run; the checkpoint sequence
-- (go_scripts -> npc_guard) is followed).
-- Sole-source verified: whole-server-tree grep for "AddSC_npc_guard"
-- hits npc_guard.cpp (+ the loader decl/call lines) only. No npc_guard
-- lua existed. Eluna creature events: 1 OnEnterCombat.
-- Entries: NPC_CENARION_HOLD_INFANTRY 15184 from the file's own
-- GuardMisc enum (the only entry with a bridgeable Talk arm);
-- NPC_STORMWIND_CITY_GUARD 68 / NPC_STORMWIND_CITY_PATROLLER 1976 /
-- NPC_ORGRIMMAR_GRUNT 3296 use npc_guard_generic's Talk-free legs;
-- NPC_ALDOR_VINDICATOR 18549 drives the Shattrath banish machine.
-- The ScriptName bindings are DB-side as usual (only entry 15184 is
-- registered below; the other entries' npc_guard_generic legs have no
-- bridgeable arms).
-- All Talk() calls in the file accounted for (1 ported below).
-- PORTED:
-- npc_guard_generic::JustEngagedWith — Talk(SAY_GUARD_SIL_AGGRO 0,
-- who) gated on me->GetEntry() == NPC_CENARION_HOLD_INFANTRY — the
-- event-1 auriaya precedent; the player target flattens through the
-- moroes precedent (Lua Talk takes no target); the _combatScheduler
-- melee/spell legs have no-timer / no-cast bridges. Registered via
-- RegisterLuaBoss("npc_guard_generic", 15184).
-- DOCUMENTED-ONLY:
-- npc_guard_generic::Reset — the 10-minute _scheduler buff machine
-- (SelectSpell SELECT_TARGET_ANY_FRIEND / SELECT_EFFECT_AURA self
-- buff + context.Repeat(Minutes(10))) — no-timer / no-cast bridges.
-- npc_guard_generic::ReceiveEmote / DoReplyToTextEmote — the
-- Stormwind 68/1976 + Orgrimmar 3296 emote reply machine
-- (TEXT_EMOTE_KISS/WAVE/SALUTE/SHY/RUDE/CHICKEN -> EMOTE_ONESHOT_*
-- HandleEmoteCommand, IsFriendlyTo-gated) — no-emote bridge.
-- npc_guard_generic::JustEngagedWith _combatScheduler — the 1s melee
-- machine (roll_chance_i(20) SELECT_TARGET_ANY_ENEMY DoCastVictim /
-- AttackerStateUpdate) + the 5s spell machine (HealthBelowPct(30)
-- 33%-chance SELECT_EFFECT_HEALING self-cast, else
-- SELECT_TARGET_ANY_ENEMY ranged DoCastVictim) — no-timer / no-cast
-- bridges.
-- npc_guard_generic::UpdateAI — _scheduler.Update / UpdateVictim /
-- _combatScheduler.Update legs — no bridges.
-- npc_guard_shattrath_faction (NPC_ALDOR_VINDICATOR 18549 +
-- other Shattrath vindicators) — the ScheduleVanish banish machine:
-- 5s victim-gated DoCast(SPELL_BANISHED_SHATTRATH_S 36671 /
-- SPELL_BANISHED_SHATTRATH_A 36642) + ObjectAccessor::GetUnit
-- player-GUID 9s chain (SPELL_EXILE 39533 + SPELL_BANISH_TELEPORT
-- 36643 self-casts) — zero Talk; no-timer / no-cast bridges — NOT
-- registered.

local ENTRY_CENARION_HOLD_INFANTRY = 15184 -- NPC_CENARION_HOLD_INFANTRY

local SAY_GUARD_SIL_AGGRO = 0

-- C++ npc_guard_generic::JustEngagedWith (entry-gated in C++ on
-- NPC_CENARION_HOLD_INFANTRY): Talk(SAY_GUARD_SIL_AGGRO, who) — the
-- event-1 auriaya precedent (the target flattens per the moroes
-- precedent; the GuardAI::_combatScheduler legs have no bridges).
local function guardGenericEnterCombat(event, creature, target)
    creature:Talk(SAY_GUARD_SIL_AGGRO)
end

RegisterCreatureEvent(ENTRY_CENARION_HOLD_INFANTRY, 1, guardGenericEnterCombat)
