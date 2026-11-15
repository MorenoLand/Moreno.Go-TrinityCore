-- Fel Orc Convert (Shattered Halls, Hellfire Citadel) — Lua port of
-- the npc_fel_orc_convert CreatureScript registered alongside
-- boss_grand_warlock_nethekurse in src/server/scripts/Outland/
-- HellfireCitadel/ShatteredHalls/boss_nethekurse.cpp.
-- Entry: 17083 Fel Orc Convert (the C++ file carries no entry
-- constant — verified externally: L4G core Shattered Halls
-- notes list "Fel Orc Convert Entry: 17083" and the classic
-- bug-tracker thread cites wowhead npc=17083/fel-orc-convert;
-- the creature_template ScriptName bindings are DB-side, no
-- TDB in this workspace).
-- Talk lines: NONE — the convert defines no Say enum; all
-- SAY_* lines in this file live on the boss (the peon yells
-- fire from the boss ev41 SetData handler in
-- boss_nethekurse.lua, fed by the relay below); no event 3
-- registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Cast-state gates: NONE — ScriptedAI own UpdateAI override, plain EventMap loop, zero HasUnitState(UNIT_STATE_CASTING) gates; hemorrhage fires ungated, C++-exact.
-- Fight shape (C++-exact for all modeled arms): Convert
-- (17083): OnEnterCombat(1): per-GUID reset to the C++ Just
-- EngagedWith schedule value (hemorrhage 3000) + 1s
-- scheduler pump (a port of UpdateAI's EventMap handling —
-- 1s granularity exact; the !UpdateVictim early return
-- collapses into the pump). Pump: hemorrhage 30478: 3s init
-- then 15s — non-triggered DoCastVictim (felmyst
-- convention), re-arm 15s regardless (C++-exact).
-- OnDied(4): cleanup + PORTED peon-death relay (below).
-- OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): the peon
-- SetData relays are PORTED below via FindNearestCreature +
-- kurse:SetData(SETDATA_DATA, SETDATA_PEON_AGGRO/DEATH); the
-- <4 count caps, the Talks, and the 4th-death NON_ATTACKABLE
-- removal live in the boss ev41 handler (boss_nethekurse.lua),
-- and the removal is a no-op in-port (the in-port boss is
-- already attackable, Reset never sets the flag); the
-- JustDied arm's GetBossState(IN_PROGRESS) gate has no
-- in-port equivalent — its writer (the boss intro
-- MoveInLineOfSight) is unported, so gating on it would block
-- the chain in-port (boss state stays NOT_STARTED until boss
-- combat), so the death relay fires whenever the boss is
-- found alive nearby;
-- assistance arm PORTED — the Reset() SetNoCallAssistance(true)
-- arm runs on event 23 (bridge in lua_creature_events.go; the sticky
-- latch holds it across the evade clear); LoS-aggro arm PORTED —
-- the C++ empty MoveInLineOfSight override (no line-of-sight
-- response) is an ev27 veto returning true (cyanigosa precedent;
-- ev27 fires on Go's aggro-engage branch, so damage-based
-- engagement is unaffected); no
-- SpellScript/AuraScript scripts in this file.

local SPELL_HEMORRHAGE = 30478

local ENTRY_FEL_ORC_CONVERT = 17083
local ENTRY_NETHEKURSE = 16807

local SETDATA_DATA = 1
local SETDATA_PEON_AGGRO = 1
local SETDATA_PEON_DEATH = 2

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetConvert(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ JustEngagedWith: events.ScheduleEvent(EVENT_HEMORRHAGE,
-- 3s) + the peon-aggro relay (boss_nethekurse.cpp:333-336):
-- the 45.0yd IsWithinDist gate is the FindNearestCreature
-- range; kurse:SetData(SETDATA_DATA, SETDATA_PEON_AGGRO) is the
-- C++ relay verbatim — the boss ev41 handler owns the <4 count
-- cap and the Talk, so re-engages after an evade are capped
-- exactly as in C++.
local function freshState()
    return {
        hemorrhage = 3000,
    }
end

local function convertTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Hemorrhage 30478: 3s init then 15s — non-triggered
    -- DoCastVictim (felmyst convention), re-arm 15s
    -- regardless (C++-exact).
    if st.hemorrhage <= 1000 then
        creature:CastSpell(nil, SPELL_HEMORRHAGE)
        st.hemorrhage = 15000
    else
        st.hemorrhage = st.hemorrhage - 1000
    end
end

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetConvert(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        convertTick(creature, guid)
    end, 1000, 0)
    local kurse = creature:FindNearestCreature(ENTRY_NETHEKURSE, 45.0)
    if kurse then
        kurse:SetData(SETDATA_DATA, SETDATA_PEON_AGGRO)
    end
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 2, function(_, creature)
    resetConvert(creature:GetGUIDLow())
end)

-- C++ MoveInLineOfSight is an empty override: converts never
-- sight-aggro (cyanigosa precedent).
RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 27, function(_, creature)
    return true
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 4, function(_, creature)
    resetConvert(creature:GetGUIDLow())
    -- C++ JustDied peon-death relay (boss_nethekurse.cpp:340-346):
    -- no distance gate in C++, so a 100.0 room-scale search;
    -- kurse:SetData(SETDATA_DATA, SETDATA_PEON_DEATH) is the C++
    -- relay verbatim — the boss ev41 handler owns the <4 count
    -- cap, the Talk, and the 4th-death activation.
    local kurse = creature:FindNearestCreature(ENTRY_NETHEKURSE, 100.0)
    if kurse then
        kurse:SetData(SETDATA_DATA, SETDATA_PEON_DEATH)
    end
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 23, function(_, creature)
    resetConvert(creature:GetGUIDLow())
    -- C++ Reset (boss_nethekurse.cpp:325): SetNoCallAssistance(true).
    creature:SetNoCallAssistance(true)
end)
