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
--   hook. The ancestral awakening aura proc arm is PORTED below
--   (ev6, census row 758).
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
-- PORTED (census rows 266-270):
--   spell_sha_sentry_totem_AuraScript::AfterRemove
--   (spell_shaman.cpp:1791-1796): EFFECT_1 DUMMY, AURA_EFFECT_HANDLE_REAL.
--   On remove the player caster stops bind sight (Player::StopCastingBindSight,
--   Player.cpp:24605-24616): the bind-sight aura comes off the air totem
--   (GetTotem slot 3). The AfterApply leg (spell_shaman.cpp:1781-1788, cast
--   Bind Sight 6277 on the totem) is Go-ported in totems.go — a Lua OnApply
--   twin cannot work: the 6495 aura applies in the cast effect loop, before
--   the post-loop summon fills the air slot. On unsummon the handler no-ops
--   (slot already cleared) and the Go teardown in destroyTotemGUID removes
--   the bind sight instead.
--   spell_sha_spirit_hunt_AuraScript::HandleProc
--   (spell_shaman.cpp:1905-1919): EFFECT_0 DUMMY. On a damaging proc the pet
--   actor casts Spirit Hunt Heal (58879) on itself and on its owner, both
--   triggered with BP0 = pct of the damage by the effect amount. The
--   zero-damage gate is the C++ null/zero-damage early return; a missing
--   owner skips both casts. The handler returns true (PreventDefaultAction),
--   so the proc loop skips the default leg — the dummy effect has no DBC
--   trigger spell anyway.
--   spell_sha_totemic_mastery_AuraScript::HandleDummy
--   (spell_shaman.cpp:2048-2061): EFFECT_0 PERIODIC_TRIGGER_SPELL. Each tick
--   the handler checks all four totem slots; with any slot empty it returns
--   and the Go default trigger cast fires; with all four filled it casts
--   Totemic Mastery (38437) itself, triggered, and returns true to veto the
--   default — exactly one 38437 per tick either way. Correct under both DBC
--   readings of the 38443 trigger spell (38437, or none).
--   spell_sha_shamanistic_rage (30823) and spell_sha_tidal_force_dummy
--   (55198) are Go-ported in procs.go (AP-pct BP0 for 30824; one-stack drop
--   of 55166) — no Lua ev6 twin: ev6 carries no attack-power or
--   RemoveAuraFromStack bridge.
--
-- PORTED (Go, census rows 271-275):
--   spell_sha_t3_6p_bonus (28823, spell_shaman.cpp:2100-2128): EFFECT_0
--   DUMMY. On proc the actor casts the class-bonus spell on the proc
--   target by its class (28824 MP5 / 28825 spell power / 28826 attack
--   power / 28827 armor) — no Lua ev6 twin: ev6 carries no target class.
--   spell_sha_t8_elemental_4p_bonus (64928, spell_shaman.cpp:2177-2197)
--   and spell_sha_t9_elemental_4p_bonus (67228, spell_shaman.cpp:2226-2246):
--   EFFECT_0 DUMMY. On a damaging proc the actor casts Electrified
--   (64930) / Lava Burst Bonus Damage (71824) on the proc target with
--   BP0 = pct of the damage by the effect amount, divided by the
--   triggered spell's max ticks — no Lua ev6 twin: ev6 carries the
--   damage but no max-ticks bridge.
--   spell_sha_t10_elemental_4p_bonus (70817, spell_shaman.cpp:2275-2295):
--   EFFECT_0 DUMMY. The caster's Flame Shock on the proc target gains
--   the effect amount in seconds on its remaining duration, max duration
--   growing past the old max — no Lua ev6 twin: Lua GetAuraEffect
--   returns an amount, not a mutable aura, and has no family-flag lookup.
--   spell_sha_t10_restoration_4p_bonus (70808, spell_shaman.cpp:2320-2342):
--   EFFECT_0 DUMMY. On a heal proc the actor casts Chained Heal (70809)
--   on the proc target with BP0 = pct of the heal by the effect amount,
--   divided by the triggered spell's max ticks — no Lua ev6 twin: ev6
--   carries no heal amounts.
--
-- CLOSED (tick 295 census, all 50 loader classes, minus rows 266-275
--   ported above): the remaining stay-outs — proc arms (ancestral
--   awakening, earth shield, earthliving, flametongue, frozen power,
--   glyphs, imp water shield, lightning overload/shield, nature
--   guardian, static shock, windfury),
--   absorb/calc arms (shadow ward twin, rend twin), SpellScript-only
--   (chain heal, cleansing pulse, earthen power, fire nova,
--   healing/mana spring totem, heroism, lava lash, stoneclaw,
--   thunderstorm), AfterDispel (flame shock), and the t3 8p
--   self-remove (family-flag GetAuraEffect overload unbridged).
--   Astral shift aura is proc, not the visual dummy above; the 51474
--   absorb leg is Go-ported above.
--
-- PORTED (census rows 758-789, RegisterAuraEvent OnProc = 6,
--   RegisterSpellEvent OnEffectHitTarget = 5): supersedes the tick-295
--   closure for ancestral awakening, earth shield, lightning shield,
--   healing stream totem, and mana spring totem (re-verified against
--   the current runtime):
--   spell_sha_earth_shield::HandleProc (spell_shaman.cpp:437-447):
--   EFFECT_0 DUMMY, rank 1 (974) only. On proc the holder self-casts
--   Earth Shield Heal (379), triggered, BP0 = the effect amount.
--   EFFECT_1 (the +healing bonus, MOD_HEALING_PCT 118) is not a
--   proc-trigger aura type, so the hook fires once per proc.
--   spell_sha_lightning_shield::HandleProc (spell_shaman.cpp:1531-1537):
--   EFFECT_0 PROC_TRIGGER_SPELL, rank 1 (324) only — 26364 is the
--   damage chain's first rank, not the aura. On proc the attacker casts
--   the rank-matched damage spell (aura:GetSpellWithRank(26364)) on the
--   holder, triggered.
--   spell_sha_ancestral_awakening::HandleProc
--   (spell_shaman.cpp:105-143, census row 758): EFFECT_0 DUMMY.
--   PreventDefaultAction (the true return); a nil/zero heal returns;
--   the actor casts the dummy 52759 on itself (the C++ nil-target
--   cast), triggered, BP0 = pct of the heal by the effect amount.
--   Direct heals only (no periodic-heal proc path forwards a heal
--   to ev6); the 52759 lowest-health-only FilterTargets arm is
--   unmodeled on the player path (pre-existing Go deviation,
--   inherited); the creature path has no 52759 dummy twin. The
--   tick-295 stay-out ("ev6 forwards damage only") is superseded:
--   fireAuraProc passes procEvent with GetHeal (lua_aura.go:1273).
--   DORMANT until Spell.dbc ships the 51556 proc entry and the
--   52759/52752 rows.
--   spell_sha_healing_stream_totem::HandleDummy
--   (spell_shaman.cpp:1056-1085): all 9 ranks, EFFECT_0 DUMMY. Each hit
--   unit is healed by 52042 with BP0 = the effect value, raised by the
--   owner's Restorative Totems dummy (icon 338, EFFECT_1) and Glyph of
--   Healing Stream Totem (55456) amounts; the totem-owner leg rides
--   caster:GetOwner(), the GetAffectingPlayer twin.
--   spell_sha_mana_spring_totem::HandleDummy
--   (spell_shaman.cpp:1610-1619): all 8 ranks, EFFECT_0 DUMMY. Each
--   mana-using hit unit is energized by 52032 with BP0 = the effect
--   value. Both totem ports follow the mana-tide-totem ev5 precedent
--   (totem caster = creature path); the original-caster legs have no Go
--   model. Rank 2+ of the shield auras need spell_script_names /
--   spell_ranks data (not in-tree) to resolve.
--
-- STAY OUT (census rows 758-789, verified against the current runtime):
--   758 ancestral_awakening aura arm: PORTED below (ev6, this tick);
--   the tick-295 stay-out is superseded by the procEvent GetHeal
--   bridge. The 52759 proc arm is Go-ported (spells.go:28452,
--   53025); FilterTargets has no player-path hook (runtime.go,
--   OnObjectAreaTargetSelect).
--   759 astral_shift_aura — DoCheckProc only; ev6 carries no event
--   spell info / mechanic mask, and Lua has no CheckProc hook.
--   760 chain_heal — GetHitHeal/SetHitHeal have no Go model
--   (runtime.go); the Riptide leg needs the family-flag GetAuraEffect
--   overload (unbridged). (The old "player path fires no ev5" note
--   was wrong — the player path fires ev5 per effect/target.)
--   762 earthliving_weapon — CheckProc-only; GetCastItem and the proc
--   attack-type mask are unbridged, so the roll-only twin would
--   over-fire on off-hand / unenchanted attacks.
--   763 fire_nova — CheckCast PORTED ev8 (fireNovaCheckCast, this file):
--   fire totem via GetTotem(0) + GetSpellMaxRangeForTarget bridge
--   (lua_player.go). HandleDummy stays out: it needs spell-side
--   GetSpellWithRank (aura-side only); player-cast ev5 gap.
--   764 flame_shock — AfterDispel (mode 5) and GetDummyAuraEffect are
--   bridged, but the rank-matched Lava Flows triggered cast is not:
--   aura:GetSpellWithRank keys off the dispelled aura's rank, and the
--   64694-chain rank ids are not named in-tree.
--   765 flametongue_weapon — CheckProc needs GetCastItem + proc type
--   mask; HandleProc needs attack time / spellpower / aura-modifier
--   math and SetCastItem, all unbridged.
--   766 frozen_power — the min-distance gate reads SPELL_SHAMAN_FREEZE
--   (63685) EFFECT_0 CalcValue; no bridge reads another spell's effect
--   value (GetDistance itself is bridged).
--   776 imp_water_shield — CheckProc needs family flags; HandleProc
--   needs the family-flag GetAuraEffect overload to find Water Shield
--   and read its trigger spell.
--   777 lightning_overload — HandleProc branches on family flags and
--   ChainTarget, neither forwarded to ev6.
--   787 nature_guardian — ev6 fires once per eligible proc effect with
--   no effect-index arg, and EFFECT_1 is a DUMMY threshold carrier (in
--   isProcTriggerAuraType), so the EFFECT_0-only arm would double-fire
--   (two 31616 heals + two 39301 threats per proc). Ranks 2-5 also
--   unsourced (30882 is the twilight egg NPC entry).
--   788 static_shock — needs the family-flag GetAuraEffect overload
--   and DropCharge; neither is bridged.
--   789 thunderstorm — player-cast ev5 gap: PreventHitDefaultEffect has
--   a real veto record on the creature-path spell object
--   (lua_spell.go:455), but the hook never fires for player casts.
--   Census go_hit/lua_hit for rows 760/761/763/764/765/776/778 are
--   phantoms (no real engine/Lua hits for 1064/974/1535/8050/10400/
--   16180/26364); the census hooks for 759, 767-775, and 779-786 list
--   arms the C++ classes never register.
--
-- BRIDGE CORRECTIONS found this tick: (1) ev6 has a creature-side fire
--   site (boss_ai.go:10894, the creature proc loop twin) — the
--   runtime.go "session-side only" line is stale. (2) aura:GetSpellWithRank
--   is live (lua_aura.go:421) and keys off the aura's own rank.

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

-- C++ spell_sha_earthen_power::FilterTargets (spell_shaman.cpp:540-565):
-- OnObjectAreaTargetSelect, EFFECT_0 TARGET_UNIT_SRC_AREA_ALLY — drop every
-- area ally without a snare-mechanic aura (EarthenPowerTargetSelector:
-- remove_if !ToUnit or !HasAuraWithMechanic(1 << MECHANIC_SNARE)).
-- The totem's triggered cast rides the creature-cast path (boss_ai.go:3218),
-- so ev7 fires; the filter only removes targets, so it cannot double-fire.
-- DORMANT until Spell.dbc ships the 59566 record.
local MECHANIC_SNARE_MASK = 2048 -- 1 << MECHANIC_SNARE (11), SharedDefines.h:1368
local function earthenPowerFilter(event, spell, effectIndex, targets)
    if effectIndex ~= 0 then
        return
    end
    for _, target in ipairs(targets:GetTargets()) do
        if target == nil or not target:HasAuraWithMechanic(MECHANIC_SNARE_MASK) then
            targets:Remove(target)
        end
    end
end
RegisterSpellEvent(SPELL_SHAMAN_TOTEM_EARTHEN_POWER, 7, earthenPowerFilter)

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

local SPELL_SHAMAN_SENTRY_TOTEM = 6495 -- spell_shaman.cpp:1767 (class spell_sha_sentry_totem)
local SPELL_SHAMAN_SPIRIT_HUNT = 58877 -- spell_shaman.cpp:1886 (class spell_sha_spirit_hunt)
local SPELL_SHAMAN_SPIRIT_HUNT_HEAL = 58879 -- spell_shaman.cpp:79
local SPELL_SHAMAN_TOTEMIC_MASTERY_AURA = 38443 -- spell_shaman.cpp:2036 (class spell_sha_totemic_mastery)
local SPELL_SHAMAN_TOTEMIC_MASTERY = 38437 -- spell_shaman.cpp:67
local SPELL_AURA_BIND_SIGHT = 1 -- SpellAuraDefines.h:81 (spells.go:644)
-- Totem slots are 0..3 Fire/Earth/Water/Air (totems.go:18-21); C++
-- SUMMON_SLOT_TOTEM_FIRE..AIR are 1..4 (PET is 0).
local TOTEM_SLOT_FIRE = 0
local TOTEM_SLOT_AIR = 3

-- C++ spell_sha_sentry_totem_AuraScript::AfterRemove
-- (spell_shaman.cpp:1791-1796): EFFECT_1 DUMMY, AURA_EFFECT_HANDLE_REAL.
-- The player caster stops bind sight, so the bind-sight aura comes off the
-- air totem. On unsummon the slot is already cleared and the handler no-ops;
-- the Go teardown in destroyTotemGUID owns that path. DORMANT until
-- Spell.dbc ships the 6495 row.
local function sentryTotemRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    local totem = caster:GetTotem(TOTEM_SLOT_AIR)
    if totem == nil then
        return
    end
    totem:RemoveAurasByType(SPELL_AURA_BIND_SIGHT)
end

RegisterAuraEvent(SPELL_SHAMAN_SENTRY_TOTEM, 3, sentryTotemRemove)

-- C++ spell_sha_spirit_hunt_AuraScript::HandleProc
-- (spell_shaman.cpp:1905-1919): EFFECT_0 DUMMY. On a damaging proc the pet
-- actor casts Spirit Hunt Heal (58879) on itself and on its owner, both
-- triggered with BP0 = pct of the damage by the effect amount. The
-- zero-damage gate is the C++ null/zero-damage early return; a missing
-- owner skips both casts. Returns true (PreventDefaultAction) so the proc
-- loop skips the default leg. DORMANT until Spell.dbc ships the 58877 proc
-- entry; the pet melee proc path carries the damage, the pet spell path
-- does not (pre-existing engine gap).
local function spiritHuntProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 or actor == nil then
        return
    end
    local owner = actor:GetOwner()
    if owner == nil then
        return
    end
    local bp0 = math.floor(damage * aura:GetAmount(0) / 100)
    actor:CastCustomSpell(actor, SPELL_SHAMAN_SPIRIT_HUNT_HEAL, true, bp0)
    actor:CastCustomSpell(owner, SPELL_SHAMAN_SPIRIT_HUNT_HEAL, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_SHAMAN_SPIRIT_HUNT, 6, spiritHuntProc)

-- C++ spell_sha_totemic_mastery_AuraScript::HandleDummy
-- (spell_shaman.cpp:2048-2061): EFFECT_0 PERIODIC_TRIGGER_SPELL. Each tick
-- the handler checks all four totem slots; with any slot empty it returns
-- and the Go default trigger cast fires; with all four filled it casts
-- Totemic Mastery (38437) itself, triggered, and returns true to veto the
-- default. DORMANT until Spell.dbc ships the 38443 row.
local function totemicMasteryPeriodic(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    for slot = TOTEM_SLOT_FIRE, TOTEM_SLOT_AIR do
        if target:GetTotem(slot) == nil then
            return
        end
    end
    target:CastSpell(target, SPELL_SHAMAN_TOTEMIC_MASTERY, true)
    return true
end

RegisterAuraEvent(SPELL_SHAMAN_TOTEMIC_MASTERY_AURA, 1, totemicMasteryPeriodic)

local SPELL_SHAMAN_EARTH_SHIELD_HEAL = 379 -- spell_shaman.cpp:42
local SPELL_SHAMAN_EARTH_SHIELD = 974 -- spell_shaman.cpp:422 (class spell_sha_earth_shield)
local SPELL_SHAMAN_LIGHTNING_SHIELD = 324 -- spell_shaman.cpp:1507 (class spell_sha_lightning_shield)
local SPELL_SHAMAN_LIGHTNING_SHIELD_R1 = 26364 -- spell_shaman.cpp:58 (damage chain first rank)
local SPELL_SHAMAN_TOTEM_HEALING_STREAM_HEAL = 52042 -- spell_shaman.cpp:66
local SPELL_SHAMAN_GLYPH_OF_HEALING_STREAM_TOTEM = 55456 -- spell_shaman.cpp:50
local SPELL_SHAMAN_MANA_SPRING_TOTEM_ENERGIZE = 52032 -- spell_shaman.cpp:59
local SHAMAN_ICON_ID_RESTORATIVE_TOTEMS = 338 -- spell_shaman.cpp:99
local SPELL_SHAMAN_HEALING_STREAM_TOTEM_SPELLS = { 52041, 52046, 52047, 52048, 52049, 52050, 58759, 58760, 58761 } -- spell_shaman.cpp:1042
local SPELL_SHAMAN_MANA_SPRING_TOTEM_SPELLS = { 52031, 52033, 52034, 52035, 52036, 58778, 58779, 58780 } -- spell_shaman.cpp:1595

-- C++ spell_sha_earth_shield_AuraScript::HandleProc
-- (spell_shaman.cpp:437-447): EFFECT_0 DUMMY. On proc the holder
-- self-casts Earth Shield Heal (379), triggered, with BP0 = the effect
-- amount; the OriginalCaster leg (GetCasterGUID) has no Go model.
-- Returns true (PreventDefaultAction). DORMANT until Spell.dbc ships
-- the 974 aura + proc records. Rank 1 only: ranks 2-5 need the
-- spell_script_names / spell_ranks data (not in-tree) to resolve.
local function earthShieldProc(event, aura, target, actor, procTarget, damage)
    if target == nil then
        return true
    end
    local amount = aura:GetAmount(0)
    if amount == nil or amount == 0 then
        return true
    end
    target:CastCustomSpell(target, SPELL_SHAMAN_EARTH_SHIELD_HEAL, true, amount)
    return true
end

RegisterAuraEvent(SPELL_SHAMAN_EARTH_SHIELD, 6, earthShieldProc)

-- C++ spell_sha_lightning_shield_AuraScript::HandleProc
-- (spell_shaman.cpp:1531-1537): EFFECT_0 PROC_TRIGGER_SPELL. On proc
-- the holder (action target) casts the rank-matched Lightning Shield
-- damage spell (aura:GetSpellWithRank(26364), the damage chain's first
-- rank) on the attacker (actor), triggered. The CheckProc gate
-- (non-nil action target) is the target ~= nil early return; a nil
-- actor vetoes the default without casting, like a failed CheckProc.
-- A rank miss also vetoes: C++ PreventDefaultAction()s before the
-- rank lookup, so the default never fires. Returns true
-- (PreventDefaultAction). DORMANT until Spell.dbc ships the 324 aura +
-- proc records. Rank 1 only: ranks 2-13 need the spell_script_names /
-- spell_ranks data (not in-tree) to resolve.
local function lightningShieldProc(event, aura, target, actor, procTarget, damage)
    if actor == nil or target == nil then
        return true
    end
    local rankSpell = aura:GetSpellWithRank(SPELL_SHAMAN_LIGHTNING_SHIELD_R1)
    if rankSpell == 0 then
        return true
    end
    target:CastSpell(actor, rankSpell, true)
    return true
end

RegisterAuraEvent(SPELL_SHAMAN_LIGHTNING_SHIELD, 6, lightningShieldProc)

-- C++ spell_sha_healing_stream_totem_SpellScript::HandleDummy
-- (spell_shaman.cpp:1056-1085): EFFECT_0 DUMMY. Each hit unit is healed
-- by Totem Healing Stream Heal (52042) with BP0 = the effect value,
-- raised by the Restorative Totems dummy aura (icon 338, EFFECT_1) and
-- the Glyph of Healing Stream Totem (55456, EFFECT_0) amounts on the
-- totem's owner (caster:GetOwner(), the GetAffectingPlayer twin).
-- Non-triggered cast; the original-caster leg has no Go model.
-- DORMANT until Spell.dbc ships the totem pulse rows and 52042.
local function healingStreamTotemHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    local damage = spell:GetEffectValue(0)
    if damage <= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local owner = caster:GetOwner()
    if owner ~= nil then
        local restorative = owner:GetDummyAuraEffect(SPELLFAMILY_SHAMAN, SHAMAN_ICON_ID_RESTORATIVE_TOTEMS, 1)
        if restorative ~= nil then
            damage = damage + math.floor(damage * restorative / 100)
        end
        local glyph = owner:GetAuraEffect(SPELL_SHAMAN_GLYPH_OF_HEALING_STREAM_TOTEM, 0)
        if glyph ~= nil then
            damage = damage + math.floor(damage * glyph / 100)
        end
    end
    caster:CastCustomSpell(hitTarget, SPELL_SHAMAN_TOTEM_HEALING_STREAM_HEAL, false, damage)
end

for _, spellId in ipairs(SPELL_SHAMAN_HEALING_STREAM_TOTEM_SPELLS) do
    RegisterSpellEvent(spellId, 5, healingStreamTotemHit)
end

-- C++ spell_sha_mana_spring_totem_SpellScript::HandleDummy
-- (spell_shaman.cpp:1610-1619): EFFECT_0 DUMMY. Each mana-using hit unit
-- is energized by Mana Spring Totem Energize (52032) with BP0 = the
-- effect value. Non-triggered cast; the original-caster leg has no Go
-- model. DORMANT until Spell.dbc ships the totem pulse rows and 52032.
local function manaSpringTotemHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetPowerType() ~= POWER_MANA then
        return
    end
    local effValue = spell:GetEffectValue(0)
    if effValue <= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastCustomSpell(hitTarget, SPELL_SHAMAN_MANA_SPRING_TOTEM_ENERGIZE, false, effValue)
end

for _, spellId in ipairs(SPELL_SHAMAN_MANA_SPRING_TOTEM_SPELLS) do
    RegisterSpellEvent(spellId, 5, manaSpringTotemHit)
end

local SPELL_SHAMAN_FIRE_NOVA_R1 = 1535 -- spell_shaman.cpp:626 (class spell_sha_fire_nova)
local SPELL_SHAMAN_ANCESTRAL_AWAKENING = 51556 -- spell_shaman.cpp:105 (class spell_sha_ancestral_awakening)
local SPELL_SHAMAN_ANCESTRAL_AWAKENING_DUMMY = 52759 -- spell_shaman.cpp:37
local SPELL_FAILED_OUT_OF_RANGE = 97 -- SharedDefines.h:1079
local SPELL_FAILED_CUSTOM_ERROR = 172 -- SharedDefines.h:1154
local SPELL_CUSTOM_ERROR_MUST_HAVE_FIRE_TOTEM = 83 -- SharedDefines.h:1259
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171 (custom value, never sent to the client)

-- C++ spell_sha_fire_nova_SpellScript::CheckFireTotem
-- (spell_shaman.cpp:626-641): the fire totem (m_SummonSlot[1]) must exist
-- else SPELL_FAILED_CUSTOM_ERROR with MUST_HAVE_FIRE_TOTEM; beyond the
-- spell's max range for the totem it fails SPELL_FAILED_OUT_OF_RANGE.
-- GetDistance is the generic-fallback WorldObject::GetDistance
-- (object.go:282: raw center-to-center 3D; the C++ reach subtraction
-- is unmodeled on generic objects, so the totem's reach term rides
-- inside GetSpellMaxRangeForTarget's maxRange instead, and the
-- caster's reach is the residual gap vs C++ IsWithinDistInMap).
-- RegisterSpellEvent OnCheckCast = 8 (lua_spell.go fireSpellCheckCast).
-- Binding 1535 is the AC spell_script_names row (R1 only). The HandleDummy
-- leg stays out: it casts the rank-matched triggered nova via
-- GetSpellWithRank, which exists only on the aura surface.
local function fireNovaCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return SPELL_CAST_OK
    end
    local totem = caster:GetTotem(TOTEM_SLOT_FIRE)
    if totem == nil then
        return SPELL_FAILED_CUSTOM_ERROR, SPELL_CUSTOM_ERROR_MUST_HAVE_FIRE_TOTEM
    end
    local maxRange = caster:GetSpellMaxRangeForTarget(totem, SPELL_SHAMAN_FIRE_NOVA_R1)
    if maxRange ~= nil and caster:GetDistance(totem) > maxRange then
        return SPELL_FAILED_OUT_OF_RANGE
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_SHAMAN_FIRE_NOVA_R1, 8, fireNovaCheckCast)

-- C++ spell_sha_ancestral_awakening_AuraScript::HandleProc
-- (spell_shaman.cpp:110-143): OnEffectProc, EFFECT_0 DUMMY.
-- PreventDefaultAction (the true return); a nil/zero heal returns;
-- the actor casts the dummy 52759 on itself (the C++ nil-target
-- cast), triggered, BP0 = pct of the heal by the effect amount.
-- Direct heals only (no periodic-heal proc path forwards a heal to
-- ev6); the 52759 lowest-health-only FilterTargets arm is unmodeled
-- on the player path (pre-existing Go deviation, inherited); the
-- creature path has no 52759 dummy twin. DORMANT until Spell.dbc
-- ships the 51556 proc entry and the 52759/52752 rows.
local function ancestralAwakeningProc(event, aura, target, actor, procTarget, damage, procEvent)
    if actor == nil or procEvent == nil then
        return true
    end
    local heal = procEvent:GetHeal()
    if heal == nil or heal == 0 then
        return true
    end
    local amount = aura:GetAmount(0)
    if amount == nil or amount == 0 then
        return true
    end
    local bp0 = math.floor(heal * amount / 100)
    actor:CastCustomSpell(actor, SPELL_SHAMAN_ANCESTRAL_AWAKENING_DUMMY, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_SHAMAN_ANCESTRAL_AWAKENING, 6, ancestralAwakeningProc)
