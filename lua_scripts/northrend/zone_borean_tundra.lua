-- Borean Tundra zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_borean_tundra.cpp.
--
-- PORTED (tick 205, RegisterAuraEvent OnRemove = 3):
--   spell_windsoul_totem_aura (46374, zone_borean_tundra.cpp:1669-1694):
--   EFFECT_0 DUMMY. OnEffectRemove, if the target is dead, the caster
--   casts Windsoul credit (46378) with a nil target, non-triggered.
--   Quest "The Power of the Elements": the Windsoul Totem gathers the
--   energy of slain Steam Ragers.
--   Aura binding is DB-side only: TDB spell_script_names row
--   (46374,'spell_windsoul_totem_aura')
--   (sql/old/3.3.5a/TDB52_to_TDB53_updates/world/
--   2014_03_26_00_world_sai.sql), Wowhead "Windsoul Totem Aura"
--   Effect 0 is Apply Aura: Dummy, 3 sec duration.
-- DELTA: C++ casts with a nullptr target (the credit spell's implicit
-- targeting); Go creature:CastSpell(nil, ...) falls back to the
-- caster's current victim and no-ops without one, so the port casts
-- explicitly on the caster.

local SPELL_WINDSOUL_TOTEM_AURA = 46374 -- TDB spell_script_names
local SPELL_WINDSOUL_CREDIT = 46378 -- zone_borean_tundra.cpp:1666

-- C++ spell_windsoul_totem_aura_AuraScript::OnRemove
-- (zone_borean_tundra.cpp:1678-1684).
local function windsoulTotemAuraRemove(event, aura, target)
    if target:IsDead() then
        local caster = aura:GetCaster()
        if caster ~= nil then
            caster:CastSpell(caster, SPELL_WINDSOUL_CREDIT)
        end
    end
end

RegisterAuraEvent(SPELL_WINDSOUL_TOTEM_AURA, 3, windsoulTotemAuraRemove)
