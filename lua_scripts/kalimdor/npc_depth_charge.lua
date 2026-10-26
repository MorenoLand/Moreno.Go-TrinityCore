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
-- Reset's SetHover(true) ports on onReset below (bridged); SetSwim
-- (true) ports on onReset below (bridged); the SetFlag(UNIT_FIELD_FLAGS,
-- UNIT_FLAG_NOT_SELECTABLE) leg IS bridged and ported on onReset
-- below (zone_azshara.cpp:339). The trap's passive/invisible posture
-- is a template concern, not scripted here. The rizzle
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
    if creature:GetDistance(player) - creature:GetCombatReach() - 1.5 < PROXIMITY_YARDS then -- IsWithinDistInMap twin (zone_azshara.cpp:351; player reach 1.5)
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
RegisterCreatureEvent(ENTRY, 1, function(_, creature)
    creature:SetMeleeAttack(false) -- zone_azshara.cpp:361: custom UpdateAI never calls DoMeleeAttackIfReady (despawn-timer UpdateAI in C++, never melees)
end)

-- C++ npc_depth_chargeAI::Reset (zone_azshara.cpp:336-342): SetHover(true)
-- + SetSwim(true) ported (bridged, lua_creature_events.go);
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE) ported, in
-- C++ arm order after the hover/swim legs.
local function onReset(event, creature)
    creature:SetHover(true)
    creature:SetSwim(true)
    creature:SetFlag(59, 0x2000000)
end

RegisterCreatureEvent(ENTRY, 23, onReset)

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
