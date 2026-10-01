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
-- OnDied(4): cleanup (the JustDied cross-creature arm is
-- blocked: instance->GetBossState(DATA_NETHEKURSE) !=
-- IN_PROGRESS early return plus the ObjectAccessor
-- GetCreature SetData(SETDATA_PEON_DEATH) relay have no
-- instance/cross-creature bridges). OnLeaveCombat(2)/
-- OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no
-- instance/cross-creature bridge — the JustEngagedWith arm
-- ObjectAccessor::GetCreature(instance->GetGuidData(NPC_
-- GRAND_WARLOCK_NETHEKURSE))->AI()->SetData(SETDATA_DATA,
-- SETDATA_PEON_AGGRO) unmodeled (kalithresh precedent), so
-- the convert contributes no peon-aggro counts to the boss
-- (the whole peon/intro machine stays on the boss side,
-- blocked); the JustDied arm's GetBossState(IN_PROGRESS)
-- gate and the SetData(SETDATA_PEON_DEATH) relay unmodeled;
-- no assistance bridge — the Reset() SetNoCallAssistance
-- arm unmodeled; no LoS-aggro bridge — the C++ empty
-- MoveInLineOfSight override (no line-of-sight response)
-- unmodeled, engagement follows engine aggro; no
-- SpellScript/AuraScript scripts in this file.

local SPELL_HEMORRHAGE = 30478

local ENTRY_FEL_ORC_CONVERT = 17083

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
-- 3s) — the ObjectAccessor/SetData arm in the same hook is
-- cross-creature blocked (documented above).
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
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 2, function(_, creature)
    resetConvert(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 4, function(_, creature)
    resetConvert(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_CONVERT, 23, function(_, creature)
    resetConvert(creature:GetGUID())
end)
