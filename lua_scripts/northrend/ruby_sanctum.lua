-- Ruby Sanctum spell scripts — Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/ruby_sanctum.cpp
-- (spell_ruby_sanctum_rallying_shout only; the creature AIs live in
-- instance_ruby_sanctum.lua / boss_halion.lua).

-- C++ spell_ruby_sanctum_rallying_shout::CountTargets + HandleDummy
-- (ruby_sanctum.cpp:186-210): OnObjectAreaTargetSelect counts the
-- TARGET_UNIT_SRC_AREA_ALLY list for EFFECT_0; OnEffectHit (DUMMY)
-- casts SPELL_RALLY (75416) on the caster, triggered, with
-- SPELLVALUE_AURA_STACK = that count, only when the count is nonzero
-- and the caster does not already carry the aura. Cast by the
-- Charscale Commander (Ruby Sanctum trash, creature-cast), so both
-- hooks fire on the creature path (ev7 target-select + ev4 effect-hit,
-- runtime.go). The count is stashed per cast keyed by the caster's low
-- GUID: ev7 runs at target selection, ev4 in the HIT phase before the
-- per-target loop, so the stash is always written before it is read;
-- ev4 consumes and clears it. The aura stack rides aura:SetStackAmount
-- after the triggered cast (lua_aura.go:509; the 50240
-- GetAura/SetStackAmount precedent) — the cast applies synchronously
-- on the triggered path, so the aura is present for the read.
local SPELL_RALLYING_SHOUT = 75415 -- ruby_sanctum.cpp:178
local SPELL_RALLY = 75416 -- ruby_sanctum.cpp:53

local rallyTargetCount = {}

local function rallyingShoutCount(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    rallyTargetCount[caster:GetGUIDLow()] = targets:GetCount()
end

local function rallyingShoutHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local low = caster:GetGUIDLow()
    local count = rallyTargetCount[low]
    rallyTargetCount[low] = nil
    if not count or count == 0 or caster:HasAura(SPELL_RALLY) then
        return
    end
    caster:CastSpell(caster, SPELL_RALLY, true)
    local aura = caster:GetAura(SPELL_RALLY)
    if aura ~= nil then
        aura:SetStackAmount(count)
    end
end

RegisterSpellEvent(SPELL_RALLYING_SHOUT, 7, rallyingShoutCount)
RegisterSpellEvent(SPELL_RALLYING_SHOUT, 4, rallyingShoutHit)
