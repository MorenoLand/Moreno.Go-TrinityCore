-- Hellfire Peninsula: Anchorite Barada — Lua port of
-- src/server/scripts/Outland/zone_hellfire_peninsula.cpp
-- (class npc_barada : CreatureScript("npc_barada")).
-- Entry: 22431 (wowhead npc=22431/anchorite-barada, verified
-- 2026-11-07; quest 10935 "The Exorcism of Colonel Jules";
-- no NPC_ constant exists in the C++ file).
-- Eluna creature events: 2 OnLeaveCombat, 4 JustDied, 23 OnReset,
-- 28 OnMovementInform, 38 OnDoAction; gossip event 2 OnGossipSelect.
-- Ported arms:
-- - OnGossipSelect gossipListId 1 (zone_hellfire_peninsula.cpp:582-592):
--   Talk(SAY_BARADA_1 0) + DoAction(ACTION_START_EVENT 1) + gossip-flag
--   removal. Gossip select args (event, player, creature, sender,
--   action, code, listID) per engine/world/gossip.go:359; listID is the
--   C++ gossipListId. Return false mirrors the C++ return (grandmother
--   precedent); menu itself is DB-side (no C++ OnGossipHello override).
-- - DoAction ACTION_START_EVENT (:597-609, event 38): Jules
--   (entry 22432) Talk(SAY_JULES_1 0) + MoveTo(0, exorcismPos[1]) +
--   Talk(SAY_BARADA_2 1) + SetFlag PACIFIED. Bridges: FindNearestCreature
--   (lua_creature_events.go:6131), DoAction (lua_creature_events.go:827),
--   MoveTo (lua_creature_events.go:4875), Talk, SetFlag
--   (lua_creature_events.go:5873).
-- - MovementInform (:611-622, event 28): POINT_MOTION_TYPE 8; id 0 ->
--   MoveTo(1, exorcismPos[1]), id 1 -> EVENT_BARADAS_TALK in 2s.
-- - EVENT_BARADAS_TALK 22-step machine (:644-796): step 0 SetFacingTo
--   (1.513286 = exorcismPos[1] orientation) + kneel + 3s; step 1
--   self-cast SPELL_BARADAS_COMMAND 39277 + 5s; steps 2/6/8/10 Talk
--   SAY_BARADA_3/4; steps 3/5/7/9/12/14/16/18 Jules Talk SAY_JULES_2/3/4/5;
--   step 4 self-cast SPELL_BARADA_FALTERS 39278 + stand + Jules
--   DoAction HOVER 2 + 11s; step 11 Jules DoAction FLIGHT 3 + 10s;
--   step 13/15/17/19 Talk SAY_BARADA_5/6/7; step 20 Jules DoAction
--   MOVE_HOME 4 + RemoveAura VOMITS 39295 + 10s; step 21 Jules
--   `success` latch set + RemoveAllAuras + RemoveAura BARADAS_COMMAND
--   + RemoveFlag PACIFIED + Talk SAY_BARADA_8 7 + EnterEvadeMode +
--   EVENT_RESET in 2min (latch clear); the latch lives in the shared
--   ColonelJulesSuccess table (npc_colonel_jules.lua) and gates the
--   Jules gossip-hello quest credit, ported there.
--   Bridges: SetFacingTo (lua_creature_events.go:4735), PerformEmote
--   (lua_creature_events.go:4090; C++ Unit::HandleEmoteCommand only
--   broadcasts the emote packet, Unit.cpp:1610), CastSpell self-cast,
--   RemoveAura (lua_creature_events.go:1504), EnterEvadeMode
--   (lua_creature_events.go:5051).
-- - JustDied (:626-632, event 4): Jules -> DoAction(MOVE_HOME 4) +
--   RemoveAllAuras(). Bridges as above + RemoveAllAuras
--   (lua_creature_events.go:1548).
-- - Reset (:568-576, event 23): timers cancelled, step cleared,
--   RemoveFlag PACIFIED, gossip flag restored.
-- Documented approximation: C++ resolves Jules via the GUID latched
-- when the exorcism event starts; Lua re-derives the same creature
-- with FindNearestCreature(22432, 20.0) at every step — the identical
-- call the C++ start-event leg uses (zone_hellfire_peninsula.cpp:600).
-- The legs diverge only if Barada dies outside the event while
-- Jules idles within 20 yd; there is no GetCreatureByGUID bridge
-- (only GetPlayerByGUID, runtime.go:1160).
-- Stay-outs (vacuous or unbridged, no observable loss in Lua):
-- - Jules HOVER (2) / FLIGHT (3) arms: Barada sends them faithfully
--   and npc_colonel_jules.lua now handles them (hover + skull-summon +
--   waypoint-chain machine ported 2026-11-10); see that file.
-- - Explicit MoveTargetedHome (step 21): redundant — base
--   CreatureAI::EnterEvadeMode moves home itself (CreatureAI.cpp:233)
--   and resets, which the ev23 handler above mirrors.
-- - playerGUID: write-only in the C++ AI (cleared in Reset, never
--   read), so no Lua state carries it.

local ENTRY_BARADA = 22431
local ENTRY_COLONEL_JULES = 22432

local ACTION_START_EVENT = 1
local ACTION_JULES_HOVER = 2
local ACTION_JULES_FLIGHT = 3
local ACTION_JULES_MOVE_HOME = 4

local SPELL_BARADAS_COMMAND = 39277
local SPELL_BARADA_FALTERS = 39278
local SPELL_JULES_VOMITS_AURA = 39295

local SAY_BARADA_1 = 0
local SAY_BARADA_2 = 1
local SAY_BARADA_3 = 2
local SAY_BARADA_4 = 3
local SAY_BARADA_5 = 4
local SAY_BARADA_6 = 5
local SAY_BARADA_7 = 6
local SAY_BARADA_8 = 7

local SAY_JULES_1 = 0
local SAY_JULES_2 = 1
local SAY_JULES_3 = 2
local SAY_JULES_4 = 3
local SAY_JULES_5 = 4

-- exorcismPos[1], the Barada cast position (zone_hellfire_peninsula.cpp:363).
local EXORCISM_X = -710.731
local EXORCISM_Y = 2749.075
local EXORCISM_Z = 101.592

local EMOTE_ONESHOT_KNEEL = 16
local EMOTE_STAND_STATE_NONE = 470

local POINT_MOTION_TYPE = 8

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_PACIFIED = 0x00020000
local UNIT_NPC_FLAG_GOSSIP = 0x00000001

local timers = {}
local state = {} -- per-GUID: { step = n }

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
    per[key] = CreateLuaEvent(fn, delay, 0)
end

local function findJules(creature)
    return creature:FindNearestCreature(ENTRY_COLONEL_JULES, 20.0)
end

-- C++ EVENT_BARADAS_TALK step machine (zone_hellfire_peninsula.cpp:644-796).
-- Delays below are the per-step re-schedules, in ms.
local function talkStep(creature, guid)
    local st = state[guid]
    if not st then
        return
    end
    local step = st.step
    st.step = step + 1
    local jules
    if step == 0 then
        creature:SetFacingTo(1.513286)
        creature:PerformEmote(EMOTE_ONESHOT_KNEEL)
        schedule(guid, "talk", 3000, function() talkStep(creature, guid) end)
    elseif step == 1 then
        creature:CastSpell(creature, SPELL_BARADAS_COMMAND)
        schedule(guid, "talk", 5000, function() talkStep(creature, guid) end)
    elseif step == 2 then
        creature:Talk(SAY_BARADA_3)
        schedule(guid, "talk", 7000, function() talkStep(creature, guid) end)
    elseif step == 3 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_2)
        end
        schedule(guid, "talk", 18000, function() talkStep(creature, guid) end)
    elseif step == 4 then
        creature:CastSpell(creature, SPELL_BARADA_FALTERS)
        creature:PerformEmote(EMOTE_STAND_STATE_NONE)
        jules = findJules(creature)
        if jules then
            jules:DoAction(ACTION_JULES_HOVER)
        end
        schedule(guid, "talk", 11000, function() talkStep(creature, guid) end)
    elseif step == 5 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_3)
        end
        schedule(guid, "talk", 13000, function() talkStep(creature, guid) end)
    elseif step == 6 then
        creature:Talk(SAY_BARADA_4)
        schedule(guid, "talk", 5000, function() talkStep(creature, guid) end)
    elseif step == 7 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_3)
        end
        schedule(guid, "talk", 13000, function() talkStep(creature, guid) end)
    elseif step == 8 then
        creature:Talk(SAY_BARADA_4)
        schedule(guid, "talk", 12000, function() talkStep(creature, guid) end)
    elseif step == 9 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_4)
        end
        schedule(guid, "talk", 12000, function() talkStep(creature, guid) end)
    elseif step == 10 then
        creature:Talk(SAY_BARADA_4)
        schedule(guid, "talk", 5000, function() talkStep(creature, guid) end)
    elseif step == 11 then
        jules = findJules(creature)
        if jules then
            jules:DoAction(ACTION_JULES_FLIGHT)
        end
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 12 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_4)
        end
        schedule(guid, "talk", 8000, function() talkStep(creature, guid) end)
    elseif step == 13 then
        creature:Talk(SAY_BARADA_5)
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 14 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_4)
        end
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 15 then
        creature:Talk(SAY_BARADA_6)
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 16 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_5)
        end
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 17 then
        creature:Talk(SAY_BARADA_7)
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 18 then
        jules = findJules(creature)
        if jules then
            jules:Talk(SAY_JULES_3)
        end
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 19 then
        creature:Talk(SAY_BARADA_7)
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 20 then
        jules = findJules(creature)
        if jules then
            jules:DoAction(ACTION_JULES_MOVE_HOME)
            jules:RemoveAura(SPELL_JULES_VOMITS_AURA)
        end
        schedule(guid, "talk", 10000, function() talkStep(creature, guid) end)
    elseif step == 21 then
        jules = findJules(creature)
        if jules then
            -- C++ sets the Jules `success` member here; the Lua
            -- latch lives in the shared ColonelJulesSuccess table
            -- (npc_colonel_jules.lua) and gates the gossip-hello
            -- quest credit.
            if ColonelJulesSuccess == nil then
                ColonelJulesSuccess = {}
            end
            ColonelJulesSuccess[jules:GetGUIDLow()] = true
            jules:RemoveAllAuras()
        end
        creature:RemoveAura(SPELL_BARADAS_COMMAND)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED)
        creature:Talk(SAY_BARADA_8)
        creature:EnterEvadeMode()
        state[guid] = nil
        schedule(guid, "reset", 120000, function()
            -- C++ EVENT_RESET (zone_hellfire_peninsula.cpp:799-802):
            -- the Jules `success` latch clears after 2min.
            local j = findJules(creature)
            if j and ColonelJulesSuccess ~= nil then
                ColonelJulesSuccess[j:GetGUIDLow()] = nil
            end
            state[guid] = nil
        end)
    end
end

-- C++ MovementInform (zone_hellfire_peninsula.cpp:611-622).
local function onMovementInform(event, creature, type, id)
    if type ~= POINT_MOTION_TYPE then
        return
    end
    if id == 0 then
        creature:MoveTo(1, EXORCISM_X, EXORCISM_Y, EXORCISM_Z)
    elseif id == 1 then
        local guid = creature:GetGUIDLow()
        schedule(guid, "talk", 2000, function() talkStep(creature, guid) end)
    end
end

-- C++ DoAction ACTION_START_EVENT (zone_hellfire_peninsula.cpp:597-609).
local function onDoAction(event, creature, action)
    if action ~= ACTION_START_EVENT then
        return
    end
    local jules = findJules(creature)
    if jules then
        jules:Talk(SAY_JULES_1)
    end
    creature:MoveTo(0, EXORCISM_X, EXORCISM_Y, EXORCISM_Z)
    creature:Talk(SAY_BARADA_2)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED)
    state[creature:GetGUIDLow()] = { step = 0 }
end

-- C++ OnGossipSelect gossipListId 1 (zone_hellfire_peninsula.cpp:582-592).
local function baradaGossipSelect(event, player, creature, sender, action, code, listID)
    player:GossipClearMenu()
    if listID ~= 1 then
        return false
    end
    player:GossipComplete()
    creature:Talk(SAY_BARADA_1)
    creature:DoAction(ACTION_START_EVENT)
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    return false
end

-- C++ Reset (zone_hellfire_peninsula.cpp:568-576).
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PACIFIED)
    creature:SetNPCFlags(bit32.bor(creature:GetNPCFlags(), UNIT_NPC_FLAG_GOSSIP))
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
end

-- C++ JustDied: Jules gets ACTION_JULES_MOVE_HOME + aura strip.
local function onDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    state[guid] = nil
    local jules = creature:FindNearestCreature(ENTRY_COLONEL_JULES, 20.0)
    if jules then
        jules:DoAction(ACTION_JULES_MOVE_HOME)
        jules:RemoveAllAuras()
    end
end

RegisterCreatureEvent(ENTRY_BARADA, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_BARADA, 4, onDied)
RegisterCreatureEvent(ENTRY_BARADA, 23, onReset)
RegisterCreatureEvent(ENTRY_BARADA, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_BARADA, 38, onDoAction)
RegisterCreatureGossipEvent(ENTRY_BARADA, 2, baradaGossipSelect)
