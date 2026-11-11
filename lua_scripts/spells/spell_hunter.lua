-- Hunter spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_hunter.cpp.
--
-- PORTED (tick 198, RegisterAuraEvent OnApply = 2):
--   spell_hun_aspect_of_the_beast_pet::OnApply
--   (spell_hunter.cpp:143-151): EFFECT_0 SPELL_AURA_UNTRACKABLE.
--   On apply, the pet's spell-mod owner must have Aspect of the
--   Beast 13161, or the aura removes itself. The C++ Load() gate
--   (GetSpellModOwner() non-null) is the owner-nil return below:
--   with no owner the script would not load, so nothing is removed.
--   The Go twin for GetSpellModOwner is target:GetOwner() — a pet's
--   spell mods come from its owner, and the owner object carries
--   HasAura.

local SPELL_ASPECT_OF_THE_BEAST_PET = 61669 -- spell_hunter.cpp:130
local SPELL_HUNTER_ASPECT_OF_THE_BEAST = 13161 -- spell_hunter.cpp:35

local function aspectOfTheBeastPetApply(event, aura, target)
    if target:GetObjectType() ~= "Creature" then
        return
    end
    local owner = target:GetOwner()
    if owner == nil then
        return
    end
    if not owner:HasAura(SPELL_HUNTER_ASPECT_OF_THE_BEAST) then
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_ASPECT_OF_THE_BEAST_PET, 2, aspectOfTheBeastPetApply)

-- PORTED (this tick, RegisterAuraEvent OnApply = 2):
--   spell_hun_aspect_of_the_beast::OnApply
--   (spell_hunter.cpp:111-116): EFFECT_0 SPELL_AURA_UNTRACKABLE,
--   AfterEffectApply REAL. With a pet out the player casts the pet
--   aura 61669 on self, triggered; the cast's TARGET_UNIT_PET
--   implicit target lands it on the pet (spells.go:8942), where the
--   port above checks the owner still has 13161. The C++ Load()
--   player gate holds by construction (the aura is player-only).
--   The triggered flag rides the same pre-existing gap as the other
--   ports (player:CastSpell ignores it, lua_player.go:2577).
-- NOT PORTED: OnRemove (spell_hunter.cpp:103-109) — the pet must
--   drop 61669, but there is no player-side bridge to resolve the
--   pet GUID into a creature object (GetCreatureByGUID lives only
--   on creature objects, lua_creature_events.go:4050; no global).
--   The pet keeps 61669 until it is re-applied and self-removes.
local function aspectOfTheBeastApply(event, aura, target)
    if target:GetPetGUID() == 0 then
        return
    end
    target:CastSpell(target, SPELL_ASPECT_OF_THE_BEAST_PET, true)
end

RegisterAuraEvent(SPELL_HUNTER_ASPECT_OF_THE_BEAST, 2, aspectOfTheBeastApply)
