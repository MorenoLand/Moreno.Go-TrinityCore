-- Mimiron frost-bomb spell script — Lua port of
-- spell_mimiron_summon_frost_bomb_target in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2640-2687).
-- One loader covers 64623 Frost Bomb.
-- C++ FilterTargets (EFFECT_0, TARGET_UNIT_SRC_AREA_ENTRY): drop
-- every target within 15yd of the caster
-- (Trinity::AllWorldObjectsInRange); if the list empties, return
-- with no target (the cast fizzles — NOT the napalm-shell
-- original-pick fallback); otherwise pick one survivor at random
-- and keep exactly it.
-- C++ HandleScript (EFFECT_0, SPELL_EFFECT_SCRIPT_EFFECT,
-- OnEffectHitTarget): the caster casts Summon Frost Bomb 64627 on
-- the hit unit, triggered.
-- PORTED via RegisterSpellEvent ev7 + ev5 (napalm-shell precedent).
-- DORMANT caveat (skadi precedent): 64623 is cast only by VX-001
-- (DoCastAOE in boss_vx_001.lua), so the hooks fire only when that
-- cast reaches the creature path.

local SPELL_FROST_BOMB_TRIGGER = 64623 -- boss_mimiron.cpp:2640
local SPELL_SUMMON_FROST_BOMB = 64627 -- boss_mimiron.cpp:142

-- C++ FilterTargets (boss_mimiron.cpp:2655-2668). AllWorldObjectsInRange
-- is IsWithinDist(go, 15, false) (GridNotifiers.h:1523-1531): 2D, strict,
-- with both combat reaches added to the range (_IsWithinDist,
-- Object.cpp:1147-1166; IsInDist2d is raw-2D < dist, Position.h:144).
-- The twin is GetDistance2d < 15 + caster reach + target reach
-- (GetCombatReach is creature-bridged; the 64623 targets are entry
-- creatures). The in-range removal runs over the snapshot GetTargets()
-- returns, so removing inside the loop is safe.
local function frostBombTargetFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if #list == 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster ~= nil then
        for _, target in ipairs(list) do
            if target:GetDistance2d(caster) < 15.0 + caster:GetCombatReach() + target:GetCombatReach() then
                targets:Remove(target)
            end
        end
    end
    local rest = targets:GetTargets()
    if #rest == 0 then
        return
    end
    local picked = rest[math.random(#rest)]
    targets:Clear()
    targets:Add(picked)
end

-- C++ HandleScript (boss_mimiron.cpp:2670-2673).
local function frostBombTargetScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(hitTarget, SPELL_SUMMON_FROST_BOMB, true)
end

RegisterSpellEvent(SPELL_FROST_BOMB_TRIGGER, 7, frostBombTargetFilter)
RegisterSpellEvent(SPELL_FROST_BOMB_TRIGGER, 5, frostBombTargetScript)
