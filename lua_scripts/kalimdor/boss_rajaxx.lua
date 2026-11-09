-- Ruins of Ahn'Qiraj: General Rajaxx — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_rajaxx.cpp
-- (boss_rajaxxAI : public BossAI(creature, DATA_RAJAXX);
-- GetAI via GetAQ20AI<boss_rajaxxAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_rajaxx at end
-- registers the boss script + spell_rajaxx_thundercrash;
-- kalimdor loader decl 80 / call 193 per
-- kalimdor_script_loader.cpp — second "// Ruins of ahn'qiraj" loader
-- group, after kurinnaxx, before moam).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_rajaxx" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:42 names NPC_RAJAXX = 15341; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- UpdateAI CASTING gates PORTED (own UpdateAI pre-loop :92 + in-loop :111 returns, boss_rajaxx.cpp): each arm defers 500ms while creature:IsCasting() (live CastingUntil window, lua_creature_events.go:1950), no re-arm consumed; defer precedes victim pick (disarm) and self-cast (thunder crash).
-- Ported arms (the self-contained in-combat legs):
-- - Engage: arm each timer at its C++ ScheduleEvent cooldown (no Talk
--   in C++ — boss_rajaxxAI has zero Talk() calls; the Yells enum
--   belongs to the Andorov event NPCs, unused here).
-- - Disarm 6713 on the victim (C++ DoCastVictim, non-triggered),
--   init 10s -> 22s.
-- - Thunder Crash 25599 self-cast (C++ DoCast(me), non-triggered),
--   init 12s -> 21s.
-- Unmodeled (documented-only, no bridges):
-- - spell_rajaxx_thundercrash (SpellScript, HandleDamageCalc:
--   damage = GetHitUnit()->GetHealth()/2, min 200) — no SpellScript
--   bridge (standing).
-- - EVENT_CHANGE_AGGRO (enum value 3): declared but never scheduled
--   or handled in C++ UpdateAI — dead event, nothing to port.
-- - The `enraged` bool: initialized in Initialize() but never read
--   or set in C++ — dead variable, nothing to port.
-- - BossAI ctor leg DATA_RAJAXX (ruins_of_ahnqiraj.h:29 = 1) and
--   _Reset() — PORTED: ev1 IN_PROGRESS + ev23 NOT_STARTED legs
--   (see onEnterCombat / onReset); ev4 DONE via base BossAI::JustDied
--   (ScriptedCreature.h:330), see onDied.
local ENTRY = 15341
local DATA_RAJAXX = 1 -- ruins_of_ahnqiraj.h:29
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SPELL_DISARM = 6713
local SPELL_THUNDERCRASH = 25599

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

-- C++ EVENT_DISARM: DoCastVictim(6713), non-triggered, init 10s -> 22s.
local function onDisarm(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "disarm", 500, function()
            onDisarm(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DISARM)
    end
    schedule(guid, "disarm", 22000, function()
        onDisarm(creature, guid)
    end)
end

-- C++ EVENT_THUNDERCRASH: DoCast(me, 25599), non-triggered,
-- init 12s -> 21s.
local function onThunderCrash(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "thundercrash", 500, function()
            onThunderCrash(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_THUNDERCRASH)
    schedule(guid, "thundercrash", 21000, function()
        onThunderCrash(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    -- C++ BossAI::_JustEngagedWith (ScriptedCreature.cpp:527-542; no
    -- JustEngagedWith override in boss_rajaxx.cpp, so the base runs):
    -- instance->SetBossState(DATA_RAJAXX, IN_PROGRESS) ahead of the
    -- pulse / DoZoneInCombat arms below (creature:SetBossState,
    -- lua_creature_events.go:5917). The CheckRequiredBosses gate is
    -- vacuous (no instance override in instance_ruins_of_ahnqiraj.cpp).
    creature:SetBossState(DATA_RAJAXX, BOSS_STATE_IN_PROGRESS)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542)
    schedule(guid, "disarm", 10000, function()
        onDisarm(creature, guid)
    end)
    schedule(guid, "thundercrash", 12000, function()
        onThunderCrash(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset _Reset() leg (boss_rajaxx.cpp:79;
    -- BossAI::_Reset ScriptedCreature.cpp:499-511: alive-gated and
    -- != DONE-gated; Go fires ev23 on the death path before ev4,
    -- moroes/algalon precedent, so both gates are live here).
    if creature:IsAlive() and creature:GetBossState(DATA_RAJAXX) ~= BOSS_STATE_DONE then
        creature:SetBossState(DATA_RAJAXX, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ has no JustDied override for boss_rajaxxAI, so base
-- BossAI::JustDied (ScriptedCreature.h:330) runs _JustDied() ->
-- SetBossState(DATA_RAJAXX, DONE) (ScriptedCreature.cpp:513-520);
-- the mal_ganis precedent, not the old "cancel only" reading.
local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(DATA_RAJAXX, BOSS_STATE_DONE)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
