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

local SPELL_ICC_STONEFORM = 70733 -- icecrown_citadel.cpp:1306
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132
local REACT_PASSIVE = 0 -- UnitDefines.h
local REACT_AGGRESSIVE = 2 -- UnitDefines.h
local EMOTE_STATE_CUSTOM_SPELL_02 = 417 -- SharedDefines.h:2054

-- C++ spell_icc_stoneform::OnApply (icecrown_citadel.cpp:1308-1318):
-- EFFECT_0 DUMMY, REAL. Creature target only: react passive,
-- NOT_SELECTABLE + IMMUNE_TO_PC flags, custom-spell-02 emote state.
-- DELTA: C++ SetImmuneToPC also ends combat refs with
-- player-controlled attackers (Unit.cpp:8453-8468, keepCombat=false);
-- Go SetFlag sets the flag only.
local function stoneformApply(event, aura, target)
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:SetReactState(REACT_PASSIVE)
    target:SetFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    target:SetFlag(59, UNIT_FLAG_IMMUNE_TO_PC)
    target:EmoteState(EMOTE_STATE_CUSTOM_SPELL_02)
end

RegisterAuraEvent(SPELL_ICC_STONEFORM, 2, stoneformApply)

-- C++ spell_icc_stoneform::OnRemove (icecrown_citadel.cpp:1322-1331):
-- react aggressive, both flags cleared, emote state 0.
local function stoneformRemove(event, aura, target)
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:SetReactState(REACT_AGGRESSIVE)
    target:RemoveFlag(59, UNIT_FLAG_NOT_SELECTABLE)
    target:RemoveFlag(59, UNIT_FLAG_IMMUNE_TO_PC)
    target:EmoteState(0)
end

RegisterAuraEvent(SPELL_ICC_STONEFORM, 3, stoneformRemove)

local SPELL_REMOVE_EMPOWERED_BLOOD = 72131 -- icecrown_citadel.cpp:1284; boss_blood_prince_council.cpp:110

-- C++ spell_generic_remove_empowered_blood::HandleScript (icecrown_citadel.cpp:1286-1305):
-- EFFECT_0 SCRIPT_EFFECT removes Empowered Blood (70227) from the hit
-- unit. Caster verified: Prince Valanar self-casts 72131 triggered in
-- the BPC controller Reset (boss_blood_prince_council.cpp:248).
-- 70939 (Lana'thel's leg) is already handled natively in Go
-- (boss_ai.go:7864) and is NOT registered here. No Go case for 72131,
-- so no double-fire.
local function removeEmpoweredBlood(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    hitTarget:RemoveAura(SPELL_ICC_EMPOWERED_BLOOD)
end

RegisterSpellEvent(SPELL_REMOVE_EMPOWERED_BLOOD, 5, removeEmpoweredBlood)

local SPELL_HARVEST_BLIGHT_SPECIMEN = 72155 -- icecrown_citadel.cpp:88

-- C++ spell_icc_harvest_blight_specimen::HandleScript / HandleQuestComplete
-- (icecrown_citadel.cpp:1455-1476): EFFECT_0 SCRIPT_EFFECT and EFFECT_1
-- QUEST_COMPLETE each remove the aura named by the effect value from the
-- hit unit. Caster verified: Alchemist Adrianna's gossip casts 72155
-- (npc_alchemist_adrianna.lua). No Go case for 72155, so no double-fire.
local function harvestBlightSpecimen(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 and effectIndex ~= 1 then
        return
    end
    if hitTarget == nil then
        return
    end
    local value = spell:GetEffectValue(effectIndex)
    if value > 0 then
        hitTarget:RemoveAura(value)
    end
end

RegisterSpellEvent(SPELL_HARVEST_BLIGHT_SPECIMEN, 5, harvestBlightSpecimen)

local SPELL_ICC_SIPHON_ESSENCE = 70299 -- icecrown_citadel.cpp:101
local AURA_REMOVE_BY_CANCEL = 2 -- SpellAuraDefines.h:61-69 (NONE 0, DEFAULT 1, CANCEL 2)

-- C++ spell_icc_siphon_essence::OnRemove (icecrown_citadel.cpp:1221-1235):
-- EFFECT_1 MOD_ROOT, REAL; only when the aura is removed by cancel does
-- the target's AI take DoAction(ACTION_SIPHON_INTERRUPTED 1). The
-- remove-mode gate is the whole leg (lua_aura.go:238 GetRemoveMode);
-- other remove modes take no arm. The DoAction receiver lives in
-- npc_darkfallen_icc.lua (DarkFallenAI :826-830).
local function siphonEssenceRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_CANCEL then
        return
    end
    if target:GetObjectType() ~= "Creature" then
        return
    end
    target:DoAction(1)
end

RegisterAuraEvent(SPELL_ICC_SIPHON_ESSENCE, 3, siphonEssenceRemove)
