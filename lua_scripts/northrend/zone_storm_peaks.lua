-- Storm Peaks zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_storm_peaks.cpp.
--
-- PORTED (SpellScript, RegisterSpellEvent ev5 = OnEffectHitTarget):
--   spell_eject_passenger_wild_wyrm (60603, zone_storm_peaks.cpp:1070-1100):
--   EFFECT_0 SCRIPT_EFFECT. OnEffectHitTarget the hit unit loses
--   SPELL_FIGHT_WYRM (56673). Cast by the wild wyrm on ACTION_GRIP_LOST.
--
-- PORTED (tick 205, RegisterAuraEvent OnApply = 2 / OnPeriodic = 1):
--   spell_jokkum_scriptcast (61319, zone_storm_peaks.cpp:967-996):
--   EFFECT_0 DUMMY. OnApply the target casts Jokkum Summon (56541) on
--   itself, triggered. Quest 13010 "Krolmir, Hammer of Storms".
--   Aura binding is DB-side only: TDB spell_script_names row
--   (61319,'spell_jokkum_scriptcast')
--   (sql/old/3.3.5a/TDB52_to_TDB53_updates/world/
--   2013_11_24_00_world_spell_script_names.sql), Wowhead "Jokkum
--   Scriptcast" Effect 0 is Apply Aura: Dummy, 500 ms.
--   spell_veranus_summon (56650, zone_storm_peaks.cpp:998-1027):
--   EFFECT_0 DUMMY. OnApply the target casts Summon Veranus and Thorim
--   (56649) on itself, triggered. Quest "Territorial Trespass".
--   Aura binding is DB-side only: TDB spell_script_names row
--   (56650,'spell_veranus_summon')
--   (sql/old/3.3.5a/TDB52_to_TDB53_updates/world/
--   2013_12_23_00_world_misc.sql), Wowhead "Player Cast Veranus
--   Summon" Effect 0 is Apply Aura: Dummy, 500 ms.
--   spell_close_rift (56763, zone_storm_peaks.cpp:1034-1065):
--   EFFECT_0 PERIODIC_TRIGGER_SPELL. On the 5th periodic tick the
--   target casts Despawn Rift (61665) with a nil target, triggered.
--   Aura binding is DB-side only: TDB spell_script_names row
--   (56763,'spell_close_rift')
--   (sql/old/3.3.5a/TDB49_to_TDB50_updates/world/
--   2012_10_21_01_world_misc.sql), Wowhead "Close Rift" Effect 0 is
--   Apply Aura: Periodically trigger spell, every 1 second, 5 sec.
-- DELTA (close rift): C++ casts with a nullptr target; Go
-- creature:CastSpell(nil, ...) falls back to the target's current
-- victim and no-ops without one, so the port casts explicitly on the
-- target (self-cast, per the tick-193 triage note). The port returns
-- nothing, so the Go default trigger cast still fires each tick, as
-- in C++ (no PreventDefaultAction in the handler).

local SPELL_JOKKUM_SCRIPTCAST = 61319 -- TDB spell_script_names
local SPELL_JOKKUM_SUMMON = 56541 -- zone_storm_peaks.cpp:952
local SPELL_PLAYER_CAST_VERANUS_SUMMON = 56650 -- TDB spell_script_names
local SPELL_SUMMON_VERANUS_AND_THORIM = 56649 -- zone_storm_peaks.cpp:955
local SPELL_CLOSE_RIFT = 56763 -- TDB spell_script_names
local SPELL_DESPAWN_RIFT = 61665 -- zone_storm_peaks.cpp:1031
local SPELL_FIGHT_WYRM = 56673 -- zone_storm_peaks.cpp:634
local SPELL_EJECT_PASSENGER_1 = 60603 -- zone_storm_peaks.cpp:647

-- C++ spell_jokkum_scriptcast_AuraScript::OnApply
-- (zone_storm_peaks.cpp:980-984).
local function jokkumScriptcastApply(event, aura, target)
    target:CastSpell(target, SPELL_JOKKUM_SUMMON, true)
end

RegisterAuraEvent(SPELL_JOKKUM_SCRIPTCAST, 2, jokkumScriptcastApply)

-- C++ spell_veranus_summon_AuraScript::OnApply
-- (zone_storm_peaks.cpp:1011-1015).
local function veranusSummonApply(event, aura, target)
    target:CastSpell(target, SPELL_SUMMON_VERANUS_AND_THORIM, true)
end

RegisterAuraEvent(SPELL_PLAYER_CAST_VERANUS_SUMMON, 2, veranusSummonApply)

-- C++ spell_close_rift_AuraScript::HandlePeriodic
-- (zone_storm_peaks.cpp:1048-1052): _counter++ == 5 on the 5th tick
-- is aura:GetTickNumber() == 5 (first tick reads 1, tick-191 bridge).
local function closeRiftPeriodic(event, aura, target)
    if aura:GetTickNumber() == 5 then
        target:CastSpell(target, SPELL_DESPAWN_RIFT, true)
    end
end

RegisterAuraEvent(SPELL_CLOSE_RIFT, 1, closeRiftPeriodic)

-- C++ spell_eject_passenger_wild_wyrm::HandleScript
-- (zone_storm_peaks.cpp:1084-1087): EFFECT_0 SCRIPT_EFFECT
-- OnEffectHitTarget — the hit unit loses SPELL_FIGHT_WYRM (56673).
-- Cast by the wild wyrm on ACTION_GRIP_LOST (DoCastAOE, :746); the
-- hit units are the passengers. Fires on the creature path only (ev5,
-- runtime.go:574-577); the return value is ignored. DORMANT until
-- Spell.dbc ships the 60603 record.
local function ejectPassengerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAura(SPELL_FIGHT_WYRM)
end

RegisterSpellEvent(SPELL_EJECT_PASSENGER_1, 5, ejectPassengerHit)
