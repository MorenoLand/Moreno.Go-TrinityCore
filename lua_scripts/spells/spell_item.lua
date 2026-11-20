-- Item spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_item.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2, OnPeriodic = 1,
-- OnRemove = 3):
--   spell_item_nitro_boosts_backfire::HandleApply
--   (spell_item.cpp:3107-3110): EFFECT_1 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
--   On apply, records the target's height; the C++ member lastZ
--   (INVALID_HEIGHT init) is the per-target table below, keyed by
--   the target GUID — the apply hook always precedes the periodic
--   hook, so the entry exists by the first tick.
--   spell_item_nitro_boosts_backfire::HandlePeriodicDummy
--   (spell_item.cpp:3112-3123): PreventDefaultAction, so the handler
--   returns true and the Go default trigger cast does not fire. A
--   falling target (current Z below lastZ) rolls 80% to self-cast
--   the parachute 54649, triggered (CastSpellExtraArgs(AuraEffect*)
--   sets TRIGGERED_FULL_MASK, SpellDefines.h:166), then the aura
--   removes itself; a rising target just updates lastZ. The remove
--   handler drops the table entry.

local SPELL_NITRO_BOOSTS_BACKFIRE = 54621 -- spell_item.cpp:3098 (class spell_item_nitro_boosts_backfire)
local SPELL_NITRO_BOOSTS_PARACHUTE = 54649 -- spell_item.cpp:3063

local lastZByGUID = {}

local function nitroBackfireApply(event, aura, target)
    lastZByGUID[target:GetGUIDLow()] = target:GetZ()
end

local function nitroBackfirePeriodic(event, aura, target)
    local guid = target:GetGUIDLow()
    local lastZ = lastZByGUID[guid]
    local curZ = target:GetZ()
    if lastZ == nil then
        lastZByGUID[guid] = curZ
        return true
    end
    if curZ < lastZ then
        if math.random(0, 99) < 80 then
            target:CastSpell(target, SPELL_NITRO_BOOSTS_PARACHUTE, true)
        end
        aura:Remove()
        lastZByGUID[guid] = nil
    else
        lastZByGUID[guid] = curZ
    end
    return true
end

local function nitroBackfireRemove(event, aura, target)
    lastZByGUID[target:GetGUIDLow()] = nil
end

RegisterAuraEvent(SPELL_NITRO_BOOSTS_BACKFIRE, 2, nitroBackfireApply)
RegisterAuraEvent(SPELL_NITRO_BOOSTS_BACKFIRE, 1, nitroBackfirePeriodic)
RegisterAuraEvent(SPELL_NITRO_BOOSTS_BACKFIRE, 3, nitroBackfireRemove)

-- PORTED (tick 295, RegisterAuraEvent OnPeriodic = 1):
--   spell_item_absorb_eye_of_grillok::PeriodicTick
--   (spell_item.cpp:126-140): EFFECT_0 PERIODIC_TRIGGER_SPELL.
--   PreventDefaultAction runs before the gates, so the handler
--   returns true on every path. The caster self-casts Eye of
--   Grillok 38495, triggered, and the creature target despawns.
--   spell_item_dementia::HandlePeriodicDummy (spell_item.cpp:395-404):
--   EFFECT_0 PERIODIC_DUMMY. PreventDefaultAction (return true);
--   the target self-casts one of 41406 / 41409, triggered, chosen
--   uniformly (C++ RAND).
--
-- PORTED (tick 295, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_item_power_circle::OnApply/OnRemove (spell_item.cpp:1633-1651):
--   EFFECT_0 DUMMY, REAL. On apply the target self-casts Limitless
--   Power 45044, triggered, and the buff's duration is set to the
--   circle aura's remaining duration. On remove the buff drops.
--   The DoCheckAreaTarget caster-only filter is unbridged and
--   unmodeled (harmless in practice — the aura is self-only).
--   spell_item_shadowmourne::OnRemove (spell_item.cpp:2055-2058):
--   EFFECT_0 DUMMY, REAL. On remove the Soul Fragment aura 71905
--   drops. The proc leg is ported below (ev6 strip).
--   spell_item_anger_capacitor::OnRemove (spell_item.cpp:260-263):
--   EFFECT_0 DUMMY, REAL. On remove the Mote of Anger aura 71432
--   drops. Registered for both ranks: 71406 (8-stack) and 71545
--   (heroic, 7-stack). The proc leg is ported below (ev6 strip).
--   spell_item_trinket_stack::OnRemove (spell_item.cpp:3750-3752):
--   EFFECT_0 PROC_TRIGGER_SPELL, REAL. On remove the stack aura
--   drops (Lightning Capacitor 37657 -> 37658, Thunder Capacitor
--   54841 -> 54842, ToC25 normal 67712 -> 67713, ToC25 heroic
--   67758 -> 67759). The proc leg is ported below (this tick, ev6).
--
-- PORTED (tick 295, RegisterAuraEvent OnApply = 2 / OnChangeAmount = 4 /
--   OnRemove = 3):
--   spell_item_shadowmourne_soul_fragment::OnStackChange/OnRemove
--   (spell_item.cpp:2073-2105): EFFECT_0 MOD_STAT. C++ binds the
--   stack switch to AfterEffectApply REAL|REAPPLY, which fires on
--   fresh apply AND on stack-raising re-casts; Go splits that across
--   mode 2 (fresh apply, stack 1) and mode 4 (merge re-cast raising
--   the stack), so the same handler rides both. Stacks: 1 -> self-cast
--   Visual Low 72521; 6 -> drop 72521, self-cast Visual High 72523;
--   10 -> drop 72523, self-cast Chaos Bane Buff 73422; all triggered.
--   On remove the visuals drop. The proc leg that feeds stacks lives
--   in spell_item_shadowmourne and stays out (no proc hook).
--
-- CLOSED (tick 295 census): the remaining item classes are
--   SpellScript-only (no RegisterSpellScript bridge), proc arms,
--   calc arms, or need unbridged data (extract_gas AutoStoreLoot /
--   SkinLootId); magic_eater_food's aura ID lives only in the world
--   DB spell_script_names, unresolvable in workspace.

local SPELL_EYE_OF_GRILLOK = 38495 -- spell_item.cpp:113
local SPELL_ABSORB_EYE_OF_GRILLOK = 38554 -- spell_item.cpp:116 (class spell_item_absorb_eye_of_grillok)
local SPELL_DEMENTIA = 41404 -- spell_item.cpp:382 (class spell_item_dementia)
local SPELL_DEMENTIA_POS = 41406 -- spell_item.cpp:308
local SPELL_DEMENTIA_NEG = 41409 -- spell_item.cpp:309
local SPELL_POWER_CIRCLE = 45043 -- spell_item.cpp:1618 (class spell_item_power_circle)
local SPELL_LIMITLESS_POWER = 45044 -- spell_item.cpp:1615
local SPELL_SHADOWMOURNE_SOUL_FRAGMENT = 71905 -- spell_item.cpp:2059 (class spell_item_shadowmourne_soul_fragment)
local SPELL_SHADOWMOURNE_VISUAL_LOW = 72521 -- spell_item.cpp:2002
local SPELL_SHADOWMOURNE_VISUAL_HIGH = 72523 -- spell_item.cpp:2003
local SPELL_SHADOWMOURNE_CHAOS_BANE_BUFF = 73422 -- spell_item.cpp:2004
local SPELL_ANGER_CAPACITOR = 71406 -- spell_item.cpp:215 (class spell_item_anger_capacitor, 8-stack)
local SPELL_ANGER_CAPACITOR_HEROIC = 71545 -- spell_item.cpp:218 (7-stack)
local SPELL_MOTE_OF_ANGER = 71432 -- spell_item.cpp:209
local SPELL_LIGHTNING_CAPACITOR_AURA = 37657 -- spell_item.cpp:3687
local SPELL_LIGHTNING_CAPACITOR_STACK = 37658 -- spell_item.cpp:3688
local SPELL_THUNDER_CAPACITOR_AURA = 54841 -- spell_item.cpp:3690
local SPELL_THUNDER_CAPACITOR_STACK = 54842 -- spell_item.cpp:3691
local SPELL_TOC25_CASTER_TRINKET_NORMAL_AURA = 67712 -- spell_item.cpp:3693
local SPELL_TOC25_CASTER_TRINKET_NORMAL_STACK = 67713 -- spell_item.cpp:3694
local SPELL_TOC25_CASTER_TRINKET_HEROIC_AURA = 67758 -- spell_item.cpp:3696
local SPELL_TOC25_CASTER_TRINKET_HEROIC_STACK = 67759 -- spell_item.cpp:3697

local TRINKET_STACK_BY_AURA = {
    [SPELL_LIGHTNING_CAPACITOR_AURA] = SPELL_LIGHTNING_CAPACITOR_STACK,
    [SPELL_THUNDER_CAPACITOR_AURA] = SPELL_THUNDER_CAPACITOR_STACK,
    [SPELL_TOC25_CASTER_TRINKET_NORMAL_AURA] = SPELL_TOC25_CASTER_TRINKET_NORMAL_STACK,
    [SPELL_TOC25_CASTER_TRINKET_HEROIC_AURA] = SPELL_TOC25_CASTER_TRINKET_HEROIC_STACK,
}

local function absorbEyeOfGrillokTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target:GetObjectType() ~= "Creature" then
        return true
    end
    caster:CastSpell(caster, SPELL_EYE_OF_GRILLOK, true)
    target:DespawnOrUnsummon()
    return true
end

RegisterAuraEvent(SPELL_ABSORB_EYE_OF_GRILLOK, 1, absorbEyeOfGrillokTick)

local function dementiaTick(event, aura, target)
    if math.random(0, 1) == 0 then
        target:CastSpell(target, SPELL_DEMENTIA_POS, true)
    else
        target:CastSpell(target, SPELL_DEMENTIA_NEG, true)
    end
    return true
end

RegisterAuraEvent(SPELL_DEMENTIA, 1, dementiaTick)

local function powerCircleApply(event, aura, target)
    target:CastSpell(target, SPELL_LIMITLESS_POWER, true)
    local buff = target:GetAura(SPELL_LIMITLESS_POWER)
    if buff ~= nil then
        buff:SetDuration(aura:GetDuration())
    end
end

RegisterAuraEvent(SPELL_POWER_CIRCLE, 2, powerCircleApply)

local function powerCircleRemove(event, aura, target)
    target:RemoveAura(SPELL_LIMITLESS_POWER)
end

RegisterAuraEvent(SPELL_POWER_CIRCLE, 3, powerCircleRemove)

local function shadowmourneSoulFragmentStack(event, aura, target)
    local stacks = aura:GetStackAmount()
    if stacks == 1 then
        target:CastSpell(target, SPELL_SHADOWMOURNE_VISUAL_LOW, true)
    elseif stacks == 6 then
        target:RemoveAura(SPELL_SHADOWMOURNE_VISUAL_LOW)
        target:CastSpell(target, SPELL_SHADOWMOURNE_VISUAL_HIGH, true)
    elseif stacks == 10 then
        target:RemoveAura(SPELL_SHADOWMOURNE_VISUAL_HIGH)
        target:CastSpell(target, SPELL_SHADOWMOURNE_CHAOS_BANE_BUFF, true)
    end
end

RegisterAuraEvent(SPELL_SHADOWMOURNE_SOUL_FRAGMENT, 2, shadowmourneSoulFragmentStack)
RegisterAuraEvent(SPELL_SHADOWMOURNE_SOUL_FRAGMENT, 4, shadowmourneSoulFragmentStack)

local function shadowmourneSoulFragmentRemove(event, aura, target)
    target:RemoveAura(SPELL_SHADOWMOURNE_VISUAL_LOW)
    target:RemoveAura(SPELL_SHADOWMOURNE_VISUAL_HIGH)
end

RegisterAuraEvent(SPELL_SHADOWMOURNE_SOUL_FRAGMENT, 3, shadowmourneSoulFragmentRemove)

local function angerCapacitorRemove(event, aura, target)
    target:RemoveAura(SPELL_MOTE_OF_ANGER)
end

RegisterAuraEvent(SPELL_ANGER_CAPACITOR, 3, angerCapacitorRemove)
RegisterAuraEvent(SPELL_ANGER_CAPACITOR_HEROIC, 3, angerCapacitorRemove)

local function trinketStackRemove(event, aura, target)
    local stack = TRINKET_STACK_BY_AURA[aura:GetAuraId()]
    if stack ~= nil then
        target:RemoveAura(stack)
    end
end

RegisterAuraEvent(SPELL_LIGHTNING_CAPACITOR_AURA, 3, trinketStackRemove)
RegisterAuraEvent(SPELL_THUNDER_CAPACITOR_AURA, 3, trinketStackRemove)
RegisterAuraEvent(SPELL_TOC25_CASTER_TRINKET_NORMAL_AURA, 3, trinketStackRemove)
RegisterAuraEvent(SPELL_TOC25_CASTER_TRINKET_HEROIC_AURA, 3, trinketStackRemove)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_item_anger_capacitor::HandleProc (spell_item.cpp:243-258):
--   EFFECT_0 DUMMY. PreventDefaultAction (return true on every path);
--   the actor self-casts Mote of Anger 71432, triggered, and at
--   Stacks motes the motes drop and the actor casts Manifest Anger
--   on the proc target: main-hand 71433, or off-hand 71434 when the
--   player has an off-hand weapon and the 50% roll lands
--   (spell_item.cpp:253-256). The off-hand leg rides
--   GetEquippedItemBySlot(16) (EQUIPMENT_SLOT_OFFHAND); it reads
--   whatever is equipped there, so a non-weapon off-hand item is an
--   approximation of C++ GetWeaponForAttack(OFF_ATTACK, true), and
--   it needs the characters DB (nil without it). Registered for both
--   ranks: heroic 71545 (7-stack), normal 71406 (8-stack). The
--   OnRemove leg that drops the motes lives above (tick 295).
--   DORMANT until Spell.dbc ships the 71406/71545 proc entries.
local SPELL_MANIFEST_ANGER_MAIN_HAND = 71433 -- spell_item.cpp:211
local SPELL_MANIFEST_ANGER_OFF_HAND = 71434 -- spell_item.cpp:212

local function makeAngerCapacitorProc(stacks)
    return function(event, aura, target, actor, procTarget, damage)
        if actor == nil then
            return true
        end
        actor:CastSpell(actor, SPELL_MOTE_OF_ANGER, true)
        local motes = actor:GetAura(SPELL_MOTE_OF_ANGER)
        if motes == nil or motes:GetStackAmount() < stacks then
            return true
        end
        actor:RemoveAura(SPELL_MOTE_OF_ANGER)
        local spellId = SPELL_MANIFEST_ANGER_MAIN_HAND
        if procTarget ~= nil and actor:GetObjectType() == "Player"
            and actor:GetEquippedItemBySlot(16) ~= nil
            and math.random(100) <= 50 then
            spellId = SPELL_MANIFEST_ANGER_OFF_HAND
        end
        if procTarget ~= nil then
            actor:CastSpell(procTarget, spellId, true)
        end
        return true
    end
end

RegisterAuraEvent(SPELL_ANGER_CAPACITOR_HEROIC, 6, makeAngerCapacitorProc(7))
RegisterAuraEvent(SPELL_ANGER_CAPACITOR, 6, makeAngerCapacitorProc(8))

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_item_heartpierce::HandleProc (spell_item.cpp:1164-1195):
--   EFFECT_0 DUMMY. PreventDefaultAction; the actor self-casts the
--   Invigoration for its power type: heroic (mana 71888, energy
--   71887, rage 71886, runic power 71885), normal (mana 71881,
--   energy 71882, rage 71883, runic power 71884); any other power
--   type casts nothing. C++ casts on a null target with aurEff; the
--   Invigoration spells are self-targeted, so the Lua twin casts
--   on the actor, triggered. The normal-rank aura is 71880.
--   DORMANT until Spell.dbc ships the 71880/71892 proc entries.
local SPELL_HEARTPIERCE = 71880 -- normal-rank aura (twin of 71892)
local SPELL_HEARTPIERCE_HEROIC = 71892 -- spell_item.cpp:1136 (class spell_item_heartpierce, heroic template)
local SPELL_INVIGORATION_MANA = 71881 -- spell_item.cpp:1116
local SPELL_INVIGORATION_ENERGY = 71882 -- spell_item.cpp:1117
local SPELL_INVIGORATION_RAGE = 71883 -- spell_item.cpp:1118
local SPELL_INVIGORATION_RP = 71884 -- spell_item.cpp:1119
local SPELL_INVIGORATION_MANA_HERO = 71888 -- spell_item.cpp:1124
local SPELL_INVIGORATION_ENERGY_HERO = 71887 -- spell_item.cpp:1123
local SPELL_INVIGORATION_RAGE_HERO = 71886 -- spell_item.cpp:1122
local SPELL_INVIGORATION_RP_HERO = 71885 -- spell_item.cpp:1121

local HEARTPIERCE_BY_POWER = {
    [0] = SPELL_INVIGORATION_MANA,
    [1] = SPELL_INVIGORATION_RAGE,
    [3] = SPELL_INVIGORATION_ENERGY,
    [6] = SPELL_INVIGORATION_RP,
}

local HEARTPIERCE_HEROIC_BY_POWER = {
    [0] = SPELL_INVIGORATION_MANA_HERO,
    [1] = SPELL_INVIGORATION_RAGE_HERO,
    [3] = SPELL_INVIGORATION_ENERGY_HERO,
    [6] = SPELL_INVIGORATION_RP_HERO,
}

local function makeHeartpierceProc(byPower)
    return function(event, aura, target, actor, procTarget, damage)
        if actor == nil then
            return true
        end
        local spellId = byPower[actor:GetPowerType()]
        if spellId == nil then
            return true
        end
        actor:CastSpell(actor, spellId, true)
        return true
    end
end

RegisterAuraEvent(SPELL_HEARTPIERCE_HEROIC, 6, makeHeartpierceProc(HEARTPIERCE_HEROIC_BY_POWER))
RegisterAuraEvent(SPELL_HEARTPIERCE, 6, makeHeartpierceProc(HEARTPIERCE_BY_POWER))

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_item_sunwell_neck::HandleProc (spell_item.cpp:3568-3582):
--   EFFECT_0 DUMMY. PreventDefaultAction (return true on every path);
--   the actor must be a player (CheckProc, :3563-3567); an Aldor-
--   exalted player casts the Aldor spell on the procTarget and a
--   Scryers-exalted player casts the Scryers spell (independent ifs,
--   :3577-3581), both triggered. Binding IDs are the `// <id> -
--   Sunwell Exalted ... Neck` comments (:3536/:3539/:3542/:3544),
--   the same comment-ID convention as the other ports in this file.
--   DORMANT until Spell.dbc ships the 45481-45484 proc entries.
local SPELL_SUNWELL_NECK_CASTER = 45481 -- spell_item.cpp:3536
local SPELL_SUNWELL_NECK_MELEE = 45482 -- spell_item.cpp:3539
local SPELL_SUNWELL_NECK_TANK = 45483 -- spell_item.cpp:3542
local SPELL_SUNWELL_NECK_HEALER = 45484 -- spell_item.cpp:3544
local SPELL_LIGHTS_WRATH = 45479 -- spell_item.cpp:3518
local SPELL_ARCANE_BOLT = 45429 -- spell_item.cpp:3519
local SPELL_LIGHTS_STRENGTH = 45480 -- spell_item.cpp:3521
local SPELL_ARCANE_STRIKE = 45428 -- spell_item.cpp:3522
local SPELL_LIGHTS_WARD = 45432 -- spell_item.cpp:3524
local SPELL_ARCANE_INSIGHT = 45431 -- spell_item.cpp:3525
local SPELL_LIGHTS_SALVATION = 45478 -- spell_item.cpp:3527
local SPELL_ARCANE_SURGE = 45430 -- spell_item.cpp:3528
local FACTION_ALDOR = 932 -- spell_item.cpp:3532
local FACTION_SCRYERS = 934 -- spell_item.cpp:3533
local REP_EXALTED = 7

local function makeSunwellNeckProc(aldorSpell, scryersSpell)
    return function(event, aura, target, actor, procTarget, damage)
        if actor == nil or procTarget == nil then
            return true
        end
        if actor:GetObjectType() ~= "Player" then
            return true
        end
        if actor:GetReputationRank(FACTION_ALDOR) == REP_EXALTED then
            actor:CastSpell(procTarget, aldorSpell, true)
        end
        if actor:GetReputationRank(FACTION_SCRYERS) == REP_EXALTED then
            actor:CastSpell(procTarget, scryersSpell, true)
        end
        return true
    end
end

RegisterAuraEvent(SPELL_SUNWELL_NECK_CASTER, 6, makeSunwellNeckProc(SPELL_LIGHTS_WRATH, SPELL_ARCANE_BOLT))
RegisterAuraEvent(SPELL_SUNWELL_NECK_MELEE, 6, makeSunwellNeckProc(SPELL_LIGHTS_STRENGTH, SPELL_ARCANE_STRIKE))
RegisterAuraEvent(SPELL_SUNWELL_NECK_TANK, 6, makeSunwellNeckProc(SPELL_LIGHTS_WARD, SPELL_ARCANE_INSIGHT))
RegisterAuraEvent(SPELL_SUNWELL_NECK_HEALER, 6, makeSunwellNeckProc(SPELL_LIGHTS_SALVATION, SPELL_ARCANE_SURGE))

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_item_trinket_stack::HandleProc (spell_item.cpp:3726-3744):
--   EFFECT_0 PROC_TRIGGER_SPELL. PreventDefaultAction (return true
--   on every path); the actor self-casts the stack spell, triggered,
--   and while the stack aura is below the EFFECT_0 threshold nothing
--   further happens; at threshold the stack aura drops and the actor
--   casts the trigger spell on the procTarget (C++ GetActionTarget,
--   the ev6 procTarget on the done event), triggered. Same factory
--   shape as the anger-capacitor proc above. The OnRemove leg that
--   drops the stack aura lives above (tick 295). DORMANT until
--   Spell.dbc ships the 37657/54841/67712/67758 proc entries.
local SPELL_LIGHTNING_CAPACITOR_TRIGGER = 37661 -- spell_item.cpp:3689
local SPELL_THUNDER_CAPACITOR_TRIGGER = 54843 -- spell_item.cpp:3692
local SPELL_TOC25_CASTER_TRINKET_NORMAL_TRIGGER = 67714 -- spell_item.cpp:3695
local SPELL_TOC25_CASTER_TRINKET_HEROIC_TRIGGER = 67760 -- spell_item.cpp:3698

local function makeTrinketStackProc(stackSpell, triggerSpell)
    return function(event, aura, target, actor, procTarget, damage)
        if actor == nil then
            return true
        end
        actor:CastSpell(actor, stackSpell, true)
        local stack = actor:GetAura(stackSpell)
        if stack == nil or stack:GetStackAmount() < aura:GetAmount() then
            return true
        end
        actor:RemoveAura(stackSpell)
        if procTarget ~= nil then
            actor:CastSpell(procTarget, triggerSpell, true)
        end
        return true
    end
end

RegisterAuraEvent(SPELL_LIGHTNING_CAPACITOR_AURA, 6, makeTrinketStackProc(SPELL_LIGHTNING_CAPACITOR_STACK, SPELL_LIGHTNING_CAPACITOR_TRIGGER))
RegisterAuraEvent(SPELL_THUNDER_CAPACITOR_AURA, 6, makeTrinketStackProc(SPELL_THUNDER_CAPACITOR_STACK, SPELL_THUNDER_CAPACITOR_TRIGGER))
RegisterAuraEvent(SPELL_TOC25_CASTER_TRINKET_NORMAL_AURA, 6, makeTrinketStackProc(SPELL_TOC25_CASTER_TRINKET_NORMAL_STACK, SPELL_TOC25_CASTER_TRINKET_NORMAL_TRIGGER))
RegisterAuraEvent(SPELL_TOC25_CASTER_TRINKET_HEROIC_AURA, 6, makeTrinketStackProc(SPELL_TOC25_CASTER_TRINKET_HEROIC_STACK, SPELL_TOC25_CASTER_TRINKET_HEROIC_TRIGGER))

-- PORTED (this tick, RegisterSpellEvent OnEffectHit = 4):
--   spell_item_trigger_spell::HandleDummy (spell_item.cpp:66-72):
--   EFFECT_0 SPELL_EFFECT_DUMMY. If the spell was cast from an item,
--   the caster casts the mapped triggered spell on itself, triggered,
--   with the item as the cast item (Unit::CastSpell(caster, id,
--   item)). One handler for all four AddSC bindings
--   (spell_item.cpp:4291-4297); the GetCastItem gate is the load-
--   bearing leg — without an item the dummy does nothing.
--   DORMANT until Spell.dbc ships the 23074/23133/23076/23075 rows.
local ITEM_TRIGGER_SPELLS = {
    [23074] = 19804, -- Arcanite Dragonling (spell_item.cpp:4291)
    [23133] = 13166, -- Gnomish Battle Chicken (spell_item.cpp:4293)
    [23076] = 4073, -- Mechanical Dragonling (spell_item.cpp:4295)
    [23075] = 12749, -- Mithril Mechanical Dragonling (spell_item.cpp:4297)
}

local function itemTriggerSpellHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local item = spell:GetCastItem()
    if item == nil then
        return
    end
    local triggered = ITEM_TRIGGER_SPELLS[spell:GetEntry()]
    if triggered == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastCustomSpell(caster, triggered, true, nil, nil, nil, item)
end

RegisterSpellEvent(23074, 4, itemTriggerSpellHit)
RegisterSpellEvent(23133, 4, itemTriggerSpellHit)
RegisterSpellEvent(23076, 4, itemTriggerSpellHit)
RegisterSpellEvent(23075, 4, itemTriggerSpellHit)

-- CENSUS CORRECTIONS (rows 604-607, verified in-house this tick):
--   604 spell_item_trigger_spell PORTED this tick (ev4, dormant) —
--   spell:GetCastItem bridged on the spell object this tick
--   (lua_spell.go, session builder; creature builder reads nil).
--   The census hits are the unrelated generic
--   spell_trigger_spell_from_caster in boss files.
--   605 spell_item_defibrillate STAYS OUT — SpellScript
--   OnEffectHitTarget RESURRECT only (spell_item.cpp:712-756;
--   census HandleDummy/OnObjectAreaTargetSelect are wrong), goblin
--   jumper cables 8342 (67% fail -> 8338) / XL 22999 (50% -> 23055)
--   / gnomish army knife 54732 (33%, no fail spell). Doubly blocked:
--   ev5 has no player-path fire site (creature path + triggered
--   funnel only, runtime.go contract) and the PreventHitDefaultEffect
--   veto has no player-path model — a fail roll would resurrect AND
--   cast the fail spell. GetCastItem is bridged now (row 604 this
--   tick), so only the ev5 fire site + prevent model block 605.
--   CORRECTION (tick 2026-11-12): the ev5 ground above is STALE —
--   the runtime.go contract now fires OnEffectHitTarget on all three
--   paths including the player path (spells.go:14735); only the
--   PreventHitDefaultEffect veto (no player-path prevent record)
--   still blocks 605.
--   606 spell_item_sunwell_neck PORTED this tick (ev6, dormant) —
--   CORRECTION to the old note below: the four aura binding IDs are
--   the `// <id> - Sunwell Exalted ... Neck` comments
--   (spell_item.cpp:3536/3539/3542/3544), not DB-only.
--   607 spell_item_trinket_stack proc leg PORTED this tick (ev6,
--   dormant) — CORRECTION to the old note below: ev6 exists and the
--   anger-capacitor port in this file already rides it. The OnRemove
--   leg was already ported (tick 295, verified).

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_purify_helboar_meat::HandleDummy
--   (spell_item.cpp:2662-2690): EFFECT_0 DUMMY. Load gate is caster
--   TYPEID_PLAYER (checked in the handler since Lua has no Load
--   hook); the caster self-casts 29277 (Purified) or 29278 (Toxic)
--   on a 50/50 roll, triggered. Binding 29200 (AC
--   'spell_item_purify_helboar_meat'; TDB 335.26091 renames it
--   'spell_hellfire_peninsula_purify_helboar_meat'). The handler
--   never reads the hit target, so the arm rides the caster leg of
--   the ev5 partition.
local SPELL_PURIFY_HELBOAR_MEAT = 29200 -- spell_item.cpp:2662
local SPELL_SUMMON_PURIFIED_HELBOAR_MEAT = 29277 -- spell_item.cpp:2658
local SPELL_SUMMON_TOXIC_HELBOAR_MEAT = 29278 -- spell_item.cpp:2659
local TYPEID_PLAYER = 4

local function purifyHelboarMeatHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    if math.random(0, 99) < 50 then
        caster:CastSpell(caster, SPELL_SUMMON_PURIFIED_HELBOAR_MEAT, true)
    else
        caster:CastSpell(caster, SPELL_SUMMON_TOXIC_HELBOAR_MEAT, true)
    end
end

RegisterSpellEvent(SPELL_PURIFY_HELBOAR_MEAT, 5, purifyHelboarMeatHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_create_heart_candy::HandleScript
--   (spell_item.cpp:2400-2419): EFFECT_0 SCRIPT_EFFECT.
--   PreventHitDefaultEffect; the hit player gains one of the 8
--   heart candies (urand(0, 7), 1-based here). Binding 26678 (AC
--   'spell_item_create_heart_candy'; TDB 26091 renames it
--   'spell_love_is_in_the_air_create_heart_candy').
local SPELL_CREATE_HEART_CANDY = 26678 -- spell_item.cpp:2400
local ITEM_HEART_CANDY = { 21818, 21817, 21821, 21819, 21816, 21823, 21822, 21820 } -- spell_item.cpp:2390-2397

local function createHeartCandyHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    if hitTarget == nil or hitTarget:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    hitTarget:AddItem(ITEM_HEART_CANDY[math.random(1, 8)], 1)
end

RegisterSpellEvent(SPELL_CREATE_HEART_CANDY, 5, createHeartCandyHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_reindeer_transformation::HandleDummy
--   (spell_item.cpp:2734-2783): EFFECT_0 DUMMY. Only while the
--   caster has a SPELL_AURA_MOUNTED aura: the mount aura is removed
--   and the caster self-casts the reindeer matching the mounted
--   speed rates (GetSpeedRate bands 4.1 / 3.8 / 1.6 flight, 2.0
--   run), triggered. Player GetSpeed returns rate * base speed
--   (lua_player.go:5109; playerBaseMoveSpeed MOVE_RUN and
--   MOVE_FLIGHT are both 7.0), so rate = GetSpeed / 7.0.
--   Binding 25860 (AC 'spell_item_reindeer_transformation'; TDB
--   26091 renames it 'spell_winter_veil_reindeer_transformation').
local SPELL_REINDEER_TRANSFORMATION = 25860 -- spell_item.cpp:2734
local SPELL_FLYING_REINDEER_310 = 44827 -- spell_item.cpp:2726
local SPELL_FLYING_REINDEER_280 = 44825 -- spell_item.cpp:2727
local SPELL_FLYING_REINDEER_60 = 44824 -- spell_item.cpp:2728
local SPELL_REINDEER_100 = 25859 -- spell_item.cpp:2729
local SPELL_REINDEER_60 = 25858 -- spell_item.cpp:2730
local SPELL_AURA_MOUNTED = 78 -- SpellAuraDefines.h:158
local MOVE_RUN = 1 -- Unit.h:263
local MOVE_FLIGHT = 6 -- Unit.h:270
local BASE_MOVE_SPEED = 7.0 -- Unit.cpp:101-112 playerBaseMoveSpeed

local function reindeerTransformationHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if not caster:HasAuraType(SPELL_AURA_MOUNTED) then
        return
    end
    local flyspeed = caster:GetSpeed(MOVE_FLIGHT) / BASE_MOVE_SPEED
    local speed = caster:GetSpeed(MOVE_RUN) / BASE_MOVE_SPEED
    caster:RemoveAurasByType(SPELL_AURA_MOUNTED)
    if flyspeed >= 4.1 then
        caster:CastSpell(caster, SPELL_FLYING_REINDEER_310, true)
    elseif flyspeed >= 3.8 then
        caster:CastSpell(caster, SPELL_FLYING_REINDEER_280, true)
    elseif flyspeed >= 1.6 then
        caster:CastSpell(caster, SPELL_FLYING_REINDEER_60, true)
    elseif speed >= 2.0 then
        caster:CastSpell(caster, SPELL_REINDEER_100, true)
    else
        caster:CastSpell(caster, SPELL_REINDEER_60, true)
    end
end

RegisterSpellEvent(SPELL_REINDEER_TRANSFORMATION, 5, reindeerTransformationHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_shimmering_vessel::HandleDummy
--   (spell_item.cpp:2640-2654): EFFECT_0 DUMMY. A hit creature is
--   set to JUST_RESPAWNED (Unit.h:206; the Go SetDeathState twin
--   promotes JUST_RESPAWNED -> ALIVE inside the call,
--   lua_creature_events.go:2119). Binding 31225 (AC
--   'spell_item_shimmering_vessel').
local SPELL_SHIMMERING_VESSEL = 31225 -- spell_item.cpp:2640
local TYPEID_UNIT = 3
local DEATH_STATE_JUST_RESPAWNED = 4 -- Unit.h:206

local function shimmeringVesselHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetTypeId() ~= TYPEID_UNIT then
        return
    end
    hitTarget:SetDeathState(DEATH_STATE_JUST_RESPAWNED)
end

RegisterSpellEvent(SPELL_SHIMMERING_VESSEL, 5, shimmeringVesselHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_socrethars_stone::HandleDummy
--   (spell_item.cpp:2851-2881): EFFECT_0 DUMMY. The C++ Load gate
--   (caster area 3900 or 3742) is folded into the handler since
--   Lua has no Load hook: area 3900 self-casts 35743 (to the seat),
--   area 3742 self-casts 35744 (from the seat), triggered; any
--   other area no-ops. Binding 35745 (AC
--   'spell_item_socrethars_stone'; TDB 26091 renames it
--   'spell_netherstorm_socrethars_stone').
local SPELL_SOCRETHARS_STONE = 35745 -- spell_item.cpp:2851
local SPELL_SOCRETHAR_TO_SEAT = 35743 -- spell_item.cpp:2845
local SPELL_SOCRETHAR_FROM_SEAT = 35744 -- spell_item.cpp:2846

local function socretharsStoneHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local areaId = caster:GetAreaId()
    if areaId == 3900 then
        caster:CastSpell(caster, SPELL_SOCRETHAR_TO_SEAT, true)
    elseif areaId == 3742 then
        caster:CastSpell(caster, SPELL_SOCRETHAR_FROM_SEAT, true)
    end
end

RegisterSpellEvent(SPELL_SOCRETHARS_STONE, 5, socretharsStoneHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_brewfest_mount_transformation::HandleDummy
--   (spell_item.cpp:3011-3065): EFFECT_0 DUMMY. Only while the
--   caster has a SPELL_AURA_MOUNTED aura: the mount aura is removed
--   and the caster self-casts the Brewfest mount matching the
--   mounted run-speed rate (GetSpeedRate(MOVE_RUN) >= 2.0 -> the
--   100% mount, else the 60% mount), triggered. 49357 keeps the
--   faction mount (Alliance ram / Horde kodo); 52845 reverses it
--   (Horde ram / Alliance kodo). Player GetSpeed returns rate *
--   base speed (MOVE_RUN base 7.0), so rate = GetSpeed / 7.0
--   (reindeer precedent above). Bindings 49357/52845 (AC
--   'spell_item_brewfest_mount_transformation'; TDB 26091 renames
--   it 'spell_brewfest_mount_transformation').
local SPELL_BREWFEST_MOUNT_TRANSFORM = 49357 -- spell_item.cpp:3005
local SPELL_BREWFEST_MOUNT_TRANSFORM_REVERSE = 52845 -- spell_item.cpp:3006
local SPELL_MOUNT_RAM_100 = 43900 -- spell_item.cpp:3001
local SPELL_MOUNT_RAM_60 = 43899 -- spell_item.cpp:3002
local SPELL_MOUNT_KODO_100 = 49379 -- spell_item.cpp:3003
local SPELL_MOUNT_KODO_60 = 49378 -- spell_item.cpp:3004
local TEAM_ALLIANCE = 0 -- TeamId TEAM_ALLIANCE; player:GetTeam() returns it

local function brewfestMountTransformationHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if not caster:HasAuraType(SPELL_AURA_MOUNTED) then
        return
    end
    caster:RemoveAurasByType(SPELL_AURA_MOUNTED)
    local spellId = spell:GetEntry()
    local isRam
    if spellId == SPELL_BREWFEST_MOUNT_TRANSFORM then
        isRam = caster:GetTeam() == TEAM_ALLIANCE
    elseif spellId == SPELL_BREWFEST_MOUNT_TRANSFORM_REVERSE then
        isRam = caster:GetTeam() ~= TEAM_ALLIANCE
    else
        return
    end
    local speed = caster:GetSpeed(MOVE_RUN) / BASE_MOVE_SPEED
    if isRam then
        if speed >= 2.0 then
            caster:CastSpell(caster, SPELL_MOUNT_RAM_100, true)
        else
            caster:CastSpell(caster, SPELL_MOUNT_RAM_60, true)
        end
    else
        if speed >= 2.0 then
            caster:CastSpell(caster, SPELL_MOUNT_KODO_100, true)
        else
            caster:CastSpell(caster, SPELL_MOUNT_KODO_60, true)
        end
    end
end

RegisterSpellEvent(SPELL_BREWFEST_MOUNT_TRANSFORM, 5, brewfestMountTransformationHit)
RegisterSpellEvent(SPELL_BREWFEST_MOUNT_TRANSFORM_REVERSE, 5, brewfestMountTransformationHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_teach_language::HandleDummy
--   (spell_item.cpp:3141-3167): EFFECT_0 DUMMY. The C++ Load gate
--   (caster TYPEID_PLAYER) is folded into the handler since Lua
--   has no Load hook. On a 34% roll the caster self-casts the
--   faction binary language (Alliance 50242 Gnomish, Horde 50246
--   Goblin), triggered. Binding 50243 (AC
--   'spell_item_teach_language'; TDB 26091 renames it
--   'spell_brewfest_botm_teach_language').
local SPELL_TEACH_LANGUAGE = 50243 -- spell_item.cpp:3141
local SPELL_LEARN_GNOMISH_BINARY = 50242 -- spell_item.cpp:3136
local SPELL_LEARN_GOBLIN_BINARY = 50246 -- spell_item.cpp:3137

local function teachLanguageHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    if math.random(1, 100) <= 34 then
        if caster:GetTeam() == TEAM_ALLIANCE then
            caster:CastSpell(caster, SPELL_LEARN_GNOMISH_BINARY, true)
        else
            caster:CastSpell(caster, SPELL_LEARN_GOBLIN_BINARY, true)
        end
    end
end

RegisterSpellEvent(SPELL_TEACH_LANGUAGE, 5, teachLanguageHit)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_universal_remote::HandleDummy (spell_item.cpp:4052-4086):
--   EFFECT_0 DUMMY. The C++ Load gate (cast from an item) rides
--   spell:GetCastItem() (lua_spell.go:294). On a hit unit the caster
--   rolls urand(0, 99): <15 casts Target Lock 8347, <25 Mobility
--   Malfunction 8346, else Control Machine 8345 — all on the hit
--   unit with the cast item. Binding 8344 per TDB 335.26091
--   spell_script_names ('spell_item_universal_remote'); AC binds it
--   to 'spell_item_gnomish_universal_remote', a loader name that
--   exists nowhere in the C++ tree — a dead row, not followed.
--   The item rides CastCustomSpell's castItem arg (the
--   itemTriggerSpell precedent above); the ev5 fire site covers the
--   player path (spells.go:14735).
local SPELL_GNOMISH_UNIVERSAL_REMOTE = 8344 -- spell_item.cpp:4052
local SPELL_UNIVERSAL_REMOTE_CONTROL_MACHINE = 8345 -- spell_item.cpp:4047
local SPELL_UNIVERSAL_REMOTE_MOBILITY_MALFUNCTION = 8346 -- spell_item.cpp:4048
local SPELL_UNIVERSAL_REMOTE_TARGET_LOCK = 8347 -- spell_item.cpp:4049

local function universalRemoteHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    local item = spell:GetCastItem()
    if item == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local chance = math.random(0, 99)
    local triggered
    if chance < 15 then
        triggered = SPELL_UNIVERSAL_REMOTE_TARGET_LOCK
    elseif chance < 25 then
        triggered = SPELL_UNIVERSAL_REMOTE_MOBILITY_MALFUNCTION
    else
        triggered = SPELL_UNIVERSAL_REMOTE_CONTROL_MACHINE
    end
    caster:CastCustomSpell(hitTarget, triggered, true, nil, nil, nil, item)
end

RegisterSpellEvent(SPELL_GNOMISH_UNIVERSAL_REMOTE, 5, universalRemoteHit)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_item_harm_prevention_belt::HandleProc (spell_item.cpp:1075-1078):
--   whole-aura OnProc (AuraProcFn, no effect binding). No
--   PreventDefaultAction in C++ — the proc effect is a dummy, so the
--   default leg is a no-op and the handler returns nothing. The aura
--   target self-casts Forcefield Collapse 13235, triggered.
--   DORMANT until Spell.dbc ships the 13234 proc entry.
local SPELL_HARM_PREVENTION_BELT = 13234 -- spell_item.cpp:1066 (class spell_item_harm_prevention_belt)
local SPELL_FORCEFIELD_COLLAPSE = 13235 -- spell_item.cpp:1063

local function harmPreventionBeltProc(event, aura, target)
    if target == nil then
        return
    end
    target:CastSpell(target, SPELL_FORCEFIELD_COLLAPSE, true)
end

RegisterAuraEvent(SPELL_HARM_PREVENTION_BELT, 6, harmPreventionBeltProc)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_dimensional_ripper_everlook::HandleScript (spell_item.cpp:1794-1807):
--   EFFECT_0 TELEPORT_UNITS. C++ Load gate (caster is a player) rides
--   GetTypeId (teach-language precedent in this file). irand(0, 119):
--   <=70 the default teleport proceeds; <100 the caster self-casts
--   Evil Twin 23445, triggered; else Transporter Malfunction (Fire)
--   23449, triggered. No PreventHitDefaultEffect in C++ — the
--   teleport fires on the malfunction paths too, matching the
--   ultrasafe/area52 transporter siblings (spell_item.cpp:1838-1877,
--   1893+). The ev5 fire site covers the player path
--   (spells.go:14735).
--   DORMANT until Spell.dbc ships the 23442 row.
local SPELL_DIMENSIONAL_RIPPER_EVERLOOK = 23442 -- spell_item.cpp:1780 (class spell_item_dimensional_ripper_everlook)
local SPELL_EVIL_TWIN = 23445 -- spell_item.cpp:1768
local SPELL_TRANSPORTER_MALFUNCTION_FIRE = 23449 -- spell_item.cpp:1769

local function dimensionalRipperEverlookHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local r = math.random(0, 119)
    if r <= 70 then
        return
    end
    if r < 100 then
        caster:CastSpell(caster, SPELL_EVIL_TWIN, true)
    else
        caster:CastSpell(caster, SPELL_TRANSPORTER_MALFUNCTION_FIRE, true)
    end
end

RegisterSpellEvent(SPELL_DIMENSIONAL_RIPPER_EVERLOOK, 5, dimensionalRipperEverlookHit)

local SPELL_MAP_OF_THE_GEYSER_FIELDS = 45853 -- spell_item.cpp:2493 (class spell_item_map_of_the_geyser_fields)
local NPC_SOUTH_SINKHOLE = 25664 -- spell_item.cpp:2488
local NPC_NORTHEAST_SINKHOLE = 25665 -- spell_item.cpp:2489
local NPC_NORTHWEST_SINKHOLE = 25666 -- spell_item.cpp:2490
local SPELL_FAILED_CUSTOM_ERROR = 172 -- SharedDefines.h:1154
local SPELL_CUSTOM_ERROR_MUST_BE_CLOSE_TO_SINKHOLE = 22 -- SharedDefines.h:1198
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_item_map_of_the_geyser_fields::CheckSinkholes
-- (spell_item.cpp:2497-2507, registered OnCheckCast): OK only when a
-- sinkhole (25664/25665/25666) is within 30yd, else
-- SPELL_FAILED_CUSTOM_ERROR with MUST_BE_CLOSE_TO_SINKHOLE.
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
-- Binding 45853 is the AC spell_script_names row.
local function geyserFieldsMapCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_MUST_BE_CLOSE_TO_SINKHOLE
    end
    if caster:FindNearestCreature(NPC_SOUTH_SINKHOLE, 30) ~= nil
        or caster:FindNearestCreature(NPC_NORTHEAST_SINKHOLE, 30) ~= nil
        or caster:FindNearestCreature(NPC_NORTHWEST_SINKHOLE, 30) ~= nil then
        return SPELL_CAST_OK
    end
    return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_MUST_BE_CLOSE_TO_SINKHOLE
end

RegisterSpellEvent(SPELL_MAP_OF_THE_GEYSER_FIELDS, 8, geyserFieldsMapCheckCast)

local SPELL_DEMON_BROILED_SURPRISE = 43723 -- spell_item.cpp:2893 (class spell_item_demon_broiled_surprise; AC spell_script_names row)
local QUEST_SUPER_HOT_STEW = 11379 -- spell_item.cpp:2888
local NPC_ABYSSAL_FLAMEBRINGER = 19973 -- spell_item.cpp:2890
local QUEST_STATUS_INCOMPLETE = 3
local SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW = 173 -- SharedDefines.h:1155
local SPELL_FAILED_NOT_HERE = 60 -- SharedDefines.h:1042

-- C++ spell_item_demon_broiled_surprise::CheckRequirement
-- (spell_item.cpp:2915-2925, registered OnCheckCast): quest 11379 must
-- be incomplete (CANT_DO_THAT_RIGHT_NOW otherwise) and a dead Abyssal
-- Flamebringer (19973) must be within 10yd (NOT_HERE otherwise).
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
-- The 43723 binding is the AC spell_script_names row — the triage's
-- 43753 is the spell the dummy arm creates, not the checked spell. The
-- dead-creature search rides the new optional FindNearestCreature
-- alive=false leg (player + creature surfaces). HandleDummy ported
-- below (RegisterSpellEvent OnEffectHitTarget = 5, EFFECT_1).
local SPELL_CREATE_DEMON_BROILED_SURPRISE = 43753 -- spell_item.cpp:2889
local function demonBroiledSurpriseCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW
    end
    if caster:GetQuestStatus(QUEST_SUPER_HOT_STEW) ~= QUEST_STATUS_INCOMPLETE then
        return SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW
    end
    local creature = caster:FindNearestCreature(NPC_ABYSSAL_FLAMEBRINGER, 10, false)
    if creature ~= nil and creature:IsDead() then
        return SPELL_CAST_OK
    end
    return SPELL_FAILED_NOT_HERE
end

RegisterSpellEvent(SPELL_DEMON_BROILED_SURPRISE, 8, demonBroiledSurpriseCheckCast)

-- C++ spell_item_demon_broiled_surprise::HandleDummy
-- (spell_item.cpp:2909-2913, registered OnEffectHitTarget EFFECT_1
-- SPELL_EFFECT_DUMMY): the caster self-casts 43753, non-triggered.
-- C++ Load() gates the script to player casters; the handler keeps
-- that gate via the Player object-type check.
local function demonBroiledSurpriseHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil and caster:GetObjectType() == "Player" then
        caster:CastSpell(caster, SPELL_CREATE_DEMON_BROILED_SURPRISE, false)
    end
end

RegisterSpellEvent(SPELL_DEMON_BROILED_SURPRISE, 5, demonBroiledSurpriseHit)

local SPELL_ROCKET_BOOTS = 51582 -- spell_item.cpp:3174 (class spell_item_rocket_boots; AC spell_script_names row)
local SPELL_ROCKET_BOOTS_PROC = 30452 -- spell_item.cpp:3171
local SPELL_FAILED_ONLY_ABOVEWATER = 88 -- SharedDefines.h:1070

-- C++ spell_item_rocket_boots::CheckCast (spell_item.cpp:3198-3203,
-- registered OnCheckCast): a caster in water fails
-- SPELL_FAILED_ONLY_ABOVEWATER. RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast). Binding 51582 is the AC
-- spell_script_names row. HandleDummy ported below
-- (RegisterSpellEvent OnEffectHitTarget = 5, EFFECT_0); the
-- battleground flag-drop leg stays out (no GetBattleground /
-- EventPlayerDroppedFlag bridge).
local function rocketBootsCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and caster:IsInWater() then
        return SPELL_FAILED_ONLY_ABOVEWATER
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_ROCKET_BOOTS, 8, rocketBootsCheckCast)

-- C++ spell_item_rocket_boots::HandleDummy
-- (spell_item.cpp:3188-3196, registered OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_DUMMY): the proc cooldown resets, then the caster
-- self-casts 30452, triggered. C++ Load() gates the script to player
-- casters; the handler keeps that gate via the Player object-type
-- check (ResetSpellCooldown is player-only).
local function rocketBootsHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil and caster:GetObjectType() == "Player" then
        caster:ResetSpellCooldown(SPELL_ROCKET_BOOTS_PROC)
        caster:CastSpell(caster, SPELL_ROCKET_BOOTS_PROC, true)
    end
end

RegisterSpellEvent(SPELL_ROCKET_BOOTS, 5, rocketBootsHit)

local SPELL_ITEM_GIFT_OF_THE_HARVESTER = 52481 -- spell_item.cpp:2463 (class spell_item_gift_of_the_harvester; AC spell_script_names row)
local NPC_GHOUL = 28845 -- spell_item.cpp:2459
local MAX_GHOULS = 5 -- spell_item.cpp:2460
local SPELL_CUSTOM_ERROR_TOO_MANY_GHOULS = 30 -- SharedDefines.h:1206

-- C++ spell_item_gift_of_the_harvester::CheckRequirement
-- (spell_item.cpp:2467-2478, registered OnCheckCast): the caster
-- fails SPELL_FAILED_CUSTOM_ERROR with TOO_MANY_GHOULS once 5 ghouls
-- (28845) are already summoned. RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast). The two-value custom-error
-- return rides the geyser-fields precedent above; GetAllMinionsByEntry
-- is bridged on the player surface (lua_player.go:3341).
local function giftOfTheHarvesterCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil then
        local ghouls = caster:GetAllMinionsByEntry(NPC_GHOUL)
        if ghouls ~= nil and #ghouls >= MAX_GHOULS then
            return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_TOO_MANY_GHOULS
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_ITEM_GIFT_OF_THE_HARVESTER, 8, giftOfTheHarvesterCheckCast)

local SPELL_ITEM_BOOK_OF_GLYPH_MASTERY = 64323 -- spell_item.cpp:2420 (class spell_item_book_of_glyph_mastery; TDB spell_script_names row, 25101 + 26091)
local SPELL_CUSTOM_ERROR_LEARNED_EVERYTHING = 62 -- SharedDefines.h:1238

-- C++ spell_item_book_of_glyph_mastery::CheckRequirement
-- (spell_item.cpp:2429-2438, registered OnCheckCast): the caster
-- fails SPELL_FAILED_CUSTOM_ERROR with LEARNED_EVERYTHING once
-- every glyph the book can teach is already known
-- (HasDiscoveredAllSpells, SkillDiscovery.cpp:195-208; 57
-- discoverable spells in TDB 25101 skill_discovery_template).
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go
-- fireSpellCheckCast). The HandleScript learn leg is ported below
-- (RegisterSpellEvent OnEffectHitTarget = 5, EFFECT_0): the weighted
-- player:GetExplicitDiscoverySpell roll (skill_discovery.go) +
-- player:LearnSpell (spell_item.cpp:2440-2447).
local function bookOfGlyphMasteryCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and caster:HasDiscoveredAllSpells(SPELL_ITEM_BOOK_OF_GLYPH_MASTERY) then
        return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_LEARNED_EVERYTHING
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_ITEM_BOOK_OF_GLYPH_MASTERY, 8, bookOfGlyphMasteryCheckCast)

-- C++ spell_item_book_of_glyph_mastery::HandleScript
-- (spell_item.cpp:2440-2447, registered OnEffectHitTarget EFFECT_0,
-- SPELL_EFFECT_SCRIPT_EFFECT): the caster learns one random explicit
-- discovery glyph (GetExplicitDiscoverySpell + LearnSpell). ev5 fires
-- per (effect, target) pair, so the EFFECT_0 guard is the
-- registration twin; the C++ Load() player gate rides the Player
-- object-type check (same idiom as the rocket-boots port above).
local function bookOfGlyphMasteryHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local discovered = caster:GetExplicitDiscoverySpell(SPELL_ITEM_BOOK_OF_GLYPH_MASTERY)
    if discovered ~= 0 then
        caster:LearnSpell(discovered)
    end
end

RegisterSpellEvent(SPELL_ITEM_BOOK_OF_GLYPH_MASTERY, 5, bookOfGlyphMasteryHit)

-- PORTED (RegisterSpellEvent OnEffectHit = 4; spell_item.cpp ev4 strip, 2026-11-20):
--   spell_item_deviate_fish::HandleDummy (:812-817): EFFECT_0 DUMMY; player Load gate
--   folded in; urand(8064, 8068) self-cast, triggered.
--   spell_item_flask_of_the_north::HandleDummy (:956-981): EFFECT_0 DUMMY; class-based
--   flask (warlock/mage/priest SP; DK/warrior STR; rogue/hunter AP; druid/paladin
--   SP-or-STR; shaman SP-or-AP), triggered. No C++ Load gate; a non-player caster
--   has no class entry and returns (GetClass resolves via the object fallback).
--   spell_item_make_a_wish::HandleDummy (:1248-1261): EFFECT_0 DUMMY; player Load gate
--   folded in; urand 1-5 -> 33053/33057/33059/33062/33064, triggered.
--   spell_item_mingos_fortune_generator::HandleDummy (:1345-1372): EFFECT_0 DUMMY;
--   urand 1-20 -> item 32688-32693/32700-32713 via player:AddItem (CreateItem twin;
--   no spell-object CreateItem bridge). Player gate is the AddItem surface, not C++.
--   spell_item_underbelly_elixir::HandleDummy (:2277-2289): EFFECT_0 DUMMY; player Load
--   gate folded in; urand 1-3 -> 59645/59831/59843, triggered.
--   spell_item_noggenfogger_elixir::HandleDummy (:1479-1490): EFFECT_0 DUMMY; player
--   Load gate folded in; urand 1-3 -> 16595/16593/16591, triggered.
--   spell_item_savory_deviate_delight::HandleDummy (:1683-1694): EFFECT_0 DUMMY; player
--   Load gate folded in; urand 1-2 flip-out/pirate, gendered (GetNativeGender).
--   spell_item_vanquished_clutches::HandleDummy (:2538-2544): EFFECT_0 DUMMY; RAND
--   64982/64983/64984 self-cast, triggered.
--   Bindings 8063/67019/33060/40802/59640/16589/8213/64981 (AC spell_script_names).
--   DORMANT until Spell.dbc ships the binding rows.
--   Consts grouped in one table: the main-chunk local limit is 200 (file at 160).
local EV4_ITEM = {
    DEVIATE_FISH = 8063,
    FLASK_OF_THE_NORTH = 67019,
    FLASK_SP = 67016,
    FLASK_AP = 67017,
    FLASK_STR = 67018,
    MAKE_A_WISH = 33060,
    MINGOS_FORTUNE_GENERATOR = 40802,
    UNDERBELLY_ELIXIR = 59640,
    NOGGENFOGGER_ELIXIR = 16589,
    SAVORY_DEVIATE_DELIGHT = 8213,
    VANQUISHED_CLUTCHES = 64981,
    FLIP_OUT_MALE = 8219,
    FLIP_OUT_FEMALE = 8220,
    YAAARRRR_MALE = 8221,
    YAAARRRR_FEMALE = 8222,
}
local GENDER_MALE = 0 -- SharedDefines.h:76

local function deviateFishHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    caster:CastSpell(caster, math.random(8064, 8068), true)
end

RegisterSpellEvent(EV4_ITEM.DEVIATE_FISH, 4, deviateFishHit)

-- Class ids (SharedDefines.h): 1 warrior, 2 paladin, 3 hunter, 4 rogue, 5 priest,
-- 6 death knight, 7 shaman, 8 mage, 9 warlock, 11 druid.
local FLASK_BY_CLASS = {
    [9] = { EV4_ITEM.FLASK_SP },
    [8] = { EV4_ITEM.FLASK_SP },
    [5] = { EV4_ITEM.FLASK_SP },
    [6] = { EV4_ITEM.FLASK_STR },
    [1] = { EV4_ITEM.FLASK_STR },
    [4] = { EV4_ITEM.FLASK_AP },
    [3] = { EV4_ITEM.FLASK_AP },
    [11] = { EV4_ITEM.FLASK_SP, EV4_ITEM.FLASK_STR },
    [2] = { EV4_ITEM.FLASK_SP, EV4_ITEM.FLASK_STR },
    [7] = { EV4_ITEM.FLASK_SP, EV4_ITEM.FLASK_AP },
}

local function flaskOfTheNorthHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local choices = FLASK_BY_CLASS[caster:GetClass()]
    if choices == nil then
        return
    end
    caster:CastSpell(caster, choices[math.random(1, #choices)], true)
end

RegisterSpellEvent(EV4_ITEM.FLASK_OF_THE_NORTH, 4, flaskOfTheNorthHit)

local MAKE_A_WISH_SPELLS = { 33053, 33057, 33059, 33062, 33064 }

local function makeAWishHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    caster:CastSpell(caster, MAKE_A_WISH_SPELLS[math.random(1, 5)], true)
end

RegisterSpellEvent(EV4_ITEM.MAKE_A_WISH, 4, makeAWishHit)

local MINGOS_ITEMS = { 32688, 32689, 32690, 32691, 32692, 32693, 32700, 32701, 32702, 32703,
    32704, 32705, 32706, 32707, 32708, 32709, 32710, 32711, 32712, 32713 }

local function mingosFortuneGeneratorHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    caster:AddItem(MINGOS_ITEMS[math.random(1, 20)], 1)
end

RegisterSpellEvent(EV4_ITEM.MINGOS_FORTUNE_GENERATOR, 4, mingosFortuneGeneratorHit)

local function underbellyElixirHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local roll = math.random(1, 3)
    local spellId = 59843
    if roll == 1 then
        spellId = 59645
    elseif roll == 2 then
        spellId = 59831
    end
    caster:CastSpell(caster, spellId, true)
end

RegisterSpellEvent(EV4_ITEM.UNDERBELLY_ELIXIR, 4, underbellyElixirHit)

local function noggenfoggerElixirHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local roll = math.random(1, 3)
    local spellId = 16591
    if roll == 1 then
        spellId = 16595
    elseif roll == 2 then
        spellId = 16593
    end
    caster:CastSpell(caster, spellId, true)
end

RegisterSpellEvent(EV4_ITEM.NOGGENFOGGER_ELIXIR, 4, noggenfoggerElixirHit)

local function savoryDeviateDelightHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local male = caster:GetNativeGender() == GENDER_MALE
    if math.random(1, 2) == 1 then
        caster:CastSpell(caster, male and EV4_ITEM.FLIP_OUT_MALE or EV4_ITEM.FLIP_OUT_FEMALE, true)
    else
        caster:CastSpell(caster, male and EV4_ITEM.YAAARRRR_MALE or EV4_ITEM.YAAARRRR_FEMALE, true)
    end
end

RegisterSpellEvent(EV4_ITEM.SAVORY_DEVIATE_DELIGHT, 4, savoryDeviateDelightHit)

local VANQUISHED_SPELLS = { 64982, 64983, 64984 }

local function vanquishedClutchesHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, VANQUISHED_SPELLS[math.random(1, 3)], true)
end

RegisterSpellEvent(EV4_ITEM.VANQUISHED_CLUTCHES, 4, vanquishedClutchesHit)

-- PORTED (RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_item_brittle_armor::HandleScript (:419-422): hit unit RemoveAuraFromStack(24575).
--   spell_item_goblin_weather_machine::HandleScript (:699-705): RAND 46740/46739/46738/46736,
--   hit unit casts on itself (C++ target->CastSpell(target), not the caster).
--   spell_item_gnomish_death_ray::HandleDummy (:1049-1059): 15% self 13493, else hit unit 13279.
--   spell_item_mercurial_shield::HandleScript (:1314-1317): hit unit RemoveAuraFromStack(26464).
--   spell_item_net_o_matic::HandleDummy (:1436-1449): roll <2 -> 16566, <4 -> 13119, else 13099.
--   spell_item_mind_control_cap::HandleDummy (:4031-4042): item-gated; 95%: 32% dullard 67809
--   else mind-control 13181 (with item); 5% backfire: target casts 13181 on caster.
--   spell_item_eggnog::HandleScript (:4280-4285, EFFECT_2 INEBRIATE): 40% then 50%: 21936/21980
--   on hit unit, with cast item.
--   spell_item_mirrens_drinking_hat::HandleScriptEffect (:3985-4002): urand 1-6: 1-3 -> 29827,
--   4-5 -> 29828, 6 -> 29829, non-triggered.
--   spell_item_six_demon_bag::HandleDummy (:2141-2171): weighted fireball 15662 / frostbolt 11538 /
--   chain lightning 21179 / polymorph 14621 (30% self) / enveloping winds 25189 / felhunter
--   14642 on caster, with cast item.
--   spell_item_nigh_invulnerability::HandleDummy (:2806-2815): item-gated; 86% 30456 else 30457,
--   with cast item.
--   spell_item_poultryizer::HandleDummy (:2839-2843): item-gated; 80% 30501 else 30504 on hit
--   unit, with cast item.
--   spell_item_complete_raptor_capture::HandleDummy (:2954-2961): despawn hit creature, self-cast 42337.
--   spell_item_impale_leviroth::HandleDummy (:2988-2997): entry-26452 creature at >=95% health
--   self-casts 49882 + ResetPlayerDamageReq().
--   spell_item_pygmy_oil::HandleDummy (:3255-3271): 53806 present -> refresh duration
--   (SetDuration(GetMaxDuration()) twin); else 53805 <5 stacks or 50% -> cast 53805,
--   else remove 53805 + cast 53806.
--   Bindings 24590/46203/13280/26465/13120/13180/21149/29830/14537/30458/30507/44875/47170/53808
--   (AC spell_script_names). DORMANT until Spell.dbc ships the binding rows.
local EV5_ITEM = {
    BRITTLE_ARMOR = 24590,
    BRITTLE_ARMOR_AURA = 24575,
    GOBLIN_WEATHER_MACHINE = 46203,
    WEATHER = { 46740, 46739, 46738, 46736 },
    GNOMISH_DEATH_RAY = 13280,
    GNOMISH_DEATH_RAY_SELF = 13493,
    GNOMISH_DEATH_RAY_TARGET = 13279,
    MERCURIAL_SHIELD = 26465,
    MERCURIAL_SHIELD_AURA = 26464,
    NET_O_MATIC = 13120,
    NET_O_MATIC_TRIGGERED1 = 16566,
    NET_O_MATIC_TRIGGERED2 = 13119,
    NET_O_MATIC_TRIGGERED3 = 13099,
    MIND_CONTROL_CAP = 13180,
    GNOMISH_MIND_CONTROL_CAP = 13181,
    DULLARD = 67809,
    EGGNOG = 21149,
    EGG_NOG_REINDEER = 21936,
    EGG_NOG_SNOWMAN = 21980,
    MIRRENS_DRINKING_HAT = 29830,
    LOCH_MODAN_LAGER = 29827,
    STOUTHAMMER_LITE = 29828,
    AERIE_PEAK_PALE_ALE = 29829,
    SIX_DEMON_BAG = 14537,
    FIREBALL = 15662,
    FROSTBOLT = 11538,
    CHAIN_LIGHTNING = 21179,
    POLYMORPH = 14621,
    ENVELOPING_WINDS = 25189,
    SUMMON_FELHOUND_MINION = 14642,
    NIGH_INVULNERABILITY = 30458,
    NIGH_INVULNERABILITY_BUFF = 30456,
    COMPLETE_VULNERABILITY = 30457,
    POULTRYIZER = 30507,
    POULTRYIZER_SUCCESS = 30501,
    POULTRYIZER_BACKFIRE = 30504,
    COMPLETE_RAPTOR_CAPTURE = 44875,
    RAPTOR_CAPTURE_CREDIT = 42337,
    IMPALE_LEVIROTH = 47170,
    LEVIROTH = 26452,
    LEVIROTH_SELF_IMPALE = 49882,
    PYGMY_OIL = 53808,
    PYGMY_OIL_PYGMY_AURA = 53806,
    PYGMY_OIL_SMALLER_AURA = 53805,
    UNUSUAL_COMPASS = 64385,
    CHICKEN_COVER = 51961,
    CHICKEN_NET = 51959,
    CAPTURE_CHICKEN_ESCAPE = 51037,
    QUEST_CHICKEN_PARTY = 12702,
    QUEST_FLOWN_THE_COOP = 12532,
    REFOCUS = 24531,
    AIMED_SHOT = 19434,
    MULTISHOT = 2643,
    VOLLEY = 42243,
    MUISEK_VESSEL = { 11885, 11886, 11887, 11888, 11889 },
    GREATMOTHERS_SOULCATCHER = 46485,
    FORCE_CAST_SUMMON_GNOME_SOUL = 46486,
    RED_RIDER_AIR_RIFLE = 67533,
    AIR_RIFLE_HOLD_VISUAL = 65582,
    AIR_RIFLE_SHOOT = 67532,
    AIR_RIFLE_SHOOT_SELF = 65577,
    PICCOLO_OF_THE_FLAMING_FIRE = 17512,
    ASHBRINGER = 28441,
    ASHBRINGER_SOUNDS = { 8906, 8907, 8908, 8920, 8921, 8922, 8923, 8924, 8925, 8926, 8927, 8928 },
    ULTRASAFE_TRANSPORTER = 36941,
    DIMENSIONAL_RIPPER_AREA52 = 36890,
    TRANSPORTER_MALFUNCTION_SMALLER = 36893,
    TRANSPORTER_MALFUNCTION_BIGGER = 36895,
    TRANSPORTER_MALFUNCTION_CHICKEN = 36940,
    TRANSFORM_HORDE = 36897,
    TRANSFORM_ALLIANCE = 36899,
    SOUL_SPLIT_EVIL = 36900,
    SOUL_SPLIT_GOOD = 36901,
    CLASS_HUNTER = 3, -- SharedDefines.h:131
    EMOTE_STATE_DANCE = 10, -- SharedDefines.h:1945
}

local function brittleArmorHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAuraFromStack(EV5_ITEM.BRITTLE_ARMOR_AURA)
end

RegisterSpellEvent(EV5_ITEM.BRITTLE_ARMOR, 5, brittleArmorHit)

local function goblinWeatherMachineHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, EV5_ITEM.WEATHER[math.random(1, 4)], true)
end

RegisterSpellEvent(EV5_ITEM.GOBLIN_WEATHER_MACHINE, 5, goblinWeatherMachineHit)

local function gnomishDeathRayHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if math.random(0, 99) < 15 then
        caster:CastSpell(caster, EV5_ITEM.GNOMISH_DEATH_RAY_SELF, true)
    else
        caster:CastSpell(hitTarget, EV5_ITEM.GNOMISH_DEATH_RAY_TARGET, true)
    end
end

RegisterSpellEvent(EV5_ITEM.GNOMISH_DEATH_RAY, 5, gnomishDeathRayHit)

local function mercurialShieldHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAuraFromStack(EV5_ITEM.MERCURIAL_SHIELD_AURA)
end

RegisterSpellEvent(EV5_ITEM.MERCURIAL_SHIELD, 5, mercurialShieldHit)

local function netOMaticHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local roll = math.random(0, 99)
    local spellId = EV5_ITEM.NET_O_MATIC_TRIGGERED3
    if roll < 2 then
        spellId = EV5_ITEM.NET_O_MATIC_TRIGGERED1
    elseif roll < 4 then
        spellId = EV5_ITEM.NET_O_MATIC_TRIGGERED2
    end
    caster:CastSpell(hitTarget, spellId, true)
end

RegisterSpellEvent(EV5_ITEM.NET_O_MATIC, 5, netOMaticHit)

local function mindControlCapHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local item = spell:GetCastItem()
    if item == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if math.random(1, 100) <= 95 then
        local spellId = EV5_ITEM.GNOMISH_MIND_CONTROL_CAP
        if math.random(1, 100) <= 32 then
            spellId = EV5_ITEM.DULLARD
        end
        caster:CastCustomSpell(hitTarget, spellId, false, nil, nil, nil, item)
    else
        hitTarget:CastSpell(caster, EV5_ITEM.GNOMISH_MIND_CONTROL_CAP, true)
    end
end

RegisterSpellEvent(EV5_ITEM.MIND_CONTROL_CAP, 5, mindControlCapHit)

local function eggnogHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 2 or hitTarget == nil then
        return
    end
    if math.random(1, 100) <= 40 then
        local caster = spell:GetCaster()
        if caster == nil then
            return
        end
        local spellId = EV5_ITEM.EGG_NOG_SNOWMAN
        if math.random(1, 100) <= 50 then
            spellId = EV5_ITEM.EGG_NOG_REINDEER
        end
        caster:CastCustomSpell(hitTarget, spellId, false, nil, nil, nil, spell:GetCastItem())
    end
end

RegisterSpellEvent(EV5_ITEM.EGGNOG, 5, eggnogHit)

local function mirrensDrinkingHatHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local roll = math.random(1, 6)
    local spellId = EV5_ITEM.AERIE_PEAK_PALE_ALE
    if roll <= 3 then
        spellId = EV5_ITEM.LOCH_MODAN_LAGER
    elseif roll <= 5 then
        spellId = EV5_ITEM.STOUTHAMMER_LITE
    end
    caster:CastSpell(caster, spellId, false)
end

RegisterSpellEvent(EV5_ITEM.MIRRENS_DRINKING_HAT, 5, mirrensDrinkingHatHit)

local function sixDemonBagHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local item = spell:GetCastItem()
    local target = hitTarget
    local spellId
    local rand = math.random(0, 99)
    if rand < 25 then
        spellId = EV5_ITEM.FIREBALL
    elseif rand < 50 then
        spellId = EV5_ITEM.FROSTBOLT
    elseif rand < 70 then
        spellId = EV5_ITEM.CHAIN_LIGHTNING
    elseif rand < 80 then
        spellId = EV5_ITEM.POLYMORPH
        if math.random(0, 100) <= 30 then
            target = caster
        end
    elseif rand < 95 then
        spellId = EV5_ITEM.ENVELOPING_WINDS
    else
        spellId = EV5_ITEM.SUMMON_FELHOUND_MINION
        target = caster
    end
    caster:CastCustomSpell(target, spellId, false, nil, nil, nil, item)
end

RegisterSpellEvent(EV5_ITEM.SIX_DEMON_BAG, 5, sixDemonBagHit)

local function nighInvulnerabilityHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local item = spell:GetCastItem()
    if item == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if math.random(1, 100) <= 86 then
        caster:CastCustomSpell(caster, EV5_ITEM.NIGH_INVULNERABILITY_BUFF, false, nil, nil, nil, item)
    else
        caster:CastCustomSpell(caster, EV5_ITEM.COMPLETE_VULNERABILITY, false, nil, nil, nil, item)
    end
end

RegisterSpellEvent(EV5_ITEM.NIGH_INVULNERABILITY, 5, nighInvulnerabilityHit)

local function poultryizerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local item = spell:GetCastItem()
    if item == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local spellId = EV5_ITEM.POULTRYIZER_BACKFIRE
    if math.random(1, 100) <= 80 then
        spellId = EV5_ITEM.POULTRYIZER_SUCCESS
    end
    caster:CastCustomSpell(hitTarget, spellId, false, nil, nil, nil, item)
end

RegisterSpellEvent(EV5_ITEM.POULTRYIZER, 5, poultryizerHit)

local function completeRaptorCaptureHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:DespawnOrUnsummon()
    caster:CastSpell(caster, EV5_ITEM.RAPTOR_CAPTURE_CREDIT, true)
end

RegisterSpellEvent(EV5_ITEM.COMPLETE_RAPTOR_CAPTURE, 5, completeRaptorCaptureHit)

local function impaleLevirothHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:GetEntry() == EV5_ITEM.LEVIROTH and not hitTarget:HealthBelowPct(95) then
        hitTarget:CastSpell(hitTarget, EV5_ITEM.LEVIROTH_SELF_IMPALE, true)
        hitTarget:ResetPlayerDamageReq()
    end
end

RegisterSpellEvent(EV5_ITEM.IMPALE_LEVIROTH, 5, impaleLevirothHit)

local function pygmyOilHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local aura = caster:GetAura(EV5_ITEM.PYGMY_OIL_PYGMY_AURA)
    if aura ~= nil then
        aura:SetDuration(aura:GetMaxDuration())
    else
        aura = caster:GetAura(EV5_ITEM.PYGMY_OIL_SMALLER_AURA)
        if aura == nil or aura:GetStackAmount() < 5 or math.random(1, 100) > 50 then
            caster:CastSpell(caster, EV5_ITEM.PYGMY_OIL_SMALLER_AURA, true)
        else
            aura:Remove()
            caster:CastSpell(caster, EV5_ITEM.PYGMY_OIL_PYGMY_AURA, true)
        end
    end
end

RegisterSpellEvent(EV5_ITEM.PYGMY_OIL, 5, pygmyOilHit)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5 unless noted):
--   spell_item_unusual_compass::HandleDummy (spell_item.cpp:3283-3287): EFFECT_0 DUMMY.
--   Caster faces a random angle 0..2pi (frand twin).
--   spell_item_chicken_cover::HandleDummy (:3313-3326): EFFECT_0 DUMMY. C++ Load gate
--   (caster TYPEID_PLAYER) folded into the handler. No 51959 aura on the hit unit and
--   quest 12702 or 12532 incomplete -> caster casts 51037 on self, hit unit Kill()
--   (KillSelf twin).
--   spell_item_refocus::HandleDummy (:3353-3365): EFFECT_0 DUMMY. Hunter only;
--   ResetSpellCooldown 19434/2643/42243 when each holds a cooldown.
--   spell_item_muisek_vessel::HandleDummy (:3378-3383): EFFECT_0 DUMMY. Dead hit
--   creature despawns. One handler, five bindings 11885-11889.
--   spell_item_greatmothers_soulcatcher::HandleDummy (:3403-3407): EFFECT_0 DUMMY.
--   Hit unit present -> caster self-casts 46486, non-triggered.
--   spell_item_red_rider_air_rifle::HandleScript (:2354-2378): EFFECT_0 SCRIPT_EFFECT.
--   PreventHitDefaultEffect; hold visual 65582 triggered, then urand(0,4): nonzero
--   shoots 67532 at the target, zero shoots 65577 at self, both non-triggered.
--   The GCD-cancel leg is moot in Go (direct-cast path never consults GCD).
--   spell_item_piccolo_of_the_flaming_fire::HandleScript (:1609-1614): EFFECT_0
--   SCRIPT_EFFECT. PreventHitDefaultEffect; hit player performs EMOTE_STATE_DANCE (10).
--   spell_item_ashbringer::OnDummyEffect (:2584-2596): EFFECT_0 DUMMY, registered
--   OnEffectHit (event 4, not ev5 — C++ uses OnEffectHit +=). PreventHitDefaultEffect;
--   1/61 PlayDirectSound of one of the 12 sounds (one-arg bridge, illidan precedent).
--   spell_item_ultrasafe_transporter::HandleScript (:1843-1877) and
--   spell_item_dimensional_ripper_area52::HandleScript (:1911-1944): EFFECT_0
--   TELEPORT_UNITS. 50% roll, then urand pick (0-6 ultrasafe / 0-3 area52); no veto
--   in C++ so the teleport proceeds on malfunction paths, matching the everlook port.
--   Bindings 64385/51961/24531/11885-11889/46485/67533/17512/28441/36941/36890
--   (AC spell_script_names; red_rider 67533 is TDB). DORMANT until Spell.dbc ships
--   the binding rows.
--   NOTE: handlers are inline anonymous functions (skadi precedent) — the file sits
--   at 189 top-level locals and the Lua main-chunk limit is 200, so no new named
--   local functions were added this tick.
RegisterSpellEvent(EV5_ITEM.UNUSUAL_COMPASS, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:SetFacingTo(math.random() * 2 * math.pi)
end)

RegisterSpellEvent(EV5_ITEM.CHICKEN_COVER, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    if not hitTarget:HasAura(EV5_ITEM.CHICKEN_NET)
        and (caster:GetQuestStatus(EV5_ITEM.QUEST_CHICKEN_PARTY) == QUEST_STATUS_INCOMPLETE
            or caster:GetQuestStatus(EV5_ITEM.QUEST_FLOWN_THE_COOP) == QUEST_STATUS_INCOMPLETE) then
        caster:CastSpell(caster, EV5_ITEM.CAPTURE_CHICKEN_ESCAPE, true)
        hitTarget:Kill()
    end
end)

RegisterSpellEvent(EV5_ITEM.REFOCUS, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    if caster:GetClass() ~= EV5_ITEM.CLASS_HUNTER then
        return
    end
    if caster:HasSpellCooldown(EV5_ITEM.AIMED_SHOT) then
        caster:ResetSpellCooldown(EV5_ITEM.AIMED_SHOT)
    end
    if caster:HasSpellCooldown(EV5_ITEM.MULTISHOT) then
        caster:ResetSpellCooldown(EV5_ITEM.MULTISHOT)
    end
    if caster:HasSpellCooldown(EV5_ITEM.VOLLEY) then
        caster:ResetSpellCooldown(EV5_ITEM.VOLLEY)
    end
end)

for _, spellId in ipairs(EV5_ITEM.MUISEK_VESSEL) do
    RegisterSpellEvent(spellId, 5, function(event, spell, hitTarget, effectIndex)
        if effectIndex ~= 0 or hitTarget == nil then
            return
        end
        if hitTarget:GetObjectType() == "Creature" and hitTarget:IsDead() then
            hitTarget:DespawnOrUnsummon()
        end
    end)
end

RegisterSpellEvent(EV5_ITEM.GREATMOTHERS_SOULCATCHER, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, EV5_ITEM.FORCE_CAST_SUMMON_GNOME_SOUL, false)
end)

RegisterSpellEvent(EV5_ITEM.RED_RIDER_AIR_RIFLE, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, EV5_ITEM.AIR_RIFLE_HOLD_VISUAL, true)
    if math.random(0, 4) ~= 0 then
        caster:CastSpell(hitTarget, EV5_ITEM.AIR_RIFLE_SHOOT, false)
    else
        caster:CastSpell(caster, EV5_ITEM.AIR_RIFLE_SHOOT_SELF, false)
    end
end)

RegisterSpellEvent(EV5_ITEM.PICCOLO_OF_THE_FLAMING_FIRE, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    if hitTarget:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    hitTarget:PerformEmote(EV5_ITEM.EMOTE_STATE_DANCE)
end)

RegisterSpellEvent(EV5_ITEM.ASHBRINGER, 4, function(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    if math.random(0, 60) < 1 then
        caster:PlayDirectSound(EV5_ITEM.ASHBRINGER_SOUNDS[math.random(1, 12)])
    end
end)

RegisterSpellEvent(EV5_ITEM.ULTRASAFE_TRANSPORTER, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if math.random(1, 100) > 50 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local spellId
    local roll = math.random(0, 6)
    if roll == 0 then
        spellId = EV5_ITEM.TRANSPORTER_MALFUNCTION_SMALLER
    elseif roll == 1 then
        spellId = EV5_ITEM.TRANSPORTER_MALFUNCTION_BIGGER
    elseif roll == 2 then
        spellId = EV5_ITEM.SOUL_SPLIT_EVIL
    elseif roll == 3 then
        spellId = EV5_ITEM.SOUL_SPLIT_GOOD
    elseif roll == 4 then
        if caster:GetTeam() == TEAM_ALLIANCE then
            spellId = EV5_ITEM.TRANSFORM_HORDE
        else
            spellId = EV5_ITEM.TRANSFORM_ALLIANCE
        end
    elseif roll == 5 then
        spellId = EV5_ITEM.TRANSPORTER_MALFUNCTION_CHICKEN
    else
        spellId = SPELL_EVIL_TWIN
    end
    caster:CastSpell(caster, spellId, true)
end)

RegisterSpellEvent(EV5_ITEM.DIMENSIONAL_RIPPER_AREA52, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if math.random(1, 100) > 50 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local spellId
    local roll = math.random(0, 3)
    if roll == 0 then
        spellId = EV5_ITEM.TRANSPORTER_MALFUNCTION_BIGGER
    elseif roll == 1 then
        spellId = EV5_ITEM.SOUL_SPLIT_EVIL
    elseif roll == 2 then
        spellId = EV5_ITEM.SOUL_SPLIT_GOOD
    else
        if caster:GetTeam() == TEAM_ALLIANCE then
            spellId = EV5_ITEM.TRANSFORM_HORDE
        else
            spellId = EV5_ITEM.TRANSFORM_ALLIANCE
        end
    end
    caster:CastSpell(caster, spellId, true)
end)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6; handler returns true = PreventDefaultAction):
--   spell_item_aegis_of_preservation::HandleProc (spell_item.cpp:102-106): EFFECT_1
--   PROC_TRIGGER_SPELL. Target self-casts 23781, triggered.
--   spell_item_desperate_defense::HandleProc (:774-778): EFFECT_2. Target self-casts 33898.
--   spell_item_fate_rune_of_unsurpassed_vigor::HandleProc (:920-924): EFFECT_0 DUMMY.
--   Target self-casts 25733, triggered.
--   spell_item_discerning_eye_beast_dummy::HandleProc (:840-844): EFFECT_0 DUMMY.
--   Actor casts 59914 on self (nullptr-target twin, heartpierce precedent).
--   spell_item_healing_touch_refund::HandleProc (:1102-1106): EFFECT_0 DUMMY.
--   Actor casts 28848 on self.
--   spell_item_totem_of_flowing_water::HandleProc (:2221-2225): EFFECT_0 DUMMY.
--   Actor casts 28850 on self.
--   spell_item_soul_preserver::HandleProc (:3480-3506): EFFECT_0 PROC_TRIGGER_SPELL.
--   Actor casts the class spell 60512/60513/60514/60515.
--   spell_item_death_choice::HandleProc (:3643-3673): EFFECT_0 PROC_TRIGGER_SPELL.
--   STR > AGI picks the strength buff, else agility; per-aura registration reproduces
--   the C++ aurEff->GetId() switch (67702 -> 67708/67703, 67771 -> 67773/67772).
--   spell_item_darkmoon_card_greatness::HandleProc (:3788-3824): EFFECT_0
--   PROC_TRIGGER_SPELL. Highest of STR/AGI/INT/SPI picks 60229/60233/60234/60235
--   (sequential strict-> comparisons, default strength on ties).
--   spell_item_mana_drain::HandleProc (:3890-3907): EFFECT_0 PROC_TRIGGER_SPELL.
--   Alive actor casts 29471 on self; alive proc target (GetActionTarget twin)
--   gets 27526 from the actor.
--   Effect-index guards follow the C++ AuraEffectProcFn effect (fireAuraProc dispatches
--   per eligible effect; a mismatched index returns false so the other effect's
--   default leg is not vetoed). Bindings per AC spell_script_names.
--   DORMANT until Spell.dbc ships the proc entries.
--   Handlers are inline anonymous functions (skadi precedent) — the file sits at the
--   200-local main-chunk limit, so no new named locals were added.
local EV6_ITEM = {
    AEGIS_OF_PRESERVATION = 23780,
    AEGIS_HEAL = 23781,
    DESPERATE_DEFENSE = 33896,
    DESPERATE_RAGE = 33898,
    FATE_RUNE = 7434,
    UNSURPASSED_VIGOR = 25733,
    DISCERNING_EYE_BEAST_DUMMY = 59915,
    DISCERNING_EYE_BEAST = 59914,
    HEALING_TOUCH_REFUND = 28847,
    HEALING_TOUCH_MANA = 28848,
    TOTEM_OF_FLOWING_WATER = 28849,
    LESSER_HEALING_WAVE_MANA = 28850,
    SOUL_PRESERVER = 60510,
    SOUL_PRESERVER_BY_CLASS = { [11] = 60512, [2] = 60513, [5] = 60514, [7] = 60515 },
    DEATH_CHOICE_NORMAL = 67702,
    DEATH_CHOICE_HEROIC = 67771,
    DEATH_CHOICE_NORMAL_AGILITY = 67703,
    DEATH_CHOICE_NORMAL_STRENGTH = 67708,
    DEATH_CHOICE_HEROIC_AGILITY = 67772,
    DEATH_CHOICE_HEROIC_STRENGTH = 67773,
    DARKMOON_CARD_GREATNESS = 57345,
    DARKMOON_CARD_STRENGTH = 60229,
    DARKMOON_CARD_AGILITY = 60233,
    DARKMOON_CARD_INTELLECT = 60234,
    DARKMOON_CARD_SPIRIT = 60235,
    MANA_DRAIN = 27522,
    MANA_DRAIN_2 = 40336,
    MANA_DRAIN_ENERGIZE = 29471,
    MANA_DRAIN_LEECH = 27526,
    STAT_STRENGTH = 0,
    STAT_AGILITY = 1,
    STAT_INTELLECT = 3,
    STAT_SPIRIT = 4,
    MARK_OF_CONQUEST = 33510,
    MARK_OF_CONQUEST_ENERGIZE = 39599,
    PROC_FLAG_DONE_RANGED_AUTO_ATTACK = 0x40,
    PROC_FLAG_DONE_SPELL_RANGED_DMG_CLASS = 0x100,
    SHARD_OF_THE_SCALE_PURIFIED = 69755,
    SHARD_OF_THE_SCALE_SHINY = 69739,
    PURIFIED_CAUTERIZING_HEAL = 69733,
    PURIFIED_SEARING_FLAMES = 69729,
    SHINY_CAUTERIZING_HEAL = 69734,
    SHINY_SEARING_FLAMES = 69730,
    PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_POS = 0x4000,
    PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_NEG = 0x10000,
    ZANDALARIAN_UNSTABLE_POWER = 24658,
    ZANDALARIAN_RESTLESS_STRENGTH = 24661,
    UNSTABLE_POWER_STACK = 24659,
    RESTLESS_STRENGTH_STACK = 24662,
    FROZEN_SHADOWEAVE = 39372,
    SHADOWMEND = 39373,
    NECROTIC_TOUCH = 71875,
    NECROTIC_TOUCH_2 = 71877,
    NECROTIC_TOUCH_PROC = 71879,
    SWIFT_HAND_OF_JUSTICE = 59906,
    SWIFT_HAND_HEAL = 59913,
    BLESSING_OF_ANCIENT_KINGS = 64411,
    PROTECTION_OF_ANCIENT_KINGS = 64413,
    PROTECTION_CAP = 20000,
    PERSISTENT_SHIELD = 26467,
    PERSISTENT_SHIELD_TRIGGERED = 26470,
    SHADOWMOURNE = 71903,
    SHADOWMOURNE_SOUL_FRAGMENT = 71905,
    SHADOWMOURNE_CHAOS_BANE_DAMAGE = 71904,
    SHADOWMOURNE_CHAOS_BANE_BUFF = 73422,
    SHADOWS_FATE = 71169,
    UNSATED_CRAVING = 71168,
    SOUL_FEAST = 71203,
    NPC_SINDRAGOSA = 36853,
    AURA_OF_MADNESS = 39446,
    AURA_OF_MADNESS_BY_CLASS = {
        [1] = { 39511, 40997, 40998, 41002, 41005, 41011 },
        [2] = { 39511, 40997, 40998, 40999, 41002, 41005, 41009, 41011, 41404 },
        [3] = { 40997, 40999, 41002, 41005, 41009, 41011, 41404 },
        [4] = { 39511, 40997, 40998, 41002, 41005, 41011 },
        [5] = { 40999, 41002, 41005, 41009, 41011, 41404 },
        [6] = { 39511, 40997, 40998, 41002, 41005, 41011 },
        [7] = { 40999, 41002, 41005, 41009, 41011, 41404 },
        [8] = { 40999, 41002, 41005, 41009, 41011, 41404 },
        [9] = { 40999, 41002, 41005, 41009, 41011, 41404 },
        [11] = { 39511, 40997, 40998, 40999, 41002, 41005, 41009, 41011, 41404 },
    },
    VALANYR = 64415,
    CRYSTAL_SPIRE = 40971,
    DEADLY_PRECISION = 71564,
    PET_HEALING = 37381,
    PET_HEALING_LINK = 37382,
}

RegisterAuraEvent(EV6_ITEM.AEGIS_OF_PRESERVATION, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 1 or target == nil then
        return effectIndex == 1
    end
    target:CastSpell(target, EV6_ITEM.AEGIS_HEAL, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.DESPERATE_DEFENSE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 2 or target == nil then
        return effectIndex == 2
    end
    target:CastSpell(target, EV6_ITEM.DESPERATE_RAGE, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.FATE_RUNE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    target:CastSpell(target, EV6_ITEM.UNSURPASSED_VIGOR, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.DISCERNING_EYE_BEAST_DUMMY, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    actor:CastSpell(actor, EV6_ITEM.DISCERNING_EYE_BEAST, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.HEALING_TOUCH_REFUND, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    actor:CastSpell(actor, EV6_ITEM.HEALING_TOUCH_MANA, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.TOTEM_OF_FLOWING_WATER, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    actor:CastSpell(actor, EV6_ITEM.LESSER_HEALING_WAVE_MANA, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.SOUL_PRESERVER, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    local spellId = EV6_ITEM.SOUL_PRESERVER_BY_CLASS[actor:GetClass()]
    if spellId ~= nil then
        actor:CastSpell(actor, spellId, true)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.DEATH_CHOICE_NORMAL, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    if actor:GetStat(EV6_ITEM.STAT_STRENGTH) > actor:GetStat(EV6_ITEM.STAT_AGILITY) then
        actor:CastSpell(actor, EV6_ITEM.DEATH_CHOICE_NORMAL_STRENGTH, true)
    else
        actor:CastSpell(actor, EV6_ITEM.DEATH_CHOICE_NORMAL_AGILITY, true)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.DEATH_CHOICE_HEROIC, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    if actor:GetStat(EV6_ITEM.STAT_STRENGTH) > actor:GetStat(EV6_ITEM.STAT_AGILITY) then
        actor:CastSpell(actor, EV6_ITEM.DEATH_CHOICE_HEROIC_STRENGTH, true)
    else
        actor:CastSpell(actor, EV6_ITEM.DEATH_CHOICE_HEROIC_AGILITY, true)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.DARKMOON_CARD_GREATNESS, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    local str = actor:GetStat(EV6_ITEM.STAT_STRENGTH)
    local agi = actor:GetStat(EV6_ITEM.STAT_AGILITY)
    local intl = actor:GetStat(EV6_ITEM.STAT_INTELLECT)
    local spi = actor:GetStat(EV6_ITEM.STAT_SPIRIT)
    local stat = 0
    local spellId = EV6_ITEM.DARKMOON_CARD_STRENGTH
    if str > stat then
        spellId = EV6_ITEM.DARKMOON_CARD_STRENGTH
        stat = str
    end
    if agi > stat then
        spellId = EV6_ITEM.DARKMOON_CARD_AGILITY
        stat = agi
    end
    if intl > stat then
        spellId = EV6_ITEM.DARKMOON_CARD_INTELLECT
        stat = intl
    end
    if spi > stat then
        spellId = EV6_ITEM.DARKMOON_CARD_SPIRIT
        stat = spi
    end
    actor:CastSpell(actor, spellId, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.MANA_DRAIN, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    if actor:IsAlive() then
        actor:CastSpell(actor, EV6_ITEM.MANA_DRAIN_ENERGIZE, true)
    end
    if procTarget ~= nil and procTarget:IsAlive() then
        actor:CastSpell(procTarget, EV6_ITEM.MANA_DRAIN_LEECH, true)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.MANA_DRAIN_2, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    if actor:IsAlive() then
        actor:CastSpell(actor, EV6_ITEM.MANA_DRAIN_ENERGIZE, true)
    end
    if procTarget ~= nil and procTarget:IsAlive() then
        actor:CastSpell(procTarget, EV6_ITEM.MANA_DRAIN_LEECH, true)
    end
    return true
end)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6; handler returns true = PreventDefaultAction):
--   spell_item_mark_of_conquest::HandleProc (spell_item.cpp:1283-1291): EFFECT_0
--   PROC_TRIGGER_SPELL. Ranged typeMask (0x40 | 0x100, SpellMgr.h:126/:129)
--   vetoes the default leg and the actor self-casts 39599; any other mask
--   returns false so the default leg fires.
--   spell_item_shard_of_the_scale::HandleProc (:3448-3460): EFFECT_0 DUMMY.
--   POS magic mask (0x4000) casts the heal proc on the proc target, NEG
--   (0x10000) the damage proc; purified 69755 -> 69733/69729, shiny
--   69739 -> 69734/69730.
--   spell_item_zandalarian_charm::HandleStackDrop (:4117-4121) + CheckProc
--   (:4111-4116): EFFECT_0 DUMMY. Gate: the event spell exists and is not
--   the aura itself (a no-spell event fails the gate); the holder drops
--   one stack of 24659 (aura 24658) / 24662 (aura 24661).
--   spell_item_frozen_shadoweave::HandleProc (:1002-1012): EFFECT_0 DUMMY.
--   Zero-damage gate; BP0 = pct(damage, aura amount); actor self-casts
--   39373 via CastCustomSpell.
--   spell_item_necrotic_touch::HandleProc (:1389-1399) + CheckProc
--   (:1383-1386): EFFECT_0 DUMMY. Gate: proc target alive; zero-damage
--   gate; BP0 = pct(damage, aura amount); holder casts 71879 on the proc
--   target. Registered for both auras 71875 and 71877.
--   spell_item_swift_hand_justice_dummy::HandleProc (:2195-2204): EFFECT_0
--   DUMMY. BP0 = CountPctFromMaxHealth(aura amount); actor self-casts
--   59913 via CastCustomSpell.
--   spell_item_blessing_of_ancient_kings::HandleProc (:450-470) + CheckProc
--   (:443-446): EFFECT_0 DUMMY. Gate: proc target exists; zero-heal
--   gate; absorb = 15% of the heal; an existing 64413 on the proc target
--   grows to min(amount + absorb, 20000) with SetDuration(GetMaxDuration)
--   as the RefreshDuration twin, else the holder casts 64413 with BP0.
--   The C++ caster-GUID filter on the 64413 lookup is unmodeled.
--   spell_item_persistent_shield::HandleProc (:1545-1557) + CheckProc
--   (:1540-1543): EFFECT_0 PROC_TRIGGER_SPELL. Gate: a nonzero heal;
--   BP0 = 15% of the heal; a stronger existing 26470 on the proc target
--   skips the manual cast. The C++ handler never calls
--   PreventDefaultAction, so every HandleProc path returns false and the
--   default leg still runs. The C++ caster-GUID filter on the 26470
--   lookup is unmodeled.
--   spell_item_shadowmourne::HandleProc (:2034-2048) + CheckProc
--   (:2027-2032): EFFECT_0 DUMMY. Gate: holder lacks 73422 and the proc
--   target is alive. Holder self-casts 71905; at 10 stacks it casts
--   71904 on the proc target and removes 71905. The AfterEffectRemove
--   leg (:2050-2053) rides as an event-3 handler below.
--   spell_item_shadows_fate::HandleProc (:1981-1991): OnProc, no C++
--   effect index (guarded on 0; the 71169 DBC proc effect is
--   unverified, world.db spell_dbc empty). No CheckProc in C++ — the
--   actor casts 71203 on the aura caster (GetCaster()) on every proc.
--   The player/valid-kill gate belongs to the sibling aura 71168
--   (spell_item_unsated_craving::CheckProc :1952-1962, bound by AC +
--   TDB to 71168, not 71169); it rides as its own ev6 handler below,
--   vetoing 71168's default DBC proc leg when the check fails.
--   spell_item_aura_of_madness::HandleProc (:375-395): EFFECT_0 DUMMY.
--   One uniform pick from the class table on the actor; the 10% Say
--   leg is omitted (broadcast_text 21954 is absent from world.db and
--   Say takes literal text).
--   spell_item_valanyr_hammer_of_ancient_kings::CheckProc (:484-487):
--   CheckProc-only, inverted veto (runtime.go:524): effective heal > 0
--   lets the default leg fire, else veto. Proc effect assumed eff0.
--   spell_item_crystal_spire_of_karabor::CheckProc (:1197-1208):
--   CheckProc-only, inverted veto: passes when the heal target's
--   health minus the effective heal is at or below
--   CountPctFromMaxHealth of the aura's eff0 base amount. Proc effect
--   assumed eff0.
--   spell_item_deadly_precision::HandleStackDrop (:507-511): EFFECT_0
--   MOD_RATING. Holder drops one stack of 71564 (the player bridge
--   takes spellId only; the C++ caster-GUID arg is unmodeled and a
--   float64 GUID round-trips lossy).
--   spell_item_pet_healing::HandleProc (:1577-1587): EFFECT_0 DUMMY.
--   Zero-damage gate; BP0 = pct(damage, aura amount); the actor casts
--   37382 on the player's pet via CastCustomSpell (player:GetPet,
--   lua_player.go:1865; nil pet skips). Hunter/Warlock T5 2P bonus.
--   The C++ nullptr-target cast relies on 37382's implicit pet target;
--   the explicit pet target is the Lua modeling of it.
--   NOT PORTED: deathbringers will 71519/71562 is Go-covered
--   (procs.go:4311 leg + deathbringersWillForms; a Lua twin would
--   double-fire). Deadly precision dummy 71563 is an
--   ev4 SpellScript, out of this ev6 strip.
--   Bindings per C++ comments; world.db spell_script_names, spell_dbc
--   and broadcast_text are all empty (0 rows), so DBC-side effect
--   indices, trigger spells and implicit targets are unverified.
--   Handlers are inline anonymous functions (skadi precedent) — no new
--   top-level locals.
RegisterAuraEvent(EV6_ITEM.MARK_OF_CONQUEST, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    if bit32.band(procEvent:GetTypeMask(), bit32.bor(EV6_ITEM.PROC_FLAG_DONE_RANGED_AUTO_ATTACK, EV6_ITEM.PROC_FLAG_DONE_SPELL_RANGED_DMG_CLASS)) ~= 0 then
        actor:CastSpell(actor, EV6_ITEM.MARK_OF_CONQUEST_ENERGIZE, true)
        return true
    end
    return false
end)

RegisterAuraEvent(EV6_ITEM.SHARD_OF_THE_SCALE_PURIFIED, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil or procTarget == nil then
        return effectIndex == 0
    end
    local typeMask = procEvent:GetTypeMask()
    if bit32.band(typeMask, EV6_ITEM.PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_POS) ~= 0 then
        actor:CastSpell(procTarget, EV6_ITEM.PURIFIED_CAUTERIZING_HEAL, true)
    end
    if bit32.band(typeMask, EV6_ITEM.PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_NEG) ~= 0 then
        actor:CastSpell(procTarget, EV6_ITEM.PURIFIED_SEARING_FLAMES, true)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.SHARD_OF_THE_SCALE_SHINY, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil or procTarget == nil then
        return effectIndex == 0
    end
    local typeMask = procEvent:GetTypeMask()
    if bit32.band(typeMask, EV6_ITEM.PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_POS) ~= 0 then
        actor:CastSpell(procTarget, EV6_ITEM.SHINY_CAUTERIZING_HEAL, true)
    end
    if bit32.band(typeMask, EV6_ITEM.PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_NEG) ~= 0 then
        actor:CastSpell(procTarget, EV6_ITEM.SHINY_SEARING_FLAMES, true)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.ZANDALARIAN_UNSTABLE_POWER, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    local spellId = procEvent:GetSpellId()
    if spellId == 0 or spellId == EV6_ITEM.ZANDALARIAN_UNSTABLE_POWER then
        return true
    end
    target:RemoveAuraFromStack(EV6_ITEM.UNSTABLE_POWER_STACK)
    return true
end)

RegisterAuraEvent(EV6_ITEM.ZANDALARIAN_RESTLESS_STRENGTH, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    local spellId = procEvent:GetSpellId()
    if spellId == 0 or spellId == EV6_ITEM.ZANDALARIAN_RESTLESS_STRENGTH then
        return true
    end
    target:RemoveAuraFromStack(EV6_ITEM.RESTLESS_STRENGTH_STACK)
    return true
end)

RegisterAuraEvent(EV6_ITEM.FROZEN_SHADOWEAVE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    local dealt = procEvent:GetDamage()
    if dealt == nil or dealt == 0 then
        return true
    end
    actor:CastCustomSpell(actor, EV6_ITEM.SHADOWMEND, true, math.floor(dealt * aura:GetAmount() / 100))
    return true
end)

RegisterAuraEvent(EV6_ITEM.NECROTIC_TOUCH, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    if procTarget == nil or not procTarget:IsAlive() then
        return true
    end
    local dealt = procEvent:GetDamage()
    if dealt == nil or dealt == 0 then
        return true
    end
    target:CastCustomSpell(procTarget, EV6_ITEM.NECROTIC_TOUCH_PROC, true, math.floor(dealt * aura:GetAmount() / 100))
    return true
end)

RegisterAuraEvent(EV6_ITEM.NECROTIC_TOUCH_2, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    if procTarget == nil or not procTarget:IsAlive() then
        return true
    end
    local dealt = procEvent:GetDamage()
    if dealt == nil or dealt == 0 then
        return true
    end
    target:CastCustomSpell(procTarget, EV6_ITEM.NECROTIC_TOUCH_PROC, true, math.floor(dealt * aura:GetAmount() / 100))
    return true
end)

RegisterAuraEvent(EV6_ITEM.SWIFT_HAND_OF_JUSTICE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    actor:CastCustomSpell(actor, EV6_ITEM.SWIFT_HAND_HEAL, true, actor:CountPctFromMaxHealth(aura:GetAmount()))
    return true
end)

RegisterAuraEvent(EV6_ITEM.BLESSING_OF_ANCIENT_KINGS, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    if procTarget == nil then
        return true
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    local absorb = math.floor(heal * 15 / 100)
    local prot = procTarget:GetAura(EV6_ITEM.PROTECTION_OF_ANCIENT_KINGS)
    if prot ~= nil then
        prot:SetAmount(math.min(prot:GetAmount() + absorb, EV6_ITEM.PROTECTION_CAP))
        local maxDuration = prot:GetMaxDuration()
        if maxDuration > 0 then
            prot:SetDuration(maxDuration)
        end
    else
        target:CastCustomSpell(procTarget, EV6_ITEM.PROTECTION_OF_ANCIENT_KINGS, true, absorb)
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.PERSISTENT_SHIELD, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil or procTarget == nil then
        return effectIndex == 0
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    local bp0 = math.floor(heal * 15 / 100)
    local existing = procTarget:GetAuraEffect(EV6_ITEM.PERSISTENT_SHIELD_TRIGGERED, 0)
    if existing ~= nil and existing > bp0 then
        return false
    end
    actor:CastCustomSpell(procTarget, EV6_ITEM.PERSISTENT_SHIELD_TRIGGERED, true, bp0)
    return false
end)

RegisterAuraEvent(EV6_ITEM.SHADOWMOURNE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    if target:HasAura(EV6_ITEM.SHADOWMOURNE_CHAOS_BANE_BUFF) then
        return true
    end
    if procTarget == nil or not procTarget:IsAlive() then
        return true
    end
    target:CastSpell(target, EV6_ITEM.SHADOWMOURNE_SOUL_FRAGMENT, true)
    local fragments = target:GetAura(EV6_ITEM.SHADOWMOURNE_SOUL_FRAGMENT)
    if fragments ~= nil and fragments:GetStackAmount() >= 10 then
        target:CastSpell(procTarget, EV6_ITEM.SHADOWMOURNE_CHAOS_BANE_DAMAGE, true)
        fragments:Remove()
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.SHADOWMOURNE, 3, function(event, aura, target)
    if target ~= nil then
        target:RemoveAurasDueToSpell(EV6_ITEM.SHADOWMOURNE_SOUL_FRAGMENT)
    end
end)

RegisterAuraEvent(EV6_ITEM.SHADOWS_FATE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    local feastTarget = aura:GetCaster()
    if feastTarget == nil then
        return true
    end
    actor:CastSpell(feastTarget, EV6_ITEM.SOUL_FEAST, true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.UNSATED_CRAVING, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    if actor:ToPlayer() == nil then
        return true
    end
    local actionTarget = procEvent:GetActionTarget()
    if actionTarget == nil or actionTarget:ToCreature() == nil then
        return true
    end
    if actionTarget:IsCritter() then
        return true
    end
    if actionTarget:GetEntry() ~= EV6_ITEM.NPC_SINDRAGOSA and actionTarget:IsSummon() then
        return true
    end
    return false
end)

RegisterAuraEvent(EV6_ITEM.AURA_OF_MADNESS, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    local spells = EV6_ITEM.AURA_OF_MADNESS_BY_CLASS[actor:GetClass()]
    if spells == nil then
        return true
    end
    actor:CastSpell(actor, spells[math.random(#spells)], true)
    return true
end)

RegisterAuraEvent(EV6_ITEM.VALANYR, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    if procEvent:GetEffectiveHeal() > 0 then
        return false
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.CRYSTAL_SPIRE, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return false
    end
    local effectiveHeal = procEvent:GetEffectiveHeal()
    if procTarget ~= nil and effectiveHeal > 0 and procTarget:GetHealth() - effectiveHeal <= procTarget:CountPctFromMaxHealth(aura:GetBaseAmount(0)) then
        return false
    end
    return true
end)

RegisterAuraEvent(EV6_ITEM.DEADLY_PRECISION, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return effectIndex == 0
    end
    target:RemoveAuraFromStack(EV6_ITEM.DEADLY_PRECISION)
    return true
end)

RegisterAuraEvent(EV6_ITEM.PET_HEALING, 6, function(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or actor == nil then
        return effectIndex == 0
    end
    local player = actor:ToPlayer()
    if player == nil then
        return true
    end
    local pet = player:GetPet()
    if pet == nil then
        return true
    end
    local dealt = procEvent:GetDamage()
    if dealt == nil or dealt == 0 then
        return true
    end
    actor:CastCustomSpell(pet, EV6_ITEM.PET_HEALING_LINK, true, math.floor(dealt * aura:GetAmount() / 100))
    return true
end)

-- EV7/EV9 -- spell_item ev7 (OnObjectAreaTargetSelect) + ev9 (AfterCast) strip.
--   echoes_of_light (spell_item.cpp:852-868): EFFECT_0 TARGET_UNIT_DEST_AREA_ALLY;
--   sort by health pct, keep only the lowest. Bindings 71610/71641.
--   toy_train_set_pulse (:3597-3626): EFFECT_ALL TARGET_UNIT_SRC_AREA_ALLY drops
--   non-players; EFFECT_0 SCRIPT_EFFECT emotes EMOTE_ONESHOT_TRAIN (275) on hit
--   players. The FindTextSoundEmoteFor sound leg stays out (no bridge).
--   Binding 61551 (AC spell_item_toy_train_set; TDB spell_item_toy_train_set_pulse).
--   taunt_flag_targeting (:3921-3950): EFFECT_0 TARGET_CORPSE_SRC_AREA_ENEMY drops
--   non-player/non-corpse, random-resize to 1; EFFECT_0 DUMMY casts 51657 on the
--   hit unit. The FinishCast(SPELL_FAILED_NO_VALID_TARGETS) leg stays out (no
--   bridge); an empty list just fizzles. The TextEmote(28008) leg stays out
--   (string-only TextEmote). Binding 51640 (AC spell_the_flag_of_ownership;
--   TDB spell_item_taunt_flag_targeting). The ev7 list holds units only, so the
--   corpse leg may never see a corpse; the player leg still works.
--   mad_alchemists_potion (:4150-4210) + crazy_alchemists_potion (:4221-4257):
--   AfterCast secondary elixir rolls; the item rides CastCustomSpell's castItem
--   arg with triggered=false (C++ CastSpell(target, chosen, GetCastItem()) is
--   non-triggered; the bridge ignores the triggered arg today). The mad-potion
--   same-group block rides static battle/guardian lists + HasAura — an
--   approximation of IsSpellMemberOfSpellGroup + the applied-aura iteration,
--   both unbridged. Bindings 45051/53750 (AC).
local EV79_ITEM = {
    ECHOES_OF_LIGHT = 71610,
    ECHOES_OF_LIGHT_2 = 71641,
    TOY_TRAIN_SET_PULSE = 61551,
    TAUNT_FLAG_TARGETING = 51640,
    TAUNT_FLAG = 51657,
    MAD_ALCHEMISTS_POTION = 45051,
    CRAZY_ALCHEMISTS_POTION = 53750,
    TYPEID_CORPSE = 7,
    POWER_MANA = 0,
    EMOTE_ONESHOT_TRAIN = 275,
    MAD_BATTLE_ELIXIRS = { 33720, 54452, 33726, 28490, 28491, 28493, 54494, 28501, 28503, 38954 },
    MAD_GUARDIAN_ELIXIRS = { 39625, 39626, 39627, 39628, 28502, 28514 },
    MAD_OTHER_ELIXIRS = { 28489, 28496 },
    ELIXIR_OF_MAJOR_MAGEBLOOD = 28509,
    CRAZY_ELIXIRS = { 43185, 53750, 53761, 53762, 53908, 53909, 53910, 53911, 53913, 53914, 53915 },
    POTION_OF_NIGHTMARES = 53753,
    RUNIC_MANA_POTION = 43186,
}

RegisterSpellEvent(EV79_ITEM.ECHOES_OF_LIGHT, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if list == nil or #list < 2 then
        return
    end
    local lowest = list[1]
    local lowestPct = lowest:GetHealthPct()
    for i = 2, #list do
        local pct = list[i]:GetHealthPct()
        if pct < lowestPct then
            lowest = list[i]
            lowestPct = pct
        end
    end
    for i = 1, #list do
        if list[i] ~= lowest then
            targets:Remove(list[i])
        end
    end
end)
RegisterSpellEvent(EV79_ITEM.ECHOES_OF_LIGHT_2, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if list == nil or #list < 2 then
        return
    end
    local lowest = list[1]
    local lowestPct = lowest:GetHealthPct()
    for i = 2, #list do
        local pct = list[i]:GetHealthPct()
        if pct < lowestPct then
            lowest = list[i]
            lowestPct = pct
        end
    end
    for i = 1, #list do
        if list[i] ~= lowest then
            targets:Remove(list[i])
        end
    end
end)

RegisterSpellEvent(EV79_ITEM.TOY_TRAIN_SET_PULSE, 7, function(event, spell, effectIndex, targets)
    local list = targets:GetTargets()
    if list == nil then
        return
    end
    for i = 1, #list do
        if list[i]:GetTypeId() ~= TYPEID_PLAYER then
            targets:Remove(list[i])
        end
    end
end)
RegisterSpellEvent(EV79_ITEM.TOY_TRAIN_SET_PULSE, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil or hitTarget:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    hitTarget:PerformEmote(EV79_ITEM.EMOTE_ONESHOT_TRAIN)
end)

RegisterSpellEvent(EV79_ITEM.TAUNT_FLAG_TARGETING, 7, function(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if list == nil then
        return
    end
    local kept = {}
    for i = 1, #list do
        local t = list[i]:GetTypeId()
        if t == TYPEID_PLAYER or t == EV79_ITEM.TYPEID_CORPSE then
            kept[#kept + 1] = list[i]
        else
            targets:Remove(list[i])
        end
    end
    if #kept > 1 then
        local pick = kept[math.random(1, #kept)]
        for i = 1, #kept do
            if kept[i] ~= pick then
                targets:Remove(kept[i])
            end
        end
    end
end)
RegisterSpellEvent(EV79_ITEM.TAUNT_FLAG_TARGETING, 5, function(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(hitTarget, EV79_ITEM.TAUNT_FLAG, true)
end)

RegisterSpellEvent(EV79_ITEM.MAD_ALCHEMISTS_POTION, 9, function(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local available = {}
    for i = 1, #EV79_ITEM.MAD_BATTLE_ELIXIRS do
        available[#available + 1] = EV79_ITEM.MAD_BATTLE_ELIXIRS[i]
    end
    for i = 1, #EV79_ITEM.MAD_GUARDIAN_ELIXIRS do
        available[#available + 1] = EV79_ITEM.MAD_GUARDIAN_ELIXIRS[i]
    end
    for i = 1, #EV79_ITEM.MAD_OTHER_ELIXIRS do
        available[#available + 1] = EV79_ITEM.MAD_OTHER_ELIXIRS[i]
    end
    if caster:GetPowerType() == EV79_ITEM.POWER_MANA then
        available[#available + 1] = EV79_ITEM.ELIXIR_OF_MAJOR_MAGEBLOOD
    end
    local chosen = available[math.random(1, #available)]
    local group = nil
    for i = 1, #EV79_ITEM.MAD_BATTLE_ELIXIRS do
        if EV79_ITEM.MAD_BATTLE_ELIXIRS[i] == chosen then
            group = EV79_ITEM.MAD_BATTLE_ELIXIRS
            break
        end
    end
    if group == nil then
        for i = 1, #EV79_ITEM.MAD_GUARDIAN_ELIXIRS do
            if EV79_ITEM.MAD_GUARDIAN_ELIXIRS[i] == chosen then
                group = EV79_ITEM.MAD_GUARDIAN_ELIXIRS
                break
            end
        end
    end
    if group ~= nil then
        for i = 1, #group do
            if group[i] ~= chosen and caster:HasAura(group[i]) then
                return
            end
        end
    end
    caster:CastCustomSpell(caster, chosen, false, nil, nil, nil, spell:GetCastItem())
end)

RegisterSpellEvent(EV79_ITEM.CRAZY_ALCHEMISTS_POTION, 9, function(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local available = {}
    for i = 1, #EV79_ITEM.CRAZY_ELIXIRS do
        available[#available + 1] = EV79_ITEM.CRAZY_ELIXIRS[i]
    end
    if not caster:IsInCombat() then
        available[#available + 1] = EV79_ITEM.POTION_OF_NIGHTMARES
    end
    if caster:GetPowerType() == EV79_ITEM.POWER_MANA then
        available[#available + 1] = EV79_ITEM.RUNIC_MANA_POTION
    end
    caster:CastCustomSpell(caster, available[math.random(1, #available)], false, nil, nil, nil, spell:GetCastItem())
end)
