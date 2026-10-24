-- Priest Shadowfiend — Lua port of
-- src/server/scripts/Pet/pet_priest.cpp
-- (class npc_pet_pri_shadowfiend : CreatureScript("npc_pet_pri_shadowfiend")).
-- Entry: 19668 (C++-sourced, never guessed: Pet.cpp:1018
-- "case 19668: // Shadowfiend" in InitStatsForLevel, corroborated by
-- botcommon.h:911 "ORIGINAL_ENTRY_SHADOWFIEND = 19668"; no NPC_
-- constant exists in pet_priest.cpp itself).
-- Eluna creature events: 22 OnSummoned.
-- Routing (proven in-tree this tick, head's "do not register pet
-- entries until routing is proven" now resolved for this class):
-- a priest-cast Shadowfiend takes the player effect-28 path
-- (spells.go effect-28 arm routes non-totem/non-lightwell titles
-- through handleSummonPet) -> spawnPet -> registerPetMotion
-- (pets.go:1204, non-critter branch), which fires
-- fireCreaturePetSummoned (ev22, args (event, creature, summoner))
-- and fireCreatureSpawned (ev23+ev5); dispatch is entry-keyed
-- (TriggerCreatureEvent by motion.Entry), independent of the
-- BossAI slot. Deliberately NOT added to boss_ai.go: pet motions
-- never consult BossAI for event dispatch, and installing a
-- luaBossAI via the bossAIByEntry map could alter nil-checks on
-- pet_combat paths that assign BossAI by entry.
-- Ported arms:
-- - IsSummonedBy (pet_priest.cpp:73-82, event 22): summoner
--   non-nil and carrying SPELL_PRIEST_GLYPH_OF_SHADOWFIEND
--   (58228) -> DoCastAOE(SPELL_PRIEST_SHADOWFIEND_DEATH 57989),
--   lowered to a non-triggered self-cast (selin_fireheart /
--   mr_smite DoCastAOE convention); player:HasAura is bridged
--   (lua_player.go) and the ev22 summoner arg is the owner
--   player object (fireCreaturePetSummoned passes s.luaPlayer()).
-- Unmodeled (documented-only, no bridges):
-- - The shadowfiend's PetAI combat behavior (melee swings,
--   Shadowcrawl, mana return on hit): engine-driven pet combat,
--   not script arms. LeavingWorld has no counterpart here:
--   PetAI despawn is engine-side, and the 50055-style owner-aura
--   strip on despawn belongs to the soul-trader/lich LeavingWorld
--   arms, not this class (this class has no LeavingWorld arm).

local ENTRY_SHADOWFIEND = 19668

local SPELL_PRIEST_GLYPH_OF_SHADOWFIEND = 58228
local SPELL_PRIEST_SHADOWFIEND_DEATH = 57989

-- C++ IsSummonedBy: glyph-aura summoner -> DoCastAOE(57989).
local function shadowfiendSummoned(event, creature, summoner)
    if not summoner or not summoner:HasAura(SPELL_PRIEST_GLYPH_OF_SHADOWFIEND) then
        return
    end
    creature:CastSpell(creature, SPELL_PRIEST_SHADOWFIEND_DEATH)
end

RegisterCreatureEvent(ENTRY_SHADOWFIEND, 22, shadowfiendSummoned)
