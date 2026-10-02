-- Emerald Dragons (world bosses) — Lua port of
-- src/server/scripts/World/boss_emerald_dragons.cpp
-- (4 CreatureScripts: boss_ysondre / boss_lethon / boss_emeriss /
-- boss_taerar (WorldBossAI over the shared emerald_dragonAI base) +
-- npc_dream_fog (ScriptedAI, NPC_DREAM_FOG = 15224, zero Talk) +
-- npc_spirit_shade (PassiveAI, NPC_SPIRIT_SHADE = 15261, zero Talk);
-- 2 SpellScriptLoaders (spell_dream_fog_sleep, spell_mark_of_nature);
-- all registered from inside AddSC_emerald_dragons() (line 808);
-- loader decl 23 / call 44 per world_script_loader.cpp — the group
-- immediately after AddSC_areatrigger_scripts() (call 43) in
-- AddWorldScripts() — verified from the loader this run; the
-- checkpoint sequence (areatrigger_scripts -> emerald_dragons) is
-- followed).
-- Entries: 14887 Ysondre (Feralas) / 14888 Lethon (Duskwood) /
-- 14889 Emeriss (Hinterlands) / 14890 Taerar (Ashenvale) — all from
-- the file's own EmeraldDragonNPC enum; NPC_DREAM_FOG = 15224 from the
-- same enum; the ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_emerald_dragons" hits boss_emerald_dragons.cpp only (+ the
-- loader decl/call lines). No emerald-dragons lua existed.
-- Eluna creature events: 1 OnEnterCombat.
-- Ported arms (C++-exact for all modeled arms) — the four identical
-- JustEngagedWith arms (event 1 — the auriaya precedent; the
-- WorldBossAI::JustEngagedWith legs have no bridges):
-- boss_ysondre JustEngagedWith — Talk(SAY_YSONDRE_AGGRO 0)
-- boss_lethon JustEngagedWith — Talk(SAY_LETHON_AGGRO 0)
-- boss_emeriss JustEngagedWith — Talk(SAY_EMERISS_AGGRO 0)
-- boss_taerar JustEngagedWith — Talk(SAY_TAERAR_AGGRO 0)
-- DOCUMENTED-ONLY (in this header; only the four dragon entries
-- registered):
-- emerald_dragonAI Reset — DoCast(me, SPELL_MARK_OF_NATURE_AURA 25041,
-- triggered) non-targeted self-cast + RemoveFlag NOT_SELECTABLE /
-- NON_ATTACKABLE + SetReactState(REACT_AGGRESSIVE) + the EVENT_TAIL_SWEEP
-- / EVENT_NOXIOUS_BREATH / EVENT_SEEPING_FOG ScheduleEvent legs (the
-- self-cast rides the vaelastrasz self-cast convention but is not a
-- Talk arm; timers / flags / react-state have no bridges).
-- emerald_dragonAI UpdateAI — the SPELL_SUMMON_PLAYER 24776 max-threat
-- pull + DoMeleeAttackIfReady legs (no bridges); the base event machine
-- (EVENT_SEEPING_FOG 120-150s DoCast pair, EVENT_NOXIOUS_BREATH 7.5-15s,
-- EVENT_TAIL_SWEEP 2s) rides no-timer / no-cast bridges.
-- emerald_dragonAI KilledUnit — who->CastSpell(who, SPELL_MARK_OF_NATURE
-- 25040, triggered) gated on victim->GetTypeId() == TYPEID_PLAYER —
-- zero Talk, no player-cast bridge.
-- boss_emeriss KilledUnit — DoCast(who, SPELL_PUTRID_MUSHROOM 24904,
-- triggered) gated on player victim — zero Talk, no player-targeted
-- cast bridge (rides on the base KilledUnit leg above).
-- boss_ysondre DamageTaken — Talk(SAY_YSONDRE_SUMMON_DRUIDS 1) rides
-- the 25%-per-stage DamageTaken machine (10x triggered
-- SPELL_SUMMON_DRUID_SPIRITS 24795 casts) — no-DamageTaken bridge.
-- boss_lethon DamageTaken — Talk(SAY_LETHON_DRAW_SPIRIT 1) rides the
-- 25%-per-stage DamageTaken machine (DoCast SPELL_DRAW_SPIRIT 24811) —
-- no-DamageTaken bridge.
-- boss_lethon SpellHitTarget — the DRAW_SPIRIT hit-target summon
-- machine (NPC_SPIRIT_SHADE 15261, 50s) — no-SpellHitTarget bridge.
-- boss_emeriss DamageTaken — Talk(SAY_EMERISS_CAST_CORRUPTION 1) rides
-- the 25%-per-stage DamageTaken machine (triggered
-- SPELL_CORRUPTION_OF_EARTH 24910 cast) — no-DamageTaken bridge.
-- boss_taerar DamageTaken — Talk(SAY_TAERAR_SUMMON_SHADES 1) rides the
-- 25%-per-stage banish machine (3x triggered victim-cast shade summons
-- 24841/24842/24843 + DoCast SPELL_SHADE 24313 + NOT_SELECTABLE /
-- NON_ATTACKABLE flags + REACT_PASSIVE + 60s _banishedTimer) —
-- no-DamageTaken bridge; the SummonedCreatureDies _shades-- leg rides
-- no-summon-death bridge; the UpdateAI banish/timeout machine rides
-- no-timer / no-flag / no-react-state bridges.
-- The four dragon ExecuteEvent EVENT_LIGHTNING_WAVE (12s, then 10-20s) /
-- EVENT_SHADOW_BOLT_WHIRL (10s, then 15-30s) / EVENT_VOLATILE_INFECTION
-- (12s, then 120s) / EVENT_ARCANE_BLAST (12s, then 7-12s) /
-- EVENT_BELLOWING_ROAR (30s, then 20-30s) arms — no-timer / no-cast
-- bridges.
-- npc_dream_fog — zero Talk; the _roamTimer 15-30s chase / 2.5s
-- MoveRandom + MoveChase + SetWalk + walk-speed machine — no-movement /
-- no-timer bridges — NOT registered.
-- npc_spirit_shade — zero Talk; the IsSummonedBy MoveFollow +
-- MovementInform dark-offering cast + 1s DespawnOrUnsummon machine —
-- no-movement / no-cast / no-despawn bridges — NOT registered.
-- spell_dream_fog_sleep + spell_mark_of_nature (SpellScriptLoaders) —
-- no-SpellScript bridge — NOT registered.
-- All Talk() calls in the file accounted for (4 ported above +
-- SAY_YSONDRE_SUMMON_DRUIDS 1 + SAY_LETHON_DRAW_SPIRIT 1 +
-- SAY_EMERISS_CAST_CORRUPTION 1 + SAY_TAERAR_SUMMON_SHADES 1
-- documented above).

local ENTRY_YSONDRE = 14887
local ENTRY_LETHON  = 14888
local ENTRY_EMERISS = 14889
local ENTRY_TAERAR  = 14890

local SAY_AGGRO = 0 -- SAY_YSONDRE_AGGRO / SAY_LETHON_AGGRO /
                    -- SAY_EMERISS_AGGRO / SAY_TAERAR_AGGRO (all 0)

-- C++ boss_ysondre/boss_lethon/boss_emeriss/boss_taerar::JustEngagedWith:
-- Talk(SAY_*_AGGRO); WorldBossAI::JustEngagedWith(who) — the auriaya
-- precedent (the WorldBossAI legs have no bridges).
local function dragonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

RegisterCreatureEvent(ENTRY_YSONDRE, 1, dragonEnterCombat)
RegisterCreatureEvent(ENTRY_LETHON,  1, dragonEnterCombat)
RegisterCreatureEvent(ENTRY_EMERISS, 1, dragonEnterCombat)
RegisterCreatureEvent(ENTRY_TAERAR,  1, dragonEnterCombat)
