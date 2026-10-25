-- Culling of Stratholme: npc_chromie_start — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/culling_of_stratholme.cpp
-- (npc_chromie_start : public CreatureScript).
-- Entry: NPC_CHROMIE 26527 (instance_culling_of_stratholme.cpp:61).
-- Ported: OnGossipSelect's GOSSIP_OFFSET_TELEPORT arm (:364-367):
-- CloseGossipMenuFor + me->CastSpell(player, SPELL_TELEPORT_PLAYER
-- 53435), non-triggered. The Lua select hook receives (sender,
-- action); GOSSIP_OFFSET_TELEPORT = 2 (enum :238-240), so the gate
-- is action == GOSSIP_ACTION_INFO_DEF + 2 = 1002 (exact).
-- Documented-only: the instance-gated OnGossipHello menu (skip /
-- explain arms ride instance->GetStableStateFor / AdvanceDungeon /
-- SetGuidData — no instance bridge), the EXPLAIN / SKIP menu-nav
-- select arms (fire only from the ungated menu), the AddItem
-- (ITEM_ARCANE_DISRUPTOR) leg (no item-grant bridge), and the GM
-- instance-control arms (SetData / SetGuidData — no instance
-- bridge).
-- Documented-only (nearest-creature axis): npc_hearthsinger_forresten_cot
-- (entry 30551 — inn cinematic, culling_of_stratholme.cpp:118/153/166):
-- FindNearestCreature(talkerEntry, 80.0f, true) (:153) and
-- FindNearestCreature(NPC_BELFAST, 80.0f, true) (:118) both ride
-- npc_hearthsinger_forresten_cotAI::UpdateAI's EventMap, whose events
-- are scheduled from MoveInLineOfSight / SetData (:166 is the
-- unbridged SetGUID hook) — and the :118 arm ends in a cross-creature
-- DoAction leg. The AI is not registered in boss_ai.go; stays out as
-- unregistered AI.
-- Documented-only (nearest-creature axis): npc_malcolm_moore (entry
-- 27891 — crate event #3 cinematic, culling_of_stratholme.cpp:1042-1081):
-- the FindNearestCreature(NPC_SCRUFFY, 100.0f, true) arms sit in
-- npc_malcolm_mooreAI::UpdateAI event cases that only fire from
-- MovementInform spline-chain events (InitializeAI ->
-- MoveAlongSplineChain — no movement bridge). The AI is not registered
-- in boss_ai.go; stays out as unregistered AI.
-- Documented-only (quest-giver axis): npc_chromie_middle
-- (culling_of_stratholme.cpp:474-545 — no entry constant anywhere in the
-- C++ tree; culling_of_stratholme.h names only NPC_CHROMIE = 26527, "we
-- don't need the other two for anything" — DB ScriptName binding only,
-- unverifiable, wounded-skirmisher precedent). OnGossipHello (:508-514):
-- the me->IsQuestGiver() / player->PrepareQuestMenu(me->GetGUID()) pair
-- (:509-510) is inseparable and PrepareQuestMenu has no bridge, so the
-- query result gates an unbridgeable call; the rest of the menu rides
-- instance->GetData(DATA_INSTANCE_PROGRESS) == CRATES_DONE (no instance
-- bridge). OnGossipSelect STEP1/2/3 menu-nav ends in AdvanceDungeon ->
-- instance->SetGuidData(DATA_UTHER_START, ...) (no instance bridge); the
-- MoveInLineOfSight whisper machine rides the same instance
-- GetData/quest-status legs. The AI is not registered in boss_ai.go;
-- stays out as unregistered AI.

local ENTRY_CHROMIE_START = 26527

local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_OFFSET_TELEPORT = 2
local SPELL_TELEPORT_PLAYER = 53435

local function chromieGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_TELEPORT then
        player:GossipComplete()
        creature:CastSpell(player, SPELL_TELEPORT_PLAYER)
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

RegisterCreatureGossipEvent(ENTRY_CHROMIE_START, 2, chromieGossipSelect)

-- Ported (crate fluff event #2): npc_bartleby_battson (entry 27907,
-- culling_of_stratholme.cpp:852-945). InitializeAI schedules the idle
-- Talk loop (EVENT_BARTLEBY_IDLE, {60,120}s seed, Repeat {120,240}s);
-- seeded here on event 5 (spawn — the AddToWorld funnel fires 5 after
-- 36, creaturemotion.go). The DoAction ACTION_START_FLUFF leg stays
-- out (no DoAction bridge): it would cancel the idle loop and seed
-- BARTLEBY1 {15,30}s, so the idle loop keeps running past fluff start
-- in Go — documented deviation, not a ported behavior. The
-- MovementInform spline-chain legs (BARTLEBY2/2_2/3, despawn) stay out
-- (no MoveAlongSplineChain/MovementInform bridge).
-- Ported (crate fluff event #4): npc_sergeant_morigan (entry 27877,
-- culling_of_stratholme.cpp:1131-1280) idle Q&A loop. InitializeAI
-- Reschedules IDLE1 {5,15}s; IDLE1 Talks the question (urand(0,1):
-- 1 -> ASK_NO 0, 0 -> ASK_YES 1; Started -> START 2) and schedules
-- IDLE2 10s; IDLE2 answers via the nearest perelli (entry 27876,
-- 10yd, FindNearestCreature bridge) with the sergeant as Talk target
-- (Question truthy -> PERELLI_NO 0, else PERELLI_YES 1) and
-- reschedules IDLE1 {15,30}s — except Question == 2, whose CHAIN1
-- spline leg stays out, ending the loop. Started is per-GUID state
-- that only the unbridged DoAction ACTION_START_FLUFF flips, so it
-- stays false in Go (same documented deviation as bartleby).
-- Documented-only (crate fluff events #1/#3/#5, spline-only):
-- npc_martha_goslin (27884) JustAppeared/MovementInform arms only
-- move along spline chains and set UNIT_NPC_EMOTESTATE (no spline /
-- emotestate bridge); npc_jena_anderson (27885) UpdateAI/MovementInform
-- arms are spline chains + cross-creature DoAction/Talk legs gated on
-- them; npc_malcolm_moore (27891) + scruffy (27892) are
-- InitializeAI/MovementInform spline chains (+ SummonCreature leg for
-- scruffy, whose chain is likewise spline-gated); npc_roger_owens
-- (27903) starts only on DoAction ACTION_START_FLUFF and runs
-- spline chains thereafter. npc_crate_helper (crate_helper_cot):
-- SpellHit event 14 IS bridged, but the 49590 arm's observable legs
-- are instance SetData (no instance bridge), GO crate swap (GO Delete
-- has no bridge; SummonGameObject alone cannot replace-and-delete)
-- and nearest-fluff DoAction (no DoAction bridge), so the
-- _crateRevealed latch alone is unobservable and stays out.
-- Documented-only: at_stratholme_inn_stairs_cot (area trigger, no
-- RegisterAreaTrigger binding in-tree) — instance GetData gate +
-- cross-creature SetGUID leg, both unbridged; forresten's SetGUID /
-- SetData legs ride the same unbridged cross-AI hooks.

local ENTRY_BARTLEBY = 27907
local ENTRY_SERGEANT = 27877
local ENTRY_PERELLI = 27876

local timers = {}
local moriganQuestion = {}
local moriganStarted = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    moriganQuestion[guid] = nil
    moriganStarted[guid] = nil
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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ EVENT_BARTLEBY_IDLE: Talk(LINE_BARTLEBY_IDLE 0), Repeat
-- {120,240}s (new random range each firing).
local function onBartlebyIdle(creature, guid)
    creature:Talk(0)
    schedule(guid, "idle", math.random(120000, 240000), function()
        onBartlebyIdle(creature, guid)
    end)
end

local function onBartlebySpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "idle", math.random(60000, 120000), function()
        onBartlebyIdle(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_BARTLEBY, 5, onBartlebySpawn)
RegisterCreatureEvent(ENTRY_BARTLEBY, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_BARTLEBY, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)

-- C++ Perelli(line[, ori]): nearest perelli Talks line to the
-- sergeant; the idle path calls Perelli without ori (default 0 = no
-- facing change) — the 5.916666f facing belongs to the chain leg
-- (EVENT_PERELLI1), not the idle loop.
local function moriganPerelli(creature, line)
    local perelli = creature:FindNearestCreature(ENTRY_PERELLI, 10)
    if perelli then
        perelli:Talk(line, creature)
    end
end

-- Cyclic idle pair: forward declarations (tharon_ja/gal_darah idiom);
-- plain assignments, never re-local, or closures bind the nil slot.
local onMoriganIdle1, onMoriganIdle2

onMoriganIdle1 = function(creature, guid)
    local question
    if moriganStarted[guid] then
        question = 2
        creature:Talk(2)
    else
        question = math.random(0, 1)
        creature:Talk(question == 1 and 0 or 1)
    end
    moriganQuestion[guid] = question
    schedule(guid, "idle2", 10000, function()
        onMoriganIdle2(creature, guid)
    end)
end

onMoriganIdle2 = function(creature, guid)
    local question = moriganQuestion[guid] or 0
    moriganPerelli(creature, question == 0 and 1 or 0)
    if question == 2 then
        -- EVENT_SERGEANT_CHAIN1 spline leg stays out; loop ends.
    else
        schedule(guid, "idle1", math.random(15000, 30000), function()
            onMoriganIdle1(creature, guid)
        end)
    end
end

local function onMoriganSpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    moriganStarted[guid] = false
    schedule(guid, "idle1", math.random(5000, 15000), function()
        onMoriganIdle1(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_SERGEANT, 5, onMoriganSpawn)
RegisterCreatureEvent(ENTRY_SERGEANT, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_SERGEANT, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
