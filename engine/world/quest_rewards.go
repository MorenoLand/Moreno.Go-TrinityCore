package world

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const questRequiredItems = 6

type questRewardView struct {
	Detail          questDetailData
	RequiredItems   []questRewardItem
	RequestText     string
	CompleteEmote   uint32
	IncompleteEmote uint32
	RewardText      string
	OfferEmotes     []questDescEmote
}

func (s *session) handleQuestgiverCompleteQuest(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	giverGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	questID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	entry, ok := s.questGiverEnder(ctx, giverGUID, questID)
	if !ok {
		return true
	}
	status, err := s.characterQuestStatus(ctx, questID)
	if err != nil {
		return false
	}
	if status == questStatusIncomplete && s.canCompleteQuest(ctx, questID) {
		s.completeQuest(ctx, questID)
		status = questStatusComplete
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		return false
	}
	if status != questStatusComplete && s.isQuestRewarded(ctx, questID) {
		return s.sendQuestRequestItems(view, giverGUID, false, false)
	}
	if status == 0 && view.Detail.Flags&questAutoCompleteFlags == 0 {
		return true
	}
	canComplete := status == questStatusComplete || view.Detail.Flags&questAutoCompleteFlags != 0
	if canComplete && len(view.RequiredItems) != 0 {
		canComplete, err = s.hasQuestRequiredItems(ctx, view.RequiredItems)
		if err != nil {
			return false
		}
	}
	if canComplete && len(view.RequiredItems) == 0 {
		s.debug("quest offer reward", "account", s.accountName, "entry", entry, "quest", questID)
		return s.sendQuestOfferReward(view, giverGUID, true)
	}
	s.debug("quest request items", "account", s.accountName, "entry", entry, "quest", questID, "complete", canComplete)
	return s.sendQuestRequestItems(view, giverGUID, canComplete, false)
}

func (s *session) handleQuestgiverRequestReward(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	giverGUID, err := reader.ReadU64()
	if err != nil {
		return false
	}
	questID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if _, ok := s.questGiverEnder(ctx, giverGUID, questID); !ok {
		return true
	}
	status, err := s.characterQuestStatus(ctx, questID)
	if err != nil {
		return false
	}
	if status == questStatusIncomplete && s.canCompleteQuest(ctx, questID) {
		s.completeQuest(ctx, questID)
		status = questStatusComplete
	}
	view, err := s.loadQuestRewardView(ctx, questID)
	if err != nil {
		return false
	}
	if status != questStatusComplete || s.isQuestRewarded(ctx, questID) {
		return true
	}
	if len(view.RequiredItems) != 0 {
		complete, itemErr := s.hasQuestRequiredItems(ctx, view.RequiredItems)
		if itemErr != nil {
			return false
		}
		if !complete {
			return true
		}
	}
	return s.sendQuestOfferReward(view, giverGUID, true)
}

func (s *session) questGiverEnder(ctx context.Context, guid uint64, questID uint32) (uint32, bool) {
	if creature := s.luaCreature(ctx, guid); creature != nil {
		entry := objectUint32OrZero(creature, "Entry")
		if entry != 0 && s.creatureEndsQuest(ctx, entry, questID) {
			return entry, true
		}
	}
	if object := s.luaGameObject(ctx, guid); object != nil {
		entry := objectUint32OrZero(object, "Entry")
		if entry != 0 && questRelationExists(ctx, s.server.WorldStore.DB, "gameobject_questender", entry, questID) {
			return entry, true
		}
	}
	// Fallback to packed GUID entry
	creatureEntry := uint32((guid >> 24) & 0x00FFFFFF)
	if creatureEntry != 0 && s.creatureEndsQuest(ctx, creatureEntry, questID) {
		return creatureEntry, true
	}
	// Fallback to DB spawn query
	spawnGUID := uint32(guid & 0x00FFFFFF)
	if s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var dbEntry int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM creature WHERE guid = ?", spawnGUID).Scan(&dbEntry); err == nil && dbEntry > 0 {
			if s.creatureEndsQuest(ctx, uint32(dbEntry), questID) {
				return uint32(dbEntry), true
			}
		}
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT id FROM gameobject WHERE guid = ?", spawnGUID).Scan(&dbEntry); err == nil && dbEntry > 0 {
			if questRelationExists(ctx, s.server.WorldStore.DB, "gameobject_questender", uint32(dbEntry), questID) {
				return uint32(dbEntry), true
			}
		}
	}
	return 0, false
}

func (s *session) loadQuestRewardView(ctx context.Context, questID uint32) (questRewardView, error) {
	view := questRewardView{}
	detail, err := s.loadQuestDetailData(ctx, questID)
	if err != nil {
		return view, err
	}
	view.Detail = detail
	ids := [questRequiredItems]int64{}
	counts := [questRequiredItems]int64{}
	columns := make([]string, 0, questRequiredItems*2)
	targets := make([]any, 0, questRequiredItems*2)
	for index := 1; index <= questRequiredItems; index++ {
		columns = append(columns, "RequiredItemId"+itoa(index), "RequiredItemCount"+itoa(index))
		targets = append(targets, &ids[index-1], &counts[index-1])
	}
	query := "SELECT " + strings.Join(columns, ", ") + " FROM quest_template WHERE ID = ?"
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, query, questID).Scan(targets...); err != nil {
		if !isMissingColumn(err) {
			return view, err
		}
	} else {
		for index, id := range ids {
			if id != 0 {
				view.RequiredItems = append(view.RequiredItems, questRewardItem{ID: uint32(id), Quantity: uint32(counts[index]), DisplayID: s.itemDisplayID(ctx, uint32(id))})
			}
		}
	}
	var requestText sql.NullString
	var completeEmote, incompleteEmote int64
	err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT EmoteOnComplete, EmoteOnIncomplete, CompletionText FROM quest_request_items WHERE ID = ?", questID).Scan(&completeEmote, &incompleteEmote, &requestText)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
		return view, err
	}
	view.CompleteEmote, view.IncompleteEmote, view.RequestText = uint32(completeEmote), uint32(incompleteEmote), requestText.String
	var rewardText sql.NullString
	var emoteTypes, emoteDelays [questDetailEmotes]int64
	targets = make([]any, 0, questDetailEmotes*2+1)
	for index := range emoteTypes {
		targets = append(targets, &emoteTypes[index])
	}
	for index := range emoteDelays {
		targets = append(targets, &emoteDelays[index])
	}
	targets = append(targets, &rewardText)
	err = s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Emote1, Emote2, Emote3, Emote4, EmoteDelay1, EmoteDelay2, EmoteDelay3, EmoteDelay4, RewardText FROM quest_offer_reward WHERE ID = ?", questID).Scan(targets...)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !missingTable(err) {
		return view, err
	}
	view.RewardText = rewardText.String
	for index, emoteType := range emoteTypes {
		if emoteType == 0 {
			break
		}
		view.OfferEmotes = append(view.OfferEmotes, questDescEmote{Type: uint32(emoteType), Delay: uint32(emoteDelays[index])})
	}
	return view, nil
}

func (s *session) hasQuestRequiredItems(ctx context.Context, items []questRewardItem) (bool, error) {
	for _, item := range items {
		var count int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci JOIN item_instance AS ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?", s.playerGUID, item.ID).Scan(&count)
		if err != nil {
			if missingTable(err) {
				return false, nil
			}
			return false, err
		}
		if count < int64(item.Quantity) {
			return false, nil
		}
	}
	return true, nil
}

func (s *session) sendQuestRequestItems(view questRewardView, giverGUID uint64, canComplete, closeOnCancel bool) bool {
	emote := view.IncompleteEmote
	if canComplete {
		emote = view.CompleteEmote
	}
	packet := buildQuestGiverRequestItems(view, giverGUID, emote, canComplete, closeOnCancel)
	return s.write(uint16(protocol.OpcodeSMSG_QUESTGIVER_REQUEST_ITEMS), packet, true) == nil
}

func (s *session) sendQuestOfferReward(view questRewardView, giverGUID uint64, autoLaunched bool) bool {
	return s.write(uint16(protocol.OpcodeSMSG_QUEST_GIVER_OFFER_REWARD_MESSAGE), buildQuestGiverOfferReward(view, giverGUID, autoLaunched), true) == nil
}

func buildQuestGiverRequestItems(view questRewardView, giverGUID uint64, emote uint32, canComplete, closeOnCancel bool) []byte {
	packet := protocol.NewBuffer(256)
	packet.WriteU64(giverGUID)
	packet.WriteU32(view.Detail.ID)
	packet.WriteCString(view.Detail.Title)
	packet.WriteCString(view.RequestText)
	packet.WriteU32(0)
	packet.WriteU32(emote)
	if closeOnCancel {
		packet.WriteU32(1)
	} else {
		packet.WriteU32(0)
	}
	packet.WriteU32(view.Detail.Flags)
	packet.WriteU32(view.Detail.SuggestedGroupNum)
	packet.WriteU32(0)
	packet.WriteU32(uint32(len(view.RequiredItems)))
	for _, item := range view.RequiredItems {
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
		packet.WriteU32(item.DisplayID)
	}
	if canComplete {
		packet.WriteU32(3)
	} else {
		packet.WriteU32(0)
	}
	packet.WriteU32(4)
	packet.WriteU32(8)
	packet.WriteU32(16)
	return packet.Bytes()
}

func buildQuestGiverOfferReward(view questRewardView, giverGUID uint64, autoLaunched bool) []byte {
	packet := protocol.NewBuffer(512)
	packet.WriteU64(giverGUID)
	packet.WriteU32(view.Detail.ID)
	packet.WriteCString(view.Detail.Title)
	packet.WriteCString(view.RewardText)
	if autoLaunched {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	packet.WriteU32(view.Detail.Flags)
	packet.WriteU32(view.Detail.SuggestedGroupNum)
	packet.WriteU32(uint32(len(view.OfferEmotes)))
	for _, emote := range view.OfferEmotes {
		packet.WriteU32(emote.Delay)
		packet.WriteU32(emote.Type)
	}
	packet.WriteU32(uint32(len(view.Detail.ChoiceItems)))
	for _, item := range view.Detail.ChoiceItems {
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
		packet.WriteU32(item.DisplayID)
	}
	packet.WriteU32(uint32(len(view.Detail.RewardItems)))
	for _, item := range view.Detail.RewardItems {
		packet.WriteU32(item.ID)
		packet.WriteU32(item.Quantity)
		packet.WriteU32(item.DisplayID)
	}
	packet.WriteU32(view.Detail.RewardMoney)
	packet.WriteU32(view.Detail.RewardXPDifficulty)
	packet.WriteU32(view.Detail.RewardHonor)
	packet.WriteF32(view.Detail.RewardKillHonor)
	packet.WriteU32(view.Detail.RewardDisplaySpell)
	packet.WriteI32(view.Detail.RewardSpell)
	packet.WriteU32(view.Detail.RewardTitleID)
	packet.WriteU32(view.Detail.RewardTalents)
	packet.WriteI32(view.Detail.RewardArenaPoints)
	packet.WriteU32(0)
	for _, value := range view.Detail.RewardFactionID {
		packet.WriteU32(value)
	}
	for _, value := range view.Detail.RewardFactionValue {
		packet.WriteI32(value)
	}
	for _, value := range view.Detail.RewardFactionOverride {
		packet.WriteI32(value)
	}
	return packet.Bytes()
}
