-- Highlord Omokk (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_highlord_omokk.cpp (boss_highlordomokkAI — BossAI
-- combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_halycon done,
-- registered for 10220; AddSC_boss_highlordomokk next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_HIGHLORD_OMOKK = 9196 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_halycon / boss_drakkisath precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132); the AI has no instance arms — Reset()
-- _Reset() is internal machinery covered here by the cancel/re-arm on
-- combat events (halycon precedent). JustDied _JustDied() likewise
-- covered by the cancel on OnDied(4).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FRENZY: victim-cast FRENZY 8269, 20s init -> 1min loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_KNOCK_AWAY: victim-cast KNOCK_AWAY 10101, 18s init -> 12s
-- loop (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (maiden
-- precedent). The C++ scheduler is driven from JustEngagedWith and
-- drained by UpdateAI; the port schedules from OnEnterCombat(1) and
-- cancels on 2/4/23 (maiden convention).

local SPELL_FRENZY = 8269
local SPELL_KNOCK_AWAY = 10101

local ENTRY_HIGHLORD_OMOKK = 9196

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

local function onFrenzy(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRENZY)
    end
    schedule(guid, "frenzy", 60000, function() onFrenzy(creature, guid) end)
end

local function onKnockAway(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCK_AWAY)
    end
    schedule(guid, "knockaway", 12000, function() onKnockAway(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "frenzy", 20000, function() onFrenzy(creature, guid) end)
    schedule(guid, "knockaway", 18000, function() onKnockAway(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 23, onCombatEnd)
