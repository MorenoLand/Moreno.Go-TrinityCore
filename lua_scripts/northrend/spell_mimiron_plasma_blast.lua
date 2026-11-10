-- Mimiron plasma-blast spell script — Lua port of
-- spell_mimiron_plasma_blast in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2044-2080).
-- C++ HandleScript (EFFECT_0, SPELL_EFFECT_SCRIPT_EFFECT,
-- OnEffectHitTarget): the caster's vehicle passenger on seat 3 casts
-- Plasma Blast 62997 on the hit unit, NOT triggered. Load() requires
-- the caster to have a vehicle kit.
-- PORTED via RegisterSpellEvent ev5 (the napalm-shell precedent). The
-- Load() gate is the nil GetVehicleKit() check (luaVehicleObject
-- returns nil with no kit); a vacant seat 3 reads nil from
-- GetPassenger and skips the cast, matching C++'s null check.
-- DORMANT caveat: 64542 has no cast site in the C++ scripts tree
-- (DBC-driven), so the hook fires only when the cast reaches the
-- creature path.

local SPELL_PLASMA_BLAST_TRIGGER = 64542 -- boss_mimiron.cpp:2044
local SPELL_PLASMA_BLAST = 62997 -- boss_mimiron.cpp:89

-- C++ HandleScript (boss_mimiron.cpp:2063-2067).
local function plasmaBlastScript(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local kit = caster:GetVehicleKit()
    if kit == nil then
        return
    end
    local passenger = kit:GetPassenger(3)
    if passenger ~= nil then
        passenger:CastSpell(hitTarget, SPELL_PLASMA_BLAST)
    end
end

RegisterSpellEvent(SPELL_PLASMA_BLAST_TRIGGER, 5, plasmaBlastScript)
