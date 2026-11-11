-- Pet spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_pet.cpp (census rows 639-660).
--
-- CENSUS CORRECTION: all 22 rows are census-marked go_hit+lua_hit, but
-- that is a false positive — zero hits for any spell_pet class name
-- anywhere in the Go tree; the hits are keyword "pet" matches
-- (spell_hunter.lua comments, pet_combat.go, boss_ai.go).
-- AddSC_pet_spell_scripts() (spell_pet.cpp:1760-1763) news ONLY
-- spell_gen_pet_calculate — the other 20 classes are never instantiated
-- anywhere in src/, so in this C++ snapshot they are dead code; their
-- arms are triaged below anyway in case the DB binds them.
--
-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_hun_pet_scaling_01 (34902, spell_pet.cpp:931-941): the
--   _tempHealth latch only. OnEffectRemove stashes the pet's health;
--   AfterEffectApply restores it when set. The three CalcAmount arms
--   (stamina 45% + Wild Hunt, AP 22% ranged AP, damage 12.87% ranged
--   AP) stay out — no DoEffectCalcAmount bridge. Binding ID 34902 is
--   the C++ enum SPELL_HUNTER_PET_SCALING_01 (:45); no
--   spell_script_names row in-tree. ev2/ev3 fire on the creature path
--   (spells.go:40867 / :46414/:46514); creature SetHealth/GetHealth
--   are bridged (boss_midnight.lua precedent).
--   spell_dk_pet_scaling_01 (54566, spell_pet.cpp:1534-1544): the same
--   _tempHealth latch shape. CalcAmount arms (stamina 30% + Ravenous
--   Dead / Glyph of the Ghoul, strength 70% + same) stay out.
--   Latch tables are keyed by pet GUID (one aura instance per pet);
--   C++ never clears _tempHealth except by overwriting on remove,
--   so the Lua table keeps the last saved value too.
--
-- STAY-OUT:
--   639 spell_gen_pet_calculate (19591/20784/34666/34667/34675, 35695,
--   61013/61017, 61697): the only registered class; all five
--   CalculateAmount arms (crit spell/melee, hit melee/spell,
--   expertise) are DoEffectCalcAmount — no Lua bridge. Go recipe:
--   aura-amount calc hook in the engine.
--   640/641 spell_warl_pet_scaling_01 (34947) / _02 (34956):
--   Apply/Remove arms need SetCreateHealth/SetCreateMana + PetLevelInfo
--   (no bridge; Go creatures have no CreateHealth/CreateMana) and
--   their _tempBonus input is set by the CalcAmount arm.
--   642-644 warl _03 (34957) / _04 (34958) / _05 (61013), 645 warl
--   passive crit, 646 warl passive damage done (entry-based
--   -16/-20/+5), 647 voidwalker glyph leg, 648 sha _04 (61783),
--   650-652 hun _02 (34903) / _03 (34904) / _04 (61017), 653 hun
--   passive crit (arms are literal no-ops: all terms commented out,
--   amount += 0 * 0.8), 654 hun passive damage done (happiness +
--   Cobra Reflexes 61682), 655 animal handler (34453), 656 dk
--   avoidance (army ghoul -90 / Night of the Dead 55620), 658 dk _02
--   (51996 melee haste), 659 dk _03 (61697 hit), 660 rune weapon _02
--   (51906 damage + haste, Guardian SetBonusDamage): all
--   DoEffectCalcAmount only — no Lua bridge.

local SPELL_HUN_PET_SCALING_01 = 34902
local SPELL_DK_PET_SCALING_01 = 54566

local hunPetHealth = {}
local dkPetHealth = {}

local function hunPetScaling01Apply(event, aura, target)
    local saved = hunPetHealth[target:GetGUID()]
    if saved and saved > 0 then
        target:SetHealth(saved)
    end
end

local function hunPetScaling01Remove(event, aura, target)
    hunPetHealth[target:GetGUID()] = target:GetHealth()
end

RegisterAuraEvent(SPELL_HUN_PET_SCALING_01, 2, hunPetScaling01Apply)
RegisterAuraEvent(SPELL_HUN_PET_SCALING_01, 3, hunPetScaling01Remove)

local function dkPetScaling01Apply(event, aura, target)
    local saved = dkPetHealth[target:GetGUID()]
    if saved and saved > 0 then
        target:SetHealth(saved)
    end
end

local function dkPetScaling01Remove(event, aura, target)
    dkPetHealth[target:GetGUID()] = target:GetHealth()
end

RegisterAuraEvent(SPELL_DK_PET_SCALING_01, 2, dkPetScaling01Apply)
RegisterAuraEvent(SPELL_DK_PET_SCALING_01, 3, dkPetScaling01Remove)
