-- Brutallus (Sunwell Plateau)
-- Lua port of src/server/scripts/EasternKingdoms/SunwellPlateau/
-- boss_brutallus.cpp
-- (boss_brutallus : public ScriptedAI (NOT BossAI — Initialize() in
-- the constructor zeroes the 4 diff-timers + the intro phase state
-- + instance = creature->GetInstanceScript(), Intro = true; Reset ->
-- Initialize() + DoCast(me, SPELL_DUAL_WIELD, true) + instance->
-- SetBossState(DATA_BRUTALLUS, NOT_STARTED); JustEngagedWith ->
-- Talk(YELL_AGGRO) + instance->SetBossState(DATA_BRUTALLUS,
-- IN_PROGRESS); KilledUnit -> Talk(YELL_KILL); JustDied ->
-- Talk(YELL_DEATH) + instance->SetBossState(DATA_BRUTALLUS, DONE) +
-- me->SummonCreature(NPC_FELMYST, me pos, z + 30, orientation,
-- TEMPSUMMON_MANUAL_DESPAWN); EnterEvadeMode -> if (!Intro)
-- ScriptedAI::EnterEvadeMode; StartIntro/DoIntro/EndIntro +
-- MoveInLineOfSight/AttackStart intro gates; UpdateAI old-style
-- diff-timer arms + DoMeleeAttackIfReady; GetAI via
-- GetSunwellPlateauAI -> GetInstanceAI) (loader lines 143/321).
-- Entry (verifiable from the C++ sources): sunwell_plateau.h
-- SWPCreatureIds names NPC_BRUTALLUS = 24882, and instance_
-- sunwell_plateau.cpp:51 maps that entry onto DATA_BRUTALLUS in
-- the boss-boundary table — the name-to-entry tie is C++-verified
-- (kalecgos entry-verifiability check).
-- Whole-server-tree grep confirms boss_brutallus.cpp as the only
-- source of the boss AI (loader lines, instance_sunwell_plateau
-- boss-boundary leg, the game/Spells burn-leg comment, and
-- spell_generic.cpp's spell_gen_burn_brutallus leg otherwise).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Driven by CreateLuaEvent timers; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee-
-- AttackIfReady. There is no UNIT_STATE_CASTING gate in Go, so
-- timers fire unconditionally (maiden precedent); the C++ UpdateAI
-- arms map onto OnEnterCombat timers with the C++-exact init
-- delays (maiden convention).
-- Verifiable numbers (file's own enums): SPELL_METEOR_SLASH =
-- 45150 / SPELL_BURN = 46394 / SPELL_STOMP = 45185 / SPELL_
-- BERSERK = 26662 / SPELL_DUAL_WIELD = 42459 (loaded-only on
-- Reset, never cast in combat by the C++ AI — data foundation,
-- not a ported arm) / SPELL_INTRO_FROST_BLAST = 45203 /
-- SPELL_INTRO_FROSTBOLT = 44843 / SPELL_INTRO_ENCAPSULATE =
-- 45665 / SPELL_INTRO_ENCAPSULATE_CHANELLING = 45661.
-- Ported arms (C++ UpdateAI timer arms):
-- - Meteor Slash: 11s init -> 11s re-arm; DoCastVictim(45150)
--   non-triggered -> GetVictim + CastSpell (mr_smite convention).
-- - Stomp: 30s init -> 30s re-arm; Talk(YELL_LOVE = 7);
--   DoCastVictim(45185) non-triggered.
-- - Burn: 60s init -> urand(60s, 180s) {60000, 180000} re-arm
--   (maiden range convention); SelectTarget(Random, 0, 100.0f,
--   true, true, -SPELL_BURN) -> randomTargetInRange (maiden
--   convention; the no-Burn-aura target filter has no Go model,
--   documented approximation — kalecgos AgonyCurseSelector
--   precedent); C++ target->CastSpell(target, 46394, true) ->
--   creature:CastSpell(target, SPELL_BURN, true) (kalecgos
--   convention — caster mismatch noted, documented).
-- - Berserk: 6min (360000) one-shot latch; Talk(YELL_BERSERK =
--   8); DoCast(me, SPELL_BERSERK) non-triggered -> self CastSpell
--   (headless_horseman self-cast precedent); per-guid Enraged
--   latch (kirtonos latch convention), cleared on OnEnterCombat
--   (1) and OnReset(23).
-- - Aggro: Talk(YELL_AGGRO = 5) on event 1.
-- - Death: Talk(YELL_DEATH = 9) on event 4.
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset's DoCast(me, SPELL_DUAL_WIELD, true) + instance->
--   SetBossState(DATA_BRUTALLUS, NOT_STARTED) legs, JustEngaged-
--   With's SetBossState(DATA_BRUTALLUS, IN_PROGRESS) leg,
--   JustDied's SetBossState(DATA_BRUTALLUS, DONE) leg, and the
--   GetSunwellPlateauAI -> GetInstanceAI leg (instance-script
--   model blocked, standing).
-- - JustDied's me->SummonCreature(NPC_FELMYST = 25038, x, y,
--   z + 30, orientation, TEMPSUMMON_MANUAL_DESPAWN) leg: entry
--   25038 IS header-verifiable (NPC_FELMYST = 25038) but me->
--   SummonCreature has no Lua summon bridge (gurtogg class) —
--   STRAND.
-- - KilledUnit's Talk(YELL_KILL = 6) leg: no KilledUnit bridge in
--   engine/scripting (no CreatureEvent id declared) — queued.
-- - The whole Madrigosa intro: StartIntro's instance->Get-
--   Creature(DATA_MADRIGOSA) leg (instance-leg-blocked) +
--   Madrigosa->Respawn / setActive / SetFarVisible / SetMax-
--   Health / SetHealth / SetDisableGravity / setDeathState
--   (CORPSE) legs, me->SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_
--   ATTACKABLE) / me->Attack / Unit::Kill(me, Madrigosa) /
--   SetFacingToObject / AttackStop / SetOrientation / StopMoving
--   legs — none of these have Lua bridges; MoveInLineOfSight's
--   SetBossState(SPECIAL) + StartIntro trigger, AttackStart's
--   intro gate, and EnterEvadeMode's !Intro gate share the
--   block — the intro stays documented-only (not a firesworn
--   stub case — none of the 11 intro phases can run without the
--   instance/model legs).
-- - spell_gen_burn_brutallus (spell_generic.cpp, AuraScript on
--   46394): SpellScript/AuraScript-check-handler-blocked class
--   (standing).

local NPC_BRUTALLUS = 24882

local SPELL_METEOR_SLASH = 45150
local SPELL_BURN = 46394
local SPELL_STOMP = 45185
local SPELL_BERSERK = 26662

local YELL_AGGRO = 5
local YELL_KILL = 6
local YELL_LOVE = 7
local YELL_BERSERK = 8
local YELL_DEATH = 9

local timers = {}
local enraged = {}

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

local function onMeteorSlash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_METEOR_SLASH)
    end
    schedule(guid, "slash", 11000, function() onMeteorSlash(creature, guid) end)
end

local function onStomp(creature, guid)
    creature:Talk(YELL_LOVE)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_STOMP)
    end
    schedule(guid, "stomp", 30000, function() onStomp(creature, guid) end)
end

local function onBurn(creature, guid)
    local target = randomTargetInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_BURN, true)
    end
    schedule(guid, "burn", {60000, 180000}, function() onBurn(creature, guid) end)
end

local function onBerserk(creature, guid)
    if not enraged[guid] then
        enraged[guid] = true
        creature:Talk(YELL_BERSERK)
        creature:CastSpell(creature, SPELL_BERSERK)
    end
end

local function onCombatStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    enraged[guid] = nil
    creature:Talk(YELL_AGGRO)
    schedule(guid, "slash", 11000, function() onMeteorSlash(creature, guid) end)
    schedule(guid, "stomp", 30000, function() onStomp(creature, guid) end)
    schedule(guid, "burn", 60000, function() onBurn(creature, guid) end)
    schedule(guid, "berserk", 360000, function() onBerserk(creature, guid) end)
end

local function onCombatEnd(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    enraged[guid] = nil
end

local function onDied(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    enraged[guid] = nil
    creature:Talk(YELL_DEATH)
end

RegisterCreatureEvent(NPC_BRUTALLUS, 1, onCombatStart)
RegisterCreatureEvent(NPC_BRUTALLUS, 2, onCombatEnd)
RegisterCreatureEvent(NPC_BRUTALLUS, 4, onDied)
RegisterCreatureEvent(NPC_BRUTALLUS, 23, onCombatEnd)
