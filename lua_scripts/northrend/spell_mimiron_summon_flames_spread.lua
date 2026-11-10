-- Mimiron flames-spread spell script — Lua port of the
-- FilterTargets leg of spell_mimiron_summon_flames_spread in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2568-2639).
-- One loader covers 64562 Summon Flames Spread Trigger.
-- C++ FilterTargets (EFFECT_0, TARGET_UNIT_SRC_AREA_ENEMY): the
-- flames chase the CLOSEST player — keep exactly the target with
-- the smallest 2D distance to the caster.
-- C++ OnHit (EFFECT_0, SPELL_EFFECT_APPLY_AURA, OnEffectHitTarget):
-- the caster faces the hit unit (SetInFront). STAYS OUT: no
-- SetInFront bridge (grep of engine/world/*.go: zero
-- methods["SetInFront"] hits).
-- C++ HandleTick (EFFECT_0, SPELL_AURA_PERIODIC_TRIGGER_SPELL):
-- ALREADY PORTED in boss_mimiron.lua:682 (RegisterAuraEvent ev1
-- summonFlamesSpreadTick) — NOT re-registered here; a second
-- registration would double-fire the tick.
-- DORMANT caveat (skadi precedent): 64562 is cast only by the
-- flames NPC's UpdateAI (DoCastAOE in npc_mimiron_flames.lua), so
-- the hook fires only when that cast reaches the creature path.

local SPELL_SUMMON_FLAMES_SPREAD_TRIGGER = 64562 -- boss_mimiron.cpp:2568

-- C++ FilterTargets (boss_mimiron.cpp:2581-2595). GetDistance2d is
-- the C++-exact twin (object.go generic fallback); the closest pick
-- runs over the snapshot GetTargets() returns.
local function flamesSpreadFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if #list == 0 then
        return
    end
    local caster = spell:GetCaster()
    local closest = list[1]
    if caster ~= nil then
        for _, target in ipairs(list) do
            if target:GetDistance2d(caster) < closest:GetDistance2d(caster) then
                closest = target
            end
        end
    end
    targets:Clear()
    targets:Add(closest)
end

RegisterSpellEvent(SPELL_SUMMON_FLAMES_SPREAD_TRIGGER, 7, flamesSpreadFilter)
