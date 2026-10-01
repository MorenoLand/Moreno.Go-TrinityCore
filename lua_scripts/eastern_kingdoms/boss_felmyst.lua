-- Felmyst (Sunwell Plateau)
-- Lua port of src/server/scripts/EasternKingdoms/SunwellPlateau/
-- boss_felmyst.cpp
-- (boss_felmyst : public ScriptedAI (NOT BossAI — Initialize() zeroes
-- phase/uiFlightCount + instance = creature->GetInstanceScript(),
-- uiBreathCount/breathX/breathY in the constructor; Reset ->
-- Initialize() + events.Reset() + SetDisableGravity(true) +
-- SetFloatValue(UNIT_FIELD_BOUNDINGRADIUS, 10) + SetFloatValue(
-- UNIT_FIELD_COMBATREACH, 10) + DespawnSummons(NPC_VAPOR_TRAIL) +
-- me->setActive(false) + instance->SetBossState(DATA_FELMYST,
-- NOT_STARTED); JustEngagedWith -> 10min EVENT_BERSERK +
-- me->setActive(true) + DoZoneInCombat() + DoCast(me, AURA_SUNWELL_
-- RADIANCE, true) + DoCast(me, AURA_NOXIOUS_FUMES, true) +
-- EnterPhase(PHASE_GROUND) + instance->SetBossState(DATA_FELMYST,
-- IN_PROGRESS); AttackStart/MoveInLineOfSight -> phase !=
-- PHASE_FLIGHT gate; KilledUnit -> Talk(YELL_KILL); JustAppeared ->
-- Talk(YELL_BIRTH); JustDied -> Talk(YELL_DEATH) + instance->
-- SetBossState(DATA_FELMYST, DONE); SpellHit(45714) -> Summon-
-- Creature(NPC_DEAD) + charm + DealDamage; JustSummoned(NPC_DEAD)
-- -> AttackStart(random) + DoZoneInCombat + 45415 self-cast;
-- MovementInform -> re-arm EVENT_FLIGHT_SEQUENCE; DamageTaken ->
-- phase != PHASE_GROUND && damage >= health: damage = 0 (no-death
-- latch, flight phase); EnterPhase(PHASE_GROUND) -> CastStop/
-- RemoveAurasDueToSpell(45495) + StopMoving + SetSpeedRate(MOVE_
-- RUN, 2.0f) + schedule CLEAVE/CORROSION/GAS_NOVA/ENCAPSULATE/
-- FLIGHT; EnterPhase(PHASE_FLIGHT) -> SetDisableGravity(true) +
-- EVENT_FLIGHT_SEQUENCE 1s; UpdateAI UpdateVictim + events.Update +
-- IsNonMeleeSpellCast gate; GetAI via GetSunwellPlateauAI ->
-- GetInstanceAI; npc_felmyst_vapor : public ScriptedAI (constructor
-- UNIT_FLAG_NOT_SELECTABLE + SetSpeedRate(MOVE_RUN, 0.8f);
-- JustEngagedWith -> DoZoneInCombat; UpdateAI -> !victim:
-- AttackStart(random in 100)); npc_felmyst_trail : public
-- ScriptedAI (constructor NOT_SELECTABLE + DoCast(me, 45399) +
-- SetTarget(me) + BOUNDINGRADIUS 0.01f; empty UpdateAI)) (loader
-- lines 144/322).
-- Entry (verifiable from the C++ sources): sunwell_plateau.h
-- SWPCreatureIds names NPC_FELMYST = 25038, and instance_
-- sunwell_plateau.cpp:53 maps that entry onto DATA_FELMYST in
-- the boss-boundary table — the name-to-entry tie is C++-verified
-- (kalecgos entry-verifiability check).
-- Whole-server-tree grep confirms boss_felmyst.cpp as the sole
-- source of boss_felmyst / npc_felmyst_vapor / npc_felmyst_trail
-- (loader lines and brutallus's JustDied summon leg only
-- otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 5 OnSpawn, 23 OnReset. Driven by CreateLuaEvent timers;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. There is no UNIT_STATE_CASTING gate in
-- Go, so timers fire unconditionally (maiden precedent); the C++
-- UpdateAI ground-phase arms map onto OnEnterCombat timers with
-- the C++-exact init delays (maiden convention).
-- Verifiable numbers (file's own enums): AURA_SUNWELL_RADIANCE =
-- 45769 / AURA_NOXIOUS_FUMES = 47002 / SPELL_CLEAVE = 19983 /
-- SPELL_CORROSION = 45866 / SPELL_GAS_NOVA = 45855 / SPELL_
-- ENCAPSULATE_CHANNEL = 45661 / SPELL_BERSERK = 45078.
-- Ported arms (C++ ground-phase timer arms):
-- - OnEnterCombat(1): DoCast(me, 45769, true) + DoCast(me, 47002,
--   true) -> creature:CastSpell(creature, ..., true) (headless_
--   horseman self-cast precedent).
-- - Berserk: 10min (600000) init -> 10s re-arm; Talk(YELL_BERSERK
--   = 4); DoCast(me, SPELL_BERSERK, true) -> triggered self-cast.
--   The C++ re-arms EVENT_BERSERK every 10s in the GROUND-phase
--   branch (the FLIGHT-phase branch fires without re-arm); the
--   port models the ground-phase branch C++-exactly.
-- - Cleave: {5s,10s} init -> {5s,10s} re-arm; DoCastVictim(19983)
--   non-triggered -> GetVictim + CastSpell (mr_smite convention).
-- - Corrosion: {10s,20s} init -> {20s,30s} re-arm; DoCastVictim
--   (45866).
-- - Gas Nova: {15s,20s} init -> {20s,25s} re-arm; DoCast(me,
--   45855) non-triggered (AoE) -> self CastSpell.
-- - Encapsulate: {20s,25s} init -> {25s,30s} re-arm; SelectTarget
--   (Random, 0, 150) -> randomTargetInRange(creature, 150)
--   (maiden convention); DoCast(target, 45661) non-triggered ->
--   CastSpell.
-- - Spawn: Talk(YELL_BIRTH = 0) on event 5 (C++ JustAppeared).
-- - Death: Talk(YELL_DEATH = 5) on event 4.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's SetDisableGravity/BoundingRadius/CombatReach/
--   DespawnSummons/setActive legs, JustEngagedWith's setActive/
--   DoZoneInCombat legs, the SetBossState(DATA_FELMYST, *)
--   legs (NOT_STARTED/IN_PROGRESS/DONE), and the
--   GetSunwellPlateauAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).
-- - EVENT_FLIGHT at 1min -> EnterPhase(PHASE_FLIGHT): none of
--   its legs have bridges (SetDisableGravity, the 11-case
--   HandleFlightSequence with MotionMaster Clear/MovePoint,
--   HandleEmoteCommand(LIFTOFF/LAND), SetFacingTo, MoveInLineOf
--   Sight/AttackStart phase gates, EnterEvadeMode fallbacks, the
--   vapor-summon choreography (NPC_VAPOR 25265, 9s despawn,
--   45389 channel, 45411 trigger), the breath run (45495 Fog
--   Breath cast, EVENT_SUMMON_FOG, NPC_VAPOR_TRAIL 25267,
--   45582 trigger / 45782 force / 45714 inform legs),
--   DespawnSummons' trail->NPC_DEAD (25268) conversion); the
--   ported ground arms keep firing on their C++ cadences —
--   registering the 1min timer with only its unbridgeable
--   phase-transition arm would be a firesworn stub.
-- - DamageTaken's phase != PHASE_GROUND no-lethal-damage latch:
--   flight-only arm, moot without the flight model (noted).
-- - KilledUnit's Talk(YELL_KILL = 1) leg: no KilledUnit bridge
--   in engine/scripting (no CreatureEvent id declared) — queued
--   (standing).
-- - SpellHit(45714)'s SummonCreature(NPC_DEAD = 25268, 5s
--   out-of-combat despawn) + SetMaxHealth/SetHealth + double
--   self-charm (45717/45726) + Unit::DealDamage leg: me->
--   SummonCreature has no Lua summon bridge (gurtogg class) —
--   STRAND; event 15 never fires in the Go engine (stratholme
--   finding).
-- - JustSummoned's NPC_DEAD AttackStart/random + DoZoneInCombat
--   + 45415 self-cast arm: shares the no-summon-bridge block.
-- - npc_felmyst_vapor (25265): entry only in the header + this
--   cpp (no corroborating usage — fails the kalecgos entry-
--   verifiability check); the AI itself is target acquisition
--   only (UpdateAI !victim -> AttackStart random in 100) —
--   unregistrable, documented.
-- - npc_felmyst_trail (25267): same entry-verifiability outcome;
--   constructor-only AI (NOT_SELECTABLE + 45399 self-cast +
--   SetTarget + bounding radius) with an empty UpdateAI —
--   unregistrable, documented.

local NPC_FELMYST = 25038

local AURA_SUNWELL_RADIANCE = 45769
local AURA_NOXIOUS_FUMES = 47002

local SPELL_CLEAVE = 19983
local SPELL_CORROSION = 45866
local SPELL_GAS_NOVA = 45855
local SPELL_ENCAPSULATE_CHANNEL = 45661
local SPELL_BERSERK = 45078

local YELL_BIRTH = 0
local YELL_KILL = 1
local YELL_BERSERK = 4
local YELL_DEATH = 5

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

local function onBerserk(creature, guid)
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(guid, "berserk", 10000, function() onBerserk(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", {5000, 10000}, function() onCleave(creature, guid) end)
end

local function onCorrosion(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CORROSION)
    end
    schedule(guid, "corrosion", {20000, 30000}, function() onCorrosion(creature, guid) end)
end

local function onGasNova(creature, guid)
    creature:CastSpell(creature, SPELL_GAS_NOVA)
    schedule(guid, "gasnova", {20000, 25000}, function() onGasNova(creature, guid) end)
end

local function onEncapsulate(creature, guid)
    local target = randomTargetInRange(creature, 150)
    if target then
        creature:CastSpell(target, SPELL_ENCAPSULATE_CHANNEL)
    end
    schedule(guid, "encapsulate", {25000, 30000}, function() onEncapsulate(creature, guid) end)
end

local function onCombatStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, AURA_SUNWELL_RADIANCE, true)
    creature:CastSpell(creature, AURA_NOXIOUS_FUMES, true)
    schedule(guid, "berserk", 600000, function() onBerserk(creature, guid) end)
    schedule(guid, "cleave", {5000, 10000}, function() onCleave(creature, guid) end)
    schedule(guid, "corrosion", {10000, 20000}, function() onCorrosion(creature, guid) end)
    schedule(guid, "gasnova", {15000, 20000}, function() onGasNova(creature, guid) end)
    schedule(guid, "encapsulate", {20000, 25000}, function() onEncapsulate(creature, guid) end)
end

local function onCombatEnd(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
    creature:Talk(YELL_DEATH)
end

local function onSpawn(event, creature)
    creature:Talk(YELL_BIRTH)
end

RegisterCreatureEvent(NPC_FELMYST, 1, onCombatStart)
RegisterCreatureEvent(NPC_FELMYST, 2, onCombatEnd)
RegisterCreatureEvent(NPC_FELMYST, 4, onDied)
RegisterCreatureEvent(NPC_FELMYST, 5, onSpawn)
RegisterCreatureEvent(NPC_FELMYST, 23, onCombatEnd)
