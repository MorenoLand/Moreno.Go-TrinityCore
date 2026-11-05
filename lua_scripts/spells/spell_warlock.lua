-- Warlock spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_warlock.cpp.
--
-- PORTED (tick 195, RegisterAuraEvent):
--   spell_warl_drain_soul HandleTick (spell_warlock.cpp:546-562):
--   EFFECT_1 PERIODIC_DAMAGE. When the caster is a player and the
--   target yields experience or honor (isHonorOrXPTarget), each tick
--   has a 20% chance to create a Soul Shard: the caster casts
--   CREATE_SOULSHARD 43836 on itself, triggered by the aura effect.
--   Glyph of Drain Soul leg (:557-559): while the caster has the
--   glyph aura 58070, a second roll against that aura effect's
--   MiscValue (DBC, via Aura:GetSpellEffectMiscValue) casts the
--   glyph proc 58068 on the caster, triggered. No PreventDefaultAction
--   in C++, so the Go default damage tick still fires (no return).
--   Registered for rank 1 (1120), the only Drain Soul rank ID in the
--   C++ tree; ranks 2-6 are DB-bound via spell_script_names and
--   unestablished (C++ tree + web search exhausted this tick).
-- DOCUMENTED-ONLY (no behavior fabricated):
--   The same loader's CheckProc / HandleProc arms (:509-544,
--   EFFECT_2 improved Drain Soul mana proc) are proc arms, not
--   periodic ticks: no proc hook exists on the RegisterAuraEvent
--   surface, so they stay out with the aura-script registration.

local SPELL_WARLOCK_DRAIN_SOUL_R1 = 1120
local SPELL_WARLOCK_CREATE_SOULSHARD = 43836
local SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA = 58070
local SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_PROC = 58068

local function drainSoulTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if not caster:IsHonorOrXPTarget(target) then
        return
    end
    if math.random(0, 99) < 20 then
        caster:CastSpell(caster, SPELL_WARLOCK_CREATE_SOULSHARD, true)
        if caster:HasAura(SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA) then
            local chance = aura:GetSpellEffectMiscValue(SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA, 0)
            if math.random(0, 99) < chance then
                caster:CastSpell(caster, SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_PROC, true)
            end
        end
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DRAIN_SOUL_R1, 1, drainSoulTick)
