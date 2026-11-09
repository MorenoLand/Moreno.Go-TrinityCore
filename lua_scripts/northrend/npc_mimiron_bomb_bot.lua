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
-- clears (the achievement_setup_boom gate). The leg rides the creature
-- SpellHitTarget hook (ev15, mine precedent): the bot is the caster,
-- the handler fires per hit player, and the MK II resolves via the
-- instance GUID store (latched by boss_leviathan_mk_ii.lua ev5).
-- C++ spell_mimiron_bomb_bot::HandleDespawn (:1721-1728, EFFECT_1
-- APPLY_AURA): a hit creature is flagged unselectable + pacified and
-- despawns 1s later. Ev15 fires once per hit target with no effect
-- index (boss_ai.go:4053-4109), so both C++ legs ride this handler,
-- gated on target type the way the C++ GetHitPlayer/GetHitCreature
-- gates are. DBC caveat: if 63801 EFFECT_1 targets only the caster,
-- only the bot itself takes this leg; the handler applies it to
-- whatever the engine's hit set contains.
local function bombBotSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_BOMB_BOT then
        return
    end
    if not target then
        return
    end
    if target:GetTypeId() == 4 then -- TYPEID_PLAYER
        local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
        if mkii then
            mkii:SetData(DATA_SETUP_BOMB, 0)
        end
        return
    end
    if target:GetTypeId() == 3 then -- TYPEID_UNIT
        target:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE + UNIT_FLAG_PACIFIED)
        CreateLuaEvent(function()
            target:Despawn()
        end, 1000, 0)
    end
end

RegisterCreatureEvent(NPC_BOMB_BOT, 5, bombBotSpawn)
RegisterCreatureEvent(NPC_BOMB_BOT, 15, bombBotSpellHitTarget)
