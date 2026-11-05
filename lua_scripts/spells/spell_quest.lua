-- Quest spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_quest.cpp.
--
-- PORTED (tick 198, RegisterAuraEvent OnRemove = 3):
--   spell_q10929_fumping::HandleEffectRemove
--   (spell_quest.cpp:2641-2651): EFFECT_0 SPELL_AURA_DUMMY. On
--   removal by expiry only, the caster casts one of Summon Sand
--   Gnome 39240 / Summon Bone Slicer 39241 on itself, triggered
--   (C++ urand picks uniformly). The mode gate is
--   Aura:GetRemoveMode (new tick 198: SpellAuraDefines.h:61-69
--   values).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so a player caster's
-- triggered self-cast lands non-triggered; pre-existing bridge gap,
-- same as all prior player-target ports.

local SPELL_FUMPING = 39238 -- spell_quest.cpp:2630
local SPELL_SUMMON_SAND_GNOME = 39240 -- spell_quest.cpp:2626
local SPELL_SUMMON_BONE_SLICER = 39241 -- spell_quest.cpp:2627
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

local function fumpingRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if math.random(0, 1) == 0 then
        caster:CastSpell(caster, SPELL_SUMMON_SAND_GNOME, true)
    else
        caster:CastSpell(caster, SPELL_SUMMON_BONE_SLICER, true)
    end
end

RegisterAuraEvent(SPELL_FUMPING, 3, fumpingRemove)

local SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3 = 43874 -- spell_quest.cpp:296
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132

-- C++ spell_q11396_11399_force_shield_arcane_purple_x3_AuraScript::
-- HandleEffectApply (spell_quest.cpp:308-314): EFFECT_0 DUMMY, REAL.
-- Target immune to PC. STILL BLOCKED leg: AddUnitState(UNIT_STATE_ROOT)
-- (no bridge).
local function forceShieldApply(event, aura, target)
    target:SetFlag(59, UNIT_FLAG_IMMUNE_TO_PC)
end

RegisterAuraEvent(SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3, 2, forceShieldApply)

-- C++ spell_q11396_11399_force_shield_arcane_purple_x3_AuraScript::
-- HandleEffectRemove (spell_quest.cpp:316-319): clears PC immunity.
local function forceShieldRemove(event, aura, target)
    target:RemoveFlag(59, UNIT_FLAG_IMMUNE_TO_PC)
end

RegisterAuraEvent(SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3, 3, forceShieldRemove)
