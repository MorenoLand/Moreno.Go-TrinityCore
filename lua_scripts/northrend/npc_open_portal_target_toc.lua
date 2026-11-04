-- Wilfred's portal target (Trial of the Crusader) — Lua port of
-- npc_open_portal_target_toc in
-- src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/trial_of_the_crusader.cpp
-- (lines 635-665).
-- Entry: 17965 (trial_of_the_crusader.h NPC_WILFRED_PORTAL — an
-- in-tree constant, not a guess). Linkage: npc_fizzlebang_toc
-- EVENT_OBLIVION summons NPC_WILFRED_PORTAL, then casts
-- SPELL_OPEN_PORTAL (67864) on itself; this class is the file's
-- only SpellHit(67864) reactor, so the summon-site + spell-pair
-- linkage binds the script name to the entry (the
-- creature_template ScriptName row itself is DB-side).
-- Ported arms (C++-exact for all modeled arms):
-- - Reset: SetReactState(REACT_PASSIVE = 0) + scheduler CancelAll
--   + SetDisableGravity(true). The scheduler cancel is modeled
--   as cancelling the pending portal timer only: C++ CancelAll
--   does not touch a DespawnOrUnsummon timer once scheduled, so
--   a despawn timer already running survives Reset, exactly
--   like C++.
-- - SpellHit (event 14, spell 67864): 2s later DoCastSelf
--   SPELL_WILFRED_PORTAL (68424, non-triggered) + DespawnOrUnsummon
--   9s (cast at +2s, despawn at +11s from the hit).
-- - UpdateAI is a scheduler pump only; no combat timers exist.
-- Unmodeled: nothing — the class has no other arms. The summoner
-- (fizzlebang) stays out: its spline-chain MovementInform +
-- instance DoUseDoorOrButton / GetCreature / DoAction legs have
-- no bridges.

local ENTRY_WILFRED_PORTAL = 17965

local SPELL_OPEN_PORTAL = 67864
local SPELL_WILFRED_PORTAL = 68424
local REACT_PASSIVE = 0

local PORTAL_DELAY = 2000
local DESPAWN_DELAY = 9000

local timers = {}

local function cancelPortalTimer(guid)
    local per = timers[guid]
    if per and per.portal then
        RemoveEventById(per.portal)
        per.portal = nil
    end
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function portalTargetReset(event, creature)
    creature:SetReactState(REACT_PASSIVE)
    cancelPortalTimer(creature:GetGUID())
    creature:SetDisableGravity(true)
end

local function portalTargetDied(event, creature)
    cancelTimers(creature:GetGUID())
end

local function portalTargetSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_OPEN_PORTAL then
        return
    end
    local guid = creature:GetGUID()
    cancelPortalTimer(guid)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    per.portal = CreateLuaEvent(function()
        per.portal = nil
        creature:CastSpell(creature, SPELL_WILFRED_PORTAL)
        per.despawn = CreateLuaEvent(function()
            per.despawn = nil
            creature:Despawn()
        end, DESPAWN_DELAY)
    end, PORTAL_DELAY)
end

RegisterCreatureEvent(ENTRY_WILFRED_PORTAL, 5, portalTargetReset)
RegisterCreatureEvent(ENTRY_WILFRED_PORTAL, 23, portalTargetReset)
RegisterCreatureEvent(ENTRY_WILFRED_PORTAL, 4, portalTargetDied)
RegisterCreatureEvent(ENTRY_WILFRED_PORTAL, 14, portalTargetSpellHit)
