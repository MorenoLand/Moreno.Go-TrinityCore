-- npc_toc_enh_shaman (Trial of the Crusader, Faction Champions) — Lua port of
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/boss_faction_champions.cpp
-- (class npc_toc_enh_shamanAI (AI_MELEE); C++ CreatureScript("npc_toc_enh_shaman")).
-- Currently only the base engage arms, the KilledUnit relay, and the
-- Reset equipment leg are ported: the melee rotation (earth shock /
-- lava lash / stormstrike / bloodlust-heroism / totem deploy / windfury,
-- boss_faction_champions.cpp:1933-1938 schedules) and the totem machine
-- (DeployTotem / JustSummoned / SummonedCreatureDespawn, Initialize
-- totem counters) are future units.
-- Entries: 34463 Alliance enhancement shaman / 34455 Horde enhancement
-- shaman (trial_of_the_crusader.h:190/205 NPC_ALLIANCE_SHAMAN_ENHANCEMENT
-- / NPC_HORDE_SHAMAN_ENHANCEMENT).
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
-- Reset — SetEquipmentSlots(51803, 48013, NO_CHANGE)
-- (boss_faction_champions.cpp:1941); base Reset threat/CC schedules
-- (:588-593) ride the unported threat/heroic machines, and the Reset
-- summons.DespawnAll() (:1942) is vacuous in-model (no totem summons
-- are tracked until the totem machine ports).

local ENTRY_ENH_SHAMAN_ALLIANCE = 34463 -- trial_of_the_crusader.h:190
local ENTRY_ENH_SHAMAN_HORDE = 34455 -- trial_of_the_crusader.h:205

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
-- this enhancement shaman AI (npc_toc_enh_shamanAI : public
-- boss_faction_championsAI, :1916); the npc_toc_druid.lua precedent
-- ports all three bridged arms.
local function enhShamanEnterCombat(event, creature, target)
    cancelTimers(creature:GetGUID())
    creature:CastSpell(creature, SPELL_ANTI_AOE, true)
    creature:SetCombatPulseDelay(5) -- boss_faction_champions.cpp:647
    creature:DoZoneInCombat() -- boss_faction_champions.cpp:649, in arm order after pulse delay
end

local function enhShamanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function enhShamanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function enhShamanSpawn(event, creature)
    cancelTimers(creature:GetGUID())
end

local function enhShamanReset(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ npc_toc_enh_shaman::Reset (boss_faction_champions.cpp:1941):
    -- SetEquipmentSlots(false, 51803, 48013, EQUIP_NO_CHANGE).
    creature:SetEquipmentSlots(51803, 48013, 0xFFFFFFFF)
end

-- C++ boss_faction_championsAI::KilledUnit relay (see
-- boss_faction_champions.lua): player victim -> faction-leader bare
-- Talk(SAY_KILLED_PLAYER 6); Alliance-entry enhancement shaman means
-- Horde players -> Garrosh (34995), Horde-entry enhancement shaman
-- means Alliance players -> Varian (34990). Trigger event 3, lookup
-- FindNearestCreature 150.0, nil-guarded.
local ENTRY_VARIAN = 34990
local ENTRY_GARROSH = 34995
local SAY_KILLED_PLAYER = 6

local function enhShamanTargetDied(event, creature, victim)
    if victim:GetObjectType() ~= "Player" then
        return
    end
    local leaderEntry = ENTRY_GARROSH
    if creature:GetEntry() == ENTRY_ENH_SHAMAN_HORDE then
        leaderEntry = ENTRY_VARIAN
    end
    local leader = creature:FindNearestCreature(leaderEntry, 150.0)
    if leader ~= nil then
        leader:Talk(SAY_KILLED_PLAYER)
    end
end

RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 1, enhShamanEnterCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 2, enhShamanLeaveCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 3, enhShamanTargetDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 4, enhShamanDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 5, enhShamanSpawn)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_ALLIANCE, 23, enhShamanReset)

RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 1, enhShamanEnterCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 2, enhShamanLeaveCombat)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 3, enhShamanTargetDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 4, enhShamanDied)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 5, enhShamanSpawn)
RegisterCreatureEvent(ENTRY_ENH_SHAMAN_HORDE, 23, enhShamanReset)
