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
-- Essence / 34568 Light Essence (trial_of_the_crusader.h:213-214
-- NPC_DARK_ESSENCE = 34567 / NPC_LIGHT_ESSENCE = 34568 — kalecgos
-- pass — NOT bound in instance_trial_of_the_crusader.cpp;
-- ScriptName->entry binding DB-side); 34743
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
-- 65766 Light / 65768 Dark) (event arms; the me->DoZoneInCombat() first
-- arm is PORTED 2026-10-23 — DoZoneInCombat bridge live
-- (engine/world/lua_creature_events.go); the BossAI::JustEngagedWith
-- instance-bookkeeping leg and setActive have no bridges; the
-- cross-AI sister AddAura/Empathy + DoZoneInCombat(pSister) legs are
-- PORTED 2026-10-23 (sister via GetCreaturesInGrid, the JustDied
-- idiom — tharon_ja precedent superseded by the live bridge); the SetCombatPulseDelay(5) arm is PORTED
-- (creature:SetCombatPulseDelay bridge, lua_creature_events.go; event 1,
-- C++ arm order)); KilledUnit Talk(SAY_KILL_PLAYER 6)
-- player-gated (event 3, who->GetTypeId()==TYPEID_PLAYER — nalorakk
-- precedent); JustDied Talk(SAY_DEATH 8) (event 4) + the sister-gated
-- lootable legs (boss_twin_valkyr.cpp:237-258) — PORTED via the
-- creature:SetFlag(79)/RemoveFlag(79) dynamic-flag bridge
-- (lua_creature_events.go, luaMotionCreature): a sister-alive death clears LOOTABLE on me
-- (Go's lethal path had already set it); a both-dead death sets it on
-- me (redundant with the lethal path) and on the sister via
-- GetCreaturesInGrid (150y — same arena; no cross-creature GUID
-- bridge). The instance SetBossState DONE/SPECIAL legs are PORTED via
-- creature:SetBossState (lua_creature_events.go:6155, luaMotionCreature;
-- DATA_TWIN_VALKIRIES=3, DONE 3 / SPECIAL 4 — the stale "no bridge"
-- claim is retracted, verified 2026-11-07); HandleRemoveAuras() is PORTED via
-- stripHelperAuras (GetPlayersInWorld + mapId/instanceId filter +
-- player:RemoveAura — the moroes precedent); summons.DespawnAll()
-- is PORTED via despawnController (the bullet-controller model; Eydis
-- summons nothing, so a no-op for her). HandleRemoveAuras()'s
-- OrbsDespawner leg (:234 — 100ms-delayed despawn of NPC_BULLET_DARK /
-- NPC_BULLET_LIGHT orbs) stays out: orbs only spawn via the unported
-- spell_bullet_controller AuraScript, so no orb creatures exist in the
-- Lua model (unblocker: AuraScript binding bridge + orb summon machine);
-- EVENT_TWIN_SPIKE — DoCastVictim(Light Twin Spike 66075
-- / Dark Twin Spike 66069), 20s init, 20s repeat (moroes precedent;
-- C++ UpdateAI runs combat events whenever UpdateVictim() holds —
-- the PHASE_EVENT leg only matters pre-combat for EVENT_START_MOVE);
-- EVENT_BERSERK — DoCastSelf(Berserk 64238) + Talk(SAY_BERSERK 7),
-- IsHeroic() ? 6min : 8min one-shot (creature:IsHeroic() bridge —
-- the old "no difficulty bridge" claim was stale, retracted).
-- EVENT_TOUCH — heroic-only SelectTarget(Random,0,200.0f,true,true,
-- OtherEssenceSpellId) -> CastSpell(target, Light Touch 67297 / Dark
-- Touch 67282), randtime(10s,15s) init / randtime(10s,15s) re-arm —
-- the random-target leg rides the thorim randomPlayerInRange
-- convention, the aura filter rides plr:HasAura (hakkar precedent);
-- the SPELLVALUE_MAX_TARGETS=1 spell-mod has no bridge (noted).
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
-- UNKNOWN19, true) + summons.DespawnAll (no aura-state / despawn
-- bridges; react-state IS bridged — lua_creature_events.go — but both
-- ReactState arms (:191 Reset-PASSIVE / :216 MovementInform-AGGRESSIVE)
-- ride the unbridged intro/MovementInform machine, so the PASSIVE arm
-- alone would strand the twins passive) +
-- DoStopTimedAchievement(EVENT_START_TWINS_FIGHT)
-- (no achievement bridge — fjola only); JustReachedHome:
-- instance->SetBossState(DATA_TWIN_VALKIRIES, FAIL) + HandleRemoveAuras
-- + summons.DespawnAll + DespawnOrUnsummon + DoUseDoorOrButton(
-- DATA_MAIN_GATE): the SetBossState FAIL + RemoveAllAuras +
-- DespawnOrUnsummon arms are PORTED on event 24 below (both twins);
-- the Fordring wipe relay (ACTION_VALKYR_WIPE 26, instance :228-230)
-- + heroic TrialCounter decrement (instance :303-305) ride the same
-- ev24 site, guarded ~= FAIL/DONE so the second twin's ev24 cannot
-- double-fire (tick 406 U2). The door legs stay out (fjola's
-- EnterEvadeMode adds another DoUseDoorOrButton leg). The
-- instance-side crusaders-chest Delete() (instance :224-225, fires on
-- any TWIN_VALKIRIES state change) stays out: no GO bridge.
-- EVENT_SPECIAL_ABILITY (fjola only, 45s) — PORTED (specialTick
-- below): the shuffled stage machine with cross-AI sister DoAction
-- arms inlined (dispatch IS bridged — creature:DoAction fires ev38,
-- lua_creature_events.go:802 — and the relay is already inlined into
-- the sender's timer, the cross-AI relay precedent). Dark stages run
-- the sister's VORTEX/PACT arms via the GetCreaturesInGrid sister
-- idiom; light stages run fjola's own.
-- EVENT_TOUCH (heroic only) — PORTED (touchTick, below):
-- SelectTarget(Random,0,200.0f,true,true,OtherEssenceSpellId) ->
-- CastSpell(target, Light Touch 67297 / Dark Touch 67282,
-- SPELLVALUE_MAX_TARGETS 1), randtime(10s,15s) init / randtime(10s,15s)
-- repeat — the stale "no random-target / aura-filtered SelectTarget
-- bridge (cairne/kazzak)" claim is retracted: the random-target leg
-- rides the thorim randomPlayerInRange convention, the aura filter
-- rides plr:HasAura (bridged — engine/world/lua_player.go:203, hakkar
-- precedent); the SPELLVALUE_MAX_TARGETS=1 spell-mod has no bridge
-- (noted, not wired).
-- npc_essence_of_twin (34567/34568 — trial_of_the_crusader.h:213-214,
-- entries C++-verifiable — the old "entry-unverifiable" note was stale):
-- OnGossipHello -> RemoveAurasDueToSpell(ESSENCE_REMOVE) +
-- CastSpell(player, ESSENCE_APPLY) — ported below (stale "no gossip
-- bridge" claim retracted — RegisterCreatureGossipEvent is bridged,
-- engine/scripting/runtime.go:902).
-- npc_unleashed_dark (34628) / npc_unleashed_light (34630) —
-- entry-unverifiable: Reset SetFlag(NON_ATTACKABLE | NOT_SELECTABLE)
-- + SetReactState(REACT_PASSIVE) + SetDisableGravity(true) +
-- SetCanFly(true) (BRIDGED 2026-10-23, creature:SetCanFly — arm
-- stays out behind the unported add AI, unregistered + motion machine)
-- + SetCombatMovement(false) + MovePoint (no flag /
-- react / gravity / combat-movement / motion bridges);
-- UpdateAI: SelectNearestPlayer(3.0f) -> DoCastAOE(Unleashed Dark
-- 65808-helper RAID_MODE(65808,67172,67173,67174) / Unleashed Light
-- 65795-helper RAID_MODE(65795,67238,67239,67240)) + MoveIdle +
-- DespawnOrUnsummon(1s), 500ms timer — the DoCastAOE legs stay out
-- behind the absent player-range scan (SelectNearestPlayer) +
-- MoveIdle/DespawnOrUnsummon bridges; MovementInform
-- POINT_MOTION_TYPE point 0: urand
-- 1/4 re-move else DisappearAndDie (no MovementInform / despawn
-- bridges) — documented-only.
-- npc_bullet_controller (34743 — entry-verifiable: NPC_BULLET_
-- CONTROLLER in the file's Summons enum, boss_twin_valkyr.cpp:60; the
-- old "entry-unverifiable" note was stale): Reset DoCastAOE(Bullet
-- Controller Periodic 66149) — stays out: the whole effect is the
-- spell_bullet_controller AuraScript (shazzrah precedent); UpdateAI:
-- UpdateVictim only — documented-only.
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
-- SetCanDualWield) — no AuraScript binding bridge (razelikh precedent)
-- and no dual-wield bridge — documented-only. The stale "no equipment
-- bridge" claim is retracted: creature:SetEquipmentSlots is bridged on the
-- event surface (lua_creature_events.go), so the unconditional Reset arms
-- are PORTED in reset(); the AuraScript-timed arms (:310) stay out on the
-- missing AuraScript bridge. The four spell scripts join the
-- SpellScript/AuraScript queue.

local ENTRY_FJOLA = 34497
local ENTRY_EYDIS = 34496
local NPC_BULLET_CONTROLLER = 34743
local controllers = {}

-- C++ JustDied (:237-259) ends with the unconditional
-- summons.DespawnAll(): Fjola's list holds the bullet controller
-- summoned on engage (boss_twin_baseAI latch); Eydis summons
-- nothing, so this is a no-op for her. BossAI Reset (:499-507)
-- and evade (CreatureAI::EnterEvadeMode ends in Reset) despawn it
-- too, so the reset/leave handlers call this as well.
local function despawnController(guid)
    local controller = controllers[guid]
    if controller then
        controller:Despawn()
        controllers[guid] = nil
    end
end
-- NPC_BULLET_CONTROLLER (boss_twin_valkyr.cpp:60) — summoned by Fjola's
-- JustEngagedWith (:446) at ToCCommonLoc[1] (Center: 563.673/139.571/
-- 393.837, trial_of_the_crusader.cpp:184-190), TEMPSUMMON_MANUAL_DESPAWN.

-- boss_twin_valkyr.h:50-51 / Trinity SetEquipmentSlots(load=false, ...) with
-- EQUIP_UNEQUIP=0 (slot zeroed) and EQUIP_NO_CHANGE->0xFFFFFFFF (slot skipped,
-- the npc_toc_boomkin precedent) for the bridge's no-change sentinel.
local EQUIP_MAIN_FJOLA = 9423
local EQUIP_MAIN_EYDIS = 37377
local EQUIP_UNEQUIP = 0
local EQUIP_NO_CHANGE = 0xFFFFFFFF

local SAY_AGGRO = 0
local SAY_KILL_PLAYER = 6
local SAY_BERSERK = 7
local SAY_DEATH = 8

local SPELL_SURGE_FJOLA = 65766
local SPELL_SURGE_EYDIS = 65768
local SPELL_SPIKE_FJOLA = 66075
local SPELL_SPIKE_EYDIS = 66069
local SPELL_BERSERK = 64238
local SPELL_LIGHT_TOUCH = 67297 -- C++ :74 (fjola's TouchSpellId)
local SPELL_DARK_TOUCH = 67282 -- C++ :86 (eydis's TouchSpellId)
-- C++ :77/:89 (MyEmphatySpellId): fjola applies TWIN_EMPATHY_DARK to
-- eydis, eydis applies TWIN_EMPATHY_LIGHT to fjola (JustEngagedWith
-- :271 — me->AddAura(spell, pSister), aura lands on the sister).
local SPELL_TWIN_EMPATHY_LIGHT = 66133
local SPELL_TWIN_EMPATHY_DARK = 66132
-- C++ RAID_MODE essence variants (:120-121): only one variant can be
-- present per instance (applied difficulty-gated), so the pool checks
-- all four — C++-exact within a running instance.
local DARK_ESSENCE_AURAS = { 65684, 67176, 67177, 67178 }
local LIGHT_ESSENCE_AURAS = { 65686, 67222, 67223, 67224 }
-- C++ RAID_MODE helper pool (boss_twin_valkyr.cpp:123): only one
-- variant can be present per instance, so the strip pool checks all
-- four — same convention as DARK/LIGHT_ESSENCE_AURAS above.
local POWERING_UP_HELPER_AURAS = { 67590, 67602, 67603, 67604 }
local EMOTE_VORTEX = 3
local EMOTE_TWIN_PACT = 4
local SAY_TWIN_PACT = 5
local SPELL_LIGHT_VORTEX = 66046
local SPELL_DARK_VORTEX = 66058
local SPELL_LIGHT_SHIELD = 65858
local SPELL_DARK_SHIELD = 65874
local SPELL_LIGHT_TWIN_PACT = 65876
local SPELL_DARK_TWIN_PACT = 65875
local SPELL_POWER_TWINS = 65879

-- C++ boss_twin_baseAI::HandleRemoveAuras (boss_twin_valkyr.cpp:228-235):
-- the four DoRemoveAurasDueToSpellOnPlayers helper-aura strips, in C++
-- call order (the :233 POWERING_UP_HELPER repeat is a no-op second
-- strip, kept in the pool order; the sister-essence pools reuse the
-- DARK/LIGHT_ESSENCE_AURAS tables — same RAID_MODE ids). The
-- instance-player set is the mapId/instanceId-filtered
-- GetPlayersInWorld() pool (the moroes precedent) and
-- player:RemoveAura is bridged (lua_player.go:219); the OrbsDespawner
-- leg rides the unported summon machine.
local function stripHelperAuras(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local pools = { LIGHT_ESSENCE_AURAS, POWERING_UP_HELPER_AURAS,
                    DARK_ESSENCE_AURAS, POWERING_UP_HELPER_AURAS }
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            for _, pool in ipairs(pools) do
                for _, aura in ipairs(pool) do
                    p:RemoveAura(aura)
                end
            end
        end
    end
end

-- Trial of the Crusader Fordring relay (tick 406 U2): the instance
-- SetBossState(DATA_TWIN_VALKIRIES, ...) cascade
-- (instance_trial_of_the_crusader.cpp:222-235) forwards DONE/FAIL to
-- Tirion Fordring via DoAction; creature:DoAction(n) fires ev38 on the
-- target. DATA_FORDRING = 12, ENTRY_FORDRING = 34996
-- (trial_of_the_crusader.h:46, :156); ACTION_VALKYR_DEFEATED = 19,
-- ACTION_VALKYR_WIPE = 26 (trial_of_the_crusader.h:116, :123 — enum
-- counted from ACTION_START_GORMOK = 1).
local DATA_FORDRING = 12
local ENTRY_FORDRING = 34996

local function sendToFordring(creature, action)
    local fordring = creature:GetInstanceCreature(DATA_FORDRING)
    if fordring == nil then fordring = creature:FindNearestCreature(ENTRY_FORDRING, 200) end
    if fordring ~= nil then fordring:DoAction(action) end
end

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

-- C++ SelectTarget(Random,0,200.0f,true,true,OtherEssenceSpellId)
-- pool (:323): SelectTarget draws the threat list (UnitAI.h), so the
-- pool is threat-list players alive within 200y carrying the OTHER
-- essence — not instance players (player HasAura bridged — hakkar
-- precedent).
local function randomPlayerWithEssence(creature, auraIds, maxDist)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead()
                and creature:GetDistance(plr) - creature:GetCombatReach() - 1.5 < maxDist then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            for _, aura in ipairs(auraIds) do
                if plr:HasAura(aura) then
                    candidates[#candidates + 1] = plr
                    break
                end
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ UpdateAI EVENT_TOUCH (:322-330): random other-essence target ->
-- CastSpell(target, TouchSpellId, MAX_TARGETS 1), repeat randtime
-- (10s,15s) — re-armed unconditionally (outside the target-if —
-- sjonnir precedent). The SPELLVALUE_MAX_TARGETS=1 spell-mod has no
-- bridge (noted).
local function touchTick(creature, guid, touchSpell, essenceAuras)
    if creature:IsCasting() then
        schedule(guid, "touch", 500, function()
            touchTick(creature, guid, touchSpell, essenceAuras)
        end)
        return
    end
    local target = randomPlayerWithEssence(creature, essenceAuras, 200)
    if target then
        creature:CastSpell(target, touchSpell)
    end
    schedule(guid, "touch", {10000, 15000}, function()
        touchTick(creature, guid, touchSpell, essenceAuras)
    end)
end

-- C++ EVENT_TWIN_SPIKE: DoCastVictim(SpikeSpellId), 20s init, 20s
-- repeat. C++ UpdateAI pre-loop (:351) + in-loop (:357) CASTING
-- gates: 500ms IsCasting defer (mal_ganis idiom) — the queued event
-- is not consumed while casting; same defer on touchTick,
-- specialTick (before stage consumption, matching the pre-loop
-- gate) and berserkTick.
local function twinSpikeTick(creature, guid, spell)
    if creature:IsCasting() then
        schedule(guid, "twinspike", 500, function()
            twinSpikeTick(creature, guid, spell)
        end)
        return
    end
    creature:CastSpell(nil, spell)
    schedule(guid, "twinspike", 20000, function()
        twinSpikeTick(creature, guid, spell)
    end)
end

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128).
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

-- C++ GetSister() (boss_twin_valkyr.cpp:260-265) resolves the sister
-- via the instance and branches on its live state, so died() below
-- resolves the sister via GetCreaturesInGrid (which, unlike
-- FindNearestCreature, also returns dead creatures) and branches on
-- sister:IsDead() — no shared death counter (a counter is wrong across
-- concurrent instances and on double event-4 fires).

-- C++ boss_fjolaAI::GenerateStageSequence + ExecuteEvent
-- EVENT_SPECIAL_ABILITY (boss_twin_valkyr.cpp:411-437, 462-476):
-- fjola-only 45s timer; the stage order is shuffled once (constructor
-- in C++, lazily at first tick here — unobservable difference, the
-- sequence is never consumed before the first tick) and reshuffled
-- when exhausted; dark stages relay to the sister's DoAction arms,
-- light stages run fjola's own. C++ DELTA: the C++ shuffle's
-- urand(0, MAX_STAGES - i) is inclusive, so it can pick index 4 —
-- one past Stage[4], aliasing CurrentStage in memory; Lua clamps the
-- draw to the valid range (i..3). Stage state is keyed by fjola GUID
-- and NOT cleared on reset/leave, matching C++ (Reset never touches
-- Stage/CurrentStage). DoCastAOE arms are self-casts (Oct-13
-- convention); the sister is the GetCreaturesInGrid idiom.
local specialState = {}

local function generateStageSequence(guid)
    local stages = { 0, 1, 2, 3 }
    for i = 1, 3 do
        local r = i + math.random(0, 4 - i)
        stages[i], stages[r] = stages[r], stages[i]
    end
    specialState[guid] = { stages = stages, current = 0 }
end

local function specialTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "special", 500, function()
            specialTick(creature, guid)
        end)
        return
    end
    local state = specialState[guid]
    if not state or state.current >= 4 then
        generateStageSequence(guid)
        state = specialState[guid]
    end
    local stage = state.stages[state.current + 1]
    state.current = state.current + 1
    local sister = nil
    for _, other in ipairs(creature:GetCreaturesInGrid(ENTRY_EYDIS, 150)) do
        sister = other
        break
    end
    if stage == 0 then -- STAGE_DARK_VORTEX: sister DoAction VORTEX
        if sister then
            sister:Talk(EMOTE_VORTEX)
            sister:CastSpell(sister, SPELL_DARK_VORTEX)
        end
    elseif stage == 1 then -- STAGE_DARK_PACT: sister DoAction PACT
        if sister then
            sister:Talk(EMOTE_TWIN_PACT)
            sister:Talk(SAY_TWIN_PACT)
            creature:CastSpell(creature, SPELL_POWER_TWINS)
            sister:CastSpell(sister, SPELL_DARK_SHIELD)
            sister:CastSpell(sister, SPELL_DARK_TWIN_PACT)
        end
    elseif stage == 2 then -- STAGE_LIGHT_VORTEX: own DoAction VORTEX
        creature:Talk(EMOTE_VORTEX)
        creature:CastSpell(creature, SPELL_LIGHT_VORTEX)
    else -- STAGE_LIGHT_PACT: own DoAction PACT
        creature:Talk(EMOTE_TWIN_PACT)
        creature:Talk(SAY_TWIN_PACT)
        if sister then
            sister:CastSpell(sister, SPELL_POWER_TWINS)
        end
        creature:CastSpell(creature, SPELL_LIGHT_SHIELD)
        creature:CastSpell(creature, SPELL_LIGHT_TWIN_PACT)
    end
    schedule(guid, "special", 45000, function()
        specialTick(creature, guid)
    end)
end

-- C++ EVENT_BERSERK: DoCastSelf(Berserk 64238) + Talk(SAY_BERSERK 7),
-- IsHeroic() ? 6min : 8min one-shot; the UpdateAI pre-loop (:351) +
-- in-loop (:357) CASTING gates defer it 500ms like the other ticks
-- (mal_ganis idiom), with no repeat re-arm once it fires.
local function berserkTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            berserkTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK)
    creature:Talk(SAY_BERSERK)
end

local function makeEnterCombat(surgeSpell, spikeSpell, touchSpell, essenceAuras, empathySpell)
    return function(event, creature, target)
        -- C++ boss_fjolaAI::JustEngagedWith (boss_twin_valkyr.cpp:442-448):
        -- SummonCreature(NPC_BULLET_CONTROLLER 34743, ToCCommonLoc[1],
        -- 0.0, TEMPSUMMON_MANUAL_DESPAWN) lands before the
        -- boss_twin_baseAI engage legs (Talk + surge below) — C++-exact;
        -- Fjola-only (eydis AI has no summon).
        if creature:GetEntry() == ENTRY_FJOLA then
            controllers[creature:GetGUID()] = creature:SummonCreature(NPC_BULLET_CONTROLLER, 563.672974, 139.571, 393.837006, 0.0, 7, 0)
        end
        -- C++ boss_twin_baseAI::JustEngagedWith first arm (:268, PORTED
        -- 2026-10-23 — DoZoneInCombat bridge live,
        -- engine/world/lua_creature_events.go); the sister block
        -- (:269-272 — AddAura(MyEmphatySpellId, pSister) +
        -- DoZoneInCombat(pSister)) is PORTED 2026-10-23: the sister is
        -- gathered via GetCreaturesInGrid (the JustDied sister idiom
        -- below; the C++ instance->GetCreature nil-guard is the empty
        -- grid), the aura lands on the sister object (sister:AddAura —
        -- receiver-side twin of me->AddAura(spell, pSister)), C++ arm
        -- order kept (aura before zone-in).
        creature:DoZoneInCombat()
        local sisterEntry = creature:GetEntry() == ENTRY_FJOLA and ENTRY_EYDIS or ENTRY_FJOLA
        for _, sister in ipairs(creature:GetCreaturesInGrid(sisterEntry, 150)) do
            sister:AddAura(empathySpell)
            sister:DoZoneInCombat()
        end
        creature:Talk(SAY_AGGRO)
        creature:CastSpell(creature, surgeSpell)
        creature:SetCombatPulseDelay(5) -- boss_twin_valkyr.cpp:278 (JustEngagedWith arm; the setActive leg has no bridge)
        local guid = creature:GetGUID()
        cancelTimers(guid)
        -- C++ boss_fjolaAI::JustEngagedWith :444: EVENT_SPECIAL_ABILITY
        -- 45s (fjola only; the timed-achievement leg has no bridge).
        if creature:GetEntry() == ENTRY_FJOLA then
            schedule(guid, "special", 45000, function()
                specialTick(creature, guid)
            end)
        end
        schedule(guid, "twinspike", 20000, function()
            twinSpikeTick(creature, guid, spikeSpell)
        end)
        -- C++ JustEngagedWith :283-284: EVENT_TOUCH scheduled
        -- randtime(10s,15s) under IsHeroic() only.
        if creature:IsHeroic() then
            schedule(guid, "touch", {10000, 15000}, function()
                touchTick(creature, guid, touchSpell, essenceAuras)
            end)
        end
        -- C++ EVENT_BERSERK: IsHeroic() ? 6min : 8min (one-shot);
        -- execute: DoCastSelf(Berserk 64238) + Talk(SAY_BERSERK 7).
        -- Stale "no difficulty bridge" claim retracted: creature:IsHeroic()
        -- is bridged (engine/world/lua_creature_events.go:260).
        schedule(guid, "berserk", creature:IsHeroic() and 360000 or 480000, function()
            berserkTick(creature, guid)
        end)
    end
end

local function makeLeaveCombat()
    return function(event, creature)
        cancelTimers(creature:GetGUID())
        despawnController(creature:GetGUID())
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
    -- C++ JustDied (boss_twin_valkyr.cpp:237-259): Talk(SAY_DEATH), then
    -- the sister-gated lootable legs, in C++ order.
    local sisterEntry = creature:GetEntry() == ENTRY_FJOLA and ENTRY_EYDIS or ENTRY_FJOLA
    local sister = nil
    for _, other in ipairs(creature:GetCreaturesInGrid(sisterEntry, 150)) do
        sister = other
        break
    end
    if sister ~= nil and sister:IsDead() then
        -- Both dead: C++ sets LOOTABLE on me (redundant — Go's lethal
        -- path already set it) and on the sister.
        creature:SetFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        sister:SetFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        -- C++ JustDied :246-249 — events.Reset() is the cancelTimers(guid)
        -- at the top; HandleRemoveAuras() ports via stripHelperAuras
        -- (C++ order kept), then SetBossState(DATA_TWIN_VALKIRIES, DONE).
        stripHelperAuras(creature)
        creature:SetBossState(3, 3) -- DATA_TWIN_VALKIRIES, DONE (boss_twin_valkyr.cpp:249)
        sendToFordring(creature, 19) -- ACTION_VALKYR_DEFEATED (instance_trial_of_the_crusader.cpp:232-234)
    elseif sister ~= nil then
        -- Sister still fighting: not lootable (clears the lethal path's
        -- auto-set).
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        creature:SetBossState(3, 4) -- DATA_TWIN_VALKIRIES, SPECIAL (boss_twin_valkyr.cpp:254)
    end
    -- Sister not found: C++ takes neither flag branch (Talk only).
    -- C++ JustDied's trailing unconditional summons.DespawnAll()
    -- (:258) runs after the sister branches, whatever they took.
    despawnController(creature:GetGUID())
end

local function reset(event, creature)
    cancelTimers(creature:GetGUID())
    despawnController(creature:GetGUID())
    -- boss_fjolaAI::Reset :392 / boss_eydisAI::Reset :499 — unconditional
    -- SetEquipmentSlots(false, EQUIP_MAIN_*, EQUIP_UNEQUIP, EQUIP_NO_CHANGE):
    -- creature:SetEquipmentSlots is bridged on the event surface.
    if creature:GetEntry() == ENTRY_FJOLA then
        creature:SetEquipmentSlots(EQUIP_MAIN_FJOLA, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    else
        creature:SetEquipmentSlots(EQUIP_MAIN_EYDIS, EQUIP_UNEQUIP, EQUIP_NO_CHANGE)
    end
end

RegisterCreatureEvent(ENTRY_FJOLA, 1, makeEnterCombat(SPELL_SURGE_FJOLA, SPELL_SPIKE_FJOLA, SPELL_LIGHT_TOUCH, DARK_ESSENCE_AURAS, SPELL_TWIN_EMPATHY_DARK))
RegisterCreatureEvent(ENTRY_FJOLA, 2, makeLeaveCombat())
RegisterCreatureEvent(ENTRY_FJOLA, 3, targetDied)
RegisterCreatureEvent(ENTRY_FJOLA, 4, died)
RegisterCreatureEvent(ENTRY_FJOLA, 23, reset)

RegisterCreatureEvent(ENTRY_EYDIS, 1, makeEnterCombat(SPELL_SURGE_EYDIS, SPELL_SPIKE_EYDIS, SPELL_DARK_TOUCH, LIGHT_ESSENCE_AURAS, SPELL_TWIN_EMPATHY_LIGHT))
RegisterCreatureEvent(ENTRY_EYDIS, 2, makeLeaveCombat())
RegisterCreatureEvent(ENTRY_EYDIS, 3, targetDied)
RegisterCreatureEvent(ENTRY_EYDIS, 4, died)
RegisterCreatureEvent(ENTRY_EYDIS, 23, reset)

-- C++ boss_twin_baseAI::JustReachedHome (boss_twin_valkyr.cpp:203-208,
-- shared by both twins): instance->SetBossState(DATA_TWIN_VALKIRIES, FAIL)
-- is PORTED via creature:SetBossState below (bridge live); the
-- instance-side Fordring wipe relay + heroic TrialCounter decrement
-- ride the same site (tick 406 U2), guarded ~= FAIL/DONE so the second
-- twin's ev24 cannot double-fire the relay or the counter — C++ fires
-- the cascade once per SetBossState call, and both twins reach home on
-- a wipe. HandleRemoveAuras() (:228-235) is PORTED via
-- stripHelperAuras (GetPlayersInWorld + mapId/instanceId filter +
-- player:RemoveAura — the moroes precedent; the stale
-- "no separable bridgeable arm" claim is retracted — player:RemoveAura
-- is bridged, lua_player.go:219); summons.DespawnAll() is vacuous
-- (no summon model, adds unregistered). The other bridgeable arm is
-- me->DespawnOrUnsummon() -> creature:Despawn() (respawns DB-spawned
-- creatures on their normal spawntimesecs, opera precedent). Event 24 fires
-- at the home-move completion. NOTE: this is ev24 (JustReachedHome),
-- not ev2 — the ev2 leave-combat handler writes no boss state, and
-- fjola's EnterEvadeMode (boss_twin_valkyr.cpp:450-454) only adds a
-- door leg; death reaches neither ev2 nor ev24.
local function reachHome(event, creature)
    -- C++ JustReachedHome :203-208 — instance->SetBossState(
    -- DATA_TWIN_VALKIRIES, FAIL) PORTED below (bridge live);
    -- HandleRemoveAuras() ports via stripHelperAuras (C++ order kept);
    -- summons.DespawnAll() is a no-op. The FAIL write + Fordring relay
    -- + TrialCounter decrement are guarded ~= FAIL/DONE (beasts ev2
    -- precedent, extended with DONE: a DONE encounter must never flip
    -- back to FAIL); strip + Despawn stay unconditional — each twin
    -- still strips auras and despawns itself.
    local state = creature:GetBossState(3) -- DATA_TWIN_VALKIRIES
    if state ~= 2 and state ~= 3 then -- FAIL / DONE
        creature:SetBossState(3, 2) -- DATA_TWIN_VALKIRIES, FAIL
        sendToFordring(creature, 26) -- ACTION_VALKYR_WIPE (instance_trial_of_the_crusader.cpp:228-230)
        -- C++ instance SetBossState generic FAIL block (:301-305):
        -- heroic-only --TrialCounter, floor 0, shared instanceData
        -- table (trialCounter, default 50). The world-state UI push
        -- (:307-310) and the zero-attempt Anub'arak despawn (:312-317)
        -- have no bridges and stay out.
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data and creature:IsHeroic() then
                local c = (data.trialCounter or 50) - 1
                if c < 0 then c = 0 end
                data.trialCounter = c
            end
        end
    end
    stripHelperAuras(creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_FJOLA, 24, reachHome)
RegisterCreatureEvent(ENTRY_EYDIS, 24, reachHome)

-- npc_essence_of_twin OnGossipHello (boss_twin_valkyr.cpp:548-555):
-- C++-exact RemoveAurasDueToSpell(ESSENCE_REMOVE) + triggered
-- player self-cast ESSENCE_APPLY + CloseGossipMenuFor. Entries
-- 34567 Dark / 34568 Light Essence (trial_of_the_crusader.h:213-214);
-- GetData (cpp:534-543): light essence removes the dark helper and
-- applies the light helper, dark essence mirrored. player:CastSpell
-- has no bridge (kalecgos pass), so the triggered player self-cast
-- is modeled as player:AddAura (kodo precedent —
-- npc_aged_dying_ancient_kodo.lua:71). RAID_MODE
-- C++ essence OnGossipHello (boss_twin_valkyr.cpp:549-555): remove
-- the opposite pool, then player->CastSpell(player, apply, true) with
-- the RAID_MODE-resolved variant (10N/25N/10H/25H = table indices
-- 1/2/3/4). The remove leg strips all four variants (the file's pool
-- convention); the apply leg picks the variant from the gossip
-- creature's IsHeroic()/Is25ManRaid() bridges. Player:CastSpell takes
-- no triggered flag, so AddAura stays as the application path
-- (documented approximation for the triggered-cast leg only).
-- Handler signature (event, player, creature) per the zulaman.lua
-- RegisterCreatureGossipEvent precedent. No forward-decl hazard: the
-- factory returns the closure before registration.
local function makeEssenceHello(removeAuras, applyVariants)
    return function(_, player, creature)
        for _, aura in ipairs(removeAuras) do
            player:RemoveAura(aura)
        end
        local idx = 1
        if creature ~= nil then
            if creature:IsHeroic() then
                idx = creature:Is25ManRaid() and 4 or 3
            elseif creature:Is25ManRaid() then
                idx = 2
            end
        end
        player:AddAura(applyVariants[idx])
        player:GossipComplete()
    end
end

RegisterCreatureGossipEvent(34568, 1, makeEssenceHello(DARK_ESSENCE_AURAS, LIGHT_ESSENCE_AURAS))
RegisterCreatureGossipEvent(34567, 1, makeEssenceHello(LIGHT_ESSENCE_AURAS, DARK_ESSENCE_AURAS))
