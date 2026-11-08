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
-- PORTED (tick 426, RegisterAuraEvent OnPeriodic = 1):
--   spell_scourge_disguise_instability (51971,
--   zone_zuldrak.cpp:984-1012): EFFECT_0 SPELL_AURA_DUMMY. Each
--   tick the target self-casts Scourge Disguise Expiring (52010),
--   triggered. DOCUMENTED DEVIATION: C++ CalcPeriodic re-rolls the
--   amplitude to irand(30, 240)s after every tick
--   (HandleUpdatePeriodic -> CalculatePeriodic) and no
--   CalcPeriodic hook exists in Go, so the tick interval here is
--   whatever the aura engine delivers from the DBC amplitude — the
--   cast leg is exact, the timing is not.
-- STILL BLOCKED:
-- PORTED (tick 425, RegisterAuraEvent OnRemove = 3):
--   spell_fetch_ingredient_aura (zone_zuldrak.cpp:866-899): one
--   shared class covers all 21 fetch auras (51018/51055/51057/51059/
--   51062/51064/51067/51069/51072/51077/51079 easy, 51081/51083/51085/
--   51087/51091 medium, 51093/51095/51097/51100/51102 hard).
--   Expire-only + invisbuff 51216 + Finklestein 28205 within 100yd ->
--   drop the invisbuff, Finklestein Talk(SAY_RUINED 4). DORMANT: the
--   sender spell_random_ingredient (:676-755) is an unported
--   SpellScript, so no fetch aura lands yet.
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

-- C++ spell_scourge_disguise_instability::HandleDummyTick
-- (zone_zuldrak.cpp:995-998): OnEffectPeriodic, EFFECT_0
-- SPELL_AURA_DUMMY: the target self-casts Scourge Disguise Expiring
-- (52010), triggered. No PreventDefaultAction in C++ (DUMMY has no
-- default tick action to suppress). DOCUMENTED DEVIATION: the
-- C++ tick interval is re-rolled to irand(30, 240)s after every
-- tick (CalcPeriodic :989-993 + HandleUpdatePeriodic :1000-1003),
-- and no CalcPeriodic hook exists in Go — the interval here is
-- whatever the aura engine delivers from the DBC amplitude.
local function scourgeDisguiseInstabilityTick(event, aura, target)
    target:CastSpell(target, SPELL_SCOURGE_DISGUISE_EXPIRING, true)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE_INSTABILITY, 1, scourgeDisguiseInstabilityTick)

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

-- C++ spell_fetch_ingredient_aura_AuraScript::OnRemove
-- (zone_zuldrak.cpp:878-889): AfterEffectRemove, EFFECT_0
-- SPELL_AURA_DUMMY, AURA_EFFECT_HANDLE_REAL. Expire-only, and only
-- while the target still carries the apprentice invisbuff (51216)
-- and Finklestein (28205) is within 100yd: drop the invisbuff and
-- have Finklestein Talk(SAY_RUINED) to the target. C++ arm order
-- (expire gate, invisbuff gate, creature-found gate) preserved.
-- PORTED 2026-11-08 (tick 425). DORMANT-BY-CONSTRUCTION: the sender,
-- spell_random_ingredient (zone_zuldrak.cpp:676-755, Finklestein
-- casting FetchIngredients[i][0] on the player), is an unported
-- SpellScript, so no fetch aura lands in Go yet; this handler is
-- exact and goes live when the sender ports. The expire-only gate
-- also depends on the aura duration, which comes from Spell.dbc
-- (absent here).
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67; teron_gorefiend.lua:121 precedent
local NPC_FINKLESTEIN = 28205 -- zone_zuldrak.cpp:340
local SPELL_ALCHEMIST_APPRENTICE_INVISBUFF = 51216 -- zone_zuldrak.cpp:365
local SAY_RUINED = 4 -- zone_zuldrak.cpp:447

local function fetchIngredientRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    if not target:HasAura(SPELL_ALCHEMIST_APPRENTICE_INVISBUFF) then
        return
    end
    local finklestein = target:FindNearestCreature(NPC_FINKLESTEIN, 100.0)
    if finklestein == nil then
        return
    end
    target:RemoveAura(SPELL_ALCHEMIST_APPRENTICE_INVISBUFF)
    finklestein:Talk(SAY_RUINED, target)
end

-- The 21 FETCH spell ids (zone_zuldrak.cpp:378-402, in
-- FetchIngredients table order :610-632): 11 easy, 5 medium, 5 hard.
-- Easy
RegisterAuraEvent(51018, 3, fetchIngredientRemove) -- SPELL_FETCH_KNOTROOT
RegisterAuraEvent(51055, 3, fetchIngredientRemove) -- SPELL_FETCH_PICKLED_EAGLE_EGG
RegisterAuraEvent(51057, 3, fetchIngredientRemove) -- SPELL_FETCH_SPECKLED_GUANO
RegisterAuraEvent(51059, 3, fetchIngredientRemove) -- SPELL_FETCH_WITHERED_BATWING
RegisterAuraEvent(51062, 3, fetchIngredientRemove) -- SPELL_FETCH_SEASONED_SLIDER_CIDER
RegisterAuraEvent(51064, 3, fetchIngredientRemove) -- SPELL_FETCH_PULVERIZED_GARGOYLE_TEETH
RegisterAuraEvent(51067, 3, fetchIngredientRemove) -- SPELL_FETCH_MUDDY_MIRE_MAGGOT
RegisterAuraEvent(51069, 3, fetchIngredientRemove) -- SPELL_FETCH_SPIKY_SPIDER_EGG
RegisterAuraEvent(51072, 3, fetchIngredientRemove) -- SPELL_FETCH_HAIRY_HERRING_HEAD
RegisterAuraEvent(51077, 3, fetchIngredientRemove) -- SPELL_FETCH_PUTRID_PIRATE_PERSPIRATION
RegisterAuraEvent(51079, 3, fetchIngredientRemove) -- SPELL_FETCH_ICECROWN_BOTTLED_WATER
-- Medium
RegisterAuraEvent(51081, 3, fetchIngredientRemove) -- SPELL_FETCH_WASPS_WINGS
RegisterAuraEvent(51083, 3, fetchIngredientRemove) -- SPELL_FETCH_PRISMATIC_MOJO
RegisterAuraEvent(51085, 3, fetchIngredientRemove) -- SPELL_FETCH_RAPTOR_CLAW
RegisterAuraEvent(51087, 3, fetchIngredientRemove) -- SPELL_FETCH_AMBERSEED
RegisterAuraEvent(51091, 3, fetchIngredientRemove) -- SPELL_FETCH_SHRUNKEN_DRAGONS_CLAW
-- Hard
RegisterAuraEvent(51093, 3, fetchIngredientRemove) -- SPELL_FETCH_CHILLED_SERPENT_MUCUS
RegisterAuraEvent(51095, 3, fetchIngredientRemove) -- SPELL_FETCH_CRYSTALLIZED_HOGSNOT
RegisterAuraEvent(51097, 3, fetchIngredientRemove) -- SPELL_FETCH_CRUSHED_BASILISK_CRYSTALS
RegisterAuraEvent(51100, 3, fetchIngredientRemove) -- SPELL_FETCH_TROLLBANE
RegisterAuraEvent(51102, 3, fetchIngredientRemove) -- SPELL_FETCH_FROZEN_SPIDER_ICHOR
