-- Anub'Rekhan (Naxxramas) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_anubrekhan.cpp
-- (boss_anubrekhan (BossAI); at_anubrekhan_entrance
-- (OnlyOnceAreaTriggerScript); AddSC_boss_anubrekhan registers both;
-- loader decl 59 / call 254 per northrend_script_loader.cpp — the FIRST
-- Naxxramas group in AddNorthrendScripts(), immediately after
-- AddSC_instance_trial_of_the_crusader(), under the "// Naxxramas"
-- marker; the call after it is AddSC_boss_maexxna()).
-- Entry: 15956 Anub'Rekhan (naxxramas.h NPC_ANUBREKHAN — kalecgos
-- pass; instance_naxxramas.cpp binds NPC_ANUBREKHAN -> DATA_ANUBREKHAN
-- GUID (case NPC_ANUBREKHAN, line 133) and the boss index 30 ->
-- BOSS_ANUBREKHAN; the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_anubrekhan"
-- hits boss_anubrekhan.cpp only (loader carries only the decl/call
-- lines); grep for "at_anubrekhan_entrance" hits the .cpp only; zero
-- sql/ hits. No anubrekhan lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent). C++ randtime -> math.random
-- (amanitar / boss_black_knight precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_anubrekhan: JustEngagedWith Talk(SAY_AGGRO 0) (event 1; the
-- BossAI::JustEngagedWith instance-bookkeeping leg, summons.
-- DoZoneInCombat, events.SetPhase(PHASE_NORMAL) have no bridge —
-- tharon_ja precedent; re-evaluated against the live DoZoneInCombat
-- bridge 2026-10-23 and STAYS OUT: boss_anubrekhan.cpp:164 is
-- summons.DoZoneInCombat() (engages the summoned crypt guards), not a
-- me->DoZoneInCombat() on the boss itself); KilledUnit Talk(SAY_SLAY 2)
-- (event 3 —
-- C++-UNCONDITIONAL: the Talk fires for any victim; only the corpse
-- scarab cast below is player-gated; no nalorakk gate here);
-- EVENT_LOCUST — Talk(EMOTE_LOCUST 3) + DoCastSelf(Locust Swarm
-- 28785 / 25-man 54021, raid-moded 2026-10-25 — same Spell.cpp:524
-- auto-resolve evidence as impale; the old unconditional 28785
-- cast the 10-man spell on 25-man), init randtime(1m40s,2m), repeat
-- 1m30s (phase_hunter self-cast precedent); EVENT_BERSERK —
-- DoCastSelf(Berserk 27680),
-- 10min init, reschedules 10min (jaraxxus berserk precedent; C++
-- casts triggered — no spell-mod bridge, noted). All timers cancelled
-- on 2/4/23 (gargolmar precedent). No JustDied Talk in C++.
-- Unmodeled (no bridges — documented, not wired):
-- boss_anubrekhan intro/summon machine — InitializeAI / Reset
-- (guardCorpses.clear) / JustReachedHome (SummonGuards) — group spawn
-- with DB-side coords (no summon-group bridge; creature:SummonCreature
-- is live but takes explicit coords);
-- SummonGuards: Is25ManRaid() (now bridged, lua_creature_events.go) -> SummonCreatureGroup(GROUP_INITIAL_25M
-- 1) — same bar (IsHeroic() is bridged, lua_creature_events.go:260,
-- so difficulty is NOT the blocker); JustEngagedWith 10-man
-- EVENT_SPAWN_GUARD leg (randtime 15s-20s) — same bar; EVENT_SPAWN_GUARD
-- -> SummonCreatureGroup(GROUP_SINGLE_SPAWN 2) — same bar (DB-side
-- group coords);
-- JustSummoned PORTED 2026-10-24 (anubrekhanJustSummoned, ev19):
-- boss_anubrekhan.cpp:118-126 IsInCombat-gated crypt-guard
-- Talk(EMOTE_SPAWN 1) via the summoner-side ev19 hook (midnight /
-- jindo precedents; summoned object carries Talk); latent until
-- SummonCreatureGroup ports, entry-gated so inert otherwise.
-- SummonedCreatureDies/Despawn: guardCorpses GUID-set bookkeeping —
-- ev21 (SummonedCreatureDied) IS bridged (lua_creature_events.go:3672),
-- but the scarab port below re-derives the pool from the grid instead
-- of tracking GUIDs (see EVENT_SCARABS);
-- EVENT_IMPALE — PORTED 2026-10-25: SelectTarget(Random,0) ->
-- DoCast(Impale 28783 / 25-man 56090, raid-moded via
-- creature:Is25ManRaid() — C++ casts auto-resolve difficulty at
-- Spell::prepare, Spell.cpp:524, and Go has no cast-time difficulty
-- substitution), init randtime(10s,20s), repeat randtime(10s,20s).
-- Phase semantics are emulated, not skipped: EventMap::ExecuteEvent
-- (EventMap.cpp) ERASES a phase-mismatched event whose time expired
-- instead of deferring it, so an impale expiring during PHASE_SWARM
-- is DROPPED with no re-arm, and LOCUST_ENDS re-schedules impale
-- randtime(10s,20s) — the swarmEnd handler below models exactly
-- that. The anti-chain leg (GetTimeUntilEvent(EVENT_LOCUST) < 5s)
-- likewise drops without re-arm (the C++ break skips Repeat); it is
-- modeled with a locustSoon flag set 5s before each locust. The
-- empty-target leg still re-arms (Repeat sits after the if/else);
-- only its EnterEvadeMode stays out (no bridge, 2026-10-21).
-- Single-key-timer delta: a C++ impale still pending past swarm end
-- would coexist with the LOCUST_ENDS re-schedule (two interleaved
-- impale timers); the Lua harness replaces it — timing-only delta.
-- EVENT_SCARABS — PORTED 2026-10-26 (scarabTick below): random
-- guardCorpses corpse -> corpse CastSpell(self, 28864) +
-- Talk(EMOTE_SCARAB 2) + DespawnOrUnsummon, init randtime(20s,30s),
-- repeat randtime(40s,60s) unconditional (Repeat sits outside the
-- empty-check); PHASE_NORMAL-only with the EventMap erase rule
-- (swarm-expired drop, LOCUST_ENDS re-schedule 20s-30s);
-- EVENT_LOCUST / EVENT_LOCUST_ENDS phase legs — events.SetPhase(
-- PHASE_SWARM/PHASE_NORMAL) has no bridge, but its OBSERVABLE
-- effects on impale are modeled (swarm drop + LOCUST_ENDS impale
-- re-schedule, see EVENT_IMPALE above); the LOCUST_ENDS re-scheduling
-- of SCARABS is modeled in swarmEndTick below;
-- KilledUnit player-gated arm — victim->CastSpell(victim,
-- SPELL_SUMMON_CORPSE_SCARABS_PLR 29105, me->GetGUID()) — ported to the
-- event-3 handler below (moroes convention: creature casts on the victim,
-- TRIGGERED_FULL_MASK via the C++ ctor); JustDied — instance->DoStartTimedAchievement(
-- ACHIEVEMENT_TIMED_TYPE_EVENT, ACHIEV_TIMED_START_EVENT 9891, kill
-- Maexxna within 20 min) — no achievement bridge (instance model
-- absent). NPC_CRYPT_GUARD 16573 (naxxramas.h — kalecgos pass; no
-- registered script of its own — its Talk now rides the boss ev19
-- summon hook, PORTED 2026-10-24; the group spawn itself stays out).
-- at_anubrekhan_entrance (OnlyOnceAreaTriggerScript — Talk(SAY_GREET 1)
-- via instance GetGuidData(DATA_ANUBREKHAN), NOT_STARTED-gated) — no
-- area-trigger bridge — documented-only.

local ENTRY_ANUBREKHAN = 15956

local SAY_AGGRO = 0
local SAY_SLAY = 2
local EMOTE_LOCUST = 3
local EMOTE_SPAWN = 1 -- boss_anubrekhan.cpp:37
local NPC_CRYPT_GUARD = 16573 -- naxxramas.h:104

local SPELL_LOCUST_SWARM = 28785
local SPELL_LOCUST_SWARM_25 = 54021 -- boss_anubrekhan.cpp:52
local SPELL_IMPALE = 28783
local SPELL_IMPALE_25 = 56090 -- boss_anubrekhan.cpp:51
local SPELL_BERSERK = 27680
local SPELL_SUMMON_CORPSE_SCARABS_PLR = 29105 -- boss_anubrekhan.cpp:55
local SPELL_SUMMON_CORPSE_SCARABS_MOB = 28864 -- boss_anubrekhan.cpp:56
local EMOTE_SCARAB = 2 -- boss_anubrekhan.cpp:38

local timers = {}

-- Per-GUID phase emulation for impale: swarm = inside a locust
-- swarm (PHASE_SWARM), soon = a locust fires within 5s (anti-chain).
local swarmState = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    swarmState[guid] = nil
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

-- C++ EVENT_IMPALE (boss_anubrekhan.cpp:184-195): SelectTarget(
-- Random, 0) -> DoCast(Impale), init randtime(10s,20s), repeat
-- randtime(10s,20s). Threat-list pick (UnitAI.h:159, threat list;
-- offline refs skipped at build, alive filter here), any-unit
-- (playerOnly defaults false), no distance filter. Swarm/soon
-- drops re-arm nothing (EventMap erase rule / anti-chain break);
-- empty target still re-arms (Repeat after the if/else).
local function impaleTick(creature, guid)
    local st = swarmState[guid]
    if st and (st.swarm or st.soon) then
        return
    end
    local spell = SPELL_IMPALE
    if creature:Is25ManRaid() then
        spell = SPELL_IMPALE_25
    end
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], spell)
    end
    schedule(guid, "impale", math.random(10000, 20000), function()
        impaleTick(creature, guid)
    end)
end

-- C++ EVENT_SCARABS (boss_anubrekhan.cpp:197-208): random
-- guardCorpses corpse -> corpse CastSpell(self,
-- SUMMON_CORPSE_SCARABS_MOB 28864) + Talk(EMOTE_SCARAB 2) +
-- DespawnOrUnsummon, init randtime(20s,30s), repeat randtime(40s,60s)
-- unconditional (Repeat sits outside the empty-check). The C++
-- guardCorpses set (SummonedCreatureDies insert /
-- SummonedCreatureDespawn erase) is modeled by re-deriving the pool
-- from the grid each fire: dead creatures stay in the motion map for
-- their corpse window (kill.go corpse-removal phase), so
-- GetCreaturesInGrid(16573) + not-IsAlive == the set in practice;
-- the 150yd bound is a room-scale approximation of the range-free set
-- (guards die in combat near the boss; crypt guards exist only as
-- boss summons, so no foreign entry-16573 corpses pollute the pool).
-- PHASE_NORMAL-only: a scarab expiring during PHASE_SWARM is dropped
-- with no re-arm (EventMap erase rule, see EVENT_IMPALE above).
-- OrigCaster attribution (me->GetGUID()) has no Lua slot (29105
-- precedent above); the creature cast path executes the summon
-- effect, so the scarabs still spawn.
local function scarabTick(creature, guid)
    local st = swarmState[guid]
    if st and st.swarm then
        return
    end
    local corpses = {}
    for _, c in ipairs(creature:GetCreaturesInGrid(NPC_CRYPT_GUARD, 150)) do
        if not c:IsAlive() then
            corpses[#corpses + 1] = c
        end
    end
    if #corpses > 0 then
        local corpse = corpses[math.random(#corpses)]
        corpse:CastSpell(corpse, SPELL_SUMMON_CORPSE_SCARABS_MOB)
        corpse:Talk(EMOTE_SCARAB)
        corpse:Despawn()
    end
    schedule(guid, "scarab", math.random(40000, 60000), function()
        scarabTick(creature, guid)
    end)
end

-- C++ EVENT_LOCUST_ENDS (boss_anubrekhan.cpp:220-222): phase back
-- to NORMAL + re-schedule impale randtime(10s,20s) + scarabs
-- randtime(20s,30s).
local function swarmEndTick(creature, guid)
    local st = swarmState[guid]
    if st then
        st.swarm = false
    end
    schedule(guid, "impale", math.random(10000, 20000), function()
        impaleTick(creature, guid)
    end)
    schedule(guid, "scarab", math.random(20000, 30000), function()
        scarabTick(creature, guid)
    end)
end

-- C++ EVENT_LOCUST: Talk(EMOTE_LOCUST 3) + DoCastSelf(Locust Swarm
-- 28785 / 25-man 54021), init randtime(1m40s,2m), repeat 1m30s.
-- LOCUST_ENDS at RAID_MODE(19s, 23s) via the swarmEnd timer.
local function locustTick(creature, guid)
    creature:Talk(EMOTE_LOCUST)
    local spell = SPELL_LOCUST_SWARM
    if creature:Is25ManRaid() then
        spell = SPELL_LOCUST_SWARM_25
    end
    creature:CastSpell(creature, spell)
    local st = swarmState[guid]
    if not st then
        st = {}
        swarmState[guid] = st
    end
    st.swarm = true
    st.soon = false
    local swarmEnd = 19000
    if creature:Is25ManRaid() then
        swarmEnd = 23000
    end
    schedule(guid, "swarmEnd", swarmEnd, function()
        swarmEndTick(creature, guid)
    end)
    schedule(guid, "locust", 90000, function()
        locustTick(creature, guid)
    end)
    schedule(guid, "locustSoon", 85000, function()
        local s2 = swarmState[guid]
        if s2 then
            s2.soon = true
        end
    end)
end

-- C++ EVENT_BERSERK: DoCastSelf(Berserk 27680, triggered), 10min
-- init, reschedules 10min.
local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature, guid)
    end)
end

local function anubrekhanEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    swarmState[guid] = { swarm = false, soon = false }
    local locustDelay = math.random(100000, 120000)
    schedule(guid, "locust", locustDelay, function()
        locustTick(creature, guid)
    end)
    schedule(guid, "locustSoon", locustDelay - 5000, function()
        local st = swarmState[guid]
        if st then
            st.soon = true
        end
    end)
    schedule(guid, "impale", math.random(10000, 20000), function()
        impaleTick(creature, guid)
    end)
    schedule(guid, "scarab", math.random(20000, 30000), function()
        scarabTick(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature, guid)
    end)
end

local function anubrekhanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit (boss_anubrekhan.cpp:143-149): TYPEID_PLAYER-gated
-- victim->CastSpell(victim, SPELL_SUMMON_CORPSE_SCARABS_PLR,
-- me->GetGUID()) + unconditional Talk(SAY_SLAY 2). player:CastSpell
-- is bridged (lua_player.go:309) but the boss cast STAYS: 29105
-- spawns 5 corpse scarabs (summon effect) and the session cast path
-- executes no SPELL_EFFECT_SUMMON while the creature path does
-- (kurinnaxx precedent) — switching would lose the scarabs; the
-- original-caster GUID attribution is likewise unbridgeable.
local function anubrekhanTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        -- C++ boss_anubrekhan.cpp:146: victim->CastSpell(victim,
        -- SPELL_SUMMON_CORPSE_SCARABS_PLR, me->GetGUID()) — but
        -- player:CastSpell is bridged (lua_player.go:309), but the
        -- boss cast stays: 29105 spawns scarabs (summon effect, no
        -- session-path executor) and the C++ origCaster attribution
        -- has no Lua slot (caster deviates from C++).
        -- The C++ CastSpellExtraArgs(origCaster) ctor sets
        -- TRIGGERED_FULL_MASK (SpellDefines.h:167), so triggered=true.
        creature:CastSpell(victim, SPELL_SUMMON_CORPSE_SCARABS_PLR, true)
    end
    creature:Talk(SAY_SLAY)
end

local function anubrekhanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ JustSummoned (boss_anubrekhan.cpp:118-126): BossAI latch +
-- IsInCombat gate + crypt-guard entry gate -> summon Talk(EMOTE_SPAWN
-- 1). The Talk's second arg (me, the whisper target) has no Lua twin
-- — single-arg Talk per the midnight precedent. Latent until
-- SummonCreatureGroup ports: no Lua path summons guards yet.
local function anubrekhanJustSummoned(_, creature, summoned)
    if creature:IsInCombat() and summoned:GetEntry() == NPC_CRYPT_GUARD then
        summoned:Talk(EMOTE_SPAWN)
    end
end

local function anubrekhanReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ANUBREKHAN, 1, anubrekhanEnterCombat)
RegisterCreatureEvent(ENTRY_ANUBREKHAN, 2, anubrekhanLeaveCombat)
RegisterCreatureEvent(ENTRY_ANUBREKHAN, 3, anubrekhanTargetDied)
RegisterCreatureEvent(ENTRY_ANUBREKHAN, 4, anubrekhanDied)
RegisterCreatureEvent(ENTRY_ANUBREKHAN, 19, anubrekhanJustSummoned)
RegisterCreatureEvent(ENTRY_ANUBREKHAN, 23, anubrekhanReset)
