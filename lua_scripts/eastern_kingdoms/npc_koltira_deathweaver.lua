-- npc_koltira_deathweaver (Scarlet Enclave, chapter2.cpp) --
-- Lua port of the npc_koltira_deathweaver struct in
-- src/server/scripts/EasternKingdoms/ScarletEnclave/chapter2.cpp
-- (lines 105-~330; the Bloody Breakout quest-12727 intro sequence).
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
-- Verifiable numbers (file's own enums): SAY_KOLTIRA_0..10 = 0..10 /
-- SAY_VALROTH_0..3 = 0..3; TEXT_ID_EVENT = 13425 (npc_text — packet title slot);
-- QUEST_BLOODY_BREAKOUT = 12727; NPC_FAKE_VALROTH = 29011 /
-- NPC_VALROTH = 29001 / NPC_ACOLYTE = 29007 / NPC_KOLTIRA_MOUNT = 25445;
-- SPELL_KOLTIRA_TRANSFORM = 52899 / SPELL_ANTI_MAGIC_ZONE = 52894 /
-- SPELL_HERO_AGGRO = 53627; koltiraPos (chapter2.cpp:98-103).
-- Bridges used: creature Talk(textId) (nefarian precedent);
-- ON_QUEST_ACCEPT (31) fires live from the Go quest-accept path
-- (quest_details.go -> fireCreatureQuestHook; handler args
-- (event, player, creature, quest), quest.ID field); OnGossipHello
-- (RegisterCreatureGossipEvent, event 1 — handler args (event,
-- player, creature)); GossipSendMenu(13425, creature) with the
-- C++-exact npc_text id in the title slot (visualweapon convention); CreateLuaEvent /
-- RemoveEventById timers (unworthy_initiate convention); onReset
-- (23, maiden convention); MoveJump (lua_creature_events.go:5767,
-- garfrost precedent: MoveJump(x, y, z, speedXY, speedZ) maps
-- positionally onto C++ MoveJump(pos, 25.0f, 15.0f)); MoveTo(id, x,
-- y, z) (lua_creature_events.go:5424 — stamps ScriptMovePointID so
-- the landing fires ev28); MovementInform ev28 (runtime.go:397,
-- handler args (event, creature, moveType, pointId), kalecgos /
-- headless-horseman precedent); JustSummoned ev19 (runtime.go:381,
-- handler args (event, creature, summon), fired from
-- summonTempCreature including group summons); SummonedCreatureDespawn
-- ev20 (runtime.go:382, handler args (event, creature, summoned),
-- the_black_morass / hyjal precedent); SummonCreatureGroup
-- (lua_creature_events.go:4229, landed 2026-11-05, illidan
-- precedent); summon tracking in a per-GUID Lua table (marli /
-- illidan precedent — no Go-side SummonList type exists, the list
-- is Lua-side state); FindNearestCreature (lua_creature_events.go:6770);
-- SetWalk (lua_creature_events.go:4020); Mount(displayId)
-- (lua_creature_events.go:6338 — C++ Unit::Mount takes the display
-- id, Unit.cpp:8310, so Mount(25445) is C++-exact); RemoveAura(spell)
-- (lua_creature_events.go:1761); Despawn / DespawnOrUnsummon(ms)
-- (lua_creature_events.go:3318/7349 — Despawn on a TempSummon fires
-- ev20 on the summoner first, the UnSummon leg); GetPlayersInWorld
-- (runtime.go:1522, chromaggus precedent) + player:GetGUIDLow()
-- (lua_player.go:105) for the _playerGUID lookup; player:IsAlive()
-- (lua_player.go:1418); player:IsWithinDist(creature, 30)
-- (scripting/object.go:333 — INTERACTION_DISTANCE*6 = 30yd);
-- player:FailQuest (lua_player.go:5237); CreateLuaEvent {min,max}
-- randtime (runtime.go:2393-2396) for EVENT_KOLTIRA_ADVICE 8-16s.
-- Ported arms:
-- - OnQuestAccept (quest 12727) -> 500ms EVENT_INTRO_0 ->
--   Talk(SAY_KOLTIRA_0); then 5s EVENT_INTRO_1 ->
--   Talk(SAY_KOLTIRA_1) (C++-exact schedule legs); also schedules
--   EVENT_CHECK_PLAYER at 5s (chapter2.cpp:133).
-- - OnGossipHello: captures _playerGUID; if the event-gossip state
--   is set, send C++-exact gossip menu TEXT_ID_EVENT (13425);
--   otherwise return false — the Go hook vetoes the default menu
--   when registered unless a hook returns boolean false
--   (gossip.go:138-150), so the explicit false is the twin of C++
--   returning false from OnGossipHello.
-- - OnReset (23): C++ arm order — SetFlag(IMMUNE_TO_NPC),
--   SetFlag(GOSSIP), SetStandState(DEAD), RemoveAllAuras, then
--   _events.Reset() (cancelTimers), _summons.DespawnAll()
--   (despawnAllSummons), _playerGUID.Clear(), _eventGossip=false.
-- - EVENT_INTRO_0's RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
--   (chapter2.cpp:166) PORTED via creature:SetNPCFlags(), C++ arm
--   order before Talk(SAY_KOLTIRA_0). The old "no flag bridges"
--   claims were stale — GetNPCFlags/SetNPCFlags (lua_objects.go:665/
--   675) and UNIT_FIELD_FLAGS SetFlag/RemoveFlag
--   (lua_creature_events.go:968/1033) are bridged.
-- - EVENT_INTRO_2's MoveJump + EVENT_INTRO_3's MovePoint (PORTED
--   2026-11-14, round120 — the old "no MotionMaster bridge"
--   selin_fireheart claim is stale: MoveJump :5767 / MoveTo :5424 /
--   ev28 :397 are all live); the intro chain no longer stalls here.
-- - MovementInform(POINT_ID_1) -> kneel + FakeValrothTalk(0) +
--   EVENT_SPAWN_WAVE_1 (1s) + EVENT_INTRO_4 (3s); POINT_ID_2 ->
--   EVENT_INTRO_5 (1s); non-point type POINT_ID_6 -> Mount(25445).
-- - EVENT_INTRO_4: DoCastSelf 52899 / SetStandState(STAND) /
--   MovePoint(POINT_ID_2) (LoadEquipment leg skipped, no bridge —
--   see below); EVENT_INTRO_5: Talk(2) / DoCastSelf 52894 /
--   EVENT_INTRO_6 (4s); EVENT_INTRO_6: Talk(3) / kneel / gossip
--   flag / _eventGossip=true.
-- - SummonCreatureGroup waves 1-3 + valroth (PORTED 2026-11-14,
--   round120 — the old "no summon-group bridge" claim is stale,
--   :4229 landed 2026-11-05) with the C++-exact wave schedule
--   (29s / 21s / 24s) and Talk + FakeValrothTalk legs.
-- - EVENT_KOLTIRA_ADVICE (8-16s randtime): Talk(7) iff the summon
--   table holds a Valroth (29001) entry — the _summons.HasEntry
--   twin.
-- - JustSummoned (ev19): per-GUID table append of {entry, guid,
--   unit} (round120 PORTABLE verdict — the old "no summon-list
--   model" claim is stale, marli/illidan precedent); SummonedCreatureDespawn
--   (ev20): drop from the table, schedule EVENT_OUTRO_1 (1s) iff
--   the despawned summon is Valroth (29001).
-- - EVENT_OUTRO_1..4: RemoveAura(52894) / STAND / RemoveFlag(GOSSIP)
--   / Talk(8) / EVENT_OUTRO_2 (7s) + cancel EVENT_CHECK_PLAYER;
--   Talk(9) / EVENT_OUTRO_3 (4s); Talk(10) / EVENT_OUTRO_4 (3s);
--   SetWalk(false) / RemoveFlag(IMMUNE_TO_NPC) / DoCastSelf 53627 /
--   MovePath leg skipped (no bridge — see below).
-- - EVENT_CHECK_PLAYER (5s loop): no _playerGUID -> return without
--   reschedule (C++ :267 return consumes the event); player dead or
--   beyond 30yd -> DespawnAll summons + DespawnOrUnsummon(1000) +
--   FailQuest(12727); reschedule 5s at the end, C++-exact — except the
--   fail path returns without reschedule (C++ re-schedules, but Koltira
--   despawns in 1s < 5s so it can never fire live; the return avoids
--   firing the check on a despawned motion).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - OnGossipHello's PrepareQuestMenu leg (no quest-menu bridge —
--   kalecgos precedent; IsQuestGiver() is creature_template data).
-- - EVENT_INTRO_4's LoadEquipment(POINT_ID_1) (chapter2.cpp:191 —
--   equipment-set swap; no LoadEquipment method on the Lua surface,
--   only a comment at lua_creature_events.go:7460; the chain
--   proceeds past it with this documented cosmetic deviation).
-- - EVENT_OUTRO_4's MovePath(NPC_KOLTIRA, false) (chapter2.cpp:263
--   — no MovePath method on the Lua surface; the tail leg only,
--   everything before it is ported).
-- Documented deviations: MovePoint(POINT_ID_2, pos, true, 3.839724f)
--   passes an orientation the Lua MoveTo(id, x, y, z) bridge has no
--   slot for, so Koltira keeps his arrival facing instead of the
--   C++ 3.839724f facing (genPath=true is accepted-and-ignored by
--   the bridge). _eventGossip is set true by the now-ported
--   EVENT_INTRO_6, so the OnGossipHello 13425 branch fires live.
--   DespawnAll snapshots the summon list before despawning: each
--   :Despawn() fires ev20 on Koltira, which would mutate the list
--   mid-iteration. C++ Reset order is preserved (timers cancelled
--   before the despawn, so a Valroth despawn re-schedules OUTRO_1
--   into the fresh event map, exactly as C++ does).
local ENTRY_KOLTIRA = 28912
local QUEST_BLOODY_BREAKOUT = 12727
local SAY_KOLTIRA_0 = 0
local SAY_KOLTIRA_1 = 1
local SAY_KOLTIRA_2 = 2
local SAY_KOLTIRA_3 = 3
local SAY_KOLTIRA_4 = 4
local SAY_KOLTIRA_5 = 5
local SAY_KOLTIRA_6 = 6
local SAY_KOLTIRA_7 = 7
local SAY_KOLTIRA_8 = 8
local SAY_KOLTIRA_9 = 9
local SAY_KOLTIRA_10 = 10
local SAY_VALROTH_0 = 0
local SAY_VALROTH_1 = 1
local SAY_VALROTH_2 = 2
local SAY_VALROTH_3 = 3
local TEXT_ID_EVENT = 13425
-- UNIT_NPC_FLAG_GOSSIP = 0x00000001 (UnitDefines.h:186 — zulaman /
-- vaelastrasz precedent)
local UNIT_NPC_FLAG_GOSSIP = 0x00000001
-- UnitDefines.h:133; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:690).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_NPC = 0x00000200
local NPC_FAKE_VALROTH = 29011
local NPC_VALROTH = 29001
local NPC_KOLTIRA_MOUNT = 25445
local SPELL_KOLTIRA_TRANSFORM = 52899
local SPELL_ANTI_MAGIC_ZONE = 52894
local SPELL_HERO_AGGRO = 53627
local POINT_ID_1 = 1
local POINT_ID_2 = 2
local POINT_ID_6 = 6
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:31 (kalecgos precedent)
local EVENT_JUMP = 1004 -- SharedDefines.h:3343 (C++ MoveJump default id)
local SUMMON_ACOLYTES_0 = 0
local SUMMON_ACOLYTES_1 = 1
local SUMMON_ACOLYTES_2 = 2
local SUMMON_VALROTH = 3
-- koltiraPos (chapter2.cpp:98-103)
local KOLTIRA_POS_0 = { 1653.36, -6038.34, 127.584 }
local KOLTIRA_POS_1 = { 1653.765, -6035.075, 127.5844 }
local KOLTIRA_POS_2 = { 1651.89, -6037.101, 127.5844 }
-- FakeValrothTalk range: INSPECT_DISTANCE (28.0f) * 2 = 56.0
local FAKE_VALROTH_RANGE = 56.0
-- EVENT_CHECK_PLAYER range: INTERACTION_DISTANCE (5.0f) * 6 = 30yd
local CHECK_PLAYER_RANGE = 30.0

local timers = {}
local eventGossip = {}
local summons = {}
local playerGUID = {}

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

local function cancelKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
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

local function despawnAllSummons(guid)
    local list = summons[guid]
    if not list then
        return
    end
    -- Snapshot first: each :Despawn() fires ev20 on Koltira, which
    -- mutates this list (see onSummonedCreatureDespawn).
    summons[guid] = nil
    for _, entry in ipairs(list) do
        if entry.unit then
            entry.unit:Despawn()
        end
    end
end

local function fakeValrothTalk(creature, textId)
    local fake = creature:FindNearestCreature(NPC_FAKE_VALROTH, FAKE_VALROTH_RANGE)
    if fake then
        fake:Talk(textId)
    end
end

local function onIntro6(creature, guid)
    -- chapter2.cpp:203-208 — Talk(3), kneel, gossip flag, _eventGossip=true
    creature:Talk(SAY_KOLTIRA_3)
    creature:SetStandState(8) -- UNIT_STAND_STATE_KNEEL (UnitDefines.h:41)
    creature:SetNPCFlags(bit32.bor(creature:GetNPCFlags(), UNIT_NPC_FLAG_GOSSIP))
    eventGossip[guid] = true
end

local function onIntro5(creature, guid)
    -- chapter2.cpp:197-201 — Talk(2), DoCastSelf 52894, INTRO_6 in 4s
    creature:Talk(SAY_KOLTIRA_2)
    creature:CastSpell(creature, SPELL_ANTI_MAGIC_ZONE)
    schedule(guid, "intro6", 4000, function() onIntro6(creature, guid) end)
end

local function onIntro4(creature)
    -- chapter2.cpp:189-195 — DoCastSelf 52899, LoadEquipment (no
    -- bridge, documented skip), STAND, MovePoint(POINT_ID_2)
    creature:CastSpell(creature, SPELL_KOLTIRA_TRANSFORM)
    creature:SetStandState(0)
    creature:MoveTo(POINT_ID_2, KOLTIRA_POS_2[1], KOLTIRA_POS_2[2], KOLTIRA_POS_2[3])
end

local function onIntro3(creature)
    -- chapter2.cpp:184-187 — SetWalk(true), MovePoint(POINT_ID_1)
    creature:SetWalk(true)
    creature:MoveTo(POINT_ID_1, KOLTIRA_POS_1[1], KOLTIRA_POS_1[2], KOLTIRA_POS_1[3])
end

local function onIntro2(creature, guid)
    -- chapter2.cpp:179-182 — MoveJump(koltiraPos[0], 25.0f, 15.0f),
    -- INTRO_3 in 2s
    creature:MoveJump(KOLTIRA_POS_0[1], KOLTIRA_POS_0[2], KOLTIRA_POS_0[3], 25.0, 15.0, EVENT_JUMP)
    schedule(guid, "intro3", 2000, function() onIntro3(creature) end)
end

local function onIntro1(creature, guid)
    -- chapter2.cpp:174 — EVENT_INTRO_1 arm order: SetStandState(STAND)
    -- before Talk(SAY_KOLTIRA_1); bridged (lua_creature_events.go:568);
    -- schedules EVENT_INTRO_2 in 2s (chapter2.cpp:176)
    creature:SetStandState(0)
    creature:Talk(SAY_KOLTIRA_1)
    schedule(guid, "intro2", 2000, function() onIntro2(creature, guid) end)
end

local function onIntro0(creature, guid)
    -- chapter2.cpp:166-168 — EVENT_INTRO_0 arm order: SetStandState(SIT)
    -- first, then RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP), then
    -- Talk(SAY_KOLTIRA_0); bridged (lua_creature_events.go:568)
    creature:SetStandState(1)
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    creature:Talk(SAY_KOLTIRA_0)
    schedule(guid, "intro1", 5000, function() onIntro1(creature, guid) end)
end

local function onAdvice(creature, guid)
    -- chapter2.cpp:236-239 — Talk(7) iff a Valroth is in the summon list
    local list = summons[guid]
    if list then
        for _, entry in ipairs(list) do
            if entry.entry == NPC_VALROTH then
                creature:Talk(SAY_KOLTIRA_7)
                return
            end
        end
    end
end

local function onSpawnValroth(creature, guid)
    -- chapter2.cpp:230-234 — Talk(6), FakeValrothTalk(3), group 3,
    -- EVENT_KOLTIRA_ADVICE in 8-16s (randtime)
    creature:Talk(SAY_KOLTIRA_6)
    fakeValrothTalk(creature, SAY_VALROTH_3)
    creature:SummonCreatureGroup(SUMMON_VALROTH)
    schedule(guid, "advice", { 8000, 16000 }, function() onAdvice(creature, guid) end)
end

local function onSpawnWave3(creature, guid)
    -- chapter2.cpp:222-228 — Talk(5), FakeValrothTalk(2), group 2,
    -- EVENT_SPAWN_VALROTH in 24s
    creature:Talk(SAY_KOLTIRA_5)
    fakeValrothTalk(creature, SAY_VALROTH_2)
    creature:SummonCreatureGroup(SUMMON_ACOLYTES_2)
    schedule(guid, "valroth", 24000, function() onSpawnValroth(creature, guid) end)
end

local function onSpawnWave2(creature, guid)
    -- chapter2.cpp:214-220 — Talk(4), FakeValrothTalk(1), group 1,
    -- EVENT_SPAWN_WAVE_3 in 21s
    creature:Talk(SAY_KOLTIRA_4)
    fakeValrothTalk(creature, SAY_VALROTH_1)
    creature:SummonCreatureGroup(SUMMON_ACOLYTES_1)
    schedule(guid, "wave3", 21000, function() onSpawnWave3(creature, guid) end)
end

local function onSpawnWave1(creature, guid)
    -- chapter2.cpp:210-212 — group 0, EVENT_SPAWN_WAVE_2 in 29s
    creature:SummonCreatureGroup(SUMMON_ACOLYTES_0)
    schedule(guid, "wave2", 29000, function() onSpawnWave2(creature, guid) end)
end

local function onOutro4(creature)
    -- chapter2.cpp:259-264 — SetWalk(false), RemoveFlag(IMMUNE_TO_NPC),
    -- DoCastSelf 53627, MovePath (no bridge, documented skip)
    creature:SetWalk(false)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_NPC)
    creature:CastSpell(creature, SPELL_HERO_AGGRO)
end

local function onOutro3(creature, guid)
    -- chapter2.cpp:253-257 — Talk(10), EVENT_OUTRO_4 in 3s
    creature:Talk(SAY_KOLTIRA_10)
    schedule(guid, "outro4", 3000, function() onOutro4(creature) end)
end

local function onOutro2(creature, guid)
    -- chapter2.cpp:247-251 — Talk(9), EVENT_OUTRO_3 in 4s
    creature:Talk(SAY_KOLTIRA_9)
    schedule(guid, "outro3", 4000, function() onOutro3(creature, guid) end)
end

local function onOutro1(creature, guid)
    -- chapter2.cpp:240-245 — RemoveAurasDueToSpell 52894, STAND,
    -- RemoveFlag(GOSSIP), Talk(8), EVENT_OUTRO_2 in 7s, cancel
    -- EVENT_CHECK_PLAYER
    creature:RemoveAura(SPELL_ANTI_MAGIC_ZONE)
    creature:SetStandState(0)
    creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(), bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
    creature:Talk(SAY_KOLTIRA_8)
    schedule(guid, "outro2", 7000, function() onOutro2(creature, guid) end)
    cancelKey(guid, "checkPlayer")
end

local function onCheckPlayer(creature, guid)
    -- chapter2.cpp:267-281 — no _playerGUID: return WITHOUT reschedule
    -- (the event is consumed); player dead or beyond 30yd: DespawnAll
    -- summons + DespawnOrUnsummon(1s) + FailQuest; reschedule 5s at the end
    local pguid = playerGUID[guid]
    if not pguid then
        return
    end
    local player = nil
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetGUIDLow() == pguid then
            player = p
            break
        end
    end
    if player then
        if not player:IsAlive() or not player:IsWithinDist(creature, CHECK_PLAYER_RANGE) then
            despawnAllSummons(guid)
            creature:DespawnOrUnsummon(1000)
            player:FailQuest(QUEST_BLOODY_BREAKOUT)
            return -- C++ re-schedules, but Koltira despawns in 1s < 5s so it
                   -- can never fire on a live Koltira; returning avoids
                   -- firing the check on a despawned motion
        end
    end
    schedule(guid, "checkPlayer", 5000, function() onCheckPlayer(creature, guid) end)
end

local function onMovementInform(_, creature, moveType, pointId)
    -- chapter2.cpp:287-307 — POINT_MOTION_TYPE only for the point legs;
    -- the else branch is the POINT_ID_6 mount leg
    local guid = creature:GetGUIDLow()
    if moveType == POINT_MOTION_TYPE then
        if pointId == POINT_ID_1 then
            creature:SetStandState(8) -- UNIT_STAND_STATE_KNEEL (UnitDefines.h:41)
            fakeValrothTalk(creature, SAY_VALROTH_0)
            schedule(guid, "wave1", 1000, function() onSpawnWave1(creature, guid) end)
            schedule(guid, "intro4", 3000, function() onIntro4(creature) end)
        elseif pointId == POINT_ID_2 then
            schedule(guid, "intro5", 1000, function() onIntro5(creature, guid) end)
        end
    else
        if pointId == POINT_ID_6 then
            creature:Mount(NPC_KOLTIRA_MOUNT)
        end
    end
end

local function onJustSummoned(_, creature, summon)
    -- chapter2.cpp:309-312 — _summons.Summon(summon): per-GUID table append
    if not summon then
        return
    end
    local guid = creature:GetGUIDLow()
    local list = summons[guid]
    if not list then
        list = {}
        summons[guid] = list
    end
    list[#list + 1] = { entry = summon:GetEntry(), guid = summon:GetGUIDLow(), unit = summon }
end

local function onSummonedCreatureDespawn(_, creature, summoned)
    -- chapter2.cpp:314-320 — Valroth despawn schedules EVENT_OUTRO_1
    -- (1s), then _summons.Despawn(summon): drop from the table
    if not summoned then
        return
    end
    local guid = creature:GetGUIDLow()
    if summoned:GetEntry() == NPC_VALROTH then
        schedule(guid, "outro1", 1000, function() onOutro1(creature, guid) end)
    end
    local list = summons[guid]
    if not list then
        return
    end
    local sguid = summoned:GetGUIDLow()
    for i, entry in ipairs(list) do
        if entry.guid == sguid then
            table.remove(list, i)
            return
        end
    end
end

local function onQuestAccept(event, player, creature, quest)
    if quest.ID ~= QUEST_BLOODY_BREAKOUT then
        return
    end
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "intro0", 500, function() onIntro0(creature, guid) end)
    schedule(guid, "checkPlayer", 5000, function() onCheckPlayer(creature, guid) end)
end

local function onGossipHello(event, player, creature)
    -- chapter2.cpp:112-126 — capture _playerGUID; the non-event branch
    -- returns false (the default menu loads); a nil return would veto
    -- it in Go (gossip.go:138-150), so the C++-false arm must return
    -- false here.
    playerGUID[creature:GetGUIDLow()] = player:GetGUIDLow()
    if not eventGossip[creature:GetGUIDLow()] then
        return false
    end
    player:GossipSendMenu(TEXT_ID_EVENT, creature)
end

local function onReset(_, creature)
    -- chapter2.cpp:141-147, C++ arm order: SetFlag(UNIT_FIELD_FLAGS,
    -- UNIT_FLAG_IMMUNE_TO_NPC) first, then SetFlag(UNIT_NPC_FLAGS,
    -- UNIT_NPC_FLAG_GOSSIP), then SetStandState(DEAD), then RemoveAllAuras,
    -- then _events.Reset() (cancelTimers), _summons.DespawnAll(),
    -- _playerGUID.Clear(), _eventGossip=false (cancelTimers clears it);
    -- SetStandState bridged (lua_creature_events.go:568)
    local guid = creature:GetGUIDLow()
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_NPC)
    creature:SetNPCFlags(bit32.bor(creature:GetNPCFlags(), UNIT_NPC_FLAG_GOSSIP))
    creature:SetStandState(7)
    creature:RemoveAllAuras() -- chapter2.cpp:144, C++ arm order (before
                               -- _events.Reset()); the "no aura-removal
                               -- bridge" header claim was stale —
                               -- creature:RemoveAllAuras() is bridged
                               -- (engine/world/lua_creature_events.go:164)
    cancelTimers(guid)
    despawnAllSummons(guid)
    playerGUID[guid] = nil
end

RegisterCreatureEvent(ENTRY_KOLTIRA, 31, onQuestAccept)
RegisterCreatureGossipEvent(ENTRY_KOLTIRA, 1, onGossipHello)
RegisterCreatureEvent(ENTRY_KOLTIRA, 23, onReset)
RegisterCreatureEvent(ENTRY_KOLTIRA, 28, onMovementInform)
RegisterCreatureEvent(ENTRY_KOLTIRA, 19, onJustSummoned)
RegisterCreatureEvent(ENTRY_KOLTIRA, 20, onSummonedCreatureDespawn)
