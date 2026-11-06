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
-- CLOSED (tick 295 census): the remaining quest classes are
--   SpellScript-only (62, no RegisterSpellScript bridge), plus
--   going_bearback (AfterApply credit via GetCharmerOrOwnerPlayer —
--   unbridged), quenching_mist + read_pronouncement (aura IDs live
--   only in the world DB spell_script_names, unresolvable in
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
