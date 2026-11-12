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

local SPELL_SHOOT_RJR = 51330 -- zone_sholazar_basin.cpp:460 (class spell_q12589_shoot_rjr)
local NPC_LUCKY_WILHELM = 28054 -- zone_sholazar_basin.cpp:440
local SPELL_FAILED_CUSTOM_ERROR = 172 -- SharedDefines.h:1154
local SPELL_CUSTOM_ERROR_MUST_TARGET_WILHELM = 27 -- SharedDefines.h:1203
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_q12589_shoot_rjr::CheckCast (zone_sholazar_basin.cpp:470-478):
-- the explicit target must be Lucky Wilhelm (28054) or the cast fails
-- SPELL_FAILED_CUSTOM_ERROR with MUST_TARGET_WILHELM. RegisterSpellEvent
-- OnCheckCast = 8 (lua_spell.go fireSpellCheckCast); the second return
-- is the custom-error id the fire site sends with the failure. Binding
-- 51330 is the AC spell_script_names row. The HandleDummy roll arm
-- (EVENT_MISS/HIT/MISS_BIRD) stays out.
local function shootRjrCheckCast(event, spell)
    local target = spell:GetTarget()
    if target ~= nil and target:GetEntry() == NPC_LUCKY_WILHELM then
        return SPELL_CAST_OK
    end
    return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_MUST_TARGET_WILHELM
end

RegisterSpellEvent(SPELL_SHOOT_RJR, 8, shootRjrCheckCast)
