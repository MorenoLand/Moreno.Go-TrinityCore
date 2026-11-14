-- Infernal Attacker (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_infernal_attackerAI; registered in AddSC_shadowmoon_valley()).
-- Entry: NPC_INFERNAL_ATTACKER = 21419 (InvisInfernalCaster enum,
-- zone_shadowmoon_valley.cpp:58), TDB-verified ('Infernal Attacker',
-- ScriptName npc_infernal_attacker).
-- Eluna creature events: 4 OnDied, 5 OnSpawn, 14 OnHitBySpell,
-- 22 OnSummoned, 23 OnReset.
-- The C++ overrides no JustEngagedWith/KilledUnit arms (UpdateAI is
-- victim-check + melee only, engine-driven in Go).
-- Ported arms (C++-exact for all modeled arms; line refs are
-- zone_shadowmoon_valley.cpp):
-- Reset (:147-151) — SetDisplayId(MODEL_INVISIBLE 20577) +
-- MoveRandom(5.0f) (MotionMaster::MoveRandom, bridged as
-- creature:MoveRandom; Go launches a single walk spline instead of
-- the generator's stepping cycle — the documented MoveRandom delta).
-- Runs on spawn (5) and reset (23), the beryl-sorcerer precedent.
-- IsSummonedBy (:153-157) — if the summoner is a creature, latch its
-- GUID. Ported via ev22 (CreatureEventOnSummoned, fire
-- lua_creature_events.go:8108-8109, args (event, creature, summoner)).
-- JustDied (:159-163) — resolve the latched caster by GUID and call
-- caster->AI()->SetData(TYPE_INFERNAL 1, DATA_DIED 1), which re-summons
-- via the caster (npc_invis_infernal_caster.lua ev41). Ported via ev4
-- with the C++ GetCreature nil-guard (GetCreatureByGUID
-- lua_creature_events.go:4383; SetData fires ev41, :800).
-- SpellHit (:165-173) — spell 37277 (SPELL_SUMMON_INFERNAL) only:
-- RemoveFlag(UNIT_FIELD_FLAGS 59, UNIT_FLAG_PACIFIED 0x20000 |
-- UNIT_FLAG_NOT_SELECTABLE 0x2000000) + SetImmuneToPC(false)
-- (= RemoveFlag(59, 0x100), Unit.cpp:8440-8449 idiom) +
-- SetDisplayId(MODEL_INFERNAL 17312). No caster gate in C++ (the
-- caster arg is ignored), so the Lua arm gates on spellId only.

local ENTRY_INFERNAL_ATTACKER = 21419

local MODEL_INVISIBLE = 20577
local MODEL_INFERNAL = 17312
local SPELL_SUMMON_INFERNAL = 37277

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_PACIFIED = 0x20000
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000
local UNIT_FLAG_IMMUNE_TO_PC = 0x100

local TYPE_INFERNAL = 1
local DATA_DIED = 1

local attackerState = {}

local function infernalAttackerSpawnOrReset(_, creature)
    attackerState[creature:GetGUIDLow()] = { casterGUID = nil }
    creature:SetDisplayId(MODEL_INVISIBLE)
    creature:MoveRandom(5.0)
end

local function infernalAttackerSummoned(_, creature, summoner)
    local st = attackerState[creature:GetGUIDLow()]
    if not st then
        return
    end
    if summoner and summoner:GetObjectType() == "Creature" then
        st.casterGUID = summoner:GetGUIDLow()
    end
end

local function infernalAttackerDied(_, creature)
    local guid = creature:GetGUIDLow()
    local st = attackerState[guid]
    attackerState[guid] = nil
    if not st or not st.casterGUID then
        return
    end
    local caster = creature:GetCreatureByGUID(st.casterGUID)
    if caster then
        caster:SetData(TYPE_INFERNAL, DATA_DIED)
    end
end

local function infernalAttackerSpellHit(_, creature, _, spellId)
    if spellId ~= SPELL_SUMMON_INFERNAL then
        return
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED + UNIT_FLAG_NOT_SELECTABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    creature:SetDisplayId(MODEL_INFERNAL)
end

RegisterCreatureEvent(ENTRY_INFERNAL_ATTACKER, 5, infernalAttackerSpawnOrReset)
RegisterCreatureEvent(ENTRY_INFERNAL_ATTACKER, 23, infernalAttackerSpawnOrReset)
RegisterCreatureEvent(ENTRY_INFERNAL_ATTACKER, 14, infernalAttackerSpellHit)
RegisterCreatureEvent(ENTRY_INFERNAL_ATTACKER, 22, infernalAttackerSummoned)
RegisterCreatureEvent(ENTRY_INFERNAL_ATTACKER, 4, infernalAttackerDied)
