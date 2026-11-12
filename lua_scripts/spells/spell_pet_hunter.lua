-- Hunter pet talent aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Pet/pet_hunter.cpp (census rows 547-549).
-- AddSC_hunter_pet_scripts() (pet_hunter.cpp:346-353) news all three.
--
-- The CheckProc gates ride the inverted ev6 veto (procEventObject,
-- lua_aura.go): the OnProc hook fires after the proc gates and before
-- the default leg, so a handler returns true (PreventDefaultAction)
-- when the C++ CheckProc FAILS, and the DBC default leg is skipped.
-- All three are DORMANT until Spell.dbc ships their proc entries.
--
-- STAY-OUT:
--   spell_pet_guard_dog (53178, pet_hunter.cpp:201-252): the CheckProc
--   (Growl icon 201) is forwardable, but HandleProc adds threat to the
--   proc target (target->GetThreatManager().AddThreat, :237-238) and
--   there is no AddThreat bridge, and the threat amount needs the
--   event spell's EFFECT_0 CalcValue, which the proc-event object
--   does not forward. No partial port: vetoing without the threat leg
--   would drop the talent's whole point.

local SPELL_PET_SILVERBACK = 62764 -- pet_hunter.cpp:254
local SPELL_PET_SILVERBACK_TRIGGER = {62800, 62801} -- pet_hunter.cpp:45-46
local PET_ICON_ID_GROWL = 201 -- pet_hunter.cpp:51

-- C++ spell_pet_silverback::CheckProc + HandleProc (pet_hunter.cpp:255-301):
-- DoCheckProc passes only when the event spell is Growl (SpellIconID
-- 201; Growl shares family flags with other spells, so the icon is the
-- filter). OnEffectProc EFFECT_0 SPELL_AURA_DUMMY: PreventDefaultAction,
-- then the actor casts the rank trigger (62800 R1 / 62801 R2) on itself,
-- triggered via the aurEff overload. The rank is the aura spell's rank
-- (aura:GetRank); a 0 rank (no rank row) vetoes instead of casting a
-- bogus trigger. DORMANT until Spell.dbc ships the 62764 proc entry.
local function silverbackProc(event, aura, target, actor, procTarget, damage, procEvent)
    if actor == nil or aura == nil or procEvent == nil then
        return true
    end
    if procEvent:GetSpellIconId() ~= PET_ICON_ID_GROWL then
        return true
    end
    local trigger = SPELL_PET_SILVERBACK_TRIGGER[aura:GetRank()]
    if trigger == nil then
        return true
    end
    actor:CastSpell(actor, trigger, true)
    return true
end

RegisterAuraEvent(SPELL_PET_SILVERBACK, 6, silverbackProc)

local SPELL_PET_CULLING_THE_HERD = 61680 -- pet_hunter.cpp:303
local PET_ICON_ID_CLAW = 262 -- pet_hunter.cpp:52
local PET_ICON_ID_BITE = 1680 -- pet_hunter.cpp:53
local PET_ICON_ID_SMACK = 473 -- pet_hunter.cpp:54

-- C++ spell_pet_culling_the_herd::CheckProc (pet_hunter.cpp:303-344):
-- DoCheckProc only, no HandleProc. Passes only when the event spell is
-- Claw, Bite or Smack (SpellIconID filter — the three share family
-- flags with other spells). Inverted ev6 veto: return true when the
-- icon matches none of the three, so a failing proc skips the DBC
-- default leg; return nothing on a pass so the default leg fires.
-- DORMANT until Spell.dbc ships the 61680 proc entry.
local function cullingTheHerdProc(event, aura, target, actor, procTarget, damage, procEvent)
    if procEvent == nil then
        return true
    end
    local icon = procEvent:GetSpellIconId()
    if icon == PET_ICON_ID_CLAW or icon == PET_ICON_ID_BITE or icon == PET_ICON_ID_SMACK then
        return
    end
    return true
end

RegisterAuraEvent(SPELL_PET_CULLING_THE_HERD, 6, cullingTheHerdProc)
