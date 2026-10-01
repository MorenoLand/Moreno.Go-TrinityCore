-- Fel Orc Neophyte (Blood Furnace, Hellfire Citadel) — Lua
-- port of the BroggokPrisionersAI/npc_fel_orc_neophyte
-- structs in src/server/scripts/Outland/HellfireCitadel/
-- BloodFurnace/boss_broggok.cpp (the base's Reset() emote
-- arm and JustEngagedWith cancel are part of the port below;
-- the go_/spell_ scripts in this file are documented only
-- in boss_broggok.lua, not registered there).
-- Entry: NPC_PRISONER2 = 17429 in blood_furnace.h, verified
-- externally (wowhead npc=17429/fel-orc-neophyte; the
-- azerothcore issue confirms the Fel Orc Neophyte entry
-- spawns alongside the Nascent Fel Orc in the Blood
-- Furnace); the creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- No Talk lines — neither struct defines a Say enum.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Fel Orc
-- Neophyte (17429): OnEnterCombat(1): per-GUID reset to the
-- C++ ScheduleEvents() values (charge 5000 / frenzy 1000)
-- + 1s scheduler pump (a port of the TaskScheduler arms
-- scheduled in JustEngagedWith — 1s granularity exact for
-- all C++ timers; the !UpdateVictim early return collapses
-- into the pump). Pump: charge 22120: 5s init then 20s —
-- non-triggered DoCastVictim (felmyst convention), re-arm
-- 20s (C++-exact); frenzy 8269: 1s init then {12s,13s} —
-- non-triggered DoCastSelf (C++ DoCastSelf), re-arm
-- {12s,13s} (C++-exact). OnDied(4): cleanup (the instance
-- OnUnitDeath prisoner-counter -> ActivateCell chain is
-- instance-gated). OnLeaveCombat(2)/OnReset(23): cancel the
-- pump, drop per-GUID state (the C++ Reset() ->
-- scheduler.CancelAll() arm collapses into the pump cancel;
-- the JustReachedHome -> instance GetBossState(IN_PROGRESS)
-- -> cross-creature DoAction(ACTION_RESET_BROGGOK) relay has
-- no instance/cross-creature bridges).
-- Deliberate deviations (all await engine bridges): no
-- instance-script model — the prisoner wave release and the
-- cell-door/lever choreography live in instance_blood_
-- furnace.cpp, which stays blocked on the instance-script
-- model; no emote bridge — the base Reset() emote arm
-- (scheduler.CancelAll + the 1s-5s emote task repeating
-- 6s-9s over the EMOTE_ONESHOT_ROAR/SHOUT/BATTLE_ROAR
-- one-shots) unmodeled, so no event 5 is registered (an
-- empty OnSpawn would be a kalithresh-style empty skeleton);
-- no SpellScript/AuraScript scripts in this file.

local SPELL_CHARGE = 22120
local SPELL_FRENZY = 8269

local ENTRY_FEL_ORC_NEOPHYTE = 17429

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetMob(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ ScheduleEvents() values: charge 5s, frenzy 1s.
local function freshState()
    return {
        charge = 5000,
        frenzy = 1000,
    }
end

local function mobTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Charge 22120: 5s init then 20s — non-triggered
    -- DoCastVictim (felmyst convention), re-arm 20s
    -- (C++-exact).
    if st.charge <= 1000 then
        creature:CastSpell(nil, SPELL_CHARGE)
        st.charge = 20000
    else
        st.charge = st.charge - 1000
    end

    -- Frenzy 8269: 1s init then {12s,13s} — non-triggered
    -- DoCastSelf (C++ DoCastSelf), re-arm {12s,13s}
    -- (C++-exact).
    if st.frenzy <= 1000 then
        creature:CastSpell(creature, SPELL_FRENZY)
        st.frenzy = math.random(12000, 13000)
    else
        st.frenzy = st.frenzy - 1000
    end
end

RegisterCreatureEvent(ENTRY_FEL_ORC_NEOPHYTE, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetMob(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        mobTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_NEOPHYTE, 2, function(_, creature)
    resetMob(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_NEOPHYTE, 4, function(_, creature)
    resetMob(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_FEL_ORC_NEOPHYTE, 23, function(_, creature)
    resetMob(creature:GetGUID())
end)
