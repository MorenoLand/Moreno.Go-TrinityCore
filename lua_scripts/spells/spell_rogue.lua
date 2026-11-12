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
-- STAY-OUT (tick 294): every other spell_rogue class — no proc hook,
--   absorb/CalcAmount hooks absent, or SpellScript-only with no
--   RegisterSpellScript in the engine. Notable: blade_flurry (also
--   entry-blocked on IDs), cheat_death (absorb + cooldown-history
--   writes), rupture (CalcAmount + REAPPLY-only leg), killing_spree
--   (periodic target list populated only by the stay-out
--   SpellScript), preparation (ResetCooldowns with family-flag
--   predicate), tricks_of_the_trade (proc + target filter).
-- STAY-OUT (tick 297, census rows 740-756, all verified in-tree):
--   blade_flurry 13877/33735/51211/65956 — MOD_MELEE_HASTE is not a
--   proc-trigger aura type (procs.go) and SelectNearbyTarget is
--   unbridged; 65956 lua_hit is a phantom (boss_faction_champions
--   cast, not the aura script).
--   cheat_death 31228 / nerves_of_steel 31130 — no absorb or
--   CalcAmount aura event (consts end at 7); no GetSpellHistory /
--   AddCooldown bridge.
--   cut_to_the_chase 51664 — rank-agnostic Slice and Dice lookup
--   needs a family-flag aura-object query; GetAura needs an exact
--   spell ID.
--   deadly_poison 2818 — no GetCastItem bridge; GetItemByPos is a
--   nil stub; no enchantment-store read. go_hit is a phantom
--   (Envenom aura-state machinery).
--   quick_recovery 31244 — ev6 carries no proc-spell info and no
--   CalcPowerCost bridge.
--   rupture 1943 — no GetTotalAttackPowerValue bridge, no CalcAmount
--   event, and the REAPPLY-only leg is unmodellable (mode 2 is
--   fresh-apply only). Census hits are phantoms (glyph-of-backstab
--   bookkeeping + header mention).
--   setup 13983 — DoCheckProc only, and no GetSelectedUnit bridge.
--   prey_on_the_weak 51685 VERIFIED CLOSED (ported tick 294).

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
local SPELL_ROGUE_DEADLY_BREW = 51625
local SPELL_ROGUE_CRIPPLING_POISON = 3409
local SPELL_ROGUE_HONOR_AMONG_THIEVES_R1 = 51698
local SPELL_ROGUE_HONOR_AMONG_THIEVES_R2 = 51700
local SPELL_ROGUE_HONOR_AMONG_THIEVES_R3 = 51701
local SPELL_ROGUE_TURN_THE_TABLES = 51627

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
