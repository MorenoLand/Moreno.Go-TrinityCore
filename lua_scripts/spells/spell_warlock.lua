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
-- PORTED (tick 202, RegisterAuraEvent):
--   spell_warl_demonic_circle_summon HandleRemove (spell_warlock.cpp:349-354)
--   + HandleDummyTick (:356-372) and spell_warl_demonic_circle_teleport
--   HandleTeleport (:398-409); the GetGameObject/RemoveGameObject bridges
--   (Unit::GetGameObject/RemoveGameObject, Unit.cpp:5199/5263) are new
--   this tick. The tick's 40-yard range is 48020's DBC friendly max
--   (SpellRange; the 40 yd tooltip range, wowwiki Demonic Circle /
--   Demonic Circle: Teleport); the reach fold follows the maiden idiom
--   (Object.cpp:1147-1165: IsWithinDist adds own + target combat reach;
--   the circle GO carries none, the player 1.5).
--   STILL BLOCKED leg: HandleTeleport's RemoveMovementImpairingAuras
--   (false) — no bridge (root/snare removal has no Lua surface).
--   (STALE as of tick 295: the bridge is live — player
--   RemoveMovementImpairingAuras, lua_player.go:3473 — and the port
--   below already calls it; the note is retired, not the leg.)
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

local SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON = 48018 -- spell_warlock.cpp:41
local SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT = 48020 -- spell_warlock.cpp:42
local SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST = 62388 -- spell_warlock.cpp:43
local SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT_RANGE = 40 -- 48020 DBC friendly max range (yards)

-- C++ spell_warl_demonic_circle_summon::HandleRemove
-- (spell_warlock.cpp:349-354): EFFECT_0 PERIODIC_DUMMY,
-- REAL_OR_REAPPLY. On removal the summoned circle goes too (Go's
-- remove hook fires only on genuine removal, never on reapply, so
-- the C++ !(mode & REAPPLY) gate is satisfied by construction) and
-- the allow-cast aura drops.
local function demonicCircleSummonRemove(event, aura, target)
    target:RemoveGameObject(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON)
    target:RemoveAura(SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST)
end

RegisterAuraEvent(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON, 3, demonicCircleSummonRemove)

-- C++ spell_warl_demonic_circle_summon::HandleDummyTick
-- (spell_warlock.cpp:356-372): EFFECT_0 PERIODIC_DUMMY. While the
-- circle exists, a target within the teleport spell's max range gains
-- the allow-cast aura (62388); out of range it drops. No
-- PreventDefault in C++, so the Go default tick still fires.
local function demonicCircleSummonTick(event, aura, target)
    local circle = target:GetGameObject(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON)
    if circle ~= nil then
        if target:GetDistance(circle) <= SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT_RANGE + 1.5 then
            if not target:HasAura(SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST) then
                target:CastSpell(target, SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST, true)
            end
        else
            target:RemoveAura(SPELL_WARLOCK_DEMONIC_CIRCLE_ALLOW_CAST)
        end
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON, 1, demonicCircleSummonTick)

-- C++ spell_warl_demonic_circle_teleport::HandleTeleport
-- (spell_warlock.cpp:398-409): EFFECT_0 MECHANIC_IMMUNITY, REAL. The
-- player target teleports to its circle's position and orientation,
-- then drops its movement-impairing auras (snares only — C++ passes
-- withRoot=false; the per-effect snare amount-zeroing leg has no Go
-- model, same documented residual as the bridge).
local function demonicCircleTeleportApply(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    local circle = target:GetGameObject(SPELL_WARLOCK_DEMONIC_CIRCLE_SUMMON)
    if circle ~= nil then
        target:TeleportNear(circle:GetX(), circle:GetY(), circle:GetZ(), circle:GetO())
        target:RemoveMovementImpairingAuras(false)
    end
end

RegisterAuraEvent(SPELL_WARLOCK_DEMONIC_CIRCLE_TELEPORT, 2, demonicCircleTeleportApply)

-- PORTED (tick 295, RegisterAuraEvent OnRemove = 3):
--   spell_warl_curse_of_doom::OnRemove (spell_warlock.cpp:275-296):
--   EFFECT_0 PERIODIC_DAMAGE, REAL. On removal the doomguard
--   (18662) is summoned only when the aura was removed by the
--   target's death AND its duration had expired: GetRemoveMode() == 5
--   (AURA_REMOVE_BY_DEATH, lua_aura.go GetRemoveMode twin) and
--   GetDuration() == 0 on the remove snapshot (the IsExpired proxy;
--   the snapshot carries the removed record's last remaining ms,
--   lua_aura.go auraRemainingMs — a death-removed live aura reads
--   > 0, a permanent aura reads -1). The C++ Load gate (caster is a
--   player) rides a handler-side check, and the honor/XP-target gate
--   rides IsHonorOrXPTarget. The cast is triggered (args(aurEff)
--   sets TRIGGERED_FULL_MASK). Registered for rank 1 (603) only;
--   rank 2 (30910) is not established in the C++ tree.
local SPELL_WARLOCK_CURSE_OF_DOOM_R1 = 603 -- spell_warlock.cpp:261 (class spell_warl_curse_of_doom)
local SPELL_WARLOCK_CURSE_OF_DOOM_EFFECT = 18662 -- spell_warlock.cpp:40
local AURA_REMOVE_BY_DEATH = 5 -- SpellAuraDefines.h:61-69

local function curseOfDoomRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_DEATH then
        return
    end
    if aura:GetDuration() ~= 0 then
        return
    end
    if not caster:IsHonorOrXPTarget(target) then
        return
    end
    caster:CastSpell(target, SPELL_WARLOCK_CURSE_OF_DOOM_EFFECT, true)
end

RegisterAuraEvent(SPELL_WARLOCK_CURSE_OF_DOOM_R1, 3, curseOfDoomRemove)
