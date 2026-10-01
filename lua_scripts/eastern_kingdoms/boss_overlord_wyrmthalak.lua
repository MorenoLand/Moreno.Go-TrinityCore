-- Overlord Wyrmthalak (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_overlord_wyrmthalak.cpp (boss_overlordwyrmthalakAI
-- — BossAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_mother_smolderweb done,
-- registered for 10596; AddSC_boss_overlordwyrmthalak next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_OVERLORD_WYRMTHALAK = 9568 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_mother_smolderweb / boss_highlord_omokk precedent).
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
-- EVENT_BLAST_WAVE: victim-cast BLASTWAVE 11130, 20s init -> 20s loop
-- (maiden convention — DoCastVictim path, creature:GetVictim()).
-- EVENT_SHOUT: victim-cast SHOUT 23511, 2s init -> 10s loop
-- (maiden convention).
-- EVENT_CLEAVE: victim-cast CLEAVE 20691, 6s init -> 7s loop
-- (maiden convention).
-- EVENT_KNOCK_AWAY: victim-cast KNOCKAWAY 20686, 12s init -> 14s loop
-- (maiden convention).
-- Unmodeled: the HealthBelowPct(51) one-shot summon latch (Summoned
-- flag reset in Initialize()/Reset()): it gates on
-- SelectTarget(SelectTargetMethod::Random, 0, 100, true) and calls
-- SummonCreature(NPC_SPIRESTONE_WARLORD=9216, SummonLocation1,
-- TEMPSUMMON_TIMED_DESPAWN, 5min) + SummonCreature(
-- NPC_SMOLDERTHORN_BERSERKER=9268, SummonLocation2, ...) then
-- AI()->AttackStart(target) on each — no SelectTarget, SummonCreature
-- or AttackStart bridges on the Lua surface (halycon gizrul
-- precedent). Summoned is write-only otherwise, so no Lua state is
-- ported. Porting the four timers without the latch does not strand
-- the fight — the latch is an add-spawn mid-fight arm, not the combat
-- trigger.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention).

local SPELL_BLASTWAVE = 11130
local SPELL_SHOUT = 23511
local SPELL_CLEAVE = 20691
local SPELL_KNOCKAWAY = 20686

local ENTRY_OVERLORD_WYRMTHALAK = 9568

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

local function onBlastWave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLASTWAVE)
    end
    schedule(guid, "blastwave", 20000, function() onBlastWave(creature, guid) end)
end

local function onShout(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHOUT)
    end
    schedule(guid, "shout", 10000, function() onShout(creature, guid) end)
end

local function onCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 7000, function() onCleave(creature, guid) end)
end

local function onKnockAway(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_KNOCKAWAY)
    end
    schedule(guid, "knockaway", 14000, function() onKnockAway(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "blastwave", 20000, function() onBlastWave(creature, guid) end)
    schedule(guid, "shout", 2000, function() onShout(creature, guid) end)
    schedule(guid, "cleave", 6000, function() onCleave(creature, guid) end)
    schedule(guid, "knockaway", 12000, function() onKnockAway(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_OVERLORD_WYRMTHALAK, 23, onCombatEnd)
