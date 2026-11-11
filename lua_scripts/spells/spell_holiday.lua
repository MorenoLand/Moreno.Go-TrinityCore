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
-- OnPeriodic leg closed this tick — no RemoveAuraFromStack bridge,
-- so the stack drop rides aura:SetStackAmount and the CHANGE_AMOUNT
-- re-run of OnChange it triggers in C++ is replayed explicitly in
-- giddyupPeriodic below).
local SPELL_BREWFEST_GIDDYUP = 42924 -- spell_holiday.cpp:1087
local SPELL_RENTAL_RACING_RAM = 43883 -- spell_holiday.cpp:1069
local SPELL_SWIFT_WORK_RAM = 43880 -- spell_holiday.cpp:1070
local SPELL_EXHAUSTED_RAM = 43332 -- spell_holiday.cpp:1077
local SPELL_RAM_TROT = 42992 -- spell_holiday.cpp:1073
local SPELL_RAM_CANTER = 42993 -- spell_holiday.cpp:1074
local SPELL_RAM_GALLOP = 42994 -- spell_holiday.cpp:1075
local AURA_REMOVE_BY_DEFAULT = 1 -- SpellAuraDefines.h:64

local function giddyupSpeedLeg(target, stacks)
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
end

local function giddyupChange(event, aura, target)
    if not target:HasAura(SPELL_RENTAL_RACING_RAM) and not target:HasAura(SPELL_SWIFT_WORK_RAM) then
        target:RemoveAura(SPELL_BREWFEST_GIDDYUP)
        return
    end
    if target:HasAura(SPELL_EXHAUSTED_RAM) then
        return
    end
    giddyupSpeedLeg(target, aura:GetStackAmount())
    if aura:GetRemoveMode() == AURA_REMOVE_BY_DEFAULT then
        target:RemoveAura(SPELL_RAM_TROT)
        target:CastSpell(target, SPELL_RAM_LEVEL_NEUTRAL, true)
    end
end

-- C++ spell_brewfest_giddyup::OnPeriodic (spell_holiday.cpp:1140-1143):
-- RemoveAuraFromStack(GetId()) drops one stack per tick. The stack
-- change re-fires OnChange in C++ (CHANGE_AMOUNT), so the switch
-- re-runs on the new count — the ramp-down through canter (6) and
-- trot (1). SetStackAmount fires no change-amount event in Go, so
-- the decrement and the switch re-run are both done here; the
-- remove-mode leg is skipped because the mode is NONE mid-tick, as
-- in C++. At 0 stacks the aura drops instead of going negative.
local function giddyupPeriodic(event, aura, target)
    local stacks = aura:GetStackAmount() - 1
    if stacks <= 0 then
        target:RemoveAura(SPELL_BREWFEST_GIDDYUP)
        return
    end
    aura:SetStackAmount(stacks)
    if not target:HasAura(SPELL_RENTAL_RACING_RAM) and not target:HasAura(SPELL_SWIFT_WORK_RAM) then
        target:RemoveAura(SPELL_BREWFEST_GIDDYUP)
        return
    end
    if target:HasAura(SPELL_EXHAUSTED_RAM) then
        return
    end
    giddyupSpeedLeg(target, stacks)
end

RegisterAuraEvent(SPELL_BREWFEST_GIDDYUP, 1, giddyupPeriodic)
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
--   classes are SpellScript-only, plus pilgrims bounty a_serving_of
--   (vehicle/seat chain), brewfest barker bunny (no player Talk
--   bridge), and the DB-bound IDs (pilgrims buff-food x5, midsummer
--   ribbon pole channel) whose aura IDs live only in world DB
--   spell_script_names. Romantic_picnic (PORTED below, tick 575) and
--   hallow_end_trick (PORTED below, tick 576, ev5) were in this list
--   until this tick — the GetPlayersInRange bridge postdates tick 295. Trick_or_treat (PORTED below, tick 577, ev5)
--   and hallow_end_wand (PORTED below, tick 579, ev3) left it this tick; tricky_treat stays out (tick 578: no binding
--   ID in-tree, no GetAuraCount bridge — see the stay-out note at the tail). The tick-295 "no RegisterSpellScript bridge"
--   line was imprecise: OnEffectHit rides RegisterSpellEvent ev4,
--   which is how 24930 Hallow's End Candy is ported above, and 43876
--   Dismount Ram this tick (tail). 43714 relay-race intro force is
--   covered by the Go default FORCE_CAST leg on both caster paths
--   (spells.go:52503, boss_ai.go:6953); its script delta is only the
--   reagent/power cost flag, unbridgeable from Lua — an ev4 port
--   would double-cast.

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

-- PORTED (this tick, RegisterSpellEvent OnEffectHit = 4):
--   spell_brewfest_dismount_ram::HandleScript
--   (spell_holiday.cpp:1387-1395): EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT.
--   On hit the caster drops the Rental Racing Ram aura (43883).
--   The Go default script-effect leg is a per-spellID switch with no
--   43876 case, so nothing else fires; ev4 runs ahead of the effects
--   loop (spells.go:14348), the same site the 24930 port rides. The
--   C++ arm uses no hit unit, so the no-target ev4 fire is exact.
local SPELL_BREWFEST_DISMOUNT_RAM = 43876 -- spell_holiday.cpp:1377

local function dismountRam(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:RemoveAura(SPELL_RENTAL_RACING_RAM)
end

RegisterSpellEvent(SPELL_BREWFEST_DISMOUNT_RAM, 4, dismountRam)

-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnPeriodic = 1):
--   spell_love_is_in_the_air_romantic_picnic::OnApply/OnPeriodic
--   (spell_holiday.cpp:54-101): EFFECT_0 PERIODIC_DUMMY. On apply the
--   target sits (UNIT_STAND_STATE_SIT = 1) and self-casts the meal
--   periodic 45103, non-triggered. Every 5s tick: if the target is no
--   longer sitting it drops the achievement aura 45123 and the picnic
--   aura removes itself; otherwise it self-casts the basket check
--   45119 and one visual at random (meal eat 45120 / drink 45121),
--   both non-triggered, then scans players within
--   INTERACTION_DISTANCE*2 = 10 yards for another player holding
--   the same aura — the caster casts the achievement aura 45123 on
--   each such player and on the target, triggered; if none is found
--   the target drops 45123. The tick-295 "no nearby-player
--   enumeration" note is stale: GetPlayersInRange is on the player
--   surface (lua_worldobject_range.go:608, alive-only default matches
--   the C++ AnyPlayerInObjectRangeCheck reqAlive default). The 45123
--   casts land non-triggered on the player path (file KNOWN LIMIT);
--   the C++ no-break loop re-casts 45123 on the target per matching
--   player, replayed here.
local SPELL_ROMANTIC_PICNIC = 45102 -- spell_holiday.cpp:34
local SPELL_ROMANTIC_PICNIC_BASKET_CHECK = 45119 -- spell_holiday.cpp:37
local SPELL_ROMANTIC_PICNIC_MEAL_PERIODIC = 45103 -- spell_holiday.cpp:38
local SPELL_ROMANTIC_PICNIC_MEAL_EAT_VISUAL = 45120 -- spell_holiday.cpp:39
local SPELL_ROMANTIC_PICNIC_DRINK_VISUAL = 45121 -- spell_holiday.cpp:41
local SPELL_ROMANTIC_PICNIC_ACHIEV = 45123 -- spell_holiday.cpp:42
local UNIT_STAND_STATE_SIT = 1 -- UnitDefines.h:34

local ROMANTIC_PICNIC_VISUALS = {
    SPELL_ROMANTIC_PICNIC_MEAL_EAT_VISUAL,
    SPELL_ROMANTIC_PICNIC_DRINK_VISUAL,
}

local function romanticPicnicApply(event, aura, target)
    target:SetStandState(UNIT_STAND_STATE_SIT)
    target:CastSpell(target, SPELL_ROMANTIC_PICNIC_MEAL_PERIODIC, false)
end

RegisterAuraEvent(SPELL_ROMANTIC_PICNIC, 2, romanticPicnicApply)

local function romanticPicnicPeriodic(event, aura, target)
    if target:GetStandState() ~= UNIT_STAND_STATE_SIT then
        target:RemoveAura(SPELL_ROMANTIC_PICNIC_ACHIEV)
        aura:Remove()
        return
    end
    target:CastSpell(target, SPELL_ROMANTIC_PICNIC_BASKET_CHECK, false)
    target:CastSpell(target, ROMANTIC_PICNIC_VISUALS[math.random(#ROMANTIC_PICNIC_VISUALS)], false)
    local foundSomeone = false
    local caster = aura:GetCaster()
    for _, player in ipairs(target:GetPlayersInRange(10)) do
        if player:GetGUID() ~= target:GetGUID() and player:HasAura(SPELL_ROMANTIC_PICNIC) then
            if caster ~= nil then
                caster:CastSpell(player, SPELL_ROMANTIC_PICNIC_ACHIEV, true)
                caster:CastSpell(target, SPELL_ROMANTIC_PICNIC_ACHIEV, true)
            end
            foundSomeone = true
        end
    end
    if not foundSomeone and target:HasAura(SPELL_ROMANTIC_PICNIC_ACHIEV) then
        target:RemoveAura(SPELL_ROMANTIC_PICNIC_ACHIEV)
    end
end

RegisterAuraEvent(SPELL_ROMANTIC_PICNIC, 1, romanticPicnicPeriodic)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_hallow_end_trick::HandleScript (spell_holiday.cpp:254-282,
--   registered OnEffectHitTarget EFFECT_0 SCRIPT_EFFECT at :288): on
--   a hit player the caster casts one costume aura, triggered —
--   urand(0, 5) picks leper gnome (1), pirate (2), ghost (3) or
--   ninja (4) by native gender, skeleton (5), or the trick buff
--   24753 (0). No PreventDefaultAction in C++, so the ev5
--   notification-only fire is exact. Binding ID 24750 comes from the
--   `// 24750 Trick` comment (spell_holiday.cpp:213) — the same
--   convention as the 24930 port above; no spell_script_names row
--   exists in-tree (sql/mysql/world.sql ships the table empty). ev5
--   fires on the creature path (boss_ai.go:3435) and the player
--   triggered funnel (spells.go:28261), not on a direct player cast
--   (finishSpellCast) — the open engine-work item from the census
--   head. The triggered costume cast lands non-triggered on the
--   player path (file KNOWN LIMIT).
local SPELL_HALLOW_END_TRICK = 24750 -- spell_holiday.cpp:213
local SPELL_PIRATE_COSTUME_MALE = 24708 -- spell_holiday.cpp:216
local SPELL_PIRATE_COSTUME_FEMALE = 24709 -- spell_holiday.cpp:217
local SPELL_NINJA_COSTUME_MALE = 24710 -- spell_holiday.cpp:218
local SPELL_NINJA_COSTUME_FEMALE = 24711 -- spell_holiday.cpp:219
local SPELL_LEPER_GNOME_COSTUME_MALE = 24712 -- spell_holiday.cpp:220
local SPELL_LEPER_GNOME_COSTUME_FEMALE = 24713 -- spell_holiday.cpp:221
local SPELL_SKELETON_COSTUME = 24723 -- spell_holiday.cpp:222
local SPELL_GHOST_COSTUME_MALE = 24735 -- spell_holiday.cpp:223
local SPELL_GHOST_COSTUME_FEMALE = 24736 -- spell_holiday.cpp:224
local SPELL_HALLOW_END_TRICK_BUFF = 24753 -- spell_holiday.cpp:225

local function hallowEndTrickHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    if target:GetObjectType() ~= "Player" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local spellId = SPELL_HALLOW_END_TRICK_BUFF
    local female = target:GetNativeGender() == GENDER_FEMALE
    local roll = math.random(0, 5)
    if roll == 1 then
        if female then
            spellId = SPELL_LEPER_GNOME_COSTUME_FEMALE
        else
            spellId = SPELL_LEPER_GNOME_COSTUME_MALE
        end
    elseif roll == 2 then
        if female then
            spellId = SPELL_PIRATE_COSTUME_FEMALE
        else
            spellId = SPELL_PIRATE_COSTUME_MALE
        end
    elseif roll == 3 then
        if female then
            spellId = SPELL_GHOST_COSTUME_FEMALE
        else
            spellId = SPELL_GHOST_COSTUME_MALE
        end
    elseif roll == 4 then
        if female then
            spellId = SPELL_NINJA_COSTUME_FEMALE
        else
            spellId = SPELL_NINJA_COSTUME_MALE
        end
    elseif roll == 5 then
        spellId = SPELL_SKELETON_COSTUME
    end
    caster:CastSpell(target, spellId, true)
end

RegisterSpellEvent(SPELL_HALLOW_END_TRICK, 5, hallowEndTrickHit)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_hallow_end_trick_or_treat::HandleScript
--   (spell_holiday.cpp:322-331, registered OnEffectHitTarget EFFECT_0
--   SCRIPT_EFFECT at :335): on a hit player the caster casts Trick
--   24714 or Treat 24715 (roll_chance_i(50)) and then Tricked or
--   Treated 24755, both triggered. No PreventDefaultAction in C++,
--   so the ev5 notification-only fire is exact. Binding ID 24751
--   comes from the `// 24751 Trick or Treat` comment
--   (spell_holiday.cpp:298) — the same convention as the 24750 port
--   above; no spell_script_names row exists in-tree. ev5 fires on
--   the creature path (boss_ai.go:3435) and the player triggered
--   funnel (spells.go:28261), not on a direct player cast
--   (finishSpellCast) — the open engine-work item from the census
--   head. The triggered casts land non-triggered on the player path
--   (file KNOWN LIMIT).
local SPELL_HALLOW_END_TRICK_OR_TREAT = 24751 -- spell_holiday.cpp:298
local SPELL_TRICK = 24714 -- spell_holiday.cpp:301
local SPELL_TREAT = 24715 -- spell_holiday.cpp:302
local SPELL_TRICKED_OR_TREATED = 24755 -- spell_holiday.cpp:303

local function hallowEndTrickOrTreatHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    if target:GetObjectType() ~= "Player" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if math.random(1, 100) <= 50 then
        caster:CastSpell(target, SPELL_TRICK, true)
    else
        caster:CastSpell(target, SPELL_TREAT, true)
    end
    caster:CastSpell(target, SPELL_TRICKED_OR_TREATED, true)
end

RegisterSpellEvent(SPELL_HALLOW_END_TRICK_OR_TREAT, 5, hallowEndTrickOrTreatHit)

-- STAYS OUT (this tick):
--   spell_hallow_end_tricky_treat::HandleScript
--   (spell_holiday.cpp:367-368, registered OnEffectHitTarget EFFECT_0
--   SCRIPT_EFFECT at :373): if the caster has the Tricky Treat
--   trigger aura 42965 and more than 3 stacks of the speed aura
--   42919, a 33% roll self-casts Upset Tummy 42966, triggered.
--   Blockers: (1) no binding spell ID anywhere in-tree — census
--   source "no-id", no `// <id>` comment above the class, no
--   spell_script_names row in the partial sql/ tree, and zero hits
--   for 42919/42965/42966 in the Go tree; (2) no GetAuraCount bridge
--   on any Lua surface (player HasAura is live, lua_player.go:2695,
--   but no count read exists in engine/ or Lua). Go recipe: bind
--   the TDB spell_script_names ID once known and expose
--   unit:GetAuraCount via the aura store (playerHasAura pattern,
--   achievements.go:834), then register ev5 with the body above.
--   Census hook OnEffectHitTarget is correct.

-- PORTED (this tick, RegisterSpellEvent AfterHit = 3):
--   spell_hallow_end_wand::HandleScriptEffect
--   (spell_holiday.cpp:407-451, registered AfterHit at :454): on a
--   hit target the caster casts one costume aura by the wand's own
--   spell ID and the target's native gender — leper gnome 24719,
--   pirate 24717, ghost 24737, ninja 24718 (female/male pairs
--   24709-24713/24735-24736); the random wand 24720 casts one of the
--   seven wand spells at random (24717/24718/24719/24724/24733/24737/
--   24741), replayed here as a triggered cast of the picked wand so
--   its own ev3 handler fires — skeleton 24724, wisp 24733 and bat
--   24741 hit the C++ default: return, so they are not registered
--   and a random pick of one is a no-op, exactly as in C++. No
--   PreventDefaultAction in C++, so the ev3 notification-only fire
--   is exact. ev3 fires on the direct player cast path
--   (spells.go:19700), the triggered funnel (spells.go:29893) and
--   the creature path (boss_ai.go:4245) — and the direct Lua
--   CastSpell path fires it too (castSpellDirectWithOverrides ->
--   fireSpellAfterHit), so the 24720 nested cast chains correctly.
--   The per-ID closure below replays the C++ switch on
--   GetSpellInfo()->Id; there is no spell:GetId bridge. DELTA: the
--   handler gates to player targets — GetNativeGender is
--   player-only (lua_player.go:360), so a creature target keeps its
--   C++ costume unported. A nil target (Go fires AfterHit with nil
--   on misses) returns without casting; C++ dereferences
--   GetHitUnit() unchecked there.
local SPELL_HALLOWED_WAND_PIRATE = 24717 -- spell_holiday.cpp:387
local SPELL_HALLOWED_WAND_NINJA = 24718 -- spell_holiday.cpp:388
local SPELL_HALLOWED_WAND_LEPER_GNOME = 24719 -- spell_holiday.cpp:389
local SPELL_HALLOWED_WAND_RANDOM = 24720 -- spell_holiday.cpp:390
local SPELL_HALLOWED_WAND_SKELETON = 24724 -- spell_holiday.cpp:391
local SPELL_HALLOWED_WAND_WISP = 24733 -- spell_holiday.cpp:392
local SPELL_HALLOWED_WAND_GHOST = 24737 -- spell_holiday.cpp:393
local SPELL_HALLOWED_WAND_BAT = 24741 -- spell_holiday.cpp:394

local HALLOWED_WAND_SPELLS = {
    SPELL_HALLOWED_WAND_PIRATE,
    SPELL_HALLOWED_WAND_NINJA,
    SPELL_HALLOWED_WAND_LEPER_GNOME,
    SPELL_HALLOWED_WAND_SKELETON,
    SPELL_HALLOWED_WAND_WISP,
    SPELL_HALLOWED_WAND_GHOST,
    SPELL_HALLOWED_WAND_BAT,
}

local function hallowEndWandHit(wandId)
    return function(event, spell, target)
        if target == nil then
            return
        end
        if target:GetObjectType() ~= "Player" then
            return
        end
        local caster = spell:GetCaster()
        if caster == nil then
            return
        end
        local female = target:GetNativeGender() == GENDER_FEMALE
        local spellId = 0
        if wandId == SPELL_HALLOWED_WAND_LEPER_GNOME then
            if female then
                spellId = SPELL_LEPER_GNOME_COSTUME_FEMALE
            else
                spellId = SPELL_LEPER_GNOME_COSTUME_MALE
            end
        elseif wandId == SPELL_HALLOWED_WAND_PIRATE then
            if female then
                spellId = SPELL_PIRATE_COSTUME_FEMALE
            else
                spellId = SPELL_PIRATE_COSTUME_MALE
            end
        elseif wandId == SPELL_HALLOWED_WAND_GHOST then
            if female then
                spellId = SPELL_GHOST_COSTUME_FEMALE
            else
                spellId = SPELL_GHOST_COSTUME_MALE
            end
        elseif wandId == SPELL_HALLOWED_WAND_NINJA then
            if female then
                spellId = SPELL_NINJA_COSTUME_FEMALE
            else
                spellId = SPELL_NINJA_COSTUME_MALE
            end
        elseif wandId == SPELL_HALLOWED_WAND_RANDOM then
            spellId = HALLOWED_WAND_SPELLS[math.random(#HALLOWED_WAND_SPELLS)]
        else
            return
        end
        caster:CastSpell(target, spellId, true)
    end
end

for _, wandId in ipairs({
    SPELL_HALLOWED_WAND_PIRATE,
    SPELL_HALLOWED_WAND_NINJA,
    SPELL_HALLOWED_WAND_LEPER_GNOME,
    SPELL_HALLOWED_WAND_RANDOM,
    SPELL_HALLOWED_WAND_GHOST,
}) do
    RegisterSpellEvent(wandId, 3, hallowEndWandHit(wandId))
end
