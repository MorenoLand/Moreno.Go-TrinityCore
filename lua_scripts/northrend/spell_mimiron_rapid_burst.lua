-- Mimiron rapid-burst aura script — Lua port of
-- spell_mimiron_rapid_burst in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2192-2231).
-- C++ HandleDummyTick (EFFECT_1, SPELL_AURA_PERIODIC_DUMMY,
-- OnEffectPeriodic): the caster casts Rapid Burst Left 63387 or Right
-- 64019 on the aura target, alternating by tick number parity (even
-- tick -> Right, odd -> Left), with the aura effect as trigger.
-- PORTED via RegisterAuraEvent ev1 (the periodic hook fires once per
-- aura tick, lua_aura.go:711; GetTickNumber / GetCaster are the aura
-- object methods). The aurEff trigger is the triggered-cast twin
-- (CastSpell(target, spell, true), the magnetic-core precedent).
-- AfterRemove (EFFECT_0, AfterEffectRemove) is PORTED via ev3: the
-- TempSummon gate is IsSummon() (lua_creature_events.go:922) and
-- Despawn() is the DespawnOrUnsummon twin (:3088) — the same pair the
-- magnetic-core OnRemoveSelf leg ported this tick
-- (boss_aerial_command_unit.lua). The Go ev3 hook fires once per
-- removal, matching the single C++ AfterRemove arm.
-- DORMANT caveat: 63382 is cast by VX-001 (DBC-driven), so the hooks
-- fire only when the aura reaches the creature tick path.

local SPELL_RAPID_BURST = 63382 -- boss_mimiron.cpp:2192
local SPELL_RAPID_BURST_LEFT = 63387 -- boss_mimiron.cpp:117
local SPELL_RAPID_BURST_RIGHT = 64019 -- boss_mimiron.cpp:118

-- C++ HandleDummyTick (boss_mimiron.cpp:2214-2218).
local function rapidBurstTick(event, aura, target)
    if target == nil then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if aura:GetTickNumber() % 2 == 0 then
        caster:CastSpell(target, SPELL_RAPID_BURST_RIGHT, true)
    else
        caster:CastSpell(target, SPELL_RAPID_BURST_LEFT, true)
    end
end

-- C++ AfterRemove (boss_mimiron.cpp:2209-2213): despawn the target
-- when it is a TempSummon; the IsSummon() gate is the ToTempSummon()
-- null check, and a non-summon target no-ops like C++.
local function rapidBurstRemove(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    if target:IsSummon() then
        target:Despawn()
    end
end

RegisterAuraEvent(SPELL_RAPID_BURST, 1, rapidBurstTick)
RegisterAuraEvent(SPELL_RAPID_BURST, 3, rapidBurstRemove)
