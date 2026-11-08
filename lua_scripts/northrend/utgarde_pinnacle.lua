-- Utgarde Pinnacle zone file — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/utgarde_pinnacle.cpp
-- (utgarde_pinnacle.h). The file holds no NPC/GO AI at all — its only
-- registration is the Beast's Mark AuraScript below.
--
-- PORTED (this run, RegisterAuraEvent OnProc = 6):
-- * spell_utgarde_pinnacle_beast_mark (48876 heroic / 59237 normal,
--   utgarde_pinnacle.cpp:40-82): AuraScript on EFFECT_0
--   SPELL_AURA_PROC_TRIGGER_SPELL. CheckProc (:53-66) fires only when
--   the proc carries damage info, the attacker exists, the damage is
--   non-zero, and the attacker is CREATURE_TYPE_BEAST; HandleProc
--   (:68-75) PreventDefaultActions and has the target cast the damage
--   spell on itself — 48877 when the aura is 59237 (normal), 48876
--   when the aura is 48876 (heroic) (:33-37, :72).
--   The "no aura-proc bridge" stay-out is RETIRED: the proc hook is
--   live (engine/scripting/runtime.go:502 AuraEventOnProc = 6;
--   engine/world/lua_aura.go:822 fireAuraProc, args (event, aura,
--   target, actor, procTarget, damage); session + creature proc loops
--   both fire it after the DBC proc gates and the charge consume,
--   engine/world/procs.go:3179 and engine/world/boss_ai.go:10510).
--   The handler replicates CheckProc from the hook args: damage == 0
--   covers the no-damage-info case (the zero-damage proc tail carries
--   damage 0, engine/world/pet_combat.go:7796), actor nil covers the
--   no-attacker case, actor:GetCreatureType() is the bridged
--   Unit::GetCreatureType (engine/world/lua_creature_events.go:4612,
--   lua_objects.go:330, lua_player.go:302 — a shapeshifted druid
--   reads the form's type, matching C++). A gate failure returns
--   false so the default trigger-spell leg runs, C++-exact for a
--   failed CheckProc; success returns true (PreventDefaultAction).
--   The aura id selects the damage spell via aura:GetAuraId()
--   (lua_aura.go:200), the m_scriptSpellId twin. The C++ cast
--   passes aurEff for base points; Go CastSpell takes triggered
--   only, so the cast is triggered with DBC base points (the
--   woe-strike precedent, boss_ingvar_the_plunderer.lua).
--   DORMANT until Spell.dbc ships the 59237 / 48876 aura records
--   and their spell_proc entries. Registration is DB-side only
--   (spell_script_names rows for 48876 / 59237); no creature
--   registration is made here (the karazhan.lua documented-only
--   precedent for the file shape, now with live aura hooks).

local SPELL_BEAST_MARK_NORMAL = 59237 -- utgarde_pinnacle.cpp:31
local SPELL_BEAST_MARK_HEROIC = 48876 -- utgarde_pinnacle.cpp:30
local SPELL_BEAST_MARK_DAMAGE_N = 48877 -- utgarde_pinnacle.cpp:32
local SPELL_BEAST_MARK_DAMAGE_H = 48876 -- utgarde_pinnacle.cpp:33
local CREATURE_TYPE_BEAST = 1 -- SharedDefines.h:2661

-- C++ spell_utgarde_pinnacle_beast_mark_AuraScript::CheckProc +
-- HandleProc (utgarde_pinnacle.cpp:53-75).
local function beastMarkProc(event, aura, target, actor, procTarget, damage)
    if target == nil or damage == nil or damage == 0 then
        return false
    end
    if actor == nil or actor:GetCreatureType() ~= CREATURE_TYPE_BEAST then
        return false
    end
    local spellId = SPELL_BEAST_MARK_DAMAGE_H
    if aura:GetAuraId() == SPELL_BEAST_MARK_NORMAL then
        spellId = SPELL_BEAST_MARK_DAMAGE_N
    end
    target:CastSpell(target, spellId, true)
    return true
end

RegisterAuraEvent(SPELL_BEAST_MARK_NORMAL, 6, beastMarkProc)
RegisterAuraEvent(SPELL_BEAST_MARK_HEROIC, 6, beastMarkProc)
