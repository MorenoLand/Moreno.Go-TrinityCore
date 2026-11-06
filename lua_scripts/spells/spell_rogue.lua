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
-- STAY-OUT (tick 294): every other spell_rogue class — no proc hook,
--   absorb/CalcAmount hooks absent, or SpellScript-only with no
--   RegisterSpellScript in the engine. Notable: blade_flurry (also
--   entry-blocked on IDs), cheat_death (absorb + cooldown-history
--   writes), rupture (CalcAmount + REAPPLY-only leg), killing_spree
--   (periodic target list populated only by the stay-out
--   SpellScript), preparation (ResetCooldowns with family-flag
--   predicate), tricks_of_the_trade (proc + target filter).

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
