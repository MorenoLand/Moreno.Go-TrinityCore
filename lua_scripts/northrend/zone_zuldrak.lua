-- Zul'Drak zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_zuldrak.cpp.
--
-- PORTED (tick 197, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_scourge_disguise (51966, zone_zuldrak.cpp:966-981):
--   EFFECT_0 TRANSFORM. OnApply the target casts Scourge Disguise
--   Instability (51971) on itself, triggered; OnRemove the target
--   removes that instability aura (C++ plain RemoveAura). No Load
--   gates in C++; no PreventDefaultAction. This supersedes the
--   "no AuraScript bridge — documented-only" note in
--   npc_storm_cloud.lua, which predates the tick-196
--   RegisterAuraEvent hook.
-- STILL BLOCKED:
--   spell_scourge_disguise_instability (51971, :984-1012):
--   CalcPeriodic picks a random 30-240s amplitude (no calc-periodic
--   hook) and the periodic tick casts Scourge Disguise Expiring
--   (52010) on the target, triggered.
--   spell_scourge_disguise_expiring (52010, :1014-1030): OnApply
--   whispers TEXT_DISGUISE_WARNING (28891) to the player target —
--   PORTED (tick 204, below) via the new player:WhisperText bridge
--   (Unit::Whisper textId overload, lua_player.go).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the disguised
-- player's triggered self-cast lands non-triggered; pre-existing
-- bridge gap, same as all prior player-target ports.

local SPELL_SCOURGE_DISGUISE = 51966 -- zone_zuldrak.cpp:957
local SPELL_SCOURGE_DISGUISE_INSTABILITY = 51971 -- zone_zuldrak.cpp:958
local SPELL_SCOURGE_DISGUISE_EXPIRING = 52010 -- zone_zuldrak.cpp:959
local TEXT_DISGUISE_WARNING = 28891 -- zone_zuldrak.cpp:961

-- C++ spell_scourge_disguise::ApplyEffect (zone_zuldrak.cpp:969-972).
local function scourgeDisguiseApply(event, aura, target)
    target:CastSpell(target, SPELL_SCOURGE_DISGUISE_INSTABILITY, true)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE, 2, scourgeDisguiseApply)

-- C++ spell_scourge_disguise::RemoveEffect (zone_zuldrak.cpp:974-977).
local function scourgeDisguiseRemove(event, aura, target)
    target:RemoveAura(SPELL_SCOURGE_DISGUISE_INSTABILITY)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE, 3, scourgeDisguiseRemove)

-- C++ spell_scourge_disguise_expiring::OnApply (zone_zuldrak.cpp:1018-1025):
-- if the player target still has the disguise, whisper the warning text
-- to them (Unit::Whisper textId overload, bossWhisper=true -> 0x2A).
-- DELTA: C++ localizes broadcast_text by target locale + sender gender;
-- the Go bridge reads the enUS Text column only.
local function scourgeDisguiseExpiringApply(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    if target:HasAura(SPELL_SCOURGE_DISGUISE) then
        target:WhisperText(TEXT_DISGUISE_WARNING, target, true)
    end
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE_EXPIRING, 2, scourgeDisguiseExpiringApply)
