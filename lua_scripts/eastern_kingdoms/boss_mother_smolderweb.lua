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
-- (blackrock_spire.h:129-132). BossAI state legs are LIVE
-- (SetBossNumber(23), instance_blackrock_spire.cpp:62):
-- JustEngagedWith's base call (boss_mother_smolderweb.cpp:56) writes
-- SetBossState(DATA_MOTHER_SMOLDERWEB=3, IN_PROGRESS) via
-- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537); Reset's
-- _Reset() (:51) writes NOT_STARTED (ScriptedCreature.cpp:499-511,
-- alive + != DONE gated); JustDied's _JustDied() (:63) writes DONE
-- (ScriptedCreature.cpp:513-521) — see onEnterCombat/onReset/onDied.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken, 23 OnReset. Driven by CreateLuaEvent timers; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI event
-- machine + DamageTaken lethal cast):
-- EVENT_CRYSTALIZE: self-cast CRYSTALIZE 16104, 20s init -> 15s loop
-- (DoCast(me, spell) path, creature:CastSpell(creature, spell)).
-- EVENT_MOTHERS_MILK: self-cast MOTHERSMILK 16468, 10s init ->
-- {5s,12.5s} loop (C++ ScheduleEvent(EVENT_MOTHERS_MILK, 5s, 12500ms)
-- inclusive range; CreateLuaEvent {min,max} table is inclusive).
-- DamageTaken: GetHealth() <= damage (lethal hit) -> self-cast
-- SUMMON_SPIRE_SPIDERLING 16103 (DoCast(me, 16103, true)). C++-exact
-- condition; one-shot per-guid flag (the fight cannot outlive the
-- lethal hit, magmus latch convention). The summon effect lives
-- inside spell 16103's own effect data (creature:SummonCreature is
-- bridged but cannot see inside a spell cast — steamrigger
-- precedent), so the port carries the cast arm only, which is what
-- the C++ arm expresses.
-- CASTING gates ported: C++ UpdateAI pre-loop gate (:79) + in-loop
-- gate (:96) are a 500ms IsCasting defer under each arm's own key
-- (live, lua_creature_events.go:1950; mal_ganis idiom) — a gated arm
-- fires nothing and consumes no re-arm, staying due until the cast
-- ends. The in-loop gate's same-tick co-expiry suppression has no
-- timer-arm model (independent callbacks, AV precedent). The
-- DamageTaken lethal cast stays ungated per C++ (no gate in that
-- hook). The C++ scheduler is driven from JustEngagedWith and
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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function onCrystalize(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "crystalize", 500, function() onCrystalize(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_CRYSTALIZE)
    schedule(guid, "crystalize", 15000, function() onCrystalize(creature, guid) end)
end

local function onMothersMilk(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "milk", 500, function() onMothersMilk(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_MOTHERSMILK)
    schedule(guid, "milk", {5000, 12500}, function() onMothersMilk(creature, guid) end)
end

local function onEnterCombat(_, creature)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540; boss_mother_smolderweb.cpp:56 base call first)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; boss_mother_smolderweb.cpp:56 base call first)
    -- BossAI::_JustEngagedWith (ScriptedCreature.cpp:537): the base call
    -- sets the boss state IN_PROGRESS; DATA_MOTHER_SMOLDERWEB=3 (blackrock_spire.h:33).
    creature:SetBossState(3, 1)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    summonFired[guid] = nil
    schedule(guid, "crystalize", 20000, function() onCrystalize(creature, guid) end)
    schedule(guid, "milk", 10000, function() onMothersMilk(creature, guid) end)
end

local function onCombatEnd(_, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset (boss_mother_smolderweb.cpp:49-52): _Reset() bookkeeping (timer cancel
-- above) + SetBossState(DATA_MOTHER_SMOLDERWEB=3, NOT_STARTED), gated on alive
-- and state != DONE (ScriptedCreature.cpp:499-511). Alive-gated: Go fires
-- ev23 on the death path too, and C++ Reset never runs on death.
local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsAlive() and creature:GetBossState(3) ~= 3 then
        creature:SetBossState(3, 0)
    end
end

-- C++ JustDied (boss_mother_smolderweb.cpp:61-64): _JustDied() bookkeeping (timer
-- cancel) + SetBossState(DATA_MOTHER_SMOLDERWEB=3, DONE).
local function onDied(_, creature)
    cancelTimers(creature:GetGUID())
    creature:SetBossState(3, 3)
end

local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if summonFired[guid] then
        return
    end
    if creature:GetHealth() <= damage then
        summonFired[guid] = true
        creature:CastSpell(creature, SPELL_SUMMON_SPIRE_SPIDERLING, true)
    end
end

RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 4, onDied)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_MOTHER_SMOLDERWEB, 23, onReset)
