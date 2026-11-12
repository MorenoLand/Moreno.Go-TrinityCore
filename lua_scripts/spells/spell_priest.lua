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

local SPELL_PRIEST_BLESSED_RECOVERY = 27811 -- spell_priest.cpp:188 (class spell_pri_blessed_recovery)
local SPELL_PRIEST_BLESSED_RECOVERY_R1 = 27813 -- spell_priest.cpp:39
local SPELL_PRIEST_IMP_SHADOWFORM = 47569 -- spell_priest.cpp:603 (class spell_pri_imp_shadowform)

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

local SPELL_PRIEST_POWER_WORD_SHIELD = 17 -- spell_priest.cpp:1064 (class spell_pri_power_word_shield)
local SPELL_PRIEST_WEAKENED_SOUL = 6788 -- PW:S ExcludeTargetAuraSpell (Unit.cpp:7272 Renewed Hope HasAura gate; spell_stats.go:1563 twin)

-- C++ spell_pri_power_word_shield_SpellScript::WeakenSoul
-- (spell_priest.cpp:1079-1083): AfterHit, the caster casts the
-- spell's ExcludeTargetAuraSpell (Weakened Soul) on the hit unit,
-- triggered. Rank 1 only: the C++ loader binds all PW:S ranks, Lua
-- binds per spell id (the renew 139 precedent). A missed hit fires
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

-- TRIAGED STAY OUT (tick 2026-11-11 18:32, census rows 662-664 + 666-670):
--   662 body_and_soul (64127, :236-295): both arms need the event spell
--   (EFFECT_0 PROC_TRIGGER_SPELL vetoes unless family flags [0] & 0x1
--   Power Word: Shield; EFFECT_1 DUMMY casts 64136 only on self-cast
--   Abolish Disease 552 + roll); ev6 carries no event spell.
--   663 circle_of_healing (34861, :296-335): SpellScript target filter
--   (raid check, glyph 6 vs 5 targets, lowest-health sort); no
--   OnObjectAreaTargetSelect bridge.
--   664 divine_aegis (47509, :336-389): HandleProc needs heal info
--   (pct(heal, amount) + existing-aegis stack + level*125 cap); ev6
--   carries damage only (heal-info forwarding is open engine work).
--   666 improved_spirit_tap (15337, :632-676): DoCheckProc only, gates
--   the default trigger on the event spell (rank of SW:D 32379 / Mind
--   Blast 8092, or Mind Flay 58381 at 50%); ev6 carries no event spell.
--   667 lightwell_renew (7001, :788-846): CalculateAmount glyph bonus
--   (55673) unbridged; HandleProc DropCharge unbridged (no aura charge
--   bridge); the per-aura _remainingAmount latch has no per-aura key.
--   668 mind_sear (49821, :889-915): SpellScript target filter (drop
--   the channel object from the area list); no bridge.
--   669 pain_and_suffering_dummy (47580, :916-942): DoCheckEffectProc
--   returning false (pure proc gate, no arm); ev6 has no CheckProc
--   model and its hook fires after charge/cooldown consumption.
--   670 penance (47540, :980-1064): HandleDummy rides ev5 (spell hit)
--   + spell-level GetSpellWithRank (47758/47757) + IsFriendlyTo; the
--   CheckCast arm is PORTED this tick (OnCheckCast ev8 below, R1 only)
--   and the ev5 player path is dormant.
--   Census hooks wrong on 661/662/664/666/667/668/669/670 (phantom
--   OnObjectAreaTargetSelect / HandleDummy / OnCheckCast / OnEffectHitTarget;
--   real hooks are in the notes above).
--   671 power_word_shield (17, :1065-1182): WeakenSoul PORTED above
--   (AfterHit ev3); CalculateAmount (:1109-1151) has no
--   DoEffectCalcAmount bridge (spellpower/dampening/ICC math
--   unbridged) and ReflectDamage (:1153-1169) has no
--   AfterEffectAbsorb bridge (no absorb event; the DamageInfo
--   attacker/spell read has no hook). Census go_hit is Renewed Hope
--   (spell_stats.go:1563 hasAura 6788); lua_hit is the Whitemane
--   boss cast (22187), not this class.
--   672 renew (139, :1220-1265): HandleApplyEffect verified
--   name-by-name (:1237-1254); the Empowered Renew cast is
--   triggered (aurEff args, SpellDefines.h:166), fixed this tick
--   (CastCustomSpell false -> true; player funnel ignores the flag
--   but the creature/NPCBot path honors it). Reapply delta stands.

-- CLOSED IN GO + TRIAGED STAY OUT (tick 2026-11-11 18:46, census rows
-- 224-229 header-source + 672-673 comment-block; spell_priest.cpp end):
--   prayer_of_mending_heal (33110, :1185-1219): CLOSED IN GO
--   (spells.go:31449-31471: T9 2P 67201 EFFECT_0 pct on the hit heal,
--   post-chain pre-crit). Real hook OnEffectHitTarget EFFECT_0
--   SPELL_EFFECT_HEAL only; census phantom HandleDamage/OnEffectApply/OnHit.
--   shadowfiend_death (57989, :1268-1315): CLOSED IN GO (boss_ai.go
--   creature leg :11268-11297: owner casts 58227 on self when the
--   shadowfiend sits below 1% after the damage). Real hooks DoCheckProc
--   + OnEffectProc EFFECT_0 DUMMY; census phantom HandleDamage/OnHit.
--   shadow_word_death (32379, :1316-1350): STAY OUT. OnHit HandleDamage
--   needs GetHitDamage (Pain and Suffering icon 2874 EFFECT_1 pct, then
--   caster casts 32409 on self triggered BP0=damage); the player-path hit
--   state is nil so GetHitDamage reads 0 at OnHit and AfterHit
--   (lua_spell.go:91-93). Go-leg candidate in the spells.go damage path.
--   Real hook OnHit only; census phantom HandleDamage.
--   vampiric_embrace (15286, :1351-1392): CLOSED IN GO (procs.go:5261-5280
--   + boss_ai.go:11307-11322: actor casts 15290 on null, BP0=selfHeal/5,
--   BP1=selfHeal). Real hook OnEffectProc EFFECT_0 DUMMY.
--   vampiric_touch (34914, :1393-1458): PARTIAL. HandleDispel arm is Go
--   (dispel.go:1781-1805: 64085 backlash, 8 * EFFECT_1 amount; OPEN
--   DEVIATION: no SpellDamageBonusTaken scaling, C++ :1428-1430). The
--   Replenishment arm (OnEffectProc EFFECT_2, actor casts 57669 on null)
--   is Go (procs.go specific leg, 34914 i==2, added this tick); NO Lua
--   port: ev6 fires once per eligible effect with no effect index in
--   its args, and EFFECT_0 + EFFECT_2 are both DUMMY, so a handler
--   would cast twice per proc; the EFFECT_0 CheckDummy gate has no Lua
--   model. Real hooks DoCheckEffectProc EFFECT_0 + AfterDispel +
--   OnEffectProc EFFECT_2; census hooks empty.
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
--   SW:D / VT-Replenishment legs noted above; the next
--   unworked census row is 674 (spell_q55_sacred_cleansing, spell_quest.cpp).

local SPELL_PRIEST_PENANCE_R1 = 47540 -- spell_priest.cpp:49 (class spell_pri_penance)
local SPELL_FAILED_BAD_TARGETS = 12 -- SharedDefines.h:994
local SPELL_FAILED_UNIT_NOT_INFRONT = 134 -- SharedDefines.h:1116

-- C++ spell_pri_penance_SpellScript::CheckCast (spell_priest.cpp:1033-1049):
-- a nil explicit target passes (no else arm — unlike holy shock); a
-- hostile target must pass IsValidAttackTarget (else BAD_TARGETS) and be
-- in front (else UNIT_NOT_INFRONT); friendly targets pass. Check-only
-- port: the HandleDummy legs ride ev5 (dormant, no player path) +
-- spell-level GetSpellWithRank (47758/47757). Rank 1 only — higher-rank
-- IDs are not in the file, never guessed. The C++ NPCBot caster
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
