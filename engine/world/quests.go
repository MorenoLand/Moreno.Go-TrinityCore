package world

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strconv"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	questStatusComplete   = 1
	questStatusIncomplete = 3
	questDialogNone       = 0
	questDialogIncomplete = 5
	questDialogReward     = 10
	questDialogAvailable  = 8
)

func (s *session) refreshQuestItemCounts(ctx context.Context, itemEntry uint32, added bool, questFilter ...uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	for slot, entry := range s.player.QuestLog {
		if entry.QuestID == 0 || added && entry.State != 0 || len(questFilter) > 0 && entry.QuestID != questFilter[0] {
			continue
		}
		quest, err := s.loadQuestQueryData(ctx, entry.QuestID)
		if err != nil {
			continue
		}
		changed := false
		for index, requiredItem := range quest.RequiredItemID {
			if requiredItem == 0 || itemEntry != 0 && requiredItem != itemEntry || quest.RequiredItemCount[index] == 0 {
				continue
			}
			var have int64
			if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci
				JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, requiredItem).Scan(&have); err != nil {
				continue
			}
			count := have
			if count > int64(quest.RequiredItemCount[index]) {
				count = int64(quest.RequiredItemCount[index])
			}
			if count < 0 {
				count = 0
			}
			if entry.ItemCounts[index] == uint16(count) {
				continue
			}
			entry.ItemCounts[index] = uint16(count)
			column := "itemcount" + strconv.Itoa(index+1)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET "+column+" = ? WHERE guid = ? AND quest = ?", entry.ItemCounts[index], s.playerGUID, entry.QuestID)
			changed = true
		}
		if !changed {
			continue
		}
		completed := len(questFilter) == 0 && s.questObjectivesComplete(ctx, entry.QuestID, entry)
		if entry.State == 0 && completed {
			entry.State = questCompleteStateFlag(questStatusComplete)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusComplete, s.playerGUID, entry.QuestID)
			_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_COMPLETE), nil, true)
			s.sendPlayerQuestLogUpdate(slot)
		} else if entry.State == questCompleteStateFlag(questStatusComplete) && !completed {
			entry.State = questCompleteStateFlag(questStatusIncomplete)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusIncomplete, s.playerGUID, entry.QuestID)
			s.sendPlayerQuestLogUpdate(slot)
		}
		s.player.QuestLog[slot] = entry
	}
}

func (s *session) handleQuestgiverHello(ctx context.Context, payload []byte) bool {
	return s.handleGossipHello(ctx, payload)
}

func (s *session) handleQuestgiverStatusQuery(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	guid, err := reader.ReadU64()
	if err != nil {
		return false
	}
	var entry uint32
	if creature := s.luaCreature(ctx, guid); creature != nil {
		entry = objectUint32OrZero(creature, "Entry")
	}
	if entry == 0 {
		entry = uint32((guid >> 24) & 0x00FFFFFF)
	}
	if entry == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rawGUID := guid & 0x00000000FFFFFFFF
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", rawGUID).Scan(&entry)
	}
	status := uint8(questDialogNone)
	if entry != 0 {
		if st, err := s.questDialogStatus(ctx, entry); err == nil {
			status = st
		}
	}
	packet := protocol.NewBuffer(9)
	packet.WriteU64(guid)
	packet.WriteU8(status)
	s.debug("quest status response", "account", s.accountName, "guid", guid, "entry", entry, "status", status)
	return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS), packet.Bytes(), true) == nil
}

func (s *session) isQuestRewarded(ctx context.Context, questID uint32) bool {
	cdb := s.server.CharactersStore.DB
	if cdb == nil {
		return false
	}
	var count int64
	err := cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus WHERE guid = ? AND quest = ? AND status = 2", s.playerGUID, questID).Scan(&count)
	if err == nil && count > 0 {
		return true
	}
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&count)
	return count > 0
}

func (s *session) canTakeQuest(ctx context.Context, questID uint32) (bool, error) {
	if s.player == nil {
		return false, nil
	}
	status, err := s.characterQuestStatus(ctx, questID)
	if err != nil {
		return false, err
	}
	if status != 0 {
		return false, nil
	}
	if s.isQuestRewarded(ctx, questID) {
		return false, nil
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return false, nil
	}
	var questFlags int64
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(Flags, 0) FROM quest_template WHERE ID = ?", questID).Scan(&questFlags)
	if questFlags&0x00008000 != 0 {
		var count int64
		if s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_weekly WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&count) == nil && count > 0 {
			return false, nil
		}
	}
	var monthlySpecialFlags int64
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&monthlySpecialFlags)
	if monthlySpecialFlags&0x00000010 != 0 {
		var count int64
		if s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_monthly WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&count) == nil && count > 0 {
			return false, nil
		}
	}

	// 1. Seasonal / World Event checks
	var eventEntry sql.NullInt64
	if err := wdb.QueryRowContext(ctx, "SELECT eventEntry FROM game_event_seasonal_questrelation WHERE questId = ?", questID).Scan(&eventEntry); err == nil && eventEntry.Valid && eventEntry.Int64 > 0 {
		var count int64
		if s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_seasonal WHERE guid = ? AND quest = ? AND event = ?", s.playerGUID, questID, eventEntry.Int64).Scan(&count) == nil && count > 0 {
			return false, nil
		}
		activeEvents := s.server.cachedActiveGameEvents(ctx)
		if _, active := activeEvents[eventEntry.Int64]; !active {
			return false, nil
		}
	}
	if err := wdb.QueryRowContext(ctx, "SELECT eventEntry FROM game_event_creature_quest WHERE quest = ?", questID).Scan(&eventEntry); err == nil && eventEntry.Valid && eventEntry.Int64 > 0 {
		activeEvents := s.server.cachedActiveGameEvents(ctx)
		if _, active := activeEvents[eventEntry.Int64]; !active {
			return false, nil
		}
	}

	var qLevel, minLvl, flags, allowableRaces int64
	var title string
	err = wdb.QueryRowContext(ctx, "SELECT QuestLevel, MinLevel, Flags, COALESCE(LogTitle, ''), AllowableRaces FROM quest_template WHERE ID = ?", questID).Scan(&qLevel, &minLvl, &flags, &title, &allowableRaces)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		err = wdb.QueryRowContext(ctx, "SELECT QuestLevel, MinLevel, Flags, COALESCE(LogTitle, '') FROM quest_template WHERE ID = ?", questID).Scan(&qLevel, &minLvl, &flags, &title)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
	}

	var prevQuest, maxLvl, allowableClasses, exclGroup, nextQuest, breadcrumb, specialFlags, addonRaces sql.NullInt64
	_ = wdb.QueryRowContext(ctx, `SELECT PrevQuestID, MaxLevel, AllowableClasses, ExclusiveGroup, NextQuestID, BreadcrumbForQuestId, SpecialFlags
		FROM quest_template_addon WHERE ID = ?`, questID).Scan(&prevQuest, &maxLvl, &allowableClasses, &exclGroup, &nextQuest, &breadcrumb, &specialFlags)
	if !allowableClasses.Valid && !prevQuest.Valid {
		_ = wdb.QueryRowContext(ctx, `SELECT PrevQuestID, MaxLevel, AllowableClasses, AllowableRaces, ExclusiveGroup, NextQuestID
			FROM quest_template_addon WHERE ID = ?`, questID).Scan(&prevQuest, &maxLvl, &allowableClasses, &addonRaces, &exclGroup, &nextQuest)
		if addonRaces.Valid && allowableRaces == 0 {
			allowableRaces = addonRaces.Int64
		}
	}

	// Min/Max Level check
	if minLvl > int64(s.player.Level) {
		return false, nil
	}
	if maxLvl.Valid && maxLvl.Int64 > 0 && int64(s.player.Level) > maxLvl.Int64 {
		return false, nil
	}

	// Class check (AllowableClasses bitmask: 1<<(class-1))
	if allowableClasses.Valid && allowableClasses.Int64 > 0 && s.player.Class > 0 {
		playerClassMask := int64(1 << uint(s.player.Class-1))
		if (allowableClasses.Int64 & playerClassMask) == 0 {
			return false, nil
		}
	}

	// Race check (AllowableRaces bitmask: 1<<(race-1))
	if allowableRaces > 0 && s.player.Race > 0 {
		playerRaceMask := int64(1 << uint(s.player.Race-1))
		if (allowableRaces & playerRaceMask) == 0 {
			return false, nil
		}
	}

	// PrevQuestID check (chain prerequisite)
	if prevQuest.Valid && prevQuest.Int64 != 0 {
		if prevQuest.Int64 > 0 {
			if !s.isQuestRewarded(ctx, uint32(prevQuest.Int64)) {
				return false, nil
			}
		} else {
			prevActiveStatus, _ := s.characterQuestStatus(ctx, uint32(-prevQuest.Int64))
			if prevActiveStatus != questStatusIncomplete && prevActiveStatus != questStatusComplete {
				return false, nil
			}
		}
	}

	// In TrinityCore, BreadcrumbForQuestId is on the breadcrumb quest to point
	// to the hub quest; the hub quest itself is not blocked by the breadcrumb.

	// ExclusiveGroup check
	if exclGroup.Valid && exclGroup.Int64 != 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var exclCount int64
		if exclGroup.Int64 > 0 {
			_ = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM character_queststatus_rewarded AS cqr
				JOIN quest_template_addon AS qta ON qta.ID = cqr.quest
				WHERE cqr.guid = ? AND qta.ExclusiveGroup = ?`, s.playerGUID, exclGroup.Int64).Scan(&exclCount)
			if exclCount > 0 {
				return false, nil
			}
		}
	}

	// ConditionMgr check (SourceType 19 & 20)
	meetsCond, err := s.meetQuestConditions(ctx, questID)
	if err != nil || !meetsCond {
		return false, nil
	}

	return true, nil
}

func (s *session) questDialogStatus(ctx context.Context, entry uint32) (uint8, error) {
	return s.questDialogStatusFromRelations(ctx, entry, "creature_questender", "creature_queststarter")
}

func (s *session) questDialogStatusFromRelations(ctx context.Context, entry uint32, enderTable, starterTable string) (uint8, error) {
	enderIDs, err := loadQuestRelationIDs(ctx, s.server.WorldStore.DB, enderTable, entry)
	if err != nil {
		return questDialogNone, err
	}
	for _, questID := range enderIDs {
		status, err := s.characterQuestStatus(ctx, questID)
		if err != nil {
			return questDialogNone, err
		}
		if status == questStatusIncomplete {
			if s.canCompleteQuest(ctx, questID) {
				s.completeQuest(ctx, questID)
				status = questStatusComplete
			}
		}
		if status == questStatusComplete {
			return questDialogReward, nil
		}
		if status == questStatusIncomplete {
			return questDialogIncomplete, nil
		}
	}
	if s.player == nil {
		return questDialogNone, nil
	}
	starterIDs, err := loadQuestRelationIDs(ctx, s.server.WorldStore.DB, starterTable, entry)
	if err != nil {
		return questDialogNone, err
	}
	for _, questID := range starterIDs {
		canTake, err := s.canTakeQuest(ctx, questID)
		if err != nil {
			return questDialogNone, err
		}
		if canTake {
			return questDialogAvailable, nil
		}
	}
	return questDialogNone, nil
}

func (s *session) characterQuestStatus(ctx context.Context, questID uint32) (int64, error) {
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var status int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT status FROM character_queststatus WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&status)
		if err == nil {
			return status, nil
		}
		if !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
			return 0, err
		}
	}
	if s.player != nil {
		for _, entry := range s.player.QuestLog {
			if entry.QuestID == questID {
				if entry.State != 0 {
					return questStatusComplete, nil
				}
				return questStatusIncomplete, nil
			}
		}
	}
	return 0, nil
}

func (s *session) loadCreatureQuestMenu(ctx context.Context, entry uint32, playerLevel uint8) ([]gossipQuestItem, error) {
	starterIDs, err := loadQuestRelationIDs(ctx, s.server.WorldStore.DB, "creature_queststarter", entry)
	if err != nil {
		return nil, err
	}
	enderIDs, err := loadQuestRelationIDs(ctx, s.server.WorldStore.DB, "creature_questender", entry)
	if err != nil {
		return nil, err
	}
	quests := make([]gossipQuestItem, 0, len(starterIDs)+len(enderIDs))
	seen := make(map[uint32]struct{})
	for _, questID := range enderIDs {
		status, err := s.characterQuestStatus(ctx, questID)
		if err != nil {
			return nil, err
		}
		if status == questStatusIncomplete {
			if s.canCompleteQuest(ctx, questID) {
				s.completeQuest(ctx, questID)
				status = questStatusComplete
			}
		}
		if status != questStatusComplete && status != questStatusIncomplete {
			continue
		}
		icon := uint32(4) // Question mark (complete / incomplete turn-in)
		quest, err := s.loadQuestMenuItem(ctx, questID, icon, playerLevel)
		if err != nil {
			return nil, err
		}
		if quest != nil {
			quests = append(quests, *quest)
			seen[questID] = struct{}{}
		}
	}
	for _, questID := range starterIDs {
		if _, exists := seen[questID]; exists {
			continue
		}
		canTake, err := s.canTakeQuest(ctx, questID)
		if err != nil || !canTake {
			continue
		}
		quest, err := s.loadQuestMenuItem(ctx, questID, 2, playerLevel)
		if err != nil {
			return nil, err
		}
		if quest != nil {
			quests = append(quests, *quest)
		}
	}
	return quests, nil
}

func (s *session) loadQuestMenuItem(ctx context.Context, questID, icon uint32, playerLevel uint8) (*gossipQuestItem, error) {
	var level, minLevel, flags int64
	var title sql.NullString
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT QuestLevel, MinLevel, Flags, COALESCE(LogTitle, '') FROM quest_template WHERE ID = ?", questID).Scan(&level, &minLevel, &flags, &title); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if minLevel > int64(playerLevel) {
		return nil, nil
	}
	return &gossipQuestItem{ID: questID, Icon: icon, Level: int32(level), Flags: uint32(flags), AutoComplete: uint32(flags)&questAutoCompleteFlags != 0, Title: title.String}, nil
}

func loadQuestRelationIDs(ctx context.Context, db *sql.DB, table string, entry uint32) ([]uint32, error) {
	rows, err := db.QueryContext(ctx, "SELECT quest FROM "+table+" WHERE id = ? ORDER BY quest", entry)
	if err != nil {
		if missingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	result := make([]uint32, 0)
	for rows.Next() {
		var questID int64
		if err := rows.Scan(&questID); err != nil {
			return nil, err
		}
		result = append(result, uint32(questID))
	}
	return result, rows.Err()
}

// handleQuestConfirmAccept processes CMSG_QUEST_CONFIRM_ACCEPT (0x19B).
// Reference: WorldSession::HandleQuestConfirmAccept (QuestHandler.cpp:655).
func (s *session) handleQuestConfirmAccept(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	questID, _ := r.ReadU32()

	buf := protocol.NewBuffer(12)
	buf.WriteU64(0)
	buf.WriteU32(questID)
	return s.handleQuestgiverAcceptQuest(ctx, buf.Bytes())
}

// handleQuestPoiQuery processes CMSG_QUEST_POI_QUERY (0x1E3).
// Reference: WorldSession::HandleQuestPOIQuery (QuestHandler.cpp:520).
func (s *session) handleQuestPoiQuery(ctx context.Context, payload []byte) bool {
	buf := protocol.NewBuffer(4)
	buf.WriteU32(0) // count = 0 POIs
	_ = s.write(uint16(protocol.OpcodeSMSG_QUEST_POI_QUERY_RESPONSE), buf.Bytes(), true)
	return true
}

// handleQueryQuestsCompleted processes CMSG_QUERY_QUESTS_COMPLETED (0x500).
// Reference: WorldSession::HandleQuestQueryCompletedQuests (QuestHandler.cpp:588).
func (s *session) handleQueryQuestsCompleted(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	var completed []uint32
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		rows, err := cdb.QueryContext(ctx, "SELECT quest FROM character_queststatus_rewarded WHERE guid = ?", s.playerGUID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var qID uint32
				if err := rows.Scan(&qID); err == nil {
					completed = append(completed, qID)
				}
			}
		}
	}
	buf := protocol.NewBuffer(4 + len(completed)*4)
	buf.WriteU32(uint32(len(completed)))
	for _, qID := range completed {
		buf.WriteU32(qID)
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_QUERY_QUESTS_COMPLETED_RESPONSE), buf.Bytes(), true)
	return true
}

// handleQuestlogSwapQuest processes CMSG_QUESTLOG_SWAP_QUEST (0x193).
// Reference: WorldSession::HandleQuestLogSwapQuest (QuestHandler.cpp:550).
func (s *session) handleQuestlogSwapQuest(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 2 {
		return true
	}
	slot1 := int(payload[0])
	slot2 := int(payload[1])
	if slot1 >= playerQuestLogSlots || slot2 >= playerQuestLogSlots || slot1 == slot2 {
		return true
	}
	s.player.QuestLog[slot1], s.player.QuestLog[slot2] = s.player.QuestLog[slot2], s.player.QuestLog[slot1]
	s.sendPlayerQuestLogUpdate(slot1)
	s.sendPlayerQuestLogUpdate(slot2)
	return true
}

const (
	QuestPartyMsgSharingQuest        = 0
	QuestPartyMsgCantTakeQuest       = 1
	QuestPartyMsgAcceptQuest         = 2
	QuestPartyMsgDeclineQuest        = 3
	QuestPartyMsgBusy                = 4
	QuestPartyMsgLogFull             = 5
	QuestPartyMsgHaveQuest           = 6
	QuestPartyMsgFinishQuest         = 7
	QuestPartyMsgCantBeSharedToday   = 8
	QuestPartyMsgSharingTimerExpired = 9
	QuestPartyMsgNotInParty          = 10
)

func (s *session) sendPushToPartyResponse(targetGUID uint64, msg uint8) {
	buf := protocol.NewBuffer(9)
	buf.WriteU64(targetGUID)
	buf.WriteU8(msg)
	_ = s.write(uint16(protocol.OpcodeMSG_QUEST_PUSH_RESULT), buf.Bytes(), true)
}

// handlePushQuestToParty processes CMSG_PUSHQUESTTOPARTY (0x199).
// Reference: WorldSession::HandlePushQuestToParty (QuestHandler.cpp:565).
func (s *session) handlePushQuestToParty(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	questID, _ := r.ReadU32()

	if s.groupID == 0 || s.server == nil {
		s.sendPushToPartyResponse(s.playerGUID, QuestPartyMsgNotInParty)
		return true
	}

	data, err := s.loadQuestDetailData(ctx, questID)
	if err != nil {
		return true
	}

	members := s.server.getGroupSessions(s.groupID)
	for _, receiver := range members {
		if receiver == nil || receiver == s || !receiver.playerLoaded || receiver.player == nil {
			continue
		}

		if receiver.isQuestRewarded(ctx, questID) {
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgFinishQuest)
			continue
		}

		st, _ := receiver.characterQuestStatus(ctx, questID)
		if st != 0 {
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgHaveQuest)
			continue
		}

		canTake, _ := receiver.canTakeQuest(ctx, questID)
		if !canTake {
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgCantTakeQuest)
			continue
		}

		// Check active quest log capacity
		activeCount := 0
		for _, q := range receiver.player.QuestLog {
			if q.QuestID != 0 {
				activeCount++
			}
		}
		if activeCount >= 25 {
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgLogFull)
			continue
		}

		if receiver.sharingQuestID != 0 {
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgBusy)
			continue
		}

		// Sharing initiated
		s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgSharingQuest)

		// Check AutoAccept
		var specialFlags int64
		if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
			_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT SpecialFlags FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
		}
		if specialFlags&4 != 0 {
			receiver.addQuestToPlayer(ctx, questID)
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgAcceptQuest)
		} else {
			receiver.sharingQuestID = questID
			receiver.sharingQuestSender = s.playerGUID
			details := buildQuestGiverDetails(data, s.playerGUID, 0)
			_ = receiver.write(uint16(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS), details, true)
		}
	}
	return true
}

// handleQuestPushResult processes MSG_QUEST_PUSH_RESULT (0x276).
// Reference: WorldSession::HandleQuestPushResult (QuestHandler.cpp:647).
func (s *session) handleQuestPushResult(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 13 {
		return true
	}
	r := protocol.NewReader(payload)
	sharerGUID, _ := r.ReadU64()
	questID, _ := r.ReadU32()
	msg, _ := r.ReadU8()

	if s.server != nil {
		sharerSess := s.server.findSessionByGUID(sharerGUID)
		if s.sharingQuestSender == sharerGUID && s.sharingQuestID == questID {
			if msg == QuestPartyMsgAcceptQuest {
				s.addQuestToPlayer(ctx, questID)
			}
			if sharerSess != nil {
				sharerSess.sendPushToPartyResponse(s.playerGUID, msg)
			}
			s.sharingQuestID = 0
			s.sharingQuestSender = 0
		} else if sharerSess != nil {
			sharerSess.sendPushToPartyResponse(s.playerGUID, msg)
		}
	}
	return true
}

// handleQuestgiverStatusMultipleQuery processes CMSG_QUESTGIVER_STATUS_MULTIPLE_QUERY (0x417).
// Reference: WorldSession::HandleQuestgiverStatusMultipleQuery (QuestHandler.cpp:45).
func (s *session) handleQuestgiverStatusMultipleQuery(ctx context.Context, payload []byte) bool {
	return s.sendQuestgiverStatusMultiple(ctx)
}

func (s *session) sendQuestgiverStatusMultiple(ctx context.Context) bool {
	if s == nil || s.server == nil || s.player == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE), []byte{0, 0, 0, 0}, true) == nil
	}
	distance := float64(s.server.Config.VisibilityDistanceContinents)
	if distance <= 0 {
		distance = 150
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0)
		FROM creature AS c JOIN creature_template AS t ON t.entry = c.id
		WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?
		AND (COALESCE(t.npcflag, 0) & 2) <> 0`, s.player.Map, float64(s.player.X)-distance, float64(s.player.X)+distance, float64(s.player.Y)-distance, float64(s.player.Y)+distance)
	if err != nil {
		return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE), []byte{0, 0, 0, 0}, true) == nil
	}
	type questgiver struct {
		guid         uint64
		entry        uint32
		faction      uint32
		enderTable   string
		starterTable string
	}
	questgivers := make([]questgiver, 0)
	for rows.Next() {
		var guid, entry, faction int64
		var x, y, z float64
		if rows.Scan(&guid, &entry, &x, &y, &z, &faction) != nil || guid <= 0 || entry <= 0 || math.Sqrt((x-float64(s.player.X))*(x-float64(s.player.X))+(y-float64(s.player.Y))*(y-float64(s.player.Y))+(z-float64(s.player.Z))*(z-float64(s.player.Z))) > distance {
			continue
		}
		questgivers = append(questgivers, questgiver{guid: creatureWorldGUID(uint32(guid), uint32(entry)), entry: uint32(entry), faction: uint32(faction), enderTable: "creature_questender", starterTable: "creature_queststarter"})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false
	}
	objectRows, objectErr := s.server.WorldStore.DB.QueryContext(ctx, `SELECT g.guid, g.id, g.position_x, g.position_y, g.position_z
		FROM gameobject AS g JOIN gameobject_template AS t ON t.entry = g.id
		WHERE g.map = ? AND g.position_x BETWEEN ? AND ? AND g.position_y BETWEEN ? AND ? AND t.type = 2
		ORDER BY g.guid`, s.player.Map, float64(s.player.X)-distance, float64(s.player.X)+distance, float64(s.player.Y)-distance, float64(s.player.Y)+distance)
	if objectErr == nil {
		for objectRows.Next() {
			var guid, entry int64
			var x, y, z float64
			if objectRows.Scan(&guid, &entry, &x, &y, &z) != nil || guid <= 0 || entry <= 0 || math.Sqrt((x-float64(s.player.X))*(x-float64(s.player.X))+(y-float64(s.player.Y))*(y-float64(s.player.Y))+(z-float64(s.player.Z))*(z-float64(s.player.Z))) > distance {
				continue
			}
			questgivers = append(questgivers, questgiver{guid: gameObjectGUID(uint32(guid), uint32(entry)), entry: uint32(entry), enderTable: "gameobject_questender", starterTable: "gameobject_queststarter"})
		}
		objectRows.Close()
	}
	player := playerPos{Map: s.player.Map, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	entries := make([]struct {
		guid   uint64
		status uint8
	}, 0, len(questgivers))
	for _, questgiver := range questgivers {
		if questgiver.faction != 0 && s.server.isHostileFaction(questgiver.faction, player) {
			continue
		}
		status, statusErr := s.questDialogStatusFromRelations(ctx, questgiver.entry, questgiver.enderTable, questgiver.starterTable)
		if statusErr != nil {
			continue
		}
		entries = append(entries, struct {
			guid   uint64
			status uint8
		}{questgiver.guid, status})
	}
	packet := protocol.NewBuffer(4 + len(entries)*9)
	packet.WriteU32(uint32(len(entries)))
	for _, entry := range entries {
		packet.WriteU64(entry.guid)
		packet.WriteU8(entry.status)
	}
	return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_STATUS_MULTIPLE), packet.Bytes(), true) == nil
}

func (s *session) completeQuest(ctx context.Context, questID uint32) {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusComplete, s.playerGUID, questID)
	}
	if s.player != nil {
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if s.player.QuestLog[slot].QuestID == questID {
				s.player.QuestLog[slot].State = questCompleteStateFlag(questStatusComplete)
				s.sendPlayerQuestLogUpdate(slot)
				break
			}
		}
	}
}

func (s *session) countPlayerInventoryItem(ctx context.Context, itemEntry uint32) uint32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var count sql.NullInt64
	_ = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT SUM(ii.count)
		FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, itemEntry).Scan(&count)
	if count.Valid && count.Int64 > 0 {
		return uint32(count.Int64)
	}
	return 0
}

func (s *session) canCompleteQuest(ctx context.Context, questID uint32) bool {
	if questID == 0 || s.player == nil {
		return false
	}
	if s.isQuestRewarded(ctx, questID) {
		return false
	}
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	wdb := s.server.WorldStore.DB

	// 1. Fetch quest requirements from quest_template
	var flags int64
	var reqNpc1, reqNpc2, reqNpc3, reqNpc4 int64
	var reqNpcCount1, reqNpcCount2, reqNpcCount3, reqNpcCount4 int64
	var reqItem1, reqItem2, reqItem3, reqItem4, reqItem5, reqItem6 int64
	var reqItemCount1, reqItemCount2, reqItemCount3, reqItemCount4, reqItemCount5, reqItemCount6 int64

	row := wdb.QueryRowContext(ctx, `SELECT Flags,
		RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4,
		RequiredNpcOrGoCount1, RequiredNpcOrGoCount2, RequiredNpcOrGoCount3, RequiredNpcOrGoCount4,
		RequiredItemId1, RequiredItemId2, RequiredItemId3, RequiredItemId4, RequiredItemId5, RequiredItemId6,
		RequiredItemCount1, RequiredItemCount2, RequiredItemCount3, RequiredItemCount4, RequiredItemCount5, RequiredItemCount6
		FROM quest_template WHERE ID = ?`, questID)
	err := row.Scan(&flags,
		&reqNpc1, &reqNpc2, &reqNpc3, &reqNpc4,
		&reqNpcCount1, &reqNpcCount2, &reqNpcCount3, &reqNpcCount4,
		&reqItem1, &reqItem2, &reqItem3, &reqItem4, &reqItem5, &reqItem6,
		&reqItemCount1, &reqItemCount2, &reqItemCount3, &reqItemCount4, &reqItemCount5, &reqItemCount6)
	if err != nil {
		// Fallback for minimalist quest_template schema
		var simpleFlags int64
		if err2 := wdb.QueryRowContext(ctx, "SELECT Flags FROM quest_template WHERE ID = ?", questID).Scan(&simpleFlags); err2 != nil {
			return false
		}
		flags = simpleFlags
	}

	if uint32(flags)&questAutoCompleteFlags != 0 {
		return true
	}

	// Check NPC/Creature/GO kill/cast counters against quest log slot
	var entryCounters [4]uint16
	foundSlot := false
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		if s.player.QuestLog[slot].QuestID == questID {
			entryCounters = s.player.QuestLog[slot].Counters
			foundSlot = true
			break
		}
	}
	if !foundSlot {
		return false
	}

	reqNpcs := [4]int64{reqNpc1, reqNpc2, reqNpc3, reqNpc4}
	reqNpcCounts := [4]int64{reqNpcCount1, reqNpcCount2, reqNpcCount3, reqNpcCount4}
	for i := 0; i < 4; i++ {
		if reqNpcs[i] != 0 && reqNpcCounts[i] > 0 {
			if int64(entryCounters[i]) < reqNpcCounts[i] {
				return false
			}
		}
	}

	// Check Item requirements in inventory
	reqItems := [6]int64{reqItem1, reqItem2, reqItem3, reqItem4, reqItem5, reqItem6}
	reqItemCounts := [6]int64{reqItemCount1, reqItemCount2, reqItemCount3, reqItemCount4, reqItemCount5, reqItemCount6}
	for i := 0; i < 6; i++ {
		if reqItems[i] != 0 && reqItemCounts[i] > 0 {
			count := s.countPlayerInventoryItem(ctx, uint32(reqItems[i]))
			if int64(count) < reqItemCounts[i] {
				return false
			}
		}
	}

	return true
}
