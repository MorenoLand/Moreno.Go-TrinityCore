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
-- (blackrock_spire.h:129-132). SetBossState legs (bridge live,
-- lua_creature_events.go:5917): DATA_HIGHLORD_OMOKK = 0
-- (blackrock_spire.h:30; EncounterCount 23, so the write is live) —
-- JustEngagedWith calls the base FIRST (boss_highlord_omokk.cpp:55),
-- so BossAI::_JustEngagedWith writes IN_PROGRESS
-- (ScriptedCreature.cpp:537); Reset's _Reset() (boss_highlord_omokk.
-- cpp:51-54) writes NOT_STARTED gated on alive + state != DONE
-- (ScriptedCreature.cpp:499-511); JustDied's _JustDied()
-- (boss_highlord_omokk.cpp:63-66) writes DONE
-- (ScriptedCreature.cpp:513-520). See onEnterCombat/onReset/onDied.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FRENZY: victim-cast FRENZY 8269, 20s init -> 1min loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_KNOCK_AWAY: victim-cast KNOCK_AWAY 10101, 18s init -> 12s
-- loop (maiden convention).
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:72) + in-loop
-- gate (:91) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:1950; mal_ganis idiom) — a gated arm
-- fires nothing and consumes no re-arm, staying due until the cast
-- ends. The in-loop gate's same-tick co-expiry suppression has no
-- timer-arm model (independent callbacks, AV precedent). The C++
-- scheduler is driven from JustEngagedWith and
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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function onFrenzy(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "frenzy", 500, function() onFrenzy(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRENZY)
    end
    schedule(guid, "frenzy", 60000, function() onFrenzy(creature, guid) end)
end

local function onKnockAway(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "knockaway", 500, function() onKnockAway(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCK_AWAY)
    end
    schedule(guid, "knockaway", 12000, function() onKnockAway(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_highlord_omokk.cpp:55 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_highlord_omokk.cpp:55 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_HIGHLORD_OMOKK=0 (blackrock_spire.h:30).
    creature:SetBossState(0, 1)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "frenzy", 20000, function() onFrenzy(creature, guid) end)
    schedule(guid, "knockaway", 18000, function() onKnockAway(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ Reset (boss_highlord_omokk.cpp:51-54): _Reset() writes
-- NOT_STARTED gated on alive + state != DONE (ScriptedCreature.cpp:
-- 499-511). Alive-gated: Go fires ev23 on the death path too, and
-- C++ Reset never runs on death.
local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
    if creature:IsAlive() and creature:GetBossState(0) ~= 3 then
        creature:SetBossState(0, 0)
    end
end

-- C++ JustDied (boss_highlord_omokk.cpp:63-66): _JustDied() writes
-- DONE (ScriptedCreature.cpp:513-520).
local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    creature:SetBossState(0, 3)
end

RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 4, onDied)
RegisterCreatureEvent(ENTRY_HIGHLORD_OMOKK, 23, onReset)
