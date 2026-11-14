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
--   grid scan for the 49206-aura victim — the grid-search bridge EXISTS
--   now (round-73 correction: creature:GetUnfriendlyUnitsInRange(30),
--   lua_creature_events.go:6832-6836 via luaUnitFriendlyMethods). The
--   leg stays out for lack of a summon-spawn hook, not the grid search:
--   ev5 fires respawn-only (lua_creature_events.go:7948-7962 comment)
--   and ev23 never fires on summon (fire sites: evade :4733, respawn
--   :3475, death pre-fire :7843/:7892, leave-combat :8149) — a summoned
--   guardian gets neither. Second gap if a hook lands: HasAura :1483
--   takes spellId only, no caster arg — C++ gates on the OWNER's aura
--   (ownerGuid), so a Lua port would cross-pick in multi-DK cases
--   (documented deviation, single-DK case exact).
-- - SpellHit dismiss (pet_dk.cpp:79-110, spell 50515 from the owner):
--   no fire site in Go — Dismiss Gargoyle is never cast as a spell
--   (zero 50515 hits in the Go engine); the Go dismiss path is
--   EffectDismissPet -> handleDismissPet -> unsummonPet (pets.go),
--   which fires no Lua events at all. The fly-away legs are mostly
--   bridged now (round-73 re-verified: CastSpell :954, SetReactState
--   :4715, SetCanFly :3988, SetFlag :6452 UNIT_FIELD_FLAGS=59,
--   MoveTo :5410, Despawn :3318; only SetSpeedRate MOVE_FLIGHT is
--   unbridged — :6065-6071 rejects mt != 1). Moot while the trigger
--   is unreachable: ev14 itself accepts pet GUIDs now (spells.go
--   :19909-10/:30191-94 take 0xF130/0xF140/0xF150; the old 0xF130-only
--   filter claim is retracted) — the stay-out ground is the no-cast
--   proof, not the GUID filter.
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
