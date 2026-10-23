-- Halycon (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_halycon.cpp (boss_halyconAI — BossAI combat
-- scheduler). Boss-script unit per eastern_kingdoms_script_loader.cpp
-- order (boss_drakkisath done, registered for 10363; AddSC_boss_halycon
-- next in the Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_HALYCON = 10220 in the BRS creatures enum. The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace), but the
-- entry itself is C++-verifiable so the port is registered
-- (boss_drakkisath / boss_moira_bronzebeard precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132); the AI has no instance arms — Reset()
-- _Reset() and Initialize() are internal machinery covered here by the
-- cancel/re-arm on combat events.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine + JustDied talk):
-- EVENT_REND: victim-cast REND 13738, {17s,20s} init -> {8s,10s} loop
-- (C++ ScheduleEvent ranges; CreateLuaEvent {min,max} table is the
-- randtime idiom, maiden convention).
-- EVENT_THRASH: self-cast THRASH 3391, {10s,12s} one-shot init (C++ ScheduleEvent
-- 10s,12s, boss_halycon.cpp:68) — C++ does NOT re-arm the timer, so the
-- one-shot is C++-exact (creature:CastSpell(creature, spell) is the
-- DoCast(me, spell) path, boss_twilight_corrupter convention).
-- JustDied: me->SummonCreature(NPC_GIZRUL_THE_SLAVENER = 10268,
-- SummonLocation {-167.9561,-411.7844,76.23057,1.53589},
-- TEMPSUMMON_TIMED_DESPAWN, 5min) then Talk(EMOTE_DEATH = 0), C++ arm
-- order (boss_halycon.cpp:71-74; najentus precedent for the talk leg).
-- The Summoned member is write-only in C++ (Initialize sets false,
-- JustDied sets true, never read) — no Lua state needed.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature casts
-- are packet-visual), so timers fire unconditionally (maiden
-- precedent). The C++ scheduler is driven from JustEngagedWith and
-- drained by UpdateAI; the port schedules from OnEnterCombat(1) and
-- cancels on 2/4/23 (maiden convention). The C++ Reset() _events.Reset()
-- is covered by the cancel/re-arm on combat events.

local SPELL_REND = 13738
local SPELL_THRASH = 3391
local EMOTE_DEATH = 0

local ENTRY_HALYCON = 10220
local ENTRY_GIZRUL_THE_SLAVENER = 10268

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

local function onRend(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_REND)
    end
    schedule(guid, "rend", {8000, 10000}, function() onRend(creature, guid) end)
end

local function onThrash(creature)
    creature:CastSpell(creature, SPELL_THRASH)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "rend", {17000, 20000}, function() onRend(creature, guid) end)
    schedule(guid, "thrash", {10000, 12000}, function() onThrash(creature) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:SummonCreature(ENTRY_GIZRUL_THE_SLAVENER, -167.9561, -411.7844, 76.23057, 1.53589, 0, 300000)
    creature:Talk(EMOTE_DEATH)
end

RegisterCreatureEvent(ENTRY_HALYCON, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_HALYCON, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_HALYCON, 4, onDied)
RegisterCreatureEvent(ENTRY_HALYCON, 23, onCombatEnd)
