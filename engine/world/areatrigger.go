package world

import (
	"context"
	"fmt"

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

	// 2b. Debug area-trigger reporting
	// Reference: MiscHandler.cpp:675 — isDebugAreaTriggers (toggled by
	// .debug areatriggers, cs_debug.cpp:432) reports the reached trigger id
	// (LANG_DEBUG_AREATRIGGER_REACHED, 1204; unseeded in-tree, so plain
	// prose like the other unseeded LANG_* notices).
	if s.player.debugAreaTriggers {
		s.sendSysMessage(fmt.Sprintf("Area Trigger Reached: %d", triggerID))
	}

	// 2c. Script hook arm
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

	// 4c. Arathi Basin exit portals
	// Reference: BattlegroundAB::HandleAreaTrigger (BattlegroundAB.cpp:259-270):
	// 3948 is the alliance exit, 3949 the horde exit; the wrong team gets the
	// portal message instead of leaving. The remaining trigger set in that
	// switch (3866/3867/3868/3869/3870/4020/4021/4674) is commented out in C++
	// and falls through to the empty Battleground::HandleAreaTrigger — no
	// arms to port. No STATUS_IN_PROGRESS gate here: Go's AB state has no
	// start/stop lifecycle, and leaving the zone mid-simulation is still the
	// correct action.
	if s.player.Map == ABMapID && (triggerID == 3948 || triggerID == 3949) {
		allianceExit := triggerID == 3948
		if teamForRace(s.player.Race) == 0 && allianceExit || teamForRace(s.player.Race) == 1 && !allianceExit {
			s.handleLeaveBattlefield(ctx, nil, false, false)
		} else {
			teamName := "Alliance"
			if !allianceExit {
				teamName = "Horde"
			}
			_ = s.write(uint16(protocol.OpcodeSMSG_AREA_TRIGGER_MESSAGE),
				protocol.BuildAreaTriggerMessage("Only The "+teamName+" can use that portal"), true)
		}
	}
	// 4d. Alterac Valley exit portals
	// Reference: BattlegroundAV::HandleAreaTrigger (BattlegroundAV.cpp:508-535):
	// 95/2608 is the alliance exit, 2606 the horde exit; the wrong team gets
	// the portal message instead of leaving. 3326-3331 are C++ no-ops — no arms
	// to port. No STATUS_IN_PROGRESS gate here: Go's AV state has no
	// start/stop lifecycle (same convention as the AB arm above).
	if s.player.Map == AVMapID && (triggerID == 95 || triggerID == 2608 || triggerID == 2606) {
		allianceExit := triggerID != 2606
		if teamForRace(s.player.Race) == 0 && allianceExit || teamForRace(s.player.Race) == 1 && !allianceExit {
			s.handleLeaveBattlefield(ctx, nil, false, false)
		} else {
			teamName := "Alliance"
			if !allianceExit {
				teamName = "Horde"
			}
			_ = s.write(uint16(protocol.OpcodeSMSG_AREA_TRIGGER_MESSAGE),
				protocol.BuildAreaTriggerMessage("Only The "+teamName+" can use that portal"), true)
		}
	}
	// 4e. Isle of Conquest back-door achievement
	// Reference: BattlegroundIC::HandleAreaTrigger (BattlegroundIC.cpp:270-291):
	// 5555 (Horde, at the Alliance keep) / 5535 (Alliance, at the Horde keep)
	// credit the Back Door Job achievement when none of the enemy's three gates
	// is destroyed. Like the arms above, this does not return: execution
	// continues to the teleport arm below.
	if s.player.Map == ICMapID && (triggerID == ICAreaTriggerBackDoorHorde || triggerID == ICAreaTriggerBackDoorAlliance) {
		s.server.handleICBackDoorTrigger(s, triggerID)
	}
	// Like the arena arm above (MiscHandler.cpp:697-700), the BG arm does not
	// return: execution continues to the teleport arm below.

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
