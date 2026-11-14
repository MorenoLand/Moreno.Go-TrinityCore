-- Felblood Kael'thas (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_felblood_kaelthas.cpp
-- (boss_felblood_kaelthas — BossAI phase-one scheduler + 50%-HP
-- phase-two latch + lethal-damage outro chain). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (Karazhan block CLOSED —
-- attumen/midnight, curator, maiden, aran, malchezaar, terestian,
-- moroes, opera, netherspite, karazhan, nightbane all ported;
-- instance_karazhan blocked on the instance-script model; Gnomeregan
-- block yielded nothing bridgeable — blastmaster_emi_shortfuse is an
-- EscortAI/gossip/instance/gameobject/summon machine, grubbis arms
-- all gate on the summoner bridge, collecting-fallout is a
-- SpellScript; AddSC_boss_felblood_kaelthas next in the Magister's
-- Terrace block; instance_magisters_terrace blocked on the
-- instance-script model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h:48
-- names BOSS_KAELTHAS_SUNSTRIDER = 24664 in the MT creatures enum.
-- Script name: no explicit CreatureScript string in the file —
-- AddSC registers via RegisterMagistersTerraceCreatureAI(
-- boss_felblood_kaelthas) = RegisterCreatureAIWithFactory, which
-- stringifies the type name (ScriptMgr.h:1234), so "boss_felblood_
-- kaelthas". TDB: 24664 'Kael'thas Sunstrider', ScriptName
-- boss_felblood_kaelthas; 24674 'Phoenix', ScriptName
-- npc_felblood_kaelthas_phoenix; 24675 'Phoenix Egg', ScriptName '';
-- all VerifiedBuild 12340 — the port is registered
-- (boss_vaelastrasz precedent).
-- Whole-server-tree grep confirms the .cpp file +
-- eastern_kingdoms_script_loader.cpp (fwd-decl, call) as the only
-- sources of the class name.
-- Verifiable numbers (file's own enums/header): SPELL_FIREBALL =
-- 44189 / SPELL_PHOENIX = 44194 / SPELL_FLAME_STRIKE = 46162
-- (random-player targeted, 44s loop — bridged) / SPELL_SHOCK_BARRIER =
-- 46165 (heroic-only arm — PORTED via creature:IsHeroic(); stale
-- "no difficulty bridge" claim retracted) / SPELL_PYROBLAST = 36819
-- (ported — maiden_of_virtue random-player precedent; stale
-- "no SelectTarget bridge" claim retracted) / SPELL_EMOTE_TALK_EXCLAMATION =
-- 48348 / SPELL_EMOTE_POINT = 48349 / SPELL_EMOTE_ROAR = 48350 /
-- SPELL_QUITE_SUICIDE = 3617 (serverside) / SAY_GRAVITY_LAPSE_1 = 2 /
-- SAY_GRAVITY_LAPSE_2 = 3 (repeat lapses — ported below) /
-- SAY_POWER_FEEDBACK = 4 (ported below) /
-- SAY_SUMMON_PHOENIX = 5 / SAY_ANNOUNCE_PYROBLAST = 6
-- (heroic-only arm — ported) / SAY_FLAME_STRIKE = 7 (ported) /
-- SAY_DEATH = 8 /
-- SAY_INTRO_1 = 0 / SAY_INTRO_2 = 1 (SetData(DATA_KAELTHAS_INTRO)-
-- gated — no instance SetData bridge) / EVENT_FIREBALL init 1ms
-- repeat 2s500ms / EVENT_PHOENIX init 12s repeat 45s /
-- EVENT_FLAME_STRIKE repeat 44s / EVENT_SHOCK_BARRIER repeat 1min /
-- EVENT_QUITE_SUICIDE 11s / DATA_KAELTHAS_SUNSTRIDER = 3
-- (magisters_terrace.h:31-34: SELIN 0, VEXALLUS 1, DELRISSA 2,
-- KAELTHAS 3; EncounterCount 4, so the writes are live-consumed).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms:
-- Phase-one combat machine (maiden-of-virtue convention — keyed
-- timers with EventMap cancel-on-reschedule semantics, victim casts
-- GetVictim nil-guarded (incarcerator convention), self-casts via
-- creature:CastSpell(creature, spell); timers start on
-- OnEnterCombat(1), cancelled on OnLeaveCombat(2)/OnDied(4)/
-- OnReset(23)):
-- - EVENT_FIREBALL: victim-cast 44189, 1ms init -> 2s500ms loop.
-- - EVENT_PHOENIX: Talk(SAY_SUMMON_PHOENIX) + self-cast 44194,
--   12s init -> 45s loop (the phoenix add's own AI is ported in
--   npc_felblood_kaelthas_phoenix.lua; the summon spell itself is
--   real, seeded 2026-11-14).
-- - EVENT_FLAME_STRIKE: Talk(SAY_FLAME_STRIKE) + random-player
--   (SelectTarget(Random, 0, 40.0f, true) — GetPlayersInWorld + map/
--   instance/alive/IsWithinDist filter, maiden_of_virtue precedent)
--   cast 46162, 44s init -> 44s loop.
-- - EVENT_SHOCK_BARRIER (heroic-only): OnEnterCombat schedules it
--   61s only under creature:IsHeroic() (C++-exact JustEngagedWith
--   gate, lua_creature_events.go:4417; the "no difficulty bridge"
--   claim was stale, retracted). Fire arm in C++ order:
--   Talk(SAY_ANNOUNCE_PYROBLAST) + self-cast 46165 + fireball
--   reschedule 2s500ms (schedule() cancel-on-reschedule = EventMap
--   RescheduleEvent) + one-shot EVENT_PYROBLAST 2s + 1min repeat.
-- - EVENT_PYROBLAST: random-player (SelectTarget(Random, 0, 40.0f,
--   true) — same maiden_of_virtue precedent as flame strike; the
--   "no SelectTarget bridge" claim was stale, retracted) cast
--   36819, single-shot (C++ never re-arms it).
-- 50%-HP phase-two latch via OnDamageTaken(9) with a per-guid
-- one-shot flag reset on OnEnterCombat(1) (gyth convention —
-- C++-exact HealthBelowPctDamaged(50): GetHealth() - damage <
-- floor(0.5*GetMaxHealth()), integer strict-<): PREPARE arm (Talk
-- SAY_GRAVITY_LAPSE_1 + REACT_PASSIVE + AttackStop + StopMoving) +
-- cancel of the phase-one timers (C++-exact observable: phase-one
-- events stay scheduled but no longer execute once the phase flips to
-- PHASE_TWO; cancelling them is the faithful net effect) + the 1s
-- CENTER_TELEPORT chain below (gravity lapse -> spheres/beam/power
-- feedback -> repeat PREPARE with SAY_GRAVITY_LAPSE_2).
-- Lethal-damage outro chain via OnDamageTaken(9) with a per-guid
-- one-shot flag (C++-exact: damage >= GetHealth() is checked before
-- the 50% latch, and the 50% latch is skipped once the outro phase
-- is set — same ordering here): Talk(SAY_DEATH) + cancel of all
-- timers + one-shot self-cast chain 48348 @1s, 48349 @3s800ms,
-- 48350 @7s400ms, 48350 @10s, 3617 @11s (C++ EVENT_EMOTE_* /
-- EVENT_QUITE_SUICIDE, PHASE_OUTRO-gated). DamageTaken tail: non-self
-- lethal damage is rewritten to health-1 (curator precedent) so he
-- survives for the 11s chain — self-damage may still kill him.
-- Deviations from C++: the UpdateAI pre-loop CASTING gate
-- (boss_felblood_kaelthas.cpp:277) is ported as a 500ms IsCasting
-- defer under each timer arm's own key (mal_ganis/rend idiom,
-- lua_creature_events.go:2480; the defer re-queues the arm and
-- its repeat schedules only after the gate passes, so no re-arm
-- is consumed). In-loop same-tick co-expiry suppression is
-- unmodeled (independent callbacks, AV precedent). The 50% latch's
-- direct prepareArms call stays ungated (C++-exact: DamageTaken
-- is not an UpdateAI event; the repeat PREPARE timer arm is
-- gated). The C++ scheduler is
-- driven from JustEngagedWith and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden
-- convention). The outro's damage cap (lethal non-self damage is
-- clamped to health-1 so the boss survives at 1 HP through the
-- death sequence) is PORTED below; the outro summons.DespawnAll leg
-- is ported for the ev19-latched sphere subset and the ev19-latched
-- phoenix subset (below); AttackStop, REACT_PASSIVE,
-- InterruptNonMeleeSpells(true), RemoveAurasDueToSpell(POWER_
-- FEEDBACK) and DoCastAOE(CLEAR_FLIGHT) are all PORTED below;
-- the outro chain plays as the observable remnant (documented).
-- The intro is SetData(DATA_
-- KAELTHAS_INTRO)-gated (instance arm — no bridge); the intro talks
-- are unreached (vaelastrasz gossip precedent). JustDied's
-- instance->SetBossState(DONE) is PORTED below (creature:SetBossState
-- bridge live 2026-11-05, moroes/karazhan precedent); the ev1
-- IN_PROGRESS (base-first JustEngagedWith) and ev23 NOT_STARTED
-- (_Reset, alive + != DONE gated) legs are ported below too.
-- EnterEvadeMode's ClearFlight cast is ported (above); the
-- summons.DespawnAll leg is ported for the ev19-latched sphere
-- subset and the ev19-latched phoenix subset (below); the
-- _DespawnAtEvade leg has no bridge. JustSummoned is PORTED below (ev19): arcane-sphere
-- MoveFollow (MoveFollow bridged, lua_objects.go:543) and
-- flame-strike-trigger dummy-cast + 15s despawn — the stale "no
-- summon/MotionMaster bridges (the_beast precedent)" claim is
-- retracted; the spell summon dispatch is live
-- (creatureEffectSummonType, boss_ai.go:4866).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - EVENT_FLAME_STRIKE now ported (above); stale "no SelectTarget
--   bridge" claim retracted (maiden_of_virtue random-player precedent).
-- - EVENT_SHOCK_BARRIER + EVENT_PYROBLAST: now PORTED (above).
--   Stale claims retracted: "no difficulty bridge" (IsHeroic bridged
--   at lua_creature_events.go:4417, keristrasza crystallize precedent)
--   and "no SelectTarget bridge" (maiden_of_virtue random-player
--   precedent, already in this file). The shock-barrier arm is the
--   heroic-gated leg of JustEngagedWith; pyroblast fires only from
--   its fire arm, one-shot.
-- - Phase-two chain now PORTED (above): PREPARE (Talk + REACT_PASSIVE +
--   AttackStop + StopMoving) -> CENTER_TELEPORT 1s (self-cast 44218) ->
--   GRAVITY_LAPSE 1s (DoCastAOE 44224 as self-cast) -> spheres 4s
--   (3x triggered self-cast 44265) + beam 5s (DoCastAOE 44251 as
--   self-cast) + POWER_FEEDBACK 35s (Talk + DoCastAOE 44232 as self-cast
--   + self-cast 44233) -> PREPARE again 11s (Talk SAY_GRAVITY_LAPSE_2).
-- - SpellHitTarget now PORTED (below, onSpellHitTarget, ev15): the
--   44224 arm (counter-indexed directional teleports 44219-44223 +
--   400ms delayed damage/fly self-casts on the hit player) and the
--   44232 CLEAR_FLIGHT arm (RemoveAura 44227 + raid-mode damage
--   44226/49887 on the hit unit). Prior stay-out rationale retracted
--   2026-11-13: Lua CastSpell routes through the real creature cast
--   path (lua_creature_events.go:954 -> castCreatureSpellInternal),
--   which fans AoE hits out per DBC area-enemy targets and fires ev15
--   once per hit target (boss_ai.go:4268-4290); the teleport effect
--   is implemented for creature casters (creatureEffectTeleportUnits,
--   boss_ai.go:5360, DBC-gated like every effect leg); player:CastSpell
--   (lua_player.go:3501) and player/creature RemoveAura (lua_player.go:
--   4329, lua_creature_events.go:1761) are bridged. Two guards carry
--   the C++-vs-Go delta: the self-cast wire target (the boss) is also
--   reported as a hit, so self-hits are skipped (C++ never hits the
--   boss with either spell; gruul wire-target precedent), and the
--   teleport index is guarded at < 5 (C++ array has 5 entries; pets
--   can push hits past it — C++ reads OOB, UB). The counter resets
--   in onGravityLapse (C++ :340) and on Reset (Initialize, :140).
-- - JustSummoned now PORTED (below, onJustSummoned, ev19): the
--   arcane-sphere MoveFollow arm (random player in 70yd via this
--   file's randomTargetInRange picker; spheres latched per-guid for
--   the DespawnEntry/DespawnAll legs) and the flame-strike dummy-cast
--   + 15s despawn arm. Stale claims retracted: "no SPELL_EFFECT_SUMMON
--   dispatch" (creatureEffectSummonType, boss_ai.go:4866, fires ev19
--   via the summonTempCreature funnel) and "no summon/MotionMaster
--   bridges" (MoveFollow bridged, lua_objects.go:543).
-- - npc_felblood_kaelthas_phoenix (second CreatureScript class in
--   the file) is PORTED in npc_felblood_kaelthas_phoenix.lua
--   (entry 24674): IsSummonedBy (ev5) + lethal DamageTaken egg
--   sequence (ev9) + SummonedCreatureDies (ev21) + HATCH/REBIRTH/
--   PREPARE timer chain; the :422 cross-AI JustSummoned call and
--   the :459 GetCreature-by-GUID leg are resolved per that file's
--   header (default-leg bookkeeping + stored egg object).
-- - spell_felblood_kaelthas_flame_strike AuraScript (AfterEffect-
--   Remove -> dummy 44191): AuraScript not modeled (standing
--   blocker).

local SPELL_FIREBALL = 44189
local SPELL_PHOENIX = 44194
local SPELL_EMOTE_TALK_EXCLAMATION = 48348
local SPELL_EMOTE_POINT = 48349
local SPELL_EMOTE_ROAR = 48350
local SPELL_QUITE_SUICIDE = 3617
local SPELL_FLAME_STRIKE = 46162
local SPELL_SHOCK_BARRIER = 46165
local SPELL_PYROBLAST = 36819
local SPELL_CLEAR_FLIGHT = 44232
local SPELL_GRAVITY_LAPSE_CENTER_TELEPORT = 44218
local SPELL_GRAVITY_LAPSE_INITIAL = 44224
local SPELL_GRAVITY_LAPSE_BEAM_VISUAL = 44251
local SPELL_SUMMON_ARCANE_SPHERE = 44265
local SPELL_FLAME_STRIKE_DUMMY = 44191 -- boss_felblood_kaelthas.cpp:70
local NPC_ARCANE_SPHERE = 24708 -- magisters_terrace.h:55
local NPC_FLAME_STRIKE = 24666 -- magisters_terrace.h:56
local NPC_PHOENIX = 24674 -- magisters_terrace.h
local SPELL_POWER_FEEDBACK = 44233
local SPELL_GRAVITY_LAPSE_FLY = 44227
local SPELL_GRAVITY_LAPSE_DAMAGE = 44226
local SPELL_GRAVITY_LAPSE_DAMAGE_HEROIC = 49887
local GRAVITY_LAPSE_TELEPORT_SPELLS = { 44219, 44220, 44221, 44222, 44223 }

local SAY_GRAVITY_LAPSE_1 = 2
local SAY_GRAVITY_LAPSE_2 = 3
local SAY_POWER_FEEDBACK = 4
local SAY_FLAME_STRIKE = 7
local SAY_SUMMON_PHOENIX = 5
local SAY_ANNOUNCE_PYROBLAST = 6
local SAY_DEATH = 8

local ENTRY_KAELTHAS = 24664

local timers = {}
local phaseTwo = {}
local outroFired = {}
local gravityLapseCount = {}
local arcaneSpheres = {} -- per-guid latched arcane spheres (C++ summons list subset for entry 24708; no DespawnEntry bridge, corenSisters precedent)
local phoenixes = {} -- per-guid latched phoenixes (C++ summons list subset for entry 24674; same DespawnEntry-less twin as spheres)

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

local function randomTargetInRange(creature, range)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - creature:GetCombatReach() - 1.5 < range then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ summons.DespawnEntry(NPC_ARCANE_SPHERE) (:357) / DespawnAll
-- (:172 evade, :185 lethal outro, _Reset on reset) twin for the
-- latched-sphere subset: no DespawnEntry bridge exists in engine/,
-- so the ev19-latched spheres are despawned directly. The phoenix
-- half of DespawnAll runs on the ev19-latched phoenixes below
-- (despawnPhoenixes; the egg itself is owned by the phoenix file,
-- not this list — see its header).
local function despawnArcaneSpheres(guid)
    local spheres = arcaneSpheres[guid]
    if spheres then
        for _, sphere in ipairs(spheres) do
            if sphere then
                sphere:Despawn()
            end
        end
        arcaneSpheres[guid] = nil
    end
end

-- C++ summons.DespawnAll twin for the latched-phoenix subset (same
-- DespawnEntry-less pattern as the spheres). The egg is not latched
-- here — it is summoned by the phoenix, so ev19 fires on the
-- phoenix, not Kael'thas; the phoenix file despawns its own egg.
local function despawnPhoenixes(guid)
    local list = phoenixes[guid]
    if list then
        for _, phoenix in ipairs(list) do
            if phoenix then
                phoenix:Despawn()
            end
        end
        phoenixes[guid] = nil
    end
end

-- C++ boss_felblood_kaelthas::JustSummoned (:251-268): summons.Summon
-- bookkeeping + entry switch. Arcane sphere: random player within
-- 70yd -> MoveFollow(target, 0, 0) (MoveFollow bridged,
-- lua_objects.go:543; randomTargetInRange is this file's threat-list
-- player picker, flame-strike precedent). Flame strike: self-cast
-- dummy 44191 + DespawnOrUnsummon(15s) (bridged,
-- lua_creature_events.go:7335, ms). Phoenix: latched per-guid for
-- the DespawnAll legs (C++ summons.Summon twin); no AI arm beyond
-- the bookkeeping (default: break). The summon dispatch itself is the
-- creature cast path's creatureEffectSummonType (boss_ai.go:4866,
-- called :3580, DBC spellKnown-gated) firing ev19 through the shared
-- summonTempCreature funnel (lua_creature_events.go:190) — the old
-- "no SPELL_EFFECT_SUMMON dispatch" claim is stale.
local function onJustSummoned(_, creature, summon)
    if not summon then
        return
    end
    local entry = summon:GetEntry()
    if entry == NPC_ARCANE_SPHERE then
        local target = randomTargetInRange(creature, 70)
        if target then
            summon:MoveFollow(target, 0, 0)
        end
        local guid = creature:GetGUIDLow()
        local list = arcaneSpheres[guid]
        if not list then
            list = {}
            arcaneSpheres[guid] = list
        end
        list[#list + 1] = summon
    elseif entry == NPC_FLAME_STRIKE then
        summon:CastSpell(summon, SPELL_FLAME_STRIKE_DUMMY)
        summon:DespawnOrUnsummon(15000)
    elseif entry == NPC_PHOENIX then
        local guid = creature:GetGUIDLow()
        local list = phoenixes[guid]
        if not list then
            list = {}
            phoenixes[guid] = list
        end
        list[#list + 1] = summon
    end
end

-- Gravity-lapse chain (boss_felblood_kaelthas.cpp:327-357). Cyclic
-- (PREPARE -> CENTER -> LAPSE -> POWER_FEEDBACK -> PREPARE), so the
-- handlers are forward-declared and plain-assigned (tharon_ja /
-- gal_darah idiom — never re-local-function, malown time-bomb).
local onCenterTeleport, onGravityLapse, onSummonArcaneSphere, onBeamVisual, onPowerFeedback, onPrepareGravityLapse

-- C++ EVENT_PREPARE_GRAVITY_LAPSE (:327-333): Talk + REACT_PASSIVE +
-- AttackStop + MotionMaster Clear + CENTER_TELEPORT 1s. MotionMaster
-- Clear has no exact bridge; StopMoving (Unit::StopMoving twin) halts
-- any current movement — the observable remainder for a stationary
-- caster. Shared by the 50% latch (SAY_GRAVITY_LAPSE_1) and the repeat
-- arm (SAY_GRAVITY_LAPSE_2).
local function prepareArms(creature, guid, say)
    creature:Talk(say)
    creature:SetReactState(0)
    creature:AttackStop()
    creature:StopMoving()
    schedule(guid, "gravity_center", 1000, function() onCenterTeleport(creature, guid) end)
end

-- C++ EVENT_GRAVITY_LAPSE_CENTER_TELEPORT (:335-337): self-cast 44218,
-- GRAVITY_LAPSE 1s.
onCenterTeleport = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gravity_center", 500, function() onCenterTeleport(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_GRAVITY_LAPSE_CENTER_TELEPORT)
    schedule(guid, "gravity_lapse", 1000, function() onGravityLapse(creature, guid) end)
end

-- C++ EVENT_GRAVITY_LAPSE (:339-345): reset _gravityLapseTargetCount
-- (:340) + DoCastAOE 44224 as self-cast (DoCastAOE convention) +
-- spheres 4s + beam 5s + power feedback 35s. The counter reset lands
-- here, ahead of the cast, so the ev15 arm (onSpellHitTarget, below)
-- indexes from 0 per lapse cycle.
onGravityLapse = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gravity_lapse", 500, function() onGravityLapse(creature, guid) end)
        return
    end
    gravityLapseCount[guid] = 0
    creature:CastSpell(creature, SPELL_GRAVITY_LAPSE_INITIAL)
    schedule(guid, "gravity_spheres", 4000, function() onSummonArcaneSphere(creature, guid) end)
    schedule(guid, "gravity_beam", 5000, function() onBeamVisual(creature, guid) end)
    schedule(guid, "gravity_feedback", 35000, function() onPowerFeedback(creature, guid) end)
end

-- C++ EVENT_SUMMON_ARCANE_SPHERE (:349-351): 3x triggered self-cast
-- 44265. The spheres spawn via the creature cast path's summon
-- dispatch (boss_ai.go:4866) and reach onJustSummoned (ev19) below,
-- which runs the MoveFollow arm and latches them for the power
-- feedback / evade / outro / reset despawn legs.
onSummonArcaneSphere = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gravity_spheres", 500, function() onSummonArcaneSphere(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_ARCANE_SPHERE, true)
    creature:CastSpell(creature, SPELL_SUMMON_ARCANE_SPHERE, true)
    creature:CastSpell(creature, SPELL_SUMMON_ARCANE_SPHERE, true)
end

-- C++ EVENT_GRAVITY_LAPSE_BEAM_VISUAL_PERIODIC (:346-348): DoCastAOE
-- 44251 as self-cast, single-shot (C++ never re-arms it).
onBeamVisual = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gravity_beam", 500, function() onBeamVisual(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_GRAVITY_LAPSE_BEAM_VISUAL)
end

-- C++ EVENT_POWER_FEEDBACK (:353-357): Talk + DoCastAOE 44232 as
-- self-cast + self-cast 44233 + summons.DespawnEntry(24708) + PREPARE
-- again 11s. The DespawnEntry leg runs on the ev19-latched spheres
-- (despawnArcaneSpheres above, C++ order: after the casts).
onPowerFeedback = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gravity_feedback", 500, function() onPowerFeedback(creature, guid) end)
        return
    end
    creature:Talk(SAY_POWER_FEEDBACK)
    creature:CastSpell(creature, SPELL_CLEAR_FLIGHT)
    creature:CastSpell(creature, SPELL_POWER_FEEDBACK)
    despawnArcaneSpheres(guid)
    schedule(guid, "gravity_prepare", 11000, function() onPrepareGravityLapse(creature, guid) end)
end

onPrepareGravityLapse = function(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "gravity_prepare", 500, function() onPrepareGravityLapse(creature, guid) end)
        return
    end
    prepareArms(creature, guid, SAY_GRAVITY_LAPSE_2)
end

local function onFireball(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireball", 500, function() onFireball(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 2500, function() onFireball(creature, guid) end)
end

local function onPhoenix(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "phoenix", 500, function() onPhoenix(creature, guid) end)
        return
    end
    creature:Talk(SAY_SUMMON_PHOENIX)
    creature:CastSpell(creature, SPELL_PHOENIX)
    schedule(guid, "phoenix", 45000, function() onPhoenix(creature, guid) end)
end

local function onFlameStrike(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flame_strike", 500, function() onFlameStrike(creature, guid) end)
        return
    end
    creature:Talk(SAY_FLAME_STRIKE)
    local target = randomTargetInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_FLAME_STRIKE)
    end
    schedule(guid, "flame_strike", 44000, function() onFlameStrike(creature, guid) end)
end

-- C++ EVENT_PYROBLAST (cpp:254-258): SelectTarget(Random, 0,
-- 40.0f, true) -> randomTargetInRange(creature, 40): threat-list
-- players alive <=40; cast 36819, one-shot: C++ schedules it
-- once from the shock-barrier arm, no repeat.
local function onPyroblast(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "pyroblast", 500, function() onPyroblast(creature, guid) end)
        return
    end
    local target = randomTargetInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_PYROBLAST)
    end
end

-- C++ EVENT_SHOCK_BARRIER (cpp:246-253), heroic-only via
-- JustEngagedWith's IsHeroic() gate (cpp:151): Talk(SAY_ANNOUNCE_
-- PYROBLAST) + self-cast 46165 + RescheduleEvent(EVENT_FIREBALL,
-- 2s500ms) — schedule() re-arm = EventMap cancel-on-reschedule —
-- + ScheduleEvent(EVENT_PYROBLAST, 2s), repeat 1min.
local function onShockBarrier(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shock_barrier", 500, function() onShockBarrier(creature, guid) end)
        return
    end
    creature:Talk(SAY_ANNOUNCE_PYROBLAST)
    creature:CastSpell(creature, SPELL_SHOCK_BARRIER)
    schedule(guid, "fireball", 2500, function() onFireball(creature, guid) end)
    schedule(guid, "pyroblast", 2000, function() onPyroblast(creature, guid) end)
    schedule(guid, "shock_barrier", 60000, function() onShockBarrier(creature, guid) end)
end

-- C++ outro events execute in the same gated UpdateAI loop
-- (:277), so each outro step defers under its own key while
-- the boss is casting (mal_ganis idiom).
local function onOutroStep(creature, guid, key, spell)
    if creature:IsCasting() then
        schedule(guid, key, 500, function() onOutroStep(creature, guid, key, spell) end)
        return
    end
    creature:CastSpell(creature, spell)
end

local function onEnterCombat(_, creature)
    creature:SetMeleeAttack(false) -- boss_felblood_kaelthas.cpp:131: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
    gravityLapseCount[guid] = nil
    arcaneSpheres[guid] = nil
    -- BossAI::JustEngagedWith base-first (boss_felblood_kaelthas.cpp:144-146
    -- -> ScriptedCreature.cpp:527-542): SetBossState IN_PROGRESS, then
    -- pulse 5 + DoZoneInCombat.
    creature:SetBossState(3, 1) -- DATA_KAELTHAS_SUNSTRIDER, IN_PROGRESS
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    schedule(guid, "fireball", 1, function() onFireball(creature, guid) end)
    schedule(guid, "flame_strike", 44000, function() onFlameStrike(creature, guid) end)
    schedule(guid, "phoenix", 12000, function() onPhoenix(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "shock_barrier", 61000, function() onShockBarrier(creature, guid) end)
    end
end

-- C++ boss_felblood_kaelthas::EnterEvadeMode (:168):
-- DoCastAOE(SPELL_CLEAR_FLIGHT 44232, true) TRIGGERED, ungated — modeled
-- as triggered self-cast (the in-tree CastSpell(..., true) idiom). The
-- summons.DespawnAll leg (:172) runs on the ev19-latched spheres
-- (despawnArcaneSpheres above) and phoenixes (despawnPhoenixes
-- below); the _DespawnAtEvade leg has no bridge.
-- Event 2 gets a dedicated handler (the urom split precedent) so the cast
-- fires on evade only, not on death/reset.
local function onLeaveCombat(_, creature)
    local guid = creature:GetGUIDLow()
    creature:CastSpell(creature, SPELL_CLEAR_FLIGHT, true)
    despawnArcaneSpheres(guid)
    despawnPhoenixes(guid)
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
    gravityLapseCount[guid] = nil
end

-- C++ JustDied (boss_felblood_kaelthas.cpp:162-166): explicit
-- instance->SetBossState(DATA_KAELTHAS_SUNSTRIDER, DONE) — NO _JustDied()
-- call (the comment: it would re-trigger the death sequence). The outro
-- chain already ran on lethal DamageTaken; the DONE write lands here.
local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
    gravityLapseCount[guid] = nil
    creature:SetBossState(3, 3) -- DATA_KAELTHAS_SUNSTRIDER, DONE
end

-- C++ Reset (boss_felblood_kaelthas.cpp:155-160): _Reset() ->
-- SetBossState(NOT_STARTED), alive-gated and != DONE-gated
-- (ScriptedCreature.cpp:499-511); Initialize() clears the gravity-lapse
-- counters, mirrored by the per-guid flag clears. Go fires ev23 on the
-- death path before ev4 (maiden precedent), so both gates are live here.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
    gravityLapseCount[guid] = nil
    -- C++ Reset -> _Reset() -> summons.DespawnAll: alive-gated like the
    -- boss-state write below (Go fires ev23 on the death path before
    -- ev4, and C++ spheres persist past death — JustDied :162-166 calls
    -- no _JustDied — so a dead-path reset leaves the latch for the later
    -- live reset to consume).
    if creature:IsAlive() then
        despawnArcaneSpheres(guid)
        despawnPhoenixes(guid)
    end
    if creature:IsAlive() and creature:GetBossState(3) ~= 3 then
        creature:SetBossState(3, 0) -- DATA_KAELTHAS_SUNSTRIDER, NOT_STARTED
    end
end

local function onDamageTaken(_, creature, attacker, damage)
    local guid = creature:GetGUIDLow()
    local health = creature:GetHealth()
    local newDamage = damage
    if not outroFired[guid] and damage >= health then
        -- C++ checks lethal damage first; the 50% latch is skipped once
        -- the outro phase is set.
        outroFired[guid] = true
        cancelTimers(guid)
        -- C++ boss_felblood_kaelthas.cpp:181-182: me->AttackStop() then
        -- me->SetReactState(REACT_PASSIVE) (outro arm order: AttackStop ->
        -- flip -> InterruptNonMeleeSpells(true) -> RemoveAurasDueToSpell
        -- (POWER_FEEDBACK), both ported below).
        creature:AttackStop()
        creature:SetReactState(0) -- REACT_PASSIVE
        creature:InterruptNonMeleeSpells(true)
        creature:RemoveAura(SPELL_POWER_FEEDBACK)
        despawnArcaneSpheres(guid) -- C++ :185 summons.DespawnAll, sphere subset
        despawnPhoenixes(guid) -- C++ :185 summons.DespawnAll, phoenix subset
        creature:CastSpell(creature, SPELL_CLEAR_FLIGHT)
        creature:Talk(SAY_DEATH)
        schedule(guid, "outro_1", 1000, function() onOutroStep(creature, guid, "outro_1", SPELL_EMOTE_TALK_EXCLAMATION) end)
        schedule(guid, "outro_2", 3800, function() onOutroStep(creature, guid, "outro_2", SPELL_EMOTE_POINT) end)
        schedule(guid, "outro_3", 7400, function() onOutroStep(creature, guid, "outro_3", SPELL_EMOTE_ROAR) end)
        schedule(guid, "outro_4", 10000, function() onOutroStep(creature, guid, "outro_4", SPELL_EMOTE_ROAR) end)
        schedule(guid, "outro_5", 11000, function() onOutroStep(creature, guid, "outro_5", SPELL_QUITE_SUICIDE) end)
    elseif not outroFired[guid] and not phaseTwo[guid]
        and health - damage < math.floor(creature:GetMaxHealth() * 50 / 100) then
        phaseTwo[guid] = true
        cancelTimers(guid)
        prepareArms(creature, guid, SAY_GRAVITY_LAPSE_1)
    end
    -- C++ DamageTaken tail: non-self lethal damage is clamped to
    -- health-1 so he survives for the 11s outro chain (self-damage may
    -- still kill him). Runs on every blow, including repeat blows after
    -- the outro fired (curator damage-rewrite precedent).
    if damage >= health and attacker ~= creature then
        newDamage = health - 1
    end
    return false, newDamage
end

-- C++ boss_felblood_kaelthas::SpellHitTarget (:221-249): fires once per
-- hit target of 44224 (gravity lapse) or 44232 (clear flight). Event 15
-- (CREATURE_EVENT_ON_SPELL_HIT_TARGET) fires once per hit target from
-- the creature cast path (boss_ai.go:4268-4290, AoE fan-out per DBC
-- area-enemy targets); the self-cast wire target (the boss) is also
-- reported, so self-hits are skipped — C++ never hits the boss with
-- either spell (gruul wire-target precedent). The nil-target guard
-- mirrors C++ target->ToUnit() (fireCreatureSpellHitTarget passes nil
-- for an unresolvable target, boss_ai.go:4499-4511).
local function onSpellHitTarget(_, creature, target, spellId)
    if not target then
        return
    end
    if target:GetGUIDLow() == creature:GetGUIDLow() then
        return
    end
    if spellId == SPELL_GRAVITY_LAPSE_INITIAL then
        local guid = creature:GetGUIDLow()
        local count = gravityLapseCount[guid] or 0
        -- C++ gravityLapseTeleportSpells[] has 5 entries; a 5-man caps
        -- players at 5, but pets can push hits past it (C++ reads OOB —
        -- UB). Guard the index; the count still increments (C++-exact).
        if count < 5 then
            creature:CastSpell(target, GRAVITY_LAPSE_TELEPORT_SPELLS[count + 1], true)
        end
        gravityLapseCount[guid] = count + 1
        local damageSpell = SPELL_GRAVITY_LAPSE_DAMAGE
        if creature:IsHeroic() then
            damageSpell = SPELL_GRAVITY_LAPSE_DAMAGE_HEROIC
        end
        -- C++ target->m_Events.AddEventAtOffset 400ms (:233-238): the hit
        -- player casts the damage + fly spells on itself (non-triggered).
        -- Untracked in the boss timers (the C++ event lives on the
        -- target, not the boss, so it outlives evade/death like C++).
        CreateLuaEvent(function()
            target:CastSpell(target, damageSpell)
            target:CastSpell(target, SPELL_GRAVITY_LAPSE_FLY)
        end, 400)
    elseif spellId == SPELL_CLEAR_FLIGHT then
        local damageSpell = SPELL_GRAVITY_LAPSE_DAMAGE
        if creature:IsHeroic() then
            damageSpell = SPELL_GRAVITY_LAPSE_DAMAGE_HEROIC
        end
        target:RemoveAura(SPELL_GRAVITY_LAPSE_FLY)
        target:RemoveAura(damageSpell)
    end
end

RegisterCreatureEvent(ENTRY_KAELTHAS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_KAELTHAS, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_KAELTHAS, 4, onDied)
RegisterCreatureEvent(ENTRY_KAELTHAS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_KAELTHAS, 15, onSpellHitTarget)
RegisterCreatureEvent(ENTRY_KAELTHAS, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_KAELTHAS, 23, onReset)
