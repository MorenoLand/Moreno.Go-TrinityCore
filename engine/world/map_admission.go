package world

import (
	"context"
	"database/sql"
	"errors"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	transferAbortMaxPlayers       uint8 = 0x02
	transferAbortTooManyInstances uint8 = 0x04
	transferAbortZoneInCombat     uint8 = 0x06
	transferAbortExpansionLevel   uint8 = 0x07
	transferAbortDifficulty       uint8 = 0x08
	transferAbortMapNotAllowed    uint8 = 0x10
)

func (s *session) sendTransferAborted(mapID uint32, reason, argument uint8) {
	buf := protocol.NewBuffer(6)
	buf.WriteU32(mapID)
	buf.WriteU8(reason)
	if reason == transferAbortExpansionLevel || reason == transferAbortDifficulty || reason == 0x09 {
		buf.WriteU8(argument)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_TRANSFER_ABORTED), buf.Bytes(), true)
}

func (s *session) skipMapDisableCheck(ctx context.Context) bool {
	if s == nil || s.server == nil || s.server.AuthStore == nil || s.server.AuthStore.DB == nil {
		return false
	}
	allowed, err := accountHasPermission(ctx, s.server.AuthStore.DB, s.accountID, s.server.RealmID, s.security, permissionSkipCheckDisableMap)
	return err == nil && allowed
}

type mapEntryDenyReason uint8

const (
	mapEntryAllowed mapEntryDenyReason = iota
	mapEntryNoEntry
	mapEntryUninstancedDungeon
	mapEntryDifficultyUnavailable
	mapEntryUnspecified
	mapEntryNotInRaid
	mapEntryCorpseInDifferentInstance
	mapEntryInstanceBindMismatch
	mapEntryTooManyInstances
	mapEntryMaxPlayers
	mapEntryZoneInCombat
)

type mapEntryCheck struct {
	Reason              mapEntryDenyReason
	RequestedDifficulty uint8
}

func (s *session) playerCannotEnterMap(ctx context.Context, mapID uint32) mapEntryCheck {
	if s == nil || s.server == nil || s.server.Data == nil || s.player == nil {
		return mapEntryCheck{Reason: mapEntryNoEntry}
	}
	entry, found, err := s.server.Data.Map(mapID)
	if err != nil || !found {
		return mapEntryCheck{Reason: mapEntryNoEntry}
	}
	if !entry.IsDungeon() {
		return mapEntryCheck{}
	}
	requestedDifficulty := s.player.DungeonDifficulty
	if entry.IsRaid() {
		requestedDifficulty = s.player.RaidDifficulty
	}
	check := mapEntryCheck{RequestedDifficulty: requestedDifficulty}
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		check.Reason = mapEntryUninstancedDungeon
		return check
	}
	var instanceTemplate uint32
	err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT map FROM instance_template WHERE map = ?", mapID).Scan(&instanceTemplate)
	if errors.Is(err, sql.ErrNoRows) || err != nil && missingTable(err) {
		check.Reason = mapEntryUninstancedDungeon
		return check
	}
	if err != nil {
		check.Reason = mapEntryUnspecified
		return check
	}
	_, difficulty, found, err := s.server.Data.DownscaledMapDifficulty(mapID, uint32(requestedDifficulty))
	if err != nil || !found {
		check.Reason = mapEntryDifficultyUnavailable
		return check
	}
	if s.isMapAdmissionGM() {
		return check
	}
	if !s.satisfiesMapAccessRequirement(ctx, mapID, difficulty) {
		check.Reason = mapEntryUnspecified
		return check
	}
	if entry.IsRaid() && !s.server.Config.InstanceIgnoreRaid && !s.isInRaidGroup() {
		check.Reason = mapEntryNotInRaid
		return check
	}
	if !s.corpseCanEnterMap(ctx, mapID) {
		check.Reason = mapEntryCorpseInDifferentInstance
		return check
	}
	group := s.server.findGroupByID(s.groupID)
	if group != nil && group.DBID != 0 && group.GroupType&groupTypeBattleground == 0 {
		groupDifficulty := uint32(group.DungeonDiff)
		if entry.IsRaid() {
			groupDifficulty = uint32(group.RaidDiff)
		}
		_, groupDifficulty, groupModeFound, modeErr := s.server.Data.DownscaledMapDifficulty(mapID, groupDifficulty)
		if modeErr != nil {
			check.Reason = mapEntryUnspecified
			return check
		}
		if groupModeFound {
			var groupInstanceID int64
			err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id FROM group_instance AS gi JOIN instance AS i ON i.id = gi.instance
				WHERE gi.guid = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`, group.DBID, mapID, groupDifficulty).Scan(&groupInstanceID)
			if err == nil && groupInstanceID > 0 && groupInstanceID <= int64(^uint32(0)) && s.hasActiveWorldportInstance(mapID, uint32(groupInstanceID)) {
				selection := worldportInstanceSelection{MapID: mapID, InstanceID: uint32(groupInstanceID), Difficulty: groupDifficulty}
				if s.worldportInstanceAtCapacity(selection, entry) {
					check.Reason = mapEntryMaxPlayers
					return check
				}
				if entry.IsRaid() && s.server.instanceEncounterInProgress(mapID, uint32(groupInstanceID)) {
					check.Reason = mapEntryZoneInCombat
					return check
				}
				var permanentInstanceID int64
				bindErr := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id FROM character_instance AS ci JOIN instance AS i ON i.id = ci.instance
					WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? AND ci.permanent <> 0 LIMIT 1`, s.playerGUID, mapID, groupDifficulty).Scan(&permanentInstanceID)
				if bindErr == nil && permanentInstanceID != groupInstanceID {
					check.Reason = mapEntryInstanceBindMismatch
					return check
				}
				if bindErr != nil && !errors.Is(bindErr, sql.ErrNoRows) && !missingTable(bindErr) {
					check.Reason = mapEntryUnspecified
					return check
				}
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
				check.Reason = mapEntryUnspecified
				return check
			}
		}
	}
	if !s.withinInstanceEntryLimit(ctx, mapID, difficulty) {
		check.Reason = mapEntryTooManyInstances
	}
	return check
}

func (s *session) canStartMapTeleport(ctx context.Context, mapID uint32) bool {
	check := s.playerCannotEnterMap(ctx, mapID)
	if check.Reason != mapEntryAllowed || s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	entry, found, err := s.server.Data.Map(mapID)
	if err != nil || !found || (entry.IsBattleground() || entry.IsBattleArena()) && s.bgData.InstanceID == 0 || uint32(s.accountExpansion) < entry.ExpansionID {
		return false
	}
	difficulty := uint32(check.RequestedDifficulty)
	if entry.IsDungeon() {
		_, difficulty, found, err = s.server.Data.DownscaledMapDifficulty(mapID, difficulty)
		if err != nil || !found {
			return false
		}
	}
	return !s.mapDisabledForTeleport(ctx, entry, mapID, difficulty) || s.skipMapDisableCheck(ctx)
}

func (s *session) sendAreaTriggerEntryFailure(ctx context.Context, mapID uint32, check mapEntryCheck) {
	reviveAtTrigger := false
	switch check.Reason {
	case mapEntryDifficultyUnavailable:
		s.sendTransferAborted(mapID, transferAbortDifficulty, check.RequestedDifficulty)
	case mapEntryNotInRaid:
		buf := protocol.NewBuffer(8)
		buf.WriteU32(0)
		buf.WriteU32(2)
		_ = s.write(uint16(protocol.OpcodeSMSG_RAID_GROUP_ONLY), buf.Bytes(), true)
		reviveAtTrigger = true
	case mapEntryCorpseInDifferentInstance:
		_ = s.write(uint16(protocol.OpcodeSMSG_CORPSE_NOT_IN_INSTANCE), nil, true)
	case mapEntryInstanceBindMismatch:
		if entry, found, err := s.server.Data.Map(mapID); err == nil && found {
			s.sendSystemMessage("You are already locked to " + entry.MapName + ".")
		}
		reviveAtTrigger = true
	case mapEntryTooManyInstances:
		s.sendTransferAborted(mapID, transferAbortTooManyInstances, 0)
		reviveAtTrigger = true
	case mapEntryMaxPlayers:
		s.sendTransferAborted(mapID, transferAbortMaxPlayers, 0)
		reviveAtTrigger = true
	case mapEntryZoneInCombat:
		s.sendTransferAborted(mapID, transferAbortZoneInCombat, 0)
		reviveAtTrigger = true
	case mapEntryNoEntry, mapEntryUninstancedDungeon:
		s.debug("area-trigger destination map cannot be created", "map", mapID, "reason", check.Reason)
	}
	if !reviveAtTrigger || s.player == nil || s.player.Health != 0 || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	var corpseMap int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId FROM corpse WHERE guid = ?", s.playerGUID).Scan(&corpseMap); err == nil && corpseMap == int64(mapID) {
		s.resurrectPlayer(ctx, 0.5)
		s.spawnCorpseBones(ctx)
	}
}

func (s *session) mapDisabledForTeleport(ctx context.Context, entry wotlk.MapEntry, mapID, difficulty uint32) bool {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var flags int64
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT flags FROM disables WHERE sourceType = 2 AND entry = ?", mapID).Scan(&flags)
	if errors.Is(err, sql.ErrNoRows) || err != nil && missingTable(err) {
		return false
	}
	if err != nil {
		return true
	}
	if !entry.IsDungeon() {
		return entry.InstanceType == 0 && flags == 0
	}
	var flag int64
	switch difficulty {
	case 0:
		flag = 0x01
	case 1:
		flag = 0x02
	case 4:
		flag = 0x04
	case 5:
		flag = 0x08
	}
	return flag != 0 && flags&flag != 0
}

func (s *session) isMapAdmissionGM() bool {
	return s != nil && s.player != nil && (s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0)
}

func (s *session) isInRaidGroup() bool {
	group := s.server.findGroupByID(s.groupID)
	return group != nil && group.IsRaid
}

func (s *session) satisfiesMapAccessRequirement(ctx context.Context, mapID, difficulty uint32) bool {
	cdb := s.server.WorldStore.DB
	var levelMin, levelMax, item, item2, questAlliance, questHorde, achievement int64
	err := cdb.QueryRowContext(ctx, `SELECT level_min, level_max, item, item2, quest_done_A, quest_done_H, completed_achievement
		FROM access_requirement WHERE mapId = ? AND difficulty = ?`, mapID, difficulty).Scan(&levelMin, &levelMax, &item, &item2, &questAlliance, &questHorde, &achievement)
	if errors.Is(err, sql.ErrNoRows) || err != nil && missingTable(err) {
		return true
	}
	if err != nil {
		return false
	}
	if !s.server.Config.InstanceIgnoreLevel && (levelMin > 0 && int64(s.player.Level) < levelMin || levelMax > 0 && int64(s.player.Level) > levelMax) {
		return false
	}
	if item > 0 && !s.hasMapAccessItem(ctx, uint32(item)) && (item2 <= 0 || !s.hasMapAccessItem(ctx, uint32(item2))) || item <= 0 && item2 > 0 && !s.hasMapAccessItem(ctx, uint32(item2)) {
		return false
	}
	quest := questAlliance
	if teamForRace(s.player.Race) != 0 {
		quest = questHorde
	}
	if quest > 0 && !s.isQuestRewarded(ctx, uint32(quest)) {
		return false
	}
	if achievement > 0 {
		leaderGUID := s.playerGUID
		if group := s.server.findGroupByID(s.groupID); group != nil {
			leaderGUID = group.LeaderGUID
		}
		leader := s.server.findSessionByGUID(leaderGUID)
		if leader == nil {
			return false
		}
		if _, found := leader.earnedAchievements[uint32(achievement)]; !found {
			return false
		}
	}
	return true
}

func (s *session) hasMapAccessItem(ctx context.Context, itemEntry uint32) bool {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || itemEntry == 0 {
		return false
	}
	rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.item, ii.count FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ? AND
		((ci.bag = 0 AND ci.slot < ?) OR (ci.bag = 0 AND ci.slot >= ? AND ci.slot < 150) OR (ci.bag >= ? AND ci.bag < ?))`, s.playerGUID, itemEntry, invSlotItemEnd, invSlotKeyringStart, invSlotBagStart, invSlotBagEnd)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var itemGUID, count int64
		if rows.Scan(&itemGUID, &count) != nil || itemGUID <= 0 || count <= 0 {
			continue
		}
		if s.trade != nil {
			inTrade := false
			for _, traded := range s.trade.Items {
				if traded.ItemGUID == uint64(itemGUID) {
					inTrade = true
					break
				}
			}
			if inTrade {
				continue
			}
		}
		return true
	}
	return false
}

func (s *session) corpseCanEnterMap(ctx context.Context, mapID uint32) bool {
	if !s.isDeadOrGhost() || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	var corpseMap int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT mapId FROM corpse WHERE guid = ?", s.playerGUID).Scan(&corpseMap)
	if errors.Is(err, sql.ErrNoRows) || err != nil && missingTable(err) {
		return true
	}
	if err != nil || corpseMap < 0 || corpseMap > int64(^uint32(0)) {
		return false
	}
	for currentMap := uint32(corpseMap); currentMap != 0; {
		if currentMap == mapID {
			return true
		}
		var parent int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT parent FROM instance_template WHERE map = ?", currentMap).Scan(&parent); err != nil || parent < 0 || parent > int64(^uint32(0)) {
			return false
		}
		currentMap = uint32(parent)
	}
	return false
}

func (s *session) withinInstanceEntryLimit(ctx context.Context, mapID, difficulty uint32) bool {
	if s.isDeadOrGhost() || s.server.Config.AccountInstancesPerHour < 0 {
		return true
	}
	if group := s.server.findGroupByID(s.groupID); group != nil && group.IsLFG {
		return true
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return false
	}
	var boundInstance int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id FROM character_instance AS ci
		JOIN instance AS i ON i.id = ci.instance WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`, s.playerGUID, mapID, difficulty).Scan(&boundInstance)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
		return false
	}
	if len(s.instanceLockTimes) < s.server.Config.AccountInstancesPerHour {
		return true
	}
	_, found := s.instanceLockTimes[uint32(boundInstance)]
	return found
}
