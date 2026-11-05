-- Shaman spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_shaman.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2, OnPeriodic = 1):
--   spell_sha_earthbind_totem_AuraScript::Apply
--   (spell_shaman.cpp:499-507): EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
--   On apply, the totem's owner player (GetCharmerOrOwnerPlayerOrPlayerItself;
--   the totem's GetOwner twin, with the player-self leg) rolls the
--   Storm, Earth and Fire talent aura (51483, EFFECT_1, any rank)
--   amount; on success the totem self-casts Earthgrab 64695,
--   non-triggered.
--   spell_sha_earthbind_totem_AuraScript::HandleEffectPeriodic
--   (spell_shaman.cpp:476-492): same effect. Each tick the owner's
--   Storm, Earth and Fire dummy aura (SPELLFAMILY_SHAMAN 11, icon
--   2289, EFFECT_0) rolls its base amount; on success the aura
--   target self-casts Earthen Power 59566, triggered. No
--   PreventDefaultAction in C++, so the handler returns nothing and
--   the Go default trigger cast still fires.
--   The npcbot creator leg (spell_shaman.cpp:481-486) stays out —
--   no npcbot model in Go.

local SPELL_SHAMAN_EARTHBIND_TOTEM = 6474 -- spell_shaman.cpp:460 (class spell_sha_earthbind_totem)
local SPELL_SHAMAN_STORM_EARTH_AND_FIRE = 51483 -- spell_shaman.cpp:62
local SPELL_SHAMAN_TOTEM_EARTHBIND_EARTHGRAB = 64695 -- spell_shaman.cpp:63
local SPELL_SHAMAN_TOTEM_EARTHEN_POWER = 59566 -- spell_shaman.cpp:65
local SPELLFAMILY_SHAMAN = 11 -- spells.go:36322
local SHAMAN_ICON_ID_STORM_EARTH_AND_FIRE = 2289 -- spell_shaman.cpp:484 (dummy icon)

local function getOwnerPlayer(unit)
    if unit == nil then
        return nil
    end
    local owner = unit:GetOwner()
    if owner ~= nil then
        return owner
    end
    if unit:GetObjectType() == "Player" then
        return unit
    end
    return nil
end

local function earthbindApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local owner = getOwnerPlayer(caster)
    if owner == nil then
        return
    end
    local amount = owner:GetAuraEffectOfRankedSpell(SPELL_SHAMAN_STORM_EARTH_AND_FIRE, 1)
    if amount == nil then
        return
    end
    if math.random(0, 99) < amount then
        caster:CastSpell(caster, SPELL_SHAMAN_TOTEM_EARTHBIND_EARTHGRAB, false)
    end
end

local function earthbindPeriodic(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local owner = getOwnerPlayer(caster)
    if owner == nil then
        return
    end
    local amount = owner:GetDummyAuraEffect(SPELLFAMILY_SHAMAN, SHAMAN_ICON_ID_STORM_EARTH_AND_FIRE, 0)
    if amount == nil then
        return
    end
    if math.random(0, 99) < amount then
        target:CastSpell(target, SPELL_SHAMAN_TOTEM_EARTHEN_POWER, true)
    end
end

RegisterAuraEvent(SPELL_SHAMAN_EARTHBIND_TOTEM, 2, earthbindApply)
RegisterAuraEvent(SPELL_SHAMAN_EARTHBIND_TOTEM, 1, earthbindPeriodic)
