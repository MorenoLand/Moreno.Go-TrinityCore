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
--   C++; Lua CastSpell has no triggered flag — cast as normal),
--   RemoveAura(THORNS), phase = PHASE_TRANSFORM.
-- - OnTargetDied (event 3, C++ KilledUnit): player victim ->
--   ChaseNewVictim's bridgeable legs — if phase EGG, RemoveAura
--   FULL_SPEED + GATHERING_SPEED and reschedule both (9s / 60s).
--   The SelectTarget/ResetThreatList/AttackStart/Talk legs have no
--   Lua bridge (no usage anywhere in lua_scripts) — documented below.
-- Unmodeled (documented-only, no bridges):
-- - ChaseNewVictim's target switch (SelectTarget Random +
--   ResetThreatList + AttackStart + Talk): no Lua bridge for any of
--   the three calls.
-- - npc_buru_egg (JustEngagedWith / JustSummoned / JustDied):
--   cross-AI via instance GetGuidData(DATA_BURU) + dynamic_cast to
--   boss_buruAI + ManageRespawn — no bridge.
-- - spell_egg_explosion (SpellScript: ACTION_EXPLODE on Buru at 5yd
--   + distance-scaled dummy damage): no SpellScript bridge.
-- - ACTION_EXPLODE's DealDamage(me, me, 45000): no bridge; also only
--   reachable via the unbridged SpellScript.
-- - EVENT_CREEPING_PLAGUE: never scheduled in C++ (only rescheduled
--   in its own handler) — dead code, C++-verbatim not ported.
-- - EVENT_RESPAWN_EGG: driven by the unbridged Eggs GuidList
--   (ManageRespawn) — no bridge.
-- - EnterEvadeMode egg respawn loop: cross-AI, no bridge.
-- - BossAI ctor leg DATA_BURU (ruins_of_ahnqiraj.h:31 = 3) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15370

local SPELL_DISMEMBER = 96
local SPELL_GATHERING_SPEED = 1834
local SPELL_FULL_SPEED = 1557
local SPELL_THORNS = 25640
local SPELL_BURU_TRANSFORM = 24721

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
-- transform, FULL_SPEED, remove THORNS, phase TRANSFORM.
-- Checked on damage (event 9) since Lua has no per-tick UpdateAI.
local function checkTransform(creature, guid)
    local state = buruState[guid]
    if state == nil or state.phase ~= PHASE_EGG then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() * 100 / maxHealth < 20 then
        state.phase = PHASE_TRANSFORM
        creature:CastSpell(creature, SPELL_BURU_TRANSFORM)
        creature:CastSpell(creature, SPELL_FULL_SPEED)
        creature:RemoveAura(SPELL_THORNS)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    buruState[guid] = { phase = PHASE_EGG }
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
    local guid = creature:GetGUID()
    cancelTimers(guid)
    buruState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    buruState[guid] = { phase = PHASE_EGG }
end

-- C++ KilledUnit: player victim -> ChaseNewVictim's bridgeable legs
-- (aura strip + speed-timer rearm; target switch has no bridge).
local function onTargetDied(event, creature, victim)
    if victim == nil or not victim:IsPlayer() then
        return
    end
    local guid = creature:GetGUID()
    local state = buruState[guid]
    if state == nil or state.phase ~= PHASE_EGG then
        return
    end
    creature:RemoveAura(SPELL_FULL_SPEED)
    creature:RemoveAura(SPELL_GATHERING_SPEED)
    armSpeedTimers(creature, guid)
end

local function onDamageTaken(event, creature, attacker, damage)
    checkTransform(creature, creature:GetGUID())
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    buruState[guid] = nil
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
