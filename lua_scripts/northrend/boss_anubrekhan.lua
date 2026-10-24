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
-- 28785), init randtime(1m40s,2m), repeat 1m30s (phase_hunter
-- self-cast precedent); EVENT_BERSERK — DoCastSelf(Berserk 27680),
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
-- SummonedCreatureDies/Despawn:
-- guardCorpses GUID-set bookkeeping — no consumer bridge;
-- EVENT_IMPALE — SelectTarget(Random,0) -> DoCast(Impale 28783
-- / 25-man 56090), init randtime(10s,20s), repeat randtime(10s,20s),
-- with the anti-chain leg (GetTimeUntilEvent(EVENT_LOCUST) < 5s ->
-- skip) — STAYS OUT: phase-gated behind events.SetPhase(PHASE_NORMAL)
-- (no phase bridge — gothik precedent); the random-target leg itself
-- is bridgeable via randomPlayer (keleseth / thorim / archavon
-- precedent), but the phase gate rules.
-- EVENT_SCARABS — random guardCorpses corpse ->
-- creatureTarget->CastSpell(SUMMON_CORPSE_SCARABS_MOB 28864) +
-- Talk(EMOTE_SCARAB 2) + DespawnOrUnsummon, init randtime(20s,30s),
-- repeat randtime(40s,60s) — cross-AI corpse cast/Talk/despawn +
-- guardCorpses GUID bookkeeping, no bridges (creature:SummonCreature /
-- creature:Despawn are live on the boss surface, not cross-AI);
-- EVENT_LOCUST / EVENT_LOCUST_ENDS phase legs — events.SetPhase(
-- PHASE_SWARM/PHASE_NORMAL) and the PHASE_NORMAL gating of IMPALE /
-- SCARABS — no phase bridge; only the unmodeled arms are phase-gated
-- so the modeled LOCUST timer emulates unhindered (documented); the
-- LOCUST_ENDS re-scheduling of IMPALE/SCARABS has no modeled content;
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
local SPELL_BERSERK = 27680
local SPELL_SUMMON_CORPSE_SCARABS_PLR = 29105 -- boss_anubrekhan.cpp:55

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

-- C++ EVENT_LOCUST: Talk(EMOTE_LOCUST 3) + DoCastSelf(Locust Swarm
-- 28785), init randtime(1m40s,2m), repeat 1m30s. (events.SetPhase(
-- PHASE_SWARM/PHASE_NORMAL) and LOCUST_ENDS have no modeled content —
-- the phase gating only affects the unmodeled IMPALE/SCARABS arms.)
local function locustTick(creature, guid)
    creature:Talk(EMOTE_LOCUST)
    creature:CastSpell(creature, SPELL_LOCUST_SWARM)
    schedule(guid, "locust", 90000, function()
        locustTick(creature, guid)
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
    schedule(guid, "locust", math.random(100000, 120000), function()
        locustTick(creature, guid)
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
-- me->GetGUID()) + unconditional Talk(SAY_SLAY 2). victim:CastSpell
-- has no bridge (player object), so the boss casts on the victim
-- instead (moroes convention); the original-caster GUID attribution
-- of the 3rd arg is unbridgeable.
local function anubrekhanTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        -- C++ boss_anubrekhan.cpp:146: victim->CastSpell(victim,
        -- SPELL_SUMMON_CORPSE_SCARABS_PLR, me->GetGUID()) — but
        -- player:CastSpell has no bridge (CastSpell is registered for
        -- creature objects only, engine/world/lua_creature_events.go:65),
        -- so the victim-self-cast would nil-call on the first player
        -- kill; the boss casts on the victim instead (caster deviates
        -- from C++; the original-caster GUID attribution is unbridgeable).
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
