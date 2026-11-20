-- Priest spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_priest.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2):
--   spell_pri_renew_AuraScript::HandleApplyEffect
--   (spell_priest.cpp:1238-1254): EFFECT_0 SPELL_AURA_PERIODIC_HEAL.
--   On apply, when the caster carries the Empowered Renew talent
--   aura (SPELLFAMILY_PRIEST 6, icon 3021, EFFECT_1), the caster
--   casts Empowered Renew 63544 on the target, triggered
--   (CastSpellExtraArgs(aurEff) = TRIGGERED_FULL_MASK,
--   SpellDefines.h:166), with BP0 =
--   CalculatePct(renew amount * total ticks, talent amount). The
--   C++ Load() gate (caster is a player, npcbot aside) is the
--   Player type check below. CalculatePct is T(base * pct / 100.0f)
--   (Util.h:66-69), the float-truncate math.floor twin below.
--   DELTA: C++ registers REAL_OR_REAPPLY, so a Renew refresh also
--   fires the arm; the Go apply hook is fresh-only (the tick-198
--   fungal-decay note), so a reapply casts nothing.

local SPELL_PRIEST_RENEW = 139 -- spell_priest.cpp:1220 (class spell_pri_renew)
local SPELL_PRIEST_RENEW_R2 = 6074
local SPELL_PRIEST_RENEW_R3 = 6075
local SPELL_PRIEST_RENEW_R4 = 6076
local SPELL_PRIEST_RENEW_R5 = 6077
local SPELL_PRIEST_RENEW_R6 = 6078
local SPELL_PRIEST_RENEW_R7 = 10927
local SPELL_PRIEST_RENEW_R8 = 10928
local SPELL_PRIEST_RENEW_R9 = 10929
local SPELL_PRIEST_RENEW_R10 = 25315
local SPELL_PRIEST_RENEW_R11 = 25221
local SPELL_PRIEST_RENEW_R12 = 25222
local SPELL_PRIEST_RENEW_R13 = 48067
local SPELL_PRIEST_RENEW_R14 = 48068
local SPELL_PRIEST_EMPOWERED_RENEW = 63544 -- spell_priest.cpp:41
local SPELLFAMILY_PRIEST = 6 -- spells.go:36319
local PRIEST_ICON_ID_EMPOWERED_RENEW_TALENT = 3021 -- spell_priest.cpp:83

local function renewApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local empoweredAmount = caster:GetDummyAuraEffect(SPELLFAMILY_PRIEST, PRIEST_ICON_ID_EMPOWERED_RENEW_TALENT, 1)
    if empoweredAmount == nil then
        return
    end
    local heal = aura:GetAmount(0) * aura:GetTotalTicks()
    local bp0 = math.floor(heal * empoweredAmount / 100)
    caster:CastCustomSpell(target, SPELL_PRIEST_EMPOWERED_RENEW, true, bp0)
end

RegisterAuraEvent(SPELL_PRIEST_RENEW, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R2, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R3, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R4, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R5, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R6, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R7, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R8, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R9, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R10, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R11, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R12, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R13, 2, renewApply)
RegisterAuraEvent(SPELL_PRIEST_RENEW_R14, 2, renewApply)

local SPELL_PRIEST_BLESSED_RECOVERY = 27811 -- spell_priest.cpp:188 (class spell_pri_blessed_recovery)
local SPELL_PRIEST_BLESSED_RECOVERY_R1 = 27813 -- spell_priest.cpp:39
local SPELL_PRIEST_IMP_SHADOWFORM = 47569 -- spell_priest.cpp:603 (class spell_pri_imp_shadowform)
local SPELL_PRIEST_IMP_SHADOWFORM_R2 = 47570 -- rank 2 (TDB binds 47570 to spell_pri_improved_shadowform; bot_priest_ai.cpp:87)

-- C++ spell_pri_blessed_recovery_AuraScript::HandleProc
-- (spell_priest.cpp:201-221): OnEffectProc, EFFECT_0 PROC_TRIGGER_SPELL.
-- PreventDefaultAction; no damage -> nothing. The action target (the
-- holder on a taken event, Unit.cpp:10412) casts the rank-matched
-- Blessed Recovery heal on itself, triggered, BP0 = pct(damage, aurEff
-- amount) / the trigger spell's max ticks (aura:GetSpellWithRank +
-- aura:GetSpellMaxTicks twins; imp_blizzard / righteous_vengeance
-- precedents). DORMANT until Spell.dbc ships the 27811 proc entry.
local function blessedRecoveryProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 or target == nil then
        return true
    end
    local triggerSpell = aura:GetSpellWithRank(SPELL_PRIEST_BLESSED_RECOVERY_R1)
    if triggerSpell == 0 then
        return true
    end
    local maxTicks = aura:GetSpellMaxTicks(triggerSpell)
    if maxTicks <= 0 then
        return true
    end
    local bp0 = math.floor(damage * aura:GetAmount(0) / 100 / maxTicks)
    target:CastCustomSpell(target, triggerSpell, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_PRIEST_BLESSED_RECOVERY, 6, blessedRecoveryProc)

-- C++ spell_pri_imp_shadowform_AuraScript::HandleProc
-- (spell_priest.cpp:612-619): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction; on a roll of the effect amount the proc actor
-- drops its movement-impairing auras (C++ GetActor: the attacker on a
-- taken event, Unit.cpp:10412, the holder on a done event,
-- Unit.cpp:10388; the Go ev6 actor arg is the same unit on both sides,
-- procs.go luaUnitObjectForGUID(ev.actorGUID)). The method is bridged
-- on both the player and creature-event surfaces. DORMANT until
-- Spell.dbc ships the 47569 proc entry.
local function impShadowformProc(event, aura, target, actor, procTarget, damage)
    if actor == nil then
        return true
    end
    if math.random(100) <= aura:GetAmount(0) then
        actor:RemoveMovementImpairingAuras(true)
    end
    return true
end

RegisterAuraEvent(SPELL_PRIEST_IMP_SHADOWFORM, 6, impShadowformProc)
RegisterAuraEvent(SPELL_PRIEST_IMP_SHADOWFORM_R2, 6, impShadowformProc)

local SPELL_PRIEST_LIGHTWELL_RENEW = 7001 -- spell_priest.cpp:788 (class spell_pri_lightwell_renew)
local SPELL_PRIEST_LIGHTWELL_RENEW_R2 = 27873
local SPELL_PRIEST_LIGHTWELL_RENEW_R3 = 27874
local SPELL_PRIEST_LIGHTWELL_RENEW_R4 = 28276
local SPELL_PRIEST_LIGHTWELL_RENEW_R5 = 48084
local SPELL_PRIEST_LIGHTWELL_RENEW_R6 = 48085
local SPELL_PRIEST_GLYPH_OF_LIGHTWELL = 55673 -- spell_priest.cpp:43
local AURA_REMOVE_BY_ENEMY_SPELL = 3 -- SpellAuraDefines.h:61-69
local LIGHTWELL_REMAINING = "remainingAmount"

-- C++ spell_pri_lightwell_renew_AuraScript::InitializeAmount
-- (spell_priest.cpp:805-808): AfterEffectApply, EFFECT_0
-- PERIODIC_HEAL — attacks equalling 30% of the target's total health
-- cancel the effect, so the latch starts at CountPctFromMaxHealth(30)
-- (CalculatePct float-truncate, the renewApply math.floor twin).
-- DELTA: C++ registers REAL_OR_REAPPLY, so a refresh resets the
-- latch; the Go OnApply hook is fresh-only (the renewApply note), so
-- a reapply keeps the old remaining amount.
local function lightwellRenewApply(event, aura, target)
    if target == nil then
        return
    end
    aura:SetScriptValue(LIGHTWELL_REMAINING, math.floor(target:GetMaxHealth() * 30 / 100))
end

RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW, 2, lightwellRenewApply)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R2, 2, lightwellRenewApply)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R3, 2, lightwellRenewApply)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R4, 2, lightwellRenewApply)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R5, 2, lightwellRenewApply)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R6, 2, lightwellRenewApply)

-- C++ spell_pri_lightwell_renew_AuraScript::HandleProc
-- (spell_priest.cpp:810-826): OnEffectProc, EFFECT_0 PERIODIC_HEAL —
-- no DamageInfo (a zero-damage event here, the fireAuraProc proxy)
-- returns; damage at or above the latch drops one charge with
-- AURA_REMOVE_BY_ENEMY_SPELL, else the latch falls by the damage.
-- No PreventDefaultAction in C++, so no true return: the EFFECT_0
-- default leg is a no-op (TriggerSpell 0). DORMANT until Spell.dbc
-- ships: the proc entry now comes from the spell_proc-row twin
-- (procs.go spellProcTableRows — PERIODIC_HEAL is no trigger aura,
-- SpellMgr.cpp:1686 isTriggerAura, so no generated entry exists)
-- and the melee taken sites carry PROC_FLAG_TAKEN_DAMAGE
-- (procs.go takenMeleeTypeMask, Unit.cpp:1430), but the entry's
-- Chance/Charges still ride the DBC spell and the aura itself needs
-- the DBC to exist. The CalculateAmount glyph arm (:797-804, AddPct
-- by the 55673 aura's EFFECT_0 amount) is PORTED below (ev11
-- lightwellRenewCalcAmount): the ev11 fire site exists at the apply
-- paths (lua_aura.go fireAuraCalcAmount).
local function lightwellRenewProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 then
        return
    end
    local remaining = aura:GetScriptValue(LIGHTWELL_REMAINING)
    if remaining <= damage then
        aura:DropCharge(AURA_REMOVE_BY_ENEMY_SPELL)
        return
    end
    aura:SetScriptValue(LIGHTWELL_REMAINING, remaining - damage)
end

RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW, 6, lightwellRenewProc)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R2, 6, lightwellRenewProc)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R3, 6, lightwellRenewProc)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R4, 6, lightwellRenewProc)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R5, 6, lightwellRenewProc)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R6, 6, lightwellRenewProc)

-- C++ spell_pri_lightwell_renew_AuraScript::CalculateAmount
-- (spell_priest.cpp:797-804): DoEffectCalcAmount, EFFECT_0
-- PERIODIC_HEAL — when the aura caster carries Glyph of Lightwell
-- 55673, AddPct the amount by the glyph's EFFECT_0 amount
-- (CalculatePct float-truncate, the math.floor twin below).
-- DELTA: C++ reads the glyph off the aura caster, which is the
-- priest (the 60123 cast passes the summoner GUID as original
-- caster); in Go the renew is attributed to the lightwell (the
-- lightwellHit note below), which never carries the priest's glyph,
-- so the glyph is read off the lightwell's summoner instead. A
-- direct priest-cast renew (caster is a Player) reads the glyph off
-- the caster itself. DORMANT until Spell.dbc ships the 7001 rows.
local function lightwellRenewCalcAmount(event, aura, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local glyph = caster:GetAuraEffect(SPELL_PRIEST_GLYPH_OF_LIGHTWELL, 0)
    if glyph == nil and caster:GetObjectType() == "Creature" then
        local summoner = caster:GetSummonerUnit()
        if summoner ~= nil then
            glyph = summoner:GetAuraEffect(SPELL_PRIEST_GLYPH_OF_LIGHTWELL, 0)
        end
    end
    if glyph == nil then
        return
    end
    local amount = aura:GetAmount(0)
    aura:SetAmount(math.floor(amount + amount * glyph / 100), 0)
end

RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW, 11, lightwellRenewCalcAmount)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R2, 11, lightwellRenewCalcAmount)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R3, 11, lightwellRenewCalcAmount)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R4, 11, lightwellRenewCalcAmount)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R5, 11, lightwellRenewCalcAmount)
RegisterAuraEvent(SPELL_PRIEST_LIGHTWELL_RENEW_R6, 11, lightwellRenewCalcAmount)

local SPELL_PRIEST_LIGHTWELL = 60123 -- spell_priest.cpp:719 (class spell_pri_lightwell)
local SPELL_PRIEST_LIGHTWELL_CHARGES = 59907 -- spell_priest.cpp:49
local LIGHTWELL_NPC_RENEWS = {
    [31897] = 7001, [31896] = 27873, [31895] = 27874,
    [31894] = 28276, [31893] = 48084, [31883] = 48085,
}

-- C++ spell_pri_lightwell::HandleScriptEffect (spell_priest.cpp:770-786):
-- OnEffectHitTarget, EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT. Load gate:
-- caster is TYPEID_UNIT; a non-summon caster returns. The caster's
-- entry maps to the Lightwell Renew rank (enum :71-94); when the
-- caster carries the Lightwell Charges aura 59907 it casts the mapped
-- renew on the hit unit, and ModCharges(-1) dropping the last charge
-- unsummons the lightwell. DELTAS: (a) C++ casts the renew with the
-- summoner GUID as original caster, so the renew aura's caster is the
-- priest; Go's CastSpell has no original-caster arg, so the renew is
-- attributed to the lightwell — healing credit goes to the lightwell,
-- and the renew's own Empowered Renew arm (caster must be a Player)
-- will not fire off a lightwell-cast renew, where C++ would proc it.
-- (b) The 59907 charges aura has no Go application site (zero Go
-- hits) and no TDB row applying it — its application rides the 724
-- Lightwell summon DBC, an open verification item; the handler
-- nil-guards it exactly as C++ does, so a missing charges aura casts
-- nothing rather than erroring. DORMANT until Spell.dbc ships the
-- 60123 row (and the 724 summon row applies 59907). Trigger path:
-- npc_spellclick_spells binds all six lightwell entries to 60123
-- with cast_flags (2,2); the Go creature cast routes through the
-- ev5 OnEffectHitTarget loop (boss_ai.go).
local function lightwellHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 4 then
        return
    end
    if not caster:IsSummon() then
        return
    end
    local renew = LIGHTWELL_NPC_RENEWS[caster:GetEntry()]
    if renew == nil then
        return
    end
    local charges = caster:GetAura(SPELL_PRIEST_LIGHTWELL_CHARGES)
    if charges == nil then
        return
    end
    caster:CastSpell(hitTarget, renew, true)
    if charges:ModCharges(-1) then
        caster:DespawnOrUnsummon()
    end
end

RegisterSpellEvent(SPELL_PRIEST_LIGHTWELL, 5, lightwellHit)

local SPELL_PRIEST_POWER_WORD_SHIELD = 17 -- spell_priest.cpp:1064 (class spell_pri_power_word_shield)
local SPELL_PRIEST_POWER_WORD_SHIELD_R2 = 592
local SPELL_PRIEST_POWER_WORD_SHIELD_R3 = 600
local SPELL_PRIEST_POWER_WORD_SHIELD_R4 = 3747
local SPELL_PRIEST_POWER_WORD_SHIELD_R5 = 6065
local SPELL_PRIEST_POWER_WORD_SHIELD_R6 = 6066
local SPELL_PRIEST_POWER_WORD_SHIELD_R7 = 10898
local SPELL_PRIEST_POWER_WORD_SHIELD_R8 = 10899
local SPELL_PRIEST_POWER_WORD_SHIELD_R9 = 10900
local SPELL_PRIEST_POWER_WORD_SHIELD_R10 = 10901
local SPELL_PRIEST_POWER_WORD_SHIELD_R11 = 25217
local SPELL_PRIEST_POWER_WORD_SHIELD_R12 = 25218
local SPELL_PRIEST_POWER_WORD_SHIELD_R13 = 48065
local SPELL_PRIEST_POWER_WORD_SHIELD_R14 = 48066
local SPELL_PRIEST_WEAKENED_SOUL = 6788 -- PW:S ExcludeTargetAuraSpell (Unit.cpp:7272 Renewed Hope HasAura gate; spell_stats.go:1563 twin)

-- C++ spell_pri_power_word_shield_SpellScript::WeakenSoul
-- (spell_priest.cpp:1079-1083): AfterHit, the caster casts the
-- spell's ExcludeTargetAuraSpell (Weakened Soul) on the hit unit,
-- triggered. All 14 ranks share the arm: TDB binds
-- (-17, 'spell_pri_power_word_shield'), the negative id meaning all
-- ranks (ObjectMgr.cpp:5790-5806), and the spell_ranks chain is
-- 592/600/3747/6065/6066/10898/10899/10900/10901/25217/25218/48065/
-- 48066 (R2-R14) — the CoH R2-R7 precedent. A missed hit fires
-- nil and casts nothing, like C++ GetHitUnit() nil.
local function powerWordShieldAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(hitTarget, SPELL_PRIEST_WEAKENED_SOUL, true)
end

RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R2, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R3, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R4, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R5, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R6, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R7, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R8, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R9, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R10, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R11, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R12, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R13, 3, powerWordShieldAfterHit)
RegisterSpellEvent(SPELL_PRIEST_POWER_WORD_SHIELD_R14, 3, powerWordShieldAfterHit)

local SPELL_PRIEST_POWER_INFUSION = 10060 -- spell_priest.cpp:1586 (class spell_pri_power_infusion)
local SPELL_MAGE_ARCANE_POWER = 12042 -- spell_priest.cpp:99
local SPELL_FAILED_AURA_BOUNCED = 9 -- SharedDefines.h:991
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_pri_power_infusion::CheckCast (spell_priest.cpp:1590-1597):
-- the target carrying Arcane Power (12042) fails the cast with
-- SPELL_FAILED_AURA_BOUNCED. The check is the script's only arm, so
-- this ports the whole script. RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast).
local function powerInfusionCheckCast(event, spell)
    local target = spell:GetTarget()
    if target ~= nil and target:HasAura(SPELL_MAGE_ARCANE_POWER) then
        return SPELL_FAILED_AURA_BOUNCED
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_PRIEST_POWER_INFUSION, 8, powerInfusionCheckCast)

local SPELL_PRIEST_DIVINE_AEGIS_TALENT = 47509 -- spell_priest.cpp:336
local SPELL_PRIEST_DIVINE_AEGIS = 47753 -- spell_priest.cpp:40

-- C++ spell_pri_divine_aegis::CheckProc + HandleProc (spell_priest.cpp:351-379):
-- EFFECT_0 SPELL_AURA_DUMMY OnEffectProc. CheckProc passes only when
-- the proc target exists; HandleProc PreventDefaultActions (the true
-- return on every path) and casts the absorb 47753 on the proc target
-- with BP0 = pct(heal, aurEff amount) + the existing aegis amount,
-- capped at procTarget level * 125. The heal rides the forwarded proc
-- event (procEventObject, lua_aura.go: GetHeal); the existing-aegis
-- stack reads via procTarget:GetAuraEffect(47753, 0) (nil = none);
-- the holder (C++ GetTarget) casts via the target arg. DORMANT until
-- Spell.dbc ships the 47509 proc entry.
local function divineAegisProc(event, aura, target, actor, procTarget, damage, procEvent)
    if procTarget == nil or aura == nil or target == nil or procEvent == nil then
        return true
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    local absorb = math.floor(heal * aura:GetAmount(0) / 100)
    local existing = procTarget:GetAuraEffect(SPELL_PRIEST_DIVINE_AEGIS, 0)
    if existing ~= nil then
        absorb = absorb + existing
    end
    local cap = procTarget:GetLevel() * 125
    if absorb > cap then
        absorb = cap
    end
    target:CastCustomSpell(procTarget, SPELL_PRIEST_DIVINE_AEGIS, true, absorb)
    return true
end

RegisterAuraEvent(SPELL_PRIEST_DIVINE_AEGIS_TALENT, 6, divineAegisProc)

local SPELL_PRIEST_CIRCLE_OF_HEALING = 34861 -- spell_priest.cpp:296 (class spell_pri_circle_of_healing)
local SPELL_PRIEST_CIRCLE_OF_HEALING_R2 = 34863
local SPELL_PRIEST_CIRCLE_OF_HEALING_R3 = 34864
local SPELL_PRIEST_CIRCLE_OF_HEALING_R4 = 34865
local SPELL_PRIEST_CIRCLE_OF_HEALING_R5 = 34866
local SPELL_PRIEST_CIRCLE_OF_HEALING_R6 = 48088
local SPELL_PRIEST_CIRCLE_OF_HEALING_R7 = 48089
local SPELL_PRIEST_GLYPH_OF_CIRCLE_OF_HEALING = 55675 -- spell_priest.cpp:42

-- C++ spell_pri_circle_of_healing_SpellScript::FilterTargets
-- (spell_priest.cpp:310-320): EFFECT_0 TARGET_UNIT_DEST_AREA_ALLY.
-- RaidCheck drops every target not in the caster's raid
-- (caster:IsInRaidWith, Unit.cpp:12155), then the list is sorted
-- lowest-health-pct first (HealthPctOrderPred, ascending) and cut to
-- 5, or 6 with the Glyph of Circle of Healing aura (55675). The Lua
-- list has no sort/resize, so the sort runs in Lua over GetHealthPct
-- (the HealthPctOrderPred ratio twin: health*100/maxHealth, 0 at
-- maxHealth 0) and the cut is Clear + Add (murmur precedent).
-- DELTA: Lua table.sort is not stable, so equal-pct targets at the
-- cut boundary may keep a different one than C++ list::sort; the
-- kept set is identical whenever no pct ties straddle the cut.
-- DORMANT until Spell.dbc ships the 34861 row. All seven ranks share
-- the filter: TDB binds (-34861, spell_pri_circle_of_healing), the
-- negative id meaning all ranks (ObjectMgr.cpp:5790-5806), and the
-- spell_ranks chain is 34863/34864/34865/34866/48088/48089 (R2-R7) —
-- the SoR precedent (20154/21084 both registered, TDB binds -20154
-- only).
-- RANKS: TDB spell_script_names binds (-34861, 'spell_pri_circle_of_healing')
-- and ObjectMgr.cpp LoadSpellScriptNames expands a negative id to every rank
-- of the chain (GetNextRankSpell); TDB spell_ranks confirms the chain
-- 34861/34863/34864/34865/34866/48088/48089 (R1-R7). The Go ev7 fire site
-- resolves handlers by exact spell id only (lua_spell.go HasHook), so each
-- rank registers the same handler (the SoR R1/R2 precedent).
local function circleOfHealingFilterTargets(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local kept = {}
    for _, target in ipairs(targets:GetTargets()) do
        if caster:IsInRaidWith(target) then
            kept[#kept + 1] = target
        end
    end
    table.sort(kept, function(a, b) return a:GetHealthPct() < b:GetHealthPct() end)
    local maxTargets = 5
    if caster:HasAura(SPELL_PRIEST_GLYPH_OF_CIRCLE_OF_HEALING) then
        maxTargets = 6
    end
    targets:Clear()
    for i = 1, math.min(maxTargets, #kept) do
        targets:Add(kept[i])
    end
end

RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING, 7, circleOfHealingFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING_R2, 7, circleOfHealingFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING_R3, 7, circleOfHealingFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING_R4, 7, circleOfHealingFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING_R5, 7, circleOfHealingFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING_R6, 7, circleOfHealingFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_CIRCLE_OF_HEALING_R7, 7, circleOfHealingFilterTargets)

local SPELL_PRIEST_MIND_SEAR = 49821 -- spell_priest.cpp:889 (class spell_pri_mind_sear)
local SPELL_PRIEST_MIND_SEAR_R2 = 53022

-- C++ spell_pri_mind_sear_SpellScript::FilterTargets
-- (spell_priest.cpp:898-901): EFFECT_0 TARGET_UNIT_DEST_AREA_ENEMY.
-- Removes the caster's channel object (GetChannelObjectGuid,
-- UNIT_FIELD_CHANNEL_OBJECT, Unit.h:1359) from the area list.
-- ORDERING QUIRK (faithful, not a bug): C++ selects targets in
-- Spell::_cast (Spell.cpp:3410) before handle_immediate runs
-- SendChannelStart (Spell.cpp:3585), so the field read here is the
-- caster's PREVIOUS channel object (cleared at channel end,
-- Spell.cpp:4650) — a fresh cast reads empty and removes nothing, so
-- the primary target stays in the area list and takes the periodic
-- damage. The Go fire site runs at the same point (finishSpellCast,
-- before startChannel), so GetChannelObject reads the same pre-cast
-- state. R2 53022 registered (TDB spell_ranks + (-49821) all-ranks
-- binding; handler is rank-agnostic). DORMANT until Spell.dbc ships
-- the 49821 row.
local function mindSearFilterTargets(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local channelObject = caster:GetChannelObject()
    if channelObject ~= nil then
        targets:Remove(channelObject)
    end
end

RegisterSpellEvent(SPELL_PRIEST_MIND_SEAR, 7, mindSearFilterTargets)
RegisterSpellEvent(SPELL_PRIEST_MIND_SEAR_R2, 7, mindSearFilterTargets)

local SPELL_PRIEST_IMPROVED_SPIRIT_TAP = 15337 -- spell_priest.cpp:632
local SPELL_PRIEST_SHADOW_WORD_DEATH_R1 = 32379 -- spell_priest.cpp:69
local SPELL_PRIEST_MIND_BLAST_R1 = 8092 -- spell_priest.cpp:68
local SPELL_PRIEST_MIND_FLAY_DAMAGE = 58381 -- spell_priest.cpp:70

-- C++ spell_pri_improved_spirit_tap::CheckProc (spell_priest.cpp:650-662):
-- DoCheckProc only, gates the default trigger: pass when the event
-- spell is a rank of Shadow Word: Death (32379) or Mind Blast (8092)
-- (procEvent:IsRankOf, lua_aura.go — the spell_ranks world table; with
-- no rank row it is true only for the R1 id itself), or Mind Flay
-- damage (58381) at 50% (roll_chance_i twin: math.random(0, 99) < 50,
-- the spell_generic idiom); every other event spell vetoes (the true
-- return = PreventDefaultAction, which skips the default trigger leg
-- for this effect). A nil event spell vetoes, like the C++ nil
-- GetSpellInfo() path. The ev6 hook fires after charge/cooldown
-- consumption, so a veto never refunds a spent charge — the known ev6
-- CheckProc approximation (lua_aura.go fireAuraProc). DORMANT until
-- Spell.dbc ships the 15337 proc entry.
local function improvedSpiritTapCheckProc(event, aura, target, actor, procTarget, damage, procEvent)
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if procEvent:IsRankOf(SPELL_PRIEST_SHADOW_WORD_DEATH_R1) or procEvent:IsRankOf(SPELL_PRIEST_MIND_BLAST_R1) then
        return
    end
    if procEvent:GetSpellId() == SPELL_PRIEST_MIND_FLAY_DAMAGE then
        if math.random(0, 99) < 50 then
            return
        end
        return true
    end
    return true
end

RegisterAuraEvent(SPELL_PRIEST_IMPROVED_SPIRIT_TAP, 6, improvedSpiritTapCheckProc)

-- TRIAGED STAY OUT (tick 2026-11-11 18:32, census rows 662-664 + 666-670):
--   662 body_and_soul (64127, :236-295): PORTED IN GO this tick
--   (procs.go per-effect legs, session loop only — a priest talent):
--   EFFECT_0 PROC_TRIGGER_SPELL vetoes unless the event spell carries
--   family flag [0] 0x1 (Power Word: Shield), else falls through to the
--   generic trigger leg; EFFECT_1 DUMMY casts 64136 on the caster only
--   on self-cast Abolish Disease 552 (actor == proc target) + the
--   EFFECT_1 amount roll. No Lua ev6 twin: ev6 DOES pass effectIndex
--   (lua_aura.go:1303, 8th handler arg), so per-effect gating is
--   possible — but a Lua port would double-fire with the Go legs, so
--   the arm stays Go-only.
--   663 circle_of_healing (34861, :296-335): PORTED 2026-11-19 (ev7
--   above): RaidCheck via the new IsInRaidWith bridge (both Lua unit
--   surfaces, Unit.cpp:12155), lowest-health sort + 5/6 glyph cap via
--   GetTargets/Clear/Add; the "no OnObjectAreaTargetSelect bridge"
--   note is STALE (ev7 landed bf31db6b). R2-R7 registered 2026-11-19
--   (34863/34864/34865/34866/48088/48089, TDB spell_ranks chain +
--   negative spell_script_names all-ranks expansion, ObjectMgr.cpp).
--   DORMANT until Spell.dbc ships the 34861 row.
--   664 divine_aegis (47509, :336-389): PORTED this tick (ev6, heal
--   forwarding: pct(heal, amount) + existing-aegis stack +
--   level*125 cap, holder casts 47753 on the proc target).
--   665 lightwell (60123, :719-786): PORTED 2026-11-20 (ev5
--   lightwellHit above): entry->renew map (31897->7001 ... 31883->
--   48085), 59907 charges gate, ModCharges(-1) -> DespawnOrUnsummon;
--   npc_spellclick_spells binds all six entries to 60123 (2,2).
--   Deltas: renew attributed to the lightwell (no original-caster arg
--   on CastSpell), 59907 has no Go application site (rides the 724
--   summon DBC). DORMANT until Spell.dbc ships the 60123 row.
--   666 improved_spirit_tap (15337, :632-676): PORTED this tick (ev6,
--   above): rank of SW:D 32379 / Mind Blast 8092 passes via the new
--   procEvent:IsRankOf bridge, Mind Flay 58381 at 50%, all else vetoes.
--   667 lightwell_renew (7001, :788-846): PORTED (ev2 InitializeAmount
--   latch + ev6 HandleProc DropCharge, above): the DropCharge/
--   ModCharges bridge + per-aura ScriptValues latch landed in
--   lua_aura.go (GetCharges/SetCharges/ModCharges/DropCharge +
--   GetScriptValue/SetScriptValue, 16bb3d16); the spell_proc-row twin
--   (procs.go spellProcTableRows, all six ranks) + the melee
--   TAKEN_DAMAGE flag landed this tick. R2-R6 registered 2026-11-19
--   (27873/27874/28276/48084/48085, TDB spell_ranks chain + (-7001)
--   binding; both handlers rank-agnostic). DORMANT until Spell.dbc
--   ships (entry Chance/Charges ride the DBC spell). The
--   CalculateAmount glyph-bonus arm (55673) is PORTED (ev11
--   lightwellRenewCalcAmount above): the ev11 fire site exists at the
--   apply paths (lua_aura.go fireAuraCalcAmount).
--   668 mind_sear (49821, :889-915): PORTED 2026-11-19 (ev7 above):
--   drop the caster's channel object from the EFFECT_0 area list via
--   the new GetChannelObject bridge (both Lua unit surfaces); the
--   C++ selection-before-channel-start ordering quirk is faithful
--   (fresh casts read an empty field and remove nothing). R2 53022
--   registered 2026-11-19 (TDB spell_ranks + (-49821) binding;
--   handler is rank-agnostic). DORMANT
--   until Spell.dbc ships the 49821 row.
--   669 pain_and_suffering_dummy (47580, :916-942): DoCheckEffectProc
--   returning false (pure proc gate, no arm); ev6 has no CheckProc
--   model and its hook fires after charge/cooldown consumption.
--   670 penance (47540, :980-1064): HandleDummy PORTED tick 2026-11-12,
--   ev5 R2-R4 registered 2026-11-19 (rank table keyed by
--   spell:GetEntry(); TDB spell_ranks 47757->52986/52987/52988 heal,
--   47758->53001/53002/53003 damage; C++ GetSpellWithRank(rank)
--   :1024-1029); the CheckCast arm is PORTED (OnCheckCast ev8 above,
--   R1-R4). The "ev5 player path is dormant" claim is STALE (a910c39d,
--   spells.go:14668-14737).
--   Census hooks wrong on 661/662/664/666/667/668/669/670 (phantom
--   OnObjectAreaTargetSelect / HandleDummy / OnCheckCast / OnEffectHitTarget;
--   real hooks are in the notes above).
--   671 power_word_shield (17, :1065-1182): WeakenSoul PORTED above
--   (AfterHit ev3); R2-R14 registered 2026-11-19 (592/600/3747/6065/
--   6066/10898/10899/10900/10901/25217/25218/48065/48066, TDB
--   spell_ranks chain + (-17) binding; handler rank-agnostic).
--   CalculateAmount (:1109-1151) stays out, but the gap is no longer
--   the hook: the ev11 DoEffectCalcAmount fire site exists now
--   (lua_aura.go fireAuraCalcAmount) — the arm needs the
--   spellpower-model bridges (SpellBaseHealingBonusDone /
--   ApplyEffectModifiers / level-penalty coefficient), none of which
--   exists on any Lua surface. ReflectDamage (:1153-1169) has no
--   AfterEffectAbsorb bridge (no absorb event; the DamageInfo
--   attacker/spell read has no hook). Census go_hit is Renewed Hope
--   (spell_stats.go:1563 hasAura 6788); lua_hit is the Whitemane
--   boss cast (22187), not this class.
--   672 renew (139, :1220-1265): HandleApplyEffect verified
--   name-by-name (:1237-1254); the Empowered Renew cast is
--   triggered (aurEff args, SpellDefines.h:166), fixed this tick
--   (CastCustomSpell false -> true; player funnel ignores the flag
--   but the creature/NPCBot path honors it). Reapply delta stands.
--   R2-R14 registered 2026-11-19 (6074/6075/6076/6077/6078/10927/
--   10928/10929/25315/25221/25222/48067/48068, TDB spell_ranks chain
--   + (-139) binding; handler rank-agnostic).

-- CLOSED IN GO + TRIAGED STAY OUT (tick 2026-11-11 18:46, census rows
-- 224-229 header-source + 672-673 comment-block; spell_priest.cpp end):
--   prayer_of_mending_heal (33110, :1185-1219): CLOSED IN GO
--   (spells.go:32000-32017: T9 2P 67201 EFFECT_0 pct on the hit heal,
--   post-chain pre-crit). Real hook OnEffectHitTarget EFFECT_0
--   SPELL_EFFECT_HEAL only; census phantom HandleDamage/OnEffectApply/OnHit.
--   shadowfiend_death (57989, :1268-1315): CLOSED IN GO (boss_ai.go
--   creature leg :11268-11297: owner casts 58227 on self when the
--   shadowfiend sits below 1% after the damage). Real hooks DoCheckProc
--   + OnEffectProc EFFECT_0 DUMMY; census phantom HandleDamage/OnHit.
--   shadow_word_death (32379, :1316-1350): STAY OUT. OnHit HandleDamage
--   needs GetHitDamage (Pain and Suffering icon 2874 EFFECT_1 pct, then
--   caster casts 32409 on self triggered BP0=damage). CORRECTION tick
--   2026-11-12: the player path DOES thread hit damage now (spells.go:14637
--   + :14668, a910c39d) — but C++ OnHit itself reads m_damage = 0: the
--   damage is still TargetInfo.Damage = 0 (Spell.h:681) at PreprocessTarget
--   (Spell.cpp:2344 sets m_damage before CallScriptOnHitHandlers at :2377;
--   the effects run after, DoProcessTargetContainer :3554-3565), so a
--   faithful ev2 port casts a 0-BP0 backlash, and an ev3 AfterHit port
--   would read post-mitigation damage (spells.go:19795-19805) C++ never
--   sees at OnHit. No Go damage-path twin exists (only 32409 aura-dispel
--   carve-outs: boss_ai.go:4484, procs.go:4526, spells.go:7429/46357).
--   Real hook OnHit only; census phantom HandleDamage.
--   vampiric_embrace (15286, :1351-1392): CLOSED IN GO (procs.go:5261-5280
--   + boss_ai.go:11307-11322: actor casts 15290 on null, BP0=selfHeal/5,
--   BP1=selfHeal). Real hook OnEffectProc EFFECT_0 DUMMY.
--   vampiric_touch (34914, :1393-1458): PARTIAL. HandleDispel arm is Go
--   (dispel.go:1781-1805: 64085 backlash, 8 * EFFECT_1 amount; OPEN
--   DEVIATION: the pre-multiply SpellDamageBonusTaken fold (C++
--   :1428-1430, VT spell/DOT type) is missing — the pipeline applies
--   taken once post-multiply (spells.go:23965, 64085-record-gated),
--   same tolerated order-residual class as UA (dispel.go:1732-1737)).
--   The Replenishment arm (OnEffectProc EFFECT_2, actor casts 57669 on
--   null) is Go (procs.go specific leg, 34914 i==2, added this tick);
--   NO Lua port: ev6 DOES pass effectIndex (lua_aura.go:1303), but a
--   Lua port would double-fire with the Go leg; the EFFECT_0
--   CheckDummy gate has no Lua model. Real hooks DoCheckEffectProc
--   EFFECT_0 + AfterDispel + OnEffectProc EFFECT_2; census hooks empty.
--   t3_4p_bonus (28809, :1459-1491): CLOSED IN GO (procs.go:5285-5301 +
--   boss_ai.go:11324-11331: actor casts 28810 on the proc target). Real
--   hook OnEffectProc EFFECT_0 DUMMY; census phantom OnCheckCast.
--   t5_heal_2p_bonus (37594, :1492-1536): CLOSED IN GO (procs.go:5302-5328
--   + boss_ai.go:11332-11353: full-health overheal gate, holder casts
--   37595 on self). Real hooks DoCheckProc + OnEffectProc EFFECT_0
--   PROC_TRIGGER_SPELL; census phantom OnCheckCast.
--   t10_heal_2p_bonus (70770, :1537-1585): CLOSED IN GO (procs.go:5329-5368
--   + boss_ai.go:11387-11424: actor casts 70772 on the proc target,
--   BP0=pct(heal, amount)/70772 max ticks). Real hook OnEffectProc
--   EFFECT_0 DUMMY; census phantom OnCheckCast.
--   power_infusion (10060, :1586-1604): PORTED this tick (OnCheckCast
--   ev8 above: target with Arcane Power 12042 -> SPELL_FAILED_AURA_BOUNCED).
--   Census rows 224-229 + 672-673 now all closed in Go except the
--   SW:D leg noted above; the next unworked census row is 692
--   (spell_q10041_q10040_who_are_they, spell_quest.cpp) — rows
--   675-686 + 690 (1-based) all closed this tick, 691 relief_for_
--   the_fallen closed in fa9c8507; the 9293c104 note calling 674
--   "q55" is off by one (674 is vampiric_touch). 692 stays out on
--   an unresolvable binding ID (no spell-ID comment in the file;
--   loader registered via spell_script_names DB); then q12805/
--   q14112/q9452 per the quest-file NEXT note.

local SPELL_PRIEST_PENANCE_R1 = 47540 -- spell_priest.cpp:49 (class spell_pri_penance)
local SPELL_PRIEST_PENANCE_R2 = 53005
local SPELL_PRIEST_PENANCE_R3 = 53006
local SPELL_PRIEST_PENANCE_R4 = 53007
local SPELL_PRIEST_PENANCE_R1_HEAL = 47757 -- spell_priest.cpp:51
local SPELL_PRIEST_PENANCE_R1_DAMAGE = 47758 -- spell_priest.cpp:50
local SPELL_PRIEST_PENANCE_R2_HEAL = 52986 -- spell_ranks (47757, 52986, 2)
local SPELL_PRIEST_PENANCE_R2_DAMAGE = 53001 -- spell_ranks (47758, 53001, 2)
local SPELL_PRIEST_PENANCE_R3_HEAL = 52987 -- spell_ranks (47757, 52987, 3)
local SPELL_PRIEST_PENANCE_R3_DAMAGE = 53002 -- spell_ranks (47758, 53002, 3)
local SPELL_PRIEST_PENANCE_R4_HEAL = 52988 -- spell_ranks (47757, 52988, 4)
local SPELL_PRIEST_PENANCE_R4_DAMAGE = 53003 -- spell_ranks (47758, 53003, 4)
local SPELL_FAILED_BAD_TARGETS = 12 -- SharedDefines.h:994
local SPELL_FAILED_UNIT_NOT_INFRONT = 134 -- SharedDefines.h:1116

-- C++ spell_pri_penance_SpellScript::CheckCast (spell_priest.cpp:1033-1049):
-- a nil explicit target passes (no else arm — unlike holy shock); a
-- hostile target must pass IsValidAttackTarget (else BAD_TARGETS) and be
-- in front (else UNIT_NOT_INFRONT); friendly targets pass. Check-only
-- port: the HandleDummy legs ride ev5 (dormant, no player path) +
-- spell-level GetSpellWithRank (47758/47757). R2-R4 53005/53006/53007
-- registered 2026-11-19 (TDB spell_ranks + (-47540) all-ranks binding;
-- CheckCast is rank-agnostic, no id-specific logic). The C++ NPCBot caster
-- exception (caster forced to Player*) has no Go twin — the ev8 fire
-- site is the player cast path only (spells.go). RegisterSpellEvent
-- OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
local function penanceCheckCast(event, spell)
    local caster = spell:GetCaster()
    local target = spell:GetTarget()
    if caster ~= nil and target ~= nil and not caster:IsFriendlyTo(target) then
        if not caster:IsValidAttackTarget(target) then
            return SPELL_FAILED_BAD_TARGETS
        end
        if not caster:IsInFront(target) then
            return SPELL_FAILED_UNIT_NOT_INFRONT
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_PRIEST_PENANCE_R1, 8, penanceCheckCast)
RegisterSpellEvent(SPELL_PRIEST_PENANCE_R2, 8, penanceCheckCast)
RegisterSpellEvent(SPELL_PRIEST_PENANCE_R3, 8, penanceCheckCast)
RegisterSpellEvent(SPELL_PRIEST_PENANCE_R4, 8, penanceCheckCast)

-- C++ spell_pri_penance_SpellScript::HandleDummy (spell_priest.cpp:1016-1031):
-- EFFECT_0 DUMMY OnEffectHitTarget. A dead hit unit casts nothing; a
-- friendly hit unit gets the rank-matched heal, a hostile one the
-- rank-matched damage, both TRIGGERED_DISALLOW_PROC_EVENTS (a triggered
-- cast — the true flag; the player path ignores it, see the quest-file
-- header note). Rank comes from the cast id (GetSpellInfo()->GetRank(),
-- :1024) via GetSpellWithRank on the 47757 heal / 47758 damage bases;
-- the Lua twin is the rank table below keyed by spell:GetEntry()
-- (lua_spell.go:191/:514, both spell-object builders), TDB spell_ranks
-- chains verified 2026-11-19 (47757->52986/52987/52988,
-- 47758->53001/53002/53003; (-47540) all-ranks binding). The C++ NPCBot
-- caster exception has no Go twin — the Load gate is the player-type
-- check below (ev5 fires on the creature path too, boss_ai.go:3435-3438).
-- RegisterSpellEvent OnEffectHitTarget = 5; the player-path fire site is
-- spells.go:14668-14737 (a910c39d, one fire per (effect, live hit target),
-- before the default).
local PENANCE_TRIGGERS = {
    [SPELL_PRIEST_PENANCE_R1] = { heal = SPELL_PRIEST_PENANCE_R1_HEAL, damage = SPELL_PRIEST_PENANCE_R1_DAMAGE },
    [SPELL_PRIEST_PENANCE_R2] = { heal = SPELL_PRIEST_PENANCE_R2_HEAL, damage = SPELL_PRIEST_PENANCE_R2_DAMAGE },
    [SPELL_PRIEST_PENANCE_R3] = { heal = SPELL_PRIEST_PENANCE_R3_HEAL, damage = SPELL_PRIEST_PENANCE_R3_DAMAGE },
    [SPELL_PRIEST_PENANCE_R4] = { heal = SPELL_PRIEST_PENANCE_R4_HEAL, damage = SPELL_PRIEST_PENANCE_R4_DAMAGE },
}

local function penanceHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or not hitTarget:IsAlive() then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 4 then
        return
    end
    local trigger = PENANCE_TRIGGERS[spell:GetEntry()]
    if trigger == nil then
        return
    end
    if caster:IsFriendlyTo(hitTarget) then
        caster:CastSpell(hitTarget, trigger.heal, true)
    else
        caster:CastSpell(hitTarget, trigger.damage, true)
    end
end

RegisterSpellEvent(SPELL_PRIEST_PENANCE_R1, 5, penanceHit)
RegisterSpellEvent(SPELL_PRIEST_PENANCE_R2, 5, penanceHit)
RegisterSpellEvent(SPELL_PRIEST_PENANCE_R3, 5, penanceHit)
RegisterSpellEvent(SPELL_PRIEST_PENANCE_R4, 5, penanceHit)
