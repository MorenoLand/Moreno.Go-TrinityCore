-- Postmaster Malown (Stratholme) -- Lua port of
-- src/server/scripts/EasternKingdoms/Stratholme/boss_postmaster_malown.cpp
-- (boss_postmaster_malownAI : public BossAI via GetStratholmeAI (BossAI
-- constructor arg TYPE_MALOWN = 7, stratholme.h:35); registered by
-- AddSC_boss_postmaster_malown in eastern_kingdoms_script_loader.cpp
-- (declaration line 133, call line 311; follows timmy_the_cruel :132/
-- 310)). The file holds 1 script: boss_postmaster_malown (BossAI event
-- arms, Talk on kill only; no gossip/quest, no SpellScript/AuraScript
-- loaders; C++ Reset() is empty).
-- Entry: no NPC_ constant in stratholme.h (DB-side ScriptName binding)
-- -- 11143 independently cited (classic.wowhead.com/npc=11143/
-- postmaster-malown; spell 24627 "Summon Postmaster Malown" summons
-- 11143).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- (DoMeleeAttackIfReady).
-- Ported arms (C++-exact for the modeled arms):
-- - JustEngagedWith schedules: WAILINGDEAD 19000ms / BACKHAND 8000ms /
--   CURSEOFWEAKNESS 20000ms / CURSEOFTONGUES 22000ms /
--   CALLOFTHEGRAVE 25000ms.
-- - EVENT_WAILINGDEAD: C++ `if (rand32() % 100 < 65)` (65% cast chance)
--   then DoCastVictim(SPELL_WAILINGDEAD 7713, true); 19000ms re-arm is
--   unconditional either way.
-- - EVENT_BACKHAND: C++ `if (rand32() % 100 < 45)` (45% cast chance)
--   then DoCastVictim(SPELL_BACKHAND 6253, true); the re-arm in C++ is
--   `events.ScheduleEvent(EVENT_WAILINGDEAD, 8s)` — the case schedules
--   EVENT_WAILINGDEAD (not EVENT_BACKHAND), verbatim from the C++ source
--   (50%-complete script quirk); preserved C++-exact here.
-- - EVENT_CURSEOFWEAKNESS: C++ `if (rand32() % 100 < 3)` (3% cast chance)
--   then DoCastVictim(SPELL_CURSEOFWEAKNESS 8552, true); the re-arm in C++
--   is `events.ScheduleEvent(EVENT_WAILINGDEAD, 20s)` — same quirk,
--   preserved C++-exact.
-- - EVENT_CURSEOFTONGUES: C++ `if (rand32() % 100 < 3)` (3% cast chance)
--   then DoCastVictim(SPELL_CURSEOFTONGUES 12889, true); the re-arm in C++
--   is `events.ScheduleEvent(EVENT_WAILINGDEAD, 22s)` — same quirk,
--   preserved C++-exact.
-- - EVENT_CALLOFTHEGRAVE: C++ `if (rand32() % 100 < 5)` (5% cast chance)
--   then DoCastVictim(SPELL_CALLOFTHEGRAVE 17831, true); the re-arm in C++
--   is `events.ScheduleEvent(EVENT_WAILINGDEAD, 25s)` — same quirk,
--   preserved C++-exact.
-- - All five casts are DoCastVictim(spell, true) -> GetVictim +
--   CastSpell(victim, spell, true) (triggered, moroes/maiden convention);
--   nil victim keeps the schedule.
-- - KilledUnit: Talk(SAY_KILL 0) only when the victim is a player
--   (C++ TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
-- Unmodeled (documented-only, no bridges):
-- - BossAI::Reset() (empty body in C++) + BossAI::JustEngagedWith +
--   TYPE_MALOWN bookkeeping (luaBossAI shim).
-- - UpdateAI's `if (!UpdateVictim()) return` gate: engine-driven.
-- - UpdateAI's UNIT_STATE_CASTING gate and post-cast break: no
--   casting-state bridge (barthilas precedent).
-- - GetStratholmeAI wrapper: plain AI factory template, no logic.
-- Verifiable numbers (file's own enums): SPELL_WAILINGDEAD = 7713 /
-- SPELL_BACKHAND = 6253 / SPELL_CURSEOFWEAKNESS = 8552 /
-- SPELL_CURSEOFTONGUES = 12889 / SPELL_CALLOFTHEGRAVE = 17831 /
-- SAY_KILL = 0.

local ENTRY_POSTMASTER_MALOWN = 11143

local SPELL_WAILINGDEAD = 7713
local SPELL_BACKHAND = 6253
local SPELL_CURSEOFWEAKNESS = 8552
local SPELL_CURSEOFTONGUES = 12889
local SPELL_CALLOFTHEGRAVE = 17831

local SAY_KILL = 0

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ DoCastVictim(spell, true) -> GetVictim + CastSpell(victim, spell,
-- true) (triggered, moroes/maiden convention); nil victim keeps the
-- schedule.
local function doCastVictimTriggered(creature, spell)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, spell, true)
    end
end

-- C++ `if (rand32() % 100 < 65)` — 65% cast chance; the 19000ms re-arm is
-- unconditional either way.
local function wailingDeadGate()
    return math.random(0, 99) < 65
end

-- C++ `if (rand32() % 100 < 45)` — 45% cast chance; the C++ re-arm is
-- `events.ScheduleEvent(EVENT_WAILINGDEAD, 8s)` (not EVENT_BACKHAND) —
-- quirk preserved verbatim; schedule() replaces any pending wailingdead
-- timer exactly like EventMap::ScheduleEvent does.
local function onBackhand(creature, guid)
    if math.random(0, 99) < 45 then
        doCastVictimTriggered(creature, SPELL_BACKHAND)
    end
    schedule(guid, "wailingdead", 8000, function() onWailingDead(creature, guid) end)
end

-- C++ `if (rand32() % 100 < 3)` — 3% cast chance; the C++ re-arm is
-- `events.ScheduleEvent(EVENT_WAILINGDEAD, 20s)` — quirk preserved
-- verbatim.
local function onCurseOfWeakness(creature, guid)
    if math.random(0, 99) < 3 then
        doCastVictimTriggered(creature, SPELL_CURSEOFWEAKNESS)
    end
    schedule(guid, "wailingdead", 20000, function() onWailingDead(creature, guid) end)
end

-- C++ `if (rand32() % 100 < 3)` — 3% cast chance; the C++ re-arm is
-- `events.ScheduleEvent(EVENT_WAILINGDEAD, 22s)` — quirk preserved
-- verbatim.
local function onCurseOfTongues(creature, guid)
    if math.random(0, 99) < 3 then
        doCastVictimTriggered(creature, SPELL_CURSEOFTONGUES)
    end
    schedule(guid, "wailingdead", 22000, function() onWailingDead(creature, guid) end)
end

-- C++ `if (rand32() % 100 < 5)` — 5% cast chance; the C++ re-arm is
-- `events.ScheduleEvent(EVENT_WAILINGDEAD, 25s)` — quirk preserved
-- verbatim.
local function onCallOfTheGrave(creature, guid)
    if math.random(0, 99) < 5 then
        doCastVictimTriggered(creature, SPELL_CALLOFTHEGRAVE)
    end
    schedule(guid, "wailingdead", 25000, function() onWailingDead(creature, guid) end)
end

-- C++ EVENT_WAILINGDEAD: 65% gate, then DoCastVictim(SPELL_WAILINGDEAD,
-- true); 19000ms re-arm unconditional.
local function onWailingDead(creature, guid)
    if wailingDeadGate() then
        doCastVictimTriggered(creature, SPELL_WAILINGDEAD)
    end
    schedule(guid, "wailingdead", 19000, function() onWailingDead(creature, guid) end)
end

-- C++ JustEngagedWith schedules: WAILINGDEAD 19s / BACKHAND 8s /
-- CURSEOFWEAKNESS 20s / CURSEOFTONGUES 22s / CALLOFTHEGRAVE 25s.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "wailingdead", 19000, function() onWailingDead(creature, guid) end)
    schedule(guid, "backhand", 8000, function() onBackhand(creature, guid) end)
    schedule(guid, "curse_of_weakness", 20000, function() onCurseOfWeakness(creature, guid) end)
    schedule(guid, "curse_of_tongues", 22000, function() onCurseOfTongues(creature, guid) end)
    schedule(guid, "call_of_the_grave", 25000, function() onCallOfTheGrave(creature, guid) end)
end

-- C++ KilledUnit: Talk(SAY_KILL 0) only when the victim is a player
-- (TYPEID_PLAYER gate -> victim:IsPlayer(), illidan convention).
local function onTargetDied(event, creature, victim)
    if victim:IsPlayer() then
        creature:Talk(SAY_KILL)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onDied(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_POSTMASTER_MALOWN, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_POSTMASTER_MALOWN, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_POSTMASTER_MALOWN, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_POSTMASTER_MALOWN, 4, onDied)
RegisterCreatureEvent(ENTRY_POSTMASTER_MALOWN, 23, onReset)
