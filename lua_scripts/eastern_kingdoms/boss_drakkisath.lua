-- General Drakkisath (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_drakkisath.cpp (boss_drakkisathAI — BossAI combat
-- scheduler). Boss-script unit per eastern_kingdoms_script_loader.cpp
-- order (boss_drakkisath opens the Blackrock Spire block; next is
-- boss_halycon).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_GENERAL_DRAKKISATH = 10363 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_magmus / boss_moira_bronzebeard precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132). SetBossState legs (BossAI DATA_GENERAL_
-- DRAKKISATH = 13, blackrock_spire.h:43; < EncounterCount 23):
-- ev1 IN_PROGRESS via base BossAI::JustEngagedWith (boss_drakkisath.
-- cpp:54, base call first; _JustEngagedWith writes IN_PROGRESS at
-- ScriptedCreature.cpp:537); ev23 NOT_STARTED via Reset's _Reset()
-- (:47-50; _Reset gates on alive + state != DONE, ScriptedCreature.
-- cpp:499-511); ev4 DONE via JustDied's _JustDied() (:61-64;
-- ScriptedCreature.cpp:513-520). Bridge live (lua_creature_events.
-- go:5917; gyth convention).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine):
-- EVENT_FIRE_NOVA: victim-cast FIRENOVA 23462, 6s init -> 10s;
-- EVENT_CLEAVE: victim-cast CLEAVE 20691, 8s init -> 8s;
-- EVENT_CONFLIGURATION: victim-cast CONFLIGURATION 16805, 15s init ->
-- 18s; EVENT_THUNDERCLAP: victim-cast THUNDERCLAP 15548, 17s init ->
-- 20s. (C++ marks the thunderclap spell id "Not sure if right ID";
-- carried through unchanged.)
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:73) + in-loop
-- gate (:98) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:1950; mal_ganis idiom) — a gated arm
-- fires nothing and consumes no re-arm, staying due until the cast
-- ends. The in-loop gate's same-tick co-expiry suppression has no
-- timer-arm model (independent callbacks, AV precedent). The C++
-- scheduler is driven from JustEngagedWith and
-- drained by UpdateAI; the port schedules from OnEnterCombat(1) and
-- cancels on 2/4/23 (maiden convention). The C++ Reset() _events.Reset()
-- is covered by the cancel/re-arm on combat events.

local SPELL_FIRENOVA = 23462
local SPELL_CLEAVE = 20691
local SPELL_CONFLIGURATION = 16805
local SPELL_THUNDERCLAP = 15548

local ENTRY_DRAKKISATH = 10363

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

local function onFireNova(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "firenova", 500, function() onFireNova(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FIRENOVA)
    end
    schedule(guid, "firenova", 10000, function() onFireNova(creature, guid) end)
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
    schedule(guid, "cleave", 8000, function() onCleave(creature, guid) end)
end

local function onConfligration(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "confligration", 500, function() onConfligration(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CONFLIGURATION)
    end
    schedule(guid, "confligration", 18000, function() onConfligration(creature, guid) end)
end

local function onThunderclap(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "thunderclap", 500, function() onThunderclap(creature, guid) end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THUNDERCLAP)
    end
    schedule(guid, "thunderclap", 20000, function() onThunderclap(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_drakkisath.cpp:54 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_drakkisath.cpp:54 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_GENERAL_DRAKKISATH=13 (blackrock_spire.h:43).
    creature:SetBossState(13, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "firenova", 6000, function() onFireNova(creature, guid) end)
    schedule(guid, "cleave", 8000, function() onCleave(creature, guid) end)
    schedule(guid, "confligration", 15000, function() onConfligration(creature, guid) end)
    schedule(guid, "thunderclap", 17000, function() onThunderclap(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustDied (boss_drakkisath.cpp:61-64): _JustDied() bookkeeping
-- (timer cancel) + SetBossState(DATA_GENERAL_DRAKKISATH=13, DONE).
local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(13, 3)
end

-- C++ Reset (boss_drakkisath.cpp:47-50): _Reset() only — alive-gated
-- (Go fires ev23 on the death path too) and skipped when already DONE
-- (ScriptedCreature.cpp:499-511).
local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsAlive() and creature:GetBossState(13) ~= 3 then
        creature:SetBossState(13, 0)
    end
end

RegisterCreatureEvent(ENTRY_DRAKKISATH, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_DRAKKISATH, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_DRAKKISATH, 4, onDied)
RegisterCreatureEvent(ENTRY_DRAKKISATH, 23, onReset)
