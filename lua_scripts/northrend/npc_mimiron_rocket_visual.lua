-- Rocket (Mimiron Visual) (Ulduar, Mimiron VX-001 phase) — Lua port of
-- the rocket-strike tail in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp.
-- Entry (ulduar.h): NPC_ROCKET_MIMIRON_VISUAL = 34050, summoned by
-- SPELL_SUMMON_ROCKET_STRIKE 63036 (cast by the 63681 hit unit,
-- boss_vx_001.lua rocketTargetScript).
--
-- C++ behavior is DBC/aura-driven and has no CreatureScript: the visual
-- spawns with aura 64064 (SPELL_ROCKET_STRIKE, creature_template_addon,
-- boss_mimiron.cpp:126), which casts the damage spell
-- SPELL_ROCKET_STRIKE_DAMAGE 63041. Go never applies
-- creature_template_addon auras, so the spawn handler casts 63041
-- directly (the mine precedent: the mine's C++ AI timers are likewise
-- replaced by Lua schedules) and despawns right after — the
-- spell_mimiron_rocket_strike_damage AfterCast twin
-- (boss_mimiron.cpp:2307-2310, TempSummon DespawnOrUnsummon). The
-- 63041 cast is instant, so the ev15 clear leg below fires inside
-- the CastSpell call, before the despawn.
-- DORMANT until Spell.dbc ships: the 64402/65034/63681/63036 casts
-- and the 63041 damage are DBC-gated (creature path needs spellKnown).
local NPC_ROCKET_MIMIRON_VISUAL = 34050

local SPELL_ROCKET_STRIKE_DAMAGE = 63041

-- boss_mimiron.cpp:184-186 Data enum; ulduar.h UlduarData
-- (DATA_COLOSSUS = 20, counting up) instance GUID key for the
-- Leviathan MK II (instance_ulduar.cpp:817).
local DATA_SETUP_ROCKET = 2
local DATA_LEVIATHAN_MK_II = 32

local function rocketVisualSpawn(event, creature)
    creature:CastSpell(creature, SPELL_ROCKET_STRIKE_DAMAGE)
    creature:Despawn()
end

-- C++ spell_mimiron_rocket_strike_damage::HandleScript
-- (boss_mimiron.cpp:2311-2313, spell 63041): when the damage hits a
-- player, the MK II's DATA_SETUP_ROCKET latch clears (the
-- achievement_setup_rocket gate). The leg rides the creature
-- SpellHitTarget hook (ev15, mine precedent): the visual is the
-- caster, the handler fires per hit player, and the MK II resolves
-- via the instance GUID store (latched by
-- boss_leviathan_mk_ii.lua ev5).
local function rocketVisualSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_ROCKET_STRIKE_DAMAGE then
        return
    end
    if not target or target:GetTypeId() ~= 4 then -- TYPEID_PLAYER
        return
    end
    local mkii = creature:GetInstanceCreature(DATA_LEVIATHAN_MK_II)
    if mkii then
        mkii:SetData(DATA_SETUP_ROCKET, 0)
    end
end

RegisterCreatureEvent(NPC_ROCKET_MIMIRON_VISUAL, 5, rocketVisualSpawn)
RegisterCreatureEvent(NPC_ROCKET_MIMIRON_VISUAL, 15, rocketVisualSpellHitTarget)
