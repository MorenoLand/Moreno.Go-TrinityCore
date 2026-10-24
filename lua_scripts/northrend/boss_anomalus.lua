-- Anomalus (The Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Nexus/boss_anomalus.cpp
-- (boss_anomalus (CreatureScript) via GetNexusAI<boss_anomalusAI>
-- (ScriptedAI, not BossAI — no instance-state BossAI plumbing of its
-- own, instance access via GetInstanceScript/SetBossState(DATA_
-- ANOMALUS)); npc_chaotic_rift (CreatureScript) via GetNexusAI<
-- npc_chaotic_riftAI> (ScriptedAI); achievement_chaos_theory
-- (AchievementCriteriaScript); AddSC_boss_anomalus() at end registers
-- all three; loader decl 79 / call 274 per northrend_script_loader.cpp
-- — the THIRD group of the "// The Nexus: Nexus" block in
-- AddNorthrendScripts(), immediately after AddSC_boss_magus_telestra()
-- (decl 78 / call 273); the call after it is AddSC_boss_ormorok()
-- (decl 80 / call 275) — loader order confirmed this run; the
-- checkpoint sequence (magus_telestra -> anomalus) is followed).
-- Entry: 26763 Anomalus (nexus.h NPC_ANOMALUS, line 43;
-- instance_nexus.cpp OnCreatureCreate binds case NPC_ANOMALUS (line
-- 50) — entry-verifiable, registration proceeds (the nexus_commanders
-- kalecgos precedent); the CreatureScript ScriptName binding is
-- DB-side as usual. NPC_CHAOTIC_RIFT (26918) / NPC_CRAZED_MANA_WRAITH
-- (26746) exist only in the .cpp's local Adds enum — local-enum-only
-- like gothik's minions — entry-unverifiable, no registration, no lua
-- for npc_chaotic_rift.
-- Sole-source verified: whole-server-tree grep for "boss_anomalus"
-- hits boss_anomalus.cpp + the loader decl/call lines only; grep for
-- "npc_chaotic_rift" / "achievement_chaos_theory" hits
-- boss_anomalus.cpp only (registered from inside
-- AddSC_boss_anomalus, no separate loader lines); zero sql/ hits. No
-- anomalus lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent (module-local GUID-keyed
-- timers table, cancelled on 2/4/23 — yogg_saron / svala / archavon
-- precedent); melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) (event 1; the instance->
-- SetBossState(DATA_ANOMALUS, IN_PROGRESS) leg has no instance bridge
-- — tharon_ja precedent).
-- JustDied — Talk(SAY_DEATH 1) (event 4; the instance->
-- SetBossState(DATA_ANOMALUS, DONE) leg has no instance bridge —
-- tharon_ja precedent).
-- uiSparkTimer — randomPlayer(target) -> CastSpell(target, 47751),
-- 5s init / 5s unconditional re-arm, non-triggered; scheduled on
-- event 1, cancelled on 2/4/23 (see sparkTick comment for the C++
-- mapping and the shield-phase caveat).
-- Unmodeled (no bridges — documented, not wired):
-- Reset — Initialize() (Phase/timers/GUID/chaosTheory reset) +
-- instance->SetBossState(DATA_ANOMALUS, NOT_STARTED) — no instance
-- bridge.
-- GetData(DATA_CHAOS_THEORY) — chaosTheory cross-AI data leg consumed
-- only by achievement_chaos_theory's OnCheck — no GetData bridge and
-- no achievement bridge.
-- SummonedCreatureDies — PORTED (event 21): chaosTheory=false when a
-- dead summon has entry NPC_CHAOTIC_RIFT — new engine fire site in
-- fireCreatureDied (summoner OwnerGUID link); the chaosTheory latch is
-- write-only in Lua (its only C++ reader is achievement_chaos_theory's
-- GetData(DATA_CHAOS_THEORY), no achievement/GetData bridges).
-- UpdateAI — the home-position distance >60.0f EnterEvadeMode hack —
-- no position / EnterEvadeMode bridges; the SPELL_RIFT_SHIELD 47748
-- HasAura + uiChaoticRiftGUID ObjectAccessor::GetCreature + Rift->
-- isDead() -> RemoveAurasDueToSpell leg — no GUID / instance bridges
-- (the HasAura leg rides the unbridged machine; creature HasAura IS
-- bridged — engine/world/lua_objects.go:228); the Phase 0 +
-- HealthBelowPct(50) -> Phase=1 + Talk(SAY_
-- SHIELD 3) + DoCast(me, SPELL_RIFT_SHIELD) + SummonCreature(NPC_
-- CHAOTIC_RIFT 26918, RiftLocation[urand(0,5)], TEMPSUMMON_TIMED_
-- DESPAWN_OUT_OF_COMBAT 1s) + Rift AttackStart(SelectTarget(Random,0))
-- + Talk(SAY_RIFT 2) machine stays out: the summon bridge is live but
-- 26918 is entry-unverifiable local-enum-only (gothik-minions
-- precedent — no template row, the summon would return nil), the
-- rift's AttackStart has no bridge, and the rift AI (burst + wraith
-- summons) has no bearer (herod precedent).
-- Ported: uiSparkTimer — SelectTarget(Random, 0) -> DoCast(SPELL_SPARK
-- 47751), 5s init, 5s repeat, non-triggered — via randomPlayer +
-- CreateLuaEvent (keleseth / thorim / archavon GetPlayersInWorld
-- precedent); C++ fires 47751 unconditionally (H_SPELL_SPARK never
-- referenced at :182), so no IsHeroic gating needed.
-- npc_chaotic_rift — entry-unverifiable (26918, local-enum-only) — no
-- registration, no lua; its arms are documented here only:
-- SetCombatMovement(false); Reset — SetDisplayId(Modelid2) +
-- DoCast(me, SPELL_ARCANEFORM 48019) — the display leg IS bridgeable
-- (GetTemplateModelId(2)) but the rift entry is unverifiable, so no
-- registration; the shield-gated casts ride the instance gate out; UpdateAI uiChaoticEnergyBurstTimer —
-- SelectTarget(Random, 0) -> DoCast(SPELL_CHAOTIC_ENERGY_BURST 47688 /
-- charged 47737 when Anomalus via instance->GetGuidData(DATA_ANOMALUS)
-- HasAura(SPELL_RIFT_SHIELD)), 1s init, 1s repeat — no instance /
-- cross-creature / random-target bridges; UpdateAI
-- uiSummonCrazedManaWraithTimer — SummonCreature(NPC_CRAZED_MANA_
-- WRAITH 26746 local-enum-only entry-unverifiable) + AttackStart(
-- SelectTarget(Random, 0)) + 5s repeat when Anomalus shielded else 10s
-- — no summon / instance / random-target bridges.
-- achievement_chaos_theory — OnCheck: Anomalus->AI()->GetData(DATA_
-- CHAOS_THEORY) != 0 — no achievement bridge (kelthuzad/thaddius
-- precedent) + the GetData leg is cross-AI — joins the
-- no-achievement-bridge queue.

local ENTRY_ANOMALUS = 26763
local NPC_CHAOTIC_RIFT = 26918 -- boss_anomalus.cpp:38

local SAY_AGGRO = 0
local SAY_DEATH = 1

local SPELL_SPARK = 47751 -- C++ :182 fires SPELL_SPARK unconditionally
-- (H_SPELL_SPARK 57062 is never referenced in the fire path); no
-- IsHeroic gating needed (creature:IsHeroic() IS bridged —
-- engine/world/lua_creature_events.go:260, blood_prince_council.lua
-- precedent — the keristrasza-era "no IsHeroic bridge" claims are
-- stale).

local timers = {}
local chaos = {} -- chaosTheory per-GUID latch: C++ Initialize() sets it
-- true (Reset twin below); SummonedCreatureDies clears it when a dead
-- summon is NPC_CHAOTIC_RIFT. Write-only in Lua — its only C++ reader
-- is achievement_chaos_theory's GetData(DATA_CHAOS_THEORY), and the
-- achievement + GetData legs have no bridges (see header).

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0) — alive players on
-- the same map/instance, no distance filter (the keleseth / thorim /
-- archavon GetPlayersInWorld precedent); only threat-listed units
-- qualify in practice.
local function randomPlayer(creature)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ uiSparkTimer: Initialize() seeds 5000; UpdateAI decrements only
-- post-UpdateVictim, so event-1 scheduling is exact. C++ :fire:
-- SelectTarget(Random, 0) -> DoCast(SPELL_SPARK), 2-arg =
-- non-triggered; re-arm 5000 unconditional. The shield-phase early
-- return (HasAura(SPELL_RIFT_SHIELD) -> return before the spark block)
-- is vacuous here — the phase-1 machine that applies the shield is
-- unmodeled, so the boss never carries the aura. Declared before the
-- event-1 handler so the closure resolves the chunk-local (algalon
-- nil-call bomb precedent — later-declared locals are NOT in scope).
local function sparkTick(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SPARK)
    end
    schedule(guid, "spark", 5000, function()
        sparkTick(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) (the instance->SetBossState
-- (DATA_ANOMALUS, IN_PROGRESS) leg has no instance bridge — tharon_ja
-- precedent).
local function anomalusEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "spark", 5000, function()
        sparkTick(creature, guid)
    end)
end

local function anomalusLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    chaos[creature:GetGUID()] = nil
end

-- C++ JustDied: Talk(SAY_DEATH) (the instance->SetBossState(DATA_
-- ANOMALUS, DONE) leg has no instance bridge — tharon_ja precedent).
local function anomalusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    cancelTimers(creature:GetGUID())
    chaos[creature:GetGUID()] = nil
end

local function anomalusReset(event, creature)
    cancelTimers(creature:GetGUID())
    chaos[creature:GetGUID()] = true -- C++ Initialize(): chaosTheory = true
end

-- C++ SummonedCreatureDies: chaosTheory = false when the dead summon's
-- entry is NPC_CHAOTIC_RIFT (boss_anomalus.cpp:130-134). Fired by the
-- new engine event-21 dispatch (fireCreatureDied -> summoner OwnerGUID
-- link); args are (event, creature, summon, killer) per Eluna.
local function anomalusSummonedDied(event, creature, summon, killer)
    if summon:GetEntry() == NPC_CHAOTIC_RIFT then
        chaos[creature:GetGUID()] = false
    end
end

RegisterCreatureEvent(ENTRY_ANOMALUS, 1, anomalusEnterCombat)
RegisterCreatureEvent(ENTRY_ANOMALUS, 2, anomalusLeaveCombat)
RegisterCreatureEvent(ENTRY_ANOMALUS, 4, anomalusDied)
RegisterCreatureEvent(ENTRY_ANOMALUS, 21, anomalusSummonedDied)
RegisterCreatureEvent(ENTRY_ANOMALUS, 23, anomalusReset)
