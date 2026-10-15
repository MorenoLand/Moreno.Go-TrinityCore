-- Gortok Palehoof (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_palehoof.cpp
-- (723 lines; 10 scripts — boss_palehoof (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_GORTOK_PALEHOOF = 1);
-- boss_ravenous_furbolg / boss_frenzied_worgen / boss_ferocious_rhino /
-- boss_massive_jormungar (CreatureScript via GetUtgardePinnacleAI
-- (PalehoofMinionsBossAI : BossAI), DATA_RAVENOUS_FURBOLG /
-- DATA_FRENZIED_WORGEN / DATA_FEROCIOUS_RHINO / DATA_MASSIVE_JORMUNGAR);
-- go_palehoof_sphere (GameObjectScript via GetUtgardePinnacleAI
-- (GameObjectAI), OnGossipHello ACTION_START_ENCOUNTER);
-- spell_palehoof_crazed (SpellScriptLoader, AuraScript OnEffectRemove ->
-- RemoveAurasDueToSpell 48147); spell_palehoof_crazed_effect
-- (SpellScriptLoader, SpellScript OnEffectHitTarget SCRIPT_EFFECT ->
-- CastSpell 48147); spell_palehoof_awaken_subboss (SpellScriptLoader,
-- SpellScript OnEffectHitTarget APPLY_AURA -> CastSpell 48048 + CombatStart
-- event); spell_palehoof_awaken_gortok (SpellScriptLoader, SpellScript
-- OnEffectHitTarget DUMMY -> CombatStart event); all registered from inside
-- AddSC_boss_palehoof(); loader decl 133 / call 328 per
-- northrend_script_loader.cpp — the SECOND group of the "// Utgarde Keep -
-- Utgarde Pinnacle" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_svala() (call 327); the checkpoint sequence
-- (boss_svala -> boss_palehoof) is followed).
-- Entries: 26687 Gortok Palehoof (utgarde_pinnacle.h NPC_GORTOK_PALEHOOF,
-- line 54); 26683 Frenzied Worgen, 26684 Ravenous Furbolg, 26685 Massive
-- Jormungar, 26686 Ferocious Rhino (utgarde_pinnacle.h lines 62-65 —
-- registered this run for their ungated timer arms) — entry-verifiable,
-- registration proceeds (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is DB-side
-- as usual. DATA_GORTOK_PALEHOOF = 1 (utgarde_pinnacle.h line 32).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_palehoof"
-- hits boss_palehoof.cpp only (+ the loader decl/call lines); the clone's
-- sql/ tree (~/workspace/moreno-trinitycore/sql/, characters/ + world/)
-- carries zero palehoof hits (empty of bindings). The lua was ported Oct 2
-- 10:25 UTC in 491fb38 (Talk-only, stale "no-timer-bridge" doctrine) and
-- UPGRADED this run under the CreateLuaEvent doctrine
-- (engine/scripting/runtime.go:798 — the maiden_of_virtue / keleseth /
-- yogg_saron precedent): module-local GUID-keyed timers table, wipe on
-- events 2 (leave combat), 4 (death), 23 (evade). creature:CastSpell(nil,
-- spell) = DoCastVictim; creature:CastSpell(creature, spell) = DoCastSelf
-- / DoCastAOE (the selin_fireheart / mr_smite / boss_ai.go DoCastAOE
-- lowering precedent); the 3rd CastSpell arg is the C++ DoCast 3rd arg
-- (2-arg / 1-arg overloads = non-triggered); {minMs, maxMs} table delay =
-- randtime range; random-target arms use GetPlayersInWorld (the baltharus
-- / thorim colossus precedent). The 2026-10-02 "no-timer-bridge" header
-- doctrine is RETRACTED for this file: CreateLuaEvent is a real
-- registered bridge, so the ungated timer arms below are ported rather
-- than parked.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_palehoof.cpp):
-- boss_palehoof JustEngagedWith (:221-229) — Talk(SAY_AGGRO 0)
-- (event 1; BossAI::JustEngagedWith passthrough and the instance
-- SendEncounterUnit(ENCOUNTER_FRAME_ENGAGE) leg have no bridges — the
-- auriaya engage-port precedent) — plus the three ungated timer arms,
-- scheduled here on the xt002 precedent (C++ schedules on
-- JustEngagedWith, Go on event 1, same inits):
--   EVENT_ARCING_SMASH (arm :236-239): DoCastVictim(SPELL_ARCING_SMASH
--   48260), 1-arg = non-triggered -> CastSpell(nil, 48260); init 7s,
--   Repeat(7s).
--   EVENT_IMPALE (arm :240-244): SelectTarget(SelectTargetMethod::Random,
--   0) — position 0, dist 0 = unlimited range, playerOnly default
--   FALSE (UnitAI.h:170) — the player-only pool is the lavanthor
--   documented approximation —
--   -> DoCast(target, SPELL_IMPALE 48261), 2-arg = non-triggered ->
--   CastSpell(target, 48261); init 11s, Repeat(10s,15s).
--   EVENT_WITHERING_ROAR (arm :245-248): DoCastSelf(SPELL_WITHERING_ROAR
--   48256), 1-arg = non-triggered -> CastSpell(creature, 48256); init
--   12s, Repeat(11s).
-- boss_palehoof KilledUnit (:269-273) — Talk(SAY_SLAY 1) player-gated
-- (event 3 — the razuvious player-gated variant precedent; the nil-guard
-- is the hodir-port convention — Eluna event 3 always carries a victim,
-- so C++-exactness is unaffected).
-- The four minibosses' timer kits are all ungated in their ExecuteEvent
-- switches (no phase groups, no DoAction-gated arms — the yogg_saron
-- adds precedent), so they are ported with per-entry event-1 scheduling
-- carrying the C++ ScheduleTasks inits:
--   boss_ravenous_furbolg (26684), ScheduleTasks :377-379, arms :386-398:
--   EVENT_CRAZED DoCastSelf(SPELL_CRAZED 48139) -> CastSpell(creature,
--   48139), init 10s, Repeat(20s,25s); EVENT_CHAIN_LIGHTNING
--   DoCastVictim(SPELL_CHAIN_LIGHTNING 48140) -> CastSpell(nil, 48140),
--   init 12s, Repeat(11s); EVENT_TERRIFYING_ROAR DoCastSelf
--   (SPELL_TERRIFYING_ROAR 48144) -> CastSpell(creature, 48144), init
--   22s, Repeat(18s); all non-triggered.
--   boss_frenzied_worgen (26683), ScheduleTasks :421-423, arms :430-440:
--   EVENT_MORTAL_WOUND DoCastVictim(SPELL_MORTAL_WOUND 48137) ->
--   CastSpell(nil, 48137), init 6s, Repeat(6s); EVENT_ENRAGE DoCastSelf
--   (SPELL_ENRAGE_1 48138) -> CastSpell(creature, 48138), init 16s,
--   Repeat(25s); EVENT_ENRAGE_2 DoCastSelf(SPELL_ENRAGE_2 48142) ->
--   CastSpell(creature, 48142), init 90s, one-shot; all non-triggered.
--   boss_ferocious_rhino (26686), ScheduleTasks :464-466, arms :473-485:
--   EVENT_GORE DoCastVictim(SPELL_GORE 48130) -> CastSpell(nil, 48130),
--   init 10s, Repeat(19s); EVENT_GRIEVOUS_WOUND SelectTarget(Random, 1)
--   — position 1, dist 0 = unlimited, playerOnly default FALSE
--   (UnitAI.h:170) — the player-only pool is the lavanthor documented
--   approximation; the offset-1 top-threat skip (UnitAI.h:172-174) has
--   no threat-list bridge (ioc dagger-throw precedent) —
--   DoCast(target, SPELL_GRIEVOUS_WOUND 48105), 2-arg = non-triggered
--   -> CastSpell(target, 48105), init 12s, Repeat(18s); EVENT_STOMP
--   DoCastSelf(SPELL_STOMP 48131) -> CastSpell(creature, 48131), init
--   5s, Repeat(10s,15s).
--   boss_massive_jormungar (26685), ScheduleTasks :509-511, arms :527-539:
--   EVENT_ACID_SPIT DoCastVictim(SPELL_ACID_SPIT 48132) ->
--   CastSpell(nil, 48132), init 6s, Repeat(7s); EVENT_ACID_SPLATTER
--   DoCastSelf(SPELL_ACID_SPLATTER 48136) -> CastSpell(creature, 48136),
--   init 16s, Repeat(16s); EVENT_POISON_BREATH SelectTarget(Random, 0)
--   -> DoCast(target, SPELL_POISON_BREATH 48133), 2-arg = non-triggered
--   -> CastSpell(target, 48133), init 13s, Repeat(14s).
-- The H_* heroic spell variants (59268/59267/59273/59265/59264/59263/
-- 59272/59271) are unmodeled (no difficulty bridge — the vazruden /
-- thespia precedent).
-- DOCUMENTED DEVIATION: the awakening preamble is unbridged — the orb
-- BasicEvent machines (OrbAirPositionEvent / OrbFlyEvent /
-- OrbFinalPositionEvent), the SPELL_AWAKEN_SUBBOSS 47669 /
-- SPELL_AWAKEN_GORTOK 47670 SpellScripts, the DoAction(ACTION_START_
-- FIGHT) unfreeze/unflag/DoZoneInCombat legs (C++: PalehoofMinionsBossAI
-- ::DoAction / CombatStartEvent BasicEvent), the miniboss JustDied ->
-- DoAction(ACTION_NEXT_PHASE) cross-creature cascade, the
-- EnterEvadeMode cross-creature cascade, the jormungar JustSummoned
-- worm machine (WormAttackEvent + MoveRandom 5.0f), and the
-- go_palehoof_sphere OnGossipHello -> DoAction(ACTION_START_ENCOUNTER)
-- GameObject leg — timers therefore schedule on combat start (event 1)
-- rather than after the 8.5s awaken preamble; the kits themselves are
-- unchanged.
-- DOCUMENTED-ONLY (in this header; no registration beyond the five
-- entries above):
-- boss_palehoof JustDied (:250-256) — DoPlaySoundToSet(PALEHOOF_SOUND_
-- DEATH 13467) with no Talk ported via creature:PlayDirectSound
-- (dedicated palehoofDied); the instance SetBossState(DONE) +
-- SendEncounterUnit disengage legs have no instance bridges.
-- PalehoofMinionsBossAI Reset (:329-335) — DoCastSelf(SPELL_FREEZE
-- 16245, true) is triggered — triggered casts stay out (the emerald
-- MARK_OF_NATURE_AURA precedent); the unfreeze/DoZoneInCombat legs
-- ride the unbridged DoAction machine (see DEVIATION above).
-- The orb / awaken-spell / sphere DoAction machine (see DEVIATION
-- above), spell_palehoof_crazed / spell_palehoof_crazed_effect /
-- spell_palehoof_awaken_subboss / spell_palehoof_awaken_gortok
-- (SpellScriptLoader / AuraScript hooks) — join the no-SpellScript /
-- no-AuraScript-bridge queues.

local ENTRY_GORTOK_PALEHOOF = 26687

local SOUND_PALEHOOF_DEATH = 13467
local ENTRY_FRENZIED_WORGEN = 26683
local ENTRY_RAVENOUS_FURBOLG = 26684
local ENTRY_MASSIVE_JORMUNGAR = 26685
local ENTRY_FEROCIOUS_RHINO = 26686

local SAY_AGGRO = 0
local SAY_SLAY = 1

local SPELL_ARCING_SMASH = 48260
local SPELL_IMPALE = 48261
local SPELL_WITHERING_ROAR = 48256

local SPELL_CRAZED = 48139
local SPELL_CHAIN_LIGHTNING = 48140
local SPELL_TERRIFYING_ROAR = 48144

local SPELL_MORTAL_WOUND = 48137
local SPELL_ENRAGE_1 = 48138
local SPELL_ENRAGE_2 = 48142

local SPELL_GORE = 48130
local SPELL_GRIEVOUS_WOUND = 48105
local SPELL_STOMP = 48131

local SPELL_ACID_SPIT = 48132
local SPELL_ACID_SPLATTER = 48136
local SPELL_POISON_BREATH = 48133

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

-- GetPlayersInWorld random-target helper — the baltharus /
-- ENERVATING_BRAND / thorim colossus precedent (alive, same map and
-- instance; maxDist nil = no distance filter, matching C++ SelectTarget
-- dist 0 = unlimited range — the yogg_saron randomPlayer precedent).
local function randomPlayerTarget(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead()
                and (maxDist == nil or creature:IsWithinDist(plr, maxDist)) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Shared timer wipe for events 2/23/4 on timer-carrying entries.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ EVENT_ARCING_SMASH arm (:236-239): DoCastVictim(SPELL_ARCING_SMASH
-- 48260), 1-arg = non-triggered; init 7s, Repeat(7s).
local function arcingSmashTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARCING_SMASH)
    schedule(guid, "arcingsmash", 7000, function()
        arcingSmashTick(creature, guid)
    end)
end

-- C++ EVENT_IMPALE arm (:240-244): SelectTarget(Random, 0) (dist 0 =
-- unlimited range) -> DoCast(target, SPELL_IMPALE 48261), 2-arg =
-- non-triggered; init 11s, Repeat(10s,15s).
local function impaleTick(creature, guid)
    local target = randomPlayerTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_IMPALE)
    end
    schedule(guid, "impale", {10000, 15000}, function()
        impaleTick(creature, guid)
    end)
end

-- C++ EVENT_WITHERING_ROAR arm (:245-248): DoCastSelf
-- (SPELL_WITHERING_ROAR 48256), 1-arg = non-triggered; init 12s,
-- Repeat(11s).
local function witheringRoarTick(creature, guid)
    creature:CastSpell(creature, SPELL_WITHERING_ROAR)
    schedule(guid, "witheringroar", 11000, function()
        witheringRoarTick(creature, guid)
    end)
end

-- C++ boss_palehoofAI::JustEngagedWith (:221-229): Talk(SAY_AGGRO) —
-- unconditional (the auriaya engage-port precedent; BossAI::JustEngagedWith
-- passthrough and the SendEncounterUnit leg have no bridges) — plus the
-- three event-schedule inits (:226-228) carried on event 1 on the xt002
-- precedent.
local function palehoofEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcingsmash", 7000, function()
        arcingSmashTick(creature, guid)
    end)
    schedule(guid, "impale", 11000, function()
        impaleTick(creature, guid)
    end)
    schedule(guid, "witheringroar", 12000, function()
        witheringRoarTick(creature, guid)
    end)
end

-- C++ KilledUnit (:269-273): if (who->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent; the
-- nil-guard is the hodir-port convention (Eluna event 3 always carries a
-- victim, so C++-exactness is unaffected).
local function palehoofKilledUnit(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    creature:Talk(SAY_SLAY)
end

-- C++ boss_ravenous_furbolgAI::ScheduleTasks (:377-379) — ungated;
-- carried here on event 1.
local function furbolgEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function crazedTick()
        creature:CastSpell(creature, SPELL_CRAZED)
        schedule(guid, "crazed", {20000, 25000}, crazedTick)
    end
    local function chainLightningTick()
        creature:CastSpell(nil, SPELL_CHAIN_LIGHTNING)
        schedule(guid, "chainlightning", 11000, chainLightningTick)
    end
    local function terrifyingRoarTick()
        creature:CastSpell(creature, SPELL_TERRIFYING_ROAR)
        schedule(guid, "terrifyingroar", 18000, terrifyingRoarTick)
    end
    schedule(guid, "crazed", 10000, crazedTick)
    schedule(guid, "chainlightning", 12000, chainLightningTick)
    schedule(guid, "terrifyingroar", 22000, terrifyingRoarTick)
end

-- C++ boss_frenzied_worgenAI::ScheduleTasks (:421-423) — ungated;
-- carried here on event 1.
local function worgenEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function mortalWoundTick()
        creature:CastSpell(nil, SPELL_MORTAL_WOUND)
        schedule(guid, "mortalwound", 6000, mortalWoundTick)
    end
    local function enrageTick()
        creature:CastSpell(creature, SPELL_ENRAGE_1)
        schedule(guid, "enrage", 25000, enrageTick)
    end
    schedule(guid, "mortalwound", 6000, mortalWoundTick)
    schedule(guid, "enrage", 16000, enrageTick)
    schedule(guid, "enrage2", 90000, function()
        creature:CastSpell(creature, SPELL_ENRAGE_2)
    end)
end

-- C++ boss_ferocious_rhinoAI::ScheduleTasks (:464-466) — ungated;
-- carried here on event 1.
local function rhinoEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function goreTick()
        creature:CastSpell(nil, SPELL_GORE)
        schedule(guid, "gore", 19000, goreTick)
    end
    local function grievousWoundTick()
        local target = randomPlayerTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_GRIEVOUS_WOUND)
        end
        schedule(guid, "grievouswound", 18000, grievousWoundTick)
    end
    local function stompTick()
        creature:CastSpell(creature, SPELL_STOMP)
        schedule(guid, "stomp", {10000, 15000}, stompTick)
    end
    schedule(guid, "gore", 10000, goreTick)
    schedule(guid, "grievouswound", 12000, grievousWoundTick)
    schedule(guid, "stomp", 5000, stompTick)
end

-- C++ boss_massive_jormungarAI::ScheduleTasks (:509-511) — ungated;
-- carried here on event 1.
local function jormungarEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local function acidSpitTick()
        creature:CastSpell(nil, SPELL_ACID_SPIT)
        schedule(guid, "acidspit", 7000, acidSpitTick)
    end
    local function acidSplatterTick()
        creature:CastSpell(creature, SPELL_ACID_SPLATTER)
        schedule(guid, "acidsplatter", 16000, acidSplatterTick)
    end
    local function poisonBreathTick()
        local target = randomPlayerTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_POISON_BREATH)
        end
        schedule(guid, "poisonbreath", 14000, poisonBreathTick)
    end
    schedule(guid, "acidspit", 6000, acidSpitTick)
    schedule(guid, "acidsplatter", 16000, acidSplatterTick)
    schedule(guid, "poisonbreath", 13000, poisonBreathTick)
end

-- C++ boss_palehoofAI::JustDied (:250-256): events.Reset() +
-- instance->SetBossState(DATA_GORTOK_PALEHOOF, DONE) (no instance
-- bridge) + DoPlaySoundToSet(me, PALEHOOF_SOUND_DEATH 13467) +
-- SendEncounterUnit disengage (no bridge); no Talk arms.
local function palehoofDied(event, creature, killer)
    creature:PlayDirectSound(SOUND_PALEHOOF_DEATH)
    cancelCombatTimers(event, creature)
end

RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 1, palehoofEnterCombat)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 3, palehoofKilledUnit)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 4, palehoofDied)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 1, furbolgEnterCombat)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 1, worgenEnterCombat)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 1, rhinoEnterCombat)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 23, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 1, jormungarEnterCombat)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 4, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 23, cancelCombatTimers)
