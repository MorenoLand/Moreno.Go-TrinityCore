-- npc_air_force_bots (C++ src/server/scripts/World/npcs_special.cpp:43-188).
-- TRIPWIRE PORTED 2026-10-24 on the new player:IsFlying bridge
-- (Unit.h:1644 FLYING|DISABLE_GRAVITY twin over the session's last
-- movement packet); the :174 flying skip reads false for taxi
-- passengers (Go simulates taxi server-side, no in-flight packet).
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
local RANGE_TRIPWIRE = 15

-- myEntry -> guard entry, C++ airforceSpawns table (:72-98), ALARMBOT rows only.
local ALARMBOT_GUARDS = {
    [2614] = 15241, [2615] = 15242, [21974] = 21976, [21993] = 15242,
    [21996] = 15241, [21997] = 21976, [22065] = 22064, [22066] = 22067,
    [22069] = 22064, [22071] = 22067, [22078] = 22077, [22079] = 22077,
    [22086] = 22085, [22087] = 22085, [22090] = 22089, [22124] = 22122,
    [22125] = 22122,
}

-- myEntry -> guard entry, same table, TRIPWIRE rows only. 22126 was
-- misfiled in ALARMBOT_GUARDS by the 2026-10-24 ALARMBOT port (C++
-- marks it TRIPWIRE); it moves here, trading range 100 + the mark
-- cast for range 15 + the flying skip.
local TRIPWIRE_GUARDS = {
    [21999] = 15241, [22001] = 15242, [22002] = 15242, [22003] = 15241,
    [22063] = 21976, [22068] = 22064, [22070] = 22067, [22080] = 22077,
    [22088] = 22085, [22126] = 22122,
}

local function airForceBotsTick(event, bot)
    local guardEntry = ALARMBOT_GUARDS[bot:GetEntry()]
    local scanRange = RANGE_ALARMBOT
    local isTripwire = false
    if guardEntry == nil then
        guardEntry = TRIPWIRE_GUARDS[bot:GetEntry()]
        if guardEntry == nil then
            return
        end
        scanRange = RANGE_TRIPWIRE
        isTripwire = true
    end
    local players = bot:GetPlayersInGrid(scanRange)
    if players == nil then
        return
    end
    local guard = nil
    for _, player in ipairs(players) do
        -- C++ MoveInLineOfSight gates, in order: TYPEID_PLAYER (the grid
        -- returns players only), range (the grid radius), hostile either
        -- way, valid attack target (hostility verdict; the grid is
        -- alive-only, covering the dead arm), TRIPWIRE flying skip :174,
        -- then targetable (covers the NOT_SELECTABLE / NON_ATTACKABLE /
        -- GM arms; pure predicates, so the targetable/flying order swap
        -- changes no outcome).
        if (bot:IsHostileTo(player) or player:IsHostileTo(bot))
            and bot:IsValidAttackTarget(player)
            and (not isTripwire or not player:IsFlying())
            and player:IsTargetableForAttack() then
            if guard == nil then
                guard = bot:FindNearestCreature(guardEntry, 150)
                if guard == nil then
                    guard = bot:SummonCreature(guardEntry, 0, 0, 0, 0, 3, 300000)
                end
            end
            if guard ~= nil and not guard:IsEngagedBy(player) then
                guard:EngageWithTarget(player)
                -- C++ :148-149: the mark cast is ALARMBOT-only;
                -- tripwire guards engage without it.
                if not isTripwire then
                    guard:CastSpell(player, SPELL_GUARDS_MARK, true)
                end
            end
        end
    end
end

for entry in pairs(ALARMBOT_GUARDS) do
    RegisterCreatureEvent(entry, 7, airForceBotsTick)
end

for entry in pairs(TRIPWIRE_GUARDS) do
    RegisterCreatureEvent(entry, 7, airForceBotsTick)
end
