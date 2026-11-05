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
