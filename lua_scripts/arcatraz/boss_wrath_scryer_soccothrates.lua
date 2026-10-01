-- Wrath-Scryer Soccothrates (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/boss_wrath_scryer_soccothrates.cpp (boss_wrath_
-- scryer_soccothrates — the only CreatureScript AI class
-- AddSC_boss_wrath_scryer_soccothrates registers; no
-- SpellScript/AuraScript scripts in this file).
-- Third boss in the Arcatraz set per outland_script_
-- loader.cpp order (instance_arcatraz.cpp stays blocked on
-- the instance-script model).
-- Entry: 20886 (NPC_SOCCOTHRATES in arcatraz.h — creature
-- entry, verifiable from the C++ sources); instance_
-- arcatraz.cpp OnCreatureCreate maps NPC_SOCCOTHRATES to
-- SoccothratesGUID. The creature_template ScriptName
-- bindings are DB-side (no TDB in this workspace).
-- Talk: SAY_AGGRO = 1 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 2 (kill — the C++
-- KilledUnit talks unconditionally, no player gate, gargolmar
-- precedent, C++-exact), SAY_KNOCK_AWAY = 3 (knock away —
-- fired from the EVENT_KNOCK_AWAY arm, C++-exact), SAY_DEATH
-- = 4 (death — fired from the C++ JustDied, C++-exact);
-- SAY_DALLIAH_DEATH = 6 fires only from the EVENT_DALLIAH_
-- DEATH arm — unreachable in this model (cross-creature
-- SetData bridge missing, see deviations); SAY_SOCCOTHRATES_
-- CONVO_1..4 = 7..10 fire only from the prefight machine —
-- unreachable (MoveInLineOfSight arm instance-blocked, see
-- deviations). SAY_AGGRO_SOCCOTHRATES_FIRST = 0 and SAY_
-- SOCCOTHRATES_25_PERCENT = 6 and SAY_DALLIAH_CONVO_1..3 =
-- 8..10 are Dalliah's creature-text lines, fired via dalliah->
-- AI()->Talk(...) — owned by the dalliah port (dalliah's
-- ME_FIRST arm and 25% taunt are cross-creature blocked
-- there), unreachable here.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady.
-- Fight shape (C++-exact for all modeled arms): Soccothrates
-- (20886): OnEnterCombat(1): per-GUID reset to the C++ ctor
-- / JustEngagedWith values (felfireShock {12s,14s} / knockAway
-- {11s,12s} / dalliahTaunt = false — the BossAI::JustEngagedWith
-- arm is instance-blocked, DATA_SOCCOTHRATES = 2 is an
-- instance-side constant, unbridgeable; the EVENT_ME_FIRST 6s
-- one-shot arm is cross-creature blocked, unmodeled) + Talk
-- (SAY_AGGRO) + 1s scheduler pump (a port of UpdateAI in C++
-- arm order — 1s granularity exact for all C++ timers; the
-- !UpdateVictim early return collapses into the pump: the
-- preFight and dalliahDeath out-of-combat branches are
-- unreachable in this model — no MoveInLineOfSight / data-
-- set / cross-creature bridges). Pump: felfire shock 35759 —
-- {12s,14s} init then {12s,14s}, TRIGGERED DoCastVictim (C++
-- `DoCastVictim(SPELL_FELFIRE_SHOCK, true)`, C++-exact),
-- re-arm {12s,14s} regardless (C++-exact); knock away 36512
-- — {11s,12s} init then {11s,12s}, non-triggered DoCast on
-- self (C++ DoCast(me), C++-exact), Talk(SAY_KNOCK_AWAY),
-- re-arm {11s,12s} regardless (C++-exact). The HealthBelowPct
-- (25) latch runs every tick (halazzi strict-fraction
-- precedent): the first sub-25% tick sets the dalliahTaunt
-- latch — the C++ Talk(SAY_SOCCOTHRATES_25_PERCENT) on
-- Dalliah's AI has no cross-creature bridge, so the yell is
-- unreachable and the latch is bookkeeping only (omor
-- summonedCount precedent). The C++ UNIT_STATE_CASTING early-
-- return gates (before and after the event switch) have no
-- cast-state bridge — the arms fire unconditionally
-- (netherspite precedent). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++ talks unconditionally —
-- no player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm is instance-blocked; the
-- dalliah->AI()->SetData(1, 1) death relay has no cross-
-- creature bridge). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the C++ Reset() observable
-- remainder — the schedule / dalliahTaunt / preFight re-latch
-- — lands on OnEnterCombat per the gargolmar precedent; the
-- _Reset arm is instance-blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI
-- shim (instance_arcatraz.cpp stays blocked on the instance-
-- script model; the BossAI ctor DATA_SOCCOTHRATES = 2
-- bookkeeping in Reset/JustEngagedWith/JustDied skipped); no
-- MoveInLineOfSight / instance-data bridge — the 70yd
-- proximity arm (instance->GetData(DATA_CONVERSATION) ==
-- NOT_STARTED + TYPEID_PLAYER gate) is unreachable, so the
-- whole preFight machine is unmodeled: the EVENT_PREFIGHT_
-- 1..9 chain, Talk(SAY_SOCCOTHRATES_CONVO_1..4), the dalliah->
-- AI()->Talk(SAY_DALLIAH_CONVO_1..3) arms, the dalliah + self
-- MovePoint arms and the SetFacingToObject / SetHomePosition
-- arms (no cross-creature / movement / facing bridges); no
-- cross-creature bridge — the EVENT_ME_FIRST 6s arm (dalliah->
-- AI()->Talk(SAY_AGGRO_SOCCOTHRATES_FIRST) when Dalliah is
-- alive and out of combat) unmodeled; the HealthBelowPct(25)
-- taunt (dalliah->AI()->Talk(SAY_SOCCOTHRATES_25_PERCENT))
-- unmodeled; the JustDied dalliah->AI()->SetData(1, 1) relay
-- unmodeled; no data-set bridge — the SetData(1) / EVENT_
-- DALLIAH_DEATH machine (6s later Talk(SAY_DALLIAH_DEATH))
-- unmodeled — no SetData bridge exists on the shim (see
-- engine/world/lua_creature_events.go); no cast-state bridge
-- — the UNIT_STATE_CASTING early-return gates unmodeled
-- (netherspite precedent); the unused spell enums (SPELL_
-- FELFIRE_LINE_UP 35770 / SPELL_CHARGE_TARGETING 36038 /
-- SPELL_CHARGE 35754) never fire in the C++ AI — the
-- ScriptData "charge left to script" caveat is upstream,
-- documented, not bridged (millhouse unused-spells
-- precedent); no SpellScript/AuraScript scripts in this file.

local SPELL_FELFIRE_SHOCK = 35759
local SPELL_KNOCK_AWAY = 36512

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_KNOCK_AWAY = 3
local SAY_DEATH = 4

local ENTRY_SOCCOTHRATES = 20886

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ ctor / JustEngagedWith schedule (the BossAI arms are
-- instance-blocked, so the whole engagement schedule lands
-- on OnEnterCombat — omor precedent): felfireShock {12s,14s}
-- / knockAway {11s,12s}; dalliahTaunt is the C++ ctor latch,
-- false at engage (the C++ Reset() re-latch). The EVENT_ME_
-- FIRST 6s arm is cross-creature blocked, unmodeled.
local function freshState()
    return {
        felfireShock = 12000 + math.random(0, 1999),
        knockAway = 11000 + math.random(0, 999),
        dalliahTaunt = false,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Felfire shock 35759: {12s,14s} init then {12s,14s} —
    -- TRIGGERED DoCastVictim (C++-exact), re-arm {12s,14s}
    -- regardless (C++-exact). The C++ UNIT_STATE_CASTING
    -- early-return gates have no cast-state bridge — the arm
    -- fires unconditionally (netherspite precedent).
    if st.felfireShock <= 1000 then
        creature:CastSpell(nil, SPELL_FELFIRE_SHOCK, true)
        st.felfireShock = 12000 + math.random(0, 1999)
    else
        st.felfireShock = st.felfireShock - 1000
    end

    -- Knock away 36512: {11s,12s} init then {11s,12s} — non-
    -- triggered DoCast on self (C++ DoCast(me), C++-exact),
    -- Talk(SAY_KNOCK_AWAY), re-arm {11s,12s} regardless
    -- (C++-exact).
    if st.knockAway <= 1000 then
        creature:CastSpell(creature, SPELL_KNOCK_AWAY)
        creature:Talk(SAY_KNOCK_AWAY)
        st.knockAway = 11000 + math.random(0, 999)
    else
        st.knockAway = st.knockAway - 1000
    end

    -- HealthBelowPct(25) latch (halazzi strict-fraction
    -- precedent): the C++ arm fires Talk(SAY_SOCCOTHRATES_
    -- 25_PERCENT) on Dalliah's AI via the instance GUID —
    -- no cross-creature bridge, so the yell is unreachable
    -- and the latch is bookkeeping only (omor precedent).
    if not st.dalliahTaunt and creature:GetHealthPct() < 25 then
        st.dalliahTaunt = true
    end
end

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_SOCCOTHRATES, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
