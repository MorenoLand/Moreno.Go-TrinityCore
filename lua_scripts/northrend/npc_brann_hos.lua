-- Halls of Stone: npc_brann_hos — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfStone/halls_of_stone.cpp
-- (npc_brann_hos : public EscortAI).
-- Entry: NPC_BRANN_BRONZEBEARD 28070 (halls_of_stone.cpp:93).
-- Ported: OnGossipHello (halls_of_stone.cpp:703-710) -> event 1:
-- "Brann, it would be our honor!" (GOSSIP_ICON_CHAT, sender 1,
-- action 1001) + menu 13100 (TEXT_ID_START). The IsQuestGiver /
-- PrepareQuestMenu arm is documented-only (no quest-menu bridge —
-- kalecgos precedent).
-- Ported: OnGossipSelect (halls_of_stone.cpp:686-696) -> event 2:
-- actions 1001/1002 close the menu and call StartWP()
-- (RemoveFlag gossip + StartEscort(player) + SetEscortPaused(false)
-- + MoveWaypoint() + step 1; the old-hillsbrad Thrall precedent).
-- Ported: UpdateAI low-HP arm (halls_of_stone.cpp:674-679) — Talk(SAY_LOW_HEALTH
-- 1) once when health drops strictly below 30%, latch re-armed when healed back
-- to >= 30%; rides the pre-damage event-9 hook with the nefarian postDamage > 0
-- guard and the maulgar/ironaya floored-integer threshold.
-- Ported: DamageTaken sparklin-news latch (halls_of_stone.cpp:405-408) —
-- if the tribunal is IN_PROGRESS (GetBossState(DATA_TRIBUNAL_OF_AGES 2) ==
-- 1), any damage taken clears the latch (SetData(DATA_BRANN_SPARKLIN_NEWS
-- 1, 0)); the latch is initialized true on spawn (ev5, the ctor Initialize
-- :289) and read by achievement_brann_spankin_new (ported in
-- engine/world/achievement_criteria_scripts.go, the abuse_the_ooze
-- precedent). Reset (halls_of_stone.cpp:304-313) re-inits the latch and
-- sets the tribunal NOT_STARTED when not escorting — PORTED (tick 515)
-- on ev23 (IsEscorted bridged lua_creature_events.go:2380); the
-- DespawnDwarf leg is PORTED this tick (dwarf objects latched in ev19).
-- Ported: the full escort event machine (halls_of_stone.cpp:428-669,
-- UpdateEscortAI phase steps 1-50 + WaypointReached :316-345) — the
-- step chain rides CreateLuaEvent timers (the controller precedent);
-- steps 2/4 have no C++ case and idle until the waypoint arrivals
-- advance them (WP13 -> step 3, WP17 -> step 5); step 1 re-checks
-- every 1s until the tribunal is NOT_STARTED (C++ returns without
-- advancing). WaypointReached is ev6 via GetCurrentWaypointId (the
-- old-hillsbrad precedent); the path itself loads from the live world
-- DB (creature_template_addon.path_id -> waypoint_data, engine
-- loadCreaturePathID / loadWaypoints, creaturemotion.go:4842/4854) —
-- neither repo holds the rows, so the C++ node ids 7/13/17/18 are used
-- verbatim. SpawnDwarf (:347-370, type 5 CORPSE_TIMED_DESPAWN 30s at
-- 946.992/397.016/208.374, DUNGEON_MODE 2/3 protectors via IsHeroic).
-- JustSummoned (:372-377) -> ev19: dwarf latched, summoned:AddThreat
-- (Brann, 0) (C++ AddThreat(me, 0, summoned) adds Brann to the
-- summoned's threat list, ScriptedCreature.cpp:202-209) +
-- summoned:AttackStart(Brann). Steps 8/12/18 drive the controller via
-- controller:DoAction(1/2/3) (the tick-515 ev38 protocol). Step 28
-- kills the controller via controller:Kill() (the KillSelf twin,
-- lua_creature_events.go:3155; fires the controller ev4, which cancels
-- its loops). Step 29 writes SetBossState(DATA_TRIBUNAL_OF_AGES,
-- DONE) + self-cast SPELL_REWARD_ACHIEVEMENT 59046 triggered. Step 49
-- runs player:GroupEventHappens(QUEST_HALLS_OF_STONE 13207) via
-- GetEscortPlayer + re-sets the gossip flag. Face Talks ride
-- FindNearestCreature(entry, 100) (lua_creature_events.go:6087) —
-- the faces have no Lua files, so no SetGuidData writer exists for
-- DATA_MARNAK/KADDRAK/ABEDNEUM and GetInstanceCreature would read
-- nil; the 100yd search is the live route (WP7 uses the same range
-- for the controller in C++).
-- npc_tribuna_controller (halls_of_stone.cpp:134-267) — PORTED (tick 515) in
-- lua_scripts/northrend/npc_tribuna_controller.lua; its Marnak/Abedneum
-- loops activate via the ev38 protocol (actions 2/3) driven by Brann's
-- escort steps 12/18; the Kaddrak glare arm is dead in C++
-- (KaddrakGUIDList never populated — UpdateFacesList commented out),
-- deliberately omitted. UpdateFacesList is commented out in C++.
-- Documented-only (no bridge, not fabricated):
-- - HandleGameObject legs (steps 8/12/17/18/28/49: face/sky-floor/
--   console GOs): no HandleGameObject bridge; the instance file
--   latches no GO GuidData (event 6 is dynamic-only, so the static
--   face/sky-floor/console GOs never land in the store) — the
--   controller-Reset precedent. The tribunal-chest RemoveFlag leg
--   (instance_halls_of_stone.cpp:134-155, DONE-time) is instance-side
--   and stays blocked on the same GO-latch gap even now that the
--   escort machine writes DONE.
-- - SAY_KILL 0 / SAY_DEATH 2 / SAY_PLAYER_DEATH 3 / SAY_VICTORY_SJONNIR
--   30-31 / SAY_ENTRANCE_MEET 32 / SAY_SPAWN_DWARF/TROGG/OOZE/EARTHEN
--   5-8: declared but never used — no JustDied/KilledUnit/PlayerDeath
--   overrides and no SpawnDwarf Talk in halls_of_stone.cpp.
-- - bIsBattle: write-only in C++ (:285/:428/:559, never read).
-- - The escort resumes at step 50 with SetEscortPaused(false); there
--   is no waypoint-end stop (the Go path loops — the StopEscort halt
--   is not applied here because C++ never ends the escort either).

local ENTRY_BRANN_HOS = 28070

local NPC_DARK_RUNE_PROTECTOR = 27983
local NPC_DARK_RUNE_STORMCALLER = 27984
local NPC_IRON_GOLEM_CUSTODIAN = 27985
local NPC_TRIBUNA_CONTROLLER = 28234
local NPC_MARNAK = 30897
local NPC_KADDRAK = 30898
local NPC_ABEDNEUM = 30899

local GOSSIP_ICON_CHAT = 0
local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_ITEM_START = "Brann, it would be our honor!"
local TEXT_ID_START = 13100

local SAY_LOW_HEALTH = 1
local SAY_ESCORT_START = 4
local SAY_EVENT_INTRO_1 = 9
local SAY_EVENT_INTRO_2 = 10
local SAY_EVENT_A_1 = 11
local SAY_EVENT_A_3 = 12
local SAY_EVENT_B_1 = 13
local SAY_EVENT_B_3 = 14
local SAY_EVENT_C_1 = 15
local SAY_EVENT_C_3 = 16
local SAY_EVENT_D_1 = 17
local SAY_EVENT_D_3 = 18
local SAY_EVENT_END_01 = 19
local SAY_EVENT_END_02 = 20
local SAY_EVENT_END_04 = 21
local SAY_EVENT_END_06 = 22
local SAY_EVENT_END_08 = 23
local SAY_EVENT_END_10 = 24
local SAY_EVENT_END_12 = 25
local SAY_EVENT_END_14 = 26
local SAY_EVENT_END_16 = 27
local SAY_EVENT_END_18 = 28
local SAY_EVENT_END_20 = 29

-- Face Talk ids (halls_of_stone.cpp:77-90): Abedneum 0-7, Kaddrak
-- 0-3, Marnak 0-3.
local FACE_ABEDNEUM_INTRO_3 = 0
local FACE_ABEDNEUM_C_2 = 1
local FACE_ABEDNEUM_D_2 = 2
local FACE_ABEDNEUM_D_4 = 3
local FACE_ABEDNEUM_END_03 = 4
local FACE_ABEDNEUM_END_05 = 5
local FACE_ABEDNEUM_END_07 = 6
local FACE_ABEDNEUM_END_21 = 7
local FACE_KADDRAK_A_2 = 0
local FACE_KADDRAK_END_09 = 1
local FACE_KADDRAK_END_11 = 2
local FACE_KADDRAK_END_13 = 3
local FACE_MARNAK_B_2 = 0
local FACE_MARNAK_END_15 = 1
local FACE_MARNAK_END_17 = 2
local FACE_MARNAK_END_19 = 3

-- DATA_TRIBUNAL_OF_AGES = 2 / DATA_BRANN_SPARKLIN_NEWS = 1
-- (halls_of_stone.h:33 / halls_of_stone.cpp:121).
local DATA_TRIBUNAL_OF_AGES = 2
local DATA_BRANN_SPARKLIN_NEWS = 1

local QUEST_HALLS_OF_STONE = 13207
local SPELL_REWARD_ACHIEVEMENT = 59046

local UNIT_NPC_FLAGS = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local REACT_PASSIVE = 0
local REACT_DEFENSIVE = 1
local STAND_STATE_STAND = 0
local STAND_STATE_KNEEL = 8

-- Controller ev38 activation protocol (tick 515): 1 Kaddrak,
-- 2 Marnak, 3 Abedneum.
local ACTION_ACTIVATE_KADDRAK = 1
local ACTION_ACTIVATE_MARNAK = 2
local ACTION_ACTIVATE_ABEDNEUM = 3

-- SpawnDwarf summon: TEMPSUMMON_CORPSE_TIMED_DESPAWN = Go type 5
-- (creaturemotion.go:501-509), 30s; SpawnLocations[0]
-- (halls_of_stone.cpp:129-133).
local SUMMON_TYPE_CORPSE_TIMED_DESPAWN = 5
local SPAWN_X, SPAWN_Y, SPAWN_Z, SPAWN_O = 946.992, 397.016, 208.374, 0.0

local lowHpLatched = {}

-- CreateLuaEvent timer harness (GUIDLow-keyed timers table, cancel on
-- events 4/23) — the tribuna-controller precedent.
local timers = {}

-- Per-Brann escort state, keyed by GetGUIDLow() (never by GetGUID()
-- userdata — cross-handler lookups miss; the GUID-key fix class).
-- step: current phase step; controller: latched controller object
-- (WP7); dwarves: JustSummoned objects for the Reset despawn leg.
local escort = {}

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

-- C++ UpdateAI low-HP arm (halls_of_stone.cpp:674-679): !bIsLowHP &&
-- HealthBelowPct(30) -> Talk(SAY_LOW_HEALTH), latch; bIsLowHP &&
-- !HealthBelowPct(30) -> latch cleared (healed back above 30%). The arm
-- sits in UpdateAI (post-damage tick read, never fires on the killing
-- blow), so the nefarian convention applies: ported on the pre-damage
-- event-9 hook with a postDamage > 0 guard and the maulgar/ironaya
-- strict-< floored integer threshold. The same hook clears the latch
-- when post-damage health climbs back to >= the threshold (C++ re-arms
-- the yell every UpdateAI tick).
local function brannDamageTaken(_, creature, _, damage)
    -- C++ DamageTaken (halls_of_stone.cpp:405-408): while the tribunal
    -- is IN_PROGRESS, any damage taken clears the sparklin-news latch.
    -- No postDamage guard: C++ DamageTaken fires pre-damage, including
    -- on the killing blow, and the leg reads no damage value.
    if creature:GetBossState(DATA_TRIBUNAL_OF_AGES) == 1 then
        creature:SetData(DATA_BRANN_SPARKLIN_NEWS, 0)
    end
    local guid = creature:GetGUIDLow()
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = creature:GetHealth() - damage
    if postDamage > 0 and postDamage < math.floor(maxHealth * 30 / 100) then
        if not lowHpLatched[guid] then
            lowHpLatched[guid] = true
            creature:Talk(SAY_LOW_HEALTH)
        end
    elseif postDamage >= math.floor(maxHealth * 30 / 100) then
        lowHpLatched[guid] = nil
    end
end

-- C++ ctor Initialize (halls_of_stone.cpp:289): brannSparklinNews
-- starts true; the ev5 spawn leg is the ctor twin (respawn rebuilds
-- the AI, same as the sjonnir ev5 SetGuidData precedent).
local function brannSpawned(event, creature)
    creature:SetData(DATA_BRANN_SPARKLIN_NEWS, 1)
end

local function brannGossipHello(event, player, creature)
    player:GossipMenuAddItem(GOSSIP_ICON_CHAT, GOSSIP_ITEM_START,
        GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(TEXT_ID_START, creature)
end

-- C++ SpawnDwarf (halls_of_stone.cpp:347-370): type 1 = DUNGEON_MODE
-- (2, 3) protectors + 1 stormcaller; type 2 = 2 stormcallers;
-- type 3 = 1 iron golem custodian. All at SpawnLocations[0],
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN 30s.
local function spawnDwarf(creature, dtype)
    local function summon(entry)
        creature:SummonCreature(entry, SPAWN_X, SPAWN_Y, SPAWN_Z,
            SPAWN_O, SUMMON_TYPE_CORPSE_TIMED_DESPAWN, 30000)
    end
    if dtype == 1 then
        local count = 2
        if creature:IsHeroic() then
            count = 3
        end
        for _ = 1, count do
            summon(NPC_DARK_RUNE_PROTECTOR)
        end
        summon(NPC_DARK_RUNE_STORMCALLER)
    elseif dtype == 2 then
        summon(NPC_DARK_RUNE_STORMCALLER)
        summon(NPC_DARK_RUNE_STORMCALLER)
    elseif dtype == 3 then
        summon(NPC_IRON_GOLEM_CUSTODIAN)
    end
end

-- Face Talk leg (halls_of_stone.cpp:435-665): the C++ sites read the
-- face via instance GetGuidData; the Lua route is the nearest live
-- face in the tribunal room (see header — no SetGuidData writer).
local function talkFace(creature, entry, textId)
    local face = creature:FindNearestCreature(entry, 100)
    if face then
        face:Talk(textId)
    end
end

-- Controller object: latched at waypoint 7, re-fetched when missing
-- (the controller never moves, so the 100yd search is exact).
local function getController(creature, guid)
    local st = escort[guid]
    if st and st.controller then
        return st.controller
    end
    local ctrl = creature:FindNearestCreature(NPC_TRIBUNA_CONTROLLER, 100)
    if st then
        st.controller = ctrl
    end
    return ctrl
end

-- JumpToNextStep(timer) twin: step += 1, run the next step after
-- timer ms. Forward-declared (tharon_ja idiom): runStep calls it,
-- its closure calls runStep.
local advance

-- C++ UpdateEscortAI phase machine (halls_of_stone.cpp:428-669).
-- Each case runs the step's actions then advances with the C++
-- JumpToNextStep delay. Steps 2 and 4 have no C++ case: the chain
-- stops there until WaypointReached advances it.
local function runStep(creature, guid)
    local st = escort[guid]
    if not st then
        return
    end
    local step = st.step
    if step == 1 then
        -- C++ :429-435: when the tribunal is not NOT_STARTED, return
        -- without advancing (retried every UpdateAI tick); the Lua
        -- retry is 1s.
        if creature:GetBossState(DATA_TRIBUNAL_OF_AGES) ~= 0 then
            schedule(guid, "step", 1000, function()
                runStep(creature, guid)
            end)
            return
        end
        creature:Talk(SAY_ESCORT_START)
        creature:SetWalk(false)
        st.step = 2
    elseif step == 3 then
        creature:SetEscortPaused(false)
        st.step = 4
    elseif step == 5 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_INTRO_3)
        advance(creature, guid, 8500)
    elseif step == 6 then
        creature:Talk(SAY_EVENT_A_1)
        advance(creature, guid, 6500)
    elseif step == 7 then
        talkFace(creature, NPC_KADDRAK, FACE_KADDRAK_A_2)
        advance(creature, guid, 12500)
    elseif step == 8 then
        creature:Talk(SAY_EVENT_A_3)
        local ctrl = getController(creature, guid)
        if ctrl then
            ctrl:DoAction(ACTION_ACTIVATE_KADDRAK)
        end
        advance(creature, guid, 5000)
    elseif step == 9 then
        creature:SetReactState(REACT_PASSIVE)
        spawnDwarf(creature, 1)
        advance(creature, guid, 20000)
    elseif step == 10 then
        creature:Talk(SAY_EVENT_B_1)
        advance(creature, guid, 6000)
    elseif step == 11 then
        talkFace(creature, NPC_MARNAK, FACE_MARNAK_B_2)
        spawnDwarf(creature, 1)
        advance(creature, guid, 20000)
    elseif step == 12 then
        creature:Talk(SAY_EVENT_B_3)
        local ctrl = getController(creature, guid)
        if ctrl then
            ctrl:DoAction(ACTION_ACTIVATE_MARNAK)
        end
        advance(creature, guid, 10000)
    elseif step == 13 then
        spawnDwarf(creature, 1)
        advance(creature, guid, 10000)
    elseif step == 14 then
        spawnDwarf(creature, 2)
        advance(creature, guid, 20000)
    elseif step == 15 then
        creature:Talk(SAY_EVENT_C_1)
        spawnDwarf(creature, 1)
        advance(creature, guid, 10000)
    elseif step == 16 then
        spawnDwarf(creature, 2)
        advance(creature, guid, 20000)
    elseif step == 17 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_C_2)
        spawnDwarf(creature, 1)
        advance(creature, guid, 20000)
    elseif step == 18 then
        creature:Talk(SAY_EVENT_C_3)
        local ctrl = getController(creature, guid)
        if ctrl then
            ctrl:DoAction(ACTION_ACTIVATE_ABEDNEUM)
        end
        advance(creature, guid, 5000)
    elseif step == 19 then
        spawnDwarf(creature, 2)
        advance(creature, guid, 10000)
    elseif step == 20 then
        spawnDwarf(creature, 1)
        advance(creature, guid, 15000)
    elseif step == 21 then
        creature:Talk(SAY_EVENT_D_1)
        spawnDwarf(creature, 3)
        advance(creature, guid, 20000)
    elseif step == 22 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_D_2)
        spawnDwarf(creature, 1)
        advance(creature, guid, 5000)
    elseif step == 23 then
        spawnDwarf(creature, 2)
        advance(creature, guid, 15000)
    elseif step == 24 then
        creature:Talk(SAY_EVENT_D_3)
        spawnDwarf(creature, 3)
        advance(creature, guid, 5000)
    elseif step == 25 then
        spawnDwarf(creature, 1)
        advance(creature, guid, 5000)
    elseif step == 26 then
        spawnDwarf(creature, 2)
        advance(creature, guid, 10000)
    elseif step == 27 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_D_4)
        spawnDwarf(creature, 1)
        advance(creature, guid, 10000)
    elseif step == 28 then
        creature:SetReactState(REACT_DEFENSIVE)
        creature:Talk(SAY_EVENT_END_01)
        creature:SetStandState(STAND_STATE_STAND)
        local ctrl = getController(creature, guid)
        if ctrl then
            ctrl:Kill()
        end
        creature:SetEscortPaused(false)
        advance(creature, guid, 6500)
    elseif step == 29 then
        creature:Talk(SAY_EVENT_END_02)
        creature:SetBossState(DATA_TRIBUNAL_OF_AGES, 3)
        creature:CastSpell(creature, SPELL_REWARD_ACHIEVEMENT, true)
        advance(creature, guid, 5500)
    elseif step == 30 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_END_03)
        advance(creature, guid, 8500)
    elseif step == 31 then
        creature:Talk(SAY_EVENT_END_04)
        advance(creature, guid, 11500)
    elseif step == 32 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_END_05)
        advance(creature, guid, 11500)
    elseif step == 33 then
        creature:Talk(SAY_EVENT_END_06)
        advance(creature, guid, 4500)
    elseif step == 34 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_END_07)
        advance(creature, guid, 22500)
    elseif step == 35 then
        creature:Talk(SAY_EVENT_END_08)
        advance(creature, guid, 7500)
    elseif step == 36 then
        talkFace(creature, NPC_KADDRAK, FACE_KADDRAK_END_09)
        advance(creature, guid, 18500)
    elseif step == 37 then
        creature:Talk(SAY_EVENT_END_10)
        advance(creature, guid, 5500)
    elseif step == 38 then
        talkFace(creature, NPC_KADDRAK, FACE_KADDRAK_END_11)
        advance(creature, guid, 20500)
    elseif step == 39 then
        creature:Talk(SAY_EVENT_END_12)
        advance(creature, guid, 2500)
    elseif step == 40 then
        talkFace(creature, NPC_KADDRAK, FACE_KADDRAK_END_13)
        advance(creature, guid, 19500)
    elseif step == 41 then
        creature:Talk(SAY_EVENT_END_14)
        advance(creature, guid, 10500)
    elseif step == 42 then
        talkFace(creature, NPC_MARNAK, FACE_MARNAK_END_15)
        advance(creature, guid, 6500)
    elseif step == 43 then
        creature:Talk(SAY_EVENT_END_16)
        advance(creature, guid, 6500)
    elseif step == 44 then
        talkFace(creature, NPC_MARNAK, FACE_MARNAK_END_17)
        advance(creature, guid, 25500)
    elseif step == 45 then
        creature:Talk(SAY_EVENT_END_18)
        advance(creature, guid, 23500)
    elseif step == 46 then
        talkFace(creature, NPC_MARNAK, FACE_MARNAK_END_19)
        advance(creature, guid, 3500)
    elseif step == 47 then
        creature:Talk(SAY_EVENT_END_20)
        advance(creature, guid, 8500)
    elseif step == 48 then
        talkFace(creature, NPC_ABEDNEUM, FACE_ABEDNEUM_END_21)
        advance(creature, guid, 5500)
    elseif step == 49 then
        local player = creature:GetEscortPlayer()
        if player then
            player:GroupEventHappens(QUEST_HALLS_OF_STONE, creature)
        end
        creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
        advance(creature, guid, 180000)
    elseif step == 50 then
        creature:SetEscortPaused(false)
    end
end

advance = function(creature, guid, delay)
    local st = escort[guid]
    if not st then
        return
    end
    st.step = st.step + 1
    schedule(guid, "step", delay, function()
        runStep(creature, guid)
    end)
end

-- C++ WaypointReached (halls_of_stone.cpp:316-345): 7 latches the
-- controller (respawn when dead; UpdateFacesList is commented out in
-- C++), 13 opens the tribunal (IN_PROGRESS + pause + step 3 after
-- 20s), 17 kneels at the console (pause + step 5 after 8.5s),
-- 18 pauses. The waypoint id comes from GetCurrentWaypointId (the
-- old-hillsbrad precedent); the ev6 args carry motion type + arrival.
local function brannReachWP(_, creature)
    if not creature:IsEscorted() then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = escort[guid]
    if not st then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 7 then
        local ctrl = creature:FindNearestCreature(NPC_TRIBUNA_CONTROLLER, 100)
        if ctrl then
            if not ctrl:IsAlive() then
                ctrl:Respawn()
            end
            st.controller = ctrl
        end
    elseif wp == 13 then
        creature:Talk(SAY_EVENT_INTRO_1)
        creature:SetBossState(DATA_TRIBUNAL_OF_AGES, 1)
        creature:SetEscortPaused(true)
        st.step = 3
        schedule(guid, "step", 20000, function()
            runStep(creature, guid)
        end)
    elseif wp == 17 then
        creature:Talk(SAY_EVENT_INTRO_2)
        creature:SetStandState(STAND_STATE_KNEEL)
        creature:SetEscortPaused(true)
        st.step = 5
        schedule(guid, "step", 8500, function()
            runStep(creature, guid)
        end)
    elseif wp == 18 then
        creature:SetEscortPaused(true)
    end
end

-- C++ JustSummoned (halls_of_stone.cpp:372-377): latch the dwarf,
-- add Brann to its threat list at 0, order it to attack Brann.
local function brannJustSummoned(_, creature, summoned)
    local guid = creature:GetGUIDLow()
    local st = escort[guid]
    if st then
        st.dwarves[#st.dwarves + 1] = summoned
    end
    summoned:AddThreat(creature, 0.0)
    summoned:AttackStart(creature)
end

-- C++ StartWP (halls_of_stone.cpp:392-399): drop the gossip flag,
-- unpause, step 1, start the escort walk.
local function startEscort(player, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    escort[guid] = { step = 1, controller = nil, dwarves = {} }
    creature:RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
    creature:StartEscort(player)
    creature:SetEscortPaused(false)
    creature:MoveWaypoint()
    runStep(creature, guid)
end

local function brannGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + 1
        or action == GOSSIP_ACTION_INFO_DEF + 2 then
        player:GossipComplete()
        startEscort(player, creature)
    end
end

-- C++ UpdateAI low-HP arm (halls_of_stone.cpp:674-679): !bIsLowHP &&
-- HealthBelowPct(30) -> Talk(SAY_LOW_HEALTH), latch; bIsLowHP &&
-- !HealthBelowPct(30) -> latch cleared (healed back above 30%).
-- The heal re-arm rides the pre-heal event-11 hook (valithria
-- precedent): a heal that lifts health back to >= the floored 30%
-- threshold clears the latch, so a second drop below 30% re-yells.
local function brannHealReceived(_, creature, _, heal)
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if creature:GetHealth() + heal >= math.floor(maxHealth * 30 / 100) then
        lowHpLatched[creature:GetGUIDLow()] = nil
    end
end

-- C++ JustDied has no override (SAY_DEATH is declared but unused):
-- the ev4 leg only stops the step machine and drops the escort state
-- (the C++ AI dies with the creature; respawn rebuilds it), plus the
-- low-HP latch re-init the ctor performs on respawn.
local function brannDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    escort[guid] = nil
    lowHpLatched[guid] = nil
end

-- C++ Reset (halls_of_stone.cpp:304-313): while NOT escorting,
-- Initialize() (latch re-init) + DespawnDwarf() +
-- SetBossState(DATA_TRIBUNAL_OF_AGES, NOT_STARTED). The ev23 gate
-- mirrors the C++ HasEscortState check: ev23 pre-fires on death
-- while escorting, and the escort state must survive until ev4.
local function brannReset(event, creature)
    if not creature:IsEscorted() then
        local guid = creature:GetGUIDLow()
        cancelTimers(guid)
        local st = escort[guid]
        if st then
            for _, dwarf in ipairs(st.dwarves) do
                if dwarf and dwarf:IsAlive() then
                    dwarf:DespawnOrUnsummon()
                end
            end
        end
        escort[guid] = nil
        lowHpLatched[guid] = nil
        creature:SetBossState(DATA_TRIBUNAL_OF_AGES, 0)
    end
end

RegisterCreatureGossipEvent(ENTRY_BRANN_HOS, 1, brannGossipHello)
RegisterCreatureGossipEvent(ENTRY_BRANN_HOS, 2, brannGossipSelect)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 4, brannDied)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 5, brannSpawned)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 6, brannReachWP)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 9, brannDamageTaken)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 11, brannHealReceived)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 19, brannJustSummoned)
RegisterCreatureEvent(ENTRY_BRANN_HOS, 23, brannReset)
