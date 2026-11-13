-- Shadow Labyrinth spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Outland/Auchindoun/ShadowLabyrinth/shadow_labyrinth.cpp.
--
-- PORTED (RegisterAuraEvent OnProc = 6):
--   spell_mark_of_malice (shadow_labyrinth.cpp:29-64): OnEffectProc,
--   EFFECT_0 SPELL_AURA_DUMMY. PreventDefaultAction on every path;
--   charges > 1 drops charges only; else the target self-casts the
--   triggered spell 33494. DORMANT: no Spell.dbc record for the aura
--   spell yet, and the Go proc system needs the aura's DBC proc
--   entry. Aura spell id 33493 confirmed in TDB spell_script_names
--   (ac_spell_script_names.sql:759: (33493,'spell_mark_of_malice')).
--   KNOWN LIMIT: the holder self-cast rides player:CastSpell, which
--   ignores the triggered flag (lua_player.go:2577 -> castSpellDirect),
--   same as all prior player-target ports.

local SPELL_MARK_OF_MALICE = 33493 -- TDB spell_script_names binding for spell_mark_of_malice; C++ names only the triggered spell 33494
local SPELL_MARK_OF_MALICE_TRIGGERED = 33494 -- shadow_labyrinth.cpp:26

-- C++ spell_mark_of_malice::HandleProc (shadow_labyrinth.cpp:44-52).
-- Charge-ordering: C++ decrements proc charges before the handler runs
-- (Aura::PrepareProcToTrigger, SpellAuras.cpp:2031), and the Go OnProc
-- hook likewise fires after charge consumption (runtime.go:490), so
-- GetCharges() reads the same post-decrement value C++ sees: the 33494
-- cast fires on the last TWO procs (post-decrement charges 1, then 0),
-- not just the last.
local function markOfMaliceProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 then
        return true
    end
    if aura:GetCharges() > 1 then
        return true -- charges > 1: drop charges only, default still prevented (shadow_labyrinth.cpp:47-49)
    end
    target:CastSpell(target, SPELL_MARK_OF_MALICE_TRIGGERED, true)
    return true -- PreventDefaultAction on every path (shadow_labyrinth.cpp:46)
end

RegisterAuraEvent(SPELL_MARK_OF_MALICE, 6, markOfMaliceProc)
