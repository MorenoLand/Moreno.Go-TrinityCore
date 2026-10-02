-- Krick (Pit of Saron) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls/PitOfSaron/boss_krickandick.cpp
-- (671 lines incl. license; 8 scripts — boss_ick (CreatureScript via
-- GetPitOfSaronAI (BossAI), DATA_ICK = 1); boss_krick (CreatureScript via
-- GetPitOfSaronAI (ScriptedAI), DATA_KRICK = 4); spell_krick_explosive_barrage
-- (AuraScript: periodic player-proximity scan -> SPELL_EXPLOSIVE_BARRAGE_SUMMON);
-- spell_ick_explosive_barrage (AuraScript: MoveIdle on apply, MoveChase(victim)
-- on remove); spell_exploding_orb_hasty_grow (AuraScript: 15 stacks ->
-- SPELL_EXPLOSIVE_BARRAGE_DAMAGE + aura-strip + DespawnOrUnsummon);
-- spell_krick_pursuit + spell_krick_pursuit_AuraScript (SpellScript +
-- AuraScript pair: ick->AI()->Talk(SAY_ICK_CHASE_1, target) + pursuit-aura /
-- threat store+fixate, ACTION_RESET_THREAT on aura remove);
-- spell_krick_pursuit_confusion (AuraScript: taunt/attack-me spell-immune
-- toggle on linked aura apply/remove); all registered from inside
-- AddSC_boss_ick(); loader decl 163 / call 358 per
-- northrend_script_loader.cpp — the FOURTH group of the "// Pit of Saron"
-- block in AddNorthrendScripts(), immediately after AddSC_boss_garfrost()
-- (call 357); the call after it is AddSC_boss_tyrannus() (call 359) —
-- verified from the loader this run; the checkpoint sequence
-- (boss_garfrost -> boss_ick) is followed).
-- Entries: 36477 Krick (pit_of_saron.h NPC_KRICK, line 50;
-- instance_pit_of_saron.cpp OnCreatureCreate binds case NPC_KRICK, line 75) —
-- entry-verifiable, registration proceeds (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript ScriptName
-- binding is DB-side as usual. 36476 Ick (pit_of_saron.h NPC_ICK, line 51) is
-- entry-verifiable but carries no bridgeable own arms — NOT registered (the
-- bronjahm npc_corrupted_soul_fragment precedent). DATA_ICK = 1 (line 33);
-- DATA_KRICK = 4 (line 38); DATA_TYRANNUS_EVENT = 7 (line 41);
-- DATA_JAINA_SYLVANAS_1 = 5 (line 39). NPC_JAINA_PART1 = 36993 (line 58);
-- NPC_SYLVANAS_PART1 = 36990 (line 56); NPC_EXPLODING_ORB = 36610 (line 92).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_ick"
-- hits boss_krickandick.cpp only (+ the loader decl/call lines); this
-- clone carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. No krick/ick lua existed.
-- Eluna creature events: 3 OnKill (player-gated).
-- Ported arms (C++-exact for all modeled arms):
-- boss_krick KilledUnit — Talk(SAY_KRICK_SLAY 1) player-gated (event 3 —
-- the razuvious player-gated variant precedent; the eighth ported variant,
-- sixth identical to nalorakk/kelthuzad/gothik/thaddius/garfrost).
-- DOCUMENTED-ONLY (in this header; only entry 36477 registered):
-- boss_ick JustEngagedWith — krick->AI()->Talk(SAY_KRICK_AGGRO 0) via
-- ObjectAccessor::GetCreature(*me, instance->GetGuidData(DATA_KRICK)) —
-- instance-driven Talk on another creature's AI (no instance bridge; the
-- garfrost tyrannus->AI()->Talk precedent).
-- boss_ick UpdateAI — the whole EventMap machine (EVENT_TOXIC_WASTE /
-- EVENT_SHADOW_BOLT random-target krick casts, EVENT_MIGHTY_KICK,
-- EVENT_SPECIAL -> RAND(EXPLOSIVE_BARRAGE/POISON_NOVA/PURSUIT)) and the
-- Talk legs riding it — krick->AI()->Talk(SAY_KRICK_BARRAGE_1 2) +
-- Talk(SAY_KRICK_BARRAGE_2 3), krick->AI()->Talk(SAY_KRICK_POISON_NOVA 4),
-- Talk(SAY_ICK_POISON_NOVA 0), krick->AI()->Talk(SAY_KRICK_CHASE 5) —
-- no-timer-bridge queue (the boss_toravon precedent); the
-- SelectTarget(SelectTargetMethod::Random, 0/1) legs add the
-- no-random-target-SelectTarget queue; the DoAction threat
-- store/reset (ACTION_STORE_OLD_TARGET / ACTION_RESET_THREAT) and
-- DoMeleeAttackIfReady have no bridge.
-- boss_ick JustDied — _JustDied() passthrough, RemoveAllPassengers on the
-- vehicle, ForceCombatStop(krick) + krick->AI()->DoAction(ACTION_OUTRO) —
-- no-DoAction / no-instance bridge legs, so 36476 Ick is NOT registered.
-- boss_krick Reset — SetReactState(REACT_PASSIVE) +
-- SetFlag(UNIT_FLAG_NON_ATTACKABLE) — no bridge (the intro-outro
-- machine rides it).
-- boss_krick DoAction(ACTION_OUTRO) — tyrannus NearTeleportTo / SummonCreature
-- choreography + MovePoint(POINT_KRICK_INTRO) — no instance / motion bridges.
-- boss_krick MovementInform — POINT_KRICK_INTRO landing ->
-- Talk(SAY_KRICK_OUTRO_1 6) + PHASE_OUTRO flip — no-MovementInform bridge
-- (the violet_hold saboteur precedent).
-- boss_krick UpdateAI outro machine — EVENT_OUTRO_1..13 / EVENT_OUTRO_END:
-- Talk(SAY_KRICK_OUTRO_3 7) / Talk(SAY_KRICK_OUTRO_5 8) /
-- Talk(SAY_KRICK_OUTRO_8 9) ride timer legs (no-timer-bridge queue); the
-- jainaOrSylvanas->AI()->Talk(SAY_JAYNA_OUTRO_2/4/10) /
-- Talk(SAY_SYLVANAS_OUTRO_2/4/10) and
-- tyrannus->AI()->Talk(SAY_TYRANNUS_OUTRO_7/9) legs are instance-driven
-- Talk on other creatures' AI (no instance bridge; the
-- instance_violet_hold ScheduleCyanigosaIntro precedent); the SummonCreature
-- / MovePoint / MOVEMENTFLAG_FLYING / SPELL_STRANGULATING /
-- SPELL_KRICK_KILL_CREDIT / SetStandState(dead) legs have no bridges.
-- The six SpellScript / AuraScript hooks (krick_explosive_barrage,
-- ick_explosive_barrage, exploding_orb_hasty_grow, krick_pursuit +
-- pursuit_AuraScript, krick_pursuit_confusion) join the no-SpellScript /
-- no-AuraScript bridge queues (the boss_moragg optic-link precedent);
-- ick->AI()->Talk(SAY_ICK_CHASE_1 1, target) inside spell_krick_pursuit is
-- spell-driven Talk with no bridge by construction.

local ENTRY_KRICK = 36477

local SAY_KRICK_SLAY = 1

-- C++ boss_krickAI::KilledUnit: if (victim->GetTypeId() != TYPEID_PLAYER)
-- return; Talk(SAY_KRICK_SLAY) — the razuvious player-gated variant
-- precedent.
local function krickKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KRICK_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_KRICK, 3, krickKilledUnit)
