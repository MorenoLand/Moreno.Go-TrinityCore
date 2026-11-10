-- Mimiron fire-search spell script — Lua port of
-- spell_mimiron_fire_search in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1799-1887).
-- One loader covers 64618 Fire Search.
-- C++ FilterTargets (EFFECT_0, TARGET_UNIT_SRC_AREA_ENTRY): pick one
-- random target from the list; _noTarget is set when the list is empty.
-- The _noTarget bookkeeping stays out: its only reader is
-- HandleAftercast, which has no bridge (CallScriptAfterCastHandlers
-- is a no-op, engine/world/spells.go:19213 — no AfterCast spell event).
-- C++ HandleScript (EFFECT_0, SPELL_EFFECT_SCRIPT_EFFECT,
-- OnEffectHitTarget): if the caster is within 15yd (2D) of the hit
-- unit and its AI reports DATA_WATERSPRAY, the caster casts Water
-- Spray 64619 triggered on the hit unit, clears DATA_WATERSPRAY and
-- sets DATA_MOVE_NEW; otherwise, if DATA_MOVE_NEW is set, the caster
-- chases the hit unit and clears DATA_MOVE_NEW.
-- The caster is the emergency fire bot (entry 34147, ulduar.h:125);
-- its AI is ported in npc_mimiron_emergency_fire_bot.lua, which owns
-- the DATA_WATERSPRAY / DATA_MOVE_NEW ScriptData the arms below read
-- (creature GetData/SetData, lua_creature_events.go:792-826).
-- Validate (SPELL_WATER_SPRAY DBC check) has no Lua twin.
-- DORMANT caveat (skadi precedent): no Spell.dbc ships in-tree, and
-- the cast side is the 64617 Fire Search aura periodic trigger from
-- creature_template_addon, so the casts may not reach these hooks at
-- all until the DBC data lands; the registrations themselves are exact.

local SPELL_FIRE_SEARCH = 64618 -- boss_mimiron.cpp:152
local SPELL_WATER_SPRAY = 64619 -- boss_mimiron.cpp:153

local DATA_WATERSPRAY = 5 -- boss_mimiron.cpp:191
local DATA_MOVE_NEW = 6 -- boss_mimiron.cpp:192

-- C++ FilterTargets (boss_mimiron.cpp:1822-1830): _noTarget =
-- targets.empty(); if not empty, keep one random target.
-- PORTED via RegisterSpellEvent ev7 (the skadi freezing-cloud
-- precedent: targets:GetTargets() / targets:Clear() / targets:Add).
local function fireSearchFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    local list = targets:GetTargets()
    if #list == 0 then
        return
    end
    local picked = list[math.random(#list)]
    targets:Clear()
    targets:Add(picked)
end

-- C++ HandleScript (boss_mimiron.cpp:1839-1858). The ev5 handler
-- fires once per (effect, live hit target) on the creature path and
-- the triggered funnel (boss_ai.go:3406-3421, spells.go:27398); the
-- player path has no per-effect list site, so a player-cast
-- fire-search stays blocked there. GetDistance2d is the generic
-- object fallback (engine/scripting/object.go:282); MoveChase and
-- CastSpell(triggered) are the boss_vx_001 / skadi precedents.
local function fireSearchScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if caster:GetDistance2d(hitTarget) <= 15.0 and caster:GetData(DATA_WATERSPRAY) ~= 0 then
        caster:CastSpell(hitTarget, SPELL_WATER_SPRAY, true)
        caster:SetData(DATA_WATERSPRAY, 0)
        caster:SetData(DATA_MOVE_NEW, 1)
    elseif caster:GetData(DATA_MOVE_NEW) ~= 0 then
        caster:MoveChase(hitTarget)
        caster:SetData(DATA_MOVE_NEW, 0)
    end
end

RegisterSpellEvent(SPELL_FIRE_SEARCH, 7, fireSearchFilter)
RegisterSpellEvent(SPELL_FIRE_SEARCH, 5, fireSearchScript)
