-- Warrior spell aura scripts -- Lua ports of the AuraScript arms in
-- src/server/scripts/Spells/spell_warrior.cpp.
--
-- PORTED (tick 201, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_warr_vigilance (50720, spell_warrior.cpp:1088-1156):
--   EFFECT_0 PROC_TRIGGER_SPELL, REAL_OR_REAPPLY. OnApply the target
--   casts the Damage Reduction aura (68066) on itself, triggered, and
--   casts Vigilance Redirect Threat (59665) on the caster, triggered.
--   OnRemove, if the target still has 68066 and has none of Blessing
--   of Sanctuary (20911), Greater Blessing of Sanctuary (25899) or
--   Renewed Hope (63944), it drops 68066. C++ uses
--   RemoveAurasDueToSpell; the Go twin is target:RemoveAura(id):
--   68066 is self-cast once by the apply leg, so a single application
--   exists and removing the spell's aura is the same removal.
--   DELTA: the Go apply hook is fresh-only (C++ REAL_OR_REAPPLY also
--   fires on re-apply); same pre-existing delta as Renew (tick 199).
-- STILL BLOCKED (same class):
--   HandleRemove's UnregisterRedirectThreat(59665, caster GUID) leg
--   (spell_warrior.cpp:1128) — no threat-redirect bridge.
--   CheckProc / HandleProc (:1130-1144): the target casts Vigilance
--   Proc (50725) on the caster with the effect amount — no proc hook
--   exists on the RegisterAuraEvent surface, so the proc leg stays out.
--   Glyph of Vigilance (63326) appears only in Validate(); no arm.

local SPELL_WARRIOR_VIGILANCE = 50720 -- spell_warrior.cpp:87
local SPELL_WARRIOR_VIGILANCE_REDIRECT_THREAT = 59665 -- spell_warrior.cpp:67
local SPELL_GEN_DAMAGE_REDUCTION_AURA = 68066 -- spell_warrior.cpp:87
local SPELL_PALADIN_BLESSING_OF_SANCTUARY = 20911 -- spell_warrior.cpp:85
local SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY = 25899 -- spell_warrior.cpp:86
local SPELL_PRIEST_RENEWED_HOPE = 63944 -- spell_warrior.cpp:87

-- C++ spell_warr_vigilance_AuraScript::HandleApply (spell_warrior.cpp:1108-1115).
local function vigilanceApply(event, aura, target)
    target:CastSpell(target, SPELL_GEN_DAMAGE_REDUCTION_AURA, true)
    local caster = aura:GetCaster()
    if caster ~= nil then
        target:CastSpell(caster, SPELL_WARRIOR_VIGILANCE_REDIRECT_THREAT, true)
    end
end

RegisterAuraEvent(SPELL_WARRIOR_VIGILANCE, 2, vigilanceApply)

-- C++ spell_warr_vigilance_AuraScript::HandleRemove (spell_warrior.cpp:1117-1129).
local function vigilanceRemove(event, aura, target)
    if target:HasAura(SPELL_GEN_DAMAGE_REDUCTION_AURA)
        and not target:HasAura(SPELL_PALADIN_BLESSING_OF_SANCTUARY)
        and not target:HasAura(SPELL_PALADIN_GREATER_BLESSING_OF_SANCTUARY)
        and not target:HasAura(SPELL_PRIEST_RENEWED_HOPE) then
        target:RemoveAura(SPELL_GEN_DAMAGE_REDUCTION_AURA)
    end
end

RegisterAuraEvent(SPELL_WARRIOR_VIGILANCE, 3, vigilanceRemove)

-- PORTED (this tick, RegisterSpellEvent OnObjectAreaTargetSelect = 7):
--   spell_warr_intimidating_shout::FilterTargets
--   (spell_warrior.cpp:625-628, 632-633): EFFECT_1 and EFFECT_2,
--   TARGET_UNIT_SRC_AREA_ENEMY — the explicit target drops out of
--   both area lists (its own EFFECT_0 cower leg still hits it).
--   ev7 fires on the creature-cast path only (boss_ai.go:3151); the
--   player-cast legs live in Go (spells.go area filters). The spell
--   object's GetTarget is the explicit unit target (the C++
--   GetExplTargetWorldObject). DORMANT until Spell.dbc drives the cast.
local SPELL_WARRIOR_INTIMIDATING_SHOUT = 5246 -- spell_warrior.cpp:615

local function intimidatingShoutFilter(event, spell, effectIndex, targets)
    local target = spell:GetTarget()
    if target ~= nil then
        targets:Remove(target)
    end
end

RegisterSpellEvent(SPELL_WARRIOR_INTIMIDATING_SHOUT, 7, intimidatingShoutFilter)
