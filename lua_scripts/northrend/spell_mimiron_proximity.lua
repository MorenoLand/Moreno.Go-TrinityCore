-- Mimiron proximity-mine spell scripts — Lua ports of
-- spell_mimiron_proximity_explosion (66351, boss_mimiron.cpp:2082-2116),
-- spell_mimiron_proximity_mines (63027, boss_mimiron.cpp:2118-2150) and
-- spell_mimiron_proximity_trigger (65346, boss_mimiron.cpp:2151-2190).
-- 66351 OnHit (EFFECT_0, SPELL_EFFECT_SCHOOL_DAMAGE, OnEffectHitTarget):
-- a hit player makes the caster's instance fetch Leviathan MK II and
-- SetData(DATA_SETUP_MINE, 0) on its AI. 66351 HandleAura (EFFECT_1,
-- SPELL_EFFECT_APPLY_AURA): the caster drops its proximity-mine
-- periodic-trigger aura. 63027 HandleScript (EFFECT_0,
-- SPELL_EFFECT_SCRIPT_EFFECT): the caster self-casts Summon Proximity
-- Mine 65347 triggered, ten times. 65346 FilterTargets (EFFECT_0,
-- TARGET_UNIT_SRC_AREA_ENTRY): drop the explicit target from the list.
-- 65346 HandleDummy (EFFECT_0, SPELL_EFFECT_DUMMY, OnEffectHit): the
-- caster casts Proximity Mine Explosion 66351 triggered.
-- PORTED via RegisterSpellEvent ev5 (explosion x2, mines), ev7
-- (trigger filter) and ev4 (trigger dummy). GetInstanceCreature +
-- SetData are the boss_leviathan_mk_ii.lua precedents
-- (DATA_LEVIATHAN_MK_II 32, DATA_SETUP_MINE 0). The null-target
-- explosion cast is the DoCastAOE self-cast twin
-- (caster:CastSpell(caster, 66351, true)).
-- GAP: the trigger filter's FinishCast(SPELL_FAILED_NO_VALID_TARGETS)
-- on an emptied list has no bridge, so the ev4 leg still fires when
-- the list empties (the mine alone in range of its own entry); in
-- practice the trigger fires with a player in range, so the list is
-- non-empty. DORMANT caveat: 63027 is DoCastAOE from
-- boss_leviathan_mk_ii.lua; 65346/66351 ride the DBC trigger chain,
-- so the hooks fire only when the casts reach the creature path.

local SPELL_PROXIMITY_MINE_EXPLOSION = 66351 -- boss_mimiron.cpp:2082
local SPELL_PROXIMITY_MINES = 63027 -- boss_mimiron.cpp:2118
local SPELL_PROXIMITY_MINE_TRIGGER = 65346 -- boss_mimiron.cpp:2151
local SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER = 65345 -- boss_mimiron.cpp:112
local SPELL_SUMMON_PROXIMITY_MINE = 65347 -- boss_mimiron.cpp:114
local DATA_LEVIATHAN_MK_II = 32 -- ulduar.h:432
local DATA_SETUP_MINE = 0 -- boss_mimiron.cpp:186

-- C++ OnHit (boss_mimiron.cpp:2092-2099) + HandleAura
-- (boss_mimiron.cpp:2102-2105), one ev5 handler branched on the effect
-- index: the C++ arms share one SpellScript class, and a single handler
-- keeps the per-(effect, hit target) fire count exact.
local function proximityExplosionHit(event, spell, hitTarget, effectIndex)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if effectIndex == 0 then
        if hitTarget == nil or hitTarget:GetObjectType() ~= "Player" then
            return
        end
        local mkii = caster:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
        if mkii ~= nil then
            mkii:SetData(DATA_SETUP_MINE, 0)
        end
    elseif effectIndex == 1 then
        caster:RemoveAura(SPELL_PROXIMITY_MINE_PERIODIC_TRIGGER)
    end
end

-- C++ HandleScript (boss_mimiron.cpp:2133-2138).
local function proximityMinesScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    for _ = 1, 10 do
        caster:CastSpell(caster, SPELL_SUMMON_PROXIMITY_MINE, true)
    end
end

-- C++ FilterTargets (boss_mimiron.cpp:2165-2170): drop the explicit
-- target from the area list. spell:GetTarget() is the wire-target twin
-- of GetExplTargetWorldObject; targets:Remove is nil-safe.
local function proximityTriggerFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local explTarget = spell:GetTarget()
    if explTarget ~= nil then
        targets:Remove(explTarget)
    end
end

-- C++ HandleDummy (boss_mimiron.cpp:2172-2175).
local function proximityTriggerDummy(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, SPELL_PROXIMITY_MINE_EXPLOSION, true)
end

RegisterSpellEvent(SPELL_PROXIMITY_MINE_EXPLOSION, 5, proximityExplosionHit)
RegisterSpellEvent(SPELL_PROXIMITY_MINES, 5, proximityMinesScript)
RegisterSpellEvent(SPELL_PROXIMITY_MINE_TRIGGER, 7, proximityTriggerFilter)
RegisterSpellEvent(SPELL_PROXIMITY_MINE_TRIGGER, 4, proximityTriggerDummy)
