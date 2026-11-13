-- Storm Peaks: Brunnhildar Prisoner — Lua port of
-- src/server/scripts/Northrend/zone_storm_peaks.cpp
-- (class npc_brunnhildar_prisoner : CreatureScript("npc_brunnhildar_prisoner")).
-- Entry: 29639 (creature_template.sql, ScriptName
-- 'npc_brunnhildar_prisoner'; no NPC_ constant exists in the C++
-- file). Quest: freeing the Brunnhildar prisoners with Ice Lance
-- from a captured proto-drake.
-- Ported arms:
-- - Reset (zone_storm_peaks.cpp:208-212, event 23): freed = false +
--   self-cast Ice Prison (54894), triggered. ev23 fires on spawn
--   (fireCreatureSpawned: Reset then Spawn, lua_creature_events.go)
--   and on evade, matching C++ JustAppeared()->Reset() (:214-217)
--   + spawn/evade Reset.
-- - SpellHit (zone_storm_peaks.cpp:228-247, event 14): Ice Lance
--   (55046) only; the caster's drake must have a free prisoner
--   seat, then self-cast Free Prisoner (55048, triggered), cast
--   Ride Drake (55074, triggered) on the drake, self-cast Shard
--   Impact (55047, triggered), freed = true.
-- DELTA (seat gate): GetAvailableSeatCount has no bridge on the
-- Lua vehicle object (lua_vehicle.go exposes GetPassenger only),
-- so the gate is emulated by checking prisoner seats 1-3 vacant
-- via GetPassenger (npc_freed_protodrake iterates seats 1-3 for
-- prisoners, zone_storm_peaks.cpp:319-321; seat 0 is the player
-- driver, so an empty seat among 1-3 is exactly a free seat).
-- DELTA (caster): C++ reads the caster's own vehicle kit, so the
-- caster is the drake; Go's player-cast path fires ev14 with the
-- player as caster (spells.go:19914), so a player caster resolves
-- the drake via GetVehicleBase(), while a creature caster is used
-- directly. The Ride Drake target is the drake in both cases.
-- Unmodeled (documented-only): UpdateAI (:219-227) despawns a
-- freed prisoner with no vehicle — no ev7 port in this strip; the
-- freed latch is kept per-GUID so a future UpdateAI port can read
-- it.
-- DORMANT until Spell.dbc ships 54894/55046/55048/55074/55047:
-- ev14 fires only when spellKnown (boss_ai.go) and creature
-- CastSpell is spellKnown-gated.

local ENTRY_BRUNNHILDAR_PRISONER = 29639

local SPELL_ICE_PRISON = 54894 -- zone_storm_peaks.cpp:183
local SPELL_ICE_LANCE = 55046 -- zone_storm_peaks.cpp:184
local SPELL_FREE_PRISONER = 55048 -- zone_storm_peaks.cpp:185
local SPELL_RIDE_DRAKE = 55074 -- zone_storm_peaks.cpp:186
local SPELL_SHARD_IMPACT = 55047 -- zone_storm_peaks.cpp:187

-- C++ npc_brunnhildar_prisonerAI::freed (zone_storm_peaks.cpp:206):
-- per-creature latch, false on Reset, true on a freeing SpellHit.
local freed = {}

-- C++ Reset (zone_storm_peaks.cpp:208-212): Initialize() then
-- self-cast Ice Prison, triggered.
local function onReset(event, creature)
    freed[creature:GetGUIDLow()] = false
    creature:CastSpell(creature, SPELL_ICE_PRISON, true)
end

-- C++ SpellHit (zone_storm_peaks.cpp:228-247): Ice Lance only; the
-- caster must be a unit whose drake has a free prisoner seat.
local function onSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_ICE_LANCE then
        return
    end
    if caster == nil then
        return
    end
    local drake = nil
    if caster:GetObjectType() == "Player" then
        drake = caster:GetVehicleBase()
    elseif caster:GetObjectType() == "Creature" then
        drake = caster
    end
    if drake == nil then
        return
    end
    local kit = drake:GetVehicleKit()
    if kit == nil then
        return
    end
    local seatFree = false
    for seat = 1, 3 do
        if kit:GetPassenger(seat) == nil then
            seatFree = true
            break
        end
    end
    if not seatFree then
        return
    end
    creature:CastSpell(creature, SPELL_FREE_PRISONER, true)
    creature:CastSpell(drake, SPELL_RIDE_DRAKE, true)
    creature:CastSpell(creature, SPELL_SHARD_IMPACT, true)
    freed[creature:GetGUIDLow()] = true
end

RegisterCreatureEvent(ENTRY_BRUNNHILDAR_PRISONER, 23, onReset)
RegisterCreatureEvent(ENTRY_BRUNNHILDAR_PRISONER, 14, onSpellHit)
