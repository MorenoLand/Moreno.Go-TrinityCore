-- Boss_Announcer (PlayerScript world-boss kill announce) --
-- Lua port of src/server/scripts/Custom/cs_worldboss_kill_announce.cpp
-- (AddSC_Boss_Announcer).
--
-- The engine fires the C++ OnCreatureKill arm as Eluna player event 7
-- (event, player, creature) from the real kill paths
-- (engine/world/kill.go fireCreatureKillHook: combat, spells, rage,
-- damage-shield, plus the .kill command). creature:IsWorldBoss is
-- Creature::isWorldBoss (type_flags & CREATURE_TYPEFLAGS_BOSS, plus the
-- instance-boss fallback in lua_creature_events.go); the name link is
-- ChatHandler::playerLink (Chat.h:119) verbatim, and the broadcast rides
-- SendWorldMessage, wired to broadcastServerMessageAll (server.go:966).
-- The message string reproduces the C++ concatenation exactly, including
-- the unclosed trailing |CFF64FF64 before the period.

local function onKillCreature(event, player, creature)
    if creature:IsWorldBoss() then
        local name = player:GetName()
        local link = "|cffffffff|Hplayer:" .. name .. "|h[" .. name .. "]|h|r"
        SendWorldMessage("|CFF64FF64World Boss |CFFFFFFFF[" .. creature:GetName() .. "]|r|CFF64FF64 has been slain by|r " .. link .. "|CFF64FF64.")
    end
end

RegisterPlayerEvent(7, onKillCreature)

-- Boss_Announcer::OnUpdateZone (cs_worldboss_kill_announce.cpp:32-46):
-- entering area 420 sets run speed 2.0 (Unit::SetSpeed, MOVE_RUN).
-- Only the SendServerMessage lines are commented out in C++; the
-- SetSpeed arm is live. The zone hook fires as Eluna player event 27
-- from updateZoneAndArea (terrain.go) whenever the zone or area changes,
-- and SetSpeed is the player UnitMethods::SetSpeed bridge
-- (lua_player.go) — absolute speed, rate 2.0 / base run 7.0.
local function onUpdateZone(event, player, newZone, newArea)
    if newArea == 420 then
        player:SetSpeed(1, 2.0)
    end
end

RegisterPlayerEvent(27, onUpdateZone)
