-- npc_toc_priest (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_priestAI (AI_HEALER); C++ CreatureScript("npc_toc_priest")).
-- Currently only the base engage arms, the KilledUnit relay, and the
-- Reset equipment leg are ported: the healer rotation (renew / shield /
-- flash heal / dispel / psychic scream / mana burn / penance,
-- boss_faction_champions.cpp:1056-1062 schedules) is a future unit.
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
-- (boss_faction_champions.cpp:1063); base Reset threat/CC schedules
-- (:588-593) ride the unported threat/heroic machines.

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

-- C++ base boss_faction_championsAI::JustEngagedWith
-- (boss_faction_champions.cpp:645-649): DoCast(me, SPELL_ANTI_AOE, true)
-- triggered, SetCombatPulseDelay(5), setActive (no bridge),
-- DoZoneInCombat() — applies to every champion subclass including
-- this priest AI (npc_toc_priestAI : public boss_faction_championsAI,
-- :1049); the npc_toc_druid.lua precedent ports all three bridged arms.
local function priestEnterCombat(event, creature, target)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
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
