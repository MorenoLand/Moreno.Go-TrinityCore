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
--   drops. The proc leg stays out (no proc hook).
--   spell_item_anger_capacitor::OnRemove (spell_item.cpp:260-263):
--   EFFECT_0 DUMMY, REAL. On remove the Mote of Anger aura 71432
--   drops. Registered for both ranks: 71406 (8-stack) and 71545
--   (heroic, 7-stack). The proc leg stays out (no proc hook).
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
local SPELL_SHADOWMOURNE = 71903 -- spell_item.cpp:2007 (class spell_item_shadowmourne)
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

local function shadowmourneRemove(event, aura, target)
    target:RemoveAura(SPELL_SHADOWMOURNE_SOUL_FRAGMENT)
end

RegisterAuraEvent(SPELL_SHADOWMOURNE, 3, shadowmourneRemove)

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
