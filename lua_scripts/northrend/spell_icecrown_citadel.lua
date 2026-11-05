-- Icecrown Citadel spell aura scripts -- Lua ports of the AuraScript
-- arms in src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp.
--
-- PORTED (tick 197, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_icc_empowered_blood (70227, :1168-1187) and
--   spell_icc_empowered_blood_3 (70304, :1195-1214): EFFECT_0
--   MOD_DAMAGE_PERCENT_DONE. OnApply the target casts the next
--   Empowered Blood rank on itself, triggered (70232 / 70320);
--   AfterEffectRemove the target drops that rank. The remove leg is
--   C++ RemoveAurasDueToSpell; the Go twin is target:RemoveAura(id):
--   each rank is self-cast once by the apply leg, so a single
--   application exists and removing the spell's aura is the same
--   removal (removeCreatureAura / removeAura drop the active aura
--   of that spell ID). No Load gates in C++; no PreventDefaultAction.
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so a player target's
-- triggered self-cast lands non-triggered; pre-existing bridge gap,
-- same as all prior player-target ports.

local SPELL_ICC_EMPOWERED_BLOOD = 70227 -- icecrown_citadel.cpp:93
local SPELL_ICC_EMPOWERED_BLOOD_2 = 70232 -- icecrown_citadel.cpp:94
local SPELL_ICC_EMPOWERED_BLOOD_3 = 70304 -- icecrown_citadel.cpp:95
local SPELL_ICC_EMPOWERED_BLOOD_4 = 70320 -- icecrown_citadel.cpp:96

-- C++ spell_icc_empowered_blood::OnApply (icecrown_citadel.cpp:1176-1179).
local function empoweredBloodApply(event, aura, target)
    target:CastSpell(target, SPELL_ICC_EMPOWERED_BLOOD_2, true)
end

RegisterAuraEvent(SPELL_ICC_EMPOWERED_BLOOD, 2, empoweredBloodApply)

-- C++ spell_icc_empowered_blood::OnRemove (icecrown_citadel.cpp:1181-1184).
local function empoweredBloodRemove(event, aura, target)
    target:RemoveAura(SPELL_ICC_EMPOWERED_BLOOD_2)
end

RegisterAuraEvent(SPELL_ICC_EMPOWERED_BLOOD, 3, empoweredBloodRemove)

-- C++ spell_icc_empowered_blood_3::OnApply (icecrown_citadel.cpp:1203-1206).
local function empoweredBlood3Apply(event, aura, target)
    target:CastSpell(target, SPELL_ICC_EMPOWERED_BLOOD_4, true)
end

RegisterAuraEvent(SPELL_ICC_EMPOWERED_BLOOD_3, 2, empoweredBlood3Apply)

-- C++ spell_icc_empowered_blood_3::OnRemove (icecrown_citadel.cpp:1208-1211).
local function empoweredBlood3Remove(event, aura, target)
    target:RemoveAura(SPELL_ICC_EMPOWERED_BLOOD_4)
end

RegisterAuraEvent(SPELL_ICC_EMPOWERED_BLOOD_3, 3, empoweredBlood3Remove)
