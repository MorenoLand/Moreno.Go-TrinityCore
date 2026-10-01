-- npc_kalecgos (Magister's Terrace zone script) --
-- Lua port of the npc_kalecgos class in
-- src/server/scripts/EasternKingdoms/MagistersTerrace/
-- magisters_terrace.cpp (the zone file's single creature script,
-- next after the 8/8-closed lackey block in the CHECKPOINT plan;
-- instance_magisters_terrace.cpp stays blocked on the instance-
-- script model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h:64
-- names NPC_KALECGOS = 24844 in the MT creatures enum; the AI is
-- retrieved via GetMagistersTerraceAI (an instance-AI retrieval
-- wrapper only — no instance state is read by the ported arms).
-- Whole-server-tree grep confirms magisters_terrace.cpp (plus its
-- loader reference) as the only sources of "npc_kalecgos". The
-- creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_felblood_kaelthas precedent). Script name is the
-- stringified class name (CreatureScript("npc_kalecgos");
-- RegisterCreatureAIWithFactory convention, felblood precedent).
-- Verifiable numbers (file's own enums): SPELL_KALECGOS_TRANSFORM
-- = 44670 / SPELL_TRANSFORM_VISUAL = 24085 / SPELL_CAMERA_SHAKE
-- = 44762 / SPELL_ORB_KILL_CREDIT = 46307 (Spells enum);
-- POINT_ID_PREPARE_LANDING = 6; EVENT_KALECGOS_TRANSFORM = 1 /
-- EVENT_KALECGOS_LANDING = 2; NPC_HUMAN_KALECGOS = 24848
-- (magisters_terrace.h:65). Gossip texts are the file's own
-- #defines (GOSSIP_ITEM_KAEL_1..5); gossip menus 12498 / 12500 /
-- 12502 / 12606 / 12607 / 12608 (OnGossipHello / OnGossipSelect
-- bodies). GOSSIP_ACTION_INFO_DEF = 1000, GOSSIP_SENDER_MAIN = 1,
-- GOSSIP_ICON_CHAT = 0 (nefarian convention).
-- Eluna gossip events: 1 OnGossipHello, 2 OnGossipSelect
-- (nefarian precedent — handler args (event, player, creature)
-- and (event, player, creature, sender, action)).
-- Ported arms (C++ OnGossipHello / OnGossipSelect, nefarian
-- convention):
-- OnGossipHello: GossipMenuAddItem(0, "Who are you?", 1, 1000);
--   GossipSendMenu(0, creature, 12498) (C++-exact item text, menu
--   id, sender, action).
-- OnGossipSelect: GossipClearMenu() (C++-exact
--   ClearGossipMenuFor), then per action:
--   1000 -> add "What can we do to assist you?" (1, 1001), send
--     menu 12500;
--   1001 -> add "What brings you to the Sunwell?" (1, 1002), send
--     menu 12502;
--   1002 -> add "You're not alone here?" (1, 1003), send menu
--     12606;
--   1003 -> add "What would Kil'jaeden want with a mortal woman?"
--     (1, 1004), send menu 12607;
--   1004 -> send menu 12608 (no item, C++-exact).
-- No combat events: the AI has no combat arms (ScriptedAI base
-- only; no JustEngagedWith/UpdateAI combat machines).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - OnGossipHello's PrepareQuestMenu leg (fires only when
--   me->IsQuestGiver() — no quest-menu / quest-giver bridge).
-- - MovementInform(POINT_ID_PREPARE_LANDING): the EMOTE_ONESHOT_
--   LAND + SetDisableGravity(false) + SetHover(false) leg + the
--   2s re-arm of EVENT_KALECGOS_LANDING — no MovementInform /
--   MotionMaster bridge (selin_fireheart precedent).
-- - EVENT_KALECGOS_LANDING arm: DoCastAOE(SPELL_CAMERA_SHAKE
--   44762) + SetObjectScale(0.6f) + 1s re-arm of
--   EVENT_KALECGOS_TRANSFORM — triggered only from MovementInform,
--   so it is stranded with the movement leg; no object-scale
--   bridge either.
-- - EVENT_KALECGOS_TRANSFORM's cast triplet: DoCast(me,
--   SPELL_ORB_KILL_CREDIT 46307, triggered) / DoCast(me,
--   SPELL_TRANSFORM_VISUAL 24085) / DoCast(me,
--   SPELL_KALECGOS_TRANSFORM 44670) — the casts themselves are
--   individually bridgeable (CastSpell(creature, id, true) is the
--   leotheras triggered-cast convention), but the event is only
--   scheduled from EVENT_KALECGOS_LANDING, which is stranded on
--   the MovementInform bridge, so the whole transform chain is
--   documented-only; me->UpdateEntry(NPC_HUMAN_KALECGOS 24848)
--   has no entry-update bridge regardless.
local ENTRY_KALECGOS = 24844

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000

local GOSSIP_ITEM_KAEL_1 = "Who are you?"
local GOSSIP_ITEM_KAEL_2 = "What can we do to assist you?"
local GOSSIP_ITEM_KAEL_3 = "What brings you to the Sunwell?"
local GOSSIP_ITEM_KAEL_4 = "You're not alone here?"
local GOSSIP_ITEM_KAEL_5 = "What would Kil'jaeden want with a mortal woman?"

-- C++ OnGossipHello: PrepareQuestMenu only when IsQuestGiver (no
-- quest-menu bridge — documented only); always offers "Who are
-- you?" and sends menu 12498.
local function onKalecgosGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_KAEL_1,
        GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF)
    player:GossipSendMenu(0, creature, 12498)
end

-- C++ OnGossipSelect: ClearGossipMenuFor, then the five-item
-- action chain GOSSIP_ACTION_INFO_DEF..+4, each step offering the
-- next question and sending the next menu; the last step sends
-- menu 12608 with no item.
local function onKalecgosGossipSelect(event, player, creature, sender, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_KAEL_2,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
        player:GossipSendMenu(0, creature, 12500)
    elseif action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_KAEL_3,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        player:GossipSendMenu(0, creature, 12502)
    elseif action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_KAEL_4,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 3)
        player:GossipSendMenu(0, creature, 12606)
    elseif action == GOSSIP_ACTION_INFO_DEF + 3 then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_KAEL_5,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 4)
        player:GossipSendMenu(0, creature, 12607)
    elseif action == GOSSIP_ACTION_INFO_DEF + 4 then
        player:GossipSendMenu(0, creature, 12608)
    end
end

RegisterCreatureGossipEvent(ENTRY_KALECGOS, 1, onKalecgosGossipHello)
RegisterCreatureGossipEvent(ENTRY_KALECGOS, 2, onKalecgosGossipSelect)
