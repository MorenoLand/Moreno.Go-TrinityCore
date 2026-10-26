-- npc_toc_shaman (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_shamanAI (AI_HEALER); C++ CreatureScript("npc_toc_shaman")).
-- Base engage arms, the KilledUnit relay, and the Reset equipment leg
-- are ported, plus the bridgeable rotation arms below: bloodlust /
-- heroism, hex, earth shock (boss_faction_champions.cpp:859-865
-- schedules, armed on enter combat per the boss_faction_champions.lua
-- precedent). Friendly-select arms stay out (no friendly-enumeration
-- bridge): healing wave / riptide / spirit cleanse
-- (DoSelectLowestHpFriendly 40yd) and earth shield
-- (SelectRandomFriendlyMissingBuff).
-- Entries: 34470 Alliance restoration shaman / 34444 Horde restoration
-- shaman (trial_of_the_crusader.h:191/206 NPC_ALLIANCE_SHAMAN_RESTORATION
-- / NPC_HORDE_SHAMAN_RESTORATION).
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
-- (boss_faction_champions.cpp:866); base Reset EVENT_THREAT schedule
-- (:588-593): UpdatePower + UpdateThreat legs PORTED below (AI_HEALER, no armor
-- read); heroic CC schedule PORTED below (removeCCCheck).

local ENTRY_SHAMAN_ALLIANCE = 34470 -- trial_of_the_crusader.h:191
local ENTRY_SHAMAN_HORDE = 34444 -- trial_of_the_crusader.h:206

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

local SPELL_HEALING_WAVE = 66055 -- boss_faction_champions.cpp:56
local SPELL_RIPTIDE = 66053 -- boss_faction_champions.cpp:57
local SPELL_SPIRIT_CLEANSE = 66056 -- boss_faction_champions.cpp:58
local SPELL_HEROISM = 65983 -- boss_faction_champions.cpp:59
local SPELL_BLOODLUST = 65980 -- boss_faction_champions.cpp:60
local SPELL_HEX = 66054 -- boss_faction_champions.cpp:61
local SPELL_EARTH_SHIELD = 66063 -- boss_faction_champions.cpp:62
local SPELL_EARTH_SHOCK = 65973 -- boss_faction_champions.cpp:63
local AURA_EXHAUSTION = 57723 -- boss_faction_champions.cpp:64
local AURA_SATED = 57724 -- boss_faction_champions.cpp:65
local POWER_MANA = 0 -- moroes.lua:81 precedent

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

-- Random alive player on the same map/instance, excluding the tank
-- (victim) — boomkin NonTankTargetSelector precedent; C++ picks from
-- the threat list (playerOnly=true default), lowered via
-- GetAITargets.
local function randomNonTankPlayer(creature)
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player"
                and not plr:IsDead() and plr:GetGUID() ~= victimGuid then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ boss_faction_championsAI::SelectEnemyCaster (:678, bool ignored):
-- first PvE combat ref whose player has POWER_MANA; GetAITargets is
-- the bridged threat list (threat-sorted, same set).
local function firstManaPlayer(creature)
    for _, target in ipairs(creature:GetAITargets()) do
        if target:GetObjectType() == "Player" and not target:IsDead() and target:GetPowerType() == POWER_MANA then
            return target
        end
    end
    return nil
end

-- C++ EVENT_HEAL_BLOODLUST_HEROISM (:899): the GetFaction gate is
-- entry-gated here (no faction bridge; druid entry-faction precedent)
-- — Alliance entry casts Heroism unless Exhaustion (57723), Horde
-- entry casts Bloodlust unless Sated (57724); DoCastAOE = self-cast
-- (phase_hunter precedent). Init 20s, re-arm 5min.
local function onBloodlustHeroism(creature, guid)
    if creature:GetEntry() == ENTRY_SHAMAN_ALLIANCE then
        if not creature:HasAura(AURA_EXHAUSTION) then
            creature:CastSpell(creature, SPELL_HEROISM)
        end
    else
        if not creature:HasAura(AURA_SATED) then
            creature:CastSpell(creature, SPELL_BLOODLUST)
        end
    end
    schedule(guid, "bloodlust", 300000, function() onBloodlustHeroism(creature, guid) end)
end

-- C++ EVENT_HEX (:912-916): random non-tank target, init {5s,30s},
-- re-arm {15s,30s}.
local function onHex(creature, guid)
    local target = randomNonTankPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_HEX)
    end
    schedule(guid, "hex", {15000, 30000}, function() onHex(creature, guid) end)
end

-- C++ EVENT_HEAL_EARTH_SHOCK (:922): enemy caster, init {5s,30s},
-- re-arm {10s,20s}.
local function onEarthShock(creature, guid)
    local target = firstManaPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_EARTH_SHOCK)
    end
    schedule(guid, "earthshock", {10000, 20000}, function() onEarthShock(creature, guid) end)
end

-- C++ Reset schedules (:859-865) armed on enter combat
-- (boss_faction_champions.lua precedent: the EventMap ticks only in
-- combat; canceled on events 2/4/23).
local function scheduleRotation(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "bloodlust", 20000, function() onBloodlustHeroism(creature, guid) end)
    schedule(guid, "hex", {5000, 30000}, function() onHex(creature, guid) end)
    schedule(guid, "earthshock", {5000, 30000}, function() onEarthShock(creature, guid) end)
end

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:645-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() — applies to every champion subclass including
-- this shaman AI (npc_toc_shamanAI : public boss_faction_championsAI,
-- :852); the npc_toc_druid.lua precedent ports all three bridged arms.
-- C++ boss_faction_championsAI::UpdateThreat (:611-619) on EVENT_THREAT
-- (:721-724, Reset init 5s at :591, then 4s repeats): per threat-list
-- ref, player victims only get ScaleThreat(0) then AddThreat(1000000 *
-- CalculateThreat). For AI_HEALER, dist_mod = 1 and armor_mod = 0, so the
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

local function shamanEnterCombat(event, creature, target)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
    scheduleRotation(creature)
    local guid = creature:GetGUID()
    schedule(guid, "threat", 5000, function() threatCheck(creature, guid) end)
    if creature:IsHeroic() then
        schedule(guid, "removecc", 5000, function() removeCCCheck(creature, guid) end)
    end
end

local function shamanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function shamanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function shamanSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function shamanReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_toc_shaman::Reset (boss_faction_champions.cpp:866):
    -- SetEquipmentSlots(false, 49992, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE).
    creature:SetEquipmentSlots(49992, 0xFFFFFFFF, 0xFFFFFFFF)
end

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry shaman means Horde players
-- -> Garrosh (34995), Horde-entry shaman means Alliance players ->
-- Varian (34990). Trigger event 3, lookup FindNearestCreature 150.0,
-- nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function shamanTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_SHAMAN_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_SHAMAN_ALLIANCE, 1, shamanEnterCombat)
RegisterCreatureEvent(ENTRY_SHAMAN_ALLIANCE, 2, shamanLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAMAN_ALLIANCE, 3, shamanTargetDied)
RegisterCreatureEvent(ENTRY_SHAMAN_ALLIANCE, 4, shamanDied)
RegisterCreatureEvent(ENTRY_SHAMAN_ALLIANCE, 5, shamanSpawn)
RegisterCreatureEvent(ENTRY_SHAMAN_ALLIANCE, 23, shamanReset)

RegisterCreatureEvent(ENTRY_SHAMAN_HORDE, 1, shamanEnterCombat)
RegisterCreatureEvent(ENTRY_SHAMAN_HORDE, 2, shamanLeaveCombat)
RegisterCreatureEvent(ENTRY_SHAMAN_HORDE, 3, shamanTargetDied)
RegisterCreatureEvent(ENTRY_SHAMAN_HORDE, 4, shamanDied)
RegisterCreatureEvent(ENTRY_SHAMAN_HORDE, 5, shamanSpawn)
RegisterCreatureEvent(ENTRY_SHAMAN_HORDE, 23, shamanReset)
