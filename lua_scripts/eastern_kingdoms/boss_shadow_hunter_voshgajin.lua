-- Shadow Hunter Vosh'gajin (Blackrock Spire; entrance encounter) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_shadow_hunter_voshgajin.cpp
-- (boss_shadowvoshAI — BossAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_overlord_wyrmthalak
-- done, registered for 9568; AddSC_boss_shadowvosh next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_SHADOW_HUNTER_VOSHGAJIN = 9236 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_overlord_wyrmthalak / boss_mother_smolderweb
-- precedent). GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper
-- only (blackrock_spire.h:129-132). SetBossState legs (bridge live,
-- lua_creature_events.go:5917): DATA_SHADOW_HUNTER_VOSHGAJIN = 1
-- (blackrock_spire.h:31; EncounterCount 23, so the write is live) —
-- JustEngagedWith calls the base FIRST
-- (boss_shadow_hunter_voshgajin.cpp:58), so BossAI::_JustEngagedWith
-- writes IN_PROGRESS (ScriptedCreature.cpp:537); Reset's _Reset()
-- (boss_shadow_hunter_voshgajin.cpp:51-55) writes NOT_STARTED gated
-- on alive + state != DONE (ScriptedCreature.cpp:499-511); JustDied's
-- _JustDied() (boss_shadow_hunter_voshgajin.cpp:65-68) writes DONE
-- (ScriptedCreature.cpp:513-520). See onEnterCombat/onReset/onDied.
-- Reset()'s DoCast(me, SPELL_ICEARMOR, true) is commented out in C++, so the
-- port omits it by design (moira precedent — EVENT_HEAL unused).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_CURSE_OF_BLOOD: victim-cast CURSEOFBLOOD 24673, 2s init -> 45s
-- loop (maiden convention — DoCastVictim path,
-- creature:GetVictim()).
-- EVENT_CLEAVE: victim-cast CLEAVE 20691, 14s init -> 7s loop (maiden
-- convention).
-- EVENT_HEX: random-player HEX 16708 — SelectTarget(Random, 0, 100,
-- true) is playerOnly, ported via the threat-list pool (GetAITargets,
-- alive, within 100yd); 8s init -> 15s
-- loop, re-arm unconditional (C++ ScheduleEvent sits outside the target
-- check), non-triggered DoCast(target).
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:76) + in-loop
-- gate (:98) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:1950; mal_ganis idiom) — a gated arm
-- fires nothing and consumes no re-arm, staying due until the cast
-- ends. The in-loop gate's same-tick co-expiry suppression has no
-- timer-arm model (independent callbacks, AV precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_CURSEOFBLOOD = 24673
local SPELL_CLEAVE = 20691
local SPELL_HEX = 16708

local ENTRY_SHADOW_HUNTER_VOSHGAJIN = 9236

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

local function onCurseOfBlood(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "curseofblood", 500, function() onCurseOfBlood(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSEOFBLOOD)
    end
    schedule(guid, "curseofblood", 45000, function() onCurseOfBlood(creature, guid) end)
end

local function onCleave(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cleave", 500, function() onCleave(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 7000, function() onCleave(creature, guid) end)
end

-- C++ SelectTarget(Random, 0, 100, true) (boss_shadow_hunter_voshgajin.cpp:88):
-- threat-list players alive within range (UnitAI.h:162-176).
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead()
                and creature:GetDistance(plr) - creature:GetCombatReach() - 1.5 < range then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5)
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_HEX: SelectTarget(Random, 0, 100, true) playerOnly,
-- DoCast(target, 16708) non-triggered, 8s init, ScheduleEvent(15s)
-- outside the target check (re-arm unconditional).
local function onHex(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "hex", 500, function() onHex(creature, guid) end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_HEX)
    end
    schedule(guid, "hex", 15000, function() onHex(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_shadow_hunter_voshgajin.cpp:58 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_shadow_hunter_voshgajin.cpp:58 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_SHADOW_HUNTER_VOSHGAJIN=1 (blackrock_spire.h:31).
    creature:SetBossState(1, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "curseofblood", 2000, function() onCurseOfBlood(creature, guid) end)
    schedule(guid, "cleave", 14000, function() onCleave(creature, guid) end)
    schedule(guid, "hex", 8000, function() onHex(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_shadow_hunter_voshgajin.cpp:51-55): _Reset() writes
-- NOT_STARTED gated on alive + state != DONE (ScriptedCreature.cpp:
-- 499-511). Alive-gated: Go fires ev23 on the death path too, and
-- C++ Reset never runs on death.
local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() and creature:GetBossState(1) ~= 3 then
        creature:SetBossState(1, 0)
    end
end

-- C++ JustDied (boss_shadow_hunter_voshgajin.cpp:65-68): _JustDied()
-- writes DONE (ScriptedCreature.cpp:513-520).
local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(1, 3)
end

RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 4, onDied)
RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 23, onReset)
