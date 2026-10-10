package world

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/config"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/crypto"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/iplocation"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/version"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocoltrace"
)

const (
	opcodePong          uint16 = uint16(protocol.OpcodeSMSG_PONG)
	opcodeAuthChallenge uint16 = uint16(protocol.OpcodeSMSG_AUTH_CHALLENGE)
	opcodeAuthSession   uint32 = uint32(protocol.OpcodeCMSG_AUTH_SESSION)
	opcodeAuthResponse  uint16 = uint16(protocol.OpcodeSMSG_AUTH_RESPONSE)
	authOK              byte   = 12
	authReject          byte   = 14
	authUnknownAccount  byte   = 21
	authFailed          byte   = 13
	authBanned          byte   = 28
	authUnavailable     byte   = 16
	authWaitQueue       byte   = 27
	// WorldSocket.cpp:487 answers a realm-ID mismatch with
	// REALM_LIST_REALM_NOT_FOUND (SharedDefines.h:3400), not
	// AUTH_LOGIN_SERVER_NOT_FOUND.
	realmListRealmNotFound byte = 39
)

// overspeedPingWindow mirrors the fixed 27 second threshold in
// WorldSocket::HandlePing before the over-speed ping counter resets.
const overspeedPingWindow = 27 * time.Second

var applicationStartTime = time.Now()

func gameTimeMS() uint32 { return uint32(time.Since(applicationStartTime) / time.Millisecond) }

type Server struct {
	AuthStore               *database.Store
	CharactersStore         *database.Store
	WorldStore              *database.Store
	Logger                  *slog.Logger
	worldTimeStartedAt      time.Time
	TraceRecorder           *protocoltrace.Recorder
	RealmID                 uint32
	allowedSecurityLevel    uint8
	Config                  config.Config
	clientCacheVersion      uint32
	Features                *Features
	Data                    *wotlk.Store
	ipLocations             *iplocation.Store
	worldstatesMu           sync.RWMutex
	worldstates             map[uint32]uint64
	sessionsMu              sync.RWMutex
	sessions                map[*session]struct{}
	autoBalanceMu           sync.RWMutex
	autoBalanceMaps         map[autoBalanceInstanceKey]*autoBalanceMapInfo
	autoBalanceCreatures    map[uint64]*autoBalanceCreatureInfo
	autoBalanceLFG          []wotlk.LFGDungeon
	autoBalanceBosses       map[uint32]struct{}
	autoBalanceBossesLoaded bool
	// spellBonusOnce/spellBonus mirror SpellMgr::mSpellBonusMap
	// (SpellMgr.cpp:1888): spell_bonus_data rows cached on first use by
	// Server.spellBonusData, for the damage-done coefficient leg.
	spellBonusOnce sync.Once
	spellBonus     map[uint32]spellBonusEntry
	queuedSessions []*session
	// disconnectTimes mirrors World's m_disconnects (World.cpp:3114): account id
	// -> unix time of last non-queued session removal, consulted by the
	// DisconnectToleranceInterval queue-skip arm.
	disconnectTimes    map[uint32]int64
	playerLimit        uint32
	closed             atomic.Bool
	ticketsEnabled     atomic.Bool
	objectsMu          sync.RWMutex
	characterGUIDMu    sync.Mutex
	characterGUIDNext  uint64
	characterGUIDReady bool
	// mailIDMu/mailIDNext mirror ObjectMgr::_mailId (ObjectMgr.cpp:232, 7319,
	// 7366-7374): the in-memory mail-ID generator seeded once from
	// MAX(id)+1 at startup, so concurrent sends can never race on the same
	// row id the way per-send MAX(id)+1 queries could.
	mailIDMu    sync.Mutex
	mailIDNext  uint32
	mailIDReady bool
	// itemGUIDMu/itemGUIDNext mirror sObjectMgr's item GUID generator
	// (ObjectGuidGenerator<HighGuid::Item>, seeded from MAX(guid)+1 at
	// ObjectMgr::SetHighestGuids, ObjectMgr.cpp:7293-7301): the in-memory
	// counter hands out consecutive item_instance guids, so concurrent item
	// creates can never race on the same row guid the way per-create
	// MAX(guid)+1 queries could.
	itemGUIDMu                sync.Mutex
	itemGUIDNext              uint32
	itemGUIDReady             bool
	instanceIDMu              sync.Mutex
	reservedInstanceIDs       map[uint32]struct{}
	instanceAdmissionMu       sync.Mutex
	pendingInstanceAdmissions map[instanceAdmissionKey]uint32
	instanceEncounters        map[instanceAdmissionKey]map[uint64]struct{}
	inventoryMu               sync.Mutex
	hiddenGameObjects         map[uint64]struct{}
	auctionGetAllMu           sync.Mutex
	auctionGetAllThrottle     map[uint64]int64
	instanceHiddenGameObjects map[instanceAdmissionKey]map[uint64]struct{}
	dynamicGameObjects        map[uint64]*dynamicGameObjectState
	instanceGameObjects       map[instanceAdmissionKey]map[uint64]*dynamicGameObjectState
	destructibleHealth        map[destructibleKey]goDestructibleHealth
	nextDynamicGOGUID         uint32
	dynamicSpellObjects       map[uint64]*dynamicSpellObjectState
	nextDynamicSpellGUID      uint32
	wsgMu                     sync.RWMutex
	wsgState                  map[uint32]*wsgBattlegroundState
	abMu                      sync.RWMutex
	abState                   map[uint32]*abBattlegroundState
	eotsMu                    sync.RWMutex
	eotsState                 map[uint32]*eotsBattlegroundState
	avMu                      sync.RWMutex
	avState                   map[uint32]*avBattlegroundState
	saMu                      sync.RWMutex
	saState                   map[uint32]*saBattlegroundState
	icMu                      sync.RWMutex
	icState                   map[uint32]*icBattlegroundState
	arenaMu                   sync.RWMutex
	arenaState                map[uint32]*arenaBattlegroundState
	arenaTickLast             time.Time
	avTickLast                time.Time
	eotsTickLast              time.Time
	saTickLast                time.Time
	wgMu                      sync.RWMutex
	wgState                   *wgBattlegroundState
	totemMu                   sync.RWMutex
	activeTotems              map[uint64][4]*activeTotem
	nextDynamicCreatureGUID   uint32
	nextPetGUID               map[instanceAdmissionKey]uint32
	creatureAuras             map[creatureAuraKey]map[uint32]struct{}
	auraMu                    sync.Mutex
	activeCreatureAuras       map[creatureAuraKey]map[uint32]*activeAura
	singleCastMu              sync.Mutex
	singleCastAuras           map[uint64][]singleCastEntry
	petAuraStackMu            sync.Mutex
	petAuraStackCache         *petAuraStackCache
	channelsMu                sync.RWMutex
	channels                  map[string]*worldChannel
	vehicleMu                 sync.RWMutex
	vehicleKits               map[uint64]*VehicleKit
	instanceVehicleKits       map[vehicleKitKey]*VehicleKit
	vehicleSeatAddons         map[uint32]*VehicleSeatAddon
	vehicleAccessories        map[uint32][]VehicleAccessory
	groupsMu                  sync.RWMutex
	groups                    map[uint64]*groupState // groupID -> groupState
	motionMu                  sync.Mutex
	instanceCreatureMotion    map[instanceAdmissionKey]map[uint64]*creatureMotion
	creatureRespawns          map[uint32]creatureRespawn
	instanceCreatureRespawns  map[instanceAdmissionKey]map[uint32]creatureRespawn
	transportMu               sync.Mutex
	transports                map[uint32]*continentTransport
	lootMu                    sync.Mutex
	creatureLoot              map[lootObjectKey]*activeLootState
	// goSkillupLists mirrors GameObject::m_SkillupList (GameObject.h:190-199):
	// the players (low GUIDs) that already took their one lockpicking
	// skill-up from a given GO (Spell::EffectOpenLock, SpellEffects.cpp:2042-2055).
	// C++ clears the list when the GO respawns (GameObject.cpp:573); Go's
	// respawn-equivalent is the chest loot re-roll after its respawn delay
	// (openGameObjectLoot), which clears the entry the same way.
	goSkillupLists     map[lootObjectKey]map[uint32]struct{}
	creatureLootOwners map[lootObjectKey]lootOwnerState
	// storedContainerLoot mirrors LootItemStorage::_lootItemStore
	// (Loot/LootItemStorage.cpp): persisted container (item) loot keyed by
	// the container item's instance GUID, loaded at startup and kept in
	// sync by takes, money looting and item destruction.
	storedContainerLoot map[uint64]*storedContainerLoot
	// generatedContainerLoot mirrors Item::m_lootGenerated for containers
	// (Player.cpp:8683-8685, Loot/LootItemStorage.cpp:133-185): once a
	// container's loot is generated its stack cannot be split
	// (Player::SplitItem, Player.cpp:13047). The store above only tracks
	// non-empty loot, but C++ sets m_lootGenerated even for an empty roll,
	// so generation needs its own marker, keyed by item instance GUID.
	generatedContainerLoot map[uint64]struct{}
	// pickpocketLootRestore mirrors Creature::_pickpocketLootRestore
	// (Creature.h): the game-time after which the creature instance's
	// pickpocket loot may be generated again, keyed per loot object.
	pickpocketLootRestore  map[lootObjectKey]int64
	vendorMu               sync.Mutex
	vendorStock            map[vendorStockKey]*vendorStockState
	statsMu                sync.RWMutex
	creatureStatsCache     map[uint32]creatureStats
	groupRolls             map[lootRollKey]*activeGroupRoll
	spiritWaveMu           sync.Mutex
	lastSpiritWave         time.Time
	lastCorpseExpiry       time.Time
	spiritReviveQueue      map[uint64]uint64            // playerGUID -> spiritGuideGUID
	spiritResurrectPending map[uint64]spiritWavePending // playerGUID -> wave visual pass (resurrect due 500ms later)
	creatureTextMgr        *creatureTextMgr
	wardenCheckMgr         *wardenCheckMgr
	spellChainMu           sync.RWMutex
	spellChainLoaded       bool
	prevSpellInChain       map[uint32]uint32
	nextSpellInChain       map[uint32]uint32
	firstSpellInChain      map[uint32]uint32
	spellCustomAttrMu      sync.RWMutex
	spellCustomAttrLoaded  bool
	spellCustomAttr        map[uint32]uint32
	enchantProcAttrMu      sync.RWMutex
	enchantProcAttrLoaded  bool
	enchantProcAttrs       map[uint32]uint32
	itemTemplateMu         sync.RWMutex
	itemTemplates          map[uint32]itemTemplateClassInfo
	itemHyperlinkMu        sync.RWMutex
	itemHyperlinkTemplates map[uint32]itemHyperlinkTemplate
	itemStoreTemplateMu    sync.RWMutex
	itemStoreTemplates     map[uint32]itemStoreTemplateInfo
	terrainMu              sync.Mutex
	terrainTiles           map[uint64][]terrainSpawn
	terrainTileKnown       map[uint64]bool
	terrainModels          map[string]*terrainModel
	weatherMu              sync.Mutex
	weather                map[uint32]*zoneWeather
	stopOnce               sync.Once
}

type session struct {
	server      *Server
	conn        net.Conn
	authSeed    [4]byte
	crypt       *crypto.AuthCrypt
	authed      bool
	accountID   uint32
	accountName string
	// recruiterID mirrors WorldSession::recruiterId (WorldSession.h:580) —
	// the account.recruiter column the C++ auth handshake SELECTs into the
	// session (WorldSocket.cpp:267). It feeds the
	// SPELL_EFFECT_SUMMON_RAF_FRIEND CheckCast gate (Spell.cpp:5936-5941).
	recruiterID                  uint32
	security                     uint8
	raidMapDifficulty            uint8
	raidMapDifficultyInitialized bool
	accountExpansion             uint8
	superseded                   bool
	muteTime                     int64
	speakTime                    int64
	speakCount                   uint32
	// addonCmdEcho mirrors AddonChannelCommandHandler::echo (Chat.h:175): the
	// 4-byte request correlator from a "TrinityCore\t" addon command frame
	// (Chat.cpp:860-899). addonCmdHadAck tracks the 'a' response; addonCmdFailed
	// approximates HasSentErrorMessage() via error notifications sent while the
	// remote-console command runs; addonCmdActive routes sendSysMessage output
	// through the framed whisper protocol for the command's duration.
	addonCmdEcho   [4]byte
	addonCmdHadAck bool
	addonCmdFailed bool
	addonCmdActive bool
	// calendarEventCooldown mirrors WorldSession's calendar event creation
	// cooldown (CalendarHandler.cpp:262-265): unix time before which a new
	// calendar event or copy may not be created
	// (CALENDAR_CREATE_EVENT_COOLDOWN = 5s).
	calendarEventCooldown int64
	gmChat                bool
	// gmVisibilityDetect mirrors Player::m_serverSideVisibilityDetect for
	// SERVERSIDE_VISIBILITY_GM (Object.cpp WorldObject::CanSeeOrDetect, the
	// "GM visibility off or hidden NPC" arm): nonzero (the account security,
	// as SetGameMaster(true) stores GetSecurity()) means the session sees
	// serverside-hidden creatures (spirit healers/guides, GHOST_VISIBILITY
	// creatures); SEC_PLAYER (0) means they stay hidden. SetGMVisible(true)
	// resets it to SEC_PLAYER while PLAYER_EXTRA_GM_ON stays set — that
	// reset is what re-hides hidden NPCs after `.gm visible on`
	// (Player::SetGMVisible, Player.cpp:2504).
	gmVisibilityDetect uint8
	// autoReplyMsg mirrors Player::autoReplyMsg (Player.h:939): the AFK/DND
	// auto-reply message announced to whispering players (Player.cpp:21050).
	autoReplyMsg            string
	gmMessage               bool
	twoSideChat             bool
	twoSideWhoList          bool
	whoSeeAllSecurityLevels bool
	twoSideAddFriend        bool
	allowGMFriend           bool
	// ignoreIdleTimeout caches rbac::RBAC_PERM_IGNORE_IDLE_CONNECTION (RBAC.h:60)
	// at auth time so the idle-connection sweeper needs no per-tick DB lookup,
	// mirroring WorldSession::Update's HasPermission check (WorldSession.cpp:290).
	ignoreIdleTimeout bool
	legitimate        map[uint64]struct{}
	characterNames    map[uint64]enumCharacter
	mounts            *MountState
	playerGUID        uint64
	playerLoading     bool
	playerLoaded      bool
	inQueue           bool
	pendingAddonInfo  []byte
	worldReady        atomic.Bool
	worldInstance     atomic.Uint64
	worldReadyGM      atomic.Bool
	// timeoutTime mirrors WorldSession::m_timeOutTime (WorldSession.h:574): unix
	// time after which the connection counts as idle
	// (WorldSession::IsConnectionIdle, WorldSession.cpp:741), armed by
	// ResetTimeOutTime (WorldSession.cpp:733).
	timeoutTime                  atomic.Int64
	farTeleportPending           bool
	farTeleportOriginOrientation float32
	farTeleportOriginMap         uint32
	farTeleportOriginInstanceID  uint32
	nearTeleportPending          bool
	nearTeleportDest             nearTeleportDestination
	initialLoginPending          bool
	initialLoginFirst            bool
	temporaryUnsummonedPetNumber uint32
	// petRenameAllowed tracks UNIT_CAN_BE_RENAMED (UnitDefines.h:116) per pet
	// GUID: set by SPELL_EFFECT_ALLOW_RENAME_PET (159,
	// SpellEffects.cpp:5525) and consumed by the pet rename opcode gate
	// (PetHandler.cpp:601); cleared after a successful rename
	// (PetHandler.cpp:624). Freshly tamed pets carry the flag implicitly via
	// the character_pet.renamed == 0 arm (Pet.cpp:267/841).
	petRenameAllowed             map[uint64]bool
	randomBGWinner               bool
	bgData                       battlegroundLoginData
	instanceLockTimes            map[uint32]int64
	player                       *playerState
	visiblePlayersMu             sync.Mutex
	visiblePlayers               map[uint64]struct{}
	visibleTransportPassengersMu sync.Mutex
	visibleTransportPassengers   map[uint64]uint64
	logoutAt                     time.Time
	logoutFlagsApplied           bool
	gameTimeStartedAt            time.Time
	writeMu                      sync.Mutex
	movementMu                   sync.RWMutex
	captureUpdatePackets         bool
	traceStatePrefix             string
	loginCreateBlock             []byte
	capturedUpdatePackets        []*protocol.Packet
	selection                    uint64
	auras                        map[uint32]struct{}
	auraSlots                    map[uint32]uint8
	activeAuras                  map[uint32]*activeAura
	// itemSetEff mirrors Player::ItemSetEff (Player.h): the item sets with
	// at least one equipped piece, keyed by set id — the set bonus spell
	// tracking AddItemsSetItem/RemoveItemsSetItem (Item.cpp) maintain.
	itemSetEff                map[uint32]*itemSetEffect
	spellMods                 [spellModOpCount][]*spellModifier // Player::m_spellMods, guarded by castMu
	spellModTaking            []*spellModTakingContext          // Player::m_spellModTakingSpell as a stack, guarded by castMu
	ownerPetAuraMu            sync.Mutex
	ownerPetAuraSources       map[ownerPetAuraKey]ownerPetAuraSource
	ownerPetAuraSourcesLoaded bool
	scale                     float32
	emoteState                uint32
	playerLocked              bool
	rooted                    bool
	// forcedSpeedChanges mirrors Player::m_forced_speed_changes (Player.h:2023):
	// pending forced-speed-change ACKs per UnitMoveType, incremented for every
	// real SMSG_FORCE_*_SPEED_CHANGE sent.
	forcedSpeedChanges [9]uint8
	// gmSpeedRates is the Go analog of Unit::m_speed_rate (Unit.cpp:376):
	// the per-UnitMoveType rate multiplier, 0 when unset. The aura system
	// writes through the same slot (C++ UpdateSpeed ends in SetSpeedRate),
	// so a .modify speed override is transient by design — the next
	// aura-driven recompute for that type overwrites it, exactly like C++.
	gmSpeedRates [9]float32
	attackTarget uint64
	duelPartner  uint64
	// duelInitiator is the DuelInfo::Initiator analog (Player.h:239-248): true
	// when this session cast the duel challenge (Spell::EffectDuel). Only the
	// challenged party may accept (DuelHandler.cpp:27).
	duelInitiator bool
	// duelCountdown is the DUEL_STATE_COUNTDOWN analog: the accept was
	// processed and the 3s StartTime is ticking (DuelHandler.cpp:44-49).
	duelCountdown    bool
	duelArbiterX     float32
	duelArbiterY     float32
	duelArbiterZ     float32
	duelOutOfBounds  time.Time
	lastSwing        time.Time
	lastOffhandSwing time.Time
	lastRangedSwing  time.Time
	// swingErrorMsg is the m_swingErrorMsg latch (Player.cpp): 0 = no
	// swing error sent, 1 = SMSG_ATTACK_SWING_NOT_IN_RANGE sent,
	// 2 = SMSG_ATTACK_SWING_BAD_FACING sent. A new error packet goes out
	// only when the state changes; a landed swing resets it to 0.
	swingErrorMsg             uint8
	autoRepeatSpell           uint32
	autoRepeatTarget          uint64
	isMoving                  bool
	isFalling                 bool
	isFallingFar              bool // MOVEMENTFLAG_FALLING_FAR (UnitDefines.h:236): the STUCK-spell movement exemptions key on this flag exactly (Spell.cpp:3814, 5319)
	lastMovementInfo          movementInfo
	lastMovementInfoSet       bool
	transportContactProbeGUID uint64
	transportContactProbeAt   time.Time
	lastFallZ                 float32
	lastFallTime              uint32
	isSwimming                bool
	breathTimer               int32
	lastBreathTick            time.Time
	inDarkWater               bool
	fatigueTimer              int32
	lastFatigueTick           time.Time
	lastRegenTick             time.Time
	lastRestBonusUpdate       time.Time
	innTriggerID              uint32
	lastCastTime              time.Time
	lastCombatTime            time.Time
	contestedPVPEnd           time.Time
	loadedCorpseBones         bool
	pvpEnd                    time.Time
	pvpHostile                bool
	areaID                    uint32
	lastZoneUpdate            time.Time
	logoutHook                bool
	questStatusSent           bool
	timeSyncNextCounter       uint32
	timeSyncDue               time.Time
	timeSyncClockDelta        int64
	timeSyncClockDeltaQueue   []timeSyncSample
	timeSyncPending           map[uint32]uint32
	gossip                    *gossipMenuState
	gossipClosed              bool
	channels                  map[string]struct{}
	channelOrder              []string // join order of channels: reference Player::GetJoinedChannels is a join-ordered std::list, and GetChannelForPlayerByNamePart returns the first joined prefix match
	tutorials                 [8]uint32
	tutorialsInDB             bool
	unreadMails               uint32
	nextMailDelivery          int64
	activeLoot                *activeLootState
	trade                     *playerTradeState
	diminishing               [DiminishingMax]diminishingReturn
	procICD                   map[uint32]time.Time
	triggeredNoProcEvents     int
	// attackPowerOfArmorApplied tracks the SPELL_AURA_MOD_ATTACK_POWER_OF_ARMOR
	// (285) total currently folded into AttackPower/RangedAttackPower, so the
	// 30s periodic tick (SpellAuraEffects.cpp:5635-5641) can apply amount
	// recomputes as deltas without a full stat recalc.
	attackPowerOfArmorApplied int32
	castProcHitMask           uint32 // Spell::m_hitMask (Spell.cpp:2603) — per-target proc hit masks ORed in as the cast's damage/heal targets resolve; reset at each cast entry (finishSpellCast, the triggered-cast path, the delayed arrival closure) with save/restore for nested casts, mirroring the per-Spell-object lifetime
	procDeep                  int    // Unit::m_procDeep (Unit.h:1729) — elevated while the player's auras are evaluated for a proc event (Unit::TriggerAurasProcOnEvent, Unit.cpp:10424-10448); a hit on a proc-deep unit cannot trigger further procs (Unit::CanProc, Unit.h:1609; Spell.cpp:2441)
	extraAttacks              uint32 // Unit::m_extraAttacks (Unit.h:802) — banked SPELL_EFFECT_ADD_EXTRA_ATTACKS swings
	comboPoints               uint8  // Unit::m_comboPoints (Unit.h) — banked combo points on comboTargetGUID
	comboTargetGUID           uint64 // Unit::m_comboTarget (Unit.h) — unit the combo points are banked against
	lastPotionId              uint32 // Player::m_lastPotionId (Player.h:2337) — last potion/cooldown-on-event item used in combat; blocks the next potion cast
	guildInvitedID            uint32
	guildInviterGUID          uint64
	groupID                   uint64 // GUID of the group this player is in (0 = no group)
	pendingGroupLeader        uint64 // GUID of the player who invited us (0 = no invite pending)
	lastStreamX               float32
	lastStreamY               float32
	lastStreamZ               float32
	latency                   atomic.Uint32
	lastPing                  time.Time
	overSpeedPings            uint32
	deathExpireTime           int64
	deathTimer                time.Time
	pvpDeath                  bool // PLAYER_EXTRA_PVP_DEATH analog (Player.h:450/971): Unit::Kill records it, CreateCorpse consumes it
	resurrection              *resurrectionData
	earnedAchievements        map[uint32]uint32
	criteriaProgress          map[uint32]*criteriaProgressState
	timedCriteria             map[uint32]*time.Timer
	inFlight                  bool
	buyback                   [12]*buybackSlot
	currentBuybackSlot        uint8
	arenaTeamInvited          uint32
	bgQueues                  [2]bgQueueEntry
	afkReporters              map[uint64]struct{}
	afkReportedCount          uint8     // Player::CanReportAfkDueToLimit (Player.cpp:22514): 15 reports per 5 minutes
	afkReportWindowEnd        time.Time // lazy reset point for afkReportedCount (== Player::UpdateAfkReport, Player.cpp:20715)
	targetGlyphSlot           uint8
	activeCast                *activeCastState
	nextSwing                 *queuedNextSwing // CURRENT_MELEE_SPELL slot (Unit.h:572), castMu-guarded like activeCast
	summonExpire              time.Time
	summonerGUID              uint64
	summonMap                 uint32
	summonX                   float32
	summonY                   float32
	summonZ                   float32
	summonO                   float32
	summonLocSet              bool
	activeChannel             *activeChannelState
	fishingBobberGUID         uint64 // live fishing bobber: Go arms no activeChannel for fishing, so the channel-cancel paths clear it via cancelFishingBobber
	runes                     *dkRuneState
	castMu                    sync.Mutex
	// redirectThreatRegistry mirrors ThreatManager::_redirectRegistry
	// (ThreatManager.h): spell id -> redirect victim guid -> percent,
	// populated by SPELL_EFFECT_REDIRECT_THREAT (Spell::EffectRedirectThreat,
	// SpellEffects.cpp:5411) and consumed by the redirect leg of
	// ThreatManager::AddThreat (ThreatManager.cpp:346-373). C++ keeps the
	// registry on the unit's own ThreatManager; Go threat managers are
	// creature-side only, so the caster-side registry lives on the session.
	// redirectThreatInfo is the flattened, percent-capped view
	// (ThreatManager::UpdateRedirectInfo, ThreatManager.cpp:829-845).
	// redirectMu is a leaf lock: never held across motionMu/castMu.
	redirectMu             sync.Mutex
	redirectThreatRegistry map[uint32]map[uint64]uint32
	redirectThreatInfo     []redirectThreatTarget
	schoolLockouts         map[uint32]int64
	gcdCooldowns           map[uint32]int64 // per-category Global Cooldown expiry Unix-ms (SpellHistory::_globalCooldowns)
	pendingBindInstanceID  uint64
	pendingBindMapID       uint32
	pendingBindDiff        uint32
	pendingBindTimer       uint32
	pendingBindMu          sync.Mutex
	sharingQuestID         uint32
	sharingQuestSender     uint64
	warden                 *wardenSession
	playerStateMu          sync.RWMutex
}

type activeCastState struct {
	CastID       uint8
	SpellID      uint32
	Timer        *time.Timer
	Cancelled    bool
	StartAt      time.Time
	CastTimeMs   uint32
	Pushbacks    int
	InterruptFlg uint32
	// HitTriggers is the PrepareTriggersExecutedOnHit snapshot (Spell.cpp:8176):
	// ADD_TARGET_TRIGGER auras present on the caster at cast completion, for
	// the on-hit trigger consumer.
	HitTriggers []spellHitTrigger
}

func (s *Server) playerSessionForGUID(guid uint64) *session {
	if s == nil || guid == 0 {
		return nil
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	for current := range s.sessions {
		if current != nil && current.playerGUID == guid && current.player != nil && current.player.GUID == guid {
			return current
		}
	}
	return nil
}

type bgQueueEntry struct {
	Active       bool
	BgTypeID     uint32
	InstanceID   uint32
	JoinTime     time.Time
	Status       uint32
	ArenaType    uint8
	IsArena      bool
	IsRated      bool
	ArenaFaction uint8
	MapID        uint32
	StartTime    time.Time
}

type battlegroundLoginData struct {
	InstanceID uint32
	Team       uint16
	JoinX      float32
	JoinY      float32
	JoinZ      float32
	JoinO      float32
	JoinMap    uint32
	TaxiStart  uint32
	TaxiEnd    uint32
	MountSpell uint32
}

type buybackSlot struct {
	ItemGUID  uint64
	ItemEntry uint32
	Count     uint32
	Price     uint32
	Timestamp uint32
}

type buybackEntry = buybackSlot

type account struct {
	ID          uint32
	SessionKey  []byte
	LastIP      string
	Locked      bool
	LockCountry string
	OS          string
	MuteTime    int64
	Security    uint8
	Expansion   uint8
}

func NewServer(stores *database.Set, logger *slog.Logger, realmID uint32, settings ...config.Config) *Server {
	c := config.Default()
	if len(settings) != 0 {
		c = settings[0]
	}
	server := &Server{AuthStore: stores.Auth, CharactersStore: stores.Characters, WorldStore: stores.World, Logger: logger, RealmID: realmID, Config: c, Features: NewFeatures(c, stores, logger), Data: wotlk.NewStore(filepath.Join(c.GameDataDir, "dbc")), sessions: make(map[*session]struct{}), playerLimit: c.PlayerLimit, disconnectTimes: make(map[uint32]int64), hiddenGameObjects: make(map[uint64]struct{}), auctionGetAllThrottle: make(map[uint64]int64), dynamicGameObjects: make(map[uint64]*dynamicGameObjectState), dynamicSpellObjects: make(map[uint64]*dynamicSpellObjectState), wsgState: make(map[uint32]*wsgBattlegroundState), abState: make(map[uint32]*abBattlegroundState), eotsState: make(map[uint32]*eotsBattlegroundState), avState: make(map[uint32]*avBattlegroundState), saState: make(map[uint32]*saBattlegroundState), icState: make(map[uint32]*icBattlegroundState), activeTotems: make(map[uint64][4]*activeTotem), creatureAuras: make(map[creatureAuraKey]map[uint32]struct{}), activeCreatureAuras: make(map[creatureAuraKey]map[uint32]*activeAura), singleCastAuras: make(map[uint64][]singleCastEntry), channels: make(map[string]*worldChannel), groups: make(map[uint64]*groupState), instanceCreatureMotion: make(map[instanceAdmissionKey]map[uint64]*creatureMotion), creatureRespawns: make(map[uint32]creatureRespawn), transports: make(map[uint32]*continentTransport), creatureLoot: make(map[lootObjectKey]*activeLootState), creatureLootOwners: make(map[lootObjectKey]lootOwnerState), pickpocketLootRestore: make(map[lootObjectKey]int64), creatureStatsCache: make(map[uint32]creatureStats), groupRolls: make(map[lootRollKey]*activeGroupRoll), wardenCheckMgr: newWardenCheckMgr(), vehicleKits: make(map[uint64]*VehicleKit), vehicleSeatAddons: make(map[uint32]*VehicleSeatAddon), vehicleAccessories: make(map[uint32][]VehicleAccessory), terrainTiles: make(map[uint64][]terrainSpawn), terrainTileKnown: make(map[uint64]bool), terrainModels: make(map[string]*terrainModel), worldstates: make(map[uint32]uint64)}
	server.ticketsEnabled.Store(true)
	if c.IPLocationFile != "" {
		locations, err := iplocation.Load(c.IPLocationFile)
		if err != nil {
			if logger != nil {
				logger.Error("IP location file load failed", "path", c.IPLocationFile, "error", err)
			}
		} else {
			server.ipLocations = locations
		}
	}
	server.Features.LFG.SetDungeonValidator(func(id uint32) bool {
		dungeon, found, err := server.Data.LFGDungeon(id)
		return err == nil && found && wotlk.IsSupportedLFGType(dungeon.TypeID)
	})
	server.Features.Scripts.SetPlayerProvider(server.luaPlayers)
	server.worldTimeStartedAt = time.Now()
	return server
}

func (s *Server) Initialize(ctx context.Context) error {
	s.warnMissingGameData()
	s.clearOnlineState(ctx)
	if err := s.loadPlayerSecurityLimit(ctx); err != nil {
		return err
	}
	if err := s.initializeCharacterGUIDs(ctx); err != nil {
		return err
	}
	s.loadClientCacheVersion(ctx)
	s.repairGuildRankOverrides(ctx)
	if err := s.Features.Initialize(ctx); err != nil {
		return err
	}
	if s.Features != nil && s.Features.Scripts != nil {
		_, _ = s.Features.Scripts.TriggerServerEvent(ctx, 14)
	}
	if s.Config.WardenEnabled {
		s.loadWardenChecks(ctx)
	}
	s.loadVehicleSeatAddons(ctx)
	s.loadVehicleAccessories(ctx)
	s.loadContinentTransports(ctx)
	s.loadWorldStates(ctx)
	s.initializeMailState(ctx)
	s.seedItemGUID(ctx)
	s.loadStoredContainerLoot(ctx)
	go s.runWorldTick(ctx)
	go s.runMailSweepLoop(ctx)
	return nil
}

// initializeMailState mirrors the mail arms of worldserver startup
// (World.cpp:2048): ObjectMgr::ReturnOrDeleteOldMails(false) — the
// CHAR_DEL_EMPTY_EXPIRED_MAIL purge (ObjectMgr.cpp:6258) of mails that expired
// with no items and an empty body, followed by the full return-or-delete sweep
// over every receiver. No sessions are connected yet, so the sweep's
// connected-receiver skip is a no-op. It also seeds the in-memory mail-ID
// generator (ObjectMgr::GenerateMailID, ObjectMgr.cpp:7319).
func (s *Server) initializeMailState(ctx context.Context) {
	if s == nil || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	cdb := s.CharactersStore.DB
	if _, err := cdb.ExecContext(ctx, "DELETE FROM mail WHERE expire_time < ? AND has_items = 0 AND body = ''", time.Now().Unix()); err != nil && s.Logger != nil {
		s.Logger.Warn("mail: startup empty-expired purge failed", "error", err)
	}
	s.seedMailID(ctx)
	s.sweepExpiredMails(ctx)
}

// seedMailID loads the mail-ID generator seed from the characters database,
// mirroring ObjectMgr::LoadMailIDs (ObjectMgr.cpp:7319): _mailId starts one
// past the highest mail row id seen at startup.
func (s *Server) seedMailID(ctx context.Context) {
	s.mailIDMu.Lock()
	defer s.mailIDMu.Unlock()
	if s.mailIDReady {
		return
	}
	var highest uint32
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_ = s.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM mail").Scan(&highest)
	}
	if highest >= s.mailIDNext {
		s.mailIDNext = highest + 1
	}
	if s.mailIDNext == 0 {
		s.mailIDNext = 1
	}
	s.mailIDReady = true
}

// generateMailID mirrors ObjectMgr::GenerateMailID (ObjectMgr.cpp:7366-7374):
// the next mail row id from the in-memory counter. At 0xFFFFFFFE the C++ side
// logs an error and returns 0; the Go side does the same.
func (s *Server) generateMailID() uint32 {
	s.mailIDMu.Lock()
	defer s.mailIDMu.Unlock()
	if !s.mailIDReady {
		s.mailIDReady = true
		s.mailIDNext = 1
	}
	if s.mailIDNext >= 0xFFFFFFFE {
		if s.Logger != nil {
			s.Logger.Error("mail: mail ID overflow, cannot generate new mail IDs")
		}
		return 0
	}
	id := s.mailIDNext
	s.mailIDNext++
	return id
}

// seedItemGUID loads the item-GUID generator seed from the characters
// database, mirroring ObjectMgr::SetHighestGuids (ObjectMgr.cpp:7293-7301):
// the item counter starts one past the highest item_instance row guid seen
// at startup.
func (s *Server) seedItemGUID(ctx context.Context) {
	s.itemGUIDMu.Lock()
	defer s.itemGUIDMu.Unlock()
	if s.itemGUIDReady {
		return
	}
	var highest uint32
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_ = s.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(MAX(guid), 0) FROM item_instance").Scan(&highest)
	}
	if highest >= s.itemGUIDNext {
		s.itemGUIDNext = highest + 1
	}
	if s.itemGUIDNext == 0 {
		s.itemGUIDNext = 1
	}
	s.itemGUIDReady = true
}

// generateItemGUID mirrors ObjectGuidGenerator<HighGuid::Item>::Generate
// (ObjectGuid.h:300-309): the next item_instance guid from the in-memory
// counter. At GetMaxCounter(HighGuid::Item)-1 (0xFFFFFFFF-1, no entry bits)
// the C++ side runs HandleCounterOverflow; the Go side logs and returns 0.
func (s *Server) generateItemGUID() uint32 {
	s.itemGUIDMu.Lock()
	defer s.itemGUIDMu.Unlock()
	if !s.itemGUIDReady {
		s.itemGUIDReady = true
		s.itemGUIDNext = 1
	}
	if s.itemGUIDNext >= 0xFFFFFFFE {
		if s.Logger != nil {
			s.Logger.Error("object: item GUID counter overflow, cannot generate new item GUIDs")
		}
		return 0
	}
	guid := s.itemGUIDNext
	s.itemGUIDNext++
	return guid
}

// generateItemGUIDRange reserves count consecutive item_instance guids and
// returns the first one, for multi-stack creates that need a guid block up
// front (the old code read MAX(guid) once and added offsets).
func (s *Server) generateItemGUIDRange(count uint64) (uint32, bool) {
	if count == 0 {
		return 0, true
	}
	s.itemGUIDMu.Lock()
	defer s.itemGUIDMu.Unlock()
	if !s.itemGUIDReady {
		s.itemGUIDReady = true
		s.itemGUIDNext = 1
	}
	if count > uint64(0xFFFFFFFE-s.itemGUIDNext) {
		if s.Logger != nil {
			s.Logger.Error("object: item GUID counter overflow, cannot reserve item GUID range")
		}
		return 0, false
	}
	base := s.itemGUIDNext
	s.itemGUIDNext += uint32(count)
	return base, true
}

// runMailSweepLoop mirrors the WUPDATE_AUCTIONS mail arm (World.cpp:2374-2385):
// ReturnOrDeleteOldMails(true) fires every 24 hours, first at the next
// CleanOldMailTime hour (default 04:00 local, clamped to 0-23 like the C++
// config load at World.cpp:1032-1036).
func (s *Server) runMailSweepLoop(ctx context.Context) {
	if s == nil {
		return
	}
	hour := int(s.Config.CleanOldMailTime) % 24
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	timer := time.NewTimer(next.Sub(now))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.sweepExpiredMails(ctx)
			timer.Reset(24 * time.Hour)
		}
	}
}

// repairGuildRankOverrides mirrors the tail of Guild::Load
// (Guild.cpp:2118-2124): when Guild.AllowMultipleGuildMaster is false (the
// default), every member holding GR_GUILDMASTER (rank 0) who is not the
// recorded leader is demoted to GR_OFFICER (rank 1). C++ applies this per
// guild at startup while loading all guilds; the Go server keeps no
// in-memory guild registry, so the same demotion runs as a single startup
// pass over the character database before any session attaches (no players
// are connected yet, so the in-memory SetRank side of
// Guild::Member::ChangeRank has nothing to update). A failure here must not
// fail startup, so it only logs.
func (s *Server) repairGuildRankOverrides(ctx context.Context) {
	if s == nil || s.Config.AllowMultipleGuildMaster {
		return
	}
	if s.CharactersStore == nil || s.CharactersStore.DB == nil {
		return
	}
	res, err := s.CharactersStore.DB.ExecContext(ctx,
		"UPDATE guild_member SET `rank` = 1 WHERE `rank` = 0 AND guid <> (SELECT leaderguid FROM guild WHERE guild.guildid = guild_member.guildid)")
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("guild rank repair failed", "error", err)
		}
		return
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 && s.Logger != nil {
		s.Logger.Info("demoted non-leader guildmasters", "count", n)
	}
}

func (s *Server) loadPlayerSecurityLimit(ctx context.Context) error {
	if s == nil || s.AuthStore == nil || s.AuthStore.DB == nil {
		return nil
	}
	var level uint8
	err := s.AuthStore.DB.QueryRowContext(ctx, "SELECT allowedSecurityLevel FROM realmlist WHERE id = ?", s.RealmID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if level >= 4 {
		level = 0
	}
	s.allowedSecurityLevel = level
	return nil
}

func (s *Server) initializeCharacterGUIDs(ctx context.Context) error {
	s.characterGUIDMu.Lock()
	defer s.characterGUIDMu.Unlock()
	if s.characterGUIDReady {
		return nil
	}
	highest, err := highestCharacterGUID(ctx, s.CharactersStore.DB)
	if err != nil {
		return err
	}
	if highest > s.characterGUIDNext {
		s.characterGUIDNext = highest
	}
	s.characterGUIDReady = true
	return nil
}

func (s *Server) allocateCharacterGUID(ctx context.Context) (uint64, error) {
	if err := s.initializeCharacterGUIDs(ctx); err != nil {
		return 0, err
	}
	s.characterGUIDMu.Lock()
	defer s.characterGUIDMu.Unlock()
	if s.characterGUIDNext >= uint64(^uint32(0)) {
		return 0, fmt.Errorf("player GUID space exhausted")
	}
	s.characterGUIDNext++
	return s.characterGUIDNext, nil
}

func (s *Server) warnMissingGameData() {
	if s == nil || s.Logger == nil || s.Config.GameDataDir == "" {
		return
	}
	dbcDir := filepath.Join(s.Config.GameDataDir, "dbc")
	required := []string{"ChrRaces.dbc", "ChrClasses.dbc", "Spell.dbc", "Map.dbc"}
	missing := make([]string, 0, len(required))
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(dbcDir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		s.Logger.Warn("DBC-backed game data is incomplete", "directory", dbcDir, "missing", strings.Join(missing, ","))
	}
}

func (s *Server) loadClientCacheVersion(ctx context.Context) {
	if s == nil {
		return
	}
	if s.Config.ClientCacheVersion != 0 {
		s.clientCacheVersion = s.Config.ClientCacheVersion
		return
	}
	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return
	}
	var cacheID sql.NullInt64
	if err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT cache_id FROM version LIMIT 1").Scan(&cacheID); err == nil && cacheID.Valid && cacheID.Int64 > 0 {
		s.clientCacheVersion = uint32(cacheID.Int64)
	}
}

func (s *Server) clearOnlineState(ctx context.Context) {
	if s == nil {
		return
	}
	if s.AuthStore != nil && s.AuthStore.DB != nil {
		if _, err := s.AuthStore.DB.ExecContext(ctx, "UPDATE account SET online = 0 WHERE online > 0 AND id IN (SELECT acctid FROM realmcharacters WHERE realmid = ?)", s.RealmID); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to clear online account state", "error", err)
		}
	}
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		if _, err := s.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET online = 0 WHERE online <> 0"); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to clear online character state", "error", err)
		}
		if _, err := s.CharactersStore.DB.ExecContext(ctx, "UPDATE character_battleground_data SET instanceId = 0"); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to reset battleground instance state", "error", err)
		}
	}
}

func (s *Server) Stop() {
	if s == nil {
		return
	}
	s.closed.Store(true)
	s.stopOnce.Do(func() {
		if s.Features != nil && s.Features.Scripts != nil {
			_, _ = s.Features.Scripts.TriggerServerEvent(context.Background(), 15)
		}
		s.sessionsMu.RLock()
		var sessions []*session
		for sess := range s.sessions {
			sessions = append(sessions, sess)
		}
		s.sessionsMu.RUnlock()
		for _, sess := range sessions {
			if sess != nil && sess.conn != nil {
				_ = sess.conn.Close()
			}
		}
		s.abMu.RLock()
		abs := make([]*abBattlegroundState, 0, len(s.abState))
		for _, state := range s.abState {
			abs = append(abs, state)
		}
		s.abMu.RUnlock()
		for _, state := range abs {
			if state == nil {
				continue
			}
			state.mu.Lock()
			for i := range state.Nodes {
				if state.Nodes[i].CaptureTimer != nil {
					state.Nodes[i].CaptureTimer.Stop()
					state.Nodes[i].CaptureTimer = nil
				}
			}
			state.mu.Unlock()
		}
		s.avMu.RLock()
		avs := make([]*avBattlegroundState, 0, len(s.avState))
		for _, state := range s.avState {
			avs = append(avs, state)
		}
		s.avMu.RUnlock()
		for _, state := range avs {
			if state == nil {
				continue
			}
			state.mu.Lock()
			for i := range state.Nodes {
				if state.Nodes[i].CaptureTimer != nil {
					state.Nodes[i].CaptureTimer.Stop()
					state.Nodes[i].CaptureTimer = nil
				}
			}
			state.mu.Unlock()
		}
		s.icMu.RLock()
		ics := make([]*icBattlegroundState, 0, len(s.icState))
		for _, state := range s.icState {
			ics = append(ics, state)
		}
		s.icMu.RUnlock()
		for _, state := range ics {
			if state == nil {
				continue
			}
			state.mu.Lock()
			for i := range state.Nodes {
				if state.Nodes[i].CaptureTimer != nil {
					state.Nodes[i].CaptureTimer.Stop()
					state.Nodes[i].CaptureTimer = nil
				}
			}
			state.mu.Unlock()
		}
		s.eotsMu.RLock()
		eotss := make([]*eotsBattlegroundState, 0, len(s.eotsState))
		for _, state := range s.eotsState {
			eotss = append(eotss, state)
		}
		s.eotsMu.RUnlock()
		for _, state := range eotss {
			if state == nil {
				continue
			}
			state.mu.Lock()
			if state.FlagReturnTimer != nil {
				state.FlagReturnTimer.Stop()
				state.FlagReturnTimer = nil
			}
			if state.FlagRespawnTimer != nil {
				state.FlagRespawnTimer.Stop()
				state.FlagRespawnTimer = nil
			}
			state.mu.Unlock()
		}
		s.wsgMu.RLock()
		wsgs := make([]*wsgBattlegroundState, 0, len(s.wsgState))
		for _, state := range s.wsgState {
			wsgs = append(wsgs, state)
		}
		s.wsgMu.RUnlock()
		for _, state := range wsgs {
			if state == nil {
				continue
			}
			state.mu.Lock()
			if state.AllianceReturnTimer != nil {
				state.AllianceReturnTimer.Stop()
				state.AllianceReturnTimer = nil
			}
			if state.HordeReturnTimer != nil {
				state.HordeReturnTimer.Stop()
				state.HordeReturnTimer = nil
			}
			state.mu.Unlock()
		}
		s.objectsMu.Lock()
		for _, dyn := range s.dynamicGameObjects {
			if dyn == nil {
				continue
			}
			if dyn.AutoCloseTimer != nil {
				dyn.AutoCloseTimer.Stop()
				dyn.AutoCloseTimer = nil
			}
			if dyn.DespawnTimer != nil {
				dyn.DespawnTimer.Stop()
				dyn.DespawnTimer = nil
			}
		}
		s.objectsMu.Unlock()
		s.auraMu.Lock()
		for _, auras := range s.activeCreatureAuras {
			for _, aura := range auras {
				if aura == nil {
					continue
				}
				aura.Stopped = true
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
			}
		}
		s.auraMu.Unlock()
		s.lootMu.Lock()
		for _, roll := range s.groupRolls {
			if roll != nil && roll.Timer != nil {
				roll.Timer.Stop()
			}
		}
		s.lootMu.Unlock()
	})
}

func (s *Server) loadWardenChecks(ctx context.Context) {
	if s.wardenCheckMgr == nil {
		s.wardenCheckMgr = newWardenCheckMgr()
	}
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		if err := s.wardenCheckMgr.loadChecks(ctx, s.WorldStore.DB, s.Config.WardenClientCheckFailAction); err != nil {
			s.Logger.Error("failed to load warden checks", "error", err)
		}
	}
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		if err := s.wardenCheckMgr.loadOverrides(ctx, s.CharactersStore.DB); err != nil {
			s.Logger.Error("failed to load warden action overrides", "error", err)
		}
	}
}

func (s *Server) updateWardenSessions(ctx context.Context, diff time.Duration) {
	if !s.Config.WardenEnabled {
		return
	}
	s.sessionsMu.RLock()
	var wardenSessions []*wardenSession
	for sess := range s.sessions {
		if sess.warden != nil {
			wardenSessions = append(wardenSessions, sess.warden)
		}
	}
	s.sessionsMu.RUnlock()

	for _, w := range wardenSessions {
		w.update(diff)
	}
}

func (s *Server) runWorldTick(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	lastUpdate := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			diff := now.Sub(lastUpdate)
			lastUpdate = now
			if s.Features != nil && s.Features.Scripts != nil {
				_ = s.Features.Scripts.Tick(ctx, 100)
				_, _ = s.Features.Scripts.TriggerServerEvent(ctx, 13, uint32(100))
			}
			s.updateContinentTransports(now)
			s.updateWeather(ctx, now)
			s.updateTimeSync(now)
			s.updateMailDeliveries(ctx, now.Unix())
			s.updateActiveCreatures(ctx)
			s.updatePetRuntime(now, diff)
			s.updateDynamicSpellAuras(ctx, now)
			s.updatePlayerCombat(ctx)
			s.updateContestedPvP(now)
			s.updatePvPFlags(now)
			s.updatePlayerRegeneration(ctx, now)
			s.updateRestedBonuses(ctx, now)
			s.processCreatureRespawns(ctx, now)
			s.updatePlayerDeathTimers(ctx, now)
			s.updatePendingInstanceBinds(ctx, diff)
			s.updateSpiritHealerResurrectWaves(ctx, now)
			s.updateCorpseExpiry(ctx)
			s.updatePlayerUnderwater(ctx, now)
			s.sweepIdleConnections(now)
			s.updateArenaBattles(now)
			s.updateAVBattles(now)
			s.updateEOTSBattles(now)
			s.updateSABattles(now)
			s.updateWardenSessions(ctx, 100*time.Millisecond)
		}
	}
}

// timeSyncSample holds one (clockDelta, round-trip latency) pair from a
// CMSG_TIME_SYNC_RESP exchange (WorldSession.h:1232). The queue keeps the six
// most recent samples (WorldSession.cpp:141).
type timeSyncSample struct {
	clockDelta int64
	latency    uint32
}

func (s *Server) updateTimeSync(now time.Time) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	sessions := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && !sess.timeSyncDue.IsZero() && !now.Before(sess.timeSyncDue) {
			sessions = append(sessions, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range sessions {
		counter := sess.timeSyncNextCounter
		if sess.write(uint16(protocol.OpcodeSMSG_TIME_SYNC_REQ), buildTimeSyncRequest(counter), true) != nil {
			continue
		}
		sess.recordTimeSyncSent(counter)
		sess.timeSyncNextCounter++
		sess.timeSyncDue = now.Add(10 * time.Second)
	}
}

// sweepIdleConnections mirrors WorldSession::Update's idle check (WorldSession.cpp:290):
// a session whose activity timeout expired (WorldSession::IsConnectionIdle,
// WorldSession.cpp:741) and that is not sitting in the login queue has its socket
// closed, unless its account holds RBAC_PERM_IGNORE_IDLE_CONNECTION (cached on the
// session at auth time). Only authed sessions are swept; pre-auth sockets have no
// WorldSession in C++ either.
func (s *Server) sweepIdleConnections(now time.Time) {
	if s == nil {
		return
	}
	unix := now.Unix()
	s.sessionsMu.RLock()
	var idle []*session
	for sess := range s.sessions {
		if sess == nil || !sess.authed || sess.inQueue || sess.ignoreIdleTimeout {
			continue
		}
		if sess.timeoutTime.Load() < unix {
			idle = append(idle, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range idle {
		sess.debug("closing idle connection", "account", sess.accountName)
		if sess.conn != nil {
			_ = sess.conn.Close()
		}
	}
}

func (s *Server) updatePlayerCombat(ctx context.Context) {
	s.sessionsMu.RLock()
	var combatSessions []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && (sess.attackTarget != 0 || sess.autoRepeatSpell != 0) && !sess.isDeadOrGhost() {
			combatSessions = append(combatSessions, sess)
		}
	}
	s.sessionsMu.RUnlock()

	now := time.Now()
	for _, sess := range combatSessions {
		// Melee combat
		if sess.attackTarget != 0 {
			target, ok := sess.getCombatTarget(ctx, sess.attackTarget)
			// The ghost leg mirrors WorldObject::IsValidAttackTarget's "can't
			// attack dead" (Object.cpp:2935-2937): ghosts carry Health=1 in Go,
			// so the Health==0 gate misses them; nowDead=false matches the
			// SendAttackStop IsValidAttackTarget-failure arm.
			if !ok || target.Health == 0 || sess.ghostAttackTargetBlocked(sess.attackTarget) {
				_ = sess.sendAttackStop(sess.attackTarget, target.Health == 0)
				sess.attackTarget = 0
				continue
			}
			if target.Map != sess.player.Map {
				_ = sess.sendAttackStop(sess.attackTarget, false)
				sess.attackTarget = 0
				continue
			}
			mainSpeed := 2 * time.Second
			if sess.player.AttackTime > 0 {
				mainSpeed = sess.getHastedMeleeSpeed(time.Duration(sess.player.AttackTime) * time.Millisecond)
			}
			offSpeed := 2 * time.Second
			if sess.player.OffhandAttackTime > 0 {
				offSpeed = sess.getHastedMeleeSpeed(time.Duration(sess.player.OffhandAttackTime) * time.Millisecond)
			}

			pReach := float32(1.5)
			if sess.player.CombatReach > 0 {
				pReach = sess.player.CombatReach
			}
			allowedRange := calcMeleeRange(pReach, target.CombatReach) + 2.0

			dist := distance3D(sess.player.X, sess.player.Y, sess.player.Z, target.X, target.Y, target.Z)
			inMelee := dist <= allowedRange
			mainReady := now.Sub(sess.lastSwing) >= mainSpeed
			offReady := sess.haveOffhandWeapon() && now.Sub(sess.lastOffhandSwing) >= offSpeed
			// TrinityCore Player::Update melee block (Player.cpp:1116-1180): an
			// attack-ready swing that cannot land reports a single swing-error
			// packet to the client, latched on m_swingErrorMsg so it is not
			// re-sent every tick (1 = not in range, 2 = bad facing). The
			// setAttackTimer(..., 100) retry cadence needs no bridge: this tick
			// already runs every 100ms (server.go:777). The off-hand arm sends
			// no error packets, it only skips; and a swing that can land
			// resets the latch.
			badFacing := !hasInArc(sess.player.Orientation, sess.player.X, sess.player.Y, target.X, target.Y, 2*math.Pi/3)
			if mainReady {
				if !inMelee {
					if sess.swingErrorMsg != 1 {
						_ = sess.write(uint16(protocol.OpcodeSMSG_ATTACK_SWING_NOT_IN_RANGE), nil, true)
						sess.swingErrorMsg = 1
					}
				} else if badFacing {
					if sess.swingErrorMsg != 2 {
						_ = sess.write(uint16(protocol.OpcodeSMSG_ATTACK_SWING_BAD_FACING), nil, true)
						sess.swingErrorMsg = 2
					}
				} else {
					sess.swingErrorMsg = 0
				}
			}

			if inMelee && !badFacing {
				// Main hand attack
				if mainReady {
					if sess.haveOffhandWeapon() && now.Sub(sess.lastOffhandSwing) < attackDisplayDelay {
						sess.lastOffhandSwing = now.Add(-(offSpeed - attackDisplayDelay))
					}
					sess.lastSwing = now
					sess.executeMeleeSwing(ctx, target, protocol.BaseAttack)
				}

				// Off-hand attack (dual-wielding)
				if offReady {
					if now.Sub(sess.lastSwing) < attackDisplayDelay {
						sess.lastSwing = now.Add(-(mainSpeed - attackDisplayDelay))
					}
					sess.lastOffhandSwing = now
					sess.executeMeleeSwing(ctx, target, protocol.OffAttack)
				}
			}
		}

		// Ranged auto-attacks / auto-repeat spells (Auto Shot, Shoot Wand) (TC: Unit::_UpdateAutoRepeatSpell)
		if sess.autoRepeatSpell != 0 {
			targetGUID := sess.autoRepeatTarget
			if targetGUID == 0 {
				targetGUID = sess.selection
			}
			rTarget, rOk := sess.getCombatTarget(ctx, targetGUID)
			// The ghost leg mirrors WorldObject::IsValidAttackTarget's "can't
			// attack dead" (Object.cpp:2935-2937): ghosts carry Health=1 in Go.
			if !rOk || rTarget.Health == 0 || sess.ghostAttackTargetBlocked(targetGUID) || rTarget.Map != sess.player.Map {
				sess.autoRepeatSpell = 0
				sess.autoRepeatTarget = 0
				buf := protocol.NewBuffer(9)
				buf.WritePackedGUID(sess.playerGUID)
				_ = sess.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
			} else {
				pReach := float32(1.5)
				if sess.player.CombatReach > 0 {
					pReach = sess.player.CombatReach
				}
				dist := distance3D(sess.player.X, sess.player.Y, sess.player.Z, rTarget.X, rTarget.Y, rTarget.Z)
				minRange := float64(0)
				maxRange := float64(30.0)
				if sess.autoRepeatSpell == 75 {
					minRange = calcMeleeRange(pReach, rTarget.CombatReach)
					maxRange = 35.0
				}
				// Wand shooting is cancelled if moving (TC Unit::_UpdateAutoRepeatSpell)
				if sess.isMoving && sess.autoRepeatSpell != 75 {
					sess.autoRepeatSpell = 0
					sess.autoRepeatTarget = 0
					buf := protocol.NewBuffer(9)
					buf.WritePackedGUID(sess.playerGUID)
					_ = sess.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
				} else if !sess.isMoving || sess.autoRepeatSpell == 75 {
					if dist >= minRange && dist <= maxRange {
						rangedSpeed := 2 * time.Second
						if sess.player.RangedAttackTime > 0 {
							rangedSpeed = sess.getHastedRangedSpeed(time.Duration(sess.player.RangedAttackTime) * time.Millisecond)
						}
						if now.Sub(sess.lastRangedSwing) >= rangedSpeed {
							sess.executeRangedAttack(ctx, rTarget, sess.autoRepeatSpell)
						}
					}
				}
			}
		}
	}
}

func (s *Server) updatePlayerRegeneration(ctx context.Context, now time.Time) {
	s.sessionsMu.RLock()
	var activeSessions []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && !sess.isDeadOrGhost() {
			activeSessions = append(activeSessions, sess)
		}
	}
	s.sessionsMu.RUnlock()

	for _, sess := range activeSessions {
		if sess.lastRegenTick.IsZero() {
			sess.lastRegenTick = now
			continue
		}
		if now.Sub(sess.lastRegenTick) < 2*time.Second {
			continue
		}
		sess.lastRegenTick = now

		p := sess.player
		if p == nil {
			continue
		}

		inCombat := sess.attackTarget != 0 || (p.UnitFlags&unitFlagInCombat != 0)
		fields := make(map[int]uint32)
		changed := false

		// Check combat drop: if no attack target, 5s passed since lastCombatTime, and no creatures targeting player
		if p.UnitFlags&unitFlagInCombat != 0 && sess.attackTarget == 0 && now.Sub(sess.lastCombatTime) >= 5*time.Second {
			hasAggro := false
			s.motionMu.Lock()
			for _, m := range s.motionMapLocked(p.Map, p.InstanceID) {
				if m != nil && m.InCombat && m.TargetGUID == sess.playerGUID {
					hasAggro = true
					break
				}
			}
			s.motionMu.Unlock()
			if !hasAggro {
				p.UnitFlags &^= unitFlagInCombat
				fields[unitFieldFlags] = unitFlagPlayerControlled | p.UnitFlags
				changed = true
				inCombat = false
			}
		}

		// 1. Health regeneration (Player::Regenerate gate, Player.cpp:2064-2072:
		// !IsInCombat() || IsPolymorphed() || m_baseHealthRegen ||
		// HasAuraType(116) || HasAuraType(161); the spirit-based addValue
		// (Player.cpp:2248-2258) also runs in combat when the player carries
		// SPELL_AURA_MOD_REGEN_DURING_COMBAT (116), scaled by
		// ApplyPct(addValue, GetTotalAuraModifier(116)); the flat
		// SPELL_AURA_MOD_HEALTH_REGEN_IN_COMBAT (161) bonus runs in and out
		// of combat (Player.cpp:2265). The polymorphed addValue,
		// m_baseHealthRegen, regen-percent / MOD_REGEN flat terms, and the
		// sitting 1.5x stay unmodeled.
		inCombatRegen := inCombat && sess.hasAuraType(spellAuraModRegenDuringCombat)
		inCombatFlatRegen := inCombat && sess.hasAuraType(spellAuraModHealthRegenInCombat)
		// Player.cpp:2066: m_baseHealthRegen alone keeps the regen block
		// running in combat, and RegenerateHealth adds m_baseHealthRegen/2.5
		// per 2s tick (Player.cpp:2266) alongside the aura flat term.
		itemHealthRegen := p.ItemHealthRegenBonus > 0
		if (!inCombat || inCombatRegen || inCombatFlatRegen || itemHealthRegen) && p.Health < p.MaxHealth {
			gainF := 0.0
			if !inCombat || inCombatRegen {
				gainF = s.octRegenHPPerSpirit(uint32(p.Class), uint32(p.Level), float64(p.Stats[4])) * octRegenLowLevelMultiplier(p.Level)
				if inCombatRegen {
					gainF *= (100 + float64(sess.getTotalAuraModifier(spellAuraModRegenDuringCombat))) / 100
				}
			}
			gainF += float64(sess.getTotalAuraModifier(spellAuraModHealthRegenInCombat))
			gainF += float64(p.ItemHealthRegenBonus) / 2.5
			if gainF < 0 {
				gainF = 0
			}
			gain := uint32(gainF)
			if gain > 0 {
				if p.Health+gain >= p.MaxHealth {
					p.Health = p.MaxHealth
				} else {
					p.Health += gain
				}
				fields[unitFieldHealth] = p.Health
				changed = true
			}
		}

		// 2. Power regeneration
		switch p.Class {
		case 1: // Warrior: rage decays out of combat
			if !inCombat && p.Powers[1] > 0 {
				if p.Powers[1] > 20 {
					p.Powers[1] -= 20
				} else {
					p.Powers[1] = 0
				}
				fields[unitFieldPower1+1] = p.Powers[1]
				changed = true
			}
		case 4: // Rogue: energy regenerates 20 per 2s tick
			if p.Powers[3] < 100 {
				if p.Powers[3]+20 >= 100 {
					p.Powers[3] = 100
				} else {
					p.Powers[3] += 20
				}
				fields[unitFieldPower1+3] = p.Powers[3]
				changed = true
			}
		case 6: // Death Knight: runic power decays out of combat
			if !inCombat && p.Powers[6] > 0 {
				if p.Powers[6] > 30 {
					p.Powers[6] -= 30
				} else {
					p.Powers[6] = 0
				}
				fields[unitFieldPower1+6] = p.Powers[6]
				changed = true
			}
		default: // Mana classes (Paladin, Hunter, Priest, Shaman, Mage, Warlock, Druid)
			if p.Powers[0] < p.MaxPowers[0] {
				// Outside 5-second rule
				if now.Sub(sess.lastCastTime) >= 5*time.Second {
					gain := s.manaRegenTickGain(uint32(p.Class), p.Level, p.Stats[3], p.Stats[4])
					// StatSystem.cpp:921: the mp5 term is
					// (auraMods + m_baseManaRegen)/5 per 2s tick.
					gain += uint32(float64(p.ItemManaRegenBonus) / 5.0)
					if gain > 0 {
						if p.Powers[0]+gain >= p.MaxPowers[0] {
							p.Powers[0] = p.MaxPowers[0]
						} else {
							p.Powers[0] += gain
						}
						fields[unitFieldPower1] = p.Powers[0]
						changed = true
					}
				}
			}
		}

		if changed && len(fields) > 0 {
			if packet, err := s.buildPlayerValuesUpdateForTarget(sess.playerGUID, fields, true); err == nil && packet != nil {
				_ = sess.write(packet.Opcode, packet.Payload.Bytes(), true)
				s.broadcastPlayerValuesUpdateFromSession(sess, fields)
			}
		}
	}
}

func (s *Server) Handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	state := &session{server: s, conn: conn, legitimate: make(map[uint64]struct{}), characterNames: make(map[uint64]enumCharacter), auras: make(map[uint32]struct{}), auraSlots: make(map[uint32]uint8), channels: make(map[string]struct{}), timeSyncPending: make(map[uint32]uint32), scale: 1, breathTimer: -1, fatigueTimer: -1, schoolLockouts: make(map[uint32]int64)}
	state.resetTimeoutTime(false)
	// WorldSocket::Start/CheckIpCallback (WorldSocket.cpp:49): the connect
	// handshake rejects a banned source IP with the 1-byte AUTH_REJECT form
	// before the auth challenge is ever sent (LOGIN_SEL_IP_INFO).
	if ipBanned(ctx, s.AuthStore, remoteAddress(conn)) {
		state.debug("world connection rejected", "remote", remoteAddress(conn), "reason", "ip banned")
		_ = state.write(opcodeAuthResponse, []byte{authReject}, false)
		return
	}
	s.addSession(state)
	defer s.removeSession(state)
	defer state.logout()
	if _, err := rand.Read(state.authSeed[:]); err != nil {
		return
	}
	var extra [32]byte
	if _, err := rand.Read(extra[:]); err != nil {
		return
	}
	challenge := protocol.NewBuffer(40)
	challenge.WriteU32(1)
	challenge.Write(state.authSeed[:])
	challenge.Write(extra[:])
	if err := state.write(opcodeAuthChallenge, challenge.Bytes(), false); err != nil {
		return
	}
	for {
		if state.logoutAt.IsZero() {
			_ = conn.SetReadDeadline(time.Time{})
		} else {
			_ = conn.SetReadDeadline(state.logoutAt)
		}
		header, payload, err := protocol.ReadClientFrame(conn, state.decrypt)
		if err != nil {
			state.debug("world connection closed", "account", state.accountName, "error", err)
			if !state.logoutAt.IsZero() && isReadTimeout(err) {
				if logoutErr := state.completeLogout(ctx); logoutErr != nil {
					state.debug("player logout failed", "account", state.accountName, "error", logoutErr)
				}
				continue
			}
			return
		}
		state.debug("world packet received", "account", state.accountName, "opcode", opcodeName(header.Opcode), "size", len(payload))
		if state.server != nil && state.server.TraceRecorder != nil {
			state.server.TraceRecorder.Record(protocoltrace.ClientToServer, header.Opcode, payload, opcodeName(header.Opcode))
		}
		if !state.logoutAt.IsZero() && !time.Now().Before(state.logoutAt) {
			if logoutErr := state.completeLogout(ctx); logoutErr != nil {
				state.debug("player logout failed", "account", state.accountName, "error", logoutErr)
			}
			continue
		}
		if state.authed && state.server.Features != nil && state.server.Features.Scripts != nil {
			packet := &scripting.Packet{Opcode: header.Opcode, Data: append([]byte(nil), payload...)}
			values, hookErr := state.server.Features.Scripts.TriggerPacketEvent(ctx, int(header.Opcode), 5, packet, state.luaPlayer())
			if hookErr != nil {
				state.debug("lua packet hook failed", "account", state.accountName, "opcode", header.Opcode, "error", hookErr)
			}
			blocked := false
			for _, value := range values {
				if allowed, ok := value.(bool); ok && !allowed {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}
		}
		// WorldSocket::ReadDataHandler refreshes the idle timer on every packet
		// (WorldSocket.cpp:384) except CMSG_KEEP_ALIVE, which only refreshes the
		// in-world timer (ResetTimeOutTime(true), WorldSocket.cpp:352); CMSG_PING
		// and CMSG_AUTH_SESSION return early without touching it.
		switch header.Opcode {
		case opcodeAuthSession, uint32(protocol.OpcodeCMSG_PING), uint32(protocol.OpcodeCMSG_KEEP_ALIVE):
		default:
			state.resetTimeoutTime(false)
		}
		switch header.Opcode {
		case opcodeAuthSession:
			if state.authed || !state.handleAuthSession(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PING):
			if !state.authed || !state.handlePing(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TIME_SYNC_RESP):
			if !state.authed || !state.handleTimeSyncResponse(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_KEEP_ALIVE):
			// WorldSocket::ReadDataHandler (WorldSocket.cpp:348): CMSG_KEEP_ALIVE
			// is tolerated before auth; it only refreshes the in-world timer
			// when a session exists (handleKeepAlive is a no-op otherwise).
			if !state.handleKeepAlive() {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_ENUM):
			if !state.authed || !state.handleCharEnum(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CREATURE_QUERY):
			if !state.authed || !state.handleCreatureQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GAMEOBJECT_QUERY):
			if !state.authed || !state.handleGameObjectQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ITEM_QUERY_SINGLE):
			if !state.authed || !state.handleItemQuerySingle(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUTOEQUIP_ITEM):
			if !state.authed || !state.handleAutoEquipItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUTOEQUIP_ITEM_SLOT):
			if !state.authed || !state.handleAutoEquipItemSlot(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SPLIT_ITEM):
			if !state.authed || !state.handleSplitItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUTOSTORE_BAG_ITEM):
			if !state.authed || !state.handleAutoStoreBagItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SWAP_INV_ITEM):
			if !state.authed || !state.handleSwapInvItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SWAP_ITEM):
			if !state.authed || !state.handleSwapItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DESTROYITEM):
			if !state.authed || !state.handleDestroyItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LIST_INVENTORY):
			if !state.authed || !state.handleListInventory(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BUY_ITEM):
			if !state.authed || !state.handleBuyItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BUY_ITEM_IN_SLOT):
			if !state.authed || !state.handleBuyItemInSlot(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SELL_ITEM):
			if !state.authed || !state.handleSellItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TRAINER_LIST):
			if !state.authed || !state.handleTrainerList(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TRAINER_BUY_SPELL):
			if !state.authed || !state.handleTrainerBuySpell(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOOT):
			if !state.authed || !state.handleLoot(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOOT_MONEY):
			if !state.authed || !state.handleLootMoney(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUTOSTORE_LOOT_ITEM):
			if !state.authed || !state.handleAutostoreLootItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOOT_RELEASE):
			if !state.authed || !state.handleLootRelease(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TAXINODE_STATUS_QUERY):
			if !state.authed || !state.handleTaxiNodeStatusQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TAXIQUERYAVAILABLENODES):
			if !state.authed || !state.handleTaxiQueryAvailableNodes(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ACTIVATETAXI), uint32(protocol.OpcodeCMSG_ACTIVATETAXIEXPRESS):
			if !state.authed || !state.handleActivateTaxi(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GET_MAIL_LIST):
			if !state.authed || !state.handleGetMailList(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SEND_MAIL):
			if !state.authed || !state.handleSendMail(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MAIL_TAKE_MONEY):
			if !state.authed || !state.handleMailTakeMoney(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MAIL_TAKE_ITEM):
			if !state.authed || !state.handleMailTakeItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MAIL_DELETE):
			if !state.authed || !state.handleMailDelete(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MAIL_MARK_AS_READ):
			if !state.authed || !state.handleMailMarkAsRead(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_QUERY_NEXT_MAIL_TIME):
			if !state.authed || !state.handleQueryNextMailTime(ctx) {
				return
			}
		case uint32(protocol.OpcodeMSG_AUCTION_HELLO):
			if !state.authed || !state.handleAuctionHello(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_LIST_ITEMS):
			if !state.authed || !state.handleAuctionListItems(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_SELL_ITEM):
			if !state.authed || !state.handleAuctionSellItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_PLACE_BID):
			if !state.authed || !state.handleAuctionPlaceBid(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_LIST_OWNER_ITEMS):
			if !state.authed || !state.handleAuctionListOwnerItems(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_LIST_BIDDER_ITEMS):
			if !state.authed || !state.handleAuctionListBidderItems(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_REMOVE_ITEM):
			if !state.authed || !state.handleAuctionRemoveItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_INITIATE_TRADE):
			if !state.authed || !state.handleInitiateTrade(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BEGIN_TRADE):
			if !state.authed || !state.handleBeginTrade(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_TRADE_GOLD):
			if !state.authed || !state.handleSetTradeGold(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_TRADE_ITEM):
			if !state.authed || !state.handleSetTradeItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CLEAR_TRADE_ITEM):
			if !state.authed || !state.handleClearTradeItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ACCEPT_TRADE):
			if !state.authed || !state.handleAcceptTrade(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_UNACCEPT_TRADE):
			if !state.authed || !state.handleUnacceptTrade(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_TRADE):
			if !state.authed || !state.handleCancelTrade(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_IGNORE_TRADE), uint32(protocol.OpcodeCMSG_BUSY_TRADE):
			// Reference: HandleIgnoreTradeOpcode/HandleBusyTradeOpcode
			// (TradeHandler.cpp:63-73) are debug-log no-ops — routing them
			// through handleCancelTrade would nuke an in-progress trade
			// whenever the client auto-responds busy/ignore.
			if !state.authed {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_QUERY):
			if !state.authed || !state.handleGuildQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_ROSTER):
			if !state.authed || !state.handleGuildRoster(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_INVITE):
			if !state.authed || !state.handleGuildInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_ACCEPT):
			if !state.authed || !state.handleGuildAccept(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_DECLINE):
			if !state.authed || !state.handleGuildDecline(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_LEAVE):
			if !state.authed || !state.handleGuildLeave(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_MOTD):
			if !state.authed || !state.handleGuildMotd(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANK_QUERY_TAB):
			if !state.authed || !state.handleGuildBankQueryTab(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CONTACT_LIST):
			if !state.authed || !state.handleContactList(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ADD_FRIEND):
			if !state.authed || !state.handleAddFriend(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DEL_FRIEND):
			if !state.authed || !state.handleDelFriend(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ADD_IGNORE):
			if !state.authed || !state.handleAddIgnore(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DEL_IGNORE):
			if !state.authed || !state.handleDelIgnore(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_CONTACT_NOTES):
			if !state.authed || !state.handleSetContactNotes(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_INVITE):
			if !state.authed || !state.handleGroupInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_ACCEPT):
			if !state.authed || !state.handleGroupAccept(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_DECLINE):
			if !state.authed || !state.handleGroupDecline(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_UNINVITE):
			if !state.authed || !state.handleGroupUninvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_UNINVITE_GUID):
			if !state.authed || !state.handleGroupUninviteGUID(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_SET_LEADER):
			if !state.authed || !state.handleGroupSetLeader(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_DISBAND):
			if !state.authed || !state.handleGroupDisband(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOOT_METHOD):
			if !state.authed || !state.handleLootMethod(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_MINIMAP_PING):
			if !state.authed || !state.handleMinimapPing(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_RAID_TARGET_UPDATE):
			if !state.authed || !state.handleRaidTargetUpdate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_RAID_CONVERT):
			if !state.authed || !state.handleGroupRaidConvert(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_PARTY_ASSIGNMENT):
			if !state.authed || !state.handlePartyAssignment(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_RAID_READY_CHECK):
			if !state.authed || !state.handleReadyCheck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_RANDOM_ROLL):
			if !state.authed || !state.handleRandomRoll(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ATTACK_SWING):
			if !state.authed || !state.handleAttackSwing(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ATTACK_STOP):
			if !state.authed || !state.handleAttackStop() {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_SHEATHED):
			if !state.authed || !state.handleSetSheathed(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CAST_SPELL):
			if !state.authed || !state.handleCastSpell(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_CAST):
			if !state.authed || !state.handleCancelCast(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_CHANNELLING):
			if !state.authed || !state.handleCancelChanneling(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_AURA):
			if !state.authed || !state.handleCancelAura(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_MOUNT_AURA):
			if !state.authed || !state.handleCancelMountAura(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_GROWTH_AURA):
			if !state.authed || !state.handleCancelGrowthAura(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_AUTO_REPEAT_SPELL):
			if !state.authed || !state.handleCancelAutoRepeatSpell(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CANCEL_TEMP_ENCHANTMENT):
			if !state.authed || !state.handleCancelTempEnchantment(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CORPSE_MAP_POSITION_QUERY):
			if !state.authed || !state.handleCorpseMapPositionQuery(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GOSSIP_HELLO):
			if !state.authed || !state.handleGossipHello(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_NPC_TEXT_QUERY):
			if !state.authed || !state.handleNpcTextQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_JOIN_CHANNEL):
			if !state.authed || !state.handleJoinChannel(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LEAVE_CHANNEL):
			if !state.authed || !state.handleLeaveChannel(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_LIST), uint32(protocol.OpcodeCMSG_CHANNEL_DISPLAY_LIST):
			if !state.authed || !state.handleChannelList(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GOSSIP_SELECT_OPTION):
			if !state.authed || !state.handleGossipSelectOption(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_STATUS_QUERY):
			if !state.authed || !state.handleQuestgiverStatusQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_HELLO):
			if !state.authed || !state.handleQuestgiverHello(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_QUERY_QUEST):
			if !state.authed || !state.handleQuestgiverQueryQuest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUEST_QUERY):
			if !state.authed || !state.handleQuestQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_ACCEPT_QUEST):
			if !state.authed || !state.handleQuestgiverAcceptQuest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_COMPLETE_QUEST):
			if !state.authed || !state.handleQuestgiverCompleteQuest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_REQUEST_REWARD):
			if !state.authed || !state.handleQuestgiverRequestReward(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_CHOOSE_REWARD):
			if !state.authed || !state.handleQuestgiverChooseReward(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_QUEST_AUTOLAUNCH):
			// De-facto no-op: WorldSession::HandleQuestgiverQuestAutoLaunch (QuestHandler.cpp:558) only debug-logs.
			if !state.authed {
				return
			}
			state.debug("quest autolaunch received", "account", state.accountName, "size", len(payload))
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_CANCEL):
			if !state.authed || !state.handleQuestgiverCancel() {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTLOG_REMOVE_QUEST):
			if !state.authed || !state.handleQuestLogRemoveQuest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_NAME_QUERY):
			if !state.authed || !state.handleNameQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUERY_TIME):
			if !state.authed || !state.handleQueryTime() {
				return
			}
		case uint32(protocol.OpcodeCMSG_PLAYED_TIME):
			if !state.authed || !state.handlePlayedTime(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ZONEUPDATE):
			if !state.authed || !state.handleZoneUpdate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_ACCOUNT_DATA):
			if !state.authed || !state.handleRequestAccountData(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_UPDATE_ACCOUNT_DATA):
			if !state.authed || !state.handleUpdateAccountData(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_ACTIONBAR_TOGGLES):
			if !state.authed || !state.handleSetActionBarToggles(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_ACTION_BUTTON):
			if !state.authed || !state.handleSetActionButton(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_WORLD_STATE_UI_TIMER_UPDATE):
			if !state.authed || !state.handleWorldStateUITimer() {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_RAID_INFO):
			if !state.authed || !state.handleRequestRaidInfo(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_READY_FOR_ACCOUNT_DATA_TIMES):
			if !state.authed || !state.handleReadyForAccountDataTimes() {
				return
			}
		case uint32(protocol.OpcodeCMSG_REALM_SPLIT):
			if !state.authed || !state.handleRealmSplit(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_CREATE):
			if !state.authed || !state.handleCharCreate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_DELETE):
			if !state.authed || !state.handleCharDelete(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PLAYER_LOGIN):
			if !state.authed || !state.handlePlayerLogin(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_OPENING_CINEMATIC):
			if !state.authed || !state.handleOpeningCinematic() {
				return
			}
		case uint32(protocol.OpcodeCMSG_NEXT_CINEMATIC_CAMERA):
			if !state.authed || !state.handleNextCinematicCamera() {
				return
			}
		case uint32(protocol.OpcodeCMSG_COMPLETE_CINEMATIC):
			if !state.authed || !state.handleCompleteCinematic(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_FACTION_ATWAR):
			if !state.authed || !state.handleSetFactionAtWar(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_STANDSTATECHANGE):
			if !state.authed || !state.handleStandStateChange(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_EMOTE):
			if !state.authed || !state.handleEmote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TEXT_EMOTE):
			if !state.authed || !state.handleTextEmote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_MOVE_WORLDPORT_ACK):
			if state.authed && state.player != nil && state.farTeleportPending && !state.completeWorldPort(ctx) {
				return
			}
		case uint32(protocol.OpcodeMSG_MOVE_TELEPORT_ACK):
			if !state.authed || !state.handleMoveTeleportAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_SET_CAN_FLY_ACK):
			if !state.authed || !state.handleMoveSetCanFlyAck(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_FORCE_RUN_SPEED_CHANGE_ACK), uint32(protocol.OpcodeCMSG_FORCE_RUN_BACK_SPEED_CHANGE_ACK),
			uint32(protocol.OpcodeCMSG_FORCE_SWIM_SPEED_CHANGE_ACK), uint32(protocol.OpcodeCMSG_FORCE_SWIM_BACK_SPEED_CHANGE_ACK),
			uint32(protocol.OpcodeCMSG_FORCE_WALK_SPEED_CHANGE_ACK), uint32(protocol.OpcodeCMSG_FORCE_FLIGHT_SPEED_CHANGE_ACK),
			uint32(protocol.OpcodeCMSG_FORCE_FLIGHT_BACK_SPEED_CHANGE_ACK):
			if !state.authed || !state.handleForceSpeedChangeAck(uint16(header.Opcode), ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MESSAGECHAT):
			if !state.authed || !state.handleMessageChat(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_SELECTION):
			if !state.authed || !state.handleSetSelection(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_ACTIVE_MOVER):
			if !state.authed || !state.handleSetActiveMover(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_JOIN):
			if !state.authed || !state.handleLFGJoin(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_LEAVE):
			if !state.authed || !state.handleLFGLeave() {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_GET_STATUS):
			if !state.authed || !state.handleLFGGetStatus() {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOGOUT_REQUEST):
			if !state.authed || !state.handleLogoutRequest(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PLAYER_LOGOUT):
			if !state.authed || !state.handlePlayerLogout() {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOGOUT_CANCEL):
			if !state.authed || !state.handleLogoutCancel() {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_WATCHED_FACTION):
			if !state.authed || !state.handleSetWatchedFaction(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_FACTION_INACTIVE):
			if !state.authed || !state.handleSetFactionInactive(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REPOP_REQUEST):
			if !state.authed || !state.handleRepopRequest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_RECLAIM_CORPSE):
			if !state.authed || !state.handleReclaimCorpse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_RESURRECT_RESPONSE):
			if !state.authed || !state.handleResurrectResponse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SELF_RES):
			if !state.authed || !state.handleSelfRes(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_HEARTH_AND_RESURRECT):
			if !state.authed || !state.handleHearthAndResurrect(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AREA_SPIRIT_HEALER_QUERY):
			if !state.authed || !state.handleAreaSpiritHealerQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AREA_SPIRIT_HEALER_QUEUE):
			if !state.authed || !state.handleAreaSpiritHealerQueue(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_WHO):
			if !state.authed || !state.handleWho(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_WHOIS):
			if !state.authed || !state.handleWhoIs(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_INSPECT):
			if !state.authed || !state.handleInspect(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAT_IGNORED):
			if !state.authed || !state.handleChatIgnored(payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LEARN_TALENT):
			if !state.authed || !state.handleLearnTalent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LEARN_PREVIEW_TALENTS):
			if !state.authed || !state.handleLearnPreviewTalents(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_UNLEARN_SKILL):
			if !state.authed || !state.handleUnlearnSkill(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ITEM_NAME_QUERY):
			if !state.authed || !state.handleItemNameQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ITEM_TEXT_QUERY):
			if !state.authed || !state.handleItemTextQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ITEM_REFUND_INFO):
			if !state.authed || !state.handleItemRefundInfo(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ITEM_REFUND):
			if !state.authed || !state.handleItemRefund(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_USE_ITEM):
			if !state.authed || !state.handleUseItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_EQUIPMENT_SET_SAVE):
			if !state.authed || !state.handleEquipmentSetSave(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_EQUIPMENT_SET_USE):
			if !state.authed || !state.handleEquipmentSetUse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DELETEEQUIPMENT_SET):
			if !state.authed || !state.handleEquipmentSetDelete(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GAMEOBJ_USE):
			if !state.authed || !state.handleGameObjectUse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GAMEOBJ_REPORT_USE):
			if !state.authed || !state.handleGameObjectReportUse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMTICKET_SYSTEMSTATUS):
			if !state.authed || !state.handleGMTicketSystemStatus(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMTICKET_GETTICKET):
			if !state.authed || !state.handleGMTicketGetTicket(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMTICKET_CREATE):
			if !state.authed || !state.handleGMTicketCreate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMTICKET_UPDATETEXT):
			if !state.authed || !state.handleGMTicketUpdate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMTICKET_DELETETICKET):
			if !state.authed || !state.handleGMTicketDelete(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMRESPONSE_RESOLVE):
			if !state.authed || !state.handleGMResponseResolve(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMSURVEY_SUBMIT):
			if !state.authed || !state.handleGMSurveySubmit(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GM_REPORT_LAG):
			if !state.authed || !state.handleGMReportLag(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BANKER_ACTIVATE):
			if !state.authed || !state.handleBankerActivate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BUY_BANK_SLOT):
			if !state.authed || !state.handleBuyBankSlot(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUTOBANK_ITEM):
			if !state.authed || !state.handleAutoBankItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUTOSTORE_BANK_ITEM):
			if !state.authed || !state.handleAutoStoreBankItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AREATRIGGER):
			if !state.authed || !state.handleAreaTrigger(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ALTER_APPEARANCE):
			if !state.authed || !state.handleAlterAppearance(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BINDER_ACTIVATE):
			if !state.authed || !state.handleBinderActivate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BUYBACK_ITEM):
			if !state.authed || !state.handleBuybackItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BUY_STABLE_SLOT):
			if !state.authed || !state.handleBuyStableSlot(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BUG):
			if !state.authed || !state.handleBug(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_AUCTION_LIST_PENDING_SALES):
			if !state.authed || !state.handleAuctionListPendingSales(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ACCEPT_LEVEL_GRANT):
			if !state.authed || !state.handleAcceptLevelGrant(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_QUERY):
			if !state.authed || !state.handleArenaTeamQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_ROSTER):
			if !state.authed || !state.handleArenaTeamRoster(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_INVITE):
			if !state.authed || !state.handleArenaTeamInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_ACCEPT):
			if !state.authed || !state.handleArenaTeamAccept(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_DECLINE):
			if !state.authed || !state.handleArenaTeamDecline(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_LEAVE):
			if !state.authed || !state.handleArenaTeamLeave(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_REMOVE):
			if !state.authed || !state.handleArenaTeamRemove(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_DISBAND):
			if !state.authed || !state.handleArenaTeamDisband(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ARENA_TEAM_LEADER):
			if !state.authed || !state.handleArenaTeamLeader(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEMASTER_HELLO):
			if !state.authed || !state.handleBattlemasterHello(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEFIELD_LIST):
			if !state.authed || !state.handleBattlefieldList(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEMASTER_JOIN):
			if !state.authed || !state.handleBattlemasterJoin(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEMASTER_JOIN_ARENA):
			if !state.authed || !state.handleBattlemasterJoinArena(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEFIELD_PORT):
			if !state.authed || !state.handleBattlefieldPort(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEFIELD_STATUS):
			if !state.authed || !state.handleBattlefieldStatus(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEFIELD_MGR_ENTRY_INVITE_RESPONSE):
			if !state.authed || !state.handleBfEntryInviteResponse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEFIELD_MGR_QUEUE_INVITE_RESPONSE):
			if !state.authed || !state.handleBfQueueInviteResponse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_BATTLEFIELD_MGR_EXIT_REQUEST):
			if !state.authed || !state.handleBfQueueExitRequest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_GET_CALENDAR):
			if !state.authed || !state.handleCalendarGetCalendar(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_GET_NUM_PENDING):
			if !state.authed || !state.handleCalendarGetNumPending(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_GET_EVENT):
			if !state.authed || !state.handleCalendarGetEvent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_GUILD_FILTER):
			if !state.authed || !state.handleCalendarGuildFilter(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_ARENA_TEAM):
			if !state.authed || !state.handleCalendarArenaTeam(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_ADD_EVENT):
			if !state.authed || !state.handleCalendarAddEvent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_UPDATE_EVENT):
			if !state.authed || !state.handleCalendarUpdateEvent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_REMOVE_EVENT):
			if !state.authed || !state.handleCalendarRemoveEvent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_COPY_EVENT):
			if !state.authed || !state.handleCalendarCopyEvent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_INVITE):
			if !state.authed || !state.handleCalendarEventInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_SIGNUP):
			if !state.authed || !state.handleCalendarEventSignup(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_RSVP):
			if !state.authed || !state.handleCalendarEventRSVP(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_REMOVE_INVITE):
			if !state.authed || !state.handleCalendarEventRemoveInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_STATUS):
			if !state.authed || !state.handleCalendarEventStatus(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_MODERATOR_STATUS):
			if !state.authed || !state.handleCalendarEventModeratorStatus(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CALENDAR_COMPLAIN):
			if !state.authed || !state.handleCalendarComplain(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_PASSWORD):
			if !state.authed || !state.handleChannelPassword(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_SET_OWNER):
			if !state.authed || !state.handleChannelSetOwner(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_OWNER):
			if !state.authed || !state.handleChannelOwner(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_MODERATOR):
			if !state.authed || !state.handleChannelModerator(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_UNMODERATOR):
			if !state.authed || !state.handleChannelUnmoderator(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_MUTE):
			if !state.authed || !state.handleChannelMute(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_UNMUTE):
			if !state.authed || !state.handleChannelUnmute(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_INVITE):
			if !state.authed || !state.handleChannelInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_KICK):
			if !state.authed || !state.handleChannelKick(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_BAN):
			if !state.authed || !state.handleChannelBan(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_UNBAN):
			if !state.authed || !state.handleChannelUnban(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_ANNOUNCEMENTS):
			if !state.authed || !state.handleChannelAnnouncements(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_VOICE_ON):
			if !state.authed || !state.handleChannelVoiceOn(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANNEL_MODERATE):
			if !state.authed || !state.handleChannelModerate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DECLINE_CHANNEL_INVITE):
			if !state.authed || !state.handleDeclineChannelInvite(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_RENAME):
			if !state.authed || !state.handleCharRename(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_CUSTOMIZE):
			if !state.authed || !state.handleCharCustomize(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_RACE_CHANGE):
			if !state.authed || !state.handleCharRaceChange(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHAR_FACTION_CHANGE):
			if !state.authed || !state.handleCharFactionChange(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_COMPLETE_MOVIE):
			if !state.authed || !state.handleCompleteMovie(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_COMPLAIN):
			if !state.authed || !state.handleComplain(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_CREATE):
			if !state.authed || !state.handleGuildCreate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_INFO):
			if !state.authed || !state.handleGuildInfo(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_PROMOTE):
			if !state.authed || !state.handleGuildPromote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_DEMOTE):
			if !state.authed || !state.handleGuildDemote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_LEADER):
			if !state.authed || !state.handleGuildLeader(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_REMOVE):
			if !state.authed || !state.handleGuildRemove(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_DISBAND):
			if !state.authed || !state.handleGuildDisband(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_ADD_RANK):
			if !state.authed || !state.handleGuildAddRank(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_DEL_RANK):
			if !state.authed || !state.handleGuildDelRank(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_RANK):
			if !state.authed || !state.handleGuildRank(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_SET_PUBLIC_NOTE):
			if !state.authed || !state.handleGuildSetPublicNote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_SET_OFFICER_NOTE):
			if !state.authed || !state.handleGuildSetOfficerNote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_INFO_TEXT):
			if !state.authed || !state.handleGuildInfoText(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANKER_ACTIVATE):
			if !state.authed || !state.handleGuildBankerActivate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANK_SWAP_ITEMS):
			if !state.authed || !state.handleGuildBankSwapItems(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANK_BUY_TAB):
			if !state.authed || !state.handleGuildBankBuyTab(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANK_UPDATE_TAB):
			if !state.authed || !state.handleGuildBankUpdateTab(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANK_DEPOSIT_MONEY):
			if !state.authed || !state.handleGuildBankDepositMoney(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GUILD_BANK_WITHDRAW_MONEY):
			if !state.authed || !state.handleGuildBankWithdrawMoney(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_GUILD_BANK_LOG_QUERY):
			if !state.authed || !state.handleGuildBankLogQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_GUILD_BANK_MONEY_WITHDRAWN):
			if !state.authed || !state.handleGuildBankMoneyWithdrawn(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_QUERY_GUILD_BANK_TEXT):
			if !state.authed || !state.handleQueryGuildBankText(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_GUILD_BANK_TEXT):
			if !state.authed || !state.handleSetGuildBankText(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DUEL_ACCEPTED):
			if !state.authed || !state.handleDuelAccepted(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DUEL_CANCELLED):
			if !state.authed || !state.handleDuelCancelled(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_FAR_SIGHT):
			if !state.authed || !state.handleFarSight(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_FORCE_MOVE_ROOT_ACK):
			if !state.authed || !state.handleForceMoveRootAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_FORCE_MOVE_UNROOT_ACK):
			if !state.authed || !state.handleForceMoveUnrootAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_FORCE_TURN_RATE_CHANGE_ACK):
			// Opcodes.cpp:866 routes 0x2DF to HandleForceSpeedChangeAck
			// (MOVE_TURN_RATE), which never applies the acked position —
			// the turn-rate ack must not take the position-applying
			// handleMovementAck path.
			if !state.authed || !state.handleForceSpeedChangeAck(uint16(protocol.OpcodeCMSG_FORCE_TURN_RATE_CHANGE_ACK), ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GET_CHANNEL_MEMBER_COUNT):
			if !state.authed || !state.handleGetChannelMemberCount(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GET_MIRRORIMAGE_DATA):
			if !state.authed || !state.handleGetMirrorImageData(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GMTICKETSYSTEM_TOGGLE):
			if !state.authed || !state.handleGmTicketSystemToggle(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GRANT_LEVEL):
			if !state.authed || !state.handleGrantLevel(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_ASSISTANT_LEADER):
			if !state.authed || !state.handleGroupAssistantLeader(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_CHANGE_SUB_GROUP):
			if !state.authed || !state.handleGroupChangeSubGroup(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_ENABLETAXI):
			if !state.authed || !state.handleEnableTaxi(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DISMISS_CRITTER):
			if !state.authed || !state.handleDismissCritter(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CHANGE_SEATS_ON_CONTROLLED_VEHICLE):
			if !state.authed || !state.handleChangeSeatsOnControlledVehicle(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_CONTROLLER_EJECT_PASSENGER):
			if !state.authed || !state.handleControllerEjectPassenger(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_DISMISS_CONTROLLED_VEHICLE):
			if !state.authed || !state.handleDismissControlledVehicle(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TUTORIAL_FLAG):
			if !state.authed || !state.handleTutorialFlag(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TUTORIAL_CLEAR):
			if !state.authed || !state.handleTutorialClear(ctx) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TUTORIAL_RESET):
			if !state.authed || !state.handleTutorialReset(ctx) {
				return
			}

		// Petitions & Guild Tabards
		case uint32(protocol.OpcodeCMSG_PETITION_BUY):
			if !state.authed || !state.handlePetitionBuy(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PETITION_SHOW_SIGNATURES):
			if !state.authed || !state.handlePetitionShowSignatures(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PETITION_QUERY):
			if !state.authed || !state.handlePetitionQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PETITION_SIGN):
			if !state.authed || !state.handlePetitionSign(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TURN_IN_PETITION):
			if !state.authed || !state.handleTurnInPetition(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_OFFER_PETITION):
			if !state.authed || !state.handleOfferPetition(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PETITION_SHOWLIST):
			if !state.authed || !state.handlePetitionShowList(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_PETITION_DECLINE):
			if !state.authed || !state.handlePetitionDecline(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_PETITION_RENAME):
			if !state.authed || !state.handlePetitionRename(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_TABARDVENDOR_ACTIVATE):
			if !state.authed || !state.handleTabardVendorActivate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_SAVE_GUILD_EMBLEM):
			if !state.authed || !state.handleSaveGuildEmblem(ctx, payload) {
				return
			}

		// Movement ACKs, Summons & Animations
		case uint32(protocol.OpcodeCMSG_MOVE_FEATHER_FALL_ACK):
			if !state.authed || !state.handleMoveFeatherFallAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_HOVER_ACK):
			if !state.authed || !state.handleMoveHoverAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_WATER_WALK_ACK):
			if !state.authed || !state.handleMoveWaterWalkAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_KNOCK_BACK_ACK):
			if !state.authed || !state.handleMoveKnockBackAck(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_NOT_ACTIVE_MOVER):
			if !state.authed || !state.handleMoveNotActiveMover(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_FALL_RESET):
			if !state.authed || !state.handleMoveFallReset(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_SPLINE_DONE):
			if !state.authed || !state.handleMoveSplineDone(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_CHNG_TRANSPORT):
			if !state.authed || !state.handleMoveChngTransport(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_SET_FLY):
			if !state.authed || !state.handleMoveSetFly(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOVE_TIME_SKIPPED):
			if !state.authed || !state.handleMoveTimeSkipped(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SUMMON_RESPONSE):
			if !state.authed || !state.handleSummonResponse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MOUNTSPECIAL_ANIM):
			if !state.authed || !state.handleMountSpecialAnim(ctx, payload) {
				return
			}

		// Vehicle Passengers & Seats
		case uint32(protocol.OpcodeCMSG_PLAYER_VEHICLE_ENTER):
			if !state.authed || !state.handlePlayerVehicleEnter(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_VEHICLE_EXIT):
			if !state.authed || !state.handleRequestVehicleExit(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_VEHICLE_NEXT_SEAT):
			if !state.authed || !state.handleRequestVehicleNextSeat(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_VEHICLE_PREV_SEAT):
			if !state.authed || !state.handleRequestVehiclePrevSeat(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_VEHICLE_SWITCH_SEAT):
			if !state.authed || !state.handleRequestVehicleSwitchSeat(ctx, payload) {
				return
			}

		// Items & Page Text
		case uint32(protocol.OpcodeCMSG_OPEN_ITEM):
			if !state.authed || !state.handleOpenItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_READ_ITEM):
			if !state.authed || !state.handleReadItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PAGE_TEXT_QUERY):
			if !state.authed || !state.handlePageTextQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_WRAP_ITEM):
			if !state.authed || !state.handleWrapItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REPAIR_ITEM):
			if !state.authed || !state.handleRepairItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SOCKET_GEMS):
			if !state.authed || !state.handleSocketGems(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_AMMO):
			if !state.authed || !state.handleSetAmmo(ctx, payload) {
				return
			}

		// Character Display, Titles & PvP
		case uint32(protocol.OpcodeCMSG_SHOWING_CLOAK):
			if !state.authed || !state.handleShowingCloak(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SHOWING_HELM):
			if !state.authed || !state.handleShowingHelm(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_TITLE):
			if !state.authed || !state.handleSetTitle(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_TOGGLE_PVP):
			if !state.authed || !state.handleTogglePvP(ctx, payload) {
				return
			}

		// Instances & Difficulty
		case uint32(protocol.OpcodeCMSG_RESET_INSTANCES):
			if !state.authed || !state.handleResetInstances(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_SET_DUNGEON_DIFFICULTY):
			if !state.authed || !state.handleSetDungeonDifficulty(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_SET_RAID_DIFFICULTY):
			if !state.authed || !state.handleSetRaidDifficulty(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_INSTANCE_LOCK_RESPONSE):
			if !state.authed || !state.handleInstanceLockResponse(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_SAVED_INSTANCE_EXTEND):
			if !state.authed || !state.handleSetSavedInstanceExtend(ctx, payload) {
				return
			}

		// Guild Permissions, Event Log & Inspect
		case uint32(protocol.OpcodeMSG_GUILD_EVENT_LOG_QUERY):
			if !state.authed || !state.handleGuildEventLogQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_GUILD_PERMISSIONS):
			if !state.authed || !state.handleGuildPermissions(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_INSPECT_ARENA_TEAMS):
			if !state.authed || !state.handleInspectArenaTeams(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_INSPECT_HONOR_STATS):
			if !state.authed || !state.handleInspectHonorStats(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_PVP_LOG_DATA):
			if !state.authed || !state.handlePvpLogData(ctx, payload) {
				return
			}

		// Spirit Healer & Corpse
		case uint32(protocol.OpcodeCMSG_SPIRIT_HEALER_ACTIVATE):
			if !state.authed || !state.handleSpiritHealerActivate(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_CORPSE_QUERY):
			if !state.authed || !state.handleCorpseQuery(ctx, payload) {
				return
			}

		// Spells & Talents
		case uint32(protocol.OpcodeCMSG_TOTEM_DESTROYED):
			if !state.authed || !state.handleTotemDestroyed(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SPELLCLICK):
			if !state.authed || !state.handleSpellClick(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_TALENT_WIPE_CONFIRM):
			if !state.authed || !state.handleTalentWipeConfirm(ctx, payload) {
				return
			}

		// Quests & Inspect Achievements
		case uint32(protocol.OpcodeCMSG_QUEST_CONFIRM_ACCEPT):
			if !state.authed || !state.handleQuestConfirmAccept(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUEST_POI_QUERY):
			if !state.authed || !state.handleQuestPoiQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUERY_QUESTS_COMPLETED):
			if !state.authed || !state.handleQueryQuestsCompleted(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTLOG_SWAP_QUEST):
			if !state.authed || !state.handleQuestlogSwapQuest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PUSHQUESTTOPARTY):
			if !state.authed || !state.handlePushQuestToParty(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_QUEST_PUSH_RESULT):
			if !state.authed || !state.handleQuestPushResult(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUESTGIVER_STATUS_MULTIPLE_QUERY):
			if !state.authed || !state.handleQuestgiverStatusMultipleQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_QUERY_INSPECT_ACHIEVEMENTS):
			if !state.authed || !state.handleQueryInspectAchievements(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_RAID_READY_CHECK_FINISHED):
			if !state.authed || !state.handleRaidReadyCheckFinished(ctx, payload) {
				return
			}

		// Pets & Pet Stabling
		case uint32(protocol.OpcodeCMSG_PET_ABANDON):
			if !state.authed || !state.handlePetAbandon(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_ACTION):
			if !state.authed || !state.handlePetAction(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_CANCEL_AURA):
			if !state.authed || !state.handlePetCancelAura(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_CAST_SPELL):
			if !state.authed || !state.handlePetCastSpell(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_LEARN_TALENT):
			if !state.authed || !state.handlePetLearnTalent(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_NAME_QUERY):
			if !state.authed || !state.handlePetNameQuery(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_RENAME):
			if !state.authed || !state.handlePetRename(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_SET_ACTION):
			if !state.authed || !state.handlePetSetAction(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_SPELL_AUTOCAST):
			if !state.authed || !state.handlePetSpellAutocast(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_PET_STOP_ATTACK):
			if !state.authed || !state.handlePetStopAttack(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REQUEST_PET_INFO):
			if !state.authed || !state.handleRequestPetInfo(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_STABLE_PET):
			if !state.authed || !state.handleStablePet(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_STABLE_REVIVE_PET):
			if !state.authed || !state.handleStableRevivePet(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_STABLE_SWAP_PET):
			if !state.authed || !state.handleStableSwapPet(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_UNSTABLE_PET):
			if !state.authed || !state.handleUnstablePet(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_LIST_STABLED_PETS):
			if !state.authed || !state.handleListStabledPets(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LEARN_PREVIEW_TALENTS_PET):
			if !state.authed || !state.handleLearnPreviewTalentsPet(ctx, payload) {
				return
			}

		// LFG / Dungeon Finder
		case uint32(protocol.OpcodeCMSG_LFD_PARTY_LOCK_INFO_REQUEST):
			if !state.authed || !state.handleLfdPartyLockInfoRequest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFD_PLAYER_LOCK_INFO_REQUEST):
			if !state.authed || !state.handleLfdPlayerLockInfoRequest(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_PROPOSAL_RESULT):
			if !state.authed || !state.handleLfgProposalResult(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_SET_BOOT_VOTE):
			if !state.authed || !state.handleLfgSetBootVote(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_SET_ROLES):
			if !state.authed || !state.handleLfgSetRoles(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LFG_TELEPORT):
			if !state.authed || !state.handleLfgTeleport(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SEARCH_LFG_JOIN):
			if !state.authed || !state.handleSearchLfgJoin(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SEARCH_LFG_LEAVE):
			if !state.authed || !state.handleSearchLfgLeave(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_LFG_COMMENT):
			if !state.authed || !state.handleSetLfgComment(ctx, payload) {
				return
			}

		// Loot
		case uint32(protocol.OpcodeCMSG_LOOT_MASTER_GIVE):
			if !state.authed || !state.handleLootMasterGive(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_LOOT_ROLL):
			if !state.authed || !state.handleLootRoll(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_OPT_OUT_OF_LOOT):
			if !state.authed || !state.handleOptOutOfLoot(ctx, payload) {
				return
			}

		// Mail
		case uint32(protocol.OpcodeCMSG_MAIL_CREATE_TEXT_ITEM):
			if !state.authed || !state.handleMailCreateTextItem(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_MAIL_RETURN_TO_SENDER):
			if !state.authed || !state.handleMailReturnToSender(ctx, payload) {
				return
			}

		// Battleground & PvP
		case uint32(protocol.OpcodeCMSG_LEAVE_BATTLEFIELD):
			if !state.authed || !state.handleLeaveBattlefield(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_REPORT_PVP_AFK):
			if !state.authed || !state.handleReportPvPAfk(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeMSG_BATTLEGROUND_PLAYER_POSITIONS):
			if !state.authed || !state.handleBattlegroundPlayerPositions(ctx, payload) {
				return
			}

		// Spells & Glyphs
		case uint32(protocol.OpcodeCMSG_REMOVE_GLYPH):
			if !state.authed || !state.handleRemoveGlyph(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_UPDATE_MISSILE_TRAJECTORY):
			if !state.authed || !state.handleUpdateMissileTrajectory(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_UPDATE_PROJECTILE_POSITION):
			if !state.authed || !state.handleUpdateProjectilePosition(ctx, payload) {
				return
			}

		// Group
		case uint32(protocol.OpcodeCMSG_REQUEST_PARTY_MEMBER_STATS):
			if !state.authed || !state.handleRequestPartyMemberStats(ctx, payload) {
				return
			}

		// Voice & Channels
		case uint32(protocol.OpcodeCMSG_SET_ACTIVE_VOICE_CHANNEL):
			if !state.authed || !state.handleSetActiveVoiceChannel(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_VOICE_SESSION_ENABLE):
			if !state.authed || !state.handleVoiceSessionEnable(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_SET_CHANNEL_WATCH):
			if !state.authed || !state.handleSetChannelWatch(ctx, payload) {
				return
			}

		// Commands & Admin
		case uint32(protocol.OpcodeCMSG_SET_FACTION_CHEAT):
			if !state.authed || !state.handleSetFactionCheat(ctx, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_WORLD_TELEPORT):
			if !state.authed || !state.handleWorldTeleport(ctx, payload) {
				return
			}

		// Character Declined Names
		case uint32(protocol.OpcodeCMSG_SET_PLAYER_DECLINED_NAMES):
			if !state.authed || !state.handleSetPlayerDeclinedNames(ctx, payload) {
				return
			}

		// Taxi
		case uint32(protocol.OpcodeCMSG_SET_TAXI_BENCHMARK_MODE):
			if !state.authed || !state.handleSetTaxiBenchmarkMode(ctx, payload) {
				return
			}

		// Warden
		case uint32(protocol.OpcodeCMSG_WARDEN_DATA):
			if !state.authed || !state.handleWardenData(ctx, payload) {
				return
			}

		case uint32(protocol.OpcodeMSG_MOVE_START_FORWARD), uint32(protocol.OpcodeMSG_MOVE_START_BACKWARD), uint32(protocol.OpcodeMSG_MOVE_STOP), uint32(protocol.OpcodeMSG_MOVE_START_STRAFE_LEFT), uint32(protocol.OpcodeMSG_MOVE_START_STRAFE_RIGHT), uint32(protocol.OpcodeMSG_MOVE_STOP_STRAFE), uint32(protocol.OpcodeMSG_MOVE_JUMP), uint32(protocol.OpcodeMSG_MOVE_START_TURN_LEFT), uint32(protocol.OpcodeMSG_MOVE_START_TURN_RIGHT), uint32(protocol.OpcodeMSG_MOVE_STOP_TURN), uint32(protocol.OpcodeMSG_MOVE_START_PITCH_UP), uint32(protocol.OpcodeMSG_MOVE_START_PITCH_DOWN), uint32(protocol.OpcodeMSG_MOVE_STOP_PITCH), uint32(protocol.OpcodeMSG_MOVE_SET_RUN_MODE), uint32(protocol.OpcodeMSG_MOVE_SET_WALK_MODE), uint32(protocol.OpcodeMSG_MOVE_FALL_LAND), uint32(protocol.OpcodeMSG_MOVE_START_SWIM), uint32(protocol.OpcodeMSG_MOVE_STOP_SWIM), uint32(protocol.OpcodeMSG_MOVE_HEARTBEAT), uint32(protocol.OpcodeMSG_MOVE_HOVER), uint32(protocol.OpcodeMSG_MOVE_SET_FACING), uint32(protocol.OpcodeMSG_MOVE_SET_PITCH), uint32(protocol.OpcodeMSG_MOVE_START_ASCEND), uint32(protocol.OpcodeMSG_MOVE_START_DESCEND), uint32(protocol.OpcodeMSG_MOVE_STOP_ASCEND), uint32(protocol.OpcodeMSG_MOVE_GRAVITY_CHNG):
			if !state.authed || !state.handleMovement(ctx, header.Opcode, payload) {
				return
			}
		case uint32(protocol.OpcodeCMSG_GROUP_CANCEL), uint32(protocol.OpcodeCMSG_TELEPORT_TO_UNIT), uint32(protocol.OpcodeCMSG_UNUSED5):
			if !state.authed || !state.playerLoaded {
				return
			}
			state.debug("reference logged-in no-op", "account", state.accountName, "opcode", opcodeName(header.Opcode), "size", len(payload))
		// Reference client opcodes intentionally bound to Handle_NULL (TrinityCore Opcodes.cpp)
		case uint32(protocol.OpcodeCMSG_ACTIVE_PVP_CHEAT),
			uint32(protocol.OpcodeCMSG_ADD_PVP_MEDAL_CHEAT),
			uint32(protocol.OpcodeCMSG_ADD_VOICE_IGNORE),
			uint32(protocol.OpcodeCMSG_ADVANCE_SPAWN_TIME),
			uint32(protocol.OpcodeCMSG_AFK_MONITOR_INFO_CLEAR),
			uint32(protocol.OpcodeCMSG_AFK_MONITOR_INFO_REQUEST),
			uint32(protocol.OpcodeCMSG_ARENA_TEAM_CREATE),
			uint32(protocol.OpcodeCMSG_AUTH_SRP6_BEGIN),
			uint32(protocol.OpcodeCMSG_AUTH_SRP6_PROOF),
			uint32(protocol.OpcodeCMSG_AUTH_SRP6_RECODE),
			uint32(protocol.OpcodeCMSG_AUTOEQUIP_GROUND_ITEM),
			uint32(protocol.OpcodeCMSG_AUTOSTORE_GROUND_ITEM),
			uint32(protocol.OpcodeCMSG_BATTLEFIELD_JOIN),
			uint32(protocol.OpcodeCMSG_BATTLEFIELD_MANAGER_ADVANCE_STATE),
			uint32(protocol.OpcodeCMSG_BATTLEFIELD_MANAGER_SET_NEXT_TRANSITION_TIME),
			uint32(protocol.OpcodeCMSG_BATTLEFIELD_MGR_QUEUE_REQUEST),
			uint32(protocol.OpcodeCMSG_BEASTMASTER),
			uint32(protocol.OpcodeCMSG_BOOTME),
			uint32(protocol.OpcodeCMSG_BOT_DETECTED),
			uint32(protocol.OpcodeCMSG_BOT_DETECTED2),
			uint32(protocol.OpcodeCMSG_BUY_LOTTERY_TICKET_OBSOLETE),
			uint32(protocol.OpcodeCMSG_CALENDAR_EVENT_INVITE_NOTES),
			uint32(protocol.OpcodeCMSG_CHANGEPLAYER_DIFFICULTY),
			uint32(protocol.OpcodeCMSG_CHANGE_GDF_ARENA_RATING),
			uint32(protocol.OpcodeCMSG_CHANGE_PERSONAL_ARENA_RATING),
			uint32(protocol.OpcodeCMSG_CHANNEL_SILENCE_ALL),
			uint32(protocol.OpcodeCMSG_CHANNEL_SILENCE_VOICE),
			uint32(protocol.OpcodeCMSG_CHANNEL_UNSILENCE_ALL),
			uint32(protocol.OpcodeCMSG_CHANNEL_UNSILENCE_VOICE),
			uint32(protocol.OpcodeCMSG_CHANNEL_VOICE_OFF),
			uint32(protocol.OpcodeCMSG_CHARACTER_POINT_CHEAT),
			uint32(protocol.OpcodeCMSG_CHAT_FILTERED),
			uint32(protocol.OpcodeCMSG_CHEAT_DUMP_ITEMS_DEBUG_ONLY),
			uint32(protocol.OpcodeCMSG_CHEAT_PLAYER_LOGIN),
			uint32(protocol.OpcodeCMSG_CHEAT_PLAYER_LOOKUP),
			uint32(protocol.OpcodeCMSG_CHEAT_SETMONEY),
			uint32(protocol.OpcodeCMSG_CHEAT_SET_ARENA_CURRENCY),
			uint32(protocol.OpcodeCMSG_CHEAT_SET_HONOR_CURRENCY),
			uint32(protocol.OpcodeCMSG_CHECK_LOGIN_CRITERIA),
			uint32(protocol.OpcodeCMSG_CLEAR_CHANNEL_WATCH),
			uint32(protocol.OpcodeCMSG_CLEAR_EXPLORATION),
			uint32(protocol.OpcodeCMSG_CLEAR_HOLIDAY_BG_WIN_TIME),
			uint32(protocol.OpcodeCMSG_CLEAR_QUEST),
			uint32(protocol.OpcodeCMSG_CLEAR_RANDOM_BG_WIN_TIME),
			uint32(protocol.OpcodeCMSG_CLEAR_SERVER_BUCK_DATA),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_ENABLE),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_ENTER_INSTANCE),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_EXIT_INSTANCE),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_GET_MAP_INFO),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_GET_PLAYER_INFO),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_INSTANCE_COMMAND),
			uint32(protocol.OpcodeCMSG_COMMENTATOR_SKIRMISH_QUEUE_COMMAND),
			uint32(protocol.OpcodeCMSG_COMPLETE_ACHIEVEMENT_CHEAT),
			uint32(protocol.OpcodeCMSG_COOLDOWN_CHEAT),
			uint32(protocol.OpcodeCMSG_CREATEGAMEOBJECT),
			uint32(protocol.OpcodeCMSG_CREATEITEM),
			uint32(protocol.OpcodeCMSG_CREATEMONSTER),
			uint32(protocol.OpcodeCMSG_DANCE_QUERY),
			uint32(protocol.OpcodeCMSG_DBLOOKUP),
			uint32(protocol.OpcodeCMSG_DEBUG_ACTIONS_START),
			uint32(protocol.OpcodeCMSG_DEBUG_ACTIONS_STOP),
			uint32(protocol.OpcodeCMSG_DEBUG_AISTATE),
			uint32(protocol.OpcodeCMSG_DEBUG_CHANGECELLZONE),
			uint32(protocol.OpcodeCMSG_DEBUG_LIST_TARGETS),
			uint32(protocol.OpcodeCMSG_DEBUG_PASSIVE_AURA),
			uint32(protocol.OpcodeCMSG_DEBUG_SERVER_GEO),
			uint32(protocol.OpcodeCMSG_DECHARGE),
			uint32(protocol.OpcodeCMSG_DELETE_DANCE),
			uint32(protocol.OpcodeCMSG_DEL_PVP_MEDAL_CHEAT),
			uint32(protocol.OpcodeCMSG_DEL_VOICE_IGNORE),
			uint32(protocol.OpcodeCMSG_DESTROYMONSTER),
			uint32(protocol.OpcodeCMSG_DESTROY_ITEMS),
			uint32(protocol.OpcodeCMSG_DISABLE_PVP_CHEAT),
			uint32(protocol.OpcodeCMSG_DROP_NEW_CONNECTION),
			uint32(protocol.OpcodeCMSG_DUMP_OBJECTS),
			uint32(protocol.OpcodeCMSG_ENABLE_DAMAGE_LOG),
			uint32(protocol.OpcodeCMSG_END_BATTLEFIELD_CHEAT),
			uint32(protocol.OpcodeCMSG_EXPIRE_RAID_INSTANCE),
			uint32(protocol.OpcodeCMSG_FLAG_QUEST),
			uint32(protocol.OpcodeCMSG_FLAG_QUEST_FINISH),
			uint32(protocol.OpcodeCMSG_FLOOD_GRACE_CHEAT),
			uint32(protocol.OpcodeCMSG_FORCEACTION),
			uint32(protocol.OpcodeCMSG_FORCEACTIONONOTHER),
			uint32(protocol.OpcodeCMSG_FORCEACTIONSHOW),
			uint32(protocol.OpcodeCMSG_FORCE_ANIM),
			uint32(protocol.OpcodeCMSG_FORCE_SAY_CHEAT),
			// CMSG_FORCE_PITCH_RATE_CHANGE_ACK (0x45D) is STATUS_NEVER +
			// Handle_NULL in Opcodes.cpp: discarded, never dispatched to the
			// speed-ack handler.
			uint32(protocol.OpcodeCMSG_FORCE_PITCH_RATE_CHANGE_ACK),
			uint32(protocol.OpcodeCMSG_GAMESPEED_SET),
			uint32(protocol.OpcodeCMSG_GAMETIME_SET),
			uint32(protocol.OpcodeCMSG_GETDEATHBINDZONE),
			uint32(protocol.OpcodeCMSG_GHOST),
			uint32(protocol.OpcodeCMSG_GMRESPONSE_CREATE_TICKET),
			uint32(protocol.OpcodeCMSG_GM_CHARACTER_RESTORE),
			uint32(protocol.OpcodeCMSG_GM_CHARACTER_SAVE),
			uint32(protocol.OpcodeCMSG_GM_CREATE_ITEM_TARGET),
			uint32(protocol.OpcodeCMSG_GM_DESTROY_ONLINE_CORPSE),
			uint32(protocol.OpcodeCMSG_GM_FREEZE),
			uint32(protocol.OpcodeCMSG_GM_GRANT_ACHIEVEMENT),
			uint32(protocol.OpcodeCMSG_GM_INVIS),
			uint32(protocol.OpcodeCMSG_GM_MOVECORPSE),
			uint32(protocol.OpcodeCMSG_GM_NUKE),
			uint32(protocol.OpcodeCMSG_GM_NUKE_ACCOUNT),
			uint32(protocol.OpcodeCMSG_GM_NUKE_CHARACTER),
			uint32(protocol.OpcodeCMSG_GM_REMOVE_ACHIEVEMENT),
			uint32(protocol.OpcodeCMSG_GM_REQUEST_PLAYER_INFO),
			uint32(protocol.OpcodeCMSG_GM_RESURRECT),
			uint32(protocol.OpcodeCMSG_GM_REVEALTO),
			uint32(protocol.OpcodeCMSG_GM_SET_CRITERIA_FOR_PLAYER),
			uint32(protocol.OpcodeCMSG_GM_SET_SECURITY_GROUP),
			uint32(protocol.OpcodeCMSG_GM_SHOW_COMPLAINTS),
			uint32(protocol.OpcodeCMSG_GM_SILENCE),
			uint32(protocol.OpcodeCMSG_GM_SUMMONMOB),
			uint32(protocol.OpcodeCMSG_GM_TEACH),
			uint32(protocol.OpcodeCMSG_GM_UBERINVIS),
			uint32(protocol.OpcodeCMSG_GM_UNSQUELCH),
			uint32(protocol.OpcodeCMSG_GM_UNTEACH),
			uint32(protocol.OpcodeCMSG_GM_UPDATE_TICKET_STATUS),
			uint32(protocol.OpcodeCMSG_GM_VISION),
			uint32(protocol.OpcodeCMSG_GM_WHISPER),
			uint32(protocol.OpcodeCMSG_GODMODE),
			uint32(protocol.OpcodeCMSG_GROUP_SWAP_SUB_GROUP),
			uint32(protocol.OpcodeCMSG_IGNORE_DIMINISHING_RETURNS_CHEAT),
			uint32(protocol.OpcodeCMSG_IGNORE_KNOCKBACK_CHEAT),
			uint32(protocol.OpcodeCMSG_IGNORE_REQUIREMENTS_CHEAT),
			uint32(protocol.OpcodeCMSG_ITEM_QUERY_MULTIPLE),
			uint32(protocol.OpcodeCMSG_LEARN_DANCE_MOVE),
			uint32(protocol.OpcodeCMSG_LEARN_SPELL),
			uint32(protocol.OpcodeCMSG_LEVEL_CHEAT),
			uint32(protocol.OpcodeCMSG_LFG_SET_NEEDS),
			uint32(protocol.OpcodeCMSG_LOAD_DANCES),
			uint32(protocol.OpcodeCMSG_LOTTERY_QUERY_OBSOLETE),
			uint32(protocol.OpcodeCMSG_LUA_USAGE),
			uint32(protocol.OpcodeCMSG_MAELSTROM_GM_SENT_MAIL),
			uint32(protocol.OpcodeCMSG_MAELSTROM_INVALIDATE_CACHE),
			uint32(protocol.OpcodeCMSG_MAELSTROM_RENAME_GUILD),
			uint32(protocol.OpcodeCMSG_MAKEMONSTERATTACKGUID),
			uint32(protocol.OpcodeCMSG_MINIGAME_MOVE),
			uint32(protocol.OpcodeCMSG_MOVE_CHARACTER_CHEAT),
			uint32(protocol.OpcodeCMSG_MOVE_CHARM_PORT_CHEAT),
			uint32(protocol.OpcodeCMSG_MOVE_GRAVITY_DISABLE_ACK),
			uint32(protocol.OpcodeCMSG_MOVE_GRAVITY_ENABLE_ACK),
			uint32(protocol.OpcodeCMSG_MOVE_SET_CAN_TRANSITION_BETWEEN_SWIM_AND_FLY_ACK),
			uint32(protocol.OpcodeCMSG_MOVE_SET_COLLISION_HGT_ACK),
			uint32(protocol.OpcodeCMSG_MOVE_SET_RAW_POSITION),
			uint32(protocol.OpcodeCMSG_MOVE_SET_RUN_SPEED),
			uint32(protocol.OpcodeCMSG_MOVE_START_SWIM_CHEAT),
			uint32(protocol.OpcodeCMSG_MOVE_STOP_SWIM_CHEAT),
			uint32(protocol.OpcodeCMSG_NEW_SPELL_SLOT),
			uint32(protocol.OpcodeCMSG_NO_SPELL_VARIANCE),
			uint32(protocol.OpcodeCMSG_PARTY_SILENCE),
			uint32(protocol.OpcodeCMSG_PARTY_UNSILENCE),
			uint32(protocol.OpcodeCMSG_PERFORM_ACTION_SET),
			uint32(protocol.OpcodeCMSG_PETGODMODE),
			uint32(protocol.OpcodeCMSG_PET_LEVEL_CHEAT),
			uint32(protocol.OpcodeCMSG_PET_UNLEARN),
			uint32(protocol.OpcodeCMSG_PET_UNLEARN_TALENTS),
			uint32(protocol.OpcodeCMSG_PLAYER_AI_CHEAT),
			uint32(protocol.OpcodeCMSG_PLAY_DANCE),
			uint32(protocol.OpcodeCMSG_PROFILEDATA_REQUEST),
			uint32(protocol.OpcodeCMSG_PVP_QUEUE_STATS_REQUEST),
			uint32(protocol.OpcodeCMSG_QUERY_OBJECT_POSITION),
			uint32(protocol.OpcodeCMSG_QUERY_OBJECT_ROTATION),
			uint32(protocol.OpcodeCMSG_QUERY_SERVER_BUCK_DATA),
			uint32(protocol.OpcodeCMSG_QUERY_VEHICLE_STATUS),
			uint32(protocol.OpcodeCMSG_RECHARGE),
			uint32(protocol.OpcodeCMSG_REDIRECTION_AUTH_PROOF),
			uint32(protocol.OpcodeCMSG_REDIRECTION_FAILED),
			uint32(protocol.OpcodeCMSG_REFER_A_FRIEND),
			uint32(protocol.OpcodeCMSG_RESET_FACTION_CHEAT),
			uint32(protocol.OpcodeCMSG_RUN_SCRIPT),
			uint32(protocol.OpcodeCMSG_SAVE_DANCE),
			uint32(protocol.OpcodeCMSG_SAVE_PLAYER),
			uint32(protocol.OpcodeCMSG_SEND_COMBAT_TRIGGER),
			uint32(protocol.OpcodeCMSG_SEND_EVENT),
			uint32(protocol.OpcodeCMSG_SEND_GENERAL_TRIGGER),
			uint32(protocol.OpcodeCMSG_SEND_LOCAL_EVENT),
			uint32(protocol.OpcodeCMSG_SERVERINFO),
			uint32(protocol.OpcodeCMSG_SERVERTIME),
			uint32(protocol.OpcodeCMSG_SERVER_BROADCAST),
			uint32(protocol.OpcodeCMSG_SERVER_COMMAND),
			uint32(protocol.OpcodeCMSG_SERVER_INFO_QUERY),
			uint32(protocol.OpcodeCMSG_SETDEATHBINDPOINT),
			uint32(protocol.OpcodeCMSG_SET_ACTIVE_TALENT_GROUP_OBSOLETE),
			uint32(protocol.OpcodeCMSG_SET_ALLOW_LOW_LEVEL_RAID1),
			uint32(protocol.OpcodeCMSG_SET_ALLOW_LOW_LEVEL_RAID2),
			uint32(protocol.OpcodeCMSG_SET_ARENA_MEMBER_SEASON_GAMES),
			uint32(protocol.OpcodeCMSG_SET_ARENA_MEMBER_WEEKLY_GAMES),
			uint32(protocol.OpcodeCMSG_SET_ARENA_TEAM_RATING_BY_INDEX),
			uint32(protocol.OpcodeCMSG_SET_ARENA_TEAM_SEASON_GAMES),
			uint32(protocol.OpcodeCMSG_SET_ARENA_TEAM_WEEKLY_GAMES),
			uint32(protocol.OpcodeCMSG_SET_BREATH),
			uint32(protocol.OpcodeCMSG_SET_CHARACTER_MODEL),
			uint32(protocol.OpcodeCMSG_SET_CRITERIA_CHEAT),
			uint32(protocol.OpcodeCMSG_SET_DURABILITY_CHEAT),
			uint32(protocol.OpcodeCMSG_SET_EXPLORATION),
			uint32(protocol.OpcodeCMSG_SET_EXPLORATION_ALL),
			uint32(protocol.OpcodeCMSG_SET_GLYPH),
			uint32(protocol.OpcodeCMSG_SET_GLYPH_SLOT),
			uint32(protocol.OpcodeCMSG_SET_GRANTABLE_LEVELS),
			uint32(protocol.OpcodeCMSG_SET_PAID_SERVICE_CHEAT),
			uint32(protocol.OpcodeCMSG_SET_PVP_RANK_CHEAT),
			uint32(protocol.OpcodeCMSG_SET_PVP_TITLE),
			uint32(protocol.OpcodeCMSG_SET_RUNE_COOLDOWN),
			uint32(protocol.OpcodeCMSG_SET_RUNE_COUNT),
			uint32(protocol.OpcodeCMSG_SET_SKILL_CHEAT),
			uint32(protocol.OpcodeCMSG_SET_STAT_CHEAT),
			uint32(protocol.OpcodeCMSG_SET_TITLE_SUFFIX),
			uint32(protocol.OpcodeCMSG_SET_VEHICLE_REC_ID_ACK),
			uint32(protocol.OpcodeCMSG_SET_WORLDSTATE),
			uint32(protocol.OpcodeCMSG_SKILL_BUY_RANK),
			uint32(protocol.OpcodeCMSG_SKILL_BUY_STEP),
			uint32(protocol.OpcodeCMSG_START_BATTLEFIELD_CHEAT),
			uint32(protocol.OpcodeCMSG_START_QUEST),
			uint32(protocol.OpcodeCMSG_STOP_DANCE),
			uint32(protocol.OpcodeCMSG_STORE_LOOT_IN_SLOT),
			uint32(protocol.OpcodeCMSG_SUSPEND_COMMS_ACK),
			uint32(protocol.OpcodeCMSG_SYNC_DANCE),
			uint32(protocol.OpcodeCMSG_TARGET_CAST),
			uint32(protocol.OpcodeCMSG_TARGET_SCRIPT_CAST),
			uint32(protocol.OpcodeCMSG_TAXICLEARALLNODES),
			uint32(protocol.OpcodeCMSG_TAXICLEARNODE),
			uint32(protocol.OpcodeCMSG_TAXIENABLEALLNODES),
			uint32(protocol.OpcodeCMSG_TAXIENABLENODE),
			uint32(protocol.OpcodeCMSG_TAXISHOWNODES),
			uint32(protocol.OpcodeCMSG_TEST_DROP_RATE),
			uint32(protocol.OpcodeCMSG_TOGGLE_XP_GAIN),
			uint32(protocol.OpcodeCMSG_TRIGGER_CINEMATIC_CHEAT),
			uint32(protocol.OpcodeCMSG_UNCLAIM_LICENSE),
			uint32(protocol.OpcodeCMSG_UNDRESSPLAYER),
			uint32(protocol.OpcodeCMSG_UNITANIMTIER_CHEAT),
			uint32(protocol.OpcodeCMSG_UNLEARN_DANCE_MOVE),
			uint32(protocol.OpcodeCMSG_UNLEARN_SPELL),
			uint32(protocol.OpcodeCMSG_UNLEARN_TALENTS),
			uint32(protocol.OpcodeCMSG_UNUSED6),
			uint32(protocol.OpcodeCMSG_USE_SKILL_CHEAT),
			uint32(protocol.OpcodeCMSG_VOICE_SET_TALKER_MUTED_REQUEST),
			uint32(protocol.OpcodeCMSG_WEATHER_SPEED_CHEAT),
			uint32(protocol.OpcodeCMSG_XP_CHEAT),
			uint32(protocol.OpcodeCMSG_ZONE_MAP),
			uint32(protocol.OpcodeMSG_CHANNEL_START),
			uint32(protocol.OpcodeMSG_CHANNEL_UPDATE),
			uint32(protocol.OpcodeMSG_DELAY_GHOST_TELEPORT),
			uint32(protocol.OpcodeMSG_DEV_SHOWLABEL),
			uint32(protocol.OpcodeMSG_GM_ACCOUNT_ONLINE),
			uint32(protocol.OpcodeMSG_GM_BIND_OTHER),
			uint32(protocol.OpcodeMSG_GM_CHANGE_ARENA_RATING),
			uint32(protocol.OpcodeMSG_GM_DESTROY_CORPSE),
			uint32(protocol.OpcodeMSG_GM_GEARRATING),
			uint32(protocol.OpcodeMSG_GM_RESETINSTANCELIMIT),
			uint32(protocol.OpcodeMSG_GM_SHOWLABEL),
			uint32(protocol.OpcodeMSG_GM_SUMMON),
			uint32(protocol.OpcodeMSG_MOVE_FEATHER_FALL),
			uint32(protocol.OpcodeMSG_MOVE_KNOCK_BACK),
			uint32(protocol.OpcodeMSG_MOVE_ROOT),
			uint32(protocol.OpcodeMSG_MOVE_UNROOT),
			uint32(protocol.OpcodeMSG_MOVE_SET_ALL_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_COLLISION_HGT),
			uint32(protocol.OpcodeMSG_MOVE_SET_FLIGHT_BACK_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_FLIGHT_BACK_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_FLIGHT_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_PITCH_RATE),
			uint32(protocol.OpcodeMSG_MOVE_SET_PITCH_RATE_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_RUN_BACK_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_RUN_BACK_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_RUN_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_RUN_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_SWIM_BACK_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_SWIM_BACK_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_SWIM_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_SWIM_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_TURN_RATE),
			uint32(protocol.OpcodeMSG_MOVE_SET_TURN_RATE_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_SET_WALK_SPEED),
			uint32(protocol.OpcodeMSG_MOVE_SET_WALK_SPEED_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_START_SWIM_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_STOP_SWIM_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_TELEPORT),
			uint32(protocol.OpcodeMSG_MOVE_TELEPORT_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_TIME_SKIPPED),
			uint32(protocol.OpcodeMSG_MOVE_TOGGLE_COLLISION_CHEAT),
			uint32(protocol.OpcodeMSG_MOVE_TOGGLE_FALL_LOGGING),
			uint32(protocol.OpcodeMSG_MOVE_TOGGLE_LOGGING),
			uint32(protocol.OpcodeMSG_MOVE_UPDATE_CAN_FLY),
			uint32(protocol.OpcodeMSG_MOVE_UPDATE_CAN_TRANSITION_BETWEEN_SWIM_AND_FLY),
			uint32(protocol.OpcodeMSG_MOVE_WATER_WALK),
			uint32(protocol.OpcodeMSG_NOTIFY_PARTY_SQUELCH),
			uint32(protocol.OpcodeMSG_RAID_READY_CHECK_CONFIRM),
			uint32(protocol.OpcodeMSG_VIEW_PHASE_SHIFT),
			uint32(protocol.OpcodeUMSG_DELETE_GUILD_CHARTER),
			uint32(protocol.OpcodeUMSG_UPDATE_GROUP_INFO),
			uint32(protocol.OpcodeUMSG_UPDATE_GROUP_MEMBERS),
			uint32(protocol.OpcodeUMSG_UPDATE_GUILD):
			// Intentionally discarded per TrinityCore Handle_NULL
			state.debug("handle_null client opcode ignored", "account", state.accountName, "opcode", opcodeName(header.Opcode))

		default:
			// WorldSocket::ReadDataHandler (WorldSocket.cpp:378): a client opcode
			// with no registered handler (or out of range) closes the connection.
			// Opcodes the reference explicitly null-handles are listed above.
			state.debug("world unknown opcode, closing connection", "account", state.accountName, "opcode", opcodeName(header.Opcode), "size", len(payload))
			return
		}
	}
}

func opcodeName(opcode uint32) string {
	if name, ok := protocol.OpcodeNames[protocol.Opcode(opcode)]; ok {
		return name
	}
	return fmt.Sprintf("0x%03X", opcode)
}

func (s *session) handleReadyForAccountDataTimes() bool {
	if err := s.write(uint16(protocol.OpcodeSMSG_ACCOUNT_DATA_TIMES), buildAccountDataTimesWithTimestamps(time.Now(), globalAccountDataMask, s.loadAccountDataTimes(context.Background(), 0, globalAccountDataMask)), true); err != nil {
		s.debug("global account data times failed", "account", s.accountName, "error", err)
		return false
	}
	return true
}

func (s *session) handleRealmSplit(payload []byte) bool {
	response, err := buildRealmSplit(payload)
	if err != nil {
		s.debug("realm split rejected", "account", s.accountName, "error", err)
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_REALM_SPLIT), response, true); err != nil {
		s.debug("realm split response failed", "account", s.accountName, "error", err)
		return false
	}
	return true
}

func (s *session) handleAuthSession(ctx context.Context, payload []byte) bool {
	b := protocol.NewReader(payload)
	build, err := b.ReadU32()
	if err != nil {
		return false
	}
	if _, err = b.ReadU32(); err != nil {
		return false
	}
	accountName, err := b.ReadCString()
	if err != nil {
		return false
	}
	debugAccount := accountName
	if _, err = b.ReadU32(); err != nil {
		return false
	}
	localChallenge, err := b.Read(4)
	if err != nil {
		return false
	}
	if _, err = b.ReadU32(); err != nil {
		return false
	}
	if _, err = b.ReadU32(); err != nil {
		return false
	}
	realmID, err := b.ReadU32()
	if err != nil {
		return false
	}
	if _, err = b.ReadU64(); err != nil {
		return false
	}
	digestBytes, err := b.Read(sha1.Size)
	if err != nil {
		return false
	}
	// WorldSocket::HandleAuthSession (WorldSocket.cpp:441): ByteBuffer::contents()
	// throws on an empty addon-info block — the reference drops the connection.
	if len(b.Bytes())-b.Position() == 0 {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "empty addon info")
		return false
	}
	account, err := loadAccount(ctx, s.server.AuthStore, accountName, s.server.RealmID)
	if err != nil || account == nil {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "unknown account")
		_ = s.write(opcodeAuthResponse, []byte{authUnknownAccount}, false)
		return false
	}
	// WorldSocket.cpp:267 (AccountInfo ctor): an account expansion above the
	// world's configured expansion is clamped down to it.
	if worldExpansion := s.server.Config.Expansion; worldExpansion >= 0 && account.Expansion > uint8(worldExpansion) {
		account.Expansion = uint8(worldExpansion)
	}
	_, _ = s.server.AuthStore.ExecStatement(ctx, "LOGIN_UPD_LAST_ATTEMPT_IP", remoteAddress(s.conn), accountName)
	if len(account.SessionKey) != crypto.SRP6SessionKeyLength {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "invalid session key length")
		_ = s.write(opcodeAuthResponse, []byte{authFailed}, false)
		return false
	}
	s.crypt, err = crypto.NewAuthCrypt(account.SessionKey)
	if err != nil {
		return false
	}
	if s.server.closed.Load() {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "world closed")
		_ = s.write(opcodeAuthResponse, []byte{authReject}, true)
		return false
	}
	if realmID != s.server.RealmID {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "realm mismatch", "realm", realmID)
		_ = s.write(opcodeAuthResponse, []byte{realmListRealmNotFound}, true)
		return false
	}
	if s.server.Config.WardenEnabled && !wardenOSAllowed(account.OS) {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "invalid client OS", "os", account.OS)
		_ = s.write(opcodeAuthResponse, []byte{authReject}, true)
		return false
	}
	h := sha1.New()
	_, _ = h.Write([]byte(accountName))
	_, _ = h.Write(make([]byte, 4))
	_, _ = h.Write(localChallenge)
	_, _ = h.Write(s.authSeed[:])
	_, _ = h.Write(account.SessionKey)
	if subtle.ConstantTimeCompare(h.Sum(nil), digestBytes) != 1 {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "invalid session digest")
		_ = s.write(opcodeAuthResponse, []byte{authFailed}, true)
		return false
	}
	if account.Locked && account.LastIP != remoteAddress(s.conn) {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "ip lock")
		_ = s.write(opcodeAuthResponse, []byte{authFailed}, true)
		return false
	}
	country := s.server.ipLocations.Country(remoteAddress(s.conn))
	if iplocation.CountryLockMismatch(account.Locked, account.LockCountry, country) {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "country lock", "country", country)
		_ = s.write(opcodeAuthResponse, []byte{authFailed}, true)
		return false
	}
	// WorldSocket.cpp:555-562: the negative-mutetime arm runs before the ban
	// and security checks, so a banned account still gets its pending mute
	// persisted via LOGIN_UPD_MUTE_TIME_LOGIN.
	account.MuteTime = normalizeLoginMuteTime(ctx, s.server.AuthStore.DB, account.ID, account.MuteTime)
	if banned, err := accountBanned(ctx, s.server.AuthStore, account.ID); err != nil || banned {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "account ban")
		_ = s.write(opcodeAuthResponse, []byte{authBanned}, true)
		return false
	}
	if s.server.allowedSecurityLevel > 0 && account.Security < s.server.allowedSecurityLevel {
		s.debug("world authentication rejected", "account", debugAccount, "reason", "security level below realm limit", "security", account.Security, "required", s.server.allowedSecurityLevel)
		_ = s.write(opcodeAuthResponse, []byte{authUnavailable}, true)
		return false
	}
	if !s.server.kickDuplicateAccountSessions(account.ID, s) {
		return false
	}
	if _, err := s.server.AuthStore.DB.ExecContext(ctx, "UPDATE account SET last_ip = ? WHERE id = ?", remoteAddress(s.conn), account.ID); err != nil {
		return false
	}
	s.authed = true
	s.accountID = account.ID
	s.accountName = accountName
	// The C++ auth handshake SELECTs account.recruiter into the session
	// (WorldSocket.cpp:267 → WorldSession.cpp:135). Error-tolerant: a
	// missing column or failed query leaves the zero (no-recruiter) state,
	// which the RAF check treats as no link, like C++'s default 0.
	s.recruiterID = accountRecruiterID(ctx, s.server.AuthStore.DB, account.ID)
	s.security = account.Security
	s.muteTime = account.MuteTime
	s.gmChat = false
	s.gmMessage = false
	if s.gmMessage, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionCommandGMChat); err != nil {
		s.gmMessage = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionCommandGMChat, "error", err)
	}
	if s.twoSideChat, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionTwoSideInteractionChat); err != nil {
		s.twoSideChat = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionTwoSideInteractionChat, "error", err)
	}
	if s.twoSideWhoList, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionTwoSideWhoList); err != nil {
		s.twoSideWhoList = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionTwoSideWhoList, "error", err)
	}
	if s.whoSeeAllSecurityLevels, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionWhoSeeAllSecurityLevels); err != nil {
		s.whoSeeAllSecurityLevels = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionWhoSeeAllSecurityLevels, "error", err)
	}
	if s.twoSideAddFriend, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionTwoSideAddFriend); err != nil {
		s.twoSideAddFriend = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionTwoSideAddFriend, "error", err)
	}
	if s.allowGMFriend, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionAllowGMFriend); err != nil {
		s.allowGMFriend = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionAllowGMFriend, "error", err)
	}
	s.accountExpansion = account.Expansion
	s.debug("world authentication accepted", "account", accountName, "build", build, "expansion", s.accountExpansion, "gm_chat", s.gmChat, "two_side_chat", s.twoSideChat, "remote", remoteAddress(s.conn))
	s.resetTimeoutTime(false)
	if s.ignoreIdleTimeout, err = accountHasPermission(ctx, s.server.AuthStore.DB, account.ID, s.server.RealmID, account.Security, permissionIgnoreIdleConnection); err != nil {
		s.ignoreIdleTimeout = false
		s.debug("RBAC permission lookup failed", "account", accountName, "permission", permissionIgnoreIdleConnection, "error", err)
	}
	s.loadTutorials(ctx)
	s.pendingAddonInfo = append([]byte(nil), b.Bytes()[b.Position():]...)

	// WorldSocket::HandleAuthSessionCallback (WorldSocket.cpp:600): the warden
	// session initializes before the queue-admission decision, so queued
	// players receive the module handshake while waiting.
	if s.server.Config.WardenEnabled && len(account.SessionKey) == crypto.SRP6SessionKeyLength {
		w, err := newWardenSession(s, account.SessionKey)
		if err != nil {
			s.server.Logger.Error("failed to initialize warden for session", "account", s.accountName, "error", err)
		} else {
			s.warden = w
		}
	}

	if s.server.checkQueue(s, ctx, account) {
		return true
	}
	return s.initializeSession(ctx)
}

func (s *session) initializeSession(ctx context.Context) bool {
	// WorldSession::InitializeSessionCallback (WorldSession.cpp:1325): a session
	// promoted out of the queue answers with the 1-byte SendAuthWaitQue(0)
	// form; a fresh login gets the 11-byte short form. inQueue is cleared only
	// after the response, matching SetInQueue(false) after the send.
	wasQueued := s.inQueue
	if wasQueued {
		if err := s.write(opcodeAuthResponse, []byte{authOK}, true); err != nil {
			return false
		}
	} else {
		authBuf := protocol.NewBuffer(11)
		authBuf.WriteU8(authOK)
		authBuf.WriteU32(0)
		authBuf.WriteU8(0)
		authBuf.WriteU32(0)
		authBuf.WriteU8(s.accountExpansion)
		if err := s.write(opcodeAuthResponse, authBuf.Bytes(), true); err != nil {
			return false
		}
	}
	s.inQueue = false
	if err := s.write(uint16(protocol.OpcodeSMSG_ADDON_INFO), buildAddonInfoResponse(s.pendingAddonInfo), true); err != nil {
		return false
	}
	s.pendingAddonInfo = nil
	cacheVersion := protocol.NewBuffer(4)
	cacheVersion.WriteU32(s.server.clientCacheVersion)
	if err := s.write(uint16(protocol.OpcodeSMSG_CLIENTCACHE_VERSION), cacheVersion.Bytes(), true); err != nil {
		return false
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_TUTORIAL_FLAGS), buildTutorialFlags(s.tutorials), true); err != nil {
		return false
	}
	return true
}

func (s *Server) checkQueue(sess *session, ctx context.Context, account *account) bool {
	if s.playerLimit == 0 {
		return false
	}
	// World::AddSession_ (World.cpp:350): Sessions = m_sessions.size() counts
	// active AND queued sessions, then --Sessions so the newcomer never counts
	// itself against the limit.
	s.sessionsMu.RLock()
	total := 0
	for current := range s.sessions {
		if current == sess || !current.authed {
			continue
		}
		total++
	}
	s.sessionsMu.RUnlock()
	if uint32(total) < s.playerLimit {
		return false
	}
	skipQueue, err := accountHasPermission(ctx, s.AuthStore.DB, account.ID, s.RealmID, account.Security, permissionSkipQueue)
	if err != nil {
		sess.debug("queue permission lookup failed", "account", account.ID, "error", err)
	} else if skipQueue {
		return false
	}
	// World::HasRecentlyDisconnected (World.cpp:382): an account that dropped
	// within DisconnectToleranceInterval skips the queue on re-login.
	if s.recentlyDisconnected(account.ID) {
		sess.debug("queue skipped: recent disconnect", "account", account.ID)
		return false
	}
	s.sessionsMu.Lock()
	sess.inQueue = true
	s.queuedSessions = append(s.queuedSessions, sess)
	pos := uint32(len(s.queuedSessions))
	s.sessionsMu.Unlock()
	sess.debug("player queued", "account", account.ID, "position", pos)
	// World::AddQueuedPlayer (World.cpp:415): the FIRST queue response is the
	// full SendAuthResponse(AUTH_WAIT_QUEUE, shortForm=false) form — billing
	// fields + expansion + queue position + migration flag. The 6-byte form is
	// only used for later position updates (SendAuthWaitQue).
	buf := protocol.NewBuffer(16)
	buf.WriteU8(authWaitQueue)
	buf.WriteU32(0)
	buf.WriteU8(0)
	buf.WriteU32(0)
	buf.WriteU8(account.Expansion)
	buf.WriteU32(pos)
	buf.WriteU8(0)
	_ = sess.write(opcodeAuthResponse, buf.Bytes(), true)
	return true
}

// recentlyDisconnected mirrors World::HasRecentlyDisconnected (World.cpp:382):
// expired entries are pruned, then a live entry for the account skips the queue.
func (s *Server) recentlyDisconnected(accountID uint32) bool {
	tolerance := s.Config.DisconnectToleranceInterval
	if tolerance == 0 || accountID == 0 {
		return false
	}
	now := time.Now().Unix()
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for id, t := range s.disconnectTimes {
		if now-t >= int64(tolerance) {
			delete(s.disconnectTimes, id)
		}
	}
	_, ok := s.disconnectTimes[accountID]
	return ok
}

func (s *Server) promoteQueuedPlayers() {
	promotedAny := false
	for s.tryPromoteQueueFront() {
		promotedAny = true
	}
	if promotedAny {
		s.updateQueuePositions()
	}
}

// tryPromoteQueueFront mirrors the promotion arm of World::RemoveQueuedPlayer
// (World.cpp:424): while a slot is free the queue head is initialized. A failed
// account load re-queues the session at the front instead of dropping it.
func (s *Server) tryPromoteQueueFront() bool {
	s.sessionsMu.Lock()
	if len(s.queuedSessions) == 0 {
		s.sessionsMu.Unlock()
		return false
	}
	activeCount := 0
	for current := range s.sessions {
		if current.authed && !current.inQueue {
			activeCount++
		}
	}
	if s.playerLimit > 0 && uint32(activeCount) >= s.playerLimit {
		s.sessionsMu.Unlock()
		return false
	}
	promoted := s.queuedSessions[0]
	s.queuedSessions = s.queuedSessions[1:]
	s.sessionsMu.Unlock()
	promoted.debug("player promoted from queue", "account", promoted.accountID)
	account, err := loadAccount(context.Background(), s.AuthStore, promoted.accountName, s.RealmID)
	if err != nil || account == nil {
		promoted.debug("queue promotion failed: account not found", "account", promoted.accountName)
		s.sessionsMu.Lock()
		s.queuedSessions = append([]*session{promoted}, s.queuedSessions...)
		s.sessionsMu.Unlock()
		return false
	}
	promoted.initializeSession(context.Background())
	return true
}

// removeQueuedSessionLocked splices a session out of the login queue.
// Callers must hold sessionsMu.
func (s *Server) removeQueuedSessionLocked(sess *session) bool {
	for i, queued := range s.queuedSessions {
		if queued == sess {
			s.queuedSessions = append(s.queuedSessions[:i], s.queuedSessions[i+1:]...)
			return true
		}
	}
	return false
}

// updateQueuePositions mirrors the tail of World::RemoveQueuedPlayer
// (World.cpp:424): every remaining queued session gets its new 1-based
// position via the 6-byte SendAuthWaitQue form.
func (s *Server) updateQueuePositions() {
	s.sessionsMu.RLock()
	queued := append([]*session(nil), s.queuedSessions...)
	s.sessionsMu.RUnlock()
	for i, qs := range queued {
		buf := protocol.NewBuffer(6)
		buf.WriteU8(authWaitQueue)
		buf.WriteU32(uint32(i + 1))
		buf.WriteU8(0)
		_ = qs.write(opcodeAuthResponse, buf.Bytes(), true)
	}
}

func wardenOSAllowed(osName string) bool { return osName == "Win" || osName == "OSX" }

func normalizeLoginMuteTime(ctx context.Context, db *sql.DB, accountID uint32, muteTime int64) int64 {
	if muteTime >= 0 {
		return muteTime
	}
	absolute := time.Now().Unix() - muteTime
	if db != nil {
		_, _ = db.ExecContext(ctx, "UPDATE account SET mutetime = ? WHERE id = ?", absolute, accountID)
	}
	return absolute
}

// kickDuplicateAccountSessions mirrors the old-session arms of World::AddSession_
// (World.cpp:320): the previous session for the account is kicked so the new
// login can take its place. It returns false when the new session must be kicked
// instead — World::RemoveSession refuses to replace a session whose player is
// still loading.
func (s *Server) kickDuplicateAccountSessions(accountID uint32, current *session) bool {
	if s == nil || accountID == 0 {
		return true
	}
	s.sessionsMu.RLock()
	duplicates := make([]*session, 0)
	for sess := range s.sessions {
		if sess != current && sess.authed && sess.accountID == accountID {
			if sess.playerLoading {
				s.sessionsMu.RUnlock()
				current.debug("new login rejected: previous session still loading player", "account", current.accountName)
				if current.conn != nil {
					_ = current.conn.Close()
				}
				return false
			}
			duplicates = append(duplicates, sess)
		}
	}
	s.sessionsMu.RUnlock()
	if len(duplicates) > 0 {
		s.sessionsMu.Lock()
		queuedRemoved := false
		for _, sess := range duplicates {
			sess.superseded = true
			delete(s.sessions, sess)
			// World::AddSession_ removes a queued old session from the login
			// queue synchronously via RemoveQueuedPlayer, not on socket close.
			if s.removeQueuedSessionLocked(sess) {
				queuedRemoved = true
			}
		}
		s.sessionsMu.Unlock()
		if queuedRemoved {
			s.updateQueuePositions()
		}
	}
	for _, sess := range duplicates {
		sess.debug("session replaced by new login", "account", sess.accountName)
		if sess.conn != nil {
			_ = sess.conn.Close()
		}
	}
	return true
}

func (s *session) loadTutorials(ctx context.Context) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	row := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT tut0, tut1, tut2, tut3, tut4, tut5, tut6, tut7 FROM account_tutorial WHERE accountId = ?", s.accountID)
	var tut0, tut1, tut2, tut3, tut4, tut5, tut6, tut7 uint32
	if err := row.Scan(&tut0, &tut1, &tut2, &tut3, &tut4, &tut5, &tut6, &tut7); err == nil {
		s.tutorials = [8]uint32{tut0, tut1, tut2, tut3, tut4, tut5, tut6, tut7}
		s.tutorialsInDB = true
	}
}

func (s *session) saveTutorials(ctx context.Context) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	if s.tutorialsInDB {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE account_tutorial SET tut0 = ?, tut1 = ?, tut2 = ?, tut3 = ?, tut4 = ?, tut5 = ?, tut6 = ?, tut7 = ? WHERE accountId = ?",
			s.tutorials[0], s.tutorials[1], s.tutorials[2], s.tutorials[3], s.tutorials[4], s.tutorials[5], s.tutorials[6], s.tutorials[7], s.accountID)
	} else {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "INSERT INTO account_tutorial(tut0, tut1, tut2, tut3, tut4, tut5, tut6, tut7, accountId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			s.tutorials[0], s.tutorials[1], s.tutorials[2], s.tutorials[3], s.tutorials[4], s.tutorials[5], s.tutorials[6], s.tutorials[7], s.accountID)
		s.tutorialsInDB = true
	}
}

func (s *session) handleTutorialFlag(ctx context.Context, payload []byte) bool {
	b := protocol.NewReader(payload)
	data, err := b.ReadU32()
	if err != nil {
		return false
	}
	index := data / 32
	if index < 8 {
		s.tutorials[index] |= 1 << (data % 32)
		s.saveTutorials(ctx)
	}
	return true
}

func (s *session) handleTutorialClear(ctx context.Context) bool {
	for i := range s.tutorials {
		s.tutorials[i] = 0xFFFFFFFF
	}
	s.saveTutorials(ctx)
	return true
}

func (s *session) handleTutorialReset(ctx context.Context) bool {
	for i := range s.tutorials {
		s.tutorials[i] = 0
	}
	s.saveTutorials(ctx)
	return true
}

func (s *session) handlePing(ctx context.Context, payload []byte) bool {
	b := protocol.NewReader(payload)
	ping, err := b.ReadU32()
	if err != nil {
		return false
	}
	latency, err := b.ReadU32()
	if err != nil {
		return false
	}
	// Reference: WorldSocket::HandlePing — over-speed ping protection with a 27 second window,
	// latency tracking on the session, and a SMSG_PONG echo of the ping counter.
	now := time.Now()
	if s.lastPing.IsZero() {
		s.lastPing = now
	} else {
		diff := now.Sub(s.lastPing)
		s.lastPing = now
		if diff < overspeedPingWindow {
			s.overSpeedPings++
			if s.server != nil && s.server.Config.MaxOverSpeedPings != 0 && s.overSpeedPings > s.server.Config.MaxOverSpeedPings {
				if s.server.AuthStore == nil {
					return false
				}
				skip, permErr := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionSkipCheckOverSpeedPing)
				if permErr != nil {
					// WorldSocket::HandlePing (WorldSocket.cpp:655): the reference
					// consults in-memory RBAC, which has no failure mode — a lookup
					// failure must not kick the way a missing permission does.
					s.debug("over-speed ping permission lookup failed", "account", s.accountName, "error", permErr)
				} else if !skip {
					s.debug("session kicked for over-speed pings", "account", s.accountName)
					return false
				}
			}
		} else {
			s.overSpeedPings = 0
		}
	}
	s.latency.Store(latency)
	response := protocol.NewBuffer(4)
	response.WriteU32(ping)
	return s.write(opcodePong, response.Bytes(), true) == nil
}

// handleKeepAlive mirrors WorldSocket::ReadDataHandler case CMSG_KEEP_ALIVE (WorldSocket.cpp:348).
// An empty client heartbeat packet; it only refreshes the in-world activity timeout
// (ResetTimeOutTime(true)), so keep-alives never extend the character-select idle timer.
func (s *session) handleKeepAlive() bool {
	s.resetTimeoutTime(true)
	return true
}

// resetTimeoutTime mirrors WorldSession::ResetTimeOutTime (WorldSession.cpp:733):
// with a live player the in-world timeout (SocketTimeOutTimeActive) applies; otherwise
// the character-select timeout (SocketTimeOutTime) applies unless onlyActive is set.
func (s *session) resetTimeoutTime(onlyActive bool) {
	var timeoutMs int64
	switch {
	case s.playerLoaded:
		timeoutMs = int64(s.server.Config.SocketTimeOutTimeActive)
	case !onlyActive:
		timeoutMs = int64(s.server.Config.SocketTimeOutTime)
	default:
		return
	}
	s.timeoutTime.Store(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond).Unix())
}

func (s *session) decrypt(data []byte) error {
	if s.crypt == nil {
		return nil
	}
	return s.crypt.DecryptRecv(data)
}

func (s *session) write(opcode uint16, payload []byte, encrypt bool) error {
	_, err := s.writeWithStatus(opcode, payload, encrypt)
	return err
}

func (s *session) writeWithStatus(opcode uint16, payload []byte, encrypt bool) (bool, error) {
	if s != nil && s.authed && s.server != nil && s.server.Features != nil && s.server.Features.Scripts != nil {
		packet := &scripting.Packet{Opcode: uint32(opcode), Data: append([]byte(nil), payload...)}
		values, hookErr := s.server.Features.Scripts.TriggerPacketEvent(context.Background(), int(opcode), 7, packet, s.luaPlayer())
		if hookErr != nil {
			s.debug("lua packet send hook failed", "account", s.accountName, "opcode", opcode, "error", hookErr)
		}
		for _, value := range values {
			if allowed, ok := value.(bool); ok && !allowed {
				return false, nil
			}
		}
		opcode = uint16(packet.Opcode)
		payload = packet.Data
	}
	if s != nil && s.captureUpdatePackets && (opcode == uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) || opcode == uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT)) {
		s.capturedUpdatePackets = append(s.capturedUpdatePackets, protocol.PacketFrom(opcode, append([]byte(nil), payload...)))
		return true, nil
	}
	if s != nil && s.server != nil && s.server.TraceRecorder != nil {
		state := opcodeName(uint32(opcode))
		if s.traceStatePrefix != "" {
			state = s.traceStatePrefix + " " + state
		}
		s.server.TraceRecorder.Record(protocoltrace.ServerToClient, uint32(opcode), payload, state)
	}
	if s == nil || s.conn == nil {
		return false, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	frame, headerSize, err := protocol.EncodeServerFrame(opcode, payload)
	if err != nil {
		return false, err
	}
	if encrypt && s.crypt != nil {
		if err := s.crypt.EncryptSend(frame[:headerSize]); err != nil {
			return false, err
		}
	}
	for len(frame) > 0 {
		n, err := s.conn.Write(frame)
		if err != nil {
			return false, err
		}
		frame = frame[n:]
	}
	if opcode != uint16(protocol.OpcodeSMSG_UPDATE_OBJECT) && opcode != uint16(protocol.OpcodeSMSG_COMPRESSED_UPDATE_OBJECT) {
		s.debug("world packet sent", "account", s.accountName, "opcode", opcodeName(uint32(opcode)), "size", len(payload))
	}
	return true, nil
}

func loadAccount(ctx context.Context, store *database.Store, username string, realmID uint32) (*account, error) {
	var result account
	var locked int64
	var expansion sql.NullInt64
	query := "SELECT id, session_key_auth, last_ip, locked, lock_country, os, mutetime, expansion FROM account WHERE username = ? AND session_key_auth IS NOT NULL LIMIT 1"
	if store.Backend == database.BackendSQLite {
		query = "SELECT id, session_key_auth, last_ip, locked, lock_country, os, mutetime, expansion FROM account WHERE UPPER(username) = UPPER(?) AND session_key_auth IS NOT NULL LIMIT 1"
	}
	var muteTime sql.NullInt64
	err := store.DB.QueryRowContext(ctx, query, username).Scan(&result.ID, &result.SessionKey, &result.LastIP, &locked, &result.LockCountry, &result.OS, &muteTime, &expansion)
	if err != nil {
		fallbackQuery := "SELECT id, session_key_auth, last_ip, locked, lock_country, os FROM account WHERE username = ? AND session_key_auth IS NOT NULL LIMIT 1"
		if store.Backend == database.BackendSQLite {
			fallbackQuery = "SELECT id, session_key_auth, last_ip, locked, lock_country, os FROM account WHERE UPPER(username) = UPPER(?) AND session_key_auth IS NOT NULL LIMIT 1"
		}
		err = store.DB.QueryRowContext(ctx, fallbackQuery, username).Scan(&result.ID, &result.SessionKey, &result.LastIP, &locked, &result.LockCountry, &result.OS)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		muteTime = sql.NullInt64{}
	}
	if muteTime.Valid {
		result.MuteTime = muteTime.Int64
	}
	result.Locked = locked != 0
	// WorldSocket.cpp:283 (AccountInfo ctor): the expansion column is taken raw —
	// a stored 0 (classic) stays 0; only values above the world's expansion clamp down.
	if expansion.Valid {
		result.Expansion = uint8(expansion.Int64)
	}
	// WorldSocket.cpp:267 (AccountInfo ctor): the auth query's ORDER BY
	// aa.RealmID DESC LIMIT 1 picks the realm-specific access row over the
	// global (-1) row — MAX would wrongly promote a global GM on a realm
	// where they were explicitly demoted (or vice versa).
	security, err := accountSecurityLevel(ctx, store.DB, result.ID, realmID)
	if err != nil {
		return nil, err
	}
	result.Security = security
	return &result, nil
}

// accountSecurityLevel mirrors the account_access arm of
// LOGIN_SEL_ACCOUNT_INFO_BY_NAME (LoginDatabase.cpp:49): the realm-specific
// row wins over the global RealmID=-1 row; a missing row or table yields
// SEC_PLAYER (0), matching C++ GetUInt8 on the NULL LEFT JOIN field.
func accountSecurityLevel(ctx context.Context, db *sql.DB, accountID uint32, realmID uint32) (uint8, error) {
	if db == nil {
		return 0, nil
	}
	for _, rid := range []any{realmID, -1} {
		var level sql.NullInt64
		err := db.QueryRowContext(ctx, "SELECT SecurityLevel FROM account_access WHERE AccountID = ? AND RealmID = ?", accountID, rid).Scan(&level)
		if err == nil {
			if level.Valid {
				return uint8(level.Int64), nil
			}
			return 0, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "no such table") || strings.Contains(lower, "doesn't exist") || strings.Contains(lower, "unknown table") {
			return 0, nil
		}
		return 0, err
	}
	return 0, nil
}

// ipBanned mirrors WorldSocket::CheckIpCallback (WorldSocket.cpp:56): any
// ip_banned row for this exact IP whose ban is still live (unbandate > now,
// or permanent unbandate = bandate) rejects the socket. A failed lookup
// fails open like the C++ null-result arm.
func ipBanned(ctx context.Context, store *database.Store, ip string) bool {
	if store == nil || store.DB == nil || ip == "" {
		return false
	}
	var count int64
	if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM ip_banned WHERE ip = ? AND (unbandate > ? OR unbandate = bandate)", ip, timeNow()).Scan(&count); err != nil {
		return false
	}
	return count != 0
}

func accountBanned(ctx context.Context, store *database.Store, id uint32) (bool, error) {
	var count int64
	err := store.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM account_banned WHERE id = ? AND active = 1 AND (unbandate > ? OR unbandate = bandate)", id, timeNow()).Scan(&count)
	return count != 0, err
}

// accountRecruiterID reads the account.recruiter column the C++ auth
// handshake loads into every session (WorldSocket.cpp:267). Any error —
// including a schema without the column — yields 0 (no recruiter), which
// the recruit-a-friend summon gate reads as "no link".
func accountRecruiterID(ctx context.Context, db *sql.DB, accountID uint32) uint32 {
	var recruiter uint32
	if err := db.QueryRowContext(ctx, "SELECT recruiter FROM account WHERE id = ?", accountID).Scan(&recruiter); err != nil {
		return 0
	}
	return recruiter
}

func timeNow() int64 { return time.Now().Unix() }

func remoteAddress(conn net.Conn) string {
	address := conn.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return strings.TrimSpace(address)
}

func (s *Server) debug(message string, args ...any) {
	if s.Logger != nil {
		s.Logger.Debug(message, args...)
	}
}

func (s *session) debug(message string, args ...any) {
	if s != nil && s.server != nil {
		s.server.debug(message, args...)
	}
}

func (s *session) logout() {
	ctx := context.Background()
	if s.duelPartner != 0 {
		s.endDuel(false, 0, false)
	}
	if err := s.completeLogoutWithPacket(ctx, false); err != nil {
		s.debug("player disconnect cleanup failed", "account", s.accountName, "error", err)
	}
}

type Features struct {
	Config  config.Config
	LFG     *LFGManager
	NPCBots *NPCBotManager
	Scripts *scripting.Runtime
}

func NewFeatures(c config.Config, stores *database.Set, logger *slog.Logger) *Features {
	return &Features{Config: c, LFG: NewLFGManager(c.SoloLFGEnable), NPCBots: NewNPCBotManager(stores.Characters, stores.World, c.NPCBots), Scripts: scripting.NewRuntime(scripting.Config{Enabled: c.LuaEnabled, ScriptPath: c.LuaScriptPath, CoreName: version.Product, CoreVersion: version.String(), RealmID: c.RealmID, CoreExpansion: c.Expansion, AuthDatabase: stores.Auth.DB, CharacterDB: stores.Characters.DB, WorldDatabase: stores.World.DB, Logger: logger})}
}

func (f *Features) Initialize(ctx context.Context) error {
	if err := f.NPCBots.Initialize(ctx); err != nil {
		return err
	}
	return f.Scripts.Load(ctx)
}

func (f *Features) OnPlayerLogin() {
	f.LFG.OnLogin()
}

func (s *session) handleNameQuery(ctx context.Context, payload []byte) bool {
	guids, err := readObjectGUIDCandidates(payload)
	if err != nil {
		s.debug("name query rejected", "account", s.accountName, "error", err)
		return false
	}
	requestedGUID := guids[0]
	var name string
	var race, gender, class int64
	resolvedGUID := uint64(0)
	resolved := false
	for _, guid := range guids {
		lowGUID := guid & 0xFFFFFFFF
		candidates := []uint64{guid}
		if lowGUID != 0 && lowGUID != guid {
			candidates = append(candidates, lowGUID)
		}
		for _, candidate := range candidates {
			if s.player != nil && (s.player.GUID == candidate || s.player.GUID == lowGUID) {
				name, race, gender, class = s.player.Name, int64(s.player.Race), int64(s.player.Gender), int64(s.player.Class)
				resolved = true
			} else if online := s.server.findSessionByGUID(candidate); online != nil && online.player != nil {
				name, race, gender, class = online.player.Name, int64(online.player.Race), int64(online.player.Gender), int64(online.player.Class)
				resolved = true
			} else if cached, ok := s.characterNames[candidate]; ok {
				name, race, gender, class = cached.Name, int64(cached.Race), int64(cached.Gender), int64(cached.Class)
				resolved = true
			} else if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name, race, gender, class FROM characters WHERE guid = ? AND (deleteInfos_Name IS NULL OR deleteInfos_Name = '')", candidate).Scan(&name, &race, &gender, &class)
				if err != nil && isMissingColumn(err) {
					err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT name, race, gender, class FROM characters WHERE guid = ?", candidate).Scan(&name, &race, &gender, &class)
				}
				resolved = err == nil
				if !resolved && !errors.Is(err, sql.ErrNoRows) {
					s.debug("name query lookup failed", "account", s.accountName, "guid", guid, "candidate", candidate, "error", err)
				}
			}
			if resolved {
				resolvedGUID = guid
				break
			}
		}
		if resolved {
			break
		}
	}
	packet := protocol.NewBuffer(32)
	if resolvedGUID == 0 {
		resolvedGUID = requestedGUID
	}
	packet.WritePackedGUID(resolvedGUID)
	if !resolved {
		packet.WriteU8(1)
		return s.write(uint16(protocol.OpcodeSMSG_NAME_QUERY_RESPONSE), packet.Bytes(), true) == nil
	}
	packet.WriteU8(0)
	packet.WriteCString(name)
	packet.WriteU8(0)
	packet.WriteU8(uint8(race))
	packet.WriteU8(uint8(gender))
	packet.WriteU8(uint8(class))
	declined := [5]string{}
	declinedLoaded := false
	guid := resolvedGUID
	lowGUID := guid & 0xFFFFFFFF
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && ((s.player != nil && (s.player.GUID == guid || s.player.GUID == lowGUID)) || s.server.findSessionByGUID(guid) != nil || s.server.findSessionByGUID(lowGUID) != nil) {
		declinedLoaded = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT genitive, dative, accusative, instrumental, prepositional FROM character_declinedname WHERE guid = ?`, lowGUID).Scan(&declined[0], &declined[1], &declined[2], &declined[3], &declined[4]) == nil
	}
	if declinedLoaded {
		packet.WriteU8(1)
		for _, name := range declined {
			packet.WriteCString(name)
		}
	} else {
		packet.WriteU8(0)
	}
	return s.write(uint16(protocol.OpcodeSMSG_NAME_QUERY_RESPONSE), packet.Bytes(), true) == nil
}

func (s *session) handleQueryTime() bool {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	packet := protocol.NewBuffer(8)
	packet.WriteU32(uint32(now.Unix()))
	packet.WriteU32(uint32(next.Sub(now).Seconds()))
	return s.write(uint16(protocol.OpcodeSMSG_QUERY_TIME_RESPONSE), packet.Bytes(), true) == nil
}

func (s *session) handlePlayedTime(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	trigger, err := reader.ReadU8()
	if err != nil {
		return false
	}
	var total, level int64
	if s.playerLoaded && s.player != nil {
		s.updatePlayedTime(time.Now())
		total, level = int64(s.player.TotalPlayedTime), int64(s.player.LevelPlayedTime)
	} else {
		err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT totaltime, leveltime FROM characters WHERE guid = ? AND account = ?", s.playerGUID, s.accountID).Scan(&total, &level)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			s.debug("played time query failed", "account", s.accountName, "error", err)
			return false
		}
	}
	packet := protocol.NewBuffer(9)
	packet.WriteU32(uint32(total))
	packet.WriteU32(uint32(level))
	packet.WriteU8(trigger)
	return s.write(uint16(protocol.OpcodeSMSG_PLAYED_TIME), packet.Bytes(), true) == nil
}

func (s *session) handleZoneUpdate(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	zone, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if !s.playerLoaded || s.player == nil {
		return true
	}
	s.updateLocalChannels(zone)
	_, areaID := s.server.zoneAndAreaID(s.player.Map, s.player.X, s.player.Y, s.player.Z, zone)
	s.exploreArea(ctx, areaID)
	s.streamNearbyObjects(ctx)
	if _, err := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_ZONE", zone, s.playerGUID); err != nil {
		s.debug("zone update failed", "account", s.accountName, "zone", zone, "error", err)
		return false
	}
	return true
}

func (s *session) handleSetActionBarToggles(payload []byte) bool {
	reader := protocol.NewReader(payload)
	toggles, err := reader.ReadU8()
	if err != nil {
		return false
	}
	if s.player != nil {
		s.player.ActionBars = uint32(toggles)
		if packet, err := s.server.buildPlayerValuesUpdate(s.playerGUID, map[int]uint32{unitFieldPlayerFieldBytes: playerFieldBytesValue(*s.player)}); err == nil && packet != nil {
			_ = s.write(packet.Opcode, packet.Payload.Bytes(), true)
		}
	}
	return true
}

func (s *session) handleSetActionButton(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	button, err := reader.ReadU8()
	if err != nil {
		return false
	}
	data, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if button >= 144 || !s.playerLoaded || s.player == nil {
		return true
	}
	s.player.Actions[button] = data
	spec := int64(s.player.ActiveTalentGroup)
	if data == 0 {
		_, err = s.server.CharactersStore.ExecStatement(ctx, "CHAR_DEL_CHAR_ACTION_BY_BUTTON_SPEC", s.playerGUID, button, spec)
		if err != nil && s.server.CharactersStore.DB != nil {
			_, err = s.server.CharactersStore.DB.ExecContext(ctx, "DELETE FROM character_action WHERE guid = ? AND button = ? AND spec = ?", s.playerGUID, button, spec)
		}
	} else {
		var action, kind int64
		action = int64(data & 0x00FFFFFF)
		kind = int64(data >> 24)
		result, updateErr := s.server.CharactersStore.ExecStatement(ctx, "CHAR_UPD_CHAR_ACTION", action, kind, s.playerGUID, button, spec)
		err = updateErr
		if err == nil && result != nil {
			var affected int64
			affected, err = result.RowsAffected()
			if err == nil && affected == 0 {
				_, err = s.server.CharactersStore.ExecStatement(ctx, "CHAR_INS_CHAR_ACTION", s.playerGUID, spec, button, action, kind)
			}
		} else if s.server.CharactersStore.DB != nil {
			res, dErr := s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_action SET action = ?, type = ? WHERE guid = ? AND button = ? AND spec = ?", action, kind, s.playerGUID, button, spec)
			if dErr == nil {
				if aff, _ := res.RowsAffected(); aff == 0 {
					_, err = s.server.CharactersStore.DB.ExecContext(ctx, "INSERT INTO character_action (guid, spec, button, action, type) VALUES (?, ?, ?, ?, ?)", s.playerGUID, spec, button, action, kind)
				}
			} else {
				err = dErr
			}
		}
	}
	if err != nil {
		s.debug("action button update failed", "account", s.accountName, "button", button, "error", err)
		return false
	}
	return true
}

func (s *session) handleUpdateAccountData(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	typeID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	timestamp, err := reader.ReadU32()
	if err != nil {
		return false
	}
	decompressedSize, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if typeID >= 8 || decompressedSize > 0xFFFF {
		return true
	}
	compressed, err := reader.Read(reader.Remaining())
	if err != nil {
		return false
	}
	data, err := decompressAccountData(compressed, decompressedSize)
	if err != nil {
		s.debug("account data decompression failed", "account", s.accountName, "type", typeID, "error", err)
		return true
	}
	var result sql.Result
	if globalAccountDataMask&(1<<typeID) != 0 {
		result, err = s.server.CharactersStore.ExecStatement(ctx, "CHAR_REP_ACCOUNT_DATA", s.accountID, typeID, timestamp, data)
	} else if s.playerLoaded {
		result, err = s.server.CharactersStore.ExecStatement(ctx, "CHAR_REP_PLAYER_ACCOUNT_DATA", s.playerGUID, typeID, timestamp, data)
	}
	_ = result
	if err != nil {
		s.debug("account data update failed", "account", s.accountName, "type", typeID, "error", err)
		return false
	}
	response := protocol.NewBuffer(8)
	response.WriteU32(typeID)
	response.WriteU32(0)
	return s.write(uint16(protocol.OpcodeSMSG_UPDATE_ACCOUNT_DATA_COMPLETE), response.Bytes(), true) == nil
}

func (s *session) handleRequestAccountData(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	typeID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if typeID >= 8 {
		return true
	}
	var timestamp int64
	var data []byte
	if globalAccountDataMask&(1<<typeID) != 0 {
		err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT time, data FROM account_data WHERE accountId = ? AND type = ?", s.accountID, typeID).Scan(&timestamp, &data)
	} else if s.playerLoaded {
		err = s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT time, data FROM character_account_data WHERE guid = ? AND type = ?", s.playerGUID, typeID).Scan(&timestamp, &data)
	} else {
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		timestamp = 0
		data = nil
	} else if err != nil {
		s.debug("account data request failed", "account", s.accountName, "type", typeID, "error", err)
		return false
	}
	compressed, err := compressAccountData(data)
	if err != nil {
		return false
	}
	packet := protocol.NewBuffer(24 + len(compressed))
	if s.playerLoaded {
		packet.WriteU64(s.playerGUID)
	} else {
		packet.WriteU64(0)
	}
	packet.WriteU32(typeID)
	packet.WriteU32(uint32(timestamp))
	packet.WriteU32(uint32(len(data)))
	packet.Write(compressed)
	return s.write(uint16(protocol.OpcodeSMSG_UPDATE_ACCOUNT_DATA), packet.Bytes(), true) == nil
}

func (s *session) handleWorldStateUITimer() bool {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(uint32(time.Now().Unix()))
	return s.write(uint16(protocol.OpcodeSMSG_WORLD_STATE_UI_TIMER_UPDATE), packet.Bytes(), true) == nil
}

func (s *session) handleRequestRaidInfo(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(0)
		return s.write(uint16(protocol.OpcodeSMSG_RAID_INSTANCE_INFO), packet.Bytes(), true) == nil
	}

	type raidLock struct {
		mapID      uint32
		difficulty uint32
		instanceID uint64
		expired    uint8
		extended   uint8
		resetTime  uint32
	}
	var locks []raidLock

	now := time.Now().Unix()
	rows, err := cdb.QueryContext(ctx, `SELECT i.map, i.difficulty, i.id, COALESCE(ci.extendState, 0), i.resettime
		FROM character_instance ci
		JOIN instance i ON i.id = ci.instance
		WHERE ci.guid = ? AND ci.permanent = 1`, s.playerGUID)
	if err == nil {
		for rows.Next() {
			var mapID, diff, instID, extendState, resetTime int64
			if err := rows.Scan(&mapID, &diff, &instID, &extendState, &resetTime); err == nil {
				rem := int64(0)
				if resetTime > now {
					rem = resetTime - now
				}
				expired := uint8(0)
				if rem == 0 && extendState != 2 { // 2 = EXTEND_STATE_EXTENDED
					expired = 1
				}
				extended := uint8(0)
				if extendState == 2 {
					extended = 1
				}
				locks = append(locks, raidLock{
					mapID:      uint32(mapID),
					difficulty: uint32(diff),
					instanceID: uint64(instID),
					expired:    expired,
					extended:   extended,
					resetTime:  uint32(rem),
				})
			}
		}
		rows.Close()
	}

	packet := protocol.NewBuffer(4 + len(locks)*22)
	packet.WriteU32(uint32(len(locks)))
	for _, l := range locks {
		packet.WriteU32(l.mapID)
		packet.WriteU32(l.difficulty)
		packet.WriteU64(l.instanceID)
		packet.WriteU8(1 - l.expired) // 1 = not expired, 0 = expired (Player.cpp:19231)
		packet.WriteU8(l.extended)
		packet.WriteU32(l.resetTime)
	}
	return s.write(uint16(protocol.OpcodeSMSG_RAID_INSTANCE_INFO), packet.Bytes(), true) == nil
}

func decompressAccountData(compressed []byte, expected uint32) ([]byte, error) {
	if expected == 0 {
		return nil, nil
	}
	reader, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, int64(expected)+1))
	if err != nil {
		return nil, err
	}
	if uint32(len(data)) != expected {
		return nil, fmt.Errorf("decompressed size %d does not match %d", len(data), expected)
	}
	return data, nil
}

func compressAccountData(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

// handleWardenData processes CMSG_WARDEN_DATA (0x2E7).
// Reference: WorldSession::HandleWardenDataOpcode (WardenHandler.cpp:25).
func (s *session) handleWardenData(ctx context.Context, payload []byte) bool {
	if s.warden == nil {
		return true
	}
	return s.warden.handleWardenData(ctx, payload)
}
