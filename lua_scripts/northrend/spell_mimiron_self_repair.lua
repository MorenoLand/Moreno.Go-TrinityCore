-- Mimiron self-repair spell script — Lua port of
-- spell_mimiron_self_repair in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2377-2404).
-- C++ HandleScript (AfterHit): the caster's AI takes DO_ASSEMBLED_COMBAT.
-- PORTED via RegisterSpellEvent ev3 (boss_ai.go:4170 fires AfterHit on
-- the creature path). DO_ASSEMBLED_COMBAT is 12 (the
-- boss_aerial_command_unit.lua / boss_leviathan_mk_ii.lua precedent);
-- creature:DoAction fires the entry's CREATURE_EVENT_ON_DO_ACTION (38)
-- hook, which the three part AIs handle. DORMANT caveat: 64383 is
-- DoCast(me) from the part AIs, so the hook fires only when the cast
-- reaches the creature path.

local SPELL_SELF_REPAIR = 64383 -- boss_mimiron.cpp:2377
local DO_ASSEMBLED_COMBAT = 12 -- boss_mimiron.cpp:298

-- C++ HandleScript (boss_mimiron.cpp:2387-2391).
local function selfRepairHit(event, spell, hitTarget)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:DoAction(DO_ASSEMBLED_COMBAT)
end

RegisterSpellEvent(SPELL_SELF_REPAIR, 3, selfRepairHit)
