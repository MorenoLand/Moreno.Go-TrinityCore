-- Ruins of Ahn'Qiraj: Ayamiss the Hunter — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_ayamiss.cpp
-- (boss_ayamissAI : public BossAI(creature, DATA_AYAMISS);
-- GetAI via GetAQ20AI<boss_ayamissAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_ayamiss at end
-- registers boss_ayamiss + npc_hive_zara_larva; kalimdor loader
-- decl 83 / call 196 per kalimdor_script_loader.cpp — fifth
-- "// Ruins of ahn'qiraj" loader group, after buru, before ossirian).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_ayamiss" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:45 names NPC_AYAMISS = 15369; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady (ground phase only in C++; Lua cannot gate
-- melee on phase without a motion bridge — documented below).
-- Ported arms (the self-contained in-combat legs):
-- - Engage: schedule STINGER_SPRAY (20-30s), POISON_STINGER (5s),
--   PARALYZE (15s). (C++ also schedules SUMMON_SWARMER /
--   SWARMER_ATTACK, but SummonCreature has no Lua bridge — zero
--   usage in lua_scripts — so those timers are not armed.)
-- - EVENT_STINGER_SPRAY: self-cast 25749, reschedule 15-20s.
-- - EVENT_POISON_STINGER: DoCastVictim 25748 (jeklik GetVictim +
--   CastSpell convention), reschedule 2-3s; cancelled at ground
--   phase per C++.
-- - EVENT_PARALYZE: random player via GetPlayersInWorld filtered by
--   map/instance/alive (maiden_of_virtue convention — C++
--   SelectTarget Random has no bridge), DoCast 25725, reschedule
--   15s. The larva summon + instance SetGuidData(DATA_PARALYZED)
--   have no bridge — documented below.
-- - Ground phase (event 9 damage leg, C++ UpdateAI's per-tick
--   GetHealthPct() < 70 check): phase AIR -> GROUND, cancel
--   POISON_STINGER, schedule LASH (5-8s) + TRASH (3-6s). The motion
--   legs (MovePoint to victim, SetCanFly/SetCombatMovement,
--   ResetThreatList) have no bridge — documented below.
-- - EVENT_LASH: DoCastVictim 25852, reschedule 8-15s.
-- - EVENT_TRASH: DoCastVictim 3391 (kurinnaxx convention),
--   reschedule 5-7s.
-- - Frenzy (event 9 damage leg, C++ per-tick <20% check): self-cast
--   FRENZY 8269, Talk(EMOTE_FRENZY 0), latched (kurinnaxx enrage
--   convention).
-- Unmodeled (documented-only, no bridges):
-- - Motion: SetCanFly/SetDisableGravity/MovePoint(POINT_AIR) on
--   engage, MovePoint(POINT_GROUND) at phase change,
--   SetCombatMovement, UNIT_STATE_ROOT via MovementInform — no Lua
--   bridge for motion or unit states.
-- - SummonCreature (NPC_SWARMER 15546, NPC_LARVA 15555): no bridge.
-- - EVENT_SWARMER_ATTACK's cross-AI AttackStart on swarmers: no
--   bridge (zero SelectTarget/AttackStart usage in lua_scripts).
-- - npc_hive_zara_larva: MovementInform/AttackStart/MoveInLineOfSight
--   all gated on instance GetBossState(DATA_AYAMISS) — no bridge.
-- - instance->SetGuidData(DATA_PARALYZED, ...) — no bridge.
-- - BossAI ctor leg DATA_AYAMISS (ruins_of_ahnqiraj.h:32 = 4) and
--   _Reset() — instance bridge, standing.
local ENTRY = 15369

local SPELL_STINGER_SPRAY = 25749
local SPELL_POISON_STINGER = 25748
local SPELL_PARALYZE = 25725
local SPELL_TRASH = 3391
local SPELL_FRENZY = 8269
local SPELL_LASH = 25852

local PHASE_AIR = 0
local PHASE_GROUND = 1

local timers = {}
local ayamissState = {}

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

local function cancelTimer(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end

-- C++ EVENT_STINGER_SPRAY: DoCast(me, 25749), init 20-30s -> 15-20s.
local function onStingerSpray(creature, guid)
    creature:CastSpell(creature, SPELL_STINGER_SPRAY)
    schedule(guid, "stinger", math.random(15000, 20000), function()
        onStingerSpray(creature, guid)
    end)
end

-- C++ EVENT_POISON_STINGER: DoCastVictim(25748), init 5s -> 2-3s.
local function onPoisonStinger(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_POISON_STINGER)
    end
    schedule(guid, "poison", math.random(2000, 3000), function()
        onPoisonStinger(creature, guid)
    end)
end

-- C++ EVENT_PARALYZE: SelectTarget Random -> DoCast(25725), 15s.
-- SelectTarget has no bridge; random player via GetPlayersInWorld
-- (maiden_of_virtue convention).
local function onParalyze(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        local target = candidates[math.random(#candidates)]
        creature:CastSpell(target, SPELL_PARALYZE)
    end
    schedule(guid, "paralyze", 15000, function()
        onParalyze(creature, guid)
    end)
end

-- C++ EVENT_LASH: DoCastVictim(25852), init 5-8s -> 8-15s.
local function onLash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_LASH)
    end
    schedule(guid, "lash", math.random(8000, 15000), function()
        onLash(creature, guid)
    end)
end

-- C++ EVENT_TRASH: DoCastVictim(3391), init 3-6s -> 5-7s.
local function onTrash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TRASH)
    end
    schedule(guid, "trash", math.random(5000, 7000), function()
        onTrash(creature, guid)
    end)
end

-- C++ ground-phase leg: GetHealthPct() < 70 in PHASE_AIR ->
-- PHASE_GROUND, cancel POISON_STINGER, arm LASH + TRASH. Checked on
-- damage (event 9); event 9 fires pre-damage so the subtraction
-- mirrors the post-damage C++ read (buru transform convention).
-- Motion/ResetThreatList have no bridge.
local function checkGroundPhase(creature, guid, damage)
    local state = ayamissState[guid]
    if state == nil or state.phase ~= PHASE_AIR then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 70 then
        state.phase = PHASE_GROUND
        cancelTimer(guid, "poison")
        schedule(guid, "lash", math.random(5000, 8000), function()
            onLash(creature, guid)
        end)
        schedule(guid, "trash", math.random(3000, 6000), function()
            onTrash(creature, guid)
        end)
    end
end

-- C++ frenzy leg: !_enraged && GetHealthPct() < 20 -> FRENZY +
-- EMOTE_FRENZY, latched. Checked on damage (event 9); event 9 is
-- pre-damage, so the subtraction mirrors the post-damage C++ read
-- (buru transform convention).
local function checkFrenzy(creature, guid, damage)
    local state = ayamissState[guid]
    if state == nil or state.enraged then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (creature:GetHealth() - damage) * 100 / maxHealth < 20 then
        state.enraged = true
        creature:CastSpell(creature, SPELL_FRENZY)
        creature:Talk(0)
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ayamissState[guid] = { phase = PHASE_AIR, enraged = false }
    schedule(guid, "stinger", math.random(20000, 30000), function()
        onStingerSpray(creature, guid)
    end)
    schedule(guid, "poison", 5000, function()
        onPoisonStinger(creature, guid)
    end)
    schedule(guid, "paralyze", 15000, function()
        onParalyze(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ayamissState[guid] = nil
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ayamissState[guid] = { phase = PHASE_AIR, enraged = false }
end

local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    checkGroundPhase(creature, guid, damage)
    checkFrenzy(creature, guid, damage)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    ayamissState[guid] = nil
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
