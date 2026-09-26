package world

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

type lootItem struct {
	Slot          uint8
	ItemEntry     uint32
	Count         uint32
	DisplayInfoID uint32
	Quality       uint32
	IsBlocked     bool
	RollWinner    uint64
}

type activeLootState struct {
	TargetGUID       uint64
	MapID            uint32
	InstanceID       uint32
	LootType         uint8
	Money            uint32
	Items            map[uint8]lootItem
	RoundRobinPlayer uint64
	Viewers          map[uint64]*session
}

type lootObjectKey struct {
	MapID      uint32
	InstanceID uint32
	GUID       uint64
}

type lootRollKey struct {
	Object lootObjectKey
	Slot   uint32
}

func (l *activeLootState) objectKey() lootObjectKey {
	return lootObjectKey{MapID: l.MapID, InstanceID: l.InstanceID, GUID: l.TargetGUID}
}

type lootOwnerState struct {
	PlayerGUID uint64
	GroupID    uint64
}

func (s *Server) creatureLootAllowed(mapID, instanceID uint32, targetGUID, standardGUID, playerGUID, groupID uint64) bool {
	if s == nil {
		return false
	}
	s.lootMu.Lock()
	owner, found := s.creatureLootOwners[lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: targetGUID}]
	if !found {
		owner, found = s.creatureLootOwners[lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: standardGUID}]
	}
	s.lootMu.Unlock()
	if !found {
		return true
	}
	if owner.GroupID != 0 {
		return groupID == owner.GroupID
	}
	return playerGUID == owner.PlayerGUID
}

func (l *activeLootState) addViewer(s *session) {
	if l.Viewers == nil {
		l.Viewers = make(map[uint64]*session)
	}
	l.Viewers[s.playerGUID] = s
}

func (l *activeLootState) removeViewer(guid uint64) {
	if l.Viewers != nil {
		delete(l.Viewers, guid)
	}
}

func (l *activeLootState) broadcastRemoved(slot uint8) {
	buf := protocol.NewBuffer(1)
	buf.WriteU8(slot)
	for _, sess := range l.Viewers {
		if sess != nil {
			_ = sess.write(uint16(protocol.OpcodeSMSG_LOOT_REMOVED), buf.Bytes(), true)
		}
	}
}

func (l *activeLootState) hasOverThresholdItem(threshold uint8) bool {
	for _, item := range l.Items {
		if item.Quality >= uint32(threshold) {
			return true
		}
	}
	return false
}

func sortedLootItems(items map[uint8]lootItem) []lootItem {
	result := make([]lootItem, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Slot < result[j].Slot })
	return result
}

func buildLootLooterPacket(loot *activeLootState, grp *groupState) []byte {
	packet := protocol.NewBuffer(24)
	packet.WriteU64(loot.TargetGUID)
	if grp != nil && grp.LootMethod == 2 && grp.MasterLooter != 0 && loot.hasOverThresholdItem(grp.LootThreshold) {
		packet.WritePackedGUID(grp.MasterLooter)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU8(0)
	return packet.Bytes()
}

const (
	rollPass       uint8 = 0
	rollNeed       uint8 = 1
	rollGreed      uint8 = 2
	rollDisenchant uint8 = 3

	rollFlagTypePass       uint8 = 0x01
	rollFlagTypeNeed       uint8 = 0x02
	rollFlagTypeGreed      uint8 = 0x04
	rollFlagTypeDisenchant uint8 = 0x08
	rollAllTypeMask        uint8 = 0x0F
)

type activeGroupRoll struct {
	SourceGUID          uint64
	Slot                uint32
	ItemEntry           uint32
	ItemCount           uint32
	RandomSuffix        uint32
	RandomPropID        uint32
	RollVoteMask        uint8
	GroupID             uint64
	MapID               uint32
	InstanceID          uint32
	EligiblePlayers     map[uint64]struct{}
	StartedAt           time.Time
	Duration            time.Duration
	TotalPlayersRolling int
	TotalNeed           int
	TotalGreed          int
	TotalPass           int
	Votes               map[uint64]uint8
	Rolls               map[uint64]uint8
	Timer               *time.Timer
}

func rollLootObjectKey(roll *activeGroupRoll) lootObjectKey {
	return lootObjectKey{MapID: roll.MapID, InstanceID: roll.InstanceID, GUID: roll.SourceGUID}
}

func (s *session) handleLoot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	if s.isDeadOrGhost() {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadPackedGUID()
	if err != nil {
		return false
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}
	high := uint16(targetGUID >> 48)

	if high == 0xF110 {
		lowGUID := uint32(targetGUID & 0x00FFFFFF)
		entry := uint32((targetGUID >> 24) & 0x00FFFFFF)

		var goMap uint32
		var goX, goY, goZ float32
		var data1 int64
		err := wdb.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.data1, 0)
			FROM gameobject AS g
			JOIN gameobject_template AS t ON t.entry = g.id
			WHERE g.guid = ? AND g.id = ? LIMIT 1`, lowGUID, entry).Scan(&goMap, &goX, &goY, &goZ, &data1)
		if err != nil {
			_ = wdb.QueryRowContext(ctx, `SELECT g.map, g.position_x, g.position_y, g.position_z, COALESCE(t.data1, 0)
				FROM gameobject AS g
				JOIN gameobject_template AS t ON t.entry = g.id
				WHERE g.guid = ? LIMIT 1`, lowGUID).Scan(&goMap, &goX, &goY, &goZ, &data1)
		}
		if goMap != s.player.Map || distance3D(s.player.X, s.player.Y, s.player.Z, goX, goY, goZ) > 10.0 {
			return s.sendLootError(targetGUID, 4) == nil
		}
		lootID := data1
		if lootID == 0 {
			lootID = int64(entry)
		}

		key := lootObjectKey{MapID: goMap, InstanceID: s.player.InstanceID, GUID: targetGUID}
		s.server.lootMu.Lock()
		if s.server.creatureLoot == nil {
			s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
		}
		loot := s.server.creatureLoot[key]
		newLoot := loot == nil
		if newLoot {
			loot = &activeLootState{TargetGUID: targetGUID, MapID: goMap, InstanceID: s.player.InstanceID, LootType: 1, Items: make(map[uint8]lootItem)}
			s.server.creatureLoot[key] = loot
		}
		s.server.lootMu.Unlock()

		if !newLoot {
			loot.addViewer(s)
			s.activeLoot = loot
			s.interruptCurrentCast()
			return s.sendLootResponse(loot) == nil
		}

		rows, err := wdb.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0)
			FROM gameobject_loot_template AS l
			LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? ORDER BY l.Item LIMIT 16`, lootID)
		if err != nil && isMissingColumn(err) {
			rows, err = wdb.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), 0
				FROM gameobject_loot_template AS l
				LEFT JOIN item_template AS t ON t.entry = l.Item
				WHERE l.Entry = ? ORDER BY l.Item LIMIT 16`, lootID)
		}
		if err == nil {
			defer rows.Close()
			var slot uint8 = 0
			for rows.Next() {
				var itemID int64
				var chance float64
				var minCount, maxCount, displayID, quality int64
				if err := rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID, &quality); err != nil {
					continue
				}
				roll := rand.Float64() * 100.0
				if chance > 0 && roll > chance {
					continue
				}
				count := uint32(minCount)
				if maxCount > minCount {
					count += uint32(rand.Intn(int(maxCount - minCount + 1)))
				}
				if count == 0 {
					count = 1
				}
				loot.Items[slot] = lootItem{
					Slot:          slot,
					ItemEntry:     uint32(itemID),
					Count:         count,
					DisplayInfoID: uint32(displayID),
					Quality:       uint32(quality),
				}
				slot++
				if slot >= 16 {
					break
				}
			}
		}
		if s.server != nil && s.groupID != 0 {
			s.server.groupsMu.Lock()
			grp := s.server.groups[s.groupID]
			if grp != nil && loot.RoundRobinPlayer == 0 && grp.LootMethod != 0 {
				grp.updateLooter(s.server, goMap, goX, goY, goZ)
				loot.RoundRobinPlayer = grp.LooterGUID
			}
			s.server.groupsMu.Unlock()
		}
		loot.addViewer(s)
		s.activeLoot = loot
		s.interruptCurrentCast()
		return s.sendLootResponse(loot) == nil
	}

	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Map != s.player.Map || target.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, target.X, target.Y, target.Z) > 5.0 {
		return s.sendLootError(targetGUID, 4) == nil
	}
	if target.Health != 0 {
		return s.sendLootError(targetGUID, 0) == nil
	}
	guid := uint32(targetGUID & 0x00FFFFFF)
	creatureEntry := uint32((targetGUID >> 24) & 0x00FFFFFF)
	stdKey := creatureWorldGUID(guid, creatureEntry)
	if !s.server.creatureLootAllowed(target.Map, target.InstanceID, targetGUID, stdKey, s.playerGUID, s.groupID) {
		return s.sendLootError(targetGUID, 0) == nil
	}

	s.server.lootMu.Lock()
	key := lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: targetGUID}
	standardKey := lootObjectKey{MapID: target.Map, InstanceID: target.InstanceID, GUID: stdKey}
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	loot := s.server.creatureLoot[key]
	if loot == nil {
		loot = s.server.creatureLoot[standardKey]
	}
	newLoot := loot == nil
	if newLoot {
		loot = &activeLootState{TargetGUID: targetGUID, MapID: target.Map, InstanceID: target.InstanceID, LootType: 1, Items: make(map[uint8]lootItem)}
		s.server.creatureLoot[key] = loot
		s.server.creatureLoot[standardKey] = loot
	}
	s.server.lootMu.Unlock()
	if !newLoot {
		if !s.server.creatureLootAllowed(target.Map, target.InstanceID, targetGUID, stdKey, s.playerGUID, s.groupID) {
			return s.sendLootError(targetGUID, 0) == nil
		}
		loot.addViewer(s)
		s.activeLoot = loot
		s.interruptCurrentCast()
		return s.sendLootResponse(loot) == nil
	}
	// Query min/max gold and lootid from creature_template
	var minGold, maxGold, lootID int64
	_ = wdb.QueryRowContext(ctx, "SELECT minGold, maxGold FROM creature_template WHERE entry = ? LIMIT 1", creatureEntry).Scan(&minGold, &maxGold)
	_ = wdb.QueryRowContext(ctx, "SELECT lootid FROM creature_template WHERE entry = ? LIMIT 1", creatureEntry).Scan(&lootID)
	if lootID == 0 {
		lootID = int64(creatureEntry)
	}
	if maxGold > minGold && maxGold > 0 {
		loot.Money = uint32(minGold + int64(rand.Intn(int(maxGold-minGold+1))))
	} else if minGold > 0 {
		loot.Money = uint32(minGold)
	}
	// Query creature_loot_template
	rows, err := wdb.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0)
		FROM creature_loot_template AS l
		LEFT JOIN item_template AS t ON t.entry = l.Item
		WHERE l.Entry = ? ORDER BY l.Item LIMIT 16`, lootID)
	if err != nil && isMissingColumn(err) {
		rows, err = wdb.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), 0
			FROM creature_loot_template AS l
			LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? ORDER BY l.Item LIMIT 16`, lootID)
	}
	if err == nil {
		defer rows.Close()
		var slot uint8 = 0
		for rows.Next() {
			var itemID int64
			var chance float64
			var minCount, maxCount, displayID, quality int64
			if err := rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID, &quality); err != nil {
				continue
			}
			// Roll chance (0-100%)
			roll := rand.Float64() * 100.0
			if chance > 0 && roll > chance {
				continue
			}
			count := uint32(minCount)
			if maxCount > minCount {
				count += uint32(rand.Intn(int(maxCount - minCount + 1)))
			}
			if count == 0 {
				count = 1
			}
			loot.Items[slot] = lootItem{
				Slot:          slot,
				ItemEntry:     uint32(itemID),
				Count:         count,
				DisplayInfoID: uint32(displayID),
				Quality:       uint32(quality),
			}
			slot++
			if slot >= 16 {
				break
			}
		}
	}
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		if grp != nil && loot.RoundRobinPlayer == 0 && grp.LootMethod != 0 {
			grp.updateLooter(s.server, target.Map, target.X, target.Y, target.Z)
			loot.RoundRobinPlayer = grp.LooterGUID
		}
		s.server.groupsMu.Unlock()
	}
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	return s.sendLootResponse(loot) == nil
}

func (s *session) handleFishingNodeUse(ctx context.Context, payload []byte, goState *dynamicGameObjectState) bool {
	return s.handleFishingUse(ctx, payload, goState, true, false)
}

func (s *session) handleFishingHoleUse(ctx context.Context, payload []byte, goState *dynamicGameObjectState) bool {
	return s.handleFishingUse(ctx, payload, goState, false, true)
}

func (s *session) handleFishingUse(ctx context.Context, payload []byte, goState *dynamicGameObjectState, requireOwner, forceSuccess bool) bool {
	if !s.playerLoaded || s.player == nil || goState == nil || (requireOwner && goState.OwnerGUID != s.playerGUID) {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadU64()
	if err != nil || targetGUID != goState.GUID {
		return true
	}
	if !forceSuccess && goState.State != GameObjectStateReady {
		_ = s.write(uint16(protocol.OpcodeSMSG_FISH_NOT_HOOKED), nil, true)
		return true
	}
	if goState.Map != s.player.Map || goState.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, goState.X, goState.Y, goState.Z) > 10.0 {
		return s.sendLootError(targetGUID, 4) == nil
	}
	if !requireOwner && goState.FishingMaxOpens > 0 && goState.FishingUses >= goState.FishingMaxOpens {
		return true
	}
	if requireOwner {
		goState.FishingHandled = true
	}
	loot := &activeLootState{TargetGUID: targetGUID, MapID: goState.Map, InstanceID: goState.InstanceID, LootType: 3, Items: make(map[uint8]lootItem)}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		if !requireOwner {
			goState.FishingUses++
		}
		return s.sendLootResponse(loot) == nil
	}
	var zoneSkill int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT skill FROM skill_fishing_base_level WHERE entry = ?", s.player.Zone).Scan(&zoneSkill); err != nil && !missingTable(err) {
		zoneSkill = 0
	}
	fishingSkill := uint16(0)
	fishingMax := uint16(0)
	for _, skill := range s.player.Skills {
		if skill.Skill == 356 {
			fishingSkill, fishingMax = skill.Value, skill.Max
			break
		}
	}
	if !forceSuccess && fishingMax > fishingSkill && fishingSkill > 0 {
		stepsNeeded := uint16(1)
		if fishingSkill >= 75 && fishingSkill <= 300 {
			stepsNeeded = fishingSkill / 44
		} else if fishingSkill > 300 {
			stepsNeeded = fishingSkill / 31
		}
		if stepsNeeded == 0 {
			stepsNeeded = 1
		}
		s.player.FishingSteps++
		if uint16(s.player.FishingSteps) >= stepsNeeded {
			s.player.FishingSteps = 0
			for index := range s.player.Skills {
				if s.player.Skills[index].Skill == 356 && s.player.Skills[index].Value < fishingMax {
					s.player.Skills[index].Value++
					if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
						_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_skills SET value = ? WHERE guid = ? AND skill = 356", s.player.Skills[index].Value, s.playerGUID)
					}
					break
				}
			}
		}
	}
	chance := 100
	if zoneSkill > 0 && int64(fishingSkill) < zoneSkill {
		chance = int(float64(fishingSkill) / float64(zoneSkill) * 100)
		if chance < 1 {
			chance = 1
		}
	}
	success := forceSuccess || s.fishingHoleNearby(ctx, goState) || rand.Intn(100)+1 <= chance
	loadRows := func(entry uint32, lootMode uint32) error {
		rows, queryErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT l.Item, l.Chance, l.MinCount, l.MaxCount, COALESCE(t.displayid, 0), COALESCE(t.Quality, 0)
			FROM fishing_loot_template AS l LEFT JOIN item_template AS t ON t.entry = l.Item
			WHERE l.Entry = ? AND (COALESCE(l.LootMode, 1) & ?) <> 0 ORDER BY l.Item LIMIT 16`, entry, lootMode)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		var slot uint8
		for rows.Next() && slot < 16 {
			var itemID int64
			var chance float64
			var minCount, maxCount, displayID, quality int64
			if scanErr := rows.Scan(&itemID, &chance, &minCount, &maxCount, &displayID, &quality); scanErr != nil {
				continue
			}
			if chance > 0 && rand.Float64()*100 > chance {
				continue
			}
			count := uint32(minCount)
			if maxCount > minCount {
				count += uint32(rand.Intn(int(maxCount - minCount + 1)))
			}
			if count == 0 {
				count = 1
			}
			loot.Items[slot] = lootItem{Slot: slot, ItemEntry: uint32(itemID), Count: count, DisplayInfoID: uint32(displayID), Quality: uint32(quality)}
			slot++
		}
		return rows.Err()
	}
	lootMode := uint32(1)
	if !success {
		lootMode = 0x8000
	}
	if err := loadRows(s.player.Zone, lootMode); err != nil && !missingTable(err) {
		return true
	}
	if len(loot.Items) == 0 && s.player.Zone != 1 {
		_ = loadRows(1, lootMode)
	}
	s.server.lootMu.Lock()
	if s.server.creatureLoot == nil {
		s.server.creatureLoot = make(map[lootObjectKey]*activeLootState)
	}
	s.server.creatureLoot[loot.objectKey()] = loot
	s.server.lootMu.Unlock()
	loot.addViewer(s)
	s.activeLoot = loot
	s.interruptCurrentCast()
	if err := s.sendLootResponse(loot); err != nil {
		return false
	}
	if requireOwner {
		s.server.despawnDynamicGameObjectInInstance(goState.Map, goState.InstanceID, targetGUID)
	} else {
		goState.FishingUses++
		if goState.FishingMaxOpens > 0 && goState.FishingUses >= goState.FishingMaxOpens {
			s.server.setGameObjectStateInInstance(goState.Map, goState.InstanceID, targetGUID, GameObjectStateActive)
			s.server.broadcastGameObjectDespawnInInstance(goState.Map, goState.InstanceID, targetGUID)
		} else {
			s.server.setGameObjectStateInInstance(goState.Map, goState.InstanceID, targetGUID, GameObjectStateReady)
		}
	}
	return true
}

func (s *session) fishingHoleNearby(ctx context.Context, bobber *dynamicGameObjectState) bool {
	if s == nil || s.server == nil || bobber == nil {
		return false
	}
	s.server.objectsMu.RLock()
	for _, object := range s.server.dynamicGameObjects {
		if object != nil && object.Type == GameObjectTypeFishingHole && object.Map == bobber.Map && distance3D(object.X, object.Y, object.Z, bobber.X, bobber.Y, bobber.Z) <= 20.0 {
			s.server.objectsMu.RUnlock()
			return true
		}
	}
	s.server.objectsMu.RUnlock()
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	var found int
	err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT 1
		FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.map = ? AND t.type = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ?
		LIMIT 1`, bobber.Map, GameObjectTypeFishingHole, bobber.X-20, bobber.X+20, bobber.Y-20, bobber.Y+20).Scan(&found)
	return err == nil && found == 1
}

func (s *session) sendLootResponse(loot *activeLootState) error {
	var grp *groupState
	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp = s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
	}

	packet := protocol.NewBuffer(8 + 1 + 4 + 1 + len(loot.Items)*22)
	packet.WriteU64(loot.TargetGUID)
	packet.WriteU8(loot.LootType)
	packet.WriteU32(loot.Money)
	packet.WriteU8(uint8(len(loot.Items)))
	items := sortedLootItems(loot.Items)
	for _, it := range items {
		var slotType uint8 = 0 // LOOT_SLOT_TYPE_ALLOW_LOOT
		if grp != nil {
			isOverThreshold := it.Quality >= uint32(grp.LootThreshold)
			switch grp.LootMethod {
			case 0: // Free for all
				slotType = 0
			case 1: // Round Robin
				if loot.RoundRobinPlayer != 0 && s.playerGUID != loot.RoundRobinPlayer {
					slotType = 3 // LOOT_SLOT_TYPE_LOCKED
				}
			case 2: // Master Loot
				if isOverThreshold {
					if s.playerGUID == grp.MasterLooter {
						slotType = 2 // LOOT_SLOT_TYPE_MASTER
					} else {
						slotType = 3 // LOOT_SLOT_TYPE_LOCKED
					}
				} else {
					if loot.RoundRobinPlayer != 0 && s.playerGUID != loot.RoundRobinPlayer {
						slotType = 3 // LOOT_SLOT_TYPE_LOCKED
					}
				}
			case 3, 4: // Group Loot / Need Before Greed
				if isOverThreshold {
					rollKey := lootRollKey{Object: loot.objectKey(), Slot: uint32(it.Slot)}
					s.server.lootMu.Lock()
					activeRoll := s.server.groupRolls[rollKey]
					s.server.lootMu.Unlock()
					if activeRoll != nil || it.IsBlocked {
						slotType = 1 // LOOT_SLOT_TYPE_ROLL_ONGOING
					} else if it.RollWinner != 0 {
						if it.RollWinner == s.playerGUID {
							slotType = 4 // LOOT_SLOT_TYPE_OWNER
						} else {
							slotType = 3 // LOOT_SLOT_TYPE_LOCKED
						}
					}
				} else {
					if loot.RoundRobinPlayer != 0 && s.playerGUID != loot.RoundRobinPlayer {
						slotType = 3 // LOOT_SLOT_TYPE_LOCKED
					}
				}
			}
		}
		packet.WriteU8(it.Slot)
		packet.WriteU32(it.ItemEntry)
		packet.WriteU32(it.Count)
		packet.WriteU32(it.DisplayInfoID)
		packet.WriteU32(0) // RandomPropertyId
		packet.WriteU32(0) // RandomSuffix
		packet.WriteU8(slotType)
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_LOOT_RESPONSE), packet.Bytes(), true); err != nil {
		return err
	}
	s.debug("loot response sent", "account", s.accountName, "target", loot.TargetGUID, "money", loot.Money, "items", len(loot.Items))

	if grp != nil {
		if grp.LootMethod == 2 { // Master Loot
			s.sendLootMasterList(loot)
		} else if grp.LootMethod == 3 || grp.LootMethod == 4 { // Group Loot / Need Before Greed
			for _, it := range sortedLootItems(loot.Items) {
				if it.Quality >= uint32(grp.LootThreshold) {
					s.server.startGroupLootRoll(loot.TargetGUID, uint32(it.Slot), it.ItemEntry, it.Count, loot.MapID, loot.InstanceID, s.groupID)
				}
			}
		}
	}
	return nil
}

func (s *session) sendLootMasterList(loot *activeLootState) {
	if s.server == nil || s.groupID == 0 {
		return
	}
	s.server.groupsMu.Lock()
	grp := s.server.groups[s.groupID]
	s.server.groupsMu.Unlock()
	if grp == nil || grp.LootMethod != 2 { // 2 = MASTER_LOOT
		return
	}
	members := s.server.getGroupSessions(s.groupID)
	var nearMembers []*session
	for _, m := range members {
		if m.player != nil && m.player.Map == s.player.Map && distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= 100.0 {
			nearMembers = append(nearMembers, m)
		}
	}
	buf := protocol.NewBuffer(1 + len(nearMembers)*8)
	buf.WriteU8(uint8(len(nearMembers)))
	for _, m := range nearMembers {
		buf.WriteU64(m.playerGUID)
	}
	for _, m := range nearMembers {
		_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_MASTER_LIST), buf.Bytes(), true)
	}
}

func (s *session) sendLootError(guid uint64, code uint8) error {
	packet := protocol.NewBuffer(10)
	packet.WriteU64(guid)
	packet.WriteU8(0)
	packet.WriteU8(code)
	return s.write(uint16(protocol.OpcodeSMSG_LOOT_RESPONSE), packet.Bytes(), true)
}

func (s *session) clearCreatureLoot(loot *activeLootState) {
	if loot == nil || s.server == nil {
		return
	}
	high := uint16(loot.TargetGUID >> 48)
	guids := []uint64{loot.TargetGUID}
	if high != 0xF110 {
		guid := uint32(loot.TargetGUID & 0x00FFFFFF)
		creatureEntry := uint32((loot.TargetGUID >> 24) & 0x00FFFFFF)
		guids = append(guids, creatureWorldGUID(guid, creatureEntry))
	}
	s.server.clearLootState(loot.MapID, loot.InstanceID, guids...)
	if high != 0xF110 {
		guid := uint32(loot.TargetGUID & 0x00FFFFFF)
		creatureEntry := uint32((loot.TargetGUID >> 24) & 0x00FFFFFF)
		stdGUID := creatureWorldGUID(guid, creatureEntry)
		s.server.broadcastCreatureValuesUpdateInInstance(loot.MapID, loot.InstanceID, loot.TargetGUID, map[int]uint32{unitFieldDynamicFlags: 0})
		s.server.broadcastCreatureValuesUpdateInInstance(loot.MapID, loot.InstanceID, stdGUID, map[int]uint32{unitFieldDynamicFlags: 0})
	}
}

func (s *Server) clearLootState(mapID, instanceID uint32, guids ...uint64) {
	if s == nil {
		return
	}
	s.lootMu.Lock()
	for _, guid := range guids {
		key := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: guid}
		delete(s.creatureLoot, key)
		delete(s.creatureLootOwners, key)
	}
	for key, roll := range s.groupRolls {
		if roll == nil || key.Object.MapID != mapID || key.Object.InstanceID != instanceID {
			continue
		}
		for _, guid := range guids {
			if key.Object.GUID == guid {
				if roll.Timer != nil {
					roll.Timer.Stop()
				}
				delete(s.groupRolls, key)
				break
			}
		}
	}
	s.lootMu.Unlock()
}

func (s *session) handleLootMoney(ctx context.Context) bool {
	if !s.playerLoaded || s.player == nil || s.activeLoot == nil || s.activeLoot.Money == 0 {
		return true
	}
	if s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
		return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
	}
	copper := s.activeLoot.Money
	s.activeLoot.Money = 0

	var nearMembers []*session
	if s.groupID != 0 && s.server != nil {
		allGroupSess := s.server.getGroupSessions(s.groupID)
		for _, m := range allGroupSess {
			if m.player != nil && m.player.Map == s.player.Map && m.player.InstanceID == s.player.InstanceID && distance3D(s.player.X, s.player.Y, s.player.Z, m.player.X, m.player.Y, m.player.Z) <= 100.0 {
				nearMembers = append(nearMembers, m)
			}
		}
	}

	if len(nearMembers) > 1 {
		copperPerPlayer := copper / uint32(len(nearMembers))
		for _, m := range nearMembers {
			m.player.Money += copperPerPlayer
			if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", m.player.Money, m.playerGUID)
			}
			notify := protocol.NewBuffer(5)
			notify.WriteU32(copperPerPlayer)
			notify.WriteU8(0) // 0 = "Your share is..."
			_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_MONEY_NOTIFY), notify.Bytes(), true)
			m.sendPlayerUpdate()
		}
	} else {
		s.player.Money += copper
		s.updateAchievementCriteria(criteriaTypeLootMoney, 0, copper)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET money = ? WHERE guid = ?", s.player.Money, s.playerGUID)
		}
		notify := protocol.NewBuffer(5)
		notify.WriteU32(copper)
		notify.WriteU8(1) // 1 = "You loot..."
		_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_MONEY_NOTIFY), notify.Bytes(), true)
		s.sendPlayerUpdate()
	}

	_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_CLEAR_MONEY), nil, true)
	if s.activeLoot.Money == 0 && len(s.activeLoot.Items) == 0 {
		s.clearCreatureLoot(s.activeLoot)
	}
	s.debug("loot money collected", "account", s.accountName, "copper", copper)
	return true
}

func (s *session) handleAutostoreLootItem(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.activeLoot == nil || len(payload) < 1 {
		return true
	}
	if s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
		return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
	}
	if s.isDeadOrGhost() {
		return true
	}
	lootSlot := payload[0]
	it, ok := s.activeLoot.Items[lootSlot]
	if !ok {
		return true
	}
	high := uint16(s.activeLoot.TargetGUID >> 48)
	if high == 0xF110 {
		if s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
			return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
		}
	} else {
		target, validTarget := s.getCombatTarget(ctx, s.activeLoot.TargetGUID)
		if !validTarget || target.Map != s.player.Map || target.InstanceID != s.player.InstanceID || target.Health != 0 || distance3D(s.player.X, s.player.Y, s.player.Z, target.X, target.Y, target.Z) > 5.0 {
			return s.sendLootError(s.activeLoot.TargetGUID, 4) == nil
		}
		guid := uint32(s.activeLoot.TargetGUID & 0x00FFFFFF)
		entry := uint32((s.activeLoot.TargetGUID >> 24) & 0x00FFFFFF)
		if !s.server.creatureLootAllowed(s.activeLoot.MapID, s.activeLoot.InstanceID, s.activeLoot.TargetGUID, creatureWorldGUID(guid, entry), s.playerGUID, s.groupID) {
			return s.sendLootError(s.activeLoot.TargetGUID, 0) == nil
		}
	}

	if s.server != nil && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil {
			isOverThreshold := it.Quality >= uint32(grp.LootThreshold)
			if grp.LootMethod == 2 && isOverThreshold {
				// Master loot item must be given by master looter
				return true
			}
			if (grp.LootMethod == 3 || grp.LootMethod == 4) && isOverThreshold {
				rollKey := lootRollKey{Object: s.activeLoot.objectKey(), Slot: uint32(it.Slot)}
				s.server.lootMu.Lock()
				activeRoll := s.server.groupRolls[rollKey]
				s.server.lootMu.Unlock()
				if activeRoll != nil || it.IsBlocked {
					return true
				}
				if it.RollWinner != 0 && it.RollWinner != s.playerGUID {
					return true
				}
			} else if grp.LootMethod != 0 {
				// Under threshold or round robin
				if s.activeLoot.RoundRobinPlayer != 0 && s.activeLoot.RoundRobinPlayer != s.playerGUID {
					return true
				}
			}
		}
	}

	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return true
	}
	res, err := s.storeOrStackItem(ctx, s.playerGUID, it.ItemEntry, it.Count)
	if err != nil {
		if errors.Is(err, errInventoryFull) {
			s.sendEquipError(equipErrInvFull, 0)
		}
		return true
	}
	s.updateAchievementCriteria(criteriaTypeLootItem, it.ItemEntry, it.Count)
	s.updateAchievementCriteria(criteriaTypeLootType, uint32(s.activeLoot.LootType), it.Count)
	if it.Quality >= 4 {
		s.updateAchievementCriteria(criteriaTypeLootEpicItem, it.ItemEntry, it.Count)
		s.updateAchievementCriteria(criteriaTypeReceiveEpicItem, it.ItemEntry, it.Count)
	}
	delete(s.activeLoot.Items, lootSlot)
	s.activeLoot.broadcastRemoved(lootSlot)
	_ = s.sendInventoryItems(ctx)

	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(s.playerGUID, res.ClientBag, slotForPush, it.ItemEntry, it.Count, res.InventoryCount), true)
	s.sendPlayerUpdate()
	if s.activeLoot.Money == 0 && len(s.activeLoot.Items) == 0 {
		s.clearCreatureLoot(s.activeLoot)
	}
	s.debug("loot item stored", "account", s.accountName, "item", it.ItemEntry, "slot", res.Slot, "bag", res.ClientBag, "stacked", res.IsStack)
	return true
}

func buildLootItemPushResult(playerGUID uint64, bag uint8, slot, entry, count, inventoryCount uint32) []byte {
	packet := protocol.NewBuffer(48)
	packet.WriteU64(playerGUID)
	packet.WriteU32(0)
	packet.WriteU32(0)
	packet.WriteU32(1)
	packet.WriteU8(bag)
	packet.WriteU32(slot)
	packet.WriteU32(entry)
	packet.WriteU32(0)
	packet.WriteI32(0)
	packet.WriteU32(count)
	packet.WriteU32(inventoryCount)
	return packet.Bytes()
}

func (s *session) handleLootRelease(payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	reader := protocol.NewReader(payload)
	targetGUID, err := reader.ReadPackedGUID()
	if err != nil {
		return false
	}
	if s.activeLoot == nil || s.activeLoot.TargetGUID != targetGUID {
		return true
	}
	loot := s.activeLoot
	if loot.MapID != s.player.Map || loot.InstanceID != s.player.InstanceID {
		return s.sendLootError(targetGUID, 4) == nil
	}
	releasedRoundRobin := loot.RoundRobinPlayer == s.playerGUID
	if releasedRoundRobin {
		loot.RoundRobinPlayer = 0
	}
	loot.removeViewer(s.playerGUID)
	s.activeLoot = nil
	if releasedRoundRobin && s.server != nil && s.groupID != 0 && uint16(loot.TargetGUID>>48) != 0xF110 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil {
			s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LOOT_LIST), buildLootLooterPacket(loot, grp))
		}
	}
	if loot.Money == 0 && len(loot.Items) == 0 {
		s.clearCreatureLoot(loot)
	}
	release := protocol.NewBuffer(9)
	release.WriteU64(targetGUID)
	release.WriteU8(1)
	_ = s.write(uint16(protocol.OpcodeSMSG_LOOT_RELEASE_RESPONSE), release.Bytes(), true)
	s.debug("loot released", "account", s.accountName, "target", targetGUID)
	return true
}

// handleLootMasterGive processes CMSG_LOOT_MASTER_GIVE (0x2A3).
// Reference: WorldSession::HandleLootMasterGiveOpcode (LootHandler.cpp:392).
func (s *session) handleLootMasterGive(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 17 {
		return true
	}
	r := protocol.NewReader(payload)
	lootGUID, _ := r.ReadU64()
	slotID, _ := r.ReadU8()
	targetGUID, _ := r.ReadU64()

	if s.server == nil || s.groupID == 0 {
		_ = s.sendLootError(lootGUID, 0) // LOOT_ERROR_DIDNT_KILL
		return true
	}
	s.server.groupsMu.Lock()
	grp := s.server.groups[s.groupID]
	s.server.groupsMu.Unlock()
	if grp == nil || grp.LootMethod != 2 || grp.MasterLooter != s.playerGUID {
		_ = s.sendLootError(lootGUID, 0) // LOOT_ERROR_DIDNT_KILL
		return true
	}

	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		_ = s.sendLootError(lootGUID, 10) // LOOT_ERROR_PLAYER_NOT_FOUND
		return true
	}
	if targetSess.player.Map != s.player.Map || targetSess.player.InstanceID != s.player.InstanceID || distance3D(s.player.X, s.player.Y, s.player.Z, targetSess.player.X, targetSess.player.Y, targetSess.player.Z) > 100.0 {
		_ = s.sendLootError(lootGUID, 14) // LOOT_ERROR_MASTER_OTHER
		return true
	}
	if s.activeLoot == nil || s.activeLoot.TargetGUID != lootGUID || s.activeLoot.MapID != s.player.Map || s.activeLoot.InstanceID != s.player.InstanceID {
		_ = s.sendLootError(lootGUID, 0)
		return true
	}
	it, ok := s.activeLoot.Items[slotID]
	if !ok {
		return true
	}

	res, err := targetSess.storeOrStackItem(ctx, targetGUID, it.ItemEntry, it.Count)
	if err != nil {
		_ = s.sendLootError(lootGUID, 12) // LOOT_ERROR_MASTER_INV_FULL
		return true
	}
	delete(s.activeLoot.Items, slotID)
	_ = targetSess.sendInventoryItems(ctx)
	targetSess.sendPlayerUpdate()
	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = targetSess.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(targetGUID, res.ClientBag, slotForPush, it.ItemEntry, it.Count, res.InventoryCount), true)

	s.activeLoot.broadcastRemoved(slotID)
	if s.activeLoot.Money == 0 && len(s.activeLoot.Items) == 0 {
		s.clearCreatureLoot(s.activeLoot)
	}
	return true
}

func buildLootRollPayload(itemGUID uint64, slot uint32, rollType uint8) []byte {
	buf := protocol.NewBuffer(13)
	buf.WriteU64(itemGUID)
	buf.WriteU32(slot)
	buf.WriteU8(rollType)
	return buf.Bytes()
}

func buildLootStartRollPacket(sourceGUID uint64, mapID, slot, itemEntry, randomSuffix, randomPropID, itemCount, countdown uint32, rollVoteMask uint8) []byte {
	buf := protocol.NewBuffer(8 + 4 + 4 + 4 + 4 + 4 + 4 + 4 + 1)
	buf.WriteU64(sourceGUID)
	buf.WriteU32(mapID)
	buf.WriteU32(slot)
	buf.WriteU32(itemEntry)
	buf.WriteU32(randomSuffix)
	buf.WriteU32(randomPropID)
	buf.WriteU32(itemCount)
	buf.WriteU32(countdown)
	buf.WriteU8(rollVoteMask)
	return buf.Bytes()
}

func (s *Server) startGroupLootRoll(sourceGUID uint64, slot uint32, itemEntry uint32, itemCount uint32, mapID, instanceID uint32, groupID uint64) {
	if groupID == 0 {
		return
	}
	members := s.getGroupSessions(groupID)
	if len(members) == 0 {
		return
	}
	var eligible []*session
	for _, m := range members {
		if m.player != nil && m.player.Map == mapID && m.player.InstanceID == instanceID {
			eligible = append(eligible, m)
		}
	}
	if len(eligible) == 0 {
		return
	}

	objectKey := lootObjectKey{MapID: mapID, InstanceID: instanceID, GUID: sourceGUID}
	rollKey := lootRollKey{Object: objectKey, Slot: slot}
	s.lootMu.Lock()
	if s.groupRolls == nil {
		s.groupRolls = make(map[lootRollKey]*activeGroupRoll)
	}
	if existing := s.groupRolls[rollKey]; existing != nil {
		s.lootMu.Unlock()
		return
	}

	s.groupsMu.Lock()
	grp := s.groups[groupID]
	s.groupsMu.Unlock()

	var disenchantID uint32
	var allowableClass uint32 = 0xFFFFFFFF
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		_ = s.WorldStore.DB.QueryRowContext(context.Background(), "SELECT DisenchantID, AllowableClass FROM item_template WHERE entry = ?", itemEntry).Scan(&disenchantID, &allowableClass)
	}

	baseMask := rollFlagTypePass | rollFlagTypeNeed | rollFlagTypeGreed
	if disenchantID > 0 {
		baseMask |= rollFlagTypeDisenchant
	}

	roll := &activeGroupRoll{
		SourceGUID:          sourceGUID,
		Slot:                slot,
		ItemEntry:           itemEntry,
		ItemCount:           itemCount,
		RollVoteMask:        baseMask,
		GroupID:             groupID,
		MapID:               mapID,
		InstanceID:          instanceID,
		EligiblePlayers:     make(map[uint64]struct{}, len(eligible)),
		StartedAt:           time.Now(),
		Duration:            60 * time.Second,
		TotalPlayersRolling: len(eligible),
		Votes:               make(map[uint64]uint8),
		Rolls:               make(map[uint64]uint8),
	}
	for _, m := range eligible {
		roll.EligiblePlayers[m.playerGUID] = struct{}{}
	}
	if cLoot := s.creatureLoot[objectKey]; cLoot != nil {
		if li, ok := cLoot.Items[uint8(slot)]; ok {
			li.IsBlocked = true
			cLoot.Items[uint8(slot)] = li
		}
	}
	s.groupRolls[rollKey] = roll
	s.lootMu.Unlock()

	// Send personalized SMSG_LOOT_START_ROLL to each eligible member (TC Group.cpp:971)
	for _, m := range eligible {
		memberMask := baseMask
		if grp != nil && grp.LootMethod == 4 { // Need Before Greed
			if allowableClass > 0 && allowableClass != 0xFFFFFFFF && m.player != nil && m.player.Class > 0 {
				playerClassMask := uint32(1 << (m.player.Class - 1))
				if (allowableClass & playerClassMask) == 0 {
					memberMask &= ^rollFlagTypeNeed // Ineligible to roll Need
				}
			}
		}

		buf := buildLootStartRollPacket(sourceGUID, mapID, slot, itemEntry, 0, 0, itemCount, 60000, memberMask)
		_ = m.write(uint16(protocol.OpcodeSMSG_LOOT_START_ROLL), buf, true)

		// Auto-pass check for players with PassOnGroupLoot
		if m.player != nil && m.player.PassOnGroupLoot {
			m.handleLootRoll(context.Background(), buildLootRollPayload(sourceGUID, slot, rollPass))
		}
	}

	// Arm 60s countdown
	roll.Timer = time.AfterFunc(60*time.Second, func() {
		s.resolveGroupLootRoll(rollKey)
	})
}

func (s *Server) resolveGroupLootRoll(rollKey lootRollKey) {
	s.lootMu.Lock()
	roll := s.groupRolls[rollKey]
	if roll == nil {
		s.lootMu.Unlock()
		return
	}
	delete(s.groupRolls, rollKey)
	if roll.Timer != nil {
		roll.Timer.Stop()
	}
	s.lootMu.Unlock()

	var winnerGUID uint64
	var maxRoll uint8
	var winningType uint8

	if roll.TotalNeed > 0 {
		for guid, vote := range roll.Votes {
			if vote == 1 { // NEED
				rNum := roll.Rolls[guid]
				if rNum == 0 {
					rNum = uint8(rand.Intn(100) + 1)
					roll.Rolls[guid] = rNum
					rBuf := protocol.NewBuffer(35)
					rBuf.WriteU64(roll.SourceGUID)
					rBuf.WriteU32(roll.Slot)
					rBuf.WriteU64(guid)
					rBuf.WriteU32(roll.ItemEntry)
					rBuf.WriteU32(roll.RandomSuffix)
					rBuf.WriteU32(roll.RandomPropID)
					rBuf.WriteU8(rNum)
					rBuf.WriteU8(1) // NEED
					rBuf.WriteU8(0)
					s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), rBuf.Bytes())
				}
				if rNum > maxRoll || winnerGUID == 0 {
					maxRoll = rNum
					winnerGUID = guid
					winningType = 1
				}
			}
		}
	} else if roll.TotalGreed > 0 {
		for guid, vote := range roll.Votes {
			if vote == 2 || vote == 3 { // GREED or DISENCHANT
				rNum := roll.Rolls[guid]
				if rNum == 0 {
					rNum = uint8(rand.Intn(100) + 1)
					roll.Rolls[guid] = rNum
					rBuf := protocol.NewBuffer(35)
					rBuf.WriteU64(roll.SourceGUID)
					rBuf.WriteU32(roll.Slot)
					rBuf.WriteU64(guid)
					rBuf.WriteU32(roll.ItemEntry)
					rBuf.WriteU32(roll.RandomSuffix)
					rBuf.WriteU32(roll.RandomPropID)
					rBuf.WriteU8(rNum)
					rBuf.WriteU8(vote)
					rBuf.WriteU8(0)
					s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), rBuf.Bytes())
				}
				if rNum > maxRoll || winnerGUID == 0 {
					maxRoll = rNum
					winnerGUID = guid
					winningType = vote
				}
			}
		}
	}

	if winnerGUID != 0 {
		wonBuf := protocol.NewBuffer(34)
		wonBuf.WriteU64(roll.SourceGUID)
		wonBuf.WriteU32(roll.Slot)
		wonBuf.WriteU32(roll.ItemEntry)
		wonBuf.WriteU32(roll.RandomSuffix)
		wonBuf.WriteU32(roll.RandomPropID)
		wonBuf.WriteU64(winnerGUID)
		wonBuf.WriteU8(maxRoll)
		wonBuf.WriteU8(winningType)
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL_WON), wonBuf.Bytes())

		// Reference Group.cpp:1557/1621: the roll winner is credited with the
		// winning roll value against the minimum-roll threshold.
		if winnerSess := s.findSessionByGUID(winnerGUID); winnerSess != nil {
			if winningType == 1 { // need
				winnerSess.setAchievementCriteria(criteriaTypeRollNeed, roll.ItemEntry, uint32(maxRoll))
			} else {
				winnerSess.setAchievementCriteria(criteriaTypeRollGreed, roll.ItemEntry, uint32(maxRoll))
			}
		}
		s.deliverGroupLootItem(roll, winnerGUID, winningType)
	} else {
		passBuf := protocol.NewBuffer(24)
		passBuf.WriteU64(roll.SourceGUID)
		passBuf.WriteU32(roll.Slot)
		passBuf.WriteU32(roll.ItemEntry)
		passBuf.WriteU32(roll.RandomPropID)
		passBuf.WriteU32(roll.RandomSuffix)
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_ALL_PASSED), passBuf.Bytes())

		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
	}
}

func (s *Server) deliverGroupLootItem(roll *activeGroupRoll, winnerGUID uint64, winningType uint8) {
	winnerSess := s.findSessionByGUID(winnerGUID)
	if winnerSess == nil || winnerSess.player == nil || winnerSess.player.Map != roll.MapID || winnerSess.player.InstanceID != roll.InstanceID || s.CharactersStore == nil || s.CharactersStore.DB == nil {
		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				li.RollWinner = winnerGUID
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
		return
	}
	ctx := context.Background()

	deliveredItem := roll.ItemEntry
	deliveredCount := roll.ItemCount

	// If won via Disenchant, deliver disenchanted material from disenchant_loot_template (TC Group.cpp:1655)
	if winningType == rollDisenchant && s.WorldStore != nil && s.WorldStore.DB != nil {
		var disenchantID uint32
		_ = s.WorldStore.DB.QueryRowContext(ctx, "SELECT DisenchantID FROM item_template WHERE entry = ?", roll.ItemEntry).Scan(&disenchantID)
		if disenchantID > 0 {
			var matItem, minCount, maxCount uint32
			err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT Item, MinCount, MaxCount FROM disenchant_loot_template WHERE Entry = ? ORDER BY Chance DESC LIMIT 1", disenchantID).Scan(&matItem, &minCount, &maxCount)
			if err == nil && matItem > 0 {
				deliveredItem = matItem
				deliveredCount = minCount
				if maxCount > minCount {
					deliveredCount += uint32(rand.Intn(int(maxCount - minCount + 1)))
				}
				if deliveredCount == 0 {
					deliveredCount = 1
				}
			}
		}
	}

	res, err := winnerSess.storeOrStackItem(ctx, winnerGUID, deliveredItem, deliveredCount)
	if err != nil {
		winnerSess.sendEquipError(equipErrInvFull, 0)
		s.lootMu.Lock()
		if cLoot := s.creatureLoot[rollLootObjectKey(roll)]; cLoot != nil {
			if li, ok := cLoot.Items[uint8(roll.Slot)]; ok {
				li.IsBlocked = false
				li.RollWinner = winnerGUID
				cLoot.Items[uint8(roll.Slot)] = li
			}
		}
		s.lootMu.Unlock()
		return
	}
	_ = winnerSess.sendInventoryItems(ctx)
	winnerSess.sendPlayerUpdate()
	slotForPush := uint32(res.Slot)
	if res.IsStack {
		slotForPush = 0xFFFFFFFF
	}
	_ = winnerSess.write(uint16(protocol.OpcodeSMSG_ITEM_PUSH_RESULT), buildLootItemPushResult(winnerGUID, res.ClientBag, slotForPush, deliveredItem, deliveredCount, res.InventoryCount), true)

	s.lootMu.Lock()
	cLoot := s.creatureLoot[rollLootObjectKey(roll)]
	if cLoot != nil {
		delete(cLoot.Items, uint8(roll.Slot))
	}
	s.lootMu.Unlock()

	if cLoot != nil {
		cLoot.broadcastRemoved(uint8(roll.Slot))
	} else {
		remBuf := protocol.NewBuffer(1)
		remBuf.WriteU8(uint8(roll.Slot))
		s.broadcastToGroup(roll.GroupID, uint16(protocol.OpcodeSMSG_LOOT_REMOVED), remBuf.Bytes())
	}
}

// handleLootRoll processes CMSG_LOOT_ROLL (0x2A0).
// Reference: WorldSession::HandleLootRoll (GroupHandler.cpp:470), Group::SendLootRoll (Group.cpp:995), Group::CountRollVote (Group.cpp:1452).
func (s *session) handleLootRoll(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 13 {
		return true
	}
	r := protocol.NewReader(payload)
	itemGUID, _ := r.ReadU64()
	itemSlot, _ := r.ReadU32()
	rollType, _ := r.ReadU8() // 0 = pass, 1 = need, 2 = greed, 3 = disenchant

	if s.server == nil || s.groupID == 0 {
		return true
	}

	rollKey := lootRollKey{Object: lootObjectKey{MapID: s.player.Map, InstanceID: s.player.InstanceID, GUID: itemGUID}, Slot: itemSlot}
	s.server.lootMu.Lock()
	roll := s.server.groupRolls[rollKey]
	if roll == nil {
		for key, other := range s.server.groupRolls {
			if other != nil && other.GroupID == s.groupID && key.Object.MapID == s.player.Map && key.Object.GUID == itemGUID && key.Slot == itemSlot && key.Object.InstanceID != s.player.InstanceID {
				s.server.lootMu.Unlock()
				return true
			}
		}
		var itemEntry uint32
		if loot := s.server.creatureLoot[rollKey.Object]; loot != nil {
			if li, ok := loot.Items[uint8(itemSlot)]; ok {
				itemEntry = li.ItemEntry
			}
		}
		s.server.lootMu.Unlock()

		rollNumber := uint8(128) // pass
		if rollType > 0 {
			rollNumber = uint8(rand.Intn(100) + 1) // 1..100
		}

		buf := protocol.NewBuffer(35)
		buf.WriteU64(itemGUID)     // sourceGuid (guid of loot object)
		buf.WriteU32(itemSlot)     // slot
		buf.WriteU64(s.playerGUID) // targetGuid (rolling player)
		buf.WriteU32(itemEntry)    // itemEntryId
		buf.WriteU32(0)            // randomSuffix
		buf.WriteU32(0)            // randomPropId
		buf.WriteU8(rollNumber)    // rollNumber
		buf.WriteU8(rollType)      // rollType (0: pass, 1: need, 2: greed, 3: disenchant)
		buf.WriteU8(0)             // autoPass
		s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), buf.Bytes())
		return true
	}
	if roll.GroupID != s.groupID || roll.MapID != s.player.Map || roll.InstanceID != s.player.InstanceID {
		s.server.lootMu.Unlock()
		return true
	}
	if _, eligible := roll.EligiblePlayers[s.playerGUID]; !eligible {
		s.server.lootMu.Unlock()
		return true
	}

	// If player already voted on this roll, ignore duplicate
	if _, voted := roll.Votes[s.playerGUID]; voted {
		s.server.lootMu.Unlock()
		return true
	}

	// In Need Before Greed, verify that player can need if they chose NEED (TC GroupHandler.cpp:487)
	if rollType == rollNeed && s.groupID != 0 {
		s.server.groupsMu.Lock()
		grp := s.server.groups[s.groupID]
		s.server.groupsMu.Unlock()
		if grp != nil && grp.LootMethod == 4 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			var allowableClass uint32 = 0xFFFFFFFF
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT AllowableClass FROM item_template WHERE entry = ?", roll.ItemEntry).Scan(&allowableClass)
			if allowableClass > 0 && allowableClass != 0xFFFFFFFF && s.player.Class > 0 {
				playerClassMask := uint32(1 << (s.player.Class - 1))
				if (allowableClass & playerClassMask) == 0 {
					rollType = rollPass // Ineligible to roll Need
				}
			}
		}
	}

	roll.Votes[s.playerGUID] = rollType

	switch rollType {
	case 0:
		roll.TotalPass++
	case 1:
		roll.TotalNeed++
		s.updateAchievementCriteria(criteriaTypeRollNeedCount, 0, 1)
	case 2:
		roll.TotalGreed++
		s.updateAchievementCriteria(criteriaTypeRollGreedCount, 0, 1)
	case 3:
		roll.TotalGreed++
		s.updateAchievementCriteria(criteriaTypeRollDisenchant, 0, 1)
	default:
		roll.TotalGreed++
		s.updateAchievementCriteria(criteriaTypeRollGreedCount, 0, 1)
	}

	itemEntry := roll.ItemEntry
	randomSuffix := roll.RandomSuffix
	randomPropID := roll.RandomPropID
	totalDone := (roll.TotalPass + roll.TotalNeed + roll.TotalGreed) >= roll.TotalPlayersRolling
	s.server.lootMu.Unlock()

	voteRollNumber := uint8(128)
	voteRollType := rollType
	if rollType == 1 { // NEED
		voteRollNumber = 0
		voteRollType = 0
	} else if rollType == 0 { // PASS
		voteRollNumber = 128
		voteRollType = 0
	}

	buf := protocol.NewBuffer(35)
	buf.WriteU64(itemGUID)
	buf.WriteU32(itemSlot)
	buf.WriteU64(s.playerGUID)
	buf.WriteU32(itemEntry)
	buf.WriteU32(randomSuffix)
	buf.WriteU32(randomPropID)
	buf.WriteU8(voteRollNumber)
	buf.WriteU8(voteRollType)
	buf.WriteU8(0) // autoPass
	s.server.broadcastToGroup(s.groupID, uint16(protocol.OpcodeSMSG_LOOT_ROLL), buf.Bytes())

	if totalDone {
		s.server.resolveGroupLootRoll(rollKey)
	}

	return true
}

// handleOptOutOfLoot processes CMSG_OPT_OUT_OF_LOOT (0x409).
// Reference: WorldSession::HandleOptOutOfLootOpcode (GroupHandler.cpp:1066).
func (s *session) handleOptOutOfLoot(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	passOnLoot, _ := r.ReadU32()
	s.player.PassOnGroupLoot = (passOnLoot != 0)
	return true
}

// onPlayerLeaveGroupRolls automatically records a pass vote for a player who leaves,
// gets kicked from, or disbands a group during active loot rolls, preventing stalled roll countdowns.
// Mirrors TrinityCore Group::CountRollVote and Group::RemoveMember (Group.cpp:780-810, 1450-1490).
func (s *Server) onPlayerLeaveGroupRolls(leavingGUID uint64, groupID uint64) {
	if s == nil || leavingGUID == 0 || groupID == 0 {
		return
	}
	s.lootMu.Lock()
	var keysToResolve []lootRollKey
	for key, roll := range s.groupRolls {
		if roll != nil && roll.GroupID == groupID {
			if _, eligible := roll.EligiblePlayers[leavingGUID]; !eligible {
				continue
			}
			if _, voted := roll.Votes[leavingGUID]; !voted {
				roll.Votes[leavingGUID] = rollPass
				roll.TotalPass++
			}
			if (roll.TotalPass + roll.TotalNeed + roll.TotalGreed) >= roll.TotalPlayersRolling {
				keysToResolve = append(keysToResolve, key)
			}
		}
	}
	s.lootMu.Unlock()

	for _, key := range keysToResolve {
		s.resolveGroupLootRoll(key)
	}
}
