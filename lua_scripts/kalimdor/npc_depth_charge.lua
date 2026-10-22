-- Azshara: Depth Charge — Lua port of
-- src/server/scripts/Kalimdor/zone_azshara.cpp
-- (npc_depth_chargeAI : public ScriptedAI; registered in AddSC_azshara()).
-- Entry: 23025 (NPC_DEPTH_CHARGE, in-enum).
-- Ported: MoveInLineOfSight -> on a player within 5 yards
-- (IsWithinDistInMap), DoCast SPELL_DEPTH_CHARGE_TRAP 38576 on that
-- player, then despawn after the 1s WeMustDieTimer. The event-27 veto
-- (return true) suppresses the default aggro engage, matching the
-- C++ empty AttackStart/JustEngagedWith overrides.
-- Zero Talk() in C++.
-- Documented-only: Reset's SetHover/SetSwim/SetFlag NOT_SELECTABLE —
-- no Lua bridge for those unit flags; the trap's passive/invisible
-- posture is a template concern, not scripted here. The rizzle
-- escort/blackjack machine (MoveInLineOfSight quest-status gate,
-- SPELL_RIZZLE_BLACKJACK 39865, SPELL_RIZZLE_ESCAPE teleport timer,
-- zone_azshara.cpp:150-163) stays documented-only: no quest-status /
-- MotionMaster bridges. Rizzle's OnGossipHello/OnGossipSelect are
-- PORTED below — entry NPC_RIZZLE 23002 is C++-verifiable
-- (go_scripts.cpp:268, Southfury enum).
local ENTRY = 23025
local SPELL_DEPTH_CHARGE_TRAP = 38576
local PROXIMITY_YARDS = 5
local DESPAWN_DELAY_MS = 1000

local despawnPending = {}

local function onMoveInLOS(event, creature, player)
    if creature:IsWithinDistInMap(player, PROXIMITY_YARDS) then
        -- C++ casts on every MoveInLineOfSight with a player in range;
        -- the WeMustDie latch gates only the despawn. Arm the despawn
        -- once per creature so repeated proximity fires within the 1s
        -- window don't stack redundant untracked Despawn timers.
        creature:CastSpell(player, SPELL_DEPTH_CHARGE_TRAP)
        local guid = creature:GetGUID()
        if not despawnPending[guid] then
            despawnPending[guid] = true
            CreateLuaEvent(function()
                despawnPending[guid] = nil
                creature:Despawn()
            end, DESPAWN_DELAY_MS)
        end
    end
    return true
end

RegisterCreatureEvent(ENTRY, 27, onMoveInLOS)

-- npc_rizzle_sprysprocket gossip (zone_azshara.cpp) — entry NPC_RIZZLE
-- 23002 (go_scripts.cpp:268).
local ENTRY_RIZZLE = 23002
local SPELL_GIVE_SOUTHFURY_MOONSTONE = 39886

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_GET_MOONSTONE = "Hand over the Southfury moonstone and I'll let you go."
local GOSSIP_MENU_MOONSTONE = 10811
local RIZZLE_DESPAWN_DELAY_MS = 3000

-- C++ OnGossipHello (zone_azshara.cpp:166-173): offers the moonstone
-- item + menu 10811 only when QUEST_CHASING_THE_MOONSTONE 10994 is
-- INCOMPLETE (else returns true with no items). The quest-status
-- gate has no bridge — kalecgos precedent — so the item is offered
-- unconditionally and the gate is documented.
local function rizzleGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_GET_MOONSTONE,
        GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(GOSSIP_MENU_MOONSTONE, creature)
end

-- C++ OnGossipSelect (zone_azshara.cpp:164-171): CloseGossipMenuFor +
-- me->CastSpell(player, SPELL_GIVE_SOUTHFURY_MOONSTONE 39886, true) +
-- MustDieTimer 3000ms -> DespawnOrUnsummon (UpdateAI :187-194). The 3s
-- timer rides this file's pending-despawn idiom (same as the
-- depth-charge trap above).
local function rizzleGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        creature:CastSpell(player, SPELL_GIVE_SOUTHFURY_MOONSTONE, true)
        local guid = creature:GetGUID()
        if not despawnPending[guid] then
            despawnPending[guid] = true
            CreateLuaEvent(function()
                despawnPending[guid] = nil
                creature:Despawn()
            end, RIZZLE_DESPAWN_DELAY_MS)
        end
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

RegisterCreatureGossipEvent(ENTRY_RIZZLE, 1, rizzleGossipHello)
RegisterCreatureGossipEvent(ENTRY_RIZZLE, 2, rizzleGossipSelect)
