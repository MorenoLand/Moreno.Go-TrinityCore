-- Krik'thir the Gatewatcher (Azjol-Nerub) — Lua port of the AuraScript
-- arm in src/server/scripts/Northrend/AzjolNerub/AzjolNerub/boss_krikthir_the_gatewatcher.cpp.
--
-- PORTED (tick 198, RegisterAuraEvent OnRemove = 3):
--   spell_gatewatcher_web_wrap::HandleEffectRemove
--   (boss_krikthir_the_gatewatcher.cpp:1015-1023): EFFECT_0
--   SPELL_AURA_MOD_ROOT. On removal by expiry only, the target casts
--   Web Wrap Wrapped 52087 on itself, triggered. The mode gate is
--   Aura:GetRemoveMode (new tick 198: SpellAuraDefines.h:61-69 values).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the wrapped player's
-- triggered self-cast lands non-triggered; pre-existing bridge gap,
-- same as all prior player-target ports.

local SPELL_WEB_WRAP = 52086 -- boss_krikthir_the_gatewatcher.cpp:74
local SPELL_WEB_WRAP_WRAPPED = 52087 -- boss_krikthir_the_gatewatcher.cpp:75
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

local function webWrapRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    if target ~= nil then
        target:CastSpell(target, SPELL_WEB_WRAP_WRAPPED, true)
    end
end

RegisterAuraEvent(SPELL_WEB_WRAP, 3, webWrapRemove)
