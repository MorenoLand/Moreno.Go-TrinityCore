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
	npcTextOptions = 8
	npcTextEmotes  = 3
)

type npcTextEmote struct {
	Delay uint32
	ID    uint32
}

type npcTextOption struct {
	Probability float32
	Text0       string
	Text1       string
	Language    uint32
	Emotes      [npcTextEmotes]npcTextEmote
}

func (s *session) handleNpcTextQuery(ctx context.Context, payload []byte) bool {
	reader := protocol.NewReader(payload)
	textID, err := reader.ReadU32()
	if err != nil {
		s.debug("npc text query rejected", "account", s.accountName, "error", err)
		return true
	}
	guid, err := reader.ReadU64()
	if err != nil {
		s.debug("npc text query rejected", "account", s.accountName, "error", err)
		return true
	}
	options, err := s.loadNPCText(ctx, textID)
	if errors.Is(err, sql.ErrNoRows) || (err != nil && missingTable(err)) {
		options = defaultNPCTextOptions()
	} else if err != nil {
		// HandleNpcTextQueryOpcode (QueryHandler.cpp:192-253) always sends
		// the packet — fall through to the "Greetings $N" default instead
		// of dropping it.
		s.debug("npc text query failed", "account", s.accountName, "text", textID, "error", err)
		options = defaultNPCTextOptions()
	}
	s.debug("npc text response", "account", s.accountName, "text", textID, "guid", guid)
	return s.write(uint16(protocol.OpcodeSMSG_NPC_TEXT_UPDATE), buildNPCTextUpdate(textID, options), true) == nil
}

func (s *session) loadNPCText(ctx context.Context, textID uint32) ([]npcTextOption, error) {
	columns := make([]string, 0, npcTextOptions*10)
	for index := 0; index < npcTextOptions; index++ {
		suffix := strconv.Itoa(index)
		columns = append(columns, "text"+suffix+"_0", "text"+suffix+"_1", "BroadcastTextID"+suffix, "lang"+suffix, "Probability"+suffix)
		for emote := 0; emote < npcTextEmotes; emote++ {
			columns = append(columns, "EmoteDelay"+suffix+"_"+strconv.Itoa(emote), "Emote"+suffix+"_"+strconv.Itoa(emote))
		}
	}
	query := "SELECT " + strings.Join(columns, ", ") + " FROM npc_text WHERE ID = ?"
	row := s.server.WorldStore.DB.QueryRowContext(ctx, query, textID)
	options := make([]npcTextOption, npcTextOptions)
	targets := make([]any, 0, len(columns))
	var text0, text1 [npcTextOptions]sql.NullString
	var broadcastTextIDs [npcTextOptions]int64
	var languages [npcTextOptions]int64
	var probabilities [npcTextOptions]float64
	var emoteDelays [npcTextOptions][npcTextEmotes]int64
	var emoteIDs [npcTextOptions][npcTextEmotes]int64
	for index := range options {
		targets = append(targets, &text0[index], &text1[index], &broadcastTextIDs[index], &languages[index], &probabilities[index])
		for emote := 0; emote < npcTextEmotes; emote++ {
			targets = append(targets, &emoteDelays[index][emote], &emoteIDs[index][emote])
		}
	}
	if err := row.Scan(targets...); err != nil {
		return nil, err
	}
	for index := range options {
		options[index].Probability = float32(probabilities[index])
		options[index].Text0, options[index].Text1 = text0[index].String, text1[index].String
		if broadcastTextIDs[index] != 0 {
			var broadcast0, broadcast1 sql.NullString
			err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Text, Text1 FROM broadcast_text WHERE ID = ?", broadcastTextIDs[index]).Scan(&broadcast0, &broadcast1)
			if err == nil {
				options[index].Text0, options[index].Text1 = broadcast0.String, broadcast1.String
			}
		}
		options[index].Language = uint32(languages[index])
		for emote := 0; emote < npcTextEmotes; emote++ {
			options[index].Emotes[emote] = npcTextEmote{Delay: uint32(emoteDelays[index][emote]), ID: uint32(emoteIDs[index][emote])}
		}
	}
	return options, nil
}

func defaultNPCTextOptions() []npcTextOption {
	options := make([]npcTextOption, npcTextOptions)
	for index := range options {
		options[index].Text0, options[index].Text1 = "Greetings $N", "Greetings $N"
	}
	return options
}

func buildNPCTextUpdate(textID uint32, options []npcTextOption) []byte {
	packet := protocol.NewBuffer(1024)
	packet.WriteU32(textID)
	for index := 0; index < npcTextOptions; index++ {
		var option npcTextOption
		if index < len(options) {
			option = options[index]
		}
		text0, text1 := option.Text0, option.Text1
		if text0 == "" {
			text0 = text1
		}
		if text1 == "" {
			text1 = text0
		}
		packet.WriteF32(option.Probability)
		packet.WriteCString(text0)
		packet.WriteCString(text1)
		packet.WriteU32(option.Language)
		for _, emote := range option.Emotes {
			packet.WriteU32(emote.Delay)
			packet.WriteU32(emote.ID)
		}
	}
	return packet.Bytes()
}
