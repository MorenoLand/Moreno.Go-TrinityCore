-- Lich Pet — Lua port of src/server/scripts/Pet/pet_generic.cpp
-- (struct npc_pet_lich : ScriptedAI, registered by
-- AddSC_generic_pet_scripts). Entry: 36979 (spell_generic.cpp:1887,
-- NPC_LICH_PET; the class itself carries no entry constant — the entry
-- comes from the creature_template ScriptName binding, as with the
-- ebon gargoyle 27829).
-- Eluna creature events: 5 OnSpawn (JustAppeared twin).
-- Routing: event registration only, deliberately NOT added to
-- boss_ai.go — pet motions never consult BossAI for event dispatch
-- (pet-routing proof: dispatch is entry-keyed via
-- TriggerCreatureEvent by motion.Entry).
-- Ported arms:
-- - JustAppeared (pet_generic.cpp:169-174, event 5): the lich pet
--   casts Lich Pet OnSummon 69735 on its owner, non-triggered
--   (DoCast default args); the owner then self-applies Lich Pet Aura
--   69732 via the spell script (spell_generic.lua). ev5 OnSpawn is
--   the JustAppeared twin (fireCreatureSpawned, Creature.cpp:699-704;
--   the summon funnel routes through motionFor, so a summoned lich
--   pet fires it). The base CreatureAI::JustAppeared call carries no
--   observable arm.
-- Unmodeled (documented-only, no fire site):
-- - LeavingWorld (pet_generic.cpp:162-167): the pet casts Lich Pet
--   Aura Remove 69736 on its owner when leaving the world. No Lua
--   fire site exists for it — expireTempSummonGuardian (the UnSummon
--   twin) and every unsummonPet path delete the motion without any
--   Lua fire (ebon gargoyle precedent), and ev37 OnRemove only fires
--   on TTL prune / server delete / GM commands, not on summon expiry.
--   The 69736 spell script itself is ported (spell_generic.lua); only
--   its pet-driven trigger is missing. Stays out.
-- DORMANT until Spell.dbc ships the 69735 row (and the 36979 summon).

local NPC_LICH_PET = 36979

local SPELL_LICH_ONSUMMON = 69735 -- pet_generic.cpp:155

-- C++ JustAppeared: cast Lich Pet OnSummon on the owner.
local function lichPetSpawn(event, creature)
    local owner = creature:GetOwner()
    if owner == nil then
        return
    end
    creature:CastSpell(owner, SPELL_LICH_ONSUMMON, false)
end

RegisterCreatureEvent(NPC_LICH_PET, 5, lichPetSpawn)
