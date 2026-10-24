-- npc_unstable_sphere (The Oculus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/boss_drakos.cpp
-- npc_unstable_sphereAI (ScriptedAI, registered from AddSC_boss_drakos).
-- Entry 28166 is NPC_UNSTABLE_SPHERE in the file's local enum
-- (boss_drakos.cpp:31) and the CreatureScript name is
-- "npc_unstable_sphere" (boss_drakos.cpp:159-162); the boss port
-- already summons this entry live (boss_drakos.lua bombTick), so the
-- entry is the summon's template by construction — the old
-- "entry-unverifiable, no registration" note in boss_drakos.lua and
-- boss_ai.go is retracted.
-- Deviations from C++: MoveRandom(40.0f) stays out (no motion
-- bridge); the sphere holds position instead of wandering. The
-- C++ Reset re-run on evade is modeled as the event-23 re-seed
-- below (alive-gated, prowlerReset convention); the sphere is
-- passive and never enters combat, so the evade path is inert.

local ENTRY_UNSTABLE_SPHERE = 28166

local REACT_PASSIVE = 0

local SPELL_UNSTABLE_SPHERE_PASSIVE = 50756
local SPELL_UNSTABLE_SPHERE_TIMER = 50758
local SPELL_UNSTABLE_SPHERE_PULSE = 50757

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

-- C++ UpdateAI pulseTimer (boss_drakos.cpp:188-197): 3s init, 3s
-- repeat, non-triggered self-cast 50757. The C++ arm has no
-- UpdateVictim gate; the Lua pump runs from spawn/reset instead,
-- since timers only exist once seeded.
local function pulseTick(creature, guid)
    creature:CastSpell(creature, SPELL_UNSTABLE_SPHERE_PULSE)
    schedule(guid, "pulse", 3000, function()
        pulseTick(creature, guid)
    end)
end

-- C++ Reset (boss_drakos.cpp:173-186): SetReactState(REACT_PASSIVE) +
-- MoveRandom(40.0f, stays out) + AddAura(50756, self) + AddAura(50758,
-- self) + DespawnOrUnsummon(19s); ctor Initialize() seeds the 3s
-- pulse timer, so the pulse starts at spawn too.
local function sphereReset(creature, guid)
    creature:SetReactState(REACT_PASSIVE)
    creature:AddAura(SPELL_UNSTABLE_SPHERE_PASSIVE)
    creature:AddAura(SPELL_UNSTABLE_SPHERE_TIMER)
    schedule(guid, "pulse", 3000, function()
        pulseTick(creature, guid)
    end)
    -- C++ DespawnOrUnsummon(19s) as a one-shot CreateLuaEvent
    -- (arlokk 4s-despawn idiom); the timer re-arms nowhere else.
    schedule(guid, "despawn", 19000, function()
        creature:Despawn()
    end)
end

local function sphereSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    sphereReset(creature, guid)
end

local function sphereResetEvent(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if creature:IsAlive() then
        sphereReset(creature, guid)
    end
end

local function sphereLeaveOrDied(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_UNSTABLE_SPHERE, 5, sphereSpawn)
RegisterCreatureEvent(ENTRY_UNSTABLE_SPHERE, 23, sphereResetEvent)
RegisterCreatureEvent(ENTRY_UNSTABLE_SPHERE, 2, sphereLeaveOrDied)
RegisterCreatureEvent(ENTRY_UNSTABLE_SPHERE, 4, sphereLeaveOrDied)
