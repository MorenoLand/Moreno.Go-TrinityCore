-- Dustwallow Marsh zone spells -- Lua port of
-- src/server/scripts/Kalimdor/zone_dustwallow_marsh.cpp (spell scripts).
-- spell_ooze_zap (42489) and spell_ooze_zap_channel_end (42485)
-- PORTED below. The census OnObjectAreaTargetSelect hook belongs to
-- spell_energize_aoe in the same file, not to either ooze spell.
-- All ports DORMANT until Spell.dbc ships the rows.

local SPELL_OOZE_ZAP = 42489
local SPELL_OOZE_ZAP_CHANNEL_END = 42485
local SPELL_OOZE_CHANNEL_CREDIT = 42486

-- C++ spell_ooze_zap::HandleDummy (zone_dustwallow_marsh.cpp:122-129,
-- registered OnEffectHitTarget EFFECT_0 SPELL_EFFECT_DUMMY): the
-- caster casts GetEffectValue() triggered on the hit unit. The
-- CheckRequirement arm (OnCheckCast, :112-120) is PORTED below.
-- PreventHitDefaultEffect has no Lua twin (row-319 precedent; the
-- dummy default is inert).
local function oozeZapHit(_, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local value = spell:GetEffectValue(0)
    if value <= 0 then
        return
    end
    caster:CastSpell(target, value, true)
end

RegisterSpellEvent(SPELL_OOZE_ZAP, 5, oozeZapHit)

-- C++ spell_ooze_zap_channel_end::HandleDummy
-- (zone_dustwallow_marsh.cpp:163-168, registered OnEffectHitTarget
-- EFFECT_0 SPELL_EFFECT_DUMMY): the caster player casts the channel
-- credit (42486) triggered on itself, then the hit unit kills itself.
-- Same PreventHitDefaultEffect note as above. C++ calls KillSelf
-- unguarded; the nil-guard here is defensive only.
local function oozeZapChannelEndHit(_, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil then
        local player = caster:ToPlayer()
        if player ~= nil then
            player:CastSpell(player, SPELL_OOZE_CHANNEL_CREDIT, true)
        end
    end
    if target ~= nil then
        target:Kill()
    end
end

RegisterSpellEvent(SPELL_OOZE_ZAP_CHANNEL_END, 5, oozeZapChannelEndHit)

local SPELL_FAILED_BAD_TARGETS = 12 -- SharedDefines.h:994
local SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW = 173 -- SharedDefines.h:1155
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_ooze_zap::CheckRequirement (zone_dustwallow_marsh.cpp:112-120):
-- the caster must carry the aura whose id is the EFFECT_1 value
-- (spell:GetEffectValue(1)) or the cast fails CANT_DO_THAT_RIGHT_NOW;
-- no explicit target fails BAD_TARGETS. RegisterSpellEvent OnCheckCast = 8
-- (lua_spell.go fireSpellCheckCast).
local function oozeZapCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster ~= nil and not caster:HasAura(spell:GetEffectValue(1)) then
        return SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW
    end
    if spell:GetTarget() == nil then
        return SPELL_FAILED_BAD_TARGETS
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_OOZE_ZAP, 8, oozeZapCheckCast)
