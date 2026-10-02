-- Sindragosa (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/boss_sindragosa.cpp
-- (1556 lines incl. license; 5 CreatureScripts
-- (boss_sindragosa (BossAI, DATA_SINDRAGOSA = 11), npc_ice_tomb
-- (ScriptedAI), npc_spinestalker (ScriptedAI), npc_rimefang_icc
-- (ScriptedAI), npc_sindragosa_trash (ScriptedAI, shared AI for
-- NPC_FROSTWARDEN_HANDLER / NPC_FROSTWING_WHELP)) +
-- 13 Sindragosa-specific SpellScript/AuraScript legs
-- (spell_sindragosa_s_fury, spell_sindragosa_unchained_magic,
-- spell_sindragosa_frost_breath, spell_sindragosa_instability,
-- spell_sindragosa_frost_beacon, spell_sindragosa_ice_tomb_trap,
-- spell_sindragosa_icy_grip, spell_sindragosa_mystic_buffet,
-- spell_rimefang_icy_blast, spell_frostwarden_handler_order_whelp,
-- spell_frostwarden_handler_focus_fire,
-- spell_frostwarden_handler_focus_fire_aura (AuraScript),
-- spell_sindragosa_ice_tomb_target) + 2 generic
-- RegisterSpellScriptWithArgs spell_trigger_spell_from_caster legs
-- + 1 AreaTriggerScript
-- (at_sindragosa_lair) + 1 AchievementCriteriaScript
-- (achievement_all_you_can_eat); all registered from inside
-- AddSC_boss_sindragosa(); loader decl 182 / call 377 per
-- northrend_script_loader.cpp — the TWELFTH group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_valithria_dreamwalker() (call 376) — verified
-- from the loader this run; the checkpoint sequence
-- (valithria_dreamwalker -> sindragosa) is followed).
-- Entry: 36853 Sindragosa (icecrown_citadel.h NPC_SINDRAGOSA, line
-- 304; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_SINDRAGOSA, line 300; also summoned via instance
-- SummonCreature(NPC_SINDRAGOSA, SindragosaSpawnPos), line 492) —
-- entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the RegisterIcecrownCitadelCreatureAI
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_sindragosa" hits boss_sindragosa.cpp only (+ the
-- loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No sindragosa
-- lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_sindragosa JustEngagedWith — Talk(SAY_AGGRO 0) (event 1 —
-- the auriaya engage-port precedent; the instance
-- CheckRequiredBosses gate / EVADE_REASON_SEQUENCE_BREAK leg /
-- DoCastSpellOnPlayers LIGHT_S_HAMMER_TELEPORT leg / DoCastSelf
-- FROST_AURA+PERMAEATING_CHILL legs / SetBossState(IN_PROGRESS) /
-- SetCombatPulseDelay / setActive / SetFarVisible / DoZoneInCombat
-- legs have no bridges).
-- boss_sindragosa KilledUnit — Talk(SAY_KILL 8) gated on
-- victim->GetTypeId() == TYPEID_PLAYER (event 3 — the razuvious
-- player-gated variant precedent).
-- boss_sindragosa JustDied — Talk(SAY_DEATH 10) (event 4 — the
-- sjonnir JustDied-Talk precedent; the DoCastAOE
-- FROST_INFUSION_CREDIT heroic leg has no-cast bridge).
-- DOCUMENTED-ONLY (in this header; only entry 36853 registered):
-- boss_sindragosa UpdateAI scheduler Talk legs — Talk(EMOTE_BERSERK_RAID
-- 11) / Talk(SAY_BERSERK 9) (EVENT_BERSERK); Talk(SAY_UNCHAINED_MAGIC
-- 1) (EVENT_UNCHAINED_MAGIC); Talk(EMOTE_WARN_BLISTERING_COLD 2)
-- (EVENT_BLISTERING_COLD); Talk(SAY_BLISTERING_COLD 3)
-- (EVENT_BLISTERING_COLD_YELL); Talk(SAY_AIR_PHASE 5) (EVENT_AIR_PHASE);
-- Talk(SAY_PHASE_2 6) (EVENT_THIRD_PHASE_CHECK, third-phase check leg)
-- — all timer-driven (no-timer-bridge; the boss_toravon precedent);
-- the air-phase movement / gravity / react-state machine and the
-- DoCast cleave / tail-smash / frost-breath / icy-grip / ice-tomb /
-- frost-bomb legs have no bridges.
-- SAY_RESPITE_FOR_A_TORMENTED_SOUL 4 is declared in the text enum
-- but never referenced by a Talk() call — dead text evidence.
-- boss_sindragosa DoAction — ACTION_START_FROSTWYRM intro machine
-- (no-DoAction bridge); JustReachedHome SetBossState FAIL leg
-- (no instance bridge, no event-24 port precedent).
-- spell_sindragosa_ice_tomb_target::HandleSindragosaTalk —
-- creatureCaster->AI()->Talk(EMOTE_WARN_FROZEN_ORB 7, GetHitUnit())
-- is cross-AI Talk (no cross-AI-Talk bridge).
-- npc_ice_tomb (36980) / npc_spinestalker (37534) /
-- npc_rimefang_icc (37533) / npc_sindragosa_trash (37531
-- NPC_FROSTWARDEN_HANDLER, 37532 NPC_FROSTWING_WHELP) — zero Talk
-- lines — NOT registered (the bronjahm npc_corrupted_soul_fragment
-- precedent); ice tomb's SetGUID / DoAction
-- ACTION_TRIGGER_ASPHYXIATION / GUID-bookkeeping legs, spinestalker
-- / rimefang intro DoAction machines, and the trash Reset /
-- JustAppeared / SetData instance legs ride bridges that do not
-- exist.
-- at_sindragosa_lair joins the no-AreaTrigger bridge queue (the
-- boss_anubrekhan at_anubrekhan_entrance precedent);
-- achievement_all_you_can_eat joins the
-- no-achievement-criteria-bridge queue (the lana'thel achievement
-- precedent); the remaining 12 Sindragosa-specific SpellScript /
-- AuraScript legs + the 2 generic spell_trigger_spell_from_caster
-- with-args legs join the no-SpellScript-bridge /
-- no-AuraScript-bridge queues (the boss_moragg optic-link
-- precedent).
-- All 11 Talk() calls in the file accounted for (11 =
-- SAY_AGGRO 0 (1) + SAY_DEATH 10 (1) + SAY_KILL 8 (1) +
-- EMOTE_BERSERK_RAID 11 (1, scheduler) + SAY_BERSERK 9 (1,
-- scheduler) + SAY_UNCHAINED_MAGIC 1 (1, scheduler) +
-- EMOTE_WARN_BLISTERING_COLD 2 (1, scheduler) +
-- SAY_BLISTERING_COLD 3 (1, scheduler) + SAY_AIR_PHASE 5 (1,
-- scheduler) + SAY_PHASE_2 6 (1, scheduler) +
-- EMOTE_WARN_FROZEN_ORB 7 (1, cross-AI spell leg)).

local ENTRY_SINDRAGOSA = 36853

local SAY_AGGRO = 0
local SAY_KILL = 8
local SAY_DEATH = 10

-- C++ boss_sindragosa::JustEngagedWith:
-- Talk(SAY_AGGRO) — the auriaya engage-port precedent.
local function sindragosaJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_sindragosa::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function sindragosaKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_sindragosa::JustDied:
-- Talk(SAY_DEATH) — the sjonnir JustDied-Talk precedent.
local function sindragosaJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_SINDRAGOSA, 1, sindragosaJustEngagedWith)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 3, sindragosaKilledUnit)
RegisterCreatureEvent(ENTRY_SINDRAGOSA, 4, sindragosaJustDied)
