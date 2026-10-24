-- Storm Peaks: npc_brann_bronzebeard_keystone — Lua port of
-- src/server/scripts/Northrend/zone_storm_peaks.cpp
-- (npc_brann_bronzebeard_keystone : ScriptedAI, quest 13285
-- "Forging the Keystone"). Entry: NPC_BRANN_BRONZEBEARD 31810.
-- Ported: OnGossipSelect (zone_storm_peaks.cpp:496-502) -> event 2.
-- C++ checks no sender/action, so any select closes the menu and
-- schedules EVENT_SCRIPT_1 at 100ms; the Lua handler does the same.
-- The 13-step EventMap chain (zone_storm_peaks.cpp:508-613) rides
-- chained CreateLuaEvent single-shots at C++-exact delays: step 1
-- at 100ms, step 2 at 4s, steps 3-12 at 6s each, step 13 at 10s
-- (total ~74s). Per-GUID state mirrors the C++ GUID fields
-- (playerGUID, voiceGUID, objectGUID[5], objectCounter); the voice
-- is kept as the summoned creature object, the player re-resolved
-- per step via GetPlayerByGUID (nil-guarded, matching the C++
-- ObjectAccessor guards).
-- Ported per step: 1 Talk(SAY_BRANN_1 0, player) + NPC-flag
-- removal (GOSSIP 0x1 | QUESTGIVER 0x2 via the zulaman
-- GetNPCFlags/SetNPCFlags twin) + SummonCreature voice 31814
-- (TEMPSUMMON_TIMED_OR_DEAD_DESPAWN = type 0, 49000ms — the voice
-- is still alive at step 9, ~46s in); 3 EmoteState mining 233;
-- 4 EmoteState none 0 + voice self-cast resurrection 58854 +
-- Talk(SAY_VOICE_1 0, player) + SummonGameObject signal 193590;
-- 5-8 voice Talks 1-4 + signals 193591-193594; 9 voice self-cast
-- 58854; 10 Talk(SAY_BRANN_2 1, player) + KilledMonsterCredit
-- (31810); 11 SetFacingTo(2.932153) + Talk(SAY_BRANN_3 2, player);
-- 13 DisappearAndDie via the halazzi Despawn twin.
-- Stays out, documented: steps 2 and 12 are movement-only
-- (SetWalk + MovePoint) and MovePoint/SetWalk have no bridge on
-- the creature event surface (storm_cloud precedent) — those steps
-- are pure delays preserving step timing; the MotionMaster Clear
-- in step 12 likewise has no bridge. Step 11's GameObject Delete
-- leg stays out: SummonGameObject returns a GUID and no
-- GO-by-GUID lookup bridge exists, so the five signals persist
-- past step 11 instead of despawning (their summon legs port).
-- The C++ signal orientations are carried as the Lua o argument
-- (the quaternion leg has no Lua twin). OnDied(4)/OnReset(23)
-- cancel the chain and drop state, the twin of the C++ Reset
-- GUID wipe (C++ does not clear the EventMap in Reset, but
-- post-wipe steps are no-ops except the unguarded summon/flag
-- legs, so cancel is the safe twin).

local ENTRY_BRANN = 31810
local ENTRY_VOICE = 31814
local SPELL_RESURRECTION = 58854

local UNIT_NPC_FLAG_GOSSIP = 0x00000001
local UNIT_NPC_FLAG_QUESTGIVER = 0x00000002
local EMOTE_STATE_WORK_MINING = 233
local EMOTE_ONESHOT_NONE = 0

local SAY_BRANN_1 = 0
local SAY_BRANN_2 = 1
local SAY_BRANN_3 = 2

local VOICE_X, VOICE_Y, VOICE_Z, VOICE_O = 7863.43, -1396.585, 1538.076, 2.949606

local SIGNALS = {
    { 193590, 7860.273, -1383.622, 1538.302, -1.658062 },
    { 193591, 7875.67, -1387.266, 1538.323, -2.373644 },
    { 193592, 7879.212, -1401.175, 1538.279, 2.967041 },
    { 193593, 7868.944, -1411.18, 1538.213, 2.111848 },
    { 193594, 7855.11, -1406.839, 1538.42, 1.151916 },
}

local brannState = {}
local brannTimer = {}

local function cancelChain(guid)
    local id = brannTimer[guid]
    if id then
        RemoveEventById(id)
        brannTimer[guid] = nil
    end
    brannState[guid] = nil
end

local runStep

local function scheduleStep(creature, guid, step, delayMs)
    brannTimer[guid] = CreateLuaEvent(function()
        brannTimer[guid] = nil
        runStep(creature, guid, step)
    end, delayMs)
end

local function stepPlayer(st)
    if not st or not st.playerGUID then
        return nil
    end
    return GetPlayerByGUID(st.playerGUID)
end

runStep = function(creature, guid, step)
    local st = brannState[guid]
    if not st then
        return
    end
    if step == 1 then
        local player = stepPlayer(st)
        if player then
            creature:Talk(SAY_BRANN_1, player)
        end
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(),
            bit32.bnot(bit32.bor(UNIT_NPC_FLAG_GOSSIP,
                UNIT_NPC_FLAG_QUESTGIVER))))
        st.voice = creature:SummonCreature(ENTRY_VOICE, VOICE_X, VOICE_Y,
            VOICE_Z, VOICE_O, 0, 49000)
        scheduleStep(creature, guid, 2, 4000)
    elseif step == 2 then
        -- SetWalk(true) + MovePoint: no movement bridge, delay only.
        scheduleStep(creature, guid, 3, 6000)
    elseif step == 3 then
        creature:EmoteState(EMOTE_STATE_WORK_MINING)
        scheduleStep(creature, guid, 4, 6000)
    elseif step == 4 then
        creature:EmoteState(EMOTE_ONESHOT_NONE)
        local player = stepPlayer(st)
        if st.voice then
            st.voice:CastSpell(st.voice, SPELL_RESURRECTION)
            if player then
                st.voice:Talk(0, player)
            end
        end
        local sig = SIGNALS[1]
        creature:SummonGameObject(sig[1], sig[2], sig[3], sig[4], sig[5], 0)
        scheduleStep(creature, guid, 5, 6000)
    elseif step == 5 or step == 6 or step == 7 or step == 8 then
        local idx = step - 4
        local player = stepPlayer(st)
        if st.voice and player then
            st.voice:Talk(idx, player)
        end
        local sig = SIGNALS[idx + 1]
        creature:SummonGameObject(sig[1], sig[2], sig[3], sig[4], sig[5], 0)
        scheduleStep(creature, guid, step + 1, 6000)
    elseif step == 9 then
        if st.voice then
            st.voice:CastSpell(st.voice, SPELL_RESURRECTION)
        end
        scheduleStep(creature, guid, 10, 6000)
    elseif step == 10 then
        local player = stepPlayer(st)
        if player then
            creature:Talk(SAY_BRANN_2, player)
            player:KilledMonsterCredit(ENTRY_BRANN)
        end
        scheduleStep(creature, guid, 11, 6000)
    elseif step == 11 then
        creature:SetFacingTo(2.932153)
        local player = stepPlayer(st)
        if player then
            creature:Talk(SAY_BRANN_3, player)
        end
        -- Signal Delete leg stays out: no GO-by-GUID bridge.
        scheduleStep(creature, guid, 12, 6000)
    elseif step == 12 then
        -- MotionMaster Clear + SetWalk(false) + MovePoint: no
        -- movement bridge, delay only.
        scheduleStep(creature, guid, 13, 10000)
    elseif step == 13 then
        brannState[guid] = nil
        creature:Despawn()
    end
end

local function brannGossipSelect(event, player, creature)
    player:GossipComplete()
    local guid = creature:GetGUID()
    cancelChain(guid)
    brannState[guid] = { playerGUID = player:GetGUID() }
    scheduleStep(creature, guid, 1, 100)
end

local function brannCancel(_, creature)
    cancelChain(creature:GetGUID())
end

RegisterCreatureGossipEvent(ENTRY_BRANN, 2, brannGossipSelect)
RegisterCreatureEvent(ENTRY_BRANN, 4, brannCancel)
RegisterCreatureEvent(ENTRY_BRANN, 23, brannCancel)
