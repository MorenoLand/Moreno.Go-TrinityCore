-- npc_toc_shaman (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_shamanAI (AI_HEALER); C++ CreatureScript("npc_toc_shaman")).
-- Currently only the base engage arms, the KilledUnit relay, and the
-- Reset equipment leg are ported: the healer rotation (healing wave /
-- riptide / spirit cleanse / bloodlust-heroism / hex / earth shield /
-- earth shock, boss_faction_champions.cpp:859-865 schedules) is a
-- future unit.
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
-- (boss_faction_champions.cpp:866); base Reset threat/CC schedules
-- (:588-593) ride the unported threat/heroic machines.

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

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:645-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() — applies to every champion subclass including
-- this shaman AI (npc_toc_shamanAI : public boss_faction_championsAI,
-- :852); the npc_toc_druid.lua precedent ports all three bridged arms.
local function shamanEnterCombat(event, creature, target)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
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
