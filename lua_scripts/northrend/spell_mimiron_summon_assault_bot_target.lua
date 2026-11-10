-- Mimiron summon-scrap-bot trigger spell script — Lua port of
-- spell_mimiron_summon_assault_bot_target in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2469-2499).
-- One loader covers 64425 Summon Scrap Bot Trigger.
-- C++ HandleDummy (EFFECT_0, SPELL_EFFECT_DUMMY, OnEffectHitTarget):
-- the hit unit self-casts the dummy aura 64426, triggered.
-- PORTED via RegisterSpellEvent ev5 (the napalm-shell precedent:
-- the funnel fires ev5 once per (effect, live hit target) on the
-- creature path). The 64426 aura's OnRemove leg lives in
-- spell_mimiron_summon_assault_bot.lua. DORMANT caveat (skadi
-- precedent): 64425 is cast only by the aerial command unit's
-- summon pump (DoCastAOE in boss_aerial_command_unit.lua), so the
-- hook fires only when that cast reaches the creature path.

local SPELL_SUMMON_ASSAULT_BOT_TRIGGER = 64425 -- boss_mimiron.cpp:2469
local SPELL_SUMMON_ASSAULT_BOT_DUMMY = 64426 -- boss_mimiron.cpp:160

-- C++ HandleDummy (boss_mimiron.cpp:2482-2485).
local function summonAssaultBotTargetScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_SUMMON_ASSAULT_BOT_DUMMY, true)
end

RegisterSpellEvent(SPELL_SUMMON_ASSAULT_BOT_TRIGGER, 5, summonAssaultBotTargetScript)
