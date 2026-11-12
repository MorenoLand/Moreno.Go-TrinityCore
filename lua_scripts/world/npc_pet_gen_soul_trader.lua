-- Ethereal Soul Trader — Lua port of src/server/scripts/Pet/pet_generic.cpp
-- (struct npc_pet_gen_soul_trader : ScriptedAI, registered by
-- AddSC_generic_pet_scripts). Entry: 27914 (spell_generic.cpp:1647,
-- NPC_ETHEREAL_SOUL_TRADER; the class itself carries no entry constant —
-- the entry comes from the creature_template ScriptName binding, as with
-- the lich pet 36979).
-- Eluna creature events: 5 OnSpawn (JustAppeared twin).
-- Routing: event registration only, deliberately NOT added to
-- boss_ai.go — pet motions never consult BossAI for event dispatch
-- (pet-routing proof: dispatch is entry-keyed via
-- TriggerCreatureEvent by motion.Entry).
-- Ported arms:
-- - JustAppeared (pet_generic.cpp:143-150, event 5): the soul trader
--   says SAY_SOUL_TRADER_INTRO (0) and casts Ethereal Pet OnSummon
--   50052 on its owner, non-triggered (DoCast default args); the owner
--   then self-applies Ethereal Pet Aura 50051 via the spell script
--   (spell_generic.lua). ev5 OnSpawn is the JustAppeared twin
--   (fireCreatureSpawned, Creature.cpp:699-704; the summon funnel routes
--   through motionFor, so a summoned soul trader fires it). The base
--   CreatureAI::JustAppeared call carries no observable arm.
-- Unmodeled (documented-only, no fire site):
-- - LeavingWorld (pet_generic.cpp:138-141): the pet casts Ethereal Pet
--   Aura Remove 50055 on its owner when leaving the world. No Lua
--   fire site exists for it — expireTempSummonGuardian (the UnSummon
--   twin) and every unsummonPet path delete the motion without any
--   Lua fire (ebon gargoyle precedent), and ev37 OnRemove only fires
--   on TTL prune / server delete / GM commands, not on summon expiry.
--   The 50055 spell script itself is ported (spell_generic.lua); only
--   its pet-driven trigger is missing. Stays out.
-- DORMANT until Spell.dbc ships the 50052 row (and the 27914 summon).

local NPC_ETHEREAL_SOUL_TRADER = 27914

local SAY_SOUL_TRADER_INTRO = 0 -- pet_generic.cpp:127
local SPELL_ETHEREAL_ONSUMMON = 50052 -- pet_generic.cpp:129

-- C++ JustAppeared: say the intro line, cast Ethereal Pet OnSummon on the owner.
local function soulTraderSpawn(event, creature)
    creature:Talk(SAY_SOUL_TRADER_INTRO)
    local owner = creature:GetOwner()
    if owner == nil then
        return
    end
    creature:CastSpell(owner, SPELL_ETHEREAL_ONSUMMON, false)
end

RegisterCreatureEvent(NPC_ETHEREAL_SOUL_TRADER, 5, soulTraderSpawn)
