-- Nascent Fel Orc (Blood Furnace, Hellfire Citadel) — Lua
-- port of the BroggokPrisionersAI/npc_nascent_fel_orc structs
-- in src/server/scripts/Outland/HellfireCitadel/BloodFurnace/
-- boss_broggok.cpp (the base's Reset() emote arm and
-- JustEngagedWith cancel are part of the port below; the
-- go_/spell_ scripts in this file are documented only in
-- boss_broggok.lua, not registered there).
-- Entry: NPC_PRISONER1 = 17398 in blood_furnace.h, verified
-- externally (wowhead npc=17398/nascent-fel-orc) and by the
-- ability match — the C++ ScheduleEvents() casts concussion
-- blow 22427 + stomp 31900, the documented Blood Furnace mob
-- abilities for the Nascent Fel Orc (wowwiki Blood Furnace
-- mob table); the creature_template ScriptName bindings are
-- DB-side (no TDB in this workspace).
-- No Talk lines — neither struct defines a Say enum.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4
-- OnDied, 23 OnReset. Timers via CreateLuaEvent (per-GUID
-- scheduler pump); melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady. C++ DoCast
-- default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for all modeled arms): Nascent Fel
-- Orc (17398): OnEnterCombat(1): per-GUID reset to the C++
-- ScheduleEvents() values (concussionBlow 15000 / stomp 7000)
-- + 1s scheduler pump (a port of the TaskScheduler arms
-- scheduled in JustEngagedWith — 1s granularity exact for
-- all C++ timers; the !UpdateVictim early return collapses
-- into the pump). Pump: concussion blow 22427: 15s init
-- then {8s,11s} — non-triggered DoCastVictim (felmyst
-- convention), re-arm {8s,11s} (C++-exact); stomp 31900:
-- 7s init then {16s,21s} — non-triggered DoCastVictim,
-- re-arm {16s,21s} (C++-exact). OnDied(4): cleanup (the
-- instance OnUnitDeath prisoner-counter -> ActivateCell
-- chain is instance-gated). OnLeaveCombat(2)/OnReset(23):
-- cancel the pump, drop per-GUID state (the C++ Reset() ->
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

local SPELL_CONCUSSION_BLOW = 22427
local SPELL_STOMP = 31900

local ENTRY_NASCENT_FEL_ORC = 17398

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

-- C++ ScheduleEvents() values: concussionBlow 15s, stomp 7s.
local function freshState()
    return {
        concussionBlow = 15000,
        stomp = 7000,
    }
end

local function mobTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- Concussion blow 22427: 15s init then {8s,11s} —
    -- non-triggered DoCastVictim (felmyst convention), re-arm
    -- {8s,11s} (C++-exact).
    if st.concussionBlow <= 1000 then
        creature:CastSpell(nil, SPELL_CONCUSSION_BLOW)
        st.concussionBlow = math.random(8000, 11000)
    else
        st.concussionBlow = st.concussionBlow - 1000
    end

    -- Stomp 31900: 7s init then {16s,21s} — non-triggered
    -- DoCastVictim, re-arm {16s,21s} (C++-exact).
    if st.stomp <= 1000 then
        creature:CastSpell(nil, SPELL_STOMP)
        st.stomp = math.random(16000, 21000)
    else
        st.stomp = st.stomp - 1000
    end
end

RegisterCreatureEvent(ENTRY_NASCENT_FEL_ORC, 1, function(_, creature)
    local guid = creature:GetGUID()
    resetMob(guid)
    states[guid] = freshState()
    timers[guid] = CreateLuaEvent(function()
        mobTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_NASCENT_FEL_ORC, 2, function(_, creature)
    resetMob(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NASCENT_FEL_ORC, 4, function(_, creature)
    resetMob(creature:GetGUID())
end)

RegisterCreatureEvent(ENTRY_NASCENT_FEL_ORC, 23, function(_, creature)
    resetMob(creature:GetGUID())
end)
