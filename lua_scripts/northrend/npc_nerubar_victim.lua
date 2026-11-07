-- Nerub'ar Victim (Borean Tundra) — Lua port of
-- src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (class npc_nerubar_victim : CreatureScript("npc_nerubar_victim");
-- struct npc_nerubar_victimAI : public ScriptedAI).
-- Entry: 25284 Wowhead-verified (wowhead.com/wotlk/npc=25284/
-- nerubar-victim; no NPC_ constant in the C++ tree — the dark-rider /
-- belnistrasz precedent). Level 70, Borean Tundra, matches the
-- cocooned Warsong peon for quest 11611 "Taken by the Scourge".
-- Eluna creature events: 4 OnDied. No timers; the C++ Reset /
-- JustEngagedWith / MoveInLineOfSight overrides are empty.
-- Ported arm:
-- - JustDied (zone_borean_tundra.cpp:139-158, event 4): killer must be
--   a player (killer:GetTypeId() == 4, dark-rider precedent) with quest
--   11611 QUEST_TAKEN_BY_THE_SCOURGE incomplete (player:GetQuestStatus
--   == 3, phase-hunter precedent); one urand(0, 99) draw: < 25 ->
--   player:CastSpell(me, SPELL_FREED_WARSONG_PEON 45532, triggered)
--   + player:KilledMonsterCredit(NPC_WARSONG_PEON 25270); < 75 ->
--   player:CastSpell(me, nerubarVictims[urand(0, 2)], triggered);
--   else nothing. C++ order and single-draw branching exact.
-- Engine note: player:CastSpell rides the triggered pipeline
-- (TRIGGERED_FULL_MASK, lua_player.go), but SPELL_EFFECT_SUMMON has
-- no session-path executor yet — the summon-effect casts fire the
-- SMSG_SPELL_GO visual but spawn nothing (bridge comment). The quest
-- progress leg (KilledMonsterCredit) fires regardless, so the arm is
-- ported as written; the freed-peon summon materializes when the
-- executor lands.

local ENTRY_NERUBAR_VICTIM = 25284

local QUEST_TAKEN_BY_THE_SCOURGE = 11611
local QUEST_STATUS_INCOMPLETE = 3

local NPC_WARSONG_PEON = 25270

local SPELL_FREED_WARSONG_PEON = 45532

-- C++ nerubarVictims (zone_borean_tundra.cpp:120-123), C++ order.
local NERUBAR_VICTIMS = {
    45526, -- SPELL_FREED_WARSONG_MAGE
    45527, -- SPELL_FREED_WARSONG_SHAMAN
    45514, -- SPELL_FREED_WARSONG_WARRIOR
}

local TYPEID_PLAYER = 4

-- C++ npc_nerubar_victimAI::JustDied (zone_borean_tundra.cpp:139-158).
local function onDied(event, creature, killer)
    if not killer or killer:GetTypeId() ~= TYPEID_PLAYER then
        return
    end
    local player = killer
    if player:GetQuestStatus(QUEST_TAKEN_BY_THE_SCOURGE) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    local roll = math.random(0, 99)
    if roll < 25 then
        player:CastSpell(creature, SPELL_FREED_WARSONG_PEON, true)
        player:KilledMonsterCredit(NPC_WARSONG_PEON)
    elseif roll < 75 then
        player:CastSpell(creature, NERUBAR_VICTIMS[math.random(1, 3)], true)
    end
end

RegisterCreatureEvent(ENTRY_NERUBAR_VICTIM, 4, onDied)
