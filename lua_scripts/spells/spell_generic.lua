-- Generic spell scripts — Lua port of
-- src/server/scripts/Spells/spell_generic.cpp
-- (spell_gen_aura_of_fear only; the rest of the file's scripts are
-- not ported here).

-- C++ spell_gen_aura_of_fear::PeriodicTick (spell_generic.cpp:391-412):
-- 28313 EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
-- PreventDefaultAction; roll_chance_i(GetSpellInfo()->ProcChance)
-- else return; the aura target casts the DBC trigger of EFFECT_0
-- (aura:GetTriggerSpell(0)) triggered on nil. The roll is
-- roll_chance_i-exact: chance > irand(0, 99) (Random.h:59-62), so
-- the gate passes while math.random(0, 99) < ProcChance. Returns
-- true, the PreventDefaultAction twin, so the Go default trigger
-- cast does not also fire.
local SPELL_AURA_OF_FEAR = 28313

local function auraOfFearTick(event, aura, target)
    if math.random(0, 99) >= aura:GetProcChance() then
        return true
    end
    local trigger = aura:GetTriggerSpell(0)
    if trigger ~= 0 then
        target:CastSpell(nil, trigger, true)
    end
    return true
end

RegisterAuraEvent(SPELL_AURA_OF_FEAR, 1, auraOfFearTick)

-- C++ spell_gen_remove_on_health_pct::PeriodicTick
-- (spell_generic.cpp:2955-2969): EFFECT_0 SPELL_AURA_PERIODIC_
-- DAMAGE on 38772 / 43937 / 62331 / 62418 (Grievous Wound,
-- Impale). When the target is healed above the EFFECT_1 CalcValue
-- health percent, the aura is removed and the tick's default damage
-- is suppressed (return true is the PreventDefaultAction twin).
-- The pct guard (nonzero) is the GetSpellEffectCalcValue
-- convention: with no Spell.dbc loaded the read is 0 and the
-- handler no-ops instead of stripping the aura on every tick.
local SPELL_REMOVE_ON_HEALTH_PCT = { 38772, 43937, 62331, 62418 }

local function removeOnHealthPctTick(event, aura, target)
    local pct = aura:GetSpellEffectCalcValue(aura:GetAuraId(), 1)
    if pct ~= 0 and target:HealthAbovePct(pct) then
        aura:Remove()
        return true
    end
end

for _, spellId in ipairs(SPELL_REMOVE_ON_HEALTH_PCT) do
    RegisterAuraEvent(spellId, 1, removeOnHealthPctTick)
end

-- C++ spell_gen_remove_on_full_health_pct::PeriodicTick
-- (spell_generic.cpp:3003-3015): EFFECT_2 SPELL_AURA_PERIODIC_
-- DAMAGE_PERCENT on 70292 / 71316 / 71317 (Glacial Strike). When
-- the target is at full health the aura is removed and the tick's
-- default damage is suppressed (return true).
local SPELL_REMOVE_ON_FULL_HEALTH_PCT = { 70292, 71316, 71317 }

local function removeOnFullHealthPctTick(event, aura, target)
    if target:IsFullHealth() then
        aura:Remove()
        return true
    end
end

for _, spellId in ipairs(SPELL_REMOVE_ON_FULL_HEALTH_PCT) do
    RegisterAuraEvent(spellId, 1, removeOnFullHealthPctTick)
end

-- C++ spell_gen_remove_on_full_health::PeriodicTick
-- (spell_generic.cpp:2950-2979): EFFECT_0 SPELL_AURA_PERIODIC_
-- DAMAGE on 31956 / 38801 / 43093 / 58517 / 59262 (Grievous Wound
-- / Grievous Throw). When the target is at full health the aura is
-- removed and the tick's default damage is suppressed (return true).
-- A single-effect spell is allowed one tick before the check fires
-- (onlyEffect = no EFFECT_1 and no EFFECT_2, read via the new
-- GetSpellEffectIsEffect bridge; with no Spell.dbc loaded both
-- slots read absent, so the first tick is skipped — the safe
-- degraded read).
local SPELL_REMOVE_ON_FULL_HEALTH = { 31956, 38801, 43093, 58517, 59262 }

local function removeOnFullHealthTick(event, aura, target)
    local spellId = aura:GetAuraId()
    local onlyEffect = not aura:GetSpellEffectIsEffect(spellId, 1)
        and not aura:GetSpellEffectIsEffect(spellId, 2)
    if onlyEffect and aura:GetTickNumber() <= 1 then
        return
    end
    if target:IsFullHealth() then
        aura:Remove()
        return true
    end
end

for _, spellId in ipairs(SPELL_REMOVE_ON_FULL_HEALTH) do
    RegisterAuraEvent(spellId, 1, removeOnFullHealthTick)
end

-- C++ spell_gen_choking_vines::HandleChoke
-- (spell_generic.cpp:968-974): EFFECT_1 SPELL_AURA_PERIODIC_
-- DAMAGE on 35244 (Choking Vines). When the aura reaches its DBC
-- max stack count it casts SPELL_CHOKING_WOUND 35247 on the target
-- (nil target = self, triggered) and removes itself. No
-- PreventDefaultAction, so the Go default damage tick still fires.
-- The max-stack guard (nonzero) is the GetSpellStackAmount
-- convention: with no Spell.dbc loaded the read is 0 and the
-- handler no-ops instead of firing the wound on every tick.
local SPELL_CHOKING_VINES = 35244
local SPELL_CHOKING_WOUND = 35247

local function chokingVinesTick(event, aura, target)
    local maxStacks = aura:GetSpellStackAmount(aura:GetAuraId())
    if maxStacks == 0 then
        return
    end
    if aura:GetStackAmount() ~= maxStacks then
        return
    end
    target:CastSpell(nil, SPELL_CHOKING_WOUND, true)
    aura:Remove()
end

RegisterAuraEvent(SPELL_CHOKING_VINES, 1, chokingVinesTick)
