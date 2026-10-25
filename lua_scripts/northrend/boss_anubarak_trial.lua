-- Anub'arak (Trial of the Crusader) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_anubarak_trial.cpp
-- (boss_anubarak_trial (BossAI); npc_swarm_scarab, npc_nerubian_burrower,
-- npc_anubarak_spike, npc_frost_sphere (ScriptedAI); spell_pursuing_spikes
-- and spell_anubarak_leeching_swarm (AuraScript via RegisterSpellScript),
-- spell_impale (SpellScriptLoader); AddSC_boss_anubarak_trial at end
-- registers all via GetTrialOfTheCrusaderAI / RegisterSpellScript / new).
-- The FIRST Trial of the Crusader group in northrend_script_loader.cpp
-- order (decl 51 / call 246, immediately after
-- AddSC_trial_of_the_champion(); Trial of the Champion block now
-- CLOSED).
-- Entry: 34564 Anub'arak (trial_of_the_crusader.h NPC_ANUBARAK —
-- kalecgos pass; the GetTrialOfTheCrusaderAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for
-- "boss_anubarak_trial", "npc_swarm_scarab", "npc_nerubian_burrower",
-- "npc_anubarak_spike", "npc_frost_sphere",
-- "spell_anubarak_leeching_swarm" and "spell_impale" hits
-- boss_anubarak_trial.cpp only (loader carries only the AddSC
-- decl/call lines); "spell_pursuing_spikes" has no quoted
-- CreatureScript-style registration (RegisterSpellScript path) — hits
-- boss_anubarak_trial.cpp only; zero sql/ hits. No anubarak_trial
-- lua existed (the overnight window did not outrun this one).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken (phase-3 leeching-swarm
-- one-shot), 23 OnReset, 24 OnReachHome. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(creature, spell) =
-- DoCastSelf (phase_hunter / apothecary_hanes precedent).
-- Ported arms (C++-exact for all modeled arms):
-- boss_anubarak_trial: JustEngagedWith Talk(SAY_AGGRO 1) (event 1;
-- the BossAI::JustEngagedWith instance-bookkeeping leg has no
-- bridge — tharon_ja precedent); KilledUnit Talk(SAY_KILL_PLAYER
-- 7) player-gated (event 3, who->GetTypeId()==TYPEID_PLAYER —
-- nalorakk precedent); JustDied Talk(SAY_DEATH 8) (event 4;
-- _JustDied instance bookkeeping has no bridge — tharon_ja
-- precedent); EVENT_BERSERK — DoCastSelf(Berserk 26662), 600s,
-- phase-independent (fires once, 10min after combat start —
-- phase_hunter self-cast precedent). The berserk scheduler is
-- cancelled on 2/4 (gargolmar precedent; 23 is unregistered but the
-- berserk is a one-shot 600s timer — 2/4 cover leave-combat and death).
-- npc_swarm_scarab (34605 NPC_SCARAB — Summons enum kalecgos
-- pass): JustDied DoCast(killer, Traitor King 68186, killer-gated)
-- (event 4; maiden_of_virtue target-cast precedent). No
-- boss_ai.go entry — npc entries register in the lua file only
-- (terestian precedent: Kil'rek 17229 / fiendish imp 17267).
-- Unmodeled (no bridges — documented, not wired):
-- boss_anubarak_trial MoveInLineOfSight Talk(SAY_INTRO 0) —
-- dead arm: _intro initializes true (:176) and is only ever set
-- false inside the unreachable branch itself (:219), so the Talk
-- never fires in C++ (22:58 MoveInLOS-arm scan verified this —
-- the old "no hook on the Lua surface" note was stale; event 27
-- exists, depth_charge precedent — but wiring a Talk C++ never
-- emits would fabricate behavior).
-- boss_anubarak_trial JustReachedHome — the scarab arm is PORTED this
-- run (anubarakReachHome, event 24 — the old "summon STRAND absent"
-- claim is re-keyed): 10x SummonCreature(NPC_SCARAB 34605,
-- AnubarakLoc[1] (695.240051, 137.834824, 142.200000) + urand(0,50)-25
-- x/y offsets, o 0.0f — the 4-arg C++ overload defaults
-- TEMPSUMMON_MANUAL_DESPAWN (7), despawnMs 0) + scarab:SetFaction(
-- FACTION_PREY 31). Documented deltas: the instance->SetBossState(
-- DATA_ANUBARAK, FAIL) leg has no instance bridge; the
-- GetMotionMaster()->MoveRandom(10) leg has no motion bridge — the
-- scarabs stand at their spawn points instead of wandering.
-- boss_anubarak_trial JustSummoned NPC_BURROW 34862 leg —
-- SetDisplayId (display bridge live 2026-10-20) +
-- SetReactState(REACT_PASSIVE) (react bridge exists) +
-- Churning Ground self-cast — PORTED 2026-10-24 (anubarakJustSummoned,
-- ev19, LATENT per the anubrekhan precedent: burrows spawn only from
-- the unported EVENT_SUMMON_NERUBIAN spell-summon, so the arm fires
-- only if a future path summons via anubarak); JustSummoned
-- NPC_SPIKE 34660 leg —
-- SelectTarget(Random,0,0.0f,true) -> EngageWithTarget (random-target
-- pick IS bridged — cairne/kazzak precedent; sole blocker is
-- EngageWithTarget, no bridge) + Talk(EMOTE_SPIKE 0, target) (rides
-- the unbridged leg).
-- boss_anubarak_trial JustEngagedWith — RemoveFlag(
-- NON_ATTACKABLE | NOT_SELECTABLE) PORTED via the RemoveFlag bridge
-- (engine/world/lua_creature_events.go; the old "no flag bridge —
-- drakkari_colossus precedent" claim was stale); summons.DoAction(
-- ACTION_SCARAB_SUBMERGE) + SummonCreature(NPC_BURROW) x4 +
-- SummonCreature(NPC_FROST_SPHERE) x6 with GUID latching (summon
-- STRAND + cross-AI DoAction bridges absent).
-- boss_anubarak_trial EVENT_FREEZE_SLASH — DoCastVictim(Freeze
-- Slash 66012), 15s init, PHASE_MELEE — bridged in isolation
-- (moroes precedent) but phase-gated behind the unbridged
-- submerge machine: ungated emulation would over-cast during the
-- submerged phase (tharon_ja bar) — documented-only.
-- boss_anubarak_trial EVENT_PENETRATING_COLD — me->CastSpell(
-- nullptr, Penetrating Cold 66013, SPELLVALUE_MAX_TARGETS
-- RAID_MODE(2,5,2,5)), 20s init, PHASE_MELEE — same phase gate
-- (tharon_ja bar) — documented-only.
-- boss_anubarak_trial EVENT_SUMMON_NERUBIAN — me->CastSpell(
-- nullptr, SPELL_SUMMON_BURROWER 66332, SPELLVALUE_MAX_TARGETS
-- RAID_MODE(1,2,2,4)), 10s init / 45s re-arm, PHASE_MELEE —
-- phase-gated + summon STRAND absent — documented-only.
-- boss_anubarak_trial EVENT_NERUBIAN_SHADOW_STRIKE (heroic) —
-- summons.DoAction(ACTION_SHADOW_STRIKE) — cross-AI DoAction
-- bridge absent — documented-only.
-- boss_anubarak_trial EVENT_SUBMERGE (boss_anubarak_trial.cpp:335-347,
-- gated on !_reachedPhase3 && !HasAura(BERSERK)) — DoCast(me,
-- Submerge 65981) + DoCast(me, Clear All Debuffs 34098) +
-- SetFlag(NON_ATTACKABLE | NOT_SELECTABLE) + Talk(EMOTE_BURROWER
-- 3) + SetPhase(PHASE_SUBMERGED) + the submerged schedules — stays out:
-- no phase / summon bridges (the SetFlag leg IS bridgeable —
-- lua_creature_events.go:968 — but the phase machine and the submerged
-- schedules ride unported ground); the old "no flag bridge" wording
-- was stale — documented-only.
-- boss_anubarak_trial EVENT_PURSUING_SPIKE — DoCast(
-- Spike Call 66169) (rides the unbridged phase machine).
-- boss_anubarak_trial EVENT_SUMMON_SCARAB — burrow GUID-list
-- walk + ObjectAccessor::GetCreature + burrow->CastSpell(
-- burrow, 66340) — no ObjectAccessor / cross-AI cast bridges —
-- documented-only.
-- boss_anubarak_trial EVENT_EMERGE (boss_anubarak_trial.cpp:372-390) —
-- DoCast(Spike Tele 66170) + summons.DespawnEntry(NPC_SPIKE) +
-- RemoveAurasDueToSpell(SUBMERGE_ANUBARAK) + RemoveFlag(
-- NON_ATTACKABLE | NOT_SELECTABLE) + DoCast(me, Emerge 65982) +
-- Talk(EMOTE_EMERGE 4) + SetPhase(PHASE_MELEE) + full timer re-arm —
-- stays out: no phase / summon / despawn bridges (the RemoveFlag leg
-- IS bridgeable — lua_creature_events.go:1033 — but the phase machine
-- and the spike/summon legs ride unported ground); the old "no flag
-- bridge" wording was stale — documented-only.
-- boss_anubarak_trial EVENT_SUMMON_FROST_SPHERE — ObjectAccessor
-- sphere GUID walk + SummonCreature(NPC_FROST_SPHERE) +
-- _sphereGUID latch (no ObjectAccessor / summon bridges).
-- boss_anubarak_trial phase-3 leg (boss_anubarak_trial.cpp:420-426) —
-- PORTED (2026-10-23) via event 9 anubarakDamageTaken: HealthBelowPct(
-- 30) && IsInPhase(PHASE_MELEE) && !_reachedPhase3 -> one-shot
-- reachedPhase3 latch + DoCastAOE(Leeching Swarm 66118, non-triggered
-- self-cast) + Talk(EMOTE_LEECHING_SWARM 6) + Talk(SAY_LEECHING_SWARM
-- 5), C++ arm order. PHASE_MELEE is the port's permanent state (Reset
-- :184 sets it; the submerge/emerge phase machine is unmodeled), so the
-- phase gate reduces to the latch; the UpdateAI post-damage health
-- semantics are kept via the postDamage > 0 guard (nefarian form).
-- The spell_anubarak_leeching_swarm AuraScript periodic life-leech
-- (66240 / 66125) has no AuraScript bridge — the cast ports, the
-- scripted periodic effect is a documented deviation (gurtogg bloodboil
-- precedent). The old "no health-pct bridge (doomwalker precedent)"
-- and "PHASE_MELEE-gated (no phase bridge)" claims were stale —
-- GetHealthPct is bridged (engine/scripting/object.go:107) and the
-- default-phase gate is trivially satisfied.
-- boss_anubarak_trial.cpp:428: `if (events.IsInPhase(PHASE_MELEE))
-- DoMeleeAttackIfReady();` — the melee-only-in-phase gate stays out:
-- no attack-gating bridge (gurtogg precedent — the engine keeps
-- attacking normally), and it is moot in the port, which never leaves
-- the melee phase (submerge/emerge unmodeled).
-- boss_anubarak_trial JustDied's grid despawn of frost spheres
-- and burrowers — PORTED via the GetCreaturesInGrid bridge
-- (engine/world/lua_creature_events.go); creature:Despawn() was
-- already bridged (the old "no despawn bridge (terestian
-- precedent)" claim was stale).
-- boss_anubarak_trial Reset's grid despawn of frost spheres (150yd)
-- + RemoveFlag(NON_ATTACKABLE | NOT_SELECTABLE) — PORTED (same
-- bridges); the Reset timer arms stay phase-gated (see below).
-- npc_swarm_scarab — zero registration beyond the JustDied arm.
-- Reset: DoCast(me, Acid Mandible 65774) + DoZoneInCombat +
-- cross-AI JustSummoned — no zone-in-combat / cross-AI / instance
-- bridges; DoAction(ACTION_SCARAB_SUBMERGE): DoCast(
-- Submerge Effect 68394) + DespawnOrUnsummon(1s) — no cross-AI
-- DoAction / despawn bridges; UpdateAI determination timer:
-- DoCast(me, Determination 66092), urand(5s,60s) init, urand(
-- 10s,60s) repeat — self-cast bridged in isolation but gated on
-- _instance->GetBossState(DATA_ANUBARAK) == IN_PROGRESS with a
-- DisappearAndDie payload (instance-script model absent): ungated
-- emulation would fire outside the encounter window (jedoga
-- over-cast bar) — documented-only; melee engine-driven.
-- npc_nerubian_burrower (34607) — zero registration. Reset
-- self-casts (Expose Weakness 67720, Spider Frenzy 66128,
-- Awakened 66311) + DoZoneInCombat + cross-AI JustSummoned ride
-- the unbridged summon gate; DoAction(ACTION_SHADOW_STRIKE):
-- SelectTarget(Random,0) -> DoCast(Shadow Strike 66134) — no
-- DoAction / random-target bridges; UpdateAI submerge timer
-- (boss_anubarak_trial.cpp:580-596): (_submergeTimer <= diff) &&
-- HealthBelowPct(80) -> the SUBMERGE_EFFECT aura branch (
-- RemoveAurasDueToSpell + DoCast(me, EMERGE 65982) + DoCast(me,
-- AWAKENED 66311) + RemoveFlag(NOT_SELECTABLE)) or the
-- !PERMAFROST_HELPER branch (DoCast(me, SUBMERGE_EFFECT 65981) +
-- SetFlag(NOT_SELECTABLE) + DoCast(me, PERSISTENT_DIRT, true)) —
-- stays documented-only: instance-gated (_instance->GetBossState(
-- DATA_ANUBARAK) != IN_PROGRESS -> DisappearAndDie, no instance
-- bridge), the phase machine has no bridge, and the burrower summon
-- context has zero registration. The flag/aura legs are bridgeable in
-- isolation (SetFlag/RemoveFlag live, lua_creature_events.go:968+;
-- creature:HasAura bridged, lua_objects.go:228) — the old "no
-- health-pct bridge (doomwalker precedent)" claim was stale
-- (GetHealthPct, object.go:107); the instance/phase/summon gates block.
-- npc_anubarak_spike (34660) — zero registration (entry
-- verifiable from the Summons enum, but zero bridgeable arms —
-- joins the entry-verifiable-but-bridge-blocked queue): DamageTaken
-- uiDamage = 0 (event 9 IS bridged — the old "no DamageTaken hook
-- (npc_unkor_the_ruthless precedent)" claim is stale; the arm stays
-- out behind the unbridged motion/threat machine); the whole chase
-- machine — DoCast(target, Mark
-- 67574) + SetSpeedRate + threat reset + AddThreat + AttackStart
-- (no motion / threat bridges); the phase-speed legs (DoCastSelf
-- Spike Speed 65920/65922/65923 + Spike Trail 65921, 7s phase
-- machine) ride the unbridged motion machine; MoveInLineOfSight
-- frost-sphere latching (no hook + no motion bridges).
-- npc_frost_sphere (34606) — Reset legs PORTED in anubarakJustSummoned
-- (ev19, burrow precedent, LATENT: boss sphere summons stay out on
-- GUID-latch/summon-STRAND grounds): SetReactState(REACT_PASSIVE) +
-- DoCast(Frost Sphere 67539) + SetDisplayId, C++ order; MoveRandom
-- stays out (no motion-master bridge);
-- DamageTaken + MovementInform(POINT_FALL_GROUND)
-- ground-impact machine (event 9 DamageTaken IS bridged — the old
-- "no DamageTaken hook" claim is stale; no MovementInform hook;
-- no flag / aura / motion bridges — display bridge live
-- 2026-10-20 but the hook machine itself is unported).
-- spell_pursuing_spikes (AuraScript 65920/65922/65923) —
-- PeriodicTick permafrost-caster check -> Spike Fail 66181 +
-- DisappearAndDie — no AuraScript binding bridge (razelikh
-- precedent) — documented-only.
-- spell_impale (SpellScript) — HandleDamageCalc permafrost
-- damage suppression — no SpellScript binding bridge (razelikh
-- precedent) — documented-only.
-- spell_anubarak_leeching_swarm (AuraScript 66118) —
-- HandleEffectPeriodic life-leeched damage/heal fan-out (66240 /
-- 66125) — no AuraScript binding bridge — documented-only.
-- C++ "Known Issues" note carried: none in this file (the boss
-- works around the scarab-summon spell with a comment block
-- noting missing sniff info).

local ENTRY_ANUBARAK = 34564
local ENTRY_SWARM_SCARAB = 34605
local ENTRY_SPIKE = 34660
local NPC_FROST_SPHERE = 34606
local NPC_BURROWER = 34607
local NPC_BURROW = 34862 -- boss_anubarak_trial.cpp:52
local SPELL_CHURNING_GROUND = 66969 -- boss_anubarak_trial.cpp:74
local SPELL_FROST_SPHERE = 67539 -- boss_anubarak_trial.cpp:95
local FACTION_PREY = 31 -- SharedDefines.h
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000
local UNIT_FLAG_INTRO_MASK = UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE

local SAY_AGGRO = 1
local SAY_LEECHING_SWARM = 5 -- boss_anubarak_trial.cpp:41
local EMOTE_LEECHING_SWARM = 6 -- boss_anubarak_trial.cpp:42
local SAY_KILL_PLAYER = 7
local SAY_DEATH = 8

local SPELL_BERSERK = 26662
local SPELL_LEECHING_SWARM = 66118 -- boss_anubarak_trial.cpp:62
local SPELL_TRAITOR_KING = 68186

local timers = {}
local anubarakState = {}

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

-- C++ EVENT_BERSERK: DoCastSelf(Berserk 26662), 600s after combat
-- start, phase-independent (never rescheduled or cancelled in
-- C++; fires once).
local function anubarakEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    -- C++ JustEngagedWith (boss_anubarak_trial.cpp:273-287):
    -- RemoveFlag(UNIT_FIELD_FLAGS, NON_ATTACKABLE | NOT_SELECTABLE)
    -- ported via the RemoveFlag bridge; the summon / DoAction legs
    -- stay out (no summon / cross-AI bridges).
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_INTRO_MASK)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 600000, function()
        creature:CastSpell(creature, SPELL_BERSERK)
    end)
end

local function anubarakLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    anubarakState[guid] = nil
end

local function anubarakTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function anubarakDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    -- C++ JustDied (boss_anubarak_trial.cpp:238-246): despawn
    -- leftover frost spheres and burrowers within 150yd via the
    -- GetCreaturesInGrid bridge.
    for _, add in ipairs(creature:GetCreaturesInGrid(NPC_FROST_SPHERE, 150)) do
        add:Despawn()
    end
    for _, add in ipairs(creature:GetCreaturesInGrid(NPC_BURROWER, 150)) do
        add:Despawn()
    end
end

-- C++ boss_anubarak_trialAI::Reset (boss_anubarak_trial.cpp:179-206):
-- RemoveFlag(UNIT_FIELD_FLAGS, NON_ATTACKABLE | NOT_SELECTABLE)
-- + grid despawn of leftover frost spheres (150yd) ported; the
-- phase-gated timer arms (FREEZE_SLASH / PENETRATING_COLD /
-- SUMMON_NERUBIAN / SUBMERGE, PHASE_MELEE) stay out behind the
-- unported submerge phase machine (tharon_ja bar).
local function anubarakReset(event, creature)
    cancelTimers(creature:GetGUID())
    anubarakState[creature:GetGUID()] = nil
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_INTRO_MASK)
    for _, sphere in ipairs(creature:GetCreaturesInGrid(NPC_FROST_SPHERE, 150)) do
        sphere:Despawn()
    end
end

-- C++ boss_anubarak_trialAI::UpdateAI phase-3 leg
-- (boss_anubarak_trial.cpp:420-426): HealthBelowPct(30) &&
-- IsInPhase(PHASE_MELEE) && !_reachedPhase3 -> one-shot latch +
-- DoCastAOE(Leeching Swarm 66118, non-triggered — C++ DoCast default)
-- + Talk(EMOTE_LEECHING_SWARM 6) + Talk(SAY_LEECHING_SWARM 5), C++
-- arm order kept. PHASE_MELEE is the port's permanent state (Reset
-- :184 sets it; the submerge/emerge phase machine is unmodeled), so
-- the phase gate reduces to the one-shot latch. UpdateAI health arms
-- are post-damage — the nefarian onDamageTaken form: fire only when
-- post-damage health is alive (>0) and HealthBelowPct(30) on it
-- (C++ integer form: health < CountPctFromMaxHealth(30) — strict <,
-- Unit.h:872; boundary-exact to 1hp modulo the pct floor).
-- spell_anubarak_leeching_swarm AuraScript periodic life-leech has no
-- AuraScript bridge — the cast ports, the scripted periodic effect is
-- a documented deviation (gurtogg bloodboil precedent).
local function anubarakDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = anubarakState[guid]
    if st and st.reachedPhase3 then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = creature:GetHealth() - damage
    if postDamage > 0 and postDamage * 100 < maxHealth * 30 then
        anubarakState[guid] = { reachedPhase3 = true }
        creature:CastSpell(creature, SPELL_LEECHING_SWARM)
        creature:Talk(EMOTE_LEECHING_SWARM)
        creature:Talk(SAY_LEECHING_SWARM)
    end
end

-- C++ boss_anubarak_trialAI::JustReachedHome
-- (boss_anubarak_trial.cpp:225-233): instance->SetBossState(
-- DATA_ANUBARAK, FAIL) has no instance bridge (documented above);
-- the scarab arm is ported: 10x SummonCreature(NPC_SCARAB 34605,
-- AnubarakLoc[1] x/y + urand(0,50)-25, o 0.0f, TEMPSUMMON_MANUAL_DESPAWN
-- (7), despawnMs 0) + scarab:SetFaction(FACTION_PREY). The
-- GetMotionMaster()->MoveRandom(10) leg has no motion bridge
-- (documented delta above).
local function anubarakReachHome(event, creature)
    for _ = 1, 10 do
        local scarab = creature:SummonCreature(ENTRY_SWARM_SCARAB,
            695.240051 + math.random(0, 50) - 25,
            137.834824 + math.random(0, 50) - 25,
            142.200000, 0, 7, 0)
        if scarab then
            scarab:SetFaction(FACTION_PREY)
        end
    end
end

-- C++ JustSummoned NPC_BURROW (boss_anubarak_trial.cpp:253-257):
-- SetReactState(REACT_PASSIVE) + CastSpell(summon, SPELL_CHURNING_GROUND
-- 66969, non-triggered) + SetDisplayId(Modelid2) in C++ order; the
-- _burrowGUID push is summons bookkeeping with no Lua twin. LATENT:
-- burrows spawn only from the unported EVENT_SUMMON_NERUBIAN spell-summon
-- (anubrekhan latent precedent). The spike case stays out (random-target
-- EngageWithTarget + Talk unbridged).
local function anubarakJustSummoned(_, creature, summoned)
    if summoned:GetEntry() == NPC_BURROW then
        summoned:SetReactState(0) -- REACT_PASSIVE
        summoned:CastSpell(summoned, SPELL_CHURNING_GROUND)
        summoned:SetDisplayId(summoned:GetTemplateModelId(2))
    end
    if summoned:GetEntry() == NPC_FROST_SPHERE then
        summoned:SetReactState(0) -- REACT_PASSIVE
        summoned:CastSpell(summoned, SPELL_FROST_SPHERE)
        summoned:SetDisplayId(summoned:GetTemplateModelId(2))
    end
end

RegisterCreatureEvent(ENTRY_ANUBARAK, 23, anubarakReset)
RegisterCreatureEvent(ENTRY_ANUBARAK, 24, anubarakReachHome)
RegisterCreatureEvent(ENTRY_ANUBARAK, 9, anubarakDamageTaken)
RegisterCreatureEvent(ENTRY_ANUBARAK, 19, anubarakJustSummoned)

RegisterCreatureEvent(ENTRY_ANUBARAK, 1, anubarakEnterCombat)
RegisterCreatureEvent(ENTRY_ANUBARAK, 2, anubarakLeaveCombat)
RegisterCreatureEvent(ENTRY_ANUBARAK, 3, anubarakTargetDied)
RegisterCreatureEvent(ENTRY_ANUBARAK, 4, anubarakDied)

-- C++ npc_swarm_scarab JustDied: if (killer) DoCast(killer,
-- SPELL_TRAITOR_KING); no other gate — C++-exact.
local function scarabDied(event, creature, killer)
    if killer then
        creature:CastSpell(killer, SPELL_TRAITOR_KING)
    end
end

RegisterCreatureEvent(ENTRY_SWARM_SCARAB, 4, scarabDied)

-- C++ npc_anubarak_spike::DamageTaken (boss_anubarak_trial.cpp:729-732):
-- uiDamage = 0 unconditionally — spikes are damage-immune in all phases.
-- Event 9 consumes the second return as the damage rewrite
-- (fireCreatureDamageTaken; halazzi precedent).
local function spikeDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_SPIKE, 9, spikeDamageTaken)
RegisterCreatureEvent(ENTRY_SPIKE, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- boss_anubarak_trial.cpp:734: custom UpdateAI never calls DoMeleeAttackIfReady (chase/self-cast machine in C++, never melees)
end)
