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
--   (spell_hunter.cpp:194-206): EFFECT_0 SPELL_AURA_OBS_MOD_POWER,
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
