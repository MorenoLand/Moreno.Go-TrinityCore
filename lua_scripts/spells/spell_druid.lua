-- Druid spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_druid.cpp.
--
-- Binding IDs come from the `// <id> - Name` header comments above
-- each class (the SpellScriptLoader name binds in DB
-- spell_script_names); the effect spell IDs are the file's enums.
--
-- PORTED (tick 294, RegisterAuraEvent):
--   spell_dru_barkskin (spell_druid.cpp:109-130): aura 22812, mode
--   3. OnRemove the target drops 63058 (Barkskin damage-reduction
--   aura). Notification-only.
--   spell_dru_enrage (spell_druid.cpp:285-347): aura 5229, modes 2
--   + 3. Mode 2: the King of the Jungle effect
--   (GetAuraEffectOfRankedSpell(48492, 0)), when present, casts
--   51185 with BP0 = that amount, triggered; while the T10 Feral 4P
--   aura 70726 is up, casts 70725 triggered. Mode 3: removes 51185
--   then 70725. The RecalculateBaseArmor leg stays out (no bridge;
--   a no-op in the Go model since the bear-form passive CalcAmount
--   arm is itself stay-out).
--   spell_dru_frenzied_regeneration (spell_druid.cpp:443-477): aura
--   22842, mode 1. PeriodicTick: returns unless the target's power
--   type is rage; returns when rage is 0; converts mod = min(rage,
--   100) rage into health: pct = floor(EFFECT_1 CalcValue of 22842
--   * mod / 100) (DBC read, 0 returns without consuming rage),
--   regen = floor(maxHealth * pct / 100) (CalculatePct, Util.h:66,
--   float multiply then truncate), cast 22845 on self with BP0 =
--   regen, then set rage to rage - mod. No return true: C++ has no
--   PreventDefaultAction (PERIODIC_DUMMY), so the Go default tick
--   still fires.
--   spell_dru_lifebloom (spell_druid.cpp:828-883): aura 33763, mode
--   3, PARTIAL. AfterRemove fires only on expire
--   (GetRemoveMode() == 4, AURA_REMOVE_BY_EXPIRE): the target casts
--   the final heal 33778 on itself with BP0 = the EFFECT_1 amount
--   (live, stack-scaled). The mana-energize leg (64372, needs the
--   caster's GetCreateMana/ManaCostPercentage) and the dispel leg
--   (no OnDispel hook) stay out.
--   spell_dru_savage_roar_aura (spell_druid.cpp:1296-1323): aura
--   52610, modes 2 + 3. Apply casts 62071 on the target, triggered;
--   remove drops 62071. 62071 is the buff the handler casts, not
--   the script's own aura.
--   spell_dru_survival_instincts_aura (spell_druid.cpp:1394-1423):
--   aura 61336, modes 2 + 4 + 3. Apply (and amount-change, mode 4:
--   C++ registers AURA_EFFECT_HANDLE_CHANGE_AMOUNT_MASK) casts
--   50322 with BP0 = CountPctFromMaxHealth(EFFECT_0 amount);
--   remove drops 50322. Mode 2 is fresh-apply only; the C++ apply
--   leg is CHANGE_AMOUNT, so mode 4 covers stack-rise re-applies.
--   spell_dru_savage_defense (spell_druid.cpp:1252-1274): aura
--   62600, mode 6 (OnEffectProc). The actor casts the absorb 62606
--   on self, triggered, with BP0 = floor(total AP * EFFECT_0 amount
--   / 100) (CalculatePct, float-truncate). DORMANT until Spell.dbc
--   ships (the anetheron 38196 class: serverSpellProcEntryFor needs
--   the DBC ProcTypeMask). AC binds the script to 62606, but 62606
--   is the absorb buff itself (Wowhead/wotlkdb 3.3.5a: 62600 triggers
--   62606); TDB's 62600 binding is the DBC-true one. No rank-2 proc
--   aura is bound in either DB's spell_script_names.
-- STAY-OUT (tick 294): every other spell_druid class — CalcAmount/
--   hooks absent, or SpellScript-only with no RegisterSpellScript in
--   the engine. Notable: rip (combo points + attack power), berserk
--   (ResetCooldowns), nurturing_instinct (GetRank), living_seed
--   (proc), wild_growth (proc + CalcAmount).

local SPELL_DRUID_BARKSKIN = 22812
local SPELL_DRUID_BARKSKIN_01 = 63058
local SPELL_DRUID_ENRAGE = 5229
local SPELL_DRUID_KING_OF_THE_JUNGLE = 48492
local SPELL_DRUID_ENRAGE_MOD_DAMAGE = 51185
local SPELL_DRUID_ITEM_T10_FERAL_4P_BONUS = 70726
local SPELL_DRUID_ENRAGED_DEFENSE = 70725
local SPELL_DRUID_FRENZIED_REGENERATION = 22842
local SPELL_DRUID_FRENZIED_REGENERATION_HEAL = 22845
local SPELL_DRUID_LIFEBLOOM = 33763
local SPELL_DRUID_LIFEBLOOM_FINAL_HEAL = 33778
local SPELL_DRUID_SAVAGE_ROAR = 52610
local SPELL_DRUID_SAVAGE_ROAR_BUFF = 62071
local SPELL_DRUID_SURVIVAL_INSTINCTS = 61336
local SPELL_DRUID_SURVIVAL_INSTINCTS_BUFF = 50322
local SPELL_DRUID_SAVAGE_DEFENSE = 62600
local SPELL_DRUID_SAVAGE_DEFENSE_ABSORB = 62606

local POWER_RAGE = 1
local AURA_REMOVE_BY_EXPIRE = 4

local function barkskinRemove(event, aura, target)
    target:RemoveAura(SPELL_DRUID_BARKSKIN_01)
end

RegisterAuraEvent(SPELL_DRUID_BARKSKIN, 3, barkskinRemove)

local function enrageApply(event, aura, target)
    local imp = target:GetAuraEffectOfRankedSpell(SPELL_DRUID_KING_OF_THE_JUNGLE, 0)
    if imp then
        target:CastCustomSpell(target, SPELL_DRUID_ENRAGE_MOD_DAMAGE, true, imp)
    end
    if target:HasAura(SPELL_DRUID_ITEM_T10_FERAL_4P_BONUS) then
        target:CastSpell(target, SPELL_DRUID_ENRAGED_DEFENSE, true)
    end
end

local function enrageRemove(event, aura, target)
    target:RemoveAura(SPELL_DRUID_ENRAGE_MOD_DAMAGE)
    target:RemoveAura(SPELL_DRUID_ENRAGED_DEFENSE)
end

RegisterAuraEvent(SPELL_DRUID_ENRAGE, 2, enrageApply)
RegisterAuraEvent(SPELL_DRUID_ENRAGE, 3, enrageRemove)

local function frenziedRegenerationTick(event, aura, target)
    if target:GetPowerType() ~= POWER_RAGE then
        return
    end
    local rage = target:GetPower(POWER_RAGE)
    if rage == 0 then
        return
    end
    local mod = math.min(rage, 100)
    local calc = aura:GetSpellEffectCalcValue(SPELL_DRUID_FRENZIED_REGENERATION, 1)
    if calc == 0 then
        return
    end
    local pct = math.floor(calc * mod / 100)
    local regen = math.floor(target:GetMaxHealth() * pct / 100)
    target:CastCustomSpell(nil, SPELL_DRUID_FRENZIED_REGENERATION_HEAL, true, regen)
    target:SetPower(rage - mod, POWER_RAGE)
end

RegisterAuraEvent(SPELL_DRUID_FRENZIED_REGENERATION, 1, frenziedRegenerationTick)

local function lifebloomRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local heal = aura:GetAmount(1)
    target:CastCustomSpell(target, SPELL_DRUID_LIFEBLOOM_FINAL_HEAL, true, heal)
end

RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM, 3, lifebloomRemove)

local function savageRoarApply(event, aura, target)
    target:CastSpell(target, SPELL_DRUID_SAVAGE_ROAR_BUFF, true)
end

local function savageRoarRemove(event, aura, target)
    target:RemoveAura(SPELL_DRUID_SAVAGE_ROAR_BUFF)
end

RegisterAuraEvent(SPELL_DRUID_SAVAGE_ROAR, 2, savageRoarApply)
RegisterAuraEvent(SPELL_DRUID_SAVAGE_ROAR, 3, savageRoarRemove)

-- C++ spell_dru_savage_defense::HandleProc (spell_druid.cpp:1261-1268):
-- PreventDefaultAction; the proc actor casts EFFECT_0's TriggerSpell
-- (62606) on self with BP0 = CalculatePct(total AP, EFFECT_0 amount).
local function savageDefenseProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil then
        return true
    end
    local absorb = math.floor(actor:GetTotalAttackPowerValue() * aura:GetAmount(0) / 100)
    actor:CastCustomSpell(nil, SPELL_DRUID_SAVAGE_DEFENSE_ABSORB, true, absorb)
    return true
end

RegisterAuraEvent(SPELL_DRUID_SAVAGE_DEFENSE, 6, savageDefenseProc)

local function survivalInstinctsApply(event, aura, target)
    local bp = target:CountPctFromMaxHealth(aura:GetAmount(0))
    target:CastCustomSpell(target, SPELL_DRUID_SURVIVAL_INSTINCTS_BUFF, true, bp)
end

local function survivalInstinctsRemove(event, aura, target)
    target:RemoveAura(SPELL_DRUID_SURVIVAL_INSTINCTS_BUFF)
end

RegisterAuraEvent(SPELL_DRUID_SURVIVAL_INSTINCTS, 2, survivalInstinctsApply)
RegisterAuraEvent(SPELL_DRUID_SURVIVAL_INSTINCTS, 4, survivalInstinctsApply)
RegisterAuraEvent(SPELL_DRUID_SURVIVAL_INSTINCTS, 3, survivalInstinctsRemove)
