-- Yogg-Saron (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_yogg_saron.cpp
-- (boss_voice_of_yogg_saron (CreatureScript via
-- GetUlduarAI<boss_voice_of_yogg_saronAI> (BossAI),
-- BOSS_YOGG_SARON = 12); boss_sara (CreatureScript via
-- GetUlduarAI<boss_saraAI> (ScriptedAI)); boss_yogg_saron
-- (CreatureScript via GetUlduarAI<boss_yogg_saronAI>
-- (PassiveAI)); boss_brain_of_yogg_saron (CreatureScript via
-- GetUlduarAI<boss_brain_of_yogg_saronAI> (PassiveAI));
-- fourteen npc_* scripts (npc_ominous_cloud /
-- npc_guardian_of_yogg_saron / npc_corruptor_tentacle /
-- npc_constrictor_tentacle / npc_crusher_tentacle /
-- npc_influence_tentacle / npc_descend_into_madness /
-- npc_immortal_guardian / npc_observation_ring_keeper /
-- npc_yogg_saron_keeper / npc_yogg_saron_illusions /
-- npc_garona / npc_turned_champion / npc_laughing_skull);
-- thirty-three spell_* scripts (all SpellScriptLoader, no
-- quoted names); all registered from inside
-- AddSC_boss_yogg_saron(); loader decl 122 / call 317 per
-- northrend_script_loader.cpp — the THIRTEENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_thorim() (decl 121 / call 316); the
-- checkpoint sequence (boss_thorim -> boss_yogg_saron) is
-- followed).
-- Entries: 33134 Sara (ulduar.h NPC_SARA, line 197);
-- 33288 Yogg-Saron (ulduar.h NPC_YOGG_SARON, line 82) —
-- both entry-verifiable, registrations proceed (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName bindings are
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the eighteen quoted creature script names hits
-- boss_yogg_saron.cpp only (+ the loader decl/call lines
-- for AddSC_boss_yogg_saron); this clone carries no sql/
-- tree, so the ScriptName bindings are DB-side by
-- construction.
-- No yogg lua existed.
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Sara Talk(SAY_SARA_AGGRO 2) (event 1;
-- the three ScheduleEvent legs (EVENT_SARAS_FERVOR /
-- EVENT_SARAS_BLESSING / EVENT_SARAS_ANGER) have no
-- bridges — the auriaya engage-port precedent; Sara's AI is
-- ScriptedAI, but the engage port lives on the Eluna
-- event-1 hook, not on the AI base).
-- JustDied — Yogg-Saron Talk(SAY_YOGG_SARON_DEATH 6)
-- (event 4; the cross-creature legs — Unit::Kill(voice),
-- sara/brain DisappearAndDie, keeper EnterEvadeMode — and
-- the player SPELL_SANITY / SPELL_INSANE aura removal have
-- no bridges — the sjonnir JustDied-Talk precedent).
-- DOCUMENTED-ONLY (in this header; no registrations beyond
-- entries 33134 and 33288):
-- Sara KilledUnit — player-gated Talk(SAY_SARA_KILL 5)
-- (victim->GetTypeId() == TYPEID_PLAYER) AND
-- !me->IsInEvadeMode(); the player gate is bridgeable (the
-- razuvious precedent) but the evade gate is not — no
-- IsInEvadeMode in the Lua creature surface — so event 3
-- is NOT registered; a partial port would deviate from
-- C++ (yelling while evading), and C++-exactness wins.
-- Sara DamageTaken — lethal (damage >= health) is NOT a
-- death rewrite: damage = health - 1 (survive with 1 hp)
-- and Talk(SAY_SARA_TRANSFORM_1) fires only in PHASE_ONE
-- alongside cross-creature DoAction(ACTION_PHASE_TRANSFORM)
-- — no phase / DoAction bridges; the event-9 bridge
-- cannot model it — bridge-blocked.
-- Sara SpellHitTarget — Talk(SAY_SARA_FERVOR_HIT /
-- SAY_SARA_BLESSING_HIT / SAY_SARA_PSYCHOSIS_HIT) gated on
-- roll_chance_i(30) and !PHASE_TRANSFORM — no SpellHit
-- bridge.
-- Sara JustSummoned — Talk(SAY_SARA_DEATH_RAY) on
-- NPC_DEATH_ORB summon — no JustSummoned bridge.
-- Sara timer Talks — Talk(SAY_SARA_TRANSFORM_2/3/4) on
-- EVENT_TRANSFORM_1/2/3 — no timer-event bridge; join the
-- no-timer-bridge queue.
-- Yogg-Saron timer Talks — Talk(SAY_YOGG_SARON_SPAWN) on
-- EVENT_YELL_BOW_DOWN; Talk(EMOTE_YOGG_SARON_EMPOWERING_SHADOWS)
-- on EVENT_SHADOW_BEACON; Talk(SAY_YOGG_SARON_DEAFENING_ROAR)
-- + Talk(EMOTE_YOGG_SARON_DEAFENING_ROAR) on
-- EVENT_DEAFENING_ROAR — no timer-event bridge; join the
-- no-timer-bridge queue.
-- Yogg-Saron DoAction(ACTION_PHASE_THREE) —
-- Talk(SAY_YOGG_SARON_PHASE_3) — no DoAction bridge; joins
-- the no-DoAction-bridge queue.
-- Yogg-Saron SpellHit — Val'anyr loot-mode leg (no Talk) —
-- no SpellHit bridge.
-- Voice of Yogg-Saron (33280, ulduar.h line 203) —
-- JustEngagedWith has no Talk arm; JustDied has no Talk
-- arm (summons.Despawn + _JustDied() only); EnterEvadeMode
-- Talk(WHISPER_VOICE_PHASE_1_WIPE, player) rides a
-- player-loop wipe leg (RemoveAurasDueToSpell SPELL_SANITY
-- / SPELL_INSANE) gated on PHASE_ONE — no wipe / aura /
-- phase bridges; timer Talks (cross-creature
-- EMOTE_YOGG_SARON_EXTINGUISH_ALL_LIFE on
-- EVENT_EXTINGUISH_ALL_LIFE; EMOTE_YOGG_SARON_MADNESS +
-- SAY_YOGG_SARON_MADNESS on EVENT_ILLUSION) ride the
-- unbridgeable voice event machine (the mimiron
-- cross-creature precedent) — no registration.
-- Brain of Yogg-Saron (33890, ulduar.h line 218) —
-- DamageTaken has no Talk arm (aura/flag/DoAction legs
-- only); DoAction ACTION_INDUCE_MADNESS /
-- ACTION_TENTACLE_KILLED have no Talk arms — no
-- registration.
-- npc_observation_ring_keeper — Talk(SAY_KEEPER_CHOSEN_1/2,
-- player) gated on OnGossipSelect menuId 10333 — no
-- gossip-select bridge; no registration (observation-ring
-- keeper entries 33213 / 33241 / 33242 / 33244 entry-verifiable,
-- ulduar.h lines 199-202).
-- npc_yogg_saron_keeper (33410 / 33411 / 33412 / 33413,
-- ulduar.h lines 205-208) — no Talk arms — no registration.
-- npc_yogg_saron_illusions / npc_garona /
-- npc_turned_champion / npc_laughing_skull — roleplay
-- Talks (SAY_CHAMBER_ROLEPLAY_1-5 / SAY_ICECROWN_ROLEPLAY_1-6 /
-- SAY_STORMWIND_ROLEPLAY_1-7) fire from IsSummonedBy /
-- the illusion UpdateAI timer machine on instance
-- DATA_ILLUSION — no bridges; no registrations.
-- Tentacle / cloud / guardian creatures (33136 / 33292 /
-- 33943 / 33966 / 33983 / 33985 / 33988 / 34072, ulduar.h
-- lines 198 / 204 / 219 / 221 / 222 / 223 / 224 / 226) —
-- no bridgeable Talk arms — no registrations.
-- The Talk arms inside SpellScripts / AuraScripts —
-- Talk(EMOTE_OMINOUS_CLOUD_PLAYER_TOUCH) in
-- spell_yogg_saron_boil_ominously (HandleDummy);
-- Talk(WHISPER_VOICE_INSANE) in spell_yogg_saron_insane
-- (OnApply) — no SpellScript / AuraScript bridges; all
-- thirty-three spell scripts join the no-SpellScript /
-- no-AuraScript-bridge queues.

local ENTRY_SARA = 33134
local ENTRY_YOGG_SARON = 33288

local SAY_SARA_AGGRO = 2
local SAY_YOGG_SARON_DEATH = 6

-- C++ JustEngagedWith (boss_saraAI, ScriptedAI):
-- Talk(SAY_SARA_AGGRO) + three ScheduleEvent legs — only
-- the Talk arm is bridgeable; the auriaya engage-port
-- precedent.
local function saraEnterCombat(event, creature, target)
    creature:Talk(SAY_SARA_AGGRO)
end

-- C++ JustDied (boss_yogg_saronAI, PassiveAI):
-- Talk(SAY_YOGG_SARON_DEATH), then cross-creature kill /
-- disappear / evade legs and player sanity-aura removal —
-- only the Talk arm is bridgeable; the sjonnir
-- JustDied-Talk precedent.
local function yoggSaronDied(event, creature, killer)
    creature:Talk(SAY_YOGG_SARON_DEATH)
end

RegisterCreatureEvent(ENTRY_SARA, 1, saraEnterCombat)
RegisterCreatureEvent(ENTRY_YOGG_SARON, 4, yoggSaronDied)
