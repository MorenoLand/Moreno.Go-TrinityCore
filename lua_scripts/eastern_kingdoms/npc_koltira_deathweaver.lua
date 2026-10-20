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
--   SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) (chapter2.cpp:142)
--   PORTED via creature:SetNPCFlags(), C++ arm order before
--   RemoveAllAuras (the UNIT_FLAG_IMMUNE_TO_NPC arm, chapter2.cpp:141,
--   is likewise PORTED via creature:SetFlag(UNIT_FIELD_FLAGS, ...)
--   2026-10-20; stand-state / summon arms unbridged, below).
-- - EVENT_INTRO_0's RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
--   (chapter2.cpp:166) PORTED via creature:SetNPCFlags(), C++ arm
--   order before Talk(SAY_KOLTIRA_0). The old "no flag bridges"
--   claims were stale — GetNPCFlags/SetNPCFlags and UNIT_FIELD_FLAGS
--   SetFlag/RemoveFlag are bridged (engine/world/lua_objects.go:657,
--   engine/world/lua_creature_events.go:690).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - OnGossipHello's PrepareQuestMenu leg (no quest-menu bridge —
--   kalecgos precedent; IsQuestGiver() is creature_template data).
-- - Reset's SetFlag(UNIT_FLAG_IMMUNE_TO_NPC) PORTED (2026-10-20);
--   SetByteValue stand-state legs (no
--   stand-state bridge), _summons.DespawnAll (no summon/
--   despawn bridge — dark-rider precedent). The Reset
--   SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) arm (chapter2.cpp:142)
--   and RemoveAllAuras arm (chapter2.cpp:144) are PORTED (see above) —
--   the old "no flag bridges" and "no aura-removal bridge
--   (eye_of_acherus precedent)" header claims were stale.
-- - EVENT_INTRO_0's SetByteValue(SIT) arm and EVENT_INTRO_1's
--   SetByteValue(STAND) arm (no stand-state bridge) — only the Talk
--   legs plus EVENT_INTRO_0's RemoveFlag(GOSSIP) arm (ported, above)
--   are modeled.
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
-- - EVENT_CHECK_PLAYER (quest-accept _playerGUID capture +
--   ObjectAccessor::GetPlayer + player alive/distance check
--   + player->FailQuest(QUEST_BLOODY_BREAKOUT) — no bridges;
--   dark-rider / death_knight_initiate precedents).
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
-- UNIT_NPC_FLAG_GOSSIP = 0x00000001 (UnitDefines.h:186 — zulaman /
-- vaelastrasz precedent)
local UNIT_NPC_FLAG_GOSSIP = 0x00000001
-- UnitDefines.h:133; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:690).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_NPC = 0x00000200

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
    -- chapter2.cpp:166 — EVENT_INTRO_0 arm order: RemoveFlag(UNIT_NPC_FLAGS,
    -- UNIT_NPC_FLAG_GOSSIP) before Talk(SAY_KOLTIRA_0) (the SetByteValue SIT
    -- leg has no stand-state bridge); via creature:SetNPCFlags()
    -- (engine/world/lua_objects.go:657)
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
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
    -- chapter2.cpp:139-144, C++ arm order: SetFlag(UNIT_FIELD_FLAGS,
    -- UNIT_FLAG_IMMUNE_TO_NPC) first, then SetFlag(UNIT_NPC_FLAGS,
    -- UNIT_NPC_FLAG_GOSSIP), then RemoveAllAuras
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_NPC)
    creature:SetNPCFlags(bit32.bor(creature:GetNPCFlags(), UNIT_NPC_FLAG_GOSSIP))
    creature:RemoveAllAuras() -- chapter2.cpp:144, C++ arm order (before
                               -- _events.Reset()); the "no aura-removal
                               -- bridge" header claim was stale —
                               -- creature:RemoveAllAuras() is bridged
                               -- (engine/world/lua_creature_events.go:164)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_KOLTIRA, 31, onQuestAccept)
RegisterCreatureGossipEvent(ENTRY_KOLTIRA, 1, onGossipHello)
RegisterCreatureEvent(ENTRY_KOLTIRA, 23, onReset)
