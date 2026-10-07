-- Gelihast (Blackfathom Deeps) — Lua port of
-- src/server/scripts/Kalimdor/BlackfathomDeeps/boss_gelihast.cpp
-- (boss_gelihastAI : public BossAI(creature, DATA_GELIHAST); AddSC_boss_gelihast
-- registers the one script; kalimdor loader decl 21 / call 134 per
-- kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms boss_gelihast.cpp as the sole
-- source of the "boss_gelihast" script name (loader lines only otherwise).
-- Entry: blackfathom_deeps.h BFDCreatureIds names NO Gelihast entry and
-- instance_blackfathom_deeps.cpp's OnCreatureCreate cases only Kelris 4832 /
-- Lorgus 12902 — zero corroborating C++ usage of any Gelihast entry anywhere
-- in src/server. Registered on the DB ScriptName "boss_gelihast" tie plus
-- wowhead corroboration (www.wowhead.com/npc=6243/gelihast, cited in
-- bug-tracker-for-wod#57 and chromiecraft#2793 alongside BFD bosses, and
-- quest=26892 "Deep in the Deeps" lists NPC #6243 as Gelihast) — jeklik/
-- vishas precedent; the creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++ has no
-- UNIT_STATE_CASTING gate in this boss's UpdateAI.
-- Ported arms:
-- - JustEngagedWith: Throw Net 6533 DoCastVictim (jeklik GetVictim +
--   CastSpell convention, non-triggered — C++ DoCastVictim default args ->
--   TRIGGERED_NONE), init {2s,4s} -> repeat {4s,7s} (C++-exact).
-- Unmodeled (documented-only, no bridges): the BossAI::JustEngagedWith
-- instance bookkeeping leg — no instance-script model (admission only via
-- the luaBossAI shim); the GetBlackfathomDeepsAI -> GetInstanceAI leg
-- (instance-script model blocked, standing). C++ has no Reset / DamageTaken
-- / Talk arms.

local ENTRY_GELIHAST = 6243

local SPELL_THROW_NET = 6533

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

-- C++ EVENT_THROW_NET: DoCastVictim, non-triggered (jeklik convention).
local function onThrowNet(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_THROW_NET)
    end
    schedule(guid, "thrownet", math.random(4000, 7000), function()
        onThrowNet(creature, guid)
    end)
end

local function gelihastLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ JustEngagedWith: BossAI::JustEngagedWith (no bridge) +
-- ScheduleEvent(EVENT_THROW_NET, 2s, 4s).
local function gelihastEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "thrownet", math.random(2000, 4000), function()
        onThrowNet(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_GELIHAST, 1, gelihastEnterCombat)
RegisterCreatureEvent(ENTRY_GELIHAST, 2, gelihastLeaveCombat)
RegisterCreatureEvent(ENTRY_GELIHAST, 4, gelihastLeaveCombat)
RegisterCreatureEvent(ENTRY_GELIHAST, 23, gelihastLeaveCombat)
