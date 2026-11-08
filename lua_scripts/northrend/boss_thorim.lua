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
--
-- DOCUMENTED-ONLY (no bridge; not registered):
-- boss_thorimAI EVENT_SAY_SIF_START (lines 700-702) / EVENT_START_SIF_CHANNEL
-- (lines 703-705) — cross-creature sif Talk / cast; no cross-creature bridge.
-- boss_thorimAI EVENT_SAY_AGGRO_2 (lines 621/697-699), EVENT_STORMHAMMER
-- (62042, lines 625/708-710) and EVENT_CHARGE_ORB (62016, lines 626/712-714) —
-- all ScheduleEvent(..., 0, PHASE_1): phase-1-gated, and the phase machine
-- (events.SetPhase(PHASE_2) at phase 2) has no bridge; porting on
-- combat-start timers would fabricate phase-2 casts — gothik precedent.
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
-- boss_thorimAI EVENT_BERSERK (lines 750+) — phase-conditional Talk(SAY_WIPE
-- 6 / SAY_BERSERK 5) + triggered casts; DoAction(ACTION_BERSERK)-gated
-- reschedule. ev38 IS bridged (tick 206); the sender is EnterEvadeMode,
-- which has no bridge (event 25 unused) — the arm stays out on the sender.
-- boss_thorimAI phase-2 machine (DamageTaken CanStartPhase2, lines ~925-951:
-- Talk(SAY_JUMPDOWN 3) + EVENT_JUMPDOWN / EVENT_ACTIVATE_LIGHTNING_FIELD /
-- EVENT_UNBALANCING_STRIKE (DoCastVictim 62130, 15s, Repeat(15s,20s)) /
-- EVENT_CHAIN_LIGHTNING (random target, 20s, Repeat(7s,15s)) /
-- EVENT_START_PERIODIC_CHARGE / EVENT_LIGHTNING_CHARGE (SetGUID-gated)) —
-- phase gating is the real blocker; no phase / instance-GuidData / distance /
-- cross-creature bridges. MoveJump leg in EVENT_JUMPDOWN has no bridge.
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
-- DespawnOrUnsummon cross-creature relay in the same arm stays out.
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
-- no SpellScript / AuraScript / ConditionScript / achievement bridges.

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
local SPELL_LIGHTNING_DESTRUCTION = 62393 -- boss_thorim.cpp:45

local SAY_AGGRO_1 = 0
local SAY_SPECIAL = 2
local SAY_SLAY = 4
local SAY_DEATH = 7

local SPELL_CREDIT_KILL = 64985
local FACTION_FRIENDLY = 35

local SPELL_SHEATH_OF_LIGHTNING = 62276

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

-- C++ boss_thorimAI::JustEngagedWith: Talk(SAY_AGGRO_1) + DoCast(me,
-- SPELL_SHEATH_OF_LIGHTNING 62276) 2-arg = non-triggered self-cast, both
-- ungated (line 630). The PHASE_1-grouped schedules (EVENT_SAY_AGGRO_2 /
-- EVENT_STORMHAMMER / EVENT_CHARGE_ORB, lines 621/625/626) are
-- phase-gated — gothik precedent, see header. Deviation: the
-- SHEATH_OF_LIGHTNING removal rides the unbridged phase-2 machine, so the
-- aura persists in Lua.
local function thorimEnterCombat(event, creature, target)
    creature:SetBossState(BOSS_THORIM, 1) -- BossAI::JustEngagedWith SetBossState(DATA_THORIM 8, IN_PROGRESS), base-first (boss_thorim.cpp:616; PORTED tick 259)
    creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1 (ScriptedCreature.cpp:540) -- base-first (boss_thorim.cpp:616)
    creature:DoZoneInCombat() -- BossAI::JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    creature:Talk(SAY_AGGRO_1)
    creature:CastSpell(creature, SPELL_SHEATH_OF_LIGHTNING)
    cancelTimers(creature:GetGUID())
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
    cancelTimers(creature:GetGUID())
end

-- C++ boss_thorimAI::Reset (boss_thorim.cpp:465-477): early-return if
-- _encounterFinished (GUID-keyed latch below — a respawned Thorim is a
-- new GUID, so the latch never leaks across spawns), then
-- SetReactState(REACT_PASSIVE) + SetDisableGravity(true) +
-- SetControlled(true, UNIT_STATE_ROOT) (PORTED as SetRooted; the root is
-- lifted only by the unported EVENT_JUMPDOWN). The pre-phase-add summon
-- loop (:487, TEMPSUMMON_CORPSE_TIMED_DESPAWN 3s) is PORTED below,
-- IsAlive-guarded: Go event 23 also fires on death, and C++ Reset never
-- runs on death. SetImmuneToPC(true), the phase machine and the
-- mini-boss respawn stay out (balcony-phase gating; no instance bridge
-- for the respawn).
local function thorimReset(event, creature)
    if thorimFinished[creature:GetGUID()] then
        return
    end
    creature:SetBossState(BOSS_THORIM, 0) -- _Reset() SetBossState(DATA_THORIM 8, NOT_STARTED), first after the _encounterFinished guard per C++ :465-471 (PORTED tick 259)
    creature:SetReactState(0)
    creature:SetDisableGravity(true)
    creature:SetRooted(true)
    cancelTimers(creature:GetGUID())
    thorimKilledCount[creature:GetGUID()] = nil
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
    if action ~= ACTION_INCREASE_PREADDS_COUNT then
        return
    end
    local guid = creature:GetGUID()
    local count = (thorimKilledCount[guid] or 0) + 1
    thorimKilledCount[guid] = count
    if count >= 6 then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:DoZoneInCombat()
    end
end

-- C++ FinishEncounter is a 1%-hp pseudo-death, not a hook: C++ never calls
-- JustDied for Thorim, so no Talk is registered on event 4 — timers are
-- only cancelled. The true-kill fallback below writes DONE via the base
-- BossAI::JustDied (_JustDied), which C++ would run if Thorim were killed
-- outright (boss_thorimAI overrides no JustDied).
local function thorimDied(event, creature, killer)
    creature:SetBossState(BOSS_THORIM, 3) -- true-kill fallback: BossAI::JustDied -> _JustDied() DONE (PORTED tick 259)
    cancelTimers(creature:GetGUID())
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
-- else-if's guard in C++, but it cannot fire this arm: Thorim enters
-- phase 1 at full health, so no single hit can cross 99% of max
-- health — the arm is effectively phase-2-only. Controller/pillar
-- aura clears, hard-mode Sif despawn and the outro Talks 8-13
-- (hard-mode branch timing) stay out.
local function thorimDamageTaken(event, creature, attacker, damage)
    local postDamage = creature:GetHealth() - damage
    if postDamage < math.floor(creature:GetMaxHealth() / 100) then
        if thorimFinished[creature:GetGUID()] then
            return false, 0
        end
        thorimFinished[creature:GetGUID()] = true
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
    local guid = creature:GetGUID()
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
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
end

-- C++ npc_runic_colossusAI::Reset (:1395-1406): _summons.DespawnAll (no
-- summon-tracking bridge — stays out; Go event 23 never fires on
-- leave-combat, so no evade duplication arises) + the ColossusAddLocations
-- summon loop (TEMPSUMMON_CORPSE_TIMED_DESPAWN 3s), PORTED below,
-- IsAlive-guarded like thorimReset. The runic-door instance leg stays out.
local function colossusReset(event, creature)
    cancelTimers(creature:GetGUID())
    colossusRunicActive[creature:GetGUID()] = nil
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
    cancelTimers(creature:GetGUID())
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
    local guid = creature:GetGUID()
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
    cancelTimers(creature:GetGUID())
end

-- C++ npc_ancient_rune_giantAI::Reset (:1517-1529): same shape as
-- colossusReset — _summons.DespawnAll stays out, the GiantAddLocations
-- summon loop (TEMPSUMMON_CORPSE_TIMED_DESPAWN 3s) PORTED below,
-- IsAlive-guarded. The stone-door instance leg stays out.
local function giantReset(event, creature)
    cancelTimers(creature:GetGUID())
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
    cancelTimers(creature:GetGUID())
    local stoneDoor = creature:FindNearestGameObject(GO_THORIM_STONE_DOOR, 100)
    if stoneDoor then stoneDoor:SetGoState(0) end
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
-- the same arm stays out (header). LATENT, Go gate HELD: Sif is
-- unregistered and EVENT_START_SIF_CHANNEL is unported, so 62565
-- never lands in Lua yet and the latch stays 1 (a gate now would
-- award on every kill).
local function thorimSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_TOUCH_OF_DOMINION_TRIGGERED then
        if creature:FindNearestCreature(ENTRY_SIF, 200) then
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
