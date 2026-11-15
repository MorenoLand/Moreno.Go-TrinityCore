-- Ashenvale: Muglash (Vor'sha escort) — Lua port of
-- src/server/scripts/Kalimdor/zone_ashenvale.cpp
-- (class npc_muglash : CreatureScript("npc_muglash");
-- struct npc_muglashAI : public EscortAI;
-- class go_naga_brazier : public GameObjectScript("go_naga_brazier")).
-- AddSC_zone_ashenvale registers the zone script
-- (kalimdor_script_loader.cpp decl/call — the "// Ashenvale"
-- loader block).
-- Entry: 12717 (NPC_MUGLASH, zone_ashenvale.cpp:173; enum-verified,
-- like the emily precedent — the creature_template ScriptName
-- binding is DB-side). Brazier GO: 178247 (ScriptName
-- go_naga_brazier, TDB-verified).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 4 JustDied,
-- 1 OnEnterCombat, 2 OnLeaveCombat, 23 OnReset. GO gossip event 1
-- OnHello on the brazier. Timers via CreateLuaEvent (moroes
-- precedent: per-GUID timers table, RemoveEventById cancel).
-- Ported arms:
-- - OnQuestAccept (zone_ashenvale.cpp:237-244, event 31): quest id
--   6641 (QUEST_VORSHA) -> Talk(SAY_MUG_START1 0, untargeted — the
--   willix precedent). SetFaction(113
--   FACTION_ESCORTEE_N_NEUTRAL_PASSIVE, C++ order: Talk, SetFaction,
--   Start) also ported, including StartEscort(player) — the escort
--   bridges landed since (ticks 372-373).
-- - JustDied (zone_ashenvale.cpp:227-232, event 4) — PORTED 2026-11-07:
--   if IsEscorted() then GetEscortPlayer():FailQuest(QUEST_VORSHA
--   6641) (hanes precedent uses a GUID latch; the bridge idiom is
--   the legoso healing arm).
-- - WaypointReached (zone_ashenvale.cpp:249-278) — PORTED 2026-11-15
--   (ev6, onReachWP below): Talk arms SAY_MUG_START2 1 (wp0) /
--   SAY_MUG_BRAZIER 2 (wp24) / SAY_MUG_GRATITUDE 7 (wp25) /
--   SAY_MUG_PATROL 8 (wp26) / SAY_MUG_RETURN 9 (wp27), the wp24
--   brazier GO flag clear (GO_NAGA_BRAZIER 178247, 10yd =
--   INTERACTION_DISTANCE*2; GAMEOBJECT_FLAGS 9, NOT_SELECTABLE 16,
--   arlokk precedent) and the wp25 GroupEventHappens(QUEST_VORSHA
--   6641); the walk rides MoveWaypoint() from OnQuestAccept
--   (StartEscort only latches the player — ruul precedent).
--   The wp24 SetEscortPaused(true) arm is PORTED 2026-11-15 with
--   the wave machine below (it was omitted before because its
--   resume rode the unported brazier/wave machine). StopEscort()
--   at wp29 — the path's last point (waypoint_data 101738, 30
--   points 0-29, TDB-verified: starts 5.7yd from the muglash
--   spawn, pt24 at the brazier, pt25-27 the credit/patrol/return
--   nodes; the old stop-at-27 claim mistook the AI's highest
--   handled id for the path end; Go paths loop by default).
--   The escort path itself is ORPHANED in TDB 335.26091 (no
--   script_waypoint rows for 12717; creature_template_addon
--   path_id is 0 and no creature_addon row exists for spawn guid
--   33115, so loadCreaturePathID falls back to the spawn GUID
--   and MoveWaypoint() is a no-op) — bound by the SQL migration
--   alongside this port (wounded-blood-elf precedent).
-- - JustEngagedWith (zone_ashenvale.cpp:216-225, event 1) —
--   PORTED 2026-11-15: if the escort player exists and the
--   escort is paused, 50% Talk(SAY_MUG_ON_GUARD 4, player)
--   (urand(0, 1) — math.random(0, 1) == 1). The paused state is
--   a Lua latch mirroring the only SetEscortPaused calls (wp24
--   true, wave 4 false); no HasEscortState bridge exists.
-- - Reset / Initialize (zone_ashenvale.cpp:206-214, event 23) —
--   PORTED 2026-11-15: eventTimer 10000, waveId 0,
--   _isBrazierExtinguished false (per-GUID wave state below) +
--   timer cancel. The IsDead guard is the marli ev23 idiom: Go
--   fires ev23 on spawn, evade and death, while C++ Reset runs
--   on spawn + evade only. Evade also fires event 2, which
--   cancels timers (moroes guestReset precedent); death cancels
--   via event 4 in onDied.
-- - UpdateAI wave machine (zone_ashenvale.cpp:305-322) —
--   PORTED 2026-11-15: while unvictimed, paused and extinguished,
--   the 10s eventTimer counts down, then ++waveId + DoWaveSummon
--   and re-arms 10s. The 1s CreateLuaEvent poll is the
--   moroes/dorothea analogue of the C++ diff tick (countdown only
--   decrements while the three conditions hold, so the first wave
--   fires 10s after the brazier is extinguished, C++-exact to
--   1s granularity). DoWaveSummon (zone_ashenvale.cpp:280-303):
--   wave 1 rider 3713 / sorceress 3717 / razortail 3712 at
--   FirstNagaCoord, wave 2 priestess 3944 / myrmidon 3711 /
--   seawitch 3715 at SecondNagaCoord, wave 3 vorsha 12940 at
--   VorshaCoord (TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, bridge
--   type 3, 60s — arlokk/jindo precedent), wave 4
--   SetEscortPaused(false) + Talk(SAY_MUG_DONE 6, untargeted).
--   Every summon gets AttackStart(muglash) inline at the summon
--   site (the C++ JustSummoned leg :234-237 — willix/ruul
--   precedent; ev19 stays unregistered, the arm is covered
--   inline). The wave NPCs are SmartAI in TDB (no ScriptName),
--   so no separate Lua file fires for them.
-- - go_naga_brazier OnGossipHello (zone_ashenvale.cpp:348-364,
--   GO gossip event 1) — PORTED 2026-11-15: the nearest muglash
--   within 10yd (INTERACTION_DISTANCE*2) gets Talk
--   (SAY_MUG_BRAZIER_WAIT 3) and _isBrazierExtinguished = true
--   (the per-GUID wave-state latch — the GO handler lives in
--   this file so it shares the state, arlokk gong precedent).
--   Returns false when muglash is found (C++ false — the native
--   GO arms proceed, Eluna polarity per gameobjects.go: the
--   hook's true return skips them) and true when none is found
--   (C++ true). The brazier is unclickable before wp24 anyway:
--   it carries GO_FLAG_NOT_SELECTABLE until the wp24 flag clear.
-- Deviations from C++: none in the arms above. The 5s node delay
-- at path pt24 / 3s at pt26 (waypoint_data 101738) is engine-side
-- and rides the Go waypoint leg as-is.
-- No SAY_MUG_VORSHA exists (enum zone_ashenvale.cpp:149-158;
-- SAY_MUG_BRAZIER_WAIT is 3, SAY_MUG_ON_GUARD is 4).

local ENTRY_MUGLASH = 12717

local SAY_MUG_START1 = 0
local SAY_MUG_START2 = 1
local SAY_MUG_BRAZIER = 2
local SAY_MUG_BRAZIER_WAIT = 3
local SAY_MUG_ON_GUARD = 4
local SAY_MUG_DONE = 6
local SAY_MUG_GRATITUDE = 7
local SAY_MUG_PATROL = 8
local SAY_MUG_RETURN = 9

local QUEST_VORSHA = 6641

local GO_NAGA_BRAZIER = 178247
local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16

local NPC_WRATH_RIDER = 3713
local NPC_WRATH_SORCERESS = 3717
local NPC_WRATH_RAZORTAIL = 3712
local NPC_WRATH_PRIESTESS = 3944
local NPC_WRATH_MYRMIDON = 3711
local NPC_WRATH_SEAWITCH = 3715
local NPC_VORSHA = 12940

local SUMMON_TYPE_OOC_DESPAWN = 3
local SUMMON_MS = 60000
local WAVE_DELAY_MS = 10000
local POLL_MS = 1000

-- C++ FirstNagaCoord[3] / SecondNagaCoord[3] / VorshaCoord
-- (zone_ashenvale.cpp:175-193).
local FIRST_NAGA_COORD = {
    { 3603.504150, 1122.631104, 1.635, 0.0 },
    { 3589.293945, 1148.664063, 5.565, 0.0 },
    { 3609.925537, 1168.759521, -1.168, 0.0 },
}
local SECOND_NAGA_COORD = {
    { 3609.925537, 1168.759521, -1.168, 0.0 },
    { 3645.652100, 1139.425415, 1.322, 0.0 },
    { 3583.602051, 1128.405762, 2.347, 0.0 },
}
local VORSHA_COORD = { 3633.056885, 1172.924072, -5.388, 0.0 }
local FIRST_WAVE = { NPC_WRATH_RIDER, NPC_WRATH_SORCERESS, NPC_WRATH_RAZORTAIL }
local SECOND_WAVE = { NPC_WRATH_PRIESTESS, NPC_WRATH_MYRMIDON, NPC_WRATH_SEAWITCH }

-- Per-GUID wave state (C++ npc_muglashAI members eventTimer /
-- waveId / _isBrazierExtinguished, plus the paused latch that
-- mirrors SetEscortPaused — no HasEscortState bridge exists).
local waveState = {}

-- Per-GUID CreateLuaEvent ids (moroes precedent).
local timers = {}

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

-- C++ npc_muglashAI::DoWaveSummon (zone_ashenvale.cpp:280-303).
local function doWaveSummon(creature, guid)
    local st = waveState[guid]
    if not st then
        return
    end
    if st.waveId == 1 or st.waveId == 2 then
        local coords = st.waveId == 1 and FIRST_NAGA_COORD or SECOND_NAGA_COORD
        local entries = st.waveId == 1 and FIRST_WAVE or SECOND_WAVE
        for i = 1, 3 do
            local pos = coords[i]
            local summoned = creature:SummonCreature(entries[i], pos[1], pos[2], pos[3], pos[4], SUMMON_TYPE_OOC_DESPAWN, SUMMON_MS)
            if summoned then
                summoned:AttackStart(creature)
            end
        end
    elseif st.waveId == 3 then
        local summoned = creature:SummonCreature(NPC_VORSHA, VORSHA_COORD[1], VORSHA_COORD[2], VORSHA_COORD[3], VORSHA_COORD[4], SUMMON_TYPE_OOC_DESPAWN, SUMMON_MS)
        if summoned then
            summoned:AttackStart(creature)
        end
    elseif st.waveId == 4 then
        st.paused = false
        creature:SetEscortPaused(false)
        creature:Talk(SAY_MUG_DONE)
    end
end

-- C++ npc_muglashAI::UpdateAI wave leg (zone_ashenvale.cpp:305-322):
-- 1s poll; the 10s countdown only runs while unvictimed, paused
-- and extinguished, then ++waveId + DoWaveSummon + re-arm 10s.
local function waveTick(creature, guid)
    local st = waveState[guid]
    if not st or not creature:IsEscorted() then
        return
    end
    if st.waveId >= 4 then
        return
    end
    if not st.paused or not st.extinguished or creature:GetVictim() then
        schedule(guid, "wave", POLL_MS, function() waveTick(creature, guid) end)
        return
    end
    st.countdownMs = st.countdownMs - POLL_MS
    if st.countdownMs > 0 then
        schedule(guid, "wave", POLL_MS, function() waveTick(creature, guid) end)
        return
    end
    st.waveId = st.waveId + 1
    doWaveSummon(creature, guid)
    st.countdownMs = WAVE_DELAY_MS
    if st.waveId < 4 then
        schedule(guid, "wave", POLL_MS, function() waveTick(creature, guid) end)
    end
end

-- C++ npc_muglashAI::Reset -> Initialize (zone_ashenvale.cpp:206-214).
local function muglashReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if creature:IsDead() then
        return
    end
    waveState[guid] = { waveId = 0, countdownMs = WAVE_DELAY_MS, extinguished = false, paused = false }
end

-- C++ evade runs Reset(); Go also fires ev23, so this only cancels
-- (moroes guestReset precedent).
local function muglashLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_muglashAI::JustEngagedWith (zone_ashenvale.cpp:216-225).
local function onEnterCombat(event, creature)
    local guid = creature:GetGUIDLow()
    local st = waveState[guid]
    if not st or not st.paused then
        return
    end
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    if math.random(0, 1) == 1 then
        creature:Talk(SAY_MUG_ON_GUARD, player)
    end
end

-- C++ OnQuestAccept for quest 6641 (zone_ashenvale.cpp:237-244, C++ order:
-- Talk, SetFaction, Start): Talk(0) + SetFaction(113) + StartEscort +
-- MoveWaypoint() (the walk rides MoveWaypoint — StartEscort only latches
-- the player, ruul precedent).
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_VORSHA then
        return
    end
    creature:Talk(SAY_MUG_START1)
    creature:SetFaction(113)
    creature:StartEscort(player)
    creature:MoveWaypoint()
end

-- C++ WaypointReached (zone_ashenvale.cpp:249-278), whole switch gated on
-- GetPlayerForEscort(): wp0 Talk(1, player); wp24 Talk(2, player) +
-- brazier GO RemoveFlag(NOT_SELECTABLE) + SetEscortPaused(true) + arm the
-- wave poller; wp25 Talk(7) + player:GroupEventHappens(6641); wp26
-- Talk(8); wp27 Talk(9); wp29 StopEscort() (path end, waypoint_data
-- 101738 pts 0-29; Go paths loop by default).
local function onReachWP(_, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == 0 then
        creature:Talk(SAY_MUG_START2, player)
    elseif wp == 24 then
        creature:Talk(SAY_MUG_BRAZIER, player)
        local brazier = creature:FindNearestGameObject(GO_NAGA_BRAZIER, 10.0)
        if brazier then
            brazier:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
        end
        local guid = creature:GetGUIDLow()
        local st = waveState[guid]
        if not st then
            st = { waveId = 0, countdownMs = WAVE_DELAY_MS, extinguished = false, paused = false }
            waveState[guid] = st
        end
        st.paused = true
        creature:SetEscortPaused(true)
        schedule(guid, "wave", POLL_MS, function() waveTick(creature, guid) end)
    elseif wp == 25 then
        creature:Talk(SAY_MUG_GRATITUDE)
        player:GroupEventHappens(QUEST_VORSHA, creature)
    elseif wp == 26 then
        creature:Talk(SAY_MUG_PATROL)
    elseif wp == 27 then
        creature:Talk(SAY_MUG_RETURN)
    elseif wp == 29 then
        creature:StopEscort()
    end
end

-- C++ JustDied (zone_ashenvale.cpp:227-232): if escorting, the escort
-- player fails QUEST_VORSHA. Also cancels the wave poller.
local function onDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
    if not creature:IsEscorted() then
        return
    end
    local player = creature:GetEscortPlayer()
    if player then
        player:FailQuest(QUEST_VORSHA)
    end
end

-- C++ go_naga_brazierAI::OnGossipHello (zone_ashenvale.cpp:348-364):
-- nearest muglash within 10yd gets Talk(SAY_MUG_BRAZIER_WAIT 3) and
-- _isBrazierExtinguished = true; false lets the native GO arms
-- proceed, true (no muglash) suppresses them.
local function onBrazierGossip(_, player, go)
    local muglash = go:FindNearestCreature(ENTRY_MUGLASH, 10.0)
    if not muglash then
        return true
    end
    muglash:Talk(SAY_MUG_BRAZIER_WAIT)
    local guid = muglash:GetGUIDLow()
    local st = waveState[guid]
    if not st then
        st = { waveId = 0, countdownMs = WAVE_DELAY_MS, extinguished = false, paused = false }
        waveState[guid] = st
    end
    st.extinguished = true
    return false
end

RegisterCreatureEvent(ENTRY_MUGLASH, 31, onQuestAccept)
RegisterCreatureEvent(ENTRY_MUGLASH, 6, onReachWP)
RegisterCreatureEvent(ENTRY_MUGLASH, 4, onDied)
RegisterCreatureEvent(ENTRY_MUGLASH, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_MUGLASH, 2, muglashLeaveCombat)
RegisterCreatureEvent(ENTRY_MUGLASH, 23, muglashReset)
RegisterGameObjectGossipEvent(GO_NAGA_BRAZIER, 1, onBrazierGossip)
