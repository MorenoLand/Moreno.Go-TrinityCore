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
--
-- PORTED (tick 200, RegisterAuraEvent OnChangeAmount = 4):
--   spell_sha_maelstrom_weapon_AuraScript::HandleBonus
--   (spell_shaman.cpp:1570-1580): EFFECT_0 SPELL_AURA_ADD_PCT_MODIFIER,
--   AURA_EFFECT_HANDLE_CHANGE_AMOUNT — fires on stack changes, not on
--   apply, so it rides the tick-200 OnChangeAmount event, not OnApply.
--   At full stacks (5) the shaman rolls the T10 Enhancement 4P bonus
--   aura (70832, EFFECT_0) amount; on success it self-casts Maelstrom
--   Power (70831). The C++ cast passes the T10 aura effect as the
--   triggered-by-aura; Go has no such parameter, so the cast is plain.
--   GetSpellInfo()->StackAmount is 5 for every rank; GetSpellStackAmount
--   reads the same value from Spell.dbc and falls back to 5 when the
--   DBC row is unknown.

local SPELL_SHAMAN_EARTHBIND_TOTEM = 6474 -- spell_shaman.cpp:460 (class spell_sha_earthbind_totem)
local SPELL_SHAMAN_STORM_EARTH_AND_FIRE = 51483 -- spell_shaman.cpp:62
local SPELL_SHAMAN_TOTEM_EARTHBIND_EARTHGRAB = 64695 -- spell_shaman.cpp:63
local SPELL_SHAMAN_TOTEM_EARTHEN_POWER = 59566 -- spell_shaman.cpp:65
local SPELL_SHAMAN_MAELSTROM_WEAPON = 53817 -- spell_shaman.cpp:1550 (class spell_sha_maelstrom_weapon)
local SPELL_SHAMAN_MAELSTROM_POWER = 70831 -- spell_shaman.cpp:92
local SPELL_SHAMAN_T10_ENHANCEMENT_4P_BONUS = 70832 -- spell_shaman.cpp:93
local MAELSTROM_WEAPON_MAX_STACKS = 5 -- spell_shaman.cpp:1572 (GetSpellInfo()->StackAmount)
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

local function maelstromWeaponChangeAmount(event, aura, target)
    local maxStacks = aura:GetSpellStackAmount(SPELL_SHAMAN_MAELSTROM_WEAPON)
    if maxStacks == 0 then
        maxStacks = MAELSTROM_WEAPON_MAX_STACKS
    end
    if aura:GetStackAmount() < maxStacks then
        return
    end
    -- C++ GetUnitOwner() is the aura owner = the hook target (self-buff).
    local amount = target:GetAuraEffect(SPELL_SHAMAN_T10_ENHANCEMENT_4P_BONUS, 0)
    if amount == nil then
        return
    end
    if math.random(0, 99) < amount then
        target:CastSpell(nil, SPELL_SHAMAN_MAELSTROM_POWER)
    end
end

RegisterAuraEvent(SPELL_SHAMAN_EARTHBIND_TOTEM, 2, earthbindApply)
RegisterAuraEvent(SPELL_SHAMAN_EARTHBIND_TOTEM, 1, earthbindPeriodic)
RegisterAuraEvent(SPELL_SHAMAN_MAELSTROM_WEAPON, 4, maelstromWeaponChangeAmount)
