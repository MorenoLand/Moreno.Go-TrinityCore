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
-- Gaps (tick 295: the first is closed — event 4 now re-runs the
-- switch on stack-raising merges, see the registration below; the
-- OnPeriodic leg (RemoveAuraFromStack) stays out — no
-- RemoveAuraFromStack bridge.
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
-- Tick 295: the change-amount gap is closed — event 4 fires when a
-- merge re-cast raises the stack count (spells.go:37239), so mid-life
-- stack changes re-run the switch (trot 1 / canter 6 / gallop 11).
-- The OnPeriodic RemoveAuraFromStack leg still stays out (no bridge).
RegisterAuraEvent(SPELL_BREWFEST_GIDDYUP, 4, giddyupChange)

-- PORTED (tick 295, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_hallow_end_candy_pirate::HandleApply/HandleRemove
--   (spell_holiday.cpp:180-202): EFFECT_0 MOD_INCREASE_SWIM_SPEED,
--   REAL. On apply the target self-casts the Defias Pirate model
--   aura by native gender (female 44742, male 44743), triggered;
--   on remove it drops the same aura.
--
-- PORTED (tick 295, RegisterAuraEvent OnPeriodic = 1):
--   spell_brewfest_ram::OnPeriodic (spell_holiday.cpp:1167-1212):
--   EFFECT_1 PERIODIC_DUMMY, one handler for all four ram-level
--   auras (43310 neutral, 42992 trot, 42993 canter, 42994 gallop),
--   switching on the aura ID; skipped while Exhausted Ram 43332 is
--   up. Neutral: Ram Fatigue 43052 loses 4 stacks; trot: loses 2
--   stacks and on tick 4 casts the green speed bunny 43345;
--   canter: casts 43052 with the stack set to 1 and on tick 8 casts
--   the yellow bunny 43346; gallop: casts 43052 with the stack set
--   to 4 (fatigue already up) or 5 and on tick 8 casts the red
--   bunny 43347; all casts triggered. No PreventDefaultAction in
--   C++, so the Go default tick still fires (no return). The C++
--   ModStackAmount legs ride aura:SetStackAmount (absolute); a
--   reduction to 0 or below drops the fatigue aura instead, matching
--   the C++ removal. The C++ SPELLVALUE_AURA_STACK cast mod has no
--   bridge, so the canter/gallop stack sets ride cast-then-set.
--   spell_midsummer_ribbon_pole_periodic_visual::PeriodicTick
--   (spell_holiday.cpp:1658-1668): EFFECT_0 PERIODIC_DUMMY. The
--   visual aura removes itself unless the target still has a ribbon
--   pole test aura (29705 / 29726 / 29727). No PreventDefaultAction
--   in C++ (no return).
--
-- PORTED (tick 295, RegisterAuraEvent OnChangeAmount = 4):
--   spell_brewfest_ram_fatigue::OnApply (spell_holiday.cpp:1231-1249):
--   EFFECT_0 DUMMY. C++ binds to AfterEffectApply REAL_OR_REAPPLY
--   and fires when the stack count reaches 101; Go mode 2 never
--   re-fires on refresh, so the stack-101 leg rides event 4 (fires
--   on stack-raising merges). At 101 the target drops all ram-level
--   auras (43310, 42992, 42993, 42994) and Giddyup 42924, then
--   self-casts Exhausted Ram 43332, triggered.
--   spell_midsummer_braziers_hit::HandleEffectApply
--   (spell_holiday.cpp:1521-1540): EFFECT_0 DUMMY. C++ binds to
--   AfterEffectApply REAPPLY-only; Go event 4 is the same fire
--   condition. When the player has Torch Tossing Training 45716 at
--   8 stacks or Practice 46630 at 20, it self-casts the team
--   success aura (Alliance 45719, Horde 46651), triggered, and the
--   braziers aura removes itself.
--
-- CLOSED (tick 295 census, corrected this tick): the remaining holiday
--   classes are SpellScript-only, plus romantic_picnic (no
--   nearby-player enumeration), pilgrims bounty a_serving_of
--   (vehicle/seat chain), brewfest barker bunny (no player Talk
--   bridge), and the DB-bound IDs (pilgrims buff-food x5, midsummer
--   ribbon pole channel) whose aura IDs live only in world DB
--   spell_script_names. The tick-295 "no RegisterSpellScript bridge"
--   line was imprecise: OnEffectHit rides RegisterSpellEvent ev4,
--   which is how 24930 Hallow's End Candy is ported above.

local SPELL_HALLOW_END_CANDY_PIRATE = 24926 -- spell_holiday.cpp:169 (class spell_hallow_end_candy_pirate)
local SPELL_HALLOWS_END_CANDY_FEMALE_DEFIAS_PIRATE = 44742 -- spell_holiday.cpp:122
local SPELL_HALLOWS_END_CANDY_MALE_DEFIAS_PIRATE = 44743 -- spell_holiday.cpp:123
local GENDER_FEMALE = 1 -- SharedDefines.h:77

local SPELL_RAM_TROT = 42992 -- spell_holiday.cpp:1073
local SPELL_RAM_CANTER = 42993 -- spell_holiday.cpp:1074
local SPELL_RAM_GALLOP = 42994 -- spell_holiday.cpp:1075
local SPELL_BREWFEST_QUEST_SPEED_BUNNY_GREEN = 43345 -- spell_holiday.cpp:1081
local SPELL_BREWFEST_QUEST_SPEED_BUNNY_YELLOW = 43346 -- spell_holiday.cpp:1082
local SPELL_BREWFEST_QUEST_SPEED_BUNNY_RED = 43347 -- spell_holiday.cpp:1083
local SPELL_TORCH_TOSSING_TRAINING = 45716 -- spell_holiday.cpp:1496
local SPELL_TORCH_TOSSING_PRACTICE = 46630 -- spell_holiday.cpp:1497
local SPELL_TORCH_TOSSING_TRAINING_SUCCESS_ALLIANCE = 45719 -- spell_holiday.cpp:1498
local SPELL_TORCH_TOSSING_TRAINING_SUCCESS_HORDE = 46651 -- spell_holiday.cpp:1499
local SPELL_MIDSUMMER_BRAZIERS_HIT = 45724 -- spell_holiday.cpp:1505 (class spell_midsummer_braziers_hit)
local SPELL_MIDSUMMER_RIBBON_POLE_PERIODIC_VISUAL = 45406 -- spell_holiday.cpp:1643 (class spell_midsummer_ribbon_pole_periodic_visual)
local SPELL_TEST_RIBBON_POLE_1 = 29705 -- spell_holiday.cpp:1593
local SPELL_TEST_RIBBON_POLE_2 = 29726 -- spell_holiday.cpp:1594
local SPELL_TEST_RIBBON_POLE_3 = 29727 -- spell_holiday.cpp:1595
local TEAM_ALLIANCE = 0 -- GetTeam twin (lua_player.go:324)
local TEAM_HORDE = 1

local function hallowEndCandyPirateApply(event, aura, target)
    if target:GetNativeGender() == GENDER_FEMALE then
        target:CastSpell(target, SPELL_HALLOWS_END_CANDY_FEMALE_DEFIAS_PIRATE, true)
    else
        target:CastSpell(target, SPELL_HALLOWS_END_CANDY_MALE_DEFIAS_PIRATE, true)
    end
end

RegisterAuraEvent(SPELL_HALLOW_END_CANDY_PIRATE, 2, hallowEndCandyPirateApply)

local function hallowEndCandyPirateRemove(event, aura, target)
    if target:GetNativeGender() == GENDER_FEMALE then
        target:RemoveAura(SPELL_HALLOWS_END_CANDY_FEMALE_DEFIAS_PIRATE)
    else
        target:RemoveAura(SPELL_HALLOWS_END_CANDY_MALE_DEFIAS_PIRATE)
    end
end

RegisterAuraEvent(SPELL_HALLOW_END_CANDY_PIRATE, 3, hallowEndCandyPirateRemove)

-- PORTED (this tick, RegisterSpellEvent OnEffectHit = 4):
--   spell_hallow_end_candy::HandleDummy (spell_holiday.cpp:139-142,
--   registered OnEffectHit EFFECT_0 DUMMY): on effect 0 the caster
--   self-casts one of the four candy auras at random (orange giant
--   24924, skeleton 24925, pirate 24926, ghost 24927), triggered.
--   Fires on the player path (spells.go:14348) and the creature path
--   (boss_ai.go:3365), once per non-zero effect with the effect
--   index. Corrects the tick-295 header note below: the
--   SpellScript-only classes are not all unbridgeable — OnEffectHit
--   rides ev4 (muru ev5 precedent).
local SPELL_HALLOWS_END_CANDY = 24930 -- spell_holiday.cpp:127
local SPELL_HALLOWS_END_CANDY_ORANGE_GIANT = 24924 -- spell_holiday.cpp:118
local SPELL_HALLOWS_END_CANDY_SKELETON = 24925 -- spell_holiday.cpp:119
local SPELL_HALLOWS_END_CANDY_GHOST = 24927 -- spell_holiday.cpp:121

local HALLOW_END_CANDY_SPELLS = {
    SPELL_HALLOWS_END_CANDY_ORANGE_GIANT,
    SPELL_HALLOWS_END_CANDY_SKELETON,
    SPELL_HALLOW_END_CANDY_PIRATE,
    SPELL_HALLOWS_END_CANDY_GHOST,
}

local function hallowEndCandyDummy(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, HALLOW_END_CANDY_SPELLS[math.random(#HALLOW_END_CANDY_SPELLS)], true)
end

RegisterSpellEvent(SPELL_HALLOWS_END_CANDY, 4, hallowEndCandyDummy)

local function dropFatigueStacks(target, amount)
    local fatigue = target:GetAura(SPELL_RAM_FATIGUE)
    if fatigue ~= nil then
        local stacks = fatigue:GetStackAmount() - amount
        if stacks <= 0 then
            target:RemoveAura(SPELL_RAM_FATIGUE)
        else
            fatigue:SetStackAmount(stacks)
        end
    end
end

local function brewfestRamTick(event, aura, target)
    if target:HasAura(SPELL_EXHAUSTED_RAM) then
        return
    end
    local id = aura:GetAuraId()
    if id == SPELL_RAM_LEVEL_NEUTRAL then
        dropFatigueStacks(target, 4)
    elseif id == SPELL_RAM_TROT then
        dropFatigueStacks(target, 2)
        if aura:GetTickNumber() == 4 then
            target:CastSpell(target, SPELL_BREWFEST_QUEST_SPEED_BUNNY_GREEN, true)
        end
    elseif id == SPELL_RAM_CANTER then
        target:CastSpell(target, SPELL_RAM_FATIGUE, true)
        local fatigue = target:GetAura(SPELL_RAM_FATIGUE)
        if fatigue ~= nil then
            fatigue:SetStackAmount(1)
        end
        if aura:GetTickNumber() == 8 then
            target:CastSpell(target, SPELL_BREWFEST_QUEST_SPEED_BUNNY_YELLOW, true)
        end
    elseif id == SPELL_RAM_GALLOP then
        local hadFatigue = target:HasAura(SPELL_RAM_FATIGUE)
        target:CastSpell(target, SPELL_RAM_FATIGUE, true)
        local fatigue = target:GetAura(SPELL_RAM_FATIGUE)
        if fatigue ~= nil then
            if hadFatigue then
                fatigue:SetStackAmount(4)
            else
                fatigue:SetStackAmount(5)
            end
        end
        if aura:GetTickNumber() == 8 then
            target:CastSpell(target, SPELL_BREWFEST_QUEST_SPEED_BUNNY_RED, true)
        end
    end
end

RegisterAuraEvent(SPELL_RAM_LEVEL_NEUTRAL, 1, brewfestRamTick)
RegisterAuraEvent(SPELL_RAM_TROT, 1, brewfestRamTick)
RegisterAuraEvent(SPELL_RAM_CANTER, 1, brewfestRamTick)
RegisterAuraEvent(SPELL_RAM_GALLOP, 1, brewfestRamTick)

local function brewfestRamFatigueChangeAmount(event, aura, target)
    if aura:GetStackAmount() == 101 then
        target:RemoveAura(SPELL_RAM_LEVEL_NEUTRAL)
        target:RemoveAura(SPELL_RAM_TROT)
        target:RemoveAura(SPELL_RAM_CANTER)
        target:RemoveAura(SPELL_RAM_GALLOP)
        target:RemoveAura(SPELL_BREWFEST_GIDDYUP)
        target:CastSpell(target, SPELL_EXHAUSTED_RAM, true)
    end
end

RegisterAuraEvent(SPELL_RAM_FATIGUE, 4, brewfestRamFatigueChangeAmount)

local function midsummerBraziersHitChangeAmount(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    local stacks = aura:GetStackAmount()
    if (target:HasAura(SPELL_TORCH_TOSSING_TRAINING) and stacks == 8)
        or (target:HasAura(SPELL_TORCH_TOSSING_PRACTICE) and stacks == 20) then
        if target:GetTeam() == TEAM_ALLIANCE then
            target:CastSpell(target, SPELL_TORCH_TOSSING_TRAINING_SUCCESS_ALLIANCE, true)
        elseif target:GetTeam() == TEAM_HORDE then
            target:CastSpell(target, SPELL_TORCH_TOSSING_TRAINING_SUCCESS_HORDE, true)
        end
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_MIDSUMMER_BRAZIERS_HIT, 4, midsummerBraziersHitChangeAmount)

local function midsummerRibbonPoleVisualTick(event, aura, target)
    if not target:HasAura(SPELL_TEST_RIBBON_POLE_1)
        and not target:HasAura(SPELL_TEST_RIBBON_POLE_2)
        and not target:HasAura(SPELL_TEST_RIBBON_POLE_3) then
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_MIDSUMMER_RIBBON_POLE_PERIODIC_VISUAL, 1, midsummerRibbonPoleVisualTick)
