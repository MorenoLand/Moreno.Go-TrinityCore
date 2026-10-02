-- Assembly of Iron (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_assembly_of_iron.cpp
-- (boss_steelbreaker / boss_runemaster_molgeim /
-- boss_stormcaller_brundir (all CreatureScript via
-- GetUlduarAI<...AI> (BossAI), BOSS_ASSEMBLY_OF_IRON = 4 —
-- registered from inside AddSC_boss_assembly_of_iron(); loader
-- decl 116 / call 311 per northrend_script_loader.cpp — the
-- SEVENTH group of the "// Ulduar" block in
-- AddNorthrendScripts(), immediately after
-- AddSC_boss_general_vezax() (decl 117 / call 310); the call
-- after it is AddSC_boss_kologarn() (decl 115 / call 312) —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_general_vezax -> boss_assembly_of_iron) is followed).
-- Entries: 32867 Steelbreaker (ulduar.h NPC_STEELBREAKER,
-- line 67); 32927 Runemaster Molgeim (ulduar.h NPC_MOLGEIM,
-- line 68); 32857 Stormcaller Brundir (ulduar.h NPC_BRUNDIR,
-- line 69) — all entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- seven script names ("boss_steelbreaker",
-- "boss_runemaster_molgeim", "boss_stormcaller_brundir",
-- "spell_shield_of_runes", "spell_assembly_meltdown",
-- "spell_assembly_rune_of_summoning",
-- "achievement_assembly_i_choose_you") hits
-- boss_assembly_of_iron.cpp only; zero sql/ hits for all seven.
-- No assembly/iron lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- boss_steelbreaker — JustEngagedWith: Talk(SAY_AGGRO 0)
-- (event 1; the BossAI::JustEngagedWith passthrough, the
-- DoCast SPELL_HIGH_VOLTAGE 61890 leg, the events.SetPhase
-- leg and the two ScheduleEvent legs (EVENT_BERSERK /
-- EVENT_FUSION_PUNCH) have no bridges — the auriaya
-- engage-port precedent); KilledUnit: player-gated
-- Talk(SAY_SLAY 1) (event 3; who->GetTypeId() ==
-- TYPEID_PLAYER — the razuvious player-gated variant
-- precedent — the twenty-sixth player-gated variant ported;
-- the phase == 3 DoCast SPELL_ELECTRICAL_CHARGE 61902 leg has
-- no bridge); JustDied: Talk(SAY_DEATH 3) (event 4; the
-- _JustDied() passthrough, the instance GetBossState DONE
-- check, the DONE branch (Talk SAY_ENCOUNTER_DEFEATED 4 +
-- DoCastAOE SPELL_KILL_CREDIT 65195), the SetLootRecipient
-- leg and the supercharge DoAction cascade to the surviving
-- bosses have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- boss_runemaster_molgeim — JustEngagedWith: Talk(SAY_AGGRO
-- 0) (event 1; the BossAI::JustEngagedWith passthrough, the
-- SetPhase leg and the three ScheduleEvent legs
-- (EVENT_BERSERK / EVENT_SHIELD_OF_RUNES /
-- EVENT_RUNE_OF_POWER) have no bridges); KilledUnit:
-- player-gated Talk(SAY_SLAY 1) (event 3; the twenty-seventh
-- player-gated variant ported); JustDied: Talk(SAY_DEATH 4)
-- (event 4; the _JustDied() passthrough, the instance
-- GetBossState DONE check, the DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED 5 + DoCastAOE SPELL_KILL_CREDIT),
-- the SetLootRecipient leg and the supercharge DoAction
-- cascade have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- boss_stormcaller_brundir — JustEngagedWith: Talk(SAY_AGGRO
-- 0) (event 1; the BossAI::JustEngagedWith passthrough, the
-- SetPhase leg, the four ScheduleEvent legs (EVENT_BERSERK /
-- EVENT_CHAIN_LIGHTNING / EVENT_OVERLOAD /
-- EVENT_MOVE_POSITION) and the FindNearestCreature
-- NPC_WORLD_TRIGGER 22515 trigger-GUID leg have no bridges);
-- KilledUnit: player-gated Talk(SAY_SLAY 1) (event 3; the
-- twenty-eighth player-gated variant ported); JustDied:
-- Talk(SAY_DEATH 4) (event 4; the _JustDied() passthrough, the
-- instance GetBossState DONE check, the DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED 5 + DoCastAOE SPELL_KILL_CREDIT),
-- the SetLootRecipient leg and the supercharge DoAction
-- cascade have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entries 32867 / 32927 / 32857):
-- Reset / Initialize (all three) — _Reset() + phase = 0 +
-- RemoveAllAuras (+ SetHover(false) / interrupt-immunity reset
-- for brundir) — no bridges.
-- DoAction (all three) — ACTION_SUPERCHARGE 1 (SetFullHealth
-- + SPELL_SUPERCHARGE 61920 + phase++ + event reschedules)
-- and ACTION_ADD_CHARGE 2 (steelbreaker, SPELL_ELECTRICAL_
-- CHARGE trigger cast — the spell_assembly_meltdown producer)
-- — no DoAction bridge (joins the no-DoAction-bridge queue).
-- GetData(DATA_PHASE_3 1) — no GetData bridge; consumer:
-- achievement_assembly_i_choose_you below.
-- UpdateAI event machines — steelbreaker EVENT_FUSION_PUNCH
-- (SPELL_FUSION_PUNCH 61903); EVENT_STATIC_DISRUPTION
-- (SPELL_STATIC_DISRUPTION 44008, phase >= 2);
-- EVENT_OVERWHELMING_POWER (Talk(SAY_POWER 2) +
-- SPELL_OVERWHELMING_POWER 64637, phase >= 3); EVENT_BERSERK
-- (Talk(SAY_BERSERK 5) + SPELL_BERSERK 47008). molgeim
-- EVENT_RUNE_OF_POWER (SPELL_SUMMON_RUNE_OF_POWER 63513,
-- random ally target); EVENT_SHIELD_OF_RUNES
-- (SPELL_SHIELD_OF_RUNES 62274); EVENT_RUNE_OF_DEATH
-- (Talk(SAY_RUNE_DEATH 2) + SPELL_RUNE_OF_DEATH 62269, phase
-- >= 2); EVENT_RUNE_OF_SUMMONING (Talk(SAY_SUMMON 3) +
-- SPELL_RUNE_OF_SUMMONING 62273, phase >= 3); EVENT_BERSERK.
-- brundir EVENT_CHAIN_LIGHTNING (SPELL_CHAIN_LIGHTNING
-- 61879); EVENT_OVERLOAD (Talk(EMOTE_OVERLOAD 7) +
-- Talk(SAY_SPECIAL 2) + SPELL_OVERLOAD 61869);
-- EVENT_LIGHTNING_WHIRL (SPELL_LIGHTNING_WHIRL 61915, phase
-- >= 2); EVENT_LIGHTNING_TENDRILS (Talk(SAY_FLIGHT 3) +
-- SPELL_LIGHTNING_TENDRILS 61887 / VISUAL 61883 + hover /
-- flight / landing state machine, phase >= 3); EVENT_FLIGHT
-- / EVENT_ENDFLIGHT / EVENT_GROUND / EVENT_LAND /
-- EVENT_MOVE_POSITION — no timer-event / cast / summon /
-- phase / movement / hover / DoStartMovement / ResetThreatList
-- bridges; all timer-leg yells ride the unbridgeable event
-- machine.
-- spell_shield_of_runes (62274 — AuraScript
-- AfterEffectRemove: on non-expire removal the caster casts
-- SPELL_SHIELD_OF_RUNES_BUFF 62277 — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue);
-- spell_assembly_meltdown (SPELL_EFFECT_INSTAKILL handler:
-- instance GetGuidData(DATA_STEELBREAKER) ->
-- DoAction(ACTION_ADD_CHARGE) — no SpellScript bridge; joins
-- the no-SpellScript-bridge queue);
-- spell_assembly_rune_of_summoning (62273 — periodic-dummy
-- AuraScript: casts SPELL_RUNE_OF_SUMMONING_SUMMON 62020 with
-- the summoner GUID + OnRemove despawn of the rune summon —
-- no AuraScript bridge; joins the no-AuraScript-bridge
-- queue).
-- achievement_assembly_i_choose_you (OnCheck: target GetAI()
-- -> GetData(DATA_PHASE_3)) — no GetData / achievement
-- bridges; joins the unmodeled-achievement queue.

local ENTRY_STEELBREAKER = 32867
local ENTRY_MOLGEIM = 32927
local ENTRY_BRUNDIR = 32857

local SAY_STEELBREAKER_AGGRO = 0
local SAY_STEELBREAKER_SLAY = 1
local SAY_STEELBREAKER_DEATH = 3
local SAY_MOLGEIM_AGGRO = 0
local SAY_MOLGEIM_SLAY = 1
local SAY_MOLGEIM_DEATH = 4
local SAY_BRUNDIR_AGGRO = 0
local SAY_BRUNDIR_SLAY = 1
local SAY_BRUNDIR_DEATH = 4

-- C++ JustEngagedWith: BossAI passthrough + DoCast
-- SPELL_HIGH_VOLTAGE + SetPhase + ScheduleEvent legs — only
-- the Talk arm is bridgeable.
local function steelbreakerEnterCombat(event, creature, target)
    creature:Talk(SAY_STEELBREAKER_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent;
-- the phase == 3 SPELL_ELECTRICAL_CHARGE leg has no bridge.
local function steelbreakerTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_STEELBREAKER_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + instance
-- GetBossState DONE check + DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED + DoCastAOE SPELL_KILL_CREDIT) +
-- SetLootRecipient + supercharge DoAction cascade — only the
-- per-boss death Talk arm is bridgeable.
local function steelbreakerDied(event, creature, killer)
    creature:Talk(SAY_STEELBREAKER_DEATH)
end

-- C++ JustEngagedWith: BossAI passthrough + SetPhase +
-- ScheduleEvent legs — only the Talk arm is bridgeable.
local function molgeimEnterCombat(event, creature, target)
    creature:Talk(SAY_MOLGEIM_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function molgeimTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_MOLGEIM_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + instance
-- GetBossState DONE check + DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED + DoCastAOE SPELL_KILL_CREDIT) +
-- SetLootRecipient + supercharge DoAction cascade — only the
-- per-boss death Talk arm is bridgeable.
local function molgeimDied(event, creature, killer)
    creature:Talk(SAY_MOLGEIM_DEATH)
end

-- C++ JustEngagedWith: BossAI passthrough + SetPhase +
-- ScheduleEvent legs + FindNearestCreature NPC_WORLD_TRIGGER
-- trigger-GUID leg — only the Talk arm is bridgeable.
local function brundirEnterCombat(event, creature, target)
    creature:Talk(SAY_BRUNDIR_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function brundirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_BRUNDIR_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough + instance
-- GetBossState DONE check + DONE branch (Talk
-- SAY_ENCOUNTER_DEFEATED + DoCastAOE SPELL_KILL_CREDIT) +
-- SetLootRecipient + supercharge DoAction cascade — only the
-- per-boss death Talk arm is bridgeable.
local function brundirDied(event, creature, killer)
    creature:Talk(SAY_BRUNDIR_DEATH)
end

RegisterCreatureEvent(ENTRY_STEELBREAKER, 1, steelbreakerEnterCombat)
RegisterCreatureEvent(ENTRY_STEELBREAKER, 3, steelbreakerTargetDied)
RegisterCreatureEvent(ENTRY_STEELBREAKER, 4, steelbreakerDied)
RegisterCreatureEvent(ENTRY_MOLGEIM, 1, molgeimEnterCombat)
RegisterCreatureEvent(ENTRY_MOLGEIM, 3, molgeimTargetDied)
RegisterCreatureEvent(ENTRY_MOLGEIM, 4, molgeimDied)
RegisterCreatureEvent(ENTRY_BRUNDIR, 1, brundirEnterCombat)
RegisterCreatureEvent(ENTRY_BRUNDIR, 3, brundirTargetDied)
RegisterCreatureEvent(ENTRY_BRUNDIR, 4, brundirDied)
