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
-- fire only from the unbridgeable SetData relay — unreachable
-- in this model); no event 3 registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
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
-- Deliberate deviations (all await engine bridges): no
-- instance/cross-creature bridge — the peon SetData relays
-- are PORTED below via FindNearestCreature + boss Talk (the
-- count/activation half stays boss-side: no cross-file state
-- bridge, and the in-port boss is already attackable, so the
-- 4th-death NON_ATTACKABLE removal is moot in-port); the
-- JustDied arm's GetBossState(IN_PROGRESS) gate has no
-- in-port equivalent (converts die while the boss is active
-- in-port), so the death Talk fires whenever the boss is
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
local SAY_PEON_ATTACKED = 1
local SAY_PEON_DIES = 2

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
-- range; boss:Talk(SAY_PEON_ATTACKED) is the SetData effect
-- (C++ talks once per relay while the count is below 4 — the
-- normal flow is 4 converts engaging once each, C++-exact;
-- re-engages after an evade would re-talk, as no cross-file
-- counter exists to enforce the cap).
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
    local guid = creature:GetGUID()
    resetConvert(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        convertTick(creature, guid)
    end, 1000, 0)
    local kurse = creature:FindNearestCreature(ENTRY_NETHEKURSE, 45.0)
    if kurse then
        kurse:Talk(SAY_PEON_ATTACKED)
    end
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 2, function(_, creature)
    resetConvert(creature:GetGUID())
end)

-- C++ MoveInLineOfSight is an empty override: converts never
-- sight-aggro (cyanigosa precedent).
RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 27, function(_, creature)
    return true
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 4, function(_, creature)
    resetConvert(creature:GetGUID())
    -- C++ JustDied peon-death relay (boss_nethekurse.cpp:340-346):
    -- no distance gate in C++, so a 100.0 room-scale search;
    -- boss:Talk(SAY_PEON_DIES) is the SetData effect (same
    -- count caveat as the aggro relay above).
    local kurse = creature:FindNearestCreature(ENTRY_NETHEKURSE, 100.0)
    if kurse then
        kurse:Talk(SAY_PEON_DIES)
    end
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 23, function(_, creature)
    resetConvert(creature:GetGUID())
    -- C++ Reset (boss_nethekurse.cpp:325): SetNoCallAssistance(true).
    creature:SetNoCallAssistance(true)
end)
