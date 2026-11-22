// SmartAI engine — item-6 census Axis1#2 (slice 1: engine + hooks + TALK /
// SET_FACTION / PLAY_EMOTE / CAST / AUTO_ATTACK / SET_EVENT_PHASE;
// slice 2: the remaining AGGRO-used actions and targets; slice 3:
// timed-action lists (CALL_TIMED_ACTIONLIST / RANDOM / RANGE + the
// list engine below), the phase actions (INC/RANDOM/RANDOM_RANGE),
// ATTACK_START, and the small state actions (SET_REACT_STATE,
// ALLOW_COMBAT_MOVEMENT, REMOVE_NPC_FLAG, SET_ROOT, SET_VISIBILITY,
// SET_ACTIVE, INTERRUPT_SPELL) — of which the phase actions,
// ATTACK_START, SET_REACT_STATE, the NPC-flag family,
// ALLOW_COMBAT_MOVEMENT, SET_ROOT, SET_VISIBILITY and INTERRUPT_SPELL
// are ported; SET_ACTIVE stays residual.
// 29 FOLLOW (114 live rows), 32 RESET_GOBJECT (30) and 33
// CALL_KILLEDMONSTER (410) are ported below with the
// FOLLOW_COMPLETED event (65), the group kill-credit fan-out, and
// the CLOSEST_PLAYER (21) + HOSTILE_RANDOM (5) target arms — the
// latter also serves 1108 CAST rows. 37 DIE (61 live rows), 38
// SET_IN_COMBAT_WITH_ZONE (34) and 40 SET_SHEATH (31) are ported
// below (the DEATH dispatch rides the insta-kill path, the zone
// engage is the shared creatureDoZoneInCombat helper). 44
// SET_INGAME_PHASE_MASK (42 rows, 36 live), 46 ATTACK_STOP (8)
// and 51 KILL_UNIT (47 rows, 35 live) are ported below. 50
// SUMMON_GO (226 rows, 174 live) and 52 ACTIVATE_TAXI (1 row)
// are ported below. 53 WP_START (930 live rows) / 54 WP_PAUSE
// (674) / 55 WP_STOP (12) are ported below with the SmartAI
// StartPath/PausePath/StopPath escort machine (waypoints-table
// paths, WAYPOINT_* events, UpdatePath/UpdateDespawn ticks). The
// 56 ADD_ITEM (0 rows) / 57 REMOVE_ITEM (8) / 58 INSTALL_AI_TEMPLATE
// (0 rows) / 59 SET_RUN (266) / 60 SET_DISABLE_GRAVITY (50) / 61
// SET_SWIM (0 rows) / 62 TELEPORT (11) / 63 SET_COUNTER (40) are
// ported below with the COUNTER_SET (77) event dispatch. The next
// 65 WP_RESUME (40 live rows) / 66 SET_ORIENTATION (1194) / 68 PLAYMOVIE
// (3) / 69 MOVE_TO_POS (579) / 70 ENABLE_TEMP_GOBJ (57) / 71 EQUIP (124)
// / 72 CLOSE_GOSSIP (557) are ported below. The 75 ADD_AURA (0 rows)
// / 76 OVERRIDE_SCRIPT_BASE_OBJECT (0 rows) / 77 RESET_SCRIPT_BASE_OBJECT
// (0 rows) / 78 CALL_SCRIPT_RESET (50) / 79 SET_RANGED_MOVEMENT (9) are
// ported below (the base-object override rebinds `me` for later actions,
// the reset re-inits timers + fires RESET events, the ranged leg arms
// the ScriptChase slot). The 84 SIMPLE_TALK (4 live rows) / 85 SELF_CAST
// (481) / 86 CROSS_CAST (127) are ported below with the VEHICLE_PASSENGER
// (29) target arm. The 89 RANDOM_MOVE (131) / 90 SET_UNIT_FIELD_BYTES_1
// (297) / 97 JUMP_TO_POS (12) are ported below. The next unported block
// is 98 SEND_GOSSIP_MENU (14) / 99 GO_SET_LOOT_STATE (22) / 100
// SEND_TARGET_TO_TARGET (155); 93/94/95/96 have zero TDB rows.
//
// C++: SmartScript / SmartAI (src/server/game/AI/SmartScripts/SmartScript.cpp,
// SmartAI.cpp, SmartScriptMgr.h). Go had no SmartAI engine and no smart_scripts
// loader (the WORLD_SEL_SMART_SCRIPTS statement existed unused, the .reload
// arm was a probe-only stub), so every SmartAI creature's on-aggro script was
// silently dead. This file loads the creature-source rows (source_type 0), keeps
// per-creature runtime state on the motion, and dispatches the direct events
// (AGGRO, DEATH, DATA_SET, AI_INIT, JUST_CREATED, TEXT_OVER,
// EVENT_PHASE_CHANGE) plus the timer events (UPDATE, UPDATE_IC, UPDATE_OOC,
// HEALTH_PCT, TARGET_HEALTH_PCT, MANA_PCT, TARGET_MANA_PCT, RANGE) from the
// creature tick. Action coverage is the AGGRO-used set (TDB 26091: 1,634
// source_type-0 AGGRO rows over 1,426 entries): TALK, CAST, SET_FACTION,
// PLAY_EMOTE, AUTO_ATTACK, SET_EVENT_PHASE, SET_INST_DATA (type 1),
// CALL_FOR_HELP, SET_DATA, SOUND, STORE_TARGET_LIST, REMOVEAURASFROMSPELL,
// REMOVE_UNIT_FIELD_BYTES_1 (byte 0 only), SUMMON_CREATURE,
// CALL_TIMED_ACTIONLIST + CALL_RANDOM_TIMED_ACTIONLIST +
// CALL_RANDOM_RANGE_TIMED_ACTIONLIST (source_type-9 timed-action lists,
// installed per target creature, stepped from smartUpdateTimers), and
// SET_REACT_STATE + the SET/ADD/REMOVE_NPC_FLAG family, INC_EVENT_PHASE +
// RANDOM_PHASE + RANDOM_PHASE_RANGE + ATTACK_START, ALLOW_COMBAT_MOVEMENT +
// SET_ROOT + SET_VISIBILITY + INTERRUPT_SPELL + per-target PLAY_EMOTE.
// RESET wiring
// landed with smartReset / smartJustReachedHome (evade-follow legs, home
// arrival, spawn): the EVADE event fires from triggerCreatureEvade, RESPAWN +
// OnReset from fireCreatureSpawned. `.reload smart_scripts` re-runs the
// loader (reloadSmartScripts). Everything else (GO/instance/area-trigger
// sources) is a documented residual in
// smartProcessAction / the call sites below, not a silent gap.
//
// Locking: every entry point assumes motionMu is NOT held (the Nov-17
// motionMu self-deadlock). The three hook sites run after their local unlock:
// triggerCreatureAggro tail, applyCreatureDeathState post-unlock,
// stepCreatureMotion (motionMu is not held on that path, :7026).

package world

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// SMART_EVENT ids (SmartScriptMgr.h:97-183).
const (
	smartEventUpdateIC            uint32 = 0
	smartEventUpdateOOC           uint32 = 1
	smartEventHealthPct           uint32 = 2
	smartEventManaPct             uint32 = 3
	smartEventAggro               uint32 = 4
	smartEventKill                uint32 = 5
	smartEventDeath               uint32 = 6
	smartEventEvade               uint32 = 7
	smartEventRange               uint32 = 9
	smartEventRespawn             uint32 = 11
	smartEventReachedHome         uint32 = 21
	smartEventReset               uint32 = 25
	smartEventDataSet             uint32 = 38 // SMART_EVENT_DATA_SET; 26 is SMART_EVENT_IC_LOS, not DATA_SET
	smartEventTargetHealthPc      uint32 = 12
	smartEventTargetManaPct       uint32 = 18
	smartEventAIInit              uint32 = 37
	smartEventTextOver            uint32 = 52
	smartEventTimedEventTriggered uint32 = 59 // SMART_EVENT_TIMED_EVENT_TRIGGERED (SmartScriptMgr.h:158), event_param1 = timed-event id
	smartEventFollowCompleted     uint32 = 65 // SMART_EVENT_FOLLOW_COMPLETED (SmartScriptMgr.h:164)
	smartEventMovementInform      uint32 = 34 // SMART_EVENT_MOVEMENTINFORM (SmartScriptMgr.h:133), var0 = movement type, var1 = point id
	smartEventWaypointStart       uint32 = 39 // SMART_EVENT_WAYPOINT_START (SmartScriptMgr.h:138), var0 = node id, var1 = path id
	smartEventWaypointReached     uint32 = 40 // SMART_EVENT_WAYPOINT_REACHED (SmartScriptMgr.h:139)
	smartEventWaypointPaused      uint32 = 55 // SMART_EVENT_WAYPOINT_PAUSED (SmartScriptMgr.h:154)
	smartEventWaypointResumed     uint32 = 56 // SMART_EVENT_WAYPOINT_RESUMED (SmartScriptMgr.h:155)
	smartEventWaypointStopped     uint32 = 57 // SMART_EVENT_WAYPOINT_STOPPED (SmartScriptMgr.h:156)
	smartEventWaypointEnded       uint32 = 58 // SMART_EVENT_WAYPOINT_ENDED (SmartScriptMgr.h:157)
	smartEventUpdate              uint32 = 60
	smartEventLink                uint32 = 61
	smartEventGossipSelect        uint32 = 62 // SMART_EVENT_GOSSIP_SELECT (SmartScriptMgr.h:161), event_param1 = menuID (sender), event_param2 = gossip list id (action)
	smartEventJustCreated         uint32 = 63
	smartEventGossipHello         uint32 = 64 // SMART_EVENT_GOSSIP_HELLO (SmartScriptMgr.h:163)
	smartEventPhaseChange         uint32 = 66
	smartEventGOLootStateChanged  uint32 = 70 // SMART_EVENT_GO_LOOT_STATE_CHANGED (SmartScriptMgr.h:169), var0 = loot state
	smartEventCounterSet          uint32 = 77 // SMART_EVENT_COUNTER_SET (SmartScriptMgr.h:176), var0 = counter id
	smartEventPhase12             uint32 = 12 // SMART_EVENT_PHASE_12 (SmartScriptMgr.h:55), the IncPhase clamp (SmartScript.cpp:3996-4000)
	smartEventPassengerBoarded    uint32 = 27 // SMART_EVENT_PASSENGER_BOARDED (SmartScriptMgr.h:126), var0 = seat id
	smartEventPassengerRemoved    uint32 = 28 // SMART_EVENT_PASSENGER_REMOVED (SmartScriptMgr.h:127), var0 = seat id
	smartEventJustSummoned        uint32 = 54 // SMART_EVENT_JUST_SUMMONED (SmartScriptMgr.h:153)
	smartEventOnSpellClick        uint32 = 73 // SMART_EVENT_ON_SPELLCLICK (SmartScriptMgr.h:172), invoker = clicker
	smartEventSummonedUnit        uint32 = 17 // SMART_EVENT_SUMMONED_UNIT (SmartScriptMgr.h:116), invoker = summoned creature
	smartEventSummonedUnitDies    uint32 = 82 // SMART_EVENT_SUMMONED_UNIT_DIES (SmartScriptMgr.h:181), invoker = dead summoned creature
	smartEventEnd                 uint32 = 83 // SMART_EVENT_END (SmartScriptMgr.h:183), loader rejects event types >= this
)

// SMART_ACTION ids (SmartScriptMgr.h:457-600), ported set. Actions
// >= SMART_ACTION_END (140) never reach dispatch: the C++ loader
// rejects them (SmartScriptMgr::IsEventValid, SmartScriptMgr.cpp:682-686),
// so their TDB rows (142:11 / 144:306 / 145:296 / 146:70 / 147:127)
// are dropped at load by loadSmartScripts below — the old "hit the
// dispatch default exactly as in C++" reading was wrong, the loader
// rejects them before any event can fire. The timed-event store
// (67/73/74/125 + mStoredEvents/mRemIDs) is ported below.
// Full action census (TDB 26091, all 140 enum ids x Go consts x rows)
// lives in hidden_files/smartai_action_census.py; 6 FAIL_QUEST (89
// rows), 7 OFFER_QUEST (1 row, directAdd) and 9 ACTIVATE_GOBJECT (60
// live rows) are ported below with the GO/player/owner target arms
// (13/14/15/17/18/20/23). 15 CALL_AREAEXPLOREDOREVENTHAPPENS (187
// live rows), 26 CALL_GROUPEVENTHAPPENS (15) and 27 COMBAT_STOP (13)
// are ported below with the THREAT_LIST (24) and LOOT_RECIPIENTS (27)
// target arms. 29 FOLLOW (114 live rows), 32 RESET_GOBJECT (30) and
// 33 CALL_KILLEDMONSTER (410) are ported below with the
// FOLLOW_COMPLETED event (65), the group kill-credit fan-out, and
// the CLOSEST_PLAYER (21) + HOSTILE_RANDOM (5) target arms — the
// latter also serves 1108 CAST rows. 37 DIE (61 live rows), 38
// SET_IN_COMBAT_WITH_ZONE (34) and 40 SET_SHEATH (31) are ported
// below; the DEATH dispatch rides the insta-kill path and the zone
// engage is the shared creatureDoZoneInCombat helper. 44
// SET_INGAME_PHASE_MASK (42 rows, 36 live), 46 ATTACK_STOP (8)
// and 51 KILL_UNIT (47 rows, 35 live) are ported below. 50
// SUMMON_GO (226 rows, 174 live) and 52 ACTIVATE_TAXI (1 row)
// are ported below. 53 WP_START (930 live rows) / 54 WP_PAUSE
// (674) / 55 WP_STOP (12) are ported below with the SmartAI
// StartPath/PausePath/StopPath escort machine (waypoints-table
// paths, WAYPOINT_* events, UpdatePath/UpdateDespawn ticks). The
// 56 ADD_ITEM (0 rows) / 57 REMOVE_ITEM (8) / 58 INSTALL_AI_TEMPLATE
// (0 rows) / 59 SET_RUN (266) / 60 SET_DISABLE_GRAVITY (50) / 61
// SET_SWIM (0 rows) / 62 TELEPORT (11) / 63 SET_COUNTER (40) are
// ported below with the COUNTER_SET (77) event dispatch. The next
// 65 WP_RESUME (40 live rows) / 66 SET_ORIENTATION (1194) / 68 PLAYMOVIE
// (3) / 69 MOVE_TO_POS (579) / 70 ENABLE_TEMP_GOBJ (57) / 71 EQUIP (124)
// / 72 CLOSE_GOSSIP (557) are ported below. The 75 ADD_AURA (0 rows)
// / 76 OVERRIDE_SCRIPT_BASE_OBJECT (0 rows) / 77 RESET_SCRIPT_BASE_OBJECT
// (0 rows) / 78 CALL_SCRIPT_RESET (50) / 79 SET_RANGED_MOVEMENT (9) are
// ported below. The 84 SIMPLE_TALK (4 rows) / 85 SELF_CAST (481) / 86
// CROSS_CAST (127) are ported below. The 89 RANDOM_MOVE (131) / 90
// SET_UNIT_FIELD_BYTES_1 (297) / 97 JUMP_TO_POS (12) are ported below.
// The 98 SEND_GOSSIP_MENU (14) / 99 GO_SET_LOOT_STATE (22) / 100
// SEND_TARGET_TO_TARGET (155) are ported below. The 101 SET_HOME_POS
// (102) / 102 SET_HEALTH_REGEN (23) / 107 SUMMON_CREATURE_GROUP (231)
// are ported below. The 108 SET_POWER (3) / 111 GAME_EVENT_STOP (2) /
// 112 GAME_EVENT_START (1) are ported below. The 113
// START_CLOSEST_WAYPOINT (2) / 114 MOVE_OFFSET (76) / 115 RANDOM_SOUND
// (14) are ported below. The 116 SET_CORPSE_DELAY (69) / 117
// DISABLE_EVADE (20) / 127 PAUSE_MOVEMENT (8) are ported below. The
// 118 GO_SET_GO_STATE (5) / 131 SPAWN_SPAWNGROUP (5) / 132
// DESPAWN_SPAWNGROUP (12) are ported below. The 134 INVOKER_CAST
// (35) / 138 OVERRIDE_LIGHT (2) / 139 OVERRIDE_WEATHER (2) are
// ported below; 119-126, 128-130, 135-137 and 140 have zero TDB
// rows, and 141-147 have no enum value (SMART_ACTION_END = 140) so
// the loader rejects their rows — with this strip every enum action
// id is ported or zero-row, and no out-of-enum action can fire.
const (
	smartActionTalk                           uint32 = 1
	smartActionSetFaction                     uint32 = 2
	smartActionMorphToEntryOrModel            uint32 = 3
	smartActionSound                          uint32 = 4
	smartActionPlayEmote                      uint32 = 5
	smartActionFailQuest                      uint32 = 6
	smartActionOfferQuest                     uint32 = 7
	smartActionActivateGObject                uint32 = 9
	smartActionSetReactState                  uint32 = 8
	smartActionRandomEmote                    uint32 = 10
	smartActionCast                           uint32 = 11
	smartActionSummonCreature                 uint32 = 12
	smartActionThreatSinglePct                uint32 = 13
	smartActionThreatAllPct                   uint32 = 14
	smartActionCallAreaExploredOrEventHappens uint32 = 15
	smartActionSetEmoteState                  uint32 = 17
	smartActionAutoAttack                     uint32 = 20
	smartActionAllowCombatMovement            uint32 = 21
	smartActionSetEventPhase                  uint32 = 22
	smartActionIncEventPhase                  uint32 = 23
	smartActionEvade                          uint32 = 24
	smartActionFleeForAssist                  uint32 = 25
	smartActionCallGroupEventHappens          uint32 = 26
	smartActionCombatStop                     uint32 = 27
	smartActionRemoveAurasFromSpell           uint32 = 28
	smartActionFollow                         uint32 = 29
	smartActionRandomPhase                    uint32 = 30
	smartActionRandomPhaseRange               uint32 = 31
	smartActionResetGObject                   uint32 = 32
	smartActionCallKilledMonster              uint32 = 33
	smartActionSetInstData                    uint32 = 34
	smartActionUpdateTemplate                 uint32 = 36
	smartActionDie                            uint32 = 37
	smartActionSetInCombatWithZone            uint32 = 38
	smartActionCallForHelp                    uint32 = 39
	smartActionSetSheath                      uint32 = 40
	smartActionForceDespawn                   uint32 = 41
	smartActionSetInvincibilityHPLevel        uint32 = 42
	smartActionMountToEntryOrModel            uint32 = 43
	smartActionSetIngamePhaseMask             uint32 = 44
	smartActionSetData                        uint32 = 45
	smartActionAttackStop                     uint32 = 46
	smartActionSetVisibility                  uint32 = 47
	smartActionSetActive                      uint32 = 48
	smartActionAttackStart                    uint32 = 49
	smartActionSummonGO                       uint32 = 50
	smartActionKillUnit                       uint32 = 51
	smartActionActivateTaxi                   uint32 = 52
	smartActionWPStart                        uint32 = 53
	smartActionWPPause                        uint32 = 54
	smartActionWPStop                         uint32 = 55
	smartActionAddItem                        uint32 = 56
	smartActionRemoveItem                     uint32 = 57
	smartActionInstallAITemplate              uint32 = 58
	smartActionSetRun                         uint32 = 59
	smartActionSetDisableGravity              uint32 = 60
	smartActionSetSwim                        uint32 = 61
	smartActionTeleport                       uint32 = 62
	smartActionSetCounter                     uint32 = 63
	smartActionStoreTargetList                uint32 = 64
	smartActionWPResume                       uint32 = 65
	smartActionSetOrientation                 uint32 = 66
	smartActionPlayMovie                      uint32 = 68
	smartActionMoveToPos                      uint32 = 69
	smartActionEnableTempGObj                 uint32 = 70
	smartActionEquip                          uint32 = 71
	smartActionCloseGossip                    uint32 = 72
	smartActionCreateTimedEvent               uint32 = 67
	smartActionTriggerTimedEvent              uint32 = 73
	smartActionRemoveTimedEvent               uint32 = 74
	smartActionAddAura                        uint32 = 75
	smartActionOverrideScriptBaseObject       uint32 = 76
	smartActionResetScriptBaseObject          uint32 = 77
	smartActionCallScriptReset                uint32 = 78
	smartActionSetRangedMovement              uint32 = 79
	smartActionCallTimedActionList            uint32 = 80
	smartActionSetNPCFlag                     uint32 = 81
	smartActionAddNPCFlag                     uint32 = 82
	smartActionRemoveNPCFlag                  uint32 = 83
	smartActionSimpleTalk                     uint32 = 84
	smartActionSelfCast                       uint32 = 85
	smartActionCrossCast                      uint32 = 86
	smartActionCallRandomTimedActionList      uint32 = 87
	smartActionCallRandomRangeTimedActionList uint32 = 88
	smartActionRandomMove                     uint32 = 89
	smartActionSetUnitFieldBytes1             uint32 = 90
	smartActionRemoveUnitFieldBytes1          uint32 = 91
	smartActionInterruptSpell                 uint32 = 92
	smartActionJumpToPos                      uint32 = 97
	smartActionSendGossipMenu                 uint32 = 98
	smartActionGOSetLootState                 uint32 = 99
	smartActionSendTargetToTarget             uint32 = 100
	smartActionSetHomePos                     uint32 = 101
	smartActionSetHealthRegen                 uint32 = 102
	smartActionSetRoot                        uint32 = 103
	smartActionSummonCreatureGroup            uint32 = 107
	smartActionSetPower                       uint32 = 108
	smartActionGameEventStop                  uint32 = 111
	smartActionGameEventStart                 uint32 = 112
	smartActionStartClosestWaypoint           uint32 = 113
	smartActionMoveOffset                     uint32 = 114
	smartActionRandomSound                    uint32 = 115
	smartActionSetCorpseDelay                 uint32 = 116
	smartActionDisableEvade                   uint32 = 117
	smartActionGoSetGoState                   uint32 = 118
	smartActionPauseMovement                  uint32 = 127
	smartActionSpawnSpawnGroup                uint32 = 131
	smartActionDespawnSpawnGroup              uint32 = 132
	smartActionInvokerCast                    uint32 = 134
	smartActionOverrideLight                  uint32 = 138
	smartActionOverrideWeather                uint32 = 139
	smartActionEnd                            uint32 = 140 // SMART_ACTION_END (SmartScriptMgr.h:600)
	smartActionTriggerRandomTimedEvent        uint32 = 125
	smartActionRespawnBySpawnID               uint32 = 133
)

// SMARTAI_SPAWN_FLAGS (SmartScriptMgr.h:1200-1205), the spawnflags
// decode for SMART_ACTION_SPAWN_SPAWNGROUP (131) and
// SMART_ACTION_DESPAWN_SPAWNGROUP (132).
const (
	smartSpawnFlagIgnoreRespawn uint32 = 0x01
	smartSpawnFlagForceSpawn    uint32 = 0x02
	smartSpawnFlagNoSaveRespawn uint32 = 0x04
)

// SMART_ESCORT_* state flags (SmartAI.h:30-33) and SMART_ESCORT_TARGETS
// (SmartScriptMgr.h:38), the stored-target-list id WP_START banks the
// escort invoker under. SMART_ESCORT_MAX_PLAYER_DIST (SmartAI.h:36) is
// the invoker range check, doubled on instance maps.
const (
	smartEscortNone      uint32 = 0x00
	smartEscortEscorting uint32 = 0x01
	smartEscortReturning uint32 = 0x02
	smartEscortPaused    uint32 = 0x04
)

const smartEscortTargets uint32 = 0xFFFFFF

// SMART_ESCORT_LAST_OOC_POINT (SmartScriptMgr.h:36): the point id an
// escort's return-to-OOC MovePoint carries; its arrival clears the
// evade state (SmartAI::MovementInform, SmartAI.cpp:396-410).
const smartEscortLastOOCPoint uint32 = 0xFFFFFF

// SMART_RANDOM_POINT (SmartScriptMgr.h:37): the point id MOVE_OFFSET
// MovePoints carry; its arrival fires SMART_EVENT_MOVEMENTINFORM (34)
// with (POINT_MOTION_TYPE, 0xFFFFFE) — the CAGED_NPC template's
// credit/despawn/talk legs match on it.
const smartRandomPoint uint32 = 0xFFFFFE

const smartEscortMaxPlayerDist float32 = 60

// SMART_TARGET ids (SmartScriptMgr.h:1221-1253), AGGRO-used set. Target 8
// (POSITION) resolves to no unit — GetTargets returns nothing for it
// (SmartScript.cpp:2864-2867) — and the SUMMON_CREATURE action reads the
// row's absolute target_x/y/z/o itself for that leg; target 13
// (GAMEOBJECT_RANGE) and 21 (CLOSEST_PLAYER) have zero AGGRO rows in
// TDB 26091.
const (
	smartTargetNone              uint32 = 0
	smartTargetSelf              uint32 = 1
	smartTargetVictim            uint32 = 2
	smartTargetHostileRandom     uint32 = 5
	smartTargetActionInvoker     uint32 = 7
	smartTargetPosition          uint32 = 8
	smartTargetCreatureRange     uint32 = 9
	smartTargetCreatureGUID      uint32 = 10
	smartTargetCreatureDist      uint32 = 11
	smartTargetStored            uint32 = 12
	smartTargetGameObjectRange   uint32 = 13
	smartTargetGameObjectGUID    uint32 = 14
	smartTargetGameObjectDist    uint32 = 15
	smartTargetInvokerParty      uint32 = 16
	smartTargetPlayerRange       uint32 = 17
	smartTargetPlayerDist        uint32 = 18
	smartTargetClosestCreature   uint32 = 19
	smartTargetClosestGameObject uint32 = 20
	smartTargetClosestPlayer     uint32 = 21
	smartTargetOwnerOrSummoner   uint32 = 23
	smartTargetThreatList        uint32 = 24
	smartTargetLootRecipients    uint32 = 27
	smartTargetVehiclePassenger  uint32 = 29
)

// SMART_EVENT_FLAG bits (SmartScriptMgr.h:1522-1531).
const (
	smartEventFlagNotRepeatable uint32 = 0x001
	smartEventFlagDontReset     uint32 = 0x100
	smartEventFlagDifficulty0   uint32 = 0x002
	smartEventFlagDifficulty1   uint32 = 0x004
	smartEventFlagDifficulty2   uint32 = 0x008
	smartEventFlagDifficulty3   uint32 = 0x010
	smartEventFlagWhileCharmed  uint32 = 0x200
)

const smartEventFlagDifficultyAll uint32 = smartEventFlagDifficulty0 | smartEventFlagDifficulty1 | smartEventFlagDifficulty2 | smartEventFlagDifficulty3

// SMARTCAST flags (SmartScriptMgr.h:1539-1549). Flag 0x04 (FORCE_CAST)
// is commented out in this C++ revision and has no leg.
const (
	smartCastInterruptPrevious uint32 = 0x01
	smartCastTriggered         uint32 = 0x02
	smartCastAuraNotPresent    uint32 = 0x20
	smartCastCombatMove        uint32 = 0x40
)

// SMART_EVENT_PHASE bits (SmartScriptMgr.h:61-78): phase 1 = bit 1, ...,
// phase 12 = bit 2048, all = 4095. SetPhase stores the phase NUMBER verbatim
// (SmartScript.cpp:4010-4014) and IsInPhase (:4020-4025) tests
// (1 << (phase-1)) & mask.
const smartEventPhaseAll uint32 = 4095

const smartMaxNestedEvents = 10

// smartScriptRow is one smart_scripts row (SmartScriptHolder: SmartScriptMgr.h).
// entryOrGUID is int32, not uint32: the DB column is a signed int and
// negative values are spawn-GUID-specific rows (SmartScriptMgr convention),
// which a uint32 scan would drop as out-of-range.
type smartScriptRow struct {
	entryOrGUID    int32
	sourceType     uint32
	id             uint32
	link           uint32
	eventType      uint32
	eventPhaseMask uint32
	eventChance    uint32
	eventFlags     uint32
	eventParam1    uint32
	eventParam2    uint32
	eventParam3    uint32
	eventParam4    uint32
	eventParam5    uint32
	actionType     uint32
	actionParam1   uint32
	actionParam2   uint32
	actionParam3   uint32
	actionParam4   uint32
	actionParam5   uint32
	actionParam6   uint32
	targetType     uint32
	targetParam1   uint32
	targetParam2   uint32
	targetParam3   uint32
	targetParam4   uint32
	targetX        float32
	targetY        float32
	targetZ        float32
	targetO        float32
}

// smartScriptEvent is a row plus its runtime state (SmartScriptHolder:
// timer, active, runOnce; SmartScript.cpp InitTimer/RecalcTimer).
// enableTimed is the timed-action-list chaining flag (SmartScriptMgr.h:1573):
// only the enabled list event ticks, and firing it enables the next one.
// timedEventID is the C++ event_id of a stored timed event (SmartScript.cpp:
// CREATE_TIMED_EVENT stores the synthetic holder with event_id = the timed
// event's id; RemoveStoredEvent matches on it); 0 on ordinary rows.
type smartScriptEvent struct {
	row          *smartScriptRow
	timer        uint32
	active       bool
	runOnce      bool
	enableTimed  bool
	timedEventID uint32
}

// smartScriptState is the per-creature SmartScript (SmartScript.h): the
// event list, current phase number (default 0 — SmartScript.cpp:51, the
// ctor sets mEventPhase = 0, so a nonzero phase mask matches nothing until
// a SET_EVENT_PHASE arm runs), last invoker, the TALK duration timer that
// fires SMART_EVENT_TEXT_OVER, and the stored-target lists
// (SmartScript::_storedTargets, SmartScript.cpp:108-121). timedList is the
// installed SMART_SCRIPT_TYPE_TIMED_ACTIONLIST (SmartScript.h:109): its
// events run one at a time, chained by enableTimed, with timedListInvoker
// as their invoker; processingTimedList is the isProcessingTimedActionList
// guard (SmartScript.cpp:3952-3958) that refuses a list overwrite from
// inside a list event's own action.
type smartScriptState struct {
	events         []*smartScriptEvent
	phase          uint32
	nested         int
	linkDepth      int
	lastInvoker    uint64
	defaultFaction uint32
	textTimer      uint32
	useTextTimer   bool
	lastTextID     uint32
	talkerEntry    uint32
	lastTick       time.Time
	firedInit      bool
	storedTargets  map[uint32][]uint64
	// SmartAI::_gossipReturn (SmartAI.h:331): set by
	// SMART_ACTION_SEND_GOSSIP_MENU (98) while a GOSSIP_HELLO event
	// runs; SmartAI::OnGossipHello resets it first (SmartAI.cpp:760)
	// and returns it, so a set value vetoes the default gossip menu.
	gossipReturn         bool
	timedList            []*smartScriptEvent
	timedListInvoker     uint64
	processingTimedList  bool
	invincibilityHPLevel uint32
	storedEvents         []*smartScriptEvent // mStoredEvents (SmartScript.h:122): CREATE_TIMED_EVENT holders, ticked in smartUpdateTimers
	remTimedEventIDs     []uint32            // mRemIDs (SmartScript.h:123): stored-event ids queued for removal, drained after the timed-list leg
	// mCounterList (SmartScript.h:94) + mTemplate (SmartScript.h:96):
	// SET_COUNTER (63) stores per-id counters on the target's script
	// and fires SMART_EVENT_COUNTER_SET (77); INSTALL_AI_TEMPLATE
	// (58) sets the template once (BASIC 0 = unset) and installs its
	// synthetic events.
	counters   map[uint32]uint32
	templateID uint32
	// SmartScript::meOrigGUID / goOrigGUID (SmartScript.h:113-115):
	// OVERRIDE_SCRIPT_BASE_OBJECT (76) stashes the owner's GUID in
	// baseOrigGUID on first override and rebinds the script's base
	// object to the first creature target (baseOverrideGUID);
	// RESET_SCRIPT_BASE_OBJECT (77) and OnReset restore the owner by
	// clearing both. Go has no GO script state, so only the creature
	// leg is modeled (the GO leg is a documented residual in the 76
	// arm); the override resolves in smartProcessAction, so every
	// later action reads the overridden creature as `me`.
	baseOverrideGUID uint64
	baseOrigGUID     uint64
	// SmartAI follow state (SmartAI.h: _followGUID/_followDistance/
	// _followAngle/_followCredit/_followArrivedEntry/_followCreditType,
	// _followArrivedTimer): armed by SMART_ACTION_FOLLOW, ticked by
	// smartUpdateFollow (SmartAI::UpdateFollow), cleared by
	// smartStopFollow. The motion's ScriptFollow* slot is the
	// MoveFollow generator it drives.
	followGUID         uint64
	followDist         float32
	followAngle        float32
	followCredit       uint32
	followArrivedEntry uint32
	followCreditType   uint32
	followArrivedTimer uint32
	// SmartAI escort state (SmartAI.h:250-278): _escortState / _path /
	// _currentWaypointNode / _waypointPauseTimer / _waypointPauseForced /
	// _repeatWaypointPath / _waypointPathEnded / _despawnState /
	// _escortQuestId / _escortNPCFlags / _run /
	// _escortInvokerCheckTimer, armed by SMART_ACTION_WP_START (53)
	// and ticked by smartUpdatePath / smartUpdateDespawn
	// (SmartAI::UpdatePath / UpdateDespawn). escortPath is the loaded
	// `waypoints`-table path (SmartWaypointMgr twin); escortPathID is
	// the script's path id (GetScript()->GetPathId()).
	// savedPathID / savedPoints / savedNextIdx / savedOneShot bank the
	// creature's pre-escort patrol slot: C++ keeps the escort path in
	// SmartAI::_path apart from the creature's default waypoint path,
	// while Go has one motion path slot, so the patrol id is parked
	// here and re-seated by EndPath; the creature idles after the
	// escort, like C++'s MoveIdle, instead of resuming the patrol at
	// once (the evade/respawn legs reload it from the path id).
	escortState             uint32
	escortPathID            uint32
	escortPath              []waypointPoint
	escortNode              uint32
	escortRepeat            bool
	escortRun               bool
	escortPauseTimer        uint32
	escortPauseForced       bool
	escortWaypointReached   bool
	escortPathEnded         bool
	escortOOCReached        bool   // _OOCReached (SmartAI.h): latched by smartMovementInform on a POINT + LAST_OOC_POINT arrival while escorting; the UpdatePath RETURNING consumer is ported in smartUpdatePath
	escortDespawnTime       uint32 // ms (SmartAI.h _despawnTime)
	escortDespawnState      uint32
	escortQuestID           uint32
	escortNPCFlags          uint32
	escortInvokerCheckTimer uint32
	savedPathID             uint32
	savedPoints             []waypointPoint
	savedNextIdx            int
	savedOneShot            bool
}

// loadSmartScripts loads the creature-source smart_scripts rows into the
// server store (SmartScriptMgr::LoadSmartScripts), plus the source_type-9
// timed-action-list rows into their own map (SmartAIMgr::GetScript,
// SmartScriptMgr.cpp:380-391: lists are keyed by entryOrGuid alone).
// The repo placeholder world.db carries zero rows, so this is a no-op
// there; a populated world DB fills the maps. GO/instance/area-trigger
// sources are out of scope. The
// conditions table is loaded alongside as a skip-set: TDB 26091 carries
// 1,205 SourceType-22 (smart event) condition rows, 69 of them on AGGRO
// events (18 distinct events), and Go has no conditions engine, so an event
// with any condition row never fires rather than firing unconditionally —
// the conservative twin of IsObjectMeetingSmartEventConditions
// (SmartScript.cpp:240, ProcessEventsFor). A missing conditions table
// leaves the set empty, which is exact: no table means no conditions.
func (s *Server) loadSmartScripts(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	s.smartScriptsMu.Lock()
	if s.smartScriptsLoaded {
		s.smartScriptsMu.Unlock()
		return
	}
	s.smartScriptsLoaded = true
	s.smartScriptsMu.Unlock()
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT entryorguid, source_type, id, link, event_type, event_phase_mask, event_chance, event_flags, event_param1, event_param2, event_param3, event_param4, event_param5, action_type, action_param1, action_param2, action_param3, action_param4, action_param5, action_param6, target_type, target_param1, target_param2, target_param3, target_param4, target_x, target_y, target_z, target_o FROM smart_scripts ORDER BY entryorguid, source_type, id, link`)
	if err != nil {
		return
	}
	defer rows.Close()
	byEntry := make(map[int32][]*smartScriptRow)
	byList := make(map[int32][]*smartScriptRow)
	for rows.Next() {
		var r smartScriptRow
		var tx, ty, tz, to float64
		if err := rows.Scan(&r.entryOrGUID, &r.sourceType, &r.id, &r.link, &r.eventType, &r.eventPhaseMask, &r.eventChance, &r.eventFlags, &r.eventParam1, &r.eventParam2, &r.eventParam3, &r.eventParam4, &r.eventParam5, &r.actionType, &r.actionParam1, &r.actionParam2, &r.actionParam3, &r.actionParam4, &r.actionParam5, &r.actionParam6, &r.targetType, &r.targetParam1, &r.targetParam2, &r.targetParam3, &r.targetParam4, &tx, &ty, &tz, &to); err != nil {
			continue
		}
		if r.sourceType != 0 && r.sourceType != 9 {
			continue
		}
		// SmartScriptMgr::IsEventValid (SmartScriptMgr.cpp:682-686):
		// an action type of 0 or >= SMART_ACTION_END (140) is
		// load-rejected — the row never reaches dispatch, and links
		// into it never fire. TDB 26091: 810 such rows (142:11 /
		// 144:306 / 145:296 / 146:70 / 147:127), 735 of them src 0/9;
		// 528 of those fire on their own events and 208 src-0 link
		// chains fire an invalid LINK row, where C++ fires nothing
		// (105 of the 208 are LINK-to-LINK chains).
		// SmartScriptMgr::IsEventValid (SmartScriptMgr.cpp:668-674):
		// an event type >= SMART_EVENT_END (83) is load-rejected the
		// same way — the row never reaches dispatch, and links into it
		// never fire. TDB 26091: 12 such rows (83:5 / 86:1 / 89:1 /
		// 90:5), all src 0. The type-mask and phase-mask checks below
		// it stay out (Go loads creature + timed-list sources only).
		if r.eventType >= smartEventEnd {
			continue
		}
		if r.actionType <= 0 || r.actionType >= smartActionEnd {
			continue
		}
		r.targetX, r.targetY, r.targetZ, r.targetO = float32(tx), float32(ty), float32(tz), float32(to)
		row := r
		if row.sourceType == 9 {
			byList[row.entryOrGUID] = append(byList[row.entryOrGUID], &row)
			continue
		}
		byEntry[row.entryOrGUID] = append(byEntry[row.entryOrGUID], &row)
	}
	s.smartScriptsMu.Lock()
	s.smartScripts = byEntry
	s.smartTimedActionLists = byList
	s.smartScriptsMu.Unlock()
	s.loadSmartConditions(ctx)
}

// loadSmartConditions fills the smart-event condition skip-set (see
// loadSmartScripts). A query error (missing table) leaves it empty.
func (s *Server) loadSmartConditions(ctx context.Context) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT SourceEntry, SourceId FROM conditions WHERE SourceType = 22`)
	if err != nil {
		return
	}
	defer rows.Close()
	conds := make(map[smartConditionKey]struct{})
	for rows.Next() {
		var entry int32
		var id uint32
		if err := rows.Scan(&entry, &id); err != nil {
			continue
		}
		conds[smartConditionKey{entry: entry, id: id}] = struct{}{}
	}
	s.smartScriptsMu.Lock()
	s.smartConditions = conds
	s.smartScriptsMu.Unlock()
}

// reloadSmartScripts is the `.reload smart_scripts` twin of C++
// HandleReloadSmartScripts (cs_reload.cpp:1156-1163), which re-runs
// SmartAIMgr::LoadSmartAIFromDB: the load-once guard drops so the
// loader re-reads the table (and the conditions skip-set with it).
// Attached motions keep their loaded events, matching C++ — live
// SmartScripts hold their own mEvents copies (SmartScript.h:107), so
// only creatures spawned or attached after the reload see new rows.
// A failed reload keeps the previous rows (the loader returns early
// on query error); C++ clears first and ends up empty instead.
func (s *Server) reloadSmartScripts(ctx context.Context) {
	if s == nil {
		return
	}
	s.smartScriptsMu.Lock()
	s.smartScriptsLoaded = false
	s.smartScriptsMu.Unlock()
	s.loadSmartScripts(ctx)
}

// reloadSmartConditions re-reads the smart-event condition skip-set:
// the `.reload conditions` leg for SmartAI. C++ reloads the whole
// conditions store (sConditionMgr->LoadConditions) and SmartScript
// checks it live per event; Go snapshots the SourceType-22 rows, so
// the snapshot refreshes here.
func (s *Server) reloadSmartConditions(ctx context.Context) {
	s.loadSmartConditions(ctx)
}

// smartConditionKey is one conditions row for a smart event: SourceType 22,
// SourceEntry = entryOrGuid, SourceId = event id (ConditionMgr.h).
type smartConditionKey struct {
	entry int32
	id    uint32
}

func (s *Server) smartHasConditions(entry int32, id uint32) bool {
	if s == nil {
		return false
	}
	s.smartScriptsMu.RLock()
	defer s.smartScriptsMu.RUnlock()
	_, ok := s.smartConditions[smartConditionKey{entry: entry, id: id}]
	return ok
}

func (s *Server) smartRowsFor(entry int32) []*smartScriptRow {
	if s == nil {
		return nil
	}
	s.smartScriptsMu.RLock()
	defer s.smartScriptsMu.RUnlock()
	return s.smartScripts[entry]
}

// smartTimedActionListRows is SmartAIMgr::GetScript (SmartScriptMgr.cpp:
// 380-391) for SMART_SCRIPT_TYPE_TIMED_ACTIONLIST: the list rows by id,
// or nil when the list does not exist (C++ logs at debug and returns an
// empty list; the install then clears and stops).
func (s *Server) smartTimedActionListRows(listID int32) []*smartScriptRow {
	if s == nil {
		return nil
	}
	s.smartScriptsMu.RLock()
	defer s.smartScriptsMu.RUnlock()
	return s.smartTimedActionLists[listID]
}

// smartStateFor returns the creature's SmartScript state, attaching it on
// first use when the creature is SmartAI and has script rows (SmartAI ctor
// + SmartScript::OnInitialize). Init events fire here, in C++ order
// (AI_INIT, then JUST_CREATED), because attach runs on the unlocked tick
// path; motionForLocked cannot fire them (it runs under motionMu).
func (s *Server) smartStateFor(ctx context.Context, motion *creatureMotion) *smartScriptState {
	if s == nil || motion == nil || motion.AIName != "SmartAI" {
		return nil
	}
	if motion.smart != nil {
		return motion.smart
	}
	rows := s.smartRowsFor(int32(motion.Entry))
	// Spawn-GUID-specific rows (SmartScriptMgr convention: a negative
	// entryorguid is -spawnGUID, TDB 26091: 59 AGGRO rows over 56 spawns)
	// ride after the entry rows, like C++'s GetScript merge.
	if guidRows := s.smartRowsFor(-int32(uint32(motion.GUID & 0x00FFFFFF))); len(guidRows) > 0 {
		rows = append(append([]*smartScriptRow{}, rows...), guidRows...)
	}
	if len(rows) == 0 {
		return nil
	}
	st := &smartScriptState{phase: 0, defaultFaction: motion.Faction}
	for _, row := range rows {
		ev := &smartScriptEvent{row: row}
		s.smartInitTimer(ev)
		st.events = append(st.events, ev)
	}
	motion.smart = st
	st.firedInit = true
	s.smartProcessEvents(ctx, motion, smartEventAIInit, 0, 0, 0)
	s.smartProcessEvents(ctx, motion, smartEventJustCreated, 0, 0, 0)
	return st
}

// smartInitTimer is SmartScript::InitTimer (:3472-3486): timed-update events
// start on their initial min/max roll, everything else starts active.
func (s *Server) smartInitTimer(ev *smartScriptEvent) {
	switch ev.row.eventType {
	case smartEventUpdate, smartEventUpdateIC, smartEventUpdateOOC:
		s.smartRecalcTimer(ev, ev.row.eventParam1, ev.row.eventParam2)
	default:
		ev.active = true
	}
}

// smartRecalcTimer is SmartScript::RecalcTimer (:3491-3496): timer =
// urand(min, max); active = (timer == 0).
func (s *Server) smartRecalcTimer(ev *smartScriptEvent, min, max uint32) {
	if max < min {
		max = min
	}
	if max == min {
		ev.timer = min
	} else {
		ev.timer = min + uint32(rand.Int63n(int64(max-min+1)))
	}
	ev.active = ev.timer == 0
}

// smartReset is SmartScript::OnReset (SmartScript.cpp:170-189): the
// base object restores first (ResetBaseObject, :191-220), then every
// event without the DONT_RESET flag re-inits its timer and clears
// runOnce, then the SMART_EVENT_RESET events fire, then the last
// invoker clears. The priority reset has no Go twin (no per-event
// priority field). Call sites are the C++ OnReset callers in
// SmartAI.cpp: EnterEvadeMode (:453, only when the EVADE state
// cleared — the owner-follow legs), JustAppeared (:539, after the
// RESPAWN event) and JustReachedHome (:544, before the REACHED_HOME
// event); CALL_SCRIPT_RESET (78) also rides it.
func (s *Server) smartReset(ctx context.Context, motion *creatureMotion) {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	// ResetBaseObject (SmartScript.cpp:191-220): the stashed original
	// GUIDs restore `me`/`go`; the owner motion is always the dispatch
	// parameter in Go, so restoring is clearing both stash fields —
	// the next action resolves the owner again.
	st.baseOverrideGUID = 0
	st.baseOrigGUID = 0
	for _, ev := range st.events {
		if ev.row.eventFlags&smartEventFlagDontReset == 0 {
			s.smartInitTimer(ev)
			ev.runOnce = false
		}
	}
	st.lastInvoker = 0
	s.smartProcessEvents(ctx, motion, smartEventReset, 0, 0, 0)
}

// smartJustReachedHome is SmartAI::JustReachedHome (SmartAI.cpp:542-545):
// OnReset, then the SMART_EVENT_REACHED_HOME events fire.
func (s *Server) smartJustReachedHome(ctx context.Context, motion *creatureMotion) {
	s.smartReset(ctx, motion)
	s.smartProcessEvents(ctx, motion, smartEventReachedHome, 0, 0, 0)
}

func (st *smartScriptState) inPhase(mask uint32) bool {
	if mask == 0 {
		return true
	}
	if st.phase == 0 {
		return false
	}
	return (uint32(1)<<(st.phase-1))&mask != 0
}

// smartProcessEvents is SmartScript::ProcessEventsFor (:221-250): every
// non-LINK event of the given type runs, nested calls capped at 10.
// Conditioned events never fire from here (the skip-set in loadSmartScripts);
// C++ checks conditions at this same point (:240), not inside ProcessEvent,
// so LINK-chained events keep firing condition-free below.
func (s *Server) smartProcessEvents(ctx context.Context, motion *creatureMotion, eventType uint32, invokerGUID uint64, var0, var1 uint32) {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	st.nested++
	if st.nested > smartMaxNestedEvents {
		st.nested--
		return
	}
	st.linkDepth = 0
	for _, ev := range st.events {
		if ev.row.eventType == smartEventLink {
			continue
		}
		if ev.row.eventType == eventType {
			if s.smartHasConditions(ev.row.entryOrGUID, ev.row.id) {
				continue
			}
			s.smartProcessEvent(ctx, motion, st, ev, invokerGUID, var0, var1)
		}
	}
	st.nested--
}

// smartSpawnMode is the C++ GetSpawnMode value FillScript gates difficulty
// flags on (SmartScript.cpp:3797-3808): 0 normal, 1 heroic (5-man), 0-3
// 10N/25N/10H/25H (raid). Go has no per-instance spawn mode, so the
// engaging player's difficulty stands in (playerSpawnMode, commands_wp.go:
// raid maps read RaidDifficulty, dungeon maps DungeonDifficulty, world
// maps 0) — identical in practice for dungeon/raid engages; a
// creature-invoked dispatch reads 0.
func (s *Server) smartSpawnMode(invokerGUID uint64) uint32 {
	if sess := s.findSessionByGUID(invokerGUID); sess != nil {
		return sess.playerSpawnMode()
	}
	return 0
}

// smartProcessEvent is SmartScript::ProcessEvent (:2882-...): the active,
// phase-mask, NOT_REPEATABLE, WHILE_CHARMED and difficulty gates, then the
// per-type dispatch — direct events run their action, timed events run
// ProcessTimedAction.
func (s *Server) smartProcessEvent(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64, var0, var1 uint32) {
	if !ev.active && ev.row.eventType != smartEventLink {
		return
	}
	if ev.row.eventPhaseMask != 0 && !st.inPhase(ev.row.eventPhaseMask) {
		return
	}
	if ev.row.eventFlags&smartEventFlagNotRepeatable != 0 && ev.runOnce {
		return
	}
	// WHILE_CHARMED gate (SmartScript.cpp:2890): the event fires while
	// charmed only when the flag is set. Go tracks charm as CharmerGUID
	// (the CallForHelp gate reads the same field).
	if ev.row.eventFlags&smartEventFlagWhileCharmed == 0 && motion.CharmerGUID != 0 {
		return
	}
	// Difficulty gate (SmartScript::FillScript, SmartScript.cpp:3797-3808):
	// a flagged event loads only on a dungeon map whose spawn mode
	// matches (1 << (spawnMode+1)) & flags; on a world map it never loads.
	// TDB 26091: 182 AGGRO rows carry difficulty flags.
	if ev.row.eventFlags&smartEventFlagDifficultyAll != 0 {
		if !dungeonMapID(s, motion.Map) && !raidMapID(s, motion.Map) {
			return
		}
		if (uint32(1)<<(s.smartSpawnMode(invokerGUID)+1))&ev.row.eventFlags == 0 {
			return
		}
	}
	switch ev.row.eventType {
	case smartEventLink:
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventDataSet:
		// SMART_EVENT_DATA_SET (SmartScript.cpp:3286-3292): the event
		// fires only for its (field, value) pair, then re-arms its
		// cooldown (RecalcTimer); the tick re-activates it on expiry.
		if ev.row.eventParam1 != var0 || ev.row.eventParam2 != var1 {
			return
		}
		s.smartRecalcTimer(ev, ev.row.eventParam3, ev.row.eventParam4)
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventCounterSet:
		// SMART_EVENT_COUNTER_SET (SmartScript.cpp:3460-3464): the
		// event fires only for its (id, value) pair, then runs the
		// timed action on the cooldown window (event_param3/4).
		if ev.row.eventParam1 != var0 || s.smartCounterValue(st, ev.row.eventParam1) != ev.row.eventParam2 {
			return
		}
		s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, invokerGUID, var0, var1)
	case smartEventUpdate, smartEventUpdateOOC, smartEventUpdateIC:
		s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, invokerGUID, var0, var1)
	case smartEventHealthPct, smartEventTargetHealthPc:
		if motion == nil || !motion.InCombat || motion.MaxHealth == 0 {
			return
		}
		health := motion.Health
		victimGUID := motion.TargetGUID
		if ev.row.eventType == smartEventTargetHealthPc {
			victim := s.findCreatureMotion(motion.Map, motion.InstanceID, victimGUID)
			if victim == nil {
				if ps := s.findSessionByGUID(victimGUID); ps != nil && ps.player != nil {
					health = ps.player.Health
					if ps.player.MaxHealth == 0 || uint32(100*uint64(health)/uint64(ps.player.MaxHealth)) > ev.row.eventParam2 || uint32(100*uint64(health)/uint64(ps.player.MaxHealth)) < ev.row.eventParam1 {
						return
					}
					s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, victimGUID, var0, var1)
					return
				}
				return
			}
			if victim.MaxHealth == 0 {
				return
			}
			health = victim.Health
			if uint32(100*uint64(health)/uint64(victim.MaxHealth)) > ev.row.eventParam2 || uint32(100*uint64(health)/uint64(victim.MaxHealth)) < ev.row.eventParam1 {
				return
			}
			s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, victimGUID, var0, var1)
			return
		}
		if uint32(100*uint64(health)/uint64(motion.MaxHealth)) > ev.row.eventParam2 || uint32(100*uint64(health)/uint64(motion.MaxHealth)) < ev.row.eventParam1 {
			return
		}
		s.smartProcessTimedAction(ctx, motion, st, ev, ev.row.eventParam3, ev.row.eventParam4, invokerGUID, var0, var1)
	case smartEventManaPct, smartEventTargetManaPct:
		// Mana pct needs the live power value, which the motion record does
		// not carry for creatures (powers live in the aura pipeline);
		// residual — the timer still re-arms below via the default arm.
		s.smartRecalcTimer(ev, ev.row.eventParam3, ev.row.eventParam4)
		return
	case smartEventRange:
		if motion == nil || !motion.InCombat || motion.TargetGUID == 0 {
			return
		}
		s.smartRecalcTimer(ev, 500, 500)
		return
	case smartEventTimedEventTriggered:
		// SMART_EVENT_TIMED_EVENT_TRIGGERED (SmartScript.cpp:3303-3307):
		// the event fires only for its timed-event id, then runs its
		// action with no timer re-arm (a stored event's own repeat
		// window re-arms the stored holder, not this row).
		if ev.row.eventParam1 != var0 {
			return
		}
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventMovementInform:
		// SMART_EVENT_MOVEMENTINFORM (SmartScript.cpp:3214-3219): the
		// event fires only for its (movement type, point id) pair,
		// 0 = any.
		if ev.row.eventParam1 != 0 && ev.row.eventParam1 != var0 {
			return
		}
		if ev.row.eventParam2 != 0 && ev.row.eventParam2 != var1 {
			return
		}
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventWaypointStart, smartEventWaypointReached,
		smartEventWaypointResumed, smartEventWaypointPaused,
		smartEventWaypointStopped, smartEventWaypointEnded:
		// SMART_EVENT_WAYPOINT_* (SmartScript.cpp:3228-3238): the event
		// fires only for its (pointID, pathID) pair, 0 = any.
		if ev.row.eventParam1 != 0 && ev.row.eventParam1 != var0 {
			return
		}
		if ev.row.eventParam2 != 0 && ev.row.eventParam2 != var1 {
			return
		}
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventGossipSelect:
		// SMART_EVENT_GOSSIP_SELECT (SmartScript.cpp:3309-3316): the
		// event fires only for its exact (sender, action) pair — the
		// packet menuID (var0) and gossip list id (var1); unlike
		// MOVEMENTINFORM/WAYPOINT there is no 0=any wildcard.
		if ev.row.eventParam1 != var0 || ev.row.eventParam2 != var1 {
			return
		}
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventGossipHello:
		// SMART_EVENT_GOSSIP_HELLO (SmartScript.cpp:3071-3095): the
		// event_param1 filter gates on var0 (noReportUse): 0 always
		// runs, 1 runs only on a plain hello (var0 == 0), 2 only on
		// a report-use (var0 != 0); any other value runs. The
		// creature hello passes var0 = 0 (SmartAI::OnGossipHello).
		// TDB 26091: all 14 SEND_GOSSIP_MENU rows carry filter 0.
		switch ev.row.eventParam1 {
		case 1:
			if var0 != 0 {
				return
			}
		case 2:
			if var0 == 0 {
				return
			}
		}
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	case smartEventPassengerBoarded, smartEventPassengerRemoved:
		// SMART_EVENT_PASSENGER_BOARDED/REMOVED
		// (SmartScript.cpp:3294-3300): the event needs its passenger
		// (unit), re-arms on the cooldown window (event_param1/2 —
		// the minMax pair, SmartScriptMgr.h:235-238), then runs its
		// action with NO var0/var1 — the seat id ProcessEventsFor
		// carries is dropped by the case's ProcessAction(e, unit)
		// call. TDB 26091: all 34 + 24 rows carry 0/0 cooldowns.
		if invokerGUID == 0 {
			return
		}
		s.smartRecalcTimer(ev, ev.row.eventParam1, ev.row.eventParam2)
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, 0, 0)
	case smartEventSummonedUnit, smartEventSummonedUnitDies:
		// SMART_EVENT_SUMMONED_UNIT / SUMMONED_UNIT_DIES
		// (SmartScript.cpp:3193-3201): the event needs its summoned
		// creature (unit), fires only when its entry matches
		// event_param1 (0 = any), then re-arms on the cooldown window
		// (event_param2/3) and runs its action with the summoned
		// creature as invoker. TDB 26091: 17 = 13 rows (6 entry-0, 7
		// entry-specific, all cooldowns 0/0), 82 = 7 rows (4 entry-0,
		// 3 entry-specific, all cooldowns 0/0).
		if invokerGUID == 0 {
			return
		}
		if ev.row.eventParam1 != 0 {
			summoned := s.findCreatureMotion(motion.Map, motion.InstanceID, invokerGUID)
			if summoned == nil || summoned.Entry != ev.row.eventParam1 {
				return
			}
		}
		s.smartRecalcTimer(ev, ev.row.eventParam2, ev.row.eventParam3)
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, 0, 0)
	case smartEventAggro, smartEventKill, smartEventDeath, smartEventEvade,
		smartEventRespawn, smartEventReachedHome, smartEventReset,
		smartEventAIInit, smartEventJustCreated,
		smartEventTextOver, smartEventPhaseChange,
		smartEventFollowCompleted,
		smartEventJustSummoned, smartEventOnSpellClick:
		s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
	default:
		// Timed/conditioned types with no Go condition evaluation
		// (SPELLHIT, DAMAGED, FRIENDLY_*, HAS_AURA, ...) never fire from
		// the direct path; their timer arm re-arms in smartUpdateTimers.
		return
	}
}

// smartProcessTimedAction is SmartScript::ProcessTimedAction (:2385-2395):
// the condition check runs first (a failing event re-arms on a short
// window instead of firing); on pass, re-arm on the repeat window, then
// run the action.
func (s *Server) smartProcessTimedAction(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, min, max uint32, invokerGUID uint64, var0, var1 uint32) {
	if s.smartHasConditions(ev.row.entryOrGUID, ev.row.id) {
		if min > 5000 {
			min = 5000
		}
		s.smartRecalcTimer(ev, min, min)
		return
	}
	s.smartRecalcTimer(ev, min, max)
	s.smartProcessAction(ctx, motion, st, ev, invokerGUID, var0, var1)
}

// smartUpdateTimers is SmartScript::OnUpdate (:3676-...) + UpdateTimer
// (:3499-3597) for the creature slice: dead/evading creatures run no
// timers, UPDATE_IC needs combat, UPDATE_OOC needs out-of-combat, and only
// the timed event types fire from here.
func (s *Server) smartUpdateTimers(ctx context.Context, motion *creatureMotion, now time.Time) {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	if motion.Evading {
		// SmartScript::OnUpdate's evade branch (SmartScript.cpp:3685-3696):
		// a finished list (no enabled event left) clears so a later
		// CALL_TIMED_ACTIONLIST can install; a running list survives the
		// evade and resumes on return.
		if len(st.timedList) > 0 {
			running := false
			for _, ev := range st.timedList {
				if ev.enableTimed {
					running = true
					break
				}
			}
			if !running {
				st.timedList = nil
			}
		}
		return
	}
	diff := uint32(0)
	if !st.lastTick.IsZero() {
		d := now.Sub(st.lastTick)
		if d > 0 {
			diff = uint32(d.Milliseconds())
			if diff > 60000 {
				diff = 60000
			}
		}
	}
	st.lastTick = now
	if st.useTextTimer {
		if st.textTimer < diff {
			textID, entry := st.lastTextID, st.talkerEntry
			st.lastTextID, st.talkerEntry, st.textTimer, st.useTextTimer = 0, 0, 0, false
			s.smartProcessEvents(ctx, motion, smartEventTextOver, 0, textID, entry)
		} else {
			st.textTimer -= diff
		}
	}
	for _, ev := range st.events {
		if ev.row.eventType == smartEventLink {
			continue
		}
		if ev.row.eventPhaseMask != 0 && !st.inPhase(ev.row.eventPhaseMask) {
			continue
		}
		if ev.row.eventType == smartEventUpdateIC && !motion.InCombat {
			continue
		}
		if ev.row.eventType == smartEventUpdateOOC && motion.InCombat {
			continue
		}
		if ev.timer < diff {
			ev.active = true
			switch ev.row.eventType {
			case smartEventUpdate, smartEventUpdateOOC, smartEventUpdateIC,
				smartEventHealthPct, smartEventTargetHealthPc, smartEventManaPct,
				smartEventTargetManaPct, smartEventRange:
				s.smartProcessEvent(ctx, motion, st, ev, 0, 0, 0)
			}
		} else {
			ev.timer -= diff
		}
	}
	s.smartUpdateStoredEvents(ctx, motion, st, diff)
	s.smartUpdateTimedActionList(ctx, motion, st, diff)
	// mRemIDs drain (SmartScript.cpp:3741-3746): after the stored-event
	// and timed-list legs, each queued id drops its stored event (first
	// match only, RemoveStoredEvent :3615-3623), then the queue clears.
	if len(st.remTimedEventIDs) > 0 {
		for _, id := range st.remTimedEventIDs {
			s.smartRemoveStoredEvent(st, id)
		}
		st.remTimedEventIDs = nil
	}
	// SmartAI::UpdateAI (SmartAI.cpp:300) runs UpdateFollow after
	// OnUpdate: the arrival check ticks here, on the same diff.
	// UpdatePath runs ahead of it and UpdateDespawn after it, the
	// C++ UpdateAI order (SmartAI.cpp:284-303).
	s.smartUpdatePath(ctx, motion, st, diff)
	s.smartUpdateFollow(ctx, motion, st, diff)
	s.smartUpdateDespawn(ctx, motion, st, diff)
}

// smartCreateTimedEvent is the SMART_ACTION_CREATE_TIMED_EVENT arm
// (SmartScript.cpp:1586-1613): the synthetic holder is an UPDATE event
// with the creator's chance (0 defaults to 100), the initial window in
// event_param1/2 and the repeat window in param3/4, NOT_REPEATABLE when
// neither repeat bound is set, the creator's target, and the
// TRIGGER_TIMED_EVENT action for the id; InitTimer rolls the initial
// window and the holder joins mStoredEvents. The row id is the timed
// event's id (C++ ev.event_id), so the ProcessTimedAction conditions
// check and RemoveStoredEvent both key on it, exactly as in C++.
func (s *Server) smartCreateTimedEvent(st *smartScriptState, ev *smartScriptEvent) {
	if st == nil || ev == nil || ev.row == nil {
		return
	}
	chance := ev.row.actionParam6
	if chance == 0 {
		chance = 100
	}
	flags := uint32(0)
	if ev.row.actionParam4 == 0 && ev.row.actionParam5 == 0 {
		flags = smartEventFlagNotRepeatable
	}
	row := &smartScriptRow{
		// entryOrGUID 0: C++ builds the holder from a default
		// SmartScriptHolder (SmartScriptMgr.h:1551, entryOrGuid(0)),
		// so the holder's condition check keys on (0, id) and never
		// inherits the creator row's conditions.
		entryOrGUID:  0,
		sourceType:   ev.row.sourceType,
		id:           ev.row.actionParam1,
		eventType:    smartEventUpdate,
		eventChance:  chance,
		eventFlags:   flags,
		eventParam1:  ev.row.actionParam2,
		eventParam2:  ev.row.actionParam3,
		eventParam3:  ev.row.actionParam4,
		eventParam4:  ev.row.actionParam5,
		actionType:   smartActionTriggerTimedEvent,
		actionParam1: ev.row.actionParam1,
		targetType:   ev.row.targetType,
		targetParam1: ev.row.targetParam1,
		targetParam2: ev.row.targetParam2,
		targetParam3: ev.row.targetParam3,
		targetParam4: ev.row.targetParam4,
		targetX:      ev.row.targetX,
		targetY:      ev.row.targetY,
		targetZ:      ev.row.targetZ,
		targetO:      ev.row.targetO,
	}
	sev := &smartScriptEvent{row: row, timedEventID: ev.row.actionParam1}
	s.smartInitTimer(sev)
	st.storedEvents = append(st.storedEvents, sev)
}

// smartRemoveStoredEvent is SmartScript::RemoveStoredEvent
// (SmartScript.cpp:3615-3623): erase the first stored event whose
// event_id matches, then stop.
func (s *Server) smartRemoveStoredEvent(st *smartScriptState, id uint32) {
	if st == nil {
		return
	}
	for i, sev := range st.storedEvents {
		if sev.timedEventID == id {
			st.storedEvents = append(st.storedEvents[:i], st.storedEvents[i+1:]...)
			return
		}
	}
}

// smartUpdateStoredEvents is the OnUpdate stored-events leg
// (SmartScript.cpp:3710-3717): each stored holder ticks like a main-loop
// UPDATE event — phase gate, timer expiry sets active and runs
// ProcessEvent (whose UPDATE arm re-arms on the repeat window via
// ProcessTimedAction, then fires the TRIGGER_TIMED_EVENT action).
// Stored events never tick while evading: the caller returns before this
// point on the evade branch, matching OnUpdate's early return.
func (s *Server) smartUpdateStoredEvents(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if st == nil || len(st.storedEvents) == 0 {
		return
	}
	for _, sev := range st.storedEvents {
		if sev.row.eventPhaseMask != 0 && !st.inPhase(sev.row.eventPhaseMask) {
			continue
		}
		if sev.timer < diff {
			sev.active = true
			s.smartProcessEvent(ctx, motion, st, sev, 0, 0, 0)
		} else {
			sev.timer -= diff
		}
	}
}

// smartUpdateTimedActionList is SmartScript::OnUpdate's timed-action-list
// leg (SmartScript.cpp:3713-3739) plus the UpdateTimer timed branch
// (:3541-3570): only enabled list events tick; the IC/OOC gates hold the
// timer while the combat state mismatches (the same gate as the main
// loop, mirroring UpdateTimer's early return); on expiry the event fires
// with the list invoker, then disables itself and enables the next
// higher-id event — the advance is unconditional, matching C++ (a gated
// action still moves the list on). When no event is enabled the list is
// finished and clears. The UpdateTimer CAST-delay and root arms have no
// Go twin in the main loop either and stay out here too.
func (s *Server) smartUpdateTimedActionList(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if len(st.timedList) == 0 {
		return
	}
	st.processingTimedList = true
	needCleanup := true
	for _, ev := range st.timedList {
		if !ev.enableTimed {
			continue
		}
		needCleanup = false
		// UpdateTimer's phase gate (:3502-3503) sits ahead of the timer:
		// a phase-mismatched list event holds its timer AND its place
		// in the list — unlike the gates inside ProcessEvent (runOnce,
		// WHILE_CHARMED, difficulty), which still advance the list.
		if ev.row.eventPhaseMask != 0 && !st.inPhase(ev.row.eventPhaseMask) {
			continue
		}
		if ev.row.eventType == smartEventUpdateIC && !motion.InCombat {
			continue
		}
		if ev.row.eventType == smartEventUpdateOOC && motion.InCombat {
			continue
		}
		if ev.timer < diff {
			ev.active = true
			s.smartProcessEvent(ctx, motion, st, ev, st.timedListInvoker, 0, 0)
			ev.enableTimed = false
			for _, next := range st.timedList {
				if next.row.id > ev.row.id {
					next.enableTimed = true
					break
				}
			}
		} else {
			ev.timer -= diff
		}
	}
	st.processingTimedList = false
	if needCleanup {
		st.timedList = nil
	}
}

// smartStateForTimedList attaches the SmartScript state for a timed-list
// install on a creature with no creature-source rows: C++ installs the
// list on any SmartAI creature (SetTimedActionList needs only IsSmart),
// so a row-less SmartAI still runs its list. No init events fire —
// there are no rows to fire them from.
func (s *Server) smartStateForTimedList(ctx context.Context, motion *creatureMotion) *smartScriptState {
	if st := s.smartStateFor(ctx, motion); st != nil {
		return st
	}
	if s == nil || motion == nil || motion.AIName != "SmartAI" {
		return nil
	}
	st := &smartScriptState{phase: 0, defaultFaction: motion.Faction}
	motion.smart = st
	return st
}

// smartSetTimedActionList is SmartScript::SetTimedActionList
// (SmartScript.cpp:3950-3977): a running list blocks a new install
// unless allowOverride is set, and a list started from inside a list
// event's own action is refused (the isProcessingTimedActionList guard
// — overwriting mid-iteration would invalidate the holder the caller
// is running from). The install clears first, so an unknown id empties
// the running list and stops. Each row is copied before its event type
// is forced — the stored rows are shared — and the timer type picks the
// forced type: 0 OOC, 1 IC, anything else UPDATE. Only the first event
// is enabled; InitTimer rolls its initial window (event_param1/2), the
// repeat window (param3/4) re-arms on fire.
func (s *Server) smartSetTimedActionList(ctx context.Context, motion *creatureMotion, listID, timerType uint32, allowOverride bool, invokerGUID uint64) {
	st := s.smartStateForTimedList(ctx, motion)
	if st == nil {
		return
	}
	if st.processingTimedList {
		return
	}
	if !allowOverride && len(st.timedList) > 0 {
		return
	}
	rows := s.smartTimedActionListRows(int32(listID))
	st.timedList = nil
	if len(rows) == 0 {
		return
	}
	st.timedListInvoker = invokerGUID
	forced := smartEventUpdate
	switch timerType {
	case 0:
		forced = smartEventUpdateOOC
	case 1:
		forced = smartEventUpdateIC
	}
	for i, row := range rows {
		rc := *row
		rc.eventType = forced
		ev := &smartScriptEvent{row: &rc, enableTimed: i == 0}
		s.smartInitTimer(ev)
		st.timedList = append(st.timedList, ev)
	}
}

// smartProcessAction is SmartScript::ProcessAction (:255-2379), AGGRO-used
// action set. Chance: a 0 chance always fires (SmartScript.cpp:256-260);
// runOnce marks the event for the NOT_REPEATABLE gate. The link step
// (:2375-2381) re-enters ProcessEvent for the linked event; C++ caps only
// ProcessEventsFor nesting (MAX_NESTED_EVENTS), not link chains, so a
// link cycle would recurse forever there — Go caps the chain at the same
// 10 as a hardening deviation (no such cycle exists in TDB 26091).
func (s *Server) smartProcessAction(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64, var0, var1 uint32) {
	if ev.row.eventType != smartEventLink && ev.row.eventChance < 100 && ev.row.eventChance != 0 {
		if uint32(rand.Intn(100)) >= ev.row.eventChance {
			return
		}
	}
	ev.runOnce = true
	if invokerGUID != 0 {
		st.lastInvoker = invokerGUID
	}
	// SMART_ACTION_OVERRIDE_SCRIPT_BASE_OBJECT (76) rebinds the
	// script's base object: once a row overrides it, every later
	// action in this script reads the overridden creature as `me`
	// (SmartScript.cpp:1625-1651), until a RESET_SCRIPT_BASE_OBJECT
	// (77) row or OnReset restores the owner. Resolve it here so the
	// whole dispatch below — targets and helpers alike — sees the
	// overridden base. A despawned override falls back to the owner
	// instead of dangling (C++ keeps the raw pointer, hence the "CAN
	// CRASH CORE" warning on the enum); event-level gates above this
	// point (phase, range) still read the owner.
	if st.baseOverrideGUID != 0 {
		if om := s.findCreatureMotion(motion.Map, motion.InstanceID, st.baseOverrideGUID); om != nil {
			motion = om
		}
	}
	switch ev.row.actionType {
	case smartActionTalk:
		s.smartActionTalk(ctx, motion, st, ev, invokerGUID)
	case smartActionSetFaction:
		faction := ev.row.actionParam1
		if faction == 0 {
			faction = st.defaultFaction
		}
		s.motionMu.Lock()
		motion.Faction = faction
		motion.FactionKnown = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldFaction: faction})
	case smartActionSound:
		s.smartActionSound(ctx, motion, st, ev, invokerGUID)
	case smartActionPlayEmote:
		// SMART_ACTION_PLAY_EMOTE (SmartScript.cpp:325-337): the emote
		// plays on every target unit, not just the owner — the self-only
		// broadcast was the slice-1 partial (TDB 26091: the 1 live row
		// targets SELF, so the old form was faithful for it). Per-target
		// coords feed the visible set, falling back to the owner's when
		// the target has no live motion or session.
		for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
			mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
			if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid); tm != nil {
				s.motionMu.Lock()
				mapID, instanceID, x, y, z, phase = tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask
				s.motionMu.Unlock()
			} else if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.player != nil {
				mapID, instanceID, x, y, z = sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z
				phase = sess.currentPlayerPhaseMask()
			}
			packet := protocol.NewBuffer(12)
			packet.WriteU32(ev.row.actionParam1)
			packet.WriteU64(tgt.guid)
			s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), mapID, instanceID, x, y, z, phase)
		}
	case smartActionRandomEmote:
		s.smartActionRandomEmote(ctx, motion, st, ev, invokerGUID)
	case smartActionFailQuest:
		s.smartActionFailQuest(ctx, motion, st, ev, invokerGUID)
	case smartActionOfferQuest:
		s.smartActionOfferQuest(ctx, motion, st, ev, invokerGUID)
	case smartActionCallAreaExploredOrEventHappens:
		s.smartActionCallAreaExploredOrEventHappens(ctx, motion, st, ev, invokerGUID)
	case smartActionCallGroupEventHappens:
		s.smartActionCallGroupEventHappens(ctx, motion, st, ev, invokerGUID)
	case smartActionCombatStop:
		s.smartActionCombatStop(ctx, motion, st, ev, invokerGUID)
	case smartActionFollow:
		s.smartActionFollow(ctx, motion, st, ev, invokerGUID)
	case smartActionActivateGObject:
		s.smartActionActivateGObject(ctx, motion, st, ev, invokerGUID)
	case smartActionSetReactState:
		s.smartActionSetReactState(ctx, motion, st, ev, invokerGUID)
	case smartActionSetEmoteState:
		s.smartActionSetEmoteState(ctx, motion, st, ev, invokerGUID)
	case smartActionSetVisibility:
		s.smartActionSetVisibility(ctx, motion, st, ev, invokerGUID)
	case smartActionInterruptSpell:
		s.smartActionInterruptSpell(ctx, motion, st, ev, invokerGUID)
	case smartActionSetRoot:
		s.smartActionSetRoot(ctx, motion, st, ev, invokerGUID)
	case smartActionCast:
		s.smartActionCast(ctx, motion, st, ev, invokerGUID)
	case smartActionSimpleTalk:
		s.smartActionSimpleTalk(ctx, motion, st, ev, invokerGUID)
	case smartActionSelfCast:
		s.smartActionSelfCast(ctx, motion, st, ev, invokerGUID)
	case smartActionCrossCast:
		s.smartActionCrossCast(ctx, motion, st, ev, invokerGUID)
	case smartActionSummonCreature:
		s.smartActionSummonCreature(ctx, motion, st, ev, invokerGUID)
	case smartActionThreatSinglePct:
		s.smartActionThreatSinglePct(ctx, motion, st, ev, invokerGUID)
	case smartActionThreatAllPct:
		s.smartActionThreatAllPct(ctx, motion, st, ev, invokerGUID)
	case smartActionAutoAttack:
		motion.NoMeleeAttack = ev.row.actionParam1 == 0
	case smartActionAllowCombatMovement:
		// SMART_ACTION_ALLOW_COMBAT_MOVEMENT (SmartScript.cpp:788-798):
		// me-only (not targets), gated on IsSmart — the dispatch only
		// runs for smart-script creatures. SmartAI::SetCombatMove
		// (SmartAI.cpp:787-815) flips _canCombatMove; the Go twin is
		// the inverted NoCombatMovement flag, honored by the combat-tick
		// chase gate. The engaged legs ride the next tick: on=true
		// re-chases via that gate (which carries the passive arm), and
		// on=false halts the active chase now (the Remove(chase)
		// twin) instead of letting the spline run to MoveEnds.
		move := ev.row.actionParam1 != 0
		s.motionMu.Lock()
		motion.NoCombatMovement = !move
		if !move && motion.InCombat {
			motion.Moving = false
		}
		s.motionMu.Unlock()
	case smartActionSetEventPhase:
		s.smartSetEventPhase(ctx, motion, st, ev.row.actionParam1, invokerGUID)
	case smartActionIncEventPhase:
		// SMART_ACTION_INC_EVENT_PHASE (SmartScript.cpp:809-819): IncPhase
		// then DecPhase, each through SetPhase, so a row with only one
		// nonzero leg still passes through the other as a no-op. IncPhase
		// clamps at PHASE_12 (SmartScript.cpp:3996-4000); DecPhase clamps
		// at 0 (SmartScript.cpp:4002-4008); each leg fires the
		// phase-change event only if it moved the phase.
		phase := st.phase
		if ev.row.actionParam1 > smartEventPhase12-phase {
			phase = smartEventPhase12
		} else {
			phase += ev.row.actionParam1
		}
		s.smartSetEventPhase(ctx, motion, st, phase, invokerGUID)
		if ev.row.actionParam2 >= st.phase {
			phase = 0
		} else {
			phase = st.phase - ev.row.actionParam2
		}
		s.smartSetEventPhase(ctx, motion, st, phase, invokerGUID)
	case smartActionFleeForAssist:
		// SMART_ACTION_FLEE_FOR_ASSIST (SmartScript.cpp:837-851):
		// DoFleeToGetAssistance on me. The withEmote leg (param1, 683
		// of 698 AGGRO rows) sends BROADCAST_TEXT_FLEE_FOR_ASSIST via
		// CreatureTextMgr — Go has no BroadcastText twin, so the emote
		// stays out, the CALL_FOR_HELP precedent.
		s.creatureFleeToGetAssistance(ctx, motion)
	case smartActionEvade:
		s.smartActionEvade(ctx, motion, st, ev, invokerGUID)
	case smartActionRandomPhase:
		// SMART_ACTION_RANDOM_PHASE (SmartScript.cpp:939-953): pick one of
		// the nonzero phases (params 1-6). All-zero rows are rejected at
		// load (SmartScriptMgr.cpp:1255-1267), so the empty pick is a
		// guard only — C++ would select from an empty vector there.
		phases := make([]uint32, 0, 6)
		for _, p := range [6]uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6} {
			if p != 0 {
				phases = append(phases, p)
			}
		}
		if len(phases) > 0 {
			s.smartSetEventPhase(ctx, motion, st, phases[rand.Intn(len(phases))], invokerGUID)
		}
	case smartActionRandomPhaseRange:
		// SMART_ACTION_RANDOM_PHASE_RANGE (SmartScript.cpp:954-965): urand
		// of phaseMin..phaseMax (params 1-2, inclusive). C++ urand ASSERTs
		// max >= min (Random.cpp:42); the clamp keeps an unvalidated row
		// defined instead of panicking rand.Intn.
		phaseMin, phaseMax := ev.row.actionParam1, ev.row.actionParam2
		if phaseMax < phaseMin {
			phaseMax = phaseMin
		}
		s.smartSetEventPhase(ctx, motion, st, phaseMin+uint32(rand.Intn(int(phaseMax-phaseMin+1))), invokerGUID)
	case smartActionResetGObject:
		s.smartActionResetGObject(ctx, motion, st, ev, invokerGUID)
	case smartActionCallKilledMonster:
		s.smartActionCallKilledMonster(ctx, motion, st, ev, invokerGUID)
	case smartActionAttackStart:
		// SMART_ACTION_ATTACK_START (SmartScript.cpp:1238-1249): engage a
		// random target. The engage runs the AttackStart funnel
		// (triggerCreatureAggro): its Evading skip is C++ Unit::Attack's
		// IsInEvadeMode gate (Unit.cpp: "creatures cannot attack while
		// evading"), not a deviation, and the passive skip does not fire
		// for fromAttack calls — C++ SmartAI::AttackStart has no
		// react-state gate on this path (SmartAI.cpp:593-608).
		targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
		if len(targets) > 0 {
			s.triggerCreatureAggro(ctx, motion.GUID, targets[rand.Intn(len(targets))].guid, true)
		}
	case smartActionKillUnit:
		s.smartActionKillUnit(ctx, motion, st, ev, invokerGUID)
	case smartActionSummonGO:
		s.smartActionSummonGO(ctx, motion, st, ev, invokerGUID)
	case smartActionActivateTaxi:
		s.smartActionActivateTaxi(ctx, motion, st, ev, invokerGUID)
	case smartActionWPStart:
		s.smartActionWPStart(ctx, motion, st, ev, invokerGUID)
	case smartActionWPPause:
		s.smartActionWPPause(ctx, motion, st, ev, invokerGUID)
	case smartActionWPStop:
		s.smartActionWPStop(ctx, motion, st, ev, invokerGUID)
	case smartActionAddItem:
		s.smartActionAddItem(ctx, motion, st, ev, invokerGUID)
	case smartActionRemoveItem:
		s.smartActionRemoveItem(ctx, motion, st, ev, invokerGUID)
	case smartActionInstallAITemplate:
		s.smartActionInstallAITemplate(ctx, motion, st, ev, invokerGUID)
	case smartActionSetRun:
		// SMART_ACTION_SET_RUN (SmartScript.cpp:1367-1374): me-only
		// (IsSmart gate = this dispatch), SmartAI::SetRun — the
		// smartSetRun twin (escort _run flag + node move types +
		// SetWalk). Targets are ignored by C++.
		s.smartSetRun(motion, st, ev.row.actionParam1 != 0)
	case smartActionSetDisableGravity:
		s.smartActionSetDisableGravity(ctx, motion, st, ev, invokerGUID)
	case smartActionSetSwim:
		s.smartActionSetSwim(ctx, motion, st, ev, invokerGUID)
	case smartActionTeleport:
		s.smartActionTeleport(ctx, motion, st, ev, invokerGUID)
	case smartActionSetCounter:
		s.smartActionSetCounter(ctx, motion, st, ev, invokerGUID)
	case smartActionWPResume:
		s.smartActionWPResume(ctx, motion, st, ev, invokerGUID)
	case smartActionSetOrientation:
		s.smartActionSetOrientation(ctx, motion, st, ev, invokerGUID)
	case smartActionPlayMovie:
		s.smartActionPlayMovie(ctx, motion, st, ev, invokerGUID)
	case smartActionMoveToPos:
		s.smartActionMoveToPos(ctx, motion, st, ev, invokerGUID)
	case smartActionEnableTempGObj:
		s.smartActionEnableTempGObj(ctx, motion, st, ev, invokerGUID)
	case smartActionEquip:
		s.smartActionEquip(ctx, motion, st, ev, invokerGUID)
	case smartActionCloseGossip:
		s.smartActionCloseGossip(ctx, motion, st, ev, invokerGUID)
	case smartActionAddAura:
		s.smartActionAddAura(ctx, motion, st, ev, invokerGUID)
	case smartActionOverrideScriptBaseObject:
		s.smartActionOverrideScriptBaseObject(ctx, motion, st, ev, invokerGUID)
	case smartActionResetScriptBaseObject:
		s.smartActionResetScriptBaseObject(ctx, motion, st, ev, invokerGUID)
	case smartActionCallScriptReset:
		s.smartActionCallScriptReset(ctx, motion, st, ev, invokerGUID)
	case smartActionSetRangedMovement:
		s.smartActionSetRangedMovement(ctx, motion, st, ev, invokerGUID)
	case smartActionRemoveAurasFromSpell:
		s.smartActionRemoveAuras(ctx, motion, st, ev, invokerGUID)
	case smartActionSetInstData:
		s.smartActionSetInstData(ctx, motion, ev)
	case smartActionCallForHelp:
		s.callCreatureForHelp(ctx, motion, float32(ev.row.actionParam1))
		// withEmote (param2, 2 AGGRO rows): C++ sends the
		// BROADCAST_TEXT_CALL_FOR_HELP monster emote
		// (SmartScript.cpp:1080-1091); Go has no BroadcastText twin,
		// so the emote leg stays out — the engage itself ports.
	case smartActionDie:
		// SMART_ACTION_DIE (SmartScript.cpp:1062-1070): if (me &&
		// !me->isDead()) me->KillSelf() — Unit::Kill(this, this)
		// (Unit.h:941). The Go twin is creatureInstaKillVictim with
		// the creature as its own killer (the Lua Kill bridge's self
		// leg); its Health == 0 early-out is the isDead guard, and
		// the victim's SmartAI DEATH rows fire inside that path at
		// the C++ JustDied position (Unit.cpp:11385-11391), so the 4
		// DEATH rows on DIE entries run. TDB 26091: 61 rows, all
		// params 0, SELF targets.
		s.creatureInstaKillVictim(ctx, motion, motion.GUID)
	case smartActionSetInCombatWithZone:
		// SMART_ACTION_SET_IN_COMBAT_WITH_ZONE (SmartScript.cpp:
		// 1071-1079): me->AI()->DoZoneInCombat() when me && IsAIEnabled
		// — the dispatch only runs for SmartAI creatures, the
		// IsAIEnabled twin. The zone engage is the shared
		// creatureDoZoneInCombat helper (the Lua DoZoneInCombat
		// bridge refactored onto it). TDB 26091: 34 rows, no params.
		s.creatureDoZoneInCombat(motion.Map, motion.InstanceID, motion.GUID)
	case smartActionSetSheath:
		// SMART_ACTION_SET_SHEATH (SmartScript.cpp:1094-1103):
		// Unit::SetSheath — SetByteValue(UNIT_FIELD_BYTES_2, sheath)
		// (Unit.h:903); Creature does not override, so the byte write
		// alone. Load-rejects sheath >= MAX_SHEATH_STATE
		// (SmartScriptMgr.cpp:1322-1328, 0 passes); TDB 26091: 31
		// rows, all 0-2, SELF targets. The Go twin is the motion
		// SheathState + Known flag (the Lua SetSheath precedent);
		// the spawn overlay applies it client-visible.
		if ev.row.actionParam1 <= 2 {
			s.motionMu.Lock()
			motion.SheathState = uint8(ev.row.actionParam1)
			motion.SheathStateKnown = true
			s.motionMu.Unlock()
		}
	case smartActionSetIngamePhaseMask:
		s.smartActionSetIngamePhaseMask(ctx, motion, st, ev, invokerGUID)
	case smartActionSetData:
		s.smartActionSetData(ctx, motion, st, ev, invokerGUID)
	case smartActionUpdateTemplate:
		s.smartActionUpdateTemplate(ctx, motion, st, ev, invokerGUID)
	case smartActionForceDespawn:
		s.smartActionForceDespawn(ctx, motion, st, ev, invokerGUID)
	case smartActionSetInvincibilityHPLevel:
		s.smartActionSetInvincibilityHPLevel(ctx, motion, st, ev, invokerGUID)
	case smartActionMountToEntryOrModel:
		s.smartActionMountToEntryOrModel(ctx, motion, st, ev, invokerGUID)
	case smartActionAttackStop:
		s.smartActionAttackStop(ctx, motion, st, ev, invokerGUID)
	case smartActionMorphToEntryOrModel:
		s.smartActionMorphToEntryOrModel(ctx, motion, st, ev, invokerGUID)
	case smartActionRespawnBySpawnID:
		s.smartActionRespawnBySpawnID(ctx, motion, ev)
	case smartActionStoreTargetList:
		targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
		guids := make([]uint64, 0, len(targets))
		for _, tgt := range targets {
			guids = append(guids, tgt.guid)
		}
		if st.storedTargets == nil {
			st.storedTargets = make(map[uint32][]uint64)
		}
		st.storedTargets[ev.row.actionParam1] = guids
	case smartActionCreateTimedEvent:
		// SMART_ACTION_CREATE_TIMED_EVENT (SmartScript.cpp:1586-1613):
		// builds a synthetic UPDATE holder — chance (0 defaults to
		// 100), initial window (min/max), repeat window, NOT_REPEATABLE
		// when no repeat window — whose action is TRIGGER_TIMED_EVENT
		// for the id, then stores it in mStoredEvents. The target
		// rides along (ev.target = e.target), so the trigger resolves
		// the creator's targets. InitTimer rolls the initial window.
		s.smartCreateTimedEvent(st, ev)
	case smartActionTriggerTimedEvent:
		// SMART_ACTION_TRIGGER_TIMED_EVENT (SmartScript.cpp:1615-1620):
		// fires every TIMED_EVENT_TRIGGERED row for the id (invoker
		// nil in C++), then queues the id for removal when the firing
		// event is NOT_REPEATABLE — for a stored event's own fire
		// that is the stored holder's flag, which is what C++ reads.
		s.smartProcessEvents(ctx, motion, smartEventTimedEventTriggered, 0, ev.row.actionParam1, 0)
		if ev.row.eventFlags&smartEventFlagNotRepeatable != 0 {
			st.remTimedEventIDs = append(st.remTimedEventIDs, ev.row.actionParam1)
		}
	case smartActionRemoveTimedEvent:
		// SMART_ACTION_REMOVE_TIMED_EVENT (SmartScript.cpp:1622-1623):
		// queues the id for removal; the drain runs after the
		// timed-list leg, matching OnUpdate's order.
		st.remTimedEventIDs = append(st.remTimedEventIDs, ev.row.actionParam1)
	case smartActionTriggerRandomTimedEvent:
		// SMART_ACTION_TRIGGER_RANDOM_TIMED_EVENT (SmartScript.cpp:
		// 2295-2298): a random id in [minId, maxId] fires its
		// TIMED_EVENT_TRIGGERED rows; the stored holder keeps ticking
		// on its own repeat window — no removal queued.
		minID, maxID := ev.row.actionParam1, ev.row.actionParam2
		if maxID < minID {
			maxID = minID
		}
		id := minID
		if maxID > minID {
			id = minID + uint32(rand.Int63n(int64(maxID-minID+1)))
		}
		s.smartProcessEvents(ctx, motion, smartEventTimedEventTriggered, 0, id, 0)
	case smartActionCallTimedActionList, smartActionCallRandomTimedActionList, smartActionCallRandomRangeTimedActionList:
		s.smartActionCallTimedActionList(ctx, motion, st, ev, invokerGUID)
	case smartActionSetNPCFlag, smartActionAddNPCFlag, smartActionRemoveNPCFlag:
		s.smartActionNPCFlag(ctx, motion, st, ev, invokerGUID)
	case smartActionRandomMove:
		s.smartActionRandomMove(ctx, motion, st, ev, invokerGUID)
	case smartActionSetUnitFieldBytes1:
		s.smartActionSetUnitFieldBytes1(ctx, motion, st, ev, invokerGUID)
	case smartActionRemoveUnitFieldBytes1:
		s.smartActionRemoveUnitFieldBytes1(ctx, motion, st, ev, invokerGUID)
	case smartActionJumpToPos:
		s.smartActionJumpToPos(ctx, motion, st, ev, invokerGUID)
	case smartActionSendGossipMenu:
		s.smartActionSendGossipMenu(ctx, motion, st, ev, invokerGUID)
	case smartActionGOSetLootState:
		s.smartActionGOSetLootState(ctx, motion, st, ev, invokerGUID)
	case smartActionSendTargetToTarget:
		s.smartActionSendTargetToTarget(ctx, motion, st, ev, invokerGUID)
	case smartActionSetHomePos:
		s.smartActionSetHomePos(ctx, motion, st, ev, invokerGUID)
	case smartActionSetHealthRegen:
		s.smartActionSetHealthRegen(ctx, motion, st, ev, invokerGUID)
	case smartActionSummonCreatureGroup:
		s.smartActionSummonCreatureGroup(ctx, motion, st, ev, invokerGUID)
	case smartActionSetPower:
		s.smartActionSetPower(ctx, motion, st, ev, invokerGUID)
	case smartActionGameEventStop:
		s.smartActionGameEventStop(ctx, motion, st, ev, invokerGUID)
	case smartActionGameEventStart:
		s.smartActionGameEventStart(ctx, motion, st, ev, invokerGUID)
	case smartActionStartClosestWaypoint:
		s.smartActionStartClosestWaypoint(ctx, motion, st, ev, invokerGUID)
	case smartActionMoveOffset:
		s.smartActionMoveOffset(ctx, motion, st, ev, invokerGUID)
	case smartActionRandomSound:
		s.smartActionRandomSound(ctx, motion, st, ev, invokerGUID)
	case smartActionSetCorpseDelay:
		s.smartActionSetCorpseDelay(ctx, motion, st, ev, invokerGUID)
	case smartActionDisableEvade:
		// SMART_ACTION_DISABLE_EVADE (SmartScript.cpp:2247-2253):
		// me-only (not targets), gated on IsSmart — the dispatch
		// only runs for smart-script creatures — and SmartAI::
		// SetEvadeDisabled flips _evadeDisabled, which the evade
		// entry reads from the motion-level twin. TDB 26091: 20
		// rows, param1 1 on 16, 0 on 4.
		motion.EvadeDisabled = ev.row.actionParam1 != 0
	case smartActionPauseMovement:
		s.smartActionPauseMovement(ctx, motion, st, ev, invokerGUID)
	case smartActionGoSetGoState:
		s.smartActionGOSetGoState(ctx, motion, st, ev, invokerGUID)
	case smartActionSpawnSpawnGroup:
		s.smartActionSpawnSpawnGroup(ctx, motion, st, ev, invokerGUID)
	case smartActionDespawnSpawnGroup:
		s.smartActionDespawnSpawnGroup(ctx, motion, st, ev, invokerGUID)
	case smartActionInvokerCast:
		s.smartActionInvokerCast(ctx, motion, st, ev, invokerGUID)
	case smartActionOverrideLight:
		// SMART_ACTION_OVERRIDE_LIGHT (SmartScript.cpp:2350-2360):
		// the base object's map sets the zone override light — zone
		// (param1), area light (param2), override light (param3, 0
		// clears), transition ms (param4). The Go twin is
		// setZoneLightOverride (weather.go, Map::SetZoneOverrideLight
		// Map.cpp:4802-4841): same erase-then-maybe-append store and
		// same zone broadcast, per-zone where C++ is per-Map. TDB
		// 26091: 2 rows, Rhonin script 3 (zone 4395, area 1802,
		// override 1773 / 0, transition 1000 / 5000).
		s.setZoneLightOverride(ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4)
	case smartActionOverrideWeather:
		// SMART_ACTION_OVERRIDE_WEATHER (SmartScript.cpp:2360-2370):
		// the base object's map sets the zone weather — zone
		// (param1), weather state (param2), intensity (param3, cast
		// to float in C++). The Go twin is setZoneWeatherOverride
		// (weather.go, Map::SetZoneWeather Map.cpp:4783-4800):
		// same per-zone store and broadcast, per-zone where C++ is
		// per-Map. TDB 26091: 2 rows, Mistwhisper Lightning Cloud
		// (zone 3711, state 5 HEAVY_RAIN, intensity 1 / 0).
		s.setZoneWeatherOverride(ev.row.actionParam1, ev.row.actionParam2, float32(ev.row.actionParam3))
	default:
		// In-enum actions with no Go twin yet land here (the C++
		// log-and-skip default, SmartScript.cpp:2370-2372). Rows
		// with action_type >= SMART_ACTION_END (140) never reach
		// this arm on either side: C++ load-rejects them
		// (SmartScriptMgr.cpp:682-686) and loadSmartScripts does
		// the same, so 146's 70 TDB rows (and 142/144/145/147)
		// are dead at load, not at dispatch. The event still
		// consumed its chance roll and link below.
	}
	if ev.row.link != 0 && ev.row.link != ev.row.id {
		if st.linkDepth >= smartMaxNestedEvents {
			return
		}
		st.linkDepth++
		for _, linked := range st.events {
			if linked.row.id == ev.row.link && linked.row.eventType == smartEventLink {
				s.smartProcessEvent(ctx, motion, st, linked, invokerGUID, var0, var1)
				break
			}
		}
		st.linkDepth--
	}
}

// creatureDoZoneInCombat is CreatureAI::DoZoneInCombat
// (CreatureAI.cpp:73-103): on a dungeon map, the creature engages every
// alive, world-ready, non-GM player on its map/instance with a zero-threat
// seed (Unit::EngageWithTarget, Unit.cpp:8429-8438 twin: ThreatMgr
// AddThreat(guid, 0.0, true) + InCombat). C++ legs carried: the
// IsDungeon gate (Go has no script logger on this path, so the
// C++ error log becomes a silent no-op), the empty-player-list early
// return, the dead-player skip, and CombatManager::CanBeginCombat's
// gates as Go models them — alive, same map/instance from the session
// enumeration (Go has no phase model, and the world-ready state is the
// in-world twin), GM off (playerExtraGMOn) — plus the controlled-unit
// leg: each qualifying player's pet motions (OwnerGUID,
// creaturemotion.go:1160) are engaged too, like C++'s m_Controlled
// loop. The vehicle-base leg has no Go twin (no vehicle model; the
// same gap is documented on AddThreat in threat.go). EngageWithTarget
// sends no packets in C++ (SMSG_ATTACK_START rides Unit::Attack, not
// the engage), so this twin seeds threat and sets InCombat without
// broadcasting. sessionsMu is never taken under motionMu: the player
// targets resolve first, then one motion lock applies the seeds.
// mandokir's JustEngagedWith tail ports on this, as does the SmartAI
// SET_IN_COMBAT_WITH_ZONE action (38).
func (s *Server) creatureDoZoneInCombat(mapID, instanceID uint32, guid uint64) {
	if entry, found, err := s.Data.Map(mapID); err != nil || !found || !entry.IsDungeon() {
		return
	}
	var players []uint64
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		p := sess.player
		if p == nil || !sess.worldReady.Load() {
			continue
		}
		if p.Map != mapID || p.InstanceID != instanceID || p.Health == 0 {
			continue
		}
		if p.ExtraFlags&playerExtraGMOn != 0 {
			continue
		}
		players = append(players, sess.playerGUID)
	}
	s.sessionsMu.RUnlock()
	if len(players) == 0 {
		return
	}
	s.motionMu.Lock()
	m := s.findCreatureMotionLocked(mapID, instanceID, guid)
	if m != nil && m.Health > 0 {
		wasInCombat := m.InCombat
		if m.ThreatMgr == nil {
			m.ThreatMgr = NewThreatManager(m)
		}
		for _, pg := range players {
			s.creatureAddThreat(m, pg, 0.0, true, s.findSessionByGUID(pg), nil)
			for _, pm := range s.motionMapLocked(mapID, instanceID) {
				if pm != nil && pm.OwnerGUID == pg && pm.Health > 0 {
					s.creatureAddThreat(m, pm.GUID, 0.0, true, nil, pm)
				}
			}
		}
		// CreatureAI::DoZoneInCombat (CreatureAI.cpp:75-104) engages via
		// EngageWithTarget -> AddThreat, so a fresh zone engage dismounts
		// and re-homes (Creature::AtEngage, Creature.cpp:3435-3436,
		// 3446-3447); later players add no second engage.
		if !wasInCombat {
			atEngageDismount(m)
			atEngageSetHomePosition(m)
		}
		m.InCombat = true
	}
	s.motionMu.Unlock()
}

// smartSetEventPhase is SmartScript::SetPhase (SmartScript.cpp:4010-4018):
// store the phase number and fire SMART_EVENT_EVENT_PHASE_CHANGE only when
// it moved.
func (s *Server) smartSetEventPhase(ctx context.Context, motion *creatureMotion, st *smartScriptState, phase uint32, invokerGUID uint64) {
	oldPhase := st.phase
	st.phase = phase
	if oldPhase != st.phase {
		s.smartProcessEvents(ctx, motion, smartEventPhaseChange, invokerGUID, 0, 0)
	}
}

// smartActionCast is SMART_ACTION_CAST (SmartScript.cpp:543-599). Param
// mapping (SmartScriptMgr.h cast struct): param1 spell, param2 cast flags,
// param3 trigger flags, param4 targets limit. TRIGGERED (0x02) casts ride
// castCreatureSpellTriggered; a nonzero triggerFlags value collapses to
// that same full-mask trigger — Go has no per-flag triggered variant.
// AURA_NOT_PRESENT (0x20, 1 AGGRO row) skips targets that already carry
// the aura. INTERRUPT_PREVIOUS (0x01, 0 AGGRO rows) interrupts first.
// COMBAT_MOVE (0x40, 4 rows) stays out: C++ keys it on the cast result
// (SetCombatMove(spellCastFailed, true)) and the Go cast helpers return
// no result to key it on. The all-failed retry (RaisePriority) stays out
// for the same reason. The stale FORCE_CAST flag (0x04, 1 row) is a
// no-op in C++ too (commented out of SmartCastFlags).
func (s *Server) smartActionCast(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		return
	}
	if limit := ev.row.actionParam4; limit > 0 && uint32(len(targets)) > limit {
		rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })
		targets = targets[:limit]
	}
	castFlags := ev.row.actionParam2
	if castFlags&smartCastInterruptPrevious != 0 {
		for _, arm := range s.interruptCreatureNonMeleeCasts(motion) {
			s.sendCreatureInterruptPackets(motion, arm)
		}
	}
	triggered := castFlags&smartCastTriggered != 0
	for _, tgt := range targets {
		if castFlags&smartCastAuraNotPresent != 0 && s.creatureSpellTargetHasAura(motion, tgt.guid, ev.row.actionParam1) {
			continue
		}
		if triggered {
			s.castCreatureSpellTriggered(ctx, motion, ev.row.actionParam1, tgt.guid)
		} else {
			s.castCreatureSpell(ctx, motion, ev.row.actionParam1, tgt.guid)
		}
	}
}

// smartActionSummonCreature is SMART_ACTION_SUMMON_CREATURE
// (SmartScript.cpp:1251-1277). Param mapping (SmartScriptMgr.h:471:
// CreatureID, summonType, duration in ms, attackInvoker): param1
// entry, param2 C++ TempSummonType (1-8), param3 duration ms, param4
// attackInvoker. The type translates down by one into the bridge's
// 0-based scheme (the SummonCreatureGroup precedent,
// lua_creature_events.go:4841); rows outside 1..8 are skipped, the
// C++ `> TEMPSUMMON_MANUAL_DESPAWN` skip twin — C++ itself would
// summon then instantly UnSummon on the Update default arm
// (TemporarySummon.cpp:168-170), so the skip loses at most one tick
// of a creature no faithful row produces. For each target the summon
// lands at the target position plus the row's target_x/y/z/o offset;
// a POSITION target resolves to no unit (GetTargets returns nothing,
// SmartScript.cpp:2864-2867), so that leg summons once at the row's
// absolute target position. attackInvoker runs the EngageWithTarget
// funnel (triggerCreatureAggro) — the C++ AttackStart twin — on the
// target unit, or on the invoker (falling back to the last invoker,
// the smartTargets convention) for the POSITION leg; the funnel's
// Evading skip is C++ Unit::Attack's IsInEvadeMode gate and the passive
// skip does not fire for fromAttack calls, so neither leg deviates on
// this path (see the smartActionAttackStart comment).
func (s *Server) smartActionSummonCreature(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	summonType := ev.row.actionParam2
	if summonType < 1 || summonType > 8 {
		return
	}
	origin := s.summonOriginFromMotion(motion)
	attackInvoker := ev.row.actionParam4 != 0
	summonAt := func(x, y, z, o float32, attackGUID uint64) {
		summoned := s.summonTempCreature(ctx, origin, ev.row.actionParam1, x, y, z, o, uint8(summonType-1), ev.row.actionParam3)
		if summoned == nil || !attackInvoker || attackGUID == 0 {
			return
		}
		s.triggerCreatureAggro(ctx, summoned.GUID, attackGUID, true)
	}
	if ev.row.targetType == smartTargetPosition {
		invoker := invokerGUID
		if invoker == 0 {
			invoker = st.lastInvoker
		}
		summonAt(ev.row.targetX, ev.row.targetY, ev.row.targetZ, ev.row.targetO, invoker)
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		x, y, z, o, ok := s.smartTargetPosition(ctx, motion, tgt)
		if !ok {
			continue
		}
		summonAt(x+ev.row.targetX, y+ev.row.targetY, z+ev.row.targetZ, o+ev.row.targetO, tgt.guid)
	}
}

// smartTargetPosition resolves one SmartAI target to its world position:
// a player reads the session state, a creature its motion, a gameobject
// its live dynamic state (Orientation included) or, for static spawns,
// the gameobject DB row (commands.go:2155 precedent).
func (s *Server) smartTargetPosition(ctx context.Context, motion *creatureMotion, tgt smartTarget) (x, y, z, o float32, ok bool) {
	if tgt.isGameObject {
		if st := s.gameObjectState(motion.Map, motion.InstanceID, tgt.guid); st != nil {
			return st.X, st.Y, st.Z, st.Orientation, true
		}
		if s.WorldStore != nil && s.WorldStore.DB != nil {
			var goMap uint32
			var gx, gy, gz, gori float32
			if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT map, position_x, position_y, position_z, orientation FROM gameobject WHERE guid = ?", uint32(tgt.guid&0xFFFFFF)).Scan(&goMap, &gx, &gy, &gz, &gori); err == nil && goMap == motion.Map {
				return gx, gy, gz, gori, true
			}
		}
		return 0, 0, 0, 0, false
	}
	if tgt.isPlayer {
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || sess.player == nil {
			return 0, 0, 0, 0, false
		}
		return sess.player.X, sess.player.Y, sess.player.Z, sess.player.Orientation, true
	}
	tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
	if tm == nil {
		return 0, 0, 0, 0, false
	}
	return tm.X, tm.Y, tm.Z, tm.Orientation, true
}

// smartActionTalk is SMART_ACTION_TALK (SmartScript.cpp:260-294): the
// talker is the first non-pet creature target (or self when the target is
// a player, when useTalkTarget is set, or when the target type is NONE
// with no targets); the talk target is that creature/player, falling back
// to the last invoker. The talk target rides broadcastCreatureTalk's
// whisper slot, which only whispers for whisper-type rows — for SAY/YELL
// rows it is ignored, matching C++ SendChat. The duration arms
// SMART_EVENT_TEXT_OVER from the tick (a 0 duration fires on the next
// tick, like C++'s mTextTimer = 0 arm).
func (s *Server) smartActionTalk(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	talker := (*creatureMotion)(nil)
	talkTargetGUID := uint64(0)
	if ev.row.targetType == smartTargetNone {
		talker = motion
	}
	for _, tgt := range targets {
		if tgt.isPlayer {
			talker = motion
			talkTargetGUID = tgt.guid
			break
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || motionIsPet(tm) {
			continue
		}
		if ev.row.actionParam3 != 0 {
			talker = motion
			talkTargetGUID = tgt.guid
		} else {
			talker = tm
		}
		break
	}
	if talker == nil {
		return
	}
	if talkTargetGUID == 0 {
		talkTargetGUID = st.lastInvoker
	}
	st.talkerEntry = talker.Entry
	st.lastTextID = ev.row.actionParam1
	st.textTimer = ev.row.actionParam2
	st.useTextTimer = true
	s.broadcastCreatureTalk(ctx, talker.Map, talker.InstanceID, talker.GUID, talker.Entry, talker.Name, uint8(ev.row.actionParam1), talkTargetGUID)
}

// smartActionSimpleTalk is SMART_ACTION_SIMPLE_TALK
// (SmartScript.cpp:309-327): every target says the creature_text group
// (param1) itself — no duration/text-over arm, no talker selection like
// TALK. A player target makes the PLAYER say the text: C++ SendChat
// passes the player as srcPlr, so the packet carries the player's
// GUID/name and fans out around the player, while the sound still plays
// from me (broadcastCreatureTalkAsPlayer). The whisper target on both
// legs is the last invoker when it is a player (GetLastInvoker,
// :314/:317). TDB 26091: 4 rows, all ACTION_INVOKER targets, groups 0
// (3 rows) and 6 (1 row); the 3 timed-list rows name entries with no
// creature_text rows at all, so the live row is 24238's SPELLHIT
// group-0 YELL.
func (s *Server) smartActionSimpleTalk(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	whisperTarget := uint64(0)
	if s.smartIsPlayerGUID(st.lastInvoker) {
		whisperTarget = st.lastInvoker
	}
	for _, tgt := range targets {
		if tgt.isGameObject {
			continue
		}
		if tgt.isPlayer {
			s.broadcastCreatureTalkAsPlayer(ctx, motion, tgt.guid, uint8(ev.row.actionParam1), whisperTarget)
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.broadcastCreatureTalk(ctx, tm.Map, tm.InstanceID, tm.GUID, tm.Entry, tm.Name, uint8(ev.row.actionParam1), whisperTarget)
	}
}

// smartActionSelfCast is SMART_ACTION_SELF_CAST (SmartScript.cpp:607-640):
// every target unit casts the spell (param1) on ITSELF — the caster is the
// target, not me (the SMART_ACTION_CAST twin casts from me instead). The
// targetsLimit (param4) randomly trims the list, TRIGGERED (param2 0x02)
// rides the triggered cast, and AURA_NOT_PRESENT (0x20) skips targets
// that already carry the aura; INTERRUPT_PREVIOUS (0x01) interrupts the
// target's non-melee casts before its own cast. TDB 26091: 481 rows, all
// triggerFlags (param3) 0 and targetsLimit 0; castFlags 0 (308),
// TRIGGERED (172), TRIGGERED|AURA_NOT_PRESENT (1); no row uses
// INTERRUPT_PREVIOUS. Player targets ride the session cast path
// (castSpellDirect, the triggered pipeline — the Lua player:CastSpell
// precedent); a player target's interrupt leg has no Go twin (the
// SMART_ACTION_INTERRUPT_SPELL precedent) and stays out.
func (s *Server) smartActionSelfCast(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		return
	}
	if limit := ev.row.actionParam4; limit > 0 && uint32(len(targets)) > limit {
		rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })
		targets = targets[:limit]
	}
	castFlags := ev.row.actionParam2
	triggered := castFlags&smartCastTriggered != 0
	for _, tgt := range targets {
		if tgt.isGameObject {
			continue
		}
		if castFlags&smartCastAuraNotPresent != 0 && s.creatureSpellTargetHasAura(motion, tgt.guid, ev.row.actionParam1) {
			continue
		}
		if tgt.isPlayer {
			sess := s.findSessionByGUID(tgt.guid)
			if sess == nil || sess.player == nil {
				continue
			}
			sess.castSpellDirect(ctx, ev.row.actionParam1, tgt.guid)
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if castFlags&smartCastInterruptPrevious != 0 {
			for _, arm := range s.interruptCreatureNonMeleeCasts(tm) {
				s.sendCreatureInterruptPackets(tm, arm)
			}
		}
		if triggered {
			s.castCreatureSpellTriggered(ctx, tm, ev.row.actionParam1, tgt.guid)
		} else {
			s.castCreatureSpell(ctx, tm, ev.row.actionParam1, tgt.guid)
		}
	}
}

// smartActionCrossCast is SMART_ACTION_CROSS_CAST
// (SmartScript.cpp:1721-1761): the caster list resolves from the
// crossCast target (param3-6) against a synthetic UPDATE_IC event —
// the destination list is the row's own target. Every caster casts the
// spell (param1) on every destination unit: AURA_NOT_PRESENT (param2
// 0x20) skips destinations that already carry the aura, TRIGGERED
// (0x02) rides the triggered cast (a plain bool in C++, the
// TRIGGERED_FULL_MASK twin), and INTERRUPT_PREVIOUS (0x01) interrupts
// the caster once, before its first cast. TDB 26091: 127 rows; casters
// CLOSEST_CREATURE (63), CREATURE_GUID (15), ACTION_INVOKER (12),
// STORED (9), SELF (8), CREATURE_DISTANCE (8), OWNER_OR_SUMMONER (3),
// VEHICLE_PASSENGER (3, all seatMask 0), PLAYER_DISTANCE (2),
// CREATURE_RANGE (2), CLOSEST_PLAYER (1), VICTIM (1); castFlags 0 (99),
// TRIGGERED (25), AURA_NOT_PRESENT (3); no row uses
// INTERRUPT_PREVIOUS. Player casters ride the session cast path like
// SELF_CAST; the player-caster interrupt leg has no Go twin.
func (s *Server) smartActionCrossCast(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		return
	}
	casterRow := *ev.row
	casterRow.targetType = ev.row.actionParam3
	casterRow.targetParam1 = ev.row.actionParam4
	casterRow.targetParam2 = ev.row.actionParam5
	casterRow.targetParam3 = ev.row.actionParam6
	casterRow.targetParam4 = 0
	casters := s.smartTargets(ctx, motion, st, &smartScriptEvent{row: &casterRow}, invokerGUID)
	castFlags := ev.row.actionParam2
	triggered := castFlags&smartCastTriggered != 0
	for _, caster := range casters {
		if caster.isGameObject {
			continue
		}
		var casterMotion *creatureMotion
		var casterSess *session
		if caster.isPlayer {
			casterSess = s.findSessionByGUID(caster.guid)
			if casterSess == nil || casterSess.player == nil {
				continue
			}
		} else {
			casterMotion = s.findCreatureMotion(motion.Map, motion.InstanceID, caster.guid)
			if casterMotion == nil {
				continue
			}
		}
		interruptedSpell := false
		for _, tgt := range targets {
			if tgt.isGameObject {
				continue
			}
			if tgt.isPlayer {
				if sess := s.findSessionByGUID(tgt.guid); sess == nil || sess.player == nil {
					continue
				}
			} else if s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid) == nil {
				continue
			}
			if castFlags&smartCastAuraNotPresent != 0 && s.creatureSpellTargetHasAura(motion, tgt.guid, ev.row.actionParam1) {
				continue
			}
			if castFlags&smartCastInterruptPrevious != 0 && !interruptedSpell {
				if casterMotion != nil {
					for _, arm := range s.interruptCreatureNonMeleeCasts(casterMotion) {
						s.sendCreatureInterruptPackets(casterMotion, arm)
					}
				}
				interruptedSpell = true
			}
			if casterSess != nil {
				casterSess.castSpellDirect(ctx, ev.row.actionParam1, tgt.guid)
			} else if triggered {
				s.castCreatureSpellTriggered(ctx, casterMotion, ev.row.actionParam1, tgt.guid)
			} else {
				s.castCreatureSpell(ctx, casterMotion, ev.row.actionParam1, tgt.guid)
			}
		}
	}
}

// smartActionInvokerCast is SMART_ACTION_INVOKER_CAST
// (SmartScript.cpp:640-679): the last invoker casts the spell
// (param1) on every target unit. The invoker is GetLastInvoker
// (SmartScript.cpp:3984-3994) — mLastInvoker, set in ProcessAction
// only when the event carries an invoker and cleared on reset, the
// st.lastInvoker twin; a missing invoker breaks the action. The
// targetsLimit (param4) randomly trims the list, TRIGGERED (param2
// 0x02) rides the triggered cast, AURA_NOT_PRESENT (0x20) skips
// targets that already carry the aura, and INTERRUPT_PREVIOUS (0x01)
// interrupts the invoker's non-melee casts — per target in C++, but
// repeat interrupts are no-ops, so Go interrupts once before the
// first cast (the CROSS_CAST precedent). triggerFlags (param3)
// collapses to the full-mask trigger, the SELF_CAST precedent — Go
// has no per-flag triggered variant. Player invokers ride the
// session cast path (castSpellDirect, the CROSS_CAST precedent); a
// player invoker's interrupt leg has no Go twin (the
// SMART_ACTION_INTERRUPT_SPELL precedent). TDB 26091: 35 rows, all
// triggerFlags 0 and targetsLimit 0; castFlags 0 (23), TRIGGERED
// (10), TRIGGERED|INTERRUPT_PREVIOUS (2); targets SELF (18) /
// ACTION_INVOKER (17). Event coverage: LINK (14) / DEATH (2) /
// timed UPDATE (2) fire in Go; GOSSIP_SELECT (11) has no fire path
// yet (carried flag), nor do SPELLCLICK (2), PASSENGER_REMOVED (2)
// or JUST_SUMMONED (2).
func (s *Server) smartActionInvokerCast(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	invoker := st.lastInvoker
	if invoker == 0 {
		return
	}
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		return
	}
	if limit := ev.row.actionParam4; limit > 0 && uint32(len(targets)) > limit {
		rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })
		targets = targets[:limit]
	}
	castFlags := ev.row.actionParam2
	triggered := castFlags&smartCastTriggered != 0
	var invokerMotion *creatureMotion
	var invokerSess *session
	if s.smartIsPlayerGUID(invoker) {
		invokerSess = s.findSessionByGUID(invoker)
		if invokerSess == nil || invokerSess.player == nil {
			return
		}
	} else {
		invokerMotion = s.findCreatureMotion(motion.Map, motion.InstanceID, invoker)
		if invokerMotion == nil {
			return
		}
	}
	interrupted := false
	for _, tgt := range targets {
		if tgt.isGameObject {
			continue
		}
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess == nil || sess.player == nil {
				continue
			}
		} else if s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid) == nil {
			continue
		}
		if castFlags&smartCastAuraNotPresent != 0 && s.creatureSpellTargetHasAura(motion, tgt.guid, ev.row.actionParam1) {
			continue
		}
		if castFlags&smartCastInterruptPrevious != 0 && !interrupted {
			if invokerMotion != nil {
				for _, arm := range s.interruptCreatureNonMeleeCasts(invokerMotion) {
					s.sendCreatureInterruptPackets(invokerMotion, arm)
				}
			}
			interrupted = true
		}
		if invokerSess != nil {
			invokerSess.castSpellDirect(ctx, ev.row.actionParam1, tgt.guid)
		} else if triggered {
			s.castCreatureSpellTriggered(ctx, invokerMotion, ev.row.actionParam1, tgt.guid)
		} else {
			s.castCreatureSpell(ctx, invokerMotion, ev.row.actionParam1, tgt.guid)
		}
	}
}

// smartVehiclePassengerTargets is SMART_TARGET_VEHICLE_PASSENGER
// (SmartScript.cpp:2756-2765): the passengers seated on me's vehicle
// kit, seatMask selecting seats by bit (0 = all seats), in seat order.
// TDB 26091: 3 CROSS_CAST caster rows, all seatMask 0.
func (s *Server) smartVehiclePassengerTargets(motion *creatureMotion, seatMask uint32) []smartTarget {
	kit := s.smartVehicleKitForTarget(motion, motion.GUID)
	if kit == nil {
		return nil
	}
	var out []smartTarget
	for _, p := range kit.seatedPassengerSeats() {
		if seatMask != 0 && (p.seat < 0 || p.seat >= 32 || seatMask&(uint32(1)<<uint(p.seat)) == 0) {
			continue
		}
		out = append(out, smartTarget{guid: p.guid, isPlayer: s.smartIsPlayerGUID(p.guid)})
	}
	return out
}

// smartPlaySound is the shared WorldObject::PlayDirectSound /
// PlayDistanceSound send (Object.cpp:3488-3494; SOUND at
// SmartScript.cpp:346-361, RANDOM_SOUND at :2128-2150): distance sends
// SMSG_PLAY_OBJECT_SOUND with the target's GUID (distance-attenuated
// client-side), otherwise SMSG_PLAY_SOUND. onlySelf sends to the target
// player alone — a creature target's ToPlayer() is nil, so onlySelf only
// ever directs a player target; otherwise the packet goes to the visible
// set around the target. GameObject targets are not units and are skipped.
func (s *Server) smartPlaySound(motion *creatureMotion, tgt smartTarget, sound uint32, onlySelf, distance bool) {
	if tgt.isGameObject {
		return
	}
	var opcode uint16
	var payload []byte
	if distance {
		opcode = uint16(protocol.OpcodeSMSG_PLAY_OBJECT_SOUND)
		payload = buildPlayObjectSound(sound, tgt.guid)
	} else {
		opcode = uint16(protocol.OpcodeSMSG_PLAY_SOUND)
		packet := protocol.NewBuffer(4)
		packet.WriteU32(sound)
		payload = packet.Bytes()
	}
	if tgt.isPlayer {
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || sess.player == nil {
			return
		}
		if onlySelf {
			_ = sess.write(opcode, payload, true)
			return
		}
		s.broadcastToVisibleSetFromPhase(opcode, payload, sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z, motion.PhaseMask)
		return
	}
	tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
	if tm == nil {
		return
	}
	s.broadcastToVisibleSetFromPhase(opcode, payload, tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask)
}

// smartActionSound is SMART_ACTION_SOUND (SmartScript.cpp:346-361):
// param1 sound, param2 onlySelf, param3 distance (1 = distance sound).
// The send rides smartPlaySound; the distance leg is SMSG_PLAY_OBJECT_SOUND
// with the target GUID, not the plain SMSG_PLAY_SOUND the old code sent.
func (s *Server) smartActionSound(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		s.smartPlaySound(motion, tgt, ev.row.actionParam1, ev.row.actionParam2 != 0, ev.row.actionParam3 == 1)
	}
}

// smartActionRemoveAuras is SMART_ACTION_REMOVEAURASFROMSPELL
// (SmartScript.cpp:882-911): param1 spell (0 removes all auras), param2
// charges (0 removes the aura outright), param3 onlyOwnedAuras. TDB
// 26091: all 38 AGGRO rows carry charges 0 and onlyOwnedAuras 0, so the
// plain remove legs are the live ones; the charge leg rides
// ModCharges(-charges, AURA_REMOVE_BY_EXPIRE) via modCreatureAuraStacks
// (creature) / repeated stack pops (player), and the spell-0 leg sweeps
// every live aura id. An onlyOwnedAuras row would need a caster-filtered
// removal twin Go does not have — it falls back to unfiltered, documented.
func (s *Server) smartActionRemoveAuras(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	spellID := ev.row.actionParam1
	charges := ev.row.actionParam2
	for _, tgt := range targets {
		if tgt.isPlayer {
			sess := s.findSessionByGUID(tgt.guid)
			if sess == nil {
				continue
			}
			if spellID == 0 {
				for {
					id := sess.smartFirstActiveAuraSpellID()
					if id == 0 {
						break
					}
					for !sess.removePlayerAuraFromStack(id) {
					}
				}
				continue
			}
			if charges > 0 {
				for i := uint32(0); i < charges; i++ {
					if sess.removePlayerAuraFromStack(spellID) {
						break
					}
				}
				continue
			}
			for !sess.removePlayerAuraFromStack(spellID) {
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		key := creatureAuraKey{Map: tm.Map, InstanceID: tm.InstanceID, GUID: tm.GUID}
		if spellID == 0 {
			s.auraMu.Lock()
			ids := make(map[uint32]struct{})
			for id := range s.activeCreatureAuras[key] {
				ids[id] = struct{}{}
			}
			for id := range s.creatureAuras[key] {
				ids[id] = struct{}{}
			}
			s.auraMu.Unlock()
			for id := range ids {
				s.removeCreatureAura(key, id)
			}
			continue
		}
		if charges > 0 {
			s.modCreatureAuraStacks(key, spellID, -int32(charges), auraRemoveByExpire, 0)
			continue
		}
		s.removeCreatureAura(key, spellID)
	}
}

// smartActionSetInstData is SMART_ACTION_SET_INST_DATA
// (SmartScript.cpp:998-1036): param1 field, param2 data, param3 type
// (0 = SetData, 1 = SetBossState). All 9 AGGRO rows are type 1 with data
// IN_PROGRESS (1), riding setInstanceBossState over the live instance
// boss-state store. Type 0 stays out: Go has no instance SetData store —
// instanceDataObject.SetData only fires a script event and stores
// nothing (instance_hooks.go:39-47).
func (s *Server) smartActionSetInstData(ctx context.Context, motion *creatureMotion, ev *smartScriptEvent) {
	if ev.row.actionParam3 != 1 {
		return
	}
	if !dungeonMapID(s, motion.Map) && !raidMapID(s, motion.Map) {
		return
	}
	s.setInstanceBossState(ctx, motion.Map, motion.InstanceID, ev.row.actionParam1, uint8(ev.row.actionParam2), 0, false)
}

// smartActionSetData is SMART_ACTION_SET_DATA (SmartScript.cpp:1173-1195):
// SmartAI targets fire their own SMART_EVENT_DATA_SET (SmartAI::SetData,
// SmartAI.cpp:718-721); any other creature target latches the pair into
// its ScriptData store (the map Lua creature SetData/GetData reads) —
// a Go-side latch with no C++ twin, since base CreatureAI::SetData is a
// no-op and only SmartAI overrides it. TDB 26091: 19 AGGRO rows, and none
// of their (field, value) pairs matches a DATA_SET event in the table, so
// the live effect is the latch.
func (s *Server) smartActionSetData(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	for _, tgt := range targets {
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if tm.AIName == "SmartAI" {
			s.smartProcessEvents(ctx, tm, smartEventDataSet, invokerGUID, ev.row.actionParam1, ev.row.actionParam2)
			continue
		}
		s.motionMu.Lock()
		if tm.ScriptData == nil {
			tm.ScriptData = make(map[uint32]uint32)
		}
		tm.ScriptData[ev.row.actionParam1] = ev.row.actionParam2
		s.motionMu.Unlock()
	}
}

// smartActionUpdateTemplate is SMART_ACTION_UPDATE_TEMPLATE
// (SmartScript.cpp:1055-1061): Creature::UpdateEntry(entry, nullptr,
// updateLevel != 0) on every creature target — the seatCreatureEntry
// twin. TDB 26091: 76 rows, all updateLevel=0 (5357->14604,
// 5736->5742/5739/5741/5743, 8519-8521->13279), so the level roll
// stays out and the creature keeps its level; the template's model,
// faction, flags and stats still re-seat.
func (s *Server) smartActionUpdateTemplate(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.seatCreatureEntry(ctx, tm, ev.row.actionParam1, ev.row.actionParam2 != 0)
	}
}

// smartActionMorphToEntryOrModel is SMART_ACTION_MORPH_TO_ENTRY_OR_MODEL
// (SmartScript.cpp:383-417): Creature::SetDisplayId on every creature
// target — the entry leg takes the template's ChooseDisplayId
// (ObjectMgr.cpp:1581-1593, the smartChooseDisplayID twin); a zero
// entry with a nonzero model sets the model id directly; both zero
// demorphs (DeMorph restores the native display). A missing template
// skips the target outright. A zero ChooseDisplayId (template with no
// valid model) is skipped like the Mount twin's guard — no TDB 26091
// row hits it (all 34 morph entries carry a model). TDB 26091: 140
// rows (37 entry, 99 model, 4 demorph).
func (s *Server) smartActionMorphToEntryOrModel(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if ev.row.actionParam1 != 0 {
			displayID, ok := s.smartChooseDisplayID(ctx, ev.row.actionParam1)
			if !ok || displayID == 0 {
				continue
			}
			s.setMotionDisplayID(ctx, tm, displayID)
		} else if ev.row.actionParam2 != 0 {
			s.setMotionDisplayID(ctx, tm, ev.row.actionParam2)
		} else {
			s.motionMu.Lock()
			native := tm.NativeDisplayID
			s.motionMu.Unlock()
			if native != 0 {
				s.setMotionDisplayID(ctx, tm, native)
			}
		}
	}
}

// smartActionRandomEmote is SMART_ACTION_RANDOM_EMOTE
// (SmartScript.cpp:475-491): one random emote per target unit, drawn
// uniformly from the nonzero params 1-6 — the PLAY_EMOTE broadcast
// (SMSG_EMOTE to the per-target visible set) with a per-target pick.
// All-zero rows would select from an empty vector in C++; TDB 26091
// has none (min 1 nonzero of 121 rows), so the empty guard is
// defensive only.
func (s *Server) smartActionRandomEmote(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	emotes := make([]uint32, 0, 6)
	for _, e := range [6]uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6} {
		if e != 0 {
			emotes = append(emotes, e)
		}
	}
	if len(emotes) == 0 {
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
		if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid); tm != nil {
			s.motionMu.Lock()
			mapID, instanceID, x, y, z, phase = tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask
			s.motionMu.Unlock()
		} else if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.player != nil {
			mapID, instanceID, x, y, z = sess.player.Map, sess.player.InstanceID, sess.player.X, sess.player.Y, sess.player.Z
			phase = sess.currentPlayerPhaseMask()
		}
		packet := protocol.NewBuffer(12)
		packet.WriteU32(emotes[rand.Intn(len(emotes))])
		packet.WriteU64(tgt.guid)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_EMOTE), packet.Bytes(), mapID, instanceID, x, y, z, phase)
	}
}

// smartThreatPct is the THREAT_PCT value (SmartScript.cpp:493-519):
// threatINC (param1) minus threatDEC (param2), floored at -100 —
// the ModifyThreatByPercent percent, applied as a 0.01*(100+pct)
// scale (ThreatManager.h:143; the Lua ModifyThreatByPercent bridge).
func smartThreatPct(inc, dec uint32) int32 {
	pct := int32(inc) - int32(dec)
	if pct < -100 {
		pct = -100
	}
	return pct
}

// smartActionFailQuest is SMART_ACTION_FAIL_QUEST
// (SmartScript.cpp:420-431): every player target fails the quest
// (param1) via the Player::FailQuest twin (session.failQuest):
// INCOMPLETE-only (or COMPLETE timed with COMPLETED_AT_START), FAILED
// status, log-slot timer 1 + QUEST_STATE_FAIL,
// SMSG_QUESTGIVER_QUEST_FAILED, quest-item destruction.
func (s *Server) smartActionFailQuest(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	questID := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		if sess := s.findSessionByGUID(tgt.guid); sess != nil {
			sess.failQuest(ctx, questID)
		}
	}
}

// smartActionOfferQuest is SMART_ACTION_OFFER_QUEST
// (SmartScript.cpp:433-463): every player target is offered the quest
// (param1). With directAdd (param2) the quest is added straight via
// AddQuestAndCheckCompletion (Player.cpp:15103: AddQuest, then
// CompleteQuest when the objectives are already done — questGiver nil,
// so no OnQuestAccept leg runs); without it the giver shows the
// quest-details window when the player can take the quest
// (CanTakeQuest, msg=true — the Go accept-flow twin is canAddQuest).
// The template gate is the GetQuestTemplate null check. The one live
// row (quest 5209) uses directAdd, so the menu leg is unexercised.
func (s *Server) smartActionOfferQuest(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	questID, directAdd := ev.row.actionParam1, ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || !sess.questTemplateGate(ctx, questID) {
			continue
		}
		if directAdd == 0 {
			if !sess.canAddQuest(ctx, questID, true) {
				continue
			}
			data, err := sess.loadQuestDetailData(ctx, questID)
			if err != nil {
				continue
			}
			sess.sendQuestGiverQuestDetails(ctx, motion.GUID, data)
			continue
		}
		// AddQuestAndCheckCompletion's completion check already rides
		// inside addQuestToPlayer's tail (Player.cpp:15103 via
		// AddQuest:15284/15290 + the immediate-complete arm), so no
		// second canCompleteQuest/completeQuest call here — it would
		// send a duplicate quest update and fire the status-change
		// hook twice.
		sess.addQuestToPlayer(ctx, questID, 0)
	}
}

// smartVehicleKitForTarget resolves the vehicle kit on a target unit,
// the luaVehicleRef.resolve twin (lua_vehicle.go): the same-map key
// first, then the GUID scan for a base whose kit is keyed on a stale
// map/instance (a player base that far-teleported).
func (s *Server) smartVehicleKitForTarget(motion *creatureMotion, guid uint64) *VehicleKit {
	if motion == nil || guid == 0 {
		return nil
	}
	if kit := s.getVehicleKit(motion.Map, motion.InstanceID, guid); kit != nil {
		return kit
	}
	for _, loc := range s.findVehicleKits(guid) {
		return loc.kit
	}
	return nil
}

// smartActionCallAreaExploredOrEventHappens is
// SMART_ACTION_CALL_AREAEXPLOREDOREVENTHAPPENS (SmartScript.cpp:522-542):
// every player target takes the AreaExploredOrEventHappens credit
// (param1) via the session twin (quests.go); a unit target that IS a
// vehicle credits each seated player passenger instead (GetVehicleKit
// + seat scan, :527-531) — both arms fire for a player vehicle base.
// Gameobject targets take neither arm (C++'s IsUnit gate). TDB 26091:
// 187 live rows.
func (s *Server) smartActionCallAreaExploredOrEventHappens(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	questID := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			if kit := s.smartVehicleKitForTarget(motion, tgt.guid); kit != nil {
				for _, passengerGUID := range kit.seatedPassengerGUIDs() {
					if sess := s.findSessionByGUID(passengerGUID); sess != nil {
						sess.areaExploredOrEventHappens(ctx, questID)
					}
				}
			}
		}
		if !tgt.isPlayer {
			continue
		}
		if sess := s.findSessionByGUID(tgt.guid); sess != nil {
			sess.areaExploredOrEventHappens(ctx, questID)
		}
	}
}

// smartInvokerPlayerSession is
// WorldObject::GetCharmerOrOwnerPlayerOrPlayerItself (Object.cpp:2226-2233)
// for a SmartScript invoker: the invoker itself when it is a player
// session, else the charmer-or-owner player of an invoker creature (one
// level — GetCharmerOrOwnerGUID, Unit.h:1170), else nil.
func (s *Server) smartInvokerPlayerSession(motion *creatureMotion, invokerGUID uint64) *session {
	if sess := s.findSessionByGUID(invokerGUID); sess != nil {
		return sess
	}
	if motion == nil {
		return nil
	}
	tm := s.findCreatureMotion(motion.Map, motion.InstanceID, invokerGUID)
	if tm == nil {
		return nil
	}
	if ownerGUID := creatureCharmerOrOwnerGUID(tm); ownerGUID != 0 {
		return s.findSessionByGUID(ownerGUID)
	}
	return nil
}

// smartActionCallGroupEventHappens is SMART_ACTION_CALL_GROUPEVENTHAPPENS
// (SmartScript.cpp:852-872): the invoker's controlling player takes the
// group credit (param1) via the session groupEventHappens twin
// (quests.go) — GetCharmerOrOwnerPlayerOrPlayerItself, so a pet/charm
// invoker credits its owner — anchored on the base object (the motion)
// for the group reward distance test; a vehicle-riding invoker credits
// each seated passenger the same way (:864-867). The C++ `unit` is the
// raw ProcessAction param, so a missing invoker no-ops (:854-855) —
// timer-fired rows pass nullptr (SmartScript.cpp:2900, UpdateTimer's
// else ProcessEvent(e)) while timed-list rows carry the stored list
// invoker (:3563-3564, Go's st.timedListInvoker); there is deliberately
// NO lastInvoker fallback here, a stale invoker would credit the wrong
// player. TDB 26091: 15 rows, all on DEATH (6) / LINK (61) /
// GOSSIP_SELECT (62) with a real invoker.
func (s *Server) smartActionCallGroupEventHappens(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if invokerGUID == 0 {
		return
	}
	questID := ev.row.actionParam1
	base := s.luaMotionCreature(motion)
	credit := func(sess *session) {
		if sess != nil {
			sess.groupEventHappens(ctx, questID, base)
		}
	}
	credit(s.smartInvokerPlayerSession(motion, invokerGUID))
	if kit := s.smartVehicleKitForTarget(motion, invokerGUID); kit != nil {
		for _, passengerGUID := range kit.seatedPassengerGUIDs() {
			credit(s.findSessionByGUID(passengerGUID))
		}
	}
}

// smartActionCombatStop is SMART_ACTION_COMBAT_STOP
// (SmartScript.cpp:873-881): me->CombatStop(true) (Unit.cpp:5809-5826),
// the Lua CombatStop bridge's twin (lua_creature_events.go:3846-3895):
// interrupt the non-melee cast, AttackStop (visual target clear, stop
// packet, assistance latch reset), RemoveAllAttackers via the shared
// removeCreatureAttackers twin, then the mutualPvP=true default leg
// (Unit.h:823): ClearInCombat (Unit.h:1052) -> EndAllCombat
// (CombatManager.h:126) -> EndAllPvECombat (CombatManager.cpp:344-351):
// RemoveMeFromThreatLists + ClearAllThreat + InCombat=false. Threat is
// NOT kept - the "mutualPvP=false / threat untouched" reading was wrong.
// The pending AssistDelayEvent dies with the engagement (evade
// precedent); no health restore / home walk / aura clear - those are
// EnterEvadeMode legs, not CombatStop's. TDB 26091: 13 rows, all
// SELF-targeted.
func (s *Server) smartActionCombatStop(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	interrupted := s.interruptCreatureNonMeleeCasts(motion)
	if len(interrupted) == 0 {
		motion.CastingUntil = time.Time{}
		s.cancelPendingCreatureCast(motion)
	}
	s.motionMu.Lock()
	victim := motion.TargetGUID
	motion.TargetGUID = 0
	motion.AlreadyCallAssistance = false
	motion.AssistAt = time.Time{}
	motion.AssistVictim = 0
	motion.Assistants = nil
	motion.InCombat = false
	if motion.ThreatMgr != nil {
		motion.ThreatMgr.ClearThreat()
	}
	mapID, instanceID, x, y, z, phase := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.PhaseMask
	s.motionMu.Unlock()
	for _, arm := range interrupted {
		s.sendCreatureInterruptPackets(motion, arm)
	}
	if victim != 0 {
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), buildAttackStop(motion.GUID, victim, false), mapID, instanceID, x, y, z, phase)
	}
	s.clearCreatureVisualTarget(motion)
	s.removeCreatureAttackers(mapID, instanceID, motion.GUID)
	s.removeThreatVictimFromAllLists(mapID, instanceID, motion.GUID)
	s.broadcastThreatClearInInstance(mapID, instanceID, x, y, z, motion.GUID, phase)
}

// smartActionSetIngamePhaseMask is SMART_ACTION_SET_INGAME_PHASE_MASK
// (SmartScript.cpp:1122-1131): every target unit takes
// Unit::SetPhaseMask(mask, true), every target gameobject
// WorldObject::SetPhaseMask(mask, true) (Object.cpp:3480). The
// creature leg is the motion PhaseMask + Known write (the Lua
// SetPhaseMask bridge twin, lua_creature_events.go:4476); the
// player leg is the .modify phase override slot (miscSetPhaseMask);
// the GO leg is setGameObjectPhaseMask (the materialize-then-write
// twin of setGameObjectLootState). Not carried, same as the Lua
// bridge: the controlled-unit / summon-slot propagation
// (Unit.cpp:12474-12485), the single-target-aura strip, and the
// UpdateObjectVisibility fan-out (Go keeps no persistent per-player
// visibility set to refresh; phase-filtered broadcasts read the
// stored mask). TDB 26091: 42 rows, 36 live (src0+9); masks 1/2/3/
// 8/64/256, ap2-6 all 0; targets SELF (35), CREATURE_RANGE (3),
// CREATURE_GUID (2), CLOSEST_GAMEOBJECT (1, GO-sourced),
// ACTION_INVOKER (1). The 6 src1 GO-source rows stay residual
// (GO smart scripts do not attach); the ev54 JUST_SUMMONED rows
// (2) and the ev28 PASSENGER_REMOVED row (1) never fire until
// those dispatches exist (same family as the KILL / GOSSIP_SELECT
// gaps).
func (s *Server) smartActionSetIngamePhaseMask(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	mask := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			s.setGameObjectPhaseMask(motion.Map, motion.InstanceID, tgt.guid, mask)
			continue
		}
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess != nil {
				miscSetPhaseMask(sess, mask)
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.PhaseMask = mask
		tm.PhaseMaskKnown = true
		s.motionMu.Unlock()
	}
}

// smartActionAttackStop is SMART_ACTION_ATTACK_STOP
// (SmartScript.cpp:1196-1202): Unit::AttackStop() per target unit —
// the Lua AttackStop bridge twin (lua_creature_events.go:3759):
// the !m_attacking early-out (no target, no-op), else the victim
// is dropped (TargetGUID = 0), AlreadyCallAssistance reset, and
// SMSG_ATTACK_STOP + the visual-target clear broadcast. Threat,
// InCombat and the attackers list are untouched — that teardown
// is Unit::CombatStop (action 27), not AttackStop. The
// InterruptSpell(CURRENT_MELEE_SPELL) leg is vacuous in Go (melee
// swings carry no cast state). Player targets take the session
// twin (attackTarget = 0 + sendAttackStop). TDB 26091: 8 rows, all
// SELF, no params.
func (s *Server) smartActionAttackStop(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			continue
		}
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.attackTarget != 0 {
				victim := sess.attackTarget
				sess.attackTarget = 0
				_ = sess.sendAttackStop(victim, false)
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		victim := tm.TargetGUID
		if victim == 0 {
			s.motionMu.Unlock()
			continue
		}
		tm.TargetGUID = 0
		tm.AlreadyCallAssistance = false
		mapID, instanceID, x, y, z, phase := tm.Map, tm.InstanceID, tm.X, tm.Y, tm.Z, tm.PhaseMask
		s.motionMu.Unlock()
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_ATTACK_STOP), buildAttackStop(tm.GUID, victim, false), mapID, instanceID, x, y, z, phase)
		s.clearCreatureVisualTarget(tm)
	}
}

// smartActionKillUnit is SMART_ACTION_KILL_UNIT
// (SmartScript.cpp:1297-1307): target->ToUnit()->KillSelf() —
// Unit::Kill(this, this, durabilityLoss = true) (Unit.h:941).
// The creature leg is creatureInstaKillVictim with the victim as
// its own killer (the DIE 37 twin, same DEATH-dispatch position);
// the player leg zeroes health then killPlayer with the victim
// session as killer and durabilityLoss true. TDB 26091: 47 rows,
// 35 live (src0+9); targets SELF (11), CREATURE_RANGE (4),
// CREATURE_GUID (11), CREATURE_DISTANCE (1), CLOSEST_CREATURE
// (18), ACTION_INVOKER (2); no params. The 6 src2 areatrigger
// rows and 6 src1 GO rows stay residual (those sources do not
// attach); the ev70/ev64 GO-event rows ride them.
func (s *Server) smartActionKillUnit(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			continue
		}
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.player != nil {
				sess.player.Health = 0
				sess.killPlayer(ctx, sess, false, true)
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.creatureInstaKillVictim(ctx, tm, tgt.guid)
	}
}

// smartActionSummonGO is SMART_ACTION_SUMMON_GO
// (SmartScript.cpp:1278-1296): WorldObject::SummonGameObject per
// target — the position is the target position with the row's
// target_x/y/z/o applied as a facing-frame offset
// (Position::RelocateOffset, Position.cpp:36-42), the rotation
// from the resulting orientation (QuaternionData::fromEulerAnglesZYX);
// a POSITION target summons once at the row's absolute target
// position. Param mapping (SmartScriptMgr.h:509: GameObjectID,
// DespawnTime in s): param1 entry, param2 despawn seconds,
// param3 GOSummonType (0 TIMED_OR_CORPSE_DESPAWN, 1 TIMED_DESPAWN).
// The spawn rides the Lua SummonGameObject funnel
// (lua_creature_events.go:4854): template lookup (a missing
// template logs and returns nil in C++, Object.cpp:2071-2075),
// the dynamic-GO allocator, the summoner's phase mask, the timed
// despawn (time.AfterFunc, the gobject-add-temp precedent),
// OwnerGUID on the dyn state. The summonType-0 corpse-despawn leg
// has no Go twin (no summoner-death GO sweep) — both types take
// the same timed path. TDB 26091: 226 rows, 174 live (src0+9);
// targets POSITION (154), SELF (18), CLOSEST_GAMEOBJECT (2);
// summonType 0 on 172 live rows, 1 on 2; despawn 3-30000s. The
// 52 src1 GO-source rows stay residual (GO smart scripts do not
// attach); the ev54 JUST_SUMMONED rows (2) never fire until that
// dispatch exists (same family as the KILL / GOSSIP_SELECT gaps).
func (s *Server) smartActionSummonGO(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	tmpl, err := gobjectTemplateByEntry(ctx, s.WorldStore.DB, ev.row.actionParam1)
	if err != nil || tmpl == nil {
		return
	}
	summonAt := func(x, y, z, o float32) {
		s.motionMu.Lock()
		mapID, instanceID, ownerGUID := motion.Map, motion.InstanceID, motion.GUID
		phaseMask := motion.PhaseMask
		s.motionMu.Unlock()
		lowGUID := s.nextDynamicGameObjectLowGUID()
		newGUID := gameObjectGUID(lowGUID, ev.row.actionParam1)
		size := tmpl.size
		if size <= 0 {
			size = 1.0
		}
		dyn := &dynamicGameObjectState{GUID: newGUID, LowGUID: lowGUID, Entry: ev.row.actionParam1, OwnerGUID: ownerGUID, Map: mapID, InstanceID: instanceID, X: x, Y: y, Z: z, Orientation: o, State: GameObjectStateReady, Type: tmpl.goType, DisplayID: tmpl.displayID, Size: size, ParentRotation: [4]float32{0, 0, 0, 1}, PhaseMask: phaseMask, IsRuntimeSpawn: true}
		if ev.row.actionParam2 > 0 {
			dyn.DespawnTimer = time.AfterFunc(time.Duration(ev.row.actionParam2)*time.Second, func() {
				s.despawnDynamicGameObjectInInstance(dyn.Map, dyn.InstanceID, dyn.GUID)
			})
		}
		s.spawnDynamicGameObject(dyn)
	}
	if ev.row.targetType == smartTargetPosition {
		summonAt(ev.row.targetX, ev.row.targetY, ev.row.targetZ, ev.row.targetO)
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		x, y, z, o, ok := s.smartTargetPosition(ctx, motion, tgt)
		if !ok {
			continue
		}
		ox, oy, oz, oo := ev.row.targetX, ev.row.targetY, ev.row.targetZ, ev.row.targetO
		cosO, sinO := math.Cos(float64(o)), math.Sin(float64(o))
		summonAt(x+ox*float32(cosO)-oy*float32(sinO), y+oy*float32(cosO)+ox*float32(sinO), z+oz, o+oo)
	}
}

// smartActionActivateTaxi is SMART_ACTION_ACTIVATE_TAXI
// (SmartScript.cpp:1811-1817): every player target takes
// Player::ActivateTaxiPathTo(taxi.id) — the path-id overload,
// the Lua StartTaxi twin (lua_player.go:1210), with no
// flightmaster interact or taximask gates. TDB 26091: 1 row
// (18417 Altruis, GOSSIP_SELECT, path 532, ACTION_INVOKER).
// The row cannot fire until the GOSSIP_SELECT (62) dispatch
// exists (same residual as the OFFER_QUEST row, 23:17 flag 1);
// the arm is wired so it fires when that dispatch lands.
func (s *Server) smartActionActivateTaxi(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		if sess := s.findSessionByGUID(tgt.guid); sess != nil {
			sess.startTaxiByPathID(ctx, ev.row.actionParam1)
		}
	}
}

// smartActionFollow is SMART_ACTION_FOLLOW (SmartScript.cpp:915-936):
// empty targets stop the follow without completing; otherwise the
// first unit target is followed at dist + 0.1 (param1, 0 = default)
// with angle param2 — above 6 it is degrees, converted to radians
// (:928-929) — arrival entry param3, credit param4, creditType
// param5 (0 monsterkill, 1 event). The IsSmart gate is structural:
// the dispatch only runs for smart-script creatures. Targets that
// no longer resolve drop out of the list first (the GetTargets /
// ObjectGuidVector::UpdateObjects sanitize, SmartScriptMgr.h:1592);
// an all-dropped list stops the follow, while a resolvable list with
// no unit in it (GO-only) falls through without stopping, exactly
// like the C++ loop. TDB 26091: 114 live rows; angles 315/45/90/180/
// 270/360 exercise the degrees leg, 6 rows carry a nonzero arrival
// entry, 2 a credit, 4 a creditType of 1.
func (s *Server) smartActionFollow(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	var targets []smartTarget
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if s.smartTargetResolvesInWorld(motion, tgt) {
			targets = append(targets, tgt)
		}
	}
	if len(targets) == 0 {
		s.smartStopFollow(ctx, motion, st, false)
		return
	}
	for _, tgt := range targets {
		if tgt.isGameObject {
			continue
		}
		dist := float32(ev.row.actionParam1) + 0.1
		angle := float32(ev.row.actionParam2)
		if angle > 6 {
			angle = angle * float32(math.Pi) / 180
		}
		s.smartSetFollow(ctx, motion, st, tgt.guid, dist, angle, ev.row.actionParam4, ev.row.actionParam3, ev.row.actionParam5)
		return
	}
}

// smartTargetResolvesInWorld is the GetTargets sanitize for a
// resolved target (SmartScript.cpp GetTargets + ObjectGuidVector::
// UpdateObjects, SmartScriptMgr.h:1592): a target counts only when
// it resolves to an object in the world. A dead-but-present creature
// still resolves — C++ has no alive gate here (FindCreatureNear
// tests guid and entry only; FollowMovementGenerator only requires
// the target IsInWorld). GO targets always resolve: Go's GO arms
// only return existing gameobjects.
func (s *Server) smartTargetResolvesInWorld(motion *creatureMotion, tgt smartTarget) bool {
	if tgt.isGameObject {
		return true
	}
	if tgt.isPlayer {
		sess := s.findSessionByGUID(tgt.guid)
		return sess != nil && sess.player != nil
	}
	return s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid) != nil
}

// smartSetFollow is SmartAI::SetFollow (SmartAI.cpp:822-840): the
// follow fields latch (arrival timer 1000ms) and MoveFollow arms
// the motion's follow slot, clearing the chase slot first — the
// Lua MoveFollow bridge's twin. A zero target stops without
// completing, like the C++ null-target arm. A self target latches
// the fields but arms no slot: MotionMaster::MoveFollow no-ops on
// self (MotionMaster.cpp:584-592). SetRun(_run) is vacuous:
// _run defaults true (SmartAI.cpp:34) and the Go follow step always
// runs (creatureSplineVelocityExplicit with walk=false); SET_RUN
// (59) is unported, so _run can never be false here.
func (s *Server) smartSetFollow(ctx context.Context, motion *creatureMotion, st *smartScriptState, targetGUID uint64, dist, angle float32, credit, entry, creditType uint32) {
	if targetGUID == 0 {
		s.smartStopFollow(ctx, motion, st, false)
		return
	}
	st.followGUID = targetGUID
	st.followDist = dist
	st.followAngle = angle
	st.followArrivedTimer = 1000
	st.followCredit = credit
	st.followArrivedEntry = entry
	st.followCreditType = creditType
	if targetGUID == motion.GUID {
		return
	}
	s.motionMu.Lock()
	motion.ScriptChaseTargetGUID = 0
	motion.ScriptChaseDist = 0
	motion.ScriptChaseAngle = 0
	motion.ScriptFollowTargetGUID = targetGUID
	motion.ScriptFollowDist = dist
	motion.ScriptFollowAngle = angle
	motion.ScriptFollowInformed = false
	s.motionMu.Unlock()
}

// smartStopFollow is SmartAI::StopFollow (SmartAI.cpp:841-870): the
// follow fields clear and the follow slot drops (the MoveIdle twin).
// On completion the despawn tail runs — SetDespawnTime(5000) +
// StartDespawn (SmartAI.h:205-213, UpdateDespawn :989-1004):
// invisible at +5s, removed at +10s — then FOLLOW_COMPLETED fires
// with a null invoker. The credit legs never run in this C++
// revision: _followGUID is cleared at the top of StopFollow, before
// the GetPlayer lookup, so the player is always null — ported
// verbatim, no credit is granted here.
func (s *Server) smartStopFollow(ctx context.Context, motion *creatureMotion, st *smartScriptState, complete bool) {
	st.followGUID = 0
	st.followDist = 0
	st.followAngle = 0
	st.followCredit = 0
	st.followArrivedTimer = 1000
	st.followArrivedEntry = 0
	st.followCreditType = 0
	s.motionMu.Lock()
	motion.ScriptFollowTargetGUID = 0
	motion.ScriptFollowDist = 0
	motion.ScriptFollowAngle = 0
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	s.motionMu.Unlock()
	if !complete {
		return
	}
	time.AfterFunc(5*time.Second, func() {
		if tm := s.findCreatureMotion(mapID, instanceID, guid); tm != nil {
			s.motionMu.Lock()
			tm.ScriptHidden = true
			s.motionMu.Unlock()
			s.broadcastCreatureDestroyToPlayers(mapID, instanceID, guid)
		}
		time.AfterFunc(5*time.Second, func() {
			s.despawnCreatureMotion(context.Background(), mapID, instanceID, guid)
		})
	})
	s.smartProcessEvents(ctx, motion, smartEventFollowCompleted, 0, 0, 0)
}

// smartUpdateFollow is SmartAI::UpdateFollow (SmartAI.cpp:967-981),
// ticked from smartUpdateTimers: every 1000ms, a live creature of
// the arrival entry within INTERACTION_DISTANCE (5.0) completes the
// follow. Entry 0 can never match — C++ requires an exact entry
// match (GridNotifiers.h:1337) — so the scan is skipped then, which
// is the 108 zero-entry rows' behavior. The scan itself is the
// CLOSEST_CREATURE twin (alive-only, self excluded, same map and
// instance). Deviation: C++ ticks UpdateFollow through evade;
// Go's smartUpdateTimers returns early while Evading, so the
// arrival check pauses during an evade-home walk.
// loadSmartWaypoints is the SmartWaypointMgr::LoadSmartWaypoints twin
// (SmartScriptMgr.cpp): the `waypoints`-table rows for one path id,
// ordered by pointid — the WORLD_SEL_SMARTAI_WP statement's rows
// filtered to the entry — falling back to waypoint_data (loadWaypoints)
// when the `waypoints` table is empty or missing (see below). A
// missing table in both places, or an empty path in both, loads
// nothing, matching C++'s failed GetPath.
func (s *Server) loadSmartWaypoints(ctx context.Context, entry uint32) []waypointPoint {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || entry == 0 {
		return nil
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, `SELECT pointid, position_x, position_y, position_z, orientation, delay FROM waypoints WHERE entry = ? ORDER BY pointid`, entry)
	if err != nil {
		// A missing `waypoints` table is a query error, not an empty
		// result — fall through to the waypoint_data fallback below
		// instead of returning nil (fold flag 1).
		return s.loadWaypoints(ctx, entry)
	}
	defer rows.Close()
	var points []waypointPoint
	for rows.Next() {
		var p waypointPoint
		var x, y, z, orientation float64
		var point, delay int64
		if err := rows.Scan(&point, &x, &y, &z, &orientation, &delay); err != nil {
			continue
		}
		p.Point, p.X, p.Y, p.Z, p.Orientation, p.Delay = uint32(point), float32(x), float32(y), float32(z), float32(orientation), uint32(delay)
		points = append(points, p)
	}
	if len(points) > 0 {
		return points
	}
	// TDB 26091 carries no `waypoints` table (no CREATE TABLE /
	// INSERT in the extract; the repo placeholder world.db has the
	// table with 0 rows), while all 761 WP_START pathIDs resolve in
	// waypoint_data — the C++ snapshot's SmartWaypointMgr predates the
	// TDB's move of SmartAI paths there. Without this fallback
	// LoadPath could never load a path against the actual data, and
	// every WP_START would no-op.
	return s.loadWaypoints(ctx, entry)
}

// smartLoadPath is SmartAI::LoadPath (SmartAI.cpp:78-101): the escort
// path for entry loads from the `waypoints` table; a load while
// escorting fails, and an empty path clears the script path id and
// fails. Nodes take the run/walk move type from the current _run
// (SetRun re-applies it to a loaded path). Coordinate normalization
// (Trinity::NormalizeMapCoord) has no Go twin — the DB rows load raw.
func (s *Server) smartLoadPath(ctx context.Context, st *smartScriptState, entry uint32) bool {
	if st == nil || st.escortState&smartEscortEscorting != 0 {
		return false
	}
	points := s.loadSmartWaypoints(ctx, entry)
	if len(points) == 0 {
		st.escortPathID = 0
		return false
	}
	moveType := uint32(0)
	if st.escortRun {
		moveType = 1
	}
	for i := range points {
		points[i].MoveType = moveType
	}
	st.escortPathID = entry
	st.escortPath = points
	return true
}

// smartSetRun is SmartAI::SetRun (SmartAI.cpp:730-736): the _run flag
// plus the move type on every loaded escort node, live motion points
// included, plus me->SetWalk(!run) on the persistent Walking flag
// (creaturemotion.go:708, the Creature::SetWalk twin).
func (s *Server) smartSetRun(motion *creatureMotion, st *smartScriptState, run bool) {
	if st == nil {
		return
	}
	st.escortRun = run
	moveType := uint32(0)
	if run {
		moveType = 1
	}
	for i := range st.escortPath {
		st.escortPath[i].MoveType = moveType
	}
	if motion == nil {
		return
	}
	s.motionMu.Lock()
	motion.Walking = !run
	if motion.PathID == st.escortPathID {
		for i := range motion.Points {
			motion.Points[i].MoveType = moveType
		}
	}
	s.motionMu.Unlock()
}

// smartSetDespawnTime is SmartAI::SetDespawnTime (SmartAI.h:205-209):
// the despawn timer in ms plus the state (1 = armed, 0 = none).
func (s *Server) smartSetDespawnTime(st *smartScriptState, t uint32) {
	if st == nil {
		return
	}
	st.escortDespawnTime = t
	if t != 0 {
		st.escortDespawnState = 1
	} else {
		st.escortDespawnState = 0
	}
}

// smartStartDespawn is SmartAI::StartDespawn (SmartAI.h:210-213).
func (s *Server) smartStartDespawn(st *smartScriptState) {
	if st == nil {
		return
	}
	st.escortDespawnState = 2
}

// smartStartPath is SmartAI::StartPath (SmartAI.cpp:45-76): an
// escorting creature stops its current path first; a nonzero path id
// loads (failure returns), a zero id reuses the loaded path, and an
// empty path returns. The escort restarts at node 1, repeats per the
// flag, and replaces the escort state outright (not AddEscortState).
// A player invoker stashes the creature's NPC flags and clears them
// (restored by EndPath). The path installs on the motion's waypoint
// slot with the startCreaturePath 1s hold; the pre-escort patrol slot
// is banked in the state (see the escortPath field note). The invoker
// check runs on the C++ ctor's 1000ms cadence.
func (s *Server) smartStartPath(ctx context.Context, motion *creatureMotion, st *smartScriptState, run bool, pathID uint32, repeat bool, invokerGUID uint64) {
	if motion == nil || st == nil {
		return
	}
	if st.escortState&smartEscortEscorting != 0 {
		s.smartStopPath(ctx, motion, st, 0, 0, false)
	}
	s.smartSetRun(motion, st, run)
	if pathID != 0 {
		if !s.smartLoadPath(ctx, st, pathID) {
			return
		}
	}
	if len(st.escortPath) == 0 {
		return
	}
	st.escortNode = 1
	st.escortPathEnded = false
	st.escortRepeat = repeat
	st.escortState = smartEscortEscorting
	st.escortInvokerCheckTimer = 1000
	if invokerGUID != 0 && s.smartIsPlayerGUID(invokerGUID) {
		s.motionMu.Lock()
		st.escortNPCFlags = motion.NPCFlags
		motion.NPCFlags = 0
		motion.NPCFlagsKnown = true
		mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCFlags: 0})
	}
	points := make([]waypointPoint, len(st.escortPath))
	copy(points, st.escortPath)
	s.motionMu.Lock()
	if motion.MoveType != 2 || motion.PathID != st.escortPathID {
		st.savedPathID, st.savedPoints, st.savedNextIdx, st.savedOneShot = motion.PathID, motion.Points, motion.NextIdx, motion.PathOneShot
	}
	motion.PathID = st.escortPathID
	motion.Points = points
	motion.NextIdx = 0
	motion.PathOneShot = !repeat
	motion.PathStarted = false
	motion.MoveType = 2
	motion.WaitUntil = time.Now().Add(time.Second)
	s.motionMu.Unlock()
}

// smartPausePath is SmartAI::PausePath (SmartAI.cpp:103-136). Off an
// escort it is a timed motion hold (me->PauseMovement(delay, ...)):
// the motion waits out the delay, and a live waypoint generator fires
// WAYPOINT_PAUSED with the current waypoint info. On an escort an
// already-paused path ignores the call (C++ logs it); otherwise the
// pause timer is set and a forced pause holds the motion (the
// motion.EscortPaused launch gate is the PauseMovement() twin),
// refreshes the run flag and re-homes the creature, while an
// unforced pause waits for the next node arrival to latch
// (_waypointReached). Either way the PAUSED state sets and
// WAYPOINT_PAUSED fires with the current node and path id.
func (s *Server) smartPausePath(ctx context.Context, motion *creatureMotion, st *smartScriptState, delay uint32, forced bool) {
	if motion == nil || st == nil {
		return
	}
	if st.escortState&smartEscortEscorting == 0 {
		s.motionMu.Lock()
		motion.WaitUntil = time.Now().Add(time.Duration(delay) * time.Millisecond)
		waypoint := motion.MoveType == 2 && len(motion.Points) > 0
		node, pathID := motion.LastWaypointID, motion.PathID
		s.motionMu.Unlock()
		if waypoint {
			s.smartProcessEvents(ctx, motion, smartEventWaypointPaused, 0, node, pathID)
		}
		return
	}
	if st.escortState&smartEscortPaused != 0 {
		return
	}
	st.escortPauseTimer = delay
	if forced {
		st.escortPauseForced = true
		s.smartSetRun(motion, st, st.escortRun)
		s.motionMu.Lock()
		motion.EscortPaused = true
		motion.HomeX, motion.HomeY, motion.HomeZ = motion.X, motion.Y, motion.Z
		motion.HomeO = motion.Orientation
		s.motionMu.Unlock()
	} else {
		st.escortWaypointReached = false
	}
	st.escortState |= smartEscortPaused
	s.smartProcessEvents(ctx, motion, smartEventWaypointPaused, 0, st.escortNode, st.escortPathID)
}

// smartStopPath is SmartAI::StopPath (SmartAI.cpp:149-184). Off an
// escort it idles the motion (MoveIdle), fires WAYPOINT_STOPPED with
// the current waypoint info when on a waypoint generator, then
// WAYPOINT_ENDED on a clean stop and starts the despawn when armed.
// On an escort it keeps the escort quest id when none is passed,
// arms the despawn time (a zero time clears the armed state — the
// C++ StopPath() default, so a natural path end never despawns),
// idles the motion, fires WAYPOINT_STOPPED and runs EndPath.
func (s *Server) smartStopPath(ctx context.Context, motion *creatureMotion, st *smartScriptState, despawnTime, quest uint32, fail bool) {
	if motion == nil || st == nil {
		return
	}
	if st.escortState&smartEscortEscorting == 0 {
		s.motionMu.Lock()
		waypoint := motion.MoveType == 2 && len(motion.Points) > 0
		node, pathID := motion.LastWaypointID, motion.PathID
		motion.MoveType = 0
		motion.Points = nil
		motion.NextIdx = 0
		motion.PathStarted = false
		s.motionMu.Unlock()
		if st.escortDespawnState != 2 {
			s.smartSetDespawnTime(st, despawnTime)
		}
		if waypoint && node != 0 {
			s.smartProcessEvents(ctx, motion, smartEventWaypointStopped, 0, node, pathID)
		}
		if !fail {
			if waypoint && node != 0 {
				s.smartProcessEvents(ctx, motion, smartEventWaypointEnded, 0, node, pathID)
			}
			if st.escortDespawnState == 1 {
				s.smartStartDespawn(st)
			}
		}
		return
	}
	if quest != 0 {
		st.escortQuestID = quest
	}
	if st.escortDespawnState != 2 {
		s.smartSetDespawnTime(st, despawnTime)
	}
	s.motionMu.Lock()
	motion.MoveType = 0
	motion.Points = nil
	motion.NextIdx = 0
	motion.PathStarted = false
	s.motionMu.Unlock()
	s.smartProcessEvents(ctx, motion, smartEventWaypointStopped, 0, st.escortNode, st.escortPathID)
	s.smartEndPath(ctx, motion, st, fail)
}

// smartResumePath is SmartAI::ResumePath (SmartAI.cpp:260-273): the
// WAYPOINT_RESUMED event fires, the PAUSED state clears, the pause
// latch and timer clear, the run flag refreshes and the motion
// resumes. No SMART_ACTION_WP_RESUME (65) dispatch yet — this is the
// pause-timer tick's resume arm.
func (s *Server) smartResumePath(ctx context.Context, motion *creatureMotion, st *smartScriptState) {
	if motion == nil || st == nil {
		return
	}
	s.smartProcessEvents(ctx, motion, smartEventWaypointResumed, 0, st.escortNode, st.escortPathID)
	st.escortState &^= smartEscortPaused
	st.escortPauseForced = false
	st.escortWaypointReached = false
	st.escortPauseTimer = 0
	s.smartSetRun(motion, st, st.escortRun)
	s.motionMu.Lock()
	motion.EscortPaused = false
	s.motionMu.Unlock()
}

// smartEscortRewardInRange is the Player::IsAtGroupRewardDistance
// gate (Player.cpp:24160-24173) the EndPath quest legs share: same
// map and instance, corpse-anchored, dungeons always, else within
// MaxGroupXPDistance of the creature.
func (s *Server) smartEscortRewardInRange(motion *creatureMotion, sess *session) bool {
	if motion == nil || sess == nil || sess.player == nil {
		return false
	}
	if sess.player.Map != motion.Map || sess.player.InstanceID != motion.InstanceID {
		return false
	}
	if sess.isDeadOrGhost() && sess.player.hasCorpse {
		return false
	}
	return dungeonMapID(s, motion.Map) || raidMapID(s, motion.Map) || distance3D(sess.player.X, sess.player.Y, sess.player.Z, motion.X, motion.Y, motion.Z) <= s.Config.MaxGroupXPDistance
}

// smartEndPath is SmartAI::EndPath (SmartAI.cpp:186-247): the escort
// states clear, the loaded path drops, the pause timer clears, the
// stashed NPC flags restore, and the patrol path id re-seats (the
// creature idles — the motion slot is empty — until an evade/respawn
// leg reloads it). A stored escort target takes the quest credit
// (GroupEventHappens, or FailQuest on failure, fanned out to the
// group like smartActionCallGroupEventHappens); the multi-target leg
// credits each stored player in reward range instead. A failed stop
// returns before WAYPOINT_ENDED; a clean stop fires it, then a
// repeating path restarts (when still AI-controlled) or the script
// path id clears, and an armed despawn starts.
func (s *Server) smartEndPath(ctx context.Context, motion *creatureMotion, st *smartScriptState, fail bool) {
	if motion == nil || st == nil {
		return
	}
	st.escortState &^= smartEscortEscorting | smartEscortPaused | smartEscortReturning
	st.escortPath = nil
	st.escortPauseTimer = 0
	restoredFlags := uint32(0)
	restored := false
	s.motionMu.Lock()
	if st.escortNPCFlags != 0 {
		motion.NPCFlags = st.escortNPCFlags
		motion.NPCFlagsKnown = true
		restoredFlags = st.escortNPCFlags
		st.escortNPCFlags = 0
		restored = true
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	motion.PathID = st.savedPathID
	motion.EscortPaused = false
	st.savedPathID, st.savedPoints, st.savedNextIdx, st.savedOneShot = 0, nil, 0, false
	s.motionMu.Unlock()
	if restored {
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCFlags: restoredFlags})
	}
	targets := st.storedTargets[smartEscortTargets]
	questID := st.escortQuestID
	if len(targets) > 0 && questID != 0 {
		if len(targets) == 1 && s.smartIsPlayerGUID(targets[0]) {
			if sess := s.findSessionByGUID(targets[0]); sess != nil {
				if fail {
					sess.failQuest(ctx, questID)
					if sess.groupID != 0 {
						for _, m := range s.getGroupSessions(sess.groupID) {
							if m == nil || m == sess || m.player == nil {
								continue
							}
							if m.player.Map == motion.Map && m.player.InstanceID == motion.InstanceID {
								m.failQuest(ctx, questID)
							}
						}
					}
				} else if s.smartEscortRewardInRange(motion, sess) {
					// C++ gates the GroupEventHappens call itself
					// on the caller's reward distance + corpse;
					// Go's groupEventHappens only gates the group
					// members, so the caller gate sits here.
					sess.groupEventHappens(ctx, questID, s.luaMotionCreature(motion))
				}
			}
		} else {
			for _, guid := range targets {
				if !s.smartIsPlayerGUID(guid) {
					continue
				}
				sess := s.findSessionByGUID(guid)
				if sess == nil || sess.player == nil {
					continue
				}
				if fail {
					sess.failQuest(ctx, questID)
					continue
				}
				if s.smartEscortRewardInRange(motion, sess) {
					sess.areaExploredOrEventHappens(ctx, questID)
				}
			}
		}
	}
	if fail {
		return
	}
	node, pathID := st.escortNode, st.escortPathID
	s.smartProcessEvents(ctx, motion, smartEventWaypointEnded, 0, node, pathID)
	if st.escortRepeat {
		if motion.CharmerGUID == 0 {
			s.smartStartPath(ctx, motion, st, st.escortRun, pathID, st.escortRepeat, 0)
		}
	} else {
		st.escortPathID = 0
	}
	if st.escortDespawnState == 1 {
		s.smartStartDespawn(st)
	}
}

// smartEscortInvokerInRange is SmartAI::IsEscortInvokerInRange
// (SmartAI.cpp:313-349): no stored player invoker means no check; a
// single stored player (or any group member) in range keeps the
// escort alive; a multi-target list needs any stored player in range.
// The check distance doubles on instance maps.
func (s *Server) smartEscortInvokerInRange(motion *creatureMotion, st *smartScriptState) bool {
	if st == nil {
		return true
	}
	targets := st.storedTargets[smartEscortTargets]
	if len(targets) == 0 {
		return true
	}
	checkDist := smartEscortMaxPlayerDist
	if dungeonMapID(s, motion.Map) || raidMapID(s, motion.Map) {
		checkDist *= 2
	}
	inRange := func(guid uint64) bool {
		sess := s.findSessionByGUID(guid)
		if sess == nil || sess.player == nil {
			return false
		}
		return sess.player.Map == motion.Map && sess.player.InstanceID == motion.InstanceID && distance3D(sess.player.X, sess.player.Y, sess.player.Z, motion.X, motion.Y, motion.Z) <= float64(checkDist)
	}
	if len(targets) == 1 && s.smartIsPlayerGUID(targets[0]) {
		if inRange(targets[0]) {
			return true
		}
		if sess := s.findSessionByGUID(targets[0]); sess != nil && sess.groupID != 0 {
			_, party, raid, _ := groupMembershipSnapshot(s, sess.groupID, targets[0])
			for guid := range party {
				if inRange(guid) {
					return true
				}
			}
			for guid := range raid {
				if inRange(guid) {
					return true
				}
			}
		}
		return false
	}
	for _, guid := range targets {
		if s.smartIsPlayerGUID(guid) && inRange(guid) {
			return true
		}
	}
	return false
}

// smartWaypointReached is SmartAI::WaypointReached (SmartAI.cpp:359-394)
// plus the MovementInform arm (SmartAI.cpp:396-410), fired from
// waypointNodeArrival: MOVEMENTINFORM (waypoint type, node index) then
// WAYPOINT_REACHED (DB point id, path id) fire for every SmartAI
// creature arriving at a waypoint node (patrol or escort). On an escort
// path the current node latches first; a pending unforced pause latches
// _waypointReached and holds the motion (PauseMovement +
// SetHomePosition), else a reached last node ends the path — C++
// compares the node id against the node COUNT (SmartAI.cpp:377-380),
// so only contiguous 1-based point ids ever end the path this way,
// mirrored literally. A forced pause holds the motion across the
// arrival: the arrival's node-delay write would otherwise release it.
func (s *Server) smartWaypointReached(ctx context.Context, motion *creatureMotion, nodeID, pathID, nodeIndex uint32) {
	if motion == nil {
		return
	}
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return
	}
	escorting := st.escortState&smartEscortEscorting != 0 && motion.PathID == st.escortPathID && len(st.escortPath) > 0
	if escorting {
		st.escortNode = nodeID
	}
	// WaypointMovementGenerator::OnArrived (WaypointMovementGenerator.cpp:256-260)
	// informs the AI before the reached hook. POINT_MOTION_TYPE rows
	// (script MovePoint landings) fire from the script-move finalize
	// site instead (smartMovementInform).
	s.smartProcessEvents(ctx, motion, smartEventMovementInform, 0, waypointMotionType, nodeIndex)
	s.smartProcessEvents(ctx, motion, smartEventWaypointReached, 0, nodeID, pathID)
	if !escorting {
		return
	}
	if st.escortPauseTimer != 0 && !st.escortPauseForced {
		st.escortWaypointReached = true
		s.motionMu.Lock()
		motion.EscortPaused = true
		motion.HomeX, motion.HomeY, motion.HomeZ = motion.X, motion.Y, motion.Z
		motion.HomeO = motion.Orientation
		s.motionMu.Unlock()
	} else if nodeID == uint32(len(st.escortPath)) {
		st.escortPathEnded = true
	}
	if st.escortState&smartEscortPaused != 0 && st.escortPauseForced {
		s.motionMu.Lock()
		motion.EscortPaused = true
		s.motionMu.Unlock()
	}
}

// smartMovementInform is SmartAI::MovementInform (SmartAI.cpp:396-410):
// a POINT + SMART_ESCORT_LAST_OOC_POINT (0xFFFFFF) arrival clears the
// evade state and, while escorting, latches _OOCReached; every arrival
// fires SMART_EVENT_MOVEMENTINFORM (34) with (type, id) through the
// (type, id) gate. Called from the script-spline finalize site in
// stepCreatureMotion, next to the Lua MovementInform fire — the
// POINT_MOTION_TYPE rows the waypoint arrival used to call out as
// missing. The UpdatePath RETURNING leg that consumes _OOCReached
// (SmartAI.cpp:955-963) is ported in smartUpdatePath; the
// ReturnToLastOOCPos launch that sets RETURNING (SmartAI.cpp:281)
// has no twin yet.
func (s *Server) smartMovementInform(ctx context.Context, motion *creatureMotion, moveType, pointID uint32) {
	if motion == nil {
		return
	}
	if moveType == movementGeneratorPoint && pointID == smartEscortLastOOCPoint {
		s.motionMu.Lock()
		motion.Evading = false
		s.motionMu.Unlock()
	}
	s.smartProcessEvents(ctx, motion, smartEventMovementInform, 0, moveType, pointID)
	if moveType == movementGeneratorPoint && pointID == smartEscortLastOOCPoint {
		if st := s.smartStateFor(ctx, motion); st != nil && st.escortState&smartEscortEscorting != 0 {
			st.escortOOCReached = true
		}
	}
}

// smartWaypointPathEnded is SmartAI::WaypointPathEnded (SmartAI.cpp:387-394),
// fired from the motion's one-shot path-end leg: WAYPOINT_ENDED fires
// for a non-escorting creature with the last node's id and path id.
// An escorting creature ends through UpdatePath's path-ended leg
// instead (SmartAI.cpp:377-380 + 948-951), so this returns early
// while escorting.
func (s *Server) smartWaypointPathEnded(ctx context.Context, motion *creatureMotion, nodeID, pathID uint32) {
	if motion == nil {
		return
	}
	st := s.smartStateFor(ctx, motion)
	if st == nil || st.escortState&smartEscortEscorting != 0 {
		return
	}
	s.smartProcessEvents(ctx, motion, smartEventWaypointEnded, 0, nodeID, pathID)
}

// smartUpdatePath is SmartAI::UpdatePath (SmartAI.cpp:919-963): the
// invoker range check runs every second — out of range stops the path
// as failed, fires DEATH (the out-of-range despawn hook) and despawns
// — then the pause leg (resume only off a set timer, out of combat,
// not returning) and the path-ended leg (StopPath with defaults), then
// the RETURNING / _OOCReached leg (clear RETURNING, resume unless
// paused). The vehicle-conditions leg and the ReturnToLastOOCPos
// launch stay out: no Go vehicle-conditions engine, and nothing sets
// RETURNING yet (the launch is SmartAI.cpp:281).
func (s *Server) smartUpdatePath(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if st == nil || motion == nil || st.escortState&smartEscortEscorting == 0 {
		return
	}
	if st.escortInvokerCheckTimer < diff {
		if !s.smartEscortInvokerInRange(motion, st) {
			s.smartStopPath(ctx, motion, st, 0, st.escortQuestID, true)
			s.smartProcessEvents(ctx, motion, smartEventDeath, motion.GUID, 0, 0)
			s.despawnCreatureMotion(ctx, motion.Map, motion.InstanceID, motion.GUID)
			return
		}
		st.escortInvokerCheckTimer = 1000
	} else {
		st.escortInvokerCheckTimer -= diff
	}
	if st.escortState&smartEscortPaused != 0 && (st.escortWaypointReached || st.escortPauseForced) {
		if st.escortPauseTimer != 0 && !motion.InCombat && st.escortState&smartEscortReturning == 0 {
			if st.escortPauseTimer <= diff {
				s.smartResumePath(ctx, motion, st)
			} else {
				st.escortPauseTimer -= diff
			}
		}
	} else if st.escortPathEnded {
		st.escortPathEnded = false
		s.smartStopPath(ctx, motion, st, 0, 0, false)
		return
	}
	if st.escortState&smartEscortReturning != 0 && st.escortOOCReached {
		st.escortOOCReached = false
		st.escortState &^= smartEscortReturning
		if st.escortState&smartEscortPaused == 0 {
			s.smartResumePath(ctx, motion, st)
		}
	}
}

// smartUpdateDespawn is SmartAI::UpdateDespawn (SmartAI.cpp:987-1002):
// state 1 (armed) never ticks — StartDespawn moves it to 2 — state 2
// hides the creature after the despawn time, then 5s later state 3
// despawns it. The hide leg uses motion.ScriptHidden (visible to GM
// seers only); C++ SetVisible(false) hides from everyone.
func (s *Server) smartUpdateDespawn(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if st == nil || motion == nil || st.escortDespawnState <= 1 || st.escortDespawnState > 3 {
		return
	}
	if st.escortDespawnTime < diff {
		if st.escortDespawnState == 2 {
			s.motionMu.Lock()
			motion.ScriptHidden = true
			s.motionMu.Unlock()
			st.escortDespawnTime = 5000
			st.escortDespawnState++
		} else {
			s.despawnCreatureMotion(ctx, motion.Map, motion.InstanceID, motion.GUID)
		}
	} else {
		st.escortDespawnTime -= diff
	}
}

// smartActionWPStart is SMART_ACTION_WP_START (SmartScript.cpp:1409-1435):
// run/pathID/repeat/quest/despawnTime/reactState are action params
// 1-6. Any player among the action targets banks the WHOLE target list
// under SMART_ESCORT_TARGETS; the react state, path start (invoker =
// the event invoker, for the player NPC-flags stash), escort quest
// and despawn time all act on the script owner. TDB 26091: 930 live
// rows.
func (s *Server) smartActionWPStart(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	for _, tgt := range targets {
		if tgt.isPlayer {
			if st.storedTargets == nil {
				st.storedTargets = make(map[uint32][]uint64)
			}
			guids := make([]uint64, 0, len(targets))
			for _, t := range targets {
				guids = append(guids, t.guid)
			}
			st.storedTargets[smartEscortTargets] = guids
			break
		}
	}
	s.motionMu.Lock()
	motion.ReactState = uint8(ev.row.actionParam6)
	motion.ReactStateKnown = true
	s.motionMu.Unlock()
	s.smartStartPath(ctx, motion, st, ev.row.actionParam1 != 0, ev.row.actionParam2, ev.row.actionParam3 != 0, invokerGUID)
	st.escortQuestID = ev.row.actionParam4
	s.smartSetDespawnTime(st, ev.row.actionParam5)
}

// smartActionWPPause is SMART_ACTION_WP_PAUSE (SmartScript.cpp:1436-1444):
// the delay is action param 1, always forced (PausePath(delay, true)).
// TDB 26091: 674 live rows.
func (s *Server) smartActionWPPause(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	s.smartPausePath(ctx, motion, st, ev.row.actionParam1, true)
}

// smartActionWPStop is SMART_ACTION_WP_STOP (SmartScript.cpp:1445-1455):
// despawnTime/quest/fail are action params 1-3. TDB 26091: 12 live
// rows.
func (s *Server) smartActionWPStop(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	s.smartStopPath(ctx, motion, st, ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3 != 0)
}

// smartActionWPResume is SMART_ACTION_WP_RESUME (SmartScript.cpp:1456-1465):
// no params. CanResumePath (SmartAI.cpp:138-146) is escorting AND paused;
// the 1ms pause timer resumes on the next UpdatePath tick through
// smartResumePath (SmartAI.cpp:941-948). The evade-pause delta carries:
// smartUpdateTimers returns early while Evading, so the resume waits out
// the evade-home walk. TDB 26091: 40 live rows, all target SELF.
// (Targets are ignored by C++.)
func (s *Server) smartActionWPResume(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if st == nil || st.escortState&smartEscortEscorting == 0 || st.escortState&smartEscortPaused == 0 {
		return
	}
	st.escortPauseTimer = 1
}

func (s *Server) smartUpdateFollow(ctx context.Context, motion *creatureMotion, st *smartScriptState, diff uint32) {
	if st.followGUID == 0 {
		return
	}
	if st.followArrivedTimer < diff {
		if st.followArrivedEntry != 0 && len(s.smartClosestCreatureTarget(motion, st.followArrivedEntry, 5, 0)) > 0 {
			s.smartStopFollow(ctx, motion, st, true)
			return
		}
		st.followArrivedTimer = 1000
	} else {
		st.followArrivedTimer -= diff
	}
}

// smartActionAddItem is SMART_ACTION_ADD_ITEM (SmartScript.cpp:1313-1322):
// every player target takes AddItem(entry, count) (Player.cpp:26543:
// CanStoreNewItem with the partial-fit leg, StoreNewItem, SendNewItem).
// The Go twin is the .additem command's storeOrStackItem +
// sendAddItemPushResult + inventory refresh; the no-space sysmessage
// leg stays out (SmartAI actions send no sysmessages). TDB 26091: 0 rows.
func (s *Server) smartActionAddItem(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	entry, count := ev.row.actionParam1, ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || sess.player == nil {
			continue
		}
		res, err := sess.storeOrStackItem(ctx, tgt.guid, entry, count)
		if err != nil || res == nil {
			fitting := sess.addItemFittingCount(ctx, tgt.guid, entry, count)
			if fitting == 0 {
				continue
			}
			res, err = sess.storeOrStackItem(ctx, tgt.guid, entry, fitting)
			if err != nil || res == nil {
				continue
			}
			count = fitting
		}
		sess.sendAddItemPushResult(sess, res, entry, count)
		_ = sess.sendInventoryItems(ctx)
		sess.sendPlayerUpdate()
	}
}

// smartActionRemoveItem is SMART_ACTION_REMOVE_ITEM
// (SmartScript.cpp:1324-1333): every player target takes
// DestroyItemCount(entry, count, true). The Go twin is the
// .additem negative-count leg's destroyPlayerItemCount plus the
// command's inventory refresh. TDB 26091: 8 rows, all target 7
// (ACTION_INVOKER) on QUEST_ACCEPTED (19) / QUEST_COMPLETED (20) /
// GOSSIP_SELECT (62) events; the gossip row stays unfireable until
// the GOSSIP_SELECT dispatch exists (the OFFER_QUEST residual).
func (s *Server) smartActionRemoveItem(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	entry, count := ev.row.actionParam1, ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil {
			continue
		}
		if destroyed := sess.destroyPlayerItemCount(ctx, tgt.guid, entry, count); destroyed > 0 {
			_ = sess.sendInventoryItems(ctx)
			sess.sendPlayerUpdate()
		}
	}
}

// SMARTAI_TEMPLATE ids (SmartScriptMgr.h:1208-1216).
const (
	smartAITemplateBasic        uint32 = 0
	smartAITemplateCaster       uint32 = 1
	smartAITemplateTurret       uint32 = 2
	smartAITemplatePassive      uint32 = 3
	smartAITemplateCagedGOPart  uint32 = 4
	smartAITemplateCagedNPCPart uint32 = 5
)

// smartInstallTemplateEvent is SmartScript::AddEvent
// (SmartScript.cpp:2473-2476) + CreateSmartEvent (:2478-2508): the
// holder takes chance 100, source CREATURE, the caller's event/action/
// target params and phase mask, InitTimer rolls its timer, and it
// joins mEvents — Go appends to st.events directly (InstallEvents,
// :3604-3613, merges mInstallEvents on the next update; the append
// lands after the live range loop, so it cannot fire mid-dispatch).
// SMART_ACTION_MOVE_OFFSET (114) has no Go dispatch, so it is passed
// as the literal id rather than a smartAction const — the census
// counts named consts as ported, and 114 is not.
func (s *Server) smartInstallTemplateEvent(st *smartScriptState, eventType, eventFlags, ep1, ep2, ep3, ep4, ep5, actionType, ap1, ap2, ap3, ap4, ap5, ap6, targetType, tp1, tp2, tp3, tp4, phaseMask uint32) {
	row := &smartScriptRow{
		entryOrGUID:    0,
		eventType:      eventType,
		eventPhaseMask: phaseMask,
		eventChance:    100,
		eventFlags:     eventFlags,
		eventParam1:    ep1,
		eventParam2:    ep2,
		eventParam3:    ep3,
		eventParam4:    ep4,
		eventParam5:    ep5,
		actionType:     actionType,
		actionParam1:   ap1,
		actionParam2:   ap2,
		actionParam3:   ap3,
		actionParam4:   ap4,
		actionParam5:   ap5,
		actionParam6:   ap6,
		targetType:     targetType,
		targetParam1:   tp1,
		targetParam2:   tp2,
		targetParam3:   tp3,
		targetParam4:   tp4,
	}
	sev := &smartScriptEvent{row: row}
	s.smartInitTimer(sev)
	st.events = append(st.events, sev)
}

// smartCreatureTextExists is sCreatureTextMgr->TextExist
// (CreatureTextMgr.h): whether the entry has any row in the group.
// It reuses broadcastCreatureTalk's lazy-load cache.
func (s *Server) smartCreatureTextExists(ctx context.Context, entry uint32, groupID uint8) bool {
	if s == nil || entry == 0 {
		return false
	}
	if s.creatureTextMgr == nil {
		s.creatureTextMgr = newCreatureTextMgr()
	}
	s.creatureTextMgr.mu.RLock()
	groups, found := s.creatureTextMgr.texts[entry]
	s.creatureTextMgr.mu.RUnlock()
	if !found {
		loaded := s.loadCreatureTexts(ctx, entry)
		s.creatureTextMgr.mu.Lock()
		if s.creatureTextMgr.texts == nil {
			s.creatureTextMgr.texts = make(map[uint32]map[uint8][]CreatureText)
		}
		s.creatureTextMgr.texts[entry] = loaded
		groups = loaded
		s.creatureTextMgr.mu.Unlock()
	}
	if groups == nil {
		return false
	}
	rows, ok := groups[groupID]
	return ok && len(rows) > 0
}

// smartActionInstallAITemplate is SMART_ACTION_INSTALL_AI_TEMPLATE
// (SmartScript.cpp:1308-1312 + InstallTemplate :2397-2471): a template
// installs at most once (mTemplate != BASIC logs and skips), then its
// AddEvent set joins the script. TDB 26091: 0 rows. Residuals: the
// installed MANA_PCT (3) / RANGE (9) events ride Go's timer-only
// arms for those types (no live power / no action fire), the
// MOVEMENTINFORM (34) events have no Go dispatch, the MOVE_OFFSET
// (114) event has no Go action, and CAGED_GO_PART returns early — a
// creature script has no GO base object (SmartScript.cpp:2460).
func (s *Server) smartActionInstallAITemplate(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if st == nil || st.templateID != smartAITemplateBasic {
		return
	}
	id := ev.row.actionParam1
	st.templateID = id
	p1, p2, p3, p4, p5 := ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6
	tp1 := ev.row.targetParam1
	switch id {
	case smartAITemplateCaster:
		s.smartInstallTemplateEvent(st, smartEventUpdateIC, 0, 0, 0, p2, p3, 0, smartActionCast, p1, tp1, 0, 0, 0, 0, smartTargetVictim, 0, 0, 0, 0, 1)
		s.smartInstallTemplateEvent(st, smartEventRange, 0, p4, 300, 0, 0, 0, smartActionAllowCombatMovement, 1, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 1)
		maxDist := uint32(0)
		if p4 > 10 {
			maxDist = p4 - 10
		}
		s.smartInstallTemplateEvent(st, smartEventRange, 0, 0, maxDist, 0, 0, 0, smartActionAllowCombatMovement, 0, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 1)
		// C++ manaMin is the literal unsigned expression
		// param5-15>100?100:param5+15 (SmartScript.cpp:2422): param5
		// < 15 underflows the subtraction, so the min is 100 there,
		// and param5 > 100 yields param5+15 unclamped — Go uint32
		// arithmetic reproduces both legs exactly.
		manaMin := p5 + 15
		if p5-15 > 100 {
			manaMin = 100
		}
		s.smartInstallTemplateEvent(st, smartEventManaPct, 0, manaMin, 100, 1000, 1000, 0, smartActionSetEventPhase, 1, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventManaPct, 0, 0, p5, 1000, 1000, 0, smartActionSetEventPhase, 0, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventManaPct, 0, 0, p5, 1000, 1000, 0, smartActionAllowCombatMovement, 1, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 0)
	case smartAITemplateTurret:
		s.smartInstallTemplateEvent(st, smartEventUpdateIC, 0, 0, 0, p2, p3, 0, smartActionCast, p1, tp1, 0, 0, 0, 0, smartTargetVictim, 0, 0, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventJustCreated, 0, 0, 0, 0, 0, 0, smartActionAllowCombatMovement, 0, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 0)
	case smartAITemplateCagedNPCPart:
		s.smartInstallTemplateEvent(st, smartEventDataSet, 0, 0, 0, 0, 0, 0, smartActionStoreTargetList, 1, 0, 0, 0, 0, 0, smartTargetClosestGameObject, p1, 10, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventUpdate, smartEventFlagNotRepeatable, 0, 0, 0, 0, 0, smartActionResetGObject, 0, 0, 0, 0, 0, 0, smartTargetGameObjectDist, p1, 5, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventDataSet, 0, 0, 0, 0, 0, 0, smartActionSetRun, p3, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventDataSet, 0, 0, 0, 0, 0, 0, smartActionSetEventPhase, 1, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 0)
		s.smartInstallTemplateEvent(st, smartEventUpdate, smartEventFlagNotRepeatable, 1000, 1000, 0, 0, 0, 114, 0, 0, 0, 0, 0, 0, smartTargetSelf, 0, p4, 0, 0, 1)
		s.smartInstallTemplateEvent(st, smartEventMovementInform, 0, 8, 0xFFFFFE, 0, 0, 0, smartActionSetData, 0, 0, 0, 0, 0, 0, smartTargetStored, 1, 0, 0, 0, 1)
		s.smartInstallTemplateEvent(st, smartEventMovementInform, 0, 8, 0xFFFFFE, 0, 0, 0, smartActionForceDespawn, p2, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 1)
		if s.smartCreatureTextExists(ctx, motion.Entry, uint8(p5)) {
			s.smartInstallTemplateEvent(st, smartEventMovementInform, 0, 8, 0xFFFFFE, 0, 0, 0, smartActionTalk, p5, 0, 0, 0, 0, 0, smartTargetNone, 0, 0, 0, 0, 1)
		}
	case smartAITemplateCagedGOPart:
		return
	default:
		return
	}
}

// smartActionSetDisableGravity is SMART_ACTION_SET_DISABLE_GRAVITY
// (SmartScript.cpp:1351-1359): me-only (IsSmart gate = this dispatch),
// SmartAI::SetDisableGravity -> Creature::SetDisableGravity. The body
// mirrors the Lua SetDisableGravity bridge (lua_creature_events.go:
// no-op-on-unchanged, GravityDisabled latch, SMSG_SPLINE_MOVE_GRAVITY_
// DISABLE/ENABLE broadcast). TDB 26091: 50 rows, all SELF, param 0/1.
func (s *Server) smartActionSetDisableGravity(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	disable := ev.row.actionParam1 != 0
	s.motionMu.Lock()
	if motion.GravityDisabled == disable {
		s.motionMu.Unlock()
		return
	}
	motion.GravityDisabled = disable
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_ENABLE
	if disable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_GRAVITY_DISABLE
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
}

// smartActionSetSwim is SMART_ACTION_SET_SWIM (SmartScript.cpp:1375-1382):
// me-only, SmartAI::SetSwim -> Unit::SetSwim. The body mirrors the Lua
// SetSwim bridge (lua_creature_events.go: no-op-on-unchanged, Swimming
// latch, SMSG_SPLINE_MOVE_START/STOP_SWIM broadcast). TDB 26091: 0 rows.
func (s *Server) smartActionSetSwim(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	enable := ev.row.actionParam1 != 0
	s.motionMu.Lock()
	if motion.Swimming == enable {
		s.motionMu.Unlock()
		return
	}
	motion.Swimming = enable
	mapID, instanceID, x, y, guid := motion.Map, motion.InstanceID, motion.X, motion.Y, motion.GUID
	phase := motion.PhaseMask
	s.motionMu.Unlock()
	opcode := protocol.OpcodeSMSG_SPLINE_MOVE_STOP_SWIM
	if enable {
		opcode = protocol.OpcodeSMSG_SPLINE_MOVE_START_SWIM
	}
	s.broadcastCreatureSplineMove(mapID, instanceID, x, y, guid, opcode, phase)
}

// smartTeleportCreature is Unit::NearTeleportTo (Unit.cpp:13332-13346)
// for a creature motion: the position write plus the teleport-movement
// broadcast, mirroring the Lua Teleport bridge (lua_creature_events.go).
func (s *Server) smartTeleportCreature(tm *creatureMotion, x, y, z, o float32) {
	s.motionMu.Lock()
	motions := s.motionMapLocked(tm.Map, tm.InstanceID)
	m, ok := motions[tm.GUID]
	if !ok {
		s.motionMu.Unlock()
		return
	}
	m.X, m.Y, m.Z, m.Orientation = x, y, z, o
	m.Moving = false
	mapID, instanceID, guid := m.Map, m.InstanceID, m.GUID
	phase := m.PhaseMask
	s.motionMu.Unlock()
	_, nearby := buildTeleportMovementPackets(guid, movementInfo{Time: s.gameTimeMilliseconds(), X: x, Y: y, Z: z, Orientation: o})
	s.broadcastCreatureTeleportMovement(mapID, instanceID, guid, x, y, z, nearby, phase)
	s.syncCreatureVehiclePassengers(m)
}

// smartActionTeleport is SMART_ACTION_TELEPORT (SmartScript.cpp:1340-1350):
// players take TeleportTo(mapID, target x/y/z/o), creatures take
// NearTeleportTo on the same coords; anything else is skipped. The
// destination rides the row's target_x/y/z/o, the map is action_param1.
// TDB 26091: 11 rows — 3 creature-source SELF rows (same-map creature
// legs) and 8 areatrigger-source rows, which stay residual (Go loads no
// areatrigger-source scripts).
func (s *Server) smartActionTeleport(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	mapID := ev.row.actionParam1
	x, y, z, o := ev.row.targetX, ev.row.targetY, ev.row.targetZ, ev.row.targetO
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess != nil {
				sess.teleportTo(mapID, x, y, z, o)
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.smartTeleportCreature(tm, x, y, z, o)
	}
}

// smartCounterValue is SmartScript::GetCounterValue
// (SmartScript.cpp:139-145): 0 when the id was never stored.
func (s *Server) smartCounterValue(st *smartScriptState, id uint32) uint32 {
	if st == nil || st.counters == nil {
		return 0
	}
	return st.counters[id]
}

// smartStoreCounter is SmartScript::StoreCounter (SmartScript.cpp:123-137):
// an existing counter adds the value (or resets to it when reset != 0),
// a new id inserts the value, then SMART_EVENT_COUNTER_SET fires for
// the id on that script (invoker nil in C++).
func (s *Server) smartStoreCounter(ctx context.Context, motion *creatureMotion, st *smartScriptState, id, value uint32, reset bool) {
	if st == nil {
		return
	}
	if st.counters == nil {
		st.counters = make(map[uint32]uint32)
	}
	if cur, ok := st.counters[id]; ok {
		if reset {
			st.counters[id] = value
		} else {
			st.counters[id] = cur + value
		}
	} else {
		st.counters[id] = value
	}
	s.smartProcessEvents(ctx, motion, smartEventCounterSet, 0, id, 0)
}

// smartActionSetCounter is SMART_ACTION_SET_COUNTER
// (SmartScript.cpp:1383-1407): with no targets the counter stores on
// the owner's script; otherwise each creature target stores on its own
// SmartAI script (CAST_AI null leg = skip, the not-using-SmartAI
// log-and-skip) and each gameobject target on its SmartGameObjectAI
// script — the GO leg stays residual, Go has no GO script engine.
// TDB 26091: 40 rows (37 src 0, 3 src 9): 13 SELF, 15 CLOSEST_CREATURE,
// 8 CLOSEST_GAMEOBJECT, 4 OWNER_OR_SUMMONER; 5 rows carry reset=1.
func (s *Server) smartActionSetCounter(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	id, value, reset := ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3 != 0
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		s.smartStoreCounter(ctx, motion, st, id, value, reset)
		return
	}
	for _, tgt := range targets {
		if tgt.isGameObject {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		tst := s.smartStateFor(ctx, tm)
		if tst == nil {
			continue
		}
		s.smartStoreCounter(ctx, tm, tst, id, value, reset)
	}
}

// smartSetFacing is the SetFacingTo half of the Lua bridge
// (lua_creature_events.go:5930): Unit::SetFacingTo (Unit.cpp:13669), an
// in-place turn — the logical orientation slot plus the in-place
// broadcastMonsterMove with the Final_Angle arm. Same mid-spline race
// delta as the Lua bridge (C++ skips the turn when the unit is moving).
func (s *Server) smartSetFacing(motion *creatureMotion, ori float32) {
	if motion == nil {
		return
	}
	s.motionMu.Lock()
	motion.Orientation = ori
	guid, mapID, instID := motion.GUID, motion.Map, motion.InstanceID
	phase := motion.PhaseMask
	transportGUID := motion.TransportGUID
	x, y, z := motion.X, motion.Y, motion.Z
	s.motionMu.Unlock()
	s.broadcastMonsterMoveInInstance(mapID, instID, guid, x, y, z, x, y, z, 1000, false, ori, true, phase, transportGUID)
}

// smartContactPoint is WorldObject::GetContactPoint (Object.cpp:3334-3338)
// + GetNearPoint2D (:3243-3270): the point at targetReach + meReach +
// dist from the target along the target-to-me angle, z pinned to the
// target's z. A creature target reads its live combat reach, a player
// target the 1.5 UNIT_FIELD_COMBATREACH default, a GO target 0
// (Object.h:313). The hover-offset correction, UpdateAllowedPositionZ
// and the LoS sweep stay out, the Lua GetContactPoint deltas.
func (s *Server) smartContactPoint(motion *creatureMotion, tgt smartTarget, tx, ty, tz float32, dist float32) (float32, float32, float32) {
	targetReach := float32(1.5)
	if tgt.isGameObject {
		targetReach = 0
	} else if !tgt.isPlayer {
		if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid); tm != nil {
			s.motionMu.Lock()
			targetReach = tm.combatReachOrDefault()
			s.motionMu.Unlock()
		}
	}
	s.motionMu.Lock()
	meReach := motion.combatReachOrDefault()
	mx, my := motion.X, motion.Y
	s.motionMu.Unlock()
	angle := math.Atan2(float64(my-ty), float64(mx-tx))
	d := float64(targetReach + meReach + dist)
	return tx + float32(d*math.Cos(angle)), ty + float32(d*math.Sin(angle)), tz
}

// smartActionSetOrientation is SMART_ACTION_SET_ORIENTATION
// (SmartScript.cpp:1466-1478): me-only facing. SELF faces the home
// orientation (the on-transport leg reads the transport home position,
// which Go does not store — a transported creature faces its spawn
// home instead), POSITION faces the row's target_o, any other target
// type faces the first resolved target (SetFacingToObject: the
// GetAbsoluteAngle twin, atan2 normalized to [0, 2pi)). TDB 26091:
// 1194 live rows.
func (s *Server) smartActionSetOrientation(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if motion == nil {
		return
	}
	if ev.row.targetType == smartTargetSelf {
		s.motionMu.Lock()
		homeO := motion.HomeO
		s.motionMu.Unlock()
		s.smartSetFacing(motion, homeO)
		return
	}
	if ev.row.targetType == smartTargetPosition {
		s.smartSetFacing(motion, ev.row.targetO)
		return
	}
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	if len(targets) == 0 {
		return
	}
	x, y, _, _, ok := s.smartTargetPosition(ctx, motion, targets[0])
	if !ok {
		return
	}
	s.motionMu.Lock()
	mx, my := motion.X, motion.Y
	s.motionMu.Unlock()
	angle := float32(math.Atan2(float64(y-my), float64(x-mx)))
	if angle < 0 {
		angle += 2 * math.Pi
	}
	s.smartSetFacing(motion, angle)
}

// smartActionPlayMovie is SMART_ACTION_PLAYMOVIE (SmartScript.cpp:1479-1489):
// every player target takes SendMovieStart(entry) (Player.cpp:6529-6535):
// SetMovie plus SMSG_TRIGGER_MOVIE, the Lua SendMovieStart twin
// (lua_player.go:8159). TDB 26091: 3 live rows, movies 14 and 16.
func (s *Server) smartActionPlayMovie(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	id := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil || sess.player == nil {
			continue
		}
		sess.player.Movie = id
		packet := protocol.NewBuffer(4)
		packet.WriteU32(id)
		_ = sess.write(uint16(protocol.OpcodeSMSG_TRIGGER_MOVIE), packet.Bytes(), true)
	}
}

// smartActionMoveToPos is SMART_ACTION_MOVE_TO_POS (SmartScript.cpp:1490-1528):
// pointId / transport / disablePathfinding / ContactDistance are action
// params 1-4. The commented-out target-type gate means every target type
// goes through the random pick; with no targets the row's target_x/y/z
// is the absolute destination. A picked target contributes its position
// (plus the row's target_x/y/z offsets); ContactDistance > 0 routes it
// through GetContactPoint first. The launch is the Lua MoveTo twin
// (movementGeneratorPoint, walk false so the persistent Walking flag
// applies, allowDead true — point splines move a corpse). The transport
// leg (param 2, CalculatePassengerPosition) has no bridge and is 0 in
// all 579 rows; disablePathfinding (param 3, 3 rows) is a no-op — Go
// has no PathGenerator, every script spline is straight-line. A
// MovePoint during an escort does not clear the escort path state (no
// MotionMaster slot replacement in Go); the arrival fires
// MovementInform(POINT_MOTION_TYPE, pointId) through the spline
// finalize leg. TDB 26091: 579 live rows.
func (s *Server) smartActionMoveToPos(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	pointID := ev.row.actionParam1
	contactDist := float32(ev.row.actionParam4)
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	var destX, destY, destZ float32
	if len(targets) == 0 {
		destX, destY, destZ = ev.row.targetX, ev.row.targetY, ev.row.targetZ
	} else {
		tgt := targets[rand.Intn(len(targets))]
		x, y, z, _, ok := s.smartTargetPosition(ctx, motion, tgt)
		if !ok {
			return
		}
		if contactDist > 0 {
			x, y, z = s.smartContactPoint(motion, tgt, x, y, z, contactDist)
		}
		destX, destY, destZ = x+ev.row.targetX, y+ev.row.targetY, z+ev.row.targetZ
	}
	s.launchCreatureScriptSpline(motion, destX, destY, destZ, false, movementGeneratorPoint, pointID, true)
}

// smartGameObjectSpawnedByDefault is GameObject::isSpawnedByDefault
// (GameObject.cpp:1081-1110): the DB spawntimesecs sign — >= 0 means
// spawned by default. A live runtime spawn reads as not spawned by
// default first (SummonGameObject sets SetSpawnedByDefault(false),
// Object.cpp:2089); a static GO with no readable row reads as spawned
// by default, matching Create's default (GameObject.cpp:127).
func (s *Server) smartGameObjectSpawnedByDefault(ctx context.Context, mapID, instanceID uint32, guid uint64) bool {
	if s == nil {
		return true
	}
	if st := s.gameObjectState(mapID, instanceID, guid); st != nil && st.IsRuntimeSpawn {
		return false
	}
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return true
	}
	low := uint32(guid & 0xFFFFFF)
	entry := uint32(guid>>24) & 0xFFFFFF
	var secs int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(spawntimesecs, 0) FROM gameobject WHERE guid = ? AND id = ? LIMIT 1", low, entry).Scan(&secs); err != nil {
		return true
	}
	return secs >= 0
}

// smartActionEnableTempGObj is SMART_ACTION_ENABLE_TEMP_GOBJ
// (SmartScript.cpp:1529-1544): every gameobject target takes
// SetRespawnTime(duration) (GameObject.cpp:1330-1336), the
// setGameObjectRespawnDelay write half. Creature targets and
// spawned-by-default GOs only earn a C++ sql.sql warning — there is
// no log channel on the smart-action path, so both legs are skips.
// The !spawnedByDefault visibility arm is a no-op for a visible
// runtime GO, the Lua SetRespawnTime delta. TDB 26091: 57 live rows.
func (s *Server) smartActionEnableTempGObj(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	duration := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			continue
		}
		if s.smartGameObjectSpawnedByDefault(ctx, motion.Map, motion.InstanceID, tgt.guid) {
			continue
		}
		s.setGameObjectRespawnDelay(motion.Map, motion.InstanceID, tgt.guid, int32(duration))
	}
}

// smartActionEquip is SMART_ACTION_EQUIP (SmartScript.cpp:1552-1588):
// entry / mask / slot1-3 are action params 1-5. A nonzero entry id
// loads that creature_equip_template row for the target's entry
// (GetEquipmentInfo, ObjectMgr.cpp:1430-1450 — id -1 picks a random
// row); a missing row logs in C++ and breaks the whole target loop.
// A zero entry id uses the slot params directly. The mask gates
// per-slot writes (mask 0 writes all three); each written slot is a
// SetUInt32Value(UNIT_VIRTUAL_ITEM_SLOT_ID + i) — the live virtual
// items plus the values-update broadcast, the Lua SetEquipmentSlots
// twin. SetCurrentEquipmentId has no Go reader (creature SaveToDB /
// game-event restore / the SpellAuraEffects equipment check do not
// exist on this path), so only the slot writes land. TDB 26091: 124
// live rows.
func (s *Server) smartActionEquip(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	equipID := int8(ev.row.actionParam1)
	mask := ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		var slots [3]uint32
		if equipID != 0 {
			items, ok := s.smartEquipmentInfo(ctx, tm.Entry, equipID)
			if !ok {
				break
			}
			slots = items
		} else {
			slots = [3]uint32{ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5}
		}
		updates := map[int]uint32{}
		s.motionMu.Lock()
		for i := 0; i < 3; i++ {
			if mask == 0 || mask&(1<<uint(i)) != 0 {
				tm.VirtualItems[i] = slots[i]
				updates[unitVirtualItemSlotID+i] = slots[i]
			}
		}
		tm.VirtualItemsKnown = true
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		if len(updates) > 0 {
			s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, updates)
		}
	}
}

// smartEquipmentInfo is ObjectMgr::GetEquipmentInfo (ObjectMgr.cpp:1430-1450):
// the creature_equip_template row for (entry, id); id -1 picks a random
// row for the entry. Missing entry or id reads as no info.
func (s *Server) smartEquipmentInfo(ctx context.Context, entry uint32, id int8) ([3]uint32, bool) {
	var items [3]uint32
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return items, false
	}
	if id == -1 {
		rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT ItemID1, ItemID2, ItemID3 FROM creature_equip_template WHERE CreatureID = ?", entry)
		if err != nil {
			return items, false
		}
		defer rows.Close()
		var all [][3]uint32
		for rows.Next() {
			var row [3]uint32
			if err := rows.Scan(&row[0], &row[1], &row[2]); err != nil {
				continue
			}
			all = append(all, row)
		}
		if len(all) == 0 {
			return items, false
		}
		return all[rand.Intn(len(all))], true
	}
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT ItemID1, ItemID2, ItemID3 FROM creature_equip_template WHERE CreatureID = ? AND ID = ?", entry, id).Scan(&items[0], &items[1], &items[2]); err != nil {
		return items, false
	}
	return items, true
}

// smartActionCloseGossip is SMART_ACTION_CLOSE_GOSSIP (SmartScript.cpp:1545-1551):
// every player target takes PlayerTalkClass::SendCloseGossip —
// SMSG_GOSSIP_COMPLETE, the sendGossipComplete twin. TDB 26091: 557
// live rows.
func (s *Server) smartActionCloseGossip(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		if sess := s.findSessionByGUID(tgt.guid); sess != nil {
			sess.sendGossipComplete()
		}
	}
}

// smartActionAddAura is SMART_ACTION_ADD_AURA (SmartScript.cpp:680-692):
// every unit target takes AddAura(spell, self) — the aura is applied
// with the target as its own caster, no cast bar and no SMSG_SPELL_GO.
// The Go twin is the Lua creature:AddAura bridge's presence store
// (lua_creature_events.go:3447): the spell id lands in creatureAuras,
// which is what HasAura / AURA_NOT_PRESENT read; the full effect
// application (amounts, periodics) rides the cast path only, so an
// AddAura'd aura has no stat/periodic legs yet. Player targets ride
// the session applyAura instead (the .aura command's self-cast leg),
// which applies the full aura at a fixed 30-minute duration rather
// than the spell's DBC duration. TDB 26091: 0 rows.
func (s *Server) smartActionAddAura(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	spell := ev.row.actionParam1
	if spell == 0 {
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			continue
		}
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess != nil && sess.player != nil {
				sess.applyAura(spell)
			}
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		key := creatureAuraKeyForMotion(tm)
		s.auraMu.Lock()
		if s.creatureAuras == nil {
			s.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
		}
		if s.creatureAuras[key] == nil {
			s.creatureAuras[key] = make(map[uint32]struct{})
		}
		s.creatureAuras[key][spell] = struct{}{}
		s.auraMu.Unlock()
	}
}

// smartActionOverrideScriptBaseObject is
// SMART_ACTION_OVERRIDE_SCRIPT_BASE_OBJECT (SmartScript.cpp:1625-1651):
// the first creature (or GO) target becomes the script's base object —
// later actions read it as `me` — and the owner's GUID is stashed on
// first override so RESET_SCRIPT_BASE_OBJECT (77) / OnReset can
// restore it. The stash is first-only: a nested override keeps the
// original owner, like C++'s `if (!meOrigGUID && me)` guard. The GO
// leg has no Go twin (Go loads no GO-source scripts and a GO carries
// no smartScriptState), so a GO first-target stops the scan with the
// owner base kept, mirroring C++'s break; player targets fall through
// to the next target, like C++'s loop. TDB 26091: 0 rows.
func (s *Server) smartActionOverrideScriptBaseObject(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if st == nil || motion == nil {
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			return
		}
		if tgt.isPlayer {
			continue
		}
		if s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid) == nil {
			continue
		}
		if st.baseOrigGUID == 0 {
			st.baseOrigGUID = motion.GUID
		}
		st.baseOverrideGUID = tgt.guid
		return
	}
}

// smartActionResetScriptBaseObject is
// SMART_ACTION_RESET_SCRIPT_BASE_OBJECT (SmartScript.cpp:1652-1654):
// ResetBaseObject restores `me`/`go` from the stashed original GUIDs
// and clears the stash. In Go the owner motion is always the dispatch
// parameter, so restoring is clearing both fields — the next action
// resolves the owner again. A lookup miss in C++ leaves the
// overridden base in place; Go never dangles, it just clears.
// TDB 26091: 0 rows.
func (s *Server) smartActionResetScriptBaseObject(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if st == nil {
		return
	}
	st.baseOverrideGUID = 0
	st.baseOrigGUID = 0
}

// smartActionCallScriptReset is SMART_ACTION_CALL_SCRIPT_RESET
// (SmartScript.cpp:1655-1658): SetPhase(0), then OnReset — timers
// re-init, runOnce clears, the SMART_EVENT_RESET events fire, the last
// invoker clears, and the base object restores (ResetBaseObject is
// OnReset's first step). The Go twin is smartSetEventPhase(0) — which
// fires PHASE_CHANGE only on a real change, like C++ SetPhase
// (SmartScript.cpp:4010-4018) — then smartReset, which clears the
// base-object override. The re-entrant RESET dispatch rides the
// smartMaxNestedEvents cap. C++'s loader rejects RESET-event rows
// carrying this action (SmartScriptMgr.cpp:1024-1028); TDB 26091
// carries none, so the rule is unexercised. TDB 26091: 50 rows, all
// target SELF, all params 0 — 34 src0 + 11 src9 + 5 GO-source (the GO
// rows stay residual: Go loads no GO-source scripts).
func (s *Server) smartActionCallScriptReset(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if st == nil || motion == nil {
		return
	}
	s.smartSetEventPhase(ctx, motion, st, 0, invokerGUID)
	s.smartReset(ctx, motion)
}

// smartActionSetRangedMovement is SMART_ACTION_SET_RANGED_MOVEMENT
// (SmartScript.cpp:1659-1680): for each target creature that IsSmart
// (AI is SmartAI) and has a victim, if CanCombatMove, MoveChase the
// victim at (distance, angle) — angle in degrees, converted to
// radians like C++'s /180*PI. The Go twin arms the ScriptChase slot
// (the Lua MoveChase bridge's shape, lua_creature_events.go:6451):
// the combat tick honors it only while the chase target is the
// victim, which holds here since the victim is passed; MoveChase
// clears the follow slot (same ACTIVE slot). The CanCombatMove gate
// is the inverted NoCombatMovement flag (the ALLOW_COMBAT_MOVEMENT
// twin) and is checked before arming, so a disabled creature never
// holds a latent chase. TDB 26091: 9 rows, all src0 SELF —
// Ravenclaw Regent (2283, 15y/0deg), Shadowmoon Acolyte (16594,
// 15y), Dawnblade Reservist (25087, 35y), Sunblade Slayer (25368,
// 60y/45deg), Captain Hartford (29490, 30y).
func (s *Server) smartActionSetRangedMovement(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	dist := float32(ev.row.actionParam1)
	angle := float32(ev.row.actionParam2) / 180.0 * float32(math.Pi)
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject || tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || tm.AIName != "SmartAI" {
			continue
		}
		s.motionMu.Lock()
		victimGUID := tm.TargetGUID
		if victimGUID != 0 && !tm.NoCombatMovement {
			tm.ScriptFollowTargetGUID = 0
			tm.ScriptFollowDist = 0
			tm.ScriptFollowAngle = 0
			tm.ScriptChaseTargetGUID = victimGUID
			tm.ScriptChaseDist = dist
			tm.ScriptChaseAngle = angle
		}
		s.motionMu.Unlock()
	}
}

// smartActionResetGObject is SMART_ACTION_RESET_GOBJECT
// (SmartScript.cpp:708-719): every gameobject target takes
// ResetDoorOrButton. Non-GO targets are skipped, like C++'s
// IsGameObject check. TDB 26091: 30 live rows (src 0/9; the 8
// GO-source rows stay residual — Go loads no GO-source scripts).
func (s *Server) smartActionResetGObject(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			continue
		}
		s.resetDoorOrButton(motion.Map, motion.InstanceID, tgt.guid)
	}
}

// smartActionCallKilledMonster is SMART_ACTION_CALL_KILLEDMONSTER
// (SmartScript.cpp:965-991): on a NONE/SELF target the creature's
// loot recipient takes the group credit (RewardPlayerAndGroupAtEvent,
// Player.cpp:24132-24158); otherwise every player target takes the
// single credit, and every unit target's vehicle seats take it too
// (the C++ IsUnit vehicle leg). The single-credit call passes no
// creature GUID (C++ calls KilledMonsterCredit with the default
// empty guid, so real_entry stays the row's entry); the group leg
// passes the recipient PLAYER as pRewardSource (SmartScript.cpp:975),
// so creature_guid is Empty too (Player.cpp:24134-24136: only a
// TYPEID_UNIT source carries a guid) and the group distance is
// measured from the recipient player, not the creature. TDB 26091:
// 410 live rows (src 0/9; 18 areatrigger + 3 GO-source rows stay
// residual).
func (s *Server) smartActionCallKilledMonster(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	entry := ev.row.actionParam1
	if ev.row.targetType == smartTargetNone || ev.row.targetType == smartTargetSelf {
		s.motionMu.Lock()
		tapPlayer := motion.TapPlayerGUID
		s.motionMu.Unlock()
		sess := s.findSessionByGUID(tapPlayer)
		if sess == nil {
			return
		}
		s.smartRewardPlayerAndGroupAtEvent(ctx, sess, entry)
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			if sess := s.findSessionByGUID(tgt.guid); sess != nil {
				sess.grantKillCreditMonster(ctx, entry, 0)
			}
			continue
		}
		if tgt.isGameObject {
			continue
		}
		if kit := s.smartVehicleKitForTarget(motion, tgt.guid); kit != nil {
			for _, passengerGUID := range kit.seatedPassengerGUIDs() {
				if sess := s.findSessionByGUID(passengerGUID); sess != nil {
					sess.grantKillCreditMonster(ctx, entry, 0)
				}
			}
		}
	}
}

// smartRewardPlayerAndGroupAtEvent is Player::RewardPlayerAndGroupAtEvent
// (Player.cpp:24132-24158) for a smart-script kill credit: solo, the
// recipient takes the credit directly; grouped, every member at
// IsAtGroupRewardDistance of the recipient player — the pRewardSource
// (SmartScript.cpp:975) — takes it (same map and instance,
// dungeon-always, else within MaxGroupXPDistance, dead members anchored
// on their corpse via groupRewardAnchorPos), but only while alive or
// not yet released (:24152). The credit carries no creature GUID
// (Player.cpp:24134-24136: only a TYPEID_UNIT source carries one),
// so real_entry stays the row's entry.
func (s *Server) smartRewardPlayerAndGroupAtEvent(ctx context.Context, sess *session, entry uint32) {
	if sess == nil || sess.player == nil {
		return
	}
	if sess.groupID == 0 || sess.server == nil {
		sess.grantKillCreditMonster(ctx, entry, 0)
		return
	}
	mapID, instanceID := sess.player.Map, sess.player.InstanceID
	cx, cy, cz := sess.player.X, sess.player.Y, sess.player.Z
	dungeon := sess.isDungeonMap(mapID)
	for _, m := range sess.server.getGroupSessions(sess.groupID) {
		if m == nil || m.player == nil {
			continue
		}
		if m.player.Map != mapID || m.player.InstanceID != instanceID {
			continue
		}
		mx, my, mz := groupRewardAnchorPos(m)
		if !dungeon && distance3D(mx, my, mz, cx, cy, cz) > sess.server.Config.MaxGroupXPDistance {
			continue
		}
		if m.isDeadOrGhost() && m.player.hasCorpse {
			continue
		}
		m.grantKillCreditMonster(ctx, entry, 0)
	}
}

// smartActionActivateGObject is SMART_ACTION_ACTIVATE_GOBJECT
// (SmartScript.cpp:693-706): every gameobject target is re-armed to
// GO_READY (loot state 1) and used via the UseDoorOrButton twin
// (delay 0 = template auto-close, alternative false). Non-GO targets
// (e.g. SELF on a creature row) are skipped, like C++'s IsGameObject
// check.
func (s *Server) smartActionActivateGObject(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			continue
		}
		s.setGameObjectLootState(motion.Map, motion.InstanceID, tgt.guid, 1)
		s.useDoorOrButton(motion.Map, motion.InstanceID, tgt.guid, false, 0)
		// GameObject::UseDoorOrButton (GameObject.cpp:1443) leaves the
		// GO GO_ACTIVATED (loot state 2); the Go twin tracks state via
		// the GO_STATE/flag broadcast only, so the loot-state tail is
		// written here to match the C++ end state.
		s.setGameObjectLootState(motion.Map, motion.InstanceID, tgt.guid, 2)
	}
}

// smartActionThreatSinglePct is SMART_ACTION_THREAT_SINGLE_PCT
// (SmartScript.cpp:506-519): ModifyThreatByPercent on every target
// unit — a victim with no threat entry no-ops (ScaleThreat's unknown
// victim leg), and pct 0 scales by 1.0, the C++ no-op. TDB 26091:
// 11 rows, all SELF-targeted via VICTIM/INVOKER.
func (s *Server) smartActionThreatSinglePct(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if motion.ThreatMgr == nil {
		return
	}
	pct := smartThreatPct(ev.row.actionParam1, ev.row.actionParam2)
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		motion.ThreatMgr.ScaleThreat(tgt.guid, float32(0.01*float64(100+pct)))
	}
}

// smartActionThreatAllPct is SMART_ACTION_THREAT_ALL_PCT
// (SmartScript.cpp:493-504): ModifyThreatByPercent on every entry in
// the owner's threat list (the EntryGUIDs snapshot — C++ iterates the
// modifiable list). TDB 26091: 4 rows.
func (s *Server) smartActionThreatAllPct(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if motion.ThreatMgr == nil {
		return
	}
	pct := smartThreatPct(ev.row.actionParam1, ev.row.actionParam2)
	for _, guid := range motion.ThreatMgr.EntryGUIDs() {
		motion.ThreatMgr.ScaleThreat(guid, float32(0.01*float64(100+pct)))
	}
}

// smartActionSetEmoteState is SMART_ACTION_SET_EMOTE_STATE
// (SmartScript.cpp:721-730): SetUInt32Value(UNIT_NPC_EMOTESTATE) on
// every target unit — the motion EmoteState slot plus the values
// broadcast, the .npc playemote arm's path. Player targets stay out:
// Go has no player-side emote-state twin on this path and no TDB
// 26091 row targets one (target types 1/0/19/10/11 only). TDB 26091:
// 646 rows, 321 of them clearing to 0.
func (s *Server) smartActionSetEmoteState(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.EmoteState = ev.row.actionParam1
		tm.EmoteStateKnown = true
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCEmoteState: ev.row.actionParam1})
	}
}

// smartActionEvade is SMART_ACTION_EVADE (SmartScript.cpp:820-835):
// when param1 (toRespawnPosition) is 0 the home position resets to
// the respawn position first, then EnterEvadeMode runs — the
// triggerCreatureEvade twin, which already fires SMART_EVENT_EVADE
// after _EnterEvadeMode (SmartAI.cpp:429) and takes the owner-follow
// leg. TDB 26091: 121 rows, param1 0 on all of them, so the reset
// always runs. The _evadeDisabled early arm (SmartAI.cpp:411-415,
// action 117) runs at the top of triggerCreatureEvade via
// motion.EvadeDisabled.
// Summons/pets/totems carry no Spawn record, so the reset is a
// no-op for them and their (undrifted) home stands.
func (s *Server) smartActionEvade(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.actionParam1 == 0 {
		s.motionMu.Lock()
		if motion.SpawnKnown {
			motion.HomeX, motion.HomeY, motion.HomeZ, motion.HomeO = motion.SpawnX, motion.SpawnY, motion.SpawnZ, motion.SpawnO
		}
		s.motionMu.Unlock()
	}
	s.triggerCreatureEvade(ctx, motion, time.Now())
}

// smartActionForceDespawn is SMART_ACTION_FORCE_DESPAWN
// (SmartScript.cpp:1104-1131): Creature::DespawnOrUnsummon(delay,
// forceRespawnTimer) on every target — the despawnCreatureMotion twin.
// C++ clamps a <=0 delay to 1ms so at least a world update tick runs
// before the despawn, keeping linked actions alive, so the despawn
// always rides a timer (ForcedDespawnDelayEvent, Creature.cpp:2219-2225;
// ForcedUnsummonDelayEvent, TemporarySummon.cpp:259-267). A nonzero
// forceRespawnTimer overrides the respawn time at expiry (the
// SaveRespawnTime leg, Creature.cpp:2258-2261). GO targets stay out:
// smartTargets yields no gameobjects.
func (s *Server) smartActionForceDespawn(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	delay := ev.row.actionParam1
	if delay == 0 {
		delay = 1
	}
	timer := ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		mapID, instanceID, guid := motion.Map, motion.InstanceID, tgt.guid
		time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
			s.despawnCreatureMotion(context.Background(), mapID, instanceID, guid)
			if timer > 0 {
				s.overrideCreatureRespawnTime(context.Background(), uint32(guid&0x00FFFFFF), mapID, instanceID, time.Now().Add(time.Duration(timer)*time.Second))
			}
		})
	}
}

// smartActionSetInvincibilityHPLevel is SMART_ACTION_SET_INVINCIBILITY_HP_LEVEL
// (SmartScript.cpp:1155-1171): SetInvincibilityHpLevel on the target
// creature's SmartAI — the CAST_AI(SmartAI) gate is the AIName check
// plus a state attach, and a nonzero percent wins over the flat minHP
// (CountPctFromMaxHealth over the target's max health). The clamp
// itself lives in fireDealDamageAIHooks (SmartAI::DamageTaken,
// SmartAI.cpp:626-636). TDB 26091: 69 rows (minHP=1 x47, pct rows x14:
// pct=1 x11, 19/49/50 x1 each; 8 rows carry 0/0, which sets level 0
// and leaves the clamp off, matching C++'s falsy-level guard).
func (s *Server) smartActionSetInvincibilityHPLevel(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || tm.AIName != "SmartAI" {
			continue
		}
		tst := s.smartStateFor(ctx, tm)
		if tst == nil {
			continue
		}
		level := ev.row.actionParam1
		if ev.row.actionParam2 != 0 {
			s.motionMu.Lock()
			maxHealth := tm.MaxHealth
			s.motionMu.Unlock()
			level = maxHealth * ev.row.actionParam2 / 100
		}
		tst.invincibilityHPLevel = level
	}
}

// smartActionMountToEntryOrModel is SMART_ACTION_MOUNT_TO_ENTRY_OR_MODEL
// (SmartScript.cpp:1133-1153): Unit::Mount on every target unit — the
// Lua Mount/Dismount twin (lua_creature_events.go:7183-7221) on the
// motion's Mount display + UNIT_FLAG_MOUNT. A nonzero creature entry
// mounts the template's ChooseDisplayId (ObjectMgr.cpp:1581-1593);
// a zero entry with a nonzero model mounts the model id directly;
// both zero dismounts. A missing template skips the target outright.
// Player targets stay out: Go has no player-side Mount twin for this
// path. TDB 26091: 42 rows (creature-entry mounts x3, model mounts
// x14, dismounts x25 — e.g. 19592/19594->19596, 24238->22657;
// 18294->14375, 22448->20964; 23030, 23580, 30703, 30704).
func (s *Server) smartActionMountToEntryOrModel(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if ev.row.actionParam1 == 0 && ev.row.actionParam2 == 0 {
			s.motionMu.Lock()
			if tm.UnitFlags&0x08000000 == 0 { // not IsMounted
				s.motionMu.Unlock()
				continue
			}
			tm.Mount = 0
			tm.UnitFlags &^= 0x08000000
			tm.UnitFlagsKnown = true
			tm.MountKnown = true
			s.motionMu.Unlock()
			continue
		}
		displayID := ev.row.actionParam2
		if ev.row.actionParam1 > 0 {
			var ok bool
			displayID, ok = s.smartChooseDisplayID(ctx, ev.row.actionParam1)
			if !ok {
				continue
			}
		}
		s.motionMu.Lock()
		if displayID != 0 {
			tm.Mount = displayID
		}
		tm.UnitFlags |= 0x08000000 // UNIT_FLAG_MOUNT
		tm.UnitFlagsKnown = true
		tm.MountKnown = true
		s.motionMu.Unlock()
	}
}

// smartChooseDisplayID is ObjectMgr::ChooseDisplayId (ObjectMgr.cpp:1581-1593)
// for a creature_template entry: a trigger template
// (CREATURE_FLAG_EXTRA_TRIGGER, 0x80) takes its first is_trigger model,
// any other template takes a uniform random valid model
// (CreatureTemplate::GetRandomValidModelId, Creature.cpp:112-125;
// GetFirstInvisibleModel, Creature.cpp:134-145). False when the template
// is missing — C++ skips the mount then — and a 0 display with true means
// the template has no valid model, which still mounts the flag
// (Unit::Mount(0), Unit.cpp:8310-8315).
func (s *Server) smartChooseDisplayID(ctx context.Context, entry uint32) (uint32, bool) {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return 0, false
	}
	var model1, model2, model3, model4, flagsExtra int64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(modelid1, 0), COALESCE(modelid2, 0), COALESCE(modelid3, 0), COALESCE(modelid4, 0), COALESCE(flags_extra, 0) FROM creature_template WHERE entry = ?", entry).Scan(&model1, &model2, &model3, &model4, &flagsExtra); err != nil {
		return 0, false
	}
	models := []int64{model1, model2, model3, model4}
	if flagsExtra&0x80 != 0 {
		for _, m := range models {
			if m == 0 {
				continue
			}
			var trigger int64
			if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(is_trigger, 0) FROM creature_model_info WHERE DisplayID = ?", m).Scan(&trigger); err == nil && trigger != 0 {
				return uint32(m), true
			}
		}
		return 0, true
	}
	var valid []uint32
	for _, m := range models {
		if m > 0 {
			valid = append(valid, uint32(m))
		}
	}
	if len(valid) == 0 {
		return 0, true
	}
	return valid[rand.Intn(len(valid))], true
}

// smartActionRespawnBySpawnID is SMART_ACTION_RESPAWN_BY_SPAWNID
// (SmartScript.cpp:2313-2326): Map::Respawn(spawnType, spawnId) —
// GetRespawnInfo, then respawnTime = now. The Go twin queues a due-now
// creature respawn record for the spawn id, which the world tick's
// processCreatureRespawns revives; a live motion for the spawn is a
// no-op, matching C++ (no RespawnInfo exists for a live spawn). The
// gameobject type stays out: Go has no GO-respawn model. TDB 26091:
// 17 rows (15 creature-type, e.g. 18471->48191, 24023->74518/74519/
// 74520; 2 gameobject-type, which stay out).
func (s *Server) smartActionRespawnBySpawnID(ctx context.Context, motion *creatureMotion, ev *smartScriptEvent) {
	if ev.row.actionParam1 != 0 {
		return
	}
	spawnID := ev.row.actionParam2
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var entry, mapID int64
	var x, y, z float64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT id, map, position_x, position_y, position_z FROM creature WHERE guid = ?", spawnID).Scan(&entry, &mapID, &x, &y, &z); err != nil {
		return
	}
	s.motionMu.Lock()
	live := s.findCreatureMotionLocked(uint32(mapID), 0, creatureWorldGUID(spawnID, uint32(entry)))
	alive := live != nil && live.Health > 0
	s.motionMu.Unlock()
	if alive {
		return
	}
	stats := s.loadCreatureStats(ctx, uint32(entry))
	now := time.Now()
	s.motionMu.Lock()
	if s.creatureRespawns == nil {
		s.creatureRespawns = make(map[uint32]creatureRespawn)
	}
	s.creatureRespawns[spawnID] = creatureRespawn{GUID: spawnID, At: now, Health: stats.MaxHealth, Entry: uint32(entry), Map: uint32(mapID), X: float32(x), Y: float32(y), Z: float32(z), CorpseRemoved: true}
	s.motionMu.Unlock()
	s.saveCreatureRespawnRow(ctx, spawnID, now, uint32(mapID), 0)
}

// smartActionRemoveUnitFieldBytes1 is SMART_ACTION_REMOVE_UNIT_FIELD_BYTES_1
// (SmartScript.cpp:1851-1856): RemoveByteFlag(UNIT_FIELD_BYTES_1, type,
// byte1) — param1 is the flag byte, param2 the byte offset. Byte 0 is the
// stand-state byte, which Go models as motion.StandState (all 12 AGGRO
// rows are offset 0, e.g. clearing SIT to stand). Bytes 1-3 (animation
// tier, shapeshift, ...) have no Go model and stay out.
func (s *Server) smartActionRemoveUnitFieldBytes1(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.actionParam2 != 0 {
		return
	}
	targets := s.smartTargets(ctx, motion, st, ev, invokerGUID)
	for _, tgt := range targets {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.StandState &^= uint8(ev.row.actionParam1)
		tm.StandStateKnown = true
		s.motionMu.Unlock()
	}
}

// smartActionRandomMove is SMART_ACTION_RANDOM_MOVE (SmartScript.cpp:1818-1843):
// param1 is the wander distance. Every creature target takes MoveRandom
// (distance) or MoveIdle (distance 0); when no creature target resolves,
// me takes it instead (the foundTarget fallback, :1836-1842). The random
// leg is the Lua MoveRandom twin (lua_objects.go): one random point within
// the distance of the creature's current position (RandomMovementGenerator
// ::DoInitialize pins _reference to the owner's position, and SetRandomLocation
// picks frand(0, distance) — the same uniform-radius pick), a single
// movementGeneratorRandom spline, walk true (the template CanRun / AlwaysRun
// walk arms have no Go model); the C++ persistent 2-10-step wander machine
// with LOS retries and PathGenerator stays out. The idle leg is the Lua
// MoveIdle twin (stopSessionCreatureMotion): halt at the current position
// with the MonsterMoveStop broadcast, no combat / target / evade clearing
// (C++ MoveIdle only swaps the default-slot generator). TDB 26091: 131
// rows (76 src-0 + 55 src-9); 35 distance-0 rows take the idle leg.
func (s *Server) smartActionRandomMove(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	distance := ev.row.actionParam1
	foundCreature := false
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer || tgt.isGameObject {
			continue
		}
		foundCreature = true
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.smartRandomMoveLeg(tm, distance)
	}
	if !foundCreature {
		s.smartRandomMoveLeg(motion, distance)
	}
}

// smartRandomMoveLeg is one target's RANDOM_MOVE arm: distance 0 halts,
// anything else launches the one-shot random point.
func (s *Server) smartRandomMoveLeg(tm *creatureMotion, distance uint32) {
	if tm == nil {
		return
	}
	if distance == 0 {
		s.motionMu.Lock()
		if !tm.Moving {
			s.motionMu.Unlock()
			return
		}
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		phase := tm.PhaseMask
		transportGUID := tm.TransportGUID
		x, y, z := tm.X, tm.Y, tm.Z
		tm.Moving = false
		s.motionMu.Unlock()
		s.broadcastMonsterMoveStopInInstance(mapID, instanceID, guid, x, y, z, phase, transportGUID)
		return
	}
	angle := rand.Float64() * 2 * math.Pi
	dist := rand.Float64() * float64(distance)
	s.motionMu.Lock()
	fromX, fromY, fromZ := tm.X, tm.Y, tm.Z
	s.motionMu.Unlock()
	s.launchCreatureScriptSpline(tm, fromX+float32(dist*math.Cos(angle)), fromY+float32(dist*math.Sin(angle)), fromZ, true, movementGeneratorRandom, 0, false)
}

// smartActionSetUnitFieldBytes1 is SMART_ACTION_SET_UNIT_FIELD_BYTES_1
// (SmartScript.cpp:1844-1850): SetByteFlag(UNIT_FIELD_BYTES_1, type, byte1)
// — param1 is the flag byte, param2 the byte offset. The flag ORs into the
// byte (Object::SetByteFlag, Object.cpp:854-864); the Go twin is the same
// OR onto motion.StandState for offset 0 (the Lua SetStandState stamp,
// spawn-overlay applied). Live rows are near-uniformly offset 0: KNEEL 8
// (210), SUBMERGED 9 (7), SIT 1 (25), SLEEP 3 (12), DEAD 7 (37) and the
// small values 2/4/5; the uint8 truncation is the fidelity — the Bjorn
// Halgurdsson hover row's param1 33554432 truncates to 0, a C++ no-op, and
// the param1-0 row is likewise a no-op. The single offset-3 row (Razormaw,
// anim-tier bit 2) stays out: Go has no stored anim-tier byte. Player
// targets stay out on the REMOVE_UNIT_FIELD_BYTES_1 precedent (no live
// player-targeted rows). TDB 26091: 297 rows (89 src-0 + 208 src-9).
func (s *Server) smartActionSetUnitFieldBytes1(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.actionParam2 != 0 {
		return
	}
	flag := uint8(ev.row.actionParam1)
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.StandState |= flag
		tm.StandStateKnown = true
		s.motionMu.Unlock()
	}
}

// smartMoveJumpEventID is EVENT_JUMP (SharedDefines.h:3343), the default
// MoveJump inform id — the GenericMovementGenerator finalize informs
// (EFFECT_MOTION_TYPE, EVENT_JUMP).
const smartMoveJumpEventID uint32 = 1004

// smartActionJumpToPos is SMART_ACTION_JUMP_TO_POS (SmartScript.cpp:1893-1899):
// every creature target jumps to the row's target_x/y/z at speedXY param1
// and speedZ param2 (MoveJump(x, y, z, 0.0f, speedxy, speedz) — orientation
// 0, hasOrientation false). The launch is the Lua MoveJump twin
// (launchCreatureJumpSpline, GenericMovementGenerator EFFECT_MOTION_TYPE 16):
// speedXY < 0.01 launches nothing (MotionMaster.cpp:790), speedZ <= 0
// degenerates to a flat arc (jumpParabolicParams), and the landing informs
// (EFFECT_MOTION_TYPE, EVENT_JUMP) through the script-spline finalize gate.
// The POSITION-target row (Haroka's Kitten) is a live C++ no-op — GetTargets
// pushes nothing for SMART_TARGET_POSITION — and no-ops here the same way
// (smartTargets returns nil). TDB 26091: 12 rows (9 src-0 + 3 src-9),
// speedXY 15-109, speedZ 0-32.
func (s *Server) smartActionJumpToPos(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	speedXY := float32(ev.row.actionParam1)
	speedZ := float32(ev.row.actionParam2)
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer || tgt.isGameObject {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.launchCreatureJumpSpline(tm, ev.row.targetX, ev.row.targetY, ev.row.targetZ, speedXY, speedZ, smartMoveJumpEventID)
	}
}

// smartSpellClick is SmartAI::OnSpellClick (SmartAI.cpp:879-884):
// the event fires only when a click row's cast ran
// (spellClickHandled), with the clicker as invoker. No veto — the
// return value is unused, unlike the gossip twins.
func (s *Server) smartSpellClick(ctx context.Context, motion *creatureMotion, invokerGUID uint64, handled bool) {
	if !handled {
		return
	}
	s.smartProcessEvents(ctx, motion, smartEventOnSpellClick, invokerGUID, 0, 0)
}

// smartPassengerBoarded is SmartAI::PassengerBoarded
// (SmartAI.cpp:667-670): apply boards (SMART_EVENT_PASSENGER_BOARDED,
// 27) and unseats (SMART_EVENT_PASSENGER_REMOVED, 28), with the
// passenger as invoker and the seat id as var0 (dropped by the event
// case, SmartScript.cpp:3294-3300). Fires from every Go
// AddPassenger/RemovePassenger site next to the Eluna vehicle event.
func (s *Server) smartPassengerBoarded(ctx context.Context, base *creatureMotion, passengerGUID uint64, seatID int8, apply bool) {
	if s == nil || base == nil || passengerGUID == 0 {
		return
	}
	eventType := smartEventPassengerRemoved
	if apply {
		eventType = smartEventPassengerBoarded
	}
	s.smartProcessEvents(ctx, base, eventType, passengerGUID, uint32(seatID), 0)
}

// smartJustSummoned is SmartAI::IsSummonedBy (SmartAI.cpp:647-650):
// the summoned creature's own JUST_SUMMONED (54) event fires with the
// summoner as invoker. TempSummon::InitSummon (TemporarySummon.cpp:231-
// 247) calls it for every temp summon; world spawns with no summoner
// (origin GUID 0) never reach it.
func (s *Server) smartJustSummoned(ctx context.Context, summoned *creatureMotion, summonerGUID uint64) {
	if s == nil || summoned == nil || summonerGUID == 0 {
		return
	}
	s.smartProcessEvents(ctx, summoned, smartEventJustSummoned, summonerGUID, 0, 0)
}

// smartSummonedUnit is SmartAI::JustSummoned (SmartAI.cpp:583-586):
// the summoner's SUMMONED_UNIT (17) event fires with the summoned
// creature as invoker. TempSummon::InitSummon (TemporarySummon.cpp:231-
// 247) calls it on the owner's AI for every temp summon whose owner is
// a creature with AI enabled (a GO owner fires SmartGameObjectAI::
// JustSummoned instead — no Go script state, stays out); the summoned
// creature's own IsSummonedBy (54) fires right after, matching the
// C++ order.
func (s *Server) smartSummonedUnit(ctx context.Context, summoner, summoned *creatureMotion) {
	if s == nil || summoner == nil || summoned == nil {
		return
	}
	s.smartProcessEvents(ctx, summoner, smartEventSummonedUnit, summoned.GUID, 0, 0)
}

// smartSummonedUnitDies is SmartAI::SummonedCreatureDies
// (SmartAI.cpp:588-591): the summoner's SUMMONED_UNIT_DIES (82) event
// fires with the dead summoned creature as invoker. Unit::Kill
// (Unit.cpp:11393-11401) calls it right after ai->JustDied, only when
// the victim is a TempSummon whose summoner is a creature with AI
// enabled (a GO summoner fires SmartGameObjectAI::SummonedCreatureDies
// instead — no Go script state, stays out); a player summoner never
// qualifies, so a creature summoner is resolved here from the dead
// motion's CreatedByGUID stamp.
func (s *Server) smartSummonedUnitDies(ctx context.Context, dead *creatureMotion) {
	if s == nil || dead == nil || !dead.IsTempSummon || dead.CreatedByGUID == 0 {
		return
	}
	summoner := s.findCreatureMotion(dead.Map, dead.InstanceID, dead.CreatedByGUID)
	if summoner == nil {
		return
	}
	s.smartProcessEvents(ctx, summoner, smartEventSummonedUnitDies, dead.GUID, 0, 0)
}

// smartGossipSelect is SmartAI::OnGossipSelect (SmartAI.cpp:765-770):
// reset _gossipReturn, fire SMART_EVENT_GOSSIP_SELECT (62) with the
// player as invoker and the (menuID, listID) pair, return
// _gossipReturn. A SEND_GOSSIP_MENU (98) action sets it while the
// event runs, vetoing the native Player::OnGossipSelect arms
// (MiscHandler.cpp:209-212).
func (s *Server) smartGossipSelect(ctx context.Context, motion *creatureMotion, invokerGUID uint64, menuID, listID uint32) bool {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return false
	}
	st.gossipReturn = false
	s.smartProcessEvents(ctx, motion, smartEventGossipSelect, invokerGUID, menuID, listID)
	return st.gossipReturn
}

// smartGossipHello is SmartAI::OnGossipHello (SmartAI.cpp:758-763):
// reset _gossipReturn, fire SMART_EVENT_GOSSIP_HELLO (64) with the
// player as invoker, return _gossipReturn. A SEND_GOSSIP_MENU (98)
// action sets it while the event runs, vetoing the default menu.
func (s *Server) smartGossipHello(ctx context.Context, motion *creatureMotion, invokerGUID uint64) bool {
	st := s.smartStateFor(ctx, motion)
	if st == nil {
		return false
	}
	st.gossipReturn = false
	s.smartProcessEvents(ctx, motion, smartEventGossipHello, invokerGUID, 0, 0)
	return st.gossipReturn
}

// smartActionSendGossipMenu is SMART_ACTION_SEND_GOSSIP_MENU
// (SmartScript.cpp:1946-1972): override the default gossip —
// SetGossipReturn(true) on the base AI — then every player target
// takes PrepareGossipMenu(base, menuId, showQuests=true) when menuId
// is set, or a cleared menu when it is 0, followed by
// SendGossipMenu(npcTextId, baseGUID), which sends SMSG_GOSSIP_MESSAGE
// with the prepared menu and the explicit npc text. The Go twin is
// prepareCreatureGossip with the explicit (non-default) menu and the
// TitleID overridden to the row's npcTextId — C++ never takes the
// gossip_menu TextID here. The GO base leg stays out: Go has no GO
// script state (the SMART_ACTION_OVERRIDE_SCRIPT_BASE_OBJECT (76) GO
// residual), and all 14 live rows are creature-source. TDB 26091: 14
// rows, all src-0 / GOSSIP_HELLO (64) / ACTION_INVOKER (7), menuId
// and npcTextId both nonzero on every row.
func (s *Server) smartActionSendGossipMenu(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	st.gossipReturn = true
	menuID := ev.row.actionParam1
	npcTextID := ev.row.actionParam2
	s.motionMu.Lock()
	baseGUID, baseEntry, npcFlags := motion.GUID, motion.Entry, motion.NPCFlags
	s.motionMu.Unlock()
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isPlayer {
			continue
		}
		sess := s.findSessionByGUID(tgt.guid)
		if sess == nil {
			continue
		}
		if menuID == 0 {
			sess.gossip = &gossipMenuState{SenderGUID: baseGUID, MenuID: 0, TitleID: npcTextID, Items: make(map[uint32]gossipMenuItem)}
			sess.gossipClosed = false
			_ = sess.sendGossipMenu()
			continue
		}
		menu, err := sess.prepareCreatureGossip(ctx, baseGUID, baseEntry, npcFlags, menuID, false, true)
		if err != nil {
			continue
		}
		if menu == nil {
			// The TAXIVENDOR abort inside PrepareGossipMenu leaves
			// C++ sending the (cleared) menu with the npc text
			// anyway — SendGossipMenu still runs.
			menu = &gossipMenuState{SenderGUID: baseGUID, MenuID: menuID, TitleID: npcTextID, Items: make(map[uint32]gossipMenuItem)}
		} else {
			menu.TitleID = npcTextID
		}
		sess.gossip = menu
		sess.gossipClosed = false
		_ = sess.sendGossipMenu()
	}
}

// smartActionGOSetLootState is SMART_ACTION_GO_SET_LOOT_STATE
// (SmartScript.cpp:1900-1907): every gameobject target takes
// SetLootState(state) — a plain field write (GameObject.cpp:2371)
// that fires OnLootStateChanged, whose Eluna twin is the
// GAMEOBJECT_EVENT_ON_LOOT_STATE_CHANGE trigger the Lua SetLootState
// bridge fires (lua_objects.go:4652). The Go twin is the
// setGameObjectLootState store plus that trigger. TDB 26091: 22 rows;
// the 13 live src-9 (timed-actionlist) rows ride the creature leg — 7
// CLOSEST_GAMEOBJECT, the 5 SELF + 1 INVOKER_PARTY rows no-op on both
// sides (IsGameObject false) — and the 9 src-1 GO-script rows stay out
// (no GO script engine). States in rows: 0/1/2/3, the whole LootState
// enum (GameObject.h:68-74); no validation, matching the C++ cast.
func (s *Server) smartActionGOSetLootState(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	state := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			continue
		}
		s.setGameObjectLootState(motion.Map, motion.InstanceID, tgt.guid, uint8(state))
		s.triggerGameObjectEvent(ctx, tgt.guid, scripting.GameObjectEventOnLootState, state)
	}
}

// smartActionGOSetGoState is SMART_ACTION_GO_SET_GO_STATE
// (SmartScript.cpp:1907-1914): every gameobject target takes
// SetGoState(state) — the GAMEOBJECT_BYTES_1 byte-0 write
// (GameObject.cpp:2407) whose Go twin is
// setGameObjectStateInInstance (values update +
// GAMEOBJECT_EVENT_ON_GO_STATE_CHANGE). The AI OnStateChanged leg
// stays out (no GO script engine) and the collision toggle has no Go
// twin. TDB 26091: 5 rows, all src-1 GO-source — no live
// creature-leg rows; the port rides the dispatch for
// timed-actionlist/creature rows of the same shape. States in rows:
// 1 (READY) and 2 (DESTROYED, SharedDefines.h:1659-1663 — the Eluna
// doc's ACTIVE_ALTERNATIVE label for 2 is stale in this revision);
// no validation, matching the C++ cast.
func (s *Server) smartActionGOSetGoState(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	state := ev.row.actionParam1
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if !tgt.isGameObject {
			continue
		}
		s.setGameObjectStateInInstance(motion.Map, motion.InstanceID, tgt.guid, uint8(state))
	}
}

// smartActionSpawnSpawnGroup is SMART_ACTION_SPAWN_SPAWNGROUP
// (SmartScript.cpp:2162-2203): with minDelay == maxDelay == 0 the
// group spawns at once via Map::SpawnGroupSpawn (spawnGroupSpawn,
// ignoreRespawn + force from spawnflags); otherwise a one-shot
// SMART_EVENT_UPDATE (chance 100, NOT_REPEATABLE, min/max delay,
// same groupId/spawnflags, delays zeroed) is stored and fires the
// instant leg on expiry. TDB 26091: 5 rows, all instant (delays 0);
// 4 timed-actionlist + 1 creature (Raelorasz). The stored event
// re-rolls its timer on a script reset (C++ keeps mStoredEvents
// timers); no live row exercises it.
func (s *Server) smartActionSpawnSpawnGroup(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	groupID := ev.row.actionParam1
	minDelay := ev.row.actionParam2
	maxDelay := ev.row.actionParam3
	spawnflags := ev.row.actionParam4
	if minDelay == 0 && maxDelay == 0 {
		if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
			return
		}
		s.spawnGroupSpawn(ctx, s.WorldStore.DB, motion.Map, motion.InstanceID, groupID, spawnflags&smartSpawnFlagIgnoreRespawn != 0, spawnflags&smartSpawnFlagForceSpawn != 0)
		return
	}
	s.smartInstallTemplateEvent(st, smartEventUpdate, smartEventFlagNotRepeatable, minDelay, maxDelay, 0, 0, 0, smartActionSpawnSpawnGroup, groupID, 0, 0, spawnflags, 0, 0, ev.row.targetType, ev.row.targetParam1, ev.row.targetParam2, ev.row.targetParam3, ev.row.targetParam4, 0)
}

// smartActionDespawnSpawnGroup is SMART_ACTION_DESPAWN_SPAWNGROUP
// (SmartScript.cpp:2205-2246): the despawn mirror of 131 — instant
// Map::SpawnGroupDespawn (spawnGroupDespawn, deleteRespawnTimes from
// the NOSAVE_RESPAWN flag) when both delays are 0, else the same
// one-shot SMART_EVENT_UPDATE store. TDB 26091: 12 rows, all instant;
// 10 timed-actionlist + 2 creature (Commander Hobb on death).
// NOSAVE_RESPAWN (deleteRespawnTimes) on 10 rows, 0 on the Thrall /
// Drek'Thar rows. Same stored-event reset caveat as 131.
func (s *Server) smartActionDespawnSpawnGroup(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	groupID := ev.row.actionParam1
	minDelay := ev.row.actionParam2
	maxDelay := ev.row.actionParam3
	spawnflags := ev.row.actionParam4
	if minDelay == 0 && maxDelay == 0 {
		if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil {
			return
		}
		s.spawnGroupDespawn(ctx, s.WorldStore.DB, motion.Map, motion.InstanceID, groupID, spawnflags&smartSpawnFlagNoSaveRespawn != 0)
		return
	}
	s.smartInstallTemplateEvent(st, smartEventUpdate, smartEventFlagNotRepeatable, minDelay, maxDelay, 0, 0, 0, smartActionDespawnSpawnGroup, groupID, 0, 0, spawnflags, 0, 0, ev.row.targetType, ev.row.targetParam1, ev.row.targetParam2, ev.row.targetParam3, ev.row.targetParam4, 0)
}

// smartActionSendTargetToTarget is SMART_ACTION_SEND_TARGET_TO_TARGET
// (SmartScript.cpp:1914-1945): copy this script's stored list id into
// every creature target's own script (a copy, not the shared slice).
// A missing id breaks with nothing stored — the map presence check is
// the GetStoredTargetVector null check, so a stored-but-empty list
// still propagates. Non-SmartAI creature targets log-and-skip in C++;
// Go skips them the same way. GO targets stay out — Go has no GO
// script state (the SMART_ACTION_OVERRIDE_SCRIPT_BASE_OBJECT (76) GO
// residual). TDB 26091: 155 rows; 93 live src-0/9 (51 timed-actionlist
// + 42 creature), the 28 GO-source + 34 areatrigger rows stay out (no
// GO / areatrigger script engine). Ids are 1 (153 rows) and 2 (2 rows).
func (s *Server) smartActionSendTargetToTarget(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	stored, ok := st.storedTargets[ev.row.actionParam1]
	if !ok {
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer || tgt.isGameObject {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || tm.AIName != "SmartAI" {
			continue
		}
		tst := s.smartStateForTimedList(ctx, tm)
		if tst == nil {
			continue
		}
		if tst.storedTargets == nil {
			tst.storedTargets = make(map[uint32][]uint64)
		}
		tst.storedTargets[ev.row.actionParam1] = append([]uint64(nil), stored...)
	}
}

// smartActionSetHomePos is SMART_ACTION_SET_HOME_POS (SmartScript.cpp:
// 1974-1999): the home position is the target type's position, not a
// param — all params are 0 in TDB 26091. SELF homes me to me's current
// position; the range/GUID/distance, closest and owner/invoker legs
// home each creature target to its own current position; POSITION
// homes to the row's target_x/y/z/o. The POSITION leg is dead in
// practice: GetTargets yields no targets for it (SmartScript.cpp:
// 2864-2867), so the loop never runs, matching C++ exactly. Any other
// target type logs and skips in C++ — TDB's one CLOSEST_PLAYER row
// (1780900 timed list, Champion Bloodwrath) is that skip leg, and it
// stays out here too. GO/player targets fail the IsCreature check:
// findCreatureMotion returns nil for them. TDB 26091: 102 rows, 100
// SELF, 1 CLOSEST_PLAYER, 1 OWNER_OR_SUMMONER (2417300 timed list,
// Frostgore, home to summoner position).
func (s *Server) smartActionSetHomePos(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	switch ev.row.targetType {
	case smartTargetSelf:
		s.motionMu.Lock()
		motion.HomeX, motion.HomeY, motion.HomeZ = motion.X, motion.Y, motion.Z
		motion.HomeO = motion.Orientation
		s.motionMu.Unlock()
	case smartTargetPosition:
		// Dead leg: no targets resolve for POSITION, so nothing homes.
	case smartTargetActionInvoker, smartTargetCreatureRange, smartTargetCreatureGUID, smartTargetCreatureDist, smartTargetGameObjectRange, smartTargetGameObjectGUID, smartTargetGameObjectDist, smartTargetClosestCreature, smartTargetClosestGameObject, smartTargetOwnerOrSummoner:
		// C++ also lists CLOSEST_ENEMY (25), CLOSEST_FRIENDLY (26)
		// and CLOSEST_UNSPAWNED_GAMEOBJECT (30); Go has no consts
		// for them and smartTargets resolves none, so no row can
		// reach this leg through them.
		for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
			if tgt.isPlayer {
				continue
			}
			tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
			if tm == nil {
				continue
			}
			s.motionMu.Lock()
			tm.HomeX, tm.HomeY, tm.HomeZ = tm.X, tm.Y, tm.Z
			tm.HomeO = tm.Orientation
			s.motionMu.Unlock()
		}
	default:
		// C++ TC_LOG_ERROR + skip for any other target type.
	}
}

// smartActionSetHealthRegen is SMART_ACTION_SET_HEALTH_REGEN
// (SmartScript.cpp:2000-2006): Creature::SetRegenerateHealth(param1
// != 0) on every creature target. The Go twin is the
// RegenHealthDisabled latch the Lua SetRegenerateHealth bridge writes
// (lua_creature_events.go:5639) and the regen tick honors
// (creaturemotion.go:145). TDB 26091: 23 rows, param1 0 x20 (regen
// off) / 1 x3 (on); 22 SELF, 1 STORED (2533500 timed list,
// Longrunner Proudhoof, stored list 1).
func (s *Server) smartActionSetHealthRegen(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	regen := ev.row.actionParam1 != 0
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.RegenHealthDisabled = !regen
		s.motionMu.Unlock()
	}
}

// smartActionSummonCreatureGroup is SMART_ACTION_SUMMON_CREATURE_GROUP
// (SmartScript.cpp:2035-2044): WorldObject::SummonCreatureGroup(group,
// &list) on the base object — targets are ignored — then, when
// param2 (attackInvoker) is set and an invoker exists, every summon
// AI()->AttackStart(unit). The group summon rides the shared
// summonCreatureGroup funnel (creature_summon_groups keyed by the
// summoner's entry, summonerType creature, groupId); the AttackStart
// leg rides triggerCreatureAggro, the SUMMON_CREATURE attackInvoker
// twin. TDB 26091: 231 rows — 15 creature + 210 timed-list rows live
// (all 15 creature rows have exact group coverage; groups 0-12 all
// present), 6 GO-source rows stay out (Go loads no GO smart scripts),
// attackInvoker set on 27 rows.
func (s *Server) smartActionSummonCreatureGroup(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	summoned := s.summonCreatureGroup(ctx, motion, ev.row.actionParam1)
	if ev.row.actionParam2 == 0 || len(summoned) == 0 {
		return
	}
	invoker := invokerGUID
	if invoker == 0 {
		invoker = st.lastInvoker
	}
	if invoker == 0 {
		return
	}
	for _, sm := range summoned {
		s.triggerCreatureAggro(ctx, sm.GUID, invoker, true)
	}
}

// smartActionSetPower is SMART_ACTION_SET_POWER
// (SmartScript.cpp:2045-2051): Unit::SetPower(Powers(param1), param2)
// on every unit target. The C++ leg no-ops when the value is
// unchanged, clamps to the max power, writes UNIT_FIELD_POWER1+power,
// and sends SMSG_POWER_UPDATE to the set (plus a group-update flag
// for players/pets). The creature leg mirrors the pet power flush
// (pet_runtime.go): values update + SMSG_POWER_UPDATE to the visible
// set, Mana mirrored for power 0 (boss_ai.go energize twin); the
// player leg writes the session power and sends the update to self +
// visible set (spells.go). The party-frame flag rides the 5s
// SMSG_PARTY_MEMBER_STATS delta flush (flushGroupMemberStats), as in
// C++ — never pushed immediately. TDB 26091: 3 rows, all RESET (25)
// events on SELF, mana (0) to 0 — Emissary Roman'khan (14862),
// Obsidian Eradicator (15262), Obsidian Nullifier (15312). 109
// ADD_POWER / 110 REMOVE_POWER have zero rows.
func (s *Server) smartActionSetPower(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	powerType := ev.row.actionParam1
	newPower := ev.row.actionParam2
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			continue
		}
		if tgt.isPlayer {
			sess := s.findSessionByGUID(tgt.guid)
			if sess == nil || sess.player == nil {
				continue
			}
			if powerType >= uint32(len(sess.player.Powers)) {
				continue
			}
			if sess.player.Powers[powerType] == newPower {
				continue
			}
			val := newPower
			if val > sess.player.MaxPowers[powerType] {
				val = sess.player.MaxPowers[powerType]
			}
			sess.player.Powers[powerType] = val
			packet := protocol.NewBuffer(packedGUIDSize(tgt.guid) + 5)
			packet.WritePackedGUID(tgt.guid)
			packet.WriteU8(uint8(powerType))
			packet.WriteU32(val)
			_ = sess.write(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), true)
			s.broadcastToVisibleSetFromSession(sess, uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes())
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		if powerType >= uint32(len(tm.Powers)) {
			continue
		}
		s.motionMu.Lock()
		if tm.Powers[powerType] == newPower {
			s.motionMu.Unlock()
			continue
		}
		val := newPower
		if val > tm.MaxPowers[powerType] {
			val = tm.MaxPowers[powerType]
		}
		tm.Powers[powerType] = val
		if powerType == 0 {
			tm.Mana = val
		}
		x, y, z, phase := tm.X, tm.Y, tm.Z, tm.PhaseMask
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(motion.Map, motion.InstanceID, tgt.guid, map[int]uint32{unitFieldPower1 + int(powerType): val})
		packet := protocol.NewBuffer(packedGUIDSize(tgt.guid) + 5)
		packet.WritePackedGUID(tgt.guid)
		packet.WriteU8(uint8(powerType))
		packet.WriteU32(val)
		s.broadcastToVisibleSetFromPhase(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), motion.Map, motion.InstanceID, x, y, z, phase)
	}
}

// smartActionGameEventStop is SMART_ACTION_GAME_EVENT_STOP
// (SmartScript.cpp:2066-2075): when the event is not active C++ logs
// a sql error and skips; otherwise StopEvent(id, true) — overwrite
// true, the schedule rewrite the luaGameEventControl force leg
// applies (lua_globals.go). The active check reads the same cached
// set the .event stop command uses (commands.go). TDB 26091: 2
// timed-action-list (src 9) rows — event 81 (Ognip Blastbolt,
// 2821000) and event 84 (Priestess Alorah, 3610200); both ids exist
// in game_event.
func (s *Server) smartActionGameEventStop(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	eventID := ev.row.actionParam1
	if _, active := s.cachedActiveGameEvents(ctx)[int64(eventID)]; !active {
		return
	}
	s.luaGameEventControl(eventID, false, true)
}

// smartActionGameEventStart is SMART_ACTION_GAME_EVENT_START
// (SmartScript.cpp:2077-2086): when the event is already active C++
// logs a sql error and skips; otherwise StartEvent(id, true) — the
// overwrite-true twin of the stop leg above. TDB 26091: 1
// timed-action-list (src 9) row — event 81 (Ognip Blastbolt,
// 2821000), the same event the stop leg above halts.
func (s *Server) smartActionGameEventStart(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	eventID := ev.row.actionParam1
	if _, active := s.cachedActiveGameEvents(ctx)[int64(eventID)]; active {
		return
	}
	s.luaGameEventControl(eventID, true, true)
}

// smartActionStartClosestWaypoint is SMART_ACTION_START_CLOSEST_WAYPOINT
// (SmartScript.cpp:2088-2126): action params 1-6 are SmartAI waypoint path
// ids; the closest node across all listed paths (3D distance from the
// target creature, strict < so ties keep the first path/node in param
// order) starts an escort — walk, repeat, no invoker — on that creature.
// The closest pair is one accumulator shared across targets (declared
// outside the C++ target loop), and StartPath runs per target with the
// running closest. Only SmartAI creatures start (IsSmart; smartStateFor
// is nil otherwise); player and GO targets are skipped, as is a path
// that loads no nodes. StartPath's nodeId only seeds the AI's
// current-waypoint bookkeeping (_currentWaypointNode, SmartAI.cpp:61):
// the waypoint generator itself always starts at nodes[0]
// (PathMovementBase::_currentNode starts 0), so the escort walks from
// node 1 and escortNode is seeded with the closest node id. TDB 26091:
// 2 rows, both SELF — Skyguard Prisoner (23383, paths 18706401/18706409/
// 18706417) and Injured Goblin Miner (2943401, 23547201/23547209/23547217)
// — the paths resolve in waypoint_data via the loadSmartWaypoints
// fallback (the `waypoints` table has no CREATE TABLE in TDB 26091).
func (s *Server) smartActionStartClosestWaypoint(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	waypoints := [6]uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6}
	bestDist := math.MaxFloat64
	var closestPath, closestNode uint32
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer || tgt.isGameObject {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		tst := s.smartStateFor(ctx, tm)
		if tst == nil {
			continue
		}
		s.motionMu.Lock()
		tx, ty, tz := tm.X, tm.Y, tm.Z
		s.motionMu.Unlock()
		for _, pathID := range waypoints {
			if pathID == 0 {
				continue
			}
			for _, p := range s.loadSmartWaypoints(ctx, pathID) {
				dx := float64(tx - p.X)
				dy := float64(ty - p.Y)
				dz := float64(tz - p.Z)
				if d := dx*dx + dy*dy + dz*dz; d < bestDist {
					bestDist = d
					closestPath, closestNode = pathID, p.Point
				}
			}
		}
		if closestPath != 0 {
			s.smartStartPath(ctx, tm, tst, false, closestPath, true, 0)
			tst.escortNode = closestNode
		}
	}
}

// smartActionMoveOffset is SMART_ACTION_MOVE_OFFSET (SmartScript.cpp:1203-1225):
// every creature target steps by the row's target_x/y/z offset in its own
// facing frame — x strafes (cos(o-pi/2)), y advances (cos(o)), z rises —
// via MovePoint(SMART_RANDOM_POINT, ...). A charmed target is skipped
// unless the event carries WHILE_CHARMED (0x200); players and GOs are not
// creatures and are skipped. The arrival fires SMART_EVENT_MOVEMENTINFORM
// (34) with (POINT_MOTION_TYPE, 0xFFFFFE) through the script-spline
// finalize leg — the CAGED_NPC template's credit/despawn/talk legs match
// on it, and its synthetic MOVE_OFFSET rows carry target offsets 0, so
// they step in place and arrive immediately, exactly as in C++. TDB
// 26091: 76 rows, all action params 0 — the offsets live in target_x/y/z
// (forward 1-200, strafe 30x2, rise 3/12/25/90) — 72 SELF + 4
// ACTION_INVOKER.
func (s *Server) smartActionMoveOffset(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	ox, oy, oz := ev.row.targetX, ev.row.targetY, ev.row.targetZ
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer || tgt.isGameObject {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		px, py, pz, po := tm.X, tm.Y, tm.Z, tm.Orientation
		charmed := tm.CharmerGUID != 0
		s.motionMu.Unlock()
		if ev.row.eventFlags&smartEventFlagWhileCharmed == 0 && charmed {
			continue
		}
		o := float64(po)
		nx := px + float32(math.Cos(o-math.Pi/2)*float64(ox)+math.Cos(o)*float64(oy))
		ny := py + float32(math.Sin(o-math.Pi/2)*float64(ox)+math.Sin(o)*float64(oy))
		s.launchCreatureScriptSpline(tm, nx, ny, pz+oz, false, movementGeneratorPoint, smartRandomPoint, true)
	}
}

// smartActionRandomSound is SMART_ACTION_RANDOM_SOUND (SmartScript.cpp:2128-2150):
// one sound picked uniformly per unit target from the non-zero action
// params 1-4 (the struct is sounds[4] + onlySelf + distance, so param5 is
// onlySelf and param6 is distance — the enum comment's "soundId1..5,
// onlySelf" is stale), played via the shared PlayDirectSound /
// PlayDistanceSound send. TDB 26091: 14 rows, all Lazy Peon (src 9),
// sounds 6292/6294, onlySelf 0, distance 1 — distance-attenuated
// SMSG_PLAY_OBJECT_SOUND to the visible set.
func (s *Server) smartActionRandomSound(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	sounds := make([]uint32, 0, 4)
	for _, snd := range [4]uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4} {
		if snd != 0 {
			sounds = append(sounds, snd)
		}
	}
	if len(sounds) == 0 {
		return
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isGameObject {
			continue
		}
		s.smartPlaySound(motion, tgt, sounds[rand.Intn(len(sounds))], ev.row.actionParam5 != 0, ev.row.actionParam6 == 1)
	}
}

// smartActionSetCorpseDelay is SMART_ACTION_SET_CORPSE_DELAY
// (SmartScript.cpp:2152-2160): every creature target takes
// SetCorpseDelay(action_param1), the m_corpseDelay override in
// seconds — the rank default (Creature.cpp:1153-1165) stands until
// the action fires. The override lives on the motion (the
// CorpseDelayOverrideSet latch) and the death paths read it when
// they build the respawn record (kill.go), so the corpse removes at
// death + the overridden delay and a compat-mode respawn pushes by
// it too. A respawn rebuilds the motion, so the override dies with
// the creature like the C++ DB reload. TDB 26091: 69 rows, all
// target SELF, param1 0-120s (5s on 33, 0s on 2), 59 on AI_INIT.
func (s *Server) smartActionSetCorpseDelay(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.CorpseDelayOverride = ev.row.actionParam1
		tm.CorpseDelayOverrideSet = true
		s.motionMu.Unlock()
	}
}

// smartActionPauseMovement is SMART_ACTION_PAUSE_MOVEMENT
// (SmartScript.cpp:2306-2312): every unit target takes
// Unit::PauseMovement(pauseTimer = param2 ms, movementSlot = param1,
// force = param3). Slots are MOTION_SLOT_DEFAULT 0 and MOTION_SLOT_ACTIVE
// 1 (MovementDefines.h:63-67); slot >= 2 is invalid and the whole call
// no-ops (IsInvalidMovementSlot) — the one live slot-2 row (24514
// Steelfeather, timer 1) is a no-op in C++ too. Only the Random and
// Waypoint generators implement Pause (the base is a no-op), and both
// live in the default slot, so slot 0 pauses the idle legs — waypoint
// patrol, random wander, and the escort path (MovePath is a waypoint
// generator, SmartAI.cpp:75) — while slot 1 pauses nothing on its own:
// chase/follow/point/home generators inherit the no-op Pause. A timer
// pauses until now + timer and clears an indefinite pause; timer 0
// pauses indefinitely (WaypointMovementGenerator::Pause), cleared by
// evade (the MotionMaster::Clear twin) — Go has no non-escort
// ResumeMovement bridge, and WP_RESUME is escort-only. The timer-0
// latch is unreachable from live rows: the C++ loader rejects
// pauseTimer 0 (SmartScriptMgr.cpp:1524), so the two live timer-0
// rows (26811/26812) never execute in C++ either. The forced
// leg stops the current spline only when the paused slot IS the
// current slot (MotionMaster::GetCurrentSlot: active while any active
// generator runs, else default): halted here as Moving = false, which
// also drops the arrival inform, like a mid-flight StopMoving. Player
// targets stay out (no Go player generator model). TDB 26091: 8 rows,
// all SELF — slot 0 timer 300000 x4 (23094), slot 0 timer 0 force 1
// x2 (26811/26812), slot 0 timer 5000 force 1 (40176), slot 2 timer 1
// (24514).
func (s *Server) smartActionPauseMovement(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	slot := ev.row.actionParam1
	if slot >= 2 {
		return
	}
	timer := ev.row.actionParam2
	force := ev.row.actionParam3 != 0
	now := time.Now()
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.MovementPausedSlot = uint8(slot)
		if timer == 0 {
			tm.MovementPaused = true
			tm.MovementPausedUntil = time.Time{}
		} else {
			tm.MovementPausedUntil = now.Add(time.Duration(timer) * time.Millisecond)
			tm.MovementPaused = false
		}
		if force {
			// MotionMaster::GetCurrentSlot (MotionMaster.cpp:176-185):
			// ACTIVE while any active generator runs — a chase or
			// follow slot, an in-flight script spline, fleeing, or
			// confused — else DEFAULT.
			active := tm.ScriptChaseTargetGUID != 0 || tm.ScriptFollowTargetGUID != 0 || tm.Evading || tm.Fleeing || tm.Confused || (tm.Moving && now.Before(tm.MoveEnds) && tm.ScriptMoveType != 0)
			currentSlot := uint32(0)
			if active {
				currentSlot = 1
			}
			if slot == currentSlot {
				tm.Moving = false
			}
		}
		s.motionMu.Unlock()
	}
}

// smartActionSetReactState is SMART_ACTION_SET_REACT_STATE
// (SmartScript.cpp:464-472): every creature target takes the param1
// react state (Creature.h:117: a plain setter, no broadcast). The
// ReactStateKnown stamp keeps the sweep from re-affirming the template
// value (creaturemotion.go:5945) — the same latch the Lua SetReactState
// bridge stamps. The Go react values match C++'s ReactStates order
// (UnitDefines.h:340-342: passive 0, defensive 1, aggressive 2).
func (s *Server) smartActionSetReactState(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.ReactState = uint8(ev.row.actionParam1)
		tm.ReactStateKnown = true
		s.motionMu.Unlock()
	}
}

// smartActionNPCFlag is SMART_ACTION_SET_NPC_FLAG / ADD_NPC_FLAG /
// REMOVE_NPC_FLAG (SmartScript.cpp:1700-1720): SetUInt32Value /
// SetFlag / RemoveFlag on UNIT_NPC_FLAGS (field 82) for creature
// targets — whole-value / OR / AND-NOT. The motion value plus the
// NPCFlagsKnown latch (the Lua SetNPCFlags bridge's twin) and the
// field-82 values broadcast carry it to clients.
func (s *Server) smartActionNPCFlag(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		switch ev.row.actionType {
		case smartActionSetNPCFlag:
			tm.NPCFlags = ev.row.actionParam1
		case smartActionAddNPCFlag:
			tm.NPCFlags |= ev.row.actionParam1
		default:
			tm.NPCFlags &^= ev.row.actionParam1
		}
		tm.NPCFlagsKnown = true
		flags := tm.NPCFlags
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		s.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldNPCFlags: flags})
	}
}

// broadcastCreatureDestroyToPlayers pushes SMSG_DESTROY_OBJECT for a
// creature to every non-GM-seer session in its map/instance — the hide
// half of the Lua SetVisible twin (lua_creature_events.go:7031). GM
// seers (gmVisibilityDetect != 0) keep seeing the unit, matching C++
// server-side visibility SERVERSIDE_VISIBILITY_GM.
func (s *Server) broadcastCreatureDestroyToPlayers(mapID, instanceID uint32, guid uint64) {
	destroyBuf := protocol.NewBuffer(9)
	destroyBuf.WriteU64(guid)
	destroyBuf.WriteU8(0)
	payload := destroyBuf.Bytes()
	opcode := uint16(protocol.OpcodeSMSG_DESTROY_OBJECT)
	s.sessionsMu.RLock()
	for target := range s.sessions {
		if !target.authed || !target.worldReady.Load() || target.player == nil ||
			target.player.Map != mapID || target.player.InstanceID != instanceID ||
			target.gmVisibilityDetect != 0 {
			continue
		}
		_ = target.write(opcode, payload, true)
	}
	s.sessionsMu.RUnlock()
}

// smartActionSetVisibility is SMART_ACTION_SET_VISIBILITY
// (SmartScript.cpp:1225-1231): Unit::SetVisible on every target unit
// (Unit.cpp:8656-8663) — server-side visibility GM/PLAYER plus an
// object-visibility update. The creature leg is the Lua SetVisible twin
// (lua_creature_events.go:7031): ScriptHidden set under motionMu, and on
// hide the destroy object is pushed to the non-GM-seer sessions in the
// instance; on show the flag clears and the periodic nearby-creature
// rebuild re-creates the unit. Player targets stay out: Go has no
// player-side server-visibility twin (TDB 26091: the 1 live row
// targets SELF).
func (s *Server) smartActionSetVisibility(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	visible := ev.row.actionParam1 != 0
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.ScriptHidden = !visible
		mapID, instanceID, guid := tm.Map, tm.InstanceID, tm.GUID
		s.motionMu.Unlock()
		if !visible {
			s.broadcastCreatureDestroyToPlayers(mapID, instanceID, guid)
		}
	}
}

// smartActionInterruptSpell is SMART_ACTION_INTERRUPT_SPELL
// (SmartScript.cpp:1858-1867): InterruptNonMeleeSpells(withDelayed,
// spell_id, withInstant) on every target unit. The creature leg runs the
// spell-filtered twin (interruptCreatureNonMeleeCastsForSpell, param2
// as the id gate — TDB 26091: the 1 live row names 44473) and sends the
// cancel packets per interrupted arm, the INTERRUPT_PREVIOUS precedent
// in smartActionCast. withDelayed/withInstant are vacuous in Go's
// synchronous cast model (see the twin's comment). Player targets stay
// out: Go has no plain player-side InterruptNonMeleeSpells twin.
func (s *Server) smartActionInterruptSpell(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		for _, arm := range s.interruptCreatureNonMeleeCastsForSpell(tm, ev.row.actionParam2) {
			s.sendCreatureInterruptPackets(tm, arm)
		}
	}
}

// smartActionSetRoot is SMART_ACTION_SET_ROOT (SmartScript.cpp:2007-2014):
// SetControlled(root, UNIT_STATE_ROOT) on creature targets only — C++
// skips non-creatures itself (IsCreature). ROOT is not in
// UNIT_STATE_CONTROLLED (Unit.h:251), so apply runs no CastStop leg —
// the plain ScriptRooted set is the exact twin (Lua SetRooted,
// lua_creature_events.go:5637); the combat-tick chase gate honors it.
func (s *Server) smartActionSetRoot(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	root := ev.row.actionParam1 != 0
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil {
			continue
		}
		s.motionMu.Lock()
		tm.ScriptRooted = root
		s.motionMu.Unlock()
	}
}

// smartActionCallTimedActionList is SMART_ACTION_CALL_TIMED_ACTIONLIST
// (SmartScript.cpp:1677-1700) plus the RANDOM (87, :1763-1792) and
// RANDOM_RANGE (88, :1794-1814) twins: each target creature that IsSmart
// installs the list with the calling script's last invoker. TARGET_NONE
// installs nothing (C++ logs the bad row and breaks; Go resolves no
// targets for it). The random twins share the calling row's param2/3 as
// timerType/allowOverride through the C++ action union overlap
// (SmartScriptMgr.h:977-983: randTimedActionList.actionLists[1..2] are
// timedActionList.timerType/allowOverride), so 87 reads its ids from
// params 1-6 and 88 its range from params 1-2 at the same positions.
func (s *Server) smartActionCallTimedActionList(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) {
	if ev.row.targetType == smartTargetNone {
		return
	}
	listID := ev.row.actionParam1
	switch ev.row.actionType {
	case smartActionCallRandomTimedActionList:
		var ids []uint32
		for _, p := range []uint32{ev.row.actionParam1, ev.row.actionParam2, ev.row.actionParam3, ev.row.actionParam4, ev.row.actionParam5, ev.row.actionParam6} {
			if p != 0 {
				ids = append(ids, p)
			}
		}
		if len(ids) == 0 {
			return
		}
		listID = ids[rand.Intn(len(ids))]
	case smartActionCallRandomRangeTimedActionList:
		min, max := ev.row.actionParam1, ev.row.actionParam2
		if max < min {
			max = min
		}
		if max == min {
			listID = min
		} else {
			listID = min + uint32(rand.Int63n(int64(max-min+1)))
		}
	}
	invoker := invokerGUID
	if invoker == 0 {
		invoker = st.lastInvoker
	}
	for _, tgt := range s.smartTargets(ctx, motion, st, ev, invokerGUID) {
		if tgt.isPlayer {
			continue
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, tgt.guid)
		if tm == nil || tm.AIName != "SmartAI" {
			continue
		}
		s.smartSetTimedActionList(ctx, tm, listID, ev.row.actionParam2, ev.row.actionParam3 != 0, invoker)
	}
}

// smartTarget is one resolved SmartAI target: the GUID plus whether it is
// a player (creature otherwise — GOs never reach the AGGRO-used actions).
type smartTarget struct {
	guid         uint64
	isPlayer     bool
	isGameObject bool
}

// smartTargets is SmartScript::GetTargets (SmartScript.cpp:2510-...): the
// target list for the row's target type, resolved against the motion, the
// invoker (falling back to the last invoker, like C++'s scriptTrigger),
// and the stored-target lists. Range scans read the instance motion map
// under motionMu — the same-instance approximation of C++'s grid search
// (C++ searches the creature's grid cell; Go has no grid, so the whole
// instance is the search space). Distances are 3D, matching C++'s
// IsInRange / GetWorldObjectsInDist.
func (s *Server) smartTargets(ctx context.Context, motion *creatureMotion, st *smartScriptState, ev *smartScriptEvent, invokerGUID uint64) []smartTarget {
	if motion == nil || ev == nil {
		return nil
	}
	trigger := invokerGUID
	if trigger == 0 {
		trigger = st.lastInvoker
	}
	switch ev.row.targetType {
	case smartTargetNone:
		return nil
	case smartTargetSelf:
		return []smartTarget{{guid: motion.GUID}}
	case smartTargetVictim:
		if motion.TargetGUID == 0 {
			return nil
		}
		return []smartTarget{{guid: motion.TargetGUID, isPlayer: s.smartIsPlayerGUID(motion.TargetGUID)}}
	case smartTargetHostileRandom:
		return s.smartHostileRandomTarget(motion, float32(ev.row.targetParam1), ev.row.targetParam2 != 0, ev.row.targetParam3)
	case smartTargetActionInvoker:
		if trigger == 0 {
			return nil
		}
		return []smartTarget{{guid: trigger, isPlayer: s.smartIsPlayerGUID(trigger)}}
	case smartTargetPosition:
		return nil
	case smartTargetCreatureRange:
		return s.smartCreatureRangeTargets(motion, ev.row.targetParam1, float32(ev.row.targetParam2), float32(ev.row.targetParam3), ev.row.targetParam4)
	case smartTargetCreatureGUID:
		return s.smartCreatureGUIDTarget(motion, ev.row.targetParam1, ev.row.targetParam2)
	case smartTargetCreatureDist:
		return s.smartCreatureRangeTargets(motion, ev.row.targetParam1, 0, float32(ev.row.targetParam2), ev.row.targetParam3)
	case smartTargetStored:
		stored := st.storedTargets[ev.row.targetParam1]
		out := make([]smartTarget, 0, len(stored))
		for _, guid := range stored {
			out = append(out, smartTarget{guid: guid, isPlayer: s.smartIsPlayerGUID(guid)})
		}
		return out
	case smartTargetInvokerParty:
		return s.smartInvokerPartyTargets(motion, trigger)
	case smartTargetGameObjectRange:
		return s.smartGameObjectRangeTargets(ctx, motion, ev.row.targetParam1, float32(ev.row.targetParam2), float32(ev.row.targetParam3), ev.row.targetParam4)
	case smartTargetGameObjectGUID:
		return s.smartGameObjectGUIDTarget(ctx, motion, ev.row.targetParam1, ev.row.targetParam2)
	case smartTargetGameObjectDist:
		return s.smartGameObjectRangeTargets(ctx, motion, ev.row.targetParam1, 0, float32(ev.row.targetParam2), ev.row.targetParam3)
	case smartTargetPlayerRange:
		return s.smartPlayerRangeTargets(motion, float32(ev.row.targetParam1), float32(ev.row.targetParam2))
	case smartTargetPlayerDist:
		return s.smartPlayerRangeTargets(motion, 0, float32(ev.row.targetParam1))
	case smartTargetClosestCreature:
		return s.smartClosestCreatureTarget(motion, ev.row.targetParam1, ev.row.targetParam2, ev.row.targetParam3)
	case smartTargetClosestGameObject:
		return s.smartClosestGameObjectTarget(ctx, motion, ev.row.targetParam1, float32(ev.row.targetParam2))
	case smartTargetClosestPlayer:
		return s.smartClosestPlayerTarget(motion, float32(ev.row.targetParam1))
	case smartTargetOwnerOrSummoner:
		return s.smartOwnerOrSummonerTarget(motion, ev.row.targetParam1)
	case smartTargetThreatList:
		return s.smartThreatListTargets(motion, float32(ev.row.targetParam1))
	case smartTargetLootRecipients:
		return s.smartLootRecipientTargets(motion)
	case smartTargetVehiclePassenger:
		return s.smartVehiclePassengerTargets(motion, ev.row.targetParam1)
	default:
		return nil
	}
}

func (s *Server) smartIsPlayerGUID(guid uint64) bool {
	if guid == 0 {
		return false
	}
	return s.findSessionByGUID(guid) != nil
}

// smartCreatureRangeTargets is the CREATURE_RANGE (SmartScript.cpp:2584)
// / CREATURE_DISTANCE (:2605) scan: creatures of the entry (0 = any) in
// [minDist, maxDist] of the motion, excluding self, capped to a random
// maxSize when set (Trinity::Containers::RandomResize).
func (s *Server) smartCreatureRangeTargets(motion *creatureMotion, entry uint32, minDist, maxDist float32, maxSize uint32) []smartTarget {
	s.motionMu.Lock()
	var found []*creatureMotion
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil || m.GUID == motion.GUID {
			continue
		}
		// No alive check: the C++ arms test entry and range only, so a
		// corpse in range is a valid target (the dead filter lives in
		// the CLOSEST_CREATURE arm's dead flag, not here).
		if entry != 0 && m.Entry != entry {
			continue
		}
		d := distance3D(motion.X, motion.Y, motion.Z, m.X, m.Y, m.Z)
		if d < float64(minDist) || d > float64(maxDist) {
			continue
		}
		found = append(found, m)
	}
	s.motionMu.Unlock()
	if maxSize > 0 && uint32(len(found)) > maxSize {
		rand.Shuffle(len(found), func(i, j int) { found[i], found[j] = found[j], found[i] })
		found = found[:maxSize]
	}
	out := make([]smartTarget, 0, len(found))
	for _, m := range found {
		out = append(out, smartTarget{guid: m.GUID})
	}
	return out
}

// smartCreatureGUIDTarget is SMART_TARGET_CREATURE_GUID
// (SmartScript.cpp:2623): the creature with spawn GUID dbGuid (entry 0 =
// any) near the trigger/base object. C++ searches the grid around the
// trigger; Go scans the instance (same documented approximation as the
// range scans) and takes the first match.
func (s *Server) smartCreatureGUIDTarget(motion *creatureMotion, dbGuid, entry uint32) []smartTarget {
	s.motionMu.Lock()
	defer s.motionMu.Unlock()
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil {
			continue
		}
		// No alive check here either: FindCreatureNear tests guid and
		// entry only.
		if uint32(m.GUID&0x00FFFFFF) != dbGuid {
			continue
		}
		if entry != 0 && m.Entry != entry {
			continue
		}
		return []smartTarget{{guid: m.GUID}}
	}
	return nil
}

// smartInvokerPartyTargets is SMART_TARGET_INVOKER_PARTY
// (SmartScript.cpp:2557-2582): the invoker player's group members on the
// same map (same instance in Go), or the invoker alone with no group.
func (s *Server) smartInvokerPartyTargets(motion *creatureMotion, trigger uint64) []smartTarget {
	invokerSess := s.findSessionByGUID(trigger)
	if invokerSess == nil || invokerSess.player == nil {
		if trigger == 0 {
			return nil
		}
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	if invokerSess.groupID == 0 {
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	grp := s.findGroupByID(invokerSess.groupID)
	if grp == nil {
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	var out []smartTarget
	for _, member := range grp.Members {
		sess := s.findSessionByGUID(member.GUID)
		if sess == nil || sess.player == nil {
			continue
		}
		if sess.player.Map != invokerSess.player.Map || sess.player.InstanceID != invokerSess.player.InstanceID {
			continue
		}
		out = append(out, smartTarget{guid: member.GUID, isPlayer: true})
	}
	if len(out) == 0 {
		return []smartTarget{{guid: trigger, isPlayer: true}}
	}
	return out
}

// smartClosestCreatureTarget is SMART_TARGET_CLOSEST_CREATURE
// (SmartScript.cpp:2684): the nearest creature of the entry (0 = any)
// within dist (0 = 100), alive unless the dead flag is set. C++'s
// FindNearestCreature arm carries no self-exclusion; Go excludes self,
// matching the CREATURE_RANGE/DISTANCE arms and the sane reading — a
// creature's "closest creature X" target is never itself.
func (s *Server) smartClosestCreatureTarget(motion *creatureMotion, entry, dist, dead uint32) []smartTarget {
	maxDist := float64(dist)
	if maxDist == 0 {
		maxDist = 100
	}
	s.motionMu.Lock()
	var best *creatureMotion
	bestDist := maxDist
	for _, m := range s.motionMapLocked(motion.Map, motion.InstanceID) {
		if m == nil || m.GUID == motion.GUID {
			continue
		}
		if entry != 0 && m.Entry != entry {
			continue
		}
		if dead == 0 && m.Health == 0 {
			continue
		}
		d := distance3D(motion.X, motion.Y, motion.Z, m.X, m.Y, m.Z)
		if d <= bestDist {
			bestDist, best = d, m
		}
	}
	s.motionMu.Unlock()
	if best == nil {
		return nil
	}
	return []smartTarget{{guid: best.GUID}}
}

// smartGOCandidate is one gameobject found by the GO target scans, with
// its 3D distance from the motion kept so the range/closest arms can
// filter without re-scanning.
type smartGOCandidate struct {
	guid uint64
	dist float64
}

// smartGameObjectNear is the shared scan behind the GAMEOBJECT target
// arms: static spawns from the gameobject table plus runtime GOs from
// the dynamic/instance stores, deduplicated by GUID (a materialized
// static GO exists in both), within maxDist of the motion. Hidden GOs
// (despawned) are filtered after the snapshot, like
// nearestGameObjectByEntry — isGameObjectHiddenInInstance takes
// objectsMu.
func (s *Server) smartGameObjectNear(ctx context.Context, motion *creatureMotion, entry uint32, maxDist float32) []smartGOCandidate {
	seen := map[uint64]bool{}
	var found []smartGOCandidate
	add := func(guid uint64, x, y, z float32) {
		if seen[guid] {
			return
		}
		d := distance3D(motion.X, motion.Y, motion.Z, x, y, z)
		if d > float64(maxDist) {
			return
		}
		seen[guid] = true
		found = append(found, smartGOCandidate{guid: guid, dist: d})
	}
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT guid, id, position_x, position_y, position_z FROM gameobject WHERE map = ? AND (? = 0 OR id = ?) AND position_x BETWEEN ? AND ? AND position_y BETWEEN ? AND ?", motion.Map, entry, entry, float64(motion.X-maxDist), float64(motion.X+maxDist), float64(motion.Y-maxDist), float64(motion.Y+maxDist))
		if err == nil {
			for rows.Next() {
				var lowGUID, id uint32
				var x, y, z float32
				if err := rows.Scan(&lowGUID, &id, &x, &y, &z); err != nil {
					continue
				}
				add(gameObjectGUID(lowGUID, id), x, y, z)
			}
			rows.Close()
		}
	}
	s.objectsMu.RLock()
	var dyn []*dynamicGameObjectState
	for _, d := range s.dynamicGameObjects {
		dyn = append(dyn, d)
	}
	for _, d := range s.instanceGameObjects[instanceAdmissionKey{MapID: motion.Map, InstanceID: motion.InstanceID}] {
		dyn = append(dyn, d)
	}
	s.objectsMu.RUnlock()
	for _, d := range dyn {
		if d == nil || d.Map != motion.Map || d.InstanceID != motion.InstanceID {
			continue
		}
		if entry != 0 && d.Entry != entry {
			continue
		}
		add(d.GUID, d.X, d.Y, d.Z)
	}
	out := found[:0]
	for _, c := range found {
		if s.isGameObjectHiddenInInstance(motion.Map, motion.InstanceID, c.guid) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// smartGameObjectRangeTargets is SMART_TARGET_GAMEOBJECT_RANGE
// (SmartScript.cpp:2676-2699) / GAMEOBJECT_DISTANCE (:2655-2674): GOs of
// the entry (0 = any) in [minDist, maxDist] of the motion, capped to a
// random maxSize when set (Trinity::Containers::RandomResize). C++
// skips the base GO when the script runs on a GO; the Go engine only
// runs creature scripts, so there is no self GO to skip.
func (s *Server) smartGameObjectRangeTargets(ctx context.Context, motion *creatureMotion, entry uint32, minDist, maxDist float32, maxSize uint32) []smartTarget {
	near := s.smartGameObjectNear(ctx, motion, entry, maxDist)
	out := make([]smartTarget, 0, len(near))
	for _, c := range near {
		if c.dist < float64(minDist) {
			continue
		}
		out = append(out, smartTarget{guid: c.guid, isGameObject: true})
	}
	if maxSize > 0 && uint32(len(out)) > maxSize {
		rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		out = out[:maxSize]
	}
	return out
}

// smartGameObjectGUIDTarget is SMART_TARGET_GAMEOBJECT_GUID
// (SmartScript.cpp:2710-2722): the GO with spawn GUID dbGuid (entry 0 =
// any) on the creature's map. C++ looks the spawn id up in the map's
// store with no distance check (FindGameObjectNear,
// SmartScript.cpp:147-154); the runtime stores are checked the same way
// for summoned GOs.
func (s *Server) smartGameObjectGUIDTarget(ctx context.Context, motion *creatureMotion, dbGuid, entry uint32) []smartTarget {
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		var id uint32
		if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM gameobject WHERE guid = ? AND map = ?", dbGuid, motion.Map).Scan(&id); err == nil {
			if entry == 0 || id == entry {
				guid := gameObjectGUID(dbGuid, id)
				if !s.isGameObjectHiddenInInstance(motion.Map, motion.InstanceID, guid) {
					return []smartTarget{{guid: guid, isGameObject: true}}
				}
			}
			return nil
		}
	}
	s.objectsMu.RLock()
	var match *dynamicGameObjectState
	for _, d := range s.dynamicGameObjects {
		if d != nil && d.LowGUID == dbGuid && d.Map == motion.Map && d.InstanceID == motion.InstanceID {
			match = d
			break
		}
	}
	if match == nil {
		for _, d := range s.instanceGameObjects[instanceAdmissionKey{MapID: motion.Map, InstanceID: motion.InstanceID}] {
			if d != nil && d.LowGUID == dbGuid {
				match = d
				break
			}
		}
	}
	s.objectsMu.RUnlock()
	if match != nil && (entry == 0 || match.Entry == entry) {
		if !s.isGameObjectHiddenInInstance(motion.Map, motion.InstanceID, match.GUID) {
			return []smartTarget{{guid: match.GUID, isGameObject: true}}
		}
	}
	return nil
}

// smartClosestGameObjectTarget is SMART_TARGET_CLOSEST_GAMEOBJECT
// (SmartScript.cpp:2761-2767): the nearest GO of the entry (0 = any)
// within dist of the motion (0 defaults to 100, like
// FindNearestGameObject).
func (s *Server) smartClosestGameObjectTarget(ctx context.Context, motion *creatureMotion, entry uint32, dist float32) []smartTarget {
	maxDist := dist
	if maxDist == 0 {
		maxDist = 100
	}
	near := s.smartGameObjectNear(ctx, motion, entry, maxDist)
	if len(near) == 0 {
		return nil
	}
	best := near[0]
	for _, c := range near[1:] {
		if c.dist < best.dist {
			best = c
		}
	}
	return []smartTarget{{guid: best.guid, isGameObject: true}}
}

// smartPlayerRangeTargets is SMART_TARGET_PLAYER_RANGE
// (SmartScript.cpp:2723-2733) / PLAYER_DISTANCE (:2734-2742): players
// in [minDist, maxDist] of the motion on the same map and instance.
// C++ filters no dead players here, so Go doesn't either.
func (s *Server) smartPlayerRangeTargets(motion *creatureMotion, minDist, maxDist float32) []smartTarget {
	s.sessionsMu.RLock()
	var found []*session
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.Map != motion.Map || sess.player.InstanceID != motion.InstanceID {
			continue
		}
		found = append(found, sess)
	}
	s.sessionsMu.RUnlock()
	var out []smartTarget
	for _, sess := range found {
		d := distance3D(motion.X, motion.Y, motion.Z, sess.player.X, sess.player.Y, sess.player.Z)
		if d < float64(minDist) || d > float64(maxDist) {
			continue
		}
		out = append(out, smartTarget{guid: sess.player.GUID, isPlayer: true})
	}
	return out
}

// smartClosestPlayerTarget is SMART_TARGET_CLOSEST_PLAYER
// (SmartScript.cpp:2767-2772): WorldObject::SelectNearestPlayer(dist),
// alive-only (NearestPlayerInObjectRangeCheck, GridNotifiers.h:1398-1412:
// IsAlive + IsWithinDistInMap, shrinking to the nearest). Dist is
// target_param1; 0 means only a player at the exact spot matches.
// TDB 26091: 11 rows ride this strip (29 x7, 33 x4; dist 5/10/50/200).
func (s *Server) smartClosestPlayerTarget(motion *creatureMotion, dist float32) []smartTarget {
	s.sessionsMu.RLock()
	var found []*session
	for sess := range s.sessions {
		if sess == nil || !sess.worldReady.Load() || sess.player == nil {
			continue
		}
		if sess.player.Map != motion.Map || sess.player.InstanceID != motion.InstanceID {
			continue
		}
		found = append(found, sess)
	}
	s.sessionsMu.RUnlock()
	var best *session
	bestDist := float64(dist)
	for _, sess := range found {
		if sess.isDeadOrGhost() {
			continue
		}
		d := distance3D(motion.X, motion.Y, motion.Z, sess.player.X, sess.player.Y, sess.player.Z)
		if d <= bestDist {
			bestDist, best = d, sess
		}
	}
	if best == nil {
		return nil
	}
	return []smartTarget{{guid: best.player.GUID, isPlayer: true}}
}

// smartHostileRandomTarget is SMART_TARGET_HOSTILE_RANDOM
// (SmartScript.cpp:2554-2565): UnitAI::SelectTarget(Random, 0,
// maxDist, playerOnly) over the creature's threat list
// (DefaultTargetSelector, UnitAI.cpp:254-278: maxDist 0 = no limit,
// playerOnly gates on TYPEID_PLAYER, the combat-range leg is a 3D
// distance strictly under maxDist + both combat reaches). The
// powerType leg (PowerUsersSelector) has no Go twin — exactly one
// TDB row carries a nonzero powerType (40, out of the Powers enum,
// matching nothing in C++ either), so it resolves to no target.
// Candidates must resolve to a live unit (the threat-selection
// precedent in threat.go); at DEATH time the list is already empty
// in both engines (Unit::setDeathState runs CombatStop, which clears
// own threat, before AI()->JustDied), so the single DEATH row
// (31411) credits nobody either way. TDB 26091: 1113 rows ride the
// arm (CAST 1108, ATTACK_START 4, CALL_KILLEDMONSTER 1).
func (s *Server) smartHostileRandomTarget(motion *creatureMotion, maxDist float32, playerOnly bool, powerType uint32) []smartTarget {
	if motion == nil || motion.ThreatMgr == nil || powerType != 0 {
		return nil
	}
	myReach := motion.CombatReach
	if myReach <= 0 {
		myReach = 1.5
	}
	resolve := func(guid uint64) (x, y, z, reach float32, alive bool, ok bool) {
		if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
			if sess.player.Map != motion.Map || sess.player.InstanceID != motion.InstanceID {
				return 0, 0, 0, 0, false, false
			}
			reach = sess.player.CombatReach
			if reach <= 0 {
				reach = 1.5
			}
			return sess.player.X, sess.player.Y, sess.player.Z, reach, !sess.isDeadOrGhost(), true
		}
		tm := s.findCreatureMotion(motion.Map, motion.InstanceID, guid)
		if tm == nil {
			return 0, 0, 0, 0, false, false
		}
		s.motionMu.Lock()
		x, y, z, reach, health := tm.X, tm.Y, tm.Z, tm.CombatReach, tm.Health
		s.motionMu.Unlock()
		if reach <= 0 {
			reach = 1.5
		}
		return x, y, z, reach, health > 0, true
	}
	var candidates []uint64
	for _, guid := range motion.ThreatMgr.EntryGUIDs() {
		if playerOnly && s.findSessionByGUID(guid) == nil {
			continue
		}
		x, y, z, reach, alive, ok := resolve(guid)
		if !ok || !alive {
			continue
		}
		if maxDist > 0 && distance3D(motion.X, motion.Y, motion.Z, x, y, z) >= float64(maxDist+myReach+reach) {
			continue
		}
		candidates = append(candidates, guid)
	}
	if len(candidates) == 0 {
		return nil
	}
	pick := candidates[rand.Intn(len(candidates))]
	return []smartTarget{{guid: pick, isPlayer: s.smartIsPlayerGUID(pick)}}
}

// smartOwnerOrSummonerTarget is SMART_TARGET_OWNER_OR_SUMMONER
// (SmartScript.cpp:2774-2806): the motion's charmer, else owner, else
// creator/summoner (GetCharmerOrOwnerGUID, then the TempSummon summoner
// leg, then GetCreatorGUID — the summon leg is folded into
// CreatedByGUID, seeded by summonTempCreature). With useCharmerOrOwner
// the owner's charmer/owner is returned instead. A player owner with no
// charmer resolves to nothing, like C++'s GetUnit on an empty GUID.
func (s *Server) smartOwnerOrSummonerTarget(motion *creatureMotion, useCharmerOrOwner uint32) []smartTarget {
	guid := motion.CharmerGUID
	if guid == 0 {
		guid = motion.OwnerGUID
	}
	if guid == 0 {
		guid = motion.CreatedByGUID
	}
	if guid == 0 {
		return nil
	}
	if useCharmerOrOwner != 0 {
		if om := s.findCreatureMotion(motion.Map, motion.InstanceID, guid); om != nil {
			chained := om.CharmerGUID
			if chained == 0 {
				chained = om.OwnerGUID
			}
			if chained == 0 {
				chained = om.CreatedByGUID
			}
			if chained == 0 {
				return nil
			}
			guid = chained
		} else {
			return nil
		}
	}
	if s.findSessionByGUID(guid) != nil {
		return []smartTarget{{guid: guid, isPlayer: true}}
	}
	if s.findCreatureMotion(motion.Map, motion.InstanceID, guid) != nil {
		return []smartTarget{{guid: guid}}
	}
	return nil
}

// smartThreatListTargets is SMART_TARGET_THREAT_LIST
// (SmartScript.cpp:2808-2815): every unit on the owner's threat list
// within maxDist (target_param1 = hostilRandom.maxDist, Mgr.h:1274-1279;
// 0 = no limit). CanHaveThreatList's Go twin is a live threat manager;
// distances are plain 3D, like the other smart target range arms.
// Unresolvable victims (no live motion or session to measure from)
// are skipped.
func (s *Server) smartThreatListTargets(motion *creatureMotion, maxDist float32) []smartTarget {
	if motion == nil || motion.ThreatMgr == nil {
		return nil
	}
	var out []smartTarget
	for _, guid := range motion.ThreatMgr.EntryGUIDs() {
		if maxDist > 0 {
			var x, y, z float32
			if tm := s.findCreatureMotion(motion.Map, motion.InstanceID, guid); tm != nil {
				x, y, z = tm.X, tm.Y, tm.Z
			} else if sess := s.findSessionByGUID(guid); sess != nil && sess.player != nil {
				x, y, z = sess.player.X, sess.player.Y, sess.player.Z
			} else {
				continue
			}
			if distance3D(motion.X, motion.Y, motion.Z, x, y, z) > float64(maxDist) {
				continue
			}
		}
		out = append(out, smartTarget{guid: guid, isPlayer: s.smartIsPlayerGUID(guid)})
	}
	return out
}

// smartLootRecipientTargets is SMART_TARGET_LOOT_RECIPIENTS
// (SmartScript.cpp:2830-2845): the tap group's members on the same map
// and instance when the creature was tapped by a group
// (GetLootRecipientGroup, Creature.cpp:1321-1326 — the m_lootRecipientGroup
// twin is motion.TapGroupID), else the single tap player
// (GetLootRecipient). The single-recipient leg carries no IsInMap gate
// in C++; the tap player is returned whenever the session resolves.
func (s *Server) smartLootRecipientTargets(motion *creatureMotion) []smartTarget {
	if motion == nil {
		return nil
	}
	s.motionMu.Lock()
	tapPlayer, tapGroup := motion.TapPlayerGUID, motion.TapGroupID
	mapID, instanceID := motion.Map, motion.InstanceID
	s.motionMu.Unlock()
	if tapGroup != 0 {
		if grp := s.findGroupByID(tapGroup); grp != nil {
			var out []smartTarget
			for _, member := range grp.Members {
				sess := s.findSessionByGUID(member.GUID)
				if sess == nil || sess.player == nil {
					continue
				}
				if sess.player.Map != mapID || sess.player.InstanceID != instanceID {
					continue
				}
				out = append(out, smartTarget{guid: member.GUID, isPlayer: true})
			}
			return out
		}
	}
	if tapPlayer != 0 {
		if sess := s.findSessionByGUID(tapPlayer); sess != nil {
			return []smartTarget{{guid: tapPlayer, isPlayer: true}}
		}
	}
	return nil
}

// smartFirstActiveAuraSpellID returns one live player aura id, or 0 — the
// peek the REMOVEAURASFROMSPELL spell-0 sweep needs to drain every aura
// without snapshotting the session's aura map.
func (s *session) smartFirstActiveAuraSpellID() uint32 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	for id := range s.activeAuras {
		return id
	}
	return 0
}
