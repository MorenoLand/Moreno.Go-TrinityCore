package world

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	creatureKillCredits  = 2
	creatureModels       = 4
	creatureQuestItems   = 6
	gameObjectData       = 24
	gameObjectQuestItems = 6
)

type creatureQueryData struct {
	Entry       uint32
	Name        string
	Title       string
	IconName    string
	Flags       uint32
	Type        uint32
	Family      uint32
	Rank        uint32
	KillCredits [creatureKillCredits]uint32
	Models      [creatureModels]uint32
	Health      float32
	Mana        float32
	Leader      bool
	QuestItems  [creatureQuestItems]uint32
	MovementID  uint32
}

type gameObjectQueryData struct {
	Entry      uint32
	Type       uint32
	DisplayID  uint32
	Name       string
	IconName   string
	CastBar    string
	Unknown    string
	Data       [gameObjectData]uint32
	Size       float32
	QuestItems [gameObjectQuestItems]uint32
}

func (s *session) handleCreatureQuery(ctx context.Context, payload []byte) bool {
	entry, guid, err := readQueryEntryAndGUID(payload)
	// DIAGNOSTIC: log all creature queries to diagnose Unknown names
	s.debug("creature query received", "account", s.accountName, "entry", entry, "guid", guid, "payload_len", len(payload))
	if err != nil {
		s.debug("creature query rejected", "account", s.accountName, "error", err)
		return false
	}
	data, err := s.loadCreatureQueryData(ctx, entry)
	if errors.Is(err, sql.ErrNoRows) {
		s.debug("creature query unknown", "account", s.accountName, "entry", entry)
		return s.write(uint16(protocol.OpcodeSMSG_CREATURE_QUERY_RESPONSE), buildCreatureQueryResponse(creatureQueryData{Entry: entry}, false), true) == nil
	}
	if err != nil {
		s.debug("creature query failed", "account", s.accountName, "entry", entry, "error", err)
		return s.write(uint16(protocol.OpcodeSMSG_CREATURE_QUERY_RESPONSE), buildCreatureQueryResponse(creatureQueryData{Entry: entry}, false), true) == nil
	}
	s.debug("creature query response", "account", s.accountName, "entry", entry, "name", data.Name)
	return s.write(uint16(protocol.OpcodeSMSG_CREATURE_QUERY_RESPONSE), buildCreatureQueryResponse(data, true), true) == nil
}

func (s *session) loadCreatureQueryData(ctx context.Context, entry uint32) (creatureQueryData, error) {
	var data creatureQueryData
	var killCredit1, killCredit2, model1, model2, model3, model4, flags, creatureType, family, rank, leader, movementID int64
	var name, title, iconName sql.NullString
	var health, mana float64
	err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT name, COALESCE(Title, ''), COALESCE(IconName, ''), type_flags, type, family, rank, KillCredit1, KillCredit2, modelid1, modelid2, modelid3, modelid4, HealthModifier, ManaModifier, RacialLeader, movementId FROM creature_template WHERE entry = ?", entry).Scan(&name, &title, &iconName, &flags, &creatureType, &family, &rank, &killCredit1, &killCredit2, &model1, &model2, &model3, &model4, &health, &mana, &leader, &movementID)
	if err != nil {
		return data, err
	}
	data.Entry = entry
	data.Name, data.Title, data.IconName = name.String, title.String, iconName.String
	data.Flags, data.Type, data.Family, data.Rank = uint32(flags), uint32(creatureType), uint32(family), uint32(rank)
	data.KillCredits = [creatureKillCredits]uint32{uint32(killCredit1), uint32(killCredit2)}
	data.Models = [creatureModels]uint32{uint32(model1), uint32(model2), uint32(model3), uint32(model4)}
	data.Health, data.Mana, data.Leader, data.MovementID = float32(health), float32(mana), leader != 0, uint32(movementID)
	items, err := loadQuestItems(ctx, s.server.WorldStore.DB, "creature_questitem", "CreatureEntry", "Idx", "ItemId", entry, creatureQuestItems)
	if err != nil {
		return data, err
	}
	copy(data.QuestItems[:], items)
	return data, nil
}

func buildCreatureQueryResponse(data creatureQueryData, allow bool) []byte {
	packet := protocol.NewBuffer(128)
	entry := data.Entry
	if !allow {
		entry |= 0x80000000
	}
	packet.WriteU32(entry)
	if !allow {
		return packet.Bytes()
	}
	packet.WriteCString(data.Name)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteCString(data.Title)
	packet.WriteCString(data.IconName)
	packet.WriteU32(data.Flags)
	packet.WriteU32(data.Type)
	packet.WriteU32(data.Family)
	packet.WriteU32(data.Rank)
	for _, value := range data.KillCredits {
		packet.WriteU32(value)
	}
	for _, value := range data.Models {
		packet.WriteU32(value)
	}
	packet.WriteF32(data.Health)
	packet.WriteF32(data.Mana)
	if data.Leader {
		packet.WriteU8(1)
	} else {
		packet.WriteU8(0)
	}
	for _, value := range data.QuestItems {
		packet.WriteU32(value)
	}
	packet.WriteU32(data.MovementID)
	return packet.Bytes()
}

func (s *session) handleGameObjectQuery(ctx context.Context, payload []byte) bool {
	entry, _, err := readQueryEntryAndGUID(payload)
	if err != nil {
		s.debug("gameobject query rejected", "account", s.accountName, "error", err)
		return false
	}
	data, err := s.loadGameObjectQueryData(ctx, entry)
	if errors.Is(err, sql.ErrNoRows) {
		s.debug("gameobject query unknown", "account", s.accountName, "entry", entry)
		return s.write(uint16(protocol.OpcodeSMSG_GAMEOBJECT_QUERY_RESPONSE), buildGameObjectQueryResponse(gameObjectQueryData{Entry: entry}, false), true) == nil
	}
	if err != nil {
		s.debug("gameobject query failed", "account", s.accountName, "entry", entry, "error", err)
		return false
	}
	s.debug("gameobject query response", "account", s.accountName, "entry", entry, "name", data.Name)
	return s.write(uint16(protocol.OpcodeSMSG_GAMEOBJECT_QUERY_RESPONSE), buildGameObjectQueryResponse(data, true), true) == nil
}

func readQueryEntryAndGUID(payload []byte) (uint32, uint64, error) {
	reader := protocol.NewReader(payload)
	entry, err := reader.ReadU32()
	if err != nil {
		return 0, 0, err
	}
	if reader.Remaining() == 0 {
		return entry, 0, nil
	}
	guid, err := reader.ReadU64()
	return entry, guid, err
}

func (s *session) loadGameObjectQueryData(ctx context.Context, entry uint32) (gameObjectQueryData, error) {
	var data gameObjectQueryData
	var objectType, displayID int64
	var name, iconName, castBar, unknown string
	var size float64
	values := make([]any, 0, 9+gameObjectData)
	values = append(values, &objectType, &displayID, &name, &iconName, &castBar, &unknown, &size)
	var raw [gameObjectData]int64
	for index := range raw {
		values = append(values, &raw[index])
	}
	args := make([]any, 0, 1)
	args = append(args, entry)
	query := "SELECT type, displayId, name, IconName, castBarCaption, unk1, size"
	for index := 0; index < gameObjectData; index++ {
		query += ", Data" + strconv.Itoa(index)
	}
	query += " FROM gameobject_template WHERE entry = ?"
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, query, args...).Scan(values...); err != nil {
		return data, err
	}
	data.Entry = entry
	data.Type, data.DisplayID, data.Name, data.IconName, data.CastBar, data.Unknown, data.Size = uint32(objectType), uint32(displayID), name, iconName, castBar, unknown, float32(size)
	for index, value := range raw {
		data.Data[index] = uint32(value)
	}
	items, err := loadQuestItems(ctx, s.server.WorldStore.DB, "gameobject_questitem", "GameObjectEntry", "Idx", "ItemId", entry, gameObjectQuestItems)
	if err != nil {
		return data, err
	}
	copy(data.QuestItems[:], items)
	return data, nil
}

func buildGameObjectQueryResponse(data gameObjectQueryData, allow bool) []byte {
	packet := protocol.NewBuffer(256)
	entry := data.Entry
	if !allow {
		entry |= 0x80000000
	}
	packet.WriteU32(entry)
	if !allow {
		return packet.Bytes()
	}
	packet.WriteU32(data.Type)
	packet.WriteU32(data.DisplayID)
	packet.WriteCString(data.Name)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteU8(0)
	packet.WriteCString(data.IconName)
	packet.WriteCString(data.CastBar)
	packet.WriteCString(data.Unknown)
	for _, value := range data.Data {
		packet.WriteU32(value)
	}
	packet.WriteF32(data.Size)
	for _, value := range data.QuestItems {
		packet.WriteU32(value)
	}
	return packet.Bytes()
}

func loadQuestItems(ctx context.Context, db *sql.DB, table, entryColumn, indexColumn, itemColumn string, entry uint32, limit int) ([]uint32, error) {
	items := make([]uint32, limit)
	rows, err := db.QueryContext(ctx, "SELECT "+itemColumn+" FROM "+table+" WHERE "+entryColumn+" = ? ORDER BY "+indexColumn+" LIMIT ?", entry, limit)
	if err != nil {
		if missingTable(err) || isMissingColumn(err) {
			return items, nil
		}
		return nil, err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var item int64
		if err := rows.Scan(&item); err != nil {
			return nil, err
		}
		if index < len(items) {
			items[index] = uint32(item)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
