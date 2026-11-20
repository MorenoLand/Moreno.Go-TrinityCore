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
--   then 70725. The RecalculateBaseArmor leg stays out (no
--   RecalculateAmount bridge): live, not a no-op, now that the
--   bear-form passive CalcAmount arm ported (this tick) — Enrage
--   apply/remove should re-run that amount.
--   spell_dru_frenzied_regeneration (spell_druid.cpp:443-477): aura
--   22842, mode 1. PeriodicTick: returns unless the target's power
--   type is rage; returns when rage is 0; converts mod = min(rage,
--   100) rage into health: regen = floor(maxHealth * (EFFECT_1
--   CalcValue of 22842 * mod / 100) / 100) — a single truncation
--   (CalculatePct, Util.h:66, float multiply then truncate; the
--   double-floor form read up to ~1% of max health low) — cast 22845
--   on self with BP0 = regen, then set rage to rage - mod. No return
--   true: C++ has no PreventDefaultAction (PERIODIC_DUMMY), so the Go
--   default tick still fires.
--   spell_dru_lifebloom (spell_druid.cpp:828-883): aura 33763, mode
--   3, PARTIAL. R2-R3 (48450/48451) ride the same handler — TDB
--   (-33763) NEG all-ranks binding, chain 33763/48450/48451.
--   AfterRemove fires only on expire
--   (GetRemoveMode() == 4, AURA_REMOVE_BY_EXPIRE): the target casts
--   the final heal 33778 on itself with BP0 = the EFFECT_1 amount
--   (live, stack-scaled), and the caster self-casts the mana
--   energize 64372 with BP0 = floor(floor(GetCreateMana() *
--   ManaCostPercentage / 100) * stack / 2) (stack = GetStackAmount();
--   DBC-dormant on GetSpellManaCostPercentage until the 33763 row
--   ships). The dispel leg (ev5 OnDispel, same two casts with
--   stack = 1: Go removes exactly one charge per successful dispel,
--   dispel.go:1036-1041) rides the same helper; the "no OnDispel
--   hook" claim in the 2026-11-20 audit was stale (ev5 fires from
--   dispel.go:1724). No OriginalCaster bridge (a triggered self-cast
--   is the closest available).
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
--   spell_dru_savage_roar + spell_dru_survival_instincts CheckCast
--   (spell_druid.cpp:1281-1288 / 1383-1386, ev8): savage roar fails
--   ONLY_SHAPESHIFT unless the caster is in cat form; survival
--   instincts fails it unless the caster is in a feral form (cat /
--   bear / direbear, Unit.cpp:9164-9168 via GetShapeshiftForm).
-- STAY-OUT (tick 294, updated this tick): the remaining
-- spell_druid classes — CalcAmount/hooks absent, or SpellScript-only
-- with no RegisterSpellScript in the engine. Notable: nurturing_instinct
-- (GetRank), living_seed (proc), wild_growth (proc + CalcAmount),
-- dash (no ev11 re-fire on shapeshift change), idol_lifebloom (no
-- CalcSpellMod event), nourish (no GetHitHeal/SetHitHeal),
-- starfall_dummy (no GetTriggerSpell on the player spell builder),
-- swift_flight_passive (no Has310Flyer bridge), typhoon
-- (PreventHitDefaultEffect unhonored on the player path),
-- wild_growth_aura (no OnEffectUpdatePeriodic event). The 2026-11-20
-- remaining-classes audit (hidden_files/
-- spell_druid_remaining_classes_2026-11-20.md) reconciles all 53
-- classes: 13 ALREADY-PORTED / 1 GO-COVERED (predatory_strikes,
-- shapeshift.go AP fold) / 31 PORT / 8 STAY-OUT. This tick ports the
-- 8 no-DBC-dependency PORTs (bear_form_passive, berserk, innervate,
-- insect_swarm, rip, starfall_aoe, tiger_s_fury,
-- t10_restoration_4p_bonus) plus the lifebloom mana/dispel legs and
-- the frenzied-regeneration single-truncation fix; the other 23 PORTs
-- are dormant ev6/proc-entry classes, still unported.

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
local SPELL_DRUID_LIFEBLOOM_R2 = 48450
local SPELL_DRUID_LIFEBLOOM_R3 = 48451
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
    local regen = math.floor(target:GetMaxHealth() * (calc * mod / 100) / 100)
    target:CastCustomSpell(nil, SPELL_DRUID_FRENZIED_REGENERATION_HEAL, true, regen)
    target:SetPower(rage - mod, POWER_RAGE)
end

RegisterAuraEvent(SPELL_DRUID_FRENZIED_REGENERATION, 1, frenziedRegenerationTick)

local SPELL_DRUID_LIFEBLOOM_ENERGIZE = 64372 -- spell_druid.cpp:59

local function lifebloomManaLeg(aura, stack)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local manaPct = aura:GetSpellManaCostPercentage()
    if manaPct == nil or manaPct <= 0 then
        return
    end
    local mana = math.floor(math.floor(caster:GetCreateMana() * manaPct / 100) * stack / 2)
    caster:CastCustomSpell(caster, SPELL_DRUID_LIFEBLOOM_ENERGIZE, true, mana)
end

local function lifebloomRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local heal = aura:GetAmount(1)
    lifebloomManaLeg(aura, aura:GetStackAmount())
    target:CastCustomSpell(target, SPELL_DRUID_LIFEBLOOM_FINAL_HEAL, true, heal)
end

RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM, 3, lifebloomRemove)
RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM_R2, 3, lifebloomRemove)
RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM_R3, 3, lifebloomRemove)

-- C++ spell_dru_lifebloom::HandleDispel (spell_druid.cpp:867-875):
-- the dispel leg runs OnRemoveEffect with stack = GetRemovedCharges()
-- (1 per successful dispel, dispel.go:1036-1041): same final heal
-- (EFFECT_1 amount) and mana energize (stack 1) as the expire leg.
-- RegisterAuraEvent OnDispel = 5 (fireAuraDispel, dispel.go:1724).
local function lifebloomDispel(event, aura, target, dispeller)
    local heal = aura:GetAmount(1)
    lifebloomManaLeg(aura, 1)
    target:CastCustomSpell(target, SPELL_DRUID_LIFEBLOOM_FINAL_HEAL, true, heal)
end

RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM, 5, lifebloomDispel)
RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM_R2, 5, lifebloomDispel)
RegisterAuraEvent(SPELL_DRUID_LIFEBLOOM_R3, 5, lifebloomDispel)

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
local SPELL_FAILED_ONLY_SHAPESHIFT = 94 -- SharedDefines.h:1076
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

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

-- C++ spell_dru_savage_roar::CheckCast (spell_druid.cpp:1281-1288):
-- a caster not in cat form fails SPELL_FAILED_ONLY_SHAPESHIFT.
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
-- Binding 52610 is the AC spell_script_names row; 62071 is the buff
-- the aura arm casts, not a CheckCast binding.
local function savageRoarCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and caster:GetShapeshiftForm() ~= FORM_CAT then
        return SPELL_FAILED_ONLY_SHAPESHIFT
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_DRUID_SAVAGE_ROAR, 8, savageRoarCheckCast)

-- C++ spell_dru_survival_instincts::CheckCast (spell_druid.cpp:1383-1386):
-- a caster not in feral form fails SPELL_FAILED_ONLY_SHAPESHIFT.
-- IsInFeralForm (Unit.cpp:9164-9168) is form == CAT/BEAR/DIREBEAR,
-- expressed via the bridged GetShapeshiftForm (lua_player.go:196).
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
-- Binding 61336 is the AC spell_script_names row; 50322 is the buff
-- the aura arm casts, not a CheckCast binding.
local function survivalInstinctsCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil then
        local form = caster:GetShapeshiftForm()
        if form ~= FORM_CAT and form ~= FORM_BEAR and form ~= FORM_DIREBEAR then
            return SPELL_FAILED_ONLY_SHAPESHIFT
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_DRUID_SURVIVAL_INSTINCTS, 8, survivalInstinctsCheckCast)

local SPELL_DRUID_FLIGHT_FORM = 33943 -- spell_druid.cpp:423 (class spell_dru_flight_form)
local SPELL_DRUID_SWIFT_FLIGHT_FORM = 40120 -- spell_ranks rank 2 of 33943
local SPELL_FAILED_NOT_SHAPESHIFT = 68 -- SharedDefines.h:1050

-- C++ spell_dru_flight_form::CheckCast (spell_druid.cpp:427-434): a
-- caster in a disallowed mount form fails SPELL_FAILED_NOT_SHAPESHIFT.
-- IsInDisallowedMountForm (Unit.cpp:9170-9185) rides the new
-- player:IsInDisallowedMountForm bridge (spells.go, SpellShapeshiftForm
-- flags lacking 0x1 or a missing form row; transform-spell carve-out
-- and display-ID arms unbridged/vacuous per that helper).
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
-- Binding is the -33943 TDB spell_script_names row (all ranks), so both
-- 33943 and its rank-2 40120 register here; no AC spell_script_names row
-- exists for 33943/-33943/40120 (grep count 0).
local function flightFormCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and caster:IsInDisallowedMountForm() then
        return SPELL_FAILED_NOT_SHAPESHIFT
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_DRUID_FLIGHT_FORM, 8, flightFormCheckCast)
RegisterSpellEvent(SPELL_DRUID_SWIFT_FLIGHT_FORM, 8, flightFormCheckCast)

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

-- PORTED (this tick, 2026-11-20 remaining-classes audit):
--   spell_dru_bear_form_passive (spell_druid.cpp:131-170): auras
--   1178 / 9635, mode 11 (DoEffectCalcAmount, EFFECT_0,
--   SPELL_AURA_MOD_BASE_RESISTANCE_PCT). While the owner has Enrage
--   (5229) and not the T10 Feral 4P aura (70726), amount += -27
--   (1178) / -16 (9635). GetAmount(0) at ev11 time is the rolled
--   DBC base (spells.go apply leg), i.e. C++'s amount parameter.
--   Partial: no dynamic recalc when Enrage lands/leaves (no
--   RecalculateAmount bridge — same gap blocks dash and enrage's
--   RecalculateBaseArmor leg).
local SPELL_DRUID_BEAR_FORM_PASSIVE = 1178 -- spell_druid.cpp:40
local SPELL_DRUID_DIRE_BEAR_FORM_PASSIVE = 9635 -- spell_druid.cpp:41

local function bearFormPassiveCalcAmount(event, aura, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    if not target:HasAura(SPELL_DRUID_ENRAGE) or target:HasAura(SPELL_DRUID_ITEM_T10_FERAL_4P_BONUS) then
        return
    end
    local mod = 0
    if aura:GetAuraId() == SPELL_DRUID_BEAR_FORM_PASSIVE then
        mod = -27
    elseif aura:GetAuraId() == SPELL_DRUID_DIRE_BEAR_FORM_PASSIVE then
        mod = -16
    else
        return
    end
    aura:SetAmount(aura:GetAmount(0) + mod, 0)
end

RegisterAuraEvent(SPELL_DRUID_BEAR_FORM_PASSIVE, 11, bearFormPassiveCalcAmount)
RegisterAuraEvent(SPELL_DRUID_DIRE_BEAR_FORM_PASSIVE, 11, bearFormPassiveCalcAmount)

--   spell_dru_berserk (spell_druid.cpp:171-191): aura 50334, mode 2
--   (AfterEffectApply, EFFECT_1, SPELL_AURA_ADD_FLAT_MODIFIER).
--   On apply, reset the cooldowns of every spell in category 971
--   (SPELL_CATEGORY_MANGLE_BEAR, spell_druid.cpp:105), update sent.
--   ResetTypeCooldowns is the PlayerMethods.h:3519 twin
--   (lua_player.go:2138); the warrior Shield Slam precedent
--   (spell_warrior.lua:394) calls it the same way.
local SPELL_DRUID_BERSERK = 50334 -- spell_druid.cpp:171
local SPELL_CATEGORY_MANGLE_BEAR = 971 -- spell_druid.cpp:105

local function berserkApply(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    target:ResetTypeCooldowns(SPELL_CATEGORY_MANGLE_BEAR, true)
end

RegisterAuraEvent(SPELL_DRUID_BERSERK, 2, berserkApply)

--   spell_dru_innervate (spell_druid.cpp:731-754): aura 29166,
--   mode 11 (DoEffectCalcAmount, EFFECT_0,
--   SPELL_AURA_PERIODIC_ENERGIZE). With 0 total ticks the amount
--   is 0; otherwise amount = floor(floor(caster GetCreateMana() *
--   base / 100) / ticks) (CalculatePct truncates once, then integer
--   division by ticks truncates). No caster -> 0, as C++.
--   GetCreateMana is the player-surface Unit::GetCreateMana
--   (lua_player.go:1403); luaUnitObjectForGUID returns the player
--   object for a player caster (pet_combat.go:479).
local SPELL_DRUID_INNERVATE = 29166 -- spell_druid.cpp:731

local function innervateCalcAmount(event, aura, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local ticks = aura:GetTotalTicks()
    if ticks == 0 then
        aura:SetAmount(0, 0)
        return
    end
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        aura:SetAmount(0, 0)
        return
    end
    local base = aura:GetAmount(0)
    aura:SetAmount(math.floor(math.floor(caster:GetCreateMana() * base / 100) / ticks), 0)
end

RegisterAuraEvent(SPELL_DRUID_INNERVATE, 11, innervateCalcAmount)

--   spell_dru_insect_swarm (spell_druid.cpp:756-775): -5570, mode
--   11 (DoEffectCalcAmount, EFFECT_0, SPELL_AURA_PERIODIC_DAMAGE).
--   With 0 total ticks the amount is unchanged; otherwise the
--   caster's T8 balance relic (64950) EFFECT_0 amount, when
--   present, adds floor(relic / ticks). TDB spell_ranks chain
--   5570/24974/24975/24976/24977/27013/48468.
local SPELL_DRUID_INSECT_SWARM = 5570 -- spell_druid.cpp:756
local SPELL_DRUID_INSECT_SWARM_RANKS = { 5570, 24974, 24975, 24976, 24977, 27013, 48468 }
local SPELL_DRUID_ITEM_T8_BALANCE_RELIC = 64950 -- spell_druid.cpp:56

local function insectSwarmCalcAmount(event, aura, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local ticks = aura:GetTotalTicks()
    if ticks == 0 then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local relic = caster:GetAuraEffect(SPELL_DRUID_ITEM_T8_BALANCE_RELIC, 0)
    if relic ~= nil then
        aura:SetAmount(aura:GetAmount(0) + math.floor(relic / ticks), 0)
    end
end

for _, spellId in ipairs(SPELL_DRUID_INSECT_SWARM_RANKS) do
    RegisterAuraEvent(spellId, 11, insectSwarmCalcAmount)
end

--   spell_dru_rip (spell_druid.cpp:1195-1250): -1079, mode 11
--   (DoEffectCalcAmount, EFFECT_0, SPELL_AURA_PERIODIC_DAMAGE).
--   Player-caster leg only (Load() admits players and NPCBot
--   creatures; the bot leg stays out — no IsNPCBot bridge, no
--   creature GetComboPoints): amount += cp * idol + floor(AP * cp /
--   100), where cp is the caster's combo points, idol is the
--   Idol of Feral Shadows (34241) EFFECT_0 amount else the Idol of
--   Worship (60774) EFFECT_0 amount (the C++ if/else-if), and AP is
--   GetTotalAttackPowerValue(BASE_ATTACK). The creature-target ev11
--   fire site is spells.go:41904 (applyAuraToTarget). TDB
--   spell_ranks chain 1079/9492/9493/9752/9894/9896/27008/49799/
--   49800.
local SPELL_DRUID_RIP_RANKS = { 1079, 9492, 9493, 9752, 9894, 9896, 27008, 49799, 49800 }
local SPELL_DRUID_IDOL_OF_FERAL_SHADOWS = 34241 -- spell_druid.cpp:53
local SPELL_DRUID_IDOL_OF_WORSHIP = 60774 -- spell_druid.cpp:54

local function ripCalcAmount(event, aura, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local cp = caster:GetComboPoints()
    local idol = caster:GetAuraEffect(SPELL_DRUID_IDOL_OF_FERAL_SHADOWS, 0) or caster:GetAuraEffect(SPELL_DRUID_IDOL_OF_WORSHIP, 0) or 0
    aura:SetAmount(aura:GetAmount(0) + cp * idol + math.floor(caster:GetTotalAttackPowerValue() * cp / 100), 0)
end

for _, spellId in ipairs(SPELL_DRUID_RIP_RANKS) do
    RegisterAuraEvent(spellId, 11, ripCalcAmount)
end

--   spell_dru_starfall_aoe (spell_druid.cpp:1324-1338): -50294,
--   mode 7 (OnObjectAreaTargetSelect, EFFECT_0,
--   TARGET_UNIT_DEST_AREA_ENEMY). The explicit target unit leaves
--   the list. spell:GetTarget() is the cast packet's unit target
--   (lua_spell.go:354), the C++ GetExplTargetUnit() twin. TDB
--   spell_ranks chain 50294/53188/53189/53190.
local SPELL_DRUID_STARFALL_RANKS = { 50294, 53188, 53189, 53190 }

local function starfallAoeTargets(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local expl = spell:GetTarget()
    if expl ~= nil then
        targets:Remove(expl)
    end
end

for _, spellId in ipairs(SPELL_DRUID_STARFALL_RANKS) do
    RegisterSpellEvent(spellId, 7, starfallAoeTargets)
end

--   spell_dru_tiger_s_fury (spell_druid.cpp:1447-1466): -5217,
--   mode 3 (AfterHit). When the hit unit carries King of the
--   Jungle (48492) EFFECT_1, it self-casts the energize 51178 with
--   BP0 = that amount, triggered (TRIGGERED_FULL_MASK). TDB
--   spell_ranks chain 5217/6793/9845/9846/50212/50213.
local SPELL_DRUID_TIGER_S_FURY_RANKS = { 5217, 6793, 9845, 9846, 50212, 50213 }
local SPELL_DRUID_TIGER_S_FURY_ENERGIZE = 51178 -- spell_druid.cpp:68

local function tigerSFuryAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    local amt = hitTarget:GetAuraEffectOfRankedSpell(SPELL_DRUID_KING_OF_THE_JUNGLE, 1)
    if amt ~= nil then
        hitTarget:CastCustomSpell(hitTarget, SPELL_DRUID_TIGER_S_FURY_ENERGIZE, true, amt)
    end
end

for _, spellId in ipairs(SPELL_DRUID_TIGER_S_FURY_RANKS) do
    RegisterSpellEvent(spellId, 3, tigerSFuryAfterHit)
end

--   spell_dru_t10_restoration_4p_bonus (spell_druid.cpp:1768-1844):
--   spell 70691, mode 7 (OnObjectAreaTargetSelect, EFFECT_0,
--   TARGET_UNIT_DEST_AREA_ALLY). Load gate is caster TYPEID_PLAYER
--   (checked in the handler; Lua has no Load hook). Groupless: the
--   caster alone. Grouped: drop the explicit target, keep only
--   players in the caster's raid, pick one at random; an empty
--   list clears the targets (FinishCast DONT_REPORT has no mid-cast
--   model — a no-op here). The NPCBot branch stays out.
local SPELL_DRUID_T10_RESTORATION_4P_BONUS = 70691 -- spell_druid.cpp:1768
local TYPEID_PLAYER = 4 -- SharedDefines.h: TypeID

local function t10Restoration4pBonusTargets(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if caster:GetGroup() == nil then
        targets:Clear()
        targets:Add(caster)
        return
    end
    local expl = spell:GetTarget()
    if expl ~= nil then
        targets:Remove(expl)
    end
    local kept = {}
    for _, unit in ipairs(targets:GetTargets()) do
        if unit ~= nil and unit:GetTypeId() == TYPEID_PLAYER and caster:IsInRaidWith(unit) then
            table.insert(kept, unit)
        end
    end
    if #kept == 0 then
        targets:Clear()
        return
    end
    local pick = kept[math.random(#kept)]
    targets:Clear()
    targets:Add(pick)
end

RegisterSpellEvent(SPELL_DRUID_T10_RESTORATION_4P_BONUS, 7, t10Restoration4pBonusTargets)

--   spell_dru_glyph_of_barkskin (spell_druid.cpp:478-499): aura
--   63057, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the proc actor casts 63058 on self,
--   triggered. DORMANT until Spell.dbc ships the 63057 proc entry.
local SPELL_DRUID_GLYPH_OF_BARKSKIN = 63057 -- spell_druid.cpp:478

local function glyphOfBarkskinProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil then
        return true
    end
    actor:CastSpell(nil, SPELL_DRUID_BARKSKIN_01, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_GLYPH_OF_BARKSKIN, 6, glyphOfBarkskinProc)

--   spell_dru_glyph_of_innervate (spell_druid.cpp:500-531): aura
--   54832, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the actor casts the regen 54833 on self
--   with BP0 = floor(floor(create mana * amount / 100) / ticks)
--   (CalculatePct then integer division by GetMaxTicks; a 0 tick
--   count never fires the cast). DORMANT until Spell.dbc ships the
--   54832 proc entry and the 54833 row.
local SPELL_DRUID_GLYPH_OF_INNERVATE = 54832 -- spell_druid.cpp:500
local SPELL_DRUID_GLYPH_OF_INNERVATE_REGEN = 54833 -- spell_druid.cpp:77

local function glyphOfInnervateProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or actor:GetObjectType() ~= "Player" then
        return true
    end
    local ticks = aura:GetSpellMaxTicks(SPELL_DRUID_GLYPH_OF_INNERVATE_REGEN)
    if ticks == 0 then
        return true
    end
    local amount = math.floor(math.floor(actor:GetCreateMana() * aura:GetAmount(0) / 100) / ticks)
    actor:CastCustomSpell(nil, SPELL_DRUID_GLYPH_OF_INNERVATE_REGEN, true, amount)
    return true
end

RegisterAuraEvent(SPELL_DRUID_GLYPH_OF_INNERVATE, 6, glyphOfInnervateProc)

--   spell_dru_glyph_of_rake (spell_druid.cpp:532-559): aura 54821,
--   mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY). CheckProc
--   admits only TYPEID_UNIT proc targets; PreventDefaultAction;
--   the actor casts 54820 on the proc target, triggered. DORMANT
--   until Spell.dbc ships the 54821 proc entry.
local SPELL_DRUID_GLYPH_OF_RAKE = 54821 -- spell_druid.cpp:532
local SPELL_DRUID_GLYPH_OF_RAKE_TRIGGERED = 54820 -- spell_druid.cpp:81
local TYPEID_UNIT = 3 -- SharedDefines.h: TypeID

local function glyphOfRakeProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil or procTarget:GetTypeId() ~= TYPEID_UNIT then
        return true
    end
    actor:CastSpell(procTarget, SPELL_DRUID_GLYPH_OF_RAKE_TRIGGERED, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_GLYPH_OF_RAKE, 6, glyphOfRakeProc)

--   spell_dru_glyph_of_rejuvenation (spell_druid.cpp:560-591):
--   aura 54754, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   CheckProc admits only proc targets below 50% health;
--   PreventDefaultAction; a nil/0 heal casts nothing, else the
--   actor casts 54755 on the proc target with BP0 =
--   floor(heal * amount / 100). DORMANT until Spell.dbc ships the
--   54754 proc entry.
local SPELL_DRUID_GLYPH_OF_REJUVENATION = 54754 -- spell_druid.cpp:560
local SPELL_DRUID_GLYPH_OF_REJUVENATION_HEAL = 54755 -- spell_druid.cpp:86

local function glyphOfRejuvenationProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil or not procTarget:HealthBelowPct(50) then
        return true
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    actor:CastCustomSpell(procTarget, SPELL_DRUID_GLYPH_OF_REJUVENATION_HEAL, true, math.floor(heal * aura:GetAmount(0) / 100))
    return true
end

RegisterAuraEvent(SPELL_DRUID_GLYPH_OF_REJUVENATION, 6, glyphOfRejuvenationProc)

--   spell_dru_glyph_of_starfire_dummy (spell_druid.cpp:684-705):
--   aura 54845, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the actor casts 54846 on the proc
--   target, triggered. DORMANT until Spell.dbc ships the 54845
--   proc entry.
local SPELL_DRUID_GLYPH_OF_STARFIRE_DUMMY = 54845 -- spell_druid.cpp:684
local SPELL_DRUID_GLYPH_OF_STARFIRE_SCRIPT = 54846 -- spell_druid.cpp:78

local function glyphOfStarfireDummyProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil then
        return true
    end
    actor:CastSpell(procTarget, SPELL_DRUID_GLYPH_OF_STARFIRE_SCRIPT, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_GLYPH_OF_STARFIRE_DUMMY, 6, glyphOfStarfireDummyProc)

--   spell_dru_living_seed (spell_druid.cpp:884-911): aura 48496,
--   mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; a nil/0 heal casts nothing, else the aura
--   owner casts 48504 on the proc target with BP0 =
--   floor(heal * amount / 100). DORMANT until Spell.dbc ships the
--   48496 proc entry.
local SPELL_DRUID_LIVING_SEED = 48496 -- spell_druid.cpp:884
local SPELL_DRUID_LIVING_SEED_PROC = 48504 -- spell_druid.cpp:62

local function livingSeedProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if target == nil or procTarget == nil then
        return true
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    target:CastCustomSpell(procTarget, SPELL_DRUID_LIVING_SEED_PROC, true, math.floor(heal * aura:GetAmount(0) / 100))
    return true
end

RegisterAuraEvent(SPELL_DRUID_LIVING_SEED, 6, livingSeedProc)

--   spell_dru_living_seed_proc (spell_druid.cpp:913-935): aura
--   48504, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the aura owner self-casts 48503 with
--   BP0 = the aura's EFFECT_0 amount. DORMANT until Spell.dbc ships
--   the 48504 proc entry.
local SPELL_DRUID_LIVING_SEED_HEAL = 48503 -- spell_druid.cpp:61

local function livingSeedHealProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if target == nil then
        return true
    end
    target:CastCustomSpell(target, SPELL_DRUID_LIVING_SEED_HEAL, true, aura:GetAmount(0))
    return true
end

RegisterAuraEvent(SPELL_DRUID_LIVING_SEED_PROC, 6, livingSeedHealProc)

--   spell_dru_omen_of_clarity (spell_druid.cpp:1041-1066): aura
--   16864, mode 6 (OnEffectProc, EFFECT_0,
--   SPELL_AURA_PROC_TRIGGER_SPELL). NO PreventDefaultAction in
--   C++: when the owner has the T10 balance 4P aura (70718) it
--   also casts 70721 on self, triggered, and the default trigger
--   leg still runs (the handler returns nil). DORMANT until
--   Spell.dbc ships the 16864 proc entry.
local SPELL_DRUID_OMEN_OF_CLARITY = 16864 -- spell_druid.cpp:1041
local SPELL_DRUID_BALANCE_T10_BONUS = 70718 -- spell_druid.cpp:93
local SPELL_DRUID_BALANCE_T10_BONUS_PROC = 70721 -- spell_druid.cpp:94

local function omenOfClarityProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if target ~= nil and target:HasAura(SPELL_DRUID_BALANCE_T10_BONUS) then
        target:CastSpell(nil, SPELL_DRUID_BALANCE_T10_BONUS_PROC, true)
    end
end

RegisterAuraEvent(SPELL_DRUID_OMEN_OF_CLARITY, 6, omenOfClarityProc)

--   spell_dru_owlkin_frenzy (spell_druid.cpp:1068-1081): aura
--   48391, mode 11 (DoEffectCalcAmount, EFFECT_2,
--   SPELL_AURA_PERIODIC_ENERGIZE). amount = floor(owner create
--   mana * base / 100) (CalculatePct). DORMANT until Spell.dbc
--   ships the 48391 row.
local SPELL_DRUID_OWLKIN_FRENZY = 48391 -- spell_druid.cpp:1068

local function owlkinFrenzyCalcAmount(event, aura, target, effectIndex)
    if effectIndex ~= 2 then
        return
    end
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    aura:SetAmount(math.floor(target:GetCreateMana() * aura:GetAmount(2) / 100), 2)
end

RegisterAuraEvent(SPELL_DRUID_OWLKIN_FRENZY, 11, owlkinFrenzyCalcAmount)

--   spell_dru_revitalize (spell_druid.cpp:1143-1193): aura 48539,
--   mode 6 (OnEffectProc, EFFECT_0,
--   SPELL_AURA_OVERRIDE_CLASS_SCRIPTS). PreventDefaultAction; the
--   roll is roll_chance_i(EFFECT_0 amount); the proc target's
--   power type selects the energize (mana 48542, rage 48541,
--   energy 48540, runic power 48543), cast by the actor on the
--   proc target, triggered. DORMANT until Spell.dbc ships the
--   48539 proc entry.
local SPELL_DRUID_REVITALIZE = 48539 -- spell_druid.cpp:1143
local SPELL_DRUID_REVITALIZE_ENERGIZE_MANA = 48542 -- spell_druid.cpp:73
local SPELL_DRUID_REVITALIZE_ENERGIZE_RAGE = 48541 -- spell_druid.cpp:74
local SPELL_DRUID_REVITALIZE_ENERGIZE_ENERGY = 48540 -- spell_druid.cpp:75
local SPELL_DRUID_REVITALIZE_ENERGIZE_RP = 48543 -- spell_druid.cpp:76
local POWER_MANA = 0 -- SharedDefines.h: Powers
local POWER_ENERGY = 3 -- SharedDefines.h: Powers
local POWER_RUNIC_POWER = 6 -- SharedDefines.h: Powers

local REVITALIZE_SPELLS = {
    [POWER_MANA] = SPELL_DRUID_REVITALIZE_ENERGIZE_MANA,
    [POWER_RAGE] = SPELL_DRUID_REVITALIZE_ENERGIZE_RAGE,
    [POWER_ENERGY] = SPELL_DRUID_REVITALIZE_ENERGIZE_ENERGY,
    [POWER_RUNIC_POWER] = SPELL_DRUID_REVITALIZE_ENERGIZE_RP,
}

local function revitalizeProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil then
        return true
    end
    if math.random(0, 99) >= aura:GetAmount(0) then
        return true
    end
    local spellId = REVITALIZE_SPELLS[procTarget:GetPowerType()]
    if spellId == nil then
        return true
    end
    actor:CastSpell(procTarget, spellId, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_REVITALIZE, 6, revitalizeProc)

--   spell_dru_t3_2p_bonus (spell_druid.cpp:1486-1537): aura 28716,
--   mode 6 (OnEffectProc, EFFECT_0,
--   SPELL_AURA_OVERRIDE_CLASS_SCRIPTS). CheckProc is the 50%
--   roll; PreventDefaultAction; the proc target's power type
--   selects the energize (mana 28722, rage 28723, energy 28724),
--   cast by the actor on the proc target, triggered. DORMANT until
--   Spell.dbc ships the 28716 proc entry.
local SPELL_DRUID_T3_2P_BONUS = 28716 -- spell_druid.cpp:1486
local SPELL_DRUID_T3_PROC_ENERGIZE_MANA = 28722 -- spell_druid.cpp:69
local SPELL_DRUID_T3_PROC_ENERGIZE_RAGE = 28723 -- spell_druid.cpp:70
local SPELL_DRUID_T3_PROC_ENERGIZE_ENERGY = 28724 -- spell_druid.cpp:71

local T3_2P_SPELLS = {
    [POWER_MANA] = SPELL_DRUID_T3_PROC_ENERGIZE_MANA,
    [POWER_RAGE] = SPELL_DRUID_T3_PROC_ENERGIZE_RAGE,
    [POWER_ENERGY] = SPELL_DRUID_T3_PROC_ENERGIZE_ENERGY,
}

local function t32pBonusProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil then
        return true
    end
    if math.random(0, 99) >= 50 then
        return true
    end
    local spellId = T3_2P_SPELLS[procTarget:GetPowerType()]
    if spellId == nil then
        return true
    end
    actor:CastSpell(procTarget, spellId, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_T3_2P_BONUS, 6, t32pBonusProc)

--   spell_dru_t3_6p_bonus (spell_druid.cpp:1539-1560): aura 28744,
--   mode 6 (OnEffectProc, EFFECT_0,
--   SPELL_AURA_OVERRIDE_CLASS_SCRIPTS). PreventDefaultAction; the
--   actor casts 28750 on the proc target, triggered. DORMANT until
--   Spell.dbc ships the 28744 proc entry.
local SPELL_DRUID_T3_6P_BONUS = 28744 -- spell_druid.cpp:1539
local SPELL_DRUID_BLESSING_OF_THE_CLAW = 28750 -- spell_druid.cpp:72

local function t36pBonusProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil then
        return true
    end
    actor:CastSpell(procTarget, SPELL_DRUID_BLESSING_OF_THE_CLAW, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_T3_6P_BONUS, 6, t36pBonusProc)

--   spell_dru_t3_8p_bonus (spell_druid.cpp:1561-1590): aura 28719,
--   mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; no proc spell info casts nothing; the
--   actor casts 28742 on self with BP0 = floor(power cost of the
--   proc spell * amount / 100) (a 0 cost casts nothing, the
--   GetSpellPowerCost convention). DORMANT until Spell.dbc ships
--   the 28719 proc entry.
local SPELL_DRUID_T3_8P_BONUS = 28719 -- spell_druid.cpp:1561
local SPELL_DRUID_EXHILARATE = 28742 -- spell_druid.cpp:85

local function t38pBonusProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or actor:GetObjectType() ~= "Player" then
        return true
    end
    local spellId = procEvent:GetSpellId()
    if spellId == nil or spellId == 0 then
        return true
    end
    local cost = actor:GetSpellPowerCost(spellId)
    if cost == nil or cost == 0 then
        return true
    end
    actor:CastCustomSpell(nil, SPELL_DRUID_EXHILARATE, true, math.floor(cost * aura:GetAmount(0) / 100))
    return true
end

RegisterAuraEvent(SPELL_DRUID_T3_8P_BONUS, 6, t38pBonusProc)

--   spell_dru_t4_2p_bonus (spell_druid.cpp:1592-1612): auras 37288
--   / 37295, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the actor casts 37238 on self,
--   triggered. DORMANT until Spell.dbc ships the 37288/37295 proc
--   entries.
local SPELL_DRUID_T4_2P_BONUS_IDS = { 37288, 37295 }
local SPELL_DRUID_INFUSION = 37238 -- spell_druid.cpp:87

local function t42pBonusProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil then
        return true
    end
    actor:CastSpell(nil, SPELL_DRUID_INFUSION, true)
    return true
end

for _, spellId in ipairs(SPELL_DRUID_T4_2P_BONUS_IDS) do
    RegisterAuraEvent(spellId, 6, t42pBonusProc)
end

--   spell_dru_t9_feral_relic (spell_druid.cpp:1669-1728): aura
--   67353, mode 6 (OnEffectProc, EFFECT_0,
--   SPELL_AURA_PROC_TRIGGER_SPELL). CheckProc admits only bear /
--   dire bear / cat form; PreventDefaultAction; the actor
--   self-casts 67354 (bear/dire bear) or 67355 (cat), triggered.
--   DORMANT until Spell.dbc ships the 67353 proc entry.
local SPELL_DRUID_T9_FERAL_RELIC = 67353 -- spell_druid.cpp:1669
local SPELL_DRUID_T9_FERAL_RELIC_BEAR = 67354 -- spell_druid.cpp:66
local SPELL_DRUID_T9_FERAL_RELIC_CAT = 67355 -- spell_druid.cpp:67

local function t9FeralRelicProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil then
        return true
    end
    local form = actor:GetShapeshiftForm()
    local spellId = nil
    if form == FORM_BEAR or form == FORM_DIREBEAR then
        spellId = SPELL_DRUID_T9_FERAL_RELIC_BEAR
    elseif form == FORM_CAT then
        spellId = SPELL_DRUID_T9_FERAL_RELIC_CAT
    end
    if spellId == nil then
        return true
    end
    actor:CastSpell(actor, spellId, true)
    return true
end

RegisterAuraEvent(SPELL_DRUID_T9_FERAL_RELIC, 6, t9FeralRelicProc)

--   spell_dru_t10_balance_4p_bonus (spell_druid.cpp:1730-1766):
--   aura 70723, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; a nil/0 damage casts nothing; the actor
--   casts 71023 on the proc target with BP0 =
--   floor(floor(damage * amount / 100) / ticks) (CalculatePct then
--   integer division by GetMaxTicks; a 0 tick count never fires
--   the cast). DORMANT until Spell.dbc ships the 70723 proc entry
--   and the 71023 row.
local SPELL_DRUID_T10_BALANCE_4P_BONUS = 70723 -- spell_druid.cpp:1730
local SPELL_DRUID_LANGUISH = 71023 -- spell_druid.cpp:91

local function t10Balance4pBonusProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil then
        return true
    end
    local dmg = procEvent:GetDamage()
    if dmg == nil or dmg == 0 then
        return true
    end
    local ticks = aura:GetSpellMaxTicks(SPELL_DRUID_LANGUISH)
    if ticks == 0 then
        return true
    end
    local amount = math.floor(math.floor(dmg * aura:GetAmount(0) / 100) / ticks)
    actor:CastCustomSpell(procTarget, SPELL_DRUID_LANGUISH, true, amount)
    return true
end

RegisterAuraEvent(SPELL_DRUID_T10_BALANCE_4P_BONUS, 6, t10Balance4pBonusProc)

--   spell_dru_eclipse (spell_druid.cpp:210-284): aura 48516, mode
--   6 (OnEffectProc, EFFECT_0/1, SPELL_AURA_DUMMY). CheckProc
--   vetoes the whole proc while the actor carries a solar (48517)
--   or lunar (48518) proc aura; CheckSolar admits only Starfire
--   (family flags[0] & 0x4) and CheckLunar only Wrath (flags[0] &
--   0x1, 60% roll), each on its own 30 s cooldown kept in the
--   aura's script store against GetGameTimeMS (an unset key reads
--   0, the C++ time_point::min twin). PreventDefaultAction; the
--   actor casts 48517 / 48518 on self, triggered. A false return
--   on the non-matching effect falls through to that effect's
--   default leg (a DUMMY no-op) and does not block the other
--   effect's fire (procs.go fires ev6 once per eligible effect).
--   DORMANT until Spell.dbc ships the 48516 proc entry.
local SPELL_DRUID_ECLIPSE = 48516 -- spell_druid.cpp:210
local SPELL_DRUID_ECLIPSE_SOLAR_PROC = 48517 -- spell_druid.cpp:43
local SPELL_DRUID_ECLIPSE_LUNAR_PROC = 48518 -- spell_druid.cpp:42

local function eclipseProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if actor == nil then
        return true
    end
    if actor:HasAura(SPELL_DRUID_ECLIPSE_SOLAR_PROC) or actor:HasAura(SPELL_DRUID_ECLIPSE_LUNAR_PROC) then
        return true
    end
    local now = GetGameTimeMS()
    if effectIndex == 0 then
        if bit32.band(procEvent:GetSpellFamilyFlags(0), 0x4) == 0 then
            return false
        end
        if aura:GetScriptValue("solar") > now then
            return false
        end
        aura:SetScriptValue("solar", now + 30000)
        actor:CastSpell(actor, SPELL_DRUID_ECLIPSE_SOLAR_PROC, true)
        return true
    elseif effectIndex == 1 then
        if bit32.band(procEvent:GetSpellFamilyFlags(0), 0x1) == 0 then
            return false
        end
        if math.random(0, 99) >= 60 then
            return false
        end
        if aura:GetScriptValue("lunar") > now then
            return false
        end
        aura:SetScriptValue("lunar", now + 30000)
        actor:CastSpell(actor, SPELL_DRUID_ECLIPSE_LUNAR_PROC, true)
        return true
    end
    return false
end

RegisterAuraEvent(SPELL_DRUID_ECLIPSE, 6, eclipseProc)

--   spell_dru_glyph_of_shred (spell_druid.cpp:594-641): aura
--   54815, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the proc target's Rip (PERIODIC_DAMAGE,
--   family 7, flag 0x00800000, cast by the actor) gains
--   EFFECT_0 amount * 1000 ms of duration and max duration, while
--   its live max stays under the DBC max + 3 * amount * 1000 +
--   4000 per Glyph of Rip (54818) / T7 lacerate bonus (60141) the
--   actor carries. A missing Rip casts nothing but still vetoes.
--   DORMANT until Spell.dbc ships the 54815 proc entry; the cap
--   reads 0 (GetSpellMaxDuration) until the Rip row ships.
local SPELL_DRUID_GLYPH_OF_SHRED = 54815 -- spell_druid.cpp:594
local SPELL_DRUID_GLYPH_OF_RIP = 54818 -- spell_druid.cpp:79
local SPELL_DRUID_RIP_DURATION_LACERATE_DMG = 60141 -- spell_druid.cpp:80
local SPELL_AURA_PERIODIC_DAMAGE = 3 -- SpellAuraDefines.h:83
local SPELLFAMILY_DRUID = 7 -- SharedDefines.h:3588

local function glyphOfShredProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil or procTarget == nil then
        return true
    end
    local ripId = procTarget:GetAuraEffectSpellIdByFamily(SPELL_AURA_PERIODIC_DAMAGE, SPELLFAMILY_DRUID, 0x00800000, 0, 0, actor:GetGUID())
    if ripId == nil then
        return true
    end
    local rip = procTarget:GetAura(ripId)
    if rip == nil then
        return true
    end
    local cap = rip:GetSpellMaxDuration(ripId) + 3 * aura:GetAmount(0) * 1000
    if actor:HasAura(SPELL_DRUID_GLYPH_OF_RIP) then
        cap = cap + 4000
    end
    if actor:HasAura(SPELL_DRUID_RIP_DURATION_LACERATE_DMG) then
        cap = cap + 4000
    end
    if rip:GetMaxDuration() < cap then
        rip:SetDuration(rip:GetDuration() + aura:GetAmount(0) * 1000)
        rip:SetMaxDuration(rip:GetMaxDuration() + aura:GetAmount(0) * 1000)
    end
    return true
end

RegisterAuraEvent(SPELL_DRUID_GLYPH_OF_SHRED, 6, glyphOfShredProc)

--   spell_dru_glyph_of_starfire (spell_druid.cpp:643-682): spell
--   54846, mode 5 (OnEffectHitTarget, EFFECT_0,
--   SPELL_EFFECT_SCRIPT_EFFECT). The hit unit's Moonfire
--   (PERIODIC_DAMAGE, family 7, flag 0x00000002, cast by the
--   caster) gains 3000 ms of duration and max duration, while its
--   live max stays under the DBC max + 9000 + 3000 per Increased
--   Moonfire Duration (38414) / Nature's Splendor (57865) the
--   caster carries. DORMANT until Spell.dbc ships the 54846 row;
--   the cap reads 0 (GetSpellMaxDuration) until the Moonfire row
--   ships.
local SPELL_DRUID_INCREASED_MOONFIRE_DURATION = 38414 -- spell_druid.cpp:55
local SPELL_DRUID_NATURES_SPLENDOR = 57865 -- spell_druid.cpp:63

local function glyphOfStarfireHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local moonfireId = target:GetAuraEffectSpellIdByFamily(SPELL_AURA_PERIODIC_DAMAGE, SPELLFAMILY_DRUID, 0x00000002, 0, 0, caster:GetGUID())
    if moonfireId == nil then
        return
    end
    local moonfire = target:GetAura(moonfireId)
    if moonfire == nil then
        return
    end
    local cap = moonfire:GetSpellMaxDuration(moonfireId) + 9000
    if caster:HasAura(SPELL_DRUID_INCREASED_MOONFIRE_DURATION) then
        cap = cap + 3000
    end
    if caster:HasAura(SPELL_DRUID_NATURES_SPLENDOR) then
        cap = cap + 3000
    end
    if moonfire:GetMaxDuration() < cap then
        moonfire:SetDuration(moonfire:GetDuration() + 3000)
        moonfire:SetMaxDuration(moonfire:GetMaxDuration() + 3000)
    end
end

RegisterSpellEvent(SPELL_DRUID_GLYPH_OF_STARFIRE_SCRIPT, 5, glyphOfStarfireHit)

--   spell_dru_item_t6_trinket (spell_druid.cpp:1614-1667): aura
--   40442, mode 6 (OnEffectProc, EFFECT_0, SPELL_AURA_DUMMY).
--   PreventDefaultAction; the proc spell's family flags select
--   the blessing (Starfire flags[0] & 0x4 -> 40445 at 25%,
--   Rejuvenation flags[0] & 0x10 -> 40446 at 25%, Mangle
--   flags[1] & 0x440 -> 40452 at 40%), rolled in that order and
--   cast by the actor on self, triggered. A spell matching none
--   casts nothing but still vetoes. DORMANT until Spell.dbc
--   ships the 40442 proc entry.
local SPELL_DRUID_ITEM_T6_TRINKET = 40442 -- spell_druid.cpp:1614
local SPELL_DRUID_BLESSING_OF_REMULOS = 40445 -- spell_druid.cpp:88
local SPELL_DRUID_BLESSING_OF_ELUNE = 40446 -- spell_druid.cpp:89
local SPELL_DRUID_BLESSING_OF_CENARIUS = 40452 -- spell_druid.cpp:90

local function itemT6TrinketProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if actor == nil then
        return true
    end
    local spellId = nil
    local chance = 0
    if bit32.band(procEvent:GetSpellFamilyFlags(0), 0x4) ~= 0 then
        spellId = SPELL_DRUID_BLESSING_OF_REMULOS
        chance = 25
    elseif bit32.band(procEvent:GetSpellFamilyFlags(0), 0x10) ~= 0 then
        spellId = SPELL_DRUID_BLESSING_OF_ELUNE
        chance = 25
    elseif bit32.band(procEvent:GetSpellFamilyFlags(1), 0x440) ~= 0 then
        spellId = SPELL_DRUID_BLESSING_OF_CENARIUS
        chance = 40
    else
        return true
    end
    if math.random(0, 99) < chance then
        actor:CastSpell(nil, spellId, true)
    end
    return true
end

RegisterAuraEvent(SPELL_DRUID_ITEM_T6_TRINKET, 6, itemT6TrinketProc)

--   spell_dru_leader_of_the_pack (spell_druid.cpp:777-826): aura
--   24932, mode 6 (OnEffectProc, EFFECT_1, SPELL_AURA_DUMMY).
--   PreventDefaultAction; a 0 EFFECT_1 amount or a 34299
--   cooldown casts nothing. Otherwise the actor casts the heal
--   34299 on self with BP0 = CountPctFromMaxHealth(EFFECT_1
--   amount) and takes the 6 s cooldown on 34299; only when the
--   aura's caster is the actor itself does the Improved Leader
--   of the Pack mana leg fire (34297 rank aura, EFFECT_1
--   CalcValue of its spell, no-caster overload, as the percent
--   of max mana for 68285). DORMANT until Spell.dbc ships the
--   24932 proc entry; the mana leg is DBC-dormant on
--   GetSpellEffectCalcValue until the 34297 row ships.
local SPELL_DRUID_LEADER_OF_THE_PACK = 24932 -- spell_druid.cpp:777
local SPELL_DRUID_IMP_LEADER_OF_THE_PACK_R1 = 34297 -- spell_druid.cpp:82
local SPELL_DRUID_IMP_LEADER_OF_THE_PACK_HEAL = 34299 -- spell_druid.cpp:83
local SPELL_DRUID_IMP_LEADER_OF_THE_PACK_MANA = 68285 -- spell_druid.cpp:84

local function leaderOfThePackProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 1 then
        return false
    end
    if actor == nil or actor:GetObjectType() ~= "Player" then
        return true
    end
    if aura:GetAmount(1) == 0 or actor:HasSpellCooldown(SPELL_DRUID_IMP_LEADER_OF_THE_PACK_HEAL) then
        return true
    end
    actor:CastCustomSpell(nil, SPELL_DRUID_IMP_LEADER_OF_THE_PACK_HEAL, true, actor:CountPctFromMaxHealth(aura:GetAmount(1)))
    actor:AddSpellCooldown(SPELL_DRUID_IMP_LEADER_OF_THE_PACK_HEAL, 6000)
    if aura:GetCasterGUID() ~= actor:GetGUID() then
        return true
    end
    local imp = actor:GetAuraOfRankedSpell(SPELL_DRUID_IMP_LEADER_OF_THE_PACK_R1)
    if imp == nil then
        return true
    end
    actor:CastCustomSpell(nil, SPELL_DRUID_IMP_LEADER_OF_THE_PACK_MANA, true, math.floor(actor:GetMaxPower(POWER_MANA) * imp:GetSpellEffectCalcValue(imp:GetAuraId(), 1) / 100))
    return true
end

RegisterAuraEvent(SPELL_DRUID_LEADER_OF_THE_PACK, 6, leaderOfThePackProc)

--   spell_dru_t10_restoration_4p_bonus_dummy
--   (spell_druid.cpp:1846-1891): aura 70664, mode 6 (OnEffectProc,
--   EFFECT_0, SPELL_AURA_DUMMY). CheckProc vetoes the proc when
--   the event spell is 70691 itself, when the heal is nil/0, when
--   the actor is not a player (the NPCBot leg stays out: no
--   IsNPCBot bridge), or when the actor is ungrouped and the proc
--   target is the actor itself; otherwise PreventDefaultAction
--   and the actor casts 70691 on self with BP0 = the heal.
--   DORMANT until Spell.dbc ships the 70664 proc entry.
local SPELL_DRUID_T10_RESTORATION_4P_BONUS_DUMMY = 70664 -- spell_druid.cpp:1846

local function t10Restoration4pBonusDummyProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if procEvent:GetSpellId() == SPELL_DRUID_T10_RESTORATION_4P_BONUS then
        return true
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    if actor == nil or actor:GetObjectType() ~= "Player" then
        return true
    end
    if actor:GetGroup() == nil and procTarget ~= nil and actor:GetGUID() == procTarget:GetGUID() then
        return true
    end
    actor:CastCustomSpell(nil, SPELL_DRUID_T10_RESTORATION_4P_BONUS, true, heal)
    return true
end

RegisterAuraEvent(SPELL_DRUID_T10_RESTORATION_4P_BONUS_DUMMY, 6, t10Restoration4pBonusDummyProc)

--   spell_dru_wild_growth (spell_druid.cpp:1913-1951): -48438,
--   mode 7 (OnObjectAreaTargetSelect, EFFECT_0/1,
--   TARGET_UNIT_DEST_AREA_ALLY). EFFECT_0 drops targets not in
--   the caster's raid (IsInRaidWith, self included), keeps the
--   lowest-health-% first up to GetEffectValue(2) targets — the
--   DBC CalcValue of the dummy EFFECT_2 at the caster's level —
--   and stashes the list; EFFECT_1 restores the stashed list.
--   The stash is module-local and safe only because EFFECT_0 and
--   EFFECT_1 fire synchronously in one target-select loop; it is
--   cleared at the start of EFFECT_0, so a guard failure (no
--   caster, or a 0 effect-2 value without the 48438 row) leaves
--   EFFECT_1 a no-op and the targets unmutated. TDB spell_ranks
--   chain 48438/53248/53249/53251. DORMANT until Spell.dbc ships
--   the 48438 row.
local SPELL_DRUID_WILD_GROWTH_RANKS = { 48438, 53248, 53249, 53251 }

local wildGrowthStash = nil

local function wildGrowthTargetSelect(event, spell, effectIndex, targets)
    if effectIndex == 0 then
        wildGrowthStash = nil
        local caster = spell:GetCaster()
        if caster == nil then
            return
        end
        local maxTargets = spell:GetEffectValue(2)
        if maxTargets == 0 then
            return
        end
        local kept = {}
        for _, unit in ipairs(targets:GetTargets()) do
            if unit ~= nil and caster:IsInRaidWith(unit) then
                table.insert(kept, unit)
            end
        end
        table.sort(kept, function(a, b) return a:GetHealthPct() < b:GetHealthPct() end)
        while #kept > maxTargets do
            table.remove(kept)
        end
        targets:Clear()
        for _, unit in ipairs(kept) do
            targets:Add(unit)
        end
        wildGrowthStash = kept
    elseif effectIndex == 1 then
        if wildGrowthStash == nil then
            return
        end
        targets:Clear()
        for _, unit in ipairs(wildGrowthStash) do
            targets:Add(unit)
        end
        wildGrowthStash = nil
    end
end

for _, spellId in ipairs(SPELL_DRUID_WILD_GROWTH_RANKS) do
    RegisterSpellEvent(spellId, 7, wildGrowthTargetSelect)
end
