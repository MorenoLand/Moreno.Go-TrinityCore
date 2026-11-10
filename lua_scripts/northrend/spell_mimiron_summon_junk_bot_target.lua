-- Mimiron summon-junk-bot trigger spell script — Lua port of
-- spell_mimiron_summon_junk_bot_target in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2724-2755).
-- One loader covers 63820 Summon Scrap Bot Trigger.
-- C++ HandleDummy (EFFECT_0, SPELL_EFFECT_DUMMY, OnEffectHitTarget):
-- the hit unit self-casts the dummy aura 64398, triggered.
-- PORTED via RegisterSpellEvent ev5 (the napalm-shell precedent:
-- the funnel fires ev5 once per (effect, live hit target) on the
-- creature path). The 64398 aura's OnRemove leg lives in
-- spell_mimiron_summon_junk_bot.lua. DORMANT caveat (skadi
-- precedent): 63820 is cast only by the aerial command unit's
-- summon pump (DoCastAOE in boss_aerial_command_unit.lua), so the
-- hook fires only when that cast reaches the creature path.

local SPELL_SUMMON_JUNK_BOT_TRIGGER = 63820 -- boss_mimiron.cpp:2724
local SPELL_SUMMON_JUNK_BOT_DUMMY = 64398 -- boss_mimiron.cpp:157

-- C++ HandleDummy (boss_mimiron.cpp:2737-2740).
local function summonJunkBotTargetScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_SUMMON_JUNK_BOT_DUMMY, true)
end

RegisterSpellEvent(SPELL_SUMMON_JUNK_BOT_TRIGGER, 5, summonJunkBotTargetScript)
