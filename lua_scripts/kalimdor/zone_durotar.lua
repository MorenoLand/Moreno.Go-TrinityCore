-- Durotar zone spells -- Lua port of
-- src/server/scripts/Kalimdor/zone_durotar.cpp (spell scripts).
-- spell_voodoo (17009) PORTED below. spell_voljin_war_drums STAYS OUT:
-- the trigger spell ID is DB-bound via spell_script_names (the loader
-- name only in C++, zone_durotar.cpp:417-455); the motivate arms
-- (75088/75086 on citizens 40256/40257, non-triggered caster cast)
-- are recorded here, not guessed — revisit when a TDB dump is
-- available (row-290 Shadowflame precedent).
-- All ports DORMANT until Spell.dbc ships the rows.

local SPELL_VOODOO = 17009

local SPELL_BREW = 16712 -- Special Brew
local SPELL_GHOSTLY = 16713 -- Ghostly
local SPELL_GROW = 16711 -- Grow
local SPELL_LAUNCH = 16716 -- Launch (Whee!)

local VOODOO_HEXES = { 16707, 16708, 16709 } -- Hex

-- C++ spell_voodoo::HandleDummy (zone_durotar.cpp:484-491, registered
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_DUMMY): RAND pick of brew /
-- ghostly / one of three hexes / grow / launch, caster casts the pick
-- non-triggered on the hit unit. RAND(a, b, RAND(c, d, e), f, g) is a
-- uniform 1/5 outer pick with a 1/3 hex pick inside.
local function voodooHit(_, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local pick = math.random(1, 5)
    local spellId
    if pick == 1 then
        spellId = SPELL_BREW
    elseif pick == 2 then
        spellId = SPELL_GHOSTLY
    elseif pick == 3 then
        spellId = VOODOO_HEXES[math.random(1, 3)]
    elseif pick == 4 then
        spellId = SPELL_GROW
    else
        spellId = SPELL_LAUNCH
    end
    caster:CastSpell(target, spellId)
end

RegisterSpellEvent(SPELL_VOODOO, 5, voodooHit)
