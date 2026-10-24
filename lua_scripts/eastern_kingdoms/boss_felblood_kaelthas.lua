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
-- Entry (verifiable from the C++ sources): magisters_terrace.h:50
-- names BOSS_KAELTHAS_SUNSTRIDER = 24664 in the MT creatures enum.
-- Script name: no explicit CreatureScript string in the file —
-- AddSC registers via RegisterMagistersTerraceCreatureAI(
-- boss_felblood_kaelthas) = RegisterCreatureAIWithFactory, which
-- stringifies the type name (ScriptMgr.h:1234), so "boss_felblood_
-- kaelthas". The creature_template ScriptName binding is DB-side (no
-- TDB in this workspace), but the entry itself is C++-verifiable so
-- the port is registered (boss_vaelastrasz precedent).
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
-- EVENT_QUITE_SUICIDE 11s / DATA_KAELTHAS_SUNSTRIDER (boss id).
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
--   12s init -> 45s loop (the phoenix add's own AI is unbridgeable
--   — see below; the summon spell itself is real).
-- - EVENT_FLAME_STRIKE: Talk(SAY_FLAME_STRIKE) + random-player
--   (SelectTarget(Random, 0, 40.0f, true) — GetPlayersInWorld + map/
--   instance/alive/IsWithinDist filter, maiden_of_virtue precedent)
--   cast 46162, 44s init -> 44s loop.
-- - EVENT_SHOCK_BARRIER (heroic-only): OnEnterCombat schedules it
--   61s only under creature:IsHeroic() (C++-exact JustEngagedWith
--   gate, lua_creature_events.go:260; the "no difficulty bridge"
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
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so timers fire unconditionally and the in-loop
-- casting gate is dropped (maiden precedent). The C++ scheduler is
-- driven from JustEngagedWith and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden
-- convention). The outro's damage cap (lethal non-self damage is
-- clamped to health-1 so the boss survives at 1 HP through the
-- death sequence), AttackStop / InterruptNonMelee-
-- Spells / RemoveAuras / summons.DespawnAll / DoCastAOE(CLEAR_
-- FLIGHT) machinery has no bridges (the REACT_PASSIVE arm is PORTED
-- — creature:SetReactState, 2026-10-20); the outro chain plays as the
-- observable remnant (documented).
-- The intro is SetData(DATA_
-- KAELTHAS_INTRO)-gated (instance arm — no bridge); the intro talks
-- are unreached (vaelastrasz gossip precedent). JustDied's
-- instance->SetBossState(DONE) has no bridge (instance precedent).
-- EnterEvadeMode's ClearFlight cast is ported (above); the
-- summons.DespawnAll + _DespawnAtEvade legs have no bridge.
-- JustSummoned's arcane-sphere MoveFollow and
-- flame-strike-trigger dummy-cast arms have no summon/MotionMaster
-- bridges (the_beast precedent).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - EVENT_FLAME_STRIKE now ported (above); stale "no SelectTarget
--   bridge" claim retracted (maiden_of_virtue random-player precedent).
-- - EVENT_SHOCK_BARRIER + EVENT_PYROBLAST: now PORTED (above).
--   Stale claims retracted: "no difficulty bridge" (IsHeroic bridged
--   at lua_creature_events.go:260, keristrasza crystallize precedent)
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
--   Still out, with exact blockers: the SpellHitTarget arm (44224 hits ->
--   counter-indexed directional teleports 44219-44223 + 400ms delayed
--   damage/fly self-casts on player targets) — no SpellHitTarget bridge
--   and Go dispatches no spell-effect teleports; the arcane-sphere
--   MoveFollow arms — spheres never spawn (no SPELL_EFFECT_SUMMON
--   dispatch), so summons.DespawnEntry(24708) is vacuous; the
--   RemoveAurasDueToSpell legs — the CLEAR_FLIGHT cast is ported but
--   aura removal on players is engine-side and absent.
-- - npc_felblood_kaelthas_phoenix (second CreatureScript class in
--   the file): IsSummonedBy's DoZoneInCombat + self-casts 44197/
--   44196, the lethal DamageTaken egg sequence (DoSummon(NPC_
--   PHOENIX_EGG 24675) + instance->GetCreature(DATA_KAELTHAS_
--   SUNSTRIDER)->AI()->JustSummoned + ember blast 44199 + damage
--   clamped to health-1), EVENT_HATCH_FROM_EGG (ObjectAccessor egg
--   despawn), EVENT_REBIRTH (self-cast 44196), EVENT_PREPARE_
--   REENGAGE (self-cast 17683 + 44197) — every arm gates on the
--   summon / instance / creature-list / zone-in-combat bridges
--   absent from the Lua surface (razorgore / go_suppression_device
--   precedent); no file written.
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
local SPELL_POWER_FEEDBACK = 44233

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
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, range) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
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
    creature:CastSpell(creature, SPELL_GRAVITY_LAPSE_CENTER_TELEPORT)
    schedule(guid, "gravity_lapse", 1000, function() onGravityLapse(creature, guid) end)
end

-- C++ EVENT_GRAVITY_LAPSE (:339-345): DoCastAOE 44224 as self-cast
-- (DoCastAOE convention) + spheres 4s + beam 5s + power feedback 35s.
-- The _gravityLapseTargetCount counter is write-only here — its only
-- consumer is the unported SpellHitTarget arm (see header).
onGravityLapse = function(creature, guid)
    creature:CastSpell(creature, SPELL_GRAVITY_LAPSE_INITIAL)
    schedule(guid, "gravity_spheres", 4000, function() onSummonArcaneSphere(creature, guid) end)
    schedule(guid, "gravity_beam", 5000, function() onBeamVisual(creature, guid) end)
    schedule(guid, "gravity_feedback", 35000, function() onPowerFeedback(creature, guid) end)
end

-- C++ EVENT_SUMMON_ARCANE_SPHERE (:349-351): 3x triggered self-cast
-- 44265. The spheres never spawn in Go (no SPELL_EFFECT_SUMMON
-- dispatch) — the casts are the packet-visual remainder.
onSummonArcaneSphere = function(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_ARCANE_SPHERE, true)
    creature:CastSpell(creature, SPELL_SUMMON_ARCANE_SPHERE, true)
    creature:CastSpell(creature, SPELL_SUMMON_ARCANE_SPHERE, true)
end

-- C++ EVENT_GRAVITY_LAPSE_BEAM_VISUAL_PERIODIC (:346-348): DoCastAOE
-- 44251 as self-cast, single-shot (C++ never re-arms it).
onBeamVisual = function(creature, guid)
    creature:CastSpell(creature, SPELL_GRAVITY_LAPSE_BEAM_VISUAL)
end

-- C++ EVENT_POWER_FEEDBACK (:353-357): Talk + DoCastAOE 44232 as
-- self-cast + self-cast 44233 + PREPARE again 11s. The
-- summons.DespawnEntry(24708) leg is vacuous (spheres never spawn).
onPowerFeedback = function(creature, guid)
    creature:Talk(SAY_POWER_FEEDBACK)
    creature:CastSpell(creature, SPELL_CLEAR_FLIGHT)
    creature:CastSpell(creature, SPELL_POWER_FEEDBACK)
    schedule(guid, "gravity_prepare", 11000, function() onPrepareGravityLapse(creature, guid) end)
end

onPrepareGravityLapse = function(creature, guid)
    prepareArms(creature, guid, SAY_GRAVITY_LAPSE_2)
end

local function onFireball(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIREBALL)
    end
    schedule(guid, "fireball", 2500, function() onFireball(creature, guid) end)
end

local function onPhoenix(creature, guid)
    creature:Talk(SAY_SUMMON_PHOENIX)
    creature:CastSpell(creature, SPELL_PHOENIX)
    schedule(guid, "phoenix", 45000, function() onPhoenix(creature, guid) end)
end

local function onFlameStrike(creature, guid)
    creature:Talk(SAY_FLAME_STRIKE)
    local target = randomTargetInRange(creature, 40)
    if target then
        creature:CastSpell(target, SPELL_FLAME_STRIKE)
    end
    schedule(guid, "flame_strike", 44000, function() onFlameStrike(creature, guid) end)
end

-- C++ EVENT_PYROBLAST (cpp:254-258): random-player
-- (SelectTarget(Random, 0, 40.0f, true) — maiden_of_virtue
-- random-player precedent) cast 36819, one-shot: C++ schedules it
-- once from the shock-barrier arm, no repeat.
local function onPyroblast(creature, guid)
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
    creature:Talk(SAY_ANNOUNCE_PYROBLAST)
    creature:CastSpell(creature, SPELL_SHOCK_BARRIER)
    schedule(guid, "fireball", 2500, function() onFireball(creature, guid) end)
    schedule(guid, "pyroblast", 2000, function() onPyroblast(creature, guid) end)
    schedule(guid, "shock_barrier", 60000, function() onShockBarrier(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetMeleeAttack(false) -- boss_felblood_kaelthas.cpp:131: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
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
-- summons.DespawnAll / _DespawnAtEvade legs have no bridge — documented.
-- Event 2 gets a dedicated handler (the urom split precedent) so the cast
-- fires on evade only, not on death/reset.
local function onLeaveCombat(_, creature)
    local guid = creature:GetGUID()
    creature:CastSpell(creature, SPELL_CLEAR_FLIGHT, true)
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    phaseTwo[guid] = nil
    outroFired[guid] = nil
end

local function onDamageTaken(_, creature, attacker, damage)
    local guid = creature:GetGUID()
    local health = creature:GetHealth()
    local newDamage = damage
    if not outroFired[guid] and damage >= health then
        -- C++ checks lethal damage first; the 50% latch is skipped once
        -- the outro phase is set.
        outroFired[guid] = true
        cancelTimers(guid)
        -- C++ boss_felblood_kaelthas.cpp:181-182: me->AttackStop() then
        -- me->SetReactState(REACT_PASSIVE) (outro arm order: AttackStop ->
        -- flip -> InterruptNonMeleeSpells, interrupt unbridged).
        creature:AttackStop()
        creature:SetReactState(0) -- REACT_PASSIVE
        creature:CastSpell(creature, SPELL_CLEAR_FLIGHT)
        creature:Talk(SAY_DEATH)
        schedule(guid, "outro_1", 1000, function()
            creature:CastSpell(creature, SPELL_EMOTE_TALK_EXCLAMATION)
        end)
        schedule(guid, "outro_2", 3800, function()
            creature:CastSpell(creature, SPELL_EMOTE_POINT)
        end)
        schedule(guid, "outro_3", 7400, function()
            creature:CastSpell(creature, SPELL_EMOTE_ROAR)
        end)
        schedule(guid, "outro_4", 10000, function()
            creature:CastSpell(creature, SPELL_EMOTE_ROAR)
        end)
        schedule(guid, "outro_5", 11000, function()
            creature:CastSpell(creature, SPELL_QUITE_SUICIDE)
        end)
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

RegisterCreatureEvent(ENTRY_KAELTHAS, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_KAELTHAS, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_KAELTHAS, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_KAELTHAS, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_KAELTHAS, 23, onCombatEnd)
