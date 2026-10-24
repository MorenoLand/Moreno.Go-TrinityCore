-- Mageguard Dalaran (Dalaran, trespasser guards) --
-- Lua port of npc_mageguard_dalaranAI in
-- src/server/scripts/Northrend/zone_dalaran.cpp (lines 62-146).
-- Entries (C++-verifiable, zone_dalaran.cpp :55-56):
-- NPC_SILVER_COVENANT_GUARDIAN_MAGE = 29254,
-- NPC_SUNREAVER_GUARDIAN_MAGE = 29255. Both share this one AI; the
-- C++ switch on GetEntry() only selects the teleport spell, so the
-- constructor arm below is entry-identical.
-- Eluna creature events: 5 OnSpawn.
-- Ported arm (C++ constructor :66-72): SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NON_ATTACKABLE) + ApplySpellImmune(0, IMMUNITY_DAMAGE,
-- SPELL_SCHOOL_NORMAL, true) + ApplySpellImmune(0, IMMUNITY_DAMAGE,
-- SPELL_SCHOOL_MASK_MAGIC, true). C++ Reset, JustEngagedWith,
-- AttackStart and UpdateAI are all empty, so spawn is the only bearer.
-- Unmodeled: MoveInLineOfSight (:82-141) — the trespasser teleport.
-- ev27 fires from the aggro-engage branch only (engage-only bridge,
-- insufficient for a sighting arm), and its gates need IsInBack (not
-- in the handler-creature fallback), zone/team reads and
-- GetClosestCreature indoor/outdoor branching; the teleport casts
-- (SPELL_TRESPASSER_A/H) would also fire on engage, not on sighting.
-- Porting the constructor arm alone strands nothing: the guards keep
-- their non-attackable, damage-immune posture with or without it.

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

local SPELL_SCHOOL_NORMAL = 1
local SPELL_SCHOOL_MASK_MAGIC = 126

local ENTRIES_MAGEGUARD_DALARAN = { 29254, 29255 }

local function mageguardSpawn(event, creature)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:ApplySpellImmune(0, 3, SPELL_SCHOOL_NORMAL, true)
    creature:ApplySpellImmune(0, 3, SPELL_SCHOOL_MASK_MAGIC, true)
end

for _, entry in ipairs(ENTRIES_MAGEGUARD_DALARAN) do
    RegisterCreatureEvent(entry, 5, mageguardSpawn)
end
