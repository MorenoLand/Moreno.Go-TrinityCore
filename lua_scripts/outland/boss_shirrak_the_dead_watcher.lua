-- Shirrak the Dead Watcher (Auchenai Crypts, Auchindoun) — Lua port of
-- src/server/scripts/Outland/Auchindoun/AuchenaiCrypts/boss_shirrak_the_dead_watcher.cpp
-- (2 CreatureScripts: boss_shirrak_the_dead_watcher (ScriptedAI via
-- GetAuchenaiCryptsAI) + npc_focus_fire (ScriptedAI); both
-- registered from inside AddSC_boss_shirrak_the_dead_watcher()
-- (boss_shirrak_the_dead_watcher.cpp:227-231); loader decl 20 /
-- call 143 per outland_script_loader.cpp — the FIRST call of the
-- "// Auchindoun - Auchenai Crypts" sub-block in
-- AddOutlandScripts(), immediately before
-- AddSC_boss_exarch_maladaar() (call 144) — verified from the
-- loader this run; sole-source verified: whole-server-tree grep
-- for "AddSC_boss_shirrak_the_dead_watcher" hits
-- boss_shirrak_the_dead_watcher.cpp (+ the loader decl/call lines)
-- only).
-- Entry: 18371 — DB-side (no NPC_ constant for the boss in the
-- file); verified this run against the wowhead TBC Classic NPC
-- page (/tbc/npc=18371/shirrak-the-dead-watcher — page title
-- "Shirrak the Dead Watcher - NPC - TBC Classic", first boss of
-- Auchenai Crypts).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 4 OnDied, 5 OnSpawn, 23 OnReset, 37 OnRemove.
-- Ported arms (C++-exact for all modeled arms):
-- boss_shirrak_the_dead_watcherAI::UpdateAI InhibitMagic block
-- (:119-139) — runs BEFORE the UpdateVictim gate, i.e. also out of
-- combat: ported as an OnSpawn-started repeating timer (event 5,
-- immediate first fire like the C++ 0 init, then
-- randtime(3000, 3999) — the muru event-5 precedent). For each
-- player in GetPlayersInWorld() on the same map+instance, not
-- dead, within 45y (the maiden_of_virtue filter precedent):
-- RemoveAurasDueToSpell(SPELL_INHIBITMAGIC 32264) then AddAura —
-- player:RemoveAura / player:AddAura (lua_player.go:211/219).
-- DEVIATION (documented, engine-limited): the C++ distance tiers
-- re-apply the aura 1-4x (dist<45 / <35 / <25 / <15); the Go
-- applyAura refreshes rather than stacking (spells.go
-- applyAuraFromCaster stops the existing aura), so the Lua port
-- applies a single stack per tick — the aura is present at the
-- right range/refresh cadence, the 1-4 stack scaling is not.
-- EVENT AttractMagic (:143-149) — DoCast(me, SPELL_ATTRACTMAGIC
-- 32265) 2-arg (non-triggered) self-cast; 28s init, 30s re-arm;
-- the C++ OVERWRITES Carnivorousbite_Timer = 1500 (not a re-arm —
-- ported as schedule("carnivorous", 1500), replacing the pending
-- timer).
-- EVENT CarnivorousBite (:152-157) — DoCast(me,
-- SPELL_CARNIVOROUSBITE 36383) 2-arg self-cast; 10s init/re-arm.
-- boss_shirrak_the_dead_watcherAI::JustEngagedWith — empty in C++
-- (:104-105): nothing ported (the combat timers start on event 1
-- to mirror the C++ UpdateVictim-gated block).
-- boss_shirrak_the_dead_watcherAI::Reset — Initialize() re-latch:
-- the Lua per-GUID timer tables reset on 2/4/23 (the gargolmar
-- precedent); the inhibit timer restarts (immediate fire) on 23
-- to mirror the C++ Inhibitmagic_Timer = 0 re-latch.
-- PORTED arms (C++-exact for the modeled legs):
-- boss_shirrak_the_dead_watcherAI::UpdateAI FocusFire block
-- (:150-163) — PORTED 2026-10-23 (focusfireTick, event 1; init 17000,
-- re-arm 15000 + rand32() % 5000): SelectTarget(Random, 1) = random pick
-- among the threat list EXCLUDING the top-threat entry (UnitAI.h:162-174;
-- the ikiss offset-1 precedent via GetAITargets, threat-sorted), then the
-- C++ player-type + IsAlive gates — then SummonCreature(NPC_FOCUS_FIRE
-- 18374) at the target's position, TEMPSUMMON_TIMED_DESPAWN 5500ms +
-- Talk(EMOTE_FOCUSED 0, target) (C++ arm order). The stale "no
-- SummonCreature bridge (shirrak precedent)" claim was retracted —
-- creature:SummonCreature is bridged (lua_creature_events.go:728). The
-- FocusedTargetGUID latch feeds only the unbridged AttackStart leg —
-- not kept.
-- npc_focus_fire (18374) — registered for JustSummoned (event 19, the
-- yor precedent): the SPELL_FOCUS_FIRE_VISUAL 42075 non-triggered
-- self-cast + SetFaction(summoner faction, pending-slot idiom) legs are
-- PORTED 2026-10-23; the UNIT_STATE_ROOT leg ports 2026-10-24 via
-- creature:SetRooted (lua_creature_events.go); the AttackStart
-- (FocusedTargetGUID) leg PORTED this run via EngageWithTarget
-- (triggerCreatureAggro fires event 1, so the fiery machine below
-- runs); SetLevel has no bridge, stays out.
-- npc_focus_fire UpdateAI fiery machine — PORTED this run as a
-- 18374 event-1 pump: FieryBlast_Timer 3000+rand(1000) init,
-- DoCast(me, SPELL_FIERY_BLAST 32302) self, then 1000ms; the
-- fiery1/fiery2 bools cap it at exactly 2 casts (first cast clears
-- fiery1, second clears fiery2 which gates the machine).
-- DoMeleeAttackIfReady — no melee bridge (the alar precedent).
-- DOCUMENTED-ONLY (in this header; entries 18371 + 18374 registered):
-- SetLevel leg of JustSummoned (no SetLevel bridge in
-- lua_creature_events.go / object.go / lua_player.go).

local ENTRY_SHIRRAK = 18371 -- DB-side; wowhead-TBC-verified this run

local SPELL_INHIBITMAGIC   = 32264 -- boss_shirrak_the_dead_watcher.cpp:32
local SPELL_ATTRACTMAGIC   = 32265 -- boss_shirrak_the_dead_watcher.cpp:33
local SPELL_CARNIVOROUSBITE = 36383 -- boss_shirrak_the_dead_watcher.cpp:34
local SPELL_FOCUS_FIRE_VISUAL = 42075 -- boss_shirrak_the_dead_watcher.cpp:38
local SPELL_FIERY_BLAST = 32302 -- boss_shirrak_the_dead_watcher.cpp:36
local EMOTE_FOCUSED = 0 -- Talk id, boss_shirrak_the_dead_watcher.cpp:36
local NPC_FOCUS_FIRE = 18374 -- boss_shirrak_the_dead_watcher.cpp:43

local timers = {}
local focusedTarget = {} -- per-shirrak-GUID FocusedTargetGUID (C++ :105)
local fieryCount = {} -- per-focus-fire-GUID fiery1/fiery2 cast count

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

-- C++ InhibitMagic block (:119-139): alive players within 45y get
-- the aura removed and re-applied each tick. Stack tiers
-- documented as engine-limited in the header.
local function inhibitTick(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, 45) then
            p:RemoveAura(SPELL_INHIBITMAGIC)
            p:AddAura(SPELL_INHIBITMAGIC)
        end
    end
    schedule(guid, "inhibit", {3000, 3999}, function()
        inhibitTick(creature, guid)
    end)
end

-- C++ CarnivorousBite (:152-157).
local function carnivorousTick(creature, guid)
    creature:CastSpell(creature, SPELL_CARNIVOROUSBITE)
    schedule(guid, "carnivorous", 10000, function()
        carnivorousTick(creature, guid)
    end)
end

-- C++ AttractMagic (:143-149): self-cast + 30s re-arm, and the
-- C++ overwrites Carnivorousbite_Timer = 1500.
local function attractTick(creature, guid)
    creature:CastSpell(creature, SPELL_ATTRACTMAGIC)
    schedule(guid, "carnivorous", 1500, function()
        carnivorousTick(creature, guid)
    end)
    schedule(guid, "attract", 30000, function()
        attractTick(creature, guid)
    end)
end

-- C++ FocusFire block (boss_shirrak_the_dead_watcher.cpp:150-163):
-- SelectTarget(Random, 1) — random pick among the threat list EXCLUDING
-- the top-threat entry (UnitAI.h:162-174; the ikiss offset-1 precedent —
-- GetAITargets is threat-sorted, lua_creature_events.go:1018), then the
-- C++ player-type + IsAlive gates — then SummonCreature(NPC_FOCUS_FIRE
-- 18374) at the target's position, TEMPSUMMON_TIMED_DESPAWN 5500ms +
-- Talk(EMOTE_FOCUSED 0, target) (C++ arm order); re-arm
-- 15000 + rand32() % 5000. The pick is stored in focusedTarget so
-- JustSummoned can AttackStart it (C++ FocusedTargetGUID).
local function focusfireTick(creature, guid)
    local candidates = {}
    local targets = creature:GetAITargets()
    for i = 2, #targets do
        local t = targets[i]
        if t and t:GetObjectType() == "Player" and not t:IsDead() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates > 0 then
        local target = candidates[math.random(#candidates)]
        focusedTarget[guid] = target:GetGUID()
        creature:SummonCreature(NPC_FOCUS_FIRE, target:GetX(), target:GetY(),
            target:GetZ(), 0, 2, 5500)
        creature:Talk(EMOTE_FOCUSED, target)
    end
    schedule(guid, "focusfire", 15000 + math.random(0, 4999), function()
        focusfireTick(creature, guid)
    end)
end

-- C++ JustSummoned (boss_shirrak_the_dead_watcher.cpp:94-107), entry-gated
-- on the SUMMON: event 19 fires on the SUMMONER's entry bindings with
-- (event, summoner, summon) (lua_creature_events.go: fireCreatureSummoned)
-- — the prior port registered it on the summoned entry with a 2-arg
-- handler, so it never fired; fixed 2026-10-24 to the summoner entry +
-- 3-arg handler. Legs: SPELL_FOCUS_FIRE_VISUAL 42075 non-triggered
-- self-cast + SetFaction(summoner faction); UNIT_STATE_ROOT ports via
-- creature:SetRooted (2026-10-24); AttackStart(FocusedTargetGUID)
-- PORTED this run via EngageWithTarget on the stored pick (the
-- focus fire's event-1 fiery pump then runs); SetLevel has no
-- bridge, stays out.
local function focusFireSummoned(event, creature, summon)
    if summon and summon:GetEntry() == NPC_FOCUS_FIRE then
        summon:CastSpell(summon, SPELL_FOCUS_FIRE_VISUAL)
        summon:SetFaction(creature:GetFaction())
        summon:SetRooted(true)
        local tguid = focusedTarget[creature:GetGUID()]
        if tguid ~= nil then
            local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
            for _, p in ipairs(GetPlayersInWorld()) do
                if p:GetGUID() == tguid and p:GetMapId() == mapId
                        and p:GetInstanceId() == instanceId then
                    summon:EngageWithTarget(p)
                    break
                end
            end
        end
    end
end

-- C++ npc_focus_fireAI::UpdateAI (:197-218): FieryBlast_Timer
-- 3000+rand(1000) init; victim-gated (the engage above supplies
-- the victim); self-cast SPELL_FIERY_BLAST 32302, then 1000ms;
-- exactly 2 casts (fiery1 then fiery2 latch off).
local function fieryTick(creature, guid)
    creature:CastSpell(creature, SPELL_FIERY_BLAST)
    local n = (fieryCount[guid] or 0) + 1
    fieryCount[guid] = n
    if n < 2 then
        schedule(guid, "fiery", 1000, function()
            fieryTick(creature, guid)
        end)
    else
        fieryCount[guid] = nil
    end
end

local function focusFireEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fiery", 3000 + math.random(0, 999), function()
        fieryTick(creature, guid)
    end)
end

local function focusFireCleanup(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    fieryCount[guid] = nil
end

local function cancelKeys(guid, keys)
    local per = timers[guid]
    if per then
        for _, key in ipairs(keys) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
end

local combatKeys = { "attract", "carnivorous", "focusfire" }

-- C++ UpdateVictim-gated combat timers start here (event 1); the
-- C++ JustEngagedWith body is empty. The inhibit timer is
-- spawn-owned (event 5), not combat-owned — it keeps running.
local function shirrakEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelKeys(guid, combatKeys)
    schedule(guid, "attract", 28000, function()
        attractTick(creature, guid)
    end)
    schedule(guid, "carnivorous", 10000, function()
        carnivorousTick(creature, guid)
    end)
    -- C++ Initialize(): FocusFire_Timer = 17000.
    schedule(guid, "focusfire", 17000, function()
        focusfireTick(creature, guid)
    end)
end

local function shirrakLeaveCombat(event, creature)
    cancelKeys(creature:GetGUID(), combatKeys)
end

local function shirrakDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_shirrak_the_dead_watcherAI::Reset -> Initialize():
-- timers re-latched; the inhibit timer restarts with an immediate
-- fire (C++ Inhibitmagic_Timer = 0).
local function shirrakReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    inhibitTick(creature, guid)
end

-- C++ ctor -> Initialize(): the inhibit loop starts at spawn
-- (UpdateAI runs pre-combat in C++).
local function shirrakSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    inhibitTick(creature, guid)
end

local function shirrakRemove(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SHIRRAK, 1, shirrakEnterCombat)
RegisterCreatureEvent(ENTRY_SHIRRAK, 2, shirrakLeaveCombat)
RegisterCreatureEvent(ENTRY_SHIRRAK, 4, shirrakDied)
RegisterCreatureEvent(ENTRY_SHIRRAK, 5, shirrakSpawn)
RegisterCreatureEvent(ENTRY_SHIRRAK, 23, shirrakReset)
RegisterCreatureEvent(ENTRY_SHIRRAK, 37, shirrakRemove)
-- NPC_FOCUS_FIRE 18374 exists only via the FocusFire summon above; its
-- JustSummoned visual/faction/root/engage setup is ported (the yor
-- precedent) on the summoner's event-19 binding, and its fiery
-- machine runs on its own event-1 binding below.
RegisterCreatureEvent(ENTRY_SHIRRAK, 19, focusFireSummoned)
RegisterCreatureEvent(NPC_FOCUS_FIRE, 1, focusFireEnterCombat)
RegisterCreatureEvent(NPC_FOCUS_FIRE, 2, focusFireCleanup)
RegisterCreatureEvent(NPC_FOCUS_FIRE, 4, focusFireCleanup)
