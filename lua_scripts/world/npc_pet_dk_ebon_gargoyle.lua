-- Death Knight Ebon Gargoyle — Lua port of
-- src/server/scripts/Pet/pet_dk.cpp
-- (class npc_pet_dk_ebon_gargoyle : CreatureScript("npc_pet_dk_ebon_gargoyle")).
-- Entry: 27829 (C++-sourced, never guessed: pet_dk triage, corroborated
-- by the 13:07 EDT pet-routing run; the class itself carries no entry
-- constant — the entry comes from the creature_template ScriptName
-- binding, as with shadowfiend 19668).
-- Eluna creature events: 4 OnDied.
-- Routing: event registration only, deliberately NOT added to
-- boss_ai.go — pet motions never consult BossAI for event dispatch
-- (13:07 EDT pet-routing proof: dispatch is entry-keyed via
-- TriggerCreatureEvent by motion.Entry). The ev4 fire site is
-- onCreatureKilled -> fireCreatureDied (kill.go, entry-keyed, no
-- BossAI gate); it runs when the gargoyle motion is the kill target
-- of a funneled kill path (player melee/spell kills resolve targets
-- through the GUID-keyed motion lookup, which is type-agnostic).
-- Ported arms:
-- - JustDied (pet_dk.cpp:70-76, event 4): owner->RemoveAurasDueToSpell
--   (SPELL_DK_SUMMON_GARGOYLE_2 50514), lowered to owner:RemoveAura
--   via the new creature:GetOwner() bridge (lua_creature_events.go).
--   The ev4 killer arg is the killer session's player, NOT the owner,
--   so the handler must use GetOwner, never the third arg.
-- Unmodeled (documented-only, no bridges / no fire site):
-- - InitializeAI (pet_dk.cpp:49-68): AnyUnfriendlyUnitInObjectRangeCheck
--   grid scan for the 49206-aura victim — no grid-search bridge, and
--   the 30 yd aura-holder search has no Go twin. Stays out.
-- - SpellHit dismiss (pet_dk.cpp:79-110, spell 50515 from the owner):
--   no fire site in Go — Dismiss Gargoyle is never cast as a spell
--   (zero 50515 hits in the Go engine); the Go dismiss path is
--   EffectDismissPet -> handleDismissPet -> unsummonPet (pets.go),
--   which fires no Lua events at all. The fly-away legs
--   (ApplyModFlag NON_ATTACKABLE, SetCanFly, SetSpeedRate,
--   MovePoint, DespawnOrUnsummon) are additionally unbridged.
--   Verified this tick: the spells.go ev14 loop filters hit targets
--   to 0xF130 creature GUIDs while pet motions carry 0xF140 GUIDs
--   (makePetGUID), so no player-cast spell ever fires ev14 on a pet
--   motion through that path either.
-- - Gargoyle endings that fire no ev4 in Go (verified this tick):
--   temp-summon expiry (expireTempSummonGuardian) and every
--   unsummonPet path (dismiss, owner death/logout) delete the motion
--   without any Lua fire; only a kill-funnel death fires ev4.

local ENTRY_EBON_GARGOYLE = 27829

local SPELL_DK_SUMMON_GARGOYLE_2 = 50514

-- C++ JustDied: stop Feeding Gargoyle when it dies.
local function gargoyleDied(event, creature, killer)
    local owner = creature:GetOwner()
    if owner ~= nil then
        owner:RemoveAura(SPELL_DK_SUMMON_GARGOYLE_2)
    end
end

RegisterCreatureEvent(ENTRY_EBON_GARGOYLE, 4, gargoyleDied)
