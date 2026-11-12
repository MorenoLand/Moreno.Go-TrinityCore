-- Bomb Bot (Ulduar, Mimiron Aerial Command Unit phase) — Lua port of
-- the bomb-bot legs in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_BOMB_BOT = 33836, summoned by the ACU's
-- SPELL_SUMMON_BOMB_BOT 63811 (boss_aerial_command_unit.lua).
--
-- The bot has NO C++ CreatureScript: its whole behavior is aura
-- SPELL_BOMB_BOT_AURA 63767, applied via creature_template_addon
-- (boss_mimiron.cpp:165). Go never applies creature_template_addon
-- auras, so the spawn handler applies it as a self-cast instead —
-- the aura's effects are DBC-driven on both engines, and Go's aura
-- system executes SPELL_AURA_PERIODIC_TRIGGER_SPELL ticks as full
-- casts (spells.go periodic-trigger arm), so the aura's triggered
-- 63801 explosion fires through the engine. The 63767 trigger
-- mechanics themselves are DBC-defined and unverifiable from repo
-- sources (no Spell.dbc on disk); if the DBC carries a different
-- aura type, the bot stays inert and the clear leg below stays
-- dormant — the file documents the dependency rather than guessing
-- a chase/explode timer.
-- DORMANT until Spell.dbc ships: the 63811 summon, the 63767 aura
-- and the 63801 damage are DBC-gated (creature path needs spellKnown).
local NPC_BOMB_BOT = 33836

local SPELL_BOMB_BOT_AURA = 63767
local SPELL_BOMB_BOT = 63801

-- boss_mimiron.cpp:184-186 Data enum; ulduar.h UlduarData
-- (DATA_COLOSSUS = 20, counting up) instance GUID key for the
-- Leviathan MK II (instance_ulduar.cpp:817).
local DATA_SETUP_BOMB = 1
local DATA_LEVIATHAN_MK_II = 32

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local UNIT_FLAG_PACIFIED = 0x00020000 -- UnitDefines.h:141

local function bombBotSpawn(event, creature)
    creature:CastSpell(creature, SPELL_BOMB_BOT_AURA)
end

-- C++ spell_mimiron_bomb_bot::HandleScript
-- (boss_mimiron.cpp:1713-1721, spell 63801 EFFECT_0 SCHOOL_DAMAGE):
-- when the explosion hits a player, the MK II's DATA_SETUP_BOMB latch
-- clears (the achievement_setup_boom gate).
-- C++ spell_mimiron_bomb_bot::HandleDespawn (:1721-1728, EFFECT_1
-- APPLY_AURA): a hit creature is flagged unselectable + pacified and
-- despawns 1s later. Both legs ride RegisterSpellEvent ev5
-- (OnEffectHitTarget, boss_ai.go:3406-3421 + triggered funnel
-- spells.go:27398, clear-fires precedent): the effect index is the
-- C++ leg selector, so no DBC shared-target-set assumption is needed
-- — eff0 fires only the player leg, eff1 only the creature leg.
-- Sweep-wipe note: the NOT_SELECTABLE|PACIFIED flag is re-affirmed
-- away within a tick (creaturemotion.go:4995), so its targeting
-- effect lasts <100ms; the 1s despawn still fires.
local function bombBotSpellHitTarget(event, spell, hitTarget, effectIndex)
    if not hitTarget then
        return
    end
    if effectIndex == 0 and hitTarget:GetTypeId() == 4 then -- TYPEID_PLAYER
        local mkii = spell:GetCaster():GetInstanceCreature(DATA_LEVIATHAN_MK_II)
        if mkii then
            mkii:SetData(DATA_SETUP_BOMB, 0)
        end
        return
    end
    if effectIndex == 1 and hitTarget:GetTypeId() == 3 then -- TYPEID_UNIT
        hitTarget:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_PACIFIED)
        CreateLuaEvent(function()
            hitTarget:Despawn()
        end, 1000, 0)
    end
end

RegisterCreatureEvent(NPC_BOMB_BOT, 5, bombBotSpawn)
RegisterSpellEvent(SPELL_BOMB_BOT, 5, bombBotSpellHitTarget)
