-- Novos the Summoner (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_novos.cpp
-- (boss_novos (BossAI), npc_crystal_channel_target (ScriptedAI),
-- spell_novos_summon_minions (SpellScript 59910),
-- achievement_oh_novos (AchievementCriteriaScript);
-- AddSC_boss_novos at end registers all via GetDrakTharonKeepAI /
-- new; npc_crystal_channel_target gets no instance shim — it is
-- GetDrakTharonKeepAI'd like the boss). The second Drak'Tharon Keep
-- group in northrend_script_loader.cpp order (decl 40 / call 235,
-- immediately after AddSC_boss_trollgore(); next:
-- AddSC_boss_king_dred()). Entry: 26631 Novos the Summoner
-- (drak_tharon_keep.h NPC_NOVOS — kalecgos pass; the
-- GetDrakTharonKeepAI ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_novos",
-- "npc_crystal_channel_target", "spell_novos_summon_minions" and
-- "achievement_oh_novos" hits boss_novos.cpp only (the loader carries
-- only the AddSC_boss_novos decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers are scheduled: every C++ scheduled arm rides an absent
-- bridge (see below), so the cancel/schedule machinery has nothing to
-- drive. Melee is engine-driven in Go (creature combat tick), like
-- C++ DoMeleeAttackIfReady. BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.
-- Ported arms (C++-exact for the modeled arms — the three wired
-- hooks fire unconditionally in C++ and are independent of the
-- unmodeled bubbled-phase machinery, which gates UpdateAI entirely
-- via a latch with no bridge):
-- boss_novos: JustEngagedWith Talk(SAY_AGGRO 0) (event 1 — the hook
-- itself is bridged; its other legs — SetCrystalsStatus(true) /
-- SetSummonerStatus(true) / SetBubbled(true) (instance GetGuidData +
-- ObjectAccessor cross-AI + flag/GO-state bridges absent; instance
-- model absent — standing) — documented-only below);
-- KilledUnit Talk(SAY_KILL 1) player-gated (event 3,
-- who->GetTypeId() == TYPEID_PLAYER — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- Unmodeled (no bridges — documented, not wired):
-- AttackStart — DoStartNoMovement melee-without-movement model —
-- no movement bridge; Go melee stays engine-driven.
-- Reset's SetCrystalsStatus(false) / SetSummonerStatus(false) /
-- SetBubbled(false) legs — instance GetGuidData + ObjectAccessor
-- cross-AI bridges absent; instance model absent (standing).
-- SetBubbled(true)'s DoCast(SPELL_ARCANE_FIELD 47346) + UNIT_FLAG_
-- NON_ATTACKABLE flag set — flag bridge absent (drakkari_colossus
-- precedent); the _bubbled latch gates UpdateAI entirely while set
-- and clears only via the unbridged DoAction machine, so no latch
-- is emulated (jedoga precedent — an ungated bubbled model would
-- diverge from C++ combat behavior).
-- UpdateAI: the !_bubbled gate (see above) + HasUnitState(
-- UNIT_STATE_CASTING) skip — no unit-state bridge.
-- EVENT_ATTACK (3s repeat, scheduled only after the crystal-handler
-- DoAction machine clears the bubble) — SelectTarget(Random) ->
-- DoCast(victim, RAND(SPELL_ARCANE_BLAST 49198 / SPELL_BLIZZARD
-- 49034 / SPELL_FROSTBOLT 49037 / SPELL_WRATH_OF_MISERY 50089)) —
-- random-target SelectTarget bridge absent (cairne/kazzak precedent).
-- EVENT_SUMMON_MINIONS (15s repeat, heroic-only, likewise
-- DoAction-gated) — DoCast(SPELL_SUMMON_MINIONS 59910) — would
-- over-cast ungated (jedoga precedent) — documented-only.
-- The DoAction machine — DoAction(ACTION_CRYSTAL_HANDLER_DIED)
-- (drak_tharon_keep.h:52) + CrystalHandlerDied: per-crystal
-- SetCrystalStatus(crystal, false) deactivation (instance GetGuidData
-- + ObjectAccessor + GO-state bridges absent), the
-- _crystalHandlerCount latch -> Talk(SAY_ARCANE_FIELD 4) (player-less
-- Talk — bridged in isolation, but gated by the DoAction),
-- SetSummonerStatus(false), SetBubbled(false), the EVENT_ATTACK /
-- EVENT_SUMMON_MINIONS schedules, and the per-handler-death
-- AI()->SetData(SPELL_SUMMON_CRYSTAL_HANDLER, 15000) re-arm on the
-- DATA_NOVOS_SUMMONER_4 target — no DoAction bridge
-- (drakkari_colossus precedent) + cross-AI SetData absent — whole
-- machine documented-only.
-- MoveInLineOfSight — the _ohNovos latch (Initialize/Reset true;
-- flips false when a HULKING_CORPSE 27597 / RISEN_SHADOWCASTER 27600
-- / FETID_TROLL_CORPSE 27598 crosses y > -771.95f) — no grid/
-- MoveInLineOfSight bridge — documented-only.
-- GetData(DATA_NOVOS_ACHIEV) (type == DATA_NOVOS_ACHIEV && _ohNovos
-- ? 1 : 0) — no cross-AI GetData bridge; its only caller is
-- achievement_oh_novos — documented-only.
-- JustSummoned — summons.Summon — summon STRAND absent (standing) —
-- documented-only.
-- npc_crystal_channel_target — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (26712 — drak_tharon_keep.h NPC_CRYSTAL_CHANNEL_TARGET, kalecgos
-- pass): zero registration. The whole AI is the cross-AI timer
-- machine: novos-side SetSummonerStatus / CrystalHandlerDied drive
-- it via AI()->SetData(spell, timer), then UpdateAI DoCast(_spell)
-- on expiry; JustSummoned forwards to novos->AI()->JustSummoned and
-- runs MovePath(summon->GetEntry() * 100) (motion-master bridge
-- absent). With the cross-AI SetData activation bridge absent it can
-- never fire — documented-only. Joins the
-- entry-verifiable-but-bridge-blocked queue.
-- spell_novos_summon_minions (59910 SpellScript:
-- OnEffectHitTarget EFFECT_0 SCRIPT_EFFECT -> caster double-cast
-- SPELL_SUMMON_COPY_OF_MINIONS 59933, triggered) — no SpellScript
-- binding bridge on the Lua surface (razelikh precedent) —
-- documented-only.
-- achievement_oh_novos — AchievementCriteriaScript (target
-- ToCreature -> AI()->GetData(DATA_NOVOS_ACHIEV)) — no
-- achievement-criteria bridge (snakes precedent) + cross-AI GetData
-- bridge absent — documented-only.
-- NOTE: SAY_SUMMONING_ADDS (3) / EMOTE_SUMMONING_ADDS (5) are
-- marked unused in the Yells enum and never fired by any code in the
-- file — nothing to port.

local ENTRY_NOVOS = 26631

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2

local function novosEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

local function novosTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function novosDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_NOVOS, 1, novosEnterCombat)
RegisterCreatureEvent(ENTRY_NOVOS, 3, novosTargetDied)
RegisterCreatureEvent(ENTRY_NOVOS, 4, novosDied)
