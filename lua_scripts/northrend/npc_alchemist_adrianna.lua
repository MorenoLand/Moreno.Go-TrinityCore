-- Icecrown Citadel: npc_alchemist_adrianna — Lua port of
-- src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.cpp
-- (npc_alchemist_adrianna : public ScriptedAI).
-- Entry: NPC_ALCHEMIST_ADRIANNA 38501 (icecrown_citadel.h:160).
-- Ported: OnGossipHello (icecrown_citadel.cpp:551-558) -> event 1:
-- when the player has both SPELL_ORANGE_BLIGHT_RESIDUE 72144 and
-- SPELL_GREEN_BLIGHT_RESIDUE 72145 (player:HasAura is bridged),
-- DoCastSelf(SPELL_HARVEST_BLIGHT_SPECIMEN 72155, false) — lowered
-- to creature:CastSpell(creature, 72155), non-triggered. C++ returns
-- false, so no menu is sent — the Lua port sends none either (the
-- DB default menu shows).
-- Documented-only: the FindCurrentSpellBySpellId recast gate
-- (icecrown_citadel.cpp:553) — no spell-state lookup bridge.
-- Also in this C++ file but unrelated to Adrianna: spell_icc_stoneform
-- (icecrown_citadel.cpp:1306-1335) — AuraScript whose OnApply/OnRemove
-- set REACT_PASSIVE/REACT_AGGRESSIVE + NOT_SELECTABLE/ImmuneToPC/emote
-- legs on the aura target; no AuraScript bridge — stays out.

local ENTRY_ALCHEMIST_ADRIANNA = 38501

local SPELL_ORANGE_BLIGHT_RESIDUE = 72144
local SPELL_GREEN_BLIGHT_RESIDUE = 72145
local SPELL_HARVEST_BLIGHT_SPECIMEN = 72155

local function adriannaGossipHello(event, player, creature)
    if player:HasAura(SPELL_ORANGE_BLIGHT_RESIDUE)
        and player:HasAura(SPELL_GREEN_BLIGHT_RESIDUE) then
        creature:CastSpell(creature, SPELL_HARVEST_BLIGHT_SPECIMEN)
    end
    -- C++ npc_alchemist_adrianna::OnGossipHello returns false (no menu
    -- sent; the default menu proceeds). Eluna GOSSIP_EVENT_ON_HELLO:
    -- a Lua false lets the default menu proceed.
    return false
end

RegisterCreatureGossipEvent(ENTRY_ALCHEMIST_ADRIANNA, 1, adriannaGossipHello)
