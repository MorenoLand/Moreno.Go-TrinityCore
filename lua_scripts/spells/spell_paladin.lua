-- Paladin spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_paladin.cpp.
--
-- CLOSED (this tick, verified, no code): spell_pal_seals
--   (spell_paladin.cpp:2241; bindings 20375/31801/31892/33127/38008/
--   41459/53720/53736): the class is a veto only — CheckDummyProc
--   returns false on EFFECT_2 SPELL_AURA_DUMMY "to avoid console
--   logging of unknown spell trigger". Go needs no twin:
--   checkEffectProcGates treats absent script check handlers as
--   passing (procs.go:3636-3641) and the default leg then silently
--   skips the TriggerSpell == 0 effect (procs.go:5618-5622) — no log,
--   no cast, no state change, the same observable behavior as the
--   veto. No Lua check-proc hook exists (ev6 is OnProc only). The
--   two ids with real EFFECT_0 proc arms (31801/53736, seal of
--   vengeance) already ride the Go hardcoded leg (procs.go:4891).
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
--   623 divine_purpose (31871, :541-558: on the EFFECT_2 amount
--   roll the proc target strips its STUN auras);
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
-- STAYS OUT (this tick): 626 holy_shock PORTED this tick (OnCheckCast ev8 below;
--   the HandleDummy damage/heal legs stay out — no spell-level
--   GetSpellWithRank (the ev5 player path is live, spells.go:14735));
--   627 illumination
--   PORTED this tick (ev6, below: Holy Shock family flag via
--   procEvent:GetSpellFamilyFlags(1), rank-matched Holy Shock via
--   aura:GetSpellWithRank(20473, rank), CalcPowerCost via
--   player:GetSpellPowerCost; the "proc event never threads the heal
--   spell" note is STALE — procs.go procSpellHealAuraTriggers sets
--   eventSpell on direct-heal events, disproven in-tree); 629
--   infusion_of_light + 636 spiritual_attunement + 637 sheath_of_light
--   + 638 t8_2p_bonus PORTED this tick (ev6, below); 632 lay_on_hands PORTED this tick
--   (OnCheckCast ev8 + AfterHit ev3 below); 634/635
--   seal_of_righteousness (needs attack-power / spellpower /
--   attack-time math, unbridged).

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

local SPELL_PALADIN_DIVINE_PURPOSE = 31871 -- spell_paladin.cpp:536 (class spell_pal_divine_purpose)
local MECHANIC_STUN_MASK = 4096 -- 1 << MECHANIC_STUN (12), SharedDefines.h:1355

-- C++ spell_pal_divine_purpose_AuraScript::HandleProc
-- (spell_paladin.cpp:543-553): OnEffectProc, EFFECT_2 DUMMY.
-- PreventDefaultAction (the true return on every path); on a roll
-- of the EFFECT_2 amount (roll_chance_i twin: math.random(0, 99)
-- < amount, the extra_proc idiom) the proc target strips its STUN
-- auras (RemoveAurasWithMechanic, bridged on the player and
-- creature surfaces; the AURA_REMOVE_BY_ENEMY_SPELL mode has no
-- Lua twin, the Go removal is mode-default). Rank 1 only: 31872
-- has no census row, never guessed. DORMANT until Spell.dbc ships
-- the 31871 proc entry.
local function divinePurposeProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 2 then
        return true
    end
    if procTarget == nil or aura == nil then
        return true
    end
    if math.random(0, 99) < aura:GetAmount(2) then
        procTarget:RemoveAurasWithMechanic(MECHANIC_STUN_MASK)
    end
    return true
end

RegisterAuraEvent(SPELL_PALADIN_DIVINE_PURPOSE, 6, divinePurposeProc)

local SPELL_PALADIN_ILLUMINATION = 20210 -- spell_paladin.cpp:1132 (class spell_pal_illumination)
local SPELL_PALADIN_HOLY_SHOCK_R1 = 20473 -- spell_paladin.cpp:41
local SPELL_PALADIN_ILLUMINATION_ENERGIZE = 20272 -- spell_paladin.cpp:44

-- C++ spell_pal_illumination::HandleProc (spell_paladin.cpp:1154-1177):
-- OnEffectProc EFFECT_0 PROC_TRIGGER_SPELL. PreventDefaultAction (the
-- true return on every path); only heal procs carry a heal spell
-- (procEvent:GetSpellId — the direct-heal proc site threads the
-- casting spell, procs.go procSpellHealAuraTriggers; the "never
-- threads the heal spell" note above is STALE, disproven in-tree).
-- When the heal spell carries the Holy Shock family flag
-- (SpellFamilyFlags[1] & 0x10000), the original spell is the
-- rank-matched Holy Shock (aura:GetSpellWithRank, lua_aura.go,
-- rank from procEvent:GetRank); otherwise the heal spell itself.
-- The paladin (actor) casts 20272 on itself with BP0 =
-- pct(CalcPowerCost of the original spell, EFFECT_1 CalcValue of
-- 20210) — GetSpellPowerCost (lua_player.go) +
-- GetSpellEffectCalcValue (lua_aura.go). C++ casts with
-- CastSpellExtraArgs(aurEff), i.e. TRIGGERED_FULL_MASK
-- (SpellDefines.h), so the Lua triggered CastCustomSpell is exact.
-- The effectIndex gate keeps the arm on EFFECT_0. DORMANT until
-- Spell.dbc ships the 20210 proc entry.
local function illuminationProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if aura == nil or actor == nil then
        return true
    end
    local originalSpell = procEvent:GetSpellId()
    if bit32.band(procEvent:GetSpellFamilyFlags(1), 0x10000) ~= 0 then
        originalSpell = aura:GetSpellWithRank(SPELL_PALADIN_HOLY_SHOCK_R1, procEvent:GetRank())
        if originalSpell == 0 then
            return true
        end
    end
    local pct = aura:GetSpellEffectCalcValue(SPELL_PALADIN_ILLUMINATION, 1)
    if pct == 0 then
        return true
    end
    local bp0 = math.floor(actor:GetSpellPowerCost(originalSpell) * pct / 100)
    actor:CastCustomSpell(actor, SPELL_PALADIN_ILLUMINATION_ENERGIZE, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_ILLUMINATION, 6, illuminationProc)

local SPELL_PALADIN_LAY_ON_HANDS = 633 -- spell_paladin.cpp:1774 (class spell_pal_lay_on_hands_SpellScript)
local SPELL_PALADIN_FORBEARANCE = 25771 -- spell_paladin.cpp:63
local SPELL_FAILED_TARGET_AURASTATE = 111 -- SharedDefines.h:1093
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_pal_lay_on_hands_SpellScript::CheckCast (spell_paladin.cpp:1774-1783):
-- a self-cast onto a target carrying Forbearance (25771), the Avenging
-- Wrath marker (61987) or the Immune Shield marker (61988) fails with
-- SPELL_FAILED_TARGET_AURASTATE. The caster==target test compares
-- GetGUIDLow + GetObjectType (the file's GetGUIDLow convention:
-- packed GUIDs lose bits through Lua numbers; same-type lows are
-- unique within a cast). RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast).
local function layOnHandsCheckCast(event, spell)
    local caster = spell:GetCaster()
    local target = spell:GetTarget()
    if caster ~= nil and target ~= nil and caster:GetGUIDLow() == target:GetGUIDLow() and caster:GetObjectType() == target:GetObjectType() then
        if target:HasAura(SPELL_PALADIN_FORBEARANCE) or target:HasAura(SPELL_PALADIN_AVENGING_WRATH_MARKER) or target:HasAura(SPELL_PALADIN_IMMUNE_SHIELD_MARKER) then
            return SPELL_FAILED_TARGET_AURASTATE
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_PALADIN_LAY_ON_HANDS, 8, layOnHandsCheckCast)

-- C++ spell_pal_lay_on_hands_SpellScript::HandleScript
-- (spell_paladin.cpp:1785-1793): AfterHit, when the caster is the hit
-- unit, the caster gains Forbearance (25771) and both markers (61987,
-- 61988), all triggered. RegisterSpellEvent AfterHit = 3.
local function layOnHandsAfterHit(event, spell, hitTarget)
    local caster = spell:GetCaster()
    if caster == nil or hitTarget == nil or caster:GetGUIDLow() ~= hitTarget:GetGUIDLow() or caster:GetObjectType() ~= hitTarget:GetObjectType() then
        return
    end
    caster:CastSpell(caster, SPELL_PALADIN_FORBEARANCE, true)
    caster:CastSpell(caster, SPELL_PALADIN_AVENGING_WRATH_MARKER, true)
    caster:CastSpell(caster, SPELL_PALADIN_IMMUNE_SHIELD_MARKER, true)
end

RegisterSpellEvent(SPELL_PALADIN_LAY_ON_HANDS, 3, layOnHandsAfterHit)

local SPELL_PALADIN_HOLY_SHOCK_R1 = 20473 -- spell_paladin.cpp:41 (class spell_pal_holy_shock)
local SPELL_FAILED_BAD_TARGETS = 12 -- SharedDefines.h:994
local SPELL_FAILED_UNIT_NOT_INFRONT = 134 -- SharedDefines.h:1116

-- C++ spell_pal_holy_shock_SpellScript::CheckCast (spell_paladin.cpp:1100-1117):
-- no explicit target fails SPELL_FAILED_BAD_TARGETS; a hostile target
-- must pass IsValidAttackTarget (else BAD_TARGETS) and be in front
-- (else SPELL_FAILED_UNIT_NOT_INFRONT); friendly targets pass.
-- Check-only port: the HandleDummy damage/heal legs ride ev5 (dormant,
-- no player path) + spell-level GetSpellWithRank. Rank 1 only —
-- higher-rank IDs are not in the file, never guessed (life_tap
-- precedent). RegisterSpellEvent OnCheckCast = 8 (lua_spell.go
-- fireSpellCheckCast).
local function holyShockCheckCast(event, spell)
    local caster = spell:GetCaster()
    local target = spell:GetTarget()
    if target == nil then
        return SPELL_FAILED_BAD_TARGETS
    end
    if caster ~= nil and not caster:IsFriendlyTo(target) then
        if not caster:IsValidAttackTarget(target) then
            return SPELL_FAILED_BAD_TARGETS
        end
        if not caster:IsInFront(target) then
            return SPELL_FAILED_UNIT_NOT_INFRONT
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_PALADIN_HOLY_SHOCK_R1, 8, holyShockCheckCast)

local SPELL_PALADIN_DIVINE_PROTECTION = 498 -- spell_paladin.cpp:1190 (class spell_pal_immunities)
local SPELL_PALADIN_DIVINE_SHIELD = 642 -- spell_paladin.cpp:1191
local SPELL_PALADIN_HAND_OF_PROTECTION = 1022 -- spell_paladin.cpp:1192

-- C++ spell_pal_immunities::CheckCast (spell_paladin.cpp:1207-1221):
-- a nil explicit target defaults to the caster (HoP); a target carrying
-- Forbearance (25771) or the Avenging Wrath marker (61987) fails with
-- SPELL_FAILED_TARGET_AURASTATE. Base IDs only (the C++ comment's
-- 498/642/1022) — higher ranks are DB-bound via spell_script_names,
-- unestablished in-tree. RegisterSpellEvent OnCheckCast = 8.
local function paladinImmunitiesCheckCast(event, spell)
    local target = spell:GetTarget()
    if target == nil then
        target = spell:GetCaster()
    end
    if target ~= nil and (target:HasAura(SPELL_PALADIN_FORBEARANCE) or target:HasAura(SPELL_PALADIN_AVENGING_WRATH_MARKER)) then
        return SPELL_FAILED_TARGET_AURASTATE
    end
    return SPELL_CAST_OK
end

for _, spellId in ipairs({ SPELL_PALADIN_DIVINE_PROTECTION, SPELL_PALADIN_DIVINE_SHIELD, SPELL_PALADIN_HAND_OF_PROTECTION }) do
    RegisterSpellEvent(spellId, 8, paladinImmunitiesCheckCast)
end

-- C++ spell_pal_immunities::TriggerDebuffs (spell_paladin.cpp:1223-1232):
-- AfterHit, the hit unit gains Forbearance (25771) and both markers
-- (61987, 61988). C++ is GetCaster()->AddAura(id, target); Lua AddAura
-- is self-only on both surfaces, so the twin is hitTarget:AddAura(id)
-- (slime-puddle ReplaceAura precedent). Base IDs only, same scope as
-- the CheckCast port above (-1022 all-ranks binding: higher HoP rank
-- IDs unestablished in-tree). RegisterSpellEvent AfterHit = 3.
local function paladinImmunitiesAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    hitTarget:AddAura(SPELL_PALADIN_FORBEARANCE)
    hitTarget:AddAura(SPELL_PALADIN_AVENGING_WRATH_MARKER)
    hitTarget:AddAura(SPELL_PALADIN_IMMUNE_SHIELD_MARKER)
end

for _, spellId in ipairs({ SPELL_PALADIN_DIVINE_PROTECTION, SPELL_PALADIN_DIVINE_SHIELD, SPELL_PALADIN_HAND_OF_PROTECTION }) do
    RegisterSpellEvent(spellId, 3, paladinImmunitiesAfterHit)
end

local SPELL_PALADIN_INFUSION_OF_LIGHT_R1 = 53569 -- spell_paladin.cpp:1340 (class spell_pal_infusion_of_light)
local SPELL_PALADIN_INFUSION_OF_LIGHT_R2 = 53576 -- bot_paladin_ai.cpp:111 (rank 2)
local SPELL_PALADIN_SACRED_SHIELD = 53601 -- spell_paladin.cpp:98
local SPELL_PALADIN_T9_HOLY_4P_BONUS = 67191 -- spell_paladin.cpp:99
local SPELL_PALADIN_FLASH_OF_LIGHT_PROC = 66922 -- spell_paladin.cpp:100
local PROC_HIT_CRITICAL = 0x2 -- SpellMgr.h:223

-- C++ spell_pal_infusion_of_light_AuraScript::HandleProc
-- (spell_paladin.cpp:1362-1397): OnEffectProc EFFECT_0
-- PROC_TRIGGER_SPELL. PreventDefaultAction (the true return on every
-- path, illumination precedent). Flash of Light leg: the event spell
-- carries family flag [0] & 0x40000000 with icon 242 — the heal
-- (procEvent:GetHeal, direct-heal events thread the spell and the
-- heal, procs.go) must be nonzero, and the healed target must carry
-- Sacred Shield (53601); the holder casts the Flash of Light HoT
-- (66922) on the healed target, triggered (aurEff args), BP0 =
-- pct(heal / durationSeconds, EFFECT_2 amount) where durationSeconds
-- is the HoT's max duration / 1000 (aura:GetSpellMaxDuration(66922),
-- lua_aura.go optional-spellID form added this tick), plus the T9
-- Holy 4P bonus (target:GetAuraEffect(67191, 0) AddPct). EFFECT_2
-- reads the live amount (aura:GetAmount(2)): the C++ CalcValue()
-- no-caster overload is the DBC base the live amount is computed
-- from at apply, and the live read is rank-correct across 53569 /
-- 53576. Holy Shock leg: a non-critical Holy Shock (flags [0] &
-- 0x200000 or [1] & 0x10000, hit mask without PROC_HIT_CRITICAL)
-- only suppresses the default action. DORMANT until Spell.dbc ships
-- the 53569/53576 proc entries.
local function infusionOfLightProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if aura == nil or target == nil then
        return true
    end
    if bit32.band(procEvent:GetSpellFamilyFlags(0), 0x40000000) ~= 0 and procEvent:GetSpellIconId() == 242 then
        local heal = procEvent:GetHeal()
        if heal == nil or heal == 0 then
            return true
        end
        local healed = procEvent:GetActionTarget()
        if healed ~= nil and healed:HasAura(SPELL_PALADIN_SACRED_SHIELD) then
            local duration = math.floor(aura:GetSpellMaxDuration(SPELL_PALADIN_FLASH_OF_LIGHT_PROC) / 1000)
            if duration <= 0 then
                return true
            end
            local pct = aura:GetAmount(2)
            local bp0 = math.floor(math.floor(heal / duration) * pct / 100)
            local bonus = target:GetAuraEffect(SPELL_PALADIN_T9_HOLY_4P_BONUS, 0)
            if bonus ~= nil then
                bp0 = bp0 + math.floor(bp0 * bonus / 100)
            end
            target:CastCustomSpell(healed, SPELL_PALADIN_FLASH_OF_LIGHT_PROC, true, bp0)
        end
        return true
    end
    if (bit32.band(procEvent:GetSpellFamilyFlags(0), 0x200000) ~= 0 or bit32.band(procEvent:GetSpellFamilyFlags(1), 0x10000) ~= 0) and bit32.band(procEvent:GetHitMask(), PROC_HIT_CRITICAL) == 0 then
        return true
    end
    -- Critical Holy Shock (and any other proc spell): C++ calls no
    -- PreventDefaultAction here, so the default leg fires.
    return
end

RegisterAuraEvent(SPELL_PALADIN_INFUSION_OF_LIGHT_R1, 6, infusionOfLightProc)
RegisterAuraEvent(SPELL_PALADIN_INFUSION_OF_LIGHT_R2, 6, infusionOfLightProc)

local SPELL_PALADIN_SPIRITUAL_ATTUNEMENT_R1 = 31785 -- spell_paladin.cpp:2268 (class spell_pal_spiritual_attunement)
local SPELL_PALADIN_SPIRITUAL_ATTUNEMENT_MANA = 31786 -- spell_paladin.cpp:124

-- C++ spell_pal_spiritual_attunement_AuraScript::CheckProc +
-- HandleProc (spell_paladin.cpp:2289-2303): OnEffectProc EFFECT_0
-- DUMMY. CheckProc: no proc when the healer is the healed unit
-- (procEvent:GetProcTarget() == GetActionTarget() — on the taken
-- side the proc target is the healer and the action target the
-- healed holder, procs.go procSpellHealTakenAuraTriggers) or when
-- there is no effective heal; the check is folded into the handler
-- (no separate ev6 check hook — the blessing-of-sanctuary
-- precedent: gate-fail returns true, PreventDefaultAction, without
-- casting; the ev6 charge-consume delta in the file header applies).
-- HandleProc: PreventDefaultAction; the healed unit casts the mana
-- spell (31786) on itself, triggered (aurEff args), BP0 =
-- pct(effective heal, aurEff amount). Rank 1 only — rank 2's id is
-- nowhere in the C++ tree (holy-shock precedent, never guessed).
-- DORMANT until Spell.dbc ships the 31785 proc entry.
local function spiritualAttunementProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if procEvent == nil then
        return true
    end
    local healer = procEvent:GetProcTarget()
    local healed = procEvent:GetActionTarget()
    if healer ~= nil and healed ~= nil and healer:GetGUIDLow() == healed:GetGUIDLow() and healer:GetObjectType() == healed:GetObjectType() then
        return true
    end
    local effectiveHeal = procEvent:GetEffectiveHeal()
    if effectiveHeal == nil or effectiveHeal == 0 then
        return true
    end
    if healed == nil then
        return true
    end
    local amount = math.floor(effectiveHeal * aura:GetAmount() / 100)
    healed:CastCustomSpell(healed, SPELL_PALADIN_SPIRITUAL_ATTUNEMENT_MANA, true, amount)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_SPIRITUAL_ATTUNEMENT_R1, 6, spiritualAttunementProc)

local SPELL_PALADIN_SHEATH_OF_LIGHT_R1 = 53501 -- spell_paladin.cpp:2316 (class spell_pal_sheath_of_light)
local SPELL_PALADIN_SHEATH_OF_LIGHT_R2 = 53502 -- bot_paladin_ai.cpp:107 (rank 2)
local SPELL_PALADIN_SHEATH_OF_LIGHT_R3 = 53503 -- bot_paladin_ai.cpp:108 (rank 3)
local SPELL_PALADIN_SHEATH_OF_LIGHT_HEAL = 54203 -- spell_paladin.cpp:109

-- C++ spell_pal_sheath_of_light_AuraScript::HandleProc
-- (spell_paladin.cpp:2338-2356): OnEffectProc EFFECT_1 DUMMY.
-- PreventDefaultAction; no heal -> nothing. The proc actor (the
-- healer) casts the sheath HoT (54203) on the proc target (the
-- healed unit on the done side), triggered (aurEff args), BP0 =
-- pct(effective heal, EFFECT_1 amount) / the HoT's max ticks
-- (aura:GetSpellMaxTicks). DORMANT until Spell.dbc ships the
-- 53501/53502/53503 proc entries.
local function sheathOfLightProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 1 then
        return true
    end
    if procEvent == nil or procEvent:GetHeal() == 0 then
        return true
    end
    if aura == nil or actor == nil then
        return true
    end
    local healed = procEvent:GetProcTarget()
    if healed == nil then
        return true
    end
    local maxTicks = aura:GetSpellMaxTicks(SPELL_PALADIN_SHEATH_OF_LIGHT_HEAL)
    if maxTicks <= 0 then
        return true
    end
    local amount = math.floor(math.floor(procEvent:GetEffectiveHeal() * aura:GetAmount(1) / 100) / maxTicks)
    actor:CastCustomSpell(healed, SPELL_PALADIN_SHEATH_OF_LIGHT_HEAL, true, amount)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_SHEATH_OF_LIGHT_R1, 6, sheathOfLightProc)
RegisterAuraEvent(SPELL_PALADIN_SHEATH_OF_LIGHT_R2, 6, sheathOfLightProc)
RegisterAuraEvent(SPELL_PALADIN_SHEATH_OF_LIGHT_R3, 6, sheathOfLightProc)

local SPELL_PALADIN_T8_2P_BONUS = 64890 -- spell_paladin.cpp:2432 (class spell_pal_t8_2p_bonus)
local SPELL_PALADIN_HOLY_MENDING = 64891 -- spell_paladin.cpp:130

-- C++ spell_pal_t8_2p_bonus_AuraScript::HandleProc
-- (spell_paladin.cpp:2446-2468): OnEffectProc EFFECT_0 DUMMY.
-- PreventDefaultAction; no heal -> nothing. The proc actor (the
-- healer) casts Holy Mending (64891) on the proc target (the healed
-- unit on the done side), triggered (aurEff args), BP0 = pct(raw
-- heal — GetHeal, not the effective heal the sheath leg uses —
-- aurEff amount) / the mending HoT's max ticks
-- (aura:GetSpellMaxTicks). DORMANT until Spell.dbc ships the 64890
-- proc entry.
local function t8TwoPieceBonusProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if procEvent == nil or procEvent:GetHeal() == 0 then
        return true
    end
    if aura == nil or actor == nil then
        return true
    end
    local healed = procEvent:GetProcTarget()
    if healed == nil then
        return true
    end
    local maxTicks = aura:GetSpellMaxTicks(SPELL_PALADIN_HOLY_MENDING)
    if maxTicks <= 0 then
        return true
    end
    local amount = math.floor(math.floor(procEvent:GetHeal() * aura:GetAmount() / 100) / maxTicks)
    actor:CastCustomSpell(healed, SPELL_PALADIN_HOLY_MENDING, true, amount)
    return true
end

RegisterAuraEvent(SPELL_PALADIN_T8_2P_BONUS, 6, t8TwoPieceBonusProc)
