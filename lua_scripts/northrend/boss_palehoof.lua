-- Gortok Palehoof (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_palehoof.cpp
-- (711 lines; 9 scripts — boss_palehoof (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_GORTOK_PALEHOOF = 1);
-- boss_ravenous_furbolg / boss_frenzied_worgen / boss_ferocious_rhino /
-- boss_massive_jormungar (CreatureScript via GetUtgardePinnacleAI
-- (PalehoofMinionsBossAI : BossAI), DATA_RAVENOUS_FURBOLG /
-- DATA_FRENZIED_WORGEN / DATA_FEROCIOUS_RHINO / DATA_MASSIVE_JORMUNGAR);
-- go_palehoof_sphere (GameObjectScript via GetUtgardePinnacleAI
-- (GameObjectAI), OnGossipHello ACTION_START_ENCOUNTER);
-- spell_palehoof_crazed (SpellScriptLoader, AuraScript OnEffectRemove ->
-- RemoveAurasDueToSpell 48147); spell_palehoof_crazed_effect
-- (SpellScriptLoader, SpellScript OnEffectHitTarget SCRIPT_EFFECT ->
-- CastSpell 48147); spell_palehoof_awaken_subboss (SpellScriptLoader,
-- SpellScript OnEffectHitTarget APPLY_AURA -> CastSpell 48048 + CombatStart
-- event); spell_palehoof_awaken_gortok (SpellScriptLoader, SpellScript
-- OnEffectHitTarget DUMMY -> CombatStart event); all registered from inside
-- AddSC_boss_palehoof(); loader decl 133 / call 328 per
-- northrend_script_loader.cpp — the SECOND group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_svala() (call 327); the checkpoint sequence
-- (boss_svala -> boss_palehoof) is followed).
-- Entries: 26687 Gortok Palehoof (utgarde_pinnacle.h NPC_GORTOK_PALEHOOF,
-- line 54) — entry-verifiable, registration proceeds (the nexus_commanders
-- / malygos / sartharion kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual. DATA_GORTOK_PALEHOOF = 1
-- (utgarde_pinnacle.h line 32).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_palehoof"
-- hits boss_palehoof.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No palehoof lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) — unconditional (the auriaya
-- engage-port precedent; BossAI::JustEngagedWith passthrough, the
-- EVENT_ARCING_SMASH / EVENT_IMPALE / EVENT_WITHERING_ROAR schedule legs
-- and the instance SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE) leg have no
-- bridges).
-- KilledUnit — Talk(SAY_SLAY 1) player-gated (event 3 — the razuvious
-- player-gated variant precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 26687):
-- JustDied — DoPlaySoundToSet(PALEHOOF_SOUND_DEATH 13467) with no Talk —
-- joins the no-sound / documented-only queue.
-- The whole phased pre-fight: go_palehoof_sphere OnGossipHello ->
-- DoAction(ACTION_START_ENCOUNTER) -> the Orb*PositionEvent / orb spell
-- machine -> SPELL_AWAKEN_SUBBOSS / SPELL_AWAKEN_GORTOK SpellScript
-- awakening (the four minibosses' SPELL_FREEZE / immune / DoZoneInCombat
-- legs) — no GameObjectScript / SpellScript / DoAction / event-machine
-- bridges; joins the respective bridge queues.
-- The four minibosses (Ravenous Furbolg, Frenzied Worgen, Ferocious Rhino,
-- Massive Jormungar) — pure spell-timer machines with zero Talk arms; no
-- registration. The PalehoofMinionsBossAI DoAction(ACTION_START_FIGHT)
-- freeze/immune/unfreeze legs, JustDied -> DoAction(ACTION_NEXT_PHASE),
-- EnterEvadeMode cross-creature cascade and the spell_palehoof_crazed /
-- spell_palehoof_crazed_effect SpellScriptLoader hooks — no bridges;
-- join the no-DoAction / no-SpellScript / no-AuraScript-bridge queues.

local ENTRY_GORTOK_PALEHOOF = 26687

local SAY_AGGRO = 0
local SAY_SLAY = 1

-- C++ boss_palehoofAI::JustEngagedWith: BossAI::JustEngagedWith(who)
-- (passthrough, no bridge); Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent).
local function palehoofEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: if (who->GetTypeId() == TYPEID_PLAYER) Talk(SAY_SLAY)
-- — the razuvious player-gated variant precedent.
local function palehoofKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 1, palehoofEnterCombat)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 3, palehoofKilledUnit)
