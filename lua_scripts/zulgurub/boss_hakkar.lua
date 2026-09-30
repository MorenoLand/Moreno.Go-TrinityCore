-- Hakkar the Soulflayer (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_hakkar.cpp
-- (boss_hakkarAI); zulgurub.h:35 (DATA_HAKKAR = 5, end boss), :64
-- (NPC_HAKKAR = 14834). Creature entry: 14834 Hakkar the Soulflayer
-- (C++ ScriptName "boss_hakkar" per AddSC_boss_hakkar;
-- classic.wowhead.com/npc=14834/hakkar-the-soulflayer).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: enter combat arms — blood siphon 24322 at 90s then every
-- 90s on the victim (triggered DoCastVictim, C++-exact) / corrupted
-- blood 24328 at 25s then {30s,45s} on the victim (triggered
-- DoCastVictim, C++-exact) / will of hakkar 24178 at 15s then
-- {25s,35s} (C++ casts it on the MaxThreat player only when more than
-- one unit is fighting; no threat model — the gate is approximated by
-- >1 alive player in the instance and the target by the victim, the
-- highest-threat approximation) / enrage 24318 at 600s then every 90s
-- self-cast only when the aura is missing (DoCast(me) non-triggered,
-- C++-exact HasAura gate) / the five priest-aspect timers: jeklik
-- 24687 at 4s then {10s,14s}, venoxis 24688 at 7s then 8s, marli 24686
-- at 12s then 10s, thekal 24689 at 8s then 15s, arlokk 24690 at 18s
-- then {10s,15s}, all triggered DoCastVictim (C++-exact). Reset cancels
-- timers. Talk(SAY_AGGRO=0) on engage; no other Talk lines in the AI.
-- Deviations from C++: EVENT_CAUSE_INSANITY is disabled upstream (the
-- cast and its re-arm are commented out in the C++ UpdateAI) — not
-- modeled; the aspect timers' instance GetBossState(DATA_X) != DONE
-- gates have no bridge (no instance-script model) — all five aspects
-- are armed unconditionally; the UpdateAI UNIT_STATE_CASTING gate (skip
-- the rest of the event queue while casting) has no UNIT_STATE bridge
-- — timers fire unconditionally (jeklik convention); no instance-script
-- model — boss admission via the luaBossAI shim (_Reset/_JustDied/
-- BossAI::JustEngagedWith and the GetZulGurubAI bookkeeping arms
-- skipped); the at_zulgurub_entrance OnlyOnceAreaTriggerScript (the
-- SAY_ENTRANCE=4 / SAY_PROTECT_ALTAR=3 / SAY_MINION_DESTROY=2 Talk arms,
-- gated on DATA_HAKKAR != DONE and instance->GetCreature) has no Lua
-- area-trigger bridge — it is not registered; SAY_FLEEING=1 is defined
-- in the C++ enum but never used by the AI — not modeled.

local ENTRY_HAKKAR = 14834

local SPELL_BLOOD_SIPHON = 24322
local SPELL_CORRUPTED_BLOOD = 24328
local SPELL_WILL_OF_HAKKAR = 24178
local SPELL_ENRAGE = 24318

local SPELL_ASPECT_OF_JEKLIK = 24687
local SPELL_ASPECT_OF_VENOXIS = 24688
local SPELL_ASPECT_OF_MARLI = 24686
local SPELL_ASPECT_OF_THEKAL = 24689
local SPELL_ASPECT_OF_ARLOKK = 24690

local SAY_AGGRO = 0

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

local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ EVENT_BLOOD_SIPHON: triggered DoCastVictim(24322); re-arm 90s.
local function onBloodSiphon(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BLOOD_SIPHON, true)
    end
    schedule(guid, "bloodsiphon", 90000, function()
        onBloodSiphon(creature, guid)
    end)
end

-- C++ EVENT_CORRUPTED_BLOOD: triggered DoCastVictim(24328); re-arm
-- {30s,45s}.
local function onCorruptedBlood(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CORRUPTED_BLOOD, true)
    end
    schedule(guid, "corruptedblood", math.random(30000, 45000), function()
        onCorruptedBlood(creature, guid)
    end)
end

-- C++ EVENT_WILL_OF_HAKKAR: only when more than one unit is fighting
-- (SelectTargetList MaxThreat, max 2, count > 1), cast on the MaxThreat
-- player. No threat model, no pet scan — the count gate is approximated
-- by >1 alive player in the instance and the target by the victim.
local function onWillOfHakkar(creature, guid)
    if #alivePlayersInInstance(creature) > 1 then
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, SPELL_WILL_OF_HAKKAR, false)
        end
    end
    schedule(guid, "willofhakkar", math.random(25000, 35000), function()
        onWillOfHakkar(creature, guid)
    end)
end

-- C++ EVENT_ENRAGE: non-triggered self-cast 24318 only if the aura is
-- missing; re-arm 90s regardless (C++-exact).
local function onEnrage(creature, guid)
    if not creature:HasAura(SPELL_ENRAGE) then
        creature:CastSpell(creature, SPELL_ENRAGE, false)
    end
    schedule(guid, "enrage", 90000, function()
        onEnrage(creature, guid)
    end)
end

-- C++ aspect timers: triggered DoCastVictim of the priest aspect whose
-- GetBossState(DATA_X) != DONE. No instance-script model — all five are
-- armed unconditionally, C++-exact re-arms otherwise.
local function armAspect(guid, key, creature, spell, first, minR, maxR)
    local function tick()
        local victim = creature:GetVictim()
        if victim then
            creature:CastSpell(victim, spell, true)
        end
        local delay = minR
        if maxR then
            delay = math.random(minR, maxR)
        end
        schedule(guid, key, delay, tick)
    end
    schedule(guid, key, first, tick)
end

local function hakkarResetState(guid)
    cancelTimers(guid)
end

local function hakkarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "bloodsiphon", 90000, function()
        onBloodSiphon(creature, guid)
    end)
    schedule(guid, "corruptedblood", 25000, function()
        onCorruptedBlood(creature, guid)
    end)
    -- EVENT_CAUSE_INSANITY is disabled upstream (commented-out C++),
    -- not modeled.
    schedule(guid, "willofhakkar", 15000, function()
        onWillOfHakkar(creature, guid)
    end)
    schedule(guid, "enrage", 600000, function()
        onEnrage(creature, guid)
    end)
    armAspect(guid, "aspectjeklik", creature, SPELL_ASPECT_OF_JEKLIK, 4000, 10000, 14000)
    armAspect(guid, "aspectvenoxis", creature, SPELL_ASPECT_OF_VENOXIS, 7000, 8000)
    armAspect(guid, "aspectmarli", creature, SPELL_ASPECT_OF_MARLI, 12000, 10000)
    armAspect(guid, "aspectthekal", creature, SPELL_ASPECT_OF_THEKAL, 8000, 15000)
    armAspect(guid, "aspectarlokk", creature, SPELL_ASPECT_OF_ARLOKK, 18000, 10000, 15000)
end

local function hakkarLeaveCombat(event, creature)
    hakkarResetState(creature:GetGUID())
end

local function hakkarDied(event, creature, killer)
    hakkarResetState(creature:GetGUID())
end

local function hakkarReset(event, creature)
    -- C++ Reset's _Reset bookkeeping has no bridge (no instance-script
    -- model) — only the timer cancel is modeled.
    hakkarResetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_HAKKAR, 1, hakkarEnterCombat)
RegisterCreatureEvent(ENTRY_HAKKAR, 2, hakkarLeaveCombat)
RegisterCreatureEvent(ENTRY_HAKKAR, 4, hakkarDied)
RegisterCreatureEvent(ENTRY_HAKKAR, 23, hakkarReset)
