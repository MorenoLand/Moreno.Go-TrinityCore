-- The Black Morass: zone script — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/TheBlackMorass/
-- the_black_morass.cpp (359 lines; classes npc_medivh_bm :
-- public CreatureScript (npc_medivh_bmAI : public ScriptedAI) and
-- npc_time_rift : public CreatureScript (npc_time_riftAI :
-- public ScriptedAI); both GetAI via GetBlackMorassAI<T> (TBMScriptName
-- "instance_the_black_morass" gate); AddSC_the_black_morass at end
-- registers both; kalimdor loader decl 44 / call 157 per
-- kalimdor_script_loader.cpp — the fourth "// CoT The Black Morass"
-- loader group, right after AddSC_boss_temporus()).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "npc_medivh_bm" and "npc_time_rift" (loader decl/call
-- lines only otherwise).
-- Entries: the_black_morass.h:55 names NPC_MEDIVH = 15608 and
-- instance_the_black_morass.cpp:137 GUID-binds it in OnCreatureCreate
-- (DATA_MEDIVH) — ramstein-strength name-to-entry tie; h:56 names
-- NPC_TIME_RIFT = 17838 and instance_the_black_morass.cpp:296 has
-- medivh SummonCreature(NPC_TIME_RIFT, ...) — summon-strength tie;
-- the creature_template ScriptName bindings stay DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset.
-- Ported arms (the self-contained in-combat legs only):
-- - npc_medivh_bm death (C++ JustDied): Talk SAY_DEATH (5) unless the
--   killer has Medivh's own entry (C++: killer &&
--   killer->GetEntry() == me->GetEntry() -> return — the self-kill
--   skip; thrall old_hillsbrad death-arm convention).
-- Unmodeled (documented-only, no bridges):
-- - npc_medivh_bm Reset: instance->GetData(TYPE_MEDIVH = 1) ==
--   IN_PROGRESS -> DoCast(me, SPELL_CHANNEL 31556, true) else remove
--   the aura — instance-data bridge blocked (standing).
-- - npc_medivh_bm MoveInLineOfSight player leg: player within 10.0f,
--   TYPE_MEDIVH neither IN_PROGRESS nor DONE -> Talk SAY_INTRO (1) +
--   SetData(TYPE_MEDIVH, IN_PROGRESS) + DoCast(me, SPELL_CHANNEL) +
--   Check_Timer = 5000 — MoveInLineOfSight proximity bridge missing
--   (temporus precedent) and the SetData leg has no instance-data
--   bridge (standing).
-- - npc_medivh_bm MoveInLineOfSight infinite-mob leg: TYPEID_UNIT
--   within 15.0f && entry in {NPC_INFINITE_ASSASIN 17835,
--   NPC_INFINITE_WHELP 21818, NPC_INFINITE_CRONOMANCER 17892,
--   NPC_INFINITE_EXECUTIONER 18994, NPC_INFINITE_VANQUISHER 18995}
--   while TYPE_MEDIVH == IN_PROGRESS -> who->StopMoving() +
--   who->CastSpell(me, SPELL_CORRUPT 31326 / SPELL_CORRUPT_AEONUS
--   37853 for NPC_AEONUS 17881, false) — no StopMoving bridge and no
--   cast-by-other-actor bridge (aeonus MoveInLineOfSight precedent).
-- - npc_medivh_bm AttackStart: entirely commented out in C++ (no-op).
-- - npc_medivh_bm JustEngagedWith: empty in C++ (no-op).
-- - npc_medivh_bm SpellHit: SPELL_CORRUPT_AEONUS hit ->
--   SpellCorrupt_Timer = 1000, SPELL_CORRUPT hit -> 3000 — SpellHit
--   never fires in the Lua API (standing SpellHit-15-never-fires
--   queue).
-- - npc_medivh_bm UpdateAI's whole machine: SpellCorrupt_Timer arm
--   (SetData(TYPE_MEDIVH, SPECIAL) on expiry + re-arm 1000/3000 from
--   HasAura(SPELL_CORRUPT_AEONUS 37853 / SPELL_CORRUPT 31326));
--   Check_Timer 5s arm (instance->GetData(DATA_SHIELD = 12) shield
--   %; Life25/Life50/Life75 latches -> Talk SAY_WEAK25 (4) /
--   SAY_WEAK50 (3) / SAY_WEAK75 (2); TYPE_MEDIVH == NOT_STARTED ->
--   DespawnOrUnsummon + Respawn; TYPE_RIFT == DONE -> Talk SAY_WIN
--   (6) + remove SPELL_CHANNEL + SetData(TYPE_MEDIVH, DONE) with the
--   "@todo start the post-event here" comment) — every leg reads or
--   writes instance data; instance-data bridge blocked (standing).
-- - SAY_ENTER (0) "where does this belong?", SAY_ORCS_ENTER (7),
--   SAY_ORCS_ANSWER (8): declared in the MedivhBm enum but never
--   Talked anywhere in C++ (archimonde SAY_SOUL_CHARGE case).
-- - SPELL_PORTAL_RUNE 32570, SPELL_BLACK_CRYSTAL 32563,
--   SPELL_PORTAL_CRYSTAL 32564, SPELL_BANISH_PURPLE 32566,
--   SPELL_BANISH_GREEN 32567, C_COUNCIL_ENFORCER 17023: declared but
--   unused in this cpp.
-- - npc_time_rift (NPC_TIME_RIFT 17838): DOCUMENTED-not-registered —
--   zero bridgeable arms. Reset reads instance->GetData
--   (DATA_PORTAL_COUNT = 11) to pick mWaveId 0/1/2 (<6 / >12 bands);
--   DoSelectSummon walks PortalWaves[mWaveId].PortalMob
--   ({{17835,21818,17892,0},{18994,17892,21818,17835},
--   {18994,18995,17892,17835}} with mRiftWaveCount reset when
--   (>2 && mWaveId < 1) || >3; whelp entries summon 3);
--   DoSummonAtRift gates on TYPE_MEDIVH != IN_PROGRESS
--   (InterruptNonMeleeSpells(true) + RemoveAllAuras on failure),
--   GetRandomNearPosition(10.0f) + map height/water Z normalization,
--   DoSummon(creature_entry, pos, 30s,
--   TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT) + AddThreat on
--   instance->GetGuidData(DATA_MEDIVH) with 0.0f; UpdateAI's 15s
--   TimeRiftWave_Timer pump then, when not non-melee casting,
--   setDeathState(JUST_DIED) + SetData(TYPE_RIFT = 2, SPECIAL) if
--   TYPE_RIFT == IN_PROGRESS. Every leg needs the instance-data,
--   summon, or nearby-medivh-GUID bridges — none exist (standing);
--   no Talk, combat, or engage arm to wire.

local ENTRY_MEDIVH = 15608
local SAY_DEATH = 5

local function onDied(event, creature, killer)
    -- C++ JustDied skips the yell when the killer has Medivh's own
    -- entry (killer && killer->GetEntry() == me->GetEntry() -> return).
    if killer and killer:GetEntry() == ENTRY_MEDIVH then
        return
    end
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_MEDIVH, 4, onDied)
