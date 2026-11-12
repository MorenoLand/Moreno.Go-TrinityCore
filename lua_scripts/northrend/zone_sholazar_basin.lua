-- Sholazar Basin zone spell scripts -- Lua ports of the SpellScript
-- arms in src/server/scripts/Northrend/zone_sholazar_basin.cpp.
--
-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12611_deathbolt::HandleScriptEffect
--   (zone_sholazar_basin.cpp:658-689): EFFECT_0 SCRIPT_EFFECT. The
--   hit unit carrying Freya's Ward (51845) casts Sevenfold
--   Retribution (51856) on the caster, triggered; otherwise the
--   caster casts Deathbolt (51855) on the hit unit, triggered.
--   Binding 51854 (AC 'spell_q12611_deathbolt'; TDB 335.26091
--   renames it 'spell_sholazar_deathbolt').
local SPELL_DEATHBOLT = 51854 -- zone_sholazar_basin.cpp:658
local SPELL_FREYAS_WARD = 51845 -- zone_sholazar_basin.cpp:654
local SPELL_SEVENFOLD_RETRIBUTION = 51856 -- zone_sholazar_basin.cpp:656
local SPELL_DEATHBOLT_TRIGGER = 51855 -- zone_sholazar_basin.cpp:657

local function deathboltHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if hitTarget:HasAura(SPELL_FREYAS_WARD) then
        hitTarget:CastSpell(caster, SPELL_SEVENFOLD_RETRIBUTION, true)
    else
        caster:CastSpell(hitTarget, SPELL_DEATHBOLT_TRIGGER, true)
    end
end

RegisterSpellEvent(SPELL_DEATHBOLT, 5, deathboltHit)
