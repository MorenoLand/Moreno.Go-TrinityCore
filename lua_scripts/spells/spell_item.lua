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
--   67758 -> 67759). The proc leg stays out (no proc hook).
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

-- CENSUS CORRECTIONS (this tick, rows 604-607):
--   604 spell_item_trigger_spell is NOT covered — the census hits
--   are the unrelated generic spell_trigger_spell_from_caster in
--   boss files. The class is a SpellScript template (OnEffectHit
--   DUMMY, spell_item.cpp:42-84) instantiated for arcanite
--   dragonling / gnomish battle chicken / mechanical dragonling /
--   mithril mechanical dragonling; no RegisterSpellScript bridge,
--   so all four STAY OUT.
--   605 spell_item_defibrillate is NOT covered — SpellScript
--   OnEffectHitTarget RESURRECT (spell_item.cpp:712-756), goblin
--   jumper cables 67% / XL 50% fail chance + fail spell; no
--   RegisterSpellScript bridge. STAYS OUT.
--   606 spell_item_sunwell_neck is NOT covered — the ev6 arm itself
--   is expressible (actor-player gate + GetReputationRank 932/934
--   exalted legs casting 45479/45429, 45480/45428, 45432/45431,
--   45478/45430 per neck, spell_item.cpp:3546-3595) but the four
--   aura binding IDs live only in the world DB spell_script_names
--   (script names spell_item_sunwell_exalted_{caster,melee,tank,
--   healer}_neck), unresolvable in-tree. STAYS OUT until the IDs
--   are established; recipe: register ev6 per aura ID once known.
--   607 spell_item_trinket_stack VERIFIED in-tree: the OnRemove leg
--   is the ported handler above (tick 295); the proc leg stays out
--   (no proc hook) as the header already records.
