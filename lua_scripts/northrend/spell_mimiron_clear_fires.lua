-- Mimiron clear-fires spell script — Lua port of
-- spell_mimiron_clear_fires in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 1743-1770).
-- One loader covers four spells: 65192 Flame Suppressant (VX-001),
-- 65224 Clear Fires, 65354 Clear Fires (frost bomb, DoCastAOE), 64619
-- Water Spray. No cast site for 65224 exists in the C++ scripts tree;
-- the loader registers it regardless, as C++ does. The adjacent loader
-- spell_mimiron_despawn_assault_bots (boss_mimiron.cpp:1771-1795, 64463
-- Despawn Assault Bots, EFFECT_0 SCRIPT_EFFECT, same hit-creature
-- despawn arm; cast side already ported, boss_mimiron.lua:388 JustDied
-- DoCastAOE) rides the same handler.

local SPELL_FLAME_SUPPRESSANT_VX = 65192 -- boss_mimiron.cpp:95
local SPELL_CLEAR_FIRES_A = 65224
local SPELL_CLEAR_FIRES = 65354 -- boss_mimiron.cpp:144
local SPELL_WATER_SPRAY = 64619 -- boss_mimiron.cpp:153
local SPELL_DESPAWN_ASSAULT_BOTS = 64463 -- boss_mimiron.cpp:80

-- C++ HandleDummy (boss_mimiron.cpp:1754-1758): EFFECT_0 SPELL_EFFECT_DUMMY,
-- OnEffectHitTarget — if the hit target is a creature, despawn it.
-- The GetObjectType() == "Creature" guard is the GetHitCreature() twin
-- (boss_marwyn.lua ev5 precedent); player hit targets have no Despawn.
-- RegisterSpellEvent ev5 = OnEffectHitTarget (runtime.go:506-589),
-- fired per (effect, live hit target) on the creature path and the
-- triggered funnel (boss_ai.go:3406-3421, spells.go:27398); the player
-- path has no per-effect list site, so a player-cast clear-fires
-- stays blocked there. DORMANT caveat (skadi precedent): no Spell.dbc
-- ships in-tree, so the casts may not reach these hooks at all until
-- the DBC data lands; the registrations themselves are exact.
local function clearFiresDummy(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget ~= nil and hitTarget:GetObjectType() == "Creature" then
        hitTarget:Despawn()
    end
end

RegisterSpellEvent(SPELL_FLAME_SUPPRESSANT_VX, 5, clearFiresDummy)
RegisterSpellEvent(SPELL_CLEAR_FIRES_A, 5, clearFiresDummy)
RegisterSpellEvent(SPELL_CLEAR_FIRES, 5, clearFiresDummy)
RegisterSpellEvent(SPELL_WATER_SPRAY, 5, clearFiresDummy)
RegisterSpellEvent(SPELL_DESPAWN_ASSAULT_BOTS, 5, clearFiresDummy)
