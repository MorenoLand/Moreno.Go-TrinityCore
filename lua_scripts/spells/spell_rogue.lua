-- Rogue spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_rogue.cpp.
--
-- Binding IDs come from the `// <id> - Name` header comments above
-- each class (the SpellScriptLoader name binds in DB
-- spell_script_names); the effect spell IDs are the file's enums.
--
-- PORTED (tick 294, RegisterAuraEvent):
--   spell_rog_prey_on_the_weak (spell_rogue.cpp:601-643): aura
--   51685, mode 1. PeriodicTick: when the target has a victim and
--   the target's health pct is above the victim's, and 58670 is
--   absent, the target casts 58670 on itself with BP0 = the EFFECT_0
--   CalcValue of 51685 (DBC read, 0 skips the cast), triggered;
--   otherwise the target drops 58670. No return: C++ has no
--   PreventDefaultAction.
--   spell_rog_overkill_mos (spell_rogue.cpp:515-548): template,
--   mode 1, two registrations sharing one handler shape.
--   Overkill aura 58428 ticks away 58427; Master of Subtlety aura
--   31666 ticks away 31665. Notification-only.
--   spell_rog_honor_among_thieves_proc (spell_rogue.cpp:1069-1134):
--   aura 52916, mode 2. AfterEffectApply: the aura's caster, when
--   present, casts 51699 on nil, triggered. The sibling SpellScript
--   (target filter) stays out; the aura leg stands alone. Mode 2 is
--   fresh-apply only; C++ is AURA_EFFECT_HANDLE_REAL_OR_REAPPLY, so
--   a reapply does not re-cast 51699 in Go (standing deviation).
--   The npcbot creature-caster leg casts the same spell on nil, so
--   one leg covers both player and creature casters.
--   spell_rog_vanish (spell_rogue.cpp:1175-1204): aura 11327, mode 2.
--   AfterEffectApply: the target drops all SPELL_AURA_MOD_STALKED
--   (68) auras; if already stealthed (1784) it stops; if Stealth is
--   on cooldown the cooldown is reset; then the target casts 1784
--   on nil, triggered. Mode 2 is fresh-apply only; C++ is
--   REAL_OR_REAPPLY (standing deviation, same as above). Needs the
--   player-surface RemoveAurasByType bridge (lua_player.go, tick
--   294: session.removeAurasByType already existed, it was only
--   unexposed on the player object).
-- PORTED (tick 297, RegisterAuraEvent mode 6 = OnProc):
--   spell_rog_deadly_brew (spell_rogue.cpp:235-266): aura 51625.
--   Proc actor casts Crippling Poison 3409 on the proc target,
--   triggered. DORMANT until Spell.dbc ships the 51625 proc entry.
--   spell_rog_honor_among_thieves (spell_rogue.cpp:1017-1067):
--   auras 51698/51700/51701. Proc suppressed while the caster has
--   51699; otherwise the holder casts the DBC EFFECT_0 trigger spell
--   on itself, triggered (original-caster GUID leg is a standing
--   deviation). DORMANT until Spell.dbc ships the proc entries.
--   spell_rog_turn_the_tables (spell_rogue.cpp:1138-1173): aura
--   51627. The caster casts the DBC EFFECT_0 trigger spell on nil,
--   triggered. DORMANT until Spell.dbc ships the 51627 proc entry.
-- PORTED (tick 298, RegisterAuraEvent mode 6 = OnProc):
--   spell_rog_cut_to_the_chase (spell_rogue.cpp:196-233): aura
--   51664. Refreshes the actor's Slice and Dice (any rank, via
--   the new player GetAuraOfRankedSpell) to the spell-info max
--   duration (aura GetSpellMaxDuration, SpellInfo::GetMaxDuration
--   twin; 0 while Spell.dbc is unshipped falls back to the live
--   aura's GetMaxDuration). The tick-297 stay-out for this class
--   is superseded by the two new bridges. DORMANT until Spell.dbc
--   ships the 51664 proc entry.
--   spell_rog_glyph_of_backstab (spell_rogue.cpp:769-800, census
--   row 244): aura 56800. Proc actor casts the triggered leg
--   63975 on the proc target, triggered. The 63975 SpellScript
--   leg stays out (rupture BonusDuration via GetScript,
--   unbridged). DORMANT until Spell.dbc ships the 56800 proc
--   entry.
-- PORTED (census rows 729-760, RegisterAuraEvent OnProc = 6):
--   spell_rog_quick_recovery (spell_rogue.cpp:646-685, census row
--   751): aura 31244. PreventDefaultAction (the true return); the
--   proc spell's power cost (player GetSpellPowerCost, the
--   CalcPowerCost twin) times the effect amount pct is cast as
--   Quick Recovery Energy 31663 on the actor (the C++ nil-target
--   self-cast), triggered, BP0 = the amount. A proc with no event
--   spell or a zero amount casts nothing. Player surface only:
--   GetSpellPowerCost has no creature twin, and C++ carries no
--   npcbot leg here. The tick-294/298 stay-out ("no CalcPowerCost
--   bridge") is superseded by the GetSpellPowerCost bridge.
--   DORMANT until Spell.dbc ships the 31244 proc entry and the
--   31663 + proc-spell cost rows.
-- STAY-OUT (tick 294, re-verified tick 298 against current
--   runtime): blade_flurry (13877/33735/51211/65956, census rows
--   741-744; census hook label DoEffectCalcAmount is WRONG, real
--   hooks are DoCheckProc + OnEffectProc): EFFECT_0
--   MOD_MELEE_HASTE is not in isProcTriggerAuraType (procs.go),
--   and CheckProc needs SelectNearbyTarget, which has no bridge
--   anywhere in the engine — casting 22482 on the engine's
--   procTarget (the victim) would double-hit the victim instead
--   of the nearby target C++ picks. Engine work. The 65956
--   lua_hit is a false positive (boss_faction_champions.lua:240
--   casts 65956 as npc_toc_rogue Blade Flurry, not the aura
--   script).
--   cheat_death (31228, row 745): PORTED below (ev9 + the new
--   AddSpellCooldown bridges, this tick); the tick-294/298 stay-out
--   (no absorb hook, unbridged cooldown writes) is superseded by
--   the AuraEventOnEffectAbsorb bridge and the 31228 unlimited-pool
--   leg in absorption.go.
--   deadly_poison (2818, row 748): SpellScript BeforeHit/
--   AfterHit only; no RegisterSpellScript in the engine, and
--   GetCastItem is open engine work; GetItemByPos is a nil stub
--   and there is no enchantment-store read. The go_hit is a false
--   positive (SpellInfo.cpp line numbers in comments).
--   nerves_of_steel (31130, row 749): CalcAmount + OnEffectAbsorb;
--   no absorb hook. Engine work.
--   rupture (1943, row 752): CalcAmount (AP x combo points) +
--   AfterEffectApply reapply reset; no CalcAmount bridge and no
--   GetTotalAttackPowerValue bridge; the REAPPLY-only leg is
--   unmodellable (mode 2 is fresh-apply only). The lua_hit is a
--   false positive (1943xx gameobject IDs in
--   boss_hodir/boss_razorscale).
--   setup (13983, row 753): DoCheckProc ONLY (census
--   HandleDummy/OnEffectHitTarget is WRONG); the gate needs
--   GetSelectedUnit, which has no bridge. Engine work.
--   killing_spree (51690, row 241): periodic target list populated
--   only by the stay-out SpellScript. preparation (14185, row 243):
--   SpellScript, ResetCooldowns with a family-flag predicate; no
--   RegisterSpellScript. glyph_of_backstab_triggered (63975, row
--   245): SpellScript; stays out — the Go default leg owns it
--   (spells.go:53830 player + boss_ai.go:7952 creature); a Lua ev5
--   leg would double-fire (finishSpellCast fires ev5 at
--   spells.go:14735 on the same cast). shiv (5938, row 246):
--   SpellScript only.
--   tricks_of_the_trade (57934/59628): proc + redirect-target
--   state via GetScript + threat redirect, unbridged.
--   prey_on_the_weak 51685 VERIFIED CLOSED (ported tick 294).
--   Every other spell_rogue class not listed under PORTED above.
local SPELL_ROGUE_PREY_ON_THE_WEAK = 51685
local SPELL_ROGUE_PREY_ON_THE_WEAK_BUFF = 58670
local SPELL_ROGUE_OVERKILL = 58428
local SPELL_ROGUE_OVERKILL_BUFF = 58427
local SPELL_ROGUE_MASTER_OF_SUBTLETY = 31666
local SPELL_ROGUE_MASTER_OF_SUBTLETY_BUFF = 31665
local SPELL_ROGUE_HONOR_AMONG_THIEVES_PROC = 52916
local SPELL_ROGUE_HONOR_AMONG_THIEVES_2 = 51699
local SPELL_ROGUE_VANISH = 11327
local SPELL_ROGUE_STEALTH = 1784
local SPELL_ROGUE_CUT_TO_THE_CHASE = 51664
local SPELL_ROGUE_SLICE_AND_DICE_R1 = 5171
local SPELL_ROGUE_GLYPH_OF_BACKSTAB = 56800
local SPELL_ROGUE_GLYPH_OF_BACKSTAB_TRIGGER = 63975
local SPELL_ROGUE_DEADLY_BREW = 51625
local SPELL_ROGUE_CRIPPLING_POISON = 3409
local SPELL_ROGUE_HONOR_AMONG_THIEVES_R1 = 51698
local SPELL_ROGUE_HONOR_AMONG_THIEVES_R2 = 51700
local SPELL_ROGUE_HONOR_AMONG_THIEVES_R3 = 51701
local SPELL_ROGUE_TURN_THE_TABLES = 51627
local SPELL_ROGUE_CHEAT_DEATH = 31228 -- spell_rogue.cpp:109
local SPELL_ROGUE_CHEAT_DEATH_COOLDOWN = 31231 -- spell_rogue.cpp:39
local SPELL_ROGUE_QUICK_RECOVERY = 31244 -- spell_rogue.cpp:645 (class spell_rog_quick_recovery)
local SPELL_ROGUE_QUICK_RECOVERY_ENERGY = 31663 -- spell_rogue.cpp:55

local SPELL_AURA_MOD_STALKED = 68

local function preyOnTheWeakTick(event, aura, target)
    local victim = target:GetVictim()
    if victim and target:GetHealthPct() > victim:GetHealthPct() then
        if not target:HasAura(SPELL_ROGUE_PREY_ON_THE_WEAK_BUFF) then
            local bp = aura:GetSpellEffectCalcValue(SPELL_ROGUE_PREY_ON_THE_WEAK, 0)
            if bp ~= 0 then
                target:CastCustomSpell(target, SPELL_ROGUE_PREY_ON_THE_WEAK_BUFF, true, bp)
            end
        end
    else
        target:RemoveAura(SPELL_ROGUE_PREY_ON_THE_WEAK_BUFF)
    end
end

RegisterAuraEvent(SPELL_ROGUE_PREY_ON_THE_WEAK, 1, preyOnTheWeakTick)

local function overkillTick(event, aura, target)
    target:RemoveAura(SPELL_ROGUE_OVERKILL_BUFF)
end

local function masterOfSubtletyTick(event, aura, target)
    target:RemoveAura(SPELL_ROGUE_MASTER_OF_SUBTLETY_BUFF)
end

RegisterAuraEvent(SPELL_ROGUE_OVERKILL, 1, overkillTick)
RegisterAuraEvent(SPELL_ROGUE_MASTER_OF_SUBTLETY, 1, masterOfSubtletyTick)

local function honorAmongThievesApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(nil, SPELL_ROGUE_HONOR_AMONG_THIEVES_2, true)
    end
end

RegisterAuraEvent(SPELL_ROGUE_HONOR_AMONG_THIEVES_PROC, 2, honorAmongThievesApply)

local function vanishApply(event, aura, target)
    target:RemoveAurasByType(SPELL_AURA_MOD_STALKED)
    if target:HasAura(SPELL_ROGUE_STEALTH) then
        return
    end
    if target:HasSpellCooldown(SPELL_ROGUE_STEALTH) then
        target:ResetSpellCooldown(SPELL_ROGUE_STEALTH)
    end
    target:CastSpell(nil, SPELL_ROGUE_STEALTH, true)
end

RegisterAuraEvent(SPELL_ROGUE_VANISH, 2, vanishApply)

-- PORTED (tick 298, RegisterAuraEvent OnProc = 6):
--   spell_rog_cut_to_the_chase::HandleProc
--   (spell_rogue.cpp:196-233): aura 51664, EFFECT_0
--   SPELL_AURA_DUMMY. PreventDefaultAction (the true return);
--   refreshes the actor's Slice and Dice to its 5-combo-point
--   maximum: duration and max duration both become the spell-info
--   max (GetSpellMaxDuration, the SpellInfo::GetMaxDuration twin;
--   0 while Spell.dbc is unshipped falls back to the live aura's
--   GetMaxDuration, same DBC row). The C++ lookup is by family
--   flag, matching any SnD rank, so the aura is found via the new
--   player GetAuraOfRankedSpell (5171 first rank); a missing SnD
--   skips the refresh, like the C++ null-AuraEffect gate. DORMANT
--   until Spell.dbc ships the 51664 proc entry. Player surface
--   only: the talent aura lives on rogues; no npcbot leg in C++.
local function cutToTheChaseProc(event, aura, target, actor, procTarget, damage)
    local holder = actor or target
    if holder == nil then
        return true
    end
    local snd = holder:GetAuraOfRankedSpell(SPELL_ROGUE_SLICE_AND_DICE_R1)
    if snd == nil then
        return true
    end
    local countMax = snd:GetSpellMaxDuration()
    if countMax == 0 then
        countMax = snd:GetMaxDuration()
    end
    if countMax <= 0 then
        return true
    end
    snd:SetDuration(countMax)
    snd:SetMaxDuration(snd:GetDuration())
    return true
end

RegisterAuraEvent(SPELL_ROGUE_CUT_TO_THE_CHASE, 6, cutToTheChaseProc)

-- PORTED (tick 298, RegisterAuraEvent OnProc = 6; census row 244):
--   spell_rog_glyph_of_backstab::HandleProc
--   (spell_rogue.cpp:769-800): aura 56800, EFFECT_0
--   SPELL_AURA_DUMMY. PreventDefaultAction (the true return); the
--   actor casts the triggered leg (63975) on the proc target,
--   triggered (the aurEff overload). The 63975 SpellScript leg
--   stays out: the Go default leg owns it (spells.go:53830 player
--   + boss_ai.go:7952 creature) — a Lua ev5 leg would double-fire,
--   since finishSpellCast fires ev5 at spells.go:14735 on the same
--   cast. DORMANT until Spell.dbc ships the 56800 proc entry.
local function glyphOfBackstabProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil then
        return true
    end
    actor:CastSpell(procTarget, SPELL_ROGUE_GLYPH_OF_BACKSTAB_TRIGGER, true)
    return true
end

RegisterAuraEvent(SPELL_ROGUE_GLYPH_OF_BACKSTAB, 6, glyphOfBackstabProc)


-- C++ spell_rog_deadly_brew_AuraScript::HandleProc
-- (spell_rogue.cpp:235-266): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction; the proc actor casts Crippling Poison 3409
-- on the proc target, triggered (aurEff args). DORMANT until
-- Spell.dbc ships the 51625 proc entry.
local function deadlyBrewProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or procTarget == nil then
        return true
    end
    actor:CastSpell(procTarget, SPELL_ROGUE_CRIPPLING_POISON, true)
    return true
end

RegisterAuraEvent(SPELL_ROGUE_DEADLY_BREW, 6, deadlyBrewProc)

-- C++ spell_rog_honor_among_thieves_AuraScript::HandleProc
-- (spell_rogue.cpp:1017-1067): OnEffectProc, EFFECT_0
-- PROC_TRIGGER_SPELL. CheckProc: the aura's caster must exist and
-- must lack 51699, else the proc is suppressed (Lua: return true =
-- PreventDefaultAction, which suppresses the default trigger leg).
-- Otherwise the aura holder casts the DBC EFFECT_0 trigger spell on
-- itself, triggered. Standing deviation: C++ passes the aura caster's
-- GUID as original caster; Lua has no twin, so the cast's caster is
-- the holder. DORMANT until Spell.dbc ships the proc entries.
local function honorAmongThievesProc(event, aura, target, actor, procTarget, damage)
    local caster = aura:GetCaster()
    if caster == nil or caster:HasAura(SPELL_ROGUE_HONOR_AMONG_THIEVES_2) then
        return true
    end
    if target == nil then
        return true
    end
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    target:CastSpell(target, triggerSpell, true)
    return true
end

RegisterAuraEvent(SPELL_ROGUE_HONOR_AMONG_THIEVES_R1, 6, honorAmongThievesProc)
RegisterAuraEvent(SPELL_ROGUE_HONOR_AMONG_THIEVES_R2, 6, honorAmongThievesProc)
RegisterAuraEvent(SPELL_ROGUE_HONOR_AMONG_THIEVES_R3, 6, honorAmongThievesProc)

-- C++ spell_rog_turn_the_tables_AuraScript::HandleProc
-- (spell_rogue.cpp:1138-1173): OnEffectProc, EFFECT_0
-- PROC_TRIGGER_SPELL. PreventDefaultAction; the aura's caster casts
-- the DBC EFFECT_0 trigger spell on nil, triggered. DORMANT until
-- Spell.dbc ships the 51627 proc entry.
local function turnTheTablesProc(event, aura, target, actor, procTarget, damage)
    local caster = aura:GetCaster()
    if caster == nil then
        return true
    end
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    caster:CastSpell(nil, triggerSpell, true)
    return true
end

RegisterAuraEvent(SPELL_ROGUE_TURN_THE_TABLES, 6, turnTheTablesProc)

-- C++ spell_rog_cheat_death::Absorb (spell_rogue.cpp:109-192):
-- 31228 EFFECT_0 SPELL_AURA_SCHOOL_ABSORB. The pool is unlimited
-- (DoEffectCalcAmount sets -1; absorption.go's unlimited set), so the
-- handler computes the absorb from the damage: on a lethal hit, with
-- the 31231 cooldown clear and the EFFECT_0 CalcValue chance roll
-- passed, the rogue casts 31231 on itself (triggered), writes the
-- 1-minute 31231 cooldown, and absorbs down to 10% of max health
-- (or all of the damage when already below 10%). A non-lethal hit,
-- a live cooldown, or a failed roll absorbs nothing (return 0). The
-- player leg is the live one (Load gates on TYPEID_PLAYER); the
-- npcbot creature leg shares the shape — one handler covers both,
-- the absorb hook passes the holder as target on either leg.
-- DORMANT until Spell.dbc ships the 31228 row (the chance reads 0
-- without it, like moonkin 69366).
local function cheatDeathAbsorb(event, aura, target, damage, absorbAmount)
    if target == nil then
        return 0
    end
    if damage < target:GetHealth() then
        return 0
    end
    if target:HasSpellCooldown(SPELL_ROGUE_CHEAT_DEATH_COOLDOWN) then
        return 0
    end
    local chance = aura:GetSpellEffectCalcValue(SPELL_ROGUE_CHEAT_DEATH, 0)
    if chance <= 0 or math.random(1, 100) > chance then
        return 0
    end
    target:CastSpell(target, SPELL_ROGUE_CHEAT_DEATH_COOLDOWN, true)
    target:AddSpellCooldown(SPELL_ROGUE_CHEAT_DEATH_COOLDOWN, 60000)
    local health10 = target:CountPctFromMaxHealth(10)
    if target:GetHealth() > health10 then
        return damage - target:GetHealth() + health10
    end
    return damage
end

RegisterAuraEvent(SPELL_ROGUE_CHEAT_DEATH, 9, cheatDeathAbsorb)

-- C++ spell_rog_quick_recovery_AuraScript::HandleProc
-- (spell_rogue.cpp:651-685): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction (the true return); the proc spell's power
-- cost (player GetSpellPowerCost, the CalcPowerCost twin) times the
-- effect amount pct is cast as Quick Recovery Energy 31663 on the
-- actor (the C++ nil-target self-cast), triggered, BP0 = the amount.
-- A proc with no event spell (melee/kill procs) or a zero amount
-- casts nothing. Player surface only: GetSpellPowerCost has no
-- creature twin, and C++ carries no npcbot leg here. DORMANT until
-- Spell.dbc ships the 31244 proc entry and the 31663 + proc-spell
-- cost rows.
local function quickRecoveryProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if actor == nil or actor:GetTypeId() ~= 4 then
        return true
    end
    if procEvent == nil then
        return true
    end
    local procSpellId = procEvent:GetSpellId()
    if procSpellId == nil or procSpellId == 0 then
        return true
    end
    local cost = actor:GetSpellPowerCost(procSpellId)
    local amount = math.floor(cost * aura:GetAmount() / 100)
    if amount <= 0 then
        return true
    end
    actor:CastCustomSpell(actor, SPELL_ROGUE_QUICK_RECOVERY_ENERGY, true, amount)
    return true
end

RegisterAuraEvent(SPELL_ROGUE_QUICK_RECOVERY, 6, quickRecoveryProc)
