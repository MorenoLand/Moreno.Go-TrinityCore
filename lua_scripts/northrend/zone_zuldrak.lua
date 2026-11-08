-- Zul'Drak zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_zuldrak.cpp.
--
-- PORTED (tick 197, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_scourge_disguise (51966, zone_zuldrak.cpp:966-981):
--   EFFECT_0 TRANSFORM. OnApply the target casts Scourge Disguise
--   Instability (51971) on itself, triggered; OnRemove the target
--   removes that instability aura (C++ plain RemoveAura). No Load
--   gates in C++; no PreventDefaultAction. This supersedes the
--   "no AuraScript bridge — documented-only" note in
--   npc_storm_cloud.lua, which predates the tick-196
--   RegisterAuraEvent hook.
-- PORTED (tick 424, RegisterAuraEvent OnPeriodic = 1):
--   spell_random_ingredient_aura (51015 / 51154 / 51157,
--   zone_zuldrak.cpp:657-671): EFFECT_0 SPELL_AURA_PERIODIC_
--   DUMMY. Each tick the target self-casts the matching Random
--   Ingredient spell (51134 / 51105 / 51107), non-triggered.
--   No PreventDefaultAction in C++ (PERIODIC_DUMMY has no
--   default tick action to suppress).
-- STILL BLOCKED:
--   spell_scourge_disguise_instability (51971, :984-1012):
--   CalcPeriodic picks a random 30-240s amplitude (no calc-periodic
--   hook) and the periodic tick casts Scourge Disguise Expiring
--   (52010) on the target, triggered.
--   spell_scourge_disguise_expiring (52010, :1014-1030): OnApply
--   whispers TEXT_DISGUISE_WARNING (28891) to the player target —
--   PORTED (tick 204, below) via the new player:WhisperText bridge
--   (Unit::Whisper textId overload, lua_player.go).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the disguised
-- player's triggered self-cast lands non-triggered; pre-existing
-- bridge gap, same as all prior player-target ports.

local SPELL_SCOURGE_DISGUISE = 51966 -- zone_zuldrak.cpp:957
local SPELL_SCOURGE_DISGUISE_INSTABILITY = 51971 -- zone_zuldrak.cpp:958
local SPELL_SCOURGE_DISGUISE_EXPIRING = 52010 -- zone_zuldrak.cpp:959
local TEXT_DISGUISE_WARNING = 28891 -- zone_zuldrak.cpp:961
local SPELL_RANDOM_INGREDIENT_EASY_AURA = 51015 -- zone_zuldrak.cpp:368
local SPELL_RANDOM_INGREDIENT_MEDIUM_AURA = 51154 -- zone_zuldrak.cpp:369
local SPELL_RANDOM_INGREDIENT_HARD_AURA = 51157 -- zone_zuldrak.cpp:370
local SPELL_RANDOM_INGREDIENT_EASY = 51134 -- zone_zuldrak.cpp:371
local SPELL_RANDOM_INGREDIENT_MEDIUM = 51105 -- zone_zuldrak.cpp:372
local SPELL_RANDOM_INGREDIENT_HARD = 51107 -- zone_zuldrak.cpp:373

-- C++ spell_scourge_disguise::ApplyEffect (zone_zuldrak.cpp:969-972).
local function scourgeDisguiseApply(event, aura, target)
    target:CastSpell(target, SPELL_SCOURGE_DISGUISE_INSTABILITY, true)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE, 2, scourgeDisguiseApply)

-- C++ spell_scourge_disguise::RemoveEffect (zone_zuldrak.cpp:974-977).
local function scourgeDisguiseRemove(event, aura, target)
    target:RemoveAura(SPELL_SCOURGE_DISGUISE_INSTABILITY)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE, 3, scourgeDisguiseRemove)

-- C++ spell_scourge_disguise_expiring::OnApply (zone_zuldrak.cpp:1018-1025):
-- if the player target still has the disguise, whisper the warning text
-- to them (Unit::Whisper textId overload, bossWhisper=true -> 0x2A).
-- DELTA: C++ localizes broadcast_text by target locale + sender gender;
-- the Go bridge reads the enUS Text column only.
local function scourgeDisguiseExpiringApply(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    if target:HasAura(SPELL_SCOURGE_DISGUISE) then
        target:WhisperText(TEXT_DISGUISE_WARNING, target, true)
    end
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE_EXPIRING, 2, scourgeDisguiseExpiringApply)

-- C++ spell_random_ingredient_aura::PeriodicTick
-- (zone_zuldrak.cpp:657-671): EFFECT_0 SPELL_AURA_PERIODIC_DUMMY.
-- Each tick the target self-casts the matching Random Ingredient
-- spell, non-triggered, keyed on the aura's own spell id
-- (C++ switch on GetSpellInfo()->Id; aura:GetAuraId() is the twin).
local function randomIngredientTick(event, aura, target)
    local cast = nil
    local spellId = aura:GetAuraId()
    if spellId == SPELL_RANDOM_INGREDIENT_EASY_AURA then
        cast = SPELL_RANDOM_INGREDIENT_EASY
    elseif spellId == SPELL_RANDOM_INGREDIENT_MEDIUM_AURA then
        cast = SPELL_RANDOM_INGREDIENT_MEDIUM
    elseif spellId == SPELL_RANDOM_INGREDIENT_HARD_AURA then
        cast = SPELL_RANDOM_INGREDIENT_HARD
    end
    if cast ~= nil and target ~= nil then
        target:CastSpell(target, cast)
    end
end

RegisterAuraEvent(SPELL_RANDOM_INGREDIENT_EASY_AURA, 1, randomIngredientTick)
RegisterAuraEvent(SPELL_RANDOM_INGREDIENT_MEDIUM_AURA, 1, randomIngredientTick)
RegisterAuraEvent(SPELL_RANDOM_INGREDIENT_HARD_AURA, 1, randomIngredientTick)
