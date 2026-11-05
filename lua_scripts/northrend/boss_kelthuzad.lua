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
-- combat start; instance legs + setActive unbridged. No timers are modeled,
-- so no cancel bookkeeping.
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
-- SpellHit — SPELL_CHAINS_DUMMY 28408 -> Talk(SAY_CHAINS 10) +
-- SelectTargetList(3, Random) -> DoCast(CHains 28410) — stays out:
-- the trigger is the unported phase-two EVENT_CHAINS arm
-- (boss_kelthuzad.cpp:446; no phase bridge, and this file models
-- no timers), so event 14 can never fire with this ID (the old
-- "SpellHit never fires in the Lua surface" ruling was stale —
-- event 14 fires from the Go spell path). EVENT_SKELETON /
-- EVENT_BANSHEE / EVENT_ABOMINATION — stay out behind the unported
-- phase-one machine (scheduled from the instance DoAction
-- BEGIN_ENCOUNTER leg, PHASE_ONE-gated; summon bridge is live and the
-- spawn coords are C++-literal, but the phase machine rules — gothik
-- precedent). NOTE for re-triage: the chains self-terminate before the
-- phase switch — skeleton last fire ~210s (count 72), banshee/abom
-- last fire 200s (count 9), vs DESPAWN_MINIONS 213s / PHASE_TWO 228s —
-- so the phase gate is vacuous in the normal flow and the chains are
-- portable as receiver-seeded CreateLuaEvent timers once a tick
-- re-verifies the arithmetic against the C++ counts above;
-- EVENT_SKELETON _skeletonCount==1 leg (boss_kelthuzad.cpp:349):
-- (*it)->SetReactState(REACT_AGGRESSIVE) + DoZoneInCombat on the
-- pre-existing room skeleton — cross-creature, rides the unported
-- phase-one DoAction machine (same EVENT_SKELETON gate) — documented-only;
-- EVENT_DESPAWN_MINIONS — PORTED 2026-11-05 in despawnMinionsTick
-- (seeded 3m33s from the BEGIN_ENCOUNTER receiver, latent with it):
-- temp-vector sweep of the latched summons table, DespawnOrUnsummon
-- on each non-combat summon + Talk(SAY_AGGRO 7), C++ order
-- (boss_kelthuzad.cpp:429-441); EVENT_PHASE_TWO — CastStop +
-- SetPhase(PHASE_TWO) + NOT_SELECTABLE/immune/ReactState/ThreatList
-- legs + Talk(EMOTE_PHASE_TWO 13) + the five phase-two event schedules
-- — no phase / react-state bridges (the threat leg IS bridged —
-- ResetAllThreat, lua_creature_events.go — but the whole
-- EVENT_PHASE_TWO machine stays out, gothik precedent); EVENT_FROSTBOLT_VOLLEY —
-- DoCastAOE(SPELL_FROSTBOLT_VOLLEY 28479) single-arg (triggered=false is the default),
-- randtime(24s,28s) init, randtime(16s,18s) repeat — stays OUT:
-- scheduled only in PHASE_TWO via EVENT_PHASE_TWO (:441/:449-452);
-- the phase machine has no bridge (gothik precedent; the Oct-12 "no
-- DoCastAOE bridge" claim is superseded by the phase-gating verdict); EVENT_SHADOW_FISSURE —
-- SelectTarget(Random, 0, 0.0f, true) -> DoCast(SHADOW_FISSURE 27810), Repeat randtime(14s,17s) —
-- stays OUT: the random-target leg rides the thorim randomPlayerInRange convention, but the arm is
-- PHASE_TWO-gated (scheduled only at :442 — the phase machine has no bridge, gothik precedent;
-- the stale "no random-target SelectTarget bridge (cairne/kazzak)" claim is retracted);
-- EVENT_DETONATE_MANA — SelectTarget(Random, 0, ManaUserTargetSelector) -> DoCast(DETONATE_MANA
-- 27819), Repeat randtime(30s,40s) — stays OUT: PHASE_TWO-gated (:443); the random-target + mana-user
-- legs are bridgeable (thorim pool + plr:GetPowerType(), lua_player.go:84), the phase gate rules;
-- stale "no random-target bridge" retracted;
-- EVENT_FROST_BLAST — SelectTarget(Random, 1) -> DoCast(FROST_BLAST 27808), Repeat randtime(25s,45s)
-- — stays OUT: PHASE_TWO-gated (:444); random-target rides the thorim convention, the position-1
-- offset rides the netherspite victim-exclusion, but the phase gate rules; stale claim retracted;
-- EVENT_CHAINS —
-- DoCastAOE(SPELL_CHAINS_DUMMY 28408) single-arg (triggered=false is the default),
-- randtime(60s,80s) init, 1min+randtime(0s,20s) repeat — stays OUT:
-- scheduled only in PHASE_TWO via EVENT_PHASE_TWO + gated on
-- Is25ManRaid (:447/:475-480); the phase machine has no bridge
-- (difficulty gate now bridged, lua_creature_events.go, but stranded
-- on the phase machine);
-- EVENT_TRANSITION_REPLY — lich-king cross-AI Talk(SAY_ANSWER_REQUEST
-- 3) via DATA_LICH_KING + the four KT portal GO states — instance
-- model / no GO-state bridge; EVENT_TRANSITION_SUMMON —
-- SummonCreatureGroup(_guardianGroups) — the group bridge is live
-- (creature:SummonCreatureGroup, 2026-11-05 tick 238; the guardian
-- ev38 receiver ports below) but the transition event machine that
-- fires it is unported — the summon stays out with the machine;
-- phase-three UpdateAI leg — HealthBelowPct(45) Talk(SAY_REQUEST_AID
-- 12) + transition scheduling — the health-pct read IS bridgeable
-- (creature:GetHealthPct(), object.go:107) but the transition arm rides
-- the unported summon machine (gothik phase-gating precedent — Talk +
-- transition stay out together);
-- UpdateAI frostbolt filler — DoCastVictim(SPELL_FROSTBOLT_SINGLE
-- 28478) — phase-gated on !PHASE_ONE and the UNIT_STATE_CASTING check
-- — the phase gate rules (IsCasting IS bridged,
-- lua_creature_events.go:1950, but no ported timer tick exists to gate); DamageTaken — damage = 0 in
-- PHASE_ONE — no phase bridge; Reset (boss_kelthuzad.cpp:223-235) —
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
-- group) per summon. The SetPhase(PHASE_ONE) leg + the five schedules
-- stay out with the phase-one machine (no timers seeded).
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
-- UpdateRandomMovement hack stays out (no motion bridge — the
-- random-circle position math has no MoveTo twin without a
-- MovementInform re-arm, no ev28); MoveInLineOfSight 20yd aggro leg
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
-- -> SPELL_MANA_DETONATION_DAMAGE 27820 — no AuraScript binding bridge
-- — joins the SpellScript/AuraScript queue.
-- spell_kelthuzad_frost_blast (AuraScript 27808): PeriodicTick ->
-- SPELL_FROST_BLAST_DMG 29879 with 26%-max-health scaling — no
-- AuraScript binding bridge — joins the SpellScript/AuraScript queue.
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
local SPELL_VISUAL_CHANNEL = 29423 -- boss_kelthuzad.cpp:89
local SAY_SUMMON_MINIONS = 14 -- boss_kelthuzad.cpp:40
local SUMMON_GROUP_MINION_FIRST = 5 -- boss_kelthuzad.cpp:116
local N_MINION_GROUPS = 7 -- boss_kelthuzad.cpp:112
local DATA_MINION_POCKET_ID = 0 -- boss_kelthuzad.cpp:82 (KTData enum, first)
local SPELL_BLOOD_TAP = 28470 -- boss_kelthuzad.cpp:108
local ENTRY_SKELETON = 16427 -- boss_kelthuzad.cpp:135 NPC_SKELETON1
local ENTRY_ABOMINATION = 16428 -- boss_kelthuzad.cpp:137 NPC_ABOMINATION1
local ENTRY_BANSHEE = 16429 -- boss_kelthuzad.cpp:138 NPC_BANSHEE1
local ACTION_ABOMINATION_DIED = 2 -- boss_kelthuzad.cpp:77 (Actions enum)
local DATA_KELTHUZAD = 14 -- naxxramas.h NAXData64 (0-based: ANUBREKHAN 0 .. KELTHUZAD 14)
local DATA_ABOMINATION_DEATH_COUNT = 1 -- boss_kelthuzad.cpp:83 (KTData enum, second)
local SPELL_MORTAL_WOUND = 28467 -- boss_kelthuzad.cpp:105
local SAY_AGGRO = 7 -- boss_kelthuzad.cpp:33
local REACT_PASSIVE = 0 -- UnitDefines.h
local REACT_AGGRESSIVE = 2 -- UnitDefines.h

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
    local guid = creature:GetGUID()
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
    local guid = creature:GetGUID()
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
            summon:Despawn()
        end
    end
    creature:Talk(SAY_AGGRO)
end

-- C++ BossAI::_Reset (Reset, :223-235: events cancelled, counts zeroed,
-- summons despawned) — timer/summons/counter twins only; the
-- ReactState/NOT_SELECTABLE/immune legs stay out with Reset itself.
local function kelthuzadReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    despawnKelthuzadSummons(guid, false)
    creature:SetData(DATA_ABOMINATION_DEATH_COUNT, 0)
end

-- C++ EnterEvadeMode (:237-245) -> Reset() + _DespawnAtEvade(): the
-- ev2 twin of the Reset bookkeeping above.
local function kelthuzadLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    despawnKelthuzadSummons(guid, false)
end

-- C++ inherited BossAI::JustEngagedWith (no override in boss_kelthuzadAI):
-- SetCombatPulseDelay(5) + me-form DoZoneInCombat at first engage; no Talk
-- here (SAY_AGGRO rides EVENT_DESPAWN_MINIONS).
local function kelthuzadEnterCombat(event, creature, target)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
end

-- C++ KilledUnit: Talk(SAY_SLAY) when victim->GetTypeId() ==
-- TYPEID_PLAYER (nalorakk precedent).
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
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
end

-- C++ npc_kelthuzad_guardianAI::Reset (boss_kelthuzad.cpp:836-840):
-- SetCombatPulseDelay(0) + ScriptedAI::Reset.
local function kelthuzadGuardianReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetCombatPulseDelay(0)
end

local function kelthuzadDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    despawnKelthuzadSummons(creature:GetGUID(), true) -- C++ _JustDied (:280): non-guardian summons despawn; guardians flee, not despawned
    kelthuzadGuardianFlee(creature)
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
-- SetPhase(PHASE_ONE) leg has no bridge and four of the five
-- schedules it feeds stay out with the phase-one machine
-- (EVENT_SKELETON 5s, EVENT_BANSHEE 30s, EVENT_ABOMINATION 30s,
-- EVENT_PHASE_TWO 3m48s); EVENT_DESPAWN_MINIONS (3m33s) is seeded
-- here 2026-11-05 — its handler needs no phase state (the chains
-- self-terminate by ~210s, before the 213s sweep).
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
    creature:SetBossState(BOSS_KELTHUZAD, 1) -- IN_PROGRESS
    creature:DoZoneInCombat()
    creature:CastSpell(creature, SPELL_VISUAL_CHANNEL) -- DoCastAOE self-cast
    creature:Talk(SAY_SUMMON_MINIONS)
    for group = SUMMON_GROUP_MINION_FIRST, SUMMON_GROUP_MINION_FIRST + N_MINION_GROUPS - 1 do
        local summoned = creature:SummonCreatureGroup(group)
        for _, summon in ipairs(summoned) do
            summon:SetReactState(0) -- REACT_PASSIVE
            summon:SetData(DATA_MINION_POCKET_ID, group)
        end
    end
    -- C++ :551: EVENT_DESPAWN_MINIONS at 3m33s (213000ms), seeded once.
    schedule(creature:GetGUID(), "despawn_minions", 213000, function()
        despawnMinionsTick(creature, creature:GetGUID())
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
    if creature:GetEntry() == ENTRY_ABOMINATION then
        local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
end

-- C++ npc_kelthuzad_abominationAI::JustDied (:732-736): relay
-- ACTION_ABOMINATION_DIED to Kel'Thuzad via DATA_KELTHUZAD (the
-- _abominationDeathCount latch behind achievement_just_cant_get_enough;
-- the achievement check itself stays out — no bridge for that
-- ScriptName).
local function kelthuzadAbominationDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    local kelthuzad = creature:GetInstanceCreature(DATA_KELTHUZAD)
    if kelthuzad then
        kelthuzad:DoAction(ACTION_ABOMINATION_DIED)
    end
end

RegisterCreatureEvent(ENTRY_KEL_THUZAD, 1, kelthuzadEnterCombat)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 2, kelthuzadLeaveCombat)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 3, kelthuzadTargetDied)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 4, kelthuzadDied)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 19, kelthuzadJustSummoned)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 20, kelthuzadSummonDespawned)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 23, kelthuzadReset)
RegisterCreatureEvent(ENTRY_KEL_THUZAD, 38, kelthuzadDoAction)
RegisterCreatureEvent(ENTRY_SKELETON, 1, kelthuzadMinionEnterCombat)
RegisterCreatureEvent(ENTRY_BANSHEE, 1, kelthuzadMinionEnterCombat)
RegisterCreatureEvent(ENTRY_ABOMINATION, 1, kelthuzadMinionEnterCombat)
RegisterCreatureEvent(ENTRY_ABOMINATION, 2, kelthuzadMinionLeaveCombat)
RegisterCreatureEvent(ENTRY_ABOMINATION, 4, kelthuzadAbominationDied)
RegisterCreatureEvent(ENTRY_GUARDIAN, 2, kelthuzadGuardianLeaveCombat)
RegisterCreatureEvent(ENTRY_GUARDIAN, 4, kelthuzadGuardianLeaveCombat)
RegisterCreatureEvent(ENTRY_GUARDIAN, 23, kelthuzadGuardianReset)
RegisterCreatureEvent(ENTRY_GUARDIAN, 38, kelthuzadGuardianDoAction)
