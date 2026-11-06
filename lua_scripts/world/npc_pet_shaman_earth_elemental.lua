-- Shaman Earth Elemental — Lua port of
-- src/server/scripts/Pet/pet_shaman.cpp
-- (class npc_pet_shaman_earth_elemental : CreatureScript
-- ("npc_pet_shaman_earth_elemental")).
-- Entry: 15352 (C++-sourced, never guessed: Pet.cpp:998
-- "case 15352: //earth elemental 36213" in InitStatsForLevel;
-- the class itself carries no entry constant — the entry comes
-- from the creature_template ScriptName binding, same evidence
-- class as the shadowfiend 19668 port).
-- Eluna creature events: 4 OnDied, 23 OnReset. Routing:
-- event registration only, deliberately NOT added to
-- boss_ai.go — pet motions never consult BossAI for event
-- dispatch (pet-routing proof in npc_pet_pri_shadowfiend.lua:
-- dispatch is entry-keyed via TriggerCreatureEvent by
-- motion.Entry). registerPetMotion fires fireCreatureSpawned
-- (ev23 then ev5) at pet spawn (pet_cooldowns.go), so the
-- Reset arm lands on spawn as C++ ai->Reset() does.
-- Ported arms (pet_shaman.cpp:44-78, C++ order):
-- - Reset (ev23): _events.Reset() + ScheduleEvent(ANGERED_
--   EARTH, 0s) + ApplySpellImmune(0, IMMUNITY_SCHOOL,
--   SPELL_SCHOOL_MASK_NATURE, true) — op 2 (SharedDefines.h:
--   1428), nature mask 8 (1 << SPELL_SCHOOL_NATURE, 3).
-- - UpdateAI: !UpdateVictim early return (timers frozen out of
--   combat — the 1s pump returns without decrementing when
--   GetVictim() is nil); ANGERED_EARTH fires DoCastVictim
--   (36213), re-armed 5s-20s (urand).
--   DoMeleeAttackIfReady is engine-driven in Go.
-- Pump lifetime: unsummonPet fires no Lua event, so the pump
-- self-cancels when the elemental is no longer the owner's
-- live pet (owner:GetPetGUID() ~= creature GUID — spawnPet
-- sets player.PetGUID, unsummonPet clears it, pets.go:1089/
-- 1591); ev4 (death) and ev23 (re-reset) cancel it too.
-- A stale pump can never ghost-cast: the tick gates on a live
-- victim and the owner-pet probe before any cast.

local SPELL_ANGERED_EARTH = 36213 -- pet_shaman.cpp:26
local ENTRY_EARTH_ELEMENTAL = 15352 -- Pet.cpp:998
local IMMUNITY_SCHOOL = 2 -- SharedDefines.h:1428
local SPELL_SCHOOL_MASK_NATURE = 8 -- 1 << SPELL_SCHOOL_NATURE (3)

local earthState = {}
local earthPump = {}

local function cancelPump(guid)
    local id = earthPump[guid]
    if id then
        RemoveEventById(id)
        earthPump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    earthState[guid] = nil
end

-- C++ UpdateAI (pet_shaman.cpp:60-73) at 1s granularity:
-- !UpdateVictim freezes the timers; ANGERED_EARTH fires on
-- the victim and re-arms 5000+rand(0,15000).
local function combatTick(creature, guid)
    local st = earthState[guid]
    if not st then
        return
    end
    -- Unsummoned/dismissed (no Lua event fires): the owner no
    -- longer carries this pet — stop the pump, drop state.
    local owner = creature:GetOwner()
    if owner == nil or owner:GetPetGUID() ~= guid then
        fullReset(guid)
        return
    end
    if creature:GetVictim() == nil then
        return
    end
    if st.angeredEarth <= 1000 then
        creature:CastSpell(nil, SPELL_ANGERED_EARTH)
        st.angeredEarth = 5000 + math.random(0, 15000)
    else
        st.angeredEarth = st.angeredEarth - 1000
    end
end

-- C++ Reset() (pet_shaman.cpp:52-57): event reset + 0s
-- schedule + nature-school immunity; the 1s pump starts
-- here (fireCreatureSpawned pre-fires ev23 on pet spawn).
RegisterCreatureEvent(ENTRY_EARTH_ELEMENTAL, 23, function(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    creature:ApplySpellImmune(0, IMMUNITY_SCHOOL, SPELL_SCHOOL_MASK_NATURE, true)
    earthState[guid] = { angeredEarth = 0 }
    earthPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ JustDied: no override — the pump cancel is the only
-- death leg (the C++ EventMap dies with the AI).
RegisterCreatureEvent(ENTRY_EARTH_ELEMENTAL, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)
