-- Utgarde Keep zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/UtgardeKeep/UtgardeKeep/utgarde_keep.cpp.
--
-- PORTED (tick 205, RegisterAuraEvent OnRemove = 3):
--   spell_ticking_time_bomb (59686, utgarde_keep.cpp:125-154):
--   EFFECT_0 PERIODIC_DUMMY. OnEffectRemove, if the caster is the
--   target, the target casts Ticking Time Bomb Explode (59687) on
--   itself, triggered. No mode gate in C++.
--   Aura binding is DB-side only: TDB spell_script_names row
--   (59686,'spell_ticking_time_bomb')
--   (sql/old/3.3.5a/TDB49_to_TDB50_updates/world/
--   2012_11_16_00_world_utgarde.sql), Wowhead "Ticking Time Bomb"
--   Effect 0 is Persistent Area Aura: Periodic Dummy, 1 sec interval.
-- DELTA: C++ compares Unit pointers (GetCaster() == GetTarget()); the
-- port compares GUIDs (aura:GetCasterGUID() vs target:GetGUID()).

local SPELL_TICKING_TIME_BOMB = 59686 -- TDB spell_script_names
local SPELL_TICKING_TIME_BOMB_EXPLODE = 59687 -- utgarde_keep.cpp:122

-- C++ spell_ticking_time_bomb_AuraScript::HandleOnEffectRemove
-- (utgarde_keep.cpp:139-144).
local function tickingTimeBombRemove(event, aura, target)
    if aura:GetCasterGUID() ~= 0 and aura:GetCasterGUID() == target:GetGUID() then
        target:CastSpell(target, SPELL_TICKING_TIME_BOMB_EXPLODE, true)
    end
end

RegisterAuraEvent(SPELL_TICKING_TIME_BOMB, 3, tickingTimeBombRemove)
