-- Hunter spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_hunter.cpp.
--
-- PORTED (tick 198, RegisterAuraEvent OnApply = 2):
--   spell_hun_aspect_of_the_beast_pet::OnApply
--   (spell_hunter.cpp:143-151): EFFECT_0 SPELL_AURA_UNTRACKABLE.
--   On apply, the pet's spell-mod owner must have Aspect of the
--   Beast 13161, or the aura removes itself. The C++ Load() gate
--   (GetSpellModOwner() non-null) is the owner-nil return below:
--   with no owner the script would not load, so nothing is removed.
--   The Go twin for GetSpellModOwner is target:GetOwner() — a pet's
--   spell mods come from its owner, and the owner object carries
--   HasAura.

local SPELL_ASPECT_OF_THE_BEAST_PET = 61669 -- spell_hunter.cpp:130
local SPELL_HUNTER_ASPECT_OF_THE_BEAST = 13161 -- spell_hunter.cpp:35

local function aspectOfTheBeastPetApply(event, aura, target)
    if target:GetObjectType() ~= "Creature" then
        return
    end
    local owner = target:GetOwner()
    if owner == nil then
        return
    end
    if not owner:HasAura(SPELL_HUNTER_ASPECT_OF_THE_BEAST) then
        aura:Remove()
    end
end

RegisterAuraEvent(SPELL_ASPECT_OF_THE_BEAST_PET, 2, aspectOfTheBeastPetApply)

-- PORTED (this tick, RegisterAuraEvent OnApply = 2):
--   spell_hun_aspect_of_the_beast::OnApply
--   (spell_hunter.cpp:111-116): EFFECT_0 SPELL_AURA_UNTRACKABLE,
--   AfterEffectApply REAL. With a pet out the player casts the pet
--   aura 61669 on self, triggered; the cast's TARGET_UNIT_PET
--   implicit target lands it on the pet (spells.go:8942), where the
--   port above checks the owner still has 13161. The C++ Load()
--   player gate holds by construction (the aura is player-only).
--   The triggered flag rides the same pre-existing gap as the other
--   ports (player:CastSpell ignores it, lua_player.go:2577).
-- NOT PORTED: OnRemove (spell_hunter.cpp:103-109) — the pet must
--   drop 61669, but there is no player-side bridge to resolve the
--   pet GUID into a creature object (GetCreatureByGUID lives only
--   on creature objects, lua_creature_events.go:4050; no global).
--   The pet keeps 61669 until it is re-applied and self-removes.
local function aspectOfTheBeastApply(event, aura, target)
    if target:GetPetGUID() == 0 then
        return
    end
    target:CastSpell(target, SPELL_ASPECT_OF_THE_BEAST_PET, true)
end

RegisterAuraEvent(SPELL_HUNTER_ASPECT_OF_THE_BEAST, 2, aspectOfTheBeastApply)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_cobra_strikes_triggered::HandleStackDrop
--   (spell_hunter.cpp:361-364): OnEffectProc, EFFECT_0
--   SPELL_AURA_ADD_FLAT_MODIFIER — each proc drops one stack.
--   C++ calls no PreventDefaultAction; the default leg is a no-op
--   (no DBC trigger spell), so the handler returns nothing. The Go
--   proc loop reaches the aura only because this tick added
--   ADD_FLAT_MODIFIER to isProcTriggerAuraType (procs.go; TC
--   SpellMgr.cpp:1716 lists it in isTriggerAura). ModStackAmount(-1)
--   removes the aura at 0 stacks; Lua SetStackAmount does not, so the
--   last stack removes explicitly. DORMANT until Spell.dbc ships the
--   53257 proc entry.
local SPELL_HUNTER_COBRA_STRIKES_TRIGGERED = 53257 -- spell_hunter.cpp:351

local function cobraStrikesTriggeredProc(event, aura, target, actor, procTarget, damage)
    local stacks = aura:GetStackAmount()
    if stacks <= 1 then
        aura:Remove()
    else
        aura:SetStackAmount(stacks - 1)
    end
end

RegisterAuraEvent(SPELL_HUNTER_COBRA_STRIKES_TRIGGERED, 6, cobraStrikesTriggeredProc)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_cobra_strikes::HandleProc
--   (spell_hunter.cpp:327-338): OnEffectProc, EFFECT_0
--   SPELL_AURA_PROC_TRIGGER_SPELL. PreventDefaultAction (the true
--   return); the holder casts the DBC trigger spell (53257, read via
--   aura:GetTriggerSpell(0), lua_aura.go:294) on itself, triggered.
--   The Lua cast corrects the target vs the Go default proc leg,
--   which would cast the trigger spell on the proc target
--   (procs.go:5505) instead of the holder self-cast C++ does.
--   KNOWN DEVIATION: C++ passes SPELLVALUE_AURA_STACK =
--   triggeredSpellInfo->StackAmount (53257's DBC stack count, the
--   2-charge grant), but no Lua cast bridge carries an aura-stack
--   mod — player:CastSpell has no stack arg and CastCustomSpell
--   overrides base points only (lua_player.go:2934/2956). The Go
--   cast path applies fresh auras at StackCount 0, read as 1
--   (spells.go:23032-23037), so this port grants 1 charge where
--   C++ grants the DBC stack count; the 53257 stack-drop handler
--   above then consumes it.
--   DORMANT until Spell.dbc ships the 53256 proc entry.
local SPELL_HUNTER_COBRA_STRIKES = 53256 -- spell_hunter.cpp:313

local function cobraStrikesProc(event, aura, target, actor, procTarget, damage)
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    target:CastSpell(target, triggerSpell, true)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_COBRA_STRIKES, 6, cobraStrikesProc)

-- NOT IN LUA: spell_hun_glyph_of_arcane_shot (56841, spell_hunter.cpp:410-467)
-- lives in Go (procs.go): the CheckProc sting gate runs before the chance
-- roll and the HandleProc mana-refund arm (61389, BP0 = pct of the triggering
-- spell's power cost) sits in the session proc loop. Lua cannot port it: the
-- ev6 hook carries no proc-spell object (no power-cost read) and has no
-- family-flag aura scan. See the procs.go comments for the DORMANT note and
-- the non-triggered-cast deviation.

-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_hun_ascpect_of_the_viper::OnApply/OnRemove
--   (spell_hunter.cpp:194-208): EFFECT_0 SPELL_AURA_OBS_MOD_POWER,
--   AfterEffectApply/AfterEffectRemove REAL. Hunter T7 4P bonus:
--   while the hunter has Viper Attack Speed 60144, applying the
--   aspect casts Vicious Viper 61609 on self (aurEff args), and
--   removing it drops 61609. The triggered flag rides the same
--   pre-existing gap as the other ports (player:CastSpell ignores
--   it, lua_player.go:2857).
-- NOT PORTED: HandleProc (spell_hunter.cpp:179-192) — OBS_MOD_POWER
--   is not a proc-trigger aura type (procs.go:1025), so the player
--   proc loop never fires ev6 for 34074; the mana math also needs
--   ranged attack time (no GetAttackTime bridge) and the glyph
--   56851 effect amount. In TC the proc works only via a spell_proc
--   DB row (OBS_MOD_POWER is not a TC trigger aura either,
--   SpellMgr.cpp:1686-1729), and Go has no spell_proc table — the leg
--   needs a spell_proc import, not a Lua handler. The Go periodic
--   OBS_MOD_POWER tick (spells.go:41749) regenerates amount% of max
--   power per tick instead, a pre-existing approximation.
local SPELL_HUNTER_ASPECT_OF_THE_VIPER = 34074 -- spell_hunter.cpp:37
local SPELL_HUNTER_VIPER_ATTACK_SPEED = 60144 -- spell_hunter.cpp:61
local SPELL_HUNTER_VICIOUS_VIPER = 61609 -- spell_hunter.cpp:60

local function aspectOfTheViperApply(event, aura, target)
    if target:HasAura(SPELL_HUNTER_VIPER_ATTACK_SPEED) then
        target:CastSpell(target, SPELL_HUNTER_VICIOUS_VIPER, true)
    end
end

RegisterAuraEvent(SPELL_HUNTER_ASPECT_OF_THE_VIPER, 2, aspectOfTheViperApply)

local function aspectOfTheViperRemove(event, aura, target)
    if target:HasAura(SPELL_HUNTER_VIPER_ATTACK_SPEED) then
        target:RemoveAura(SPELL_HUNTER_VICIOUS_VIPER)
    end
end

RegisterAuraEvent(SPELL_HUNTER_ASPECT_OF_THE_VIPER, 3, aspectOfTheViperRemove)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_glyph_of_mend_pet::HandleProc
--   (spell_hunter.cpp:487-491): EFFECT_0 SPELL_AURA_DUMMY.
--   PreventDefaultAction (the true return); the proc target — the
--   Mend Pet heal target — casts Glyph of Mend Pet Happiness 57894
--   on itself, triggered (aurEff args). The ev6 hook fires per
--   eligible effect after the proc gates (procs.go:3262), and the
--   heal arms feed it with the heal target as procTarget
--   (procs.go:1838/1980); the DUMMY default leg is nothing, so the
--   veto only mirrors the PreventDefaultAction. C++ casts on
--   nullptr; the Lua self-target is the twin — the happiness buff
--   is self-targeted, and the creature CastSpell nil-target
--   fallback would resolve to the pet's attack target instead.
--   DORMANT until Spell.dbc ships the 57870 proc flags
--   (serverSpellProcEntryFor needs ProcTypeMask != 0, procs.go:1222).
local SPELL_HUNTER_GLYPH_OF_MEND_PET = 57870 -- spell_hunter.cpp:473
local SPELL_HUNTER_GLYPH_OF_MEND_PET_HAPPINESS = 57894 -- spell_hunter.cpp:71

local function glyphOfMendPetProc(event, aura, target, actor, procTarget, damage)
    if procTarget == nil then
        return true
    end
    procTarget:CastSpell(procTarget, SPELL_HUNTER_GLYPH_OF_MEND_PET_HAPPINESS, true)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_GLYPH_OF_MEND_PET, 6, glyphOfMendPetProc)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_hunting_party::HandleProc
--   (spell_hunter.cpp:577-580): OnEffectProc, EFFECT_0
--   SPELL_AURA_DUMMY. PreventDefaultAction (the true return); the
--   proc actor casts Replenishment 57669 on itself, triggered
--   (aurEff args; the C++ nullptr target is the self-cast, the
--   mend-pet precedent). DUMMY is a proc-trigger aura type
--   (procs.go:1322), so ev6 can fire; DORMANT until Spell.dbc ships
--   the 53290 proc entry.
local SPELL_HUNTER_HUNTING_PARTY = 53290 -- spell_hunter.cpp:561
local SPELL_REPLENISHMENT = 57669 -- spell_hunter.cpp:68

local function huntingPartyProc(event, aura, target, actor, procTarget, damage)
    if actor == nil then
        return true
    end
    actor:CastSpell(actor, SPELL_REPLENISHMENT, true)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_HUNTING_PARTY, 6, huntingPartyProc)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_improved_mend_pet::CheckProc + HandleProc
--   (spell_hunter.cpp:609-617): DoCheckProc rolls the EFFECT_0
--   amount as the chance (roll_chance_i, the invigoration
--   math.random precedent); OnEffectProc, EFFECT_0 SPELL_AURA_DUMMY.
--   PreventDefaultAction (the true return); the holder casts
--   Improved Mend Pet 24406 on itself, triggered (aurEff args).
--   The failed roll returns without casting — the DUMMY default leg
--   is a no-op, so the veto is harmless either way. DORMANT until
--   Spell.dbc ships the 19572 proc entry.
local SPELL_HUNTER_IMPROVED_MEND_PET = 19572 -- spell_hunter.cpp:594
local SPELL_HUNTER_IMPROVED_MEND_PET_TRIGGERED = 24406 -- spell_hunter.cpp:45

local function improvedMendPetProc(event, aura, target, actor, procTarget, damage)
    if math.random(100) > aura:GetAmount(0) then
        return true
    end
    target:CastSpell(target, SPELL_HUNTER_IMPROVED_MEND_PET_TRIGGERED, true)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_IMPROVED_MEND_PET, 6, improvedMendPetProc)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_hun_invigoration::HandleScriptEffect
--   (spell_hunter.cpp:647-653): OnEffectHitTarget, EFFECT_0
--   SPELL_EFFECT_SCRIPT_EFFECT. The hit unit's Invigoration talent
--   aura (family HUNTER, icon 3487) rolls its amount as the chance;
--   on success the hit unit casts the energize 53398 on itself,
--   triggered. GetDummyAuraEffect is the C++ twin on both surfaces
--   (player lua_player.go:2687, creature lua_creature_events.go:1464);
--   its !SpellFamilyFlags gate is C++-exact (Unit.cpp:4510-4522), so
--   the port is exactly as correct as the C++ arm. roll_chance_i is
--   math.random(100) <= amount (kalecgos precedent). DORMANT until
--   Spell.dbc drives 53412's script-effect cast (proc data).
local SPELL_HUNTER_INVIGORATION = 53412 -- spell_hunter.cpp:633
local SPELL_HUNTER_INVIGORATION_TRIGGERED = 53398 -- spell_hunter.cpp:46
local SPELLFAMILY_HUNTER = 9 -- SharedDefines.h:3590
local SPELL_HUNTER_INVIGORATION_ICON = 3487 -- spell_hunter.cpp:651

local function invigorationScript(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local amount = target:GetDummyAuraEffect(SPELLFAMILY_HUNTER, SPELL_HUNTER_INVIGORATION_ICON, 0)
    if amount == nil then
        return
    end
    if math.random(100) <= amount then
        target:CastSpell(target, SPELL_HUNTER_INVIGORATION_TRIGGERED, true)
    end
end

RegisterSpellEvent(SPELL_HUNTER_INVIGORATION, 5, invigorationScript)

-- 53478 Last Stand (pet): PORTED IN GO (2026-11-10), not Lua.
--   spell_hun_last_stand_pet::HandleDummy (spell_hunter.cpp:720-726)
--   is a pet-cast EFFECT_0 DUMMY arm, but pet casts ride
--   executePetSpellWithOptions, which has no fireSpellEffectHit site —
--   the ev4 Lua handler landed 2026-11-10 could never fire. The leg
--   lives in the pet-path DUMMY dispatch (pet_combat.go): the pet
--   casts the triggered heal 53479 on itself with BP0 = 30% of its
--   max health. DORMANT until Spell.dbc drives 53478 (pet ability).

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_hun_masters_call::HandleScriptEffect
--   (spell_hunter.cpp:876-879): OnEffectHitTarget, EFFECT_1
--   SPELL_EFFECT_SCRIPT_EFFECT. The hit unit casts the triggered
--   freedom effect 62305 on itself (C++ nullptr target = self-cast;
--   the Lua self-target is the twin, mend-pet precedent).
-- NOT PORTED: DoCheckCast (spell_hunter.cpp:838-869) — no Lua
--   OnCheckCast bridge, and Go's default enforces none of the
--   pet-alive / charmed / stunned / fleeing / confused / LOS legs.
--   HandleDummy eff0 (spell_hunter.cpp:871-874) — the pet casts
--   GetEffectValue() on the hit unit; spell:GetEffectValue(0)
--   exists (lua_spell.go:215) but there is no player-side
--   pet-object bridge (only GetPetGUID, lua_player.go:1466;
--   GetCreatureByGUID is creature-object-only).
local SPELL_HUNTER_MASTERS_CALL = 53271 -- spell_hunter.cpp:814
local SPELL_HUNTER_MASTERS_CALL_TRIGGERED = 62305 -- spell_hunter.cpp:47

local function mastersCallScript(event, spell, target, effectIndex)
    if effectIndex ~= 1 or target == nil then
        return
    end
    target:CastSpell(target, SPELL_HUNTER_MASTERS_CALL_TRIGGERED, true)
end

RegisterSpellEvent(SPELL_HUNTER_MASTERS_CALL, 5, mastersCallScript)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_misdirection::HandleProc (spell_hunter.cpp:917-921):
--   OnEffectProc, EFFECT_1 SPELL_AURA_DUMMY. PreventDefaultAction
--   (the true return); the target casts the redirect proc aura
--   35079 on itself, triggered (aurEff args ride the standard
--   triggered-flag gap; the redirect pct comes from 35079's DBC
--   effect, registered by handleEffectRedirectThreat,
--   spell_threat.go:185). DUMMY is a proc-trigger aura type
--   (procs.go:1077), so ev6 can fire; DORMANT until Spell.dbc ships
--   the 34477 proc entry. The OnRemove arm is Go-covered:
--   removeAuraWithMode unregisters the 35079 redirect key
--   (spells.go:35331); the 34477-key unregister is a dead arm —
--   registration keys the proc spell's id.
local SPELL_HUNTER_MISDIRECTION = 34477 -- spell_hunter.cpp:48
local SPELL_HUNTER_MISDIRECTION_PROC = 35079 -- spell_hunter.cpp:49

local function misdirectionProc(event, aura, target, actor, procTarget, damage)
    target:CastSpell(target, SPELL_HUNTER_MISDIRECTION_PROC, true)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_MISDIRECTION, 6, misdirectionProc)

-- PORTED (this tick, RegisterSpellEvent OnEffectHit = 4):
--   spell_hun_pet_carrion_feeder::HandleDummy
--   (spell_hunter.cpp:1001-1005): OnEffectHit, EFFECT_0
--   SPELL_EFFECT_DUMMY. The pet self-casts the triggered heal 54045,
--   non-triggered (C++ passes false; creature CastSpell honors the
--   flag, lua_creature_events.go:953). The C++ Load() IsPet gate rides
--   the in-tree approximation (nonzero OwnerGUID,
--   lua_creature_events.go:4253) — no Lua IsPet method exists.
--   NOT PORTED: CheckIfCorpseNear (spell_hunter.cpp:1010-1026) — no Lua
--   OnCheckCast bridge, and Go's default enforces no corpse search
--   (53271 DoCheckCast precedent). DORMANT until Spell.dbc drives
--   54044 (pet ability).
local SPELL_HUNTER_PET_CARRION_FEEDER = 54044 -- spell_hunter.cpp:964
local SPELL_HUNTER_PET_CARRION_FEEDER_TRIGGERED = 54045 -- spell_hunter.cpp:54

local function carrionFeederDummy(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetOwnerGUID() == 0 then
        return
    end
    caster:CastSpell(caster, SPELL_HUNTER_PET_CARRION_FEEDER_TRIGGERED, false)
end

RegisterSpellEvent(SPELL_HUNTER_PET_CARRION_FEEDER, 4, carrionFeederDummy)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_hun_pet_heart_of_the_phoenix::HandleScript
--   (spell_hunter.cpp:1043-1055): OnEffectHitTarget, EFFECT_0
--   SPELL_EFFECT_SCRIPT_EFFECT. With an owner and no 55711 debuff on
--   the pet, the owner casts the resurrect 54114 on the pet with BP0 =
--   100, triggered (player CastCustomSpell, lua_player.go:2879), then
--   the pet self-casts the debuff 55711, triggered. The C++ Load()
--   IsPet gate rides the nonzero-OwnerGUID approximation (see above);
--   creature HasAura is bridged (lua_creature_events.go:1414) and
--   GetOwner returns the player object (lua_creature_events.go:627).
--   DORMANT until Spell.dbc drives 55709 (pet ability).
local SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX = 55709 -- spell_hunter.cpp:51
local SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX_TRIGGERED = 54114 -- spell_hunter.cpp:52
local SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX_DEBUFF = 55711 -- spell_hunter.cpp:53

local function heartOfThePhoenixScript(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetOwnerGUID() == 0 then
        return
    end
    local owner = caster:GetOwner()
    if owner == nil then
        return
    end
    if caster:HasAura(SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX_DEBUFF) then
        return
    end
    owner:CastCustomSpell(caster, SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX_TRIGGERED, true, 100)
    caster:CastSpell(caster, SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX_DEBUFF, true)
end

RegisterSpellEvent(SPELL_HUNTER_PET_HEART_OF_THE_PHOENIX, 5, heartOfThePhoenixScript)

-- PORTED (this tick, RegisterAuraEvent OnProc = 6):
--   spell_hun_roar_of_sacrifice::HandleProc
--   (spell_hunter.cpp:1294-1301): OnEffectProc, EFFECT_1
--   SPELL_AURA_DUMMY. PreventDefaultAction (the true return); the
--   action target casts the triggered 67481 on the aura caster with
--   BP0 = pct(damage, eff1 amount) (CalculatePct, math.floor
--   precedent in spell_priest.lua). The CheckProc school-mask gate
--   lives in Go, not here: ev6 args carry no school, and the aura
--   object has no MiscValue bridge — procs.go adds the
--   roarOfSacrificeProcAllowed gate ahead of the chance roll
--   (56841 precedent), so this handler only fires on gated events.
--   DUMMY is a proc-trigger aura type (procs.go:1077), so ev6 can
--   fire; DORMANT until Spell.dbc ships the 53480 proc entry.
local SPELL_HUNTER_ROAR_OF_SACRIFICE = 53480 -- spell_hunter.cpp:1265
local SPELL_ROAR_OF_SACRIFICE_TRIGGERED = 67481 -- spell_hunter.cpp:63

local function roarOfSacrificeProc(event, aura, target, actor, procTarget, damage)
    if procTarget == nil or damage == nil then
        return
    end
    local caster = aura:GetCaster()
    if caster == nil then
        return
    end
    local bp0 = math.floor(damage * aura:GetAmount(1) / 100)
    procTarget:CastCustomSpell(caster, SPELL_ROAR_OF_SACRIFICE_TRIGGERED, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_ROAR_OF_SACRIFICE, 6, roarOfSacrificeProc)

-- PORTED (this tick, RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_hun_scatter_shot::HandleDummy
--   (spell_hunter.cpp:1328-1334): OnEffectHitTarget, EFFECT_0
--   SPELL_EFFECT_DUMMY. The hunter interrupts Auto Shot
--   (CURRENT_AUTOREPEAT_SPELL = 3 in the Go
--   luaCastSpellTypeArg numbering, lua_creature_events.go:322)
--   and stops attacking (AttackStop, lua_player.go:3085). The
--   C++ Load() player gate rides GetObjectType() == "Player"
--   (maiden-of-virtue precedent).
-- NOT PORTED: SendAttackSwingCancelAttack (spell_hunter.cpp:1334)
--   — no bridge, and Go has no SMSG_ATTACK_SWING_CANCEL opcode
--   (spells.go:38973); AttackStop already sends SMSG_ATTACK_STOP,
--   so only the client swing-cancel display is missing.
--   DORMANT until Spell.dbc drives 37506's dummy cast.
local SPELL_HUNTER_SCATTER_SHOT = 37506 -- spell_hunter.cpp:1313

local function scatterShotDummy(event, spell, target, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    caster:InterruptSpell(3)
    caster:AttackStop()
end

RegisterSpellEvent(SPELL_HUNTER_SCATTER_SHOT, 5, scatterShotDummy)

-- PORTED (this tick, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_hun_viper_attack_speed::OnApply/OnRemove
--   (spell_hunter.cpp:1638-1652): EFFECT_0 SPELL_AURA_DUMMY,
--   AfterEffectApply/AfterEffectRemove REAL. On apply, a hunter
--   already running Aspect of the Viper 34074 casts Vicious Viper
--   61609 on self (aurEff args; the triggered flag rides the
--   pre-existing player:CastSpell gap, lua_player.go:2857); on
--   remove the hunter drops 61609 unconditionally. The twin leg
--   lives in the 34074 handlers above (aspect apply/remove check
--   60144), so whichever of the two applies second casts 61609
--   and whichever removes first drops it.
local function viperAttackSpeedApply(event, aura, target)
    if target:HasAura(SPELL_HUNTER_ASPECT_OF_THE_VIPER) then
        target:CastSpell(target, SPELL_HUNTER_VICIOUS_VIPER, true)
    end
end

RegisterAuraEvent(SPELL_HUNTER_VIPER_ATTACK_SPEED, 2, viperAttackSpeedApply)

local function viperAttackSpeedRemove(event, aura, target)
    target:RemoveAura(SPELL_HUNTER_VICIOUS_VIPER)
end

RegisterAuraEvent(SPELL_HUNTER_VIPER_ATTACK_SPEED, 3, viperAttackSpeedRemove)

-- PORTED (this tick, RegisterSpellEvent OnObjectAreaTargetSelect = 7):
--   spell_hun_target_only_pet_and_owner::FilterTargets
--   (spell_hunter.cpp:1495-1501): EFFECT_0 and EFFECT_1,
--   TARGET_UNIT_CASTER_AREA_PARTY. The list is replaced with the
--   caster plus its owner — Call of the Wild 53434 and Furious
--   Howl 24604 buff the pet and its hunter only, never the party
--   (bpet_hunter.cpp:42/321-324 casts 53434 as the pet, "10% AP
--   for pet and hunter"). ev7 fires on the creature-cast path
--   only (boss_ai.go:3151); pet casts ride
--   castCreatureSpellInternalWithBasePoints (53478 precedent).
--   The player-cast leg lives in Go (spells.go): a hunter casting
--   either spell keeps the caster alone, since a player has no
--   owner (Unit.h:1147). DORMANT until Spell.dbc drives the casts.
local SPELL_HUNTER_CALL_OF_THE_WILD = 53434 -- spell_hunter.cpp:1485
local SPELL_HUNTER_FURIOUS_HOWL = 24604 -- spell_hunter.cpp:1485

local function targetOnlyPetAndOwnerFilter(event, spell, effectIndex, targets)
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    targets:Clear()
    targets:Add(caster)
    local owner = caster:GetOwner()
    if owner ~= nil then
        targets:Add(owner)
    end
end

RegisterSpellEvent(SPELL_HUNTER_CALL_OF_THE_WILD, 7, targetOnlyPetAndOwnerFilter)
RegisterSpellEvent(SPELL_HUNTER_FURIOUS_HOWL, 7, targetOnlyPetAndOwnerFilter)

-- PORTED (this tick, RegisterAuraEvent OnPeriodic = 1):
--   spell_hun_rapid_recuperation::HandlePeriodic
--   (spell_hunter.cpp:1140-1148): OnEffectPeriodic, EFFECT_0
--   SPELL_AURA_PERIODIC_TRIGGER_SPELL. PreventDefaultAction (the
--   true return, geddon infernoTick precedent) — the Go default
--   periodic leg would cast the trigger spell with its DBC base
--   points instead of the mana-pct override. BP0 = CalculatePct of
--   the target's max mana by the EFFECT_0 amount (math.floor,
--   roar-of-sacrifice precedent); the holder casts the DBC trigger
--   spell (read via aura:GetTriggerSpell(0), lua_aura.go:294) on
--   itself, triggered (aurEff args). One handler serves both ranks
--   (56654 rank 1, 58882 rank 2 — same class, spell_hunter.cpp:1124).
--   The session periodic tick fires ev1 for player auras
--   (spells.go:41587). DORMANT until Spell.dbc drives the
--   56654/58882 periodic auras.
local SPELL_HUNTER_RAPID_RECUPERATION_R1 = 56654 -- spell_hunter.cpp:69
local SPELL_HUNTER_RAPID_RECUPERATION_R2 = 58882 -- spell_hunter.cpp:70

local function rapidRecuperationTick(event, aura, target)
    local triggerSpell = aura:GetTriggerSpell(0)
    if triggerSpell == 0 then
        return true
    end
    local bp0 = math.floor(target:GetMaxPower(0) * aura:GetAmount(0) / 100)
    target:CastCustomSpell(target, triggerSpell, true, bp0)
    return true
end

RegisterAuraEvent(SPELL_HUNTER_RAPID_RECUPERATION_R1, 1, rapidRecuperationTick)
RegisterAuraEvent(SPELL_HUNTER_RAPID_RECUPERATION_R2, 1, rapidRecuperationTick)

-- NOT IN LUA: spell_hun_disengage (781, spell_hunter.cpp:379-400)
-- — OnCheckCast only (player out of combat fails the cast with
-- SPELL_FAILED_CANT_DO_THAT_RIGHT_NOW). No Lua OnCheckCast bridge
-- exists (masters-call precedent above), and Go's default enforces
-- none of the gate. CENSUS CORRECTION: the 2026-11-10 census marks
-- row 594 go_hit/lua_hit True, but no 781 handling exists anywhere
-- in the Go tree — the hits are the verb "disengage" in unrelated
-- comments (threat.go, pets.go, lua_creature_events.go). The row is
-- uncovered; the gate needs a CheckCast bridge or a Go leg.

-- NOT IN LUA: spell_hun_lock_and_load (56342, spell_hunter.cpp:741-805)
-- — DoCheckProc (marker-aura 67544 gate, ev6-readable via
-- actor:HasAura) plus two DoCheckEffectProc arms the ev6 hook
-- cannot express: the trap arm needs the proc type mask
-- (PROC_FLAG_DONE_TRAP_ACTIVATION) and the triggering spell's
-- frost school mask, and the periodic arm needs
-- PROC_FLAG_DONE_PERIODIC; the ev6 args carry no type mask, no
-- proc spell, and no effect index, so the handler cannot tell a
-- trap proc from a periodic proc. STAYS OUT until ev6
-- event-spell/school/type-mask forwarding lands (open engine work,
-- same as census rows 547-551 + 572).

-- NOT IN LUA: spell_hun_piercing_shots (53234, spell_hunter.cpp:1070-1119)
-- — the HandleProc math is ev6-expressible (damage arg, actor,
-- procTarget; bp = CalculatePct(damage, eff0 amount) / max ticks,
-- cast 63468 with BP0 via CastCustomSpell, roar-of-sacrifice
-- precedent) except for one read: piercingShots->GetMaxTicks(),
-- the 63468 DoT's tick count, has no cross-spell bridge — aura
-- has GetTotalTicks only for its own spell (lua_aura.go) and
-- GetSpellEffectCalcValue reads base points, not duration/
-- amplitude. RECIPE: add an aura:GetSpellTotalTicks(spellId)
-- bridge on the GetSpellEffectCalcValue precedent (DBC duration /
-- amplitude, +1 for START_PERIODIC_AT_APPLY), then port the
-- handler with a procTarget-nil CheckProc gate.
