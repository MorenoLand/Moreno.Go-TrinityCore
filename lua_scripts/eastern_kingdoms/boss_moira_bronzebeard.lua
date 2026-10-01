-- Moira Bronzebeard (Blackrock Depths; Lyceum) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockDepths/boss_moira_bronzebeard.cpp (boss_moira_bronzebeardAI —
-- ScriptedAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_moira_bronzebeard
-- follows boss_magmus; the BRD block continues with boss_tomb_of_seven,
-- coren_direbrew).
-- Entry (verifiable from the C++ sources): instance_blackrock_depths.cpp
-- names NPC_MOIRA = 8929 in the BRD instance creatures enum (line 45).
-- The creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_magmus / boss_draganthaurissan / npc_phalanx
-- precedent).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_MINDBLAST: victim-cast MINDBLAST 10947, 16s init -> 14s;
-- EVENT_SHADOW_WORD_PAIN: victim-cast SHADOWWORDPAIN 10894, 2s init ->
-- 18s; EVENT_SMITE: victim-cast SMITE 10934, 8s init -> 10s.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally (maiden
-- precedent). The C++ scheduler is driven from JustEngagedWith and
-- drained by UpdateAI; the port schedules from OnEnterCombat(1) and
-- cancels on 2/4/23 (maiden convention). The C++ Reset() _events.Reset()
-- is covered by the cancel/re-arm on combat events.
-- Not ported (commented out in C++, "not used atm"): EVENT_HEAL — the
-- _events.ScheduleEvent(EVENT_HEAL, 12s) line is disabled in C++ and
-- SPELL_HEAL 10917 / SPELL_RENEW 10929 / SPELL_SHIELD 10901 have no live
-- arms in the AI.

local SPELL_MINDBLAST = 10947
local SPELL_SHADOWWORDPAIN = 10894
local SPELL_SMITE = 10934

local ENTRY_MOIRA = 8929

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

local function onMindBlast(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MINDBLAST)
    end
    schedule(guid, "mindblast", 14000, function() onMindBlast(creature, guid) end)
end

local function onShadowWordPain(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWWORDPAIN)
    end
    schedule(guid, "shadowwordpain", 18000, function() onShadowWordPain(creature, guid) end)
end

local function onSmite(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SMITE)
    end
    schedule(guid, "smite", 10000, function() onSmite(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "mindblast", 16000, function() onMindBlast(creature, guid) end)
    schedule(guid, "shadowwordpain", 2000, function() onShadowWordPain(creature, guid) end)
    schedule(guid, "smite", 8000, function() onSmite(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_MOIRA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MOIRA, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MOIRA, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_MOIRA, 23, onCombatEnd)
