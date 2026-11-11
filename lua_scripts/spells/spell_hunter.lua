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

-- PORTED (this tick, RegisterSpellEvent OnEffectHit = 4):
--   spell_hun_last_stand_pet::HandleDummy (spell_hunter.cpp:720-726):
--   OnEffectHitTarget, EFFECT_0 SPELL_EFFECT_DUMMY. The pet casts
--   the triggered heal 53479 on itself with BP0 = 30% of its max
--   health (CountPctFromMaxHealth, creature bridge
--   lua_creature_events.go:466); CastCustomSpell carries the BP0
--   override (lua_creature_events.go:936+, spell_dk.lua precedent).
--   Pet casts ride castCreatureSpellInternalWithBasePoints, whose
--   ev4 fire site is boss_ai.go:3365. DORMANT until Spell.dbc drives
--   53478 (pet ability).
local SPELL_HUNTER_PET_LAST_STAND = 53478 -- spell_hunter.cpp:706
local SPELL_HUNTER_PET_LAST_STAND_TRIGGERED = 53479 -- spell_hunter.cpp:50

local function lastStandPetDummy(event, spell, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    caster:CastCustomSpell(caster, SPELL_HUNTER_PET_LAST_STAND_TRIGGERED, true, caster:CountPctFromMaxHealth(30))
end

RegisterSpellEvent(SPELL_HUNTER_PET_LAST_STAND, 4, lastStandPetDummy)

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
