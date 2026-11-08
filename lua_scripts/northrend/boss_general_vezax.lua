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
-- (:139) at engage (event 1; SetBossState(DATA_VEZAX 11,
-- IN_PROGRESS 1) PORTED 2026-11-05 via creature:SetBossState
-- (lua_creature_events.go:5917; maiden tick-257 precedent) — the
-- BossAI::JustEngagedWith base arms (SetCombatPulseDelay(5) +
-- DoZoneInCombat) were already ported; the CheckRequiredBosses /
-- EnterEvadeMode gate has no bridge (stay-out);
-- CheckShamanisticRage (PORTED this run —
-- gal_darah GetPlayersInWorld precedent + player:HasSpell /
-- player:CastSpell bridges, no longer "no map-player-scan
-- bridge"), and all six ScheduleEvent legs are ported below
-- (the vapors leg rides saroniteVaporsTick).
-- EVENT_SURGE_OF_DARKNESS (:188-193) — Talk
-- (EMOTE_SURGE_OF_DARKNESS 8) + Talk(SAY_SURGE_OF_DARKNESS 2) +
-- DoCast(me, SPELL_SURGE_OF_DARKNESS 62662) 2-arg non-triggered,
-- 60s init (:146), re-arm {50s,70s} (:192) — Talk order
-- (emote, say, cast) kept.
-- EVENT_SHADOW_CRASH (:164-173) — CheckPlayersInRange(4/9,
-- 15.0f, 50.0f) / SelectTarget(Random, 150.0f, true) +
-- DoCast(target, SPELL_SHADOW_CRASH 62660) 2-arg non-triggered,
-- init {8s,10s} (:142), re-arm {8s,12s} (:171) — bridged via the
-- freya/gal_darah GetPlayersInWorld candidate-selection
-- precedent; the RAID_MODE(4,9) count gate is the Is25ManRaid
-- gate in vezaxRandomTarget below (ring needs >= 4 / >= 9 players);
-- fallback: threat-list player within 150y; cast if target.
-- EVENT_SEARING_FLAMES (:174-176) — DoCastAOE
-- (SPELL_SEARING_FLAMES 62661) 2-arg non-triggered self-cast,
-- 12s init (:143), re-arm {14s,17500ms} (:176) — the hard-mode
-- cancel leg (:206) stays out with the unported vapor machine.
-- EVENT_MARK_OF_THE_FACELESS (:178-187) —
-- CheckPlayersInRange / SelectTarget(Random, 150.0f, true) +
-- DoCast(target, SPELL_MARK_OF_THE_FACELESS 63276) 2-arg
-- non-triggered, init {35s,40s} (:144), re-arm {35s,45s} (:185) —
-- same candidate-selection bridge as SHADOW_CRASH.
-- EVENT_BERSERK (:209-212) — Talk(SAY_BERSERK 4) + DoCast(me,
-- SPELL_BERSERK 26662) 2-arg non-triggered, 600s init (:147), no
-- re-arm — Talk-before-cast order kept.
-- KilledUnit (:228-232) — player-gated Talk(SAY_SLAY 1) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- JustDied (:234-239) — _JustDied() SetBossState(DATA_VEZAX 11, DONE 3)
-- PORTED 2026-11-05 via creature:SetBossState (lua_creature_events.go:5917;
-- maiden tick-257 precedent) + Talk(SAY_DEATH
-- 3) + DoRemoveAurasDueToSpellOnPlayers(SPELL_AURA_OF_DESPAIR
-- 62692, true, true) player leg — PORTED via stripDespairAuras
-- (GetPlayersInWorld + mapId/instanceId filter + player:RemoveAura —
-- the moroes precedent; event 4, C++ arm order; the
-- includePets/includeControlled legs have no pet/controlled iteration
-- bridge — stay out); also wipes the surge/berserk timers.
-- DOCUMENTED-ONLY (in this header; no registration beyond entry
-- 33271):
-- Reset / Initialize — _Reset() SetBossState(DATA_VEZAX 11,
-- NOT_STARTED 0) PORTED 2026-11-05 as a NEW ev23 handler split
-- from ev2 (ev2 leave-combat does NOT call Reset in C++ — the
-- sjonnir precedent; maiden tick-257 pattern) via
-- creature:SetBossState (lua_creature_events.go:5917); the
-- smellSaronite Initialize() flag is PORTED tick 417 as
-- creature:SetData(DATA_SMELL_SARONITE 31813188, 1) in vezaxReset
-- (ev23) + vezaxSpawned (ev5, ctor twin) — the
-- achievement_smell_saronite write leg, cleared by vezaxDoAction.
-- vaporCount is PORTED tick 473 as a GUID-keyed module table
-- (reset ev23/ev5, drives the hard-mode machine below);
-- animusDead stays out (written only, never read in C++).
-- SpellHitTarget — SPELL_SHADOW_CRASH_HIT 62659 TYPEID_PLAYER leg
-- -> shadowDodger = false (consumer: achievement_shadowdodger
-- below) PORTED tick 461 via RegisterCreatureEvent (15): ev15 fires
-- once per hit target with the HITTING spell's id (boss_ai.go:4039 /
-- :4069, fireCreatureSpellHitTarget; arugal precedent), so keying on
-- 62659 — not the 62660 cast — is exact. The tick-417 block is
-- retired: it assumed the only keyable id was 62660; the 62659-keyed
-- leg fires only on real 62659 hits. DORMANT until Spell.dbc ships
-- the 62660 -> 62659 trigger link: Go's trigger arm routes the
-- sub-spell through castCreatureSpellInternal (boss_ai.go:6705), whose
-- hit loop fires ev15 with 62659; nothing casts 62659 today, so the
-- latch stays 1 until then — the same dormancy as the LK spell-script
-- legs (ticks 456-460).
-- achievement_shadowdodger (OnCheck: GetData
-- (DATA_SHADOWDODGER)); achievement_smell_saronite (OnCheck:
-- GetData(DATA_SMELL_SARONITE)) — smell_saronite write leg PORTED
-- tick 417 (vezaxReset / vezaxSpawned / vezaxDoAction); shadowdodger
-- write leg PORTED tick 461 (vezaxReset / vezaxSpawned set, ev15
-- clear) + Go case in achievement_criteria_scripts.go.
-- CheckShamanisticRage — PORTED at engage (map-player-scan via
-- GetPlayersInWorld + map/instance filter -> player:HasSpell
-- (30823) -> player:CastSpell(player, 68415, false) non-triggered
-- (player:CastSpell bridged 2026-10-24, lua_player.go:309 — caster
-- now C++-exact).
-- EVENT_SARONITE_VAPORS (:194-208) — DoCast
-- (SPELL_SUMMON_SARONITE_VAPORS 63081) self-cast (2-arg
-- DoCast(uint32) resolves SELF — UnitAI.cpp:113), 30s init
-- (:145), re-arm {30s,35s}, + the hard-mode machine (vaporCount
-- == 6 && smellSaronite -> Talk(SAY_HARDMODE 5) +
-- Talk(EMOTE_BARRIER 7) + summons.DespawnAll() + DoCast(me,
-- SPELL_SARONITE_BARRIER 63364) + DoCast
-- (SPELL_SUMMON_SARONITE_ANIMUS 63145) + LOOT_MODE_HARD_MODE_1 +
-- CancelEvent(EVENT_SARONITE_VAPORS/EVENT_SEARING_FLAMES)) —
-- PORTED tick 473 via saroniteVaporsTick: the cast + count +
-- Talk/barrier/animus-cast/AddLootMode(0x2)/CancelEvent legs are
-- all bridged (AddLootMode lua_creature_events.go:2272;
-- GetData read of the smellSaronite slot, :809); only
-- summons.DespawnAll() stays out (no bridge for spell-summoned
-- summons — the razorscale evade precedent; the vapor adds
-- themselves never spawn until the 63081 summon effect lands,
-- so nothing exists to despawn in the Go model). DORMANT until
-- Spell.dbc ships 63081/63364/63145: the 30s cast fires on
-- schedule but the summon effect resolves only with the DBC row.
-- boss_saronite_animus (no creature-entry enum in C++ —
-- spell-summoned via 63145; entry-unverifiable — no
-- registration): Reset one-shot DoCast(me,
-- SPELL_VISUAL_SARONITE_ANIMUS 63319); EVENT_PROFOUND_OF_
-- DARKNESS — DoCastAOE(SPELL_PROFOUND_OF_DARKNESS 63420,
-- TRIGGERED), 3s init, 3s repeat — timer-driven but the add
-- exists only via the unbridged summon machine (task
-- exclusion: summon/add legs out); JustDied -> vezax DoAction
-- (ACTION_ANIMUS_DIE) (boss_general_vezax.cpp:338) — ev38 dispatch
-- IS bridged (lua_creature_events.go:802); the animus bearer is
-- entry-unverifiable and unregistered; no Talk arms.
-- npc_saronite_vapors (no creature-entry enum in C++ —
-- spell-summoned via 63081; entry-unverifiable — no
-- registration): Talk(EMOTE_VAPORS 0) in the constructor — no
-- spawn / constructor bridge; Reset EVENT_RANDOM_MOVE 5s-7.5s
-- -> MoveRandom(30.0f) — no scheduler / movement bridges;
-- DamageTaken (lethal -> damage = 0 + flags + root +
-- stand-state-dead + DoCast SPELL_SARONITE_VAPORS 63323 +
-- DespawnOrUnsummon(30s) + vezax DoAction(ACTION_VAPORS_DIE)) —
-- event 9 DamageTaken IS bridged and ev38 dispatch IS bridged
-- (lua_creature_events.go:802); the sender is the vapors
-- DamageTaken (boss_general_vezax.cpp:436) on an
-- entry-unverifiable, unregistered bearer.
-- spell_general_vezax_mark_of_the_faceless (63276 — periodic
-- dummy AuraScript: caster casts
-- SPELL_MARK_OF_THE_FACELESS_DAMAGE 63278 with bp1 =
-- aurEff amount — PORTED tick 422 below via RegisterAuraEvent
-- (63276, 1); the "no AuraScript bridge" claim is retired —
-- RegisterAuraEvent + aura:GetCaster/GetAmount are live);
-- spell_general_vezax_mark_of_the_faceless_leech (63276/63134
-- target filter removing expl target, FinishCast on empty —
-- no SpellScript bridge; joins the no-SpellScript-bridge
-- queue); spell_general_vezax_saronite_vapors (63323 —
-- OnApply: bp * 2^stack mana restore -> SPELL_SARONITE_VAPORS_
-- ENERGIZE 63337 + DAMAGE 63338 at 2x — PORTED tick 422 below
-- via RegisterAuraEvent (63323, 2 + 4); the "no AuraScript
-- bridge" claim is retired).
-- (Achievements: see the SpellHitTarget / Reset notes above —
-- smell_saronite write leg PORTED tick 417; shadowdodger BLOCKED
-- on the 62659 spell-id key.)

local ENTRY_GENERAL_VEZAX = 33271

-- DATA_VEZAX = 11 (ulduar.h:47; BOSS_VEZAX).

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_SURGE_OF_DARKNESS = 2
local SAY_DEATH = 3
local SAY_BERSERK = 4
local SAY_HARDMODE = 5

local EMOTE_BARRIER = 7
local EMOTE_SURGE_OF_DARKNESS = 8

local SPELL_AURA_OF_DESPAIR = 62692
local SPELL_SURGE_OF_DARKNESS = 62662
local SPELL_SHADOW_CRASH = 62660
local SPELL_SHADOW_CRASH_HIT = 62659 -- boss_general_vezax.cpp:56
local SPELL_SEARING_FLAMES = 62661
local SPELL_MARK_OF_THE_FACELESS = 63276
local SPELL_BERSERK = 26662
local SPELL_SHAMANTIC_RAGE = 30823
local SPELL_CORRUPTED_RAGE = 68415
local SPELL_SUMMON_SARONITE_VAPORS = 63081 -- boss_general_vezax.cpp:61
local SPELL_SARONITE_BARRIER = 63364 -- boss_general_vezax.cpp:53
local SPELL_SUMMON_SARONITE_ANIMUS = 63145 -- boss_general_vezax.cpp:64

local LOOT_MODE_HARD_MODE_1 = 0x2 -- SharedDefines.h:43

local DATA_SHADOWDODGER = 29962997 -- boss_general_vezax.cpp:99 (Misc enum)
local DATA_SMELL_SARONITE = 31813188 -- boss_general_vezax.cpp:98 (Misc enum)
local ACTION_VAPORS_DIE = 1 -- boss_general_vezax.cpp:75 (VezaxActions; VAPORS_SPAWN 0, ANIMUS_DIE 2)
local ACTION_ANIMUS_DIE = 2 -- boss_general_vezax.cpp:76

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

-- C++ vaporCount (boss_general_vezax.cpp:116, Initialize/Reset) —
-- GUID-keyed, zeroed on ev23/ev5, cleared on death.
local vaporCounts = {}

local function cancelTimerKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ EVENT_SURGE_OF_DARKNESS (:188-193): Talk
-- (EMOTE_SURGE_OF_DARKNESS 8) + Talk(SAY_SURGE_OF_DARKNESS 2) +
-- DoCast(me, SPELL_SURGE_OF_DARKNESS 62662) (2-arg non-triggered);
-- 60s init (:146), re-arm {50s,70s} (:192) — Talk order kept.
local function surgeOfDarknessTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "surge", 500, function()
            surgeOfDarknessTick(creature, guid)
        end)
        return
    end
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
-- CAST-STATE 2026-11-04: boss UpdateAI pre-loop (:157) + in-loop
-- (:215) CASTING gates PORTED as a 500ms IsCasting defer at the top
-- of all 6 ported timer ticks (surge / shadow_crash / searing_flames /
-- mark / berserk / saronite_vapors; mal_ganis idiom, IsCasting live
-- lua_creature_events.go:1950) — defer precedes Talk + target pick,
-- so a deferred tick re-fires intact and consumes no re-arm. The
-- engage / slay / died hooks are ungated per C++ (separate hooks);
-- saronite_animus gates (:348/:363) guard only the unported
-- profound-of-darkness machine; vapors stay out with the summon
-- machine, so no defer rides them.
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK)
end

-- C++ SelectTarget player arms (boss_general_vezax.cpp:283-306
-- CheckPlayersInRange + :168/:182 fallback SelectTarget).
-- CheckPlayersInRange iterates the MAP player list, distance-only:
-- no alive test, and it returns null unless at least RAID_MODE(4,9)
-- players are in the 15-50y ring (Is25ManRaid gate). The fallback
-- SelectTarget(Random, 0, 150.0f, true) draws the THREAT list, so the
-- wide pool is IsEngagedBy-filtered + alive (threat refs skip dead /
-- offline at build, UnitAI.h) — not the whole instance pool.
-- Ring distances are player->GetDistance(me pos) (Object.cpp:1181:
-- exact minus the PLAYER's reach, inclusive 15-50); the fallback is a
-- SelectTarget 150 gate (IsWithinCombatRange, Unit.cpp:583) — both
-- twinned below, reach-exact (player reach 1.5).
-- Player reach is DEFAULT_PLAYER_COMBAT_REACH 1.5 (ObjectDefines.h:40).
local PLAYER_COMBAT_REACH = 1.5
local function vezaxRandomTarget(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local playersMin = 4
    if creature:Is25ManRaid() then
        playersMin = 9
    end
    local ring, wide = {}, {}
    local reach = creature:GetCombatReach()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId then
            local dist = creature:GetDistance(plr)
            if dist - PLAYER_COMBAT_REACH >= 15 and dist - PLAYER_COMBAT_REACH <= 50 then
                ring[#ring + 1] = plr
            end
            if dist - reach - PLAYER_COMBAT_REACH < 150 and not plr:IsDead()
                    and creature:IsEngagedBy(plr) then
                wide[#wide + 1] = plr
            end
        end
    end
    if #ring >= playersMin then
        return ring[math.random(#ring)]
    end
    if #wide > 0 then
        return wide[math.random(#wide)]
    end
    return nil
end

-- C++ EVENT_SHADOW_CRASH (:164-173): CheckPlayersInRange /
-- SelectTarget(Random, 150.0f) + DoCast(target, SPELL_SHADOW_CRASH
-- 62660) (2-arg non-triggered); init {8s,10s} (:142), unconditional
-- re-arm {8s,12s} (:171); cast only if a target was found.
local function shadowCrashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadow_crash", 500, function()
            shadowCrashTick(creature, guid)
        end)
        return
    end
    local target = vezaxRandomTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_CRASH)
    end
    schedule(guid, "shadow_crash", {8000, 12000}, function()
        shadowCrashTick(creature, guid)
    end)
end

-- C++ EVENT_SEARING_FLAMES (:174-176): DoCastAOE(SPELL_SEARING_FLAMES
-- 62661) (2-arg non-triggered) — the boss_ai.go DoCastAOE self-cast
-- lowering; 12s init (:143), unconditional re-arm {14s,17500ms}
-- (:176). The hard-mode cancel leg (:206) stays out with the
-- unported vapor hard-mode machine (unreachable in this model).
local function searingFlamesTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "searing_flames", 500, function()
            searingFlamesTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SEARING_FLAMES)
    schedule(guid, "searing_flames", {14000, 17500}, function()
        searingFlamesTick(creature, guid)
    end)
end

-- C++ EVENT_MARK_OF_THE_FACELESS (:178-187): CheckPlayersInRange /
-- SelectTarget(Random, 150.0f) + DoCast(target,
-- SPELL_MARK_OF_THE_FACELESS 63276) (2-arg non-triggered); init
-- {35s,40s} (:144), unconditional re-arm {35s,45s} (:185); cast
-- only if a target was found.
local function markOfTheFacelessTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "mark", 500, function()
            markOfTheFacelessTick(creature, guid)
        end)
        return
    end
    local target = vezaxRandomTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_MARK_OF_THE_FACELESS)
    end
    schedule(guid, "mark", {35000, 45000}, function()
        markOfTheFacelessTick(creature, guid)
    end)
end

-- C++ EVENT_SARONITE_VAPORS (:194-208): DoCast
-- (SPELL_SUMMON_SARONITE_VAPORS 63081) 2-arg self-cast, 30s init
-- (:145), re-arm {30s,35s} (:196) — then ++vaporCount == 6 &&
-- smellSaronite -> Talk(SAY_HARDMODE 5) + Talk(EMOTE_BARRIER 7) +
-- DoCast(me, SPELL_SARONITE_BARRIER 63364) + DoCast
-- (SPELL_SUMMON_SARONITE_ANIMUS 63145) + AddLootMode
-- (LOOT_MODE_HARD_MODE_1 0x2) + CancelEvent(EVENT_SARONITE_VAPORS
-- / EVENT_SEARING_FLAMES). C++ re-arms before the count check
-- (:196) and cancels after (:205-206); net effect at hard mode is
-- cancel, so the Lua leg re-arms only on the non-hard-mode path.
-- summons.DespawnAll() stays out (no bridge for spell-summoned
-- summons). IsCasting 500ms defer per the UpdateAI gates (:157/:215).
local function saroniteVaporsTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "saronite_vapors", 500, function()
            saroniteVaporsTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_SARONITE_VAPORS)
    local count = (vaporCounts[guid] or 0) + 1
    vaporCounts[guid] = count
    if count == 6 and creature:GetData(DATA_SMELL_SARONITE) ~= 0 then
        creature:Talk(SAY_HARDMODE)
        creature:Talk(EMOTE_BARRIER)
        creature:CastSpell(creature, SPELL_SARONITE_BARRIER)
        creature:CastSpell(creature, SPELL_SUMMON_SARONITE_ANIMUS)
        creature:AddLootMode(LOOT_MODE_HARD_MODE_1)
        cancelTimerKey(guid, "saronite_vapors")
        cancelTimerKey(guid, "searing_flames")
        return
    end
    schedule(guid, "saronite_vapors", {30000, 35000}, function()
        saroniteVaporsTick(creature, guid)
    end)
end

-- C++ CheckShamanisticRage (boss_general_vezax.cpp:244-251): map
-- player scan -> player:HasSpell(SPELL_SHAMANTIC_RAGE 30823) ->
-- player->CastSpell(player, SPELL_CORRUPTED_RAGE 68415) 2-arg
-- non-triggered. The "no map-player-scan bridge" claim is stale —
-- GetPlayersInWorld + map/instance filter is the in-tree bridge
-- (gal_darah precedent); player:HasSpell is live
-- (engine/world/lua_player.go:145). player:CastSpell is live (lua_player.go:309) — the player casts
-- on itself, C++-exact. No alive gate — C++-exact
-- (C++ has none).
local function checkShamanisticRage(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and plr:HasSpell(SPELL_SHAMANTIC_RAGE) then
            -- C++: player->CastSpell(player, 68415, false) — now live
            -- via player:CastSpell (lua_player.go:309).
            plr:CastSpell(plr, SPELL_CORRUPTED_RAGE, false)
        end
    end
end

-- C++ JustEngagedWith (:134-148): BossAI::JustEngagedWith base
-- (SetBossState(DATA_VEZAX, IN_PROGRESS) PORTED 2026-11-05;
-- CheckRequiredBosses / EnterEvadeMode gate has no bridge —
-- stay-out) + Talk
-- (SAY_AGGRO) + DoCast(me, SPELL_AURA_OF_DESPAIR 62692) (:139,
-- 2-arg non-triggered self-cast) + CheckShamanisticRage +
-- ScheduleEvent legs — the four unbridgeable ScheduleEvent legs are
-- the vapor/summon legs below (event 1).
local function vezaxEnterCombat(event, creature, target)
    -- C++ JustEngagedWith (:134-148): BossAI::JustEngagedWith first
    -- (:136) — SetBossState(DATA_VEZAX, IN_PROGRESS) before Talk;
    -- the CheckRequiredBosses / EnterEvadeMode gate has no bridge
    -- (stay-out); state leg PORTED 2026-11-05 (maiden tick-257).
    creature:SetBossState(DATA_VEZAX, 1)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_general_vezax.cpp:136)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO)
    creature:CastSpell(creature, SPELL_AURA_OF_DESPAIR)
    checkShamanisticRage(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "surge", 60000, function()
        surgeOfDarknessTick(creature, guid)
    end)
    schedule(guid, "shadow_crash", {8000, 10000}, function()
        shadowCrashTick(creature, guid)
    end)
    schedule(guid, "searing_flames", 12000, function()
        searingFlamesTick(creature, guid)
    end)
    schedule(guid, "mark", {35000, 40000}, function()
        markOfTheFacelessTick(creature, guid)
    end)
    schedule(guid, "saronite_vapors", 30000, function()
        saroniteVaporsTick(creature, guid)
    end)
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature, guid)
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

-- C++ instance->DoRemoveAurasDueToSpellOnPlayers(
-- SPELL_AURA_OF_DESPAIR 62692, true, true) player leg
-- (boss_general_vezax.cpp:234-239, JustDied): strip the aura from every
-- player sharing the creature's map+instance — GetPlayersInWorld +
-- mapId/instanceId filter + player:RemoveAura (the moroes precedent;
-- C++-exact, no alive gate). The includePets/includeControlled legs have
-- no pet/controlled iteration bridge — stay out.
local function stripDespairAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            p:RemoveAura(SPELL_AURA_OF_DESPAIR)
        end
    end
end

local function vezaxDied(event, creature, killer)
    -- C++ JustDied (:234-239): _JustDied() first — SetBossState
    -- (DATA_VEZAX, DONE) before Talk(SAY_DEATH); PORTED 2026-11-05
    -- (maiden tick-257 precedent).
    creature:SetBossState(DATA_VEZAX, 3)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
    vaporCounts[creature:GetGUID()] = nil
    -- C++ JustDied (boss_general_vezax.cpp:234-239 — arm order: _JustDied,
    -- Talk(SAY_DEATH), DoRemoveAurasDueToSpellOnPlayers): the player leg
    -- is PORTED via stripDespairAuras above (C++ arm order; the
    -- includePets/includeControlled legs stay out — no pet/controlled
    -- iteration bridge).
    stripDespairAuras(creature)
end

-- Shared timer wipe for event 2 (leave-combat does NOT call Reset
-- in C++ — the sjonnir precedent; ev23 below is the Reset handler).
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset (boss_general_vezax.cpp:127-133): _Reset() —
-- SetBossState(DATA_VEZAX, NOT_STARTED)
-- (ScriptedCreature.cpp:499-511; the bjarngrim precedent sets it
-- unconditionally on ev23; maiden tick-257 pattern). The
-- Initialize() flag inits stay out (consumed only by the unported
-- GetData / hard-mode machines).
local function vezaxReset(event, creature)
    cancelTimers(creature:GetGUID())
    vaporCounts[creature:GetGUID()] = 0
    creature:SetBossState(DATA_VEZAX, 0)
    -- C++ Reset -> Initialize (:110-115, :127-133): smellSaronite =
    -- true, shadowDodger = true. The achievement_smell_saronite /
    -- achievement_shadowdodger Go gates read these slots off the
    -- killed victim's motion (achievement_criteria_scripts.go); the
    -- smellSaronite clear leg is vezaxDoAction below, the
    -- shadowDodger clear leg is the ev15 SpellHitTarget handler.
    -- Alive-gated: ev23 fires on death too (fireCreatureDied runs
    -- Reset before the kill-credit criteria read,
    -- kill.go:1091/:1400), and a death-time re-arm would clobber the
    -- fight's latch before the gate reads it; the spawn handler
    -- sets the C++ default.
    if creature:IsAlive() then
        creature:SetData(DATA_SMELL_SARONITE, 1)
        creature:SetData(DATA_SHADOWDODGER, 1)
    end
end

-- C++ ctor twin (boss_general_vezax.cpp:110-115): smellSaronite =
-- true, shadowDodger = true. The auriaya spawn-latch precedent: a
-- fresh motion's ScriptData reads 0, so the spawn handler sets the
-- C++ default before any engage can occur.
local function vezaxSpawned(event, creature)
    vaporCounts[creature:GetGUID()] = 0
    creature:SetData(DATA_SMELL_SARONITE, 1)
    creature:SetData(DATA_SHADOWDODGER, 1)
end

-- C++ DoAction(ACTION_VAPORS_DIE) (boss_general_vezax.cpp:268-270):
-- smellSaronite = false. Sent by the vapors DamageTaken lethal leg
-- (:436) via vezax->AI()->DoAction; the ev38 dispatch is bridged
-- (lua_creature_events.go:802; elderDoAction precedent in
-- boss_freya.lua). Dormant until the vapors bearer lands: the sender
-- is npc_saronite_vapors, spell-summoned via 63081 with no creature
-- entry anywhere in the C++ tree (ENTRY-BLOCKED), and the vapor
-- summon effect is DBC-gated — so in the Go-modeled fight no
-- vapor can die and the latch stays 1, matching C++ for a
-- zero-vapor-death fight. shadowDodger deliberately has NO write leg
-- here: its clear leg is the ev15 SpellHitTarget handler below
-- (SPELL_SHADOW_CRASH_HIT 62659, player-gated).
-- C++ DoAction(ACTION_ANIMUS_DIE) (boss_general_vezax.cpp:271-276):
-- RemoveAurasDueToSpell(SPELL_SARONITE_BARRIER 63364) +
-- ScheduleEvent(EVENT_SEARING_FLAMES, 7s, 12s) + animusDead = true.
-- PORTED tick 473: RemoveAura(63364) (lua_creature_events.go:1504)
-- + schedule(guid, "searing_flames", {7000, 12000}); animusDead
-- stays out (written only, never read in C++). Dormant until the
-- animus bearer lands: the sender is the animus JustDied (:338),
-- entry-unverifiable (spell-summoned via 63145, no creature entry
-- in the C++ tree), so no ANIMUS_DIE ever fires in the Go model.
local function vezaxDoAction(event, creature, action)
    if action == ACTION_VAPORS_DIE then
        creature:SetData(DATA_SMELL_SARONITE, 0)
    elseif action == ACTION_ANIMUS_DIE then
        -- C++ DoAction(ACTION_ANIMUS_DIE) (:271-276):
        -- RemoveAurasDueToSpell(SPELL_SARONITE_BARRIER 63364) +
        -- ScheduleEvent(EVENT_SEARING_FLAMES, 7s, 12s); animusDead
        -- stays out (written only, never read in C++). schedule()
        -- cancels any pending searing_flames key first, so this
        -- re-arms cleanly after the hard-mode cancel.
        creature:RemoveAura(SPELL_SARONITE_BARRIER)
        local guid = creature:GetGUID()
        schedule(guid, "searing_flames", {7000, 12000}, function()
            searingFlamesTick(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 1, vezaxEnterCombat)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 3, vezaxTargetDied)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 4, vezaxDied)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 5, vezaxSpawned)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 23, vezaxReset)
RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 38, vezaxDoAction)

-- C++ SpellHitTarget (boss_general_vezax.cpp:222-226): a player hit
-- by SPELL_SHADOW_CRASH_HIT 62659 clears shadowDodger. PORTED tick
-- 461 via RegisterCreatureEvent (15); the player gate is the
-- GetObjectType() == "Player" idiom (maiden/moroes precedent).
local function vezaxSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_SHADOW_CRASH_HIT and target ~= nil and target:GetObjectType() == "Player" then
        creature:SetData(DATA_SHADOWDODGER, 0)
    end
end

RegisterCreatureEvent(ENTRY_GENERAL_VEZAX, 15, vezaxSpellHitTarget)

-- C++ spell_general_vezax_mark_of_the_faceless (boss_general_vezax.cpp:451-485):
-- OnEffectPeriodic EFFECT_0 PERIODIC_DUMMY — the caster casts
-- SPELL_MARK_OF_THE_FACELESS_DAMAGE 63278 on the aura target,
-- triggered, bp1 = effect amount. PORTED tick 422 via
-- RegisterAuraEvent (63276, 1); aura:GetCaster/GetAmount are live
-- (lua_aura.go). Dormant until the DBC aura triggers run (no
-- Spell.dbc in tree — the intense_cold precedent): 63276 is cast
-- by the EVENT_MARK_OF_THE_FACELESS leg above, but without DBC
-- effect rows Go applies no periodic aura, so no tick fires yet.
local SPELL_MARK_OF_THE_FACELESS_DAMAGE = 63278 -- boss_general_vezax.cpp:52

local function markOfTheFacelessPeriodic(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    caster:CastCustomSpell(target, SPELL_MARK_OF_THE_FACELESS_DAMAGE, true, nil, aura:GetAmount())
end

RegisterAuraEvent(SPELL_MARK_OF_THE_FACELESS, 1, markOfTheFacelessPeriodic)

-- C++ spell_general_vezax_saronite_vapors (boss_general_vezax.cpp:516-553):
-- AfterEffectApply EFFECT_0 DUMMY, REAL_OR_REAPPLY — the caster
-- casts SPELL_SARONITE_VAPORS_ENERGIZE 63337 (bp0 = amount *
-- 2^stack) + SPELL_SARONITE_VAPORS_DAMAGE 63338 (bp0 = mana * 2)
-- on the aura target, triggered. PORTED tick 422 via
-- RegisterAuraEvent (63323, 2) + (63323, 4): event 4
-- (OnChangeAmount) is the Go twin of the C++ REAPPLY arm —
-- REAPPLY fires only from AuraEffect::ChangeAmount with
-- onStackOrReapply (SpellAuraEffects.cpp:662-672), i.e. on stack
-- change, never on a pure duration refresh (RefreshDuration calls
-- no HandleEffect, SpellAuras.cpp:905-927). Fully dormant: 63323
-- is cast only by the unported vapors DamageTaken leg (:436) on an
-- entry-unverifiable bearer, so no aura can apply yet.
local SPELL_SARONITE_VAPORS = 63323 -- boss_general_vezax.cpp:58
local SPELL_SARONITE_VAPORS_ENERGIZE = 63337 -- boss_general_vezax.cpp:59
local SPELL_SARONITE_VAPORS_DAMAGE = 63338 -- boss_general_vezax.cpp:60

local function saroniteVaporsApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    local mana = math.floor(aura:GetAmount() * (2 ^ aura:GetStackAmount()))
    caster:CastCustomSpell(target, SPELL_SARONITE_VAPORS_ENERGIZE, true, mana)
    caster:CastCustomSpell(target, SPELL_SARONITE_VAPORS_DAMAGE, true, mana * 2)
end

RegisterAuraEvent(SPELL_SARONITE_VAPORS, 2, saroniteVaporsApply)
RegisterAuraEvent(SPELL_SARONITE_VAPORS, 4, saroniteVaporsApply)
