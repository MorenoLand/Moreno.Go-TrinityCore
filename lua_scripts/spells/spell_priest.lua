-- Priest spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_priest.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2):
--   spell_pri_renew_AuraScript::HandleApplyEffect
--   (spell_priest.cpp:1238-1254): EFFECT_0 SPELL_AURA_PERIODIC_HEAL.
--   On apply, when the caster carries the Empowered Renew talent
--   aura (SPELLFAMILY_PRIEST 6, icon 3021, EFFECT_1), the caster
--   casts Empowered Renew 63544 on the target with BP0 =
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
    caster:CastCustomSpell(target, SPELL_PRIEST_EMPOWERED_RENEW, false, bp0)
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
--   CheckCast arm (IsValidAttackTarget / isInFront) has no OnCheckCast
--   bridge and the ev5 player path is dormant.
--   Census hooks wrong on 661/662/664/666/667/668/669/670 (phantom
--   OnObjectAreaTargetSelect / HandleDummy / OnCheckCast / OnEffectHitTarget;
--   real hooks are in the notes above).
