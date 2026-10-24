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
-- DoCast(target, SPELL_RUNIC_CHARGE 62613), non-triggered (the baltharus
-- GetPlayersInWorld candidate-selection precedent).
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
-- despawn): stale "no SpellHit bridge" retracted (2026-10-21) — ev14
-- CreatureEventOnHitBySpell is bridged; the arm stays out: its payload is
-- a cross-creature relay (instance->GetCreature(DATA_SIF) -> sif
-- Talk(SAY_SIF_DESPAWN)/DespawnOrUnsummon) with no instance/cross-AI
-- bridge. SpellHitTarget (SPELL_LIGHTNING_RELEASE 62466 -> player target
-- -> _dontStandInTheLightning = false) stays out: ev15 IS bridged
-- (caster-side; thorim casts 62466 on the pillar and the spell's
-- player hits fire thorim's ev15) but the latch's only consumer is
-- the Don't-Stand-in-the-Lightning achievement check (no achievement
-- bridge) and the triggering Lightning Release cast itself rides the
-- unported pillar machine.
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
-- DoAction legs stay out. The pre-phase ShouldSparWith / DamageTaken
-- pet-damage nullify arms (boss_thorim.cpp:1170-1178, !attacker->GetAffectingPlayer()
-- -> damage = 0) ride the same unregistered pre-phase trash entries; the
-- unit-side GetAffectingPlayer pet-owner resolution has no Lua bridge.
-- The ten spell scripts / condition_thorim_arena_leap / three achievements —
-- no SpellScript / AuraScript / ConditionScript / achievement bridges.

local ENTRY_THORIM = 32865
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_RENAME = 0x00000010
local ENTRY_RUNIC_COLOSSUS = 32872
local ENTRY_ANCIENT_RUNE_GIANT = 32873
local ENTRY_DARK_RUNE_WARBRINGER = 32877
local ENTRY_LIGHTNING_ORB = 33138 -- ulduar.h:189
local SPELL_LIGHTNING_DESTRUCTION = 62393 -- boss_thorim.cpp:45

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

-- C++ boss_thorimAI::Reset (boss_thorim.cpp:461): me->SetControlled(true,
-- UNIT_STATE_ROOT) — the balcony-phase root (lifted only by the unported
-- EVENT_JUMPDOWN). The companion Reset legs (SetReactState(REACT_PASSIVE),
-- SetDisableGravity(true), SetImmuneToPC(true), the phase machine, mini-boss
-- respawn, pre-phase-add summons) have no bridges and stay out.
local function thorimReset(event, creature)
    creature:SetRooted(true)
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

-- C++ boss_thorimAI::DamageTaken else-if (boss_thorim.cpp:940-943):
-- HealthBelowPctDamaged(1, damage) -> damage = 0 + FinishEncounter().
-- Only the SetFlag(UNIT_FLAG_RENAME) leg (:579) is ported here; the
-- faction/credit/attack-stop/Talk legs of FinishEncounter stay out.
-- The phase-1 jumpdown branch is the else-if's guard in C++, but it
-- cannot fire this arm: Thorim enters phase 1 at full health, so no
-- single hit can cross 99% of max health — the arm is effectively
-- phase-2-only. The RENAME set is idempotent, so no latch is needed
-- (repeated sub-1% hits re-zero, the same outcome as C++'s
-- friendly-faction leg, which has no bridge).
local function thorimDamageTaken(event, creature, attacker, damage)
    local postDamage = creature:GetHealth() - damage
    if postDamage < creature:GetMaxHealth() / 100 then
        -- C++ boss_thorim.cpp FinishEncounter order: AttackStop
        -- precedes the RENAME flag set.
        creature:AttackStop()
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_RENAME)
        return false, 0
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

-- C++ npc_runic_colossusAI::JustEngagedWith (boss_thorim.cpp:1444-1449):
-- DoZoneInCombat() first (PORTED 2026-10-23 — bridge live,
-- engine/world/lua_creature_events.go), then _events.Reset() and the
-- three ScheduleEvent arms. (EVENT_RUNIC_SMASH rides
-- DoAction(ACTION_ACTIVATE_RUNIC_SMASH) — no DoAction bridge.)
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

-- C++ npc_ancient_rune_giantAI::JustEngagedWith (boss_thorim.cpp:1531-1535):
-- DoZoneInCombat() first (PORTED 2026-10-23 — bridge live,
-- engine/world/lua_creature_events.go), then _events.Reset() and the
-- three ScheduleEvent arms.
local function giantEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- C++ :1532, first arm (bridge live 2026-10-23)
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
    end
end

RegisterCreatureEvent(ENTRY_THORIM, 1, thorimEnterCombat)
RegisterCreatureEvent(ENTRY_THORIM, 2, thorimLeaveCombat)
RegisterCreatureEvent(ENTRY_THORIM, 3, thorimTargetDied)
RegisterCreatureEvent(ENTRY_THORIM, 4, thorimDied)
RegisterCreatureEvent(ENTRY_THORIM, 9, thorimDamageTaken)
RegisterCreatureEvent(ENTRY_THORIM, 23, thorimReset)
RegisterCreatureEvent(ENTRY_THORIM, 19, thorimJustSummoned)

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
