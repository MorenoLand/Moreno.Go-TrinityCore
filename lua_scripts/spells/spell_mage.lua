-- Mage spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_mage.cpp.
--
-- Binding IDs come from the `// <id> - Name` header comments above
-- each class (the SpellScriptLoader name binds in DB
-- spell_script_names); the effect spell IDs are the file's enums.
--
-- PORTED (tick 294, RegisterAuraEvent):
--   spell_mage_frostfire_bolt (spell_mage.cpp:695-721): aura 44614,
--   modes 2 + 3. Both legs: the aura's caster, when present, casts
--   68391 (Permafrost) on the target, triggered by the aura effect.
--   PORTED AS-REGISTERED: C++ binds AfterEffectRemove to
--   ApplyPermafrost (:718), not to RemovePermafrost (:710-713, dead
--   code) — so removal re-casts 68391, exactly as the C++ loader
--   registers it. The intended remove leg is an upstream bug, not a
--   porting decision.
--   spell_mage_mirror_image (spell_mage.cpp:1279-1299): aura 55342,
--   mode 1. PeriodicTick: the EFFECT_2 trigger spell (DBC read, 0
--   skips the cast) is cast by the target on nil, triggered. No
--   return true: C++ has no PreventDefaultAction. Inert until
--   Spell.dbc loads (0-read convention).
--   spell_mage_living_bomb (spell_mage.cpp:1101-1136): aura 44457,
--   mode 3. AfterRemove fires only when the remove mode is
--   ENEMY_SPELL (3) or EXPIRE (4): the aura's caster, when present,
--   casts the EFFECT_1 amount as a spell ID on the target,
--   triggered by the aura effect (0 amount skips the cast — the
--   DBC read for the explosion spell ID).
--   spell_mage_combustion_proc (spell_mage.cpp:419-449): aura 28682,
--   mode 3. OnRemove the target drops 11129 (Combustion).
--   Notification-only.
--   spell_mage_fingers_of_frost (spell_mage.cpp:587-625): aura
--   74396, mode 3 only. OnRemove the target drops 44544 (Fingers of
--   Frost aurastate aura). The proc leg (RemoveAuraFromStack on
--   proc) is the Go arm in procs.go (rows 181-185).
--   Mode 2 fires on fresh apply only; the frostfire apply leg is
--   AURA_EFFECT_HANDLE_REAL_OR_REAPPLY_MASK, so a reapply does not
--   re-cast 68391 in Go (standing deviation).
-- PORTED IN GO (tick 2026-11-10, rows 176-180):
--   spell_mage_burning_determination (spell_mage.cpp:261-292):
--   54747/54748 CheckProc gate in procs.go (interrupt/silence
--   mechanic only); default trigger DBC-driven, dormant until
--   Spell.dbc ships the proc entries.
--   spell_mage_combustion (spell_mage.cpp:380-417): 11129 CheckProc
--   gate in procs.go (non-crit casts 28682 triggered, no charge;
--   crit lets the default proc run), dormant until Spell.dbc ships
--   the 11129 proc entry.
--   spell_mage_imp_mana_gems (spell_mage.cpp:506-535): 37447/61062
--   EFFECT_1 OnEffectProc arm in procs.go (actor casts 37445
--   triggered on self); OVERRIDE_CLASS_SCRIPTS joined the
--   proc-trigger types per SpellMgr.cpp:1708, dormant until
--   Spell.dbc ships the proc entries.
--   spell_mage_cold_snap (spell_mage.cpp:343-377): 11958 DUMMY arm
--   in spells.go (resets every other Frost Mage cooldown with a
--   recovery time); live on cast, not dormant.
-- PORTED IN GO (tick 2026-11-10, rows 181-185):
--   spell_mage_fingers_of_frost (spell_mage.cpp:587-625): 74396
--   EFFECT_0 OnEffectProc arm in procs.go (actor loses one stack
--   of 74396); mode-3 OnRemove leg stays the Lua leg above.
--   Dormant until Spell.dbc ships the 74396 proc entry.
--   spell_mage_focus_magic (spell_mage.cpp:723-761): 54646
--   CheckProc gate (caster alive) + EFFECT_0 OnEffectProc arm in
--   procs.go (holder casts 54648 on the caster);
--   MOD_SPELL_CRIT_CHANCE joined the proc-trigger types
--   (SpellMgr.cpp:1718). Dormant until Spell.dbc ships the 54646
--   proc entry.
--   spell_mage_gen_extra_effects (spell_mage.cpp:766-819): 48108
--   CheckProc gate (no self-target events; T8 4P roll suppresses)
--   + OnProc arm in procs.go (actor with T10 2P aura 70752 casts
--   70753 on self; no PreventDefaultAction, default 57761 still
--   fires); 48108 has a single DBC effect
--   (ADD_FLAT_MODIFIER, SpellModOp 10 CASTING_TIME, Wowhead
--   WotLK). Dormant until Spell.dbc ships the 48108 proc entry.
--   spell_mage_glyph_of_polymorph (spell_mage.cpp:822-852): 56375
--   EFFECT_0 OnEffectProc arm in procs.go (proc target loses all
--   periodic-damage auras except 32409, plus periodic-damage-
--   percent and periodic-leech); player + creature target legs.
--   Dormant until Spell.dbc ships the 56375 proc entry.
--   spell_mage_glyph_of_icy_veins (spell_mage.cpp:853-883): 56374
--   EFFECT_0 OnEffectProc arm in procs.go (actor loses negative
--   haste-spells auras (C++ passes true, false) and all
--   decrease-speed auras (C++ defaults, both polarities)).
--   Dormant until Spell.dbc
--   ships the 56374 proc entry.
-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_mage_blazing_speed (spell_mage.cpp:228-258): aura 31641,
--   EFFECT_0 PROC_TRIGGER_SPELL. PreventDefaultAction (return true)
--   so the Go default trigger leg does not fire; the action target
--   casts 31643 (Blazing Speed buff, spell_mage.cpp:35) on itself,
--   triggered. The ev6 `target` arg is the aura holder on both proc
--   loops, which is C++ GetActionTarget on the taken event
--   (Unit.cpp:10412; procs.go procActionTargetGUID) — the talent
--   procs only when hit, so the taken-only reading holds. DORMANT
--   until Spell.dbc ships the 31641 proc entry.
-- STAY-OUT (tick 294): every other spell_mage class — no proc hook,
--   absorb/CalcAmount/ManaShield hooks absent, SpellScript-only
--   with no RegisterSpellScript in the engine, or cross-script state
--   (arcane_missiles / missile_barrage_proc share a flag no Lua leg
--   can set). Notable: ice_barrier (absorb + GetDummyAuraEffect
--   not on the aura-hook surface), fire_frost_ward (absorb).
--   (Ignite left this list this tick: GetSpellMaxTicks + the new
--   aura:GetRank bridge cover its rank/max-ticks math.)

local SPELL_MAGE_FROSTFIRE_BOLT = 44614
local SPELL_MAGE_PERMAFROST_AURA = 68391
local SPELL_MAGE_MIRROR_IMAGE = 55342
local SPELL_MAGE_LIVING_BOMB = 44457
local SPELL_MAGE_COMBUSTION_PROC = 28682
local SPELL_MAGE_COMBUSTION = 11129
local SPELL_MAGE_FINGERS_OF_FROST = 74396
local SPELL_MAGE_FINGERS_OF_FROST_AURASTATE_AURA = 44544

local AURA_REMOVE_BY_ENEMY_SPELL = 3
local AURA_REMOVE_BY_EXPIRE = 4

local function frostfireBoltApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, SPELL_MAGE_PERMAFROST_AURA, true)
    end
end

RegisterAuraEvent(SPELL_MAGE_FROSTFIRE_BOLT, 2, frostfireBoltApply)
RegisterAuraEvent(SPELL_MAGE_FROSTFIRE_BOLT, 3, frostfireBoltApply)

local function mirrorImageTick(event, aura, target)
    local trigger = aura:GetTriggerSpell(2)
    if trigger ~= 0 then
        target:CastSpell(nil, trigger, true)
    end
end

RegisterAuraEvent(SPELL_MAGE_MIRROR_IMAGE, 1, mirrorImageTick)

local function livingBombRemove(event, aura, target)
    local mode = aura:GetRemoveMode()
    if mode ~= AURA_REMOVE_BY_ENEMY_SPELL and mode ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local id = aura:GetAmount(1)
    if id == 0 then
        return
    end
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, id, true)
    end
end

RegisterAuraEvent(SPELL_MAGE_LIVING_BOMB, 3, livingBombRemove)

local function combustionProcRemove(event, aura, target)
    target:RemoveAura(SPELL_MAGE_COMBUSTION)
end

RegisterAuraEvent(SPELL_MAGE_COMBUSTION_PROC, 3, combustionProcRemove)

local function fingersOfFrostRemove(event, aura, target)
    target:RemoveAura(SPELL_MAGE_FINGERS_OF_FROST_AURASTATE_AURA)
end

RegisterAuraEvent(SPELL_MAGE_FINGERS_OF_FROST, 3, fingersOfFrostRemove)

local SPELL_MAGE_BLAZING_SPEED_AURA = 31641 -- spell_mage.cpp:228 (class spell_mage_blazing_speed)
local SPELL_MAGE_BLAZING_SPEED_BUFF = 31643 -- spell_mage.cpp:35

local function blazingSpeedProc(event, aura, target, actor, procTarget, damage)
    target:CastSpell(target, SPELL_MAGE_BLAZING_SPEED_BUFF, true)
    return true
end

RegisterAuraEvent(SPELL_MAGE_BLAZING_SPEED_AURA, 6, blazingSpeedProc)

local SPELL_MAGE_IMP_BLIZZARD_AURA = 11185 -- spell_mage.cpp:471 (class spell_mage_imp_blizzard)
local SPELL_MAGE_CHILLED = 12484 -- spell_mage.cpp:56

local function impBlizzardProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil then
        return true
    end
    local chilled = aura:GetSpellWithRank(SPELL_MAGE_CHILLED)
    if chilled == 0 then
        return true
    end
    actor:CastSpell(procTarget, chilled, true)
    return true
end

RegisterAuraEvent(SPELL_MAGE_IMP_BLIZZARD_AURA, 6, impBlizzardProc)

local SPELL_MAGE_IGNITE_AURA = 11119 -- spell_mage.cpp:1053 (class spell_mage_ignite)
local SPELL_MAGE_IGNITE = 12654 -- spell_mage.cpp:43
local SPELL_MAGE_MAGIC_ABSORPTION_AURA = 29441 -- spell_mage.cpp:1138 (class spell_mage_magic_absorption)
local SPELL_MAGE_MAGIC_ABSORPTION_MANA = 29442 -- spell_mage.cpp:58

local function igniteProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 or procTarget == nil then
        return true
    end
    local maxTicks = aura:GetSpellMaxTicks(SPELL_MAGE_IGNITE)
    local rank = aura:GetRank()
    if maxTicks <= 0 or rank == 0 then
        return true
    end
    local amount = math.floor(damage * (8 * rank) / 100 / maxTicks)
    target:CastCustomSpell(procTarget, SPELL_MAGE_IGNITE, true, amount)
    return true
end

RegisterAuraEvent(SPELL_MAGE_IGNITE_AURA, 6, igniteProc)

local function magicAbsorptionProc(event, aura, target, actor, procTarget, damage)
    local bp0 = math.floor(target:GetMaxPower(0) * aura:GetAmount() / 100)
    target:CastCustomSpell(target, SPELL_MAGE_MAGIC_ABSORPTION_MANA, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_MAGE_MAGIC_ABSORPTION_AURA, 6, magicAbsorptionProc)

local SPELL_MAGE_HOT_STREAK = 44445 -- spell_mage.cpp:919 (class spell_mage_hot_streak; rank 1; ranks 2-3 IDs are not in the file, never guessed)
local SPELL_MAGE_HOT_STREAK_PROC = 48108 -- spell_mage.cpp:61
local PROC_HIT_CRITICAL = 2 -- SharedDefines.h: PROC_HIT_CRITICAL (procs.go procHitCritical)

-- C++ spell_mage_hot_streak::HandleProc (spell_mage.cpp:935-957):
-- EFFECT_0 SPELL_AURA_DUMMY OnEffectProc. PreventDefaultAction (the
-- true return on every path); the EFFECT_1 counter doubles on a
-- spell crit (hit mask PROC_HIT_CRITICAL) and resets to 25 on any
-- path that reaches the end — a non-crit, or a crit that rolls the
-- EFFECT_0 chance at 100 and casts 48108 on the actor, triggered
-- via the aurEff overload. A crit below 100 or a failed roll returns
-- early with the doubled counter kept. The ev6 effectIndex 8th arg
-- (lua_aura.go fireAuraProc) gates the arm to EFFECT_0: both effects
-- are DUMMY, so without it the counter would double twice per crit;
-- other indices veto the no-op default leg. DORMANT until Spell.dbc
-- ships the 44445 proc entry.
local function hotStreakProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if aura == nil or actor == nil or procEvent == nil then
        return true
    end
    local counter = aura:GetAmount(1)
    if bit32.band(procEvent:GetHitMask(), PROC_HIT_CRITICAL) ~= 0 then
        counter = counter * 2
        if counter < 100 then
            aura:SetAmount(counter, 1)
            return true
        end
        if math.random(0, 99) >= aura:GetAmount(0) then
            aura:SetAmount(counter, 1)
            return true
        end
        actor:CastSpell(actor, SPELL_MAGE_HOT_STREAK_PROC, true)
    end
    aura:SetAmount(25, 1)
    return true
end

RegisterAuraEvent(SPELL_MAGE_HOT_STREAK, 6, hotStreakProc)

local SPELL_MAGE_EMPOWERED_FIRE = 31656 -- spell_mage.cpp:538 (class spell_mage_empowered_fire)
local SPELL_MAGE_EMPOWERED_FIRE_PROC = 67545 -- spell_mage.cpp:65

-- C++ spell_mage_empowered_fire::CheckProc + HandleProc
-- (spell_mage.cpp:556-572): EFFECT_0 SPELL_AURA_ADD_FLAT_MODIFIER
-- OnEffectProc. CheckProc passes only when the proc spell is Ignite
-- 12654 (procEvent:GetSpellId); HandleProc PreventDefaultActions
-- (the true return on every path) and the holder casts 67545 on
-- itself with BP0 = pct(base mana, EFFECT_0 CalcValue of 67545) —
-- GetCreateMana (lua_player.go) + GetSpellEffectCalcValue
-- (lua_aura.go). C++ casts non-triggered; Lua CastCustomSpell rides
-- the triggered-only funnel (standing gap, same as ignite). The
-- effectIndex gate keeps the arm on EFFECT_0. DORMANT until
-- Spell.dbc ships the 31656 proc entry.
local function empoweredFireProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if procEvent == nil or procEvent:GetSpellId() ~= SPELL_MAGE_IGNITE then
        return true
    end
    if aura == nil or target == nil or target:GetTypeId() ~= 4 then
        return true
    end
    local percent = aura:GetSpellEffectCalcValue(SPELL_MAGE_EMPOWERED_FIRE_PROC, 0)
    if percent == 0 then
        return true
    end
    local bp0 = math.floor(target:GetCreateMana() * percent / 100)
    target:CastCustomSpell(target, SPELL_MAGE_EMPOWERED_FIRE_PROC, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_MAGE_EMPOWERED_FIRE, 6, empoweredFireProc)

local SPELL_MAGE_ARCANE_POTENCY = 31571 -- spell_mage.cpp:143
local SPELL_MAGE_ARCANE_POTENCY_TRIGGER = {57529, 57531} -- spell_mage.cpp:156-157
local SPELL_ICON_MAGE_CLEARCASTING = 212 -- spell_mage.cpp:79
local SPELL_ICON_MAGE_PRESENCE_OF_MIND = 139 -- spell_mage.cpp:78

-- C++ spell_mage_arcane_potency::CheckProc + HandleProc (spell_mage.cpp:164-186):
-- EFFECT_0 SPELL_AURA_DUMMY OnEffectProc. CheckProc passes only when the
-- event spell's icon is Clearcasting (212) or Presence of Mind (139)
-- (the family-mask share with Brain Freeze / Missile Barrage is
-- filtered by icon, not flags); HandleProc PreventDefaultActions (the
-- true return on every path) and the actor casts the rank-indexed
-- trigger (57529/57531) on itself, triggered via the aurEff overload.
-- The icon rides the forwarded proc event (procEventObject, lua_aura.go:
-- GetSpellIconId); the rank is the aura's (aura:GetRank, 0 vetoes
-- instead of casting a bogus trigger — silverback precedent).
-- DORMANT until Spell.dbc ships the 31571 proc entry.
local function arcanePotencyProc(event, aura, target, actor, procTarget, damage, procEvent)
    if actor == nil or aura == nil or procEvent == nil then
        return true
    end
    local icon = procEvent:GetSpellIconId()
    if icon ~= SPELL_ICON_MAGE_CLEARCASTING and icon ~= SPELL_ICON_MAGE_PRESENCE_OF_MIND then
        return true
    end
    local trigger = SPELL_MAGE_ARCANE_POTENCY_TRIGGER[aura:GetRank()]
    if trigger == nil then
        return true
    end
    actor:CastSpell(actor, trigger, true)
    return true
end

RegisterAuraEvent(SPELL_MAGE_ARCANE_POTENCY, 6, arcanePotencyProc)


-- NOT IN LUA: spell_mage_blast_wave (11113, spell_mage.cpp:195-226)
-- — SpellScript OnEffectHitTarget KNOCK_BACK (glyph 62126 gate);
-- no RegisterSpellScript bridge. STAYS OUT.
local SPELL_MAGE_BURNOUT_AURA = 44449 -- spell_mage.cpp:293 (class spell_mage_burnout; rank 1; ranks 2-3 IDs are not in the file, never guessed)
local SPELL_MAGE_BURNOUT = 44450 -- spell_mage.cpp:36

-- C++ spell_mage_burnout::CheckProc + HandleProc (spell_mage.cpp:311-328):
-- OnEffectProc EFFECT_1 DUMMY. CheckProc passes only when the damage
-- event carries a spell (procEvent:GetSpellId, the damage-info spell
-- twin — procs.go threads the casting spell on damage events);
-- HandleProc PreventDefaultActions (the true return on every path)
-- and the holder casts 44450 on itself with BP0 = pct(CalcPowerCost
-- of the damage spell, EFFECT_1 amount) — GetSpellPowerCost
-- (lua_player.go) on the session-player holder. C++ casts with
-- CastSpellExtraArgs(aurEff), i.e. TRIGGERED_FULL_MASK
-- (SpellDefines.h), so the Lua triggered CastCustomSpell is exact.
-- The effectIndex gate keeps the arm on EFFECT_1. DORMANT until
-- Spell.dbc ships the 44449 proc entry.
local function burnoutProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 1 then
        return true
    end
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if aura == nil or target == nil then
        return true
    end
    local mana = math.floor(target:GetSpellPowerCost(procEvent:GetSpellId()) * aura:GetAmount(1) / 100)
    target:CastCustomSpell(target, SPELL_MAGE_BURNOUT, true, mana)
    return true
end

RegisterAuraEvent(SPELL_MAGE_BURNOUT_AURA, 6, burnoutProc)

local SPELL_MAGE_MASTER_OF_ELEMENTS_AURA = 29074 -- spell_mage.cpp:1226 (class spell_mage_master_of_elements; rank 1; ranks 2-3 IDs are not in the file, never guessed)
local SPELL_MAGE_MASTER_OF_ELEMENTS_ENERGIZE = 29077 -- spell_mage.cpp:44

-- C++ spell_mage_master_of_elements::CheckProc + HandleProc
-- (spell_mage.cpp:1243-1268): OnEffectProc EFFECT_0 DUMMY. CheckProc
-- passes only when the damage event carries a spell
-- (procEvent:GetSpellId); HandleProc PreventDefaultActions (the true
-- return on every path) and, when the pct is positive, the holder
-- casts 29077 on itself with BP0 = pct(CalcPowerCost of the damage
-- spell, EFFECT_0 amount) — GetSpellPowerCost (lua_player.go). C++
-- casts with CastSpellExtraArgs(aurEff), i.e. TRIGGERED_FULL_MASK
-- (SpellDefines.h), so the Lua triggered CastCustomSpell is exact.
-- The effectIndex gate keeps the arm on EFFECT_0. DORMANT until
-- Spell.dbc ships the 29074 proc entry.
local function masterOfElementsProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    if aura == nil or target == nil then
        return true
    end
    local mana = math.floor(target:GetSpellPowerCost(procEvent:GetSpellId()) * aura:GetAmount(0) / 100)
    if mana > 0 then
        target:CastCustomSpell(target, SPELL_MAGE_MASTER_OF_ELEMENTS_ENERGIZE, true, mana)
    end
    return true
end

RegisterAuraEvent(SPELL_MAGE_MASTER_OF_ELEMENTS_AURA, 6, masterOfElementsProc)
-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_mage_imp_blizzard (11185, spell_mage.cpp:471-501):
--   OnEffectProc EFFECT_0 OVERRIDE_CLASS_SCRIPTS. PreventDefaultAction
--   (return true); the actor casts the rank-matched Chilled
--   (GetSpellWithRank(12484, this aura's rank), new aura bridge on
--   the GetSpellEffectCalcValue precedent) on the procTarget,
--   triggered. DORMANT until Spell.dbc ships the 11185 proc entry.
-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_mage_ignite (11119, spell_mage.cpp:1053-1098): OnEffectProc
--   EFFECT_0 DUMMY. PreventDefaultAction (return true); the holder
--   casts Ignite 12654 on the procTarget with BP0 = pct(damage,
--   8 * rank) / maxTicks (new aura:GetRank bridge + GetSpellMaxTicks
--   on the GetSpellEffectCalcValue precedent; the damage gate is the
--   ev6 damage-arg convention for the C++ damageInfo check).
--   KNOWN DEVIATION: C++ casts non-triggered; Lua CastCustomSpell
--   rides the triggered-only funnel (standing gap, same as vendetta).
--   DORMANT until Spell.dbc ships the 11119 proc entry.
--   spell_mage_magic_absorption (29441, spell_mage.cpp:1138-1173):
--   OnEffectProc EFFECT_0 DUMMY. PreventDefaultAction (return true);
--   the holder (C++ GetActionTarget on the resist event) casts
--   29442 on itself with BP0 = pct(max mana, aurEff amount), via
--   GetMaxPower(0). KNOWN DEVIATION: C++ casts non-triggered; Lua
--   CastCustomSpell rides the triggered-only funnel (standing gap,
--   same as ignite). DORMANT until Spell.dbc ships the 29441 proc entry.
--   spell_mage_hot_streak (44445 R1, spell_mage.cpp:919-968):
--   OnEffectProc EFFECT_0 DUMMY. PreventDefaultAction (return true);
--   EFFECT_1 consecutive-crit counter doubles per crit (25 -> 50 ->
--   100), rolls EFFECT_0 chance at 100, actor casts 48108 triggered
--   on itself; any path reaching the end resets the counter to 25.
--   R2/R3 IDs unverified in-tree (world.db 0-byte). DORMANT until
--   Spell.dbc ships the 44445 proc entry.
-- CENSUS TRIAGE (this tick, rows 613-616 + 620-622, verified in-house):
--   620 mana_shield STAYS OUT — CORRECTION: the ev6 port landed this
--   tick as 191b872e is reverted here. SPELL_AURA_MANA_SHIELD is not
--   in isProcTriggerAuraType (procs.go:1350) and the player proc loop
--   skips non-trigger effects (procs.go:3915), so ev6 never fires for
--   1463 — no DBC proc entry can enable it. The HandleProc Arcane
--   Surge 37436 leg needs a mana-shield proc leg in the Go loops
--   first; the CalculateAmount spellpower leg and the incanter's-
--   absorption Trigger leg stay out with it (SpellBaseHealing-
--   BonusDone / GetAuraEffectOfRankedSpell unbridged).
--   613 empowered_fire PORTED this tick (ev6, above): CheckProc
--   (proc spell is Ignite 12654 via procEvent:GetSpellId) +
--   HandleProc (BP0 = pct(GetCreateMana, CalcValue of 67545),
--   holder casts 67545 on itself). KNOWN DEVIATION: C++ casts
--   non-triggered; Lua CastCustomSpell rides the triggered-only
--   funnel (same as ignite). DORMANT until Spell.dbc ships the
--   31656 proc entry.
--   614 fire_frost_ward STAYS OUT — CalculateAmount needs
--   SpellBaseHealingBonusDone + level penalty; Absorb needs
--   OnEffectAbsorb + GetAuraEffectOfRankedSpell. Both unbridged.
--   615 hot_streak PORTED this tick (ev6, above): the ev6
--   effectIndex 8th arg gates the EFFECT_1 counter arm to
--   EFFECT_0, so the counter doubles once per crit. DORMANT until
--   Spell.dbc ships the 44445 proc entry.
--   616 ice_barrier STAYS OUT — CalculateAmount needs
--   SpellBaseHealingBonusDone + ApplyEffectModifiers + level
--   penalty; AfterRemove needs GetDummyAuraEffect + ProcChance.
--   621 master_of_elements PORTED this tick (ev6: CheckProc damage
--   spell via procEvent:GetSpellId; HandleProc BP0 = pct(CalcPowerCost,
--   EFFECT_0 amount), mana > 0 gate, holder casts 29077 self).
--   622 missile_barrage STAYS OUT — CheckProc needs the proc
--   spell's family flags (Arcane Blast [0] 0x20000000 full chance,
--   else 50% roll); ev6 has no event spell. Recipe: ev6
--   event-spell forwarding.
-- CENSUS CORRECTION: row 610 hooks are HandleDummy only; real is
-- OnEffectProc EFFECT_0 PROC_TRIGGER_SPELL. Rows 608/611/612 hooks
-- are likewise wrong (608: DoCheckProc + OnEffectProc; 611: DoCheckProc
-- + OnEffectProc EFFECT_1 DUMMY; 612: OnEffectProc EFFECT_0
-- OVERRIDE_CLASS_SCRIPTS).

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_mage_polymorph_cast_visual::HandleDummy (spell_mage.cpp:1386-1418):
--   EFFECT_0 DUMMY. The caster finds the nearest Aurosalia (18744)
--   within 30 yards; when found (and a creature — FindNearestCreature
--   returns creatures only, the C++ TYPEID_UNIT check is vacuous
--   here), that creature casts a random polymorph form on itself,
--   triggered: squirrel 32813 / giraffe 32816 / serpent 32817 /
--   dragonhawk 32818 / worgen 32819 / sheep 32820 (PolymorphForms[6],
--   spell_mage.cpp:1422-1430; urand(0, 5) rides math.random(1, 6)).
--   No PreventDefaultAction in C++ — the default dummy leg still runs.
--   DORMANT until Spell.dbc ships the 32826 row.
local SPELL_MAGE_POLYMORPH_VISUAL = 32826 -- spell_mage.cpp:1386 (class spell_mage_polymorph_cast_visual)
local NPC_AUROSALIA = 18744 -- spell_mage.cpp:1382
local SPELL_MAGE_POLYMORPH_FORMS = { 32813, 32816, 32817, 32818, 32819, 32820 } -- spell_mage.cpp:1422-1430

local function polymorphVisualHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local target = caster:FindNearestCreature(NPC_AUROSALIA, 30.0)
    if target == nil then
        return
    end
    target:CastSpell(target, SPELL_MAGE_POLYMORPH_FORMS[math.random(1, 6)], true)
end

RegisterSpellEvent(SPELL_MAGE_POLYMORPH_VISUAL, 5, polymorphVisualHit)
