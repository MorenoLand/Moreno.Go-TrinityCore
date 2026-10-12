package world

import (
	"context"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Instance-validity homebind subsystem.
//
// C++ owners:
//   - Player::UpdateHomebindTime (Player.cpp:22106-22147): while the player's
//     instance is invalid a 60s SMSG_RAID_GROUP_ONLY countdown runs, ending in
//     RepopAtGraveyard; regaining validity hides the reminder (0,0).
//   - Group::_homebindIfInstance (Group.cpp:2387-2391): group removal (and
//     disband) inside a dungeon map marks the player's instance invalid.
//   - Group::AddMember (Group.cpp:451): re-adding a player recomputes validity
//     via Player::CheckInstanceValidity.
//   - Player::CheckInstanceValidity (Player.cpp:19370-19418): GM/dungeon/
//     raid-group/group-bind/other-players arms.
//   - Player::LoadFromDB (Player.cpp:17831-17835): validity is rechecked once
//     the login map position is determined.
//   - WorldSession::LogoutPlayer (WorldSession.cpp:540-542): a player whose
//     instance is invalid is teleported to their homebind on logout.
//   - MovementHandler (MovementHandler.cpp:69-70): leaving the instance
//     restores validity on the next movement packet.
//
// session.instanceValid mirrors Player::m_InstanceValid (Player.h, init true at
// Player.cpp:324); session.homebindTimerMs mirrors Player::m_HomebindTimer.

const homebindTimerDurationMs = 60000

// sessionIsGameMaster mirrors the IsGameMaster arms in CheckInstanceValidity /
// UpdateHomebindTime via the drowning.go:86 GM test.
func sessionIsGameMaster(s *session) bool {
	if s == nil || s.player == nil {
		return false
	}
	return (s.player.ExtraFlags&playerExtraGMOn != 0) || (s.player.PlayerFlags&playerFlagGM != 0) || s.security > 0
}

// sendRaidGroupOnly mirrors the two SMSG_RAID_GROUP_ONLY forms in
// UpdateHomebindTime: (remainingMs, 1) shows the countdown reminder,
// (0, 0) hides it.
func (s *session) sendRaidGroupOnly(remainingMs, flag uint32) {
	buf := protocol.NewBuffer(8)
	buf.WriteU32(remainingMs)
	buf.WriteU32(flag)
	_ = s.write(uint16(protocol.OpcodeSMSG_RAID_GROUP_ONLY), buf.Bytes(), true)
}

// updateHomebindTimers runs Player::UpdateHomebindTime for every connected
// player from the world tick (Player::Update, Player.cpp:1316).
func (s *Server) updateHomebindTimers(ctx context.Context, diff time.Duration) {
	if s == nil {
		return
	}
	diffMs := uint32(diff / time.Millisecond)
	if diffMs == 0 {
		diffMs = 1
	}
	s.sessionsMu.RLock()
	due := make([]*session, 0, len(s.sessions))
	for sess := range s.sessions {
		if sess != nil && sess.playerLoaded && sess.player != nil && !sess.instanceValid {
			due = append(due, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range due {
		sess.updateHomebindTimer(ctx, diffMs)
	}
}

func (s *session) updateHomebindTimer(ctx context.Context, diffMs uint32) {
	// GMs never get the homebind timer; a regained-valid instance resets a
	// running timer and hides the reminder (Player.cpp:22108-22119).
	if s.instanceValid || sessionIsGameMaster(s) {
		if s.homebindTimerMs != 0 {
			s.homebindTimerMs = 0
			s.sendRaidGroupOnly(0, 0)
		}
		return
	}
	if s.homebindTimerMs > 0 {
		if diffMs >= s.homebindTimerMs {
			s.homebindTimerMs = 0
			// teleport to nearest graveyard (Player.cpp:22126-22128)
			s.repopAtGraveyard(ctx)
		} else {
			s.homebindTimerMs -= diffMs
		}
		return
	}
	// instance is invalid, start homebind timer (Player.cpp:22131-22145)
	s.homebindTimerMs = homebindTimerDurationMs
	s.sendRaidGroupOnly(homebindTimerDurationMs, 1)
}

// homebindIfInstance mirrors Group::_homebindIfInstance: a player removed
// from (or disbanded out of) a group while inside a dungeon map is marked
// instance-invalid, which starts the homebind countdown.
func (s *Server) homebindIfInstance(target *session) {
	if s == nil || target == nil || target.player == nil {
		return
	}
	if sessionIsGameMaster(target) {
		return
	}
	if dungeonMapID(s, target.player.Map) {
		target.instanceValid = false
	}
}

// checkInstanceValidity mirrors Player::CheckInstanceValidity
// (Player.cpp:19370-19418).
func (s *session) checkInstanceValidity(ctx context.Context, g *groupState) bool {
	// game masters' instances are always valid
	if sessionIsGameMaster(s) {
		return true
	}
	// non-instances are always valid
	mapID := s.player.Map
	if !dungeonMapID(s.server, mapID) {
		return true
	}
	isRaid := raidMapID(s.server, mapID)
	difficulty := s.player.DungeonDifficulty
	if isRaid {
		difficulty = s.player.RaidDifficulty
		// raid instances require the player to be in a raid group to be valid
		// (the CONFIG_INSTANCE_IGNORE_RAID config has no Go twin)
		if g == nil || !g.IsRaid {
			return false
		}
	}
	instanceID := s.player.InstanceID
	if g != nil {
		// check if player's group is bound to this instance
		boundID, ok := s.groupBoundInstanceID(ctx, g.DBID, mapID, difficulty)
		if !ok || boundID != instanceID {
			return false
		}
		// ignore players that currently have a homebind timer active; anyone
		// else in this instance copy outside this group invalidates
		return !s.otherPlayerInInstance(g)
	}
	// instance is invalid if we are not grouped and there are other players
	if s.otherPlayerInInstance(nil) {
		return false
	}
	// check if the player is bound to this instance
	boundID, ok := s.playerBoundInstanceID(ctx, mapID, difficulty)
	return ok && boundID == instanceID
}

// otherPlayerInInstance mirrors the map player-list arms of
// CheckInstanceValidity: any other non-GM, instance-valid player in the same
// map instance copy, optionally restricted to a different group than g.
func (s *session) otherPlayerInInstance(g *groupState) bool {
	key := s.worldInstance.Load()
	s.server.sessionsMu.RLock()
	defer s.server.sessionsMu.RUnlock()
	for sess := range s.server.sessions {
		if sess == s || sess == nil || !sess.playerLoaded || sess.player == nil {
			continue
		}
		if sessionIsGameMaster(sess) || !sess.instanceValid {
			continue
		}
		if sess.worldInstance.Load() != key {
			continue
		}
		if g == nil || sess.groupID != g.ID {
			return true
		}
	}
	return false
}

// groupBoundInstanceID mirrors Group::GetBoundInstance(difficulty, mapId) for
// the group-bind arm of CheckInstanceValidity (group_instance JOIN instance).
func (s *session) groupBoundInstanceID(ctx context.Context, dbGroupID uint32, mapID uint32, difficulty uint8) (uint32, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || dbGroupID == 0 {
		return 0, false
	}
	var id int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id FROM group_instance AS gi JOIN instance AS i ON i.id = gi.instance WHERE gi.guid = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`, dbGroupID, mapID, difficulty).Scan(&id)
	if err != nil {
		return 0, false
	}
	return uint32(id), true
}

// playerBoundInstanceID mirrors Player::GetBoundInstance(map, difficulty)
// (findInstanceBind in spells.go scans loaded rows; the DB form here matches
// the map_admission.go:363 bind lookup).
func (s *session) playerBoundInstanceID(ctx context.Context, mapID uint32, difficulty uint8) (uint32, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0, false
	}
	var id int64
	err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT i.id FROM character_instance AS ci JOIN instance AS i ON i.id = ci.instance WHERE ci.guid = ? AND i.map = ? AND i.difficulty = ? LIMIT 1`, s.playerGUID, mapID, difficulty).Scan(&id)
	if err != nil {
		return 0, false
	}
	return uint32(id), true
}

// instanceLoginGameMasterException mirrors
// Player::IsInstanceLoginGameMasterException (Player.cpp:19359-19366): a GM
// keeps a technically invalid instance on login.
func (s *session) instanceLoginGameMasterException() bool {
	if s != nil && s.security > 0 {
		s.sendSystemMessage("You are a game master, instance login restrictions do not apply.")
		return true
	}
	return false
}
