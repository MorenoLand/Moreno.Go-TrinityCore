-- Warrior spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_warrior.cpp.
--
-- PORTED (tick 201, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_warr_vigilance (50720, spell_warrior.cpp:1088-1156):
--   EFFECT_0 PROC_TRIGGER_SPELL, REAL_OR_REAPPLY. OnApply the target
--   casts the Damage Reduction aura (68066) on itself, triggered, and
--   casts Vigilance Redirect Threat (59665) on the caster, triggered.
--   OnRemove, if the target still has 68066 and has none of Blessing
--   of Sanctuary (20911), Greater Blessing of Sanctuary (25899) or
--   Renewed Hope (63944), it drops 68066. C++ uses
--   RemoveAurasDueToSpell; the Go twin is target:RemoveAura(id):
--   68066 is self-cast once by the apply leg, so a single application
--   exists and removing the spell's aura is the same removal.
--   DELTA: the Go apply hook is fresh-only (C++ REAL_OR_REAPPLY also
--   fires on re-apply); same pre-existing delta as Renew (tick 199).
-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   CheckProc / HandleProc (:1130-1144): the caster-nil gate rides
--   aura:GetCaster(); the target casts Vigilance Proc (50725) on the
--   caster, triggered; the true return is PreventDefaultAction, so
--   the DBC trigger leg does not also fire (procs.go:3919,
--   boss_ai.go:10852). HandleRemove's UnregisterRedirectThreat
--   (59665, caster GUID) leg (:1128) is Go-covered
--   (spells.go:36962). Glyph of Vigilance (63326) folds in Go
--   (spell_threat.go handleEffectRedirectThreat).

local SPELL_WARRIOR_VIGILANCE = 50720 -- spell_warrior.cpp:87
local SPELL_WARRIOR_VIGILANCE_REDIRECT_THREAT = 59665 -- spell_warrior.cpp:67
local SPELL_WARRIOR_VIGILANCE_PROC = 50725 -- spell_warrior.cpp:66
local SPELL_WARRIOR_TAUNT = 355 -- spell_warrior.cpp:61
local SPELL_GEN_DAMAGE_REDUCTION_AURA = 68066 -- spell_warrior.cpp:87
local SPELL_PALADIN_BLESSING_OF_SANCTUARY = 20911 -- spell_warrior.cpp:85
local SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY = 25899 -- spell_warrior.cpp:86
local SPELL_PRIEST_RENEWED_HOPE = 63944 -- spell_warrior.cpp:87

-- C++ spell_warr_vigilance_AuraScript::HandleApply (spell_warrior.cpp:1108-1115).
local function vigilanceApply(event, aura, target)
    target:CastSpell(target, SPELL_GEN_DAMAGE_REDUCTION_AURA, true)
    local caster = aura:GetCaster()
    if caster ~= nil then
        target:CastSpell(caster, SPELL_WARRIOR_VIGILANCE_REDIRECT_THREAT, true)
    end
end

RegisterAuraEvent(SPELL_WARRIOR_VIGILANCE, 2, vigilanceApply)

-- C++ spell_warr_vigilance_AuraScript::HandleRemove (spell_warrior.cpp:1117-1129).
local function vigilanceRemove(event, aura, target)
    if target:HasAura(SPELL_GEN_DAMAGE_REDUCTION_AURA)
        and not target:HasAura(SPELL_PALADIN_BLESSING_OF_SANCTUARY)
        and not target:HasAura(SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY)
        and not target:HasAura(SPELL_PRIEST_RENEWED_HOPE) then
        target:RemoveAura(SPELL_GEN_DAMAGE_REDUCTION_AURA)
    end
end

RegisterAuraEvent(SPELL_WARRIOR_VIGILANCE, 3, vigilanceRemove)

-- C++ spell_warr_vigilance_AuraScript::CheckProc + HandleProc
-- (spell_warrior.cpp:1130-1144): the caster-nil gate rides
-- aura:GetCaster(); the target casts Vigilance Proc (50725) on the
-- caster, triggered; the true return is PreventDefaultAction, so the
-- DBC trigger leg does not also fire (procs.go:4016, boss_ai.go:11077).
local function vigilanceProc(event, aura, target, actor, procTarget, damage)
    local caster = aura:GetCaster()
    if caster ~= nil then
        target:CastSpell(caster, SPELL_WARRIOR_VIGILANCE_PROC, true)
    end
    return true
end

RegisterAuraEvent(SPELL_WARRIOR_VIGILANCE, 6, vigilanceProc)

-- C++ spell_warr_vigilance_trigger_SpellScript::HandleScript
-- (spell_warrior.cpp:1200-1206): OnEffectHitTarget (RegisterSpellEvent
-- OnEffectHitTarget = 5, the muru/kaelthas precedent — not OnHit = 2).
-- PreventHitDefaultEffect (Go's script-effect default has no 50725
-- leg); the hit player resets Taunt (355) cooldown, update=true. The
-- GetObjectType() gate is the GetHitPlayer() twin.
local function vigilanceTrigger(event, spell, hitTarget)
    if hitTarget ~= nil and hitTarget:GetObjectType() == "Player" then
        hitTarget:ResetSpellCooldown(SPELL_WARRIOR_TAUNT, true)
    end
end

RegisterSpellEvent(SPELL_WARRIOR_VIGILANCE_PROC, 5, vigilanceTrigger)

-- PORTED (this tick, RegisterSpellEvent OnObjectAreaTargetSelect = 7):
--   spell_warr_intimidating_shout::FilterTargets
--   (spell_warrior.cpp:625-628, 632-633): EFFECT_1 and EFFECT_2,
--   TARGET_UNIT_SRC_AREA_ENEMY — the explicit target drops out of
--   both area lists (its own EFFECT_0 cower leg still hits it).
--   ev7 fires on both paths (creature boss_ai.go, player
--   lua_spell.go firePlayerSpellTargetSelects); the Go player legs
--   already filtered the flat list, so the Lua Remove is a no-op there. The spell
--   object's GetTarget is the explicit unit target (the C++
--   GetExplTargetWorldObject). DORMANT until Spell.dbc drives the cast.
local SPELL_WARRIOR_INTIMIDATING_SHOUT = 5246 -- spell_warrior.cpp:615

local function intimidatingShoutFilter(event, spell, effectIndex, targets)
    local target = spell:GetTarget()
    if target ~= nil then
        targets:Remove(target)
    end
end

RegisterSpellEvent(SPELL_WARRIOR_INTIMIDATING_SHOUT, 7, intimidatingShoutFilter)

local SPELL_WARRIOR_DAMAGE_SHIELD = 58872 -- spell_warrior.cpp:229
local SPELL_WARRIOR_DAMAGE_SHIELD_DAMAGE = 59653 -- spell_warrior.cpp:41

-- C++ spell_warr_damage_shield_AuraScript::OnProc (spell_warrior.cpp:246-255):
-- PreventDefaultAction; damage is pct of the holder's shield block value by
-- the EFFECT_0 amount (CalculatePct truncation); the holder casts 59653 on
-- the proc target, triggered (CastSpellExtraArgs(aurEff) sets
-- TRIGGERED_FULL_MASK, SpellDefines.h:166), BP0. DORMANT until Spell.dbc
-- ships the 58872 proc entry. GetShieldBlockValue bridge landed
-- 2026-11-19 (lua_player.go): the port's target:GetShieldBlockValue()
-- call previously read a missing method. TDB spell_proc holds no row
-- for 58872 (dump-verified 2026-11-19); the generated DBC entry governs.
local function damageShieldProc(event, aura, target, actor, procTarget, damage)
    if target == nil or procTarget == nil then
        return true
    end
    local dmg = math.floor(target:GetShieldBlockValue() * aura:GetAmount(0) / 100)
    target:CastCustomSpell(procTarget, SPELL_WARRIOR_DAMAGE_SHIELD_DAMAGE, true, dmg)
    return true
end

RegisterAuraEvent(SPELL_WARRIOR_DAMAGE_SHIELD, 6, damageShieldProc)

local SPELL_WARRIOR_SUDDEN_DEATH_R1 = 29723 -- spell_warrior.cpp:57
local SPELL_WARRIOR_SUDDEN_DEATH_R2 = 29725
local SPELL_WARRIOR_SUDDEN_DEATH_R3 = 29724
local SPELL_WARRIOR_BLOODSURGE_R1 = 46913 -- spell_warrior.cpp:39
local SPELL_WARRIOR_BLOODSURGE_R2 = 46914
local SPELL_WARRIOR_BLOODSURGE_R3 = 46915
local SPELL_WARRIOR_T10_MELEE_4P_BONUS = 70847 -- spell_warrior.cpp:77
local SPELL_WARRIOR_EXTRA_CHARGE = 70849 -- spell_warrior.cpp:48
local SPELL_WARRIOR_SLAM_GCD_REDUCED = 71072 -- spell_warrior.cpp:61
local SPELL_WARRIOR_EXECUTE_GCD_REDUCED = 71069 -- spell_warrior.cpp:47

-- C++ spell_warr_extra_proc_AuraScript::HandleProc (spell_warrior.cpp:458-476):
-- no PreventDefaultAction. If the holder has the T10 melee 4P bonus (70847)
-- and its EFFECT_0 amount wins the roll (roll_chance_i twin:
-- math.random(0, 99) < amount, the spell_generic idiom), cast 70849 on
-- self, then the talent's GCD-reduced spell on self (71072 Bloodsurge /
-- 71069 Sudden Death), both triggered (CastSpell with the triggering
-- aura effect). The IsRankOf branch is decided by which spell id the
-- handler is registered for. TDB binds all ranks (-29723, -46913), so
-- R2/R3 register the same factory: Sudden Death 29724/29725, Bloodsurge
-- 46914/46915 (sequential ranks; 29724 is R3 and 46915 is R3 per
-- bot_warrior_ai.cpp:113/117, corroborated by Wowhead proc chances
-- 29723=3%/29725=6%/29724=9%). DORMANT
-- until Spell.dbc ships the proc entries.
local function extraProc(gcdReduced)
    return function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
        if target == nil then
            return
        end
        if effectIndex ~= nil and effectIndex ~= 0 then
            return
        end
        local bonus = target:GetAuraEffect(SPELL_WARRIOR_T10_MELEE_4P_BONUS, 0)
        if bonus == nil then
            return
        end
        if math.random(0, 99) >= bonus then
            return
        end
        target:CastSpell(nil, SPELL_WARRIOR_EXTRA_CHARGE, true)
        target:CastSpell(nil, gcdReduced, true)
    end
end

RegisterAuraEvent(SPELL_WARRIOR_SUDDEN_DEATH_R1, 6, extraProc(SPELL_WARRIOR_EXECUTE_GCD_REDUCED))
RegisterAuraEvent(SPELL_WARRIOR_SUDDEN_DEATH_R2, 6, extraProc(SPELL_WARRIOR_EXECUTE_GCD_REDUCED))
RegisterAuraEvent(SPELL_WARRIOR_SUDDEN_DEATH_R3, 6, extraProc(SPELL_WARRIOR_EXECUTE_GCD_REDUCED))
RegisterAuraEvent(SPELL_WARRIOR_BLOODSURGE_R1, 6, extraProc(SPELL_WARRIOR_SLAM_GCD_REDUCED))
RegisterAuraEvent(SPELL_WARRIOR_BLOODSURGE_R2, 6, extraProc(SPELL_WARRIOR_SLAM_GCD_REDUCED))
RegisterAuraEvent(SPELL_WARRIOR_BLOODSURGE_R3, 6, extraProc(SPELL_WARRIOR_SLAM_GCD_REDUCED))

local SPELL_WARRIOR_EXECUTE_R1 = 5308 -- spell_warrior.cpp:378
local SPELL_WARRIOR_EXECUTE_R2 = 20658
local SPELL_WARRIOR_EXECUTE_R3 = 20660
local SPELL_WARRIOR_EXECUTE_R4 = 20661
local SPELL_WARRIOR_EXECUTE_R5 = 20662
local SPELL_WARRIOR_EXECUTE_R6 = 25234
local SPELL_WARRIOR_EXECUTE_R7 = 25236
local SPELL_WARRIOR_EXECUTE_R8 = 47470
local SPELL_WARRIOR_EXECUTE_R9 = 47471
local SPELL_WARRIOR_EXECUTE = 20647 -- spell_warrior.cpp:46 (damage spell)
local SPELL_WARRIOR_GLYPH_OF_EXECUTION = 58367 -- spell_warrior.cpp:49
local SPELL_AURA_PROC_TRIGGER_SPELL = 42 -- SpellAuraDefines.h:122, procs.go:1287
local SPELLFAMILY_GENERIC = 0 -- SharedDefines.h:3581
local WARRIOR_ICON_ID_SUDDEN_DEATH = 1989 -- spell_warrior.cpp:79
local POWER_RAGE = 1 -- SharedDefines.h power enum, spell_druid.lua:98 precedent

-- C++ spell_warr_execute_SpellScript::HandleEffect (spell_warrior.cpp:397-421):
-- OnEffectHitTarget EFFECT_0 DUMMY — rageUsed is the extra rage up to
-- 300 - CalcPowerCost (30 rage total at base 15 cost); newRage floors
-- at 0, then at the Sudden Death rage save when the talent aura is
-- present (icon 1989, EFFECT_1 CalcValue * 10: 3/7/10 rage by rank,
-- Blizzard-confirmed; read live via GetSpellEffectCalcValue, not
-- hardcoded). The power is set before the Glyph of Execution bonus:
-- the glyph's EFFECT_0 amount * 10 joins rageUsed only for damage.
-- bp = GetEffectValue(0) + floor(rageUsed * DamageMultiplier + AP *
-- 0.2), and the 20647 damage spell is cast on the hit target,
-- triggered, BP0 = bp. DamageMultiplier is EffectChainAmplitude
-- (SpellInfo.cpp:345) via GetEffectDamageMultiplier — no new DBC
-- field was needed; the Go loader already carries it. Spot check on
-- the formula: R1 tooltip 125 + 3/rage means DamageMultiplier 0.3,
-- R5 600 + 15/rage means 1.5. Binds all 9 ranks via DB ScriptName
-- (-5308, TDB spell_script_names, chain R1-R9). ev5 fires on the
-- player direct-cast path (spells.go:14892). DORMANT until Spell.dbc
-- ships the records.
local function executeEffect(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local rage = caster:GetPower(POWER_RAGE)
    local rageUsed = math.min(300 - spell:GetPowerCost(), rage)
    local newRage = math.max(0, rage - rageUsed)
    local suddenDeath = caster:GetAuraEffectByIcon(SPELL_AURA_PROC_TRIGGER_SPELL, SPELLFAMILY_GENERIC, WARRIOR_ICON_ID_SUDDEN_DEATH, 0)
    if suddenDeath ~= nil then
        local ragesave = suddenDeath:GetSpellEffectCalcValue(suddenDeath:GetAuraId(), 1) * 10
        newRage = math.max(newRage, ragesave)
    end
    caster:SetPower(newRage, POWER_RAGE)
    local glyph = caster:GetAuraEffect(SPELL_WARRIOR_GLYPH_OF_EXECUTION, 0)
    if glyph ~= nil then
        rageUsed = rageUsed + glyph * 10
    end
    local bp = spell:GetEffectValue(0) + math.floor(rageUsed * spell:GetEffectDamageMultiplier(0) + caster:GetTotalAttackPowerValue(0) * 0.2)
    caster:CastCustomSpell(hitTarget, SPELL_WARRIOR_EXECUTE, true, bp)
end

RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R1, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R2, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R3, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R4, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R5, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R6, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R7, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R8, 5, executeEffect)
RegisterSpellEvent(SPELL_WARRIOR_EXECUTE_R9, 5, executeEffect)

local SPELL_WARRIOR_CHARGE_R1 = 100 -- diminishing.go:113 (Charge Stun ranks)
local SPELL_WARRIOR_CHARGE_R2 = 6178
local SPELL_WARRIOR_CHARGE_R3 = 11578
local SPELL_WARRIOR_CHARGE = 34846 -- spell_warrior.cpp:40
local SPELL_WARRIOR_JUGGERNAUT_CRIT_BONUS_TALENT = 64976 -- spell_warrior.cpp:52
local SPELL_WARRIOR_JUGGERNAUT_CRIT_BONUS_BUFF = 65156 -- spell_warrior.cpp:51

-- C++ spell_warr_charge_SpellScript::HandleDummy (spell_warrior.cpp:179-190):
-- OnEffectHitTarget EFFECT_1 DUMMY — the caster casts Charge (34846) on
-- itself, triggered, BP0 = effect value; Juggernaut talent (64976) adds
-- the 65156 crit buff. ev5 fires on the player direct-cast path
-- (spells.go:14892). Binds the three Charge ranks via DB ScriptName.
-- DORMANT until Spell.dbc ships the records.
local function chargeDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastCustomSpell(caster, SPELL_WARRIOR_CHARGE, true, spell:GetEffectValue(1))
    if caster:HasAura(SPELL_WARRIOR_JUGGERNAUT_CRIT_BONUS_TALENT) then
        caster:CastSpell(caster, SPELL_WARRIOR_JUGGERNAUT_CRIT_BONUS_BUFF, true)
    end
end

RegisterSpellEvent(SPELL_WARRIOR_CHARGE_R1, 5, chargeDummy)
RegisterSpellEvent(SPELL_WARRIOR_CHARGE_R2, 5, chargeDummy)
RegisterSpellEvent(SPELL_WARRIOR_CHARGE_R3, 5, chargeDummy)

local SPELL_WARRIOR_OVERPOWER_R1 = 7384 -- spell_warrior.cpp:716
local SPELL_WARRIOR_OVERPOWER_R2 = 7887
local SPELL_WARRIOR_OVERPOWER_R3 = 11584
local SPELL_WARRIOR_OVERPOWER_R4 = 11585
local SPELL_WARRIOR_UNRELENTING_ASSAULT_RANK_1 = 46859 -- spell_warrior.cpp:62
local SPELL_WARRIOR_UNRELENTING_ASSAULT_RANK_2 = 46860 -- spell_warrior.cpp:63
local SPELL_WARRIOR_UNRELENTING_ASSAULT_TRIGGER_1 = 64849 -- spell_warrior.cpp:64
local SPELL_WARRIOR_UNRELENTING_ASSAULT_TRIGGER_2 = 64850 -- spell_warrior.cpp:65

-- C++ spell_warr_overpower_SpellScript::HandleEffect (spell_warrior.cpp:725-743):
-- OnEffectHitTarget EFFECT_0 ANY — if the caster has Unrelenting Assault
-- R1/R2 and the hit target is a player currently casting a non-melee
-- spell, the target casts 64849/64850 on itself, triggered.
-- IsNonMeleeSpellCast(false, false, true) (Unit.cpp:3182): withDelayed=false,
-- skipChanneled=false, skipAutorepeat=true; skipInstant defaults TRUE
-- (Unit.h:1369), so instant generic casts do NOT count — only a live
-- non-delayed cast with a cast time, plus any live channeled cast;
-- autorepeat is skipped. The Go twin matches: activeCast is armed only
-- inside `if castTime > 0` (spells.go:4505), so GetCurrentSpell(1) is nil
-- for instants; GetCurrentSpell(2) reads the channel.
-- The GetObjectType() gate is the GetHitPlayer() twin (vigilanceTrigger
-- precedent above). ev5 fires on the player direct-cast path
-- (spells.go:14892). DORMANT until Spell.dbc ships the records.
local function overpowerEffect(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil or hitTarget:GetObjectType() ~= "Player" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local triggerId = nil
    if caster:HasAura(SPELL_WARRIOR_UNRELENTING_ASSAULT_RANK_1) then
        triggerId = SPELL_WARRIOR_UNRELENTING_ASSAULT_TRIGGER_1
    elseif caster:HasAura(SPELL_WARRIOR_UNRELENTING_ASSAULT_RANK_2) then
        triggerId = SPELL_WARRIOR_UNRELENTING_ASSAULT_TRIGGER_2
    end
    if triggerId == nil then
        return
    end
    if hitTarget:GetCurrentSpell(1) ~= nil then
        hitTarget:CastSpell(hitTarget, triggerId, true)
        return
    end
    if hitTarget:GetCurrentSpell(2) ~= nil then
        hitTarget:CastSpell(hitTarget, triggerId, true)
    end
end

RegisterSpellEvent(SPELL_WARRIOR_OVERPOWER_R1, 5, overpowerEffect)
RegisterSpellEvent(SPELL_WARRIOR_OVERPOWER_R2, 5, overpowerEffect)
RegisterSpellEvent(SPELL_WARRIOR_OVERPOWER_R3, 5, overpowerEffect)
RegisterSpellEvent(SPELL_WARRIOR_OVERPOWER_R4, 5, overpowerEffect)

local SPELL_WARRIOR_SHATTERING_THROW_R1 = 64380 -- spell_warrior.cpp:897
local SPELL_WARRIOR_SHATTERING_THROW_R2 = 65941
local MECHANIC_IMMUNE_SHIELD = 29 -- SharedDefines.h:1386

-- C++ spell_warr_shattering_throw_SpellScript::HandleScript
-- (spell_warrior.cpp:905-912): OnEffectHitTarget EFFECT_0 SCRIPT_EFFECT —
-- PreventHitDefaultEffect, then the hit unit drops all auras with the
-- IMMUNE_SHIELD mechanic. ev5 fires on the player direct-cast path
-- (spells.go:14892). The PreventHitDefaultEffect leg needs no modeling:
-- the session spell object carries no such method (creature-path only,
-- lua_spell.go:633) and the Go SCRIPT_EFFECT default is a no-op. Mask
-- idiom is the spell_generic mana-burn twin (2 ^ mechanic).
-- DORMANT until Spell.dbc ships the records.
local function shatteringThrowScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAurasWithMechanic(2 ^ MECHANIC_IMMUNE_SHIELD)
end

RegisterSpellEvent(SPELL_WARRIOR_SHATTERING_THROW_R1, 5, shatteringThrowScript)
RegisterSpellEvent(SPELL_WARRIOR_SHATTERING_THROW_R2, 5, shatteringThrowScript)

local SPELL_WARRIOR_SLAM_R1 = 1464 -- spell_warrior.cpp:929
local SPELL_WARRIOR_SLAM_R2 = 8820
local SPELL_WARRIOR_SLAM_R3 = 11604
local SPELL_WARRIOR_SLAM_R4 = 11605
local SPELL_WARRIOR_SLAM_R5 = 25241
local SPELL_WARRIOR_SLAM_R6 = 25242
local SPELL_WARRIOR_SLAM_R7 = 47474
local SPELL_WARRIOR_SLAM_R8 = 47475
local SPELL_WARRIOR_SLAM = 50783 -- spell_warrior.cpp:55

-- C++ spell_warr_slam_SpellScript::HandleDummy (spell_warrior.cpp:941-949):
-- OnEffectHitTarget EFFECT_0 DUMMY — the caster casts Slam (50783) on the
-- hit unit, triggered, BP0 = effect value. ev5 fires on the player
-- direct-cast path (spells.go:14892). TDB spell_ranks chain
-- 1464/8820/11604/11605/25241/25242/47474/47475 (R1-R8) + (-1464)
-- all-ranks binding: all eight ranks registered on the same
-- rank-agnostic handler (C++ casts fixed 50783 with BP0 = effect
-- value, no rank ids). DORMANT until Spell.dbc ships the records.
local function slamDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastCustomSpell(hitTarget, SPELL_WARRIOR_SLAM, true, spell:GetEffectValue(0))
end

for _, spellId in ipairs({ SPELL_WARRIOR_SLAM_R1, SPELL_WARRIOR_SLAM_R2, SPELL_WARRIOR_SLAM_R3, SPELL_WARRIOR_SLAM_R4, SPELL_WARRIOR_SLAM_R5, SPELL_WARRIOR_SLAM_R6, SPELL_WARRIOR_SLAM_R7, SPELL_WARRIOR_SLAM_R8 }) do
    RegisterSpellEvent(spellId, 5, slamDummy)
end

local SPELL_WARRIOR_SWORD_AND_BOARD = 46951 -- spell_warrior.cpp:1021
local SPELL_WARRIOR_SWORD_AND_BOARD_R2 = 46952
local SPELL_WARRIOR_SWORD_AND_BOARD_R3 = 46953
local SPELL_CATEGORY_SHIELD_SLAM = 1209 -- spell_warrior.cpp:88

-- C++ spell_warr_sword_and_board_AuraScript::HandleProc
-- (spell_warrior.cpp:1031-1040): OnEffectProc EFFECT_0 PROC_TRIGGER_SPELL —
-- no PreventDefaultAction; resets the cooldowns of every spell whose DBC
-- category is Shield Slam (1209). ResetTypeCooldowns is the
-- SpellHistory::ResetCooldowns predicate bridge (lua_player.go:1962);
-- the update=true second arg is the vigilanceTrigger precedent above.
-- TDB binds all ranks (-46951); R2/R3 register the same handler.
-- DORMANT until Spell.dbc ships the proc entries.
local function swordAndBoardProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if target == nil then
        return
    end
    if effectIndex ~= nil and effectIndex ~= 0 then
        return
    end
    target:ResetTypeCooldowns(SPELL_CATEGORY_SHIELD_SLAM, true)
end

RegisterAuraEvent(SPELL_WARRIOR_SWORD_AND_BOARD, 6, swordAndBoardProc)
RegisterAuraEvent(SPELL_WARRIOR_SWORD_AND_BOARD_R2, 6, swordAndBoardProc)
RegisterAuraEvent(SPELL_WARRIOR_SWORD_AND_BOARD_R3, 6, swordAndBoardProc)

local SPELL_WARRIOR_SWEEPING_STRIKES_R1 = 12328 -- spell_warrior.cpp:965
local SPELL_WARRIOR_SWEEPING_STRIKES_R2 = 18765
local SPELL_WARRIOR_SWEEPING_STRIKES_R3 = 35429
local SPELL_WARRIOR_SWEEPING_STRIKES_EXTRA_ATTACK_1 = 12723 -- spell_warrior.cpp:59
local SPELL_WARRIOR_SWEEPING_STRIKES_EXTRA_ATTACK_2 = 26654 -- spell_warrior.cpp:60
local SPELL_WARRIOR_BLADESTORM_PERIODIC_WHIRLWIND = 50622 -- spell_warrior.cpp:36
local SPELL_WARRIOR_EXECUTE = 20647 -- spell_warrior.cpp:46

-- C++ spell_warr_sweeping_strikes_AuraScript::CheckProc + HandleProc
-- (spell_warrior.cpp:979-1004): DoCheckProc + OnEffectProc EFFECT_0 DUMMY.
-- CheckProc picks a nearby target via SelectNearbyTarget(procTarget) and
-- vetoes when none; HandleProc calls PreventDefaultAction, which rides
-- the true return. SelectNearbyTarget (Unit.cpp:10811, dist
-- NOMINAL_MELEE_RANGE 5.0f, ObjectDefines.h:42): unfriendly units in
-- range, minus the victim and the exclude (proc target), minus non-LoS /
-- totems / spirit services / critters, random pick. The Go twin is
-- GetUnfriendlyUnitsInRange(5) (unfriendly, self excluded; its alive
-- filter only runs when dead != 0, so IsAlive is applied in Lua) with
-- the victim, proc-target, LoS and totem/critter (GetCreatureType 11/8)
-- filters applied in Lua; spirit-service exclusion has no bridge
-- (negligible). If the proc spell is Bladestorm periodic whirlwind
-- (50622) or Execute (20647) while the second target is not under 20%
-- HP, the holder casts extra attack 2 (26654) on it, triggered; else
-- extra attack 1 (12723), triggered, BP0 = proc damage. The <20% gate
-- is the AURA_STATE_HEALTHLESS_20_PERCENT twin via GetHealthPct.
-- Zero-damage guard is deliberate: the Lua surface cannot see whether
-- DamageInfo exists (damage arrives as a bare uint32), so a 0-damage
-- fully-absorbed hit's BP0-0 cast is skipped rather than misfiring on
-- no-damage procs.
-- DORMANT until Spell.dbc ships the 35429 proc entry.
local function sweepingStrikesProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if target == nil or procTarget == nil then
        return true
    end
    if effectIndex ~= nil and effectIndex ~= 0 then
        return
    end
    if damage == nil or damage <= 0 then
        return true
    end
    local victimGuid = 0
    local victim = target:GetVictim()
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local procGuid = procTarget:GetGUID()
    local picks = {}
    for _, cand in ipairs(target:GetUnfriendlyUnitsInRange(5)) do
        local guid = cand:GetGUID()
        if guid ~= procGuid and guid ~= victimGuid
            and cand:IsAlive()
            and cand:GetCreatureType() ~= 11 and cand:GetCreatureType() ~= 8
            and target:IsWithinLoS(cand) then
            picks[#picks + 1] = cand
        end
    end
    if #picks == 0 then
        return true
    end
    local second = picks[math.random(#picks)]
    local procSpell = 0
    if procEvent ~= nil then
        procSpell = procEvent:GetSpellId()
    end
    if procSpell == SPELL_WARRIOR_BLADESTORM_PERIODIC_WHIRLWIND
        or (procSpell == SPELL_WARRIOR_EXECUTE and second:GetHealthPct() >= 20) then
        target:CastSpell(second, SPELL_WARRIOR_SWEEPING_STRIKES_EXTRA_ATTACK_2, true)
    else
        target:CastCustomSpell(second, SPELL_WARRIOR_SWEEPING_STRIKES_EXTRA_ATTACK_1, true, damage)
    end
    return true
end

RegisterAuraEvent(SPELL_WARRIOR_SWEEPING_STRIKES_R1, 6, sweepingStrikesProc)
RegisterAuraEvent(SPELL_WARRIOR_SWEEPING_STRIKES_R2, 6, sweepingStrikesProc)
RegisterAuraEvent(SPELL_WARRIOR_SWEEPING_STRIKES_R3, 6, sweepingStrikesProc)

local SPELL_WARRIOR_SECOND_WIND_R1 = 29834 -- spell_warrior.cpp:846 (class spell_warr_second_wind)
local SPELL_WARRIOR_SECOND_WIND_R2 = 29838 -- bot_warrior_ai.cpp:108, rank 2 (talent chain via Talent.dbc; no spell_ranks row, Butchery precedent)
local SPELL_WARRIOR_SECOND_WIND_TRIGGER_1 = 29841 -- spell_warrior.cpp:69
local SPELL_WARRIOR_SECOND_WIND_TRIGGER_2 = 29842 -- spell_warrior.cpp:70
local SECOND_WIND_TRIGGERS = {
    [SPELL_WARRIOR_SECOND_WIND_R1] = SPELL_WARRIOR_SECOND_WIND_TRIGGER_1,
    [SPELL_WARRIOR_SECOND_WIND_R2] = SPELL_WARRIOR_SECOND_WIND_TRIGGER_2,
}
local SECOND_WIND_ROOT_STUN_MASK = 4224 -- (1 << MECHANIC_ROOT (7)) | (1 << MECHANIC_STUN (12)), SharedDefines.h:1364,1369

-- C++ spell_warr_second_wind_AuraScript::CheckProc + HandleProc
-- (spell_warrior.cpp:865-882): DoCheckProc + OnEffectProc EFFECT_0 DUMMY.
-- CheckProc reads the triggering spell's GetAllEffectsMechanicMask and
-- only procs on a root- or stun-mechanic spell (no triggering spell means
-- no proc); a failed gate vetoes like sweepingStrikes (return true -- the
-- C++ proc never fires, default leg included). HandleProc calls
-- PreventDefaultAction (the ev6 true return), then the action target
-- (the holder on the taken event) self-casts triggeredSpells[rank - 1]
-- triggered via the aurEff overload (TRIGGERED_FULL_MASK,
-- SpellDefines.h:166) -- the CastSpell `true` arg is exact. The rank
-- reads via aura:GetAuraId (aura:GetRank is 0 here -- the talent chain
-- has no spell_ranks row). TDB (-29834) all-ranks binding. DORMANT
-- until Spell.dbc ships the 29834/29838 proc entries.
local function secondWindProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= nil and effectIndex ~= 0 then
        return
    end
    if procEvent == nil then
        return true
    end
    if bit32.band(procEvent:GetAllEffectsMechanicMask(), SECOND_WIND_ROOT_STUN_MASK) == 0 then
        return true
    end
    local trigger = SECOND_WIND_TRIGGERS[aura:GetAuraId()]
    if trigger == nil then
        return true
    end
    local caster = procEvent:GetActionTarget()
    if caster == nil then
        return true
    end
    caster:CastSpell(caster, trigger, true)
    return true
end

RegisterAuraEvent(SPELL_WARRIOR_SECOND_WIND_R1, 6, secondWindProc)
RegisterAuraEvent(SPELL_WARRIOR_SECOND_WIND_R2, 6, secondWindProc)

local SPELL_WARRIOR_IMPROVED_SPELL_REFLECTION = 59088 -- spell_warrior.cpp:557 (class spell_warr_improved_spell_reflection)
local SPELL_WARRIOR_IMPROVED_SPELL_REFLECTION_R2 = 59089 -- bot_warrior_ai.cpp:93 (rank 2; NEG -59088 binding expands via the talent chain, Butchery precedent)
local SPELL_WARRIOR_IMPROVED_SPELL_REFLECTION_TRIGGER = 59725 -- spell_warrior.cpp:68

-- C++ spell_warr_improved_spell_reflection_AuraScript::HandleProc
-- (spell_warrior.cpp:572-579): OnEffectProc EFFECT_1 DUMMY, no
-- CheckProc. PreventDefaultAction (the ev6 true return); the proc
-- actor (the aura holder on the actor-side event the talent procs
-- on) self-casts the trigger (59725), triggered (CastSpellExtraArgs
-- (aurEff) sets TRIGGERED_FULL_MASK, SpellDefines.h:166), with
-- AddSpellMod(SPELLVALUE_MAX_TARGETS, aurEff->GetAmount()) — the
-- EFFECT_1 amount is the party-member cap, threaded by the new
-- player CastSpellWithMaxTargets bridge (lua_player.go; the player
-- funnel's pendingMaxTargets override, spells.go). The actor falls
-- back to the holder when the proc object did not resolve.
-- DORMANT until Spell.dbc ships the 59088/59725 rows (both unseeded;
-- seed axis parked).
local function improvedSpellReflectionProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= nil and effectIndex ~= 1 then
        return
    end
    local caster = actor
    if caster == nil then
        caster = target
    end
    if caster == nil then
        return true
    end
    caster:CastSpellWithMaxTargets(caster, SPELL_WARRIOR_IMPROVED_SPELL_REFLECTION_TRIGGER, true, aura:GetAmount(1))
    return true
end

RegisterAuraEvent(SPELL_WARRIOR_IMPROVED_SPELL_REFLECTION, 6, improvedSpellReflectionProc)
RegisterAuraEvent(SPELL_WARRIOR_IMPROVED_SPELL_REFLECTION_R2, 6, improvedSpellReflectionProc)


local SPELL_WARRIOR_DEEP_WOUNDS_R1 = 12162 -- spell_warrior.cpp:42 (class spell_warr_deep_wounds)
local SPELL_WARRIOR_DEEP_WOUNDS_R2 = 12850 -- spell_warrior.cpp:43
local SPELL_WARRIOR_DEEP_WOUNDS_R3 = 12868 -- spell_warrior.cpp:44
local SPELL_WARRIOR_DEEP_WOUNDS_PERIODIC = 12721 -- spell_warrior.cpp:45

-- C++ spell_warr_deep_wounds_SpellScript::HandleDummy (spell_warrior.cpp:287-304):
-- EFFECT_0 DUMMY OnEffectHitTarget. The aura leg passes the average weapon
-- damage as BP0 (spell_warr_deep_wounds_aura, :348-363); the total bleed is
-- 16% * rank of it (ApplyPct, :293 — CalculatePct is float-multiply then
-- truncate, the math.floor twin below), divided by the periodic spell's
-- tick count (GetMaxTicks, :297-298, ASSERTed > 0), then the caster casts
-- the periodic (12721) on the hit unit, triggered, BP0 = per-tick damage
-- (TRIGGERED_FULL_MASK, :300-302). Rank comes from the cast id
-- (GetSpellInfo()->GetRank(), :293) via the table below keyed by
-- spell:GetEntry() (the penance precedent, spell_priest.lua); TDB
-- spell_ranks verified 2026-11-19: (12162,12162,1), (12162,12850,2),
-- (12162,12868,3). The tick count reads via the new spell:GetSpellMaxTicks
-- bridge (lua_spell.go, both builders); a 0 read (DBC missing) skips the
-- cast rather than dividing by zero. A 0 effect value also skips: C++
-- casts unconditionally but only ever receives positive weapon damage from
-- its aura leg — a 0 read is the DBC dummy value, since the aura leg
-- (12834/12849/12867) is ported below (the new procEvent:GetAttackType
-- and player weapon-damage bridges) and the generic
-- DBC proc leg casts 12162 with no BP0 (procs.go:5646). DORMANT until
-- Spell.dbc ships the 12162/12721/12834 rows (all unseeded; seed axis
-- parked). RegisterSpellEvent OnEffectHitTarget = 5; the player-path fire
-- site is spells.go:14866-14934 (one fire per (effect, live hit target),
-- before the default); the creature-path twin is boss_ai.go.
local DEEP_WOUNDS_RANKS = {
    [SPELL_WARRIOR_DEEP_WOUNDS_R1] = 1,
    [SPELL_WARRIOR_DEEP_WOUNDS_R2] = 2,
    [SPELL_WARRIOR_DEEP_WOUNDS_R3] = 3,
}
local function deepWoundsHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or not hitTarget:IsAlive() then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local rank = DEEP_WOUNDS_RANKS[spell:GetEntry()]
    if rank == nil then
        return
    end
    local damage = spell:GetEffectValue(0)
    if damage <= 0 then
        return
    end
    local ticks = spell:GetSpellMaxTicks(SPELL_WARRIOR_DEEP_WOUNDS_PERIODIC)
    if ticks == 0 then
        return
    end
    local bp0 = math.floor(math.floor(damage * 16 * rank / 100) / ticks)
    caster:CastCustomSpell(hitTarget, SPELL_WARRIOR_DEEP_WOUNDS_PERIODIC, true, bp0)
end

RegisterSpellEvent(SPELL_WARRIOR_DEEP_WOUNDS_R1, 5, deepWoundsHit)
RegisterSpellEvent(SPELL_WARRIOR_DEEP_WOUNDS_R2, 5, deepWoundsHit)
RegisterSpellEvent(SPELL_WARRIOR_DEEP_WOUNDS_R3, 5, deepWoundsHit)


local SPELL_WARRIOR_DEEP_WOUNDS_AURA_R1 = 12834 -- spell_warrior.cpp:319 (class spell_warr_deep_wounds_aura); bot_warrior_ai.cpp:81-83 ranks 12834/12849/12867
local SPELL_WARRIOR_DEEP_WOUNDS_AURA_R2 = 12849
local SPELL_WARRIOR_DEEP_WOUNDS_AURA_R3 = 12867
local TYPEID_PLAYER = 4

-- C++ spell_warr_deep_wounds_aura_AuraScript (spell_warrior.cpp:324-370):
-- DoCheckProc (CheckProc, :331-345): the event must carry damage info
-- and the actor must be a player (the NPCBot arm is a bot-only fork,
-- not in this tree). DoCheckProc has no separate hook (runtime.go):
-- the port expresses it by returning true (the inverted veto) when
-- the gate fails. The damage-info leg rides the proc entry: the
-- generated entry (procs.go) gates on the DBC melee proc flags, and
-- every event reaching the hook carries damage info, so only the
-- actor-type gate is expressed here. OnEffectProc (OnProc, :348-363)
-- EFFECT_0 PROC_TRIGGER_SPELL: PreventDefaultAction (the ev6 true
-- return); the average weapon damage is the actor's off-hand
-- (min+max)/2 on OFF_ATTACK (procEvent:GetAttackType, 1) else the
-- main-hand pair (the new bridges: procEventInfo.attackType threaded
-- at the melee done-side site, procs.go; player GetMin/MaxDamage +
-- GetMin/MaxOffhandDamage, lua_player.go); the actor casts the rank's
-- trigger spell (12162/12850/12868, the spell_ranks chain of the
-- trigger, TDB verified 2026-11-19) on the proc target, triggered,
-- BP0 = the average weapon damage truncated to int32 (C++ AddSpellBP0
-- takes int32, :360-362). The aura ranks pair R1/R2/R3 with the
-- trigger ranks (12849/12850 and 12867/12868 are adjacent DBC ids).
-- DORMANT until Spell.dbc ships the 12834/12849/12867 rows (all
-- unseeded; seed axis parked).
local function makeDeepWoundsAuraProc(triggerSpell)
    return function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
        if effectIndex ~= nil and effectIndex ~= 0 then
            return
        end
        if actor == nil or actor:GetTypeId() ~= TYPEID_PLAYER then
            return true
        end
        if procTarget == nil or procEvent == nil then
            return true
        end
        local minDamage, maxDamage
        if procEvent:GetAttackType() == 1 then
            minDamage, maxDamage = actor:GetMinOffhandDamage(), actor:GetMaxOffhandDamage()
        else
            minDamage, maxDamage = actor:GetMinDamage(), actor:GetMaxDamage()
        end
        actor:CastCustomSpell(procTarget, triggerSpell, true, math.floor((minDamage + maxDamage) / 2))
        return true
    end
end

RegisterAuraEvent(SPELL_WARRIOR_DEEP_WOUNDS_AURA_R1, 6, makeDeepWoundsAuraProc(SPELL_WARRIOR_DEEP_WOUNDS_R1))
RegisterAuraEvent(SPELL_WARRIOR_DEEP_WOUNDS_AURA_R2, 6, makeDeepWoundsAuraProc(SPELL_WARRIOR_DEEP_WOUNDS_R2))
RegisterAuraEvent(SPELL_WARRIOR_DEEP_WOUNDS_AURA_R3, 6, makeDeepWoundsAuraProc(SPELL_WARRIOR_DEEP_WOUNDS_R3))


local SPELL_WARRIOR_CONCUSSION_BLOW = 12809 -- spell_warrior.cpp:200 (class spell_warr_concussion_blow)

-- C++ spell_warr_concussion_blow_SpellScript::HandleDummy
-- (spell_warrior.cpp:212-216): OnEffectLaunchTarget EFFECT_1
-- SCHOOL_DAMAGE. SetEffectValue(CalculatePct(GetCaster()->
-- GetTotalAttackPowerValue(BASE_ATTACK), EFFECT_2 CalcValue)) — the
-- launch damage is a percentage of total attack power, the percentage
-- read from the DBC effect-2 value via spell:GetEffectValue(2) (the
-- CalcValueForLevel twin, lua_spell.go); the rewrite rides
-- spell:SetEffectValue (event 10, runtime.go). DORMANT until Spell.dbc
-- ships the 12809 row (unseeded; seed axis parked).
local function concussionBlowLaunch(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local pct = spell:GetEffectValue(2)
    if pct <= 0 then
        return
    end
    spell:SetEffectValue(math.floor(caster:GetTotalAttackPowerValue(0) * pct / 100))
end

RegisterSpellEvent(SPELL_WARRIOR_CONCUSSION_BLOW, 10, concussionBlowLaunch)

local SPELL_WARRIOR_INTERVENE = 3411 -- spell_warrior.cpp:594 (class spell_warr_intervene, RegisterSpellScript)
local SPELL_WARRIOR_INTERVENE_THREAT = 59667 -- spell_warrior.cpp:76

-- C++ spell_warr_intervene::HandleThreat (spell_warrior.cpp:603-607):
-- OnEffectHitTarget EFFECT_0 CHARGE — the hit unit casts Intervene
-- Threat (59667) on itself, triggered. No PreventHitDefaultEffect:
-- the charge movement leg still runs. ev5 fires on the player
-- direct-cast path (spells.go:14892) and the creature path
-- (boss_ai.go). No Go twin: 59667 has no Go call site. TDB
-- spell_script_names binds 3411 (positive, R1-only; no spell_ranks
-- chain). DORMANT until Spell.dbc ships the 3411 row (unseeded;
-- seed axis parked).
local function interveneThreat(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_WARRIOR_INTERVENE_THREAT, true)
end

RegisterSpellEvent(SPELL_WARRIOR_INTERVENE, 5, interveneThreat)
