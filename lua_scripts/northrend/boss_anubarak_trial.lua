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
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
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
-- cancelled on 2/4/23 (gargolmar precedent).
-- npc_swarm_scarab (34605 NPC_SCARAB — Summons enum kalecgos
-- pass): JustDied DoCast(killer, Traitor King 68186, killer-gated)
-- (event 4; maiden_of_virtue target-cast precedent). No
-- boss_ai.go entry — npc entries register in the lua file only
-- (terestian precedent: Kil'rek 17229 / fiendish imp 17267).
-- Unmodeled (no bridges — documented, not wired):
-- boss_anubarak_trial MoveInLineOfSight Talk(SAY_INTRO 0) — no
-- MoveInLineOfSight hook on the Lua surface.
-- boss_anubarak_trial JustReachedHome — instance->SetBossState(
-- DATA_ANUBARAK, FAIL) (instance-script model absent, standing) +
-- SummonCreature(NPC_SCARAB) x10 (summon STRAND absent, standing).
-- boss_anubarak_trial JustSummoned NPC_BURROW 34862 leg —
-- SetDisplayId (no display bridge) + SetReactState(REACT_PASSIVE)
-- (no react bridge) + Churning Ground self-cast (rides the
-- unbridged summon); JustSummoned NPC_SPIKE 34660 leg —
-- SelectTarget(Random,0,0.0f,true) -> EngageWithTarget (no
-- random-target SelectTarget bridge — cairne/kazzak precedent) +
-- Talk(EMOTE_SPIKE 0, target) (rides the unbridged leg).
-- boss_anubarak_trial JustEngagedWith — RemoveFlag(
-- NON_ATTACKABLE | NOT_SELECTABLE) (no flag bridge —
-- drakkari_colossus precedent); summons.DoAction(
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
-- RAID_MODE(1,2,2,4)), 45s init, PHASE_MELEE — phase-gated +
-- summon STRAND absent — documented-only.
-- boss_anubarak_trial EVENT_NERUBIAN_SHADOW_STRIKE (heroic) —
-- summons.DoAction(ACTION_SHADOW_STRIKE) — cross-AI DoAction
-- bridge absent — documented-only.
-- boss_anubarak_trial EVENT_SUBMERGE — DoCast(me,
-- Submerge 65981) + DoCast(me, Clear All Debuffs 34098) +
-- SetFlag(NON_ATTACKABLE | NOT_SELECTABLE) + Talk(EMOTE_BURROWER
-- 3) + phase switch — no flag / phase bridges — documented-only.
-- boss_anubarak_trial EVENT_PURSUING_SPIKE — DoCast(
-- Spike Call 66169) (rides the unbridged phase machine).
-- boss_anubarak_trial EVENT_SUMMON_SCARAB — burrow GUID-list
-- walk + ObjectAccessor::GetCreature + burrow->CastSpell(
-- burrow, 66340) — no ObjectAccessor / cross-AI cast bridges —
-- documented-only.
-- boss_anubarak_trial EVENT_EMERGE — DoCast(Spike Tele 66170) +
-- summons.DespawnEntry(NPC_SPIKE) + RemoveAurasDueToSpell +
-- RemoveFlag + DoCast(me, Emerge 65982) + Talk(EMOTE_EMERGE 4) +
-- phase switch + full timer re-arm — no flag / phase / summon
-- bridges — documented-only.
-- boss_anubarak_trial EVENT_SUMMON_FROST_SPHERE — ObjectAccessor
-- sphere GUID walk + SummonCreature(NPC_FROST_SPHERE) +
-- _sphereGUID latch (no ObjectAccessor / summon bridges).
-- boss_anubarak_trial phase-3 leg — HealthBelowPct(30) (no
-- health-pct bridge — doomwalker precedent) + DoCastAOE(
-- Leeching Swarm 66118) (no DoCastAOE bridge —
-- terestian/shazzrah precedent) + Talk(EMOTE_LEECHING_SWARM 6) +
-- Talk(SAY_LEECHING_SWARM 5) (ride the unbridged leg).
-- boss_anubarak_trial JustDied's grid despawn of frost spheres
-- and burrowers — no despawn bridge (terestian precedent).
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
-- DoAction / random-target bridges; UpdateAI submerge timer:
-- HealthBelowPct(80) (no health-pct bridge) + flag / aura / permafrost
-- gates (no flag / aura bridges) — all documented-only.
-- npc_anubarak_spike (34660) — zero registration (entry
-- verifiable from the Summons enum, but zero bridgeable arms —
-- joins the entry-verifiable-but-bridge-blocked queue): DamageTaken
-- uiDamage = 0 (no DamageTaken hook — npc_unkor_the_ruthless
-- precedent); the whole chase machine — DoCast(target, Mark
-- 67574) + SetSpeedRate + threat reset + AddThreat + AttackStart
-- (no motion / threat bridges); the phase-speed legs (DoCastSelf
-- Spike Speed 65920/65922/65923 + Spike Trail 65921, 7s phase
-- machine) ride the unbridged motion machine; MoveInLineOfSight
-- frost-sphere latching (no hook + no motion bridges).
-- npc_frost_sphere (34606) — zero registration (joins the
-- entry-verifiable-but-bridge-blocked queue): Reset —
-- SetReactState(REACT_PASSIVE) + DoCast(Frost Sphere 67539) +
-- SetDisplayId + MoveRandom (no react / display / motion-master
-- bridges); DamageTaken + MovementInform(POINT_FALL_GROUND)
-- ground-impact machine (no DamageTaken / MovementInform hooks;
-- no flag / aura / display / motion bridges).
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

local SAY_AGGRO = 1
local SAY_KILL_PLAYER = 7
local SAY_DEATH = 8

local SPELL_BERSERK = 26662
local SPELL_TRAITOR_KING = 68186

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

-- C++ EVENT_BERSERK: DoCastSelf(Berserk 26662), 600s after combat
-- start, phase-independent (never rescheduled or cancelled in
-- C++; fires once).
local function anubarakEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "berserk", 600000, function()
        creature:CastSpell(creature, SPELL_BERSERK)
    end)
end

local function anubarakLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function anubarakTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function anubarakDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

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
