-- Shaman Fire Elemental — Lua port of
-- src/server/scripts/Pet/pet_shaman.cpp
-- (class npc_pet_shaman_fire_elemental : CreatureScript
-- ("npc_pet_shaman_fire_elemental")).
-- Entry: 15438 (C++-sourced, never guessed: Pet.cpp:1006
-- "case 15438: //fire elemental" in InitStatsForLevel; the
-- class itself carries no entry constant — the entry comes
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
-- Ported arms (pet_shaman.cpp:87-145, C++ order):
-- - Reset (ev23): _events.Reset() + ScheduleEvent(FIRENOVA,
--   5s-20s) + ScheduleEvent(FIREBLAST, 5s-20s) +
--   ScheduleEvent(FIRESHIELD, 0s) + ApplySpellImmune(0,
--   IMMUNITY_SCHOOL, SPELL_SCHOOL_MASK_FIRE, true) — op 2
--   (SharedDefines.h:1428), fire mask 4 (1 << SPELL_SCHOOL_
--   FIRE, 2).
-- - UpdateAI: !UpdateVictim early return (timers frozen out of
--   combat — the 1s pump returns without decrementing when
--   GetVictim() is nil); HasUnitState(UNIT_STATE_CASTING)
--   return (timers frozen while casting — the gruul IsCasting
--   defer idiom); then all expired events fire in switch order
--   (FireNova 12470 / FireShield 13376 / FireBlast 57984,
--   DoCastVictim each): Nova/Blast re-arm 5s-20s (urand),
--   Shield re-arms 2s. DoMeleeAttackIfReady is engine-driven.
-- Pump lifetime: unsummonPet fires no Lua event, so the pump
-- self-cancels when the elemental is no longer the owner's
-- live pet (owner:GetPetGUID() ~= creature GUID — spawnPet
-- sets player.PetGUID, unsummonPet clears it, pets.go:1089/
-- 1591); ev4 (death) and ev23 (re-reset) cancel it too.
-- A stale pump can never ghost-cast: the tick gates on a live
-- victim and the owner-pet probe before any cast.

local SPELL_FIRENOVA = 12470 -- pet_shaman.cpp:28
local SPELL_FIREBLAST = 57984 -- pet_shaman.cpp:27
local SPELL_FIRESHIELD = 13376 -- pet_shaman.cpp:29
local ENTRY_FIRE_ELEMENTAL = 15438 -- Pet.cpp:1006
local IMMUNITY_SCHOOL = 2 -- SharedDefines.h:1428
local SPELL_SCHOOL_MASK_FIRE = 4 -- 1 << SPELL_SCHOOL_FIRE (2)

local fireState = {}
local firePump = {}

local function cancelPump(guid)
    local id = firePump[guid]
    if id then
        RemoveEventById(id)
        firePump[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    fireState[guid] = nil
end

-- C++ UpdateAI (pet_shaman.cpp:100-138) at 1s granularity:
-- casting gate first (timers frozen), then !UpdateVictim
-- (timers frozen), then all expired events in C++ switch
-- order (Nova, Shield, Blast).
local function combatTick(creature, guid)
    local st = fireState[guid]
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
    -- C++ HasUnitState(UNIT_STATE_CASTING) return (pet_shaman.
    -- cpp:103-104): the gruul IsCasting() defer idiom — the
    -- 1s pump re-fires with every timer intact.
    if creature:IsCasting() then
        return
    end
    if creature:GetVictim() == nil then
        return
    end
    if st.fireNova <= 1000 then
        creature:CastSpell(nil, SPELL_FIRENOVA)
        st.fireNova = 5000 + math.random(0, 15000)
    else
        st.fireNova = st.fireNova - 1000
    end
    if st.fireShield <= 1000 then
        creature:CastSpell(nil, SPELL_FIRESHIELD)
        st.fireShield = 2000
    else
        st.fireShield = st.fireShield - 1000
    end
    if st.fireBlast <= 1000 then
        creature:CastSpell(nil, SPELL_FIREBLAST)
        st.fireBlast = 5000 + math.random(0, 15000)
    else
        st.fireBlast = st.fireBlast - 1000
    end
end

-- C++ Reset() (pet_shaman.cpp:93-99): event reset + Nova/
-- Blast 5s-20s + Shield 0s + fire-school immunity; the 1s
-- pump starts here (fireCreatureSpawned pre-fires ev23 on
-- pet spawn).
RegisterCreatureEvent(ENTRY_FIRE_ELEMENTAL, 23, function(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    creature:ApplySpellImmune(0, IMMUNITY_SCHOOL, SPELL_SCHOOL_MASK_FIRE, true)
    fireState[guid] = {
        fireNova = 5000 + math.random(0, 15000),
        fireBlast = 5000 + math.random(0, 15000),
        fireShield = 0,
    }
    firePump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end)

-- C++ JustDied: no override — the pump cancel is the only
-- death leg (the C++ EventMap dies with the AI).
RegisterCreatureEvent(ENTRY_FIRE_ELEMENTAL, 4, function(_, creature)
    fullReset(creature:GetGUID())
end)
