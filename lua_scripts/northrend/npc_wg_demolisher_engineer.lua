-- Wintergrasp demolisher engineers — Lua port of
-- npc_wg_demolisher_engineerAI in
-- src/server/scripts/Northrend/zone_wintergrasp.cpp (class :120-200).
-- Entries (TDB-verified with ScriptName 'npc_wg_demolisher_engineer'):
-- 30400 "Goblin Mechanic" (Horde) and 30499 "Gnomish Engineer"
-- (Alliance).
local NPC_GOBLIN_MECHANIC = 30400
local NPC_GNOMISH_ENGINEER = 30499

local NPC_WINTERGRASP_CONTROL_ARMS = 27852

local SPELL_CORPORAL = 33280
local SPELL_LIEUTENANT = 55629

local SPELL_BUILD_SIEGE_VEHICLE_FORCE_HORDE = 61409
local SPELL_BUILD_SIEGE_VEHICLE_FORCE_ALLIANCE = 56662
local SPELL_BUILD_CATAPULT_FORCE = 56664
local SPELL_BUILD_DEMOLISHER_FORCE = 56659
local SPELL_ACTIVATE_CONTROL_ARMS = 49899

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000

local GOSSIP_HELLO_DEMO1 = "Build catapult."
local GOSSIP_HELLO_DEMO2 = "Build demolisher."
local GOSSIP_HELLO_DEMO3 = "Build siege engine."
local GOSSIP_HELLO_DEMO4 = "I cannot build more!"

-- C++ CanBuild (:178-199): the WG battlefield always exists in Go
-- (getOrCreateWGState mirrors the always-registered C++ battlefield),
-- so the null-battlefield arm is vacuous; the entry switch is the
-- whole gate. Vehicle counts come from the GetWGVehicleCounts bridge
-- (lua_player.go, next to GetWGDefenderTeam): current Horde, max
-- Horde, current Alliance, max Alliance.
local function canBuild(creature, player)
    local vehiclesHorde, maxHorde, vehiclesAlliance, maxAlliance =
        player:GetWGVehicleCounts()
    local entry = creature:GetEntry()
    if entry == NPC_GOBLIN_MECHANIC then
        return maxHorde > vehiclesHorde
    elseif entry == NPC_GNOMISH_ENGINEER then
        return maxAlliance > vehiclesAlliance
    end
    return false
end

-- C++ OnGossipHello (:129-150): quest menu first (GossipAddQuests
-- stages nothing for a non-questgiver, matching the C++ IsQuestGiver
-- gate), then rank-gated build options when CanBuild, else the
-- cannot-build line. The registered hello hook vetoes the default
-- menu (gossip.go), matching the C++ return true.
local function engineerGossipHello(event, player, creature)
    player:GossipAddQuests(creature)
    if canBuild(creature, player) then
        if player:HasAura(SPELL_CORPORAL) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HELLO_DEMO1,
                GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF)
        elseif player:HasAura(SPELL_LIEUTENANT) then
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HELLO_DEMO1,
                GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF)
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HELLO_DEMO2,
                GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
            player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HELLO_DEMO3,
                GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 2)
        end
    else
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_HELLO_DEMO4,
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 9)
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

-- C++ OnGossipSelect (:152-176): the force spell per option
-- (triggered creature-on-player cast; the creature CastSpell bridge
-- honors the flag), then the control-arms
-- activation on the nearest control arms within 30y (triggered).
-- The menu is already cleared server-side before the hook fires
-- (gossip.go), matching CloseGossipMenuFor. CanBuild is re-checked
-- per C++. The action arrives directly as arg 5.
local function engineerGossipSelect(event, player, creature, sender, action, code, listID)
    if not canBuild(creature, player) then
        return
    end
    local option = action - GOSSIP_ACTION_INFO_DEF
    if option == 0 then
        creature:CastSpell(player, SPELL_BUILD_CATAPULT_FORCE, true)
    elseif option == 1 then
        creature:CastSpell(player, SPELL_BUILD_DEMOLISHER_FORCE, true)
    elseif option == 2 then
        if player:GetTeam() == 0 then
            creature:CastSpell(player, SPELL_BUILD_SIEGE_VEHICLE_FORCE_ALLIANCE, true)
        else
            creature:CastSpell(player, SPELL_BUILD_SIEGE_VEHICLE_FORCE_HORDE, true)
        end
    else
        return
    end
    local controlArms = creature:FindNearestCreature(NPC_WINTERGRASP_CONTROL_ARMS, 30, true)
    if controlArms then
        creature:CastSpell(controlArms, SPELL_ACTIVATE_CONTROL_ARMS, true)
    end
end

for _, entry in ipairs({NPC_GOBLIN_MECHANIC, NPC_GNOMISH_ENGINEER}) do
    RegisterCreatureGossipEvent(entry, 1, engineerGossipHello)
    RegisterCreatureGossipEvent(entry, 2, engineerGossipSelect)
end

-- Note: 49899 carries a second spell_script_names row
-- (spell_wintergrasp_activate_robotic_arms, loaded from another C++
-- file, not this unit); the cast above fires whichever handler the
-- Go loader registered for it.
