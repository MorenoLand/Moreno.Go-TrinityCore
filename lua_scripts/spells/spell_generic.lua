-- Generic spell scripts — Lua port of
-- src/server/scripts/Spells/spell_generic.cpp
-- (spell_gen_aura_of_fear only; the rest of the file's scripts are
-- not ported here).

-- C++ spell_gen_aura_of_fear::PeriodicTick (spell_generic.cpp:391-412):
-- 28313 EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
-- PreventDefaultAction; roll_chance_i(GetSpellInfo()->ProcChance)
-- else return; the aura target casts the DBC trigger of EFFECT_0
-- (aura:GetTriggerSpell(0)) triggered on nil. The roll is
-- roll_chance_i-exact: chance > irand(0, 99) (Random.h:59-62), so
-- the gate passes while math.random(0, 99) < ProcChance. Returns
-- true, the PreventDefaultAction twin, so the Go default trigger
-- cast does not also fire.
local SPELL_AURA_OF_FEAR = 28313

local function auraOfFearTick(event, aura, target)
    if math.random(0, 99) >= aura:GetProcChance() then
        return true
    end
    local trigger = aura:GetTriggerSpell(0)
    if trigger ~= 0 then
        target:CastSpell(nil, trigger, true)
    end
    return true
end

RegisterAuraEvent(SPELL_AURA_OF_FEAR, 1, auraOfFearTick)
