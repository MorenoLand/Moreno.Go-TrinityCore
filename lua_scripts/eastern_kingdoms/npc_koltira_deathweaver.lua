-- npc_koltira_deathweaver (Scarlet Enclave, chapter2.cpp) --
-- Lua port of the npc_koltira_deathweaver struct in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter2.cpp
-- (lines 105-~300; the Bloody Breakout quest-12727 intro sequence).
-- Entry (verifiable from the C++ sources): chapter2.cpp's own
-- BloodyBreakout enum (line 90) names NPC_KOLTIRA = 28912.
-- Whole-server-tree grep confirms chapter2.cpp as the only source
-- of "npc_koltira_deathweaver" (the loader reference at line 631 is
-- the parent file's bare RegisterCreatureAI call — no
-- CreatureScript wrapper; ScriptName binding is DB-side, per the
-- npc_unworthy_initiate precedent the file-own enum entry is
-- C++-verifiable so the port is registered). Script name is the
-- stringified class name (RegisterCreatureAIWithFactory,
-- ScriptMgr.h:1234 — felblood precedent).
-- Verifiable numbers (file's own enums): SAY_KOLTIRA_0 = 0 /
-- SAY_KOLTIRA_1 = 1; TEXT_ID_EVENT = 13425 (gossip menu);
-- QUEST_BLOODY_BREAKOUT = 12727; EVENT_INTRO_0/EVENT_INTRO_1
-- schedule legs: 500ms then 5s.
-- Bridges used: creature Talk(textId) (nefarian precedent);
-- ON_QUEST_ACCEPT (31) fires live from the Go quest-accept path
-- (quest_details.go -> fireCreatureQuestHook; handler args
-- (event, player, creature, quest), quest.ID field); OnGossipHello
-- (RegisterCreatureGossipEvent, event 1 — handler args (event,
-- player, creature)); GossipSendMenu(0, creature, 13425) with the
-- C++-exact menu id (kalecgos convention); CreateLuaEvent /
-- RemoveEventById timers (unworthy_initiate convention); onReset
-- (23, maiden convention).
-- Ported arms:
-- - OnQuestAccept (quest 12727) -> 500ms EVENT_INTRO_0 ->
--   Talk(SAY_KOLTIRA_0); then 5s EVENT_INTRO_1 ->
--   Talk(SAY_KOLTIRA_1) (C++-exact schedule legs).
-- - OnGossipHello: if the event-gossip state is set, send
--   C++-exact gossip menu TEXT_ID_EVENT (13425); otherwise do
--   nothing — the Go server loads the default (quest) menu when
--   the hook leaves s.gossip unset (gossip.go:82), which matches
--   C++ returning false from OnGossipHello.
-- - OnReset (23): cancel timers + clear the event-gossip state
--   (the C++ Reset's _events.Reset() / _eventGossip=false residue;
--   flag/stand-state/aura/summon arms unbridged, below).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - OnGossipHello's PrepareQuestMenu leg (no quest-menu bridge —
--   kalecgos precedent; IsQuestGiver() is creature_template data).
-- - Reset's SetFlag(UNIT_FLAG_IMMUNE_TO_NPC) /
--   SetFlag(UNIT_NPC_FLAG_GOSSIP) (no flag bridges — unworthy_
--   initiate precedent), SetByteValue stand-state legs (no
--   stand-state bridge), RemoveAllAuras (no aura-removal bridge —
--   eye_of_acherus precedent), _summons.DespawnAll (no summon/
--   despawn bridge — dark-rider precedent).
-- - EVENT_INTRO_0's SetByteValue(SIT) + RemoveFlag(GOSSIP) arms
--   and EVENT_INTRO_1's SetByteValue(STAND) arm (stand-state /
--   flag bridges, as above) — only the Talk legs are ported.
-- - EVENT_INTRO_2's MoveJump + EVENT_INTRO_3's MovePoint (no
--   MotionMaster bridge — selin_fireheart precedent); the intro
--   chain stalls here in the port — the timers are NOT ported
--   (firing them would be dead code / a fabricated trigger).
-- - MovementInform(POINT_ID_1/POINT_ID_2) (no MovementInform
--   bridge — kalecgos precedent), stranding EVENT_SPAWN_WAVE_1,
--   EVENT_INTRO_4 (DoCastSelf 52899 / LoadEquipment / MovePoint),
--   EVENT_INTRO_5 (Talk 2 / DoCastSelf 52894) and EVENT_INTRO_6
--   (Talk 3 / kneel / gossip flag / _eventGossip=true).
-- - FakeValrothTalk (no FindNearestCreature / ObjectAccessor
--   bridge — dark-rider precedent).
-- - SummonCreatureGroup waves 1-3 + valroth (no summon-group
--   bridge); EVENT_KOLTIRA_ADVICE (stranded on the summon list);
--   JustSummoned / SummonedCreatureDespawn (no summon-list model;
--   SummonedCreatureDespawn's EVENT_OUTRO_1 trigger has no
--   bridge).
-- - EVENT_OUTRO_1..4 (RemoveAurasDueToSpell 52894 / gossip flag /
--   MovePath(NPC_KOLTIRA) — no aura-removal / flag / MotionMaster
--   bridges; DoCastSelf 53627's trigger is stranded).
-- - EVENT_CHECK_PLAYER (no ObjectAccessor player-GUID bridge —
--   dark-rider precedent; no DespawnOrUnsummon bridge; no
--   FailQuest / quest-status bridge — death_knight_initiate
--   precedent).
-- - MovementInform's POINT_ID_6 Mount(NPC_KOLTIRA_MOUNT 25445)
--   arm (no MovementInform / mount bridge).
-- Documented deviations: _eventGossip is set true only by the
-- stranded EVENT_INTRO_6, so the OnGossipHello 13425 branch never
-- fires on the current surface — it is the genuine C++ arm (not a
-- stub), kept so the handler stays faithful if the MovementInform
-- bridge ever lands. The C++ intro chain is timer-driven from
-- OnQuestAccept (500ms -> 5s); the port mirrors those exact legs
-- and stops where the C++ chain leaves the timer domain.
local ENTRY_KOLTIRA = 28912
local QUEST_BLOODY_BREAKOUT = 12727
local SAY_KOLTIRA_0 = 0
local SAY_KOLTIRA_1 = 1
local TEXT_ID_EVENT = 13425

local timers = {}
local eventGossip = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    eventGossip[guid] = nil
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

local function onIntro1(creature)
    creature:Talk(SAY_KOLTIRA_1)
end

local function onIntro0(creature, guid)
    creature:Talk(SAY_KOLTIRA_0)
    schedule(guid, "intro1", 5000, function() onIntro1(creature) end)
end

local function onQuestAccept(event, player, creature, quest)
    if quest.ID ~= QUEST_BLOODY_BREAKOUT then
        return
    end
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "intro0", 500, function() onIntro0(creature, guid) end)
end

local function onGossipHello(event, player, creature)
    if eventGossip[creature:GetGUID()] then
        player:GossipSendMenu(0, creature, TEXT_ID_EVENT)
    end
end

local function onReset(_, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KOLTIRA, 31, onQuestAccept)
RegisterCreatureGossipEvent(ENTRY_KOLTIRA, 1, onGossipHello)
RegisterCreatureEvent(ENTRY_KOLTIRA, 23, onReset)
