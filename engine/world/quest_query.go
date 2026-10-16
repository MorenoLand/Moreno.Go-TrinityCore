package world

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	questObjectivesCount     = 4
	questItemObjectivesCount = 6
	questRewardsCount        = 4
	questChoicesCount        = 6
	questFactionsCount       = 5
	questHiddenRewardsFlag   = 0x00000200
)

type questQueryItem struct {
	ID       uint32
	Quantity uint32
}

type questQueryData struct {
	ID                    uint32
	Method                uint32
	Level                 int32
	MinLevel              uint32
	SortID                int32
	Type                  uint32
	SuggestedGroupNum     uint32
	AllowableRaces        uint32
	RequiredFactionID     [2]uint32
	RequiredFactionValue  [2]int32
	RewardNextQuest       uint32
	RewardXPDifficulty    uint32
	RewardMoney           uint32
	RewardBonusMoney      uint32
	RewardDisplaySpell    uint32
	RewardSpell           int32
	RewardHonor           uint32
	RewardKillHonor       float32
	StartItem             uint32
	Flags                 uint32
	RewardTitleID         uint32
	RequiredPlayerKills   uint32
	RewardTalents         uint32
	RewardArenaPoints     int32
	RewardItems           [questRewardsCount]questQueryItem
	ChoiceItems           [questChoicesCount]questQueryItem
	RewardFactionID       [questFactionsCount]uint32
	RewardFactionValue    [questFactionsCount]int32
	RewardFactionOverride [questFactionsCount]int32
	POIContinent          uint32
	POIX                  float32
	POIY                  float32
	POIPriority           uint32
	Title                 string
	Objectives            string
	Details               string
	AreaDescription       string
	CompletedText         string
	RequiredNpcOrGo       [questObjectivesCount]int32
	RequiredNpcOrGoCount  [questObjectivesCount]uint32
	ItemDrop              [questObjectivesCount]uint32
	RequiredItemID        [questItemObjectivesCount]uint32
	RequiredItemCount     [questItemObjectivesCount]uint32
	ObjectiveText         [questObjectivesCount]string
}

func (s *session) handleQuestQuery(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	questID, err := reader.ReadU32()
	if err != nil {
		return true
	}
	data, err := s.loadQuestQueryData(ctx, questID)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		s.debug("quest query failed", "account", s.accountName, "quest", questID, "error", err)
		return true
	}
	s.debug("quest query response", "account", s.accountName, "quest", questID)
	return s.write(uint16(protocol.OpcodeSMSG_QUEST_QUERY_RESPONSE), buildQuestQueryResponse(data), true) == nil
}

func questQueryColumns() []string {
	columns := []string{"QuestType", "QuestLevel", "MinLevel", "QuestSortID", "QuestInfoID", "SuggestedGroupNum", "AllowableRaces", "RequiredFactionId1", "RequiredFactionId2", "RequiredFactionValue1", "RequiredFactionValue2", "RewardNextQuest", "RewardXPDifficulty", "RewardMoney", "RewardBonusMoney", "RewardDisplaySpell", "RewardSpell", "RewardHonor", "RewardKillHonor", "StartItem", "Flags", "RewardTitle", "RequiredPlayerKills", "RewardTalents", "RewardArenaPoints"}
	for index := 1; index <= questRewardsCount; index++ {
		columns = append(columns, "RewardItem"+strconv.Itoa(index), "RewardAmount"+strconv.Itoa(index))
	}
	for index := 1; index <= questChoicesCount; index++ {
		columns = append(columns, "RewardChoiceItemID"+strconv.Itoa(index), "RewardChoiceItemQuantity"+strconv.Itoa(index))
	}
	for index := 1; index <= questFactionsCount; index++ {
		columns = append(columns, "RewardFactionID"+strconv.Itoa(index), "RewardFactionValue"+strconv.Itoa(index), "RewardFactionOverride"+strconv.Itoa(index))
	}
	columns = append(columns, "POIContinent", "POIx", "POIy", "POIPriority", "LogTitle", "LogDescription", "QuestDescription", "AreaDescription", "QuestCompletionLog")
	for index := 1; index <= questObjectivesCount; index++ {
		columns = append(columns, "RequiredNpcOrGo"+strconv.Itoa(index))
	}
	for index := 1; index <= questObjectivesCount; index++ {
		columns = append(columns, "RequiredNpcOrGoCount"+strconv.Itoa(index))
	}
	for index := 1; index <= questObjectivesCount; index++ {
		columns = append(columns, "ItemDrop"+strconv.Itoa(index))
	}
	for index := 1; index <= questItemObjectivesCount; index++ {
		columns = append(columns, "RequiredItemId"+strconv.Itoa(index))
	}
	for index := 1; index <= questItemObjectivesCount; index++ {
		columns = append(columns, "RequiredItemCount"+strconv.Itoa(index))
	}
	for index := 1; index <= questObjectivesCount; index++ {
		columns = append(columns, "ObjectiveText"+strconv.Itoa(index))
	}
	return columns
}

func (s *session) loadQuestQueryData(ctx context.Context, questID uint32) (questQueryData, error) {
	var data questQueryData
	var method, level, minLevel, sortID, questType, suggestedGroup, allowableRaces, factionID1, factionID2, factionValue1, factionValue2 int64
	var rewardNext, rewardXP, rewardMoney, rewardBonusMoney, rewardDisplaySpell, rewardSpell, rewardHonor, startItem, flags, rewardTitle, requiredKills, rewardTalents, rewardArena int64
	var rewardKillHonor float64
	var rewardIDs [questRewardsCount]int64
	var rewardCounts [questRewardsCount]int64
	var choiceIDs [questChoicesCount]int64
	var choiceCounts [questChoicesCount]int64
	var rewardFactionIDs [questFactionsCount]int64
	var rewardFactionValues [questFactionsCount]int64
	var rewardFactionOverrides [questFactionsCount]int64
	var poiContinent, poiPriority int64
	var poiX, poiY float64
	var title, objectives, details, areaDescription, completedText sql.NullString
	var requiredNpcOrGo [questObjectivesCount]int64
	var requiredNpcOrGoCount [questObjectivesCount]int64
	var itemDrop [questObjectivesCount]int64
	var requiredItemID [questItemObjectivesCount]int64
	var requiredItemCount [questItemObjectivesCount]int64
	var objectiveText [questObjectivesCount]sql.NullString
	targets := []any{&method, &level, &minLevel, &sortID, &questType, &suggestedGroup, &allowableRaces, &factionID1, &factionID2, &factionValue1, &factionValue2, &rewardNext, &rewardXP, &rewardMoney, &rewardBonusMoney, &rewardDisplaySpell, &rewardSpell, &rewardHonor, &rewardKillHonor, &startItem, &flags, &rewardTitle, &requiredKills, &rewardTalents, &rewardArena}
	for index := range rewardIDs {
		targets = append(targets, &rewardIDs[index], &rewardCounts[index])
	}
	for index := range choiceIDs {
		targets = append(targets, &choiceIDs[index], &choiceCounts[index])
	}
	for index := range rewardFactionIDs {
		targets = append(targets, &rewardFactionIDs[index], &rewardFactionValues[index], &rewardFactionOverrides[index])
	}
	targets = append(targets, &poiContinent, &poiX, &poiY, &poiPriority, &title, &objectives, &details, &areaDescription, &completedText)
	for index := range requiredNpcOrGo {
		targets = append(targets, &requiredNpcOrGo[index])
	}
	for index := range requiredNpcOrGoCount {
		targets = append(targets, &requiredNpcOrGoCount[index])
	}
	for index := range itemDrop {
		targets = append(targets, &itemDrop[index])
	}
	for index := range requiredItemID {
		targets = append(targets, &requiredItemID[index])
	}
	for index := range requiredItemCount {
		targets = append(targets, &requiredItemCount[index])
	}
	for index := range objectiveText {
		targets = append(targets, &objectiveText[index])
	}
	query := "SELECT " + strings.Join(questQueryColumns(), ", ") + " FROM quest_template WHERE ID = ?"
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, query, questID).Scan(targets...); err != nil {
		return data, err
	}
	data.ID, data.Method, data.Level, data.MinLevel, data.SortID, data.Type = questID, uint32(method), int32(level), uint32(minLevel), int32(sortID), uint32(questType)
	data.SuggestedGroupNum, data.AllowableRaces = uint32(suggestedGroup), uint32(allowableRaces)
	data.RequiredFactionID, data.RequiredFactionValue = [2]uint32{uint32(factionID1), uint32(factionID2)}, [2]int32{int32(factionValue1), int32(factionValue2)}
	data.RewardNextQuest, data.RewardXPDifficulty, data.RewardMoney, data.RewardBonusMoney = uint32(rewardNext), uint32(rewardXP), uint32(rewardMoney), uint32(rewardBonusMoney)
	data.RewardDisplaySpell, data.RewardSpell, data.RewardHonor, data.RewardKillHonor = uint32(rewardDisplaySpell), int32(rewardSpell), uint32(rewardHonor), float32(rewardKillHonor)
	data.StartItem, data.Flags, data.RewardTitleID, data.RequiredPlayerKills, data.RewardTalents, data.RewardArenaPoints = uint32(startItem), uint32(flags), uint32(rewardTitle), uint32(requiredKills), uint32(rewardTalents), int32(rewardArena)
	for index := range data.RewardItems {
		data.RewardItems[index] = questQueryItem{ID: uint32(rewardIDs[index]), Quantity: uint32(rewardCounts[index])}
	}
	for index := range data.ChoiceItems {
		data.ChoiceItems[index] = questQueryItem{ID: uint32(choiceIDs[index]), Quantity: uint32(choiceCounts[index])}
	}
	for index := range data.RewardFactionID {
		data.RewardFactionID[index], data.RewardFactionValue[index], data.RewardFactionOverride[index] = uint32(rewardFactionIDs[index]), int32(rewardFactionValues[index]), int32(rewardFactionOverrides[index])
	}
	data.POIContinent, data.POIX, data.POIY, data.POIPriority = uint32(poiContinent), float32(poiX), float32(poiY), uint32(poiPriority)
	data.Title, data.Objectives, data.Details, data.AreaDescription, data.CompletedText = title.String, objectives.String, details.String, areaDescription.String, completedText.String
	for index := range data.RequiredNpcOrGo {
		data.RequiredNpcOrGo[index], data.RequiredNpcOrGoCount[index], data.ItemDrop[index] = int32(requiredNpcOrGo[index]), uint32(requiredNpcOrGoCount[index]), uint32(itemDrop[index])
	}
	for index := range data.RequiredItemID {
		data.RequiredItemID[index], data.RequiredItemCount[index] = uint32(requiredItemID[index]), uint32(requiredItemCount[index])
	}
	for index := range data.ObjectiveText {
		data.ObjectiveText[index] = objectiveText[index].String
	}
	return data, nil
}

func buildQuestQueryResponse(data questQueryData) []byte {
	packet := protocol.NewBuffer(2048)
	hiddenRewards := data.Flags&questHiddenRewardsFlag != 0
	packet.WriteU32(data.ID)
	packet.WriteU32(data.Method)
	packet.WriteI32(data.Level)
	packet.WriteU32(data.MinLevel)
	packet.WriteI32(data.SortID)
	packet.WriteU32(data.Type)
	packet.WriteU32(data.SuggestedGroupNum)
	for index := range data.RequiredFactionID {
		packet.WriteU32(data.RequiredFactionID[index])
		packet.WriteI32(data.RequiredFactionValue[index])
	}
	packet.WriteU32(data.RewardNextQuest)
	packet.WriteU32(data.RewardXPDifficulty)
	if hiddenRewards {
		packet.WriteU32(0)
	} else {
		packet.WriteU32(data.RewardMoney)
	}
	packet.WriteU32(data.RewardBonusMoney)
	packet.WriteU32(data.RewardDisplaySpell)
	packet.WriteI32(data.RewardSpell)
	packet.WriteU32(data.RewardHonor)
	packet.WriteF32(data.RewardKillHonor)
	packet.WriteU32(data.StartItem)
	packet.WriteU32(data.Flags & 0xFFFF)
	packet.WriteU32(data.RewardTitleID)
	packet.WriteU32(data.RequiredPlayerKills)
	packet.WriteU32(data.RewardTalents)
	packet.WriteI32(data.RewardArenaPoints)
	packet.WriteU32(0)
	for _, item := range data.RewardItems {
		if hiddenRewards {
			packet.WriteU32(0)
			packet.WriteU32(0)
			continue
		}
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
	}
	for _, item := range data.ChoiceItems {
		if hiddenRewards {
			packet.WriteU32(0)
			packet.WriteU32(0)
			continue
		}
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
	}
	for _, value := range data.RewardFactionID {
		packet.WriteU32(value)
	}
	for _, value := range data.RewardFactionValue {
		packet.WriteI32(value)
	}
	for _, value := range data.RewardFactionOverride {
		packet.WriteI32(value)
	}
	packet.WriteU32(data.POIContinent)
	packet.WriteF32(data.POIX)
	packet.WriteF32(data.POIY)
	packet.WriteU32(data.POIPriority)
	packet.WriteCString(data.Title)
	packet.WriteCString(data.Objectives)
	packet.WriteCString(data.Details)
	packet.WriteCString(data.AreaDescription)
	packet.WriteCString(data.CompletedText)
	for index := range data.RequiredNpcOrGo {
		value := uint32(data.RequiredNpcOrGo[index])
		if data.RequiredNpcOrGo[index] < 0 {
			value = uint32(-data.RequiredNpcOrGo[index]) | 0x80000000
		}
		packet.WriteU32(value)
		packet.WriteU32(data.RequiredNpcOrGoCount[index])
		packet.WriteU32(data.ItemDrop[index])
		packet.WriteU32(0)
	}
	for index := range data.RequiredItemID {
		packet.WriteU32(data.RequiredItemID[index])
		packet.WriteU32(data.RequiredItemCount[index])
	}
	for _, value := range data.ObjectiveText {
		packet.WriteCString(value)
	}
	return packet.Bytes()
}
