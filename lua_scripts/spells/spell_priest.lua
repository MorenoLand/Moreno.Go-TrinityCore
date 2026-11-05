-- Priest spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_priest.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2):
--   spell_pri_renew_AuraScript::HandleApplyEffect
--   (spell_priest.cpp:1238-1254): EFFECT_0 SPELL_AURA_PERIODIC_HEAL.
--   On apply, when the caster carries the Empowered Renew talent
--   aura (SPELLFAMILY_PRIEST 6, icon 3021, EFFECT_1), the caster
--   casts Empowered Renew 63544 on the target with BP0 =
--   CalculatePct(renew amount * total ticks, talent amount). The
--   C++ Load() gate (caster is a player, npcbot aside) is the
--   Player type check below. CalculatePct is T(base * pct / 100.0f)
--   (Util.h:66-69), the float-truncate math.floor twin below.
--   DELTA: C++ registers REAL_OR_REAPPLY, so a Renew refresh also
--   fires the arm; the Go apply hook is fresh-only (the tick-198
--   fungal-decay note), so a reapply casts nothing.

local SPELL_PRIEST_RENEW = 139 -- spell_priest.cpp:1220 (class spell_pri_renew)
local SPELL_PRIEST_EMPOWERED_RENEW = 63544 -- spell_priest.cpp:41
local SPELLFAMILY_PRIEST = 6 -- spells.go:36319
local PRIEST_ICON_ID_EMPOWERED_RENEW_TALENT = 3021 -- spell_priest.cpp:83

local function renewApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local empoweredAmount = caster:GetDummyAuraEffect(SPELLFAMILY_PRIEST, PRIEST_ICON_ID_EMPOWERED_RENEW_TALENT, 1)
    if empoweredAmount == nil then
        return
    end
    local heal = aura:GetAmount(0) * aura:GetTotalTicks()
    local bp0 = math.floor(heal * empoweredAmount / 100)
    caster:CastCustomSpell(target, SPELL_PRIEST_EMPOWERED_RENEW, false, bp0)
end

RegisterAuraEvent(SPELL_PRIEST_RENEW, 2, renewApply)
