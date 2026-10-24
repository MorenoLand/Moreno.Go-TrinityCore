-- npcs_special PART 1 (C++ lines 93-1153) — triage result 2026-10-18.
-- Source: src/server/scripts/World/npcs_special.cpp.
-- VERDICT: 8 of 9 classes are DOCUMENTED-ONLY (gap classes); npc_injured_patient's
-- Reset arm is PORTED below (all legs bridged). Every other class is blocked
-- either by missing entry constants (rule: entries only from C++ NPC_ consts,
-- never invented) or by missing method bridges, verified against the CURRENT
-- runtime this run (lua_creature_events.go,
-- lua_objects.go, lua_player.go, scripting/object.go).
-- Sibling file handles C++ lines 1154+.

-- npc_air_force_bots (C++ :43-188) — DOCUMENTED-ONLY.
-- Entries ARE in-file (airforceSpawns table :72-98: 2614,2615,21974,21993,
-- 21996,21997,21999,22001,22002,22003,22063,22065,22066,22068,22069,22070,
-- 22071,22078,22079,22080,22086,22087,22088,22090,22124,22125,22126).
-- Arms: UpdateAI :125-150 (summon guard via SummonCreature :136, keep
-- _toAttack GUID set, guard->EngageWithTarget :147, ALARMBOT casts
-- SPELL_GUARDS_MARK 38067 on the target :149); MoveInLineOfSight :153-181
-- (TYPEID_PLAYER gate, range check :165 RANGE_ALARMBOT 100 / RANGE_TRIPWIRE
-- 15, IsHostileTo :168, IsValidAttackTarget :171, TRIPWIRE skips flying :174).
-- Missing bridges: EngageWithTarget / IsEngagedBy, IsHostileTo,
-- IsValidAttackTarget, unit-flag manipulation (the creature:
-- SummonCreature bridge is live — stale "no bridge anywhere in
-- engine/" retracted — but the machine's core legs are the
-- engage/hostility gates, not the summon).

-- npc_chicken_cluck (C++ :194-292) — DOCUMENTED-ONLY.
-- Arms: Reset :216-221 (faction FACTION_PREY, remove QUESTGIVER flag);
-- UpdateAI :224-239 (120s questgiver-flag reset); ReceiveEmote :241-260
-- (TEXT_EMOTE_CHICKEN 22: quest 3861 status NONE + 1/30 roll -> questgiver
-- flag + FACTION_FRIENDLY + Talk hello; TEXT_EMOTE_CHEER 21: quest COMPLETE
-- -> questgiver flag + Talk); OnQuestAccept :262-266 / OnQuestReward :268-272.
-- Missing: no entry consts in the C++ tree (DB-side scriptname binding;
-- cannot register); the FACTION_PREY<->FRIENDLY legs are load-bearing,
-- not cosmetic, and the arms ride the unbridgeable quest-status gate
-- (SetFaction itself is bridged — lua_creature_events.go:1125); no
-- precise GetQuestStatus — only
-- player:HasQuest bool (lua_player.go:303), which cannot distinguish
-- COMPLETE (the cheer leg) from other states.

-- npc_dancing_flames (C++ :304-414) — DOCUMENTED-ONLY.
-- Arms: Reset :327-340 (self-cast SPELL_BRAZIER 45423 + SPELL_FIERY_AURA
-- 45427, Relocate z+0.94, SetDisableGravity, ONESHOT_DANCE 94, heartbeat);
-- UpdateAI :342-354 (3500ms re-dance via Active/CanIteract); JustEngagedWith
-- :356 no-op; ReceiveEmote :359-389 (LOS + 30yd gate, SetFacingToObject,
-- KISS->SHY 24 / WAVE->WAVE 3 / BOW->BOW 2 / JOKE->LAUGH 11, DANCE->cast
-- SPELL_SEDUCTION 47057 when player lacks that aura).
-- Shape-portable fragments exist (event 8 + player + textEmote, PerformEmote,
-- creature:CastSpell(player,...), player:HasAura, IsWithinDistInMap,
-- OnAIUpdate 7 with 100ms diff) but: no entry consts in the C++ tree
-- (cannot register); the IsWithinLOS bridge went live 2026-10-23
-- (WorldObject::IsWithinLOSInMap twin) but the emote gate stays
-- unported with the class on the no-entry-consts ground;
-- no Relocate / heartbeat bridges (SetDisableGravity is bridged since
-- 2026-10-23, but the class stays documented-only on the no-entry-consts
-- ground above).
-- The me->SetFacingToObject(player) leg (C++ :364) is bridgeable via
-- the atan2 twin (devourer_of_souls precedent) — the old "no
-- SetFacingToObject bridge" parenthetical is retracted — but the class
-- stays documented-only on the no-entry-consts + no-IsWithinLOS
-- grounds above. Emote/text-emote IDs verified from
-- src/server/shared/SharedDefines.h (TEXT_EMOTE_KISS 58, WAVE 101, BOW 17,
-- JOKE 329, DANCE 34; EMOTE_ONESHOT_DANCE 94, SHY 24, LAUGH 11) for the
-- sibling lines 1154+ run if it ever gains entry constants.

-- npc_torch_tossing_target_bunny_controller (C++ :418-452) — DOCUMENTED-ONLY.
-- Arm: Reset :434-443 schedules casts of SPELL_TORCH_TARGET_PICKER 45907 on
-- a nullptr target at 2s / 5s / 7s / 10s ... (outer Repeat 5s, inner +3s).
-- Shape is timer-portable (OnSpawn 5 / OnReset 23 + CreateLuaEvent chains),
-- but CastSpell(nil, ...) falls back to the creature's victim and is a no-op
-- without one (lua_creature_events.go:57-95) — there is no null-target cast
-- bridge, so the ported timer would silently cast nothing. Also no entry
-- consts in the C++ tree.

-- npc_midsummer_bunny_pole (C++ :456-574) — DOCUMENTED-ONLY.
-- Arms: DoAction :484-492 starts the event (EVENT_CAST_RED_FIRE_RING);
-- UpdateAI :499-543 alternates SPELL_RED_FIRE_RING 46836 / SPELL_BLUE_FIRE_RING
-- 46842 casts on GO_RIBBON_POLE 181605 within 10yd, stopping when no nearby
-- player has aura SPELL_RIBBON_DANCE_COSMETIC 29726 (checkNearbyPlayers).
-- No DoAction bridge exists (no creature event for it); checkNearbyPlayers
-- needs a radius player-aura search — no bridge; no entry consts in tree.

-- npc_doctor (C++ :583-772; Gustav Vanhowzen 12939 / Gregory Victor 12920,
-- in-file enum Doctor :534-541) — DOCUMENTED-ONLY (summon-machine).
-- Arms: Reset :646-650; BeginEvent :655-683 (fires from OnQuestAccept :768-772
-- on quests 6624/6622, loads AllianceCoords 7 / HordeCoords 6 positions);
-- PatientDied :686-708 (FailQuest after 5 deaths — bridge live,
-- lua_player.go:410; arm rides the unbridged DoAction relay);
-- PatientSaved :710-746 (AreaExploredOrEventHappens at 15 saved — no
-- bridge); UpdateAI :893-946 (10s summon timer, picks random Coordinates
-- point, summons 12938/12936/12937 or 12923/12924/12925 via
-- me->SummonCreature :922 (bridge live — stale "NO SummonCreature
-- bridge" retracted) — but the machine's heart is the cross-AI write
-- (Patient->AI()->DoctorGUID + Coord, no bridge), the PatientDied /
-- PatientSaved DoAction relay (no bridge), FailQuest (bridged,
-- lua_player.go:410) and AreaExploredOrEventHappens (no bridge) — summon alone spawns
-- patients the machine can never track.
-- UNIT_FLAG_NOT_SELECTABLE / UNIT_FLAG_PLAYER_CONTROLLED manipulation
-- is bridgeable (creature:SetFlag, lua_creature_events.go:1919) — the
-- blockers are the cross-AI legs above, not the flag.

-- npc_injured_patient (C++ :778-890; entries 12923/12924/12925,
-- 12936/12937/12938 in-file) — Reset arm PORTED below; rest DOCUMENTED-ONLY.
-- Arms: Reset :788-814 PORTED (75/50/25% health by entry — SetHealth,
-- SetStandState, SetFlag/RemoveFlag all bridged; the 2026-10-18 "not bridged"
-- note is stale);
-- SpellHit :828-871 (spell 20804 from a player on quest 6624/6622 ->
-- doctor->PatientSaved via DoctorGUID link, Talk SAY_DOC 0 — Talk IS bridged;
-- NOT_SELECTABLE / IN_COMBAT flags, stand-up, SetWalk, MovePoint to
-- A_RUNTOX/A_RUNTOY/A_RUNTOZ or H_RUNTOX/H_RUNTOY/H_RUNTOZ are not);
-- UpdateAI :873-888 (world-tick HP decay — could approximate with
-- SetHealth(GetHealth()-5) on OnAIUpdate 7, but setDeathState has no bridge
-- and the PatientDied -> quest-fail chain rides the unbridged DoAction
-- relay (FailQuest itself bridged, lua_player.go:410)).
-- The quest credit/fail legs (FailQuest bridged, lua_player.go:410;
-- AreaExploredOrEventHappens + DoAction relay unbridged) keep the
-- doctor machine out; the Reset arm below is the self-contained
-- ported fragment.

-- npc_injured_patient Reset arm — PORTED (C++ npcs_special.cpp:788-814).
-- Entries are the literal C++ switch cases (never invented): 12923/12938
-- injured (75%), 12924/12936 badly injured (50%), 12925/12937 critically
-- injured (25%). C++ arm order: RemoveFlag(UNIT_FLAG_NOT_SELECTABLE) ->
-- SetFlag(UNIT_FLAG_IN_COMBAT) -> stand-state DEAD (the SetUInt32Value
-- UNIT_FIELD_BYTES_1 write twins to creature:SetStandState) ->
-- SetHealth(CountPctFromMaxHealth(pct)) by entry. Registered on 5 (OnSpawn)
-- and 23 (OnReset); C++ Reset fires on both spawn and evade-reset.
-- (The 2026-10-18 "UNIT_STAND_STATE_DEAD / UNIT_FLAG_IN_COMBAT /
-- NOT_SELECTABLE are not [bridged]" note is stale: SetStandState bridged
-- 2026-10-21, SetFlag/RemoveFlag bridged 2026-10-23.)
local INJURED_PATIENT_75 = { [12923] = true, [12938] = true }
local INJURED_PATIENT_50 = { [12924] = true, [12936] = true }
local INJURED_PATIENT_25 = { [12925] = true, [12937] = true }
local INJURED_PATIENT_ENTRIES = { 12923, 12938, 12924, 12936, 12925, 12937 }

local function injuredPatientReset(event, creature)
    creature:RemoveFlag(59, 0x02000000) -- UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE
    creature:SetFlag(59, 0x00080000) -- UNIT_FIELD_FLAGS, UNIT_FLAG_IN_COMBAT
    creature:SetStandState(7) -- UNIT_STAND_STATE_DEAD
    local entry = creature:GetEntry()
    local pct = 0
    if INJURED_PATIENT_75[entry] then
        pct = 75
    elseif INJURED_PATIENT_50[entry] then
        pct = 50
    elseif INJURED_PATIENT_25[entry] then
        pct = 25
    end
    if pct > 0 then
        creature:SetHealth(math.floor(creature:GetMaxHealth() * pct / 100))
    end
end

for _, patientEntry in ipairs(INJURED_PATIENT_ENTRIES) do
    RegisterCreatureEvent(patientEntry, 5, injuredPatientReset)
    RegisterCreatureEvent(patientEntry, 23, injuredPatientReset)
end

-- npc_garments_of_quests (C++ :962-1123; entries 12429/12423/12427/12430/12428
-- with quests 5621/5624/5625/5648/5650, in-file enums :940-960) — DOCUMENTED-ONLY.
-- Arm: SpellHit :1002-1035 — SPELL_LESSER_HEAL_R2 2052 (not in combat, quest
-- INCOMPLETE -> stand up, Talk SAY_HEALED 2, IsHealed latch) and
-- SPELL_FORTITUDE_R1 1243 (IsHealed -> Talk SAY_THANKS 0, CanRun latch ->
-- quest credit via TalkedToCreature :1031). The Talk legs are shape-portable
-- (event 14 args (event, creature, player, spellId); Talk(textId, target) is
-- bridged with whisper target), but SetStandState stays out with the
-- rest (bridged 2026-10-21 — the arm is strand-bound, not bridge-bound):
-- the functional core — TalkedToCreature quest credit :1031 and the
-- EscortAI runaway in UpdateAI :1037-1068 (EscortAI::Start — no bridge)
-- — has none.
-- A talk-only partial would be cosmetic with no quest function, so it fails
-- the self-contained rule.

-- npc_guardian (C++ :1127-1153) — DOCUMENTED-ONLY.
-- Arms: Reset :1126-1129 (UNIT_FLAG_NON_ATTACKABLE — creature:SetFlag is
-- bridged (lua_creature_events.go:1840) but this NPC has no Lua port at
-- all: ScriptName binding only, no entry const in the C++ tree);
-- UpdateAI :1141-1148 (UpdateVictim gate; isAttackReady ->
-- DoCastVictim(SPELL_DEATHTOUCH 5, triggered); resetAttackTimer).
-- No melee-swing-timing bridge exists (isAttackReady / resetAttackTimer not
-- exposed; no creature melee event) — the attack-speed interval cannot be
-- read, and inventing a CreateLuaEvent interval would fabricate behavior.
-- No entry consts in the C++ tree.
