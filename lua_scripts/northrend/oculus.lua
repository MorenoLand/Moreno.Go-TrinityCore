-- The Oculus (Nexus) — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/oculus.cpp
-- (spell_oculus_ride_ruby_emerald_amber_drake_que only; the rest of
-- the file's creature/spell scripts are not ported here).

-- C++ spell_oculus_ride_ruby_emerald_amber_drake_que::HandlePeriodic
-- (oculus.cpp:449-466): PreventDefaultAction; the aura target casts
-- the DBC trigger of EFFECT_0 (aura:GetTriggerSpell(0)) triggered on
-- the aura's caster — the C++ comment notes the default trigger's
-- caster is wrong, so the handler re-casts it explicitly. One
-- handler for all three drake-que auras (49427 emerald, 49459 amber,
-- 49463 ruby). Returns true, the PreventDefaultAction twin, so the
-- Go default trigger cast does not also fire.
local SPELL_RIDE_EMERALD_DRAKE_QUE = 49427
local SPELL_RIDE_AMBER_DRAKE_QUE = 49459
local SPELL_RIDE_RUBY_DRAKE_QUE = 49463

local function drakeQueTick(event, aura, target)
    local trigger = aura:GetTriggerSpell(0)
    if trigger ~= 0 then
        local caster = aura:GetCaster()
        if caster then
            target:CastSpell(caster, trigger, true)
        end
    end
    return true
end

RegisterAuraEvent(SPELL_RIDE_EMERALD_DRAKE_QUE, 1, drakeQueTick)
RegisterAuraEvent(SPELL_RIDE_AMBER_DRAKE_QUE, 1, drakeQueTick)
RegisterAuraEvent(SPELL_RIDE_RUBY_DRAKE_QUE, 1, drakeQueTick)
