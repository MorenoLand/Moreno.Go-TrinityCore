-- Culling of Stratholme: Arthas (npc_arthas_stratholme) — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/CullingOfStratholme/
-- npc_arthas.cpp (1676 lines; npc_arthas_stratholmeAI + npc_stratholme_rp_dummy
-- + spell_stratholme_crusader_strike) + culling_of_stratholme.h (progress
-- states, actions, entries) + the instance SetData/SetGuidData legs
-- (instance_culling_of_stratholme.cpp:317-460, 647-654), which ride the
-- shared instanceData table as one-shot flags consumed by
-- instance_culling_of_stratholme.lua's OnUpdate pump.
-- Entry NPC_ARTHAS 26499 (culling_of_stratholme.h:161); kalimdor loader
-- decl 48 / call 161 per kalimdor_script_loader.cpp.
-- Ported arms:
-- - Combat pump (UpdateAICombat): Holy Light 52444 self-cast below 40% hp
--   (1s pump), Exorcism 52445 on urand(7,14)s with the still-casting retry
--   leg (npc_arthas.cpp:899); KilledUnit zombie line 39; JustDied
--   (instanceData.arthasDied + DespawnOrUnsummon 5s).
-- - The full RP escort machine: AdvanceToState (17-state snapback:
--   ReactState + gossip flag + 5yd NearTeleportTo), DoAction (ev38; -1
--   progress update, RP3/RP5 after-actions, -4..-8 RP-start legs),
--   MovementInform (ev28; types 8 POINT / 16 EFFECT / 17 SPLINE_CHAIN;
--   all point-id cases), the 122 RPEvents-enum handlers (C++-exact timers,
--   Talk lines, facings incl. the @todo literals, emotes, casts, summons),
--   EngageInfinites, MoveInfiniteOnSpawn (entryOverride 27742),
--   JustEngagedWith (_progressRP=false + SetHomePosition),
--   JustReachedHome (devotion aura + _progressRP=true + _afterCombat flush),
--   JustAppeared (devotion aura + AdvanceToState), gossip hello/select
--   (AdvanceDungeon flags, progress-gated exactly like C++).
-- - rp_dummy forwarding: ev28 on Uther (26528) + Jaina (26497) only,
--   forwarding (type, id) to the summoner's arthas dispatch.
-- Bridges (engine/world/lua_creature_events.go): MoveAlongSplineChain
-- (pointId, chainId, walk [, entryOverride]) -> bool, fires ev28 type 17
-- at chain end, false when no chain in DB; PerformEmote =
-- HandleEmoteCommand twin (:3838); GetCreaturesInGrid =
-- GetCreatureListWithEntryInGrid twin (Lua array); SetHomePosition(x,y,z,o);
-- GetX/GetY/GetZ/GetO via the object.go generic fallback; DoAction fires
-- ev38; Talk(line) — the engine takes only the text id, so C++ talk-target
-- legs collapse to the bare line. The RP pump freezes while engaged
-- (events 1/2 track it; C++ UpdateAI returns early into UpdateAICombat)
-- and while _progressRP is false; it ticks at 100ms per GUID.
-- InstanceData flags written here, consumed by the instance pump with the
-- exact C++ SetData guards: utherFinished, startWaves, reachTownHall,
-- townHallDone, gauntletReached, gauntletDone, malganisDone; gossip flags
-- startPurge, startTownHall, toGauntlet, startGauntlet, startMalganis
-- (InitiateArthasEvent triples). Despawn legs also set arthasGone so the
-- instance pump drops the stale arthas bind and EVENT_RESPAWN_ARTHAS fires
-- 1s later (the Go despawn takes effect before the next OnUpdate tick;
-- without the clear, setProgress would DoAction a despawned creature and
-- never respawn him).
-- Stay-outs (documented, not ported): SplineChainResumeInfo resume-on-home
-- (no ResumeSplineChain bridge); CanAIAttack 30yd chase cap (no
-- chase-range bridge); spell_stratholme_crusader_strike SpellScript kill
-- leg (standing SpellScript block — the Crusader Strike CASTS are ported:
-- KILL1 triggered, KILL2/ARTHAS_KILL plain; KILL1's
-- TRIGGERED_IGNORE_SET_FACING maps to triggered=true, the ignore-facing
-- half has no bridge); chromie LaunchMoveSpline fly path (no bridge —
-- SPAWN/LAND/TRANSFORM timers still run; LAND's SetByteValue
-- (UNIT_FIELD_BYTES_1, 3, 0) anim-tier byte has NO bridge, so LAND is a
-- no-op); E.RP2_EVENT_ARTHAS_MOVE_5 has no C++ UpdateAI case (scheduled but
-- never handled — the pump fires it as a no-op, faithful); DATA_UTHER_START
-- (chromie-middle AI unregistered) and DATA_GM_RECALL/DATA_GM_OVERRIDE (GM
-- menu unported); setActive(true) on the Uther/Jaina summons (no SetActive
-- bridge); RP2 priest/footman positions (never referenced by npc_arthas.cpp,
-- omitted); instance->instance summons ride me:SummonCreature (summoner =
-- arthas; C++ has no summoner); citizen/resident rp_dummy forwarding is
-- vacuous (the arthas switch has no case for their chain ids) — not
-- registered. Epoch/Mal'Ganis keep their own boss AIs.

local ENTRY = 26499

local NPC_UTHER = 26528
local NPC_JAINA = 26497
local NPC_MALGANIS_BUNNY = 20562
local NPC_CITIZEN = 28167
local NPC_RESIDENT = 28169
local NPC_FOOTMAN = 27745
local NPC_KNIGHT = 27746
local NPC_PRIEST = 27747
local NPC_SORCERESS = 27752
local NPC_RISEN_ZOMBIE = 27737
local NPC_CITIZEN_INFINITE = 28340
local NPC_RESIDENT_INFINITE = 28341
local NPC_TIME_RIFT = 28409
local NPC_TIME_RIFT_LARGE = 28439
local NPC_INFINITE_ADVERSARY = 27742
local NPC_INFINITE_HUNTER = 27743
local NPC_INFINITE_AGENT = 27744
local NPC_EPOCH = 26532
local NPC_MALGANIS = 26533
local NPC_CHROMIE_3 = 30997

local GO_HIDDEN_PASSAGE = 188686
local GO_CHEST_NORMAL = 190663
local GO_CHEST_HEROIC = 193597

local SPELL_HOLY_LIGHT = 52444
local SPELL_EXORCISM = 52445
local SPELL_DEVOTION_AURA = 52442
local SPELL_CRUSADER_STRIKE = 50773
local SPELL_SHADOWSTEP_VISUAL = 51908
local SPELL_TRANSFORM_VISUAL = 33133
local SPELL_MALGANIS_QUEST_CREDIT = 58124
local SPELL_MALGANIS_KILL_CREDIT = 58630
local SPELL_CHROMIE_3_TRANSFORM = 58986

local LINE_SLAY_ZOMBIE = 39
local LINE_TOWN_HALL_PENDING = 15

-- COSProgressStates (culling_of_stratholme.h:104-122).
local JUST_STARTED = 0x00001
local CRATES_IN_PROGRESS = 0x00002
local CRATES_DONE = 0x00004
local UTHER_TALK = 0x00008
local PURGE_PENDING = 0x00010
local PURGE_STARTING = 0x00020
local WAVES_IN_PROGRESS = 0x00040
local WAVES_DONE = 0x00080
local TOWN_HALL_PENDING = 0x00100
local TOWN_HALL = 0x00200
local TOWN_HALL_COMPLETE = 0x00400
local GAUNTLET_TRANSITION = 0x00800
local GAUNTLET_PENDING = 0x01000
local GAUNTLET_IN_PROGRESS = 0x02000
local GAUNTLET_COMPLETE = 0x04000
local MALGANIS_IN_PROGRESS = 0x08000
local COMPLETE = 0x10000

local REACT_PASSIVE = 0
local REACT_DEFENSIVE = 1
local REACT_AGGRESSIVE = 2

local UNIT_NPC_FLAGS_FIELD = 82 -- UNIT_NPC_FLAGS (UpdateFields.h:139)
local UNIT_NPC_FLAG_GOSSIP = 1
local UNIT_NPC_FLAG_QUESTGIVER = 2 -- UnitDefines.h:187
local UNIT_NPC_EMOTESTATE_FIELD = 83 -- UNIT_NPC_EMOTESTATE (UpdateFields.h:140)
local GAMEOBJECT_FLAGS_FIELD = 9 -- GAMEOBJECT_FLAGS (UpdateFields.h:399)
local GO_FLAG_NOT_SELECTABLE = 16
local GO_STATE_ACTIVE = 1

-- Emotes (SharedDefines.h): TALK 1, EXCLAMATION 5, LAUGH 11, RUDE 14,
-- ROAR 15, POINT_NO_SHEATHE 397, STATE_COWER 431.
local EMOTE_ONESHOT_TALK = 1
local EMOTE_ONESHOT_EXCLAMATION = 5
local EMOTE_ONESHOT_LAUGH = 11
local EMOTE_ONESHOT_RUDE = 14
local EMOTE_ONESHOT_ROAR = 15
local EMOTE_ONESHOT_POINT_NO_SHEATHE = 397
local EMOTE_STATE_COWER = 431

local DATA_MAL_GANIS = 3 -- culling_of_stratholme.h:124-130
local BOSS_STATE_DONE = 3

local SUMMON_MANUAL_DESPAWN = 7

-- Actions enum (npc_arthas.cpp:199-208).
local ACTION_NONE = 0
local RP3_ACTION_AFTER_INITIAL = 1
local RP3_ACTION_AFTER_SPAWN1 = 2
local RP3_ACTION_AFTER_SPAWN2 = 3
local RP3_ACTION_AFTER_SPAWN3 = 4
local RP3_ACTION_AFTER_EPOCH = 5
local RP5_ACTION_AFTER_MALGANIS = 6

-- COSInstanceActions (culling_of_stratholme.h:168-176).
local ACTION_PROGRESS_UPDATE = 1
local ACTION_START_RP_EVENT2 = 4
local ACTION_START_RP_EVENT3 = 5
local ACTION_START_RP_EVENT4_1 = 6
local ACTION_START_RP_EVENT4_2 = 7
local ACTION_START_RP_EVENT5 = 8

-- SplineChains (npc_arthas.cpp:91-141).
local RP1_CHAIN_ARTHAS1 = 1
local RP1_CHAIN_UTHER1 = 1
local RP1_CHAIN_JAINA1 = 1
local RP1_CHAIN_ARTHAS2 = 2
local RP1_CHAIN_UTHER2 = 2
local RP1_CHAIN_JAINA2 = 2
local RP1_CHAIN_UTHER3 = 3
local RP1_CHAIN_JAINA3 = 3
local RP1_CHAIN_JAINA4 = 4
local RP1_CHAIN_ARTHAS3 = 3
local RP1_CHAIN_ARTHAS4 = 4
local RP2_CHAIN_ARTHAS1 = 5
local RP2_CHAIN_CITIZEN1 = 1
local RP2_CHAIN_ARTHAS2 = 6
local RP2_CHAIN_ARTHAS3 = 7
local RP3_CHAIN_ARTHAS1 = 91
local RP3_CHAIN_ARTHAS2 = 92
local RP3_CHAIN_ARTHAS3 = 95
local RP3_CHAIN_ARTHAS3_2 = 96
local RP3_CHAIN_ARTHAS4 = 97
local RP3_CHAIN_ARTHAS5 = 98
local RP3_CHAIN_EPOCH = 1
local RP3_CHAIN_SPAWN1_LOC1 = 4
local RP3_CHAIN_SPAWN1_LOC2 = 2
local RP3_CHAIN_SPAWN1_LOC3 = 1
local RP3_CHAIN_SPAWN1_LOC4 = 3
local RP3_CHAIN_SPAWN2_LOC1 = 5
local RP3_CHAIN_SPAWN2_LOC2 = 6
local RP3_CHAIN_SPAWN2_LOC3 = 7
local RP3_CHAIN_SPAWN2_LOC4 = 8
local RP3_CHAIN_SPAWN3_LOC1 = 9
local RP3_CHAIN_SPAWN3_LOC2 = 10
local RP3_CHAIN_SPAWN3_LOC3 = 11
local RP3_CHAIN_SPAWN3_LOC4 = 12
local RP4_CHAIN_ARTHAS1 = 101
local RP4_CHAIN_ARTHAS2 = 128
local RP4_CHAIN_GAUNTLET1 = 1024
local RP4_CHAIN_GAUNTLET2 = 1025
local RP5_CHAIN_ARTHAS1 = 121
local RP5_CHAIN_ARTHAS2 = 124
local RP5_CHAIN_ARTHAS3 = 125
local RP5_CHAIN_ARTHAS4 = 127

-- PointIDs (npc_arthas.cpp:143-170).
local RP1_POINTID_UTHER1 = 1
local RP1_POINTID_ARTHAS2 = 2
local RP1_POINTID_UTHER2 = 3
local RP1_POINTID_JAINA2 = 4
local RP1_POINTID_UTHER3 = 5
local RP1_POINTID_JAINA4 = 6
local RP1_POINTID_ARTHAS3 = 7
local RP1_POINTID_ARTHAS4 = 8
local RP2_POINTID_ARTHAS1 = 9
local RP2_POINTID_ARTHAS2 = 10
local RP2_POINTID_ARTHAS3 = 11
local RP3_POINTID_TOWN_HALL = 12 -- no MovementInform case in C++
local RP3_POINTID_ARTHAS1 = 13
local RP3_POINTID_ARTHAS2 = 14
local RP3_POINTID_ARTHAS3 = 15
local RP3_POINTID_ARTHAS4 = 16
local RP3_POINTID_ARTHAS5 = 17
local RP4_POINTID_ARTHAS1 = 18
local RP4_POINTID_ARTHAS2 = 19
local RP4_POINTID_GAUNTLET1 = 20
local RP4_POINTID_GAUNTLET2 = 21
local RP5_POINTID_ARTHAS1 = 22
local RP5_POINTID_ARTHAS3 = 23
local RP5_POINTID_ARTHAS4 = 24

-- RPEvents enum (npc_arthas.cpp:210-331), C++-exact ids.
-- RPEvents enum (npc_arthas.cpp:210-331), C++-exact ids.
local E = {
    RP1_EVENT_START1 = 1,
    RP1_EVENT_START2 = 2,
    RP1_EVENT_ARTHAS1 = 3,
    RP1_EVENT_UTHER1 = 4,
    RP1_EVENT_ARTHAS2 = 5,
    RP1_EVENT_ARTHAS3 = 6,
    RP1_EVENT_UTHER_FACE = 7,
    RP1_EVENT_JAINA_FACE = 8,
    RP1_EVENT_UTHER2 = 9,
    RP1_EVENT_ARTHAS4 = 10,
    RP1_EVENT_UTHER3 = 11,
    RP1_EVENT_ARTHAS_TURN = 12,
    RP1_EVENT_ARTHAS5 = 13,
    RP1_EVENT_UTHER4 = 14,
    RP1_EVENT_ARTHAS6 = 15,
    RP1_EVENT_UTHER5 = 16,
    RP1_EVENT_ARTHAS7 = 17,
    RP1_EVENT_JAINA1 = 18,
    RP1_EVENT_ARTHAS8 = 19,
    RP1_EVENT_ARTHAS8_2 = 20,
    RP1_EVENT_UTHER6 = 21,
    RP1_EVENT_UTHER_LEAVE = 22,
    RP1_EVENT_JAINA_LEAVE = 23,
    RP1_EVENT_ARTHAS9 = 24,
    RP1_EVENT_JAINA2 = 25,
    RP1_EVENT_JAINA_LEAVE2 = 26,
    RP1_EVENT_ARTHAS_LEAVE = 27,
    RP1_EVENT_ARTHAS10 = 28,
    RP1_EVENT_ARTHAS_LEAVE2 = 29,
    RP1_EVENT_FINISHED = 30,
    RP2_EVENT_ARTHAS_MOVE_1 = 31,
    RP2_EVENT_CITIZEN1 = 32,
    RP2_EVENT_ARTHAS2 = 33,
    RP2_EVENT_ARTHAS_MOVE_2 = 34,
    RP2_EVENT_CITIZEN2 = 35,
    RP2_EVENT_KILL1 = 36,
    RP2_EVENT_ARTHAS_MOVE_3 = 37,
    RP2_EVENT_KILL2 = 38,
    RP2_EVENT_REACT1 = 39,
    RP2_EVENT_REACT2 = 40,
    RP2_EVENT_REACT3 = 41,
    RP2_EVENT_REACT4 = 42,
    RP2_EVENT_REACT5 = 43,
    RP2_EVENT_ARTHAS_MOVE_4 = 44,
    RP2_EVENT_ARTHAS3 = 45,
    RP2_EVENT_MALGANIS1 = 46,
    RP2_EVENT_TROOPS_FACE = 47,
    RP2_EVENT_ARTHAS_FACE = 48,
    RP2_EVENT_MALGANIS2 = 49,
    RP2_EVENT_MALGANIS_LEAVE1 = 50,
    RP2_EVENT_MALGANIS_LEAVE2 = 51,
    RP2_EVENT_ARTHAS4 = 52,
    RP2_EVENT_ARTHAS4_2 = 53,
    RP2_EVENT_ARTHAS_MOVE_5 = 54,
    RP2_EVENT_ARTHAS5 = 55,
    RP2_EVENT_ARTHAS5_2 = 56,
    RP2_EVENT_ARTHAS5_3 = 57,
    RP2_EVENT_WAVE_START = 58,
    EVENT_TOWN_HALL_REACHED = 59,
    RP3_EVENT_RESIDENT_FACE = 60,
    RP3_EVENT_ARTHAS_FACE = 61,
    RP3_EVENT_CITIZEN1 = 62,
    RP3_EVENT_ARTHAS2 = 63,
    RP3_EVENT_ARTHAS_KILL = 64,
    RP3_EVENT_INFINITE_LAUGH = 65,
    RP3_EVENT_ARTHAS3 = 66,
    RP3_EVENT_CITIZEN2 = 67,
    RP3_EVENT_TRANSFORM1 = 68,
    RP3_EVENT_TRANSFORM2 = 69,
    RP3_EVENT_TRANSFORM3 = 70,
    RP3_EVENT_AGGRO = 71,
    RP3_EVENT_ARTHAS4 = 72,
    RP3_EVENT_ARTHAS_MOVE_1 = 73,
    RP3_EVENT_ARTHAS_MOVE_1_2 = 74,
    RP3_EVENT_SPAWN1 = 75,
    RP3_EVENT_SPAWN1_FACE = 76,
    RP3_EVENT_SPAWN1_AGGRO = 77,
    RP3_EVENT_ARTHAS11 = 78,
    RP3_EVENT_ARTHAS_MOVE_2 = 79,
    RP3_EVENT_ARTHAS20 = 80,
    RP3_EVENT_SPAWN2 = 81,
    RP3_EVENT_ARTHAS_FACE2 = 82,
    RP3_EVENT_SPAWN2_FACE = 83,
    RP3_EVENT_SPAWN2_AGGRO = 84,
    RP3_EVENT_ARTHAS21 = 85,
    RP3_EVENT_ARTHAS_MOVE_3 = 86,
    RP3_EVENT_SPAWN3 = 87,
    RP3_EVENT_ARTHAS30 = 88,
    RP3_EVENT_SPAWN3_FACE = 89,
    RP3_EVENT_SPAWN3_AGGRO = 90,
    RP3_EVENT_EPOCH_SPAWN = 91,
    RP3_EVENT_ARTHAS31 = 92,
    RP3_EVENT_EPOCH_FACE = 93,
    RP3_EVENT_EPOCH1 = 94,
    RP3_EVENT_ARTHAS32 = 95,
    RP3_EVENT_ARTHAS32_2 = 96,
    RP3_EVENT_EPOCH_AGGRO = 97,
    RP4_EVENT_ARTHAS2 = 98,
    RP4_EVENT_HIDDEN_PASSAGE = 99,
    RP4_EVENT_ARTHAS3 = 100,
    RP4_EVENT_GAUNTLET_REACHED = 101,
    RP4_EVENT_ARTHAS_MOVE = 102,
    RP4_EVENT_ARTHAS11 = 103,
    RP4_EVENT_ARTHAS_FACE = 104,
    RP4_EVENT_ARTHAS12 = 105,
    RP4_EVENT_GAUNTLET_RESUME = 106,
    RP4_EVENT_ARTHAS13 = 107,
    RP4_EVENT_GAUNTLET_DONE = 108,
    RP5_EVENT_ARTHAS2 = 109,
    RP5_EVENT_MALGANIS1 = 110,
    RP5_EVENT_MALGANIS_DONE = 111,
    RP5_EVENT_MALGANIS12 = 112,
    RP5_EVENT_MALGANIS_LEAVE = 113,
    RP5_EVENT_ARTHAS10 = 114,
    RP5_EVENT_ARTHAS10_2 = 115,
    RP5_EVENT_ARTHAS11 = 116,
    RP5_EVENT_ARTHAS11_2 = 117,
    RP5_EVENT_ARTHAS_LEAVE = 118,
    RP5_EVENT_ARTHAS_LEAVE2 = 119,
    RP5_EVENT_CHROMIE_SPAWN = 120,
    RP5_EVENT_CHROMIE_LAND = 121,
    RP5_EVENT_CHROMIE_TRANSFORM = 122,
}

-- ArthasPositions (npc_arthas.cpp:451-502), referenced positions only
-- (RP2 priest/footman slots are never read by npc_arthas.cpp).
local POS = {
    RP1_ARTHAS_INITIAL = { 1983.857, 1287.043, 145.5596, 3.0892330 },
    RP1_UTHER_SPAWN = { 1783.843, 1267.481, 139.7800, 0.2698664 },
    RP1_JAINA_SPAWN = { 1876.788, 1305.723, 146.2474, 6.0737460 },
    ARTHAS_PURGE_PENDING_POS = { 2047.948, 1287.598, 142.8568, 3.176499 },
    ARTHAS_WAVES_POS = { 2091.994, 1277.257, 140.4707, 1.134464 },
    RP2_MALGANIS_POS = { 2113.454, 1287.986, 136.3829, 3.071779 },
    ARTHAS_TOWN_HALL_POS = { 2366.240, 1195.253, 132.0441, 3.159046 },
    RP3_SPAWN1_LOC1 = { 2433.154, 1192.572, 148.1547, 5.542059 },
    RP3_SPAWN1_LOC2 = { 2432.990, 1192.760, 148.1474, 1.026526 },
    RP3_SPAWN1_LOC3 = { 2432.824, 1191.816, 148.1556, 4.927707 },
    RP3_SPAWN1_LOC4 = { 2432.711, 1192.857, 148.1550, 6.257423 },
    RP3_SPAWN1_RIFT = { 2433.357, 1192.168, 148.1593, 3.001966 },
    RP3_SPAWN2_LOC1 = { 2414.349, 1136.075, 148.1592, 1.345922 },
    RP3_SPAWN2_LOC2 = { 2403.961, 1180.299, 148.1587, 5.139339 },
    RP3_SPAWN2_LOC3 = { 2414.671, 1136.262, 148.1592, 2.305776 },
    RP3_SPAWN2_LOC4 = { 2403.908, 1179.994, 148.1586, 4.654133 },
    RP3_SPAWN2_RIFT1 = { 2404.311, 1178.306, 148.1585, 1.605703 },
    RP3_SPAWN2_RIFT2 = { 2414.041, 1136.068, 148.1593, 2.234021 },
    RP3_SPAWN3_LOC1 = { 2429.026, 1102.693, 148.1499, 1.952652 },
    RP3_SPAWN3_LOC2 = { 2441.173, 1115.225, 148.1264, 2.302970 },
    RP3_SPAWN3_LOC3 = { 2430.645, 1104.685, 148.1306, 1.135255 },
    RP3_SPAWN3_LOC4 = { 2439.649, 1113.719, 148.1298, 3.111888 },
    RP3_SPAWN3_RIFT1 = { 2429.296, 1102.007, 148.1593, 6.213372 },
    RP3_SPAWN3_RIFT2 = { 2440.057, 1114.226, 148.1593, 6.108652 },
    RP3_EPOCH_SPAWN = { 2457.008, 1113.929, 150.0776, 3.272437 },
    RP3_EPOCH_RIFT = { 2456.058, 1113.838, 150.0917, 1.745329 },
    ARTHAS_TOWN_HALL_END_POS = { 2425.898, 1118.842, 148.0759, 6.073746 },
    ARTHAS_GAUNTLET_POS = { 2534.988, 1126.163, 130.8621, 2.844887 },
    ARTHAS_GAUNTLET_END_POS = { 2363.440, 1404.906, 128.7869, 2.775074 },
    RP5_MALGANIS_POS = { 2296.862, 1501.015, 128.4456, 5.131268 },
    ARTHAS_FINAL_POS = { 2301.055, 1478.977, 128.1299, 1.758816 },
    RP5_CHROMIE_SPAWN = { 2319.560, 1506.408, 152.0474, 0 },
}

-- ArthasSnapbackPositions (npc_arthas.cpp:521-547).
local Snapback = {
    [JUST_STARTED] = { react = REACT_PASSIVE, gossip = false, pos = POS.RP1_ARTHAS_INITIAL },
    [CRATES_IN_PROGRESS] = { react = REACT_PASSIVE, gossip = false, pos = POS.RP1_ARTHAS_INITIAL },
    [CRATES_DONE] = { react = REACT_PASSIVE, gossip = false, pos = POS.RP1_ARTHAS_INITIAL },
    [UTHER_TALK] = { react = REACT_PASSIVE, gossip = false, pos = POS.RP1_ARTHAS_INITIAL },
    [PURGE_PENDING] = { react = REACT_PASSIVE, gossip = true, pos = POS.ARTHAS_PURGE_PENDING_POS },
    [PURGE_STARTING] = { react = REACT_PASSIVE, gossip = false, pos = POS.ARTHAS_PURGE_PENDING_POS },
    [WAVES_IN_PROGRESS] = { react = REACT_AGGRESSIVE, gossip = false, pos = POS.ARTHAS_WAVES_POS },
    [WAVES_DONE] = { react = REACT_DEFENSIVE, gossip = false, pos = POS.ARTHAS_WAVES_POS },
    [TOWN_HALL_PENDING] = { react = REACT_DEFENSIVE, gossip = true, pos = POS.ARTHAS_TOWN_HALL_POS },
    [TOWN_HALL] = { react = REACT_DEFENSIVE, gossip = false, pos = POS.ARTHAS_TOWN_HALL_POS },
    [TOWN_HALL_COMPLETE] = { react = REACT_PASSIVE, gossip = true, pos = POS.ARTHAS_TOWN_HALL_END_POS },
    [GAUNTLET_TRANSITION] = { react = REACT_PASSIVE, gossip = false, pos = POS.ARTHAS_TOWN_HALL_END_POS },
    [GAUNTLET_PENDING] = { react = REACT_PASSIVE, gossip = true, pos = POS.ARTHAS_GAUNTLET_POS },
    [GAUNTLET_IN_PROGRESS] = { react = REACT_DEFENSIVE, gossip = false, pos = POS.ARTHAS_GAUNTLET_POS },
    [GAUNTLET_COMPLETE] = { react = REACT_PASSIVE, gossip = true, pos = POS.ARTHAS_GAUNTLET_END_POS },
    [MALGANIS_IN_PROGRESS] = { react = REACT_DEFENSIVE, gossip = false, pos = POS.ARTHAS_GAUNTLET_END_POS },
    [COMPLETE] = { react = REACT_PASSIVE, gossip = false, pos = POS.ARTHAS_FINAL_POS },
}

local SNAPBACK_DISTANCE = 5.0
local RP_TICK_MS = 100

local pumps = {}

local function cancelPump(guid)
    local id = pumps[guid]
    if id then
        RemoveEventById(id)
        pumps[guid] = nil
    end
end

-- Per-GUID RP state (npc_arthas_stratholmeAI members): _progressRP,
-- _afterCombat, the EventMap list, the pump id, and the engaged flag
-- (events 1/2 track it — no IsEngaged bridge; C++ UpdateAI returns early
-- while engaged).
local rp = {}

local function rpFor(creature)
    local guid = creature:GetGUID()
    local st = rp[guid]
    if not st then
        st = { progressRP = true, afterCombat = ACTION_NONE, events = {},
            pumpId = nil, inCombat = false }
        rp[guid] = st
    end
    return st
end

local function cancelRpEvent(st, id)
    for i, ev in ipairs(st.events) do
        if ev.id == id then
            table.remove(st.events, i)
            return
        end
    end
end

local function cancelRpPump(creature, st)
    if st.pumpId then
        RemoveEventById(st.pumpId)
        st.pumpId = nil
    end
    rp[creature:GetGUID()] = nil
end

local fireRpEvent -- forward: the pump closure below calls it
local advanceToState -- forward: doAction below calls it
local doAction -- forward: scheduleActionOOC below calls it

local function startRpPump(creature, st)
    if st.pumpId then
        return
    end
    st.pumpId = CreateLuaEvent(function()
        -- C++ UpdateAI: engaged -> UpdateAICombat, return; !_progressRP ->
        -- return. The EventMap does not tick down in either case.
        if st.inCombat or not st.progressRP then
            return
        end
        local due = {}
        for _, ev in ipairs(st.events) do
            ev.remainingMs = ev.remainingMs - RP_TICK_MS
            if ev.remainingMs <= 0 then
                due[#due + 1] = ev.id
            end
        end
        for _, id in ipairs(due) do
            cancelRpEvent(st, id)
            fireRpEvent(creature, st, id)
        end
    end, RP_TICK_MS, 0)
end

-- C++ EventMap::ScheduleEvent replaces a pending event of the same id
-- (instance_culling_of_stratholme.lua scheduleEvent twin).
local function scheduleRpEvent(creature, st, id, ms)
    for _, ev in ipairs(st.events) do
        if ev.id == id then
            ev.remainingMs = ms
            startRpPump(creature, st)
            return
        end
    end
    st.events[#st.events + 1] = { id = id, remainingMs = ms }
    startRpPump(creature, st)
end

local function currentProgress(creature)
    local data = creature:GetMap():GetInstanceData()
    if data and data.progress then
        return data.progress
    end
    return JUST_STARTED
end

local function setInstanceFlag(creature, name)
    local data = creature:GetMap():GetInstanceData()
    if data then
        data[name] = true
    end
end

-- AdvanceToState (npc_arthas.cpp:556-619).
advanceToState = function(creature, st, newState)
    st.progressRP = true
    local snap = Snapback[newState]
    if snap then
        creature:SetReactState(snap.react)
        creature:SetImmuneToAll(snap.react == REACT_PASSIVE)
        if snap.gossip then
            creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        else
            creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
        end
        local p = snap.pos
        local dx = creature:GetX() - p[1]
        local dy = creature:GetY() - p[2]
        local dz = creature:GetZ() - p[3]
        if math.sqrt(dx * dx + dy * dy + dz * dz) > SNAPBACK_DISTANCE then
            creature:Teleport(p[1], p[2], p[3], p[4])
        end
    end
    if newState == WAVES_DONE then
        scheduleRpEvent(creature, st, E.EVENT_TOWN_HALL_REACHED, 3000)
    elseif newState == UTHER_TALK then
        local uther = creature:SummonCreature(NPC_UTHER,
            POS.RP1_UTHER_SPAWN[1], POS.RP1_UTHER_SPAWN[2],
            POS.RP1_UTHER_SPAWN[3], POS.RP1_UTHER_SPAWN[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if uther then
            uther:MoveAlongSplineChain(RP1_POINTID_UTHER1, RP1_CHAIN_UTHER1, false)
        end
        local jaina = creature:SummonCreature(NPC_JAINA,
            POS.RP1_JAINA_SPAWN[1], POS.RP1_JAINA_SPAWN[2],
            POS.RP1_JAINA_SPAWN[3], POS.RP1_JAINA_SPAWN[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if jaina then
            jaina:MoveAlongSplineChain(0, RP1_CHAIN_JAINA1, true)
        end
        creature:MoveAlongSplineChain(0, RP1_CHAIN_ARTHAS1, false)
    elseif newState == TOWN_HALL_PENDING then
        creature:Talk(LINE_TOWN_HALL_PENDING)
    elseif newState == COMPLETE then
        if #st.events == 0 then
            -- Instance loading into COMPLETE: spawn chromie.
            scheduleRpEvent(creature, st, E.RP5_EVENT_CHROMIE_SPAWN, 1000)
            scheduleRpEvent(creature, st, E.RP5_EVENT_CHROMIE_LAND, 12668)
            scheduleRpEvent(creature, st, E.RP5_EVENT_CHROMIE_TRANSFORM, 15491)
            creature:SetVisible(false)
        end
    end
end

-- DoAction (npc_arthas.cpp:635-673) + SetGUID RP-start legs
-- (npc_arthas.cpp:675-713): the SetGUID legs arrive as negative
-- DoAction values (no SetGUID Lua dispatch; the instance pump calls
-- arthas:DoAction(-ACTION_START_RP_EVENTn)).
doAction = function(creature, st, action)
    if action == -ACTION_PROGRESS_UPDATE then
        advanceToState(creature, st, currentProgress(creature))
    elseif action == RP3_ACTION_AFTER_INITIAL then
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS4, 1000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS_MOVE_1, 7000)
    elseif action == RP3_ACTION_AFTER_SPAWN1 then
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS11, 1000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS_MOVE_2, 3000)
    elseif action == RP3_ACTION_AFTER_SPAWN2 then
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS21, 1000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS_MOVE_3, 4000)
    elseif action == RP3_ACTION_AFTER_SPAWN3 then
        scheduleRpEvent(creature, st, E.RP3_EVENT_EPOCH_SPAWN, 3000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS31, 4000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_EPOCH_FACE, 6000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_EPOCH1, 9000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS32, 22000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_ARTHAS32_2, 27000)
        scheduleRpEvent(creature, st, E.RP3_EVENT_EPOCH_AGGRO, 30000)
    elseif action == RP3_ACTION_AFTER_EPOCH then
        setInstanceFlag(creature, "townHallDone")
    elseif action == RP5_ACTION_AFTER_MALGANIS then
        scheduleRpEvent(creature, st, E.RP5_EVENT_MALGANIS_DONE, 3000)
    elseif action == -ACTION_START_RP_EVENT2 then
        creature:Talk(10) -- RP2_LINE_ARTHAS1
        scheduleRpEvent(creature, st, E.RP2_EVENT_ARTHAS_MOVE_1, 9000)
    elseif action == -ACTION_START_RP_EVENT3 then
        creature:Talk(16) -- RP3_LINE_ARTHAS1
        creature:MoveAlongSplineChain(RP3_POINTID_ARTHAS1, RP3_CHAIN_ARTHAS1, false)
    elseif action == -ACTION_START_RP_EVENT4_1 then
        creature:Talk(28) -- RP4_LINE_ARTHAS1
        creature:MoveAlongSplineChain(RP4_POINTID_ARTHAS1, RP4_CHAIN_ARTHAS1, true)
    elseif action == -ACTION_START_RP_EVENT4_2 then
        creature:Talk(31) -- RP4_LINE_ARTHAS10
        scheduleRpEvent(creature, st, E.RP4_EVENT_ARTHAS_MOVE, 5000)
    elseif action == -ACTION_START_RP_EVENT5 then
        creature:Talk(35) -- RP5_LINE_ARTHAS1
        creature:SummonCreature(NPC_MALGANIS,
            POS.RP5_MALGANIS_POS[1], POS.RP5_MALGANIS_POS[2],
            POS.RP5_MALGANIS_POS[3], POS.RP5_MALGANIS_POS[4],
            SUMMON_MANUAL_DESPAWN, 0)
        creature:MoveAlongSplineChain(RP5_POINTID_ARTHAS1, RP5_CHAIN_ARTHAS1, false)
    end
    -- -ACTION_START_RP_EVENT1 has no C++ SetGUID case: RP1 starts via
    -- AdvanceToState(UTHER_TALK).
end

-- ScheduleActionOOC (npc_arthas.cpp:621-629).
local function scheduleActionOOC(creature, st, action)
    if st.progressRP then
        doAction(creature, st, action)
    else
        st.afterCombat = action
    end
end

local function engageInfinites(creature)
    for _, entry in ipairs({ NPC_INFINITE_ADVERSARY, NPC_INFINITE_AGENT,
            NPC_INFINITE_HUNTER }) do
        for _, target in ipairs(creature:GetCreaturesInGrid(entry, 100)) do
            target:SetImmuneToAll(false)
            creature:EngageWithTarget(target)
            target:EngageWithTarget(creature)
        end
    end
end

-- MoveInfiniteOnSpawn (npc_arthas.cpp:1514-1519): the chain is looked up
-- by the ADVERSARY entry for all three spawn types.
local function moveInfiniteOnSpawn(spawn, chainId)
    if spawn then
        spawn:MoveAlongSplineChain(0, chainId, true, NPC_INFINITE_ADVERSARY)
    end
end

-- E.RP3_EVENT_SPAWN1/2/3 shared body (npc_arthas.cpp:1250-1330): urand(0,2)
-- extra: 0 = extra adversary, 1 = extra hunter, 2 = extra agent.
local function spawnInfiniteWave(creature, locs, chains, rifts, riftMs)
    local extra = math.random(0, 2)
    local s1 = creature:SummonCreature(NPC_INFINITE_ADVERSARY,
        locs[1][1], locs[1][2], locs[1][3], locs[1][4],
        SUMMON_MANUAL_DESPAWN, 0)
    moveInfiniteOnSpawn(s1, chains[1])
    local s2 = creature:SummonCreature(
        extra ~= 0 and NPC_INFINITE_HUNTER or NPC_INFINITE_ADVERSARY,
        locs[2][1], locs[2][2], locs[2][3], locs[2][4],
        SUMMON_MANUAL_DESPAWN, 0)
    moveInfiniteOnSpawn(s2, chains[2])
    local s3 = creature:SummonCreature(
        extra < 2 and NPC_INFINITE_HUNTER or NPC_INFINITE_AGENT,
        locs[3][1], locs[3][2], locs[3][3], locs[3][4],
        SUMMON_MANUAL_DESPAWN, 0)
    moveInfiniteOnSpawn(s3, chains[3])
    local s4 = creature:SummonCreature(NPC_INFINITE_AGENT,
        locs[4][1], locs[4][2], locs[4][3], locs[4][4],
        SUMMON_MANUAL_DESPAWN, 0)
    moveInfiniteOnSpawn(s4, chains[4])
    for _, riftPos in ipairs(rifts) do
        local rift = creature:SummonCreature(NPC_TIME_RIFT,
            riftPos[1], riftPos[2], riftPos[3], riftPos[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if rift then
            rift:DespawnOrUnsummon(riftMs)
        end
    end
end

local function faceInfinitesAtArthas(creature)
    for _, entry in ipairs({ NPC_INFINITE_ADVERSARY, NPC_INFINITE_AGENT,
            NPC_INFINITE_HUNTER }) do
        for _, target in ipairs(creature:GetCreaturesInGrid(entry, 100)) do
            if target:IsAlive() then
                target:SetFacingToObject(creature)
            end
        end
    end
end

local function nearest(creature, entry, dist)
    return creature:FindNearestCreature(entry, dist)
end

-- UpdateAI RP event pump (npc_arthas.cpp:914-1490). Each branch mirrors its
-- C++ case; talkerEntry 0 = self, else the nearest creature of that entry
-- within 100yd, dispatched after the branch (the C++ post-switch Talk leg).
-- Talk lines are the RP*_LINE_* enum values (npc_arthas.cpp:333-394).
fireRpEvent = function(creature, st, id)
    local talkerEntry, talkerLine = nil, 0
    if id == E.RP1_EVENT_START1 then
        local jaina = nearest(creature, NPC_JAINA, 100)
        if jaina then
            jaina:SetFacingToObject(creature)
        end
    elseif id == E.RP1_EVENT_START2 then
        local uther = nearest(creature, NPC_UTHER, 100)
        if uther then
            uther:SetFacingToObject(creature)
            creature:SetFacingToObject(uther)
        end
    elseif id == E.RP1_EVENT_ARTHAS1 then
        talkerEntry, talkerLine = 0, 0
    elseif id == E.RP1_EVENT_UTHER1 then
        talkerEntry, talkerLine = NPC_UTHER, 0
    elseif id == E.RP1_EVENT_ARTHAS2 then
        talkerEntry, talkerLine = 0, 1
        creature:MoveAlongSplineChain(RP1_POINTID_ARTHAS2, RP1_CHAIN_ARTHAS2, true)
        local uther = nearest(creature, NPC_UTHER, 100)
        if uther then
            uther:MoveAlongSplineChain(RP1_POINTID_UTHER2, RP1_CHAIN_UTHER2, true)
        end
        local jaina = nearest(creature, NPC_JAINA, 100)
        if jaina then
            jaina:MoveAlongSplineChain(RP1_POINTID_JAINA2, RP1_CHAIN_JAINA2, true)
        end
    elseif id == E.RP1_EVENT_UTHER_FACE then
        local uther = nearest(creature, NPC_UTHER, 100)
        if uther then
            uther:SetFacingToObject(creature)
        end
    elseif id == E.RP1_EVENT_JAINA_FACE then
        local jaina = nearest(creature, NPC_JAINA, 100)
        if jaina then
            jaina:SetFacingToObject(creature)
        end
    elseif id == E.RP1_EVENT_ARTHAS3 then
        creature:SetFacingTo(6.248279) -- @todo migrate
        talkerEntry, talkerLine = 0, 2
        for _, entry in ipairs({ NPC_FOOTMAN, NPC_SORCERESS, NPC_KNIGHT, NPC_PRIEST }) do
            for _, unit in ipairs(creature:GetCreaturesInGrid(entry, 100)) do
                if unit:IsAlive() then
                    unit:SetFacingToObject(creature)
                end
            end
        end
    elseif id == E.RP1_EVENT_UTHER2 then
        talkerEntry, talkerLine = NPC_UTHER, 1
    elseif id == E.RP1_EVENT_ARTHAS4 then
        talkerEntry, talkerLine = 0, 3
    elseif id == E.RP1_EVENT_UTHER3 then
        talkerEntry, talkerLine = NPC_UTHER, 2
    elseif id == E.RP1_EVENT_ARTHAS_TURN then
        local uther = nearest(creature, NPC_UTHER, 100)
        if uther then
            creature:SetFacingToObject(uther)
        end
    elseif id == E.RP1_EVENT_ARTHAS5 then
        talkerEntry, talkerLine = 0, 4
    elseif id == E.RP1_EVENT_UTHER4 then
        talkerEntry, talkerLine = NPC_UTHER, 3
    elseif id == E.RP1_EVENT_ARTHAS6 then
        talkerEntry, talkerLine = 0, 5
    elseif id == E.RP1_EVENT_UTHER5 then
        talkerEntry, talkerLine = NPC_UTHER, 4
    elseif id == E.RP1_EVENT_ARTHAS7 then
        talkerEntry, talkerLine = 0, 6
    elseif id == E.RP1_EVENT_JAINA1 then
        talkerEntry, talkerLine = NPC_JAINA, 0
    elseif id == E.RP1_EVENT_ARTHAS8 then
        talkerEntry, talkerLine = 0, 7
    elseif id == E.RP1_EVENT_ARTHAS8_2 then
        creature:PerformEmote(EMOTE_ONESHOT_EXCLAMATION)
    elseif id == E.RP1_EVENT_UTHER6 then
        talkerEntry, talkerLine = NPC_UTHER, 5
    elseif id == E.RP1_EVENT_UTHER_LEAVE then
        local uther = nearest(creature, NPC_UTHER, 100)
        if uther then
            uther:MoveAlongSplineChain(RP1_POINTID_UTHER3, RP1_CHAIN_UTHER3, true)
        end
    elseif id == E.RP1_EVENT_JAINA_LEAVE then
        local jaina = nearest(creature, NPC_JAINA, 100)
        if jaina then
            jaina:MoveAlongSplineChain(0, RP1_CHAIN_JAINA3, true)
        end
    elseif id == E.RP1_EVENT_ARTHAS9 then
        talkerEntry, talkerLine = 0, 8
        local jaina = nearest(creature, NPC_JAINA, 100)
        if jaina then
            creature:SetFacingToObject(jaina)
        end
    elseif id == E.RP1_EVENT_JAINA2 then
        talkerEntry, talkerLine = NPC_JAINA, 1
    elseif id == E.RP1_EVENT_JAINA_LEAVE2 then
        local jaina = nearest(creature, NPC_JAINA, 100)
        if jaina then
            jaina:MoveAlongSplineChain(RP1_POINTID_JAINA4, RP1_CHAIN_JAINA4, true)
        end
    elseif id == E.RP1_EVENT_ARTHAS_LEAVE then
        creature:MoveAlongSplineChain(RP1_POINTID_ARTHAS3, RP1_CHAIN_ARTHAS3, true)
    elseif id == E.RP1_EVENT_ARTHAS10 then
        talkerEntry, talkerLine = 0, 9
        creature:SetFacingTo(3.141593) -- @todo migrate
    elseif id == E.RP1_EVENT_ARTHAS_LEAVE2 then
        creature:MoveAlongSplineChain(RP1_POINTID_ARTHAS4, RP1_CHAIN_ARTHAS4, false)
    elseif id == E.RP1_EVENT_FINISHED then
        talkerEntry, talkerLine = 0, 40
        creature:SetFacingTo(POS.ARTHAS_PURGE_PENDING_POS[4])
        setInstanceFlag(creature, "utherFinished")
    elseif id == E.RP2_EVENT_ARTHAS_MOVE_1 then
        creature:MoveAlongSplineChain(RP2_POINTID_ARTHAS1, RP2_CHAIN_ARTHAS1, true)
    elseif id == E.RP2_EVENT_CITIZEN1 then
        local citizen = nearest(creature, NPC_CITIZEN, 100)
        if citizen then
            citizen:MoveAlongSplineChain(0, RP2_CHAIN_CITIZEN1, true)
            citizen:Talk(0) -- RP2_LINE_CITIZEN1
        end
    elseif id == E.RP2_EVENT_ARTHAS2 then
        talkerEntry, talkerLine = 0, 11
    elseif id == E.RP2_EVENT_ARTHAS_MOVE_2 then
        creature:MoveAlongSplineChain(RP2_POINTID_ARTHAS2, RP2_CHAIN_ARTHAS2, true)
    elseif id == E.RP2_EVENT_CITIZEN2 then
        talkerEntry, talkerLine = NPC_CITIZEN, 1
    elseif id == E.RP2_EVENT_KILL1 then
        local citizen = nearest(creature, NPC_CITIZEN, 100)
        if citizen then
            creature:CastSpell(citizen, SPELL_CRUSADER_STRIKE, true)
        end
        local resident = nearest(creature, NPC_RESIDENT, 100)
        if resident then
            resident:SetUInt32Value(UNIT_NPC_EMOTESTATE_FIELD, EMOTE_STATE_COWER)
            resident:Talk(0) -- RP2_LINE_RESIDENT1
        end
    elseif id == E.RP2_EVENT_ARTHAS_MOVE_3 then
        creature:MoveAlongSplineChain(RP2_POINTID_ARTHAS3, RP2_CHAIN_ARTHAS3, true)
    elseif id == E.RP2_EVENT_KILL2 then
        local resident = nearest(creature, NPC_RESIDENT, 100)
        if resident then
            creature:CastSpell(resident, SPELL_CRUSADER_STRIKE)
        end
    elseif id == E.RP2_EVENT_REACT1 or id == E.RP2_EVENT_REACT2
            or id == E.RP2_EVENT_REACT3 or id == E.RP2_EVENT_REACT4
            or id == E.RP2_EVENT_REACT5 then
        local entry = math.random(0, 1) == 1 and NPC_CITIZEN or NPC_RESIDENT
        local victims = creature:GetCreaturesInGrid(entry, 60)
        if #victims > 0 then
            local victim = victims[math.random(#victims)]
            if victim:IsAlive() then
                local emotes = { EMOTE_ONESHOT_TALK, EMOTE_ONESHOT_EXCLAMATION,
                    EMOTE_ONESHOT_RUDE, EMOTE_ONESHOT_ROAR }
                victim:PerformEmote(emotes[math.random(#emotes)])
            end
        end
    elseif id == E.RP2_EVENT_ARTHAS_MOVE_4 then
        creature:SetFacingTo(2.234021) -- @todo
    elseif id == E.RP2_EVENT_ARTHAS3 then
        talkerEntry, talkerLine = 0, 12
    elseif id == E.RP2_EVENT_MALGANIS1 then
        local bunny = nearest(creature, NPC_MALGANIS_BUNNY, 80)
        if bunny then
            bunny:CastSpell(bunny, SPELL_SHADOWSTEP_VISUAL)
        end
        local malganis = creature:SummonCreature(NPC_MALGANIS,
            POS.RP2_MALGANIS_POS[1], POS.RP2_MALGANIS_POS[2],
            POS.RP2_MALGANIS_POS[3], POS.RP2_MALGANIS_POS[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if malganis then
            malganis:CastSpell(malganis, SPELL_SHADOWSTEP_VISUAL)
            malganis:Talk(0) -- RP2_LINE_MALGANIS1
        end
    elseif id == E.RP2_EVENT_TROOPS_FACE then
        local malganis = nearest(creature, NPC_MALGANIS, 80)
        if malganis then
            for _, entry in ipairs({ NPC_FOOTMAN, NPC_PRIEST }) do
                for _, unit in ipairs(creature:GetCreaturesInGrid(entry, 50)) do
                    if unit:IsAlive() then
                        unit:SetFacingToObject(malganis)
                    end
                end
            end
        end
    elseif id == E.RP2_EVENT_ARTHAS_FACE then
        local malganis = nearest(creature, NPC_MALGANIS, 80)
        if malganis then
            creature:SetFacingToObject(malganis)
        end
    elseif id == E.RP2_EVENT_MALGANIS2 then
        talkerEntry, talkerLine = NPC_MALGANIS, 1
    elseif id == E.RP2_EVENT_MALGANIS_LEAVE1 then
        local malganis = nearest(creature, NPC_MALGANIS, 80)
        if malganis then
            malganis:CastSpell(malganis, SPELL_SHADOWSTEP_VISUAL)
        end
    elseif id == E.RP2_EVENT_MALGANIS_LEAVE2 then
        local malganis = nearest(creature, NPC_MALGANIS, 80)
        if malganis then
            malganis:DespawnOrUnsummon()
        end
        local bunny = nearest(creature, NPC_MALGANIS_BUNNY, 80)
        if bunny then
            bunny:CastSpell(bunny, SPELL_SHADOWSTEP_VISUAL)
        end
    elseif id == E.RP2_EVENT_ARTHAS4 then
        talkerEntry, talkerLine = 0, 13
    elseif id == E.RP2_EVENT_ARTHAS4_2 then
        creature:PerformEmote(EMOTE_ONESHOT_EXCLAMATION)
    elseif id == E.RP2_EVENT_ARTHAS_MOVE_5 then
        -- No C++ UpdateAI case: scheduled from RP2_POINTID_ARTHAS3 but
        -- never handled (falls to default) — faithful no-op.
    elseif id == E.RP2_EVENT_ARTHAS5 then
        talkerEntry, talkerLine = 0, 14
    elseif id == E.RP2_EVENT_ARTHAS5_2 or id == E.RP2_EVENT_ARTHAS5_3 then
        creature:PerformEmote(EMOTE_ONESHOT_TALK)
    elseif id == E.RP2_EVENT_WAVE_START then
        setInstanceFlag(creature, "startWaves")
    elseif id == E.EVENT_TOWN_HALL_REACHED then
        setInstanceFlag(creature, "arthasGone")
        cancelRpPump(creature, st)
        creature:DespawnOrUnsummon()
        setInstanceFlag(creature, "reachTownHall")
    elseif id == E.RP3_EVENT_RESIDENT_FACE then
        local infinite = nearest(creature, NPC_RESIDENT_INFINITE, 100)
        if infinite then
            infinite:SetFacingToObject(creature)
        end
    elseif id == E.RP3_EVENT_ARTHAS_FACE then
        creature:SetFacingTo(0.541052) -- @todo
    elseif id == E.RP3_EVENT_CITIZEN1 then
        talkerEntry, talkerLine = NPC_CITIZEN_INFINITE, 0
    elseif id == E.RP3_EVENT_ARTHAS2 then
        talkerEntry, talkerLine = 0, 17
        creature:MoveAlongSplineChain(RP3_POINTID_ARTHAS2, RP3_CHAIN_ARTHAS2, true)
    elseif id == E.RP3_EVENT_ARTHAS_KILL then
        local citizen = nearest(creature, NPC_CITIZEN_INFINITE, 100)
        if citizen then
            creature:CastSpell(citizen, SPELL_CRUSADER_STRIKE)
        end
    elseif id == E.RP3_EVENT_INFINITE_LAUGH then
        local citizen = nearest(creature, NPC_CITIZEN_INFINITE, 100)
        if citizen then
            citizen:PerformEmote(EMOTE_ONESHOT_LAUGH)
        end
    elseif id == E.RP3_EVENT_ARTHAS3 then
        talkerEntry, talkerLine = 0, 18
    elseif id == E.RP3_EVENT_CITIZEN2 then
        talkerEntry, talkerLine = NPC_CITIZEN_INFINITE, 1
    elseif id == E.RP3_EVENT_TRANSFORM1 or id == E.RP3_EVENT_TRANSFORM3 then
        local citizen = nearest(creature, NPC_CITIZEN_INFINITE, 100)
        if citizen then
            citizen:CastSpell(citizen, SPELL_TRANSFORM_VISUAL)
            citizen:UpdateEntry(NPC_INFINITE_HUNTER)
        end
    elseif id == E.RP3_EVENT_TRANSFORM2 then
        local resident = nearest(creature, NPC_RESIDENT_INFINITE, 100)
        if resident then
            resident:CastSpell(resident, SPELL_TRANSFORM_VISUAL)
            resident:UpdateEntry(NPC_INFINITE_AGENT)
        end
    elseif id == E.RP3_EVENT_AGGRO then
        engageInfinites(creature)
        scheduleActionOOC(creature, st, RP3_ACTION_AFTER_INITIAL)
    elseif id == E.RP3_EVENT_ARTHAS4 then
        talkerEntry, talkerLine = 0, 19
    elseif id == E.RP3_EVENT_ARTHAS_MOVE_1 then
        creature:MoveAlongSplineChain(RP3_POINTID_ARTHAS3, RP3_CHAIN_ARTHAS3, false)
    elseif id == E.RP3_EVENT_ARTHAS_MOVE_1_2 then
        talkerEntry, talkerLine = 0, 20
        creature:MoveAlongSplineChain(0, RP3_CHAIN_ARTHAS3_2, true)
    elseif id == E.RP3_EVENT_SPAWN1 then
        spawnInfiniteWave(creature,
            { POS.RP3_SPAWN1_LOC1, POS.RP3_SPAWN1_LOC2, POS.RP3_SPAWN1_LOC3,
                POS.RP3_SPAWN1_LOC4 },
            { RP3_CHAIN_SPAWN1_LOC1, RP3_CHAIN_SPAWN1_LOC2,
                RP3_CHAIN_SPAWN1_LOC3, RP3_CHAIN_SPAWN1_LOC4 },
            { POS.RP3_SPAWN1_RIFT }, 4000)
    elseif id == E.RP3_EVENT_SPAWN1_FACE or id == E.RP3_EVENT_SPAWN2_FACE
            or id == E.RP3_EVENT_SPAWN3_FACE then
        faceInfinitesAtArthas(creature)
    elseif id == E.RP3_EVENT_SPAWN1_AGGRO then
        engageInfinites(creature)
        scheduleActionOOC(creature, st, RP3_ACTION_AFTER_SPAWN1)
    elseif id == E.RP3_EVENT_ARTHAS11 then
        talkerEntry, talkerLine = 0, 21
    elseif id == E.RP3_EVENT_ARTHAS_MOVE_2 then
        creature:MoveAlongSplineChain(RP3_POINTID_ARTHAS4, RP3_CHAIN_ARTHAS4, false)
    elseif id == E.RP3_EVENT_ARTHAS20 then
        talkerEntry, talkerLine = 0, 22
    elseif id == E.RP3_EVENT_SPAWN2 then
        spawnInfiniteWave(creature,
            { POS.RP3_SPAWN2_LOC1, POS.RP3_SPAWN2_LOC2, POS.RP3_SPAWN2_LOC3,
                POS.RP3_SPAWN2_LOC4 },
            { RP3_CHAIN_SPAWN2_LOC1, RP3_CHAIN_SPAWN2_LOC2,
                RP3_CHAIN_SPAWN2_LOC3, RP3_CHAIN_SPAWN2_LOC4 },
            { POS.RP3_SPAWN2_RIFT1, POS.RP3_SPAWN2_RIFT2 }, 5000)
    elseif id == E.RP3_EVENT_ARTHAS_FACE2 then
        creature:SetFacingTo(1.762783) -- @todo
    elseif id == E.RP3_EVENT_SPAWN2_AGGRO then
        engageInfinites(creature)
        scheduleActionOOC(creature, st, RP3_ACTION_AFTER_SPAWN2)
    elseif id == E.RP3_EVENT_ARTHAS21 then
        talkerEntry, talkerLine = 0, 24
    elseif id == E.RP3_EVENT_ARTHAS_MOVE_3 then
        creature:MoveAlongSplineChain(RP3_POINTID_ARTHAS5, RP3_CHAIN_ARTHAS5, false)
    elseif id == E.RP3_EVENT_SPAWN3 then
        spawnInfiniteWave(creature,
            { POS.RP3_SPAWN3_LOC1, POS.RP3_SPAWN3_LOC2, POS.RP3_SPAWN3_LOC3,
                POS.RP3_SPAWN3_LOC4 },
            { RP3_CHAIN_SPAWN3_LOC1, RP3_CHAIN_SPAWN3_LOC2,
                RP3_CHAIN_SPAWN3_LOC3, RP3_CHAIN_SPAWN3_LOC4 },
            { POS.RP3_SPAWN3_RIFT1, POS.RP3_SPAWN3_RIFT2 }, 5000)
    elseif id == E.RP3_EVENT_ARTHAS30 then
        talkerEntry, talkerLine = 0, 25
    elseif id == E.RP3_EVENT_SPAWN3_AGGRO then
        engageInfinites(creature)
        scheduleActionOOC(creature, st, RP3_ACTION_AFTER_SPAWN3)
    elseif id == E.RP3_EVENT_EPOCH_SPAWN then
        local epoch = creature:SummonCreature(NPC_EPOCH,
            POS.RP3_EPOCH_SPAWN[1], POS.RP3_EPOCH_SPAWN[2],
            POS.RP3_EPOCH_SPAWN[3], POS.RP3_EPOCH_SPAWN[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if epoch then
            epoch:MoveAlongSplineChain(0, RP3_CHAIN_EPOCH, false)
        end
        local rift = creature:SummonCreature(NPC_TIME_RIFT_LARGE,
            POS.RP3_EPOCH_RIFT[1], POS.RP3_EPOCH_RIFT[2],
            POS.RP3_EPOCH_RIFT[3], POS.RP3_EPOCH_RIFT[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if rift then
            rift:DespawnOrUnsummon(27000)
        end
    elseif id == E.RP3_EVENT_ARTHAS31 then
        talkerEntry, talkerLine = 0, 26
        creature:SetFacingTo(6.073746) -- @todo
    elseif id == E.RP3_EVENT_EPOCH_FACE then
        local epoch = nearest(creature, NPC_EPOCH, 100)
        if epoch then
            epoch:SetFacingToObject(creature)
        end
    elseif id == E.RP3_EVENT_EPOCH1 then
        talkerEntry, talkerLine = NPC_EPOCH, 0
    elseif id == E.RP3_EVENT_ARTHAS32 then
        talkerEntry, talkerLine = 0, 27
    elseif id == E.RP3_EVENT_ARTHAS32_2 then
        creature:PerformEmote(EMOTE_ONESHOT_POINT_NO_SHEATHE)
    elseif id == E.RP3_EVENT_EPOCH_AGGRO then
        talkerEntry, talkerLine = NPC_EPOCH, 1
        local epoch = nearest(creature, NPC_EPOCH, 100)
        if epoch then
            epoch:SetImmuneToAll(false)
            creature:EngageWithTarget(epoch)
            epoch:EngageWithTarget(creature)
        end
        scheduleActionOOC(creature, st, RP3_ACTION_AFTER_EPOCH)
    elseif id == E.RP4_EVENT_ARTHAS2 then
        creature:SetFacingTo(0.226893) -- @todo
        talkerEntry, talkerLine = 0, 29
    elseif id == E.RP4_EVENT_HIDDEN_PASSAGE then
        local passage = creature:FindNearestGameObject(GO_HIDDEN_PASSAGE, 100)
        if passage then
            passage:SetGoState(GO_STATE_ACTIVE)
        end
    elseif id == E.RP4_EVENT_ARTHAS3 then
        talkerEntry, talkerLine = 0, 30
        creature:MoveAlongSplineChain(RP4_POINTID_ARTHAS2, RP4_CHAIN_ARTHAS2, false)
    elseif id == E.RP4_EVENT_GAUNTLET_REACHED then
        setInstanceFlag(creature, "arthasGone")
        cancelRpPump(creature, st)
        creature:DespawnOrUnsummon()
        setInstanceFlag(creature, "gauntletReached")
    elseif id == E.RP4_EVENT_ARTHAS_MOVE then
        creature:MoveAlongSplineChain(RP4_POINTID_GAUNTLET1, RP4_CHAIN_GAUNTLET1, false)
    elseif id == E.RP4_EVENT_ARTHAS11 then
        talkerEntry, talkerLine = 0, 32
    elseif id == E.RP4_EVENT_ARTHAS_FACE then
        creature:SetFacingTo(1.780236) -- @todo
    elseif id == E.RP4_EVENT_ARTHAS12 then
        talkerEntry, talkerLine = 0, 33
    elseif id == E.RP4_EVENT_GAUNTLET_RESUME then
        creature:MoveAlongSplineChain(RP4_POINTID_GAUNTLET2, RP4_CHAIN_GAUNTLET2, false)
    elseif id == E.RP4_EVENT_ARTHAS13 then
        talkerEntry, talkerLine = 0, 34
        creature:SetFacingTo(POS.ARTHAS_GAUNTLET_END_POS[4])
    elseif id == E.RP4_EVENT_GAUNTLET_DONE then
        creature:PerformEmote(EMOTE_ONESHOT_POINT_NO_SHEATHE)
        setInstanceFlag(creature, "gauntletDone")
    elseif id == E.RP5_EVENT_ARTHAS2 then
        talkerEntry, talkerLine = 0, 36
        local malganis = nearest(creature, NPC_MALGANIS, 100)
        if malganis then
            creature:SetFacingToObject(malganis)
        end
    elseif id == E.RP5_EVENT_MALGANIS1 then
        local malganis = nearest(creature, NPC_MALGANIS, 100)
        if malganis then
            malganis:Talk(2) -- RP5_LINE_MALGANIS1
            malganis:SetImmuneToAll(false)
            creature:EngageWithTarget(malganis)
            malganis:EngageWithTarget(creature)
        end
        scheduleActionOOC(creature, st, RP5_ACTION_AFTER_MALGANIS)
    elseif id == E.RP5_EVENT_MALGANIS_DONE then
        local malganis = nearest(creature, NPC_MALGANIS, 100)
        if malganis then
            malganis:SetFacingToObject(creature)
            malganis:CastSpell(malganis, SPELL_MALGANIS_QUEST_CREDIT, true)
            malganis:CastSpell(malganis, SPELL_MALGANIS_KILL_CREDIT, true)
            local chestEntry = creature:IsHeroic() and GO_CHEST_HEROIC or GO_CHEST_NORMAL
            local chest = malganis:FindNearestGameObject(chestEntry, 100)
            if chest then
                chest:RemoveFlag(GAMEOBJECT_FLAGS_FIELD, GO_FLAG_NOT_SELECTABLE)
            end
        end
        scheduleRpEvent(creature, st, E.RP5_EVENT_MALGANIS12, 3000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_MALGANIS_LEAVE, 19000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_ARTHAS10, 20000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_ARTHAS10_2, 27000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_ARTHAS11, 30000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_ARTHAS11_2, 35000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_ARTHAS_LEAVE, 45000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_CHROMIE_SPAWN, 65000)
        scheduleRpEvent(creature, st, E.RP5_EVENT_CHROMIE_LAND, 76668)
        scheduleRpEvent(creature, st, E.RP5_EVENT_CHROMIE_TRANSFORM, 79491)
        creature:SetBossState(DATA_MAL_GANIS, BOSS_STATE_DONE)
        setInstanceFlag(creature, "malganisDone")
    elseif id == E.RP5_EVENT_MALGANIS12 then
        talkerEntry, talkerLine = NPC_MALGANIS, 10
    elseif id == E.RP5_EVENT_MALGANIS_LEAVE then
        local malganis = nearest(creature, NPC_MALGANIS, 100)
        if malganis then
            malganis:CastSpell(malganis, SPELL_SHADOWSTEP_VISUAL)
        end
    elseif id == E.RP5_EVENT_ARTHAS10 then
        local malganis = nearest(creature, NPC_MALGANIS, 100)
        if malganis then
            malganis:DespawnOrUnsummon()
        end
        creature:MoveAlongSplineChain(0, RP5_CHAIN_ARTHAS2, false)
        talkerEntry, talkerLine = 0, 37
    elseif id == E.RP5_EVENT_ARTHAS10_2 then
        creature:PerformEmote(EMOTE_ONESHOT_EXCLAMATION)
    elseif id == E.RP5_EVENT_ARTHAS11 then
        talkerEntry, talkerLine = 0, 38
    elseif id == E.RP5_EVENT_ARTHAS11_2 then
        creature:PerformEmote(EMOTE_ONESHOT_TALK)
    elseif id == E.RP5_EVENT_ARTHAS_LEAVE then
        creature:MoveAlongSplineChain(RP5_POINTID_ARTHAS3, RP5_CHAIN_ARTHAS3, true)
    elseif id == E.RP5_EVENT_ARTHAS_LEAVE2 then
        creature:MoveAlongSplineChain(RP5_POINTID_ARTHAS4, RP5_CHAIN_ARTHAS4, true)
    elseif id == E.RP5_EVENT_CHROMIE_SPAWN then
        local chromie = creature:SummonCreature(NPC_CHROMIE_3,
            POS.RP5_CHROMIE_SPAWN[1], POS.RP5_CHROMIE_SPAWN[2],
            POS.RP5_CHROMIE_SPAWN[3], POS.RP5_CHROMIE_SPAWN[4],
            SUMMON_MANUAL_DESPAWN, 0)
        if chromie then
            chromie:RemoveFlag(UNIT_NPC_FLAGS_FIELD,
                UNIT_NPC_FLAG_GOSSIP + UNIT_NPC_FLAG_QUESTGIVER)
            -- LaunchMoveSpline fly path: no bridge (stay-out).
        end
    elseif id == E.RP5_EVENT_CHROMIE_LAND then
        -- SetByteValue(UNIT_FIELD_BYTES_1, 3, 0) (anim-tier byte) has no
        -- bridge — LAND is a no-op; TRANSFORM below still runs.
    elseif id == E.RP5_EVENT_CHROMIE_TRANSFORM then
        local chromie = nearest(creature, NPC_CHROMIE_3, 100)
        if chromie then
            chromie:CastSpell(chromie, SPELL_CHROMIE_3_TRANSFORM)
            chromie:Talk(0) -- RP5_LINE_CHROMIE0
            chromie:SetFlag(UNIT_NPC_FLAGS_FIELD,
                UNIT_NPC_FLAG_GOSSIP + UNIT_NPC_FLAG_QUESTGIVER)
        end
    end

    -- C++ post-switch talker dispatch (npc_arthas.cpp:1481-1490).
    if talkerEntry ~= nil then
        local talker
        if talkerEntry == 0 then
            talker = creature
        else
            talker = nearest(creature, talkerEntry, 100)
        end
        if talker then
            talker:Talk(talkerLine)
        end
    end
end

-- MovementInform (npc_arthas.cpp:715-902). Motion types: POINT 8, EFFECT
-- 16, SPLINE_CHAIN 17. The same dispatch serves arthas's own chains and
-- the Uther/Jaina forwards (their ids share the one PointIDs enum).
local function onMovementInformFor(arthas, type, id)
    local st = rpFor(arthas)
    if not st.progressRP then
        return
    end
    if type ~= 8 and type ~= 16 and type ~= 17 then
        return
    end
    if id == RP1_POINTID_UTHER1 then
        scheduleRpEvent(arthas, st, E.RP1_EVENT_START1, 0)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_START2, 1000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS1, 4000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER1, 8000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS2, 15000)
    elseif id == RP1_POINTID_UTHER2 then
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER_FACE, 0)
    elseif id == RP1_POINTID_JAINA2 then
        scheduleRpEvent(arthas, st, E.RP1_EVENT_JAINA_FACE, 0)
    elseif id == RP1_POINTID_ARTHAS2 then
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS3, 1000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER2, 12000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS_TURN, 13000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS4, 14000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER3, 18000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS5, 24000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER4, 30000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS6, 36000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER5, 39000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS7, 44000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_JAINA1, 57000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS8, 59000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS8_2, 65000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER6, 71000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_UTHER_LEAVE, 74000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_JAINA_LEAVE, 75000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS9, 76000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_JAINA2, 78000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_JAINA_LEAVE2, 82000)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS_LEAVE, 88000)
    elseif id == RP1_POINTID_UTHER3 then
        local uther = nearest(arthas, NPC_UTHER, 500)
        if uther then
            uther:DespawnOrUnsummon()
        end
    elseif id == RP1_POINTID_JAINA4 then
        local jaina = nearest(arthas, NPC_JAINA, 500)
        if jaina then
            jaina:DespawnOrUnsummon()
        end
    elseif id == RP1_POINTID_ARTHAS3 then
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS10, 0)
        scheduleRpEvent(arthas, st, E.RP1_EVENT_ARTHAS_LEAVE2, 12000)
    elseif id == RP1_POINTID_ARTHAS4 then
        scheduleRpEvent(arthas, st, E.RP1_EVENT_FINISHED, 0)
    elseif id == RP2_POINTID_ARTHAS1 then
        local citizen = nearest(arthas, NPC_CITIZEN, 100)
        if citizen then
            citizen:SetFacingToObject(arthas)
        end
        local resident = nearest(arthas, NPC_RESIDENT, 100)
        if resident then
            resident:SetFacingToObject(arthas)
        end
        scheduleRpEvent(arthas, st, E.RP2_EVENT_CITIZEN1, 2000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS2, 10000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS_MOVE_2, 11000)
    elseif id == RP2_POINTID_ARTHAS2 then
        scheduleRpEvent(arthas, st, E.RP2_EVENT_CITIZEN2, 0)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_KILL1, 1000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS_MOVE_3, 3000)
    elseif id == RP2_POINTID_ARTHAS3 then
        scheduleRpEvent(arthas, st, E.RP2_EVENT_KILL2, 1000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_REACT1, 2000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_REACT2, 3000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_REACT3, 4000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_REACT4, 6000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_REACT5, 6000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS_MOVE_4, 4000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS3, 6000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_MALGANIS1, 10000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_TROOPS_FACE, 11000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS_FACE, 13000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_MALGANIS2, 22000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_MALGANIS_LEAVE1, 34000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_MALGANIS_LEAVE2, 35000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS4, 35000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS4_2, 39000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS_MOVE_5, 45000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS5, 45000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS5_2, 51000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_ARTHAS5_3, 57000)
        scheduleRpEvent(arthas, st, E.RP2_EVENT_WAVE_START, 64000)
    elseif id == RP3_POINTID_ARTHAS1 then
        for _, infinite in ipairs(arthas:GetCreaturesInGrid(NPC_CITIZEN_INFINITE, 100)) do
            infinite:SetFacingToObject(arthas)
        end
        scheduleRpEvent(arthas, st, E.RP3_EVENT_RESIDENT_FACE, 1000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS_FACE, 2000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_CITIZEN1, 3000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS2, 12000)
    elseif id == RP3_POINTID_ARTHAS2 then
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS_KILL, 1000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_INFINITE_LAUGH, 2000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS3, 6000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_CITIZEN2, 8000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_TRANSFORM1, 10000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_TRANSFORM2, 12000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_TRANSFORM3, 14000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_AGGRO, 15000)
    elseif id == RP3_POINTID_ARTHAS3 then
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS_MOVE_1_2, 1000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN1, 2000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN1_FACE, 5000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN1_AGGRO, 7000)
    elseif id == RP3_POINTID_ARTHAS4 then
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS20, 1000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN2, 2000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS_FACE2, 3000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN2_FACE, 4000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN2_AGGRO, 6000)
    elseif id == RP3_POINTID_ARTHAS5 then
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN3, 2000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_ARTHAS30, 4000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN3_FACE, 5000)
        scheduleRpEvent(arthas, st, E.RP3_EVENT_SPAWN3_AGGRO, 6000)
    elseif id == RP4_POINTID_ARTHAS1 then
        scheduleRpEvent(arthas, st, E.RP4_EVENT_ARTHAS2, 1000)
        scheduleRpEvent(arthas, st, E.RP4_EVENT_HIDDEN_PASSAGE, 4000) -- @todo sniff timer
        scheduleRpEvent(arthas, st, E.RP4_EVENT_ARTHAS3, 5000)
    elseif id == RP4_POINTID_ARTHAS2 then
        scheduleRpEvent(arthas, st, E.RP4_EVENT_GAUNTLET_REACHED, 1000)
    elseif id == RP4_POINTID_GAUNTLET1 then
        scheduleRpEvent(arthas, st, E.RP4_EVENT_ARTHAS11, 1000)
        scheduleRpEvent(arthas, st, E.RP4_EVENT_ARTHAS_FACE, 5000)
        scheduleRpEvent(arthas, st, E.RP4_EVENT_ARTHAS12, 21000)
        scheduleRpEvent(arthas, st, E.RP4_EVENT_GAUNTLET_RESUME, 25000)
    elseif id == RP4_POINTID_GAUNTLET2 then
        scheduleRpEvent(arthas, st, E.RP4_EVENT_ARTHAS13, 1000)
        scheduleRpEvent(arthas, st, E.RP4_EVENT_GAUNTLET_DONE, 7000)
    elseif id == RP5_POINTID_ARTHAS1 then
        scheduleRpEvent(arthas, st, E.RP5_EVENT_ARTHAS2, 1000)
        scheduleRpEvent(arthas, st, E.RP5_EVENT_MALGANIS1, 6000)
    elseif id == RP5_POINTID_ARTHAS3 then
        scheduleRpEvent(arthas, st, E.RP5_EVENT_ARTHAS_LEAVE2, 0)
    elseif id == RP5_POINTID_ARTHAS4 then
        local p = POS.ARTHAS_FINAL_POS
        arthas:Teleport(p[1], p[2], p[3], p[4])
    end
    -- RP3_POINTID_TOWN_HALL (12) has no C++ case.
end

local function onMovementInform(event, creature, type, id)
    onMovementInformFor(creature, type, id)
end

-- npc_stratholme_rp_dummy (npc_arthas.cpp:1616-1632): forwards movement
-- informs to the summoner's AI. Citizen/resident forwarding is vacuous
-- (no switch case for their chain ids) — not registered.
local function onRpDummyMovementInform(event, creature, type, id)
    local summoner = creature:GetSummonerUnit()
    if summoner then
        onMovementInformFor(summoner, type, id)
    end
end

local function onDoAction(event, creature, action)
    doAction(creature, rpFor(creature), action)
end

-- Threat-list pool (Oct-25 CoS strip idiom): C++ SelectTarget draws
-- the threat list (UnitAI.h); dist 0 means no range filter, and
-- playerOnly defaults false, so any alive threat unit qualifies.
local function randomThreatUnit(creature)
    local targets = creature:GetAITargets()
    local found = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- C++ UpdateAICombat legs: HealthBelowPct(40) -> DoCastSelf(52444) every
-- AI tick; _exorcismCooldown (init urand(7,14)s from the AI ctor) on
-- expiry -> DoCast(SelectTarget(Random, 0), 52445), then re-arm
-- urand(7,14)s. 1s per-GUID pump (sironas convention); C++'s per-tick
-- checks make this an approximation (like old_hillsbrad). The C++
--   "still casting -> retry next tick" leg is PORTED (npc_arthas.cpp:899):
--   while IsCasting the pump sets exorcism = 0 and retries next 1s tick.
local function startCombatPump(creature, guid)
    local exorcism = math.random(7, 14)
    pumps[guid] = CreateLuaEvent(function()
        local maxHealth = creature:GetMaxHealth()
        if maxHealth > 0
                and creature:GetHealth() < math.floor(maxHealth * 40 / 100) then
            creature:CastSpell(creature, SPELL_HOLY_LIGHT)
        end
        exorcism = exorcism - 1
        if exorcism <= 0 then
            if creature:IsCasting() then
                exorcism = 0 -- C++ UpdateAICombat (npc_arthas.cpp:899): still casting -> _exorcismCooldown = 0, retry next tick; no cast, no re-arm
            else
                local target = randomThreatUnit(creature)
                if target then
                    creature:CastSpell(target, SPELL_EXORCISM)
                    exorcism = math.random(7, 14)
                else
                    exorcism = 0 -- C++ leaves the cooldown unset on nil target; retry next tick
                end
            end
        end
    end, 1000, 0)
end

-- C++ JustEngagedWith (npc_arthas.cpp:1537-1552): _progressRP = false +
-- SetHomePosition(current position); the SplineChainResumeInfo capture
-- stays out (no ResumeSplineChain bridge). Then the combat pump starts.
local function onEnterCombat(event, creature, target)
    local st = rpFor(creature)
    st.progressRP = false
    st.inCombat = true
    creature:SetHomePosition(creature:GetX(), creature:GetY(),
        creature:GetZ(), creature:GetO())
    local guid = creature:GetGUID()
    cancelPump(guid)
    startCombatPump(creature, guid)
end

local function onLeaveCombat(event, creature)
    rpFor(creature).inCombat = false
    cancelPump(creature:GetGUID())
end

local function onReset(event, creature)
    cancelPump(creature:GetGUID())
end

-- C++ JustReachedHome (npc_arthas.cpp:1566-1584): devotion aura if
-- missing, _progressRP = true, resume spline chain (stay-out), then flush
-- _afterCombat via DoAction.
local function onReachHome(event, creature)
    local st = rpFor(creature)
    if not creature:HasAura(SPELL_DEVOTION_AURA) then
        creature:CastSpell(creature, SPELL_DEVOTION_AURA)
    end
    st.progressRP = true
    st.inCombat = false
    if st.afterCombat ~= ACTION_NONE then
        local action = st.afterCombat
        st.afterCombat = ACTION_NONE
        doAction(creature, st, action)
    end
end

local function onDied(event, creature, killer)
    cancelPump(creature:GetGUID())
    cancelRpPump(creature, rpFor(creature))
    -- C++ JustDied (npc_arthas.cpp:1578-1582): DespawnOrUnsummon(5000) +
    -- instance SetData(DATA_ARTHAS_DIED) — the SetData leg rides the
    -- shared instanceData table: instance_culling_of_stratholme.lua
    -- consumes the flag on its ON_UPDATE pump and regresses to the
    -- stable state, forced.
    local data = creature:GetMap():GetInstanceData()
    if data then
        data.arthasDied = true
    end
    creature:DespawnOrUnsummon(5000)
end

-- C++ JustAppeared (npc_arthas.cpp:1585-1590): _progressRP = true,
-- AdvanceToState(current progress), DoCastSelf(SPELL_DEVOTION_AURA).
local function onSpawn(event, creature)
    local st = rpFor(creature)
    st.progressRP = true
    advanceToState(creature, st, currentProgress(creature))
    creature:CastSpell(creature, SPELL_DEVOTION_AURA)
end

-- C++ OnGossipHello (npc_arthas.cpp:1604-1607) returns false: the default
-- DB gossip menu proceeds (gossip.go hello veto: true vetoes, false lets
-- the menu show).
local function onGossipHello(event, player, creature)
    return false
end

-- C++ OnGossipSelect (npc_arthas.cpp:1593-1602) + AdvanceDungeon
-- (npc_arthas.cpp:1592-1596): GetGossipActionFor is computed but never
-- branched on — all five AdvanceDungeon calls run, each gated on its
-- from-state. The matching flag is written; the instance pump implements
-- InitiateArthasEvent (from->to guard + setProgress + DoAction(-action)).
-- Returns true (C++ returns true: handled).
local function onGossipSelect(event, player, creature, sender, action)
    local data = creature:GetMap():GetInstanceData()
    if not data then
        return true
    end
    local progress = data.progress or JUST_STARTED
    if progress == PURGE_PENDING then
        data.startPurge = true
    elseif progress == TOWN_HALL_PENDING then
        data.startTownHall = true
    elseif progress == TOWN_HALL_COMPLETE then
        data.toGauntlet = true
    elseif progress == GAUNTLET_PENDING then
        data.startGauntlet = true
    elseif progress == GAUNTLET_COMPLETE then
        data.startMalganis = true
    end
    return true
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    if victim:GetEntry() == NPC_RISEN_ZOMBIE then
        creature:Talk(LINE_SLAY_ZOMBIE, victim)
    end
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY, 24, onReachHome)
RegisterCreatureEvent(ENTRY, 28, onMovementInform)
RegisterCreatureEvent(ENTRY, 38, onDoAction)
RegisterCreatureEvent(NPC_UTHER, 28, onRpDummyMovementInform)
RegisterCreatureEvent(NPC_JAINA, 28, onRpDummyMovementInform)
RegisterCreatureGossipEvent(ENTRY, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY, 2, onGossipSelect)
