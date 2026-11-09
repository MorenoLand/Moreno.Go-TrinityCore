-- Kel'Thuzad (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_kelthuzad.cpp
-- (boss_kelthuzad (BossAI, BOSS_KELTHUZAD); npc_kelthuzad_skeleton /
-- npc_kelthuzad_banshee / npc_kelthuzad_abomination (ScriptedAI via the
-- shared npc_kelthuzad_minionAI base); npc_kelthuzad_guardian
-- (ScriptedAI); spell_kelthuzad_chains / spell_kelthuzad_detonate_mana
-- (AuraScripts via RegisterSpellScript); spell_kelthuzad_frost_blast
-- (AuraScript, direct registration); at_kelthuzad_center
-- (AreaTriggerScript); achievement_just_cant_get_enough
-- (AchievementCriteriaScript); AddSC_boss_kelthuzad registers all ten;
-- loader decl 64 / call 259 per northrend_script_loader.cpp — the
-- SIXTH Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_boss_razuvious(), under the "// Naxxramas" marker; the call
-- after it is AddSC_boss_loatheb()).
-- Entry: 15990 Kel'Thuzad (naxxramas.h NPC_KEL_THUZAD — kalecgos pass;
-- instance_naxxramas.cpp OnCreatureCreate binds NPC_KEL_THUZAD ->
-- KelthuzadGUID and GetGuidData(DATA_KELTHUZAD) returns it; the
-- CreatureScript ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual). The minion/guardian
-- entries (NPC_SKELETON1 16427 / NPC_SKELETON2 23561 / NPC_ABOMINATION1
-- 16428 / NPC_ABOMINATION2 23562 / NPC_BANSHEE1 16429 / NPC_BANSHEE2
-- 23563 / NPC_GUARDIAN 16441) live only in this file's local enum — no
-- NPC_ constants in naxxramas.h, no instance bindings — ScriptName->
-- entry binding is DB-side, so nothing registers for them (grobbulus
-- precedent).
-- Sole-source verified: whole-server-tree grep for "boss_kelthuzad",
-- "npc_kelthuzad_skeleton", "npc_kelthuzad_banshee",
-- "npc_kelthuzad_abomination", "npc_kelthuzad_guardian",
-- "spell_kelthuzad_chains", "spell_kelthuzad_detonate_mana",
-- "spell_kelthuzad_frost_blast", "at_kelthuzad_center",
-- "achievement_just_cant_get_enough", "KelThuzadCharmedPlayerAI" hits
-- boss_kelthuzad.cpp only (loader carries only the decl/call lines);
-- zero sql/ hits. No kelthuzad lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied. Melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ has no JustEngagedWith override for Kel'Thuzad (SAY_AGGRO 7
-- fires on the EVENT_DESPAWN_MINIONS timer leg, not on combat start),
-- so the inherited BossAI::JustEngagedWith runs at first AtEngage
-- (ScriptedCreature.h:329): SetCombatPulseDelay(5) + me-form DoZoneInCombat
-- PORTED 2026-11-03 (ScriptedCreature.cpp:540/542; anubrekhan/gothik/
-- sapphiron precedent); SAY_AGGRO still rides EVENT_DESPAWN_MINIONS, not
-- combat start; instance legs + setActive unbridged. Timers are modeled
-- from the BEGIN_ENCOUNTER receiver (2026-11-05) and cancelled on
-- events 2/4/23.
-- Victim gating: C++ KilledUnit checks TYPEID_PLAYER (nalorakk
-- precedent); Lua expresses it via victim:GetObjectType().
-- Ported arms (C++-exact for all modeled arms):
-- boss_kelthuzad: KilledUnit Talk(SAY_SLAY 8) (event 3 — C++-GATED:
-- Talk fires only when victim->GetTypeId() == TYPEID_PLAYER —
-- nalorakk precedent); JustDied Talk(SAY_DEATH 9) (event 4; the
-- JustDied guardian relay (ACTION_KELTHUZAD_DIED) is PORTED 2026-11-04
-- via kelthuzadGuardianFlee; the _JustDied instance bookkeeping has
-- no bridge — tharon_ja precedent).
-- Unmodeled (no bridges — documented, not wired):
-- SpellHit — PORTED 2026-11-05 in kelthuzadSpellHit (event 14):
-- SPELL_CHAINS_DUMMY 28408 -> Talk(SAY_CHAINS 10) + 3 random players
-- -> DoCast(CHAINS 28410) each; the trigger is the now-ported
-- phase-two EVENT_CHAINS self-cast (ev14 fires on the self-hit,
-- boss_ai.go:3828); latent until the 28408 spell data resolves in Go.
-- EVENT_SKELETON / EVENT_BANSHEE / EVENT_ABOMINATION — PORTED
-- 2026-11-05 as receiver-seeded CreateLuaEvent timers (skeletonTick /
-- bansheeTick / abominationTick): the re-triage arithmetic is verified
-- against the C++ counts — skeleton last fire 206s (count 72),
-- banshee/abom last fire 200s (count 9), vs DESPAWN_MINIONS 213s /
-- PHASE_TWO 228s, so the phase gate is vacuous in the normal flow;
-- the _skeletonCount==1 leg arms a random pre-existing room skeleton
-- (23561, 200yd) instead of summoning; the empty-list EnterEvadeMode
-- leg has no bridge, so the chain stops with no re-arm (documented
-- in the tick).
-- EVENT_DESPAWN_MINIONS — PORTED 2026-11-05 in despawnMinionsTick
-- (seeded 3m33s from the BEGIN_ENCOUNTER receiver, latent with it):
-- temp-vector sweep of the latched summons table, DespawnOrUnsummon
-- on each non-combat summon + Talk(SAY_AGGRO 7), C++ order
-- (boss_kelthuzad.cpp:429-441); EVENT_PHASE_TWO — PORTED 2026-11-05 in
-- phaseTwoTick (seeded 3m48s from the BEGIN_ENCOUNTER receiver):
-- StopSpellCast + RemoveFlag NOT_SELECTABLE + RemoveFlag immune +
-- ResetAllThreat + SetReactState(AGGRESSIVE) + Talk(EMOTE_PHASE_TWO
-- 13) + the five phase-two schedules; SetPhase itself has no bridge
-- but no arm reads phase state (gothik phase-gating verdict retired —
-- the chains self-terminate before the switch, so the machine is fully
-- receiver-seeded). EVENT_FROSTBOLT_VOLLEY / EVENT_SHADOW_FISSURE /
-- EVENT_DETONATE_MANA / EVENT_FROST_BLAST / EVENT_CHAINS — PORTED
-- 2026-11-05 (volleyTick / fissureTick / detonateTick / frostBlastTick
-- / chainsTick): DoCastAOE self-casts, thorim random-player picks
-- (frost blast position 1 = netherspite victim exclusion, UnitAI.h
-- :162-163), mana-user filter via GetPowerType() == POWER_MANA
-- (moroes precedent), chains 25-man-gated; all defer 500ms on
-- IsCasting (firemaw idiom). The Oct-12 "no DoCastAOE bridge" and
-- "no random-target bridge" claims are superseded/retracted.
-- EVENT_TRANSITION_REPLY — PORTED 2026-11-05 (lich-king Talk leg
-- only): Talk(SAY_ANSWER_REQUEST 3) via GetInstanceCreature
-- (DATA_LICH_KING 20); the four KT portal GO states stay out (no
-- GO-state bridge). EVENT_TRANSITION_SUMMON — PORTED 2026-11-05 in
-- transitionSummonTick: the guardian-group permutation (urand swap)
-- + SummonCreatureGroup + DoAction(ACTION_JUST_SUMMONED) per guardian
-- (the guardian ev38 receiver ports below).
-- phase-three UpdateAI leg — PORTED 2026-11-05 in phaseThreePoll (1s
-- poll from phase two): HealthBelowPct(45) one-shot latch ->
-- Talk(SAY_REQUEST_AID 12) + transition scheduling.
-- UpdateAI frostbolt filler — PORTED 2026-11-05 in frostboltTick:
-- DoCastVictim(SPELL_FROSTBOLT_SINGLE 28478), seeded 2s after phase
-- two, 3s repeat, IsCasting-gated; the phase-one filler is a silent
-- C++ no-op (DoCast fails while channeling) and is not seeded.
-- DamageTaken — PORTED 2026-11-05 in kelthuzadDamageTaken (event 9):
-- damage = 0 in PHASE_ONE via the phase-two latch (return false, 0
-- rewrite, malchezaar precedent). Reset (boss_kelthuzad.cpp:223-235) —
-- SetReactState(REACT_PASSIVE) + NOT_SELECTABLE + SetImmuneToPC(true) —
-- the bridges exist (creature:SetReactState; SetFlag/RemoveFlag(59) for
-- the immunity pair, 07:47 sweep) but Reset itself is unported (the
-- whole encounter is intro/DoAction-machine-gated), so the legs stay
-- out with it; EnterEvadeMode — portal GO-state legs — instance model;
-- DoAction(ACTION_BEGIN_ENCOUNTER) — PORTED 2026-11-05 in
-- kelthuzadDoAction (event 38; latent — the at_kelthuzad_center
-- sender has no area-trigger bridge, krick precedent): NOT_STARTED
-- gate (TO_BE_DECIDED 5 treated as the unset twin of NOT_STARTED) +
-- SetImmuneToPC(false) via RemoveFlag(59, 0x100) + SetBossState
-- IN_PROGRESS + DoZoneInCombat + DoCastAOE(SPELL_VISUAL_CHANNEL
-- 29423) self-cast + Talk(SAY_SUMMON_MINIONS 14) + SummonCreatureGroup
-- groups 5-11 with REACT_PASSIVE + SetData(DATA_MINION_POCKET_ID,
-- group) per summon. The SetPhase(PHASE_ONE) leg has no bridge, but
-- all five schedules it feeds are seeded in the receiver 2026-11-05
-- (skeleton 5s, banshee 30s, abomination 30s, despawn-minions 3m33s,
-- phase-two 3m48s) — no arm reads phase state.
-- DoAction(ACTION_ABOMINATION_DIED) — PORTED 2026-11-05: the
-- abomination JustDied relay (ev4 on 16428) calls kt:DoAction(2) via
-- GetInstanceCreature(DATA_KELTHUZAD 14) and the receiver increments
-- the SetData(DATA_ABOMINATION_DEATH_COUNT 1) latch (Reset zeroes it);
-- the achievement_just_cant_get_enough consumer (>= 18) stays out —
-- no bridge for that ScriptName.
-- GetAIForCharmedPlayer -> KelThuzadCharmedPlayerAI
-- (SimpleCharmedPlayerAI) — no charmed-player AI bridge.
-- npc_kelthuzad_skeleton / npc_kelthuzad_banshee /
-- npc_kelthuzad_abomination (ScriptedAI; summoned entries 16427 /
-- 16429 / 16428 — C++-sourced, boss_kelthuzad.cpp:135-138, registered
-- in boss_ai.go 2026-11-05, guardian precedent; pre-existing 23561 /
-- 23563 / 23562 stay DB-side, unregistered): JustEngagedWith
-- pocket-aggro PORTED 2026-11-05 in kelthuzadMinionEnterCombat (ev1;
-- non-passive -> DoZoneInCombat, pocket sweep via GetCreaturesInGrid
-- 80yd + GetData pocket match + SetReactState(AGGRESSIVE) +
-- AttackStart; react-state read is the new creature:GetReactState
-- bridge); abomination _woundTimer DoCastVictim(SPELL_MORTAL_WOUND
-- 28467) PORTED 2026-11-05 (ev1-seeded 10-20s, 14-18s re-arm,
-- victim-gated with 5s re-poll); abomination JustDied relay PORTED
-- 2026-11-05 (ev4 -> kt:DoAction(2), see DoAction below);
-- UpdateRandomMovement hack PORTED 2026-11-07 (tick 394, wanderTick /
-- kelthuzadMinionMovementInform, ev28 live tick 392): ctor 4-12s seed
-- (seedMinionWander, BEGIN_ENCOUNTER receiver only — fresh spawn-tick
-- summons DoZoneInCombat at once, which zeroes the timer in C++ via
-- AtEngage -> JustEngagedWith), MoveTo(1, circle point, 3.0yd) + ev28
-- re-arm {2s,10s}+{2s,10s}; ev1 zeroes the timer (C++
-- JustEngagedWith _movementTimer = 0). MoveInLineOfSight 20yd aggro leg
-- stays out (ev27 is engage-only; the ev7-poll twin used by runthak
-- is not seeded here — the ev1 pocket arm covers the engage path);
-- SetData(DATA_MINION_POCKET_ID) is bridged (creature:SetData,
-- per-motion latch) and is written by the ACTION_BEGIN_ENCOUNTER
-- receiver below. npc_kelthuzad_guardian (16441 — entry IS C++-sourced,
-- boss_kelthuzad.cpp:139 NPC_GUARDIAN; the old "entry-unverifiable"
-- claim was wrong): ACTION_JUST_SUMMONED receiver PORTED below
-- (kelthuzadGuardianDoAction, event 38 — SetVisible(false) +
-- SetHomePosition + DoZoneInCombat + SetCombatPulseDelay(5) + 2s
-- visibility timer + 25s blood-tap timer; latent until the
-- EVENT_TRANSITION_SUMMON machine lands — the producer bridge
-- (creature:SummonCreatureGroup, 2026-11-05 tick 238) is live but the
-- transition event machine is unported, krick-receiver precedent);
-- UpdateAI _bloodTapTimer DoCastVictim(SPELL_BLOOD_TAP 28470) rides
-- the receiver's seeded timer (urand 18s,26s re-arm);
-- ACTION_KELTHUZAD_DIED — PORTED 2026-11-04 in kelthuzadDied via
-- kelthuzadGuardianFlee (sender is kelthuzad JustDied, ev4-registered,
-- not the guardian's DoAction receipt; MoveHome is the MoveTargetedHome
-- twin, lua_creature_events.go:4278); the old "no bridge" claims for
-- this arm are stale.
-- EMOTE_GUARDIAN_APPEAR 1 rides the DoAction visibility timer,
-- not a bridge gap.
-- JustReachedHome DespawnOrUnsummon (boss_kelthuzad.cpp:832-835):
-- ev24 is live, but unreachable — MoveTargetedHome never runs,
-- so home-arrival never fires; trigger-chain-blocked, not
-- entry-blocked.
-- spell_kelthuzad_chains (AuraScript 28410): scale-mod apply/remove
-- handlers — no AuraScript binding bridge (razelikh precedent) — joins
-- the SpellScript/AuraScript queue.
-- spell_kelthuzad_detonate_mana (AuraScript 27819): periodic mana-drain
-- -> SPELL_MANA_DETONATION_DAMAGE 27820 — PORTED below via
-- RegisterAuraEvent ev1 (detonateManaTick).
-- spell_kelthuzad_frost_blast (AuraScript 27808): PeriodicTick ->
-- SPELL_FROST_BLAST_DMG 29879 with 26%-max-health scaling — PORTED
-- below via RegisterAuraEvent ev1 (frostBlastAuraTick).
-- at_kelthuzad_center (AreaTriggerScript): NOT_STARTED-gated
-- DoAction(ACTION_BEGIN_ENCOUNTER) via DATA_KELTHUZAD — no
-- area-trigger bridge (anubrekhan precedent) — documented-only; the
-- receiver is PORTED latent in kelthuzadDoAction (event 38).
-- achievement_just_cant_get_enough: DATA_ABOMINATION_DEATH_COUNT >= 18
-- via DATA_KELTHUZAD GetData — no achievement bridge — documented-only.

local ENTRY_KEL_THUZAD = 15990
local ENTRY_GUARDIAN = 16441 -- boss_kelthuzad.cpp:139 NPC_GUARDIAN
local EMOTE_GUARDIAN_FLEE = 0
local EMOTE_GUARDIAN_APPEAR = 1 -- boss_kelthuzad.cpp:848
local ACTION_JUST_SUMMONED = 1 -- boss_kelthuzad.cpp:76 (Actions enum)
local ACTION_BEGIN_ENCOUNTER = 0 -- boss_kelthuzad.cpp:74 (Actions enum)
local BOSS_KELTHUZAD = 14 -- naxxramas.h NAXEncounter (0-based: ANUBREKHAN 0 .. SAPPHIRON 13, KELTHUZAD 14)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)
local SPELL_VISUAL_CHANNEL = 29423 -- boss_kelthuzad.cpp:89
local SAY_SUMMON_MINIONS = 14 -- boss_kelthuzad.cpp:40
local SUMMON_GROUP_MINION_FIRST = 5 -- boss_kelthuzad.cpp:116
local N_MINION_GROUPS = 7 -- boss_kelthuzad.cpp:112
local DATA_MINION_POCKET_ID = 0 -- boss_kelthuzad.cpp:82 (KTData enum, first)
local SPELL_BLOOD_TAP = 28470 -- boss_kelthuzad.cpp:108
local ENTRY_SKELETON = 16427 -- boss_kelthuzad.cpp:135 NPC_SKELETON1
local ENTRY_ABOMINATION = 16428 -- boss_kelthuzad.cpp:137 NPC_ABOMINATION1
local ENTRY_BANSHEE = 16429 -- boss_kelthuzad.cpp:138 NPC_BANSHEE1
local ENTRY_SKELETON2 = 23561 -- boss_kelthuzad.cpp:134 NPC_SKELETON2 (pre-existing room skeleton)
local SPELL_FROSTBOLT_SINGLE = 28478 -- boss_kelthuzad.cpp:92
local SPELL_FROSTBOLT_VOLLEY = 28479 -- boss_kelthuzad.cpp:93
local SPELL_SHADOW_FISSURE = 27810 -- boss_kelthuzad.cpp:94
local SPELL_DETONATE_MANA = 27819 -- boss_kelthuzad.cpp:95
local SPELL_MANA_DETONATION_DAMAGE = 27820 -- boss_kelthuzad.cpp:96
local SPELL_FROST_BLAST = 27808 -- boss_kelthuzad.cpp:97
local SPELL_FROST_BLAST_DMG = 29879 -- boss_kelthuzad.cpp:98
local SPELL_CHAINS = 28410 -- boss_kelthuzad.cpp:99
local SPELL_CHAINS_DUMMY = 28408 -- boss_kelthuzad.cpp:100
local SAY_CHAINS = 10 -- boss_kelthuzad.cpp:37
local SAY_REQUEST_AID = 12 -- boss_kelthuzad.cpp:38
local EMOTE_PHASE_TWO = 13 -- boss_kelthuzad.cpp:39
local SAY_ANSWER_REQUEST = 3 -- boss_kelthuzad.cpp:44 (Lich King)
local DATA_LICH_KING = 20 -- naxxramas.h NAXData64 (0-based)
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local UNIT_FLAG_IMMUNE_TO_PC = 0x100 -- UnitDefines.h:132
local POWER_MANA = 0 -- moroes/mar'li precedent
local SUMMON_GROUP_GUARDIAN_FIRST = 1 -- boss_kelthuzad.cpp:115
local N_GUARDIAN_SPAWNS = 4 -- boss_kelthuzad.cpp:111
-- Per-motion count latches (KTData twin; C++ GetData handles only index 1,
-- tick-235 convention — absent reads 0).
local DATA_SKELETON_COUNT = 2
local DATA_BANSHEE_COUNT = 3
local DATA_ABOMINATION_COUNT = 4
local DATA_PHASE_THREE = 5
local DATA_GUARDIAN_COUNT = 6
local DATA_PHASE_TWO = 7 -- set by phaseTwoTick; the DamageTaken leg reads it
-- C++ minionSpawnPoints (boss_kelthuzad.cpp:143-151), 7 points, z 143.65.
local MINION_SPAWN_POINTS = {
    { 3768.40, -5072.00, 143.65 },
    { 3729.30, -5044.10, 143.65 },
    { 3683.00, -5054.05, 143.65 },
    { 3654.15, -5093.48, 143.65 },
    { 3664.55, -5140.50, 143.65 },
    { 3704.00, -5170.00, 143.65 },
    { 3751.95, -5158.90, 143.65 },
}
local ACTION_ABOMINATION_DIED = 2 -- boss_kelthuzad.cpp:77 (Actions enum)
local DATA_KELTHUZAD = 14 -- naxxramas.h NAXData64 (0-based: ANUBREKHAN 0 .. KELTHUZAD 14)
local DATA_ABOMINATION_DEATH_COUNT = 1 -- boss_kelthuzad.cpp:83 (KTData enum, second)
local SPELL_MORTAL_WOUND = 28467 -- boss_kelthuzad.cpp:105
local SAY_AGGRO = 7 -- boss_kelthuzad.cpp:33
local REACT_PASSIVE = 0 -- UnitDefines.h
local REACT_AGGRESSIVE = 2 -- UnitDefines.h
local MOVEMENT_MINION_RANDOM = 1 -- boss_kelthuzad.cpp:128 (Movements enum)

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ GetRandomPositionOnCircle (boss_kelthuzad.cpp:158-165):
-- uniform angle, relDistance = rand+rand folded to [0,1], radius 3.0,
-- z = center z (the @hack note :589-592: the minion-cove floor is flat,
-- so z is never recomputed).
local function randomPositionOnCircle(hx, hy, hz, radius)
    local angle = math.random() * 2 * math.pi
    local relDistance = math.random() + math.random()
    if relDistance > 1 then
        relDistance = 1 - relDistance
    end
    return hx + math.sin(angle) * relDistance * radius, hy + math.cos(angle) * relDistance * radius, hz
end

-- Forward declaration (tharon_ja/gal_darah idiom): wanderTick is
-- referenced by seedMinionWander / kelthuzadMinionMovementInform
-- before its definition below; assigned by plain assignment, never
-- re-`local function`.
local wanderTick

-- C++ npc_kelthuzad_minionAI::UpdateRandomMovement
-- (boss_kelthuzad.cpp:670-683): timer expiry -> MovePoint
-- (MOVEMENT_MINION_RANDOM 1, random circle point around _home, 3.0yd);
-- the timer itself lives in the "wander" schedule key (seeded 4-12s
-- at summon, re-armed 4-20s on arrival).
wanderTick = function(creature, guid)
    local hx, hy, hz = creature:GetHomePosition()
    local x, y, z = randomPositionOnCircle(hx, hy, hz, 3.0)
    creature:MoveTo(MOVEMENT_MINION_RANDOM, x, y, z)
end

-- C++ ctor seed (boss_kelthuzad.cpp:596): _movementTimer =
-- urandms(4,12). Seeded only for the BEGIN_ENCOUNTER group summons
-- (REACT_PASSIVE, no DoZoneInCombat); fresh spawn-tick summons engage
-- at once via DoZoneInCombat -> AtEngage -> JustEngagedWith, which
-- zeroes the timer in C++, so they never wander.
local function seedMinionWander(summon)
    local guid = summon:GetGUIDLow()
    schedule(guid, "wander", math.random(4000, 12000), function()
        wanderTick(summon, guid)
    end)
end

-- C++ npc_kelthuzad_minionAI::MovementInform (boss_kelthuzad.cpp:664-668):
-- id == MOVEMENT_MINION_RANDOM -> _movementTimer =
-- urandms(2,10) + urandms(2,10) (4000-20000ms). POINT_MOTION_TYPE is
-- moveType 8 (krick precedent); the type is unchecked in C++ but the
-- id can only arrive from a MoveTo here.
local function kelthuzadMinionMovementInform(_, creature, moveType, id)
    if moveType ~= 8 or id ~= MOVEMENT_MINION_RANDOM then
        return
    end
    local guid = creature:GetGUIDLow()
    schedule(guid, "wander", math.random(2000, 10000) + math.random(2000, 10000), function()
        wanderTick(creature, guid)
    end)
end

local SAY_SLAY = 8
local SAY_DEATH = 9

-- C++ BossAI SummonList twin (boss_kelthuzadAI::JustSummoned, :248-251:
-- every summon latches; SummonedCreatureDespawn prunes on despawn).
-- Latched via ev19 (rotface precedent) — the group summons from the
-- BEGIN_ENCOUNTER receiver arrive through summonTempCreature, which
-- fires ev19 on the summoner (lua_creature_events.go:190).
local kelthuzadSummons = {}

local function kelthuzadJustSummoned(event, creature, summon)
    if not summon then
        return
    end
    local guid = creature:GetGUIDLow()
    local latched = kelthuzadSummons[guid]
    if not latched then
        latched = {}
        kelthuzadSummons[guid] = latched
    end
    latched[#latched + 1] = summon
end

local function kelthuzadSummonDespawned(event, creature, summon)
    if not summon then
        return
    end
    local guid = creature:GetGUIDLow()
    local latched = kelthuzadSummons[guid]
    if latched then
        local summonGuid = summon:GetGUID()
        for i, s in ipairs(latched) do
            if s:GetGUID() == summonGuid then
                table.remove(latched, i)
                break
            end
        end
    end
end

-- C++ temp-vector-first despawn (boss_kelthuzad.cpp:430-438): collect,
-- then despawn — Despawn fires ev20, which prunes this table, so the
-- table must not be iterated while despawning. skipGuardians is the
-- JustDied twin (:263-280): guardians flee instead of despawning.
local function despawnKelthuzadSummons(guid, skipGuardians)
    local latched = kelthuzadSummons[guid]
    if not latched then
        return
    end
    kelthuzadSummons[guid] = nil
    local toDespawn = {}
    for _, summon in ipairs(latched) do
        if summon and (not skipGuardians or summon:GetEntry() ~= ENTRY_GUARDIAN) then
            toDespawn[#toDespawn + 1] = summon
        end
    end
    for _, summon in ipairs(toDespawn) do
        -- A wandering minion's pending "wander" timer must not fire on
        -- the despawned creature (C++ despawn ends the AI outright).
        cancelTimers(summon:GetGUIDLow())
        summon:Despawn()
    end
end

-- C++ EVENT_DESPAWN_MINIONS (boss_kelthuzad.cpp:429-441): despawn every
-- latched summon not in combat (temp vector first, C++ order), then
-- Talk(SAY_AGGRO 7). Seeded once from the BEGIN_ENCOUNTER receiver
-- (3m33s, 213000ms); latent with it.
local function despawnMinionsTick(creature, guid)
    local latched = kelthuzadSummons[guid]
    if latched then
        kelthuzadSummons[guid] = nil
        local toDespawn = {}
        for _, summon in ipairs(latched) do
            if summon and not summon:IsInCombat() then
                toDespawn[#toDespawn + 1] = summon
            end
        end
        for _, summon in ipairs(toDespawn) do
            -- A wandering minion's pending "wander" timer must not fire
            -- on the despawned creature (C++ despawn ends the AI).
            cancelTimers(summon:GetGUIDLow())
            summon:Despawn()
        end
    end
    creature:Talk(SAY_AGGRO)
end

-- C++ BossAI::_Reset (Reset, :223-235: events cancelled, counts zeroed,
-- summons despawned) — timer/summons/counter twins only; the
-- ReactState/NOT_SELECTABLE/immune legs stay out with Reset itself.
local function kelthuzadReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:GetBossState(BOSS_KELTHUZAD) ~= BOSS_STATE_DONE then
        creature:SetBossState(BOSS_KELTHUZAD, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
    end
    despawnKelthuzadSummons(guid, false)
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read, kill.go:1091/:1400),
    -- and a death-time zero would clobber the fight's counter before
    -- the achievement gate reads it; the wipe re-arm is preserved.
    if creature:IsAlive() then
        creature:SetData(DATA_ABOMINATION_DEATH_COUNT, 0)
    end
    -- C++ Reset (:223-235) zeroes the spawn-chain counts and _phaseThree;
    -- _guardianCount is NOT reset there (only the phase-three leg zeroes
    -- it), so latch 6 is left alone.
    creature:SetData(DATA_SKELETON_COUNT, 0)
    creature:SetData(DATA_BANSHEE_COUNT, 0)
    creature:SetData(DATA_ABOMINATION_COUNT, 0)
    creature:SetData(DATA_PHASE_THREE, 0)
    creature:SetData(DATA_PHASE_TWO, 0)
end

-- C++ EnterEvadeMode (:237-245) -> Reset() + _DespawnAtEvade(): the
-- ev2 twin of the Reset bookkeeping above.
local function kelthuzadLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    despawnKelthuzadSummons(guid, false)
    -- C++ EnterEvadeMode calls Reset (:237-245), which zeroes the same
    -- count latches as the ev23 handler above.
    creature:SetData(DATA_ABOMINATION_DEATH_COUNT, 0)
    creature:SetData(DATA_SKELETON_COUNT, 0)
    creature:SetData(DATA_BANSHEE_COUNT, 0)
    creature:SetData(DATA_ABOMINATION_COUNT, 0)
    creature:SetData(DATA_PHASE_THREE, 0)
    creature:SetData(DATA_PHASE_TWO, 0)
end

-- C++ inherited BossAI::JustEngagedWith (no override in boss_kelthuzadAI):
-- SetCombatPulseDelay(5) + me-form DoZoneInCombat at first engage; no Talk
-- here (SAY_AGGRO rides EVENT_DESPAWN_MINIONS).
local function kelthuzadEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_KELTHUZAD, BOSS_STATE_IN_PROGRESS) -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537), base-first (faerlina precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
end

-- C++ KilledUnit: Talk(SAY_SLAY) when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk precedent).
-- C++ DamageTaken (boss_kelthuzad.cpp:282-286): damage = 0 in PHASE_ONE.
-- The phase latch is set by phaseTwoTick; before that every hit rewrites
-- to 0 (return false, 0 — the ev9 rewrite twin, malchezaar/opera
-- precedent). Pure pre-damage zeroing, no post-damage latch semantics.
local function kelthuzadDamageTaken(event, creature, attacker, damage)
    if creature:GetData(DATA_PHASE_TWO) ~= 1 then
        return false, 0
    end
end

local function kelthuzadTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_KELTHUZAD_THRONE = 181640 -- naxxramas.h:124

-- C++ SetBossState(BOSS_KELTHUZAD, DONE) leg (instance_naxxramas.cpp):
-- throne flag clear + DONE flag (no pump leg reads it yet).
-- C++ boss_kelthuzadAI::JustDied (:263-280) relay, in C++ order BEFORE
-- Talk(SAY_DEATH): each alive guardian summon (NPC_GUARDIAN 16441) runs
-- ACTION_KELTHUZAD_DIED (:808-819): Talk(EMOTE_GUARDIAN_FLEE 0) +
-- REACT_PASSIVE + RemoveAllAuras + CombatStop + StopMoving +
-- SetImmuneToPC(true) [SetFlag(59, 0x100) twin] + 30s DespawnOrUnsummon
-- backstop + MoveTargetedHome [MoveHome]. Lua has no kelthuzad summon
-- tracking, so the freya-elder-sweep approximation applies (guardians
-- are chamber-local; the C++ list has no range limit); latent until
-- the phase-1 guardian machine lands. JustReachedHome despawn is
-- unreachable (guardians unregistered), so the 30s backstop is the
-- despawn.
local function kelthuzadGuardianFlee(creature)
    for _, guardian in ipairs(creature:GetCreaturesInGrid(ENTRY_GUARDIAN, 500)) do
        if guardian:IsAlive() then
            guardian:Talk(EMOTE_GUARDIAN_FLEE)
            guardian:SetReactState(0)
            guardian:RemoveAllAuras()
            guardian:CombatStop()
            guardian:StopMoving()
            guardian:SetFlag(59, 0x100)
            CreateLuaEvent(function()
                guardian:Despawn()
            end, 30000)
            guardian:MoveHome()
        end
    end
end

-- Forward declaration (tharon_ja/gal_darah idiom):
-- kelthuzadGuardianDoAction references bloodTapTick before its definition
-- below; assigned there by plain assignment, never re-`local function`.
local bloodTapTick

-- C++ npc_kelthuzad_guardianAI::DoAction(ACTION_JUST_SUMMONED 1)
-- (boss_kelthuzad.cpp:801-811): SetVisible(false) + SetHomePosition +
-- DoZoneInCombat + SetCombatPulseDelay(5) + _visibilityTimer 2s +
-- _bloodTapTimer 25s. The trigger is the boss's EVENT_TRANSITION_SUMMON
-- (unported phase-one machine) and the producer is SummonCreatureGroup
-- (bridged 2026-11-05, tick 238) — the receiver ports latent (krick
-- precedent, tick 238): it fires when the transition machine lands.
local function kelthuzadGuardianDoAction(event, creature, action)
    if action ~= ACTION_JUST_SUMMONED then
        return
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetVisible(false)
    creature:SetHomePosition()
    creature:DoZoneInCombat()
    creature:SetCombatPulseDelay(5)
    -- C++ UpdateAI :845-853: _visibilityTimer -> SetVisible(true) +
    -- Talk(EMOTE_GUARDIAN_APPEAR 1).
    schedule(guid, "visibility", 2000, function()
        creature:SetVisible(true)
        creature:Talk(EMOTE_GUARDIAN_APPEAR)
    end)
    -- C++ UpdateAI :859-864: _bloodTapTimer -> DoCastVictim(SPELL_BLOOD_TAP
    -- 28470), re-arms urand(18s,26s). Seeded here (C++ seeds it in DoAction).
    schedule(guid, "blood_tap", 25000, function()
        bloodTapTick(creature, guid)
    end)
end

-- C++ UpdateAI :855-864: the blood-tap arm runs only with a victim
-- (UpdateVictim gate above it); without one the timer freezes — the
-- 5s re-poll below is the no-victim twin (no cast, no re-arm consumed).
bloodTapTick = function(creature, guid)
    if not creature:GetVictim() then
        schedule(guid, "blood_tap", 5000, function()
            bloodTapTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_BLOOD_TAP)
    schedule(guid, "blood_tap", math.random(18000, 26000), function()
        bloodTapTick(creature, guid)
    end)
end

local function kelthuzadGuardianLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_kelthuzad_guardianAI::Reset (boss_kelthuzad.cpp:836-840):
-- SetCombatPulseDelay(0) + ScriptedAI::Reset.
local function kelthuzadGuardianReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetCombatPulseDelay(0)
end

local function kelthuzadDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    despawnKelthuzadSummons(creature:GetGUIDLow(), true) -- C++ _JustDied (:280): non-guardian summons despawn; guardians flee, not despawned
    kelthuzadGuardianFlee(creature)
    creature:SetBossState(BOSS_KELTHUZAD, BOSS_STATE_DONE) -- BossAI::_JustDied (ScriptedCreature.cpp:519)
    creature:Talk(SAY_DEATH)
    local throne = creature:FindNearestGameObject(GO_KELTHUZAD_THRONE, 1000)
    if throne then
        throne:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end
    local mapObj = creature:GetMap()
    if mapObj then
        local data = mapObj:GetInstanceData()
        if data then
            data.kelthuzadDone = true
        end
    end
end

-- Forward declarations (tharon_ja/gal_darah idiom): the BEGIN_ENCOUNTER
-- receiver below seeds these ticks before their definitions; assigned by
-- plain assignment, never re-`local function`.
local skeletonTick
local bansheeTick
local abominationTick
local phaseTwoTick
local volleyTick
local fissureTick
local detonateTick
local frostBlastTick
local chainsTick
local frostboltTick
local phaseThreePoll
local transitionReplyTick
local transitionSummonTick

-- C++ guardian-group permutation (boss_kelthuzad.cpp:219-220, :492-497):
-- _guardianGroups starts {1,2,3,4} and each TRANSITION_SUMMON swaps a
-- random remaining group to the front; C++ Reset does NOT touch it (or
-- _guardianCount), so the Lua table persists across wipes too — only the
-- phase-three leg zeroes the count latch.
local kelthuzadGuardianGroups = {}

local function randomMinionSpawnPoint()
    return MINION_SPAWN_POINTS[math.random(#MINION_SPAWN_POINTS)]
end

-- C++ EVENT_SKELETON (boss_kelthuzad.cpp:335-374): count++ then the
-- count==1 leg arms a random pre-existing room skeleton (23561, 200yd;
-- sniffs say the first skeleton is pre-existing, :338) or summons a
-- fresh 16427 at a random spawn point (CORPSE_TIMED_DESPAWN, Go type 5,
-- 2000ms corpse timer) + DoZoneInCombat. Re-arm ladder: <10 -> 5s,
-- <19 -> 4s, <31 -> 3s, ==31 -> 4s, <72 -> 2s, then stops (count 72,
-- last fire 206s — before DESPAWN_MINIONS 213s / PHASE_TWO 228s, so the
-- phase gate is vacuous). Empty room-skeleton list is the C++
-- EnterEvadeMode + return leg (no evade bridge): the chain stops with
-- no re-arm, matching the observable timer behavior.
skeletonTick = function(creature, guid)
    local count = creature:GetData(DATA_SKELETON_COUNT) + 1
    creature:SetData(DATA_SKELETON_COUNT, count)
    if count == 1 then
        local skeletons = creature:GetCreaturesInGrid(ENTRY_SKELETON2, 200)
        if #skeletons == 0 then
            return
        end
        local skeleton = skeletons[math.random(#skeletons)]
        skeleton:SetReactState(REACT_AGGRESSIVE)
        skeleton:DoZoneInCombat()
    else
        local pt = randomMinionSpawnPoint()
        local summon = creature:SummonCreature(ENTRY_SKELETON, pt[1], pt[2], pt[3], 0, 5, 2000)
        if summon then
            summon:DoZoneInCombat()
        end
    end
    local nextTime = 0
    if count < 10 then
        nextTime = 5
    elseif count < 19 then
        nextTime = 4
    elseif count < 31 then
        nextTime = 3
    elseif count == 31 then
        nextTime = 4
    elseif count < 72 then
        nextTime = 2
    end
    if nextTime > 0 then
        schedule(guid, "skeleton", nextTime * 1000, function()
            skeletonTick(creature, guid)
        end)
    end
end

-- C++ EVENT_BANSHEE (boss_kelthuzad.cpp:376-394): summon 16429 at a
-- random spawn point (type 5, 2000ms) + DoZoneInCombat. Ladder: <3 ->
-- 30s, <7 -> 20s, <9 -> 15s, then stops (count 9, last fire 200s).
bansheeTick = function(creature, guid)
    local count = creature:GetData(DATA_BANSHEE_COUNT) + 1
    creature:SetData(DATA_BANSHEE_COUNT, count)
    local pt = randomMinionSpawnPoint()
    local summon = creature:SummonCreature(ENTRY_BANSHEE, pt[1], pt[2], pt[3], 0, 5, 2000)
    if summon then
        summon:DoZoneInCombat()
    end
    local nextTime = 0
    if count < 3 then
        nextTime = 30
    elseif count < 7 then
        nextTime = 20
    elseif count < 9 then
        nextTime = 15
    end
    if nextTime > 0 then
        schedule(guid, "banshee", nextTime * 1000, function()
            bansheeTick(creature, guid)
        end)
    end
end

-- C++ EVENT_ABOMINATION (boss_kelthuzad.cpp:396-414): summon 16428 at a
-- random spawn point (type 5, 2000ms) + DoZoneInCombat. Same ladder as
-- the banshee (count 9, last fire 200s).
abominationTick = function(creature, guid)
    local count = creature:GetData(DATA_ABOMINATION_COUNT) + 1
    creature:SetData(DATA_ABOMINATION_COUNT, count)
    local pt = randomMinionSpawnPoint()
    local summon = creature:SummonCreature(ENTRY_ABOMINATION, pt[1], pt[2], pt[3], 0, 5, 2000)
    if summon then
        summon:DoZoneInCombat()
    end
    local nextTime = 0
    if count < 3 then
        nextTime = 30
    elseif count < 7 then
        nextTime = 20
    elseif count < 9 then
        nextTime = 15
    end
    if nextTime > 0 then
        schedule(guid, "abomination", nextTime * 1000, function()
            abominationTick(creature, guid)
        end)
    end
end

-- Random alive player (thorim convention); excludeVictim is the
-- SelectTarget position-1 twin — UnitAI.h:162-163: a nonzero offset
-- skips that many entries in MaxThreat order for Random, i.e. the
-- victim (netherspite convention, nil victim falls back to skipping
-- the first alive entry).
local function randomPlayer(creature, excludeVictim)
    local victimGuid = nil
    local victim = creature:GetVictim()
    if excludeVictim and victim then
        victimGuid = victim:GetGUID()
    end
    local candidates = {}
    local aliveSeen = 0
    for _, t in ipairs(creature:GetAITargets()) do
        local skip = false
        if victimGuid ~= nil then
            skip = t:GetGUID() == victimGuid
        elseif excludeVictim and not t:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip and t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ ManaUserTargetSelector (boss_kelthuzad.cpp:201-207): TYPEID_PLAYER
-- && POWER_MANA. GetPowerType is the moroes/mar'li precedent.
local function randomManaPlayer(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() and t:GetPowerType() == POWER_MANA then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_FROSTBOLT_VOLLEY (boss_kelthuzad.cpp:449-452): DoCastAOE
-- self-cast, repeat randtime(16s,18s). IsCasting defers 500ms
-- (firemaw/mal_ganis idiom — the C++ pre-loop CASTING return defers
-- event execution, it never drops the event).
volleyTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostbolt_volley", 500, function()
            volleyTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FROSTBOLT_VOLLEY)
    schedule(guid, "frostbolt_volley", math.random(16000, 18000), function()
        volleyTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_FISSURE (boss_kelthuzad.cpp:454-458): random player
-- (position 0, no distance limit, playerOnly) -> DoCast 27810, repeat
-- randtime(14s,17s). Re-arms even with no target (C++ Repeat is
-- unconditional).
fissureTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadow_fissure", 500, function()
            fissureTick(creature, guid)
        end)
        return
    end
    local target = randomPlayer(creature, false)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_FISSURE)
    end
    schedule(guid, "shadow_fissure", math.random(14000, 17000), function()
        fissureTick(creature, guid)
    end)
end

-- C++ EVENT_DETONATE_MANA (boss_kelthuzad.cpp:460-467): random mana
-- player -> DoCast 27819, repeat randtime(30s,40s).
detonateTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "detonate_mana", 500, function()
            detonateTick(creature, guid)
        end)
        return
    end
    local target = randomManaPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_DETONATE_MANA)
    end
    schedule(guid, "detonate_mana", math.random(30000, 40000), function()
        detonateTick(creature, guid)
    end)
end

-- C++ EVENT_FROST_BLAST (boss_kelthuzad.cpp:469-473): random player,
-- position 1 (victim excluded) -> DoCast 27808, repeat
-- randtime(25s,45s).
frostBlastTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frost_blast", 500, function()
            frostBlastTick(creature, guid)
        end)
        return
    end
    local target = randomPlayer(creature, true)
    if target then
        creature:CastSpell(target, SPELL_FROST_BLAST)
    end
    schedule(guid, "frost_blast", math.random(25000, 45000), function()
        frostBlastTick(creature, guid)
    end)
end

-- C++ EVENT_CHAINS (boss_kelthuzad.cpp:475-480): DoCastAOE dummy 28408
-- self-cast, repeat 1min + randtime(0s,20s). The dummy landing on KT
-- fires the SpellHit arm below (ev14, boss_ai.go:3828 self-hit leg).
chainsTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "chains", 500, function()
            chainsTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CHAINS_DUMMY)
    schedule(guid, "chains", 60000 + math.random(0, 20000), function()
        chainsTick(creature, guid)
    end)
end

-- C++ UpdateAI frostbolt filler (boss_kelthuzad.cpp:508-513):
-- DoCastVictim(SPELL_FROSTBOLT_SINGLE 28478) whenever the 3s cooldown
-- is down and KT is not casting. Seeded 2s after phase two (the
-- _frostboltCooldown = 2s leg at :440); the phase-one filler is a
-- silent no-op in C++ (DoCast fails while the VISUAL_CHANNEL is live,
-- UnitAI.cpp:167-168), so it is not seeded in phase one.
frostboltTick = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frostbolt", 500, function()
            frostboltTick(creature, guid)
        end)
        return
    end
    if not creature:GetVictim() then
        schedule(guid, "frostbolt", 5000, function()
            frostboltTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_FROSTBOLT_SINGLE)
    schedule(guid, "frostbolt", 3000, function()
        frostboltTick(creature, guid)
    end)
end

-- C++ phase-three leg (boss_kelthuzad.cpp:313-327): HealthBelowPct(45)
-- one-shot — Talk(SAY_REQUEST_AID 12) + TRANSITION_REPLY at 4s +
-- TRANSITION_SUMMON at 7-9s and 13-15s (+19-21s, 25-27s on 25-man).
-- Polled 1s from phase two (no UpdateAI tick in Lua); the latch makes
-- it one-shot. GetHealthPct fires up to 1 HP early (ironaya
-- convention) — the leg trips marginally early, never late.
phaseThreePoll = function(creature, guid)
    if creature:GetData(DATA_PHASE_THREE) == 1 then
        return
    end
    if creature:GetHealthPct() < 45 then
        creature:SetData(DATA_PHASE_THREE, 1)
        creature:SetData(DATA_GUARDIAN_COUNT, 0)
        creature:Talk(SAY_REQUEST_AID)
        schedule(guid, "transition_reply", 4000, function()
            transitionReplyTick(creature, guid)
        end)
        schedule(guid, "transition_summon_1", math.random(7000, 9000), function()
            transitionSummonTick(creature, guid)
        end)
        schedule(guid, "transition_summon_2", math.random(13000, 15000), function()
            transitionSummonTick(creature, guid)
        end)
        if creature:Is25ManRaid() then
            schedule(guid, "transition_summon_3", math.random(19000, 21000), function()
                transitionSummonTick(creature, guid)
            end)
            schedule(guid, "transition_summon_4", math.random(25000, 27000), function()
                transitionSummonTick(creature, guid)
            end)
        end
        return
    end
    schedule(guid, "phase_three", 1000, function()
        phaseThreePoll(creature, guid)
    end)
end

-- C++ EVENT_TRANSITION_REPLY (boss_kelthuzad.cpp:483-488): Lich King
-- Talk(SAY_ANSWER_REQUEST 3) via DATA_LICH_KING; the four KT portal GO
-- states stay out (no GO-state bridge).
transitionReplyTick = function(creature, guid)
    local lichKing = creature:GetInstanceCreature(DATA_LICH_KING)
    if lichKing then
        lichKing:Talk(SAY_ANSWER_REQUEST)
    end
end

-- C++ EVENT_TRANSITION_SUMMON (boss_kelthuzad.cpp:490-501): urand
-- (_guardianCount, 3) swap + SummonCreatureGroup of the next guardian
-- group + DoAction(ACTION_JUST_SUMMONED) per guardian (the receiver is
-- ported latent above). Group rows ride creature_summon_groups (TDB).
transitionSummonTick = function(creature, guid)
    local count = creature:GetData(DATA_GUARDIAN_COUNT)
    if count >= N_GUARDIAN_SPAWNS then
        return
    end
    local groups = kelthuzadGuardianGroups[guid]
    if not groups then
        groups = {}
        for i = 1, N_GUARDIAN_SPAWNS do
            groups[i] = SUMMON_GROUP_GUARDIAN_FIRST + i - 1
        end
        kelthuzadGuardianGroups[guid] = groups
    end
    local selected = math.random(count + 1, N_GUARDIAN_SPAWNS)
    if selected ~= count + 1 then
        groups[selected], groups[count + 1] = groups[count + 1], groups[selected]
    end
    local group = groups[count + 1]
    creature:SetData(DATA_GUARDIAN_COUNT, count + 1)
    local summoned = creature:SummonCreatureGroup(group)
    for _, guardian in ipairs(summoned) do
        guardian:DoAction(ACTION_JUST_SUMMONED)
    end
end

-- C++ EVENT_PHASE_TWO (boss_kelthuzad.cpp:431-448): CastStop (the
-- VISUAL_CHANNEL ends) + NOT_SELECTABLE/immune cleared + threat reset
-- + REACT_AGGRESSIVE + Talk(EMOTE_PHASE_TWO 13) + the five phase-two
-- schedules (SetPhase itself has no bridge, but no arm reads phase
-- state — the phase-one chains all self-terminate by 206s, before this
-- 228s fire, so the machine is fully receiver-seeded).
phaseTwoTick = function(creature, guid)
    creature:SetData(DATA_PHASE_TWO, 1) -- the DamageTaken leg reads this
    creature:StopSpellCast() -- CastStop twin (taldaram/gothik precedent)
    creature:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveFlag(59, UNIT_FLAG_IMMUNE_TO_PC) -- SetImmuneToPC(false)
    creature:ResetAllThreat() -- ResetThreatList
    creature:SetReactState(REACT_AGGRESSIVE)
    creature:Talk(EMOTE_PHASE_TWO)
    schedule(guid, "frostbolt_volley", math.random(24000, 28000), function()
        volleyTick(creature, guid)
    end)
    schedule(guid, "shadow_fissure", math.random(6000, 10000), function()
        fissureTick(creature, guid)
    end)
    schedule(guid, "detonate_mana", math.random(27000, 33000), function()
        detonateTick(creature, guid)
    end)
    schedule(guid, "frost_blast", math.random(25000, 45000), function()
        frostBlastTick(creature, guid)
    end)
    if creature:Is25ManRaid() then
        schedule(guid, "chains", math.random(60000, 80000), function()
            chainsTick(creature, guid)
        end)
    end
    schedule(guid, "frostbolt", 2000, function()
        frostboltTick(creature, guid)
    end)
    schedule(guid, "phase_three", 1000, function()
        phaseThreePoll(creature, guid)
    end)
end

-- C++ SpellHit (boss_kelthuzad.cpp:288-297): SPELL_CHAINS_DUMMY 28408
-- -> Talk(SAY_CHAINS 10) + 3 random players -> DoCast(CHains 28410)
-- each. The trigger is the phase-two EVENT_CHAINS self-cast; the ev14
-- fire is the creature-caster self-hit leg (boss_ai.go:3828). Latent
-- until the 28408 spell data resolves in Go (no Spell.dbc in workspace
-- — freya 62207 caveat); the 3-target pick samples without replacement
-- (C++ SelectTargetList(3, Random)).
local function kelthuzadSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_CHAINS_DUMMY then
        return
    end
    creature:Talk(SAY_CHAINS)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    for i = 1, 3 do
        if #candidates == 0 then
            break
        end
        local idx = math.random(#candidates)
        creature:CastSpell(candidates[idx], SPELL_CHAINS)
        table.remove(candidates, idx)
    end
end

-- C++ boss_kelthuzadAI::DoAction(ACTION_BEGIN_ENCOUNTER 0)
-- (boss_kelthuzad.cpp:529-555), in C++ order. The sender is the
-- unported at_kelthuzad_center area trigger (:989-1002, no AT bridge),
-- so the receiver ports latent (krick precedent): it fires when an
-- area-trigger bridge lands. The NOT_STARTED gate reads Go's boss
-- state, where an unset boss is TO_BE_DECIDED (5) — the Go twin of
-- C++'s Create-time NOT_STARTED pre-fill (InstanceScript.h:237;
-- instance_boss_states.go), so 5 proceeds like 0; any other state
-- (IN_PROGRESS/FAIL/DONE) returns, C++-exact. SetImmuneToPC(false)
-- is the RemoveFlag(59, 0x100) twin (Unit.cpp:8440-8449; nefarian
-- precedent) — the tick-241 "no bridge" claim was stale. The
-- SetPhase(PHASE_ONE) leg has no bridge, but all five schedules it
-- feeds are seeded here 2026-11-05 (skeleton 5s, banshee 30s,
-- abomination 30s, despawn-minions 3m33s, phase-two 3m48s) — no arm
-- reads phase state (the chains self-terminate by 206s, before the
-- 213s sweep and the 228s switch).
-- The summon groups ride creature_summon_groups (TDB rows; the repo
-- world.db has no such table, so the loop is inert until live TDB
-- provides them — yogg-illusion caveat); each summon gets
-- REACT_PASSIVE + its pocket id via SetData(DATA_MINION_POCKET_ID,
-- group), the per-motion store the minion pocket legs read.
local function kelthuzadDoAction(event, creature, action)
    if action == ACTION_ABOMINATION_DIED then
        -- C++ DoAction(ACTION_ABOMINATION_DIED) (boss_kelthuzad.cpp:557-559):
        -- ++_abominationDeathCount; the GetData twin is the per-motion
        -- SetData latch (absent reads 0, tick-235 convention).
        local count = creature:GetData(DATA_ABOMINATION_DEATH_COUNT)
        creature:SetData(DATA_ABOMINATION_DEATH_COUNT, count + 1)
        return
    end
    if action ~= ACTION_BEGIN_ENCOUNTER then
        return
    end
    local state = creature:GetBossState(BOSS_KELTHUZAD)
    if state ~= 0 and state ~= 5 then
        return
    end
    creature:RemoveFlag(59, 0x100) -- SetImmuneToPC(false)
    creature:SetBossState(BOSS_KELTHUZAD, BOSS_STATE_IN_PROGRESS) -- IN_PROGRESS
    creature:DoZoneInCombat()
    creature:CastSpell(creature, SPELL_VISUAL_CHANNEL) -- DoCastAOE self-cast
    creature:Talk(SAY_SUMMON_MINIONS)
    for group = SUMMON_GROUP_MINION_FIRST, SUMMON_GROUP_MINION_FIRST + N_MINION_GROUPS - 1 do
        local summoned = creature:SummonCreatureGroup(group)
        for _, summon in ipairs(summoned) do
            summon:SetReactState(0) -- REACT_PASSIVE
            summon:SetData(DATA_MINION_POCKET_ID, group)
            seedMinionWander(summon)
        end
    end
    local guid = creature:GetGUIDLow()
    -- C++ :550-554: the five phase-one schedules, all seeded once here.
    -- EVENT_SKELETON 5s, EVENT_BANSHEE 30s, EVENT_ABOMINATION 30s,
    -- EVENT_DESPAWN_MINIONS 3m33s (213000ms), EVENT_PHASE_TWO 3m48s
    -- (228000ms). SetPhase(PHASE_ONE) itself has no bridge, but no arm
    -- reads phase state — the chains self-terminate by 206s, before the
    -- 213s sweep and the 228s phase switch.
    schedule(guid, "skeleton", 5000, function()
        skeletonTick(creature, guid)
    end)
    schedule(guid, "banshee", 30000, function()
        bansheeTick(creature, guid)
    end)
    schedule(guid, "abomination", 30000, function()
        abominationTick(creature, guid)
    end)
    schedule(guid, "despawn_minions", 213000, function()
        despawnMinionsTick(creature, guid)
    end)
    schedule(guid, "phase_two", 228000, function()
        phaseTwoTick(creature, guid)
    end)
end

-- Forward declaration (tharon_ja/gal_darah idiom): woundTick is
-- referenced by kelthuzadMinionEnterCombat before its definition
-- below; assigned there by plain assignment, never re-`local function`.
local woundTick

-- C++ npc_kelthuzad_minionAI::JustEngagedWith (boss_kelthuzad.cpp:581-601):
-- non-passive -> DoZoneInCombat + return; passive with no pocket ->
-- return; else the pocket sweep: same-entry creatures within 80yd
-- sharing the pocket id go REACT_AGGRESSIVE + AttackStart(who), then
-- self AGGRESSIVE + AttackStart(who). Abominations also seed the wound
-- timer on first engage — the C++ timer is constructor-seeded and
-- frozen out of combat, so the ev1 seed is its in-combat twin (fresh
-- 10-20s each engage, bloodTapTick precedent).
local function kelthuzadMinionEnterCombat(event, creature, target)
    -- C++ npc_kelthuzad_minionAI::JustEngagedWith
    -- (boss_kelthuzad.cpp:611): _movementTimer = 0 — once zero it is
    -- never checked again, so the wander stops permanently on engage.
    -- (A MovePoint already in flight still lands and re-arms via ev28,
    -- the C++ MovementInform behavior — the spline is not cancelled.)
    local guid = creature:GetGUIDLow()
    cancelKey(guid, "wander")
    if creature:GetEntry() == ENTRY_ABOMINATION then
        schedule(guid, "wound", math.random(10000, 20000), function()
            woundTick(creature, guid)
        end)
    end
    if creature:GetReactState() ~= REACT_PASSIVE then
        creature:DoZoneInCombat()
        return
    end
    local pocket = creature:GetData(DATA_MINION_POCKET_ID)
    if pocket == 0 then
        return
    end
    for _, other in ipairs(creature:GetCreaturesInGrid(creature:GetEntry(), 80)) do
        if other:GetData(DATA_MINION_POCKET_ID) == pocket then
            other:SetReactState(REACT_AGGRESSIVE)
            if target then
                other:AttackStart(target)
            end
        end
    end
    creature:SetReactState(REACT_AGGRESSIVE)
    if target then
        creature:AttackStart(target)
    end
end

-- C++ npc_kelthuzad_abominationAI::UpdateAI :716-728: _woundTimer ->
-- DoCastVictim(SPELL_MORTAL_WOUND 28467), re-arms urand(14s,18s).
-- Gated on a victim (UpdateVictim above it); the 5s re-poll is the
-- no-victim twin (no cast, no re-arm consumed).
woundTick = function(creature, guid)
    if not creature:GetVictim() then
        schedule(guid, "wound", 5000, function()
            woundTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_MORTAL_WOUND)
    schedule(guid, "wound", math.random(14000, 18000), function()
        woundTick(creature, guid)
    end)
end

local function kelthuzadMinionLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ has no JustDied arm for skeleton/banshee (boss_kelthuzad.cpp:
-- only the abomination relays); this handler is pure timer
-- bookkeeping — a wander timer re-armed by an in-flight MoveTo
-- landing after engage must not fire on the corpse.
local function kelthuzadMinionDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_kelthuzad_abominationAI::JustDied (:732-736): relay
-- ACTION_ABOMINATION_DIED to Kel'Thuzad via DATA_KELTHUZAD (the
-- _abominationDeathCount latch behind achievement_just_cant_get_enough;
-- the achievement check itself stays out — no bridge for that
-- ScriptName).
local function kelthuzadAbominationDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    local kelthuzad = creature:GetInstanceCreature(DATA_KELTHUZAD)
    if kelthuzad then
        kelthuzad:DoAction(ACTION_ABOMINATION_DIED)
    end
end

RegisterCreatureEvent(ENTRY_KEL_THUZAD, 1, kelthuzadEnterCombat)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 2, kelthuzadLeaveCombat)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 3, kelthuzadTargetDied)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 9, kelthuzadDamageTaken)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 4, kelthuzadDied)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 14, kelthuzadSpellHit)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 19, kelthuzadJustSummoned)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 20, kelthuzadSummonDespawned)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 23, kelthuzadReset)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 38, kelthuzadDoAction)
RegisterCreatureEvent(ENTRY_SKELETON, 1, kelthuzadMinionEnterCombat)
RegisterCreatureEvent(ENTRY_BANSHEE, 1, kelthuzadMinionEnterCombat)
RegisterCreatureEvent(ENTRY_ABOMINATION, 1, kelthuzadMinionEnterCombat)
RegisterCreatureEvent(ENTRY_ABOMINATION, 2, kelthuzadMinionLeaveCombat)
RegisterCreatureEvent(ENTRY_SKELETON, 4, kelthuzadMinionDied)
RegisterCreatureEvent(ENTRY_BANSHEE, 4, kelthuzadMinionDied)
RegisterCreatureEvent(ENTRY_ABOMINATION, 4, kelthuzadAbominationDied)
RegisterCreatureEvent(ENTRY_SKELETON, 28, kelthuzadMinionMovementInform)
RegisterCreatureEvent(ENTRY_BANSHEE, 28, kelthuzadMinionMovementInform)
RegisterCreatureEvent(ENTRY_ABOMINATION, 28, kelthuzadMinionMovementInform)
RegisterCreatureEvent(ENTRY_GUARDIAN, 2, kelthuzadGuardianLeaveCombat)
RegisterCreatureEvent(ENTRY_GUARDIAN, 4, kelthuzadGuardianLeaveCombat)
RegisterCreatureEvent(ENTRY_GUARDIAN, 23, kelthuzadGuardianReset)
RegisterCreatureEvent(ENTRY_GUARDIAN, 38, kelthuzadGuardianDoAction)

-- C++ spell_kelthuzad_detonate_mana_AuraScript::HandleScript
-- (boss_kelthuzad.cpp:930-942): OnEffectPeriodic, EFFECT_0,
-- PreventDefaultAction. Drains maxMana/10 (integer division; the
-- arm is skipped when that is 0); BP0 is the actually-drained mana
-- x10 — C++ Unit::ModifyPower returns the signed applied gain
-- (Unit.cpp:8577-8605), so -gain*10 is drain*10. The Go creature
-- bridge returns the NEW power instead (lua_creature_events.go:
-- 2743-2771, whose comment claims C++ parity — it does not) and
-- the player bridge returns nil (lua_player.go:3287), so the drain
-- is measured with GetPower(0) before/after; the player and
-- creature ModifyPower arg orders also differ ((delta, type) vs
-- (type, delta)), hence the GetTypeId() == 4 branch (vashj
-- precedent). Returns true, the PreventDefaultAction twin.
local function detonateManaTick(event, aura, target)
    local drain = math.floor(target:GetMaxPower(0) / 10)
    if drain > 0 then
        local before = target:GetPower(0)
        if target:GetTypeId() == 4 then
            target:ModifyPower(-drain, 0)
        else
            target:ModifyPower(0, -drain)
        end
        local drained = before - target:GetPower(0)
        target:CastCustomSpell(target, SPELL_MANA_DETONATION_DAMAGE, true, drained * 10)
    end
    return true
end

RegisterAuraEvent(SPELL_DETONATE_MANA, 1, detonateManaTick)

-- C++ spell_kelthuzad_frost_blast::PeriodicTick
-- (boss_kelthuzad.cpp:965-975): OnEffectPeriodic, EFFECT_1,
-- PreventDefaultAction. The caster casts SPELL_FROST_BLAST_DMG
-- 29879 on the aura target with BP0 = 26% of the target's max
-- health (CountPctFromMaxHealth, object.go fallback). Returns true,
-- the PreventDefaultAction twin. Triggered=true follows the vezax
-- mark-of-the-faceless precedent (C++ passes no triggered flag,
-- but Lua periodic damage casts ride triggered).
local function frostBlastAuraTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastCustomSpell(target, SPELL_FROST_BLAST_DMG, true, target:CountPctFromMaxHealth(26))
    end
    return true
end

RegisterAuraEvent(SPELL_FROST_BLAST, 1, frostBlastAuraTick)
