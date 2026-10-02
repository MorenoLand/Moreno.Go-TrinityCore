-- Tenebron / Shadron / Vesperon (Obsidian Sanctum drake mini-bosses) —
-- Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/ObsidianSanctum/obsidian_sanctum.cpp
-- (npc_tenebron / npc_shadron / npc_vesperon (CreatureScripts), each via
-- GetObsidianSanctumAI<...AI> (dummy_dragonAI : ScriptedAI);
-- registered from inside AddSC_obsidian_sanctum(); loader decl 95 /
-- call 290 per northrend_script_loader.cpp — the SECOND group of the
-- "// Obsidian Sanctum" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sartharion() (decl 94 / call 289), under the
-- "// Obsidian Sanctum" marker; the call after it is
-- AddSC_instance_obsidian_sanctum() (decl 96 / call 291) — loader
-- order confirmed this run; the checkpoint sequence
-- (boss_sartharion -> obsidian_sanctum) is followed).
-- Entries: 30452 Tenebron (obsidian_sanctum.h NPC_TENEBRON, line 40),
-- 30451 Shadron (NPC_SHADRON, line 41), 30449 Vesperon
-- (NPC_VESPERON, line 42) — entry-verifiable (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- twelve script names registered from AddSC_obsidian_sanctum()
-- hits obsidian_sanctum.cpp only; zero sql/ hits. No obsidian_sanctum
-- lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers in the ported arms; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; dummy_dragonAI base,
-- shared across the three drakes):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the DoZoneInCombat
-- and the ScheduleEvent(EVENT_SHADOW_FISSURE / EVENT_SHADOW_BREATH)
-- legs have no bridges — the tharon_ja / malygos DoZoneInCombat /
-- no-timer-event precedents).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious
-- player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player").
-- JustDied — Talk(SAY_DEATH 2) (event 4; the _canLoot /
-- SetLootRecipient leg, the entry-switched power-aura-removal legs
-- (SPELL_POWER_OF_TENEBRON 61248 / SPELL_POWER_OF_SHADRON 58105 /
-- SPELL_POWER_OF_VESPERON 61251 via RemoveAurasDueToSpell +
-- DoRemoveAurasDueToSpellOnPlayers), the acolyte KillSelf legs
-- (FindNearestCreature NPC_ACOLYTE_OF_SHADRON 31218 /
-- NPC_ACOLYTE_OF_VESPERON 31219, 100.0f), the SetBossState
-- (DATA_TENEBRON / DATA_SHADRON / DATA_VESPERON, DONE) legs, and
-- the Twilight Revenge on Sartharion (SPELL_TWILIGHT_REVENGE 60639
-- DoCast via instance GUID fetch) all have no bridges — no
-- aura / instance / cast / creature-search bridges).
-- DOCUMENTED-ONLY (in this header; no registration beyond entries
-- 30452 / 30451 / 30449 — no bridgeable arms):
-- dummy_dragonAI Reset (flag clear + events.Reset + Initialize —
-- no bridge); SetData DATA_CAN_LOOT (_canLoot — no bridge);
-- MovementInform (the POINT_ID_INIT 100 / POINT_ID_LAND 200
-- waypoint machine over dragonCommon[6]: instance-gated
-- EnterEvadeMode, Clear + DoZoneInCombat + random-target chase
-- legs at the LAND point, waypointId wrap + EVENT_FREE_MOVEMENT
-- — no motion / instance / target-selection bridges);
-- OpenPortal (portal grid search (GO_TWILIGHT_PORTAL 193988,
-- 50.0f), Tenebron egg summons (NPC_TWILIGHT_EGG 30882 /
-- NPC_SARTHARION_TWILIGHT_EGG 31204, six positions,
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN 20s), Shadron / Vesperon
-- acolyte summons (NPC_ACOLYTE_OF_SHADRON 31218 /
-- NPC_ACOLYTE_OF_VESPERON 31219, two position sets per drake),
-- the Vesperon InterruptNonMeleeSpells + 32747 cast leg, the
-- Talk(WHISPER_OPEN_PORTAL 6 / WHISPER_OPENED_PORTAL 7) legs
-- (portal-entry Talk — no bridgeable combat event), the
-- portal SetRespawnTime(30000) leg — no summon / cast /
-- creature-search / GO-respawn bridges);
-- ExecuteEvent EVENT_SHADOW_FISSURE (random-target
-- DoCast(SPELL_SHADOW_FISSURE 57579) — no target-selection /
-- cast bridges); EVENT_SHADOW_BREATH (Talk(SAY_BREATH 3) +
-- DoCastVictim(SPELL_SHADOW_BREATH 57570) — no cast bridge);
-- the UpdateAI EVENT_FREE_MOVEMENT machine (no motion bridge).
-- npc_tenebron — Reset (base); JustEngagedWith (base +
-- ScheduleEvent EVENT_HATCH_EGGS 30s — no timer bridge);
-- UpdateAI EVENT_HATCH_EGGS -> OpenPortal — no timer bridge.
-- npc_shadron — Reset (base + aura strips
-- SPELL_TWILIGHT_TORMENT_VESP 57948 / SPELL_GIFT_OF_TWILIGTH_SHA
-- 57835 + SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) — no aura /
-- instance bridges); JustEngagedWith (base + ScheduleEvent
-- EVENT_ACOLYTE_SHADRON 1min — no timer bridge); UpdateAI
-- EVENT_ACOLYTE_SHADRON (instance-gated OpenPortal +
-- SetBossState(DATA_PORTAL_OPEN, IN_PROGRESS) — no timer /
-- instance bridges).
-- npc_vesperon — Reset (base); JustEngagedWith (base +
-- ScheduleEvent EVENT_ACOLYTE_VESPERON 1min — no timer bridge);
-- UpdateAI EVENT_ACOLYTE_VESPERON (instance-gated OpenPortal +
-- DoCastVictim(SPELL_TWILIGHT_TORMENT_VESP 57948) — no timer /
-- instance / cast bridges).
-- npc_acolyte_of_shadron (NPC_ACOLYTE_OF_SHADRON 31218,
-- entry-verifiable from the local enum) — Reset
-- (DespawnOrUnsummon(28s), instance-gated AddAura gift-of-twilight
-- 58766/57835, AddAura SPELL_TWILIGHT_SHIFT_ENTER 57620), JustDied
-- (instance SetBossState + player-loop cast/remove legs +
-- RemoveAurasDueToSpell legs on DATA_SARTHARION / DATA_SHADRON
-- via GUID fetch), UpdateAI (melee) — no despawn / aura / cast /
-- instance bridges; no bridgeable arms (no Talk arms).
-- npc_acolyte_of_vesperon (NPC_ACOLYTE_OF_VESPERON 31219) —
-- same verdict: Reset / JustDied / UpdateAI melee legs all
-- aura / cast / instance-gated — no bridgeable arms.
-- npc_twilight_eggs (NPC_TWILIGHT_EGG 30882 /
-- NPC_SARTHARION_TWILIGHT_EGG 31204) — Reset (aura +
-- ScheduleEvent EVENT_TWILIGHT_EGGS 20s), SpawnWhelps (whelp
-- summons NPC_TWILIGHT_WHELP 30890 / NPC_SARTHARION_TWILIGHT_WHELP
-- 31214 + KillSelf), JustSummoned (DoZoneInCombat), UpdateAI —
-- no aura / timer / summon bridges; no bridgeable arms.
-- npc_flame_tsunami (NPC_FLAME_TSUNAMI 30616, boss_sartharion.cpp
-- enum) — ctor (SetDisplayId 11686 + aura SPELL_FLAME_TSUNAMI
-- 57494), Reset (REACT_PASSIVE + flag legs + timer schedules),
-- UpdateAI (DoCast(SPELL_FLAME_TSUNAMI_DMG_AURA 57491) +
-- lava-blaze (NPC_LAVA_BLAZE 30643) search + buff cast
-- SPELL_FLAME_TSUNAMI_BUFF 60430) — no aura / cast /
-- creature-search bridges; no bridgeable arms.
-- npc_twilight_fissure — entry UNVERIFIABLE from C++ (no local
-- NPC enum; the fissure is spawned by SPELL_SHADOW_FISSURE
-- 57579, entry not named in the C++ tree) — joins the
-- entry-unverifiable queue (the npc_image_belgaristrasz precedent);
-- Reset / UpdateAI (DoCastAOE(SPELL_VOID_BLAST 57581) +
-- RemoveAllAuras + KillSelf) — no aura / cast bridges regardless.
-- npc_twilight_whelp — Reset (RemoveAllAuras + DoZoneInCombat +
-- ScheduleEvent EVENT_FADE_ARMOR 1s), UpdateAI (DoCastVictim
-- SPELL_FADE_ARMOR 60708) — no aura / timer / cast bridges; no
-- bridgeable arms.
-- achievement_twilight_assist / achievement_twilight_duo /
-- achievement_twilight_zone (target->GetAI()->GetData
-- (TWILIGHT_ACHIEVEMENTS) >= 1 / >= 2 / == 3 — Sartharion's
-- GetData arm, itself documented-only) — no achievement bridge;
-- all three join the unmodeled-achievement queue.

local ENTRY_TENEBRON = 30452
local ENTRY_SHADRON = 30451
local ENTRY_VESPERON = 30449

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

-- C++ dummy_dragonAI::JustEngagedWith: Talk(SAY_AGGRO) +
-- DoZoneInCombat + ScheduleEvent(EVENT_SHADOW_FISSURE,
-- EVENT_SHADOW_BREATH) — only the Talk arm is bridgeable.
local function dragonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ dummy_dragonAI::KilledUnit: victim->GetTypeId() ==
-- TYPEID_PLAYER gate, then Talk(SAY_SLAY) — the razuvious
-- player-gated variant precedent.
local function dragonTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ dummy_dragonAI::JustDied: _canLoot leg + entry-switched
-- power-aura removals + acolyte KillSelf + instance SetBossState
-- + Talk(SAY_DEATH) + Twilight Revenge on Sartharion — only the
-- Talk arm is bridgeable.
local function dragonDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

for _, entry in ipairs({ ENTRY_TENEBRON, ENTRY_SHADRON, ENTRY_VESPERON }) do
    RegisterCreatureEvent(entry, 1, dragonEnterCombat)
    RegisterCreatureEvent(entry, 3, dragonTargetDied)
    RegisterCreatureEvent(entry, 4, dragonDied)
end
