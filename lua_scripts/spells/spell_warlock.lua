-- Warlock spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_warlock.cpp.
--
-- PORTED (tick 195, RegisterAuraEvent):
--   spell_warl_drain_soul HandleTick (spell_warlock.cpp:546-562):
--   EFFECT_1 PERIODIC_DAMAGE. When the caster is a player and the
--   target yields experience or honor (isHonorOrXPTarget), each tick
--   has a 20% chance to create a Soul Shard: the caster casts
--   CREATE_SOULSHARD 43836 on itself, triggered by the aura effect.
--   Glyph of Drain Soul leg (:557-559): while the caster has the
--   glyph aura 58070, a second roll against that aura effect's
--   MiscValue (DBC, via Aura:GetSpellEffectMiscValue) casts the
--   glyph proc 58068 on the caster, triggered. No PreventDefaultAction
--   in C++, so the Go default damage tick still fires (no return).
--   Registered for rank 1 (1120), the only Drain Soul rank ID in the
--   C++ tree; ranks 2-6 are DB-bound via spell_script_names and
--   unestablished (C++ tree + web search exhausted this tick).
-- PORTED (tick 196, RegisterAuraEvent OnApply):
--   spell_warl_health_funnel ApplyEffect (spell_warlock.cpp:838-846):
--   EFFECT_0 PERIODIC_HEAL. If the caster is missing, nothing
--   happens; if the caster has Improved Health Funnel R2 (18704)
--   the target casts buff 60956 on itself, else if R1 (18703) buff
--   60955, both triggered.
-- PORTED (tick 197, RegisterAuraEvent OnRemove):
--   spell_warl_health_funnel RemoveEffect (spell_warlock.cpp:848-853):
--   EFFECT_0 PERIODIC_HEAL. OnRemove the target drops both
--   Improved Health Funnel buffs (60955/60956). C++ uses
--   RemoveAurasDueToSpell; the Go twin is target:RemoveAura(id):
--   each buff is self-cast once by the apply leg, so a single
--   application exists and removing the spell's aura is the same
--   removal (removeCreatureAura / removeAura drop the active aura
--   of that spell ID). This retires tick 196's "no
--   RemoveAurasDueToSpell bridge" note. Ordering is immaterial:
--   the arm never queries the funnel aura itself, and C++ registers
--   plain OnEffectRemove with no mode gate.
-- DOCUMENTED-ONLY (no behavior fabricated):
--   The same loader's CheckProc / HandleProc arms (:509-544,
--   EFFECT_2 improved Drain Soul mana proc) are proc arms, not
--   periodic ticks: no proc hook exists on the RegisterAuraEvent
--   surface, so they stay out with the aura-script registration.

local SPELL_WARLOCK_HEALTH_FUNNEL = 755
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R1 = 18703
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R2 = 18704
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R1 = 60955
local SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R2 = 60956
local SPELL_WARLOCK_DRAIN_SOUL_R1 = 1120
local SPELL_WARLOCK_CREATE_SOULSHARD = 43836
local SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA = 58070
local SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_PROC = 58068

local function drainSoulTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if not caster:IsHonorOrXPTarget(target) then
        return
    end
    if math.random(0, 99) < 20 then
        caster:CastSpell(caster, SPELL_WARLOCK_CREATE_SOULSHARD, true)
        if caster:HasAura(SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA) then
            local chance = aura:GetSpellEffectMiscValue(SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_AURA, 0)
            if math.random(0, 99) < chance then
                caster:CastSpell(caster, SPELL_WARLOCK_GLYPH_OF_DRAIN_SOUL_PROC, true)
            end
        end
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DRAIN_SOUL_R1, 1, drainSoulTick)

-- C++ spell_warl_health_funnel::ApplyEffect (spell_warlock.cpp:838-846).
local function healthFunnelApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if caster:HasAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R2) then
        target:CastSpell(target, SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R2, true)
    elseif caster:HasAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_R1) then
        target:CastSpell(target, SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R1, true)
    end
end

RegisterAuraEvent(SPELL_WARLOCK_HEALTH_FUNNEL, 2, healthFunnelApply)

-- C++ spell_warl_health_funnel::RemoveEffect (spell_warlock.cpp:848-853).
local function healthFunnelRemove(event, aura, target)
    target:RemoveAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R1)
    target:RemoveAura(SPELL_WARLOCK_IMPROVED_HEALTH_FUNNEL_BUFF_R2)
end

RegisterAuraEvent(SPELL_WARLOCK_HEALTH_FUNNEL, 3, healthFunnelRemove)

-- C++ spell_warl_haunt::HandleRemove (spell_warlock.cpp:797-806):
-- EFFECT_1 SPELL_AURA_DUMMY, REAL_OR_REAPPLY. On removal the target
-- casts Haunt Heal 48210 on the caster with BP0 = the effect amount
-- (aurEff->GetAmount()). Deviation: C++ sets args.OriginalCaster to
-- the caster GUID; CastCustomSpell takes an originalCaster arg but
-- the Go model ignores it on both paths, so it is not passed. The
-- loader's SpellScript side (spell_warl_haunt_SpellScript) is a
-- separate arm and stays out of this row.
local SPELL_WARLOCK_HAUNT = 48181 -- spell_warlock.cpp:55
local SPELL_WARLOCK_HAUNT_HEAL = 48210 -- spell_warlock.cpp:56

local function hauntRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    target:CastCustomSpell(caster, SPELL_WARLOCK_HAUNT_HEAL, false, aura:GetAmount(1))
end

RegisterAuraEvent(SPELL_WARLOCK_HAUNT, 3, hauntRemove)
