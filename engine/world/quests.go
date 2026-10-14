package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	questStatusComplete                   = 1
	questStatusIncomplete                 = 3
	questStatusFailed                     = 5      // QUEST_STATUS_FAILED (QuestDef.h:108)
	questStatusRewarded                   = 6      // Player::GetQuestStatus reports QUEST_STATUS_REWARDED, never NONE, for rewarded quests
	questFlagTracking              uint32 = 0x400  // QUEST_FLAGS_TRACKING (QuestDef.h:142): automatically rewarded on complete, never in the client quest log
	questSpecialExplorationOrEvent        = 0x002  // QUEST_SPECIAL_FLAGS_EXPLORATION_OR_EVENT (QuestDef.h:161)
	questSlotStateFail             uint32 = 0x0002 // QUEST_STATE_FAIL (Player.h:496)
	itemBondingQuestItem           uint32 = 4      // BIND_QUEST_ITEM (ItemTemplate.h:102)
	QuestTypeRaid                  uint32 = 62
	QuestTypeRaid10                uint32 = 88
	QuestTypeRaid25                uint32 = 89
	RaidDifficulty25ManMask        uint8  = 1
	questDialogNone                       = 0
	questDialogIncomplete                 = 5
	questDialogReward                     = 10
	questDialogAvailable                  = 8
)

func QuestAllowedInRaid(questType uint32, difficulty uint8, raidGroup, ignoreRaid bool) bool {
	if !raidGroup || ignoreRaid {
		return true
	}
	switch questType {
	case QuestTypeRaid:
		return true
	case QuestTypeRaid10:
		return difficulty&RaidDifficulty25ManMask == 0
	case QuestTypeRaid25:
		return difficulty&RaidDifficulty25ManMask != 0
	default:
		return false
	}
}

func (s *session) initializeQuestItemCounts(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	for slot, entry := range s.player.QuestLog {
		if entry.QuestID != questID || entry.State != 0 {
			continue
		}
		quest, err := s.loadQuestQueryData(ctx, entry.QuestID)
		if err != nil {
			continue
		}
		changed := false
		for index, requiredItem := range quest.RequiredItemID {
			if requiredItem == 0 || quest.RequiredItemCount[index] == 0 {
				continue
			}
			have, err := s.questInventoryItemCount(ctx, s.playerGUID, requiredItem, true)
			if err != nil {
				continue
			}
			if have > quest.RequiredItemCount[index] {
				have = quest.RequiredItemCount[index]
			}
			if entry.ItemCounts[index] == uint16(have) {
				continue
			}
			entry.ItemCounts[index] = uint16(have)
			column := "itemcount" + strconv.Itoa(index+1)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET "+column+" = ? WHERE guid = ? AND quest = ?", entry.ItemCounts[index], s.playerGUID, entry.QuestID)
			changed = true
		}
		if !changed {
			continue
		}
		s.player.QuestLog[slot] = entry
	}
}

func (s *session) questInventoryItemCount(ctx context.Context, guid uint64, itemEntry uint32, includeBank bool) (uint32, error) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, fmt.Errorf("characters database not available")
	}
	bankIncluded := 0
	if includeBank {
		bankIncluded = 1
	}
	var count int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory ci
		JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ? AND (
			(ci.bag = 0 AND ((ci.slot >= 0 AND ci.slot < ?) OR (ci.slot >= ? AND ci.slot < ?) OR (? = 1 AND ci.slot >= ? AND ci.slot < ?)))
			OR ci.bag IN (SELECT item FROM character_inventory WHERE guid = ? AND bag = 0 AND ((slot >= ? AND slot < ?) OR (? = 1 AND slot >= ? AND slot < ?)))
		)`, guid, itemEntry, invSlotItemEnd, invSlotKeyringStart, invSlotKeyringEnd, bankIncluded, bankSlotStart, bankBagSlotEnd, guid, invSlotBagStart, invSlotBagEnd, bankIncluded, bankBagSlotStart, bankBagSlotEnd).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, nil
	}
	if count > int64(^uint32(0)) {
		return ^uint32(0), nil
	}
	return uint32(count), nil
}

func (s *session) adjustQuestItemCount(ctx context.Context, itemEntry, count uint32, added bool) {
	if s != nil && s.player != nil {
		s.adjustQuestItemCountForState(ctx, s.player, itemEntry, count, added, true)
	}
}

func (s *session) adjustQuestItemCountForState(ctx context.Context, state *playerState, itemEntry, count uint32, added, notify bool) {
	if state == nil || itemEntry == 0 || count == 0 || s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	for slot, entry := range state.QuestLog {
		if entry.QuestID == 0 || added && entry.State != 0 {
			continue
		}
		quest, err := s.loadQuestQueryData(ctx, entry.QuestID)
		if err != nil {
			continue
		}
		changed, matched := false, false
		for index, requiredItem := range quest.RequiredItemID {
			if requiredItem != itemEntry || quest.RequiredItemCount[index] == 0 {
				continue
			}
			matched = true
			current := uint32(entry.ItemCounts[index])
			if !added && current >= quest.RequiredItemCount[index] {
				current, err = s.questInventoryItemCount(ctx, state.GUID, itemEntry, false)
				if err != nil {
					continue
				}
			}
			updated := QuestItemCountAfterDelta(entry.ItemCounts[index], current, quest.RequiredItemCount[index], count, added)
			if entry.ItemCounts[index] == updated {
				if !added {
					break
				}
				continue
			}
			entry.ItemCounts[index] = updated
			column := "itemcount" + strconv.Itoa(index+1)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET "+column+" = ? WHERE guid = ? AND quest = ?", entry.ItemCounts[index], state.GUID, entry.QuestID)
			changed = true
			if !added {
				break
			}
		}
		if !changed && !(added && matched) {
			continue
		}
		if added && notify && s.questObjectivesComplete(ctx, entry.QuestID, entry) {
			// Item-objective completion runs through
			// Player::CompleteQuest (Player.cpp:15292) — C++ sends no
			// SMSG_QUESTUPDATE_COMPLETE here (Player::SendQuestComplete
			// fires only from Player::AreaExploredOrEventHappens,
			// Player.cpp:16518), so none rides along. The local state
			// copy stays in sync so the trailing slot writeback below
			// does not clobber the flag completeQuest just set. This
			// arm only runs with state == s.player (adjustQuestItemCount
			// passes s.player; the other callers pass added=false).
			entry.State = questCompleteStateFlag(questStatusComplete)
			s.completeQuest(ctx, entry.QuestID)
		} else if !added && entry.State == questCompleteStateFlag(questStatusComplete) {
			entry.State = questCompleteStateFlag(questStatusIncomplete)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusIncomplete, state.GUID, entry.QuestID)
			if notify {
				s.sendPlayerQuestLogUpdate(slot)
			}
		}
		state.QuestLog[slot] = entry
	}
}

func QuestItemCountAfterDelta(current uint16, inventoryCount, required, count uint32, added bool) uint16 {
	value := uint64(current)
	if !added && value >= uint64(required) {
		value = uint64(inventoryCount)
	}
	if added {
		if value >= uint64(required) {
			return current
		}
		value += uint64(count)
	} else if uint64(count) >= value {
		value = 0
	} else {
		value -= uint64(count)
	}
	if value > uint64(required) {
		value = uint64(required)
	}
	if value > uint64(^uint16(0)) {
		value = uint64(^uint16(0))
	}
	return uint16(value)
}

func (s *session) handleQuestgiverHello(ctx context.Context, payload []byte) bool {
	return s.helloCreatureNPC(ctx, payload, npcFlagQuestgiver, true)
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
	if entry == 0 {
		// TrinityCore HandleQuestgiverStatusQueryOpcode (QuestHandler.cpp:41)
		// sends no packet when the questgiver object does not exist.
		return true
	}
	status := uint8(questDialogNone)
	// WorldSession::HandleQuestgiverStatusQueryOpcode (QuestHandler.cpp:41-56):
	// a creature hostile to the player keeps DIALOG_STATUS_NONE —
	// GetQuestDialogStatus (and the Eluna OnDialogStatus hook inside it) never
	// runs for it. The gate applies to units only; gameobjects skip it,
	// mirroring C++.
	hostile := false
	if s.player != nil && uint16(guid>>48) == 0xF130 && entry != 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var faction int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(faction, 0) FROM creature_template WHERE entry = ?", entry).Scan(&faction); err == nil && faction != 0 {
			player := playerPos{Race: s.player.Race, Class: s.player.Class, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
			hostile = s.server.isHostileFaction(uint32(faction), player)
		}
	}
	if !hostile {
		// Eluna CREATURE_EVENT_ON_DIALOG_STATUS (event 35), fired from
		// Player::GetQuestDialogStatus before the AI/relation checks
		// (Player.cpp:16290-16296).
		s.fireCreatureQuestHook(ctx, guid, scripting.CreatureEventOnDialogStatus)
		s.fireGameObjectQuestHook(ctx, guid, scripting.GameObjectEventOnDialogStatus)
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
	_ = cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ? AND active = 1", s.playerGUID, questID).Scan(&count)
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
	var reqSkillID, reqSkillPoints, reqMinRepFaction, reqMinRepValue, reqMaxRepFaction, reqMaxRepValue sql.NullInt64
	_ = wdb.QueryRowContext(ctx, `SELECT PrevQuestID, MaxLevel, AllowableClasses, ExclusiveGroup, NextQuestID, BreadcrumbForQuestId, SpecialFlags,
		RequiredSkillID, RequiredSkillPoints, RequiredMinRepFaction, RequiredMinRepValue, RequiredMaxRepFaction, RequiredMaxRepValue
		FROM quest_template_addon WHERE ID = ?`, questID).Scan(&prevQuest, &maxLvl, &allowableClasses, &exclGroup, &nextQuest, &breadcrumb, &specialFlags,
		&reqSkillID, &reqSkillPoints, &reqMinRepFaction, &reqMinRepValue, &reqMaxRepFaction, &reqMaxRepValue)
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

	// Player::SatisfyQuestSkill (Player.cpp:15595): the non-pure skill value
	// (GetSkillValue twin: playerSkillTotalValue) must reach RequiredSkillPoints.
	if reqSkillID.Valid && reqSkillPoints.Valid && reqSkillID.Int64 > 0 {
		if s.playerSkillTotalValue(uint32(reqSkillID.Int64)) < int32(reqSkillPoints.Int64) {
			return false, nil
		}
	}

	// Player::SatisfyQuestReputation (Player.cpp:15830): min/max faction gates
	// plus the RepObjectiveFaction2 accept gate (rep already at/above its
	// objective value blocks re-accept).
	if reqMinRepFaction.Valid && reqMinRepFaction.Int64 > 0 && reqMinRepValue.Valid {
		if s.reputationStanding(uint32(reqMinRepFaction.Int64)) < int32(reqMinRepValue.Int64) {
			return false, nil
		}
	}
	if reqMaxRepFaction.Valid && reqMaxRepFaction.Int64 > 0 && reqMaxRepValue.Valid {
		if s.reputationStanding(uint32(reqMaxRepFaction.Int64)) >= int32(reqMaxRepValue.Int64) {
			return false, nil
		}
	}
	var repObjFaction2, repObjValue2 sql.NullInt64
	if wdb.QueryRowContext(ctx, "SELECT COALESCE(RequiredFactionId2, 0), COALESCE(RequiredFactionValue2, 0) FROM quest_template WHERE ID = ?", questID).Scan(&repObjFaction2, &repObjValue2) == nil &&
		repObjFaction2.Valid && repObjFaction2.Int64 > 0 && repObjValue2.Valid {
		if s.reputationStanding(uint32(repObjFaction2.Int64)) >= int32(repObjValue2.Int64) {
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
			// Player::SatisfyQuestPreviousQuest (Player.cpp:15665): a negative
			// PrevQuestID requires the previous quest ACTIVE-and-incomplete;
			// complete-but-unrewarded does not satisfy it.
			prevActiveStatus, _ := s.characterQuestStatus(ctx, uint32(-prevQuest.Int64))
			if prevActiveStatus != questStatusIncomplete {
				return false, nil
			}
		}
	}

	// Player::SatisfyQuestBreadcrumbQuest (Player.cpp:15750): a breadcrumb
	// quest is only available when its target hub quest is itself available.
	// Recursive with msg=false, mirroring the C++.
	if breadcrumb.Valid && breadcrumb.Int64 != 0 {
		target := breadcrumb.Int64
		if target < 0 {
			target = -target
		}
		if ok, err := s.canTakeQuest(ctx, uint32(target)); err != nil || !ok {
			return false, err
		}
	}

	// Player::SatisfyQuestDependentBreadcrumbQuests (Player.cpp:15770): the hub
	// quest is blocked while any breadcrumb quest pointing at it sits in the
	// quest log (incomplete, complete, or failed).
	if wdb != nil {
		depRows, derr := wdb.QueryContext(ctx, "SELECT ID FROM quest_template_addon WHERE BreadcrumbForQuestId = ?", questID)
		if derr == nil {
			for depRows.Next() {
				var depID int64
				if derr = depRows.Scan(&depID); derr != nil {
					break
				}
				depStatus, serr := s.characterQuestStatus(ctx, uint32(depID))
				if serr != nil {
					depRows.Close()
					return false, serr
				}
				if depStatus != 0 {
					depRows.Close()
					return false, nil
				}
			}
			derr = depRows.Close()
			if derr != nil {
				return false, derr
			}
		}
	}

	// Player::SatisfyQuestDependentPreviousQuests (Player.cpp:15692): when
	// quests chain into this one via NextQuestId, one chain head must be
	// completed; a head in a non-positive exclusive group satisfies only when
	// every other quest in that group is completed too (each-from-all arm),
	// while a head in a non-negative group satisfies on its own
	// (one-from-all arm).
	if wdb != nil {
		chainRows, cerr := wdb.QueryContext(ctx, "SELECT ID FROM quest_template_addon WHERE NextQuestID = ?", questID)
		if cerr == nil {
			chainOK, haveChain, chainErr := false, false, error(nil)
			for chainRows.Next() {
				var chainID int64
				if chainErr = chainRows.Scan(&chainID); chainErr != nil {
					break
				}
				haveChain = true
				if !s.isQuestRewarded(ctx, uint32(chainID)) {
					continue
				}
				var chainGroup int64
				_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(ExclusiveGroup, 0) FROM quest_template_addon WHERE ID = ?", chainID).Scan(&chainGroup)
				if chainGroup >= 0 {
					chainOK = true
					break
				}
				groupOK := true
				grpRows, gerr := wdb.QueryContext(ctx, "SELECT ID FROM quest_template_addon WHERE ExclusiveGroup = ? AND ID != ?", chainGroup, chainID)
				if gerr != nil {
					chainErr = gerr
					break
				}
				for grpRows.Next() {
					var grpID int64
					if gerr = grpRows.Scan(&grpID); gerr != nil {
						break
					}
					if !s.isQuestRewarded(ctx, uint32(grpID)) {
						groupOK = false
						break
					}
				}
				grpRows.Close()
				if gerr != nil {
					chainErr = gerr
					break
				}
				if groupOK {
					chainOK = true
					break
				}
			}
			if cerr = chainRows.Close(); cerr == nil {
				cerr = chainErr
			}
			if cerr != nil {
				return false, cerr
			}
			if haveChain && !chainOK {
				return false, nil
			}
		}
	}

	// Player::SatisfyQuestExclusiveGroup (Player.cpp:15930): only positive
	// groups gate. For each other quest in the group: its daily/weekly/
	// seasonal repeat gates apply (msg=false), it must not be in the quest
	// log, and it must not be rewarded unless both quests are repeatable
	// (QUEST_SPECIAL_FLAGS_REPEATABLE, QuestDef.h:160).
	if exclGroup.Valid && exclGroup.Int64 > 0 {
		var selfSpecial int64
		_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&selfSpecial)
		repeatableSelf := selfSpecial&0x001 != 0
		grpRows, gerr := wdb.QueryContext(ctx, `SELECT qta.ID, COALESCE(qta.SpecialFlags, 0), COALESCE(qt.Flags, 0)
			FROM quest_template_addon qta JOIN quest_template qt ON qt.ID = qta.ID
			WHERE qta.ExclusiveGroup = ? AND qta.ID != ?`, exclGroup.Int64, questID)
		if gerr != nil {
			return false, gerr
		}
		for grpRows.Next() {
			var memberID, memberSpecial, memberFlags int64
			if gerr = grpRows.Scan(&memberID, &memberSpecial, &memberFlags); gerr != nil {
				break
			}
			if !s.satisfyQuestRepeatGates(ctx, uint32(memberID), memberFlags) {
				grpRows.Close()
				return false, nil
			}
			memberStatus, merr := s.characterQuestStatus(ctx, uint32(memberID))
			if merr != nil {
				grpRows.Close()
				return false, merr
			}
			if memberStatus != 0 {
				grpRows.Close()
				return false, nil
			}
			if s.isQuestRewarded(ctx, uint32(memberID)) && !(repeatableSelf && memberSpecial&0x001 != 0) {
				grpRows.Close()
				return false, nil
			}
		}
		grpRows.Close()
		if gerr != nil {
			return false, gerr
		}
	}

	// Player::SatisfyQuestTimed (Player.cpp:15917): only one timed quest at a
	// time — a timed quest (QUEST_SPECIAL_FLAGS_TIMED, QuestDef.h:173) is
	// unavailable while any other timed quest is in the quest log.
	var timedSpecial int64
	if wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&timedSpecial) == nil &&
		timedSpecial&0x400 != 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var timedCount int64
		if s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM character_queststatus cq
			JOIN quest_template_addon qta ON qta.ID = cq.quest
			WHERE cq.guid = ? AND (COALESCE(qta.SpecialFlags, 0) & 1024) != 0`, s.playerGUID).Scan(&timedCount) == nil && timedCount > 0 {
			return false, nil
		}
	}

	// ConditionMgr check (SourceType 19 & 20)
	meetsCond, err := s.meetQuestConditions(ctx, questID)
	if err != nil || !meetsCond {
		return false, nil
	}

	return true, nil
}

// questHighLevelHideDiff mirrors CONFIG_QUEST_HIGH_LEVEL_HIDE_DIFF
// ("Quests.HighLevelHideDiff", World.cpp:1129): Go has no world-config model
// for it, so the C++ default of 7 is used.
const questHighLevelHideDiff = 7

// canSeeStartQuest mirrors Player::CanSeeStartQuest (Player.cpp:14913-14924),
// the cheat gate's first disjunct in HandleQuestgiverChooseRewardOpcode
// (QuestHandler.cpp:299): an auto-complete quest the player never accepted
// (status NONE) still has to be see-startable. It is CanTakeQuest minus the
// status, timed-quest and ConditionMgr arms, with the MinLevel gate replaced
// by the high-level hide-diff rule (player level + 7 >= MinLevel). The
// Satisfy* blocks are the same twins canTakeQuest uses; the breadcrumb arm
// recurses into CanTakeQuest exactly like the C++ (Player.cpp:15750).
// DisableMgr has no Go model (same delta as canTakeQuest).
func (s *session) canSeeStartQuest(ctx context.Context, questID uint32) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return false
	}
	wdb := s.server.WorldStore.DB
	var minLvl, questFlags, allowableRaces int64
	if err := wdb.QueryRowContext(ctx, "SELECT MinLevel, Flags, AllowableRaces FROM quest_template WHERE ID = ?", questID).Scan(&minLvl, &questFlags, &allowableRaces); err != nil {
		return false
	}
	var prevQuest, maxLvl, allowableClasses, exclGroup, breadcrumb, specialFlags, addonRaces sql.NullInt64
	var reqSkillID, reqSkillPoints, reqMinRepFaction, reqMinRepValue, reqMaxRepFaction, reqMaxRepValue sql.NullInt64
	_ = wdb.QueryRowContext(ctx, `SELECT PrevQuestID, MaxLevel, AllowableClasses, ExclusiveGroup, BreadcrumbForQuestId, SpecialFlags,
		RequiredSkillID, RequiredSkillPoints, RequiredMinRepFaction, RequiredMinRepValue, RequiredMaxRepFaction, RequiredMaxRepValue
		FROM quest_template_addon WHERE ID = ?`, questID).Scan(&prevQuest, &maxLvl, &allowableClasses, &exclGroup, &breadcrumb, &specialFlags,
		&reqSkillID, &reqSkillPoints, &reqMinRepFaction, &reqMinRepValue, &reqMaxRepFaction, &reqMaxRepValue)
	if !allowableClasses.Valid && !prevQuest.Valid {
		_ = wdb.QueryRowContext(ctx, "SELECT AllowableRaces FROM quest_template_addon WHERE ID = ?", questID).Scan(&addonRaces)
		if addonRaces.Valid && allowableRaces == 0 {
			allowableRaces = addonRaces.Int64
		}
	}
	// Player.cpp:14921 — the hide-diff rule, not the SatisfyQuestLevel gate.
	if int64(s.player.Level)+questHighLevelHideDiff < minLvl {
		return false
	}
	if allowableClasses.Valid && allowableClasses.Int64 > 0 && s.player.Class > 0 {
		if (allowableClasses.Int64 & int64(1<<uint(s.player.Class-1))) == 0 {
			return false
		}
	}
	if allowableRaces > 0 && s.player.Race > 0 {
		if (allowableRaces & int64(1<<uint(s.player.Race-1))) == 0 {
			return false
		}
	}
	if reqSkillID.Valid && reqSkillPoints.Valid && reqSkillID.Int64 > 0 {
		if s.playerSkillTotalValue(uint32(reqSkillID.Int64)) < int32(reqSkillPoints.Int64) {
			return false
		}
	}
	if reqMinRepFaction.Valid && reqMinRepFaction.Int64 > 0 && reqMinRepValue.Valid {
		if s.reputationStanding(uint32(reqMinRepFaction.Int64)) < int32(reqMinRepValue.Int64) {
			return false
		}
	}
	if reqMaxRepFaction.Valid && reqMaxRepFaction.Int64 > 0 && reqMaxRepValue.Valid {
		if s.reputationStanding(uint32(reqMaxRepFaction.Int64)) >= int32(reqMaxRepValue.Int64) {
			return false
		}
	}
	if prevQuest.Valid && prevQuest.Int64 != 0 {
		if prevQuest.Int64 > 0 {
			if !s.isQuestRewarded(ctx, uint32(prevQuest.Int64)) {
				return false
			}
		} else if prevStatus, _ := s.characterQuestStatus(ctx, uint32(-prevQuest.Int64)); prevStatus != questStatusIncomplete {
			return false
		}
	}
	if breadcrumb.Valid && breadcrumb.Int64 != 0 {
		target := breadcrumb.Int64
		if target < 0 {
			target = -target
		}
		if ok, err := s.canTakeQuest(ctx, uint32(target)); err != nil || !ok {
			return false
		}
	}
	if wdb != nil {
		depRows, derr := wdb.QueryContext(ctx, "SELECT ID FROM quest_template_addon WHERE BreadcrumbForQuestId = ?", questID)
		if derr == nil {
			blocked := false
			for depRows.Next() {
				var depID int64
				if derr = depRows.Scan(&depID); derr != nil {
					break
				}
				if depStatus, serr := s.characterQuestStatus(ctx, uint32(depID)); serr != nil {
					derr = serr
					break
				} else if depStatus != 0 {
					blocked = true
					break
				}
			}
			cerr := depRows.Close()
			if derr != nil || cerr != nil || blocked {
				return false
			}
		}
	}
	if wdb != nil {
		chainRows, cerr := wdb.QueryContext(ctx, "SELECT ID FROM quest_template_addon WHERE NextQuestID = ?", questID)
		if cerr == nil {
			chainOK, haveChain := false, false
			var chainErr error
			for chainRows.Next() {
				var chainID int64
				if chainErr = chainRows.Scan(&chainID); chainErr != nil {
					break
				}
				haveChain = true
				if !s.isQuestRewarded(ctx, uint32(chainID)) {
					continue
				}
				var chainGroup int64
				_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(ExclusiveGroup, 0) FROM quest_template_addon WHERE ID = ?", chainID).Scan(&chainGroup)
				if chainGroup >= 0 {
					chainOK = true
					break
				}
				groupOK := true
				grpRows, gerr := wdb.QueryContext(ctx, "SELECT ID FROM quest_template_addon WHERE ExclusiveGroup = ? AND ID != ?", chainGroup, chainID)
				if gerr != nil {
					chainErr = gerr
					break
				}
				for grpRows.Next() {
					var grpID int64
					if gerr = grpRows.Scan(&grpID); gerr != nil {
						break
					}
					if !s.isQuestRewarded(ctx, uint32(grpID)) {
						groupOK = false
						break
					}
				}
				grpRows.Close()
				if gerr != nil {
					chainErr = gerr
					break
				}
				if groupOK {
					chainOK = true
					break
				}
			}
			if cerr = chainRows.Close(); cerr == nil {
				cerr = chainErr
			}
			if cerr != nil || (haveChain && !chainOK) {
				return false
			}
		}
	}
	if exclGroup.Valid && exclGroup.Int64 > 0 {
		var selfSpecial int64
		_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&selfSpecial)
		repeatableSelf := selfSpecial&0x001 != 0
		grpRows, gerr := wdb.QueryContext(ctx, `SELECT qta.ID, COALESCE(qta.SpecialFlags, 0), COALESCE(qt.Flags, 0)
			FROM quest_template_addon qta JOIN quest_template qt ON qt.ID = qta.ID
			WHERE qta.ExclusiveGroup = ? AND qta.ID != ?`, exclGroup.Int64, questID)
		if gerr != nil {
			return false
		}
		blocked := false
		for grpRows.Next() {
			var memberID, memberSpecial, memberFlags int64
			if gerr = grpRows.Scan(&memberID, &memberSpecial, &memberFlags); gerr != nil {
				break
			}
			if !s.satisfyQuestRepeatGates(ctx, uint32(memberID), memberFlags) {
				blocked = true
				break
			}
			memberStatus, merr := s.characterQuestStatus(ctx, uint32(memberID))
			if merr != nil {
				gerr = merr
				break
			}
			if memberStatus != 0 {
				blocked = true
				break
			}
			if s.isQuestRewarded(ctx, uint32(memberID)) && !(repeatableSelf && memberSpecial&0x001 != 0) {
				blocked = true
				break
			}
		}
		grpRows.Close()
		if gerr != nil || blocked {
			return false
		}
	}
	// Player::SatisfyQuestDay/Week/Month/Seasonal (Player.cpp:15980+),
	// msg=false arms: the repeat-cooldown state plus the seasonal
	// game-event-active gate. DisableMgr and the dungeon-finder daily arm
	// have no Go model (documented deltas, same as canTakeQuest).
	if !s.satisfyQuestRepeatGates(ctx, questID, questFlags) {
		return false
	}
	if specialFlags.Valid && specialFlags.Int64&0x00000010 != 0 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var count int64
		if s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_monthly WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&count) == nil && count > 0 {
			return false
		}
	}
	var eventEntry sql.NullInt64
	if err := wdb.QueryRowContext(ctx, "SELECT eventEntry FROM game_event_seasonal_questrelation WHERE questId = ?", questID).Scan(&eventEntry); err == nil && eventEntry.Valid && eventEntry.Int64 > 0 {
		if _, active := s.server.cachedActiveGameEvents(ctx)[eventEntry.Int64]; !active {
			return false
		}
	}
	if err := wdb.QueryRowContext(ctx, "SELECT eventEntry FROM game_event_creature_quest WHERE quest = ?", questID).Scan(&eventEntry); err == nil && eventEntry.Valid && eventEntry.Int64 > 0 {
		if _, active := s.server.cachedActiveGameEvents(ctx)[eventEntry.Int64]; !active {
			return false
		}
	}
	return true
}

// satisfyQuestRepeatGates mirrors the msg=false arms of
// Player::SatisfyQuestDay/Week/Seasonal (Player.cpp:15954+) as consumed by
// Player::SatisfyQuestExclusiveGroup: the repeat-cooldown state of another
// quest in the exclusive group blocks taking this quest. The dungeon-finder
// daily arm has no Go model (documented delta).
func (s *session) satisfyQuestRepeatGates(ctx context.Context, questID uint32, questFlags int64) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	cdb := s.server.CharactersStore.DB
	// Day: QUEST_FLAGS_DAILY (QuestDef.h:144), tracked in
	// character_queststatus_daily; PLAYER_MAX_DAILY_QUESTS = 25 caps the day.
	if questFlags&0x1000 != 0 {
		var done, total int64
		if cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_daily WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&done) == nil && done > 0 {
			return false
		}
		if cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_daily WHERE guid = ?", s.playerGUID).Scan(&total) == nil && total >= 25 {
			return false
		}
	}
	// Week: QUEST_FLAGS_WEEKLY (QuestDef.h:147).
	if questFlags&0x8000 != 0 {
		var done int64
		if cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_weekly WHERE guid = ? AND quest = ?", s.playerGUID, questID).Scan(&done) == nil && done > 0 {
			return false
		}
	}
	// Seasonal: game_event_seasonal_questrelation.
	if wdb := s.server.WorldStore.DB; wdb != nil {
		var eventEntry sql.NullInt64
		if wdb.QueryRowContext(ctx, "SELECT eventEntry FROM game_event_seasonal_questrelation WHERE questId = ?", questID).Scan(&eventEntry) == nil && eventEntry.Valid && eventEntry.Int64 > 0 {
			var done int64
			if cdb.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_queststatus_seasonal WHERE guid = ? AND quest = ? AND event = ?", s.playerGUID, questID, eventEntry.Int64).Scan(&done) == nil && done > 0 {
				return false
			}
		}
	}
	return true
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
			if s.isQuestRewarded(ctx, questID) {
				continue
			}
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
	// Player::GetQuestStatus (Player.cpp:16131-16143): a quest missing from
	// the active status map but present in the rewarded set reports
	// QUEST_STATUS_REWARDED, not NONE. The rewarded-row predicate
	// (quest_reward_grant.go state.Rewarded) already mirrors
	// Quest::CanIncreaseRewardedQuestCounters, so the repeatable/daily/DF
	// exclusions from GetQuestRewardStatus hold without a template read.
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var rewarded int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COUNT(1) FROM character_queststatus_rewarded WHERE guid = ? AND quest = ? AND active = 1", s.playerGUID, questID).Scan(&rewarded); err == nil && rewarded > 0 {
			return questStatusRewarded, nil
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
	return s.buildQuestMenu(ctx, starterIDs, enderIDs, playerLevel)
}

// loadGameObjectQuestMenu mirrors the gameobject arm of
// Player::PrepareQuestMenu (Player.cpp:14748): the quest relations come
// from gameobject_queststarter/ender (ObjectMgr.cpp:8411), the menu items
// otherwise build exactly like the creature menu.
func (s *session) loadGameObjectQuestMenu(ctx context.Context, entry uint32, playerLevel uint8) ([]gossipQuestItem, error) {
	starterIDs, err := loadQuestRelationIDs(ctx, s.server.WorldStore.DB, "gameobject_queststarter", entry)
	if err != nil {
		return nil, err
	}
	enderIDs, err := loadQuestRelationIDs(ctx, s.server.WorldStore.DB, "gameobject_questender", entry)
	if err != nil {
		return nil, err
	}
	return s.buildQuestMenu(ctx, starterIDs, enderIDs, playerLevel)
}

func (s *session) buildQuestMenu(ctx context.Context, starterIDs, enderIDs []uint32, playerLevel uint8) ([]gossipQuestItem, error) {
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
	// PlayerMenu::SendGossipMenu (GossipDef.cpp:230): the per-quest byte is
	// IsAutoComplete() && IsRepeatable() && !IsDailyOrWeekly() && !IsMonthly()
	// — not the bare AUTOCOMPLETE flag: IsAutoComplete (QuestDef.cpp:315)
	// also counts Method == 0, and repeatable + daily/weekly/monthly arms
	// from the quest_template_addon SpecialFlags still apply.
	var method, specialFlags int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(Method, 0) FROM quest_template WHERE ID = ?", questID).Scan(&method)
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	}
	isAutoComplete := method == 0 || uint32(flags)&questAutoCompleteFlags != 0
	autoCompleteByte := isAutoComplete &&
		specialFlags&questSpecialRepeatable != 0 &&
		uint32(flags)&(questFlagsDaily|questFlagsWeekly) == 0 &&
		specialFlags&questSpecialMonthly == 0
	return &gossipQuestItem{ID: questID, Icon: icon, Level: int32(level), Flags: uint32(flags), AutoComplete: autoCompleteByte, Title: title.String}, nil
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
// Reference: WorldSession::HandleQuestConfirmAccept (QuestHandler.cpp:474).
// Unlike the normal accept path, the quest giver is nil here: the quest comes
// from a party share, so no quest-accept hooks fire (Player::
// AddQuestAndCheckCompletion returns before the giver switch when the giver
// is nil, Player.cpp:15105-15122). Every early return below leaves the sharing
// info set, matching C++; only the template-missing fallthrough and the
// completed path clear it.
func (s *session) handleQuestConfirmAccept(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	questID, _ := r.ReadU32()

	data, err := s.loadQuestDetailData(ctx, questID)
	if err != nil {
		s.sharingQuestID = 0
		s.sharingQuestSender = 0
		return true
	}
	if data.Flags&questFlagsPartyAccept == 0 {
		return true
	}
	if s.sharingQuestSender == 0 || s.server == nil {
		return true
	}
	sharer := s.server.findSessionByGUID(s.sharingQuestSender)
	if sharer == nil || sharer.player == nil {
		return true
	}
	// Player::IsInSameRaidWith — Go has no raid/party distinction, so same
	// group is the whole check.
	if s.groupID == 0 || sharer.groupID != s.groupID {
		return true
	}
	if st, _ := sharer.characterQuestStatus(ctx, questID); st == 0 {
		return true
	}
	if canTake, err := s.canTakeQuest(ctx, questID); err != nil || !canTake {
		return true
	}
	// QuestHandler.cpp:497-499 — the share-accept path runs the full
	// CanAddQuest(msg=true) gate, not just the log-capacity check: a quest
	// whose source item can't be stored is refused with the C++ equip error
	// instead of being added with the item silently dropped.
	if s.canAddQuest(ctx, questID, true) {
		s.addQuestToPlayer(ctx, questID, 0)
		// QuestHandler.cpp:501-503 — the quest's source spell is cast triggered.
		s.castQuestSourceSpell(ctx, questID)
	}
	s.sharingQuestID = 0
	s.sharingQuestSender = 0
	return true
}

// handleQuestPoiQuery processes CMSG_QUEST_POI_QUERY (0x1E3).
// Reference: WorldSession::HandleQuestPOIQuery (QueryHandler.cpp:321-363).
func (s *session) handleQuestPoiQuery(ctx context.Context, payload []byte) bool {
	r := protocol.NewReader(payload)
	count, err := r.ReadU32()
	if err != nil {
		return false
	}
	// QueryHandler.cpp:323-324: more than MAX_QUEST_LOG_SIZE (25) quest ids is a
	// silent drop — no packet at all.
	if count > playerQuestLogSlots {
		return true
	}
	// QueryHandler.cpp:328-330: duplicate quest ids are collapsed into a set.
	seen := make(map[uint32]struct{}, count)
	questIDs := make([]uint32, 0, count)
	for i := uint32(0); i < count; i++ {
		questID, err := r.ReadU32()
		if err != nil {
			return false
		}
		if _, dup := seen[questID]; dup {
			continue
		}
		seen[questID] = struct{}{}
		questIDs = append(questIDs, questID)
	}

	buf := protocol.NewBuffer(4 + len(questIDs)*8)
	buf.WriteU32(uint32(len(questIDs)))
	for _, questID := range questIDs {
		// QueryHandler.cpp:332-356 echoes every quest id; quests with POI data
		// append the QuestPOIWrapper buffer. Go has no quest_poi model (no
		// loader for quest_poi/quest_poi_points), so every quest takes the
		// C++ no-wrapper arm: quest id + zero POI count (documented delta).
		buf.WriteU32(questID)
		buf.WriteU32(0)
	}
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
	QuestPartyMsgNotEligibleToday    = 11 // QUEST_PARTY_MSG_NOT_ELIGIBLE_TODAY (QuestDef.h:79), distinct from 8
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

	// Player::CanShareQuest (Player.cpp:16145): the quest must carry
	// QUEST_FLAGS_SHARABLE and the sender must have it active. (The
	// pool-availability arm has no Go model.)
	if data.Flags&questFlagsSharable == 0 {
		return true
	}
	if st, _ := s.characterQuestStatus(ctx, questID); st == 0 {
		return true
	}

	members := s.server.getGroupSessions(s.groupID)

	// QuestHandler.cpp:610-613 — auto-complete quests are offered the
	// RequestItems screen instead of the share prompt:
	// (IsAutoComplete && IsRepeatable && !IsDailyOrWeekly) ||
	// QUEST_FLAGS_AUTOCOMPLETE, where IsAutoComplete = Method == 0 ||
	// AUTOCOMPLETE flag (QuestDef.cpp:315), IsRepeatable = SpecialFlags & 1.
	var method, specialFlags int64
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(Method, 0) FROM quest_template WHERE ID = ?", questID).Scan(&method)
		_ = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	}
	isAutoComplete := method == 0 || data.Flags&questAutoCompleteFlags != 0
	isRepeatable := specialFlags&questSpecialRepeatable != 0
	autoCompleteShare := (isAutoComplete && isRepeatable && data.Flags&(questFlagsDaily|questFlagsWeekly) == 0) || data.Flags&questAutoCompleteFlags != 0

	for _, receiver := range members {
		if receiver == nil || receiver == s || !receiver.worldReady.Load() || receiver.player == nil {
			continue
		}

		st, _ := receiver.characterQuestStatus(ctx, questID)
		if st != 0 || receiver.isQuestRewarded(ctx, questID) {
			// Player::SatisfyQuestStatus(quest, false) (Player.cpp:15878)
			// rejects REWARDED and any non-NONE status with
			// QUEST_PARTY_MSG_HAVE_QUEST; the QUEST_STATUS_COMPLETE ->
			// QUEST_PARTY_MSG_FINISH_QUEST arm below it is unreachable for
			// the same reason and is kept only for parity.
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgHaveQuest)
			continue
		}
		if st == questStatusComplete {
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgFinishQuest)
			continue
		}

		// Player::SatisfyQuestDay (Player.cpp:15980-16008): daily and
		// dungeon-finder quests answer QUEST_PARTY_MSG_NOT_ELIGIBLE_TODAY
		// before the CanTakeQuest gate. A daily fails when the receiver
		// already completed it today OR the daily roster is full
		// (PLAYER_MAX_DAILY_QUESTS = 25, no free slot); a dungeon-finder
		// quest fails only when already done today. The roster lives in
		// the receiver's session state, kept current at every daily
		// turn-in, exactly like C++'s PLAYER_FIELD_DAILY_QUESTS slots.
		if isDFQuest := specialFlags&0x08 != 0; data.Flags&questFlagsDaily != 0 || isDFQuest {
			eligible := true
			if isDFQuest {
				_, done := receiver.player.DungeonFinderQuests[questID]
				eligible = !done
			} else {
				haveSlot := false
				for _, id := range receiver.player.DailyQuests {
					if id == questID {
						eligible = false
						break
					}
					if id == 0 {
						haveSlot = true
					}
				}
				if eligible && !haveSlot {
					eligible = false
				}
			}
			if !eligible {
				s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgNotEligibleToday)
				continue
			}
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

		if autoCompleteShare {
			// QuestHandler.cpp:610-613 — auto-complete quests skip the share
			// prompt: the receiver gets the RequestItems screen with the
			// repeatable-completion state (CanCompleteRepeatableQuest).
			if view, err := s.loadQuestRewardView(ctx, questID); err == nil {
				receiver.sendQuestRequestItems(view, s.playerGUID, receiver.canCompleteRepeatableQuest(ctx, questID, view, isRepeatable), true)
			}
			continue
		}

		// QuestHandler.cpp:636 — the details packet's InformUnit field carries the
		// receiver's sharing info (the sharer's GUID, set two lines above), not
		// zero: PlayerMenu::SendQuestGiverQuestDetails reads InformUnit from
		// GetPlayer()->GetPlayerSharingQuest() (GossipDef.cpp:411). The packet
		// carries the receiver's own guid as the quest-giver npc.
		receiver.sharingQuestID = questID
		receiver.sharingQuestSender = s.playerGUID
		details := buildQuestGiverDetails(data, receiver.playerGUID, s.playerGUID)
		_ = receiver.write(uint16(protocol.OpcodeSMSG_QUEST_GIVER_QUEST_DETAILS), details, true)
		// QuestHandler.cpp:618-622 + 637-638 — the auto-accept arm: the gate
		// is QUEST_FLAGS_AUTO_ACCEPT (0x00080000, QuestDef.h:151), not the
		// quest_template_addon SpecialFlags (0x4 there is the monthly flag).
		// The arm re-runs CanAddQuest(msg=true): the source-item storability
		// check was never done for shared auto-accepts. The CanTakeQuest
		// re-check is subsumed by the gate above (no receiver quest state
		// changes between the two). AddQuestAndCheckCompletion takes the
		// sharer as the giver, so the timed-quest shared-giver arm
		// (Player.cpp:15253) inherits the sharer's remaining time instead of
		// starting a fresh TimeAllowed. After the add the sharer is answered
		// ACCEPT_QUEST and the receiver's sharing info is cleared.
		if data.Flags&questFlagsAutoAccept != 0 && receiver.canAddQuest(ctx, questID, true) {
			receiver.addQuestToPlayer(ctx, questID, s.playerGUID)
			s.sendPushToPartyResponse(receiver.playerGUID, QuestPartyMsgAcceptQuest)
			receiver.sharingQuestID = 0
			receiver.sharingQuestSender = 0
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
	_, _ = r.ReadU32()
	msg, _ := r.ReadU8()

	// QuestHandler.cpp:647-665 — MSG_QUEST_PUSH_RESULT only relays the client's
	// accept/decline message to the sharer; the quest itself is added by
	// CMSG_QUEST_CONFIRM_ACCEPT with its full gate set (party-accept flag, same
	// raid, sharer still active, CanTakeQuest). Go previously added the quest
	// here with no gates at all, so the sharer could grant quests to players
	// outside the raid, players already rewarded, or quests that fail
	// CanTakeQuest — and the follow-up confirm-accept then no-op'd on the
	// already-cleared sharing info.
	if s.sharingQuestSender == 0 {
		return true
	}
	if s.sharingQuestSender == sharerGUID {
		if s.server != nil {
			if sharerSess := s.server.findSessionByGUID(sharerGUID); sharerSess != nil {
				sharerSess.sendPushToPartyResponse(s.playerGUID, msg)
			}
		}
		s.sharingQuestID = 0
		s.sharingQuestSender = 0
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
	player := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.effectiveFactionTemplate(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	entries := make([]struct {
		guid   uint64
		status uint8
	}, 0, len(questgivers))
	for _, questgiver := range questgivers {
		if questgiver.faction != 0 && s.server.isHostileFaction(questgiver.faction, player) {
			continue
		}
		// Eluna CREATURE_EVENT_ON_DIALOG_STATUS (event 35), fired per
		// creature questgiver in the multiple-status sweep
		// (Player.cpp:17160-17176). Gameobject GUIDs are skipped by the
		// hook's TYPEID_UNIT gate.
		s.fireCreatureQuestHook(ctx, questgiver.guid, scripting.CreatureEventOnDialogStatus)
		s.fireGameObjectQuestHook(ctx, questgiver.guid, scripting.GameObjectEventOnDialogStatus)
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

// sendQuestComplete mirrors Player::SendQuestComplete
// (Player.cpp:17003-17009): SMSG_QUESTUPDATE_COMPLETE carries the quest id
// (uint32).
func (s *session) sendQuestComplete(questID uint32) {
	packet := protocol.NewBuffer(4)
	packet.WriteU32(questID)
	_ = s.write(uint16(protocol.OpcodeSMSG_QUESTUPDATE_COMPLETE), packet.Bytes(), true)
}

// sendQuestUpdate mirrors Player::SendQuestUpdate (Player.cpp:16221-16267):
// every spell_area row keyed by the quest (quest_start or quest_end) is
// re-evaluated at the player's current zone/area. Rows whose requirements
// no longer fit land on the remove list; fitting autocast rows land on the
// cast list, and a spell present on both lists is kept (C++ erases cast
// spells from the remove set before applying it). Casts fire only when the
// player lacks the aura (the C++ HasAura gate) via the triggered
// CastSpell(this, spellId, true) arm; removals are RemoveAurasDueToSpell.
// The closing UpdateVisibleGameobjectsOrSpellClicks sweep has no Go bridge
// (standing no-bridge, group_lua.go documents it).
func (s *session) sendQuestUpdate(ctx context.Context, questID uint32) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || s.player == nil || s.server.Data == nil {
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT spell, area, quest_start, quest_end, aura_spell, racemask, gender, autocast, quest_start_status, quest_end_status FROM spell_area WHERE quest_start = ? OR quest_end = ?`, questID, questID)
	if err != nil {
		return
	}
	defer rows.Close()
	toRemove := map[uint32]bool{}
	toCast := map[uint32]bool{}
	zoneID, areaID := s.player.Zone, s.areaID
	for rows.Next() {
		var spellID uint32
		var rule spellAreaRule
		var autocast int64
		if rows.Scan(&spellID, &rule.Area, &rule.QuestStart, &rule.QuestEnd, &rule.AuraSpell, &rule.RaceMask, &rule.Gender, &autocast, &rule.QuestStartStatus, &rule.QuestEndStatus) != nil || spellID == 0 {
			continue
		}
		rule.Autocast = autocast != 0
		fits := s.spellAreaRuleFits(ctx, rule, zoneID, areaID)
		if fits && spellAreaExtraConditionSpell(spellID) && !s.spellAreaExtraConditionFits(spellID, zoneID, areaID) {
			fits = false
		}
		if !fits {
			toRemove[spellID] = true
		} else if rule.Autocast {
			toCast[spellID] = true
		}
	}
	for spellID := range toCast {
		delete(toRemove, spellID)
		if !s.hasAura(spellID) {
			if _, found, spellErr := s.server.Data.Spell(spellID); spellErr == nil && found {
				s.castSpellDirect(ctx, spellID, s.playerGUID)
			}
		}
	}
	for spellID := range toRemove {
		s.removeAreaRestrictedAura(ctx, spellID)
	}
}

func (s *session) completeQuest(ctx context.Context, questID uint32) {
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusComplete, s.playerGUID, questID)
	}
	// Player::CompleteQuest (Player.cpp:15297): the COMPLETE transition
	// runs through Player::SetQuestStatus (update=true) — SendQuestUpdate
	// first (Player.cpp:16179), then the OnQuestStatusChange hook.
	s.sendQuestUpdate(ctx, questID)
	s.triggerPlayerEvent(ctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), questID)
	if s.player != nil {
		for slot := 0; slot < playerQuestLogSlots; slot++ {
			if s.player.QuestLog[slot].QuestID == questID {
				s.player.QuestLog[slot].State = questCompleteStateFlag(questStatusComplete)
				s.sendPlayerQuestLogUpdate(slot)
				break
			}
		}
	}
	// Player::CompleteQuest (Player.cpp:15304-15307): TRACKING-flagged
	// quests are rewarded automatically on completion — RewardQuest(qInfo,
	// 0, this, false) — and never appear in the client quest log.
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var flags int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(Flags, 0) FROM quest_template WHERE ID = ?", questID).Scan(&flags); err == nil && flags&int64(questFlagTracking) != 0 {
			s.rewardTrackingQuest(ctx, questID)
		}
	}
}

// incompleteQuest mirrors Player::IncompleteQuest (Player.cpp:15320-15329):
// the quest drops back to INCOMPLETE and the QUEST_STATE_COMPLETE log flag
// is cleared. Like SetQuestStatus, the client quest update goes out.
func (s *session) incompleteQuest(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusIncomplete, s.playerGUID, questID)
	// Player::IncompleteQuest (Player.cpp:15324): the INCOMPLETE transition
	// runs through Player::SetQuestStatus (update=true) — SendQuestUpdate
	// first (Player.cpp:16179), then the OnQuestStatusChange hook.
	s.sendQuestUpdate(ctx, questID)
	s.triggerPlayerEvent(ctx, scripting.PlayerEventQuestStatusChange, s.luaPlayer(), questID)
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		if s.player.QuestLog[slot].QuestID == questID {
			s.player.QuestLog[slot].State &^= questCompleteStateFlag(questStatusComplete)
			s.sendPlayerQuestLogUpdate(slot)
			break
		}
	}
}

// completeQuestFromSpell mirrors Spell::EffectQuestComplete
// (SpellEffects.cpp:4371-4389): the quest template must exist
// (SpellEffects.cpp:4380-4382), and the completion only lands when the
// quest sits in the target's log (AreaExploredOrEventHappens arm — Go:
// completeQuest) or, without a log entry, when the quest carries
// QUEST_FLAGS_TRACKING (SetRewardedQuest arm). Anything else is a no-op.
func (s *session) completeQuestFromSpell(ctx context.Context, questID uint32) {
	if s == nil || s.player == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	var flags int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Flags FROM quest_template WHERE ID = ?", questID).Scan(&flags); err != nil {
		return
	}
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		if s.player.QuestLog[slot].QuestID == questID {
			s.completeQuest(ctx, questID)
			return
		}
	}
	if flags&int64(questFlagTracking) != 0 {
		s.markQuestRewarded(ctx, questID)
	}
}

// markQuestRewarded mirrors Player::SetRewardedQuest (Player.cpp): the quest
// lands in the rewarded set without any client broadcast — the serverside
// flag arm of EffectQuestComplete for QUEST_FLAGS_TRACKING quests.
func (s *session) markQuestRewarded(ctx context.Context, questID uint32) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	insertIgnore := "INSERT OR IGNORE"
	if s.server.CharactersStore.Backend != database.BackendSQLite {
		insertIgnore = "INSERT IGNORE"
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx, insertIgnore+" INTO character_queststatus_rewarded (guid, quest, active) VALUES (?, ?, 1)", s.playerGUID, questID)
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
	var reqMoney, repFaction, repValue, requiredPlayerKills int64

	row := wdb.QueryRowContext(ctx, `SELECT Flags,
		RequiredNpcOrGo1, RequiredNpcOrGo2, RequiredNpcOrGo3, RequiredNpcOrGo4,
		RequiredNpcOrGoCount1, RequiredNpcOrGoCount2, RequiredNpcOrGoCount3, RequiredNpcOrGoCount4,
		RequiredItemId1, RequiredItemId2, RequiredItemId3, RequiredItemId4, RequiredItemId5, RequiredItemId6,
		RequiredItemCount1, RequiredItemCount2, RequiredItemCount3, RequiredItemCount4, RequiredItemCount5, RequiredItemCount6,
		COALESCE(RewOrReqMoney, 0), COALESCE(RepObjectiveFaction, 0), COALESCE(RepObjectiveValue, 0),
		COALESCE(RequiredPlayerKills, 0)
		FROM quest_template WHERE ID = ?`, questID)
	err := row.Scan(&flags,
		&reqNpc1, &reqNpc2, &reqNpc3, &reqNpc4,
		&reqNpcCount1, &reqNpcCount2, &reqNpcCount3, &reqNpcCount4,
		&reqItem1, &reqItem2, &reqItem3, &reqItem4, &reqItem5, &reqItem6,
		&reqItemCount1, &reqItemCount2, &reqItemCount3, &reqItemCount4, &reqItemCount5, &reqItemCount6,
		&reqMoney, &repFaction, &repValue, &requiredPlayerKills)
	if err != nil {
		// Fallback for minimalist quest_template schema
		var simpleFlags int64
		if err2 := wdb.QueryRowContext(ctx, "SELECT Flags FROM quest_template WHERE ID = ?", questID).Scan(&simpleFlags); err2 != nil {
			return false
		}
		flags = simpleFlags
	}

	// Player::CanCompleteQuest (Player.cpp:14979): the autocomplete arm uses
	// Quest::IsAutoComplete (QuestDef.cpp:315) — Method == 0 also counts, not
	// just the QUEST_FLAGS_AUTOCOMPLETE bit.
	autoComplete := uint32(flags)&questAutoCompleteFlags != 0
	if !autoComplete {
		var method int64
		if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(Method, 0) FROM quest_template WHERE ID = ?", questID).Scan(&method); err == nil && method == 0 {
			autoComplete = true
		}
	}

	// 2. Locate the quest log slot.
	var entry questLogEntry
	foundSlot := false
	for slot := 0; slot < playerQuestLogSlots; slot++ {
		if s.player.QuestLog[slot].QuestID == questID {
			entry = s.player.QuestLog[slot]
			foundSlot = true
			break
		}
	}

	// Player::CanCompleteQuest (Player.cpp:14979-14981): the auto-complete
	// arm fires only for quests not in the log, gated on CanTakeQuest — the
	// quest must be takeable right now, not merely flagged auto-complete.
	if !foundSlot {
		if autoComplete {
			if canTake, takeErr := s.canTakeQuest(ctx, questID); takeErr == nil && canTake {
				return true
			}
		}
		return false
	}

	// Player::CanCompleteQuest (Player.cpp:14986): only INCOMPLETE quests
	// can transition to complete.
	if status, statusErr := s.characterQuestStatus(ctx, questID); statusErr != nil || status != questStatusIncomplete {
		return false
	}

	// Player::CanCompleteQuest (Player.cpp:15021-15022): a timed quest whose
	// timer has run out (Timer == 0) cannot be completed.
	var timedSpecial int64
	if err := wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&timedSpecial); err == nil && timedSpecial&questSpecialFlagsTimed != 0 && entry.Timer == 0 {
		return false
	}

	// Check NPC/Creature/GO kill/cast counters against quest log slot
	entryCounters := entry.Counters
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

	// Player::CanCompleteQuest (Player.cpp:15017-15019): player-kill
	// objective (QUEST_SPECIAL_FLAGS_PLAYER_KILL).
	if requiredPlayerKills > 0 && int64(entry.PlayerCount) < requiredPlayerKills {
		return false
	}

	// Player::CanCompleteQuest (Player.cpp:15021-15022): exploration/event
	// objective (QUEST_SPECIAL_FLAGS_EXPLORATION_OR_EVENT).
	var specialFlags int64
	_ = wdb.QueryRowContext(ctx, "SELECT COALESCE(SpecialFlags, 0) FROM quest_template_addon WHERE ID = ?", questID).Scan(&specialFlags)
	if specialFlags&questSpecialExplorationOrEvent != 0 && !entry.Explored {
		return false
	}

	// Player::CanCompleteQuest (Player.cpp:15028-15032): required money —
	// HasEnoughMoney(-RewOrReqMoney).
	if reqMoney < 0 && int64(s.player.Money) < -reqMoney {
		return false
	}

	// Player::CanCompleteQuest (Player.cpp:15034-15036): the first
	// reputation objective — ReputationMgr::GetReputation is base standing
	// plus recorded standing (ReputationMgr.cpp:119).
	if repFaction > 0 && int64(s.reputationStanding(uint32(repFaction))) < repValue {
		return false
	}

	return true
}
