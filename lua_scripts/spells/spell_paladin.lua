-- Paladin spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_paladin.cpp.
--
-- PORTED (tick 201, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_pal_blessing_of_sanctuary (20911 Blessing of Sanctuary and
--   25899 Greater Blessing of Sanctuary, spell_paladin.cpp:476-529):
--   EFFECT_0 DUMMY, REAL_OR_REAPPLY. OnApply the caster casts the
--   Sanctuary buff (67480) on the target, triggered; on remove the
--   target drops that buff. C++ registers AfterEffectApply /
--   AfterEffectRemove; the Go apply/remove hooks are the single
--   apply-path hooks and the handler does not depend on the effect
--   being applied first, so the ordering delta is immaterial.
--   DELTA: C++ removes the buff by (spell, caster GUID); Go
--   target:RemoveAura(id) takes no caster filter. The buff is only
--   ever cast by the blessing's caster, so the unfiltered removal is
--   the same removal in practice.
-- STILL BLOCKED (same class):
--   CheckProc / HandleProc (:511-519): on a mana-power target the
--   proc casts Sanctuary Energize (57319) — no proc hook exists on
--   the RegisterAuraEvent surface, so the proc leg stays out.

local SPELL_PALADIN_BLESSING_OF_SANCTUARY = 20911 -- spell_paladin.cpp:476
local SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY = 25899 -- spell_paladin.cpp:477
local SPELL_PALADIN_BLESSING_OF_SANCTUARY_BUFF = 67480 -- spell_paladin.cpp:38

-- C++ spell_pal_blessing_of_sanctuary_AuraScript::HandleEffectApply
-- (spell_paladin.cpp:500-505), shared by both blessing ranks.
local function blessingOfSanctuaryApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster ~= nil then
        caster:CastSpell(target, SPELL_PALADIN_BLESSING_OF_SANCTUARY_BUFF, true)
    end
end

RegisterAuraEvent(SPELL_PALADIN_BLESSING_OF_SANCTUARY, 2, blessingOfSanctuaryApply)
RegisterAuraEvent(SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY, 2, blessingOfSanctuaryApply)

-- C++ spell_pal_blessing_of_sanctuary_AuraScript::HandleEffectRemove
-- (spell_paladin.cpp:507-511), shared by both blessing ranks.
local function blessingOfSanctuaryRemove(event, aura, target)
    target:RemoveAura(SPELL_PALADIN_BLESSING_OF_SANCTUARY_BUFF)
end

RegisterAuraEvent(SPELL_PALADIN_BLESSING_OF_SANCTUARY, 3, blessingOfSanctuaryRemove)
RegisterAuraEvent(SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY, 3, blessingOfSanctuaryRemove)
