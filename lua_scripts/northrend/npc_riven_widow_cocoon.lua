-- Riven Widow Cocoon (Howling Fjord) — Lua port of
-- src/server/scripts/Northrend/zone_howling_fjord.cpp
-- (class npc_riven_widow_cocoon : CreatureScript("npc_riven_widow_cocoon");
-- struct npc_riven_widow_cocoonAI : public ScriptedAI).
-- Entry: 24210 Wowhead-verified (wowhead.com/wotlk/npc=24210/
-- riven-widow-cocoon; no NPC_ constant in the C++ tree — the dark-rider /
-- belnistrasz precedent). Level 67, Howling Fjord, matches the cocoon.
-- Eluna creature events: 4 OnDied. No timers; the C++ Reset /
-- JustEngagedWith / MoveInLineOfSight overrides are empty. No quest
-- gate in C++ — the arm fires for any player killer.
-- Ported arm:
-- - JustDied (zone_howling_fjord.cpp:577-585, event 4): killer must be
--   a player (killer:GetTypeId() == 4, dark-rider precedent);
--   roll_chance_i(20) -> player:CastSpell(me,
--   SPELL_SUMMON_FREED_MIST_WHISPER_SCOUT 43289, triggered) +
--   player:KilledMonsterCredit(NPC_MIST_WHISPER_SCOUT 24211); else
--   player:CastSpell(me, rivenWidowCocoonVictims[urand(0, 10)],
--   triggered). C++ order exact.
-- Engine note: player:CastSpell rides the triggered pipeline
-- (TRIGGERED_FULL_MASK, lua_player.go), but SPELL_EFFECT_SUMMON has
-- no session-path executor yet — the summon-effect casts fire the
-- SMSG_SPELL_GO visual but spawn nothing (bridge comment). The quest
-- progress leg (KilledMonsterCredit) fires regardless, so the arm is
-- ported as written; the freed-scout summon materializes when the
-- executor lands.

local ENTRY_RIVEN_WIDOW_COCOON = 24210

local NPC_MIST_WHISPER_SCOUT = 24211

local SPELL_SUMMON_FREED_MIST_WHISPER_SCOUT = 43289

-- C++ rivenWidowCocoonVictims (zone_howling_fjord.cpp:534-546), C++ order.
local COCOON_VICTIMS = {
    43275, -- SPELL_SUMMON_BABY_RIVEN_WIDOWS
    43276, -- SPELL_SUMMON_DARKCLAW_BAT
    43277, -- SPELL_SUMMON_FANGGORE_WORG
    43278, -- SPELL_SUMMON_GJALERBRON_RUNECASTER
    43279, -- SPELL_SUMMON_GJALERBRON_SLEEPWATCHER
    43280, -- SPELL_SUMMON_GJALERBRON_WARRIOR
    43281, -- SPELL_SUMMON_PUTRID_HORROR
    43282, -- SPELL_SUMMON_WINTERSKORN_BERSERKER
    43283, -- SPELL_SUMMON_WINTERSKORN_WOODSMAN
    43284, -- SPELL_SUMMON_WINTERSKORN_TRIBESMAN
    43285, -- SPELL_SUMMON_WINTERSKORN_ORACLE
}

local TYPEID_PLAYER = 4

-- C++ npc_riven_widow_cocoonAI::JustDied (zone_howling_fjord.cpp:577-585).
local function onDied(event, creature, killer)
    if not killer or killer:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local player = killer
    if math.random(1, 100) <= 20 then
        player:CastSpell(creature, SPELL_SUMMON_FREED_MIST_WHISPER_SCOUT, true)
        player:KilledMonsterCredit(NPC_MIST_WHISPER_SCOUT)
    else
        player:CastSpell(creature, COCOON_VICTIMS[math.random(1, 11)], true)
    end
end

RegisterCreatureEvent(ENTRY_RIVEN_WIDOW_COCOON, 4, onDied)
