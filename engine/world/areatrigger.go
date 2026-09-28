package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleAreaTrigger processes CMSG_AREATRIGGER (0x0B4).
// Reference: WorldSession::HandleAreaTriggerOpcode (MiscHandler.cpp:645-780).
func (s *session) handleAreaTrigger(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return false
	}
	r := protocol.NewReader(payload)
	triggerID, err := r.ReadU32()
	if err != nil {
		return false
	}

	// 1. In-flight check: players in flight ignore area triggers
	// Reference: MiscHandler.cpp:653-658
	if s.isInFlight() {
		s.debug("areatrigger ignored: player in flight", "account", s.accountName, "trigger", triggerID)
		return true
	}

	// 2. DBC Radius & Oriented Bounding Box Validation
	// Reference: MiscHandler.cpp:660-673 and Player::IsInAreaTriggerRadius (Player.cpp:2417)
	if s.server != nil && s.server.Data != nil {
		atEntry, found, err := s.server.Data.AreaTrigger(triggerID)
		if err == nil && found {
			if !atEntry.IsInAreaTriggerRadius(s.player.Map, s.player.X, s.player.Y, s.player.Z) {
				s.debug("areatrigger ignored: out of radius", "account", s.accountName, "trigger", triggerID)
				return true
			}
		}
	}

	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}

	// 3. Area explored / Quest objective completion
	// Reference: MiscHandler.cpp:681-685 (sObjectMgr->GetQuestForAreaTrigger)
	var questID uint32
	if err := wdb.QueryRowContext(ctx, "SELECT quest FROM areatrigger_involvedrelation WHERE id = ?", triggerID).Scan(&questID); err == nil && questID != 0 {
		status, _ := s.characterQuestStatus(ctx, questID)
		if status == questStatusIncomplete {
			cdb := s.server.CharactersStore.DB
			if cdb != nil {
				_, _ = cdb.ExecContext(ctx, "UPDATE character_queststatus SET status = ? WHERE guid = ? AND quest = ?", questStatusComplete, s.playerGUID, questID)
			}
			for slot := 0; slot < playerQuestLogSlots; slot++ {
				if s.player.QuestLog[slot].QuestID == questID {
					s.player.QuestLog[slot].State = questCompleteStateFlag(questStatusComplete)
					s.sendPlayerQuestLogUpdate(slot)
					break
				}
			}
		}
	}

	// 4. Tavern / Inn resting trigger
	// Reference: MiscHandler.cpp:686-695 (sObjectMgr->IsTavernAreaTrigger)
	var tavernID uint32
	if err := wdb.QueryRowContext(ctx, "SELECT id FROM areatrigger_tavern WHERE id = ?", triggerID).Scan(&tavernID); err == nil && tavernID != 0 {
		s.player.PlayerFlags |= playerFlagResting
		s.sendPlayerUpdate()
		return true
	}

	// 5. Teleport trigger
	// Reference: MiscHandler.cpp:705-779 (areatrigger_teleport)
	var targetMap int64
	var targetX, targetY, targetZ, targetOri float64
	if err := wdb.QueryRowContext(ctx, "SELECT target_map, target_position_x, target_position_y, target_position_z, target_orientation FROM areatrigger_teleport WHERE id = ?", triggerID).Scan(&targetMap, &targetX, &targetY, &targetZ, &targetOri); err == nil {
		if uint32(targetMap) != s.player.Map {
			check := s.playerCannotEnterMap(ctx, uint32(targetMap))
			if check.Reason != mapEntryAllowed {
				s.sendAreaTriggerEntryFailure(ctx, uint32(targetMap), check)
				return true
			}
		}
		s.teleportTo(uint32(targetMap), float32(targetX), float32(targetY), float32(targetZ), float32(targetOri))
		return true
	}

	s.debug("areatrigger handled", "account", s.accountName, "trigger", triggerID)
	return true
}
