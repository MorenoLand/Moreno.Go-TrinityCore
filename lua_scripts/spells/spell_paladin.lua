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
-- PORTED (tick 202, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_pal_aura_mastery (31821 Aura Mastery, spell_paladin.cpp:267-294):
--   EFFECT_0 ADD_PCT_MODIFIER, REAL. OnApply the target casts the
--   immunity aura (64364) on itself, triggered; on remove the target
--   drops it. Same (spell, caster GUID) removal delta as the blessing
--   above: the immunity aura on the target was cast by the target
--   itself in the apply leg, so the unfiltered removal is the same
--   removal in practice. The 64364 area-target leg (DoCheckAreaTarget,
--   concentration-aura gate) is census row 191, not this row. Row 191
--   verdict: PORTED (Go, this tick) — the C++ registers only
--   DoCheckAreaTarget (spell_paladin.cpp:318-321); the gate lives in
--   the triggered funnel's friendly-area case (spells.go), filtering
--   to targets carrying Concentration Aura (19746) from the caster.
-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_pal_avenging_wrath (31884 Avenging Wrath,
--   spell_paladin.cpp:337-393): on apply, a Sanctified Wrath talent
--   aura (53375 EFFECT_2) makes the target cast Sanctified Wrath
--   (57318) on itself with both base points set to the talent
--   amount; the target then casts the Avenging Wrath marker (61987)
--   on itself, triggered, and gains the Immune Shield marker
--   (61988). On remove the target drops Sanctified Wrath (57318);
--   the markers are left to their own expiry (C++ removes neither).
-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_pal_blessing_of_sanctuary proc leg (:505-519): on a
--   mana-power target the proc casts Sanctuary Energize (57319) on
--   the target, triggered; the handler returns true (PreventDefault-
--   Action), including on the gate-fail path (C++ CheckProc keeps a
--   failed effect out of the proc mask entirely). DORMANT until
--   Spell.dbc ships the 20911/25899 proc entries
--   (serverSpellProcEntryFor needs ProcTypeMask).
--   DELTA: ev6 fires after the charge consume (procs.go), so a
--   gate-fail still burns a charge where C++ CheckProc would keep
--   the effect out of the proc mask entirely.
-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_pal_improved_aura (31869 Sanctified Retribution,
--   spell_paladin.cpp:1248-1310, loader spell_pal_sanctified_retribution
--   with _spellId 63531): on apply the target drops the owned
--   Sanctified Retribution aura (63531) cast by this caster, then
--   casts it on itself, triggered, so the spellmod re-applies; on
--   remove the aura is dropped unless Swift Retribution (53379) is
--   still up. Swift Retribution's own apply/remove legs are census
--   row 862 (same class, loader spell_pal_swift_retribution); the
--   remove-leg cross-check here only reads 53379. The
--   concentration/devotion registrations (63510/63514) are census rows
--   860-861, ported below via the same apply/remove legs; Swift
--   Retribution (53379) is row 862.
--   DELTA: C++ removes the aura by (spell, caster GUID); Go
--   target:RemoveAura(id) takes no caster filter (same delta as the
--   blessing above). The remove-leg presence check reads
--   GetAuraEffectOfRankedSpell(53379, 0): Swift Retribution's
--   ADD_FLAT_MODIFIER is its EFFECT_0 (the script registers
--   EFFECT_FIRST_FOUND for that aura type).
-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   624 eye_for_an_eye (9799, :777-790: holder casts 25997 on the
--   attacker, BP0 = min(pct(damage, amount), maxHealth/2));
--   625 heart_of_the_crusader (20335, :1040-1047: actor casts
--   rank-matched 21183 on the proc target);
--   628 improved_lay_of_hands (20234, :1322-1327: action target
--   casts its DBC trigger spell on itself);
--   630 judgements_of_the_just (53695, :1698-1703: holder casts
--   68055 on the action target);
--   631 judgements_of_the_wise (31876, :1730-1737: actor casts
--   31930 + Replenishment 57669 on itself);
--   633 righteous_vengeance (53380, :1944-1965: actor casts 61840
--   on the proc target, BP0 = pct(damage, amount) / max ticks).
--   All return true (PreventDefaultAction) and are DORMANT until
--   Spell.dbc ships their proc entries.
-- STAYS OUT (this tick): 623 divine_purpose (no
--   unit:RemoveAurasWithMechanic bridge; Go has the
--   auraSpellMechanicMask + mechanicStun=12 twins, no removal
--   helper); 626 holy_shock (no OnCheckCast bridge + spell-level
--   GetSpellWithRank); 627 illumination + 629 infusion_of_light +
--   636 spiritual_attunement + 637 sheath_of_light + 638 t8_2p_bonus
--   (all need heal info / proc-spell family flags / CalcPowerCost,
--   ev6 carries damage only); 632 lay_on_hands (no OnCheckCast
--   bridge); 634/635 seal_of_righteousness (needs attack-power /
--   spellpower / attack-time math, unbridged).

local POWER_MANA = 0 -- Powers::POWER_MANA (Powers.h)

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

local SPELL_PALADIN_AURA_MASTERY = 31821 -- spell_paladin.cpp:267
local SPELL_PALADIN_AURA_MASTERY_IMMUNE = 64364 -- spell_paladin.cpp:93

-- C++ spell_pal_aura_mastery_AuraScript::HandleEffectApply
-- (spell_paladin.cpp:282-285): the target casts the immunity aura
-- (64364) on itself, triggered.
local function auraMasteryApply(event, aura, target)
    target:CastSpell(target, SPELL_PALADIN_AURA_MASTERY_IMMUNE, true)
end

RegisterAuraEvent(SPELL_PALADIN_AURA_MASTERY, 2, auraMasteryApply)

-- C++ spell_pal_aura_mastery_AuraScript::HandleEffectRemove
-- (spell_paladin.cpp:287-290): the target drops the immunity aura.
local function auraMasteryRemove(event, aura, target)
    target:RemoveAura(SPELL_PALADIN_AURA_MASTERY_IMMUNE)
end

RegisterAuraEvent(SPELL_PALADIN_AURA_MASTERY, 3, auraMasteryRemove)

local SPELL_PALADIN_AVENGING_WRATH = 31884 -- spell_paladin.cpp:337
local SPELL_PALADIN_SANCTIFIED_WRATH = 57318 -- spell_paladin.cpp:81
local SPELL_PALADIN_SANCTIFIED_WRATH_TALENT_R1 = 53375 -- spell_paladin.cpp:82
local SPELL_PALADIN_AVENGING_WRATH_MARKER = 61987 -- spell_paladin.cpp:64
local SPELL_PALADIN_IMMUNE_SHIELD_MARKER = 61988 -- spell_paladin.cpp:65

-- C++ spell_pal_avenging_wrath_AuraScript::HandleApply
-- (spell_paladin.cpp:358-373): with a Sanctified Wrath talent aura
-- (53375 EFFECT_2) the target casts Sanctified Wrath (57318) on
-- itself with BP0/BP1 set to the talent amount; the target then
-- casts the Avenging Wrath marker (61987) on itself, triggered, and
-- gains the Immune Shield marker (61988).
local function avengingWrathApply(event, aura, target)
    local amount = target:GetAuraEffectOfRankedSpell(SPELL_PALADIN_SANCTIFIED_WRATH_TALENT_R1, 2)
    if amount ~= nil then
        target:CastCustomSpell(target, SPELL_PALADIN_SANCTIFIED_WRATH, true, amount, amount)
    end
    target:CastSpell(target, SPELL_PALADIN_AVENGING_WRATH_MARKER, true)
    target:AddAura(SPELL_PALADIN_IMMUNE_SHIELD_MARKER)
end

RegisterAuraEvent(SPELL_PALADIN_AVENGING_WRATH, 2, avengingWrathApply)

-- C++ spell_pal_avenging_wrath_AuraScript::HandleRemove
-- (spell_paladin.cpp:375-378): the target drops Sanctified Wrath.
local function avengingWrathRemove(event, aura, target)
    target:RemoveAura(SPELL_PALADIN_SANCTIFIED_WRATH)
end

RegisterAuraEvent(SPELL_PALADIN_AVENGING_WRATH, 3, avengingWrathRemove)

local SPELL_PALADIN_BLESSING_OF_SANCTUARY_ENERGIZE = 57319 -- spell_paladin.cpp:39

-- C++ spell_pal_blessing_of_sanctuary_AuraScript::HandleProc
-- (spell_paladin.cpp:511-519) with the CheckProc gate (:505-508):
-- on a mana-power target the proc casts Sanctuary Energize (57319)
-- on the target, triggered; the handler returns true
-- (PreventDefaultAction) on both paths (charge-consume delta in
-- the file header).
local function blessingOfSanctuaryProc(event, aura, target, actor, procTarget, damage)
    if target == nil or target:GetPowerType() ~= POWER_MANA then
        return true
    end
    target:CastSpell(target, SPELL_PALADIN_BLESSING_OF_SANCTUARY_ENERGIZE, true)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_BLESSING_OF_SANCTUARY, 6, blessingOfSanctuaryProc)
RegisterAuraEvent(SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY, 6, blessingOfSanctuaryProc)

local SPELL_PALADIN_SANCTIFIED_RETRIBUTION_R1 = 31869 -- spell_paladin.cpp:87
local SPELL_PALADIN_SANCTIFIED_RETRIBUTION_AURA = 63531 -- spell_paladin.cpp:92
local SPELL_PALADIN_SWIFT_RETRIBUTION_R1 = 53379 -- spell_paladin.cpp:88

-- C++ spell_pal_improved_aura_AuraScript::HandleEffectApply
-- (spell_paladin.cpp:1275-1280): the target drops the owned
-- Sanctified Retribution aura (63531) cast by this caster, then
-- casts it on itself, triggered, so the spellmod re-applies.
local function sanctifiedRetributionApply(event, aura, target)
    target:RemoveAura(SPELL_PALADIN_SANCTIFIED_RETRIBUTION_AURA)
    target:CastSpell(target, SPELL_PALADIN_SANCTIFIED_RETRIBUTION_AURA, true)
end

RegisterAuraEvent(SPELL_PALADIN_SANCTIFIED_RETRIBUTION_R1, 2, sanctifiedRetributionApply)

-- C++ spell_pal_improved_aura_AuraScript::HandleEffectRemove
-- (spell_paladin.cpp:1282-1289): if Swift Retribution is still up
-- the shared aura stays; otherwise the target drops it.
local function sanctifiedRetributionRemove(event, aura, target)
    if target:GetAuraEffectOfRankedSpell(SPELL_PALADIN_SWIFT_RETRIBUTION_R1, 0) ~= nil then
        return
    end
    target:RemoveAura(SPELL_PALADIN_SANCTIFIED_RETRIBUTION_AURA)
end

RegisterAuraEvent(SPELL_PALADIN_SANCTIFIED_RETRIBUTION_R1, 3, sanctifiedRetributionRemove)

local SPELL_PALADIN_IMPROVED_DEVOTION_AURA = 20138 -- spell_paladin.cpp:1245
local SPELL_PALADIN_IMPROVED_DEVOTION_EFFECT = 63514 -- spell_paladin.cpp:91
local SPELL_PALADIN_IMPROVED_CONCENTRATION_AURA = 20254 -- spell_paladin.cpp:1244
local SPELL_PALADIN_IMPROVED_CONCENTRATION_EFFECT = 63510 -- spell_paladin.cpp:90

-- C++ spell_pal_improved_aura_AuraScript::HandleEffectApply
-- (spell_paladin.cpp:1275-1280): the target drops its owned effect aura,
-- then casts it on itself, triggered, so the spellmod re-applies.
-- C++ HandleEffectRemove (spell_paladin.cpp:1282-1289): the target drops
-- the effect aura, unless the twin talent is still up (Sanctified kept
-- while Swift Retribution is present, Swift kept while Sanctified
-- Retribution is present). Devotion/concentration removes are
-- unconditional: their first-rank ids match neither cross-check branch.
-- Same (spell, caster GUID) removal delta as the Sanctified leg above.
-- Census rows 860-862 (Improved Devotion / Concentration / Swift
-- Retribution); the 31869 Sanctified leg above shares this factory's
-- shape. NOT dormant: apply/remove legs need no proc entry.
local function improvedAuraLegs(effectId, twinTalentId)
    local function apply(event, aura, target)
        target:RemoveAura(effectId)
        target:CastSpell(target, effectId, true)
    end
    local function remove(event, aura, target)
        if twinTalentId ~= nil and target:GetAuraEffectOfRankedSpell(twinTalentId, 0) ~= nil then
            return
        end
        target:RemoveAura(effectId)
    end
    return apply, remove
end

local devotionApply, devotionRemove = improvedAuraLegs(SPELL_PALADIN_IMPROVED_DEVOTION_EFFECT, nil)
RegisterAuraEvent(SPELL_PALADIN_IMPROVED_DEVOTION_AURA, 2, devotionApply)
RegisterAuraEvent(SPELL_PALADIN_IMPROVED_DEVOTION_AURA, 3, devotionRemove)

local concentrationApply, concentrationRemove = improvedAuraLegs(SPELL_PALADIN_IMPROVED_CONCENTRATION_EFFECT, nil)
RegisterAuraEvent(SPELL_PALADIN_IMPROVED_CONCENTRATION_AURA, 2, concentrationApply)
RegisterAuraEvent(SPELL_PALADIN_IMPROVED_CONCENTRATION_AURA, 3, concentrationRemove)

local swiftApply, swiftRemove = improvedAuraLegs(SPELL_PALADIN_SANCTIFIED_RETRIBUTION_AURA, SPELL_PALADIN_SANCTIFIED_RETRIBUTION_R1)
RegisterAuraEvent(SPELL_PALADIN_SWIFT_RETRIBUTION_R1, 2, swiftApply)
RegisterAuraEvent(SPELL_PALADIN_SWIFT_RETRIBUTION_R1, 3, swiftRemove)

local SPELL_PALADIN_EYE_FOR_AN_EYE = 9799 -- spell_paladin.cpp:761 (class spell_pal_eye_for_an_eye)
local SPELL_PALADIN_EYE_FOR_AN_EYE_DAMAGE = 25997 -- spell_paladin.cpp:61

-- C++ spell_pal_eye_for_an_eye_AuraScript::OnProc
-- (spell_paladin.cpp:777-790): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction (the true return); no damage -> nothing.
-- The holder casts the damage spell on the proc target (the
-- attacker), triggered, BP0 = min(pct(damage, aurEff amount),
-- holder max health / 2). DORMANT until Spell.dbc ships the 9799
-- proc entry.
local function eyeForAnEyeProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 or procTarget == nil then
        return true
    end
    local amount = math.floor(damage * aura:GetAmount() / 100)
    local cap = math.floor(target:GetMaxHealth() / 2)
    if amount > cap then
        amount = cap
    end
    target:CastCustomSpell(procTarget, SPELL_PALADIN_EYE_FOR_AN_EYE_DAMAGE, true, amount)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_EYE_FOR_AN_EYE, 6, eyeForAnEyeProc)

local SPELL_PALADIN_HEART_OF_THE_CRUSADER = 20335 -- spell_paladin.cpp:1026 (class spell_pal_heart_of_the_crusader)
local SPELL_PALADIN_HEART_OF_THE_CRUSADER_EFF_R1 = 21183 -- spell_paladin.cpp:112

-- C++ spell_pal_heart_of_the_crusader_AuraScript::HandleProc
-- (spell_paladin.cpp:1040-1047): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction; the proc actor casts the rank-matched
-- effect spell (21183 by this aura's rank) on the proc target,
-- triggered (aurEff args). DORMANT until Spell.dbc ships the
-- 20335 proc entry.
local function heartOfTheCrusaderProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil then
        return true
    end
    local spellId = aura:GetSpellWithRank(SPELL_PALADIN_HEART_OF_THE_CRUSADER_EFF_R1)
    if spellId == 0 then
        return true
    end
    actor:CastSpell(procTarget, spellId, true)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_HEART_OF_THE_CRUSADER, 6, heartOfTheCrusaderProc)

local SPELL_PALADIN_IMPROVED_LAY_ON_HANDS = 20234 -- spell_paladin.cpp:1308 (class spell_pal_improved_lay_of_hands)

-- C++ spell_pal_improved_lay_of_hands_AuraScript::HandleProc
-- (spell_paladin.cpp:1322-1327): OnEffectProc, EFFECT_0
-- PROC_TRIGGER_SPELL. PreventDefaultAction; the action target
-- (the ev6 proc target on a done event) casts the DBC trigger
-- spell on itself, triggered (aurEff args; the C++ original-caster
-- GUID arg has no Lua twin). DORMANT until Spell.dbc ships the
-- 20234 proc entry.
local function improvedLayOnHandsProc(event, aura, target, actor, procTarget, damage)
    if procTarget == nil then
        return true
    end
    local spellId = aura:GetTriggerSpell(0)
    if spellId == 0 then
        return true
    end
    procTarget:CastSpell(procTarget, spellId, true)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_IMPROVED_LAY_ON_HANDS, 6, improvedLayOnHandsProc)

local SPELL_PALADIN_JUDGEMENTS_OF_THE_JUST = 53695 -- spell_paladin.cpp:1682 (class spell_pal_judgements_of_the_just)
local SPELL_PALADIN_JUDGEMENTS_OF_THE_JUST_PROC = 68055 -- spell_paladin.cpp:102

-- C++ spell_pal_judgements_of_the_just_AuraScript::OnProc
-- (spell_paladin.cpp:1698-1703): OnEffectProc, EFFECT_0
-- ADD_FLAT_MODIFIER. PreventDefaultAction; the holder casts the
-- proc spell on the action target (the ev6 proc target on a done
-- event), triggered (aurEff args). DORMANT until Spell.dbc ships
-- the 53695 proc entry.
local function judgementsOfTheJustProc(event, aura, target, actor, procTarget, damage)
    if procTarget == nil then
        return true
    end
    target:CastSpell(procTarget, SPELL_PALADIN_JUDGEMENTS_OF_THE_JUST_PROC, true)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_JUDGEMENTS_OF_THE_JUST, 6, judgementsOfTheJustProc)

local SPELL_PALADIN_JUDGEMENTS_OF_THE_WISE = 31876 -- spell_paladin.cpp:1715 (class spell_pal_judgements_of_the_wise)
local SPELL_PALADIN_JUDGEMENTS_OF_THE_WISE_MANA = 31930 -- spell_paladin.cpp:106
local SPELL_PALADIN_REPLENISHMENT = 57669 -- spell_paladin.cpp:107

-- C++ spell_pal_judgements_of_the_wise_AuraScript::HandleProc
-- (spell_paladin.cpp:1730-1737): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction; the proc actor casts the mana spell and
-- Replenishment on itself (the C++ nullptr target is the self-cast,
-- the hunting-party precedent), triggered (aurEff args). DORMANT
-- until Spell.dbc ships the 31876 proc entry.
local function judgementsOfTheWiseProc(event, aura, target, actor, procTarget, damage)
    if actor == nil then
        return true
    end
    actor:CastSpell(actor, SPELL_PALADIN_JUDGEMENTS_OF_THE_WISE_MANA, true)
    actor:CastSpell(actor, SPELL_PALADIN_REPLENISHMENT, true)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_JUDGEMENTS_OF_THE_WISE, 6, judgementsOfTheWiseProc)

local SPELL_PALADIN_RIGHTEOUS_VENGEANCE = 53380 -- spell_paladin.cpp:1930 (class spell_pal_righteous_vengeance)
local SPELL_PALADIN_RIGHTEOUS_VENGEANCE_DAMAGE = 61840 -- spell_paladin.cpp:108

-- C++ spell_pal_righteous_vengeance_AuraScript::HandleProc
-- (spell_paladin.cpp:1944-1965): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction; no damage -> nothing. The proc actor casts
-- the damage spell on the proc target, triggered, BP0 =
-- pct(damage, aurEff amount) / the damage spell's max ticks
-- (aura:GetSpellMaxTicks twin). DORMANT until Spell.dbc ships the
-- 53380 proc entry.
local function righteousVengeanceProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 or actor == nil or procTarget == nil then
        return true
    end
    local maxTicks = aura:GetSpellMaxTicks(SPELL_PALADIN_RIGHTEOUS_VENGEANCE_DAMAGE)
    if maxTicks <= 0 then
        return true
    end
    local amount = math.floor(damage * aura:GetAmount() / 100 / maxTicks)
    actor:CastCustomSpell(procTarget, SPELL_PALADIN_RIGHTEOUS_VENGEANCE_DAMAGE, true, amount)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_RIGHTEOUS_VENGEANCE, 6, righteousVengeanceProc)
