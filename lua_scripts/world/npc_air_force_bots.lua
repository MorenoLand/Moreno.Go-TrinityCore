-- npc_air_force_bots, ALARMBOT half (C++ src/server/scripts/World/npcs_special.cpp:43-188).
-- TRIPWIRE entries stay out: the :174 flying skip has no player:IsFlying
-- bridge (no flying-state model on the player surface).
-- C++ shape: MoveInLineOfSight queues hostile players in range; UpdateAI
-- summons the guard when the queue is non-empty and engages each queued
-- target, the ALARMBOT guard casting SPELL_GUARDS_MARK on engage. Go has no
-- sighting hook for a NullCreatureAI bot (ev27 fires from the aggro-engage
-- branch only), so the event-7 tick replays both halves as one scan:
-- players in grid range passing the MLOS gates are engaged at once.
-- Deviations: range is judged at tick time, not at sighting time (a player
-- crossing the radius inside one 100ms tick is missed); the guard is found
-- by nearest-of-entry within 150y, not by the C++ _myGuard GUID (two
-- same-entry guards near one bot resolve to the nearest); the visibility,
-- immunity and taxi-flight arms of IsValidAttackTarget have no Go model
-- and are skipped; a dead guard is re-summoned at once (C++ waits for the
-- corpse despawn, which for TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT is
-- prompt).

local SPELL_GUARDS_MARK = 38067
local RANGE_ALARMBOT = 100

-- myEntry -> guard entry, C++ airforceSpawns table (:72-98), ALARMBOT rows only.
local ALARMBOT_GUARDS = {
    [2614] = 15241, [2615] = 15242, [21974] = 21976, [21993] = 15242,
    [21996] = 15241, [21997] = 21976, [22065] = 22064, [22066] = 22067,
    [22069] = 22064, [22071] = 22067, [22078] = 22077, [22079] = 22077,
    [22086] = 22085, [22087] = 22085, [22090] = 22089, [22124] = 22122,
    [22125] = 22122, [22126] = 22122,
}

local function airForceBotsTick(event, bot)
    local guardEntry = ALARMBOT_GUARDS[bot:GetEntry()]
    if guardEntry == nil then
        return
    end
    local players = bot:GetPlayersInGrid(RANGE_ALARMBOT)
    if players == nil then
        return
    end
    local guard = nil
    for _, player in ipairs(players) do
        -- C++ MoveInLineOfSight gates, in order: TYPEID_PLAYER (the grid
        -- returns players only), range (the grid radius), hostile either
        -- way, valid attack target (hostility verdict; the grid is
        -- alive-only, covering the dead arm), targetable (covers the
        -- NOT_SELECTABLE / NON_ATTACKABLE / GM arms).
        if (bot:IsHostileTo(player) or player:IsHostileTo(bot))
            and bot:IsValidAttackTarget(player)
            and player:IsTargetableForAttack() then
            if guard == nil then
                guard = bot:FindNearestCreature(guardEntry, 150)
                if guard == nil then
                    guard = bot:SummonCreature(guardEntry, 0, 0, 0, 0, 3, 300000)
                end
            end
            if guard ~= nil and not guard:IsEngagedBy(player) then
                guard:EngageWithTarget(player)
                guard:CastSpell(player, SPELL_GUARDS_MARK, true)
            end
        end
    end
end

for entry in pairs(ALARMBOT_GUARDS) do
    RegisterCreatureEvent(entry, 7, airForceBotsTick)
end
