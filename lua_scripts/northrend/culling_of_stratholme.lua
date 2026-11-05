-- Culling of Stratholme: npc_chromie_start — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/culling_of_stratholme.cpp
-- (npc_chromie_start : public CreatureScript).
-- Entry: NPC_CHROMIE 26527 (instance_culling_of_stratholme.cpp:61).
-- Ported: OnGossipSelect's GOSSIP_OFFSET_TELEPORT arm (:364-367):
-- CloseGossipMenuFor + me->CastSpell(player, SPELL_TELEPORT_PLAYER
-- 53435), non-triggered. The Lua select hook receives (sender,
-- action); GOSSIP_OFFSET_TELEPORT = 2 (enum :238-240), so the gate
-- is action == GOSSIP_ACTION_INFO_DEF + 2 = 1002 (exact).
-- Ported: GOSSIP_OFFSET_SKIP_1 (1005) falls through to the TELEPORT
-- case in C++ (AdvanceDungeonFar leg drops — instance SetData, no
-- instance bridge), so it teleports exactly like TELEPORT.
-- Ported: GOSSIP_OFFSET_EXPLAIN_2 (1004) sends GOSSIP_TEXT_EXPLAIN_3
-- 12952 with no items (kalecgos terminal-step idiom); the
-- AdvanceDungeon leg drops (instance SetData) and the AddItem
-- (ITEM_ARCANE_DISRUPTOR 37888) leg drops (no item-grant bridge).
-- Documented-only: the instance-gated OnGossipHello menu (skip /
-- explain arms ride instance->GetStableStateFor / AdvanceDungeon /
-- SetGuidData — no instance bridge), the EXPLAIN / SKIP /
-- EXPLAIN_1 menu-nav select arms (their option texts live only in
-- DB gossip_menu_option rows — C++ carries no string constants for
-- them, unlike the kalecgos GOSSIP_ITEM_* precedent), and the GM
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
local GOSSIP_OFFSET_EXPLAIN_2 = 4
local GOSSIP_OFFSET_SKIP_1 = 5
local GOSSIP_TEXT_EXPLAIN_3 = 12952
local SPELL_TELEPORT_PLAYER = 53435

local function chromieGossipSelect(event, player, creature, sender, action)
    if action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_TELEPORT
        or action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_SKIP_1 then
        player:GossipComplete()
        creature:CastSpell(player, SPELL_TELEPORT_PLAYER)
    elseif action == GOSSIP_ACTION_INFO_DEF + GOSSIP_OFFSET_EXPLAIN_2 then
        player:GossipClearMenu()
        player:GossipSendMenu(GOSSIP_TEXT_EXPLAIN_3, creature)
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
-- 36, creaturemotion.go). The DoAction ACTION_START_FLUFF leg is
-- ported via the crate_helper event-14 relay at the end of this file
-- (ev38 DoAction dispatch exists but is bypassed: the relay calls
-- bartlebyStartFluff directly): the once-guard + idle-loop cancel are
-- ported, and the
-- BARTLEBY1 {15,30}s Talk(1) is ported as a one-shot timer in
-- bartlebyStartFluff: the Talk is separable from the
-- MoveAlongSplineChain leg in the same C++ case, which stays out, so
-- bartleby says line 1 at {15,30}s and then stays put instead of
-- walking/kneeling/talking lines 2-3/despawning (documented
-- deviation). The
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
-- flipped by the crate_helper event-14 relay at the end of this file
-- (nearest sergeant, 15yd): the next IDLE1 then Talks START (2) and
-- IDLE2 answers, ending the loop as in C++ minus the spline chain.
-- Documented-only (crate fluff events #1/#3/#5, spline-only):
-- npc_martha_goslin (27884) JustAppeared/MovementInform arms only
-- move along spline chains and set UNIT_NPC_EMOTESTATE (no spline /
-- emotestate bridge); npc_jena_anderson (27885) UpdateAI/MovementInform
-- arms are spline chains + cross-creature DoAction/Talk legs gated on
-- them; npc_malcolm_moore (27891) + scruffy (27892) are
-- InitializeAI/MovementInform spline chains (+ SummonCreature leg for
-- scruffy, whose chain is likewise spline-gated); npc_roger_owens
-- (27903) starts only on DoAction ACTION_START_FLUFF: the
-- EVENT_ROGER_START stand/facing/Talk(0) case is ported via the relay
-- below, and the MOVE1+ spline-chain legs stay out.
-- npc_crate_helper (crate_helper_cot,
-- entry 27827): the SpellHit event-14 relay IS ported at the end of
-- this file — on spell 49590 it picks the closest fluff creature by
-- the C++ Find radii (jena 45yd, bartleby 5yd, morigan 15yd, roger
-- 30yd; ReplaceIfCloser order, strict-closest wins) and delivers the
-- in-scope legs directly (bartleby idle-cancel/Started flip + BARTLEBY1
-- Talk(1) timer, morigan Started flip, roger START stand/facing/
-- Talk(0) timer); jena's Started flag is never read in C++ (no
-- observable leg), roger's MOVE1+ chain legs stay out (spline-gated),
-- and the malcolm Spawn fallback stays out (spline machine). The instance
-- SetData (DATA_CRATE_REVEALED) and GO crate swap legs stay out (no
-- instance bridge; GO Delete has no bridge).
-- Documented-only: at_stratholme_inn_stairs_cot (area trigger, no
-- RegisterAreaTrigger binding in-tree) — instance GetData gate +
-- cross-creature SetGUID leg, both unbridged; forresten's SetGUID /
-- SetData legs ride the same unbridged cross-AI hooks.

local ENTRY_BARTLEBY = 27907
local ENTRY_SERGEANT = 27877
local ENTRY_PERELLI = 27876
local UNIT_STAND_STATE_STAND = 0

local timers = {}
local moriganQuestion = {}
local moriganStarted = {}
local bartlebyStarted = {}

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
    bartlebyStarted[guid] = nil
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

-- C++ npc_bartleby_battsonAI::DoAction(ACTION_START_FLUFF): once-guard
-- + idle cancel + BARTLEBY1 Talk(1) one-shot; only the spline leg stays out.
-- Called directly by the crate_helper relay below — ev38 DoAction
-- dispatch exists but is bypassed: the relay already holds the
-- bartleby object, so no GUID-to-creature lookup is needed.
local function bartlebyStartFluff(creature)
    local guid = creature:GetGUID()
    if bartlebyStarted[guid] then
        return
    end
    bartlebyStarted[guid] = true
    local per = timers[guid]
    if per and per["idle"] then
        RemoveEventById(per["idle"])
        per["idle"] = nil
    end
    -- C++ EVENT_BARTLEBY1: Talk(LINE_BARTLEBY1 1) at {15,30}s; the
    -- MoveAlongSplineChain leg in the same case stays out (no bridge).
    schedule(guid, "fluff1", math.random(15000, 30000), function()
        creature:Talk(1)
    end)
end

-- C++ npc_roger_owensAI::DoAction(ACTION_START_FLUFF) + EVENT_ROGER_START
-- (culling_of_stratholme.cpp:1296-1297, 1336-1342): the START case's
-- stand/facing/Talk(0) legs are ported as a one-shot {5,12}s timer --
-- the Talk is separable from the EVENT_ROGER_MOVE1 spline leg it
-- schedules, which stays out (no MoveAlongSplineChain/MovementInform
-- bridge), so roger says line 0 at {5,12}s and then stays put instead
-- of walking the chain/talking lines 1-4/despawning. No Started latch
-- in C++; schedule() replaces a pending fluff timer if a second
-- helper reveals roger (C++ would stack a second START event).
-- Called directly by the crate_helper relay below.
local function rogerStartFluff(creature)
    local guid = creature:GetGUID()
    schedule(guid, "fluff", math.random(5000, 12000), function()
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        creature:SetFacingTo(1.53589)
        creature:Talk(0)
    end)
end

local function onBartlebySpawn(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    bartlebyStarted[guid] = false
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

-- C++ npc_crate_helperAI::SpellHit (culling_of_stratholme.cpp:1410-
-- 1443): on SPELL_ARCANE_DISRUPTION 49590, once per helper (the
-- _crateRevealed latch), the instance SetData + GO crate swap legs
-- stay out (no instance/GO-Delete bridge), and the nearest-fluff
-- DoAction leg is delivered directly: the C++ npc_*::Find(me) helpers
-- are FindNearestCreature(entry, radius, true) calls — jena 27885 at
-- 45yd, bartleby 27907 at 5yd, sergeant 27877 at 15yd, roger 27903 at
-- 30yd — and ReplaceIfCloser keeps the strict-closest candidate in
-- that order (first wins ties). Lua GetDistance is the 3D metric vs
-- C++ 2D-squared, same winner on flat Stratholme ground. The
-- no-candidate malcolm Spawn fallback stays out (spline machine).
local ENTRY_CRATE_HELPER = 27827
local ENTRY_JENA = 27885
local ENTRY_ROGER = 27903
local SPELL_ARCANE_DISRUPTION = 49590

local crateRevealed = {}

local function onCrateHelperSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_ARCANE_DISRUPTION then
        return
    end
    local guid = creature:GetGUID()
    if crateRevealed[guid] then
        return
    end
    crateRevealed[guid] = true
    local closest = nil
    local closestDist = nil
    local candidates = {
        { ENTRY_JENA, 45 },
        { ENTRY_BARTLEBY, 5 },
        { ENTRY_SERGEANT, 15 },
        { ENTRY_ROGER, 30 },
    }
    for _, pair in ipairs(candidates) do
        local found = creature:FindNearestCreature(pair[1], pair[2])
        if found then
            local dist = creature:GetDistance(found)
            if closestDist == nil or dist < closestDist then
                closest = found
                closestDist = dist
            end
        end
    end
    if closest == nil then
        return
    end
    local entry = closest:GetEntry()
    if entry == ENTRY_BARTLEBY then
        bartlebyStartFluff(closest)
    elseif entry == ENTRY_SERGEANT then
        moriganStarted[closest:GetGUID()] = true
    elseif entry == ENTRY_ROGER then
        rogerStartFluff(closest)
    end
    -- jena: Started is set but never read in C++ (no observable leg).
    -- roger: only the EVENT_ROGER_START case is ported (stand/facing/
    -- Talk 0); the MOVE1+ spline-chain legs stay out.
end

RegisterCreatureEvent(ENTRY_CRATE_HELPER, 14, onCrateHelperSpellHit)

-- Roger has no spawn behavior in C++ (no InitializeAI); these only
-- cancel a pending fluff timer if roger despawns/leaves combat.
RegisterCreatureEvent(ENTRY_ROGER, 4, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
RegisterCreatureEvent(ENTRY_ROGER, 23, function(event, creature)
    cancelTimers(creature:GetGUID())
end)
