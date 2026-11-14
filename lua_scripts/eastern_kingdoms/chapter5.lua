-- npc_highlord_darion_mograine (Scarlet Enclave, chapter5.cpp) --
-- Lua port of the npc_highlord_darion_mograine EscortAI in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter5.cpp
-- (lines 32-1650). Entry 29173 verifiable from the C++ enum
-- (NPC_HIGHLORD_DARION_MOGRAINE, chapter5.cpp:139). The Light of
-- Dawn event: gossip start -> intro step machine (steps 0-9) ->
-- escort waypoints 0-1 -> 5-minute battle -> post-battle steps
-- 11-73 (next tick) -> escort waypoints 2-8.
-- Ported arms (this tick):
-- - Reset (ev23): Initialize + Mount(25279) + SetVisible(true) +
--   world states REMAINS 0 / EVENT_BEGIN 0 + stored-creature
--   sweep (dawn dead, death respawned), skipped while escorting.
-- - Gossip (RegisterCreatureGossipEvent 1/2): quest 12801
--   INCOMPLETE shows "I am ready."; select clears/closes the menu,
--   sets step 1, StartEscort(player) + MoveWaypoint (legoso idiom).
-- - WaypointReached (ev6, GetCurrentWaypointId): wp0 run-off +
--   hold; wp1 hold + SpawnNPC + Korfax/Maxwell talks + retarget
--   waves + Dismount + Might of Mograine (triggered) + battle
--   start; wp2 walk + Light of Dawn self-cast; wp3 regroup
--   MovePoints + JumpToNextStep(10000); wp4 kneel + hold; wp5
--   hold; wp6 hold + one-shot special-attack emote +
--   JumpToNextStep(1000); wp7 hold + JumpToNextStep(2000); wp8
--   main-hand unequip + Ashbringer on Tirion (triggered) +
--   emote 81 + hold.
-- - uiStep machine steps 0-9 (phase pump, CreateLuaEvent
--   one-shots, maiden idiom): step 1 world states + gossip-flag
--   clear; step 2 Talk(3) + nearest Koltira/Orbaz/Thassarian
--   capture (GetCreaturesInRange, distance-ordered, illidan
--   precedent — no GetClosestCreatureWithEntry bridge); step 3
--   Talk(4); steps 4-7 ghoul/abomination/warrior/behemoth
--   summon loops (500ms re-fire, shack 33271 each tick);
--   step 8 Talk(5); step 9 escort resume + charge scatter.
-- - SpawnNPC (waypoint 1 + 1s battle spawncheck): refills every
--   wave from the stored GUIDs, dawn side set to Darion's
--   faction + AddThreat 0, scourge side to 2084.
-- - NPCChangeTarget: random player in 100yd (mellichar idiom —
--   C++ SelectTarget(Random, 0) reads Darion's threat list) +
--   AddThreat 0 + AttackStart on the summoned creature.
-- - Battle pump (1s tick while bIsBattle): anti-magic zone /
--   death strike / death embrace / icy touch / unholy blight /
--   fight speech timers, spawncheck, targetcheck, Tirion summon
--   at fight-duration <= 5000ms, battle end at 0 (aura strip,
--   CombatStop, wave despawns, evade/stand/faction resets,
--   regroup MovePoints, escort resume).
-- Deltas:
-- - Step 10 has NO case in C++ (cases run 9 -> 11): after step 9
--   the machine sits at step 10; the C++ timer is never reset so
--   step 10 re-fires every UpdateAI tick as a no-op. The Lua pump
--   polls step 10 once per second instead of busy-spinning; the
--   waypoint-3 JumpToNextStep(10000) is what advances it to 11 in
--   both versions (the escort machine bridges the hole).
-- - SpawnNPC reproduces the C++ abomination/warrior entry swap
--   (chapter5.cpp:1505-1525): the abomination loop summons
--   NPC_WARRIOR_OF_THE_FROZEN_WASTES (29206) into the abomination
--   slots and the warrior loop summons NPC_RAMPAGING_ABOMINATION
--   (29186) into the warrior slot. Upstream bug, ported exact.
-- - Battle timers tick at 1s granularity (C++ is per-UpdateAI
--   tick); re-arms use the same urand ranges.
-- - EngagementOver (CreatureAI.cpp:282, engagement-flag reset)
--   has no bridge; subsumed by the battle-end CombatStop(true)
--   + InterruptNonMeleeSpells + RemoveAllAuras sequence.
-- - MoveInLineOfSight 20yd + LOS aggro leg stays out (ev27 is
--   engage-only, no LOS-gated bridge); engine aggro covers the
--   approach. PrepareQuestMenu has no bridge (gossip hello
--   omits it). Steps 11-73 (post-battle dialogue machine) are
--   the next unit, same file.
-- - npc_the_lich_king_tirion_dawn (29183): empty Reset/UpdateAI/
--   JustDied, AttackStart suppressed — nothing to port.
-- - spell_teleport_leaders_blessing: SpellScript, no bridge.
local ENTRY_DARION = 29173
local ENTRY_KOLTIRA = 29199
local ENTRY_ORBAZ = 29204
local ENTRY_THASSARIAN = 29200
local ENTRY_LICH_KING = 29183
local ENTRY_ABOMINATION = 29186
local ENTRY_BEHEMOTH = 29190
local ENTRY_GHOUL = 29219
local ENTRY_WARRIOR = 29206
local ENTRY_ALEXANDROS = 29227
local ENTRY_DARION_GHOST = 29228
local ENTRY_TIRION = 29175
local ENTRY_KORFAX = 29176
local ENTRY_MAXWELL = 29178
local ENTRY_ELIGOR = 29177
local ENTRY_RAYNE = 29181
local ENTRY_DEFENDER = 29174
local ENTRY_RIMBLAT = 29182

local SPELL_ANTI_MAGIC_ZONE = 52893
local SPELL_DEATH_STRIKE = 53639
local SPELL_DEATH_EMBRACE = 53635
local SPELL_ICY_TOUCH = 49723
local SPELL_LIGHT_OF_DAWN = 53658
local SPELL_MIGHT_OF_MOGRAINE = 53642
local SPELL_UNHOLY_BLIGHT = 53640
local SPELL_MOGRAINE_CHARGE = 53679
local SPELL_ASHBRINGER = 53701
local SPELL_SHACK = 33271
local SPELL_THE_LIGHT_OF_DAWN_Q = 53606

local SAY_INTRO = 3 -- SAY_LIGHT_OF_DAWN04
local SAY_RISE = 4 -- SAY_LIGHT_OF_DAWN05
local SAY_ANNOUNCE = 5 -- SAY_LIGHT_OF_DAWN06
local SAY_KORFAX = 6 -- SAY_LIGHT_OF_DAWN07
local SAY_MAXWELL = 7 -- SAY_LIGHT_OF_DAWN08
local SAY_FIGHT = 8 -- SAY_LIGHT_OF_DAWN09
local SAY_KNEEL = 26 -- SAY_LIGHT_OF_DAWN27
local SAY_SWORD = 32 -- SAY_LIGHT_OF_DAWN33
local EMOTE_REGROUP = 72 -- EMOTE_LIGHT_OF_DAWN05
local EMOTE_ASHBRINGER = 81 -- EMOTE_LIGHT_OF_DAWN14
local EMOTE_SPECIAL_ATTACK = 54 -- EMOTE_ONESHOT_SPECIALATTACK1H
local EMOTE_READY_2H = 375 -- EMOTE_STATE_READY2H

local WORLD_STATE_REMAINS = 3592
local WORLD_STATE_COUNTDOWN = 3603
local WORLD_STATE_EVENT_BEGIN = 3605
local FACTION_SCOURGE = 2084 -- FACTION_UNDEAD_SCOURGE_3
local FACTION_SCARLET = 2089 -- FACTION_SCARLET_CRUSADE
local QUEST_LIGHT_OF_DAWN = 12801
local QUEST_INCOMPLETE = 3
local UNIT_NPC_FLAG_GOSSIP = 0x00000001
local UNIT_NPC_FLAGS_FIELD = 82 -- OBJECT_END 6 + 0x4C
local STAND_STAND = 0
local STAND_KNEEL = 8
local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local SUMMON_TYPE_CORPSE = 5 -- Go 0-based TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN

local COUNT_DEFENDER = 10
local COUNT_EARTHSHATTER = 1
local COUNT_ABOMINATION = 2
local COUNT_BEHEMOTH = 1
local COUNT_GHOUL = 5
local COUNT_WARRIOR = 1

-- chapter5.cpp:240-272 (Lua 1-based: Loc[i+1]).
local LOC = {
    {2281.335, -5300.409, 85.170, 0},
    {2283.896, -5287.914, 83.066, 1.55},
    {2281.461, -5263.014, 81.164, 0},
    {2262.277, -5293.477, 82.167, 0},
    {2270.286, -5287.73, 82.262, 0},
    {2269.511, -5288.289, 82.225, 0},
    {2262.277, -5293.477, 82.167, 0},
    {2270.286, -5287.73, 82.262, 0},
    {2269.511, -5288.289, 82.225, 0},
    {2273.205, -5288.848, 82.617, 0},
    {2274.739, -5287.926, 82.684, 0},
    {2253.673, -5318.004, 81.724, 0},
    {2287.028, -5309.644, 87.253, 0},
    {2286.978, -5308.025, 86.83, 0},
    {2248.877, -5307.586, 82.166, 0},
    {2278.58, -5316.933, 88.319, 0},
    {2278.535, -5315.479, 88.08, 0},
    {2259.416, -5304.505, 82.149, 0},
    {2289.259, -5280.355, 82.112, 0},
    {2289.02, -5281.985, 82.207, 0},
    {2273.289, -5273.675, 81.701, 0},
    {2273.332, -5275.544, 81.849, 0},
    {2281.198, -5257.397, 80.224, 4.66},
    {2281.156, -5259.934, 80.647, 0},
    {2281.294, -5281.895, 82.445, 1.35},
    {2281.093, -5263.013, 81.125, 0},
    {2281.313, -5250.282, 79.322, 4.69},
    {2281.523, -5261.058, 80.877, 0},
    {2272.709, -5255.552, 78.226, 0},
    {2273.972, -5257.676, 78.862, 0},
}

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function newState()
    return {
        step = 0, isBattle = false, escorting = false,
        fightDuration = 300000, summonCounter = 0,
        antiMagic = math.random(1000, 6000),
        deathStrike = math.random(5000, 10000),
        deathEmbrace = math.random(5000, 10000),
        icyTouch = math.random(5000, 10000),
        unholyBlight = math.random(5000, 10000),
        fightSpeech = 15000, spawncheck = 1000, targetcheck = 10000,
        tirion = nil, alexandros = nil, darionGhost = nil,
        korfax = nil, maxwell = nil, eligor = nil, rayne = nil,
        koltira = nil, orbaz = nil, thassarian = nil, lichKing = nil,
        defender = {}, earthshatter = {},
        abomination = {}, behemoth = {}, ghoul = {}, warrior = {},
    }
end

local function getState(guid)
    local st = state[guid]
    if not st then
        st = newState()
        state[guid] = st
    end
    return st
end

-- ObjectAccessor::GetCreature twin (nil when gone/despawned).
local function getc(creature, guid)
    if not guid then
        return nil
    end
    return creature:GetCreatureByGUID(guid)
end

local function storeGuid(creature)
    if not creature then
        return nil
    end
    return creature:GetGUIDLow()
end

local function nearestEntry(creature, entry, range)
    local list = creature:GetCreaturesInRange(range, entry, 0, 0)
    if list ~= nil and #list > 0 then
        return list[1]
    end
    return nil
end

-- Forward declarations: the pumps reference each other. They must sit
-- ABOVE jumpToNextStep (the tharon_ja/gal_darah idiom — plain
-- assignment below, never a second `local function`, or closures
-- bind the nil forward slot).
local stepTick, battleTick, spawnNPC, npcChangeTarget, despawnNPC

local function jumpToNextStep(creature, guid, delay)
    local st = getState(guid)
    st.step = st.step + 1
    schedule(guid, "phase", delay, function()
        stepTick(creature, guid)
    end)
end

-- chapter5.cpp:1501-1599 SpawnNPC. NOTE the entry swap (see header):
-- the abomination loop summons the warrior entry and vice versa.
spawnNPC = function(creature, guid)
    local st = getState(guid)
    local loc0 = LOC[1]
    local function summonAt(entry, x, y, z, o, ms)
        return creature:SummonCreature(entry, x, y, z, o, SUMMON_TYPE_CORPSE, ms)
    end
    local function scatter(entry)
        return loc0[1] + math.random(0, 29), loc0[2] + math.random(0, 29), loc0[3]
    end
    for i = 1, COUNT_GHOUL do
        if not getc(creature, st.ghoul[i]) then
            local x, y, z = scatter()
            local temp = summonAt(ENTRY_GHOUL, x, y, z, 0, 300000)
            if temp then
                temp:SetFaction(FACTION_SCOURGE)
                st.ghoul[i] = storeGuid(temp)
            end
        end
    end
    for i = 1, COUNT_ABOMINATION do
        if not getc(creature, st.abomination[i]) then
            local x, y, z = scatter()
            local temp = summonAt(ENTRY_WARRIOR, x, y, z, 0, 300000)
            if temp then
                temp:SetFaction(FACTION_SCOURGE)
                st.abomination[i] = storeGuid(temp)
            end
        end
    end
    for i = 1, COUNT_WARRIOR do
        if not getc(creature, st.warrior[i]) then
            local x, y, z = scatter()
            local temp = summonAt(ENTRY_ABOMINATION, x, y, z, 0, 300000)
            if temp then
                temp:SetFaction(FACTION_SCOURGE)
                st.warrior[i] = storeGuid(temp)
            end
        end
    end
    for i = 1, COUNT_BEHEMOTH do
        if not getc(creature, st.behemoth[i]) then
            local x, y, z = scatter()
            local temp = summonAt(ENTRY_BEHEMOTH, x, y, z, 0, 300000)
            if temp then
                temp:SetFaction(FACTION_SCOURGE)
                st.behemoth[i] = storeGuid(temp)
            end
        end
    end
    for i = 1, COUNT_DEFENDER do
        if not getc(creature, st.defender[i]) then
            local x, y, z = scatter()
            local temp = summonAt(ENTRY_DEFENDER, x, y, z, 0, 300000)
            if temp then
                temp:SetFaction(FACTION_SCARLET)
                creature:AddThreat(temp, 0)
                st.defender[i] = storeGuid(temp)
            end
        end
    end
    for i = 1, COUNT_EARTHSHATTER do
        if not getc(creature, st.earthshatter[i]) then
            local x, y, z = scatter()
            local temp = summonAt(ENTRY_RIMBLAT, x, y, z, 0, 300000)
            if temp then
                temp:SetFaction(FACTION_SCARLET)
                creature:AddThreat(temp, 0)
                st.earthshatter[i] = storeGuid(temp)
            end
        end
    end
    local function summonHero(key, entry, ms)
        local temp = getc(creature, st[key])
        if not temp then
            local x, y, z = scatter()
            temp = summonAt(entry, x, y, z, 0, ms)
            if temp then
                temp:SetFaction(FACTION_SCARLET)
                creature:AddThreat(temp, 0)
                st[key] = storeGuid(temp)
            end
        end
    end
    summonHero("korfax", ENTRY_KORFAX, 600000)
    summonHero("maxwell", ENTRY_MAXWELL, 600000)
    summonHero("eligor", ENTRY_ELIGOR, 600000)
    summonHero("rayne", ENTRY_RAYNE, 300000)
end

-- chapter5.cpp:1491-1499 NPCChangeTarget.
npcChangeTarget = function(creature, guid)
    local temp = getc(creature, guid)
    if not temp or temp:IsDead() then
        return
    end
    local players = creature:GetPlayersInRange(100)
    if players == nil or #players == 0 then
        return
    end
    local target = players[math.random(#players)]
    if target and not target:IsDead() then
        temp:AddThreat(target, 0)
        temp:AttackStart(target)
    end
end

local function retargetWaves(creature, st)
    for i = 1, COUNT_GHOUL do
        npcChangeTarget(creature, st.ghoul[i])
    end
    for i = 1, COUNT_WARRIOR do
        npcChangeTarget(creature, st.warrior[i])
    end
    for i = 1, COUNT_ABOMINATION do
        npcChangeTarget(creature, st.abomination[i])
    end
    for i = 1, COUNT_BEHEMOTH do
        npcChangeTarget(creature, st.behemoth[i])
    end
    npcChangeTarget(creature, st.koltira)
    npcChangeTarget(creature, st.orbaz)
    npcChangeTarget(creature, st.thassarian)
end

-- chapter5.cpp:1601-1608 DespawnNPC.
despawnNPC = function(creature, guid)
    local temp = getc(creature, guid)
    if temp and not temp:IsDead() then
        temp:SetVisible(false)
        temp:Kill()
    end
end

-- chapter5.cpp:584-733 uiStep machine, steps 0-9 (+ step 10 no-op).
stepTick = function(creature, guid)
    local st = getState(guid)
    if st.isBattle or st.step >= 11 then
        return
    end
    local step = st.step
    if step == 0 then
        -- Countdown: only commented-out world-state writes in C++.
        schedule(guid, "phase", 3000, function()
            stepTick(creature, guid)
        end)
    elseif step == 1 then
        creature:UpdateWorldState(WORLD_STATE_COUNTDOWN, 0)
        creature:UpdateWorldState(WORLD_STATE_EVENT_BEGIN, 1)
        creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        jumpToNextStep(creature, guid, 3000)
    elseif step == 2 then
        creature:Talk(SAY_INTRO)
        local koltira = nearestEntry(creature, ENTRY_KOLTIRA, 50)
        if koltira then
            st.koltira = storeGuid(koltira)
        end
        local orbaz = nearestEntry(creature, ENTRY_ORBAZ, 50)
        if orbaz then
            st.orbaz = storeGuid(orbaz)
        end
        local thassarian = nearestEntry(creature, ENTRY_THASSARIAN, 50)
        if thassarian then
            st.thassarian = storeGuid(thassarian)
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 3 then
        creature:Talk(SAY_RISE)
        jumpToNextStep(creature, guid, 3000)
    elseif step >= 4 and step <= 7 then
        -- Summon loops: 500ms re-fire until the counter exhausts,
        -- then step++ with the timer left at 500ms (C++-exact).
        creature:CastSpell(creature, SPELL_SHACK)
        local entry, count, key
        if step == 4 then
            entry, count, key = ENTRY_GHOUL, COUNT_GHOUL, "ghoul"
        elseif step == 5 then
            entry, count, key = ENTRY_ABOMINATION, COUNT_ABOMINATION, "abomination"
        elseif step == 6 then
            entry, count, key = ENTRY_WARRIOR, COUNT_WARRIOR, "warrior"
        else
            entry, count, key = ENTRY_BEHEMOTH, COUNT_BEHEMOTH, "behemoth"
        end
        if st.summonCounter < count then
            local temp = creature:SummonCreature(entry,
                creature:GetX() - 20 + math.random(0, 39),
                creature:GetY() - 20 + math.random(0, 39),
                creature:GetZ(), 0, SUMMON_TYPE_CORPSE, 300000)
            if temp then
                temp:SetWalk(false)
                temp:SetFaction(FACTION_SCOURGE)
                st[key][st.summonCounter + 1] = storeGuid(temp)
            end
            st.summonCounter = st.summonCounter + 1
            schedule(guid, "phase", 500, function()
                stepTick(creature, guid)
            end)
        else
            st.summonCounter = 0
            st.step = st.step + 1
            schedule(guid, "phase", 500, function()
                stepTick(creature, guid)
            end)
        end
    elseif step == 8 then
        creature:Talk(SAY_ANNOUNCE)
        jumpToNextStep(creature, guid, 5000)
    elseif step == 9 then
        -- Charge begins: escort resumes, the death side scatters
        -- to random points around LightofDawnLoc[0].
        creature:SetEscortPaused(false)
        local loc0 = LOC[1]
        local function scatterMove(guidKey)
            local temp = getc(creature, guidKey)
            if temp then
                temp:SetWalk(false)
                temp:MoveTo(0, loc0[1] + math.random(0, 29),
                    loc0[2] + math.random(0, 29), loc0[3])
            end
        end
        scatterMove(st.koltira)
        scatterMove(st.orbaz)
        scatterMove(st.thassarian)
        for i = 1, COUNT_ABOMINATION do
            scatterMove(st.abomination[i])
        end
        for i = 1, COUNT_BEHEMOTH do
            scatterMove(st.behemoth[i])
        end
        for i = 1, COUNT_GHOUL do
            scatterMove(st.ghoul[i])
        end
        for i = 1, COUNT_WARRIOR do
            scatterMove(st.warrior[i])
        end
        jumpToNextStep(creature, guid, 5000)
    else
        -- Step 10: no case in C++; the 1s poll stands in for the
        -- per-tick no-op re-fire (waypoint 3 advances past it).
        schedule(guid, "phase", 1000, function()
            stepTick(creature, guid)
        end)
    end
end

-- chapter5.cpp:1378-1481 battle end (fight duration expired).
local function endBattle(creature, guid)
    local st = getState(guid)
    st.isBattle = false
    st.fightDuration = 300000
    if creature:HasAura(SPELL_MIGHT_OF_MOGRAINE) then
        creature:RemoveAura(SPELL_MIGHT_OF_MOGRAINE)
    end
    creature:RemoveAllAuras()
    creature:CombatStop(true)
    creature:InterruptNonMeleeSpells(false)
    creature:SetWalk(false)
    for i = 1, COUNT_DEFENDER do
        despawnNPC(creature, st.defender[i])
    end
    for i = 1, COUNT_EARTHSHATTER do
        despawnNPC(creature, st.earthshatter[i])
    end
    for i = 1, COUNT_ABOMINATION do
        despawnNPC(creature, st.abomination[i])
    end
    for i = 1, COUNT_BEHEMOTH do
        despawnNPC(creature, st.behemoth[i])
    end
    for i = 1, COUNT_GHOUL do
        despawnNPC(creature, st.ghoul[i])
    end
    for i = 1, COUNT_WARRIOR do
        despawnNPC(creature, st.warrior[i])
    end
    local faction = creature:GetFaction()
    local function standDown(guidKey, locIndex, evade, castLight)
        local temp = getc(creature, guidKey)
        if not temp then
            return
        end
        if evade then
            temp:EnterEvadeMode()
        end
        temp:RemoveAllAuras()
        temp:CombatStop(true)
        temp:AttackStop()
        temp:SetFaction(faction)
        temp:SetWalk(false)
        local loc = LOC[locIndex]
        temp:MoveTo(0, loc[1], loc[2], loc[3])
        if castLight then
            temp:CastSpell(temp, SPELL_LIGHT_OF_DAWN, false)
        end
    end
    standDown(st.korfax, 10, true, false)
    standDown(st.maxwell, 13, false, false)
    standDown(st.eligor, 16, true, false)
    despawnNPC(creature, st.rayne)
    standDown(st.koltira, 19, true, true)
    local orbaz = getc(creature, st.orbaz)
    if orbaz then
        orbaz:Talk(71) -- EMOTE_LIGHT_OF_DAWN04
    end
    standDown(st.thassarian, 21, true, true)
    local tirion = getc(creature, st.tirion)
    if tirion then
        tirion:Talk(25) -- SAY_LIGHT_OF_DAWN26
    end
    creature:SetEscortPaused(false)
    -- The machine resumes at step 10 (no-op poll) until the escort
    -- reaches waypoint 3, whose JumpToNextStep advances it to 11.
    schedule(guid, "phase", 1000, function()
        stepTick(creature, guid)
    end)
end

-- chapter5.cpp:1275-1377 battle branch, 1s tick.
battleTick = function(creature, guid)
    local st = getState(guid)
    if not st.isBattle then
        return
    end
    local function fire(timerKey, spell, victim, rearmMin, rearmMax)
        if st[timerKey] <= 1000 then
            if victim then
                creature:CastSpell(nil, spell)
            else
                creature:CastSpell(creature, spell)
            end
            st[timerKey] = math.random(rearmMin, rearmMax)
        else
            st[timerKey] = st[timerKey] - 1000
        end
    end
    fire("antiMagic", SPELL_ANTI_MAGIC_ZONE, false, 25000, 30000)
    fire("deathStrike", SPELL_DEATH_STRIKE, true, 5000, 10000)
    fire("deathEmbrace", SPELL_DEATH_EMBRACE, true, 5000, 10000)
    fire("icyTouch", SPELL_ICY_TOUCH, true, 5000, 10000)
    fire("unholyBlight", SPELL_UNHOLY_BLIGHT, true, 5000, 10000)
    if st.fightSpeech <= 1000 then
        creature:Talk(SAY_FIGHT)
        st.fightSpeech = math.random(15000, 20000)
    else
        st.fightSpeech = st.fightSpeech - 1000
    end
    if st.spawncheck <= 1000 then
        spawnNPC(creature, guid)
        st.spawncheck = 1000
    else
        st.spawncheck = st.spawncheck - 1000
    end
    if st.targetcheck <= 1000 then
        retargetWaves(creature, st)
        st.targetcheck = 10000
    else
        st.targetcheck = st.targetcheck - 1000
    end
    if st.fightDuration <= 6000 and not st.tirion then
        local loc0 = LOC[1]
        local temp = creature:SummonCreature(ENTRY_TIRION,
            loc0[1], loc0[2], loc0[3], 1.528, SUMMON_TYPE_CORPSE, 600000)
        if temp then
            temp:SetFaction(creature:GetFaction())
            temp:SetEquipmentSlots(0, 0xFFFFFFFF, 0xFFFFFFFF)
            temp:Talk(24) -- SAY_LIGHT_OF_DAWN25
            st.tirion = storeGuid(temp)
        end
    end
    if st.fightDuration <= 1000 then
        endBattle(creature, guid)
        return
    end
    st.fightDuration = st.fightDuration - 1000
    schedule(guid, "battle", 1000, function()
        battleTick(creature, guid)
    end)
end

-- chapter5.cpp:478-573 WaypointReached.
local function onWaypointReached(_, creature)
    local guid = creature:GetGUIDLow()
    local st = getState(guid)
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:SetWalk(false)
        creature:SetEscortPaused(true)
    elseif wp == 1 then
        creature:SetEscortPaused(true)
        spawnNPC(creature, guid)
        local korfax = getc(creature, st.korfax)
        if korfax then
            korfax:Talk(SAY_KORFAX)
        end
        local maxwell = getc(creature, st.maxwell)
        if maxwell then
            maxwell:Talk(SAY_MAXWELL)
        end
        retargetWaves(creature, st)
        creature:Dismount()
        creature:CastSpell(creature, SPELL_MIGHT_OF_MOGRAINE, true)
        local koltira = getc(creature, st.koltira)
        if koltira then
            koltira:Dismount()
        end
        local thassarian = getc(creature, st.thassarian)
        if thassarian then
            thassarian:Dismount()
        end
        st.isBattle = true
        schedule(guid, "battle", 1000, function()
            battleTick(creature, guid)
        end)
    elseif wp == 2 then
        creature:SetWalk(true)
        creature:CastSpell(creature, SPELL_LIGHT_OF_DAWN)
    elseif wp == 3 then
        creature:Talk(EMOTE_REGROUP)
        if creature:HasAura(SPELL_LIGHT_OF_DAWN) then
            creature:RemoveAura(SPELL_LIGHT_OF_DAWN)
        end
        local function regroup(guidKey, locIndex, ready2h)
            local temp = getc(creature, guidKey)
            if not temp then
                return
            end
            if temp:HasAura(SPELL_LIGHT_OF_DAWN) then
                temp:RemoveAura(SPELL_LIGHT_OF_DAWN)
            end
            temp:SetWalk(true)
            if ready2h then
                temp:EmoteState(EMOTE_READY_2H)
            end
            local loc = LOC[locIndex]
            temp:MoveTo(0, loc[1], loc[2], loc[3])
        end
        regroup(st.koltira, 20, false)
        regroup(st.thassarian, 22, false)
        regroup(st.korfax, 11, true)
        regroup(st.maxwell, 14, false)
        regroup(st.eligor, 17, false)
        jumpToNextStep(creature, guid, 10000)
    elseif wp == 4 then
        creature:Talk(SAY_KNEEL)
        creature:SetStandState(STAND_KNEEL)
        local koltira = getc(creature, st.koltira)
        if koltira then
            koltira:SetStandState(STAND_KNEEL)
        end
        local thassarian = getc(creature, st.thassarian)
        if thassarian then
            thassarian:SetStandState(STAND_KNEEL)
        end
        creature:SetEscortPaused(true)
    elseif wp == 5 then
        creature:Talk(SAY_SWORD)
        creature:SetEscortPaused(true)
    elseif wp == 6 then
        creature:SetEscortPaused(true)
        creature:PerformEmote(EMOTE_SPECIAL_ATTACK)
        jumpToNextStep(creature, guid, 1000)
    elseif wp == 7 then
        creature:SetEscortPaused(true)
        jumpToNextStep(creature, guid, 2000)
    elseif wp == 8 then
        creature:SetEquipmentSlots(0, 0xFFFFFFFF, 0xFFFFFFFF)
        local tirion = getc(creature, st.tirion)
        if tirion then
            creature:CastSpell(tirion, SPELL_ASHBRINGER, true)
        end
        creature:Talk(EMOTE_ASHBRINGER)
        creature:SetEscortPaused(true)
    end
end

-- chapter5.cpp:345-449 Reset (skipped while escorting). The sweep
-- reads the OLD state's GUIDs first, then the state is replaced.
local function onReset(_, creature)
    local guid = creature:GetGUIDLow()
    local old = state[guid]
    if old and old.escorting then
        return
    end
    cancelTimers(guid)
    creature:SetStandState(STAND_STAND)
    creature:Mount(25279)
    creature:SetVisible(true)
    creature:UpdateWorldState(WORLD_STATE_REMAINS, 0)
    creature:UpdateWorldState(WORLD_STATE_EVENT_BEGIN, 0)
    -- Stored dawn creatures die; the death side respawns (C++-exact
    -- setDeathState(JUST_DIED) / Respawn legs; GUIDs cleared after).
    if old then
        local function killStored(guidKey)
            local temp = getc(creature, guidKey)
            if temp then
                temp:Kill()
            end
        end
        local function respawnStored(guidKey)
            local temp = getc(creature, guidKey)
            if temp then
                temp:Respawn()
            end
        end
        killStored(old.tirion)
        killStored(old.korfax)
        killStored(old.maxwell)
        killStored(old.eligor)
        killStored(old.rayne)
        for i = 1, COUNT_DEFENDER do
            killStored(old.defender[i])
        end
        for i = 1, COUNT_EARTHSHATTER do
            killStored(old.earthshatter[i])
        end
        respawnStored(old.koltira)
        respawnStored(old.orbaz)
        respawnStored(old.thassarian)
        respawnStored(old.lichKing)
        for i = 1, COUNT_ABOMINATION do
            killStored(old.abomination[i])
        end
        for i = 1, COUNT_BEHEMOTH do
            killStored(old.behemoth[i])
        end
        for i = 1, COUNT_GHOUL do
            killStored(old.ghoul[i])
        end
        for i = 1, COUNT_WARRIOR do
            killStored(old.warrior[i])
        end
    end
    state[guid] = newState()
    schedule(guid, "phase", 3000, function()
        stepTick(creature, guid)
    end)
end

local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

-- chapter5.cpp:1612-1641 gossip.
local function onGossipHello(_, player, creature)
    if player:GetQuestStatus(QUEST_LIGHT_OF_DAWN) == QUEST_INCOMPLETE then
        player:GossipMenuAddItem(GOSSIP_ICON_CHAT, "I am ready.",
            GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    end
    player:GossipSendMenu(player:GetGossipTextId(creature), creature)
end

local function onGossipSelect(_, player, creature, _, action)
    player:GossipClearMenu()
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipComplete()
        local guid = creature:GetGUIDLow()
        local st = getState(guid)
        st.step = 1
        st.escorting = true
        creature:StartEscort(player)
        creature:MoveWaypoint()
    end
end

RegisterCreatureEvent(ENTRY_DARION, 4, onDied)
RegisterCreatureEvent(ENTRY_DARION, 6, onWaypointReached)
RegisterCreatureEvent(ENTRY_DARION, 23, onReset)
RegisterCreatureGossipEvent(ENTRY_DARION, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_DARION, 2, onGossipSelect)
