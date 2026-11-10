-- Mimiron summon-fire-bot trigger spell script — Lua port of
-- spell_mimiron_summon_fire_bot_target in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2536-2567).
-- One loader covers 64620 Summon Fire Bot Trigger.
-- C++ HandleDummy (EFFECT_0, SPELL_EFFECT_DUMMY, OnEffectHitTarget):
-- the hit unit self-casts the dummy aura 64621, triggered.
-- PORTED via RegisterSpellEvent ev5 (the napalm-shell precedent:
-- the funnel fires ev5 once per (effect, live hit target) on the
-- creature path). The 64621 aura's OnRemove leg lives in
-- spell_mimiron_summon_fire_bot.lua. DORMANT caveat (skadi
-- precedent): 64620 is cast only by the aerial command unit's
-- summon pump (DoCastAOE in boss_aerial_command_unit.lua), so the
-- hook fires only when that cast reaches the creature path.

local SPELL_SUMMON_FIRE_BOT_TRIGGER = 64620 -- boss_mimiron.cpp:2536
local SPELL_SUMMON_FIRE_BOT_DUMMY = 64621 -- boss_mimiron.cpp:148

-- C++ HandleDummy (boss_mimiron.cpp:2549-2552).
local function summonFireBotTargetScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_SUMMON_FIRE_BOT_DUMMY, true)
end

RegisterSpellEvent(SPELL_SUMMON_FIRE_BOT_TRIGGER, 5, summonFireBotTargetScript)
