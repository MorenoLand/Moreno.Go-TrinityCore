-- npc_solarium_priest (The Eye zone script)
-- Lua port of src/server/scripts/Outland/TempestKeep/Eye/boss_astromancer.cpp
-- (npc_solarium_priestAI — ScriptedAI, registered by
-- AddSC_boss_high_astromancer_solarian in boss_astromancer.cpp).
-- Entry: NPC_SOLARIUM_PRIEST = 18806 (boss_astromancer.cpp:67).
-- Eluna creature events: 1 OnEnterCombat, 4 OnDied, 5 OnSpawn,
-- 23 OnReset. Melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady. JustEngagedWith is empty in C++;
-- the timers start on combat entry (their initial values come from
-- Reset / Initialize, and they only count while in combat — see
-- below).
-- Verifiable numbers (file's own Spells enum): SPELL_SOLARIUM_
-- GREAT_HEAL = 33387, SPELL_SOLARIUM_HOLY_SMITE = 25054,
-- SPELL_SOLARIUM_ARCANE_TORRENT = 33390 (boss_astromancer.cpp:
-- 57-59). DATA_ASTROMANCER = 4 (the_eye.h:36).
-- Ported arms (C++-exact):
-- - Reset (ev5/ev23): Initialize re-arms heal 9000, holysmite 1,
--   aoesilence 15000; timers cancelled (C++ timers only advance
--   while UpdateVictim succeeds, and Reset re-arms them to the
--   initial values — re-arming on combat entry with the same
--   initials is the observable twin).
-- - Great heal (UpdateAI :461-478): on a 9000ms timer, urand(0, 1)
--   picks the heal target — 0: Solarian via instance GetGuidData
--   (DATA_ASTROMANCER), 1: self. If the target resolves, DoCast
--   (target, 33387) and the timer resets to 9000; if it does not
--   resolve, the timer is NOT reset (C++ leaves healTimer <= diff,
--   so the next UpdateAI tick retries — the 100ms retry below).
-- - Holy smite (UpdateAI :480-486): DoCastVictim(25054), 1ms
--   initial / 4000ms repeat.
-- - Arcane torrent (UpdateAI :488-494): DoCastVictim(33390),
--   15000ms initial / 13000ms repeat.
-- CORRECTION to the tick-291 audit spec (verified against CURRENT
-- C++ before porting): the spec's "add creature:SetGuidData(4) in
-- solarian OnSpawn" is REJECTED — it would fabricate a heal C++
-- never performs. instance_the_eye.cpp GetGuidData (:105-120)
-- has NO case for DATA_ASTROMANCER (4): the switch covers 0, 1,
-- 2, 5, 6, 7, 8 and returns ObjectGuid::Empty for 4, so
-- ObjectAccessor::GetUnit(*me, Empty) is always nullptr and the
-- Solarian leg of the heal NEVER fires in C++ (wrong DATA
-- constant — the mapped key is DATA_HIGH_ASTROMANCER_SOLARIAN
-- = 2). The port reads creature:GetInstanceCreature(4), which
-- returns nil for the unlatched key (lua_creature_events.go —
-- the Go form of the C++ nullptr), so the Solarian leg stays
-- dead exactly as in C++ and only the self leg (urand 1) ever
-- casts. No SetGuidData(4) latch is added anywhere.

local ENTRY_NPC_SOLARIUM_PRIEST = 18806

local SPELL_SOLARIUM_GREAT_HEAL = 33387
local SPELL_SOLARIUM_HOLY_SMITE = 25054
local SPELL_SOLARIUM_ARCANE_TORRENT = 33390

local DATA_ASTROMANCER = 4

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

-- C++ UpdateAI (:461-478): healTimer 9000; urand(0, 1) picks
-- Solarian (GetGuidData(4) — always Empty in C++, see header)
-- or self; cast + reset 9000 only when the target resolves,
-- else retry on the next UpdateAI tick (100ms).
local function healTick(creature, guid)
    if creature:IsInCombat() then
        local target = nil
        if math.random(0, 1) == 0 then
            target = creature:GetInstanceCreature(DATA_ASTROMANCER)
        else
            target = creature
        end
        if target ~= nil then
            creature:CastSpell(target, SPELL_SOLARIUM_GREAT_HEAL)
            schedule(guid, "heal", 9000, function()
                healTick(creature, guid)
            end)
            return
        end
        schedule(guid, "heal", 100, function()
            healTick(creature, guid)
        end)
        return
    end
    schedule(guid, "heal", 9000, function()
        healTick(creature, guid)
    end)
end

-- C++ UpdateAI (:480-486): DoCastVictim holy smite, 1ms
-- initial / 4000ms repeat.
local function holysmiteTick(creature, guid)
    if creature:IsInCombat() then
        creature:CastSpell(nil, SPELL_SOLARIUM_HOLY_SMITE)
    end
    schedule(guid, "holysmite", 4000, function()
        holysmiteTick(creature, guid)
    end)
end

-- C++ UpdateAI (:488-494): DoCastVictim arcane torrent,
-- 15000ms initial / 13000ms repeat.
local function aoesilenceTick(creature, guid)
    if creature:IsInCombat() then
        creature:CastSpell(nil, SPELL_SOLARIUM_ARCANE_TORRENT)
    end
    schedule(guid, "aoesilence", 13000, function()
        aoesilenceTick(creature, guid)
    end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    schedule(guid, "heal", 9000, function()
        healTick(creature, guid)
    end)
    schedule(guid, "holysmite", 1, function()
        holysmiteTick(creature, guid)
    end)
    schedule(guid, "aoesilence", 15000, function()
        aoesilenceTick(creature, guid)
    end)
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_NPC_SOLARIUM_PRIEST, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_NPC_SOLARIUM_PRIEST, 4, onDied)
RegisterCreatureEvent(ENTRY_NPC_SOLARIUM_PRIEST, 5, onReset)
RegisterCreatureEvent(ENTRY_NPC_SOLARIUM_PRIEST, 23, onReset)
