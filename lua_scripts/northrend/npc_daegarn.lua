-- Daegarn (Howling Fjord) --
-- Lua port of src/server/scripts/Northrend/zone_howling_fjord.cpp
-- (npc_daegarn — Reset 40s idle-Talk loop + quest-11300 gladiator
-- machine: OnQuestAccept chain, JustSummoned walk-to-center,
-- SummonedCreatureDies gladiator chain, 20s/5s stuck watchdog).
-- Entry: 24151 'Daegarn' (TDB-verified, ScriptName npc_daegarn,
-- faction 1892).
-- Gladiators (TDB-verified, faction 1919, ScriptName ''): Firjus
-- 24213 'Firjus the Soul Crusher', Jlarborn 24215 'Jlarborn the
-- Strategist', Yoros 24214 'Yorus the Flesh Harvester', Oluf 23931
-- 'Oluf the Violent' — no C++ AI of their own; Daegarn drives them.
-- C++ Reset (:279-291): state clears + DespawnAll + 40s repeating
-- Talk(SAY_TEXT 0); ported as resetState below (ev23, also fires on
-- spawn — fireCreatureSpawned pre-fires OnReset).
-- C++ OnQuestAccept (:375-403): quest 11300, guarded by
-- _eventInProgress; stores the player GUID, cancels the scheduler,
-- despawns summons, arms the 20s watchdog, summons Firjus.
-- C++ JustSummoned (:326-342): tracks the summon; a live player
-- gets SetWalk(false) + SetHomePosition(center) + MovePoint(0,
-- center); otherwise Reset().
-- C++ SummonedCreatureDespawn (:344-346): untracks the summon.
-- C++ SummonedCreatureDies (:349-374): untracks, then chains
-- Firjus -> Jlarborn -> Yoros -> Oluf -> Reset().
-- C++ watchdog (:388-410): 20s, then every 5s while the player is
-- alive + engaged and the FIRST summon engages the player; else
-- Reset(). The C++ loop reads *_summons.begin() on every iteration
-- instead of *itr, so only the first summon is ever tested —
-- replicated here via summons[1].
-- C++ SummonGladiator (:415-418): SummonCreature(entry, summonPos,
-- TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 30s).
-- C++ EnterEvadeMode (:293-313) stays out: MoveFollow /
-- MoveTargetedHome / VehicleKit legs, no Lua equivalent in the
-- ported model (Daegarn never fights; no charmer/vehicle in play).
-- Bridges: creature:Talk lua_creature_events.go:1252; quest:GetId()
-- (hanes precedent); creature:SummonCreature (type 3 =
-- TIMED_DESPAWN_OUT_OF_COMBAT, lua_creature_events.go:4436);
-- creature:SetWalk (:4319); creature:SetHomePosition (:7489);
-- creature:MoveTo is the MovePoint twin (valiant precedent);
-- creature:GetPlayerByGUID (:4733); player:IsInCombat() is the
-- Unit::IsEngaged twin (lua_player.go:1412); summon:IsEngagedBy
-- (player) is the player->IsEngagedBy(summon) twin — same threat
-- entry, receiver swapped (lua_creature_events.go:1594);
-- creature:GetCreatureByGUID takes the low GUID (:4696);
-- CreateLuaEvent one-shot = repeats 1 (runtime.go:2457), 0 repeats
-- forever; SummonedCreatureDespawn is ev20 (runtime.go:382), not
-- scanner-tracked but needed as the DespawnAll twin.
-- Deviations: an unknown death-chain entry summons entry 0 in C++
-- (fails silently on a null template); Lua skips the call.
-- Eluna creature events: 19 JustSummoned, 20 SummonedCreatureDespawn,
-- 21 SummonedCreatureDies, 23 OnReset, 31 OnQuestAccept (ev23 fires
-- on spawn too — fireCreatureSpawned pre-fires OnReset, then
-- OnSpawn; the hanes ev31 latch precedent).

local DAEGARN_ENTRY = 24151
local QUEST_DEFEAT_AT_RING = 11300
local SAY_TEXT = 0

local NPC_FIRJUS = 24213
local NPC_JLARBORN = 24215
local NPC_YOROS = 24214
local NPC_OLUF = 23931

local SUMMON_POS = {838.81, -4678.06, -94.182, 0.0}
local CENTER_POS = {801.88, -4721.87, -96.143, 0.0}

local NEXT_GLADIATOR = {
    [NPC_FIRJUS] = NPC_JLARBORN,
    [NPC_JLARBORN] = NPC_YOROS,
    [NPC_YOROS] = NPC_OLUF,
}

local daegarnTalk = {}
local daegarnWatchdog = {}
local daegarnEvent = {} -- [guid] = { playerGuid = n, summons = {guidLow,...}, entries = {[guidLow] = entry} }

local function cancelTalk(guid)
    local id = daegarnTalk[guid]
    if id then
        RemoveEventById(id)
        daegarnTalk[guid] = nil
    end
end

local function cancelWatchdog(guid)
    local id = daegarnWatchdog[guid]
    if id then
        RemoveEventById(id)
        daegarnWatchdog[guid] = nil
    end
end

local function untrackSummon(st, guidLow)
    for i, g in ipairs(st.summons) do
        if g == guidLow then
            table.remove(st.summons, i)
            break
        end
    end
    st.entries[guidLow] = nil
end

local function despawnSummons(creature, guid)
    local st = daegarnEvent[guid]
    if not st then
        return
    end
    local snapshot = {}
    for _, g in ipairs(st.summons) do
        snapshot[#snapshot + 1] = g
    end
    for _, g in ipairs(snapshot) do
        local summon = creature:GetCreatureByGUID(g)
        if summon then
            summon:Despawn()
        end
    end
    st.summons = {}
    st.entries = {}
end

-- C++ Reset (:279-291): state clears + DespawnAll + 40s Talk re-arm.
local function resetState(creature)
    local guid = creature:GetGUIDLow()
    cancelWatchdog(guid)
    despawnSummons(creature, guid)
    daegarnEvent[guid] = nil
    cancelTalk(guid)
    daegarnTalk[guid] = CreateLuaEvent(function()
        creature:Talk(SAY_TEXT)
    end, 40000, 0)
end

local function onReset(_, creature)
    resetState(creature)
end

local function summonGladiator(creature, entry)
    if entry == nil or entry == 0 then
        return
    end
    creature:SummonCreature(entry, SUMMON_POS[1], SUMMON_POS[2], SUMMON_POS[3], SUMMON_POS[4], 3, 30000)
end

-- C++ watchdog (:388-410): 20s first fire, then 5s while held.
local function armWatchdog(creature, guid, delay)
    cancelWatchdog(guid)
    daegarnWatchdog[guid] = CreateLuaEvent(function()
        daegarnWatchdog[guid] = nil
        local st = daegarnEvent[guid]
        if not st then
            return
        end
        local reset = true
        local player = creature:GetPlayerByGUID(st.playerGuid)
        if player and player:IsAlive() and player:IsInCombat() then
            local first = st.summons[1]
            if first then
                local summon = creature:GetCreatureByGUID(first)
                if summon and summon:IsEngagedBy(player) then
                    reset = false
                    armWatchdog(creature, guid, 5000)
                end
            end
        end
        if reset then
            resetState(creature)
        end
    end, delay, 1)
end

-- C++ OnQuestAccept (:375-403): quest 11300 starts the chain.
local function onQuestAccept(_, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_DEFEAT_AT_RING then
        return
    end
    local guid = creature:GetGUIDLow()
    if daegarnEvent[guid] then
        return
    end
    cancelTalk(guid)
    cancelWatchdog(guid)
    despawnSummons(creature, guid)
    daegarnEvent[guid] = { playerGuid = player:GetGUID(), summons = {}, entries = {} }
    armWatchdog(creature, guid, 20000)
    summonGladiator(creature, NPC_FIRJUS)
end

-- C++ JustSummoned (:326-342).
local function onJustSummoned(_, creature, summon)
    local guid = creature:GetGUIDLow()
    local st = daegarnEvent[guid]
    if not st then
        return
    end
    local summonGuid = summon:GetGUIDLow()
    st.summons[#st.summons + 1] = summonGuid
    st.entries[summonGuid] = summon:GetEntry()
    local player = creature:GetPlayerByGUID(st.playerGuid)
    if player and player:IsAlive() then
        summon:SetWalk(false)
        summon:SetHomePosition(CENTER_POS[1], CENTER_POS[2], CENTER_POS[3], CENTER_POS[4])
        summon:MoveTo(0, CENTER_POS[1], CENTER_POS[2], CENTER_POS[3])
    else
        resetState(creature)
    end
end

-- C++ SummonedCreatureDespawn (:344-346).
local function onSummonedDespawn(_, creature, summon)
    local guid = creature:GetGUIDLow()
    local st = daegarnEvent[guid]
    if not st then
        return
    end
    untrackSummon(st, summon:GetGUIDLow())
end

-- C++ SummonedCreatureDies (:349-374).
local function onSummonedDied(_, creature, summon, killer)
    local guid = creature:GetGUIDLow()
    local st = daegarnEvent[guid]
    if not st then
        return
    end
    local entry = summon:GetEntry()
    untrackSummon(st, summon:GetGUIDLow())
    if entry == NPC_OLUF then
        resetState(creature)
        return
    end
    summonGladiator(creature, NEXT_GLADIATOR[entry] or 0)
end

RegisterCreatureEvent(DAEGARN_ENTRY, 19, onJustSummoned)
RegisterCreatureEvent(DAEGARN_ENTRY, 20, onSummonedDespawn)
RegisterCreatureEvent(DAEGARN_ENTRY, 21, onSummonedDied)
RegisterCreatureEvent(DAEGARN_ENTRY, 23, onReset)
RegisterCreatureEvent(DAEGARN_ENTRY, 31, onQuestAccept)
