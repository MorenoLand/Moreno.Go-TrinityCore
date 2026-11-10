-- Mimiron channel-object spell scripts — Lua port of
-- spell_mimiron_p3wx2_laser_barrage and spell_mimiron_spinning_up in
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_mimiron.cpp (lines 2017-2041, 2405-2430).
-- One loader covers 63274 P3Wx2 Laser Barrage and 63414 Spinning Up.
-- Both carry the same HACK comment: the core sets no UNIT_FIELD_CHANNEL_OBJECT
-- automatically when the spell targets more than a single target, so the
-- script sets it per hit unit in OnEffectHitTarget (EFFECT_0,
-- SPELL_EFFECT_APPLY_AURA) — last hit wins, in effect-major order.
-- The Go twin is creature:SetChannelObjectGuid(unit)
-- (lua_creature_events.go), which writes the motion's ChannelTargetGUID
-- stamped at cast time (boss_ai.go:3087) ahead of the ev5 loop
-- (boss_ai.go:3406-3421), so the write overwrites the stamp exactly
-- like C++ overwrites SendChannelStart's value.
-- DORMANT caveat (fire-search precedent): no Spell.dbc ships in-tree,
-- so the channeled/multi-target shape of these casts is unverified and
-- the hooks may not fire until the DBC data lands; the registrations
-- themselves are exact.

local SPELL_P3WX2_LASER_BARRAGE = 63274 -- boss_mimiron.cpp:2017
local SPELL_SPINNING_UP = 63414 -- boss_mimiron.cpp:2405

-- C++ spell_mimiron_p3wx2_laser_barrage_SpellScript::OnHit
-- (boss_mimiron.cpp:2027-2031): caster sets its channel object to the
-- hit unit's GUID, unconditionally.
local function laserBarrageHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    caster:SetChannelObjectGuid(hitTarget)
end

-- C++ spell_mimiron_spinning_up_SpellScript::OnHit
-- (boss_mimiron.cpp:2417-2422): same leg, skipped when the hit unit
-- IS the caster (eff0 hits both caster and target per the spellmgr
-- hack noted at :2406-2407; the caster must not channel itself).
-- GUIDLow comparison is the pointer-identity twin (object.go:166).
local function spinningUpHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:GetGUIDLow() == caster:GetGUIDLow() then
        return
    end
    caster:SetChannelObjectGuid(hitTarget)
end

RegisterSpellEvent(SPELL_P3WX2_LASER_BARRAGE, 5, laserBarrageHit)
RegisterSpellEvent(SPELL_SPINNING_UP, 5, spinningUpHit)
