-- Hodir (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_hodir.cpp
-- (boss_hodir (CreatureScript) via GetUlduarAI<boss_hodirAI>
-- (BossAI), BOSS_HODIR = 7 — registered from inside
-- AddSC_boss_hodir(); loader decl 119 / call 314 per
-- northrend_script_loader.cpp — the TENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_mimiron() (decl 118 / call 313); the call
-- after it is AddSC_boss_freya() — loader order confirmed
-- 2026-10-06 in the 23:03 EDT audit.
-- Entry: 32845 Hodir (ulduar.h NPC_HODIR, line 78 —
-- entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos
-- precedent); the CreatureScript ScriptName binding is
-- DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of
-- the twelve script names ("boss_hodir", "npc_icicle",
-- "npc_snowpacked_icicle", "npc_hodir_priest",
-- "npc_hodir_shaman", "npc_hodir_druid", "npc_hodir_mage",
-- "npc_toasty_fire", "npc_ice_block", "npc_flash_freeze",
-- "spell_biting_cold", "spell_biting_cold_dot") hits
-- boss_hodir.cpp (+ the loader decl/call lines for
-- AddSC_boss_hodir) only; zero sql/ hits for all twelve.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 22 OnSummoned,
-- 23 OnReset.
-- Timer bridge: CreateLuaEvent harness, GUID-keyed timers
-- table, cancel on events 2/4/23 (the maiden_of_virtue /
-- moroes / curator / nightbane precedent). CastSpell(nil,
-- spell) = DoCastVictim; CastSpell(creature, spell) =
-- DoCastSelf; DoCastAOE lowers to self-cast (boss_ai.go
-- DoCastAOE lowering; the selin_fireheart / mr_smite
-- precedent). The 3rd CastSpell arg is the C++ DoCast 3rd
-- arg (2-arg / 1-arg overloads = non-triggered). {min,max}
-- table delay = randtime range.
-- Ported arms (all C++-exact for the modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; C++ lines
-- 370); DoCast(me, SPELL_BITING_COLD 62038, true) (line
-- 371) -> CastSpell(creature, 62038, true); BossAI
-- passthrough has no bridge.
-- EVENT_FREEZE (id 1; init 25s at line 380, arm at
-- 444-446): DoCastAOE(SPELL_FREEZE 62469), 1-arg =
-- non-triggered -> CastSpell(creature, 62469); repeat
-- 30-45s.
-- EVENT_BLOWS (id 5; init 60-65s at line 381, arm at
-- 473-477): Talk(SAY_STALACTITE 3) + Talk(EMOTE_BLOWS 8) +
-- DoCast(me, SPELL_FROZEN_BLOWS 62478), 2-arg =
-- non-triggered -> CastSpell(creature, 62478); repeat
-- 60-65s.
-- EVENT_FLASH_FREEZE (id 2; init 45s at line 382, arm at
-- 453-455): Talk(SAY_FLASH_FREEZE 2) + Talk(EMOTE_FREEZE
-- 7) only — the random-target SPELL_ICICLE_SNOWDRIFT loop
-- (lines 456-458) stays out on bridge grounds: the CASTER is the random
-- player (target->CastSpell(target, ...)), and CastSpell on the Lua
-- surface is a creature-object method only (lua_creature_events.go:60) —
-- no player-cast bridge. The bare DoCast(SPELL_FLASH_FREEZE
-- 61968) (line 459, AI-target-driven per AISpellInfo), and
-- the whole EVENT_FLASH_FREEZE_EFFECT machinery (lines
-- 462-470: grid creature-list, SPELL_FLASH_FREEZE_VISUAL
-- 62148, FlashFreeze() summon legs) have no bridges;
-- repeat 25-35s is taken from the EFFECT reschedule (line
-- 470), which is the C++-exact Talk cadence.
-- EVENT_RARE_CACHE (id 6; init 3min at line 383, arm at
-- 479-483): Talk(SAY_HARD_MODE_FAILED 6) only — the
-- iCouldSayThatThisCacheWasRare flag clear (line 481) and
-- instance->SetData(DATA_HODIR_RARE_CACHE, 0) (line 482)
-- have no bridges; fires once (CancelEvent, line 483).
-- EVENT_BERSERK (id 7; init 8min at line 384, arm at
-- 485-488): Talk(SAY_BERSERK 5) + DoCast(me,
-- SPELL_BERSERK 47008, true) -> CastSpell(creature, 47008,
-- true); fires once (CancelEvent, line 488).
-- EVENT_ICICLE (id 4; init 2s at line 379, arm at 448-451):
-- SelectTarget(Random, 0, 100.0f, true) playerOnly=TRUE (jaraxxus /
-- coren_direbrew precedent) -> DoCast(target, SPELL_ICICLE 62234)
-- non-triggered, null-guarded like the C++ if; repeat 5500ms
-- (RAID_MODE 10-man value — the 10-vs-25-man raid-size selector has no
-- bridge (IsHeroic() is normal-vs-heroic only); the re-arm
-- ScheduleEvent sits outside the target check, so it is unconditional).
-- KilledUnit — player-gated Talk(SAY_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER, C++ lines 389-391 —
-- the razuvious player-gated variant precedent).
-- DamageTaken — lethal (damage >= health) -> Talk(SAY_DEATH
-- 4) + damage = 0 (event 9; C++ lines 396-399; the numeric
-- second return rewrites damage C++-exact — the moroes
-- damage-rewrite precedent).
-- Cast-state gates PORTED 2026-11-04: the six boss timer arms
-- (icicle / freeze / blows / flash-freeze / rare-cache /
-- berserk) defer 500ms on creature:IsCasting() (live,
-- lua_creature_events.go:1950), no repeat re-arm consumed —
-- the mal_ganis idiom for the boss UpdateAI pre-loop (:437)
-- + in-loop (:492) CASTING gates. The stale "timers fire
-- unconditionally" deviation above is retracted for these arms.
-- Melee stays engine-driven, like C++ DoMeleeAttackIfReady.
-- Ungated per C++: flash-freeze add UpdateAI (:210, no gate),
-- icicle add UpdateAI (:583, plain timer), snowpacked-icicle
-- UpdateAI (:638, plain expiry), toasty-fire (no UpdateAI).
-- Priest/shaman/druid/mage add gates are PORTED tick 476 (IsInCombat
-- + HasFlag stun twin, 1s poll while gated; CASTING gates ride the
-- 500ms IsCasting defer) — see the helper block below.
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entry 32845):
-- JustEngagedWith achievement flags (gettingColdInHereTimer
-- / gettingColdInHere / cheeseTheFreeze /
-- iHaveTheCoolestFriends / iCouldSayThatThisCacheWasRare,
-- lines 373-377) — the four booleans ARE modeled as per-GUID
-- hodirLatches (set true at engage, cleared at Reset/leave/die
-- and by the DoAction receiver, tick 476); the
-- gettingColdInHereTimer poll and the instance-SetData readers
-- have no bridges.
-- DamageTaken lethal companion legs still documented-only:
-- MoveIdle / SetControlled stun / MotionMaster Clear /
-- instance->SetData(DATA_HODIR_RARE_CACHE) (line 400) — no
-- bridges. RemoveAllAttackers / InterruptNonMeleeSpells /
-- StopMoving / CombatStop are PORTED in hodirDamageTaken. The rest of the
-- machine is ported in hodirDamageTaken (2026-10-21): the stale
-- "SetFaction unbridged" claim that used to gate it is retracted
-- (creature:SetFaction is bridged, lua_creature_events.go:1091),
-- so the arm now terminates C++-exact — kill-credit, friendly
-- faction, 10s despawn — instead of leaving an unkillable boss.
-- DamageTaken lethal instance->SetData(DATA_HODIR_RARE_CACHE,
-- 1) (line 400) — no instance SetData bridge; joins the
-- no-SetData-bridge queue.
-- DamageTaken else-leg (lines 421-426): the REACT_AGGRESSIVE
-- arm is PORTED in hodirDamageTaken (!IsInCombat gate; ev9 fires
-- pre-aggro so the gate is reachable); DoZoneInCombat (:425) is
-- PORTED 2026-10-23 via the live bridge (C++ arm order — after
-- SetReactState); AttackStart(who) has no bridge — the engine
-- converges on the same tick via triggerCreatureAggro.
-- gettingColdInHere UpdateAI leg (lines 491-499:
-- PvE-combat-ref iteration, SPELL_BITING_COLD_TRIGGERED
-- 62039 aura-stack check, SetData(DATA_GETTING_COLD_IN_HERE))
-- — achievement / instance-data leg, no bridges.
-- DoAction arms (lines 511-522:
-- ACTION_I_HAVE_THE_COOLEST_FRIENDS /
-- ACTION_CHEESE_THE_FREEZE) — PORTED tick 476 (hodirDoAction, ev38):
-- per-GUID latch clears in hodirLatches (set true at engage,
-- cleared at Reset/leave/die); the latch readers are the future Go
-- criteria cases (2963/2965 + 2961/2962).
-- Reset (lines 359-366): _Reset() + SetReactState REACT_PASSIVE
-- + the frozen-helper summon loop (PORTED in hodirReset, below;
-- the stale "no bridge" claim is retracted); the ctor's
-- SetReactState(REACT_PASSIVE) (line 336) rides the same ev23
-- hook (Eluna On_Reset fires at spawn).
-- npc_hodir_priest / npc_hodir_shaman / npc_hodir_druid /
-- npc_hodir_mage — PORTED tick 476 (helper block below):
-- priest EVENT_HEAL DoCastAOE 62809 (lines 696-698) +
-- HealthBelowPct(30) self-heal poll (line 687) + JustDied -> hodir
-- DoAction(1) (:723); shaman EVENT_STORM_CLOUD random-player
-- triggered 65123 (:772-774) + JustDied send (:788); druid
-- EVENT_STARLIGHT self-cast triggered 62807 (:837-838) + JustDied
-- send (:852); mage EVENT_CONJURE_FIRE summon-list-size gate +
-- triggered 62823 (:915-919) + EVENT_MELT_ICE
-- FindNearestCreature(FLASH_FREEZE 32926, 50yd) + triggered 64528
-- (:921-923) + JustDied send (:935). STAY-OUTS: priest
-- EVENT_DISPEL_MAGIC grid search + DoCast(SPELL_DISPEL_MAGIC 63499,
-- true) (lines 700-709 — no friendly-grid bridge); all four
-- DoSpellAttackIfReady combat legs (SPELL_SMITE 61923 /
-- SPELL_LAVA_BURST 61924 / SPELL_WRATH 62793 / SPELL_FIREBALL 61909 —
-- engine combat, no bridge).
-- npc_flash_freezeAI — PORTED (flashFreezeSummoned, below):
-- IsSummonedBy (lines 232-249): AddThreat(summoner, 250) +
-- DoCast(target, SPELL_BLOCK_OF_ICE 61969, true) (line 243) +
-- NearTeleportTo the target's position + player-gated Hodir
-- DoAction(ACTION_CHEESE_THE_FREEZE) (lines 246-248). ev22 fires
-- on the summoned creature with (event, creature, summoner) from
-- the summonTempCreature fire site (lua_creature_events.go:198);
-- the stale "summon legs have no bridge" claim is retracted.
-- SetInCombatWith has no bridge — the AddThreat engage leg covers
-- it (Go AddThreat syncs TargetGUID + ATTACK_START on a switch).
-- UpdateAI despawn/die checks (lines 209-232) stay out — separate
-- arm, not this shape.
-- npc_ice_blockAI — PORTED (iceBlockSummoned, below):
-- IsSummonedBy (lines 278-296): summoner stun/root legs +
-- AddThreat(summoner, 250) + the who-redirect AddThreat(me, 250,
-- summoner) + the creature-summoner-only DoCast(target,
-- SPELL_FLASH_FREEZE_HELPER 61990, true) (line 292) +
-- NearTeleportTo. SetInCombatWith has no bridge (same AddThreat
-- delta as above). DamageTaken -> hodir REACT_AGGRESSIVE /
-- DoZoneInCombat / AttackStart (lines 301-318) stays out —
-- AttackStart has no bridge.
-- npc_icicleAI — icicleTimer arm (lines 584-600): entry
-- 33169 NPC_ICICLE -> DoCast(me, SPELL_ICICLE_FALL 69428)
-- + DoCast(me, SPELL_FALL_DAMAGE 62236, true); entry 33173
-- NPC_ICICLE_SNOWDRIFT -> DoCast(me,
-- SPELL_FALL_SNOWDRIFT 62460) + DoCast(me, SPELL_ICICLE_FALL
-- 69428) — plain timer self-casts but on summoned icicle
-- adds; adds legs BLOCKED per doctrine; no registration.
-- npc_snowpacked_icicleAI — PORTED (snowpackedIcicleReset /
-- snowpackedIcicleDied, below): ctor SetFlag(NON_ATTACKABLE |
-- NOT_SELECTABLE | PACIFIED) + SetControlled(root) + SetReactState
-- (REACT_PASSIVE); Reset re-inits the 12s despawnTimer; UpdateAI
-- expiry -> FindNearestGameObject(GO_SNOWDRIFT 194173, 2.0) +
-- RemoveGameObject(go, true) + DespawnOrUnsummon (lines 632-642).
-- GO removal rides the GO RemoveFromWorld hide-in-instance idiom
-- (spotlight precedent); the C++ owner check inside RemoveGameObject
-- is unmodeled (no Go GO owner read for nearest GOs). Ctor
-- SetDisplayId(Modelid2) ported in snowpackedIcicleReset via
-- GetTemplateModelId(2) (bridge live). The add
-- registers in this file — summoned adds need no separate boss_ai.go
-- registration (yogg-guardian precedent).
-- npc_toasty_fireAI — PORTED (toastyFireReset /
-- toastyFireSpellHit, below): Reset -> triggered self-cast SINGED
-- 62821 (line 967); SpellHit (BLOCK_OF_ICE 61969 / ICE_SHARD 62457 /
-- ICE_SHARD_HIT 65370) -> FindNearestGameObject(GO_TOASTY_FIRE
-- 194300, 1.0) + RemoveGameObject(go, true) + DespawnOrUnsummon
-- (lines 970-978). The header's "RemoveGameObject unbridged"
-- claim is RETIRED: GO RemoveFromWorld (hide-in-instance,
-- lua_objects.go:1407, spotlight precedent) is the Go removal idiom.
-- Ctor SetDisplayId(Modelid2) ported in toastyFireReset via
-- GetTemplateModelId(2) (bridge live).
-- spell_biting_cold / spell_biting_cold_dot — PORTED tick 367
-- via RegisterAuraEvent (the Nov-3 "no AuraScript bridge" claim is
-- RETIRED: the periodic hook fires per aura tick on the player path
-- and the tick-366 creature:IsMoving bridge gates both arms; the
-- achievement legs — DATA_GETTING_COLD_IN_HERE stack check,
-- ACTION_CHEESE_THE_FREEZE / ACTION_I_HAVE_THE_COOLEST_FRIENDS —
-- still join the unmodeled-achievement queue).
-- Achievement data legs (DATA_GETTING_COLD_IN_HERE /
-- DATA_HODIR_RARE_CACHE — the C++ @todo names Storm
-- Cloud and Toasty Fires) — no GetData / instance-SetData /
-- achievement bridges; they join the unmodeled-achievement
-- queue. The ACTION_CHEESE_THE_FREEZE /
-- ACTION_I_HAVE_THE_COOLEST_FRIENDS DoAction legs ARE ported
-- (hodirDoAction receiver, per-GUID hodirLatches); only the Go
-- criteria cases reading them (2961/2962 + 2963/2965) are missing.

local ENTRY_HODIR = 32845
local BOSS_HODIR = 7 -- ulduar.h:43 (moved above first use, tick 259: Lua local scope starts after its declaration — uses above it bind the nil global)

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_FLASH_FREEZE = 2
local SAY_STALACTITE = 3
local SAY_DEATH = 4
local SAY_BERSERK = 5
local SAY_HARD_MODE_FAILED = 6
local EMOTE_FREEZE = 7
local EMOTE_BLOWS = 8

local SPELL_BITING_COLD = 62038
local SPELL_BITING_COLD_TRIGGERED = 62039 -- boss_hodir.cpp:56
local SPELL_BITING_COLD_DAMAGE = 62188 -- boss_hodir.cpp:57
local SPELL_FREEZE = 62469
local SPELL_FROZEN_BLOWS = 62478
local SPELL_BERSERK = 47008
local SPELL_ICICLE = 62234

local SPELL_KILL_CREDIT = 64899 -- boss_hodir.cpp:73
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117
local UNIT_FLAG_NON_ATTACKABLE = 0x00000002 -- UnitDefines.h:125
local REACT_PASSIVE = 0 -- UnitDefines.h:340
local REACT_AGGRESSIVE = 2 -- UnitDefines.h:342
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:125
local UNIT_FLAG_PACIFIED = 0x00020000 -- UnitDefines.h:141

local ENTRY_SNOWPACKED_ICICLE = 33174 -- boss_hodir.cpp:99
local ENTRY_TOASTY_FIRE = 33342 -- boss_hodir.cpp:102
local GO_SNOWDRIFT = 194173 -- boss_hodir.cpp:108
local GO_TOASTY_FIRE = 194300 -- boss_hodir.cpp:107
local SPELL_SINGED = 62821 -- boss_hodir.cpp:87
local SPELL_BLOCK_OF_ICE = 61969 -- boss_hodir.cpp:61
local SPELL_ICE_SHARD = 62457 -- boss_hodir.cpp:70
local SPELL_ICE_SHARD_HIT = 65370 -- boss_hodir.cpp:71

local timers = {}

-- Per-GUID Hodir achievement latches (C++ boss_hodirAI members,
-- boss_hodir.cpp:340-348): set true by JustEngagedWith (:374-377),
-- cleared by Reset/Initialize (:339-345) and by the DoAction receiver
-- (:511-522, hodirDoAction below); the Go criteria readers
-- (2961-2968/3182-3184/2963-2965) are future work — the latches are
-- dormant state until they land.
local hodirLatches = {}

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

-- C++ SelectTarget(Random, idx, range, true) playerOnly=true draws the
-- threat list (UnitAI.h DefaultTargetSelector); the pool is
-- creature:GetAITargets() filtered to alive players within range
-- (auriaya swarming precedent) — the old GetPlayersInWorld pool
-- admitted unengaged players and retired 2026-10-25.
local function randomPlayerInRange(creature, range)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 < range then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- 2-arg RAID_MODE (ScriptedCreature.h:266): the 25-man value for every
-- difficulty except 10-man normal. Go twin: Is25ManRaid() covers
-- 25N/25H (difficulty&1), IsHeroic() covers 10H/25H (difficulty>=2).
local function isRaidMode25(creature)
    return creature:Is25ManRaid() or creature:IsHeroic()
end

-- C++ EVENT_ICICLE (id 4; init 2s at line 379, arm at 448-451):
-- SelectTarget(Random, 0, 100.0f, true) playerOnly=TRUE ->
-- DoCast(target, SPELL_ICICLE 62234), non-triggered, null-guarded like
-- the C++ if; re-arm RAID_MODE(5500ms, 3500ms) unconditional — the
-- ScheduleEvent sits outside the target check.
local function onIcicle(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "icicle", 500, function() onIcicle(creature, guid) end)
        return
    end
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_ICICLE)
    end
    local rearm = 5500
    if isRaidMode25(creature) then
        rearm = 3500
    end
    schedule(guid, "icicle", rearm, function() onIcicle(creature, guid) end)
end

-- C++ EVENT_FREEZE (lines 444-446): DoCastAOE(SPELL_FREEZE),
-- 1-arg = non-triggered; init 25s (line 380), repeat 30-45s
-- (line 446).
local function onFreeze(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "freeze", 500, function() onFreeze(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FREEZE)
    schedule(guid, "freeze", {30000, 45000}, function() onFreeze(creature, guid) end)
end

-- C++ EVENT_BLOWS (lines 473-477): Talk(SAY_STALACTITE) +
-- Talk(EMOTE_BLOWS) + DoCast(me, SPELL_FROZEN_BLOWS),
-- 2-arg = non-triggered; init 60-65s (line 381), repeat
-- 60-65s (line 477).
local function onBlows(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "blows", 500, function() onBlows(creature, guid) end)
        return
    end
    creature:Talk(SAY_STALACTITE)
    creature:Talk(EMOTE_BLOWS)
    creature:CastSpell(creature, SPELL_FROZEN_BLOWS)
    schedule(guid, "blows", {60000, 65000}, function() onBlows(creature, guid) end)
end

-- C++ EVENT_FLASH_FREEZE (lines 453-455): Talk-only port.
-- The random-target SPELL_ICICLE_SNOWDRIFT loop stays out on bridge
-- grounds (caster is the random player; CastSpell is creature-only on
-- the Lua surface). The bare
-- DoCast(SPELL_FLASH_FREEZE), and the whole
-- EVENT_FLASH_FREEZE_EFFECT machinery (lines 462-470) have
-- no bridges. Talk-to-Talk cadence is 25.5-35.5s: the 500ms
-- EVENT_FLASH_FREEZE_EFFECT leg (line 460) plus the 25-35s
-- reschedule (line 470); init 45s (line 382).
local function onFlashFreeze(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flashfreeze", 500, function() onFlashFreeze(creature, guid) end)
        return
    end
    creature:Talk(SAY_FLASH_FREEZE)
    creature:Talk(EMOTE_FREEZE)
    schedule(guid, "flashfreeze", {25500, 35500}, function() onFlashFreeze(creature, guid) end)
end

-- C++ EVENT_RARE_CACHE one-shot (lines 479-483) and EVENT_BERSERK
-- one-shot (lines 485-488): named ticks so the 500ms IsCasting defer
-- below does not consume the cast — the auriaya berserk precedent.
-- Both sit under the boss UpdateAI pre-loop (:437) + in-loop (:492)
-- CASTING gates, like the other four boss ticks.
local function onRareCache(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "rarecache", 500, function() onRareCache(creature, guid) end)
        return
    end
    creature:Talk(SAY_HARD_MODE_FAILED)
    -- C++ EVENT_RARE_CACHE :481: iCouldSayThatThisCacheWasRare = false
    -- (before the SetData leg, which has no bridge).
    local latches = hodirLatches[guid]
    if latches then
        latches.rareCache = false
    end
end

local function onBerserk(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function() onBerserk(creature, guid) end)
        return
    end
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

-- C++ JustEngagedWith (lines 370-384): BossAI passthrough
-- (SetBossState(DATA_HODIR 7, IN_PROGRESS) PORTED tick 259,
-- base-first) + Talk(SAY_AGGRO) + DoCast(me,
-- SPELL_BITING_COLD, true) -> CastSpell(creature, 62038,
-- true); the six ScheduleEvent legs become CreateLuaEvent
-- timers below.
local function hodirEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetBossState(BOSS_HODIR, 1) -- BossAI::JustEngagedWith SetBossState(DATA_HODIR 7, IN_PROGRESS), base-first (boss_hodir.cpp:369)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_hodir.cpp:369)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_BITING_COLD, true)
    -- C++ JustEngagedWith :373-377: gettingColdInHereTimer = 1000,
    -- gettingColdInHere / cheeseTheFreeze / iHaveTheCoolestFriends /
    -- iCouldSayThatThisCacheWasRare = true (before the ScheduleEvents).
    hodirLatches[guid] = {gettingCold = true, cheeseTheFreeze = true, coolestFriends = true, rareCache = true}
    schedule(guid, "icicle", 2000, function() onIcicle(creature, guid) end)
    schedule(guid, "freeze", 25000, function() onFreeze(creature, guid) end)
    schedule(guid, "blows", {60000, 65000}, function() onBlows(creature, guid) end)
    schedule(guid, "flashfreeze", 45000, function() onFlashFreeze(creature, guid) end)
    -- C++ EVENT_RARE_CACHE (lines 479-483): Talk-only port,
    -- init 3min (line 383), fires once (CancelEvent, line
    -- 483); the flag clear + instance SetData legs have no
    -- bridges.
    schedule(guid, "rarecache", 180000, function() onRareCache(creature, guid) end)
    -- C++ EVENT_BERSERK (lines 485-488): Talk(SAY_BERSERK) +
    -- DoCast(me, SPELL_BERSERK, true) -> CastSpell(creature,
    -- 47008, true); init 8min (line 384), fires once
    -- (CancelEvent, line 488).
    schedule(guid, "berserk", 480000, function() onBerserk(creature, guid) end)
end

local function hodirLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
    hodirLatches[creature:GetGUIDLow()] = nil
end

local function hodirDied(event, creature, killer)
    creature:SetBossState(BOSS_HODIR, 3) -- true-kill fallback: BossAI::JustDied -> _JustDied() DONE; the lethal path writes DONE in hodirDamageTaken below (C++ :420)
    cancelTimers(creature:GetGUIDLow())
    hodirLatches[creature:GetGUIDLow()] = nil
end

-- C++ Reset (lines 359-366): _Reset() (cancelTimers twin) +
-- SetReactState(REACT_PASSIVE) + the frozen-helper summon loop
-- (boss_hodir.cpp:362-364) — PORTED: FRIENDS_COUNT =
-- RAID_MODE(4, 8) helpers, entries + literal SummonPositions
-- (boss_hodir.cpp Entry/SummonPositions tables), TEMPSUMMON_MANUAL_
-- DESPAWN (Eluna type 7, 0ms), each self-casting
-- SPELL_SUMMON_FLASH_FREEZE_HELPER 61989 triggered (the stale "no
-- summon bridge" claim is retracted). The helper AIs are PORTED tick
-- 476 (helper block below); dormant until the helpers enter combat
-- (see there).
local FROZEN_HELPER_ENTRIES = {32897, 33325, 33328, 32893, 33326, 32901, 32900, 33327}
local FROZEN_HELPER_POSITIONS = {
    {1983.75, -243.36, 432.767, 1.57},
    {1999.90, -230.49, 432.767, 1.57},
    {2010.06, -243.45, 432.767, 1.57},
    {2021.12, -236.65, 432.767, 1.57},
    {2028.10, -244.66, 432.767, 1.57},
    {2014.18, -232.80, 432.767, 1.57},
    {1992.90, -237.54, 432.767, 1.57},
    {1976.60, -233.53, 432.767, 1.57},
}
local function hodirReset(event, creature)
    hodirLatches[creature:GetGUIDLow()] = nil -- C++ Reset :359-360: Initialize() first (latch booleans false)
    creature:SetBossState(BOSS_HODIR, 0) -- _Reset() SetBossState(DATA_HODIR 7, NOT_STARTED), first per C++ :359-360 — the tick-258 auriaya convention (unconditional; the death-path ev23 pre-fire is overwritten by the ev4 DONE write in the same tick)
    cancelTimers(creature:GetGUIDLow())
    creature:SetReactState(REACT_PASSIVE)
    local helperCount = 4
    if isRaidMode25(creature) then
        helperCount = 8
    end
    for i = 1, helperCount do
        local pos = FROZEN_HELPER_POSITIONS[i]
        local helper = creature:SummonCreature(FROZEN_HELPER_ENTRIES[i],
            pos[1], pos[2], pos[3], pos[4], 7, 0)
        if helper then
            helper:CastSpell(helper, 61989, true)
        end
    end
end

-- C++ KilledUnit (lines 389-391): who->GetTypeId() ==
-- TYPEID_PLAYER, then Talk(SAY_SLAY) — the razuvious
-- player-gated variant precedent.
local function hodirTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ DamageTaken (lines 393-421): damage >= me->GetHealth()
-- -> damage = 0 + Talk(SAY_DEATH) + the full cheat-death machine,
-- ported in C++ order with the live bridges: RemoveAllAuras,
-- RemoveAllAttackers, AttackStop, SetReactState(REACT_PASSIVE),
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE),
-- SetRooted(true) for the UNIT_STATE_ROOT leg (the black-knight
-- root-half approximation), InterruptNonMeleeSpells (no-op twin),
-- StopMoving + CombatStop(true) (C++ :413, ahead of the credit
-- cast), DoCastAOE SPELL_KILL_CREDIT 64899 as
-- creature:CastSpell(creature, spell, true) BEFORE the faction
-- change (the C++ comment: the credit targets enemies only),
-- SetFaction(FACTION_FRIENDLY), DespawnOrUnsummon(10s) as a
-- fire-and-forget 10s one-shot (the freyaDamageTaken precedent;
-- C++ re-arms the despawn per hit, unreachable here — the machine
-- leaves nothing able to land another hit), and _JustDied() as
-- cancelTimers + SetBossState(DATA_HODIR 7, DONE) (PORTED tick 259,
-- last per C++ :420). The numeric second return performs the C++-exact
-- damage=0 rewrite (the moroes damage-rewrite precedent). No
-- bridges: MoveIdle, the SetControlled stun leg, MotionMaster
-- Clear, instance SetData(DATA_HODIR_RARE_CACHE) — documented-only.
local function hodirDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        creature:Talk(SAY_DEATH)
        creature:RemoveAllAuras()
        creature:RemoveAllAttackers()
        creature:AttackStop()
        creature:SetReactState(REACT_PASSIVE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetRooted(true)
        creature:InterruptNonMeleeSpells()
        creature:StopMoving() -- boss_hodir.cpp:409 (MoveIdle sibling still unbridged)
        creature:CombatStop(true) -- boss_hodir.cpp:413, ahead of the credit cast
        creature:CastSpell(creature, SPELL_KILL_CREDIT, true)
        creature:SetFaction(FACTION_FRIENDLY)
        CreateLuaEvent(function()
            creature:Despawn()
        end, 10000)
        cancelTimers(creature:GetGUIDLow())
        creature:SetBossState(BOSS_HODIR, 3) -- _JustDied() SetBossState(DATA_HODIR 7, DONE), last per C++ :420 (PORTED tick 259)
        return false, 0
    elseif not creature:IsInCombat() then
        -- C++ DamageTaken else-leg (lines 421-426): the out-of-combat
        -- first hit flips REACT_PASSIVE -> REACT_AGGRESSIVE (the ctor/
        -- Reset passive is what the boss carries until first damage);
        -- ev9 fires pre-aggro (combat.go), so the !IsInCombat gate is
        -- reachable here. AttackStart(who) has no bridge — the engine
        -- converges on the same tick (triggerCreatureAggro starts combat
        -- with the attacker). DoZoneInCombat (:425) PORTED 2026-10-23
        -- below in C++ arm order.
        creature:SetReactState(REACT_AGGRESSIVE)
        -- C++ :425 — me-form DoZoneInCombat after SetReactState
        -- (C++ arm order; AttackStart(who) still has no bridge).
        creature:DoZoneInCombat()
    end
end

local function snowpackedIcicleReset(_, creature)
    creature:SetDisplayId(creature:GetTemplateModelId(2))
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_PACIFIED)
    creature:SetRooted(true)
    creature:SetReactState(REACT_PASSIVE)
    local guid = creature:GetGUIDLow()
    schedule(guid, "snowpacked", 12000, function()
        local drift = creature:FindNearestGameObject(GO_SNOWDRIFT, 2.0)
        if drift then
            drift:RemoveFromWorld()
        end
        creature:Despawn()
    end)
end

local function snowpackedIcicleDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function toastyFireReset(_, creature)
    creature:SetDisplayId(creature:GetTemplateModelId(2))
    creature:CastSpell(creature, SPELL_SINGED, true)
end

local function toastyFireSpellHit(_, creature, _caster, spellId)
    if spellId == SPELL_BLOCK_OF_ICE or spellId == SPELL_ICE_SHARD or spellId == SPELL_ICE_SHARD_HIT then
        local fire = creature:FindNearestGameObject(GO_TOASTY_FIRE, 1.0)
        if fire then
            fire:RemoveFromWorld()
        end
        creature:Despawn()
    end
end

RegisterCreatureEvent(ENTRY_SNOWPACKED_ICICLE, 23, snowpackedIcicleReset)
RegisterCreatureEvent(ENTRY_SNOWPACKED_ICICLE, 4, snowpackedIcicleDied)

-- C++ npc_icicleAI (boss_hodir.cpp:558-600): ctor SetDisplayId
-- (Modelid1) + flags NON_ATTACKABLE|PACIFIED|NOT_SELECTABLE +
-- rooted + REACT_PASSIVE; Reset re-inits the 2500ms timer; UpdateAI
-- has no UpdateVictim gate, so an ev23 timer harness is faithful:
-- entry 33173 (SNOWDRIFT) -> DoCast(me, FALL_SNOWDRIFT 62460) +
-- DoCast(me, ICICLE_FALL 69428), both non-triggered; entry 33169
-- -> DoCast(me, ICICLE_FALL 69428) + DoCast(me, FALL_DAMAGE
-- 62236, true); 10000ms loop. Same summoned-add class as the
-- snowpacked/toasty ports above.
local ENTRY_ICICLE = 33169 -- boss_hodir.cpp:100
local ENTRY_ICICLE_SNOWDRIFT = 33173 -- boss_hodir.cpp:101
local SPELL_FALL_SNOWDRIFT = 62460 -- boss_hodir.cpp:68
local SPELL_ICICLE_FALL = 69428 -- boss_hodir.cpp:66
local SPELL_FALL_DAMAGE = 62236 -- boss_hodir.cpp:67

local function icicleTick(creature, guid)
    if creature:GetEntry() == ENTRY_ICICLE_SNOWDRIFT then
        creature:CastSpell(creature, SPELL_FALL_SNOWDRIFT)
        creature:CastSpell(creature, SPELL_ICICLE_FALL)
    elseif creature:GetEntry() == ENTRY_ICICLE then
        creature:CastSpell(creature, SPELL_ICICLE_FALL)
        creature:CastSpell(creature, SPELL_FALL_DAMAGE, true)
    end
    schedule(guid, "icicle", 10000, function() icicleTick(creature, guid) end)
end

local function icicleReset(_, creature)
    creature:SetDisplayId(creature:GetTemplateModelId(1))
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_PACIFIED)
    creature:SetRooted(true)
    creature:SetReactState(REACT_PASSIVE)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "icicle", 2500, function() icicleTick(creature, guid) end)
end

local function icicleDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_ICICLE, 23, icicleReset)
RegisterCreatureEvent(ENTRY_ICICLE, 4, icicleDied)
RegisterCreatureEvent(ENTRY_ICICLE_SNOWDRIFT, 23, icicleReset)
RegisterCreatureEvent(ENTRY_ICICLE_SNOWDRIFT, 4, icicleDied)
RegisterCreatureEvent(ENTRY_TOASTY_FIRE, 23, toastyFireReset)
RegisterCreatureEvent(ENTRY_TOASTY_FIRE, 14, toastyFireSpellHit)

-- C++ npc_flash_freezeAI::IsSummonedBy (boss_hodir.cpp:232-249):
-- summoner nil-guarded like the C++ ToUnit() check; the stored
-- targetGUID serves the unported UpdateAI only. AddThreat(summoner,
-- 250) is the engage leg — SetInCombatWith has no bridge (the Go
-- AddThreat bridge syncs TargetGUID + ATTACK_START on a switch).
-- DoCast(target, SPELL_BLOCK_OF_ICE 61969, true) with target ==
-- summoner -> CastSpell(summoner, 61969, true); NearTeleportTo the
-- summoner's position; if the target is a player, Hodir (instance
-- GetCreature(BOSS_HODIR)) -> DoAction(ACTION_CHEESE_THE_FREEZE 2)
-- (ev38 dispatch; Hodir's DoAction arm is PORTED tick 476 as the
-- hodirLatches receiver — the latch readers are future Go criteria
-- cases).
local ENTRY_FLASH_FREEZE = 32926 -- boss_hodir.cpp:98
local ENTRY_ICE_BLOCK = 32938 -- boss_hodir.cpp:97
local SPELL_FLASH_FREEZE_HELPER = 61990 -- boss_hodir.cpp:64
local ACTION_CHEESE_THE_FREEZE = 2 -- boss_hodir.cpp:140
local UNIT_FLAG_STUNNED = 0x00040000 -- UnitDefines.h:142

local function flashFreezeSummoned(_, creature, summoner)
    if not summoner then
        return
    end
    creature:AddThreat(summoner, 250)
    creature:CastSpell(summoner, SPELL_BLOCK_OF_ICE, true)
    creature:NearTeleport(summoner:GetX(), summoner:GetY(), summoner:GetZ(), summoner:GetO())
    if summoner:GetObjectType() == "Player" then
        local hodir = creature:GetInstanceCreature(BOSS_HODIR)
        if hodir then
            hodir:DoAction(ACTION_CHEESE_THE_FREEZE)
        end
    end
end

-- C++ npc_ice_blockAI::IsSummonedBy (boss_hodir.cpp:278-296):
-- summoner nil-guarded like the C++ ToUnit() check. The stun/root
-- legs hit the summoner itself: SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_STUNNED | UNIT_FLAG_PACIFIED) + SetControlled(true,
-- UNIT_STATE_ROOT) -> summoner:SetFlag + summoner:SetRooted(true)
-- (SetRooted IS the root leg, lua_creature_events.go:4021; both the
-- creature and player surfaces carry SetFlag/SetRooted). AddThreat
-- (summoner, 250) is the engage leg — SetInCombatWith has no bridge,
-- as above. AddThreat(me, 250, summoner) is the C++ who-redirect
-- (ScriptedCreature.cpp:202-209: who gains victim on ITS threat
-- list) -> summoner:AddThreat(creature, 250). The cast/teleport
-- legs sit under the C++ GetCreature(targetGUID) check, i.e. they
-- run only when the summoner is a creature — the player-summoned
-- case (SPELL_SUMMON_BLOCK_OF_ICE cast by the FlashFreeze target on
-- itself, :544) skips them; DoCast(target, SPELL_FLASH_FREEZE_HELPER
-- 61990, true) -> CastSpell(summoner, 61990, true) + NearTeleportTo
-- the summoner's position.
local function iceBlockSummoned(_, creature, summoner)
    if not summoner then
        return
    end
    summoner:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED + UNIT_FLAG_PACIFIED)
    summoner:SetRooted(true)
    creature:AddThreat(summoner, 250)
    summoner:AddThreat(creature, 250)
    if summoner:GetObjectType() == "Creature" then
        creature:CastSpell(summoner, SPELL_FLASH_FREEZE_HELPER, true)
        creature:NearTeleport(summoner:GetX(), summoner:GetY(), summoner:GetZ(), summoner:GetO())
    end
end

RegisterCreatureEvent(ENTRY_FLASH_FREEZE, 22, flashFreezeSummoned)
RegisterCreatureEvent(ENTRY_ICE_BLOCK, 22, iceBlockSummoned)

-- C++ npc_hodir_priest / npc_hodir_shaman / npc_hodir_druid /
-- npc_hodir_mage (boss_hodir.cpp:657-955) — PORTED tick 476 (this
-- tick): Reset-seeded event timers + HealthBelowPct poll + JustDied
-- DoAction senders. The C++ UpdateAI gate (!UpdateVictim() ||
-- HasUnitState(STUNNED) || HasFlag(UNIT_FLAG_STUNNED), priest :678,
-- shaman :760, druid :825, mage :903) returns before events.Update,
-- so the timers freeze while the helper has no victim or is frozen;
-- the Go twin gates every tick on IsInCombat() (the ev9 precedent in
-- this file) + HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED)
-- (lua_creature_events.go:5761; the ice-block summon sets the flag
-- via summoner:SetFlag). The HasUnitState half has no bridge. While
-- gated the tick re-arms after 1s (the maulgar phasecheck poll)
-- instead of consuming the event. The CASTING gates (priest :683,
-- shaman :765, druid :830, mage :908) ride the file's 500ms IsCasting defer. STAY-OUTS:
-- priest EVENT_DISPEL_MAGIC (:700-709, friendly-grid search +
-- HasAura(SPELL_FREEZE) + DoCast DISPEL_MAGIC — GetAITargets is
-- threat-list only, no friendly-grid bridge); all four
-- DoSpellAttackIfReady combat legs (SMITE 61923 / LAVA_BURST 61924 /
-- WRATH 62793 / FIREBALL 61909 — engine combat, no bridge).
-- Dormant until the helpers enter combat (the spell chain 61989 ->
-- flash freeze -> block of ice -> ice block, plus the unported
-- ice-block DamageTaken freeing leg); the ticks poll harmlessly
-- until then. Entry->role bindings verified this tick from the AC
-- creature_template.sql rows (names + npc_ulduar_hodir_* ScriptName
-- role suffixes; TC's C++ classes are npc_hodir_* — same roles, fork
-- naming differs; all eight rows carry difficulty_entry_1/2/3 = 0,
-- so no difficulty-entry swap); the C++ Entry[8] array
-- (boss_hodir.cpp:166-175) + ulduar.h:143-156 agree.
local SPELL_GREATER_HEAL = 62809 -- boss_hodir.cpp:91
local SPELL_DISPEL_MAGIC = 63499 -- boss_hodir.cpp:92 (stay-out, see above)
local SPELL_STORM_CLOUD = 65123 -- boss_hodir.cpp:81
local SPELL_STARLIGHT = 62807 -- boss_hodir.cpp:76
local SPELL_CONJURE_FIRE = 62823 -- boss_hodir.cpp:85
local SPELL_MELT_ICE = 64528 -- boss_hodir.cpp:86

local ENTRY_PRIEST_A = 32897 -- boss_hodir.cpp:168 (ulduar.h:155 NPC_FIELD_MEDIC_PENNY)
local ENTRY_PRIEST_B = 33326 -- boss_hodir.cpp:172 (ulduar.h:156 NPC_FIELD_MEDIC_JESSI)
local ENTRY_SHAMAN_A = 33328 -- boss_hodir.cpp:170 (ulduar.h:147 NPC_ELEMENTALIST_MAHFUUN)
local ENTRY_SHAMAN_B = 32900 -- boss_hodir.cpp:174 (ulduar.h:148 NPC_ELEMENTALIST_AVUUN)
local ENTRY_DRUID_A = 33325 -- boss_hodir.cpp:169 (ulduar.h:143 NPC_EIVI_NIGHTFEATHER)
local ENTRY_DRUID_B = 32901 -- boss_hodir.cpp:173 (ulduar.h:144 NPC_ELLIE_NIGHTFEATHER)
local ENTRY_MAGE_A = 32893 -- boss_hodir.cpp:171 (ulduar.h:151 NPC_MISSY_FLAMECUFFS)
local ENTRY_MAGE_B = 33327 -- boss_hodir.cpp:175 (ulduar.h:152 NPC_SISSY_FLAMECUFFS)

local ACTION_I_HAVE_THE_COOLEST_FRIENDS = 1 -- boss_hodir.cpp:139

-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention, boss_high_king_maulgar.lua).
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

local function helperGated(creature, guid, key, fn)
    if not creature:IsInCombat() or creature:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED) then
        schedule(guid, key, 1000, fn)
        return true
    end
    return false
end

-- C++ priest EVENT_HEAL (:696-698): DoCastAOE(SPELL_GREATER_HEAL),
-- 1-arg = non-triggered -> self-cast (the selin_fireheart /
-- mr_smite DoCastAOE lowering); repeat {7500ms, 10s} (:698).
local function priestHealTick(creature, guid)
    if helperGated(creature, guid, "priestHeal", function() priestHealTick(creature, guid) end) then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "priestHeal", 500, function() priestHealTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_GREATER_HEAL)
    schedule(guid, "priestHeal", {7500, 10000}, function() priestHealTick(creature, guid) end)
end

-- C++ priest UpdateAI :686-688: HealthBelowPct(30) -> DoCast(me,
-- SPELL_GREATER_HEAL), non-triggered, checked every UpdateAI tick
-- before the event loop (after the first CASTING gate). Go twin: a
-- 1s poll (the maulgar phasecheck idiom) with the same gates.
local function priestHealthPoll(creature, guid)
    if helperGated(creature, guid, "priestHealth", function() priestHealthPoll(creature, guid) end) then
        return
    end
    if not creature:IsCasting() and healthBelowPct(creature, 30) then
        creature:CastSpell(creature, SPELL_GREATER_HEAL)
    end
    schedule(guid, "priestHealth", 1000, function() priestHealthPoll(creature, guid) end)
end

-- C++ priest Reset (:669-673): events.Reset() + EVENT_HEAL {4s,8s} +
-- EVENT_DISPEL_MAGIC {15s,20s} (dispel leg stays out, see above).
local function priestReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "priestHeal", {4000, 8000}, function() priestHealTick(creature, guid) end)
    schedule(guid, "priestHealth", 1000, function() priestHealthPoll(creature, guid) end)
end

-- C++ JustDied (:723/:788/:852/:935, identical all four): Hodir
-- (instance GetCreature(BOSS_HODIR)) -> DoAction(1) — the
-- flash-freeze idiom in this file (GetInstanceCreature +
-- creature:DoAction, ev38 dispatch lua_creature_events.go:827).
local function helperDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    local hodir = creature:GetInstanceCreature(BOSS_HODIR)
    if hodir then
        hodir:DoAction(ACTION_I_HAVE_THE_COOLEST_FRIENDS)
    end
end

-- C++ shaman EVENT_STORM_CLOUD (:772-774): SelectTarget(Random, 0,
-- 100.0f, true) playerOnly -> DoCast(target, 65123, true); repeat
-- {15s,20s}, re-arm unconditional (outside the target check).
local function shamanStormTick(creature, guid)
    if helperGated(creature, guid, "shamanStorm", function() shamanStormTick(creature, guid) end) then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "shamanStorm", 500, function() shamanStormTick(creature, guid) end)
        return
    end
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_STORM_CLOUD, true)
    end
    schedule(guid, "shamanStorm", {15000, 20000}, function() shamanStormTick(creature, guid) end)
end

-- C++ shaman Reset (:752-756): EVENT_STORM_CLOUD {10s,12500ms}.
local function shamanReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "shamanStorm", {10000, 12500}, function() shamanStormTick(creature, guid) end)
end

-- C++ druid EVENT_STARLIGHT (:837-838): DoCast(me, 62807, true);
-- repeat {25s,35s}.
local function druidStarlightTick(creature, guid)
    if helperGated(creature, guid, "druidStarlight", function() druidStarlightTick(creature, guid) end) then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "druidStarlight", 500, function() druidStarlightTick(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_STARLIGHT, true)
    schedule(guid, "druidStarlight", {25000, 35000}, function() druidStarlightTick(creature, guid) end)
end

-- C++ druid Reset (:817-821): EVENT_STARLIGHT {15s,17500ms}.
local function druidReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "druidStarlight", {15000, 17500}, function() druidStarlightTick(creature, guid) end)
end

-- C++ mage SummonList (boss_hodir.cpp:876, :889-897): JustSummoned
-- tracks toasty fires (entry 33342), SummonedCreatureDespawn drops
-- them, Reset summons.DespawnAll(), the CONJURE_FIRE arm reads
-- summons.size(). Go twins: ev19 (CreatureEventOnJustSummonedCreature,
-- runtime.go:381 — the gothik/kelthuzad precedent) + ev20
-- (CreatureEventOnSummonedCreatureDespawn, runtime.go:382 — the
-- kelthuzad/ichoron precedent), both summoner-side with
-- (event, creature, summon). No ev21 leg: C++ has no
-- SummonedCreatureDies override, so a dead toasty stays counted until
-- it despawns. The table holds the toasty objects (the mimiron
-- summon-ref precedent) so Reset can DespawnAll them.
local mageToasties = {} -- mageGUID -> { [toastyGUID] = toastyCreature }

local function mageJustSummoned(event, creature, summon)
    if not summon or summon:GetEntry() ~= ENTRY_TOASTY_FIRE then
        return
    end
    local guid = creature:GetGUIDLow()
    local list = mageToasties[guid]
    if not list then
        list = {}
        mageToasties[guid] = list
    end
    list[summon:GetGUIDLow()] = summon
end

local function mageSummonDespawned(event, creature, summon)
    if not summon or summon:GetEntry() ~= ENTRY_TOASTY_FIRE then
        return
    end
    local list = mageToasties[creature:GetGUIDLow()]
    if list then
        list[summon:GetGUIDLow()] = nil
    end
end

local function mageToastyCount(guid)
    local n = 0
    local list = mageToasties[guid]
    if list then
        for _ in pairs(list) do
            n = n + 1
        end
    end
    return n
end

-- C++ mage EVENT_CONJURE_FIRE (:915-919): if (summons.size() >=
-- RAID_MODE(2,4)) break — NO re-arm, the event dies at cap; else
-- DoCast(me, 62823, true) + re-arm {15s,20s}.
local function mageConjureFireTick(creature, guid)
    if helperGated(creature, guid, "mageConjureFire", function() mageConjureFireTick(creature, guid) end) then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "mageConjureFire", 500, function() mageConjureFireTick(creature, guid) end)
        return
    end
    local cap = 2
    if isRaidMode25(creature) then
        cap = 4
    end
    if mageToastyCount(guid) >= cap then
        return
    end
    creature:CastSpell(creature, SPELL_CONJURE_FIRE, true)
    schedule(guid, "mageConjureFire", {15000, 20000}, function() mageConjureFireTick(creature, guid) end)
end

-- C++ mage EVENT_MELT_ICE (:921-923): FindNearestCreature(NPC_FLASH_FREEZE,
-- 50.0f, true) -> DoCast(FlashFreeze, 64528, true); re-arm {10s,15s}
-- unconditional (outside the null check).
local function mageMeltIceTick(creature, guid)
    if helperGated(creature, guid, "mageMeltIce", function() mageMeltIceTick(creature, guid) end) then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "mageMeltIce", 500, function() mageMeltIceTick(creature, guid) end)
        return
    end
    local flashFreeze = creature:FindNearestCreature(ENTRY_FLASH_FREEZE, 50.0)
    if flashFreeze then
        creature:CastSpell(flashFreeze, SPELL_MELT_ICE, true)
    end
    schedule(guid, "mageMeltIce", {10000, 15000}, function() mageMeltIceTick(creature, guid) end)
end

-- C++ mage Reset (:881-886): events.Reset() + summons.DespawnAll() +
-- EVENT_CONJURE_FIRE {10s,12500ms} + EVENT_MELT_ICE 5s. Go fires ev23
-- on death too (fireCreatureDiedByCreature 23+4 pair), and C++ does NOT
-- despawn on death — the DespawnAll leg is IsDead-guarded (IsDead reads
-- Health == 0, zeroed before the 23+4 fire).
local function mageReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if not creature:IsDead() then
        local list = mageToasties[guid]
        if list then
            local snapshot = {}
            for _, toasty in pairs(list) do
                snapshot[#snapshot + 1] = toasty
            end
            mageToasties[guid] = nil
            for _, toasty in ipairs(snapshot) do
                toasty:Despawn()
            end
        end
    end
    schedule(guid, "mageConjureFire", {10000, 12500}, function() mageConjureFireTick(creature, guid) end)
    schedule(guid, "mageMeltIce", 5000, function() mageMeltIceTick(creature, guid) end)
end

-- C++ boss_hodirAI::DoAction (:511-522): ACTION_I_HAVE_THE_COOLEST_FRIENDS
-- (1) clears iHaveTheCoolestFriends, ACTION_CHEESE_THE_FREEZE (2)
-- clears cheeseTheFreeze; other actions ignored (default break).
-- PORTED tick 476 (event 38; the mimiron DoAction-receiver precedent).
-- The latches are achievement-only (read by the future Go criteria
-- cases for 2963/2965 + 2961/2962); the receiver is live now that the
-- four add JustDied senders land above.
local function hodirDoAction(event, creature, action)
    local latches = hodirLatches[creature:GetGUIDLow()]
    if not latches then
        return
    end
    if action == ACTION_I_HAVE_THE_COOLEST_FRIENDS then
        latches.coolestFriends = false
    elseif action == ACTION_CHEESE_THE_FREEZE then
        latches.cheeseTheFreeze = false
    end
end

RegisterCreatureEvent(ENTRY_PRIEST_A, 23, priestReset)
RegisterCreatureEvent(ENTRY_PRIEST_A, 4, helperDied)
RegisterCreatureEvent(ENTRY_PRIEST_B, 23, priestReset)
RegisterCreatureEvent(ENTRY_PRIEST_B, 4, helperDied)
RegisterCreatureEvent(ENTRY_SHAMAN_A, 23, shamanReset)
RegisterCreatureEvent(ENTRY_SHAMAN_A, 4, helperDied)
RegisterCreatureEvent(ENTRY_SHAMAN_B, 23, shamanReset)
RegisterCreatureEvent(ENTRY_SHAMAN_B, 4, helperDied)
RegisterCreatureEvent(ENTRY_DRUID_A, 23, druidReset)
RegisterCreatureEvent(ENTRY_DRUID_A, 4, helperDied)
RegisterCreatureEvent(ENTRY_DRUID_B, 23, druidReset)
RegisterCreatureEvent(ENTRY_DRUID_B, 4, helperDied)
RegisterCreatureEvent(ENTRY_MAGE_A, 23, mageReset)
RegisterCreatureEvent(ENTRY_MAGE_A, 4, helperDied)
RegisterCreatureEvent(ENTRY_MAGE_A, 19, mageJustSummoned)
RegisterCreatureEvent(ENTRY_MAGE_A, 20, mageSummonDespawned)
RegisterCreatureEvent(ENTRY_MAGE_B, 23, mageReset)
RegisterCreatureEvent(ENTRY_MAGE_B, 4, helperDied)
RegisterCreatureEvent(ENTRY_MAGE_B, 19, mageJustSummoned)
RegisterCreatureEvent(ENTRY_MAGE_B, 20, mageSummonDespawned)

-- C++ spell_biting_cold::HandleEffectPeriodic
-- (boss_hodir.cpp:996-1027, EFFECT_0 SPELL_AURA_PERIODIC_DUMMY,
-- aura 62038): per-target stillness counter — the C++ list holds
-- one entry per AuraScript instance and each instance's target is
-- its own aura target, so a per-target-GUID table is the exact
-- model. First tick inserts (guid, 1) with no cast; count >= 4
-- casts 62039 triggered on the target and resets to 1 (the moving
-- check does NOT run on the cast tick); otherwise moving resets to
-- 1, still increments. No PreventDefaultAction — the
-- PERIODIC_DUMMY default is a no-op in both engines. PORTED tick
-- 367 via RegisterAuraEvent (the periodic hook fires per aura tick
-- on the player path; target:IsMoving is the tick-366 bridge).
local bitingColdCounters = {}

local function bitingColdTick(event, aura, target)
    local guid = target:GetGUIDLow()
    local count = bitingColdCounters[guid]
    if not count then
        bitingColdCounters[guid] = 1
        return
    end
    if count >= 4 then
        target:CastSpell(target, SPELL_BITING_COLD_TRIGGERED, true)
        bitingColdCounters[guid] = 1
    elseif target:IsMoving() then
        bitingColdCounters[guid] = 1
    else
        bitingColdCounters[guid] = count + 1
    end
end

-- C++ AuraScript destruction drops the per-instance list with the
-- aura; the remove hook is the exact twin.
local function bitingColdRemove(event, aura, target)
    bitingColdCounters[target:GetGUIDLow()] = nil
end

-- C++ spell_biting_cold_dot::HandleEffectPeriodic
-- (boss_hodir.cpp:1052-1067, EFFECT_0 SPELL_AURA_PERIODIC_DUMMY,
-- aura 62039): damage = 200 * 2^stacks, the caster casts 62188 on
-- self triggered with the damage as BP0 (CastSpellExtraArgs
-- AddSpellBP0, TRIGGERED_FULL_MASK); if the caster is moving,
-- RemoveAuraFromStack(62039) drops ONE stack — SetStackAmount(n-1)
-- with aura:Remove() at the last stack is the exact twin (Lua has
-- no RemoveAuraFromStack bridge; RemoveAura would drop all stacks).
-- PORTED tick 367 via RegisterAuraEvent.
local function bitingColdDotTick(event, aura, target)
    local caster = aura:GetCaster()
    if not caster then
        return
    end
    local stacks = aura:GetStackAmount()
    local damage = 200 * (2 ^ stacks)
    caster:CastCustomSpell(caster, SPELL_BITING_COLD_DAMAGE, true, damage)
    if caster:IsMoving() then
        if stacks <= 1 then
            aura:Remove()
        else
            aura:SetStackAmount(stacks - 1)
        end
    end
end

RegisterAuraEvent(SPELL_BITING_COLD, 1, bitingColdTick)
RegisterAuraEvent(SPELL_BITING_COLD, 3, bitingColdRemove)
RegisterAuraEvent(SPELL_BITING_COLD_TRIGGERED, 1, bitingColdDotTick)

RegisterCreatureEvent(ENTRY_HODIR, 1, hodirEnterCombat)
RegisterCreatureEvent(ENTRY_HODIR, 2, hodirLeaveCombat)
RegisterCreatureEvent(ENTRY_HODIR, 3, hodirTargetDied)
RegisterCreatureEvent(ENTRY_HODIR, 4, hodirDied)
RegisterCreatureEvent(ENTRY_HODIR, 9, hodirDamageTaken)
RegisterCreatureEvent(ENTRY_HODIR, 23, hodirReset)
RegisterCreatureEvent(ENTRY_HODIR, 38, hodirDoAction)
