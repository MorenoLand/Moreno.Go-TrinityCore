-- Quest spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_quest.cpp.
--
-- PORTED (tick 198, RegisterAuraEvent OnRemove = 3):
--   spell_q10929_fumping::HandleEffectRemove
--   (spell_quest.cpp:2641-2651): EFFECT_0 SPELL_AURA_DUMMY. On
--   removal by expiry only, the caster casts one of Summon Sand
--   Gnome 39240 / Summon Bone Slicer 39241 on itself, triggered
--   (C++ urand picks uniformly). The mode gate is
--   Aura:GetRemoveMode (new tick 198: SpellAuraDefines.h:61-69
--   values).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so a player caster's
-- triggered self-cast lands non-triggered; pre-existing bridge gap,
-- same as all prior player-target ports.

local SPELL_FUMPING = 39238 -- spell_quest.cpp:2630
local SPELL_SUMMON_SAND_GNOME = 39240 -- spell_quest.cpp:2626
local SPELL_SUMMON_BONE_SLICER = 39241 -- spell_quest.cpp:2627
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

local function fumpingRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    if math.random(0, 1) == 0 then
        caster:CastSpell(caster, SPELL_SUMMON_SAND_GNOME, true)
    else
        caster:CastSpell(caster, SPELL_SUMMON_BONE_SLICER, true)
    end
end

RegisterAuraEvent(SPELL_FUMPING, 3, fumpingRemove)

local SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3 = 43874 -- spell_quest.cpp:296
local UNIT_FLAG_IMMUNE_TO_PC = 0x00000100 -- UnitDefines.h:132

-- C++ spell_q11396_11399_force_shield_arcane_purple_x3_AuraScript::
-- HandleEffectApply (spell_quest.cpp:308-314): EFFECT_0 DUMMY, REAL.
-- Target immune to PC. STILL BLOCKED leg: AddUnitState(UNIT_STATE_ROOT)
-- (no bridge).
local function forceShieldApply(event, aura, target)
    target:SetFlag(59, UNIT_FLAG_IMMUNE_TO_PC)
end

RegisterAuraEvent(SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3, 2, forceShieldApply)

-- C++ spell_q11396_11399_force_shield_arcane_purple_x3_AuraScript::
-- HandleEffectRemove (spell_quest.cpp:316-319): clears PC immunity.
local function forceShieldRemove(event, aura, target)
    target:RemoveFlag(59, UNIT_FLAG_IMMUNE_TO_PC)
end

RegisterAuraEvent(SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3, 3, forceShieldRemove)

-- PORTED (tick 295, RegisterAuraEvent OnPeriodic = 1):
--   spell_q2203_thaumaturgy_channel::HandleEffectPeriodic
--   (spell_quest.cpp:137-143): EFFECT_0 PERIODIC_TRIGGER_SPELL.
--   PreventDefaultAction rides the return-true twin; the caster
--   self-casts Thaumaturgy Channel 21029, non-triggered.
--   spell_q11010_q11102_q11023_aggro_check_aura::HandleTriggerSpell
--   (spell_quest.cpp:1546-1552): EFFECT_0 PERIODIC_TRIGGER_SPELL.
--   No PreventDefaultAction, so the Go default tick still fires (no
--   return); the target self-casts Aggro Check 40112, non-triggered.
--   spell_q11010_q11102_q11023_aggro_burst::HandleEffectPeriodic
--   (spell_quest.cpp:1605-1611): EFFECT_0 PERIODIC_DUMMY. No
--   PreventDefaultAction (no return); the target self-casts Choose
--   Loc 40056, non-triggered.
--   spell_q12619_emblazon_runeblade::HandleEffectPeriodic
--   (spell_quest.cpp:2420-2429): EFFECT_0 PERIODIC_TRIGGER_SPELL.
--   PreventDefaultAction (return true); the caster self-casts the
--   effect's DBC trigger spell (aura:GetTriggerSpell(0), 0-skip),
--   triggered. KNOWN LIMIT: player:CastSpell ignores the triggered
--   flag (see header note above), same as all prior player ports.
--
-- PORTED (tick 295, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_q13264_q13276_q13288_q13289_burst_at_the_seams_59579
--   (spell_quest.cpp:2064-2103): EFFECT_0 DUMMY, REAL. On apply the
--   target self-casts, all triggered: Troll Explosion 52565,
--   Explode Abomination Meat 52520, Explode Troll Meat 52578 twice,
--   Explode Troll Bloody Meat 52580, Burst at the Seams Bone 52516.
--   On remove the target casts the kill-credit spell on the caster
--   by its entry (Icy Ghoul 31142 -> 59590, Vicious Geist 31147 ->
--   60041, Risen Alliance Soldiers 31205 -> 60039), then self-casts
--   Burst at the Seams 59580, triggered. Go OnApply is fresh-only;
--   C++ uses REAL (not REAL_OR_REAPPLY), so no re-apply delta.
--   spell_q13264_q13276_q13288_q13289_bloated_abom_feign_death
--   (spell_quest.cpp:2136-2160): EFFECT_0 DUMMY, REAL. On apply the
--   target sets UNIT_DYNFLAG_DEAD and UNIT_FLAG2_FEIGN_DEATH and,
--   as a creature, goes REACT_PASSIVE (0). On remove the creature
--   despawns. Creature-only in practice (abomination NPCs).
--
-- CLOSED (tick 295 census, corrected 2026-11-11): the remaining quest
--   classes are SpellScript-only. The "no RegisterSpellScript bridge"
--   claim is now STALE in both directions: RegisterSpellEvent ev4
--   (OnEffectHit) EXISTS and fires on the player path (spells.go:14625),
--   so no-target OnEffectHit legs port (q5206/q12634/q12683 ported
--   below); ev5 (OnEffectHitTarget) ALSO fires on the player path now
--   (a910c39d, spells.go:14668-14737: one fire per (effect, live hit
--   target), effect-major, before that effect's default), so direct
--   player-cast ev5 legs port (rows 675-686 + 690 ported below).
--   Also staying out: quenching_mist + read_pronouncement (aura IDs
--   live only in the world DB spell_script_names, unresolvable in
--   workspace), and the q12619 emblazon effect SpellScript.

local SPELL_THAUMATURGY_CHANNEL_AURA = 9712 -- spell_quest.cpp:117 (class spell_q2203_thaumaturgy_channel)
local SPELL_THAUMATURGY_CHANNEL = 21029 -- spell_quest.cpp:120
local SPELL_AGGRO_CHECK_AURA = 40113 -- spell_quest.cpp:1535 (class spell_q11010_q11102_q11023_aggro_check_aura)
local SPELL_AGGRO_CHECK = 40112 -- spell_quest.cpp:1530
local SPELL_AGGRO_BURST_AURA = 40119 -- spell_quest.cpp:1594 (class spell_q11010_q11102_q11023_aggro_burst)
local SPELL_CHOOSE_LOC = 40056 -- spell_quest.cpp:1529
local SPELL_EMBLAZON_RUNEBLADE = 51769 -- spell_quest.cpp:2412 (class spell_q12619_emblazon_runeblade)
local SPELL_BURST_AT_THE_SEAMS_59579 = 59579 -- spell_quest.cpp:2011
local SPELL_TROLL_EXPLOSION = 52565 -- spell_quest.cpp:2005
local SPELL_EXPLODE_ABOMINATION_MEAT = 52520 -- spell_quest.cpp:2004
local SPELL_EXPLODE_TROLL_MEAT = 52578 -- spell_quest.cpp:2006
local SPELL_EXPLODE_TROLL_BLOODY_MEAT = 52580 -- spell_quest.cpp:2007
local SPELL_BURST_AT_THE_SEAMS_BONE = 52516 -- spell_quest.cpp:2003
local SPELL_BURST_AT_THE_SEAMS_59580 = 59580 -- spell_quest.cpp:2012
local SPELL_ASSIGN_GHOUL_KILL_CREDIT_TO_MASTER = 59590 -- spell_quest.cpp:2014
local SPELL_ASSIGN_GEIST_KILL_CREDIT_TO_MASTER = 60041 -- spell_quest.cpp:2015
local SPELL_ASSIGN_SKELETON_KILL_CREDIT_TO_MASTER = 60039 -- spell_quest.cpp:2016
local NPC_ICY_GHOUL = 31142 -- spell_quest.cpp:1995
local NPC_VICIOUS_GEIST = 31147 -- spell_quest.cpp:1996
local NPC_RISEN_ALLIANCE_SOLDIERS = 31205 -- spell_quest.cpp:1997
local SPELL_BLOATED_ABOMINATION_FEIGN_DEATH = 52593 -- spell_quest.cpp:2002
local UNIT_DYNAMIC_FLAGS = 79 -- UpdateFields.h:136 (OBJECT_END 6 + 0x49)
local UNIT_DYNFLAG_DEAD = 0x20 -- SharedDefines.h:3133
local UNIT_FIELD_FLAGS_2 = 60 -- UpdateFields.h:118 (OBJECT_END 6 + 0x36)
local UNIT_FLAG2_FEIGN_DEATH = 0x1 -- UnitDefines.h:161
local REACT_PASSIVE = 0

local function thaumaturgyChannelTick(event, aura, target)
    local caster = aura:GetCaster()
    if caster ~= nil then
        caster:CastSpell(caster, SPELL_THAUMATURGY_CHANNEL, false)
    end
    return true
end

RegisterAuraEvent(SPELL_THAUMATURGY_CHANNEL_AURA, 1, thaumaturgyChannelTick)

local function aggroCheckAuraTick(event, aura, target)
    target:CastSpell(target, SPELL_AGGRO_CHECK, false)
end

RegisterAuraEvent(SPELL_AGGRO_CHECK_AURA, 1, aggroCheckAuraTick)

local function aggroBurstTick(event, aura, target)
    target:CastSpell(target, SPELL_CHOOSE_LOC, false)
end

RegisterAuraEvent(SPELL_AGGRO_BURST_AURA, 1, aggroBurstTick)

local function emblazonRunebladeTick(event, aura, target)
    local trigger = aura:GetTriggerSpell(0)
    if trigger == 0 then
        return true
    end
    local caster = aura:GetCaster()
    if caster ~= nil then
        caster:CastSpell(caster, trigger, true)
    end
    return true
end

RegisterAuraEvent(SPELL_EMBLAZON_RUNEBLADE, 1, emblazonRunebladeTick)

local function burstAtTheSeamsApply(event, aura, target)
    target:CastSpell(target, SPELL_TROLL_EXPLOSION, true)
    target:CastSpell(target, SPELL_EXPLODE_ABOMINATION_MEAT, true)
    target:CastSpell(target, SPELL_EXPLODE_TROLL_MEAT, true)
    target:CastSpell(target, SPELL_EXPLODE_TROLL_MEAT, true)
    target:CastSpell(target, SPELL_EXPLODE_TROLL_BLOODY_MEAT, true)
    target:CastSpell(target, SPELL_BURST_AT_THE_SEAMS_BONE, true)
end

RegisterAuraEvent(SPELL_BURST_AT_THE_SEAMS_59579, 2, burstAtTheSeamsApply)

local function burstAtTheSeamsRemove(event, aura, target)
    local caster = aura:GetCaster()
    if caster ~= nil then
        local entry = target:GetEntry()
        if entry == NPC_ICY_GHOUL then
            target:CastSpell(caster, SPELL_ASSIGN_GHOUL_KILL_CREDIT_TO_MASTER, true)
        elseif entry == NPC_VICIOUS_GEIST then
            target:CastSpell(caster, SPELL_ASSIGN_GEIST_KILL_CREDIT_TO_MASTER, true)
        elseif entry == NPC_RISEN_ALLIANCE_SOLDIERS then
            target:CastSpell(caster, SPELL_ASSIGN_SKELETON_KILL_CREDIT_TO_MASTER, true)
        end
    end
    target:CastSpell(target, SPELL_BURST_AT_THE_SEAMS_59580, true)
end

RegisterAuraEvent(SPELL_BURST_AT_THE_SEAMS_59579, 3, burstAtTheSeamsRemove)

local function bloatedAbomFeignDeathApply(event, aura, target)
    target:SetFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_DEAD)
    target:SetFlag(UNIT_FIELD_FLAGS_2, UNIT_FLAG2_FEIGN_DEATH)
    target:SetReactState(REACT_PASSIVE)
end

RegisterAuraEvent(SPELL_BLOATED_ABOMINATION_FEIGN_DEATH, 2, bloatedAbomFeignDeathApply)

local function bloatedAbomFeignDeathRemove(event, aura, target)
    target:DespawnOrUnsummon()
end

RegisterAuraEvent(SPELL_BLOATED_ABOMINATION_FEIGN_DEATH, 3, bloatedAbomFeignDeathRemove)

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnEffectHit = 4):
--   spell_q5206_test_fetid_skull (spell_quest.cpp:164-214): EFFECT_0
--   DUMMY OnEffectHit, caster-is-player Load gate (explicit
--   GetTypeId()==4 check — ev4 also fires on the creature path,
--   boss_ai.go:3373). 50% roll: Create Resonating
--   Skull 17269, else Create Bone Dust 17270, triggered self-cast.
--   spell_q12634_despawn_fruit_tosser (spell_quest.cpp:603-649):
--   EFFECT_0 DUMMY OnEffectHit. urand(0,3): 1 -> Orange Falls to
--   Ground 51837, 2 -> Papaya Falls to Ground 51839, 0/3 -> Bananas
--   Fall to Ground 51836; then a 5% roll overrides to Summon
--   Adventurous Dwarf 52070. Triggered self-cast.
--   spell_q12683_take_sputum_sample (spell_quest.cpp:651-685):
--   EFFECT_0 DUMMY OnEffectHit. reqAuraId is the spell's EFFECT_1
--   DBC value, the cast spell its EFFECT_0 DBC value (both via
--   spell:GetEffectValue, lua_spell.go:215); if the caster has the
--   aura, it casts the spell on itself, triggered. C++ checks
--   HasAuraEffect(reqAuraId, 0) (aura with an effect at index 0);
--   this tick's unit:HasAuraEffect bridge (lua_player.go /
--   lua_creature_events.go, the GetAuraEffect nil twin) ports it
--   exactly — the old HasAura twin + its KNOWN DELTA note are
--   retired.
--   The ev4 hook fires on the player path (spells.go:14625, ahead of
--   the effects loop, matching C++ HIT-phase order); the ev5
--   OnEffectHitTarget hook fires on the player path too (a910c39d,
--   spells.go:14668-14737) — the funnel-only claim is STALE,
--   corrected tick 2026-11-12.
--   KNOWN LIMIT: player:CastSpell ignores the triggered flag (see
--   header note above), same as all prior player ports.

local SPELL_TEST_FETID_SKULL = 17271 -- spell_quest.cpp:164 (class spell_q5206_test_fetid_skull)
local SPELL_CREATE_RESONATING_SKULL = 17269 -- spell_quest.cpp:160
local SPELL_CREATE_BONE_DUST = 17270 -- spell_quest.cpp:161
local SPELL_DESPAWN_FRUIT_TOSSER = 51840 -- spell_quest.cpp:594 (class spell_q12634_despawn_fruit_tosser)
local SPELL_BANANAS_FALL_TO_GROUND = 51836 -- spell_quest.cpp:597
local SPELL_ORANGE_FALLS_TO_GROUND = 51837 -- spell_quest.cpp:598
local SPELL_PAPAYA_FALLS_TO_GROUND = 51839 -- spell_quest.cpp:599
local SPELL_SUMMON_ADVENTUROUS_DWARF = 52070 -- spell_quest.cpp:600
local SPELL_TAKE_SPUTUM_SAMPLE = 52308 -- spell_quest.cpp:650 (class spell_q12683_take_sputum_sample)

local function testFetidSkullHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if caster:GetTypeId() ~= 4 then
        return
    end
    if math.random(0, 1) == 0 then
        caster:CastSpell(caster, SPELL_CREATE_RESONATING_SKULL, true)
    else
        caster:CastSpell(caster, SPELL_CREATE_BONE_DUST, true)
    end
end

RegisterSpellEvent(SPELL_TEST_FETID_SKULL, 4, testFetidSkullHit)

local function despawnFruitTosserHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local spellId = SPELL_BANANAS_FALL_TO_GROUND
    local roll = math.random(0, 3)
    if roll == 1 then
        spellId = SPELL_ORANGE_FALLS_TO_GROUND
    elseif roll == 2 then
        spellId = SPELL_PAPAYA_FALLS_TO_GROUND
    end
    if math.random(1, 100) <= 5 then
        spellId = SPELL_SUMMON_ADVENTUROUS_DWARF
    end
    caster:CastSpell(caster, spellId, true)
end

RegisterSpellEvent(SPELL_DESPAWN_FRUIT_TOSSER, 4, despawnFruitTosserHit)

local function takeSputumSampleHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local reqAuraId = spell:GetEffectValue(1)
    if reqAuraId > 0 and caster:HasAuraEffect(reqAuraId, 0) then
        local spellId = spell:GetEffectValue(0)
        if spellId > 0 then
            caster:CastSpell(caster, spellId, true)
        end
    end
end

RegisterSpellEvent(SPELL_TAKE_SPUTUM_SAMPLE, 4, takeSputumSampleHit)

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnEffectHit = 4):
--   spell_symbol_of_life_dummy (spell_quest.cpp:845-881): EFFECT_0
--   DUMMY OnEffectHitTarget, reached via ev4 + spell:GetTarget()
--   (ev5 has a direct-path fire site too now, a910c39d — the ev4
--   route stands, fired ahead of the effects loop). If the hit creature has Permanent
--   Feign Death 29266, the aura is removed, UNIT_DYNAMIC_FLAGS (79)
--   and UNIT_FIELD_FLAGS_2 (60) are cleared to 0 (RemoveFlag with a
--   full mask is the SetUInt32Value twin, lua_creature_events.go:6130),
--   health is set to half max and mana to 75% of max.
--   spell_q12659_ahunaes_knife (spell_quest.cpp:888-924): EFFECT_0
--   DUMMY OnEffectHitTarget, same ev4 + spell:GetTarget() route.
--   Caster-is-player Load gate is an explicit GetTypeId()==4 check.
--   The hit creature despawns, then the caster takes Scalps kill
--   credit 28622.
--   spell_q9452_cast_net (spell_quest.cpp:1123-1169): EFFECT_0 DUMMY
--   OnEffectHit is ev4-native (no target needed; caster-is-player
--   Load gate is an explicit GetTypeId()==4 check): 66% AddItem Red
--   Snapper 23614, else the caster self-casts Fished Up Murloc 29869,
--   triggered. Binding 29866 is enum-grounded (SPELL_CAST_NET,
--   :1128; effect layout EFFECT_0 DUMMY + EFFECT_1 ACTIVATE_OBJECT
--   matches the Register at :1156-1157).
--   KNOWN DELTAS: ev4 fires ahead of the effects loop, before miss
--   filtering — the C++ arms fire only on a hit target; all three
--   spells target friendly/helpless units, so a miss is
--   near-impossible. C++ PreventHitDefaultEffect has no ev4 twin
--   (notification-only); the q9452 dummy effect has no Go default leg
--   to suppress. KNOWN LIMIT: player:CastSpell ignores the triggered
--   flag (see header note above), same as all prior player ports.
--
-- STAY OUT (same tick):
--   spell_q10041_q10040_who_are_they (spell_quest.cpp:803-841):
--   EFFECT_0 SCRIPT_EFFECT OnEffectHitTarget; GetHitPlayer casts the
--   gendered disguise 38080/38081 + generic 32756, triggered
--   (player:GetNativeGender bridged, lua_player.go:360; reachable via
--   ev4 + spell:GetTarget()) — no binding spell ID resolvable
--   in-tree (no comment; the enum holds only the disguise spells;
--   spell_script_names lives in the world DB, not the workspace).
--   spell_q9874_liquid_fire (spell_quest.cpp:930-971): EFFECT_0 DUMMY
--   OnEffectHitTarget; hit creature without Flames 39199 gives the
--   caster Villager kill credit 18240, self-casts 39199 triggered,
--   despawns after 60s (all bridges live: HasAura, KilledMonsterCredit,
--   CastSpell, DespawnOrUnsummon(ms)) — no binding spell ID
--   resolvable in-tree (no comment; the enum holds only 18240/39199).
--   spell_q12805_lifeblood_dummy (spell_quest.cpp:975-1013): EFFECT_0
--   SCRIPT_EFFECT OnEffectHitTarget; kill credit 29303, hit creature
--   self-casts its EFFECT_0 DBC value (spell:GetEffectValue(0),
--   lua_spell.go:215), despawns after 2s — no binding spell ID
--   resolvable in-tree (no comment; the enum holds only 29303).
--   spell_q14112_14145_chum_the_water (spell_quest.cpp:1083-1116):
--   EFFECT_0 SCRIPT_EFFECT OnEffectHitTarget; caster-only self-cast
--   of RAND(66737, 66738, 66739, 66740), NON-triggered in C++ (Lua
--   funnel is triggered-only — would be a noted deviation) — no
--   binding spell ID resolvable in-tree (no comment; the enum holds
--   only the four summon spells).
--   spell_q14076_14092_pound_drum (spell_quest.cpp:1173-1210):
--   HandleSummon OnCast (ev1, which DOES fire on the player path,
--   spells.go:10861) 50/50 self-cast 66510/66492 triggered, plus a
--   GO SetLootState(3) OnEffectHitTarget leg — no binding spell ID
--   resolvable in-tree (no comment; the enum holds only the two
--   summon spells).
--   spell_q12279_cast_net (spell_quest.cpp:1210-1235): single
--   EFFECT_1 ACTIVATE_OBJECT OnEffectHitTarget leg, GO SetLootState
--   only — no binding spell ID resolvable in-tree (no comment, no
--   enum); secondary: ev5-only + GO Use unbridged (no methods["Use"]
--   on GO objects; SetLootState/SetRespawnTime are bridged,
--   lua_objects.go:4324/4353), which also blocks the q9452
--   HandleActiveObject leg (:1157) and the q14076 GO leg.
--   Census go_hit/lua_hit true on 691/693/694/695/696/697/698/699 are
--   phantoms (zero class-name hits in engine/ + lua_scripts/).

local SPELL_SYMBOL_OF_LIFE = 8593 -- spell_quest.cpp:844 (class spell_symbol_of_life_dummy)
local SPELL_PERMANENT_FEIGN_DEATH = 29266 -- spell_quest.cpp:842
local POWER_MANA = 0
local CLEAR_ALL_FLAGS = 0xFFFFFFFF -- RemoveFlag full-mask twin of SetUInt32Value(field, 0)
local SPELL_CAST_NET = 29866 -- spell_quest.cpp:1128 (class spell_q9452_cast_net)
local ITEM_RED_SNAPPER = 23614 -- spell_quest.cpp:1127
local SPELL_FISHED_UP_MURLOC = 29869 -- spell_quest.cpp:1129

local function symbolOfLifeHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local target = spell:GetTarget()
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    if target:HasAura(SPELL_PERMANENT_FEIGN_DEATH) then
        target:RemoveAura(SPELL_PERMANENT_FEIGN_DEATH)
        target:RemoveFlag(UNIT_DYNAMIC_FLAGS, CLEAR_ALL_FLAGS)
        target:RemoveFlag(UNIT_FIELD_FLAGS_2, CLEAR_ALL_FLAGS)
        target:SetHealth(math.floor(target:GetMaxHealth() / 2))
        target:SetPower(POWER_MANA, math.floor(target:GetMaxPower(POWER_MANA) * 0.75))
    end
end

RegisterSpellEvent(SPELL_SYMBOL_OF_LIFE, 4, symbolOfLifeHit)

local function castNetHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if caster:GetTypeId() ~= 4 then
        return
    end
    if math.random(1, 100) <= 66 then
        caster:AddItem(ITEM_RED_SNAPPER, 1)
    else
        caster:CastSpell(caster, SPELL_FISHED_UP_MURLOC, true)
    end
end

RegisterSpellEvent(SPELL_CAST_NET, 4, castNetHit)

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnEffectHit = 4):
--   spell_q11010_q11102_q11023_choose_loc (spell_quest.cpp:1624-1660):
--   EFFECT_0 DUMMY OnEffectHit. Caster searches players in 65yd
--   (GetPlayersInRange, shared worldobject range surface); any player
--   flying gets a Fel Cannon 23082 summoned at their position
--   (SummonCreature entry,x,y,z,0 — default summonType 7 is the C++
--   MANUAL_DESPAWN default). No Load() gate in C++ (caster is the
--   burst-aura target, creature or player; ev4 fires on both paths).
--   KNOWN DELTA: C++ checks HasAuraType(SPELL_AURA_FLY) or
--   MOD_INCREASE_MOUNTED_FLIGHT_SPEED (unbridged); this uses
--   player:IsFlying() (movement-flag based, lua_player.go:1500) —
--   identical for mounted/flight-form players, reads false before
--   the first movement packet.

local NPC_FEL_CANNON2 = 23082 -- spell_quest.cpp:1532 (Quest11010_11102_11023Data)

local function chooseLocHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    for _, player in ipairs(caster:GetPlayersInRange(65)) do
        if player:IsFlying() then
            caster:SummonCreature(NPC_FEL_CANNON2, player:GetPositionX(), player:GetPositionY(), player:GetPositionZ(), 0)
        end
    end
end

RegisterSpellEvent(SPELL_CHOOSE_LOC, 4, chooseLocHit)

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q13665_q13790_bested_trigger (spell_quest.cpp:2698-2724):
--   EFFECT_0 SCRIPT_EFFECT OnEffectHitTarget. The hit unit's
--   charmer-or-owner-or-self self-casts the effect's DBC value
--   (spell:GetEffectValue(0)), triggered. Binding: the 14 spell IDs
--   in the C++ comment (:2696-2697) — 13665/13745/13750/13756/13761/
--   13767/13772/13777/13782/13787 (The Grand Melee) + 13790/13793/
--   13811/13814 (Among the Champions). ev5 fires on the creature
--   path (boss_ai.go:3402 loop inside castCreatureSpellInternalWith-
--   BasePoints); the caster is the defeated champion creature.
--   GetCharmerOrOwnerOrSelf is bridged exactly: GetControllerGUIDS
--   (charmer else owner else own GUID — lua_creature_events.go:524,
--   lua_player.go:232) resolved via the GetPlayerByGUID global
--   (scripting/runtime.go:1366) or, for creature targets,
--   GetCreatureByGUID (lua_creature_events.go:4050); unresolvable
--   GUIDs fall through to the hit unit. KNOWN LIMIT: player:CastSpell
--   ignores the triggered flag (see header note above), same as all
--   prior player ports.

local BESTED_TRIGGER_SPELLS = {13665, 13745, 13750, 13756, 13761, 13767, 13772, 13777, 13782, 13787, 13790, 13793, 13811, 13814} -- spell_quest.cpp:2696-2697

local function bestedTriggerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil then
        return
    end
    local target = hitTarget
    local guid = hitTarget:GetControllerGUIDS()
    if guid ~= 0 then
        local resolved = GetPlayerByGUID(guid)
        if resolved == nil and hitTarget:GetObjectType() == "Creature" then
            resolved = hitTarget:GetCreatureByGUID(guid)
        end
        if resolved ~= nil then
            target = resolved
        end
    end
    local spellId = spell:GetEffectValue(0)
    if spellId > 0 then
        target:CastSpell(target, spellId, true)
    end
end

for _, spellId in ipairs(BESTED_TRIGGER_SPELLS) do
    RegisterSpellEvent(spellId, 5, bestedTriggerHit)
end

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnEffectHit = 4):
--   spell_q11306_failed_mix_43376 (spell_quest.cpp:2836-2863):
--   EFFECT_1 SEND_EVENT OnEffectHit. The caster finds the nearest
--   Generic Trigger Lab 24042 within 100yd; it whispers Talk(0) to
--   the caster. Binding 43376 is in the class name.
--   spell_q11306_failed_mix_43378 (spell_quest.cpp:2864-2892):
--   EFFECT_2 SEND_EVENT OnEffectHit, same shape, Talk(1). Binding
--   43378 is in the class name. ev4 fires on the player path
--   (spells.go:14625); the caster is the player mixing the blood.
--   Talk(textId, whisperTarget) is the bridged AI::Talk twin
--   (lua_creature_events.go:1196); FindNearestCreature(entry, dist)
--   is bridged on player + creature surfaces.

local SPELL_FAILED_MIX_43376 = 43376 -- spell_quest.cpp:2836 (class spell_q11306_failed_mix_43376)
local SPELL_FAILED_MIX_43378 = 43378 -- spell_quest.cpp:2864 (class spell_q11306_failed_mix_43378)
local CREATURE_GENERIC_TRIGGER_LAB = 24042 -- spell_quest.cpp:2762 (ApplyHeatAndStir enum)
local TALK_FAILED_MIX_0 = 0 -- spell_quest.cpp:2764
local TALK_FAILED_MIX_1 = 1 -- spell_quest.cpp:2765

local function failedMix43376Hit(event, spell, effectIndex)
    if effectIndex ~= 1 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local trigger = caster:FindNearestCreature(CREATURE_GENERIC_TRIGGER_LAB, 100)
    if trigger ~= nil then
        trigger:Talk(TALK_FAILED_MIX_0, caster)
    end
end

RegisterSpellEvent(SPELL_FAILED_MIX_43376, 4, failedMix43376Hit)

local function failedMix43378Hit(event, spell, effectIndex)
    if effectIndex ~= 2 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    local trigger = caster:FindNearestCreature(CREATURE_GENERIC_TRIGGER_LAB, 100)
    if trigger ~= nil then
        trigger:Talk(TALK_FAILED_MIX_1, caster)
    end
end

RegisterSpellEvent(SPELL_FAILED_MIX_43378, 4, failedMix43378Hit)

-- PORTED (tick 2026-11-11, RegisterSpellEvent OnCheckCast = 8):
--   spell_q11010_q11102_q11023_q11008_check_fly_mount
--   (spell_quest.cpp:1671-1678): OnCheckCast. The caster must carry
--   a SPELL_AURA_FLY (201) or MOD_INCREASE_MOUNTED_FLIGHT_SPEED
--   (207) aura, else SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW (173).
--   Both 39844 (Skyguard Blasting Charge) and 40160 (Throw Bomb)
--   share the one C++ loader, so both bind here. Retires the
--   hardcoded spells.go gate (same arm, fired before the combo-point
--   leg); the ev8 consult fires after it, matching C++ CheckCast
--   order (combo points Spell.cpp:6186-6209, OnCheckCast at the end
--   :7977-7996). caster:HasAuraType is the Unit::HasAuraType bridge
--   (lua_player.go:2796); GetCaster is the session player on this
--   path, so a nil caster fails closed.

local SPELL_SKYGUARD_BLASTING_CHARGE = 39844 -- spell_quest.cpp:1662 (class spell_q11010_q11102_q11023_q11008_check_fly_mount)
local SPELL_THROW_BOMB = 40160 -- spell_quest.cpp:1662
local SPELL_AURA_FLY = 201 -- SpellAuraDefines.h:281
local SPELL_AURA_MOD_INCREASE_MOUNTED_FLIGHT_SPEED = 207 -- SpellAuraDefines.h:287
local SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW = 173 -- SharedDefines.h
local SPELL_CAST_OK = 255 -- SharedDefines.h:1171

local function checkFlyMountCheckCast(event, spell)
    local caster = spell:GetCaster()
    if caster == nil then
        return SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW
    end
    if not (caster:HasAuraType(SPELL_AURA_FLY) or caster:HasAuraType(SPELL_AURA_MOD_INCREASE_MOUNTED_FLIGHT_SPEED)) then
        return SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW
    end
    return SPELL_CAST_OK
end

RegisterSpellEvent(SPELL_SKYGUARD_BLASTING_CHARGE, 8, checkFlyMountCheckCast)
RegisterSpellEvent(SPELL_THROW_BOMB, 8, checkFlyMountCheckCast)

-- STAY OUT (tick 2026-11-11 census rows 712-721 + 736-738, verified
--   name-by-name: all 13 classes have ZERO hits in engine/ +
--   lua_scripts/, so the census go_hit/lua_hit=true flags on these
--   no-id/comment-block rows are phantoms, as are the go_hit=true
--   flags on bested rows 724/727/731/732 — those IDs appear in Go
--   only as unrelated constants):
--   spell_q12527_zuldrak_rat (spell_quest.cpp:1697-1736): ev5
--   EFFECT_1 SCRIPT_EFFECT; hit-aura stack >= SpellInfo StackAmount
--   gates a triggered 50928 summon + creature despawn — no
--   spell:GetHitAura bridge, no binding spell ID in-tree.
--   spell_q12730_quenching_mist (:1767-1806): AuraScript periodic
--   heal removing 53504 — no binding aura ID in-tree.
--   spell_q13291_q13292_q13239_q13261_frostbrood_skytalon_grab_decoy
--   (:1808-1843): ev5 DUMMY; hit creature casts 59319 triggered on
--   the caster — no binding spell ID (comment holds quest IDs only).
--   (Corrected tick 2026-11-12: the ev5 player-path fire site IS live
--   (a910c39d), so the binding ID is the sole blocker.)
--   spell_q13011_bear_flank_master (:1907-1956): ev5 SCRIPT_EFFECT,
--   caster-is-creature Load gate, hit player 50% casts 56569 on the
--   creature + AI Talk(0) else self-casts 56566 — no binding ID.
--   spell_q13086_cannons_target (:1957-1985): ev4 DUMMY; caster casts
--   GetEffectValue() triggered at the explicit dest — no binding ID;
--   no position-targeted CastSpell bridge (spell:GetTargetDest
--   exists, lua_spell.go:91, but no CastSpell(x,y,z) arm).
--   spell_q12641_recall_eye_of_acherus (:2381-2415): ev5; caster's
--   charmer-or-owner player stops charm/bindsight + removes the Eye
--   aura — GetCharmerOrOwner object + StopCastingCharm/BindSight
--   unbridged, no binding ID.
--   spell_q12919_gymers_grab (:2474-2514): ev5; hit creature casts
--   43671 on the caster with BP0=2 + 55424 on itself — no binding
--   ID; Lua CastSpell takes no base-points arg.
--   spell_q12919_gymers_throw (:2515-2551): ev5; caster vehicle
--   passenger(1) exits + caster casts 55569 on it — no binding ID
--   only: GetVehicleKit (lua_creature_events.go:3282) + GetPassenger
--   (lua_vehicle.go:74) ARE bridged, so the arm ports once a binding
--   ID is found.
--   spell_q13400_illidan_kill_master (:2552-2590): ev5 DUMMY; caster
--   vehicle passenger(0) self-casts 61748 — no binding ID only, same
--   vehicle bridges live.
--   spell_q12414_hand_over_reins (:2666-2697): ev5 EFFECT_1; hit unit
--   ExitVehicle + caster creature despawns — no binding ID
--   (ExitVehicle itself IS bridged, lua_player.go:2889).
--   spell_59064_59439_portals (:2726-2765): ev5 SCRIPT_EFFECT; hit
--   unit self-casts GetEffectValue() NON-triggered — binding IDs in
--   the class name, but the caster is the player casting directly,
--   so no ev5 direct-path fire site (triggered funnel only).
--   spell_q11306_mixing_blood (:2766-2793): ev4 EFFECT_1 SEND_EVENT;
--   nearest trigger 24042 DoCastSelf(38594) — no binding ID; no
--   DoCastSelf bridge (nearest-creature + self-cast pieces exist).
--   spell_q11306_mixing_vrykul_blood (:2794-2835): ev5 EFFECT_1
--   DUMMY; caster self-casts 43376/43378/43970 (30/30/30) or 43377
--   (10%), triggered — no binding ID.

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   The ev5 player-path fire site is live (a910c39d,
--   spells.go:14668-14737): one fire per (effect, live hit target),
--   effect-major, before that effect's default; missed, immune and
--   GO targets never fire. The tick-295 header claim that ev5 fires
--   on the player path only inside the triggered funnel is STALE.
--   spell_generic_quest_update_entry_SpellScript
--   (spell_quest.cpp:35-77): OnEffectHitTarget, hit creature, not a
--   pet, entry == original -> UpdateEntry(new) + EngageWithTarget
--   (shouldAttack, all three rows) + despawn when set (none here).
--   The IsPet gate has no bridge and is vacuous behind the entry
--   check (the entry pins the exact quest NPC). One factory covers
--   all three bindings: q55_sacred_cleansing (8913, EFFECT_1,
--   Morbent 1200 -> Weakened Morbent 24782), q10255_administer_
--   antidote (34665, EFFECT_0, Helboar 16880 -> Dreadtusk 16992),
--   q11515_fel_siphon_dummy (44936, EFFECT_0, Felblood Initiate
--   24918 -> Emaciated Felblood 24955).
--   spell_q6124_6129_apply_salve (spell_quest.cpp:215-275): EFFECT_0
--   DUMMY ev5. Caster-is-player Load gate + cast-item gate (both
--   explicit checks); Horde cures the sickly gazelle 12296 -> 12297,
--   Alliance the sickly deer 12298 -> 12299, then 30s despawn +
--   KilledMonsterCredit(new entry). player:GetTeam (lua_player.go:386,
--   characters.go teamForRace: Alliance 0, Horde 1) is the C++
--   TeamId twin.
--   spell_q11396_11399_scourging_crystal_controller
--   (spell_quest.cpp:332-367): EFFECT_0 DUMMY ev5, binding 50133.
--   Hit creature with the 43874 force shield and without the 43878
--   controller aura gets 43878 cast on it, non-triggered with the
--   cast item (Go player CastSpell takes no item arg — the cast
--   rides un-attributed; the triggered flag is ignored on the player
--   path, see header note).
--   spell_q11396_11399_scourging_crystal_controller_dummy
--   (spell_quest.cpp:368-397): EFFECT_0 DUMMY ev5, binding 43882.
--   Hit creature loses the 43874 force shield (RemoveAurasDueToSpell
--   twin is creature:RemoveAura, lua_creature_events.go:1676 — the
--   symbolOfLife precedent).
--   spell_q11587_arcane_prisoner_rescue (spell_quest.cpp:428-461):
--   EFFECT_0 DUMMY ev5, binding 45449. 50/50 self-cast Summon Arcane
--   Prisoner Male 45446 / Female 45448, triggered.
--   spell_q11730_ultrasonic_screwdriver (spell_quest.cpp:480-551):
--   EFFECT_0 DUMMY ev5, binding 46023. Caster-is-player + cast-item
--   Load gate (explicit checks); hit creature entry picks the summon
--   (25752 -> 46063, 25753 -> 46068, 25758 -> 46058, 25792 -> 46066,
--   25793 -> 46034), non-triggered self-cast with the cast item (same
--   no-item-arg delta as 50133), then Robot Kill Credit 46027
--   triggered + immediate despawn.
--   spell_q12459_seeds_of_natures_wrath (spell_quest.cpp:556-601):
--   EFFECT_0 DUMMY ev5, binding 49587. Hit creature entry picks the
--   weakened entry (26841 -> 27821, 27808 -> 27809, 27122 -> 27807).
--   spell_q12851_going_bearback (spell_quest.cpp:696-746): AuraScript
--   ONLY, no SpellScript arm — ported as RegisterAuraEvent OnApply
--   = 2 (no AfterApply aura event exists; the arm only reads
--   HasAura(54683) and casts, so the after-vs-on timing is
--   immaterial). Target already ablaze (54683) -> nothing; else the
--   caster's charmer-or-owner-player (GetControllerGUIDS +
--   GetPlayerByGUID, the bested-trigger pattern — a player caster
--   resolves to itself, an unresolvable controller casts nothing,
--   like the C++ null) gets the entry's credit (Frostworg 29358 ->
--   58183, Frostgiant 29351 -> 58184) from the target, and the target
--   self-casts Immolation 54690 + Ablaze 54683, all triggered.
--   DELTAS: Go OnApply is fresh-only vs C++ REAL_OR_REAPPLY, so a
--   re-apply after Ablaze expires re-grants nothing (the HasAura
--   gate makes re-applies no-ops while Ablaze holds).
--   KNOWN LIMIT: player:CastSpell ignores the triggered flag (see
--   header note above), same as all prior player ports.

local SPELL_SACRED_CLEANSING = 8913 -- spell_quest.cpp:79 (class spell_q55_sacred_cleansing)
local NPC_MORBENT = 1200 -- spell_quest.cpp:75
local NPC_WEAKENED_MORBENT = 24782 -- spell_quest.cpp:76
local SPELL_ADMINISTER_ANTIDOTE = 34665 -- spell_quest.cpp:276 (class spell_q10255_administer_antidote)
local NPC_HELBOAR = 16880 -- spell_quest.cpp:272
local NPC_DREADTUSK = 16992 -- spell_quest.cpp:273
local SPELL_FEL_SIPHON_DUMMY = 44936 -- spell_quest.cpp:409 (class spell_q11515_fel_siphon_dummy)
local NPC_FELBLOOD_INITIATE = 24918 -- spell_quest.cpp:405
local NPC_EMACIATED_FELBLOOD = 24955 -- spell_quest.cpp:406
local SPELL_APPLY_SALVE = 19512 -- spell_quest.cpp:215 (class spell_q6124_6129_apply_salve)
local NPC_SICKLY_GAZELLE = 12296 -- spell_quest.cpp:207
local NPC_CURED_GAZELLE = 12297 -- spell_quest.cpp:208
local NPC_SICKLY_DEER = 12298 -- spell_quest.cpp:209
local NPC_CURED_DEER = 12299 -- spell_quest.cpp:210
local TEAM_ALLIANCE = 0 -- SharedDefines.h TeamId
local TEAM_HORDE = 1 -- SharedDefines.h TeamId
local SALVE_DESPAWN_MS = 30000 -- spell_quest.cpp:213 (30s)
local SPELL_SCOURGING_CRYSTAL_CONTROLLER = 50133 -- spell_quest.cpp:332 (class spell_q11396_11399_scourging_crystal_controller)
local SPELL_SCOURGING_CRYSTAL_CHANNEL = 43878 -- spell_quest.cpp:292 (SPELL_SCOURGING_CRYSTAL_CONTROLLER enum)
local SPELL_SCOURGING_CRYSTAL_CONTROLLER_DUMMY = 43882 -- spell_quest.cpp:368 (class spell_q11396_11399_scourging_crystal_controller_dummy)
local SPELL_ARCANE_PRISONER_RESCUE = 45449 -- spell_quest.cpp:428 (class spell_q11587_arcane_prisoner_rescue)
local SPELL_SUMMON_ARCANE_PRISONER_MALE = 45446 -- spell_quest.cpp:424
local SPELL_SUMMON_ARCANE_PRISONER_FEMALE = 45448 -- spell_quest.cpp:425
local SPELL_ULTRASONIC_SCREWDRIVER = 46023 -- spell_quest.cpp:480 (class spell_q11730_ultrasonic_screwdriver)
local SPELL_ROBOT_KILL_CREDIT = 46027 -- spell_quest.cpp:472
local NPC_SCAVENGEBOT_004A8 = 25752 -- spell_quest.cpp:473
local NPC_SENTRYBOT_57K = 25753 -- spell_quest.cpp:474
local NPC_DEFENDOTANK_66D = 25758 -- spell_quest.cpp:475
local NPC_SCAVENGEBOT_005B6 = 25792 -- spell_quest.cpp:476
local NPC_55D_COLLECTATRON = 25793 -- spell_quest.cpp:477
local ULTRASONIC_ROBOT_SUMMONS = {[NPC_SCAVENGEBOT_004A8] = 46063, [NPC_SENTRYBOT_57K] = 46068, [NPC_DEFENDOTANK_66D] = 46058, [NPC_SCAVENGEBOT_005B6] = 46066, [NPC_55D_COLLECTATRON] = 46034} -- spell_quest.cpp:467-471
local SPELL_SEEDS_OF_NATURES_WRATH = 49587 -- spell_quest.cpp:556 (class spell_q12459_seeds_of_natures_wrath)
local NPC_REANIMATED_FROSTWYRM = 26841 -- spell_quest.cpp:546
local NPC_WEAK_REANIMATED_FROSTWYRM = 27821 -- spell_quest.cpp:547
local NPC_TURGID = 27808 -- spell_quest.cpp:549
local NPC_WEAK_TURGID = 27809 -- spell_quest.cpp:550
local NPC_DEATHGAZE = 27122 -- spell_quest.cpp:552
local NPC_WEAK_DEATHGAZE = 27807 -- spell_quest.cpp:553
local SEEDS_WEAK_ENTRIES = {[NPC_REANIMATED_FROSTWYRM] = NPC_WEAK_REANIMATED_FROSTWYRM, [NPC_TURGID] = NPC_WEAK_TURGID, [NPC_DEATHGAZE] = NPC_WEAK_DEATHGAZE} -- spell_quest.cpp:572-574
local SPELL_GOING_BEARBACK = 54798 -- spell_quest.cpp:696 (class spell_q12851_going_bearback)
local NPC_FROSTGIANT = 29351 -- spell_quest.cpp:688
local NPC_FROSTWORG = 29358 -- spell_quest.cpp:689
local SPELL_FROSTGIANT_CREDIT = 58184 -- spell_quest.cpp:690
local SPELL_FROSTWORG_CREDIT = 58183 -- spell_quest.cpp:691
local SPELL_IMMOLATION = 54690 -- spell_quest.cpp:692
local SPELL_ABLAZE = 54683 -- spell_quest.cpp:693

local function questUpdateEntryHit(oldEntry, newEntry, effIndex)
    return function(event, spell, hitTarget, effectIndex)
        if effectIndex ~= effIndex then
            return
        end
        if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
            return
        end
        if hitTarget:GetEntry() == oldEntry then
            hitTarget:UpdateEntry(newEntry)
            local caster = spell:GetCaster()
            if caster ~= nil then
                hitTarget:EngageWithTarget(caster)
            end
        end
    end
end

RegisterSpellEvent(SPELL_SACRED_CLEANSING, 5, questUpdateEntryHit(NPC_MORBENT, NPC_WEAKENED_MORBENT, 1))
RegisterSpellEvent(SPELL_ADMINISTER_ANTIDOTE, 5, questUpdateEntryHit(NPC_HELBOAR, NPC_DREADTUSK, 0))
RegisterSpellEvent(SPELL_FEL_SIPHON_DUMMY, 5, questUpdateEntryHit(NPC_FELBLOOD_INITIATE, NPC_EMACIATED_FELBLOOD, 0))

local function applySalveHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 4 then
        return
    end
    if spell:GetCastItem() == nil then
        return
    end
    local team = caster:GetTeam()
    local newEntry = 0
    if team == TEAM_HORDE and hitTarget:GetEntry() == NPC_SICKLY_GAZELLE then
        newEntry = NPC_CURED_GAZELLE
    elseif team == TEAM_ALLIANCE and hitTarget:GetEntry() == NPC_SICKLY_DEER then
        newEntry = NPC_CURED_DEER
    end
    if newEntry ~= 0 then
        hitTarget:UpdateEntry(newEntry)
        hitTarget:DespawnOrUnsummon(SALVE_DESPAWN_MS)
        caster:KilledMonsterCredit(newEntry)
    end
end

RegisterSpellEvent(SPELL_APPLY_SALVE, 5, applySalveHit)

local function scourgingCrystalControllerHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:HasAura(SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3) and not hitTarget:HasAura(SPELL_SCOURGING_CRYSTAL_CHANNEL) then
        local caster = spell:GetCaster()
        if caster ~= nil then
            caster:CastSpell(hitTarget, SPELL_SCOURGING_CRYSTAL_CHANNEL, false)
        end
    end
end

RegisterSpellEvent(SPELL_SCOURGING_CRYSTAL_CONTROLLER, 5, scourgingCrystalControllerHit)

local function scourgingCrystalControllerDummyHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    hitTarget:RemoveAura(SPELL_FORCE_SHIELD_ARCANE_PURPLE_X3)
end

RegisterSpellEvent(SPELL_SCOURGING_CRYSTAL_CONTROLLER_DUMMY, 5, scourgingCrystalControllerDummyHit)

local function arcanePrisonerRescueHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if math.random(0, 1) == 0 then
        caster:CastSpell(caster, SPELL_SUMMON_ARCANE_PRISONER_MALE, true)
    else
        caster:CastSpell(caster, SPELL_SUMMON_ARCANE_PRISONER_FEMALE, true)
    end
end

RegisterSpellEvent(SPELL_ARCANE_PRISONER_RESCUE, 5, arcanePrisonerRescueHit)

local function ultrasonicScrewdriverHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 4 then
        return
    end
    if spell:GetCastItem() == nil then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local spellId = ULTRASONIC_ROBOT_SUMMONS[hitTarget:GetEntry()]
    if spellId == nil then
        return
    end
    caster:CastSpell(caster, spellId, false)
    caster:CastSpell(caster, SPELL_ROBOT_KILL_CREDIT, true)
    hitTarget:DespawnOrUnsummon()
end

RegisterSpellEvent(SPELL_ULTRASONIC_SCREWDRIVER, 5, ultrasonicScrewdriverHit)

local function seedsOfNaturesWrathHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    local newEntry = SEEDS_WEAK_ENTRIES[hitTarget:GetEntry()]
    if newEntry ~= nil then
        hitTarget:UpdateEntry(newEntry)
    end
end

RegisterSpellEvent(SPELL_SEEDS_OF_NATURES_WRATH, 5, seedsOfNaturesWrathHit)

local function goingBearbackApply(event, aura, target)
    if target:HasAura(SPELL_ABLAZE) then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local player = nil
    if caster:GetObjectType() == "Player" then
        player = caster
    else
        local guid = caster:GetControllerGUIDS()
        if guid ~= 0 then
            player = GetPlayerByGUID(guid)
        end
    end
    if player == nil then
        return
    end
    local entry = target:GetEntry()
    local credit = nil
    if entry == NPC_FROSTWORG then
        credit = SPELL_FROSTWORG_CREDIT
    elseif entry == NPC_FROSTGIANT then
        credit = SPELL_FROSTGIANT_CREDIT
    end
    if credit == nil then
        return
    end
    target:CastSpell(player, credit, true)
    target:CastSpell(target, SPELL_IMMOLATION, true)
    target:CastSpell(target, SPELL_ABLAZE, true)
end

RegisterAuraEvent(SPELL_GOING_BEARBACK, 2, goingBearbackApply)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12937_relief_for_the_fallen (spell_quest.cpp:754-794):
--   EFFECT_0 DUMMY ev5, binding 55804. Caster-is-player Load gate
--   (explicit check); hit creature -> caster self-casts 55809
--   triggered, KilledMonsterCredit(30035), target despawns at once.
--   spell_q12659_ahunaes_knife (spell_quest.cpp:888-924): EFFECT_0
--   DUMMY ev5, binding 52090. Caster-is-player Load gate (explicit
--   check); hit creature despawns at once, caster takes
--   KilledMonsterCredit(28622). Same ev5 fire site as the strip above
--   (a910c39d, spells.go:14668-14737).
--   NEXT: q10041 (SCRIPT_EFFECT, GetNativeGender + PreventHitDefaultEffect
--   both bridged) and q9874 (39199/18240 in-tree) have NO binding spell
--   id anywhere in the C++ file or census (both "no-id") — stay out
--   until a binding id is resolvable; then q12805/q14112/q9452.

local SPELL_HEALING_FINISHED = 55804 -- spell_quest.cpp:747 (class spell_q12937_relief_for_the_fallen)
local SPELL_TRIGGER_AID_OF_THE_EARTHEN = 55809 -- spell_quest.cpp:750
local NPC_FALLEN_EARTHEN_DEFENDER = 30035 -- spell_quest.cpp:751
local SPELL_AHUNAES_KNIFE = 52090 -- spell_quest.cpp:882 (class spell_q12659_ahunaes_knife)
local NPC_SCALPS_KC_BUNNY = 28622 -- spell_quest.cpp:885

local function reliefForTheFallenHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 4 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    caster:CastSpell(caster, SPELL_TRIGGER_AID_OF_THE_EARTHEN, true)
    caster:KilledMonsterCredit(NPC_FALLEN_EARTHEN_DEFENDER)
    hitTarget:DespawnOrUnsummon()
end

RegisterSpellEvent(SPELL_HEALING_FINISHED, 5, reliefForTheFallenHit)

local function ahunaesKnifeHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetTypeId() ~= 4 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    hitTarget:DespawnOrUnsummon()
    caster:KilledMonsterCredit(NPC_SCALPS_KC_BUNNY)
end

RegisterSpellEvent(SPELL_AHUNAES_KNIFE, 5, ahunaesKnifeHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q11010_q11102_q11023_aggro_check (spell_quest.cpp:1565-1592):
--   EFFECT_0 DUMMY ev5, binding 40112 (comment :1564; SPELL_AGGRO_CHECK
--   const already in-file). The hit player with a FLY (201) or
--   MOD_INCREASE_MOUNTED_FLIGHT_SPEED (207) aura self-casts Flak
--   Cannon Trigger 40110. DELTA: C++ casts with
--   TRIGGERED_IGNORE_CASTER_MOUNTED_OR_ON_VEHICLE; Lua takes only a
--   triggered bool (player:CastSpell ignores it, see header note;
--   creature:CastSpell routes triggered through
--   castCreatureSpellTriggered, lua_creature_events.go:936-960) — the
--   ignore-mounted leg has no twin. The 40113/40119 aura arms and
--   40056 were already ported; only this SpellScript arm was missing.
--   COLLISION (same tick, sibling landed mid-strip — see the block
--   below): this tick first ported 50287 + 49370, then found the
--   sibling's versions already in the file and removed its own
--   duplicates. 50287 stays ported via the sibling's block
--   (GetControllerGUID, lua_creature_events.go:519, is the exact
--   GetCharmerOrOwnerGUID twin). 49370 stays out per the sibling's
--   GetOriginalCaster finding (no bridge; lua_spell.go has no
--   OriginalCaster method, server.go:763 is Go-internal only) —
--   this tick's GetCaster() substitution was withdrawn; parent to
--   adjudicate whether the direct-cast identity holds.
--
-- STAY OUT (same tick, rows 698-704):
--   spell_q14076_14092_pound_drum (spell_quest.cpp:1173-1210): no
--   binding spell ID (no comment; the enum holds only 66510/66492);
--   OnCast ev1 50/50 summon leg + GO SetLootState leg both
--   unbindable. spell_q12279_cast_net (:1210-1235): single GO
--   SetLootState leg, no binding ID (no comment, no enum).
--   spell_q12987_read_pronouncement (:1237-1280): AuraScript
--   AfterEffectApply; no binding aura ID in-tree; also the
--   sCreatureTextMgr SendChat(CHAT_MSG_ADDON) leg has no bridge
--   (creature:Talk is AI::Talk, a different channel).
--   Rows 701-704 stay out per the sibling's block below (same
--   verdicts: no binding IDs; q12277 arm otherwise bridgeable).
--   spell_q9452_cast_net GO leg (:1157), re-verified this tick:
--   STAY OUT — ev5 never fires for GO targets on the player path
--   (spells.go:14727, the 0xF110 continue) and GO Use is unbridged
--   (zero "Use" methods in lua_objects.go); SetLootState
--   (lua_objects.go:4324) + SetRespawnTime (:4353) are bridged but
--   unreachable. Rows 691/694/695/696 re-confirmed STAY OUT: no
--   binding spell ID resolvable in-tree (classes :803-841, :930-971,
--   :975-1013, :1083-1116 carry no comment ID).

local SPELL_FLAK_CANNON_TRIGGER = 40110 -- spell_quest.cpp:1528 (Quest11010_11102_11023Data)

local function aggroCheckHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Player" then
        return
    end
    if hitTarget:HasAuraType(SPELL_AURA_FLY) or hitTarget:HasAuraType(SPELL_AURA_MOD_INCREASE_MOUNTED_FLIGHT_SPEED) then
        hitTarget:CastSpell(hitTarget, SPELL_FLAK_CANNON_TRIGGER, true)
    end
end

RegisterSpellEvent(SPELL_AGGRO_CHECK, 5, aggroCheckHit)


-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12372_azure_on_death_force_whisper (spell_quest.cpp:1496-1522):
--   EFFECT_0 SCRIPT_EFFECT ev5, binding 50287 (in-file comment :1491).
--   The hit creature, if a Wyrmrest Defender 27629, whispers Talk(1)
--   to its charmer-or-owner. GetControllerGUID is the exact
--   GetCharmerOrOwnerGUID twin (0 when none, lua_creature_events.go
--   :519) — NOT GetControllerGUIDS, which falls back to the own GUID
--   and would whisper to self. The controller resolves via
--   GetPlayerByGUID / GetCreatureByGUID (the bested-trigger pattern);
--   an absent or unresolvable controller calls Talk(1) with no
--   target, and broadcastCreatureTalk drops whisper rows for a null
--   target exactly like C++ (creature_text.go).
--
-- STAY OUT (same tick, census rows 701-706, verified name-by-name;
--   all six classes have ZERO hits in engine/ + lua_scripts/ outside
--   this note, so the census go_hit/lua_hit=true flags are phantoms):
--   spell_q12987_read_pronouncement (spell_quest.cpp:1244-1277):
--   AuraScript AfterEffectApply; the player target's nearest Ice
--   Spike Bunny 30215 sends chat 1/2 + KilledMonsterCredit(30210) —
--   no binding aura ID in-tree (enum HodirsHelm holds NPCs only);
--   also no SendChat bridge anywhere in engine/world.
--   spell_q12277_wintergarde_mine_explosion (:1288-1334): EFFECT_0
--   DUMMY ev5; the caster must be TYPEID_UNIT with an owner, hit
--   creature 27436/27437 -> caster casts 48744/48745 on the owner,
--   triggered — no binding spell ID in-tree (enum holds NPCs +
--   credit spells only).
--   spell_q12066_bunny_kill_credit (:1341-1366): EFFECT_0 DUMMY
--   ev5; hit creature casts 47390 on the caster, NON-triggered —
--   no binding spell ID in-tree (enum holds only 47390).
--   spell_q12735_song_of_cleansing (:1379-1417): EFFECT_0
--   SCRIPT_EFFECT ev5; caster area 4385/4290/4388 -> self-cast
--   52954/52958/52959, non-triggered — no binding spell ID
--   in-tree (enum holds areas + summon spells only).
--   spell_q12372_cast_from_gossip_trigger (:1424-1449): EFFECT_0
--   SCRIPT_EFFECT ev5; caster self-casts 49207 triggered — no
--   binding spell ID in-tree (enum holds only 49207).
--   spell_q12372_destabilize_azure_dragonshrine_dummy (:1460-1490):
--   EFFECT_0 DUMMY ev5; binding 49370 IS grounded in-file
--   (comment :1451) and GetVehicleKit / GetPassenger /
--   KilledMonsterCredit are all bridged — but the arm reads
--   GetOriginalCaster, which has NO Lua bridge (lua_spell.go has
--   no OriginalCaster method; Go-internal only, server.go:763).
--   Substituting spell:GetCaster() would diverge in the triggered
--   chain the C++ code explicitly handles, so this stays out until
--   a GetOriginalCaster bridge lands.

local SPELL_AZURE_ON_DEATH_FORCE_WHISPER = 50287 -- spell_quest.cpp:1491 (class spell_q12372_azure_on_death_force_whisper)
local NPC_WYRMREST_DEFENDER = 27629 -- spell_quest.cpp:1494
local TALK_AZURE_ON_DEATH_WHISPER = 1 -- spell_quest.cpp:1457 (WHISPER_ON_HIT_BY_FORCE_WHISPER)

local function azureOnDeathForceWhisperHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
        return
    end
    if hitTarget:GetEntry() ~= NPC_WYRMREST_DEFENDER then
        return
    end
    local guid = hitTarget:GetControllerGUID()
    if guid ~= 0 then
        local controller = GetPlayerByGUID(guid)
        if controller == nil then
            controller = hitTarget:GetCreatureByGUID(guid)
        end
        if controller ~= nil then
            hitTarget:Talk(TALK_AZURE_ON_DEATH_WHISPER, controller)
            return
        end
    end
    hitTarget:Talk(TALK_AZURE_ON_DEATH_WHISPER)
end

RegisterSpellEvent(SPELL_AZURE_ON_DEATH_FORCE_WHISPER, 5, azureOnDeathForceWhisperHit)

-- PORTED (tick 2026-11-12, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_q12372_destabilize_azure_dragonshrine_dummy (spell_quest.cpp:1460-1490):
--   EFFECT_0 DUMMY ev5, binding 49370 (in-file comment :1451).
--   ADJUDICATION (parent tick, overruling the 01:54/01:56 stay-out):
--   the arm reads GetOriginalCaster, which has no Lua bridge, but
--   spell:GetCaster() is EXACT here, not approximate: 49370 is a
--   vehicle-cast spell (SpellInfo.cpp:1756: "if vehicle cast this to
--   master" — the Wyrmrest Defender casts it on its passenger), a
--   direct cast with no trigger chain anywhere in the C++ tree (zero
--   49370 sites outside the comment), so original caster IS caster;
--   and Go carries no original-caster concept at all (spells.go
--   :47760 comment), so no bridge could distinguish them. The kit
--   resolves the same from either side: caster = defender -> its
--   kit, passenger 0 is the riding player -> KilledMonsterCredit
--   (27698, Quest12372Data :1455). The hit creature is a presence
--   gate only (C++ ignores it past the null check).
--   spell_q13280_13283_plant_battle_standard (spell_quest.cpp:1029-1055):
--   EFFECT_0 DUMMY ev5, bindings 59643 (horde) + 4338 (alliance),
--   both comment-grounded (:1014-1019); the C++ GetSpellInfo()->Id
--   check reads spell:GetEntry(). The caster roars
--   (EMOTE_ONESHOT_ROAR 15, SharedDefines.h:1950, via PerformEmote,
--   the HandleEmoteCommand twin); a vehicle caster credits its
--   seat-0 player rider 31766 (enum :1022); the hit unit's auras are
--   stripped and it self-casts the faction state spell (59642 horde
--   / 4339 alliance, enum :1024-1025), triggered. NOTE: the credit
--   leg needs the caster to BE the vehicle (the C++ IsVehicle gate);
--   vehicle action-bar casts ride castCreatureSpell in Go, so the
--   creature path reports the vehicle as caster — a player-path cast
--   would skip the credit leg only.
--   STAY OUT (same tick): spell_q13280_13283_jump_jets (:1057-1081):
--   OnCast ev1, no binding spell ID in-tree (enum holds only 4340);
--   the binding-ID hunt agent covers it.

local SPELL_DESTABILIZE_AZURE_DRAGONSHRINE = 49370 -- spell_quest.cpp:1451 (class spell_q12372_destabilize_azure_dragonshrine_dummy)
local NPC_WYRMREST_TEMPLE_CREDIT = 27698 -- spell_quest.cpp:1455 (Quest12372Data)
local SPELL_PLANT_HORDE_BATTLE_STANDARD = 59643 -- spell_quest.cpp:1017 (class spell_q13280_13283_plant_battle_standard)
local SPELL_PLANT_ALLIANCE_BATTLE_STANDARD = 4338 -- spell_quest.cpp:1018
local SPELL_HORDE_BATTLE_STANDARD_STATE = 59642 -- spell_quest.cpp:1024
local SPELL_ALLIANCE_BATTLE_STANDARD_STATE = 4339 -- spell_quest.cpp:1025
local NPC_KING_OF_THE_MOUNTAINT_KC = 31766 -- spell_quest.cpp:1022
local EMOTE_ONESHOT_ROAR = 15 -- SharedDefines.h:1950

local function destabilizeAzureDragonshrineHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    if hitTarget == nil or hitTarget:GetObjectType() ~= "Creature" then
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
    local passenger = kit:GetPassenger(0)
    if passenger == nil or passenger:GetObjectType() ~= "Player" then
        return
    end
    passenger:KilledMonsterCredit(NPC_WYRMREST_TEMPLE_CREDIT)
end

RegisterSpellEvent(SPELL_DESTABILIZE_AZURE_DRAGONSHRINE, 5, destabilizeAzureDragonshrineHit)

local function plantBattleStandardHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:PerformEmote(EMOTE_ONESHOT_ROAR)
    if caster:IsVehicle() then
        local kit = caster:GetVehicleKit()
        if kit ~= nil then
            local player = kit:GetPassenger(0)
            if player ~= nil and player:GetObjectType() == "Player" then
                player:KilledMonsterCredit(NPC_KING_OF_THE_MOUNTAINT_KC)
            end
        end
    end
    if hitTarget == nil then
        return
    end
    local triggeredSpellID = SPELL_ALLIANCE_BATTLE_STANDARD_STATE
    if spell:GetEntry() == SPELL_PLANT_HORDE_BATTLE_STANDARD then
        triggeredSpellID = SPELL_HORDE_BATTLE_STANDARD_STATE
    end
    hitTarget:RemoveAllAuras()
    hitTarget:CastSpell(hitTarget, triggeredSpellID, true)
end

RegisterSpellEvent(SPELL_PLANT_HORDE_BATTLE_STANDARD, 5, plantBattleStandardHit)
RegisterSpellEvent(SPELL_PLANT_ALLIANCE_BATTLE_STANDARD, 5, plantBattleStandardHit)

