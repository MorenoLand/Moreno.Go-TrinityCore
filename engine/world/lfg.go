package world

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	LFGRoleNone   uint8 = 0x00
	LFGRoleLeader uint8 = 0x01
	LFGRoleTank   uint8 = 0x02
	LFGRoleHealer uint8 = 0x04
	LFGRoleDamage uint8 = 0x08
	LFGRoleAny    uint8 = LFGRoleLeader | LFGRoleTank | LFGRoleHealer | LFGRoleDamage

	LFGStateNone            uint8 = 0
	LFGStateRoleCheck       uint8 = 1
	LFGStateQueued          uint8 = 2
	LFGStateProposal        uint8 = 3
	LFGStateBoot            uint8 = 4
	LFGStateDungeon         uint8 = 5
	LFGStateFinishedDungeon uint8 = 6
	LFGStateRaidBrowser     uint8 = 7

	LFGUpdateDefault            uint8 = 0
	LFGUpdateRoleCheckAborted   uint8 = 4
	LFGUpdateJoinQueue          uint8 = 5
	LFGUpdateRoleCheckFailed    uint8 = 6
	LFGUpdateRemovedFromQueue   uint8 = 7
	LFGUpdateProposalFailed     uint8 = 8
	LFGUpdateProposalDeclined   uint8 = 9
	LFGUpdateGroupFound         uint8 = 10
	LFGUpdateAddedToQueue       uint8 = 12
	LFGUpdateProposalBegin      uint8 = 13
	LFGUpdateStatus             uint8 = 14
	LFGUpdateGroupMemberOffline uint8 = 15

	LFGRoleCheckDefault      uint32 = 0
	LFGRoleCheckFinished     uint32 = 1
	LFGRoleCheckInitializing uint32 = 2
	LFGRoleCheckMissingRole  uint32 = 3
	LFGRoleCheckWrongRoles   uint32 = 4
	LFGRoleCheckAborted      uint32 = 5
	LFGRoleCheckNoRole       uint32 = 6

	LFGProposalInitiating uint8 = 0
	LFGProposalFailed     uint8 = 1
	LFGProposalSuccess    uint8 = 2

	LFGAnswerPending int8 = -1
	LFGAnswerDeny    int8 = 0
	LFGAnswerAgree   int8 = 1

	LFGLockStatusInsufficientExpansion  uint32 = 1
	LFGLockStatusTooLowLevel            uint32 = 2
	LFGLockStatusTooHighLevel           uint32 = 3
	LFGLockStatusTooLowGearScore        uint32 = 4
	LFGLockStatusTooHighGearScore       uint32 = 5
	LFGLockStatusRaidLocked             uint32 = 6
	LFGLockStatusAttunementTooLowLevel  uint32 = 1001
	LFGLockStatusAttunementTooHighLevel uint32 = 1002
	LFGLockStatusQuestNotCompleted      uint32 = 1022
	LFGLockStatusMissingItem            uint32 = 1025
	LFGLockStatusNotInSeason            uint32 = 1031
	LFGLockStatusMissingAchievement     uint32 = 1034

	LFGTeleportErrorOK              uint32 = 0
	LFGTeleportErrorPlayerDead      uint32 = 1
	LFGTeleportErrorFalling         uint32 = 2
	LFGTeleportErrorInVehicle       uint32 = 3
	LFGTeleportErrorFatigue         uint32 = 4
	LFGTeleportErrorInvalidLocation uint32 = 6
	LFGTeleportErrorCharming        uint32 = 8

	LFGDungeonTypeNone    uint32 = 0
	LFGDungeonTypeDungeon uint32 = 1
	LFGDungeonTypeRaid    uint32 = 2
	LFGDungeonTypeHeroic  uint32 = 5
	LFGDungeonTypeRandom  uint32 = 6

	LFGJoinOK             uint32 = 0
	LFGJoinNotMeetReqs    uint32 = 5
	LFGJoinInvalidDungeon uint32 = 11
)

type LFGQueueEntry struct {
	GUID     uint64
	Roles    uint8
	Dungeons []uint32
	Comment  string
	QueuedAt time.Time
	State    uint8
}

type LfgRoleCheck struct {
	GroupID   uint64
	Leader    uint64
	Dungeons  []uint32
	Roles     map[uint64]uint8 // playerGUID -> roles
	State     uint32
	CreatedAt time.Time
}

type LFGProposalPlayer struct {
	Role   uint8
	Group  uint64
	Accept int8 // -1 = pending, 0 = decline, 1 = agree
}

type LFGProposal struct {
	ID         uint32
	DungeonID  uint32
	State      uint8
	Group      uint64
	Leader     uint64
	CancelTime time.Time
	Encounters uint32
	Silent     bool
	Players    map[uint64]*LFGProposalPlayer
}

type LFGManager struct {
	mu              sync.RWMutex
	enabled         bool
	solo            bool
	queue           map[uint64]LFGQueueEntry
	roleChecks      map[uint64]*LfgRoleCheck
	proposals       map[uint32]*LFGProposal
	nextProposalID  uint32
	validateDungeon func(uint32) bool
}

func NewLFGManager(enabled bool) *LFGManager {
	return &LFGManager{
		enabled:    enabled,
		queue:      make(map[uint64]LFGQueueEntry),
		roleChecks: make(map[uint64]*LfgRoleCheck),
		proposals:  make(map[uint32]*LFGProposal),
	}
}

func (m *LFGManager) SetDungeonValidator(validate func(uint32) bool) {
	m.mu.Lock()
	m.validateDungeon = validate
	m.mu.Unlock()
}

func (m *LFGManager) OnLogin() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled && !m.solo {
		m.solo = true
	}
}

func (m *LFGManager) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

func (m *LFGManager) Solo() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.solo
}

func (m *LFGManager) ToggleSolo() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.solo = !m.solo
	return m.solo
}

func (m *LFGManager) RequiresFullGroup(players, groupSize int) bool {
	if players < 0 || groupSize < 0 {
		return true
	}
	return !m.Solo() && players != groupSize
}

func (m *LFGManager) Join(guid uint64, roles uint8, dungeons []uint32, comment string) (uint32, LFGQueueEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled || len(dungeons) == 0 {
		return LFGJoinNotMeetReqs, LFGQueueEntry{GUID: guid, Roles: roles, State: LFGStateNone}
	}
	roles &= LFGRoleTank | LFGRoleHealer | LFGRoleDamage
	if roles == 0 {
		return LFGJoinNotMeetReqs, LFGQueueEntry{GUID: guid, Roles: roles, State: LFGStateNone}
	}
	filtered := append([]uint32(nil), dungeons...)
	if m.validateDungeon != nil {
		valid := filtered[:0]
		for _, dungeon := range filtered {
			if m.validateDungeon(dungeon) {
				valid = append(valid, dungeon)
			}
		}
		filtered = valid
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i] < filtered[j] })
	filtered = uniqueUint32(filtered)
	if len(filtered) == 0 {
		return LFGJoinInvalidDungeon, LFGQueueEntry{GUID: guid, Roles: roles, State: LFGStateNone}
	}
	entry := LFGQueueEntry{GUID: guid, Roles: roles, Dungeons: filtered, Comment: comment, QueuedAt: time.Now(), State: LFGStateQueued}
	m.queue[guid] = entry
	return LFGJoinOK, cloneLFGQueueEntry(entry)
}

func (m *LFGManager) Leave(guid uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.queue[guid]; !ok {
		return false
	}
	delete(m.queue, guid)
	return true
}

func (m *LFGManager) Status(guid uint64) (LFGQueueEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.queue[guid]
	if !ok {
		return LFGQueueEntry{GUID: guid, State: LFGStateNone}, false
	}
	return cloneLFGQueueEntry(entry), true
}

func uniqueUint32(values []uint32) []uint32 {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func cloneLFGQueueEntry(entry LFGQueueEntry) LFGQueueEntry {
	entry.Dungeons = append([]uint32(nil), entry.Dungeons...)
	return entry
}

func (m *LFGManager) StartRoleCheck(groupID uint64, leaderGUID uint64, dungeons []uint32, leaderRoles uint8) *LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	rc := &LfgRoleCheck{
		GroupID:   groupID,
		Leader:    leaderGUID,
		Dungeons:  append([]uint32(nil), dungeons...),
		Roles:     make(map[uint64]uint8),
		State:     LFGRoleCheckInitializing,
		CreatedAt: time.Now(),
	}
	rc.Roles[leaderGUID] = leaderRoles
	m.roleChecks[groupID] = rc
	return rc
}

func (m *LFGManager) GetRoleCheck(groupID uint64) *LfgRoleCheck {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.roleChecks[groupID]
}

func (m *LFGManager) UpdateRoleCheck(groupID uint64, guid uint64, roles uint8) *LfgRoleCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	rc, ok := m.roleChecks[groupID]
	if !ok {
		return nil
	}
	rc.Roles[guid] = roles
	allAnswered := true
	for _, r := range rc.Roles {
		if r == LFGRoleNone {
			allAnswered = false
			break
		}
	}
	if allAnswered {
		rc.State = LFGRoleCheckFinished
	}
	return rc
}

func (m *LFGManager) CreateProposal(dungeonID uint32, groupID uint64, leaderGUID uint64, players map[uint64]uint8) *LFGProposal {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextProposalID++
	prop := &LFGProposal{
		ID:         m.nextProposalID,
		DungeonID:  dungeonID,
		State:      LFGProposalInitiating,
		Group:      groupID,
		Leader:     leaderGUID,
		CancelTime: time.Now().Add(45 * time.Second),
		Players:    make(map[uint64]*LFGProposalPlayer, len(players)),
	}
	for guid, role := range players {
		prop.Players[guid] = &LFGProposalPlayer{
			Role:   role,
			Group:  groupID,
			Accept: LFGAnswerPending,
		}
	}
	m.proposals[prop.ID] = prop
	return prop
}

func (m *LFGManager) GetProposal(proposalID uint32) *LFGProposal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.proposals[proposalID]
}

func (m *LFGManager) UpdateProposal(proposalID uint32, guid uint64, accept bool) (*LFGProposal, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prop, ok := m.proposals[proposalID]
	if !ok {
		return nil, false
	}
	player, ok := prop.Players[guid]
	if !ok {
		return nil, false
	}
	if !accept {
		player.Accept = LFGAnswerDeny
		prop.State = LFGProposalFailed
		delete(m.proposals, proposalID)
		return prop, true
	}

	player.Accept = LFGAnswerAgree
	allAgree := true
	for _, p := range prop.Players {
		if p.Accept != LFGAnswerAgree {
			allAgree = false
			break
		}
	}
	if allAgree {
		prop.State = LFGProposalSuccess
		delete(m.proposals, proposalID)
	}
	return prop, true
}

func (s *session) handleLFGJoin(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	b := protocol.NewReader(payload)
	roles, err := b.ReadU32()
	if err != nil {
		return false
	}
	if _, err := b.ReadBool(); err != nil {
		return false
	}
	if _, err := b.ReadBool(); err != nil {
		return false
	}
	count, err := b.ReadU8()
	if err != nil || count > 50 {
		return false
	}
	dungeons := make([]uint32, 0, count)
	for index := uint8(0); index < count; index++ {
		dungeon, readErr := b.ReadU32()
		if readErr != nil {
			return false
		}
		dungeons = append(dungeons, dungeon&0x00FFFFFF)
	}
	needsCount, err := b.ReadU8()
	if err != nil {
		return false
	}
	if needsCount > 16 || b.Remaining() < int(needsCount) {
		return false
	}
	if _, err := b.Read(int(needsCount)); err != nil {
		return false
	}
	comment, err := b.ReadCString()
	if err != nil {
		return false
	}
	if len(comment) > 255 {
		comment = comment[:255]
	}
	result, entry := s.server.Features.LFG.Join(s.playerGUID, uint8(roles), dungeons, comment)
	s.debug("lfg join", "account", s.accountName, "result", result, "dungeons", entry.Dungeons)
	if err := s.sendLFGJoinResult(result, uint32(LFGStateNone)); err != nil {
		return false
	}
	if result == LFGJoinOK {
		return s.sendLFGUpdatePlayer(LFGUpdateJoinQueue, entry) == nil
	}
	return true
}

func (s *session) handleLFGLeave() bool {
	if !s.playerLoaded {
		return true
	}
	if s.server.Features.LFG.Leave(s.playerGUID) {
		s.debug("lfg leave", "account", s.accountName)
	}
	return s.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: s.playerGUID, State: LFGStateNone}) == nil
}

func (s *session) handleLFGGetStatus() bool {
	if !s.playerLoaded {
		return true
	}
	entry, _ := s.server.Features.LFG.Status(s.playerGUID)
	if err := s.sendLFGUpdatePlayer(LFGUpdateStatus, entry); err != nil {
		return false
	}
	return s.sendLFGUpdateParty(LFGUpdateStatus) == nil
}

func (s *session) sendLFGJoinResult(result uint32, state uint32) error {
	packet := protocol.NewBuffer(8)
	packet.WriteU32(result)
	packet.WriteU32(state)
	return s.write(uint16(protocol.OpcodeSMSG_LFG_JOIN_RESULT), packet.Bytes(), true)
}

func (s *session) sendLFGUpdatePlayer(updateType uint8, entry LFGQueueEntry) error {
	packet := protocol.NewBuffer(16 + len(entry.Dungeons)*4 + len(entry.Comment))
	packet.WriteU8(updateType)
	if len(entry.Dungeons) == 0 || entry.State == LFGStateNone {
		packet.WriteU8(0)
	} else {
		packet.WriteU8(1)
		packet.WriteU8(1)
		packet.WriteU8(0)
		packet.WriteU8(0)
		packet.WriteU8(uint8(len(entry.Dungeons)))
		for _, dungeon := range entry.Dungeons {
			packet.WriteU32(dungeon)
		}
		packet.WriteCString(entry.Comment)
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_UPDATE_PLAYER), packet.Bytes(), true)
}

func (s *session) sendLFGUpdateParty(updateType uint8) error {
	packet := protocol.NewBuffer(2)
	packet.WriteU8(updateType)
	packet.WriteU8(0)
	return s.write(uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY), packet.Bytes(), true)
}

func (s *session) getLFGDungeonEntrance(dungeonID uint32) (mapID uint32, x, y, z, ori float32) {
	baseID := dungeonID & 0x00FFFFFF
	if s.server != nil && s.server.Data != nil {
		dungeon, found, _ := s.server.Data.LFGDungeon(baseID)
		if found && dungeon.MapID >= 0 {
			mapID = uint32(dungeon.MapID)
		}
	}
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var posX, posY, posZ, orientation float32
		err := s.server.WorldStore.DB.QueryRow(
			"SELECT position_x, position_y, position_z, orientation FROM lfg_dungeon_template WHERE dungeonId = ? LIMIT 1",
			baseID,
		).Scan(&posX, &posY, &posZ, &orientation)
		if err == nil {
			return mapID, posX, posY, posZ, orientation
		}
	}
	return mapID, 0, 0, 0, 0
}

func (s *session) teleportToLFGDungeon(dungeonID uint32) {
	if s.player == nil {
		return
	}
	s.player.LfgEntryPointMap = s.player.Map
	s.player.LfgEntryPointX = s.player.X
	s.player.LfgEntryPointY = s.player.Y
	s.player.LfgEntryPointZ = s.player.Z
	s.player.LfgEntryPointO = s.player.Orientation

	mapID, x, y, z, ori := s.getLFGDungeonEntrance(dungeonID)
	s.teleportTo(mapID, x, y, z, ori)
}

func (s *session) sendLFGRoleCheckUpdate(roleCheck *LfgRoleCheck) error {
	packet := protocol.NewBuffer(64)
	packet.WriteU32(roleCheck.State)
	if roleCheck.State == LFGRoleCheckInitializing {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU8(uint8(len(roleCheck.Dungeons)))
	for _, d := range roleCheck.Dungeons {
		packet.WriteU32(d)
	}
	packet.WriteU8(uint8(len(roleCheck.Roles)))

	// Leader first
	leaderRoles := roleCheck.Roles[roleCheck.Leader]
	packet.WriteU64(roleCheck.Leader)
	if leaderRoles > 0 {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(uint32(leaderRoles))
	leaderLevel := uint8(80)
	if s.server != nil {
		if ls := s.server.findSessionByGUID(roleCheck.Leader); ls != nil && ls.player != nil {
			leaderLevel = ls.player.Level
		}
	}
	packet.WriteU8(leaderLevel)

	// Other members
	for pguid, roles := range roleCheck.Roles {
		if pguid == roleCheck.Leader {
			continue
		}
		packet.WriteU64(pguid)
		if roles > 0 {
			packet.WriteU8(1)
		} else {
			packet.WriteU8(0)
		}
		packet.WriteU32(uint32(roles))
		level := uint8(80)
		if s.server != nil {
			if ms := s.server.findSessionByGUID(pguid); ms != nil && ms.player != nil {
				level = ms.player.Level
			}
		}
		packet.WriteU8(level)
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_ROLE_CHECK_UPDATE), packet.Bytes(), true)
}

func (s *session) sendLFGProposalUpdate(proposal *LFGProposal) error {
	guid := s.playerGUID
	packet := protocol.NewBuffer(64)
	packet.WriteU32(proposal.DungeonID)
	packet.WriteU8(proposal.State)
	packet.WriteU32(proposal.ID)
	packet.WriteU32(proposal.Encounters)
	if proposal.Silent {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU8(uint8(len(proposal.Players)))

	for pguid, pdata := range proposal.Players {
		packet.WriteU32(uint32(pdata.Role))
		if pguid == guid {
			packet.WriteU8(1) // isSelf
		} else {
			packet.WriteU8(0)
		}
		packet.WriteU8(0) // inDungeon
		if s.groupID != 0 && pdata.Group == s.groupID {
			packet.WriteU8(1) // sameGroup
		} else {
			packet.WriteU8(0)
		}
		if pdata.Accept != LFGAnswerPending {
			packet.WriteU8(1) // answered
		} else {
			packet.WriteU8(0)
		}
		if pdata.Accept == LFGAnswerAgree {
			packet.WriteU8(1) // accepted
		} else {
			packet.WriteU8(0)
		}
	}
	return s.write(uint16(protocol.OpcodeSMSG_LFG_PROPOSAL_UPDATE), packet.Bytes(), true)
}

func (s *session) getLockedDungeons(guid uint64) map[uint32]uint32 {
	locks := make(map[uint32]uint32)
	if s.server == nil || s.server.Data == nil {
		return locks
	}

	allDungeons, err := s.server.Data.LFGDungeons()
	if err != nil || len(allDungeons) == 0 {
		return locks
	}

	targetSess := s.server.findSessionByGUID(guid)
	var level uint8 = 80
	var expansion uint8 = s.accountExpansion
	if targetSess != nil {
		if targetSess.player != nil {
			level = targetSess.player.Level
		}
		expansion = targetSess.accountExpansion
	} else if guid == s.playerGUID && s.player != nil {
		level = s.player.Level
	}

	// 1. Check bound instances from character_instance JOIN instance
	type boundKey struct {
		mapID      uint32
		difficulty uint32
	}
	boundInstances := make(map[boundKey]bool)
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		now := time.Now().Unix()
		rows, err := s.server.CharactersStore.DB.Query(
			"SELECT i.map, i.difficulty, i.resettime FROM character_instance ci JOIN instance i ON ci.instance = i.id WHERE ci.guid = ?",
			guid,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mapID, diff uint32
				var resetTime int64
				if err := rows.Scan(&mapID, &diff, &resetTime); err == nil {
					if resetTime > now {
						boundInstances[boundKey{mapID: mapID, difficulty: diff}] = true
					}
				}
			}
		}
	}

	// 2. Check access requirements (e.g. min item level)
	accessReqItemLevel := make(map[boundKey]uint32)
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.Query(
			"SELECT mapId, difficulty, item_level FROM access_requirement WHERE item_level > 0",
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var mapID, diff, itemLvl uint32
				if err := rows.Scan(&mapID, &diff, &itemLvl); err == nil {
					accessReqItemLevel[boundKey{mapID: mapID, difficulty: diff}] = itemLvl
				}
			}
		}
	}

	// 3. Compute player's average item level
	var avgItemLevel float32
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var sumItemLevel int64
		var countItemLevel int64
		var itemGUIDs []uint64
		rows, err := s.server.CharactersStore.DB.Query(
			"SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND slot >= 0 AND slot <= 18 AND slot NOT IN (6, 14, 17, 18) AND item != 0",
			guid,
		)
		if err == nil {
			for rows.Next() {
				var itemGUID uint64
				if err := rows.Scan(&itemGUID); err == nil && itemGUID != 0 {
					itemGUIDs = append(itemGUIDs, itemGUID)
				}
			}
			rows.Close()

			for _, itemGUID := range itemGUIDs {
				var itemEntry uint32
				if err := s.server.CharactersStore.DB.QueryRow("SELECT itemEntry FROM item_instance WHERE guid = ?", itemGUID).Scan(&itemEntry); err == nil && itemEntry != 0 {
					var itemLvl int64
					if err := s.server.WorldStore.DB.QueryRow("SELECT ItemLevel FROM item_template WHERE entry = ?", itemEntry).Scan(&itemLvl); err == nil && itemLvl > 0 {
						sumItemLevel += itemLvl
					}
				}
				countItemLevel++
			}
			if countItemLevel > 0 {
				avgItemLevel = float32(sumItemLevel) / float32(countItemLevel)
			}
		}
	}

	for _, dungeon := range allDungeons {
		if dungeon.TypeID == LFGDungeonTypeRandom {
			continue
		}

		var lockData uint32
		if dungeon.ExpansionLevel > uint32(expansion) {
			lockData = LFGLockStatusInsufficientExpansion
		} else if uint32(level) < dungeon.MinLevel {
			lockData = LFGLockStatusTooLowLevel
		} else if dungeon.MaxLevel > 0 && uint32(level) > dungeon.MaxLevel {
			lockData = LFGLockStatusTooHighLevel
		} else if dungeon.Difficulty > 0 && boundInstances[boundKey{mapID: uint32(dungeon.MapID), difficulty: dungeon.Difficulty}] {
			lockData = LFGLockStatusRaidLocked
		} else if reqILvl, ok := accessReqItemLevel[boundKey{mapID: uint32(dungeon.MapID), difficulty: dungeon.Difficulty}]; ok && reqILvl > 0 && avgItemLevel < float32(reqILvl) {
			lockData = LFGLockStatusTooLowGearScore
		}

		if lockData != 0 {
			locks[dungeon.Entry()] = lockData
		}
	}

	return locks
}

// handleLfdPartyLockInfoRequest processes CMSG_LFD_PARTY_LOCK_INFO_REQUEST (0x371).
// Reference: WorldSession::HandleLfdPartyLockInfoRequestOpcode (LFGHandler.cpp:115).
func (s *session) handleLfdPartyLockInfoRequest(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.server == nil || s.groupID == 0 {
		buf := protocol.NewBuffer(1)
		buf.WriteU8(0) // count = 0 players
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_PARTY_INFO), buf.Bytes(), true)
		return true
	}

	grp := s.server.getGroup(s.groupID)
	if grp == nil {
		buf := protocol.NewBuffer(1)
		buf.WriteU8(0)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_PARTY_INFO), buf.Bytes(), true)
		return true
	}

	type memberLock struct {
		guid  uint64
		locks map[uint32]uint32
	}
	var otherLocks []memberLock
	for _, m := range grp.Members {
		if m.GUID == s.playerGUID {
			continue
		}
		locks := s.getLockedDungeons(m.GUID)
		otherLocks = append(otherLocks, memberLock{guid: m.GUID, locks: locks})
	}

	buf := protocol.NewBuffer(64)
	buf.WriteU8(uint8(len(otherLocks)))
	for _, ml := range otherLocks {
		buf.WriteU64(ml.guid)
		buf.WriteU32(uint32(len(ml.locks)))
		entries := make([]uint32, 0, len(ml.locks))
		for entry := range ml.locks {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
		for _, entry := range entries {
			buf.WriteU32(entry)
			buf.WriteU32(ml.locks[entry])
		}
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_LFG_PARTY_INFO), buf.Bytes(), true)
	return true
}

// handleLfdPlayerLockInfoRequest processes CMSG_LFD_PLAYER_LOCK_INFO_REQUEST (0x36E).
// Reference: WorldSession::HandleLfgPlayerLockInfoRequestOpcode (LFGHandler.cpp:154).
func (s *session) handleLfdPlayerLockInfoRequest(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}

	var randomDungeons []wotlk.LFGDungeon
	var lockedDungeons map[uint32]uint32

	if s.server != nil && s.server.Data != nil {
		allDungeons, err := s.server.Data.LFGDungeons()
		if err == nil {
			for _, d := range allDungeons {
				if d.TypeID == LFGDungeonTypeRandom {
					if d.ExpansionLevel <= uint32(s.accountExpansion) &&
						d.MinLevel <= uint32(s.player.Level) &&
						(d.MaxLevel == 0 || uint32(s.player.Level) <= d.MaxLevel) {
						randomDungeons = append(randomDungeons, d)
					}
				}
			}
		}
		lockedDungeons = s.getLockedDungeons(s.playerGUID)
	}

	bufSize := 5 + len(randomDungeons)*22 + len(lockedDungeons)*8
	buf := protocol.NewBuffer(bufSize)
	buf.WriteU8(uint8(len(randomDungeons)))
	for _, rd := range randomDungeons {
		buf.WriteU32(rd.Entry())
		buf.WriteU8(0)  // done
		buf.WriteU32(0) // quest money
		buf.WriteU32(0) // quest xp
		buf.WriteU32(0) // unknown1
		buf.WriteU32(0) // unknown2
		buf.WriteU8(0)  // item rewards count
	}

	buf.WriteU32(uint32(len(lockedDungeons)))
	if len(lockedDungeons) > 0 {
		entries := make([]uint32, 0, len(lockedDungeons))
		for entry := range lockedDungeons {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i] < entries[j] })
		for _, entry := range entries {
			buf.WriteU32(entry)
			buf.WriteU32(lockedDungeons[entry])
		}
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_LFG_PLAYER_INFO), buf.Bytes(), true)
	return true
}

// handleLfgProposalResult processes CMSG_LFG_PROPOSAL_RESULT (0x362).
// Reference: WorldSession::HandleLfgProposalResultOpcode (LFGHandler.cpp:68).
func (s *session) handleLfgProposalResult(ctx context.Context, payload []byte) bool {
	if len(payload) < 5 {
		return true
	}
	r := protocol.NewReader(payload)
	proposalID, _ := r.ReadU32()
	acceptVal, _ := r.ReadU8()
	accept := acceptVal != 0
	s.debug("lfg proposal result", "account", s.accountName, "proposal", proposalID, "accept", accept)

	if s.server == nil || s.server.Features == nil || s.server.Features.LFG == nil {
		return true
	}

	lfg := s.server.Features.LFG
	proposal, updated := lfg.UpdateProposal(proposalID, s.playerGUID, accept)
	if !updated || proposal == nil {
		return true
	}

	if !accept {
		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				_ = ps.sendLFGProposalUpdate(proposal)
				_ = ps.sendLFGUpdatePlayer(LFGUpdateProposalDeclined, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
			}
		}
		return true
	}

	if proposal.State == LFGProposalSuccess {
		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				_ = ps.sendLFGProposalUpdate(proposal)
				_ = ps.sendLFGUpdatePlayer(LFGUpdateGroupFound, LFGQueueEntry{GUID: pguid, State: LFGStateDungeon})
				_ = ps.sendLFGUpdatePlayer(LFGUpdateRemovedFromQueue, LFGQueueEntry{GUID: pguid, State: LFGStateNone})
				ps.updateAchievementCriteria(criteriaTypeUseLFDToGroup, 0, 1)
			}
		}

		if proposal.Group != 0 {
			grp := s.server.getGroup(proposal.Group)
			if grp != nil {
				s.server.groupsMu.Lock()
				grp.IsLFG = true
				grp.LFGDungeonID = proposal.DungeonID
				grp.LFGState = LFGStateDungeon
				s.server.groupsMu.Unlock()
				if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
					_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(), "REPLACE INTO lfg_data (guid, dungeon, state) VALUES (?, ?, ?)", proposal.Group, proposal.DungeonID, LFGStateDungeon)
				}
			}
		}

		for pguid := range proposal.Players {
			if ps := s.server.findSessionByGUID(pguid); ps != nil {
				ps.teleportToLFGDungeon(proposal.DungeonID)
			}
		}
		return true
	}

	// Still pending: broadcast updated progress to players
	for pguid := range proposal.Players {
		if ps := s.server.findSessionByGUID(pguid); ps != nil {
			_ = ps.sendLFGProposalUpdate(proposal)
		}
	}
	return true
}

// handleLfgSetBootVote processes CMSG_LFG_SET_BOOT_VOTE (0x367).
// Reference: WorldSession::HandleLfgSetBootVoteOpcode (LFGHandler.cpp:80).
func (s *session) handleLfgSetBootVote(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 {
		return true
	}
	agree := payload[0]
	s.debug("lfg boot vote", "account", s.accountName, "agree", agree)
	s.updateAchievementCriteria(criteriaTypeLFGVoteKick, 0, 1)
	return true
}

// handleLfgSetRoles processes CMSG_LFG_SET_ROLES (0x35E).
// Reference: WorldSession::HandleLfgSetRolesOpcode (LFGHandler.cpp:52).
func (s *session) handleLfgSetRoles(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 {
		return true
	}
	roles := payload[0] & (LFGRoleTank | LFGRoleHealer | LFGRoleDamage | LFGRoleLeader)
	s.debug("lfg set roles", "account", s.accountName, "roles", roles)
	if roles > 0 {
		s.updateAchievementCriteria(criteriaTypeLFGAnyRole, 0, 1)
	}

	if s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		lfg := s.server.Features.LFG
		lfg.mu.Lock()
		if entry, ok := lfg.queue[s.playerGUID]; ok {
			entry.Roles = roles & (LFGRoleTank | LFGRoleHealer | LFGRoleDamage)
			lfg.queue[s.playerGUID] = entry
		}
		lfg.mu.Unlock()

		if s.groupID != 0 {
			rc := lfg.UpdateRoleCheck(s.groupID, s.playerGUID, roles)
			if rc != nil {
				grp := s.server.getGroup(s.groupID)
				if grp != nil {
					for _, m := range grp.Members {
						if ms := s.server.findSessionByGUID(m.GUID); ms != nil {
							_ = ms.sendLFGRoleCheckUpdate(rc)
						}
					}
				}
				if rc.State == LFGRoleCheckFinished {
					s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_UPDATE_PARTY), []byte{LFGUpdateAddedToQueue, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0})
				}
			}
		}
	}

	// SMSG_LFG_ROLE_CHOSEN (0x2BB): guid (8), ready (1), roles (4)
	packet := protocol.NewBuffer(13)
	packet.WriteU64(s.playerGUID)
	if roles > 0 {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(uint32(roles))
	if s.groupID != 0 && s.server != nil {
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LFG_ROLE_CHOSEN), packet.Bytes())
	} else {
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_ROLE_CHOSEN), packet.Bytes(), true)
	}
	return true
}

// handleLfgTeleport processes CMSG_LFG_TELEPORT (0x369).
// Reference: WorldSession::HandleLfgTeleportOpcode (LFGHandler.cpp:125).
func (s *session) handleLfgTeleport(ctx context.Context, payload []byte) bool {
	if len(payload) < 1 || s.player == nil {
		return true
	}
	out := payload[0] != 0
	s.debug("lfg teleport", "account", s.accountName, "out", out)

	var lfgDungeonID uint32
	if s.groupID != 0 && s.server != nil {
		grp := s.server.getGroup(s.groupID)
		if grp != nil && grp.IsLFG {
			lfgDungeonID = grp.LFGDungeonID
		}
	}
	if lfgDungeonID == 0 && s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		entry, ok := s.server.Features.LFG.Status(s.playerGUID)
		if ok && len(entry.Dungeons) > 0 {
			lfgDungeonID = entry.Dungeons[0]
		}
	}

	if lfgDungeonID == 0 {
		// SMSG_LFG_TELEPORT_DENIED (0x200): error (4) -> LFG_TELEPORTERROR_INVALID_LOCATION (6)
		packet := protocol.NewBuffer(4)
		packet.WriteU32(LFGTeleportErrorInvalidLocation)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
		return true
	}

	if out {
		destMap := s.player.LfgEntryPointMap
		destX := s.player.LfgEntryPointX
		destY := s.player.LfgEntryPointY
		destZ := s.player.LfgEntryPointZ
		destOri := s.player.LfgEntryPointO

		if destMap == 0 && destX == 0 && destY == 0 {
			destMap = 0
			destX = -8949.95
			destY = 512.28
			destZ = 96.35
		}
		s.teleportTo(destMap, destX, destY, destZ, destOri)
		return true
	}

	// Reference: LFGMgr::TeleportPlayer (LFGMgr.cpp:1529-1582)
	teleportErr := LFGTeleportErrorOK
	if s.player.Health == 0 || (s.player.PlayerFlags&playerFlagGhost != 0) {
		teleportErr = LFGTeleportErrorPlayerDead
	} else if s.isFalling {
		teleportErr = LFGTeleportErrorFalling
	} else if s.isFatigueActive() {
		teleportErr = LFGTeleportErrorFatigue
	} else if s.player.VehicleGUID != 0 {
		teleportErr = LFGTeleportErrorInVehicle
	} else if s.hasAura(9454) { // Freeze debuff (Player.cpp:9454 check)
		teleportErr = LFGTeleportErrorInvalidLocation
	}

	if teleportErr != LFGTeleportErrorOK {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(teleportErr)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
		return true
	}

	dungeonMap, _, _, _, _ := s.getLFGDungeonEntrance(lfgDungeonID)
	if dungeonMap != 0 && s.player.Map == dungeonMap {
		packet := protocol.NewBuffer(4)
		packet.WriteU32(LFGTeleportErrorInvalidLocation)
		_ = s.write(uint16(protocol.OpcodeSMSG_LFG_TELEPORT_DENIED), packet.Bytes(), true)
		return true
	}

	s.finishTaxiFlight()
	s.teleportToLFGDungeon(lfgDungeonID)
	return true
}

// handleSearchLfgJoin processes CMSG_SEARCH_LFG_JOIN (0x35C).
// Reference: WorldSession::HandleSearchLfgJoinOpcode (LFGHandler.cpp:145).
func (s *session) handleSearchLfgJoin(ctx context.Context, payload []byte) bool {
	if len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	dungeonID, _ := r.ReadU32()
	s.debug("search lfg join", "account", s.accountName, "dungeon", dungeonID)
	return true
}

// handleSearchLfgLeave processes CMSG_SEARCH_LFG_LEAVE (0x35D).
// Reference: WorldSession::HandleSearchLfgLeaveOpcode (LFGHandler.cpp:160).
func (s *session) handleSearchLfgLeave(ctx context.Context, payload []byte) bool {
	if len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	dungeonID, _ := r.ReadU32()
	s.debug("search lfg leave", "account", s.accountName, "dungeon", dungeonID)
	return true
}

// handleSetLfgComment processes CMSG_SET_LFG_COMMENT (0x368).
// Reference: WorldSession::HandleLfgSetCommentOpcode (LFGHandler.cpp:60).
func (s *session) handleSetLfgComment(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	comment, _ := r.ReadCString()
	if s.server != nil && s.server.Features != nil && s.server.Features.LFG != nil {
		lfg := s.server.Features.LFG
		lfg.mu.Lock()
		if entry, ok := lfg.queue[s.playerGUID]; ok {
			entry.Comment = comment
			lfg.queue[s.playerGUID] = entry
		}
		lfg.mu.Unlock()
	}
	s.debug("lfg set comment", "account", s.accountName, "comment", comment)
	return true
}

// completeLFGDungeon rewards players in an LFG dungeon group and triggers criteria.
func (s *Server) completeLFGDungeon(groupID uint32, dungeonID uint32) {
	s.groupsMu.Lock()
	grp := s.groups[uint64(groupID)]
	if grp == nil || !grp.IsLFG {
		s.groupsMu.Unlock()
		return
	}
	if grp.LFGState == LFGStateFinishedDungeon {
		s.groupsMu.Unlock()
		return
	}
	grp.LFGState = LFGStateFinishedDungeon
	grp.LFGDungeonID = dungeonID
	members := append([]groupMember(nil), grp.Members...)
	s.groupsMu.Unlock()
	if s.CharactersStore != nil && s.CharactersStore.DB != nil {
		_, _ = s.CharactersStore.DB.ExecContext(context.Background(), "REPLACE INTO lfg_data (guid, dungeon, state) VALUES (?, ?, ?)", groupID, dungeonID, LFGStateFinishedDungeon)
	}
	for _, m := range members {
		if sess := s.findSessionByGUID(m.GUID); sess != nil {
			sess.updateAchievementCriteria(criteriaTypeLFGCompletion, dungeonID, 1)
			sess.updateAchievementCriteria(criteriaTypeLFGDungeonReward, 0, 1)
			_ = sess.sendLFGDungeonReward(dungeonID)
		}
	}
}

// sendLFGDungeonReward sends SMSG_LFG_PLAYER_REWARD (0x1FF) to the client.
func (s *session) sendLFGDungeonReward(dungeonID uint32) error {
	packet := protocol.NewBuffer(32)
	packet.WriteU32(dungeonID)
	packet.WriteU32(dungeonID)
	packet.WriteU32(0) // copper
	packet.WriteU32(0) // xp
	packet.WriteU8(0)  // reward count
	return s.write(uint16(protocol.OpcodeSMSG_LFG_PLAYER_REWARD), packet.Bytes(), true)
}
