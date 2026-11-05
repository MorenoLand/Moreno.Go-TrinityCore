-- Holiday spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_holiday.cpp.
--
-- PORTED (tick 197, RegisterAuraEvent OnRemove = 3):
--   spell_brewfest_exhausted_ram (43332, spell_holiday.cpp:1294-1300):
--   EFFECT_0 MOD_DECREASE_SPEED. OnRemove the target casts Ram
--   Level Neutral (43310) on itself, triggered. No Load gates in
--   C++; no PreventDefaultAction. (C++ registers this arm with the
--   AuraEffectApplyFn type — a TrinityCore quirk, no behavior, same
--   as the direbrew disarm apply arm.)
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the rider's triggered
-- self-cast lands non-triggered; pre-existing bridge gap, same as
-- all prior player-target ports.
-- PORTED (tick 198, RegisterAuraEvent, at the tail):
--   spell_brewfest_apple_trap OnApply (43450) and
--   spell_brewfest_giddyup OnChange (42924, apply + remove legs).

local SPELL_BREWFEST_EXHAUSTED_RAM = 43332 -- spell_holiday.cpp:1294
local SPELL_RAM_LEVEL_NEUTRAL = 43310 -- spell_holiday.cpp:1072

-- C++ spell_brewfest_exhausted_ram::OnRemove (spell_holiday.cpp:1297-1300).
local function exhaustedRamRemove(event, aura, target)
    target:CastSpell(target, SPELL_RAM_LEVEL_NEUTRAL, true)
end

RegisterAuraEvent(SPELL_BREWFEST_EXHAUSTED_RAM, 3, exhaustedRamRemove)

-- C++ spell_brewfest_apple_trap::OnApply (spell_holiday.cpp:1269-1272):
-- EFFECT_0 SPELL_AURA_FORCE_REACTION. On apply the target drops the
-- Ram Fatigue aura (43052).
local SPELL_BREWFEST_APPLE_TRAP = 43450 -- spell_holiday.cpp:1260
local SPELL_RAM_FATIGUE = 43052 -- spell_holiday.cpp:1076

local function appleTrapApply(event, aura, target)
    target:RemoveAura(SPELL_RAM_FATIGUE)
end

RegisterAuraEvent(SPELL_BREWFEST_APPLE_TRAP, 2, appleTrapApply)

-- C++ spell_brewfest_giddyup::OnChange (spell_holiday.cpp:1096-1137):
-- registered as AfterEffectApply AND OnEffectRemove, both with
-- AURA_EFFECT_HANDLE_CHANGE_AMOUNT_MASK, so the one body runs on
-- both events. No ram aura (rental 43883 / swift 43880) -> the
-- giddyup aura removes itself; exhausted ram (43332) -> nothing.
-- Otherwise the stack count picks the speed: 1 trot 42992, 6 canter
-- 42993, 11 gallop 42994 (each drops the other level auras first).
-- On removal with the DEFAULT mode the target drops trot and casts
-- Ram Level Neutral 43310. The mode gate is Aura:GetRemoveMode (new
-- tick 198: SpellAuraDefines.h:61-69 values); on apply it reads 0
-- (NONE), so the remove leg is skipped there, as in C++ (the
-- application carries no remove mode outside a removal).
-- Gaps: no change-amount hook exists, so mid-life stack changes do
-- not re-run the switch — only the apply (stack 1) and remove (final
-- stacks) events fire it; the OnPeriodic leg (RemoveAuraFromStack)
-- stays out — no RemoveAuraFromStack bridge.
local SPELL_BREWFEST_GIDDYUP = 42924 -- spell_holiday.cpp:1087
local SPELL_RENTAL_RACING_RAM = 43883 -- spell_holiday.cpp:1069
local SPELL_SWIFT_WORK_RAM = 43880 -- spell_holiday.cpp:1070
local SPELL_EXHAUSTED_RAM = 43332 -- spell_holiday.cpp:1077
local SPELL_RAM_TROT = 42992 -- spell_holiday.cpp:1073
local SPELL_RAM_CANTER = 42993 -- spell_holiday.cpp:1074
local SPELL_RAM_GALLOP = 42994 -- spell_holiday.cpp:1075
local AURA_REMOVE_BY_DEFAULT = 1 -- SpellAuraDefines.h:64

local function giddyupChange(event, aura, target)
    if not target:HasAura(SPELL_RENTAL_RACING_RAM) and not target:HasAura(SPELL_SWIFT_WORK_RAM) then
        target:RemoveAura(SPELL_BREWFEST_GIDDYUP)
        return
    end
    if target:HasAura(SPELL_EXHAUSTED_RAM) then
        return
    end
    local stacks = aura:GetStackAmount()
    if stacks == 1 then
        target:RemoveAura(SPELL_RAM_LEVEL_NEUTRAL)
        target:RemoveAura(SPELL_RAM_CANTER)
        target:CastSpell(target, SPELL_RAM_TROT, true)
    elseif stacks == 6 then
        target:RemoveAura(SPELL_RAM_TROT)
        target:RemoveAura(SPELL_RAM_GALLOP)
        target:CastSpell(target, SPELL_RAM_CANTER, true)
    elseif stacks == 11 then
        target:RemoveAura(SPELL_RAM_CANTER)
        target:CastSpell(target, SPELL_RAM_GALLOP, true)
    end
    if aura:GetRemoveMode() == AURA_REMOVE_BY_DEFAULT then
        target:RemoveAura(SPELL_RAM_TROT)
        target:CastSpell(target, SPELL_RAM_LEVEL_NEUTRAL, true)
    end
end

RegisterAuraEvent(SPELL_BREWFEST_GIDDYUP, 2, giddyupChange)
RegisterAuraEvent(SPELL_BREWFEST_GIDDYUP, 3, giddyupChange)
