-- npc_salanar_the_horseman (The Scarlet Enclave, Realm of Shadows,
-- Quest 12687 "Into the Realm of Shadows") -- Lua port of the
-- npc_salanar_the_horseman class in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter1.cpp
-- (class npc_salanar_the_horseman line 785; AI struct line 790;
-- AddSC_the_scarlet_enclave_c1 registers it with the other chapter1
-- scripts; EK loader decl :95 / call :273).
-- Entry: 28788 ONLY (TDB 26091 creature_template, verified against
-- the dump's own CREATE TABLE column order: 28788 is the sole
-- Salanar the Horseman row carrying ScriptName
-- 'npc_salanar_the_horseman', lvl 55, faction 2084, npcflag 0,
-- gossip_menu_id 0). The other two same-named rows run no C++
-- script: 28653 (lvl 80, menu 9739, npcflag 3, SmartAI) shows menu
-- 9739 in the normal world, 28908 (menu 9751, npcflag 1) has empty
-- ScriptName.
-- Gossip menu 9739 option 0: "Salanar, I must return to the Realm
-- of Shadows." (gossip_menu_option OptionText, broadcast 28863),
-- sender MAIN, action GOSSIP_ACTION_INFO_DEF (1000).
-- Verifiable numbers (chapter1.cpp:771-783 SalanarTheHorseman enum):
-- GOSSIP_SALANAR_MENU = 9739 / GOSSIP_SALANAR_OPTION = 0 /
-- SALANAR_SAY = 0 / QUEST_INTO_REALM_OF_SHADOWS = 12687 /
-- NPC_SALANAR_IN_REALM_OF_SHADOWS = 28788 /
-- SPELL_REALM_OF_SHADOWS = 52693.
-- Eluna creature events: gossip 2 OnGossipSelect (zulaman /
-- hummel precedents). The hook passes (event, player, creature,
-- sender, action) with no menuId, so the C++ menuId == 9739 leg
-- is unexpressible -- equivalent in practice, since the handler is
-- registered only for 28788, whose only menu would be 9739.
-- Ported arms (chapter1.cpp:785-837):
-- - OnGossipSelect (chapter1.cpp:796-801): menu 9739 + option 0 ->
--   player:CastSpell(player, SPELL_REALM_OF_SHADOWS 52693) +
--   SendCloseGossip, in that C++ order. NOTE this arm has NO
--   ClearGossipMenuFor (unlike the initiate's OnGossipSelect at
--   :648-669, which clears unconditionally first) -- do not copy
--   the initiate's order here. The player CastSpell bridge drops
--   the triggered flag (lua_player.go CastSpell has no triggered
--   arg, like CastCustomSpell's accepted-but-dropped flag).
--   Spell 52693 is an INFERRED seed row (tools/spelldbc_seed/
--   main.go, DUMMY self-cast like 7267 Grovel); replace with a
--   real DBC row when client data ships.
-- - MoveInLineOfSight (chapter1.cpp:805-831): PORTED via the ev27
--   proximity dispatch (stepCreatureMoveInLOSProximity,
--   creaturemotion.go: 50yd MonsterSight, CanSeeOrDetect gates,
--   veto discarded) -- a friendly vehicle never reaches the
--   aggro-engage branch, so the proximity pass is the trigger.
--   Bridges live since 2026-11-14: creature:GetCharmer,
--   creature/player RemoveAurasDueToSpell, player
--   GroupEventHappens (lua_player.go:5306). Talk group 0 has no
--   creature_text row for 28788 in TDB 26091, so the Talk leg is
--   silent until DB text ships.
-- REACHABILITY: 28788 ships npcflag 0 + gossip_menu_id 0, so no
-- gossip window opens on it in the current DB state and the select
-- arm cannot fire -- the same holds in live TC (HandleGossipHello
-- requires UNIT_NPC_FLAG_GOSSIP). The handler is registered
-- faithfully anyway: if a menu is ever presented (DB correction),
-- it does exactly the C++ thing. The live quest flow reaches menu
-- 9739 through 28653 (SmartAI), not through this script.

local ENTRY_SALANAR = 28788
local SPELL_REALM_OF_SHADOWS = 52693
local SPELL_EFFECT_OVERTAKE = 52349
local QUEST_INTO_REALM_OF_SHADOWS = 12687
local QUEST_STATUS_INCOMPLETE = 3
local SALANAR_SAY = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000

-- C++ OnGossipSelect (chapter1.cpp:796-801).
local function onGossipSelect(_, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN or action ~= GOSSIP_ACTION_INFO_DEF then
        return
    end
    player:CastSpell(player, SPELL_REALM_OF_SHADOWS)
    player:GossipComplete()
end

RegisterCreatureGossipEvent(ENTRY_SALANAR, 2, onGossipSelect)

-- C++ MoveInLineOfSight (chapter1.cpp:805-831), in C++ order:
-- ScriptedAI::MoveInLineOfSight first (no veto here — the Lua
-- handler returns nothing, so the engage branch proceeds exactly
-- as C++), then the vehicle machine. The player
-- RemoveAurasDueToSpell(52693) leg sits OUTSIDE the quest/entry
-- gate but inside the charmer-is-player gate, as in C++.
local function onMoveInLOS(_, creature, who)
    if who:GetObjectType() ~= "Creature" then
        return
    end
    if not who:IsVehicle() then
        return
    end
    if not creature:IsWithinDistInMap(who, 5.0) then
        return
    end
    local charmer = who:GetCharmer()
    if not charmer then
        return
    end
    if charmer:GetObjectType() ~= "Player" then
        return
    end
    if creature:GetEntry() == ENTRY_SALANAR
            and charmer:GetQuestStatus(QUEST_INTO_REALM_OF_SHADOWS) == QUEST_STATUS_INCOMPLETE then
        charmer:GroupEventHappens(QUEST_INTO_REALM_OF_SHADOWS, creature)
        creature:Talk(SALANAR_SAY)
        charmer:RemoveAurasDueToSpell(SPELL_EFFECT_OVERTAKE)
        who:DespawnOrUnsummon()
    end
    charmer:RemoveAurasDueToSpell(SPELL_REALM_OF_SHADOWS)
end

RegisterCreatureEvent(ENTRY_SALANAR, 27, onMoveInLOS)
