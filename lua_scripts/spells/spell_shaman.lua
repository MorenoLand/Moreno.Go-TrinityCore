-- Shaman spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_shaman.cpp.
--
-- PORTED (tick 199, RegisterAuraEvent OnApply = 2, OnPeriodic = 1):
--   spell_sha_earthbind_totem_AuraScript::Apply
--   (spell_shaman.cpp:499-507): EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
--   On apply, the totem's owner player (GetCharmerOrOwnerPlayerOrPlayerItself;
--   the totem's GetOwner twin, with the player-self leg) rolls the
--   Storm, Earth and Fire talent aura (51483, EFFECT_1, any rank)
--   amount; on success the totem self-casts Earthgrab 64695,
--   non-triggered.
--   spell_sha_earthbind_totem_AuraScript::HandleEffectPeriodic
--   (spell_shaman.cpp:476-492): same effect. Each tick the owner's
--   Storm, Earth and Fire dummy aura (SPELLFAMILY_SHAMAN 11, icon
--   2289, EFFECT_0) rolls its base amount; on success the aura
--   target self-casts Earthen Power 59566, triggered. No
--   PreventDefaultAction in C++, so the handler returns nothing and
--   the Go default trigger cast still fires.
--   The npcbot creator leg (spell_shaman.cpp:481-486) stays out —
--   no npcbot model in Go.
--
-- PORTED (tick 200, RegisterAuraEvent OnChangeAmount = 4):
--   spell_sha_maelstrom_weapon_AuraScript::HandleBonus
--   (spell_shaman.cpp:1570-1580): EFFECT_0 SPELL_AURA_ADD_PCT_MODIFIER,
--   AURA_EFFECT_HANDLE_CHANGE_AMOUNT — fires on stack changes, not on
--   apply, so it rides the tick-200 OnChangeAmount event, not OnApply.
--   At full stacks (5) the shaman rolls the T10 Enhancement 4P bonus
--   aura (70832, EFFECT_0) amount; on success it self-casts Maelstrom
--   Power (70831). The C++ cast passes the T10 aura effect as the
--   triggered-by-aura; Go has no such parameter, so the cast is plain.
--   GetSpellInfo()->StackAmount is 5 for every rank; GetSpellStackAmount
--   reads the same value from Spell.dbc and falls back to 5 when the
--   DBC row is unknown.
--
-- PORTED (tick 295, RegisterAuraEvent OnPeriodic = 1):
--   spell_sha_astral_shift_visual_dummy::PeriodicTick
--   (spell_shaman.cpp:276-292): EFFECT_0 PERIODIC_DUMMY. Each tick the
--   visual aura removes itself unless the target still carries a
--   stun, flee, or silence flag. No PreventDefaultAction in C++, so
--   the Go default tick still fires (no return). Caveat: the leg is
--   only as live as the Go UnitFlags mirror — if the engine never
--   sets the STUNNED/FLEEING/SILENCED bits on the player state, it
--   removes on the first tick.
--   spell_sha_mana_tide::PeriodicTick (spell_shaman.cpp:1634-1656):
--   EFFECT_0 PERIODIC_TRIGGER_SPELL. Each tick casts the effect's
--   DBC trigger spell (aura:GetTriggerSpell(0), 0-skip) on nil with
--   BP0 = the effect amount, triggered (CastSpellExtraArgs(aurEff)
--   sets TRIGGERED_FULL_MASK, SpellDefines.h:166). PreventDefaultAction
--   in C++ rides the return-true twin, suppressing the Go default
--   trigger cast.
--
-- PORTED (Go, census rows 248-250):
--   spell_sha_ancestral_awakening_proc::HandleDummy
--   (spell_shaman.cpp:169-180): EFFECT_0 DUMMY. Per hit unit the caster
--   casts 52752 triggered with BP0 = GetEffectValue(). The FilterTargets
--   arm (lowest-health-pct raid target only, when 2+ targets) has no Go
--   hook. The ancestral awakening aura proc arm stays out.
--   spell_sha_astral_shift::Absorb (spell_shaman.cpp:224-231): the
--   SCHOOL_ABSORB leg for 51474, gated on the stun/fear/silence flags
--   (stun also needs a stun-mechanic aura); the CalculateAmount -1 pool
--   is modeled by never consuming it. The astral shift aura CheckProc
--   arm stays out.
--   spell_sha_bloodlust::ApplyDebuff (spell_shaman.cpp:313-319): the
--   AfterHit arm — each hit target gains Sated 57724, triggered. The
--   RemoveInvalidTargets arm has no Go hook.
--
-- PORTED (census rows 261-265, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_sha_mana_tide_totem_SpellScript::HandleDummy
--   (spell_shaman.cpp:1675-1695): EFFECT_0 DUMMY. Each mana-using hit
--   unit is energized by Mana Tide Totem (39609) with BP0 = the effect
--   value (6) percent of its max mana, plus the Glyph of Mana Tide
--   (55441) amount from the totem owner's aura. Non-triggered cast;
--   the original-caster leg has no Go model.
--
-- CLOSED (tick 295 census, all 50 loader classes): the other 46 are
--   stay-outs — proc arms (ancestral awakening, earth shield,
--   earthliving, flametongue, frozen power, glyphs, imp water shield,
--   lightning overload/shield, nature guardian, shamanistic rage,
--   spirit hunt, static shock, tidal force, t3 6p, t8/t9/t10 bonuses,
--   windfury), absorb/calc arms (shadow ward twin,
--   rend twin), SpellScript-only (chain heal, cleansing
--   pulse, earthen power, fire nova, healing/mana spring totem,
--   heroism, lava lash, stoneclaw, thunderstorm), AfterDispel (flame
--   shock), summon-slot arms (sentry totem, totemic mastery — no
--   totem-slot bridge; only the PET slot is modeled), and the t3 8p
--   self-remove (family-flag GetAuraEffect overload unbridged).
--   Astral shift aura is proc, not the visual dummy above; the 51474
--   absorb leg is Go-ported above.

local SPELL_SHAMAN_EARTHBIND_TOTEM = 6474 -- spell_shaman.cpp:460 (class spell_sha_earthbind_totem)
local SPELL_SHAMAN_STORM_EARTH_AND_FIRE = 51483 -- spell_shaman.cpp:62
local SPELL_SHAMAN_TOTEM_EARTHBIND_EARTHGRAB = 64695 -- spell_shaman.cpp:63
local SPELL_SHAMAN_TOTEM_EARTHEN_POWER = 59566 -- spell_shaman.cpp:65
local SPELL_SHAMAN_MAELSTROM_WEAPON = 53817 -- spell_shaman.cpp:1550 (class spell_sha_maelstrom_weapon)
local SPELL_SHAMAN_MAELSTROM_POWER = 70831 -- spell_shaman.cpp:92
local SPELL_SHAMAN_T10_ENHANCEMENT_4P_BONUS = 70832 -- spell_shaman.cpp:93
local SPELL_SHAMAN_MANA_TIDE_TOTEM_SPELL = 39610 -- spell_shaman.cpp:1660 (class spell_sha_mana_tide_totem)
local SPELL_SHAMAN_MANA_TIDE_TOTEM = 39609 -- spell_shaman.cpp:58
local SPELL_SHAMAN_GLYPH_OF_MANA_TIDE = 55441 -- spell_shaman.cpp:49
local POWER_MANA = 0 -- Powers::POWER_MANA (Powers.h)
local MAELSTROM_WEAPON_MAX_STACKS = 5 -- spell_shaman.cpp:1572 (GetSpellInfo()->StackAmount)
local SPELLFAMILY_SHAMAN = 11 -- spells.go:36322
local SHAMAN_ICON_ID_STORM_EARTH_AND_FIRE = 2289 -- spell_shaman.cpp:484 (dummy icon)

local function getOwnerPlayer(unit)
    if unit == nil then
        return nil
    end
    local owner = unit:GetOwner()
    if owner ~= nil then
        return owner
    end
    if unit:GetObjectType() == "Player" then
        return unit
    end
    return nil
end

local function earthbindApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local owner = getOwnerPlayer(caster)
    if owner == nil then
        return
    end
    local amount = owner:GetAuraEffectOfRankedSpell(SPELL_SHAMAN_STORM_EARTH_AND_FIRE, 1)
    if amount == nil then
        return
    end
    if math.random(0, 99) < amount then
        caster:CastSpell(caster, SPELL_SHAMAN_TOTEM_EARTHBIND_EARTHGRAB, false)
    end
end

local function earthbindPeriodic(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local owner = getOwnerPlayer(caster)
    if owner == nil then
        return
    end
    local amount = owner:GetDummyAuraEffect(SPELLFAMILY_SHAMAN, SHAMAN_ICON_ID_STORM_EARTH_AND_FIRE, 0)
    if amount == nil then
        return
    end
    if math.random(0, 99) < amount then
        target:CastSpell(target, SPELL_SHAMAN_TOTEM_EARTHEN_POWER, true)
    end
end

local function maelstromWeaponChangeAmount(event, aura, target)
    local maxStacks = aura:GetSpellStackAmount(SPELL_SHAMAN_MAELSTROM_WEAPON)
    if maxStacks == 0 then
        maxStacks = MAELSTROM_WEAPON_MAX_STACKS
    end
    if aura:GetStackAmount() < maxStacks then
        return
    end
    -- C++ GetUnitOwner() is the aura owner = the hook target (self-buff).
    local amount = target:GetAuraEffect(SPELL_SHAMAN_T10_ENHANCEMENT_4P_BONUS, 0)
    if amount == nil then
        return
    end
    if math.random(0, 99) < amount then
        target:CastSpell(nil, SPELL_SHAMAN_MAELSTROM_POWER)
    end
end

RegisterAuraEvent(SPELL_SHAMAN_EARTHBIND_TOTEM, 2, earthbindApply)
RegisterAuraEvent(SPELL_SHAMAN_EARTHBIND_TOTEM, 1, earthbindPeriodic)
RegisterAuraEvent(SPELL_SHAMAN_MAELSTROM_WEAPON, 4, maelstromWeaponChangeAmount)

local SPELL_SHAMAN_ASTRAL_SHIFT_VISUAL_DUMMY = 52179 -- spell_shaman.cpp:276 (class spell_sha_astral_shift_visual_dummy)
local SPELL_SHAMAN_MANA_TIDE = 16191 -- spell_shaman.cpp:1634 (class spell_sha_mana_tide)
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h:117 (lua_player.go HasFlag twin)
local UNIT_FLAG_SILENCED = 0x2000 -- UnitDefines.h:137
local UNIT_FLAG_STUNNED = 0x40000 -- UnitDefines.h:142
local UNIT_FLAG_FLEEING = 0x800000 -- UnitDefines.h:147

local function astralShiftVisualDummyTick(event, aura, target)
    if not target:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_STUNNED + UNIT_FLAG_FLEEING + UNIT_FLAG_SILENCED) then
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_SHAMAN_ASTRAL_SHIFT_VISUAL_DUMMY, 1, astralShiftVisualDummyTick)

local function manaTideTick(event, aura, target)
    local trigger = aura:GetTriggerSpell(0)
    if trigger == 0 then
        return true
    end
    target:CastCustomSpell(nil, trigger, true, aura:GetAmount(0))
    return true
end

RegisterAuraEvent(SPELL_SHAMAN_MANA_TIDE, 1, manaTideTick)

-- C++ spell_sha_mana_tide_totem_SpellScript::HandleDummy
-- (spell_shaman.cpp:1675-1695): EFFECT_0 OnEffectHitTarget — each
-- mana-using hit unit is energized by Mana Tide Totem (39609) with
-- BP0 = the effect value (6) percent of its max mana, plus the
-- Glyph of Mana Tide (55441) dummy amount from the totem owner's
-- aura. The C++ cast is non-triggered (plain CastSpellExtraArgs
-- carrying only the original-caster GUID and BP0); the
-- original-caster leg has no Go model. DORMANT until Spell.dbc
-- ships the 39610 and 39609 rows.
local function manaTideTotemHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetPowerType() ~= POWER_MANA then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local effValue = spell:GetEffectValue(0)
    if effValue <= 0 then
        return
    end
    local owner = caster:GetOwner()
    if owner ~= nil then
        local glyph = owner:GetAuraEffect(SPELL_SHAMAN_GLYPH_OF_MANA_TIDE, 0)
        if glyph ~= nil then
            effValue = effValue + glyph
        end
    end
    local bp0 = math.floor(hitTarget:GetMaxPower(POWER_MANA) * effValue / 100)
    caster:CastCustomSpell(hitTarget, SPELL_SHAMAN_MANA_TIDE_TOTEM, false, bp0)
end

RegisterSpellEvent(SPELL_SHAMAN_MANA_TIDE_TOTEM_SPELL, 5, manaTideTotemHit)
