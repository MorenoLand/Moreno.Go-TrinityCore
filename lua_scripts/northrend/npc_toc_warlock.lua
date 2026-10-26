-- npc_toc_warlock (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_warlockAI (AI_RANGED); C++ CreatureScript("npc_toc_warlock")).
-- Base engage arms, the KilledUnit relay, and the Reset equipment leg
-- are ported, plus the full DPS rotation below (hellfire / corruption
-- / curse of agony / curse of exhaustion / fear / searing pain /
-- unstable affliction, boss_faction_champions.cpp:1239-1245 schedules,
-- armed on enter combat per the boss_faction_champions.lua precedent)
-- and the JustEngagedWith felhunter summon (:1249, self-cast; the pet
-- AI rides the unbridged summon strand, boomkin Force of Nature
-- precedent). DoSpellAttackIfReady(SHADOW_BOLT) stays out as a
-- documented deviation (boomkin wrath precedent: Go melee is
-- engine-driven, the champion melees between casts instead of
-- shadow-bolt-spamming).
-- Entries: 34474 Alliance warlock / 34450 Horde warlock
-- (trial_of_the_crusader.h:192/207 NPC_ALLIANCE_WARLOCK /
-- NPC_HORDE_WARLOCK).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 5 OnSpawn, 23 OnReset. Melee is engine-driven in Go.
-- Ported arms (C++-exact for all modeled arms):
-- Engage (base boss_faction_championsAI::JustEngagedWith) — DoCast(me,
-- SPELL_ANTI_AOE 68595, true) triggered, SetCombatPulseDelay(5),
-- DoZoneInCombat() (boss_faction_champions.cpp:645-649; setActive has
-- no bridge, controller SetData is instance-gated).
-- KilledUnit relay (base :655-666) — player victim -> faction-leader
-- bare Talk(SAY_KILLED_PLAYER 6); entry-faction inference per the
-- npc_toc_druid.lua precedent (champions spawn opposite-faction to the
-- raid, so an Alliance-entry champion means a Horde raid -> Garrosh).
-- Reset — SetEquipmentSlots(49992, NO_CHANGE, NO_CHANGE)
-- (boss_faction_champions.cpp:1246); base Reset EVENT_THREAT schedule
-- (:588-593): UpdatePower + UpdateThreat legs PORTED below (AI_RANGED, no armor
-- read); heroic CC schedule PORTED below (removeCCCheck).

local ENTRY_WARLOCK_ALLIANCE = 34474 -- trial_of_the_crusader.h:192
local ENTRY_WARLOCK_HORDE = 34450 -- trial_of_the_crusader.h:207

local SPELL_ANTI_AOE = 68595 -- boss_faction_champions.cpp:42

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

local SPELL_HELLFIRE = 65816 -- boss_faction_champions.cpp:98
local SPELL_CORRUPTION = 65810 -- boss_faction_champions.cpp:99
local SPELL_CURSE_OF_AGONY = 65814 -- boss_faction_champions.cpp:100
local SPELL_CURSE_OF_EXHAUSTION = 65815 -- boss_faction_champions.cpp:101
local SPELL_FEAR = 65809 -- boss_faction_champions.cpp:102
local SPELL_SEARING_PAIN = 65819 -- boss_faction_champions.cpp:103
local SPELL_SHADOW_BOLT = 65821 -- boss_faction_champions.cpp:104
local SPELL_UNSTABLE_AFFLICTION = 65812 -- boss_faction_champions.cpp:105
local SPELL_SUMMON_FELHUNTER = 67514 -- boss_faction_champions.cpp:107

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

-- Random alive threat-list entry within maxDist yards (tank
-- included) — boomkin randomPlayerInRange precedent; C++
-- SelectTarget(Random, 0, dist[, playerOnly]) picks from the threat
-- list, lowered via GetAITargets. Corruption/agony/exhaustion use
-- the playerOnly=false default (pets/totems eligible); fear/UA pass
-- playerOnly=true.
local function randomPlayerInRange(creature, maxDist, playersOnly)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if (not playersOnly or plr:GetObjectType() == "Player")
                and not plr:IsDead() then
            if creature:GetDistance(plr) - creature:GetCombatReach() < maxDist then
                candidates[#candidates + 1] = plr
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EnemiesInRange (10.0f) (:693-697): threat-list victims within
-- 10.0 yd 2D, strict <; GetAITargets is the bridged threat list
-- (boss_faction_champions.lua countCheck precedent).
local function enemiesInRange(creature, dist)
    local count = 0
    for _, target in ipairs(creature:GetAITargets()) do
        if creature:GetDistance2d(target) - creature:GetCombatReach() < dist then
            count = count + 1
        end
    end
    return count
end

-- C++ EVENT_HELLFIRE (:1270): EnemiesInRange(10) >= 2 ->
-- DoCastAOE = self-cast, init {10s,30s}, re-arm {10s,30s}.
local function onHellfire(creature, guid)
    if enemiesInRange(creature, 10.0) >= 2 then
        creature:CastSpell(creature, SPELL_HELLFIRE)
    end
    schedule(guid, "hellfire", {10000, 30000}, function() onHellfire(creature, guid) end)
end

-- C++ EVENT_CORRUPTION (:1275-1279): random target 30yd, init
-- {2s,5s}, re-arm {15s,25s}.
local function onCorruption(creature, guid)
    local target = randomPlayerInRange(creature, 30.0)
    if target then
        creature:CastSpell(target, SPELL_CORRUPTION)
    end
    schedule(guid, "corruption", {15000, 25000}, function() onCorruption(creature, guid) end)
end

-- C++ EVENT_CURSE_OF_AGONY (:1280-1284): random target 30yd, init
-- {5s,10s}, re-arm {20s,35s}.
local function onCurseOfAgony(creature, guid)
    local target = randomPlayerInRange(creature, 30.0)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_AGONY)
    end
    schedule(guid, "agony", {20000, 35000}, function() onCurseOfAgony(creature, guid) end)
end

-- C++ EVENT_CURSE_OF_EXHAUSTION (:1285-1289): random target 30yd,
-- init {5s,10s}, re-arm {20s,35s}.
local function onCurseOfExhaustion(creature, guid)
    local target = randomPlayerInRange(creature, 30.0)
    if target then
        creature:CastSpell(target, SPELL_CURSE_OF_EXHAUSTION)
    end
    schedule(guid, "exhaustion", {20000, 35000}, function() onCurseOfExhaustion(creature, guid) end)
end

-- C++ EVENT_FEAR (:1290-1294): random target 20yd playerOnly, init
-- {5s,15s}, re-arm {5s,20s}.
local function onFear(creature, guid)
    local target = randomPlayerInRange(creature, 20.0, true)
    if target then
        creature:CastSpell(target, SPELL_FEAR)
    end
    schedule(guid, "fear", {5000, 20000}, function() onFear(creature, guid) end)
end

-- C++ EVENT_SEARING_PAIN (:1295-1298): DoCastVictim, init {5s,12s},
-- re-arm {10s,25s}.
local function onSearingPain(creature, guid)
    creature:CastSpell(nil, SPELL_SEARING_PAIN)
    schedule(guid, "searing", {10000, 25000}, function() onSearingPain(creature, guid) end)
end

-- C++ EVENT_UNSTABLE_AFFLICTION (:1299-1303): random target 30yd
-- playerOnly, init {7s,15s}, re-arm {10s,25s}.
local function onUnstableAffliction(creature, guid)
    local target = randomPlayerInRange(creature, 30.0, true)
    if target then
        creature:CastSpell(target, SPELL_UNSTABLE_AFFLICTION)
    end
    schedule(guid, "ua", {10000, 25000}, function() onUnstableAffliction(creature, guid) end)
end

-- C++ Reset schedules (:1239-1245) armed on enter combat
-- (boss_faction_champions.lua precedent: the EventMap ticks only in
-- combat; canceled on events 2/4/23).
local function scheduleRotation(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "hellfire", {10000, 30000}, function() onHellfire(creature, guid) end)
    schedule(guid, "corruption", {2000, 5000}, function() onCorruption(creature, guid) end)
    schedule(guid, "agony", {5000, 10000}, function() onCurseOfAgony(creature, guid) end)
    schedule(guid, "exhaustion", {5000, 10000}, function() onCurseOfExhaustion(creature, guid) end)
    schedule(guid, "fear", {5000, 15000}, function() onFear(creature, guid) end)
    schedule(guid, "searing", {5000, 12000}, function() onSearingPain(creature, guid) end)
    schedule(guid, "ua", {7000, 15000}, function() onUnstableAffliction(creature, guid) end)
end

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:645-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() — applies to every champion subclass including
-- this warlock AI (npc_toc_warlockAI : public boss_faction_championsAI,
-- :1232); the npc_toc_druid.lua precedent ports all three bridged arms.
-- C++ boss_faction_championsAI::UpdateThreat (:611-619) on EVENT_THREAT
-- (:721-724, Reset init 5s at :591, then 4s repeats): per threat-list
-- ref, player victims only get ScaleThreat(0) then AddThreat(1000000 *
-- CalculateThreat). For AI_RANGED, dist_mod = 1 and armor_mod = 0, so the
-- amount is 1000000 * 30000 / (health + 1) with no distance or armor
-- read. Lowered via GetAITargets (the bridged threat list) +
-- per-victim ModifyThreatByPercent(-100) (ScaleThreat x0) + AddThreat,
-- in C++ per-victim order; creature entries are left untouched, as in
-- C++ (no IsDead filter in C++ either).
local function threatCheck(creature, guid)
    -- C++ UpdatePower (:621-624), first in the EVENT_THREAT case
    -- (:721-724): mana only, ModifyPower(MANA, GetMaxPower(MANA) / 3)
    -- (C++ integer division; boss_faction_champions.lua main-file idiom).
    if creature:GetPowerType() == 0 then
        creature:ModifyPower(0, math.floor(creature:GetMaxPower(0) / 3))
    end
    for _, victim in ipairs(creature:GetAITargets()) do
        if victim:GetObjectType() == "Player" then
            creature:ModifyThreatByPercent(victim, -100)
            creature:AddThreat(victim, 1000000.0 * (30000.0 / (victim:GetHealth() + 1)))
        end
    end
    schedule(guid, "threat", 4000, function() threatCheck(creature, guid) end)
end

-- C++ boss_faction_championsAI::RemoveCC (:628-635) on heroic
-- EVENT_REMOVE_CC (:726-734): breakable-CC gate, then
-- RemoveAurasByType STUN 12 / FEAR 7 / ROOT 26 / PACIFY 25 / CONFUSE 5
-- in C++ order; 2min re-arm after a strip, 3s re-check otherwise.
-- Full arm note in boss_faction_champions.lua (removeCCCheck).
local function removeCCCheck(creature, guid)
    if creature:HasBreakableByDamageCrowdControlAura() then
        creature:RemoveAurasByType(12)
        creature:RemoveAurasByType(7)
        creature:RemoveAurasByType(26)
        creature:RemoveAurasByType(25)
        creature:RemoveAurasByType(5)
        schedule(guid, "removecc", 120000, function() removeCCCheck(creature, guid) end)
    else
        schedule(guid, "removecc", 3000, function() removeCCCheck(creature, guid) end)
    end
end

local function warlockEnterCombat(event, creature, target)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
    -- C++ npc_toc_warlockAI::JustEngagedWith (:1249-1252): base arms
    -- first, then DoCast(SPELL_SUMMON_FELHUNTER) self-cast; the pet AI
    -- rides the unbridged summon strand.
    creature:CastSpell(creature, SPELL_SUMMON_FELHUNTER)
    scheduleRotation(creature)
    local guid = creature:GetGUID()
    schedule(guid, "threat", 5000, function() threatCheck(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "removecc", 5000, function() removeCCCheck(creature, guid) end)
    end
end

local function warlockLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function warlockDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function warlockSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function warlockReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_toc_warlock::Reset (boss_faction_champions.cpp:1246):
    -- SetEquipmentSlots(false, 49992, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE).
    creature:SetEquipmentSlots(49992, 0xFFFFFFFF, 0xFFFFFFFF)
end

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry warlock means Horde players
-- -> Garrosh (34995), Horde-entry warlock means Alliance players ->
-- Varian (34990). Trigger event 3, lookup FindNearestCreature 150.0,
-- nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function warlockTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_WARLOCK_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_WARLOCK_ALLIANCE, 1, warlockEnterCombat)
RegisterCreatureEvent(ENTRY_WARLOCK_ALLIANCE, 2, warlockLeaveCombat)
RegisterCreatureEvent(ENTRY_WARLOCK_ALLIANCE, 3, warlockTargetDied)
RegisterCreatureEvent(ENTRY_WARLOCK_ALLIANCE, 4, warlockDied)
RegisterCreatureEvent(ENTRY_WARLOCK_ALLIANCE, 5, warlockSpawn)
RegisterCreatureEvent(ENTRY_WARLOCK_ALLIANCE, 23, warlockReset)

RegisterCreatureEvent(ENTRY_WARLOCK_HORDE, 1, warlockEnterCombat)
RegisterCreatureEvent(ENTRY_WARLOCK_HORDE, 2, warlockLeaveCombat)
RegisterCreatureEvent(ENTRY_WARLOCK_HORDE, 3, warlockTargetDied)
RegisterCreatureEvent(ENTRY_WARLOCK_HORDE, 4, warlockDied)
RegisterCreatureEvent(ENTRY_WARLOCK_HORDE, 5, warlockSpawn)
RegisterCreatureEvent(ENTRY_WARLOCK_HORDE, 23, warlockReset)
