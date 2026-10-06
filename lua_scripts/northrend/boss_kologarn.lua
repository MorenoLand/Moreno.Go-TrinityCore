-- Kologarn (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_kologarn.cpp
-- (boss_kologarn (CreatureScript) via
-- GetUlduarAI<boss_kologarnAI> (BossAI), BOSS_KOLOGARN = 5
-- — registered from inside AddSC_boss_kologarn(); loader decl
-- 115 / call 312 per northrend_script_loader.cpp — the EIGHTH
-- group of the "// Ulduar" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_assembly_of_iron() (decl 116 /
-- call 311); the call after it is AddSC_boss_mimiron() —
-- loader order confirmed this run; the checkpoint sequence
-- (boss_assembly_of_iron -> boss_kologarn) is followed).
-- Entry: 32930 Kologarn (ulduar.h NPC_KOLOGARN, line 70 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- nine script names ("boss_kologarn",
-- "spell_ulduar_rubble_summon", "spell_ulduar_squeezed_lifeless",
-- "spell_ulduar_cancel_stone_grip",
-- "spell_ulduar_stone_grip_cast_target",
-- "spell_ulduar_stone_grip_absorb", "spell_ulduar_stone_grip",
-- "spell_kologarn_stone_shout",
-- "spell_kologarn_summon_focused_eyebeam") hits
-- boss_kologarn.cpp only; zero sql/ hits for all nine. The
-- lua port below was written from a prior-session audit and
-- re-audited this run: all Talk arms and id claims verified
-- against the C++ (the xt002 22:38 / vezax 22:48 /
-- assembly_of_iron 22:54 precedent).
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4
-- OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the six
-- ScheduleEvent legs, the vehicle-kit DoZoneInCombat arms leg
-- and the BossAI::JustEngagedWith passthrough have no bridges
-- — the auriaya engage-port precedent).
-- KilledUnit — player-gated Talk(SAY_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent — the twenty-ninth
-- player-gated variant ported).
-- JustDied — Talk(SAY_DEATH 6) (event 4; the DoCast
-- SPELL_KOLOGARN_PACIFY 63726 leg, the MoveTargetedHome leg,
-- the SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
-- leg, the SetCorpseDelay(604800) leg and the _JustDied()
-- passthrough have no bridges — the sjonnir JustDied-Talk
-- precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 32930):
-- Reset — _Reset() + RemoveFlag(NOT_SELECTABLE) +
-- eyebeamTarget.Clear() — no bridges.
-- PassengerBoarded — Talk(SAY_LEFT_ARM_GONE 2) /
-- Talk(SAY_RIGHT_ARM_GONE 3) on arm detach, SPELL_ARM_DEAD_
-- DAMAGE 63629 + rubble stalker SPELL_FALLING_RUBBLE 63821 /
-- SPELL_SUMMON_RUBBLE 63633 + EVENT_STONE_SHOUT scheduling /
-- CRITERIA_DISARMED timed-achievement start legs, and the
-- attach branch (EVENT_STONE_SHOUT cancel + DoZoneInCombat) —
-- no PassengerBoarded bridge.
-- JustSummoned — focused-eyebeam visual casts
-- (SPELL_FOCUSED_EYEBEAM_VISUAL_LEFT 63676 /
-- SPELL_FOCUSED_EYEBEAM_VISUAL_RIGHT 63702), NPC_RUBBLE
-- summon bookkeeping, REACT_PASSIVE + MoveChase(eyebeamTarget)
-- — no summon / react-state / motion-master bridges.
-- UpdateAI event machine — EVENT_MELEE_CHECK
-- (SPELL_PETRIFY_BREATH 62030); EVENT_SWEEP
-- (NPC_ARM_SWEEP_STALKER + SPELL_ARM_SWEEP 63766);
-- EVENT_SMASH (SPELL_TWO_ARM_SMASH 63356 / SPELL_ONE_ARM_
-- SMASH 63573); EVENT_STONE_SHOUT (SPELL_STONE_SHOUT 63716);
-- EVENT_ENRAGE (DoCast 47008 + Talk(SAY_BERSERK 7));
-- EVENT_RESPAWN_LEFT_ARM / EVENT_RESPAWN_RIGHT_ARM
-- (InstallAccessory NPC_LEFT_ARM / NPC_RIGHT_ARM); EVENT_
-- STONE_GRIP (DoCast 62166 + Talk(SAY_GRAB_PLAYER 5) +
-- Talk(EMOTE_STONE_GRIP 8)); EVENT_FOCUSED_EYEBEAM
-- (SPELL_SUMMON_FOCUSED_EYEBEAM 63342) — no timer-event /
-- cast / vehicle / summon bridges; all timer-leg yells ride
-- the unbridgeable event machine.
-- spell_ulduar_rubble_summon (SCRIPT_EFFECT SpellScript: caster
-- casts GetEffectValue() with originalCaster = instance
-- GetGuidData(BOSS_KOLOGARN) — no SpellScript bridge; joins
-- the no-SpellScript-bridge queue);
-- spell_ulduar_stone_grip_cast_target (SpellScript area-target
-- filters: EFFECT_0 removes the main tank + non-player targets,
-- EFFECT_1/2 subsequential fill — no SpellScript bridge;
-- joins the no-SpellScript-bridge queue);
-- spell_ulduar_cancel_stone_grip (SCRIPT_EFFECT SpellScript:
-- target removes the two CalcValue spell ids — no SpellScript
-- bridge; joins the no-SpellScript-bridge queue);
-- spell_ulduar_squeezed_lifeless (SpellScript INSTAKILL
-- handler — no SpellScript bridge; joins the
-- no-SpellScript-bridge queue);
-- spell_ulduar_stone_grip_absorb (AuraScript AfterEffectRemove
-- school-absorb: non-enemy-spell removal -> rubble stalker
-- casts SPELL_STONE_GRIP_CANCEL 65594 — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue);
-- spell_ulduar_stone_grip (AuraScript: vehicle OnRemove ->
-- owner removes aurEff-amount spell; stun AfterEffectRemove ->
-- caster RemoveAurasDueToSpell(GetId()) — no AuraScript
-- bridge; joins the no-AuraScript-bridge queue);
-- spell_kologarn_stone_shout (SpellScript area-target filter
-- on EFFECT_0 — no SpellScript bridge; joins the
-- no-SpellScript-bridge queue);
-- spell_kologarn_summon_focused_eyebeam (SpellScript
-- FORCE_CAST handler: caster casts the effect's TriggerSpell
-- — no SpellScript bridge; joins the no-SpellScript-bridge
-- queue).

local ENTRY_KOLOGARN = 32930

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 6

-- C++ JustEngagedWith: six ScheduleEvent legs + vehicle-kit
-- DoZoneInCombat arms leg + BossAI passthrough — only the
-- Talk arm is bridgeable.
local function kologarnEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then Talk
-- (SAY_SLAY) — the razuvious player-gated variant precedent.
local function kologarnTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: DoCast PACIFY + MoveTargetedHome + SetFlag +
-- SetCorpseDelay + _JustDied() + Talk(SAY_DEATH) — only the
-- Talk arm is bridgeable.
local function kologarnDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_KOLOGARN, 1, kologarnEnterCombat)
RegisterCreatureEvent(ENTRY_KOLOGARN, 3, kologarnTargetDied)
RegisterCreatureEvent(ENTRY_KOLOGARN, 4, kologarnDied)
