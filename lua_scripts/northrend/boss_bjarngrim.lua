-- General Bjarngrim (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_bjarngrim.cpp
-- (boss_bjarngrim (CreatureScript) via GetHallsOfLightningAI<boss_bjarngrimAI>
-- (ScriptedAI); npc_stormforged_lieutenant (CreatureScript) via
-- GetHallsOfLightningAI<npc_stormforged_lieutenantAI> (ScriptedAI); both
-- registered from inside AddSC_boss_bjarngrim(); loader decl 98 /
-- call 293 per northrend_script_loader.cpp — the FIRST group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_instance_obsidian_sanctum() (decl 96 / call 291), under
-- the "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_boss_loken() (decl 99 / call 294) — loader order
-- confirmed this run; the checkpoint sequence
-- (instance_obsidian_sanctum -> boss_bjarngrim) is followed).
-- Entry: 28586 General Bjarngrim (halls_of_lightning.h NPC_BJARNGRIM,
-- line 39; DATA_BJARNGRIM = 0, line 31);
-- instance_halls_of_lightning.cpp OnCreatureCreate binds case
-- NPC_BJARNGRIM (line 51) — entry-verifiable, registration proceeds
-- (the nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_bjarngrim"
-- and "npc_stormforged_lieutenant" hits boss_bjarngrim.cpp (+ the loader
-- decl/call lines for AddSC_boss_bjarngrim) only; zero sql/ hits. No
-- bjarngrim lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the CallForHelp(30.0f)
-- lieutenant-fetch leg and instance->SetBossState(DATA_BJARNGRIM,
-- IN_PROGRESS) have no bridges — the boss_bookkeeping precedent).
-- KilledUnit — Talk(SAY_SLAY 4) (event 3 — C++-UNCONDITIONAL: the Talk
-- fires for any victim; no nalorakk gate here — the anubrekhan
-- unconditional variant precedent).
-- JustDied — Talk(SAY_DEATH 5) (event 4;
-- instance->SetBossState(DATA_BJARNGRIM, DONE) has no instance bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 28586
-- — no bridgeable arms):
-- npc_stormforged_lieutenant (NPC_STORMFORGED_LIEUTENANT = 29240, local
-- enum in boss_bjarngrim.cpp — entry-verifiable from the local enum,
-- the acolyte_of_shadron / acolyte_of_vesperon precedent, but the
-- arms are bridge-blocked): Reset (Initialize flag clears — no timer
-- bridge); JustEngagedWith (instance->GetGuidData(DATA_BJARNGRIM) +
-- IsAlive / GetVictim gate + AttackStart — no instance bridge); UpdateAI
-- (SPELL_ARC_WELD 59085 + instance-fetched DoCast(pBjarngrim,
-- SPELL_RENEW_STEEL_N 52774) — no cast / timer / instance bridges; no
-- Talk arms anywhere) — no registration.
-- boss_bjarngrim Reset / Initialize — the canBuff AddAura
-- (SPELL_TEMPORARY_ELECTRICAL_CHARGE 52092) leg, the
-- m_auiStormforgedLieutenantGUID respawn loop (C++ self-notes the GUIDs
-- are never assigned), the stance-reset DoRemoveStanceAura +
-- DoCast(SPELL_DEFENSIVE_STANCE 53790) leg, SetEquipmentSlots
-- (EQUIP_SWORD 37871 / EQUIP_SHIELD 35642), and
-- instance->SetBossState(DATA_BJARNGRIM, NOT_STARTED) — no aura /
-- cast / equipment / instance bridges.
-- EnterEvadeMode — canBuff flag leg — no bridge.
-- DoRemoveStanceAura (stance-switched RemoveAurasDueToSpell 53790 /
-- 53791 / 53792) — no aura-removal bridge.
-- UpdateAI — the stance-change machine (Talk(SAY_DEFENSIVE_STANCE 1) /
-- Talk(SAY_BATTLE_STANCE 2) / Talk(SAY_BERSEKER_STANCE 3) +
-- Talk(EMOTE_DEFENSIVE_STANCE 6 / EMOTE_BATTLE_STANCE 7 /
-- EMOTE_BERSEKER_STANCE 8) — Talk arms riding a no-timer-event machine)
-- + the stance-switched DoCastSelf / DoCastVictim timer events
-- (defensive: SPELL_REFLECTION 36096, KNOCK_AWAY 52029, PUMMEL 12555,
-- IRONFORM 52022; berserker: INTERCEPT 58769, WHIRLWIND 52027,
-- CLEAVE 15284; battle: MORTAL_STRIKE 16856, SLAM 52026) — no
-- timer-event / cast bridges.

local ENTRY_BJARNGRIM = 28586

local SAY_AGGRO = 0
local SAY_SLAY = 4
local SAY_DEATH = 5

-- C++ JustEngagedWith: Talk(AGGRO) + CallForHelp(30.0f) +
-- SetBossState(DATA_BJARNGRIM, IN_PROGRESS) — only the Talk arm is
-- bridgeable.
local function bjarngrimEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: unconditional Talk(SAY_SLAY) — the anubrekhan
-- unconditional variant precedent: the Talk fires for any victim.
local function bjarngrimTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

-- C++ JustDied: Talk(DEATH) + SetBossState(DATA_BJARNGRIM, DONE) —
-- only the Talk arm is bridgeable.
local function bjarngrimDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_BJARNGRIM, 1, bjarngrimEnterCombat)
RegisterCreatureEvent(ENTRY_BJARNGRIM, 3, bjarngrimTargetDied)
RegisterCreatureEvent(ENTRY_BJARNGRIM, 4, bjarngrimDied)
