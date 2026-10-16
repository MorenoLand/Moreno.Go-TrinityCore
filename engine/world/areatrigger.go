package world

import (
	"context"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// handleAreaTrigger processes CMSG_AREATRIGGER (0x0B4).
// Reference: WorldSession::HandleAreaTriggerOpcode (MiscHandler.cpp:645-780).
func (s *session) handleAreaTrigger(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	triggerID, err := r.ReadU32()
	if err != nil {
		// Truncated packet: WorldSession::Update catches ByteBufferException and
		// skips the packet (WorldSession.cpp:425-432) — drop, conn open.
		return true
	}

	// 1. In-flight check: players in flight ignore area triggers
	// Reference: MiscHandler.cpp:653-658
	if s.isInFlight() {
		s.debug("areatrigger ignored: player in flight", "account", s.accountName, "trigger", triggerID)
		return true
	}

	// 2. DBC Radius & Oriented Bounding Box Validation
	// Reference: MiscHandler.cpp:660-673 and Player::IsInAreaTriggerRadius (Player.cpp:2417).
	// Unknown trigger IDs are ignored before any arm runs (MiscHandler.cpp:660-665).
	if s.server == nil || s.server.Data == nil {
		return true
	}
	atEntry, found, err := s.server.Data.AreaTrigger(triggerID)
	if err != nil || !found {
		s.debug("areatrigger ignored: unknown trigger", "account", s.accountName, "trigger", triggerID)
		return true
	}
	if !atEntry.IsInAreaTriggerRadius(s.player.Map, s.player.X, s.player.Y, s.player.Z) {
		s.debug("areatrigger ignored: out of radius", "account", s.accountName, "trigger", triggerID)
		return true
	}

	// 2b. Script hook arm
	// Reference: ScriptMgr::OnAreaTrigger (ScriptMgr.cpp:1702-1713): an
	// Eluna true return makes ScriptMgr return false, so the quest/tavern/
	// BG/teleport arms below CONTINUE — only a native AreaTriggerScript
	// true vetoes them. Go has no native AreaTrigger scripts, so the Lua
	// hook fires but never vetoes (the old code had this inverted).
	if s.server != nil && s.server.Features != nil && s.server.Features.Scripts != nil {
		_, _ = s.server.Features.Scripts.TriggerServerEvent(ctx, scripting.ServerEventAreaTrigger, s.luaPlayer(), triggerID)
	}

	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return true
	}

	// 3. Area explored / Quest objective completion
	// Reference: MiscHandler.cpp:681-685 (sObjectMgr->GetQuestForAreaTrigger):
	// the whole arm sits under player->IsAlive() — a dead player touching
	// a trigger does not complete quest objectives.
	var questID uint32
	if err := wdb.QueryRowContext(ctx, "SELECT quest FROM areatrigger_involvedrelation WHERE id = ?", triggerID).Scan(&questID); err == nil && questID != 0 && !s.isDeadOrGhost() {
		status, _ := s.characterQuestStatus(ctx, questID)
		if status == questStatusIncomplete {
			// Player::AreaExploredOrEventHappens (Player.cpp:16503): the
			// explore objective fires SendQuestComplete (quest id on the
			// wire, Player.cpp:17003-17009), and the quest completes only
			// when all objectives are done. Go has no per-quest Explored
			// objective model, so the update-complete broadcast rides
			// along with the standing completeQuest path.
			s.sendQuestComplete(questID)
			s.completeQuest(ctx, questID)
		}
	}

	// 4. Tavern / Inn resting trigger
	// Reference: MiscHandler.cpp:686-695 (sObjectMgr->IsTavernAreaTrigger)
	var tavernID uint32
	if err := wdb.QueryRowContext(ctx, "SELECT id FROM areatrigger_tavern WHERE id = ?", triggerID).Scan(&tavernID); err == nil && tavernID != 0 {
		s.innTriggerID = triggerID
		s.setRestingFlag(s.player, true)
		// WorldSession::HandleAreaTriggerOpcode (MiscHandler.cpp:686-695):
		// inn rest clears the FFA PvP byte flag on FFA realms.
		if (s.server.Config.GameType == 4 || s.server.Config.GameType == 6) && s.player.PVPFlags&pvpFlagFFA != 0 {
			s.player.PVPFlags &^= pvpFlagFFA
		}
		s.sendPlayerUpdate()
		return true
	}

	// 4b. Battleground area-trigger arm
	// Reference: MiscHandler.cpp:697-700 — runs after the tavern return, does not
	// return itself (execution continues to the teleport arm). Battleground::
	// HandleAreaTrigger is empty, so only arena map scripts have trigger arms.
	if arena := s.server.findArenaState(s.player.Map, 0); arena != nil {
		arena.mu.RLock()
		inProgress := arena.Status == ArenaStatusInProgress
		mapID := arena.MapID
		arena.mu.RUnlock()
		if inProgress {
			s.handleArenaAreaTrigger(triggerID, mapID)
		}
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
			// MiscHandler.cpp:766-768: an LFG-group member standing in a
			// dungeon is redirected to the battleground entry point instead
			// of the trigger target. An invalid entry point (JoinMap == 0,
			// the MAPID_INVALID leg of Player::TeleportToBGEntryPoint,
			// Player.cpp:1896-1906) falls through to the normal teleport.
			if g := s.server.findGroupByID(s.groupID); g != nil && g.IsLFG && s.bgData.JoinMap != 0 {
				if mapEntry, found, mapErr := s.server.Data.Map(s.player.Map); mapErr == nil && found && mapEntry.IsDungeon() {
					s.teleportTo(s.bgData.JoinMap, s.bgData.JoinX, s.bgData.JoinY, s.bgData.JoinZ, s.bgData.JoinO)
					return true
				}
			}
		}
		s.teleportTo(uint32(targetMap), float32(targetX), float32(targetY), float32(targetZ), float32(targetOri))
		return true
	}

	s.debug("areatrigger handled", "account", s.accountName, "trigger", triggerID)
	return true
}
