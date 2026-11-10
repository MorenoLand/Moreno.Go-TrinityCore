-- Mimiron napalm-shell spell script — Lua port of
-- spell_mimiron_napalm_shell in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1968-2002).
-- One loader covers 63667 Napalm Shell.
-- C++ FilterTargets (EFFECT_0, TARGET_UNIT_SRC_AREA_ENEMY): pick a
-- random target; drop every target within 15yd of the caster
-- (Trinity::AllWorldObjectsInRange); if any target survives, re-pick
-- randomly among the survivors; keep exactly one target. When every
-- target is in range the list empties and the ORIGINAL pick is kept.
-- C++ HandleScript (EFFECT_0, SPELL_EFFECT_SCRIPT_EFFECT,
-- OnEffectHitTarget): the caster casts Napalm Shell 63666 on the hit
-- unit, NOT triggered (no TRIGGERED flag in C++).
-- Validate (SPELL_NAPALM_SHELL DBC check) has no Lua twin.
-- DORMANT caveat (skadi precedent): no Spell.dbc ships in-tree, and
-- 63667 has no cast site in the C++ scripts tree — Leviathan MK II
-- casts 64539 Force Cast Napalm Shell and the 64539 -> 63667 link
-- lives in DBC — so the casts may not reach these hooks at all until
-- the DBC data lands; the registrations themselves are exact.

local SPELL_NAPALM_SHELL_TRIGGER = 63667 -- boss_mimiron.cpp:1968
local SPELL_NAPALM_SHELL = 63666 -- boss_mimiron.cpp:88

-- C++ FilterTargets (boss_mimiron.cpp:1980-1993). PORTED via
-- RegisterSpellEvent ev7 (the skadi freezing-cloud precedent).
-- AllWorldObjectsInRange is IsWithinDist(go, 15, false)
-- (GridNotifiers.h:1523-1531): 2D, strict, with both combat reaches
-- added to the range (_IsWithinDist, Object.cpp:1147-1166; IsInDist2d
-- is raw-2D < dist, Position.h:144). The twin is GetDistance2d < 15 +
-- caster reach + 1.5 player reach (the engine's own player-reach
-- constant, lua_creature_events.go:6450; player_state.go:2347).
-- The in-range removal runs over the snapshot GetTargets() returns,
-- so removing inside the loop is safe.
local function napalmShellFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if #list == 0 then
        return
    end
    local picked = list[math.random(#list)]
    local caster = spell:GetCaster()
    if caster ~= nil then
        for _, target in ipairs(list) do
            if target:GetDistance2d(caster) < 15.0 + caster:GetCombatReach() + 1.5 then
                targets:Remove(target)
            end
        end
        local rest = targets:GetTargets()
        if #rest > 0 then
            picked = rest[math.random(#rest)]
        end
    end
    targets:Clear()
    targets:Add(picked)
end

-- C++ HandleScript (boss_mimiron.cpp:1995-1998). The ev5 handler
-- fires once per (effect, live hit target) on the creature path and
-- the triggered funnel (boss_ai.go:3406-3421, spells.go:27398); the
-- player path has no per-effect list site, so a player-cast
-- napalm-shell stays blocked there.
local function napalmShellScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(hitTarget, SPELL_NAPALM_SHELL)
end

RegisterSpellEvent(SPELL_NAPALM_SHELL_TRIGGER, 7, napalmShellFilter)
RegisterSpellEvent(SPELL_NAPALM_SHELL_TRIGGER, 5, napalmShellScript)
