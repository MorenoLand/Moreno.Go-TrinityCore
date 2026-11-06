-- Black Temple spell scripts — Lua port of
-- src/server/scripts/Outland/BlackTemple/black_temple.cpp
-- (spell_illidari_nightlord_shadow_inferno only; the file's
-- creature AIs live in their boss_*.lua files, and
-- spell_soul_fragment_anger stays out — its AfterCast caster-
-- despawn leg has no spell-cast hook on the Lua surface).

-- C++ spell_illidari_nightlord_shadow_inferno::OnPeriodic
-- (black_temple.cpp:254-274): 39645 EFFECT_0 SPELL_AURA_
-- PERIODIC_TRIGGER_SPELL. PreventDefaultAction; bp =
-- GetTickNumber() * GetAmount(); the aura owner casts
-- SPELL_SHADOW_INFERNO_DAMAGE (39646, black_temple.cpp:37)
-- triggered on itself with BP0 = bp. Returns true, the
-- PreventDefaultAction twin, so the Go default trigger cast
-- does not also fire. GetTickNumber reads 1 on the first tick
-- and GetAmount defaults to effect 0 (lua_aura.go), both
-- C++-exact.
local SPELL_SHADOW_INFERNO = 39645
local SPELL_SHADOW_INFERNO_DAMAGE = 39646

local function shadowInfernoTick(event, aura, target)
    local bp = aura:GetTickNumber() * aura:GetAmount()
    target:CastCustomSpell(target, SPELL_SHADOW_INFERNO_DAMAGE, true, bp)
    return true
end

RegisterAuraEvent(SPELL_SHADOW_INFERNO, 1, shadowInfernoTick)
