-- Lord Marrowgar (Icecrown Citadel) — Lua port of
-- src/server/scripts/Northrend/FrozenHalls-relative path
-- src/server/scripts/Northrend/IcecrownCitadel/boss_lord_marrowgar.cpp
-- (703 lines incl. license; 10 scripts — 3 CreatureScripts
-- (boss_lord_marrowgar (BossAI, DATA_LORD_MARROWGAR = 0),
-- npc_coldflame (ScriptedAI summon, SPELL_IMPALED vehicle legs),
-- npc_bone_spike (ScriptedAI trap summon, vehicle passenger legs)) +
-- 6 SpellScripts (spell_marrowgar_coldflame,
-- spell_marrowgar_coldflame_bonestorm, spell_marrowgar_coldflame_damage
-- (AuraScript), spell_marrowgar_bone_spike_graveyard,
-- spell_marrowgar_bone_storm, spell_marrowgar_bone_slice) + 1
-- AreaTriggerScript (at_lord_marrowgar_entrance); all registered from
-- inside AddSC_boss_lord_marrowgar(); loader decl 171 / call 366 per
-- northrend_script_loader.cpp — the FIRST group of the
-- "// Icecrown Citadel" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_marwyn() (call 364 — closes the "// Halls of
-- Reflection" block); the call after it is
-- AddSC_boss_lady_deathwhisper() — verified from the loader this
-- run; the checkpoint sequence (boss_marwyn ->
-- boss_lord_marrowgar) is followed).
-- Entry: 36612 Lord Marrowgar (icecrown_citadel.h NPC_LORD_MARROWGAR,
-- line 169; instance_icecrown_citadel.cpp OnCreatureCreate binds case
-- NPC_LORD_MARROWGAR, line 221) — entry-verifiable, registration
-- proceeds (the nexus_commanders kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for
-- "AddSC_boss_lord_marrowgar" hits boss_lord_marrowgar.cpp only (+
-- the loader decl/call lines); this clone carries no sql/ tree, so
-- ScriptName bindings are DB-side by construction. No marrowgar lua
-- existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnKill (player-gated), 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_lord_marrowgar JustEngagedWith — Talk(SAY_AGGRO 1) (event 1 —
-- the auriaya engage-port precedent; the me->setActive(true),
-- DoZoneInCombat, instance->SetBossState IN_PROGRESS and five
-- ScheduleEvent legs have no bridge).
-- boss_lord_marrowgar KilledUnit — Talk(SAY_KILL 4) player-gated
-- (event 3 — the razuvious player-gated variant precedent; the
-- twelfth ported variant, tenth identical to
-- nalorakk/kelthuzad/gothik/thaddius/garfrost/krick/tyrannus/falric/
-- marwyn).
-- boss_lord_marrowgar JustDied — Talk(SAY_DEATH 5) (event 4 — the
-- sjonnir JustDied-Talk precedent; the _JustDied BossAI helper has
-- no bridge by construction). Melee engine-driven.
-- DOCUMENTED-ONLY (in this header; only entry 36612 registered):
-- npc_coldflame (36672) and npc_bone_spike (36619) expose zero own
-- Talk lines in their classes; their legs (IsSummonedBy
-- vehicle/cast/teleport legs, UpdateAI EVENT_COLDFLAME_TRIGGER /
-- EVENT_FAIL_BONED machines, JustDied/KilledUnit aura-removal legs,
-- PassengerBoarded spline leg, DATA_BONED_ACHIEVEMENT instance
-- SetData leg) have no bridges — not registered (the bronjahm
-- npc_corrupted_soul_fragment precedent).
-- boss_lord_marrowgar Reset scheduler legs — EVENT_ENABLE_BONE_SLICE
-- / EVENT_BONE_SPIKE_GRAVEYARD / EVENT_COLDFLAME /
-- EVENT_WARN_BONE_STORM / EVENT_ENRAGE — no-timer-bridge queue (the
-- boss_toravon precedent); the aura-removal / speed-rate legs have
-- no bridge.
-- boss_lord_marrowgar UpdateAI — the whole EventMap machine — no
-- bridges: EVENT_WARN_BONE_STORM legs carry Talk(EMOTE_BONE_STORM 7)
-- and the SPELL_BONE_STORM self-cast; EVENT_BONE_STORM_BEGIN carries
-- Talk(SAY_BONE_STORM 2), aura-duration and 3x speed legs;
-- EVENT_BONE_STORM_MOVE carries random-NonTank SelectTarget +
-- MovePoint legs (no-random-target-SelectTarget / no-motion
-- queues); EVENT_BONE_STORM_END carries movement removal,
-- MoveChase and event-reschedule legs; EVENT_BONE_SPIKE_GRAVEYARD /
-- EVENT_COLDFLAME carry DoCast / DoCastAOE legs (no-cast bridge);
-- EVENT_ENRAGE carries DoCast SPELL_BERSERK + Talk(SAY_BERSERK 6) —
-- all timer-driven (no-timer-bridge); the UNIT_STATE_CASTING gate,
-- bone-storm-aura gate, bone-slice DoCastVictim legs and
-- DoMeleeAttackIfReady have no bridge. JustReachedHome
-- instance-SetBossState(FAIL) and DATA_BONED_ACHIEVEMENT-reset legs
-- have no instance bridge; DoAction(ACTION_CLEAR_SPIKE_IMMUNITIES)
-- and MovementInform have no bridge.
-- Talk(SAY_ENTER_ZONE 0) rides DoAction(ACTION_TALK_ENTER_ZONE),
-- which only fires from at_lord_marrowgar_entrance — no-DoAction
-- bridge (the krick ACTION_OUTRO precedent); the area trigger joins
-- the unbridged-area-trigger queue (the pit_of_saron cavern-triggers
-- precedent).
-- Talk(SAY_BONESPIKE 3) is called by marrowgarAI->Talk from inside
-- spell_marrowgar_bone_spike_graveyard SpellScript HandleSpikes —
-- no-SpellScript bridge (the boss_moragg optic-link precedent);
-- spell_marrowgar_coldflame / spell_marrowgar_coldflame_bonestorm /
-- spell_marrowgar_bone_storm / spell_marrowgar_bone_slice join the
-- no-SpellScript-bridge queue, and spell_marrowgar_coldflame_damage
-- joins the no-AuraScript-bridge queue.
-- All 8 Talk() calls in the file accounted for (8 = aggro 1 + kill
-- 4 + death 5 + enter-zone 0 + bone-storm 2 + bonespike 3 +
-- berserk 6 + emote-bone-storm 7).

local ENTRY_LORD_MARROWGAR = 36612

local SAY_AGGRO = 1
local SAY_KILL = 4
local SAY_DEATH = 5

-- C++ boss_lord_marrowgar::JustEngagedWith: Talk(SAY_AGGRO) — the
-- auriaya engage-port precedent.
local function marrowgarJustEngagedWith(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ boss_lord_marrowgar::KilledUnit:
-- if (victim->GetTypeId() == TYPEID_PLAYER) Talk(SAY_KILL) — the
-- razuvious player-gated variant precedent.
local function marrowgarKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

-- C++ boss_lord_marrowgar::JustDied: Talk(SAY_DEATH) — the sjonnir
-- JustDied-Talk precedent.
local function marrowgarJustDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 1, marrowgarJustEngagedWith)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 3, marrowgarKilledUnit)
RegisterCreatureEvent(ENTRY_LORD_MARROWGAR, 4, marrowgarJustDied)
