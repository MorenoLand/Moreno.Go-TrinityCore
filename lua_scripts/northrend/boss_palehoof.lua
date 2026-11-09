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
-- registered this run for their timer arms) — entry-verifiable,
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
-- registered bridge, so the timer arms below are ported rather
-- than parked.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_palehoof.cpp):
-- boss_palehoof JustEngagedWith (:222-230, base-first) — BossAI::
-- JustEngagedWith arms SetBossState(DATA_GORTOK_PALEHOOF 1,
-- IN_PROGRESS) PORTED tick 261 via creature:SetBossState
-- (lua_creature_events.go:5917) + SetCombatPulseDelay(5) + me-form
-- DoZoneInCombat (ScriptedCreature.cpp:540/542) ported before
-- Talk(SAY_AGGRO 0) (event 1; the instance SendEncounterUnit
-- (ENCOUNTER_FRAME_ENGAGE) leg ports after the timer arms via
-- creature:SendEncounterUnit, bridge live 2026-11-05; the setActive
-- leg stays out) — plus the three timer arms (BossAI-base-gated, see
-- below), scheduled here on the xt002 precedent (C++ schedules on
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
-- The four minibosses' timer kits run under BossAI::UpdateAI base gates
-- (ScriptedCreature.cpp:578/:584; PalehoofMinionsBossAI overrides no
-- UpdateAI), so all twelve arms carry the 500ms IsCasting defer (the timer arms themselves
-- carry no phase groups and no DoAction gate; their engagement rides the
-- unported ACTION_START_FIGHT DoAction machine — see deviation below), ported
-- with per-entry event-1 scheduling carrying the C++ ScheduleTasks inits:
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
--   (UnitAI.h:170) — any threat-list unit, not player-only; the
--   offset-1 top-threat skip (UnitAI.h:172-174) ports via the
--   bridged threat-ordered GetAITargets (PORTED 2026-10-25; the
--   stale "no threat-list bridge" claim is corrected) —
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
-- 59272/59271) are unreferenced in the C++ — the enum decls (:34-65)
-- are the only hits; no arm ever selects them, so there is nothing to
-- port. (The old "no difficulty bridge — vazruden/thespia precedent"
-- claim is STALE: creature:IsHeroic() IS bridged
-- (engine/world/lua_creature_events.go:3728); it was unneeded for the
-- timer kits, but nextPhase below uses it for DUNGEON_MODE(2, 4).)
-- DOCUMENTED DEVIATION: the awakening preamble is PARTLY unbridged —
-- the SPELL_AWAKEN_SUBBOSS 47669 / SPELL_AWAKEN_GORTOK 47670
-- SpellScripts (CombatStartEvent -> DoAction(ACTION_START_FIGHT)
-- unfreeze/unflag/DoZoneInCombat legs, C++: PalehoofMinionsBossAI
-- ::DoAction / CombatStartEvent BasicEvent) and the SPELLVALUE_MAX_
-- TARGETS 1 mod on the awaken casts (Go CastSpell takes triggered
-- only) stay out; the orb BasicEvent machines (OrbAirPositionEvent /
-- OrbFlyEvent / OrbFinalPositionEvent), the miniboss JustDied ->
-- DoAction(ACTION_NEXT_PHASE) cross-creature cascade, and the
-- go_palehoof_sphere OnGossipHello -> DoAction(ACTION_START_ENCOUNTER)
-- GameObject leg are PORTED tick 405 (see palehoofReset /
-- nextPhase / startEncounter / onSphereGossip) — timers therefore
-- schedule on combat start (event 1) rather than after the 8.5s
-- awaken preamble; the kits themselves are unchanged. The jormungar
-- JustSummoned worm machine (WormAttackEvent + MoveRandom 5.0f;
-- WormAttackEvent::Execute's SetReactState(REACT_AGGRESSIVE) (:118)
-- rides the unbridged BasicEvent) stays out, as do the
-- spell_palehoof_crazed / spell_palehoof_crazed_effect /
-- spell_palehoof_awaken_subboss / spell_palehoof_awaken_gortok
-- (SpellScriptLoader / AuraScript hooks) — the crazed aura-remove leg
-- IS ported below via RegisterAuraEvent; the rest join the
-- no-SpellScript / no-AuraScript-bridge queues.
-- DOCUMENTED-ONLY (in this header; no registration beyond the five
-- entries above):
-- boss_palehoof JustDied (:254-260) — DoPlaySoundToSet(PALEHOOF_SOUND_
-- DEATH 13467) with no Talk ported via creature:PlayDirectSound
-- (dedicated palehoofDied); the instance SetBossState(DONE) leg ports
-- via creature:SetBossState (bridge live 2026-11-05) and the
-- SendEncounterUnit disengage leg via creature:SendEncounterUnit.
-- EnterEvadeMode (:262-267) — SendEncounterUnit disengage ports on
-- event 2 (dedicated palehoofLeaveCombat); the summons.DespawnAll /
-- _DespawnAtEvade legs stay out (no bridges). C++ EnterEvadeMode does
-- NOT call _Reset/BossAI::Reset, so ev2 carries no SetBossState leg;
-- the _DespawnAtEvade FAIL leg rides the ev23 state-aware leg in
-- palehoofReset (IN_PROGRESS -> FAIL, spawn path -> NOT_STARTED).
-- boss_palehoof Reset (:200-210) — _Reset() SetBossState
-- (DATA_GORTOK_PALEHOOF 1, NOT_STARTED) PORTED tick 261 via
-- creature:SetBossState (lua_creature_events.go:5917), first per C++
-- order (_Reset :202 runs before the sphere re-arm), != DONE-gated
-- per ScriptedCreature.cpp:509-510 (creature:GetBossState bridged,
-- lua_creature_events.go:5936; the hakkar precedent). The
-- SummonCreatureGroup(1) + _orb capture + _encountersCount = 0 legs
-- are PORTED tick 405 (see palehoofReset / palehoofJustSummoned); the
-- miniboss JustDied relay and sphere gossip legs are PORTED tick 405
-- too (see minibossDied / nextPhase / onSphereGossip). The minibosses'
-- own boss states are untouched in C++ and in Lua: PalehoofMinionsBossAI
-- overrides Reset (:329-335), JustEngagedWith (:336-341) and JustDied
-- (:353-357) WITHOUT base calls, so their DATA_* states never move —
-- no SetBossState legs to port for 26683/26684/26685/26686 (verified
-- tick 261).
-- PalehoofMinionsBossAI Reset (:329-335) — DoCastSelf(SPELL_FREEZE
-- 16245, true) PORTED as a triggered self-cast on event 23
-- (minibossReset; the anubrekhan/auriaya/ichoron triggered-3rd-arg
-- precedent — the old "triggered casts stay out" header claim is
-- STALE); the matching ACTION_START_FIGHT RemoveAurasDueToSpell
-- (:341-345) is ported via creature:RemoveAura at combat start; the
-- SetImmuneToPC/DoZoneInCombat legs ride the unbridged DoAction
-- machine (see DEVIATION above; re-evaluated 2026-10-23, stays out).
-- The awaken SpellScripts (spell_palehoof_awaken_subboss /
-- spell_palehoof_awaken_gortok) join the no-SpellScript-bridge queue;
-- the orb / sphere DoAction legs are PORTED tick 405 (see DEVIATION
-- above); spell_palehoof_crazed / spell_palehoof_crazed_effect
-- (SpellScriptLoader / AuraScript hooks) stay out except the crazed
-- aura-remove leg ported below via RegisterAuraEvent.

local ENTRY_GORTOK_PALEHOOF = 26687
local DATA_GORTOK_PALEHOOF = 1 -- utgarde_pinnacle.h:32

local SOUND_PALEHOOF_DEATH = 13467
local GO_GORTOK_PALEHOOF_SPHERE = 188593 -- utgarde_pinnacle.h:94
local ENTRY_FRENZIED_WORGEN = 26683
local ENTRY_RAVENOUS_FURBOLG = 26684
local ENTRY_MASSIVE_JORMUNGAR = 26685
local NPC_JORMUNGAR_WORM = 27228 -- utgarde_pinnacle.h:67
local ENTRY_FEROCIOUS_RHINO = 26686

local SAY_AGGRO = 0
local SAY_SLAY = 1

local SPELL_ARCING_SMASH = 48260
local SPELL_IMPALE = 48261
local SPELL_WITHERING_ROAR = 48256

local SPELL_CRAZED = 48139
local SPELL_CRAZED_TAUNT = 48147 -- boss_palehoof.cpp:602 (SPELL_CRAZED_TAUNT)
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

-- C++ orb / awaken ids (boss_palehoof.cpp:39-42, :103): NPC_PALEHOOF_ORB
-- 22515 (utgarde_pinnacle.h:66); the orb is summoned by Reset's
-- SummonCreatureGroup(1) and captured by entry.
local ENTRY_PALEHOOF_ORB = 22515
local SPELL_ORB_VISUAL = 48044
local SPELL_AWAKEN_SUBBOSS = 47669
local SPELL_AWAKEN_GORTOK = 47670

-- C++ PalehoofMinionsBossAI::Reset (:329-335): DoCastSelf(SPELL_FREEZE
-- 16245, true) — triggered self-cast, re-applied on every reset/evade
-- (the anubrekhan triggered-self-cast precedent); removed at
-- ACTION_START_FIGHT (:341-345) via RemoveAurasDueToSpell.
local SPELL_FREEZE = 16245
local SPELL_ORB_CHANNEL = 48048 -- boss_palehoof.cpp:40

local timers = {}

-- Orb + encounter state, keyed by palehoof GUID (the timers-table
-- precedent): orbs[] holds the captured orb creature (Reset
-- SummonCreatureGroup return + ev19 JustSummoned), encounters[] the
-- _encountersCount, groupSummons[] the last summoned group, for the
-- _Reset() summons.DespawnAll twin before each re-summon.
local orbs = {}
local encounters = {}
local groupSummons = {}

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
-- C++ SelectTarget(Random, 0) with playerOnly default FALSE: any
-- alive threat-list unit (the keleseth GetAITargets precedent).
local function randomPlayerTarget(creature, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead()
                and (maxDist == nil or creature:IsWithinDist(t, maxDist)) then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1) (:478): offset 1 pops the current victim
-- (UnitAI.h non-distance list is victim-first), not the threat-sorted
-- head: exclude by victim GUID; the rest of the threat list is the pool.
local function randomThreatTargetSkippingFirst(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Shared timer wipe for events 2/23/4 on timer-carrying entries.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ PalehoofMinionsBossAI::EnterEvadeMode (:359-362): a miniboss
-- evade cascades to palehoof. EnterEvadeMode bridged
-- (lua_creature_events.go:5014).
local function minibossLeaveCombat(event, creature)
    cancelCombatTimers(event, creature)
    local palehoof = creature:FindNearestCreature(ENTRY_GORTOK_PALEHOOF, 100.0)
    if palehoof then
        palehoof:EnterEvadeMode()
    end
end

local jormungarWorms = {}

-- C++ boss_massive_jormungarAI::EnterEvadeMode (:545-548): worms
-- despawn first (summons.DespawnAll), then the cascade above.
local function jormungarLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    local worms = jormungarWorms[guid]
    if worms then
        for _, worm in ipairs(worms) do
            worm:DespawnOrUnsummon()
        end
        jormungarWorms[guid] = nil
    end
    minibossLeaveCombat(event, creature)
end

-- C++ boss_massive_jormungarAI::JustSummoned (:514-520): worm entry
-- 27228 -> WormAttackEvent at 2s (SetReactState(REACT_AGGRESSIVE) +
-- SetTempSummonType(CORPSE_DESPAWN = 5) + DoZoneInCombat) +
-- MoveRandom(5.0) now.
local function jormungarJustSummoned(event, creature, summoned)
    if summoned:GetEntry() ~= NPC_JORMUNGAR_WORM then
        return
    end
    summoned:MoveRandom(5.0)
    local guid = creature:GetGUIDLow()
    local worms = jormungarWorms[guid]
    if worms == nil then
        worms = {}
        jormungarWorms[guid] = worms
    end
    worms[#worms + 1] = summoned
    CreateLuaEvent(function()
        summoned:SetReactState(2) -- REACT_AGGRESSIVE
        summoned:SetTempSummonType(5) -- TEMPSUMMON_CORPSE_DESPAWN
        summoned:DoZoneInCombat()
    end, 2000)
end

-- C++ PalehoofMinionsBossAI::Reset (:329-335) — DoCastSelf(SPELL_FREEZE
-- 16245, true): the triggered freeze is re-applied on every reset/evade
-- (event 23 fires first on the Go respawn/death/evade paths; no 23 fire
-- at first grid spawn, so the spawn-time freeze visual stays unmodeled).
local function minibossReset(event, creature)
    -- C++ Reset never runs on death; Go ev23 pre-fires on death.
    if not creature:IsAlive() then
        return
    end
    creature:SetCombatPulseDelay(0) -- boss_palehoof.cpp:331 (base Reset arm 1; pulse disable)
    creature:CastSpell(creature, SPELL_FREEZE, true)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ EVENT_ARCING_SMASH arm (:236-239): DoCastVictim(SPELL_ARCING_SMASH
-- 48260), 1-arg = non-triggered; init 7s, Repeat(7s).
local function arcingSmashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "arcingsmash", 500, function()
            arcingSmashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_ARCING_SMASH)
    schedule(guid, "arcingsmash", 7000, function()
        arcingSmashTick(creature, guid)
    end)
end

-- C++ EVENT_IMPALE arm (:240-244): SelectTarget(Random, 0) (dist 0 =
-- unlimited range) -> DoCast(target, SPELL_IMPALE 48261), 2-arg =
-- non-triggered; init 11s, Repeat(10s,15s).
local function impaleTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "impale", 500, function()
            impaleTick(creature, guid)
        end)
        return
    end
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
    if creature:IsCasting() then
        schedule(guid, "witheringroar", 500, function()
            witheringRoarTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_WITHERING_ROAR)
    schedule(guid, "witheringroar", 11000, function()
        witheringRoarTick(creature, guid)
    end)
end

-- C++ boss_palehoofAI::JustEngagedWith (:222-230): BossAI::JustEngagedWith
-- base-first — SetBossState(DATA_GORTOK_PALEHOOF 1, IN_PROGRESS) PORTED
-- tick 261 via creature:SetBossState (lua_creature_events.go:5917) +
-- SetCombatPulseDelay(5) + me-form DoZoneInCombat
-- (ScriptedCreature.cpp:540/542) ported above Talk; SendEncounterUnit
-- ports after the timer arms below — C++ sends it last, :229);
-- Talk(SAY_AGGRO) — unconditional (the auriaya
-- engage-port precedent) — plus the three event-schedule inits (:226-228)
-- carried on event 1 on the xt002 precedent.
local function palehoofEnterCombat(event, creature, target)
    -- BossAI::JustEngagedWith (:224, base-first) — SetBossState
    -- (DATA_GORTOK_PALEHOOF 1, IN_PROGRESS) PORTED tick 261 via
    -- creature:SetBossState (lua_creature_events.go:5917) +
    -- SetCombatPulseDelay(5) + me-form DoZoneInCombat
    -- (ScriptedCreature.cpp:540/542).
    creature:SetBossState(DATA_GORTOK_PALEHOOF, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUIDLow()
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
    creature:SendEncounterUnit(0, creature) -- ENCOUNTER_FRAME_ENGAGE (boss_palehoof.cpp:229)
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

-- C++ boss_ravenous_furbolgAI::ScheduleTasks (:377-379) — BossAI-base-gated;
-- carried here on event 1.
local function furbolgEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_palehoof.cpp:338 (base JustEngagedWith arm 1; setActive leg has no bridge)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ PalehoofMinionsBossAI::DoAction(ACTION_START_FIGHT) (:341-345):
    -- RemoveAurasDueToSpell(SPELL_FREEZE) — the unfreeze leg (the
    -- SetImmuneToPC/DoZoneInCombat legs stay out — re-evaluated
    -- 2026-10-23 against the live bridge: DoAction machine).
    creature:RemoveAura(SPELL_FREEZE)
    local function crazedTick()
        if creature:IsCasting() then
            schedule(guid, "crazed", 500, crazedTick)
            return
        end
        creature:CastSpell(creature, SPELL_CRAZED)
        schedule(guid, "crazed", {20000, 25000}, crazedTick)
    end
    local function chainLightningTick()
        if creature:IsCasting() then
            schedule(guid, "chainlightning", 500, chainLightningTick)
            return
        end
        creature:CastSpell(nil, SPELL_CHAIN_LIGHTNING)
        schedule(guid, "chainlightning", 11000, chainLightningTick)
    end
    local function terrifyingRoarTick()
        if creature:IsCasting() then
            schedule(guid, "terrifyingroar", 500, terrifyingRoarTick)
            return
        end
        creature:CastSpell(creature, SPELL_TERRIFYING_ROAR)
        schedule(guid, "terrifyingroar", 18000, terrifyingRoarTick)
    end
    schedule(guid, "crazed", 10000, crazedTick)
    schedule(guid, "chainlightning", 12000, chainLightningTick)
    schedule(guid, "terrifyingroar", 22000, terrifyingRoarTick)
end

-- C++ boss_frenzied_worgenAI::ScheduleTasks (:421-423) — BossAI-base-gated;
-- carried here on event 1.
local function worgenEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_palehoof.cpp:338 (base JustEngagedWith arm 1; setActive leg has no bridge)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:RemoveAura(SPELL_FREEZE)
    local function mortalWoundTick()
        if creature:IsCasting() then
            schedule(guid, "mortalwound", 500, mortalWoundTick)
            return
        end
        creature:CastSpell(nil, SPELL_MORTAL_WOUND)
        schedule(guid, "mortalwound", 6000, mortalWoundTick)
    end
    local function enrageTick()
        if creature:IsCasting() then
            schedule(guid, "enrage", 500, enrageTick)
            return
        end
        creature:CastSpell(creature, SPELL_ENRAGE_1)
        schedule(guid, "enrage", 25000, enrageTick)
    end
    schedule(guid, "mortalwound", 6000, mortalWoundTick)
    schedule(guid, "enrage", 16000, enrageTick)
    local function enrage2Tick()
        if creature:IsCasting() then
            schedule(guid, "enrage2", 500, enrage2Tick)
            return
        end
        creature:CastSpell(creature, SPELL_ENRAGE_2)
    end
    schedule(guid, "enrage2", 90000, enrage2Tick)
end

-- C++ boss_ferocious_rhinoAI::ScheduleTasks (:464-466) — BossAI-base-gated;
-- carried here on event 1.
local function rhinoEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_palehoof.cpp:338 (base JustEngagedWith arm 1; setActive leg has no bridge)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:RemoveAura(SPELL_FREEZE)
    local function goreTick()
        if creature:IsCasting() then
            schedule(guid, "gore", 500, goreTick)
            return
        end
        creature:CastSpell(nil, SPELL_GORE)
        schedule(guid, "gore", 19000, goreTick)
    end
    local function grievousWoundTick()
        if creature:IsCasting() then
            schedule(guid, "grievouswound", 500, grievousWoundTick)
            return
        end
        local target = randomThreatTargetSkippingFirst(creature)
        if target then
            creature:CastSpell(target, SPELL_GRIEVOUS_WOUND)
        end
        schedule(guid, "grievouswound", 18000, grievousWoundTick)
    end
    local function stompTick()
        if creature:IsCasting() then
            schedule(guid, "stomp", 500, stompTick)
            return
        end
        creature:CastSpell(creature, SPELL_STOMP)
        schedule(guid, "stomp", {10000, 15000}, stompTick)
    end
    schedule(guid, "gore", 10000, goreTick)
    schedule(guid, "grievouswound", 12000, grievousWoundTick)
    schedule(guid, "stomp", 5000, stompTick)
end

-- C++ boss_massive_jormungarAI::ScheduleTasks (:509-511) — BossAI-base-gated;
-- carried here on event 1.
local function jormungarEnterCombat(event, creature)
    creature:SetCombatPulseDelay(5) -- boss_palehoof.cpp:338 (base JustEngagedWith arm 1; setActive leg has no bridge)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:RemoveAura(SPELL_FREEZE)
    local function acidSpitTick()
        if creature:IsCasting() then
            schedule(guid, "acidspit", 500, acidSpitTick)
            return
        end
        creature:CastSpell(nil, SPELL_ACID_SPIT)
        schedule(guid, "acidspit", 7000, acidSpitTick)
    end
    local function acidSplatterTick()
        if creature:IsCasting() then
            schedule(guid, "acidsplatter", 500, acidSplatterTick)
            return
        end
        creature:CastSpell(creature, SPELL_ACID_SPLATTER)
        schedule(guid, "acidsplatter", 16000, acidSplatterTick)
    end
    local function poisonBreathTick()
        if creature:IsCasting() then
            schedule(guid, "poisonbreath", 500, poisonBreathTick)
            return
        end
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

-- C++ boss_palehoofAI::Reset (:200-210): _Reset + SummonCreatureGroup
-- (PORTED tick 405, see palehoofReset below) + sphere re-arm: instance->GetGameObject(
-- DATA_GORTOK_PALEHOOF_SPHERE) -> SetGoState(GO_STATE_READY) +
-- RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE), via the
-- kirtonos/gandling FindNearestGameObject precedent (100y room scale;
-- C++ uses the instance GUID, Lua the nearest entry match). The _Reset()
-- SetBossState(DATA_GORTOK_PALEHOOF 1, NOT_STARTED) leg is PORTED tick
-- 261 via creature:SetBossState (lua_creature_events.go:5917), first
-- per C++ order (_Reset :202 runs before the sphere re-arm), !=
-- DONE-gated per ScriptedCreature.cpp:509-510 (creature:GetBossState
-- bridged, lua_creature_events.go:5936; the hakkar precedent).
-- Evade correction (in-house): C++ EnterEvadeMode (:262-267) overrides
-- without Reset and ends in _DespawnAtEvade() -> FAIL
-- (ScriptedCreature.cpp:617), so the evade-path fire (stored state
-- IN_PROGRESS) sets FAIL; the spawn-path fire sets NOT_STARTED. The
-- death pre-fire is skipped via the IsDead guard (DONE lands in ev4).
local function palehoofReset(event, creature)
    if not creature:IsDead() then
        if creature:GetBossState(DATA_GORTOK_PALEHOOF) == 1 then
            creature:SetBossState(DATA_GORTOK_PALEHOOF, 2) -- FAIL: evade path
        elseif creature:GetBossState(DATA_GORTOK_PALEHOOF) ~= 3 then
            creature:SetBossState(DATA_GORTOK_PALEHOOF, 0) -- NOT_STARTED: spawn path
        end
    end
    cancelCombatTimers(event, creature)
    -- C++ Reset (:202-205): _Reset + _orb.Clear +
    -- SummonCreatureGroup(SUMMON_MINIBOSSES_GROUP = 1) +
    -- _encountersCount = 0 — PORTED tick 405 (SummonCreatureGroup
    -- bridge, lua_creature_events.go:3567, returns the summoned
    -- creatures; the orb is captured from the return array by entry,
    -- and again via ev19 JustSummoned below, the C++ capture path).
    -- C++ order: the summon runs before the sphere re-arm below.
    -- The _Reset() summons.DespawnAll twin runs first: without it an
    -- evade-reset would duplicate the group (DespawnOrUnsummon on the
    -- tracked TempSummons is the twin, nil-safe when a summon is
    -- already gone). Alive-gated: Go ev23 fires on death too (the
    -- magtheridon reset-then-died order), and C++ Reset never runs on
    -- death.
    local guid = creature:GetGUIDLow()
    orbs[guid] = nil
    encounters[guid] = 0
    if creature:IsAlive() then
        local previous = groupSummons[guid]
        if previous then
            for _, summon in ipairs(previous) do
                summon:DespawnOrUnsummon()
            end
        end
        groupSummons[guid] = nil
        local summoned = creature:SummonCreatureGroup(1)
        if summoned then
            groupSummons[guid] = summoned
            for _, summon in ipairs(summoned) do
                if summon:GetEntry() == ENTRY_PALEHOOF_ORB then
                    orbs[guid] = summon
                end
            end
        end
        local sphere = creature:FindNearestGameObject(GO_GORTOK_PALEHOOF_SPHERE, 100.0)
        if sphere then
            sphere:SetGoState(1) -- GO_STATE_READY
            sphere:RemoveFlag(9, 16) -- GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE
        end
    end
end

-- C++ boss_palehoofAI::JustSummoned (:214-218): BossAI::JustSummoned
-- + if (summon->GetEntry() == NPC_PALEHOOF_ORB) _orb =
-- summon->GetGUID() — PORTED tick 405 on ev19 (the summon funnel
-- fires CreatureEventOnJustSummonedCreature on the summoner,
-- lua_creature_events.go:190). The BossAI::JustSummoned leg is the
-- summons.Insert tracking, unmodeled — groupSummons[] above is this
-- file's equivalent.
local function palehoofJustSummoned(event, creature, summon)
    if summon:GetEntry() == ENTRY_PALEHOOF_ORB then
        orbs[creature:GetGUIDLow()] = summon
    end
end

-- C++ boss_palehoofAI::DoAction(ACTION_NEXT_PHASE) (:280-291): orb
-- lookup via _orb; _encountersCount++; at _dungeonMode (2 normal / 4
-- heroic, DUNGEON_MODE(2, 4)) the orb casts SPELL_AWAKEN_GORTOK
-- 47670, else SPELL_AWAKEN_SUBBOSS 47669 — both triggered; the
-- subboss cast carries SPELLVALUE_MAX_TARGETS 1 in C++, which the Go
-- CastSpell bridge cannot express (3rd arg is triggered only,
-- lua_creature_events.go:891) — documented deviation, the cast still
-- fires. The downstream awaken SpellScripts (47669/47670 ->
-- CombatStartEvent -> ACTION_START_FIGHT) stay out (no SpellScript
-- bridge), so the awakened miniboss never unfreezes via this path;
-- its timer kit still arms on ev1 combat as before.
local function nextPhase(palehoof)
    local guid = palehoof:GetGUIDLow()
    local count = (encounters[guid] or 0) + 1
    encounters[guid] = count
    local orb = orbs[guid]
    if orb == nil then
        return
    end
    local mode = 2
    if palehoof:IsHeroic() then
        mode = 4
    end
    if count == mode then
        orb:CastSpell(orb, SPELL_AWAKEN_GORTOK, true)
    else
        orb:CastSpell(orb, SPELL_AWAKEN_SUBBOSS, true)
    end
end

-- C++ PalehoofMinionsBossAI::JustDied (:353-357): instance->
-- GetCreature(DATA_GORTOK_PALEHOOF) -> DoAction(ACTION_NEXT_PHASE)
-- — PORTED tick 405 (the tick-404 U3 stay-out unblocker, now that the
-- orb spawns): the palehoof lookup re-expresses the instance
-- GetCreature via FindNearestCreature (the boss_skadi Grauf
-- precedent); the DoAction machine itself stays out, the NEXT_PHASE
-- leg runs inline above. No BossAI::JustDied base call in C++ (no
-- SetBossState leg for the minibosses).
local function minibossDied(event, creature, killer)
    cancelCombatTimers(event, creature)
    local palehoof = creature:FindNearestCreature(ENTRY_GORTOK_PALEHOOF, 100.0)
    if palehoof == nil then
        return
    end
    nextPhase(palehoof)
end

-- C++ boss_palehoofAI::DoAction(ACTION_START_ENCOUNTER) (:300-305):
-- orb casts SPELL_ORB_VISUAL 48044 (triggered) and queues the orb
-- BasicEvent machines — PORTED tick 405 as CreateLuaEvent timers on
-- the captured orb (the BasicEvent twin): OrbAirPositionEvent at 3s
-- (:143-157, SetWalk(true) + MovePoint(0, OrbPositions[POSITION_FLY]
-- 238.6077, -460.7103, 112.5671)); OrbFlyEvent at 6s (:159-170,
-- SetWalk(false) + MovePoint(0, OrbPositions[POSITION_FINAL] 279.26,
-- -452.1, 110.0)); OrbFinalPositionEvent at +10s (:128-135, orb
-- casts SPELL_AWAKEN_SUBBOSS 47669 triggered, MAX_TARGETS 1 —
-- unexpressible, see nextPhase above). MoveTo is the MovePoint twin
-- (lua_creature_events.go:4623).
local function startEncounter(palehoof)
    local orb = orbs[palehoof:GetGUIDLow()]
    if orb == nil then
        return
    end
    -- Guid-keyed schedule() (tick 519): bare CreateLuaEvent timers
    -- were never cancelled on ev2/4/23, so a second sphere gossip
    -- stacked duplicates; C++ BasicEvents die with the orb.
    local guid = palehoof:GetGUIDLow()
    orb:CastSpell(orb, SPELL_ORB_VISUAL, true)
    schedule(guid, "orbAir", 3000, function()
        orb:SetWalk(true)
        orb:MoveTo(0, 238.6077, -460.7103, 112.5671)
    end)
    schedule(guid, "orbFly", 6000, function()
        orb:SetWalk(false)
        orb:MoveTo(0, 279.26, -452.1, 110.0)
    end)
    schedule(guid, "orbFinal", 16000, function()
        orb:CastSpell(orb, SPELL_AWAKEN_SUBBOSS, true)
    end)
end

-- C++ go_palehoof_sphereAI::OnGossipHello (:571-585): palehoof alive
-- and boss state != DONE -> sphere NOT_SELECTABLE + GO_STATE_ACTIVE
-- + palehoof DoAction(ACTION_START_ENCOUNTER); returns true —
-- PORTED tick 405 on RegisterGameObjectGossipEvent (the go_barrel
-- precedent; handler args (event, player, go)). The palehoof lookup
-- re-expresses instance->GetCreature via FindNearestCreature.
local function onSphereGossip(event, player, go)
    -- C++ returns true unconditionally (:585); false lets native GO
    -- Use arms proceed (gameobjects.go:825-836). Only run the
    -- encounter when palehoof is alive and state != DONE.
    local palehoof = go:FindNearestCreature(ENTRY_GORTOK_PALEHOOF, 100.0)
    if palehoof ~= nil and palehoof:IsAlive()
            and palehoof:GetBossState(DATA_GORTOK_PALEHOOF) ~= 3 then
        go:SetFlag(9, 16) -- GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE
        go:SetGoState(0) -- GO_STATE_ACTIVE
        startEncounter(palehoof)
    end
    return true
end

-- C++ spell_palehoof_crazed_effect (:630-633): the hit unit casts
-- SPELL_CRAZED_TAUNT 48147 on the caster, triggered. RegisterSpellEvent
-- ev5 = OnEffectHitTarget (lua_spell.go:592); the blanket
-- "no-SpellScript-bridge" header claim is stale (gal_darah/novos
-- precedent).
RegisterSpellEvent(48139, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex == 0 and hitTarget ~= nil then
        local caster = spell:GetCaster()
        if caster ~= nil then
            hitTarget:CastSpell(caster, SPELL_CRAZED_TAUNT, true)
        end
    end
end)

-- C++ spell_palehoof_awaken_subboss (:662-668): caster casts
-- SPELL_ORB_CHANNEL 48048 on the hit unit; the hit unit drops
-- NOT_SELECTABLE; CombatStartEvent at 8500ms runs DoAction
-- (ACTION_START_FIGHT) = RemoveAurasDueToSpell(SPELL_FREEZE) +
-- SetImmuneToPC(false) + DoZoneInCombat + orb DespawnOrUnsummon(1s)
-- (:292-299). SetImmuneToPC has no bridge (stays out).
RegisterSpellEvent(47669, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil then
        caster:CastSpell(hitTarget, SPELL_ORB_CHANNEL)
    end
    hitTarget:RemoveFlag(59, 0x02000000) -- UNIT_FLAG_NOT_SELECTABLE
    CreateLuaEvent(function()
        hitTarget:RemoveAura(16245) -- SPELL_FREEZE
        hitTarget:DoZoneInCombat()
        if caster ~= nil then
            caster:DespawnOrUnsummon(1000)
        end
    end, 8500)
end)

-- C++ spell_palehoof_awaken_gortok (:692-696): same START_FIGHT
-- machine at 8000ms (orb = caster, despawns at 1s).
RegisterSpellEvent(47670, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveFlag(59, 0x02000000) -- UNIT_FLAG_NOT_SELECTABLE
    local orb = spell:GetCaster()
    CreateLuaEvent(function()
        hitTarget:RemoveAura(16245) -- SPELL_FREEZE
        hitTarget:DoZoneInCombat()
        if orb ~= nil then
            orb:DespawnOrUnsummon(1000)
        end
    end, 8000)
end)

-- C++ boss_palehoofAI::JustDied (:254-260): events.Reset() +
-- instance->SetBossState(DATA_GORTOK_PALEHOOF, DONE) + DoPlaySoundToSet(me, PALEHOOF_SOUND_DEATH 13467) +
-- SendEncounterUnit disengage; no Talk arms. Arm order is C++-exact.
local function palehoofDied(event, creature, killer)
    cancelCombatTimers(event, creature)
    creature:SetBossState(DATA_GORTOK_PALEHOOF, 3) -- DATA_GORTOK_PALEHOOF, DONE (boss_palehoof.cpp:257)
    creature:PlayDirectSound(SOUND_PALEHOOF_DEATH)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE (boss_palehoof.cpp:259)
    -- C++ instance_utgarde_pinnacle OnGameObjectCreate leg (sphere
    -- flagged NOT_SELECTABLE when GORTOK_PALEHOOF is DONE), run at
    -- DONE time: the static sphere never fires GO-create in Go
    -- (instance_hooks.go:306), and the Reset re-arm above already
    -- uses this same anchored lookup (100yd, same room).
    local sphere = creature:FindNearestGameObject(GO_GORTOK_PALEHOOF_SPHERE, 100.0)
    if sphere then
        sphere:SetFlag(9, 16) -- GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE
    end
end

-- C++ boss_palehoofAI::EnterEvadeMode (:262-267): disengage first,
-- then events.Reset + summons.DespawnAll + _DespawnAtEvade (no
-- bridges). Event 2 only — the minibosses keep the shared
-- cancelCombatTimers on event 2 (no SendEncounterUnit in their AI).
local function palehoofLeaveCombat(event, creature)
    creature:SendEncounterUnit(1, creature) -- ENCOUNTER_FRAME_DISENGAGE
    cancelCombatTimers(event, creature)
end

RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 1, palehoofEnterCombat)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 2, palehoofLeaveCombat)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 3, palehoofKilledUnit)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 4, palehoofDied)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 19, palehoofJustSummoned)
RegisterCreatureEvent(ENTRY_GORTOK_PALEHOOF, 23, palehoofReset)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 1, furbolgEnterCombat)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 2, minibossLeaveCombat)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 4, minibossDied)
RegisterCreatureEvent(ENTRY_RAVENOUS_FURBOLG, 23, minibossReset)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 1, worgenEnterCombat)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 2, minibossLeaveCombat)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 4, minibossDied)
RegisterCreatureEvent(ENTRY_FRENZIED_WORGEN, 23, minibossReset)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 1, rhinoEnterCombat)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 2, minibossLeaveCombat)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 4, minibossDied)
RegisterCreatureEvent(ENTRY_FEROCIOUS_RHINO, 23, minibossReset)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 1, jormungarEnterCombat)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 2, jormungarLeaveCombat)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 4, minibossDied)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 23, minibossReset)
RegisterCreatureEvent(ENTRY_MASSIVE_JORMUNGAR, 19, jormungarJustSummoned)

RegisterGameObjectGossipEvent(GO_GORTOK_PALEHOOF_SPHERE, 1, onSphereGossip)

-- C++ spell_palehoof_crazed_AuraScript::OnRemove (boss_palehoof.cpp:597-605):
-- EFFECT_1 SPELL_AURA_PERIODIC_TRIGGER_SPELL, AURA_EFFECT_HANDLE_REAL.
-- On remove the target drops its Crazed Taunt aura (48147) — the
-- RemoveAurasDueToSpell leg, covered by the target:RemoveAura
-- single-application twin (tick-197 precedent). Retires the census
-- BLOCKED row for 48139.
local function crazedRemove(event, aura, target)
    target:RemoveAura(SPELL_CRAZED_TAUNT)
end

RegisterAuraEvent(SPELL_CRAZED, 3, crazedRemove)
