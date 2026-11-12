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
--   / 100) (CalculatePct, float-truncate). TDB maps this script name
--   to 62606, the absorb buff it casts — a false missing; no handler
--   registers on 62606. DORMANT until Spell.dbc
--   ships (the anetheron 38196 class: serverSpellProcEntryFor needs
--   the DBC ProcTypeMask). AC binds the script to 62606, but 62606
--   is the absorb buff itself (Wowhead/wotlkdb 3.3.5a: 62600 triggers
--   62606); TDB's 62600 binding is the DBC-true one. No rank-2 proc
--   aura is bound in either DB's spell_script_names.
--   spell_dru_moonkin_form_passive (spell_druid.cpp:936-976): aura
--   69366, mode 9 (OnEffectAbsorb). Unlimited pool; stunned +
--   stun-mechanic aura gates CalculatePct(damage, EFFECT_0 CalcValue),
--   else 0. New AuraEvent 9 bridge + HasAuraWithMechanic (this tick).
--   spell_dru_primal_tenacity (spell_druid.cpp:1102-1140): aura 33851,
--   mode 9 (OnEffectAbsorb). Same shape on EFFECT_1, plus the cat-form
--   gate. Both DORMANT until Spell.dbc ships.
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

-- C++ spell_dru_forms_trinket::CheckProc + HandleProc
-- (spell_druid.cpp:348-421): aura 37336, EFFECT_0
-- SPELL_AURA_PROC_TRIGGER_SPELL. CheckProc allows only the forms
-- the trinket knows (bear/direbear, cat, moonkin, none, tree);
-- any other form vetoes the proc. HandleProc calls
-- PreventDefaultAction and casts the form's trigger spell on the
-- proc actor, triggered (the aurEff CastSpell overload). The Lua
-- return-true veto is both legs: a disallowed form suppresses the
-- default trigger exactly as CheckProc false does, and an allowed
-- form suppresses it as PreventDefaultAction does. Form values are
-- SpellAuraDefines.h:409-438. DORMANT until Spell.dbc ships the
-- 37336 proc entry (serverSpellProcEntryFor needs the DBC
-- ProcTypeMask, the savage-defense 62600 precedent).
local SPELL_DRUID_FORMS_TRINKET = 37336 -- spell_druid.cpp:348
local SPELL_DRUID_FORMS_TRINKET_BEAR = 37340 -- spell_druid.cpp:44
local SPELL_DRUID_FORMS_TRINKET_CAT = 37341 -- spell_druid.cpp:45
local SPELL_DRUID_FORMS_TRINKET_MOONKIN = 37343 -- spell_druid.cpp:46
local SPELL_DRUID_FORMS_TRINKET_NONE = 37344 -- spell_druid.cpp:47
local SPELL_DRUID_FORMS_TRINKET_TREE = 37342 -- spell_druid.cpp:48
local FORM_NONE = 0 -- SpellAuraDefines.h:409
local FORM_CAT = 1 -- SpellAuraDefines.h:410
local FORM_TREE = 2 -- SpellAuraDefines.h:411
local FORM_BEAR = 5 -- SpellAuraDefines.h:414
local FORM_DIREBEAR = 8 -- SpellAuraDefines.h:417
local FORM_MOONKIN = 31 -- SpellAuraDefines.h:438

local FORM_TRINKET_SPELLS = {
    [FORM_BEAR] = SPELL_DRUID_FORMS_TRINKET_BEAR,
    [FORM_DIREBEAR] = SPELL_DRUID_FORMS_TRINKET_BEAR,
    [FORM_CAT] = SPELL_DRUID_FORMS_TRINKET_CAT,
    [FORM_MOONKIN] = SPELL_DRUID_FORMS_TRINKET_MOONKIN,
    [FORM_NONE] = SPELL_DRUID_FORMS_TRINKET_NONE,
    [FORM_TREE] = SPELL_DRUID_FORMS_TRINKET_TREE,
}

local function formsTrinketProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    local unit = actor
    if unit == nil and procEvent ~= nil then
        unit = procEvent:GetActor()
    end
    if unit == nil then
        return true
    end
    local triggerSpell = FORM_TRINKET_SPELLS[unit:GetShapeshiftForm()]
    if triggerSpell == nil then
        return true
    end
    unit:CastSpell(unit, triggerSpell, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_FORMS_TRINKET, 6, formsTrinketProc)

-- C++ spell_dru_moonkin_form_passive::Absorb (spell_druid.cpp:936-976):
-- 69366 EFFECT_0 SPELL_AURA_SCHOOL_ABSORB. The pool is unlimited
-- (CalculateAmount sets -1), so the handler always returns an explicit
-- amount: while the target is stunned (UNIT_FLAG_STUNNED) with a
-- stun-mechanic aura, absorb = CalculatePct(damage, EFFECT_0
-- CalcValue); otherwise 0. The Go pool candidate is the DBC base, not
-- C++'s 0, so returning nil would keep a bogus pool absorb. DORMANT
-- until Spell.dbc ships the 69366 row.
local SPELL_DRUID_MOONKIN_FORM_PASSIVE = 69366
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117
local UNIT_FLAG_STUNNED = 0x00040000 -- UnitDefines.h:142
local MECHANIC_STUN_MASK = 4096 -- 1 << MECHANIC_STUN (12), SharedDefines.h:1369

local function moonkinFormPassiveAbsorb(event, aura, target, damage, absorbAmount)
    if target == nil then
        return 0
    end
    if not target:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED) then
        return 0
    end
    if not target:HasAuraWithMechanic(MECHANIC_STUN_MASK) then
        return 0
    end
    local pct = aura:GetSpellEffectCalcValue(SPELL_DRUID_MOONKIN_FORM_PASSIVE, 0)
    if pct <= 0 then
        return 0
    end
    return math.floor(damage * pct / 100)
end

RegisterAuraEvent(SPELL_DRUID_MOONKIN_FORM_PASSIVE, 9, moonkinFormPassiveAbsorb)

-- C++ spell_dru_primal_tenacity::Absorb (spell_druid.cpp:1102-1140):
-- 33851 EFFECT_1 SPELL_AURA_SCHOOL_ABSORB. Same shape as moonkin:
-- unlimited pool (-1), absorb = CalculatePct(damage, EFFECT_1
-- CalcValue) only while the target is in cat form, stunned, with a
-- stun-mechanic aura; otherwise 0. DORMANT until Spell.dbc ships the
-- 33851 row.
local SPELL_DRUID_PRIMAL_TENACITY = 33851

local function primalTenacityAbsorb(event, aura, target, damage, absorbAmount)
    if target == nil then
        return 0
    end
    if target:GetShapeshiftForm() ~= FORM_CAT then
        return 0
    end
    if not target:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED) then
        return 0
    end
    if not target:HasAuraWithMechanic(MECHANIC_STUN_MASK) then
        return 0
    end
    local pct = aura:GetSpellEffectCalcValue(SPELL_DRUID_PRIMAL_TENACITY, 1)
    if pct <= 0 then
        return 0
    end
    return math.floor(damage * pct / 100)
end

RegisterAuraEvent(SPELL_DRUID_PRIMAL_TENACITY, 9, primalTenacityAbsorb)
