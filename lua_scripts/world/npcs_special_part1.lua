-- npcs_special PART 1 (C++ lines 93-1153) — triage result 2026-10-18.
-- Source: src/server/scripts/World/npcs_special.cpp.
-- VERDICT: 6 of 9 classes are DOCUMENTED-ONLY (gap classes); npc_injured_patient's
-- Reset arm is PORTED below (all legs bridged), npc_midsummer_bunny_pole is
-- PORTED below (SetData relay + ring chain) and npc_air_force_bots ALARMBOT
-- is PORTED in lua_scripts/world/npc_air_force_bots.lua. Every other class is blocked
-- either by missing entry constants (rule: entries only from C++ NPC_ consts,
-- never invented) or by missing method bridges, verified against the CURRENT
-- runtime this run (lua_creature_events.go,
-- lua_objects.go, lua_player.go, scripting/object.go).
-- Sibling file handles C++ lines 1154+.

-- npc_air_force_bots (C++ :43-188) — ALARMBOT + TRIPWIRE PORTED in
-- lua_scripts/world/npc_air_force_bots.lua (TRIPWIRE on the 2026-10-24
-- player:IsFlying bridge; 22126 refiled TRIPWIRE, see that file).
-- Entries ARE in-file (airforceSpawns table :72-98: 2614,2615,21974,21993,
-- 21996,21997,21999,22001,22002,22003,22063,22065,22066,22068,22069,22070,
-- 22071,22078,22079,22080,22086,22087,22088,22090,22124,22125,22126).
-- Arms: UpdateAI :125-150 (summon guard via SummonCreature :136, keep
-- _toAttack GUID set, guard->EngageWithTarget :147, ALARMBOT casts
-- SPELL_GUARDS_MARK 38067 on the target :149); MoveInLineOfSight :153-181
-- (TYPEID_PLAYER gate, range check :165 RANGE_ALARMBOT 100 / RANGE_TRIPWIRE
-- 15, IsHostileTo :168, IsValidAttackTarget :171, TRIPWIRE skips flying :174).
-- Bridges live since 2026-10-24: creature:IsHostileTo,
-- player:IsHostileTo, creature:IsValidAttackTarget, creature:IsEngagedBy,
-- creature:EngageWithTarget (the creature:SummonCreature bridge was already
-- live); player:IsFlying landed the same night, closing the :174 flying
-- skip and the TRIPWIRE port. The old "unit-flag manipulation" block is
-- retracted: neither arm touches unit flags.

-- npc_chicken_cluck (C++ :194-292) — DOCUMENTED-ONLY.
-- Arms: Reset :216-221 (faction FACTION_PREY, remove QUESTGIVER flag);
-- UpdateAI :224-239 (120s questgiver-flag reset — additionally blocked:
-- the expiry arm calls EnterEvadeMode(), which has NO Lua bridge
-- (boss_ai.go explicit), so the timer arm stays out even before the
-- entry ground); ReceiveEmote :241-260
-- (TEXT_EMOTE_CHICKEN 22: quest 3861 status NONE + 1/30 roll -> questgiver
-- flag + FACTION_FRIENDLY + Talk hello; TEXT_EMOTE_CHEER 21: quest COMPLETE
-- -> questgiver flag + Talk); OnQuestAccept :262-266 / OnQuestReward :268-272.
-- Missing: no entry consts in the C++ tree (DB-side scriptname binding;
-- cannot register); the FACTION_PREY<->FRIENDLY legs are load-bearing,
-- not cosmetic. Corrected 2026-10-24: player:GetQuestStatus IS bridged
-- since 2026-10-24 (lua_player.go: NONE 0 / COMPLETE 1 / INCOMPLETE 3 /
-- REWARDED 6) and CAN distinguish COMPLETE (the cheer leg) from other
-- states — the old "only HasQuest bool" claim is stale. The class stays
-- documented-only on the no-entry-consts ground alone
-- (SetFaction itself is bridged — lua_creature_events.go:1125).

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
-- SetFacingToObject bridge" parenthetical is retracted — and the
-- "no-IsWithinLOS" ground is retracted too (IsWithinLOS bridged
-- 2026-10-23, IsWithinDist + PerformEmote + CastSpell + HasAura all
-- bridged): the ReceiveEmote + UpdateAI re-dance arms are now
-- bridge-complete, and the class stays documented-only on the
-- no-entry-consts ground alone (sole hit npcs_special.cpp).
-- Emote/text-emote IDs verified from
-- src/server/shared/SharedDefines.h (TEXT_EMOTE_KISS 58, WAVE 101, BOW 17,
-- JOKE 329, DANCE 34; EMOTE_ONESHOT_DANCE 94, SHY 24, LAUGH 11) for the
-- sibling lines 1154+ run if it ever gains entry constants.

-- npc_torch_tossing_target_bunny_controller (C++ :418-452) — DOCUMENTED-ONLY.
-- Arm: Reset :434-443 schedules casts of SPELL_TORCH_TARGET_PICKER 45907 on
-- a nullptr target at 2s / 5s / 7s / 10s ... (outer Repeat 5s, inner +3s).
-- Shape is timer-portable (OnSpawn 5 / OnReset 23 + CreateLuaEvent chains),
-- but CastSpell(nil, ...) falls back to the creature's victim and is a no-op
-- without one (lua_creature_events.go:314-330, re-verified 2026-10-24)
-- — there is no null-target cast
-- bridge, so the ported timer would silently cast nothing. Doubly
-- blocked: 45907's effect is a SpellScript (spell_holiday.cpp:1543,
-- OnEffectHitTarget dummy: hit unit self-casts the indicator spells),
-- and SpellScript has zero hook kinds in runtime.go — even a
-- target-carrying cast would fire no picker effect. Also no entry
-- consts in the C++ tree (sole hit npcs_special.cpp).

-- npc_midsummer_bunny_pole (C++ :456-574) — PORTED 2026-10-24.
-- Entry IS in-tree (NPC_POLE_RIBBON_BUNNY 17066, go_scripts.cpp:797),
-- so the class registers; the old "no entry consts" ground is retracted.
-- DoAction :484-492 has no dispatch bridge, so the start rides the
-- creature:SetData relay instead: the ribbon-pole GO gossip handler
-- (go_scripts_part2.lua) writes the running latch, and the bunny's
-- event-7 latch-rise detector below starts the ring chain — the GO
-- handler and the bunny handler share the motion's ScriptData map
-- (both objects are luaMotionCreature builds), never a DoAction call.
-- UpdateAI :499-543 ports as a CreateLuaEvent chain (maiden idiom):
-- red at latch-rise (C++ 1ms schedule), then blue/red every 5000ms,
-- each step checking nearby players for aura 29726 first (C++ order)
-- and stopping (latch 0 + timers cancelled) when none has it — note
-- C++ checkNearbyPlayers returns true when the list is EMPTY, so the
-- Lua helper below is named for the aura presence, not the C++ name.
-- Casts land on GO_RIBBON_POLE 181605 via FindNearestGameObject(10).
local POLE_BUNNY_ENTRY = 17066
local GO_RIBBON_POLE = 181605
local SPELL_RIBBON_DANCE_COSMETIC = 29726
local SPELL_RED_FIRE_RING = 46836
local SPELL_BLUE_FIRE_RING = 46842
local DATA_BUNNY_RUNNING = 1

local ringTimers = {}
local ringRunning = {}

local function cancelRingTimers(guid)
    local per = ringTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        ringTimers[guid] = nil
    end
end

local function scheduleRing(guid, key, delay, fn)
    local per = ringTimers[guid]
    if not per then
        per = {}
        ringTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function hasRibbonDancer(bunny)
    for _, player in ipairs(bunny:GetPlayersInGrid(10)) do
        if player:HasAura(SPELL_RIBBON_DANCE_COSMETIC) then
            return true
        end
    end
    return false
end

local onBlueRing

local function onRedRing(bunny, guid)
    if bunny:GetData(DATA_BUNNY_RUNNING) ~= 1 then
        return
    end
    if not hasRibbonDancer(bunny) then
        bunny:SetData(DATA_BUNNY_RUNNING, 0)
        cancelRingTimers(guid)
        return
    end
    local pole = bunny:FindNearestGameObject(GO_RIBBON_POLE, 10)
    if pole then
        bunny:CastSpell(pole, SPELL_RED_FIRE_RING, true)
    end
    scheduleRing(guid, "blue", 5000, function() onBlueRing(bunny, guid) end)
end

onBlueRing = function(bunny, guid)
    if bunny:GetData(DATA_BUNNY_RUNNING) ~= 1 then
        return
    end
    if not hasRibbonDancer(bunny) then
        bunny:SetData(DATA_BUNNY_RUNNING, 0)
        cancelRingTimers(guid)
        return
    end
    local pole = bunny:FindNearestGameObject(GO_RIBBON_POLE, 10)
    if pole then
        bunny:CastSpell(pole, SPELL_BLUE_FIRE_RING, true)
    end
    scheduleRing(guid, "red", 5000, function() onRedRing(bunny, guid) end)
end

local function onBunnyAIUpdate(event, bunny)
    local guid = bunny:GetGUID()
    if bunny:GetData(DATA_BUNNY_RUNNING) == 1 then
        if not ringRunning[guid] then
            ringRunning[guid] = true
            onRedRing(bunny, guid)
        end
    else
        ringRunning[guid] = nil
    end
end

local function onBunnyReset(event, bunny)
    bunny:SetData(DATA_BUNNY_RUNNING, 0)
    cancelRingTimers(bunny:GetGUID())
    ringRunning[bunny:GetGUID()] = nil
end

RegisterCreatureEvent(POLE_BUNNY_ENTRY, 7, onBunnyAIUpdate)
RegisterCreatureEvent(POLE_BUNNY_ENTRY, 23, onBunnyReset)
RegisterCreatureEvent(POLE_BUNNY_ENTRY, 4, onBunnyReset)

-- npc_doctor (C++ :583-772; Gustav Vanhowzen 12939 / Gregory Victor 12920,
-- in-file enum Doctor :534-541) -- PORTED below (Lua-state twin of the
-- summon-machine; the old "DOCUMENTED-ONLY" verdict is retracted).
-- Ported arms: Reset :646-650 (state clear + RemoveFlag NOT_SELECTABLE,
-- events 23/4); BeginEvent :655-683 via OnQuestAccept event 31 (quests
-- 6624/6622; coords copied per entry, SetFlag NOT_SELECTABLE); UpdateAI
-- :886-946 via event-7 pump (100 ticks = C++ 10s timer; 20-summon cap
-- resets; empty-coords tick retries without resetting, C++-exact);
-- PatientSaved :710-746 via patient SpellHit event 14 (15th save:
-- remaining patients Despawned, AreaExploredOrEventHappens on the
-- INCOMPLETE quest -- new player bridge this tick, lua_player.go --
-- then doctor Reset); patient SpellHit :828-871 flags/stand/Talk legs
-- (NOT_SELECTABLE on, IN_COMBAT off, SetStandState(0), Talk SAY_DOC 0).
-- Deviations (documented, no bridge): the 15-save mass-death is Despawn,
-- not setDeathState JUST_DIED (no bridge); SetWalk(false) + MovePoint
-- run-to legs stay out (no SetWalk/MovePoint bridges); the PatientDied
-- fail-after-5 path stays out (Lua patients floor at 2-6 HP on the
-- ported decay arm, never reaching the C++ death arm, so it cannot fire).
-- The cross-AI DoctorGUID/Coord write is modeled as module tables keyed
-- by GUID (doctorState + patientDoctor), the timers-table idiom.
-- Summon args are C++-exact: type 3 TIMED_DESPAWN_OUT_OF_COMBAT, 5000ms.

-- npc_injured_patient (C++ :778-890; entries 12923/12924/12925,
-- 12936/12937/12938 in-file) — Reset arm PORTED below; rest DOCUMENTED-ONLY.
-- Arms: Reset :788-814 PORTED (75/50/25% health by entry — SetHealth,
-- SetStandState, SetFlag/RemoveFlag all bridged; the 2026-10-18 "not bridged"
-- note is stale);
-- SpellHit :828-871 (spell 20804 from a player on quest 6624/6622 ->
-- doctor->PatientSaved via DoctorGUID link, Talk SAY_DOC 0 — Talk IS bridged;
-- NOT_SELECTABLE / IN_COMBAT flags, stand-up, SetWalk, MovePoint to
-- A_RUNTOX/A_RUNTOY/A_RUNTOZ or H_RUNTOX/H_RUNTOY/H_RUNTOZ are not);
-- UpdateAI :873-888 first arm PORTED below (world-tick HP decay:
-- health > 6 -> SetHealth(GetHealth()-5) on OnAIUpdate 7, C++-exact;
-- the health <= 6 death arm stays out -- setDeathState has no bridge
-- and the PatientDied -> quest-fail chain rides the unbridged DoAction
-- relay (FailQuest itself bridged, lua_player.go:410)).
-- The quest credit/fail legs (FailQuest bridged, lua_player.go:410;
-- AreaExploredOrEventHappens + DoAction relay unbridged) keep the
-- doctor machine out; the Reset + UpdateAI-decay arms below are the
-- self-contained ported fragments.

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

-- npc_injured_patient UpdateAI decay arm -- PORTED (C++ npcs_special.cpp:873-878, first arm only).
-- C++ arm order: alive and health > 6 -> ModifyHealth(-5); the health <= 6
-- death arm (RemoveFlag IN_COMBAT, SetFlag NOT_SELECTABLE, setDeathState
-- JUST_DIED, dynamic flag 32, doctor PatientDied) stays out -- no
-- setDeathState bridge, and the PatientDied chain rides the unbridged
-- DoAction relay -- so patients floor at 2-6 HP instead of dying.
-- Event 7 fires on the 100ms world tick for every alive motion
-- (creaturemotion.go), matching the C++ per-tick decay; returns
-- nothing, so no veto of the engine combat step.
local function injuredPatientDecay(event, creature)
    if creature:IsDead() then
        return
    end
    local health = creature:GetHealth()
    if health > 6 then
        creature:SetHealth(health - 5)
    end
end

for _, patientEntry in ipairs(INJURED_PATIENT_ENTRIES) do
    RegisterCreatureEvent(patientEntry, 7, injuredPatientDecay)
end

-- npc_doctor machine -- PORTED (C++ npcs_special.cpp:583-772, :886-946).
-- State lives in module tables keyed by GUID because Lua has no cross-AI
-- write: doctorState[doctorGUID] is the doctor AI (playerGUID, coords
-- pool, count, saved, ticks, patients); patientDoctor[patientGUID] is
-- the C++ Patient->AI()->DoctorGUID link. The doctor object itself is
-- kept in state.doctorObj for the 15-save Reset flag clear (opera
-- live-ref precedent); doctors never despawn mid-event.
local DOCTOR_ALLIANCE = 12939
local DOCTOR_HORDE = 12920
local DOCTOR_ENTRIES = { 12939, 12920 }
local QUEST_TRIAGE_ALLIANCE = 6624
local QUEST_TRIAGE_HORDE = 6622
local QUEST_INCOMPLETE = 3
local SPELL_TRIAGE_HEAL = 20804

local TRIAGE_ALLIANCE_COORDS = {
    { -3757.38, -4533.05, 14.16, 3.62 },
    { -3754.36, -4539.13, 14.16, 5.13 },
    { -3749.54, -4540.25, 14.28, 3.34 },
    { -3742.10, -4536.85, 14.28, 3.64 },
    { -3755.89, -4529.07, 14.05, 0.57 },
    { -3749.51, -4527.08, 14.07, 5.26 },
    { -3746.37, -4525.35, 14.16, 5.22 },
}
local TRIAGE_HORDE_COORDS = {
    { -1013.75, -3492.59, 62.62, 4.34 },
    { -1017.72, -3490.92, 62.62, 4.34 },
    { -1015.77, -3497.15, 62.82, 4.34 },
    { -1019.51, -3495.49, 62.82, 4.34 },
    { -1017.25, -3500.85, 62.98, 4.34 },
    { -1020.95, -3499.21, 62.98, 4.34 },
}
local TRIAGE_ALLIANCE_SOLDIERS = { 12938, 12936, 12937 }
local TRIAGE_HORDE_SOLDIERS = { 12923, 12924, 12925 }

local TRIAGE_FLAG_NOT_SELECTABLE = 0x02000000
local TRIAGE_FLAG_IN_COMBAT = 0x00080000
local TRIAGE_FLAG_PLAYER_CONTROLLED = 0x00000008

local doctorState = {}
local patientDoctor = {}

local function doctorClearState(doctorGUID)
    local state = doctorState[doctorGUID]
    if state then
        for patientGUID, _ in pairs(state.patients) do
            patientDoctor[patientGUID] = nil
        end
    end
    doctorState[doctorGUID] = nil
end

-- C++ Reset :646-650 (Initialize + RemoveFlag NOT_SELECTABLE).
local function doctorReset(event, creature)
    doctorClearState(creature:GetGUID())
    creature:RemoveFlag(59, TRIAGE_FLAG_NOT_SELECTABLE)
end

-- Doctor death has no C++ arm; clear the Lua state so a stale table
-- cannot resurrect the event.
local function doctorDied(event, creature)
    doctorClearState(creature:GetGUID())
end

-- C++ BeginEvent :655-683, fired from OnQuestAccept :768-772.
local function doctorQuestAccept(event, player, creature, quest)
    if not quest then
        return
    end
    local questId = quest:GetId()
    if questId ~= QUEST_TRIAGE_ALLIANCE and questId ~= QUEST_TRIAGE_HORDE then
        return
    end
    local entry = creature:GetEntry()
    local coords = nil
    if entry == DOCTOR_ALLIANCE then
        coords = TRIAGE_ALLIANCE_COORDS
    elseif entry == DOCTOR_HORDE then
        coords = TRIAGE_HORDE_COORDS
    else
        return
    end
    local pool = {}
    for i, coord in ipairs(coords) do
        pool[i] = coord
    end
    doctorState[creature:GetGUID()] = {
        doctorObj = creature,
        playerGUID = player:GetGUID(),
        coords = pool,
        count = 0,
        saved = 0,
        ticks = 0,
        patients = {},
    }
    creature:SetFlag(59, TRIAGE_FLAG_NOT_SELECTABLE)
end

-- C++ UpdateAI :886-946. Event 7 ticks every 100ms, so 100 ticks is the
-- C++ 10s SummonPatientTimer; the timer reset + count increment happen
-- even on a failed summon (C++ order), while an empty coords pool
-- returns without resetting (next tick retries).
local function doctorAIUpdate(event, creature)
    local state = doctorState[creature:GetGUID()]
    if not state then
        return
    end
    if state.count >= 20 then
        doctorReset(event, creature)
        return
    end
    state.ticks = state.ticks + 1
    if state.ticks < 100 then
        return
    end
    if #state.coords == 0 then
        return
    end
    local soldiers = TRIAGE_HORDE_SOLDIERS
    if creature:GetEntry() == DOCTOR_ALLIANCE then
        soldiers = TRIAGE_ALLIANCE_SOLDIERS
    end
    local patientEntry = soldiers[math.random(#soldiers)]
    local idx = math.random(#state.coords)
    local coord = state.coords[idx]
    local patient = creature:SummonCreature(patientEntry, coord[1], coord[2], coord[3], coord[4], 3, 5000)
    if patient then
        patient:SetFlag(59, TRIAGE_FLAG_PLAYER_CONTROLLED)
        local patientGUID = patient:GetGUID()
        state.patients[patientGUID] = { coord = coord, obj = patient }
        patientDoctor[patientGUID] = creature:GetGUID()
        table.remove(state.coords, idx)
    end
    state.ticks = 0
    state.count = state.count + 1
end

-- C++ PatientSaved :710-746. Returns true when the event completed
-- (15th save), so the caller skips the SpellHit tail: C++ runs that
-- tail on a JUST_DIED corpse (a visual no-op), while Lua already
-- despawned the patient and must not call methods on it.
local function doctorPatientSaved(state, doctorGUID, patientGUID, player)
    state.saved = state.saved + 1
    local record = state.patients[patientGUID]
    if state.saved == 15 then
        for guid, rec in pairs(state.patients) do
            if rec.obj then
                rec.obj:Despawn()
            end
            patientDoctor[guid] = nil
        end
        if player:GetQuestStatus(QUEST_TRIAGE_ALLIANCE) == QUEST_INCOMPLETE then
            player:AreaExploredOrEventHappens(QUEST_TRIAGE_ALLIANCE)
        elseif player:GetQuestStatus(QUEST_TRIAGE_HORDE) == QUEST_INCOMPLETE then
            player:AreaExploredOrEventHappens(QUEST_TRIAGE_HORDE)
        end
        local doctorObj = state.doctorObj
        doctorState[doctorGUID] = nil
        if doctorObj then
            doctorObj:RemoveFlag(59, TRIAGE_FLAG_NOT_SELECTABLE)
        end
        return true
    end
    if record then
        table.insert(state.coords, record.coord)
    end
    return false
end

-- C++ npc_injured_patient SpellHit :828-871. The caster->ToPlayer()
-- gate is the GetQuestStatus-function check (object.go returns nil for
-- unknown methods, so a creature caster reads nil, not an error).
-- The flags/stand/Talk tail runs even without a doctor link or quest
-- match (C++ order); only the 15-save completion skips it (see above).
-- SetWalk(false) + MovePoint run-to stay out: no bridges.
local function patientSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_TRIAGE_HEAL then
        return
    end
    if creature:IsDead() then
        return
    end
    if caster == nil or type(caster.GetQuestStatus) ~= "function" then
        return
    end
    local doctorGUID = patientDoctor[creature:GetGUID()]
    local eventDone = false
    if doctorGUID then
        local state = doctorState[doctorGUID]
        if state and caster:GetGUID() == state.playerGUID then
            if caster:GetQuestStatus(QUEST_TRIAGE_ALLIANCE) == QUEST_INCOMPLETE or caster:GetQuestStatus(QUEST_TRIAGE_HORDE) == QUEST_INCOMPLETE then
                eventDone = doctorPatientSaved(state, doctorGUID, creature:GetGUID(), caster)
            end
        end
    end
    if eventDone then
        return
    end
    creature:SetFlag(59, TRIAGE_FLAG_NOT_SELECTABLE)
    creature:RemoveFlag(59, TRIAGE_FLAG_IN_COMBAT)
    creature:SetStandState(0)
    creature:Talk(0)
end

for _, doctorEntry in ipairs(DOCTOR_ENTRIES) do
    RegisterCreatureEvent(doctorEntry, 31, doctorQuestAccept)
    RegisterCreatureEvent(doctorEntry, 7, doctorAIUpdate)
    RegisterCreatureEvent(doctorEntry, 23, doctorReset)
    RegisterCreatureEvent(doctorEntry, 4, doctorDied)
end

for _, patientEntry in ipairs(INJURED_PATIENT_ENTRIES) do
    RegisterCreatureEvent(patientEntry, 14, patientSpellHit)
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
-- Re-keyed 2026-10-24: the swing-readiness DATA exists (engine tick
-- swings when now-LastAttack >= UNIT_FIELD_BASEATTACKTIME x aura factor,
-- creaturemotion.go) but no Lua bridge exposes it (isAttackReady /
-- resetAttackTimer unexposed) — and a bridge alone would unlock nothing:
-- no entry const in the C++ tree means no registration, and the port
-- would also need NoMeleeAttack suppression (the C++ UpdateAI never
-- melee-swings; the Go engine tick would swing alongside the cast).
-- Inventing a CreateLuaEvent interval would fabricate behavior.
