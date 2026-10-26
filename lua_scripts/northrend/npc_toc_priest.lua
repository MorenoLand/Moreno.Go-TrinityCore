-- npc_toc_priest (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_priestAI (AI_HEALER); C++ CreatureScript("npc_toc_priest")).
-- Base engage arms, the KilledUnit relay, and the Reset equipment leg
-- are ported, plus the bridgeable rotation arms below: psychic scream
-- and mana burn (boss_faction_champions.cpp:1056-1062 schedules,
-- armed on enter combat per the boss_faction_champions.lua precedent).
-- Friendly-select arms stay out (no friendly-enumeration bridge):
-- renew / shield / flash heal / penance (DoSelectLowestHpFriendly
-- 40yd); dispel stays out as a mixed leg (urand(0,1) ? random enemy
-- 30yd : lowest-hp friendly — porting only the enemy half would
-- misfire on the friendly branch).
-- Entries: 34466 Alliance discipline priest / 34447 Horde discipline
-- priest (trial_of_the_crusader.h:187/202 NPC_ALLIANCE_PRIEST_DISCIPLINE
-- / NPC_HORDE_PRIEST_DISCIPLINE).
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
-- (boss_faction_champions.cpp:1063); base Reset EVENT_THREAT schedule
-- (:588-593): UpdatePower + UpdateThreat legs PORTED below (AI_HEALER, no armor
-- read); heroic CC schedule PORTED below (removeCCCheck).

local ENTRY_PRIEST_ALLIANCE = 34466 -- trial_of_the_crusader.h:187
local ENTRY_PRIEST_HORDE = 34447 -- trial_of_the_crusader.h:202

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

local SPELL_RENEW = 66177 -- boss_faction_champions.cpp:79
local SPELL_SHIELD = 66099 -- boss_faction_champions.cpp:80
local SPELL_FLASH_HEAL = 66104 -- boss_faction_champions.cpp:81
local SPELL_DISPEL = 65546 -- boss_faction_champions.cpp:82
local SPELL_PSYCHIC_SCREAM = 65543 -- boss_faction_champions.cpp:83
local SPELL_MANA_BURN = 66100 -- boss_faction_champions.cpp:84
local SPELL_PENANCE = 66097 -- boss_faction_champions.cpp:85
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

-- C++ EnemiesInRange (10.0f) (:693-697): threat-list victims within
-- 10.0 yd 2D reach-subtracted, strict <; GetAITargets is the bridged threat list
-- (boss_faction_champions.lua countCheck precedent).
local function enemiesInRange(creature, dist)
    local count = 0
    local reach = creature:GetCombatReach()
    for _, target in ipairs(creature:GetAITargets()) do
        if creature:GetDistance2d(target) - reach < dist then -- C++ GetDistance2d subtracts both reaches (Object.cpp:1187); target reach band unbridged
            count = count + 1
        end
    end
    return count
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

-- C++ EVENT_HEAL_PSYCHIC_SCREAM (:1101-1105): EnemiesInRange(10) >= 2
-- -> DoCastAOE = self-cast, init {10s,30s}, re-arm {10s,25s}.
local function onPsychicScream(creature, guid)
    if enemiesInRange(creature, 10.0) >= 2 then
        creature:CastSpell(creature, SPELL_PSYCHIC_SCREAM)
    end
    schedule(guid, "scream", {10000, 25000}, function() onPsychicScream(creature, guid) end)
end

-- C++ EVENT_MANA_BURN (:1106-1110): enemy caster, init {15s,30s},
-- re-arm {15s,30s}.
local function onManaBurn(creature, guid)
    local target = firstManaPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_MANA_BURN)
    end
    schedule(guid, "manaburn", {15000, 30000}, function() onManaBurn(creature, guid) end)
end

-- C++ Reset schedules (:1056-1062) armed on enter combat
-- (boss_faction_champions.lua precedent: the EventMap ticks only in
-- combat; canceled on events 2/4/23).
local function scheduleRotation(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "scream", {10000, 30000}, function() onPsychicScream(creature, guid) end)
    schedule(guid, "manaburn", {15000, 30000}, function() onManaBurn(creature, guid) end)
end

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:645-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() — applies to every champion subclass including
-- this priest AI (npc_toc_priestAI : public boss_faction_championsAI,
-- :1049); the npc_toc_druid.lua precedent ports all three bridged arms.
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

local function priestEnterCombat(event, creature, target)
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

local function priestLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function priestDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function priestSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function priestReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_toc_priest::Reset (boss_faction_champions.cpp:1063):
    -- SetEquipmentSlots(false, 49992, EQUIP_NO_CHANGE, EQUIP_NO_CHANGE).
    creature:SetEquipmentSlots(49992, 0xFFFFFFFF, 0xFFFFFFFF)
end

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry priest means Horde players
-- -> Garrosh (34995), Horde-entry priest means Alliance players ->
-- Varian (34990). Trigger event 3, lookup FindNearestCreature 150.0,
-- nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function priestTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_PRIEST_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_PRIEST_ALLIANCE, 1, priestEnterCombat)
RegisterCreatureEvent(ENTRY_PRIEST_ALLIANCE, 2, priestLeaveCombat)
RegisterCreatureEvent(ENTRY_PRIEST_ALLIANCE, 3, priestTargetDied)
RegisterCreatureEvent(ENTRY_PRIEST_ALLIANCE, 4, priestDied)
RegisterCreatureEvent(ENTRY_PRIEST_ALLIANCE, 5, priestSpawn)
RegisterCreatureEvent(ENTRY_PRIEST_ALLIANCE, 23, priestReset)

RegisterCreatureEvent(ENTRY_PRIEST_HORDE, 1, priestEnterCombat)
RegisterCreatureEvent(ENTRY_PRIEST_HORDE, 2, priestLeaveCombat)
RegisterCreatureEvent(ENTRY_PRIEST_HORDE, 3, priestTargetDied)
RegisterCreatureEvent(ENTRY_PRIEST_HORDE, 4, priestDied)
RegisterCreatureEvent(ENTRY_PRIEST_HORDE, 5, priestSpawn)
RegisterCreatureEvent(ENTRY_PRIEST_HORDE, 23, priestReset)
