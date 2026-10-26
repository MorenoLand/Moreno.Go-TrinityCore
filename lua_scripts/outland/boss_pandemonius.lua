-- Pandemonius (Mana Tombs, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/ManaTombs/boss_pandemonius.cpp
-- (1 CreatureScript: boss_pandemonius (BossAI via GetManaTombsAI,
-- DATA_PANDEMONIUS = 0 — mana_tombs.h:31); registered from inside
-- AddSC_boss_pandemonius() (boss_pandemonius.cpp:125-128); loader
-- decl 25 / call 148 per outland_script_loader.cpp — the FIRST call
-- of the "// Auchindoun - Mana Tombs" sub-block in
-- AddOutlandScripts(), immediately before AddSC_boss_nexusprince_
-- shaffar() (call 149) — verified from the loader this run;
-- sole-source verified: whole-server-tree grep for
-- "AddSC_boss_pandemonius" hits boss_pandemonius.cpp (+ the loader
-- decl/call lines) only).
-- Entry: 18341 — DB-side (no NPC_ constant in boss_pandemonius.cpp);
-- verified this run against the wowhead TBC Classic NPC page
-- (/tbc/npc=18341/pandemonius — page title "Pandemonius - NPC - TBC
-- Classic", "first boss found in Mana-Tombs").
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- boss_pandemoniusAI::JustEngagedWith (:76-84) — Talk(SAY_AGGRO 0);
-- the BossAI::JustEngagedWith legs have no bridge (the auriaya
-- precedent) — then ScheduleEvent(EVENT_DARK_SHELL, 20s,
-- DARK_SHELL_EVENT_GROUP) + ScheduleEvent(EVENT_VOID_BLAST, 8s,
-- 23s) — the (8s, 23s) pair is randtime(8000, 23000), ported as the
-- {min, max} table delay per the Oct-13 CreateLuaEvent convention.
-- EVENT_VOID_BLAST (:86-104) — SelectTarget(Random, 0, 100.0f,
-- true): random player from the threat list within 100 yards —
-- ported via GetAITargets (the bridged threat list; midnight
-- precedent) + the IsWithinDistInMap(t, 100.0) player filter (the
-- shaffar MoveInLOS precedent) + "Player" object-type gate (the
-- illidan convention) + IsAlive (C++ threat refs go offline on
-- death, ThreatManager.cpp ShouldBeOffline, and SelectTargetList
-- skips offline refs; Go's SortedEntries returns dead entries too). DoCast(target, SPELL_VOID_BLAST 32325) 2-arg
-- (non-triggered) — CastSpell(target, spell); ++VoidBlastCounter
-- ONLY when a target was found (the increment sits inside the
-- target-found leg, :90-93). Counter == 5: counter = 0 and re-arm
-- randtime(15000, 25000); else re-arm 500ms + DelayEvents(500ms,
-- DARK_SHELL_EVENT_GROUP) (:96-102).
-- EVENT_DARK_SHELL (:105-111) — the IsNonMeleeSpellCast /
-- InterruptNonMeleeSpells(true) legs: interrupt bridged (no-op twin,
-- ported); IsNonMeleeSpellCast gate unmodeled (shaffar blink
-- precedent); Talk(EMOTE_DARK_SHELL 3); DoCast(me,
-- SPELL_DARK_SHELL 32358) 2-arg (non-triggered) self-cast —
-- CastSpell(creature, spell); re-arm 20s in the shell group.
-- boss_pandemoniusAI::KilledUnit — Talk(SAY_KILL 1) (event 3).
-- boss_pandemoniusAI::JustDied — Talk(SAY_DEATH 2) (event 4).
-- DOCUMENTED-ONLY (in this header; only entry 18341 registered):
-- boss_pandemoniusAI ctor / Reset — BossAI _Reset() + the
-- DATA_PANDEMONIUS instance binding (GetManaTombsAI): the
-- BossAI/instance legs have no bridge (the auriaya precedent) —
-- the Lua port carries no encounter state, and the per-GUID timer
-- tables reset on 2/4/23 (the gargolmar precedent) matching the
-- C++ Reset re-latch of VoidBlastCounter = 0.
-- events.DelayEvents(500ms, DARK_SHELL_EVENT_GROUP): no
-- event-group-delay bridge. Modeled as a shellShift accumulator:
-- each non-5th void blast adds 500ms to the NEXT dark-shell
-- interval (C++ stretches the pending interval; steady-state drift
-- identical — ~2s per 5-blast cycle — the only deviation is which
-- interval boundary absorbs it).

local ENTRY_PANDEMONIUS = 18341 -- DB-side; wowhead-verified this run

local SAY_AGGRO       = 0
local SAY_KILL        = 1
local SAY_DEATH       = 2
local EMOTE_DARK_SHELL = 3

local SPELL_VOID_BLAST = 32325 -- boss_pandemonius.cpp:32
local SPELL_DARK_SHELL = 32358 -- boss_pandemonius.cpp:33

local timers = {}
local blastCounters = {} -- per-GUID VoidBlastCounter (C++ :70)
local shellShifts = {}   -- per-GUID DelayEvents(500ms, group1) accumulator

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

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 100.0f, true)
-- (:89): random player from the threat list within 100 yards.
local function randomPlayerIn100(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and t:IsAlive()
                and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < 100.0 then -- IsWithinDistInMap twin (Object.cpp:1238; player reach 1.5)
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_VOID_BLAST (:86-104).
local function voidBlastTick(creature, guid)
    local target = randomPlayerIn100(creature)
    if target then
        creature:CastSpell(target, SPELL_VOID_BLAST)
        blastCounters[guid] = (blastCounters[guid] or 0) + 1
    end
    if (blastCounters[guid] or 0) == 5 then
        blastCounters[guid] = 0
        schedule(guid, "void_blast", {15000, 25000}, function()
            voidBlastTick(creature, guid)
        end)
    else
        -- C++ also calls events.DelayEvents(500ms,
        -- DARK_SHELL_EVENT_GROUP): absorbed into shellShifts (header).
        shellShifts[guid] = (shellShifts[guid] or 0) + 500
        schedule(guid, "void_blast", 500, function()
            voidBlastTick(creature, guid)
        end)
    end
end

-- C++ EVENT_DARK_SHELL (:105-111): InterruptNonMeleeSpells(true)
-- (bridged no-op twin, ported below); Talk + non-triggered
-- self-cast + 20s re-arm (+ the DelayEvents accumulator).
local function darkShellTick(creature, guid)
    creature:InterruptNonMeleeSpells(true)
    creature:Talk(EMOTE_DARK_SHELL)
    creature:CastSpell(creature, SPELL_DARK_SHELL)
    local shift = shellShifts[guid] or 0
    shellShifts[guid] = 0
    schedule(guid, "dark_shell", 20000 + shift, function()
        darkShellTick(creature, guid)
    end)
end

local function clearState(guid)
    cancelTimers(guid)
    blastCounters[guid] = nil
    shellShifts[guid] = nil
end

-- C++ boss_pandemoniusAI::JustEngagedWith (:76-84).
local function pandemoniusEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    clearState(guid)
    blastCounters[guid] = 0
    shellShifts[guid] = 0
    creature:Talk(SAY_AGGRO)
    schedule(guid, "dark_shell", 20000, function()
        darkShellTick(creature, guid)
    end)
    schedule(guid, "void_blast", {8000, 23000}, function()
        voidBlastTick(creature, guid)
    end)
end

local function pandemoniusLeaveCombat(event, creature)
    clearState(creature:GetGUID())
end

local function pandemoniusReset(event, creature)
    clearState(creature:GetGUID())
end

-- C++ boss_pandemoniusAI::KilledUnit (:66-69): Talk(SAY_KILL).
local function pandemoniusTargetDied(event, creature, victim)
    creature:Talk(SAY_KILL)
end

-- C++ boss_pandemoniusAI::JustDied (:61-64): Talk(SAY_DEATH).
local function pandemoniusDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    clearState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PANDEMONIUS, 1, pandemoniusEnterCombat)
RegisterCreatureEvent(ENTRY_PANDEMONIUS, 2, pandemoniusLeaveCombat)
RegisterCreatureEvent(ENTRY_PANDEMONIUS, 3, pandemoniusTargetDied)
RegisterCreatureEvent(ENTRY_PANDEMONIUS, 4, pandemoniusDied)
RegisterCreatureEvent(ENTRY_PANDEMONIUS, 23, pandemoniusReset)
