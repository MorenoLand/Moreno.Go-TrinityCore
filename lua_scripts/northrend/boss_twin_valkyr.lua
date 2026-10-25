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
-- lootable legs (boss_twin_valkyr.cpp:238-262) — PORTED via the
-- creature:SetFlag(79)/RemoveFlag(79) dynamic-flag bridge
-- (lua_creature_events.go): a sister-alive death clears LOOTABLE on me
-- (Go's lethal path had already set it); a both-dead death sets it on
-- me (redundant with the lethal path) and on the sister via
-- GetCreaturesInGrid (150y — same arena; no cross-creature GUID
-- bridge). The instance SetBossState DONE/SPECIAL legs have no
-- bridge (documented); HandleRemoveAuras() is PORTED via
-- stripHelperAuras (GetPlayersInWorld + mapId/instanceId filter +
-- player:RemoveAura — the moroes precedent); summons.DespawnAll()
-- is a no-op (documented);
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
-- DATA_MAIN_GATE): the RemoveAllAuras + DespawnOrUnsummon arms are PORTED
-- on event 24 below (both twins); instance SetBossState / door legs have
-- no bridges — fjola's EnterEvadeMode adds another DoUseDoorOrButton leg.
-- EVENT_SPECIAL_ABILITY (fjola only, 45s) — the shuffled stage
-- machine: cross-AI sister->AI()->DoAction(ACTION_VORTEX /
-- ACTION_PACT) or own DoAction for light stages (no cross-AI DoAction
-- bridge); DoAction ACTION_VORTEX: Talk(EMOTE_VORTEX 3) + DoCastAOE(
-- Light Vortex 66046 / Dark Vortex 66058) (stays out: fires only via
-- the unbridged cross-AI DoAction through fjola's 45s stage
-- machine); DoAction ACTION_PACT: Talk(
-- EMOTE_TWIN_PACT 4) + Talk(SAY_TWIN_PACT 5) + cross-AI sister cast
-- Power of the Twins 65879 + DoCastSelf(Light Shield 65858 / Dark
-- Shield 65874) + DoCastSelf(Light Twin Pact 65876 / Dark Twin Pact
-- 65875) — the self-cast legs are port-pattern-ready in isolation
-- (phase_hunter precedent) but fire only via cross-AI DoAction —
-- documented-only.
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
                and creature:IsWithinDist(plr, maxDist) then
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
    local target = randomPlayerWithEssence(creature, essenceAuras, 200)
    if target then
        creature:CastSpell(target, touchSpell)
    end
    schedule(guid, "touch", {10000, 15000}, function()
        touchTick(creature, guid, touchSpell, essenceAuras)
    end)
end

-- C++ EVENT_TWIN_SPIKE: DoCastVictim(SpikeSpellId), 20s init, 20s
-- repeat.
local function twinSpikeTick(creature, guid, spell)
    creature:CastSpell(nil, spell)
    schedule(guid, "twinspike", 20000, function()
        twinSpikeTick(creature, guid, spell)
    end)
end

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128).
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

-- Both sisters share this module scope, so the C++ GetSister() liveness
-- check is modeled as a per-pull death count: the first death of a pull
-- takes the sister-alive branch, the second the both-dead branch. Reset
-- on engage — ev23 also fires on the death path (ahead of ev4), so the
-- counter cannot live in reset().
local sisterDeaths = 0

local function makeEnterCombat(surgeSpell, spikeSpell, touchSpell, essenceAuras, empathySpell)
    return function(event, creature, target)
        sisterDeaths = 0
        -- C++ boss_fjolaAI::JustEngagedWith (boss_twin_valkyr.cpp:442-448):
        -- SummonCreature(NPC_BULLET_CONTROLLER 34743, ToCCommonLoc[1],
        -- 0.0, TEMPSUMMON_MANUAL_DESPAWN) lands before the
        -- boss_twin_baseAI engage legs (Talk + surge below) — C++-exact;
        -- Fjola-only (eydis AI has no summon).
        if creature:GetEntry() == ENTRY_FJOLA then
            creature:SummonCreature(NPC_BULLET_CONTROLLER, 563.672974, 139.571, 393.837006, 0.0, 7, 0)
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
            creature:CastSpell(creature, SPELL_BERSERK)
            creature:Talk(SAY_BERSERK)
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
    -- C++ JustDied (boss_twin_valkyr.cpp:238-262): Talk(SAY_DEATH), then
    -- the sister-gated lootable legs, in C++ order.
    sisterDeaths = sisterDeaths + 1
    if sisterDeaths >= 2 then
        -- Both dead: C++ sets LOOTABLE on me (redundant — Go's lethal
        -- path already set it) and on the sister.
        creature:SetFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        local sisterEntry = creature:GetEntry() == ENTRY_FJOLA and ENTRY_EYDIS or ENTRY_FJOLA
        for _, other in ipairs(creature:GetCreaturesInGrid(sisterEntry, 150)) do
            other:SetFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
        end
        -- C++ JustDied :248 — events.Reset() is the cancelTimers(guid)
        -- at the top, summons.DespawnAll() is a no-op,
        -- instance->SetBossState(DATA_TWIN_VALKIRIES, DONE) has no
        -- instance bridge; HandleRemoveAuras() ports via
        -- stripHelperAuras (C++ order kept).
        stripHelperAuras(creature)
    else
        -- Sister still fighting: not lootable (clears the lethal path's
        -- auto-set).
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
    end
end

local function reset(event, creature)
    cancelTimers(creature:GetGUID())
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
-- has no instance bridge; HandleRemoveAuras() (:228-235) is PORTED via
-- stripHelperAuras (GetPlayersInWorld + mapId/instanceId filter +
-- player:RemoveAura — the moroes precedent; the stale
-- "no separable bridgeable arm" claim is retracted — player:RemoveAura
-- is bridged, lua_player.go:219); summons.DespawnAll() is vacuous
-- (no summon model, adds unregistered). The other bridgeable arm is
-- me->DespawnOrUnsummon() -> creature:Despawn() (respawns DB-spawned
-- creatures on their normal spawntimesecs, opera precedent). Event 24 fires
-- at the home-move completion.
local function reachHome(event, creature)
    -- C++ JustReachedHome :203-208 — instance->SetBossState(
    -- DATA_TWIN_VALKIRIES, FAIL) has no instance bridge;
    -- HandleRemoveAuras() ports via stripHelperAuras (C++ order kept);
    -- summons.DespawnAll() is a no-op.
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
-- 10/25 has no bridge: the remove leg strips all four variants (the
-- file's pool convention); the apply leg uses the 10-man normal
-- variant (65684 dark / 65686 light — index 0), documented
-- approximation. Handler signature (event, player, creature) per the
-- zulaman.lua RegisterCreatureGossipEvent precedent. No
-- forward-decl hazard: the factory returns the closure before
-- registration.
local function makeEssenceHello(removeAuras, applyAura)
    return function(_, player, _)
        for _, aura in ipairs(removeAuras) do
            player:RemoveAura(aura)
        end
        player:AddAura(applyAura)
        player:GossipComplete()
    end
end

RegisterCreatureGossipEvent(34568, 1, makeEssenceHello(DARK_ESSENCE_AURAS, 65686))
RegisterCreatureGossipEvent(34567, 1, makeEssenceHello(LIGHT_ESSENCE_AURAS, 65684))
