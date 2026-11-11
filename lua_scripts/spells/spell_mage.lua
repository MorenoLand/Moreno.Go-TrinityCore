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

-- NOT IN LUA: spell_mage_arcane_potency (31571, spell_mage.cpp:143-193)
-- — CheckProc filters the proc spell by SpellIconID (Clearcasting /
-- Presence of Mind) and HandleProc casts the rank-indexed trigger
-- (57529/57531); ev6 carries no proc-spell object. STAYS OUT until
-- ev6 event-spell forwarding lands (open engine work, same as
-- census rows 547-551 + 572 + 597 + 601 + 603).
-- NOT IN LUA: spell_mage_blast_wave (11113, spell_mage.cpp:195-226)
-- — SpellScript OnEffectHitTarget KNOCK_BACK (glyph 62126 gate);
-- no RegisterSpellScript bridge. STAYS OUT.
-- NOT IN LUA: spell_mage_burnout (44449, spell_mage.cpp:294-333)
-- — CheckProc needs damageInfo->GetSpellInfo() and HandleProc needs
-- CalcPowerCost(target, school mask); ev6 passes only a damage
-- number, no damage-info spell/school. STAYS OUT until ev6
-- damage-info forwarding lands.
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
--   GetMaxPower(0). DORMANT until Spell.dbc ships the 29441 proc entry.
-- CENSUS CORRECTION: row 610 hooks are HandleDummy only; real is
-- OnEffectProc EFFECT_0 PROC_TRIGGER_SPELL. Rows 608/611/612 hooks
-- are likewise wrong (608: DoCheckProc + OnEffectProc; 611: DoCheckProc
-- + OnEffectProc EFFECT_1 DUMMY; 612: OnEffectProc EFFECT_0
-- OVERRIDE_CLASS_SCRIPTS).
