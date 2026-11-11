-- Thorim (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_thorim.cpp
-- (boss_thorim (CreatureScript) via GetUlduarAI<boss_thorimAI> (BossAI),
-- BOSS_THORIM = 8; npc_runic_colossus (32872); npc_ancient_rune_giant
-- (32873) — all registered from inside AddSC_boss_thorim(); loader decl
-- 121 / call 316 per northrend_script_loader.cpp — the TWELFTH group of
-- the "// Ulduar" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_freya() (decl 120 / call 315); the call after it is
-- AddSC_boss_yogg_saron()).
-- Porting doctrine: the maiden/baltharus CreateLuaEvent convention —
-- GUID-keyed timers table, cancel on events 2/4/23; CastSpell(nil,
-- spell) = DoCastVictim; CastSpell(creature, spell) = DoCastSelf (and
-- DoCastAOE); the 3rd arg triggered flag follows the C++ DoCast 3rd arg
-- (all arms below are 2-arg / non-triggered); {min,max} delay tables are
-- Repeat(min,max) ranges. The 2026-10-02 "no-timer-bridge" / "zero
-- bridgeable arms" claims are superseded by this doctrine; phase
-- gating remains the real blocker.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 23 OnReset.
--
-- PORTED ARMS (C++-exact for all modeled arms):
-- boss_thorimAI JustEngagedWith (line 616) — Talk(SAY_AGGRO_1 0);
-- DoCast(me, SPELL_SHEATH_OF_LIGHTNING 62276) (line 630 — self-cast,
-- non-triggered; the phase-2 removal at line 724 rides the unbridged
-- phase machine, so in Lua the aura persists — documented deviation).
-- boss_thorimAI KilledUnit — player-gated Talk(SAY_SLAY 4).
-- npc_runic_colossusAI JustEngagedWith (boss_thorim.cpp:1444-1449) — the
-- DoZoneInCombat first arm is PORTED 2026-10-23 (bridge live,
-- engine/world/lua_creature_events.go); the three ScheduleEvent arms
-- below follow in C++ order.
-- npc_runic_colossusAI EVENT_RUNIC_BARRIER (enum line 117; arm lines
-- 1463-1466, Repeat(35s,45s)) — Talk(EMOTE_RUNIC_BARRIER 0) +
-- DoCastAOE(SPELL_RUNIC_BARRIER 62338), non-triggered.
-- npc_runic_colossusAI EVENT_SMASH (enum line 118; arm lines 1468-1470,
-- Repeat(15s,18s)) — DoCastAOE(SPELL_SMASH 62339), non-triggered.
-- npc_runic_colossusAI EVENT_RUNIC_CHARGE (enum line 119; arm lines
-- 1472-1479, Repeat(20s)) — random player 8-40y,
-- DoCast(target, SPELL_RUNIC_CHARGE 62613), non-triggered (threat-list
-- pick, baltharus precedent).
-- npc_ancient_rune_giantAI JustEngagedWith (boss_thorim.cpp:1531-1536) —
-- the DoZoneInCombat first arm is PORTED 2026-10-23 (bridge live,
-- engine/world/lua_creature_events.go).
-- npc_ancient_rune_giantAI EVENT_RUNIC_FORTIFICATION (enum line 123;
-- arm lines 1559-1561, 1ms once) — Talk(EMOTE_RUNIC_MIGHT 0) +
-- DoCastAOE(SPELL_RUNIC_FORTIFICATION 62942), non-triggered.
-- npc_ancient_rune_giantAI EVENT_STOMP (enum line 124; arm lines
-- 1563-1565, Repeat(10s,12s)) — DoCastAOE(SPELL_STOMP 62411),
-- non-triggered.
-- npc_ancient_rune_giantAI EVENT_RUNE_DETONATION (enum line 125; arm
-- lines 1567-1571, Repeat(10s,12s)) — random player target 60y,
-- DoCast(target, SPELL_RUNE_DETONATION 62526), non-triggered.
-- npc_thorim_trashAI JustEngagedWith (lines 1272-1281) — the
-- DARK_RUNE_WARBRINGER-gated arm: DoCast(me, SPELL_AURA_OF_CELERITY
-- 62320), 2-arg = non-triggered self-cast. The gate is entry-resolved
-- in C++ (entry 32877 = NPC_DARK_RUNE_WARBRINGER, ulduar.h:184), so the
-- port registers that entry directly; the companion
-- DoAction(ACTION_ACTIVATE_RUNIC_SMASH) leg on the runic colossus is
-- PORTED 2026-11-05 (tick 214): arena acolyte 33110 + iron ring guard
-- 32874 engage -> colossus:DoAction(1) (ev38), see arenaAddEnterCombat.
-- npc_runic_colossusAI EVENT_RUNIC_SMASH (lines 1480-1483) — DoCast(me,
-- RAND(62058, 62057)), 7s init, Repeat(6s) — PORTED 2026-11-05 (tick 214)
-- via colossusDoAction (ev38) + _runicActive latch.
-- npc_thorim_pre_phaseAI JustDied (lines 1164-1168) -> thorim
-- DoAction(ACTION_INCREASE_PREADDS_COUNT 0); boss_thorimAI DoAction
-- (lines 835-840) ++_killedCount >= 6 -> SetImmuneToPC(false) +
-- DoZoneInCombat — PORTED 2026-11-05 (tick 214) via preAddDied (ev4
-- on the 4 pre-add entries) + thorimDoAction (ev38).
-- boss_thorimAI phase-1 timer arms PORTED 2026-11-08 (tick 478) on the
-- thorimPhase latch (1 at engage, 2 at the DamageTaken transition):
-- EVENT_SAY_AGGRO_2 (9s one-shot Talk 1), EVENT_STORMHAMMER (62042
-- DoCastVictim, 40s init, Repeat {15s,20s}), EVENT_CHARGE_ORB (62016
-- DoCastAOE, 30s init, Repeat {15s,20s}); each tick stops re-arming
-- once the latch flips (EventMap phase filter twin). EVENT_BERSERK
-- (369s, no phase group; phase-conditional body) + the DoAction
-- (ACTION_BERSERK 5) receiver (phase-2 guard + _orbSummoned latch +
-- 1s reschedule) PORTED the same tick.
-- boss_thorimAI DamageTaken phase-2 transition PORTED 2026-11-08
-- (tick 478): CanStartPhase2 (player attacker within 10yd, reach-
-- subtracted strict <, + colossus/giant ev4 death latches) -> Talk
-- SAY_JUMPDOWN 3 + phase latch 2 + EVENT_JUMPDOWN 8s + berserk
-- reschedule 300s + Sif InterruptNonMeleeSpells + hard-mode
-- SPELL_CREDIT_SIFFED 64980 (triggered AoE, DATA_LOSE_YOUR_ILLUSION
-- latch read). EVENT_JUMPDOWN partial: RemoveAura SHEATH +
-- SetReactState AGGRESSIVE + SetDisableGravity(false) + SetRooted(false)
-- (SetControlled ROOT twin) + UNBALANCING_STRIKE (62130 DoCastVictim,
-- 15s, Repeat {15s,20s}) + CHAIN_LIGHTNING (62131 random player 100y,
-- 20s, Repeat {7s,15s}); MoveJump leg stays out (no bridge).
--
-- DOCUMENTED-ONLY (no bridge; not registered):
-- boss_thorimAI EVENT_SAY_SIF_START (lines 700-702) / EVENT_START_SIF_CHANNEL
-- (lines 703-705) — cross-creature sif Talk / cast; no cross-creature bridge.
-- (The phase-1 arms around them — SAY_AGGRO_2 / STORMHAMMER / CHARGE_ORB /
-- BERSERK — are PORTED 2026-11-08, tick 478, on the thorimPhase latch; the
-- gothik stay-out is retired where the phase IS modeled.)
-- boss_thorimAI EVENT_SUMMON_ADDS (lines 716-718) — SummonWave() summon leg.
-- The SummonWave summon-time SetReactState arms ride the same unported
-- leg: JustSummoned -> summon->SetReactState(REACT_PASSIVE) (lines 651
-- lightning orb / 670 dark-rune trash / 674 Sif) and the TrashJumpEvent
-- re-arm to REACT_AGGRESSIVE (line 401 — m_Events BasicEvent, no bridge);
-- the 'no summon bridge / summon machine' wording is stale — creature:
-- SummonCreature fires CREATURE_EVENT_ON_JUST_SUMMONED_CREATURE (19)
-- on the summoner (lua_creature_events.go:766) and SetReactState IS
-- bridged (frozen_core precedent) — the dark-rune arms stay out on the
-- TrashJumpEvent legs (:386-410): stage-1 DoZoneInCombat + SetBoundary
-- and the SPELL_LEAP movement — without them the pre-phase adds would
-- stand passive forever (2026-10-23: re-evaluated against the live
-- DoZoneInCombat bridge — the stage-1 leg (boss_thorim.cpp:400,
-- _owner->AI()->DoZoneInCombat(_owner)) stays out because it is a
-- cross-creature call inside the unbridged BasicEvent machine, not a
-- plain me->DoZoneInCombat() in a ported JustEngagedWith arm.)
-- boss_thorimAI EVENT_BERSERK sender (npc_thorim_arena_phaseAI::
-- EnterEvadeMode, lines 1285-1292) stays out: Go ev2 carries no EvadeReason,
-- so the NO_HOSTILES/BOUNDARY gate cannot be modeled; the timer arm and the
-- DoAction receiver are PORTED (tick 478).
-- boss_thorimAI phase-2 machine, remainder: EVENT_ACTIVATE_LIGHTNING_FIELD
-- (bunny grid sweep + per-bunny LightningFieldEvent BasicEvent, no bridge) /
-- EVENT_START_PERIODIC_CHARGE (unregistered controller cast) /
-- EVENT_LIGHTNING_CHARGE (SetGUID-gated pillar cast, PORTED 2026-11-10
-- in activateLightningOrbTick below via GetInstanceCreature + the
-- schedule helper; the header's old "no instance-GUID bridge" note was
-- stale).
-- The transition itself + EVENT_JUMPDOWN (minus MoveJump) + UNBALANCING_STRIKE
-- + CHAIN_LIGHTNING are PORTED 2026-11-08, tick 478.
-- boss_thorimAI FinishEncounter / EVENT_OUTRO_1..3 (lines 598-601: Talks
-- SAY_END_NORMAL/HARD 8-13, hard-mode SPELL_STORMHAMMER_SIF cast) — rides
-- the 1%-hp pseudo-death, not a hook; event 4 is deliberately NOT
-- registered since C++ never calls Talk(SAY_DEATH) from JustDied.
-- boss_thorimAI SpellHit (SPELL_TOUCH_OF_DOMINION_TRIGGERED 62565 — sif
-- despawn): stale "no SpellHit bridge" retracted (2026-10-21) — ev14
-- CreatureEventOnHitBySpell is bridged; the _hardMode = false latch leg
-- is PORTED 2026-11-08 (tick 417, thorimSpellHit below; LATENT — Sif is
-- unregistered and EVENT_START_SIF_CHANNEL is unported, so 62565 never
-- lands yet; the Go gate stays HELD). The Sif Talk(SAY_SIF_DESPAWN) /
-- DespawnOrUnsummon cross-creature relay in the same arm is PORTED
-- 2026-11-09 (thorimSpellHit below; BPC cross-creature Talk +
-- freya DespawnOrUnsummon precedents).
-- SpellHitTarget (SPELL_LIGHTNING_RELEASE 62466 -> player target ->
-- _dontStandInTheLightning = false) — ev15 IS bridged (caster-side;
-- thorim casts 62466 on the pillar and the spell's player hits fire
-- thorim's ev15); the latch leg is PORTED 2026-11-08 (tick 417,
-- thorimSpellHitTarget below; LATENT — the 62466 cast rides the unported
-- phase-2 / pillar machine, so the clear never fires yet; the Go gate
-- stays HELD).
-- npc_runic_colossusAI EVENT_RUNIC_SMASH (lines 1480-1483) — PORTED
-- 2026-11-05 (tick 214), see PORTED ARMS.
-- npc_runic_colossusAI / npc_ancient_rune_giantAI Reset + JustDied — GO door legs
-- (DATA_RUNIC_DOOR / DATA_STONE_DOOR) PORTED below via boss-anchored
-- FindNearestGameObject + SetGoState (magmus/malchezaar precedent; the
-- header's old "no GO bridge" claim was stale); colossus JustDied's thorim
-- Talk(SAY_SPECIAL 2) + giant SetImmuneToPC/DoAction(ACTIVATE_ADDS) legs
-- PORTED in colossusDied below (500yd FindNearestCreature sweeps, 60yd
-- grid sweep for the giant's honor-guard/acolyte summons).
-- npc_sifAI (entry 33196) — all events gated by DoAction(ACTION_START_HARD_MODE);
-- ev38 IS bridged (tick 206) but the sender is EVENT_JUMPDOWN, unported
-- (MoveJump leg has no bridge); not registered.
-- npc_thorim_pre_phaseAI / npc_thorim_arena_phaseAI — per-entry trash-ability
-- tables (separate trash creature entries, not the boss file scope); the
-- DARK_RUNE_WARBRINGER AURA_OF_CELERITY engage arm is ported (see PORTED
-- ARMS), as are the pre-phase JustDied kill-count relay and the arena-phase
-- runic-smash relay (both PORTED 2026-11-05, tick 214). The per-entry Reset
-- ability-schedule tables stay out. The pre-phase ShouldSparWith / DamageTaken
-- pet-damage nullify arms (boss_thorim.cpp:1170-1178, !attacker->GetAffectingPlayer()
-- -> damage = 0) ride the same unregistered pre-phase trash entries; the
-- unit-side GetAffectingPlayer pet-owner resolution has no Lua bridge.
-- The ten spell scripts / condition_thorim_arena_leap / three achievements —
-- no AuraScript / ConditionScript / achievement bridges. SpellScript
-- OnEffectHitTarget IS bridged (RegisterSpellEvent ev5, skadi/gal_darah
-- precedent): 64767/64909 ported below; 62042's FilterTargets (ev7) +
-- HandleScript (ev5) ported below, LoseHammer AfterCast stays out
-- (no AfterCast bridge, spells.go:19257); 62580/62604 frostbolt
-- volley FilterTargets (ev7) ported below; 62057/62058 runic smash
-- HandleScript (ev5) ported below.

local ENTRY_THORIM = 32865
local BOSS_THORIM = 8 -- ulduar.h:44
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_RENAME = 0x00000010
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local ENTRY_RUNIC_COLOSSUS = 32872
local ENTRY_ANCIENT_RUNE_GIANT = 32873
local GO_THORIM_RUNIC_DOOR = 194557 -- ulduar.h:286
local GO_THORIM_STONE_DOOR = 194558 -- ulduar.h:287
local ENTRY_DARK_RUNE_WARBRINGER = 32877
local ENTRY_LIGHTNING_ORB = 33138 -- ulduar.h:189
local ENTRY_SIF = 33196 -- ulduar.h:192
local ENTRY_JORMUNGAR_BEHEMOTH = 32882 -- ulduar.h:173
local ENTRY_MERCENARY_CAPTAIN_A = 32908 -- ulduar.h:174
local ENTRY_MERCENARY_SOLDIER_A = 32885 -- ulduar.h:176
local ENTRY_DARK_RUNE_ACOLYTE_PRE = 32886 -- ulduar.h:178
local ENTRY_IRON_RING_GUARD = 32874 -- ulduar.h:181
local ENTRY_IRON_HONOR_GUARD = 32875 -- ulduar.h:182
local ENTRY_DARK_RUNE_ACOLYTE = 33110 -- ulduar.h:187
local ACTION_INCREASE_PREADDS_COUNT = 0 -- boss_thorim.cpp:268
local ACTION_ACTIVATE_RUNIC_SMASH = 1 -- boss_thorim.cpp:269
local ACTION_BERSERK = 5 -- boss_thorim.cpp:273
local SPELL_LIGHTNING_DESTRUCTION = 62393 -- boss_thorim.cpp:45

local SAY_AGGRO_1 = 0
local SAY_AGGRO_2 = 1 -- boss_thorim.cpp:145
local SAY_SPECIAL = 2
local SAY_JUMPDOWN = 3 -- boss_thorim.cpp:147
local SAY_SLAY = 4
local SAY_BERSERK = 5 -- boss_thorim.cpp:149
local SAY_WIPE = 6 -- boss_thorim.cpp:150
local SAY_DEATH = 7
local SAY_SIF_DESPAWN = 1 -- Sif text ids (boss_thorim.cpp:166-168): SAY_SIF_START 0, SAY_SIF_DESPAWN 1, SAY_SIF_EVENT 2

local SPELL_CREDIT_KILL = 64985
local FACTION_FRIENDLY = 35

local SPELL_SHEATH_OF_LIGHTNING = 62276
local SPELL_STORMHAMMER = 62042 -- boss_thorim.cpp:39
local SPELL_STORMHAMMER_SIF = 64767 -- boss_thorim.cpp:40
local SPELL_STORMHAMMER_BOOMERANG = 64909 -- boss_thorim.cpp:41
local SPELL_DEAFENING_THUNDER = 62470 -- boss_thorim.cpp:42
local SPELL_SIF_TRANSFORM = 64778 -- boss_thorim.cpp:73
local SPELL_FROSTBOLT_VOLLEY = 62580 -- boss_thorim.cpp:70
local SPELL_FROSTBOLT_VOLLEY_HEROIC = 62604 -- boss_thorim.cpp:1733
local THORIM_WEAPON_DISPLAY_ID = 45900 -- boss_thorim.cpp:323
local SPELL_CHARGE_ORB = 62016 -- boss_thorim.cpp:43
local SPELL_UNBALANCING_STRIKE = 62130 -- boss_thorim.cpp:54
local SPELL_CHAIN_LIGHTNING = 62131 -- boss_thorim.cpp:48
local SPELL_BERSERK_PHASE_1 = 62560 -- boss_thorim.cpp:55
local SPELL_BERSERK_PHASE_2 = 62555 -- boss_thorim.cpp:56
local SPELL_SUMMON_LIGHTNING_ORB = 62391 -- boss_thorim.cpp:44
local SPELL_CREDIT_SIFFED = 64980 -- boss_thorim.cpp:60

-- Achievement latches (boss_thorim.cpp:318-321, Data enum):
-- ACHIEVEMENT_DONT_STAND_IN_THE_LIGHTNING = 29712972,
-- ACHIEVEMENT_LOSE_YOUR_ILLUSION = 31763183. Both start true
-- (Initialize :460/:462, called by ctor + Reset) and are cleared by
-- their SpellHit arms below (tick 417; the Go gates stay HELD — both
-- clear triggers ride unported machines, so a gate now would award
-- on every kill).
local DATA_DONT_STAND_IN_THE_LIGHTNING = 29712972
local DATA_LOSE_YOUR_ILLUSION = 31763183
local SPELL_TOUCH_OF_DOMINION_TRIGGERED = 62565 -- boss_thorim.cpp:47
local SPELL_LIGHTNING_RELEASE = 62466 -- boss_thorim.cpp:51

local EMOTE_RUNIC_BARRIER = 0
local SPELL_RUNIC_BARRIER = 62338
local SPELL_SMASH = 62339
local SPELL_RUNIC_CHARGE = 62613
local SPELL_RUNIC_SMASH_LEFT = 62058 -- boss_thorim.cpp:81
local SPELL_RUNIC_SMASH_RIGHT = 62057 -- boss_thorim.cpp:80
local SPELL_RUNIC_SMASH = 62465 -- boss_thorim.cpp:79
local NPC_GOLEM_LEFT_HAND_BUNNY = 33141 -- ulduar.h:191
local NPC_GOLEM_RIGHT_HAND_BUNNY = 33140 -- ulduar.h:190

local EMOTE_RUNIC_MIGHT = 0
local SPELL_RUNIC_FORTIFICATION = 62942
local SPELL_STOMP = 62411
local SPELL_RUNE_DETONATION = 62526

-- Dark Rune Warbringer (arena-phase trash)
local SPELL_AURA_OF_CELERITY = 62320

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

local thorimFinished = {}
local thorimKilledCount = {}
local colossusRunicActive = {}
-- Phase machine, modeled as a GUID-keyed latch (assembly precedent):
-- 1 from engage (events.SetPhase(PHASE_1), :619), 2 at the DamageTaken
-- transition (:923-929). Cleared in thorimReset (C++ Initialize in
-- Reset, :456-461). Phase-1 ticks check it and stop re-arming when it
-- flips — the EventMap phase-group filter twin.
local thorimPhase = {}
local thorimOrbSummoned = {}
local thorimColossusDead = {}
local thorimGiantDead = {}

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

-- C++ SelectTarget legs: runic charge is a lambda pick (TYPEID_PLAYER
-- + IsInRange(me, 8, 40), boss_thorim.cpp:1475) and rune detonation is
-- Random, 0, 60, true (:1568) — both draw from the threat list, not
-- the map-player list (baltharus precedent). Gate twin: IsInRange
-- (Object.cpp:1329) and SelectTarget (Unit.cpp:583) — raw center minus
-- own reach minus player reach 1.5, >= min, < max, reach-exact.
local function randomPlayerInRange(creature, minDist, maxDist)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead() then
            local dist = creature:GetDistance(t) - creature:GetCombatReach() - 1.5
            if (minDist <= 0 or dist >= minDist) and dist < maxDist then
                candidates[#candidates + 1] = t
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_SAY_AGGRO_2 (boss_thorim.cpp:697-699): one-shot Talk(SAY_AGGRO_2 1),
-- 9s init, PHASE_1 group. Phase-gated: no Talk once the latch flips
-- (EventMap phase filter twin).
local function thorimSayAggro2Tick(creature, guid)
    if thorimPhase[guid] ~= 1 then
        return
    end
    creature:Talk(SAY_AGGRO_2)
end

-- C++ EVENT_STORMHAMMER (boss_thorim.cpp:708-710): DoCast(SPELL_STORMHAMMER
-- 62042) = DoCastVictim, non-triggered, 40s init, Repeat(15s, 20s), PHASE_1.
local function thorimStormhammerTick(creature, guid)
    if thorimPhase[guid] ~= 1 then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "stormhammer", 500, function()
            thorimStormhammerTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_STORMHAMMER)
    schedule(guid, "stormhammer", {15000, 20000}, function()
        thorimStormhammerTick(creature, guid)
    end)
end

-- C++ EVENT_CHARGE_ORB (boss_thorim.cpp:712-714): DoCastAOE(SPELL_CHARGE_ORB
-- 62016), non-triggered, 30s init, Repeat(15s, 20s), PHASE_1.
local function thorimChargeOrbTick(creature, guid)
    if thorimPhase[guid] ~= 1 then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "chargeOrb", 500, function()
            thorimChargeOrbTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CHARGE_ORB)
    schedule(guid, "chargeOrb", {15000, 20000}, function()
        thorimChargeOrbTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK (boss_thorim.cpp:751-762): one-shot, 369s init, NO phase
-- group — the body branches on the latch: phase 1 -> Talk(SAY_WIPE 6) +
-- DoCastAOE(SPELL_BERSERK_PHASE_1 62560, triggered) + DoCast(me,
-- SPELL_SUMMON_LIGHTNING_ORB 62391, triggered); phase 2 -> Talk(SAY_BERSERK 5)
-- + DoCast(me, SPELL_BERSERK_PHASE_2 62555, triggered). Rescheduled to 300s
-- by the DamageTaken transition and to 1s by DoAction(ACTION_BERSERK).
local function thorimBerserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            thorimBerserkTick(creature, guid)
        end)
        return
    end
    if thorimPhase[guid] == 1 then
        creature:Talk(SAY_WIPE)
        creature:CastSpell(creature, SPELL_BERSERK_PHASE_1, true)
        creature:CastSpell(creature, SPELL_SUMMON_LIGHTNING_ORB, true)
    else
        creature:Talk(SAY_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK_PHASE_2, true)
    end
end

-- Forward declarations: thorimJumpdownTick schedules both ticks below, so
-- the names must be in scope at its definition (malown idiom — plain
-- assignment defs, never a second `local function`).
local thorimUnbalancingStrikeTick, thorimChainLightningTick

-- C++ EVENT_JUMPDOWN (boss_thorim.cpp:720-732): 8s after the DamageTaken
-- transition. PORTED legs in C++ order: hard-mode Sif DoAction stays out
-- (Sif unregistered); RemoveAurasDueToSpell(SPELL_SHEATH_OF_LIGHTNING);
-- SetReactState(REACT_AGGRESSIVE); SetDisableGravity(false);
-- SetControlled(false, UNIT_STATE_ROOT) — the SetRooted(false) twin
-- (thorimReset uses SetRooted(true) for the same leg); MoveJump stays out
-- (no bridge — deviation: the boss stays on the balcony, but the combat
-- arms below still land); then the PHASE_2 schedules.
local function thorimJumpdownTick(creature, guid)
    creature:RemoveAura(SPELL_SHEATH_OF_LIGHTNING)
    creature:SetReactState(2) -- REACT_AGGRESSIVE
    creature:SetDisableGravity(false)
    creature:SetRooted(false)
    schedule(guid, "unbalancingStrike", 15000, function()
        thorimUnbalancingStrikeTick(creature, guid)
    end)
    schedule(guid, "chainLightning", 20000, function()
        thorimChainLightningTick(creature, guid)
    end)
end

-- C++ EVENT_UNBALANCING_STRIKE (boss_thorim.cpp:733-735): DoCastVictim(
-- SPELL_UNBALANCING_STRIKE 62130), non-triggered, 15s init, Repeat(15s, 20s),
-- PHASE_2.
thorimUnbalancingStrikeTick = function(creature, guid)
    if thorimPhase[guid] ~= 2 then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "unbalancingStrike", 500, function()
            thorimUnbalancingStrikeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_UNBALANCING_STRIKE)
    schedule(guid, "unbalancingStrike", {15000, 20000}, function()
        thorimUnbalancingStrikeTick(creature, guid)
    end)
end

-- C++ EVENT_CHAIN_LIGHTNING (boss_thorim.cpp:736-740): random player 100y,
-- DoCast(target, SPELL_CHAIN_LIGHTNING 62131), non-triggered, 20s init,
-- Repeat(7s, 15s), PHASE_2.
thorimChainLightningTick = function(creature, guid)
    if thorimPhase[guid] ~= 2 then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "chainLightning", 500, function()
            thorimChainLightningTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 0, 100)
    if target then
        creature:CastSpell(target, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chainLightning", {7000, 15000}, function()
        thorimChainLightningTick(creature, guid)
    end)
end

-- C++ boss_thorimAI::JustEngagedWith: Talk(SAY_AGGRO_1) + DoCast(me,
-- SPELL_SHEATH_OF_LIGHTNING 62276) 2-arg = non-triggered self-cast, both
-- ungated (line 630). The PHASE_1-grouped schedules (EVENT_SAY_AGGRO_2 /
-- EVENT_STORMHAMMER / EVENT_CHARGE_ORB / EVENT_BERSERK, lines 621/625/626/
-- 628) are PORTED 2026-11-08 (tick 478) below, gated on the thorimPhase
-- latch — the phase machine IS modeled now (assembly precedent), so the
-- gothik stay-out is retired for these arms. The SHEATH_OF_LIGHTNING
-- removal rides the ported EVENT_JUMPDOWN leg below.
local function thorimEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_THORIM, 1) -- BossAI::JustEngagedWith SetBossState(DATA_THORIM 8, IN_PROGRESS), base-first (boss_thorim.cpp:616; PORTED tick 259)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_thorim.cpp:616)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO_1)
    creature:CastSpell(creature, SPELL_SHEATH_OF_LIGHTNING)
    cancelTimers(creature:GetGUIDLow())
    local guid = creature:GetGUIDLow()
    thorimPhase[guid] = 1
    thorimOrbSummoned[guid] = false
    schedule(guid, "sayAggro2", 9000, function()
        thorimSayAggro2Tick(creature, guid)
    end)
    schedule(guid, "stormhammer", 40000, function()
        thorimStormhammerTick(creature, guid)
    end)
    schedule(guid, "chargeOrb", 30000, function()
        thorimChargeOrbTick(creature, guid)
    end)
    schedule(guid, "berserk", 369000, function()
        thorimBerserkTick(creature, guid)
    end)
    -- C++ JustEngagedWith (:632-636): runicColossus->SetImmuneToPC(false)
    -- + DoAction(ACTION_ACTIVATE_ADDS) — the miniboss base iterates the
    -- colossus's own summons (ColossusAddLocations: 4x Iron Ring Guard +
    -- 2x Dark Rune Acolyte) clearing the same flag. The 60yd sweep from
    -- the colossus excludes the giant's Dark Rune Acolytes (same entry,
    -- y≈-430, 90+yd from the colossus cluster). The GO lever leg
    -- (:638-639) stays out.
    local colossus = creature:FindNearestCreature(ENTRY_RUNIC_COLOSSUS, 150)
    if colossus then
        colossus:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        for _, guard in ipairs(colossus:GetCreaturesInGrid(ENTRY_IRON_RING_GUARD, 60)) do
            guard:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        end
        for _, acolyte in ipairs(colossus:GetCreaturesInGrid(ENTRY_DARK_RUNE_ACOLYTE, 60)) do
            acolyte:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        end
    end
    -- C++ JustEngagedWith last leg (:642): SummonCreature(NPC_SIF,
    -- SifSpawnPosition) — default TEMPSUMMON_MANUAL_DESPAWN, the bridge
    -- default.
    creature:SummonCreature(ENTRY_SIF, 2148.301, -297.8453, 438.3308, 2.687807)
end

local function thorimLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ boss_thorimAI::Reset (boss_thorim.cpp:465-477): early-return if
-- _encounterFinished (GUID-keyed latch below — a respawned Thorim is a
-- new GUID, so the latch never leaks across spawns), then
-- SetReactState(REACT_PASSIVE) + SetDisableGravity(true) +
-- SetControlled(true, UNIT_STATE_ROOT) (PORTED as SetRooted; the root is
-- lifted by the ported EVENT_JUMPDOWN leg, tick 478). The pre-phase-add summon
-- loop (:487, TEMPSUMMON_CORPSE_TIMED_DESPAWN 3s) is PORTED below,
-- IsAlive-guarded: Go event 23 also fires on death, and C++ Reset never
-- runs on death. The phase/orb/dead latches clear here (tick 478).
-- SetImmuneToPC(true) and the mini-boss respawn stay out (balcony-phase
-- gating; no instance bridge for the respawn).
local function thorimReset(event, creature)
    if thorimFinished[creature:GetGUIDLow()] then
        return
    end
    creature:SetBossState(BOSS_THORIM, 0) -- _Reset() SetBossState(DATA_THORIM 8, NOT_STARTED), first after the _encounterFinished guard per C++ :465-471 (PORTED tick 259)
    creature:SetReactState(0)
    creature:SetDisableGravity(true)
    creature:SetRooted(true)
    cancelTimers(creature:GetGUIDLow())
    thorimKilledCount[creature:GetGUIDLow()] = nil
    -- C++ Reset -> Initialize (boss_thorim.cpp:456-461): _hardMode
    -- = true, _dontStandInTheLightning = true. Alive-gated: ev23
    -- fires on death too (fireCreatureDied runs Reset before the
    -- kill-credit criteria read, kill.go:1091/:1400), and a
    -- death-time re-arm would clobber the fight's latches before
    -- the achievement gates read them. (Both gates are HELD this
    -- tick — their clear legs are dormant — so this guards the
    -- future unblock, not a live gate.)
    if creature:IsAlive() then
        creature:SetData(DATA_DONT_STAND_IN_THE_LIGHTNING, 1)
        creature:SetData(DATA_LOSE_YOUR_ILLUSION, 1)
    end
    local guid = creature:GetGUIDLow()
    thorimPhase[guid] = nil
    thorimOrbSummoned[guid] = nil
    thorimColossusDead[guid] = nil
    thorimGiantDead[guid] = nil
    if creature:IsAlive() then
        creature:SummonCreature(ENTRY_JORMUNGAR_BEHEMOTH, 2149.68, -263.477, 419.679, 3.120, 5, 3000)
        creature:SummonCreature(ENTRY_MERCENARY_CAPTAIN_A, 2131.31, -271.640, 419.840, 2.188, 5, 3000)
        creature:SummonCreature(ENTRY_MERCENARY_SOLDIER_A, 2127.24, -259.182, 419.974, 5.917, 5, 3000)
        creature:SummonCreature(ENTRY_MERCENARY_SOLDIER_A, 2123.32, -254.770, 419.840, 6.170, 5, 3000)
        creature:SummonCreature(ENTRY_MERCENARY_SOLDIER_A, 2120.10, -258.990, 419.840, 6.250, 5, 3000)
        creature:SummonCreature(ENTRY_DARK_RUNE_ACOLYTE_PRE, 2129.09, -277.142, 419.756, 1.222, 5, 3000)
    end
end

-- C++ npc_thorim_pre_phaseAI::JustDied (boss_thorim.cpp:1164-1168):
-- instance->GetCreature(BOSS_THORIM)->AI()->DoAction(
-- ACTION_INCREASE_PREADDS_COUNT 0) — the instance fetch rides the
-- 500yd FindNearestCreature sweep (colossusDied precedent below).
local function preAddDied(event, creature, killer)
    local thorim = creature:FindNearestCreature(ENTRY_THORIM, 500)
    if thorim then
        thorim:DoAction(ACTION_INCREASE_PREADDS_COUNT)
    end
end

-- C++ boss_thorimAI::DoAction ACTION_INCREASE_PREADDS_COUNT
-- (boss_thorim.cpp:835-840): ++_killedCount >= 6 -> SetImmuneToPC(false)
-- + DoZoneInCombat(me) — the encounter start. The count is GUID-keyed
-- and cleared in thorimReset (C++ Initialize() in Reset, :456-461).
-- The RemoveFlag leg is currently a no-op: Lua never sets Thorim immune
-- (header), but it is the C++ arm, kept verbatim.
local function thorimDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == ACTION_INCREASE_PREADDS_COUNT then
        local count = (thorimKilledCount[guid] or 0) + 1
        thorimKilledCount[guid] = count
        if count >= 6 then
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
            creature:DoZoneInCombat()
        end
        return
    end
    -- C++ boss_thorimAI::DoAction ACTION_BERSERK (boss_thorim.cpp:825-833):
    -- phase-2 guard first (events.IsInPhase(PHASE_2) -> return), then the
    -- _orbSummoned latch: first send only, RescheduleEvent(EVENT_BERSERK, 1s).
    -- The sender (npc_thorim_arena_phaseAI::EnterEvadeMode, :1285-1292) stays
    -- out: Go ev2 carries no EvadeReason, so the NO_HOSTILES/BOUNDARY gate
    -- cannot be modeled — an ev2 send would over-fire on any add evade.
    if action == ACTION_BERSERK then
        if thorimPhase[guid] == 2 then
            return
        end
        if not thorimOrbSummoned[guid] then
            thorimOrbSummoned[guid] = true
            schedule(guid, "berserk", 1000, function()
                thorimBerserkTick(creature, guid)
            end)
        end
        return
    end
end

-- C++ FinishEncounter is a 1%-hp pseudo-death, not a hook: C++ never calls
-- JustDied for Thorim, so no Talk is registered on event 4 — timers are
-- only cancelled. The true-kill fallback below writes DONE via the base
-- BossAI::JustDied (_JustDied), which C++ would run if Thorim were killed
-- outright (boss_thorimAI overrides no JustDied).
local function thorimDied(event, creature, killer)
    creature:SetBossState(BOSS_THORIM, 3) -- true-kill fallback: BossAI::JustDied -> _JustDied() DONE (PORTED tick 259)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ boss_thorimAI::KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function thorimTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ boss_thorimAI::DamageTaken else-if (boss_thorim.cpp:940-943):
-- HealthBelowPctDamaged(1, damage) -> damage = 0 + FinishEncounter().
-- The floor form is C++-exact: HealthBelowPctDamaged compares strict <
-- against integer-truncated max/100 (Unit.h:873); no postDamage > 0
-- guard — the Damaged form fires on lethal hits by definition. The
-- FinishEncounter self-legs are ported in C++ order (:572-579, :597):
-- credit DoCastAOE (self-cast, triggered) -> ReactPassive ->
-- Interrupt -> RemoveAllAttackers -> AttackStop -> FactionFriendly ->
-- RENAME flag -> Talk(SAY_DEATH). The _JustDied() SetBossState
-- (DATA_THORIM 8, DONE) leg PORTED tick 259 between the RENAME flag
-- and Talk (C++ FinishEncounter order :597-599). The phase-1 jumpdown branch is the
-- else-if's guard in C++ and is PORTED above (tick 478): Thorim enters
-- phase 1 at full health, so no single hit can cross 99% of max
-- health — the arm is effectively phase-2-only. Controller/pillar
-- aura clears, hard-mode Sif despawn and the outro Talks 8-13
-- (hard-mode branch timing) stay out.
local function thorimDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local postDamage = creature:GetHealth() - damage
    -- C++ boss_thorimAI::DamageTaken phase-1 branch (boss_thorim.cpp:921-936):
    -- if (IsInPhase(PHASE_1) && CanStartPhase2(attacker)): Talk(SAY_JUMPDOWN 3)
    -- + SetPhase(PHASE_2) + ScheduleEvent(EVENT_JUMPDOWN, 8s) +
    -- RescheduleEvent(EVENT_BERSERK, 300s, PHASE_2) + Sif
    -- InterruptNonMeleeSpells + hard-mode DoCastAOE(SPELL_CREDIT_SIFFED 64980,
    -- triggered). CanStartPhase2 (:911-919): attacker is a player within 10yd
    -- (IsWithinDistInMap — strict <, reach-subtracted, randomPlayerInRange
    -- idiom) AND the runic colossus + rune giant are both dead (latched in
    -- colossusDied/giantDied below; the FindNearestCreature sweep is
    -- live-only, so a dead mini-boss is unreadable — the ev4 latch is the
    -- twin). EVENT_ACTIVATE_LIGHTNING_FIELD (15s) stays out (bunny
    -- BasicEvent machine). The hard-mode read rides the
    -- DATA_LOSE_YOUR_ILLUSION latch (Reset 1 / SpellHit 62565 clear —
    -- the same slot C++ uses for _hardMode, :531-533).
    if thorimPhase[guid] == 1 and attacker and attacker:GetObjectType() == "Player"
        and creature:GetDistance(attacker) - creature:GetCombatReach() - 1.5 < 10
        and thorimColossusDead[guid] and thorimGiantDead[guid] then
        creature:Talk(SAY_JUMPDOWN)
        thorimPhase[guid] = 2
        schedule(guid, "jumpdown", 8000, function()
            thorimJumpdownTick(creature, guid)
        end)
        schedule(guid, "berserk", 300000, function()
            thorimBerserkTick(creature, guid)
        end)
        local sif = creature:FindNearestCreature(ENTRY_SIF, 200)
        if sif then
            sif:InterruptNonMeleeSpells()
        end
        if creature:GetData(DATA_LOSE_YOUR_ILLUSION) == 1 then
            creature:CastSpell(creature, SPELL_CREDIT_SIFFED, true)
        end
    elseif postDamage < math.floor(creature:GetMaxHealth() / 100) then
        if thorimFinished[creature:GetGUIDLow()] then
            return false, 0
        end
        thorimFinished[creature:GetGUIDLow()] = true
        creature:CastSpell(creature, SPELL_CREDIT_KILL, true)
        creature:SetReactState(0)
        creature:InterruptNonMeleeSpells()
        creature:RemoveAllAttackers()
        creature:AttackStop()
        creature:SetFaction(FACTION_FRIENDLY)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_RENAME)
        creature:SetBossState(BOSS_THORIM, 3) -- FinishEncounter _JustDied() SetBossState(DATA_THORIM 8, DONE), C++ order after the RENAME flag, before Talk(SAY_DEATH) (:597-599; PORTED tick 259)
        creature:Talk(SAY_DEATH)
        return false, 0
    end
end

-- C++ EVENT_RUNIC_BARRIER: Talk(EMOTE_RUNIC_BARRIER 0) +
-- DoCastAOE(SPELL_RUNIC_BARRIER 62338) (non-triggered), 12-15s init,
-- Repeat(35s, 45s).
local function colossusRunicBarrierTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "runicbarrier", 500, function()
            colossusRunicBarrierTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_RUNIC_BARRIER)
    creature:CastSpell(creature, SPELL_RUNIC_BARRIER)
    schedule(guid, "runicbarrier", {35000, 45000}, function()
        colossusRunicBarrierTick(creature, guid)
    end)
end

-- C++ EVENT_SMASH: DoCastAOE(SPELL_SMASH 62339) (non-triggered),
-- 15-18s init, Repeat(15s, 18s).
local function colossusSmashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "smash", 500, function()
            colossusSmashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SMASH)
    schedule(guid, "smash", {15000, 18000}, function()
        colossusSmashTick(creature, guid)
    end)
end

-- C++ EVENT_RUNIC_CHARGE: random player 8-40y,
-- DoCast(target, SPELL_RUNIC_CHARGE 62613) (non-triggered), 20-24s init,
-- Repeat(20s).
local function colossusRunicChargeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "runiccharge", 500, function()
            colossusRunicChargeTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 8, 40)
    if target then
        creature:CastSpell(target, SPELL_RUNIC_CHARGE)
    end
    schedule(guid, "runiccharge", 20000, function()
        colossusRunicChargeTick(creature, guid)
    end)
end

-- C++ EVENT_RUNIC_SMASH (boss_thorim.cpp:1480-1483): DoCast(me,
-- RAND(SPELL_RUNIC_SMASH_LEFT 62058, SPELL_RUNIC_SMASH_RIGHT 62057)),
-- non-triggered self-cast, 7s init via DoAction, Repeat(6s).
local function colossusRunicSmashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "runicSmash", 500, function()
            colossusRunicSmashTick(creature, guid)
        end)
        return
    end
    if math.random(2) == 1 then
        creature:CastSpell(creature, SPELL_RUNIC_SMASH_LEFT)
    else
        creature:CastSpell(creature, SPELL_RUNIC_SMASH_RIGHT)
    end
    schedule(guid, "runicSmash", 6000, function()
        colossusRunicSmashTick(creature, guid)
    end)
end

-- C++ npc_runic_colossusAI::DoAction (boss_thorim.cpp:1414-1426):
-- npc_thorim_minibossAI::DoAction passes through (handles only
-- ACTION_ACTIVATE_ADDS 2); if _runicActive return; on
-- ACTION_ACTIVATE_RUNIC_SMASH (1): latch + ScheduleEvent(
-- EVENT_RUNIC_SMASH, 7s). The latch is GUID-keyed and cleared in
-- colossusReset (C++ Initialize() in Reset).
local function colossusDoAction(event, creature, action)
    if action ~= ACTION_ACTIVATE_RUNIC_SMASH then
        return
    end
    local guid = creature:GetGUIDLow()
    if colossusRunicActive[guid] then
        return
    end
    colossusRunicActive[guid] = true
    schedule(guid, "runicSmash", 7000, function()
        colossusRunicSmashTick(creature, guid)
    end)
end

-- C++ npc_runic_colossusAI::JustEngagedWith (boss_thorim.cpp:1444-1449):
-- DoZoneInCombat() first (PORTED 2026-10-23 — bridge live,
-- engine/world/lua_creature_events.go), then _events.Reset() and the
-- three ScheduleEvent arms. (EVENT_RUNIC_SMASH rides
-- DoAction(ACTION_ACTIVATE_RUNIC_SMASH) — PORTED 2026-11-05, tick 214.)
local function colossusEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- C++ :1444, first arm (bridge live 2026-10-23)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "runicbarrier", {12000, 15000}, function()
        colossusRunicBarrierTick(creature, guid)
    end)
    schedule(guid, "smash", {15000, 18000}, function()
        colossusSmashTick(creature, guid)
    end)
    schedule(guid, "runiccharge", {20000, 24000}, function()
        colossusRunicChargeTick(creature, guid)
    end)
end

local function colossusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_runic_colossusAI::Reset (:1395-1406): _summons.DespawnAll (no
-- summon-tracking bridge — stays out; Go event 23 never fires on
-- leave-combat, so no evade duplication arises) + the ColossusAddLocations
-- summon loop (TEMPSUMMON_CORPSE_TIMED_DESPAWN 3s), PORTED below,
-- IsAlive-guarded like thorimReset. The runic-door instance leg stays out.
local function colossusReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    colossusRunicActive[creature:GetGUIDLow()] = nil
    local runicDoor = creature:FindNearestGameObject(GO_THORIM_RUNIC_DOOR, 100)
    if runicDoor then runicDoor:SetGoState(1) end
    if creature:IsAlive() then
        creature:SummonCreature(ENTRY_IRON_RING_GUARD, 2218.38, -297.50, 412.18, 1.030, 5, 3000)
        creature:SummonCreature(ENTRY_IRON_RING_GUARD, 2235.07, -297.98, 412.18, 1.613, 5, 3000)
        creature:SummonCreature(ENTRY_IRON_RING_GUARD, 2235.26, -338.34, 412.18, 1.589, 5, 3000)
        creature:SummonCreature(ENTRY_IRON_RING_GUARD, 2217.69, -337.39, 412.18, 1.241, 5, 3000)
        creature:SummonCreature(ENTRY_DARK_RUNE_ACOLYTE, 2227.58, -308.30, 412.18, 1.591, 5, 3000)
        creature:SummonCreature(ENTRY_DARK_RUNE_ACOLYTE, 2227.47, -345.37, 412.18, 1.566, 5, 3000)
    end
end

local function colossusDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    local runicDoor = creature:FindNearestGameObject(GO_THORIM_RUNIC_DOOR, 100)
    if runicDoor then runicDoor:SetGoState(0) end
    -- C++ npc_runic_colossusAI::JustDied (boss_thorim.cpp:1429-1441), in
    -- order after the door leg: thorim Talk(SAY_SPECIAL 2) + giant
    -- SetImmuneToPC(false) + giant DoAction(ACTION_ACTIVATE_ADDS)
    -- (miniboss base :1362-1370: un-immune its summons). Instance GUID
    -- fetches ride the FindNearestCreature sweep (500yd, kalecgos
    -- precedent); the summon list rides the colossus-activation grid
    -- sweep (60yd, same file): giant's honor guards + acolytes only —
    -- the colossus-cluster acolytes sit ~90yd from the giant's adds.
    local thorim = creature:FindNearestCreature(ENTRY_THORIM, 500)
    if thorim then thorim:Talk(SAY_SPECIAL) end
    -- CanStartPhase2 latch (boss_thorim.cpp:911-919): the colossus is dead.
    -- Keyed by Thorim's GUID — the sweep is the same 500yd fetch as the
    -- Talk leg above.
    if thorim then
        thorimColossusDead[thorim:GetGUIDLow()] = true
    end
    local giant = creature:FindNearestCreature(ENTRY_ANCIENT_RUNE_GIANT, 500)
    if giant then
        giant:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        for _, guard in ipairs(giant:GetCreaturesInGrid(ENTRY_IRON_HONOR_GUARD, 60)) do
            guard:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        end
        for _, acolyte in ipairs(giant:GetCreaturesInGrid(ENTRY_DARK_RUNE_ACOLYTE, 60)) do
            acolyte:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        end
    end
end

-- C++ EVENT_STOMP: DoCastAOE(SPELL_STOMP 62411) (non-triggered),
-- 10-12s init, Repeat(10s, 12s).
local function giantStompTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "stomp", 500, function()
            giantStompTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", {10000, 12000}, function()
        giantStompTick(creature, guid)
    end)
end

-- C++ EVENT_RUNE_DETONATION: random player target 60y,
-- DoCast(target, SPELL_RUNE_DETONATION 62526) (non-triggered), 25s init,
-- Repeat(10s, 12s).
local function giantRuneDetonationTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "runedetonation", 500, function()
            giantRuneDetonationTick(creature, guid)
        end)
        return
    end
    local target = randomPlayerInRange(creature, 0, 60)
    if target then
        creature:CastSpell(target, SPELL_RUNE_DETONATION)
    end
    schedule(guid, "runedetonation", {10000, 12000}, function()
        giantRuneDetonationTick(creature, guid)
    end)
end

-- C++ npc_ancient_rune_giantAI::JustEngagedWith (boss_thorim.cpp:1531-1535):
-- DoZoneInCombat() first (PORTED 2026-10-23 — bridge live,
-- engine/world/lua_creature_events.go), then _events.Reset() and the
-- three ScheduleEvent arms.
-- C++ EVENT_RUNIC_FORTIFICATION one-shot (boss_thorim.cpp:1556-1559):
-- named tick so the 500ms IsCasting defer does not consume the
-- cast — the auriaya berserk precedent; under the giant
-- pre-loop (:1552) + in-loop (:1576) CASTING gates.
local function giantRunicFortificationTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "runicfortification", 500, function()
            giantRunicFortificationTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_RUNIC_MIGHT)
    creature:CastSpell(creature, SPELL_RUNIC_FORTIFICATION)
end

local function giantEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- C++ :1532, first arm (bridge live 2026-10-23)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "runicfortification", 1, function()
        giantRunicFortificationTick(creature, guid)
    end)
    schedule(guid, "stomp", {10000, 12000}, function()
        giantStompTick(creature, guid)
    end)
    schedule(guid, "runedetonation", 25000, function()
        giantRuneDetonationTick(creature, guid)
    end)
end

local function giantLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_ancient_rune_giantAI::Reset (:1517-1529): same shape as
-- colossusReset — _summons.DespawnAll stays out, the GiantAddLocations
-- summon loop (TEMPSUMMON_CORPSE_TIMED_DESPAWN 3s) PORTED below,
-- IsAlive-guarded. The stone-door instance leg stays out.
local function giantReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    local stoneDoor = creature:FindNearestGameObject(GO_THORIM_STONE_DOOR, 100)
    if stoneDoor then stoneDoor:SetGoState(1) end
    if creature:IsAlive() then
        creature:SummonCreature(ENTRY_IRON_HONOR_GUARD, 2198.05, -428.77, 419.95, 6.056, 5, 3000)
        creature:SummonCreature(ENTRY_IRON_HONOR_GUARD, 2220.31, -436.22, 412.26, 1.064, 5, 3000)
        creature:SummonCreature(ENTRY_IRON_HONOR_GUARD, 2158.88, -441.73, 438.25, 0.127, 5, 3000)
        creature:SummonCreature(ENTRY_DARK_RUNE_ACOLYTE, 2198.29, -436.92, 419.95, 0.261, 5, 3000)
        creature:SummonCreature(ENTRY_DARK_RUNE_ACOLYTE, 2230.93, -434.27, 412.26, 1.931, 5, 3000)
    end
end

local function giantDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    local stoneDoor = creature:FindNearestGameObject(GO_THORIM_STONE_DOOR, 100)
    if stoneDoor then stoneDoor:SetGoState(0) end
    -- CanStartPhase2 latch (boss_thorim.cpp:911-919): the rune giant is dead.
    local thorim = creature:FindNearestCreature(ENTRY_THORIM, 500)
    if thorim then
        thorimGiantDead[thorim:GetGUIDLow()] = true
    end
end

-- C++ JustSummoned NPC_LIGHTNING_ORB (boss_thorim.cpp:649-663):
-- SetReactState(REACT_PASSIVE) + CastSpell(summon, SPELL_LIGHTNING_DESTRUCTION
-- 62393, triggered) in C++ order; the spline-path movement leg has no bridge.
-- LATENT: orbs spawn only from the unported SPELL_SUMMON_LIGHTNING_ORB 62391
-- spell-summon (anubrekhan latent precedent). NOTE: TrashJumpEvent (:386-410)
-- clears only dark-rune trash, not the orb — the orb stays passive in C++ too.
local function thorimJustSummoned(_, creature, summoned)
    if summoned:GetEntry() == ENTRY_LIGHTNING_ORB then
        summoned:SetReactState(0) -- REACT_PASSIVE
        summoned:CastSpell(summoned, SPELL_LIGHTNING_DESTRUCTION, true)
    elseif summoned:GetEntry() == ENTRY_SIF then
        -- C++ JustSummoned NPC_SIF (:673-675): bare SetReactState
        -- (REACT_PASSIVE) — unlike the dark-rune trash, Sif gets no
        -- TrashJumpEvent, so this leg is complete on its own.
        summoned:SetReactState(0) -- REACT_PASSIVE
    end
end

-- C++ boss_thorimAI::SpellHit (boss_thorim.cpp:548-556): on
-- SPELL_TOUCH_OF_DOMINION_TRIGGERED 62565 (Sif's Touch of Dominion,
-- :703-705) _hardMode = false — the lose-your-illusion latch (the
-- same slot doubles as DATA_THORIM_HARDMODE, :531-533) — but ONLY
-- inside `if (Creature* sif = instance->GetCreature(DATA_SIF))`:
-- no Sif in the instance, no clear. The Lua twin is the
-- FindNearestCreature sweep (colossusDied precedent): Sif is
-- summoned at engage (2148.301, -297.8453), well inside 200yd of
-- Thorim. The Sif Talk(SAY_SIF_DESPAWN)/DespawnOrUnsummon relay in
-- the same arm is PORTED 2026-11-09 below (same FindNearestCreature
-- gate; C++ order Talk -> DespawnOrUnsummon(6s) -> _hardMode=false).
-- LATENT, Go gate HELD: Sif is
-- unregistered and EVENT_START_SIF_CHANNEL is unported, so 62565
-- never lands in Lua yet and the latch stays 1 (a gate now would
-- award on every kill).
local function thorimSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_TOUCH_OF_DOMINION_TRIGGERED then
        local sif = creature:FindNearestCreature(ENTRY_SIF, 200)
        if sif then
            sif:Talk(SAY_SIF_DESPAWN)
            sif:DespawnOrUnsummon(6000)
            creature:SetData(DATA_LOSE_YOUR_ILLUSION, 0)
        end
    end
end

-- C++ boss_thorimAI::SpellHitTarget (boss_thorim.cpp:560-562): on
-- SPELL_LIGHTNING_RELEASE 62466 hitting a player,
-- _dontStandInTheLightning = false. ev15 fires caster-side with the
-- hit target (gruul precedent). LATENT, Go gate HELD: the 62466 cast
-- rides EVENT_LIGHTNING_CHARGE (:746-749) behind the unported phase-2
-- / pillar machine, so the clear never fires in Lua yet and the latch
-- stays 1 (a gate now would award on every kill).
local function thorimSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_LIGHTNING_RELEASE and target and target:GetObjectType() == "Player" then
        creature:SetData(DATA_DONT_STAND_IN_THE_LIGHTNING, 0)
    end
end

RegisterCreatureEvent(ENTRY_THORIM, 1, thorimEnterCombat)
RegisterCreatureEvent(ENTRY_THORIM, 2, thorimLeaveCombat)
RegisterCreatureEvent(ENTRY_THORIM, 3, thorimTargetDied)
RegisterCreatureEvent(ENTRY_THORIM, 4, thorimDied)
RegisterCreatureEvent(ENTRY_THORIM, 9, thorimDamageTaken)
RegisterCreatureEvent(ENTRY_THORIM, 14, thorimSpellHit)
RegisterCreatureEvent(ENTRY_THORIM, 15, thorimSpellHitTarget)
RegisterCreatureEvent(ENTRY_THORIM, 23, thorimReset)
RegisterCreatureEvent(ENTRY_THORIM, 19, thorimJustSummoned)
RegisterCreatureEvent(ENTRY_THORIM, 38, thorimDoAction)

RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 1, colossusEnterCombat)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 2, colossusLeaveCombat)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 4, colossusDied)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 23, colossusReset)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 38, colossusDoAction)

-- C++ npc_runic_colossusAI::MoveInLineOfSight (boss_thorim.cpp:1409) is an
-- EMPTY override — it suppresses the default ScriptedAI::MoveInLineOfSight
-- aggro engage (the colossus only fights when the gauntlet reaches it).
-- Go's fireCreatureMoveInLOS (lua_creature_events.go:531) fires ev27 on the
-- aggro-engage branch and a boolean true vetoes exactly that engage, so
-- return true models :1383 C++-exact (the cyanigosa precedent).
local function colossusMoveInLOS(event, creature, unit)
    return true
end

RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 27, colossusMoveInLOS)

RegisterCreatureEvent(ENTRY_ANCIENT_RUNE_GIANT, 1, giantEnterCombat)
RegisterCreatureEvent(ENTRY_ANCIENT_RUNE_GIANT, 2, giantLeaveCombat)
RegisterCreatureEvent(ENTRY_ANCIENT_RUNE_GIANT, 4, giantDied)
RegisterCreatureEvent(ENTRY_ANCIENT_RUNE_GIANT, 23, giantReset)

-- C++ npc_thorim_trashAI::JustEngagedWith (boss_thorim.cpp:1272-1281):
-- if (_info->Type == DARK_RUNE_WARBRINGER) DoCast(me,
-- SPELL_AURA_OF_CELERITY 62320), 2-arg = non-triggered self-cast.
-- The gate is entry-resolved (entry 32877 = NPC_DARK_RUNE_WARBRINGER,
-- ulduar.h:184), so the arm registers on that entry directly; the
-- companion DoAction(ACTION_ACTIVATE_RUNIC_SMASH) colossus leg is
-- PORTED 2026-11-05 (tick 214), see arenaAddEnterCombat.
local function warbringerEnterCombat(event, creature, target)
    creature:CastSpell(creature, SPELL_AURA_OF_CELERITY)
end

-- C++ npc_thorim_arena_phaseAI::JustEngagedWith (boss_thorim.cpp:1272-1280):
-- the warbringer AURA_OF_CELERITY arm is ported above; the companion
-- leg — if !_isInArena, instance->GetCreature(DATA_RUNIC_COLOSSUS)->
-- AI()->DoAction(ACTION_ACTIVATE_RUNIC_SMASH 1) — fires for the
-- arena-phase members with _isInArena=false: the Dark Rune Acolyte
-- (33110; the pre-phase acolyte 32886 is entry-gated true, :1240)
-- and the Iron Ring Guard (32874; default branch, :1245). The
-- champion/warbringer/commoner/evoker are _isInArena=true (:1236)
-- and never send. The instance fetch rides the 500yd
-- FindNearestCreature sweep (colossusDied precedent).
local function arenaAddEnterCombat(event, creature, target)
    local colossus = creature:FindNearestCreature(ENTRY_RUNIC_COLOSSUS, 500)
    if colossus then
        colossus:DoAction(ACTION_ACTIVATE_RUNIC_SMASH)
    end
end

RegisterCreatureEvent(ENTRY_DARK_RUNE_WARBRINGER, 1, warbringerEnterCombat)
RegisterCreatureEvent(ENTRY_DARK_RUNE_ACOLYTE, 1, arenaAddEnterCombat)
RegisterCreatureEvent(ENTRY_IRON_RING_GUARD, 1, arenaAddEnterCombat)
RegisterCreatureEvent(ENTRY_JORMUNGAR_BEHEMOTH, 4, preAddDied)
RegisterCreatureEvent(ENTRY_MERCENARY_CAPTAIN_A, 4, preAddDied)
RegisterCreatureEvent(ENTRY_MERCENARY_SOLDIER_A, 4, preAddDied)
RegisterCreatureEvent(ENTRY_DARK_RUNE_ACOLYTE_PRE, 4, preAddDied)

-- C++ spell_thorim_stormhammer_sif::HandleScript (boss_thorim.cpp:1970-1976):
-- EFFECT_0 SCRIPT_EFFECT OnEffectHitTarget — the hit unit casts
-- SPELL_STORMHAMMER_BOOMERANG (64909) on the caster, triggered, and
-- casts SPELL_SIF_TRANSFORM (64778) on itself, triggered. The AfterCast
-- LoseHammer leg (caster UNIT_VIRTUAL_ITEM_SLOT_ID = 0, :1978-1981)
-- stays out: CallScriptAfterCastHandlers is a Go no-op (spells.go:19257),
-- no AfterCast bridge. LATENT — the hard-mode outro cast (EVENT_OUTRO_1,
-- :798-799) stays out, so 64767 never lands yet. DORMANT until Spell.dbc
-- ships the 64767 record.
local function stormhammerSifHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:CastSpell(caster, SPELL_STORMHAMMER_BOOMERANG, true)
    hitTarget:CastSpell(hitTarget, SPELL_SIF_TRANSFORM, true)
end

RegisterSpellEvent(SPELL_STORMHAMMER_SIF, 5, stormhammerSifHit)

-- C++ spell_thorim_stormhammer_boomerang::RecoverHammer
-- (boss_thorim.cpp:2007-2010): EFFECT_0 DUMMY OnEffectHitTarget — the
-- hit unit (the caster, Thorim — the boomerang is cast on the caster)
-- restores UNIT_VIRTUAL_ITEM_SLOT_ID to THORIM_WEAPON_DISPLAY_ID 45900.
-- SetEquipmentSlots(main, 0xFFFFFFFF, 0xFFFFFFFF) writes slot 0 only
-- (malchezaar/zuljin precedent). DORMANT until Spell.dbc ships 64909.
local function stormhammerBoomerangHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:SetEquipmentSlots(THORIM_WEAPON_DISPLAY_ID, 0xFFFFFFFF, 0xFFFFFFFF)
end

RegisterSpellEvent(SPELL_STORMHAMMER_BOOMERANG, 5, stormhammerBoomerangHit)

-- C++ spell_thorim_stormhammer::FilterTargets (boss_thorim.cpp:1911-1924):
-- OnObjectAreaTargetSelect EFFECT_ALL TARGET_UNIT_SRC_AREA_ENEMY —
-- drop targets above the balcony (Z > 428.0, HeightPositionCheck) or
-- outside the arena (ArenaBoundaries: inside the floor circle
-- (2134.77, -262.307, r 45.4) and outside the balcony circle
-- (2135.178, -321.122, r 32.0, inverted)); an empty list fails the cast
-- (FinishCast SPELL_FAILED_NO_VALID_TARGETS), else one random target
-- is kept. The AfterCast LoseHammer leg (caster
-- UNIT_VIRTUAL_ITEM_SLOT_ID = 0, :1935-1938) stays out:
-- CallScriptAfterCastHandlers is a Go no-op (spells.go:19257), no
-- AfterCast bridge. DELTA: no FinishCast bridge — an emptied list
-- leaves the cast to resolve with no script-effect hit, so the
-- damage/stun arms still land on the victim where C++ aborts the
-- whole cast. DORMANT until Spell.dbc ships the 62042 record.
local function stormhammerFilter(event, spell, effectIndex, targets)
    local kept = {}
    for _, target in ipairs(targets:GetTargets()) do
        if target:GetZ() <= 428.0
                and target:GetDistance2d(2134.77, -262.307) <= 45.4
                and target:GetDistance2d(2135.178, -321.122) > 32.0 then
            kept[#kept + 1] = target
        end
    end
    targets:Clear()
    if #kept > 0 then
        targets:Add(kept[math.random(#kept)])
    end
end

RegisterSpellEvent(SPELL_STORMHAMMER, 7, stormhammerFilter)

-- C++ spell_thorim_stormhammer::HandleScript (boss_thorim.cpp:1926-1933):
-- EFFECT_2 SCRIPT_EFFECT OnEffectHitTarget — the hit unit casts
-- SPELL_DEAFENING_THUNDER (62470) on itself, triggered, and casts
-- SPELL_STORMHAMMER_BOOMERANG (64909) on the caster, triggered.
-- DORMANT until Spell.dbc ships the 62042 record.
local function stormhammerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 2 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_DEAFENING_THUNDER, true)
    hitTarget:CastSpell(caster, SPELL_STORMHAMMER_BOOMERANG, true)
end

RegisterSpellEvent(SPELL_STORMHAMMER, 5, stormhammerHit)

-- C++ spell_thorim_activate_lightning_orb_periodic::PeriodicTick
-- (boss_thorim.cpp:2090-2109): PreventDefaultAction; the aura caster
-- (the Thorim controller, :744) searches thunder orbs (NPC_THUNDER_ORB)
-- within 100.0f that sit ABOVE the balcony (UpperOrbCheck, :2064-2076:
-- HeightPositionCheck(true), Z > 428.0), picks one at random, and hands
-- its GUID to Thorim's SetGUID(DATA_CHARGED_PILLAR) (:511-522: the pillar
-- casts SPELL_LIGHTNING_ORB_CHARGED on itself triggered and
-- SPELL_LIGHTNING_PILLAR_2 (62976) on no target, then
-- EVENT_LIGHTNING_CHARGE fires 8s later — Thorim casts
-- SPELL_LIGHTNING_RELEASE on the pillar). LATENT:
-- EVENT_START_PERIODIC_CHARGE (the controller cast, :744) is unported,
-- so the aura never lands yet. The SetGUID arm is inlined here because
-- GetInstanceCreature(BOSS_THORIM) IS the instance-GUID bridge (the
-- file header's "no instance-GUID bridge" note is stale — illidari
-- council precedent). SPELL_LIGHTNING_PILLAR_2 stays out: CastSpell(nil)
-- falls back to the current target or no-ops, there is no nil-target
-- cast bridge. The release is scheduled on Thorim's GUID through the
-- file's schedule helper (replacement semantics vs C++'s duplicate
-- EventMap entries — vacuous here: the aura ticks every 16s (DBC
-- amplitude) against an 8s release, so a re-tick can never land inside
-- the window). DELTAS: GetCreaturesInRange is a 3D + alive-only sweep
-- where C++ visits 2D grid cells with no alive filter.
local SPELL_ACTIVATE_LIGHTNING_ORB_PERIODIC = 62184 -- boss_thorim.cpp:57
local SPELL_LIGHTNING_ORB_CHARGED = 62186 -- boss_thorim.cpp:49
local NPC_THUNDER_ORB = 33378 -- ulduar.h:193
local THORIM_BALCONY_Z_CHECK = 428.0 -- boss_thorim.cpp:369

local function activateLightningOrbTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return true
    end
    local upper = {}
    for _, orb in ipairs(caster:GetCreaturesInRange(100, NPC_THUNDER_ORB)) do
        if orb:GetZ() > THORIM_BALCONY_Z_CHECK then
            upper[#upper + 1] = orb
        end
    end
    if #upper == 0 then
        return true
    end
    local pillar = upper[math.random(1, #upper)]
    local thorim = caster:GetInstanceCreature(BOSS_THORIM)
    if thorim == nil then
        return true
    end
    pillar:CastSpell(pillar, SPELL_LIGHTNING_ORB_CHARGED, true)
    local pillarLow = pillar:GetGUIDLow()
    local thorimLow = thorim:GetGUIDLow()
    schedule(thorimLow, "lightningCharge", 8000, function()
        local pillarNow = thorim:GetCreatureByGUID(pillarLow)
        if pillarNow ~= nil then
            thorim:CastSpell(pillarNow, SPELL_LIGHTNING_RELEASE, false)
        end
    end)
    return true
end

RegisterAuraEvent(SPELL_ACTIVATE_LIGHTNING_ORB_PERIODIC, 1, activateLightningOrbTick)

-- C++ spell_thorim_frostbolt_volley::FilterTargets
-- (boss_thorim.cpp:1734-1767): OnObjectAreaTargetSelect EFFECT_ALL
-- TARGET_UNIT_SRC_AREA_ENEMY -> keep players and pets, drop every
-- other target (creatures return !IsPet(), anything else is dropped);
-- the identical lambda as kologarn stone_shout. No Lua IsPet method
-- exists; the in-tree approximation is a nonzero OwnerGUID
-- (hyjal_trash / spell_hunter precedent, lua_creature_events.go:4253)
-- — it over-counts totems/guardians, which C++ drops. No effectIndex
-- gate: C++ registers EFFECT_ALL (rotface precedent). Bound 62580 /
-- 62604 (boss enum :70 + comment :1733; boss casts 62580 at :1661-1662,
-- DoCastAOE SPELL_FROSTBOLT_VOLLEY; 62604 is the 25-man twin per the
-- paired comment). DORMANT until Spell.dbc ships: the volley casts
-- are DBC-gated.
local function frostboltVolleyFilter(event, spell, effectIndex, targets)
    for _, target in ipairs(targets:GetTargets()) do
        if target == nil then
            -- nothing to keep; the list holds resolved objects only
        elseif target:GetObjectType() == "Player" then
            -- keep
        elseif target:GetObjectType() == "Creature" and target:GetOwnerGUID() ~= 0 then
            -- pet: keep
        else
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(SPELL_FROSTBOLT_VOLLEY, 7, frostboltVolleyFilter)
RegisterSpellEvent(SPELL_FROSTBOLT_VOLLEY_HEROIC, 7, frostboltVolleyFilter)

-- C++ spell_thorim_runic_smash::HandleScript
-- (boss_thorim.cpp:2039-2050): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_TRIGGER_SPELL -> PreventHitDefaultEffect, then every
-- golem-hand bunny of the entry matching the cast side within 150 yd
-- schedules its own RunicSmashExplosionEvent at dist * 30 ms, which
-- casts SPELL_RUNIC_SMASH 62465 on itself, non-triggered (C++ passes
-- no triggered arg). Bound 62058/62057 (boss enum :80-81; the colossus
-- casts RAND(left, right) at :1480-1483). The delayed casts ride the
-- file's schedule() helper keyed by the caster GUID with a per-bunny
-- key, so colossus Reset/leave-combat/death cancels them (cancelTimers
-- below); the 6s re-cast outruns the 4.5s max delay, so no pending
-- explosion is ever clobbered. DORMANT until Spell.dbc ships: the
-- 62057/62058 casts are DBC-gated.
local function runicSmashHandleScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local bunnyEntry = NPC_GOLEM_RIGHT_HAND_BUNNY
    if spell:GetEntry() == SPELL_RUNIC_SMASH_LEFT then
        bunnyEntry = NPC_GOLEM_LEFT_HAND_BUNNY
    end
    local casterGuid = caster:GetGUIDLow()
    for _, trigger in ipairs(caster:GetCreaturesInGrid(bunnyEntry, 150.0)) do
        local delay = math.max(1, math.floor(caster:GetExactDistance(trigger) * 30.0))
        schedule(casterGuid, "runicSmashExplosion" .. trigger:GetGUIDLow(), delay, function()
            trigger:CastSpell(trigger, SPELL_RUNIC_SMASH, false)
        end)
    end
end
RegisterSpellEvent(SPELL_RUNIC_SMASH_LEFT, 5, runicSmashHandleScript)
RegisterSpellEvent(SPELL_RUNIC_SMASH_RIGHT, 5, runicSmashHandleScript)
