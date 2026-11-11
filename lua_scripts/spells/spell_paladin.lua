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
--   concentration/devotion registrations (63510/63514) have no census
--   rows and are not registered here.
--   DELTA: C++ removes the aura by (spell, caster GUID); Go
--   target:RemoveAura(id) takes no caster filter (same delta as the
--   blessing above). The remove-leg presence check reads
--   GetAuraEffectOfRankedSpell(53379, 0): Swift Retribution's
--   ADD_FLAT_MODIFIER is its EFFECT_0 (the script registers
--   EFFECT_FIRST_FOUND for that aura type).

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
