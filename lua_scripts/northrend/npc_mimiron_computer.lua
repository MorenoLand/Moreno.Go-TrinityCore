-- Mimiron self-destruct computer — Lua port of npc_mimiron_computerAI in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1410-1512).
-- Entry (ulduar.h:133): NPC_COMPUTER = 34143.
-- C++ DoAction DO_ACTIVATE_COMPUTER: Talk SAY_SELF_DESTRUCT_INITIATED,
-- schedule EVENT_SELF_DESTRUCT_10 in 3s. DO_DEACTIVATE_COMPUTER: Talk
-- SAY_SELF_DESTRUCT_TERMINATED, remove the self-destruction aura +
-- visual, events.Reset(). The countdown chain talks 10..1 at 60s steps,
-- then FINALIZED talks, calls mimiron DoAction(DO_ACTIVATE_SELF_DESTRUCT)
-- and self-casts SPELL_SELF_DESTRUCTION_AURA (64610) +
-- SPELL_SELF_DESTRUCTION_VISUAL (64613).
-- PORTED via RegisterCreatureEvent ev38 (the boss_mimiron.lua DoAction
-- receiver precedent) + GUID-keyed CreateLuaEvent timers (the
-- npc_mimiron_flames.lua precedent). The DO_ACTIVATE_HARD_MODE leg the
-- 10-step fires is already live in boss_mimiron.lua's mimironDoAction
-- (its "sender stays out" comment is now stale — the sender lands here);
-- DO_ACTIVATE_SELF_DESTRUCT has no case in C++ boss_mimironAI::DoAction
-- (:397-422), so the call hits the Lua receiver's default, a faithful
-- no-op. DELTA: C++ re-schedules without cancelling (the button goes
-- NOT_SELECTABLE after the first press, so it cannot re-fire in
-- practice); the Lua schedule is cancel-first, so a second activation
-- restarts rather than doubles the chain.

local NPC_MIMIRON_COMPUTER = 34143 -- ulduar.h:133
local BOSS_MIMIRON = 10 -- ulduar.h:46
local DATA_COMPUTER = 35 -- ulduar.h UlduarData (DATA_AERIAL_COMMAND_UNIT = 34, counting up)

local SAY_SELF_DESTRUCT_INITIATED = 0 -- boss_mimiron.cpp:54
local SAY_SELF_DESTRUCT_TERMINATED = 1 -- boss_mimiron.cpp:55
local SAY_SELF_DESTRUCT_10 = 2 -- boss_mimiron.cpp:56
local SAY_SELF_DESTRUCT_9 = 3 -- boss_mimiron.cpp:57
local SAY_SELF_DESTRUCT_8 = 4 -- boss_mimiron.cpp:58
local SAY_SELF_DESTRUCT_7 = 5 -- boss_mimiron.cpp:59
local SAY_SELF_DESTRUCT_6 = 6 -- boss_mimiron.cpp:60
local SAY_SELF_DESTRUCT_5 = 7 -- boss_mimiron.cpp:61
local SAY_SELF_DESTRUCT_4 = 8 -- boss_mimiron.cpp:62
local SAY_SELF_DESTRUCT_3 = 9 -- boss_mimiron.cpp:63
local SAY_SELF_DESTRUCT_2 = 10 -- boss_mimiron.cpp:64
local SAY_SELF_DESTRUCT_1 = 11 -- boss_mimiron.cpp:65
local SAY_SELF_DESTRUCT_FINALIZED = 12 -- boss_mimiron.cpp:66

local SPELL_SELF_DESTRUCTION_AURA = 64610 -- boss_mimiron.cpp:168
local SPELL_SELF_DESTRUCTION_VISUAL = 64613 -- boss_mimiron.cpp:169

local DO_ACTIVATE_HARD_MODE = 13 -- boss_mimiron.cpp:300
local DO_ACTIVATE_COMPUTER = 14 -- boss_mimiron.cpp:301
local DO_DEACTIVATE_COMPUTER = 15 -- boss_mimiron.cpp:302
local DO_ACTIVATE_SELF_DESTRUCT = 16 -- boss_mimiron.cpp:303

local REACT_PASSIVE = 0

-- GUID-keyed CreateLuaEvent timers (C++ EventMap; the flames precedent).
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
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, 0)
end

-- C++ UpdateAI EVENT_SELF_DESTRUCT_10..FINALIZED (:1442-1500): talk the
-- step, then the next step in 60s; FINALIZED talks, calls mimiron
-- DoAction(DO_ACTIVATE_SELF_DESTRUCT) and self-casts both visuals.
local countdownSay = {
    SAY_SELF_DESTRUCT_10, SAY_SELF_DESTRUCT_9, SAY_SELF_DESTRUCT_8,
    SAY_SELF_DESTRUCT_7, SAY_SELF_DESTRUCT_6, SAY_SELF_DESTRUCT_5,
    SAY_SELF_DESTRUCT_4, SAY_SELF_DESTRUCT_3, SAY_SELF_DESTRUCT_2,
    SAY_SELF_DESTRUCT_1,
}

local function countdownStep(creature, guid, step)
    if step <= #countdownSay then
        creature:Talk(countdownSay[step])
        if step == 1 then
            -- C++ EVENT_SELF_DESTRUCT_10 (:1444-1449): the 10-step also
            -- activates hard mode on Mimiron.
            local mimiron = creature:GetInstanceCreature(BOSS_MIMIRON)
            if mimiron then
                mimiron:DoAction(DO_ACTIVATE_HARD_MODE)
            end
        end
        schedule(guid, "selfdestruct", 60000, function()
            countdownStep(creature, guid, step + 1)
        end)
        return
    end
    creature:Talk(SAY_SELF_DESTRUCT_FINALIZED)
    local mimiron = creature:GetInstanceCreature(BOSS_MIMIRON)
    if mimiron then
        mimiron:DoAction(DO_ACTIVATE_SELF_DESTRUCT)
    end
    creature:CastSpell(creature, SPELL_SELF_DESTRUCTION_AURA)
    creature:CastSpell(creature, SPELL_SELF_DESTRUCTION_VISUAL)
end

-- C++ ctor (:1417-1420): SetReactState(REACT_PASSIVE).
local function computerSpawn(event, creature)
    creature:SetReactState(REACT_PASSIVE)
end

-- C++ DoAction (:1424-1439).
local function computerDoAction(event, creature, action)
    local guid = creature:GetGUIDLow()
    if action == DO_ACTIVATE_COMPUTER then
        creature:Talk(SAY_SELF_DESTRUCT_INITIATED)
        schedule(guid, "selfdestruct", 3000, function()
            countdownStep(creature, guid, 1)
        end)
    elseif action == DO_DEACTIVATE_COMPUTER then
        creature:Talk(SAY_SELF_DESTRUCT_TERMINATED)
        creature:RemoveAura(SPELL_SELF_DESTRUCTION_AURA)
        creature:RemoveAura(SPELL_SELF_DESTRUCTION_VISUAL)
        cancelTimers(guid)
    end
end

local function computerCancel(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(NPC_MIMIRON_COMPUTER, 5, computerSpawn)
RegisterCreatureEvent(NPC_MIMIRON_COMPUTER, 38, computerDoAction)
RegisterCreatureEvent(NPC_MIMIRON_COMPUTER, 2, computerCancel)
RegisterCreatureEvent(NPC_MIMIRON_COMPUTER, 4, computerCancel)

-- Unmodeled: none. DO_ACTIVATE_HARD_MODE is sent by the countdown's
-- 10-step to mimiron (boss_mimiron.lua mimironDoAction already receives
-- it); the button sender lives in go_mimiron_hardmode_button.lua.
