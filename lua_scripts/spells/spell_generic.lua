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
