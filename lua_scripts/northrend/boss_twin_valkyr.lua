-- Twin Val'kyr (Trial of the Crusader) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_twin_valkyr.cpp
-- (boss_fjola / boss_eydis (boss_twin_baseAI, BossAI);
-- npc_essence_of_twin / npc_unleashed_dark / npc_unleashed_light /
-- npc_bullet_controller (ScriptedAI); spell_bullet_controller
-- (AuraScript via RegisterSpellScript); spell_powering_up /
-- spell_valkyr_essences / spell_power_of_the_twins
-- (SpellScriptLoader); AddSC_boss_twin_valkyr at end registers all
-- via RegisterTrialOfTheCrusaderCreatureAI (=
-- RegisterCreatureAIWithFactory(ai_name, GetTrialOfTheCrusaderAI),
-- trial_of_the_crusader.h line 288) / RegisterSpellScript). The
-- FIFTH Trial of the Crusader group in
-- northrend_script_loader.cpp order (decl 55 / call 250,
-- immediately after AddSC_trial_of_the_crusader(), under the
-- "// Trial of the Crusader" marker).
-- Entry: 34497 Fjola Lightbane (trial_of_the_crusader.h
-- NPC_FJOLA_LIGHTBANE — kalecgos pass;
-- instance_trial_of_the_crusader.cpp binds NPC_FJOLA_LIGHTBANE ->
-- DATA_FJOLA_LIGHTBANE with a CircleBoundary); 34496 Eydis
-- Darkbane (NPC_EYDIS_DARKBANE — kalecgos pass; bound to
-- DATA_EYDIS_DARKBANE the same way). Add entries: 34567 Dark
-- Essence / 34568 Light Essence (trial_of_the_crusader.h
-- kalecgos pass — NOT bound in instance_trial_of_the_crusader.cpp;
-- ScriptName->entry binding DB-side, entry-unverifiable); 34743
-- Bullet Controller / 34628 Bullet Dark / 34630 Bullet Light
-- (Summons enum in the .cpp — kalecgos pass, no instance binding).
-- Sole-source verified: whole-server-tree grep for "boss_fjola",
-- "boss_eydis", "npc_essence_of_twin", "npc_unleashed_dark",
-- "npc_unleashed_light", "npc_bullet_controller",
-- "spell_powering_up", "spell_valkyr_essences" and
-- "spell_power_of_the_twins" hits boss_twin_valkyr.cpp only
-- ("spell_bullet_controller" has no quoted registration — the
-- RegisterSpellScript path — hits the file only; loader carries
-- only the AddSC_boss_twin_valkyr decl/call lines); zero sql/ hits.
-- No twin_valkyr lua existed (checked first).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell) =
-- DoCastVictim, creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf (phase_hunter /
-- apothecary_hanes precedent).
-- Ported arms (C++-exact for all modeled arms; boss_twin_baseAI is
-- shared, so both sisters carry the same arms with their own
-- spell ids — the fjola/eydis AIs differ only in the id fields set
-- in Reset):
-- JustEngagedWith Talk(SAY_AGGRO 0) (event 1) + DoCastSelf(Surge
-- 65766 Light / 65768 Dark) (event arms; the BossAI::JustEngagedWith
-- instance-bookkeeping leg, DoZoneInCombat, cross-AI sister
-- AddAura/Empathy and SetCombatPulseDelay/setActive have no bridges
-- — tharon_ja precedent); KilledUnit Talk(SAY_KILL_PLAYER 6)
-- player-gated (event 3, who->GetTypeId()==TYPEID_PLAYER — nalorakk
-- precedent); JustDied Talk(SAY_DEATH 8) (event 4; _JustDied
-- instance bookkeeping — SetBossState DONE/SPECIAL, lootable flags,
-- HandleRemoveAuras, summons.DespawnAll — has no bridge — tharon_ja
-- precedent); EVENT_TWIN_SPIKE — DoCastVictim(Light Twin Spike 66075
-- / Dark Twin Spike 66069), 20s init, 20s repeat (moroes precedent;
-- C++ UpdateAI runs combat events whenever UpdateVictim() holds —
-- the PHASE_EVENT leg only matters pre-combat for EVENT_START_MOVE).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Unmodeled (no bridges — documented, not wired):
-- Whole intro/positioning machine — JustAppeared:
-- events.SetPhase(PHASE_EVENT) + ScheduleEvent(EVENT_START_MOVE,
-- 4s) (no phase bridge — anubarak_trial precedent); EVENT_START_MOVE:
-- events.SetPhase(PHASE_COMBAT) + MoveAlongSplineChain(
-- POINT_INITIAL_MOVEMENT, SPLINE_INITIAL_MOVEMENT) (no phase/motion
-- bridges); MovementInform SPLINE_CHAIN -> SetImmuneToPC(false) +
-- SetReactState(REACT_AGGRESSIVE) + DoCloseDoorOrButton(
-- DATA_MAIN_GATE) (no MovementInform / immune / react / door
-- bridges — fjola only, "avoid call twice"); Reset:
-- SetReactState(REACT_PASSIVE) + ModifyAuraState(AURA_STATE_UNKNOWN22/
-- UNKNOWN19, true) + summons.DespawnAll (no react / aura-state /
-- despawn bridges) + DoStopTimedAchievement(EVENT_START_TWINS_FIGHT)
-- (no achievement bridge — fjola only); JustReachedHome:
-- instance->SetBossState(DATA_TWIN_VALKIRIES, FAIL) + HandleRemoveAuras
-- + summons.DespawnAll + DespawnOrUnsummon + DoUseDoorOrButton(
-- DATA_MAIN_GATE) (instance model + no aura/despawn/door bridges —
-- fjola's EnterEvadeMode adds another DoUseDoorOrButton leg).
-- EVENT_SPECIAL_ABILITY (fjola only, 45s) — the shuffled stage
-- machine: cross-AI sister->AI()->DoAction(ACTION_VORTEX /
-- ACTION_PACT) or own DoAction for light stages (no cross-AI DoAction
-- bridge); DoAction ACTION_VORTEX: Talk(EMOTE_VORTEX 3) + DoCastAOE(
-- Light Vortex 66046 / Dark Vortex 66058) (no DoCastAOE bridge —
-- terestian/shazzrah precedent); DoAction ACTION_PACT: Talk(
-- EMOTE_TWIN_PACT 4) + Talk(SAY_TWIN_PACT 5) + cross-AI sister cast
-- Power of the Twins 65879 + DoCastSelf(Light Shield 65858 / Dark
-- Shield 65874) + DoCastSelf(Light Twin Pact 65876 / Dark Twin Pact
-- 65875) — the self-cast legs are port-pattern-ready in isolation
-- (phase_hunter precedent) but fire only via cross-AI DoAction —
-- documented-only.
-- EVENT_TOUCH (heroic only): SelectTarget(Random,0,200.0f,true,true,
-- OtherEssenceSpellId) -> CastSpell(target, Light Touch 67297 / Dark
-- Touch 67282, SPELLVALUE_MAX_TARGETS 1), 10s/15s init, 10s/15s
-- repeat — no random-target / aura-filtered SelectTarget bridge
-- (cairne/kazzak precedent) — documented-only.
-- EVENT_BERSERK: DoCastSelf(Berserk 64238) + Talk(SAY_BERSERK 7) —
-- timer is IsHeroic() ? 6min : 8min: no difficulty bridge (kelidan
-- precedent) — documented-only.
-- npc_essence_of_twin (34567/34568 — entry-unverifiable): OnGossipHello
-- -> RemoveAurasDueToSpell(ESSENCE_REMOVE) + CastSpell(player,
-- ESSENCE_APPLY) — no gossip bridge — documented-only.
-- npc_unleashed_dark (34628) / npc_unleashed_light (34630) —
-- entry-unverifiable: Reset SetFlag(NON_ATTACKABLE | NOT_SELECTABLE)
-- + SetReactState(REACT_PASSIVE) + SetDisableGravity(true) +
-- SetCanFly(true) + SetCombatMovement(false) + MovePoint (no flag /
-- react / gravity / fly / combat-movement / motion bridges);
-- UpdateAI: SelectNearestPlayer(3.0f) -> DoCastAOE(Unleashed Dark
-- 65808-helper RAID_MODE(65808,67172,67173,67174) / Unleashed Light
-- 65795-helper RAID_MODE(65795,67238,67239,67240)) + MoveIdle +
-- DespawnOrUnsummon(1s), 500ms timer — no player-range scan / DoCastAOE
-- / despawn bridges; MovementInform POINT_MOTION_TYPE point 0: urand
-- 1/4 re-move else DisappearAndDie (no MovementInform / despawn
-- bridges) — documented-only.
-- npc_bullet_controller (34743 — entry-unverifiable): Reset
-- DoCastAOE(Bullet Controller Periodic 66149) — no DoCastAOE bridge;
-- UpdateAI: UpdateVictim only — documented-only.
-- spell_bullet_controller (AuraScript 66149 — also 68396 in C++ note):
-- PeriodicTick -> caster casts Summon Periodic Light 66152 +
-- Summon Periodic Dark 66153 (triggered, SPELLVALUE_MAX_TARGETS
-- urand(1,6)) — no AuraScript binding bridge (razelikh precedent) —
-- documented-only.
-- spell_powering_up (SpellScript): HandleScriptEffect — 100-stack
-- Powering Up 67590-helper check -> Empowered Dark 65724 / Empowered
-- Light 65748 by dummy-aura presence + RemoveAurasDueToSpell — no
-- SpellScript binding bridge (razelikh precedent) — documented-only.
-- spell_valkyr_essences (AuraScript): OnEffectAbsorb handler —
-- 5%-chance Surge of Speed 65828 self-cast + vortex-damage Powering Up
-- stack fan-out + floating-ball pickup Powering Up stacks — no
-- AuraScript binding bridge (razelikh precedent) — documented-only.
-- spell_power_of_the_twins (AuraScript 65879): AfterEffectApply/Remove
-- -> cross-AI sister EnableDualWield(true/false) (SetEquipmentSlots +
-- SetCanDualWield) — no AuraScript / equipment / dual-wield bridges —
-- documented-only. The four spell scripts join the
-- SpellScript/AuraScript queue.

local ENTRY_FJOLA = 34497
local ENTRY_EYDIS = 34496

local SAY_AGGRO = 0
local SAY_KILL_PLAYER = 6
local SAY_DEATH = 8

local SPELL_SURGE_FJOLA = 65766
local SPELL_SURGE_EYDIS = 65768
local SPELL_SPIKE_FJOLA = 66075
local SPELL_SPIKE_EYDIS = 66069

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

-- C++ EVENT_TWIN_SPIKE: DoCastVictim(SpikeSpellId), 20s init, 20s
-- repeat.
local function twinSpikeTick(creature, guid, spell)
    creature:CastSpell(nil, spell)
    schedule(guid, "twinspike", 20000, function()
        twinSpikeTick(creature, guid, spell)
    end)
end

local function makeEnterCombat(surgeSpell, spikeSpell)
    return function(event, creature, target)
        creature:Talk(SAY_AGGRO)
        creature:CastSpell(creature, surgeSpell)
        local guid = creature:GetGUID()
        cancelTimers(guid)
        schedule(guid, "twinspike", 20000, function()
            twinSpikeTick(creature, guid, spikeSpell)
        end)
    end
end

local function makeLeaveCombat()
    return function(event, creature)
        cancelTimers(creature:GetGUID())
    end
end

local function targetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL_PLAYER)
    end
end

local function died(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

local function reset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_FJOLA, 1, makeEnterCombat(SPELL_SURGE_FJOLA, SPELL_SPIKE_FJOLA))
RegisterCreatureEvent(ENTRY_FJOLA, 2, makeLeaveCombat())
RegisterCreatureEvent(ENTRY_FJOLA, 3, targetDied)
RegisterCreatureEvent(ENTRY_FJOLA, 4, died)
RegisterCreatureEvent(ENTRY_FJOLA, 23, reset)

RegisterCreatureEvent(ENTRY_EYDIS, 1, makeEnterCombat(SPELL_SURGE_EYDIS, SPELL_SPIKE_EYDIS))
RegisterCreatureEvent(ENTRY_EYDIS, 2, makeLeaveCombat())
RegisterCreatureEvent(ENTRY_EYDIS, 3, targetDied)
RegisterCreatureEvent(ENTRY_EYDIS, 4, died)
RegisterCreatureEvent(ENTRY_EYDIS, 23, reset)
