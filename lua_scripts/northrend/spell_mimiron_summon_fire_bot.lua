-- Mimiron summon-fire-bot aura script — Lua port of
-- spell_mimiron_summon_fire_bot in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2501-2535).
-- One loader covers the 64621 Summon Fire Bot dummy aura.
-- C++ OnRemove (EFFECT_0, SPELL_AURA_DUMMY): if the aura caster has
-- an instance script and the Mimiron boss state is IN_PROGRESS, the
-- caster casts the summon 64622 on itself, NOT triggered, with the
-- aerial command unit instance GUID as original caster.
-- PORTED via RegisterAuraEvent ev3 (lua_aura.go fireAuraRemove fires
-- ev3 once per removal). The original-caster extra arg has no Go
-- model (CastCustomSpell accepts and ignores it,
-- lua_creature_events.go) — passed for intent, the cast itself
-- still fires. The { aurEff } extra arg has no Lua twin.
-- The 64621 aura itself is applied by the 64620 trigger leg
-- (spell_mimiron_summon_fire_bot_target.lua).

local SPELL_SUMMON_FIRE_BOT_DUMMY = 64621 -- boss_mimiron.cpp:148
local SPELL_SUMMON_FIRE_BOT = 64622 -- boss_mimiron.cpp:147
local BOSS_MIMIRON = 10 -- ulduar.h UlduarBosses
local BOSS_STATE_IN_PROGRESS = 1 -- EncounterState (NOT_STARTED 0, IN_PROGRESS 1)
local DATA_AERIAL_COMMAND_UNIT = 34 -- ulduar.h UlduarData (DATA_LEVIATHAN_MK_II = 32, counting up)

-- C++ OnRemove (boss_mimiron.cpp:2514-2521).
local function summonFireBotRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if caster:GetBossState(BOSS_MIMIRON) ~= BOSS_STATE_IN_PROGRESS then
        return
    end
    local acu = caster:GetInstanceCreature(DATA_AERIAL_COMMAND_UNIT)
    caster:CastCustomSpell(caster, SPELL_SUMMON_FIRE_BOT, false, nil, nil, nil, nil, acu)
end

RegisterAuraEvent(SPELL_SUMMON_FIRE_BOT_DUMMY, 3, summonFireBotRemove)
