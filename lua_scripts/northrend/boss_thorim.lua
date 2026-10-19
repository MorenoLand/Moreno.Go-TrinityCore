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
-- npc_runic_colossusAI JustEngagedWith (lines 1447-1449) — schedules the
-- three arms below (the DoZoneInCombat leg has no bridge).
-- npc_runic_colossusAI EVENT_RUNIC_BARRIER (enum line 117; arm lines
-- 1463-1466, Repeat(35s,45s)) — Talk(EMOTE_RUNIC_BARRIER 0) +
-- DoCastAOE(SPELL_RUNIC_BARRIER 62338), non-triggered.
-- npc_runic_colossusAI EVENT_SMASH (enum line 118; arm lines 1468-1470,
-- Repeat(15s,18s)) — DoCastAOE(SPELL_SMASH 62339), non-triggered.
-- npc_runic_colossusAI EVENT_RUNIC_CHARGE (enum line 119; arm lines
-- 1472-1479, Repeat(20s)) — random player 8-40y,
-- DoCast(target, SPELL_RUNIC_CHARGE 62613), non-triggered (the baltharus
-- GetPlayersInWorld candidate-selection precedent).
-- npc_ancient_rune_giantAI JustEngagedWith (lines 1534-1536).
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
-- DoAction(ACTION_ACTIVATE_RUNIC_SMASH) leg on the runic colossus has
-- no DoAction / instance bridge.
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
-- boss_thorimAI EVENT_BERSERK (lines 750+) — phase-conditional Talk(SAY_WIPE
-- 6 / SAY_BERSERK 5) + triggered casts; DoAction(ACTION_BERSERK)-gated
-- reschedule; no DoAction / berserk bridge.
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
-- despawn) / SpellHitTarget legs — no SpellHit / cross-creature bridges.
-- npc_runic_colossusAI EVENT_RUNIC_SMASH (lines 1480-1483) —
-- DoAction(ACTION_ACTIVATE_RUNIC_SMASH)-gated; no DoAction bridge.
-- npc_runic_colossusAI / npc_ancient_rune_giantAI JustDied — GO door legs
-- (DATA_RUNIC_DOOR / DATA_STONE_DOOR) + cross-creature thorim Talk(SAY_SPECIAL
-- 2); no GO / cross-creature-Talk bridges.
-- npc_sifAI (entry 33196) — all events gated by DoAction(ACTION_START_HARD_MODE);
-- no DoAction bridge; not registered.
-- npc_thorim_pre_phaseAI / npc_thorim_arena_phaseAI — per-entry trash-ability
-- tables (separate trash creature entries, not the boss file scope); the
-- DARK_RUNE_WARBRINGER AURA_OF_CELERITY engage arm is ported (see PORTED
-- ARMS), the per-entry Reset ability-schedule tables and the arena-phase
-- DoAction legs stay out.
-- The ten spell scripts / condition_thorim_arena_leap / three achievements —
-- no SpellScript / AuraScript / ConditionScript / achievement bridges.

local ENTRY_THORIM = 32865
local ENTRY_RUNIC_COLOSSUS = 32872
local ENTRY_ANCIENT_RUNE_GIANT = 32873
local ENTRY_DARK_RUNE_WARBRINGER = 32877

local SAY_AGGRO_1 = 0
local SAY_SLAY = 4

local SPELL_SHEATH_OF_LIGHTNING = 62276

local EMOTE_RUNIC_BARRIER = 0
local SPELL_RUNIC_BARRIER = 62338
local SPELL_SMASH = 62339
local SPELL_RUNIC_CHARGE = 62613

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

-- C++ SelectTarget(Random, 0, range, playerOnly) legs (baltharus precedent).
local function randomPlayerInRange(creature, minDist, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            local dist = creature:GetDistance(plr)
            if dist >= minDist and dist <= maxDist then
                candidates[#candidates + 1] = plr
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
    creature:Talk(SAY_AGGRO_1)
    creature:CastSpell(creature, SPELL_SHEATH_OF_LIGHTNING)
    cancelTimers(creature:GetGUID())
end

local function thorimLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function thorimReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ FinishEncounter is a 1%-hp pseudo-death, not a hook: C++ never calls
-- JustDied for Thorim, so no Talk is registered on event 4 — timers are
-- only cancelled.
local function thorimDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_thorimAI::KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function thorimTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ EVENT_RUNIC_BARRIER: Talk(EMOTE_RUNIC_BARRIER 0) +
-- DoCastAOE(SPELL_RUNIC_BARRIER 62338) (non-triggered), 12-15s init,
-- Repeat(35s, 45s).
local function colossusRunicBarrierTick(creature, guid)
    creature:Talk(EMOTE_RUNIC_BARRIER)
    creature:CastSpell(creature, SPELL_RUNIC_BARRIER)
    schedule(guid, "runicbarrier", {35000, 45000}, function()
        colossusRunicBarrierTick(creature, guid)
    end)
end

-- C++ EVENT_SMASH: DoCastAOE(SPELL_SMASH 62339) (non-triggered),
-- 15-18s init, Repeat(15s, 18s).
local function colossusSmashTick(creature, guid)
    creature:CastSpell(creature, SPELL_SMASH)
    schedule(guid, "smash", {15000, 18000}, function()
        colossusSmashTick(creature, guid)
    end)
end

-- C++ EVENT_RUNIC_CHARGE: random player 8-40y,
-- DoCast(target, SPELL_RUNIC_CHARGE 62613) (non-triggered), 20-24s init,
-- Repeat(20s).
local function colossusRunicChargeTick(creature, guid)
    local target = randomPlayerInRange(creature, 8, 40)
    if target then
        creature:CastSpell(target, SPELL_RUNIC_CHARGE)
    end
    schedule(guid, "runiccharge", 20000, function()
        colossusRunicChargeTick(creature, guid)
    end)
end

-- C++ npc_runic_colossusAI::JustEngagedWith: DoZoneInCombat();
-- _events.Reset(); ScheduleEvent(EVENT_RUNIC_BARRIER, 12s, 15s),
-- EVENT_SMASH (15s, 18s), EVENT_RUNIC_CHARGE (20s, 24s).
-- (The DoZoneInCombat leg has no bridge; EVENT_RUNIC_SMASH rides
-- DoAction(ACTION_ACTIVATE_RUNIC_SMASH) — no DoAction bridge.)
local function colossusEnterCombat(event, creature, target)
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

local function colossusReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function colossusDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ EVENT_STOMP: DoCastAOE(SPELL_STOMP 62411) (non-triggered),
-- 10-12s init, Repeat(10s, 12s).
local function giantStompTick(creature, guid)
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", {10000, 12000}, function()
        giantStompTick(creature, guid)
    end)
end

-- C++ EVENT_RUNE_DETONATION: random player target 60y,
-- DoCast(target, SPELL_RUNE_DETONATION 62526) (non-triggered), 25s init,
-- Repeat(10s, 12s).
local function giantRuneDetonationTick(creature, guid)
    local target = randomPlayerInRange(creature, 0, 60)
    if target then
        creature:CastSpell(target, SPELL_RUNE_DETONATION)
    end
    schedule(guid, "runedetonation", {10000, 12000}, function()
        giantRuneDetonationTick(creature, guid)
    end)
end

-- C++ npc_ancient_rune_giantAI::JustEngagedWith: DoZoneInCombat();
-- _events.Reset(); ScheduleEvent(EVENT_RUNIC_FORTIFICATION, 1ms),
-- EVENT_STOMP (10s, 12s), EVENT_RUNE_DETONATION (25s).
local function giantEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "runicfortification", 1, function()
        creature:Talk(EMOTE_RUNIC_MIGHT)
        creature:CastSpell(creature, SPELL_RUNIC_FORTIFICATION)
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

local function giantReset(event, creature)
    cancelTimers(creature:GetGUID())
end

local function giantDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_THORIM, 1, thorimEnterCombat)
RegisterCreatureEvent(ENTRY_THORIM, 2, thorimLeaveCombat)
RegisterCreatureEvent(ENTRY_THORIM, 3, thorimTargetDied)
RegisterCreatureEvent(ENTRY_THORIM, 4, thorimDied)
RegisterCreatureEvent(ENTRY_THORIM, 23, thorimReset)

RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 1, colossusEnterCombat)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 2, colossusLeaveCombat)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 4, colossusDied)
RegisterCreatureEvent(ENTRY_RUNIC_COLOSSUS, 23, colossusReset)

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
-- companion DoAction(ACTION_ACTIVATE_RUNIC_SMASH) colossus leg has no
-- DoAction / instance bridge.
local function warbringerEnterCombat(event, creature, target)
    creature:CastSpell(creature, SPELL_AURA_OF_CELERITY)
end

RegisterCreatureEvent(ENTRY_DARK_RUNE_WARBRINGER, 1, warbringerEnterCombat)
