-- npc_highlord_darion_mograine (Scarlet Enclave, chapter5.cpp) --
-- Lua port of the npc_highlord_darion_mograine EscortAI in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter5.cpp
-- (lines 32-1650). Entry 29173 verifiable from the C++ enum
-- (NPC_HIGHLORD_DARION_MOGRAINE, chapter5.cpp:139). The Light of
-- Dawn event: gossip start -> intro step machine (steps 0-9) ->
-- escort waypoints 0-1 -> 5-minute battle -> post-battle steps
-- 11-73 -> escort waypoints 2-8.
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
--   capture (FindNearestCreature, the GetClosestCreatureWithEntry
--   twin, lua_creature_events.go:7013); step 3
--   Talk(4); steps 4-7 ghoul/abomination/warrior/behemoth
--   summon loops (500ms re-fire, shack 33271 each tick);
--   step 8 Talk(5); step 9 escort resume + charge scatter.
-- - uiStep machine steps 11-73 (post-battle dialogue machine):
--   Tirion/Alexandros/ghost/Lich King talks (ids 27-67 + emotes
--   73-84), Alexandros + Darion-ghost summons, ghost JUST_DIED,
--   Lich King soul feasts, Darion charge (step 33) + kick (35),
--   Maxwell charge (step 42, one defender + Rimblat on the lich
--   king's live position), kicked heroes (43), stand-up (44-46),
--   Ashbringer rebirth (47, Tirion equips 13262 + 53702),
--   Light of Dawn GO summon (48) + 41542 burst + roar, Tirion
--   charge + LK Relocate (52), LK teleport visual + Kill (59-60),
--   Tirion farewell walk (61-69), Darion stand + Talk (70),
--   quest credit to players in 500yd + fresh Darion (71),
--   escort end (72), despawn sweep (73).
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
-- - Steps 34 and 36 have NO case in C++ (cases run 33 -> 35,
--   35 -> 37): the machine stalls there until the resumed escort
--   reaches waypoint 6/7, whose JumpToNextStep advances it. The
--   Lua 1s poll stands in for the per-tick no-op re-fire.
-- - HandleEmoteCommand has no bridge: oneshot kick/roar ride
--   PerformEmote (wp6 precedent); EMOTESTATE writes ride
--   EmoteState (SetUInt32Value is not on the creature surface);
--   KillSelf rides Kill(); the virtual-item equip rides
--   SetEquipmentSlots. SummonGameObject / Relocate / SetSpeedRate
--   (MOVE_RUN=1 only) / SetDeathState / DespawnOrUnsummon are
--   bridged and verified in lua_creature_events.go.
-- - Step 42 overwrites st.defender[1]/st.earthshatter[1] with the
--   10s charge summons, C++-exact (uiDefenderGUID[0] /
--   uiEarthshatterGUID[0] are reused); the SpawnNPC defender
--   previously in slot 1 is orphaned in both versions.
-- - MoveInLineOfSight 20yd + LOS aggro leg stays out (ev27 is
--   engage-only, no LOS-gated bridge); engine aggro covers the
--   approach. PrepareQuestMenu has no bridge (gossip hello
--   omits it). Chapter5 is fully ported: steps 0-73 + waypoints
--   0-8 + battle + gossip + Reset.
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
local SPELL_ALEXANDROS_SPAWN = 53667
local SPELL_SOUL_FEAST_ALEX = 53677
local SPELL_SOUL_FEAST_TIRION = 53685
local SPELL_REBIRTH_ASHBRINGER = 53702
local SPELL_TIRION_CHARGE = 53705
local SPELL_TELEPORT_VISUAL = 52233
local SPELL_LIGHT_BURST = 41542 -- workaround light explosion, chapter5.cpp:1107
local GO_LIGHT_OF_DAWN = 191330
local EQUIP_TIRION_ASHBRINGER = 13262
local STAND_DEAD = 7
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local DEATH_JUST_DIED = 1
local MOVE_RUN = 1
local EMOTE_KICK = 60 -- EMOTE_ONESHOT_KICK
local EMOTE_ROAR = 15 -- EMOTE_ONESHOT_ROAR
local EMOTE_ATTACK_UNARMED = 438 -- EMOTE_STATE_ATTACK_UNARMED

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
local SUMMON_TYPE_CORPSE = 1 -- Go 0-based TEMPSUMMON_TIMED_OR_CORPSE_DESPAWN (spells.go:536-539; bridge type 1, hazzarah/kiljaeden/ragnaros precedent)

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

-- GetClosestCreatureWithEntry twin (lua_creature_events.go:7013):
-- nearest creature of entry within range, alive by default.
local function nearestEntry(creature, entry, range)
    return creature:FindNearestCreature(entry, range)
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
    -- C++ SelectTarget(Random, 0): a random entry off Darion's threat
    -- list — no range cap, no player filter (GetAITargets, moroes
    -- precedent; the dawn summons sit on the list at 0 threat).
    local targets = creature:GetAITargets()
    if targets == nil or #targets == 0 then
        return
    end
    local target = targets[math.random(#targets)]
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

-- chapter5.cpp:584-1310 uiStep machine, steps 0-73 (+ step 10 no-op).
stepTick = function(creature, guid)
    local st = getState(guid)
    if st.isBattle or st.step >= 74 then
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
    elseif step == 11 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(27) -- SAY_LIGHT_OF_DAWN28
        end
        jumpToNextStep(creature, guid, 21000)
    elseif step == 12 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(28) -- SAY_LIGHT_OF_DAWN29
        end
        jumpToNextStep(creature, guid, 13000)
    elseif step == 13 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(29) -- SAY_LIGHT_OF_DAWN30
        end
        jumpToNextStep(creature, guid, 13000)
    elseif step == 14 then
        creature:SetStandState(STAND_STAND)
        creature:Talk(30) -- SAY_LIGHT_OF_DAWN31
        jumpToNextStep(creature, guid, 7000)
    elseif step == 15 then
        -- Summon gate: Alexandros at Loc[22].
        local loc = LOC[23]
        local temp = creature:SummonCreature(ENTRY_ALEXANDROS,
            loc[1], loc[2], loc[3], loc[4], SUMMON_TYPE_CORPSE, 300000)
        if temp then
            temp:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            temp:CastSpell(temp, SPELL_ALEXANDROS_SPAWN, true)
            temp:Talk(73) -- EMOTE_LIGHT_OF_DAWN06
            st.alexandros = storeGuid(temp)
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 16 then
        local temp = getc(creature, st.alexandros)
        if temp then
            temp:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            local loc = LOC[24]
            temp:MoveTo(0, loc[1], loc[2], loc[3])
            temp:Talk(31) -- SAY_LIGHT_OF_DAWN32
        end
        creature:SetEscortPaused(false)
        jumpToNextStep(creature, guid, 5000)
    elseif step == 17 then
        creature:SetStandState(STAND_KNEEL)
        creature:Talk(33) -- SAY_LIGHT_OF_DAWN34
        jumpToNextStep(creature, guid, 5000)
    elseif step == 18 then
        -- Darion's spirit out at Loc[24].
        local loc = LOC[25]
        local temp = creature:SummonCreature(ENTRY_DARION_GHOST,
            loc[1], loc[2], loc[3], loc[4], SUMMON_TYPE_CORPSE, 300000)
        if temp then
            temp:Talk(34) -- SAY_LIGHT_OF_DAWN35
            temp:SetWalk(false)
            st.darionGhost = storeGuid(temp)
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 19 then
        local temp = getc(creature, st.darionGhost)
        if temp then
            temp:Talk(74) -- EMOTE_LIGHT_OF_DAWN07
            local loc = LOC[26]
            temp:MoveTo(0, loc[1], loc[2], loc[3])
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 20 then
        local temp = getc(creature, st.darionGhost)
        if temp then
            temp:Talk(35) -- SAY_LIGHT_OF_DAWN36
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 21 then
        local temp = getc(creature, st.darionGhost)
        if temp then
            temp:Talk(75) -- EMOTE_LIGHT_OF_DAWN08
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 22 then
        local temp = getc(creature, st.alexandros)
        if temp then
            temp:Talk(36) -- SAY_LIGHT_OF_DAWN37
        end
        jumpToNextStep(creature, guid, 8000)
    elseif step == 23 then
        local temp = getc(creature, st.darionGhost)
        if temp then
            temp:Talk(37) -- SAY_LIGHT_OF_DAWN38
        end
        jumpToNextStep(creature, guid, 8000)
    elseif step == 24 then
        local temp = getc(creature, st.alexandros)
        if temp then
            temp:Talk(38) -- SAY_LIGHT_OF_DAWN39
        end
        local tirion = getc(creature, st.tirion)
        if tirion then
            local loc = LOC[2]
            tirion:MoveTo(0, loc[1], loc[2], loc[3])
        end
        jumpToNextStep(creature, guid, 15000)
    elseif step == 25 then
        local temp = getc(creature, st.darionGhost)
        if temp then
            temp:Talk(39) -- SAY_LIGHT_OF_DAWN40
        end
        jumpToNextStep(creature, guid, 11000)
    elseif step == 26 then
        local temp = getc(creature, st.alexandros)
        if temp then
            temp:Talk(40) -- SAY_LIGHT_OF_DAWN41
        end
        jumpToNextStep(creature, guid, 5000)
    elseif step == 27 then
        local temp = getc(creature, st.darionGhost)
        if temp then
            temp:SetDeathState(DEATH_JUST_DIED)
        end
        jumpToNextStep(creature, guid, 24000)
    elseif step == 28 then
        local temp = getc(creature, st.alexandros)
        if temp then
            temp:Talk(41) -- SAY_LIGHT_OF_DAWN42
        end
        jumpToNextStep(creature, guid, 6000)
    elseif step == 29 then
        -- Lich king spawns at Loc[26].
        local loc = LOC[27]
        local temp = creature:SummonCreature(ENTRY_LICH_KING,
            loc[1], loc[2], loc[3], loc[4], SUMMON_TYPE_CORPSE, 300000)
        if temp then
            temp:Talk(42) -- SAY_LIGHT_OF_DAWN43
            st.lichKing = storeGuid(temp)
            local alex = getc(creature, st.alexandros)
            if alex then
                temp:CastSpell(alex, SPELL_SOUL_FEAST_ALEX, false)
            end
        end
        jumpToNextStep(creature, guid, 2000)
    elseif step == 30 then
        local alex = getc(creature, st.alexandros)
        if alex then
            alex:Talk(76) -- EMOTE_LIGHT_OF_DAWN09
            alex:SetVisible(false)
        end
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:InterruptNonMeleeSpells(false)
            lk:Talk(44) -- SAY_LIGHT_OF_DAWN45
        end
        jumpToNextStep(creature, guid, 3000)
    elseif step == 31 then
        creature:SetStandState(STAND_STAND)
        creature:Talk(77) -- EMOTE_LIGHT_OF_DAWN10
        creature:Talk(43) -- SAY_LIGHT_OF_DAWN44
        jumpToNextStep(creature, guid, 3000)
    elseif step == 32 then
        local lk = getc(creature, st.lichKing)
        if lk then
            local loc = LOC[28]
            lk:MoveTo(0, loc[1], loc[2], loc[3])
        end
        jumpToNextStep(creature, guid, 6000)
    elseif step == 33 then
        -- Darion charges the lich king; the escort resumes and
        -- waypoint 6 advances the machine past the step-34 hole.
        local lk = getc(creature, st.lichKing)
        if lk then
            creature:CastSpell(creature, SPELL_MOGRAINE_CHARGE)
        end
        creature:SetSpeedRate(MOVE_RUN, 3.0)
        creature:SetWalk(false)
        creature:SetEscortPaused(false)
        jumpToNextStep(creature, guid, 0)
    elseif step == 34 or step == 36 then
        -- No case in C++ (cases run 33 -> 35, 35 -> 37): the 1s
        -- poll stands in for the per-tick no-op re-fire; waypoint
        -- 6/7 JumpToNextStep advances past the hole.
        schedule(guid, "phase", 1000, function()
            stepTick(creature, guid)
        end)
    elseif step == 35 then
        -- Lich king counterattacks: Darion kicked, escort resumes
        -- and waypoint 7 advances past the step-36 hole.
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:PerformEmote(EMOTE_KICK)
            lk:Talk(45) -- SAY_LIGHT_OF_DAWN46
        end
        creature:SetSpeedRate(MOVE_RUN, 6.0)
        creature:SetStandState(STAND_DEAD)
        creature:SetEscortPaused(false)
        jumpToNextStep(creature, guid, 0)
    elseif step == 37 then
        creature:SetStandState(STAND_KNEEL)
        jumpToNextStep(creature, guid, 3000)
    elseif step == 38 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(46) -- SAY_LIGHT_OF_DAWN47
        end
        jumpToNextStep(creature, guid, 8000)
    elseif step == 39 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(47) -- SAY_LIGHT_OF_DAWN48
        end
        jumpToNextStep(creature, guid, 15000)
    elseif step == 40 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(48) -- SAY_LIGHT_OF_DAWN49
        end
        jumpToNextStep(creature, guid, 17000)
    elseif step == 41 then
        -- Lich king - Apocalypse: soul feast on Tirion.
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(78) -- EMOTE_LIGHT_OF_DAWN11
            lk:Talk(50) -- SAY_LIGHT_OF_DAWN51
            local tirion = getc(creature, st.tirion)
            if tirion then
                tirion:SetStandState(STAND_KNEEL)
                lk:CastSpell(tirion, SPELL_SOUL_FEAST_TIRION, false)
                tirion:Talk(79) -- EMOTE_LIGHT_OF_DAWN12
            end
        end
        jumpToNextStep(creature, guid, 2000)
    elseif step == 42 then
        -- Maxwell yells for attack: one defender + Rimblat charge
        -- the lich king's position; the dawn heroes follow.
        local lk = getc(creature, st.lichKing)
        local lx, ly, lz = 0, 0, 0
        if lk then
            lx, ly, lz = lk:GetX(), lk:GetY(), lk:GetZ()
        end
        if lx ~= 0 and ly ~= 0 then
            local loc0 = LOC[1]
            local function chargeSummon(entry, key)
                local temp = creature:SummonCreature(entry,
                    loc0[1] + math.random(0, 9),
                    loc0[2] + math.random(0, 9), loc0[3],
                    0, SUMMON_TYPE_CORPSE, 10000)
                if temp then
                    temp:EmoteState(EMOTE_ATTACK_UNARMED)
                    temp:SetWalk(false)
                    temp:SetSpeedRate(MOVE_RUN, 2.0)
                    temp:SetFaction(creature:GetFaction())
                    temp:MoveTo(0, lx, ly, lz)
                    st[key][1] = storeGuid(temp)
                end
            end
            chargeSummon(ENTRY_DEFENDER, "defender")
            chargeSummon(ENTRY_RIMBLAT, "earthshatter")
        end
        local function chargeHero(guidKey, talk)
            local temp = getc(creature, guidKey)
            if not temp then
                return
            end
            temp:EmoteState(EMOTE_ATTACK_UNARMED)
            temp:SetWalk(false)
            temp:SetSpeedRate(MOVE_RUN, 2.0)
            temp:MoveTo(0, lx, ly, lz)
            if talk then
                temp:Talk(talk)
            end
        end
        chargeHero(st.maxwell, 49) -- SAY_LIGHT_OF_DAWN50
        chargeHero(st.korfax, nil)
        chargeHero(st.eligor, nil)
        jumpToNextStep(creature, guid, 4500)
    elseif step == 43 then
        -- They all got kicked.
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(80) -- EMOTE_LIGHT_OF_DAWN13
        end
        local function kicked(guidKey, locIndex)
            local temp = getc(creature, guidKey)
            if not temp then
                return
            end
            temp:EmoteState(0) -- EMOTE_ONESHOT_NONE
            temp:SetSpeedRate(MOVE_RUN, 6.0)
            temp:SetStandState(STAND_DEAD)
            local loc = LOC[locIndex]
            temp:MoveTo(0, loc[1], loc[2], loc[3])
        end
        kicked(st.maxwell, 15) -- Loc[14]
        kicked(st.korfax, 12) -- Loc[11]
        kicked(st.eligor, 18) -- Loc[17]
        local function kickedWave(guidKey)
            local temp = getc(creature, guidKey)
            if not temp then
                return
            end
            temp:SetSpeedRate(MOVE_RUN, 6.0)
            temp:SetStandState(STAND_DEAD)
            local loc0 = LOC[1]
            temp:MoveTo(0, loc0[1] + math.random(0, 9),
                loc0[2] + math.random(0, 9), loc0[3])
        end
        kickedWave(st.defender[1])
        kickedWave(st.earthshatter[1])
        jumpToNextStep(creature, guid, 3000)
    elseif step == 44 then
        -- Make them stand up.
        local function stand(guidKey)
            local temp = getc(creature, guidKey)
            if temp then
                temp:SetStandState(STAND_STAND)
            end
        end
        stand(st.maxwell)
        stand(st.korfax)
        stand(st.eligor)
        jumpToNextStep(creature, guid, 1000)
    elseif step == 45 then
        creature:Talk(51) -- SAY_LIGHT_OF_DAWN52
        jumpToNextStep(creature, guid, 5000)
    elseif step == 46 then
        -- Darion stands up, "not today"; throws sword.
        creature:SetSpeedRate(MOVE_RUN, 1.0)
        creature:SetWalk(true)
        creature:SetStandState(STAND_STAND)
        creature:Talk(52) -- SAY_LIGHT_OF_DAWN53
        creature:SetEscortPaused(false)
        jumpToNextStep(creature, guid, 7000)
    elseif step == 47 then
        -- Ashbringer rebirth.
        creature:SetStandState(STAND_KNEEL)
        creature:Talk(82) -- EMOTE_LIGHT_OF_DAWN15
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:SetStandState(STAND_STAND)
            tirion:SetEquipmentSlots(EQUIP_TIRION_ASHBRINGER, 0xFFFFFFFF, 0xFFFFFFFF)
            tirion:CastSpell(tirion, SPELL_REBIRTH_ASHBRINGER, false)
        end
        jumpToNextStep(creature, guid, 1000)
    elseif step == 48 then
        -- Show the cleansing effect (dawn of light).
        creature:SummonGameObject(GO_LIGHT_OF_DAWN,
            2283.896, -5287.914, 83.066, 0, 30)
        local tirion = getc(creature, st.tirion)
        if tirion then
            if tirion:HasAura(SPELL_REBIRTH_ASHBRINGER) then
                tirion:RemoveAura(SPELL_REBIRTH_ASHBRINGER)
            end
            tirion:CastSpell(tirion, SPELL_LIGHT_BURST, false)
            tirion:PerformEmote(EMOTE_ROAR)
        end
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:InterruptNonMeleeSpells(false)
        end
        jumpToNextStep(creature, guid, 2500)
    elseif step == 49 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(53) -- SAY_LIGHT_OF_DAWN54
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 50 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(54) -- SAY_LIGHT_OF_DAWN55
        end
        jumpToNextStep(creature, guid, 5000)
    elseif step == 51 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(55) -- SAY_LIGHT_OF_DAWN56
        end
        jumpToNextStep(creature, guid, 1000)
    elseif step == 52 then
        -- Tirion charges; the lich king is relocated (kicked back).
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(83) -- EMOTE_LIGHT_OF_DAWN16
            tirion:CastSpell(tirion, SPELL_TIRION_CHARGE, false)
            tirion:EmoteState(EMOTE_READY_2H)
            tirion:SetSpeedRate(MOVE_RUN, 3.0)
            tirion:SetWalk(false)
            local loc = LOC[3]
            tirion:MoveTo(0, loc[1], loc[2], loc[3])
            local lk = getc(creature, st.lichKing)
            if lk then
                local kloc = LOC[29]
                lk:Relocate(kloc[1], kloc[2], kloc[3], kloc[4])
            end
        end
        jumpToNextStep(creature, guid, 1500)
    elseif step == 53 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(56) -- SAY_LIGHT_OF_DAWN57
        end
        jumpToNextStep(creature, guid, 1000)
    elseif step == 54 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:SetSpeedRate(MOVE_RUN, 1.0)
            creature:SetWalk(true)
            local loc = LOC[30]
            lk:MoveTo(0, loc[1], loc[2], loc[3])
        end
        jumpToNextStep(creature, guid, 4000)
    elseif step == 55 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:SetStandState(STAND_KNEEL)
        end
        jumpToNextStep(creature, guid, 2000)
    elseif step == 56 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:SetStandState(STAND_STAND)
        end
        jumpToNextStep(creature, guid, 1500)
    elseif step == 57 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(57) -- SAY_LIGHT_OF_DAWN58
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 58 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(58) -- SAY_LIGHT_OF_DAWN59
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 59 then
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:CastSpell(lk, SPELL_TELEPORT_VISUAL, false)
        end
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:EmoteState(0) -- EMOTE_ONESHOT_NONE
            tirion:SetSpeedRate(MOVE_RUN, 1.0)
            local loc = LOC[7]
            tirion:MoveTo(0, loc[1], loc[2], loc[3])
        end
        jumpToNextStep(creature, guid, 2500)
    elseif step == 60 then
        -- Lich king disappears here.
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:Talk(84) -- EMOTE_LIGHT_OF_DAWN17
            lk:Kill()
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 61 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(59) -- SAY_LIGHT_OF_DAWN60
        end
        jumpToNextStep(creature, guid, 3000)
    elseif step == 62 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:SetWalk(true)
            local loc = LOC[8]
            tirion:MoveTo(0, loc[1], loc[2], loc[3])
        end
        jumpToNextStep(creature, guid, 5500)
    elseif step == 63 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            local loc = LOC[9]
            tirion:MoveTo(0, loc[1], loc[2], loc[3])
            tirion:Talk(60) -- SAY_LIGHT_OF_DAWN61
        end
        jumpToNextStep(creature, guid, 15000)
    elseif step == 64 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(61) -- SAY_LIGHT_OF_DAWN62
        end
        jumpToNextStep(creature, guid, 7000)
    elseif step == 65 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(62) -- SAY_LIGHT_OF_DAWN63
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 66 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(63) -- SAY_LIGHT_OF_DAWN64
        end
        jumpToNextStep(creature, guid, 11000)
    elseif step == 67 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(64) -- SAY_LIGHT_OF_DAWN65
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 68 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(65) -- SAY_LIGHT_OF_DAWN66
        end
        jumpToNextStep(creature, guid, 8000)
    elseif step == 69 then
        local tirion = getc(creature, st.tirion)
        if tirion then
            tirion:Talk(66) -- SAY_LIGHT_OF_DAWN67
        end
        jumpToNextStep(creature, guid, 10000)
    elseif step == 70 then
        creature:SetStandState(STAND_STAND)
        creature:Talk(67) -- SAY_LIGHT_OF_DAWN68
        jumpToNextStep(creature, guid, 10000)
    elseif step == 71 then
        -- Quest credit for players within 500yd, then a fresh
        -- Darion spawns for the quest turn-in.
        local players = creature:GetPlayersInRange(500)
        if players then
            for _, player in ipairs(players) do
                player:CastSpell(player, SPELL_THE_LIGHT_OF_DAWN_Q)
            end
        end
        creature:SetVisible(false)
        creature:SummonCreature(ENTRY_DARION,
            creature:GetX(), creature:GetY(), creature:GetZ(),
            0, SUMMON_TYPE_CORPSE, 180000)
        jumpToNextStep(creature, guid, 1000)
    elseif step == 72 then
        -- Escort ends.
        creature:SetEscortPaused(false)
        jumpToNextStep(creature, guid, 25000)
    elseif step == 73 then
        local koltira = getc(creature, st.koltira)
        if koltira then
            koltira:DespawnOrUnsummon()
        end
        local orbaz = getc(creature, st.orbaz)
        if orbaz then
            orbaz:DespawnOrUnsummon()
        end
        local thassarian = getc(creature, st.thassarian)
        if thassarian then
            thassarian:DespawnOrUnsummon()
        end
        local lk = getc(creature, st.lichKing)
        if lk then
            lk:DespawnOrUnsummon()
        end
        creature:DespawnOrUnsummon()
        -- No JumpToNextStep: C++ leaves the timer unset, so step 73
        -- would re-fire as a no-op; the despawn ends the machine.
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
