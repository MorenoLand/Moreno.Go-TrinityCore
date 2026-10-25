-- Outhouse bunny (Doing Your Duty, quest 12227) --
-- Lua port of src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- npc_outhouse_bunny (lines 273-345).
-- Creature entry: 27326 (NPC_OUTHOUSE_BUNNY,
-- src/server/scripts/World/go_scripts.cpp:585).
-- Spells: SPELL_OUTHOUSE_GROANS 48382, SPELL_CAMERA_SHAKE 47533,
-- SPELL_DUST_FIELD 48329.
-- Sounds: SOUND_FEMALE 12671, SOUND_MALE 12670 — ported via
-- creature:PlayDirectSound (DoPlaySoundToSet is source->PlayDirectSound,
-- ScriptedCreature.cpp:188-201).
-- Eluna creature events: 14 OnHitBySpell, 23 OnReset. Melee is
-- engine-driven in Go.
-- Fight shape (C++-exact for all modeled arms):
-- SpellHit(14): caster (WorldObject) -> ToUnit() must be non-nil;
-- spell 48382 only: ++_counter (per-creature, reset to 0 on Reset);
-- if _counter < 5 -> DoCast(caster, 47533, TRIGGERED) else _counter = 0;
-- then unconditionally DoCast(me, 48329, TRIGGERED), then the
-- gender-gated DoPlaySoundToSet (female 12671 / male 12670).
-- Gender latch: creature:SetData/GetData bridged 2026-10-24
-- (CreatureAI::SetData transport, lua_creature_events.go); the bunny
-- reads type 1 as gender (zone_grizzly_hills.cpp:296). Reset re-inits
-- _gender to 0 (male, :290-293). The go_amberpine_outhouse select
-- caller (go_scripts.cpp:624) is still unported — go:CastSpell has no
-- GO-caster executor — so live flow never sets gender and GetData
-- reads the C++ default 0 (male), exactly as C++ behaves when the
-- outhouse is never used.

local ENTRY_OUTHOUSE_BUNNY = 27326

local SPELL_OUTHOUSE_GROANS = 48382
local SPELL_CAMERA_SHAKE = 47533
local SPELL_DUST_FIELD = 48329

local GENDER_MALE = 0
local GENDER_FEMALE = 1
local SOUND_MALE = 12670
local SOUND_FEMALE = 12671

local counters = {}

local function bunnyReset(_, creature)
    -- zone_grizzly_hills.cpp:290-293: Reset re-initializes _counter and
    -- _gender to 0.
    counters[creature:GetGUID()] = nil
    creature:SetData(1, GENDER_MALE)
end

local function bunnySpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_OUTHOUSE_GROANS or caster == nil then
        return
    end
    -- zone_grizzly_hills.cpp:298-323: counter / camera-shake / dust-field arm.
    -- C++ SpellHit drops a non-unit caster (caster->ToUnit() nil); the Go
    -- event-14 fire path only ever passes a player or creature caster.
    local guid = creature:GetGUID()
    local counter = counters[guid] or 0
    counter = counter + 1
    if counter < 5 then
        creature:CastSpell(caster, SPELL_CAMERA_SHAKE, true)
    else
        counter = 0
    end
    counters[guid] = counter
    creature:CastSpell(creature, SPELL_DUST_FIELD, true)
    -- zone_grizzly_hills.cpp:312-323: gender-gated sound, after the dust
    -- cast as in C++ order. GENDER_NONE (2) plays nothing (no C++ arm).
    local gender = creature:GetData(1)
    if gender == GENDER_FEMALE then
        creature:PlayDirectSound(SOUND_FEMALE)
    elseif gender == GENDER_MALE then
        creature:PlayDirectSound(SOUND_MALE)
    end
end

RegisterCreatureEvent(ENTRY_OUTHOUSE_BUNNY, 14, bunnySpellHit)
RegisterCreatureEvent(ENTRY_OUTHOUSE_BUNNY, 23, bunnyReset)
