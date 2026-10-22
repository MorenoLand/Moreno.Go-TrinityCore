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
-- DamageTaken lethal companion legs — RemoveAllAttackers /
-- InterruptNonMeleeSpells / StopMoving / MoveIdle / SetControlled
-- stun / CombatStop / instance->SetData(DATA_HODIR_RARE_CACHE)
-- (line 400) — no bridges; documented-only. The rest of the
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
-- pre-aggro so the gate is reachable); DoZoneInCombat /
-- AttackStart(who) have no bridges — the engine converges on the
-- same tick via triggerCreatureAggro.
-- gettingColdInHere UpdateAI leg (lines 491-499:
-- PvE-combat-ref iteration, SPELL_BITING_COLD_TRIGGERED
-- 62039 aura-stack check, SetData(DATA_GETTING_COLD_IN_HERE))
-- — achievement / instance-data leg, no bridges.
-- DoAction arms (lines 503-515:
-- ACTION_I_HAVE_THE_COOLEST_FRIENDS /
-- ACTION_CHEESE_THE_FREEZE) — DoAction-gated achievement
-- flags, no bridge.
-- Reset (lines 359-366): _Reset() + SetReactState REACT_PASSIVE
-- are PORTED in hodirReset (the passive leg is separable from the
-- frozen-helper SummonCreature loop, lines 368-373 — summon legs,
-- no bridge); the ctor's SetReactState(REACT_PASSIVE) (line 336)
-- rides the same ev23 hook (Eluna On_Reset fires at spawn).
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
-- summon / teleport legs have no bridges (stale "no despawn
-- bridge" wording: creature:Despawn() IS bridged, but the
-- bearer is an unregistered summon add and the arm rides the
-- summon machine, paired with gameobject-removal legs); no
-- registration.
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
-- npc_snowpacked_icicleAI — 12s despawnTimer ->
-- RemoveGameObject(GO_SNOWDRIFT 194173) +
-- DespawnOrUnsummon (lines 632-642) — despawn leg, no
-- bridge; no registration.
-- npc_toasty_fireAI — Reset: DoCast(me, SPELL_SINGED
-- 62821, true) (line 967); SpellHit (SPELL_BLOCK_OF_ICE /
-- SPELL_ICE_SHARD / SPELL_ICE_SHARD_HIT) -> RemoveGameObject
-- + DespawnOrUnsummon (lines 970-978) — SpellHit /
-- despawn legs, no bridges; no registration.
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

-- C++ SelectTarget(Random, idx, range, true) playerOnly=true arms ride the
-- jaraxxus GetPlayersInWorld precedent (0.0f = any-range modeled with
-- 100 — curator precedent). Caveat shared with the keleseth/thorim/
-- beast ports: C++ draws from the threat list (engaged units); the
-- bridge can include nearby unengaged players.
local function randomPlayerInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:IsWithinDist(plr, range) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_ICICLE (id 4; init 2s at line 379, arm at 448-451):
-- SelectTarget(Random, 0, 100.0f, true) playerOnly=TRUE ->
-- DoCast(target, SPELL_ICICLE 62234), non-triggered, null-guarded like
-- the C++ if; re-arm RAID_MODE(5500ms, 3500ms) unconditional — the
-- ScheduleEvent sits outside the target check, so 5500ms (10-man value;
-- the 10-vs-25-man raid-size selector has no bridge).
local function onIcicle(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_ICICLE)
    end
    schedule(guid, "icicle", 5500, function() onIcicle(creature, guid) end)
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
-- SetReactState(REACT_PASSIVE); the frozen-helper SummonCreature
-- loop (lines 368-373) stays out — no summon bridge.
local function hodirReset(event, creature)
    cancelTimers(creature:GetGUID())
    creature:SetReactState(REACT_PASSIVE)
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
-- AttackStop, SetReactState(REACT_PASSIVE),
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE),
-- SetRooted(true) for the UNIT_STATE_ROOT leg (the black-knight
-- root-half approximation), DoCastAOE SPELL_KILL_CREDIT 64899 as
-- creature:CastSpell(creature, spell, true) BEFORE the faction
-- change (the C++ comment: the credit targets enemies only),
-- SetFaction(FACTION_FRIENDLY), DespawnOrUnsummon(10s) as a
-- fire-and-forget 10s one-shot (the freyaDamageTaken precedent;
-- C++ re-arms the despawn per hit, unreachable here — the machine
-- leaves nothing able to land another hit), and _JustDied() as
-- cancelTimers. The numeric second return performs the C++-exact
-- damage=0 rewrite (the moroes damage-rewrite precedent). No
-- bridges: RemoveAllAttackers, InterruptNonMeleeSpells,
-- StopMoving/MoveIdle, the SetControlled stun leg, CombatStop,
-- instance SetData(DATA_HODIR_RARE_CACHE) — documented-only.
local function hodirDamageTaken(event, creature, attacker, damage)
    if damage >= creature:GetHealth() then
        creature:Talk(SAY_DEATH)
        creature:RemoveAllAuras()
        creature:AttackStop()
        creature:SetReactState(REACT_PASSIVE)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:SetRooted(true)
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
        -- reachable here. DoZoneInCombat + AttackStart(who) have no
        -- bridges — the engine converges on the same tick
        -- (triggerCreatureAggro starts combat with the attacker).
        creature:SetReactState(REACT_AGGRESSIVE)
    end
end

RegisterCreatureEvent(ENTRY_HODIR, 1, hodirEnterCombat)
RegisterCreatureEvent(ENTRY_HODIR, 2, hodirLeaveCombat)
RegisterCreatureEvent(ENTRY_HODIR, 3, hodirTargetDied)
RegisterCreatureEvent(ENTRY_HODIR, 4, hodirDied)
RegisterCreatureEvent(ENTRY_HODIR, 9, hodirDamageTaken)
RegisterCreatureEvent(ENTRY_HODIR, 23, hodirReset)
