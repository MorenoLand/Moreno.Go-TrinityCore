-- Holiday spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_holiday.cpp.
--
-- PORTED (tick 197, RegisterAuraEvent OnRemove = 3):
--   spell_brewfest_exhausted_ram (43332, spell_holiday.cpp:1294-1300):
--   EFFECT_0 MOD_DECREASE_SPEED. OnRemove the target casts Ram
--   Level Neutral (43310) on itself, triggered. No Load gates in
--   C++; no PreventDefaultAction. (C++ registers this arm with the
--   AuraEffectApplyFn type — a TrinityCore quirk, no behavior, same
--   as the direbrew disarm apply arm.)
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the rider's triggered
-- self-cast lands non-triggered; pre-existing bridge gap, same as
-- all prior player-target ports.

local SPELL_BREWFEST_EXHAUSTED_RAM = 43332 -- spell_holiday.cpp:1294
local SPELL_RAM_LEVEL_NEUTRAL = 43310 -- spell_holiday.cpp:1072

-- C++ spell_brewfest_exhausted_ram::OnRemove (spell_holiday.cpp:1297-1300).
local function exhaustedRamRemove(event, aura, target)
    target:CastSpell(target, SPELL_RAM_LEVEL_NEUTRAL, true)
end

RegisterAuraEvent(SPELL_BREWFEST_EXHAUSTED_RAM, 3, exhaustedRamRemove)
