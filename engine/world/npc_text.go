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
	var broadcastResolved [npcTextOptions]bool
	for index := range options {
		options[index].Probability = float32(probabilities[index])
		options[index].Text0, options[index].Text1 = text0[index].String, text1[index].String
		if broadcastTextIDs[index] != 0 {
			// HandleNpcTextQueryOpcode (QueryHandler.cpp:226-237): the
			// broadcast text wins over the npc_text columns, resolved
			// locale- and gender-aware (see broadcastNPCText).
			if broadcast0, broadcast1, ok := s.broadcastNPCText(ctx, broadcastTextIDs[index]); ok {
				options[index].Text0, options[index].Text1 = broadcast0, broadcast1
				broadcastResolved[index] = true
			}
		}
		options[index].Language = uint32(languages[index])
		for emote := 0; emote < npcTextEmotes; emote++ {
			options[index].Emotes[emote] = npcTextEmote{Delay: uint32(emoteDelays[index][emote]), ID: uint32(emoteIDs[index][emote])}
		}
	}
	// QueryHandler.cpp:239-246: with no broadcast text and a non-enUS
	// session, npc_text_locale overrides the base Text_0/Text_1 per option
	// (non-empty locale strings only, ObjectMgr::GetLocaleString).
	if s.locale != 0 && int(s.locale) < len(localeNames) && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		columns := make([]string, 0, npcTextOptions*2)
		for index := 0; index < npcTextOptions; index++ {
			suffix := strconv.Itoa(index)
			columns = append(columns, "Text"+suffix+"_0", "Text"+suffix+"_1")
		}
		var locText [npcTextOptions][2]sql.NullString
		targets := make([]any, 0, len(columns))
		for index := range locText {
			targets = append(targets, &locText[index][0], &locText[index][1])
		}
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT "+strings.Join(columns, ", ")+" FROM npc_text_locale WHERE ID = ? AND Locale = ?", textID, localeNames[s.locale]).Scan(targets...); err == nil {
			for index := range options {
				// A dangling BroadcastTextID (row missing from
				// broadcast_text) counts as no broadcast text in C++
				// (GetBroadcastText null), so the locale fallback still
				// applies to that option.
				if broadcastResolved[index] {
					continue
				}
				if locText[index][0].Valid && locText[index][0].String != "" {
					options[index].Text0 = locText[index][0].String
				}
				if locText[index][1].Valid && locText[index][1].String != "" {
					options[index].Text1 = locText[index][1].String
				}
			}
		}
	}
	return options, nil
}

// broadcastNPCText mirrors the broadcast-text arm of
// HandleNpcTextQueryOpcode (QueryHandler.cpp:226-237):
// BroadcastText::GetText(locale, GENDER_MALE, true) for Text_0 and
// GetText(locale, GENDER_FEMALE, true) for Text_1 — the forceGender=true
// call always reads the Text1 column for the female line, and a non-empty
// broadcast_text_locale row overrides the enUS text per column.
func (s *session) broadcastNPCText(ctx context.Context, broadcastID int64) (string, string, bool) {
	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return "", "", false
	}
	var text, text1 sql.NullString
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Text, Text1 FROM broadcast_text WHERE ID = ?", broadcastID).Scan(&text, &text1); err != nil {
		return "", "", false
	}
	if s.locale != 0 && int(s.locale) < len(localeNames) {
		var locText, locText1 sql.NullString
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT Text, Text1 FROM broadcast_text_locale WHERE ID = ? AND locale = ?", broadcastID, localeNames[s.locale]).Scan(&locText, &locText1); err == nil {
			if locText.Valid && locText.String != "" {
				text = locText
			}
			if locText1.Valid && locText1.String != "" {
				text1 = locText1
			}
		}
	}
	return text.String, text1.String, true
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
