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
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
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
-- Deviations from C++: no UNIT_STATE_CASTING model in Go
-- (creature casts are packet-visual), so timers fire
-- unconditionally — melee stays engine-driven, like C++
-- DoMeleeAttackIfReady (the maiden_of_virtue precedent).
-- DOCUMENTED-ONLY (in this header; no registration beyond
-- entry 32845):
-- JustEngagedWith achievement flags (gettingColdInHereTimer
-- / gettingColdInHere / cheeseTheFreeze /
-- iHaveTheCoolestFriends / iCouldSayThatThisCacheWasRare,
-- lines 373-377) — DoAction/SetData-gated, no bridges.
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
-- DoAction arms (lines 503-515:
-- ACTION_I_HAVE_THE_COOLEST_FRIENDS /
-- ACTION_CHEESE_THE_FREEZE) — DoAction-gated achievement
-- flags, no bridge.
-- Reset (lines 359-366): _Reset() + SetReactState REACT_PASSIVE
-- + the frozen-helper summon loop (PORTED in hodirReset, below;
-- the stale "no bridge" claim is retracted); the ctor's
-- SetReactState(REACT_PASSIVE) (line 336) rides the same ev23
-- hook (Eluna On_Reset fires at spawn).
-- npc_hodir_priest / npc_hodir_shaman / npc_hodir_druid /
-- npc_hodir_mage — adds legs, BLOCKED: EVENT_HEAL
-- DoCastAOE(SPELL_GREATER_HEAL 62809) (lines 696-698,
-- priest add); EVENT_DISPEL_MAGIC grid search +
-- DoCast(SPELL_DISPEL_MAGIC 63499, true) (lines 700-709);
-- HealthBelowPct(30) DoCast(62809) (line 687);
-- EVENT_STORM_CLOUD random-target DoCast(SPELL_STORM_CLOUD
-- 65123, true) (lines 772-774); EVENT_STARLIGHT
-- DoCast(me, SPELL_STARLIGHT 62807, true) (lines 837-838,
-- plain timer self-cast but on the druid add);
-- EVENT_CONJURE_FIRE summon-list-size gate +
-- DoCast(SPELL_CONJURE_FIRE 62823, true) (lines 915-919);
-- EVENT_MELT_ICE FindNearestCreature(NPC_FLASH_FREEZE)
-- + DoCast(SPELL_MELT_ICE 64528, true) (lines 921-923);
-- all four JustDied -> hodir DoAction
-- (ACTION_I_HAVE_THE_COOLEST_FRIENDS) — no DoAction
-- bridge; their DoSpellAttackIfReady combat legs
-- (SPELL_SMITE 61923 / SPELL_LAVA_BURST 61924 /
-- SPELL_WRATH 62793 / SPELL_FIREBALL 61909) have no cast
-- bridges — no registrations.
-- npc_flash_freezeAI — IsSummonedBy: DoCast(target,
-- SPELL_BLOCK_OF_ICE 61969, true) (line 243), NearTeleportTo
-- teleport legs, DoAction(ACTION_CHEESE_THE_FREEZE) (lines
-- 246-247); UpdateAI despawn/die checks (lines 209-232) —
-- summon legs have no bridge (stale "no despawn / no teleport"
-- wording: creature:Despawn() and creature:Teleport ARE bridged,
-- but the bearer is an unregistered summon add and the arm rides
-- the summon machine, paired with gameobject-removal legs and a
-- DoAction leg); no registration.
-- npc_ice_blockAI — IsSummonedBy: DoCast(target,
-- SPELL_FLASH_FREEZE_HELPER 61990, true) (line 292);
-- DamageTaken -> hodir REACT_AGGRESSIVE / DoZoneInCombat /
-- AttackStart (lines 301-318) — summon / zone-in legs, no
-- bridges; no registration.
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
-- spell_biting_cold / spell_biting_cold_dot —
-- SpellScript / AuraScript — no SpellScript / AuraScript
-- bridges; both join the no-SpellScript-bridge queue.
-- Achievement data legs (DATA_GETTING_COLD_IN_HERE /
-- ACTION_CHEESE_THE_FREEZE /
-- ACTION_I_HAVE_THE_COOLEST_FRIENDS /
-- DATA_HODIR_RARE_CACHE — the C++ @todo names Storm
-- Cloud and Toasty Fires) — no GetData / DoAction /
-- achievement bridges; all join the unmodeled-achievement
-- queue.

local ENTRY_HODIR = 32845

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
                and creature:IsWithinDist(unit, range) then
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
    creature:CastSpell(creature, SPELL_FREEZE)
    schedule(guid, "freeze", {30000, 45000}, function() onFreeze(creature, guid) end)
end

-- C++ EVENT_BLOWS (lines 473-477): Talk(SAY_STALACTITE) +
-- Talk(EMOTE_BLOWS) + DoCast(me, SPELL_FROZEN_BLOWS),
-- 2-arg = non-triggered; init 60-65s (line 381), repeat
-- 60-65s (line 477).
local function onBlows(creature, guid)
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
    creature:Talk(SAY_FLASH_FREEZE)
    creature:Talk(EMOTE_FREEZE)
    schedule(guid, "flashfreeze", {25500, 35500}, function() onFlashFreeze(creature, guid) end)
end

-- C++ JustEngagedWith (lines 370-384): BossAI passthrough
-- (no bridge) + Talk(SAY_AGGRO) + DoCast(me,
-- SPELL_BITING_COLD, true) -> CastSpell(creature, 62038,
-- true); the six ScheduleEvent legs become CreateLuaEvent
-- timers below.
local function hodirEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_BITING_COLD, true)
    schedule(guid, "icicle", 2000, function() onIcicle(creature, guid) end)
    schedule(guid, "freeze", 25000, function() onFreeze(creature, guid) end)
    schedule(guid, "blows", {60000, 65000}, function() onBlows(creature, guid) end)
    schedule(guid, "flashfreeze", 45000, function() onFlashFreeze(creature, guid) end)
    -- C++ EVENT_RARE_CACHE (lines 479-483): Talk-only port,
    -- init 3min (line 383), fires once (CancelEvent, line
    -- 483); the flag clear + instance SetData legs have no
    -- bridges.
    schedule(guid, "rarecache", 180000, function() creature:Talk(SAY_HARD_MODE_FAILED) end)
    -- C++ EVENT_BERSERK (lines 485-488): Talk(SAY_BERSERK) +
    -- DoCast(me, SPELL_BERSERK, true) -> CastSpell(creature,
    -- 47008, true); init 8min (line 384), fires once
    -- (CancelEvent, line 488).
    schedule(guid, "berserk", 480000, function()
        creature:Talk(SAY_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK, true)
    end)
end

local function hodirLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function hodirDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset (lines 359-366): _Reset() (cancelTimers twin) +
-- SetReactState(REACT_PASSIVE) + the frozen-helper summon loop
-- (boss_hodir.cpp:362-364) — PORTED: FRIENDS_COUNT =
-- RAID_MODE(4, 8) helpers, entries + literal SummonPositions
-- (boss_hodir.cpp Entry/SummonPositions tables), TEMPSUMMON_MANUAL_
-- DESPAWN (Eluna type 7, 0ms), each self-casting
-- SPELL_SUMMON_FLASH_FREEZE_HELPER 61989 triggered (the stale "no
-- summon bridge" claim is retracted). Delta: summoned add AIs are
-- unported — the helpers are inert.
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
    cancelTimers(creature:GetGUID())
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
-- cancelTimers. The numeric second return performs the C++-exact
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
        cancelTimers(creature:GetGUID())
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
    local guid = creature:GetGUID()
    schedule(guid, "snowpacked", 12000, function()
        local drift = creature:FindNearestGameObject(GO_SNOWDRIFT, 2.0)
        if drift then
            drift:RemoveFromWorld()
        end
        creature:Despawn()
    end)
end

local function snowpackedIcicleDied(_, creature)
    cancelTimers(creature:GetGUID())
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
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "icicle", 2500, function() icicleTick(creature, guid) end)
end

local function icicleDied(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_ICICLE, 23, icicleReset)
RegisterCreatureEvent(ENTRY_ICICLE, 4, icicleDied)
RegisterCreatureEvent(ENTRY_ICICLE_SNOWDRIFT, 23, icicleReset)
RegisterCreatureEvent(ENTRY_ICICLE_SNOWDRIFT, 4, icicleDied)
RegisterCreatureEvent(ENTRY_TOASTY_FIRE, 23, toastyFireReset)
RegisterCreatureEvent(ENTRY_TOASTY_FIRE, 14, toastyFireSpellHit)

RegisterCreatureEvent(ENTRY_HODIR, 1, hodirEnterCombat)
RegisterCreatureEvent(ENTRY_HODIR, 2, hodirLeaveCombat)
RegisterCreatureEvent(ENTRY_HODIR, 3, hodirTargetDied)
RegisterCreatureEvent(ENTRY_HODIR, 4, hodirDied)
RegisterCreatureEvent(ENTRY_HODIR, 9, hodirDamageTaken)
RegisterCreatureEvent(ENTRY_HODIR, 23, hodirReset)
