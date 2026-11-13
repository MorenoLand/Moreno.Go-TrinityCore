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
--   (spells.go:36257). Glyph of Vigilance (63326) folds in Go
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
--   ev7 fires on the creature-cast path only (boss_ai.go:3151); the
--   player-cast legs live in Go (spells.go area filters). The spell
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
-- ships the 58872 proc entry.
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
local SPELL_WARRIOR_BLOODSURGE_R1 = 46913 -- spell_warrior.cpp:39
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
-- handler is registered for. Higher ranks share the class; only the R1
-- ids are established in-tree. DORMANT until Spell.dbc ships the
-- 29723/46913 proc entries.
local function extraProc(gcdReduced)
    return function(event, aura, target, actor, procTarget, damage)
        if target == nil then
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
RegisterAuraEvent(SPELL_WARRIOR_BLOODSURGE_R1, 6, extraProc(SPELL_WARRIOR_SLAM_GCD_REDUCED))

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
-- skipChanneled=false, skipAutorepeat=true; skipInstant defaults false, so
-- ANY live non-delayed generic cast counts (instant included) plus any live
-- channeled cast; autorepeat is skipped. The Go twin is the live-slot read:
-- GetCurrentSpell(1) is non-nil only while a cast is unfinished (spells.go
-- clears activeCast on completion), GetCurrentSpell(2) while channeling.
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
local SPELL_WARRIOR_SLAM = 50783 -- spell_warrior.cpp:55

-- C++ spell_warr_slam_SpellScript::HandleDummy (spell_warrior.cpp:941-949):
-- OnEffectHitTarget EFFECT_0 DUMMY — the caster casts Slam (50783) on the
-- hit unit, triggered, BP0 = effect value. ev5 fires on the player
-- direct-cast path (spells.go:14892). Only the R1 id is established
-- in-tree; higher ranks share the class (extraProc precedent above).
-- DORMANT until Spell.dbc ships the records.
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

RegisterSpellEvent(SPELL_WARRIOR_SLAM_R1, 5, slamDummy)

local SPELL_WARRIOR_SWORD_AND_BOARD = 46951 -- spell_warrior.cpp:1021
local SPELL_CATEGORY_SHIELD_SLAM = 1209 -- spell_warrior.cpp:88

-- C++ spell_warr_sword_and_board_AuraScript::HandleProc
-- (spell_warrior.cpp:1031-1040): OnEffectProc EFFECT_0 PROC_TRIGGER_SPELL —
-- no PreventDefaultAction; resets the cooldowns of every spell whose DBC
-- category is Shield Slam (1209). ResetTypeCooldowns is the
-- SpellHistory::ResetCooldowns predicate bridge (lua_player.go:1962);
-- the update=true second arg is the vigilanceTrigger precedent above.
-- DORMANT until Spell.dbc ships the 46951 proc entry.
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

