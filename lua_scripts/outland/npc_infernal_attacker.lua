-- Infernal Attacker (Shadowmoon Valley) --
-- Lua port of src/server/scripts/Outland/zone_shadowmoon_valley.cpp
-- (npc_infernal_attackerAI; registered in AddSC_shadowmoon_valley()).
-- Entry: NPC_INFERNAL_ATTACKER = 21419 (InvisInfernalCaster enum,
-- zone_shadowmoon_valley.cpp:56), C++-verifiable, so registration
-- proceeds (the phase-hunter precedent).
-- Eluna creature events: 5 OnSpawn, 14 OnHitBySpell, 23 OnReset.
-- The C++ overrides no JustEngagedWith/KilledUnit arms (UpdateAI is
-- victim-check + melee only, engine-driven in Go).
-- Ported arms (C++-exact for all modeled arms; line refs are
-- zone_shadowmoon_valley.cpp):
-- Reset (:152-156) — SetDisplayId(MODEL_INVISIBLE 20577) +
-- MoveRandom(5.0f) (MotionMaster::MoveRandom, bridged as
-- creature:MoveRandom; Go launches a single walk spline instead of
-- the generator's stepping cycle — the documented MoveRandom delta).
-- Runs on spawn (5) and reset (23), the beryl-sorcerer precedent.
-- SpellHit (:169-177) — spell 37277 (SPELL_SUMMON_INFERNAL) only:
-- RemoveFlag(UNIT_FIELD_FLAGS 59, UNIT_FLAG_PACIFIED 0x20000 |
-- UNIT_FLAG_NOT_SELECTABLE 0x2000000) + SetImmuneToPC(false)
-- (= RemoveFlag(59, 0x100), Unit.cpp:8440-8449 idiom) +
-- SetDisplayId(MODEL_INFERNAL 17312). No caster gate in C++ (the
-- caster arg is ignored), so the Lua arm gates on spellId only.
-- The stale "no SpellHit bridge" claim on the shadowmoon block in
-- boss_ai.go is retracted: event 14 fires live
-- (engine/world/lua_creature_events.go).
-- DOCUMENTED-ONLY (no bridges):
-- IsSummonedBy (:158-162) — the caster-GUID latch feeds only the
-- JustDied leg below; Lua has no IsSummonedBy event.
-- JustDied (:164-167) — caster->AI()->SetData(TYPE_INFERNAL 1,
-- DATA_DIED 1) re-summons via the caster: the caster
-- (npc_invis_infernal_caster) carries no C++ entry constant
-- (DB ScriptName binding only, unverifiable — the
-- wounded-skirmisher precedent), so it cannot be registered and
-- the caster object cannot be resolved from Lua (no
-- creature-by-GUID bridge; FindNearestCreature needs the
-- unknown entry). The caster's own arms (Reset summon +
-- 1-3s/12s summon-spell poll + SetData re-summon) stay out
-- with it.

local ENTRY_INFERNAL_ATTACKER = 21419

local MODEL_INVISIBLE = 20577
local MODEL_INFERNAL = 17312
local SPELL_SUMMON_INFERNAL = 37277

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_PACIFIED = 0x20000
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000
local UNIT_FLAG_IMMUNE_TO_PC = 0x100

local function infernalAttackerSpawnOrReset(_, creature)
    creature:SetDisplayId(MODEL_INVISIBLE)
    creature:MoveRandom(5.0)
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
