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
-- GetUnfriendlyUnitsInRange(5) (alive, unfriendly, self excluded) with
-- the victim, proc-target, LoS and totem/critter (GetCreatureType 11/8)
-- filters applied in Lua; spirit-service exclusion has no bridge
-- (negligible). If the proc spell is Bladestorm periodic whirlwind
-- (50622) or Execute (20647) while the second target is not under 20%
-- HP, the holder casts extra attack 2 (26654) on it, triggered; else
-- extra attack 1 (12723), triggered, BP0 = proc damage. The <20% gate
-- is the AURA_STATE_HEALTHLESS_20_PERCENT twin via GetHealthPct.
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
-- (12834) is a STAY-OUT (no procEvent attack-type bridge for the
-- off-hand/main-hand weapon-damage leg, census 2026-11-19) and the generic
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
