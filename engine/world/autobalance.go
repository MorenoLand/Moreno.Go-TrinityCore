package world

import (
	"context"
	"fmt"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
)

// AutoBalance first slice (AutoBalance.cpp): module config wiring is in
// engine/config (AutoBalanceConfig); this file carries the PlayerScript and
// AllMapScript arms that need no creature-stat model yet:
//   - OnLogin announce (AutoBalance_PlayerScript::OnLogin)
//   - OnGiveXP dungeon XP scale-down (AutoBalance_PlayerScript::OnGiveXP)
//   - OnPlayerEnterAll/OnPlayerLeaveAll instance tracking + PlayerChangeNotify
//     (AutoBalance_AllMapScript) with the mapLevel latch from OnLevelChanged
//
// Deferred to a later slice (needs the creature stat model): forced-creature
// ID lists, ModifyCreatureAttributes (tanh scaling, level scaling, base-stat
// rebuild), and the UnitScript damage/heal multiplier hooks.

type autoBalanceInstanceKey struct {
	mapID      uint32
	instanceID uint32
}

type autoBalanceMapInfo struct {
	playerCount uint32
	mapLevel    uint8
}

func (s *Server) autoBalanceInfo(mapID, instanceID uint32) *autoBalanceMapInfo {
	key := autoBalanceInstanceKey{mapID: mapID, instanceID: instanceID}
	s.autoBalanceMu.Lock()
	defer s.autoBalanceMu.Unlock()
	if s.autoBalanceMaps == nil {
		s.autoBalanceMaps = make(map[autoBalanceInstanceKey]*autoBalanceMapInfo)
	}
	info, ok := s.autoBalanceMaps[key]
	if !ok {
		info = &autoBalanceMapInfo{}
		s.autoBalanceMaps[key] = info
	}
	return info
}

func autoBalanceIsGM(sess *session) bool {
	return sess != nil && sess.player != nil && sess.player.ExtraFlags&playerExtraGMOn != 0
}

// autoBalancePlayerEnter mirrors AutoBalance_AllMapScript::OnPlayerEnterAll:
// GMs never count, mapLevel latches to the highest entering player level, and
// the instance player count increments (the C++ count++ arm, not the
// commented-out GetPlayersCountExceptGMs recount).
func (s *Server) autoBalancePlayerEnter(sess *session, mapID, instanceID uint32) {
	if s == nil || !s.Config.AutoBalance.Enable || autoBalanceIsGM(sess) {
		return
	}
	info := s.autoBalanceInfo(mapID, instanceID)
	s.autoBalanceMu.Lock()
	if sess.player.Level > info.mapLevel {
		info.mapLevel = sess.player.Level
	}
	info.playerCount++
	count := info.playerCount
	s.autoBalanceMu.Unlock()
	if s.Config.AutoBalance.PlayerChangeNotify {
		if entry, found, err := s.Data.Map(mapID); err == nil && found && entry.IsDungeon() {
			offset := s.Config.AutoBalance.PlayerCountDifficultyOffset
			s.autoBalanceNotifyInstance(mapID, instanceID, fmt.Sprintf(
				"|cffFF0000 [AutoBalance]|r|cffFF8000 %s entered the Instance %s. Auto setting player count to %d (Player Difficulty Offset = %d) |r",
				sess.player.Name, entry.MapName, int64(count)+int64(offset), offset))
		}
	}
}

// autoBalancePlayerLeave mirrors AutoBalance_AllMapScript::OnPlayerLeaveAll:
// the count decrements, an emptied instance resets mapLevel to 0 (and skips
// the notify, like C++'s early return), otherwise dungeon players are told.
// The >0 guard is a deliberate safety: C++ would underflow the uint32 on a
// leave-without-enter, which would pin the count at 4 billion.
func (s *Server) autoBalancePlayerLeave(sess *session, mapID, instanceID uint32) {
	if s == nil || !s.Config.AutoBalance.Enable || autoBalanceIsGM(sess) {
		return
	}
	info := s.autoBalanceInfo(mapID, instanceID)
	s.autoBalanceMu.Lock()
	if info.playerCount > 0 {
		info.playerCount--
	}
	count := info.playerCount
	if count == 0 {
		info.mapLevel = 0
		s.autoBalanceMu.Unlock()
		return
	}
	s.autoBalanceMu.Unlock()
	if s.Config.AutoBalance.PlayerChangeNotify {
		if entry, found, err := s.Data.Map(mapID); err == nil && found && entry.IsDungeon() {
			name := ""
			if sess != nil && sess.player != nil {
				name = sess.player.Name
			}
			s.autoBalanceNotifyInstance(mapID, instanceID, fmt.Sprintf(
				"|cffFF0000 [-AutoBalance]|r|cffFF8000 %s left the Instance %s. Auto setting player count to %d (Player Difficulty Offset = %d) |r",
				name, entry.MapName, count, s.Config.AutoBalance.PlayerCountDifficultyOffset))
		}
	}
}

// autoBalanceTransferInstance fires the leave/enter pair for a completed far
// teleport, skipping the leave when the origin is unknown (login paths) or
// identical to the destination.
func (s *Server) autoBalanceTransferInstance(sess *session, oldMapID, oldInstanceID, newMapID, newInstanceID uint32) {
	if s == nil {
		return
	}
	if (oldMapID != 0 || oldInstanceID != 0) && (oldMapID != newMapID || oldInstanceID != newInstanceID) {
		s.autoBalancePlayerLeave(sess, oldMapID, oldInstanceID)
	}
	s.autoBalancePlayerEnter(sess, newMapID, newInstanceID)
}

// autoBalanceOnLevelChanged mirrors AutoBalance_PlayerScript::OnLevelChanged:
// the instance mapLevel latches up to the new level (the !enabled and
// LevelScaling==0 early-outs are C++-exact).
func (s *session) autoBalanceOnLevelChanged() {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	cfg := s.server.Config.AutoBalance
	if !cfg.Enable || cfg.LevelScaling == 0 {
		return
	}
	info := s.server.autoBalanceInfo(s.player.Map, s.player.InstanceID)
	s.server.autoBalanceMu.Lock()
	if info.mapLevel < s.player.Level {
		info.mapLevel = s.player.Level
	}
	s.server.autoBalanceMu.Unlock()
}

// autoBalanceInstancePlayerCount is the Go analog of
// Map::GetPlayersCountExceptGMs for one instance: worldReady, non-GM sessions
// on (mapID, instanceID).
func (s *Server) autoBalanceInstancePlayerCount(mapID, instanceID uint32) uint32 {
	if s == nil {
		return 0
	}
	var count uint32
	s.sessionsMu.RLock()
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID &&
			sess.player.InstanceID == instanceID && sess.player.ExtraFlags&playerExtraGMOn == 0 {
			count++
		}
	}
	s.sessionsMu.RUnlock()
	return count
}

func (s *Server) autoBalanceNotifyInstance(mapID, instanceID uint32, msg string) {
	if s == nil {
		return
	}
	s.sessionsMu.RLock()
	var targets []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID && sess.player.InstanceID == instanceID {
			targets = append(targets, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, target := range targets {
		target.sendSysMessage(msg)
	}
}

// autoBalanceInstanceMaxPlayers mirrors the max-players resolution used at
// worldport_instance.go (InstanceMap::GetMaxPlayers): the difficulty row
// overrides the map row when it carries a nonzero MaxPlayers.
func autoBalanceInstanceMaxPlayers(ctx context.Context, srv *Server, entry wotlk.MapEntry, difficulty uint8) uint32 {
	maxPlayers := entry.MaxPlayers
	if srv != nil {
		if diff, found, err := srv.Data.MapDifficulty(entry.ID, uint32(difficulty)); err == nil && found && diff.MaxPlayers != 0 {
			maxPlayers = diff.MaxPlayers
		}
	}
	return maxPlayers
}

// autoBalanceScaleDungeonXP mirrors AutoBalance_PlayerScript::OnGiveXP: with
// AutoBalance.DungeonScaleDownXP, dungeon kill XP scales by
// currentPlayerCount/maxPlayerCount so a solo player earns the same total XP
// as a full group. Note the C++ arm is NOT gated on AutoBalance.enable —
// only on victim && DungeonScaleDownXP — and that is preserved here.
func (s *Server) autoBalanceScaleDungeonXP(ctx context.Context, sess *session, amount uint32) uint32 {
	if s == nil || sess == nil || sess.player == nil || amount == 0 {
		return amount
	}
	entry, found, err := s.Data.Map(sess.player.Map)
	if err != nil || !found || !entry.IsDungeon() {
		return amount
	}
	maxPlayers := autoBalanceInstanceMaxPlayers(ctx, s, entry, sess.player.DungeonDifficulty)
	if maxPlayers == 0 {
		return amount
	}
	count := s.autoBalanceInstancePlayerCount(sess.player.Map, sess.player.InstanceID)
	return uint32(float64(amount) * float64(count) / float64(maxPlayers))
}
