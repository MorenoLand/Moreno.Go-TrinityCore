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
--   claim is now STALE in one direction: RegisterSpellEvent ev4
--   (OnEffectHit) EXISTS and fires on the player path (spells.go:14625),
--   so no-target OnEffectHit legs port (q5206/q12634/q12683 ported
--   below); ev5 (OnEffectHitTarget) fires on the player path ONLY
--   inside the triggered-cast funnel (spells.go:28261) — direct casts
--   (finishSpellCast) have no per-effect fire site, so all direct
--   player-cast ev5 legs stay out. Also staying out: going_bearback
--   (AfterApply credit via GetCharmerOrOwnerPlayer — unbridged),
--   quenching_mist + read_pronouncement (aura IDs live only in the
--   world DB spell_script_names, unresolvable in workspace), and the
--   q12619 emblazon effect SpellScript.

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
--   aura, it casts the spell on itself, triggered. KNOWN DELTA:
--   C++ checks HasAuraEffect(reqAuraId, 0) (aura with an effect at
--   index 0); no HasAuraEffect bridge exists, so this uses
--   HasAura(reqAuraId) — identical unless the aura lacks an
--   effect-0, which no DBC aura carries here.
--   The ev4 hook fires on the player path (spells.go:14625, ahead of
--   the effects loop, matching C++ HIT-phase order); the ev5
--   OnEffectHitTarget hook fires on the player path ONLY inside the
--   triggered-cast funnel (spells.go:28261), so direct-cast ev5 legs
--   stay out on that leg alone.
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
    if reqAuraId > 0 and caster:HasAura(reqAuraId) then
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
--   (ev5 has no direct-path fire site; only the triggered funnel
--   fires it, spells.go:28261). If the hit creature has Permanent
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
local SPELL_AHUNAES_KNIFE = 52090 -- spell_quest.cpp:887 (class spell_q12659_ahunaes_knife)
local NPC_SCALPS_KC_BUNNY = 28622 -- spell_quest.cpp:891
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

local function ahunaesKnifeHit(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local target = spell:GetTarget()
    if target == nil or target:GetObjectType() ~= "Creature" then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    if caster:GetTypeId() ~= 4 then
        return
    end
    target:DespawnOrUnsummon()
    caster:KilledMonsterCredit(NPC_SCALPS_KC_BUNNY)
end

RegisterSpellEvent(SPELL_AHUNAES_KNIFE, 4, ahunaesKnifeHit)

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
