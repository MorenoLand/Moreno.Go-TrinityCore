-- Dalliah the Doomsayer (The Arcatraz, Tempest Keep) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- arcatraz/boss_dalliah_the_doomsayer.cpp (boss_dalliah_the_
-- doomsayer — the only CreatureScript AI class AddSC_boss_
-- dalliah_the_doomsayer registers; no SpellScript/AuraScript
-- scripts in this file).
-- Second boss in the Arcatraz set per outland_script_
-- loader.cpp order (instance_arcatraz.cpp stays blocked on
-- the instance-script model).
-- Entry: 20885 (NPC_DALLIAH in arcatraz.h — creature entry,
-- verifiable from the C++ sources); instance_arcatraz.cpp
-- OnCreatureCreate maps NPC_DALLIAH to DalliahGUID. The
-- creature_template ScriptName bindings are DB-side (no TDB
-- in this workspace).
-- Talk: SAY_AGGRO = 1 (pull — fired from the C++
-- JustEngagedWith, C++-exact), SAY_SLAY = 2 (kill — the C++
-- KilledUnit talks unconditionally, no player gate, gargolmar
-- precedent, C++-exact), SAY_WHIRLWIND = 3 (whirlwind — fired
-- from the EVENT_WHIRLWIND arm, C++-exact), SAY_HEAL = 4
-- (heal — fired from the EVENT_HEAL arm, C++-exact), SAY_
-- DEATH = 5 (death — fired from the C++ JustDied, C++-exact);
-- SAY_SOCCOTHRATES_DEATH = 7 fires only from the
-- EVENT_SOCCOTHRATES_DEATH arm, unreachable in this model
-- (see deviations). SAY_AGGRO_DALLIAH_FIRST = 0 and SAY_
-- DALLIAH_25_PERCENT = 5 are Soccothrates' creature-text
-- lines, fired via soccothrates->AI()->Talk(...) — owned by
-- the boss_wrath_scryer_soccothrates.cpp port, unreachable
-- here (cross-creature blocked).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent (per-GUID scheduler pump); melee is
-- engine-driven in Go (creature combat tick), like C++ DoMelee
-- AttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Dalliah
-- (20885): OnEnterCombat(1): per-GUID reset to the C++
-- Initialize-equivalent schedule (gift {1s,4s} / whirlwind
-- {7s,9s}; the BossAI::JustEngagedWith arm is instance-
-- blocked, DATA_DALLIAH = 1 is an instance-side constant,
-- unbridgeable) + Talk(SAY_AGGRO) + 1s scheduler pump (a port
-- of UpdateAI in C++ arm order — 1s granularity exact for all
-- C++ timers; the !UpdateVictim early return collapses into
-- the pump; the soccothratesDeath out-of-combat branch is
-- unreachable in this model — no data-set bridge). Pump:
-- gift of the doomsayer 36173 — {1s,4s} init then {16s,21s},
-- TRIGGERED DoCastVictim (C++ `DoCastVictim(SPELL_GIFT_OF_
-- THE_DOOMSAYER, true)`, C++-exact), re-arm {16s,21s}
-- regardless (C++-exact); whirlwind 36142 — {7s,9s} init then
-- {19s,21s}, non-triggered DoCast on self (C++ DoCast(me),
-- C++-exact), Talk(SAY_WHIRLWIND), re-arm {19s,21s} (C++-
-- exact), schedule the heal arm in 6s (C++-exact); heal
-- 36144 — one-shot, non-triggered DoCast on self (C++ DoCast
-- (me), C++-exact), Talk(SAY_HEAL). The HealthBelowPct(25)
-- latch runs every tick (halazzi strict-fraction precedent):
-- the first sub-25% tick sets the soccothratesTaunt latch —
-- the C++ Talk(SAY_DALLIAH_25_PERCENT) on Soccothrates' AI
-- has no cross-creature bridge, so the yell is unreachable
-- and the latch is bookkeeping only (omor summonedCount
-- precedent). The C++ UNIT_STATE_CASTING early-return gates
-- (before and after the event switch) have no cast-state
-- bridge — the arms fire unconditionally (netherspite
-- precedent). Melee is engine-driven.
-- OnTargetDied(3): Talk(SAY_SLAY) (C++ talks unconditionally —
-- no player gate, C++-exact). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the _JustDied arm is instance-blocked; the
-- soccothrates->AI()->SetData(1, 1) death relay has no cross-
-- creature bridge). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the C++ Reset() observable
-- remainder — the schedule re-latch — lands on OnEnterCombat
-- per the gargolmar precedent; the _Reset arm is instance-
-- blocked).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — boss admission via the luaBossAI shim
-- (instance_arcatraz.cpp stays blocked on the instance-script
-- model; the BossAI ctor DATA_DALLIAH = 1 bookkeeping in
-- Reset/JustEngagedWith/JustDied skipped); no cross-creature
-- bridge — the EVENT_ME_FIRST 6s arm (Talk(SAY_AGGRO_DALLIAH_
-- FIRST) on Soccothrates' AI via instance GetGuidData(DATA_
-- SOCCOTHRATES)) unmodeled; the HealthBelowPct(25) taunt
-- (Talk(SAY_DALLIAH_25_PERCENT) on Soccothrates' AI)
-- unmodeled — soccothrates-side origin only; the JustDied
-- soccothrates->AI()->SetData(1, 1) relay unmodeled; no
-- data-set bridge — the EVENT_SOCCOTHRATES_DEATH machine
-- (Soccothrates' JustDied calls dalliah->AI()->SetData(1, 1)
-- when Dalliah is alive and out of combat; 6s later Dalliah
-- says SAY_SOCCOTHRATES_DEATH = 7 via the !UpdateVictim arm)
-- unmodeled — no SetData bridge exists on the shim (see
-- engine/world/lua_creature_events.go); no difficulty bridge
-- — the heroic-only EVENT_SHADOW_WAVE 39016 arm (TRIGGERED
-- DoCastVictim {11s,16s}) unmodeled (thespia precedent); no
-- cast-state bridge — the UNIT_STATE_CASTING early-return
-- gates unmodeled (netherspite precedent); no SpellScript/
-- AuraScript scripts in this file.

local SPELL_GIFT_OF_THE_DOOMSAYER = 36173
local SPELL_WHIRLWIND = 36142
local SPELL_HEAL = 36144

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_WHIRLWIND = 3
local SAY_HEAL = 4
local SAY_DEATH = 5

local ENTRY_DALLIAH = 20885

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

-- C++ JustEngagedWith schedule (the BossAI arm is instance
-- blocked, so the whole engagement schedule lands on
-- OnEnterCombat — omor precedent): gift {1s,4s} / whirlwind
-- {7s,9s}; soccothratesTaunt is the C++ ctor latch, false at
-- engage (the C++ Reset() re-latch).
local function freshState()
    return {
        gift = 1000 + math.random(0, 2999),
        whirlwind = 7000 + math.random(0, 1999),
        heal = nil,
        soccothratesTaunt = false,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Gift of the doomsayer 36173: {1s,4s} init then {16s,21s}
    -- — TRIGGERED DoCastVictim (C++-exact), re-arm {16s,21s}
    -- regardless (C++-exact). The C++ UNIT_STATE_CASTING
    -- early-return gates have no cast-state bridge — the arm
    -- fires unconditionally (netherspite precedent).
    if st.gift <= 1000 then
        creature:CastSpell(nil, SPELL_GIFT_OF_THE_DOOMSAYER, true)
        st.gift = 16000 + math.random(0, 4999)
    else
        st.gift = st.gift - 1000
    end

    -- Whirlwind 36142: {7s,9s} init then {19s,21s} — non-
    -- triggered DoCast on self (C++ DoCast(me), C++-exact),
    -- Talk(SAY_WHIRLWIND), re-arm {19s,21s} (C++-exact) and
    -- schedule the one-shot heal arm in 6s (C++-exact).
    if st.whirlwind <= 1000 then
        creature:CastSpell(creature, SPELL_WHIRLWIND)
        creature:Talk(SAY_WHIRLWIND)
        st.whirlwind = 19000 + math.random(0, 1999)
        st.heal = 6000
    else
        st.whirlwind = st.whirlwind - 1000
    end

    -- Heal 36144: one-shot — non-triggered DoCast on self (C++
    -- DoCast(me), C++-exact), Talk(SAY_HEAL).
    if st.heal then
        if st.heal <= 1000 then
            creature:CastSpell(creature, SPELL_HEAL)
            creature:Talk(SAY_HEAL)
            st.heal = nil
        else
            st.heal = st.heal - 1000
        end
    end

    -- HealthBelowPct(25) latch (halazzi strict-fraction
    -- precedent): the C++ arm fires Talk(SAY_DALLIAH_25_
    -- PERCENT) on Soccothrates' AI via the instance GUID —
    -- no cross-creature bridge, so the yell is unreachable
    -- and the latch is bookkeeping only (omor precedent).
    if not st.soccothratesTaunt and creature:GetHealthPct() < 25 then
        st.soccothratesTaunt = true
    end
end

RegisterCreatureEvent(ENTRY_DALLIAH, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetBoss(guid)
    states[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 2, function(_, creature)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally — no player gate
    -- (gargolmar precedent, C++-exact).
    creature:Talk(SAY_SLAY)
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 4, function(_, creature)
    creature:Talk(SAY_DEATH)
    resetBoss(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_DALLIAH, 23, function(_, creature)
    resetBoss(creature:GetGUID())
end)
