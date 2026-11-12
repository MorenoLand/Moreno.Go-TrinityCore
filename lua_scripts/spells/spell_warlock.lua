-- Warlock spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_warlock.cpp.
--
-- PORTED (tick 195, RegisterAuraEvent):
--   spell_warl_drain_soul HandleTick (spell_warlock.cpp:546-562):
--   EFFECT_1 PERIODIC_DAMAGE. When the caster is a player and the
--   target yields experience or honor (isHonorOrXPTarget), each tick
--   has a 20% chance to create a Soul Shard: the caster casts
--   CREATE_SOULSHARD 43836 on itself, triggered by the aura effect.
--   Glyph of Drain Soul leg (:557-559): while the caster has the
--   glyph aura 58070, a second roll against that aura effect's
--   MiscValue (DBC, via Aura:GetSpellEffectMiscValue) casts the
--   glyph proc 58068 on the caster, triggered. No PreventDefaultAction
--   in C++, so the Go default damage tick still fires (no return).
--   Registered for rank 1 (1120), the only Drain Soul rank ID in the
--   C++ tree; ranks 2-6 are DB-bound via spell_script_names and
--   unestablished (C++ tree + web search exhausted this tick).
-- PORTED (tick 196, RegisterAuraEvent OnApply):
--   spell_warl_health_funnel ApplyEffect (spell_warlock.cpp:838-846):
--   EFFECT_0 PERIODIC_HEAL. If the caster is missing, nothing
--   happens; if the caster has Improved Health Funnel R2 (18704)
--   the target casts buff 60956 on itself, else if R1 (18703) buff
--   60955, both triggered.
-- PORTED (tick 197, RegisterAuraEvent OnRemove):
--   spell_warl_health_funnel RemoveEffect (spell_warlock.cpp:848-853):
--   EFFECT_0 PERIODIC_HEAL. OnRemove the target drops both
--   Improved Health Funnel buffs (60955/60956). C++ uses
--   RemoveAurasDueToSpell; the Go twin is target:RemoveAura(id):
--   each buff is self-cast once by the apply leg, so a single
--   application exists and removing the spell's aura is the same
--   removal (removeCreatureAura / removeAura drop the active aura
--   of that spell ID). This retires tick 196's "no
--   RemoveAurasDueToSpell bridge" note. Ordering is immaterial:
--   the arm never queries the funnel aura itself, and C++ registers
--   plain OnEffectRemove with no mode gate.
-- PORTED (tick 202, RegisterAuraEvent):
--   spell_warl_demonic_circle_summon HandleRemove (spell_warlock.cpp:349-354)
--   + HandleDummyTick (:356-372) and spell_warl_demonic_circle_teleport
--   HandleTeleport (:398-409); the GetGameObject/RemoveGameObject bridges
--   (Unit::GetGameObject/RemoveGameObject, Unit.cpp:5199/5263) are new
--   this tick. The tick's 40-yard range is 48020's DBC friendly max
--   (SpellRange; the 40 yd tooltip range, wowwiki Demonic Circle /
--   Demonic Circle: Teleport); the reach fold follows the maiden idiom
--   (Object.cpp:1147-1165: IsWithinDist adds own + target combat reach;
--   the circle GO carries none, the player 1.5).
--   STILL BLOCKED leg: HandleTeleport's RemoveMovementImpairingAuras
--   (false) — no bridge (root/snare removal has no Lua surface).
--   (STALE as of tick 295: the bridge is live — player
--   RemoveMovementImpairingAuras, lua_player.go:3473 — and the port
--   below already calls it; the note is retired, not the leg.)
-- (STALE as of census rows 789-804: the CheckProc / HandleProc arms
--   (:517-544, EFFECT_2 improved Drain Soul mana proc) are now
--   ported below via RegisterAuraEvent OnProc = 6; the "no proc
--   hook" claim above is retired, not the arms.)

local SPELL_WARLOCK_HEALTH_FUNNEL = 755
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R1 = 18703
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R2 = 18704
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R1 = 60955
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R2 = 60956
local SPELL_WARLOCK_DRAIN_SOUL_R1 = 1120
local SPELL_WARLOCK_CREATE_SOULSHARD = 43836
local SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA = 58070
local SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_PROC = 58068

local function drainSoulTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if not caster:IsHonorOrXPTarget(target) then
        return
    end
    if math.random(0, 99) < 20 then
        caster:CastSpell(caster, SPELL_WARLOCK_CREATE_SOULSHARD, true)
        if caster:HasAura(SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA) then
            local chance = aura:GetSpellEffectMiscValue(SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA, 0)
            if math.random(0, 99) < chance then
                caster:CastSpell(caster, SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_PROC, true)
            end
        end
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DRAIN_SOUL_R1, 1, drainSoulTick)

-- C++ spell_warl_health_funnel::ApplyEffect (spell_warlock.cpp:838-846).
local function healthFunnelApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if caster:HasAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R2) then
        target:CastSpell(target, SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R2, true)
    elseif caster:HasAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R1) then
        target:CastSpell(target, SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R1, true)
    end
end

RegisterAuraEvent(SPELL_WARLOCK_HEALTH_FUNNEL, 2, healthFunnelApply)

-- C++ spell_warl_health_funnel::RemoveEffect (spell_warlock.cpp:848-853).
local function healthFunnelRemove(event, aura, target)
    target:RemoveAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R1)
    target:RemoveAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R2)
end

RegisterAuraEvent(SPELL_WARLOCK_HEALTH_FUNNEL, 3, healthFunnelRemove)

-- C++ spell_warl_haunt::HandleRemove (spell_warlock.cpp:797-806):
-- EFFECT_1 SPELL_AURA_DUMMY, REAL_OR_REAPPLY. On removal the target
-- casts Haunt Heal 48210 on the caster with BP0 = the effect amount
-- (aurEff->GetAmount()). Deviation: C++ sets args.OriginalCaster to
-- the caster GUID; CastCustomSpell takes an originalCaster arg but
-- the Go model ignores it on both paths, so it is not passed. The
-- loader's SpellScript side (spell_warl_haunt_SpellScript) is a
-- separate arm and stays out of this row.
local SPELL_WARLOCK_HAUNT = 48181 -- spell_warlock.cpp:55
local SPELL_WARLOCK_HAUNT_HEAL = 48210 -- spell_warlock.cpp:56

local function hauntRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    target:CastCustomSpell(caster, SPELL_WARLOCK_HAUNT_HEAL, false, aura:GetAmount(1))
end

RegisterAuraEvent(SPELL_WARLOCK_HAUNT, 3, hauntRemove)

local SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON = 48018 -- spell_warlock.cpp:41
local SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT = 48020 -- spell_warlock.cpp:42
local SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST = 62388 -- spell_warlock.cpp:43
local SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT_RANGE = 40 -- 48020 DBC friendly max range (yards)

-- C++ spell_warl_demonic_circle_summon::HandleRemove
-- (spell_warlock.cpp:349-354): EFFECT_0 PERIODIC_DUMMY,
-- REAL_OR_REAPPLY. On removal the summoned circle goes too (Go's
-- remove hook fires only on genuine removal, never on reapply, so
-- the C++ !(mode & REAPPLY) gate is satisfied by construction) and
-- the allow-cast aura drops.
local function demonicCircleSummonRemove(event, aura, target)
    target:RemoveGameObject(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON)
    target:RemoveAura(SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST)
end

RegisterAuraEvent(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON, 3, demonicCircleSummonRemove)

-- C++ spell_warl_demonic_circle_summon::HandleDummyTick
-- (spell_warlock.cpp:356-372): EFFECT_0 PERIODIC_DUMMY. While the
-- circle exists, a target within the teleport spell's max range gains
-- the allow-cast aura (62388); out of range it drops. No
-- PreventDefault in C++, so the Go default tick still fires.
local function demonicCircleSummonTick(event, aura, target)
    local circle = target:GetGameObject(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON)
    if circle ~= nil then
        if target:GetDistance(circle) <= SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT_RANGE + 1.5 then
            if not target:HasAura(SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST) then
                target:CastSpell(target, SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST, true)
            end
        else
            target:RemoveAura(SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST)
        end
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON, 1, demonicCircleSummonTick)

-- C++ spell_warl_demonic_circle_teleport::HandleTeleport
-- (spell_warlock.cpp:398-409): EFFECT_0 MECHANIC_IMMUNITY, REAL. The
-- player target teleports to its circle's position and orientation,
-- then drops its movement-impairing auras (snares only — C++ passes
-- withRoot=false; the per-effect snare amount-zeroing leg has no Go
-- model, same documented residual as the bridge).
local function demonicCircleTeleportApply(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    local circle = target:GetGameObject(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON)
    if circle ~= nil then
        target:TeleportNear(circle:GetX(), circle:GetY(), circle:GetZ(), circle:GetO())
        target:RemoveMovementImpairingAuras(false)
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT, 2, demonicCircleTeleportApply)

-- PORTED (tick 295, RegisterAuraEvent OnRemove = 3):
--   spell_warl_curse_of_doom::OnRemove (spell_warlock.cpp:275-296):
--   EFFECT_0 PERIODIC_DAMAGE, REAL. On removal the doomguard
--   (18662) is summoned only when the aura was removed by the
--   target's death AND its duration had expired: GetRemoveMode() == 5
--   (AURA_REMOVE_BY_DEATH, lua_aura.go GetRemoveMode twin) and
--   GetDuration() == 0 on the remove snapshot (the IsExpired proxy;
--   the snapshot carries the removed record's last remaining ms,
--   lua_aura.go auraRemainingMs — a death-removed live aura reads
--   > 0, a permanent aura reads -1). The C++ Load gate (caster is a
--   player) rides a handler-side check, and the honor/XP-target gate
--   rides IsHonorOrXPTarget. The cast is triggered (args(aurEff)
--   sets TRIGGERED_FULL_MASK). Registered for rank 1 (603) only;
--   rank 2 (30910) is not established in the C++ tree.
local SPELL_WARLOCK_CURSE_OF_DOOM_R1 = 603 -- spell_warlock.cpp:261 (class spell_warl_curse_of_doom)
local SPELL_WARLOCK_CURSE_OF_DOOM_EFFECT = 18662 -- spell_warlock.cpp:40
local AURA_REMOVE_BY_DEATH = 5 -- SpellAuraDefines.h:61-69

local function curseOfDoomRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_DEATH then
        return
    end
    if aura:GetDuration() ~= 0 then
        return
    end
    if not caster:IsHonorOrXPTarget(target) then
        return
    end
    caster:CastSpell(target, SPELL_WARLOCK_CURSE_OF_DOOM_EFFECT, true)
end

RegisterAuraEvent(SPELL_WARLOCK_CURSE_OF_DOOM_R1, 3, curseOfDoomRemove)

local SPELL_WARLOCK_BANISH = 710 -- spell_warlock.cpp:141
local SPELL_MISS_IMMUNE = 7 -- SharedDefines.h:1558 (SPELL_MISS_IMMUNE)

-- C++ spell_warl_banish::HandleBanish (spell_warlock.cpp:155-168):
-- BeforeHit. Only an IMMUNE miss re-casts Banish on a banished
-- target: the target's Banish aura from this caster is removed.
-- RegisterSpellEvent BeforeHit = 6 (lua_spell.go fireSpellBeforeHit
-- passes the real miss reason; notification-only, but this arm
-- vetoes nothing). Rank 1 only: rank 2 (18647) has no
-- spell_script_names binding in-tree.
local function banishBeforeHit(event, spell, hitTarget, missInfo)
    if missInfo ~= SPELL_MISS_IMMUNE then
        return
    end
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local banish = hitTarget:GetAura(SPELL_WARLOCK_BANISH)
    if banish == nil then
        return
    end
    if banish:GetCasterGUID() ~= caster:GetGUID() then
        return
    end
    banish:Remove()
end

RegisterSpellEvent(SPELL_WARLOCK_BANISH, 6, banishBeforeHit)

local SPELL_WARLOCK_IMPROVED_DRAIN_SOUL_R1 = 18213 -- spell_warlock.cpp:88
local SPELL_WARLOCK_IMPROVED_DRAIN_SOUL_PROC = 18371 -- spell_warlock.cpp:89
local POWER_MANA = 0

-- C++ spell_warl_drain_soul::CheckProc (:517-529) + HandleProc
-- (:531-545): EFFECT_2 PROC_TRIGGER_SPELL. The CheckProc gate (the
-- dying unit carries this caster's Drain Soul) is the early return
-- returning true (PreventDefaultAction, which suppresses the DBC
-- default leg, like a failed CheckProc); the improved Drain Soul
-- mana leg casts 18371 on the caster, triggered, BP0 = the
-- EFFECT_2 pct of the caster's max mana. GetAuraOfRankedSpell is
-- player-only, so a non-player actor vetoes after the gate. The
-- CheckProc gate is rank-1-only: procTarget:GetAura(1120) matches
-- the exact spell ID, and GetAuraApplicationOfRankedSpell is
-- unbridged, so higher-rank Drain Soul auras cannot satisfy the
-- gate (consistent with the rank-1-only tick-195 note above).
-- DORMANT until Spell.dbc ships the 1120 proc entry. Residual:
-- Go fires ev6 after charge/cooldown consumption, so a vetoed
-- (failed-gate) proc still spends a charge where C++ would not.
local function drainSoulProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil then
        return true
    end
    local drainSoul = procTarget:GetAura(SPELL_WARLOCK_DRAIN_SOUL_R1)
    if drainSoul == nil then
        return true
    end
    if drainSoul:GetCasterGUID() ~= actor:GetGUID() then
        return true
    end
    if actor:GetObjectType() ~= "Player" then
        return true
    end
    local impDrainSoul = actor:GetAuraOfRankedSpell(SPELL_WARLOCK_IMPROVED_DRAIN_SOUL_R1)
    if impDrainSoul == nil then
        return true
    end
    local pct = impDrainSoul:GetAmount(2)
    if pct == 0 then
        return true
    end
    local maxMana = actor:GetMaxPower(POWER_MANA)
    if maxMana == 0 then
        return true
    end
    local amount = math.floor(maxMana * pct / 100)
    actor:CastCustomSpell(actor, SPELL_WARLOCK_IMPROVED_DRAIN_SOUL_PROC, true, amount)
    return true
end

RegisterAuraEvent(SPELL_WARLOCK_DRAIN_SOUL_R1, 6, drainSoulProc)

local SPELL_WARLOCK_SOUL_LEECH = 30293 -- spell_warlock.cpp:1388
local SPELL_WARLOCK_SOUL_LEECH_HEAL = 30294 -- spell_warlock.cpp:78
local SPELL_WARLOCK_IMP_SOUL_LEECH_R1 = 54117 -- spell_warlock.cpp:79
local SPELL_WARLOCK_SOUL_LEECH_CASTER_MANA = { 54300, 59117 } -- spell_warlock.cpp:82-83

-- C++ spell_warl_soul_leech::HandleProc (spell_warlock.cpp:1412-1442):
-- EFFECT_0 DUMMY. On proc the actor self-casts Soul Leech Heal
-- 30294, triggered, BP0 = the effect pct of the proc damage.
-- Improved Soul Leech leg: the holder's ranked 54117 aura gives
-- the rank (GetRank; 0 without spell_ranks data skips the leg)
-- and the caster self-casts the rank-matched caster-mana spell
-- (54300/59117), triggered. BLOCKED legs: the pet-mana cast
-- (54607/59118) and Replenishment (57669) both target nil in C++
-- (pet / party via DBC); Go's nil-target CastSpell falls back to
-- the caster, which would energize the warlock instead of the pet
-- / party, so they stay out pending a pet/party target bridge.
-- DORMANT until Spell.dbc ships the 30293 proc entry.
local function soulLeechProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 then
        return true
    end
    if actor == nil then
        return true
    end
    local pct = aura:GetAmount(0)
    if pct == 0 then
        return true
    end
    local amount = math.floor(damage * pct / 100)
    actor:CastCustomSpell(actor, SPELL_WARLOCK_SOUL_LEECH_HEAL, true, amount)
    if target == nil or target:GetObjectType() ~= "Player" then
        return true
    end
    local impSoulLeech = target:GetAuraOfRankedSpell(SPELL_WARLOCK_IMP_SOUL_LEECH_R1)
    if impSoulLeech == nil then
        return true
    end
    local rank = impSoulLeech:GetRank()
    if rank < 1 or rank > 2 then
        return true
    end
    actor:CastSpell(actor, SPELL_WARLOCK_SOUL_LEECH_CASTER_MANA[rank], true)
    return true
end

RegisterAuraEvent(SPELL_WARLOCK_SOUL_LEECH, 6, soulLeechProc)

local SPELL_WARLOCK_T4_2P_BONUS = 37377 -- spell_warlock.cpp:1493 (class spell_warl_t4_2p_bonus)
local SPELL_WARLOCK_FLAMESHADOW = 37379 -- spell_warlock.cpp:86 (SPELL_WARLOCK_FLAMESHADOW)

-- C++ spell_warl_t4_2p_bonus_AuraScript::HandleProc
-- (spell_warlock.cpp:1506-1511): EFFECT_0 DUMMY OnEffectProc —
-- PreventDefaultAction; the proc actor self-casts the template
-- trigger, triggered. 37377 binds to spell_warl_t4_2p_bonus_shadow
-- in spell_script_names (TDB 25101 + 26091), so the trigger is
-- Flameshadow 37379, not Shadowflame 37378 (39437 binds _fire).
-- DORMANT until Spell.dbc ships the 37377 proc entry.
local function t4TwoPieceBonusProc(event, aura, target, actor, procTarget, damage)
    if actor == nil then
        return true
    end
    actor:CastSpell(actor, SPELL_WARLOCK_FLAMESHADOW, true)
    return true
end

RegisterAuraEvent(SPELL_WARLOCK_T4_2P_BONUS, 6, t4TwoPieceBonusProc)

local SPELL_WARLOCK_SEED_OF_CORRUPTION_DUMMY = 27243 -- spell_warlock.cpp:1177
local SPELL_WARLOCK_SEED_OF_CORRUPTION_DAMAGE_R1 = 27285 -- spell_warlock.cpp:75

-- C++ spell_warl_seed_of_corruption_dummy::HandleProc
-- (spell_warlock.cpp:1202-1227): EFFECT_1 DUMMY. On proc the
-- damage is eaten from the buffer (EFFECT_1 amount); a surviving
-- buffer only detonates when the target's health after the hit
-- drops below 1% (HealthBelowPctDamaged: health - damage <
-- maxHealth * 1 / 100, Unit.h:873 — C++ reads post-damage health
-- too, DealSpellDamage precedes ProcSkillsAndAuras in
-- DoDamageAndTriggers, Spell.cpp:2548/2586, so the subtraction
-- is the C++ formula, not a double-count). On detonation the aura
-- is removed and the caster casts the rank-matched Seed of
-- Corruption damage spell on the ACTION TARGET (the seeded holder,
-- GetActionTarget(); the ev6 procTarget is the attacker on the
-- taken side, Unit.cpp:10412-10417), triggered.
-- BLOCKED leg: CalculateBuffer (:1192-1200, SP-scaled buffer via
-- SpellDamageBonusDone) has no DoEffectCalcAmount bridge, so the
-- buffer starts at its DBC base. DORMANT until Spell.dbc ships
-- the 27243 proc entry (and spell_ranks for the rank lookup).
local function seedOfCorruptionDummyProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 then
        return true
    end
    if target == nil or procTarget == nil then
        return true
    end
    local amount = aura:GetAmount(1) - damage
    if amount > 0 then
        aura:SetAmount(amount, 1)
        if target:GetHealth() - damage >= math.floor(target:GetMaxHealth() / 100) then
            return true
        end
    end
    local caster = aura:GetCaster()
    aura:Remove()
    if caster == nil then
        return true
    end
    local rankSpell = aura:GetSpellWithRank(SPELL_WARLOCK_SEED_OF_CORRUPTION_DAMAGE_R1)
    if rankSpell == 0 then
        return true
    end
    caster:CastSpell(target, rankSpell, true)
    return true
end

RegisterAuraEvent(SPELL_WARLOCK_SEED_OF_CORRUPTION_DUMMY, 6, seedOfCorruptionDummyProc)

-- C++ spell_warl_haunt_SpellScript::HandleAfterHit
-- (spell_warlock.cpp:772-778): AfterHit. The hit aura's EFFECT_1
-- amount becomes pct of the final hit damage. The aura is applied
-- earlier in the same cast (spells.go applyAuraToTarget), and
-- GetHitDamage reads the final damage at AfterHit (runtime.go
-- contract). The GetCasterGUID guard is the GetHitAura twin: only
-- this cast's aura is rescaled, never another caster's Haunt.
local function hauntAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    local aura = hitTarget:GetAura(SPELL_WARLOCK_HAUNT)
    if aura == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil and aura:GetCasterGUID() ~= caster:GetGUID() then
        return
    end
    aura:SetAmount(math.floor(spell:GetHitDamage() * aura:GetAmount(1) / 100), 1)
end

RegisterSpellEvent(SPELL_WARLOCK_HAUNT, 3, hauntAfterHit)

local SPELL_WARLOCK_DECIMATION_R2 = 63158 -- spell_warlock.cpp:306, bot_warlock_ai.cpp:85
local SPELL_WARLOCK_DECIMATION_BUFF = 63167 -- bot_warlock_ai.cpp:109

-- C++ spell_warl_decimation::CheckProc (spell_warlock.cpp:316-324):
-- DoCheckProc only. The proc passes only when the action target
-- carries AURA_STATE_HEALTHLESS_35_PERCENT, which Unit.cpp:468
-- maintains as HealthBelowPct(35) (Unit.h:872: health <
-- CountPctFromMaxHealth(35), floor(maxHealth * 35 / 100)) — and only
-- when the event carries a spell (GetSpellInfo() nil returns false,
-- so melee events never proc). The ev6 veto is inverted vs C++:
-- return true (PreventDefaultAction, lua_aura.go fireAuraProc) when
-- the gate FAILS, so a passing proc falls through and the DBC default
-- leg applies the Decimation buff 63167. Registered for rank 2 (63158)
-- only; rank 1 (63156) is unestablished (zero C++ tree hits). No
-- HasAuraState bridge, so the gate is the integer-exact HealthBelowPct
-- proxy (the seed-of-corruption-dummy idiom above). The nil-spell leg
-- rides the forwarded proc event (procEventObject, lua_aura.go):
-- GetSpellId() == 0 vetoes. DORMANT
-- until Spell.dbc ships the 63158 proc entry.
local function decimationProc(event, aura, target, actor, procTarget, damage, procEvent)
    if procTarget == nil or procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if procTarget:GetHealth() < math.floor(procTarget:GetMaxHealth() * 35 / 100) then
        return
    end
    return true
end

RegisterAuraEvent(SPELL_WARLOCK_DECIMATION_R2, 6, decimationProc)

local SPELL_WARLOCK_NIGHTFALL = 18094 -- spell_warlock.cpp:665
local SPELL_WARLOCK_GLYPH_OF_CORRUPTION = 56218 -- spell_warlock.cpp:666
local SPELL_WARLOCK_SHADOW_TRANCE = 17941 -- spell_warlock.cpp:77

-- C++ spell_warl_glyph_of_corruption_nightfall::HandleProc
-- (spell_warlock.cpp:679-684): PreventDefaultAction; the proc actor casts
-- Shadow Trance (17941) on itself. Same handler for both ids bound to the
-- class (Nightfall 18094, Glyph of Corruption 56218). DORMANT until
-- Spell.dbc ships the proc entries.
local function nightfallProc(event, aura, target, actor, procTarget, damage)
    local caster = actor or target
    if caster == nil then
        return true
    end
    caster:CastSpell(caster, SPELL_WARLOCK_SHADOW_TRANCE, true)
    return true
end

RegisterAuraEvent(SPELL_WARLOCK_NIGHTFALL, 6, nightfallProc)
RegisterAuraEvent(SPELL_WARLOCK_GLYPH_OF_CORRUPTION, 6, nightfallProc)

local SPELL_WARLOCK_SEED_OF_CORRUPTION_GENERIC = 32865 -- spell_warlock.cpp:76
-- Class ids per spell_warlock.cpp:1246-1252. 39367 shares the class but has
-- no census row in 865-879.
local SPELL_WARLOCK_SEED_OF_CORRUPTION_GENERIC_AURAS = { 32863, 36123, 38252, 39367, 44141, 70388 }

-- C++ spell_warl_seed_of_corruption_generic::HandleProc
-- (spell_warlock.cpp:1260-1283): PreventDefaultAction; the proc damage is
-- eaten from the EFFECT_1 buffer. A surviving buffer persists (no
-- HealthBelowPctDamaged gate, unlike the 27243 dummy class); on depletion
-- the aura is removed and the caster casts 32865 on the ACTION TARGET (the
-- seeded holder, the ev6 target — the seed-of-corruption-dummy idiom,
-- spell_warlock.lua:360-404). DORMANT until Spell.dbc ships the proc
-- entries. Session-side ev6 only: fires when the seeded holder is a
-- player; a creature-held seed has no proc evaluation site.
local function seedOfCorruptionGenericProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 then
        return true
    end
    if target == nil then
        return true
    end
    local amount = aura:GetAmount(1) - damage
    if amount > 0 then
        aura:SetAmount(amount, 1)
        return true
    end
    local caster = aura:GetCaster()
    aura:Remove()
    if caster == nil then
        return true
    end
    caster:CastSpell(target, SPELL_WARLOCK_SEED_OF_CORRUPTION_GENERIC, true)
    return true
end

for _, spellId in ipairs(SPELL_WARLOCK_SEED_OF_CORRUPTION_GENERIC_AURAS) do
    RegisterAuraEvent(spellId, 6, seedOfCorruptionGenericProc)
end

local SPELL_WARLOCK_LIFE_TAP_R1 = 1454 -- spell_warlock.cpp (class spell_warl_life_tap_SpellScript)
local SPELL_FAILED_FIZZLE = 33 -- SharedDefines.h:1015
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_warl_life_tap_SpellScript::CheckCast (spell_warlock.cpp:922-927):
-- the caster must have strictly more health than the EFFECT_0 health
-- cost (spell:GetEffectValue(0), the CalcValue twin) or the cast
-- fizzles. Check-only port: the energize leg still needs
-- spellpower-coefficient / ModifyHealth bridges (census row 796).
-- Rank 1 only — higher Life Tap ranks are DB-bound via
-- spell_script_names, unestablished in-tree (the drain_soul
-- rank-1-only precedent). RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast).
local function lifeTapCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and caster:GetHealth() > spell:GetEffectValue(0) then
        return SPELL_CAST_OK
    end
    return SPELL_FAILED_FIZZLE
end

RegisterSpellEvent(SPELL_WARLOCK_LIFE_TAP_R1, 8, lifeTapCheckCast)
