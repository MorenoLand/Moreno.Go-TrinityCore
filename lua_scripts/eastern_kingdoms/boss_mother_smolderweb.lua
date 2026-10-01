-- Mother Smolderweb (Blackrock Spire; Caverns of Abomination) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_mother_smolderweb.cpp (boss_mothersmolderwebAI —
-- BossAI combat scheduler). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_highlord_omokk done,
-- registered for 9196; AddSC_boss_mothersmolderweb next in the
-- Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_MOTHER_SMOLDERWEB = 10596 in the BRS creatures enum. The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_highlord_omokk / boss_halycon precedent).
-- GetBlackrockSpireAI is a GetInstanceAI retrieval wrapper only
-- (blackrock_spire.h:129-132); the AI has no instance arms — Reset()
-- _Reset() is internal machinery covered here by the cancel/re-arm on
-- combat events (halycon precedent). JustDied _JustDied() likewise
-- covered by the cancel on OnDied(4).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine + DamageTaken lethal cast):
-- EVENT_CRYSTALIZE: self-cast CRYSTALIZE 16104, 20s init -> 15s loop
-- (DoCast(me, spell) path, creature:CastSpell(creature, spell)).
-- EVENT_MOTHERS_MILK: self-cast MOTHERSMILK 16468, 10s init ->
-- 5s loop (C++ range 5s,12500ms uses the lower bound,
-- recurring-timer convention).
-- DamageTaken: GetHealth() <= damage (lethal hit) -> self-cast
-- SUMMON_SPIRE_SPIDERLING 16103 (DoCast(me, 16103, true)). C++-exact
-- condition; one-shot per-guid flag (the fight cannot outlive the
-- lethal hit, magmus latch convention). The summon effect itself has
-- no SummonCreature bridge on the Lua surface, so the port carries
-- the cast arm only, which is what the C++ arm expresses.
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally (maiden
-- precedent). The C++ scheduler is driven from JustEngagedWith and
-- drained by UpdateAI; the port schedules from OnEnterCombat(1) and
-- cancels on 2/4/23 (maiden convention).

local SPELL_CRYSTALIZE = 16104
local SPELL_MOTHERSMILK = 16468
local SPELL_SUMMON_SPIRE_SPIDERLING = 16103

local ENTRY_MOTHER_SMOLDERWEB = 10596

local timers = {}
local summonFired = {}

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

local function onCrystalize(creature, guid)
    creature:CastSpell(creature, SPELL_CRYSTALIZE)
    schedule(guid, "crystalize", 15000, function() onCrystalize(creature, guid) end)
end

local function onMothersMilk(creature, guid)
    creature:CastSpell(creature, SPELL_MOTHERSMILK)
    schedule(guid, "milk", 5000, function() onMothersMilk(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    summonFired[guid] = nil
    schedule(guid, "crystalize", 20000, function() onCrystalize(creature, guid) end)
    schedule(guid, "milk", 10000, function() onMothersMilk(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if summonFired[guid] then
        return
    end
    if creature:GetHealth() <= damage then
        summonFired[guid] = true
        creature:CastSpell(creature, SPELL_SUMMON_SPIRE_SPIDERLING)
    end
end

RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 23, onCombatEnd)
