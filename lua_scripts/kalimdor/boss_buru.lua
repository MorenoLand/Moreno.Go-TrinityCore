-- Ruins of Ahn'Qiraj: Buru the Gorger — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_buru.cpp
-- (boss_buruAI : public BossAI(creature, DATA_BURU);
-- GetAI via GetAQ20AI<boss_buruAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_buru at end
-- registers boss_buru + npc_buru_egg + spell_egg_explosion;
-- kalimdor loader decl 82 / call 195 per
-- kalimdor_script_loader.cpp — fourth "// Ruins of ahn'qiraj" loader
-- group, after moam, before ayamiss).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_buru" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:44 names NPC_BURU = 15370; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage (event 1, buru target convention): Talk(EMOTE_TARGET 0,
--   target), self-cast THORNS 25640, schedule DISMEMBER 5s,
--   GATHERING_SPEED 9s, FULL_SPEED 60s; phase = PHASE_EGG.
-- - EVENT_DISMEMBER: DoCastVictim 96 (jeklik GetVictim + CastSpell
--   convention), reschedule 5s.
-- - EVENT_GATHERING_SPEED: self-cast 1834, reschedule 9s.
-- - EVENT_FULL_SPEED: self-cast 1557, one-shot (C++ does not
--   reschedule it).
-- - Transform (event 9 damage leg, C++ UpdateAI's per-tick
--   GetHealthPct() < 20 check): phase EGG -> self-cast
--   BURU_TRANSFORM 24721, self-cast FULL_SPEED 1557 (triggered in
--   C++ — kurinnaxx sand-trap triggered-flag convention),
--   RemoveAura(THORNS), phase = PHASE_TRANSFORM.
-- - OnTargetDied (event 3, C++ KilledUnit): player victim ->
--   ChaseNewVictim in full, C++ order, if phase EGG: RemoveAura
--   FULL_SPEED + GATHERING_SPEED and reschedule both (9s / 60s),
--   then random alive player from GetAITargets (azuregos idiom) ->
--   ResetAllThreat -> EngageWithTarget (AttackStart twin) ->
--   Talk(EMOTE_TARGET 0, victim). No switch when the pool is empty
--   (C++ SelectTarget nullptr leg).
-- Unmodeled (documented-only, no bridges):
-- - npc_buru_egg JustDied (boss_buru.cpp:225-233) — PORTED 2026-11-07
--   (eggDied below): entry 15514 Wowhead-verified (browser,
--   wowhead.com/classic/npc=15514/buru-egg — "Egg Explosion" +
--   "Summon Hive'Zara Hatchling" match C++ 19593 + 1881; no
--   NPC_BURU_EGG constant exists in ruins_of_ahnqiraj.h, the entry
--   is DB-side via ScriptName "npc_buru_egg"). The three death-cast
--   arms port in C++ order (DoCastAOE = self-cast per convention,
--   all triggered); the ManageRespawn leg stays out (cross-AI
--   dynamic_cast to boss_buruAI, no bridge — the Eggs GuidList +
--   EVENT_RESPAWN_EGG scheduling with it).
-- - npc_buru_egg's cross-AI legs: JustSummoned PORTED 2026-11-13
--   (eggJustSummoned below — the header's old "no bridge" claim for it
--   is stale: GetInstanceCreature + the ev5 SetGuidData latch + ev19
--   all exist now). JustEngagedWith (boss_buru.cpp:213-216, Buru
--   AttackStart when Buru not in combat) stays out — the IsInCombat
--   read needs a bridge check first (follow-up, not this tick).
-- - spell_egg_explosion (SpellScript): the distance-scaled dummy
--   damage arm is PORTED below (RegisterSpellEvent 19593 ev5); the
--   AfterCast arm (FindNearestCreature Buru at 5yd -> DoAction
--   ACTION_EXPLODE) is PORTED below as a fold into eggDied — the
--   AfterCast hook exists in Go (SpellEventAfterCast=9,
--   runtime.go:717), but the egg
--   is the sole 19593 caster in the tree and the 19593 cast
--   resolves synchronously, so the fold runs at AfterCast position
--   (one DoAction: the script binds 19593; the 5255 binding is
--   DB-side and unverifiable in-tree).
-- - ACTION_EXPLODE's DealDamage(me, me, 45000): PORTED below via
--   the event-38 handler (creature:DealDamage bridge,
--   lua_creature_events.go:1754).
-- - EVENT_CREEPING_PLAGUE: never scheduled in C++ (only rescheduled
--   in its own handler) — dead code, C++-verbatim not ported.
-- - EVENT_RESPAWN_EGG: driven by the unbridged Eggs GuidList
--   (ManageRespawn) — no bridge.
-- - EnterEvadeMode egg respawn loop: cross-AI, no bridge.
-- - BossAI ctor leg DATA_BURU (ruins_of_ahnqiraj.h:31 = 3) and
--   _Reset() — PORTED: ev1 IN_PROGRESS + ev23 NOT_STARTED legs
--   (see onEnterCombat / onReset); ev4 DONE via base BossAI::JustDied
--   (ScriptedCreature.h:330), see onDied.
local ENTRY = 15370
local DATA_BURU = 3 -- ruins_of_ahnqiraj.h:31
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SPELL_DISMEMBER = 96
local SPELL_GATHERING_SPEED = 1834
local SPELL_FULL_SPEED = 1557
local SPELL_THORNS = 25640
local SPELL_BURU_TRANSFORM = 24721

local ENTRY_BURU_EGG = 15514 -- Wowhead-verified 2026-11-07, see header
local ENTRY_HATCHLING = 15521 -- ruins_of_ahnqiraj.h:52, NPC_HATCHLING
local SPELL_EXPLODE = 19593 -- boss_buru.cpp:39
local SPELL_EXPLODE_2 = 5255 -- boss_buru.cpp:40
local SPELL_SUMMON_HATCHLING = 1881 -- boss_buru.cpp:38

local ACTION_EXPLODE = 0 -- boss_buru.cpp:61

local PHASE_EGG = 0
local PHASE_TRANSFORM = 1

local timers = {}
local buruState = {}

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

-- C++ EVENT_DISMEMBER: DoCastVictim(96), reschedule 5s.
local function onDismember(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISMEMBER)
    end
    schedule(guid, "dismember", 5000, function()
        onDismember(creature, guid)
    end)
end

-- C++ EVENT_GATHERING_SPEED: DoCast(me, 1834), reschedule 9s.
local function onGatheringSpeed(creature, guid)
    creature:CastSpell(creature, SPELL_GATHERING_SPEED)
    schedule(guid, "gathering", 9000, function()
        onGatheringSpeed(creature, guid)
    end)
end

-- C++ EVENT_FULL_SPEED: DoCast(me, 1557), one-shot.
local function onFullSpeed(creature, guid)
    creature:CastSpell(creature, SPELL_FULL_SPEED)
end

local function armSpeedTimers(creature, guid)
    schedule(guid, "gathering", 9000, function()
        onGatheringSpeed(creature, guid)
    end)
    schedule(guid, "fullspeed", 60000, function()
        onFullSpeed(creature, guid)
    end)
end

-- C++ UpdateAI transform leg: GetHealthPct() < 20 && phase EGG ->
-- transform, FULL_SPEED (triggered in C++), remove THORNS, phase
-- TRANSFORM. Checked on damage (event 9); event 9 fires pre-damage
-- so the subtraction mirrors the post-damage C++ read.
local function checkTransform(creature, guid, damage)
    local state = buruState[guid]
    if state == nil or state.phase ~= PHASE_EGG then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    -- C++ arm is UpdateAI post-damage; no tick follows the killing
    -- blow, so the arm never fires on lethal damage (nefarian rule).
    local postDamage = creature:GetHealth() - damage
    if postDamage <= 0 then
        return
    end
    if postDamage * 100 / maxHealth < 20 then
        state.phase = PHASE_TRANSFORM
        creature:CastSpell(creature, SPELL_BURU_TRANSFORM)
        creature:CastSpell(creature, SPELL_FULL_SPEED, true)
        creature:RemoveAura(SPELL_THORNS)
    end
end

local function onEnterCombat(event, creature, target)
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-542,
    -- base-first in boss_buru.cpp:89): instance->SetBossState
    -- (DATA_BURU, IN_PROGRESS) ahead of the pulse / DoZoneInCombat
    -- arms below (creature:SetBossState, lua_creature_events.go:5917).
    -- The CheckRequiredBosses gate is vacuous (no instance override
    -- in instance_ruins_of_ahnqiraj.cpp).
    creature:SetBossState(DATA_BURU, BOSS_STATE_IN_PROGRESS)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    buruState[guid] = { phase = PHASE_EGG }
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    if target then
        creature:Talk(0, target)
    end
    creature:CastSpell(creature, SPELL_THORNS)
    schedule(guid, "dismember", 5000, function()
        onDismember(creature, guid)
    end)
    armSpeedTimers(creature, guid)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    buruState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    buruState[guid] = { phase = PHASE_EGG }
    -- C++ Reset: no Reset() override in boss_buru.cpp, so base
    -- BossAI::Reset (ScriptedCreature.h:328) runs _Reset()
    -- (ScriptedCreature.cpp:499-511: alive-gated and != DONE-gated;
    -- Go fires ev23 on the death path before ev4, moroes/algalon
    -- precedent, so both gates are live here).
    if creature:IsAlive() and creature:GetBossState(DATA_BURU) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_BURU, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ KilledUnit: player victim -> ChaseNewVictim in full (C++
-- boss_buru.cpp:113-127 order): aura strip + speed-timer rearm,
-- then random alive player from GetAITargets -> ResetAllThreat ->
-- EngageWithTarget -> Talk(EMOTE_TARGET 0, victim).
local function onTargetDied(event, creature, victim)
    if victim == nil or victim:GetObjectType() ~= "Player" then
        return
    end
    local guid = creature:GetGUIDLow()
    local state = buruState[guid]
    if state == nil or state.phase ~= PHASE_EGG then
        return
    end
    creature:RemoveAura(SPELL_FULL_SPEED)
    creature:RemoveAura(SPELL_GATHERING_SPEED)
    armSpeedTimers(creature, guid)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return
    end
    local target = candidates[math.random(#candidates)]
    creature:ResetAllThreat()
    creature:EngageWithTarget(target)
    creature:Talk(0, target)
end

local function onDamageTaken(event, creature, attacker, damage)
    checkTransform(creature, creature:GetGUIDLow(), damage)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    buruState[guid] = nil
    -- C++ has no JustDied override for boss_buruAI (the JustDied at
    -- boss_buru.cpp:225 belongs to npc_buru_eggAI), so base
    -- BossAI::JustDied (ScriptedCreature.h:330) runs _JustDied() ->
    -- SetBossState(DATA_BURU, DONE) (ScriptedCreature.cpp:513-520);
    -- the mal_ganis precedent.
    creature:SetBossState(DATA_BURU, BOSS_STATE_DONE)
end

-- C++ npc_buru_eggAI::JustDied (boss_buru.cpp:225-233), C++ order:
-- DoCastAOE(SPELL_EXPLODE, true) + DoCastAOE(SPELL_EXPLODE_2, true)
-- + DoCast(me, SPELL_SUMMON_HATCHLING, true). DoCastAOE is self-cast
-- per convention; all three casts are triggered in C++. The
-- ManageRespawn leg stays out (cross-AI dynamic_cast, no bridge).
-- C++ spell_egg_explosion::HandleAfterCast (boss_buru.cpp:254-258)
-- folds in here at AfterCast position: the 19593 cast resolves
-- synchronously, so the FindNearestCreature(NPC_BURU, 5.0f) +
-- DoAction(ACTION_EXPLODE) runs after the 19593 cast and before
-- the 5255 cast, in C++ order (one DoAction — see header).
local function eggDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_EXPLODE, true)
    local buru = creature:FindNearestCreature(ENTRY, 5)
    if buru ~= nil then
        buru:DoAction(ACTION_EXPLODE)
    end
    creature:CastSpell(creature, SPELL_EXPLODE_2, true)
    creature:CastSpell(creature, SPELL_SUMMON_HATCHLING, true)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY_BURU_EGG, 4, eggDied)

-- C++ instance OnCreatureCreate latching (instance_ruins_of_ahnqiraj.cpp
-- :36/:85, DATA_BURU) has no Go equivalent — the instance guid store is
-- Lua-populated (instance_guid_store.go header), so Buru latches its own
-- GUID from ev5 OnSpawn (fired by motionFor on first spawn and respawn),
-- the mirror of the C++ create leg. Consumed by eggJustSummoned below.
local function onSpawn(event, creature)
    creature:SetGuidData(DATA_BURU)
end

RegisterCreatureEvent(ENTRY, 5, onSpawn)

-- C++ npc_buru_eggAI::JustSummoned (boss_buru.cpp:217-223): summoned
-- entry == NPC_HATCHLING (15521) -> Buru via instance
-- GetGuidData(DATA_BURU) -> SelectTarget(Random) from Buru's threat
-- list -> hatchling AttackStart(target). The ev19 fire reaches the egg
-- because the hatchling summon (SPELL_SUMMON_HATCHLING 1881, cast in
-- eggDied) resolves through the creature spell-summon funnels
-- (creatureEffectSummonType boss_ai.go:4866 / creatureEffectSummonPet
-- :5138), both routing via summonTempCreature with SummonerMotion, whose
-- ev19 fire is lua_creature_events.go:190. GetInstanceCreature is the
-- ObjectAccessor::GetCreature twin (lua_creature_events.go:~7075);
-- EngageWithTarget is the AttackStart twin (ayamiss swarmer precedent).
-- C++ SelectTarget(Random) with no args takes any threat-list unit, so
-- the pool is alive-only, not player-only (onSwarmerAttack precedent).
local function eggJustSummoned(event, creature, summoned)
    if summoned:GetEntry() ~= ENTRY_HATCHLING then
        return
    end
    local buru = creature:GetInstanceCreature(DATA_BURU)
    if buru == nil then
        return
    end
    local candidates = {}
    for _, unit in ipairs(buru:GetAITargets()) do
        if not unit:IsDead() then
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates > 0 then
        summoned:EngageWithTarget(candidates[math.random(#candidates)])
    end
end

RegisterCreatureEvent(ENTRY_BURU_EGG, 19, eggJustSummoned)

-- C++ boss_buruAI::DoAction(ACTION_EXPLODE) (boss_buru.cpp:100-105):
-- phase EGG only -> Unit::DealDamage(me, me, 45000). Reached via
-- the egg-explosion AfterCast fold in eggDied (creature:DoAction
-- fires event 38 on the entry, razorgore precedent); the phase
-- gate is the C++ _phase check, read from the Lua buruState.
local function buruDoAction(event, creature, action)
    if action ~= ACTION_EXPLODE then
        return
    end
    local state = buruState[creature:GetGUIDLow()]
    if state == nil or state.phase ~= PHASE_EGG then
        return
    end
    creature:DealDamage(creature, 45000)
end

RegisterCreatureEvent(ENTRY, 38, buruDoAction)

-- C++ spell_egg_explosion::HandleDummyHitTarget
-- (boss_buru.cpp:260-264): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY — per hit unit, DealDamage(caster, target,
-- -16 * caster distance to target + 500). The loader binds spell
-- 19593 (SPELL_EXPLODE, boss_buru.cpp:39 — the egg's JustDied
-- DoCastAOE above, same enum-ID evidence standard as the
-- zone_blasted_lands 27686 port). Distance is the 3D center
-- distance (C++ WorldObject::GetDistance; Lua GetDistance object
-- form, scripting/object.go:282, no reach subtraction). C++
-- converts the float to uint32 (DealDamage, Unit.h:939), so
-- truncation toward zero; the 0 clamp mirrors that inside the
-- spell radius — beyond 31.25yd the C++ float goes negative and
-- the uint32 conversion wraps, unreachable while the DBC radius
-- holds targets inside it. DORMANT until Spell.dbc ships 19593.
local function eggExplosionDummy(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local damage = math.floor(500 - 16 * caster:GetDistance(target))
    if damage < 0 then
        damage = 0
    end
    caster:DealDamage(target, damage)
end

RegisterSpellEvent(SPELL_EXPLODE, 5, eggExplosionDummy)
