-- Simon game (Blade's Edge Mountains) -- Lua port of
-- src/server/scripts/Outland/zone_blades_edge_mountains.cpp
-- (enums SimonGame :307-353, SimonEvents :354-367, SimonColors
-- :369-376; class npc_simon_bunny :378-819; class go_simon_cluster
-- :824-845; enum ApexisRelic :847-854; class go_apexis_relic
-- :856-893). The Apexis relic/monument starts the game, the bunny
-- plays a growing color sequence, and the four color clusters take
-- the player's answers; 10 rounds wins the reward buffs, 4 failed
-- presses (small node) ends the game, a wrong press on a large node
-- summons the Apexis Guardian.
-- Entries (C++ enum, verifiable from the C++ sources): bunny
-- NPC_SIMON_BUNNY 22923, guardian NPC_APEXIS_GUARDIAN 22275,
-- relic GO_APEXIS_RELIC 185890, monument GO_APEXIS_MONUMENT
-- 185944, aura GOs GO_AURA_BLUE 185872 / GREEN 185873 / RED
-- 185874 / YELLOW 185875. Cluster entries are DB-side (no C++
-- constant, no data rows in sql/sqlite/world.sql) — the bunny
-- discovers them at StartGame by scanning GOs in range for the
-- cluster displayIds (C++-exact, :522-583) and registers the
-- cluster gossip handlers for the discovered entries then.
-- Eluna creature events: 40 OnSetGUID (StartGame entry point),
-- 38 OnDoAction (internal actions + cluster-press transport, see
-- below), 15 OnSpellHitTarget (bad-press damage), 2/4/23/37
-- cleanup. GO gossip: 1 hello (clusters, registered dynamically),
-- 2 select (relic/monument).
-- Event machine: a 500ms CreateLuaEvent pump is the C++ UpdateAI
-- EventMap twin — every C++ delay (500ms/1s/1500ms/2s/3s) is a
-- multiple of 500ms, so all timers are quantized to the 500ms pump grid. Each pump tick runs
-- Update(diff) then executes the single earliest-due event
-- (ties: insertion order — EventMap is a multimap and
-- ExecuteEvent takes begin(); the Lua pump breaks ties by lowest
-- id, so the second event on a tie waits an extra 500ms pump
-- tick vs ~50ms in C++), mirroring ExecuteEvent's
-- one-event-per-UpdateAI. ScheduleEvent never cancels a pending
-- event of the same id (EventMap.h: "An existing event is not
-- canceled"; only RescheduleEvent cancels-then-schedules) — the
-- Lua table assignment overwrites instead. CancelEventGroup(0)
-- is a no-op in C++ (group 0 invalid, EventMap.cpp), so
-- PERIODIC_PLAYER_CHECK survives ROUND_FINISHED; the Lua
-- round-cancel preserves it explicitly. DoCast(self) arms are
-- creature:CastSpell(creature,
-- spell) — CastSpell(nil, spell) would no-op here (no victim,
-- creature never in combat); DoCast(player) arms pass the player.
-- Transports (both ends are Lua in this file, C++ semantics kept):
-- - Cluster press: C++ go_simon_cluster calls bunny->AI()->
--   SetData(clusterEntry, 0) (:488-511). Go creature:SetData only
--   writes a motion store and fires NO event
--   (lua_creature_events.go:792-811), so the cluster handler calls
--   bunny:DoAction(clusterEntry) instead; the ev38 handler treats
--   values 8/9/10 as the C++ DoAction actions and any other value
--   as a SetData type (cluster entry). Internal DoAction calls use
--   the local doAction directly, the C++ self-call twin.
-- - Cluster gossip registration is dynamic: entries are unknown
--   until StartGame discovers them, so StartGame registers
--   RegisterGameObjectGossipEvent(entry, 1, ...) per discovered
--   entry (guarded by a module-level set — re-registration would
--   double-fire). Runtime registration from inside a handler is
--   supported: triggerNUpdated holds the re-entrant runtime mutex
--   while dispatching and its merge step preserves hooks added
--   mid-dispatch (scripting/runtime.go:903-985).
-- - Player identity: the relic select passes player:GetGUID()
--   to bunny:SetGUID; the bunny resolves it with GetPlayerByGUID,
--   which compares full GUIDs (scripting/globals.go:187) — the
--   scanner-bunny precedent (player GUIDs are small character
--   counters, so the UInt64 userdata round-trip is exact).
-- - Relic menu: no hello handler is registered — the native
--   questgiver/goober arm opens the DB gossip menu
--   (gameobjects.go:910/935, useQuestGiverGameObject), the C++
--   PrepareGossipMenu + SendPreparedGossip twin (:866-871); the
--   select handler (:873-885) ignores sender/action exactly as
--   C++ ignores menuId/gossipListId.
-- Ported arms (C++ order):
-- - SetGUID (:513-521): SetCanFly(true), large = id != 0, latch
--   player GUID, StartGame.
-- - StartGame (:526-583): reset state, scale 2.0/1.0 via SetScale
--   (the SetObjectScale twin — SetObjectScale is not a Lua method
--   and nil-aborts StartGame mid-flight, fixed 2026-11-10), zCoord
--   8.0/2.75, searchDistance 13.0/5.0, cluster scan by displayId
--   (small 7369/7371/7373/7375, large 7364/7365/7366/7367),
--   ROUND_FINISHED 1s + PERIODIC_PLAYER_CHECK 2s, relic
--   NOT_SELECTABLE.
-- - UpdateAI (:409-461): PERIODIC_PLAYER_CHECK (CheckPlayer else
--   ResetNode, re-arm 2s), SETUP_PRE_GAME (SetUpPreGame, cancel
--   GAME_TICK, PLAY_SEQUENCE 1s), PLAY_SEQUENCE (PlayNextColor +
--   1500ms re-arm, or listening + START_PLAYER_LEVEL visual +
--   PrepareClusters + GAME_TICK 3s), GAME_TICK (AUDIBLE_GAME_TICK,
--   gameTicks > gameLevel -> TOO_LONG_TIME 500ms else GAME_TICK
--   3s, gameTicks++), RESET_CLUSTERS (PrepareClusters(true)),
--   TOO_LONG_TIME -> WRONG_SEQUENCE, ROUND_FINISHED ->
--   ROUND_FINISHED action.
-- - DoAction (:463-486): ROUND_FINISHED (listening=false,
--   START_AI_LEVEL visual, GiveRewardForLevel, cancel round
--   events (PERIODIC_PLAYER_CHECK survives), level
--   10 -> ResetNode else SETUP_PRE_GAME 1s), CORRECT_FULL_SEQUENCE
--   (gameLevel++, ROUND_FINISHED), WRONG_SEQUENCE (GivePunishment,
--   ROUND_FINISHED).
-- - SetData (:488-511): ignored unless listening; entry -> color
--   (RED/BLUE/GREEN/YELLOW check order), PlayColor, push to
--   playerSequence, RESET_CLUSTERS 500ms, CheckPlayerSequence.
-- - CheckPlayerSequence (:608-623): prefix compare; full match ->
--   CORRECT_FULL_SEQUENCE, mismatch -> WRONG_SEQUENCE; over-long
--   input does nothing (C++-exact fall-through).
-- - GenerateColorSequence (:629-638): gameLevel+1 uniform colors,
--   copied to playableSequence.
-- - PrepareClusters (:642-655): clusters NOT_SELECTABLE-cleared;
--   aura GOs removed unless clustersOnly.
-- - PlayNextColor (:661-665) / PlayColor (:668-692): visual spell
--   + direct sound per color (DoPlaySoundToSet ≡ PlayDirectSound,
--   ScriptedCreature.cpp:188-200 — NOT PlayDistanceSound, which
--   sends SMSG_PLAY_OBJECT_SOUND).
-- - SetUpPreGame (:699-733): clusters NOT_SELECTABLE (large:
--   break after the first); small nodes teleport the bunny to
--   each cluster and cast its pre-game spell on it; then teleport
--   to the relic at z + zCoordCorrection; GenerateColorSequence.
--   DELTA: C++ teleports with the cluster/relic orientation;
--   GO:GetO() exists (generic object fallback,
--   scripting/object.go:185) and is used.
-- - GiveRewardForLevel (:736-756): level 6 large -> punishment,
--   else buff 40310; level 8 -> 40311; level 10 -> 40312; cast
--   triggered on the player.
-- - GivePunishment (:764-785): large -> summon guardian 22275 at
--   bunny pos z - zCoordCorrection (TEMPSUMMON_TIMED_DESPAWN_OUT_
--   OF_COMBAT = type 3, 20s), EngageWithTarget, ResetNode; small
--   -> fails++, BAD_PRESS_TRIGGER 41241 triggered on the player,
--   fails >= 4 -> ResetNode.
-- - SpellHitTarget (:787-800): BAD_PRESS_TRIGGER hit -> player
--   casts BAD_PRESS_DAMAGE 40065 on itself, triggered, bp0 =
--   floor(fails * 0.33 * maxHealth) (C++ float32 truncation;
--   float64 here, at most 1 off on a rounding edge).
--   DBC caveat: ev15 fires only for spells known to the server
--   DBC (boss_ai.go:4080-4120); no Spell.dbc ships in this repo,
--   so whether 41241 is known is data-dependent, not code.
-- - CheckPlayer (:803-817): no player / dead -> false; too far
--   (>= 2 * searchDistance) -> GivePunishment + false.
-- - ResetNode (:586-601): DISABLE_NODE sound 11758, clusters
--   NOT_SELECTABLE, aura GOs removed, relic selectable again,
--   DespawnOrUnsummon(1000). DELTA: the pump is cancelled here
--   and state drops on ev37 (despawn) so the synchronous
--   ROUND_FINISHED continuation after a punishment still runs
--   its visual + reward on live state, C++-exact (C++ despawn
--   is also delayed 1s); a resetting guard makes the double
--   ResetNode on the large too-far path a no-op (C++ plays the
--   sound twice — inaudible difference).
-- - go_simon_cluster OnGossipHello (:833-840): nearest bunny
--   22923 within 12.0 gets the press (DoAction transport above),
--   AddUse, return true (handled, native goober arm suppressed).
--   The goober-spell cast (goober.spellId) stays out: DB-side
--   template data, unestablishable here.
-- - go_apexis_relic OnGossipSelect (:873-885): shard check
--   (ITEM_APEXIS_SHARD 32569, 35 large / 1 small) via
--   player:GetItemCount, take-reagents spell (41146 large /
--   41145 small) non-triggered on the player, summon bunny at
--   the relic position (type 7 = TEMPSUMMON_MANUAL_DESPAWN,
--   the C++ default), SetGUID(player GUID, large 1/0).
-- Stay-outs (documented-only): none in the bunny/cluster/relic
-- machine — every arm above is ported. The sibling units in the
-- same C++ file (npc_oscillating_frequency_scanner_master_bunny
-- is already ported in npc_oscillating_frequency_scanner_master_
-- bunny.lua; spell_oscillating_field is ported (RegisterSpellEvent
-- 37408 ev5, npc_oscillating_frequency_scanner_master_bunny.lua))
-- are not this unit.

local NPC_SIMON_BUNNY = 22923
local NPC_APEXIS_GUARDIAN = 22275

local GO_APEXIS_RELIC = 185890
local GO_APEXIS_MONUMENT = 185944
local GO_AURA_BLUE = 185872
local GO_AURA_GREEN = 185873
local GO_AURA_RED = 185874
local GO_AURA_YELLOW = 185875

local GO_BLUE_CLUSTER_DISPLAY = 7369
local GO_GREEN_CLUSTER_DISPLAY = 7371
local GO_RED_CLUSTER_DISPLAY = 7373
local GO_YELLOW_CLUSTER_DISPLAY = 7375
local GO_BLUE_CLUSTER_DISPLAY_LARGE = 7364
local GO_GREEN_CLUSTER_DISPLAY_LARGE = 7365
local GO_RED_CLUSTER_DISPLAY_LARGE = 7366
local GO_YELLOW_CLUSTER_DISPLAY_LARGE = 7367

local SPELL_PRE_GAME_BLUE = 40176
local SPELL_PRE_GAME_GREEN = 40177
local SPELL_PRE_GAME_RED = 40178
local SPELL_PRE_GAME_YELLOW = 40179
local SPELL_VISUAL_BLUE = 40244
local SPELL_VISUAL_GREEN = 40245
local SPELL_VISUAL_RED = 40246
local SPELL_VISUAL_YELLOW = 40247

local SOUND_BLUE = 11588
local SOUND_GREEN = 11589
local SOUND_RED = 11590
local SOUND_YELLOW = 11591
local SOUND_DISABLE_NODE = 11758

local SPELL_AUDIBLE_GAME_TICK = 40391
local SPELL_VISUAL_START_PLAYER_LEVEL = 40436
local SPELL_VISUAL_START_AI_LEVEL = 40387

local SPELL_BAD_PRESS_TRIGGER = 41241
local SPELL_BAD_PRESS_DAMAGE = 40065
local SPELL_REWARD_BUFF_1 = 40310
local SPELL_REWARD_BUFF_2 = 40311
local SPELL_REWARD_BUFF_3 = 40312

local SPELL_TAKE_REAGENTS_SOLO = 41145
local SPELL_TAKE_REAGENTS_GROUP = 41146
local ITEM_APEXIS_SHARD = 32569

local EVENT_SIMON_SETUP_PRE_GAME = 1
local EVENT_SIMON_PLAY_SEQUENCE = 2
local EVENT_SIMON_RESET_CLUSTERS = 3
local EVENT_SIMON_PERIODIC_PLAYER_CHECK = 4
local EVENT_SIMON_TOO_LONG_TIME = 5
local EVENT_SIMON_GAME_TICK = 6
local EVENT_SIMON_ROUND_FINISHED = 7

local ACTION_SIMON_CORRECT_FULL_SEQUENCE = 8
local ACTION_SIMON_WRONG_SEQUENCE = 9
local ACTION_SIMON_ROUND_FINISHED = 10

local SIMON_BLUE = 0
local SIMON_RED = 1
local SIMON_GREEN = 2
local SIMON_YELLOW = 3
local SIMON_MAX_COLORS = 4

local GAMEOBJECT_FLAGS = 9 -- UpdateFields.h: OBJECT_END 6 + 3
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641

local SIMON_VISUAL = {
    [SIMON_BLUE] = SPELL_VISUAL_BLUE,
    [SIMON_RED] = SPELL_VISUAL_RED,
    [SIMON_GREEN] = SPELL_VISUAL_GREEN,
    [SIMON_YELLOW] = SPELL_VISUAL_YELLOW,
}
local SIMON_SOUND = {
    [SIMON_BLUE] = SOUND_BLUE,
    [SIMON_RED] = SOUND_RED,
    [SIMON_GREEN] = SOUND_GREEN,
    [SIMON_YELLOW] = SOUND_YELLOW,
}
local SIMON_PRE_GAME = {
    [SIMON_BLUE] = SPELL_PRE_GAME_BLUE,
    [SIMON_RED] = SPELL_PRE_GAME_RED,
    [SIMON_GREEN] = SPELL_PRE_GAME_GREEN,
    [SIMON_YELLOW] = SPELL_PRE_GAME_YELLOW,
}
local SIMON_DISPLAY_SMALL = {
    [SIMON_BLUE] = GO_BLUE_CLUSTER_DISPLAY,
    [SIMON_RED] = GO_RED_CLUSTER_DISPLAY,
    [SIMON_GREEN] = GO_GREEN_CLUSTER_DISPLAY,
    [SIMON_YELLOW] = GO_YELLOW_CLUSTER_DISPLAY,
}
local SIMON_DISPLAY_LARGE = {
    [SIMON_BLUE] = GO_BLUE_CLUSTER_DISPLAY_LARGE,
    [SIMON_RED] = GO_RED_CLUSTER_DISPLAY_LARGE,
    [SIMON_GREEN] = GO_GREEN_CLUSTER_DISPLAY_LARGE,
    [SIMON_YELLOW] = GO_YELLOW_CLUSTER_DISPLAY_LARGE,
}

local bunnyState = {}
local registeredCluster = {}

-- Forward declaration: startGame registers this handler before its
-- definition below; plain assignment there (never `local function`,
-- which would shadow this slot with nil).
local onClusterGossip

local function cancelPump(guid, st)
    if st.pumpId then
        RemoveEventById(st.pumpId)
        st.pumpId = nil
    end
end

local function scheduleEvent(st, eventId, delayMs)
    st.events[eventId] = delayMs
end

local function cancelEvent(st, eventId)
    st.events[eventId] = nil
end

-- C++ CancelEventGroup(0) is a no-op (group 0 is invalid,
-- EventMap.cpp), so PERIODIC_PLAYER_CHECK survives every round;
-- the round-scoped events are cancelled, the periodic check is
-- not (its remaining delay is preserved, C++-exact).
local function cancelRoundEvents(st)
    local periodic = st.events[EVENT_SIMON_PERIODIC_PLAYER_CHECK]
    st.events = {}
    if periodic ~= nil then
        st.events[EVENT_SIMON_PERIODIC_PLAYER_CHECK] = periodic
    end
end

-- C++ DoCast(self) twin: explicit self target (see header).
local function playColor(creature, color)
    if color < SIMON_BLUE or color > SIMON_YELLOW then
        return
    end
    creature:CastSpell(creature, SIMON_VISUAL[color])
    creature:PlayDirectSound(SIMON_SOUND[color])
end

local function generateColorSequence(st)
    st.colorSequence = {}
    for _ = 0, st.gameLevel do
        st.colorSequence[#st.colorSequence + 1] = math.random(SIMON_BLUE, SIMON_YELLOW)
    end
    st.playableSequence = {}
    for _, color in ipairs(st.colorSequence) do
        st.playableSequence[#st.playableSequence + 1] = color
    end
end

local function playNextColor(creature, st)
    playColor(creature, st.playableSequence[1])
    table.remove(st.playableSequence, 1)
end

-- C++ PrepareClusters (:642-655).
local function prepareClusters(creature, st, clustersOnly)
    for color = SIMON_BLUE, SIMON_YELLOW do
        local entry = st.clusterIds[color + 1]
        if entry ~= 0 then
            local cluster = creature:FindNearestGameObject(entry, st.searchDistance)
            if cluster then
                cluster:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
            end
        end
    end

    if clustersOnly then
        return
    end

    for auraId = GO_AURA_BLUE, GO_AURA_YELLOW do
        local auraGo = creature:FindNearestGameObject(auraId, st.searchDistance)
        if auraGo then
            auraGo:RemoveFromWorld()
        end
    end
end

-- C++ ResetNode (:586-601). Guarded: the large too-far path calls
-- it twice in one tick (GivePunishment, then the UpdateAI arm).
-- The pump dies here; state drops on ev37 when the delayed
-- despawn lands, so a synchronous ROUND_FINISHED continuation
-- still sees live state.
local function resetNode(creature, guid, st)
    if st.resetting then
        return
    end
    st.resetting = true

    creature:PlayDirectSound(SOUND_DISABLE_NODE)

    for color = SIMON_BLUE, SIMON_YELLOW do
        local entry = st.clusterIds[color + 1]
        if entry ~= 0 then
            local cluster = creature:FindNearestGameObject(entry, st.searchDistance)
            if cluster then
                cluster:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
            end
        end
    end

    for auraId = GO_AURA_BLUE, GO_AURA_YELLOW do
        local auraGo = creature:FindNearestGameObject(auraId, st.searchDistance)
        if auraGo then
            auraGo:RemoveFromWorld()
        end
    end

    local relicEntry = st.large and GO_APEXIS_MONUMENT or GO_APEXIS_RELIC
    local relic = creature:FindNearestGameObject(relicEntry, st.searchDistance)
    if relic then
        relic:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end

    cancelPump(guid, st)
    creature:DespawnOrUnsummon(1000)
end

-- C++ GivePunishment (:764-785).
local function givePunishment(creature, guid, st)
    if st.large then
        local player = GetPlayerByGUID(st.playerGuid)
        if player then
            local guardian = creature:SummonCreature(NPC_APEXIS_GUARDIAN, creature:GetX(), creature:GetY(), creature:GetZ() - st.zCoordCorrection, 0.0, 3, 20000)
            if guardian then
                guardian:EngageWithTarget(player)
            end
        end
        resetNode(creature, guid, st)
    else
        st.fails = st.fails + 1
        local player = GetPlayerByGUID(st.playerGuid)
        if player then
            creature:CastSpell(player, SPELL_BAD_PRESS_TRIGGER, true)
        end
        if st.fails >= 4 then
            resetNode(creature, guid, st)
        end
    end
end

-- C++ GiveRewardForLevel (:736-756).
local function giveRewardForLevel(creature, guid, st, level)
    local rewSpell = 0
    if level == 6 then
        if st.large then
            givePunishment(creature, guid, st)
        else
            rewSpell = SPELL_REWARD_BUFF_1
        end
    elseif level == 8 then
        rewSpell = SPELL_REWARD_BUFF_2
    elseif level == 10 then
        rewSpell = SPELL_REWARD_BUFF_3
    end

    if rewSpell ~= 0 then
        local player = GetPlayerByGUID(st.playerGuid)
        if player then
            creature:CastSpell(player, rewSpell, true)
        end
    end
end

-- C++ DoAction (:463-486). Called directly for self-actions (the
-- C++ self-call twin); the ev38 handler routes external DoAction
-- calls here too.
local function doAction(creature, guid, st, action)
    if action == ACTION_SIMON_ROUND_FINISHED then
        st.listening = false
        creature:CastSpell(creature, SPELL_VISUAL_START_AI_LEVEL)
        giveRewardForLevel(creature, guid, st, st.gameLevel)
        cancelRoundEvents(st)
        if st.gameLevel == 10 then
            resetNode(creature, guid, st)
        else
            scheduleEvent(st, EVENT_SIMON_SETUP_PRE_GAME, 1000)
        end
    elseif action == ACTION_SIMON_CORRECT_FULL_SEQUENCE then
        st.gameLevel = st.gameLevel + 1
        doAction(creature, guid, st, ACTION_SIMON_ROUND_FINISHED)
    elseif action == ACTION_SIMON_WRONG_SEQUENCE then
        givePunishment(creature, guid, st)
        doAction(creature, guid, st, ACTION_SIMON_ROUND_FINISHED)
    end
end

-- C++ CheckPlayer (:803-817).
local function checkPlayer(creature, guid, st)
    local player = GetPlayerByGUID(st.playerGuid)
    if not player then
        return false
    end
    if player:IsDead() then
        return false
    end
    if player:GetDistance2d(creature) >= 2.0 * st.searchDistance then
        givePunishment(creature, guid, st)
        return false
    end
    return true
end

-- C++ CheckPlayerSequence (:608-623).
local function checkPlayerSequence(creature, guid, st)
    local correct = true
    if #st.playerSequence <= #st.colorSequence then
        for i = 1, #st.playerSequence do
            if st.playerSequence[i] ~= st.colorSequence[i] then
                correct = false
            end
        end
    end

    if correct and #st.playerSequence == #st.colorSequence then
        doAction(creature, guid, st, ACTION_SIMON_CORRECT_FULL_SEQUENCE)
    elseif not correct then
        doAction(creature, guid, st, ACTION_SIMON_WRONG_SEQUENCE)
    end
end

-- C++ SetData (:488-511), reached via the DoAction transport
-- (see header): the cluster entry arrives as the action value.
local function onClusterPress(creature, guid, st, clusterEntry)
    if not st.listening then
        return
    end

    local pressedColor = SIMON_MAX_COLORS
    if clusterEntry == st.clusterIds[SIMON_RED + 1] then
        pressedColor = SIMON_RED
    elseif clusterEntry == st.clusterIds[SIMON_BLUE + 1] then
        pressedColor = SIMON_BLUE
    elseif clusterEntry == st.clusterIds[SIMON_GREEN + 1] then
        pressedColor = SIMON_GREEN
    elseif clusterEntry == st.clusterIds[SIMON_YELLOW + 1] then
        pressedColor = SIMON_YELLOW
    end

    playColor(creature, pressedColor)
    st.playerSequence[#st.playerSequence + 1] = pressedColor
    scheduleEvent(st, EVENT_SIMON_RESET_CLUSTERS, 500)
    checkPlayerSequence(creature, guid, st)
end

-- C++ SetUpPreGame (:699-733).
local function setUpPreGame(creature, st)
    for color = SIMON_BLUE, SIMON_YELLOW do
        local cluster = creature:FindNearestGameObject(st.clusterIds[color + 1], 2.0 * st.searchDistance)
        if cluster then
            cluster:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)

            -- break since we don't need glowing auras for large clusters
            if st.large then
                break
            end

            creature:Teleport(cluster:GetX(), cluster:GetY(), cluster:GetZ(), cluster:GetO())
            creature:CastSpell(cluster, SIMON_PRE_GAME[color], true)
        end
    end

    local relicEntry = st.large and GO_APEXIS_MONUMENT or GO_APEXIS_RELIC
    local relic = creature:FindNearestGameObject(relicEntry, st.searchDistance)
    if relic then
        creature:Teleport(relic:GetX(), relic:GetY(), relic:GetZ() + st.zCoordCorrection, relic:GetO())
    end

    generateColorSequence(st)
end

-- C++ UpdateAI (:409-461) — one due event per 500ms pump tick.
local function gamePump(creature, guid)
    local st = bunnyState[guid]
    if not st or st.resetting then
        return
    end

    -- EventMap Update: all timers advance, then ExecuteEvent
    -- takes the single earliest-due event (C++ ties: insertion
    -- order; the Lua pump breaks ties by lowest id, so the second
    -- tied event waits an extra pump tick).
    local fireId = nil
    for id, delay in pairs(st.events) do
        local remaining = delay - 500
        st.events[id] = remaining
        if remaining <= 0 then
            if fireId == nil or remaining < st.events[fireId] or (remaining == st.events[fireId] and id < fireId) then
                fireId = id
            end
        end
    end
    if fireId == nil then
        return
    end
    st.events[fireId] = nil

    if fireId == EVENT_SIMON_PERIODIC_PLAYER_CHECK then
        if not checkPlayer(creature, guid, st) then
            resetNode(creature, guid, st)
        else
            scheduleEvent(st, EVENT_SIMON_PERIODIC_PLAYER_CHECK, 2000)
        end
    elseif fireId == EVENT_SIMON_SETUP_PRE_GAME then
        setUpPreGame(creature, st)
        cancelEvent(st, EVENT_SIMON_GAME_TICK)
        scheduleEvent(st, EVENT_SIMON_PLAY_SEQUENCE, 1000)
    elseif fireId == EVENT_SIMON_PLAY_SEQUENCE then
        if #st.playableSequence > 0 then
            playNextColor(creature, st)
            scheduleEvent(st, EVENT_SIMON_PLAY_SEQUENCE, 1500)
        else
            st.listening = true
            creature:CastSpell(creature, SPELL_VISUAL_START_PLAYER_LEVEL)
            st.playerSequence = {}
            prepareClusters(creature, st, false)
            st.gameTicks = 0
            scheduleEvent(st, EVENT_SIMON_GAME_TICK, 3000)
        end
    elseif fireId == EVENT_SIMON_GAME_TICK then
        creature:CastSpell(creature, SPELL_AUDIBLE_GAME_TICK)
        if st.gameTicks > st.gameLevel then
            scheduleEvent(st, EVENT_SIMON_TOO_LONG_TIME, 500)
        else
            scheduleEvent(st, EVENT_SIMON_GAME_TICK, 3000)
        end
        st.gameTicks = st.gameTicks + 1
    elseif fireId == EVENT_SIMON_RESET_CLUSTERS then
        prepareClusters(creature, st, true)
    elseif fireId == EVENT_SIMON_TOO_LONG_TIME then
        doAction(creature, guid, st, ACTION_SIMON_WRONG_SEQUENCE)
    elseif fireId == EVENT_SIMON_ROUND_FINISHED then
        doAction(creature, guid, st, ACTION_SIMON_ROUND_FINISHED)
    end
end

-- C++ StartGame (:526-583): cluster scan by displayId, event
-- seeding, relic lock, pump start. Cluster gossip handlers are
-- registered here for the discovered entries (see header).
local function startGame(creature, guid, playerGuid, id)
    local large = id ~= 0
    local st = {
        large = large,
        listening = false,
        gameLevel = 0,
        fails = 0,
        gameTicks = 0,
        playerGuid = playerGuid,
        clusterIds = { 0, 0, 0, 0 },
        zCoordCorrection = large and 8.0 or 2.75,
        searchDistance = large and 13.0 or 5.0,
        colorSequence = {},
        playableSequence = {},
        playerSequence = {},
        events = {},
        resetting = false,
    }
    bunnyState[guid] = st

    creature:SetScale(large and 2.0 or 1.0)

    local displays = large and SIMON_DISPLAY_LARGE or SIMON_DISPLAY_SMALL
    for _, go in ipairs(creature:GetGameObjectsInRange(st.searchDistance)) do
        local displayId = go:GetDisplayId()
        for color = SIMON_BLUE, SIMON_YELLOW do
            if displayId == displays[color] then
                st.clusterIds[color + 1] = go:GetEntry()
            end
        end
    end

    for color = SIMON_BLUE, SIMON_YELLOW do
        local entry = st.clusterIds[color + 1]
        if entry ~= 0 and not registeredCluster[entry] then
            registeredCluster[entry] = true
            RegisterGameObjectGossipEvent(entry, 1, onClusterGossip)
        end
    end

    scheduleEvent(st, EVENT_SIMON_ROUND_FINISHED, 1000)
    scheduleEvent(st, EVENT_SIMON_PERIODIC_PLAYER_CHECK, 2000)

    local relicEntry = large and GO_APEXIS_MONUMENT or GO_APEXIS_RELIC
    local relic = creature:FindNearestGameObject(relicEntry, st.searchDistance)
    if relic then
        relic:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    end

    st.pumpId = CreateLuaEvent(function()
        gamePump(creature, guid)
    end, 500, 0)
end

-- C++ SetGUID (:513-521).
local function onSetGUID(_, creature, playerGuid, id)
    local guid = creature:GetGUIDLow()
    local old = bunnyState[guid]
    if old then
        cancelPump(guid, old)
    end
    creature:SetCanFly(true)
    startGame(creature, guid, playerGuid, id)
end

-- C++ DoAction (:463-486) for 8/9/10; any other value is a
-- SetData cluster-entry press (see header).
local function onDoAction(_, creature, action)
    local guid = creature:GetGUIDLow()
    local st = bunnyState[guid]
    if not st then
        return
    end
    if action == ACTION_SIMON_CORRECT_FULL_SEQUENCE or action == ACTION_SIMON_WRONG_SEQUENCE or action == ACTION_SIMON_ROUND_FINISHED then
        doAction(creature, guid, st, action)
    else
        onClusterPress(creature, guid, st, action)
    end
end

-- C++ SpellHitTarget (:787-800).
local function onSpellHitTarget(_, creature, target, spellId)
    if spellId ~= SPELL_BAD_PRESS_TRIGGER then
        return
    end
    if target == nil then
        return
    end
    local guid = creature:GetGUIDLow()
    local st = bunnyState[guid]
    if not st then
        return
    end
    local bp = math.floor(st.fails * 0.33 * target:GetMaxHealth())
    target:CastCustomSpell(target, SPELL_BAD_PRESS_DAMAGE, true, bp)
end

local function onCleanup(_, creature)
    local guid = creature:GetGUIDLow()
    local st = bunnyState[guid]
    if st then
        cancelPump(guid, st)
    end
    bunnyState[guid] = nil
end

-- C++ go_simon_cluster OnGossipHello (:833-840). The goober-spell
-- cast stays out (DB-side goober.spellId, unestablishable here).
-- Plain assignment to the forward-declared local (see top).
onClusterGossip = function(_, player, go)
    local bunny = go:FindNearestCreature(NPC_SIMON_BUNNY, 12.0)
    if bunny then
        bunny:DoAction(go:GetEntry())
    end
    go:AddUse()
    return true
end

-- C++ go_apexis_relic OnGossipSelect (:873-885). The hello/menu
-- half is the native questgiver/goober arm (see header).
local function onRelicSelect(_, player, go)
    local large = go:GetEntry() == GO_APEXIS_MONUMENT
    if player:GetItemCount(ITEM_APEXIS_SHARD) >= (large and 35 or 1) then
        player:CastSpell(player, large and SPELL_TAKE_REAGENTS_GROUP or SPELL_TAKE_REAGENTS_SOLO, false)
        local bunny = player:SummonCreature(NPC_SIMON_BUNNY, go:GetX(), go:GetY(), go:GetZ(), 0)
        if bunny then
            bunny:SetGUID(player:GetGUID(), large and 1 or 0)
        end
    end
    return true
end

RegisterCreatureEvent(NPC_SIMON_BUNNY, 40, onSetGUID)
RegisterCreatureEvent(NPC_SIMON_BUNNY, 38, onDoAction)
RegisterCreatureEvent(NPC_SIMON_BUNNY, 15, onSpellHitTarget)
-- Cleanup only on died (4) / remove (37): C++ has no Reset override
-- and leave-combat touches nothing, so an evade (ev23 then ev2) must
-- NOT cancel the pump or drop state, or the node bricks (relic stays
-- NOT_SELECTABLE, bunny is MANUAL_DESPAWN).
RegisterCreatureEvent(NPC_SIMON_BUNNY, 4, onCleanup)
RegisterCreatureEvent(NPC_SIMON_BUNNY, 37, onCleanup)

RegisterGameObjectGossipEvent(GO_APEXIS_RELIC, 2, onRelicSelect)
RegisterGameObjectGossipEvent(GO_APEXIS_MONUMENT, 2, onRelicSelect)
