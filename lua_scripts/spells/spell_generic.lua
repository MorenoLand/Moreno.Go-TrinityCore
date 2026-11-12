-- Generic spell scripts — Lua port of
-- src/server/scripts/Spells/spell_generic.cpp
-- (spell_gen_aura_of_fear only; the rest of the file's scripts are
-- not ported here).

-- C++ spell_gen_aura_of_fear::PeriodicTick (spell_generic.cpp:391-412):
-- 28313 EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL.
-- PreventDefaultAction; roll_chance_i(GetSpellInfo()->ProcChance)
-- else return; the aura target casts the DBC trigger of EFFECT_0
-- (aura:GetTriggerSpell(0)) triggered on nil. The roll is
-- roll_chance_i-exact: chance > irand(0, 99) (Random.h:59-62), so
-- the gate passes while math.random(0, 99) < ProcChance. Returns
-- true, the PreventDefaultAction twin, so the Go default trigger
-- cast does not also fire.
local SPELL_AURA_OF_FEAR = 28313

local function auraOfFearTick(event, aura, target)
    if math.random(0, 99) >= aura:GetProcChance() then
        return true
    end
    local trigger = aura:GetTriggerSpell(0)
    if trigger ~= 0 then
        target:CastSpell(nil, trigger, true)
    end
    return true
end

RegisterAuraEvent(SPELL_AURA_OF_FEAR, 1, auraOfFearTick)

-- C++ spell_gen_remove_on_health_pct::PeriodicTick
-- (spell_generic.cpp:2955-2969): EFFECT_0 SPELL_AURA_PERIODIC_
-- DAMAGE on 38772 / 43937 / 62331 / 62418 (Grievous Wound,
-- Impale). When the target is healed above the EFFECT_1 CalcValue
-- health percent, the aura is removed and the tick's default damage
-- is suppressed (return true is the PreventDefaultAction twin).
-- The pct guard (nonzero) is the GetSpellEffectCalcValue
-- convention: with no Spell.dbc loaded the read is 0 and the
-- handler no-ops instead of stripping the aura on every tick.
local SPELL_REMOVE_ON_HEALTH_PCT = { 38772, 43937, 62331, 62418 }

local function removeOnHealthPctTick(event, aura, target)
    local pct = aura:GetSpellEffectCalcValue(aura:GetAuraId(), 1)
    if pct ~= 0 and target:HealthAbovePct(pct) then
        aura:Remove()
        return true
    end
end

for _, spellId in ipairs(SPELL_REMOVE_ON_HEALTH_PCT) do
    RegisterAuraEvent(spellId, 1, removeOnHealthPctTick)
end

-- C++ spell_gen_remove_on_full_health_pct::PeriodicTick
-- (spell_generic.cpp:3003-3015): EFFECT_2 SPELL_AURA_PERIODIC_
-- DAMAGE_PERCENT on 70292 / 71316 / 71317 (Glacial Strike). When
-- the target is at full health the aura is removed and the tick's
-- default damage is suppressed (return true).
local SPELL_REMOVE_ON_FULL_HEALTH_PCT = { 70292, 71316, 71317 }

local function removeOnFullHealthPctTick(event, aura, target)
    if target:IsFullHealth() then
        aura:Remove()
        return true
    end
end

for _, spellId in ipairs(SPELL_REMOVE_ON_FULL_HEALTH_PCT) do
    RegisterAuraEvent(spellId, 1, removeOnFullHealthPctTick)
end

-- C++ spell_gen_remove_on_full_health::PeriodicTick
-- (spell_generic.cpp:2950-2979): EFFECT_0 SPELL_AURA_PERIODIC_
-- DAMAGE on 31956 / 38801 / 43093 / 58517 / 59262 (Grievous Wound
-- / Grievous Throw). When the target is at full health the aura is
-- removed and the tick's default damage is suppressed (return true).
-- A single-effect spell is allowed one tick before the check fires
-- (onlyEffect = no EFFECT_1 and no EFFECT_2, read via the new
-- GetSpellEffectIsEffect bridge; with no Spell.dbc loaded both
-- slots read absent, so the first tick is skipped — the safe
-- degraded read).
local SPELL_REMOVE_ON_FULL_HEALTH = { 31956, 38801, 43093, 58517, 59262 }

local function removeOnFullHealthTick(event, aura, target)
    local spellId = aura:GetAuraId()
    local onlyEffect = not aura:GetSpellEffectIsEffect(spellId, 1)
        and not aura:GetSpellEffectIsEffect(spellId, 2)
    if onlyEffect and aura:GetTickNumber() <= 1 then
        return
    end
    if target:IsFullHealth() then
        aura:Remove()
        return true
    end
end

for _, spellId in ipairs(SPELL_REMOVE_ON_FULL_HEALTH) do
    RegisterAuraEvent(spellId, 1, removeOnFullHealthTick)
end

-- C++ spell_gen_choking_vines::HandleChoke
-- (spell_generic.cpp:968-974): EFFECT_1 SPELL_AURA_PERIODIC_
-- DAMAGE on 35244 (Choking Vines). When the aura reaches its DBC
-- max stack count it casts SPELL_CHOKING_WOUND 35247 on the target
-- (nil target = self, triggered) and removes itself. No
-- PreventDefaultAction, so the Go default damage tick still fires.
-- The max-stack guard (nonzero) is the GetSpellStackAmount
-- convention: with no Spell.dbc loaded the read is 0 and the
-- handler no-ops instead of firing the wound on every tick.
local SPELL_CHOKING_VINES = 35244
local SPELL_CHOKING_WOUND = 35247

local function chokingVinesTick(event, aura, target)
    local maxStacks = aura:GetSpellStackAmount(aura:GetAuraId())
    if maxStacks == 0 then
        return
    end
    if aura:GetStackAmount() ~= maxStacks then
        return
    end
    target:CastSpell(nil, SPELL_CHOKING_WOUND, true)
    aura:Remove()
end

RegisterAuraEvent(SPELL_CHOKING_VINES, 1, chokingVinesTick)

-- C++ spell_gen_decay_over_time_fungal_decay::ModDuration
-- (spell_generic.cpp:1377-1385): EFFECT_0 MOD_DECREASE_SPEED. On
-- apply, when the remaining duration still equals the max (a fresh
-- apply / reapply, not a stack decay), both are rewritten to
-- AURA_DURATION 12600 (enum FungalDecay, spell_generic.cpp:1368;
-- "found in sniffs, there is no duration entry we can possibly use").
-- Deviation: C++ registers REAL_OR_REAPPLY, so a reapply re-runs the
-- check; the Go OnApply hook fires on fresh applies only, so a
-- reapply keeps the old durations.
local SPELL_FUNGAL_DECAY = 32065
local AURA_DURATION_FUNGAL_DECAY = 12600 -- spell_generic.cpp:1368

local function fungalDecayApply(event, aura, target)
    if aura:GetDuration() == aura:GetMaxDuration() then
        aura:SetMaxDuration(AURA_DURATION_FUNGAL_DECAY)
        aura:SetDuration(AURA_DURATION_FUNGAL_DECAY)
    end
end

RegisterAuraEvent(SPELL_FUNGAL_DECAY, 2, fungalDecayApply)

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnCheckCast = 8):
--   spell_gen_allow_cast_from_item_only::CheckRequirement
--   (spell_generic.cpp:155-160): OnCheckCast. The cast fails
--   SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW (173) unless it carries a
--   cast item (spell:GetCastItem() non-nil, lua_spell.go:294;
--   creature path reads nil, so a creature cast fails closed like
--   C++ GetCastItem() on a non-item cast). Binding: the five
--   Adaptive Warding trigger spells (enum spell_generic.cpp:72-76),
--   cast triggered by the 28764 aura proc (which skips CheckCast in
--   both C++ and Go), so the gate only bites on direct casts.
--   Binding provenance: oncheckcast triage row 18 (spell_script_names
--   reading); local world.db is 0-byte, so the DB row itself is not
--   in-tree verifiable — the five IDs are the C++ enum values.

local SPELL_GEN_ADAPTIVE_WARDING_SPELLS = { 28765, 28766, 28768, 28769, 28770 } -- spell_generic.cpp:72-76
local SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW = 173 -- SharedDefines.h
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171

local function allowCastFromItemOnlyCheckCast(event, spell)
    if spell:GetCastItem() == nil then
        return SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW
    end
    return SPELL_CAST_OK
end

for _, spellId in ipairs(SPELL_GEN_ADAPTIVE_WARDING_SPELLS) do
    RegisterSpellEvent(spellId, 8, allowCastFromItemOnlyCheckCast)
end

-- PORTED (tick 2026-11-11, RegisterSpellEvent AfterHit = 3):
--   spell_gen_clear_fear_poly::HandleAfterHit
--   (spell_generic.cpp:952-966): AfterHit, once per hit target
--   (Spell.cpp:2659, CallScriptAfterHitHandlers in
--   TargetInfo::DoDamageAndTriggers; Go fires ev3 per hit target on
--   both paths, spells.go:19713 / boss_ai.go:4245). The hit unit
--   loses every aura whose mechanic mask intersects FEAR or
--   POLYMORPH (Unit::RemoveAurasWithMechanic, Unit.cpp:4217-4232;
--   the unit:RemoveAurasWithMechanic bridge, player + creature
--   surfaces). Binding: spell 8129 Mana Burn, whole script (only
--   arm); spell_script_names row not in-tree verifiable (local
--   world.db is 0-byte), same caveat as the Adaptive Warding port
--   above.

local SPELL_MANA_BURN = 8129 -- spell_generic.cpp:952
local MECHANIC_FEAR = 5 -- SharedDefines.h:1362
local MECHANIC_POLYMORPH = 17 -- SharedDefines.h:1374

local function manaBurnAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    hitTarget:RemoveAurasWithMechanic((2 ^ MECHANIC_FEAR) + (2 ^ MECHANIC_POLYMORPH))
end

RegisterSpellEvent(SPELL_MANA_BURN, 3, manaBurnAfterHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_despawn_self::HandleDummy (spell_generic.cpp:1473-1494):
--   OnEffectHitTarget, EFFECT_ALL, SPELL_EFFECT_ANY. Load gate is
--   caster TYPEID_UNIT (GetTypeId()==3, checked in the handler since
--   Lua has no Load hook); when the hit effect is SPELL_EFFECT_DUMMY
--   (3) or SPELL_EFFECT_SCRIPT_EFFECT (77) the caster despawns
--   (GetCaster()->ToCreature()->DespawnOrUnsummon()). The effect-kind
--   read is the new creature-path spell:GetEffectType(effectIndex)
--   bridge (lua_spell.go, GetTriggerSpell twin). Bindings 25952 /
--   42268 / 52267 share the class (AC 'spell_gen_despawn_self';
--   TDB 335.26091 renames 42268 to
--   'spell_fjord_mindless_abomination_explosion_fx_master').
local SPELL_GEN_DESPAWN_SELF_1 = 25952 -- spell_generic.cpp:1473
local SPELL_GEN_DESPAWN_SELF_2 = 42268
local SPELL_GEN_DESPAWN_SELF_3 = 52267
local SPELL_EFFECT_DUMMY = 3 -- SharedDefines.h:814
local SPELL_EFFECT_SCRIPT_EFFECT = 77 -- SharedDefines.h:888

local function despawnSelfHit(event, spell, hitTarget, effectIndex)
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 3 then
        return
    end
    local effectType = spell:GetEffectType(effectIndex)
    if effectType == SPELL_EFFECT_DUMMY or effectType == SPELL_EFFECT_SCRIPT_EFFECT then
        caster:DespawnOrUnsummon()
    end
end

RegisterSpellEvent(SPELL_GEN_DESPAWN_SELF_1, 5, despawnSelfHit)
RegisterSpellEvent(SPELL_GEN_DESPAWN_SELF_2, 5, despawnSelfHit)
RegisterSpellEvent(SPELL_GEN_DESPAWN_SELF_3, 5, despawnSelfHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnCheckCast = 8 /
-- RegisterSpellEvent AfterHit = 3):
--   spell_gen_bandage::CheckCast + HandleScript
--   (spell_generic.cpp:495-525): CheckCast fails
--   SPELL_FAILED_TARGET_AURASTATE (111, SharedDefines.h:1093) when
--   the explicit target already carries SPELL_RECENTLY_BANDAGED
--   (11196); AfterHit the caster casts 11196 on the hit unit,
--   triggered. Bindings 23567 / 23568 / 23569 / 23696 / 24412 /
--   24413 / 24414 / 35207 share the class (AC
--   'spell_gen_bandage'; no rows for these IDs in either TDB —
--   AC-only bindings, from the divergence triage).
local SPELL_GEN_BANDAGE = { 23567, 23568, 23569, 23696, 24412, 24413, 24414, 35207 } -- spell_generic.cpp:495
local SPELL_RECENTLY_BANDAGED = 11196 -- spell_generic.cpp:492
local SPELL_FAILED_TARGET_AURASTATE = 111 -- SharedDefines.h:1093
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171

local function bandageCheckCast(event, spell)
    local target = spell:GetTarget()
    if target ~= nil and target:HasAura(SPELL_RECENTLY_BANDAGED) then
        return SPELL_FAILED_TARGET_AURASTATE
    end
    return SPELL_CAST_OK
end

local function bandageAfterHit(event, spell, hitTarget)
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(hitTarget, SPELL_RECENTLY_BANDAGED, true)
end

for _, spellId in ipairs(SPELL_GEN_BANDAGE) do
    RegisterSpellEvent(spellId, 8, bandageCheckCast)
    RegisterSpellEvent(spellId, 3, bandageAfterHit)
end

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_elune_candle::HandleScript
--   (spell_generic.cpp:1595-1645): EFFECT_0 DUMMY. The caster casts
--   on the hit unit, triggered: a random one of the four Omen
--   spells (urand(0, 3)) when the hit unit is Omen (entry 15467),
--   else the normal candle spell. Binding 26374 (AC
--   'spell_gen_elune_candle'; TDB 26091 renames it
--   'spell_lunar_festival_elune_candle').
local SPELL_ELUNE_CANDLE = 26374 -- spell_generic.cpp:1595
local NPC_OMEN = 15467 -- spell_generic.cpp:1585
local SPELL_ELUNE_CANDLE_NORMAL = 26636 -- spell_generic.cpp:1592
local SPELL_ELUNE_CANDLE_OMEN = { 26622, 26624, 26625, 26649 } -- spell_generic.cpp:1588-1591

local function eluneCandleHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local spellId = SPELL_ELUNE_CANDLE_NORMAL
    if hitTarget:GetEntry() == NPC_OMEN then
        spellId = SPELL_ELUNE_CANDLE_OMEN[math.random(1, 4)]
    end
    caster:CastSpell(hitTarget, spellId, true)
end

RegisterSpellEvent(SPELL_ELUNE_CANDLE, 5, eluneCandleHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnCheckCast = 8):
--   spell_gen_arcane_charge::CheckRequirement
--   (spell_generic.cpp:225-245): the cast is refused with
--   SPELL_FAILED_DONT_REPORT unless the explicit target unit is
--   a demon or undead (CREATURE_TYPEMASK_DEMON_OR_UNDEAD, the
--   (1 << (type - 1)) mask of Unit::GetCreatureTypeMask); no
--   explicit target passes. GetCreatureType reads the template
--   type on creatures and the race/shapeshift type on players
--   (lua_creature_events.go:4862, lua_player.go:342), so the
--   same 3/6 comparison covers both. Binding 45072 (AC
--   'spell_gen_arcane_charge').
local SPELL_ARCANE_CHARGE = 45072 -- spell_generic.cpp:225
local SPELL_FAILED_DONT_REPORT = 27 -- SharedDefines.h:1009
local CREATURE_TYPE_DEMON = 3 -- UnitMethods.h:1109
local CREATURE_TYPE_UNDEAD = 6 -- UnitMethods.h:1112
local TYPEID_UNIT = 3
local TYPEID_PLAYER = 4

local function arcaneChargeCheckCast(event, spell)
    local target = spell:GetTarget()
    if target ~= nil then
        local typeId = target:GetTypeId()
        if typeId == TYPEID_UNIT or typeId == TYPEID_PLAYER then
            local creatureType = target:GetCreatureType()
            if creatureType ~= CREATURE_TYPE_DEMON and creatureType ~= CREATURE_TYPE_UNDEAD then
                return SPELL_FAILED_DONT_REPORT
            end
        end
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_ARCANE_CHARGE, 8, arcaneChargeCheckCast)

-- PORTED (this tick, RegisterAuraEvent OnCheckAreaTarget = 8):
--   spell_gen_av_drekthar_presence (45831, spell_generic.cpp:458-490):
--   DoCheckAreaTarget only — an area-aura target is accepted when its
--   entry is one of the five Alliance marshals (14762-14765, 11948
--   Vandar Stormspike) or the five Horde warmasters/Drek'thar
--   (14772, 14776, 14773, 14777, 11946); everything else is rejected.
--   The ev8 handler signature (event, aura, target) -> bool is the
--   thorim blizzard precedent (boss_thorim.lua). C++ receives Unit*
--   targets; players fall through to the default reject, so the Lua
--   guard rejects non-creatures the same way.
--   (AC sql:1223-1230 + TDB 26091 bind all eight presence ranks
--   45822/45823/45824/45826/45828/45829/45830/45831 to this one
--   class; the whitelist check is identical for each.)
local SPELL_AV_DREKTHAR_PRESENCE = {
    45822, 45823, 45824, 45826, 45828, 45829, 45830, 45831,
} -- spell_generic.cpp:458

local DREKTHAR_PRESENCE_ENTRIES = {
    [14762] = true, -- Dun Baldar North Marshal
    [14763] = true, -- Dun Baldar South Marshal
    [14764] = true, -- Icewing Marshal
    [14765] = true, -- Stonehearth Marshal
    [11948] = true, -- Vandar Stormspike
    [14772] = true, -- East Frostwolf Warmaster
    [14776] = true, -- Tower Point Warmaster
    [14773] = true, -- Iceblood Warmaster
    [14777] = true, -- West Frostwolf Warmaster
    [11946] = true, -- Drek'thar
}

local function drektharPresenceCheckAreaTarget(event, aura, target)
    if target == nil or target:GetObjectType() ~= "Creature" then
        return false
    end
    return DREKTHAR_PRESENCE_ENTRIES[target:GetEntry()] == true
end

for _, spellId in ipairs(SPELL_AV_DREKTHAR_PRESENCE) do
    RegisterAuraEvent(spellId, 8, drektharPresenceCheckAreaTarget)
end

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_eject_passenger (49259/49261, spell_generic.cpp:3631-3655):
--   EFFECT_0 SCRIPT_EFFECT. The hit unit's vehicle kit ejects the
--   passenger in seat (GetEffectValue() - 1) — the DBC CalcValue is
--   the 1-based seat (Validate requires >= 1). GetVehicleKit /
--   GetPassenger / ExitVehicle are all bridged on both surfaces
--   (lua_player.go:3183/3204, lua_creature_events.go:3387/3463,
--   lua_vehicle.go:74); GetEffectValue is bridged on the spell object
--   (lua_spell.go:215). A missing kit or empty seat is a no-op, as in
--   C++. Bindings 49259/49261 (AC 'spell_gen_eject_passenger').
local SPELL_EJECT_PASSENGER_1 = 49259 -- spell_generic.cpp:3631
local SPELL_EJECT_PASSENGER_2 = 49261 -- spell_generic.cpp:3631

local function ejectPassengerHit(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local kit = target:GetVehicleKit()
    if kit == nil then
        return
    end
    local passenger = kit:GetPassenger(spell:GetEffectValue(0) - 1)
    if passenger ~= nil then
        passenger:ExitVehicle()
    end
end

RegisterSpellEvent(SPELL_EJECT_PASSENGER_1, 5, ejectPassengerHit)
RegisterSpellEvent(SPELL_EJECT_PASSENGER_2, 5, ejectPassengerHit)

-- PORTED (this tick, RegisterAuraEvent OnPeriodic = 1):
--   spell_gen_burn_brutallus (46394, spell_generic.cpp:782-800):
--   OnEffectUpdatePeriodic only: every 11th tick doubles the
--   periodic-damage amount (GetTickNumber() % 11 == 0 ->
--   SetAmount(GetAmount() * 2)). The update-periodic hook has no
--   dedicated AuraEvent, but it is exactly reproducible on ev1:
--   C++ AuraEffect::Update does ++_ticksDone, then calls the
--   update-periodic handlers, then the periodic tick
--   (SpellAuraEffects.cpp:827-834), and Go ev1 fires before the
--   default tick with _ticksDone already incremented (spells.go
--   comment at the fire site) — so the ev1 handler sees the same
--   tick number and the doubled amount applies to the same tick.
--   This is the kalecgos precedent (boss_kalecgos.lua:951-959,
--   tick % 5 == 1 -> SetAmount(GetAmount() * 2); SetAmount
--   lua_aura.go:945, GetAmount :418). Binding 46394 (AC
--   'spell_gen_burn_brutallus').
local SPELL_BURN_BRUTALLUS = 46394 -- spell_generic.cpp:782

local function burnBrutallusTick(event, aura, target)
    if aura:GetTickNumber() % 11 == 0 then
        aura:SetAmount(aura:GetAmount() * 2)
    end
end

RegisterAuraEvent(SPELL_BURN_BRUTALLUS, 1, burnBrutallusTick)

-- PORTED (this tick, RegisterAuraEvent OnPeriodic = 1):
--   spell_gen_aura_of_anger (41337, spell_generic.cpp:369-385):
--   OnEffectUpdatePeriodic only: EFFECT_1 grows by 5 each tick
--   and EFFECT_0 is set to 100 * tick number. Same shape as the
--   burn_brutallus port above (46394): no dedicated update-periodic
--   AuraEvent, but C++ AuraEffect::Update does ++_ticksDone, then
--   calls the update-periodic handlers, then the periodic tick
--   (SpellAuraEffects.cpp:827-834), and Go ev1 fires before the
--   default tick with _ticksDone already incremented — so the ev1
--   handler sees the same tick number and the new EFFECT_0 amount
--   applies to the same tick's damage (SetAmount writes Amount,
--   lua_aura.go:945). The handler returns nothing: no
--   PreventDefaultAction in C++, the default periodic damage still
--   fires. Delta: C++ guards the EFFECT_1 leg on GetEffect(1)
--   non-nil; the DBC is not shipped in-tree so the effect's
--   presence is unverified — if EFFECT_1 is absent the
--   SetAmount(..., 1) write lands in an unread Amounts slot and
--   is inert. Binding 41337 (TDB 26091 'spell_reliquary_of_souls_
--   aura_of_anger'; the C++ class is spell_gen_aura_of_anger,
--   registered at spell_generic.cpp:4353).
local SPELL_AURA_OF_ANGER = 41337 -- spell_generic.cpp:369

local function auraOfAngerTick(event, aura, target)
    aura:SetAmount(aura:GetAmount(1) + 5, 1)
    aura:SetAmount(100 * aura:GetTickNumber(), 0)
end

RegisterAuraEvent(SPELL_AURA_OF_ANGER, 1, auraOfAngerTick)

-- PORTED (tick 2026-11-12, RegisterAuraEvent OnApply = 2 /
-- OnRemove = 3):
--   spell_gen_aura_service_uniform::OnApply/OnRemove
--   (spell_generic.cpp:422-456): EFFECT_0 SPELL_AURA_TRANSFORM.
--   On apply a player target takes the goblin service model
--   (male 31002 / female 31003 by native gender); on remove the
--   display is restored. RestoreDisplayId has no Go bridge: the
--   twin is DeMorph (lua_player.go:322, SetDisplayId of the native
--   display). Deviation: C++ RestoreDisplayId prefers a still-active
--   transform aura's model over the native display (Unit.cpp:10609);
--   Go restores the native display unconditionally. Binding 71450
--   (AC 'spell_gen_aura_service_uniform'; TDB 26091 renames it
--   'spell_love_is_in_the_air_service_uniform').
local SPELL_SERVICE_UNIFORM = 71450 -- spell_generic.cpp:415
local MODEL_GOBLIN_MALE = 31002 -- spell_generic.cpp:418
local MODEL_GOBLIN_FEMALE = 31003 -- spell_generic.cpp:419
local GENDER_MALE = 0 -- SharedDefines.h:76

local function serviceUniformApply(event, aura, target)
    if target == nil or target:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    if target:GetNativeGender() == GENDER_MALE then
        target:SetDisplayId(MODEL_GOBLIN_MALE)
    else
        target:SetDisplayId(MODEL_GOBLIN_FEMALE)
    end
end

local function serviceUniformRemove(event, aura, target)
    if target == nil or target:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    target:DeMorph()
end

RegisterAuraEvent(SPELL_SERVICE_UNIFORM, 2, serviceUniformApply)
RegisterAuraEvent(SPELL_SERVICE_UNIFORM, 3, serviceUniformRemove)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_clone (spell_generic.cpp:969-984): EFFECT_1 and
--   EFFECT_2 SCRIPT_EFFECT. PreventHitDefaultEffect, then the hit
--   unit casts the effect's value (DBC CalcValue, the spell to
--   mirror) on the caster, triggered. The creature-path spell object
--   carries the live veto record (lua_spell.go:513); GetEffectValue
--   is bridged on both spell builders (lua_spell.go:215/430).
--   Bindings 45204/45785/49889/50218/51719/57528 (AC + TDB 26091
--   agree on all six; 45785 is the Kil'jaeden sinister-reflection
--   clone, the renamed strict candidate).
local SPELL_GEN_CLONE = {
    45204, 45785, 49889, 50218, 51719, 57528,
}

local function cloneHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 1 and effectIndex ~= 2 then
        return
    end
    spell:PreventHitDefaultEffect(effectIndex)
    if hitTarget == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    hitTarget:CastSpell(caster, spell:GetEffectValue(effectIndex), true)
end

for _, spellId in ipairs(SPELL_GEN_CLONE) do
    RegisterSpellEvent(spellId, 5, cloneHit)
end

-- PORTED (this tick, RegisterAuraEvent PeriodicTick = 1):
--   spell_gen_restoration::PeriodicTick (spell_generic.cpp:2887-2905,
--   registered OnEffectPeriodic EFFECT_0 SPELL_AURA_PERIODIC_TRIGGER_SPELL
--   at :2909): each tick heals the holder for 10% of max health and
--   restores 10% of max mana (integer division; skipped when max mana
--   is 0, i.e. non-mana users). PreventDefaultAction is the true
--   return, suppressing the DBC trigger-spell default tick.
--   DELTAS: no HealBySpell/EnergizeBySpell bridge exists — the heal
--   rides SetHealth (clamped to max; skips heal procs and the combat
--   log, the spell_pet.lua precedent) and the mana rides ModifyPower
--   (delta, POWER_MANA) on player holders only; creature holders get
--   the heal leg but no mana leg (motion-creature ModifyPower takes
--   (type, delta) and the periodic target's builder is not settled).
local SPELL_GEN_RESTORATION = { 23493, 24379 }

local POWER_MANA = 0

local function restorationTick(event, aura, target)
    if target == nil then
        return true
    end
    local heal = target:CountPctFromMaxHealth(10)
    local health = target:GetHealth()
    local maxHealth = target:GetMaxHealth()
    if maxHealth > 0 and health < maxHealth then
        target:SetHealth(math.min(maxHealth, health + heal))
    end
    if target:GetObjectType() == "Player" then
        local maxMana = target:GetMaxPower(POWER_MANA)
        if maxMana ~= 0 then
            target:ModifyPower(math.floor(maxMana / 10), POWER_MANA)
        end
    end
    return true
end

for _, spellId in ipairs(SPELL_GEN_RESTORATION) do
    RegisterAuraEvent(spellId, 1, restorationTick)
end

-- PORTED (this tick, RegisterAuraEvent OnPeriodic = 1):
--   spell_gen_chains_of_ice::UpdatePeriodic (spell_generic.cpp:870-889):
--   EFFECT_1 PERIODIC_DUMMY. Each tick the slow (EFFECT_0) amount
--   grows by the periodic effect's amount, capped at 0
--   (min(sum, 0)); a missing EFFECT_0 returns early in C++.
--   OnEffectUpdatePeriodic has no Go twin, but it fires once per
--   periodic tick ahead of the tick in C++ (AuraEffect::Update),
--   which is exactly the ev1 fire site (runtime.go: the hook runs
--   before the default tick for every periodic aura type), so the
--   ramp rides ev1. No PreventDefaultAction in C++, so the handler
--   returns nothing and the Go default tick still fires. Bindings
--   45524/66020 per TDB 335.26091 spell_script_names
--   ('spell_gen_chains_of_ice'); AC binds 45524 to
--   'spell_dk_chains_of_ice', a loader name that exists nowhere in
--   the C++ tree — a dead row, not followed. DORMANT until Spell.dbc
--   ships the 45524/66020 periodic entries.
local SPELL_GEN_CHAINS_OF_ICE = { 45524, 66020 }

local function chainsOfIceTick(event, aura, target)
    local slow = aura:GetAmount(0)
    local step = aura:GetAmount(1)
    aura:SetAmount(math.min(slow + step, 0), 0)
end

for _, spellId in ipairs(SPELL_GEN_CHAINS_OF_ICE) do
    RegisterAuraEvent(spellId, 1, chainsOfIceTick)
end

-- PORTED (this tick, RegisterAuraEvent OnPeriodic = 1):
--   spell_gen_nitrous_boost::PeriodicTick (spell_generic.cpp:2456-2464):
--   EFFECT_1 PERIODIC_TRIGGER_SPELL. PreventDefaultAction (return
--   true); if the aura still has a caster and the target holds >= 10
--   mana, drain 10 mana, else the aura removes itself. The mana read
--   rides GetPower(POWER_MANA) (player surface, lua_player.go:1340;
--   the creature fallback reads the current-power field) and the
--   drain rides ModifyPower(-10, POWER_MANA).
--   DORMANT until Spell.dbc ships the 27746 periodic entry.
local SPELL_GEN_NITROUS_BOOST = 27746 -- spell_generic.cpp:2451 (class spell_gen_nitrous_boost)

local function nitrousBoostTick(event, aura, target)
    if target == nil then
        return true
    end
    if aura:GetCaster() ~= nil and target:GetPower(POWER_MANA) >= 10 then
        target:ModifyPower(-10, POWER_MANA)
    else
        aura:Remove()
    end
    return true
end

RegisterAuraEvent(SPELL_GEN_NITROUS_BOOST, 1, nitrousBoostTick)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_orc_disguise::HandleScript (spell_generic.cpp:2621-2631):
--   EFFECT_0 SCRIPT_EFFECT. On a player hit target the caster casts
--   the gendered disguise on the target, triggered: male 45760,
--   female 45762 (GetNativeGender; GENDER_MALE = 0, SharedDefines.h:76).
--   The player gate rides GetTypeId (TYPEID_PLAYER local in this file);
--   ev5 fires on the player path (spells.go:14735).
--   DORMANT until Spell.dbc ships the 45759 row.
local SPELL_GEN_ORC_DISGUISE = 45759 -- spell_generic.cpp:2605 (class spell_gen_orc_disguise)
local SPELL_ORC_DISGUISE_MALE = 45760 -- spell_generic.cpp:2600
local SPELL_ORC_DISGUISE_FEMALE = 45762 -- spell_generic.cpp:2601

local function orcDisguiseHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    if hitTarget:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if hitTarget:GetNativeGender() == GENDER_MALE then
        caster:CastSpell(hitTarget, SPELL_ORC_DISGUISE_MALE, true)
    else
        caster:CastSpell(hitTarget, SPELL_ORC_DISGUISE_FEMALE, true)
    end
end

RegisterSpellEvent(SPELL_GEN_ORC_DISGUISE, 5, orcDisguiseHit)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_cannon_blast::HandleScript (spell_generic.cpp:4331-4338):
--   EFFECT_0 SCRIPT_EFFECT. bp is the effect-0 CalcValue
--   (spell:GetEffectValue(0)); the hit unit self-casts Cannon Blast
--   Damage 42576, triggered, with BP0 = bp (CastCustomSpell 4-arg
--   form, savage-defense precedent in spell_druid.lua).
--   DORMANT until Spell.dbc ships the 42578 row.
local SPELL_GEN_CANNON_BLAST = 42578 -- spell_generic.cpp:4321 (class spell_gen_cannon_blast)
local SPELL_CANNON_BLAST_DAMAGE = 42576 -- spell_generic.cpp:4318

local function cannonBlastHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastCustomSpell(hitTarget, SPELL_CANNON_BLAST_DAMAGE, true, spell:GetEffectValue(0))
end

RegisterSpellEvent(SPELL_GEN_CANNON_BLAST, 5, cannonBlastHit)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_gen_lich_pet_aura::HandleProc + CheckProc (spell_generic.cpp:1893-1919):
--   EFFECT_0 PROC_TRIGGER_SPELL. CheckProc gates on the proc target
--   being a player (TYPEID_PLAYER); the handler returns true on both
--   paths — the C++ PreventDefaultAction veto plus the CheckProc veto
--   (a failed CheckProc fires neither the handler nor the default
--   trigger leg). Each owned lich-pet minion (entry 36979, temp
--   summon, GetAllMinionsByEntry bridge) self-casts Lich Pet Aura
--   OnKill 69731, non-triggered (DoCastSelf default args).
--   IsAIEnabled has no Go model (no AI-disabled motion state), so
--   the gate is vacuous in-tree. DORMANT until Spell.dbc ships the
--   69732 proc entry.
local SPELL_LICH_PET_AURA = 69732 -- spell_generic.cpp:1893 (class spell_gen_lich_pet_aura)
local NPC_LICH_PET = 36979 -- spell_generic.cpp:1887
local SPELL_LICH_PET_AURA_ONKILL = 69731 -- spell_generic.cpp:1890

local function lichPetAuraProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return true
    end
    if procTarget == nil or procTarget:GetTypeId() ~= TYPEID_PLAYER then
        return true
    end
    for _, minion in ipairs(target:GetAllMinionsByEntry(NPC_LICH_PET)) do
        minion:CastSpell(minion, SPELL_LICH_PET_AURA_ONKILL, false)
    end
    return true
end

RegisterAuraEvent(SPELL_LICH_PET_AURA, 6, lichPetAuraProc)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_lich_pet_onsummon::HandleScriptEffect (spell_generic.cpp:1921-1941):
--   EFFECT_0 SCRIPT_EFFECT. The hit unit casts Lich Pet Aura 69732
--   on itself, triggered. DORMANT until Spell.dbc ships the 69735 row.
local SPELL_LICH_PET_ONSUMMON = 69735 -- spell_generic.cpp:1921 (class spell_gen_lich_pet_onsummon)

local function lichPetOnSummonHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_LICH_PET_AURA, true)
end

RegisterSpellEvent(SPELL_LICH_PET_ONSUMMON, 5, lichPetOnSummonHit)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_gen_lich_pet_aura_remove::HandleScriptEffect (spell_generic.cpp:1943-1957):
--   EFFECT_0 SCRIPT_EFFECT. The hit unit drops Lich Pet Aura 69732.
--   DORMANT until Spell.dbc ships the 69736 row.
local SPELL_LICH_PET_AURA_REMOVE = 69736 -- spell_generic.cpp:1943 (class spell_gen_lich_pet_aura_remove)

local function lichPetAuraRemoveHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAura(SPELL_LICH_PET_AURA)
end

RegisterSpellEvent(SPELL_LICH_PET_AURA_REMOVE, 5, lichPetAuraRemoveHit)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_ethereal_pet_aura::HandleProc + CheckProc (spell_generic.cpp:1659-1691):
--   EFFECT_0 PROC_TRIGGER_SPELL. CheckProc gates on the level gap
--   between the aura target (owner) and the proc target being <= 9;
--   the handler returns true on both paths — the C++ PreventDefaultAction
--   veto plus the CheckProc veto (a failed CheckProc fires neither the
--   handler nor the default trigger leg). Each owned ethereal soul
--   trader minion (entry 27914, temp summon, GetAllMinionsByEntry
--   bridge) says SAY_STEAL_ESSENCE (1) and casts Steal Essence Visual
--   50101 on the proc target, non-triggered (C++ CastSpell default args).
--   IsAIEnabled has no Go model (no AI-disabled motion state), so the
--   gate is vacuous in-tree. DORMANT until Spell.dbc ships the 50051 row.
local SPELL_ETHEREAL_PET_AURA_PROC = 50051 -- spell_generic.cpp:1659 (class spell_ethereal_pet_aura)
local NPC_ETHEREAL_SOUL_TRADER = 27914 -- spell_generic.cpp:1647
local SAY_STEAL_ESSENCE = 1 -- spell_generic.cpp:1650
local SPELL_STEAL_ESSENCE_VISUAL = 50101 -- spell_generic.cpp:1656

local function etherealPetAuraProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or target == nil or procTarget == nil then
        return true
    end
    if math.abs(target:GetLevel() - procTarget:GetLevel()) > 9 then
        return true
    end
    for _, minion in ipairs(target:GetAllMinionsByEntry(NPC_ETHEREAL_SOUL_TRADER)) do
        minion:Talk(SAY_STEAL_ESSENCE)
        minion:CastSpell(procTarget, SPELL_STEAL_ESSENCE_VISUAL, false)
    end
    return true
end

RegisterAuraEvent(SPELL_ETHEREAL_PET_AURA_PROC, 6, etherealPetAuraProc)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_ethereal_pet_onsummon::HandleScriptEffect (spell_generic.cpp:1693-1713):
--   EFFECT_0 SCRIPT_EFFECT. The hit unit casts Ethereal Pet Aura 50051
--   on itself, triggered. DORMANT until Spell.dbc ships the 50052 row.
local SPELL_ETHEREAL_PET_ONSUMMON = 50052 -- spell_generic.cpp:1693 (class spell_ethereal_pet_onsummon)

local function etherealPetOnSummonHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:CastSpell(hitTarget, SPELL_ETHEREAL_PET_AURA_PROC, true)
end

RegisterSpellEvent(SPELL_ETHEREAL_PET_ONSUMMON, 5, etherealPetOnSummonHit)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_ethereal_pet_aura_remove::HandleScriptEffect (spell_generic.cpp:1716-1734):
--   EFFECT_0 SCRIPT_EFFECT. The hit unit drops the ethereal pet aura.
--   NOTE: the C++ enum names SPELL_ETHEREAL_PET_AURA = 50055, so the
--   remove leg drops aura 50055 (the spell's own id), not 50051 — ported
--   exactly as written. DORMANT until Spell.dbc ships the 50055 row.
local SPELL_ETHEREAL_PET_AURA_REMOVE = 50055 -- spell_generic.cpp:1716 (class spell_ethereal_pet_aura_remove)
local SPELL_ETHEREAL_PET_AURA = 50055 -- spell_generic.cpp:1654 (enum EtherealPet)

local function etherealPetAuraRemoveHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 or hitTarget == nil then
        return
    end
    hitTarget:RemoveAura(SPELL_ETHEREAL_PET_AURA)
end

RegisterSpellEvent(SPELL_ETHEREAL_PET_AURA_REMOVE, 5, etherealPetAuraRemoveHit)

-- PORTED (this tick, RegisterAuraEvent OnRemove = 3):
--   spell_steal_essence_visual::HandleRemove (spell_generic.cpp:1737-1755):
--   EFFECT_0 DUMMY, AURA_EFFECT_HANDLE_REAL. On remove the caster casts
--   Create Token 50063 on itself, triggered, and — when the caster is a
--   creature — the soul trader says SAY_CREATE_TOKEN (2). The C++
--   ToCreature() gate is the TYPEID_UNIT check. DORMANT until Spell.dbc
--   ships the 50101 row.
local SPELL_STEAL_ESSENCE_VISUAL_AURA = 50101 -- spell_generic.cpp:1737 (class spell_steal_essence_visual)
local SPELL_CREATE_TOKEN = 50063 -- spell_generic.cpp:1655 (enum EtherealPet)
local SAY_CREATE_TOKEN = 2 -- spell_generic.cpp:1651

local function stealEssenceVisualRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    caster:CastSpell(caster, SPELL_CREATE_TOKEN, true)
    if caster:GetTypeId() == TYPEID_UNIT then
        caster:Talk(SAY_CREATE_TOKEN)
    end
end

RegisterAuraEvent(SPELL_STEAL_ESSENCE_VISUAL_AURA, 3, stealEssenceVisualRemove)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_gen_blood_reserve::HandleProc + CheckProc (spell_generic.cpp:574-608):
--   EFFECT_0 PROC_TRIGGER_SPELL. CheckProc gates on the proc action
--   target being below 35% health; the handler returns true on both
--   paths — the C++ PreventDefaultAction veto plus the CheckProc veto.
--   The action target (procEvent:GetActionTarget(), the damaged unit —
--   the aura holder on a damage-taken proc) casts Blood Reserve Heal
--   64569 on itself with BP0 = the aura effect amount (CastCustomSpell
--   4-arg form, cannon_blast precedent; C++ CastSpellExtraArgs(aurEff)
--   + AddSpellBP0, triggered defaults false), then drops the 64568 aura.
--   HealthBelowPct rides GetHealthPct (generic object fallback);
--   strict < 35 matches the C++ strict-below comparison.
--   DORMANT until Spell.dbc ships the 64568 row.
local SPELL_BLOOD_RESERVE_AURA = 64568 -- spell_generic.cpp:570
local SPELL_BLOOD_RESERVE_HEAL = 64569 -- spell_generic.cpp:571

local function bloodReserveProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if effectIndex ~= 0 or procEvent == nil then
        return true
    end
    local actionTarget = procEvent:GetActionTarget()
    if actionTarget == nil or actionTarget:GetHealthPct() >= 35 then
        return true
    end
    actionTarget:CastCustomSpell(actionTarget, SPELL_BLOOD_RESERVE_HEAL, false, aura:GetAmount())
    actionTarget:RemoveAura(SPELL_BLOOD_RESERVE_AURA)
    return true
end

RegisterAuraEvent(SPELL_BLOOD_RESERVE_AURA, 6, bloodReserveProc)

-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_gen_burning_depths_necrolyte_image::HandleApply + HandleRemove
--   (spell_generic.cpp:799-824): EFFECT_0 TRANSFORM, REAL handles.
--   On apply the caster casts the EFFECT_2 value (the image spell id,
--   read here as aura:GetAmount(2) — the CalcValue twin; guarded to
--   never fire a bogus 0-id cast) on the target, non-triggered; on
--   remove the target drops that aura. ONE DELTA: C++ removes only the
--   auras cast by this caster (RemoveAurasDueToSpell + GetCasterGUID);
--   Lua RemoveAura has no caster scope, so it drops all stacks of the
--   image spell. DORMANT until Spell.dbc ships the 48750 row.
local SPELL_BURNING_DEPTHS_NECROLYTE_IMAGE = 48750 -- spell_generic.cpp:798

local function burningDepthsApply(event, aura, target)
    local caster = aura:GetCaster()
    if caster == nil or target == nil then
        return
    end
    local imageSpell = aura:GetAmount(2)
    if imageSpell == 0 then
        return
    end
    caster:CastSpell(target, imageSpell, false)
end

RegisterAuraEvent(SPELL_BURNING_DEPTHS_NECROLYTE_IMAGE, 2, burningDepthsApply)

local function burningDepthsRemove(event, aura, target)
    if target == nil then
        return
    end
    local imageSpell = aura:GetAmount(2)
    if imageSpell == 0 then
        return
    end
    target:RemoveAura(imageSpell)
end

RegisterAuraEvent(SPELL_BURNING_DEPTHS_NECROLYTE_IMAGE, 3, burningDepthsRemove)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6, aura half only):
--   spell_gen_decay_over_time_aura::Decay + CheckProc (spell_generic.cpp:1340-1363),
--   bound to 36659 by RegisterSpellAndAuraScriptPair (spell_generic.cpp:4382).
--   CheckProc gates on the proc event's spell being this aura's own spell
--   (procEvent:GetSpellId() == 36659; a spell-less event reads 0 and vetoes);
--   Decay vetoes the default leg and drops one stack
--   (SetStackAmount(GetStackAmount() - 1), the ModStackAmount(-1) twin).
--   SPELL HALF STAYS OUT: spell_gen_decay_over_time_spell::ModAuraStack
--   (AfterHit, :1324-1338) needs GetHitAura (no Lua bridge) plus the DBC
--   StackAmount value (no bridge). DORMANT until Spell.dbc ships 36659.
local SPELL_DECAY_OVER_TIME_TAIL_STING = 36659 -- spell_generic.cpp:1393

local function decayOverTimeProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if procEvent == nil or procEvent:GetSpellId() ~= SPELL_DECAY_OVER_TIME_TAIL_STING then
        return true
    end
    aura:SetStackAmount(aura:GetStackAmount() - 1)
    return true
end

RegisterAuraEvent(SPELL_DECAY_OVER_TIME_TAIL_STING, 6, decayOverTimeProc)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6, veto-only gate):
--   spell_gen_black_magic::CheckProc (spell_generic.cpp:532-566).
--   The class registers ONLY DoCheckProc — no proc handler of its own —
--   so the Lua port is a pure gate on the engine default leg: the ev6
--   hook fires before the default trigger (procs.go/boss_ai.go fire
--   sites), a true return vetoes it, a nil return lets it run. The
--   handler mirrors the C++ if/elif/elif chain exactly: no event spell
--   -> veto; magic-damage-class-negative -> allow; melee-damage-class
--   -> allow only for druid Shred/Mangle (family flags [0] 0x8000 /
--   [1] 0x400); none-damage-class-negative -> allow only for druid
--   Infected Wounds (icon 2857); anything else -> veto. The family
--   check rides the new procEvent:GetSpellFamilyName bridge
--   (lua_aura.go); flags/icon/typeMask were already bridged.
--   DORMANT until Spell.dbc ships the 59630 row.
local SPELL_GEN_BLACK_MAGIC = 59630 -- spell_generic.cpp:532
local PROC_FLAG_DONE_SPELL_MELEE_DMG_CLASS = 0x10 -- SpellMgr.h:123
local PROC_FLAG_DONE_SPELL_NONE_DMG_CLASS_NEG = 0x1000 -- SpellMgr.h:135
local PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_NEG = 0x10000 -- SpellMgr.h:141
local SPELLFAMILY_DRUID = 7 -- SharedDefines.h:3588

local function blackMagicProc(event, aura, target, actor, procTarget, damage, procEvent, effectIndex)
    if procEvent == nil or procEvent:GetSpellId() == 0 then
        return true
    end
    local typeMask = procEvent:GetTypeMask()
    if bit32.band(typeMask, PROC_FLAG_DONE_SPELL_MAGIC_DMG_CLASS_NEG) ~= 0 then
        return
    end
    if bit32.band(typeMask, PROC_FLAG_DONE_SPELL_MELEE_DMG_CLASS) ~= 0 then
        if procEvent:GetSpellFamilyName() == SPELLFAMILY_DRUID
            and (procEvent:GetSpellFamilyFlags(0) == 0x8000 or procEvent:GetSpellFamilyFlags(1) == 0x400) then
            return
        end
        return true
    end
    if bit32.band(typeMask, PROC_FLAG_DONE_SPELL_NONE_DMG_CLASS_NEG) ~= 0 then
        if procEvent:GetSpellFamilyName() == SPELLFAMILY_DRUID
            and procEvent:GetSpellIconId() == 2857 then -- SPELLICON_DRUID_INFECTED_WOUNDS, spell_generic.cpp:529
            return
        end
        return true
    end
    return true
end

RegisterAuraEvent(SPELL_GEN_BLACK_MAGIC, 6, blackMagicProc)

-- C++ spell_stasis_field_aura::OnPeriodic (spell_generic.cpp:4200-4229)
-- and spell_corrupting_plague_aura::OnPeriodic
-- (spell_generic.cpp:4142-4171): EFFECT_0
-- SPELL_AURA_PERIODIC_TRIGGER_SPELL on 40306 / 40349. Each tick
-- searches for a creature of the field's entries within 15yd that
-- lacks the field aura (40307 / 40350); if one is found the tick's
-- default trigger fires (return nothing), otherwise
-- PreventDefaultAction (return true). DELTAS: FindNearestCreature
-- is the 3D reach-adjusted bridge (lua_creature_events.go:6490)
-- where C++ uses GetDistance2d < 15; and only the nearest of each
-- entry is checked, where C++ scans the whole grid — a farther
-- matching creature without the aura is missed. DORMANT until
-- Spell.dbc ships the 40306/40349 rows.
local SPELL_STASIS_FIELD_AURA = 40306 -- spell_generic.cpp:4200
local SPELL_CORRUPTING_PLAGUE_AURA = 40349 -- spell_generic.cpp:4142
local SPELL_STASIS_FIELD = 40307 -- spell_generic.cpp:4175
local SPELL_CORRUPTING_PLAGUE = 40350 -- spell_generic.cpp:4117
local NPC_APEXIS_FLAYER = 22175 -- spell_generic.cpp:4114
local NPC_SHARD_HIDE_BOAR = 22180 -- spell_generic.cpp:4115
local NPC_AETHER_RAY = 22181 -- spell_generic.cpp:4116
local NPC_DAGGERTAIL_LIZARD = 22255 -- spell_generic.cpp:4174

local PLAGUE_FIELD_SEARCH = {
    [SPELL_STASIS_FIELD_AURA] = { aura = SPELL_STASIS_FIELD, entries = { NPC_APEXIS_FLAYER, NPC_SHARD_HIDE_BOAR, NPC_AETHER_RAY, NPC_DAGGERTAIL_LIZARD } },
    [SPELL_CORRUPTING_PLAGUE_AURA] = { aura = SPELL_CORRUPTING_PLAGUE, entries = { NPC_APEXIS_FLAYER, NPC_SHARD_HIDE_BOAR, NPC_AETHER_RAY } },
}

local function plagueFieldTick(event, aura, target)
    local field = PLAGUE_FIELD_SEARCH[aura:GetAuraId()]
    if field == nil or target == nil then
        return
    end
    for _, entry in ipairs(field.entries) do
        local creature = target:FindNearestCreature(entry, 15)
        if creature ~= nil and not creature:HasAura(field.aura) then
            return
        end
    end
    return true
end

RegisterAuraEvent(SPELL_STASIS_FIELD_AURA, 1, plagueFieldTick)
RegisterAuraEvent(SPELL_CORRUPTING_PLAGUE_AURA, 1, plagueFieldTick)

-- C++ spell_gen_absorb0_hitlimit1::Absorb (spell_generic.cpp:45-67):
-- 64765 EFFECT_0 SPELL_AURA_SCHOOL_ABSORB. The per-hit absorb caps at
-- the EFFECT_1 dummy CalcValue (Load): absorbAmount = min(limit,
-- absorbAmount). The Go event passes the pool as absorbAmount and the
-- remaining damage as damage; returning the capped amount rewrites it.
-- DORMANT until Spell.dbc ships the 64765 row.
local SPELL_ABSORB0_HITLIMIT1 = 64765 -- spell_generic.cpp:45

local function absorb0Hitlimit1(event, aura, target, damage, absorbAmount)
    local limit = aura:GetSpellEffectCalcValue(SPELL_ABSORB0_HITLIMIT1, 1)
    return math.min(limit, absorbAmount)
end

RegisterAuraEvent(SPELL_ABSORB0_HITLIMIT1, 9, absorb0Hitlimit1)
