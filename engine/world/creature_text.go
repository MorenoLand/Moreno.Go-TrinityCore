package world

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// CreatureText matches the schema of world database `creature_text`.
// Reference: TrinityCore CreatureTextMgr.h / CreatureTextMgr.cpp.
type CreatureText struct {
	CreatureID  uint32
	GroupID     uint8
	ID          uint8
	Text        string
	Type        uint8
	Language    uint8
	Probability float32
	Emote       uint32
	Duration    uint32
	Sound       uint32
	// TextRange is the row's broadcast range (CreatureTextEntry.TextRange,
	// CreatureTextMgr.h:55; TEXT_RANGE_NORMAL=0, AREA=1, ZONE=2, MAP=3,
	// WORLD=4 at :33-40). DB default 0.
	TextRange uint8
}

type creatureTextMgr struct {
	mu    sync.RWMutex
	texts map[uint32]map[uint8][]CreatureText
}

func newCreatureTextMgr() *creatureTextMgr {
	return &creatureTextMgr{
		texts: make(map[uint32]map[uint8][]CreatureText),
	}
}

func (s *Server) loadCreatureTexts(ctx context.Context, creatureID uint32) map[uint8][]CreatureText {
	if s == nil || s.WorldStore == nil || s.WorldStore.DB == nil || creatureID == 0 {
		return nil
	}
	rows, err := s.WorldStore.DB.QueryContext(ctx, "SELECT CreatureID, GroupID, ID, Text, Type, Language, Probability, Emote, Duration, Sound, TextRange FROM creature_text WHERE CreatureID = ? ORDER BY GroupID, ID", creatureID)
	withRange := true
	if err != nil && isMissingColumn(err) {
		// DBs missing the TextRange column (CreatureTextEntry.TextRange,
		// CreatureTextMgr.h:55): fall back to the range-less load; rows keep
		// TextRange 0 (TEXT_RANGE_NORMAL, the DB default).
		rows, err = s.WorldStore.DB.QueryContext(ctx, "SELECT CreatureID, GroupID, ID, Text, Type, Language, Probability, Emote, Duration, Sound FROM creature_text WHERE CreatureID = ? ORDER BY GroupID, ID", creatureID)
		withRange = false
	}
	if err != nil {
		return nil
	}
	defer rows.Close()

	groups := make(map[uint8][]CreatureText)
	for rows.Next() {
		var t CreatureText
		var p float64
		var scanErr error
		if withRange {
			scanErr = rows.Scan(&t.CreatureID, &t.GroupID, &t.ID, &t.Text, &t.Type, &t.Language, &p, &t.Emote, &t.Duration, &t.Sound, &t.TextRange)
		} else {
			scanErr = rows.Scan(&t.CreatureID, &t.GroupID, &t.ID, &t.Text, &t.Type, &t.Language, &p, &t.Emote, &t.Duration, &t.Sound)
		}
		if scanErr == nil {
			t.Probability = float32(p)
			// CreatureTextMgr.cpp:160-163 clamps rows with an out-of-range
			// TextRange back to TEXT_RANGE_NORMAL at DB load (TEXT_RANGE_WORLD=4).
			if t.TextRange > 4 {
				t.TextRange = 0
			}
			groups[t.GroupID] = append(groups[t.GroupID], t)
		}
	}
	return groups
}

// broadcastCreatureTalk emits sound, emote, and chat message matching TrinityCore CreatureTextMgr::SendChat.
func (s *Server) broadcastCreatureTalk(ctx context.Context, mapID uint32, creatureGUID uint64, entry uint32, creatureName string, groupID uint8, whisperTarget uint64) {
	if s == nil || entry == 0 {
		return
	}
	if s.creatureTextMgr == nil {
		s.creatureTextMgr = newCreatureTextMgr()
	}

	s.creatureTextMgr.mu.RLock()
	groups, found := s.creatureTextMgr.texts[entry]
	s.creatureTextMgr.mu.RUnlock()

	if !found {
		loaded := s.loadCreatureTexts(ctx, entry)
		s.creatureTextMgr.mu.Lock()
		if s.creatureTextMgr.texts == nil {
			s.creatureTextMgr.texts = make(map[uint32]map[uint8][]CreatureText)
		}
		s.creatureTextMgr.texts[entry] = loaded
		groups = loaded
		s.creatureTextMgr.mu.Unlock()
	}

	if groups == nil {
		return
	}
	list := groups[groupID]
	if len(list) == 0 {
		return
	}

	// Select text by probability or random element
	chosen := list[0]
	if len(list) > 1 {
		chosen = list[rand.Intn(len(list))]
	}

	msg := chosen.Text
	if strings.Contains(msg, "%s") {
		msg = fmt.Sprintf(msg, creatureName)
	}

	chatType := chosen.Type
	if chatType == 0 {
		chatType = 12 // CHAT_MSG_MONSTER_SAY
	}
	// CreatureAI::Talk(id, target) -> CreatureTextMgr::SendChat(source, id,
	// whisperTarget): for CHAT_MSG_MONSTER_WHISPER (0x0F) / RAID_BOSS_WHISPER
	// (0x2A) with TEXT_RANGE_NORMAL the chat packet and the sound go to the
	// whisper target only (CreatureTextMgrImpl.h:87, CreatureTextMgr.cpp:306).
	// Non-player whisper targets drop the message, like C++.
	// The whisper drop applies only when the row's effective range is
	// TEXT_RANGE_NORMAL: SendChat replaces the passed range with the row's
	// TextRange (CreatureTextMgr.cpp:251-252; CreatureAI::Talk always passes
	// TEXT_RANGE_NORMAL, CreatureAI.cpp:48-51), and both the sound leg
	// (CreatureTextMgr.cpp:322-331) and the chat leg
	// (CreatureTextMgrImpl.h:106-115) drop a null/non-player whisper target
	// only under TEXT_RANGE_NORMAL. A whisper row with AREA/ZONE/MAP/WORLD
	// range falls through to the range broadcast like any other row.
	whispered := (chatType == 0x0F || chatType == 0x2A) && chosen.TextRange == 0
	var whisperSess *session
	if whispered {
		if whisperTarget != 0 {
			whisperSess = s.findSessionByGUID(whisperTarget)
		}
		if whisperSess == nil || whisperSess.player == nil {
			whisperSess = nil
		}
	}

	// 1. Play Sound
	if chosen.Sound > 0 && (!whispered || whisperSess != nil) {
		soundPkt := protocol.NewBuffer(4)
		soundPkt.WriteU32(chosen.Sound)
		if whispered {
			_ = whisperSess.write(uint16(protocol.OpcodeSMSG_PLAY_SOUND), soundPkt.Bytes(), true)
		} else {
			s.broadcastToNearby(uint16(protocol.OpcodeSMSG_PLAY_SOUND), soundPkt.Bytes(), nil)
		}
	}

	// 2. Play Emote
	if chosen.Emote > 0 {
		emotePkt := protocol.NewBuffer(12)
		emotePkt.WriteU32(chosen.Emote)
		emotePkt.WriteU64(creatureGUID)
		// CreatureTextMgr::SendEmote takes no whisper target (HandleEmoteCommand
		// broadcasts to nearby), so the emote stays broadcast on the whisper arm,
		// even when the chat message itself was dropped.
		s.broadcastToNearby(uint16(protocol.OpcodeSMSG_EMOTE), emotePkt.Bytes(), nil)
	}

	// 3. Broadcast chat message
	if whispered && whisperSess == nil {
		return
	}
	chatPkt := protocol.BuildMonsterChatMessage(chatType, uint32(chosen.Language), creatureGUID, creatureName, msg)
	if whispered {
		_ = whisperSess.write(uint16(protocol.OpcodeSMSG_MESSAGECHAT), chatPkt, true)
		return
	}
	s.broadcastToNearby(uint16(protocol.OpcodeSMSG_MESSAGECHAT), chatPkt, nil)
}
