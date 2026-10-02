-- General Vezax (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_general_vezax.cpp
-- (boss_general_vezax (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_general_vezaxAI> (BossAI),
-- BOSS_VEZAX = 11 — registered from inside
-- AddSC_boss_general_vezax(); loader decl 117 / call 310 per
-- northrend_script_loader.cpp — the SIXTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_xt002() (decl 114 / call 309); the call
-- after it is AddSC_boss_assembly_of_iron() — loader order
-- confirmed this run; the checkpoint sequence
-- (boss_xt002 -> boss_general_vezax) is followed).
-- Entry: 33271 General Vezax (ulduar.h NPC_VEZAX, line 81 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- nine script names ("boss_general_vezax",
-- "boss_saronite_animus", "npc_saronite_vapors",
-- "spell_general_vezax_mark_of_the_faceless",
-- "spell_general_vezax_mark_of_the_faceless_leech",
-- "spell_general_vezax_saronite_vapors", "achievement_shadowdodger",
-- "achievement_smell_saronite") hits boss_general_vezax.cpp
-- (+ the loader decl/call lines for AddSC_boss_general_vezax)
-- only; zero sql/ hits for all nine. No vezax lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith passthrough, the DoCast
-- SPELL_AURA_OF_DESPAIR 62692 leg, the CheckShamanisticRage
-- (player-shaman SPELL_SHAMANTIC_RAGE 30823 ->
-- SPELL_CORRUPTED_RAGE 68415) leg, and the six ScheduleEvent
-- legs have no bridges — the auriaya engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — the twenty-fifth
-- player-gated variant ported).
-- JustDied — Talk(SAY_DEATH 3) (event 4; the _JustDied()
-- passthrough + DoRemoveAurasDueToSpellOnPlayers
-- SPELL_AURA_OF_DESPAIR leg have no bridges — the sjonnir
-- JustDied-Talk precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33271):
-- Reset / Initialize — _Reset() + shadowDodger /
-- smellSaronite / animusDead / vaporCount init — no bridges.
-- SpellHitTarget — SPELL_SHADOW_CRASH_HIT 62659
-- TYPEID_PLAYER leg -> shadowDodger = false (consumer:
-- achievement_shadowdodger below) — no SpellHit bridge.
-- GetData(DATA_SHADOWDODGER 29962997 / DATA_SMELL_SARONITE
-- 31813188 — no GetData bridge; consumers: the two
-- achievements below); DoAction(ACTION_VAPORS_DIE 1 /
-- ACTION_ANIMUS_DIE 2 — no DoAction bridge; producers:
-- npc_saronite_vapors / boss_saronite_animus below).
-- CheckShamanisticRage — HasSpell(SPELL_SHAMANTIC_RAGE)
-- player-scan -> SPELL_CORRUPTED_RAGE — no map-player-scan
-- bridge.
-- UpdateAI event machine — EVENT_SURGE_OF_DARKNESS
-- (Talk(EMOTE_SURGE_OF_DARKNESS 8) + Talk(SAY_SURGE_OF_DARKNESS
-- 2) + DoCast 62662); EVENT_SARONITE_VAPORS (DoCast 63081 +
-- vaporCount == 6 hard-mode leg -> Talk(SAY_HARDMODE 5) +
-- Talk(EMOTE_BARRIER 7) + summons.DespawnAll + DoCast 63364 /
-- 63145 + LOOT_MODE_HARD_MODE_1); EVENT_BERSERK
-- (Talk(SAY_BERSERK 4) + DoCast 26662) — no timer-event / cast
-- / summon / loot-mode bridges; all timer-leg yells ride the
-- unbridgeable event machine.
-- boss_saronite_animus (no creature-entry enum in the C++
-- tree — spawned via SPELL_SUMMON_SARONITE_ANIMUS 63145;
-- entry-unverifiable — no registration): Reset DoCast
-- SPELL_VISUAL_SARONITE_ANIMUS 63319 + EVENT_PROFOUND_OF_
-- DARKNESS 3s -> DoCastAOE 63420 — no cast / timer bridges;
-- JustDied -> vezax DoAction(ACTION_ANIMUS_DIE) — no DoAction
-- bridge; no Talk arms anywhere.
-- npc_saronite_vapors (no creature-entry enum in the C++ tree
-- — spawned via SPELL_SUMMON_SARONITE_VAPORS 63081;
-- entry-unverifiable — no registration): Talk(EMOTE_VAPORS 0)
-- in the constructor — no spawn / constructor bridge; Reset
-- EVENT_RANDOM_MOVE 5s-7.5s -> MoveRandom(30.0f) — no
-- scheduler / movement bridges; DamageTaken (lethal ->
-- damage = 0 + flags + root + stand-state-dead + DoCast
-- SPELL_SARONITE_VAPORS 63323 + DespawnOrUnsummon(30s) +
-- vezax DoAction(ACTION_VAPORS_DIE)) — no damage-taken /
-- DoAction bridges.
-- spell_general_vezax_mark_of_the_faceless (63276 — periodic
-- dummy AuraScript: caster casts
-- SPELL_MARK_OF_THE_FACELESS_DAMAGE 63278 with bp1 =
-- aurEff amount — no AuraScript bridge; joins the
-- no-AuraScript-bridge queue);
-- spell_general_vezax_mark_of_the_faceless_leech (63276/63134
-- target filter removing expl target, FinishCast on empty —
-- no SpellScript bridge; joins the no-SpellScript-bridge
-- queue); spell_general_vezax_saronite_vapors (63323 —
-- OnApply: bp * 2^stack mana restore -> SPELL_SARONITE_VAPORS_
-- ENERGIZE 63337 + DAMAGE 63338 at 2x — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue).
-- achievement_shadowdodger (OnCheck: GetData
-- (DATA_SHADOWDODGER)); achievement_smell_saronite (OnCheck:
-- GetData(DATA_SMELL_SARONITE)) — no GetData / achievement
-- bridges; both join the unmodeled-achievement queue.

local ENTRY_GENERAL_VEZAX = 33271

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 3

-- C++ JustEngagedWith: BossAI passthrough + DoCast
-- SPELL_AURA_OF_DESPAIR + CheckShamanisticRage + ScheduleEvent
-- legs — only the Talk arm is bridgeable.
local function vezaxEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function vezaxTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough +
-- DoRemoveAurasDueToSpellOnPlayers(62692) + Talk(SAY_DEATH) —
-- only the Talk arm is bridgeable.
local function vezaxDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 1, vezaxEnterCombat)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 3, vezaxTargetDied)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 4, vezaxDied)
