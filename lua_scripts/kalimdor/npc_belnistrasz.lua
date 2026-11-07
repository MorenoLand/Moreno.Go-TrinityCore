-- Razorfen Downs: Belnistrasz (quest 3525 "Extinguishing the Idol") — Lua
-- port of src/server/scripts/Kalimdor/RazorfenDowns/razorfen_downs.cpp
-- (class npc_belnistrasz : CreatureScript("npc_belnistrasz") {
-- npc_belnistraszAI : public ScriptedAI }; GetAI via
-- GetRazorfenDownsAI<npc_belnistraszAI> (razorfen_downs.h —
-- GetInstanceAI<AI>(obj, "instance_razorfen_downs") gate — the
-- instance-side legs have no bridge, standing). AddSC_razorfen_downs at
-- end registers four zone scripts (kalimdor_script_loader.cpp decl :73 /
-- call :186, the "// Razorfen Downs" loader block); of the four,
-- npc_tomb_creature was already ported (lua_scripts/kalimdor/
-- npc_tomb_creature.lua), npc_idol_room_spawner and go_gong have zero
-- bridgeable arms (documented-only below) — this file covers
-- npc_belnistrasz only.
-- Entry: 8516 wowhead-verified (classic.wowhead.com/npc=8516/belnistrasz;
-- no NPC_ constant in the C++ tree — landslide/noxxion/ptheradras
-- precedent); creature_text rows back Talk ids 0-7.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset, 31 OnQuestAccept (engine/world/quest_details.go fires it
-- as (event, player, creature, quest); quest:GetId() wired —
-- luaQuest surface, lua_creature_events.go). Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms:
-- - Reset (C++ Reset, event 23): only the !eventInProgress branch is
--   reachable in this port (eventInProgress latches true on quest
--   accept) — DoCastSelf(SPELL_ARCANE_INTELLECT 13326) under the C++
--   HasAura guard (non-triggered self-cast); local event/channeling
--   state re-armed, timers cleared; the gossip-questgiver flag re-set
--   (SetFlag(UNIT_NPC_FLAGS, 82, 2)) IS bridged and ported.
-- - OnQuestAccept (C++ OnQuestAccept, event 31): quest id 3525
--   (QUEST_EXTINGUISHING_THE_IDOL) -> eventInProgress latch,
--   Talk(SAY_QUEST_ACCEPTED 0); the RemoveFlag questgiver leg IS
--   bridged (RemoveFlag(82, 2)) and ported, as is SetFaction(250
--   FACTION_ESCORTEE_N_NEUTRAL_ACTIVE, C++ order: latch, Talk,
--   RemoveFlag, SetFaction, MovePath). MovePath(PATH_ESCORT 871710)
--   alone has no bridge — documented-only.
-- - Engage (C++ JustEngagedWith): arms Fireball at 1s and Frost Nova
--   at {8s,12s}, and with urand(0,100) > 40 -> Talk(SAY_AGGRO 6);
--   channeling (never set here — MovementInform has no bridge) ->
--   Talk(SAY_WATCH_OUT 7) (C++ targeted-player Talk broadcasts here;
--   deviation, documented). C++ UpdateAI gates ALL combat events on
--   eventInProgress (UpdateAI opens `if (!eventInProgress) return;`),
--   so a pre-quest-3525 engage produces zero casts: the fire legs
--   below re-schedule their frozen lines without casting until the
--   quest-accept latch is set. Timers re-armed on engage but the latch
--   is never wiped (C++ Reset skips the latch once set).
-- - Fireball (EVENT_FIREBALL): DoCastVictim(SPELL_FIREBALL 9053),
--   non-triggered (2-arg DoCastVictim); init 1s -> repeat 8s; gated on
--   the quest-accept latch (C++ UpdateAI !eventInProgress early
--   return): frozen lines re-schedule without casting. CASTING/
--   victimless gate PORTED as a drop (razorfen_downs.cpp:238-241,
--   post-ExecuteEvent return consumes the event — NOT the 500ms
--   defer used for pre-loop gates, where the event survives).
-- - Frost Nova (EVENT_FROST_NOVA): DoCastAOE(SPELL_FROST_NOVA 11831)
--   resolves to self-cast (kazrogal/illidan precedent), non-triggered;
--   init {8s,12s} -> repeat 15s; same quest-latch gate; CASTING/
--   victimless gate PORTED as a drop (razorfen_downs.cpp:244-247).
-- - LeaveCombat (event 2): cancel timers (C++ UpdateAI early-return
--   without victim never clears scheduled events, but the hyjal.lua
--   convention clears timers on leave-combat; no timer re-arm
--   semantics change).
-- - Died (event 4): clear timers + state; the 5s DespawnOrUnsummon leg
--   ports via the freya delayed-despawn idiom (CreateLuaEvent one-shot
--   -> creature:Despawn, bridged); the instance
--   SetBossState(DATA_EXTINGUISHING_THE_IDOL 6, DONE) leg stays out —
--   NOT for lack of a bridge (creature:SetBossState is live,
--   lua_creature_events.go:5917) but because C++ drops the write:
--   EncounterCount = 5, so id 6 >= bosses.size() 5 and
--   InstanceScript::SetBossState returns false (InstanceScript.cpp).
--   Writing it from Lua would fabricate state C++ never has — see
--   instance_razorfen_downs.lua.
-- Unmodeled (documented-only, no bridges):
-- - MovementInform WAYPOINT id 17 (channeling start), the escort
--   MovePath machine, EVENT_CHANNEL/IDOL_ROOM_SPAWNER/PROGRESS/
--   COMPLETE chain (SummonCreature NPC_IDOL_ROOM_SPAWNER 8611 with
--   SetData spawner-count, Talk progress lines 1-5, idol-shutdown
--   visual 12774, camera shake 12816, brazier GO summon 152097,
--   GroupEventHappens credit, idol-fire GO deletes 151951/151952/
--   151973, InterruptSpell channeled) — the whole idol-ritual machine
--   needs MovementInform + summon + quest-credit + GO-summon bridges.
-- - npc_idol_room_spawner ("npc_idol_room_spawner"): its only arm is
--   SetData summoning 4 trash entries (Withered Battle Boar 7333,
--   Death's Head Geomancer 7335, Withered Quilguard 7329,
--   Plaguemaw the Rotting 7356) — zero bridges, no registration.
-- - go_gong ("go_gong", GO 148917): OnGossipHello PORTED below
--   (SendCustomAnim(0) + the instance SetData(DATA_WAVE 5, IN_PROGRESS)
--   wave machine via the shared instanceData table — summons, gong
--   flag, gongWave counter; see the handler).

local ENTRY_BELNISTRASZ = 8516
local GO_GONG = 148917 -- razorfen_downs.cpp:373 (go_gong)

local SAY_QUEST_ACCEPTED = 0
local SAY_AGGRO          = 6
local SAY_WATCH_OUT      = 7

local QUEST_EXTINGUISHING_THE_IDOL = 3525

local SPELL_ARCANE_INTELLECT = 13326

-- GossipDef.h:36: GOSSIP_OPTION_QUESTGIVER = 2 (UNIT_NPC_FLAG_QUESTGIVER).
-- Field 82 = UNIT_NPC_FLAGS (UpdateFields.h:139).
local UNIT_NPC_FLAG_QUESTGIVER = 2
local SPELL_FIREBALL         = 9053
local SPELL_FROST_NOVA       = 11831

local timers = {}
local state  = {}

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { eventInProgress = false, channeling = false }
        state[guid] = st
    end
    return st
end

local function clearState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

-- C++ EVENT_FIREBALL: DoCastVictim(9053); re-arm 8s. Gate is PORTED:
-- razorfen_downs.cpp:238-241 checks HasUnitState(UNIT_STATE_CASTING)
-- || !UpdateVictim() AFTER ExecuteEvent pops the event, so a gated
-- fire returns WITHOUT re-arming and the event is LOST (unlike the
-- mal_ganis/BossAI pre-loop gate, where the queued event survives and
-- the 500ms defer preserves it). The drop below is C++-exact.
local function onFireball(creature, guid)
    -- C++ UpdateAI never ticks events while !eventInProgress
    -- (razorfen_downs.cpp:154-157): the 1s initial stays frozen, so the
    -- frozen leg re-arms with the initial, not the 8s repeat — the
    -- fireball fires ~1s after the quest-accept latch, like C++.
    if not getState(guid).eventInProgress then
        schedule(guid, "fireball", 1000, function()
            onFireball(creature, guid)
        end)
        return
    end
    -- Post-ExecuteEvent gate (razorfen_downs.cpp:238-241): casting or
    -- victimless -> return with NO re-arm; the event is consumed.
    if creature:IsCasting() then
        return
    end
    local victim = creature:GetVictim()
    if not victim then
        return
    end
    creature:CastSpell(victim, SPELL_FIREBALL)
    schedule(guid, "fireball", 8000, function()
        onFireball(creature, guid)
    end)
end

-- C++ EVENT_FROST_NOVA: DoCastAOE(11831) -> self-cast (kazrogal/illidan
-- precedent); re-arm 15s. Same post-ExecuteEvent gate as Fireball
-- (razorfen_downs.cpp:244-247): casting or victimless -> drop, no
-- re-arm; the self-cast still needs a live victim to fire at all.
local function onFrostNova(creature, guid)
    -- Frozen leg: re-arm with the rolled {8s,12s} initial (see engage),
    -- not the 15s repeat — C++ fires the frozen initial 8-12s after the
    -- quest-accept latch (razorfen_downs.cpp:154-157).
    if not getState(guid).eventInProgress then
        schedule(guid, "frostnova", getState(guid).frostNovaInit or 8000,
            function()
                onFrostNova(creature, guid)
            end)
        return
    end
    -- Post-ExecuteEvent gate (razorfen_downs.cpp:244-247): casting or
    -- victimless -> return with NO re-arm; the event is consumed.
    if creature:IsCasting() or not creature:GetVictim() then
        return
    end
    creature:CastSpell(creature, SPELL_FROST_NOVA)
    schedule(guid, "frostnova", 15000, function()
        onFrostNova(creature, guid)
    end)
end

-- C++ JustEngagedWith: channeling -> Talk(WATCH_OUT); else arm the two
-- combat timers + 60% aggro talk (Talk broadcasts; the C++ targeted-
-- player variant has no bridge).
local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    -- C++ Reset() skips everything once eventInProgress latches true,
    -- so the quest-accept latch survives engage/evade; never wipe state
    -- here (C++ JustEngagedWith re-arms the timers unconditionally).
    cancelTimers(guid)
    local st = getState(guid)
    if st.channeling then
        creature:Talk(SAY_WATCH_OUT)
        return
    end
    schedule(guid, "fireball", 1000, function()
        onFireball(creature, guid)
    end)
    -- The {8s,12s} initial is rolled once here and reused by the frozen
    -- leg below, matching C++'s frozen-timer semantics
    -- (razorfen_downs.cpp:154-157).
    st.frostNovaInit = math.random(8000, 12000)
    schedule(guid, "frostnova", st.frostNovaInit, function()
        onFrostNova(creature, guid)
    end)
    if math.random(0, 100) > 40 then
        creature:Talk(SAY_AGGRO)
    end
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Reset: only the !eventInProgress branch is reachable here —
-- refresh Arcane Intellect under the C++ HasAura guard; re-arm local
-- event/channeling state (the gossip-questgiver flag re-set has no
-- bridge).
local function onReset(event, creature)
    local guid = creature:GetGUID()
    local st = getState(guid)
    cancelTimers(guid)
    if not st.eventInProgress then
        -- C++ npc_belnistraszAI::Reset(): DoCastSelf(ARCANE_INTELLECT)
        -- is Reset-only (razorfen_downs.cpp). Alive-gated; the timer
        -- cancel and latch reset ride the death leg deliberately.
        if creature:IsAlive() then
            if not creature:HasAura(SPELL_ARCANE_INTELLECT) then
                creature:CastSpell(creature, SPELL_ARCANE_INTELLECT)
            end
        end
        st.channeling = false
        -- C++ npc_belnistraszAI::Reset (razorfen_downs.cpp:108):
        -- me->SetFlag(UNIT_NPC_FLAGS, GOSSIP_OPTION_QUESTGIVER) — the
        -- questgiver service flag is re-armed on reset while the event
        -- has not started (the !eventInProgress gate above).
        creature:SetFlag(82, UNIT_NPC_FLAG_QUESTGIVER)
    end
end

-- C++ OnQuestAccept for quest 3525: latch the event + Talk(0); the
-- escort start (gossip-flag removal, faction swap, MovePath 871710)
-- has no bridges.
local function onQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_EXTINGUISHING_THE_IDOL then
        return
    end
    local st = getState(creature:GetGUID())
    st.eventInProgress = true
    creature:Talk(SAY_QUEST_ACCEPTED)
    -- C++ npc_belnistraszAI::OnQuestAccept (razorfen_downs.cpp:137):
    -- me->RemoveFlag(UNIT_NPC_FLAGS, GOSSIP_OPTION_QUESTGIVER) — the
    -- questgiver flag goes away once the escort event starts; the
    -- MovePath leg alone has no bridge — documented-only.
    creature:RemoveFlag(82, UNIT_NPC_FLAG_QUESTGIVER)
    creature:SetFaction(250)
end

local function onDied(event, creature, killer)
    clearState(creature:GetGUID())
    -- C++ npc_belnistraszAI::JustDied (razorfen_downs.cpp:125-128):
    -- instance->SetBossState(DATA_EXTINGUISHING_THE_IDOL, DONE) is a
    -- dead write in C++ (id 6 >= bosses.size() 5, EncounterCount = 5 —
    -- InstanceScript::SetBossState returns false; see header), so it
    -- is NOT ported despite the live creature:SetBossState bridge;
    -- me->DespawnOrUnsummon(5s) ports via the freya delayed-despawn
    -- idiom (CreateLuaEvent one-shot).
    CreateLuaEvent(function()
        creature:Despawn()
    end, 5000)
end

RegisterCreatureEvent(ENTRY_BELNISTRASZ, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 4, onDied)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 23, onReset)
RegisterCreatureEvent(ENTRY_BELNISTRASZ, 31, onQuestAccept)

-- C++ go_gong::OnGossipHello (razorfen_downs.cpp:383-389): SendCustomAnim(0)
-- + instance->SetData(DATA_WAVE, IN_PROGRESS) — the gong wave machine
-- (instance_razorfen_downs.cpp:109-190), via the shared instanceData
-- table: the gong goes NOT_SELECTABLE, wave gongWave summons Tomb
-- Fiends (wave 0, 10x at PosSummonTutenkash[0..9]) / Tomb Reavers
-- (wave 1, 4x at [10..13]) / Tuten'kash (wave 2, 1x at [14]), each
-- MovePoint(0, 2533.479±5, 870.020±5, 47.678); gongWave++. The go
-- object is stashed as instanceData.gong (the C++ OnGameObjectCreate
-- latch twin — the static gong never fires the Go create hook) for
-- the tomb-creature death counter in npc_tomb_creature.lua. C++
-- returns true (handled) == Lua true. Summons use the C++ default
-- (WorldObject::SummonCreature, MANUAL_DESPAWN = Go type 7 default).
local NPC_TOMB_FIEND = 7349 -- razorfen_downs.h:38
local NPC_TOMB_REAVER = 7351 -- razorfen_downs.h:39
local NPC_TUTEN_KASH = 7355 -- razorfen_downs.h:40
local GAMEOBJECT_FLAGS = 9 -- UpdateFields.h: OBJECT_END 6 + 3 (arlokk precedent)
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641

-- PosSummonTutenkash, instance_razorfen_downs.cpp:28-44 (verbatim).
local POS_SUMMON_TUTENKASH = {
    { 2487.339, 805.9111, 43.08361, 2.844887 },
    { 2485.405, 804.1145, 43.68511, 3.054326 },
    { 2488.431, 801.2809, 42.70374, 4.29351 },
    { 2489.914, 804.7949, 43.25175, 1.658063 },
    { 2541.246, 907.0941, 46.64201, 2.024582 },
    { 2544.701, 907.6331, 46.38007, 1.605703 },
    { 2541.49, 911.1756, 46.26493, 4.817109 },
    { 2544.693, 912.8887, 46.39912, 2.129302 },
    { 2524.036, 834.4852, 48.37031, 0.8028514 },
    { 2527.017, 829.9793, 48.06498, 0.6981317 },
    { 2542.818, 904.9359, 46.80911, 4.642576 },
    { 2543.287, 911.2448, 46.32785, 0.6806784 },
    { 2489.083, 806.5914, 43.21102, 3.682645 },
    { 2486.828, 802.8737, 43.19883, 2.9147 },
    { 2487.939, 804.2224, 43.10735, 1.692969 },
}

local function onGongGossip(_, player, go)
    go:SendCustomAnim(0)
    local mapObj = go:GetMap()
    local data = nil
    if mapObj then
        data = mapObj:GetInstanceData()
    end
    if data == nil then
        return true
    end
    data.gong = go
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    local wave = data.gongWave or 0
    -- C++ switch (gongWave) sets the summon range/entry, then the
    -- summon loop runs over the stored members; wave >= 3 reuses the
    -- stale wave-2 members (unreachable — the gong stays
    -- NOT_SELECTABLE after wave 2, Tuten'kash's death never re-arms
    -- it), mirrored here by persisting the members in instanceData.
    local low, high, entry = data.summonLowRange or 0, data.summonHighRange or 0, data.summonCreature or 0
    if wave == 0 then
        low, high, entry = 0, 10, NPC_TOMB_FIEND
    elseif wave == 1 then
        low, high, entry = 10, 14, NPC_TOMB_REAVER
    elseif wave == 2 then
        low, high, entry = 14, 15, NPC_TUTEN_KASH
    end
    data.summonLowRange, data.summonHighRange, data.summonCreature = low, high, entry
    for i = low, high - 1 do
        local pos = POS_SUMMON_TUTENKASH[i + 1]
        local summoned = go:SummonCreature(entry, pos[1], pos[2], pos[3], pos[4])
        if summoned ~= nil then
            summoned:MoveTo(0, 2533.479 + math.random(-5, 5), 870.020 + math.random(-5, 5), 47.678)
        end
    end
    data.gongWave = wave + 1
    return true
end

RegisterGameObjectGossipEvent(GO_GONG, 1, onGongGossip)
