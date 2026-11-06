-- Mage spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_mage.cpp.
--
-- Binding IDs come from the `// <id> - Name` header comments above
-- each class (the SpellScriptLoader name binds in DB
-- spell_script_names); the effect spell IDs are the file's enums.
--
-- PORTED (tick 294, RegisterAuraEvent):
--   spell_mage_frostfire_bolt (spell_mage.cpp:695-721): aura 44614,
--   modes 2 + 3. Both legs: the aura's caster, when present, casts
--   68391 (Permafrost) on the target, triggered by the aura effect.
--   PORTED AS-REGISTERED: C++ binds AfterEffectRemove to
--   ApplyPermafrost (:718), not to RemovePermafrost (:710-713, dead
--   code) — so removal re-casts 68391, exactly as the C++ loader
--   registers it. The intended remove leg is an upstream bug, not a
--   porting decision.
--   spell_mage_mirror_image (spell_mage.cpp:1279-1299): aura 55342,
--   mode 1. PeriodicTick: the EFFECT_2 trigger spell (DBC read, 0
--   skips the cast) is cast by the target on nil, triggered. No
--   return true: C++ has no PreventDefaultAction. Inert until
--   Spell.dbc loads (0-read convention).
--   spell_mage_living_bomb (spell_mage.cpp:1101-1136): aura 44457,
--   mode 3. AfterRemove fires only when the remove mode is
--   ENEMY_SPELL (3) or EXPIRE (4): the aura's caster, when present,
--   casts the EFFECT_1 amount as a spell ID on the target,
--   triggered by the aura effect (0 amount skips the cast — the
--   DBC read for the explosion spell ID).
--   spell_mage_combustion_proc (spell_mage.cpp:419-449): aura 28682,
--   mode 3. OnRemove the target drops 11129 (Combustion).
--   Notification-only.
--   spell_mage_fingers_of_frost (spell_mage.cpp:587-625): aura
--   74396, mode 3 only. OnRemove the target drops 44544 (Fingers of
--   Frost aurastate aura). The proc leg (RemoveAuraFromStack on
--   proc) stays out — no proc hook, no RemoveAuraFromStack bridge.
--   Mode 2 fires on fresh apply only; the frostfire apply leg is
--   AURA_EFFECT_HANDLE_REAL_OR_REAPPLY_MASK, so a reapply does not
--   re-cast 68391 in Go (standing deviation).
-- STAY-OUT (tick 294): every other spell_mage class — no proc hook,
--   absorb/CalcAmount/ManaShield hooks absent, SpellScript-only
--   with no RegisterSpellScript in the engine, or cross-script state
--   (arcane_missiles / missile_barrage_proc share a flag no Lua leg
--   can set). Notable: ice_barrier (absorb + GetDummyAuraEffect
--   not on the aura-hook surface), ignite (DBC GetMaxTicks/rank
--   math), fire_frost_ward (absorb).

local SPELL_MAGE_FROSTFIRE_BOLT = 44614
local SPELL_MAGE_PERMAFROST_AURA = 68391
local SPELL_MAGE_MIRROR_IMAGE = 55342
local SPELL_MAGE_LIVING_BOMB = 44457
local SPELL_MAGE_COMBUSTION_PROC = 28682
local SPELL_MAGE_COMBUSTION = 11129
local SPELL_MAGE_FINGERS_OF_FROST = 74396
local SPELL_MAGE_FINGERS_OF_FROST_AURASTATE_AURA = 44544

local AURA_REMOVE_BY_ENEMY_SPELL = 3
local AURA_REMOVE_BY_EXPIRE = 4

local function frostfireBoltApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, SPELL_MAGE_PERMAFROST_AURA, true)
    end
end

RegisterAuraEvent(SPELL_MAGE_FROSTFIRE_BOLT, 2, frostfireBoltApply)
RegisterAuraEvent(SPELL_MAGE_FROSTFIRE_BOLT, 3, frostfireBoltApply)

local function mirrorImageTick(event, aura, target)
    local trigger = aura:GetTriggerSpell(2)
    if trigger ~= 0 then
        target:CastSpell(nil, trigger, true)
    end
end

RegisterAuraEvent(SPELL_MAGE_MIRROR_IMAGE, 1, mirrorImageTick)

local function livingBombRemove(event, aura, target)
    local mode = aura:GetRemoveMode()
    if mode ~= AURA_REMOVE_BY_ENEMY_SPELL and mode ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local id = aura:GetAmount(1)
    if id == 0 then
        return
    end
    local caster = aura:GetCaster()
    if caster then
        caster:CastSpell(target, id, true)
    end
end

RegisterAuraEvent(SPELL_MAGE_LIVING_BOMB, 3, livingBombRemove)

local function combustionProcRemove(event, aura, target)
    target:RemoveAura(SPELL_MAGE_COMBUSTION)
end

RegisterAuraEvent(SPELL_MAGE_COMBUSTION_PROC, 3, combustionProcRemove)

local function fingersOfFrostRemove(event, aura, target)
    target:RemoveAura(SPELL_MAGE_FINGERS_OF_FROST_AURASTATE_AURA)
end

RegisterAuraEvent(SPELL_MAGE_FINGERS_OF_FROST, 3, fingersOfFrostRemove)
