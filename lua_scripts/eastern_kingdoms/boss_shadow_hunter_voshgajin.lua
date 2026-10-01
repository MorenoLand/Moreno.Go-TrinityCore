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
-- only (blackrock_spire.h:129-132); the AI has no instance arms —
-- Reset() _Reset() is internal machinery covered here by the
-- cancel/re-arm on combat events (halycon precedent). JustDied
-- _JustDied() likewise covered by the cancel on OnDied(4). Reset()'s
-- DoCast(me, SPELL_ICEARMOR, true) is commented out in C++, so the
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
-- Unmodeled: EVENT_HEX — gates on
-- SelectTarget(SelectTargetMethod::Random, 0, 100, true) then
-- DoCast(target, SPELL_HEX=16708); no SelectTarget bridge on the Lua
-- surface (doomwalker / kazzak / doomrel precedent). The hex arm is a
-- random-target mid-fight cast, not the combat trigger, so porting the
-- two victim-cast timers without it does not strand the fight.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_CURSEOFBLOOD = 24673
local SPELL_CLEAVE = 20691

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
    per[key] = CreateLuaEvent(fn, delay)
end

local function onCurseOfBlood(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CURSEOFBLOOD)
    end
    schedule(guid, "curseofblood", 45000, function() onCurseOfBlood(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 7000, function() onCleave(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "curseofblood", 2000, function() onCurseOfBlood(creature, guid) end)
    schedule(guid, "cleave", 14000, function() onCleave(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_SHADOW_HUNTER_VOSHGAJIN, 23, onCombatEnd)
