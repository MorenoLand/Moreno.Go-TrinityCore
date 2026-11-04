-- Shenthul (Orgrimmar, Cleft of Shadow) -- Lua port of
-- src/server/scripts/Kalimdor/zone_orgrimmar.cpp
-- (npc_shenthul : public CreatureScript("npc_shenthul");
-- struct npc_shenthulAI : public ScriptedAI).
-- Entry: 3401 (Wowhead Classic NPC page npc=3401, Shenthul <Rogue
-- Trainer> in Orgrimmar; no NPC_SHENTHUL constant exists in the C++
-- tree — the creature_template ScriptName binding is DB-side, the
-- wilda precedent: wowhead-verified entry, no in-tree constant).
-- Eluna creature events: 31 OnQuestAccept, 8 OnReceiveEmote,
-- 5 OnSpawn, 23 OnReset, 4 OnDied.
-- Ported arms (C++-exact for all modeled arms):
-- - OnQuestAccept (event 31): quest 2460 (QUEST_SHATTERED_SALUTE)
--   -> CanTalk = true + store the player (C++ order: CanTalk,
--   PlayerGUID; the timers are untouched, so an accept mid-cycle
--   keeps the running countdown, exactly like C++).
-- - UpdateAI poll (1s CreateLuaEvent, seeded on spawn/reset,
--   cancel-first): CanTalk && !CanEmote -> 6s -> PerformEmote(66
--   EMOTE_ONESHOT_SALUTE) + CanEmote + 60s window; CanEmote ->
--   window expiry -> FailQuest if the stored player still has
--   quest 2460 incomplete + Reset. C++ UpdateAI ticks outside
--   combat (the victim check is last), so the poll runs from
--   spawn, not from enter-combat (spirit_guide idiom).
-- - ReceiveEmote (event 8): salute (78 TEXT_EMOTE_SALUTE) +
--   quest 2460 incomplete + CanEmote -> AreaExploredOrEventHappens
--   + Reset. C++ does NOT gate on the stored player here, so
--   neither does this port: any incomplete-quest player saluting
--   during the window earns credit.
-- Notes: the stored player object is the fizzule precedent (a
-- handler object kept across CreateLuaEvent ticks); the FailQuest
-- twin is a no-op unless the quest is incomplete, so the only
-- divergence from C++ is the logged-out-player corner (C++
-- ObjectAccessor::GetPlayer returns nil offline, while the stored
-- session handle may still fail the quest). Melee is
-- engine-driven in Go; C++ JustEngagedWith is empty.

local ENTRY_SHENTHUL = 3401

local QUEST_SHATTERED_SALUTE = 2460
local QUEST_STATUS_INCOMPLETE = 3
local TEXT_EMOTE_SALUTE = 78
local EMOTE_ONESHOT_SALUTE = 66

local SALUTE_DELAY = 6000
local SALUTE_WINDOW = 60000
local POLL_MS = 1000

local pollTimer = {}
local state = {}

local function cancelPoll(guid)
    local id = pollTimer[guid]
    if id then
        RemoveEventById(id)
        pollTimer[guid] = nil
    end
end

local function resetState(guid)
    state[guid] = {
        canTalk = false,
        canEmote = false,
        saluteTimer = SALUTE_DELAY,
        resetTimer = 0,
        player = nil,
    }
end

local function startPoll(creature)
    local guid = creature:GetGUID()
    cancelPoll(guid)
    resetState(guid)
    local function tick()
        if creature:IsDead() then
            pollTimer[guid] = nil
            return
        end
        local st = state[guid]
        if not st then
            pollTimer[guid] = nil
            return
        end
        if st.canEmote then
            if st.resetTimer <= POLL_MS then
                local p = st.player
                if p and p:GetQuestStatus(QUEST_SHATTERED_SALUTE) == QUEST_STATUS_INCOMPLETE then
                    p:FailQuest(QUEST_SHATTERED_SALUTE)
                end
                resetState(guid)
            else
                st.resetTimer = st.resetTimer - POLL_MS
            end
        end
        st = state[guid]
        if st.canTalk and not st.canEmote then
            if st.saluteTimer <= POLL_MS then
                creature:PerformEmote(EMOTE_ONESHOT_SALUTE)
                st.canEmote = true
                st.resetTimer = SALUTE_WINDOW
            else
                st.saluteTimer = st.saluteTimer - POLL_MS
            end
        end
        pollTimer[guid] = CreateLuaEvent(tick, POLL_MS)
    end
    pollTimer[guid] = CreateLuaEvent(tick, POLL_MS)
end

local function shenthulSpawnOrReset(event, creature)
    startPoll(creature)
end

local function shenthulDied(event, creature)
    cancelPoll(creature:GetGUID())
    state[creature:GetGUID()] = nil
end

local function shenthulQuestAccept(event, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_SHATTERED_SALUTE then
        return
    end
    local guid = creature:GetGUID()
    local st = state[guid]
    if not st then
        return
    end
    st.canTalk = true
    st.player = player
end

local function shenthulReceiveEmote(event, creature, player, textEmote)
    local st = state[creature:GetGUID()]
    if not st then
        return
    end
    if textEmote == TEXT_EMOTE_SALUTE
        and player:GetQuestStatus(QUEST_SHATTERED_SALUTE) == QUEST_STATUS_INCOMPLETE
        and st.canEmote then
        player:AreaExploredOrEventHappens(QUEST_SHATTERED_SALUTE)
        resetState(creature:GetGUID())
    end
end

RegisterCreatureEvent(ENTRY_SHENTHUL, 5, shenthulSpawnOrReset)
RegisterCreatureEvent(ENTRY_SHENTHUL, 23, shenthulSpawnOrReset)
RegisterCreatureEvent(ENTRY_SHENTHUL, 4, shenthulDied)
RegisterCreatureEvent(ENTRY_SHENTHUL, 31, shenthulQuestAccept)
RegisterCreatureEvent(ENTRY_SHENTHUL, 8, shenthulReceiveEmote)
