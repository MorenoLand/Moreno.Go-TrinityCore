-- General Vezax (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_general_vezax.cpp
-- (boss_general_vezax (CreatureScript) via
-- RegisterUlduarCreatureAI<boss_general_vezaxAI> (BossAI),
-- BOSS_VEZAX = 11 (ulduar.h:47) — registered from inside
-- AddSC_boss_general_vezax(); the SIXTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_xt002(); the call after it is
-- AddSC_boss_assembly_of_iron() — loader order confirmed).
-- Entry: 33271 General Vezax (ulduar.h NPC_VEZAX, line 81 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent);
-- the CreatureScript ScriptName binding is DB-side as usual.
-- Talk ids (VezaxYells, boss_general_vezax.cpp:28-36, verified
-- this run): SAY_AGGRO 0, SAY_SLAY 1, SAY_SURGE_OF_DARKNESS 2,
-- SAY_DEATH 3, SAY_BERSERK 4, SAY_HARDMODE 5. Emotes
-- (VezaxEmotes, boss_general_vezax.cpp:38-48): EMOTE_ANIMUS 6,
-- EMOTE_BARRIER 7, EMOTE_SURGE_OF_DARKNESS 8.
-- Sole-source verified: whole-server-tree grep for each of the
-- nine script names ("boss_general_vezax",
-- "boss_saronite_animus", "npc_saronite_vapors",
-- "spell_general_vezax_mark_of_the_faceless",
-- "spell_general_vezax_mark_of_the_faceless_leech",
-- "spell_general_vezax_saronite_vapors", "achievement_shadowdodger",
-- "achievement_smell_saronite") hits boss_general_vezax.cpp
-- (+ the loader decl/call lines for AddSC_boss_general_vezax)
-- only; zero sql/ hits for all nine. The vezax lua existed from a
-- prior session; parity audited and closed that run (the xt002
-- 22:38 / ignis 22:29 / krystallus 21:59 / sjonnir 22:04
-- precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timer-driven casts ride the
-- CreateLuaEvent convention (baltharus / maiden / auriaya
-- precedent): module-local GUID-keyed timers table, cancelTimers
-- on events 2/4/23, schedule(guid, key, delayMs, fn);
-- creature:CastSpell(nil, spell) = DoCastVictim (2-arg,
-- non-triggered), creature:CastSpell(creature, spell) =
-- DoCastSelf / DoCastAOE, third arg true = triggered,
-- {minMs, maxMs} = randtime re-arm. Melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_general_vezax.cpp):
-- JustEngagedWith (:134-148) — Talk(SAY_AGGRO 0) + DoCast(me,
-- SPELL_AURA_OF_DESPAIR 62692) 2-arg non-triggered self-cast
-- (:139) at engage (event 1; the BossAI::JustEngagedWith
-- passthrough, the CheckShamanisticRage (player-shaman
-- SPELL_SHAMANTIC_RAGE 30823 -> SPELL_CORRUPTED_RAGE 68415)
-- map-player-scan leg and the four unbridgeable ScheduleEvent
-- legs have no bridges — the malygos engage-aura precedent).
-- EVENT_SURGE_OF_DARKNESS (:188-193) — Talk
-- (EMOTE_SURGE_OF_DARKNESS 8) + Talk(SAY_SURGE_OF_DARKNESS 2) +
-- DoCast(me, SPELL_SURGE_OF_DARKNESS 62662) 2-arg non-triggered,
-- 60s init (:146), re-arm {50s,70s} (:192) — no phase/machine
-- leg touches it (the vapor hard-mode leg only cancels
-- EVENT_SARONITE_VAPORS and EVENT_SEARING_FLAMES) — Talk order
-- (emote, say, cast) kept.
-- EVENT_BERSERK (:209-212) — Talk(SAY_BERSERK 4) + DoCast(me,
-- SPELL_BERSERK 26662) 2-arg non-triggered, 600s init (:147), no
-- re-arm — Talk-before-cast order kept.
-- KilledUnit (:228-232) — player-gated Talk(SAY_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied (:234-239) — _JustDied() passthrough + Talk(SAY_DEATH
-- 3) + DoRemoveAurasDueToSpellOnPlayers(62692) leg — only the
-- Talk arm is bridgeable (event 4; the sjonnir JustDied-Talk
-- precedent); also wipes the surge/berserk timers.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33271):
-- Reset / Initialize — _Reset() + shadowDodger /
-- smellSaronite / animusDead / vaporCount init — no bridges.
-- SpellHitTarget — SPELL_SHADOW_CRASH_HIT 62659
-- TYPEID_PLAYER leg -> shadowDodger = false (consumer:
-- achievement_shadowdodger below) — no SpellHit bridge.
-- GetData(DATA_SHADOWDODGER 29962997 / DATA_SMELL_SARONITE
-- 31813188 — no GetData bridge; consumers: the two
-- achievements below); DoAction(ACTION_VAPORS_DIE 1 /
-- ACTION_ANIMUS_DIE 2 — no DoAction bridge; producers:
-- npc_saronite_vapors / boss_saronite_animus below).
-- CheckShamanisticRage — HasSpell(SPELL_SHAMANTIC_RAGE)
-- player-scan -> SPELL_CORRUPTED_RAGE — no map-player-scan
-- bridge.
-- EVENT_SHADOW_CRASH (:164-173) — CheckPlayersInRange(4/9,
-- 15.0f, 50.0f) / SelectTarget(Random, 150.0f) +
-- DoCast(target, SPELL_SHADOW_CRASH 62660), init {8s,10s}
-- (:142), re-arm {8s,12s} — random-target selection leg — no
-- target-search bridge; left out.
-- EVENT_SEARING_FLAMES (:174-177) — DoCastAOE
-- (SPELL_SEARING_FLAMES 62661), re-arm {14s,17500ms} — canceled
-- by the unbridgeable vapor hard-mode leg (animus phases; the
-- task-named exclusion) and rescheduled by the unbridgeable
-- ACTION_ANIMUS_DIE DoAction leg — left out.
-- EVENT_MARK_OF_THE_FACELESS (:178-187) —
-- CheckPlayersInRange / SelectTarget(Random) + DoCast(target,
-- SPELL_MARK_OF_THE_FACELESS 63276), init {35s,40s} (:144),
-- re-arm {35s,45s} — random-target selection leg; left out.
-- EVENT_SARONITE_VAPORS (:194-208) — DoCast
-- (SPELL_SUMMON_SARONITE_VAPORS 63081) self-cast (2-arg
-- DoCast(uint32) resolves SELF — UnitAI.cpp:113), 30s init
-- (:145), re-arm {30s,35s}, + the hard-mode machine (vaporCount
-- == 6 && smellSaronite -> Talk(SAY_HARDMODE 5) +
-- Talk(EMOTE_BARRIER 7) + summons.DespawnAll() + DoCast(me,
-- SPELL_SARONITE_BARRIER 63364) + DoCast
-- (SPELL_SUMMON_SARONITE_ANIMUS 63145) + LOOT_MODE_HARD_MODE_1 +
-- CancelEvent(EVENT_SARONITE_VAPORS/EVENT_SEARING_FLAMES)) —
-- summon leg + hard-mode phase machine; left out.
-- boss_saronite_animus (no creature-entry enum in C++ —
-- spell-summoned via 63145; entry-unverifiable — no
-- registration): Reset one-shot DoCast(me,
-- SPELL_VISUAL_SARONITE_ANIMUS 63319); EVENT_PROFOUND_OF_
-- DARKNESS — DoCastAOE(SPELL_PROFOUND_OF_DARKNESS 63420,
-- TRIGGERED), 3s init, 3s repeat — timer-driven but the add
-- exists only via the unbridged summon machine (task
-- exclusion: summon/add legs out); JustDied -> vezax DoAction
-- (ACTION_ANIMUS_DIE) — no DoAction bridge; no Talk arms.
-- npc_saronite_vapors (no creature-entry enum in C++ —
-- spell-summoned via 63081; entry-unverifiable — no
-- registration): Talk(EMOTE_VAPORS 0) in the constructor — no
-- spawn / constructor bridge; Reset EVENT_RANDOM_MOVE 5s-7.5s
-- -> MoveRandom(30.0f) — no scheduler / movement bridges;
-- DamageTaken (lethal -> damage = 0 + flags + root +
-- stand-state-dead + DoCast SPELL_SARONITE_VAPORS 63323 +
-- DespawnOrUnsummon(30s) + vezax DoAction(ACTION_VAPORS_DIE)) —
-- no damage-taken / DoAction bridges.
-- spell_general_vezax_mark_of_the_faceless (63276 — periodic
-- dummy AuraScript: caster casts
-- SPELL_MARK_OF_THE_FACELESS_DAMAGE 63278 with bp1 =
-- aurEff amount — no AuraScript bridge; joins the
-- no-AuraScript-bridge queue);
-- spell_general_vezax_mark_of_the_faceless_leech (63276/63134
-- target filter removing expl target, FinishCast on empty —
-- no SpellScript bridge; joins the no-SpellScript-bridge
-- queue); spell_general_vezax_saronite_vapors (63323 —
-- OnApply: bp * 2^stack mana restore -> SPELL_SARONITE_VAPORS_
-- ENERGIZE 63337 + DAMAGE 63338 at 2x — no AuraScript bridge;
-- joins the no-AuraScript-bridge queue).
-- achievement_shadowdodger (OnCheck: GetData
-- (DATA_SHADOWDODGER)); achievement_smell_saronite (OnCheck:
-- GetData(DATA_SMELL_SARONITE)) — no GetData / achievement
-- bridges; both join the unmodeled-achievement queue.

local ENTRY_GENERAL_VEZAX = 33271

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SURGE_OF_DARKNESS = 2
local SAY_DEATH = 3
local SAY_BERSERK = 4

local EMOTE_SURGE_OF_DARKNESS = 8

local SPELL_AURA_OF_DESPAIR = 62692
local SPELL_SURGE_OF_DARKNESS = 62662
local SPELL_BERSERK = 26662

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

-- C++ EVENT_SURGE_OF_DARKNESS (:188-193): Talk
-- (EMOTE_SURGE_OF_DARKNESS 8) + Talk(SAY_SURGE_OF_DARKNESS 2) +
-- DoCast(me, SPELL_SURGE_OF_DARKNESS 62662) (2-arg non-triggered);
-- 60s init (:146), re-arm {50s,70s} (:192) — Talk order kept.
local function surgeOfDarknessTick(creature, guid)
    creature:Talk(EMOTE_SURGE_OF_DARKNESS)
    creature:Talk(SAY_SURGE_OF_DARKNESS)
    creature:CastSpell(creature, SPELL_SURGE_OF_DARKNESS)
    schedule(guid, "surge", {50000, 70000}, function()
        surgeOfDarknessTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (:209-212): Talk(SAY_BERSERK 4) + DoCast(me,
-- SPELL_BERSERK 26662) (2-arg non-triggered); 600s init (:147),
-- no re-arm — Talk-before-cast order kept.
local function berserkTick(creature)
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ JustEngagedWith (:134-148): BossAI passthrough + Talk
-- (SAY_AGGRO) + DoCast(me, SPELL_AURA_OF_DESPAIR 62692) (:139,
-- 2-arg non-triggered self-cast) + CheckShamanisticRage +
-- ScheduleEvent legs — only the Talk arm, the engage aura cast
-- and the two bridgeable timer legs are modeled here (event 1).
local function vezaxEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_AURA_OF_DESPAIR)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "surge", 60000, function()
        surgeOfDarknessTick(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature)
    end)
end

-- C++ KilledUnit (:228-232): who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function vezaxTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:234-239): _JustDied() passthrough + Talk
-- (SAY_DEATH) + DoRemoveAurasDueToSpellOnPlayers(62692) — only
-- the Talk arm is bridgeable; the surge/berserk timers are wiped
-- with it.
local function vezaxDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
end

-- Shared timer wipe for events 2/23.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 1, vezaxEnterCombat)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 3, vezaxTargetDied)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 4, vezaxDied)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 23, cancelCombatTimers)
